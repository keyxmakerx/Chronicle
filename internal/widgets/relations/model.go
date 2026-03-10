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

import (
	"context"
	"encoding/json"
	"time"
)

// Relation represents a single directional link between two entities within
// a campaign. The service layer always creates relations in pairs (forward
// and reverse) to maintain bi-directional consistency.
type Relation struct {
	ID                  int       `json:"id"`
	CampaignID          string    `json:"campaignId"`
	SourceEntityID      string    `json:"sourceEntityId"`
	TargetEntityID      string    `json:"targetEntityId"`
	RelationType        string    `json:"relationType"`
	ReverseRelationType string          `json:"reverseRelationType"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	DmOnly              bool            `json:"dmOnly"`
	CreatedAt           time.Time       `json:"createdAt"`
	CreatedBy           string          `json:"createdBy"`

	// Joined fields from the entities table (populated by repository queries).
	TargetEntityName  string `json:"targetEntityName,omitempty"`
	TargetEntityIcon  string `json:"targetEntityIcon,omitempty"`
	TargetEntityColor string `json:"targetEntityColor,omitempty"`
	TargetEntitySlug  string `json:"targetEntitySlug,omitempty"`
	TargetEntityType  string `json:"targetEntityType,omitempty"`
}

// CampaignRelation holds a relation row with joined entity details for both
// source and target entities. Used by the relations graph to build nodes and
// edges without additional queries.
type CampaignRelation struct {
	SourceEntityID    string `json:"sourceEntityId"`
	SourceEntityName  string `json:"sourceEntityName"`
	SourceEntityIcon  string `json:"sourceEntityIcon"`
	SourceEntityColor string `json:"sourceEntityColor"`
	SourceEntitySlug  string `json:"sourceEntitySlug"`
	SourceEntityType  string `json:"sourceEntityType"`
	TargetEntityID    string `json:"targetEntityId"`
	TargetEntityName  string `json:"targetEntityName"`
	TargetEntityIcon  string `json:"targetEntityIcon"`
	TargetEntityColor string `json:"targetEntityColor"`
	TargetEntitySlug  string `json:"targetEntitySlug"`
	TargetEntityType  string `json:"targetEntityType"`
	RelationType      string `json:"relationType"`
}

// --- Request DTOs (bound from HTTP requests) ---

// CreateRelationRequest holds the data submitted when creating a new relation.
// The reverse relation is created automatically by the service layer.
type CreateRelationRequest struct {
	TargetEntityID      string          `json:"targetEntityId"`
	RelationType        string          `json:"relationType"`
	ReverseRelationType string          `json:"reverseRelationType"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	DmOnly              bool            `json:"dmOnly"`
}

// UpdateRelationMetadataRequest holds the data for updating relation metadata.
type UpdateRelationMetadataRequest struct {
	Metadata json.RawMessage `json:"metadata"`
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
	{Forward: "sells", Reverse: "sold by"},
	{Forward: "sold by", Reverse: "sells"},
}

// RelationTypePair holds a forward and reverse relation type label.
// Used by the frontend to suggest relation types and auto-fill the reverse.
type RelationTypePair struct {
	Forward string `json:"forward"`
	Reverse string `json:"reverse"`
}

// --- Graph Visualization ---

// GraphRelation is a flattened relation with both source and target entity
// details for the relations graph visualization. Only forward relations are
// included (not reverse duplicates).
type GraphRelation struct {
	SourceEntityID    string `json:"source_entity_id"`
	TargetEntityID    string `json:"target_entity_id"`
	RelationType      string `json:"relation_type"`
	DmOnly            bool   `json:"dm_only"`
	SourceEntityName  string `json:"source_name"`
	SourceEntityIcon  string `json:"source_icon"`
	SourceEntityColor string `json:"source_color"`
	SourceEntitySlug  string `json:"source_slug"`
	SourceEntityType  string `json:"source_type"`
	TargetEntityName  string `json:"target_name"`
	TargetEntityIcon  string `json:"target_icon"`
	TargetEntityColor string `json:"target_color"`
	TargetEntitySlug  string `json:"target_slug"`
	TargetEntityType  string `json:"target_type"`
}

// GraphNode represents an entity in the relations graph.
type GraphNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Icon   string `json:"icon"`
	Color  string `json:"color"`
	Slug   string `json:"slug"`
	Type   string `json:"type"`
	Orphan bool   `json:"orphan,omitempty"` // True if entity has no connections.
}

// GraphEdge represents a relation or mention link in the relations graph.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
	Kind   string `json:"kind"` // "relation" (default) or "mention"
}

// MentionLinkData holds a directional @mention reference for the graph.
// Defined here to avoid importing the entities package.
type MentionLinkData struct {
	SourceEntityID string
	TargetEntityID string
}

// MentionLinkProvider supplies @mention link data for the graph visualization.
// Implemented by the entities service.
type MentionLinkProvider interface {
	GetMentionLinksForGraph(ctx context.Context, campaignID string, includeDmOnly bool, userID string) ([]MentionLinkData, error)
}

// EntityTypeSummary holds minimal entity type info for the graph filter UI.
type EntityTypeSummary struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

// EntityTypeListerForGraph provides entity type listings for the graph page.
// Implemented by an adapter wrapping the entity service.
type EntityTypeListerForGraph interface {
	ListEntityTypesForGraph(ctx context.Context, campaignID string) ([]EntityTypeSummary, error)
}

// GraphData is the JSON response for the relations graph API.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphFilter holds query parameters for filtering the relations graph.
type GraphFilter struct {
	Types           []string // Filter nodes by entity type slugs.
	Search          string   // Filter nodes whose name matches (case-insensitive).
	FocusEntityID   string   // Ego-graph center: show only N hops from this entity.
	Hops            int      // Number of hops from focus entity (default 2).
	IncludeMentions bool     // Include @mention edges alongside explicit relations.
	IncludeOrphans  bool     // Include entities with zero connections.
}
