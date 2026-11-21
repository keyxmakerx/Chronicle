package entities

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePracticeRepo is PracticeRepository over a map, with the cap and the
// ask transitions the SQL gives.
type fakePracticeRepo struct {
	mu   sync.Mutex
	rows map[string]PracticeHero
	tick int
}

// touch moves a row's version the way the table's ON UPDATE does.
func (f *fakePracticeRepo) touch(h *PracticeHero) {
	f.tick++
	h.UpdatedAt = time.Unix(1_700_000_000, 0).Add(time.Duration(f.tick) * time.Millisecond)
}

func newFakePracticeRepo() *fakePracticeRepo {
	return &fakePracticeRepo{rows: map[string]PracticeHero{}}
}

func (f *fakePracticeRepo) count(campaignID, userID string) int {
	n := 0
	for _, h := range f.rows {
		if h.CampaignID == campaignID && h.UserID == userID {
			n++
		}
	}
	return n
}

func (f *fakePracticeRepo) Insert(_ context.Context, h *PracticeHero, max int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.count(h.CampaignID, h.UserID) >= max {
		return false, nil
	}
	c := *h
	c.CreatedAt = time.Now()
	f.touch(&c)
	f.rows[h.ID] = c
	return true, nil
}
func (f *fakePracticeRepo) Restore(_ context.Context, h *PracticeHero) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[h.ID] = *h
	return nil
}
func (f *fakePracticeRepo) Find(_ context.Context, campaignID, id string) (*PracticeHero, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, ok := f.rows[id]
	if !ok || h.CampaignID != campaignID {
		return nil, nil
	}
	return &h, nil
}
func (f *fakePracticeRepo) ListByUser(_ context.Context, campaignID, userID string) ([]PracticeHero, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []PracticeHero
	for _, h := range f.rows {
		if h.CampaignID == campaignID && h.UserID == userID {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (f *fakePracticeRepo) ListByCampaign(_ context.Context, campaignID string) ([]PracticeHero, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []PracticeHero
	for _, h := range f.rows {
		if h.CampaignID == campaignID {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (f *fakePracticeRepo) Replace(_ context.Context, h *PracticeHero) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.rows[h.ID]
	if !ok || cur.CampaignID != h.CampaignID || cur.UserID != h.UserID {
		return false, nil
	}
	cur.Name, cur.Picks, cur.Level = h.Name, h.Picks, h.Level
	cur.AskStatus, cur.AskNote, cur.AskedAt = "", "", nil
	f.touch(&cur)
	f.rows[h.ID] = cur
	return true, nil
}
func (f *fakePracticeRepo) Delete(_ context.Context, campaignID, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.rows[id]
	if !ok || cur.CampaignID != campaignID || cur.UserID != userID {
		return false, nil
	}
	delete(f.rows, id)
	return true, nil
}
func (f *fakePracticeRepo) SetAsk(_ context.Context, campaignID, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.rows[id]
	if !ok || cur.CampaignID != campaignID || cur.UserID != userID || cur.AskStatus == PracticeAskWaiting {
		return false, nil
	}
	cur.AskStatus, cur.AskNote = PracticeAskWaiting, ""
	f.touch(&cur)
	f.rows[id] = cur
	return true, nil
}
func (f *fakePracticeRepo) Decline(_ context.Context, campaignID, id, note string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.rows[id]
	if !ok || cur.CampaignID != campaignID || cur.AskStatus != PracticeAskWaiting {
		return false, nil
	}
	cur.AskStatus, cur.AskNote = PracticeAskDeclined, note
	f.touch(&cur)
	f.rows[id] = cur
	return true, nil
}
func (f *fakePracticeRepo) TakeWaiting(_ context.Context, campaignID, id string, version int64) (*PracticeHero, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.rows[id]
	if !ok || cur.CampaignID != campaignID || cur.AskStatus != PracticeAskWaiting {
		return nil, nil
	}
	if cur.Version() != version {
		return nil, ErrPracticeChanged
	}
	delete(f.rows, id)
	return &cur, nil
}
func (f *fakePracticeRepo) ClearWaitingForUser(_ context.Context, campaignID, userID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for id, h := range f.rows {
		if h.CampaignID == campaignID && h.UserID == userID && h.AskStatus == PracticeAskWaiting {
			h.AskStatus, h.AskNote, h.AskedAt = "", "", nil
			f.rows[id] = h
			n++
		}
	}
	return n, nil
}
func (f *fakePracticeRepo) DeleteForUser(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (f *fakePracticeRepo) DeleteForUserEverywhere(context.Context, string) (int64, error) {
	return 0, nil
}
func (f *fakePracticeRepo) MoveUser(context.Context, string, string, string, int) error { return nil }

// fakePlanner serves one plan and records how it was asked.
type fakePlanner struct {
	plan      *HeroPlan
	directors []bool
	unlimited []bool
	users     []string
}

func (p *fakePlanner) HeroPlan(ctx context.Context, _, userID string, director bool) (*HeroPlan, error) {
	p.directors = append(p.directors, director)
	p.unlimited = append(p.unlimited, PlanUnlimited(ctx))
	p.users = append(p.users, userID)
	if PlanUnlimited(ctx) {
		return unlimitedHeroPlan(), nil
	}
	return p.plan, nil
}

// unlimitedHeroPlan is testHeroPlan with no rules applied: nothing held, no
// ask-first, Fury on the list.
func unlimitedHeroPlan() *HeroPlan {
	plan := testHeroPlan()
	cls := &plan.Steps[1]
	cls.Choices = []HeroChoice{{Name: "Conduit"}, {Name: "Tactician"}, {Name: "Fury"}}
	cls.Held, cls.Rules = nil, nil
	return plan
}

// testHeroPlan: Fury is held back by the rules, Tactician needs a yes, and
// Human carries a buy list of two points.
func testHeroPlan() *HeroPlan {
	buy := map[string]any{"signature_points": float64(2), "traits": []any{
		map[string]any{"name": "Tough", "cost": float64(1)}, map[string]any{"name": "Quick", "cost": float64(1)}, map[string]any{"name": "Big", "cost": float64(2)}}}
	return &HeroPlan{SystemName: "Test", Steps: []HeroStep{
		{FieldKey: "ancestry", Label: "Ancestry", Choices: []HeroChoice{{Name: "Human", Properties: buy}, {Name: "Dwarf"}}},
		{FieldKey: "class", Label: "Class",
			Choices: []HeroChoice{{Name: "Conduit"}, {Name: "Tactician", State: HeroChoiceStateAskFirst}},
			Held:    []HeroChoice{{Name: "Fury"}},
			Rules:   &HeroStepRules{Limited: true, Required: true}},
	}}
}

func newPracticeSvc() (PracticeService, *fakePracticeRepo, *fakePlanner) {
	repo, planner := newFakePracticeRepo(), &fakePlanner{plan: testHeroPlan()}
	return NewPracticeService(repo, planner, PracticeDeps{}), repo, planner
}

// claimFor claims a practice hero as a Director who looked at its current state.
func claimFor(svc PracticeService, ctx context.Context, campaignID, id string, actor PracticeActor) (*PracticeHero, error) {
	seen := "0"
	if h, err := svc.Open(ctx, campaignID, id, PracticeActor{UserID: "dm", Director: true}); err == nil {
		seen = strconv.FormatInt(h.Version(), 10)
	}
	return svc.(*practiceService).claim(ctx, campaignID, id, actor, seen)
}

var (
	pat      = PracticeActor{UserID: "pat"}
	sam      = PracticeActor{UserID: "sam"}
	director = PracticeActor{UserID: "dm", Director: true}
)

func TestPracticeService_Create(t *testing.T) {
	long := strings.Repeat("x", 201)
	tests := []struct {
		name     string
		in       PracticeInput
		wantCode int
		wantPick map[string]HeroPick
	}{
		{"a plain pick is stored as the plan spells it", PracticeInput{Name: " Old Marrow ", Picks: map[string]HeroPick{"class": {Name: "conduit"}}}, 0,
			map[string]HeroPick{"class": {Name: "Conduit"}}},
		{"a not-offered entry is stored as a plain pick", PracticeInput{Name: "A", Picks: map[string]HeroPick{"class": {Name: "Fury"}}}, 0,
			map[string]HeroPick{"class": {Name: "Fury"}}},
		{"an ask-first entry is stored as a plain pick, with its options checked", PracticeInput{Name: "A", Picks: map[string]HeroPick{"class": {Name: "Tactician"}}}, 0,
			map[string]HeroPick{"class": {Name: "Tactician"}}},
		{"bought options are kept", PracticeInput{Name: "A", Picks: map[string]HeroPick{"ancestry": {Name: "Human", Options: []string{"Tough", "Quick"}}}}, 0,
			map[string]HeroPick{"ancestry": {Name: "Human", Options: []string{"Tough", "Quick"}}}},
		{"no name", PracticeInput{Name: "  "}, 400, nil},
		{"a name past the hero name limit", PracticeInput{Name: long}, 400, nil},
		{"unknown step", PracticeInput{Name: "A", Picks: map[string]HeroPick{"nope": {Name: "x"}}}, 400, nil},
		{"a name not in the practice plan", PracticeInput{Name: "A", Picks: map[string]HeroPick{"class": {Name: "Censor"}}}, 400, nil},
		{"options over budget", PracticeInput{Name: "A", Picks: map[string]HeroPick{"ancestry": {Name: "Human", Options: []string{"Tough", "Quick", "Big"}}}}, 400, nil},
		{"an option the entry does not offer", PracticeInput{Name: "A", Picks: map[string]HeroPick{"ancestry": {Name: "Human", Options: []string{"Wings"}}}}, 400, nil},
		{"options on an entry with no list", PracticeInput{Name: "A", Picks: map[string]HeroPick{"ancestry": {Name: "Dwarf", Options: []string{"Tough"}}}}, 400, nil},
		{"picks too large to keep", PracticeInput{Name: "A", Picks: map[string]HeroPick{"ancestry": {Name: "Human", Options: bigOptions()}}}, 400, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newPracticeSvc()
			got, err := svc.Create(context.Background(), "c1", pat, tc.in)
			if tc.wantCode != 0 {
				assertAppError(t, err, tc.wantCode)
				if repo.count("c1", "pat") != 0 {
					t.Error("a refused hero was saved")
				}
				return
			}
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if got.UserID != "pat" || got.CampaignID != "c1" || got.Name != strings.TrimSpace(tc.in.Name) {
				t.Errorf("saved %+v", got)
			}
			if len(got.Picks) != len(tc.wantPick) {
				t.Fatalf("picks = %+v, want %+v", got.Picks, tc.wantPick)
			}
			for k, want := range tc.wantPick {
				g := got.Picks[k]
				if g.Name != want.Name || strings.Join(g.Options, ",") != strings.Join(want.Options, ",") {
					t.Errorf("pick %s = %+v, want %+v", k, g, want)
				}
			}
		})
	}
}

func bigOptions() []string {
	out := make([]string, 0, 8000)
	for i := 0; i < 8000; i++ {
		out = append(out, "option-number-"+strings.Repeat("z", 4))
	}
	return out
}

func TestPracticeService_Cap(t *testing.T) {
	svc, repo, _ := newPracticeSvc()
	ctx := context.Background()
	for i := 0; i < MaxPracticeHeroes; i++ {
		if _, err := svc.Create(ctx, "c1", pat, PracticeInput{Name: "H"}); err != nil {
			t.Fatalf("hero %d: %v", i, err)
		}
	}
	_, err := svc.Create(ctx, "c1", pat, PracticeInput{Name: "One too many"})
	assertAppError(t, err, 409)
	if !strings.Contains(err.Error(), "You have three practice heroes. Throw one away to start another.") {
		t.Errorf("refusal words: %v", err)
	}
	// The cap is per player and per campaign.
	if _, err := svc.Create(ctx, "c1", sam, PracticeInput{Name: "Sam's"}); err != nil {
		t.Errorf("another player: %v", err)
	}
	if _, err := svc.Create(ctx, "c2", pat, PracticeInput{Name: "Other table"}); err != nil {
		t.Errorf("another campaign: %v", err)
	}
	// Throwing one away makes room.
	mine, _ := svc.ListMine(ctx, "c1", "pat")
	if err := svc.Delete(ctx, "c1", mine[0].ID, pat); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "c1", pat, PracticeInput{Name: "Again"}); err != nil {
		t.Errorf("after throwing one away: %v", err)
	}
	if n := repo.count("c1", "pat"); n != MaxPracticeHeroes {
		t.Errorf("count = %d", n)
	}
}

// Concurrent creates cannot get past the cap.
func TestPracticeService_CapHoldsUnderRace(t *testing.T) {
	svc, repo, _ := newPracticeSvc()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.Create(context.Background(), "c1", pat, PracticeInput{Name: "H"})
		}()
	}
	wg.Wait()
	if n := repo.count("c1", "pat"); n != MaxPracticeHeroes {
		t.Errorf("count = %d, want %d", n, MaxPracticeHeroes)
	}
}

// The plan the service validates against is the maker's own limited one, with
// the held entries put back; never the unlimited plan and never the
// Directors' view.
func TestPracticeService_PlanIsTheMakersOwnLimitedPlan(t *testing.T) {
	svc, _, planner := newPracticeSvc()
	plan, err := svc.Plan(context.Background(), "c1", "pat")
	if err != nil {
		t.Fatal(err)
	}
	if planner.directors[0] || planner.unlimited[0] || planner.users[0] != "pat" {
		t.Errorf("planner asked director=%v unlimited=%v user=%q", planner.directors[0], planner.unlimited[0], planner.users[0])
	}
	if !plan.Practice || len(plan.Steps[1].Choices) != 3 || plan.Steps[1].Choices[2].State != HeroChoiceStateNotOffered {
		t.Errorf("plan = %+v", plan.Steps[1])
	}
}

func TestPracticeService_Access(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newPracticeSvc()
	h, err := svc.Create(ctx, "c1", pat, PracticeInput{Name: "Mine"})
	if err != nil {
		t.Fatal(err)
	}
	scribe := PracticeActor{UserID: "scribe"} // staff but not a Director
	tests := []struct {
		name     string
		do       func() error
		wantCode int
	}{
		{"the maker opens it", func() error { _, e := svc.Open(ctx, "c1", h.ID, pat); return e }, 0},
		{"a Director opens it", func() error { _, e := svc.Open(ctx, "c1", h.ID, director); return e }, 0},
		{"another player gets a 404", func() error { _, e := svc.Open(ctx, "c1", h.ID, sam); return e }, 404},
		{"a Scribe who is not a Director gets a 404", func() error { _, e := svc.Open(ctx, "c1", h.ID, scribe); return e }, 404},
		{"another campaign's id is a 404", func() error { _, e := svc.Open(ctx, "c2", h.ID, pat); return e }, 404},
		{"a Director cannot change it", func() error { _, e := svc.Update(ctx, "c1", h.ID, director, PracticeInput{Name: "x"}); return e }, 404},
		{"another player cannot change it", func() error { _, e := svc.Update(ctx, "c1", h.ID, sam, PracticeInput{Name: "x"}); return e }, 404},
		{"a Director cannot throw it away", func() error { return svc.Delete(ctx, "c1", h.ID, director) }, 404},
		{"another player cannot ask for it", func() error { return svc.Ask(ctx, "c1", h.ID, sam) }, 404},
		{"a player cannot list everyone's", func() error { _, e := svc.ListAll(ctx, "c1", pat); return e }, 403},
		{"a Director lists everyone's", func() error { _, e := svc.ListAll(ctx, "c1", director); return e }, 0},
		{"a player cannot say yes", func() error { _, e := claimFor(svc, ctx, "c1", h.ID, pat); return e }, 403},
		{"a player cannot say not now", func() error { _, e := svc.Decline(ctx, "c1", h.ID, pat, ""); return e }, 403},
		{"nothing to answer before an ask", func() error { _, e := svc.Decline(ctx, "c1", h.ID, director, ""); return e }, 409},
		{"nothing to claim before an ask", func() error { _, e := claimFor(svc, ctx, "c1", h.ID, director); return e }, 409},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.do()
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertAppError(t, err, tc.wantCode)
		})
	}
	if got, _ := svc.Open(ctx, "c1", h.ID, pat); got.Name != "Mine" {
		t.Errorf("a refused change altered the hero: %+v", got)
	}
}

