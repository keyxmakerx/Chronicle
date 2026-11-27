package designs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// secretKind keeps whatever object it is given, so a leak shows up in the
// stored data. Strip drops "secret" and "poison"; Validate refuses "poison",
// which only passes when Strip ran first.
type secretKind struct{}

func (secretKind) Validate(raw json.RawMessage) (json.RawMessage, error) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil, errors.New("not an object")
	}
	if _, bad := m["poison"]; bad {
		return nil, errors.New("poison is not a part")
	}
	return raw, nil
}

func (secretKind) Lacks(context.Context, string, json.RawMessage) ([]string, error) { return nil, nil }

func (secretKind) Strip(raw json.RawMessage) (json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	delete(m, "secret")
	delete(m, "poison")
	return json.Marshal(m)
}

// addingKind is secretKind that also makes the thing itself.
type addingKind struct {
	secretKind
	got  []json.RawMessage
	to   [2]string
	fail error
}

func (a *addingKind) Add(_ context.Context, campaignID, userID string, raw json.RawMessage) (AddResult, error) {
	if a.fail != nil {
		return AddResult{}, a.fail
	}
	a.got = append(a.got, raw)
	a.to = [2]string{campaignID, userID}
	return AddResult{ID: "made-1", LeftToPoint: []string{"Point it at a shop."}}, nil
}

const (
	kindSecret = "secret-kind"
	kindAdding = "adding-kind"
	// leaky is a sender's payload that still carries the Director-only part.
	leaky = `{"part":"oak","secret":"the vault code"}`
)

// secretWorld is newWorld with the two test kinds registered.
func secretWorld() (*service, *gallery, *memGallery, *memRepo, *addingKind) {
	g, lib, mg, _ := galleryWorld()
	add := &addingKind{}
	lib.kinds.(*Kinds).Register(kindSecret, secretKind{})
	lib.kinds.(*Kinds).Register(kindAdding, add)
	repo := lib.repo.(*memRepo)
	return lib, g, mg, repo, add
}

func saveKind(t *testing.T, s *service, kind string) *Design {
	t.Helper()
	d, err := s.Save(context.Background(), "A1", "alice", SaveInput{Kind: kind, Name: "Vault", Data: json.RawMessage(leaky)})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	return d
}

func leakCode(kind string) string {
	return encode(codeBody{Kind: kind, SystemID: "drawsteel", Name: "Vault", Data: json.RawMessage(leaky)})
}

// pathCase is one way a payload leaves a campaign (it returns what went out:
// a stored row, a code, a file).
type pathCase struct {
	name string
	run  func(t *testing.T, s *service, g *gallery, mg *memGallery, kind string) []byte
}

