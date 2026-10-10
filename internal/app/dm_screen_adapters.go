package app

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// The DM Screen owns no data; these adapters let it read the plugins it
// summarises without importing them.

// dmDowntimeAdapter reads the armory's downtime switch and pending moves.
type dmDowntimeAdapter struct {
	stash  armory.StashService
	addons addons.AddonService
}

func (a *dmDowntimeAdapter) Downtime(ctx context.Context, campaignID string, v dmscreen.Viewer) (bool, int, bool, error) {
	on, err := a.addons.IsEnabledForCampaign(ctx, campaignID, armory.AddonSlug)
	if err != nil || !on {
		return false, 0, false, err
	}
	open, err := a.stash.IsDowntimeOpen(ctx, campaignID)
	if err != nil {
		return false, 0, false, err
	}
	// A failed request count must not take the switch away with it.
	page, err := a.stash.StashesPage(ctx, campaignID, armory.Actor{UserID: v.UserID, Role: v.Role})
	if err != nil {
		slog.Warn("dm screen: counting pending stash requests", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return open, 0, true, nil
	}
	return open, page.WaitingCount(), true, nil
}

// SetDowntime switches downtime through the armory, which checks the owner
// rule itself. A campaign without the armory has no downtime to switch.
func (a *dmDowntimeAdapter) SetDowntime(ctx context.Context, campaignID string, v dmscreen.Viewer, open bool) (int, int, error) {
	on, err := a.addons.IsEnabledForCampaign(ctx, campaignID, armory.AddonSlug)
	if err != nil {
		return 0, 0, err
	}
	if !on {
		return 0, 0, apperror.NewNotFound("downtime is not available")
	}
	res, err := a.stash.SetDowntime(ctx, campaignID, armory.Actor{UserID: v.UserID, Role: v.Role}, open)
	if err != nil {
		return 0, 0, err
	}
	return res.Applied, res.Failed, nil
}

// dmWorldAdapter reads the default calendar's date and today's weather.
type dmWorldAdapter struct {
	svc calendar.CalendarService
}

func (a *dmWorldAdapter) World(ctx context.Context, campaignID string, v dmscreen.Viewer) (*dmscreen.WorldView, error) {
	pv := permissions.RequestViewer(v.Role, v.UserID)
	cal, err := a.svc.GetDefaultCalendarForViewer(ctx, campaignID, pv)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	w := &dmscreen.WorldView{CalendarID: cal.ID, DateLabel: cal.FullDateLabel(), TimeLabel: cal.FormatCurrentTime()}
	days, err := a.svc.ListDayWeather(ctx, cal.ID, campaignID, cal.CurrentYear, cal.CurrentMonth, pv)
	if err != nil {
		return w, err
	}
	for _, d := range days {
		if d.Day == cal.CurrentDay {
			w.Weather = weatherLine(d)
			break
		}
	}
	return w, nil
}

// weatherLine renders a day's weather as "Light rain, 11°C, wind W".
func weatherLine(d calendar.DayWeather) string {
	var parts []string
	if d.PresetLabel != nil && *d.PresetLabel != "" {
		parts = append(parts, *d.PresetLabel)
	}
	if d.TemperatureCelsius != nil {
		parts = append(parts, fmt.Sprintf("%.0f°C", *d.TemperatureCelsius))
	}
	if d.Wind != nil && d.Wind.Direction != nil && *d.Wind.Direction != "" {
		parts = append(parts, "wind "+*d.Wind.Direction)
	}
	if len(parts) == 0 && d.Description != nil {
		return *d.Description
	}
	return strings.Join(parts, ", ")
}

// dmNightAdapter finds the next game night in the coming two months (the
// longest window the sessions service lists at once).
type dmNightAdapter struct {
	svc     sessions.SessionService
	members campaigns.CampaignService
}

func (a *dmNightAdapter) NextNight(ctx context.Context, campaignID string, _ dmscreen.Viewer) (*dmscreen.NightView, error) {
	list, err := a.members.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	members := make([]sessions.NightMember, 0, len(list))
	for _, m := range list {
		members = append(members, sessions.NightMember{UserID: m.UserID, Name: m.DisplayName})
	}
	// Same "played only once its date has ended everywhere" rule as the
	// sessions plugin, so a night in progress still counts as next.
	todayT := time.Now().UTC().Add(-14 * time.Hour)
	today := todayT.Format("2006-01-02")
	nights, err := a.svc.ListGameNights(ctx, campaignID, today, todayT.AddDate(0, 0, 60).Format("2006-01-02"), today, members)
	if err != nil {
		return nil, err
	}
	for _, n := range nights {
		if n.Past {
			continue
		}
		return &dmscreen.NightView{
			Name: n.Name, When: nightWhen(n.Date, n.Time),
			Going: n.Tally.Going, Maybe: n.Tally.Maybe, Cant: n.Tally.Cant, NoAnswer: n.Tally.NoAnswer,
		}, nil
	}
	return nil, nil
}

// nightWhen renders "Fri 9 Oct, 19:30" from the stored date and time.
func nightWhen(date, clock string) string {
	when := date
	if t, err := time.Parse("2006-01-02", date); err == nil {
		when = t.Format("Mon 2 Jan")
	}
	if clock != "" {
		when += ", " + clock
	}
	return when
}

// dmPartyAdapter lists claimed characters with their players' names.
type dmPartyAdapter struct {
	entities  entities.EntityService
	campaigns campaigns.CampaignService
}

func (a *dmPartyAdapter) Heroes(ctx context.Context, campaignID string, v dmscreen.Viewer) ([]dmscreen.Hero, error) {
	claimed, err := a.entities.ListClaimed(ctx, campaignID, v.Role, v.UserID)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	if members, err := a.campaigns.ListMembers(ctx, campaignID); err == nil {
		for _, m := range members {
			names[m.UserID] = m.DisplayName
		}
	}
	out := make([]dmscreen.Hero, 0, len(claimed))
	for _, e := range claimed {
		h := dmscreen.Hero{ID: e.ID, Name: e.Name, Fields: e.FieldsData}
		if e.OwnerUserID != nil {
			h.PlayerName = names[*e.OwnerUserID]
			h.PlayerUserID = *e.OwnerUserID
		}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// dmHiddenAdapter lists hidden NPCs and creatures and reveals them.
type dmHiddenAdapter struct {
	entities entities.EntityService
	lists    entities.CharacterListReader
}

// HiddenCharacters asks for hidden entities only, newest first, and takes one
// more than limit per type: that spare row is how it knows the panel cannot
// show them all, without counting every hidden entity in the campaign.
func (a *dmHiddenAdapter) HiddenCharacters(ctx context.Context, campaignID string, v dmscreen.Viewer, limit int) ([]dmscreen.Hidden, bool, error) {
	npcTypes, err := a.lists.NPCTypeIDs(ctx, campaignID)
	if err != nil {
		return nil, false, err
	}
	// A parent type's listing includes its sub-types, so the same entity can
	// come back once per type in the family.
	seen := map[string]bool{}
	var hidden []entities.Entity
	for _, typeID := range npcTypes {
		list, _, err := a.entities.List(ctx, campaignID, typeID, v.Role, v.UserID, entities.ListOptions{Page: 1, PerPage: limit + 1, Sort: "updated", PrivateOnly: true})
		if err != nil {
			return nil, false, err
		}
		for _, e := range list {
			if e.IsPrivate && !seen[e.ID] {
				seen[e.ID] = true
				hidden = append(hidden, e)
			}
		}
	}
	sort.Slice(hidden, func(i, j int) bool { return hidden[i].UpdatedAt.After(hidden[j].UpdatedAt) })
	more := len(hidden) > limit
	if more {
		hidden = hidden[:limit]
	}
	out := make([]dmscreen.Hidden, 0, len(hidden))
	for _, e := range hidden {
		out = append(out, dmscreen.Hidden{ID: e.ID, Name: e.Name, TypeName: e.TypeName})
	}
	return out, more, nil
}

// Reveal only ever un-hides, and only the NPC and creature types the panel
// lists: setting the flag (never toggling it) keeps a double-click from
// hiding the entity again.
func (a *dmHiddenAdapter) Reveal(ctx context.Context, entityID, campaignID string) (string, error) {
	e, err := a.entities.GetByID(ctx, entityID)
	if err != nil {
		return "", err
	}
	if e.CampaignID != campaignID {
		return "", apperror.NewNotFound("entity not found")
	}
	npcTypes, err := a.lists.NPCTypeIDs(ctx, campaignID)
	if err != nil {
		return "", err
	}
	if !slices.Contains(npcTypes, e.EntityTypeID) {
		return "", apperror.NewNotFound("entity not found")
	}
	if !e.IsPrivate {
		return e.Name, nil
	}
	if err := a.entities.SetPrivateInCampaign(ctx, entityID, campaignID, false); err != nil {
		return "", err
	}
	return e.Name, nil
}

// foundryReportMaxAge is how old the GM's Foundry players report may be and
// still count: the GM's client re-sends the list as people come and go, so a
// stale list says nothing about who is online now.
const foundryReportMaxAge = 5 * time.Minute

// dmBrowserHub is the slice of the websocket hub the presence adapter reads.
type dmBrowserHub interface {
	BrowserUserIDs(campaignID string) []string
	FoundryPresence(campaignID string) (*time.Time, bool)
}

// dmPresenceAdapter says which of a campaign's players are here. A player is
// here when their browser has a live socket to Chronicle, or when the GM's
// Foundry client reports them online (and is itself connected and fresh).
//
// Browser sockets are opened only by pages with live widgets (notes, journal,
// maps, quest boards), not by every campaign page, so a player reading a plain
// page is invisible to the first signal; the Foundry report covers players at
// the table.
type dmPresenceAdapter struct {
	members campaigns.CampaignService
	hub     dmBrowserHub
	foundry syncapi.FoundryPlayerRepository
	now     func() time.Time
}

func (a *dmPresenceAdapter) Players(ctx context.Context, campaignID string) ([]dmscreen.PlayerPresence, error) {
	members, err := a.members.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	var reports []syncapi.FoundryPlayer
	_, foundryUp := a.hub.FoundryPresence(campaignID)
	if a.foundry != nil && foundryUp {
		// A failed read only loses the Foundry signal; browser presence stands.
		if reports, err = a.foundry.List(ctx, campaignID); err != nil {
			slog.Warn("dm screen: reading foundry players", slog.String("campaign_id", campaignID), slog.Any("error", err))
			reports = nil
		}
	}
	here := hereUsers(a.hub.BrowserUserIDs(campaignID), reports, foundryUp, a.clock())

	var out []dmscreen.PlayerPresence
	for _, m := range members {
		if m.Role != campaigns.RolePlayer {
			continue
		}
		// A DM grant makes a Player-role member a co-DM, who runs the game.
		if granted, err := a.members.IsUserDmGranted(ctx, campaignID, m.UserID); err == nil && granted {
			continue
		}
		out = append(out, dmscreen.PlayerPresence{UserID: m.UserID, Name: m.DisplayName, Here: here[m.UserID]})
	}
	return out, nil
}

func (a *dmPresenceAdapter) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// hereUsers merges the two signals. A Foundry row counts only when the GM's
// client is connected now, the row is linked to a member, it says online, and
// the report is recent enough to trust.
func hereUsers(browser []string, reports []syncapi.FoundryPlayer, foundryUp bool, now time.Time) map[string]bool {
	here := map[string]bool{}
	for _, id := range browser {
		here[id] = true
	}
	if !foundryUp {
		return here
	}
	for _, p := range reports {
		if p.MemberUserID == "" || !p.Online || now.Sub(p.ReportedAt) > foundryReportMaxAge {
			continue
		}
		here[p.MemberUserID] = true
	}
	return here
}

// dmScreenSyncAPIAdapter hands the DM Screen to the sync API for the Foundry
// module, which reads the same View the site panel draws.
type dmScreenSyncAPIAdapter struct {
	svc dmscreen.Service
}

func (a *dmScreenSyncAPIAdapter) Screen(ctx context.Context, campaignID, userID string, role int) (any, error) {
	return a.svc.Build(ctx, campaignID, dmscreen.Viewer{UserID: userID, Role: role})
}

func (a *dmScreenSyncAPIAdapter) Reveal(ctx context.Context, entityID, campaignID, userID string, role int) (string, error) {
	return a.svc.Reveal(ctx, entityID, campaignID, dmscreen.Viewer{UserID: userID, Role: role})
}

func (a *dmScreenSyncAPIAdapter) SetDowntime(ctx context.Context, campaignID, userID string, role int, open bool) (any, error) {
	return a.svc.SetDowntime(ctx, campaignID, dmscreen.Viewer{UserID: userID, Role: role}, open)
}
