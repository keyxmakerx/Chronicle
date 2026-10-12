package systems

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// The search box asks for matches on every keystroke. Answering from the book
// YAML and the data files each time re-parses both, so the entry to
// chapter/page map is built once per version of the files that produce it and
// searched from memory afterwards.

// maxBookFindIndexes bounds the cache: one slot per (system directory,
// viewer kind), and campaign systems each have their own directory.
const maxBookFindIndexes = 32

// findIndexBuilds counts index builds, so tests can prove a cache hit.
var findIndexBuilds atomic.Int64

// bookFindEntry is one searchable entry with its lower-cased text prepared
// once, so a keystroke does no per-entry string work beyond Contains.
type bookFindEntry struct {
	name, summary string
	result        BookFindResult
}

// bookFindIndex is every entry the viewer's generated chapters show, in the
// order the book lists them.
type bookFindIndex struct {
	entries []bookFindEntry
}

type bookFindCacheEntry struct {
	version string
	once    sync.Once
	idx     *bookFindIndex
	err     error
}

var bookFindCache = struct {
	sync.Mutex
	m map[string]*bookFindCacheEntry
}{m: map[string]*bookFindCacheEntry{}}

// cachedBookFindIndex returns the search index for a system's book as one
// kind of viewer. Players and Directors see different chapters, so they have
// separate indexes. Concurrent callers share one build; a changed book or
// data file starts a fresh one.
func cachedBookFindIndex(sysDir string, manifest *SystemManifest, director bool) (*bookFindIndex, error) {
	version := bookFilesVersion(sysDir)
	key := fmt.Sprintf("%s\x00%s\x00%t", sysDir, manifest.ID, director)

	bookFindCache.Lock()
	e := bookFindCache.m[key]
	if e == nil || e.version != version {
		e = &bookFindCacheEntry{version: version}
		evictOne(bookFindCache.m, key, maxBookFindIndexes)
		bookFindCache.m[key] = e
	}
	bookFindCache.Unlock()

	e.once.Do(func() {
		findIndexBuilds.Add(1)
		b, err := LoadBook(sysDir, manifest)
		if err != nil {
			e.err = err
			return
		}
		e.idx = buildBookFindIndex(sysDir, manifest, FilterBook(b, director))
	})
	if e.err != nil {
		// Not kept: a book that failed to open may be fixed by the next request.
		bookFindCache.Lock()
		if bookFindCache.m[key] == e {
			delete(bookFindCache.m, key)
		}
		bookFindCache.Unlock()
		return nil, e.err
	}
	return e.idx, nil
}

// bookFilesVersion fingerprints every file the book is built from (its YAML
// and the data files) by path, size and modification time. Statting is far
// cheaper than parsing, and any edit or package update changes it.
func bookFilesVersion(sysDir string) string {
	var sb strings.Builder
	add := func(path string, info fs.FileInfo) {
		rel, err := filepath.Rel(sysDir, path)
		if err != nil {
			rel = path
		}
		fmt.Fprintf(&sb, "%s|%d|%d\n", rel, info.Size(), info.ModTime().UnixNano())
	}
	_ = filepath.WalkDir(filepath.Join(sysDir, bookDirName), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			add(path, info)
		}
		return nil
	})
	dataDir := filepath.Join(sysDir, "data")
	if entries, err := os.ReadDir(dataDir); err == nil {
		for _, d := range entries {
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".json") {
				continue
			}
			if info, err := d.Info(); err == nil {
				add(filepath.Join(dataDir, d.Name()), info)
			}
		}
	}
	return sb.String()
}

// buildBookFindIndex maps every entry of the (already filtered) book's
// generated chapters to the chapter and page that show it. An entry listed on
// more than one page keeps the first, as the search always did.
func buildBookFindIndex(sysDir string, manifest *SystemManifest, b *Book) *bookFindIndex {
	idx := &bookFindIndex{}
	seen := map[string]bool{}
	for _, ch := range indexChapters(b) {
		for pi, p := range ch.Pages {
			cat, s, ok := pageSlice(p)
			if !ok {
				continue
			}
			ie, err := loadIndexEntries(sysDir, manifest, cat)
			if err != nil {
				continue
			}
			for _, it := range ie.slice(s) {
				k := cat + "/" + it.ID
				if seen[k] {
					continue
				}
				seen[k] = true
				summary := flattenRefMarkup(it.Summary)
				idx.entries = append(idx.entries, bookFindEntry{
					name:    strings.ToLower(it.Name),
					summary: strings.ToLower(summary),
					result: BookFindResult{Chapter: ch.ID, Page: pi, ID: it.ID, Name: it.Name,
						Summary: summary, Where: ch.Title + " · " + p.Title},
				})
			}
		}
	}
	return idx
}

// search returns entries whose name contains the query, names starting with it
// first, then entries only the summary matches. The index is read-only, so
// concurrent searches are safe.
func (idx *bookFindIndex) search(query string) []BookFindResult {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []BookFindResult{}
	}
	var named, other []BookFindResult
	for _, e := range idx.entries {
		switch {
		case strings.Contains(e.name, q):
			named = append(named, e.result)
		case strings.Contains(e.summary, q):
			other = append(other, e.result)
		}
	}
	sort.SliceStable(named, func(i, j int) bool {
		pi := strings.HasPrefix(strings.ToLower(named[i].Name), q)
		pj := strings.HasPrefix(strings.ToLower(named[j].Name), q)
		return pi && !pj
	})
	out := append(named, other...)
	if len(out) > maxBookFindResults {
		out = out[:maxBookFindResults]
	}
	if out == nil {
		out = []BookFindResult{}
	}
	return out
}

// evictOne makes room for key in a bounded cache map by dropping an arbitrary
// other entry; every entry is cheap to rebuild, so recency tracking isn't
// worth its cost. The caller holds the map's lock.
func evictOne[V any](m map[string]V, key string, limit int) {
	if _, ok := m[key]; ok || len(m) < limit {
		return
	}
	for k := range m {
		delete(m, k)
		return
	}
}
