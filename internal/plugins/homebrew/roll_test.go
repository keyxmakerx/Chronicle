package homebrew

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// scopePlayers is a Players that knows who plays each page.
type scopePlayers map[string]string

func (p scopePlayers) PlayerOf(_ context.Context, _, page string) string { return p[page] }

type rollRig struct {
	*askRig
	total   int
	rollErr error
	rolled  []Act
}

// newRollRig is the ask rig with a "roll" step whose owner rolls whatever
// total the test sets, as a roll log would on the server.
func newRollRig(t *testing.T) *rollRig {
	t.Helper()
	rr := &rollRig{askRig: newAskRig(t)}
	offer := func(_ context.Context, _ string, a Act) (RollOffer, error) {
		var args struct{ Target int }
		_ = json.Unmarshal(a.Args, &args)
		return RollOffer{Label: "Might", Formula: "2d10+2", Target: args.Target}, nil
	}
	err := rr.svc.reg.RegisterAction(ActionDef{Name: "roll", Label: "The player rolls first",
		Fields: []Field{{Name: "target", Label: "Needs at least", Type: FieldWhole}}, Check: okCheck,
		Rolls: &PlayerRoll{Offer: offer, Roll: func(ctx context.Context, c string, a Act) (Rolled, error) {
			if rr.rollErr != nil {
				return Rolled{}, rr.rollErr
			}
			rr.rolled = append(rr.rolled, a)
			o, _ := offer(ctx, c, a)
			return Rolled{RollOffer: o, Total: rr.total, RollID: "roll-1"}, nil
		}}})
	if err != nil {
		t.Fatal(err)
	}
	return rr
}

// rollRecipe saves a rule that steps a, waits on a roll needing 12, then
// steps b, fires it as v from the given page, and returns the held run.
func (rr *rollRig) rollRecipe(t *testing.T, v Viewer, scope string, ask bool) Run {
	t.Helper()
	then := []Do{{Action: "add", Part: "a"}}
	if ask {
		then = append(then, Do{Action: AskAction, Args: raw(`{"ask":"draw a card"}`)})
	}
	then = append(then, Do{Action: "roll", Args: raw(`{"target":12}`)}, Do{Action: "add", Part: "b"})
	rr.save(t, "Lucky draw", []Part{counter("a"), counter("b")}, Rule{When: When{Event: "rest"}, Timing: TimingNow, Then: then})
	if err := rr.svc.Fire(context.Background(), camp, v, Fired{Event: "rest", Scope: scope}); err != nil {
		t.Fatal(err)
	}
	for _, id := range rr.runs.order {
		if r := rr.runs.runs[id]; r.Asked > 0 {
			return r
		}
	}
	t.Fatalf("no run stopped: %v", rr.statuses())
	return Run{}
}

func TestRollFirstHoldsForThePlayer(t *testing.T) {
	ctx := context.Background()
	rr := newRollRig(t)
	r := rr.rollRecipe(t, miraFires, "", false)
	if r.Status != RunHeld || r.Asked != 2 || r.WaitFor != "mira-1" {
		t.Fatalf("run = %+v", r)
	}
	if rr.values["a"] != 1 || rr.values["b"] != 0 {
		t.Fatalf("the step before the roll runs and the one after waits: %v", rr.values)
	}
	if len(rr.live.events) != 1 || rr.live.events[0] != (AskEvent{ID: r.ID, State: AskRoll, Asker: "mira-1"}) {
		t.Errorf("live = %+v", rr.live.events)
	}
	// The player is rung, never the Directors: nobody has to answer.
	if len(rr.bell.rung) != 1 || rr.bell.rung[0].Kind != BellRoll || strings.Join(rr.bell.rung[0].Users, ",") != "mira-1" {
		t.Errorf("bell = %+v", rr.bell.rung)
	}
	d, err := rr.svc.Drawer(ctx, camp, dm)
	if err != nil {
		t.Fatal(err)
	}
	w := d.Waiting[0]
	if w.Kind != KindRoll || w.Ask != nil || w.Roll == nil || w.Roll.Player != "Mira" || w.Roll.Formula != "2d10+2" || w.Roll.Target != 12 {
		t.Fatalf("drawer = %+v (roll %+v)", w, w.Roll)
	}
	if !strings.Contains(w.Waits, "Mira to roll Might (2d10+2), 12 or more") {
		t.Errorf("waits = %q", w.Waits)
	}
	if _, err := rr.svc.RunHeld(ctx, camp, r.ID, dm); appCode(err) != 409 {
		t.Errorf("Run on a waiting roll = %v, want 409", err)
	}
	if _, err := rr.svc.Answer(ctx, camp, r.ID, dm, AnswerInput{Answer: AnswerYes}); appCode(err) != 409 {
		t.Errorf("an answer to a waiting roll = %v, want 409", err)
	}
	mine, err := rr.svc.MyAsks(ctx, camp, mira)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].State != AskRoll || !mine[0].CanRoll || mine[0].Roll == nil || mine[0].Roll.Label != "Might" || mine[0].CanTakeBack {
		t.Fatalf("Mira's asks = %+v", mine)
	}
}

