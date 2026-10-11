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

// stubSvcForPlayerNotesSecrets serves one viewable entity whose player notes
// carry a GM-only span in both stored forms.
type stubSvcForPlayerNotesSecrets struct {
	EntityService
}

func (s *stubSvcForPlayerNotesSecrets) GetByID(_ context.Context, id string) (*Entity, error) {
	notes := `{"type":"doc","content":[{"type":"paragraph","content":[` +
		`{"type":"text","text":"Meet at dawn. "},` +
		`{"type":"text","text":"It is a trap.","marks":[{"type":"secret"}]}]}]}`
	html := `<p>Meet at dawn. <span data-secret="true">It is a trap.</span></p>`
	return &Entity{ID: id, CampaignID: "c1", PlayerNotes: &notes, PlayerNotesHTML: &html}, nil
}

func (s *stubSvcForPlayerNotesSecrets) CheckEntityAccess(context.Context, string, int, string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: true}, nil
}

func TestGetPlayerNotes_GMOnlyContent(t *testing.T) {
	tests := []struct {
		name       string
		role       campaigns.Role
		wantSecret bool
	}{
		{"player", campaigns.RolePlayer, false},
		{"scribe", campaigns.RoleScribe, true},
		{"owner", campaigns.RoleOwner, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{service: &stubSvcForPlayerNotesSecrets{}}
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1/player-notes", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", "e1")
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tt.role,
			})

			if err := h.GetPlayerNotes(c); err != nil {
				t.Fatalf("GetPlayerNotes: %v", err)
			}
			body := rec.Body.String()
			if strings.Count(body, "Meet at dawn.") != 2 {
				t.Errorf("public text missing from a field; body=%s", body)
			}
			want := 0
			if tt.wantSecret {
				want = 2
			}
			if got := strings.Count(body, "It is a trap."); got != want {
				t.Errorf("GM-only text appears %d times, want %d; body=%s", got, want, body)
			}
		})
	}
}
