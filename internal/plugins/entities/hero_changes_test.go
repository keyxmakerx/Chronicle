package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

var heroNow = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) *time.Time { t := heroNow.Add(-d); return &t }

func TestHeroChangeWindow(t *testing.T) {
	day := 24 * time.Hour
	making := &campaigns.HeroChangeRule{AfterMakingDays: 8}
	level := &campaigns.HeroChangeRule{AfterLevel: true, FromLevel: 2, AfterLevelDays: 8}
	fromThree := &campaigns.HeroChangeRule{AfterLevel: true, FromLevel: 3, AfterLevelDays: 8}
	both := &campaigns.HeroChangeRule{AfterMakingDays: 3, AfterLevel: true, AfterLevelDays: 8}
	cases := []struct {
		name      string
		rule      *campaigns.HeroChangeRule
		st        HeroChangeState
		made      time.Time
		level     int
		open      bool
		daysLeft  int
		nextLevel int
		reopened  bool
	}{
		{"no rule is any time", nil, HeroChangeState{}, heroNow.Add(-400 * day), 1, true, 0, 0, false},
		{"inside the days after making", making, HeroChangeState{}, heroNow.Add(-2 * day), 1, true, 6, 0, false},
		{"past the days after making", making, HeroChangeState{}, heroNow.Add(-9 * day), 1, false, 0, 0, false},
		{"a new level opens it for its days", level, HeroChangeState{Level: 2, LevelAt: ago(3 * day)}, heroNow.Add(-90 * day), 2, true, 5, 0, false},
		{"the level's days ran out", level, HeroChangeState{Level: 2, LevelAt: ago(9 * day)}, heroNow.Add(-90 * day), 2, false, 0, 3, false},
		{"a level not recorded opens nothing", level, HeroChangeState{}, heroNow.Add(-90 * day), 2, false, 0, 3, false},
		{"a recorded level the hero has since left opens nothing", level, HeroChangeState{Level: 3, LevelAt: ago(day)}, heroNow.Add(-90 * day), 2, false, 0, 3, false},
		{"below the first level that opens", fromThree, HeroChangeState{Level: 2, LevelAt: ago(day)}, heroNow.Add(-90 * day), 2, false, 0, 3, false},
		{"level 1 waits for level 2", level, HeroChangeState{}, heroNow.Add(-90 * day), 1, false, 0, 2, false},
		{"both: the later end wins", both, HeroChangeState{Level: 2, LevelAt: ago(day)}, heroNow.Add(-day), 2, true, 7, 0, false},
		{"a Director's reopening", making, HeroChangeState{ReopenedUntil: func() *time.Time { t := heroNow.Add(4 * day); return &t }()}, heroNow.Add(-90 * day), 1, true, 4, 0, true},
		{"an old reopening is spent", making, HeroChangeState{ReopenedUntil: ago(day)}, heroNow.Add(-90 * day), 1, false, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := heroChangeWindow(tc.rule, tc.st, tc.made, tc.level, "Tess", heroNow)
			if l.Open != tc.open {
				t.Fatalf("open = %v, want %v (%+v)", l.Open, tc.open, l)
			}
			if got := l.DaysLeft(heroNow); got != tc.daysLeft {
				t.Errorf("days left = %d, want %d", got, tc.daysLeft)
			}
			if l.NextLevel != tc.nextLevel {
				t.Errorf("next level = %d, want %d", l.NextLevel, tc.nextLevel)
			}
			if l.Reopened != tc.reopened {
				t.Errorf("reopened = %v, want %v", l.Reopened, tc.reopened)
			}
			if l.Ruled != (tc.rule != nil) {
				t.Errorf("ruled = %v", l.Ruled)
			}
			if !l.Open && l.Next == "" {
				t.Error("a shut lock must say what opens it")
			}
		})
	}
}

