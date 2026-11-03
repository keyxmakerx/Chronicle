package transformations

import "context"

// PageDirectory is owned by this plugin and implemented in internal/app over
// the entities service, so this plugin never imports another one.
type PageDirectory interface {
	// Access answers what the viewer may do with a page, using the page's
	// own rules. A page that is missing or in another campaign comes back
	// with Exists false.
	Access(ctx context.Context, campaignID, entityID string, v Viewer) (PageAccess, error)
}

// PageAccess is one viewer's standing on one page.
type PageAccess struct {
	Exists  bool
	CanView bool
	CanEdit bool
}
