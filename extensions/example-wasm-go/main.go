// Chronicle WASM Plugin Example: Session Logger (Go/TinyGo)
//
// Demonstrates how to build a Chronicle WASM logic extension using the
// Extism Go PDK. This plugin:
//
// 1. Listens for entity.created/updated hooks and logs activity to KV store.
// 2. Exports a `get_activity_log` function that returns stored activity.
// 3. Exports a `create_session_summary` function that creates a calendar event.
// 4. Shows how to call Chronicle host functions from Go.
//
// Build with TinyGo:
//
//	tinygo build -o dist/session_logger.wasm -target wasip1 main.go
//
// Build with Go 1.24+ (native WASM support):
//
//	GOOS=wasip1 GOARCH=wasm go build -o dist/session_logger.wasm main.go
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
// capabilities determine which functions are available.

//go:wasmimport extism:host/user chronicle_log
func chronicleLog(offset uint64) uint64

//go:wasmimport extism:host/user get_entity
func getEntity(offset uint64) uint64

//go:wasmimport extism:host/user create_event
func createEvent(offset uint64) uint64

//go:wasmimport extism:host/user kv_get
func kvGet(offset uint64) uint64

//go:wasmimport extism:host/user kv_set
func kvSet(offset uint64) uint64

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// HookEvent is the payload Chronicle sends for hook events.
type HookEvent struct {
	Type       string `json:"type"`
	EntityID   string `json:"entity_id,omitempty"`
	CampaignID string `json:"campaign_id,omitempty"`
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

// CalendarEventInput is sent to the create_event host function.
type CalendarEventInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Year        int    `json:"year"`
	Month       int    `json:"month"`
	Day         int    `json:"day"`
}

// ---------------------------------------------------------------------------
// Host function helpers
// ---------------------------------------------------------------------------

// logMessage sends a log message to the Chronicle server log.
func logMessage(msg string) {
	mem := pdk.AllocateString(msg)
	chronicleLog(mem.Offset())
}

// getEntityJSON calls the get_entity host function.
func getEntityJSON(entityID string) ([]byte, error) {
	input, _ := json.Marshal(map[string]string{"entity_id": entityID})
	mem := pdk.AllocateBytes(input)
	resultOffset := getEntity(mem.Offset())
	resultMem := pdk.FindMemory(resultOffset)
	return resultMem.ReadBytes(), nil
}

// kvGetValue reads a value from the plugin's KV store.
func kvGetValue(key string) ([]byte, error) {
	input, _ := json.Marshal(map[string]string{"key": key})
	mem := pdk.AllocateBytes(input)
	resultOffset := kvGet(mem.Offset())
	resultMem := pdk.FindMemory(resultOffset)
	return resultMem.ReadBytes(), nil
}

// kvSetValue writes a value to the plugin's KV store.
func kvSetValue(key string, value []byte) error {
	input, _ := json.Marshal(map[string]any{
		"key":   key,
		"value": string(value),
	})
	mem := pdk.AllocateBytes(input)
	kvSet(mem.Offset())
	return nil
}

// createCalendarEvent calls the create_event host function.
func createCalendarEvent(event CalendarEventInput) ([]byte, error) {
	input, _ := json.Marshal(event)
	mem := pdk.AllocateBytes(input)
	resultOffset := createEvent(mem.Offset())
	resultMem := pdk.FindMemory(resultOffset)
	return resultMem.ReadBytes(), nil
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

	if event.EntityID == "" {
		_ = pdk.OutputJSON(map[string]any{"ok": true, "skipped": "no entity_id"})
		return 0
	}

	logMessage(fmt.Sprintf("session-logger: %s on entity %s", event.Type, event.EntityID))

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
		EntityID:  event.EntityID,
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
	if err != nil || len(existing) == 0 {
		_ = pdk.OutputJSON(ActivityLog{Entries: []ActivityEntry{}})
		return 0
	}

	// Return the raw stored JSON.
	pdk.Output(existing)
	return 0
}

// create_session_summary creates a calendar event summarizing recent activity.
// Input: {"year": 1492, "month": 3, "day": 15, "title": "Session 5 Summary"}
//
//go:wasmexport create_session_summary
func createSessionSummary() int32 {
	var input struct {
		Year  int    `json:"year"`
		Month int    `json:"month"`
		Day   int    `json:"day"`
		Title string `json:"title"`
	}
	if err := pdk.InputJSON(&input); err != nil {
		pdk.SetError(err)
		return 1
	}

	// Build a description from the activity log.
	var actLog ActivityLog
	existing, _ := kvGetValue("activity_log")
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &actLog)
	}

	desc := fmt.Sprintf("Activity summary: %d events recorded.", len(actLog.Entries))

	result, err := createCalendarEvent(CalendarEventInput{
		Name:        input.Title,
		Description: desc,
		Year:        input.Year,
		Month:       input.Month,
		Day:         input.Day,
	})
	if err != nil {
		pdk.SetError(err)
		return 1
	}

	pdk.Output(result)
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
