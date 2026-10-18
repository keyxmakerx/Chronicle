package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// identityStubSvc records the writes the two routes reach, so a test can say
// both "was refused" and "never touched the store".
type identityStubSvc struct {
	EntityService
	entity     *Entity
	merged     map[string]any
	replaced   bool
	updateIn   *UpdateEntityInput
	writeCount int
	hidden     bool // the caller can no longer view the page
}

func (s *identityStubSvc) CheckEntityAccess(_ context.Context, _ string, _ int, _ string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: !s.hidden}, nil
}

func (s *identityStubSvc) GetByID(_ context.Context, _ string) (*Entity, error) {
	e := *s.entity
	return &e, nil
}

// The type marks "background" GM-only, so the allowlist alone is not enough.
func (s *identityStubSvc) GetEntityTypeByID(_ context.Context, _ int) (*EntityType, error) {
	return &EntityType{Fields: []FieldDefinition{{Key: "ancestry"}, {Key: "background", GMOnly: true}}}, nil
}

func (s *identityStubSvc) MergeFields(_ context.Context, _ string, p map[string]any) error {
	s.merged = p
	s.writeCount++
	return nil
}

func (s *identityStubSvc) UpdateFields(_ context.Context, _ string, _ map[string]any) error {
	s.replaced = true
	s.writeCount++
	return nil
}

func (s *identityStubSvc) Update(_ context.Context, _ string, in UpdateEntityInput) (*Entity, error) {
	s.updateIn = &in
	s.writeCount++
	e := *s.entity
	return &e, nil
}

func runIdentityCall(t *testing.T, svc *identityStubSvc, handler func(*Handler, echo.Context) error, role campaigns.Role, userID, campaignID, body string) error {
	t.Helper()
	return runIdentityCallOn(t, &Handler{service: svc}, handler, role, userID, campaignID, body)
}

func runIdentityCallOn(t *testing.T, h *Handler, handler func(*Handler, echo.Context) error, role campaigns.Role, userID, campaignID, body string) error {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c := e.NewContext(req, httptest.NewRecorder())
	c.SetParamNames("id", "eid")
	c.SetParamValues(campaignID, "e1")
	c.Set("auth_user_id", userID)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: campaignID},
		MemberRole: role,
	})
	return handler(h, c)
}

func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	if status == http.StatusOK {
		if err != nil {
			t.Fatalf("want success, got %v", err)
		}
		return
	}
	assertAppError(t, err, status)
}

