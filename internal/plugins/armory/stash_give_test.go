package armory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- fakes ---

// giveDir adds an item catalogue to the shared directory fake.
type giveItems struct{ list []EntityRef }

type giveDir struct {
	*fakeDir
	items *giveItems
}

func (d giveDir) ListItems(context.Context, string, int, string, int) ([]EntityRef, error) {
	return d.items.list, nil
}

// fakeHandouts stands in for the maps and entities plugins. Maps m1 and m2
// belong to the campaign; "foreign" does not.
type fakeHandouts struct {
	maps     map[string]string
	made     []EntityRef
	madeMap  map[string]string // handout id -> map id
	allowed  map[string][]string
	allowCnt int
}

func newFakeHandouts() *fakeHandouts {
	return &fakeHandouts{
		maps:    map[string]string{"m1": "Dungeon", "m2": "Crypt"},
		madeMap: map[string]string{}, allowed: map[string][]string{},
	}
}

func (h *fakeHandouts) ListMaps(context.Context, string) ([]NamedRef, error) {
	return []NamedRef{{ID: "m1", Name: "Dungeon"}, {ID: "m2", Name: "Crypt"}}, nil
}
func (h *fakeHandouts) FindMap(_ context.Context, _, id string) (*NamedRef, error) {
	if n, ok := h.maps[id]; ok {
		return &NamedRef{ID: id, Name: n}, nil
	}
	return nil, nil
}
func (h *fakeHandouts) FindHandout(_ context.Context, _, name, mapID string) (*EntityRef, error) {
	for i := range h.made {
		if h.made[i].Name == name && h.madeMap[h.made[i].ID] == mapID {
			r := h.made[i]
			return &r, nil
		}
	}
	return nil, nil
}
func (h *fakeHandouts) CreateHandout(_ context.Context, _, _, name string, m NamedRef) (*EntityRef, error) {
	r := EntityRef{ID: "h" + m.ID, Name: name, IsItem: true, HandoutMapID: m.ID}
	h.made = append(h.made, r)
	h.madeMap[r.ID] = m.ID
	return &r, nil
}
func (h *fakeHandouts) AllowViewers(_ context.Context, _, id string, users []string) ([]string, error) {
	h.allowCnt++
	var added []string
	for _, u := range users {
		dup := false
		for _, have := range h.allowed[id] {
			dup = dup || have == u
		}
		if !dup {
			h.allowed[id] = append(h.allowed[id], u)
			added = append(added, u)
		}
	}
	return added, nil
}
func (h *fakeHandouts) RevokeViewers(_ context.Context, _, id string, users []string) error {
	drop := map[string]bool{}
	for _, u := range users {
		drop[u] = true
	}
	var keep []string
	for _, u := range h.allowed[id] {
		if !drop[u] {
			keep = append(keep, u)
		}
	}
	h.allowed[id] = keep
	return nil
}

type fakeNotifier struct {
	users                  [][]string
	messages, details, lnk []string
}

func (n *fakeNotifier) ItemGiven(_ context.Context, _ string, users []string, msg, detail, link string) error {
	n.users, n.messages, n.details, n.lnk = append(n.users, users), append(n.messages, msg), append(n.details, detail), append(n.lnk, link)
	return nil
}

type fakeNames map[string]string

func (n fakeNames) DisplayNames(context.Context, string, []string) (map[string]string, error) {
	return n, nil
}

type giveFx struct {
	*fx
	dir    giveDir
	hand   *fakeHandouts
	notify *fakeNotifier
	events *fakeEvents
}

