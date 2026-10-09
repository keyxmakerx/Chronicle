package quests

import (
	"context"
	"strings"
	"testing"
)

const place = "tavern"

var pageHome = PageHome(place)

// catType is a category of the test campaign; otherType belongs to another one.
const (
	catType   = 7
	otherType = 8
)

type boardEnv struct {
	svc   BoardService
	repo  *fakeBoardRepo
	quest *fakeQuestRepo
	ents  *fakeEntities
	maps  *fakeMaps
}

func newBoardEnv() boardEnv {
	e := boardEnv{repo: newFakeBoardRepo(), quest: newFakeQuestRepo(), ents: newFakeEntities()}
	e.ents.add(camp, place, "Tavern", false)
	e.ents.add(camp, "q1", "Goat", false)
	e.ents.add(camp, "q-secret", "Secret quest", true)
	e.ents.add(camp, "page-ok", "Barkeep", false)
	e.ents.add(camp, "page-secret", "Hidden Villain", true)
	e.ents.add(other, "tavern2", "Other tavern", false)
	m := newFakeMaps()
	m.add(camp, "map-1", "Vale")
	m.add(other, "map-x", "Foreign")
	types := fakeTypes{camp: {catType: true}, other: {otherType: true}}
	e.maps = m
	e.svc = NewBoardService(e.repo, e.quest, e.ents, types, m, fakeNames{"dm": "Dana", "pl": "Pat", "scr": "Sam", "pl2": "Pia"}, nil)
	return e
}

func (e boardEnv) board(t *testing.T, who string) string {
	t.Helper()
	b, err := e.svc.CreateBoard(context.Background(), camp, pageHome, dm, "Board "+who, who)
	if err != nil {
		t.Fatal(err)
	}
	return b.ID
}

func TestBoardManagementPermissions(t *testing.T) {
	tests := []struct {
		name string
		v    Viewer
		want int
	}{
		{"dm", dm, 0}, {"scribe", scribe, 403}, {"player", player, 403}, {"guest", guest, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBoardEnv()
			bid := e.board(t, WhoAll)
			ctx := context.Background()
			checks := map[string]error{}
			_, checks["create"] = e.svc.CreateBoard(ctx, camp, pageHome, tt.v, "n", WhoDM)
			_, checks["patch"] = e.svc.PatchBoard(ctx, camp, pageHome, bid, tt.v, BoardPatch{})
			all, _ := e.repo.ListBoards(ctx, camp, pageHome)
			ids := make([]string, len(all))
			for i, b := range all {
				ids[i] = b.ID
			}
			checks["order"] = e.svc.SetOrder(ctx, camp, pageHome, tt.v, ids)
			_, checks["looks"] = e.svc.SetLooks(ctx, camp, pageHome, tt.v, LooksPatch{})
			_, checks["clear"] = e.svc.ClearPlayerItems(ctx, camp, pageHome, bid, tt.v)
			checks["delete"] = e.svc.DeleteBoard(ctx, camp, pageHome, bid, tt.v)
			for op, err := range checks {
				if code(err) != tt.want {
					t.Errorf("%s: got %v want %d", op, err, tt.want)
				}
			}
		})
	}
}

func TestBoardCapsAndValidation(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	for i := 0; i < MaxBoardsPerPage; i++ {
		if _, err := e.svc.CreateBoard(ctx, camp, pageHome, dm, "b", WhoDM); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.CreateBoard(ctx, camp, pageHome, dm, "b", WhoDM); code(err) != 422 {
		t.Fatalf("13th board: %v", err)
	}
	e2 := newBoardEnv()
	for _, tt := range []struct{ name, who string }{{"", WhoDM}, {strings.Repeat("n", 81), WhoDM}, {"ok", "everyone"}} {
		if _, err := e2.svc.CreateBoard(ctx, camp, pageHome, dm, tt.name, tt.who); code(err) != 422 {
			t.Errorf("%q/%q: %v", tt.name, tt.who, err)
		}
	}
	bid := e2.board(t, WhoAll)
	for i := 0; i < MaxItemsPerBoard; i++ {
		if _, err := e2.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindNote, Text: "n"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e2.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindNote, Text: "n"}); code(err) != 422 {
		t.Fatalf("61st item: %v", err)
	}
}

