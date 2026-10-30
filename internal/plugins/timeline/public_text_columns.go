package timeline

import "github.com/keyxmakerx/chronicle/internal/database"

// PublicTextColumns lists this plugin's stored rich text that is served as
// stored to everyone who can see its timeline or event, for the boot-time
// reconciler that removes GM-only parts.
func PublicTextColumns() []database.PublicTextColumn {
	return []database.PublicTextColumn{
		{Table: "timelines", Column: "description", JSON: true},
		{Table: "timelines", Column: "description_html"},
		{Table: "timeline_events", Column: "description", JSON: true},
		{Table: "timeline_events", Column: "description_html"},
	}
}
