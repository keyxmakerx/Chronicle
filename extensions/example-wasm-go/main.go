// Chronicle WASM Plugin Example: Session Logger (Go/TinyGo)
//
// Demonstrates how to build a Chronicle WASM logic extension using the
// Extism Go PDK. This plugin:
//
// 1. Listens for entity.created/updated hooks and logs activity to KV store.
// 2. Exports a `get_activity_log` function that returns stored activity.
// 3. Shows how to call Chronicle host functions from Go.
//
// Build with TinyGo:
//
//	tinygo build -o dist/session_logger.wasm -target wasip1 -buildmode=c-shared main.go
//
// Build with Go 1.24+ (native WASM support). -buildmode=c-shared is required:
// without it the module loads but every call traps, because the Go runtime
// is never initialised.
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o dist/session_logger.wasm main.go
package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/extism/go-pdk"
)

// ---------------------------------------------------------------------------
// Host function declarations
// ---------------------------------------------------------------------------
// These are provided by the Chronicle runtime. The plugin's declared
// capabilities determine which functions are available. Each signature must
// match the host's exactly or the plugin fails to instantiate.

//go:wasmimport extism:host/user chronicle_log
func chronicleLog(offset uint64)

//go:wasmimport extism:host/user kv_get
func kvGet(offset uint64) uint64

//go:wasmimport extism:host/user kv_set
func kvSet(offset uint64) uint64

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// HookEvent is the input Chronicle sends to on_hook.
type HookEvent struct {
	Type       string `json:"type"`
	CampaignID string `json:"campaign_id"`
	Payload    struct {
		EntityID string `json:"entity_id"`
		Name     string `json:"name"`
	} `json:"payload"`
	Timestamp string `json:"timestamp"`
}

// ActivityEntry represents one logged activity.
type ActivityEntry struct {
	Timestamp string `json:"timestamp"`
	EventType string `json:"event_type"`
	EntityID  string `json:"entity_id"`
}

// ActivityLog is the stored activity log (kept in KV store).
type ActivityLog struct {
	Entries []ActivityEntry `json:"entries"`
}

// ---------------------------------------------------------------------------
// Host function helpers
// ---------------------------------------------------------------------------

// logMessage sends an info line to the Chronicle server log.
func logMessage(msg string) {
	input, _ := json.Marshal(map[string]string{"level": "info", "message": msg})
	mem := pdk.AllocateBytes(input)
	chronicleLog(mem.Offset())
}

// hostResult is how every host function answers: {"data": ...} on success,
// {"error": "..."} on failure.
type hostResult struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

// kvGetValue reads a value from the plugin's KV store ("null" when unset).
func kvGetValue(key string) ([]byte, error) {
	input, _ := json.Marshal(map[string]string{"key": key})
	mem := pdk.AllocateBytes(input)
	resultOffset := kvGet(mem.Offset())
	resultMem := pdk.FindMemory(resultOffset)
	var res hostResult
	if err := json.Unmarshal(resultMem.ReadBytes(), &res); err != nil {
		return nil, err
	}
	if res.Error != "" {
		return nil, fmt.Errorf("kv_get: %s", res.Error)
	}
	return res.Data, nil
}

// kvSetValue writes a JSON value to the plugin's KV store. The value is sent
// as JSON, not as a string holding JSON, so kv_get gives the same shape back.
func kvSetValue(key string, value []byte) error {
	input, _ := json.Marshal(map[string]any{
		"key":   key,
		"value": json.RawMessage(value),
	})
	mem := pdk.AllocateBytes(input)
	kvSet(mem.Offset())
	return nil
}

// ---------------------------------------------------------------------------
// Exported functions
// ---------------------------------------------------------------------------

// on_hook is called by Chronicle when entity.created or entity.updated fires.
// It appends an activity entry to the KV-stored log.
//
//go:wasmexport on_hook
func onHook() int32 {
	var event HookEvent
	if err := pdk.InputJSON(&event); err != nil {
		pdk.SetError(err)
		return 1
	}

	if event.Payload.EntityID == "" {
		_ = pdk.OutputJSON(map[string]any{"ok": true, "skipped": "no entity_id"})
		return 0
	}

	logMessage(fmt.Sprintf("session-logger: %s on entity %s", event.Type, event.Payload.EntityID))

	// Load existing activity log from KV store.
	var actLog ActivityLog
	existing, err := kvGetValue("activity_log")
	if err == nil && len(existing) > 0 {
		_ = json.Unmarshal(existing, &actLog)
	}

	// Append new entry (keep last 100 entries).
	actLog.Entries = append(actLog.Entries, ActivityEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		EventType: event.Type,
		EntityID:  event.Payload.EntityID,
	})
	if len(actLog.Entries) > 100 {
		actLog.Entries = actLog.Entries[len(actLog.Entries)-100:]
	}

	// Save back to KV store.
	data, _ := json.Marshal(actLog)
	_ = kvSetValue("activity_log", data)

	_ = pdk.OutputJSON(map[string]any{
		"ok":      true,
		"entries": len(actLog.Entries),
	})
	return 0
}

// get_activity_log returns the stored activity log.
//
//go:wasmexport get_activity_log
func getActivityLog() int32 {
	existing, err := kvGetValue("activity_log")
	if err != nil || len(existing) == 0 || string(existing) == "null" {
		_ = pdk.OutputJSON(ActivityLog{Entries: []ActivityEntry{}})
		return 0
	}

	// Return the raw stored JSON.
	pdk.Output(existing)
	return 0
}

// on_message handles plugin-to-plugin messages.
//
//go:wasmexport on_message
func onMessage() int32 {
	var envelope struct {
		SenderExtID string          `json:"sender_ext_id"`
		Payload     json.RawMessage `json:"payload"`
	}
	if err := pdk.InputJSON(&envelope); err != nil {
		pdk.SetError(err)
		return 1
	}

	logMessage(fmt.Sprintf("session-logger: received message from %s", envelope.SenderExtID))

	_ = pdk.OutputJSON(map[string]any{
		"ok":            true,
		"received_from": envelope.SenderExtID,
	})
	return 0
}

func main() {}
