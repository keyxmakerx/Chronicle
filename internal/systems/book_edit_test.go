package systems

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// fakeBookRepo is an in-memory BookEditRepository, scoped like the real one.
type fakeBookRepo struct {
	mu       sync.Mutex
	chapters []HouseChapter
	pages    []StoredBookPage
	nextID   int64
	writes   int
}

func (f *fakeBookRepo) Load(_ context.Context, campaignID, systemID string) (*BookEdits, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &BookEdits{}
	for _, c := range f.chapters {
		if c.CampaignID == campaignID && c.SystemID == systemID {
			out.Chapters = append(out.Chapters, c)
		}
	}
	for _, p := range f.pages {
		if p.CampaignID == campaignID && p.SystemID == systemID {
			out.Pages = append(out.Pages, p)
		}
	}
	return out, nil
}

func (f *fakeBookRepo) find(c, s, ch string, index *int, id int64) int {
	for i, p := range f.pages {
		if p.CampaignID != c || p.SystemID != s || p.ChapterID != ch {
			continue
		}
		if index != nil && p.PackageIndex != nil && *p.PackageIndex == *index {
			return i
		}
		if index == nil && p.PackageIndex == nil && p.ID == id {
			return i
		}
	}
	return -1
}

func (f *fakeBookRepo) SaveCopy(_ context.Context, p StoredBookPage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if i := f.find(p.CampaignID, p.SystemID, p.ChapterID, p.PackageIndex, 0); i >= 0 {
		p.ID = f.pages[i].ID
		f.pages[i] = p
		return nil
	}
	f.nextID++
	p.ID = f.nextID
	f.pages = append(f.pages, p)
	return nil
}

func (f *fakeBookRepo) AddOwn(_ context.Context, p StoredBookPage) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	f.nextID++
	p.ID, p.PackageIndex = f.nextID, nil
	p.SortOrder = int(f.nextID)
	f.pages = append(f.pages, p)
	return p.ID, nil
}

func (f *fakeBookRepo) UpdateOwn(_ context.Context, c, s, ch string, id int64, pageJSON, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if i := f.find(c, s, ch, nil, id); i >= 0 {
		f.pages[i].PageJSON, f.pages[i].UpdatedBy = pageJSON, userID
	}
	return nil
}

func (f *fakeBookRepo) DeleteCopy(_ context.Context, c, s, ch string, index int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.find(c, s, ch, &index, 0)
	if i < 0 {
		return false, nil
	}
	f.pages = append(f.pages[:i], f.pages[i+1:]...)
	return true, nil
}

func (f *fakeBookRepo) DeleteOwn(_ context.Context, c, s, ch string, id int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.find(c, s, ch, nil, id)
	if i < 0 {
		return false, nil
	}
	f.pages = append(f.pages[:i], f.pages[i+1:]...)
	return true, nil
}

func (f *fakeBookRepo) PromoteCopy(_ context.Context, c, s, ch string, index int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.find(c, s, ch, &index, 0)
	if i < 0 {
		return 0, apperror.NewNotFound("no copy")
	}
	f.pages[i].PackageIndex, f.pages[i].BaseHash = nil, ""
	f.pages[i].SortOrder = 1000 + int(f.pages[i].ID)
	return f.pages[i].ID, nil
}

func (f *fakeBookRepo) AddChapter(_ context.Context, ch HouseChapter, first StoredBookPage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	ch.SortOrder = len(f.chapters) + 1
	f.chapters = append(f.chapters, ch)
	f.nextID++
	first.ID, first.CampaignID, first.SystemID, first.ChapterID, first.SortOrder = f.nextID, ch.CampaignID, ch.SystemID, ch.ChapterID, 1
	f.pages = append(f.pages, first)
	return nil
}

func (f *fakeBookRepo) UpdateChapter(_ context.Context, ch HouseChapter) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	for i, c := range f.chapters {
		if c.CampaignID == ch.CampaignID && c.SystemID == ch.SystemID && c.ChapterID == ch.ChapterID {
			ch.SortOrder = c.SortOrder
			f.chapters[i] = ch
		}
	}
	return nil
}

func (f *fakeBookRepo) DeleteChapter(_ context.Context, c, s, ch string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	found := false
	var keep []HouseChapter
	for _, x := range f.chapters {
		if x.CampaignID == c && x.SystemID == s && x.ChapterID == ch {
			found = true
			continue
		}
		keep = append(keep, x)
	}
	f.chapters = keep
	var pk []StoredBookPage
	for _, p := range f.pages {
		if p.CampaignID == c && p.SystemID == s && p.ChapterID == ch {
			continue
		}
		pk = append(pk, p)
	}
	f.pages = pk
	return found, nil
}

