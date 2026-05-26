// helpers.go — packages plugin helpers exposed to packages.templ.
//
// Per cordinator/decisions/2026-05-23-packages-treatment.md (NW-2.2
// Chunk G), the per-row admin UI for a package type is rendered via
// an HTMX lazy-load fragment owned by the type's plugin. This file
// holds the type→URL dispatch.
//
// The owning-plugin slug appears as a URL-path literal in
// actionsFragmentURLFor — same kind of URL-path reference that
// already exists at packages.templ:49 (the autopin-banner hx-get).
// The plugin-isolation grep guard's regex requires a closing quote
// immediately after the slug to flag a violation, which URL paths
// don't trip. A future "per-type UI registry" interface would
// decouple this entirely; deferred to a follow-up.

package packages

// actionsFragmentURLFor returns the URL of the per-row actions
// fragment for the given package's type, or "" if the type has no
// type-specific fragment. packages.templ calls this when rendering
// each row's button group to know whether to insert an hx-get slot.
//
// Today only foundry-module packages have a type-specific fragment;
// system packages render no extra actions beyond the generic
// Check/Versions/Usage/Delete buttons (Versions + Usage stay generic
// because the version-list rendering itself is generic — see Chunk G
// decision doc's "deferred to G2" section for the per-version foundry
// UI residual).
func actionsFragmentURLFor(pkg Package) string {
	switch pkg.Type {
	case PackageTypeFoundryModule:
		return "/admin/foundry-vtt/packages/" + pkg.ID + "/actions-fragment"
	case PackageTypeSystem:
		return ""
	default:
		return ""
	}
}
