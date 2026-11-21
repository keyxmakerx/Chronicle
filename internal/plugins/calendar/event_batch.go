// Package calendar - event_batch.go saves a whole year-track draft (or undoes
// one) in one request and one database transaction: the events to create, the
// repeating events to change, the events to delete and the weather days to
// write or clear. Every item is judged by the same rules as its single route,
// and one refusal refuses the whole batch, so Keep means all of it or none.
package calendar

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

const (
	// maxBatchEventWrites caps creates, updates and deletes together. A
	// filled year with a second fill replacing the first is a few hundred of
	// each; the cap is well past that yet keeps one transaction, and the
	// request body, from growing without bound.
	maxBatchEventWrites = 2000
	// maxBatchWeatherDays caps the days written, and separately the days
	// cleared, in one batch.
	maxBatchWeatherDays = 2000
)

// EventBatchService is the batch half of the calendar service. It is its own
// interface, reached by a type assertion like EventRollService, so
// CalendarService and its fakes stay as they are.
type EventBatchService interface {
	// ApplyEventBatch applies in to calendarID in one transaction, or changes
	// nothing and returns the refusal.
	ApplyEventBatch(ctx context.Context, calendarID, campaignID string, in EventBatchInput, actor BatchActor) (*EventBatchResult, error)
}

// BatchActor is who sends a batch and what the campaign lets them do. The
// handler fills it from the request; the service never sees Echo.
type BatchActor struct {
	Viewer permissions.Viewer
	UserID string
	// CanDelete is the Owner's right, the same gate as the single delete route.
	CanDelete bool
}

// EventBatchInput is one draft's writes.
type EventBatchInput struct {
	// Create events are made as POST .../events would make them. The caller
	// leaves CanAuthorDmOnly, Author and CreatedBy to ApplyEventBatch.
	Create []CreateEventInput
	// Update items change stored events as PUT .../events/:eid would.
	Update []EventBatchUpdate
	// Delete lists the ids of events to remove.
	Delete []string
	// Weather is optional; Clear runs before Set.
	Weather *EventBatchWeather
}

// EventBatchUpdate is one stored event's partial update.
type EventBatchUpdate struct {
	ID    string
	Input UpdateEventInput
}

// EventBatchWeather is the weather half of a batch.
type EventBatchWeather struct {
	Clear []DayDate
	Set   []DayWeatherInput
}

// EventBatchResult says what a batch did.
type EventBatchResult struct {
	// Created holds the new events' ids, in the order they were sent.
	Created []string `json:"created"`
	Updated int      `json:"updated"`
	Deleted int      `json:"deleted"`
	// WeatherSet and WeatherCleared count the days written and cleared.
	WeatherSet     int `json:"weatherSet"`
	WeatherCleared int `json:"weatherCleared"`
}

// EventBatchWrites is what the repository writes in its one transaction.
type EventBatchWrites struct {
	Create       []*Event
	Update       []*Event
	DeleteIDs    []string
	ClearWeather []DayDate
	SetWeather   []DayWeatherInput
}

// EventBatchRepository writes a batch atomically. It is separate from
// EventRepository because it spans events and weather, which are two
// repositories' tables, in one transaction.
type EventBatchRepository interface {
	ApplyEventBatch(ctx context.Context, calendarID string, w EventBatchWrites) error
}

type eventBatchRepo struct {
	db *sql.DB
}

// NewEventBatchRepository creates the MariaDB-backed batch writer.
func NewEventBatchRepository(db *sql.DB) EventBatchRepository {
	return &eventBatchRepo{db: db}
}

