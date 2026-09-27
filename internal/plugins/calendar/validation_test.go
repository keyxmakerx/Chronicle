// validation_test.go: table-driven input-validation tests for
// CalendarService — announced/visibility/payload shape, icon/color/text
// column-width and character-class checks, plus the required-field checks
// each Create/Update method states.
package calendar

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// wantValidationErr accepts either 400 (apperror.ValidateRequired's "missing
// required field") or 422 (apperror.NewValidation's "value doesn't match
// the supported set") — both are the service correctly rejecting bad input;
// which of the two apperror constructors a given check uses is an
// implementation detail this test does not pin.
func wantValidationErr(t *testing.T, err error, label string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected a client-error rejection, got nil", label)
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("%s: expected *apperror.AppError, got %T: %v", label, err, err)
	}
	if appErr.Code != 400 && appErr.Code != 422 {
		t.Fatalf("%s: expected status 400 or 422, got %d (%s): %v", label, appErr.Code, appErr.Type, err)
	}
}

func TestCreateCalendar_Validation(t *testing.T) {
	calRepo := &fakeCalendarRepo{}
	svc := newTestCalendarService(calRepo, nil, nil, nil)

	tests := []struct {
		name  string
		input CreateCalendarInput
	}{
		{"blank name is rejected", CreateCalendarInput{Name: ""}},
		{"unsupported mode is rejected", CreateCalendarInput{Name: "Calendar", Mode: "gregorian"}},
		{"negative hours_per_day is rejected", CreateCalendarInput{Name: "Calendar", HoursPerDay: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateCalendar(context.Background(), testCampaignA, tt.input)
			wantValidationErr(t, err, tt.name)
		})
	}

	// Positive control + defaulting: a valid create with no mode/geometry
	// fields set must succeed and default to fantasy / 24h / 60m / 60s.
	var created *Calendar
	calRepo.createFn = func(_ context.Context, cal *Calendar) error {
		created = cal
		return nil
	}
	cal, err := svc.CreateCalendar(context.Background(), testCampaignA, CreateCalendarInput{Name: "New Calendar"})
	if err != nil {
		t.Fatalf("valid create must succeed: %v", err)
	}
	if cal.Mode != ModeFantasy || cal.HoursPerDay != 24 || cal.MinutesPerHour != 60 || cal.SecondsPerMinute != 60 {
		t.Errorf("unexpected defaults: mode=%q hours=%d minutes=%d seconds=%d", cal.Mode, cal.HoursPerDay, cal.MinutesPerHour, cal.SecondsPerMinute)
	}
	if created == nil || created.CampaignID != testCampaignA {
		t.Errorf("Create must be called with a calendar scoped to the campaign, got %+v", created)
	}
}

func TestUpdateCalendar_Validation(t *testing.T) {
	stored := Calendar{ID: "cal-1", CampaignID: testCampaignA, Name: "Old Name", Mode: ModeFantasy,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60}
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			c := stored
			return &c, nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)

	t.Run("blank name fails loudly instead of silently preserving", func(t *testing.T) {
		err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{Name: ""})
		wantValidationErr(t, err, "blank name")
	})
	t.Run("unsupported mode is rejected", func(t *testing.T) {
		err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{
			Name: "Old Name", Mode: patch.Of("gregorian"),
		})
		wantValidationErr(t, err, "bad mode")
	})
	t.Run("enabling real-time without a zone is rejected", func(t *testing.T) {
		enable := true
		err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{
			Name: "Old Name", SetRealTime: &enable,
		})
		wantValidationErr(t, err, "real-time enable with no zone")
	})

	t.Run("absent fields preserve the stored geometry (partial update)", func(t *testing.T) {
		var written *Calendar
		calRepo.updateFn = func(_ context.Context, cal *Calendar) error {
			written = cal
			return nil
		}
		// A rename-only push must not reset hours/minutes/seconds — the
		// exact class of bug the partial-update contract exists to prevent.
		err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{
			Name: "New Name",
		})
		if err != nil {
			t.Fatalf("rename-only update: %v", err)
		}
		if written.HoursPerDay != 24 || written.MinutesPerHour != 60 || written.SecondsPerMinute != 60 {
			t.Errorf("rename-only push must preserve the stored geometry, got hours=%d minutes=%d seconds=%d",
				written.HoursPerDay, written.MinutesPerHour, written.SecondsPerMinute)
		}
		if written.Name != "New Name" {
			t.Errorf("the field actually sent must still apply, got name=%q", written.Name)
		}
	})
}

