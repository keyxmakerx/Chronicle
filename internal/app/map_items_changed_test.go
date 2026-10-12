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
// update and delete of a pin, drawing, shadow and picture announces the right
// kind, that the notice is id-only, and who receives it: creating or deleting
// a DM-only item is DM-only, while public items, updates and shadows are open.
func TestMapItemsChanged_EveryWritePublishesIdOnlyNotice(t *testing.T) {
	secret := itemsSecret
	marker := func(vis string) *maps.Marker {
		return &maps.Marker{ID: "m1", MapID: "map-1", Name: itemsSecret, Description: &secret, Visibility: vis}
	}
	drawing := func(typ, vis string) *maps.Drawing {
		return &maps.Drawing{ID: "d1", MapID: "map-1", DrawingType: typ, Visibility: vis}
	}
	type write struct {
		name     string
		wantKind string
		do       func(a *mapEventPublisherAdapter, event string)
		// dmOnlyOn lists the events whose notice must be DM-only.
		dmOnlyOn []string
	}
	both := []string{"created", "deleted"}
	writes := []write{
		{"dm-only marker", "markers", func(a *mapEventPublisherAdapter, e string) { a.PublishMarkerEvent(e, "camp-1", marker("dm_only")) }, both},
		{"public marker", "markers", func(a *mapEventPublisherAdapter, e string) { a.PublishMarkerEvent(e, "camp-1", marker("everyone")) }, nil},
		{"dm-only drawing", "drawings", func(a *mapEventPublisherAdapter, e string) {
			a.PublishDrawingEvent(e, "camp-1", drawing("freehand", "dm_only"))
		}, both},
		{"public drawing", "drawings", func(a *mapEventPublisherAdapter, e string) {
			a.PublishDrawingEvent(e, "camp-1", drawing("freehand", "everyone"))
		}, nil},
		{"dm-only picture", "drawings", func(a *mapEventPublisherAdapter, e string) {
			a.PublishDrawingEvent(e, "camp-1", drawing(maps.DrawingTypeImage, "dm_only"))
		}, both},
		{"public picture", "drawings", func(a *mapEventPublisherAdapter, e string) {
			a.PublishDrawingEvent(e, "camp-1", drawing(maps.DrawingTypeImage, "everyone"))
		}, nil},
		// A shadow changes what players see, so its notice is always open.
		{"shadow", "shadows", func(a *mapEventPublisherAdapter, e string) {
			a.PublishDrawingEvent(e, "camp-1", drawing(maps.DrawingTypeShadow, "everyone"))
		}, nil},
	}
	for _, w := range writes {
		for _, e := range []string{"created", "updated", "deleted"} {
			t.Run(w.name+" "+e, func(t *testing.T) {
				bus := &sliceBus{}
				a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{}}
				w.do(a, e)
				wantDM := false
				for _, d := range w.dmOnlyOn {
					wantDM = wantDM || d == e
				}
				assertIdOnlyNotice(t, bus.itemsChanged(), w.wantKind, wantDM)
			})
		}
	}
}

// Tokens have no layer on the web viewer and a notice per hidden drag would
// leak movement, so no token write may announce anything.
func TestMapItemsChanged_TokensNeverAnnounce(t *testing.T) {
	token := &maps.Token{ID: "t1", MapID: "map-1", Name: itemsSecret, IsHidden: true}
	for _, e := range []string{"created", "updated", "deleted"} {
		t.Run(e, func(t *testing.T) {
			bus := &sliceBus{}
			a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{}}
			a.PublishTokenEvent(e, "camp-1", token)
			if n := len(bus.itemsChanged()); n != 0 {
				t.Errorf("want no notice, got %d", n)
			}
		})
	}
	t.Run("drag", func(t *testing.T) {
		bus := &sliceBus{}
		a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{}}
		a.PublishTokenPositionEvent("camp-1", "map-1", "t1", 10, 20, true)
		if n := len(bus.itemsChanged()); n != 0 {
			t.Errorf("want no notice, got %d", n)
		}
	})
}

func assertIdOnlyNotice(t *testing.T, got []*ws.Message, wantKind string, wantDM bool) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("want exactly one items-changed notice, got %d", len(got))
	}
	m := got[0]
	if m.CampaignID != "camp-1" || m.ResourceID != "map-1" {
		t.Errorf("notice addressed to campaign %q resource %q", m.CampaignID, m.ResourceID)
	}
	if m.RequiresDM != wantDM || m.StrictAudience {
		t.Errorf("RequiresDM = %v (strict %v), want %v", m.RequiresDM, m.StrictAudience, wantDM)
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
	(&mapEventPublisherAdapter{}).publishItemsChanged("camp-1", "map-1", "markers", false, nil)
}