func sharePaths() []pathCase {
	ctx := context.Background()
	return []pathCase{
		{"share code", func(t *testing.T, s *service, _ *gallery, _ *memGallery, kind string) []byte {
			d := saveKind(t, s, kind)
			b, err := decodeCode(code(t, s, d.ID))
			if err != nil {
				t.Fatal(err)
			}
			return b.Data
		}},
		{"gallery publish", func(t *testing.T, s *service, g *gallery, mg *memGallery, kind string) []byte {
			d := saveKind(t, s, kind)
			m, err := g.Publish(ctx, "A1", "alice", d.ID, PublishInput{Name: "Vault"})
			if err != nil {
				t.Fatal(err)
			}
			e, _ := mg.Get(ctx, m.ID)
			return e.Data
		}},
		{"gallery update", func(t *testing.T, s *service, g *gallery, mg *memGallery, kind string) []byte {
			d := saveKind(t, s, kind)
			m, err := g.Publish(ctx, "A1", "alice", d.ID, PublishInput{Name: "Vault"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := g.Republish(ctx, "A1", "alice", d.ID, RepublishInput{}); err != nil {
				t.Fatal(err)
			}
			e, _ := mg.Get(ctx, m.ID)
			return e.Data
		}},
		{"suggestion", func(t *testing.T, s *service, _ *gallery, _ *memGallery, kind string) []byte {
			d := saveKind(t, s, kind)
			sg, err := s.Suggest(ctx, "A1", "alice", d.ID, SuggestInput{})
			if err != nil {
				t.Fatal(err)
			}
			var out []byte
			for _, v := range sg.Data {
				out = append(out, v...)
			}
			return out
		}},
	}
}

// addCase is one way a payload that still carries the secret reaches B1. The
// sender is trusted with nothing.
type addCase struct {
	name string
	run  func(t *testing.T, s *service, g *gallery, mg *memGallery, kind string) (*Taken, error)
}

func addPaths() []addCase {
	ctx := context.Background()
	return []addCase{
		{"take code", func(t *testing.T, s *service, _ *gallery, _ *memGallery, kind string) (*Taken, error) {
			return s.TakeCode(ctx, "B1", "bob", leakCode(kind))
		}},
		{"take link", func(t *testing.T, s *service, _ *gallery, _ *memGallery, kind string) (*Taken, error) {
			d := saveKind(t, s, kind)
			st, err := s.SetLink(ctx, "A1", "alice", d.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			return s.TakeLink(ctx, "bob", tokenOf(t, st), "B1")
		}},
		{"take shared", func(t *testing.T, s *service, _ *gallery, _ *memGallery, kind string) (*Taken, error) {
			d := saveKind(t, s, kind)
			if _, err := s.AddOwner(ctx, "A1", "alice", d.ID, "bob"); err != nil {
				t.Fatal(err)
			}
			return s.TakeShared(ctx, "B1", "bob", d.ID)
		}},
		{"gallery copy", func(t *testing.T, _ *service, g *gallery, mg *memGallery, kind string) (*Taken, error) {
			// Seeded straight into the gallery, as a row an older build stored.
			_ = mg.Insert(ctx, &GalleryEntry{ID: "e1", PublisherUserID: "alice", Kind: kind, SystemID: "drawsteel", Name: "Vault",
				MakerName: "Alice", Data: json.RawMessage(leaky), DataHash: "h", Status: StatusVisible, Reasons: []string{}})
			return g.Copy(ctx, "B1", "bob", "e1")
		}},
		{"copy package", func(t *testing.T, s *service, _ *gallery, _ *memGallery, kind string) (*Taken, error) {
			s.pkgs = fakePkgs{names: map[string]string{"drawsteel": "Draw Steel"}, items: map[string][]PackageItem{
				"drawsteel": {{Kind: kind, Name: "Vault", File: "designs/vault.json", Data: json.RawMessage(leaky)}},
			}}
			return s.CopyPackage(ctx, "B1", "bob", packageID(kind, "designs/vault.json"))
		}},
	}
}

func TestStripRunsOnEverySharePath(t *testing.T) {
	for _, tc := range sharePaths() {
		t.Run(tc.name, func(t *testing.T) {
			s, g, mg, _, _ := secretWorld()
			got := tc.run(t, s, g, mg, kindSecret)
			if !strings.Contains(string(got), `"oak"`) {
				t.Fatalf("path produced no data: %q", got)
			}
			if strings.Contains(string(got), "secret") || strings.Contains(string(got), "vault code") {
				t.Fatalf("the secret left the campaign: %s", got)
			}
		})
	}
}

func TestStripRunsOnEveryAddPath(t *testing.T) {
	for _, tc := range addPaths() {
		t.Run(tc.name, func(t *testing.T) {
			s, g, mg, repo, _ := secretWorld()
			tk, err := tc.run(t, s, g, mg, kindSecret)
			if err != nil {
				t.Fatal(err)
			}
			if tk.Design == nil || !strings.Contains(string(tk.Data), `"oak"`) {
				t.Fatalf("no library copy: %+v", tk)
			}
			if strings.Contains(string(tk.Data), "secret") {
				t.Fatalf("the secret reached the copy: %s", tk.Data)
			}
			stored, _ := repo.Get(context.Background(), tk.ID)
			if stored == nil || strings.Contains(string(stored.Data), "secret") {
				t.Fatalf("stored copy = %+v", stored)
			}
		})
	}
}

// Strip comes before Validate: a payload whose only fault is a part Strip
// removes is accepted, and one Validate refuses after stripping is not.
func TestStripRunsBeforeValidate(t *testing.T) {
	tests := []struct {
		name string
		data string
		want int
	}{
		{"fault is a stripped part", `{"part":"oak","poison":1}`, 0},
		{"not an object", `[1]`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _, _, _ := secretWorld()
			_, err := s.TakeCode(context.Background(), "B1", "bob", encode(codeBody{Kind: kindSecret, SystemID: "drawsteel", Name: "V", Data: json.RawMessage(tc.data)}))
			wantCode(t, err, tc.want)
		})
	}
}

func TestAdderMakesTheThingAndLeavesTheLibraryAlone(t *testing.T) {
	for _, tc := range addPaths() {
		t.Run(tc.name, func(t *testing.T) {
			s, g, mg, repo, add := secretWorld()
			tk, err := tc.run(t, s, g, mg, kindAdding)
			if err != nil {
				t.Fatal(err)
			}
			if tk.Design != nil || tk.ID != "made-1" || len(tk.LeftToPoint) != 1 {
				t.Fatalf("taken = %+v", tk)
			}
			if n, _ := repo.Count(context.Background(), "B1"); n != 0 {
				t.Fatalf("B1's library has %d designs", n)
			}
			if len(add.got) != 1 || strings.Contains(string(add.got[0]), "secret") || !strings.Contains(string(add.got[0]), "oak") {
				t.Fatalf("Add was given %q", add.got)
			}
			if add.to != [2]string{"B1", "bob"} {
				t.Fatalf("Add went to %v", add.to)
			}
		})
	}
}

func TestAddErrorsAreNeverRaw(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"plain error", errors.New("sql: connection refused"), http.StatusInternalServerError},
		{"app error passes through", apperror.NewConflict("a shop room already has that name"), http.StatusConflict},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _, _, add := secretWorld()
			add.fail = tc.err
			_, err := s.TakeCode(context.Background(), "B1", "bob", leakCode(kindAdding))
			wantCode(t, err, tc.want)
			// The client reads the message; a driver's words never belong in it.
			var ae *apperror.AppError
			if errors.As(err, &ae) && strings.Contains(ae.Message, "sql:") {
				t.Fatalf("raw error reached the client: %q", ae.Message)
			}
		})
	}
}