// newGiveFx is the shared stash fixture with give's collaborators wired. gm is
// the Owner. c4 is a character no player has claimed.
func newGiveFx() *giveFx {
	f := newFx()
	f.dir.ents["c4"] = &EntityRef{ID: "c4", Name: "Ghost", IsCharacter: true}
	g := &giveFx{fx: f, hand: newFakeHandouts(), notify: &fakeNotifier{}, events: &fakeEvents{}}
	g.dir = giveDir{f.dir, &giveItems{[]EntityRef{{ID: "i1", Name: "Potion", IsItem: true}}}}
	g.svc = NewStashService(StashDeps{
		Repo: f.repo, Directory: g.dir, Visibility: f.vis, Actor: fakeActor{f.dir},
		Fields: f.fields, Relations: f.rels, Events: g.events, Handouts: g.hand, Notifier: g.notify,
		UserNames: fakeNames{"gm": "Greta", "u1": "Tess", "u2": "Robin"},
	})
	return g
}

var gm = Actor{"gm", rOwner}

// --- tests ---

func TestGive_ItemCreditsHistoryEventAndBell(t *testing.T) {
	g := newGiveFx()
	out, err := g.svc.Give(context.Background(), "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 2})
	if err != nil {
		t.Fatal(err)
	}
	if g.carried("c2", "i1") != 2 {
		t.Fatalf("c2 holds %d", g.carried("c2", "i1"))
	}
	if out.ItemName != "Potion" || out.CharacterName != "Mira" {
		t.Fatalf("outcome %+v", out)
	}
	if len(g.repo.moves) != 1 {
		t.Fatalf("moves %d", len(g.repo.moves))
	}
	m := g.repo.moves[0]
	if !m.IsGive() || m.Status != MoveApplied || m.From != charEP("c2") || m.To != charEP("c2") || m.Quantity != 2 || m.RequestedBy != "gm" {
		t.Fatalf("move %+v", m)
	}
	if got := g.events.types(); len(got) != 1 || got[0] != EventStashMoved {
		t.Fatalf("events %v", got)
	}
	if ids, _ := g.events.got[0].payload["characterIds"].([]string); len(ids) != 1 || ids[0] != "c2" {
		t.Fatalf("event characters %v", g.events.got[0].payload["characterIds"])
	}
	if len(g.notify.users) != 1 || g.notify.users[0][0] != "u2" ||
		g.notify.messages[0] != "Your GM gave you 2 × Potion" || g.notify.details[0] != "On Mira" || g.notify.lnk[0] != "/campaigns/camp/entities/c2" {
		t.Fatalf("notification %v %v %v %v", g.notify.users, g.notify.messages, g.notify.details, g.notify.lnk)
	}
}

func TestGive_AddsToExistingLineKeepingMetadata(t *testing.T) {
	g := newGiveFx()
	if _, err := g.svc.Give(context.Background(), "camp", gm, GiveInput{CharacterID: "c1", ItemID: "i1", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if g.carried("c1", "i1") != 4 || !strings.Contains(string(g.rels.rels["c1"][0].Metadata), `"equipped":true`) {
		t.Fatalf("held %d meta %s", g.carried("c1", "i1"), g.rels.rels["c1"][0].Metadata)
	}
}

func TestGive_Rejections(t *testing.T) {
	tests := []struct {
		name  string
		camp  string
		who   Actor
		in    GiveInput
		want  int
		setup func(*giveFx)
	}{
		{"player", "camp", Actor{"u1", rPlayer}, GiveInput{CharacterID: "c1", ItemID: "i1", Quantity: 1}, http.StatusForbidden, nil},
		{"own character, still a player", "camp", Actor{"u2", rPlayer}, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 1}, http.StatusForbidden, nil},
		{"scribe", "camp", Actor{"sc", rScribe}, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 1}, http.StatusForbidden, nil},
		{"not an item", "camp", gm, GiveInput{CharacterID: "c2", ItemID: "n1", Quantity: 1}, http.StatusNotFound, nil},
		{"unknown item", "camp", gm, GiveInput{CharacterID: "c2", ItemID: "nope", Quantity: 1}, http.StatusNotFound, nil},
		{"not a character", "camp", gm, GiveInput{CharacterID: "n1", ItemID: "i1", Quantity: 1}, http.StatusNotFound, nil},
		{"an item as the character", "camp", gm, GiveInput{CharacterID: "i1", ItemID: "i1", Quantity: 1}, http.StatusNotFound, nil},
		{"other campaign", "other", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 1}, http.StatusNotFound, nil},
		{"map of another campaign", "camp", gm, GiveInput{CharacterID: "c2", MapID: "foreign", Quantity: 1}, http.StatusNotFound, nil},
		{"quantity zero", "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 0}, http.StatusBadRequest, nil},
		{"quantity negative", "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: -3}, http.StatusBadRequest, nil},
		{"quantity too large", "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: maxMoveQuantity + 1}, http.StatusBadRequest, nil},
		{"item and map", "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", MapID: "m1", Quantity: 1}, http.StatusBadRequest, nil},
		{"neither", "camp", gm, GiveInput{CharacterID: "c2", Quantity: 1}, http.StatusBadRequest, nil},
		{"maps not wired", "camp", gm, GiveInput{CharacterID: "c2", MapID: "m1", Quantity: 1}, http.StatusBadRequest,
			func(g *giveFx) {
				g.svc = NewStashService(StashDeps{Repo: g.repo, Directory: g.dir, Visibility: g.vis, Relations: g.rels})
			}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newGiveFx()
			if tc.setup != nil {
				tc.setup(g)
			}
			_, err := g.svc.Give(context.Background(), tc.camp, tc.who, tc.in)
			if code(err) != tc.want {
				t.Fatalf("got %v (code %d), want %d", err, code(err), tc.want)
			}
			if len(g.repo.moves) != 0 || g.carried("c2", "i1") != 0 || len(g.hand.made) != 0 || len(g.events.got) != 0 || len(g.notify.users) != 0 {
				t.Fatal("a refused give changed something")
			}
		})
	}
}

func TestGive_MapCreatesOnceThenReuses(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	give := func(char, mapID string) {
		t.Helper()
		if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: char, MapID: mapID, Quantity: 1}); err != nil {
			t.Fatal(err)
		}
	}
	give("c2", "m1")
	give("c2", "m1")
	give("c1", "m1")
	if len(g.hand.made) != 1 || g.hand.made[0].Name != "Map: Dungeon" {
		t.Fatalf("handouts %+v", g.hand.made)
	}
	if g.carried("c2", "hm1") != 2 || g.carried("c1", "hm1") != 1 {
		t.Fatalf("c2=%d c1=%d", g.carried("c2", "hm1"), g.carried("c1", "hm1"))
	}
	if got := strings.Join(g.hand.allowed["hm1"], ","); got != "u2,u1" {
		t.Fatalf("allow list %q", got)
	}
	give("c2", "m2")
	if len(g.hand.made) != 2 || g.hand.made[1].Name != "Map: Crypt" {
		t.Fatalf("a second map must get its own handout: %+v", g.hand.made)
	}
}