func TestPracticeService_AskAndAnswer(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newPracticeSvc()
	h, _ := svc.Create(ctx, "c1", pat, PracticeInput{Name: "Mine", Picks: map[string]HeroPick{"class": {Name: "Fury"}}})

	if err := svc.Ask(ctx, "c1", h.ID, pat); err != nil {
		t.Fatal(err)
	}
	if err := svc.Ask(ctx, "c1", h.ID, pat); err != nil {
		t.Errorf("asking twice should be harmless: %v", err)
	}
	if got, _ := svc.Open(ctx, "c1", h.ID, pat); got.AskStatus != PracticeAskWaiting {
		t.Fatalf("status = %q", got.AskStatus)
	}

	dec, err := svc.Decline(ctx, "c1", h.ID, director, "  Not this arc.  ")
	if err != nil || dec.AskStatus != PracticeAskDeclined || dec.AskNote != "Not this arc." {
		t.Fatalf("decline: %+v %v", dec, err)
	}
	_, err = svc.Decline(ctx, "c1", h.ID, director, "again")
	assertAppError(t, err, 409)

	// The maker may ask again after a no; changing it clears the answer.
	if err := svc.Ask(ctx, "c1", h.ID, pat); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", h.ID, pat, PracticeInput{Name: "Changed", Picks: map[string]HeroPick{"class": {Name: "Conduit"}}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Open(ctx, "c1", h.ID, pat); got.AskStatus != "" || got.AskNote != "" || got.Name != "Changed" {
		t.Errorf("a change should clear the ask: %+v", got)
	}

	// A yes takes the hero off the shelf exactly once.
	if err := svc.Ask(ctx, "c1", h.ID, pat); err != nil {
		t.Fatal(err)
	}
	if _, err := claimFor(svc, ctx, "c1", h.ID, director); err != nil {
		t.Fatalf("claim: %v", err)
	}
	_, err = claimFor(svc, ctx, "c1", h.ID, director)
	assertAppError(t, err, 404)
	if repo.count("c1", "pat") != 0 {
		t.Error("a claimed practice hero is still on the shelf")
	}
}

