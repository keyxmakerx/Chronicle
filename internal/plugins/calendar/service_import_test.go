// service_import_test.go: table-driven tests for CalendarService's import/
// preset wiring — PreviewImport/PreviewPreset as pure parses,
// and CreateCalendarFromImport's two product-level invariants: the created
// calendar's current date is always traceable to the import file or an
// explicit override (never a silent default), and an event whose kind slug
// doesn't resolve in the target campaign is warned about and created
// without a kind rather than dropped or failing the whole import. Uses the
// same fakes (mocks_test.go) and helpers (strPtr, testCampaignA) as
// service_test.go.
package calendar

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestPreviewImport_MalformedDataIsBadRequestNotInternalError(t *testing.T) {
	svc := newTestCalendarService(nil, nil, nil, nil)
	_, err := svc.PreviewImport(context.Background(), []byte(`{"not":"a calendar"}`))
	if err == nil {
		t.Fatal("expected an error for unrecognized JSON, got nil")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected an *apperror.AppError (never a raw parse error), got %T: %v", err, err)
	}
	if appErr.Code < 400 || appErr.Code >= 500 {
		t.Errorf("expected a 4xx client error (the file's problem, not a 500), got code=%d type=%s", appErr.Code, appErr.Type)
	}
}

func TestPreviewPreset_UnknownNameIsNotFound(t *testing.T) {
	svc := newTestCalendarService(nil, nil, nil, nil)
	_, err := svc.PreviewPreset(context.Background(), "does-not-exist")
	assertNotFound(t, err)
}

func TestPreviewPreset_ShippedPresetParses(t *testing.T) {
	svc := newTestCalendarService(nil, nil, nil, nil)
	ir, err := svc.PreviewPreset(context.Background(), "blank")
	if err != nil {
		t.Fatalf("PreviewPreset(\"blank\"): %v", err)
	}
	if ir.CalendarName == "" {
		t.Error("expected a non-empty calendar name from the blank preset")
	}
}

func TestCreateCalendarFromImport_NilResultIsValidationError(t *testing.T) {
	svc := newTestCalendarService(nil, nil, nil, nil)
	_, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, nil, CreateCalendarFromImportOptions{})
	if err == nil {
		t.Fatal("expected an error for a nil import result, got nil")
	}
}

// TestCreateCalendarFromImport_CurrentDateInvariant is the direct test of
// #741's "an import never silently resets the calendar's current date":
// whatever ends up as the created calendar's current date must trace to
// either what the import specified (ir.Today) or an explicit opts override
// — CreateCalendarFromImport must error rather than invent day 1 when
// neither supplies a day-level date.
func TestCreateCalendarFromImport_CurrentDateInvariant(t *testing.T) {
	tests := []struct {
		name       string
		today      ImportedToday
		opts       CreateCalendarFromImportOptions
		wantErr    bool
		wantErrHas string
		wantYear   int
		wantMonth  int
		wantDay    int
	}{
		{
			name:       "Calendaria-shaped: year only, no override at all -> error naming the missing month",
			today:      ImportedToday{Year: 100},
			wantErr:    true,
			wantErrHas: "current_month",
		},
		{
			name:       "year only, month override but no day override -> error naming the missing day",
			today:      ImportedToday{Year: 100},
			opts:       CreateCalendarFromImportOptions{CurrentMonth: intPtrForTest(3)},
			wantErr:    true,
			wantErrHas: "current_day",
		},
		{
			name:      "year only, both month and day explicitly confirmed -> succeeds using the confirmed date",
			today:     ImportedToday{Year: 100},
			opts:      CreateCalendarFromImportOptions{CurrentMonth: intPtrForTest(3), CurrentDay: intPtrForTest(4)},
			wantYear:  100,
			wantMonth: 3,
			wantDay:   4,
		},
		{
			name:      "Chronicle-shaped: fully specified by the import, no override needed",
			today:     ImportedToday{Year: 5, Month: intPtrForTest(2), Day: intPtrForTest(10)},
			wantYear:  5,
			wantMonth: 2,
			wantDay:   10,
		},
		{
			name:      "fully specified by the import, but the caller overrides all three explicitly",
			today:     ImportedToday{Year: 5, Month: intPtrForTest(2), Day: intPtrForTest(10)},
			opts:      CreateCalendarFromImportOptions{CurrentYear: intPtrForTest(9), CurrentMonth: intPtrForTest(8), CurrentDay: intPtrForTest(7)},
			wantYear:  9,
			wantMonth: 8,
			wantDay:   7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestCalendarService(nil, nil, nil, nil)
			ir := &ImportResult{CalendarName: "Test Calendar", Today: tt.today}
			cal, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, ir, tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrHas) {
					t.Errorf("error = %q, want it to mention %q", err.Error(), tt.wantErrHas)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateCalendarFromImport: %v", err)
			}
			if cal.CurrentYear != tt.wantYear || cal.CurrentMonth != tt.wantMonth || cal.CurrentDay != tt.wantDay {
				t.Errorf("current date = %d-%d-%d, want %d-%d-%d",
					cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, tt.wantYear, tt.wantMonth, tt.wantDay)
			}
		})
	}
}

