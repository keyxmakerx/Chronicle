package encounters

import (
	"context"
	"sort"
	"strings"
	"time"
)

// memRepo is an in-memory Repository with the same version rules as SQL.
type memRepo struct {
	rows  map[string]Row
	clock int
}

func newMemRepo() *memRepo { return &memRepo{rows: map[string]Row{}} }

func (m *memRepo) tick() time.Time {
	m.clock++
	return time.Unix(int64(m.clock), 0)
}

func (m *memRepo) ListForPage(_ context.Context, campaignID, entityID string) ([]Row, error) {
	var out []Row
	for _, r := range m.rows {
		if r.CampaignID == campaignID && r.EntityID == entityID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *memRepo) Get(_ context.Context, campaignID, id string) (Row, bool, error) {
	r, ok := m.rows[id]
	if !ok || r.CampaignID != campaignID {
		return Row{}, false, nil
	}
	return r, true, nil
}

func (m *memRepo) Create(_ context.Context, r Row, _ string) error {
	r.Version = 1
	r.UpdatedAt = m.tick()
	m.rows[r.ID] = r
	return nil
}

func (m *memRepo) Update(_ context.Context, campaignID, id, name string, ready bool, data []byte, expected int, _ string) (bool, error) {
	r, ok := m.rows[id]
	if !ok || r.CampaignID != campaignID || r.Version != expected {
		return false, nil
	}
	r.Name, r.Ready, r.Data, r.Version, r.UpdatedAt = name, ready, data, r.Version+1, m.tick()
	m.rows[id] = r
	return true, nil
}

func (m *memRepo) Delete(_ context.Context, campaignID, id string) (bool, error) {
	r, ok := m.rows[id]
	if !ok || r.CampaignID != campaignID {
		return false, nil
	}
	delete(m.rows, id)
	return true, nil
}

func (m *memRepo) Search(_ context.Context, campaignID, q string, limit int) ([]Row, int, error) {
	var out []Row
	for _, r := range m.rows {
		if r.CampaignID == campaignID && (q == "" || strings.Contains(strings.ToLower(r.Name), strings.ToLower(q))) {
			r.PageName = "Page " + r.EntityID
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ready != out[j].Ready {
			return out[i].Ready
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	total := len(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, total, nil
}

// fakePages is a PageDirectory over a fixed set of pages in campaign "c1".
// hidden pages exist but the viewer may not see them; readOnly pages may be
// seen but not edited.
type fakePages struct {
	names    map[string]string
	hidden   map[string]bool
	readOnly map[string]bool
}

func newFakePages() *fakePages {
	return &fakePages{
		names: map[string]string{
			"page1": "The Siege of Harrowmere", "page2": "Lastwell",
			"hero1": "Ash", "hero2": "Bryn",
			"gob": "Goblin War Chief", "hob": "Hobgoblin Soldier",
			"secret": "The traitor's room",
		},
		hidden:   map[string]bool{"secret": true},
		readOnly: map[string]bool{},
	}
}

func (f *fakePages) Access(_ context.Context, campaignID, entityID string, _ Viewer) (PageAccess, error) {
	if _, ok := f.names[entityID]; !ok || campaignID != "c1" {
		return PageAccess{}, nil
	}
	view := !f.hidden[entityID]
	return PageAccess{Exists: true, CanView: view, CanEdit: view && !f.readOnly[entityID]}, nil
}

func (f *fakePages) Names(_ context.Context, campaignID string, ids []string, _ Viewer) (map[string]string, error) {
	out := map[string]string{}
	if campaignID != "c1" {
		return out, nil
	}
	for _, id := range ids {
		if n, ok := f.names[id]; ok && !f.hidden[id] {
			out[id] = n
		}
	}
	return out, nil
}

// fakeResults is a ResultDirectory over a fixed set of tests in campaign c1.
type fakeResults struct {
	tests map[string]TestOutcome
	err   error
}

func newFakeResults() *fakeResults {
	return &fakeResults{tests: map[string]TestOutcome{
		"lastwell": {Name: "Restore the Lastwell core", Results: map[string]string{"win": "Total success", "part": "Partial success", "lose": "Total failure"}},
		"hunt":     {Name: "Track the stalkers", Results: map[string]string{"found": "Found them"}},
	}}
}

func (f *fakeResults) Outcomes(_ context.Context, campaignID string, ids []string, _ Viewer) (map[string]TestOutcome, error) {
	out := map[string]TestOutcome{}
	if f.err != nil {
		return nil, f.err
	}
	if campaignID != "c1" {
		return out, nil
	}
	for _, id := range ids {
		if o, ok := f.tests[id]; ok {
			out[id] = o
		}
	}
	return out, nil
}
