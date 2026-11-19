package app

// error_handler_campaign_home_test.go pins the way out of an error page inside
// a campaign: a member gets "Back to <campaign> home" first, anyone else gets
// nothing about the campaign (a hidden page answers like a missing one), and
// "Go back" is only ever drawn hidden, to be revealed by boot.js.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestErrorHandler_CampaignHomeLink(t *testing.T) {
	camp := &campaigns.Campaign{ID: "c1", Name: "Shattered Heroes"}
	tests := []struct {
		name string
		cc   *campaigns.CampaignContext
		want bool
	}{
		{"a member", &campaigns.CampaignContext{Campaign: camp, IsMember: true, MemberRole: campaigns.RolePlayer}, true},
		{"a visitor of a public campaign", &campaigns.CampaignContext{Campaign: camp}, false},
		{"no campaign context", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/hidden", nil)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			if tt.cc != nil {
				c.Set("campaign_context", tt.cc)
			}
			(&App{}).errorHandler(c, apperror.NewNotFound("entity not found"))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d", rec.Code)
			}
			body := rec.Body.String()
			if got := strings.Contains(body, `href="/campaigns/c1" class="quiet-word" data-campaign-home`) && strings.Contains(body, "Back to Shattered Heroes home"); got != tt.want {
				t.Errorf("campaign home link drawn = %v, want %v", got, tt.want)
			}
			if !tt.want && strings.Contains(body, "Shattered Heroes") {
				t.Errorf("the page names a campaign the viewer is not a member of")
			}
			// Go back is hidden until boot.js finds somewhere to go, and is
			// never a javascript: address.
			if !strings.Contains(body, `class="quiet-word hidden" data-go-back`) || strings.Contains(body, "javascript:") {
				t.Errorf("Go back must be drawn hidden and script-free")
			}
		})
	}
}
