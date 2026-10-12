package pageblocks

import "context"

// In-memory fakes for the service tests. They enforce campaign scoping and
// the version check the way the SQL does.

type fakePage struct {
	campaign string
	hidden   bool // invisible below owner visibility
	// editable is the page's own edit rule for a Scribe or above.
	editable bool
}

type fakeEntities struct{ pages map[string]fakePage }

func (f *fakeEntities) Access(_ context.Context, campaignID, entityID string, v Viewer) (PageAccess, error) {
	p, ok := f.pages[entityID]
	if !ok || p.campaign != campaignID {
		return PageAccess{}, nil
	}
	canView := !p.hidden || v.VisibilityRole >= 3
	return PageAccess{
		Exists:  true,
		CanView: canView,
		CanEdit: canView && p.editable && v.MemberRole >= 2,
	}, nil
}

type fakeRepo struct {
	rows  map[string]Stored // campaign|entity|block
	saves int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[string]Stored{}} }

func key(c, e, b string) string { return c + "|" + e + "|" + b }

func (f *fakeRepo) Get(_ context.Context, c, e, b string) (Stored, bool, error) {
	s, ok := f.rows[key(c, e, b)]
	return s, ok, nil
}

func (f *fakeRepo) Save(_ context.Context, c, e, b, kind string, data []byte, expected int, _ string) (bool, error) {
	cur, ok := f.rows[key(c, e, b)]
	switch {
	case !ok && expected != 0, ok && cur.Version != expected:
		return false, nil
	}
	f.saves++
	if ok {
		kind = cur.Kind
	}
	f.rows[key(c, e, b)] = Stored{Kind: kind, Data: data, Version: expected + 1}
	return true, nil
}
