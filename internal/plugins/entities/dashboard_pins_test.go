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

// TestUpdateEntityTypeDashboard_KeepsPinsUnlessNamed pins that the category
// dashboard save is partial for pinned pages: the description editor, which
// doesn't send them, leaves the pins as they are; a body that names them,
// even as an empty list, replaces them.
func TestUpdateEntityTypeDashboard_KeepsPinsUnlessNamed(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"description only keeps pins", `{"description":"Towns"}`, "p1,p2"},
		{"named pins replace them", `{"description":"Towns","pinned_entity_ids":["p3"]}`, "p3"},
		{"an empty list clears them", `{"description":"Towns","pinned_entity_ids":[]}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			saved := false
			types := &mockEntityTypeRepo{
				findByIDFn: func(context.Context, int) (*EntityType, error) {
					return &EntityType{ID: 4, CampaignID: "c1", PinnedEntityIDs: []string{"p1", "p2"}}, nil
				},
				updateDashboardFn: func(_ context.Context, _ int, _ *string, pinned []string) error {
					got, saved = pinned, true
					return nil
				},
			}
			h := &Handler{service: NewEntityService(&mockEntityRepo{}, types, &mockPermissionRepo{})}
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/campaigns/c1/entity-types/4/dashboard", strings.NewReader(tt.body))
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "etid")
			c.SetParamValues("c1", "4")
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner, IsMember: true})
			if err := h.UpdateEntityTypeDashboard(c); err != nil {
				t.Fatal(err)
			}
			if !saved || strings.Join(got, ",") != tt.want {
				t.Errorf("saved pins = %v, want %q", got, tt.want)
			}
		})
	}
}
