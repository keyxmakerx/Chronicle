package systems

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
)

// A campaign's edition of a package's book is the package book plus the
// campaign's own pages and chapters, merged on read (book_edit_service.go).
// This file holds the shapes both sides of that merge share: the authored
// page as it is stored, hashed, sent to the editor and exported, and the
// stored rows.

// houseChapterPrefix starts every house-rules chapter id. Package chapter ids
// match bookChapterID, which has no underscore, so the two can never clash.
const houseChapterPrefix = "house_"

// housePartTitle is the one extra part, at the end of the book, that holds
// every house-rules chapter.
const housePartTitle = "House rules"

// Page states shown to Directors as the editor's status dots.
const (
	PageStatePackage = "package" // untouched package page
	PageStateEdited  = "edited"  // campaign copy of an unchanged package page
	PageStateChanged = "changed" // campaign copy, but the package page moved on or is gone
	PageStateMine    = "mine"    // a page the campaign added
)

// Limits on what a campaign can add, on top of the per-file limits every
// page already obeys.
const (
	maxOwnPagesPerChapter = 200
	maxHouseChapters      = 50
	maxHouseChapterTitle  = 200
	// maxEditionBytes bounds everything one campaign stores for one book:
	// every reader's GET /book loads and checks all of it.
	maxEditionBytes = 8 << 20
	// maxBookSystemID matches the system_id columns.
	maxBookSystemID = 64
)

// authoredPage is bookPageYAML in the shape it travels and is stored in:
// JSON keys identical to the YAML keys. The YAML structs carry no JSON tags
// (and their nested anonymous structs cannot take them), so this mirror is
// the one place the stored/wire form is defined; the YAML tags double as the
// export form, with empty fields omitted so exported files read like
// hand-written ones.
type authoredPage struct {
	Title    string `json:"title" yaml:"title,omitempty"`
	Director bool   `json:"director" yaml:"director,omitempty"`
	Wide     bool   `json:"wide" yaml:"wide,omitempty"`
	// Columns is omitempty in JSON too, so adding it left the hash of every
	// existing page unchanged.
	Columns bool            `json:"columns,omitempty" yaml:"columns,omitempty"`
	Blocks  []authoredBlock `json:"blocks" yaml:"blocks"`
}

type authoredBlock struct {
	Type     string         `json:"type,omitempty" yaml:"type,omitempty"`
	Director bool           `json:"director,omitempty" yaml:"director,omitempty"`
	Title    string         `json:"title,omitempty" yaml:"title,omitempty"`
	Text     string         `json:"text,omitempty" yaml:"text,omitempty"`
	Label    string         `json:"label,omitempty" yaml:"label,omitempty"`
	Dice     string         `json:"dice,omitempty" yaml:"dice,omitempty"`
	Modifier bool           `json:"modifier,omitempty" yaml:"modifier,omitempty"`
	Bands    []authoredBand `json:"bands,omitempty" yaml:"bands,omitempty"`
	Items    []authoredItem `json:"items,omitempty" yaml:"items,omitempty"`
	Steps    []string       `json:"steps,omitempty" yaml:"steps,omitempty"`
	Name     string         `json:"name,omitempty" yaml:"name,omitempty"`
	Tagline  string         `json:"tagline,omitempty" yaml:"tagline,omitempty"`
	Look     string         `json:"look,omitempty" yaml:"look,omitempty"`
	Notice   []string       `json:"notice,omitempty" yaml:"notice,omitempty"`
	Stats    []authoredStat `json:"stats,omitempty" yaml:"stats,omitempty"`
	Note     string         `json:"note,omitempty" yaml:"note,omitempty"`
	Widget   string         `json:"widget,omitempty" yaml:"widget,omitempty"`
}

// authoredBand keeps "max" in JSON even when null: the last band's missing
// max is meaningful, so the editor must be able to tell it from absent.
type authoredBand struct {
	Max   *int   `json:"max" yaml:"max,omitempty"`
	Label string `json:"label" yaml:"label"`
	Text  string `json:"text,omitempty" yaml:"text,omitempty"`
}

type authoredItem struct {
	Title   string `json:"title" yaml:"title"`
	Summary string `json:"summary,omitempty" yaml:"summary,omitempty"`
	Text    string `json:"text,omitempty" yaml:"text,omitempty"`
	Chapter string `json:"chapter,omitempty" yaml:"chapter,omitempty"`
}

type authoredStat struct {
	Label string `json:"label" yaml:"label"`
	Value string `json:"value" yaml:"value"`
}

// authoredChapter is a chapter file as exported.
type authoredChapter struct {
	Title    string         `yaml:"title"`
	Intro    string         `yaml:"intro,omitempty"`
	Director bool           `yaml:"director,omitempty"`
	Pages    []authoredPage `yaml:"pages"`
}

