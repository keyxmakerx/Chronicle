// sky_event_test.go: table-driven tests for the sky event payload: what a
// write accepts and refuses, the moons it may name, and how an import
// carries one over.
package calendar

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

func TestParseSkyEventPayload(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		sky     bool   // a sky event comes back
		wantErr string // "" when accepted; else a piece of the message
	}{
		{"moon night as stored today", `{"type":"blood","moons":[3]}`, true, ""},
		{"comet with every field it reads", `{"type":"comet","direction":{"compass":"W","altitude":"low"},"color":"#ffd9a0","intensity":0.8,"tail_degrees":30}`, true, ""},
		{"meteor storm", `{"type":"meteor-storm","rate_per_hour":2000,"moonlit":false,"color":"#FFFFFF"}`, true, ""},
		{"aurora with two colours", `{"type":"aurora","colors":["#33ff99","#aa66ff"],"intensity":0}`, true, ""},
		{"partial solar eclipse", `{"type":"eclipse-solar","moons":[1],"subtype":"partial","magnitude":0.98,"duration_minutes":600}`, true, ""},
		{"custom doorway", `{"type":"custom","shape":"doorway","size":"large","duration_hours":0.25}`, true, ""},
		{"just a type", `{"type":"dark-night"}`, true, ""},
		{"a field the type does not read is kept", `{"type":"dark-night","shape":"veil"}`, true, ""},
		{"null fields read as absent", `{"type":"comet","color":null,"intensity":null}`, true, ""},

		{"another feature's shape", `{"type":"generated","lane":"sky","sig":"abc"}`, false, ""},
		{"type in the wrong case is not a sky type", `{"type":"Comet","whatever":1}`, false, ""},
		{"no type", `{"moons":[1]}`, false, ""},
		{"a number type", `{"type":3}`, false, ""},
		{"an array", `[1,2]`, false, ""},
		{"empty", ``, false, ""},

		{"bad colour", `{"type":"comet","color":"orange"}`, false, "color must be a colour written like #ffd9a0"},
		{"short hex colour", `{"type":"comet","color":"#fff"}`, false, "color must be"},
		{"bad colour in colors", `{"type":"aurora","colors":["#33ff99","green"]}`, false, `colors: "green" must be`},
		{"three colours", `{"type":"aurora","colors":["#000000","#111111","#222222"]}`, false, "colors lists at most 2"},
		{"nine moons", `{"type":"conjunction","moons":[1,2,3,4,5,6,7,8,9]}`, false, "moons lists at most 8"},
		{"unknown field", `{"type":"comet","colour":"#ffd9a0"}`, false, `"colour" is not a sky event field`},
		{"unknown field in direction", `{"type":"comet","direction":{"compass":"W","height":"low"}}`, false, `"height" is not a sky event field`},
		{"lower-case compass", `{"type":"comet","direction":{"compass":"w"}}`, false, "direction compass must be one of N, NE"},
		{"bad altitude", `{"type":"comet","direction":{"altitude":"overhead"}}`, false, "direction altitude must be low, mid or high"},
		{"direction as a word", `{"type":"comet","direction":"west"}`, false, "direction must be a compass point and an altitude"},
		{"intensity above 1", `{"type":"aurora","intensity":1.5}`, false, "intensity must be a number from 0 to 1"},
		{"intensity as text", `{"type":"aurora","intensity":"bright"}`, false, "intensity must be a number from 0 to 1"},
		{"rate 0", `{"type":"meteor-shower","rate_per_hour":0}`, false, "rate_per_hour must be a whole number from 1 to 2000"},
		{"rate not whole", `{"type":"meteor-shower","rate_per_hour":2.5}`, false, "rate_per_hour must be a whole number"},
		{"moonlit as text", `{"type":"meteor-shower","moonlit":"yes"}`, false, "moonlit must be true or false"},
		{"tail 0", `{"type":"comet","tail_degrees":0}`, false, "tail_degrees must be a number from 1 to 90"},
		{"magnitude 1", `{"type":"eclipse-solar","magnitude":1}`, false, "magnitude must be a number from 0.1 to 0.98"},
		{"totality 601 minutes", `{"type":"eclipse-solar","duration_minutes":601}`, false, "duration_minutes must be a whole number from 1 to 600"},
		{"custom for 25 hours", `{"type":"custom","duration_hours":25}`, false, "duration_hours must be a number from 0.25 to 24"},
		{"bad subtype", `{"type":"eclipse-solar","subtype":"hybrid"}`, false, "subtype must be total, annular or partial"},
		{"bad shape", `{"type":"custom","shape":"dragon"}`, false, "shape must be one of streak"},
		{"bad size", `{"type":"custom","size":"huge"}`, false, "size must be small, medium or large"},
		{"moons as names", `{"type":"blood","moons":["Selune"]}`, false, "moons must be a list of moons"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseSkyEventPayload(tt.raw)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("accepted %s; want an error containing %q", tt.raw, tt.wantErr)
				}
				wantValidationErr(t, err, tt.name)
				msg := apperror.SafeMessage(err)
				if !strings.HasPrefix(msg, "sky event: ") || !strings.Contains(msg, tt.wantErr) {
					t.Errorf("message %q, want it to start \"sky event: \" and contain %q", msg, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused %s: %v", tt.raw, err)
			}
			if (p != nil) != tt.sky {
				t.Errorf("sky event = %+v, want one: %v", p, tt.sky)
			}
		})
	}
}

