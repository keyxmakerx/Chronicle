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
	heard []AskAnswered
	// during runs inside the owner's answer, while the run is claimed.
	during func()
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
				if or.during != nil {
					or.during()
				}
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

// A rule's ask rings the asker that it waits; an owner's ask rings nothing
// from here, its owner rings its own, while both still go out live.
func TestTheWaitingBellIsSkippedForAnOwnersAsk(t *testing.T) {
	for _, tc := range []struct {
		name      string
		owner     bool
		wantKinds []string
	}{
		{"a rule's ask", false, []string{BellWaiting, BellAsked}},
		{"an owner's ask", true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			or := newOwnerRig(t)
			if tc.owner {
				or.plea(t, "Pact", "copy-1")
			} else {
				or.askRecipe(t)
			}
			var kinds []string
			for _, n := range or.bell.rung {
				kinds = append(kinds, n.Kind)
			}
			if strings.Join(kinds, ",") != strings.Join(tc.wantKinds, ",") {
				t.Errorf("rung %v, want %v", kinds, tc.wantKinds)
			}
			if len(or.live.events) != 1 || or.live.events[0].State != AskWaiting {
				t.Errorf("live = %+v", or.live.events)
			}
		})
	}
}

func TestCloseForEndsAnOwnersAskWithoutAnAnswer(t *testing.T) {
	for _, tc := range []struct {
		name      string
		key       string
		by        Viewer
		answered  bool // answered before the close
		wantFound bool
		wantCode  int
		wantState string
	}{
		{name: "closes the waiting ask", key: "copy-1", by: dm, wantFound: true, wantState: RunWithdrawn},
		{name: "a key nothing waits under", key: "copy-9", by: dm, wantState: RunHeld},
		{name: "a player can't close it", key: "copy-1", by: mira, wantCode: 403, wantState: RunHeld},
		{name: "an answer already given stands", key: "copy-1", by: dm, answered: true, wantState: RunDeclined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			or := newOwnerRig(t)
			or.save(t, "Omens", []Part{counter("doom")},
				Rule{When: When{Event: "plea"}, Timing: TimingNow, Then: []Do{{Action: "add", Part: "doom"}}})
			r := or.plea(t, "Pact", "copy-1")
			if tc.answered {
				if _, err := or.svc.Answer(context.Background(), camp, r.ID, dm, AnswerInput{Answer: AnswerNo}); err != nil {
					t.Fatal(err)
				}
			}
			heard, events := len(or.heard), len(or.live.events)
			found, err := or.svc.CloseFor(context.Background(), camp, "plea", tc.key, tc.by, "Closed: a Director took the card back.")
			if found != tc.wantFound || appCode(err) != tc.wantCode {
				t.Fatalf("found %v, err %v", found, err)
			}
			got := or.runs.runs[r.ID]
			if got.Status != tc.wantState {
				t.Errorf("status %s, want %s", got.Status, tc.wantState)
			}
			if len(or.heard) != heard || or.values["doom"] != 0 || len(or.bell.rung) != 0 {
				t.Errorf("a close told the owner, ran a step or rang: heard %d, doom %d, rung %v", len(or.heard)-heard, or.values["doom"], or.bell.rung)
			}
			if !tc.wantFound {
				if len(or.live.events) != events {
					t.Errorf("nothing closed, yet live = %+v", or.live.events[events:])
				}
				return
			}
			if last := got.Lines[len(got.Lines)-1]; last != "Closed: a Director took the card back." {
				t.Errorf("lines = %v", got.Lines)
			}
			if len(or.live.events) != events+1 || or.live.events[events].State != AskTakenBack || or.live.events[events].ID != r.ID {
				t.Errorf("live = %+v", or.live.events)
			}
			d, err := or.svc.Drawer(context.Background(), camp, dm)
			if err != nil || len(d.Waiting) != 0 {
				t.Errorf("the drawer still shows it: %+v, %v", d, err)
			}
			if _, err := or.svc.Answer(context.Background(), camp, r.ID, dm, AnswerInput{Answer: AnswerYes}); appCode(err) != 409 {
				t.Errorf("a closed ask was answered: %v", err)
			}
			mine, _ := or.svc.MyAsks(context.Background(), camp, mira)
			if len(mine) != 1 || mine[0].State != AskTakenBack || mine[0].CanTakeBack {
				t.Errorf("the asker sees %+v", mine)
			}
		})
	}
}

// While a Yes is going through (the run claimed as running, the owner
// acting), the ask is found, not missing: a second answer or a close is a
// 409, so the owner never acts on it a second way, and the Yes completes.
func TestAnAskBeingAnsweredIsFoundAndRefused(t *testing.T) {
	cases := []struct {
		name string
		call func(or *ownerRig) (bool, error)
	}{
		{"a second yes", func(or *ownerRig) (bool, error) {
			_, found, err := or.svc.AnswerFor(context.Background(), camp, "plea", "copy-1", dm2, AnswerInput{Answer: AnswerYes})
			return found, err
		}},
		{"a no", func(or *ownerRig) (bool, error) {
			_, found, err := or.svc.AnswerFor(context.Background(), camp, "plea", "copy-1", dm2, AnswerInput{Answer: AnswerNo})
			return found, err
		}},
		{"a close", func(or *ownerRig) (bool, error) {
			return or.svc.CloseFor(context.Background(), camp, "plea", "copy-1", dm2, "Closed: a Director took the card back.")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			or := newOwnerRig(t)
			or.save(t, "Omens", []Part{counter("doom")},
				Rule{When: When{Event: "plea"}, Timing: TimingNow, Then: []Do{{Action: "add", Part: "doom"}}})
			r := or.plea(t, "Pact", "copy-1")
			var found bool
			var err error
			or.during = func() {
				or.during = nil
				if got := or.runs.runs[r.ID].Status; got != RunRunning {
					t.Errorf("the owner acts while the run is %s", got)
				}
				found, err = tc.call(or)
			}
			if _, _, aerr := or.svc.AnswerFor(context.Background(), camp, "plea", "copy-1", dm, AnswerInput{Answer: AnswerYes}); aerr != nil {
				t.Fatal(aerr)
			}
			if !found || appCode(err) != 409 || !strings.Contains(err.Error(), "already being answered") {
				t.Errorf("found %v, err %v", found, err)
			}
			if len(or.heard) != 1 || or.runs.runs[r.ID].Status != RunDone || or.values["doom"] != 1 {
				t.Errorf("the first yes must complete alone: heard %d, %s, doom %d", len(or.heard), or.runs.runs[r.ID].Status, or.values["doom"])
			}
		})
	}
}

// The stored ask step holds what a reader who may see nothing reads, so the
// owner's names can't show through; the drawer still asks the owner.
func TestTheStoredAskWordsAreReaderNeutral(t *testing.T) {
	or := newOwnerRig(t)
	or.hidden = true
	r := or.plea(t, "Pact", "copy-1")
	if got := askWords(r.Rule, 0); got != "play a card" {
		t.Errorf("stored words = %q", got)
	}
	asks, err := or.svc.Asks(context.Background(), camp, dm)
	if err != nil || len(asks) != 1 || asks[0].Ask.Ask != "play Pact" {
		t.Errorf("the Director reads %+v, %v", asks, err)
	}
}
