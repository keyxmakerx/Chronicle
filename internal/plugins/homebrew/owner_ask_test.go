package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ownerRig is the ask rig with an owner's ask, "plea": its owner words it
// ("Tam wants to play Pact", or "play a card" to a reader it hides from) and
// records each answer it hears, failing when told to.
type ownerRig struct {
	*askRig
	heard  []AskAnswered
	fail   error
	hidden bool
}

func newOwnerRig(t *testing.T) *ownerRig {
	t.Helper()
	or := &ownerRig{askRig: newAskRig(t)}
	err := or.svc.reg.RegisterEvent(EventDef{Name: "plea", Label: "A card is about to be played", Check: okCheck,
		Fields: []Field{{Name: "card", Label: "Card", Type: FieldText}},
		Match: func(args, payload json.RawMessage) (bool, error) {
			var a, p struct{ Card string }
			_ = json.Unmarshal(args, &a)
			if err := json.Unmarshal(payload, &p); err != nil {
				return false, err
			}
			return a.Card == "" || a.Card == p.Card, nil
		},
		Ask: &EventAsk{Owner: "Card decks",
			Words: func(_ context.Context, _ string, payload json.RawMessage, reader Viewer) AskWords {
				var p struct{ Card string }
				_ = json.Unmarshal(payload, &p)
				if !reader.Director && or.hidden {
					return AskWords{What: "play a card"}
				}
				return AskWords{Who: "Tam", What: "play " + p.Card}
			},
			Answered: func(_ context.Context, _ string, a AskAnswered) error {
				or.heard = append(or.heard, a)
				return or.fail
			}}})
	if err != nil {
		t.Fatal(err)
	}
	return or
}

// plea puts Mira's play of a card to the Directors and returns the run.
func (or *ownerRig) plea(t *testing.T, card, key string) Run {
	t.Helper()
	id, err := or.svc.AskFor(context.Background(), camp, miraFires, OwnerAsk{Event: "plea", Key: key, Scope: "hero-1",
		Payload: raw(`{"card":"` + card + `"}`)})
	if err != nil {
		t.Fatal(err)
	}
	r, ok := or.runs.runs[id]
	if !ok {
		t.Fatalf("no run %q", id)
	}
	return r
}

func TestOwnerAskHoldsAndGathersEveryRulesSteps(t *testing.T) {
	or := newOwnerRig(t)
	first := or.save(t, "Omens", []Part{counter("doom")},
		Rule{When: When{Event: "plea", Args: raw(`{"card":"Pact"}`)}, Timing: TimingHeld, Then: []Do{{Action: "add", Part: "doom"}}},
		Rule{When: When{Event: "plea", Args: raw(`{"card":"Other"}`)}, Timing: TimingNow, Then: []Do{{Action: "big"}}})
	second := or.save(t, "Debts", []Part{counter("debt")},
		Rule{When: When{Event: "plea"}, Timing: TimingNow, Then: []Do{{Action: "grant", Args: raw(`{"deck":"Omens","copies":2}`)}, {Action: "add", Part: "debt"}}})

	r := or.plea(t, "Pact", "copy-1")
	if r.Status != RunHeld || r.Asked != 1 || r.RecipeID != "" || r.RecipeName != "Card decks" || r.CreatedBy != "mira-1" || r.Scope != "hero-1" {
		t.Fatalf("run = %+v", r)
	}
	var got []string
	for _, d := range r.Rule.Then {
		got = append(got, d.Action+"@"+d.Recipe)
	}
	// Recipes in the store's order (by name), each rule's steps in order.
	want := []string{AskAction + "@", "grant@" + second.ID, "add@" + second.ID, "add@" + first.ID}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("steps = %v, want %v", got, want)
	}
	if or.values["doom"]+or.values["debt"] != 0 || len(or.grants) != 0 || len(or.heard) != 0 {
		t.Errorf("nothing runs and the owner hears nothing before an answer")
	}
	// The owner rings its own bell; the live message still goes.
	if len(or.bell.rung) != 0 || len(or.live.events) != 1 || or.live.events[0].State != AskWaiting {
		t.Errorf("bell %+v, live %+v", or.bell.rung, or.live.events)
	}
	// Asked again under the same key while it waits: the same ask.
	if again := or.plea(t, "Pact", "copy-1"); again.ID != r.ID {
		t.Errorf("a second ask for the same key made %q", again.ID)
	}
	// Fired like any other event, it is refused: it is asked, never fired.
	if err := or.svc.Fire(context.Background(), camp, miraFires, Fired{Event: "plea", Payload: raw(`{"card":"Pact"}`)}); err == nil {
		t.Error("an owner's ask was fired")
	}

	asks, err := or.svc.Asks(context.Background(), camp, dm)
	if err != nil || len(asks) != 1 {
		t.Fatalf("asks = %+v, %v", asks, err)
	}
	a := asks[0].Ask
	if a.Asker != "Tam" || a.Ask != "play Pact" || len(a.Later) != 3 || len(a.Later[1].Parts) != 1 || a.Later[1].Parts[0].Key != "debt" || len(a.Later[2].Parts) != 1 || a.Later[2].Parts[0].Key != "doom" {
		t.Errorf("the drawer's card = %+v", a)
	}
}

