package systems

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// referenceFind is the search as it worked before the index was cached: it
// walks the book and loads the entries on every call. The cached path must
// give the same answers.
func referenceFind(sysDir string, manifest *SystemManifest, b *Book, query string) []BookFindResult {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []BookFindResult{}
	}
	var named, other []BookFindResult
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
				r := BookFindResult{Chapter: ch.ID, Page: pi, ID: it.ID, Name: it.Name,
					Summary: flattenRefMarkup(it.Summary), Where: ch.Title + " · " + p.Title}
				switch {
				case strings.Contains(strings.ToLower(it.Name), q):
					seen[k] = true
					named = append(named, r)
				case strings.Contains(strings.ToLower(flattenRefMarkup(it.Summary)), q):
					seen[k] = true
					other = append(other, r)
				}
			}
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

func TestCachedFindMatchesUncachedSearch(t *testing.T) {
	dir, m := indexTestBook(t, nil)
	b, err := LoadBook(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		director bool
		query    string
	}{
		{"empty", false, ""},
		{"blank", false, "   "},
		{"name prefix", false, "dash"},
		{"name substring and summary", false, "a"},
		{"summary only", false, "foe"},
		{"mixed case", false, "BLADE"},
		{"no match", false, "zzz"},
		{"director sees secret chapter", true, "a"},
		{"director name", true, "rally"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, err := cachedBookFindIndex(dir, m, tt.director)
			if err != nil {
				t.Fatal(err)
			}
			want := referenceFind(dir, m, FilterBook(b, tt.director), tt.query)
			if got := idx.search(tt.query); !reflect.DeepEqual(got, want) {
				t.Errorf("results differ\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestCachedFindDoesNotReparse(t *testing.T) {
	tests := []struct {
		name     string
		director bool
		lookups  int
	}{
		{"player keystrokes", false, 5},
		{"director keystrokes", true, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, m := indexTestBook(t, nil)
			builds, parses := findIndexBuilds.Load(), indexFileParses.Load()
			for i := 0; i < tt.lookups; i++ {
				idx, err := cachedBookFindIndex(dir, m, tt.director)
				if err != nil {
					t.Fatal(err)
				}
				_ = idx.search("dash"[:i%4+1])
			}
			if got := findIndexBuilds.Load() - builds; got != 1 {
				t.Errorf("index built %d times, want 1", got)
			}
			if got := indexFileParses.Load() - parses; got != 1 {
				t.Errorf("data file parsed %d times, want 1", got)
			}
		})
	}
}

func TestCachedFindInvalidatesOnChange(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		body   string
		wantIn string
	}{
		{"data file", "data/abilities.json",
			`[{"slug":"z-new","name":"Zephyr Step","summary":"Quick.","properties":{"class":"Fury","level":1}}]`, "Zephyr Step"},
		{"book chapter", "book/chapters/by-class.yaml",
			"title: Sorted by class\nindex:\n  category: abilities\n  group: class\n  other: Everyone\n", "Sorted by class"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, m := indexTestBook(t, nil)
			if _, err := cachedBookFindIndex(dir, m, false); err != nil {
				t.Fatal(err)
			}
			builds := findIndexBuilds.Load()
			path := filepath.Join(dir, filepath.FromSlash(tt.file))
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			// Coarse filesystems may keep the old mtime; size already differs
			// here, and bumping mtime keeps the test honest on either.
			later := time.Now().Add(time.Hour)
			if err := os.Chtimes(path, later, later); err != nil {
				t.Fatal(err)
			}
			idx, err := cachedBookFindIndex(dir, m, false)
			if err != nil {
				t.Fatal(err)
			}
			if got := findIndexBuilds.Load() - builds; got != 1 {
				t.Errorf("changed file rebuilt the index %d times, want 1", got)
			}
			found := false
			for _, e := range idx.entries {
				if e.result.Name == tt.wantIn || strings.Contains(e.result.Where, tt.wantIn) {
					found = true
				}
			}
			if !found {
				t.Errorf("rebuilt index does not reflect the change (%q)", tt.wantIn)
			}
		})
	}
}

func TestCachedFindConcurrentSharesOneBuild(t *testing.T) {
	dir, m := indexTestBook(t, nil)
	builds := findIndexBuilds.Load()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx, err := cachedBookFindIndex(dir, m, false)
			if err != nil {
				t.Error(err)
				return
			}
			_ = idx.search("a")
		}()
	}
	wg.Wait()
	if got := findIndexBuilds.Load() - builds; got != 1 {
		t.Errorf("concurrent callers built the index %d times, want 1", got)
	}
}

func TestParsedFileCacheIsBounded(t *testing.T) {
	tests := []struct{ name string }{{"many systems"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < maxBookIndexCacheFiles*2; i++ {
				dir, m := indexTestBook(t, nil)
				if _, err := loadIndexEntries(dir, m, "abilities"); err != nil {
					t.Fatal(err)
				}
			}
			bookIndexCache.Lock()
			n := len(bookIndexCache.m)
			bookIndexCache.Unlock()
			if n > maxBookIndexCacheFiles {
				t.Errorf("parsed-file cache holds %d files, limit %d", n, maxBookIndexCacheFiles)
			}
		})
	}
}