const testCampaign = "camp-1"

// testPackage loads the shared fixture book as a package of system "ds".
func testPackage(t *testing.T, files map[string]string) *BookPackage {
	t.Helper()
	if files == nil {
		files = testBookFiles()
	}
	dir := writeBook(t, files)
	pkg, err := LoadBookPackage(dir, &SystemManifest{ID: "ds", Name: "Draw Steel", Widgets: []WidgetDef{{Slug: "rulebook-frontpage"}}})
	if err != nil {
		t.Fatalf("LoadBookPackage: %v", err)
	}
	return pkg
}

func newTestService(repo *fakeBookRepo) BookEditService {
	svc := NewBookEditService(repo).(*bookEditService)
	n := 0
	svc.newID = func() string { n++; return fmt.Sprintf("%08d", n) }
	return svc
}

func intp(n int) *int { return &n }

func storedCopy(chapter string, index int, baseHash, page string) StoredBookPage {
	return StoredBookPage{ID: int64(100 + index), CampaignID: testCampaign, SystemID: "ds", ChapterID: chapter, PackageIndex: intp(index), PageJSON: page, BaseHash: baseHash}
}

func storedOwn(id int64, chapter string, order int, page string) StoredBookPage {
	return StoredBookPage{ID: id, CampaignID: testCampaign, SystemID: "ds", ChapterID: chapter, SortOrder: order, PageJSON: page}
}

func chapterOf(t *testing.T, b *Book, id string) BookChapter {
	t.Helper()
	for _, p := range b.Parts {
		for _, c := range p.Chapters {
			if c.ID == id {
				return c
			}
		}
	}
	t.Fatalf("chapter %q not in the book", id)
	return BookChapter{}
}

func pageTitles(c BookChapter) []string {
	var out []string
	for _, p := range c.Pages {
		out = append(out, p.Title)
	}
	return out
}

func authoredHash(t *testing.T, pkg *BookPackage, chapter string, i int) string {
	t.Helper()
	return hashBookPage(pkg.Source.chapters[chapter].Pages[i])
}

func TestApplyBookEdits_MergeStates(t *testing.T) {
	pkg := testPackage(t, nil)
	h0 := authoredHash(t, pkg, "basics", 0)
	editedPage := `{"title":"My roll","blocks":[{"text":"House version."}]}`

	tests := []struct {
		name       string
		edits      BookEdits
		wantTitles []string
		wantStates []string
		wantKeys   []string
	}{
		{
			name:       "no edits is the package book",
			wantTitles: []string{"The power roll", "SECRET-PAGE"},
			wantStates: []string{"package", "package"},
			wantKeys:   []string{"p0", "p1"},
		},
		{
			name:       "copy of an unchanged page is edited",
			edits:      BookEdits{Pages: []StoredBookPage{storedCopy("basics", 0, h0, editedPage)}},
			wantTitles: []string{"My roll", "SECRET-PAGE"},
			wantStates: []string{"edited", "package"},
			wantKeys:   []string{"p0", "p1"},
		},
		{
			name:       "copy made from an older package page is changed",
			edits:      BookEdits{Pages: []StoredBookPage{storedCopy("basics", 0, "stale", editedPage)}},
			wantTitles: []string{"My roll", "SECRET-PAGE"},
			wantStates: []string{"changed", "package"},
			wantKeys:   []string{"p0", "p1"},
		},
		{
			name:       "copy without a base hash is changed",
			edits:      BookEdits{Pages: []StoredBookPage{storedCopy("basics", 1, "", editedPage)}},
			wantTitles: []string{"The power roll", "My roll"},
			wantStates: []string{"package", "changed"},
			wantKeys:   []string{"p0", "p1"},
		},
		{
			name: "own pages follow the package pages in order",
			edits: BookEdits{Pages: []StoredBookPage{
				storedOwn(7, "basics", 2, `{"title":"Second","blocks":[{"text":"b"}]}`),
				storedOwn(5, "basics", 1, `{"title":"First","blocks":[{"text":"a"}]}`),
			}},
			wantTitles: []string{"The power roll", "SECRET-PAGE", "First", "Second"},
			wantStates: []string{"package", "package", "mine", "mine"},
			wantKeys:   []string{"p0", "p1", "o5", "o7"},
		},
		{
			name: "copy past the package end is kept, changed, before own pages",
			edits: BookEdits{Pages: []StoredBookPage{
				storedOwn(5, "basics", 1, `{"title":"Own","blocks":[{"text":"a"}]}`),
				storedCopy("basics", 4, "x", `{"title":"Orphan B","blocks":[{"text":"b"}]}`),
				storedCopy("basics", 3, "x", `{"title":"Orphan A","blocks":[{"text":"a"}]}`),
			}},
			wantTitles: []string{"The power roll", "SECRET-PAGE", "Orphan A", "Orphan B", "Own"},
			wantStates: []string{"package", "package", "changed", "changed", "mine"},
			wantKeys:   []string{"p0", "p1", "p3", "p4", "o5"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edits := tt.edits
			book := ApplyBookEdits(pkg, &edits)
			ch := chapterOf(t, book, "basics")
			if got := pageTitles(ch); strings.Join(got, "|") != strings.Join(tt.wantTitles, "|") {
				t.Errorf("titles = %v, want %v", got, tt.wantTitles)
			}

			ed := buildEdition(pkg, &edits)
			var states, keys []string
			for _, pe := range ed.byID["basics"].entry.Pages {
				states, keys = append(states, pe.State), append(keys, pe.Key)
			}
			if strings.Join(states, "|") != strings.Join(tt.wantStates, "|") {
				t.Errorf("states = %v, want %v", states, tt.wantStates)
			}
			if strings.Join(keys, "|") != strings.Join(tt.wantKeys, "|") {
				t.Errorf("keys = %v, want %v", keys, tt.wantKeys)
			}
		})
	}
}