func TestCreateEvent_Validation(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	eventRepo := &fakeEventRepo{}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	tests := []struct {
		name  string
		input CreateEventInput
	}{
		{"blank name", CreateEventInput{Name: ""}},
		{"unsupported visibility", CreateEventInput{Name: "Feast", Visibility: "secret"}},
		{"unsupported recurrence_type", CreateEventInput{Name: "Feast", RecurrenceType: strPtr("fortnightly")}},
		{"unsupported announced", CreateEventInput{Name: "Feast", Announced: strPtr("sometimes")}},
		{"malformed payload JSON", CreateEventInput{Name: "Feast", Payload: strPtr("{not json")}},
		// CanAuthorDmOnly: true so this reaches the JSON-shape check at all —
		// a non-empty visibility_rules from a non-author is refused first,
		// which is not what this case means to test.
		{"malformed visibility_rules JSON", CreateEventInput{Name: "Feast", VisibilityRules: strPtr("{not json"), CanAuthorDmOnly: true}},
		// calendar_events.color/icon are VARCHAR(20)/VARCHAR(50) — the LIVE
		// schema (migration 002 widened them past their original 001
		// definition); a value that doesn't fit must fail as a clean
		// validation error here, never reach the driver as a raw "data too
		// long" error. These are well-FORMED (pattern-valid) but too long,
		// so they pin the LENGTH rule specifically — see
		// TestCreateEvent_Validation_LengthVsPatternAreIndependentChecks for
		// the explicit length-vs-pattern split.
		{"color longer than the VARCHAR(20) column", CreateEventInput{Name: "Feast", Color: strPtr("#" + strings.Repeat("a", 25))}},
		{"icon longer than the VARCHAR(50) column", CreateEventInput{Name: "Feast", Icon: strPtr("fa-" + strings.Repeat("a", 50))}},
		// An event's icon/color are optional kind-overrides, but any
		// non-empty value goes through the same closed-character-class
		// checks as an event kind's: icons and colors are interpolated into
		// class and style attributes by the clients that render them, so a
		// quote or an angle bracket must be structurally impossible.
		{"non-fa icon (emoji) is refused", CreateEventInput{Name: "Feast", Icon: strPtr("⭐")}},
		{"icon with a quote is refused (HTML-injection shape)", CreateEventInput{Name: "Feast", Icon: strPtr(`fa-x" onload="`)}},
		{"icon with an angle bracket is refused", CreateEventInput{Name: "Feast", Icon: strPtr("fa-x<b>")}},
		{"icon with a space is refused", CreateEventInput{Name: "Feast", Icon: strPtr("fa x")}},
		{"uppercase icon is refused", CreateEventInput{Name: "Feast", Icon: strPtr("FA-X")}},
		{"non-hex color is refused", CreateEventInput{Name: "Feast", Color: strPtr("red")}},
		{"color with a quote is refused (HTML-injection shape)", CreateEventInput{Name: "Feast", Color: strPtr(`#f"f`)}},
		{"color with an angle bracket is refused", CreateEventInput{Name: "Feast", Color: strPtr("#f<b")}},
		{"color with a space is refused", CreateEventInput{Name: "Feast", Color: strPtr("#f f")}},
		// N2: text columns.
		{"description over the TEXT column's byte capacity", CreateEventInput{Name: "Feast", Description: strPtr(strings.Repeat("d", maxTextColumnBytes+1))}},
		{"description_html over the TEXT column's byte capacity", CreateEventInput{Name: "Feast", DescriptionHTML: strPtr(strings.Repeat("d", maxTextColumnBytes+1))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateEvent(context.Background(), "cal-1", testCampaignA, tt.input)
			wantValidationErr(t, err, tt.name)
		})
	}
}

