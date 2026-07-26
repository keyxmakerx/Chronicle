// worldstate_wire_test.go — C-CAL-WORLDSTATE-WIRE (2026-07-26).
//
// calendar.worldstate.changed was a publisher-side dead letter: the emitter
// existed, the adapter had no case, and the message was discarded before the
// bus. Nothing the GM authored in calendar_celestial_events had ever reached a
// WebSocket client. Fixing the routing (internal/app) only helps if the
// payload is worth receiving, so this file pins the two properties that make
// it so:
//
//  1. It carries the day's celestial detail with STABLE ids, so a consumer can
//     dedupe a reconnect replay instead of duplicating a note per delivery.
//  2. The dm_only half rides a SEPARATE, separately-flagged message. The hub
//     gates on Message.RequiresDM, a per-message flag, so one message cannot
//     be rich for the GM and redacted for the table. Getting this wrong turns
//     the fix into a leak.
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

// payloadRecordingPublisher captures payloads as well as names —
// recordingPublisher (ws_dotted_test.go) discards them.
type payloadRecordingPublisher struct {
	mu   sync.Mutex
	sent []struct {
		Type    string
		Payload any
	}
}

func (p *payloadRecordingPublisher) PublishCalendarEvent(eventType, _, _ string, payload any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, struct {
		Type    string
		Payload any
	}{eventType, payload})
}

func (p *payloadRecordingPublisher) byType(t string) []any {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []any
	for _, s := range p.sent {
		if s.Type == t {
			out = append(out, s.Payload)
		}
	}
	return out
}

// mixedCelestials is a date carrying one public and one GM-only sky event —
// the exact situation the audience split exists for.
func mixedCelestials() []CelestialEvent {
	return []CelestialEvent{
		{ID: 11, Type: "meteor-shower", Name: "Tears of Selune", StartHour: 22, DurationHours: 4, Visibility: "everyone"},
		{ID: 12, Type: "eclipse", Name: "The Long Dark", StartHour: 13, DurationHours: 1, Visibility: "dm_only"},
	}
}

// TestBuildWorldStateChangePayloads_SplitsByAudience is the security pin. The
// player-safe copy must never contain a dm_only row; the DM copy must contain
// every row; and both must describe the same date and mood so a consumer that
// only ever sees one of them still gets a coherent world state.
func TestBuildWorldStateChangePayloads_SplitsByAudience(t *testing.T) {
	cal := sampleSeedCalendar()
	playerSafe, dmCopy := BuildWorldStateChangePayloads(
		cal, 1492, 4, 15, &DayWeather{WeatherType: "rain"}, mixedCelestials(), nil)

	if playerSafe == nil {
		t.Fatal("player-safe payload must always be produced")
	}
	if dmCopy == nil {
		t.Fatal("a date with dm_only celestial events must produce a DM copy")
	}

	if len(playerSafe.Events) != 1 || playerSafe.Events[0].ID != 11 {
		t.Fatalf("player-safe events = %+v, want only the everyone row (id 11)", playerSafe.Events)
	}
	for _, ev := range playerSafe.Events {
		if ev.Visibility == "dm_only" {
			t.Fatalf("dm_only event %q leaked into the player-safe payload", ev.Name)
		}
	}
	if len(dmCopy.Events) != 2 {
		t.Fatalf("DM events = %+v, want both rows", dmCopy.Events)
	}

	if playerSafe.Audience != WorldStateAudienceEveryone || dmCopy.Audience != WorldStateAudienceDM {
		t.Errorf("audience markers = %q / %q", playerSafe.Audience, dmCopy.Audience)
	}
	if playerSafe.Date != dmCopy.Date {
		t.Errorf("copies disagree on the date: %+v vs %+v", playerSafe.Date, dmCopy.Date)
	}
	if playerSafe.Weather != dmCopy.Weather {
		t.Errorf("copies disagree on the weather: %+v vs %+v", playerSafe.Weather, dmCopy.Weather)
	}
}

