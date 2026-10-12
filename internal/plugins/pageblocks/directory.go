package pageblocks

import "context"

// EntityDirectory is owned by this plugin and implemented in internal/app over
// the entities service, so pageblocks never imports another plugin (plugin
// isolation).
type EntityDirectory interface {
	// Access answers what the viewer may do with a page, using the same
	// rules as the page itself. A page that is missing or in another
	// campaign comes back with Exists false.
	Access(ctx context.Context, campaignID, entityID string, v Viewer) (PageAccess, error)
}

// PageAccess is one viewer's standing on one page.
type PageAccess struct {
	Exists  bool
	CanView bool
	// CanEdit is the page's own edit rule: a Scribe or above who may edit
	// this page.
	CanEdit bool
}
