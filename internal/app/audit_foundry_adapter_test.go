package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// Recent Activity lists what Foundry changed, connected and settled; reads,
// failures, the module's own notes and Chronicle's pushes stay on the Sync
// history page.
func TestFoundryActivityRow(t *testing.T) {
	chronicle := syncapi.SyncEvent{ReportedBy: "chronicle", OK: true}
	client := syncapi.SyncEvent{ReportedBy: "client", OK: true}
	with := func(e syncapi.SyncEvent, f func(*syncapi.SyncEvent)) syncapi.SyncEvent { f(&e); return e }
	tests := []struct {
		name string
		ev   syncapi.SyncEvent
		want bool
	}{
		{"a page Foundry wrote", with(chronicle, func(e *syncapi.SyncEvent) { e.Direction = syncapi.DirToChronicle; e.Kind = "page" }), true},
		{"a failed write", with(chronicle, func(e *syncapi.SyncEvent) { e.Direction = syncapi.DirToChronicle; e.OK = false }), false},
		{"a module note sent to Chronicle", with(client, func(e *syncapi.SyncEvent) { e.Direction = syncapi.DirToChronicle }), false},
		{"Chronicle's push to Foundry", with(client, func(e *syncapi.SyncEvent) { e.Direction = syncapi.DirToFoundry }), false},
		{"connected", with(client, func(e *syncapi.SyncEvent) { e.Direction = syncapi.DirLink; e.Kind = "connection" }), true},
		{"a sync warning", with(client, func(e *syncapi.SyncEvent) { e.Direction = syncapi.DirLink; e.Kind = "sync" }), false},
		{"a settled conflict", with(client, func(e *syncapi.SyncEvent) { e.Direction = syncapi.DirToFoundry; e.Status = "conflict" }), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := foundryActivityRow(tt.ev); got != tt.want {
				t.Errorf("foundryActivityRow = %v, want %v", got, tt.want)
			}
		})
	}
}