// TestCreateEvent_Validation_LengthVsPatternAreIndependentChecks (N1) proves
// icon/color length and pattern are two SEPARATE checks: a value can be
// pattern-valid but too long (length fails first), or short but
// pattern-invalid (pattern fails), and the two failures are distinguishable.
func TestCreateEvent_Validation_LengthVsPatternAreIndependentChecks(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	svc := newTestCalendarService(calRepo, &fakeEventRepo{}, nil, nil)

	// Pattern-valid, too long: fails on LENGTH.
	tooLongIcon := "fa-" + strings.Repeat("a", 50) // 53 chars, valid fa- shape, over the 50-char column
	_, err := svc.CreateEvent(context.Background(), "cal-1", testCampaignA, CreateEventInput{Name: "F", Icon: &tooLongIcon})
	wantErrorContains(t, err, "too long", "over-length but well-formed icon")

	// Short enough, malformed: fails on PATTERN, not length.
	badIcon := "FA-X"
	_, err = svc.CreateEvent(context.Background(), "cal-1", testCampaignA, CreateEventInput{Name: "F", Icon: &badIcon})
	wantErrorContains(t, err, "Font Awesome", "short but malformed icon")

	tooLongColor := "#" + strings.Repeat("a", 25) // valid hex digits, over the 20-char column
	_, err = svc.CreateEvent(context.Background(), "cal-1", testCampaignA, CreateEventInput{Name: "F", Color: &tooLongColor})
	wantErrorContains(t, err, "too long", "over-length but well-formed color")

	badColor := "red"
	_, err = svc.CreateEvent(context.Background(), "cal-1", testCampaignA, CreateEventInput{Name: "F", Color: &badColor})
	wantErrorContains(t, err, "hex color", "short but malformed color")
}

// wantErrorContains fails unless err is non-nil and its message contains
// substr — used where the test cares WHICH rule rejected the input, not
// just that something did.
func wantErrorContains(t *testing.T, err error, substr, label string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected an error containing %q, got nil", label, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("%s: expected error to contain %q, got: %v", label, substr, err)
	}
}

// TestCreateEvent_IconAndColor_AbsentOrValidAccepted is the positive control
// for the icon/color cases above: leaving both unset (inherit from the
// kind) and a genuine fa-/hex value must both be accepted.
func TestCreateEvent_IconAndColor_AbsentOrValidAccepted(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	svc := newTestCalendarService(calRepo, &fakeEventRepo{}, nil, nil)

	if _, err := svc.CreateEvent(context.Background(), "cal-1", testCampaignA, CreateEventInput{Name: "Feast"}); err != nil {
		t.Errorf("no icon/color override at all must be accepted, got: %v", err)
	}
	if _, err := svc.CreateEvent(context.Background(), "cal-1", testCampaignA, CreateEventInput{
		Name: "Feast", Icon: strPtr("fa-star"), Color: strPtr("#fff"),
	}); err != nil {
		t.Errorf("a valid fa- icon and hex color must be accepted, got: %v", err)
	}
}

