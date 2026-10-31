package sessions

import "github.com/keyxmakerx/chronicle/internal/database"

// PublicTextColumns lists the recap, which every member reads, for the
// boot-time reconciler that removes GM-only parts saved before
// UpdateSessionRecap stripped them.
func PublicTextColumns() []database.PublicTextColumn {
	return []database.PublicTextColumn{
		{Table: "sessions", Column: "recap", JSON: true},
		{Table: "sessions", Column: "recap_html"},
	}
}
