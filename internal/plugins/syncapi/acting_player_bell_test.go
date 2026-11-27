package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// changeBell records the changes each hero bell rang with.
type changeBell struct{ rang [][]entities.HeroPickChange }

func (b *changeBell) HeroChanged(_ context.Context, _ *entities.Entity, _ string, ch []entities.HeroPickChange) error {
	b.rang = append(b.rang, ch)
	return nil
}

func (b *changeBell) LevelRaised(context.Context, *entities.Entity, string, int) error { return nil }

// storingEntitySvc is a claimed hero whose stored fields change as writes
// land, so a second push meets what the first one stored.
type storingEntitySvc struct {
	actingEntitySvc
	fields     map[string]any
	typeFields []entities.FieldDefinition
}

// GetByID makes the hero old enough that a rule of a few days has run out.
func (s *storingEntitySvc) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	owner := "pat"
	cp := map[string]any{}
	for k, v := range s.fields {
		cp[k] = v
	}
	return &entities.Entity{ID: id, CampaignID: "camp-1", EntityTypeID: 1, OwnerUserID: &owner, Name: "Tess",
		CreatedAt: time.Now().Add(-90 * 24 * time.Hour), FieldsData: cp}, nil
}

func (s *storingEntitySvc) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	return &entities.EntityType{ID: id, CampaignID: "camp-1", Fields: s.typeFields}, nil
}

func (s *storingEntitySvc) MergeFields(_ context.Context, _ string, p map[string]any) error {
	s.merges = append(s.merges, p)
	s.fields = entities.MergeFieldPatch(s.fields, p)
	return nil
}

type shutRules struct{}

func (shutRules) HeroChangeRule(context.Context, string) (*campaigns.HeroChangeRule, error) {
	return &campaigns.HeroChangeRule{AfterMakingDays: 3}, nil
}

func (shutRules) SaveHeroChangeRule(context.Context, string, *campaigns.HeroChangeRule) error {
	return nil
}

func bellFields() []entities.FieldDefinition {
	return []entities.FieldDefinition{
		{Key: "class", Type: "text"}, {Key: "stamina", Type: "number"}, {Key: "pronouns", Type: "text"},
		{Key: "backstory", Label: "Backstory", Type: "textarea"}, {Key: "perks_json", Type: "text"},
		{Key: "skills_json", Type: "text"}, {Key: "gm_notes", Type: "text", GMOnly: true},
		{Key: "victories", Label: "Victories", Type: "number", Play: &entities.FieldPlay{Edit: "owner", Kind: "counter", Bell: true}},
	}
}

func pushBody(t *testing.T, fields map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"actingUserId": "pat", "fields_data": fields})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A player's Foundry edit of a list, a story or an identity text rings the
// bell, whether the hero's picks are open or shut; a Director's does not; and
// a locked pick is held as before while the rest is stored and rings.
func TestUpdateEntityFields_ActingPlayerBell(t *testing.T) {
	roles := map[string]campaigns.Role{"u-1": campaigns.RoleOwner, "pat": campaigns.RolePlayer, "dir": campaigns.RoleOwner}
	plan := &entities.HeroPlan{Steps: []entities.HeroStep{{FieldKey: "class", Label: "Class",
		Choices: []entities.HeroChoice{{Name: "Fury"}, {Name: "Censor"}}}}}
	start := func() map[string]any {
		return map[string]any{"class": "Fury", "stamina": float64(20), "pronouns": "she/her", "backstory": "Old tale",
			"skills_json": `["Sneak"]`, "perks_json": `[{"name":"Lucky","description":"x"}]`, "victories": float64(2)}
	}
	tests := []struct {
		name      string
		shut      bool
		body      string
		wantBells int
		wantHeld  []string
		wantStore bool
	}{
		{"a list while picks are open", false, `{"actingUserId":"pat","fields_data":{"skills_json":"[\"Sneak\",\"Alchemy\"]"}}`, 1, nil, true},
		{"a list while picks are shut", true, `{"actingUserId":"pat","fields_data":{"skills_json":"[\"Sneak\",\"Alchemy\"]"}}`, 1, nil, true},
		{"a story while picks are shut", true, `{"actingUserId":"pat","fields_data":{"backstory":"A new tale"}}`, 1, nil, true},
		{"an identity text", true, `{"actingUserId":"pat","fields_data":{"pronouns":"they/them"}}`, 1, nil, true},
		{"a locked pick is held, a list beside it stored and rung", true, `{"actingUserId":"pat","fields_data":{"class":"Censor","skills_json":"[]"}}`, 1, []string{"class"}, true},
		{"a locked pick alone rings nothing", true, `{"actingUserId":"pat","fields_data":{"class":"Censor"}}`, 0, []string{"class"}, false},
		{"a number rings nothing", true, `{"actingUserId":"pat","fields_data":{"stamina":30}}`, 0, nil, true},
		{"a number the system asks the bell for rings", true, `{"actingUserId":"pat","fields_data":{"victories":3}}`, 1, nil, true},
		{"that number resent unchanged rings nothing", true, `{"actingUserId":"pat","fields_data":{"victories":2}}`, 0, nil, true},
		{"a Director's own edit rings nothing", true, `{"actingUserId":"dir","fields_data":{"skills_json":"[]","backstory":"x"}}`, 0, nil, true},
		{"the plain sync rings nothing", true, `{"fields_data":{"skills_json":"[]","backstory":"x"}}`, 0, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := &storingEntitySvc{fields: start(), typeFields: bellFields()}
			bell := &changeBell{}
			h := NewAPIHandler(&stubSyncSvcForRole{}, ents, actingCampaignSvc{roles: roles}, nil)
			h.SetHeroPlanner(fixedPlan{plan})
			var rules entities.HeroChangeRules
			if tt.shut {
				rules = shutRules{}
			}
			h.SetHeroChanges(entities.NewHeroChangeService(rules, nil, bell))
			c, rec := gateContext(http.MethodPut, tt.body)
			if err := h.UpdateEntityFields(c); err != nil {
				t.Fatalf("write refused: %v", err)
			}
			if (len(ents.merges) > 0) != tt.wantStore {
				t.Fatalf("stored %v, want stored=%v", ents.merges, tt.wantStore)
			}
			if len(bell.rang) != tt.wantBells {
				t.Fatalf("bells = %d, want %d", len(bell.rang), tt.wantBells)
			}
			var resp struct{ Held []struct{ Key string } }
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
			var held []string
			for _, k := range resp.Held {
				held = append(held, k.Key)
			}
			if strings.Join(held, ",") != strings.Join(tt.wantHeld, ",") {
				t.Fatalf("held %v, want %v", held, tt.wantHeld)
			}
			for _, ch := range bell.rang {
				for _, c := range ch {
					if c.FieldKey == "class" {
						t.Fatalf("a held pick rang: %+v", c)
					}
				}
			}
		})
	}
}

