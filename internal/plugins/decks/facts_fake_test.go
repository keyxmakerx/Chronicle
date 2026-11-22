package decks

import (
	"context"
	"sync"
	"time"
)

// fakeFacts is a campaign's facts held in maps. A key with an error in errs
// answers that error, whichever the fact.
type fakeFacts struct {
	mu     sync.Mutex
	nums   map[string]NumberSet
	trks   map[string][]TrackerFact
	tags   map[string][]TagFact
	lays   map[string][]LayerFact
	fields map[string][]NumberField
	opts   WhenOptions
	errs   map[string]error
	reads  int
}

func newFakeFacts() *fakeFacts {
	return &fakeFacts{nums: map[string]NumberSet{}, trks: map[string][]TrackerFact{}, tags: map[string][]TagFact{},
		lays: map[string][]LayerFact{}, fields: map[string][]NumberField{}, errs: map[string]error{},
		opts: WhenOptions{
			Heroes:   []OptionHero{{ID: "h-tam", Name: "Tam"}, {ID: "h-ilse", Name: "Ilse"}},
			Fields:   []NumberField{{Key: "hp", Label: "Hit points", MaxKey: "hpmax"}, {Key: "hpmax", Label: "Max hit points"}, {Key: "stamina", Label: "Stamina"}},
			Trackers: []TrackerKindOption{{ID: "k-doom", Name: "Doom"}},
			Tags:     []TagOption{{ID: 7, Name: "Cursed"}, {ID: 8, Name: "Secretly-a-spy"}},
		}}
}

// sheet gives a page a stored sheet with these values.
func (f *fakeFacts) sheet(page string, vals map[string]float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nums[page] = NumberSet{Sheet: true, Values: vals, Sources: map[string]string{}, ChangedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
}

func (f *fakeFacts) fail(key string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[key] = err
}

func (f *fakeFacts) Numbers(_ context.Context, _, page string) (NumberSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if err := f.errs["numbers:"+page]; err != nil {
		return NumberSet{}, err
	}
	return f.nums[page], nil
}

func (f *fakeFacts) Trackers(_ context.Context, _, page string) ([]TrackerFact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if err := f.errs["trackers:"+page]; err != nil {
		return nil, err
	}
	return f.trks[page], nil
}

func (f *fakeFacts) Tags(_ context.Context, _, page string) ([]TagFact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if err := f.errs["tags:"+page]; err != nil {
		return nil, err
	}
	return f.tags[page], nil
}

func (f *fakeFacts) Layers(_ context.Context, _, page string) ([]LayerFact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if err := f.errs["layers:"+page]; err != nil {
		return nil, err
	}
	return f.lays[page], nil
}

func (f *fakeFacts) Fields(_ context.Context, _, page string) ([]NumberField, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["fields:"+page]; err != nil {
		return nil, err
	}
	if l, ok := f.fields[page]; ok {
		return l, nil
	}
	return f.opts.Fields, nil
}

func (f *fakeFacts) Options(context.Context, string) (WhenOptions, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["options"]; err != nil {
		return WhenOptions{}, err
	}
	return f.opts, nil
}

// newFactsFixture is the usual campaign with facts wired in. Pokes are counted.
func newFactsFixture(t interface {
	Helper()
	Fatalf(string, ...any)
}, fx *fixture, f *fakeFacts) *int {
	t.Helper()
	pokes := new(int)
	fx.svc = NewService(fx.repo, fx.heroes, fx.members, WithNotifier(fx.cap), WithBell(fx.cap), WithPictures(fx.pics),
		WithRandom(func(int) int { return 0 }), WithFacts(f), WithMoments(memMoments{repo: fx.repo}), WithPoke(func(string) { *pokes++ }))
	return pokes
}

// memMoments is the moment store over the in-memory repository, for tests that
// need "say" and fits_since but no moments. It holds no moment, so a hand read
// finds no invitation. Lifecycle and race tests run on the real database.
type memMoments struct {
	repo *memRepo
}

func (m memMoments) WatchedDecks(context.Context, string) ([]string, error) { return nil, nil }
func (m memMoments) PileCounts(_ context.Context, _, deckID string) (map[string]int, error) {
	out := map[string]int{}
	for _, c := range m.repo.copies {
		if c.DeckID == deckID && c.Zone == ZonePile {
			out[c.CardID]++
		}
	}
	return out, nil
}
func (m memMoments) SetFits(_ context.Context, _, _ string, start, _ []string) (int, error) {
	return len(start), nil
}
func (m memMoments) ClearFits(context.Context, string, string) error { return nil }
func (m memMoments) SetSaid(_ context.Context, _, deckID, cardID string, said bool) (bool, error) {
	c, ok := m.repo.cards[cardID]
	if !ok || c.DeckID != deckID {
		return false, nil
	}
	if said {
		now := time.Now()
		c.SaidAt = &now
	} else {
		c.SaidAt = nil
	}
	m.repo.cards[cardID] = c
	return true, nil
}
func (m memMoments) Open(context.Context, string, string) (Moment, bool, error) {
	return Moment{}, false, nil
}
func (m memMoments) ListOpen(context.Context, string) ([]Moment, error) { return nil, nil }
func (m memMoments) Get(context.Context, string, string) (Moment, bool, error) {
	return Moment{}, false, nil
}
func (m memMoments) InvitedFor(context.Context, string, string, string) (Moment, bool, error) {
	return Moment{}, false, nil
}
func (m memMoments) Create(context.Context, Moment) (bool, error) { return false, nil }
func (m memMoments) Repick(context.Context, string, string, string, Moment) (bool, error) {
	return false, nil
}
func (m memMoments) Refresh(context.Context, string, string, string, Moment) (bool, error) {
	return true, nil
}
func (m memMoments) Close(context.Context, string, string, []string, string, string) (bool, error) {
	return false, nil
}
func (m memMoments) Invite(context.Context, string, string, string) (bool, error) { return false, nil }
func (m memMoments) LastClosed(context.Context, string, string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
