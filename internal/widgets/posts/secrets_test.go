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
// the owner and Co-DMs but never players or Scribes.
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
		{"scribe sees what players see", campaigns.RoleScribe, false},
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

// TestRequirePostWrite pins that a Scribe writes only what they are served: a
// GM-only post reads as missing, and text with GM-only parts is not theirs to
// change, since their copy of it left those parts out.
func TestRequirePostWrite(t *testing.T) {
	secret := `<p>Lore. <span data-secret="true">The duke is a vampire.</span></p>`
	plain := `<p>Lore.</p>`
	scribe := &campaigns.CampaignContext{MemberRole: campaigns.RoleScribe}
	coDM := &campaigns.CampaignContext{MemberRole: campaigns.RoleScribe, IsDmGranted: true}

	tests := []struct {
		name        string
		cc          *campaigns.CampaignContext
		post        Post
		changesText bool
		want        int // 0 = allowed
	}{
		{"scribe edits plain text", scribe, Post{EntryHTML: &plain}, true, 0},
		{"scribe edits text with a secret", scribe, Post{EntryHTML: &secret}, true, http.StatusForbidden},
		{"scribe renames a post with a secret", scribe, Post{EntryHTML: &secret}, false, 0},
		{"scribe touches a GM-only post", scribe, Post{IsPrivate: true, EntryHTML: &plain}, false, http.StatusNotFound},
		{"co-dm edits text with a secret", coDM, Post{EntryHTML: &secret}, true, 0},
		{"co-dm touches a GM-only post", coDM, Post{IsPrivate: true}, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.post
			err := requirePostWrite(tt.cc, &p, tt.changesText)
			if tt.want == 0 {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			assertAppError(t, err, tt.want)
		})
	}
}