// ApplyEventBatch runs the writes in one transaction, reusing the single
// writes' own SQL so a batched event is stored exactly as a lone one is.
func (r *eventBatchRepo) ApplyEventBatch(ctx context.Context, calendarID string, w EventBatchWrites) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	for _, e := range w.Create {
		if err := insertEvent(ctx, tx, e); err != nil {
			return err
		}
	}
	for _, e := range w.Update {
		if err := updateEventRow(ctx, tx, e); err != nil {
			return err
		}
	}
	for _, id := range w.DeleteIDs {
		// Scoped to the calendar so an id from another one can never go.
		if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_events WHERE id = ? AND calendar_id = ?`, id, calendarID); err != nil {
			return err
		}
	}
	if len(w.ClearWeather) > 0 {
		if err := clearDaysIn(ctx, tx, calendarID, w.ClearWeather); err != nil {
			return err
		}
	}
	if len(w.SetWeather) > 0 {
		if err := setDaysIn(ctx, tx, calendarID, w.SetWeather); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetEventBatchRepository injects the batch writer. Optional wiring like
// SetEventRollRepository: unset, a batch answers that it is unavailable.
func (s *calendarService) SetEventBatchRepository(r EventBatchRepository) { s.batchRepo = r }

// ApplyEventBatch checks every item against the rules of its single route,
// then writes them all in one transaction. Nothing is written, and nothing
// published, unless every item passes.
func (s *calendarService) ApplyEventBatch(ctx context.Context, calendarID, campaignID string, in EventBatchInput, actor BatchActor) (*EventBatchResult, error) {
	if s.batchRepo == nil {
		return nil, apperror.NewInternalMessage("saving a draft in one go is not available", nil)
	}
	weather := in.Weather
	if weather == nil {
		weather = &EventBatchWeather{}
	}
	writes := len(in.Create) + len(in.Update) + len(in.Delete)
	if writes == 0 && len(weather.Clear) == 0 && len(weather.Set) == 0 {
		return nil, apperror.NewBadRequest("the batch has nothing in it")
	}
	if writes > maxBatchEventWrites {
		return nil, apperror.NewBadRequest(fmt.Sprintf("at most %d events can be created, changed and deleted in one batch", maxBatchEventWrites))
	}
	if len(weather.Set) > maxBatchWeatherDays || len(weather.Clear) > maxBatchWeatherDays {
		return nil, apperror.NewBadRequest(fmt.Sprintf("at most %d days of weather can be set, and %d cleared, in one batch", maxBatchWeatherDays, maxBatchWeatherDays))
	}
	// The same gates as the single routes: delete is the Owner's, weather
	// is the Director's. Creating needs Scribe, which the route enforces,
	// and dm_only creation is checked per event below.
	if len(in.Delete) > 0 && !actor.CanDelete {
		return nil, apperror.NewForbidden("insufficient permissions")
	}
	if (len(weather.Set) > 0 || len(weather.Clear) > 0) && !actor.Viewer.SkipsPerUserRules() {
		return nil, apperror.NewForbidden("only the campaign owner or a granted co-DM may set a day's weather")
	}

	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, actor.Viewer)
	if err != nil {
		return nil, err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return nil, err
	}

	var w EventBatchWrites

	// One read finds every stored event the update and delete items name.
	named := map[string]bool{}
	var ids []string
	for _, u := range in.Update {
		if u.ID == "" {
			return nil, apperror.NewBadRequest("every change needs an event id")
		}
		if named[u.ID] {
			return nil, apperror.NewBadRequest("an event can be changed only once in a batch")
		}
		named[u.ID] = true
		ids = append(ids, u.ID)
	}
	deleting := map[string]bool{}
	for _, id := range in.Delete {
		if id == "" {
			return nil, apperror.NewBadRequest("every delete needs an event id")
		}
		if named[id] {
			return nil, apperror.NewBadRequest("an event cannot be both changed and deleted in one batch")
		}
		if deleting[id] {
			return nil, apperror.NewBadRequest("an event can be deleted only once in a batch")
		}
		deleting[id] = true
		ids = append(ids, id)
	}
	stored := map[string]*Event{}
	if len(ids) > 0 {
		found, err := s.eventRepo.GetEventsByIDs(ctx, calendarID, ids)
		if err != nil {
			return nil, fmt.Errorf("get events: %w", err)
		}
		for i := range found {
			if eventVisibleToViewer(found[i], actor.Viewer) {
				stored[found[i].ID] = &found[i]
			}
		}
	}

	result := &EventBatchResult{Created: make([]string, 0, len(in.Create))}
	for _, c := range in.Create {
		c.CanAuthorDmOnly = actor.Viewer.SkipsPerUserRules()
		c.Author = actor.Viewer
		c.CreatedBy = actor.UserID
		evt, err := buildValidatedEvent(calendarID, c)
		if err != nil {
			return nil, err
		}
		if err := s.checkNewEvent(ctx, cal, campaignID, evt, actor.Viewer); err != nil {
			return nil, err
		}
		w.Create = append(w.Create, evt)
		result.Created = append(result.Created, evt.ID)
	}
	// A stored event that is gone, or not this calendar's, or not visible to
	// the sender, is skipped rather than refused: Undo must still work after
	// someone deleted one of the events it would put back or remove, and
	// nothing here says which of the three it was.
	for _, u := range in.Update {
		evt := stored[u.ID]
		if evt == nil {
			continue
		}
		if err := s.mergeEventUpdate(ctx, cal, evt, campaignID, u.Input, actor.Viewer); err != nil {
			return nil, err
		}
		w.Update = append(w.Update, evt)
	}
	for _, id := range in.Delete {
		if stored[id] != nil {
			w.DeleteIDs = append(w.DeleteIDs, id)
		}
	}
	if err := checkDayWeather(cal, weather.Set); err != nil {
		return nil, err
	}
	for _, d := range weather.Clear {
		if err := validateDayDate(cal, d.Year, d.Month, d.Day); err != nil {
			return nil, err
		}
	}
	w.ClearWeather, w.SetWeather = weather.Clear, weather.Set

	if err := s.batchRepo.ApplyEventBatch(ctx, calendarID, w); err != nil {
		return nil, fmt.Errorf("apply event batch: %w", err)
	}

	result.Updated, result.Deleted = len(w.Update), len(w.DeleteIDs)
	result.WeatherSet, result.WeatherCleared = len(w.SetWeather), len(w.ClearWeather)
	for _, e := range w.Create {
		s.publish(PubEventCreated, campaignID, calendarID, e)
	}
	for _, e := range w.Update {
		s.publish(PubEventUpdated, campaignID, calendarID, e)
	}
	for _, id := range w.DeleteIDs {
		s.publish(PubEventDeleted, campaignID, calendarID, map[string]string{"id": id})
	}
	if len(w.SetWeather) > 0 || len(w.ClearWeather) > 0 {
		s.publish(PubWeatherChanged, campaignID, calendarID, nil)
	}
	return result, nil
}