func TestHeroLockedChanges(t *testing.T) {
	picks := builderPicks{"class": {}, "ancestry": {}}
	e := &Entity{FieldsData: map[string]any{
		"class": "Fury", "ancestry": "", "ancestry" + heroOptionsSuffix: "", "class" + heroOptionsSuffix: `["Grit","Brawn"]`, "culture": "Wode",
	}}
	labels := map[string]string{"class": "Class", "ancestry": "Ancestry"}
	cases := []struct {
		name  string
		patch map[string]any
		want  []string
	}{
		{"a changed pick", map[string]any{"class": "Tactician"}, []string{"Class: Fury → Tactician"}},
		{"the same pick in another case", map[string]any{"class": " fury "}, nil},
		{"filling a blank is not a change", map[string]any{"ancestry": "Dwarf"}, nil},
		{"clearing a pick is a change", map[string]any{"class": nil}, []string{"Class: Fury → "}},
		{"what an entry bought, in another order", map[string]any{"class" + heroOptionsSuffix: `["Brawn","Grit"]`}, nil},
		{"what an entry bought, changed", map[string]any{"class" + heroOptionsSuffix: `["Grit"]`}, []string{"Class choices: Brawn, Grit → Grit"}},
		{"buying for a blank is not a change", map[string]any{"ancestry" + heroOptionsSuffix: `["Tough"]`}, nil},
		{"a field that is not a step is ignored", map[string]any{"culture": "Other"}, nil},
		{"a list is never in the locked set", map[string]any{"skills_json": `["Sneak"]`}, nil},
		{"a story is never in the locked set", map[string]any{"backstory": "A new tale"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, ch := range heroLockedChanges(e, tc.patch, picks, labels) {
				got = append(got, ch.Label+": "+ch.From+" → "+ch.To)
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A multi step short of its allowance can be finished while the picks are
// shut; what it already holds can't be swapped or dropped.
func TestHeroLockedChangesMulti(t *testing.T) {
	picks := builderPicksFrom(multiPlan())
	labels := map[string]string{"subclass": "Subclass"}
	hero := func(sub string) *Entity {
		return &Entity{FieldsData: map[string]any{"class": "Conduit", "subclass": sub}}
	}
	cases := []struct {
		name  string
		e     *Entity
		patch map[string]any
		want  []string
	}{
		{"the second domain finishes the step", hero("War"), map[string]any{"subclass": "Life, War"}, nil},
		{"the same domains in Foundry's order", hero("Life, War"), map[string]any{"subclass": "War, Life"}, nil},
		{"a held domain swapped is a change", hero("War"), map[string]any{"subclass": "Life"}, []string{"Subclass: War → Life"}},
		{"a full step swapped is a change", hero("Life, War"), map[string]any{"subclass": "Life, Fate"}, []string{"Subclass: Life, War → Life, Fate"}},
		{"dropping a domain is a change", hero("Life, War"), map[string]any{"subclass": "Life"}, []string{"Subclass: Life, War → Life"}},
		{"past the allowance is a change", hero("War"), map[string]any{"subclass": "Life, War, Fate"}, []string{"Subclass: War → Life, War, Fate"}},
		{"a class with one subclass gets no second", &Entity{FieldsData: map[string]any{"class": "Fury", "subclass": "Reaver"}},
			map[string]any{"subclass": "Reaver, Berserker"}, []string{"Subclass: Reaver → Reaver, Berserker"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, ch := range heroLockedChanges(tc.e, tc.patch, picks, labels) {
				got = append(got, ch.Label+": "+ch.From+" → "+ch.To)
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHeroLevel(t *testing.T) {
	cases := []struct {
		v    any
		want int
	}{{float64(3), 3}, {4, 4}, {" 5 ", 5}, {"x", 0}, {nil, 0}}
	for _, tc := range cases {
		if got := heroLevel(map[string]any{HeroLevelKey: tc.v}); got != tc.want {
			t.Errorf("heroLevel(%v) = %d, want %d", tc.v, got, tc.want)
		}
	}
}

func TestHeroRuleSummary(t *testing.T) {
	cases := []struct {
		rule *campaigns.HeroChangeRule
		want string
	}{
		{nil, "Players can change their hero's picks any time."},
		{&campaigns.HeroChangeRule{AfterMakingDays: 1}, "Players can change their hero's picks for the first 1 day after making it."},
		{&campaigns.HeroChangeRule{AfterLevel: true, FromLevel: 2, AfterLevelDays: 8}, "Players can change their hero's picks for 8 days after each new level."},
		{&campaigns.HeroChangeRule{AfterMakingDays: 7, AfterLevel: true, FromLevel: 4, AfterLevelDays: 3},
			"Players can change their hero's picks for the first 7 days after making it, and for 3 days after each new level from level 4 on."},
	}
	for _, tc := range cases {
		if got := heroRuleSummary(tc.rule); got != tc.want {
			t.Errorf("got %q\nwant %q", got, tc.want)
		}
	}
}

func TestHeroLockState(t *testing.T) {
	until := heroNow.Add(50 * time.Hour)
	cases := []struct {
		l    HeroLock
		want string
	}{
		{HeroLock{Open: true}, "Open"},
		{HeroLock{Open: true, Until: &until}, "Open 3 days more"},
		{HeroLock{Open: true, Until: &until, Reopened: true}, "Open 3 days more, reopened by a Director"},
		{HeroLock{NextLevel: 4}, "Locked until level 4"},
		{HeroLock{}, "Locked"},
	}
	for _, tc := range cases {
		if got := heroLockState(tc.l, heroNow); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

// fakeHeroRules is a campaign's stored rule.
type fakeHeroRules struct {
	rule  *campaigns.HeroChangeRule
	saved *campaigns.HeroChangeRule
	saves int
}

func (f *fakeHeroRules) HeroChangeRule(context.Context, string) (*campaigns.HeroChangeRule, error) {
	return f.rule, nil
}

func (f *fakeHeroRules) SaveHeroChangeRule(_ context.Context, _ string, r *campaigns.HeroChangeRule) error {
	f.saved, f.saves = r, f.saves+1
	return nil
}

// fakeHeroStore keeps state in memory.
type fakeHeroStore struct {
	states map[string]HeroChangeState
	levels []int
	reopen *time.Time
}

func (f *fakeHeroStore) Get(_ context.Context, id string) (HeroChangeState, error) {
	return f.states[id], nil
}

func (f *fakeHeroStore) List(context.Context, string) (map[string]HeroChangeState, error) {
	return f.states, nil
}

func (f *fakeHeroStore) RecordLevel(_ context.Context, _, _ string, level int, _ time.Time) error {
	f.levels = append(f.levels, level)
	return nil
}

func (f *fakeHeroStore) Reopen(_ context.Context, _, _ string, until time.Time, _ string) error {
	f.reopen = &until
	return nil
}

// fakeHeroBell counts what rang.
type fakeHeroBell struct {
	rang   [][]HeroPickChange
	levels []int
}

func (f *fakeHeroBell) LevelRaised(_ context.Context, _ *Entity, _ string, level int) error {
	f.levels = append(f.levels, level)
	return nil
}

func (f *fakeHeroBell) HeroChanged(_ context.Context, _ *Entity, _ string, ch []HeroPickChange) error {
	f.rang = append(f.rang, ch)
	return nil
}

func newTestHeroChanges(rule *campaigns.HeroChangeRule) (*HeroChangeService, *fakeHeroStore, *fakeHeroBell) {
	st, bell := &fakeHeroStore{states: map[string]HeroChangeState{}}, &fakeHeroBell{}
	s := NewHeroChangeService(&fakeHeroRules{rule: rule}, st, bell)
	s.now = func() time.Time { return heroNow }
	return s, st, bell
}

func TestHeroChangeService_FieldsChanged(t *testing.T) {
	owner := "u1"
	cases := []struct {
		name       string
		owned      bool
		was, now   any
		wantLevels int
	}{
		{"a claimed hero going up records", true, float64(1), float64(2), 1},
		{"going down records nothing", true, float64(3), float64(2), 0},
		{"the same level records nothing", true, float64(2), float64(2), 0},
		{"an unclaimed page records nothing", false, float64(1), float64(2), 0},
		{"level 1 records nothing", true, nil, float64(1), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, st, _ := newTestHeroChanges(nil)
			e := &Entity{ID: "e1", CampaignID: "c1"}
			if tc.owned {
				e.OwnerUserID = &owner
			}
			if err := s.FieldsChanged(context.Background(), e, map[string]any{HeroLevelKey: tc.was}, map[string]any{HeroLevelKey: tc.now}); err != nil {
				t.Fatal(err)
			}
			if len(st.levels) != tc.wantLevels {
				t.Fatalf("levels recorded = %v", st.levels)
			}
		})
	}
}

func TestHeroLevelRaised(t *testing.T) {
	hero := &Entity{ID: "e1", FieldsData: map[string]any{HeroLevelKey: float64(2), "class": "Fury"}}
	cases := []struct {
		name  string
		patch map[string]any
		want  int
	}{
		{"a raise", map[string]any{HeroLevelKey: float64(3)}, 3},
		{"a raise written as text", map[string]any{HeroLevelKey: "4"}, 4},
		{"the same level", map[string]any{HeroLevelKey: float64(2)}, 0},
		{"a lower level", map[string]any{HeroLevelKey: float64(1)}, 0},
		{"no level in the write", map[string]any{"class": "Censor"}, 0},
		{"the level cleared", map[string]any{HeroLevelKey: nil}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HeroLevelRaised(hero, tc.patch); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
	if got := HeroLevelRaised(&Entity{ID: "e2"}, map[string]any{HeroLevelKey: float64(1)}); got != 1 {
		t.Errorf("a hero with no level yet: got %d, want 1", got)
	}
}

func TestHeroChangeService_LevelRaised(t *testing.T) {
	s, _, bell := newTestHeroChanges(nil)
	e := &Entity{ID: "e1", CampaignID: "c1"}
	s.LevelRaised(context.Background(), e, "pat", 3)
	s.LevelRaised(context.Background(), e, "pat", 0) // no raise
	s.LevelRaised(context.Background(), e, "", 4)    // no acting player
	if len(bell.levels) != 1 || bell.levels[0] != 3 {
		t.Fatalf("rang %v, want only [3]", bell.levels)
	}
	var none *HeroChangeService
	none.LevelRaised(context.Background(), e, "pat", 3) // not wired: nothing, no panic
}

func TestHeroChangeService_HeldPicks(t *testing.T) {
	plan := &HeroPlan{Steps: []HeroStep{{FieldKey: "class", Label: "Class", Choices: []HeroChoice{{Name: "Fury"}, {Name: "Tactician"}}}}}
	shut := &campaigns.HeroChangeRule{AfterMakingDays: 1}
	made := heroNow.Add(-30 * 24 * time.Hour)
	e := &Entity{ID: "e1", CampaignID: "c1", Name: "Tess", CreatedAt: made, FieldsData: map[string]any{"class": "Fury"}}

	s, _, _ := newTestHeroChanges(shut)
	held, changes, err := s.HeldPicks(context.Background(), e, map[string]any{"class": "Tactician"}, plan, nil)
	if err != nil || len(changes) != 0 || !strings.Contains(held["class"], "locked") {
		t.Fatalf("a shut hero's change must be held: held=%v changes=%v err=%v", held, changes, err)
	}
	open, _, _ := newTestHeroChanges(nil)
	held, changes, err = open.HeldPicks(context.Background(), e, map[string]any{"class": "Tactician"}, plan, nil)
	if err != nil || len(held) != 0 || len(changes) != 1 {
		t.Fatalf("an open hero's change passes and is counted: held=%v changes=%v err=%v", held, changes, err)
	}
	var none *HeroChangeService
	if held, _, err := none.HeldPicks(context.Background(), e, map[string]any{"class": "Tactician"}, plan, nil); err != nil || len(held) != 0 {
		t.Fatal("an unwired service never holds")
	}
}

func TestHeroChangeService_Reopen(t *testing.T) {
	s, st, _ := newTestHeroChanges(&campaigns.HeroChangeRule{AfterMakingDays: 1})
	e := &Entity{ID: "e1", CampaignID: "c1"}
	for _, n := range []int{0, -1, HeroReopenMaxDays + 1} {
		assertAppError(t, s.Reopen(context.Background(), e, n, "dm"), http.StatusBadRequest)
	}
	if err := s.Reopen(context.Background(), e, 5, "dm"); err != nil {
		t.Fatal(err)
	}
	if st.reopen == nil || !st.reopen.Equal(heroNow.Add(5*24*time.Hour)) {
		t.Fatalf("reopened until %v", st.reopen)
	}
}

// The fields door refuses a claimed owner's change to a pick outside the
// Director's times, lets a blank be filled, rings the bell for a change it
// lets through, and never locks staff.
func TestUpdateFieldsAPI_HeroChangeTimes(t *testing.T) {
	owner := "user-owner"
	plan := &HeroPlan{Steps: []HeroStep{
		{FieldKey: "class", Label: "Class", Choices: []HeroChoice{{Name: "Fury"}, {Name: "Shadow"}}},
		{FieldKey: "kit", Label: "Kit", Choices: []HeroChoice{{Name: "Panther"}}},
	}}
	shut := &campaigns.HeroChangeRule{AfterMakingDays: 3}
	old := heroNow.Add(-30 * 24 * time.Hour)
	cases := []struct {
		name      string
		rule      *campaigns.HeroChangeRule
		role      campaigns.Role
		userID    string
		body      string
		status    int
		wantWrite bool
		wantBells int
	}{
		{"shut: a changed pick is refused", shut, campaigns.RolePlayer, owner, `{"fields_patch":{"class":"Shadow"}}`, 403, false, 0},
		{"shut: a blank can be filled", shut, campaigns.RolePlayer, owner, `{"fields_patch":{"kit":"Panther"}}`, 200, true, 0},
		{"shut: resending the same pick is fine", shut, campaigns.RolePlayer, owner, `{"fields_patch":{"class":"Fury"}}`, 200, true, 0},
		{"any time: a change rings the bell", nil, campaigns.RolePlayer, owner, `{"fields_patch":{"class":"Shadow"}}`, 200, true, 1},
		{"staff are never locked", shut, campaigns.RoleScribe, "user-scribe", `{"fields_patch":{"class":"Shadow"}}`, 200, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &identityStubSvc{entity: &Entity{ID: "e1", CampaignID: "c1", Name: "Hero", OwnerUserID: &owner,
				CreatedAt: old, FieldsData: map[string]any{"class": "Fury"}}}
			hc, _, bell := newTestHeroChanges(tc.rule)
			h := &Handler{service: svc, heroPlanner: &stubPlanner{plan: plan}, heroChanges: hc}
			wantStatus(t, runIdentityCallOn(t, h, (*Handler).UpdateFieldsAPI, tc.role, tc.userID, "c1", tc.body), tc.status)
			if (svc.writeCount > 0) != tc.wantWrite {
				t.Fatalf("write reached = %v, want %v", svc.writeCount > 0, tc.wantWrite)
			}
			if len(bell.rang) != tc.wantBells {
				t.Fatalf("bells = %d, want %d", len(bell.rang), tc.wantBells)
			}
		})
	}
}

// Only the Director team may set the times or reopen a hero; anyone else is
// answered as if the route were absent.
func TestHeroChangeRoutes_DirectorsOnly(t *testing.T) {
	owner := "user-owner"
	cases := []struct {
		name    string
		handler func(*Handler, *echo.Context) error
		body    string
		role    campaigns.Role
		status  int
	}{
		{"player may not set the times", (*Handler).SaveHeroChanges, "when=any", campaigns.RolePlayer, 404},
		{"scribe may not set the times", (*Handler).SaveHeroChanges, "when=any", campaigns.RoleScribe, 404},
		{"owner sets the times", (*Handler).SaveHeroChanges, "when=set&after_level=1&from_level=2&level_days=8", campaigns.RoleOwner, 303},
		{"player may not reopen", (*Handler).ReopenHeroChanges, "days=5", campaigns.RolePlayer, 404},
		{"owner reopens", (*Handler).ReopenHeroChanges, "days=5", campaigns.RoleOwner, 303},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &identityStubSvc{entity: &Entity{ID: "e1", CampaignID: "c1", Name: "Hero", OwnerUserID: &owner}}
			hc, st, _ := newTestHeroChanges(nil)
			rules := hc.rules.(*fakeHeroRules)
			h := &Handler{service: svc, heroChanges: hc}
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}, {Name: "eid", Value: "e1"}})
			c.Set("auth_user_id", "user-dm")
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tc.role})
			err := tc.handler(h, c)
			if tc.status == 303 {
				if err != nil || rec.Code != http.StatusSeeOther {
					t.Fatalf("want a redirect, got %d %v", rec.Code, err)
				}
				if rules.saves == 0 && st.reopen == nil {
					t.Fatal("nothing was stored")
				}
				return
			}
			assertAppError(t, err, tc.status)
			if rules.saves != 0 || st.reopen != nil {
				t.Fatal("a refused call stored something")
			}
		})
	}
}