func TestValidEventPayload(t *testing.T) {
	s := func(v string) *string { return &v }
	tests := []struct {
		name    string
		payload *string
		ok      bool
	}{
		{"none", nil, true},
		{"empty", s(""), true},
		{"another feature's shape", s(`{"type":"generated","sig":"x","anything":[1,2]}`), true},
		{"a sky event", s(`{"type":"comet","color":"#ffd9a0"}`), true},
		{"not JSON", s(`{"type":`), false},
		{"a bad sky event", s(`{"type":"comet","color":"red"}`), false},
		{"too long", s(`{"type":"generated","pad":"` + strings.Repeat("x", maxEventPayloadBytes) + `"}`), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validEventPayload(tt.payload)
			if tt.ok && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !tt.ok {
				wantValidationErr(t, err, tt.name)
			}
		})
	}
}

// skyFixture is eventDatesFixture's service with one moon, id 7, on the
// calendar, and the stored event's payload set to stored.
func skyFixture(t *testing.T, stored *string) (CalendarService, *int, **Event) {
	t.Helper()
	svc, writes := eventDatesFixture(t)
	cs := svc.(*calendarService)
	cs.calRepo.(*fakeCalendarRepo).getMoonsFn = func(_ context.Context, _ string) ([]Moon, error) {
		return []Moon{{ID: 7, Name: "Selune"}}, nil
	}
	var saved *Event
	repo := cs.eventRepo.(*fakeEventRepo)
	repo.getEventFn = func(_ context.Context, _ string) (*Event, error) {
		return &Event{ID: "evt-1", CalendarID: "cal-1", Name: "Feast", Year: 1000, Month: 1, Day: 10, Visibility: "everyone", Payload: stored}, nil
	}
	repo.createEventFn = func(_ context.Context, e *Event) error { *writes++; saved = e; return nil }
	repo.updateEventFn = func(_ context.Context, e *Event) error { *writes++; saved = e; return nil }
	return svc, writes, &saved
}

