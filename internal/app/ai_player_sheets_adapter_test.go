package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// sheetEntities is the entities service as the player sheets door sees it:
// Tyne claimed by player-1, Brann claimed by player-2, and Mira, claimed by
// player-1 but hidden from her by the GM.
type sheetEntities struct {
	ents   map[string]*entities.Entity
	hidden map[string]bool
	merged map[string]map[string]any
	source changesource.Source
}

func newSheetEntities() *sheetEntities {
	own := func(u string) *string { return &u }
	return &sheetEntities{
		ents: map[string]*entities.Entity{
			"tyne":      {ID: "tyne", CampaignID: "camp-1", Name: "Tyne", EntityTypeID: 1, OwnerUserID: own("player-1"), FieldsData: map[string]any{"ancestry": "Elf", "stamina": "30"}},
			"brann":     {ID: "brann", CampaignID: "camp-1", Name: "Brann", EntityTypeID: 1, OwnerUserID: own("player-2")},
			"mira":      {ID: "mira", CampaignID: "camp-1", Name: "Mira", EntityTypeID: 1, OwnerUserID: own("player-1")},
			"elsewhere": {ID: "elsewhere", CampaignID: "camp-2", Name: "Tyne", EntityTypeID: 1, OwnerUserID: own("player-1")},
		},
		hidden: map[string]bool{"mira": true},
		merged: map[string]map[string]any{},
	}
}

func (s *sheetEntities) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	if e, ok := s.ents[id]; ok {
		cp := *e
		return &cp, nil
	}
	return nil, apperror.NewNotFound("entity not found")
}

func (s *sheetEntities) ListClaimed(_ context.Context, campaignID string, _ int, _ string) ([]entities.Entity, error) {
	var out []entities.Entity
	for _, id := range []string{"tyne", "brann", "mira", "elsewhere"} {
		if e := s.ents[id]; e.CampaignID == campaignID {
			out = append(out, *e)
		}
	}
	return out, nil
}

func (s *sheetEntities) GetEntityTypeByID(context.Context, int) (*entities.EntityType, error) {
	return &entities.EntityType{ID: 1, Name: "Character", Fields: []entities.FieldDefinition{
		{Key: "class"}, {Key: "stamina"}, {Key: "ancestry"}, {Key: "secret_past", GMOnly: true},
	}}, nil
}

func (s *sheetEntities) CheckEntityAccess(_ context.Context, id string, _ int, _ string) (*entities.EffectivePermission, error) {
	return &entities.EffectivePermission{CanView: !s.hidden[id]}, nil
}

func (s *sheetEntities) MergeFields(ctx context.Context, id string, patch map[string]any) error {
	s.merged[id] = patch
	s.source, _ = changesource.From(ctx)
	return nil
}

type sheetPlanner struct{}

func (sheetPlanner) HeroPlan(context.Context, string, string, bool) (*entities.HeroPlan, error) {
	return &entities.HeroPlan{Steps: []entities.HeroStep{{FieldKey: "class", Choices: []entities.HeroChoice{{Name: "Fury"}, {Name: "Tactician"}}}}}, nil
}

// A player's AI gets exactly the sheet editor's door for a claimed owner:
// identity text and builder picks on their own character, nothing else.
func TestAIPlayerSheets_PatchSheet(t *testing.T) {
	tests := []struct {
		name, user, id string
		patch          map[string]any
		ok             bool
	}{
		{"identity text on own character", "player-1", "tyne", map[string]any{"ancestry": "Dwarf"}, true},
		{"a builder pick from the list", "player-1", "tyne", map[string]any{"class": "fury"}, true},
		{"a builder pick not on the list", "player-1", "tyne", map[string]any{"class": "Archmage"}, false},
		{"a number the Director keeps", "player-1", "tyne", map[string]any{"stamina": "99"}, false},
		{"a GM-only field", "player-1", "tyne", map[string]any{"secret_past": "x"}, false},
		{"a number, not text", "player-1", "tyne", map[string]any{"ancestry": 3}, false},
		{"someone else's character", "player-1", "brann", map[string]any{"ancestry": "x"}, false},
		{"own character the GM hid", "player-1", "mira", map[string]any{"ancestry": "x"}, false},
		{"own character in another campaign", "player-1", "elsewhere", map[string]any{"ancestry": "x"}, false},
		{"empty patch", "player-1", "tyne", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := newSheetEntities()
			door := aiPlayerSheets{entities: ents, planner: sheetPlanner{}}
			err := door.PatchSheet(context.Background(), "camp-1", tt.user, tt.id, tt.patch, "Claude", true)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
			if !tt.ok {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code >= 500 {
					t.Errorf("refusal is not a plain answer: %v", err)
				}
				if len(ents.merged) != 0 {
					t.Fatalf("a refused change was written: %+v", ents.merged)
				}
				return
			}
			if ents.merged[tt.id] == nil {
				t.Fatal("an allowed change was not written")
			}
			if ents.source.Kind != changesource.KindAI || ents.source.UserID != tt.user || ents.source.Label != "Claude" {
				t.Errorf("written as %+v, want the player's AI", ents.source)
			}
		})
	}
}