// toAuthored converts a decoded page to its stored/wire mirror.
func toAuthored(p bookPageYAML) authoredPage {
	out := authoredPage{Title: p.Title, Director: p.Director, Wide: p.Wide, Columns: p.Columns, Blocks: []authoredBlock{}}
	for _, b := range p.Blocks {
		ab := authoredBlock{
			Type: b.Type, Director: b.Director, Title: b.Title, Text: b.Text, Label: b.Label,
			Dice: b.Dice, Modifier: b.Modifier, Steps: b.Steps, Name: b.Name, Tagline: b.Tagline,
			Look: b.Look, Notice: b.Notice, Note: b.Note, Widget: b.Widget,
		}
		for _, x := range b.Bands {
			ab.Bands = append(ab.Bands, authoredBand{Max: x.Max, Label: x.Label, Text: x.Text})
		}
		for _, x := range b.Items {
			ab.Items = append(ab.Items, authoredItem{Title: x.Title, Summary: x.Summary, Text: x.Text, Chapter: x.Chapter})
		}
		for _, x := range b.Stats {
			ab.Stats = append(ab.Stats, authoredStat{Label: x.Label, Value: x.Value})
		}
		out.Blocks = append(out.Blocks, ab)
	}
	return out
}

// hashBookPage is the canonical identity of an authored page: sha256 of its
// JSON. A campaign copy records the hash of the package page it started
// from, so a package change to that page is noticed later.
func hashBookPage(p bookPageYAML) string {
	raw, _ := json.Marshal(toAuthored(p)) // plain strings and numbers; cannot fail
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// StoredBookPage is one campaign_book_pages row.
type StoredBookPage struct {
	ID           int64
	CampaignID   string
	SystemID     string
	ChapterID    string
	PackageIndex *int // nil: a page the campaign added
	SortOrder    int
	PageJSON     string
	BaseHash     string // "" when NULL
	UpdatedBy    string
}

// HouseChapter is one campaign_book_chapters row.
type HouseChapter struct {
	CampaignID string
	SystemID   string
	ChapterID  string
	Title      string
	Intro      string
	Director   bool
	SortOrder  int
	CreatedBy  string
}

// BookEdits is everything a campaign has changed for one system's book.
type BookEdits struct {
	Chapters []HouseChapter
	Pages    []StoredBookPage
}

// BookPackage is the loaded package book the merge starts from. The handler
// builds it from the system's folder so the service never touches the disk.
type BookPackage struct {
	SystemID   string
	SystemName string
	Book       *Book
	Source     *BookSource
	Widgets    map[string]bool
}

// --- editor wire shapes ---

// BookPageEntry is one page as the editor sees it.
type BookPageEntry struct {
	Key     string        `json:"key"`
	State   string        `json:"state"`
	Page    authoredPage  `json:"page"`
	Theirs  *authoredPage `json:"theirs"`
	Problem string        `json:"problem,omitempty"`
}

// BookChapterEntry is one chapter as the editor sees it. A package chapter
// that failed to load carries Problem and no pages: it cannot be edited. A
// rules-index chapter carries Generated and no pages: it is made from the
// system's data, so it changes with the package, not here.
type BookChapterEntry struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Intro     string          `json:"intro"`
	Director  bool            `json:"director"`
	House     bool            `json:"house"`
	Generated bool            `json:"generated,omitempty"`
	Problem   string          `json:"problem,omitempty"`
	Pages     []BookPageEntry `json:"pages"`
}

// BookEditorPart groups chapters like BookPart does for readers.
type BookEditorPart struct {
	Title    string             `json:"title"`
	Director bool               `json:"director"`
	House    bool               `json:"house"`
	Chapters []BookChapterEntry `json:"chapters"`
}

// BookEditorSource is the editor's whole starting state (GET /book/source).
type BookEditorSource struct {
	Title      string              `json:"title"`
	Mark       string              `json:"mark,omitempty"`
	SystemName string              `json:"systemName"`
	Theme      map[string]string   `json:"theme,omitempty"`
	Turn       string              `json:"turn"`
	Terms      map[string]BookTerm `json:"terms"`
	Widgets    []string            `json:"widgets"`
	ExportURL  string              `json:"exportUrl"`
	Parts      []BookEditorPart    `json:"parts"`
}

// pageKey renders the key the editor addresses a page by: "p<N>" for the
// package page at index N, "o<id>" for a page the campaign added.
func pageKey(s StoredBookPage) string {
	if s.PackageIndex != nil {
		return "p" + strconv.Itoa(*s.PackageIndex)
	}
	return "o" + strconv.FormatInt(s.ID, 10)
}

// parsePageKey reverses pageKey. ok is false for anything else, including
// negative or non-numeric parts.
func parsePageKey(key string) (index int, ownID int64, own bool, ok bool) {
	if len(key) < 2 {
		return 0, 0, false, false
	}
	n, err := strconv.ParseInt(key[1:], 10, 64)
	if err != nil || n < 0 || strings.TrimLeft(key[1:], "0123456789") != "" {
		return 0, 0, false, false
	}
	switch key[0] {
	case 'p':
		if n > 1<<30 {
			return 0, 0, false, false
		}
		return int(n), 0, false, true
	case 'o':
		return 0, n, true, true
	}
	return 0, 0, false, false
}
