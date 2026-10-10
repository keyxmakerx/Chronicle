package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// sliceBus records every message, unlike captureBus which keeps only the last,
// because a write now publishes the items-changed notice and the item event.
type sliceBus struct{ msgs []*ws.Message }

func (b *sliceBus) Publish(m *ws.Message) { b.msgs = append(b.msgs, m) }

func (b *sliceBus) itemsChanged() []*ws.Message {
	var out []*ws.Message
	for _, m := range b.msgs {
		if m.Type == ws.MsgMapItemsChanged {
			out = append(out, m)
		}
	}
	return out
}

// secret is planted in every field a write could leak; none of it may appear
// in the notice, because every client of the campaign receives it.
const itemsSecret = "SECRET-DM-CONTENT"

// TestMapItemsChanged_EveryWritePublishesIdOnlyNotice pins that each create,
// update and delete of a pin, drawing, shadow, picture and token announces the
// right kind to the whole campaign, and that the notice is id-only.
func TestMapItemsChanged_EveryWritePublishesIdOnlyNotice(t *testing.T) {
	secret := itemsSecret
	marker := &maps.Marker{ID: "m1", MapID: "map-1", Name: itemsSecret, Description: &secret, Visibility: "dm_only"}
	token := &maps.Token{ID: "t1", MapID: "map-1", Name: itemsSecret, IsHidden: true}
	drawing := func(typ string) *maps.Drawing {
		return &maps.Drawing{ID: "d1", MapID: "map-1", DrawingType: typ, Visibility: "dm_only"}
	}
	events := []string{"created", "updated", "deleted"}

	type write struct {
		name     string
		wantKind string
		do       func(a *mapEventPublisherAdapter, event string)
	}
	writes := []write{
		{"marker", "markers", func(a *mapEventPublisherAdapter, e string) { a.PublishMarkerEvent(e, "camp-1", marker) }},
		{"drawing", "drawings", func(a *mapEventPublisherAdapter, e string) {
			a.PublishDrawingEvent(e, "camp-1", drawing("freehand"))
		}},
		{"picture", "drawings", func(a *mapEventPublisherAdapter, e string) {
			a.PublishDrawingEvent(e, "camp-1", drawing(maps.DrawingTypeImage))
		}},
		{"shadow", "shadows", func(a *mapEventPublisherAdapter, e string) {
			a.PublishDrawingEvent(e, "camp-1", drawing(maps.DrawingTypeShadow))
		}},
		{"token", "tokens", func(a *mapEventPublisherAdapter, e string) { a.PublishTokenEvent(e, "camp-1", token) }},
	}
	for _, w := range writes {
		for _, e := range events {
			t.Run(w.name+" "+e, func(t *testing.T) {
				bus := &sliceBus{}
				a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{}}
				w.do(a, e)
				assertIdOnlyNotice(t, bus.itemsChanged(), w.wantKind)
			})
		}
	}

	t.Run("token drag", func(t *testing.T) {
		bus := &sliceBus{}
		a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{}}
		a.PublishTokenPositionEvent("camp-1", "map-1", "t1", 10, 20, true)
		assertIdOnlyNotice(t, bus.itemsChanged(), "tokens")
	})
}

func assertIdOnlyNotice(t *testing.T, got []*ws.Message, wantKind string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("want exactly one items-changed notice, got %d", len(got))
	}
	m := got[0]
	if m.CampaignID != "camp-1" || m.ResourceID != "map-1" {
		t.Errorf("notice addressed to campaign %q resource %q", m.CampaignID, m.ResourceID)
	}
	// Every client must get it: audience gating would mean a player's page
	// never learns that it should refetch.
	if m.RequiresDM || m.StrictAudience || len(m.AllowedUsers) != 0 || len(m.DeniedUsers) != 0 {
		t.Errorf("notice must not be audience-gated: %+v", m)
	}
	var payload map[string]any
	if err := json.Unmarshal(m.Payload, &payload); err != nil {
		t.Fatalf("payload is not an object: %v", err)
	}
	if len(payload) != 2 || payload["map_id"] != "map-1" || payload["kind"] != wantKind {
		t.Errorf("payload = %v, want only map_id and kind=%q", payload, wantKind)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), itemsSecret) {
		t.Errorf("notice leaks item content: %s", raw)
	}
}

// TestMapItemsChanged_NotSentWhenNothingToAddress covers the guards: an unknown
// lifecycle word, a missing campaign and a missing bus announce nothing.
func TestMapItemsChanged_NotSentWhenNothingToAddress(t *testing.T) {
	cases := []struct {
		name string
		run  func(a *mapEventPublisherAdapter)
	}{
		{"unknown event", func(a *mapEventPublisherAdapter) {
			a.PublishMarkerEvent("gibberish", "camp-1", &maps.Marker{ID: "m", MapID: "map-1"})
		}},
		{"no campaign", func(a *mapEventPublisherAdapter) {
			a.PublishMarkerEvent("created", "", &maps.Marker{ID: "m", MapID: "map-1"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := &sliceBus{}
			tc.run(&mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{}})
			if n := len(bus.itemsChanged()); n != 0 {
				t.Errorf("want no notice, got %d", n)
			}
		})
	}
	// A nil bus (tests, or a deployment without websockets) must not panic.
	(&mapEventPublisherAdapter{}).publishItemsChanged("camp-1", "map-1", "markers")
}
