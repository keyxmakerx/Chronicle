package aiexport

import (
	"context"
	"fmt"
	"log/slog"
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
// Resilience: a single category's failure never aborts the export. The
// bad section is logged and replaced with a short "could not be exported"
// note, then the remaining categories render as normal. This is what makes
// "export everything" degrade to a partial document instead of the generic
// error modal the owner previously hit when one private entity carried
// HTML the converter rejected. (Per-field conversion failures are absorbed
// even lower down, in bodyOrSkip.)
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
			// One category's lister/DB failure must not blank the whole
			// export. Log for triage, drop a visible note so the owner
			// knows the section was skipped, and continue. (The calendar
			// branch already skips on error; this extends the same
			// tolerance to every category.)
			slog.Warn("aiexport: skipping category after render error",
				slog.String("category", string(c)),
				slog.String("campaign_id", campaignID),
				slog.Any("error", err))
			body.WriteString(categorySkipNote(c))
			continue
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
		// Page through the whole campaign. A single PerPage:10000 request
		// was silently clamped to 24 by entityService.List, so "export
		// everything" only ever emitted the first 24 entities.
		ents, err := s.listAllEntities(ctx, campaignID, role, ownerID)
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
				rels, err := s.Relations.ListByEntity(ctx, e.CampaignID, e.ID)
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

// exportPageSize is the largest page entityService.List honors — it clamps
// PerPage to 100 (and silently defaults anything larger to 24). The export
// must page at this size, never with a fat single-page request.
const exportPageSize = 100

// exportMaxPages bounds the paging loop so a pathological campaign — or a
// test stub that always returns a full page — can never spin forever.
// 500 × 100 = 50,000 entities, far beyond any real campaign.
const exportMaxPages = 500

// listAllEntities pages through every entity visible at the given role and
// returns them all. It exists because a single request for PerPage:10000 was
// clamped to 24 by entityService.List, silently truncating "export
// everything" to the first 24 entities. Rows are de-duplicated by ID so a
// same-name reorder at a page boundary (the default ORDER BY is name-only)
// can't double-count, and paging stops as soon as a page is short or adds
// nothing new.
func (s *Service) listAllEntities(ctx context.Context, campaignID string, role int, ownerID string) ([]entities.Entity, error) {
	seen := make(map[string]struct{})
	var all []entities.Entity
	for page := 1; page <= exportMaxPages; page++ {
		ents, _, err := s.Entities.List(ctx, campaignID, 0, role, ownerID, entities.ListOptions{
			Page: page, PerPage: exportPageSize,
		})
		if err != nil {
			return nil, err
		}
		added := 0
		for i := range ents {
			if _, dup := seen[ents[i].ID]; dup {
				continue
			}
			seen[ents[i].ID] = struct{}{}
			all = append(all, ents[i])
			added++
		}
		// Short page (the last one) or no forward progress → done.
		if len(ents) < exportPageSize || added == 0 {
			break
		}
	}
	return all, nil
}

// categorySkipNote is the visible placeholder Generate emits when a whole
// category fails to render (a lister/DB error). Keeps the document flowing
// so a partial export beats a blank error modal.
func categorySkipNote(c Category) string {
	return fmt.Sprintf("> _[The %q section could not be exported and was skipped.]_\n\n", string(c))
}
