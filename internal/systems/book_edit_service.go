package systems

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// BookEditService owns everything about a campaign's edits to a system's
// book: merging them over the package book, validating what a Director
// sends, and the limits. It never sees a request, so who may edit is the
// handler's decision, made before any method here is reached.
type BookEditService interface {
	// Edition is the book a campaign reads: the package book with its edits
	// merged in, before the per-viewer filter.
	Edition(ctx context.Context, campaignID string, pkg *BookPackage) (*Book, error)
	EditorSource(ctx context.Context, campaignID string, pkg *BookPackage) (*BookEditorSource, error)

	SavePage(ctx context.Context, campaignID, userID string, pkg *BookPackage, chapterID, key string, rawPage []byte) (*BookPageEntry, error)
	AddPage(ctx context.Context, campaignID, userID string, pkg *BookPackage, chapterID string, rawPage []byte) (*BookPageEntry, error)
	// DeletePage drops a copy (the package page returns) or deletes an own
	// page. The entry is nil when no page is left at that key.
	DeletePage(ctx context.Context, campaignID string, pkg *BookPackage, chapterID, key string) (*BookPageEntry, error)
	KeepPage(ctx context.Context, campaignID, userID string, pkg *BookPackage, chapterID, key string) (*BookPageEntry, error)

	CreateChapter(ctx context.Context, campaignID, userID string, pkg *BookPackage, title string) (*BookChapterEntry, error)
	UpdateChapter(ctx context.Context, campaignID string, pkg *BookPackage, chapterID string, in UpdateBookChapterInput) (*BookChapterEntry, error)
	DeleteChapter(ctx context.Context, campaignID string, pkg *BookPackage, chapterID string) error

	// Export returns the campaign's edition as a zip of package-format files.
	Export(ctx context.Context, campaignID string, pkg *BookPackage) ([]byte, error)
}

// UpdateBookChapterInput is a PARTIAL update of a house-rules chapter: an
// absent key keeps the stored value. An explicit null clears the intro; title
// and director cannot be empty, so null keeps them.
type UpdateBookChapterInput struct {
	Title    patch.Field[string] `json:"title"`
	Intro    patch.Field[string] `json:"intro"`
	Director patch.Field[bool]   `json:"director"`
}

type bookEditService struct {
	repo  BookEditRepository
	newID func() string
}

// NewBookEditService creates the service over a repository.
func NewBookEditService(repo BookEditRepository) BookEditService {
	return &bookEditService{repo: repo, newID: randomChapterSuffix}
}

func randomChapterSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:]) // crypto/rand does not fail on supported platforms
	return hex.EncodeToString(b[:])
}

// LoadBookPackage reads the package book with its authored source and the
// widget slugs its pages may mount.
func LoadBookPackage(sysDir string, manifest *SystemManifest) (*BookPackage, error) {
	book, src, err := loadBookWithSource(sysDir, manifest)
	if err != nil {
		return nil, err
	}
	pkg := &BookPackage{Book: book, Source: src, Widgets: map[string]bool{}}
	if manifest != nil {
		pkg.SystemID, pkg.SystemName = manifest.ID, manifest.Name
		for _, w := range manifest.Widgets {
			pkg.Widgets[w.Slug] = true
		}
	}
	return pkg, nil
}

// --- the merge ---

// editionChapter is one chapter of a campaign's edition: what readers get
// (built), what the editor gets (entry) and what the mutations need to know.
type editionChapter struct {
	entry    BookChapterEntry
	built    BookChapter
	editable bool
	house    *HouseChapter
	nPackage int
	copies   map[int]*StoredBookPage
	owns     []*StoredBookPage
	pkgPages []bookPageYAML
}

type editionPart struct {
	title    string
	director bool
	house    bool
	chapters []*editionChapter
}

type edition struct {
	parts []editionPart
	byID  map[string]*editionChapter
	// storedBytes is the size of every stored page, for maxEditionBytes.
	storedBytes int
}

