package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/cutscenes"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

type cutsceneBus struct{ got []*ws.Message }

func (b *cutsceneBus) Publish(m *ws.Message) { b.got = append(b.got, m) }

// Each showing message goes to exactly one kind of socket and audience: the
// chosen people's browser tabs, strictly, and the Foundry module's DM-held
// socket with the viewer list. A message that reached the other kind would
// show the scene twice, or show it to someone not picked.
func TestCutsceneLivePublisherAudiences(t *testing.T) {
	bus := &cutsceneBus{}
	p := &cutsceneLivePublisher{bus: bus}
	until := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	sh := cutscenes.Showing{ID: "sh-1", Scene: cutscenes.Scene{ID: "s-1", Name: "The Eye Opens"}, RewatchUntil: until,
		Viewers: []cutscenes.ShowingViewer{{UserID: "ana"}, {UserID: "bo"}}}
	p.Show("camp-1", sh)
	p.Stop("camp-1", sh)

	browser := []string{ws.BrowserSource}
	module := []string{foundry_vtt.ModuleSource}
	viewers := []string{"ana", "bo"}
	tests := []struct {
		typ      ws.MessageType
		allowed  []string
		strict   bool
		dm       bool
		sources  []string
		payloads []string
	}{
		{ws.MsgCutsceneShow, viewers, true, false, browser, []string{"showingId", "scene"}},
		{ws.MsgCutsceneFoundry, nil, false, true, module, []string{"showingId", "viewers", "scene", "rewatchUntil"}},
		{ws.MsgCutsceneStop, viewers, true, false, browser, []string{"showingId"}},
		{ws.MsgCutsceneFoundryStop, nil, false, true, module, []string{"showingId"}},
	}
	if len(bus.got) != len(tests) {
		t.Fatalf("published %d messages, want %d", len(bus.got), len(tests))
	}
	for i, tt := range tests {
		m := bus.got[i]
		t.Run(string(tt.typ), func(t *testing.T) {
			if m.Type != tt.typ || m.CampaignID != "camp-1" || m.ResourceID != "sh-1" {
				t.Fatalf("message %d = %s %s %s", i, m.Type, m.CampaignID, m.ResourceID)
			}
			if !reflect.DeepEqual(m.AllowedUsers, tt.allowed) || m.StrictAudience != tt.strict || m.RequiresDM != tt.dm || !reflect.DeepEqual(m.Sources, tt.sources) {
				t.Errorf("audience allowed=%v strict=%v dm=%v sources=%v", m.AllowedUsers, m.StrictAudience, m.RequiresDM, m.Sources)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(m.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload) != len(tt.payloads) {
				t.Errorf("payload keys %v, want %v", payload, tt.payloads)
			}
			for _, k := range tt.payloads {
				if _, ok := payload[k]; !ok {
					t.Errorf("payload lacks %q", k)
				}
			}
			if v, ok := payload["viewers"]; ok && string(v) != `["ana","bo"]` {
				t.Errorf("viewers %s", v)
			}
		})
	}

	// No bus yet, or nobody picked: nothing is sent.
	(&cutsceneLivePublisher{}).Show("camp-1", sh)
	empty := &cutsceneBus{}
	(&cutsceneLivePublisher{bus: empty}).Show("camp-1", cutscenes.Showing{ID: "sh-2"})
	if len(empty.got) != 0 {
		t.Errorf("a showing with no viewers was published: %+v", empty.got)
	}
}

type cutsceneFoundryHub struct{ connected bool }

func (h cutsceneFoundryHub) FoundryPresence(string) (*time.Time, bool) { return nil, h.connected }

// A preview goes only to the Foundry module's DM-held sockets, never a
// browser, and only when the module is connected to hear it; the add-on
// decides whether the editor offers it at all.
func TestCutsceneFoundryPreview(t *testing.T) {
	on := func(context.Context, string) (bool, error) { return true, nil }
	off := func(context.Context, string) (bool, error) { return false, nil }
	broken := func(context.Context, string) (bool, error) { return true, errors.New("down") }
	if !(&cutsceneFoundryPreview{enabled: on}).Offered(context.Background(), "camp-1") {
		t.Error("Sync API on: not offered")
	}
	for name, f := range map[string]func(context.Context, string) (bool, error){"off": off, "broken": broken, "unbound": nil} {
		if (&cutsceneFoundryPreview{enabled: f}).Offered(context.Background(), "camp-1") {
			t.Errorf("%s: offered", name)
		}
	}

	sc := cutscenes.Scene{ID: "s-1", Name: "Unsaved"}
	bus := &cutsceneBus{}
	p := &cutsceneFoundryPreview{hub: cutsceneFoundryHub{connected: true}, bus: bus}
	if !p.Preview("camp-1", "dir", sc) || len(bus.got) != 1 {
		t.Fatalf("connected: sent %d", len(bus.got))
	}
	m := bus.got[0]
	if m.Type != ws.MsgCutsceneFoundryPreview || !m.RequiresDM || !reflect.DeepEqual(m.Sources, []string{foundry_vtt.ModuleSource}) || len(m.AllowedUsers) != 0 {
		t.Errorf("audience: %s dm=%v sources=%v allowed=%v", m.Type, m.RequiresDM, m.Sources, m.AllowedUsers)
	}
	var payload struct {
		PreviewID string          `json:"previewId"`
		Director  string          `json:"director"`
		Scene     cutscenes.Scene `json:"scene"`
	}
	if err := json.Unmarshal(m.Payload, &payload); err != nil || payload.PreviewID == "" || payload.PreviewID != m.ResourceID || payload.Director != "dir" || payload.Scene.Name != "Unsaved" {
		t.Errorf("payload %s (%v)", m.Payload, err)
	}

	quiet := &cutsceneBus{}
	if (&cutsceneFoundryPreview{hub: cutsceneFoundryHub{}, bus: quiet}).Preview("camp-1", "dir", sc) || len(quiet.got) != 0 {
		t.Error("Foundry not connected: still sent")
	}
	if (&cutsceneFoundryPreview{}).Preview("camp-1", "dir", sc) {
		t.Error("unbound: reported sent")
	}
}