// TestBuildWorldStateChangePayloads_NoDMCopyWhenNothingIsHidden keeps the
// common case cheap: with no dm_only rows the two payloads would be identical,
// so sending both would double every world-state broadcast for no gain.
func TestBuildWorldStateChangePayloads_NoDMCopyWhenNothingIsHidden(t *testing.T) {
	cal := sampleSeedCalendar()

	t.Run("all public", func(t *testing.T) {
		celestials := []CelestialEvent{{ID: 1, Type: "meteor-shower", Name: "Tears", Visibility: "everyone"}}
		playerSafe, dmCopy := BuildWorldStateChangePayloads(cal, 1492, 4, 15, nil, celestials, nil)
		if dmCopy != nil {
			t.Fatal("no dm_only rows, but a DM copy was produced")
		}
		if len(playerSafe.Events) != 1 {
			t.Fatalf("public event was dropped: %+v", playerSafe.Events)
		}
	})

	t.Run("no celestial events at all", func(t *testing.T) {
		playerSafe, dmCopy := BuildWorldStateChangePayloads(cal, 1492, 4, 15, nil, nil, nil)
		if dmCopy != nil {
			t.Fatal("empty date produced a DM copy")
		}
		// Non-nil empty slice: `events: []` is parseable by a consumer that
		// iterates; `events: null` is a TypeError waiting to happen.
		raw, err := json.Marshal(playerSafe)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !json.Valid(raw) {
			t.Fatal("payload is not valid JSON")
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if string(m["events"]) != "[]" {
			t.Errorf("events = %s, want [] (never null)", m["events"])
		}
	})
}

// TestWorldStateChangePayload_StaysBackwardCompatible is the consumer pin.
// The Foundry module shipped formatWorldstateLine against the OLD
// {date:{year,month,day}, moodTint:{color,intensity}} shape while this event
// was still a dead letter (Chronicle-Foundry-Module PR #82). Enrichment is
// only safe if it is strictly additive at those exact paths — a rename here
// silently blanks the module's world-state line the moment it finally starts
// receiving the event it was written for.
func TestWorldStateChangePayload_StaysBackwardCompatible(t *testing.T) {
	cal := sampleSeedCalendar()
	color := "#112233"
	intensity := 0.4
	cal.MoodTintColor = &color
	cal.MoodTintIntensity = &intensity

	playerSafe, _ := BuildWorldStateChangePayloads(cal, 1492, 4, 15, &DayWeather{WeatherType: "rain"}, mixedCelestials(), nil)
	raw, err := json.Marshal(playerSafe)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got struct {
		Date struct {
			Year  int `json:"year"`
			Month int `json:"month"`
			Day   int `json:"day"`
		} `json:"date"`
		MoodTint struct {
			Color     *string `json:"color"`
			Intensity float64 `json:"intensity"`
		} `json:"moodTint"`
		Events []map[string]json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Date.Year != 1492 || got.Date.Month != 4 || got.Date.Day != 15 {
		t.Errorf("date = %+v, want 1492-4-15 at the same path the module reads", got.Date)
	}
	if got.MoodTint.Color == nil || *got.MoodTint.Color != color || got.MoodTint.Intensity != intensity {
		t.Errorf("moodTint = %+v, want the pre-existing {color,intensity} shape", got.MoodTint)
	}

	// The enrichment half: the fields that make the payload announceable.
	if len(got.Events) != 1 {
		t.Fatalf("want the one public celestial event, got %d", len(got.Events))
	}
	for _, key := range []string{"id", "type", "name", "start_time", "duration", "visibility"} {
		if _, ok := got.Events[0][key]; !ok {
			t.Errorf("celestial event missing %q — a consumer cannot dedupe or describe it", key)
		}
	}
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(raw, &topLevel); err != nil {
		t.Fatalf("unmarshal top level: %v", err)
	}
	for _, key := range []string{"audience", "date", "moodTint", "events", "moons", "weather"} {
		if _, ok := topLevel[key]; !ok {
			t.Errorf("payload missing top-level key %q", key)
		}
	}
}

// TestBuildWorldStateChangePayloads_CarriesMoonPhases pins the moon half: the
// broadcast reuses the seed's computed moon shape, so a consumer parses ONE
// moon shape whether it arrived by push or by GET /calendar/world-state.
func TestBuildWorldStateChangePayloads_CarriesMoonPhases(t *testing.T) {
	cal := sampleSeedCalendar()
	phases := map[int][]MoonPhaseVocab{
		1: {{MoonID: 1, Name: "The Silver Crown", StartPct: 0, EndPct: 100, Glyph: "🌕"}},
	}
	playerSafe, _ := BuildWorldStateChangePayloads(cal, 1492, 4, 15, nil, nil, phases)

	if len(playerSafe.Moons) != len(cal.Moons) {
		t.Fatalf("moons = %d, want %d", len(playerSafe.Moons), len(cal.Moons))
	}
	if playerSafe.Moons[0].NamedPhase != "The Silver Crown" {
		t.Errorf("authored moon vocab not applied: %q", playerSafe.Moons[0].NamedPhase)
	}
	// Same computation the seed uses — a divergence here would mean the push
	// and the refetch disagree about the sky.
	seed := AssembleWorldStateSeed(cal, 1492, 4, 15, nil, nil, phases, 1)
	if len(seed.Moons) != len(playerSafe.Moons) || seed.Moons[0].CyclePct != playerSafe.Moons[0].CyclePct {
		t.Error("broadcast and seed disagree on moon phase")
	}
}

// --- service-level emits ---

// worldStateEmitRepo wires the mock repo with everything publishWorldStateChange
// re-reads, so the emitted payload is the enriched one.
func worldStateEmitRepo(cal *Calendar, celestials []CelestialEvent) *mockCalendarRepo {
	moons := append([]Moon(nil), cal.Moons...)
	months := append([]Month(nil), cal.Months...)
	seasons := append([]Season(nil), cal.Seasons...)
	return &mockCalendarRepo{
		getByIDFn:     func(context.Context, string) (*Calendar, error) { return cal, nil },
		setMoodTintFn: func(context.Context, string, *string, *float64) error { return nil },
		updateFn:      func(context.Context, *Calendar) error { return nil },
		getMonthsFn:   func(context.Context, string) ([]Month, error) { return months, nil },
		getMoonsFn:    func(context.Context, string) ([]Moon, error) { return moons, nil },
		getSeasonsFn:  func(context.Context, string) ([]Season, error) { return seasons, nil },
		getCelestialEventsFn: func(context.Context, string, int, int, int) ([]CelestialEvent, error) {
			return celestials, nil
		},
	}
}

// TestSetWorldState_EmitsEnrichedSplit is the end-to-end service assertion:
// one write produces the player-safe broadcast plus — because the date carries
// a dm_only eclipse — the DM copy under the internal ".dm" name the adapter
// translates into a RequiresDM message.
func TestSetWorldState_EmitsEnrichedSplit(t *testing.T) {
	cal := sampleSeedCalendar()
	pub := &payloadRecordingPublisher{}
	svc := NewCalendarService(worldStateEmitRepo(cal, mixedCelestials()))
	svc.SetEventPublisher(pub)

	color := "#112233"
	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{
		Mood: &WorldStateMoodTint{Color: &color, Intensity: 0.4},
	}); err != nil {
		t.Fatalf("SetWorldState: %v", err)
	}

	public := pub.byType(EventWorldStateChanged)
	dm := pub.byType(EventWorldStateChangedDM)
	if len(public) != 1 {
		t.Fatalf("player-safe emits = %d, want 1", len(public))
	}
	if len(dm) != 1 {
		t.Fatalf("DM emits = %d, want 1 (the date has a dm_only eclipse)", len(dm))
	}

	pp, ok := public[0].(*WorldStateChangePayload)
	if !ok {
		t.Fatalf("player-safe payload type = %T, want *WorldStateChangePayload", public[0])
	}
	if len(pp.Events) != 1 || pp.Events[0].Visibility == "dm_only" {
		t.Fatalf("player-safe emit carried %+v", pp.Events)
	}
	dp, ok := dm[0].(*WorldStateChangePayload)
	if !ok {
		t.Fatalf("DM payload type = %T", dm[0])
	}
	if len(dp.Events) != 2 {
		t.Fatalf("DM emit carried %+v, want both events", dp.Events)
	}
}

// TestSetWorldState_NoDMEmitWhenNothingIsHidden — the everyday case emits
// exactly one message.
func TestSetWorldState_NoDMEmitWhenNothingIsHidden(t *testing.T) {
	cal := sampleSeedCalendar()
	pub := &payloadRecordingPublisher{}
	svc := NewCalendarService(worldStateEmitRepo(cal, []CelestialEvent{
		{ID: 1, Type: "meteor-shower", Name: "Tears", Visibility: "everyone"},
	}))
	svc.SetEventPublisher(pub)

	color := "#112233"
	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{
		Mood: &WorldStateMoodTint{Color: &color, Intensity: 0.4},
	}); err != nil {
		t.Fatalf("SetWorldState: %v", err)
	}
	if n := len(pub.byType(EventWorldStateChangedDM)); n != 0 {
		t.Fatalf("DM emits = %d, want 0", n)
	}
	if n := len(pub.byType(EventWorldStateChanged)); n != 1 {
		t.Fatalf("player-safe emits = %d, want 1", n)
	}
}