// Foundry resends the whole mapped set on every sync. The second identical
// push by an acting player rings nothing for any kind of key, with realistic
// values: perks as a list of objects and a long backstory.
func TestUpdateEntityFields_ActingPlayerResendRingsNothing(t *testing.T) {
	roles := map[string]campaigns.Role{"u-1": campaigns.RoleOwner, "pat": campaigns.RolePlayer}
	backstory := strings.Repeat("The ash fell on Wode for nine winters, and \"nobody\" came. ", 150) + "\n\nShe left at dawn."
	mapped := func() map[string]any {
		var perks any
		if err := json.Unmarshal([]byte(`[{"name":"Lucky","description":"Once a day, reroll a die.","uses":1},{"name":"Tough","description":"+2 stamina"}]`), &perks); err != nil {
			t.Fatal(err)
		}
		return map[string]any{
			"class": "Fury", "stamina": float64(20), "pronouns": "she/her", "backstory": backstory,
			"skills_json": `["Sneak","Pick Lock"]`, "perks_json": perks,
		}
	}
	// The hero starts with older values, so the first push changes everything.
	ents := &storingEntitySvc{typeFields: bellFields(), fields: map[string]any{
		"class": "Fury", "stamina": float64(15), "pronouns": "they/them", "backstory": "Old tale",
		"skills_json": `["Sneak"]`, "perks_json": `["Lucky"]`}}
	bell := &changeBell{}
	h := NewAPIHandler(&stubSyncSvcForRole{}, ents, actingCampaignSvc{roles: roles}, nil)
	h.SetHeroPlanner(fixedPlan{&entities.HeroPlan{Steps: []entities.HeroStep{{FieldKey: "class", Label: "Class",
		Choices: []entities.HeroChoice{{Name: "Fury"}}}}}})
	h.SetHeroChanges(entities.NewHeroChangeService(nil, nil, bell))

	push := func() {
		t.Helper()
		c, _ := gateContext(http.MethodPut, pushBody(t, mapped()))
		if err := h.UpdateEntityFields(c); err != nil {
			t.Fatalf("write refused: %v", err)
		}
	}
	push()
	if len(bell.rang) != 1 || len(bell.rang[0]) != 4 {
		t.Fatalf("the first push changes pronouns, backstory, skills and perks: %+v", bell.rang)
	}
	for _, c := range bell.rang[0] {
		if c.FieldKey == "backstory" && (c.From != "" || c.To != "") {
			t.Fatalf("the bell carries story text: %+v", c)
		}
	}
	push()
	push()
	if len(bell.rang) != 1 {
		t.Fatalf("an identical push rang again: %d bells", len(bell.rang))
	}
}
