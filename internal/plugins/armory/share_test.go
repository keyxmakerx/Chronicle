package armory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeShares is an in-memory ShareStore.
type fakeShares struct {
	rows []ItemShare
	// failSetMade makes SetMadeGrant fail, like a dropped connection.
	failSetMade bool
}

func (f *fakeShares) ListByCharacter(_ context.Context, _, cid string) ([]ItemShare, error) {
	var out []ItemShare
	for _, r := range f.rows {
		if r.CharacterID == cid {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeShares) ListByItem(_ context.Context, _, item string) ([]ItemShare, error) {
	var out []ItemShare
	for _, r := range f.rows {
		if r.ItemID == item {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeShares) Add(_ context.Context, s ItemShare) error {
	for _, r := range f.rows {
		if r.CharacterID == s.CharacterID && r.ItemID == s.ItemID && r.UserID == s.UserID {
			return nil
		}
	}
	f.rows = append(f.rows, s)
	return nil
}
func (f *fakeShares) Remove(_ context.Context, _, cid, item, user string) error {
	var keep []ItemShare
	for _, r := range f.rows {
		if r.CharacterID != cid || r.ItemID != item || r.UserID != user {
			keep = append(keep, r)
		}
	}
	f.rows = keep
	return nil
}
func (f *fakeShares) SetMadeGrant(_ context.Context, _, cid, item, user string) error {
	if f.failSetMade {
		return errors.New("db down")
	}
	for i := range f.rows {
		if f.rows[i].CharacterID == cid && f.rows[i].ItemID == item && f.rows[i].UserID == user {
			f.rows[i].MadeGrant = true
		}
	}
	return nil
}

type fakeAudit struct{ events []map[string]any }

func (a *fakeAudit) LogEvent(_ context.Context, _, _, action string, d map[string]any) error {
	d["action"] = action
	a.events = append(a.events, d)
	return nil
}

type shareFx struct {
	*giveFx
	shares *fakeShares
	audit  *fakeAudit
}

// newShareFx: Mira (c2, Robin/u2) holds z1, a hidden cloak only she and the
// GM can see, and i1, a potion everyone can see. Thorin (c1) is Tess's (u1),
// Grub (c3) is Lee's (u3), Ghost (c4) has no player.
func newShareFx() *shareFx {
	g := newGiveFx()
	f := &shareFx{giveFx: g, shares: &fakeShares{}, audit: &fakeAudit{}}
	g.dir.ents["z1"] = &EntityRef{ID: "z1", Name: "Zephyr Cloak", IsItem: true, Restricted: true}
	g.dir.ents["s1"] = &EntityRef{ID: "s1", Name: "Sealed letter", IsItem: true, Restricted: true}
	g.hand.allowed["z1"] = []string{"u2"}
	g.rels.next++
	g.rels.rels["c2"] = append(g.rels.rels["c2"],
		&HasItemRelation{ID: g.rels.next, ItemEntityID: "z1", ItemName: "Zephyr Cloak", Metadata: []byte(`{"quantity":1}`)},
		&HasItemRelation{ID: g.rels.next + 1, ItemEntityID: "i1", ItemName: "Potion", Metadata: []byte(`{"quantity":2}`)},
		&HasItemRelation{ID: g.rels.next + 2, ItemEntityID: "s1", ItemName: "Sealed letter", Metadata: []byte(`{"quantity":1}`), DmOnly: true},
	)
	g.rels.next += 2
	f.rebuild()
	return f
}

func (f *shareFx) rebuild() {
	f.svc = NewStashService(StashDeps{
		Repo: f.repo, Directory: f.dir, Visibility: f.vis, Actor: fakeActor{f.dir.fakeDir},
		Fields: f.fields, Relations: f.rels, Events: f.events, Handouts: f.hand, Notifier: f.notify,
		UserNames: fakeNames{"gm": "Greta", "u1": "Tess", "u2": "Robin", "u3": "Lee"},
		Shares:    f.shares, Auditor: f.audit,
	})
}

func (f *shareFx) allowed(item string) string {
	a := append([]string(nil), f.hand.allowed[item]...)
	sort.Strings(a)
	return strings.Join(a, ",")
}

var robin = Actor{"u2", rPlayer}

// Who may open the Share box, for what: only the holder's player or Owner
// visibility, only for a hidden item the character holds and the actor can
// see.
func TestShareBox_Authorization(t *testing.T) {
	tests := []struct {
		name       string
		who        Actor
		char, item string
		setup      func(*shareFx)
		want       int
	}{
		{"holder's player", robin, "c2", "z1", nil, 0},
		{"owner", gm, "c2", "z1", nil, 0},
		{"scribe is not the holder", Actor{"sc", rScribe}, "c2", "z1", nil, http.StatusForbidden},
		{"another player", Actor{"u1", rPlayer}, "c2", "z1", nil, http.StatusForbidden},
		{"player on someone else's character", robin, "c1", "i1", nil, http.StatusForbidden},
		{"an item the character does not hold", robin, "c2", "i2", nil, http.StatusNotFound},
		{"a DM-only line the player cannot see", robin, "c2", "s1", nil, http.StatusNotFound},
		{"an item everyone can already see", robin, "c2", "i1", nil, http.StatusBadRequest},
		{"an item the player cannot see", robin, "c2", "z1", func(f *shareFx) { f.vis.hidden["z1"] = true }, http.StatusNotFound},
		{"not an item", robin, "c2", "n1", nil, http.StatusNotFound},
		{"not a character", robin, "n1", "z1", nil, http.StatusNotFound},
		{"another campaign's character", robin, "zz", "z1", nil, http.StatusNotFound},
		{"sharing switched off", robin, "c2", "z1", func(f *shareFx) {
			f.shares = nil
			f.svc = NewStashService(StashDeps{Repo: f.repo, Directory: f.dir, Visibility: f.vis, Relations: f.rels, Handouts: f.hand})
		}, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newShareFx()
			if tc.setup != nil {
				tc.setup(f)
			}
			_, err := f.svc.ShareBox(context.Background(), "camp", tc.who, tc.char, tc.item)
			if code(err) != tc.want {
				t.Fatalf("code %d (%v), want %d", code(err), err, tc.want)
			}
			_, err = f.svc.Share(context.Background(), "camp", tc.who, tc.char, tc.item, []string{"u1"})
			if code(err) != tc.want {
				t.Fatalf("save: code %d (%v), want %d", code(err), err, tc.want)
			}
			if tc.want != 0 && f.allowed("z1") != "u2" {
				t.Fatalf("a refused share changed the allow list: %s", f.allowed("z1"))
			}
		})
	}
}

// Only the campaign's other players can be offered or chosen: never the
// holder's own player, an unclaimed character, or a stranger.
func TestShare_WhoCanBeChosen(t *testing.T) {
	ctx := context.Background()
	f := newShareFx()
	v, err := f.svc.ShareBox(ctx, "camp", robin, "c2", "z1")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range v.Party {
		got = append(got, m.Character+"/"+m.Player+"/"+m.UserID)
	}
	if strings.Join(got, ",") != "Grub/Lee/u3,Thorin/Tess/u1" {
		t.Fatalf("party %v", got)
	}
	tests := []struct {
		name  string
		users []string
		want  int
	}{
		{"a stranger", []string{"zz"}, http.StatusBadRequest},
		{"the holder's own player", []string{"u2"}, http.StatusBadRequest},
		{"the GM", []string{"gm"}, http.StatusBadRequest},
		{"blank ids are ignored", []string{"", " "}, 0},
		{"other players", []string{"u1", "u3", "u1"}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", tc.users); code(err) != tc.want {
				t.Fatalf("code %d (%v), want %d", code(err), err, tc.want)
			}
		})
	}
	if f.allowed("z1") != "u1,u2,u3" {
		t.Fatalf("allow list %s", f.allowed("z1"))
	}
}

// Taking a share back removes only the grants sharing added.
func TestShare_TakingBackRemovesOnlyWhatSharingAdded(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		// before runs ahead of the first share; between, after it and before
		// the take-back.
		before, between func(*shareFx)
		want            string
	}{
		{"a grant sharing made goes", nil, nil, "u2"},
		{"a grant the GM set by hand stays", func(f *shareFx) { f.hand.allowed["z1"] = append(f.hand.allowed["z1"], "u1") }, nil, "u1,u2"},
		{"another holder still sharing keeps it", nil, func(f *shareFx) {
			f.rels.next++
			f.rels.rels["c3"] = append(f.rels.rels["c3"], &HasItemRelation{ID: f.rels.next, ItemEntityID: "z1", ItemName: "Zephyr Cloak", Metadata: []byte(`{"quantity":1}`)})
			if _, err := f.svc.Share(ctx, "camp", Actor{"u3", rPlayer}, "c3", "z1", []string{"u1"}); err != nil {
				panic(err)
			}
		}, "u1,u2"},
		{"the player holding it themselves keeps it", nil, func(f *shareFx) {
			if _, err := f.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c1", ItemID: "z1", Quantity: 1}); err != nil {
				panic(err)
			}
		}, "u1,u2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newShareFx()
			if tc.before != nil {
				tc.before(f)
			}
			if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", []string{"u1"}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(f.allowed("z1"), "u1") {
				t.Fatalf("not shared: %s", f.allowed("z1"))
			}
			if tc.between != nil {
				tc.between(f)
			}
			out, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(out.SharedWith) != 0 || sharedMessage(out) != "No longer shared by Mira" {
				t.Fatalf("outcome %+v", out)
			}
			if got := f.allowed("z1"); got != tc.want {
				t.Fatalf("allow list %s, want %s", got, tc.want)
			}
		})
	}
}

