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
	MsgMapUpdated       MessageType = "map.updated"
	MsgDrawingCreated   MessageType = "drawing.created"
	MsgDrawingUpdated   MessageType = "drawing.updated"
	MsgDrawingDeleted   MessageType = "drawing.deleted"
	MsgTokenCreated     MessageType = "token.created"
	MsgTokenMoved       MessageType = "token.moved"
	MsgTokenUpdated     MessageType = "token.updated"
	MsgTokenDeleted     MessageType = "token.deleted"
	MsgMarkerCreated    MessageType = "marker.created"
	MsgMarkerUpdated    MessageType = "marker.updated"
	MsgMarkerDeleted    MessageType = "marker.deleted"
	MsgFogUpdated       MessageType = "fog.updated"
	MsgLayerUpdated     MessageType = "layer.updated"
)

// Calendar sync messages.
const (
	MsgCalendarEventCreated MessageType = "calendar.event.created"
	MsgCalendarEventUpdated MessageType = "calendar.event.updated"
	MsgCalendarEventDeleted MessageType = "calendar.event.deleted"
	MsgCalendarDateAdvanced MessageType = "calendar.date.advanced"
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
	Type       MessageType    `json:"type"`
	CampaignID string         `json:"campaignId"`
	ResourceID string         `json:"resourceId,omitempty"` // ID of the affected resource.
	SenderID   string         `json:"senderId,omitempty"`   // Connection ID of sender (for echo suppression).
	Payload    json.RawMessage `json:"payload,omitempty"`    // Type-specific data.
}

// Encode serializes a Message to JSON bytes.
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeMessage parses a JSON byte slice into a Message.
func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
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
