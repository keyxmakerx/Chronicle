package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeCraftingFights is crafting as the fight listener reaches it.
type fakeCraftingFights struct {
	crafting.Service
	fights  []crafting.Fight
	viewers []crafting.Viewer
	spoils  int
}

func (f *fakeCraftingFights) FightEnded(_ context.Context, _ string, v crafting.Viewer, fight crafting.Fight) (*crafting.FightSpoils, error) {
	f.fights, f.viewers = append(f.fights, fight), append(f.viewers, v)
	return &crafting.FightSpoils{Lines: []string{"Troll left spoils."}, Spoils: f.spoils}, nil
}

func (f *fakeCraftingFights) DropNotes(_ context.Context, _ string, v crafting.Viewer, ids []string) (map[string]string, error) {
	f.viewers = append(f.viewers, v)
	return map[string]string{ids[0]: "Has a drop list"}, nil
}

func TestCraftingFightListener(t *testing.T) {
	ctx := context.Background()
	dm := encounters.Viewer{UserID: "dm", VisibilityRole: permissions.RoleOwner}
	fight := encounters.EndedFight{Name: "Shrine cellar", Creatures: []encounters.EndedCreature{
		{EntityID: "troll", Name: "Troll", Copies: 2, Down: 2}, {Name: "Rats", Copies: 3, Down: 1}}}

	t.Run("a fight that left spoils points at the tray", func(t *testing.T) {
		svc := &fakeCraftingFights{spoils: 2}
		rep, err := (&craftingFightListener{svc: svc}).FightEnded(ctx, "camp 1", dm, fight)
		if err != nil || rep.Link != "/campaigns/camp%201/crafting#spoils" || rep.LinkText == "" || rep.Note == "" || len(rep.Lines) != 1 {
			t.Fatalf("report %+v, %v", rep, err)
		}
		want := crafting.Fight{Name: "Shrine cellar", Creatures: []crafting.FightCreature{
			{PageID: "troll", Name: "Troll", Down: 2, Copies: 2}, {Name: "Rats", Down: 1, Copies: 3}}}
		if !reflect.DeepEqual(svc.fights, []crafting.Fight{want}) {
			t.Errorf("heard %+v", svc.fights)
		}
		// The Fight tab is the DM team's, which is crafting's Director.
		if v := svc.viewers[0]; !v.Director || v.UserID != "dm" || v.VisibilityRole != permissions.RoleOwner {
			t.Errorf("as %+v", v)
		}
	})
	t.Run("a fight that left nothing has no link", func(t *testing.T) {
		rep, err := (&craftingFightListener{svc: &fakeCraftingFights{}}).FightEnded(ctx, "camp", dm, fight)
		if err != nil || rep.Link != "" || rep.Note != "" || len(rep.Lines) != 1 {
			t.Errorf("report %+v, %v", rep, err)
		}
	})
	t.Run("crafting off hears nothing", func(t *testing.T) {
		svc := &fakeCraftingFights{spoils: 1}
		l := &craftingFightListener{svc: svc, enabled: func(context.Context, string) bool { return false }}
		if rep, err := l.FightEnded(ctx, "camp", dm, fight); err != nil || rep != nil {
			t.Errorf("report %+v, %v", rep, err)
		}
		if notes, err := l.Leaves(ctx, "camp", dm, []string{"troll"}); err != nil || notes != nil {
			t.Errorf("notes %v, %v", notes, err)
		}
		if len(svc.fights)+len(svc.viewers) != 0 {
			t.Error("crafting was asked")
		}
	})
	t.Run("leaves", func(t *testing.T) {
		svc := &fakeCraftingFights{}
		notes, err := (&craftingFightListener{svc: svc}).Leaves(ctx, "camp", encounters.Viewer{UserID: "p", VisibilityRole: permissions.RolePlayer}, []string{"troll"})
		if err != nil || notes["troll"] != "Has a drop list" || svc.viewers[0].Director {
			t.Errorf("notes %v, %v, as %+v", notes, err, svc.viewers)
		}
	})
}

// fakeCraftingPages is the entities plugin as the pages adapter reads it.
type fakeCraftingPages struct {
	entities.EntityService
	pages map[string]*entities.Entity
	sees  map[string]bool
}

func (f *fakeCraftingPages) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	if e, ok := f.pages[id]; ok {
		return e, nil
	}
	return nil, apperror.NewNotFound("entity not found")
}

func (f *fakeCraftingPages) CheckEntityAccess(_ context.Context, id string, role int, _ string) (*entities.EffectivePermission, error) {
	return &entities.EffectivePermission{CanView: role >= permissions.RoleOwner || f.sees[id]}, nil
}

func TestCraftingPagesAdapter(t *testing.T) {
	ctx := context.Background()
	a := &craftingPagesAdapter{entities: &fakeCraftingPages{
		pages: map[string]*entities.Entity{
			"troll":  {ID: "troll", CampaignID: "camp", Name: "Troll", TypeName: "Creature"},
			"shrine": {ID: "shrine", CampaignID: "camp", Name: "The Weeping Shrine"},
			"far":    {ID: "far", CampaignID: "elsewhere", Name: "Far away"},
		},
		sees: map[string]bool{"troll": true, "far": true},
	}}
	for _, c := range []struct {
		id   string
		want *crafting.PageInfo
	}{
		{"troll", &crafting.PageInfo{ID: "troll", Name: "Troll", Kind: "Creature"}},
		{"shrine", &crafting.PageInfo{ID: "shrine", Name: "The Weeping Shrine", Kind: "Page"}},
		{"far", nil},
		{"gone", nil},
		{"", nil},
	} {
		got, err := a.Page(ctx, "camp", c.id)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("page %q = %+v, %v", c.id, got, err)
		}
	}
	for _, c := range []struct {
		id   string
		role int
		want bool
	}{
		{"troll", permissions.RolePlayer, true},
		{"shrine", permissions.RolePlayer, false},
		{"shrine", permissions.RoleOwner, true},
		{"far", permissions.RolePlayer, false},
		{"gone", permissions.RoleOwner, false},
	} {
		if got, err := a.CanSee(ctx, "camp", c.id, c.role, "u"); err != nil || got != c.want {
			t.Errorf("can see %q as %d = %v, %v", c.id, c.role, got, err)
		}
	}
}
