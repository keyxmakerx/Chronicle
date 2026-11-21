package rolls

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// fakeRepo keeps rolls in memory and applies visibleTo's rule in Go.
type fakeRepo struct {
	rolls    []Roll
	settings Settings
	has      bool
	saved    [][]string
}

func (f *fakeRepo) Insert(_ context.Context, r Roll) (string, bool, error) {
	for _, x := range f.rolls {
		if r.OriginID != "" && x.CampaignID == r.CampaignID && x.Origin == r.Origin && x.OriginID == r.OriginID {
			return x.ID, true, nil
		}
	}
	f.rolls = append(f.rolls, r)
	return r.ID, false, nil
}

func (f *fakeRepo) List(_ context.Context, campaignID string, v Viewer, _ string, limit int) ([]Roll, error) {
	var out []Roll
	for i := len(f.rolls) - 1; i >= 0 && len(out) < limit; i-- {
		r := f.rolls[i]
		if r.CampaignID == campaignID {
			out = append(out, r) // deliberately unfiltered: the service must filter
		}
	}
	return out, nil
}

func (f *fakeRepo) Settings(context.Context, string) (Settings, error) {
	if !f.has {
		return Settings{PlayersPick: true}, nil
	}
	return f.settings, nil
}

func (f *fakeRepo) SaveSettings(_ context.Context, _ string, s Settings, changed []string) error {
	f.settings, f.has = s, true
	f.saved = append(f.saved, changed)
	return nil
}

type sent struct {
	v View
	a Audience
}

type fakePub struct{ got []sent }

func (p *fakePub) RollMade(_ string, v View, a Audience) { p.got = append(p.got, sent{v, a}) }

func newTestService() (*service, *fakeRepo, *fakePub) {
	repo, pub := &fakeRepo{}, &fakePub{}
	s := NewService(repo, pub).(*service)
	s.roll = func(n int) int { return n } // every die shows its highest face
	s.now = func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }
	return s, repo, pub
}

var (
	player   = Viewer{UserID: "u-player", Name: "Kai"}
	other    = Viewer{UserID: "u-other", Name: "Mara"}
	director = Viewer{UserID: "u-dm", Name: "Dee", Director: true}
)

func status(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func TestTableRollIsDecidedOnTheServer(t *testing.T) {
	s, repo, pub := newTestService()
	v, err := s.Table(context.Background(), "c1", player, TableInput{Formula: "2d10+2", Label: "Brutal Slam"})
	if err != nil {
		t.Fatal(err)
	}
	if *v.Total != 22 || len(v.Dice) != 2 || *v.Modifier != 2 || v.Mode != ModePublic || !v.Mine {
		t.Fatalf("view = %+v", v)
	}
	if len(repo.rolls) != 1 || repo.rolls[0].RollerName != "Kai" || repo.rolls[0].Origin != OriginChronicle {
		t.Fatalf("stored = %+v", repo.rolls)
	}
	if len(pub.got) != 1 || !pub.got[0].a.Everyone || pub.got[0].v.Mine {
		t.Fatalf("published = %+v", pub.got)
	}
}

func TestTableRefusals(t *testing.T) {
	tests := []struct {
		name string
		v    Viewer
		in   TableInput
		code int
	}{
		{"bad formula", player, TableInput{Formula: "2d7"}, http.StatusBadRequest},
		{"bad mode", player, TableInput{Formula: "d20", Mode: "everyone"}, http.StatusBadRequest},
		{"signed out", Viewer{}, TableInput{Formula: "d20"}, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, repo, _ := newTestService()
			_, err := s.Table(context.Background(), "c1", tt.v, tt.in)
			if status(err) != tt.code {
				t.Fatalf("err = %v, want %d", err, tt.code)
			}
			if len(repo.rolls) != 0 {
				t.Fatal("a refused roll was stored")
			}
		})
	}
}

