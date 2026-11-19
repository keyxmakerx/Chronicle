package foundry_vtt

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Until a Foundry module has connected, the chip beside a map's title is a
// setup hint: the DM team sees it, and everyone else gets nothing where it
// would sit. Once Foundry has connected, every member sees its state.
func TestPresencePill_NeverConnectedIsForTheDMTeam(t *testing.T) {
	never, seen, live := fakePresence{}, fakePresence{seen: true}, fakePresence{seen: true, connected: true}
	tests := []struct {
		name         string
		role         campaigns.Role
		dmAccess     bool
		viewAsPlayer bool
		presence     fakePresence
		want         string
	}{
		{"owner, never connected", campaigns.RoleOwner, false, false, never, "Foundry: never connected"},
		{"member with DM access, never connected", campaigns.RolePlayer, true, false, never, "Foundry: never connected"},
		{"scribe, never connected", campaigns.RoleScribe, false, false, never, ""},
		{"player, never connected", campaigns.RolePlayer, false, false, never, ""},
		{"owner viewing as a player, never connected", campaigns.RoleOwner, false, true, never, ""},
		{"player, connected now", campaigns.RolePlayer, false, false, live, "Connected to Foundry"},
		{"player, connected before", campaigns.RolePlayer, false, false, seen, "Foundry: last seen"},
		{"owner, connected now", campaigns.RoleOwner, false, false, live, "Connected to Foundry"},
		{"owner viewing as a player, connected now", campaigns.RoleOwner, false, true, live, "Connected to Foundry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(nil)
			h.SetPresenceLookup(tt.presence)
			req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/foundry-vtt/presence-pill-fragment", nil)
			if tt.viewAsPlayer {
				req.AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "camp-1"})
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign:    &campaigns.Campaign{ID: "camp-1"},
				MemberRole:  tt.role,
				IsDmGranted: tt.dmAccess,
				IsMember:    true,
			})
			if err := h.CampaignShowPresencePillHandler(c); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: an empty fragment still has to swap the placeholder away", rec.Code)
			}
			body := strings.TrimSpace(rec.Body.String())
			if tt.want == "" {
				if body != "" {
					t.Errorf("fragment = %q, want nothing", body)
				}
				return
			}
			if !strings.Contains(body, tt.want) {
				t.Errorf("fragment = %q, want it to say %q", body, tt.want)
			}
		})
	}
}

// A handler nobody wired a presence source into knows of no connection, so
// it too says nothing to a player.
func TestPresencePill_UnwiredSaysNothingToAPlayer(t *testing.T) {
	h := NewHandler(nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/foundry-vtt/presence-pill-fragment", nil), rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsMember: true,
	})
	if err := h.CampaignShowPresencePillHandler(c); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "" {
		t.Errorf("fragment = %q, want nothing", body)
	}
}