// TestCreateCalendarFromImport_EventKindSlugFallback covers #741's "warn,
// never refuse": an imported event referencing a kind slug that doesn't
// exist in the target campaign is created WITHOUT a kind, with a warning
// recorded on ir.Warnings, rather than dropped or aborting the import.
func TestCreateCalendarFromImport_EventKindSlugFallback(t *testing.T) {
	var createdEvents []*Event
	eventRepo := &fakeEventRepo{
		createEventFn: func(_ context.Context, evt *Event) error {
			createdEvents = append(createdEvents, evt)
			return nil
		},
	}
	kindRepo := &fakeEventKindRepo{
		listFn: func(_ context.Context, _ string) ([]EventKind, error) {
			return []EventKind{{ID: 7, Slug: "festival", Name: "Festival"}}, nil
		},
	}
	svc := newTestCalendarService(nil, eventRepo, kindRepo, nil)

	festivalSlug := "festival"
	missingSlug := "does-not-exist"
	ir := &ImportResult{
		CalendarName: "Test Calendar",
		Today:        ImportedToday{Year: 1, Month: intPtrForTest(1), Day: intPtrForTest(1)},
		Events: []ExportEvent{
			{Name: "Founding Day", Year: 1, Month: 1, Day: 1, Visibility: "everyone", Kind: &festivalSlug},
			{Name: "Mystery Event", Year: 1, Month: 2, Day: 2, Visibility: "everyone", Kind: &missingSlug},
		},
	}

	if _, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, ir, CreateCalendarFromImportOptions{}); err != nil {
		t.Fatalf("CreateCalendarFromImport: %v", err)
	}

	if len(createdEvents) != 2 {
		t.Fatalf("got %d created events, want 2", len(createdEvents))
	}
	if createdEvents[0].KindID == nil || *createdEvents[0].KindID != 7 {
		t.Errorf("event with a resolvable slug: KindID = %v, want 7", createdEvents[0].KindID)
	}
	if createdEvents[1].KindID != nil {
		t.Errorf("event with an unresolvable slug: KindID = %v, want nil (imported without a kind)", createdEvents[1].KindID)
	}

	foundWarning := false
	for _, w := range ir.Warnings {
		if strings.Contains(w, missingSlug) {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("expected a warning naming %q, got %v", missingSlug, ir.Warnings)
	}
}

// TestCreateCalendarFromImport_NoEventsIsANoOp confirms the common case (a
// preset, or an import with no events) never touches EventRepository.
func TestCreateCalendarFromImport_NoEventsIsANoOp(t *testing.T) {
	called := false
	eventRepo := &fakeEventRepo{
		createEventFn: func(_ context.Context, _ *Event) error {
			called = true
			return nil
		},
	}
	svc := newTestCalendarService(nil, eventRepo, nil, nil)
	ir := &ImportResult{CalendarName: "Test Calendar", Today: ImportedToday{Year: 1, Month: intPtrForTest(1), Day: intPtrForTest(1)}}
	if _, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, ir, CreateCalendarFromImportOptions{}); err != nil {
		t.Fatalf("CreateCalendarFromImport: %v", err)
	}
	if called {
		t.Error("CreateEvent was called for an import with no events")
	}
}

