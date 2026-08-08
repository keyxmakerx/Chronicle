// entity_ties_service.go — service-layer logic for entity<->event/era ties
// (C-CAL-ENTITY-TIES-DATA-MODEL). Owns enum validation; the link/unlink and
// both-direction queries delegate to the repo. Cascade-on-delete is
// DB-enforced (ON DELETE CASCADE), so there is no cascade orchestration here.
package calendar

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// LinkEntityToEvent ties an entity to an event with a participation role.
// Empty role defaults to "involved" (the picker default); an invalid role is
// rejected. Idempotent: re-linking updates the role.
func (s *calendarService) LinkEntityToEvent(ctx context.Context, entityID, eventID, role string) error {
	pr, err := validateEventRole(role)
	if err != nil {
		return err
	}
	return s.repo.LinkEntityEvent(ctx, entityID, eventID, string(pr))
}

// UnlinkEntityFromEvent removes an entity<->event tie.
func (s *calendarService) UnlinkEntityFromEvent(ctx context.Context, entityID, eventID string) error {
	return s.repo.UnlinkEntityEvent(ctx, entityID, eventID)
}

// CreateEntityFromEvent creates an entity (via the cross-plugin EntityCreator)
// named after the event, then links it to that event with the default
// "involved" role. The entity exists even if the link fails, so a link error is
// surfaced (not swallowed) for an explicit retry. C-CAL-EDITOR-EXPANSION PR1.
func (s *calendarService) CreateEntityFromEvent(ctx context.Context, creator EntityCreator, campaignID, userID string, typeID int, name, eventID string) (string, error) {
	if creator == nil {
		return "", apperror.NewInternal(fmt.Errorf("entity creator not configured"))
	}
	entityID, err := creator.CreateEntity(ctx, campaignID, userID, typeID, name)
	if err != nil {
		return "", err
	}
	if err := s.repo.LinkEntityEvent(ctx, entityID, eventID, string(ParticipationRoles[0])); err != nil {
		return "", apperror.NewInternal(fmt.Errorf("linking new entity %s to event %s: %w", entityID, eventID, err))
	}
	return entityID, nil
}

// LinkEntityToEra ties an entity to an era with an optional role (era ties are
// coarser). A nil/empty role stores NULL; a non-empty invalid role is
// rejected. Idempotent.
func (s *calendarService) LinkEntityToEra(ctx context.Context, entityID string, eraID int, role *string) error {
	pr, err := validateEraRole(role)
	if err != nil {
		return err
	}
	return s.repo.LinkEntityEra(ctx, entityID, eraID, pr)
}

// UnlinkEntityFromEra removes an entity<->era tie.
func (s *calendarService) UnlinkEntityFromEra(ctx context.Context, entityID string, eraID int) error {
	return s.repo.UnlinkEntityEra(ctx, entityID, eraID)
}

// EventsForEntity returns the events tied to an entity (entity-side query).
func (s *calendarService) EventsForEntity(ctx context.Context, entityID string) ([]EntityEventTie, error) {
	return s.repo.EventsForEntity(ctx, entityID)
}

