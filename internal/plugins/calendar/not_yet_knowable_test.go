package calendar

// not_yet_knowable_test.go pins that every event read hides what a player
// cannot know yet: a future event announced only on its day, and an event
// inside an era still hidden from players. Each read is checked against the
// same calendar, so a read that applies one filter and forgets the other
// fails here, and a new read must be added to the table (or the source
// guard below names it).

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// knowableFixture is a calendar on day 10 of year 1000 with three events: one
// past and common knowledge, one in the future announced only on its day, and
// one announced ahead but inside an era that is secret until day 22.
func knowableFixture() (*calendarService, map[string]bool) {
	cal := Calendar{
		ID: "cal-1", CampaignID: testCampaignA, Mode: ModeFantasy, Visibility: "everyone",
		CurrentYear: 1000, CurrentMonth: 1, CurrentDay: 10,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
	}
	ahead := AnnouncedAhead
	events := []Event{
		{ID: "evt-known", CalendarID: "cal-1", Name: "Market day", Year: 1000, Month: 1, Day: 5, Visibility: "everyone"},
		{ID: "evt-unannounced", CalendarID: "cal-1", Name: "Market ambush", Year: 1000, Month: 1, Day: 20, Visibility: "everyone", KindID: intPtr(1)},
		{ID: "evt-secret-era", CalendarID: "cal-1", Name: "Market of the long night", Year: 1000, Month: 1, Day: 25, Visibility: "everyone", Announced: &ahead},
	}
	copyEvents := func() []Event { return append([]Event(nil), events...) }
	byID := map[string]Event{}
	for _, e := range events {
		byID[e.ID] = e
	}

	calRepo := &fakeCalendarRepo{
		getByIDFn:        func(context.Context, string) (*Calendar, error) { c := cal; return &c, nil },
		getDefaultFn:     func(context.Context, string) (*Calendar, error) { c := cal; return &c, nil },
		listByCampaignFn: func(context.Context, string) ([]Calendar, error) { return []Calendar{cal}, nil },
		getMonthsFn: func(context.Context, string) ([]Month, error) {
			return []Month{{Name: "First", Days: 30, SortOrder: 0}}, nil
		},
		getErasFn: func(context.Context, string) ([]Era, error) {
			return []Era{{ID: 6, CalendarID: "cal-1", Name: "Year one", StartYear: 1000, StartMonth: 1, StartDay: 1}, {ID: 7, CalendarID: "cal-1", Name: "The long night", StartYear: 1000, StartMonth: 1, StartDay: 22, HiddenUntilBegins: true}}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			if e, ok := byID[id]; ok {
				return &e, nil
			}
			return nil, nil
		},
		getEventsByIDsFn: func(_ context.Context, _ string, ids []string) ([]Event, error) {
			var out []Event
			for _, id := range ids {
				if e, ok := byID[id]; ok {
					out = append(out, e)
				}
			}
			return out, nil
		},
		listForMonthFn: func(context.Context, string, int, int, int) ([]Event, error) { return copyEvents(), nil },
		listUpcomingFn: func(context.Context, string, int, int, int, int, int) ([]Event, error) {
			return copyEvents()[1:], nil
		},
		listForPageFn: func(context.Context, string, string, int) ([]Event, error) { return copyEvents(), nil },
		searchFn:      func(context.Context, string, string, int) ([]Event, error) { return copyEvents(), nil },
		listAllFn:     func(context.Context, string) ([]Event, error) { return copyEvents(), nil },
	}
	kindRepo := &fakeEventKindRepo{
		listFn: func(context.Context, string) ([]EventKind, error) {
			return []EventKind{{ID: 1, Slug: "ambush", Name: "Ambush", DefaultAnnounced: AnnouncedOnDay}}, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, kindRepo, nil).(*calendarService)
	rolls := newFakeRollRepo()
	for id := range byID {
		rolls.cfg[id] = &RollConfig{}
	}
	svc.SetEventRollRepository(rolls)

	all := map[string]bool{}
	for id := range byID {
		all[id] = true
	}
	return svc, all
}

// eventRead runs one read for v and returns the event ids it let through.
// reach, when set, is the ids the read can return at all (an upcoming list
// never holds a past event).
type eventRead struct {
	name  string
	reach []string
	run   func(ctx context.Context, s *calendarService, ids []string, v permissions.Viewer) ([]string, error)
}

// oneByOne collects the ids a single-event read answers for.
func oneByOne(get func(ctx context.Context, s *calendarService, id string, v permissions.Viewer) error) func(context.Context, *calendarService, []string, permissions.Viewer) ([]string, error) {
	return func(ctx context.Context, s *calendarService, ids []string, v permissions.Viewer) ([]string, error) {
		var out []string
		for _, id := range ids {
			err := get(ctx, s, id, v)
			if err == nil {
				out = append(out, id)
				continue
			}
			if !isCalendarNotFound(err) {
				return nil, err
			}
		}
		return out, nil
	}
}

func idsOf(events []Event) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.ID)
	}
	return out
}

