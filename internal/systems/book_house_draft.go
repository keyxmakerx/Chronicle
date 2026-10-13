package systems

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// A house-rules chapter can also be written whole, in the package's own
// chapter-file format (intro, director, pages), instead of page by page in
// the editor. That is how the AI tool writes one. A draft passes the same
// checks as an editor save, and is written in one step, so a chapter is
// never left half replaced.

// houseDraftYAML is the part of a chapter file a draft carries; the title
// travels separately, as the name the chapter is found by. Pages is a
// pointer so "no pages: line" (keep the chapter's pages) differs from an
// empty list (a mistake).
type houseDraftYAML struct {
	Intro    *string         `yaml:"intro"`
	Director *bool           `yaml:"director"`
	Pages    *[]bookPageYAML `yaml:"pages"`
}

// HouseChapterDraft is a checked chapter, ready to preview or write.
type HouseChapterDraft struct {
	// Intro and Director are nil when the draft leaves them as they are.
	Intro    *string
	Director *bool
	pages    []bookPageYAML // nil: the chapter's pages stay as they are
	stored   []string       // each page as stored, in order
	built    []BookPage     // each page as readers get it, in order
}

// ParseHouseChapterDraft reads and checks a chapter-file draft against the
// package's rules (its widgets, the book's block checks and limits).
func ParseHouseChapterDraft(raw []byte, pkg *BookPackage) (*HouseChapterDraft, error) {
	var y houseDraftYAML
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := decodeBookBytes(raw, &y); err != nil {
			return nil, apperror.NewValidation(friendlyDecodeError(err))
		}
	}
	d := &HouseChapterDraft{Intro: y.Intro, Director: y.Director}
	if d.Intro != nil {
		in := strings.TrimSpace(*d.Intro)
		if len(in) > maxBookString {
			return nil, apperror.NewValidation(fmt.Sprintf("The introduction is longer than %d characters.", maxBookString))
		}
		d.Intro = &in
	}
	if y.Pages == nil {
		return d, nil
	}
	pages := *y.Pages
	if len(pages) == 0 {
		return nil, apperror.NewValidation("A chapter needs at least one page.")
	}
	if len(pages) > maxOwnPagesPerChapter {
		return nil, apperror.NewValidation(fmt.Sprintf("A chapter can hold at most %d pages of your own.", maxOwnPagesPerChapter))
	}
	var widgets map[string]bool
	if pkg != nil {
		widgets = pkg.Widgets
	}
	for i, p := range pages {
		page, perr := buildStoredPage(p, widgets)
		if perr != nil {
			return nil, apperror.NewValidation(perr.detail(fmt.Sprintf("page %d", i+1)))
		}
		raw, err := json.Marshal(toAuthored(p))
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		d.stored = append(d.stored, string(raw))
		d.built = append(d.built, page)
	}
	d.pages = pages
	return d, nil
}

// HasPages reports whether the draft replaces the chapter's pages.
func (d *HouseChapterDraft) HasPages() bool { return d != nil && d.pages != nil }

// PageTitles lists the draft's page titles, in order.
func (d *HouseChapterDraft) PageTitles() []string {
	out := make([]string, 0, len(d.pages))
	for _, p := range d.pages {
		out = append(out, strings.TrimSpace(p.Title))
	}
	return out
}

// draftBlockNames are the block kinds as the editor's palette names them,
// singular and plural. Plain text is left out of a description.
var draftBlockNames = map[string][2]string{
	"callout":  {"highlighted box", "highlighted boxes"},
	"roll":     {"dice roll", "dice rolls"},
	"cards":    {"row of fold-out cards", "rows of fold-out cards"},
	"flaps":    {"set of flaps", "sets of flaps"},
	"example":  {"worked example", "worked examples"},
	"creature": {"creature", "creatures"},
	"note":     {"Director's note", "Director's notes"},
	"links":    {"set of chapter links", "sets of chapter links"},
	"contents": {"contents list", "contents lists"},
	"widget":   {"widget", "widgets"},
}