// checkRoom refuses a write that would take the campaign's stored pages past
// maxEditionBytes. replaced is the page being overwritten, if any, so
// re-saving a page near the limit still works.
func (ed *edition) checkRoom(stored string, replaced *StoredBookPage) error {
	total := ed.storedBytes + len(stored)
	if replaced != nil {
		total -= len(replaced.PageJSON)
	}
	if total > maxEditionBytes {
		return apperror.NewValidation(fmt.Sprintf("This campaign's rulebook changes have reached the %d MB limit. Remove or shorten some pages first.", maxEditionBytes>>20))
	}
	return nil
}

// positionOf returns the index of the page with key in the chapter, or -1.
func (c *editionChapter) positionOf(key string) int {
	for i, p := range c.entry.Pages {
		if p.Key == key {
			return i
		}
	}
	return -1
}

// buildEdition merges a campaign's edits over the package book.
//
// Rules: a copy replaces the package page at its index; own pages follow the
// package pages; a copy whose index is past the package's last page (the
// package shrank) is kept after the package pages, in index order, as
// "changed"; house chapters form a last part; edits for a chapter the
// package no longer lists, or one that failed to load, are ignored.
func buildEdition(pkg *BookPackage, edits *BookEdits) *edition {
	ed := &edition{byID: map[string]*editionChapter{}}
	if edits == nil {
		edits = &BookEdits{}
	}
	for _, p := range edits.Pages {
		ed.storedBytes += len(p.PageJSON)
	}

	copies := map[string]map[int]*StoredBookPage{}
	owns := map[string][]*StoredBookPage{}
	for i := range edits.Pages {
		p := &edits.Pages[i]
		if p.PackageIndex != nil {
			if copies[p.ChapterID] == nil {
				copies[p.ChapterID] = map[int]*StoredBookPage{}
			}
			copies[p.ChapterID][*p.PackageIndex] = p
		} else {
			owns[p.ChapterID] = append(owns[p.ChapterID], p)
		}
	}
	for _, list := range owns {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].SortOrder != list[j].SortOrder {
				return list[i].SortOrder < list[j].SortOrder
			}
			return list[i].ID < list[j].ID
		})
	}

	for _, part := range pkg.Book.Parts {
		ep := editionPart{title: part.Title, director: part.Director}
		for _, ch := range part.Chapters {
			ec := mergePackageChapter(pkg, ch, copies[ch.ID], owns[ch.ID])
			ep.chapters = append(ep.chapters, ec)
			ed.byID[ch.ID] = ec
		}
		ed.parts = append(ed.parts, ep)
	}

	houses := append([]HouseChapter(nil), edits.Chapters...)
	sort.SliceStable(houses, func(i, j int) bool {
		if houses[i].SortOrder != houses[j].SortOrder {
			return houses[i].SortOrder < houses[j].SortOrder
		}
		return houses[i].ChapterID < houses[j].ChapterID
	})
	if len(houses) > 0 {
		hp := editionPart{title: housePartTitle, house: true}
		for i := range houses {
			h := &houses[i]
			ec := mergeHouseChapter(pkg, h, owns[h.ChapterID])
			hp.chapters = append(hp.chapters, ec)
			ed.byID[h.ChapterID] = ec
		}
		ed.parts = append(ed.parts, hp)
	}
	return ed
}

