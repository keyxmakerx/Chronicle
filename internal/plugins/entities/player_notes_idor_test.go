// player_notes_idor_test.go — SEC-IDOR-5 visibility + cross-campaign gate for
// GetPlayerNotes. The player-notes JSON endpoint previously enforced only the
// campaign-ownership check and omitted the canonical CheckEntityAccess gate its
// siblings (GetEntry, GetFieldsAPI) apply — so a player excluded from an entity
// by grant/custom visibility could still read its player_notes by id. These
// tests drive the handler with a campaign context set (the pattern
// gm_fields_handler_test.go uses) and assert the gate at each of: a viewable
// same-campaign entity, a custom-restricted entity, and a foreign-campaign id.
package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// stubSvcForPlayerNotes embeds EntityService and overrides only the reads
// GetPlayerNotes performs. campaignOf drives the IDOR check; canView drives the
// CheckEntityAccess decision.
type stubSvcForPlayerNotes struct {
	EntityService
	campaignOf map[string]string
	canView    map[string]bool
}

func (s *stubSvcForPlayerNotes) GetByID(_ context.Context, id string) (*Entity, error) {
	camp, ok := s.campaignOf[id]
	if !ok {
		return nil, apperror.NewNotFound("entity not found")
	}
	notes, html := "secret-player-notes", "<p>secret-player-notes</p>"
	return &Entity{ID: id, CampaignID: camp, PlayerNotes: &notes, PlayerNotesHTML: &html}, nil
}

func (s *stubSvcForPlayerNotes) CheckEntityAccess(_ context.Context, entityID string, _ int, _ string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: s.canView[entityID]}, nil
}

func TestGetPlayerNotes_VisibilityAndCampaignGate(t *testing.T) {
	svc := &stubSvcForPlayerNotes{
		campaignOf: map[string]string{
			"viewable":   "c1", // same campaign, gate admits
			"restricted": "c1", // same campaign, custom-restricted: gate denies
			"foreign":    "c2", // another campaign (IDOR probe)
		},
		canView: map[string]bool{"viewable": true, "restricted": false, "foreign": true},
	}

	cases := []struct {
		name   string
		eid    string
		wantOK bool
	}{
		{"viewable same-campaign entity → served", "viewable", true},
		{"custom-restricted entity → 404 (visibility gate)", "restricted", false},
		{"foreign-campaign entity → 404 (kills IDOR)", "foreign", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{service: svc}
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/"+tc.eid+"/player-notes", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", tc.eid)
			// The campaign-context middleware normally sets this; set it directly
			// by its known key so GetCampaignContext resolves it (a real Player).
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign:   &campaigns.Campaign{ID: "c1"},
				MemberRole: campaigns.RolePlayer,
			})

			err := h.GetPlayerNotes(c)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("permitted viewer got error: %v", err)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("permitted viewer got %d, want 200", rec.Code)
				}
				if !strings.Contains(rec.Body.String(), "secret-player-notes") {
					t.Errorf("permitted viewer should receive player_notes; body=%s", rec.Body)
				}
				return
			}
			// Denied: handler returns an apperror (NotFound) and writes no body.
			assertAppError(t, err, http.StatusNotFound)
			if strings.Contains(rec.Body.String(), "secret-player-notes") {
				t.Errorf("blocked request leaked player_notes; body=%s", rec.Body)
			}
		})
	}
}