func TestCreateItemMatrix(t *testing.T) {
	tests := []struct {
		name string
		who  string
		v    Viewer
		in   ItemInput
		want int
	}{
		{"all/player note", WhoAll, player, ItemInput{Kind: KindNote, Text: "hi"}, 0},
		{"all/guest note", WhoAll, guest, ItemInput{Kind: KindNote, Text: "hi"}, 403},
		{"scribe board/player", WhoScribe, player, ItemInput{Kind: KindNote, Text: "hi"}, 403},
		{"scribe board/scribe", WhoScribe, scribe, ItemInput{Kind: KindNote, Text: "hi"}, 0},
		{"dm board/scribe", WhoDM, scribe, ItemInput{Kind: KindNote, Text: "hi"}, 403},
		{"dm board/dm", WhoDM, dm, ItemInput{Kind: KindNote, Text: "hi"}, 0},
		{"notice by player", WhoAll, player, ItemInput{Kind: KindNotice, RefID: "q1"}, 403},
		{"notice by scribe on all", WhoAll, scribe, ItemInput{Kind: KindNotice, RefID: "q1"}, 403},
		{"notice by scribe on scribe", WhoScribe, scribe, ItemInput{Kind: KindNotice, RefID: "q1"}, 0},
		{"notice by dm", WhoDM, dm, ItemInput{Kind: KindNotice, RefID: "q1"}, 0},
		{"notice invisible quest", WhoScribe, scribe, ItemInput{Kind: KindNotice, RefID: "q-secret"}, 422},
		{"notice foreign page", WhoDM, dm, ItemInput{Kind: KindNotice, RefID: "tavern2"}, 422},
		{"page ok", WhoAll, player, ItemInput{Kind: KindPage, RefID: "page-ok"}, 0},
		{"page hidden from player", WhoAll, player, ItemInput{Kind: KindPage, RefID: "page-secret"}, 422},
		{"page hidden ok for dm", WhoAll, dm, ItemInput{Kind: KindPage, RefID: "page-secret"}, 0},
		{"map ok", WhoAll, player, ItemInput{Kind: KindMap, RefID: "map-1"}, 0},
		{"map foreign", WhoAll, player, ItemInput{Kind: KindMap, RefID: "map-x"}, 422},
		{"note empty", WhoAll, player, ItemInput{Kind: KindNote, Text: "  "}, 422},
		{"note too long", WhoAll, player, ItemInput{Kind: KindNote, Text: strings.Repeat("x", 401)}, 422},
		{"unknown kind", WhoAll, player, ItemInput{Kind: "gif"}, 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBoardEnv()
			bid := e.board(t, tt.who)
			iv, err := e.svc.CreateItem(context.Background(), camp, pageHome, bid, tt.v, tt.in)
			if code(err) != tt.want {
				t.Fatalf("got %v want %d", err, tt.want)
			}
			if err == nil && iv.ByDM != tt.v.IsDM {
				t.Fatalf("byDm=%v for %+v", iv.ByDM, tt.v)
			}
		})
	}
}

