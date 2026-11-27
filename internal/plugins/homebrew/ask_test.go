package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// askPeople, askBell and askLive record what an ask told people.
type askPeople struct{ dirs []string }

func (p askPeople) Name(_ context.Context, _, id string) string {
	return map[string]string{"mira-1": "Mira", "dm-1": "Dana"}[id]
}
func (p askPeople) Directors(context.Context, string) ([]string, error) { return p.dirs, nil }

type askBell struct{ rung []Notice }

func (b *askBell) Ring(_ context.Context, _ string, n Notice) error {
	b.rung = append(b.rung, n)
	return nil
}

type askLive struct{ events []AskEvent }

func (l *askLive) AskChanged(_ string, e AskEvent) { l.events = append(l.events, e) }

var (
	mira = Viewer{UserID: "mira-1"}
	// miraFires is Mira's move as an owner fires it: the Director's
	// standing for the rule, with Mira named as the one who set it off.
	miraFires = Viewer{UserID: "mira-1", Director: true}
	dm2       = Viewer{UserID: "dm-2", Director: true}
)

type askRig struct {
	*runRig
	bell   *askBell
	live   *askLive
	grants []json.RawMessage
}

// newAskRig is the run rig with the ask step, a "grant" action whose Check
// refuses more than 5 copies (as a deck refuses too many copies), and
// people, bell and live to watch.
func newAskRig(t *testing.T) *askRig {
	t.Helper()
	rg := newRunRig(t)
	ar := &askRig{runRig: rg, bell: &askBell{}, live: &askLive{}}
	reg := rg.svc.reg
	if err := reg.RegisterAction(AskDirector()); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterAction(ActionDef{Name: "grant", Label: "Grant copies", Waits: true,
		Fields: []Field{{Name: "deck", Label: "Deck", Type: FieldText}, {Name: "copies", Label: "Copies", Type: FieldWhole}},
		Check: func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
			var a struct {
				Deck   string `json:"deck"`
				Copies int    `json:"copies"`
			}
			if err := json.Unmarshal(raw, &a); err != nil || a.Copies < 1 || a.Copies > 5 {
				return nil, apperror.NewValidation("give 1 to 5 copies")
			}
			return json.Marshal(a)
		},
		Run: func(_ context.Context, _ string, a Act) (string, error) {
			ar.grants = append(ar.grants, a.Args)
			return "Granted.", nil
		}}); err != nil {
		t.Fatal(err)
	}
	rg.svc.people = askPeople{dirs: []string{"dm-1", "dm-2"}}
	rg.svc.bell, rg.svc.live = ar.bell, ar.live
	return ar
}