// TestCreateCalendarFromImport_PropagatesMode covers the Mode round-trip
// fix alongside #779: a Chronicle import specifying reallife mode must
// create a reallife calendar, and one that leaves Mode unspecified (every
// external format) must still default to fantasy exactly like a manual
// CreateCalendar call with no mode does.
func TestCreateCalendarFromImport_PropagatesMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		wantMode string
	}{
		{"unspecified (external format) defaults to fantasy", "", ModeFantasy},
		{"explicit reallife from a Chronicle export", ModeRealLife, ModeRealLife},
		{"explicit fantasy from a Chronicle export", ModeFantasy, ModeFantasy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestCalendarService(nil, nil, nil, nil)
			ir := &ImportResult{
				CalendarName: "Test Calendar",
				Today:        ImportedToday{Year: 1, Month: intPtrForTest(1), Day: intPtrForTest(1)},
				Settings:     ImportedSettings{Mode: tt.mode},
			}
			cal, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, ir, CreateCalendarFromImportOptions{})
			if err != nil {
				t.Fatalf("CreateCalendarFromImport: %v", err)
			}
			if cal.Mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", cal.Mode, tt.wantMode)
			}
		})
	}
}

func intPtrForTest(v int) *int { return &v }

// TestCreateCalendarFromImport_EventsDefaultToDmOnlyVisibility is the direct
// test of the fail-closed fix: every recreated event ends up "dm_only"
// regardless of what the source's own Visibility said, because Chronicle's
// export format has no way to carry a per-event visibility_rules allow-list
// forward — an event narrowly shared in the source campaign could otherwise
// round-trip as plain "everyone" here. One summary warning covers the whole
// import, not one per event.
func TestCreateCalendarFromImport_EventsDefaultToDmOnlyVisibility(t *testing.T) {
	var createdEvents []*Event
	eventRepo := &fakeEventRepo{
		createEventFn: func(_ context.Context, evt *Event) error {
			createdEvents = append(createdEvents, evt)
			return nil
		},
	}
	svc := newTestCalendarService(nil, eventRepo, nil, nil)

	ir := &ImportResult{
		CalendarName: "Test Calendar",
		Today:        ImportedToday{Year: 1, Month: intPtrForTest(1), Day: intPtrForTest(1)},
		Events: []ExportEvent{
			{Name: "Public Festival", Year: 1, Month: 1, Day: 1, Visibility: "everyone"},
			{Name: "Another Public Event", Year: 1, Month: 1, Day: 2, Visibility: "everyone"},
		},
	}

	if _, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, ir, CreateCalendarFromImportOptions{}); err != nil {
		t.Fatalf("CreateCalendarFromImport: %v", err)
	}

	if len(createdEvents) != 2 {
		t.Fatalf("got %d created events, want 2", len(createdEvents))
	}
	for _, evt := range createdEvents {
		if evt.Visibility != "dm_only" {
			t.Errorf("event %q: Visibility = %q, want \"dm_only\" (fail closed) even though the source said %q",
				evt.Name, evt.Visibility, "everyone")
		}
	}

	// A Player-tier viewer must not be able to see either imported event.
	player := playerViewer("u-player")
	owner := ownerViewer("u-owner")
	for _, evt := range createdEvents {
		if eventVisibleToViewer(*evt, player) {
			t.Errorf("event %q must not be visible to a Player", evt.Name)
		}
		if !eventVisibleToViewer(*evt, owner) {
			t.Errorf("event %q must be visible to the Owner", evt.Name)
		}
	}

	summaryCount := 0
	for _, w := range ir.Warnings {
		if strings.Contains(w, "Director-only visibility") {
			summaryCount++
		}
	}
	if summaryCount != 1 {
		t.Errorf("expected exactly one summary warning about the dm_only default, got %d in %v", summaryCount, ir.Warnings)
	}
}

