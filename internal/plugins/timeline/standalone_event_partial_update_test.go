// standalone_event_partial_update_test.go — sweep R4, the timeline half of
// the absent-means-preserve contract.
//
// Reproduced before the fix: the edit modal PUTs five keys
// ({name, year, month, day, visibility}) and UpdateStandaloneEvent assigned
// all twenty-one fields unguarded, so RENAMING an event cleared eight of
// them at once — its entity link, its rich-text description_html, its start
// and end times, its recurrence config, and its per-player
// visibility_rules, which the edit request struct does not even carry.
//
// The R3 booking predicted the client-side repair would need the inline
// Alpine $dispatch widened to carry raw TipTap HTML through %q into an HTML
// attribute. Server-side presence-merge removes that surface entirely: the
// modal keeps dispatching its ten keys and sending its five.
package timeline

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// storedEvent is the fully-populated row every case starts from.
func storedEvent() *TimelineEvent {
	s := func(v string) *string { return &v }
	i := func(v int) *int { return &v }
	return &TimelineEvent{
		ID:              "evt-1",
		TimelineID:      "tl-1",
		Name:            "The Siege of Ashfall",
		Description:     s("plain text notes"),
		DescriptionHTML: s("<p>a long formatted write-up</p>"),
		EntityID:        s("ent-castle"),
		Year:            1492,
		Month:           3,
		Day:             11,
		StartHour:       i(9),
		StartMinute:     i(30),
		EndYear:         i(1492),
		EndMonth:        i(3),
		EndDay:          i(14),
		EndHour:         i(18),
		EndMinute:       i(0),
		IsRecurring:     true,
		RecurrenceType:  s("yearly"),
		Category:        s("battle"),
		Visibility:      "dm_only",
		VisibilityRules: s(`{"allowed_users":["u-1"]}`),
		Label:           s("Siege"),
		Color:           s("#aa0000"),
	}
}

func runEventUpdate(t *testing.T, input UpdateTimelineEventInput) *TimelineEvent {
	t.Helper()
	var written *TimelineEvent
	repo := &mockTimelineRepo{
		getEventFn:    func(_ context.Context, _ string) (*TimelineEvent, error) { return storedEvent(), nil },
		updateEventFn: func(_ context.Context, e *TimelineEvent) error { written = e; return nil },
	}
	if err := newTestTimelineService(repo).UpdateStandaloneEvent(context.Background(), "tl-1", "evt-1", input); err != nil {
		t.Fatalf("UpdateStandaloneEvent: %v", err)
	}
	if written == nil {
		t.Fatal("nothing was written")
	}
	return written
}

// THE headline regression: the exact five-key body the edit modal sends on a
// rename must change the name and nothing else.
func TestRename_ClearsNothingElse(t *testing.T) {
	got := runEventUpdate(t, UpdateTimelineEventInput{
		Name:       patch.Of("The Siege of Ashfall (revised)"),
		Year:       patch.Of(1492),
		Month:      patch.Of(3),
		Day:        patch.Of(11),
		Visibility: patch.Of("dm_only"),
	})
	want := storedEvent()

	if got.Name != "The Siege of Ashfall (revised)" {
		t.Errorf("Name = %q, want the edited value", got.Name)
	}
	assertPtrEq(t, "EntityID", got.EntityID, want.EntityID)
	assertPtrEq(t, "DescriptionHTML", got.DescriptionHTML, want.DescriptionHTML)
	assertPtrEq(t, "Description", got.Description, want.Description)
	assertPtrEq(t, "RecurrenceType", got.RecurrenceType, want.RecurrenceType)
	assertPtrEq(t, "Category", got.Category, want.Category)
	assertPtrEq(t, "VisibilityRules", got.VisibilityRules, want.VisibilityRules)
	assertPtrEq(t, "Label", got.Label, want.Label)
	assertPtrEq(t, "Color", got.Color, want.Color)
	assertIntPtrEq(t, "StartHour", got.StartHour, want.StartHour)
	assertIntPtrEq(t, "StartMinute", got.StartMinute, want.StartMinute)
	assertIntPtrEq(t, "EndYear", got.EndYear, want.EndYear)
	assertIntPtrEq(t, "EndMonth", got.EndMonth, want.EndMonth)
	assertIntPtrEq(t, "EndDay", got.EndDay, want.EndDay)
	assertIntPtrEq(t, "EndHour", got.EndHour, want.EndHour)
	assertIntPtrEq(t, "EndMinute", got.EndMinute, want.EndMinute)
	if !got.IsRecurring {
		t.Error("IsRecurring = false, want true preserved")
	}
}

// All three directions, on the field whose loss is access-control data.
func TestStandaloneEvent_AbsentPreserves_PresentReplaces_NullClears(t *testing.T) {
	cases := []struct {
		name  string
		input UpdateTimelineEventInput
		want  *string
	}{
		{"absent preserves the per-player rules", UpdateTimelineEventInput{Name: patch.Of("x")}, strPtrTL(`{"allowed_users":["u-1"]}`)},
		{"present replaces them", UpdateTimelineEventInput{VisibilityRules: patch.Of(`{"denied_users":["u-2"]}`)}, strPtrTL(`{"denied_users":["u-2"]}`)},
		{"explicit null clears them", UpdateTimelineEventInput{VisibilityRules: patch.Null[string]()}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runEventUpdate(t, tc.input)
			assertPtrEq(t, "VisibilityRules", got.VisibilityRules, tc.want)
		})
	}
}

// An input that mentions nothing must change nothing — including passing the
// validators, which now read the stored row rather than the empty input.
func TestStandaloneEvent_EmptyInputChangesNothing(t *testing.T) {
	got := runEventUpdate(t, UpdateTimelineEventInput{})
	want := storedEvent()
	if got.Name != want.Name {
		t.Errorf("Name = %q, want preserved", got.Name)
	}
	if got.Visibility != want.Visibility {
		t.Errorf("Visibility = %q, want preserved — an absent visibility must not fall through to the validator's reject", got.Visibility)
	}
	if got.Year != want.Year || got.Month != want.Month || got.Day != want.Day {
		t.Errorf("date = %d-%d-%d, want %d-%d-%d preserved", got.Year, got.Month, got.Day, want.Year, want.Month, want.Day)
	}
}

// The client half: the modal must send an explicit null for a field the
// operator EMPTIED, because absent now means preserve — otherwise a cleared
// description silently comes back.
func TestEditModalClient_SendsNullForEmptiedFields(t *testing.T) {
	src, err := os.ReadFile("timeline.templ")
	if err != nil {
		t.Fatalf("read timeline.templ: %v", err)
	}
	for _, want := range []string{
		"body.description = self.description || null;",
		"body.category = self.category || null;",
		"body.color = self.color || null;",
		"body.label = self.label || null;",
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("timeline.templ no longer sends %q — under absent-means-preserve, an emptied field that is simply omitted can never be cleared", want)
		}
	}
	// …and it must NOT have grown a description_html key: routing raw TipTap
	// HTML through the inline Alpine dispatch is the security surface the
	// server-side fix exists to avoid.
	if strings.Contains(string(src), "body.description_html") {
		t.Error("the edit modal now sends description_html; the server preserves it on absence, so the client must not carry raw editor HTML through the inline dispatch")
	}
}

func strPtrTL(s string) *string { return &s }

func assertPtrEq(t *testing.T, field string, got, want *string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %q, want nil", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %q", field, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %q, want %q", field, *got, *want)
	}
}

func assertIntPtrEq(t *testing.T, field string, got, want *int) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %d, want nil", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %d", field, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %d, want %d", field, *got, *want)
	}
}
