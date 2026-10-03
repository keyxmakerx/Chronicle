package systems

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// A system's Rulebook book: plain YAML files in the package's book/ folder
// that Chronicle checks, filters per viewer and serves as JSON to the core
// rulebook widget. The author-facing format is docs/system-rulebook-book.md.
// book/ sits outside data/ on purpose: SystemDataAPI serves data/ verbatim,
// and the book carries Director-only content that must be filtered first.

const (
	bookDirName      = "book"
	bookIndexFile    = "book.yaml"
	bookChaptersDir  = "chapters"
	maxBookFileBytes = 1 << 20
	maxBookChapters  = 200
	maxBookPages     = 200
	maxBookBlocks    = 60
	maxBookListItems = 60
	maxBookString    = 20000
)

// bookThemeKeys are the theme tokens a book may set. Values become CSS
// custom properties, so each kind is checked against a strict pattern.
var bookThemeKeys = map[string]*regexp.Regexp{
	"paper":        bookColorPattern,
	"paper-alt":    bookColorPattern,
	"ink":          bookColorPattern,
	"ink-soft":     bookColorPattern,
	"muted":        bookColorPattern,
	"edge":         bookColorPattern,
	"accent":       bookColorPattern,
	"heading-font": bookFontPattern,
	"body-font":    bookFontPattern,
}

var (
	bookColorPattern   = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	bookFontPattern    = regexp.MustCompile(`^[A-Za-z0-9 ,'"-]{1,120}$`)
	bookChapterID      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	bookDicePattern    = regexp.MustCompile(`^([1-9]|10)d([2-9]|[1-9][0-9]|100)$`)
	bookWidgetSlug     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	glossaryRefPattern = regexp.MustCompile(`\{@[a-z]+ ([^}|]+)(?:\|([^}]+))?\}`)
)

// Book is the wire shape of a system's rulebook book (camelCase JSON).
type Book struct {
	Title      string              `json:"title"`
	Mark       string              `json:"mark,omitempty"`
	SystemName string              `json:"systemName"`
	Turn       string              `json:"turn"`
	Theme      map[string]string   `json:"theme,omitempty"`
	IsDirector bool                `json:"isDirector"`
	CanEdit    bool                `json:"canEdit"`
	Terms      map[string]BookTerm `json:"terms"`
	Parts      []BookPart          `json:"parts"`
}

// BookTerm is one hover definition.
type BookTerm struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// BookPart groups chapters ("Player's book", "Director's book").
type BookPart struct {
	Title    string        `json:"title"`
	Director bool          `json:"director"`
	Chapters []BookChapter `json:"chapters"`
}

// BookChapter is one chapter file.
type BookChapter struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Intro    string     `json:"intro,omitempty"`
	Director bool       `json:"director"`
	Pages    []BookPage `json:"pages"`
}

// BookPage is one page of a chapter; Wide spans the whole spread.
type BookPage struct {
	Title    string      `json:"title,omitempty"`
	Director bool        `json:"director"`
	Wide     bool        `json:"wide"`
	Blocks   []BookBlock `json:"blocks"`
}

// BookBlock is one block on a page. Only the fields of its Type are set.
type BookBlock struct {
	Type        string     `json:"type"`
	Director    bool       `json:"director"`
	Title       string     `json:"title,omitempty"`
	Text        string     `json:"text,omitempty"`
	Label       string     `json:"label,omitempty"`
	Dice        *BookDice  `json:"dice,omitempty"`
	Modifier    bool       `json:"modifier,omitempty"`
	Bands       []BookBand `json:"bands,omitempty"`
	Items       []BookItem `json:"items,omitempty"`
	Steps       []string   `json:"steps,omitempty"`
	Name        string     `json:"name,omitempty"`
	Tagline     string     `json:"tagline,omitempty"`
	Look        string     `json:"look,omitempty"`
	Notice      []string   `json:"notice,omitempty"`
	Stats       []BookStat `json:"stats,omitempty"`
	Note        string     `json:"note,omitempty"`
	HiddenStats bool       `json:"hiddenStats,omitempty"`
	Widget      string     `json:"widget,omitempty"`
}

// BookDice is a parsed dice expression such as 2d10.
type BookDice struct {
	Count int `json:"count"`
	Sides int `json:"sides"`
}

// BookBand is one result band of a roll; Max is nil on the last band.
type BookBand struct {
	Max   *int   `json:"max"`
	Label string `json:"label"`
	Text  string `json:"text,omitempty"`
}

// BookItem is a card or a flap.
type BookItem struct {
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	Text    string `json:"text,omitempty"`
}