func TestPatchDeleteAuthority(t *testing.T) {
	tests := []struct {
		name  string
		who   string
		actor Viewer
		want  int
	}{
		{"owner on all board", WhoAll, player, 0},
		{"other player", WhoAll, Viewer{UserID: "pl2", MemberRole: 1, VisibilityRole: 1}, 403},
		{"scribe not manager on all board", WhoAll, scribe, 403},
		{"dm manager", WhoAll, dm, 0},
		{"scribe manager on scribe board", WhoScribe, scribe, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBoardEnv()
			bid := e.board(t, tt.who)
			ctx := context.Background()
			// The pin is the player's (board opened to all, then switched where needed).
			mk := player
			if tt.who == WhoScribe {
				mk = scribe
				mk.UserID = "someone-else"
			}
			iv, err := e.svc.CreateItem(ctx, camp, pageHome, bid, mk, ItemInput{Kind: KindNote, Text: "n"})
			if err != nil {
				t.Fatal(err)
			}
			x := 50.0
			_, err = e.svc.PatchItem(ctx, camp, pageHome, bid, iv.ID, tt.actor, parseItemPatch(t, `{"x":50}`))
			_ = x
			if code(err) != tt.want {
				t.Fatalf("patch got %v want %d", err, tt.want)
			}
			if code(e.svc.DeleteItem(ctx, camp, pageHome, bid, iv.ID, tt.actor)) != tt.want {
				t.Fatalf("delete want %d", tt.want)
			}
		})
	}
}

