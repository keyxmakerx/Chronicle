package rolls

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// The campaign's own dice in these tests: Neon, crit animations of its own.
var neon = Settings{Look: LookRune, DiceSet: "neon", DiceColour: "blue", CritSucc: "storm", CritFail: "grasp", PlayersPick: true, DiceOffered: []string{}}

func withSettings(st Settings, mut func(*Settings)) Settings {
	st.DiceOffered = append([]string{}, st.DiceOffered...)
	mut(&st)
	return st
}

// The dice a roll carries are stored as JSON in roller_dice, so the longest
// names the engine has, with the longest colour key, must still fit.
func TestRollerDiceFitsItsColumn(t *testing.T) {
	longest := func(keys []string) string {
		out := ""
		for _, k := range keys {
			if len(k) > len(out) {
				out = k
			}
		}
		return out
	}
	b, err := json.Marshal(DiceLook{Set: longest(DiceSetKeys), Colour: strings.Repeat("c", 24), CritSucc: longest(CritSuccKeys), CritFail: longest(CritFailKeys), Own: true})
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("migrations/003_roll_member_dice.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`roller_dice +VARCHAR\((\d+)\)`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("no roller_dice column in the migration")
	}
	if n, _ := strconv.Atoi(m[1]); len(b) > n {
		t.Fatalf("the longest dice JSON is %d characters, roller_dice holds %d", len(b), n)
	}
}

