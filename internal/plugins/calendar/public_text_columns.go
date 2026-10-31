package calendar

import "github.com/keyxmakerx/chronicle/internal/database"

// PublicTextColumns lists this plugin's stored rich text that is served as
// stored to everyone who can see its event, for the boot-time reconciler
// that removes GM-only parts. An event's own visibility is what hides it.
func PublicTextColumns() []database.PublicTextColumn {
	return []database.PublicTextColumn{
		{Table: "calendar_events", Column: "description", JSON: true},
		{Table: "calendar_events", Column: "description_html"},
	}
}
