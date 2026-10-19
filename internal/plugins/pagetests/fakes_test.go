package pagetests

import (
	"context"
	"errors"
	"sort"
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

func (m *memRepo) sorted(keep func(Row) bool) []Row {
	var out []Row
	for _, r := range m.rows {
		if keep(r) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	return out
}

func (m *memRepo) ListForPage(_ context.Context, campaignID, entityID string) ([]Row, error) {
	return m.sorted(func(r Row) bool { return r.CampaignID == campaignID && r.EntityID == entityID }), nil
}

func (m *memRepo) ListForCampaign(_ context.Context, campaignID string, limit int) ([]Row, error) {
	out := m.sorted(func(r Row) bool { return r.CampaignID == campaignID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memRepo) GetMany(_ context.Context, campaignID string, ids []string) ([]Row, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	return m.sorted(func(r Row) bool { return r.CampaignID == campaignID && want[r.ID] }), nil
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

func (m *memRepo) Update(_ context.Context, campaignID, id, name string, shown bool, data []byte, expected int, _ string) (bool, error) {
	r, ok := m.rows[id]
	if !ok || r.CampaignID != campaignID || r.Version != expected {
		return false, nil
	}
	r.Name, r.Shown, r.Data, r.Version, r.UpdatedAt = name, shown, data, r.Version+1, m.tick()
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

// fakePages is a PageDirectory for campaign c1.
type fakePages struct {
	names    map[string]string
	hidden   map[string]bool
	readOnly map[string]bool
}

func newFakePages() *fakePages {
	return &fakePages{
		names:    map[string]string{"page1": "Lastwell", "page2": "The Barrow", "secret": "The traitor's room"},
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

// fakeCal is a Calendar that answers a fixed day, none, or an error.
type fakeCal struct {
	ok  bool
	err error
}

func (f fakeCal) Today(context.Context, string) (Day, string, bool, error) {
	if f.err != nil {
		return Day{}, "", false, f.err
	}
	if !f.ok {
		return Day{}, "", false, nil
	}
	return Day{Year: 812, Month: 3, Day: 14}, "14 Sanguin 812", true, nil
}

var errCalendar = errors.New("calendar down")