func TestGive_MapToUnclaimedCharacterStaysGMOnly(t *testing.T) {
	g := newGiveFx()
	if _, err := g.svc.Give(context.Background(), "camp", gm, GiveInput{CharacterID: "c4", MapID: "m1", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if g.hand.allowCnt != 0 || g.carried("c4", "hm1") != 1 {
		t.Fatalf("allow calls %d, held %d", g.hand.allowCnt, g.carried("c4", "hm1"))
	}
	if len(g.notify.users) != 0 {
		t.Fatal("nobody to notify")
	}
}

func TestGive_LongMapNameStaysInsideTheEntityNameLimit(t *testing.T) {
	if got := handoutName(strings.Repeat("é", 400)); len([]rune(got)) != len(handoutPrefix)+maxHandoutMapName || len(got) > 400 {
		t.Fatalf("name length %d runes / %d bytes", len([]rune(got)), len(got))
	}
}

func TestGive_HistoryWordingPerViewer(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 2}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		who  Actor
		want string
	}{
		{"the giver", gm, "You gave Mira 2 × Potion"},
		{"another gm", Actor{"sc", rScribe}, "Greta gave Mira 2 × Potion"},
		{"the player", Actor{"u2", rPlayer}, "Your GM gave you 2 × Potion"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lines, err := g.svc.CharacterHistory(ctx, "camp", tc.who, "c2")
			if err != nil || len(lines) != 1 {
				t.Fatalf("lines %d err %v", len(lines), err)
			}
			if got := MoveSummary(lines[0]); got != tc.want {
				t.Fatalf("%q want %q", got, tc.want)
			}
		})
	}
	// With the giver's name unknown a player still gets a sentence.
	if got := giveSummary(MoveLine{Move: Move{Kind: MoveKindItem, Quantity: 1}, ItemName: "Map: Crypt"}, false, true, "u2", ""); got != "Your GM gave you 1 × Map: Crypt" {
		t.Fatalf("%q", got)
	}
}

