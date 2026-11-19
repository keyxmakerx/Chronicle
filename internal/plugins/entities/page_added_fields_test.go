package entities

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestRestrictionDefs pins that a page's own added fields count alongside its
// category's, and that hiding a category field on the page never drops it.
func TestRestrictionDefs(t *testing.T) {
	typeFields := []FieldDefinition{{Key: "might"}, {Key: "gm_notes", GMOnly: true}}
	cases := []struct {
		name      string
		overrides *FieldOverrides
		wantKeys  []string
	}{
		{"no overrides", nil, []string{"might", "gm_notes"}},
		{"empty overrides", &FieldOverrides{}, []string{"might", "gm_notes"}},
		{"hidden category field stays", &FieldOverrides{Hidden: []string{"gm_notes"}}, []string{"might", "gm_notes"}},
		{"added field joins", &FieldOverrides{Added: []FieldDefinition{{Key: "weakness", GMOnly: true}}}, []string{"might", "gm_notes", "weakness"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RestrictionDefs(typeFields, tc.overrides)
			if len(got) != len(tc.wantKeys) {
				t.Fatalf("got %d defs, want %d", len(got), len(tc.wantKeys))
			}
			for i, k := range tc.wantKeys {
				if got[i].Key != k {
					t.Errorf("def %d = %q, want %q", i, got[i].Key, k)
				}
			}
		})
	}
	// Appending must never write into the category's own slice.
	backing := make([]FieldDefinition, 1, 4)
	backing[0] = FieldDefinition{Key: "a"}
	_ = RestrictionDefs(backing, &FieldOverrides{Added: []FieldDefinition{{Key: "b"}}})
	if backing[:2][1].Key == "b" {
		t.Error("RestrictionDefs wrote into the category's backing array")
	}
}

// pageAddedFixture is a page whose category declares nothing restricted, with
// a GM-only and an owner-only field added to just this page.
func pageAddedFixture() (*Entity, *EntityType) {
	et := &EntityType{ID: 7, Fields: []FieldDefinition{{Key: "might", Label: "Might"}}}
	owner := "owner-1"
	ent := &Entity{
		ID: "e1", CampaignID: "c1", EntityTypeID: 7, OwnerUserID: &owner,
		FieldsData: map[string]any{"might": 2, "secret_weakness": "silver", "diary": "dear diary"},
		FieldOverrides: &FieldOverrides{Added: []FieldDefinition{
			{Key: "secret_weakness", Label: "Secret weakness", Type: "text", GMOnly: true},
			{Key: "diary", Label: "Diary", Type: "text", OwnerOnly: true},
		}},
	}
	return ent, et
}

// TestGetFieldsAPI_StripsPageAddedRestrictedFields pins that the attributes
// API withholds a page-added field's value exactly as it would a category
// field with the same marker.
func TestGetFieldsAPI_StripsPageAddedRestrictedFields(t *testing.T) {
	cases := []struct {
		name       string
		role       campaigns.Role
		userID     string
		wantSecret bool
		wantDiary  bool
	}{
		{"player who doesn't own it", campaigns.RolePlayer, "other", false, false},
		{"scribe", campaigns.RoleScribe, "other", false, false},
		{"player who owns it", campaigns.RolePlayer, "owner-1", false, true},
		{"campaign owner", campaigns.RoleOwner, "gm", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ent, et := pageAddedFixture()
			h := &Handler{service: &stubSvcForFields{entity: ent, etype: et}}
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1/fields", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}, {Name: "eid", Value: "e1"}})
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tc.role})
			auth.SetSession(c, &auth.Session{UserID: tc.userID})
			if err := h.GetFieldsAPI(c); err != nil {
				t.Fatalf("GetFieldsAPI: %v", err)
			}
			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			fd, _ := resp["fields_data"].(map[string]any)
			if _, ok := fd["might"]; !ok {
				t.Errorf("open field missing; body=%s", rec.Body)
			}
			if _, ok := fd["secret_weakness"]; ok != tc.wantSecret {
				t.Errorf("secret_weakness present=%v, want %v; body=%s", ok, tc.wantSecret, rec.Body)
			}
			if _, ok := fd["diary"]; ok != tc.wantDiary {
				t.Errorf("diary present=%v, want %v; body=%s", ok, tc.wantDiary, rec.Body)
			}
		})
	}
}

// TestSearchableFields_PageAdded pins that search never indexes a page-added
// restricted value.
func TestSearchableFields_PageAdded(t *testing.T) {
	ent, et := pageAddedFixture()
	got := searchableFields(ent.FieldsData, et.Fields, ent.FieldOverrides)
	if _, ok := got["secret_weakness"]; ok {
		t.Error("search indexed a page-added GM-only value")
	}
	if _, ok := got["diary"]; ok {
		t.Error("search indexed a page-added owner-only value")
	}
	if _, ok := got["might"]; !ok {
		t.Error("search dropped an open value")
	}
}
