package entities

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestPartyFacts(t *testing.T) {
	fields := []FieldDefinition{
		{Key: "ancestry", Label: "Ancestry"},
		{Key: "class", Label: "Class", GMOnly: true},
		{Key: "career", Label: "Career", OwnerOnly: true},
		{Key: "level", Label: "Level"},
	}
	data := map[string]any{"ancestry": "  High   Elf ", "class": "Secret Class", "career": "Spy", "level": float64(3)}
	ancestry, class, level := partyFacts(data, fields)
	if ancestry != "High Elf" || class != "" || level != "3" {
		t.Errorf("got ancestry %q class %q level %q; a GM-only or owner-only field must never be read", ancestry, class, level)
	}
	// A system that names the same ideas differently still shows them.
	a, c, l := partyFacts(
		map[string]any{"race": "Polder", "calling": "Shadow", "level": 2},
		[]FieldDefinition{{Key: "race"}, {Key: "calling"}, {Key: "level"}},
	)
	if a != "Polder" || c != "Shadow" || l != "2" {
		t.Errorf("synonyms: got %q %q %q", a, c, l)
	}
	// A value that is not a short scalar is left out, never printed as Go syntax.
	a, _, l = partyFacts(map[string]any{"ancestry": []any{"x"}, "level": 2.5}, []FieldDefinition{{Key: "ancestry"}, {Key: "level"}})
	if a != "" || l != "" {
		t.Errorf("non-scalar values should be skipped, got %q %q", a, l)
	}
}

func TestBuildParty(t *testing.T) {
	typ := &EntityType{ID: 3, Fields: []FieldDefinition{{Key: "ancestry"}, {Key: "class"}, {Key: "level"}}}
	hero := func(id, name string) Entity {
		return Entity{ID: id, Name: name, EntityTypeID: 3, FieldsData: map[string]any{"ancestry": "Dwarf", "class": "Fury", "level": 3}}
	}
	members := []CastMember{
		{Entity: hero("e1", "Brannoch"), OwnerName: "Sam"},
		{Entity: hero("e2", "Ysolde"), OwnerName: "Priya", IsViewer: true},
		{Entity: Entity{ID: "e3", Name: "Nobody", EntityTypeID: 99}},
	}
	v := buildParty("c1", members, map[int]*EntityType{3: typ})
	if len(v.Heroes) != 3 {
		t.Fatalf("heroes = %d", len(v.Heroes))
	}
	if h := v.Heroes[0]; h.Line != "Dwarf Fury" || h.Level != "Level 3" || h.Player != "Sam" || h.Href != "/campaigns/c1/entities/e1" {
		t.Errorf("first hero = %+v", h)
	}
	if !v.Heroes[1].You {
		t.Error("the viewer's own hero is marked")
	}
	if h := v.Heroes[2]; h.Line != "" || h.Level != "" {
		t.Errorf("a hero of an unknown type shows its name only, got %+v", h)
	}

	var many []CastMember
	for i := 0; i < partyMaxHeroes+3; i++ {
		many = append(many, CastMember{Entity: hero("e", "H")})
	}
	if v := buildParty("c1", many, nil); len(v.Heroes) != partyMaxHeroes || v.More != 3 {
		t.Errorf("heroes %d more %d, want %d and 3", len(v.Heroes), v.More, partyMaxHeroes)
	}
}