// When the holder whose share made the grant takes it back while another
// holder still shares, the grant passes to that share, and goes when it does.
func TestShare_GrantPassesToTheRemainingShare(t *testing.T) {
	ctx := context.Background()
	f := newShareFx()
	f.rels.next++
	f.rels.rels["c3"] = append(f.rels.rels["c3"], &HasItemRelation{ID: f.rels.next, ItemEntityID: "z1", ItemName: "Zephyr Cloak", Metadata: []byte(`{"quantity":1}`)})
	lee := Actor{"u3", rPlayer}
	steps := []struct {
		who   Actor
		char  string
		users []string
		want  string
	}{
		{robin, "c2", []string{"u1"}, "u1,u2"},
		{lee, "c3", []string{"u1"}, "u1,u2"},
		{robin, "c2", nil, "u1,u2"},
		{lee, "c3", nil, "u2"},
	}
	for i, st := range steps {
		if _, err := f.svc.Share(ctx, "camp", st.who, st.char, "z1", st.users); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if got := f.allowed("z1"); got != st.want {
			t.Fatalf("step %d: allow list %s, want %s", i, got, st.want)
		}
	}
}

func TestShare_OutcomeAndAudit(t *testing.T) {
	ctx := context.Background()
	f := newShareFx()
	out, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", []string{"u1", "u3"})
	if err != nil {
		t.Fatal(err)
	}
	if got := sharedMessage(out); got != "Shared with Grub, Thorin" {
		t.Fatalf("message %q", got)
	}
	if len(f.audit.events) != 1 || f.audit.events[0]["action"] != shareAuditAction || f.audit.events[0]["item_id"] != "z1" {
		t.Fatalf("audit %v", f.audit.events)
	}
	// Saving the same set again changes nothing and logs nothing.
	if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", []string{"u3", "u1"}); err != nil {
		t.Fatal(err)
	}
	if len(f.audit.events) != 1 {
		t.Fatalf("a no-op save was logged: %v", f.audit.events)
	}
	v, _ := f.svc.ShareBox(ctx, "camp", robin, "c2", "z1")
	for _, m := range v.Party {
		if !m.Checked {
			t.Fatalf("%s should be ticked", m.Character)
		}
	}
}

