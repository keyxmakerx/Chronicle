package chronicle

import (
	"encoding/json"
	"fmt"
	"sync"
)

// MockHost provides in-memory mock implementations of all Chronicle host
// functions for local plugin testing. Use it to verify plugin behavior
// without running the full Chronicle server.
//
// Example usage in tests:
//
//	mock := chronicle.NewMockHost()
//	mock.AddEntity("ent-1", map[string]any{"name": "Gandalf", "type": "npc"})
//	mock.AddTag(chronicle.Tag{ID: 1, Name: "NPC"})
//
//	// Then run your plugin logic with the mock data...
//	result := mock.GetEntity("ent-1")
type MockHost struct {
	mu       sync.RWMutex
	entities map[string]json.RawMessage   // keyed by entity ID
	tags     map[string][]Tag             // keyed by campaign ID
	entityTags map[string][]int           // keyed by entity ID -> tag IDs
	kvStore  map[string]string            // keyed by "extID:key"
	events   []json.RawMessage            // created calendar events
	relations []json.RawMessage           // created relations
	logs     []string                     // captured log messages
	messages []SendMessageInput           // captured outbound messages
}

// NewMockHost creates a new mock host with empty state.
func NewMockHost() *MockHost {
	return &MockHost{
		entities:   make(map[string]json.RawMessage),
		tags:       make(map[string][]Tag),
		entityTags: make(map[string][]int),
		kvStore:    make(map[string]string),
	}
}

// --- Setup helpers ---

// AddEntity adds an entity to the mock store. The data should be a map
// or struct that serializes to the expected entity JSON shape.
func (m *MockHost) AddEntity(entityID string, data any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling entity: %w", err)
	}
	m.entities[entityID] = b
	return nil
}

// AddTag adds a tag to a campaign's tag list.
func (m *MockHost) AddTag(campaignID string, tag Tag) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tags[campaignID] = append(m.tags[campaignID], tag)
}

// SetEntityTags sets the tag IDs for an entity.
func (m *MockHost) SetEntityTags(entityID string, tagIDs []int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entityTags[entityID] = tagIDs
}

// --- Host function mocks ---

// GetEntity returns the stored entity JSON or an error.
func (m *MockHost) GetEntity(entityID string) (json.RawMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.entities[entityID]
	if !ok {
		return nil, fmt.Errorf("entity %s not found", entityID)
	}
	return data, nil
}

// SearchEntities returns entities matching a simple name substring match.
func (m *MockHost) SearchEntities(campaignID, query string, limit int) (json.RawMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return all entities for simplicity in mock.
	results := make([]json.RawMessage, 0)
	for _, data := range m.entities {
		results = append(results, data)
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	b, _ := json.Marshal(results)
	return b, nil
}

// ListTags returns all tags for a campaign.
func (m *MockHost) ListTags(campaignID string) (json.RawMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tags := m.tags[campaignID]
	if tags == nil {
		tags = []Tag{}
	}
	b, _ := json.Marshal(tags)
	return b, nil
}

// GetEntityTags returns the tag IDs for an entity.
func (m *MockHost) GetEntityTags(entityID string) (json.RawMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tagIDs := m.entityTags[entityID]
	if tagIDs == nil {
		tagIDs = []int{}
	}

	// Build Tag objects from IDs by searching all campaign tags.
	var result []Tag
	for _, tags := range m.tags {
		for _, tag := range tags {
			for _, id := range tagIDs {
				if tag.ID == id {
					result = append(result, tag)
				}
			}
		}
	}
	b, _ := json.Marshal(result)
	return b, nil
}

// MockSetEntityTags records a set_entity_tags call.
func (m *MockHost) MockSetEntityTags(entityID string, tagIDs []int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entityTags[entityID] = tagIDs
	return nil
}

// CreateEvent records a calendar event creation and returns the input as-is.
func (m *MockHost) CreateEvent(input json.RawMessage) (json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, input)

	// Return the input with a mock ID added.
	var eventData map[string]any
	_ = json.Unmarshal(input, &eventData)
	eventData["id"] = fmt.Sprintf("evt-%d", len(m.events))
	result, _ := json.Marshal(eventData)
	return result, nil
}

// CreateRelation records a relation creation.
func (m *MockHost) CreateRelation(input json.RawMessage) (json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.relations = append(m.relations, input)

	var relData map[string]any
	_ = json.Unmarshal(input, &relData)
	relData["id"] = fmt.Sprintf("rel-%d", len(m.relations))
	result, _ := json.Marshal(relData)
	return result, nil
}

// KVGet returns a value from the mock KV store.
func (m *MockHost) KVGet(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.kvStore[key]
	return val, ok
}

// KVSet stores a value in the mock KV store.
func (m *MockHost) KVSet(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kvStore[key] = value
}

// KVDelete removes a value from the mock KV store.
func (m *MockHost) KVDelete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.kvStore, key)
}

// Log records a log message.
func (m *MockHost) Log(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, msg)
}

// SendMessage records an outbound message.
func (m *MockHost) SendMessage(msg SendMessageInput) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
}

// --- Inspection helpers ---

// Logs returns all captured log messages.
func (m *MockHost) Logs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]string, len(m.logs))
	copy(result, m.logs)
	return result
}

// Events returns all created calendar events.
func (m *MockHost) Events() []json.RawMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]json.RawMessage, len(m.events))
	copy(result, m.events)
	return result
}

// Relations returns all created relations.
func (m *MockHost) Relations() []json.RawMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]json.RawMessage, len(m.relations))
	copy(result, m.relations)
	return result
}

// Messages returns all captured outbound messages.
func (m *MockHost) Messages() []SendMessageInput {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]SendMessageInput, len(m.messages))
	copy(result, m.messages)
	return result
}

// EntityTagIDs returns the current tag IDs for an entity.
func (m *MockHost) EntityTagIDs(entityID string) []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := m.entityTags[entityID]
	if ids == nil {
		return []int{}
	}
	result := make([]int, len(ids))
	copy(result, ids)
	return result
}

// Reset clears all mock state.
func (m *MockHost) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entities = make(map[string]json.RawMessage)
	m.tags = make(map[string][]Tag)
	m.entityTags = make(map[string][]int)
	m.kvStore = make(map[string]string)
	m.events = nil
	m.relations = nil
	m.logs = nil
	m.messages = nil
}
