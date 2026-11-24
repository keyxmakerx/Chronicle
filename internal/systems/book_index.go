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
	"sync/atomic"
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
	// Filters names the properties a reader can narrow the page by.
	Filters []string `yaml:"filters,omitempty"`
	// Sections names a property whose values head the page's entries.
	Sections string `yaml:"sections,omitempty"`
	// Tiers names the properties drawn as a power roll's result tiers.
	Tiers []string `yaml:"tiers,omitempty"`
}

const (
	maxBookIndexFilters = 6
	maxBookIndexTiers   = 4
	maxBookFacetValues  = 12
)

// BookIndexList is the reader's layout for an index page.
type BookIndexList struct {
	Filters  []BookIndexFilter `json:"filters,omitempty"`
	Sections string            `json:"sections,omitempty"`
	// SectionLabel names the sections ("Level" makes "Level 2").
	SectionLabel string   `json:"sectionLabel,omitempty"`
	Tiers        []string `json:"tiers,omitempty"`
}

// BookIndexFilter is one property a reader can narrow an index page by.
type BookIndexFilter struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// BookIndexFacet is one entry's values for one filter.
type BookIndexFacet struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
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
	items []ReferenceItem
	// closing are the lines a kind's chapter ends with: the left-out
	// entries that have a sentence, for the reader (book_reading.go).
	closing []ReferenceItem
	fields  []FieldDef
	label   string
	// lenient marks a reader's list, whose campaign entries the Director
	// wrote: an entry whose group value is a list or a set goes to the last,
	// "other" page instead of failing the chapter, which a package's own
	// file must not do.
	lenient bool
}

// maxBookIndexCacheFiles bounds the parsed-file cache: the provider already
// holds every data file for the enabled systems, so this second copy must not
// grow with each package or campaign system ever opened.
const maxBookIndexCacheFiles = 16

// indexFileParses counts real file parses, so tests can prove a cache hit.
var indexFileParses atomic.Int64

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
	// The file name comes from the manifest's own slug, never the caller's
	// string, and must still be a plain data file name.
	file := def.Slug + ".json"
	if !systemDataFilePattern.MatchString(file) || filepath.Base(file) != file {
		return nil, fmt.Errorf("%q is not a data file name", def.Slug)
	}
	path := filepath.Join(sysDir, "data", file)
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
		evictOne(bookIndexCache.m, path, maxBookIndexCacheFiles)
		bookIndexCache.m[path] = ce
		bookIndexCache.Unlock()
	}
	return &indexEntries{items: ce.items, fields: def.Fields, label: def.Name}, nil
}