// The holder's player gets a Share link on hidden lines only; everyone who
// sees the panel reads who a line is shared with.
func TestCharacterPanel_ShareLinkAndLine(t *testing.T) {
	ctx := context.Background()
	f := newShareFx()
	if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", []string{"u1"}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		who       Actor
		wantShare bool
	}{{robin, true}, {gm, false}}
	for _, tc := range tests {
		t.Run(tc.who.UserID, func(t *testing.T) {
			v, err := f.svc.CharacterPanel(ctx, "camp", tc.who, "c2")
			if err != nil || v == nil {
				t.Fatalf("%v %v", v, err)
			}
			for _, it := range v.Items {
				if it.CanShare != (tc.wantShare && it.ItemID == "z1") {
					t.Errorf("%s CanShare=%v", it.ItemID, it.CanShare)
				}
				if it.ItemID == "z1" && strings.Join(it.SharedWith, ",") != "Thorin" {
					t.Errorf("shared with %v", it.SharedWith)
				}
			}
			var sb strings.Builder
			if err := CharacterPanel(v, "tok").Render(ctx, &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			if has := strings.Contains(out, "Share&hellip;") || strings.Contains(out, "Share…"); has != tc.wantShare {
				t.Errorf("share link present=%v", has)
			}
			if !strings.Contains(out, "Shared with Thorin") {
				t.Error("shared line missing")
			}
		})
	}
}