var everyEventRead = []eventRead{
	{"ListEventsForMonth", nil, func(ctx context.Context, s *calendarService, _ []string, v permissions.Viewer) ([]string, error) {
		evs, err := s.ListEventsForMonth(ctx, "cal-1", testCampaignA, 1000, 1, v)
		return idsOf(evs), err
	}},
	{"GetEventForViewer", nil, oneByOne(func(ctx context.Context, s *calendarService, id string, v permissions.Viewer) error {
		_, err := s.GetEventForViewer(ctx, id, "cal-1", testCampaignA, v)
		return err
	})},
	{"ListEventsByIDsForViewer", nil, func(ctx context.Context, s *calendarService, ids []string, v permissions.Viewer) ([]string, error) {
		evs, err := s.ListEventsByIDsForViewer(ctx, "cal-1", testCampaignA, ids, v)
		return idsOf(evs), err
	}},
	{"ListEventIndexForViewer", nil, func(ctx context.Context, s *calendarService, _ []string, v permissions.Viewer) ([]string, error) {
		entries, _, err := s.ListEventIndexForViewer(ctx, "cal-1", testCampaignA, "", v)
		var out []string
		for _, e := range entries {
			out = append(out, e.ID)
		}
		return out, err
	}},
	// "Year one" ends where the secret era begins, so it holds two events.
	{"ListEraEventsForViewer", []string{"evt-known", "evt-unannounced"}, func(ctx context.Context, s *calendarService, _ []string, v permissions.Viewer) ([]string, error) {
		evs, _, err := s.ListEraEventsForViewer(ctx, 6, "cal-1", testCampaignA, v)
		return idsOf(evs), err
	}},
	{"ListUpcomingEvents", []string{"evt-unannounced", "evt-secret-era"}, func(ctx context.Context, s *calendarService, _ []string, v permissions.Viewer) ([]string, error) {
		evs, err := s.ListUpcomingEvents(ctx, "cal-1", testCampaignA, 10, v)
		return idsOf(evs), err
	}},
	{"SearchCalendarEvents", nil, func(ctx context.Context, s *calendarService, _ []string, v permissions.Viewer) ([]string, error) {
		res, err := s.SearchCalendarEvents(ctx, testCampaignA, "market", v.Role())
		var out []string
		for _, r := range res {
			out = append(out, r["id"])
		}
		return out, err
	}},
	{"ListEventsForCalendar", nil, func(ctx context.Context, s *calendarService, _ []string, v permissions.Viewer) ([]string, error) {
		evs, err := s.ListEventsForCalendar(ctx, testCampaignA, "cal-1", v.Role())
		return idsOf(evs), err
	}},
	{"PageTimeline", nil, func(ctx context.Context, s *calendarService, _ []string, v permissions.Viewer) ([]string, error) {
		tl, err := s.PageTimeline(ctx, testCampaignA, "page-1", false, v)
		if err != nil || tl == nil {
			return nil, err
		}
		var out []string
		for _, e := range tl.Events {
			out = append(out, e.ID)
		}
		return out, nil
	}},
	{"GetEventRoll", nil, oneByOne(func(ctx context.Context, s *calendarService, id string, v permissions.Viewer) error {
		_, err := s.GetEventRoll(ctx, id, "cal-1", testCampaignA, 1000, v)
		return err
	})},
}

