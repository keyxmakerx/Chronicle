package transformations

import (
	"context"
	"sort"
	"time"
)

// memRepo is an in-memory Repository with the same limit rule as SQL.
type memRepo struct {
	rows  map[string]Row
	clock int
}

func newMemRepo() *memRepo { return &memRepo{rows: map[string]Row{}} }

func (m *memRepo) ListForPage(_ context.Context, campaignID, entityID string) ([]Row, error) {
	var out []Row
	for _, r := range m.rows {
		if r.CampaignID == campaignID && r.EntityID == entityID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *memRepo) Get(_ context.Context, campaignID, id string) (Row, bool, error) {
	r, ok := m.rows[id]
	if !ok || r.CampaignID != campaignID {
		return Row{}, false, nil
	}
	return r, true, nil
}

func (m *memRepo) Create(ctx context.Context, r Row, _ string, max int) (bool, error) {
	have, _ := m.ListForPage(ctx, r.CampaignID, r.EntityID)
	if len(have) >= max {
		return false, nil
	}
	m.clock++
	r.CreatedAt = time.Unix(int64(m.clock), 0)
	m.rows[r.ID] = r
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

// fakePages knows pages by id; each says whether players may see or edit it.
type fakePages struct {
	pages map[string]fakePage
}

type fakePage struct {
	campaign            string
	playersSee, dmEdits bool
}

func (f fakePages) Access(_ context.Context, campaignID, entityID string, v Viewer) (PageAccess, error) {
	p, ok := f.pages[entityID]
	if !ok || p.campaign != campaignID {
		return PageAccess{}, nil
	}
	if v.IsDM() {
		return PageAccess{Exists: true, CanView: true, CanEdit: p.dmEdits}, nil
	}
	return PageAccess{Exists: true, CanView: p.playersSee}, nil
}
