package dmscreen

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// memBoard is an in-memory BoardRepository that enforces the same one-pin-per-
// page rule as the table's unique key.
type memBoard struct {
	items []BoardItem
}

func (m *memBoard) List(_ context.Context, campaignID string) ([]BoardItem, error) {
	var out []BoardItem
	for _, it := range m.items {
		if it.CampaignID == campaignID {
			out = append(out, it)
		}
	}
	return out, nil
}

func (m *memBoard) Count(ctx context.Context, campaignID string) (int, error) {
	l, _ := m.List(ctx, campaignID)
	return len(l), nil
}

func (m *memBoard) Add(_ context.Context, it *BoardItem) error {
	if it.Kind == BoardKindPage {
		for _, o := range m.items {
			if o.CampaignID == it.CampaignID && o.EntityID == it.EntityID {
				return ErrAlreadyPinned
			}
		}
	}
	m.items = append(m.items, *it)
	return nil
}

func (m *memBoard) Delete(_ context.Context, campaignID, id string) (bool, error) {
	for i, it := range m.items {
		if it.ID == id && it.CampaignID == campaignID {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

// fakeAdventures answers only for the pages it holds, as the real source does
// for pages the viewer may open in the Adventures category.
type fakeAdventures struct {
	pages       map[string]AdventurePage
	noCategory  bool
	lastViewer  Viewer
	searchQuery string
}

func (f *fakeAdventures) Pages(_ context.Context, _ string, ids []string, v Viewer) (map[string]AdventurePage, error) {
	f.lastViewer = v
	out := map[string]AdventurePage{}
	for _, id := range ids {
		if p, ok := f.pages[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func (f *fakeAdventures) Search(_ context.Context, _, query string, _ Viewer, _ int) ([]AdventurePage, bool, error) {
	f.searchQuery = query
	if f.noCategory {
		return nil, false, nil
	}
	var out []AdventurePage
	for _, p := range f.pages {
		out = append(out, p)
	}
	return out, true, nil
}

var (
	dmViewer     = Viewer{UserID: "dm", Role: 3}
	scribeViewer = Viewer{UserID: "sc", Role: 2}
	playerViewer = Viewer{UserID: "pl", Role: 1}
)

func newBoardSvc(adv *fakeAdventures, b *memBoard) Service {
	return NewService(Sources{Board: b, Adventures: adv})
}

func codeOf(err error) int {
	if err == nil {
		return 0
	}
	return apperror.SafeCode(err)
}

func TestBoard_PlayersAreRefusedEverywhere(t *testing.T) {
	adv := &fakeAdventures{pages: map[string]AdventurePage{"p1": {ID: "p1", Name: "Lastwell"}}}
	b := &memBoard{}
	svc := newBoardSvc(adv, b)
	ctx := context.Background()
	id := uuid.NewString()
	b.items = []BoardItem{{ID: id, CampaignID: "c1", Kind: BoardKindNote, Body: "secret"}}

	tests := []struct {
		name string
		call func() error
	}{
		{"pin", func() error { return svc.PinPage(ctx, "c1", playerViewer, "p1") }},
		{"note", func() error { return svc.AddNote(ctx, "c1", playerViewer, "hello") }},
		{"remove", func() error { return svc.RemoveItem(ctx, "c1", playerViewer, id) }},
		{"picker", func() error { _, err := svc.PickPages(ctx, "c1", playerViewer, ""); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codeOf(tt.call()); got != 403 {
				t.Fatalf("code = %d, want 403", got)
			}
		})
	}
	if len(b.items) != 1 {
		t.Fatalf("a refused player changed the board: %d items", len(b.items))
	}
	// The build refuses a player too, so the tab's contents never reach one.
	if _, err := svc.Build(ctx, "c1", playerViewer); codeOf(err) != 403 {
		t.Fatalf("Build for a player: code %d, want 403", codeOf(err))
	}
}

func TestPinPage(t *testing.T) {
	tests := []struct {
		name      string
		viewer    Viewer
		entityID  string
		preFill   int
		wantCode  int
		wantItems int
	}{
		{"owner pins an adventures page", dmViewer, "p1", 0, 0, 1},
		{"scribe pins an adventures page", scribeViewer, "p1", 0, 0, 1},
		{"a page outside Adventures or hidden is not found", dmViewer, "other", 0, 404, 0},
		{"a blank page id is a bad request", dmViewer, "  ", 0, 400, 0},
		{"a full tab is a conflict", dmViewer, "p1", maxBoardItems, 409, maxBoardItems},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adv := &fakeAdventures{pages: map[string]AdventurePage{"p1": {ID: "p1", Name: "Lastwell"}}}
			b := &memBoard{}
			for i := 0; i < tt.preFill; i++ {
				b.items = append(b.items, BoardItem{ID: uuid.NewString(), CampaignID: "c1", Kind: BoardKindNote, Body: "x"})
			}
			err := newBoardSvc(adv, b).PinPage(context.Background(), "c1", tt.viewer, tt.entityID)
			if got := codeOf(err); got != tt.wantCode {
				t.Fatalf("code = %d (%v), want %d", got, err, tt.wantCode)
			}
			if len(b.items) != tt.wantItems {
				t.Fatalf("items = %d, want %d", len(b.items), tt.wantItems)
			}
			if tt.wantCode == 0 && adv.lastViewer != tt.viewer {
				t.Fatalf("the page was read as %+v, not the pinning viewer %+v", adv.lastViewer, tt.viewer)
			}
		})
	}
}

func TestPinPage_TwiceKeepsOne(t *testing.T) {
	adv := &fakeAdventures{pages: map[string]AdventurePage{"p1": {ID: "p1", Name: "Lastwell"}}}
	b := &memBoard{}
	svc := newBoardSvc(adv, b)
	for i := 0; i < 2; i++ {
		if err := svc.PinPage(context.Background(), "c1", dmViewer, "p1"); err != nil {
			t.Fatalf("pin %d: %v", i, err)
		}
	}
	if len(b.items) != 1 {
		t.Fatalf("items = %d, want 1", len(b.items))
	}
}

func TestAddNote(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		preFill  int
		wantCode int
		wantBody string
	}{
		{"trimmed and stored", "  Ask about the bell  ", 0, 0, "Ask about the bell"},
		{"empty is refused", "", 0, 400, ""},
		{"spaces only is refused", "   \n ", 0, 400, ""},
		{"500 characters fit", strings.Repeat("a", maxBoardNote), 0, 0, strings.Repeat("a", maxBoardNote)},
		{"501 characters do not", strings.Repeat("a", maxBoardNote+1), 0, 400, ""},
		{"length counts characters, not bytes", strings.Repeat("é", maxBoardNote), 0, 0, strings.Repeat("é", maxBoardNote)},
		{"a full tab is a conflict", "one more", maxBoardItems, 409, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &memBoard{}
			for i := 0; i < tt.preFill; i++ {
				b.items = append(b.items, BoardItem{ID: uuid.NewString(), CampaignID: "c1", Kind: BoardKindNote, Body: "x"})
			}
			err := newBoardSvc(&fakeAdventures{}, b).AddNote(context.Background(), "c1", scribeViewer, tt.text)
			if got := codeOf(err); got != tt.wantCode {
				t.Fatalf("code = %d (%v), want %d", got, err, tt.wantCode)
			}
			if tt.wantCode == 0 {
				last := b.items[len(b.items)-1]
				if last.Kind != BoardKindNote || last.Body != tt.wantBody || last.CreatedBy != "sc" {
					t.Fatalf("stored %+v", last)
				}
			}
		})
	}
}

func TestRemoveItem(t *testing.T) {
	mine := uuid.NewString()
	theirs := uuid.NewString()
	tests := []struct {
		name     string
		id       string
		wantCode int
		wantLeft int
	}{
		{"removes the item", mine, 0, 1},
		{"another campaign's item is not found", theirs, 404, 2},
		{"an unknown id is not found", uuid.NewString(), 404, 2},
		{"a malformed id is not found", "../etc", 404, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &memBoard{items: []BoardItem{
				{ID: mine, CampaignID: "c1", Kind: BoardKindNote, Body: "a"},
				{ID: theirs, CampaignID: "c2", Kind: BoardKindNote, Body: "b"},
			}}
			err := newBoardSvc(&fakeAdventures{}, b).RemoveItem(context.Background(), "c1", dmViewer, tt.id)
			if got := codeOf(err); got != tt.wantCode {
				t.Fatalf("code = %d, want %d", got, tt.wantCode)
			}
			if len(b.items) != tt.wantLeft {
				t.Fatalf("items left = %d, want %d", len(b.items), tt.wantLeft)
			}
		})
	}
}

func TestBuild_AdventuresTab(t *testing.T) {
	adv := &fakeAdventures{pages: map[string]AdventurePage{
		"p1": {ID: "p1", Name: "Lastwell", TypeName: "Adventure", Subtitle: "3 · Running"},
	}}
	b := &memBoard{items: []BoardItem{
		{ID: "i1", CampaignID: "c1", Kind: BoardKindPage, EntityID: "p1"},
		{ID: "i2", CampaignID: "c1", Kind: BoardKindNote, Body: "Bring the bell"},
		// Trashed, moved out of Adventures or hidden: the source does not return it.
		{ID: "i3", CampaignID: "c1", Kind: BoardKindPage, EntityID: "gone"},
		{ID: "i4", CampaignID: "c2", Kind: BoardKindNote, Body: "other campaign"},
	}}
	view, err := newBoardSvc(adv, b).Build(context.Background(), "c1", dmViewer)
	if err != nil {
		t.Fatal(err)
	}
	if view.Adventures == nil || len(view.Adventures.Items) != 2 {
		t.Fatalf("items = %+v, want the page and the note only", view.Adventures)
	}
	page, note := view.Adventures.Items[0], view.Adventures.Items[1]
	if page.Kind != BoardKindPage || page.Title != "Lastwell" || page.Kicker != "Adventure" || page.Subtitle != "3 · Running" || page.EntityID != "p1" {
		t.Fatalf("page card = %+v", page)
	}
	if note.Kind != BoardKindNote || note.Text != "Bring the bell" {
		t.Fatalf("note card = %+v", note)
	}
	if len(b.items) != 4 {
		t.Fatal("a pin whose page is out of sight was deleted; restoring the page should bring it back")
	}
	if adv.lastViewer != dmViewer {
		t.Fatalf("pages read as %+v", adv.lastViewer)
	}
}

func TestBuild_AdventuresUnavailable(t *testing.T) {
	// Without a board (the plugin's migration failed) the screen still builds,
	// the tab is left out, and the actions answer not-found.
	svc := NewService(Sources{})
	view, err := svc.Build(context.Background(), "c1", dmViewer)
	if err != nil {
		t.Fatal(err)
	}
	if view.Adventures != nil {
		t.Fatal("the tab should be absent")
	}
	tests := []struct {
		name string
		call func() error
	}{
		{"pin", func() error { return svc.PinPage(context.Background(), "c1", dmViewer, "p1") }},
		{"note", func() error { return svc.AddNote(context.Background(), "c1", dmViewer, "x") }},
		{"remove", func() error { return svc.RemoveItem(context.Background(), "c1", dmViewer, uuid.NewString()) }},
		{"picker", func() error { _, err := svc.PickPages(context.Background(), "c1", dmViewer, ""); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codeOf(tt.call()); got != 404 {
				t.Fatalf("code = %d, want 404", got)
			}
		})
	}
}

type failingBoard struct{ memBoard }

func (failingBoard) List(context.Context, string) ([]BoardItem, error) {
	return nil, errors.New("boom")
}

func TestBuild_BoardFailureLeavesTheRestOfTheScreen(t *testing.T) {
	svc := NewService(Sources{Board: &failingBoard{}, Adventures: &fakeAdventures{}})
	view, err := svc.Build(context.Background(), "c1", dmViewer)
	if err != nil {
		t.Fatalf("a broken board must not break the screen: %v", err)
	}
	if view.Adventures != nil {
		t.Fatal("a failed board should leave the tab out")
	}
}

func TestPickPages(t *testing.T) {
	adv := &fakeAdventures{pages: map[string]AdventurePage{
		"p1": {ID: "p1", Name: "Lastwell", TypeName: "Adventure"},
	}}
	b := &memBoard{items: []BoardItem{{ID: uuid.NewString(), CampaignID: "c1", Kind: BoardKindPage, EntityID: "p1"}}}
	svc := newBoardSvc(adv, b)

	res, err := svc.PickPages(context.Background(), "c1", dmViewer, "  last ")
	if err != nil {
		t.Fatal(err)
	}
	if adv.searchQuery != "last" {
		t.Fatalf("query = %q, want it trimmed", adv.searchQuery)
	}
	if !res.HasCategory || len(res.Pages) != 1 || !res.Pages[0].Pinned {
		t.Fatalf("result = %+v, want one pinned page", res)
	}

	adv.noCategory = true
	res, err = svc.PickPages(context.Background(), "c1", dmViewer, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasCategory || res.Pages == nil || len(res.Pages) != 0 {
		t.Fatalf("no-category result = %+v, want has_category=false and an empty (non-nil) list", res)
	}
}