// EventsForEntityFiltered is the viewer-filtered sibling of EventsForEntity
// (C-CALV4-TIEFIX-PB Bug 1 item 3). EventsForEntity's query joins only
// entity_event_links + calendar_events, never calendars, so calendar-level
// visibility is invisible to it: an event on a dm_only *calendar* is
// reachable through an entity tie even when the event's own visibility is
// 'everyone'. This method closes that gap.
//
// Composed from two EXISTING CalendarRepository methods (EventsForEntity +
// GetByID) rather than a new SQL-joining repo method: a repo-level join would
// need a new CalendarRepository interface method, which forces updating the
// hand-written mockCalendarRepo in service_test.go — a file this dispatch
// does not own, and COMMON's Bounds section explicitly forbids widening that
// ~60-method interface (two other in-flight C-CALV4 slices depend on that
// test file's build staying untouched). This composition enforces the
// identical policy — calendarVisibleTo + canUserView, the same two predicates
// every other visibility check in this package uses — without touching that
// interface. Calendars are fetched once per DISTINCT CalendarID among the
// ties (cached in a map), not once per event.
//
// Deliberately a NEW method rather than a signature change to EventsForEntity:
// entity_calendar_block.go (owned by a different C-CALV4 slice — not in this
// dispatch's file list) still calls the old, unfiltered signature today;
// changing it would collide. Originally concrete-only on *calendarService for
// the same file-ownership reason; C-CALV4-SEAM-P5 §7 put it on the
// CalendarService interface because its only planned consumer — the phase-B
// entity-page host — lives outside this package and cannot type-assert an
// unexported type. Wiring this into entity_calendar_block.go's call site —
// replacing its lines 78-91, which only apply the per-event canUserView half
// of this check — is follow-up work for whoever owns that file.
//
// Rows come from ONE pass over the raw tie list into a freshly allocated
// slice — never a second pass over an already-filtered slice, which would
// corrupt filterEventsByUser-style events[:0] in-place filtering (COMMON §7).
func (s *calendarService) EventsForEntityFiltered(ctx context.Context, entityID string, role int, userID string) ([]EntityEventTie, error) {
	all, err := s.repo.EventsForEntity(ctx, entityID)
	if err != nil {
		return nil, err
	}
	// C-AUTHZ-EMPTY-USERID / ADR-049: the bypass is a stated property of the
	// viewer, never `userID == ""` — that value is what an ANONYMOUS request
	// carries, and it must take the strictest path, not the most trusting one.
	viewer := permissions.RequestViewer(role, userID)
	if viewer.SkipsPerUserRules() {
		return all, nil
	}

	cals := make(map[string]*Calendar, 1)
	out := make([]EntityEventTie, 0, len(all))
	for _, tie := range all {
		cal, seen := cals[tie.Event.CalendarID]
		if !seen {
			cal, err = s.repo.GetByID(ctx, tie.Event.CalendarID)
			if err != nil {
				return nil, err
			}
			cals[tie.Event.CalendarID] = cal
		}
		if !calendarVisibleTo(cal, viewer) {
			continue
		}
		if !canUserView(tie.Event.Visibility, tie.Event.VisibilityRules, role, userID) {
			continue
		}
		out = append(out, tie)
	}
	return out, nil
}

// ErasForEntity returns the eras tied to an entity (entity-side query).
func (s *calendarService) ErasForEntity(ctx context.Context, entityID string) ([]EntityEraTie, error) {
	return s.repo.ErasForEntity(ctx, entityID)
}

// EntitiesForEvent returns the entities tied to an event (event-side query).
// role + userID are the viewer context forwarded to the repo so entity
// visibility is enforced (cordinator#32 gap #1 / C-CAL-ENTITY-TIES-LEAK-FIX).
func (s *calendarService) EntitiesForEvent(ctx context.Context, eventID string, role int, userID string) ([]EntityTieRef, error) {
	return s.repo.EntitiesForEvent(ctx, eventID, role, userID)
}

// EntitiesForEra returns the entities tied to an era (era-side query). Same
// viewer-context gating as EntitiesForEvent above.
func (s *calendarService) EntitiesForEra(ctx context.Context, eraID int, role int, userID string) ([]EntityTieRef, error) {
	return s.repo.EntitiesForEra(ctx, eraID, role, userID)
}

// EntitiesForCalendar returns the distinct entities tied to any event/era of a
// calendar (the Calendars dashboard associations panel, W1). role + userID are
// the viewer context threaded to the repo so entity visibility is enforced
// (cordinator#32 gap #1) — owners/co-DMs see all, players only entities they
// may themselves see.
func (s *calendarService) EntitiesForCalendar(ctx context.Context, calendarID string, role int, userID string) ([]EntityTieRef, error) {
	return s.repo.EntitiesForCalendar(ctx, calendarID, role, userID)
}
