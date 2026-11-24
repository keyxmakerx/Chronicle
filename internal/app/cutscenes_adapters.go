package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/cutscenes"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// cutsceneMembersAdapter implements cutscenes.Members over the campaigns
// service. A Director is the owner or anyone holding the campaign's DM-only
// grant, the same rule the request context uses.
type cutsceneMembersAdapter struct {
	svc campaigns.CampaignService
}

func (a *cutsceneMembersAdapter) Members(ctx context.Context, campaignID string) ([]cutscenes.Member, error) {
	c, err := a.svc.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	list, err := a.svc.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	grants := map[string]bool{}
	for _, id := range c.ParseSettings().DmGrantIDs {
		grants[id] = true
	}
	out := make([]cutscenes.Member, 0, len(list))
	for _, m := range list {
		out = append(out, cutscenes.Member{UserID: m.UserID, Name: m.DisplayName, Director: m.Role >= campaigns.RoleOwner || grants[m.UserID]})
	}
	return out, nil
}

// cutscenePresenceAdapter says where each member is now, from the same
// signals as the DM Screen's presence (dmPresenceAdapter) but kept apart, so
// the Show sheet can say In Chronicle, In Foundry or Both. hub is bound once
// the websocket hub exists; until then everyone reads as away.
type cutscenePresenceAdapter struct {
	hub     dmBrowserHub
	foundry syncapi.FoundryPlayerRepository
	now     func() time.Time
}

func (a *cutscenePresenceAdapter) Places(ctx context.Context, campaignID string) (map[string]cutscenes.Place, error) {
	out := map[string]cutscenes.Place{}
	if a.hub == nil {
		return out, nil
	}
	var reports []syncapi.FoundryPlayer
	_, foundryUp := a.hub.FoundryPresence(campaignID)
	if a.foundry != nil && foundryUp {
		var err error
		// A failed read only loses the Foundry signal; browser presence stands.
		if reports, err = a.foundry.List(ctx, campaignID); err != nil {
			slog.Warn("cutscenes: reading foundry players", slog.String("campaign_id", campaignID), slog.Any("error", err))
			reports = nil
		}
	}
	now := time.Now()
	if a.now != nil {
		now = a.now()
	}
	browser := append(a.hub.BrowserUserIDs(campaignID), a.hub.RecentBrowserUsers(campaignID, pageSeenWindow)...)
	inChronicle, inFoundry := presenceSignals(browser, reports, foundryUp, now)
	for id := range inChronicle {
		p := out[id]
		p.Chronicle = true
		out[id] = p
	}
	for id := range inFoundry {
		p := out[id]
		p.Foundry = true
		out[id] = p
	}
	return out, nil
}

// cutsceneLivePublisher puts showings on screens over the hub. bus is bound
// once the event bus exists; until then a showing is only stored, and players
// find it in their Watch again list.
type cutsceneLivePublisher struct {
	bus ws.EventBus
}

// viewerIDs lists a showing's people.
func viewerIDs(s cutscenes.Showing) []string {
	out := make([]string, 0, len(s.Viewers))
	for _, v := range s.Viewers {
		out = append(out, v.UserID)
	}
	return out
}

// Show sends the scene to its viewers' browser tabs, strictly (the Director's
// own tabs only when they picked themselves), and hands it to the campaign's
// Foundry module with the viewer list, which only the module's DM-held socket
// receives.
func (p *cutsceneLivePublisher) Show(campaignID string, s cutscenes.Showing) {
	if p.bus == nil || len(s.Viewers) == 0 {
		return
	}
	ids := viewerIDs(s)
	live := ws.NewMessage(ws.MsgCutsceneShow, campaignID, s.ID, map[string]any{"showingId": s.ID, "scene": s.Scene})
	live.AllowedUsers, live.StrictAudience = ids, true
	live.Sources = []string{ws.BrowserSource}
	p.bus.Publish(live)

	relay := ws.NewMessage(ws.MsgCutsceneFoundry, campaignID, s.ID, map[string]any{
		"showingId": s.ID, "viewers": ids, "scene": s.Scene, "rewatchUntil": s.RewatchUntil,
	})
	relay.RequiresDM = true
	relay.Sources = []string{foundry_vtt.ModuleSource}
	p.bus.Publish(relay)
}

// Stop takes the showing off the same screens.
func (p *cutsceneLivePublisher) Stop(campaignID string, s cutscenes.Showing) {
	if p.bus == nil || len(s.Viewers) == 0 {
		return
	}
	live := ws.NewMessage(ws.MsgCutsceneStop, campaignID, s.ID, map[string]string{"showingId": s.ID})
	live.AllowedUsers, live.StrictAudience = viewerIDs(s), true
	live.Sources = []string{ws.BrowserSource}
	p.bus.Publish(live)

	relay := ws.NewMessage(ws.MsgCutsceneFoundryStop, campaignID, s.ID, map[string]string{"showingId": s.ID})
	relay.RequiresDM = true
	relay.Sources = []string{foundry_vtt.ModuleSource}
	p.bus.Publish(relay)
}

// cutsceneSyncAdapter implements syncapi.CutsceneAPIService over the cut
// scenes service.
type cutsceneSyncAdapter struct {
	svc cutscenes.Service
}

func (a cutsceneSyncAdapter) MemberShowings(ctx context.Context, campaignID, userID string) (any, int, error) {
	list, err := a.svc.MemberShowings(ctx, campaignID, userID)
	if err != nil {
		return nil, 0, err
	}
	return list, len(list), nil
}