func TestUpdateFieldsAPI_ClaimedOwnerIdentityOnly(t *testing.T) {
	owner := "user-owner"
	cases := []struct {
		name       string
		role       campaigns.Role
		userID     string
		campaignID string
		body       string
		wantStatus int
		wantWrite  bool
	}{
		{"owner allowed for allowlisted key", campaigns.RolePlayer, owner, "c1", `{"fields_patch":{"ancestry":"Dwarf"}}`, 200, true},
		{"owner allowed for every allowlisted key at once", campaigns.RolePlayer, owner, "c1",
			`{"fields_patch":{"ancestry":"a","culture":"b","career":"c","kit":"d","race":"e","species":"f"}}`, 200, true},
		{"owner denied an allowlisted key the type marks GM-only", campaigns.RolePlayer, owner, "c1", `{"fields_patch":{"background":"x"}}`, 403, false},
		{"owner denied a non-text value", campaigns.RolePlayer, owner, "c1", `{"fields_patch":{"ancestry":{"a":1}}}`, 400, false},
		{"owner denied a number", campaigns.RolePlayer, owner, "c1", `{"fields_patch":{"kit":5}}`, 400, false},
		{"owner denied an overlong value", campaigns.RolePlayer, owner, "c1", `{"fields_patch":{"kit":"` + strings.Repeat("x", 201) + `"}}`, 400, false},
		{"owner denied a key a page override marks GM-only", campaigns.RolePlayer, owner, "c1", `{"fields_patch":{"heritage":"x"}}`, 403, false},
		{"scribe may set a GM-only key", campaigns.RoleScribe, "user-scribe", "c1", `{"fields_patch":{"background":"x"}}`, 200, true},
		{"owner may clear an allowlisted key", campaigns.RolePlayer, owner, "c1", `{"fields_patch":{"kit":null}}`, 200, true},
		{"owner denied a non-allowlisted key", campaigns.RolePlayer, owner, "c1", `{"fields_patch":{"might":5}}`, 403, false},
		{"owner denied when one key is outside the list", campaigns.RolePlayer, owner, "c1", `{"fields_patch":{"ancestry":"Elf","stamina":99}}`, 403, false},
		{"owner denied fields_data", campaigns.RolePlayer, owner, "c1", `{"fields_data":{"ancestry":"Elf"}}`, 403, false},
		{"owner denied an empty patch", campaigns.RolePlayer, owner, "c1", `{"fields_patch":{}}`, 403, false},
		{"owner denied a body with neither", campaigns.RolePlayer, owner, "c1", `{}`, 403, false},
		{"other player denied", campaigns.RolePlayer, "user-other", "c1", `{"fields_patch":{"ancestry":"Dwarf"}}`, 403, false},
		{"anonymous user id denied", campaigns.RolePlayer, "", "c1", `{"fields_patch":{"ancestry":"Dwarf"}}`, 403, false},
		{"owner of an entity in another campaign denied", campaigns.RolePlayer, owner, "c2", `{"fields_patch":{"ancestry":"Dwarf"}}`, 404, false},
		{"scribe unchanged: any key", campaigns.RoleScribe, "user-scribe", "c1", `{"fields_patch":{"might":5}}`, 200, true},
		{"scribe unchanged: fields_data", campaigns.RoleScribe, "user-scribe", "c1", `{"fields_data":{"might":5}}`, 200, true},
		{"gm unchanged: fields_data", campaigns.RoleOwner, "user-gm", "c1", `{"fields_data":{"might":5}}`, 200, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &identityStubSvc{entity: &Entity{ID: "e1", CampaignID: "c1", Name: "Hero", OwnerUserID: &owner,
				FieldOverrides: &FieldOverrides{Added: []FieldDefinition{{Key: "heritage", GMOnly: true}}}}}
			err := runIdentityCall(t, svc, (*Handler).UpdateFieldsAPI, tc.role, tc.userID, tc.campaignID, tc.body)
			wantStatus(t, err, tc.wantStatus)
			if (svc.writeCount > 0) != tc.wantWrite {
				t.Fatalf("write reached = %v, want %v", svc.writeCount > 0, tc.wantWrite)
			}
		})
	}
}

func TestUpdateMetadataAPI_ClaimedOwnerRenameOnly(t *testing.T) {
	owner := "user-owner"
	cases := []struct {
		name       string
		role       campaigns.Role
		userID     string
		body       string
		wantStatus int
		wantWrite  bool
	}{
		{"owner rename allowed", campaigns.RolePlayer, owner, `{"name":"New Name"}`, 200, true},
		{"owner cannot change descriptor", campaigns.RolePlayer, owner, `{"name":"X","type_label":"Hero"}`, 403, false},
		{"owner cannot change descriptor alone", campaigns.RolePlayer, owner, `{"type_label":"Hero"}`, 403, false},
		{"owner cannot change parent", campaigns.RolePlayer, owner, `{"name":"X","parent_id":"p1"}`, 403, false},
		{"owner cannot clear parent", campaigns.RolePlayer, owner, `{"parent_id":null}`, 403, false},
		{"owner cannot change visibility", campaigns.RolePlayer, owner, `{"name":"X","is_private":false}`, 403, false},
		{"owner cannot change visibility mode", campaigns.RolePlayer, owner, `{"name":"X","visibility":"everyone"}`, 403, false},
		{"owner cannot null the name", campaigns.RolePlayer, owner, `{"name":null}`, 403, false},
		{"owner empty body refused", campaigns.RolePlayer, owner, `{}`, 403, false},
		{"other player denied", campaigns.RolePlayer, "user-other", `{"name":"X"}`, 403, false},
		{"scribe unchanged: all three keys", campaigns.RoleScribe, "user-scribe", `{"name":"X","type_label":"T","parent_id":"p"}`, 200, true},
		{"gm unchanged", campaigns.RoleOwner, "user-gm", `{"type_label":"T"}`, 200, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &identityStubSvc{entity: &Entity{ID: "e1", CampaignID: "c1", Name: "Hero", OwnerUserID: &owner}}
			err := runIdentityCall(t, svc, (*Handler).UpdateMetadataAPI, tc.role, tc.userID, "c1", tc.body)
			wantStatus(t, err, tc.wantStatus)
			if (svc.writeCount > 0) != tc.wantWrite {
				t.Fatalf("write reached = %v, want %v", svc.writeCount > 0, tc.wantWrite)
			}
		})
	}
}