// Blocks describes the draft's blocks in plain words, in the order they
// first appear: "1 highlighted box, 2 dice rolls". "" when only text.
func (d *HouseChapterDraft) Blocks() string {
	counts := map[string]int{}
	var order []string
	for _, p := range d.pages {
		for _, b := range p.Blocks {
			if _, ok := draftBlockNames[b.Type]; !ok {
				continue
			}
			if counts[b.Type] == 0 {
				order = append(order, b.Type)
			}
			counts[b.Type]++
		}
	}
	parts := make([]string, 0, len(order))
	for _, t := range order {
		n, names := counts[t], draftBlockNames[t]
		name := names[0]
		if n > 1 {
			name = names[1]
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, name))
	}
	return strings.Join(parts, ", ")
}

// DirectorPages reports, per page in order, whether it is Directors-only.
func (d *HouseChapterDraft) DirectorPages() []bool {
	out := make([]bool, 0, len(d.pages))
	for _, p := range d.pages {
		out = append(out, p.Director)
	}
	return out
}

// HiddenParts counts the Director-only pages and blocks in the draft.
func (d *HouseChapterDraft) HiddenParts() int {
	return countHidden(d.pages)
}

func countHidden(pages []bookPageYAML) int {
	n := 0
	for _, p := range pages {
		if p.Director {
			n++
			continue
		}
		for _, b := range p.Blocks {
			if b.Director || b.Type == "note" {
				n++
			}
		}
	}
	return n
}

// PlayerChapters is the set of chapter ids a player can open in the edition.
func PlayerChapters(src *BookEditorSource) map[string]bool {
	out := map[string]bool{}
	for _, part := range src.Parts {
		if part.Director {
			continue
		}
		for _, ch := range part.Chapters {
			if !ch.Director {
				out[ch.ID] = true
			}
		}
	}
	return out
}

// HiddenPartsOf counts the Director-only pages and blocks of a chapter as
// the editor holds it, so a replacement that drops them can say so.
func HiddenPartsOf(ch BookChapterEntry) int {
	n := 0
	for _, pe := range ch.Pages {
		if pe.Page.Director {
			n++
			continue
		}
		for _, b := range pe.Page.Blocks {
			if b.Director || b.Type == "note" {
				n++
			}
		}
	}
	return n
}

// previewChapterID names the chapter in a draft preview. It is never stored.
const previewChapterID = houseChapterPrefix + "preview"

// HouseChapterPreview is a one-chapter Book of the draft, in the package's
// own theme, for a Director to read before it is written: the same shape
// GET /book returns, so the reader draws it with its usual look and motion.
// Nil when the draft carries no pages.
func HouseChapterPreview(pkg *BookPackage, title, intro string, director bool, d *HouseChapterDraft) *Book {
	if !d.HasPages() || pkg == nil || pkg.Book == nil {
		return nil
	}
	if d.Intro != nil {
		intro = *d.Intro
	}
	if d.Director != nil {
		director = *d.Director
	}
	b := *pkg.Book
	b.Tabs = nil
	b.Start, b.First = previewChapterID, previewChapterID
	b.CanEdit = false
	b.Parts = []BookPart{{Title: housePartTitle, Chapters: []BookChapter{{
		ID: previewChapterID, Title: strings.TrimSpace(title), Intro: intro, Director: director,
		Pages: append([]BookPage(nil), d.built...),
	}}}}
	return FilterBook(&b, true)
}

// houseChapterFile is a house chapter as the AI tool reads it back.
type houseChapterFile struct {
	Intro    string         `yaml:"intro,omitempty"`
	Director bool           `yaml:"director,omitempty"`
	Pages    []authoredPage `yaml:"pages"`
}

