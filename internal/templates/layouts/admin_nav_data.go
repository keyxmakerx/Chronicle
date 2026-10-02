package layouts

import (
	"context"
	"strings"
)

// admin_nav_data.go declares the admin sidebar as data so the template only
// renders it and later work (per-admin pins) can extend the same slice. The
// active-page rule lives on each item rather than in the template so it can be
// unit-tested against plain paths.

// adminBadge names which attention count an item carries. Kept as a closed set
// so the template never evaluates arbitrary expressions.
type adminBadge string

const (
	adminBadgeNone     adminBadge = ""
	adminBadgeDegraded adminBadge = "degraded" // plugins that failed to start
)

// AdminNavItem is one admin page link and the rule for when it is current.
type AdminNavItem struct {
	Label string
	Href  string
	Icon  string // Font Awesome class, without the "fa-solid" prefix
	// Exact limits the match to Href itself; /admin must not light up for
	// every admin page beneath it.
	Exact bool
	// Prefixes are further path roots that also make this item current,
	// for pages that share a sidebar slot (restore lives under Backups).
	Prefixes []string
	// Excludes are path roots that belong to a different item even though
	// they sit under Href (package rules live under /admin/packages).
	Excludes []string
	Badge    adminBadge
}

// Active reports whether path is a page this item stands for.
func (it AdminNavItem) Active(path string) bool {
	if path == it.Href {
		return true
	}
	if it.Exact {
		return false
	}
	for _, ex := range it.Excludes {
		if adminPathUnder(path, ex) {
			return false
		}
	}
	if adminPathUnder(path, it.Href) {
		return true
	}
	for _, p := range it.Prefixes {
		if adminPathUnder(path, p) {
			return true
		}
	}
	return false
}

// adminPathUnder matches the root itself or anything below it, but not a
// sibling that merely shares leading characters (/admin/api vs /admin/apiary).
func adminPathUnder(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}

// AdminNavSection is a foldable group of admin items.
type AdminNavSection struct {
	ID    string // stable: it keys the remembered fold state
	Label string
	Items []AdminNavItem
}

// AdminNavTree is the whole admin block: a standalone Home link and the
// foldable sections.
type AdminNavTree struct {
	Home     AdminNavItem
	Sections []AdminNavSection
}

// adminNav is the admin sidebar. Every Href is a GET page that renders full
// HTML for a browser request.
var adminNav = AdminNavTree{
	Home: AdminNavItem{Label: "Home", Href: "/admin", Icon: "fa-gauge", Exact: true},
	Sections: []AdminNavSection{
		{ID: "community", Label: "Community", Items: []AdminNavItem{
			{Label: "People", Href: "/admin/users", Icon: "fa-users"},
			{Label: "Campaigns", Href: "/admin/campaigns", Icon: "fa-book-open"},
		}},
		{ID: "packages", Label: "Packages", Items: []AdminNavItem{
			{Label: "Game systems & modules", Href: "/admin/packages", Icon: "fa-box",
				Excludes: []string{"/admin/packages/settings"}},
			{Label: "Package rules", Href: "/admin/packages/settings", Icon: "fa-scale-balanced"},
		}},
		{ID: "apps", Label: "Apps & extensions", Items: []AdminNavItem{
			{Label: "Features", Href: "/admin/addons", Icon: "fa-plug"},
			{Label: "Extensions", Href: "/admin/extensions", Icon: "fa-puzzle-piece"},
		}},
		{ID: "security", Label: "Security", Items: []AdminNavItem{
			{Label: "Sign-ins & sessions", Href: "/admin/security", Icon: "fa-shield-halved"},
			{Label: "API & access", Href: "/admin/api", Icon: "fa-satellite-dish"},
		}},
		{ID: "site", Label: "Site & data", Items: []AdminNavItem{
			{Label: "Storage", Href: "/admin/storage", Icon: "fa-hard-drive"},
			{Label: "Data clean-up", Href: "/admin/data-hygiene", Icon: "fa-broom"},
			// Backup and restore are separate plugins sharing one slot, with a
			// tab strip on each page to flip between them.
			{Label: "Backups & restore", Href: "/admin/backup", Icon: "fa-box-archive",
				Prefixes: []string{"/admin/restore"}},
			{Label: "Email", Href: "/admin/smtp", Icon: "fa-envelope"},
		}},
		{ID: "tools", Label: "Tools", Items: []AdminNavItem{
			{Label: "Health", Href: "/admin/systems", Icon: "fa-microchip"},
			{Label: "Database", Href: "/admin/database", Icon: "fa-database", Badge: adminBadgeDegraded},
			{Label: "AI diagnostics", Href: "/admin/diagnostics/workspace", Icon: "fa-stethoscope"},
			{Label: "Design Lab", Href: "/admin/design-lab", Icon: "fa-palette"},
		}},
	},
}

// Current returns the section ID and item Href current for path. The
// section is "" for Home, and both are "" when nothing matches. The first
// matching item wins, so item order is the tie-break.
func (t AdminNavTree) Current(path string) (sectionID, href string) {
	if t.Home.Active(path) {
		return "", t.Home.Href
	}
	for _, s := range t.Sections {
		for _, it := range s.Items {
			if it.Active(path) {
				return s.ID, it.Href
			}
		}
	}
	return "", ""
}

// adminNavBadgeCount is the attention count an item carries right now.
func adminNavBadgeCount(ctx context.Context, it AdminNavItem) int {
	switch it.Badge {
	case adminBadgeDegraded:
		return GetDegradedPluginCount(ctx)
	}
	return 0
}

// adminNavSectionCount sums the item badges so a folded section still shows
// that something inside needs attention.
func adminNavSectionCount(ctx context.Context, s AdminNavSection) int {
	n := 0
	for _, it := range s.Items {
		n += adminNavBadgeCount(ctx, it)
	}
	return n
}

// adminNavXData is the Alpine state for the admin block. It is a constant so
// no server value is ever spliced into script; the current section arrives in
// a data-cur attribute that templ escapes. The section holding the open page
// is forced open without being saved, so browsing does not overwrite the
// viewer's own choices; only an explicit toggle is remembered. Stored values
// are parsed defensively since they are client-written.
const adminNavXData = `{
		open: localStorage.getItem('chronicle-admin-nav') !== 'collapsed',
		saved: {},
		secs: {},
		init() {
			try {
				var s = JSON.parse(localStorage.getItem('chronicle-admin-nav-sections') || '{}');
				if (s && typeof s === 'object' && !Array.isArray(s)) { this.saved = s; }
			} catch (e) {}
			this.secs = Object.assign({}, this.saved);
			var cur = this.$el.dataset.cur;
			if (cur) { this.secs[cur] = true; }
		},
		toggle(id) {
			this.secs[id] = !this.secs[id];
			this.saved[id] = this.secs[id];
			try { localStorage.setItem('chronicle-admin-nav-sections', JSON.stringify(this.saved)); } catch (e) {}
		}
	}`

// adminNavSectionXData scopes one section: it reads its own ID from data-sec
// so the expressions on the fold head stay constant strings.
const adminNavSectionXData = `{ id: '', init() { this.id = this.$el.dataset.sec; } }`