func TestOwnerAskAnswers(t *testing.T) {
	cases := []struct {
		name      string
		in        AnswerInput
		by        Viewer
		fail      error
		wantCode  int
		wantState string
		wantHeard string
		wantDoom  int
		wantGrant string
	}{
		{name: "yes lets it through, then the rules run", in: AnswerInput{Answer: AnswerYes}, by: dm,
			wantState: RunDone, wantHeard: AnswerYes, wantDoom: 1, wantGrant: `{"deck":"Omens","copies":2}`},
		{name: "yes and changes a later step", in: AnswerInput{Answer: AnswerYes, Steps: []StepEdit{{Step: 2, Args: raw(`{"deck":"Omens","copies":4}`)}}}, by: dm,
			wantState: RunDone, wantHeard: AnswerYes, wantDoom: 1, wantGrant: `{"deck":"Omens","copies":4}`},
		{name: "no turns it down and runs nothing", in: AnswerInput{Answer: AnswerNo}, by: dm, wantState: RunDeclined, wantHeard: AnswerNo},
		{name: "a reply turns it down with the reply", in: AnswerInput{Answer: AnswerOther, Reply: "After the storm."}, by: dm,
			wantState: RunReplied, wantHeard: AnswerOther},
		{name: "a player can't answer", in: AnswerInput{Answer: AnswerYes}, by: mira, wantCode: 403, wantState: RunHeld},
		{name: "a refused change leaves it waiting", in: AnswerInput{Answer: AnswerYes, Steps: []StepEdit{{Step: 2, Args: raw(`{"deck":"Omens","copies":9}`)}}}, by: dm,
			wantCode: 422, wantState: RunHeld},
		{name: "the owner failing stops the run with its words", in: AnswerInput{Answer: AnswerYes}, by: dm,
			fail: apperror.NewConflict("that card is not waiting for an answer"), wantCode: 409, wantState: RunFailed, wantHeard: AnswerYes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			or := newOwnerRig(t)
			or.fail = tc.fail
			or.save(t, "Omens", []Part{counter("doom")},
				Rule{When: When{Event: "plea"}, Timing: TimingNow, Then: []Do{{Action: "add", Part: "doom"}, {Action: "grant", Args: raw(`{"deck":"Omens","copies":2}`)}}})
			r := or.plea(t, "Pact", "copy-1")

			_, found, err := or.svc.AnswerFor(context.Background(), camp, "plea", "copy-1", tc.by, tc.in)
			if tc.by.Director && !found {
				t.Fatal("the ask was not found by its key")
			}
			if appCode(err) != tc.wantCode {
				t.Fatalf("answer: %v, want code %d", err, tc.wantCode)
			}
			got := or.runs.runs[r.ID]
			if got.Status != tc.wantState {
				t.Errorf("status = %s, want %s (%v)", got.Status, tc.wantState, got.Lines)
			}
			heard := ""
			if len(or.heard) == 1 {
				heard = or.heard[0].Kind
				if or.heard[0].Asker != "mira-1" || or.heard[0].By.UserID != tc.by.UserID || or.heard[0].Reply != tc.in.Reply || !strings.Contains(string(or.heard[0].Payload), `"askKey":"copy-1"`) {
					t.Errorf("heard = %+v", or.heard[0])
				}
			} else if len(or.heard) > 1 {
				t.Fatalf("the owner heard %d answers", len(or.heard))
			}
			if heard != tc.wantHeard {
				t.Errorf("owner heard %q, want %q", heard, tc.wantHeard)
			}
			if or.values["doom"] != tc.wantDoom {
				t.Errorf("doom = %d, want %d", or.values["doom"], tc.wantDoom)
			}
			grant := ""
			if len(or.grants) == 1 {
				grant = string(or.grants[0])
			}
			if grant != tc.wantGrant {
				t.Errorf("grant = %s, want %s", grant, tc.wantGrant)
			}
			if len(or.bell.rung) != 0 {
				t.Errorf("an owner's ask rang the bell itself: %+v", or.bell.rung)
			}
			// However it went, a second answer finds nothing to answer.
			if got.Status != RunHeld {
				if _, found, _ := or.svc.AnswerFor(context.Background(), camp, "plea", "copy-1", dm, AnswerInput{Answer: AnswerYes}); found {
					t.Error("an answered ask was found again")
				}
				if _, err := or.svc.Answer(context.Background(), camp, r.ID, dm2, AnswerInput{Answer: AnswerNo}); appCode(err) != 409 {
					t.Errorf("a second answer: %v", err)
				}
				if len(or.heard) > 1 {
					t.Errorf("the owner heard a second answer")
				}
			}
		})
	}
}

