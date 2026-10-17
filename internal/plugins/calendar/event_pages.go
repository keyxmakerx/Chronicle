// Package calendar - event_pages.go links pages to an event through
// entity_event_links, for the event page's "Linked pages" list. A link made
// here always carries the "involved" role; nothing yet asks for a finer one.
package calendar

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// EventPages: see the CalendarService doc comment.
func (s *calendarService) EventPages(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) ([]EntityTieRef, error) {
	// The same gates as reading the event itself, so a tie never reveals an
	// event the viewer could not open.
	if _, err := s.GetEventForViewer(ctx, eventID, calendarID, campaignID, v); err != nil {
		return nil, err
	}
	refs, err := s.eventRepo.EntitiesForEvent(ctx, eventID, v.Role(), v.UserID())
	if err != nil {
		return nil, fmt.Errorf("list event pages: %w", err)
	}
	if refs == nil {
		refs = []EntityTieRef{}
	}
	return refs, nil
}

// LinkEventPage: see the CalendarService doc comment.
func (s *calendarService) LinkEventPage(ctx context.Context, eventID, calendarID, campaignID, entityID string, v permissions.Viewer) error {
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return apperror.NewValidation("entity_id is required")
	}
	if err := s.eventForTieEdit(ctx, eventID, calendarID, campaignID, v); err != nil {
		return err
	}
	// The repository refuses a page from another campaign.
	return s.eventRepo.LinkEntityEvent(ctx, entityID, eventID, string(RoleInvolved))
}

// UnlinkEventPage: see the CalendarService doc comment.
func (s *calendarService) UnlinkEventPage(ctx context.Context, eventID, calendarID, campaignID, entityID string, v permissions.Viewer) error {
	if err := s.eventForTieEdit(ctx, eventID, calendarID, campaignID, v); err != nil {
		return err
	}
	if err := s.eventRepo.UnlinkEntityEvent(ctx, entityID, eventID); err != nil {
		return fmt.Errorf("unlink event page: %w", err)
	}
	return nil
}

// eventForTieEdit is UpdateEvent's gate: the calendar and the event must be
// visible to v, so nobody edits ties on an event they cannot see.
func (s *calendarService) eventForTieEdit(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) error {
	if _, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v); err != nil {
		return err
	}
	_, err := s.eventInCalendarForViewer(ctx, eventID, calendarID, v)
	return err
}
