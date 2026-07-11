package aiexport

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// stubEntityLister satisfies EntityLister with canned return values.
// Mirrors the calendar_api_handler_test pattern (interface-fill stubs
// rather than mock-library noise).
type stubEntityLister struct {
	ents  []entities.Entity
	types []entities.EntityType
}

func (s *stubEntityLister) List(_ context.Context, _ string, _ int, _ int, _ string, _ entities.ListOptions) ([]entities.Entity, int, error) {
	return s.ents, len(s.ents), nil
}
func (s *stubEntityLister) GetEntityTypes(_ context.Context, _ string) ([]entities.EntityType, error) {
	return s.types, nil
}

type stubNoteLister struct{ list []notes.Note }

func (s *stubNoteLister) ListByUserAndCampaign(_ context.Context, _, _ string) ([]notes.Note, error) {
	return s.list, nil
}

type stubCalendarLister struct {
	cal    *calendar.Calendar
	events []calendar.Event
}

func (s *stubCalendarLister) GetCalendar(_ context.Context, _ string) (*calendar.Calendar, error) {
	return s.cal, nil
}
func (s *stubCalendarLister) ListAllEventsForCalendar(_ context.Context, _ string) ([]calendar.Event, error) {
	return s.events, nil
}

type stubSessionLister struct {
	list    []sessions.Session
	atts    map[string][]sessions.Attendee
	linked  map[string][]sessions.SessionEntity
}

func (s *stubSessionLister) ListSessions(_ context.Context, _ string) ([]sessions.Session, error) {
	return s.list, nil
}
func (s *stubSessionLister) ListAttendees(_ context.Context, sessID string) ([]sessions.Attendee, error) {
	return s.atts[sessID], nil
}
func (s *stubSessionLister) ListSessionEntities(_ context.Context, sessID string) ([]sessions.SessionEntity, error) {
	return s.linked[sessID], nil
}

type stubTimelineLister struct {
	tls    []timeline.Timeline
	events map[string][]timeline.EventLink
}

func (s *stubTimelineLister) ListTimelines(_ context.Context, _ string, _ int, _ string) ([]timeline.Timeline, error) {
	return s.tls, nil
}
func (s *stubTimelineLister) ListTimelineEvents(_ context.Context, tlID string, _ int, _ string) ([]timeline.EventLink, error) {
	return s.events[tlID], nil
}

type stubRelationLister struct {
	byEntity map[string][]relations.Relation
}

func (s *stubRelationLister) ListByEntity(_ context.Context, _, entityID string) ([]relations.Relation, error) {
	return s.byEntity[entityID], nil
}

type stubTagLister struct {
	byEntity map[string][]tags.Tag
}

func (s *stubTagLister) GetEntityTagsBatch(_ context.Context, _ []string, _ bool) (map[string][]tags.Tag, error) {
	return s.byEntity, nil
}

// TestService_GenerateAllCategories drives the whole orchestrator
// through every category with a minimal dummy campaign. Asserts the
// document contains every section header + the token estimate is
// substituted (not the placeholder string).
func TestService_GenerateAllCategories(t *testing.T) {
	svc := NewService(
		&stubEntityLister{
			ents: []entities.Entity{
				{ID: "e1", Name: "Lyra Vance", EntityTypeID: 1,
					TypeName: "Character", EntryHTML: sp("<p>PC body.</p>")},
			},
			types: []entities.EntityType{
				{ID: 1, Name: "Character", NamePlural: "Characters"},
			},
		},
		&stubNoteLister{list: []notes.Note{
			{ID: "n1", Title: "Plot Threads", EntryHTML: sp("<p>note body</p>")},
		}},
		&stubCalendarLister{
			cal: &calendar.Calendar{ID: "cal1", Name: "Ashfall Reckoning",
				Months: []calendar.Month{{Name: "Highsummer", Days: 30, SortOrder: 1}},
			},
			events: []calendar.Event{
				{ID: "ev1", Name: "Tide", Year: 1247, Month: 1, Day: 4,
					DescriptionHTML: sp("<p>desc</p>")},
			},
		},
		&stubSessionLister{
			list: []sessions.Session{{ID: "s1", Name: "Session 1",
				Status: sessions.StatusPlanned, Summary: sp("Cross into the maze.")}},
		},
		&stubTimelineLister{
			tls: []timeline.Timeline{{ID: "tl1", Name: "Rise & Fall"}},
			events: map[string][]timeline.EventLink{
				"tl1": {{ID: 1, TimelineID: "tl1", EventName: "Crowning",
					EventYear: 1102, EventMonth: 1, EventDay: 1}},
			},
		},
		&stubRelationLister{byEntity: nil},
		&stubTagLister{byEntity: nil},
	)

	got, err := svc.Generate(context.Background(), "Ashfall", "owner-1", "camp-1", Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, want := range []string{
		"# Ashfall — AI Export",
		"# Entities",
		"# Notes",
		"# Calendar Events",
		"# Sessions",
		"# Timelines",
		"Privacy mode: safe",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Generate output missing %q. First 500 chars:\n%s", want, got[:min(500, len(got))])
		}
	}
	if strings.Contains(got, "__TOKEN_COUNT__") {
		t.Errorf("token placeholder not substituted; output starts:\n%s", got[:min(500, len(got))])
	}
}

// TestService_Generate_CategorySubset confirms a non-default
// category selection emits only the chosen sections.
func TestService_Generate_CategorySubset(t *testing.T) {
	svc := NewService(
		&stubEntityLister{
			ents: []entities.Entity{
				{ID: "e1", Name: "Solo", EntityTypeID: 1, TypeName: "Thing",
					EntryHTML: sp("<p>x</p>")},
			},
			types: []entities.EntityType{{ID: 1, Name: "Thing", NamePlural: "Things"}},
		},
		&stubNoteLister{},
		&stubCalendarLister{},
		&stubSessionLister{},
		&stubTimelineLister{},
		&stubRelationLister{},
		&stubTagLister{},
	)
	got, err := svc.Generate(context.Background(), "Ashfall", "owner-1", "camp-1",
		Options{Categories: []Category{CategoryEntities}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(got, "# Entities") {
		t.Errorf("expected # Entities heading:\n%s", got)
	}
	for _, unwanted := range []string{"# Notes", "# Calendar Events", "# Sessions", "# Timelines"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("category subset leaked section %q:\n%s", unwanted, got)
		}
	}
}

// TestService_Generate_MissingCampaignID guards the most obvious
// caller mistake.
func TestService_Generate_MissingCampaignID(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.Generate(context.Background(), "x", "owner-1", "", Options{})
	if err == nil {
		t.Fatal("expected error on empty campaignID")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