// BookStat is one creature number.
type BookStat struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// --- YAML file shapes. Decoded with KnownFields so a misspelt key is
// reported instead of silently ignored. ---

type bookIndexYAML struct {
	Title    string            `yaml:"title,omitempty"`
	Mark     string            `yaml:"mark,omitempty"`
	Turn     string            `yaml:"turn,omitempty"`
	Glossary string            `yaml:"glossary,omitempty"`
	Theme    map[string]string `yaml:"theme,omitempty"`
	Terms    map[string]string `yaml:"terms,omitempty"`
	Parts    []bookPartYAML    `yaml:"parts"`
}

type bookPartYAML struct {
	Title    string   `yaml:"title"`
	Director bool     `yaml:"director,omitempty"`
	Chapters []string `yaml:"chapters"`
}

type bookChapterYAML struct {
	Title    string         `yaml:"title"`
	Intro    string         `yaml:"intro"`
	Director bool           `yaml:"director"`
	Pages    []bookPageYAML `yaml:"pages"`
}

// bookPageYAML is one authored page. It is also the shape a campaign's
// edited pages are stored and sent in (see authoredPage), so every page,
// package or campaign, goes through the same decoder and the same checks.
type bookPageYAML struct {
	Title    string          `yaml:"title"`
	Director bool            `yaml:"director"`
	Wide     bool            `yaml:"wide"`
	Blocks   []bookBlockYAML `yaml:"blocks"`
}

type bookBlockYAML struct {
	Type     string `yaml:"type"`
	Director bool   `yaml:"director"`
	Title    string `yaml:"title"`
	Text     string `yaml:"text"`
	Label    string `yaml:"label"`
	Dice     string `yaml:"dice"`
	Modifier bool   `yaml:"modifier"`
	Bands    []struct {
		Max   *int   `yaml:"max"`
		Label string `yaml:"label"`
		Text  string `yaml:"text"`
	} `yaml:"bands"`
	Items []struct {
		Title   string `yaml:"title"`
		Summary string `yaml:"summary"`
		Text    string `yaml:"text"`
	} `yaml:"items"`
	Steps   []string `yaml:"steps"`
	Name    string   `yaml:"name"`
	Tagline string   `yaml:"tagline"`
	Look    string   `yaml:"look"`
	Notice  []string `yaml:"notice"`
	Stats   []struct {
		Label string `yaml:"label"`
		Value string `yaml:"value"`
	} `yaml:"stats"`
	Note   string `yaml:"note"`
	Widget string `yaml:"widget"`
}

// HasBook reports whether the system in sysDir ships a book.
func HasBook(sysDir string) bool {
	if sysDir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(sysDir, bookDirName, bookIndexFile))
	return err == nil && info.Mode().IsRegular()
}

// LoadBook reads and checks the book in sysDir. An error means the cover file
// (book.yaml) itself is unusable. A broken chapter does not fail the book: it
// becomes a chapter holding one "problem" block naming the mistake, so the
// rest of the book keeps working. The manifest's widget slugs are the only
// ones a "widget" block may mount.
func LoadBook(sysDir string, manifest *SystemManifest) (*Book, error) {
	b, _, err := loadBookWithSource(sysDir, manifest)
	return b, err
}

// BookSource is the authored form of the package book that loaded: the
// cover file and, per chapter that loaded cleanly, its pages as written.
// Campaign edits are hashed against it, compared with it and exported from
// it, which the built Book cannot serve (it has lost the authored shape).
type BookSource struct {
	index    bookIndexYAML
	chapters map[string]*bookChapterYAML
}

