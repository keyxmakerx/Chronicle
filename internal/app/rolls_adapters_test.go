package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
	"github.com/keyxmakerx/chronicle/internal/systems"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

type rollsBus struct{ msgs []*ws.Message }

func (b *rollsBus) Publish(m *ws.Message) { b.msgs = append(b.msgs, m) }

// TestRollsPublisherAudience runs each audience through the hub's own
// per-recipient gate, so a roll reaches exactly the sockets its mode allows.
func TestRollsPublisherAudience(t *testing.T) {
	owner := &ws.Client{UserID: "u-dm", Role: permissions.RoleOwner, Source: "browser"}
	coDM := &ws.Client{UserID: "u-co", Role: permissions.RolePlayer, IsDmGranted: true, Source: "browser"}
	roller := &ws.Client{UserID: "u-roller", Role: permissions.RolePlayer, Source: "browser"}
	other := &ws.Client{UserID: "u-other", Role: permissions.RolePlayer, Source: "browser"}
	foundry := &ws.Client{UserID: "u-dm", Role: permissions.RoleOwner, Source: foundry_vtt.ModuleSource}

	tests := []struct {
		name string
		a    rolls.Audience
		want map[*ws.Client]bool
		sent bool
	}{
		{"everyone", rolls.Audience{Everyone: true}, map[*ws.Client]bool{owner: true, coDM: true, roller: true, other: true, foundry: true}, true},
		{"directors only", rolls.Audience{DirectorsOnly: true}, map[*ws.Client]bool{owner: true, coDM: true, foundry: true}, true},
		{"directors and the roller", rolls.Audience{Users: []string{"u-roller"}}, map[*ws.Client]bool{owner: true, coDM: true, roller: true, foundry: true}, true},
		{"the roller alone", rolls.Audience{Users: []string{"u-roller"}, Strict: true}, map[*ws.Client]bool{roller: true}, true},
		{"nobody", rolls.Audience{}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &rollsBus{}
			(&rollsPublisher{bus: bus}).RollMade("c1", rolls.View{ID: "r1"}, tt.a)
			if !tt.sent {
				if len(bus.msgs) != 0 {
					t.Fatalf("sent %d messages to nobody", len(bus.msgs))
				}
				return
			}
			if len(bus.msgs) != 1 || bus.msgs[0].Type != ws.MsgRollMade || bus.msgs[0].ResourceID != "r1" {
				t.Fatalf("msgs = %+v", bus.msgs)
			}
			for _, c := range []*ws.Client{owner, coDM, roller, other, foundry} {
				if got := bus.msgs[0].DeliversTo(c); got != tt.want[c] {
					t.Errorf("%s (%s) receives = %v, want %v", c.UserID, c.Source, got, tt.want[c])
				}
			}
		})
	}
}

type stubEnabledSystem struct{ sys systems.System }

func (s stubEnabledSystem) EnabledSystem(context.Context, string) systems.System { return s.sys }

func TestRollsCritAdapterFallsBackToDefault(t *testing.T) {
	for name, a := range map[string]*rollsCritAdapter{
		"no system source":  {},
		"no enabled system": {sys: stubEnabledSystem{}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := a.CritRules(context.Background(), "c1"); !reflect.DeepEqual(got, systems.DefaultCritRules()) {
				t.Fatalf("got %v, want the default rules", got)
			}
		})
	}
}

type powerSystems struct{ sys systems.System }

func (p powerSystems) EnabledSystem(context.Context, string) systems.System { return p.sys }

type powerHeroes struct {
	entities.EntityService
	mine  []entities.Entity
	types map[int]*entities.EntityType
}

func (p powerHeroes) ListByOwner(_ context.Context, _, _ string) ([]entities.Entity, error) {
	return p.mine, nil
}

func (p powerHeroes) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	if t, ok := p.types[id]; ok {
		return t, nil
	}
	return nil, errors.New("no such type")
}

// powerCharacters lists the campaign's character types.
type powerCharacters []int

func (p powerCharacters) CharacterTypeIDs(context.Context, string) ([]int, error) { return p, nil }

