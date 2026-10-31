package sessions

// Every member reads a recap, so below the owner and Co-DMs it is served
// without GM-only parts and without the names of pages its reader can't open
// (ADR-055 rule 3), and a writer whose copy left something out can't save
// over it.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

const (
	recapHiddenPage  = "cccccccc-0000-4000-8000-000000000001"
	recapVisiblePage = "cccccccc-0000-4000-8000-000000000002"
)

// recapSeesOnly lets a reader open recapVisiblePage and nothing else.
type recapSeesOnly struct{}

func (recapSeesOnly) HideUnviewableMentions(_ context.Context, _ string, _ int, _ string, texts ...*string) (int, error) {
	total := 0
	for _, t := range texts {
		if t != nil {
			var n int
			*t, n = sanitize.HideMentions(*t, func(id string) bool { return id == recapVisiblePage })
			total += n
		}
	}
	return total, nil
}

func recapHandler(recapHTML string, saved *bool) *Handler {
	recap := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"The night."}]}]}`
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			r, rh := recap, recapHTML
			return &Session{ID: id, CampaignID: "camp-1", Name: "One", Status: StatusCompleted, Recap: &r, RecapHTML: &rh}, nil
		},
		updateRecapFn: func(_ context.Context, _ string, _, _ *string) error {
			*saved = true
			return nil
		},
	}
	h := NewHandler(NewSessionService(repo, nil, &stubEntityVisibility{}))
	h.SetMentionHider(recapSeesOnly{})
	return h
}

func recapCtx(role campaigns.Role, dm bool, method, body string) (echo.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(method, "/campaigns/camp-1/sessions/s1/recap", strings.NewReader(body)), rec)
	c.SetParamNames("id", "sid")
	c.SetParamValues("camp-1", "s1")
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsDmGranted: dm, IsMember: true})
	return c, rec
}

func TestRecapServedToReader(t *testing.T) {
	named := `<p>We met <a data-mention-id="` + recapVisiblePage + `">Bram</a> and <a data-mention-id="` + recapHiddenPage + `">The vampire duke</a>.</p>`
	secret := `<p>We rested. <span data-secret="true">The duke followed us.</span></p>`
	tests := []struct {
		name       string
		role       campaigns.Role
		dm         bool
		stored     string
		leak       string
		wantLocked bool
	}{
		{"player: hidden page not named", campaigns.RolePlayer, false, named, "vampire duke", false},
		{"scribe: hidden page not named, recap locked", campaigns.RoleScribe, false, named, "vampire duke", true},
		{"scribe: old GM-only text removed, recap locked", campaigns.RoleScribe, false, secret, "followed us", true},
		{"co-dm sees the recap as stored", campaigns.RoleScribe, true, named, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved := false
			h := recapHandler(tt.stored, &saved)

			c, rec := recapCtx(tt.role, tt.dm, http.MethodGet, "")
			if err := h.GetRecapAPI(c); err != nil {
				t.Fatalf("GetRecapAPI: %v", err)
			}
			body := rec.Body.String()
			if tt.leak != "" && (strings.Contains(body, tt.leak) || strings.Contains(body, recapHiddenPage)) {
				t.Errorf("served what the reader can't see: %s", body)
			}
			if tt.leak == "" && !strings.Contains(body, "vampire duke") {
				t.Errorf("a co-dm lost the stored recap: %s", body)
			}
			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if locked, _ := got["locked"].(bool); locked != tt.wantLocked {
				t.Errorf("locked = %v, want %v", locked, tt.wantLocked)
			}

			c, rec = recapCtx(tt.role, tt.dm, http.MethodGet, "")
			if err := h.NightPageAPI(c); err != nil {
				t.Fatalf("NightPageAPI: %v", err)
			}
			if tt.leak != "" && strings.Contains(rec.Body.String(), tt.leak) {
				t.Errorf("night page served what the reader can't see: %s", rec.Body.String())
			}

			if tt.role < campaigns.RoleScribe {
				return
			}
			c, _ = recapCtx(tt.role, tt.dm, http.MethodPut, `{"entry_html":"<p>We met a hidden page.</p>"}`)
			err := h.UpdateRecapAPI(c)
			if tt.wantLocked {
				if err == nil || saved {
					t.Fatalf("a save from a copy with parts left out went through (err = %v)", err)
				}
				return
			}
			if err != nil || !saved {
				t.Fatalf("err = %v, saved = %v; want saved", err, saved)
			}
		})
	}
}