func TestPracticeService_RestoreAfterFailedYes(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newPracticeSvc()
	h, _ := svc.Create(ctx, "c1", pat, PracticeInput{Name: "Mine"})
	_ = svc.Ask(ctx, "c1", h.ID, pat)
	before, _ := svc.Open(ctx, "c1", h.ID, pat)
	taken, err := claimFor(svc, ctx, "c1", h.ID, director)
	if err != nil {
		t.Fatal(err)
	}
	svc.(*practiceService).restore(ctx, taken)
	got, err := svc.Open(ctx, "c1", h.ID, pat)
	if err != nil || got.AskStatus != PracticeAskWaiting || got.Name != "Mine" {
		t.Errorf("restored hero = %+v, %v", got, err)
	}
	// Put back as it was, the version a Director saw still matches.
	if got.Version() != before.Version() {
		t.Errorf("restored version %d, was %d", got.Version(), before.Version())
	}
}

func TestPracticeService_DeclineNoteBounded(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newPracticeSvc()
	h, _ := svc.Create(ctx, "c1", pat, PracticeInput{Name: "Mine"})
	_ = svc.Ask(ctx, "c1", h.ID, pat)
	_, err := svc.Decline(ctx, "c1", h.ID, director, strings.Repeat("n", 501))
	assertAppError(t, err, 400)
}