// A GM can hide a claimed character from its player; the claim must not
// keep a write path open to a page the player can no longer see.
func TestIdentityWrites_RefusedOnHiddenPage(t *testing.T) {
	owner := "user-owner"
	calls := []struct {
		name    string
		handler func(*Handler, echo.Context) error
		body    string
	}{
		{"fields", (*Handler).UpdateFieldsAPI, `{"fields_patch":{"ancestry":"Dwarf"}}`},
		{"rename", (*Handler).UpdateMetadataAPI, `{"name":"New"}`},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			svc := &identityStubSvc{entity: &Entity{ID: "e1", CampaignID: "c1", Name: "Hero", OwnerUserID: &owner}, hidden: true}
			wantStatus(t, runIdentityCall(t, svc, tc.handler, campaigns.RolePlayer, owner, "c1", tc.body), 404)
			if svc.writeCount != 0 {
				t.Fatal("write reached a hidden page")
			}
		})
	}
}

// The owner's rename must carry only the name into the service, so the
// descriptor and parent stay as stored.
func TestUpdateMetadataAPI_OwnerRenameLeavesOtherFieldsAbsent(t *testing.T) {
	owner := "user-owner"
	svc := &identityStubSvc{entity: &Entity{ID: "e1", CampaignID: "c1", Name: "Hero", OwnerUserID: &owner}}
	if err := runIdentityCall(t, svc, (*Handler).UpdateMetadataAPI, campaigns.RolePlayer, owner, "c1", `{"name":"New"}`); err != nil {
		t.Fatal(err)
	}
	if svc.updateIn == nil || !svc.updateIn.Name.Present() || svc.updateIn.TypeLabel.Present() || svc.updateIn.ParentID.Present() || svc.updateIn.IsPrivate != nil {
		t.Fatalf("rename must set only the name: %+v", svc.updateIn)
	}
}

func TestOwnerIdentityAllowlist(t *testing.T) {
	want := []string{"ancestry", "background", "career", "culture", "heritage", "kit", "race", "species"}
	got := OwnerIdentityFieldKeys()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("allowlist changed: %v", got)
	}
}