func TestBuildEdition_TheirsIsThePackagePage(t *testing.T) {
	pkg := testPackage(t, nil)
	edits := &BookEdits{Pages: []StoredBookPage{
		storedCopy("basics", 0, "stale", `{"title":"Mine","blocks":[{"text":"x"}]}`),
		storedCopy("basics", 5, "x", `{"title":"Orphan","blocks":[{"text":"x"}]}`),
	}}
	ed := buildEdition(pkg, edits)
	pages := ed.byID["basics"].entry.Pages
	if pages[0].Theirs == nil || pages[0].Theirs.Title != "The power roll" {
		t.Errorf("changed page theirs = %+v", pages[0].Theirs)
	}
	if pages[1].Theirs != nil {
		t.Errorf("untouched package page must not carry theirs: %+v", pages[1].Theirs)
	}
	if last := pages[len(pages)-1]; last.Theirs != nil {
		t.Errorf("a copy past the package end has no theirs, got %+v", last.Theirs)
	}
}

func TestApplyBookEdits_IgnoresEditsForChaptersThePackageDoesNotList(t *testing.T) {
	pkg := testPackage(t, nil)
	edits := &BookEdits{Pages: []StoredBookPage{
		storedCopy("removed-chapter", 0, "x", `{"title":"GHOST","blocks":[{"text":"x"}]}`),
		storedOwn(9, "removed-chapter", 1, `{"title":"GHOST-OWN","blocks":[{"text":"x"}]}`),
		// A package chapter that failed to load cannot be merged into either.
		storedCopy("broken", 0, "x", `{"title":"GHOST-BROKEN","blocks":[{"text":"x"}]}`),
	}}
	raw, _ := json.Marshal(ApplyBookEdits(pkg, edits))
	if strings.Contains(string(raw), "GHOST") {
		t.Errorf("edit for a missing or broken chapter leaked into the book: %s", raw)
	}
	if len(chapterOf(t, ApplyBookEdits(pkg, edits), "broken").Pages[0].Blocks) != 1 {
		t.Error("broken chapter must stay the package's problem chapter")
	}
}

