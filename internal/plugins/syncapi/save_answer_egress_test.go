package syncapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// storedPageSvc answers a save with the whole stored page, as the real
// service does: a secret block and a GM-only field value included.
type storedPageSvc struct {
	gateEntitySvc
}

func (s *storedPageSvc) Update(_ context.Context, id string, _ entities.UpdateEntityInput) (*entities.Entity, error) {
	s.writes = append(s.writes, "update")
	html := `<p>Open. <span data-secret="true">The duke is a vampire.</span></p>`
	return &entities.Entity{ID: id, CampaignID: "camp-1", EntityTypeID: 1, EntryHTML: &html,
		FieldsData: map[string]any{"gm_notes": "Kill him in act two", "motto": "Onward"}}, nil
}

func (s *storedPageSvc) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	return &entities.EntityType{ID: id, CampaignID: "camp-1", Fields: []entities.FieldDefinition{{Key: "gm_notes", GMOnly: true}, {Key: "motto"}}}, nil
}

// A save's answer is the copy a read would give: a Scribe's empty save must
// not hand back the secret text or the GM-only value; the owner gets both.
func TestUpdateEntityAnswer_IsTheViewersCopy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		role       campaigns.Role
		wantHidden bool
	}{
		{"scribe", campaigns.RoleScribe, true},
		{"owner", campaigns.RoleOwner, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ents := &storedPageSvc{}
			h := NewAPIHandler(&stubSyncSvcForRole{}, ents, gateCampaignSvc{role: tc.role}, nil)
			c, rec := gateContext(http.MethodPut, `{}`)
			if err := h.UpdateEntity(c); err != nil {
				t.Fatalf("UpdateEntity: %v", err)
			}
			body := rec.Body.String()
			for _, secret := range []string{"vampire", "Kill him"} {
				if strings.Contains(body, secret) == tc.wantHidden {
					t.Errorf("%q in answer = %v; body=%s", secret, !tc.wantHidden, body)
				}
			}
			if !strings.Contains(body, "Onward") {
				t.Errorf("an open field went missing; body=%s", body)
			}
		})
	}
}

// noTypeCreateSvc saves the page, then cannot load its category.
type noTypeCreateSvc struct {
	stubEntitySvcCapturingCreate
}

func (s *noTypeCreateSvc) GetEntityTypeByID(context.Context, int) (*entities.EntityType, error) {
	return nil, context.DeadlineExceeded
}

// A create that is already saved still answers 201, or a retrying client
// would make the page twice; without its category the answer carries only
// the id and name, never the unfiltered fields.
func TestCreateEntityAnswer_SavedButUnfilterable(t *testing.T) {
	esvc := &noTypeCreateSvc{}
	h := NewAPIHandler(nil, esvc, gateCampaignSvc{role: campaigns.RoleScribe}, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/entities",
		strings.NewReader(`{"name":"Duke","entity_type_id":3,"is_private":false,"fields_data":{"gm_notes":"Kill him in act two"}}`))
	req.Header.Set("Content-Type", "application/json")
	c := echo.New().NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	c.Set(apiKeyContextKey, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "u-1"})
	if err := h.CreateEntity(c); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"ent-new"`) || !strings.Contains(body, `"Duke"`) || strings.Contains(body, "Kill him") {
		t.Errorf("answer should be only id and name; body=%s", body)
	}
}