// loadBookWithSource is LoadBook plus the authored source of every chapter
// that loaded. Chapters that failed to load have no entry.
func loadBookWithSource(sysDir string, manifest *SystemManifest) (*Book, *BookSource, error) {
	bookDir := filepath.Join(sysDir, bookDirName)
	var idx bookIndexYAML
	if err := decodeBookFile(filepath.Join(bookDir, bookIndexFile), &idx); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", bookIndexFile, err)
	}

	src := &BookSource{index: idx, chapters: map[string]*bookChapterYAML{}}
	b := &Book{
		Title: strings.TrimSpace(idx.Title),
		Mark:  strings.TrimSpace(idx.Mark),
		Turn:  idx.Turn,
		Terms: map[string]BookTerm{},
	}
	if manifest != nil {
		b.SystemName = manifest.Name
	}
	if b.Title == "" {
		b.Title = "Rulebook"
	}
	if len([]rune(b.Mark)) > 3 {
		return nil, nil, fmt.Errorf("%s: mark must be at most 3 letters", bookIndexFile)
	}
	switch b.Turn {
	case "":
		b.Turn = "flip"
	case "flip", "slide", "fade":
	default:
		return nil, nil, fmt.Errorf("%s: turn must be flip, slide or fade, not %q", bookIndexFile, b.Turn)
	}
	if len(idx.Theme) > 0 {
		b.Theme = map[string]string{}
		for k, v := range idx.Theme {
			pat, ok := bookThemeKeys[k]
			if !ok {
				return nil, nil, fmt.Errorf("%s: theme has no setting called %q", bookIndexFile, k)
			}
			v = strings.TrimSpace(v)
			if !pat.MatchString(v) {
				return nil, nil, fmt.Errorf("%s: theme %s value %q is not allowed", bookIndexFile, k, v)
			}
			b.Theme[k] = v
		}
	}

	if idx.Glossary != "" {
		if err := loadBookGlossary(sysDir, idx.Glossary, b.Terms); err != nil {
			return nil, nil, fmt.Errorf("%s: glossary: %w", bookIndexFile, err)
		}
	}
	for name, text := range idx.Terms {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		b.Terms[strings.ToLower(name)] = BookTerm{Name: name, Text: strings.TrimSpace(text)}
	}

	widgets := map[string]bool{}
	if manifest != nil {
		for _, w := range manifest.Widgets {
			widgets[w.Slug] = true
		}
	}

	total := 0
	for i, p := range idx.Parts {
		part := BookPart{Title: strings.TrimSpace(p.Title), Director: p.Director}
		if part.Title == "" {
			return nil, nil, fmt.Errorf("%s: part %d has no title", bookIndexFile, i+1)
		}
		for _, id := range p.Chapters {
			total++
			if total > maxBookChapters {
				return nil, nil, fmt.Errorf("%s: more than %d chapters", bookIndexFile, maxBookChapters)
			}
			ch, authored := loadBookChapter(bookDir, id, widgets)
			if authored != nil {
				src.chapters[id] = authored
			}
			part.Chapters = append(part.Chapters, ch)
		}
		b.Parts = append(b.Parts, part)
	}
	if len(b.Parts) == 0 {
		return nil, nil, fmt.Errorf("%s: no parts", bookIndexFile)
	}
	return b, src, nil
}

// loadBookChapter loads one chapter, turning any mistake into a problem
// chapter. The authored chapter is returned only when the chapter loaded.
func loadBookChapter(bookDir, id string, widgets map[string]bool) (BookChapter, *bookChapterYAML) {
	file := bookChaptersDir + "/" + id + ".yaml"
	if !bookChapterID.MatchString(id) {
		return problemChapter(id, fmt.Sprintf("book.yaml lists a chapter called %q; chapter names use lower-case letters, digits and -", id)), nil
	}
	var cy bookChapterYAML
	if err := decodeBookFile(filepath.Join(bookDir, bookChaptersDir, id+".yaml"), &cy); err != nil {
		return problemChapter(id, fmt.Sprintf("%s: %v", file, err)), nil
	}
	ch := BookChapter{ID: id, Title: strings.TrimSpace(cy.Title), Intro: strings.TrimSpace(cy.Intro), Director: cy.Director}
	if ch.Title == "" {
		return problemChapter(id, file+": the chapter has no title"), nil
	}
	if len(cy.Pages) == 0 {
		return problemChapter(id, file+": the chapter has no pages"), nil
	}
	if len(cy.Pages) > maxBookPages {
		return problemChapter(id, fmt.Sprintf("%s: more than %d pages", file, maxBookPages)), nil
	}
	for pi, p := range cy.Pages {
		page, perr := buildBookPage(p, widgets)
		if perr != nil {
			return problemChapter(id, perr.detail(fmt.Sprintf("%s, page %d", file, pi+1))), nil
		}
		ch.Pages = append(ch.Pages, page)
	}
	return ch, &cy
}

// bookPageError is a page that fails the book checks. Block is the 1-based
// block at fault, or 0 when the page itself is.
type bookPageError struct {
	Block int
	Msg   string
}

func (e *bookPageError) Error() string { return e.detail("page") }

// detail renders the error after a caller-chosen label such as "page 2" or
// "chapters/basics.yaml, page 2", so package files and campaign pages report
// mistakes in the same words.
func (e *bookPageError) detail(label string) string {
	if e.Block > 0 {
		return fmt.Sprintf("%s, block %d: %s", label, e.Block, e.Msg)
	}
	return fmt.Sprintf("%s: %s", label, e.Msg)
}

