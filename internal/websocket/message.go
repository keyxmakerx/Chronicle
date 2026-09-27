// Package websocket provides a campaign-scoped WebSocket hub for real-time
// bidirectional communication between Chronicle and external clients (Foundry VTT).
// The hub broadcasts domain events (entity changes, map updates, calendar advances)
// to all connections in a campaign, enabling live sync without polling.
package websocket

import "encoding/json"

// MessageType identifies the kind of event being broadcast.
type MessageType string

// Entity sync messages.
const (
	MsgEntityCreated MessageType = "entity.created"
	MsgEntityUpdated MessageType = "entity.updated"
	MsgEntityDeleted MessageType = "entity.deleted"
)

// Map sync messages.
const (
	MsgMapUpdated     MessageType = "map.updated"
	MsgDrawingCreated MessageType = "drawing.created"
	MsgDrawingUpdated MessageType = "drawing.updated"
	MsgDrawingDeleted MessageType = "drawing.deleted"
	MsgTokenCreated   MessageType = "token.created"
	MsgTokenMoved     MessageType = "token.moved"
	MsgTokenUpdated   MessageType = "token.updated"
	MsgTokenDeleted   MessageType = "token.deleted"
	MsgMarkerCreated  MessageType = "marker.created"
	MsgMarkerUpdated  MessageType = "marker.updated"
	MsgMarkerDeleted  MessageType = "marker.deleted"
	// Fog and layer messages split into per-lifecycle types so clients
	// can discriminate create / update / delete without re-fetching the
	// full sub-resource list. The original MsgFogUpdated / MsgLayerUpdated
	// constants stay valid for actual updates; the *Created / *Deleted
	// types are added alongside.
	MsgFogCreated   MessageType = "fog.created"
	MsgFogUpdated   MessageType = "fog.updated"
	MsgFogDeleted   MessageType = "fog.deleted"
	MsgLayerCreated MessageType = "layer.created"
	MsgLayerUpdated MessageType = "layer.updated"
	MsgLayerDeleted MessageType = "layer.deleted"
)

// Calendar sync messages.
const (
	MsgCalendarEventCreated     MessageType = "calendar.event.created"
	MsgCalendarEventUpdated     MessageType = "calendar.event.updated"
	MsgCalendarEventDeleted     MessageType = "calendar.event.deleted"
	MsgCalendarDateAdvanced     MessageType = "calendar.date.advanced"
	MsgCalendarSeasonChanged    MessageType = "calendar.season.changed"
	MsgCalendarMoonPhaseChanged MessageType = "calendar.moon.phase_changed"
	MsgCalendarWeatherChanged   MessageType = "calendar.weather.changed"
	MsgCalendarStructureUpdated MessageType = "calendar.structure.updated"
	MsgCalendarEraChanged       MessageType = "calendar.era.changed"
	// Cycle + festival mutations fan out a sub-resource event in addition
	// to the umbrella structure.updated, so a subscriber can act at the
	// granularity it needs.
	MsgCalendarCycleChanged        MessageType = "calendar.cycle.changed"
	MsgCalendarFestivalChanged     MessageType = "calendar.festival.changed"
	MsgCalendarWorldstateChanged   MessageType = "calendar.worldstate.changed"
	MsgCalendarWeatherZonesChanged MessageType = "calendar.weather.zones.changed"
)

// Entity type sync messages.
const (
	MsgEntityTypeCreated MessageType = "entity_type.created"
	MsgEntityTypeUpdated MessageType = "entity_type.updated"
	MsgEntityTypeDeleted MessageType = "entity_type.deleted"
)

// Note sync messages.
const (
	MsgNoteCreated MessageType = "note.created"
	MsgNoteUpdated MessageType = "note.updated"
	MsgNoteDeleted MessageType = "note.deleted"
)

// Entity notes sync messages (player-notes addon). Distinct from note.*
// because the data model + audience semantics are different — these
// fire from internal/widgets/entity_notes, those from internal/widgets/notes.
const (
	MsgEntityNoteCreated MessageType = "entity_note.created"
	MsgEntityNoteUpdated MessageType = "entity_note.updated"
	MsgEntityNoteDeleted MessageType = "entity_note.deleted"
)

// Sync control messages.
const (
	MsgSyncStatus   MessageType = "sync.status"
	MsgSyncError    MessageType = "sync.error"
	MsgSyncConflict MessageType = "sync.conflict"
)

// Message is the envelope for all WebSocket communication.
// Clients and servers exchange these JSON messages over the WS connection.
type Message struct {
	Type       MessageType     `json:"type"`
	CampaignID string          `json:"campaignId"`
	ResourceID string          `json:"resourceId,omitempty"` // ID of the affected resource.
	SenderID   string          `json:"senderId,omitempty"`   // Connection ID of sender (for echo suppression).
	Payload    json.RawMessage `json:"payload,omitempty"`    // Type-specific data.

	// RequiresDM marks a message that must only be delivered to clients
	// with DM-equivalent visibility (campaign Owner or IsDmGranted=true).
	// Set by emitters whose source row carries dm_only / hidden state (a
	// dm_only marker or drawing, a hidden token, any fog event). The hub's
	// broadcast loop drops the message for non-DM connections at delivery
	// time — see hub.go. JSON-omitted so unaffected payloads are unchanged.
	RequiresDM bool `json:"requiresDm,omitempty"`

	// AllowedUsers and DeniedUsers narrow a message's audience beyond the
	// binary RequiresDM gate: the per-user visibility_rules a map marker
	// or drawing can carry (ADR-055 rule 3 applied to this channel). A
	// "specific" visibility marker isn't dm_only — RequiresDM is false for
	// it — but it must still only reach the users its rules admit.
	//
	// Populated by the emitter that holds the source row (mapEventPublisherAdapter,
	// which parses maps.Marker/Drawing's VisibilityRules), consumed only by
	// the hub's broadcast loop, which drops the message per-recipient the
	// same way it does for RequiresDM. `json:"-"`: this is server-side
	// audience metadata, never client-facing — echoing back which user IDs
	// are denied would itself be evidence that hidden content exists. A
	// recipient outside the audience gets nothing: no message, no stub.
	AllowedUsers []string `json:"-"`
	DeniedUsers  []string `json:"-"`
}

// Encode serializes a Message to JSON bytes.
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// NewMessage creates a Message with the given type, campaign, and payload.
// The payload is marshaled to JSON. If marshaling fails, payload is set to null.
func NewMessage(msgType MessageType, campaignID, resourceID string, payload any) *Message {
	var raw json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err == nil {
			raw = data
		}
	}
	return &Message{
		Type:       msgType,
		CampaignID: campaignID,
		ResourceID: resourceID,
		Payload:    raw,
	}
}