func mergePackageChapter(pkg *BookPackage, ch BookChapter, copies map[int]*StoredBookPage, owns []*StoredBookPage) *editionChapter {
	ec := &editionChapter{
		entry: BookChapterEntry{ID: ch.ID, Title: ch.Title, Intro: ch.Intro, Director: ch.Director, Pages: []BookPageEntry{}},
		built: BookChapter{ID: ch.ID, Title: ch.Title, Intro: ch.Intro, Director: ch.Director},
	}
	authored := pkg.Source.chapters[ch.ID]
	if ch.Generated {
		// Made from the system's data: shown as is, never edited.
		ec.built = ch
		ec.entry.Generated = true
		return ec
	}
	if authored == nil {
		// Failed to load: kept as the package's problem chapter, not editable.
		ec.built = ch
		if len(ch.Pages) > 0 && len(ch.Pages[0].Blocks) > 0 {
			ec.entry.Problem = ch.Pages[0].Blocks[0].Text
		}
		return ec
	}
	ec.editable = true
	ec.pkgPages = authored.Pages
	ec.nPackage = len(authored.Pages)
	ec.copies = copies

	for i, ap := range authored.Pages {
		theirs := toAuthored(ap)
		cp := copies[i]
		if cp == nil {
			ec.entry.Pages = append(ec.entry.Pages, BookPageEntry{Key: fmt.Sprintf("p%d", i), State: PageStatePackage, Page: theirs})
			ec.built.Pages = append(ec.built.Pages, ch.Pages[i])
			continue
		}
		state := PageStateEdited
		if cp.BaseHash != hashBookPage(ap) {
			state = PageStateChanged
		}
		ec.addStored(pkg, cp, state, &theirs)
	}

	var orphanIdx []int
	for i := range copies {
		if i >= ec.nPackage {
			orphanIdx = append(orphanIdx, i)
		}
	}
	sort.Ints(orphanIdx)
	for _, i := range orphanIdx {
		ec.addStored(pkg, copies[i], PageStateChanged, nil)
	}
	for _, o := range owns {
		ec.owns = append(ec.owns, o)
		ec.addStored(pkg, o, PageStateMine, nil)
	}
	return ec
}

func mergeHouseChapter(pkg *BookPackage, h *HouseChapter, owns []*StoredBookPage) *editionChapter {
	title, intro := strings.TrimSpace(h.Title), strings.TrimSpace(h.Intro)
	ec := &editionChapter{
		entry:    BookChapterEntry{ID: h.ChapterID, Title: title, Intro: intro, Director: h.Director, House: true, Pages: []BookPageEntry{}},
		built:    BookChapter{ID: h.ChapterID, Title: title, Intro: intro, Director: h.Director},
		editable: true,
		house:    h,
		owns:     owns,
	}
	for _, o := range owns {
		ec.addStored(pkg, o, PageStateMine, nil)
	}
	return ec
}

// addStored appends a stored page, checking it again on every read: the
// rules (or the package's widget list) can change after it was saved. A page
// that no longer passes shows Directors why, as a problem block.
func (c *editionChapter) addStored(pkg *BookPackage, s *StoredBookPage, state string, theirs *authoredPage) {
	entry := BookPageEntry{Key: pageKey(*s), State: state, Theirs: theirs, Page: authoredPage{Blocks: []authoredBlock{}}}
	position := len(c.entry.Pages) + 1

	var decoded bookPageYAML
	if err := decodeBookBytes([]byte(s.PageJSON), &decoded); err != nil {
		entry.Problem = fmt.Sprintf("page %d: %s", position, friendlyDecodeError(err))
		c.entry.Pages = append(c.entry.Pages, entry)
		c.built.Pages = append(c.built.Pages, problemPage(entry.Problem))
		return
	}
	entry.Page = toAuthored(decoded)
	page, perr := buildStoredPage(decoded, pkg.Widgets)
	if perr != nil {
		entry.Problem = perr.detail(fmt.Sprintf("page %d", position))
		c.entry.Pages = append(c.entry.Pages, entry)
		c.built.Pages = append(c.built.Pages, problemPage(entry.Problem))
		return
	}
	c.entry.Pages = append(c.entry.Pages, entry)
	c.built.Pages = append(c.built.Pages, page)
}

func problemPage(detail string) BookPage {
	return BookPage{
		Title:  "This page couldn't be loaded",
		Blocks: []BookBlock{{Type: "problem", Text: detail}},
	}
}