// TestCreateEvent_DescriptionHTMLIsSanitized pins the sanitize-on-write
// invariant: an event's HTML description is sanitized before it is stored.
func TestCreateEvent_DescriptionHTMLIsSanitized(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	var created *Event
	eventRepo := &fakeEventRepo{
		createEventFn: func(_ context.Context, evt *Event) error {
			created = evt
			return nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	dirty := `<p onclick="alert(1)">hi</p><script>alert(2)</script>`
	_, err := svc.CreateEvent(context.Background(), "cal-1", testCampaignA, CreateEventInput{
		Name: "Feast", DescriptionHTML: &dirty,
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if created.DescriptionHTML == nil {
		t.Fatal("DescriptionHTML must not be dropped entirely")
	}
	got := *created.DescriptionHTML
	if got == dirty {
		t.Errorf("DescriptionHTML was stored unsanitized: %q", got)
	}
	if strings.Contains(got, "onclick") || strings.Contains(got, "<script") {
		t.Errorf("sanitize.HTML did not strip dangerous markup, got %q", got)
	}
}

func TestUpdateEvent_PartialUpdatePreservesAbsentFields(t *testing.T) {
	visRules := `{"allowed_users":["u-1"]}`
	stored := Event{
		ID: "evt-1", CalendarID: "cal-1", Name: "Old Name", Visibility: "dm_only",
		VisibilityRules: &visRules, IsRecurring: true, RecurrenceType: strPtr("yearly"), AllDay: true,
	}
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	var written *Event
	eventRepo := &fakeEventRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			e := stored
			return &e, nil
		},
		updateEventFn: func(_ context.Context, evt *Event) error {
			written = evt
			return nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	// A rename-only PUT (mirrors CreateStandaloneEvent/UpdateEvent siblings'
	// "a rename must not touch anything else" regression class). The owner
	// viewer keeps this test focused on the merge mechanics — the
	// authorization rule for a NON-author has its own test.
	err := svc.UpdateEvent(context.Background(), "evt-1", "cal-1", testCampaignA, UpdateEventInput{
		Name: patch.Of("New Name"),
	}, ownerViewer("u-owner"))
	if err != nil {
		t.Fatalf("rename-only update: %v", err)
	}
	if written.Name != "New Name" {
		t.Errorf("the field actually sent must apply, got name=%q", written.Name)
	}
	if written.Visibility != "dm_only" {
		t.Errorf("visibility must be preserved, got %q", written.Visibility)
	}
	if written.VisibilityRules == nil || *written.VisibilityRules != visRules {
		t.Errorf("visibility_rules must be preserved, got %v", written.VisibilityRules)
	}
	if !written.IsRecurring || written.RecurrenceType == nil || *written.RecurrenceType != "yearly" {
		t.Errorf("recurrence must be preserved, got is_recurring=%v type=%v", written.IsRecurring, written.RecurrenceType)
	}
	if !written.AllDay {
		t.Error("all_day must be preserved")
	}

	// An explicit null DOES clear — proving absent and null are genuinely
	// distinct outcomes, not just "null happens to also preserve".
	err = svc.UpdateEvent(context.Background(), "evt-1", "cal-1", testCampaignA, UpdateEventInput{
		Name:            patch.Of("New Name"),
		VisibilityRules: patch.Null[string](),
	}, ownerViewer("u-owner"))
	if err != nil {
		t.Fatalf("explicit-null update: %v", err)
	}
	if written.VisibilityRules != nil {
		t.Errorf("an explicit null must clear visibility_rules, got %v", *written.VisibilityRules)
	}
}

func TestSetEventVisibility_Validation(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			return &Event{ID: id, CalendarID: "cal-1", Visibility: "everyone"}, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	err := svc.SetEventVisibility(context.Background(), "evt-1", "cal-1", testCampaignA, UpdateEventVisibilityInput{
		Visibility: "secret",
	}, ownerViewer("u-owner"))
	wantValidationErr(t, err, "unsupported visibility value")

	err = svc.SetEventVisibility(context.Background(), "evt-1", "cal-1", testCampaignA, UpdateEventVisibilityInput{
		Visibility:      "everyone",
		VisibilityRules: patch.Of("{not json"),
	}, ownerViewer("u-owner"))
	wantValidationErr(t, err, "malformed visibility_rules JSON")
}

func TestEventKind_Validation(t *testing.T) {
	svc := newTestCalendarService(nil, nil, &fakeEventKindRepo{}, nil)

	tests := []struct {
		name  string
		input EventKindInput
	}{
		{"blank name", EventKindInput{Slug: "holiday", Name: ""}},
		{"blank slug", EventKindInput{Slug: "", Name: "Holiday"}},
		{"slug with uppercase/spaces is rejected", EventKindInput{Slug: "Holiday Time", Name: "Holiday"}},
		{"unsupported default_announced", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa-star", Color: "#84cc16", DefaultAnnounced: "sometimes"}},
		{"blank icon is refused, not silently accepted", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "", Color: "#84cc16", DefaultAnnounced: AnnouncedOnDay}},
		{"non-fa icon (emoji) is refused", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "⭐", Color: "#84cc16", DefaultAnnounced: AnnouncedOnDay}},
		{"icon with a quote is refused (HTML-injection shape)", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: `fa-star" onload="alert(1)`, Color: "#84cc16", DefaultAnnounced: AnnouncedOnDay}},
		{"icon with an angle bracket is refused", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa-star<script>", Color: "#84cc16", DefaultAnnounced: AnnouncedOnDay}},
		{"icon with a space is refused", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa star", Color: "#84cc16", DefaultAnnounced: AnnouncedOnDay}},
		{"uppercase icon is refused", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "FA-STAR", Color: "#84cc16", DefaultAnnounced: AnnouncedOnDay}},
		{"icon over 50 characters is refused", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa-" + strings.Repeat("a", 50), Color: "#84cc16", DefaultAnnounced: AnnouncedOnDay}},
		{"blank color is refused, not silently accepted", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa-star", Color: ""}},
		{"color with a quote is refused (HTML-injection shape)", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa-star", Color: `#fff" onload="alert(1)`, DefaultAnnounced: AnnouncedOnDay}},
		{"color with an angle bracket is refused", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa-star", Color: "#fff<script>", DefaultAnnounced: AnnouncedOnDay}},
		{"color with a space is refused", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa-star", Color: "#ff ff00"}},
		{"non-hex color name is refused", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa-star", Color: "red"}},
		{"a #-prefixed but non-hex-digit value is refused", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa-star", Color: "#ggezzz"}},
		{"color over 20 characters is refused", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "fa-star", Color: "#" + strings.Repeat("a", 25)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateEventKind(context.Background(), testCampaignA, tt.input)
			wantValidationErr(t, err, tt.name)
		})
	}
}

// TestEventKind_IconAndColor_ValidValuesAccepted is the positive control for
// TestEventKind_Validation's icon/color cases: a genuine Font Awesome name
// and a genuine hex color must NOT be rejected.
func TestEventKind_IconAndColor_ValidValuesAccepted(t *testing.T) {
	svc := newTestCalendarService(nil, nil, &fakeEventKindRepo{}, nil)

	tests := []struct {
		name  string
		slug  string
		icon  string
		color string
	}{
		{"short fa- name, 3-digit hex", "k-1", "fa-star", "#fff"},
		{"hyphenated fa- name, 6-digit hex", "k-2", "fa-calendar-days", "#84cc16"},
		{"fa- name with digits", "k-3", "fa-star2", "#000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateEventKind(context.Background(), testCampaignA, EventKindInput{
				Slug: tt.slug, Name: "Kind", Icon: tt.icon, Color: tt.color, DefaultAnnounced: AnnouncedOnDay,
			})
			if err != nil {
				t.Errorf("valid icon/color must be accepted, got: %v", err)
			}
		})
	}
}

func TestEra_Validation(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)

	t.Run("blank name is rejected", func(t *testing.T) {
		_, err := svc.CreateEra(context.Background(), "cal-1", testCampaignA, EraInput{
			Name: "", StartYear: 1, StartMonth: 1, StartDay: 1,
		})
		wantValidationErr(t, err, "blank era name")
	})
	t.Run("end date before start date is rejected", func(t *testing.T) {
		endYear := 0
		_, err := svc.CreateEra(context.Background(), "cal-1", testCampaignA, EraInput{
			Name: "Backwards Era", StartYear: 10, StartMonth: 1, StartDay: 1, EndYear: &endYear,
		})
		wantValidationErr(t, err, "end before start")
	})
	t.Run("color longer than the VARCHAR(20) column is rejected", func(t *testing.T) {
		_, err := svc.CreateEra(context.Background(), "cal-1", testCampaignA, EraInput{
			Name: "First Age", StartYear: 1, StartMonth: 1, StartDay: 1,
			Color: "this-hex-color-string-is-way-too-long-for-the-column",
		})
		wantValidationErr(t, err, "era color too long")
	})
	// An era's color renders the same way an event kind's does (interpolated
	// into a style attribute), so it goes through the same closed hex
	// character class — a short-but-invalid value must fail on PATTERN, not
	// slip through because only the length was ever checked.
	t.Run("non-hex color is refused", func(t *testing.T) {
		_, err := svc.CreateEra(context.Background(), "cal-1", testCampaignA, EraInput{
			Name: "First Age", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "red",
		})
		wantErrorContains(t, err, "hex color", "non-hex era color")
	})
	t.Run("color with an HTML-injection shape is refused", func(t *testing.T) {
		_, err := svc.CreateEra(context.Background(), "cal-1", testCampaignA, EraInput{
			Name: "First Age", StartYear: 1, StartMonth: 1, StartDay: 1, Color: `"><b>x</b>`,
		})
		wantErrorContains(t, err, "hex color", "era color with an HTML-injection shape")
	})
	t.Run("empty color is accepted (no override)", func(t *testing.T) {
		if _, err := svc.CreateEra(context.Background(), "cal-1", testCampaignA, EraInput{
			Name: "First Age", StartYear: 1, StartMonth: 1, StartDay: 1,
		}); err != nil {
			t.Errorf("an era with no color at all must be accepted, got: %v", err)
		}
	})
	t.Run("valid hex color is accepted", func(t *testing.T) {
		if _, err := svc.CreateEra(context.Background(), "cal-1", testCampaignA, EraInput{
			Name: "First Age", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#84cc16",
		}); err != nil {
			t.Errorf("a valid hex era color must be accepted, got: %v", err)
		}
	})
}

func TestCreateCalendar_EpochNameLength(t *testing.T) {
	svc := newTestCalendarService(&fakeCalendarRepo{}, nil, nil, nil)
	tooLong := strPtr(strings.Repeat("x", 101)) // calendars.epoch_name is VARCHAR(100)
	_, err := svc.CreateCalendar(context.Background(), testCampaignA, CreateCalendarInput{
		Name: "Calendar", EpochName: tooLong,
	})
	wantValidationErr(t, err, "epoch_name longer than the VARCHAR(100) column")
}
