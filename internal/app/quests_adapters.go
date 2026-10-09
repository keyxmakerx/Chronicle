package app

// quests_adapters.go bridges the quests plugin to the entities, maps and
// campaigns services. The quests plugin owns the narrow interfaces; these
// adapters live here so it never imports another plugin.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// questEntityAdapter implements quests.EntityDirectory over the entities
// service, with names read in one query per load through PageCards.
type questEntityAdapter struct {
	svc   entities.EntityService
	cards entities.PageCards
}

func entityInfo(e *entities.Entity) quests.EntityInfo {
	info := quests.EntityInfo{ID: e.ID, Name: e.Name, TypeName: e.TypeName, TypeSlug: e.TypeSlug}
	if e.ImagePath != nil {
		info.ImagePath = *e.ImagePath
	}
	return info
}

// Entities skips a missing page and one in another campaign (a clean absence,
// not an error), so a foreign id can never be resolved through this plugin.
func (a *questEntityAdapter) Entities(ctx context.Context, campaignID string, ids []string) (map[string]quests.EntityInfo, error) {
	cards, err := a.cards.InCampaign(ctx, campaignID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]quests.EntityInfo, len(cards))
	for id, c := range cards {
		out[id] = quests.EntityInfo{ID: c.ID, Name: c.Name, TypeName: c.TypeName, TypeSlug: c.TypeSlug, ImagePath: c.ImagePath}
	}
	return out, nil
}

// FilterViewable delegates to the entities plugin's one visibility predicate.
func (a *questEntityAdapter) FilterViewable(ctx context.Context, campaignID string, ids []string, role int, userID string) (map[string]bool, error) {
	return a.svc.FilterViewableEntityIDs(ctx, campaignID, ids, role, userID)
}

// Search reuses the entity name search (all types).
func (a *questEntityAdapter) Search(ctx context.Context, campaignID, query string, role int, userID string, limit int) ([]quests.EntityInfo, error) {
	opts := entities.DefaultListOptions()
	opts.PerPage = limit
	found, _, err := a.svc.Search(ctx, campaignID, query, 0, role, userID, opts)
	if err != nil {
		return nil, err
	}
	out := make([]quests.EntityInfo, 0, len(found))
	for i := range found {
		out = append(out, entityInfo(&found[i]))
	}
	return out, nil
}

// questMapAdapter implements quests.MapDirectory over the maps service. With
// the maps addon off it finds no maps, so quests cannot pin or link one and
// never points at the addon's routes.
type questMapAdapter struct {
	svc    maps.MapService
	addons questAddonChecker
}

// questAddonChecker is the slice of the addons service the map adapter needs.
type questAddonChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
}

func (a *questMapAdapter) Enabled(ctx context.Context, campaignID string) (bool, error) {
	return a.addons.IsEnabledForCampaign(ctx, campaignID, "maps")
}

// Maps reads the campaign's map list once and picks the requested ones, so a
// board with many map pins costs one lookup, and a foreign id is never found.
func (a *questMapAdapter) Maps(ctx context.Context, campaignID string, ids []string) (map[string]quests.MapInfo, error) {
	out := make(map[string]quests.MapInfo, len(ids))
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			want[id] = true
		}
	}
	if len(want) == 0 {
		return out, nil
	}
	ms, err := a.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		if want[m.ID] {
			out[m.ID] = quests.MapInfo{ID: m.ID, Name: m.Name}
		}
	}
	return out, nil
}

func (a *questMapAdapter) ListMaps(ctx context.Context, campaignID string) ([]quests.MapInfo, error) {
	on, err := a.Enabled(ctx, campaignID)
	if err != nil || !on {
		return nil, err
	}
	ms, err := a.svc.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]quests.MapInfo, 0, len(ms))
	for _, m := range ms {
		out = append(out, quests.MapInfo{ID: m.ID, Name: m.Name})
	}
	return out, nil
}

// questMemberNamesAdapter implements quests.MemberNames from the member list.
type questMemberNamesAdapter struct {
	svc campaigns.CampaignService
}

func (a *questMemberNamesAdapter) DisplayNames(ctx context.Context, campaignID string, _ []string) (map[string]string, error) {
	members, err := a.svc.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(members))
	for _, m := range members {
		out[m.UserID] = m.DisplayName
	}
	return out, nil
}

// questCharacterAdapter implements quests.CharacterDirectory over the Armory's
// character listing (the same family the Armory's own give box offers), with
// the claiming member's display name.
type questCharacterAdapter struct {
	dir   *armoryStashDirectoryAdapter
	names *questMemberNamesAdapter
}

