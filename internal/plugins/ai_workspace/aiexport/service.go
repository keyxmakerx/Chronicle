package aiexport

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// Service is the top-level entry point. PR-B's campaigns handler
// constructs one of these per request and calls Generate. The
// dependencies are narrow Service interfaces (see interfaces.go)
// so the orchestrator is testable without a database.
type Service struct {
	Entities  EntityLister
	Notes     NoteLister
	Calendar  CalendarLister
	Sessions  SessionLister
	Timelines TimelineLister
	Relations RelationLister
	Tags      TagLister
}

// NewService constructs a Service. Every dependency is required for
// v1; a nil dependency for a Category the owner has enabled produces
// a clear error at Generate time rather than a panic.
func NewService(
	ents EntityLister,
	notes NoteLister,
	cal CalendarLister,
	sess SessionLister,
	tl TimelineLister,
	rel RelationLister,
	tg TagLister,
) *Service {
	return &Service{
		Entities:  ents,
		Notes:     notes,
		Calendar:  cal,
		Sessions:  sess,
		Timelines: tl,
		Relations: rel,
		Tags:      tg,
	}
}

// Generate runs every enabled Category renderer and returns the
// assembled markdown. The token-estimate placeholder in the header
// is substituted with the final count once the body is rendered.
//
// Errors from one category abort the export — better to surface a
// renderer failure than ship a half-document the owner pastes
// into an AI tool not realising chunks are missing.
//
// Caller responsibility: ownerID + campaignID are the operator's
// authenticated identity; this method trusts them. The owner-gate
// check lives in the campaigns handler (PR-B) before this is called.
func (s *Service) Generate(ctx context.Context, campaignName, ownerID, campaignID string, opts Options) (string, error) {
	if campaignID == "" {
		return "", fmt.Errorf("aiexport: campaignID required")
	}

	var body strings.Builder

	for _, c := range opts.EnabledCategories() {
		section, err := s.renderCategory(ctx, c, ownerID, campaignID, opts)
		if err != nil {
			return "", fmt.Errorf("aiexport: %s: %w", c, err)
		}
		if section != "" {
			body.WriteString(section)
		}
	}

	header := RenderHeader(campaignName, time.Now(), opts)
	full := header + body.String()
	full = substituteTokenCount(full, estimateTokens(full))
	return full, nil
}

// renderCategory dispatches one Category to its renderer. Each branch
// performs the lister calls + filters + delegates to the per-category
// Render* helper in renderer.go.
func (s *Service) renderCategory(
	ctx context.Context,
	c Category,
	ownerID, campaignID string,
	opts Options,
) (string, error) {
	role := roleFor(opts.Privacy)

	switch c {
	case CategoryEntities:
		if s.Entities == nil {
			return "", fmt.Errorf("entities lister not wired")
		}
		ents, _, err := s.Entities.List(ctx, campaignID, 0, role, ownerID, entities.ListOptions{
			Page: 1, PerPage: 10000, // single-page export; campaigns rarely exceed
		})
		if err != nil {
			return "", err
		}
		if len(ents) == 0 {
			return "", nil
		}
		types, err := s.Entities.GetEntityTypes(ctx, campaignID)
		if err != nil {
			return "", err
		}

		// Per operator decision (2026-05-26 AskUserQuestion 1): maps OUT
		// of v1. Per decision 2 (relations both endpoints): one query per
		// entity, accept the N+1 because entity count is bounded (100-300
		// per typical campaign per scoping report §1).
		relByEntity := map[string][]relations.Relation{}
		if s.Relations != nil {
			for _, e := range ents {
				rels, err := s.Relations.ListByEntity(ctx, e.ID)
				if err != nil {
					return "", fmt.Errorf("entity %q relations: %w", e.Name, err)
				}
				if len(rels) > 0 {
					relByEntity[e.ID] = rels
				}
			}
		}

		tagsByEntity := map[string][]tags.Tag{}
		if s.Tags != nil {
			ids := make([]string, 0, len(ents))
			for _, e := range ents {
				ids = append(ids, e.ID)
			}
			tagsByEntity, err = s.Tags.GetEntityTagsBatch(ctx, ids, opts.Privacy != PrivacyModeSafe)
			if err != nil {
				return "", err
			}
		}

		return RenderEntities(ctx, ents, types, tagsByEntity, relByEntity, opts)

	case CategoryNotes:
		if s.Notes == nil {
			return "", fmt.Errorf("notes lister not wired")
		}
		list, err := s.Notes.ListByUserAndCampaign(ctx, ownerID, campaignID)
		if err != nil {
			return "", err
		}
		return RenderNotes(ctx, list, opts)

	case CategoryCalendarEvents:
		if s.Calendar == nil {
			return "", fmt.Errorf("calendar lister not wired")
		}
		cal, err := s.Calendar.GetCalendar(ctx, campaignID)
		if err != nil || cal == nil {
			// Calendar addon disabled or no calendar yet: skip gracefully.
			return "", nil //nolint:nilerr // intentional skip on missing calendar
		}
		events, err := s.Calendar.ListAllEventsForCalendar(ctx, cal.ID)
		if err != nil {
			return "", err
		}
		return RenderCalendarEvents(ctx, cal, events, opts)

	case CategorySessions:
		if s.Sessions == nil {
			return "", fmt.Errorf("sessions lister not wired")
		}
		list, err := s.Sessions.ListSessions(ctx, campaignID)
		if err != nil {
			return "", err
		}
		attendees := map[string][]sessions.Attendee{}
		linked := map[string][]sessions.SessionEntity{}
		for _, s2 := range list {
			a, err := s.Sessions.ListAttendees(ctx, s2.ID)
			if err != nil {
				return "", fmt.Errorf("session %q attendees: %w", s2.Name, err)
			}
			attendees[s2.ID] = a
			ents, err := s.Sessions.ListSessionEntities(ctx, s2.ID)
			if err != nil {
				return "", fmt.Errorf("session %q linked entities: %w", s2.Name, err)
			}
			linked[s2.ID] = ents
		}
		return RenderSessions(ctx, list, attendees, linked, opts)

	case CategoryTimelines:
		if s.Timelines == nil {
			return "", fmt.Errorf("timelines lister not wired")
		}
		tls, err := s.Timelines.ListTimelines(ctx, campaignID, role, ownerID)
		if err != nil {
			return "", err
		}
		eventsByTimeline := map[string][]timeline.EventLink{}
		for _, tl := range tls {
			evs, err := s.Timelines.ListTimelineEvents(ctx, tl.ID, role, ownerID)
			if err != nil {
				return "", fmt.Errorf("timeline %q events: %w", tl.Name, err)
			}
			eventsByTimeline[tl.ID] = evs
		}
		return RenderTimelines(ctx, tls, eventsByTimeline, opts)
	}

	return "", fmt.Errorf("unknown category %q", c)
}