// TestCreateCalendarFromImport_DescriptionHTMLIsSanitized pins that an
// imported event's description_html goes through the same sanitize.HTML
// path CreateEvent uses — see TestCreateEvent_DescriptionHTMLIsSanitized for
// the direct (hand-created) equivalent this mirrors.
func TestCreateCalendarFromImport_DescriptionHTMLIsSanitized(t *testing.T) {
	var created *Event
	eventRepo := &fakeEventRepo{
		createEventFn: func(_ context.Context, evt *Event) error {
			created = evt
			return nil
		},
	}
	svc := newTestCalendarService(nil, eventRepo, nil, nil)

	dirty := `<p onclick="alert(1)">hi</p><script>alert(2)</script>`
	ir := &ImportResult{
		CalendarName: "Test Calendar",
		Today:        ImportedToday{Year: 1, Month: intPtrForTest(1), Day: intPtrForTest(1)},
		Events: []ExportEvent{
			{Name: "Tainted Event", Year: 1, Month: 1, Day: 1, Visibility: "everyone", DescriptionHTML: &dirty},
		},
	}

	if _, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, ir, CreateCalendarFromImportOptions{}); err != nil {
		t.Fatalf("CreateCalendarFromImport: %v", err)
	}
	if created == nil {
		t.Fatal("expected the event to be created")
	}
	if created.DescriptionHTML == nil {
		t.Fatal("DescriptionHTML must not be dropped entirely")
	}
	got := *created.DescriptionHTML
	if got == dirty {
		t.Errorf("imported DescriptionHTML was stored unsanitized: %q", got)
	}
	if strings.Contains(got, "onclick") || strings.Contains(got, "<script") {
		t.Errorf("sanitize.HTML did not strip dangerous markup from an imported event, got %q", got)
	}
}

// TestCreateCalendarFromImport_InvalidEventIsSkippedNotAborted covers the
// "warn, don't abort" half of the validation fix: an event that fails
// buildValidatedEvent's checks (here, a blank name) is skipped with its own
// warning rather than failing the whole import — a sibling valid event
// still gets created.
func TestCreateCalendarFromImport_InvalidEventIsSkippedNotAborted(t *testing.T) {
	var createdEvents []*Event
	eventRepo := &fakeEventRepo{
		createEventFn: func(_ context.Context, evt *Event) error {
			createdEvents = append(createdEvents, evt)
			return nil
		},
	}
	svc := newTestCalendarService(nil, eventRepo, nil, nil)

	ir := &ImportResult{
		CalendarName: "Test Calendar",
		Today:        ImportedToday{Year: 1, Month: intPtrForTest(1), Day: intPtrForTest(1)},
		Events: []ExportEvent{
			{Name: "", Year: 1, Month: 1, Day: 1, Visibility: "everyone"}, // fails ValidateRequired("name", ...)
			{Name: "Valid Event", Year: 1, Month: 1, Day: 2, Visibility: "everyone"},
		},
	}

	if _, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, ir, CreateCalendarFromImportOptions{}); err != nil {
		t.Fatalf("CreateCalendarFromImport must not abort over one invalid event: %v", err)
	}
	if len(createdEvents) != 1 {
		t.Fatalf("got %d created events, want exactly 1 (the valid one)", len(createdEvents))
	}
	if createdEvents[0].Name != "Valid Event" {
		t.Errorf("created event = %q, want the valid one", createdEvents[0].Name)
	}

	foundWarning := false
	for _, w := range ir.Warnings {
		if strings.Contains(w, "skipped") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("expected a warning about the skipped event, got %v", ir.Warnings)
	}
}