// The block lists the claimed heroes of the listed types with their facts, and
// who plays them only for a member. A GM-only field never reaches the page,
// and a Director previewing as a player gets a player's list.
func TestPartyBlockHandler(t *testing.T) {
	owner := "u2"
	typ := &EntityType{ID: 7, CampaignID: "c1", Slug: "pc", Name: "Player Character", Enabled: true, Fields: []FieldDefinition{
		{Key: "ancestry"}, {Key: "class", GMOnly: true}, {Key: "level"},
	}}
	var rolesSeen []int
	repo := &mockEntityRepo{listClaimedFn: func(_ context.Context, _ string, role int, _ string) ([]Entity, error) {
		rolesSeen = append(rolesSeen, role)
		return []Entity{
			{ID: "h1", CampaignID: "c1", EntityTypeID: 7, Name: "Ysolde Vane", OwnerUserID: &owner,
				FieldsData: map[string]any{"ancestry": "High Elf", "class": "Elementalist", "level": float64(3)}},
			{ID: "h2", CampaignID: "c1", EntityTypeID: 8, Name: "Not a hero type", OwnerUserID: &owner},
		}, nil
	}}
	typeRepo := &mockEntityTypeRepo{listByCampaignFn: func(context.Context, string) ([]EntityType, error) { return []EntityType{*typ}, nil }}
	build := func(addonOn bool) *Handler {
		h := NewHandler(NewEntityService(repo, typeRepo, &mockPermissionRepo{}))
		h.SetCharacterLists(&fakeCastLists{chars: []int{7}})
		h.SetMemberLister(rosterMembers{})
		h.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: addonOn}})
		return h
	}

	c, rec := advCtx(campaigns.RolePlayer, false, http.MethodGet, "/campaigns/c1/party")
	if err := build(true).PartyBlock(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{"Ysolde Vane", "High Elf", "Level 3", "played by Bo", "/campaigns/c1/entities/h1", "1 hero"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	for _, not := range []string{"Elementalist", "Not a hero type"} {
		if strings.Contains(body, not) {
			t.Errorf("body should not hold %q:\n%s", not, body)
		}
	}

	// The addon off: nothing, so the block is removed.
	c, rec = advCtx(campaigns.RolePlayer, false, http.MethodGet, "/campaigns/c1/party")
	if err := build(false).PartyBlock(c); err != nil || rec.Body.String() != "" {
		t.Errorf("addon off: err %v body %q", err, rec.Body.String())
	}

	// The owner previewing as a player is judged as a player.
	rolesSeen = nil
	c, _ = advCtx(campaigns.RoleOwner, false, http.MethodGet, "/campaigns/c1/party")
	c.Request().AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "1"})
	if err := build(true).PartyBlock(c); err != nil {
		t.Fatal(err)
	}
	if len(rolesSeen) == 0 {
		t.Fatal("the party was never listed")
	}
	for _, r := range rolesSeen {
		if r != int(campaigns.RolePlayer) {
			t.Errorf("previewing owner listed the party as role %d, want the player's", r)
		}
	}
}

// Your hero shows the viewer's own claimed hero and nobody else's, and a way
// to choose one when they have none; a visitor and a campaign that does not
// let players claim heroes get nothing, so the block is removed.
func TestYourHeroBlock(t *testing.T) {
	me, other := "u1", "u2"
	typ := EntityType{ID: 7, CampaignID: "c1", Slug: "pc", Name: "Player Character", Enabled: true, Fields: []FieldDefinition{
		{Key: "ancestry"}, {Key: "class", GMOnly: true}, {Key: "level"},
	}}
	heroes := func(mineToo bool) []Entity {
		out := []Entity{{ID: "h2", CampaignID: "c1", EntityTypeID: 7, Name: "Brannoch Ironvein", OwnerUserID: &other,
			FieldsData: map[string]any{"ancestry": "Dwarf", "level": float64(3)}}}
		if mineToo {
			out = append(out, Entity{ID: "h1", CampaignID: "c1", EntityTypeID: 7, Name: "Ysolde Vane", OwnerUserID: &me,
				FieldsData: map[string]any{"ancestry": "High Elf", "class": "Elementalist", "level": float64(3)}})
		}
		return out
	}
	build := func(mineToo, addonOn bool) *Handler {
		repo := &mockEntityRepo{listClaimedFn: func(context.Context, string, int, string) ([]Entity, error) { return heroes(mineToo), nil }}
		typeRepo := &mockEntityTypeRepo{listByCampaignFn: func(context.Context, string) ([]EntityType, error) { return []EntityType{typ}, nil }}
		h := NewHandler(NewEntityService(repo, typeRepo, &mockPermissionRepo{}))
		h.SetCharacterLists(&fakeCastLists{chars: []int{7}})
		h.SetMemberLister(rosterMembers{})
		h.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: addonOn}})
		return h
	}

	c, rec := advCtx(campaigns.RolePlayer, false, http.MethodGet, "/campaigns/c1/your-hero")
	if err := build(true, true).YourHeroBlock(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{"Ysolde Vane", "High Elf", "Level 3", "Open sheet", "/campaigns/c1/entities/h1"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	for _, not := range []string{"Brannoch", "Elementalist", "Choose a hero"} {
		if strings.Contains(body, not) {
			t.Errorf("body should not hold %q:\n%s", not, body)
		}
	}

	c, rec = advCtx(campaigns.RolePlayer, false, http.MethodGet, "/campaigns/c1/your-hero")
	if err := build(false, true).YourHeroBlock(c); err != nil {
		t.Fatal(err)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Choose a hero") || strings.Contains(body, "Brannoch") {
		t.Errorf("no hero of their own should offer a choice and show nobody else's:\n%s", body)
	}

	c, rec = advCtx(campaigns.RolePlayer, false, http.MethodGet, "/campaigns/c1/your-hero")
	if err := build(true, false).YourHeroBlock(c); err != nil || rec.Body.String() != "" {
		t.Errorf("addon off: err %v body %q", err, rec.Body.String())
	}
}