// buildStoredPage is buildBookPage for a campaign's page. A text block with
// no text yet is a Director's unfinished draft, not a mistake: it is kept in
// the stored page (so the editor still shows the empty box) and simply shows
// nothing, while every other block must pass the book checks. Block numbers
// in errors still count the empty blocks, so they match what the editor shows.
func buildStoredPage(p bookPageYAML, widgets map[string]bool) (BookPage, *bookPageError) {
	page := BookPage{Title: strings.TrimSpace(p.Title), Director: p.Director, Wide: p.Wide, Blocks: []BookBlock{}}
	if len(p.Blocks) == 0 {
		return page, &bookPageError{Msg: "the page has no blocks"}
	}
	if len(p.Blocks) > maxBookBlocks {
		return page, &bookPageError{Msg: fmt.Sprintf("more than %d blocks", maxBookBlocks)}
	}
	for bi, by := range p.Blocks {
		if isEmptyTextBlock(by) {
			continue
		}
		blk, err := buildBookBlock(by, widgets)
		if err != nil {
			return page, &bookPageError{Block: bi + 1, Msg: err.Error()}
		}
		page.Blocks = append(page.Blocks, blk)
	}
	return page, nil
}

// isEmptyTextBlock reports a text block nobody has written in yet.
func isEmptyTextBlock(b bookBlockYAML) bool {
	return (b.Type == "" || b.Type == "text") && strings.TrimSpace(b.Text) == ""
}

// book renders the merged edition as the reader's Book (before FilterBook,
// so a Director flag on an edited page is honoured there).
func (ed *edition) book(pkg *BookPackage) *Book {
	out := *pkg.Book
	out.Parts = make([]BookPart, 0, len(ed.parts))
	for _, p := range ed.parts {
		bp := BookPart{Title: p.title, Director: p.director, Chapters: make([]BookChapter, 0, len(p.chapters))}
		for _, c := range p.chapters {
			bp.Chapters = append(bp.Chapters, c.built)
		}
		out.Parts = append(out.Parts, bp)
	}
	return &out
}

// ApplyBookEdits merges a campaign's edits over the package book. It does
// not filter: callers run FilterBook on the result.
func ApplyBookEdits(pkg *BookPackage, edits *BookEdits) *Book {
	return buildEdition(pkg, edits).book(pkg)
}

func (ed *edition) editorSource(pkg *BookPackage) *BookEditorSource {
	src := &BookEditorSource{
		Title: pkg.Book.Title, Mark: pkg.Book.Mark, SystemName: pkg.Book.SystemName,
		Theme: pkg.Book.Theme, Turn: pkg.Book.Turn, Terms: pkg.Book.Terms,
		Widgets: []string{}, Parts: []BookEditorPart{},
	}
	for slug := range pkg.Widgets {
		src.Widgets = append(src.Widgets, slug)
	}
	sort.Strings(src.Widgets)
	for _, p := range ed.parts {
		ep := BookEditorPart{Title: p.title, Director: p.director, House: p.house, Chapters: []BookChapterEntry{}}
		for _, c := range p.chapters {
			ep.Chapters = append(ep.Chapters, c.entry)
		}
		src.Parts = append(src.Parts, ep)
	}
	return src
}

// --- reads ---

func (s *bookEditService) load(ctx context.Context, campaignID string, pkg *BookPackage) (*edition, error) {
	edits, err := s.repo.Load(ctx, campaignID, pkg.SystemID)
	if err != nil {
		return nil, err
	}
	return buildEdition(pkg, edits), nil
}

func (s *bookEditService) Edition(ctx context.Context, campaignID string, pkg *BookPackage) (*Book, error) {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	return ed.book(pkg), nil
}

func (s *bookEditService) EditorSource(ctx context.Context, campaignID string, pkg *BookPackage) (*BookEditorSource, error) {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	return ed.editorSource(pkg), nil
}

// --- page mutations ---

// editableChapter finds a chapter the Director may put pages in.
func (ed *edition) editableChapter(chapterID string) (*editionChapter, error) {
	c := ed.byID[chapterID]
	if c == nil {
		return nil, apperror.NewNotFound("chapter not found")
	}
	if !c.editable {
		return nil, apperror.NewValidation("This chapter couldn't be loaded, so it can't be edited here.")
	}
	return c, nil
}

