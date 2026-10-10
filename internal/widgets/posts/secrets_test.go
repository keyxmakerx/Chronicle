package posts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestListPosts_GMOnlyContent pins that a visible post's GM-only parts reach
// Scribes and owners but never players.
func TestListPosts_GMOnlyContent(t *testing.T) {
	html := `<p>Public lore. <span data-secret="true">The duke is a vampire.</span></p>`
	entry := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[` +
		`{"type":"text","text":"Public lore. "},` +
		`{"type":"text","text":"The duke is a vampire.","marks":[{"type":"secret"}]}]}]}`)

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
			h := NewHandler(postsDmGrantService{posts: []Post{{ID: "p1", Name: "Lore", Entry: entry, EntryHTML: &html}}})
			h.SetEntityGate(postsDmGrantGate{campaignOf: map[string]string{"ent": "camp-1"}})

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/ent/posts", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("camp-1", "ent")
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: tt.role, IsMember: true,
			})
			auth.SetSession(c, &auth.Session{UserID: "u1"})

			if err := h.ListPosts(c); err != nil {
				t.Fatalf("ListPosts: %v", err)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "Public lore.") {
				t.Errorf("public text missing; body=%s", body)
			}
			want := 0
			if tt.wantSecret {
				want = 2 // once in entry, once in entryHtml
			}
			if got := strings.Count(body, "vampire"); got != want {
				t.Errorf("GM-only text appears %d times, want %d; body=%s", got, want, body)
			}
		})
	}
}