func TestApplyBookEdits_HouseChaptersFormLastPart(t *testing.T) {
	pkg := testPackage(t, nil)
	edits := &BookEdits{
		Chapters: []HouseChapter{
			{ChapterID: "house_b", Title: "Second", SortOrder: 2},
			{ChapterID: "house_a", Title: "First", Intro: "  Our table.  ", SortOrder: 1, Director: true},
		},
		Pages: []StoredBookPage{
			storedOwn(1, "house_a", 1, `{"title":"A1","blocks":[{"text":"a"}]}`),
			storedOwn(2, "house_b", 1, `{"title":"B1","blocks":[{"text":"b"}]}`),
		},
	}
	book := ApplyBookEdits(pkg, edits)
	last := book.Parts[len(book.Parts)-1]
	if last.Title != "House rules" || last.Director {
		t.Fatalf("last part = %+v", last)
	}
	if len(last.Chapters) != 2 || last.Chapters[0].ID != "house_a" || last.Chapters[0].Intro != "Our table." || !last.Chapters[0].Director {
		t.Errorf("house chapters = %+v", last.Chapters)
	}
	if len(book.Parts) != len(pkg.Book.Parts)+1 {
		t.Error("package parts must be untouched")
	}
	if len(ApplyBookEdits(pkg, &BookEdits{}).Parts) != len(pkg.Book.Parts) {
		t.Error("no house chapters, no House rules part")
	}
}

// TestApplyBookEdits_PlayersNeverGetDirectorEdits: the merge runs before the
// filter, so Director flags on campaign pages and blocks are honoured.
func TestApplyBookEdits_PlayersNeverGetDirectorEdits(t *testing.T) {
	pkg := testPackage(t, nil)
	h0 := authoredHash(t, pkg, "basics", 0)
	edits := &BookEdits{
		Chapters: []HouseChapter{
			{ChapterID: "house_secret", Title: "HOUSE-SECRET-CHAPTER", Director: true, SortOrder: 1},
			{ChapterID: "house_open", Title: "Open house", SortOrder: 2},
		},
		Pages: []StoredBookPage{
			storedCopy("basics", 0, h0, `{"title":"EDITED-SECRET-PAGE","director":true,"blocks":[{"text":"EDITED-SECRET-TEXT"}]}`),
			storedOwn(1, "basics", 1, `{"title":"Public own","blocks":[{"text":"visible"},{"type":"note","text":"OWN-SECRET-NOTE"},{"text":"OWN-SECRET-BLOCK","director":true},{"type":"creature","name":"Imp","look":"small","stats":[{"label":"Stamina","value":"OWN-SECRET-STAT"}],"note":"OWN-SECRET-CREATURE-NOTE"}]}`),
			storedOwn(2, "house_secret", 1, `{"title":"x","blocks":[{"text":"HOUSE-SECRET-TEXT"}]}`),
			storedOwn(3, "house_open", 1, `{"title":"Open page","blocks":[{"text":"house text"}]}`),
		},
	}
	book := ApplyBookEdits(pkg, edits)

	playerRaw, _ := json.Marshal(FilterBook(book, false))
	for _, leak := range []string{"SECRET", "problem"} {
		if strings.Contains(string(playerRaw), leak) {
			t.Errorf("player copy leaks %q: %s", leak, playerRaw)
		}
	}
	for _, want := range []string{"Public own", "visible", "Open page", "Imp", "small"} {
		if !strings.Contains(string(playerRaw), want) {
			t.Errorf("player copy is missing %q", want)
		}
	}

	dirRaw, _ := json.Marshal(FilterBook(book, true))
	for _, want := range []string{"EDITED-SECRET-TEXT", "OWN-SECRET-NOTE", "OWN-SECRET-BLOCK", "OWN-SECRET-STAT", "HOUSE-SECRET-TEXT"} {
		if !strings.Contains(string(dirRaw), want) {
			t.Errorf("director copy is missing %q", want)
		}
	}
}

func TestApplyBookEdits_StoredPageThatNoLongerValidatesBecomesAProblem(t *testing.T) {
	pkg := testPackage(t, nil)
	edits := &BookEdits{Pages: []StoredBookPage{
		storedOwn(1, "basics", 1, `{"title":"Bad dice","blocks":[{"type":"roll","dice":"2d1000","bands":[{"label":"x"}]}]}`),
		storedOwn(2, "basics", 2, `{"title":"Bad widget","blocks":[{"type":"widget","widget":"gone"}]}`),
		storedOwn(3, "basics", 3, `{"title":"Bad key","blocks":[{"text":"x","colour":"red"}]}`),
		storedOwn(4, "basics", 4, `not json at all: [`),
	}}
	book := ApplyBookEdits(pkg, edits)
	ch := chapterOf(t, book, "basics")
	problems := 0
	for _, p := range ch.Pages[2:] {
		if len(p.Blocks) == 1 && p.Blocks[0].Type == "problem" {
			problems++
		}
	}
	if problems != 4 {
		t.Fatalf("problem pages = %d, want 4: %+v", problems, ch.Pages[2:])
	}
	if got := ch.Pages[2].Blocks[0].Text; !strings.Contains(got, "page 3, block 1") || !strings.Contains(got, "dice") {
		t.Errorf("director-facing reason = %q", got)
	}
	player, _ := json.Marshal(FilterBook(book, false))
	if strings.Contains(string(player), "Bad ") || strings.Contains(string(player), "problem") {
		t.Errorf("problem pages must never reach players: %s", player)
	}
	ed := buildEdition(pkg, edits)
	if e := ed.byID["basics"].entry.Pages[2]; e.Problem == "" || e.Page.Title != "Bad dice" {
		t.Errorf("editor must still get the page to fix, got %+v", e)
	}
}

