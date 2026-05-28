// Package calendar_v2 provides reusable widget components shared by
// the calendar plugin's V2 surface and any downstream consumer
// (Chronicle-DnD-5.5e, Chronicle-Draw-Steel, AI Workspace event
// imports). Established by Wave 1 PR 4 per the template-first
// directive in decisions/2026-05-28-cal-timeline-v2-design.md
// §"Wave 1 PR 4 template-first directive — reusable widget layer".
//
// Components:
//   - EventCard      → templ component for a single event (3 densities)
//   - MultiDayRibbon → templ component for events spanning multiple days
//   - VisibilityEditor → templ component for chip-row visibility editing (Q-V2-7)
//
// CSS animation classes live in static/css/input.css under the
// "/* --- V2 widget animations (Wave 1 PR 4) --- */" section. Each
// pattern has a `@media (prefers-reduced-motion: reduce)` override.
//
// All values consume tokens from PR #357 — zero hardcoded color,
// shadow, or motion constants.
package calendar_v2

// Density controls how much detail an EventCard renders.
//
//   - DensityCompact:  cell-inline; just `● Event Name` (Month-view single-day cells)
//   - DensityStandard: drawer/list; metadata + category chip + visibility icon
//   - DensityDetailed: full body; markdown description + visibility editor section
type Density string

const (
	DensityCompact  Density = "compact"
	DensityStandard Density = "standard"
	DensityDetailed Density = "detailed"
)

// Tier controls visual prominence of an event. The three-tier model
// (Minor / Standard / Major) per V2 design §C1. PR 4 uses platform-
// default treatment per tier; PR 5 wires campaign-aware tier
// definitions from PR #358's `event_tier_definitions`.
type Tier string

const (
	TierMinor    Tier = "minor"
	TierStandard Tier = "standard"
	TierMajor    Tier = "major"
)

// EventCardData is the rendering payload for the EventCard widget.
// Kept widget-package-local so downstream consumers can populate it
// from any source (calendar.Event today; AI Workspace import-preview
// rows tomorrow; etc.). No imports into the calendar plugin.
type EventCardData struct {
	ID             string
	Name           string
	CategoryColor  string // hex; empty for no left bar
	CategoryName   string // shown as chip in Standard+ density
	Tier           Tier   // defaults to TierStandard if empty
	IsPublic       bool   // controls visibility lock icon
	DescriptionHTML string // sanitized HTML; rendered in Detailed density
	StartLabel     string // e.g. "Mirtul 15"
	TimeLabel      string // e.g. "14:00 — 16:00" (optional)
}

// tierClasses returns the Tailwind classes for an event-card's tier
// treatment. Major gets elevation +1 + bolder border; standard keeps
// default; minor goes to 60% opacity + flat.
func tierClasses(t Tier) string {
	switch t {
	case TierMajor:
		return "card card-elev border-l-4 border-accent ring-1 ring-accent/30"
	case TierMinor:
		return "card opacity-60"
	default:
		return "card card-elev border-l-4 border-edge"
	}
}

// tierBadge returns the small badge character for a tier. Major shows
// a filled diamond; standard a hollow one (subtle); minor nothing.
func tierBadge(t Tier) string {
	switch t {
	case TierMajor:
		return "◆"
	case TierStandard:
		return "◇"
	}
	return ""
}

// VisibilityRule represents one entry in a visibility allow/deny list.
// The chip-row builder per Q-V2-7 lock surfaces these as rule chips
// that operators add/remove inline.
type VisibilityRule struct {
	Mode   string // "allow" | "deny"
	Kind   string // "user" | "role"
	Target string // user_id or role name
	Label  string // human-readable display
}

// VisibilityEditorData is the rendering payload for the chip-row
// visibility editor. Locked design per Q-V2-7 in
// decisions/2026-05-28-cal-timeline-v2-design.md.
type VisibilityEditorData struct {
	// IsPublic toggles between "Everyone (public)" and "Specific
	// people"; when false, the rule chip-row + add affordances render.
	IsPublic bool
	// Rules is the current allow/deny rule set. Renders as chips.
	Rules []VisibilityRule
	// AvailableUsers + AvailableRoles populate the inline pickers.
	AvailableUsers []UserOption
	AvailableRoles []RoleOption
	// FieldPrefix lets a host form scope the editor's input names
	// (e.g. "event.visibility" vs "entity.visibility"). The hidden
	// input that round-trips the rules JSON uses this prefix.
	FieldPrefix string
}

// UserOption + RoleOption are the picker entries. Kept minimal to
// keep the widget's API surface narrow.
type UserOption struct {
	ID    string
	Label string
}

type RoleOption struct {
	Name  string
	Label string
}
