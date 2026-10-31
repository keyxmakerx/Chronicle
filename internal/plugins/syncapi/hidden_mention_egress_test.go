package syncapi

// A page a player may read can mention a page they can't; the stored link
// text is that page's name, so the API serves it as "a hidden page", as the
// web does.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

const egressHiddenPage = "eeeeeeee-0000-4000-8000-000000000001"

// stubEntitySvcHidesOnePage lets no caller open egressHiddenPage.
type stubEntitySvcHidesOnePage struct {
	stubEntitySvcForDmGrantEgress
}

func (s *stubEntitySvcHidesOnePage) HideUnviewableMentions(_ context.Context, _ string, _ int, _ string, texts ...*string) (int, error) {
	total := 0
	for _, t := range texts {
		if t != nil {
			var n int
			*t, n = sanitize.HideMentions(*t, func(id string) bool { return id != egressHiddenPage })
			total += n
		}
	}
	return total, nil
}

func TestGetEntity_HidesMentionsOfHiddenPages(t *testing.T) {
	tests := []struct {
		name     string
		role     campaigns.Role
		wantName bool
	}{
		{"player", campaigns.RolePlayer, false},
		{"owner", campaigns.RoleOwner, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := `<p>Watched by <a data-mention-id="` + egressHiddenPage + `">The smuggler king</a>.</p>`
			doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"The smuggler king","marks":[{"type":"link","attrs":{"data-mention-id":"` + egressHiddenPage + `"}}]}]}]}`
			ent := &entities.Entity{ID: "e1", CampaignID: "camp-1", EntityTypeID: 7, EntryHTML: &html, Entry: &doc, PlayerNotesHTML: &html}
			svc := &stubEntitySvcHidesOnePage{stubEntitySvcForDmGrantEgress{entity: ent, etype: &entities.EntityType{ID: 7, CampaignID: "camp-1"}}}
			h := NewAPIHandler(nil, svc, &stubCampaignSvcForDmGrant{role: tt.role}, nil)

			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/entities/e1", nil), rec)
			c.SetParamNames("id", "entityID")
			c.SetParamValues("camp-1", "e1")
			c.Set(apiKeyContextKey, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "member-1", IsActive: true})
			if err := h.GetEntity(c); err != nil {
				t.Fatalf("GetEntity: %v", err)
			}
			body := rec.Body.String()
			if got := strings.Count(body, "smuggler king"); (got == 3) != tt.wantName || (got != 0 && !tt.wantName) {
				t.Errorf("hidden page named %d times (want named = %v): %s", got, tt.wantName, body)
			}
			if !tt.wantName && strings.Contains(body, egressHiddenPage) {
				t.Errorf("hidden page's id was served: %s", body)
			}
			if *ent.EntryHTML != html {
				t.Error("the stored entity was rewritten")
			}
		})
	}
}
