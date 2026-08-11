package layouts

// addon_paths.go exposes the sidebar's addon destinations to code that must
// report where the navigation actually points.
//
// WHY AN ACCESSOR AND NOT A COPY. `addonURLMap` (app.templ) is the single map
// `sidebarAddonLink` renders every addon href from. The operator diagnostic
// `campaign.surfaces` has to answer "where does the sidebar's Calendar item
// link?", and the one answer that cannot go stale is the map itself — a second
// copy of "/apps/calendar" in a diagnostic would keep reporting the old
// destination for exactly as long as it took someone to notice, which on this
// question is indefinitely. So the map stays where the template reads it and
// this returns from it.

// AddonSidebarPath returns the campaign-relative path the sidebar links an
// addon to (e.g. "calendar" → "/apps/calendar"), or "" for a slug with no
// sidebar destination. The caller prefixes "/campaigns/<id>", exactly as
// sidebarAddonLink does.
func AddonSidebarPath(slug string) string { return addonURLMap[slug] }
