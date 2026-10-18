package trackers

import "context"

// EntityDirectory is owned by this plugin and implemented in internal/app
// over the entities service, so trackers never imports another plugin.
type EntityDirectory interface {
	// Access answers what the viewer may do with a page, using the page's own
	// rules. A page that is missing or in another campaign has Exists false.
	Access(ctx context.Context, campaignID, entityID string, v Viewer) (PageAccess, error)
	// Types lists the campaign's page categories, for "on every page of".
	Types(ctx context.Context, campaignID string) ([]TypeRef, error)
}

// PageAccess is one viewer's standing on one page.
type PageAccess struct {
	Exists  bool
	CanView bool
	// TypeID is the page's category, which decides the kinds a page gets
	// without anyone placing them.
	TypeID int
}

// GameClock reads today's in-game date, implemented in internal/app over
// the calendar service. It returns nil, nil when the campaign has no default
// calendar the viewer may see: a log line is still written, just undated.
type GameClock interface {
	Today(ctx context.Context, campaignID string, v Viewer) (*GameDate, error)
}
