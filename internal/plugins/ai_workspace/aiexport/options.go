// Package aiexport renders a campaign's owner-scoped content into a
// single markdown document suitable for pasting into AI tools (Claude,
// ChatGPT, NotebookLM, etc). The package is intentionally lossy —
// markdown is the wire format owners paste into chat, not a backup.
//
// Scope (operationalises decisions/2026-05-26-ai-export-pipeline-design.md
// + reports/chronicle/2026-05-26-c-ai-export-scoping.md):
//
//   - Five categories: entities (grouped by entity type), notes (folder-
//     aware), calendar events (grouped by month using the calendar's own
//     month names), sessions (with attendees + linked entities + opt-in
//     GM notes), timeline events (grouped by parent timeline).
//   - Privacy modes: Safe (drops dm_only / IsPrivate / not-shared-with-
//     owner), Permitted (matches owner's on-screen view), Everything
//     (no visibility filtering).
//   - Token estimate rendered in the header so the owner can tell at
//     a glance whether the export fits in a target AI's context window.
//
// SEC-6-AMENDED invariant: every HTML field passes through
// sanitize.HTMLPtr BEFORE the HTML-to-markdown converter sees it. The
// renderer must never feed raw EntryHTML / NotesHTML / DescriptionHTML
// from the DB straight into the converter — historical or tooling-
// inserted rows could carry <script> or javascript: URLs that the
// converter would faithfully translate. AST structural pin in
// renderer_test.go enforces every renderer body contains the call.
//
// Scope-OUT (D4=(c) backup carve-out): zero edits to
// internal/app/export_adapters.go, internal/plugins/campaigns/
// export_handler.go, or internal/plugins/restore/. AI-export is a
// SEPARATE egress surface — the lossless backup pipeline remains the
// source of truth for round-trip-safe exports.
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

// AllCategories is the v1 set, in render order. Maps + media + tags-
// as-standalone-category are deferred to v2 per the scoping report
// §1 ("Categories surfaced during audit but recommended OUT of v1").
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

// Options bundles every owner-controlled toggle for a single Generate
// call. Constructed by the campaigns settings handler in PR-B; this
// package only consumes the struct.
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