func readIndexItems(path, moduleID, category string) ([]ReferenceItem, error) {
	indexFileParses.Add(1)
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
		if s.Key == "" || groupValue(it, s.Key) == s.Value {
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

// groupValue is an entry's value for a group property; missing and null are
// both "", so they share one page.
func groupValue(it ReferenceItem, key string) string {
	switch it.Properties[key].(type) {
	case []any, map[string]any:
		// Not one plain value: a package's index refuses it (indexPages), a
		// reader's lists it on the other page.
		return ""
	}
	v := strings.TrimSpace(propString(it.Properties, key))
	if v == "<nil>" {
		return ""
	}
	return v
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
	list, problem := ie.indexList(spec)
	if problem != "" {
		return ch, file + ": " + problem
	}
	pages, problem := indexPages(ch, cat, spec.Group, spec.Other, per, ie)
	if problem != "" {
		return ch, file + ": " + problem
	}
	setIndexList(pages, list)
	ch.Pages = pages
	ch.Generated = true
	return ch, ""
}

// indexList checks a chapter's filters, sections and tiers against the
// category and resolves each filter's label. Nil when the chapter names none.
func (ie *indexEntries) indexList(spec *bookIndexSpecYAML) (*BookIndexList, string) {
	if len(spec.Filters) == 0 && spec.Sections == "" && len(spec.Tiers) == 0 {
		return nil, ""
	}
	if len(spec.Filters) > maxBookIndexFilters {
		return nil, fmt.Sprintf("index filters may name at most %d properties", maxBookIndexFilters)
	}
	if len(spec.Tiers) > maxBookIndexTiers {
		return nil, fmt.Sprintf("index tiers may name at most %d properties", maxBookIndexTiers)
	}
	// A Director-only field's values would reach players as choices.
	check := func(what, key string) string {
		if !bookIndexKeyPattern.MatchString(key) {
			return fmt.Sprintf("index %s %q must be a property name (lower-case letters, digits and _)", what, key)
		}
		if ie.gmOnly(key) {
			return fmt.Sprintf("index %s %q is a Director-only field", what, key)
		}
		return ""
	}
	list := &BookIndexList{}
	seen := map[string]bool{}
	for _, k := range spec.Filters {
		if p := check("filter", k); p != "" {
			return nil, p
		}
		if seen[k] {
			return nil, fmt.Sprintf("index filter %q is named twice", k)
		}
		seen[k] = true
		list.Filters = append(list.Filters, BookIndexFilter{Key: k, Label: ie.propertyLabel(k)})
	}
	if spec.Sections != "" {
		if p := check("sections", spec.Sections); p != "" {
			return nil, p
		}
		list.Sections, list.SectionLabel = spec.Sections, ie.propertyLabel(spec.Sections)
	}
	for _, k := range spec.Tiers {
		if p := check("tier", k); p != "" {
			return nil, p
		}
		list.Tiers = append(list.Tiers, k)
	}
	return list, ""
}

// propertyLabel names a property for a reader: its manifest label, its
// display twin's, or the key in words ("subclass" reads "Subclass").
func (ie *indexEntries) propertyLabel(key string) string {
	for _, k := range []string{key, key + "_display"} {
		if l := ie.fieldLabel(k); l != k {
			return l
		}
	}
	r := []rune(strings.ReplaceAll(key, "_", " "))
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}

// gmOnly reports whether a property, or its display twin, is a manifest
// field only Directors see.
func (ie *indexEntries) gmOnly(key string) bool {
	for _, f := range ie.fields {
		if f.GMOnly && (f.Key == key || f.Key == key+"_display") {
			return true
		}
	}
	return false
}

// setIndexList gives every index block of a generated chapter its layout.
func setIndexList(pages []BookPage, list *BookIndexList) {
	if list == nil {
		return
	}
	for i := range pages {
		for j := range pages[i].Blocks {
			if pages[i].Blocks[j].Type == "index" {
				pages[i].Blocks[j].List = list
			}
		}
	}
}

// facets are an entry's values for the filters and sections of its page:
// a plain value, or each plain value of a list. Lower-case words read with a
// capital, as page titles do ("heroic" reads "Heroic").
func (ie *indexEntries) facets(it ReferenceItem, list *BookIndexList, director bool) []BookIndexFacet {
	if list == nil {
		return nil
	}
	keys := make([]string, 0, len(list.Filters)+1)
	for _, f := range list.Filters {
		keys = append(keys, f.Key)
	}
	if list.Sections != "" {
		keys = append(keys, list.Sections)
	}
	var out []BookIndexFacet
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] || (!director && ie.gmOnly(k)) {
			continue
		}
		seen[k] = true
		var vals []string
		add := func(v any) {
			switch v.(type) {
			case string, float64, int, int64, bool, json.Number:
			default:
				return
			}
			s := strings.TrimSpace(fmt.Sprint(v))
			if s == "" || len(vals) >= maxBookFacetValues {
				return
			}
			if strings.ToLower(s) == s {
				r := []rune(s)
				s = string(unicode.ToUpper(r[0])) + string(r[1:])
			}
			vals = append(vals, s)
		}
		switch v := it.Properties[k].(type) {
		case []any:
			for _, x := range v {
				add(x)
			}
		default:
			add(v)
		}
		if len(vals) > 0 {
			out = append(out, BookIndexFacet{Key: k, Values: vals})
		}
	}
	return out
}