// The rule every screen throws a roll by: the campaign's dice, with the
// roller's own pick laid over them only while the campaign lets players pick
// and only for sets still on offer.
func TestEffectiveDice(t *testing.T) {
	camp := DiceLook{Set: "neon", Colour: "blue", CritSucc: "storm", CritFail: "grasp"}
	clockwork := DicePick{Set: "clockwork", Colour: "brass"}
	tests := []struct {
		name string
		st   Settings
		pick DicePick
		want DiceLook
	}{
		{"no pick: the campaign's dice", neon, DicePick{}, camp},
		{"default only: picking is off, the pick is skipped", withSettings(neon, func(s *Settings) { s.PlayersPick = false }), clockwork, camp},
		{"players may choose: every set on offer", neon, clockwork,
			DiceLook{Set: "clockwork", Colour: "brass", CritSucc: "storm", CritFail: "grasp", Own: true}},
		{"players may choose: the set is on a short offer", withSettings(neon, func(s *Settings) { s.DiceOffered = []string{"neon", "clockwork"} }), clockwork,
			DiceLook{Set: "clockwork", Colour: "brass", CritSucc: "storm", CritFail: "grasp", Own: true}},
		{"pick off the offer: the campaign's set and colour", withSettings(neon, func(s *Settings) { s.DiceOffered = []string{"neon", "oak"} }), clockwork, camp},
		{"pick off the offer keeps the crits picked", withSettings(neon, func(s *Settings) { s.DiceOffered = []string{"neon"} }),
			DicePick{Set: "clockwork", Colour: "brass", CritSucc: "phoenix"},
			DiceLook{Set: "neon", Colour: "blue", CritSucc: "phoenix", CritFail: "grasp", Own: true}},
		{"crits only", neon, DicePick{CritFail: "abyss"}, DiceLook{Set: "neon", Colour: "blue", CritSucc: "storm", CritFail: "abyss", Own: true}},
		{"a set the engine no longer draws", neon, DicePick{Set: "plastic"}, camp},
		{"a crit from the wrong list", neon, DicePick{CritSucc: "grasp"}, camp},
		{"a colour that isn't a colour key", neon, DicePick{Set: "oak", Colour: "Dark Red"}, camp},
		{"the campaign on the defaults", Settings{PlayersPick: true}, DicePick{}, DiceLook{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveDice(tt.st, tt.pick); got != tt.want {
				t.Fatalf("effectiveDice = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A set taken off the offer after a player picked it: their rolls go back to
// the campaign's dice, the pick is kept, and offering the set again brings it
// back.
func TestPickRemovedLater(t *testing.T) {
	ctx := context.Background()
	s, repo, pub := newTestService()
	repo.settings, repo.has = withSettings(neon, func(*Settings) {}), true
	if _, err := s.SetMyDice(ctx, "c1", player, DicePick{Set: "clockwork", Colour: "brass"}); err != nil {
		t.Fatal(err)
	}
	thrownIn := func() string {
		t.Helper()
		pub.got = nil
		if _, err := s.Table(ctx, "c1", player, TableInput{Formula: "d20"}); err != nil {
			t.Fatal(err)
		}
		if len(pub.got) != 1 || pub.got[0].v.RollerDice == nil {
			t.Fatalf("sent %+v; want one roll carrying its dice", pub.got)
		}
		return pub.got[0].v.RollerDice.Set
	}
	if got := thrownIn(); got != "clockwork" {
		t.Fatalf("thrown in %q while offered, want clockwork", got)
	}
	if _, err := s.SetSettings(ctx, "c1", director, UpdateSettingsInput{DiceOffered: patch.Of([]string{"neon", "oak"})}); err != nil {
		t.Fatal(err)
	}
	if got := thrownIn(); got != "neon" {
		t.Fatalf("thrown in %q after the set left the offer, want the campaign's neon", got)
	}
	if mine, _ := s.MyDice(ctx, "c1", player); mine.Pick.Set != "clockwork" || mine.Effective.Set != "neon" || mine.Effective.Own {
		t.Fatalf("my dice = %+v; want the pick kept but not used", mine)
	}
	if _, err := s.SetSettings(ctx, "c1", director, UpdateSettingsInput{DiceOffered: patch.Of([]string{})}); err != nil {
		t.Fatal(err)
	}
	if got := thrownIn(); got != "clockwork" {
		t.Fatalf("thrown in %q after the set came back, want clockwork", got)
	}
	if _, err := s.SetSettings(ctx, "c1", director, UpdateSettingsInput{PlayersPick: patch.Of(false)}); err != nil {
		t.Fatal(err)
	}
	if got := thrownIn(); got != "neon" {
		t.Fatalf("thrown in %q with picking off, want the campaign's neon", got)
	}
}

// A player's pick is checked on the server against what the campaign offers.
func TestSetMyDice(t *testing.T) {
	short := withSettings(neon, func(s *Settings) { s.DiceOffered = []string{"neon", "oak"} })
	off := withSettings(neon, func(s *Settings) { s.PlayersPick = false })
	tests := []struct {
		name   string
		st     Settings
		v      Viewer
		pick   DicePick
		status int
		stored DicePick
	}{
		{"a set on offer", short, player, DicePick{Set: "oak", Colour: "walnut"}, 0, DicePick{Set: "oak", Colour: "walnut"}},
		{"a set off the offer", short, player, DicePick{Set: "clockwork"}, http.StatusBadRequest, DicePick{}},
		{"an unknown set", neon, player, DicePick{Set: "plastic"}, http.StatusBadRequest, DicePick{}},
		{"a bad colour", neon, player, DicePick{Set: "oak", Colour: "../x"}, http.StatusBadRequest, DicePick{}},
		{"a fail crit as a success", neon, player, DicePick{CritSucc: "grasp"}, http.StatusBadRequest, DicePick{}},
		{"a success crit as a fail", neon, player, DicePick{CritFail: "storm"}, http.StatusBadRequest, DicePick{}},
		{"crits only", neon, player, DicePick{CritSucc: "phoenix", CritFail: "abyss"}, 0, DicePick{CritSucc: "phoenix", CritFail: "abyss"}},
		{"a colour with no set follows the campaign's", neon, player, DicePick{Colour: "brass", CritSucc: "dawn"}, 0, DicePick{CritSucc: "dawn"}},
		{"picking is off", off, player, DicePick{Set: "neon"}, http.StatusForbidden, DicePick{}},
		{"picking is off, a Director too", off, director, DicePick{Set: "oak"}, http.StatusForbidden, DicePick{}},
		{"picking is off, going back to the campaign's", off, player, DicePick{}, 0, DicePick{}},
		{"signed out", neon, Viewer{}, DicePick{Set: "oak"}, http.StatusForbidden, DicePick{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, repo, _ := newTestService()
			repo.settings, repo.has = tt.st, true
			_, err := s.SetMyDice(context.Background(), "c1", tt.v, tt.pick)
			if status(err) != tt.status {
				t.Fatalf("status = %d (%v), want %d", status(err), err, tt.status)
			}
			if got := repo.picks["c1/"+tt.v.UserID]; got != tt.stored {
				t.Fatalf("stored %+v, want %+v", got, tt.stored)
			}
		})
	}
}

// Players never set the campaign's dice, the sets on offer or whether
// players may pick: the service refuses anyone but a Director, whatever the
// body carries, and nothing is written.
func TestOnlyDirectorsSetTheCampaignDice(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateSettingsInput
	}{
		{"the campaign's set", UpdateSettingsInput{DiceSet: patch.Of("clockwork")}},
		{"the campaign's colour", UpdateSettingsInput{DiceColour: patch.Of("brass")}},
		{"the sets on offer", UpdateSettingsInput{DiceOffered: patch.Of([]string{"neon", "clockwork"})}},
		{"every set on offer", UpdateSettingsInput{DiceOffered: patch.Of([]string{})}},
		{"whether players may pick", UpdateSettingsInput{PlayersPick: patch.Of(true)}},
		{"the crits", UpdateSettingsInput{CritSucc: patch.Of("phoenix"), CritFail: patch.Of("abyss")}},
	}
	for _, tt := range tests {
		for _, who := range []Viewer{player, {UserID: "u-scribe", Name: "Sam"}, {}} {
			t.Run(tt.name+"/"+who.UserID, func(t *testing.T) {
				s, repo, _ := newTestService()
				start := withSettings(neon, func(s *Settings) { s.PlayersPick = false; s.DiceOffered = []string{"neon"} })
				repo.settings, repo.has = start, true
				if _, err := s.SetSettings(context.Background(), "c1", who, tt.in); status(err) != http.StatusForbidden {
					t.Fatalf("status = %d (%v), want 403", status(err), err)
				}
				if len(repo.saved) != 0 || !reflect.DeepEqual(repo.settings, start) {
					t.Fatalf("settings changed to %+v", repo.settings)
				}
			})
		}
		t.Run(tt.name+"/director", func(t *testing.T) {
			s, repo, _ := newTestService()
			repo.settings, repo.has = withSettings(neon, func(*Settings) {}), true
			if _, err := s.SetSettings(context.Background(), "c1", director, tt.in); err != nil {
				t.Fatalf("a Director was refused: %v", err)
			}
		})
	}
}

// Every way a roll reaches the log carries the roller's dice to everyone it
// is sent to; a roll with no known roller carries the campaign's, and a
// blind roll's stub carries none.
func TestEveryRollCarriesTheRollersDice(t *testing.T) {
	ctx := context.Background()
	mine := DiceLook{Set: "clockwork", Colour: "brass", CritSucc: "storm", CritFail: "grasp", Own: true}
	camp := DiceLook{Set: "neon", Colour: "blue", CritSucc: "storm", CritFail: "grasp"}
	tests := []struct {
		name string
		make func(*service) error
		want DiceLook
	}{
		{"a Table roll", func(s *service) error {
			_, err := s.Table(ctx, "c1", player, TableInput{Formula: "2d10+2"})
			return err
		}, mine},
		{"a Foundry roll by a linked player", func(s *service) error {
			_, _, err := s.Report(ctx, "c1", ReportInput{Origin: OriginFoundry, OriginID: "m1", RollerID: player.UserID, RollerName: "Kai", Formula: "1d20", Dice: []Die{{Sides: 20, Value: 7}}, Total: 7, Mode: ModePublic})
			return err
		}, mine},
		{"a Foundry roll nobody linked", func(s *service) error {
			_, _, err := s.Report(ctx, "c1", ReportInput{Origin: OriginFoundry, OriginID: "m2", RollerName: "Guest", Formula: "1d20", Dice: []Die{{Sides: 20, Value: 7}}, Total: 7, Mode: ModePublic})
			return err
		}, camp},
		{"a crafting roll", func(s *service) error {
			return s.Log(ctx, "c1", LogInput{Source: SourceCrafting, RollerID: player.UserID, RollerName: "Kai", Formula: "2d10", Dice: []Die{{Sides: 10, Value: 3}, {Sides: 10, Value: 4}}, Total: 7})
		}, mine},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, repo, pub := newTestService()
			repo.settings, repo.has = withSettings(neon, func(*Settings) {}), true
			repo.picks = map[string]DicePick{"c1/" + player.UserID: {Set: "clockwork", Colour: "brass"}}
			if err := tt.make(s); err != nil {
				t.Fatal(err)
			}
			if len(pub.got) != 1 || pub.got[0].v.RollerDice == nil || *pub.got[0].v.RollerDice != tt.want {
				t.Fatalf("sent %+v; want dice %+v", pub.got, tt.want)
			}
			page, err := s.List(ctx, "c1", other, "", 10)
			if err != nil || len(page.Rolls) != 1 || page.Rolls[0].RollerDice == nil || *page.Rolls[0].RollerDice != tt.want {
				t.Fatalf("another player's log %+v (%v); want dice %+v", page, err, tt.want)
			}
		})
	}
	t.Run("a blind roll's stub carries none", func(t *testing.T) {
		s, repo, pub := newTestService()
		repo.settings, repo.has = withSettings(neon, func(*Settings) {}), true
		if _, err := s.Table(ctx, "c1", player, TableInput{Formula: "d20", Mode: ModeBlind}); err != nil {
			t.Fatal(err)
		}
		for _, m := range pub.got {
			if m.v.Hidden && m.v.RollerDice != nil {
				t.Fatalf("the stub carries dice %+v", m.v.RollerDice)
			}
		}
	})
}
