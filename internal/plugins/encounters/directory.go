package encounters

import "context"

// PageDirectory is owned by this plugin and implemented in internal/app over
// the entities service, so encounters never imports another plugin.
type PageDirectory interface {
	// Access answers what the viewer may do with a page, using the page's
	// own rules. A page that is missing or in another campaign comes back
	// with Exists false.
	Access(ctx context.Context, campaignID, entityID string, v Viewer) (PageAccess, error)
	// Names returns the names of the pages among ids that are live pages of
	// campaignID the viewer may see, keyed by id. Every other id is absent.
	Names(ctx context.Context, campaignID string, ids []string, v Viewer) (map[string]string, error)
}

// PageAccess is one viewer's standing on one page.
type PageAccess struct {
	Exists  bool
	CanView bool
	CanEdit bool
}
