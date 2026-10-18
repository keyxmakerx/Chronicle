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

	// FilterViewable returns the pages among ids that belong to campaignID and
	// that v may see, keyed by id. A missing, foreign or hidden id is simply
	// absent, so the two cases cannot be told apart.
	FilterViewable(ctx context.Context, campaignID string, ids []string, v Viewer) (map[string]PageLink, error)
	// FindByName finds a page by the name it is written with, among the pages
	// v may see. ok is false for an unknown name and for a hidden page alike.
	FindByName(ctx context.Context, campaignID, name string, v Viewer) (link PageLink, ok bool, err error)
	// FirstPageMapBlock returns the id of the first page_map block in the
	// layout of the page's category. ok is false when the page is missing, in
	// another campaign, or its layout has none.
	FirstPageMapBlock(ctx context.Context, campaignID, entityID string) (blockID string, ok bool, err error)
}

// PageAccess is one viewer's standing on one page.
type PageAccess struct {
	Exists  bool
	CanView bool
	// CanEdit is the page's own edit rule: a Scribe or above who may edit
	// this page.
	CanEdit bool
}

// PageLink is a page a viewer may open, as a marker links to it.
type PageLink struct {
	ID   string
	Name string
	// URL is the page's path, for the hover card and its link.
	URL string
}
