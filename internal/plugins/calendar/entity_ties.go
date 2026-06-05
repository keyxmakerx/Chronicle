// entity_ties.go — C-CAL-ENTITY-TIES-DATA-MODEL.
//
// The real persistence behind Phase 1.5's mock attach-entity picker: optional
// many-to-many ties between entities and calendar events/eras, each carrying
// a participation role. Operator framing: "an entity can be but does not have
// to be tied to events and eras" — both sides are optional.
//
// Placement (migration 009) is OPTION (a): hard cross-plugin FKs to the core
// `entities` table + this plugin's calendar_events/calendar_eras, all ON
// DELETE CASCADE, so cascade-on-delete is DB-enforced. Go-level cross-plugin
// access (the entity-side UI in PR2) goes through a service interface, never
// a direct repo import (CLAUDE.md rule 8).
package calendar

import "github.com/keyxmakerx/chronicle/internal/apperror"

// ParticipationRole is the entity's role in an event/era tie. The vocabulary
// is PINNED to Phase 1.5's showcase attach-entity picker
// (static/js/cal-almanac.js __calParticipationRoles) — keep these four
// identical and in this order so the Phase-2 port swaps the mock picker onto
// this model with no translation. Changing the set means changing 1.5 too.
type ParticipationRole string

const (
	// RoleInvolved — an active participant in the event/era.
	RoleInvolved ParticipationRole = "involved"
	// RolePresent — physically present but not a driver.
	RolePresent ParticipationRole = "present"
	// RoleAffected — impacted by it without being present.
	RoleAffected ParticipationRole = "affected"
	// RoleMentioned — referenced only.
	RoleMentioned ParticipationRole = "mentioned"
)

// ParticipationRoles is the ordered, canonical set — the single source of
// truth shared by validation and (in PR2) the role dropdowns. MUST match the
// showcase enum exactly.
var ParticipationRoles = []ParticipationRole{RoleInvolved, RolePresent, RoleAffected, RoleMentioned}

// IsValid reports whether r is one of the four pinned roles.
func (r ParticipationRole) IsValid() bool {
	for _, v := range ParticipationRoles {
		if r == v {
			return true
		}
	}
	return false
}

// validateEventRole validates a required event-tie role (event ties always
// carry a role). Empty defaults to "involved" — the picker's default.
func validateEventRole(role string) (ParticipationRole, error) {
	if role == "" {
		return RoleInvolved, nil
	}
	pr := ParticipationRole(role)
	if !pr.IsValid() {
		return "", apperror.NewValidation(
			"participation_role must be one of: involved, present, affected, mentioned")
	}
	return pr, nil
}

// validateEraRole validates an optional era-tie role (era ties are coarser, so
// a nil/empty role is allowed → stored NULL). Returns nil when unset.
func validateEraRole(role *string) (*string, error) {
	if role == nil || *role == "" {
		return nil, nil
	}
	pr := ParticipationRole(*role)
	if !pr.IsValid() {
		return nil, apperror.NewValidation(
			"participation_role must be one of: involved, present, affected, mentioned")
	}
	s := string(pr)
	return &s, nil
}

// --- link rows ---

// EntityEventLink is one entity<->event tie row.
type EntityEventLink struct {
	ID                int    `json:"id"`
	EntityID          string `json:"entity_id"`
	EventID           string `json:"event_id"`
	ParticipationRole string `json:"participation_role"`
}

// EntityEraLink is one entity<->era tie row. ParticipationRole is nil when
// the tie carries no finer semantics.
type EntityEraLink struct {
	ID                int     `json:"id"`
	EntityID          string  `json:"entity_id"`
	EraID             int     `json:"era_id"`
	ParticipationRole *string `json:"participation_role,omitempty"`
}

// --- both-direction query result shapes ---
//
// These carry the joined display fields so a caller can render a tie without a
// second lookup across the plugin boundary.

// EntityTieRef is an entity as seen from an event/era (event/era-side query):
// the entity's display info + the role of the tie. Type/Icon/Color come from
// the entity's entity_type (same JOIN the event list already uses).
type EntityTieRef struct {
	EntityID          string  `json:"entity_id"`
	EntityName        string  `json:"entity_name"`
	EntityType        string  `json:"entity_type"` // entity_types.slug (e.g. "npc")
	EntityIcon        string  `json:"entity_icon"`
	EntityColor       string  `json:"entity_color"`
	ParticipationRole *string `json:"participation_role,omitempty"`
}

// EntityEventTie is an event as seen from an entity (entity-side query): the
// linked event + the role. The embedded Event carries date/name for display
// and linking.
type EntityEventTie struct {
	Event             Event  `json:"event"`
	ParticipationRole string `json:"participation_role"`
}

// EntityEraTie is an era as seen from an entity (entity-side query).
type EntityEraTie struct {
	Era               Era     `json:"era"`
	ParticipationRole *string `json:"participation_role,omitempty"`
}