func TestTableRollsAreLimited(t *testing.T) {
	s, _, _ := newTestService()
	for i := 0; i < tableBurst; i++ {
		if _, err := s.Table(context.Background(), "c1", player, TableInput{Formula: "d6"}); err != nil {
			t.Fatalf("roll %d: %v", i, err)
		}
	}
	if _, err := s.Table(context.Background(), "c1", player, TableInput{Formula: "d6"}); status(err) != http.StatusTooManyRequests {
		t.Fatalf("err = %v, want 429", err)
	}
	if _, err := s.Table(context.Background(), "c1", other, TableInput{Formula: "d6"}); err != nil {
		t.Fatalf("another player is not held back: %v", err)
	}
	s.now = func() time.Time { return time.Date(2026, 10, 28, 12, 2, 0, 0, time.UTC) }
	if _, err := s.Table(context.Background(), "c1", player, TableInput{Formula: "d6"}); err != nil {
		t.Fatalf("after the window: %v", err)
	}
}

// TestAudiences pins who each mode's live message may reach.
func TestAudiences(t *testing.T) {
	tests := []struct {
		name  string
		v     Viewer
		mode  string
		wants []Audience
		stub  []bool
	}{
		{"public", player, ModePublic, []Audience{{Everyone: true}}, []bool{false}},
		{"gm by a player", player, ModeGM, []Audience{{Users: []string{"u-player"}}}, []bool{false}},
		{"blind by a player", player, ModeBlind, []Audience{{DirectorsOnly: true}, {Users: []string{"u-player"}, Strict: true}}, []bool{false, true}},
		{"blind by a director is director-only", director, ModeBlind, []Audience{{Users: []string{"u-dm"}}}, []bool{false}},
		{"self", player, ModeSelf, []Audience{{Users: []string{"u-player"}, Strict: true}}, []bool{false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, pub := newTestService()
			if _, err := s.Table(context.Background(), "c1", tt.v, TableInput{Formula: "d20", Mode: tt.mode}); err != nil {
				t.Fatal(err)
			}
			if len(pub.got) != len(tt.wants) {
				t.Fatalf("sent %d messages, want %d: %+v", len(pub.got), len(tt.wants), pub.got)
			}
			for i, w := range tt.wants {
				g := pub.got[i]
				if g.a.Everyone != w.Everyone || g.a.DirectorsOnly != w.DirectorsOnly || g.a.Strict != w.Strict || len(g.a.Users) != len(w.Users) || (len(w.Users) > 0 && g.a.Users[0] != w.Users[0]) {
					t.Errorf("message %d audience = %+v, want %+v", i, g.a, w)
				}
				if g.v.Hidden != tt.stub[i] {
					t.Errorf("message %d hidden = %v, want %v", i, g.v.Hidden, tt.stub[i])
				}
				if g.v.Hidden && (g.v.Total != nil || g.v.Dice != nil || g.v.Modifier != nil) {
					t.Errorf("a blind stub carries its result: %+v", g.v)
				}
			}
		})
	}
}

func TestListShowsEachViewerOnlyTheirs(t *testing.T) {
	s, _, _ := newTestService()
	ctx := context.Background()
	for _, m := range []string{ModePublic, ModeGM, ModeBlind, ModeSelf} {
		if _, err := s.Table(ctx, "c1", player, TableInput{Formula: "d20", Label: m, Mode: m}); err != nil {
			t.Fatal(err)
		}
	}
	labels := func(v Viewer) map[string]View {
		p, err := s.List(ctx, "c1", v, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]View{}
		for _, r := range p.Rolls {
			out[r.Label] = r
		}
		return out
	}
	mine := labels(player)
	if len(mine) != 4 || !mine[ModeBlind].Hidden || mine[ModeBlind].Total != nil || mine[ModeSelf].Hidden {
		t.Errorf("the roller sees %+v", mine)
	}
	dm := labels(director)
	if len(dm) != 3 || dm[ModeBlind].Hidden || dm[ModeBlind].Total == nil {
		t.Errorf("the director sees %+v", dm)
	}
	if _, ok := dm[ModeSelf]; ok {
		t.Error("a self roll reached the director")
	}
	others := labels(other)
	if len(others) != 1 || others[ModePublic].Total == nil {
		t.Errorf("another player sees %+v", others)
	}
}