func TestCreateEvent_SkyEvent(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		ok      bool
	}{
		{"a moon of this calendar", `{"type":"blood","moons":[7]}`, true},
		{"no moons", `{"type":"comet","direction":{"compass":"W"}}`, true},
		{"another feature's shape naming moons", `{"type":"generated","moons":[99]}`, true},
		{"a moon of another calendar", `{"type":"blood","moons":[7,99]}`, false},
		{"bad colour", `{"type":"comet","color":"#ffd9a"}`, false},
		{"unknown field", `{"type":"comet","speed":3}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, writes, _ := skyFixture(t, nil)
			p := tt.payload
			in := CreateEventInput{Name: "Feast", Year: 1000, Month: 1, Day: 10, Author: ownerViewer("u-owner"), Payload: &p}
			_, err := svc.CreateEvent(context.Background(), "cal-1", testCampaignA, in)
			if tt.ok {
				if err != nil {
					t.Fatalf("CreateEvent: %v", err)
				}
				return
			}
			wantValidationErr(t, err, tt.name)
			if *writes != 0 {
				t.Errorf("a refused event must not be stored; got %d writes", *writes)
			}
		})
	}
}

// TestUpdateEvent_SkyEventPartial pins the partial-update contract for
// payload: absent keeps the stored one without judging it again, null
// clears, and a sent one is checked like on create.
func TestUpdateEvent_SkyEventPartial(t *testing.T) {
	s := func(v string) *string { return &v }
	// A stored payload a newer rule would refuse (an unknown field).
	oldStored := s(`{"type":"blood","moons":[7],"note":"from an old file"}`)
	tests := []struct {
		name   string
		stored *string
		in     UpdateEventInput
		ok     bool
		want   *string // the payload saved, when ok
	}{
		{"absent keeps an old payload unjudged", oldStored, UpdateEventInput{Name: patch.Of("Harvest")}, true, oldStored},
		{"null clears", oldStored, UpdateEventInput{Payload: patch.Null[string]()}, true, nil},
		{"a new sky event", nil, UpdateEventInput{Payload: patch.Of(`{"type":"hunters-moon","moons":[7],"color":"#ff8844"}`)}, true,
			s(`{"type":"hunters-moon","moons":[7],"color":"#ff8844"}`)},
		{"a moon of another calendar", nil, UpdateEventInput{Payload: patch.Of(`{"type":"blue-moon","moons":[8]}`)}, false, nil},
		{"bad type of value", nil, UpdateEventInput{Payload: patch.Of(`{"type":"comet","intensity":"high"}`)}, false, nil},
		{"out of range", nil, UpdateEventInput{Payload: patch.Of(`{"type":"meteor-shower","rate_per_hour":5000}`)}, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, writes, saved := skyFixture(t, tt.stored)
			err := svc.UpdateEvent(context.Background(), "evt-1", "cal-1", testCampaignA, tt.in, ownerViewer("u-owner"))
			if !tt.ok {
				wantValidationErr(t, err, tt.name)
				if *writes != 0 {
					t.Errorf("a refused update must not be stored; got %d writes", *writes)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateEvent: %v", err)
			}
			got := (*saved).Payload
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("saved payload = %v, want %v", deref(got), deref(tt.want))
			}
		})
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestImportedEventPayload(t *testing.T) {
	s := func(v string) *string { return &v }
	moons := map[int]int{3: 30, 4: 40}
	tests := []struct {
		name     string
		in       *string
		want     *string
		wantNote string // a piece of the note; "" for none
	}{
		{"none", nil, nil, ""},
		{"another feature's shape is untouched", s(`{"type":"generated","moons":[3]}`), s(`{"type":"generated","moons":[3]}`), ""},
		{"no moons is untouched", s(`{"type":"comet","color":"#ffd9a0"}`), s(`{"type":"comet","color":"#ffd9a0"}`), ""},
		{"moons mapped", s(`{"type":"conjunction","moons":[3,4]}`), s(`{"type":"conjunction","moons":[30,40]}`), ""},
		{"an unmatched moon is left off", s(`{"type":"conjunction","moons":[3,9]}`), s(`{"type":"conjunction","moons":[30]}`), "1 of its sky event's moons"},
		{"a bad sky event is left off whole", s(`{"type":"comet","color":"red"}`), nil, "its sky event was left off (sky event: color"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, note := ImportedEventPayload(tt.in, moons)
			if deref(got) != deref(tt.want) {
				t.Errorf("payload = %s, want %s", deref(got), deref(tt.want))
			}
			if (tt.wantNote == "") != (note == "") || !strings.Contains(note, tt.wantNote) {
				t.Errorf("note = %q, want one containing %q", note, tt.wantNote)
			}
		})
	}
}

// TestChronicleExportImport_SkyEventRoundTrips pins that a sky event comes
// back from a Chronicle export byte for byte, with the moon refs an import
// needs to point it at the re-created moons.
func TestChronicleExportImport_SkyEventRoundTrips(t *testing.T) {
	payload := `{"type":"eclipse-solar","moons":[12],"subtype":"total","duration_minutes":4}`
	cal := &Calendar{
		Name: "Sky Calendar", Mode: ModeFantasy, CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
		Months: []Month{{Name: "Only", Days: 30}},
		Moons:  []Moon{{ID: 12, Name: "Selune", CycleDays: 30}},
	}
	events := []Event{{Name: "Dark noon", Year: 1, Month: 1, Day: 5, Visibility: "everyone", Payload: &payload}}
	raw, err := json.Marshal(BuildExport(cal, events, true))
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if len(ir.Events) != 1 || ir.Events[0].Payload == nil || *ir.Events[0].Payload != payload {
		t.Fatalf("imported payload = %+v, want %s", ir.Events, payload)
	}
	if len(ir.MoonRefs) != 1 || ir.MoonRefs[0] != 12 {
		t.Fatalf("MoonRefs = %v, want [12]", ir.MoonRefs)
	}
	got, note := ImportedEventPayload(ir.Events[0].Payload, map[int]int{12: 55})
	if note != "" {
		t.Errorf("note = %q, want none", note)
	}
	p, err := ParseSkyEventPayload(deref(got))
	if err != nil || p == nil || len(p.Moons) != 1 || p.Moons[0] != 55 || p.Subtype != "total" || *p.DurationMinutes != 4 {
		t.Errorf("remapped = %s (%v), want moon 55 and the rest kept", deref(got), err)
	}
}

// TestSkyEventKeepsMoonNightReaders pins that the older moon-night reader
// still reads a moon night and leaves the new types to SkyEvent.
func TestSkyEventKeepsMoonNightReaders(t *testing.T) {
	night := `{"type":"harvest","moons":[2]}`
	comet := `{"type":"comet","color":"#ffd9a0"}`
	e := Event{Payload: &night}
	if mn := e.ParsePayload(); mn == nil || mn.Type != MoonNightHarvest || len(mn.Moons) != 1 {
		t.Errorf("ParsePayload(moon night) = %+v", mn)
	}
	if sk := e.SkyEvent(); sk == nil || sk.Type != MoonNightHarvest {
		t.Errorf("SkyEvent(moon night) = %+v", sk)
	}
	e.Payload = &comet
	if mn := e.ParsePayload(); mn != nil {
		t.Errorf("ParsePayload(comet) = %+v, want nil", mn)
	}
	if sk := e.SkyEvent(); sk == nil || sk.Color != "#ffd9a0" {
		t.Errorf("SkyEvent(comet) = %+v", sk)
	}
}
