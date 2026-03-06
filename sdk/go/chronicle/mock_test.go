package chronicle

import (
	"encoding/json"
	"testing"
)

func TestMockHostEntity(t *testing.T) {
	mock := NewMockHost()

	// Add an entity.
	err := mock.AddEntity("ent-1", map[string]any{"name": "Gandalf", "type": "npc"})
	if err != nil {
		t.Fatalf("AddEntity failed: %v", err)
	}

	// Get it back.
	data, err := mock.GetEntity("ent-1")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}

	var entity map[string]any
	if err := json.Unmarshal(data, &entity); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if entity["name"] != "Gandalf" {
		t.Errorf("expected name 'Gandalf', got %v", entity["name"])
	}

	// Not found.
	_, err = mock.GetEntity("ent-999")
	if err == nil {
		t.Error("expected error for non-existent entity")
	}
}

func TestMockHostTags(t *testing.T) {
	mock := NewMockHost()
	mock.AddTag("camp-1", Tag{ID: 1, Name: "NPC"})
	mock.AddTag("camp-1", Tag{ID: 2, Name: "Location"})

	data, err := mock.ListTags("camp-1")
	if err != nil {
		t.Fatalf("ListTags failed: %v", err)
	}

	var tags []Tag
	if err := json.Unmarshal(data, &tags); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
}

func TestMockHostEntityTags(t *testing.T) {
	mock := NewMockHost()
	mock.AddTag("camp-1", Tag{ID: 1, Name: "NPC"})
	mock.SetEntityTags("ent-1", []int{1})

	ids := mock.EntityTagIDs("ent-1")
	if len(ids) != 1 || ids[0] != 1 {
		t.Errorf("expected [1], got %v", ids)
	}

	// Update via mock set.
	_ = mock.MockSetEntityTags("ent-1", []int{1, 2})
	ids = mock.EntityTagIDs("ent-1")
	if len(ids) != 2 {
		t.Errorf("expected 2 tag IDs, got %d", len(ids))
	}
}

func TestMockHostKVStore(t *testing.T) {
	mock := NewMockHost()

	// Initially empty.
	_, ok := mock.KVGet("key1")
	if ok {
		t.Error("expected key not found")
	}

	// Set and get.
	mock.KVSet("key1", "value1")
	val, ok := mock.KVGet("key1")
	if !ok || val != "value1" {
		t.Errorf("expected 'value1', got %q (found=%v)", val, ok)
	}

	// Delete.
	mock.KVDelete("key1")
	_, ok = mock.KVGet("key1")
	if ok {
		t.Error("expected key deleted")
	}
}

func TestMockHostCreateEvent(t *testing.T) {
	mock := NewMockHost()

	input, _ := json.Marshal(CreateEventInput{Name: "Festival", Year: 1492, Month: 6, Day: 15})
	result, err := mock.CreateEvent(input)
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	var event map[string]any
	if err := json.Unmarshal(result, &event); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if event["name"] != "Festival" {
		t.Errorf("expected name 'Festival', got %v", event["name"])
	}
	if event["id"] != "evt-1" {
		t.Errorf("expected id 'evt-1', got %v", event["id"])
	}

	events := mock.Events()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestMockHostCreateRelation(t *testing.T) {
	mock := NewMockHost()

	input, _ := json.Marshal(CreateRelationInput{
		SourceEntityID: "ent-1",
		TargetEntityID: "ent-2",
		RelationType:   "ally_of",
	})
	result, err := mock.CreateRelation(input)
	if err != nil {
		t.Fatalf("CreateRelation failed: %v", err)
	}

	var rel map[string]any
	if err := json.Unmarshal(result, &rel); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if rel["id"] != "rel-1" {
		t.Errorf("expected id 'rel-1', got %v", rel["id"])
	}

	rels := mock.Relations()
	if len(rels) != 1 {
		t.Errorf("expected 1 relation, got %d", len(rels))
	}
}

func TestMockHostLog(t *testing.T) {
	mock := NewMockHost()
	mock.Log("hello")
	mock.Log("world")

	logs := mock.Logs()
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}
	if logs[0] != "hello" || logs[1] != "world" {
		t.Errorf("unexpected logs: %v", logs)
	}
}

func TestMockHostSendMessage(t *testing.T) {
	mock := NewMockHost()
	mock.SendMessage(SendMessageInput{
		TargetExtID: "ext-2",
		TargetSlug:  "plugin-b",
		Payload:     json.RawMessage(`{"action":"ping"}`),
	})

	msgs := mock.Messages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].TargetExtID != "ext-2" {
		t.Errorf("unexpected target: %s", msgs[0].TargetExtID)
	}
}

func TestMockHostReset(t *testing.T) {
	mock := NewMockHost()
	_ = mock.AddEntity("ent-1", map[string]string{"name": "test"})
	mock.Log("msg")
	mock.KVSet("k", "v")

	mock.Reset()

	_, err := mock.GetEntity("ent-1")
	if err == nil {
		t.Error("expected entity cleared after reset")
	}
	if len(mock.Logs()) != 0 {
		t.Error("expected logs cleared after reset")
	}
	if _, ok := mock.KVGet("k"); ok {
		t.Error("expected KV cleared after reset")
	}
}
