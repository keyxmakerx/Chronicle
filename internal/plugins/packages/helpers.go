// helpers.go — per-type UI hooks the packages page consults.
//
// The per-row admin UI for a package type is rendered via an HTMX lazy-load
// fragment owned by the type's own plugin. The packages plugin must not import
// that plugin, so the owner registers a hook at startup (internal/app) and the
// page builder resolves it per row.

package packages

// TypeUI is one package type's contribution to the packages page.
type TypeUI struct {
	// ActionsFragmentURL returns the URL of the type's per-row actions
	// fragment, or "" for no fragment. Nil means no fragment.
	ActionsFragmentURL func(pkg Package) string
}

// typeUIHooks maps a package type to its registered UI hook. Held on the
// Handler rather than in a package-level map so tests and a degraded boot
// never share state.
type typeUIHooks map[PackageType]TypeUI

// actionsFragmentURLFor returns the per-row actions fragment URL for the
// package's type, or "" when the type registered none (system packages render
// only the generic Check/Versions/Usage/Delete buttons).
func (hooks typeUIHooks) actionsFragmentURLFor(pkg Package) string {
	ui, ok := hooks[pkg.Type]
	if !ok || ui.ActionsFragmentURL == nil {
		return ""
	}
	return ui.ActionsFragmentURL(pkg)
}