// TestSetWorldState_EnrichmentFailureStillBroadcasts is the anti-regression
// for the bug class this whole dispatch is about. If the celestial load fails,
// the change signal must still go out (thinner) — the write already
// succeeded, and a consumer that hears nothing has no way to learn it is
// stale. Degrading loudly beats dropping silently.
func TestSetWorldState_EnrichmentFailureStillBroadcasts(t *testing.T) {
	cal := sampleSeedCalendar()
	repo := worldStateEmitRepo(cal, nil)
	repo.getCelestialEventsFn = func(context.Context, string, int, int, int) ([]CelestialEvent, error) {
		return nil, errors.New("celestial table unavailable")
	}
	pub := &payloadRecordingPublisher{}
	svc := NewCalendarService(repo)
	svc.SetEventPublisher(pub)

	color := "#112233"
	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{
		Mood: &WorldStateMoodTint{Color: &color, Intensity: 0.4},
	}); err != nil {
		t.Fatalf("SetWorldState must not fail because enrichment did: %v", err)
	}
	sent := pub.byType(EventWorldStateChanged)
	if len(sent) != 1 {
		t.Fatalf("player-safe emits = %d, want 1 even on enrichment failure", len(sent))
	}
	p, ok := sent[0].(*WorldStateChangePayload)
	if !ok {
		t.Fatalf("payload type = %T", sent[0])
	}
	if p.Date.Year == 0 {
		t.Error("fallback payload lost the date — that is the one thing it must keep")
	}
	if p.Events == nil {
		t.Error("fallback payload must emit events: [] rather than null")
	}
	if n := len(pub.byType(EventWorldStateChangedDM)); n != 0 {
		t.Errorf("fallback must not emit a DM copy it cannot populate (got %d)", n)
	}
}
