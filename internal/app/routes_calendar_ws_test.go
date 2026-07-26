package app

import (
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// C-CAL-WORLDSTATE-WIRE regression pins.
//
// calendarEventPublisherAdapter ends in `default: return`. Two live emitters —
// worldstate_service.go's SetWorldState and service.go's SetWeatherZones —
// published event names that had no case, so every one of those messages was
// silently discarded INSIDE the publisher. Nothing logged, nothing broadcast:
// the operator's meteors and eclipses had never once reached a WebSocket
// client, which is why "celestial events unfindable/unsyncable" had no trace
// to follow.
//
// A `default: return` is a silent-drop machine by construction, so the whole
// mapping is pinned below rather than only the two names added here. Adding an
// emitter without a case must fail a test, not a play session.

// collectBus records every Publish so the two-message audience split is
// observable (captureBus in routes_test.go keeps only the last).
type collectBus struct {
	msgs []*ws.Message
}

func (b *collectBus) Publish(msg *ws.Message) { b.msgs = append(b.msgs, msg) }

// TestPublishCalendarEvent_RoutesEveryEmittedName is the full mapping pin,
// including the two names that were dead letters before this change.
func TestPublishCalendarEvent_RoutesEveryEmittedName(t *testing.T) {
	cases := []struct {
		event     string
		wantMsg   ws.MessageType
		wantReqDM bool
	}{
		{"event.created", ws.MsgCalendarEventCreated, false},
		{"event.updated", ws.MsgCalendarEventUpdated, false},
		{"event.deleted", ws.MsgCalendarEventDeleted, false},
		{"date.advanced", ws.MsgCalendarDateAdvanced, false},
		{"calendar.weather.changed", ws.MsgCalendarWeatherChanged, false},
		{"calendar.structure.updated", ws.MsgCalendarStructureUpdated, false},
		{"calendar.season.changed", ws.MsgCalendarSeasonChanged, false},
		{"calendar.era.changed", ws.MsgCalendarEraChanged, false},
		{"calendar.moon.phase_changed", ws.MsgCalendarMoonPhaseChanged, false},
		{"calendar.cycle.changed", ws.MsgCalendarCycleChanged, false},
		{"calendar.festival.changed", ws.MsgCalendarFestivalChanged, false},
		// Previously dropped by `default: return`.
		{calendar.EventWorldStateChanged, ws.MsgCalendarWorldstateChanged, false},
		{calendar.EventWorldStateChangedDM, ws.MsgCalendarWorldstateChanged, true},
		{"calendar.weather.zones.changed", ws.MsgCalendarWeatherZonesChanged, false},
	}

	for _, tc := range cases {
		t.Run(tc.event, func(t *testing.T) {
			bus := &captureBus{}
			a := &calendarEventPublisherAdapter{bus: bus}
			a.PublishCalendarEvent(tc.event, "camp-1", "cal-1", map[string]any{"k": "v"})

			if bus.last == nil {
				t.Fatalf("%q was dropped by the adapter (the default: return dead-letter regression)", tc.event)
			}
			if bus.last.Type != tc.wantMsg {
				t.Errorf("type = %q, want %q", bus.last.Type, tc.wantMsg)
			}
			if bus.last.RequiresDM != tc.wantReqDM {
				t.Errorf("RequiresDM = %v, want %v", bus.last.RequiresDM, tc.wantReqDM)
			}
			if bus.last.CampaignID != "camp-1" || bus.last.ResourceID != "cal-1" {
				t.Errorf("campaign/resource = %q/%q, want camp-1/cal-1", bus.last.CampaignID, bus.last.ResourceID)
			}
		})
	}
}

// TestPublishCalendarEvent_DMNameNeverCrossesTheWire pins that the internal
// ".dm" suffix is an ADAPTER-LOCAL routing token. A client that saw
// "calendar.worldstate.changed.dm" as a message type would have to learn two
// type strings for one concept — and would silently ignore the richer copy.
func TestPublishCalendarEvent_DMNameNeverCrossesTheWire(t *testing.T) {
	bus := &captureBus{}
	a := &calendarEventPublisherAdapter{bus: bus}
	a.PublishCalendarEvent(calendar.EventWorldStateChangedDM, "camp-1", "cal-1", nil)

	if bus.last == nil {
		t.Fatal("DM world-state copy was dropped")
	}
	if got := string(bus.last.Type); got != string(ws.MsgCalendarWorldstateChanged) {
		t.Fatalf("public type = %q, want %q", got, ws.MsgCalendarWorldstateChanged)
	}
	if !bus.last.RequiresDM {
		t.Fatal("DM copy must carry RequiresDM — without it the hub delivers dm_only celestial events to every player")
	}
}

// TestPublishCalendarEvent_UnknownNameStillDropped keeps the default branch
// honest in the other direction: this adapter is the campaign-scoped calendar
// bridge, and an unrecognised name must not be guessed into some MessageType.
func TestPublishCalendarEvent_UnknownNameStillDropped(t *testing.T) {
	bus := &captureBus{}
	a := &calendarEventPublisherAdapter{bus: bus}
	a.PublishCalendarEvent("calendar.something.invented", "camp-1", "cal-1", nil)
	if bus.last != nil {
		t.Fatalf("unknown event name published as %q", bus.last.Type)
	}

	// Empty campaign id is also a hard drop — a campaign-less broadcast would
	// fan out to the wrong room or none at all.
	a.PublishCalendarEvent(calendar.EventWorldStateChanged, "", "cal-1", nil)
	if bus.last != nil {
		t.Fatalf("campaign-less event published as %q", bus.last.Type)
	}
}

// TestPublishCalendarEvent_WorldStateSplitOnTheBus is the end-to-end shape of
// the audience split as a subscriber sees it: two messages of the SAME public
// type, distinguished only by RequiresDM and by the `audience` marker in the
// payload, with the DM copy carrying the strict superset of celestial events.
func TestPublishCalendarEvent_WorldStateSplitOnTheBus(t *testing.T) {
	bus := &collectBus{}
	a := &calendarEventPublisherAdapter{bus: bus}

	playerSafe := &calendar.WorldStateChangePayload{
		Audience: calendar.WorldStateAudienceEveryone,
		Date:     calendar.WorldStateDate{Year: 1492, Month: 4, Day: 15},
		Events: []calendar.WorldStateEvent{
			{ID: 1, Type: "meteor-shower", Name: "Tears of Selune", Visibility: "everyone"},
		},
	}
	dmCopy := &calendar.WorldStateChangePayload{
		Audience: calendar.WorldStateAudienceDM,
		Date:     calendar.WorldStateDate{Year: 1492, Month: 4, Day: 15},
		Events: []calendar.WorldStateEvent{
			{ID: 1, Type: "meteor-shower", Name: "Tears of Selune", Visibility: "everyone"},
			{ID: 2, Type: "eclipse", Name: "The Long Dark", Visibility: "dm_only"},
		},
	}

	a.PublishCalendarEvent(calendar.EventWorldStateChanged, "camp-1", "cal-1", playerSafe)
	a.PublishCalendarEvent(calendar.EventWorldStateChangedDM, "camp-1", "cal-1", dmCopy)

	if len(bus.msgs) != 2 {
		t.Fatalf("published %d messages, want 2", len(bus.msgs))
	}
	if bus.msgs[0].RequiresDM {
		t.Error("player-safe copy must NOT set RequiresDM (the hub would then hide it from players)")
	}
	if !bus.msgs[1].RequiresDM {
		t.Error("DM copy must set RequiresDM")
	}

	var got calendar.WorldStateChangePayload
	if err := json.Unmarshal(bus.msgs[0].Payload, &got); err != nil {
		t.Fatalf("player-safe payload did not decode: %v", err)
	}
	for _, ev := range got.Events {
		if ev.Visibility == "dm_only" {
			t.Fatalf("dm_only celestial event %q rode the un-flagged broadcast", ev.Name)
		}
	}
	if got.Audience != calendar.WorldStateAudienceEveryone {
		t.Errorf("audience marker = %q, want %q", got.Audience, calendar.WorldStateAudienceEveryone)
	}
}
