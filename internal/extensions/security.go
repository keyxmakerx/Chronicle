package extensions

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Security limits for extension installation.
const (
	DefaultMaxZipSize       = 50 * 1024 * 1024  // 50 MB.
	DefaultMaxExtractedSize = 100 * 1024 * 1024 // 100 MB.
	DefaultMaxFiles         = 1000
	DefaultMaxFileSize      = 20 * 1024 * 1024  // 20 MB per file.
	DefaultMaxCSSSize       = 500 * 1024         // 500 KB per CSS file.
	DefaultMaxEntityPack    = 10000              // Max entities in a pack.
)

// allowedFileExts is the strict allowlist of file extensions for extensions.
// Layer 1 (content) extensions use JSON/CSS/images. Layer 2 (widget)
// extensions additionally use .js files that register via Chronicle.registerWidget().
var allowedFileExts = map[string]bool{
	".json": true,
	".css":  true,
	".svg":  true,
	".png":  true,
	".webp": true,
	".jpg":  true,
	".jpeg": true,
	".txt":  true,
	".md":   true,
	".js":   true,
}

// ValidateZipEntry checks that a zip entry path is safe (no traversal,
// no absolute paths, allowed file type).
func ValidateZipEntry(name string) error {
	if name == "" {
		return fmt.Errorf("empty file name")
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("absolute path not allowed: %s", name)
	}
	cleaned := filepath.Clean(name)
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path traversal not allowed: %s", name)
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("leading slash not allowed: %s", name)
	}
	// Skip directory entries.
	if strings.HasSuffix(name, "/") {
		return nil
	}
	// Reject dotfiles (e.g., .env, .git, .htaccess).
	base := filepath.Base(name)
	if strings.HasPrefix(base, ".") {
		return fmt.Errorf("dotfiles not allowed: %s", name)
	}
	// Check file extension against allowlist.
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedFileExts[ext] {
		return fmt.Errorf("file type %q not allowed: %s", ext, name)
	}
	return nil
}

// ExtractZip safely extracts a zip file to the target directory.
// Validates every entry for path safety and file type. Returns the
// parsed manifest from the extracted contents.
func ExtractZip(zipPath, targetDir string) (*ExtensionManifest, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("opening zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	if len(r.File) > DefaultMaxFiles {
		return nil, fmt.Errorf("too many files in zip: %d (max %d)", len(r.File), DefaultMaxFiles)
	}

	// Determine if all files share a common root directory (common in zip tools).
	commonPrefix := detectCommonPrefix(r.File)

	var totalSize int64
	var manifestData []byte

	for _, f := range r.File {
		name := f.Name
		// Strip common prefix if present (e.g., "my-extension/" prefix).
		if commonPrefix != "" {
			name = strings.TrimPrefix(name, commonPrefix)
			if name == "" {
				continue // Skip the root directory entry itself.
			}
		}

		// Validate each entry for safety.
		if err := ValidateZipEntry(name); err != nil {
			return nil, fmt.Errorf("unsafe zip entry: %w", err)
		}

		destPath := filepath.Join(targetDir, filepath.Clean(name))

		// Ensure the destination is within the target directory.
		if !strings.HasPrefix(destPath, filepath.Clean(targetDir)+string(filepath.Separator)) {
			return nil, fmt.Errorf("zip entry escapes target directory: %s", name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return nil, fmt.Errorf("creating directory %s: %w", name, err)
			}
			continue
		}

		// Check individual file size.
		if f.UncompressedSize64 > uint64(DefaultMaxFileSize) {
			return nil, fmt.Errorf("file %s too large: %d bytes (max %d)", name, f.UncompressedSize64, DefaultMaxFileSize)
		}
		totalSize += int64(f.UncompressedSize64)
		if totalSize > int64(DefaultMaxExtractedSize) {
			return nil, fmt.Errorf("total extracted size exceeds limit (%d bytes)", DefaultMaxExtractedSize)
		}

		// Create parent directories.
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return nil, fmt.Errorf("creating parent dir for %s: %w", name, err)
		}

		// Extract the file.
		data, err := extractFile(f)
		if err != nil {
			return nil, fmt.Errorf("extracting %s: %w", name, err)
		}

		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", name, err)
		}

		// Capture manifest.json.
		if name == "manifest.json" {
			manifestData = data
		}
	}

	if manifestData == nil {
		return nil, fmt.Errorf("manifest.json not found in zip")
	}

	manifest, err := ParseManifest(manifestData)
	if err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	return manifest, nil
}