// askRecipe saves a recipe whose button rule steps a, asks, then grants
// and steps b, and fires it as Mira; it returns the waiting run.
func (ar *askRig) askRecipe(t *testing.T) Run {
	t.Helper()
	ar.save(t, "Lucky draw", []Part{counter("a"), counter("b"), counter("c")},
		Rule{When: When{Event: "rest"}, Timing: TimingNow, Then: []Do{
			{Action: "add", Part: "a"},
			{Action: AskAction, Args: raw(`{"ask":"wants to draw a card"}`)},
			{Action: "grant", Args: raw(`{"deck":"Fortunes","copies":2}`)},
			{Action: "add", Part: "b"},
		}})
	if err := ar.svc.Fire(context.Background(), camp, miraFires, Fired{Event: "rest", Scope: "hero-1"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range ar.runs.order {
		if r := ar.runs.runs[id]; r.Asked > 0 {
			return r
		}
	}
	t.Fatalf("no run asked: %v", ar.statuses())
	return Run{}
}

func appCode(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func TestAskHoldsTheRun(t *testing.T) {
	ar := newAskRig(t)
	r := ar.askRecipe(t)
	if r.Status != RunHeld || r.Asked != 2 || r.CreatedBy != "mira-1" {
		t.Fatalf("run = %+v", r)
	}
	if ar.values["a"] != 1 || ar.values["b"] != 0 || len(ar.grants) != 0 {
		t.Fatalf("the step before the ask runs and those after wait: a=%d b=%d grants=%d", ar.values["a"], ar.values["b"], len(ar.grants))
	}
	if askWords(r.Rule, 1) != "draw a card" {
		t.Errorf("the author's \"wants to\" is dropped: %q", askWords(r.Rule, 1))
	}
	if len(ar.live.events) != 1 || ar.live.events[0] != (AskEvent{ID: r.ID, State: AskWaiting, Asker: "mira-1"}) {
		t.Errorf("live = %+v", ar.live.events)
	}
	if len(ar.bell.rung) != 2 {
		t.Fatalf("bell = %+v", ar.bell.rung)
	}
	if w := ar.bell.rung[0]; w.Kind != BellWaiting || w.Message != "Waiting for your Director: draw a card" ||
		strings.Join(w.Users, ",") != "mira-1" || !strings.HasSuffix(w.Link, "/homebrew/asks") {
		t.Errorf("the asker is told the ask waits: %+v", w)
	}
	if d := ar.bell.rung[1]; d.Kind != BellAsked || d.Message != "Mira wants to draw a card" ||
		strings.Join(d.Users, ",") != "dm-1,dm-2" || !strings.HasSuffix(d.Link, "/dm-screen") {
		t.Errorf("bell = %+v", d)
	}
	d, err := ar.svc.Drawer(context.Background(), camp, dm)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Waiting) != 1 || d.Waiting[0].Ask == nil {
		t.Fatalf("drawer waiting = %+v", d.Waiting)
	}
	ask := d.Waiting[0].Ask
	if ask.Asker != "Mira" || ask.Ask != "draw a card" || ask.Step != 1 || len(ask.Later) != 2 {
		t.Fatalf("ask = %+v", ask)
	}
	if got := ask.Later[0]; got.Action != "grant" || len(got.Fields) != 2 || len(got.Parts) != 0 {
		t.Errorf("grant offers its own fields: %+v", got)
	}
	if got := ask.Later[1]; len(got.Parts) != 3 {
		t.Errorf("a step on a part offers the recipe's parts of its kinds: %+v", got)
	}
	// Run can't skip the question.
	if _, err := ar.svc.RunHeld(context.Background(), camp, r.ID, dm); appCode(err) != 409 {
		t.Errorf("Run on an ask = %v", err)
	}
}

func TestAnswer(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		by      Viewer
		in      AnswerInput
		code    int
		status  string
		b       int    // b after the answer
		grant   string // the grant's args as run, "" for none
		changes string
		bell    string // the asker's bell message, "" for none
	}{
		{"yes runs the later steps", dm, AnswerInput{Answer: AnswerYes}, 0, RunDone, 1, `{"deck":"Fortunes","copies":2}`, "", "Your Director said yes: draw a card"},
		{"yes with changes runs them changed", dm, AnswerInput{Answer: AnswerYes, Steps: []StepEdit{{Step: 2, Args: raw(`{"deck":"Rewards","copies":4}`)}}},
			0, RunDone, 1, `{"deck":"Rewards","copies":4}`, "Deck: Rewards (was Fortunes) · Copies: 4 (was 2)", "Your Director said yes, with changes: draw a card"},
		{"yes with a part changed", dm, AnswerInput{Answer: AnswerYes, Steps: []StepEdit{{Step: 3, Part: ptr("c")}}},
			0, RunDone, 0, `{"deck":"Fortunes","copies":2}`, "Add one: on C c", "Your Director said yes, with changes: draw a card"},
		{"yes with args the owner refuses runs nothing", dm, AnswerInput{Answer: AnswerYes, Steps: []StepEdit{{Step: 2, Args: raw(`{"deck":"Rewards","copies":9}`)}}},
			422, RunHeld, 0, "", "", ""},
		{"yes may not change the steps before the ask", dm, AnswerInput{Answer: AnswerYes, Steps: []StepEdit{{Step: 0, Part: ptr("c")}}},
			422, RunHeld, 0, "", "", ""},
		{"yes may not point a step at a part of the wrong kind", dm, AnswerInput{Answer: AnswerYes, Steps: []StepEdit{{Step: 3, Part: ptr("nope")}}},
			422, RunHeld, 0, "", "", ""},
		{"no stops the run", dm, AnswerInput{Answer: AnswerNo}, 0, RunDeclined, 0, "", "", "Your Director said no: draw a card"},
		{"other stops the run with the reply", dm2, AnswerInput{Answer: AnswerOther, Reply: "Not until the fight ends."}, 0, RunReplied, 0, "", "", "Your Director replied: Not until the fight ends."},
		{"other needs a reply", dm, AnswerInput{Answer: AnswerOther}, 422, RunHeld, 0, "", "", ""},
		{"no can't carry changes", dm, AnswerInput{Answer: AnswerNo, Steps: []StepEdit{{Step: 2}}}, 422, RunHeld, 0, "", "", ""},
		{"a player can't answer", mira, AnswerInput{Answer: AnswerYes}, 403, RunHeld, 0, "", "", ""},
		{"an answer is yes, no or other", dm, AnswerInput{Answer: "maybe"}, 422, RunHeld, 0, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := newAskRig(t)
			r := ar.askRecipe(t)
			ar.bell.rung = nil
			_, err := ar.svc.Answer(ctx, camp, r.ID, tt.by, tt.in)
			if appCode(err) != tt.code && (tt.code != 0 || err != nil) {
				t.Fatalf("err = %v, want code %d", err, tt.code)
			}
			got := ar.runs.runs[r.ID]
			if got.Status != tt.status || ar.values["b"] != tt.b {
				t.Fatalf("status %s b=%d, want %s b=%d (lines %v)", got.Status, ar.values["b"], tt.status, tt.b, got.Lines)
			}
			switch {
			case tt.grant == "" && len(ar.grants) != 0:
				t.Errorf("grant ran: %s", ar.grants)
			case tt.grant != "" && (len(ar.grants) != 1 || !sameJSON(ar.grants[0], raw(tt.grant))):
				t.Errorf("grant ran with %s, want %s", ar.grants, tt.grant)
			}
			if tt.code != 0 {
				if len(ar.bell.rung) != 0 || got.Answer != nil {
					t.Errorf("a refused answer told someone or stored: %+v %+v", ar.bell.rung, got.Answer)
				}
				return
			}
			if got.Answer == nil || strings.Join(got.Answer.Changes, " · ") != tt.changes {
				t.Errorf("answer = %+v, want changes %q", got.Answer, tt.changes)
			}
			if len(ar.bell.rung) != 1 || ar.bell.rung[0].Message != tt.bell || ar.bell.rung[0].Users[0] != "mira-1" ||
				ar.bell.rung[0].Kind != BellAnswered {
				t.Errorf("bell = %+v, want %q to Mira", ar.bell.rung, tt.bell)
			}
			if last := ar.live.events[len(ar.live.events)-1]; last.ID != r.ID || last.State != tt.in.Answer {
				t.Errorf("live = %+v", ar.live.events)
			}
			// A held run is answered once.
			if _, err := ar.svc.Answer(ctx, camp, r.ID, dm2, AnswerInput{Answer: AnswerNo}); appCode(err) != 409 {
				t.Errorf("second answer = %v, want 409", err)
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestAskSetAsideIsNo(t *testing.T) {
	ar := newAskRig(t)
	r := ar.askRecipe(t)
	if _, err := ar.svc.Dismiss(context.Background(), camp, r.ID, dm); err != nil {
		t.Fatal(err)
	}
	if got := ar.runs.runs[r.ID]; got.Status != RunDeclined || got.Answer == nil || got.Answer.Kind != AnswerNo {
		t.Errorf("set aside = %+v", got)
	}
}

func TestTakeBack(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		by   Viewer
		code int
	}{
		{"the asker takes it back", mira, 0},
		{"another player can't", player, 403},
		{"a Director who didn't ask can't", dm, 403},
		{"nobody signed in can't", Viewer{}, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := newAskRig(t)
			r := ar.askRecipe(t)
			out, err := ar.svc.TakeBack(ctx, camp, r.ID, tt.by)
			if tt.code != 0 {
				if appCode(err) != tt.code {
					t.Fatalf("err = %v, want %d", err, tt.code)
				}
				if ar.runs.runs[r.ID].Status != RunHeld {
					t.Error("a refused take-back changed the run")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if out.State != AskTakenBack || ar.runs.runs[r.ID].Status != RunWithdrawn || ar.values["b"] != 0 {
				t.Fatalf("taken back = %+v, run %+v", out, ar.runs.runs[r.ID])
			}
			if _, err := ar.svc.Answer(ctx, camp, r.ID, dm, AnswerInput{Answer: AnswerYes}); appCode(err) != 409 {
				t.Errorf("answering a withdrawn ask = %v, want 409", err)
			}
			if _, err := ar.svc.TakeBack(ctx, camp, r.ID, mira); appCode(err) != 409 {
				t.Errorf("a second take-back = %v, want 409", err)
			}
		})
	}
}

func TestMyAsks(t *testing.T) {
	ctx := context.Background()
	ar := newAskRig(t)
	r := ar.askRecipe(t)
	got, err := ar.svc.MyAsks(ctx, camp, mira)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].State != AskWaiting || !got[0].CanTakeBack || got[0].Ask != "draw a card" {
		t.Fatalf("waiting = %+v", got)
	}
	if others, _ := ar.svc.MyAsks(ctx, camp, player); len(others) != 0 {
		t.Errorf("another player sees Mira's ask: %+v", others)
	}
	if _, err := ar.svc.Answer(ctx, camp, r.ID, dm, AnswerInput{Answer: AnswerYes, Reply: "Draw from the top.",
		Steps: []StepEdit{{Step: 2, Args: raw(`{"deck":"Fortunes","copies":3}`)}}}); err != nil {
		t.Fatal(err)
	}
	got, _ = ar.svc.MyAsks(ctx, camp, mira)
	if len(got) != 1 || got[0].State != AnswerYes || got[0].CanTakeBack || got[0].Reply != "Draw from the top." ||
		strings.Join(got[0].Changes, ",") != "Copies: 3 (was 2)" {
		t.Fatalf("answered = %+v", got)
	}
	b, _ := json.Marshal(got[0])
	for _, secret := range []string{"Lucky draw", "grant", "Fortunes\"", "Granted"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("the asker's view carries %q: %s", secret, b)
		}
	}
	waiting, err := ar.svc.Asks(ctx, camp, dm)
	if err != nil || len(waiting) != 0 {
		t.Errorf("answered ask still waiting: %+v %v", waiting, err)
	}
	if _, err := ar.svc.Asks(ctx, camp, mira); appCode(err) != 403 {
		t.Errorf("a player listed the waiting asks: %v", err)
	}
}

func TestAskRegistration(t *testing.T) {
	tests := []struct {
		name string
		def  ActionDef
		ok   bool
	}{
		{"the ask step", AskDirector(), true},
		{"an ask can't run", ActionDef{Name: "x", Label: "X", Asks: true, Check: okCheck, Run: func(context.Context, string, Act) (string, error) { return "", nil }}, false},
		{"an ask can't wait", ActionDef{Name: "x", Label: "X", Asks: true, Waits: true, Check: okCheck}, false},
		{"an ask takes no part", ActionDef{Name: "x", Label: "X", Asks: true, PartKinds: []string{"counter"}, Check: okCheck}, false},
		{"an ask doesn't run in the browser", ActionDef{Name: "x", Label: "X", Asks: true, ClientRun: true, Check: okCheck}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewRegistry().RegisterAction(tt.def)
			if (err == nil) != tt.ok {
				t.Errorf("err = %v, want ok %v", err, tt.ok)
			}
		})
	}
}

func TestCheckAskArgs(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{`{}`, `{}`, true},
		{`{"ask":"  wants to   draw a card "}`, `{"ask":"draw a card"}`, true},
		{`{"ask":"to rest"}`, `{"ask":"rest"}`, true},
		{`{"ask":"torch the inn"}`, `{"ask":"torch the inn"}`, true},
		{`{"ask":"` + strings.Repeat("a", MaxAskChars+1) + `"}`, "", false},
		{`{"other":1}`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := checkAskArgs(context.Background(), camp, raw(tt.in))
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v", err)
			}
			if tt.ok && string(got) != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

// A rule held for the Director (timed held) that has an ask asks once Run
// reaches it, and the steps before the ask ran then.
func TestHeldRuleReachesAsk(t *testing.T) {
	ctx := context.Background()
	ar := newAskRig(t)
	ar.save(t, "Later", []Part{counter("a"), counter("b")},
		Rule{When: When{Event: "rest"}, Timing: TimingHeld, Then: []Do{{Action: "add", Part: "a"}, {Action: AskAction}, {Action: "add", Part: "b"}}})
	if err := ar.svc.Fire(ctx, camp, dm, Fired{Event: "rest"}); err != nil {
		t.Fatal(err)
	}
	id := ar.runs.order[0]
	if ar.runs.runs[id].Asked != 0 {
		t.Fatal("a held rule asked before Run")
	}
	if _, err := ar.svc.RunHeld(ctx, camp, id, dm); err != nil {
		t.Fatal(err)
	}
	got := ar.runs.runs[id]
	if got.Status != RunHeld || got.Asked != 2 || ar.values["a"] != 1 || ar.values["b"] != 0 {
		t.Fatalf("after Run: %+v a=%d b=%d", got, ar.values["a"], ar.values["b"])
	}
	if _, err := ar.svc.Answer(ctx, camp, id, dm, AnswerInput{Answer: AnswerYes}); err != nil {
		t.Fatal(err)
	}
	if ar.values["b"] != 1 || ar.runs.runs[id].Status != RunDone {
		t.Errorf("yes did not run the rest: %v", ar.runs.runs[id])
	}
	if got := fmt.Sprint(ar.runs.runs[id].Lines); !strings.Contains(got, "Answered yes.") {
		t.Errorf("lines = %s", got)
	}
}
