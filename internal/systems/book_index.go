package systems

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Rules-index chapters: a book chapter can be generated from one of the
// system's reference data categories instead of written page by page, so the
// book holds every ability, kit or creature the package ships and stays in
// step with the data. The chapter file names the category and, optionally, a
// property to split it into one page per value ("one page per class"). Pages
// carry only the slice to show; the entries load lazily from BookIndexAPI,
// which keeps the book JSON small however large the data is.

const (
	bookIndexPerPage     = 40
	bookIndexMaxPerPage  = 100
	maxBookIndexDataSize = 8 << 20
	maxBookIndexItems    = 500
	maxBookFindResults   = 30
)

var bookIndexKeyPattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// bookIndexSpecYAML is a chapter file's "index:" section.
type bookIndexSpecYAML struct {
	Category string `yaml:"category"`
	Group    string `yaml:"group,omitempty"`
	Other    string `yaml:"other,omitempty"`
	PerPage  int    `yaml:"per-page,omitempty"`
}

// bookIndexSlice is what one generated page shows: the entries whose Key
// property equals Value (Key empty: every entry), from From up to To in name
// order (To 0: to the end).
type bookIndexSlice struct {
	Key, Value string
	From, To   int
}

// indexEntries is one category's entries in name order.
type indexEntries struct {
	items  []ReferenceItem
	fields []FieldDef
	label  string
}

// bookIndexCache holds parsed data files by path, size and modification
// time, so the book and the index API don't re-read a large file per request.
var bookIndexCache = struct {
	sync.Mutex
	m map[string]bookIndexCacheEntry
}{m: map[string]bookIndexCacheEntry{}}

type bookIndexCacheEntry struct {
	size  int64
	mtime int64
	items []ReferenceItem
}

// loadIndexEntries reads one manifest category's data file.
func loadIndexEntries(sysDir string, manifest *SystemManifest, category string) (*indexEntries, error) {
	if manifest == nil {
		return nil, fmt.Errorf("the system has no manifest")
	}
	var def *CategoryDef
	for i := range manifest.Categories {
		if manifest.Categories[i].Slug == category {
			def = &manifest.Categories[i]
			break
		}
	}
	if def == nil {
		return nil, fmt.Errorf("the manifest has no category called %q", category)
	}
	if !systemDataFilePattern.MatchString(category + ".json") {
		return nil, fmt.Errorf("%q is not a data file name", category)
	}
	path := filepath.Join(sysDir, "data", category+".json")
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("data/%s.json not found", category)
	}
	if info.Size() > maxBookIndexDataSize {
		return nil, fmt.Errorf("data/%s.json is larger than %d bytes", category, maxBookIndexDataSize)
	}

	bookIndexCache.Lock()
	ce, ok := bookIndexCache.m[path]
	bookIndexCache.Unlock()
	if !ok || ce.size != info.Size() || ce.mtime != info.ModTime().UnixNano() {
		items, err := readIndexItems(path, manifest.ID, category)
		if err != nil {
			return nil, err
		}
		ce = bookIndexCacheEntry{size: info.Size(), mtime: info.ModTime().UnixNano(), items: items}
		bookIndexCache.Lock()
		bookIndexCache.m[path] = ce
		bookIndexCache.Unlock()
	}
	return &indexEntries{items: ce.items, fields: def.Fields, label: def.Name}, nil
}

func readIndexItems(path, moduleID, category string) ([]ReferenceItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("data/%s.json not found", category)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxBookIndexDataSize+1))
	if err != nil {
		return nil, fmt.Errorf("data/%s.json could not be read", category)
	}
	var items []ReferenceItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("data/%s.json is not a list of entries", category)
	}
	items = normalizeReferenceItems(items, moduleID, category, path, nil)
	sort.SliceStable(items, func(i, j int) bool { return lessFold(items[i].Name, items[j].Name) })
	return items, nil
}

func lessFold(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	return a < b
}

