package entities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// limitedPlan is a plan as a player gets it once the Directors limited the
// ancestry step (Fury kept off, Mothfolk ask-first) and required one kit.
func limitedPlan() *HeroPlan {
	return &HeroPlan{Steps: []HeroStep{
		{FieldKey: "ancestry", Label: "Ancestry",
			Choices: []HeroChoice{{Name: "Devil", Properties: ancestryProps()}, {Name: "Human"}, {Name: "Mothfolk", State: HeroChoiceStateAskFirst}},
			Held:    []HeroChoice{{Name: "Fury", Properties: ancestryProps()}},
			Rules:   &HeroStepRules{Limited: true, NotHereCount: 1, NotHere: []HeroNotHere{{Name: "Fury", Reason: "too much"}}}},
		{FieldKey: "kit", Label: "Kit", Choices: []HeroChoice{{Name: "Mountain"}},
			Rules: &HeroStepRules{Limited: true, Required: true, Prefilled: "Mountain"}},
		{FieldKey: "culture", Label: "Culture", Choices: []HeroChoice{{Name: "Forge"}, {Name: "Sea"}},
			Rules: &HeroStepRules{Limited: true, Required: true}},
		{FieldKey: "career", Label: "Career", Choices: []HeroChoice{{Name: "Sage"}}},
	}}
}

