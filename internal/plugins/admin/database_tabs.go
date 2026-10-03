package admin

// Tabs of the Database page. They appear as entries of the shared Health &
// diagnostics strip (components.AdminTabsHealth), whose hrefs must match
// databaseTabHref.
const (
	DatabaseTabMigrations = "migrations"
	DatabaseTabHealth     = "health"
	DatabaseTabSchema     = "schema"
)

// normalizeDatabaseTab whitelists ?tab=; unknown values (including the retired
// "backups") fall back to the updates tab so a stale link still lands on a page.
func normalizeDatabaseTab(raw string) string {
	switch raw {
	case DatabaseTabHealth, DatabaseTabSchema:
		return raw
	default:
		return DatabaseTabMigrations
	}
}

// databaseTabHref is the strip href for a tab; it is what the page passes as
// "current" so the strip marks the right entry.
func databaseTabHref(tab string) string {
	return "/admin/database?tab=" + normalizeDatabaseTab(tab)
}