func TestGive_PanelOffersGiveToOwnerOnly(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	tests := []struct {
		who  Actor
		want bool
	}{{gm, true}, {Actor{"sc", rScribe}, false}, {Actor{"u2", rPlayer}, false}}
	for _, tc := range tests {
		v, err := g.svc.CharacterPanel(ctx, "camp", tc.who, "c2")
		if err != nil || v == nil {
			t.Fatalf("%v %v", v, err)
		}
		if v.CanGive != tc.want {
			t.Errorf("%s: CanGive=%v", tc.who.UserID, v.CanGive)
		}
		var sb strings.Builder
		if err := CharacterPanel(v, "tok").Render(ctx, &sb); err != nil {
			t.Fatal(err)
		}
		if has := strings.Contains(sb.String(), "Give an item"); has != tc.want {
			t.Errorf("%s: button present=%v", tc.who.UserID, has)
		}
	}
}

// A give row is applied the moment it is written. Nothing that settles or
// replays a move may debit and credit it again.
func TestGive_RowIsNeverReapplied(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.svc.Approve(ctx, "camp", gm, g.repo.moves[0].ID); code(err) != http.StatusConflict {
		t.Fatalf("approving an applied give: %v", err)
	}
	// Even forced pending (a crashed write), running it refuses.
	forced := &Move{ID: 77, CampaignID: "camp", Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 2,
		From: charEP("c2"), To: charEP("c2"), Status: MovePending, RequestedBy: "gm"}
	g.repo.moves = append(g.repo.moves, forced)
	m, err := g.svc.Approve(ctx, "camp", gm, 77)
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != MoveFailed || m.Reason != giveReplayReason {
		t.Fatalf("status %q reason %q", m.Status, m.Reason)
	}
	if g.carried("c2", "i1") != 2 {
		t.Fatalf("c2 holds %d after a replay", g.carried("c2", "i1"))
	}
	// A player cannot forge one through Move: from == to is refused.
	_, err = g.svc.Move(ctx, "camp", Actor{"u2", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c2"), To: charEP("c2")})
	if code(err) != http.StatusBadRequest {
		t.Fatalf("self move: %v", err)
	}
}

func TestGive_ReadersHandleTheSelfMove(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c1", ItemID: "i1", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if pend, _ := g.svc.PendingRequests(ctx, "camp", gm); len(pend) != 0 {
		t.Fatalf("a give is not a request: %d", len(pend))
	}
	page, err := g.svc.StashesPage(ctx, "camp", gm)
	if err != nil || len(page.Pending) != 0 {
		t.Fatalf("page %v err %v", page, err)
	}
	// The sync API list shows it with the same sentence as the web.
	api := apiLines([]MoveLine{{Move: *g.repo.moves[0], ItemName: "Potion", ToName: "Thorin", RequesterName: "Greta"}})
	if len(api) != 1 || api[0].Summary != "Greta gave Thorin 1 × Potion" || api[0].From != api[0].To {
		t.Fatalf("api %+v", api)
	}
	// Closing and opening downtime sweeps pending requests only.
	if _, err := g.svc.SetDowntime(ctx, "camp", gm, true); err != nil {
		t.Fatal(err)
	}
	if g.carried("c1", "i1") != 4 {
		t.Fatalf("downtime sweep changed holdings: %d", g.carried("c1", "i1"))
	}
}

func TestGive_HistoryFailureStillReportsTheGive(t *testing.T) {
	g := newGiveFx()
	g.repo.failInsert = true
	if _, err := g.svc.Give(context.Background(), "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 1}); err != nil {
		t.Fatalf("the item was given, so a retry would double it: %v", err)
	}
	if g.carried("c2", "i1") != 1 {
		t.Fatal("item not given")
	}
}

func TestGiveDialog(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	tests := []struct {
		name         string
		who          Actor
		char, item   string
		want         int
		items, chars int
		maps         int
	}{
		{"from a character", gm, "c2", "", 0, 1, 0, 2},
		{"from an item card", gm, "", "i1", 0, 0, 4, 0},
		{"player", Actor{"u2", rPlayer}, "c2", "", http.StatusForbidden, 0, 0, 0},
		{"neither", gm, "", "", http.StatusBadRequest, 0, 0, 0},
		{"not a character", gm, "n1", "", http.StatusNotFound, 0, 0, 0},
		{"not an item", gm, "", "n1", http.StatusNotFound, 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := g.svc.GiveDialog(ctx, "camp", tc.who, tc.char, tc.item, "")
			if code(err) != tc.want {
				t.Fatalf("code %d (%v), want %d", code(err), err, tc.want)
			}
			if err == nil && (len(v.Items) != tc.items || len(v.Characters) != tc.chars || len(v.Maps) != tc.maps) {
				t.Fatalf("items %d characters %d maps %d", len(v.Items), len(v.Characters), len(v.Maps))
			}
		})
	}
}