// buildBookPage checks one authored page and converts it to its wire shape.
// It is the single definition of a valid page: package files and campaign
// edits both pass through it.
func buildBookPage(p bookPageYAML, widgets map[string]bool) (BookPage, *bookPageError) {
	page := BookPage{Title: strings.TrimSpace(p.Title), Director: p.Director, Wide: p.Wide}
	if len(p.Blocks) == 0 {
		return page, &bookPageError{Msg: "the page has no blocks"}
	}
	if len(p.Blocks) > maxBookBlocks {
		return page, &bookPageError{Msg: fmt.Sprintf("more than %d blocks", maxBookBlocks)}
	}
	for bi, by := range p.Blocks {
		blk, err := buildBookBlock(by, widgets)
		if err != nil {
			return page, &bookPageError{Block: bi + 1, Msg: err.Error()}
		}
		page.Blocks = append(page.Blocks, blk)
	}
	return page, nil
}

// problemChapter stands in for a chapter that could not be loaded. It is for
// Directors only: FilterBook drops it for players, since a broken file could
// have been a Director-only chapter whose name must not show.
func problemChapter(id, detail string) BookChapter {
	title := "Chapter unavailable"
	if bookChapterID.MatchString(id) {
		title = id
	}
	return BookChapter{
		ID:    id,
		Title: title,
		Pages: []BookPage{{
			Title:  "This chapter couldn't be loaded",
			Blocks: []BookBlock{{Type: "problem", Text: detail}},
		}},
	}
}

// buildBookBlock checks one block and converts it to its wire shape.
func buildBookBlock(y bookBlockYAML, widgets map[string]bool) (BookBlock, error) {
	t := y.Type
	if t == "" {
		t = "text"
	}
	b := BookBlock{Type: t, Director: y.Director}
	for _, s := range []string{y.Title, y.Text, y.Label, y.Name, y.Tagline, y.Look, y.Note} {
		if len(s) > maxBookString {
			return b, fmt.Errorf("text longer than %d characters", maxBookString)
		}
	}
	need := func(field, v string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("a %s block needs %s", t, field)
		}
		return nil
	}
	switch t {
	case "text":
		b.Text = y.Text
		return b, need("text", y.Text)
	case "callout":
		b.Title, b.Text = y.Title, y.Text
		return b, need("text", y.Text)
	case "note":
		b.Director, b.Text = true, y.Text
		return b, need("text", y.Text)
	case "roll":
		m := bookDicePattern.FindStringSubmatch(strings.TrimSpace(y.Dice))
		if m == nil {
			return b, fmt.Errorf("dice must look like 2d10 (1-10 dice of 2-100 sides), not %q", y.Dice)
		}
		count, _ := strconv.Atoi(m[1])
		sides, _ := strconv.Atoi(m[2])
		b.Dice = &BookDice{Count: count, Sides: sides}
		b.Label, b.Modifier = y.Label, y.Modifier
		if len(y.Bands) == 0 {
			return b, errors.New("a roll block needs bands")
		}
		prev := -1 << 31
		for i, band := range y.Bands {
			last := i == len(y.Bands)-1
			if strings.TrimSpace(band.Label) == "" {
				return b, fmt.Errorf("band %d needs a label", i+1)
			}
			if !last && band.Max == nil {
				return b, fmt.Errorf("band %d needs max (only the last band has none)", i+1)
			}
			if last && band.Max != nil {
				return b, errors.New("the last band must not have max")
			}
			if band.Max != nil {
				if *band.Max <= prev {
					return b, errors.New("bands must go from lowest to highest")
				}
				prev = *band.Max
			}
			b.Bands = append(b.Bands, BookBand{Max: band.Max, Label: band.Label, Text: band.Text})
		}
		return b, nil
	case "cards", "flaps":
		if len(y.Items) == 0 {
			return b, fmt.Errorf("a %s block needs items", t)
		}
		if len(y.Items) > maxBookListItems {
			return b, fmt.Errorf("more than %d items", maxBookListItems)
		}
		for i, it := range y.Items {
			if strings.TrimSpace(it.Title) == "" {
				return b, fmt.Errorf("item %d needs a title", i+1)
			}
			b.Items = append(b.Items, BookItem{Title: it.Title, Summary: it.Summary, Text: it.Text})
		}
		return b, nil
	case "example":
		if len(y.Steps) == 0 {
			return b, errors.New("an example block needs steps")
		}
		if len(y.Steps) > maxBookListItems {
			return b, fmt.Errorf("more than %d steps", maxBookListItems)
		}
		b.Title, b.Steps = y.Title, y.Steps
		return b, nil
	case "creature":
		if err := need("name", y.Name); err != nil {
			return b, err
		}
		if len(y.Notice) > maxBookListItems || len(y.Stats) > maxBookListItems {
			return b, fmt.Errorf("more than %d notices or stats", maxBookListItems)
		}
		b.Name, b.Tagline, b.Look, b.Notice, b.Note = y.Name, y.Tagline, y.Look, y.Notice, y.Note
		for i, s := range y.Stats {
			if strings.TrimSpace(s.Label) == "" {
				return b, fmt.Errorf("stat %d needs a label", i+1)
			}
			b.Stats = append(b.Stats, BookStat{Label: s.Label, Value: s.Value})
		}
		b.HiddenStats = len(b.Stats) > 0
		return b, nil
	case "widget":
		slug := strings.TrimSpace(y.Widget)
		if !bookWidgetSlug.MatchString(slug) || !widgets[slug] {
			return b, fmt.Errorf("widget %q is not one of this package's widgets in manifest.json", y.Widget)
		}
		b.Widget = slug
		return b, nil
	default:
		return b, fmt.Errorf("there is no block type %q", t)
	}
}

