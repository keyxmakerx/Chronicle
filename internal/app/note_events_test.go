package app

import (
	"encoding/json"
	"reflect"
	"testing"

	ws "github.com/keyxmakerx/chronicle/internal/websocket"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// noteCaptureBus records every message published to it.
type noteCaptureBus struct{ msgs []*ws.Message }

func (b *noteCaptureBus) Publish(m *ws.Message) { b.msgs = append(b.msgs, m) }

// TestNoteEvents_WireCarriesIDsOnly pins the note WebSocket payload as it
// actually goes on the wire: the note and page ids, nothing of the note
// itself, and no "id" key (a client reading payload.id as a whole note must
// skip the message rather than overwrite its copy with an empty one).
func TestNoteEvents_WireCarriesIDsOnly(t *testing.T) {
	bus := &noteCaptureBus{}
	a := &noteEventPublisherAdapter{bus: bus}
	page := "page-1"
	a.PublishNoteEvent(notes.NoteEvent{
		Type: "updated", CampaignID: "c1", NoteID: "n1", EntityID: &page,
		Audience: notes.Audience{Users: []string{"u-owner"}},
	})
	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(bus.msgs))
	}
	raw, err := bus.msgs[0].Encode()
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Type       string            `json:"type"`
		CampaignID string            `json:"campaignId"`
		ResourceID string            `json:"resourceId"`
		Payload    map[string]string `json:"payload"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("wire message is not the expected shape: %v (%s)", err, raw)
	}
	if wire.Type != string(ws.MsgNoteUpdated) || wire.ResourceID != "n1" {
		t.Errorf("wrong envelope: %s", raw)
	}
	if !reflect.DeepEqual(wire.Payload, map[string]string{"noteId": "n1", "entityId": "page-1"}) {
		t.Errorf("payload must carry ids only, got %v", wire.Payload)
	}
}

// TestNoteEvents_AudienceMapping pins how a note's audience becomes the
// hub's allowlist, and that a private note binds the GM too.
func TestNoteEvents_AudienceMapping(t *testing.T) {
	tests := []struct {
		name        string
		aud         notes.Audience
		wantAllowed []string
		wantStrict  bool
	}{
		{"party: everyone", notes.Audience{Everyone: true}, nil, false},
		{"private: the owner, not the GM", notes.Audience{Users: []string{"u-owner"}}, []string{"u-owner"}, true},
		{"custom: the named, not the GM", notes.Audience{Users: []string{"u-owner", "u-named"}}, []string{"u-owner", "u-named"}, true},
		{"GM share: the owner, GMs by the hub's bypass", notes.Audience{GMs: true, Users: []string{"u-owner"}}, []string{"u-owner"}, false},
		{"malformed (nobody): nobody, never everyone", notes.Audience{}, []string{noteNoRecipient}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &noteCaptureBus{}
			(&noteEventPublisherAdapter{bus: bus}).PublishNoteEvent(notes.NoteEvent{Type: "created", CampaignID: "c1", NoteID: "n1", Audience: tt.aud})
			m := bus.msgs[0]
			if !reflect.DeepEqual(m.AllowedUsers, tt.wantAllowed) || m.StrictAudience != tt.wantStrict {
				t.Errorf("allowed %v strict %v, want %v %v", m.AllowedUsers, m.StrictAudience, tt.wantAllowed, tt.wantStrict)
			}
		})
	}
}