// The Table's power roll comes from the game system's declaration and the
// viewer's own claimed hero; a campaign with none shows no line.
func TestRollsPowerAdapter(t *testing.T) {
	ctx := context.Background()
	decl := fakeSystem{manifest: &systems.SystemManifest{Crafting: &systems.CraftingRules{
		Dice: "2d10", Edge: 2, Bane: -2, Tiers: []int{11, 16},
		Characteristics: []systems.CraftingStat{{Key: "might", Label: "Might"}, {Key: "agility", Label: "Agility"}},
	}}}
	const heroType, noteType = 7, 9
	hero := entities.Entity{ID: "e-sivin", Name: "Sivin", EntityTypeID: heroType, FieldsData: map[string]any{"might": float64(1), "agility": "+2"}}
	plain := map[int]*entities.EntityType{heroType: {ID: heroType}}
	chars := powerCharacters{heroType}

	t.Run("no system", func(t *testing.T) {
		got, err := (&rollsPowerAdapter{sys: powerSystems{}}).PowerRoll(ctx, "c1", "u1", false)
		if err != nil || got.Declared {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	t.Run("a system with no declaration", func(t *testing.T) {
		got, _ := (&rollsPowerAdapter{sys: powerSystems{sys: fakeSystem{manifest: &systems.SystemManifest{}}}}).PowerRoll(ctx, "c1", "u1", false)
		if got.Declared {
			t.Fatalf("declared without a block: %+v", got)
		}
	})
	t.Run("a hero's numbers", func(t *testing.T) {
		a := &rollsPowerAdapter{sys: powerSystems{sys: decl}, entities: powerHeroes{mine: []entities.Entity{hero}, types: plain}, characters: chars}
		got, err := a.PowerRoll(ctx, "c1", "u1", false)
		if err != nil || !got.Declared || got.Hero != "Sivin" || got.HeroID != "e-sivin" || len(got.Characteristics) != 2 {
			t.Fatalf("got %+v, %v", got, err)
		}
		if v := got.Characteristics[0].Value; v == nil || *v != 1 {
			t.Errorf("might = %v", v)
		}
		if v := got.Characteristics[1].Value; v == nil || *v != 2 {
			t.Errorf("agility = %v", v)
		}
	})
	t.Run("no hero keeps the line without numbers", func(t *testing.T) {
		got, _ := (&rollsPowerAdapter{sys: powerSystems{sys: decl}, entities: powerHeroes{}, characters: chars}).PowerRoll(ctx, "c1", "u1", false)
		if !got.Declared || got.Hero != "" || got.Characteristics[0].Value != nil {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("the hero is the first owned page of a character type", func(t *testing.T) {
		note := entities.Entity{Name: "Shopping list", EntityTypeID: noteType, FieldsData: map[string]any{"might": float64(5)}}
		a := &rollsPowerAdapter{sys: powerSystems{sys: decl}, entities: powerHeroes{mine: []entities.Entity{note, hero}, types: plain}, characters: chars}
		got, _ := a.PowerRoll(ctx, "c1", "u1", false)
		if got.Hero != "Sivin" || got.Characteristics[0].Value == nil || *got.Characteristics[0].Value != 1 {
			t.Fatalf("picked the wrong page: %+v", got)
		}
	})
	// A house rule set off on one hero's page rolls that hero, when the
	// player owns it; otherwise their first hero.
	second := entities.Entity{ID: "e-orla", Name: "Orla", EntityTypeID: heroType, FieldsData: map[string]any{"might": float64(4)}}
	for _, tt := range []struct {
		name, heroID, want string
		might              int
	}{
		{"no page: the first hero", "", "Sivin", 1},
		{"a page they own: that hero", "e-orla", "Orla", 4},
		{"a page they don't own: the first hero", "e-someone-else", "Sivin", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := &rollsPowerAdapter{sys: powerSystems{sys: decl}, entities: powerHeroes{mine: []entities.Entity{hero, second}, types: plain}, characters: chars}
			got, err := a.PowerRollOn(ctx, "c1", "u1", tt.heroID, false)
			if err != nil || got.Hero != tt.want || got.Characteristics[0].Value == nil || *got.Characteristics[0].Value != tt.might {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}

	// A number the GM marked GM-only, on the category or on just this page,
	// never reaches a player; a Director still sees it.
	gmOnType := map[int]*entities.EntityType{heroType: {ID: heroType, Fields: []entities.FieldDefinition{{Key: "might", GMOnly: true}}}}
	onPage := hero
	onPage.FieldOverrides = &entities.FieldOverrides{Added: []entities.FieldDefinition{{Key: "might", GMOnly: true}}}
	for _, tt := range []struct {
		name      string
		hero      entities.Entity
		types     map[int]*entities.EntityType
		canSeeGM  bool
		wantMight bool
	}{
		{"GM-only on the category hides it from a player", hero, gmOnType, false, false},
		{"GM-only on the page hides it from a player", onPage, plain, false, false},
		{"a Director sees a GM-only category field", hero, gmOnType, true, true},
		{"a Director sees a GM-only page field", onPage, plain, true, true},
		{"an unreadable category hides everything from a player", hero, nil, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := &rollsPowerAdapter{sys: powerSystems{sys: decl}, entities: powerHeroes{mine: []entities.Entity{tt.hero}, types: tt.types}, characters: chars}
			got, err := a.PowerRoll(ctx, "c1", "u1", tt.canSeeGM)
			if err != nil || got.Hero != "Sivin" {
				t.Fatalf("got %+v, %v", got, err)
			}
			if has := got.Characteristics[0].Value != nil; has != tt.wantMight {
				t.Errorf("might shown = %v, want %v", has, tt.wantMight)
			}
			if tt.types != nil && got.Characteristics[1].Value == nil {
				t.Errorf("agility is not GM-only and should show")
			}
		})
	}
}

type rollAsPages map[string]*entities.Entity

func (p rollAsPages) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	if e, ok := p[id]; ok {
		return e, nil
	}
	return nil, apperror.NewNotFound("entity not found")
}

type failingPages struct{}

func (failingPages) GetByID(context.Context, string) (*entities.Entity, error) {
	return nil, errors.New("database is down")
}

// A Table roll may name only a live page of this campaign that the viewer
// has claimed, or any of them for a Director, since Foundry speaks as it; a
// public roll only a page every player may see.
func TestRollsCharacterAdapter(t *testing.T) {
	ctx := context.Background()
	const heroType, noteType = 7, 9
	owner := "u-kai"
	pages := rollAsPages{
		"hero-kai":   {ID: "hero-kai", CampaignID: "c1", EntityTypeID: heroType, OwnerUserID: &owner},
		"hero-free":  {ID: "hero-free", CampaignID: "c1", EntityTypeID: heroType},
		"note-kai":   {ID: "note-kai", CampaignID: "c1", EntityTypeID: noteType, OwnerUserID: &owner},
		"hero-other": {ID: "hero-other", CampaignID: "c2", EntityTypeID: heroType, OwnerUserID: &owner},
		"hero-priv":  {ID: "hero-priv", CampaignID: "c1", EntityTypeID: heroType, OwnerUserID: &owner, IsPrivate: true},
		"hero-cust":  {ID: "hero-cust", CampaignID: "c1", EntityTypeID: heroType, OwnerUserID: &owner, Visibility: entities.VisibilityCustom},
		"hero-open":  {ID: "hero-open", CampaignID: "c1", EntityTypeID: heroType, OwnerUserID: &owner, Visibility: entities.VisibilityDefault},
	}
	a := &rollsCharacterAdapter{entities: pages}
	kai := rolls.Viewer{UserID: owner}
	mara := rolls.Viewer{UserID: "u-mara"}
	dm := rolls.Viewer{UserID: "u-dm", Director: true}
	for _, tt := range []struct {
		name string
		v    rolls.Viewer
		id   string
		mode string
		want bool
	}{
		{"the hero's player", kai, "hero-kai", rolls.ModePublic, true},
		{"a page every player sees, in public", kai, "hero-open", rolls.ModePublic, true},
		{"a private page, in public", kai, "hero-priv", rolls.ModePublic, false},
		{"a private page, in public, by a Director", dm, "hero-priv", rolls.ModePublic, false},
		{"a private page, to the Directors", kai, "hero-priv", rolls.ModeGM, true},
		{"a private page, blind", kai, "hero-priv", rolls.ModeBlind, true},
		{"a page shared with some players, in public", dm, "hero-cust", rolls.ModePublic, false},
		{"a page shared with some players, to the Directors", dm, "hero-cust", rolls.ModeGM, true},
		{"another player", mara, "hero-kai", rolls.ModePublic, false},
		{"a Director, someone's hero", dm, "hero-kai", rolls.ModePublic, true},
		{"a Director, an unclaimed hero", dm, "hero-free", rolls.ModePublic, true},
		{"a player, an unclaimed hero", kai, "hero-free", rolls.ModePublic, false},
		{"a claimed page outside the characters list", kai, "note-kai", rolls.ModePublic, true},
		{"a hero of another campaign", dm, "hero-other", rolls.ModePublic, false},
		{"no such page", dm, "gone", rolls.ModePublic, false},
		{"signed out", rolls.Viewer{Director: true}, "hero-kai", rolls.ModePublic, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.MayRollAs(ctx, "c1", tt.v, tt.id, tt.mode)
			if err != nil || got != tt.want {
				t.Fatalf("got %v, %v; want %v", got, err, tt.want)
			}
		})
	}
	t.Run("a read failure is an error, not a yes", func(t *testing.T) {
		got, err := (&rollsCharacterAdapter{entities: failingPages{}}).MayRollAs(ctx, "c1", dm, "hero-kai", rolls.ModePublic)
		if err == nil || got {
			t.Fatalf("got %v, %v", got, err)
		}
	})
}
