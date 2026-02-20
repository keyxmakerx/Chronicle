// Package relations implements the entity relations widget for Chronicle.
// Relations are bi-directional links between entities within a campaign.
// When a relation is created from entity A to entity B, the reverse relation
// is automatically created from B to A. This enables graph-like navigation
// between entities (e.g., "allied with", "parent of"/"child of").
//
// Relations are a Widget in Chronicle's three-tier extension architecture:
// they provide API endpoints for the frontend relations display component
// and are mounted on entity profile pages.
package relations

import "time"

// Relation represents a single directional link between two entities within
// a campaign. The service layer always creates relations in pairs (forward
// and reverse) to maintain bi-directional consistency.
type Relation struct {
	ID                  int       `json:"id"`
	CampaignID          string    `json:"campaignId"`
	SourceEntityID      string    `json:"sourceEntityId"`
	TargetEntityID      string    `json:"targetEntityId"`
	RelationType        string    `json:"relationType"`
	ReverseRelationType string    `json:"reverseRelationType"`
	CreatedAt           time.Time `json:"createdAt"`
	CreatedBy           string    `json:"createdBy"`

	// Joined fields from the entities table (populated by repository queries).
	TargetEntityName  string `json:"targetEntityName,omitempty"`
	TargetEntityIcon  string `json:"targetEntityIcon,omitempty"`
	TargetEntityColor string `json:"targetEntityColor,omitempty"`
	TargetEntitySlug  string `json:"targetEntitySlug,omitempty"`
	TargetEntityType  string `json:"targetEntityType,omitempty"`
}

// --- Request DTOs (bound from HTTP requests) ---

// CreateRelationRequest holds the data submitted when creating a new relation.
// The reverse relation is created automatically by the service layer.
type CreateRelationRequest struct {
	TargetEntityID      string `json:"targetEntityId"`
	RelationType        string `json:"relationType"`
	ReverseRelationType string `json:"reverseRelationType"`
}

// --- Common Relation Types ---

// CommonRelationTypes provides a set of predefined relation type pairs for
// the frontend UI. Each pair has a forward and reverse label. Symmetric
// relations use the same label in both directions.
var CommonRelationTypes = []RelationTypePair{
	{Forward: "allied with", Reverse: "allied with"},
	{Forward: "enemy of", Reverse: "enemy of"},
	{Forward: "parent of", Reverse: "child of"},
	{Forward: "child of", Reverse: "parent of"},
	{Forward: "member of", Reverse: "has member"},
	{Forward: "has member", Reverse: "member of"},
	{Forward: "located in", Reverse: "contains"},
	{Forward: "contains", Reverse: "located in"},
	{Forward: "owns", Reverse: "owned by"},
	{Forward: "owned by", Reverse: "owns"},
	{Forward: "serves", Reverse: "served by"},
	{Forward: "served by", Reverse: "serves"},
	{Forward: "rules", Reverse: "ruled by"},
	{Forward: "ruled by", Reverse: "rules"},
	{Forward: "knows", Reverse: "known by"},
}

// RelationTypePair holds a forward and reverse relation type label.
// Used by the frontend to suggest relation types and auto-fill the reverse.
type RelationTypePair struct {
	Forward string `json:"forward"`
	Reverse string `json:"reverse"`
}