func TestRollFirstJudgesTheTotal(t *testing.T) {
	tests := []struct {
		name   string
		total  int
		status string
		state  string
		b      int
		line   string
	}{
		{"at the target the rule carries on", 12, RunDone, AskRolled, 1, "Mira rolled 12 on Might (2d10+2), needing 12: the rule goes on."},
		{"over the target the rule carries on", 17, RunDone, AskRolled, 1, "Mira rolled 17 on Might (2d10+2), needing 12: the rule goes on."},
		{"under the target the rule stops", 11, RunMissed, AskMissed, 0, "Mira rolled 11 on Might (2d10+2), under 12: the rule stops here."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			rr := newRollRig(t)
			r := rr.rollRecipe(t, miraFires, "hero-1", false)
			rr.total = tt.total
			got, err := rr.svc.Roll(ctx, camp, r.ID, mira)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tt.state || got.Roll == nil || got.Roll.Total == nil || *got.Roll.Total != tt.total || got.Roll.Made != (tt.b == 1) {
				t.Fatalf("view = %+v", got)
			}
			stored := rr.runs.runs[r.ID]
			if stored.Status != tt.status || rr.values["b"] != tt.b {
				t.Fatalf("status = %s, b = %d", stored.Status, rr.values["b"])
			}
			if !strings.Contains(strings.Join(stored.Lines, "\n"), tt.line) {
				t.Errorf("lines = %q", stored.Lines)
			}
			if stored.Answer == nil || stored.Answer.Roll == nil || stored.Answer.Roll.RollID != "roll-1" {
				t.Errorf("answer = %+v", stored.Answer)
			}
			// The owner rolled as Mira, a player, with her name for its log.
			if len(rr.rolled) != 1 || rr.rolled[0].By.UserID != "mira-1" || rr.rolled[0].By.Director || rr.rolled[0].By.Name != "Mira" || rr.rolled[0].Scope != "hero-1" {
				t.Errorf("rolled = %+v", rr.rolled)
			}
			last := rr.live.events[len(rr.live.events)-1]
			if last.State != tt.state || last.Asker != "mira-1" {
				t.Errorf("live = %+v", rr.live.events)
			}
			bell := rr.bell.rung[len(rr.bell.rung)-1]
			if bell.Kind != BellRolled || strings.Join(bell.Users, ",") != "dm-1,dm-2" || bell.Message != tt.line {
				t.Errorf("Directors' bell = %+v", bell)
			}
			if _, err := rr.svc.Roll(ctx, camp, r.ID, mira); appCode(err) != 409 {
				t.Errorf("a second roll = %v, want 409", err)
			}
		})
	}
}

func TestRollFirstWhoMayRoll(t *testing.T) {
	tests := []struct {
		name string
		by   Viewer
		code int
	}{
		{"another player", Viewer{UserID: "tam-1"}, 403},
		{"a Director", dm, 403},
		{"a second Director", dm2, 403},
		{"nobody signed in", Viewer{}, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := newRollRig(t)
			r := rr.rollRecipe(t, miraFires, "", false)
			rr.total = 20
			if _, err := rr.svc.Roll(context.Background(), camp, r.ID, tt.by); appCode(err) != tt.code {
				t.Fatalf("err = %v, want %d", err, tt.code)
			}
			if len(rr.rolled) != 0 || rr.runs.runs[r.ID].Status != RunHeld {
				t.Fatal("someone else's roll was made")
			}
		})
	}
}

func TestRollFirstSetAside(t *testing.T) {
	ctx := context.Background()
	rr := newRollRig(t)
	r := rr.rollRecipe(t, miraFires, "", false)
	if _, err := rr.svc.Dismiss(ctx, camp, r.ID, mira); appCode(err) != 403 {
		t.Fatalf("the player set it aside: %v", err)
	}
	out, err := rr.svc.Dismiss(ctx, camp, r.ID, dm)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != RunDismissed || rr.values["b"] != 0 {
		t.Fatalf("set aside = %+v, b = %d", out, rr.values["b"])
	}
	last := rr.live.events[len(rr.live.events)-1]
	if last.State != AskSetAside || last.Asker != "mira-1" {
		t.Errorf("live = %+v", rr.live.events)
	}
	if bell := rr.bell.rung[len(rr.bell.rung)-1]; bell.Users[0] != "mira-1" || !strings.HasPrefix(bell.Message, "Your Director set aside the roll") {
		t.Errorf("Mira's bell = %+v", bell)
	}
	mine, _ := rr.svc.MyAsks(ctx, camp, mira)
	if len(mine) != 1 || mine[0].State != AskSetAside || mine[0].CanRoll {
		t.Errorf("Mira's asks = %+v", mine)
	}
	if _, err := rr.svc.Roll(ctx, camp, r.ID, mira); appCode(err) != 409 {
		t.Errorf("a roll after set aside = %v, want 409", err)
	}
}

