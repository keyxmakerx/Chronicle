package app

import (
	"context"
	"encoding/json"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/cutscenes"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
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

// Answered tells the Director's browser tabs to redraw the scene's panel,
// and the showing's other viewers, in Chronicle and through the module in
// Foundry, who is adding to the story. Neither carries the answer's words.
func (p *cutsceneLivePublisher) Answered(campaignID string, s cutscenes.Showing, a cutscenes.Answer) {
	if p.bus == nil {
		return
	}
	dir := ws.NewMessage(ws.MsgCutsceneMomentAnswer, campaignID, s.ID, map[string]any{
		"showingId": s.ID, "sceneId": s.SceneID, "moment": a.Moment,
	})
	dir.RequiresDM = true
	dir.Sources = []string{ws.BrowserSource}
	p.bus.Publish(dir)

	var others []string
	for _, id := range viewerIDs(s) {
		if id != a.UserID {
			others = append(others, id)
		}
	}
	payload := map[string]any{"showingId": s.ID, "moment": a.Moment, "userId": a.UserID, "name": a.Name}
	if len(others) > 0 {
		live := ws.NewMessage(ws.MsgCutsceneMomentAnswering, campaignID, s.ID, payload)
		live.AllowedUsers, live.StrictAudience = others, true
		live.Sources = []string{ws.BrowserSource}
		p.bus.Publish(live)
	}
	relay := ws.NewMessage(ws.MsgCutsceneFoundryAnswering, campaignID, s.ID, map[string]any{
		"showingId": s.ID, "moment": a.Moment, "userId": a.UserID, "name": a.Name, "viewers": viewerIDs(s),
	})
	relay.RequiresDM = true
	relay.Sources = []string{foundry_vtt.ModuleSource}
	p.bus.Publish(relay)
}

// Go lets moment i go on the viewers' screens with the lines that joined
// the story there, in Chronicle and through the module in Foundry.
func (p *cutsceneLivePublisher) Go(campaignID string, s cutscenes.Showing, i int, lines []cutscenes.ShownLine) {
	if p.bus == nil || len(s.Viewers) == 0 {
		return
	}
	if lines == nil {
		lines = []cutscenes.ShownLine{}
	}
	live := ws.NewMessage(ws.MsgCutsceneMomentGo, campaignID, s.ID, map[string]any{"showingId": s.ID, "moment": i, "lines": lines})
	live.AllowedUsers, live.StrictAudience = viewerIDs(s), true
	live.Sources = []string{ws.BrowserSource}
	p.bus.Publish(live)

	relay := ws.NewMessage(ws.MsgCutsceneFoundryGo, campaignID, s.ID, map[string]any{
		"showingId": s.ID, "moment": i, "lines": lines, "viewers": viewerIDs(s),
	})
	relay.RequiresDM = true
	relay.Sources = []string{foundry_vtt.ModuleSource}
	p.bus.Publish(relay)
}

// cutsceneFoundryPreview implements cutscenes.FoundryPreviewer. A campaign
// uses Foundry when its Sync API add-on is on (making a Foundry key turns it
// on); a module is there to hear a preview when the hub has seen its socket
// lately. hub and bus are bound once they exist; until then nothing is sent.
type cutsceneFoundryPreview struct {
	hub interface {
		FoundryPresence(string) (*time.Time, bool)
	}
	bus     ws.EventBus
	enabled func(ctx context.Context, campaignID string) (bool, error)
}

func (p *cutsceneFoundryPreview) Offered(ctx context.Context, campaignID string) bool {
	if p.enabled == nil {
		return false
	}
	on, err := p.enabled(ctx, campaignID)
	return err == nil && on
}

// Preview sends the scene to the module's DM-held sockets only, never a
// browser, with who pressed it so the module picks that Director's screen.
func (p *cutsceneFoundryPreview) Preview(campaignID, directorID string, sc cutscenes.Scene) bool {
	if p.hub == nil || p.bus == nil {
		return false
	}
	if _, connected := p.hub.FoundryPresence(campaignID); !connected {
		return false
	}
	id := uuid.NewString()
	msg := ws.NewMessage(ws.MsgCutsceneFoundryPreview, campaignID, id, map[string]any{
		"previewId": id, "director": directorID, "scene": sc,
	})
	msg.RequiresDM = true
	msg.Sources = []string{foundry_vtt.ModuleSource}
	p.bus.Publish(msg)
	return true
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

func (a cutsceneSyncAdapter) Answer(ctx context.Context, campaignID, showingID string, moment int, userID, way, text string) (any, error) {
	return a.svc.Answer(ctx, campaignID, showingID, moment, cutscenes.Viewer{UserID: userID}, cutscenes.AnswerInput{Way: way, Text: text})
}

// cutsceneStoryAdapter implements cutscenes.Story over the entities
// service: the pages a Director can add a moment's line to, and the append.
type cutsceneStoryAdapter struct {
	svc entities.EntityService
}

func (a cutsceneStoryAdapter) Pages(ctx context.Context, campaignID string) ([]cutscenes.PageRef, error) {
	// The Director sees every page, so the owner's view lists them all.
	list, err := a.svc.ListEntityNames(ctx, campaignID, int(campaigns.RoleOwner), "")
	if err != nil {
		return nil, err
	}
	out := make([]cutscenes.PageRef, 0, len(list))
	for _, e := range list {
		if !e.IsAlias {
			out = append(out, cutscenes.PageRef{ID: e.ID, Name: e.Name})
		}
	}
	return out, nil
}

// AddLine appends the line as its own paragraph at the end of the page's
// words, followed by who it came from. It saves against the page's revision
// and tries again when someone saved the page in between, so an open editor
// never loses its words to this or this to it.
func (a cutsceneStoryAdapter) AddLine(ctx context.Context, campaignID, pageID, line, from string) error {
	for try := 0; try < 3; try++ {
		rev, err := a.svc.EntryRev(ctx, pageID)
		if err != nil {
			return err
		}
		e, err := a.svc.GetByID(ctx, pageID)
		if err != nil {
			return err
		}
		if e.CampaignID != campaignID {
			return apperror.NewNotFound("page not found")
		}
		doc, htm, err := appendStoryLine(e.Entry, e.EntryHTML, line, from)
		if err != nil {
			return err
		}
		_, conflict, err := a.svc.SaveEntry(ctx, pageID, doc, htm, &rev)
		if err != nil {
			return err
		}
		if conflict == nil {
			return nil
		}
	}
	return apperror.NewConflict("The page was being edited. Try Add to the story again.")
}

// appendStoryLine is a page's entry with one paragraph added at the end: the
// line in quotes, then "from <name>" in italics. An empty page starts a new
// document; words that can't be read as one are refused, never replaced.
func appendStoryLine(entry, entryHTML *string, line, from string) (string, string, error) {
	para := map[string]any{"type": "paragraph", "content": []any{
		map[string]any{"type": "text", "text": "“" + line + "” "},
		map[string]any{"type": "text", "text": "from " + from, "marks": []any{map[string]any{"type": "italic"}}},
	}}
	doc := map[string]any{"type": "doc"}
	if entry != nil && strings.TrimSpace(*entry) != "" {
		doc = map[string]any{}
		if err := json.Unmarshal([]byte(*entry), &doc); err != nil || doc["type"] != "doc" {
			return "", "", apperror.NewConflict("That page's words can't be added to here. Open the page and add the line by hand.")
		}
	}
	content, _ := doc["content"].([]any)
	doc["content"] = append(content, para)
	b, err := json.Marshal(doc)
	if err != nil {
		return "", "", err
	}
	prev := ""
	if entryHTML != nil {
		prev = *entryHTML
	}
	return string(b), prev + "<p>“" + html.EscapeString(line) + "” <em>from " + html.EscapeString(from) + "</em></p>", nil
}
