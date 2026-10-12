package entities

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// runOwner runs h as camp-1's owner and returns the status and body.
func runOwner(t *testing.T, h echo.HandlerFunc, req *http.Request, params ...string) (int, string, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	var names, values []string
	for i := 0; i+1 < len(params); i += 2 {
		names, values = append(names, params[i]), append(values, params[i+1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner,
	})
	err := h(c)
	code := rec.Code
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		code = ae.Code
	} else if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return code, rec.Body.String(), rec.Header().Get("Location")
}

// A category's Customize tab opens only for this campaign's categories and
// only for the tabs the workspace has.
func TestEntityTypeCustomizeTab(t *testing.T) {
	tests := []struct {
		name, campaign, tab string
		code                int
		want                string
	}{
		{"own category's layout", "camp-1", "layout", http.StatusOK, `data-widget="layout-editor"`},
		{"another campaign's category", "camp-2", "layout", http.StatusNotFound, ""},
		{"a tab the workspace lacks", "camp-1", "secrets", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			et := &EntityType{ID: 7, CampaignID: tt.campaign, Name: "NPC", NamePlural: "NPCs"}
			h := &Handler{service: &stubSvcForFields{etype: et}}
			req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entity-types/7/customize-tab/"+tt.tab, nil)
			code, body, _ := runOwner(t, h.EntityTypeCustomizeTab, req, "id", "camp-1", "etid", "7", "tab", tt.tab)
			if code != tt.code {
				t.Fatalf("code = %d, want %d", code, tt.code)
			}
			if tt.want != "" && !strings.Contains(body, tt.want) {
				t.Errorf("body missing %q", tt.want)
			}
		})
	}
}

// The old categories page sends a visit to Customize's Categories area.
func TestEntityTypesPage_RedirectsToCustomize(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entity-types", nil)
	code, _, loc := runOwner(t, h.EntityTypesPage, req, "id", "camp-1")
	if code != http.StatusSeeOther || loc != "/campaigns/camp-1/customize/categories" {
		t.Errorf("got %d to %q, want 303 to Customize's Categories", code, loc)
	}
}
