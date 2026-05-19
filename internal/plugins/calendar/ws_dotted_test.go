// ws_dotted_test.go — C-CAL-WS-DOTTED regression guard.
//
// Pins the WS event name catalog: all 5 structural events must use the
// dotted "calendar.X.Y" form, and SetCycles/SetFestivals must emit the
// new sub-resource events in addition to the umbrella structure.updated.
//
// Without this guard, a future refactor that reverts to the short form
// (or drops the new emissions) silently breaks the Foundry-side editor's
// subscription contract — the FM-CAL-EDITOR uses these granular events
// to know which inspector pane to refresh without a full reload.

package calendar

import (
	"context"
	"sync"
	"testing"
)

// recordingPublisher captures every PublishCalendarEvent call so tests
// can assert on the emitted event names.
type recordingPublisher struct {
	mu     sync.Mutex
	events []recordedEvent
}

type recordedEvent struct {
	Type       string
	CampaignID string
	ResourceID string
}

func (r *recordingPublisher) PublishCalendarEvent(eventType, campaignID, resourceID string, _ any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recordedEvent{Type: eventType, CampaignID: campaignID, ResourceID: resourceID})
}

func (r *recordingPublisher) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	for i, e := range r.events {
		out[i] = e.Type
	}
	return out
}

// contains reports whether the slice has at least one entry equal to want.
func contains(haystack []string, want string) bool {
	for _, s := range haystack {
		if s == want {
			return true
		}
	}
	return false
}

// TestSetCycles_EmitsDottedStructureAndCycleEvents pins that SetCycles
// fires BOTH the umbrella `calendar.structure.updated` and the granular
// `calendar.cycle.changed`. Granular event lets the Foundry editor
// refresh only its Cycles pane on remote change instead of a full
// re-fetch.
func TestSetCycles_EmitsDottedStructureAndCycleEvents(t *testing.T) {
	repo := &mockCalendarRepo{
		// SetCycles only succeeds; structure events fire from the
		// service wrapping the repo call.
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
	}
	pub := &recordingPublisher{}
	svc := NewCalendarService(repo)
	svc.SetEventPublisher(pub)

	if err := svc.SetCycles(context.Background(), "cal-1", []CycleInput{{Name: "Test", CycleLength: 7}}); err != nil {
		t.Fatalf("SetCycles returned unexpected error: %v", err)
	}

	got := pub.types()
	if !contains(got, "calendar.structure.updated") {
		t.Errorf("SetCycles did not emit calendar.structure.updated; got %v", got)
	}
	if !contains(got, "calendar.cycle.changed") {
		t.Errorf("SetCycles did not emit calendar.cycle.changed (the granular event added in C-CAL-WS-DOTTED); got %v", got)
	}
}

// TestSetFestivals_EmitsDottedStructureAndFestivalEvents mirrors the
// cycle assertion. SetFestivals previously only fired structure.updated;
// the granular festival event lets the editor refresh just the festival
// pane.
func TestSetFestivals_EmitsDottedStructureAndFestivalEvents(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
	}
	pub := &recordingPublisher{}
	svc := NewCalendarService(repo)
	svc.SetEventPublisher(pub)

	if err := svc.SetFestivals(context.Background(), "cal-1", []FestivalInput{{Name: "Test"}}); err != nil {
		t.Fatalf("SetFestivals returned unexpected error: %v", err)
	}

	got := pub.types()
	if !contains(got, "calendar.structure.updated") {
		t.Errorf("SetFestivals did not emit calendar.structure.updated; got %v", got)
	}
	if !contains(got, "calendar.festival.changed") {
		t.Errorf("SetFestivals did not emit calendar.festival.changed (the granular event added in C-CAL-WS-DOTTED); got %v", got)
	}
}

// TestSetWeather_UsesDottedEventName pins the rename from weather.changed
// to calendar.weather.changed. Same rationale: the bus-side ws.MessageType
// is "calendar.weather.changed"; emitter strings should match 1:1 so the
// app/routes.go adapter doesn't have to translate.
func TestSetWeather_UsesDottedEventName(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
	}
	pub := &recordingPublisher{}
	svc := NewCalendarService(repo)
	svc.SetEventPublisher(pub)

	preset := "rain"
	if err := svc.SetWeather(context.Background(), "cal-1", WeatherInput{PresetID: &preset}); err != nil {
		t.Fatalf("SetWeather returned unexpected error: %v", err)
	}

	got := pub.types()
	if !contains(got, "calendar.weather.changed") {
		t.Errorf("SetWeather did not emit calendar.weather.changed (renamed from weather.changed in C-CAL-WS-DOTTED); got %v", got)
	}
	// Negative assertion: short form must NOT be emitted, otherwise the
	// rename is a no-op and the adapter still has to translate.
	if contains(got, "weather.changed") {
		t.Errorf("SetWeather still emits short-form weather.changed; rename incomplete")
	}
}