// A check only checks.
func TestAIPlayerSheets_CheckDoesNotWrite(t *testing.T) {
	ents := newSheetEntities()
	door := aiPlayerSheets{entities: ents, planner: sheetPlanner{}}
	if err := door.PatchSheet(context.Background(), "camp-1", "player-1", "tyne", map[string]any{"ancestry": "x"}, "", false); err != nil {
		t.Fatal(err)
	}
	if len(ents.merged) != 0 {
		t.Fatal("a check wrote the sheet")
	}
}

// The prompt lists only the player's own characters they can open, with no
// GM-only key and the builder's names.
func TestAIPlayerSheets_Sheets(t *testing.T) {
	door := aiPlayerSheets{entities: newSheetEntities(), planner: sheetPlanner{}}
	got, err := door.Sheets(context.Background(), "camp-1", "player-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "tyne" {
		t.Fatalf("sheets = %+v, want only Tyne", got)
	}
	s := got[0]
	if s.Values["ancestry"] != "Elf" || s.Type != "Character" || len(s.Picks["class"]) != 2 {
		t.Errorf("sheet = %+v", s)
	}
	if _, ok := s.Values["stamina"]; ok {
		t.Error("a Director's number is offered to the player's AI")
	}
	if _, ok := s.Values["secret_past"]; ok {
		t.Error("a GM-only field is offered to the player's AI")
	}
}

// The brief offers only fields the character has, each once by the rule the
// door applies, and shows the rest of what the player sees to recommend from.
func TestPlayerSheetOf(t *testing.T) {
	own := "p1"
	e := &entities.Entity{ID: "e1", Name: "Korr", OwnerUserID: &own, FieldsData: map[string]any{
		"ancestry": "Devil", "might": float64(2), "gm_notes": "secret", "backstory": "Forge-born.",
	}}
	fields := []entities.FieldDefinition{{Key: "ancestry"}, {Key: "class"}, {Key: "might"}, {Key: "backstory"},
		{Key: "skills_json"}, {Key: "gm_notes", GMOnly: true}}
	plan := &entities.HeroPlan{Steps: []entities.HeroStep{
		{FieldKey: "ancestry", Choices: []entities.HeroChoice{{Name: "Devil", Properties: map[string]any{
			"ancestry_points": float64(3), "traits": []any{map[string]any{"name": "Wings", "cost": float64(2)}}}}}},
		{FieldKey: "class", Choices: []entities.HeroChoice{{Name: "Shadow"}}},
		{FieldKey: "career", Choices: []entities.HeroChoice{{Name: "Sage"}}},
	}}
	s := playerSheetOf(e, "Character", fields, plan)
	if len(s.Free) != 0 || len(s.Picks) != 2 || !s.Own["ancestry"] || s.Own["class"] {
		t.Fatalf("free %v picks %v own %v: want ancestry and class as choices only, ancestry also free", s.Free, s.Picks, s.Own)
	}
	if _, ok := s.Picks["career"]; ok {
		t.Error("a step the character has no field for is offered")
	}
	if len(s.Story) != 1 || len(s.Lists) != 1 {
		t.Errorf("story %v lists %v", s.Story, s.Lists)
	}
	if b := s.Buys["ancestry_choices_json"]; b.Entry != "Devil" || b.Budget != 3 || len(b.Names) != 1 {
		t.Errorf("buys = %+v", s.Buys)
	}
	if s.Shown["might"] != "2" {
		t.Errorf("shown = %v, want might to read from", s.Shown)
	}
	if _, ok := s.Shown["gm_notes"]; ok {
		t.Error("a GM-only field reached the player's AI")
	}
}