// extractFile reads the contents of a zip file entry.
func extractFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	// Limit read to DefaultMaxFileSize + 1 to detect oversized files.
	data, err := io.ReadAll(io.LimitReader(rc, int64(DefaultMaxFileSize)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > DefaultMaxFileSize {
		return nil, fmt.Errorf("file exceeds max size")
	}
	return data, nil
}

// detectCommonPrefix checks if all zip entries share a single root directory.
// Returns the prefix (e.g., "my-extension/") or empty string.
func detectCommonPrefix(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}

	// Find the first entry that isn't a directory.
	var first string
	for _, f := range files {
		if !f.FileInfo().IsDir() {
			first = f.Name
			break
		}
	}
	if first == "" {
		return ""
	}

	// Extract the top-level directory.
	parts := strings.SplitN(first, "/", 2)
	if len(parts) < 2 {
		return "" // No subdirectory structure.
	}
	prefix := parts[0] + "/"

	// Check if ALL entries start with this prefix.
	for _, f := range files {
		if !strings.HasPrefix(f.Name, prefix) {
			return "" // Not a common prefix.
		}
	}

	return prefix
}

// ValidateCSS checks that extension CSS is safe (no @import, no external
// url(), no expression(), no behavior:).
func ValidateCSS(content []byte) error {
	s := string(content)

	if strings.Contains(s, "@import") {
		return fmt.Errorf("@import is not allowed in extension CSS")
	}

	// Check for url() calls that reference external resources.
	// Allow data: URIs but block everything else.
	urlPattern := regexp.MustCompile(`url\s*\(\s*(['"]?)`)
	matches := urlPattern.FindAllStringIndex(s, -1)
	for _, m := range matches {
		// Extract what follows url( to check if it's a data: URI.
		after := s[m[1]:]
		if !strings.HasPrefix(after, "data:") && !strings.HasPrefix(strings.TrimLeft(after, "'\""), "data:") {
			return fmt.Errorf("url() with external resources is not allowed")
		}
	}

	lower := strings.ToLower(s)
	if strings.Contains(lower, "expression(") {
		return fmt.Errorf("expression() is not allowed")
	}
	if strings.Contains(lower, "behavior:") {
		return fmt.Errorf("behavior: is not allowed")
	}

	return nil
}

// ValidateSVG checks that an SVG file doesn't contain embedded scripts.
func ValidateSVG(content []byte) error {
	lower := strings.ToLower(string(content))

	if strings.Contains(lower, "<script") {
		return fmt.Errorf("SVG must not contain <script> tags")
	}
	if strings.Contains(lower, "javascript:") {
		return fmt.Errorf("SVG must not contain javascript: URIs")
	}

	// Check for event handler attributes (onclick, onload, onerror, etc.).
	eventPattern := regexp.MustCompile(`\bon\w+\s*=`)
	if eventPattern.MatchString(lower) {
		return fmt.Errorf("SVG must not contain event handler attributes")
	}

	return nil
}

// ValidateExtractedFiles performs post-extraction validation on content files.
// Checks CSS and SVG files for unsafe content.
func ValidateExtractedFiles(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))

		switch ext {
		case ".css":
			if info.Size() > int64(DefaultMaxCSSSize) {
				return fmt.Errorf("CSS file %s exceeds size limit (%d bytes)", filepath.Base(path), DefaultMaxCSSSize)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading CSS %s: %w", filepath.Base(path), err)
			}
			if err := ValidateCSS(data); err != nil {
				return fmt.Errorf("CSS %s: %w", filepath.Base(path), err)
			}

		case ".svg":
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading SVG %s: %w", filepath.Base(path), err)
			}
			if err := ValidateSVG(data); err != nil {
				return fmt.Errorf("SVG %s: %w", filepath.Base(path), err)
			}
		}

		return nil
	})
}
