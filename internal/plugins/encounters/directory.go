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

// ResultDirectory is how an encounter reads the tests that pick its
// versions. It is implemented in internal/app over the Tests block, so
// encounters never imports that plugin; it is nil when the Tests block is
// unavailable, and every pick then reads as unknown.
type ResultDirectory interface {
	// Outcomes returns, for the tests among ids the viewer may see, their
	// name, results (id to label) and recorded result id, keyed by id.
	Outcomes(ctx context.Context, campaignID string, ids []string, v Viewer) (map[string]TestOutcome, error)
}

// TestOutcome is one test as a version's pick needs it.
type TestOutcome struct {
	Name     string
	Results  map[string]string
	Recorded string
}
