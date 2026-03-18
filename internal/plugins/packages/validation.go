package packages

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// --- URL Validation ---

// knownGitHosts is the allowlist of public git hosting providers for "any_git" policy.
var knownGitHosts = map[string]bool{
	"github.com":    true,
	"gitlab.com":    true,
	"bitbucket.org": true,
	"codeberg.org":  true,
	"sr.ht":         true,
}

// gitHostPattern matches common git hosting URL patterns.
var gitHostPattern = regexp.MustCompile(`^https://[^/]+/[^/]+/[^/]+`)

// ValidateRepoURL checks the submitted URL against the site's repo policy.
// Returns nil if the URL is valid for the given policy.
func ValidateRepoURL(repoURL, policy string) error {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return fmt.Errorf("repository URL is required")
	}

	parsed, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// All policies require HTTPS.
	if parsed.Scheme != "https" {
		return fmt.Errorf("only HTTPS URLs are allowed")
	}

	// Block private/loopback IPs to prevent SSRF.
	if err := validateHostNotPrivate(parsed.Hostname()); err != nil {
		return err
	}

	switch policy {
	case RepoPolicyGitHubOnly:
		if parsed.Hostname() != "github.com" {
			return fmt.Errorf("only GitHub repositories are allowed (current policy: GitHub Only)")
		}
		// Must match github.com/owner/repo pattern.
		if !repoPattern.MatchString(repoURL) {
			return fmt.Errorf("invalid GitHub repository URL format (expected: https://github.com/owner/repo)")
		}

	case RepoPolicyAnyGit:
		if !knownGitHosts[parsed.Hostname()] {
			return fmt.Errorf("repository host %q is not in the allowed list (policy: Known Git Hosts Only)", parsed.Hostname())
		}
		if !gitHostPattern.MatchString(repoURL) {
			return fmt.Errorf("URL must point to a specific repository (e.g., https://github.com/owner/repo)")
		}

	case RepoPolicyAllowAll:
		// Any HTTPS URL is fine. Private IP check above is sufficient.
		if !gitHostPattern.MatchString(repoURL) {
			return fmt.Errorf("URL must point to a specific repository")
		}

	default:
		// Unknown policy — default to most restrictive.
		return ValidateRepoURL(repoURL, RepoPolicyGitHubOnly)
	}

	return nil
}

// validateHostNotPrivate resolves the hostname and rejects private/loopback IPs
// to prevent SSRF attacks via user-supplied URLs.
func validateHostNotPrivate(hostname string) error {
	// Block obvious private hostnames without DNS lookup.
	lower := strings.ToLower(hostname)
	if lower == "localhost" || lower == "127.0.0.1" || lower == "[::1]" || lower == "::1" {
		return fmt.Errorf("localhost URLs are not allowed")
	}

	// Resolve the hostname and check each IP.
	ips, err := net.LookupHost(hostname)
	if err != nil {
		// DNS resolution failure — allow it through (the download will fail later).
		// This prevents blocking legitimate hosts that are temporarily unresolvable.
		return nil
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL resolves to a private/loopback IP address (%s) — this is not allowed", ipStr)
		}
	}

	return nil
}

// --- Content Validation ---

// dangerousExtensions is the set of file extensions that indicate executable
// or potentially malicious content. Packages should only contain data files.
var dangerousExtensions = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".com": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true,
	".dll": true, ".so": true, ".dylib": true,
	".msi": true, ".deb": true, ".rpm": true,
	".ps1": true, ".psm1": true, ".psd1": true,
	".jar": true, ".class": true,
	".py": true, ".rb": true, ".pl": true,
}

// ValidatePackageContents checks extracted package files for safety.
// validateManifest requires a manifest.json at root.
// scanContent rejects executables, symlinks, and oversized files.
func ValidatePackageContents(extractDir string, validateManifest, scanContent bool, maxFileSize int64) error {
	if validateManifest {
		if err := validateManifestFile(extractDir); err != nil {
			return fmt.Errorf("manifest validation failed: %w", err)
		}
	}

	if scanContent {
		if err := scanContentFiles(extractDir, maxFileSize); err != nil {
			return fmt.Errorf("content scan failed: %w", err)
		}
	}

	return nil
}

// validateManifestFile checks that a valid manifest exists at the package root.
// Accepts manifest.json (Chronicle system packs) or module.json (Foundry modules).
func validateManifestFile(dir string) error {
	// Try manifest.json first (Chronicle system packs), then module.json (Foundry modules).
	manifestPath := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		manifestPath = filepath.Join(dir, "module.json")
		data, err = os.ReadFile(manifestPath)
		if err != nil {
			return fmt.Errorf("no manifest found at package root (expected manifest.json or module.json)")
		}
	}

	// Verify it's valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("%s is not valid JSON: %w", filepath.Base(manifestPath), err)
	}

	// Check for required fields.
	if _, ok := parsed["id"]; !ok {
		return fmt.Errorf("%s missing required field: id", filepath.Base(manifestPath))
	}
	if _, ok := parsed["name"]; !ok {
		// Foundry modules use "title" instead of "name".
		if _, ok := parsed["title"]; !ok {
			return fmt.Errorf("%s missing required field: name (or title)", filepath.Base(manifestPath))
		}
	}

	return nil
}

// scanContentFiles walks the extracted directory and rejects dangerous files.
func scanContentFiles(dir string, maxFileSize int64) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Reject symlinks.
		if info.Mode()&os.ModeSymlink != 0 {
			rel, _ := filepath.Rel(dir, path)
			return fmt.Errorf("symlinks are not allowed: %s", rel)
		}

		if info.IsDir() {
			return nil
		}

		// Check file extension.
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if dangerousExtensions[ext] {
			rel, _ := filepath.Rel(dir, path)
			return fmt.Errorf("potentially dangerous file type not allowed: %s", rel)
		}

		// Check file size.
		if maxFileSize > 0 && info.Size() > maxFileSize {
			rel, _ := filepath.Rel(dir, path)
			return fmt.Errorf("file exceeds maximum size (%d bytes): %s", maxFileSize, rel)
		}

		return nil
	})
}
