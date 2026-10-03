package app

import (
	"context"
	"fmt"
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
	page, err := a.stash.StashesPage(ctx, campaignID, armory.Actor{UserID: v.UserID, Role: v.Role})
	if err != nil {
		return open, 0, true, err
	}
	return open, len(page.Pending), true, nil
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
		}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// dmHiddenAdapter lists hidden NPCs and creatures and reveals them.
type dmHiddenAdapter struct {
	entities entities.EntityService
}

func (a *dmHiddenAdapter) HiddenCharacters(ctx context.Context, campaignID string, v dmscreen.Viewer, limit int) ([]dmscreen.Hidden, error) {
	types, err := a.entities.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	var hidden []entities.Entity
	for _, typeID := range npcTypeIDs(types) {
		list, _, err := a.entities.List(ctx, campaignID, typeID, v.Role, v.UserID, entities.ListOptions{Page: 1, PerPage: 50, Sort: "updated"})
		if err != nil {
			return nil, err
		}
		for _, e := range list {
			if e.IsPrivate {
				hidden = append(hidden, e)
			}
		}
	}
	sort.Slice(hidden, func(i, j int) bool { return hidden[i].UpdatedAt.After(hidden[j].UpdatedAt) })
	if len(hidden) > limit {
		hidden = hidden[:limit]
	}
	out := make([]dmscreen.Hidden, 0, len(hidden))
	for _, e := range hidden {
		out = append(out, dmscreen.Hidden{ID: e.ID, Name: e.Name, TypeName: e.TypeName})
	}
	return out, nil
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
	types, err := a.entities.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return "", err
	}
	if !slices.Contains(npcTypeIDs(types), e.EntityTypeID) {
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