func TestCanEditIdentity(t *testing.T) {
	owner := "u1"
	ent := &Entity{OwnerUserID: &owner}
	cases := []struct {
		name string
		role campaigns.Role
		uid  string
		want bool
	}{
		{"scribe", campaigns.RoleScribe, "x", true},
		{"claimed player", campaigns.RolePlayer, "u1", true},
		{"other player", campaigns.RolePlayer, "u2", false},
		{"non-member owner id", campaigns.RoleNone, "u1", false},
	}
	for _, tc := range cases {
		if got := CanEditIdentity(tc.role, ent, tc.uid); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

// stubPlanner serves a fixed creator plan, and counts reads so a test can
// show the plan is only loaded when a patch reaches past the identity keys.
type stubPlanner struct {
	plan  *HeroPlan
	reads int
}

func (p *stubPlanner) HeroPlan(_ context.Context, _, _ string, director bool) (*HeroPlan, error) {
	p.reads++
	if director {
		panic("the owner door must read the plan as a player sees it")
	}
	return p.plan, nil
}

// A claimed owner may set a hero builder step, but only to a name on that
// step's list; the step does not turn the field into free text.
func TestUpdateFieldsAPI_ClaimedOwnerBuilderPicks(t *testing.T) {
	owner := "user-owner"
	plan := &HeroPlan{Steps: []HeroStep{
		{FieldKey: "class", Label: "Class", Choices: []HeroChoice{{Name: "Fury"}, {Name: "Shadow"}}},
		{FieldKey: "ancestry", Label: "Ancestry", Choices: []HeroChoice{{Name: "Dwarf"}}},
		{FieldKey: "background", Label: "Background", Choices: []HeroChoice{{Name: "Sage"}}},
	}}
	cases := []struct {
		name       string
		role       campaigns.Role
		userID     string
		body       string
		wantStatus int
		wantWrite  bool
		wantReads  int
	}{
		{"owner sets a step to a listed name", campaigns.RolePlayer, owner, `{"fields_patch":{"class":"Shadow"}}`, 200, true, 1},
		{"listed name in another case", campaigns.RolePlayer, owner, `{"fields_patch":{"class":"  fury "}}`, 200, true, 1},
		{"owner clears a step", campaigns.RolePlayer, owner, `{"fields_patch":{"class":null}}`, 200, true, 1},
		{"step with identity keys together", campaigns.RolePlayer, owner, `{"fields_patch":{"class":"Fury","culture":"anything"}}`, 200, true, 1},
		{"name not on the step's list", campaigns.RolePlayer, owner, `{"fields_patch":{"class":"Wizard"}}`, 400, false, 1},
		{"a step takes text, not a number", campaigns.RolePlayer, owner, `{"fields_patch":{"class":5}}`, 400, false, 1},
		{"a step the type marks GM-only stays closed", campaigns.RolePlayer, owner, `{"fields_patch":{"background":"Sage"}}`, 403, false, 0},
		{"a field that is not a step stays closed", campaigns.RolePlayer, owner, `{"fields_patch":{"class":"Fury","might":3}}`, 403, false, 1},
		{"identity keys alone never load the plan", campaigns.RolePlayer, owner, `{"fields_patch":{"ancestry":"Anything"}}`, 200, true, 0},
		{"other player denied without loading the plan", campaigns.RolePlayer, "user-other", `{"fields_patch":{"class":"Fury"}}`, 403, false, 0},
		{"scribe never loads the plan", campaigns.RoleScribe, "user-scribe", `{"fields_patch":{"class":"Wizard"}}`, 200, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &identityStubSvc{entity: &Entity{ID: "e1", CampaignID: "c1", Name: "Hero", OwnerUserID: &owner}}
			pl := &stubPlanner{plan: plan}
			err := runIdentityCallWith(t, svc, pl, (*Handler).UpdateFieldsAPI, tc.role, tc.userID, "c1", tc.body)
			wantStatus(t, err, tc.wantStatus)
			if (svc.writeCount > 0) != tc.wantWrite {
				t.Fatalf("write reached = %v, want %v", svc.writeCount > 0, tc.wantWrite)
			}
			if pl.reads != tc.wantReads {
				t.Fatalf("plan reads = %d, want %d", pl.reads, tc.wantReads)
			}
		})
	}
}

// Without a planner (no game system wired) the door is exactly the identity
// list, as before.
func TestUpdateFieldsAPI_NoPlannerKeepsIdentityOnly(t *testing.T) {
	owner := "user-owner"
	svc := &identityStubSvc{entity: &Entity{ID: "e1", CampaignID: "c1", Name: "Hero", OwnerUserID: &owner}}
	err := runIdentityCall(t, svc, (*Handler).UpdateFieldsAPI, campaigns.RolePlayer, owner, "c1", `{"fields_patch":{"class":"Fury"}}`)
	wantStatus(t, err, 403)
	if svc.writeCount != 0 {
		t.Fatal("write reached without a plan")
	}
}

func runIdentityCallWith(t *testing.T, svc *identityStubSvc, pl HeroPlanner, handler func(*Handler, echo.Context) error, role campaigns.Role, userID, campaignID, body string) error {
	t.Helper()
	return runIdentityCallOn(t, &Handler{service: svc, heroPlanner: pl}, handler, role, userID, campaignID, body)
}
