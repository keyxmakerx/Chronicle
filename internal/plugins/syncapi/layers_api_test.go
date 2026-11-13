package syncapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

type fakeLayers struct {
	gotCampaign, gotEntity, gotUser string
	gotRole                         int
	err                             error
}

func (f *fakeLayers) PageLayers(_ context.Context, campaignID, entityID, userID string, role int) (any, error) {
	f.gotCampaign, f.gotEntity, f.gotUser, f.gotRole = campaignID, entityID, userID, role
	if f.err != nil {
		return nil, f.err
	}
	return map[string]any{"layers": []any{}, "canEdit": true}, nil
}

func TestLayersAPI_NotWiredIs404(t *testing.T) {
	h := NewAPIHandler(nil, nil, &dmScreenRoleCampaigns{role: campaigns.RoleOwner}, nil)
	c, _ := dmScreenContext(http.MethodGet, "", 42, "entityID", "hero-1")
	if got := dmStatusOf(t, h.GetEntityLayers(c)); got != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", got)
	}
}

// The reader decides who sees the page, so the handler must pass the page
// and the caller's real role, and hand back the reader's refusal unchanged.
func TestLayersAPI_PassesPageAndRole(t *testing.T) {
	cases := []struct {
		name   string
		keyID  int
		member campaigns.Role
		want   int
	}{
		{"bearer key", 42, campaigns.RoleOwner, 3},
		{"session player", synthKeySessionID, campaigns.RolePlayer, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeLayers{}
			h := NewAPIHandler(nil, nil, &dmScreenRoleCampaigns{role: tc.member}, nil)
			h.SetLayersReader(f)
			c, rec := dmScreenContext(http.MethodGet, "", tc.keyID, "entityID", "hero-1")
			if err := h.GetEntityLayers(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || f.gotEntity != "hero-1" || f.gotCampaign != "camp-1" || f.gotUser != "user-1" || f.gotRole != tc.want {
				t.Fatalf("code %d entity %q campaign %q user %q role %d", rec.Code, f.gotEntity, f.gotCampaign, f.gotUser, f.gotRole)
			}
		})
	}
	f := &fakeLayers{err: apperror.NewNotFound("page not found")}
	h := NewAPIHandler(nil, nil, &dmScreenRoleCampaigns{role: campaigns.RoleOwner}, nil)
	h.SetLayersReader(f)
	c, _ := dmScreenContext(http.MethodGet, "", 42, "entityID", "hidden")
	if got := dmStatusOf(t, h.GetEntityLayers(c)); got != http.StatusNotFound {
		t.Fatalf("hidden page status = %d, want 404", got)
	}
}

// A reconnecting module re-reads only the heroes whose layers changed, so
// the change must be in the feed, and must not collide with map layers.
func TestClassifyChange_HeroLayers(t *testing.T) {
	rt, op, ok := classifyChange(ws.MsgHeroLayersChanged)
	if rt != "hero_layers" || op != "updated" || !ok {
		t.Fatalf("hero_layers.changed = %q %q %v", rt, op, ok)
	}
	if rt, _, _ := classifyChange(ws.MsgLayerCreated); rt != "layer" {
		t.Fatalf("map layer classified as %q", rt)
	}
}
