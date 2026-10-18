package calendar

import (
	"context"
	"net/http"
	"sort"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestParseDice(t *testing.T) {
	tests := []struct {
		in      string
		want    DiceExpr
		wantErr bool
	}{
		{"2d10", DiceExpr{2, 10, 0}, false},
		{"d20", DiceExpr{1, 20, 0}, false},
		{" 1D20 + 3 ", DiceExpr{1, 20, 3}, false},
		{"3d6-1", DiceExpr{3, 6, -1}, false},
		{"0d6", DiceExpr{}, true},
		{"101d6", DiceExpr{}, true},
		{"2d1", DiceExpr{}, true},
		{"2d10+1001", DiceExpr{}, true},
		{"2d10x", DiceExpr{}, true},
		{"", DiceExpr{}, true},
	}
	for _, tt := range tests {
		got, err := ParseDice(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseDice(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParseDice(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestDiceRollStaysInRange(t *testing.T) {
	d := DiceExpr{Count: 2, Sides: 10, Modifier: 1}
	lo, hi := d.Range()
	for i := 0; i < 500; i++ {
		total, faces, err := d.Roll()
		if err != nil {
			t.Fatal(err)
		}
		if total < lo || total > hi || len(faces) != 2 {
			t.Fatalf("roll %d with faces %v is outside %d..%d", total, faces, lo, hi)
		}
	}
}

func bloodnight() RollConfig {
	return RollConfig{Source: RollSourceDice, Dice: "2d10", OnTheNight: true, Results: []RollBand{
		{Max: intp(11), Name: "Thin blood", Players: "The moon hangs dull.", Director: "Nothing stirs."},
		{Max: intp(16), Name: "The Hunger", Players: "The moon runs red.", Director: "Each Remnant rolls.", TellPlayers: true},
		{Max: intp(99), Name: "Red Tide", Players: "The sea looks like blood.", Director: "Run Red Tide.", TellPlayers: true},
	}}
}

func TestRollConfigNormalize(t *testing.T) {
	tests := []struct {
		name    string
		mut     func(*RollConfig)
		wantErr bool
	}{
		{"valid", func(*RollConfig) {}, false},
		{"bad dice", func(c *RollConfig) { c.Dice = "two dice" }, true},
		{"no results", func(c *RollConfig) { c.Results = nil }, true},
		{"unnamed result", func(c *RollConfig) { c.Results[1].Name = "  " }, true},
		{"missing max", func(c *RollConfig) { c.Results[0].Max = nil }, true},
		{"out of order", func(c *RollConfig) { c.Results[1].Max = intp(5) }, true},
		{"unknown source", func(c *RollConfig) { c.Source = "cards" }, true},
		{"table without id", func(c *RollConfig) { c.Source = RollSourceTable; c.TableID = "" }, true},
		{"table", func(c *RollConfig) { c.Source = RollSourceTable; c.TableID = "omens" }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := bloodnight()
			tt.mut(&c)
			err := c.Normalize()
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalize() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	c := bloodnight()
	if err := c.Normalize(); err != nil {
		t.Fatal(err)
	}
	if c.Results[2].Max != nil {
		t.Error("the last result's max should be dropped: it covers every higher total")
	}
	tc := bloodnight()
	tc.Source, tc.TableID = RollSourceTable, "omens"
	if err := tc.Normalize(); err != nil || tc.Results != nil || tc.Dice != "" {
		t.Errorf("a table roll keeps no bands or dice, got %+v (err %v)", tc, err)
	}
}

func TestRollBandFor(t *testing.T) {
	c := bloodnight()
	_ = c.Normalize()
	for total, want := range map[int]int{2: 0, 11: 0, 12: 1, 16: 1, 17: 2, 20: 2} {
		if got := c.bandFor(total); got != want {
			t.Errorf("bandFor(%d) = %d, want %d", total, got, want)
		}
	}
}

// A player gets only the name and player text of a told night: never the
// total, the Director's text, the lock or how it was rolled.
func TestRollNightViewRedactsForPlayers(t *testing.T) {
	c := bloodnight()
	_ = c.Normalize()
	hunger := RollNight{Year: 5, Month: 1, Day: 3, Total: intp(14), Faces: []int{7, 7}, Source: RollNightAhead, Locked: true}
	thin := RollNight{Year: 5, Month: 1, Day: 4, Total: intp(4), Source: RollNightAhead}

	got, ok := c.view(hunger, false)
	if !ok {
		t.Fatal("a told night should reach players")
	}
	want := RollNightView{Year: 5, Month: 1, Day: 3, Result: "The Hunger", Players: "The moon runs red."}
	if got.Players != want.Players || got.Result != want.Result || got.Director != "" || got.Total != nil ||
		got.Faces != nil || got.Locked != nil || got.Told != nil || got.Source != "" || got.BandIndex != nil {
		t.Errorf("player view = %+v, want only %+v", got, want)
	}
	if _, ok := c.view(thin, false); ok {
		t.Error("a night whose result is not told must not reach players")
	}

	no := false
	hunger.Told = &no
	if _, ok := c.view(hunger, false); ok {
		t.Error("a night the Director stopped telling must not reach players")
	}
	yes := true
	thin.Told = &yes
	if v, ok := c.view(thin, false); !ok || v.Players != "The moon hangs dull." {
		t.Errorf("a night the Director chose to tell should reach players, got %+v %v", v, ok)
	}

	d, ok := c.view(hunger, true)
	if !ok || d.Result != "The Hunger" || d.Director != "Each Remnant rolls." || d.Total == nil || *d.Told {
		t.Errorf("Director view = %+v", d)
	}
}

func TestPickEntryHonoursWeights(t *testing.T) {
	entries := []RollTableEntry{{Name: "never", Weight: 0}, {Name: "always", Weight: 100}}
	counts := map[string]int{}
	for i := 0; i < 300; i++ {
		e, err := pickEntry(entries)
		if err != nil {
			t.Fatal(err)
		}
		counts[e.Name]++
	}
	if counts["always"] < 280 {
		t.Errorf("weights not honoured: %v", counts)
	}
	if _, err := pickEntry(nil); err == nil {
		t.Error("an empty table should be refused")
	}
}

// --- service ---

type fakeRollRepo struct {
	cfg    map[string]*RollConfig
	nights map[string]map[DayDate]RollNight
}

func newFakeRollRepo() *fakeRollRepo {
	return &fakeRollRepo{cfg: map[string]*RollConfig{}, nights: map[string]map[DayDate]RollNight{}}
}

func (f *fakeRollRepo) GetConfig(_ context.Context, id string) (*RollConfig, error) {
	return f.cfg[id], nil
}
func (f *fakeRollRepo) ConfigsFor(_ context.Context, ids []string) (map[string]*RollConfig, error) {
	out := map[string]*RollConfig{}
	for _, id := range ids {
		if c := f.cfg[id]; c != nil {
			out[id] = c
		}
	}
	return out, nil
}
func (f *fakeRollRepo) SetConfig(_ context.Context, id string, c RollConfig) error {
	f.cfg[id] = &c
	return nil
}
func (f *fakeRollRepo) DeleteConfig(_ context.Context, id string) error {
	delete(f.cfg, id)
	delete(f.nights, id)
	return nil
}
func (f *fakeRollRepo) ListNights(_ context.Context, id string, from, to DayDate) ([]RollNight, error) {
	var out []RollNight
	for d, n := range f.nights[id] {
		if d.inRange(from, to) {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].date().before(out[j].date()) })
	return out, nil
}
func (f *fakeRollRepo) PutNights(_ context.Context, id string, ns []RollNight, onlyNew bool) error {
	if f.nights[id] == nil {
		f.nights[id] = map[DayDate]RollNight{}
	}
	for _, n := range ns {
		old, have := f.nights[id][n.date()]
		if have && (old.Locked || onlyNew) {
			continue
		}
		f.nights[id][n.date()] = n
	}
	return nil
}
func (f *fakeRollRepo) LockNights(_ context.Context, id string, ds []DayDate, locked bool) (int, error) {
	c := 0
	for _, d := range ds {
		if n, ok := f.nights[id][d]; ok {
			n.Locked = locked
			f.nights[id][d] = n
			c++
		}
	}
	return c, nil
}
func (f *fakeRollRepo) SetTold(_ context.Context, id string, d DayDate, told *bool) (int, error) {
	if n, ok := f.nights[id][d]; ok {
		n.Told = told
		f.nights[id][d] = n
		return 1, nil
	}
	return 0, nil
}
func (f *fakeRollRepo) ClearNights(_ context.Context, id string, ds []DayDate, keepLocked bool) (int, error) {
	c := 0
	for _, d := range ds {
		if n, ok := f.nights[id][d]; ok && !(keepLocked && n.Locked) {
			delete(f.nights[id], d)
			c++
		}
	}
	return c, nil
}

// rollFixture is a two-month calendar (30 and 20 days), today at year 5,
// month 2, day 10, with one event on the 3rd of every month and the given
// roll on it.
func rollFixture(t *testing.T, cfg *RollConfig, ev func(*Event)) (*calendarService, *fakeRollRepo) {
	t.Helper()
	monthly := RecurrenceMonthly
	evt := &Event{ID: "ev-blood", CalendarID: "cal", Name: "Bloodnight", Year: 1, Month: 1, Day: 3,
		IsRecurring: true, RecurrenceType: &monthly, Visibility: "everyone"}
	if ev != nil {
		ev(evt)
	}
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp", Name: "Reckoning", Mode: ModeFantasy,
				HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
				CurrentYear: 5, CurrentMonth: 2, CurrentDay: 10}, nil
		},
		getMonthsFn: func(context.Context, string) ([]Month, error) {
			return []Month{{Name: "One", Days: 30}, {Name: "Two", Days: 20}}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			if id != evt.ID {
				return nil, nil
			}
			e := *evt
			return &e, nil
		},
		listForMonthFn: func(context.Context, string, int, int, int) ([]Event, error) {
			return []Event{*evt}, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil).(*calendarService)
	rolls := newFakeRollRepo()
	svc.SetEventRollRepository(rolls)
	if cfg != nil {
		rolls.cfg[evt.ID] = cfg
	}
	return svc, rolls
}

func TestRollTheYearKeepsLockedNights(t *testing.T) {
	cfg := bloodnight()
	_ = cfg.Normalize()
	cfg.OnTheNight = false
	svc, rolls := rollFixture(t, &cfg, nil)
	ctx := context.Background()

	got, err := svc.RollEventNights(ctx, "ev-blood", "cal", "camp", RollNightsInput{Year: 6}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nights) != 2 || len(got.Upcoming) != 0 {
		t.Fatalf("year 6 should have two rolled nights and none waiting, got %+v", got)
	}
	first := DayDate{Year: 6, Month: 1, Day: 3}
	locked := 1
	rolls.nights["ev-blood"][first] = RollNight{Year: 6, Month: 1, Day: 3, Total: &locked, Source: RollNightManual}
	if _, err := svc.LockEventRollNights(ctx, "ev-blood", "cal", "camp", []DayDate{first}, true, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RollEventNights(ctx, "ev-blood", "cal", "camp", RollNightsInput{Year: 6}, owner); err != nil {
		t.Fatal(err)
	}
	if n := rolls.nights["ev-blood"][first]; n.Total == nil || *n.Total != 1 || n.Source != RollNightManual {
		t.Errorf("a locked night was rerolled: %+v", n)
	}

	n, err := svc.ClearEventRollNights(ctx, "ev-blood", "cal", "camp", 6, true, owner)
	if err != nil || n != 1 {
		t.Fatalf("clearing unlocked nights cleared %d (err %v), want 1", n, err)
	}
}

func TestRollRefusesDatesTheEventDoesNotLandOn(t *testing.T) {
	cfg := bloodnight()
	svc, _ := rollFixture(t, &cfg, nil)
	_, err := svc.RollEventNights(context.Background(), "ev-blood", "cal", "camp",
		RollNightsInput{Dates: []DayDate{{Year: 6, Month: 1, Day: 4}}}, owner)
	if apperror.SafeCode(err) != http.StatusUnprocessableEntity {
		t.Fatalf("rolling a day the event is not on = %v, want 422", err)
	}
}

func TestRollWritesAreTheDirectors(t *testing.T) {
	cfg := bloodnight()
	svc, _ := rollFixture(t, &cfg, nil)
	ctx := context.Background()
	checks := map[string]error{}
	_, checks["set"] = svc.SetEventRoll(ctx, "ev-blood", "cal", "camp", &cfg, 5, player)
	_, checks["roll"] = svc.RollEventNights(ctx, "ev-blood", "cal", "camp", RollNightsInput{Year: 5}, player)
	_, checks["lock"] = svc.LockEventRollNights(ctx, "ev-blood", "cal", "camp", nil, true, player)
	checks["tell"] = svc.TellEventRollNight(ctx, "ev-blood", "cal", "camp", DayDate{Year: 5, Month: 1, Day: 3}, nil, player)
	_, checks["clear"] = svc.ClearEventRollNights(ctx, "ev-blood", "cal", "camp", 5, false, player)
	for name, err := range checks {
		if apperror.SafeCode(err) != http.StatusForbidden {
			t.Errorf("%s by a player = %v, want 403", name, err)
		}
	}
}

// A player reading the year sees told nights up to today, as player text,
// and nothing of the future or the roll itself.
func TestPlayerSeesOnlyToldNightsUpToToday(t *testing.T) {
	cfg := bloodnight()
	_ = cfg.Normalize()
	cfg.OnTheNight = false
	svc, rolls := rollFixture(t, &cfg, nil)
	rolls.nights["ev-blood"] = map[DayDate]RollNight{
		{Year: 5, Month: 1, Day: 3}: {Year: 5, Month: 1, Day: 3, Total: intp(14), Source: RollNightAhead}, // told, past
		{Year: 5, Month: 2, Day: 3}: {Year: 5, Month: 2, Day: 3, Total: intp(4), Source: RollNightAhead},  // not told
	}
	// A future told night: year 6.
	rolls.nights["ev-blood"][DayDate{Year: 6, Month: 1, Day: 3}] = RollNight{Year: 6, Month: 1, Day: 3, Total: intp(18), Source: RollNightAhead}

	got, err := svc.GetEventRoll(context.Background(), "ev-blood", "cal", "camp", 5, player)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config != nil || got.Upcoming != nil || len(got.Nights) != 1 || got.Nights[0].Players != "The moon runs red." {
		t.Fatalf("player year 5 = %+v", got)
	}
	future, err := svc.GetEventRoll(context.Background(), "ev-blood", "cal", "camp", 6, player)
	if err != nil || len(future.Nights) != 0 {
		t.Fatalf("a player saw a future night: %+v (err %v)", future, err)
	}
	month, err := svc.ListMonthRolls(context.Background(), "cal", "camp", 6, 1, player)
	if err != nil || len(month) != 0 {
		t.Fatalf("a player's month read showed a future night: %+v (err %v)", month, err)
	}
}

// A night that arrives unrolled is rolled by the first read, and only once.
func TestRollOnTheNight(t *testing.T) {
	cfg := bloodnight()
	_ = cfg.Normalize()
	svc, rolls := rollFixture(t, &cfg, nil)
	ctx := context.Background()
	if _, err := svc.ListMonthRolls(ctx, "cal", "camp", 5, 2, player); err != nil {
		t.Fatal(err)
	}
	arrived := DayDate{Year: 5, Month: 2, Day: 3}
	n, ok := rolls.nights["ev-blood"][arrived]
	if !ok || n.Source != RollNightOnDay || n.Total == nil {
		t.Fatalf("the arrived night was not rolled on the night: %+v", rolls.nights["ev-blood"])
	}
	if _, ok := rolls.nights["ev-blood"][DayDate{Year: 5, Month: 1, Day: 3}]; ok {
		t.Error("a night outside the month read should be left for its own month")
	}
	first := *n.Total
	if _, err := svc.GetEventRoll(ctx, "ev-blood", "cal", "camp", 5, owner); err != nil {
		t.Fatal(err)
	}
	if got := rolls.nights["ev-blood"][arrived]; *got.Total != first {
		t.Error("a second read rolled the night again")
	}
	// The 3rd of month one is 37 days back, past the window: an old
	// calendar does not fill its history the first time someone looks.
	if len(rolls.nights["ev-blood"]) != 1 {
		t.Errorf("only the arrived night within the window should be rolled, got %v", rolls.nights["ev-blood"])
	}
}

func TestDmOnlyEventRollIsHiddenFromPlayers(t *testing.T) {
	cfg := bloodnight()
	svc, _ := rollFixture(t, &cfg, func(e *Event) { e.Visibility = "dm_only" })
	_, err := svc.GetEventRoll(context.Background(), "ev-blood", "cal", "camp", 5, player)
	if apperror.SafeCode(err) != http.StatusNotFound {
		t.Fatalf("a player reading a Director-only event's roll = %v, want 404", err)
	}
}

func TestTableRollNeedsTheTable(t *testing.T) {
	svc, _ := rollFixture(t, nil, nil)
	cfg := &RollConfig{Source: RollSourceTable, TableID: "omens"}
	_, err := svc.SetEventRoll(context.Background(), "ev-blood", "cal", "camp", cfg, 5, owner)
	if apperror.SafeCode(err) != http.StatusUnprocessableEntity {
		t.Fatalf("a table roll with no table source = %v, want 422", err)
	}
	svc.SetRollTableSource(fakeTables{"omens": {Name: "Omens", Entries: []RollTableEntry{{Name: "Crows", Brief: "Death is near.", Weight: 1}}}})
	got, err := svc.SetEventRoll(context.Background(), "ev-blood", "cal", "camp", cfg, 5, owner)
	if err != nil || got.Config == nil {
		t.Fatalf("set table roll: %+v %v", got, err)
	}
	rolled, err := svc.RollEventNights(context.Background(), "ev-blood", "cal", "camp", RollNightsInput{Year: 6}, owner)
	if err != nil || len(rolled.Nights) != 2 || rolled.Nights[0].Result != "Crows" || rolled.Nights[0].Director != "Death is near." {
		t.Fatalf("table year roll = %+v %v", rolled, err)
	}
}

// An event with no roll yet lists its nights for the Director, so adding a
// roll shows what it will cover, and nothing for a player.
func TestNoRollYetListsNightsForTheDirector(t *testing.T) {
	svc, _ := rollFixture(t, nil, nil)
	got, err := svc.GetEventRoll(context.Background(), "ev-blood", "cal", "camp", 5, owner)
	if err != nil || got.Config != nil || len(got.Upcoming) != 2 || len(got.Nights) != 0 {
		t.Fatalf("director view with no roll = %+v %v", got, err)
	}
	got, err = svc.GetEventRoll(context.Background(), "ev-blood", "cal", "camp", 5, player)
	if err != nil || len(got.Upcoming) != 0 {
		t.Fatalf("player view with no roll = %+v %v", got, err)
	}
}

type fakeTables map[string]*RollTable

func (f fakeTables) RollTable(_ context.Context, _, id string) (*RollTable, error) { return f[id], nil }

// Roll refuses an expression built past the limits without ParseDice.
func TestDiceRollRefusesOutOfRange(t *testing.T) {
	for _, d := range []DiceExpr{{Count: maxRollDiceCount + 1, Sides: 6}, {Count: 0, Sides: 6}, {Count: 1, Sides: 1}} {
		if _, _, err := d.Roll(); err == nil {
			t.Errorf("%+v rolled", d)
		}
	}
}