func TestListPagesOldestFirst(t *testing.T) {
	s, _, _ := newTestService()
	ctx := context.Background()
	for _, l := range []string{"a", "b", "c"} {
		if _, err := s.Table(ctx, "c1", player, TableInput{Formula: "d6", Label: l}); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.List(ctx, "c1", player, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !p.More || len(p.Rolls) != 2 || p.Rolls[0].Label != "b" || p.Rolls[1].Label != "c" {
		t.Fatalf("page = %+v", p)
	}
}

func TestReportFromFoundry(t *testing.T) {
	ctx := context.Background()
	good := ReportInput{Origin: OriginFoundry, OriginID: "msg1", RollerID: "u-player", RollerName: "Jun", Formula: "2d10+1",
		Dice: []Die{{10, 3}, {10, 4}}, Total: 8, Mode: ModePublic}

	t.Run("records once and replays harmlessly", func(t *testing.T) {
		s, repo, pub := newTestService()
		id, dup, err := s.Report(ctx, "c1", good)
		if err != nil || dup {
			t.Fatalf("first: %v %v", err, dup)
		}
		id2, dup2, err := s.Report(ctx, "c1", good)
		if err != nil || !dup2 || id2 != id {
			t.Fatalf("replay: %v %v %s", err, dup2, id2)
		}
		if len(repo.rolls) != 1 || len(pub.got) != 1 {
			t.Fatalf("stored %d, sent %d", len(repo.rolls), len(pub.got))
		}
		r := repo.rolls[0]
		if r.Source != SourceFoundry || r.Modifier != 1 || r.Label != "2d10+1" {
			t.Fatalf("stored %+v", r)
		}
	})

	bad := []struct {
		name string
		edit func(*ReportInput)
	}{
		{"self", func(r *ReportInput) { r.Mode = ModeSelf }},
		{"no origin id", func(r *ReportInput) { r.OriginID = "" }},
		{"value off the die", func(r *ReportInput) { r.Dice = []Die{{6, 7}} }},
		{"one-sided die", func(r *ReportInput) { r.Dice = []Die{{1, 1}} }},
		{"too many dice", func(r *ReportInput) { r.Dice = make([]Die, MaxDice+1) }},
		{"total out of range", func(r *ReportInput) { r.Total = MaxTotal + 1 }},
		{"unknown origin", func(r *ReportInput) { r.Origin = OriginChronicle }},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			s, repo, _ := newTestService()
			in := good
			tt.edit(&in)
			if _, _, err := s.Report(ctx, "c1", in); status(err) != http.StatusBadRequest {
				t.Fatalf("err = %v, want 400", err)
			}
			if len(repo.rolls) != 0 {
				t.Fatal("a refused report was stored")
			}
		})
	}

	t.Run("unlinked gm roll goes to directors only", func(t *testing.T) {
		s, _, pub := newTestService()
		in := good
		in.RollerID, in.Mode = "", ModeGM
		if _, _, err := s.Report(ctx, "c1", in); err != nil {
			t.Fatal(err)
		}
		if len(pub.got) != 1 || !pub.got[0].a.DirectorsOnly {
			t.Fatalf("sent %+v", pub.got)
		}
	})
}

