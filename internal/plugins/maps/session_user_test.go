package maps

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestHandlers_ReadSignedInUserFromAuth pins that the maps handlers take the
// viewer's id from the auth plugin's session, the one RequireAuth sets. Pin
// rules, own-item deletes and creator stamps all depend on it; an id read
// from anywhere else comes back empty and silently fails every one of them.
func TestHandlers_ReadSignedInUserFromAuth(t *testing.T) {
	cases := []struct {
		name   string
		userID string // "" = anonymous visitor
	}{
		{"signed-in player", "player-1"},
		{"anonymous visitor", ""},
	}
	for _, tc := range cases {
		t.Run("list markers: "+tc.name, func(t *testing.T) {
			repo := everyoneMapRepo()
			repo.listedUserID = "unset"
			h := NewHandler(NewMapService(repo))
			c, _ := newDMWriteRequest(http.MethodGet, "/campaigns/camp-1/maps/map-1/markers", "")
			c.SetParamNames("id", "mid")
			c.SetParamValues("camp-1", "map-1")
			c.Set("campaign_context", dmWriteCampaignCtx(campaigns.RolePlayer, false))
			if tc.userID != "" {
				auth.SetSession(c, &auth.Session{UserID: tc.userID})
			}
			if err := h.ListMarkersAPI(c); err != nil {
				t.Fatalf("list markers: %v", err)
			}
			if repo.listedUserID != tc.userID {
				t.Errorf("markers filtered for %q, want %q", repo.listedUserID, tc.userID)
			}
		})
	}

	t.Run("create marker stamps the creator", func(t *testing.T) {
		var created *Marker
		repo := everyoneMapRepo()
		repo.createMarkerFn = func(_ context.Context, mk *Marker) error { created = mk; return nil }
		h := NewHandler(NewMapService(repo))
		c, _ := newDMWriteRequest(http.MethodPost, "/campaigns/camp-1/maps/map-1/markers",
			`{"name":"Camp","x":10,"y":10}`)
		c.SetParamNames("id", "mid")
		c.SetParamValues("camp-1", "map-1")
		c.Set("campaign_context", dmWriteCampaignCtx(campaigns.RoleScribe, false))
		auth.SetSession(c, &auth.Session{UserID: "scribe-1"})
		if err := h.CreateMarkerAPI(c); err != nil {
			t.Fatalf("create marker: %v", err)
		}
		if created == nil || created.CreatedBy == nil || *created.CreatedBy != "scribe-1" {
			t.Fatalf("created_by = %v, want scribe-1", created)
		}
	})
}
