package systems

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// A book can ship its own cover art (an SVG) and font files (woff2) in
// book/art and book/fonts. book/ is never served as a folder; only the files
// book.yaml declares are reachable, by BookAsset, so a package cannot make an
// arbitrary file under book/ public by guessing its name.

// bookAssetTypes maps a served file's extension to its content type.
var bookAssetTypes = map[string]string{
	".svg":   "image/svg+xml",
	".woff2": "font/woff2",
}

// setBookAssets checks the cover and font declarations in book.yaml against
// the files in the package, so a typo is reported to the author here rather
// than showing as a blank cover in the browser.
func setBookAssets(b *Book, idx bookIndexYAML, sysDir string) error {
	if idx.Cover != "" {
		if !bookCoverPattern.MatchString(idx.Cover) {
			return fmt.Errorf("cover must be art/<name>.svg using lower-case letters, digits and -, not %q", idx.Cover)
		}
		if err := checkBookAsset(sysDir, idx.Cover); err != nil {
			return fmt.Errorf("cover %s: %w", idx.Cover, err)
		}
		b.Cover = idx.Cover
	}
	if len(idx.Fonts) > maxBookFonts {
		return fmt.Errorf("more than %d fonts", maxBookFonts)
	}
	for i, f := range idx.Fonts {
		family := strings.TrimSpace(f.Family)
		if !bookFontPattern.MatchString(family) {
			return fmt.Errorf("font %d: family %q is not allowed", i+1, f.Family)
		}
		if !bookFontFile.MatchString(f.File) {
			return fmt.Errorf("font %d: file must be fonts/<name>.woff2 using lower-case letters, digits and -, not %q", i+1, f.File)
		}
		if f.Weight != 0 && (f.Weight < 100 || f.Weight > 900) {
			return fmt.Errorf("font %d: weight must be 100 to 900, not %d", i+1, f.Weight)
		}
		switch f.Style {
		case "", "normal", "italic":
		default:
			return fmt.Errorf("font %d: style must be normal or italic, not %q", i+1, f.Style)
		}
		if err := checkBookAsset(sysDir, f.File); err != nil {
			return fmt.Errorf("font %d (%s): %w", i+1, f.File, err)
		}
		b.Fonts = append(b.Fonts, BookFont{Family: family, URL: f.File, Weight: f.Weight, Style: f.Style})
	}
	return nil
}

// checkBookAsset confirms rel is a plain file of an allowed size. A symlink
// is refused so a package cannot point a declared name at a file elsewhere.
func checkBookAsset(sysDir, rel string) error {
	info, err := os.Lstat(filepath.Join(sysDir, bookDirName, filepath.FromSlash(rel)))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return errors.New("file not found in the package's book folder")
	case err != nil:
		return errors.New("file could not be read")
	case !info.Mode().IsRegular():
		return errors.New("is not a plain file")
	case info.Size() > maxBookAssetBytes:
		return fmt.Errorf("is larger than %d bytes", maxBookAssetBytes)
	}
	return nil
}

// WithAssetBase returns a copy of the book whose cover and font paths are
// URLs under base (the book's asset route, no trailing slash). The shared
// Fonts slice is copied so a cached book is never rewritten in place.
func (b *Book) WithAssetBase(base string) *Book {
	out := *b
	if b.Cover != "" {
		out.Cover = base + "/" + path.Base(b.Cover)
	}
	if len(b.Fonts) > 0 {
		out.Fonts = make([]BookFont, len(b.Fonts))
		for i, f := range b.Fonts {
			f.URL = base + "/" + path.Base(f.URL)
			out.Fonts[i] = f
		}
	}
	return &out
}

// BookAsset returns the bytes and content type of the cover or font file
// called name, but only if book.yaml declares it. Anything else is
// os.ErrNotExist, whatever is on disk.
func BookAsset(sysDir, name string) ([]byte, string, error) {
	if !bookAssetName.MatchString(name) {
		return nil, "", os.ErrNotExist
	}
	var idx bookIndexYAML
	if err := decodeBookFile(filepath.Join(sysDir, bookDirName, bookIndexFile), &idx); err != nil {
		return nil, "", os.ErrNotExist
	}
	declared := ""
	if m := bookCoverPattern.FindStringSubmatch(idx.Cover); m != nil && m[1] == name {
		declared = idx.Cover
	}
	for _, f := range idx.Fonts {
		if m := bookFontFile.FindStringSubmatch(f.File); m != nil && m[1] == name {
			declared = f.File
		}
	}
	if declared == "" {
		return nil, "", os.ErrNotExist
	}
	if err := checkBookAsset(sysDir, declared); err != nil {
		return nil, "", os.ErrNotExist
	}
	f, err := os.Open(filepath.Join(sysDir, bookDirName, filepath.FromSlash(declared)))
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxBookAssetBytes+1))
	if err != nil || len(data) > maxBookAssetBytes {
		return nil, "", os.ErrNotExist
	}
	// Content is checked against the extension so a mislabelled file is not
	// served under a type the browser would trust.
	ext := path.Ext(name)
	switch ext {
	case ".woff2":
		if !bytes.HasPrefix(data, []byte("wOF2")) {
			return nil, "", os.ErrNotExist
		}
	case ".svg":
		if !bytes.Contains(data[:min(len(data), 1024)], []byte("<svg")) {
			return nil, "", os.ErrNotExist
		}
	}
	return data, bookAssetTypes[ext], nil
}
