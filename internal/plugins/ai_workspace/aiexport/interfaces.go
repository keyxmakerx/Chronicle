package aiexport

import (
	"context"

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

// NoteLister mirrors notes.NoteService for the listing path. The
// owner-side filter is already enforced inside
// notes.Service.ListByUserAndCampaign — own + shared + explicit
// share-with-owner — so the aiexport renderer doesn't reimplement
// the filter. PrivacyModePermitted / Everything use this same list;
// Safe additionally drops rows where IsShared is false AND the owner
// is not the author AND SharedWith doesn't include the owner — but
// the service's filter is already the Safe semantics for non-owner
// callers. Owner sees their own + shared; that's the v1 default.
type NoteLister interface {
	ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]notes.Note, error)
}

// CalendarLister loads the calendar configuration (Months / Eras
// needed for human-readable date labels) and every event on it.
// ListAllEventsForCalendar intentionally bypasses role-filtering —
// the aiexport renderer applies the privacy filter itself so it
// can choose between Owner-view / Safe / Everything modes.
type CalendarLister interface {
	GetCalendar(ctx context.Context, campaignID string) (*calendar.Calendar, error)
	ListAllEventsForCalendar(ctx context.Context, calendarID string) ([]calendar.Event, error)
}

// SessionLister loads sessions + their nested joins. Attendees +
// SessionEntity slices are fetched per-session; v1 accepts the N+1
// pattern because session counts are modest (10-50 per campaign per
// the scoping report's volume estimates).
type SessionLister interface {
	ListSessions(ctx context.Context, campaignID string) ([]sessions.Session, error)
	ListAttendees(ctx context.Context, sessionID string) ([]sessions.Attendee, error)
	ListSessionEntities(ctx context.Context, sessionID string) ([]sessions.SessionEntity, error)
}

// TimelineLister returns timelines + their event slices. ListTimelineEvents
// returns timeline.EventLink — the join+overlay row that handles both
// calendar-linked events and standalone timeline events uniformly.
type TimelineLister interface {
	ListTimelines(ctx context.Context, campaignID string, role int, userID string) ([]timeline.Timeline, error)
	ListTimelineEvents(ctx context.Context, timelineID string, role int, userID string) ([]timeline.EventLink, error)
}

// RelationLister exposes a single entity's relations. Bidirectional
// rendering (per operator decision 2026-05-26 — both endpoints get
// the relation listed) means we query per-entity rather than per-pair;
// the duplication is documented and accepted.
type RelationLister interface {
	ListByEntity(ctx context.Context, campaignID, entityID string) ([]relations.Relation, error)
}

// TagLister batch-fetches tags for the entity list. Entity rows
// don't carry Tags via the repository (`entity.Tags` is "populated
// at the handler level via batch fetch" per
// internal/plugins/entities/model.go:286). aiexport's orchestrator
// calls this once per campaign-export with the full entity ID list.
type TagLister interface {
	GetEntityTagsBatch(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]tags.Tag, error)
}
