package systems

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
)

// bookWidgetCache keeps each system's set of book-placed widgets, rebuilt
// when a file under its book/ folder changes. The Customizer asks on every
// open, and re-reading every chapter each time is wasted work.
var bookWidgetCache = struct {
	sync.Mutex
	m map[string]bookWidgetEntry
}{m: map[string]bookWidgetEntry{}}

type bookWidgetEntry struct {
	version string
	slugs   map[string]bool
}

// bookWidgetSlugs returns the widgets the package book in sysDir places
// with a "widget" block. Those are the book's own pop-ups (a chapter's
// scene, the credits roll), written for a book page, not for a lore page.
// A missing book, or a chapter that doesn't decode, adds nothing: the worst
// case is a book widget still offered in the Customizer.
func bookWidgetSlugs(sysDir string) map[string]bool {
	if !HasBook(sysDir) {
		return nil
	}
	version := bookDirVersion(sysDir)
	bookWidgetCache.Lock()
	e, ok := bookWidgetCache.m[sysDir]
	bookWidgetCache.Unlock()
	if ok && e.version == version {
		return e.slugs
	}

	slugs := map[string]bool{}
	bookDir := filepath.Join(sysDir, bookDirName)
	var idx bookIndexYAML
	if err := decodeBookFile(filepath.Join(bookDir, bookIndexFile), &idx); err == nil {
		for _, p := range idx.Parts {
			for _, id := range p.Chapters {
				if !bookChapterID.MatchString(id) {
					continue
				}
				var cy bookChapterYAML
				if err := decodeBookFile(filepath.Join(bookDir, bookChaptersDir, id+".yaml"), &cy); err != nil {
					continue
				}
				for _, pg := range cy.Pages {
					for _, b := range pg.Blocks {
						if b.Type == "widget" {
							if slug := strings.TrimSpace(b.Widget); slug != "" {
								slugs[slug] = true
							}
						}
					}
				}
			}
		}
	}

	bookWidgetCache.Lock()
	bookWidgetCache.m[sysDir] = bookWidgetEntry{version: version, slugs: slugs}
	bookWidgetCache.Unlock()
	return slugs
}

// bookDirVersion fingerprints the files under sysDir's book/ folder by path,
// size and modification time, so a package update or an edit changes it.
func bookDirVersion(sysDir string) string {
	var sb strings.Builder
	_ = filepath.WalkDir(filepath.Join(sysDir, bookDirName), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			fmt.Fprintf(&sb, "%s|%d|%d\n", path, info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	return sb.String()
}