func TestShareBox_Renders(t *testing.T) {
	ctx := context.Background()
	f := newShareFx()
	f.dir.ents["z1"].HandoutMapID = "m1"
	v, err := f.svc.ShareBox(ctx, "camp", robin, "c2", "z1")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := ShareBox(v).Render(ctx, &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"Who else can see Zephyr Cloak?", `name="user" value="u1"`, "Thorin", "<small>Tess</small>",
		"They can open its page and the map. It stays on Mira.", "Not saved yet.", "/campaigns/camp/armory/characters/c2/items/z1/share"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	f.dir.ents["z1"].HandoutMapID = ""
	v, _ = f.svc.ShareBox(ctx, "camp", robin, "c2", "z1")
	sb.Reset()
	_ = ShareBox(v).Render(ctx, &sb)
	if !strings.Contains(sb.String(), "They can open its page. It stays on Mira.") {
		t.Error("non-map hint")
	}
}

func TestShareRoutes(t *testing.T) {
	f := newShareFx()
	h := NewStashHandler(f.svc)
	post := func(uid string, role campaigns.Role, form url.Values) (*httptest.ResponseRecorder, error) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("eid", "iid")
		c.SetParamValues("c2", "z1")
		c.Set("auth_user_id", uid)
		c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp"}, MemberRole: role})
		return rec, h.Share(c)
	}
	rec, err := post("u2", campaigns.RolePlayer, url.Values{"user": {"u1", "u3"}})
	if err != nil {
		t.Fatal(err)
	}
	var trig map[string]map[string]any
	_ = json.Unmarshal([]byte(rec.Header().Get("HX-Trigger")), &trig)
	if rec.Code != http.StatusNoContent || trig["armory-shared"]["message"] != "Shared with Grub, Thorin" || trig["armory-shared"]["itemId"] != "z1" {
		t.Fatalf("%d %s", rec.Code, rec.Header().Get("HX-Trigger"))
	}
	if _, err := post("u1", campaigns.RolePlayer, url.Values{"user": {"u1"}}); code(err) != http.StatusForbidden {
		t.Fatalf("another player: %v", err)
	}
}

// A failed grant or revoke must never leave a grant nothing tracks, nor a row
// that can't be retried.
func TestShare_FailuresLeaveNothingUntracked(t *testing.T) {
	ctx := context.Background()
	t.Run("grant fails: no row is left", func(t *testing.T) {
		f := newShareFx()
		f.hand.failAllow = true
		if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", []string{"u1"}); err == nil {
			t.Fatal("want an error")
		}
		if len(f.shares.rows) != 0 || f.allowed("z1") != "u2" {
			t.Fatalf("rows %v, allowed %s", f.shares.rows, f.allowed("z1"))
		}
	})
	t.Run("recording the grant fails: the grant is undone", func(t *testing.T) {
		f := newShareFx()
		f.shares.failSetMade = true
		if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", []string{"u1"}); err == nil {
			t.Fatal("want an error")
		}
		if len(f.shares.rows) != 0 || f.allowed("z1") != "u2" {
			t.Fatalf("rows %v, allowed %s", f.shares.rows, f.allowed("z1"))
		}
	})
	t.Run("revoke fails: the row stays and a retry finishes", func(t *testing.T) {
		f := newShareFx()
		if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", []string{"u1"}); err != nil {
			t.Fatal(err)
		}
		f.hand.failRevoke = true
		if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", nil); err == nil {
			t.Fatal("want an error")
		}
		if len(f.shares.rows) != 1 || f.allowed("z1") != "u1,u2" {
			t.Fatalf("rows %v, allowed %s", f.shares.rows, f.allowed("z1"))
		}
		f.hand.failRevoke = false
		if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", nil); err != nil {
			t.Fatal(err)
		}
		if len(f.shares.rows) != 0 || f.allowed("z1") != "u2" {
			t.Fatalf("rows %v, allowed %s", f.shares.rows, f.allowed("z1"))
		}
	})
}

