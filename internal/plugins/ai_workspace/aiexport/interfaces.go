package aiexport

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// EntityLister is the narrow contract aiexport needs from
// entities.EntityService. Kept narrow so tests can stub without
// implementing the 40-method EntityService surface.
type EntityLister interface {
	List(ctx context.Context, campaignID string, typeID int, role int, userID string, opts entities.ListOptions) ([]entities.Entity, int, error)
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
}

// NoteLister mirrors notes.NoteService for the listing path. The owner-side
// filter (own + shared + explicit share-with-owner) is already enforced
// inside notes.Service.ListByUserAndCampaign, so the aiexport renderer does
// not reimplement it.
type NoteLister interface {
	ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]notes.Note, error)
}

// CalendarLister loads the calendar configuration (Months / Eras needed for
// human-readable date labels) and every event on it.
// ListAllEventsForCalendar intentionally bypasses role-filtering — the
// aiexport renderer applies the privacy filter itself (Safe mode's own
// drops, see renderer.go's RenderCalendarEvents) so it can choose between
// Owner-view / Safe / Everything modes rather than trusting a per-viewer
// read to already agree with any one of those definitions.
//
// ListAllEventsForCalendar's campaignID parameter is a deliberate addition
// over the pre-V5 shape: the calendar plugin's own bulk read
// (CalendarService.ListAllEventsForCalendar) is campaign-scoped for IDOR
// safety, the same reason GetCalendar itself takes campaignID.
type CalendarLister interface {
	GetCalendar(ctx context.Context, campaignID string) (*calendar.Calendar, error)
	ListAllEventsForCalendar(ctx context.Context, campaignID, calendarID string) ([]calendar.Event, error)
}

// SessionLister loads sessions + their nested joins. Attendees +
// SessionEntity slices are fetched per-session; the N+1 pattern is accepted
// because session counts are modest (10-50 per campaign).
type SessionLister interface {
	ListSessions(ctx context.Context, campaignID string) ([]sessions.Session, error)
	ListAttendees(ctx context.Context, sessionID string) ([]sessions.Attendee, error)
	ListSessionEntities(ctx context.Context, sessionID string) ([]sessions.SessionEntity, error)
}

// TimelineLister returns timelines + their event slices. ListTimelineEvents
// returns timeline.EventLink — the join+overlay row that handles both
// calendar-linked events and standalone timeline events uniformly.
type TimelineLister interface {
	// Both take a permissions.Viewer (see ADR-049: "no user" and "trusted
	// system caller" are distinct, not both the empty user id). The export
	// builds a RequestViewer from the operator's real id — never a system
	// caller.
	ListTimelines(ctx context.Context, campaignID string, v permissions.Viewer) ([]timeline.Timeline, error)
	// ListTimelineEvents also takes campaignID, so a non-owner's result can
	// be narrowed by EntityVisibilityGate (a linked entity's visibility is a
	// separate check from the event's own).
	ListTimelineEvents(ctx context.Context, timelineID, campaignID string, v permissions.Viewer) ([]timeline.EventLink, error)
}

// RelationLister exposes a single entity's relations. Rendering is
// bidirectional (both endpoints get the relation listed), so it queries
// per-entity rather than per-pair; the duplication is accepted.
type RelationLister interface {
	ListByEntity(ctx context.Context, campaignID, entityID string) ([]relations.Relation, error)
}

// TagLister batch-fetches tags for the entity list, since the entities
// repository does not populate entity.Tags. Called once per campaign-export
// with the full entity ID list.
type TagLister interface {
	GetEntityTagsBatch(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]tags.Tag, error)
}
