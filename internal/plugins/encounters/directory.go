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
	// PickerTypes returns the page types the builder offers: heroes from
	// the campaign's character list, creatures from its creature types and
	// NPC list. Sub-types are included.
	PickerTypes(ctx context.Context, campaignID string) (PickerTypes, error)
}

// PickerTypes is which page types the builder's pickers search, and the
// game system's stat blocks for creature pages.
type PickerTypes struct {
	Heroes     []int
	Creatures  []int
	StatBlocks []StatBlock
}

// StatBlock is a game-system widget that draws a creature's stat block, and
// the page types it draws for. A creature row folds it out in place; a page
// of no listed type folds out its own fields instead.
type StatBlock struct {
	Widget   string `json:"widget"`
	SystemID string `json:"systemId"`
	TypeIDs  []int  `json:"typeIds"`
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