func TestOwnerAskAsTheAskerSeesIt(t *testing.T) {
	or := newOwnerRig(t)
	r := or.plea(t, "Pact", "copy-1")
	check := func(step, wantAsk string, canTakeBack bool) {
		t.Helper()
		mine, err := or.svc.MyAsks(context.Background(), camp, mira)
		if err != nil || len(mine) != 1 {
			t.Fatalf("%s: mine = %+v, %v", step, mine, err)
		}
		if mine[0].Ask != wantAsk || mine[0].CanTakeBack != canTakeBack {
			t.Errorf("%s: %+v", step, mine[0])
		}
	}
	check("while the deck shows it", "play Pact", false)
	or.hidden = true
	check("once the deck hides it", "play a card", false)
	if _, err := or.svc.TakeBack(context.Background(), camp, r.ID, mira); appCode(err) != 409 {
		t.Errorf("an owner's ask was taken back: %v", err)
	}
	if _, _, err := or.svc.AnswerFor(context.Background(), camp, "plea", "copy-1", dm, AnswerInput{Answer: AnswerOther, Reply: "Later."}); err != nil {
		t.Fatal(err)
	}
	mine, _ := or.svc.MyAsks(context.Background(), camp, mira)
	if mine[0].State != AnswerOther || mine[0].Reply != "Later." || mine[0].Ask != "play a card" {
		t.Errorf("answered: %+v", mine[0])
	}
}

func TestOwnerAskKeysAndRegistration(t *testing.T) {
	or := newOwnerRig(t)
	if _, found, err := or.svc.AnswerFor(context.Background(), camp, "plea", "copy-from-before", dm, AnswerInput{Answer: AnswerYes}); found || err != nil {
		t.Errorf("an unknown key: found %v, %v", found, err)
	}
	for _, tc := range []struct {
		name string
		a    OwnerAsk
	}{
		{"no key", OwnerAsk{Event: "plea", Payload: raw(`{}`)}},
		{"not an ask", OwnerAsk{Event: "rest", Key: "k", Payload: raw(`{}`)}},
		{"a payload that is not an object", OwnerAsk{Event: "plea", Key: "k", Payload: raw(`[1]`)}},
	} {
		if _, err := or.svc.AskFor(context.Background(), camp, miraFires, tc.a); err == nil {
			t.Errorf("%s: asked", tc.name)
		}
	}
	if _, err := or.svc.AskFor(context.Background(), camp, mira, OwnerAsk{Event: "plea", Key: "k"}); appCode(err) != 403 {
		t.Errorf("asked without a Director's standing: %v", err)
	}

	words := func(context.Context, string, json.RawMessage, Viewer) AskWords { return AskWords{} }
	heard := func(context.Context, string, AskAnswered) error { return errors.New("x") }
	for _, tc := range []struct {
		name string
		e    EventDef
	}{
		{"no words", EventDef{Name: "a1", Label: "A", Check: okCheck, Ask: &EventAsk{Answered: heard}}},
		{"no answered", EventDef{Name: "a2", Label: "A", Check: okCheck, Ask: &EventAsk{Words: words}}},
		{"from a part", EventDef{Name: "a3", Label: "A", Check: okCheck, PartKinds: []string{"counter"}, Ask: &EventAsk{Words: words, Answered: heard}}},
		{"a button", EventDef{Name: "a4", Label: "A", Check: okCheck, Press: "Go", Ask: &EventAsk{Words: words, Answered: heard}}},
	} {
		if err := or.svc.reg.RegisterEvent(tc.e); err == nil {
			t.Errorf("%s: registered", tc.name)
		}
	}
}
