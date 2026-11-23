package websocket

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
)

// Sources narrows a message to kinds of socket and never widens it: a cut
// scene reaches the browser tabs of the people it is for and nothing else,
// and its Foundry relay reaches the module's socket alone.
func TestDeliversTo_Sources(t *testing.T) {
	show := func() *Message {
		return &Message{Type: MsgCutsceneShow, AllowedUsers: []string{"ana", "dir"}, StrictAudience: true, Sources: []string{BrowserSource}}
	}
	relay := func() *Message {
		return &Message{Type: MsgCutsceneFoundry, RequiresDM: true, Sources: []string{foundry_vtt.ModuleSource}}
	}
	owner, player := permissions.RoleOwner, permissions.RolePlayer
	tests := []struct {
		name   string
		msg    *Message
		client *Client
		want   bool
	}{
		{"show reaches a viewer's browser", show(), &Client{UserID: "ana", Role: player, Source: BrowserSource}, true},
		{"show skips a player it is not for", show(), &Client{UserID: "bo", Role: player, Source: BrowserSource}, false},
		{"show skips the Director who did not pick themselves", show(), &Client{UserID: "owner", Role: owner, Source: BrowserSource}, false},
		{"show reaches the Director who picked themselves", show(), &Client{UserID: "dir", Role: owner, Source: BrowserSource}, true},
		{"show skips a co-DM it is not for", show(), &Client{UserID: "codm", Role: player, IsDmGranted: true, Source: BrowserSource}, false},
		{"show skips the viewer's Foundry module socket", show(), &Client{UserID: "ana", Role: player, Source: foundry_vtt.ModuleSource}, false},
		{"show skips the viewer's notes-grant socket", show(), &Client{UserID: "ana", Role: player, Source: NotesAppSource}, false},
		{"show skips a plain sync-key socket", show(), &Client{UserID: "ana", Role: player, Source: "foundry"}, false},
		{"relay reaches the module socket of the GM's key", relay(), &Client{UserID: "owner", Role: owner, Source: foundry_vtt.ModuleSource}, true},
		{"relay skips the GM's browser tab", relay(), &Client{UserID: "owner", Role: owner, Source: BrowserSource}, false},
		{"relay skips a module socket on a player's key", relay(), &Client{UserID: "ana", Role: player, Source: foundry_vtt.ModuleSource}, false},
		{"relay skips a plain sync-key socket", relay(), &Client{UserID: "owner", Role: owner, Source: "foundry"}, false},
		{"no Sources keeps every kind", &Message{Type: MsgRollMade, Public: true}, &Client{UserID: "ana", Role: player, Source: foundry_vtt.ModuleSource}, true},
		{"Sources never widens a notes grant", &Message{Type: MsgRollMade, Public: true, Sources: []string{NotesAppSource}}, &Client{UserID: "ana", Role: player, Source: NotesAppSource}, false},
		{"Sources never widens a DM-only message", &Message{Type: MsgRollMade, RequiresDM: true, Sources: []string{BrowserSource}}, &Client{UserID: "ana", Role: player, Source: BrowserSource}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.DeliversTo(tt.client); got != tt.want {
				t.Errorf("DeliversTo = %v, want %v", got, tt.want)
			}
		})
	}
}

// Sources is routing, never sent: a client must not learn which kinds of
// socket a message was meant for.
func TestSourcesNeverEncoded(t *testing.T) {
	m := &Message{Type: MsgCutsceneShow, CampaignID: "c", Sources: []string{BrowserSource}}
	b, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `{"type":"cutscene.show","campaignId":"c"}` {
		t.Errorf("encoded %s", got)
	}
}