func TestApplyBookEdits_StarterPageShowsNothing(t *testing.T) {
	pkg := testPackage(t, nil)
	edits := &BookEdits{
		Chapters: []HouseChapter{{ChapterID: "house_a", Title: "New"}},
		Pages:    []StoredBookPage{storedOwn(1, "house_a", 1, `{"title":"New page","blocks":[{}]}`)},
	}
	book := ApplyBookEdits(pkg, edits)
	raw, _ := json.Marshal(chapterOf(t, book, "house_a"))
	if strings.Contains(string(raw), "problem") {
		t.Errorf("an unwritten starter page is not a mistake: %s", raw)
	}
	if player, _ := json.Marshal(FilterBook(book, false)); strings.Contains(string(player), "New page") {
		t.Errorf("an empty page must not reach players: %s", player)
	}
}

func TestBookEditService_RequestPageValidation(t *testing.T) {
	pkg := testPackage(t, nil)
	tests := []struct {
		name string
		page string
		want string
	}{
		{"unknown page key", `{"title":"x","colour":"red","blocks":[{"text":"a"}]}`, `no setting called "colour"`},
		{"unknown block key", `{"blocks":[{"text":"a","bogus":1}]}`, `no setting called "bogus"`},
		{"bad dice", `{"blocks":[{"text":"ok"},{"type":"roll","dice":"2d1000","bands":[{"label":"x"}]}]}`, "block 2: dice must look like 2d10"},
		{"widget not in manifest", `{"blocks":[{"type":"widget","widget":"evil"}]}`, "not one of this package's widgets"},
		{"unknown block type", `{"blocks":[{"type":"rol"}]}`, `no block type "rol"`},
		{"no blocks", `{"title":"x","blocks":[]}`, "no blocks"},
		{"empty text block", `{"blocks":[{"text":""}]}`, "needs text"},
		{"too long", `{"blocks":[{"text":"` + strings.Repeat("x", maxBookString+1) + `"}]}`, "longer than"},
		{"null page", `null`, "no blocks"},
		{"not an object", `[1,2]`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeBookRepo{}
			svc := newTestService(repo)
			ctx := context.Background()
			_, errPut := svc.SavePage(ctx, testCampaign, "u1", pkg, "basics", "p0", []byte(tt.page))
			_, errAdd := svc.AddPage(ctx, testCampaign, "u1", pkg, "basics", []byte(tt.page))
			for _, err := range []error{errPut, errAdd} {
				if err == nil {
					t.Fatal("expected a validation error")
				}
				if code := apperror.SafeCode(err); code != 422 && code != 400 {
					t.Errorf("code = %d, want 422 (or 400 for unreadable JSON): %v", code, err)
				}
				if tt.want != "" && !strings.Contains(apperror.SafeMessage(err), tt.want) {
					t.Errorf("message = %q, want it to mention %q", apperror.SafeMessage(err), tt.want)
				}
				if strings.Contains(apperror.SafeMessage(err), "systems.") {
					t.Errorf("message names a Go type: %q", apperror.SafeMessage(err))
				}
			}
			if repo.writes != 0 {
				t.Errorf("a rejected page must not be stored (%d writes)", repo.writes)
			}
		})
	}
}