// decodeBookFile strictly decodes one YAML file, size-capped.
func decodeBookFile(path string, out any) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("file not found")
		}
		return errors.New("file could not be read")
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxBookFileBytes+1))
	if err != nil {
		return errors.New("file could not be read")
	}
	if len(data) > maxBookFileBytes {
		return fmt.Errorf("file is larger than %d bytes", maxBookFileBytes)
	}
	return decodeBookBytes(data, out)
}

// decodeBookBytes is the one strict decoder for authored book content, used
// for package files and for pages a campaign sends (JSON is YAML). Unknown
// keys are errors so a misspelt one is reported, not silently dropped.
func decodeBookBytes(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("file is empty")
		}
		return err
	}
	return nil
}

// loadBookGlossary adds hover terms from a data/<file> list of entries with
// name and summary (or description). Reference markup like {@condition x|y}
// is flattened to its shown text: the renderer prints definitions as plain text.
func loadBookGlossary(sysDir, file string, terms map[string]BookTerm) error {
	if !systemDataFilePattern.MatchString(file) {
		return fmt.Errorf("%q is not a data file name", file)
	}
	data, err := os.ReadFile(filepath.Join(sysDir, "data", file))
	if err != nil {
		return fmt.Errorf("data/%s not found", file)
	}
	var entries []struct {
		Name        string `json:"name"`
		Summary     string `json:"summary"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("data/%s is not a list of entries", file)
	}
	for _, e := range entries {
		name := strings.TrimSpace(e.Name)
		text := e.Summary
		if text == "" {
			text = e.Description
		}
		if name == "" || text == "" {
			continue
		}
		terms[strings.ToLower(name)] = BookTerm{Name: name, Text: flattenRefMarkup(text)}
	}
	return nil
}

func flattenRefMarkup(s string) string {
	return glossaryRefPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := glossaryRefPattern.FindStringSubmatch(m)
		if sub[2] != "" {
			return sub[2]
		}
		return sub[1]
	})
}

// FilterBook returns the copy of b a viewer may receive. Directors get
// everything. Players never receive Director-only parts, chapters, pages,
// blocks, creature numbers or notes, nor broken chapters:
// this is the only place that decision is made, so the browser never holds
// what it must not show.
func FilterBook(b *Book, director bool) *Book {
	out := *b
	out.IsDirector = director
	out.Parts = nil
	for _, p := range b.Parts {
		if p.Director && !director {
			continue
		}
		np := BookPart{Title: p.Title, Director: p.Director}
		for _, ch := range p.Chapters {
			if ch.Director && !director {
				continue
			}
			nc := ch
			nc.Pages = nil
			for _, pg := range ch.Pages {
				if pg.Director && !director {
					continue
				}
				npg := pg
				npg.Blocks = nil
				for _, blk := range pg.Blocks {
					if blk.Director && !director {
						continue
					}
					if !director {
						if blk.Type == "problem" {
							continue
						}
						if blk.Type == "creature" {
							blk.Stats, blk.Note = nil, ""
						}
					}
					npg.Blocks = append(npg.Blocks, blk)
				}
				if len(npg.Blocks) > 0 {
					nc.Pages = append(nc.Pages, npg)
				}
			}
			if len(nc.Pages) > 0 {
				np.Chapters = append(np.Chapters, nc)
			}
		}
		if len(np.Chapters) > 0 {
			out.Parts = append(out.Parts, np)
		}
	}
	if out.Parts == nil {
		out.Parts = []BookPart{}
	}
	return &out
}
