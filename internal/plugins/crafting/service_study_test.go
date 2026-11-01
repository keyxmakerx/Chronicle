package crafting

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// newStudyFx is the spoils campaign where the troll's Bloodmoss needs study
// with Reason and its Spring water is a one chance find, with two trolls
// down.
func newStudyFx(t *testing.T) *spoilFx {
	t.Helper()
	fx := newSpoilFx()
	fx.sp.drops["troll"] = DropList{PageID: "troll", Version: 1, Lines: []DropLine{
		{ItemID: "moss", Mode: DropTake, Qty: 2, Study: true, StudyStat: "reason"},
		{ItemID: "water", Mode: DropHarvest, Stat: "reason", Yields: []int{1, 2, 3}, Study: true, StudyStat: "reason", Once: true}}}
	if _, err := fx.s.FightEnded(context.Background(), "c", director, Fight{Name: "Bridge", Creatures: []FightCreature{
		{PageID: "troll", Name: "Troll", Down: 2, Copies: 2}}}); err != nil {
		t.Fatalf("fight: %v", err)
	}
	return fx
}

// trayLine finds a line in a tray by spoil and line position.
func trayLine(t *testing.T, tray []SpoilView, spoil, line int) SpoilLineView {
	t.Helper()
	if spoil >= len(tray) || line >= len(tray[spoil].Lines) {
		t.Fatalf("no line %d of spoil %d in %+v", line, spoil, tray)
	}
	return tray[spoil].Lines[line]
}

func (fx *spoilFx) miraTray(t *testing.T) []SpoilView {
	t.Helper()
	cc, err := fx.s.Character(context.Background(), "c", "mira", player)
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	return cc.Spoils
}

func (fx *spoilFx) study(lid int64, v Viewer, hero string, dice ...int) (*StudyResult, error) {
	fx.dice = dice
	return fx.s.StudySpoil(context.Background(), "c", lid, v, TakeInput{CharacterID: hero})
}

func TestSaveDrops_Study(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		line DropLine
		code int
		want DropLine
	}{
		{"by its label", DropLine{ItemID: "moss", Mode: DropTake, Qty: 1, Study: true, StudyStat: "Reason", Once: true}, 0,
			DropLine{ItemID: "moss", Mode: DropTake, Qty: 1, Study: true, StudyStat: "reason", Once: true}},
		{"adding nothing", DropLine{ItemID: "moss", Mode: DropTake, Qty: 1, Study: true}, 0,
			DropLine{ItemID: "moss", Mode: DropTake, Qty: 1, Study: true}},
		{"one chance without a study", DropLine{ItemID: "moss", Mode: DropTake, Qty: 1, StudyStat: "reason", Once: true}, 0,
			DropLine{ItemID: "moss", Mode: DropTake, Qty: 1}},
		{"a characteristic the system lacks", DropLine{ItemID: "moss", Mode: DropTake, Qty: 1, Study: true, StudyStat: "luck"}, 422, DropLine{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fx := newSpoilFx()
			got, err := fx.s.SaveDrops(ctx, "c", "goblin", director, DropInput{Lines: []DropLine{c.line}})
			if code(err) != c.code {
				t.Fatalf("code %d (%v), want %d", code(err), err, c.code)
			}
			if c.code == 0 && !reflect.DeepEqual(got.Lines[0], c.want) {
				t.Errorf("saved %+v, want %+v", got.Lines[0], c.want)
			}
		})
	}
}

func TestStudy_APlayerReadsSomethingUnidentified(t *testing.T) {
	fx := newStudyFx(t)
	moss, water := trayLine(t, fx.miraTray(t), 0, 0), trayLine(t, fx.miraTray(t), 0, 1)
	// Nothing names the item: no id, name, kinds or harvest test.
	if !moss.Locked || moss.Name != unidentified || moss.ItemID != "" || len(moss.Tags) != 0 || moss.Kinds != "" || moss.Qty != 2 {
		t.Errorf("locked take %+v", moss)
	}
	if !water.Locked || water.Name != unidentified || water.Roll != "" || water.Tiers != nil || water.Qty != 0 {
		t.Errorf("locked harvest %+v", water)
	}
	if s := moss.Study; s == nil || s.Roll != "2d10 + Reason" || s.Once || s.Wait != "" ||
		!reflect.DeepEqual(s.Says, []string{"not yet", "learn what it is", "and which recipes use it"}) || s.Tiers[1] != "12 to 16" {
		t.Errorf("study %+v", moss.Study)
	}
	if s := water.Study; s == nil || !s.Once || s.Says[0] != "lose it" {
		t.Errorf("one chance study %+v", water.Study)
	}
	// The Director reads everything, and the mark.
	tab, err := fx.s.Spoils(context.Background(), "c", director)
	if err != nil {
		t.Fatal(err)
	}
	if l := trayLine(t, tab.Tray, 0, 1); !l.Locked || l.Name != "Spring water" || l.Roll != "2d10 + Reason" || l.Study == nil {
		t.Errorf("the Director's line %+v", l)
	}
}