// A kind with no Adder keeps today's copy into the library, and the answer
// still says what it always did, with leftToPoint beside it.
func TestKindWithoutAdderCopiesAsBefore(t *testing.T) {
	s, _, _, repo, _ := newSecretless()
	tk, err := s.TakeCode(context.Background(), "B1", "bob", encode(codeBody{Kind: KindRoomTheme, SystemID: "drawsteel", Name: "Ward", Data: json.RawMessage(`{"part":"oak"}`)}))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.Count(context.Background(), "B1"); n != 1 || tk.Design == nil || tk.Origin != OriginCode {
		t.Fatalf("taken = %+v, library = %d", tk, n)
	}
	raw, _ := json.Marshal(tk)
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	if got["id"] != tk.Design.ID || got["name"] != "Ward" || got["origin"] != "code" {
		t.Fatalf("library copy JSON lost its fields: %s", raw)
	}
	if l, ok := got["leftToPoint"].([]any); !ok || len(l) != 0 {
		t.Fatalf("leftToPoint = %v", got["leftToPoint"])
	}

	// An Adder's answer is exactly {id, leftToPoint}.
	raw, _ = json.Marshal(&Taken{ID: "made-1", LeftToPoint: []string{"Point it."}})
	if string(raw) != `{"id":"made-1","leftToPoint":["Point it."]}` {
		t.Fatalf("adder JSON = %s", raw)
	}
}

func newSecretless() (*service, *gallery, *memGallery, *memRepo, *fakeDir) {
	g, lib, mg, dir := galleryWorld()
	return lib, g, mg, lib.repo.(*memRepo), dir
}

// otherKind is a kind in a group of its own.
type otherKind struct{ secretKind }

func (otherKind) Describe() KindDescription {
	return KindDescription{Group: "things", Label: "Things", One: "Thing", Where: "Find it with your things.", Order: 5}
}

func TestDescriberDrivesTheGroupMapping(t *testing.T) {
	ctx := context.Background()
	g, lib, mg, _ := galleryWorld()
	lib.kinds.(*Kinds).Register("thing-a", otherKind{})
	lib.kinds.(*Kinds).Register("thing-b", otherKind{})
	seed(mg, "r1", "bob", KindRoomTheme, "drawsteel", "Oak Room", "Bob", StatusVisible)
	seed(mg, "a1", "bob", "thing-a", "drawsteel", "A", "Bob", StatusVisible)
	seed(mg, "b1", "bob", "thing-b", "drawsteel", "B", "Bob", StatusVisible)
	seed(mg, "c1", "bob", "crafting-circle", "drawsteel", "Star", "Bob", StatusVisible)
	tests := []struct {
		name string
		kind string
		want string
		code int
	}{
		{"room-theme is still rooms", KindFilterRooms, "r1", 0},
		{"a described group maps to all its kinds", "things", "b1,a1", 0},
		{"crafting stays browsable before its kinds describe themselves", KindFilterCrafting, "c1", 0},
		{"an unknown group is refused", "gizmos", "", http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, err := g.Browse(ctx, "A1", "alice", BrowseInput{Kind: tc.kind})
			wantCode(t, err, tc.code)
			if tc.code != 0 {
				return
			}
			var ids []string
			for _, e := range page.Entries {
				ids = append(ids, e.ID)
			}
			if strings.Join(ids, ",") != tc.want {
				t.Fatalf("got %v, want %s", ids, tc.want)
			}
		})
	}
}

