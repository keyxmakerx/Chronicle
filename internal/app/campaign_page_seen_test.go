package app

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type seenRecorder struct{ marks []string }

func (s *seenRecorder) MarkBrowserSeen(campaignID, userID string) {
	s.marks = append(s.marks, campaignID+"/"+userID)
}

// The middleware records only successful page loads and HTMX fetches by a real
// member of a campaign route; polls, writes, failures and strangers do not
// count as "has Chronicle open".
func TestCampaignPageSeen(t *testing.T) {
	member := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, IsMember: true}
	outsider := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}}
	tests := []struct {
		name    string
		method  string
		path    string
		headers map[string]string
		cc      *campaigns.CampaignContext
		status  int
		want    int
	}{
		{"full page load", "GET", "/campaigns/:id/entities", map[string]string{"Accept": "text/html,application/xhtml+xml"}, member, 200, 1},
		{"htmx fetch", "GET", "/campaigns/:id/dm-screen", map[string]string{"HX-Request": "true"}, member, 200, 1},
		{"json poll does not count", "GET", "/campaigns/:id/notes", map[string]string{"Accept": "application/json"}, member, 200, 0},
		{"write does not count", "POST", "/campaigns/:id/entities", map[string]string{"Accept": "text/html"}, member, 200, 0},
		{"failed page does not count", "GET", "/campaigns/:id/entities", map[string]string{"Accept": "text/html"}, member, 403, 0},
		{"non-member does not count", "GET", "/campaigns/:id/entities", map[string]string{"Accept": "text/html"}, outsider, 200, 0},
		{"no campaign context", "GET", "/campaigns/:id/entities", map[string]string{"Accept": "text/html"}, nil, 200, 0},
		{"other route does not count", "GET", "/settings", map[string]string{"Accept": "text/html"}, member, 200, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &seenRecorder{}
			e := echo.New()
			e.Use(campaignPageSeen(rec))
			handler := func(c echo.Context) error {
				if tt.cc != nil {
					c.Set("campaign_context", tt.cc)
				}
				c.Set("auth_user_id", "u1")
				return c.NoContent(tt.status)
			}
			e.Add(tt.method, tt.path, handler)
			req := httptest.NewRequest(tt.method, expandPath(tt.path), nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			e.ServeHTTP(httptest.NewRecorder(), req)
			if len(rec.marks) != tt.want {
				t.Fatalf("marks = %v, want %d", rec.marks, tt.want)
			}
			if tt.want == 1 && rec.marks[0] != "c1/u1" {
				t.Errorf("mark = %q", rec.marks[0])
			}
		})
	}
}

func expandPath(p string) string {
	if len(p) > 14 && p[:14] == "/campaigns/:id" {
		return "/campaigns/c1" + p[14:]
	}
	return p
}