func TestLogFromAnotherPlugin(t *testing.T) {
	s, repo, pub := newTestService()
	err := s.Log(context.Background(), "c1", LogInput{Source: SourceCrafting, RollerID: "u-player", RollerName: "Vessa",
		Label: "Project roll", Formula: "2d10", Dice: []Die{{10, 5}, {10, 6}}, Modifier: 2, Total: 13, Outcome: "14 of 30"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.rolls) != 1 || repo.rolls[0].Mode != ModePublic || len(pub.got) != 1 || pub.got[0].v.Outcome != "14 of 30" {
		t.Fatalf("stored %+v sent %+v", repo.rolls, pub.got)
	}
}

// Record is Log that tells the caller which row its roll became.
func TestRecordReturnsTheLoggedRollsID(t *testing.T) {
	s, repo, pub := newTestService()
	id, err := s.Record(context.Background(), "c1", LogInput{Source: SourceShop, RollerName: "Thorin", Label: "Haggle: Potion",
		Formula: "1d20", Dice: []Die{{20, 18}}, Modifier: 2, Total: 20, Outcome: "20% off", From: "The Gilded Cup"})
	if err != nil || id == "" {
		t.Fatalf("id %q err %v", id, err)
	}
	if len(repo.rolls) != 1 || repo.rolls[0].ID != id || repo.rolls[0].Source != SourceShop || repo.rolls[0].Mode != ModePublic || len(pub.got) != 1 {
		t.Fatalf("stored %+v sent %+v", repo.rolls, pub.got)
	}
	if _, err := s.Record(context.Background(), "c1", LogInput{Mode: "loud"}); err == nil {
		t.Error("an unknown mode was recorded")
	}
}

func TestSettings(t *testing.T) {
	s, _, _ := newTestService()
	ctx := context.Background()
	got, _ := s.Settings(ctx, "c1")
	if got.Look != LookQuick || !got.PlayersPick || got.DiceSet != "" || len(got.DiceOffered) != 0 {
		t.Fatalf("defaults = %+v", got)
	}
	if _, err := s.SetSettings(ctx, "c1", player, UpdateSettingsInput{Look: patch.Of(LookRune)}); status(err) != http.StatusForbidden {
		t.Fatalf("a player set the table's dice: %v", err)
	}
	if _, err := s.SetSettings(ctx, "c1", director, UpdateSettingsInput{Look: patch.Of("gold")}); status(err) != http.StatusBadRequest {
		t.Fatalf("unknown look: %v", err)
	}
	if _, err := s.SetSettings(ctx, "c1", director, UpdateSettingsInput{Look: patch.Of(LookNone)}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Settings(ctx, "c1"); got.Look != LookNone {
		t.Fatalf("look = %q", got.Look)
	}
}

func TestSetSettingsValidation(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateSettingsInput
		want int // 0 = accepted
	}{
		{"a known set", UpdateSettingsInput{DiceSet: patch.Of("oak")}, 0},
		{"the default set, empty", UpdateSettingsInput{DiceSet: patch.Of("")}, 0},
		{"an unknown set", UpdateSettingsInput{DiceSet: patch.Of("plastic")}, http.StatusBadRequest},
		{"a path as a set", UpdateSettingsInput{DiceSet: patch.Of("../x")}, http.StatusBadRequest},
		{"a colour slug", UpdateSettingsInput{DiceColour: patch.Of("walnut-2")}, 0},
		{"a colour with spaces", UpdateSettingsInput{DiceColour: patch.Of("Dark Red")}, http.StatusBadRequest},
		{"a colour too long", UpdateSettingsInput{DiceColour: patch.Of("abcdefghijklmnopqrstuvwxy")}, http.StatusBadRequest},
		{"a crit success", UpdateSettingsInput{CritSucc: patch.Of("storm")}, 0},
		{"a fail crit as a success", UpdateSettingsInput{CritSucc: patch.Of("grasp")}, http.StatusBadRequest},
		{"a crit fail", UpdateSettingsInput{CritFail: patch.Of("abyss")}, 0},
		{"a success crit as a fail", UpdateSettingsInput{CritFail: patch.Of("phoenix")}, http.StatusBadRequest},
		{"quality hd", UpdateSettingsInput{Quality: patch.Of(QualityHD)}, 0},
		{"quality unknown", UpdateSettingsInput{Quality: patch.Of("ultra")}, http.StatusBadRequest},
		{"offered sets", UpdateSettingsInput{DiceOffered: patch.Of([]string{"jewelled", "oak"})}, 0},
		{"an unknown offered set", UpdateSettingsInput{DiceOffered: patch.Of([]string{"jewelled", "plastic"})}, http.StatusBadRequest},
		{"offered without the campaign's set", UpdateSettingsInput{DiceOffered: patch.Of([]string{"oak"})}, http.StatusBadRequest},
		{"the campaign's set moved off the offer", UpdateSettingsInput{DiceSet: patch.Of("bone"), DiceOffered: patch.Of([]string{"jewelled"})}, http.StatusBadRequest},
		{"the campaign's set joins the offer", UpdateSettingsInput{DiceSet: patch.Of("bone"), DiceOffered: patch.Of([]string{"bone"})}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, _ := newTestService()
			_, err := s.SetSettings(context.Background(), "c1", director, tt.in)
			if tt.want == 0 && err != nil || tt.want != 0 && status(err) != tt.want {
				t.Fatalf("got %v, want status %d", err, tt.want)
			}
		})
	}
}