func TestPatchRules(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	bid := e.board(t, WhoAll)
	iv, _ := e.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindNote, Text: "orig", X: 10, Y: 20})
	// Player cannot hide; absent keys preserve.
	if _, err := e.svc.PatchItem(ctx, camp, pageHome, bid, iv.ID, player, parseItemPatch(t, `{"hidden":true}`)); code(err) != 403 {
		t.Fatalf("player hide: %v", err)
	}
	got, err := e.svc.PatchItem(ctx, camp, pageHome, bid, iv.ID, player, parseItemPatch(t, `{"x":200}`))
	if err != nil || got.X != 100 || got.Y != 20 || got.Text != "orig" {
		t.Fatalf("partial patch: %v %+v", err, got)
	}
	// DM hides it: it vanishes for the owner, who gets 404.
	if _, err := e.svc.PatchItem(ctx, camp, pageHome, bid, iv.ID, dm, parseItemPatch(t, `{"hidden":true}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PatchItem(ctx, camp, pageHome, bid, iv.ID, player, parseItemPatch(t, `{"x":1}`)); code(err) != 404 {
		t.Fatalf("hidden item for owner: %v", err)
	}
}

func TestBoardIDOR(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	bid := e.board(t, WhoAll)
	iv, _ := e.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindNote, Text: "n"})
	// A second board on another place page, and another campaign's view of ours.
	other2, _ := e.svc.CreateBoard(ctx, other, PageHome("tavern2"), dm, "x", WhoAll)
	if other2 == nil {
		t.Fatal("setup")
	}
	checks := map[string]error{}
	_, checks["create item, foreign board"] = e.svc.CreateItem(ctx, camp, pageHome, other2.ID, dm, ItemInput{Kind: KindNote, Text: "n"})
	_, checks["patch item, wrong board"] = e.svc.PatchItem(ctx, camp, pageHome, other2.ID, iv.ID, dm, parseItemPatch(t, `{"x":1}`))
	checks["delete item, wrong board"] = e.svc.DeleteItem(ctx, camp, pageHome, other2.ID, iv.ID, dm)
	checks["delete foreign board"] = e.svc.DeleteBoard(ctx, camp, pageHome, other2.ID, dm)
	_, checks["patch foreign board"] = e.svc.PatchBoard(ctx, camp, pageHome, other2.ID, dm, BoardPatch{})
	_, checks["clear foreign board"] = e.svc.ClearPlayerItems(ctx, camp, pageHome, other2.ID, dm)
	_, checks["board of another campaign via our id"] = e.svc.CreateItem(ctx, other, PageHome("tavern2"), bid, dm, ItemInput{Kind: KindNote, Text: "n"})
	_, checks["view foreign page"] = e.svc.View(ctx, camp, PageHome("tavern2"), dm)
	for name, err := range checks {
		if code(err) != 404 {
			t.Errorf("%s: got %v want 404", name, err)
		}
	}
	if n, _ := e.repo.CountItems(ctx, camp, bid); n != 1 {
		t.Fatalf("item must be untouched, count=%d", n)
	}
}

func TestViewFiltersForPlayers(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	bid := e.board(t, WhoAll)
	mustItem := func(v Viewer, in ItemInput) *ItemView {
		t.Helper()
		iv, err := e.svc.CreateItem(ctx, camp, pageHome, bid, v, in)
		if err != nil {
			t.Fatal(err)
		}
		return iv
	}
	notice := mustItem(dm, ItemInput{Kind: KindNotice, RefID: "q1"})
	secretNotice := mustItem(dm, ItemInput{Kind: KindNotice, RefID: "q-secret"}) // DM can pin a page players cannot open
	secretPage := mustItem(dm, ItemInput{Kind: KindPage, RefID: "page-secret"})
	okPage := mustItem(player, ItemInput{Kind: KindPage, RefID: "page-ok"})
	hiddenNote := mustItem(dm, ItemInput{Kind: KindNote, Text: "dm eyes"})
	if _, err := e.svc.PatchItem(ctx, camp, pageHome, bid, hiddenNote.ID, dm, parseItemPatch(t, `{"hidden":true}`)); err != nil {
		t.Fatal(err)
	}
	mapItem := mustItem(player, ItemInput{Kind: KindMap, RefID: "map-1"})
	str := mustItem(player, ItemInput{Kind: KindString, From: okPage.ID, To: mapItem.ID})
	strToSecret := mustItem(dm, ItemInput{Kind: KindString, From: notice.ID, To: secretNotice.ID})
	strToHidden := mustItem(dm, ItemInput{Kind: KindString, From: notice.ID, To: hiddenNote.ID})

	// A player cannot string to something they cannot see.
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindString, From: okPage.ID, To: secretNotice.ID}); code(err) != 422 {
		t.Fatalf("string to invisible item: %v", err)
	}

	pv, err := e.svc.View(ctx, camp, pageHome, player)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]ItemView{}
	for _, it := range pv.Boards[0].Items {
		have[it.ID] = it
	}
	for _, id := range []string{secretNotice.ID, hiddenNote.ID, strToSecret.ID, strToHidden.ID} {
		if _, ok := have[id]; ok {
			t.Errorf("item %s must be omitted for a player", id)
		}
	}
	for _, id := range []string{notice.ID, okPage.ID, mapItem.ID, str.ID, secretPage.ID} {
		if _, ok := have[id]; !ok {
			t.Errorf("item %s missing for player", id)
		}
	}
	sp := have[secretPage.ID]
	if !sp.Concealed || sp.Name != "" || sp.EntityID != "" || sp.URL != "" || sp.ImagePath != "" {
		t.Errorf("concealed page leaks: %+v", sp)
	}
	if n := have[notice.ID]; !n.HasSheet || n.Title != "Goat" || n.Kicker != "Help wanted" || n.Status != StatusNotStarted || n.QuestID != "q1" {
		t.Errorf("notice item: %+v", n)
	}
	if p := have[okPage.ID]; p.Name != "Barkeep" || p.URL != "/campaigns/camp-1/entities/page-ok" || !p.Mine || p.OwnerName != "Pat" || p.OwnerInitial != "P" || p.ByDM {
		t.Errorf("page item: %+v", p)
	}
	if m := have[mapItem.ID]; m.URL != "/campaigns/camp-1/maps/map-1" || m.Name != "Vale" {
		t.Errorf("map item: %+v", m)
	}
	if pv.CanManage || pv.Me.IsDM || pv.Me.Role != 1 || pv.Me.UserID != "pl" || !pv.Boards[0].CanChange {
		t.Errorf("player envelope: %+v", pv)
	}
	for _, it := range pv.Boards[0].Items {
		if it.Hidden != nil {
			t.Errorf("hidden flag leaked to player on %s", it.ID)
		}
	}

	dv, _ := e.svc.View(ctx, camp, pageHome, dm)
	if len(dv.Boards[0].Items) != 9 || !dv.CanManage {
		t.Fatalf("dm sees everything: %d", len(dv.Boards[0].Items))
	}
	for _, it := range dv.Boards[0].Items {
		if it.Hidden == nil {
			t.Errorf("dm item %s lacks hidden flag", it.ID)
		}
		if it.ID == secretPage.ID && (it.Concealed || it.Name == "") {
			t.Errorf("dm must see the real page: %+v", it)
		}
	}
	gv, err := e.svc.View(ctx, camp, pageHome, guest)
	if err != nil || gv.Boards[0].CanChange || len(gv.Boards[0].Items) != len(pv.Boards[0].Items) {
		t.Errorf("guest view: %v %+v", err, gv)
	}
	// A hidden place page is a 404 for players.
	e.ents.hidden[place] = true
	if _, err := e.svc.View(ctx, camp, pageHome, player); code(err) != 404 {
		t.Errorf("hidden place: %v", err)
	}
}

func TestDeleteItemRemovesStringsAndClearPlayerItems(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	bid := e.board(t, WhoAll)
	a, _ := e.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindNote, Text: "a"})
	b, _ := e.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindNote, Text: "b"})
	dmNote, _ := e.svc.CreateItem(ctx, camp, pageHome, bid, dm, ItemInput{Kind: KindNote, Text: "dm"})
	notice, _ := e.svc.CreateItem(ctx, camp, pageHome, bid, dm, ItemInput{Kind: KindNotice, RefID: "q1"})
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindString, From: a.ID, To: b.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, dm, ItemInput{Kind: KindString, From: dmNote.ID, To: a.ID}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteItem(ctx, camp, pageHome, bid, b.ID, player); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.repo.CountItems(ctx, camp, bid); n != 4 { // a, dmNote, notice, dm string
		t.Fatalf("string tied to deleted item must go, count=%d", n)
	}
	removed, err := e.svc.ClearPlayerItems(ctx, camp, pageHome, bid, dm)
	if err != nil || removed != 1 {
		t.Fatalf("clear: %d %v", removed, err)
	}
	left, _ := e.repo.ListItems(ctx, camp, []string{bid})
	ids := map[string]bool{}
	for _, it := range left {
		ids[it.ID] = true
	}
	if !ids[dmNote.ID] || !ids[notice.ID] || ids[a.ID] || len(left) != 2 {
		t.Fatalf("clear must keep DM items and notices, drop the dangling DM string: %+v", left)
	}
}

func TestOrderAndLooks(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	a, b := e.board(t, WhoDM), e.board(t, WhoAll)
	for _, ids := range [][]string{{a}, {a, a}, {a, "nope"}, {a, b, "x"}} {
		if err := e.svc.SetOrder(ctx, camp, pageHome, dm, ids); code(err) != 422 {
			t.Errorf("%v: %v", ids, err)
		}
	}
	if err := e.svc.SetOrder(ctx, camp, pageHome, dm, []string{b, a}); err != nil {
		t.Fatal(err)
	}
	v, _ := e.svc.View(ctx, camp, pageHome, dm)
	if v.Boards[0].ID != b {
		t.Fatal("order not applied")
	}
	if _, err := e.svc.SetLooks(ctx, camp, pageHome, dm, parseLooksPatch(t, `{"board":"neon"}`)); code(err) != 422 {
		t.Fatal("bad look accepted")
	}
	l, err := e.svc.SetLooks(ctx, camp, pageHome, dm, parseLooksPatch(t, `{"ledger":"midnight"}`))
	if err != nil || l.Board != LookLit || l.Ledger != LookMidnight {
		t.Fatalf("looks: %v %+v", err, l)
	}
}

func TestPicker(t *testing.T) {
	e := newBoardEnv()
	p := NewPickerService(e.ents, func() MapDirectory {
		m := newFakeMaps()
		m.add(camp, "map-1", "Vale")
		m.add(other, "mx", "Valley")
		return m
	}(), fakeCharacters{
		{ID: "c1", Name: "Mira", Player: "Sam"},
		{ID: "c2", Name: "Bram", Player: "Ana"},
		{ID: "npc", Name: "Barkeep"},
	})
	ctx := context.Background()
	tests := []struct {
		name, kind, q string
		v             Viewer
		want          int
		wantCode      int
	}{
		{"pages visible only", "page", "e", player, 4, 0}, // Tavern, Goat, Barkeep, ...
		{"hidden filtered", "page", "villain", player, 0, 0},
		{"dm sees hidden", "page", "villain", dm, 1, 0},
		{"short query empty", "quest", "a", player, 0, 0},
		{"maps by name", "map", "val", player, 1, 0},
		{"bad kind", "x", "ab", player, 0, 422},
		{"guest refused", "page", "ab", guest, 0, 403},
		{"party for dm, unclaimed left out", "character", "", dm, 2, 0},
		{"party filtered by name", "character", "mi", dm, 1, 0},
		{"party refused to scribe", "character", "", scribe, 0, 403},
		{"party refused to player", "character", "", player, 0, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Search(ctx, camp, tt.v, tt.kind, tt.q)
			if code(err) != tt.wantCode {
				t.Fatalf("err %v", err)
			}
			if err == nil && tt.name != "pages visible only" && len(got) != tt.want {
				t.Fatalf("got %d want %d: %+v", len(got), tt.want, got)
			}
			for _, it := range got {
				if it.ID == "page-secret" && tt.v.VisibilityRole < 3 || it.ID == "tavern2" {
					t.Fatalf("leaked %s", it.ID)
				}
			}
		})
	}
}

func TestViewOmitsNoticeHiddenOnQuestPage(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	bid := e.board(t, WhoAll)
	n, err := e.svc.CreateItem(ctx, camp, pageHome, bid, dm, ItemInput{Kind: KindNotice, RefID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	e.quest.docs["q1"] = []byte(`{"notice":{"title":"Goat"},"layout":{"notice":{"hidden":true}}}`)
	e.quest.camp["q1"] = camp

	cases := []struct {
		name string
		v    Viewer
		want bool
	}{
		{"player", player, false},
		{"scribe", scribe, false},
		{"dm", dm, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view, err := e.svc.View(ctx, camp, pageHome, tc.v)
			if err != nil {
				t.Fatal(err)
			}
			got := false
			for _, it := range view.Boards[0].Items {
				if it.ID == n.ID {
					got = true
				}
			}
			if got != tc.want {
				t.Fatalf("notice shown=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestViewHidesRewardWithItsTag(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	bid := e.board(t, WhoAll)
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, dm, ItemInput{Kind: KindNotice, RefID: "q1"}); err != nil {
		t.Fatal(err)
	}
	e.quest.docs["q1"] = []byte(`{"notice":{"title":"Goat","reward":"500 gp"},"layout":{"tag":{"hidden":true}}}`)
	e.quest.camp["q1"] = camp
	for _, tc := range []struct {
		name string
		v    Viewer
		want string
	}{{"player", player, ""}, {"dm", dm, "500 gp"}} {
		view, err := e.svc.View(ctx, camp, pageHome, tc.v)
		if err != nil {
			t.Fatal(err)
		}
		if got := view.Boards[0].Items[0].Reward; got != tc.want {
			t.Fatalf("%s: reward %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestViewGuestSeesNoMemberNames(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	bid := e.board(t, WhoAll)
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindNote, Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	view, err := e.svc.View(ctx, camp, pageHome, guest)
	if err != nil || len(view.Boards) == 0 || len(view.Boards[0].Items) == 0 {
		t.Fatalf("guest should still see the pin: %v", err)
	}
	for _, it := range view.Boards[0].Items {
		if it.OwnerName != "" || it.OwnerInitial != "" {
			t.Fatalf("guest sees who pinned: %+v", it)
		}
	}
}

// fakeCharacters is a fixed CharacterDirectory.
type fakeCharacters []CharacterInfo

func (f fakeCharacters) ListCharacters(context.Context, string, int, string) ([]CharacterInfo, error) {
	return f, nil
}

func TestPickerCharactersWithNoneClaimed(t *testing.T) {
	e := newBoardEnv()
	p := NewPickerService(e.ents, newFakeMaps(), fakeCharacters{{ID: "c1", Name: "Mira"}, {ID: "c2", Name: "Bram"}})
	got, err := p.Search(context.Background(), camp, dm, PickCharacter, "")
	if err != nil || len(got) != 2 {
		t.Fatalf("want every character when none is claimed: %v %+v", err, got)
	}
}

func TestTypeHomeRefusesForeignOrMissingCategory(t *testing.T) {
	tests := []struct {
		name     string
		campaign string
		home     Home
		want     int
	}{
		{"own category", camp, TypeHome(catType), 0},
		{"another campaign's category", camp, TypeHome(otherType), 404},
		{"unknown category", camp, TypeHome(999), 404},
		{"zero value is no home", camp, Home{}, 404},
		{"both set", camp, Home{EntityID: place, TypeID: catType}, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBoardEnv()
			ctx := context.Background()
			_, err := e.svc.View(ctx, tt.campaign, tt.home, dm)
			if code(err) != tt.want {
				t.Errorf("View: got %v want %d", err, tt.want)
			}
			_, err = e.svc.CreateBoard(ctx, tt.campaign, tt.home, dm, "x", WhoAll)
			if code(err) != tt.want {
				t.Errorf("CreateBoard: got %v want %d", err, tt.want)
			}
			if tt.want == 404 && len(e.repo.boards) != 0 {
				t.Errorf("a refused home must write nothing, got %d boards", len(e.repo.boards))
			}
		})
	}
}

func TestTypeHomeViewAndChangeRules(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	home := TypeHome(catType)
	mk := func(who string) string {
		b, err := e.svc.CreateBoard(ctx, camp, home, dm, "B "+who, who)
		if err != nil {
			t.Fatal(err)
		}
		return b.ID
	}
	all, scr, dmOnly := mk(WhoAll), mk(WhoScribe), mk(WhoDM)

	// A category page is visible to everyone with campaign view access, so
	// every viewer reads the boards, but only DM sees the hidden-item flag.
	tests := []struct {
		name       string
		v          Viewer
		canChange  map[string]bool
		canManage  bool
		wantHidden bool
	}{
		{"dm", dm, map[string]bool{all: true, scr: true, dmOnly: true}, true, true},
		{"scribe", scribe, map[string]bool{all: true, scr: true, dmOnly: false}, false, false},
		{"player", player, map[string]bool{all: true, scr: false, dmOnly: false}, false, false},
		{"guest", guest, map[string]bool{all: false, scr: false, dmOnly: false}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := e.svc.View(ctx, camp, home, tt.v)
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Boards) != 3 || out.CanManage != tt.canManage {
				t.Fatalf("boards=%d canManage=%v", len(out.Boards), out.CanManage)
			}
			for _, b := range out.Boards {
				if b.CanChange != tt.canChange[b.ID] {
					t.Errorf("board %s canChange=%v want %v", b.Name, b.CanChange, tt.canChange[b.ID])
				}
			}
		})
	}

	// Pinning follows the same per-board rule as a page home.
	if _, err := e.svc.CreateItem(ctx, camp, home, all, player, ItemInput{Kind: KindNote, Text: "hi"}); err != nil {
		t.Errorf("player on all board: %v", err)
	}
	if _, err := e.svc.CreateItem(ctx, camp, home, dmOnly, player, ItemInput{Kind: KindNote, Text: "hi"}); code(err) != 403 {
		t.Errorf("player on dm board: got %v want 403", err)
	}
	if _, err := e.svc.CreateItem(ctx, camp, home, all, guest, ItemInput{Kind: KindNote, Text: "hi"}); code(err) != 403 {
		t.Errorf("guest: got %v want 403", err)
	}
	if _, err := e.svc.CreateBoard(ctx, camp, home, player, "x", WhoAll); code(err) != 403 {
		t.Errorf("player creating a board: got %v want 403", err)
	}
}

func TestTypeHomeIsolatedFromPageHome(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	bid := e.board(t, WhoAll) // on the page
	tb, err := e.svc.CreateBoard(ctx, camp, TypeHome(catType), dm, "Cat", WhoAll)
	if err != nil {
		t.Fatal(err)
	}
	// A board id is only reachable through its own home.
	checks := map[string]error{}
	_, checks["page board via category"] = e.svc.CreateItem(ctx, camp, TypeHome(catType), bid, dm, ItemInput{Kind: KindNote, Text: "n"})
	_, checks["category board via page"] = e.svc.CreateItem(ctx, camp, pageHome, tb.ID, dm, ItemInput{Kind: KindNote, Text: "n"})
	checks["delete category board via page"] = e.svc.DeleteBoard(ctx, camp, pageHome, tb.ID, dm)
	for name, err := range checks {
		if code(err) != 404 {
			t.Errorf("%s: got %v want 404", name, err)
		}
	}
	// Looks are per home too.
	if _, err := e.svc.SetLooks(ctx, camp, TypeHome(catType), dm, parseLooksPatch(t, `{"board":"midnight"}`)); err != nil {
		t.Fatal(err)
	}
	if l, _ := e.repo.GetLooks(ctx, camp, pageHome); l.Board != LookLit {
		t.Errorf("page looks changed by a category write: %+v", l)
	}
}

func TestTypeHomeGetsNoPageVisibilityCheck(t *testing.T) {
	// A page home 404s for a player when the page is hidden; a category has no
	// page to hide, but concealed pins on it still hide unviewable pages.
	e := newBoardEnv()
	ctx := context.Background()
	home := TypeHome(catType)
	b, err := e.svc.CreateBoard(ctx, camp, home, dm, "Cat", WhoAll)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateItem(ctx, camp, home, b.ID, dm, ItemInput{Kind: KindPage, RefID: "page-secret"}); err != nil {
		t.Fatal(err)
	}
	out, err := e.svc.View(ctx, camp, home, player)
	if err != nil {
		t.Fatal(err)
	}
	it := out.Boards[0].Items[0]
	if !it.Concealed || it.Name != "" || it.EntityID != "" {
		t.Errorf("hidden page leaked on a category board: %+v", it)
	}
}

// With the maps addon off, boards offer no maps, refuse a map pin, and drop
// map pins made while it was on (their route is behind the addon).
func TestBoardsMapsAddonOff(t *testing.T) {
	ctx := context.Background()
	e := newBoardEnv()
	bid := e.board(t, WhoAll)
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindMap, RefID: "map-1"}); err != nil {
		t.Fatal(err)
	}
	on, err := e.svc.View(ctx, camp, pageHome, player)
	if err != nil || !on.MapsOn || len(on.Boards[0].Items) != 1 {
		t.Fatalf("addon on: %+v %v", on, err)
	}
	e.maps.off = true
	off, err := e.svc.View(ctx, camp, pageHome, player)
	if err != nil {
		t.Fatal(err)
	}
	if off.MapsOn || len(off.Boards[0].Items) != 0 {
		t.Fatalf("addon off: mapsOn=%v items=%+v", off.MapsOn, off.Boards[0].Items)
	}
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, dm, ItemInput{Kind: KindMap, RefID: "map-1"}); code(err) != 422 {
		t.Fatalf("map pin with addon off: %v", err)
	}
}