// TestCreateCalendarFromImport_FailedApplyImportDeletesTheBareCalendar is
// the direct test of the compensating-cleanup fix: when ApplyImport fails
// (a malformed payload, a bad column value — anything past the bare
// calendar row's own creation), the just-created row is deleted rather than
// left behind as an orphaned, empty calendar a retry would pile up next to.
func TestCreateCalendarFromImport_FailedApplyImportDeletesTheBareCalendar(t *testing.T) {
	deleteCalls := 0
	var deletedID string
	calRepo := &fakeCalendarRepo{
		applyImportFn: func(_ context.Context, _ *Calendar, _ *ImportResult) error {
			return errors.New("boom: malformed payload")
		},
		deleteFn: func(_ context.Context, id string) error {
			deleteCalls++
			deletedID = id
			return nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)
	ir := &ImportResult{CalendarName: "Test Calendar", Today: ImportedToday{Year: 1, Month: intPtrForTest(1), Day: intPtrForTest(1)}}

	_, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, ir, CreateCalendarFromImportOptions{})
	if err == nil {
		t.Fatal("expected the ApplyImport error to propagate")
	}
	if !strings.Contains(err.Error(), "malformed payload") {
		t.Errorf("expected the original cause in the error, got: %v", err)
	}
	if deleteCalls != 1 {
		t.Fatalf("expected exactly one cleanup Delete call, got %d", deleteCalls)
	}
	if deletedID == "" {
		t.Error("Delete was called with an empty calendar id")
	}
}

// TestCreateCalendarFromImport_FailedEventRecreationDeletesTheBareCalendar
// is the same cleanup invariant, triggered by applyImportedEvents' own DB
// error path (a genuine write failure, not a validation-skip).
func TestCreateCalendarFromImport_FailedEventRecreationDeletesTheBareCalendar(t *testing.T) {
	deleteCalls := 0
	calRepo := &fakeCalendarRepo{
		deleteFn: func(_ context.Context, _ string) error {
			deleteCalls++
			return nil
		},
	}
	eventRepo := &fakeEventRepo{
		createEventFn: func(_ context.Context, _ *Event) error {
			return errors.New("boom: db write failed")
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)
	ir := &ImportResult{
		CalendarName: "Test Calendar",
		Today:        ImportedToday{Year: 1, Month: intPtrForTest(1), Day: intPtrForTest(1)},
		Events:       []ExportEvent{{Name: "Some Event", Year: 1, Month: 1, Day: 1, Visibility: "everyone"}},
	}

	_, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, ir, CreateCalendarFromImportOptions{})
	if err == nil {
		t.Fatal("expected the event-creation error to propagate")
	}
	if deleteCalls != 1 {
		t.Fatalf("expected exactly one cleanup Delete call, got %d", deleteCalls)
	}
}

// TestCreateCalendarFromImport_CurrentDateOutOfRangeIsRejected is the direct
// test of validateImportCurrentDate: a month or day that doesn't fit the
// import's own month structure must be a clean validation error, not land
// unchecked in current_month/current_day.
func TestCreateCalendarFromImport_CurrentDateOutOfRangeIsRejected(t *testing.T) {
	months := []MonthInput{{Name: "First", Days: 30, SortOrder: 0}, {Name: "Second", Days: 28, SortOrder: 1}}

	tests := []struct {
		name       string
		today      ImportedToday
		wantErrHas string
	}{
		{
			name:       "month beyond the calendar's 2 months",
			today:      ImportedToday{Year: 1, Month: intPtrForTest(5), Day: intPtrForTest(1)},
			wantErrHas: "current_month",
		},
		{
			name:       "month zero",
			today:      ImportedToday{Year: 1, Month: intPtrForTest(0), Day: intPtrForTest(1)},
			wantErrHas: "current_month",
		},
		{
			name:       "day beyond the second month's 28 days",
			today:      ImportedToday{Year: 1, Month: intPtrForTest(2), Day: intPtrForTest(99)},
			wantErrHas: "current_day",
		},
		{
			name:       "in range: last day of the last month",
			today:      ImportedToday{Year: 1, Month: intPtrForTest(2), Day: intPtrForTest(28)},
			wantErrHas: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestCalendarService(nil, nil, nil, nil)
			ir := &ImportResult{CalendarName: "Test Calendar", Today: tt.today, Months: months}
			_, err := svc.CreateCalendarFromImport(context.Background(), testCampaignA, ir, CreateCalendarFromImportOptions{})
			if tt.wantErrHas == "" {
				if err != nil {
					t.Fatalf("expected an in-range date to succeed, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrHas) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tt.wantErrHas)
			}
		})
	}
}
