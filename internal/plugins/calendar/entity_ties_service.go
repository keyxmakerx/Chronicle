// entity_ties_service.go — service-layer logic for entity<->event/era ties
// (C-CAL-ENTITY-TIES-DATA-MODEL). Owns enum validation; the link/unlink and
// both-direction queries delegate to the repo. Cascade-on-delete is
// DB-enforced (ON DELETE CASCADE), so there is no cascade orchestration here.
package calendar

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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
