package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// The methods another thread's optional interfaces will look for, with the
// signatures the Store framework names.
var _ interface {
	Strip(json.RawMessage) (json.RawMessage, error)
	Add(context.Context, string, string, json.RawMessage) (homebrewAddResult, error)
	Package(context.Context, string, string, string) (json.RawMessage, error)
} = homebrewStoreKind{}

// storeTeam answers who is on a campaign: "dm" owns it, "codm" was granted
// the Director's view, "jo" plays, and anyone else is not in it.
type storeTeam struct{}

func (storeTeam) GetMember(_ context.Context, _, userID string) (*campaigns.CampaignMember, error) {
	switch userID {
	case "dm":
		return &campaigns.CampaignMember{UserID: userID, Role: campaigns.RoleOwner}, nil
	case "codm", "jo":
		return &campaigns.CampaignMember{UserID: userID, Role: campaigns.RoleScribe}, nil
	}
	return nil, apperror.NewNotFound("member")
}

func (storeTeam) IsUserDmGranted(_ context.Context, _, userID string) (bool, error) {
	return userID == "codm", nil
}

const (
	storeSecretWord = "cellar-under-the-chapel"
	storePageID     = "page-77"
)

func storeKind(t *testing.T) (homebrewStoreKind, *memRecipes) {
	t.Helper()
	reg := homebrew.NewRegistry()
	placeName := func(_ context.Context, campaignID, _ string) (string, bool) {
		return "Hearthvale", campaignID == "camp-a"
	}
	parts := []homebrew.PartKind{
		{Name: "words", Label: "Local words", Share: shareWordsPart,
			Check: func(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
				return checkWordsPart(ctx, campaignID, raw, placeName)
			}},
		{Name: "creature-recipe", Label: "Creature recipe", ForceSee: homebrew.SeeDirector, Share: shareRecipeLink,
			Check: func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
				return nil, apperror.NewValidation("points at a page that isn't in this campaign")
			}},
	}
	for _, p := range parts {
		if err := reg.RegisterPart(p); err != nil {
			t.Fatal(err)
		}
	}
	store := &memRecipes{stored: map[string]homebrew.Recipe{}}
	return homebrewStoreKind{svc: homebrew.NewService(store, reg, nil, nil), members: storeTeam{}}, store
}

