package timeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// AITimelines is the timeline plugin's part of the AI engine (aiplug): the
// `timelines` lookup, each timeline with its events. It reads through the
// service's viewer-aware list methods, so a Director-only timeline or event,
// and the link to a page the viewer may not open, stay out for exactly the
// viewers the Timeline screens hide them from: Safe privacy and a player's
// key read as a player.
type AITimelines struct {
	svc AITimelineReader
	// off says why the lookup can't answer now (the add-on is switched off or
	// the plugin is degraded); "" means it can.
	off func(ctx context.Context, campaignID string) string
}

// AITimelineReader is the part of TimelineService the lookup reads.
type AITimelineReader interface {
	ListTimelines(ctx context.Context, campaignID string, v permissions.Viewer) ([]Timeline, error)
	ListTimelineEvents(ctx context.Context, timelineID, campaignID string, v permissions.Viewer) ([]EventLink, error)
}

// NewAITimelines builds the lookup over the timeline service. off may be nil.
func NewAITimelines(svc AITimelineReader, off func(ctx context.Context, campaignID string) string) *AITimelines {
	return &AITimelines{svc: svc, off: off}
}

var _ aiplug.Lookup = (*AITimelines)(nil)

// maxAITimelines bounds the per-timeline event reads one answer makes.
const maxAITimelines = 40

// LookupInfo describes `what: timelines` and its tool. It is not owner-only:
// the service hides what the viewer may not see, so a player's key gets the
// player's timelines.
func (*AITimelines) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "timelines", Aliases: []string{"timeline"}, Keys: "none",
		Brief: "the campaign's timelines with their events: date, title and the page each links to, as the asker may see them (Director-only timelines and events only for the Director team)",
		Tool: aiplug.Tool{
			Name: "timelines", Title: "Timelines", Did: "Looked at the timelines",
			Desc:       "The campaign's timelines, each with its events in date order: date (year-month-day), title and the page it links to. Director-only timelines and events are marked.",
			PlayerDesc: "The timelines you can see, each with its events in date order: date (year-month-day), title and the page it links to.",
		},
	}
}

// Answer lists the timelines the Actor may see. Director-only rows exist in
// the answer only for the Director team, and are marked there.
func (t *AITimelines) Answer(ctx context.Context, campaignID string, a aiplug.Actor, _ aiplug.Record) (string, string, error) {
	chip := "Timelines"
	if t.off != nil {
		if why := t.off(ctx, campaignID); why != "" {
			return chip, "", aiplug.BadRequestf("%s", why)
		}
	}
	v := a.Viewer()
	list, err := t.svc.ListTimelines(ctx, campaignID, v)
	if err != nil {
		return chip, "", err
	}
	director := a.CanAuthorDmOnly()
	var b strings.Builder
	fmt.Fprintf(&b, "## Timelines (%d)\n\nDates are year-month-day on the timeline's calendar.\n", len(list))
	var lines []string
	events := 0
	for i, tl := range list {
		if i >= maxAITimelines {
			lines = append(lines, fmt.Sprintf("…and %d more timelines; ask again after the Director narrows them.", len(list)-i))
			break
		}
		head := "\n### " + aiplug.OneLine(tl.Name, 120)
		if director && tl.IsDMOnly() {
			head += " (Director only)"
		}
		lines = append(lines, head, "")
		evs, err := t.svc.ListTimelineEvents(ctx, tl.ID, campaignID, v)
		if err != nil {
			return chip, "", err
		}
		if len(evs) == 0 {
			lines = append(lines, "No events.")
		}
		for _, e := range evs {
			events++
			lines = append(lines, eventLine(e, director))
		}
	}
	if len(list) == 0 {
		b.WriteString("None yet.\n")
	}
	aiplug.WriteLines(&b, lines)
	return fmt.Sprintf("Timelines · %d", len(list)), b.String(), nil
}

// eventLine is one event: its date, title, page and, for a Director, a
// mark when the event is hidden from players.
func eventLine(e EventLink, director bool) string {
	line := fmt.Sprintf("- %d-%02d-%02d: %s", e.EventYear, e.EventMonth, e.EventDay, aiplug.OneLine(e.EffectiveLabel(), 120))
	if e.EventEntityName != "" {
		line += " (page: " + aiplug.OneLine(e.EventEntityName, 80) + ")"
	}
	if director && e.EffectiveVisibility() == "dm_only" {
		line += " [Director only]"
	}
	return line
}