func (s *bookEditService) SavePage(ctx context.Context, campaignID, userID string, pkg *BookPackage, chapterID, key string, rawPage []byte) (*BookPageEntry, error) {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	ch, err := ed.editableChapter(chapterID)
	if err != nil {
		return nil, err
	}
	index, ownID, own, ok := parsePageKey(key)
	if !ok {
		return nil, apperror.NewBadRequest("bad page key")
	}
	pos := ch.positionOf(key)
	if pos < 0 {
		return nil, apperror.NewNotFound("page not found")
	}
	_, stored, err := checkRequestPage(rawPage, pkg.Widgets, pos+1)
	if err != nil {
		return nil, err
	}

	if own {
		var replaced *StoredBookPage
		for _, o := range ch.owns {
			if o.ID == ownID {
				replaced = o
			}
		}
		if err := ed.checkRoom(stored, replaced); err != nil {
			return nil, err
		}
		if err := s.repo.UpdateOwn(ctx, campaignID, pkg.SystemID, chapterID, ownID, stored, userID); err != nil {
			return nil, err
		}
		return s.entry(ctx, campaignID, pkg, chapterID, key)
	}
	// A new copy remembers the package page it started from; saving again
	// keeps that memory, so a page the package has since changed stays
	// flagged until the Director answers with "Keep mine".
	sp := StoredBookPage{
		CampaignID: campaignID, SystemID: pkg.SystemID, ChapterID: chapterID, PackageIndex: &index,
		PageJSON: stored, UpdatedBy: userID,
	}
	if err := ed.checkRoom(stored, ch.copies[index]); err != nil {
		return nil, err
	}
	if existing := ch.copies[index]; existing != nil {
		sp.BaseHash = existing.BaseHash
	} else if index < ch.nPackage {
		sp.BaseHash = hashBookPage(ch.pkgPages[index])
	}
	if err := s.repo.SaveCopy(ctx, sp); err != nil {
		return nil, err
	}
	return s.entry(ctx, campaignID, pkg, chapterID, key)
}

func (s *bookEditService) AddPage(ctx context.Context, campaignID, userID string, pkg *BookPackage, chapterID string, rawPage []byte) (*BookPageEntry, error) {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	ch, err := ed.editableChapter(chapterID)
	if err != nil {
		return nil, err
	}
	if len(ch.owns) >= maxOwnPagesPerChapter {
		return nil, apperror.NewValidation(fmt.Sprintf("A chapter can hold at most %d pages of your own.", maxOwnPagesPerChapter))
	}
	_, stored, err := checkRequestPage(rawPage, pkg.Widgets, len(ch.entry.Pages)+1)
	if err != nil {
		return nil, err
	}
	if err := ed.checkRoom(stored, nil); err != nil {
		return nil, err
	}
	id, err := s.repo.AddOwn(ctx, StoredBookPage{
		CampaignID: campaignID, SystemID: pkg.SystemID, ChapterID: chapterID, PageJSON: stored, UpdatedBy: userID,
	})
	if err != nil {
		return nil, err
	}
	return s.entry(ctx, campaignID, pkg, chapterID, fmt.Sprintf("o%d", id))
}

