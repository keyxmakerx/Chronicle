package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// Quest and board messages carry ids only, and keep the audience the quests
// plugin decided.
func TestQuestAnnouncer(t *testing.T) {
	tests := []struct {
		name     string
		send     func(a *questAnnouncerAdapter)
		typ      ws.MessageType
		resource string
		payload  string
		dmOnly   bool
	}{
		{"quest", func(a *questAnnouncerAdapter) { a.QuestChanged("c1", "e1", 4, false) }, ws.MsgQuestUpdated, "e1", `{"version":4}`, false},
		{"hidden quest", func(a *questAnnouncerAdapter) { a.QuestChanged("c1", "e1", 4, true) }, ws.MsgQuestUpdated, "e1", `{"version":4}`, true},
		{"page boards", func(a *questAnnouncerAdapter) { a.BoardsChanged("c1", quests.PageHome("e2"), true) }, ws.MsgNoticeBoardsUpdated, "e2", `{"home":"page"}`, true},
		{"category boards", func(a *questAnnouncerAdapter) { a.BoardsChanged("c1", quests.TypeHome(13), false) }, ws.MsgNoticeBoardsUpdated, "13", `{"home":"category"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &spotlightCaptureBus{}
			tt.send(&questAnnouncerAdapter{bus: bus})
			if len(bus.msgs) != 1 {
				t.Fatalf("published %d messages, want 1", len(bus.msgs))
			}
			m := bus.msgs[0]
			if m.Type != tt.typ || m.CampaignID != "c1" || m.ResourceID != tt.resource || string(m.Payload) != tt.payload || m.RequiresDM != tt.dmOnly {
				t.Errorf("message = %+v payload %s", m, m.Payload)
			}
		})
	}
}
