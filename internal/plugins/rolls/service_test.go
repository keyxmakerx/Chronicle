package rolls

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeRepo keeps rolls in memory and applies visibleTo's rule in Go.
type fakeRepo struct {
	rolls []Roll
	look  string
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

func (f *fakeRepo) Look(context.Context, string) (string, error) { return f.look, nil }
func (f *fakeRepo) SetLook(_ context.Context, _ string, l string) error {
	f.look = l
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

func TestSettings(t *testing.T) {
	s, _, _ := newTestService()
	ctx := context.Background()
	got, _ := s.Settings(ctx, "c1")
	if got.Look != LookQuick {
		t.Fatalf("default look = %q", got.Look)
	}
	if _, err := s.SetSettings(ctx, "c1", player, Settings{Look: LookRune}); status(err) != http.StatusForbidden {
		t.Fatalf("a player set the table's dice: %v", err)
	}
	if _, err := s.SetSettings(ctx, "c1", director, Settings{Look: "gold"}); status(err) != http.StatusBadRequest {
		t.Fatalf("unknown look: %v", err)
	}
	if _, err := s.SetSettings(ctx, "c1", director, Settings{Look: LookNone}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Settings(ctx, "c1"); got.Look != LookNone {
		t.Fatalf("look = %q", got.Look)
	}
}
