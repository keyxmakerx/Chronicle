package app

import (
	"fmt"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
	"github.com/keyxmakerx/chronicle/internal/widgets/entity_notes"
)

// TestEntityNoteEvents_ReachOnlyReaders pins that a Player Notes change
// nudges exactly the people the list endpoint would show the note to.
// A nudge to anyone else tells them a hidden note exists on the page.
func TestEntityNoteEvents_ReachOnlyReaders(t *testing.T) {
	type viewer struct {
		name    string
		id      string
		role    int
		granted bool
	}
	viewers := []viewer{
		{"author player", "u-author", permissions.RolePlayer, false},
		{"other player", "u-other", permissions.RolePlayer, false},
		{"shared-with player", "u-friend", permissions.RolePlayer, false},
		{"co-DM", "u-scribe", permissions.RoleScribe, false},
		{"DM-granted player", "u-granted", permissions.RolePlayer, true},
		{"owner", "u-owner", permissions.RoleOwner, false},
	}
	audiences := []entity_notes.Audience{
		entity_notes.AudiencePrivate, entity_notes.AudienceDMOnly,
		entity_notes.AudienceDMScribe, entity_notes.AudienceEveryone,
		entity_notes.AudienceCustom,
	}
	for _, aud := range audiences {
		note := &entity_notes.Note{ID: "n1", EntityID: "e1", CampaignID: "c1", AuthorUserID: "u-author", Audience: aud}
		if aud == entity_notes.AudienceCustom {
			note.SharedWith = []string{"u-friend"}
		}
		bus := &noteCaptureBus{}
		(&entityNotesNotifierHolder{bus: bus}).Notify("entity_notes.updated", note, aud)
		if len(bus.msgs) != 1 {
			t.Fatalf("%s: published %d messages, want 1", aud, len(bus.msgs))
		}
		msg := bus.msgs[0]
		for _, v := range viewers {
			t.Run(fmt.Sprintf("%s/%s", aud, v.name), func(t *testing.T) {
				reads := entity_notes.NotePassesACL(note, entity_notes.ViewerContext{
					UserID: v.id, CampaignID: "c1",
					IsOwner:     v.role >= permissions.RoleOwner,
					IsScribe:    v.role == permissions.RoleScribe,
					IsDMGranted: v.granted,
				})
				got := msg.DeliversTo(&ws.Client{UserID: v.id, Role: v.role, IsDmGranted: v.granted, Source: "browser"})
				if got != reads {
					t.Errorf("nudge delivered=%v, but the list shows the note=%v", got, reads)
				}
			})
		}
	}
}