// slice returns the entries one generated page shows.
func (ie *indexEntries) slice(s bookIndexSlice) []ReferenceItem {
	var out []ReferenceItem
	for _, it := range ie.items {
		if s.Key == "" || strings.TrimSpace(propString(it.Properties, s.Key)) == s.Value {
			out = append(out, it)
		}
	}
	if s.To > 0 && s.To < len(out) {
		out = out[:s.To]
	}
	if s.From > 0 {
		if s.From >= len(out) {
			return nil
		}
		out = out[s.From:]
	}
	return out
}

// fieldLabel is the manifest label of a property, or the key itself.
func (ie *indexEntries) fieldLabel(key string) string {
	for _, f := range ie.fields {
		if f.Key == key && f.Label != "" {
			return f.Label
		}
	}
	return key
}

// buildIndexChapter turns a chapter file's index section into its pages.
// Each page holds one "index" block naming the slice it shows.
func buildIndexChapter(sysDir string, manifest *SystemManifest, ch BookChapter, spec *bookIndexSpecYAML, file string) (BookChapter, string) {
	cat := strings.TrimSpace(spec.Category)
	if cat == "" {
		return ch, file + ": index needs a category"
	}
	if spec.Group != "" && !bookIndexKeyPattern.MatchString(spec.Group) {
		return ch, fmt.Sprintf("%s: index group %q must be a property name (lower-case letters, digits and _)", file, spec.Group)
	}
	if len(spec.Other) > 200 {
		return ch, file + ": index other is too long"
	}
	per := spec.PerPage
	if per == 0 {
		per = bookIndexPerPage
	}
	if per < 1 || per > bookIndexMaxPerPage {
		return ch, fmt.Sprintf("%s: index per-page must be between 1 and %d", file, bookIndexMaxPerPage)
	}
	ie, err := loadIndexEntries(sysDir, manifest, cat)
	if err != nil {
		return ch, fmt.Sprintf("%s: index: %v", file, err)
	}
	if len(ie.items) == 0 {
		return ch, fmt.Sprintf("%s: data/%s.json has no entries", file, cat)
	}

	var pages []BookPage
	add := func(title string, s bookIndexSlice, n int) {
		pages = append(pages, BookPage{
			Title:    title,
			Director: ch.Director,
			Blocks: []BookBlock{{
				Type: "index", Category: cat, Key: s.Key, Value: s.Value,
				From: s.From, To: s.To, Count: n,
			}},
		})
	}

	if spec.Group != "" {
		counts := map[string]int{}
		for _, it := range ie.items {
			counts[strings.TrimSpace(propString(it.Properties, spec.Group))]++
		}
		values := make([]string, 0, len(counts))
		for v := range counts {
			if v != "" && v != "<nil>" {
				values = append(values, v)
			}
		}
		sort.Slice(values, func(i, j int) bool { return lessNatural(values[i], values[j]) })
		label := ie.fieldLabel(spec.Group)
		for _, v := range values {
			title := v
			if isNumber(v) {
				title = label + " " + v
			}
			add(title, bookIndexSlice{Key: spec.Group, Value: v}, counts[v])
		}
		// Entries without the property, or with a null, share one last page.
		for _, empty := range []string{"", "<nil>"} {
			if n := counts[empty]; n > 0 {
				other := strings.TrimSpace(spec.Other)
				if other == "" {
					other = "Other"
				}
				add(other, bookIndexSlice{Key: spec.Group, Value: empty}, n)
			}
		}
	} else {
		total := len(ie.items)
		for from := 0; from < total; from += per {
			to := from + per
			if to > total {
				to = total
			}
			title := ch.Title
			if total > per {
				title = rangeTitle(ie.items[from].Name, ie.items[to-1].Name)
			}
			s := bookIndexSlice{From: from}
			if to < total {
				s.To = to
			}
			add(title, s, to-from)
		}
	}
	if len(pages) > maxBookPages {
		return ch, fmt.Sprintf("%s: the index makes more than %d pages; raise per-page or pick another group", file, maxBookPages)
	}
	ch.Pages = pages
	ch.Generated = true
	return ch, ""
}

// rangeTitle names a page of an ungrouped index by its first and last
// entries' initials ("A–D"), or the initial alone when they share it.
func rangeTitle(first, last string) string {
	a, b := initial(first), initial(last)
	if a == b {
		return a
	}
	return a + "–" + b
}