func TestStudy_NobodyTakesItFirstButTheDirector(t *testing.T) {
	ctx := context.Background()
	fx := newStudyFx(t)
	id := fx.lineFor(t, "Troll", "moss")
	if _, err := fx.s.TakeSpoil(ctx, "c", id, player, TakeInput{CharacterID: "mira"}); code(err) != 422 {
		t.Errorf("a player's take: %v", err)
	}
	if got, err := fx.s.TakeSpoil(ctx, "c", id, director, TakeInput{CharacterID: "tobin"}); err != nil || got.Got != "2 × Bloodmoss" {
		t.Errorf("the Director's give: %+v, %v", got, err)
	}
}

func TestStudySpoil_AMissWaitsForARespite(t *testing.T) {
	fx := newStudyFx(t)
	id := fx.lineFor(t, "Troll", "moss")
	got, err := fx.study(id, player, "mira", 3, 3)
	if err != nil || got.Known || got.Lost || got.Total != 8 || got.Tier != 0 || got.Item != "" || got.Mode != DropTake {
		t.Fatalf("miss: %+v, %v", got, err)
	}
	if got.Terms[1].Text != "Reason, from Mira’s sheet" {
		t.Errorf("terms %+v", got.Terms)
	}
	if l := fx.sp.lines[id]; l.Status != SpoilWaiting {
		t.Errorf("a miss took the line: %+v", l)
	}
	s := trayLine(t, fx.miraTray(t), 0, 0).Study
	if s.Wait != "Mira tries again after the next respite" || !reflect.DeepEqual(s.Tried, []string{"Mira rolled 8"}) {
		t.Errorf("after the miss %+v", s)
	}
	// The other copy is the same item, so it waits too.
	other := fx.lineFor(t, "Troll", "moss")
	for _, lid := range []int64{id, other} {
		if _, err := fx.study(lid, player, "mira", 9, 9); code(err) != 409 || !strings.Contains(err.Error(), "after the next respite") {
			t.Errorf("again before a respite: %v", err)
		}
	}
	// Another hero may try at once, and after a respite Mira may too.
	if _, err := fx.study(id, director, "tobin", 2, 2); err != nil {
		t.Errorf("Tobin: %v", err)
	}
	fx.sp.respite = len(fx.sp.studies)
	if s := trayLine(t, fx.miraTray(t), 0, 0).Study; s.Wait != "" || len(s.Tried) != 0 {
		t.Errorf("after a respite %+v", s)
	}
	if _, err := fx.study(id, player, "mira", 3, 3); err != nil {
		t.Errorf("after a respite: %v", err)
	}
}

func TestStudySpoil_ALandedStudyTellsTheParty(t *testing.T) {
	cases := []struct {
		name   string
		dice   []int
		tier   int
		usedIn []string
	}{
		{"12 to 16", []int{5, 5}, 1, nil},
		{"17 or more", []int{8, 9}, 2, []string{"Healing potion"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fx := newStudyFx(t)
			id := fx.lineFor(t, "Troll", "moss")
			got, err := fx.study(id, player, "mira", c.dice...)
			if err != nil || !got.Known || got.Tier != c.tier || got.Item != "Bloodmoss" || got.Kinds != "herb" || !reflect.DeepEqual(got.UsedIn, c.usedIn) {
				t.Fatalf("study: %+v, %v", got, err)
			}
			// Every copy reads by name now, and anyone may take it.
			for spoil := 0; spoil < 2; spoil++ {
				l := trayLine(t, fx.miraTray(t), spoil, 0)
				if l.Locked || l.Name != "Bloodmoss" || l.Kinds != "herb" || l.Identified != "identified by Mira" || !reflect.DeepEqual(l.UsedIn, c.usedIn) {
					t.Errorf("copy %d after the study %+v", spoil, l)
				}
			}
			if _, err := fx.s.TakeSpoil(context.Background(), "c", id, player, TakeInput{CharacterID: "mira"}); err != nil {
				t.Errorf("take after the study: %v", err)
			}
			if _, err := fx.study(fx.lineFor(t, "Troll", "moss"), director, "tobin", 9, 9); code(err) != 409 {
				t.Errorf("studying a known item: %v", err)
			}
		})
	}
}

