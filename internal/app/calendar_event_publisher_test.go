package app

import (
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

type calendarCaptureBus struct{ msgs []*ws.Message }

func (b *calendarCaptureBus) Publish(m *ws.Message) { b.msgs = append(b.msgs, m) }

// TestCalendarPublisher_EveryEmitterTypeMaps walks every event type the
// calendar can publish; a new one without a wire mapping fails here rather
// than publishing into nothing.
func TestCalendarPublisher_EveryEmitterTypeMaps(t *testing.T) {
	for _, et := range calendar.AllPublishedEventTypes() {
		t.Run(et, func(t *testing.T) {
			want, ok := calendarWireTypes[et]
			if !ok || want == "" {
				t.Fatalf("calendar event %q has no WebSocket mapping", et)
			}
			bus := &calendarCaptureBus{}
			(&calendarEventPublisherAdapter{bus: bus}).PublishCalendarEvent(et, "camp-1", "cal-1", nil)
			if len(bus.msgs) != 1 || bus.msgs[0].Type != want {
				t.Fatalf("published %v, want one %q", bus.msgs, want)
			}
			if bus.msgs[0].CampaignID != "camp-1" {
				t.Errorf("campaign = %q", bus.msgs[0].CampaignID)
			}
		})
	}
	if len(calendarWireTypes) != len(calendar.AllPublishedEventTypes()) {
		t.Errorf("mapping has %d entries for %d emitter types", len(calendarWireTypes), len(calendar.AllPublishedEventTypes()))
	}
}

// TestCalendarPublisher_NeverReachesPlayers: the hub drops a RequiresDM
// message for any socket where permissions.CanSeeDmOnly is false (hub.go
// broadcast loop). Every calendar message must carry the flag, so a dm_only
// event, an unannounced one or a hidden moon cannot reach a player socket.
func TestCalendarPublisher_NeverReachesPlayers(t *testing.T) {
	recipients := []struct {
		name        string
		role        int
		isDmGranted bool
		want        bool
	}{
		{"owner (Foundry GM key)", permissions.RoleOwner, false, true},
		{"dm-granted member", permissions.RolePlayer, true, true},
		{"player", permissions.RolePlayer, false, false},
		{"scribe", permissions.RoleScribe, false, false},
	}
	dmOnly := &calendar.Event{ID: "ev-1", Name: "secret", Visibility: "dm_only"}
	for _, et := range calendar.AllPublishedEventTypes() {
		bus := &calendarCaptureBus{}
		var payload any
		if et == calendar.PubEventCreated || et == calendar.PubEventUpdated {
			payload = dmOnly
		}
		(&calendarEventPublisherAdapter{bus: bus}).PublishCalendarEvent(et, "camp-1", "cal-1", payload)
		msg := bus.msgs[0]
		if !msg.RequiresDM {
			t.Fatalf("%s is not RequiresDM", et)
		}
		for _, r := range recipients {
			// Mirrors the hub's delivery predicate.
			delivered := !msg.RequiresDM || permissions.CanSeeDmOnly(r.role, r.isDmGranted)
			if delivered != r.want {
				t.Errorf("%s -> %s: delivered=%v want %v", et, r.name, delivered, r.want)
			}
		}
	}
}

// TestCalendarPublisher_PayloadShapes pins resource ids and the gm-only wire
// visibility, and that the caller's event is not mutated.
func TestCalendarPublisher_PayloadShapes(t *testing.T) {
	bus := &calendarCaptureBus{}
	a := &calendarEventPublisherAdapter{bus: bus}
	evt := &calendar.Event{ID: "ev-1", Name: "n", Visibility: "dm_only"}
	a.PublishCalendarEvent(calendar.PubEventCreated, "camp-1", "cal-1", evt)
	a.PublishCalendarEvent(calendar.PubEventDeleted, "camp-1", "cal-1", map[string]string{"id": "ev-2"})
	a.PublishCalendarEvent(calendar.PubDateAdvanced, "camp-1", "cal-1", calendar.DatePayload{Year: 5, Month: 2, Day: 3})
	a.PublishCalendarEvent(calendar.PubEventCreated, "", "cal-1", evt)
	a.PublishCalendarEvent("nonsense", "camp-1", "cal-1", nil)
	if len(bus.msgs) != 3 {
		t.Fatalf("got %d messages, want 3 (empty campaign and unknown type are dropped)", len(bus.msgs))
	}
	if bus.msgs[0].ResourceID != "ev-1" || bus.msgs[1].ResourceID != "ev-2" || bus.msgs[2].ResourceID != "cal-1" {
		t.Errorf("resource ids = %q %q %q", bus.msgs[0].ResourceID, bus.msgs[1].ResourceID, bus.msgs[2].ResourceID)
	}
	var got map[string]any
	if err := json.Unmarshal(bus.msgs[0].Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got["visibility"] != "gm-only" {
		t.Errorf("wire visibility = %v, want gm-only", got["visibility"])
	}
	if evt.Visibility != "dm_only" {
		t.Error("the caller's event was mutated")
	}
	var date map[string]int
	if err := json.Unmarshal(bus.msgs[2].Payload, &date); err != nil || date["year"] != 5 || date["month"] != 2 || date["day"] != 3 {
		t.Errorf("date payload = %s", bus.msgs[2].Payload)
	}
}
