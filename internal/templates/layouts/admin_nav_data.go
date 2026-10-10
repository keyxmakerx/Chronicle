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
		// Everything an admin adds to the site, in one group.
		{ID: "packages", Label: "Add-ons", Items: []AdminNavItem{
			{Label: "Game systems & modules", Href: "/admin/packages", Icon: "fa-box",
				Excludes: []string{"/admin/packages/settings"}},
			{Label: "Features", Href: "/admin/addons", Icon: "fa-plug"},
			{Label: "Extensions", Href: "/admin/extensions", Icon: "fa-puzzle-piece"},
			{Label: "Package rules", Href: "/admin/packages/settings", Icon: "fa-scale-balanced"},
		}},
		{ID: "security", Label: "Security", Items: []AdminNavItem{
			{Label: "Sign-ins & sessions", Href: "/admin/security", Icon: "fa-shield-halved"},
			{Label: "API & access", Href: "/admin/api", Icon: "fa-satellite-dish"},
			{Label: "Admin activity", Href: "/admin/activity", Icon: "fa-clock-rotate-left"},
		}},
		{ID: "site", Label: "Site & data", Items: []AdminNavItem{
			// Storage limits and clean-up are tabs on the storage pages.
			{Label: "Storage & cleanup", Href: "/admin/storage", Icon: "fa-hard-drive",
				Prefixes: []string{"/admin/data-hygiene"}},
			// Backup and restore are separate plugins sharing one slot, with a
			// tab strip on each page to flip between them.
			{Label: "Backups & restore", Href: "/admin/backup", Icon: "fa-box-archive",
				Prefixes: []string{"/admin/restore"}},
			{Label: "Email", Href: "/admin/smtp", Icon: "fa-envelope"},
			{Label: "Site look", Href: "/admin/site-look", Icon: "fa-palette"},
		}},
		{ID: "tools", Label: "Tools", Items: []AdminNavItem{
			// Parts of Chronicle, Database and the AI helper are tabs on one
			// area; the degraded-plugin count rides on the row so it is seen
			// without opening the Database tab.
			{Label: "Health & diagnostics", Href: "/admin/systems", Icon: "fa-microchip",
				Prefixes: []string{"/admin/database", "/admin/diagnostics"}, Badge: adminBadgeDegraded},
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

// AdminNavPinnableHrefs lists the item links an admin may pin, in menu order.
// Home is left out: it is always one click away at the top.
func AdminNavPinnableHrefs() []string {
	var out []string
	for _, s := range adminNav.Sections {
		for _, it := range s.Items {
			out = append(out, it.Href)
		}
	}
	return out
}

// adminNavPinnedItems resolves stored pin links to menu items in pin order.
// A link that no longer names an item (the page was renamed) is skipped, so a
// stale pin never renders a dead row.
func adminNavPinnedItems(pins []string) []AdminNavItem {
	byHref := map[string]AdminNavItem{}
	for _, s := range adminNav.Sections {
		for _, it := range s.Items {
			byHref[it.Href] = it
		}
	}
	var out []AdminNavItem
	seen := map[string]bool{}
	for _, h := range pins {
		if it, ok := byHref[h]; ok && !seen[h] {
			seen[h] = true
			out = append(out, it)
		}
	}
	return out
}

// adminNavIsPinned reports whether href is among the admin's pins.
func adminNavIsPinned(pins []string, href string) bool {
	for _, p := range pins {
		if p == href {
			return true
		}
	}
	return false
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
		// pinToggle saves the admin's pins and swaps in the re-rendered nav.
		// The list is read from the page at click time (every pinned item
		// shows a pressed toggle), so a stale copy never overwrites a newer
		// one. Alpine and htmx pick up the new block on their own.
		pinToggle(b) {
			if (b.disabled) return;
			var href = b.dataset.adminPin, on = b.getAttribute('aria-pressed') !== 'true';
			var root = document.getElementById('admin-nav');
			var pins = [].slice.call(root.querySelectorAll('[data-admin-pin][aria-pressed="true"]'))
				.map(function (x) { return x.dataset.adminPin; })
				.filter(function (k) { return k !== href; });
			if (on) pins.push(href);
			b.disabled = true;
			Chronicle.apiFetch('/admin/nav/pins', { method: 'PUT', body: { pins: pins } })
				.then(function (r) {
					if (r.ok) return r.text();
					return r.json().catch(function () { return {}; }).then(function (j) { throw { m: j.message || j.error }; });
				})
				.then(function (h) {
					var t = document.createElement('template');
					t.innerHTML = h.trim();
					var n = t.content.firstElementChild;
					if (!n) { window.location.reload(); return; }
					root.replaceWith(n);
					if (window.htmx) window.htmx.process(n);
				})
				.catch(function (e) {
					b.disabled = false;
					Chronicle.notify((e && e.m) || 'That page could not be pinned. Check your connection and try again.', 'error');
				});
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