func TestSetSettingsIsPartial(t *testing.T) {
	s, repo, _ := newTestService()
	ctx := context.Background()
	if _, err := s.SetSettings(ctx, "c1", director, UpdateSettingsInput{
		Look: patch.Of(LookRune), DiceSet: patch.Of("oak"), DiceColour: patch.Of("walnut"), CritSucc: patch.Of("dawn"),
		CritFail: patch.Of("frost"), Quality: patch.Of(QualityBasic), PlayersPick: patch.Of(false),
		DiceOffered: patch.Of([]string{"oak", "bone"}),
	}); err != nil {
		t.Fatal(err)
	}
	// Only the colour is sent: everything else stays, and only it is written.
	got, err := s.SetSettings(ctx, "c1", director, UpdateSettingsInput{DiceColour: patch.Of("pale")})
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{Look: LookRune, DiceSet: "oak", DiceColour: "pale", CritSucc: "dawn", CritFail: "frost", Quality: QualityBasic, PlayersPick: false, DiceOffered: []string{"oak", "bone"}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %+v want %+v", *got, want)
	}
	if last := repo.saved[len(repo.saved)-1]; !reflect.DeepEqual(last, []string{"diceColour"}) {
		t.Fatalf("wrote %v", last)
	}
	// An explicit null keeps the value; an empty string puts the default back.
	var raw UpdateSettingsInput
	if err := json.Unmarshal([]byte(`{"diceSet":null,"critSucc":"","diceOffered":null}`), &raw); err != nil {
		t.Fatal(err)
	}
	got, err = s.SetSettings(ctx, "c1", director, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.DiceSet != "oak" || got.CritSucc != "" || len(got.DiceOffered) != 2 {
		t.Fatalf("null or empty handled wrong: %+v", *got)
	}
	// Offering every set is stored as "every set".
	got, err = s.SetSettings(ctx, "c1", director, UpdateSettingsInput{DiceOffered: patch.Of(append([]string{"oak"}, DiceSetKeys...))})
	if err != nil || len(got.DiceOffered) != 0 {
		t.Fatalf("all sets offered: %+v %v", got, err)
	}
}

// The server's lists must name exactly what the browser's engine can draw.
func TestDiceKeysMatchEngine(t *testing.T) {
	src, err := os.ReadFile("../../../static/js/dice/engine.js")
	if err != nil {
		t.Fatalf("read engine.js: %v", err)
	}
	keys := func(re string) []string {
		m := regexp.MustCompile(re).FindStringSubmatch(string(src))
		if m == nil {
			t.Fatalf("engine.js has no match for %s", re)
		}
		var out []string
		for _, k := range regexp.MustCompile(`'([a-z]+)'`).FindAllStringSubmatch(m[1], -1) {
			out = append(out, k[1])
		}
		return out
	}
	tests := []struct {
		name string
		re   string
		want []string
	}{
		{"sets", `var SET_KEYS = \[([^\]]*)\]`, DiceSetKeys},
		{"crit successes", `succ: \[([^\]]*)\]`, CritSuccKeys},
		{"crit fails", `fail: \[([^\]]*)\]`, CritFailKeys},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keys(tt.re); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("engine has %v, server has %v", got, tt.want)
			}
		})
	}
}

// A pick is stored by name, and the sets on offer as one comma-separated
// list, so a longer list of sets has to still fit the columns.
func TestDiceKeysFitTheirColumns(t *testing.T) {
	src, err := os.ReadFile("migrations/002_roll_dice_looks.up.sql")
	if err != nil {
		t.Fatalf("read the migration: %v", err)
	}
	width := func(col string) int {
		m := regexp.MustCompile(col + ` +VARCHAR\((\d+)\)`).FindStringSubmatch(string(src))
		if m == nil {
			t.Fatalf("no %s column in the migration", col)
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}
	for col, keys := range map[string][]string{"dice_set": DiceSetKeys, "crit_succ": CritSuccKeys, "crit_fail": CritFailKeys} {
		for _, k := range keys {
			if len(k) > width(col) {
				t.Errorf("%q is longer than %s holds (%d)", k, col, width(col))
			}
		}
	}
	// Every set offered is stored as an empty list, so the longest stored
	// list is every set but the shortest one.
	shortest := len(DiceSetKeys[0])
	for _, k := range DiceSetKeys {
		shortest = min(shortest, len(k))
	}
	if longest := len(strings.Join(DiceSetKeys, ",")) - shortest - 1; longest > width("dice_offered") {
		t.Errorf("the longest list of offered sets is %d characters, dice_offered holds %d", longest, width("dice_offered"))
	}
}