func TestGiveDialog_Renders(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	// Mira already holds the Dungeon's handout, so its row says so.
	if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c2", MapID: "m1", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	g.dir.ents["hm1"] = &g.hand.made[0]
	for _, r := range g.rels.rels["c2"] {
		if r.ItemEntityID == "hm1" {
			r.ItemName = "Map: Dungeon" // the fake names a line by its id
		}
	}
	g.dir.items.list = append(g.dir.items.list, EntityRef{ID: "z1", Name: "Zephyr Cloak", IsItem: true, Restricted: true})
	fromChar, err := g.svc.GiveDialog(ctx, "camp", gm, "c2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	fromItem, err := g.svc.GiveDialog(ctx, "camp", gm, "", "i1", "")
	if err != nil {
		t.Fatal(err)
	}
	render := func(c interface {
		Render(context.Context, io.Writer) error
	}) string {
		var sb strings.Builder
		if err := c.Render(ctx, &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}
	out := render(GiveBox(fromChar))
	for _, want := range []string{
		`class="ag-box"`, "Give to Mira", "From the Armory", "A map", `data-character="c2"`,
		`data-post="/campaigns/camp/armory/give"`, `data-field="item_id"`, `data-field="map_id"`,
		`placeholder="Search the Armory"`, `data-search="/campaigns/camp/armory/give?character=c2&amp;list=items"`, "Hidden", "Mira has it",
		"Robin gets a notification. It shows on Mira in Foundry too.",
		"Robin, Mira’s player, will be able to see this item’s page. Nobody else will.",
		"Mira gets “Map: Dungeon”. Opening it shows the map. Only players holding it can see its page.",
		"How many", "Not given yet. Give it, or throw the choice away.", "Discard",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("character box missing %q", want)
		}
	}
	if n := strings.Count(out, "has it"); n != 1 {
		t.Errorf("only the held map is tagged, got %d", n)
	}
	out = render(GiveCardBox(fromItem))
	for _, want := range []string{"Give Potion to", `data-item="i1"`, `data-field="character_id"`, "Thorin", "Mira", "Robin", "Tess",
		"It shows on Ghost in Foundry too."} {
		if !strings.Contains(out, want) {
			t.Errorf("card box missing %q", want)
		}
	}
	if strings.Contains(out, `data-field="map_id"`) || strings.Contains(out, "<script") || strings.Contains(out, "Search the Armory") {
		t.Error("card box must have no map tab, no search and no script element")
	}
}

func TestGiveRoutes(t *testing.T) {
	g := newGiveFx()
	h := NewStashHandler(g.svc)
	ctx := func(c echo.Context, role campaigns.Role, uid string) {
		c.Set("auth_user_id", uid)
		c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp"}, MemberRole: role})
	}
	post := func(role campaigns.Role, uid string, form url.Values) (*httptest.ResponseRecorder, error) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		ctx(c, role, uid)
		return rec, h.Give(c)
	}

	t.Run("owner gives", func(t *testing.T) {
		rec, err := post(campaigns.RoleOwner, "gm", url.Values{"character_id": {"c2"}, "item_id": {"i1"}, "quantity": {"3"}})
		if err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("HX-Trigger"), "armory-moved") {
			t.Fatalf("status %d trigger %q", rec.Code, rec.Header().Get("HX-Trigger"))
		}
		if g.carried("c2", "i1") != 3 {
			t.Fatalf("held %d", g.carried("c2", "i1"))
		}
		// The box reads its toast and the line to flash from armory-given,
		// sent as ASCII so the × survives the header.
		if h := rec.Header().Get("HX-Trigger"); strings.ContainsRune(h, '×') || !strings.Contains(h, `\u00d7`) {
			t.Fatalf("header not ASCII-escaped: %s", h)
		}
		var trig map[string]map[string]any
		_ = json.Unmarshal([]byte(rec.Header().Get("HX-Trigger")), &trig)
		if got := trig["armory-given"]; got["message"] != "Gave Mira 3 × Potion" || got["itemId"] != "i1" || got["characterId"] != "c2" {
			t.Fatalf("armory-given %v", trig["armory-given"])
		}
	})
	t.Run("blank quantity is one (the map form has none)", func(t *testing.T) {
		if _, err := post(campaigns.RoleOwner, "gm", url.Values{"character_id": {"c2"}, "map_id": {"m1"}}); err != nil {
			t.Fatal(err)
		}
		if g.carried("c2", "hm1") != 1 {
			t.Fatalf("held %d", g.carried("c2", "hm1"))
		}
	})
	t.Run("player is refused", func(t *testing.T) {
		_, err := post(campaigns.RolePlayer, "u2", url.Values{"character_id": {"c2"}, "item_id": {"i1"}, "quantity": {"1"}})
		if code(err) != http.StatusForbidden {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("junk quantity", func(t *testing.T) {
		_, err := post(campaigns.RoleOwner, "gm", url.Values{"character_id": {"c2"}, "item_id": {"i1"}, "quantity": {"lots"}})
		if code(err) != http.StatusBadRequest {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("dialog", func(t *testing.T) {
		for _, tc := range []struct {
			role campaigns.Role
			uid  string
			q    string
			want int
		}{
			{campaigns.RoleOwner, "gm", "character=c2", http.StatusOK},
			{campaigns.RoleOwner, "gm", "item=i1", http.StatusOK},
			{campaigns.RolePlayer, "u2", "character=c2", http.StatusForbidden},
		} {
			req := httptest.NewRequest(http.MethodGet, "/?"+tc.q, nil)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			ctx(c, tc.role, tc.uid)
			err := h.GiveDialog(c)
			got := rec.Code
			if err != nil {
				got = code(err)
			}
			if got != tc.want {
				t.Errorf("%s as %s: %d want %d (%v)", tc.q, tc.uid, got, tc.want, err)
			}
		}
	})
}

// A give lets the holder's player see the item, so it shows on their panel and
// reads by name in their history.
func TestGive_ItemLetsTheHolderSeeIt(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(g.hand.allowed["i1"], ","); got != "u2" {
		t.Fatalf("allow list %q", got)
	}
	if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c4", ItemID: "i1", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(g.hand.allowed["i1"], ","); got != "u2" {
		t.Fatalf("a character with no player must add nobody: %q", got)
	}
}

func TestGive_QuantityMessage(t *testing.T) {
	g := newGiveFx()
	for _, q := range []int{0, maxMoveQuantity + 1} {
		_, err := g.svc.Give(context.Background(), "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: q})
		var ae *apperror.AppError
		if !errors.As(err, &ae) || !strings.Contains(ae.Message, "Enter a quantity from 1 to 1,000,000.") {
			t.Fatalf("q=%d: %v", q, err)
		}
	}
}

func TestGiveDialog_ItemSearchIsServerSide(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	var items []EntityRef
	for i := 0; i < 120; i++ {
		items = append(items, EntityRef{ID: fmt.Sprintf("x%03d", i), Name: fmt.Sprintf("Sword %03d", i), IsItem: true})
	}
	items = append(items, EntityRef{ID: "z1", Name: "Zephyr Cloak", IsItem: true, Restricted: true})
	g.dir.items.list = items
	tests := []struct {
		name, q    string
		shown, all int
	}{
		{"no query caps at 50", "", 50, 121},
		{"narrowed", "zeph", 1, 1},
		{"narrowed, still more than a page", "sword 0", 50, 100},
		{"nothing", "dragon", 0, 0},
		{"query is trimmed and any case", "  CLOAK ", 1, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := g.svc.GiveDialog(ctx, "camp", gm, "c2", "", tc.q)
			if err != nil {
				t.Fatal(err)
			}
			if len(v.Items) != tc.shown || v.ItemTotal != tc.all {
				t.Fatalf("shown %d of %d", len(v.Items), v.ItemTotal)
			}
			var sb strings.Builder
			if err := GiveItemChoices(v).Render(ctx, &sb); err != nil {
				t.Fatal(err)
			}
			more := strings.Contains(sb.String(), fmt.Sprintf("Showing %d of %d", tc.shown, tc.all))
			if more != (tc.all > tc.shown) {
				t.Fatalf("more-line present=%v", more)
			}
		})
	}
	// Restricted items carry the warning for their row.
	v, _ := g.svc.GiveDialog(ctx, "camp", gm, "c2", "", "zeph")
	var sb strings.Builder
	_ = GiveBox(v).Render(ctx, &sb)
	if !strings.Contains(sb.String(), "Robin, Mira’s player, will be able to see this item’s page.") {
		t.Fatalf("hint missing: %s", sb.String())
	}
	// Nothing matching says so in the list itself.
	v, _ = g.svc.GiveDialog(ctx, "camp", gm, "c2", "", "dragon")
	sb.Reset()
	_ = GiveItemChoices(v).Render(ctx, &sb)
	if !strings.Contains(sb.String(), "Nothing in the Armory matches “dragon”.") {
		t.Fatalf("empty line missing: %s", sb.String())
	}
}

func TestGiveDialog_ItemFlowHintOnlyForPrivateItems(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	g.dir.ents["i1"].Restricted = true
	for id, want := range map[string]bool{"i1": true, "i2": false} {
		v, err := g.svc.GiveDialog(ctx, "camp", gm, "", id, "")
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		_ = GiveCardBox(v).Render(ctx, &sb)
		if has := strings.Contains(sb.String(), "’s player, will be able to see this item’s page"); has != want {
			t.Errorf("%s: hint present=%v", id, has)
		}
	}
}

func TestGive_HistoryWordingForOtherPlayers(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	// c1 is Thorin (u1); u2 may see his history through a stash viewer-less
	// path only as an entity-edit holder, so read the lines directly.
	if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c1", ItemID: "i1", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	// u1 owns c1: recipient wording.
	lines, _ := g.svc.CharacterHistory(ctx, "camp", Actor{"u1", rPlayer}, "c1")
	if got := MoveSummary(lines[0]); got != "Your GM gave you 1 × Potion" {
		t.Fatalf("recipient: %q", got)
	}
	// u2 is a player with edit access to c1 but not its claimant.
	g.svc = NewStashService(StashDeps{Repo: g.repo, Directory: g.dir, Visibility: g.vis, Actor: allowAll{}, Fields: g.fields,
		Relations: g.rels, UserNames: fakeNames{"gm": "Greta"}})
	lines, err := g.svc.CharacterHistory(ctx, "camp", Actor{"u2", rPlayer}, "c1")
	if err != nil || len(lines) != 1 {
		t.Fatalf("%v %v", lines, err)
	}
	if got := MoveSummary(lines[0]); got != "Greta gave Thorin 1 × Potion" {
		t.Fatalf("other player: %q", got)
	}
}

type allowAll struct{}

func (allowAll) CanUserActAsBuyer(context.Context, string, string, string, int) (bool, error) {
	return true, nil
}

func TestGiveRoutes_ItemListFragment(t *testing.T) {
	g := newGiveFx()
	h := NewStashHandler(g.svc)
	req := httptest.NewRequest(http.MethodGet, "/?character=c2&list=items&q=pot", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("auth_user_id", "gm")
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp"}, MemberRole: campaigns.RoleOwner})
	if err := h.GiveDialog(c); err != nil {
		t.Fatal(err)
	}
	out := rec.Body.String()
	if !strings.Contains(out, "Potion") || strings.Contains(out, "ag-box") {
		t.Fatalf("expected the list alone: %s", out)
	}
}

// The card menu's give entry shows for exactly the people the panel button
// does: Owner visibility, which includes a co-DM whose role is only Player.
func TestItemCard_GiveEntryFollowsOwnerVisibility(t *testing.T) {
	tests := []struct {
		name       string
		role       campaigns.Role
		dm         bool
		wantMenu   bool
		wantGiveIn bool
	}{
		{"owner", campaigns.RoleOwner, false, true, true},
		{"co-DM granted, player role", campaigns.RolePlayer, true, true, true},
		{"scribe", campaigns.RoleScribe, false, true, false},
		{"player", campaigns.RolePlayer, false, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp"}, MemberRole: tc.role, IsDmGranted: tc.dm}
			var sb strings.Builder
			if err := ItemCardComponent(cc, &ItemCard{ID: "i1", Name: "Potion"}).Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			if has := strings.Contains(out, "armory-coll-i1"); has != tc.wantMenu {
				t.Errorf("menu present=%v", has)
			}
			if has := strings.Contains(out, "armory/give"); has != tc.wantGiveIn {
				t.Errorf("give url present=%v", has)
			}
		})
	}
}

// The panel marks a map handout with the map icon, and every line carries its
// item id so the Give box can flash the one a give landed on.
func TestCharacterPanel_MapLinesAndLandingIDs(t *testing.T) {
	g := newGiveFx()
	ctx := context.Background()
	if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c2", MapID: "m1", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	g.dir.ents["hm1"] = &g.hand.made[0]
	for _, r := range g.rels.rels["c2"] {
		if r.ItemEntityID == "hm1" {
			r.ItemName = "Map: Dungeon"
		}
	}
	v, err := g.svc.CharacterPanel(ctx, "camp", gm, "c2")
	if err != nil {
		t.Fatal(err)
	}
	maps := 0
	for _, it := range v.Items {
		if it.IsMap {
			maps++
			if it.ItemID != "hm1" {
				t.Errorf("%s marked as a map", it.ItemID)
			}
		}
	}
	if maps != 1 {
		t.Fatalf("map lines %d", maps)
	}
	var sb strings.Builder
	if err := CharacterPanel(v, "tok").Render(ctx, &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{`data-item-id="hm1"`, `data-item-id="i1"`, "fa-map", "data-give-fold", "fa-gift", `aria-expanded="false"`} {
		if !strings.Contains(out, want) {
			t.Errorf("panel missing %q", want)
		}
	}
}