func seedStoreRecipe(t *testing.T, k homebrewStoreKind) string {
	t.Helper()
	data := `{"placeId":"` + storePageID + `","words":[{"word":"inn","local":"hearth"},{"word":"vault","local":"` + storeSecretWord + `","director":true}]}`
	r, err := k.svc.Create(context.Background(), "camp-a", homebrew.Viewer{UserID: "dm", Director: true}, homebrew.RecipeInput{
		Name: "Hearthvale words",
		Doc: homebrew.Doc{Parts: []homebrew.Part{
			{Key: "w", Kind: "words", Label: "Words", See: homebrew.SeeParty, Data: json.RawMessage(data)},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

func TestHomebrewStoreKindShareAddRoundTrip(t *testing.T) {
	k, store := storeKind(t)
	ctx := context.Background()
	id := seedStoreRecipe(t, k)

	pkg, err := k.Package(ctx, "camp-a", "dm", id)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{storeSecretWord, storePageID, "vault", `"director"`, `"placeId"`} {
		if bytes.Contains(pkg, []byte(banned)) {
			t.Errorf("package holds %q: %s", banned, pkg)
		}
	}
	canon, err := k.Validate(pkg)
	if err != nil || !bytes.Equal(canon, pkg) {
		t.Fatalf("validate = %s, %v", canon, err)
	}
	// What the Store would do on add: strip again, then validate, then add.
	again, err := k.Strip(pkg)
	if err != nil || !bytes.Equal(again, pkg) {
		t.Fatalf("strip changed a stripped package: %s, %v", again, err)
	}
	lacks, err := k.Lacks(ctx, "camp-b", pkg)
	if err != nil || len(lacks) != 0 {
		t.Fatalf("lacks = %v, %v", lacks, err)
	}

	added, err := k.Add(ctx, "camp-b", "dm", pkg)
	if err != nil {
		t.Fatal(err)
	}
	if len(added.LeftToPoint) != 1 || added.LeftToPoint[0] != "Words: choose a place page." {
		t.Errorf("left to point = %v", added.LeftToPoint)
	}
	got, ok := store.stored[added.ID]
	if !ok || !got.Doc.Off {
		t.Fatalf("added = %+v (found %v)", got, ok)
	}
	stored, _ := json.Marshal(got.Doc)
	if bytes.Contains(stored, []byte(storeSecretWord)) || bytes.Contains(stored, []byte(storePageID)) {
		t.Errorf("stored copy holds the sender's text or id: %s", stored)
	}
}

func TestHomebrewStoreKindDirectorTeamOnly(t *testing.T) {
	k, store := storeKind(t)
	ctx := context.Background()
	id := seedStoreRecipe(t, k)
	pkg, err := k.Package(ctx, "camp-a", "dm", id)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		user string
		ok   bool
	}{{"dm", true}, {"codm", true}, {"jo", false}, {"stranger", false}, {"", false}} {
		t.Run("add as "+tt.user, func(t *testing.T) {
			n := len(store.stored)
			_, err := k.Add(ctx, "camp-b", tt.user, pkg)
			if tt.ok != (err == nil) || (!tt.ok && hbCode(err) != 403) {
				t.Errorf("err = %v", err)
			}
			if !tt.ok && len(store.stored) != n {
				t.Error("a refused add stored a recipe")
			}
		})
		t.Run("package as "+tt.user, func(t *testing.T) {
			_, err := k.Package(ctx, "camp-a", tt.user, id)
			if tt.ok != (err == nil) || (!tt.ok && hbCode(err) != 403) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestHomebrewStoreKindRefusesHostile(t *testing.T) {
	k, store := storeKind(t)
	ctx := context.Background()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"not JSON", `{{`, "can read"},
		{"a kind no one here owns", `{"name":"X","doc":{"parts":[{"key":"a","kind":"far-off","label":"A"}],"rules":[]}}`, "doesn't have"},
		{"a setting the owner doesn't have", `{"name":"X","doc":{"parts":[{"key":"w","kind":"words","label":"W","data":{"words":[{"word":"a","local":"b"}],"script":"x"}}],"rules":[]}}`, "recognise"},
		{"every word is the Director's", `{"name":"X","doc":{"parts":[{"key":"w","kind":"words","label":"W","data":{"words":[{"word":"a","local":"b","director":true}]}}],"rules":[]}}`, "nothing of it is shared"},
		{"oversized", `{"name":"X","doc":{"parts":[],"rules":[]},"pad":"` + strings.Repeat("a", homebrew.MaxBodyBytes) + `"}`, "too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := k.Add(ctx, "camp-b", "dm", json.RawMessage(tt.in)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("add err = %v, want %q", err, tt.want)
			}
		})
	}
	if len(store.stored) != 0 {
		t.Errorf("a refused add stored %d recipes", len(store.stored))
	}
	// Validate (no campaign) also turns away a part that still holds what
	// sharing drops, and unknown fields.
	for name, in := range map[string]string{
		"id kept":       `{"name":"X","doc":{"parts":[{"key":"w","kind":"words","label":"W","data":{"placeId":"p","words":[{"word":"a","local":"b"}]}}],"rules":[]}}`,
		"director kept": `{"name":"X","doc":{"parts":[{"key":"w","kind":"words","label":"W","data":{"words":[{"word":"a","local":"b"},{"word":"c","local":"d","director":true}]}}],"rules":[]}}`,
		"notes":         `{"name":"X","notes":"hi","doc":{"parts":[],"rules":[]}}`,
	} {
		if _, err := k.Validate(json.RawMessage(in)); err == nil {
			t.Errorf("%s: validate accepted", name)
		}
	}
}

func TestShareHooksStripAndLeaveTheRestAlone(t *testing.T) {
	tests := []struct {
		name  string
		share func(json.RawMessage, []homebrew.Unlinked) (json.RawMessage, []homebrew.Unlinked, error)
		in    string
		want  string
		picks int
	}{
		{"item drops the Director's fields", shareItemPart,
			`{"category":"Weapons","fields":[{"label":"Reach"},{"label":"Curse","director":true}]}`,
			`{"category":"Weapons","fields":[{"label":"Reach"}]}`, 0},
		{"visible field is left as it is", shareFieldPart, `{"category":"Heroes","label":"Rank"}`, `{"category":"Heroes","label":"Rank"}`, 0},
		{"crafting drops recipe ids", shareCraftingPart, `{"who":"all","start":"draft","recipes":["r1","r2"]}`, `{"who":"all","start":"draft"}`, 1},
		{"crafting with none has nothing to pick", shareCraftingPart, `{"who":"all","start":"draft"}`, `{"who":"all","start":"draft"}`, 0},
		{"creature recipe drops its page", shareRecipeLink, `{"page":"p1"}`, `{"page":""}`, 1},
		{"creature mark drops every page", shareCreatureMark,
			`{"recipe":"a","addons":["b"],"marked":["c"],"variants":{"c":"d"}}`, `{"recipe":""}`, 3},
		{"ancestry drops its slug", shareAncestry, `{"slug":"elf"}`, `{"slug":""}`, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, picks, err := tt.share(json.RawMessage(tt.in), nil)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("data = %s, want %s", got, tt.want)
			}
			if len(picks) != tt.picks {
				t.Errorf("picks = %v", picks)
			}
			// Stripping what is already stripped changes nothing, and the owner
			// makes the same markers when given them back as the sender's.
			again, more, err := tt.share(got, picks)
			if err != nil || string(again) != tt.want || len(more) != len(picks) {
				t.Errorf("second pass = %s, %v, %v", again, more, err)
			}
		})
	}
	// A marker the sender wrote is kept only where the owner would have made
	// it, and never invented.
	if _, picks, _ := shareWordsPart(json.RawMessage(`{"words":[{"word":"a","local":"b"}]}`), []homebrew.Unlinked{{Pick: "a place page"}, {Pick: "anything"}}); len(picks) != 1 || picks[0].Pick != "a place page" {
		t.Errorf("words picks = %v", picks)
	}
	if _, picks, _ := shareWordsPart(json.RawMessage(`{"era":"Dusk","words":[{"word":"a","local":"b"}]}`), []homebrew.Unlinked{{Pick: "a place page"}}); len(picks) != 0 {
		t.Errorf("an era's words kept a place marker: %v", picks)
	}
	if _, _, err := shareFieldPart(json.RawMessage(`{"category":"Heroes","label":"Curse","director":true}`), nil); err == nil {
		t.Error("a Director-only field was shared")
	}
}

func TestPartyEffectShare(t *testing.T) {
	effect := func(see string) json.RawMessage {
		return json.RawMessage(`{"name":"Eclipse","text":"` + storeSecretWord + `","who":"party","see":"` + see + `","lasts":"until_change"}`)
	}
	if _, err := sharePartyEffect(effect("director")); err == nil || !strings.Contains(err.Error(), "only the Director") {
		t.Errorf("a Director-only effect was shared: %v", err)
	}
	if got, err := sharePartyEffect(effect("party")); err != nil || !bytes.Contains(got, []byte("Eclipse")) {
		t.Errorf("a party effect was refused: %s, %v", got, err)
	}
	if _, err := sharePartyEffect(json.RawMessage(`{"name":"x","extra":1}`)); err == nil {
		t.Error("an unknown setting was passed along")
	}
}

// A Director-only party.effect can't be packaged, and one a sender slips in is
// refused with its text nowhere in what is stored.
func TestHomebrewStoreKindRefusesDirectorOnlyEffect(t *testing.T) {
	k, store := storeKind(t)
	reg := homebrew.NewRegistry()
	if err := reg.RegisterAction(homebrew.ActionDef{Name: "party.effect", Label: "Effect", Check: checkPartyEffect, Share: sharePartyEffect}); err != nil {
		t.Fatal(err)
	}
	k.svc = homebrew.NewService(store, reg, nil, nil)
	hostile := `{"name":"X","doc":{"parts":[],"rules":[{"when":{"event":"moon.changed"},"then":[{"action":"party.effect","args":{"name":"Eclipse","text":"` + storeSecretWord + `","who":"party","see":"director","lasts":"until_change"}}]}]}}`
	if _, err := k.Add(context.Background(), "camp-b", "dm", json.RawMessage(hostile)); err == nil || !strings.Contains(err.Error(), "only the Director") {
		t.Errorf("add err = %v", err)
	}
	if _, err := k.Strip(json.RawMessage(hostile)); err == nil {
		t.Error("strip passed a Director-only effect")
	}
	if len(store.stored) != 0 {
		t.Error("a refused add stored a recipe")
	}
}