func TestStudySpoil_AOneChanceMissLosesIt(t *testing.T) {
	fx := newStudyFx(t)
	id := fx.lineFor(t, "Troll", "water")
	got, err := fx.study(id, player, "mira", 1, 1)
	if err != nil || !got.Lost || got.Known || got.Item != "" {
		t.Fatalf("miss: %+v, %v", got, err)
	}
	if l := fx.sp.lines[id]; l.Status != SpoilLost || l.CharacterID != "mira" || l.Rolled == nil || *l.Rolled != 4 {
		t.Errorf("line %+v", l)
	}
	if _, err := fx.study(id, director, "tobin", 9, 9); code(err) != 404 {
		t.Errorf("studying a lost find: %v", err)
	}
	// The other troll's copy is still there for another hero.
	if len(fx.miraTray(t)[1].Lines) != 2 || len(fx.miraTray(t)[0].Lines) != 1 {
		t.Errorf("tray %+v", fx.miraTray(t))
	}
}

func TestStudySpoil_Refusals(t *testing.T) {
	ctx := context.Background()
	fx := newStudyFx(t)
	id := fx.lineFor(t, "Troll", "moss")
	if _, err := fx.study(id, player, "tobin", 9, 9); code(err) != 404 {
		t.Errorf("another player's hero: %v", err)
	}
	if _, err := fx.study(999, player, "mira", 9, 9); code(err) != 404 {
		t.Errorf("no such line: %v", err)
	}
	if _, err := fx.s.PutSpoils(ctx, "c", director, PutInput{PageID: "shrine", Copies: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.study(fx.lineFor(t, "The Weeping Shrine", "water"), player, "mira", 9, 9); code(err) != 422 {
		t.Errorf("a line that needs no study: %v", err)
	}
	if len(fx.sp.studies) != 0 {
		t.Errorf("a refused study was written: %+v", fx.sp.studies)
	}
}

func TestRevealSpoil(t *testing.T) {
	ctx := context.Background()
	fx := newStudyFx(t)
	id := fx.lineFor(t, "Troll", "moss")
	if err := fx.s.RevealSpoil(ctx, "c", id, player); code(err) != 403 {
		t.Errorf("a player's reveal: %v", err)
	}
	if err := fx.s.RevealSpoil(ctx, "c", id, director); err != nil {
		t.Fatal(err)
	}
	if l := trayLine(t, fx.miraTray(t), 0, 0); l.Locked || l.Name != "Bloodmoss" || l.Identified != "revealed by the Director" || l.UsedIn != nil {
		t.Errorf("after the reveal %+v", l)
	}
	// A second reveal changes nothing.
	if err := fx.s.RevealSpoil(ctx, "c", id, director); err != nil || len(fx.sp.studies) != 1 {
		t.Errorf("again: %v, %d rows", err, len(fx.sp.studies))
	}
}

func TestSpoils_TakenLatelyListsStudies(t *testing.T) {
	ctx := context.Background()
	fx := newStudyFx(t)
	if _, err := fx.study(fx.lineFor(t, "Troll", "moss"), player, "mira", 3, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.study(fx.lineFor(t, "Troll", "water"), director, "tobin", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := fx.s.RevealSpoil(ctx, "c", fx.lineFor(t, "Troll", "moss"), director); err != nil {
		t.Fatal(err)
	}
	tab, err := fx.s.Spoils(ctx, "c", director)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tv := range tab.Taken {
		got = append(got, tv.Text)
	}
	want := []string{"Bloodmoss was revealed to the party", "Tobin studied Spring water (rolled 4): lost, its one chance", "Mira studied Bloodmoss (rolled 8): not yet"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("taken lately\n%s", strings.Join(got, "\n"))
	}
}

func TestStudy_AHeroThePlayerCantSeeIsNotNamed(t *testing.T) {
	fx := newStudyFx(t)
	fx.pages.hidden["tobin"] = true
	if _, err := fx.study(fx.lineFor(t, "Troll", "moss"), director, "tobin", 3, 3); err != nil {
		t.Fatal(err)
	}
	if s := trayLine(t, fx.miraTray(t), 0, 0).Study; !reflect.DeepEqual(s.Tried, []string{"Another hero rolled 8"}) || s.Wait != "" {
		t.Errorf("study %+v", s)
	}
}