func (s *bookEditService) DeletePage(ctx context.Context, campaignID string, pkg *BookPackage, chapterID, key string) (*BookPageEntry, error) {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	ch, err := ed.editableChapter(chapterID)
	if err != nil {
		return nil, err
	}
	index, ownID, own, ok := parsePageKey(key)
	if !ok {
		return nil, apperror.NewBadRequest("bad page key")
	}
	if ch.positionOf(key) < 0 {
		return nil, apperror.NewNotFound("page not found")
	}
	if own {
		if ch.house != nil && len(ch.owns) <= 1 {
			return nil, apperror.NewValidation("A chapter needs at least one page. Remove the chapter instead.")
		}
		if _, err := s.repo.DeleteOwn(ctx, campaignID, pkg.SystemID, chapterID, ownID); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if _, err := s.repo.DeleteCopy(ctx, campaignID, pkg.SystemID, chapterID, index); err != nil {
		return nil, err
	}
	if index >= ch.nPackage {
		return nil, nil
	}
	return s.entry(ctx, campaignID, pkg, chapterID, key)
}

func (s *bookEditService) KeepPage(ctx context.Context, campaignID, userID string, pkg *BookPackage, chapterID, key string) (*BookPageEntry, error) {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	ch, err := ed.editableChapter(chapterID)
	if err != nil {
		return nil, err
	}
	index, _, own, ok := parsePageKey(key)
	if !ok {
		return nil, apperror.NewBadRequest("bad page key")
	}
	if ch.positionOf(key) < 0 {
		return nil, apperror.NewNotFound("page not found")
	}
	cp := ch.copies[index]
	if own || cp == nil {
		return nil, apperror.NewValidation("There is nothing to keep: this page has no changes of yours to settle.")
	}
	if index >= ch.nPackage {
		// The package no longer has a page here, so there is nothing left to
		// compare against: the copy becomes the campaign's own page.
		id, err := s.repo.PromoteCopy(ctx, campaignID, pkg.SystemID, chapterID, index)
		if err != nil {
			return nil, err
		}
		return s.entry(ctx, campaignID, pkg, chapterID, fmt.Sprintf("o%d", id))
	}
	sp := *cp
	sp.BaseHash = hashBookPage(ch.pkgPages[index])
	sp.UpdatedBy = userID
	if err := s.repo.SaveCopy(ctx, sp); err != nil {
		return nil, err
	}
	return s.entry(ctx, campaignID, pkg, chapterID, key)
}

// entry re-reads the edition and returns one page, so the response is
// exactly what the next GET /book/source would show.
func (s *bookEditService) entry(ctx context.Context, campaignID string, pkg *BookPackage, chapterID, key string) (*BookPageEntry, error) {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	if ch := ed.byID[chapterID]; ch != nil {
		if pos := ch.positionOf(key); pos >= 0 {
			e := ch.entry.Pages[pos]
			return &e, nil
		}
	}
	return nil, apperror.NewNotFound("page not found")
}

// checkRequestPage decodes a page a Director sent with the strict book
// decoder and runs the book's own checks. It returns the JSON to store, which
// is the normalised page, not the raw request.
func checkRequestPage(raw []byte, widgets map[string]bool, position int) (bookPageYAML, string, error) {
	var p bookPageYAML
	if len(bytes.TrimSpace(raw)) == 0 {
		return p, "", apperror.NewValidation("The page is empty.")
	}
	raw, err := normalizeJSON(raw)
	if err != nil {
		return p, "", apperror.NewBadRequest("The page is not valid JSON.")
	}
	if err := decodeBookBytes(raw, &p); err != nil {
		return p, "", apperror.NewValidation(fmt.Sprintf("page %d: %s", position, friendlyDecodeError(err)))
	}
	if _, perr := buildStoredPage(p, widgets); perr != nil {
		return p, "", apperror.NewValidation(perr.detail(fmt.Sprintf("page %d", position)))
	}
	stored, err := json.Marshal(toAuthored(p))
	if err != nil {
		return p, "", apperror.NewInternal(err)
	}
	return p, string(stored), nil
}

var (
	yamlUnknownField = regexp.MustCompile(`field (\S+) not found in type \S+`)
	yamlGoTypeName   = regexp.MustCompile(`systems\.\w+`)
)

// normalizeJSON re-encodes request JSON compactly with real UTF-8. The strict
// book decoder is a YAML decoder (JSON is YAML), and YAML cannot read the
// surrogate-pair escapes JSON allows for characters outside the basic plane,
// nor tabs between tokens.
func normalizeJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data")
	}
	return json.Marshal(v)
}