func initial(s string) string {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	return "#"
}

func isNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// lessNatural orders numbers numerically and text case-insensitively, numbers
// first, so levels read 1, 2, 10 rather than 1, 10, 2.
func lessNatural(a, b string) bool {
	fa, ea := strconv.ParseFloat(a, 64)
	fb, eb := strconv.ParseFloat(b, 64)
	switch {
	case ea == nil && eb == nil:
		return fa < fb
	case ea == nil:
		return true
	case eb == nil:
		return false
	}
	return lessFold(a, b)
}

// --- What the index API sends ---

// BookIndexEntry is one entry as a generated page lists it. Text and field
// values use the book's own markup, so rule words become hover terms.
type BookIndexEntry struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Summary string     `json:"summary,omitempty"`
	Text    string     `json:"text,omitempty"`
	Fields  []BookStat `json:"fields,omitempty"`
}

// refMarkupToTerms rewrites {@category term|shown} as the book's [[term|shown]].
// A term written as a slug (damage-weakness) is looked up by its words.
func refMarkupToTerms(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "[[", "["), "]]", "]")
	return glossaryRefPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := glossaryRefPattern.FindStringSubmatch(m)
		term := strings.TrimSpace(strings.ReplaceAll(sub[1], "-", " "))
		shown := strings.TrimSpace(sub[2])
		if shown == "" {
			shown = strings.TrimSpace(sub[1])
		}
		if strings.ContainsAny(term+shown, "|[]\n") {
			return shown
		}
		return "[[" + term + "|" + shown + "]]"
	})
}

// indexEntry shapes one item for a reader. Fields follow the manifest's
// columns (the category browser's), skipping empty values; a field flagged
// gm_only is left out for players. A "_display" field whose structured twin
// carries rule markup shows the twin with hover terms instead.
func (ie *indexEntries) indexEntry(it ReferenceItem, director bool) BookIndexEntry {
	e := BookIndexEntry{ID: it.ID, Name: it.Name, Summary: flattenRefMarkup(it.Summary)}
	if d := strings.TrimSpace(it.Description); d != "" && flattenRefMarkup(d) != e.Summary {
		e.Text = refMarkupToTerms(d)
	}
	for _, f := range ie.fields {
		if f.GMOnly && !director {
			continue
		}
		v := strings.TrimSpace(propString(it.Properties, f.Key))
		if base := strings.TrimSuffix(f.Key, "_display"); base != f.Key {
			if raw, ok := it.Properties[base].(string); ok && strings.Contains(raw, "{@") {
				v = refMarkupToTerms(strings.TrimSpace(raw))
			}
		}
		if v == "" || v == "<nil>" || strings.HasPrefix(v, "map[") || strings.HasPrefix(v, "[") && !strings.HasPrefix(v, "[[") {
			continue
		}
		label := f.Label
		if label == "" {
			label = f.Key
		}
		e.Fields = append(e.Fields, BookStat{Label: label, Value: v})
	}
	return e
}

// --- Finding entries ---

// indexChapters lists the generated chapters of a (filtered) book.
func indexChapters(b *Book) []BookChapter {
	var out []BookChapter
	for _, p := range b.Parts {
		for _, c := range p.Chapters {
			if c.Generated {
				out = append(out, c)
			}
		}
	}
	return out
}

// pageSlice reads the slice a generated page shows.
func pageSlice(p BookPage) (string, bookIndexSlice, bool) {
	if len(p.Blocks) != 1 || p.Blocks[0].Type != "index" {
		return "", bookIndexSlice{}, false
	}
	b := p.Blocks[0]
	return b.Category, bookIndexSlice{Key: b.Key, Value: b.Value, From: b.From, To: b.To}, true
}

// BookFindResult is one rules-index entry a search found, with the chapter
// and page that show it.
type BookFindResult struct {
	Chapter string `json:"chapter"`
	Page    int    `json:"page"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	Where   string `json:"where"`
}

// findInIndex searches the generated chapters a reader can see, names first.
func findInIndex(sysDir string, manifest *SystemManifest, b *Book, query string) []BookFindResult {
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
