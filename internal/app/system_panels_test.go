package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

type fakeSystem struct{ manifest *systems.SystemManifest }

func (f fakeSystem) Info() *systems.SystemManifest            { return f.manifest }
func (f fakeSystem) DataProvider() systems.DataProvider       { return nil }
func (f fakeSystem) TooltipRenderer() systems.TooltipRenderer { return nil }

type fakeEnabled struct{ sys systems.System }

func (f fakeEnabled) EnabledSystem(context.Context, string) systems.System { return f.sys }

type fakeTypes struct {
	types []entities.EntityType
	err   error
}

func (f fakeTypes) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return f.types, f.err
}

func TestSystemPanelResolver(t *testing.T) {
	cat := func(s string) *string { return &s }
	types := []entities.EntityType{
		{ID: 1, Slug: "character", PresetCategory: cat("character"), Enabled: true},
		{ID: 2, Slug: "location", Enabled: true},
		{ID: 3, Slug: "drawsteel-monster", Enabled: true},
		{ID: 4, Slug: "player-character", PresetCategory: cat(entities.PresetCategoryPlayerCharacter), Enabled: true},
	}
	withPanel := &systems.SystemManifest{
		ID: "drawsteel",
		EntityPanels: []systems.EntityPanelDef{
			{Widget: "negotiation", AppliesTo: systems.EntityPanelAppliesNPC},
		},
	}
	noPanel := &systems.SystemManifest{ID: "plain"}

	tests := []struct {
		name     string
		sys      systems.System
		typeID   int
		claimed  bool
		typesErr error
		want     []entities.SystemPanel
	}{
		{"npc type, system with panel", fakeSystem{withPanel}, 1, false, nil,
			[]entities.SystemPanel{{Widget: "negotiation", SystemID: "drawsteel"}}},
		{"system monster type", fakeSystem{withPanel}, 3, false, nil,
			[]entities.SystemPanel{{Widget: "negotiation", SystemID: "drawsteel"}}},
		{"location is not an npc", fakeSystem{withPanel}, 2, false, nil, nil},
		{"player character is not an npc", fakeSystem{withPanel}, 4, false, nil, nil},
		{"system without panels", fakeSystem{noPanel}, 1, false, nil, nil},
		{"no system enabled", nil, 1, false, nil, nil},
		{"claimed sheet of an npc type is a player character", fakeSystem{withPanel}, 1, true, nil, nil},
		{"type lookup fails", fakeSystem{withPanel}, 1, false, errors.New("db down"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolve := newSystemPanelResolver(fakeEnabled{tt.sys}, fakeTypes{types, tt.typesErr})
			var et *entities.EntityType
			for i := range types {
				if types[i].ID == tt.typeID {
					et = &types[i]
				}
			}
			got := resolve(context.Background(), "camp-1", et, tt.claimed)
			if len(got) != len(tt.want) {
				t.Fatalf("panels = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("panel %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