// friendlyDecodeError turns the decoder's wording, which names Go types, into
// plain language a Director can act on.
func friendlyDecodeError(err error) string {
	var te *yaml.TypeError
	if errors.As(err, &te) {
		var msgs []string
		for _, m := range te.Errors {
			m = yamlUnknownField.ReplaceAllString(m, `there is no setting called "$1"`)
			msgs = append(msgs, yamlGoTypeName.ReplaceAllString(m, "a page or block"))
		}
		return strings.Join(msgs, "; ")
	}
	return "the page could not be read: " + err.Error()
}

// --- chapters ---

func (s *bookEditService) CreateChapter(ctx context.Context, campaignID, userID string, pkg *BookPackage, title string) (*BookChapterEntry, error) {
	title = strings.TrimSpace(title)
	if err := checkChapterTitle(title); err != nil {
		return nil, err
	}
	edits, err := s.repo.Load(ctx, campaignID, pkg.SystemID)
	if err != nil {
		return nil, err
	}
	if len(edits.Chapters) >= maxHouseChapters {
		return nil, apperror.NewValidation(fmt.Sprintf("A book can hold at most %d house-rules chapters.", maxHouseChapters))
	}
	total := len(edits.Chapters)
	for _, p := range pkg.Book.Parts {
		total += len(p.Chapters)
	}
	if total >= maxBookChapters {
		return nil, apperror.NewValidation(fmt.Sprintf("A book can hold at most %d chapters.", maxBookChapters))
	}
	starter, err := json.Marshal(authoredPage{Title: "New page", Blocks: []authoredBlock{{}}})
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	id := houseChapterPrefix + s.newID()
	err = s.repo.AddChapter(ctx,
		HouseChapter{CampaignID: campaignID, SystemID: pkg.SystemID, ChapterID: id, Title: title, CreatedBy: userID},
		StoredBookPage{PageJSON: string(starter), UpdatedBy: userID})
	if err != nil {
		return nil, err
	}
	return s.chapterEntry(ctx, campaignID, pkg, id)
}

func checkChapterTitle(title string) error {
	if title == "" {
		return apperror.NewValidation("A chapter needs a title.")
	}
	if len([]rune(title)) > maxHouseChapterTitle {
		return apperror.NewValidation(fmt.Sprintf("A chapter title can be at most %d characters.", maxHouseChapterTitle))
	}
	return nil
}

func (s *bookEditService) UpdateChapter(ctx context.Context, campaignID string, pkg *BookPackage, chapterID string, in UpdateBookChapterInput) (*BookChapterEntry, error) {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	ch := ed.byID[chapterID]
	if ch == nil {
		return nil, apperror.NewNotFound("chapter not found")
	}
	if ch.house == nil {
		return nil, apperror.NewValidation("Only house-rules chapters can be changed here.")
	}
	merged := *ch.house
	merged.Title = strings.TrimSpace(in.Title.Val(merged.Title))
	if err := checkChapterTitle(merged.Title); err != nil {
		return nil, err
	}
	if in.Intro.IsNull() {
		merged.Intro = ""
	} else {
		merged.Intro = in.Intro.Val(merged.Intro)
	}
	if len(merged.Intro) > maxBookString {
		return nil, apperror.NewValidation(fmt.Sprintf("The introduction is longer than %d characters.", maxBookString))
	}
	merged.Director = in.Director.Val(merged.Director)
	if err := s.repo.UpdateChapter(ctx, merged); err != nil {
		return nil, err
	}
	return s.chapterEntry(ctx, campaignID, pkg, chapterID)
}

func (s *bookEditService) DeleteChapter(ctx context.Context, campaignID string, pkg *BookPackage, chapterID string) error {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return err
	}
	ch := ed.byID[chapterID]
	if ch == nil {
		return apperror.NewNotFound("chapter not found")
	}
	if ch.house == nil {
		return apperror.NewValidation("Only house-rules chapters can be removed. Use a page's own controls to go back to the package's text.")
	}
	_, err = s.repo.DeleteChapter(ctx, campaignID, pkg.SystemID, chapterID)
	return err
}