// indexPages lays a category's entries out as a generated chapter's pages:
// one page per value of group, or pages of per entries in name order.
func indexPages(ch BookChapter, cat, group, other string, per int, ie *indexEntries) ([]BookPage, string) {
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

	// addGroup adds one value's pages: one page, or more than one when the
	// value holds more entries than a page may list.
	addGroup := func(title string, key, value string, n int) {
		if n <= maxBookIndexItems {
			add(title, bookIndexSlice{Key: key, Value: value}, n)
			return
		}
		for from, part := 0, 1; from < n; from, part = from+maxBookIndexItems, part+1 {
			s := bookIndexSlice{Key: key, Value: value, From: from}
			size := n - from
			if size > maxBookIndexItems {
				s.To, size = from+maxBookIndexItems, maxBookIndexItems
			}
			add(fmt.Sprintf("%s (%d)", title, part), s, size)
		}
	}

	if group != "" {
		counts := map[string]int{}
		for _, it := range ie.items {
			switch it.Properties[group].(type) {
			case []any, map[string]any:
				if !ie.lenient {
					return nil, fmt.Sprintf("index group %q holds lists or sets on some entries; group by a property with one plain value", group)
				}
			}
			counts[groupValue(it, group)]++
		}
		values := make([]string, 0, len(counts))
		for v := range counts {
			if v != "" {
				values = append(values, v)
			}
		}
		sort.Slice(values, func(i, j int) bool { return lessNatural(values[i], values[j]) })
		label := ie.fieldLabel(group)
		for _, v := range values {
			title := v
			switch {
			case isNumber(v):
				title = label + " " + v
			case strings.ToLower(v) == v:
				// Data values are often lower-case keys ("condition").
				r := []rune(v)
				title = string(unicode.ToUpper(r[0])) + string(r[1:])
			}
			addGroup(title, group, v, counts[v])
		}
		// Entries without the property, or with a null, share one last page.
		if n := counts[""]; n > 0 {
			other = strings.TrimSpace(other)
			if other == "" && ie.lenient && ie.label != "" {
				// The package had no such page; name it with the kind's own
				// word ("Other ancestries").
				other = "Other " + strings.ToLower(ie.label)
			}
			if other == "" {
				other = "Other"
			}
			addGroup(other, group, "", n)
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
		return nil, fmt.Sprintf("the index makes more than %d pages; raise per-page or pick another group", maxBookPages)
	}
	return pages, ""
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
	// Facets are the entry's values for its page's filters and sections.
	Facets []BookIndexFacet `json:"facets,omitempty"`
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
// carries rule markup shows the twin with hover terms instead. The page's own
// group property (groupKey) is left out: the page title already says it.
func (ie *indexEntries) indexEntry(it ReferenceItem, director bool, groupKey string) BookIndexEntry {
	e := BookIndexEntry{ID: it.ID, Name: it.Name, Summary: flattenRefMarkup(it.Summary)}
	if d := strings.TrimSpace(it.Description); d != "" && flattenRefMarkup(d) != e.Summary {
		e.Text = refMarkupToTerms(d)
	}
	for _, f := range ie.fields {
		if (f.GMOnly && !director) || (groupKey != "" && f.Key == groupKey) {
			continue
		}
		v := strings.TrimSpace(propString(it.Properties, f.Key))
		if t, ok := twinText(it.Properties, f.Key); ok {
			// A campaign entry has no generated twin; its base reads as
			// the entry page reads it, so the column is never blank.
			v = strings.TrimSpace(t)
		} else if base := strings.TrimSuffix(f.Key, "_display"); base != f.Key {
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
		e.Fields = append(e.Fields, BookStat{Label: label, Value: v, Key: f.Key})
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

// bookEntryHit is where a book lists one rules-index entry: the chapter, the
// page's place in it and title, the slice and layout of that page.
type bookEntryHit struct {
	Chapter   string
	Page      int
	PageTitle string
	Slice     bookIndexSlice
	List      *BookIndexList
	Item      ReferenceItem
}

// findBookEntry finds the first page of b whose index lists the entry, so an
// entry name in the text opens only what the reader's own book lists. load
// reads the category's entries, once, and only if some page lists it.
func findBookEntry(b *Book, cat, id string, load func() (*indexEntries, error)) (*indexEntries, *bookEntryHit, error) {
	var ie *indexEntries
	for _, ch := range indexChapters(b) {
		for pi, p := range ch.Pages {
			pc, s, ok := pageSlice(p)
			if !ok || pc != cat {
				continue
			}
			if ie == nil {
				var err error
				if ie, err = load(); err != nil {
					return nil, nil, err
				}
			}
			for _, it := range ie.slice(s) {
				if it.ID == id {
					return ie, &bookEntryHit{Chapter: ch.ID, Page: pi, PageTitle: p.Title, Slice: s, List: p.Blocks[0].List, Item: it}, nil
				}
			}
		}
	}
	return ie, nil, nil
}

// pageSlice reads the slice a generated page shows.
func pageSlice(p BookPage) (string, bookIndexSlice, bool) {
	if len(p.Blocks) != 1 || p.Blocks[0].Type != "index" {
		return "", bookIndexSlice{}, false
	}
	b := p.Blocks[0]
	return b.Category, bookIndexSlice{Key: b.Key, Value: b.Value, From: b.From, To: b.To}, true
}

// bookShowsSlice reports whether a page of b lists exactly the wanted entries, so
// the index API serves only what the reader's own book shows.
func bookShowsSlice(b *Book, category string, want bookIndexSlice) bool {
	_, ok := sliceList(b, category, want)
	return ok
}

// sliceList is the layout of the page of b that lists exactly the wanted
// entries, and whether there is such a page.
func sliceList(b *Book, category string, want bookIndexSlice) (*BookIndexList, bool) {
	for _, ch := range indexChapters(b) {
		for _, p := range ch.Pages {
			if pc, s, ok := pageSlice(p); ok && pc == category && s == want {
				return p.Blocks[0].List, true
			}
		}
	}
	return nil, false
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