func TestRollFirstWaitsOnTheHerosPlayer(t *testing.T) {
	tests := []struct {
		name  string
		by    Viewer
		scope string
		want  string
	}{
		{"a Director's press on Tam's page waits on Tam", dm, "hero-tam", "tam-1"},
		{"a page nobody plays waits on who set it off", miraFires, "hero-none", "mira-1"},
		{"no page waits on who set it off", miraFires, "", "mira-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := newRollRig(t)
			rr.svc.players = scopePlayers{"hero-tam": "tam-1"}
			r := rr.rollRecipe(t, tt.by, tt.scope, false)
			if r.WaitFor != tt.want {
				t.Fatalf("waits on %q, want %q", r.WaitFor, tt.want)
			}
			mine, _ := rr.svc.MyAsks(context.Background(), camp, Viewer{UserID: tt.want})
			if len(mine) != 1 || !mine[0].CanRoll {
				t.Errorf("the player's asks = %+v", mine)
			}
		})
	}
}

func TestRollFirstAfterAYes(t *testing.T) {
	ctx := context.Background()
	rr := newRollRig(t)
	r := rr.rollRecipe(t, miraFires, "", true)
	// The Director's "Yes, and…" raises the target through the step's Fields.
	if _, err := rr.svc.Answer(ctx, camp, r.ID, dm, AnswerInput{Answer: AnswerYes,
		Steps: []StepEdit{{Step: 2, Args: raw(`{"target":14}`)}}}); err != nil {
		t.Fatal(err)
	}
	held := rr.runs.runs[r.ID]
	if held.Status != RunHeld || held.Asked != 3 || held.WaitFor != "mira-1" || rr.values["b"] != 0 {
		t.Fatalf("run = %+v", held)
	}
	yes := rr.bell.rung[len(rr.bell.rung)-1]
	if yes.Message != "Your Director said yes, if you roll first: draw a card" || len(rr.bell.rung) != 2 {
		t.Errorf("bells = %+v", rr.bell.rung)
	}
	mine, _ := rr.svc.MyAsks(ctx, camp, mira)
	if len(mine) != 1 || mine[0].State != AskRoll || mine[0].Answered != AnswerYes || mine[0].Ask != "draw a card" ||
		mine[0].Roll.Target != 14 || strings.Join(mine[0].Changes, "") != "Needs at least: 14 (was 12)" {
		t.Fatalf("Mira's asks = %+v", mine)
	}
	rr.total = 15
	got, err := rr.svc.Roll(ctx, camp, r.ID, mira)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != AskRolled || got.Answered != AnswerYes || rr.values["b"] != 1 {
		t.Fatalf("view = %+v, b = %d", got, rr.values["b"])
	}
}

func TestRollFirstFailedRollWaitsAgain(t *testing.T) {
	rr := newRollRig(t)
	r := rr.rollRecipe(t, miraFires, "", false)
	rr.rollErr = apperror.NewTooManyRequests("That's a lot of rolls at once.")
	if _, err := rr.svc.Roll(context.Background(), camp, r.ID, mira); appCode(err) != 429 {
		t.Fatalf("err = %v", err)
	}
	if got := rr.runs.runs[r.ID]; got.Status != RunHeld || got.DoneAt != nil {
		t.Fatalf("run = %+v", got)
	}
	rr.rollErr, rr.total = nil, 12
	if _, err := rr.svc.Roll(context.Background(), camp, r.ID, mira); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterPlayerRoll(t *testing.T) {
	roll := &PlayerRoll{Offer: func(context.Context, string, Act) (RollOffer, error) { return RollOffer{}, nil },
		Roll: func(context.Context, string, Act) (Rolled, error) { return Rolled{}, nil }}
	run := func(context.Context, string, Act) (string, error) { return "", nil }
	tests := []struct {
		name string
		def  ActionDef
		ok   bool
	}{
		{"a roll", ActionDef{Rolls: roll}, true},
		{"without Roll", ActionDef{Rolls: &PlayerRoll{Offer: roll.Offer}}, false},
		{"with Run", ActionDef{Rolls: roll, Run: run}, false},
		{"that also asks", ActionDef{Rolls: roll, Asks: true}, false},
		{"on a part", ActionDef{Rolls: roll, PartKinds: []string{"counter"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.def
			d.Name, d.Label, d.Check = "roll-first", "Roll", okCheck
			err := NewRegistry().RegisterAction(d)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v", err)
			}
		})
	}
	reg := NewRegistry()
	if err := reg.RegisterAction(ActionDef{Name: "roll-first", Label: "Roll", Check: okCheck, Rolls: roll}); err != nil {
		t.Fatal(err)
	}
	if c := reg.Catalog(); len(c.Actions) != 1 || !c.Actions[0].Rolls {
		t.Errorf("catalog = %+v", c.Actions)
	}
}