func TestBookEditService_PageLifecycle(t *testing.T) {
	pkg := testPackage(t, nil)
	repo := &fakeBookRepo{}
	svc := newTestService(repo)
	ctx := context.Background()
	h0 := authoredHash(t, pkg, "basics", 0)

	// First edit of a package page: a copy tied to the package page it came from.
	e, err := svc.SavePage(ctx, testCampaign, "u1", pkg, "basics", "p0", []byte(`{"title":"Mine","blocks":[{"text":"x"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if e.Key != "p0" || e.State != "edited" || e.Theirs == nil || e.Theirs.Title != "The power roll" {
		t.Errorf("after first save: %+v", e)
	}
	if repo.pages[0].BaseHash != h0 || repo.pages[0].UpdatedBy != "u1" {
		t.Errorf("stored copy = %+v", repo.pages[0])
	}

	// The package moves on; saving again must not quietly settle the conflict.
	repo.pages[0].BaseHash = "older-package-page"
	e, err = svc.SavePage(ctx, testCampaign, "u2", pkg, "basics", "p0", []byte(`{"title":"Mine v2","blocks":[{"text":"y"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if e.State != "changed" || repo.pages[0].BaseHash != "older-package-page" {
		t.Errorf("a re-save must keep the changed flag: %+v / %q", e, repo.pages[0].BaseHash)
	}

	// Keep mine settles it.
	e, err = svc.KeepPage(ctx, testCampaign, "u2", pkg, "basics", "p0")
	if err != nil || e.State != "edited" || repo.pages[0].BaseHash != h0 {
		t.Errorf("keep: %+v, %v, hash %q", e, err, repo.pages[0].BaseHash)
	}
	if _, err := svc.KeepPage(ctx, testCampaign, "u2", pkg, "basics", "p1"); apperror.SafeCode(err) != 422 {
		t.Errorf("keep on an untouched page: %v", err)
	}

	// Go back to the package's page.
	e, err = svc.DeletePage(ctx, testCampaign, pkg, "basics", "p0")
	if err != nil || e == nil || e.State != "package" || e.Page.Title != "The power roll" || len(repo.pages) != 0 {
		t.Errorf("drop copy: %+v, %v", e, err)
	}

	// Own pages: add, edit, delete.
	e, err = svc.AddPage(ctx, testCampaign, "u1", pkg, "basics", []byte(`{"title":"Extra","blocks":[{"text":"z"}]}`))
	if err != nil || e.State != "mine" || !strings.HasPrefix(e.Key, "o") {
		t.Fatalf("add: %+v, %v", e, err)
	}
	if e, err = svc.SavePage(ctx, testCampaign, "u1", pkg, "basics", e.Key, []byte(`{"title":"Extra 2","blocks":[{"text":"z"}]}`)); err != nil || e.Page.Title != "Extra 2" {
		t.Errorf("edit own: %+v, %v", e, err)
	}
	key := e.Key
	if e, err = svc.DeletePage(ctx, testCampaign, pkg, "basics", key); err != nil || e != nil || len(repo.pages) != 0 {
		t.Errorf("delete own: %+v, %v", e, err)
	}

	// Addresses that do not exist.
	for _, k := range []string{"p9", "o12345", "x1", "p-1", "p", "o1x"} {
		if _, err := svc.SavePage(ctx, testCampaign, "u1", pkg, "basics", k, []byte(`{"blocks":[{"text":"a"}]}`)); err == nil {
			t.Errorf("save to %q succeeded", k)
		}
	}
	if _, err := svc.AddPage(ctx, testCampaign, "u1", pkg, "nope", []byte(`{"blocks":[{"text":"a"}]}`)); apperror.SafeCode(err) != 404 {
		t.Errorf("unknown chapter: %v", err)
	}
	if _, err := svc.AddPage(ctx, testCampaign, "u1", pkg, "broken", []byte(`{"blocks":[{"text":"a"}]}`)); apperror.SafeCode(err) != 422 {
		t.Errorf("broken package chapter must not take pages: %v", err)
	}
}

func TestBookEditService_OrphanedCopyBecomesOwnPageOnKeep(t *testing.T) {
	pkg := testPackage(t, nil)
	repo := &fakeBookRepo{pages: []StoredBookPage{storedCopy("basics", 6, "old", `{"title":"Orphan","blocks":[{"text":"x"}]}`)}}
	repo.pages[0].CampaignID, repo.pages[0].SystemID = testCampaign, "ds"
	svc := newTestService(repo)
	e, err := svc.KeepPage(context.Background(), testCampaign, "u1", pkg, "basics", "p6")
	if err != nil || e.State != "mine" || !strings.HasPrefix(e.Key, "o") {
		t.Fatalf("keep orphan: %+v, %v", e, err)
	}
	// Dropping an orphan leaves nothing behind.
	repo.pages = []StoredBookPage{storedCopy("basics", 6, "old", `{"title":"Orphan","blocks":[{"text":"x"}]}`)}
	if e, err := svc.DeletePage(context.Background(), testCampaign, pkg, "basics", "p6"); err != nil || e != nil {
		t.Errorf("drop orphan: %+v, %v", e, err)
	}
}

func TestBookEditService_Limits(t *testing.T) {
	ctx := context.Background()
	page := []byte(`{"blocks":[{"text":"a"}]}`)

	t.Run("own pages per chapter", func(t *testing.T) {
		pkg := testPackage(t, nil)
		repo := &fakeBookRepo{}
		for i := 1; i <= maxOwnPagesPerChapter; i++ {
			repo.pages = append(repo.pages, storedOwn(int64(i), "basics", i, `{"blocks":[{"text":"a"}]}`))
		}
		svc := newTestService(repo)
		if _, err := svc.AddPage(ctx, testCampaign, "u", pkg, "basics", page); apperror.SafeCode(err) != 422 {
			t.Errorf("201st own page: %v", err)
		}
		// Another chapter is unaffected.
		if _, err := svc.AddPage(ctx, testCampaign, "u", pkg, "secret-in-player-part", page); err != nil {
			t.Errorf("other chapter: %v", err)
		}
	})

	t.Run("house chapters", func(t *testing.T) {
		pkg := testPackage(t, nil)
		repo := &fakeBookRepo{}
		for i := 0; i < maxHouseChapters; i++ {
			repo.chapters = append(repo.chapters, HouseChapter{CampaignID: testCampaign, SystemID: "ds", ChapterID: fmt.Sprintf("house_%d", i), Title: "c"})
		}
		if _, err := newTestService(repo).CreateChapter(ctx, testCampaign, "u", pkg, "One more"); apperror.SafeCode(err) != 422 {
			t.Errorf("51st house chapter: %v", err)
		}
	})

	t.Run("chapters in the whole book", func(t *testing.T) {
		files := map[string]string{"book/book.yaml": ""}
		var ids []string
		for i := 0; i < maxBookChapters-2; i++ {
			id := fmt.Sprintf("c%d", i)
			ids = append(ids, id)
			files["book/chapters/"+id+".yaml"] = "title: T\npages:\n  - blocks:\n      - text: hi\n"
		}
		files["book/book.yaml"] = "parts:\n  - title: P\n    chapters: [" + strings.Join(ids, ",") + "]\n"
		pkg := testPackage(t, files)
		repo := &fakeBookRepo{}
		svc := newTestService(repo)
		for i := 0; i < 2; i++ {
			if _, err := svc.CreateChapter(ctx, testCampaign, "u", pkg, "House"); err != nil {
				t.Fatalf("chapter %d: %v", i, err)
			}
		}
		if _, err := svc.CreateChapter(ctx, testCampaign, "u", pkg, "Over"); apperror.SafeCode(err) != 422 {
			t.Errorf("chapter 201: %v", err)
		}
	})

	t.Run("titles", func(t *testing.T) {
		pkg := testPackage(t, nil)
		svc := newTestService(&fakeBookRepo{})
		for _, title := range []string{"", "   ", strings.Repeat("x", maxHouseChapterTitle+1)} {
			if _, err := svc.CreateChapter(ctx, testCampaign, "u", pkg, title); apperror.SafeCode(err) != 422 {
				t.Errorf("title %q: %v", title, err)
			}
		}
	})
}

func TestBookEditService_HouseChapterLifecycle(t *testing.T) {
	ctx := context.Background()
	pkg := testPackage(t, nil)
	repo := &fakeBookRepo{}
	svc := newTestService(repo)

	ch, err := svc.CreateChapter(ctx, testCampaign, "u1", pkg, "  Table rules ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ch.ID, "house_") || !ch.House || ch.Title != "Table rules" || len(ch.Pages) != 1 || ch.Pages[0].State != "mine" {
		t.Fatalf("created = %+v", ch)
	}

	// Rename only: intro and the Directors-only flag are untouched.
	repo.chapters[0].Intro, repo.chapters[0].Director = "Welcome.", true
	got, err := svc.UpdateChapter(ctx, testCampaign, pkg, ch.ID, UpdateBookChapterInput{Title: patch.Of("Renamed")})
	if err != nil || got.Title != "Renamed" || got.Intro != "Welcome." || !got.Director {
		t.Errorf("rename-only: %+v, %v", got, err)
	}
	// Explicit null clears the intro; an absent director still preserves.
	got, err = svc.UpdateChapter(ctx, testCampaign, pkg, ch.ID, UpdateBookChapterInput{Intro: patch.Null[string]()})
	if err != nil || got.Intro != "" || got.Title != "Renamed" || !got.Director {
		t.Errorf("null intro: %+v, %v", got, err)
	}
	// A present value replaces; null title/director cannot clear a required field.
	got, err = svc.UpdateChapter(ctx, testCampaign, pkg, ch.ID, UpdateBookChapterInput{Director: patch.Of(false), Title: patch.Null[string]()})
	if err != nil || got.Director || got.Title != "Renamed" {
		t.Errorf("director false: %+v, %v", got, err)
	}
	if _, err := svc.UpdateChapter(ctx, testCampaign, pkg, ch.ID, UpdateBookChapterInput{Title: patch.Of("  ")}); apperror.SafeCode(err) != 422 {
		t.Errorf("blank title: %v", err)
	}
	if _, err := svc.UpdateChapter(ctx, testCampaign, pkg, "basics", UpdateBookChapterInput{Title: patch.Of("x")}); apperror.SafeCode(err) != 422 {
		t.Errorf("renaming a package chapter: %v", err)
	}

	// The only page cannot be deleted; a second one makes that possible.
	only := ch.Pages[0].Key
	if _, err := svc.DeletePage(ctx, testCampaign, pkg, ch.ID, only); apperror.SafeCode(err) != 422 {
		t.Errorf("delete last page: %v", err)
	}
	if _, err := svc.AddPage(ctx, testCampaign, "u1", pkg, ch.ID, []byte(`{"blocks":[{"text":"a"}]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeletePage(ctx, testCampaign, pkg, ch.ID, only); err != nil {
		t.Errorf("delete with a sibling: %v", err)
	}

	// Removing the chapter takes its pages along; package chapters stay.
	if err := svc.DeleteChapter(ctx, testCampaign, pkg, "basics"); apperror.SafeCode(err) != 422 {
		t.Errorf("deleting a package chapter: %v", err)
	}
	if err := svc.DeleteChapter(ctx, testCampaign, pkg, ch.ID); err != nil {
		t.Fatal(err)
	}
	if len(repo.chapters) != 0 || len(repo.pages) != 0 {
		t.Errorf("left behind: %+v %+v", repo.chapters, repo.pages)
	}
	if err := svc.DeleteChapter(ctx, testCampaign, pkg, ch.ID); apperror.SafeCode(err) != 404 {
		t.Errorf("deleting twice: %v", err)
	}
}

// TestBookEditService_CampaignsAreIsolated: one campaign's edits never show
// in another campaign's edition.
func TestBookEditService_CampaignsAreIsolated(t *testing.T) {
	ctx := context.Background()
	pkg := testPackage(t, nil)
	svc := newTestService(&fakeBookRepo{})
	if _, err := svc.AddPage(ctx, "camp-a", "u", pkg, "basics", []byte(`{"title":"ONLY-A","blocks":[{"text":"a"}]}`)); err != nil {
		t.Fatal(err)
	}
	b, err := svc.Edition(ctx, "camp-b", pkg)
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(b); strings.Contains(string(raw), "ONLY-A") {
		t.Error("campaign B sees campaign A's page")
	}
}

// A campaign's stored pages are bounded in total, since every reader's GET
// /book loads all of them; re-saving a page near the limit still works.
func TestBookEditService_EditionSizeLimit(t *testing.T) {
	pkg := testPackage(t, nil)
	big := `{"blocks":[{"text":"` + strings.Repeat("x", maxEditionBytes/2) + `"}]}`
	repo := &fakeBookRepo{pages: []StoredBookPage{
		storedOwn(1, "basics", 1, big),
		storedOwn(2, "basics", 2, big),
	}}
	svc := newTestService(repo)
	ctx := context.Background()
	page := []byte(`{"title":"More","blocks":[{"text":"one more page"}]}`)

	if _, err := svc.AddPage(ctx, testCampaign, "u1", pkg, "basics", page); apperror.SafeCode(err) != 422 {
		t.Fatalf("AddPage past the limit: want 422, got %v", err)
	}
	if _, err := svc.SavePage(ctx, testCampaign, "u1", pkg, "basics", "p0", page); apperror.SafeCode(err) != 422 {
		t.Fatalf("SavePage of a new copy past the limit: want 422, got %v", err)
	}
	if _, err := svc.SavePage(ctx, testCampaign, "u1", pkg, "basics", "o1", page); err != nil {
		t.Fatalf("shrinking a page at the limit must succeed: %v", err)
	}
}