func TestStoreCatalog(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newWorld()
	s.kinds.(*Kinds).Register("thing-a", otherKind{})
	s.kinds.(*Kinds).Register("thing-b", otherKind{})
	s.kinds.(*Kinds).Register(kindSecret, secretKind{}) // no Describer: not in the Store
	s.pkgs = fakePkgs{names: map[string]string{"drawsteel": "Draw Steel", "dnd5e": "D&D 5e"}}

	got, err := s.StoreCatalog(ctx, "A1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	// One entry per group, things (order 5) before rooms (order 10).
	want := []StoreKind{
		{ID: "things", Label: "Things", One: "Thing", Where: "Find it with your things."},
		{ID: "rooms", Label: "Shop rooms", One: "Shop room", Where: "Find it in your designs."},
	}
	if len(got.Kinds) != len(want) || got.Kinds[0] != want[0] || got.Kinds[1] != want[1] {
		t.Fatalf("kinds = %+v", got.Kinds)
	}
	if got.Systems["drawsteel"] != "Draw Steel" || got.Systems["dnd5e"] != "D&D 5e" || len(got.Systems) != 2 {
		t.Fatalf("systems = %v", got.Systems)
	}
	// Every described kind is named with its group; an undescribed one is not.
	if got.Things["thing-a"] != (StoreThing{Group: "things", One: "Thing"}) || got.Things[KindRoomTheme].Group != "rooms" {
		t.Fatalf("things = %v", got.Things)
	}
	if _, ok := got.Things[kindSecret]; ok {
		t.Fatalf("an undescribed kind is in things: %v", got.Things)
	}

	// No source: still a well-formed, empty answer.
	s.pkgs = nil
	if got, _ = s.StoreCatalog(ctx, "A1", "alice"); got.Systems == nil || got.Things == nil {
		t.Fatal("systems must be an object, not null")
	}
	for _, who := range []string{"dan", "pat", "bob", ""} {
		_, err := s.StoreCatalog(ctx, "A1", who)
		wantCode(t, err, http.StatusForbidden)
	}
}

func TestStoreRoute(t *testing.T) {
	s, _, _ := newWorld()
	s.pkgs = fakePkgs{names: map[string]string{"drawsteel": "Draw Steel"}}
	h := NewHandler(s)
	run := func(role campaigns.Role, user string) (*httptest.ResponseRecorder, error) {
		c, rec := dctx(http.MethodGet, "", role)
		c.SetPathValues(echo.PathValues{{Name: "id", Value: "A1"}})
		c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "A1"}, MemberRole: role})
		auth.SetSession(c, &auth.Session{UserID: user})
		return rec, campaigns.RequireRole(campaigns.RoleOwner)(h.Store)(c)
	}

	t.Run("owner gets the shape", func(t *testing.T) {
		rec, err := run(campaigns.RoleOwner, "alice")
		if err != nil || rec.Code != http.StatusOK {
			t.Fatalf("code %d, err %v", rec.Code, err)
		}
		var got struct {
			Kinds   []map[string]string `json:"kinds"`
			Systems map[string]string   `json:"systems"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(rec.Body.Bytes()), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Kinds) != 1 || got.Kinds[0]["id"] != "rooms" || got.Kinds[0]["label"] != "Shop rooms" ||
			got.Kinds[0]["one"] != "Shop room" || got.Kinds[0]["where"] == "" || got.Systems["drawsteel"] != "Draw Steel" {
			t.Fatalf("body = %s", rec.Body.String())
		}
	})
	for _, tc := range []struct {
		name string
		role campaigns.Role
		user string
	}{{"player", campaigns.RolePlayer, "pat"}, {"scribe", campaigns.RoleScribe, "pat"}, {"stranger", campaigns.RoleNone, "bob"}} {
		t.Run(tc.name+" is refused", func(t *testing.T) {
			_, err := run(tc.role, tc.user)
			wantCode(t, err, http.StatusForbidden)
		})
	}
	t.Run("the service refuses a non-owner even past the gate", func(t *testing.T) {
		_, err := run(campaigns.RoleOwner, "dan")
		wantCode(t, err, http.StatusForbidden)
	})
}
