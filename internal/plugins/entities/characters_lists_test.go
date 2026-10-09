package entities

// characters_lists_test.go pins the Characters page's list controls: only the
// owner's page carries them, the party shows only the page types the owner
// listed, the viewer's own characters sit on top, and the change endpoint is
// not reachable without signing in.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeCastLists is a CharacterListManager with fixed answers.
type fakeCastLists struct {
	chars, npcs []int
	chosen      CharacterLists
}

func (f *fakeCastLists) CharacterTypeIDs(context.Context, string) ([]int, error) { return f.chars, nil }
func (f *fakeCastLists) NPCTypeIDs(context.Context, string) ([]int, error)       { return f.npcs, nil }
func (f *fakeCastLists) Chosen(context.Context, string) (CharacterLists, error)  { return f.chosen, nil }
func (f *fakeCastLists) Editor(_ context.Context, _ string, k CharacterListKind) (CastBandEditor, error) {
	return CastBandEditor{Kind: k, Label: bandLabel(k)}, nil
}
func (f *fakeCastLists) Add(context.Context, string, CharacterListKind, int) (string, error) {
	return "", nil
}
func (f *fakeCastLists) Remove(context.Context, string, CharacterListKind, int) (string, error) {
	return "", nil
}

func renderCastContent(t *testing.T, view CastView) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp1", Name: "Test Campaign"}}
	var sb strings.Builder
	if err := CharactersContent(cc, view).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func TestCharactersContent_ControlsAreOwnerOnly(t *testing.T) {
	ed := &CastBandEditor{
		Kind:    CharacterListCharacters,
		Label:   "The Party",
		Chips:   []CastChip{{ID: 1, Name: "Character"}},
		Options: []CastTypeOption{{ID: 2, Name: "Villain", IsSub: true, Count: 3}},
	}
	party := []CastMember{{Entity: Entity{ID: "a", Name: "Aldric"}}}

	player := renderCastContent(t, CastView{ShowPlayers: true, PartyListed: true, Party: party})
	for _, owners := range []string{"ag-add", "ag-chip", "ag-fold", "characters/lists", "csrf_token"} {
		if strings.Contains(player, owners) {
			t.Errorf("a player's Characters page carries owner control %q", owners)
		}
	}

	owner := renderCastContent(t, CastView{ShowPlayers: true, PartyListed: true, Party: party, PartyEditor: ed, CSRFToken: "tok"})
	for _, want := range []string{"ag-add", `data-chip="1"`, `data-box="add"`, `data-box="rm-1"`, "/campaigns/camp1/characters/lists", `name="csrf_token" value="tok"`, "Villain"} {
		if !strings.Contains(owner, want) {
			t.Errorf("the owner's Characters page is missing %q", want)
		}
	}
	// Swap safety: the controls are inline handlers, never a script tag or a
	// templ script helper.
	if strings.Contains(owner, "<script") {
		t.Error("the list controls must not emit a script tag")
	}
	if strings.Contains(owner, "confirm(") {
		t.Error("removing a type confirms in place, not with a browser dialog")
	}
}

func TestCharactersContent_BandsFollowTheLists(t *testing.T) {
	party := []CastMember{{Entity: Entity{ID: "a", Name: "Aldric"}}}
	ed := &CastBandEditor{Kind: CharacterListCharacters, Label: "The Party"}

	tests := []struct {
		name string
		view CastView
		want map[string]bool
	}{
		{"no party type listed hides the band from a player",
			CastView{ShowPlayers: true}, map[string]bool{"The Party": false}},
		{"no party type listed shows the owner the Add prompt",
			CastView{ShowPlayers: true, PartyEditor: ed}, map[string]bool{"The Party": true, "No page types yet": true}},
		{"a listed party shows its cards",
			CastView{ShowPlayers: true, PartyListed: true, Party: party}, map[string]bool{"The Party": true, "Aldric": true, "No page types yet": false}},
		{"the viewer's own characters get a Yours band",
			CastView{Yours: party}, map[string]bool{"Yours": true, "Aldric": true}},
		{"no own characters, no Yours band",
			CastView{ShowPlayers: true, PartyListed: true, Party: party}, map[string]bool{"Yours": false}},
		{"the old My characters link is gone",
			CastView{ShowPlayers: true, IsMember: true, PartyListed: true, Party: party}, map[string]bool{"My characters": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderCastContent(t, tt.view)
			for needle, want := range tt.want {
				if got := strings.Contains(out, needle); got != want {
					t.Errorf("contains %q = %v, want %v", needle, got, want)
				}
			}
		})
	}
}

func TestFilterCastByType(t *testing.T) {
	in := []CastMember{
		{Entity: Entity{ID: "a", EntityTypeID: 1}},
		{Entity: Entity{ID: "b", EntityTypeID: 2}},
		{Entity: Entity{ID: "c", EntityTypeID: 1}},
	}
	tests := []struct {
		name    string
		allowed []int
		want    string
	}{
		{"nothing listed", nil, ""},
		{"one type", []int{1}, "ac"},
		{"both types", []int{1, 2}, "abc"},
		{"a type with no pages", []int{9}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			for _, m := range filterCastByType(in, tt.allowed) {
				got += m.Entity.ID
			}
			if got != tt.want {
				t.Errorf("kept %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpdateCharacterListsNeedsSignIn(t *testing.T) {
	e := echo.New()
	e.Use(emw.Recover())
	h := NewHandler(&castEntitySvc{})
	h.SetCharacterLists(&fakeCastLists{})
	RegisterRoutes(e, h, guardCampaignSvc{public: true}, guardAuthSvc{})

	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/characters/lists", strings.NewReader("list=npcs&op=add&type_id=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if !isLoginRedirect(rec) {
		t.Errorf("anonymous POST /characters/lists: code=%d, want a sign-in redirect even on a public campaign", rec.Code)
	}
}