// HouseChapterYAML writes a house chapter in the draft format, so an AI can
// change it and send it back whole. With director false it is the player's
// view: Director-only pages and blocks are left out (and counted in hidden),
// a creature keeps only what players see, and a link keeps only chapters in
// playerChapters (the ids players can open), as the reader trims them.
func HouseChapterYAML(ch BookChapterEntry, director bool, playerChapters map[string]bool) (out []byte, hidden int, err error) {
	f := houseChapterFile{Intro: ch.Intro, Director: ch.Director, Pages: []authoredPage{}}
	for _, pe := range ch.Pages {
		pg := pe.Page
		if pg.Director && !director {
			hidden++
			continue
		}
		var blocks []authoredBlock
		for _, b := range nonEmptyBlocks(pg.Blocks) {
			if !director {
				if b.Director || b.Type == "note" {
					hidden++
					continue
				}
				if b.Type == "creature" {
					b.Stats, b.Note = nil, ""
				}
				if b.Type == "links" {
					var items []authoredItem
					for _, it := range b.Items {
						if playerChapters[it.Chapter] {
							items = append(items, it)
						}
					}
					if len(items) == 0 {
						continue
					}
					b.Items = items
				}
			}
			blocks = append(blocks, b)
		}
		if len(blocks) == 0 {
			continue
		}
		pg.Blocks = blocks
		f.Pages = append(f.Pages, pg)
	}
	out, err = marshalYAML(f)
	return out, hidden, err
}

// WriteHouseChapter writes a whole house-rules chapter in one step. With
// chapterID "" it makes a new chapter called title; otherwise it changes that
// chapter, renaming it when title is set, and the draft's pages (if it has
// any) replace the chapter's pages.
func (s *bookEditService) WriteHouseChapter(ctx context.Context, campaignID, userID string, pkg *BookPackage, chapterID, title string, d *HouseChapterDraft) (*BookChapterEntry, error) {
	if d == nil {
		d = &HouseChapterDraft{}
	}
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	create := chapterID == ""
	var hc HouseChapter
	freed := 0
	if create {
		if err := checkChapterTitle(title); err != nil {
			return nil, err
		}
		if err := ed.checkNewChapter(pkg); err != nil {
			return nil, err
		}
		if !d.HasPages() {
			return nil, apperror.NewValidation("A new chapter needs at least one page.")
		}
		hc = HouseChapter{CampaignID: campaignID, SystemID: pkg.SystemID, ChapterID: houseChapterPrefix + s.newID(), Title: title, CreatedBy: userID}
	} else {
		ch := ed.byID[chapterID]
		if ch == nil {
			return nil, apperror.NewNotFound("chapter not found")
		}
		if ch.house == nil {
			return nil, apperror.NewValidation("Only house-rules chapters can be changed here.")
		}
		hc = *ch.house
		if title != "" {
			if err := checkChapterTitle(title); err != nil {
				return nil, err
			}
			hc.Title = title
		}
		if d.HasPages() {
			for _, o := range ch.owns {
				freed += len(o.PageJSON)
			}
		}
	}
	if d.Intro != nil {
		hc.Intro = *d.Intro
	}
	if d.Director != nil {
		hc.Director = *d.Director
	}

	var pages []StoredBookPage
	if d.HasPages() {
		total := ed.storedBytes - freed
		for _, raw := range d.stored {
			total += len(raw)
			pages = append(pages, StoredBookPage{
				CampaignID: campaignID, SystemID: pkg.SystemID, ChapterID: hc.ChapterID, PageJSON: raw, UpdatedBy: userID,
			})
		}
		if total > maxEditionBytes {
			return nil, apperror.NewValidation(fmt.Sprintf("This campaign's rulebook changes have reached the %d MB limit. Remove or shorten some pages first.", maxEditionBytes>>20))
		}
	}
	if err := s.repo.WriteHouseChapter(ctx, hc, create, pages); err != nil {
		return nil, err
	}
	return s.chapterEntry(ctx, campaignID, pkg, hc.ChapterID)
}
