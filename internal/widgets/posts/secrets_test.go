package posts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
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
	named := `<p>Lore of <a data-mention-id="` + hiddenPage + `">The vampire duke</a>.</p>`
	scribe := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}
	coDM := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe, IsDmGranted: true}

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
		{"scribe edits text naming a page they can't open", scribe, Post{EntryHTML: &named}, true, http.StatusForbidden},
		{"co-dm edits text naming a hidden page", coDM, Post{EntryHTML: &named}, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.post
			h := NewHandler(nil)
			h.SetMentionHider(seesOnly{})
			c := echo.New().NewContext(httptest.NewRequest(http.MethodPut, "/", nil), httptest.NewRecorder())
			err := h.requirePostWrite(c, tt.cc, &p, tt.changesText)
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

const (
	hiddenPage  = "bbbbbbbb-0000-4000-8000-000000000001"
	visiblePage = "bbbbbbbb-0000-4000-8000-000000000002"
)

// seesOnly lets a reader open visiblePage and nothing else.
type seesOnly struct{}

func (seesOnly) HideUnviewableMentions(_ context.Context, _ string, _ int, _ string, texts ...*string) (int, error) {
	total := 0
	for _, t := range texts {
		if t != nil {
			var n int
			*t, n = sanitize.HideMentions(*t, func(id string) bool { return id == visiblePage })
			total += n
		}
	}
	return total, nil
}

// TestListPosts_HiddenMentions pins that a post served below the owner and
// Co-DMs names only pages its reader can open, in both of its forms, and
// that with no check wired every mention is hidden.
func TestListPosts_HiddenMentions(t *testing.T) {
	link := func(id string) string { return `{"type":"link","attrs":{"data-mention-id":"` + id + `"}}` }
	html := `<p>Ask <a data-mention-id="` + visiblePage + `">Bram</a> about <a data-mention-id="` + hiddenPage + `">The vampire duke</a>.</p>`
	entry := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[` +
		`{"type":"text","text":"Bram","marks":[` + link(visiblePage) + `]},` +
		`{"type":"text","text":"The vampire duke","marks":[` + link(hiddenPage) + `]}]}]}`)

	tests := []struct {
		name      string
		role      campaigns.Role
		hider     sanitize.MentionHider
		wantBram  int
		wantVamps int
	}{
		{"player", campaigns.RolePlayer, seesOnly{}, 2, 0},
		{"player, no check wired", campaigns.RolePlayer, nil, 0, 0},
		{"owner", campaigns.RoleOwner, seesOnly{}, 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(postsDmGrantService{posts: []Post{{ID: "p1", Name: "Lore", Entry: entry, EntryHTML: &html}}})
			h.SetEntityGate(postsDmGrantGate{campaignOf: map[string]string{"ent": "camp-1"}})
			if tt.hider != nil {
				h.SetMentionHider(tt.hider)
			}
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/ent/posts", nil), rec)
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
			if got := strings.Count(body, "Bram"); got != tt.wantBram {
				t.Errorf("visible page named %d times, want %d; body=%s", got, tt.wantBram, body)
			}
			if got := strings.Count(body, "vampire duke"); got != tt.wantVamps {
				t.Errorf("hidden page named %d times, want %d; body=%s", got, tt.wantVamps, body)
			}
			if tt.wantVamps == 0 && strings.Contains(body, hiddenPage) {
				t.Errorf("hidden page's id was served; body=%s", body)
			}
		})
	}
}