func (s *bookEditService) chapterEntry(ctx context.Context, campaignID string, pkg *BookPackage, chapterID string) (*BookChapterEntry, error) {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}
	ch := ed.byID[chapterID]
	if ch == nil {
		return nil, apperror.NewNotFound("chapter not found")
	}
	e := ch.entry
	return &e, nil
}

// --- export ---

var exportSlugStrip = regexp.MustCompile(`[^a-z0-9]+`)

// exportHouseID names a house chapter's file in the export: "house-<slug>",
// made unique against every id already in the book so exporting can never
// overwrite a package chapter.
func exportHouseID(title string, used map[string]bool) string {
	slug := strings.Trim(exportSlugStrip.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	if slug == "" {
		slug = "chapter"
	}
	base := "house-" + slug
	id := base
	for n := 2; used[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	used[id] = true
	return id
}

func (s *bookEditService) Export(ctx context.Context, campaignID string, pkg *BookPackage) ([]byte, error) {
	ed, err := s.load(ctx, campaignID, pkg)
	if err != nil {
		return nil, err
	}

	used := map[string]bool{}
	for _, p := range ed.parts {
		for _, c := range p.chapters {
			if !c.entry.House {
				used[c.entry.ID] = true
			}
		}
	}

	idx := pkg.Source.index
	idx.Parts = append([]bookPartYAML(nil), idx.Parts...)
	files := map[string][]byte{}
	var houseIDs []string

	for _, p := range ed.parts {
		for _, c := range p.chapters {
			if !c.editable {
				if g := pkg.Source.generated[c.entry.ID]; g != nil && c.entry.Generated {
					data, err := marshalYAML(g)
					if err != nil {
						return nil, apperror.NewInternal(err)
					}
					files["book/"+bookChaptersDir+"/"+c.entry.ID+".yaml"] = data
				}
				continue // a broken package chapter has no authored form to write out
			}
			pages := make([]authoredPage, 0, len(c.entry.Pages))
			for _, pe := range c.entry.Pages {
				pg := pe.Page
				pg.Blocks = nonEmptyBlocks(pg.Blocks)
				if len(pg.Blocks) == 0 {
					continue // a starter page nobody wrote in would fail the package's own checks
				}
				pages = append(pages, pg)
			}
			if len(pages) == 0 {
				continue
			}
			out := authoredChapter{Title: c.entry.Title, Intro: c.entry.Intro, Director: c.entry.Director, Pages: pages}
			id := c.entry.ID
			if c.entry.House {
				id = exportHouseID(c.entry.Title, used)
				houseIDs = append(houseIDs, id)
			} else if ac := pkg.Source.chapters[id]; ac != nil {
				out.Title, out.Intro = ac.Title, ac.Intro
			}
			data, err := marshalYAML(out)
			if err != nil {
				return nil, apperror.NewInternal(err)
			}
			files["book/"+bookChaptersDir+"/"+id+".yaml"] = data
		}
	}
	if len(houseIDs) > 0 {
		idx.Parts = append(idx.Parts, bookPartYAML{Title: housePartTitle, Chapters: houseIDs})
	}
	data, err := marshalYAML(idx)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	files["book/"+bookIndexFile] = data

	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if _, err := w.Write(files[n]); err != nil {
			return nil, apperror.NewInternal(err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return buf.Bytes(), nil
}

// nonEmptyBlocks drops text blocks with no text, which the book checks reject.
func nonEmptyBlocks(in []authoredBlock) []authoredBlock {
	var out []authoredBlock
	for _, b := range in {
		if (b.Type == "" || b.Type == "text") && strings.TrimSpace(b.Text) == "" {
			continue
		}
		out = append(out, b)
	}
	return out
}

// marshalYAML writes two-space YAML like the files authors write by hand.
func marshalYAML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
