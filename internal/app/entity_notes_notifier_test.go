package app

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	ws "github.com/keyxmakerx/chronicle/internal/websocket"
	"github.com/keyxmakerx/chronicle/internal/widgets/entity_notes"
)

// TestEntityNotesNotifier_WirePayloadIsIDsOnly pins the serialized message,
// not the Go call: the service hands the notifier the whole note, so only this
// wiring stands between a private note's text and every campaign subscriber.
func TestEntityNotesNotifier_WirePayloadIsIDsOnly(t *testing.T) {
	const (
		secretTitle = "SECRET-TITLE-9f3a"
		secretBody  = "<p>SECRET-BODY-71bc</p>"
	)
	events := map[string]ws.MessageType{
		"entity_notes.created": ws.MsgEntityNoteCreated,
		"entity_notes.updated": ws.MsgEntityNoteUpdated,
		"entity_notes.deleted": ws.MsgEntityNoteDeleted,
	}
	for event, wantType := range events {
		t.Run(event, func(t *testing.T) {
			bus := &captureBus{}
			h := &entityNotesNotifierHolder{bus: bus}
			h.Notify(event, &entity_notes.Note{
				ID: "n1", EntityID: "e1", CampaignID: "c1",
				Title: secretTitle, BodyHTML: secretBody,
				Audience: entity_notes.AudiencePrivate,
			}, entity_notes.Audience(entity_notes.AudiencePrivate))

			msg := bus.last
			if msg == nil {
				t.Fatal("nothing published")
			}
			if msg.Type != wantType {
				t.Errorf("type = %q, want %q", msg.Type, wantType)
			}
			wire, err := json.Marshal(msg)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"SECRET-TITLE", "SECRET-BODY", secretTitle, secretBody} {
				if strings.Contains(string(wire), secret) {
					t.Errorf("wire message leaks note content %q: %s", secret, wire)
				}
			}
			var payload map[string]any
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			keys := make([]string, 0, len(payload))
			for k := range payload {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if got := strings.Join(keys, ","); got != "entityId,noteId" {
				t.Errorf("payload keys = %s, want entityId,noteId", got)
			}
			if payload["entityId"] != "e1" || payload["noteId"] != "n1" {
				t.Errorf("payload = %v", payload)
			}
		})
	}
}