func (a *questCharacterAdapter) ListCharacters(ctx context.Context, campaignID string, role int, userID string) ([]quests.CharacterInfo, error) {
	chars, err := a.dir.ListCharacters(ctx, campaignID, role, userID)
	if err != nil {
		return nil, err
	}
	names, err := a.names.DisplayNames(ctx, campaignID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]quests.CharacterInfo, 0, len(chars))
	for _, c := range chars {
		info := quests.CharacterInfo{ID: c.ID, Name: c.Name}
		if c.OwnerUserID != "" {
			info.Player = names[c.OwnerUserID]
			if info.Player == "" {
				info.Player = "a player"
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// questTypeAdapter implements quests.TypeDirectory over the entities service.
type questTypeAdapter struct {
	svc entities.EntityService
}

// TypeInCampaign treats a missing type and one of another campaign alike, as
// not found, so a foreign category id cannot be probed through the boards.
func (a *questTypeAdapter) TypeInCampaign(ctx context.Context, campaignID string, typeID int) (bool, error) {
	et, err := a.svc.GetEntityTypeByID(ctx, typeID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return et.CampaignID == campaignID, nil
}

// questAnnouncerAdapter implements quests.Announcer over the websocket bus.
type questAnnouncerAdapter struct {
	bus ws.EventBus
}

func (a *questAnnouncerAdapter) QuestChanged(campaignID, entityID string, version int, dmOnly bool) {
	msg := ws.NewMessage(ws.MsgQuestUpdated, campaignID, entityID, map[string]int{"version": version})
	msg.RequiresDM = dmOnly
	a.bus.Publish(msg)
}

func (a *questAnnouncerAdapter) BoardsChanged(campaignID string, h quests.Home, dmOnly bool) {
	id, home := h.EntityID, "page"
	if h.IsType() {
		id, home = strconv.Itoa(h.TypeID), "category"
	}
	msg := ws.NewMessage(ws.MsgNoticeBoardsUpdated, campaignID, id, map[string]string{"home": home})
	msg.RequiresDM = dmOnly
	a.bus.Publish(msg)
}

// questCalendarAdapter implements quests.CalendarDirectory over the calendar
// service. With the calendar addon off, or no calendar, it reports none, so a
// campaign without one has no due dates and never reaches the addon's data.
type questCalendarAdapter struct {
	svc    calendar.CalendarService
	addons questAddonChecker
}

// questCalendarViewer is a trusted in-process caller: the quests routes have
// already decided who may do this, and the event is the DM's own record of a
// quest, so it may author dm_only content and see the calendar whatever its
// visibility.
func questCalendarViewer() permissions.Viewer { return permissions.SystemViewer(permissions.RoleOwner) }

func (a *questCalendarAdapter) Calendar(ctx context.Context, campaignID string) (quests.QuestCalendar, error) {
	on, err := a.addons.IsEnabledForCampaign(ctx, campaignID, calendar.PluginSlug)
	if err != nil || !on {
		return nil, err
	}
	cal, err := a.svc.GetPrimaryCalendarForViewer(ctx, campaignID, questCalendarViewer())
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &questCalendar{cal: cal}, nil
}

func eventVisibility(dmOnly bool) string {
	if dmOnly {
		return "dm_only"
	}
	return "everyone"
}

// SaveDueEvent updates the quest's event and falls back to creating one when
// it is gone (deleted by hand, or its calendar is no longer the primary one).
// An update sends only what this feature owns: name, link, date, visibility.
func (a *questCalendarAdapter) SaveDueEvent(ctx context.Context, campaignID string, qc quests.QuestCalendar, eventID string, ev quests.DueEvent) (string, error) {
	calID := qc.ID()
	vis := eventVisibility(ev.DMOnly)
	if eventID != "" {
		err := a.svc.UpdateEvent(ctx, eventID, calID, campaignID, calendar.UpdateEventInput{
			Name:       patch.Of(ev.Title),
			EntityID:   patch.Of(ev.EntityID),
			Year:       patch.Of(ev.Day.Year),
			Month:      patch.Of(ev.Day.Month),
			Day:        patch.Of(ev.Day.Day),
			AllDay:     patch.Of(true),
			Visibility: patch.Of(vis),
		}, questCalendarViewer())
		if err == nil {
			return eventID, nil
		}
		if !isNotFound(err) {
			return "", err
		}
	}
	entityID := ev.EntityID
	announced := calendar.AnnouncedAhead // a due date is knowable before the day, not only on it
	created, err := a.svc.CreateEvent(ctx, calID, campaignID, calendar.CreateEventInput{
		Name:            ev.Title,
		EntityID:        &entityID,
		Year:            ev.Day.Year,
		Month:           ev.Day.Month,
		Day:             ev.Day.Day,
		AllDay:          true,
		Visibility:      vis,
		Announced:       &announced,
		CreatedBy:       ev.CreatedBy,
		CanAuthorDmOnly: true,
		Author:          questCalendarViewer(),
	})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// SetDueEventVisibility flips only the visibility, and only when it differs,
// so an entity update that did not change who can see the page writes nothing.
func (a *questCalendarAdapter) SetDueEventVisibility(ctx context.Context, campaignID string, qc quests.QuestCalendar, eventID string, dmOnly bool) error {
	v := questCalendarViewer()
	evt, err := a.svc.GetEventForViewer(ctx, eventID, qc.ID(), campaignID, v)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if (evt.Visibility == "dm_only") == dmOnly {
		return nil
	}
	err = a.svc.SetEventVisibility(ctx, eventID, qc.ID(), campaignID, calendar.UpdateEventVisibilityInput{Visibility: eventVisibility(dmOnly)}, v)
	if isNotFound(err) {
		return nil
	}
	return err
}

func (a *questCalendarAdapter) DeleteDueEvent(ctx context.Context, campaignID string, qc quests.QuestCalendar, eventID string) error {
	err := a.svc.DeleteEvent(ctx, eventID, qc.ID(), campaignID, questCalendarViewer())
	if isNotFound(err) {
		return nil
	}
	return err
}

// questCalendar implements quests.QuestCalendar over a loaded calendar. The
// date maths is the calendar's own; nothing here re-derives a leap rule.
type questCalendar struct {
	cal *calendar.Calendar
}

func (c *questCalendar) ID() string   { return c.cal.ID }
func (c *questCalendar) Name() string { return c.cal.Name }

func (c *questCalendar) Today() quests.DueDay {
	return quests.DueDay{Year: c.cal.CurrentYear, Month: c.cal.CurrentMonth, Day: c.cal.CurrentDay}
}

func (c *questCalendar) Months() []quests.CalendarMonth {
	out := make([]quests.CalendarMonth, 0, len(c.cal.Months))
	for _, m := range c.cal.Months {
		out = append(out, quests.CalendarMonth{Name: m.Name, Days: m.Days, LeapDays: m.LeapYearDays})
	}
	return out
}

func (c *questCalendar) Leap() (int, int) { return c.cal.LeapYearEvery, c.cal.LeapYearOffset }

// Valid is the validation AbsoluteDay does not do: a month the calendar has
// and a day that month has in that year (leap days included).
func (c *questCalendar) Valid(d quests.DueDay) bool {
	if d.Month < 1 || d.Month > len(c.cal.Months) {
		return false
	}
	return d.Day >= 1 && d.Day <= c.cal.MonthDays(d.Month-1, d.Year)
}

// Label is FullDateLabel for the due day: the calendar reads its formatting
// from the current-date fields, so a copy with those set to d formats it.
func (c *questCalendar) Label(d quests.DueDay) string {
	at := *c.cal
	at.CurrentYear, at.CurrentMonth, at.CurrentDay = d.Year, d.Month, d.Day
	return at.FullDateLabel()
}

func (c *questCalendar) DaysFromToday(d quests.DueDay) int {
	return c.cal.AbsoluteDay(d.Year, d.Month, d.Day) - c.cal.CurrentAbsoluteDay()
}

// questDueSyncer is the slice of the quest service the entity-event fan-out
// needs.
type questDueSyncer interface {
	SyncDueEvent(ctx context.Context, campaignID, entityID string) error
	RemoveDueEvent(ctx context.Context, campaignID, entityID string) error
}

// questEntityEvents wraps the entity event publisher so a page's visibility
// change reaches the quest's calendar event. The quests service is built
// after the publisher is installed, so it is attached later with attach.
type questEntityEvents struct {
	next entities.EntityEventPublisher

	mu     sync.Mutex // also serialises syncs so the last one reads the latest state
	quests questDueSyncer
}

func newQuestEntityEvents(next entities.EntityEventPublisher) *questEntityEvents {
	return &questEntityEvents{next: next}
}

func (p *questEntityEvents) attach(q questDueSyncer) {
	p.mu.Lock()
	p.quests = q
	p.mu.Unlock()
}

func (p *questEntityEvents) PublishEntityTypeEvent(eventType, campaignID string, et *entities.EntityType) {
	p.next.PublishEntityTypeEvent(eventType, campaignID, et)
}

// PublishEntityEvent forwards the event first, then syncs in the background:
// the entity write has finished and must never wait on, or fail from, the
// calendar. Only "updated" and "deleted" can change a due event.
func (p *questEntityEvents) PublishEntityEvent(eventType, campaignID, entityID string, entity *entities.Entity) {
	p.next.PublishEntityEvent(eventType, campaignID, entityID, entity)
	if campaignID == "" || entityID == "" || (eventType != "updated" && eventType != "deleted") {
		return
	}
	go p.sync(eventType, campaignID, entityID)
}

func (p *questEntityEvents) sync(eventType, campaignID, entityID string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("quest due event sync panicked", slog.Any("panic", r), slog.String("entity_id", entityID))
		}
	}()
	// The publisher carries no request context, so a bounded one of its own.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.quests == nil {
		return
	}
	var err error
	if eventType == "deleted" {
		err = p.quests.RemoveDueEvent(ctx, campaignID, entityID)
	} else {
		err = p.quests.SyncDueEvent(ctx, campaignID, entityID)
	}
	if err != nil {
		slog.Warn("quest due event sync failed", slog.String("campaign_id", campaignID), slog.String("entity_id", entityID), slog.String("event", eventType), slog.Any("error", err))
	}
}
