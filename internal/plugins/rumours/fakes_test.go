package rumours

import (
	"context"
	"sort"
	"time"
)

const (
	tCamp = "camp-1"
	tPage = "5f0a7c52-6b64-4d6f-8d0c-0b6f3a1f0001"
	// Ids of the seeded rumours.
	idShip   = "6f0a7c52-6b64-4d6f-8d0c-0b6f3a1f0001" // Director's, true, party thinks false
	idTreas  = "6f0a7c52-6b64-4d6f-8d0c-0b6f3a1f0002" // Mira's, false, party thinks true
	idLog    = "6f0a7c52-6b64-4d6f-8d0c-0b6f3a1f0003" // Director's, open, unmarked
	idFavour = "6f0a7c52-6b64-4d6f-8d0c-0b6f3a1f0004" // Tomas's, open, unmarked
)

// The people in the table: a Director, two Scribes, a player and a visitor
// with no seat. Role is the campaign's visibility role.
var (
	vDM      = Viewer{UserID: "dm-1", Role: 3}
	vMira    = Viewer{UserID: "mira-1", Role: 2}
	vTomas   = Viewer{UserID: "tomas-1", Role: 2}
	vJo      = Viewer{UserID: "jo-1", Role: 1}
	vVisitor = Viewer{UserID: "", Role: 0}
)

// fakeRepo is an in-memory Repository. By default it drops the truth from a
// read made without withTruth the way the real SQL does; leaky switches that
// off so a test can prove the SERVICE still keeps the truth from a viewer who
// may not have it when the repository hands it over.
type fakeRepo struct {
	rows       []Rumour
	names      map[string]string // user id -> display name
	leaky      bool
	noPage     bool
	truthReads int // reads made withTruth
	allReads   int
	seq        int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{names: map[string]string{"dm-1": "Keyx", "mira-1": "Mira", "tomas-1": "Tomas", "jo-1": "Jo"}}
}

// seeded is a page with four rumours, the Director's verdicts set on two and
// the party's marks on two.
func seeded() *fakeRepo {
	f := newFakeRepo()
	at := func(age int) time.Time {
		return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC).Add(-time.Duration(age) * time.Hour)
	}
	f.rows = []Rumour{
		{ID: idShip, CampaignID: tCamp, EntityID: tPage, Quote: "She sank her own ship for the insurance.", Source: "Old Bram", Sure: 3,
			Truth: TruthTrue, Belief: BeliefFalse, BeliefBy: "tomas-1", BeliefName: "Tomas", ByDM: true, CreatedBy: "dm-1", AuthorName: "Keyx", CreatedAt: at(4)},
		{ID: idTreas, CampaignID: tCamp, EntityID: tPage, Quote: "Dessa Quill took the ship's treasure and sailed east.", Source: "Mara", Sure: 1,
			Truth: TruthFalse, Belief: BeliefTrue, BeliefBy: "mira-1", BeliefName: "Mira", CreatedBy: "mira-1", AuthorName: "Mira", CreatedAt: at(3)},
		{ID: idLog, CampaignID: tCamp, EntityID: tPage, Quote: "She keeps a second logbook under the floorboards.", Source: "Tobbs", Sure: 2,
			Truth: TruthOpen, ByDM: true, CreatedBy: "dm-1", AuthorName: "Keyx", CreatedAt: at(2)},
		{ID: idFavour, CampaignID: tCamp, EntityID: tPage, Quote: "The harbourmaster owes her a favour.", Source: "A deckhand", Sure: 1,
			Truth: TruthOpen, CreatedBy: "tomas-1", AuthorName: "Tomas", CreatedAt: at(1)},
	}
	return f
}

func (f *fakeRepo) List(_ context.Context, campaignID, entityID string, withTruth bool, limit int) ([]Rumour, int, error) {
	f.allReads++
	if withTruth {
		f.truthReads++
	}
	var in []Rumour
	for _, r := range f.rows {
		if r.CampaignID == campaignID && r.EntityID == entityID {
			in = append(in, r)
		}
	}
	sort.SliceStable(in, func(i, j int) bool { return in[i].CreatedAt.After(in[j].CreatedAt) })
	total := len(in)
	if len(in) > limit {
		in = in[:limit]
	}
	for i := range in {
		if !withTruth && !f.leaky {
			in[i].Truth = ""
		}
	}
	return in, total, nil
}

func (f *fakeRepo) Get(_ context.Context, campaignID, entityID, id string, withTruth bool) (*Rumour, error) {
	for _, r := range f.rows {
		if r.ID == id && r.CampaignID == campaignID && r.EntityID == entityID {
			if !withTruth && !f.leaky {
				r.Truth = ""
			}
			return &r, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) Insert(_ context.Context, r Rumour, max int) error {
	if f.noPage {
		return errNoPage
	}
	n := 0
	for _, x := range f.rows {
		if x.EntityID == r.EntityID {
			n++
		}
	}
	if n >= max {
		return errPageFull
	}
	f.seq++
	r.Truth = TruthOpen
	r.AuthorName = f.names[r.CreatedBy]
	r.CreatedAt = time.Date(2026, 10, 18, 13, 0, f.seq, 0, time.UTC)
	f.rows = append(f.rows, r)
	return nil
}

func (f *fakeRepo) at(campaignID, entityID, id string) *Rumour {
	for i := range f.rows {
		if f.rows[i].ID == id && f.rows[i].CampaignID == campaignID && f.rows[i].EntityID == entityID {
			return &f.rows[i]
		}
	}
	return nil
}

func (f *fakeRepo) UpdateText(_ context.Context, campaignID, entityID, id, quote, source string, sure int) (bool, error) {
	r := f.at(campaignID, entityID, id)
	if r == nil {
		return false, nil
	}
	r.Quote, r.Source, r.Sure = quote, source, sure
	return true, nil
}

func (f *fakeRepo) SetTruth(_ context.Context, campaignID, entityID, id, truth string) (bool, error) {
	r := f.at(campaignID, entityID, id)
	if r == nil {
		return false, nil
	}
	r.Truth = truth
	return true, nil
}

func (f *fakeRepo) SetBelief(_ context.Context, campaignID, entityID, id, belief, byUser string, byDM bool) (bool, error) {
	r := f.at(campaignID, entityID, id)
	if r == nil {
		return false, nil
	}
	r.Belief, r.BeliefBy, r.BeliefName, r.BeliefByDM = belief, byUser, f.names[byUser], byDM
	return true, nil
}

func (f *fakeRepo) Delete(_ context.Context, campaignID, entityID, id string) (bool, error) {
	for i, r := range f.rows {
		if r.ID == id && r.CampaignID == campaignID && r.EntityID == entityID {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

// truthOf is what the store holds for a rumour, for asserting a refused write
// changed nothing.
func (f *fakeRepo) truthOf(id string) string {
	for _, r := range f.rows {
		if r.ID == id {
			return r.Truth
		}
	}
	return "<gone>"
}

// newTestService is the real service on a repository, with a fixed clock.
func newTestService(repo Repository) Service {
	s := NewService(repo).(*service)
	s.now = func() time.Time { return time.Date(2026, 10, 18, 15, 0, 0, 0, time.UTC) }
	return s
}

func timeHours(n int) time.Duration { return time.Duration(n) * time.Hour }

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339, s) }
