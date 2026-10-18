package entities

import (
	"context"
	"sort"
	"time"
)

// fakeStandingRepo is an in-memory StandingRepository. By default it filters
// DM-only reasons the way the real SQL does; leaky switches that off so a test
// can prove the SERVICE still keeps a DM-only reason from a player when the
// repository hands one over.
type fakeStandingRepo struct {
	playersSee map[string]bool // absent = true
	rows       []StandingRow
	reasons    []StandingReason
	names      map[string]string // live pages by id
	chars      []StandingSubjectRef
	leaky      bool
	full       bool
	reads      int // reads that touched standings
	seq        int
}

func newFakeStandingRepo() *fakeStandingRepo {
	return &fakeStandingRepo{playersSee: map[string]bool{}, names: map[string]string{}}
}

func (f *fakeStandingRepo) PlayersSee(_ context.Context, entityID string) (bool, error) {
	if v, ok := f.playersSee[entityID]; ok {
		return v, nil
	}
	return true, nil
}

func (f *fakeStandingRepo) SetPlayersSee(_ context.Context, _, entityID, _ string, see bool) error {
	f.playersSee[entityID] = see
	return nil
}

func (f *fakeStandingRepo) List(_ context.Context, campaignID, entityID string) ([]StandingRow, error) {
	f.reads++
	var out []StandingRow
	for _, r := range f.rows {
		if r.EntityID == entityID && r.CampaignID == campaignID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStandingRepo) Find(_ context.Context, entityID, key string) (*StandingRow, error) {
	for i := range f.rows {
		if f.rows[i].EntityID == entityID && f.rows[i].SubjectKey == key {
			r := f.rows[i]
			return &r, nil
		}
	}
	return nil, nil
}

func (f *fakeStandingRepo) ReasonsFor(_ context.Context, ids []string, includeDM bool, per int) (map[string][]StandingReason, map[string]int, error) {
	f.reads++
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := map[string][]StandingReason{}
	counts := map[string]int{}
	rs := append([]StandingReason(nil), f.reasons...)
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].CreatedAt.After(rs[j].CreatedAt) })
	for _, r := range rs {
		if !want[r.StandingID] || (r.DMOnly && !includeDM && !f.leaky) {
			continue
		}
		if !r.DMOnly || includeDM {
			counts[r.StandingID]++
		}
		if len(out[r.StandingID]) < per {
			out[r.StandingID] = append(out[r.StandingID], r)
		}
	}
	return out, counts, nil
}

func (f *fakeStandingRepo) SaveReason(_ context.Context, w StandingReasonWrite) error {
	if f.full {
		return errStandingFull
	}
	f.seq++
	var row *StandingRow
	for i := range f.rows {
		if f.rows[i].EntityID == w.EntityID && f.rows[i].SubjectKey == w.SubjectKey {
			row = &f.rows[i]
		}
	}
	if row == nil {
		f.rows = append(f.rows, StandingRow{ID: "st-" + w.SubjectKey + w.EntityID, CampaignID: w.CampaignID, EntityID: w.EntityID, SubjectKey: w.SubjectKey})
		row = &f.rows[len(f.rows)-1]
	}
	row.Level = w.Level
	r := w.Reason
	r.StandingID = row.ID
	r.CreatedAt = time.Unix(int64(10000+f.seq), 0)
	f.reasons = append(f.reasons, r)
	return nil
}

func (f *fakeStandingRepo) DeleteReason(_ context.Context, entityID, key, reasonID string) (bool, error) {
	var sid string
	for _, r := range f.rows {
		if r.EntityID == entityID && r.SubjectKey == key {
			sid = r.ID
		}
	}
	for i, r := range f.reasons {
		if r.ID == reasonID && r.StandingID == sid && sid != "" {
			f.reasons = append(f.reasons[:i], f.reasons[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStandingRepo) Names(_ context.Context, _ string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if n, ok := f.names[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

func (f *fakeStandingRepo) Characters(_ context.Context, _ string, _ int) ([]StandingSubjectRef, error) {
	return f.chars, nil
}

// fakeGate lets every page through except those named in hidden.
type fakeGate struct{ hidden map[string]bool }

func (g fakeGate) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, _ int, _ string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = !g.hidden[id]
	}
	return out, nil
}
