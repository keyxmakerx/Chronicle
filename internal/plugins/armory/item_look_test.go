package armory

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestNormalizeItemLook(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		cleared bool
		wantErr bool
	}{
		{"look only", `{"look":"ember"}`, `{"look":"ember"}`, false, false},
		{"every field", `{"look":"frost","tint":"#ffd66b","sound":"bell","words":" 4 cold "}`, `{"look":"frost","tint":"#ffd66b","sound":"bell","words":"4 cold"}`, false, false},
		{"silent", `{"look":"rot","sound":"none"}`, `{"look":"rot","sound":"none"}`, false, false},
		{"empty look clears", `{"look":""}`, ``, true, false},
		{"unknown look", `{"look":"fireball"}`, ``, false, true},
		{"colour off the palette", `{"look":"ember","tint":"#123456"}`, ``, false, true},
		{"css in the colour", `{"look":"ember","tint":"red;background:url(x)"}`, ``, false, true},
		{"unknown sound", `{"look":"ember","sound":"scream"}`, ``, false, true},
		{"words too long", `{"look":"ember","words":"` + strings.Repeat("é", 25) + `"}`, ``, false, true},
		{"words at the limit", `{"look":"ember","words":"` + strings.Repeat("é", 24) + `"}`, `{"look":"ember","words":"` + strings.Repeat("é", 24) + `"}`, false, false},
		{"control characters", `{"look":"ember","words":"a\u0007b"}`, ``, false, true},
		{"unknown field", `{"look":"ember","script":"x"}`, ``, false, true},
		{"trailing data", `{"look":"ember"}{"look":"frost"}`, ``, false, true},
		{"not json", `look=ember`, ``, false, true},
		{"too large", `{"look":"ember","words":"` + strings.Repeat(" ", maxItemLookBytes) + `"}`, ``, false, true},
		{"spots", `{"look":"ember","spots":{"trail":{"look":"frost"},"impact":{"look":"storm","tint":"#ffd66b"}}}`, `{"look":"ember","spots":{"impact":{"look":"storm","tint":"#ffd66b"},"trail":{"look":"frost"}}}`, false, false},
		{"every spot at once fits", `{"look":"glass","tint":"#ffd66b","sound":"bell","words":"` + strings.Repeat("é", 24) + `","spots":{"item":{"look":"glass","tint":"#ffd66b"},"trail":{"look":"glass","tint":"#ffd66b"},"impact":{"look":"glass","tint":"#ffd66b"},"body":{"look":"glass","tint":"#ffd66b"},"ground":{"look":"glass","tint":"#ffd66b"}}}`, `{"look":"glass","tint":"#ffd66b","sound":"bell","words":"` + strings.Repeat("é", 24) + `","spots":{"body":{"look":"glass","tint":"#ffd66b"},"ground":{"look":"glass","tint":"#ffd66b"},"impact":{"look":"glass","tint":"#ffd66b"},"item":{"look":"glass","tint":"#ffd66b"},"trail":{"look":"glass","tint":"#ffd66b"}}}`, false, false},
		{"empty spots dropped", `{"look":"ember","spots":{}}`, `{"look":"ember"}`, false, false},
		{"unknown spot", `{"look":"ember","spots":{"hand":{"look":"frost"}}}`, ``, false, true},
		{"unknown look on a spot", `{"look":"ember","spots":{"trail":{"look":"fireball"}}}`, ``, false, true},
		{"empty look on a spot", `{"look":"ember","spots":{"trail":{"look":""}}}`, ``, false, true},
		{"colour off the palette on a spot", `{"look":"ember","spots":{"trail":{"look":"frost","tint":"#000000"}}}`, ``, false, true},
		{"made from with no look of its own", `{"look":"","madeFrom":"0b1c-22","words":"6 fire"}`, `{"look":"","words":"6 fire","madeFrom":"0b1c-22"}`, false, false},
		{"made from with a look", `{"look":"frost","madeFrom":"abc"}`, `{"look":"frost","madeFrom":"abc"}`, false, false},
		{"made from that is not an id", `{"look":"frost","madeFrom":"../x"}`, ``, false, true},
		{"a colour needs a look", `{"look":"","madeFrom":"abc","tint":"#ffd66b"}`, ``, false, true},
		{"unknown field on a spot", `{"look":"ember","spots":{"trail":{"look":"frost","move":"shake"}}}`, ``, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, cleared, err := NormalizeItemLook([]byte(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr && !isStatus(err, http.StatusBadRequest) {
				t.Fatalf("err = %v, want 400", err)
			}
			if cleared != tc.cleared {
				t.Fatalf("cleared = %v, want %v", cleared, tc.cleared)
			}
			if string(got) != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

type fakeLookRepo struct {
	looks   map[string]json.RawMessage
	upserts int
	deletes int
	savedBy string
}

func newLookRepo(looks map[string]string) *fakeLookRepo {
	r := &fakeLookRepo{looks: map[string]json.RawMessage{}}
	for k, v := range looks {
		r.looks[k] = json.RawMessage(v)
	}
	return r
}
func (f *fakeLookRepo) Get(_ context.Context, _, id string) (json.RawMessage, error) {
	return f.looks[id], nil
}
func (f *fakeLookRepo) Upsert(_ context.Context, _, id, userID string, l json.RawMessage) error {
	f.upserts++
	f.savedBy = userID
	f.looks[id] = l
	return nil
}
func (f *fakeLookRepo) Delete(_ context.Context, _, id string) error {
	f.deletes++
	delete(f.looks, id)
	return nil
}
func (f *fakeLookRepo) List(_ context.Context, _ string, limit int) (map[string]json.RawMessage, bool, error) {
	out := map[string]json.RawMessage{}
	for k, v := range f.looks {
		if len(out) == limit {
			return out, true, nil
		}
		out[k] = v
	}
	return out, false, nil
}

// fakePages knows each entity's campaign and name.
type fakePages map[string][2]string

func (f fakePages) EntityName(_ context.Context, campaignID, entityID string) (string, bool, error) {
	p, ok := f[entityID]
	if !ok || p[0] != campaignID {
		return "", false, nil
	}
	return p[1], true, nil
}

var lookPages = fakePages{
	"sword":  {"camp-1", "Sword"},
	"ring":   {"camp-1", "Ring"},
	"adder":  {"camp-1", "Glass adder"},
	"stag":   {"camp-1", "Ashen stag"},
	"bare":   {"camp-1", "Bare page"},
	"remote": {"camp-2", "Elsewhere"},
}

func TestItemLookService_GetLook(t *testing.T) {
	repo := newLookRepo(map[string]string{
		"sword": `{"look":"ember"}`,
		"ring":  `{"look":"ember"}`,
		"adder": `{"look":"glass","madeFrom":"stag"}`,
	})
	cases := []struct {
		name     string
		campaign string
		entity   string
		role     int
		vis      EntityVisibilityFilter
		wantNil  bool
		wantCode int
	}{
		{"player sees a visible item", "camp-1", "sword", permissions.RolePlayer, &fakeRoomVis{viewable: map[string]bool{"sword": true}}, false, 0},
		{"hidden item is not found", "camp-1", "ring", permissions.RolePlayer, &fakeRoomVis{viewable: map[string]bool{}}, true, http.StatusNotFound},
		{"owner sees a hidden item", "camp-1", "ring", permissions.RoleOwner, &fakeRoomVis{viewable: map[string]bool{}}, false, 0},
		{"any page can hold a look", "camp-1", "adder", permissions.RoleOwner, nil, false, 0},
		{"a page with no look answers null", "camp-1", "bare", permissions.RoleOwner, nil, true, 0},
		{"missing page is not found", "camp-1", "ghost", permissions.RoleOwner, nil, true, http.StatusNotFound},
		{"other campaign is not found", "camp-2", "sword", permissions.RoleOwner, nil, true, http.StatusNotFound},
		{"no visibility filter fails closed", "camp-1", "sword", permissions.RolePlayer, nil, true, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewItemLookService(repo, lookPages, tc.vis)
			got, err := svc.GetLook(context.Background(), tc.campaign, tc.entity, tc.role, "u1")
			if tc.wantCode != 0 {
				if !isStatus(err, tc.wantCode) {
					t.Fatalf("err = %v, want %d", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if (got.Look == nil) != tc.wantNil {
				t.Fatalf("got %s, want nil %v", got.Look, tc.wantNil)
			}
		})
	}
}

func TestItemLookService_MadeFrom(t *testing.T) {
	repo := newLookRepo(map[string]string{
		"sword": `{"look":"","madeFrom":"adder","words":"6 fire"}`,
		"ring":  `{"look":"frost","madeFrom":"bare"}`,
		"adder": `{"look":"glass","madeFrom":"stag"}`,
	})
	seeAdder := &fakeRoomVis{viewable: map[string]bool{"sword": true, "ring": true, "adder": true}}
	hideAdder := &fakeRoomVis{viewable: map[string]bool{"sword": true, "ring": true}}
	cases := []struct {
		name       string
		entity     string
		role       int
		vis        EntityVisibilityFilter
		wantSource string // JSON of the source, or "" for none
	}{
		{"owner sees the source and its look, one hop only", "sword", permissions.RoleOwner, nil, `{"id":"adder","name":"Glass adder","url":"/campaigns/camp-1/entities/adder","look":{"look":"glass"}}`},
		{"player who can see the source gets its name", "sword", permissions.RolePlayer, seeAdder, `{"id":"adder","name":"Glass adder","url":"/campaigns/camp-1/entities/adder","look":{"look":"glass"}}`},
		{"player who cannot see the source gets only its look", "sword", permissions.RolePlayer, hideAdder, `{"look":{"look":"glass"}}`},
		{"a source with no look is still named to the owner", "ring", permissions.RoleOwner, nil, `{"id":"bare","name":"Bare page","url":"/campaigns/camp-1/entities/bare","look":null}`},
		{"a hidden source with no look says nothing", "ring", permissions.RolePlayer, hideAdder, ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewItemLookService(repo, lookPages, tc.vis).GetLook(context.Background(), "camp-1", tc.entity, tc.role, "u1")
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			var src string
			if got.Source != nil {
				b, _ := json.Marshal(got.Source)
				src = string(b)
			}
			if src != tc.wantSource {
				t.Fatalf("source = %s, want %s", src, tc.wantSource)
			}
		})
	}

	// A deleted source leaves the item with its own look.
	gone := newLookRepo(map[string]string{"ring": `{"look":"frost","madeFrom":"ghost"}`})
	got, err := NewItemLookService(gone, lookPages, nil).GetLook(context.Background(), "camp-1", "ring", permissions.RoleOwner, "u1")
	if err != nil || got.Source != nil || string(got.Look) != `{"look":"frost","madeFrom":"ghost"}` {
		t.Fatalf("deleted source: got %+v err %v", got, err)
	}
}

func TestItemLookService_SaveLook(t *testing.T) {
	repo := newLookRepo(map[string]string{"adder": `{"look":"glass"}`})
	svc := NewItemLookService(repo, lookPages, nil)
	ctx := context.Background()

	got, err := svc.SaveLook(ctx, "camp-1", "sword", "owner", []byte(`{"look":"storm","words":"7 lightning"}`))
	if err != nil || string(got.Look) != `{"look":"storm","words":"7 lightning"}` || repo.upserts != 1 || repo.savedBy != "owner" {
		t.Fatalf("save: got %+v err %v upserts %d by %q", got, err, repo.upserts, repo.savedBy)
	}

	got, err = svc.SaveLook(ctx, "camp-1", "sword", "owner", []byte(`{"look":"","madeFrom":"adder"}`))
	if err != nil || got.Source == nil || got.Source.Name != "Glass adder" || string(got.Source.Look) != `{"look":"glass"}` {
		t.Fatalf("made from: got %+v err %v", got, err)
	}

	got, err = svc.SaveLook(ctx, "camp-1", "sword", "owner", []byte(`{"look":""}`))
	if err != nil || got.Look != nil || repo.deletes != 1 || repo.looks["sword"] != nil {
		t.Fatalf("clear: got %+v err %v deletes %d", got, err, repo.deletes)
	}

	refused := []struct {
		name, campaign, entity, body string
		code                         int
	}{
		{"bad look", "camp-1", "sword", `{"look":"nope"}`, http.StatusBadRequest},
		{"other campaign", "camp-2", "sword", `{"look":"ember"}`, http.StatusNotFound},
		{"made from itself", "camp-1", "sword", `{"look":"ember","madeFrom":"sword"}`, http.StatusBadRequest},
		{"made from a missing page", "camp-1", "sword", `{"look":"ember","madeFrom":"ghost"}`, http.StatusBadRequest},
		{"made from another campaign's page", "camp-1", "sword", `{"look":"ember","madeFrom":"remote"}`, http.StatusBadRequest},
	}
	for _, tc := range refused {
		if _, err := svc.SaveLook(ctx, tc.campaign, tc.entity, "owner", []byte(tc.body)); !isStatus(err, tc.code) {
			t.Fatalf("%s: err = %v, want %d", tc.name, err, tc.code)
		}
	}
	if repo.upserts != 2 {
		t.Fatalf("a refused save wrote: upserts = %d", repo.upserts)
	}
}

func TestItemLookService_ListLooks(t *testing.T) {
	repo := newLookRepo(map[string]string{
		"sword": `{"look":"","madeFrom":"adder","words":"6 fire"}`,
		"ring":  `{"look":"frost"}`,
		"adder": `{"look":"glass","madeFrom":"stag"}`,
		"ghost": `{"look":"ember"}`,
	})
	cases := []struct {
		name string
		role int
		vis  EntityVisibilityFilter
		want string // name<made from> per page, in order; ghost has no page
	}{
		{"owner sees every page and its source", permissions.RoleOwner, nil, "Glass adder<Ashen stag>|Ring|Sword<Glass adder>"},
		{"player sees only visible pages, and no hidden source", permissions.RolePlayer, &fakeRoomVis{viewable: map[string]bool{"sword": true, "ring": true}}, "Ring|Sword"},
		{"no visibility filter fails closed", permissions.RolePlayer, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, more, err := NewItemLookService(repo, lookPages, tc.vis).ListLooks(context.Background(), "camp-1", tc.role, "u1")
			if err != nil || more {
				t.Fatalf("err %v more %v", err, more)
			}
			var parts []string
			for _, p := range got {
				if p.Look.MadeFrom != "" {
					t.Fatalf("%s kept a page id in MadeFrom", p.Name)
				}
				s := p.Name
				if p.MadeFromName != "" {
					s += "<" + p.MadeFromName + ">"
				}
				parts = append(parts, s)
			}
			if g := strings.Join(parts, "|"); g != tc.want {
				t.Fatalf("got %q, want %q", g, tc.want)
			}
		})
	}
}

func TestItemLookService_CheckLook(t *testing.T) {
	repo := newLookRepo(nil)
	svc := NewItemLookService(repo, lookPages, nil)
	ctx := context.Background()
	if cleared, err := svc.CheckLook(ctx, "camp-1", "sword", []byte(`{"look":"ember","madeFrom":"adder"}`)); err != nil || cleared {
		t.Fatalf("good look: cleared %v err %v", cleared, err)
	}
	if cleared, err := svc.CheckLook(ctx, "camp-1", "sword", []byte(`{"look":""}`)); err != nil || !cleared {
		t.Fatalf("clearing: cleared %v err %v", cleared, err)
	}
	if _, err := svc.CheckLook(ctx, "camp-1", "sword", []byte(`{"look":"ember","madeFrom":"sword"}`)); !isStatus(err, http.StatusBadRequest) {
		t.Fatalf("made from itself: err %v", err)
	}
	if _, err := svc.CheckLook(ctx, "camp-1", "ghost", []byte(`{"look":"ember"}`)); !isStatus(err, http.StatusNotFound) {
		t.Fatalf("missing page: err %v", err)
	}
	if repo.upserts+repo.deletes != 0 {
		t.Fatal("a check wrote")
	}
}