func TestEventReads_HideNotYetKnowableEvents(t *testing.T) {
	ids := []string{"evt-known", "evt-unannounced", "evt-secret-era"}
	for _, r := range everyEventRead {
		t.Run(r.name, func(t *testing.T) {
			for _, tc := range []struct {
				who  string
				v    permissions.Viewer
				want []string
			}{
				{"player", permissions.RequestViewer(int(permissions.RolePlayer), "user-1"), []string{"evt-known"}},
				{"owner", permissions.RequestViewer(int(permissions.RoleOwner), "user-0"), ids},
			} {
				svc, _ := knowableFixture()
				got, err := r.run(context.Background(), svc, ids, tc.v)
				if err != nil {
					t.Fatalf("%s: %v", tc.who, err)
				}
				sort.Strings(got)
				var want []string
				for _, id := range tc.want {
					if r.reach == nil || contains(r.reach, id) {
						want = append(want, id)
					}
				}
				sort.Strings(want)
				if strings.Join(got, ",") != strings.Join(want, ",") {
					t.Errorf("%s: got %v, want %v", tc.who, got, want)
				}
			}
		})
	}
}

// eventReadExempt are the exported methods that read events from storage
// without the player filters, each for a reason that keeps players out.
var eventReadExempt = map[string]string{
	"ListAllEventsForCalendar": "system callers only (refuses any other viewer)",
	"ListOccurrenceOverrides":  "system callers only (refuses any other viewer)",
	"EventRollsForBackup":      "owner and granted co-DM only (requireBackupViewer)",
}

// eventStoreReads are the EventRepository methods that return events.
var eventStoreReads = map[string]bool{
	"GetEvent": true, "GetEventsByIDs": true, "ListEventsForMonth": true,
	"ListEventsForYear": true, "ListEventsForDateRange": true, "ListEventsForEntity": true,
	"ListEventsForPage": true, "ListUpcomingEvents": true, "SearchEvents": true,
	"ListAllEvents": true, "ListRuleEvents": true,
}

// TestEventReads_EveryReadIsInTheTable keeps the table above complete: an
// exported service method that reads events from storage for a viewer and
// is neither in the table nor exempt fails here, so a new read cannot ship
// without being checked. Unexported helpers are followed one level, which
// is how the reads here are built.
func TestEventReads_EveryReadIsInTheTable(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	methods := map[string]*ast.FuncDecl{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			if star, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
				if id, ok := star.X.(*ast.Ident); ok && id.Name == "calendarService" {
					methods[fn.Name.Name] = fn
				}
			}
		}
	}

	// calls lists the s.eventRepo.X and s.y method names fn calls.
	calls := func(fn *ast.FuncDecl) (repo, self []string) {
		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch x := sel.X.(type) {
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok && id.Name == "s" && x.Sel.Name == "eventRepo" {
					repo = append(repo, sel.Sel.Name)
				}
			case *ast.Ident:
				if x.Name == "s" {
					self = append(self, sel.Sel.Name)
				}
			}
			return true
		})
		return repo, self
	}
	readsEvents := func(fn *ast.FuncDecl) bool {
		repo, self := calls(fn)
		for _, r := range repo {
			if eventStoreReads[r] {
				return true
			}
		}
		for _, name := range self {
			if h, ok := methods[name]; ok && !ast.IsExported(name) {
				hr, _ := calls(h)
				for _, r := range hr {
					if eventStoreReads[r] {
						return true
					}
				}
			}
		}
		return false
	}
	takesViewer := func(fn *ast.FuncDecl) bool {
		for _, p := range fn.Type.Params.List {
			switch tp := p.Type.(type) {
			case *ast.SelectorExpr:
				if tp.Sel.Name == "Viewer" {
					return true
				}
			case *ast.Ident:
				if tp.Name == "int" {
					for _, n := range p.Names {
						if n.Name == "role" {
							return true
						}
					}
				}
			}
		}
		return false
	}
	// Writes take a viewer too, but return no events to show; only reads
	// that hand events (or facts about them) back are in scope.
	isWrite := func(name string) bool {
		for _, p := range []string{"Create", "Update", "Delete", "Set", "Link", "Unlink", "Move", "Apply", "Preview", "Import", "Roll", "Lock", "Tell", "Clear", "Validate"} {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
		return false
	}

	inTable := map[string]bool{}
	for _, r := range everyEventRead {
		inTable[r.name] = true
	}
	var missing []string
	for name, fn := range methods {
		if !ast.IsExported(name) || isWrite(name) || inTable[name] || eventReadExempt[name] != "" {
			continue
		}
		if takesViewer(fn) && readsEvents(fn) {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("these reads return events to a viewer but are not in everyEventRead (add them, or to eventReadExempt with the reason players cannot reach them): %v", missing)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