// A GM-only line is hidden from its player, so it doesn't count as them
// holding the item and the grant sharing made still goes.
func TestShare_GMOnlyHoldingDoesNotKeepTheGrant(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		dmOnly bool
		want   string
	}{
		{"a visible line keeps it", false, "u1,u2"},
		{"a GM-only line does not", true, "u2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newShareFx()
			if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", []string{"u1"}); err != nil {
				t.Fatal(err)
			}
			f.rels.next++
			f.rels.rels["c1"] = append(f.rels.rels["c1"], &HasItemRelation{ID: f.rels.next, ItemEntityID: "z1", ItemName: "Zephyr Cloak", Metadata: []byte(`{"quantity":1}`), DmOnly: tc.dmOnly})
			if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", nil); err != nil {
				t.Fatal(err)
			}
			if got := f.allowed("z1"); got != tc.want {
				t.Fatalf("allow list %s, want %s", got, tc.want)
			}
		})
	}
}

// A share ends with the holding: when the last unit leaves, the holder's
// shares go by the same rule as taking them back by hand.
func TestShare_EndsWhenTheHoldingDoes(t *testing.T) {
	ctx := context.Background()
	lee := Actor{"u3", rPlayer}
	otherHolds := func(qty string) func(*shareFx) {
		return func(f *shareFx) {
			f.rels.next++
			f.rels.rels["c3"] = append(f.rels.rels["c3"], &HasItemRelation{ID: f.rels.next, ItemEntityID: "z1", ItemName: "Zephyr Cloak", Metadata: []byte(`{"quantity":` + qty + `}`)})
			if _, err := f.svc.Share(ctx, "camp", lee, "c3", "z1", []string{"u1"}); err != nil {
				panic(err)
			}
		}
	}
	tests := []struct {
		name     string
		between  func(*shareFx)
		keepZero bool
		wantList string
		wantRows int
	}{
		{"a take that deletes the line", nil, false, "u2", 0},
		{"a take that keeps a zero line, then drops it", nil, true, "u2", 0},
		{"another holder still sharing keeps the grant", otherHolds("1"), false, "u1,u2", 1},
		{"another holder who let go does not", func(f *shareFx) {
			otherHolds("1")(f)
			f.rels.rels["c3"][0].Metadata = []byte(`{"quantity":0}`)
		}, false, "u2", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newShareFx()
			if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", []string{"u1"}); err != nil {
				t.Fatal(err)
			}
			if tc.between != nil {
				tc.between(f)
			}
			s := f.svc.(*stashService)
			ok, err := s.adjustCarried(ctx, "camp", "c2", "z1", "u2", -1, true, tc.keepZero)
			if err != nil || !ok {
				t.Fatalf("take: %v %v", ok, err)
			}
			if tc.keepZero {
				if len(f.shares.rows) == 0 {
					t.Fatal("a kept zero line released early")
				}
				s.dropEmptyCarried(ctx, "camp", "c2", "z1")
			}
			if got := f.allowed("z1"); got != tc.wantList {
				t.Fatalf("allow list %s, want %s", got, tc.wantList)
			}
			if len(f.shares.rows) != tc.wantRows {
				t.Fatalf("rows %v, want %d", f.shares.rows, tc.wantRows)
			}
		})
	}
	t.Run("a partial take keeps the share", func(t *testing.T) {
		f := newShareFx()
		f.rels.rels["c2"][0].Metadata = []byte(`{"quantity":2}`)
		if _, err := f.svc.Share(ctx, "camp", robin, "c2", "z1", []string{"u1"}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.(*stashService).adjustCarried(ctx, "camp", "c2", "z1", "u2", -1, true, false); err != nil {
			t.Fatal(err)
		}
		if len(f.shares.rows) != 1 || f.allowed("z1") != "u1,u2" {
			t.Fatalf("rows %v, allowed %s", f.shares.rows, f.allowed("z1"))
		}
	})
}
