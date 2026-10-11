// Package aiexport renders a campaign's owner-scoped content into a single
// markdown document for pasting into AI tools. It is intentionally lossy —
// markdown is a paste format, not a backup; the lossless path is
// internal/app/export_adapters.go / campaigns/export_handler.go /
// internal/plugins/restore/.
//
// Categories: entities, notes, calendar events, sessions, timeline events.
// Privacy modes: Safe (drops dm_only/private/not-shared-with-owner),
// Permitted (owner's on-screen view), Everything (unfiltered).
//
// SECURITY: every HTML field must pass sanitize.HTMLPtr before the
// HTML-to-markdown converter sees it, or a raw DB field's <script>/
// javascript: URL gets faithfully translated. Pinned by the AST check in
// renderer_test.go.
package aiexport

// Category identifies one of the v1 markdown-rendered content
// categories. The owner toggles each on/off in the settings UI; the
// orchestrator skips disabled categories at Generate time.
type Category string

const (
	CategoryEntities       Category = "entities"
	CategoryNotes          Category = "notes"
	CategoryCalendarEvents Category = "calendar_events"
	CategorySessions       Category = "sessions"
	CategoryTimelines      Category = "timelines"
)

// AllCategories is the current set, in render order.
// TODO(keyxmakerx/Chronicle#740): maps, media, and tags-as-standalone
// category are out of scope until V2.
func AllCategories() []Category {
	return []Category{
		CategoryEntities,
		CategoryNotes,
		CategoryCalendarEvents,
		CategorySessions,
		CategoryTimelines,
	}
}

// PrivacyMode controls how visibility flags filter the export.
type PrivacyMode int

const (
	// PrivacyModeSafe (default) drops dm_only / IsPrivate / not-shared-
	// with-owner content. Suitable for "paste into Claude" workflows
	// where the owner wants the world, not the GM-side intel.
	PrivacyModeSafe PrivacyMode = iota

	// PrivacyModePermitted matches the owner's on-screen view —
	// Owner-role bypass on dm_only / IsPrivate is honored, but rows
	// the owner can't see via the permission model still drop. Session
	// GM notes (json:"-") are included since the owner IS the GM.
	PrivacyModePermitted

	// PrivacyModeEverything includes every row regardless of visibility
	// flags. Owner explicitly opted in via a confirm-understand checkbox.
	PrivacyModeEverything
)

// String returns the canonical name (used by tests + the UI).
func (m PrivacyMode) String() string {
	switch m {
	case PrivacyModeSafe:
		return "safe"
	case PrivacyModePermitted:
		return "permitted"
	case PrivacyModeEverything:
		return "everything"
	default:
		return "unknown"
	}
}

// Options bundles every owner-controlled toggle for a single Generate call.
// Constructed by the campaigns settings handler; this package only consumes
// the struct.
type Options struct {
	// Categories enumerates which categories to render. Empty slice
	// means "all" (AllCategories). The orchestrator deduplicates +
	// preserves the canonical render order regardless of input order.
	Categories []Category

	// Privacy is one of the three PrivacyMode values. Defaults to Safe
	// (the zero value) so a caller that forgets to set it gets the
	// most-restrictive behavior.
	Privacy PrivacyMode

	// IncludeSessionGMNotes opts the GM-only Notes / NotesHTML fields
	// into the session render. Only honored in PrivacyModePermitted /
	// PrivacyModeEverything; ignored in Safe.
	IncludeSessionGMNotes bool

	// ParentNames maps a page id to its name for parents the reader may
	// see but that are not themselves in the rendered set (a single-page
	// lookup). A parent in neither place is left unnamed, so a hidden
	// parent's name never reaches the AI.
	ParentNames map[string]string
}

// EnabledCategories returns the canonical render order, filtered to
// the Options.Categories selection (or all five when unspecified).
// Stable order — entities first so the wikilink resolver has the
// page table by the time later categories reference it.
func (o Options) EnabledCategories() []Category {
	if len(o.Categories) == 0 {
		return AllCategories()
	}
	set := make(map[Category]struct{}, len(o.Categories))
	for _, c := range o.Categories {
		set[c] = struct{}{}
	}
	out := make([]Category, 0, len(set))
	for _, c := range AllCategories() {
		if _, ok := set[c]; ok {
			out = append(out, c)
		}
	}
	return out
}