func TestOwnerWritesUnderRules(t *testing.T) {
	uid := "u1"
	held := &Entity{CampaignID: "c1", OwnerUserID: &uid, FieldsData: map[string]any{"ancestry": "Fury", "culture": "Forge"}}
	fresh := &Entity{CampaignID: "c1", OwnerUserID: &uid, FieldsData: map[string]any{}}
	types := []FieldDefinition{{Key: "ancestry"}, {Key: "kit"}, {Key: "culture"}, {Key: "career"}}
	picks := builderPicksFrom(limitedPlan())
	tests := []struct {
		name      string
		entity    *Entity
		patch     map[string]any
		refuseAsk bool
		wantErr   string
	}{
		{name: "H1 a typed name off the list is refused", entity: fresh, patch: map[string]any{"ancestry": "Fury"}, wantErr: "not on the ancestry list"},
		{name: "H1 a made-up name gets the same refusal", entity: fresh, patch: map[string]any{"ancestry": "Zzz"}, wantErr: "not on the ancestry list"},
		{name: "H1 a name on the list is fine", entity: fresh, patch: map[string]any{"ancestry": "human"}},
		{name: "H1 a step with no rule keeps its free text", entity: fresh, patch: map[string]any{"career": "Anything I like"}},
		{name: "H1 a limited step may still be cleared", entity: fresh, patch: map[string]any{"ancestry": nil}},
		{name: "H2 a required step can't be cleared", entity: held, patch: map[string]any{"culture": nil}, wantErr: "can't be left empty"},
		{name: "H2 a required step can't be blanked", entity: held, patch: map[string]any{"culture": "  "}, wantErr: "can't be left empty"},
		{name: "H2 a required step takes its list", entity: held, patch: map[string]any{"culture": "Sea"}},
		{name: "H2 a prefilled step takes only its one entry", entity: fresh, patch: map[string]any{"kit": "Other"}, wantErr: "not on the kit list"},
		{name: "H5 a grandfathered hero keeps its entry when it is re-sent", entity: held, patch: map[string]any{"ancestry": "Fury"}},
		{name: "H5 a grandfathered hero can't pick a kept-off entry anew", entity: fresh, patch: map[string]any{"ancestry": "Fury"}, wantErr: "not on the ancestry list"},
		{name: "H5 a grandfathered hero can still change what it bought",
			entity: held, patch: map[string]any{"ancestry_choices_json": `["Durable"]`}},
		{name: "H5 but not buy from a kept-off entry it is switching to",
			entity: fresh, patch: map[string]any{"ancestry": "Fury", "ancestry_choices_json": `["Durable"]`}, wantErr: "not on the ancestry list"},
		{name: "an ask-first entry passes the web door for the handler to record", entity: fresh, patch: map[string]any{"ancestry": "Mothfolk"}},
		{name: "an ask-first entry is refused to a player's AI", entity: fresh, patch: map[string]any{"ancestry": "Mothfolk"}, refuseAsk: true, wantErr: "ask your Director"},
		{name: "an ask-first entry the hero already holds is fine for the AI", entity: &Entity{CampaignID: "c1", OwnerUserID: &uid, FieldsData: map[string]any{"ancestry": "Mothfolk"}},
			patch: map[string]any{"ancestry": "Mothfolk"}, refuseAsk: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authorizeFieldsWriteAs(campaigns.RolePlayer, tt.entity, uid, "c1", false, tt.patch, types, picks, tt.refuseAsk)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected refusal: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

// The refusal for an off-list name is the same plain sentence whether or not
// an entry the player can't read has that name: nothing in it depends on the
// hidden entry, so it can't be used to look for one.
func TestOffListRefusalIsTheSameForHiddenNames(t *testing.T) {
	uid := "u1"
	e := &Entity{CampaignID: "c1", OwnerUserID: &uid, FieldsData: map[string]any{}}
	picks := builderPicksFrom(limitedPlan())
	types := []FieldDefinition{{Key: "ancestry"}}
	msg := func(v string) string {
		err := authorizeFieldsWrite(campaigns.RolePlayer, e, uid, "c1", false, map[string]any{"ancestry": v}, types, picks)
		if err == nil {
			t.Fatalf("%q was accepted", v)
		}
		return strings.ReplaceAll(err.Error(), v, "X")
	}
	if a, b := msg("SecretDirectorsEntry"), msg("NoSuchEntryAtAll"); a != b {
		t.Fatalf("refusals differ:\n%q\n%q", a, b)
	}
}

func TestOwnerFieldKindUnderRules(t *testing.T) {
	types := []FieldDefinition{{Key: "ancestry"}, {Key: "culture"}, {Key: "kit"}}
	empty := &HeroPlan{Steps: []HeroStep{
		{FieldKey: "ancestry", Label: "Ancestry", Choices: []HeroChoice{}, Rules: &HeroStepRules{Limited: true}},
		{FieldKey: "culture", Label: "Culture", Choices: []HeroChoice{{Name: "Forge"}}},
	}}
	if k, own := OwnerFieldKindOf("ancestry", types, empty); k != OwnerFieldChoice || own {
		t.Fatalf("a limited step with nothing offered = %v/%v, want a choice with no own text", k, own)
	}
	if k, own := OwnerFieldKindOf("culture", types, empty); k != OwnerFieldChoice || !own {
		t.Fatalf("an unlimited step = %v/%v, want a choice that also takes the player's own text", k, own)
	}
	if k, _ := OwnerFieldKindOf("kit", types, empty); k != OwnerFieldText {
		t.Fatalf("a step the plan doesn't list = %v, want free text as before", k)
	}
}

func TestTakeAskFirst(t *testing.T) {
	uid := "u1"
	picks := builderPicksFrom(limitedPlan())
	holding := &Entity{OwnerUserID: &uid, FieldsData: map[string]any{"ancestry": "Mothfolk"}}
	tests := []struct {
		name     string
		entity   *Entity
		patch    map[string]any
		wantAsks []string
		wantKeys []string
	}{
		{"an ask-first pick leaves the patch with what it would buy", &Entity{}, map[string]any{"ancestry": "Mothfolk", "ancestry_choices_json": `["x"]`, "career": "Sage"}, []string{"ancestry"}, []string{"career"}},
		{"an open pick stays", &Entity{}, map[string]any{"ancestry": "Human"}, nil, []string{"ancestry"}},
		{"a pick the hero already holds stays", holding, map[string]any{"ancestry": "mothfolk"}, nil, []string{"ancestry"}},
		{"a clear stays", &Entity{}, map[string]any{"ancestry": nil}, nil, []string{"ancestry"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asks := takeAskFirst(tt.entity, tt.patch, picks)
			var got []string
			for _, a := range asks {
				got = append(got, a.fieldKey)
			}
			if strings.Join(got, ",") != strings.Join(tt.wantAsks, ",") {
				t.Fatalf("asks = %v, want %v", got, tt.wantAsks)
			}
			var keys []string
			for k := range tt.patch {
				keys = append(keys, k)
			}
			if len(keys) != len(tt.wantKeys) {
				t.Fatalf("patch keys = %v, want %v", keys, tt.wantKeys)
			}
		})
	}
}

func TestBuildHeroFieldsUnderRules(t *testing.T) {
	plan := limitedPlan()
	tests := []struct {
		name     string
		picks    map[string]HeroPick
		want     map[string]any
		wantAsks []string
		wantErr  string
	}{
		{"a required step with one entry is filled in even if skipped",
			map[string]HeroPick{"culture": {Name: "Forge"}}, map[string]any{"culture": "Forge", "kit": "Mountain"}, nil, ""},
		{"a required short list can't be skipped",
			map[string]HeroPick{"kit": {Name: "Mountain"}}, nil, nil, "choose the culture; it can't be skipped"},
		{"a blank pick doesn't satisfy a required list",
			map[string]HeroPick{"culture": {}}, nil, nil, "can't be skipped"},
		{"a kept-off name is refused like any unknown name",
			map[string]HeroPick{"ancestry": {Name: "Fury"}, "culture": {Name: "Sea"}}, nil, nil, "not on the ancestry list"},
		{"an ask-first pick is not stored, and is asked for",
			map[string]HeroPick{"ancestry": {Name: "Mothfolk", Options: []string{"Durable"}}, "culture": {Name: "Sea"}},
			map[string]any{"culture": "Sea", "kit": "Mountain"}, []string{"ancestry"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, asks, err := buildHeroFields(plan, tt.picks)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("fields = %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("%s = %v, want %v", k, got[k], v)
				}
			}
			var names []string
			for _, a := range asks {
				names = append(names, a.FieldKey)
			}
			if strings.Join(names, ",") != strings.Join(tt.wantAsks, ",") {
				t.Fatalf("asks = %v, want %v", names, tt.wantAsks)
			}
		})
	}
}

type recordingRules struct {
	asked []HeroAsk
	fail  bool
}

func (r *recordingRules) RequestChoice(_ context.Context, in HeroAsk) (*HeroAskOutcome, error) {
	if r.fail {
		return nil, apperror.NewValidation("no")
	}
	r.asked = append(r.asked, in)
	return &HeroAskOutcome{ID: 5, FieldKey: in.FieldKey, EntryName: in.EntryName, Status: "waiting"}, nil
}

func (r *recordingRules) Breaks(context.Context, string, *Entity) ([]HeroBreak, error) { return nil, nil }

func (r *recordingRules) Waiting(context.Context, string, *Entity) ([]HeroWaitingPick, error) {
	return nil, nil
}

// An ask-first pick is answered with a request, never written to fields_data;
// the rest of the same write is saved.
func TestUpdateFieldsAPI_AskFirstNeverWritesTheField(t *testing.T) {
	owner := "user-owner"
	cases := []struct {
		name       string
		body       string
		failAsk    bool
		wantStatus int
		wantMerged map[string]any
		wantWrite  bool
		wantAsked  []string
	}{
		{"only the ask-first pick: nothing is written", `{"fields_patch":{"ancestry":"Mothfolk"}}`, false, 200, nil, false, []string{"ancestry"}},
		{"the rest of the patch is saved", `{"fields_patch":{"ancestry":"Mothfolk","career":"Sage"}}`, false, 200, map[string]any{"career": "Sage"}, true, []string{"ancestry"}},
		{"an open pick is written as before", `{"fields_patch":{"ancestry":"Human"}}`, false, 200, map[string]any{"ancestry": "Human"}, true, nil},
		{"a kept-off name is refused and writes nothing", `{"fields_patch":{"ancestry":"Fury"}}`, false, 400, nil, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &identityStubSvc{entity: &Entity{ID: "e1", CampaignID: "c1", Name: "Hero", OwnerUserID: &owner}}
			rules := &recordingRules{fail: tc.failAsk}
			h := &Handler{service: svc, heroPlanner: &stubPlanner{plan: limitedPlan()}, heroRules: rules}
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}, {Name: "eid", Value: "e1"}})
			c.Set("auth_user_id", owner)
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer})
			err := h.UpdateFieldsAPI(c)
			wantStatus(t, err, tc.wantStatus)
			if (svc.writeCount > 0) != tc.wantWrite {
				t.Fatalf("write reached = %v, want %v", svc.writeCount > 0, tc.wantWrite)
			}
			if tc.wantWrite {
				if len(svc.merged) != len(tc.wantMerged) {
					t.Fatalf("merged = %v, want %v", svc.merged, tc.wantMerged)
				}
				for k, v := range tc.wantMerged {
					if svc.merged[k] != v {
						t.Errorf("merged %s = %v, want %v", k, svc.merged[k], v)
					}
				}
			}
			var got []string
			for _, a := range rules.asked {
				got = append(got, a.FieldKey)
				if a.UserID != owner || a.EntityID != "e1" {
					t.Errorf("request = %+v, want the owner and the hero", a)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.wantAsked, ",") {
				t.Fatalf("asked = %v, want %v", got, tc.wantAsked)
			}
			if tc.wantStatus == 200 && len(tc.wantAsked) > 0 {
				var body struct {
					Requests []HeroAskOutcome `json:"requests"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Requests) != 1 || body.Requests[0].Status != "waiting" {
					t.Fatalf("response = %s, want the waiting outcome", rec.Body.String())
				}
			}
		})
	}
}

type notesRules struct{ recordingRules }

func (notesRules) Breaks(context.Context, string, *Entity) ([]HeroBreak, error) {
	return []HeroBreak{{FieldKey: "ancestry", Value: "Fury", Kind: "not_offered"}}, nil
}

func (notesRules) Waiting(context.Context, string, *Entity) ([]HeroWaitingPick, error) {
	return []HeroWaitingPick{{ID: 1, FieldKey: "culture", EntryName: "Sea"}}, nil
}

// The hero page hears about broken rules and waiting blanks only for the
// player it belongs to and the Directors, and only for a hero someone plays.
func TestAddHeroRuleNotes(t *testing.T) {
	owner := "u1"
	played := &Entity{ID: "e", OwnerUserID: &owner}
	tests := []struct {
		name    string
		h       *Handler
		e       *Entity
		allowed bool
		want    bool
	}{
		{"the owner or a Director gets both", &Handler{heroRules: notesRules{}}, played, true, true},
		{"anyone else gets neither", &Handler{heroRules: notesRules{}}, played, false, false},
		{"a hero nobody plays has no player's rules", &Handler{heroRules: notesRules{}}, &Entity{ID: "e"}, true, false},
		{"without rules wired nothing is added", &Handler{}, played, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := map[string]any{}
			tt.h.addHeroRuleNotes(context.Background(), resp, "c1", tt.e, tt.allowed)
			_, breaks := resp["choice_breaks"]
			_, waiting := resp["choice_waiting"]
			if breaks != tt.want || waiting != tt.want {
				t.Fatalf("response = %v, want notes %v", resp, tt.want)
			}
		})
	}
}
