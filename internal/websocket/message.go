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

// MsgHexChanged tells map viewers that a map's hex layer changed. The payload
// is {map_id, version, party_path?} and never cell contents: clients refetch
// the role-filtered read, so who may see what is decided in one place.
// party_path rides along only when every hex on it is explored. Not a
// change-feed type: the filtered read is the source of truth.
const MsgHexChanged MessageType = "hex.changed"

// MsgMapItemsChanged tells map viewers that a map's pins, drawings, tokens or
// shadows changed. The payload is {map_id, kind} with kind one of "markers",
// "drawings", "tokens" or "shadows", and never any item content: a viewer
// refetches the role-filtered read, so a player only ever learns what their
// own refetch returns. It goes to every client of the campaign, like
// hex.changed, because it tells a viewer who may not see the item nothing. Not
// a change-feed type: the filtered read is the source of truth.
const MsgMapItemsChanged MessageType = "map.items.changed"

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

// Relation sync messages, one per relation row written. ResourceID is the
// row's source entity, so a client can follow one entity's relations (a
// character's inventory) and the change feed collapses them per entity.
const (
	MsgRelationCreated         MessageType = "relation.created"
	MsgRelationDeleted         MessageType = "relation.deleted"
	MsgRelationMetadataUpdated MessageType = "relation.metadata_updated"
)

// Note sync messages.
const (
	MsgNoteCreated MessageType = "note.created"
	MsgNoteUpdated MessageType = "note.updated"
	MsgNoteDeleted MessageType = "note.deleted"
)

// isNoteMessage reports whether t is one of the note.* events above.
func isNoteMessage(t MessageType) bool {
	return t == MsgNoteCreated || t == MsgNoteUpdated || t == MsgNoteDeleted
}

// Entity notes sync messages (player-notes addon). Distinct from note.*
// because the data model + audience semantics are different — these
// fire from internal/widgets/entity_notes, those from internal/widgets/notes.
const (
	MsgEntityNoteCreated MessageType = "entity_note.created"
	MsgEntityNoteUpdated MessageType = "entity_note.updated"
	MsgEntityNoteDeleted MessageType = "entity_note.deleted"
)

// Stash and downtime messages. Payloads carry ids and statuses only.
const (
	MsgStashMoved        MessageType = "stash.moved"
	MsgStashRequested    MessageType = "stash.requested"
	MsgStashSettled      MessageType = "stash.settled"
	MsgStashMoneyChanged MessageType = "stash.money_changed"
	MsgDowntimeChanged   MessageType = "downtime.changed"
)

// Foundry table messages. Sent to the campaign's Foundry module only to
// act at the table; never recorded in the sync change feed.
const (
	// MsgNPCSpotlight asks Foundry to spotlight the NPC page's token.
	// ResourceID is the entity id; always published RequiresDM.
	MsgNPCSpotlight MessageType = "npc.spotlight"

	// MsgSystemStateUpdated tells the GM side that a page's per-system
	// state changed. ResourceID is the entity id; the payload names the
	// system and key only, never the state. Always published RequiresDM
	// and deliberately not a change-feed type: the state is read through
	// its own route, not replayed from the feed.
	MsgSystemStateUpdated MessageType = "system_state.updated"
)

// Quest messages. They carry ids only (and a quest's version), so a page
// reading them fetches again through its own route; not change-feed types.
const (
	// MsgQuestUpdated says a quest sheet was saved. ResourceID is the page id.
	MsgQuestUpdated MessageType = "quest.updated"
	// MsgNoticeBoardsUpdated says a home's notice boards changed. ResourceID
	// is the page id, or the category id for a category's boards.
	MsgNoticeBoardsUpdated MessageType = "notice_boards.updated"
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

	// Seq is the change-feed sequence number, stamped by the recording bus for
	// allowlisted types. A client that tracks the highest seq it has seen can
	// resume from GET /sync/changes?since=<seq> after a dropped connection.
	Seq int64 `json:"seq,omitempty"`

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

	// StrictAudience makes AllowedUsers/DeniedUsers bind DM-equivalent
	// clients too, instead of letting them bypass. For content that is
	// private to named people — a player's private journal note — where "the
	// GM sees everything" does not hold. Set only with a non-empty
	// AllowedUsers: an empty allowlist still means everyone.
	StrictAudience bool `json:"-"`
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
