// Package calendar — entity_ties.go holds the optional many-to-many ties
// between entities and calendar events/eras: an entity can be, but does not
// have to be, tied to either. Both directions carry a participation role;
// event ties always have one (an event tie is always "someone's involvement
// in something"), era ties are coarser and may carry none.
package calendar

// ParticipationRole is an entity's role in an event/era tie.
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
// truth a future validator and role picker both read from.
var ParticipationRoles = []ParticipationRole{RoleInvolved, RolePresent, RoleAffected, RoleMentioned}

// IsValid reports whether r is one of the four roles.
func (r ParticipationRole) IsValid() bool {
	for _, v := range ParticipationRoles {
		if r == v {
			return true
		}
	}
	return false
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
// These carry the joined display fields so a caller can render a tie without
// a second lookup across the plugin boundary.

// EntityTieRef is an entity as seen from an event/era (event/era-side query):
// the entity's display info + the role of the tie. Type/Icon/Color come from
// the entity's entity_type, the same join the event list already uses.
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
