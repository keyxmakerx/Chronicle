package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// What happened to a run.
const (
	RunDone      = "done"
	RunHeld      = "held" // waits in the Homebrew drawer for the Director
	RunFailed    = "failed"
	RunDismissed = "dismissed"
	RunStopped   = "stopped"   // rules kept setting each other off
	RunRunning   = "running"   // claimed by a Director pressing Run or answering
	RunDeclined  = "declined"  // the Director said no to an ask
	RunReplied   = "replied"   // the Director wrote back instead of answering yes
	RunWithdrawn = "withdrawn" // the asker took the ask back
	RunMissed    = "missed"    // the player's roll fell short of the target
)

const (
	// maxRunSteps bounds one chain of rules setting each other off, as the
	// Workshop's preview does.
	maxRunSteps = 50
	// keepRuns is how many finished runs a campaign keeps.
	keepRuns = 200
	// recentRuns and maxWaiting are what the drawer shows.
	recentRuns = 20
	maxWaiting = 50
	// maxHeldRuns is how many runs may wait in one campaign. A held run stays
	// until a Director answers it, so without a bound anything that can set a
	// held rule off again and again would fill the table.
	maxHeldRuns = 200
)

// Fired is an event an owner says happened.
type Fired struct {
	Event string
	// RecipeID limits the firing to one recipe's rules, for an event a
	// recipe's own part fires; "" reaches every recipe (a respite).
	RecipeID string
	// Part is the part key it came from; "" for an event from no part.
	Part string
	// Scope is the page it happened on; "" for none.
	Scope string
	// Payload is what happened, for the event's Match.
	Payload json.RawMessage
	// Once is for an event a player can set off as often as they like: a rule
	// it would hold is not held again while the same rule already waits for
	// the same scope and payload. The Director answers it once.
	Once bool
}

// Run is one rule set off by one firing, with what it did.
type Run struct {
	ID         string
	CampaignID string
	RecipeID   string
	RecipeName string
	What       string
	// Rule is the rule as it was when its event fired.
	Rule      Rule
	Scope     string
	Payload   json.RawMessage
	Status    string
	Lines     []string
	CreatedBy string
	CreatedAt time.Time
	DoneBy    string
	DoneAt    *time.Time
	// Asked is the "Ask the Director" step the run waits at, or last waited
	// at, counted from 1; 0 for a run that never asked. A held run with it
	// set is waiting for an answer rather than for Run.
	Asked int
	// Answer is the Director's answer to that ask, once given, and the
	// player's roll at a "rolls first" step, once made.
	Answer *Answer
	// WaitFor is who a held run waits on when that is a player (their roll);
	// "" while it waits on a Director.
	WaitFor string
}

// RunStore keeps runs.
type RunStore interface {
	CreateRun(ctx context.Context, r Run) error
	GetRun(ctx context.Context, campaignID, id string) (Run, bool, error)
	// FinishRun writes a run's new status, lines and who finished it, only
	// while its stored status is still from, and reports whether it did.
	FinishRun(ctx context.Context, r Run, from string) (bool, error)
	// ListRuns lists held runs, or finished ones, newest first.
	ListRuns(ctx context.Context, campaignID string, held bool, limit int) ([]Run, error)
	// ListAsks lists the runs one person set off, or that wait or waited on
	// their roll, that stopped for an ask or a roll, newest first.
	ListAsks(ctx context.Context, campaignID, userID string, limit int) ([]Run, error)
	// PruneRuns deletes all but the newest keep finished runs.
	PruneRuns(ctx context.Context, campaignID string, keep int) error
}

// RunView is a run as the drawer shows it.
type RunView struct {
	ID     string    `json:"id"`
	Recipe string    `json:"recipe"`
	What   string    `json:"what"`
	Status string    `json:"status"`
	Lines  []string  `json:"lines"`
	At     time.Time `json:"at"`
	// Waits says why a held run is waiting.
	Waits string `json:"waits,omitempty"`
	// Steps are a held run's actions, so the drawer can say what Run will
	// do and run the browser ones itself.
	Steps []RunStep `json:"steps,omitempty"`
	// RecipeID and Scope tell a held run's browser steps which recipe they
	// belong to and the page the event happened on, so an owner's browser
	// action can read its part and act on that page.
	RecipeID string `json:"recipeId,omitempty"`
	Scope    string `json:"scope,omitempty"`
	// Ask is set on a run that asked the Director: who asked, for what, the
	// steps an answer may change, and the answer once given. It is not set
	// while the run waits on a player's roll, which no Director answers.
	Ask *AskDetail `json:"ask,omitempty"`
	// Kind says what a held run that stopped at a step waits for: "ask" (a
	// Director's answer) or "roll" (a player's roll).
	Kind string `json:"kind,omitempty"`
	// Roll is who a run waiting on a roll waits for and what they roll.
	Roll *RollWait `json:"roll,omitempty"`
}

// RunStep is one action of a held run.
type RunStep struct {
	Action  string          `json:"action"`
	Label   string          `json:"label"`
	Part    string          `json:"part,omitempty"`
	Args    json.RawMessage `json:"args,omitempty"`
	Browser bool            `json:"browser,omitempty"`
	// ByHand is an action no one can run for the Director.
	ByHand bool `json:"byHand,omitempty"`
	// Rolls is a step the player rolls for on their own screen.
	Rolls bool `json:"rolls,omitempty"`
	// Asks is an "Ask the Director" step, answered rather than run.
	Asks bool `json:"asks,omitempty"`
}

// Press is an event the Director sets off from the drawer.
type Press struct {
	Event string `json:"event"`
	Label string `json:"label"`
	// Button is a named button's name, sent back with the press; Arg is the
	// event's field that holds it. Both are empty for a registered press.
	Button string `json:"button,omitempty"`
	Arg    string `json:"arg,omitempty"`
	// Rules counts the saved rules it would start.
	Rules int `json:"rules"`
}

// Drawer is the Homebrew drawer's rule sections.
type Drawer struct {
	Presses []Press   `json:"presses"`
	Waiting []RunView `json:"waiting"`
	Recent  []RunView `json:"recent"`
}

// budget is the steps left in one chain, carried in the context so a rule
// whose action changes a part that sets off another rule shares it.
type budget struct {
	left    int
	stopped bool
}

type budgetKey struct{}

func withBudget(ctx context.Context) (context.Context, *budget) {
	if b, ok := ctx.Value(budgetKey{}).(*budget); ok {
		return ctx, b
	}
	b := &budget{left: maxRunSteps}
	return context.WithValue(ctx, budgetKey{}, b), b
}

// Fire starts every saved rule whose When the firing meets. A rule timed
// for now runs at once when the server can do all it says; any other waits
// in the drawer. Its caller has checked the change was the Director's.
func (s *service) Fire(ctx context.Context, campaignID string, v Viewer, f Fired) error {
	if err := requireDirector(v); err != nil {
		return err
	}
	if s.runs == nil {
		return nil
	}
	def, ok := s.reg.event(f.Event)
	if !ok {
		return fmt.Errorf("homebrew: fired an unknown event %q", f.Event)
	}
	recipes, err := s.recipesFor(ctx, campaignID, f.RecipeID)
	if err != nil {
		return err
	}
	ctx, b := withBudget(ctx)
	wrote := false
	var waiting *[]Run
	for _, r := range recipes {
		for _, rule := range r.Doc.Rules {
			if rule.When.Event != f.Event || rule.When.Part != f.Part {
				continue
			}
			if def.Match != nil {
				ok, err := def.Match(rule.When.Args, f.Payload)
				if err != nil {
					return fmt.Errorf("homebrew: match %s: %w", f.Event, err)
				}
				if !ok {
					continue
				}
			}
			run := Run{ID: s.newID(), CampaignID: campaignID, RecipeID: r.ID, RecipeName: r.Name,
				What: clip(s.what(r.Doc, rule), maxWhatChars), Rule: rule, Scope: f.Scope, Payload: f.Payload, CreatedBy: v.UserID, CreatedAt: time.Now().UTC()}
			if b.left <= 0 {
				if b.stopped {
					return nil
				}
				b.stopped = true
				run.Status = RunStopped
				run.Lines = []string{fmt.Sprintf("Stopped after %d rules: these rules keep setting each other off.", maxRunSteps)}
				return s.saveRun(ctx, run)
			}
			b.left--
			if s.holds(rule) || s.stopAt(rule, 0) >= 0 {
				if waiting == nil {
					held, err := s.runs.ListRuns(ctx, campaignID, true, maxHeldRuns)
					if err != nil {
						return fmt.Errorf("homebrew: list held runs: %w", err)
					}
					waiting = &held
				}
				if len(*waiting) >= maxHeldRuns {
					slog.Warn("homebrew: a rule was not held because too many already wait",
						slog.String("campaign_id", campaignID), slog.String("event", f.Event))
					continue
				}
				if f.Once && alreadyWaits(*waiting, run) {
					continue
				}
				*waiting = append(*waiting, run)
			}
			if s.holds(rule) {
				run.Status = RunHeld
			} else {
				var at int
				run.Status, run.Lines, at = s.execute(ctx, campaignID, r, rule, 0, f.Scope, f.Payload, v)
				if run.Status == RunHeld {
					run.Asked = at + 1
					s.setWaiter(ctx, &run)
				} else {
					now := run.CreatedAt
					run.DoneBy, run.DoneAt = v.UserID, &now
				}
			}
			if err := s.saveRun(ctx, run); err != nil {
				return err
			}
			if run.Status == RunHeld && run.Asked > 0 {
				s.announce(ctx, run, true)
			}
			wrote = true
		}
	}
	if wrote {
		if err := s.runs.PruneRuns(ctx, campaignID, keepRuns); err != nil {
			return fmt.Errorf("homebrew: prune runs: %w", err)
		}
	}
	return nil
}

// alreadyWaits says whether a held run of the same rule, for the same scope
// and payload, is among those waiting.
func alreadyWaits(waiting []Run, run Run) bool {
	rule, err := json.Marshal(run.Rule)
	if err != nil {
		return false
	}
	for _, w := range waiting {
		if w.RecipeID != run.RecipeID || w.Scope != run.Scope || !sameJSON(w.Payload, run.Payload) {
			continue
		}
		if got, err := json.Marshal(w.Rule); err == nil && sameJSON(got, rule) {
			return true
		}
	}
	return false
}

// sameJSON compares two JSON values by what they say, not how they are
// written, since a stored value may come back spaced differently.
func sameJSON(a, b json.RawMessage) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func (s *service) saveRun(ctx context.Context, run Run) error {
	if err := s.runs.CreateRun(ctx, run); err != nil {
		return fmt.Errorf("homebrew: save run: %w", err)
	}
	return nil
}

func (s *service) recipesFor(ctx context.Context, campaignID, recipeID string) ([]Recipe, error) {
	if recipeID == "" {
		rs, err := s.repo.List(ctx, campaignID)
		if err != nil {
			return nil, fmt.Errorf("homebrew: list recipes: %w", err)
		}
		return rs, nil
	}
	if !validToken(recipeID) {
		return nil, nil
	}
	r, ok, err := s.repo.Get(ctx, campaignID, recipeID)
	if err != nil {
		return nil, fmt.Errorf("homebrew: get recipe: %w", err)
	}
	if !ok {
		return nil, nil
	}
	return []Recipe{r}, nil
}

// holds says whether a rule waits for the Director: it is timed for later,
// carries a test only a person can judge, does something the server can't, or
// uses an action that is the Director's call even though the server can run it.
//
// Only the steps before the first ask count: the ask itself stops the run
// for the Director, so what comes after it is the Director's to allow.
func (s *service) holds(rule Rule) bool {
	if rule.Timing != TimingNow || rule.When.Test != "" {
		return true
	}
	for _, d := range rule.Then {
		a, ok := s.reg.action(d.Action)
		if ok && stops(a) {
			return false
		}
		if !ok || a.ClientRun || a.Run == nil || a.Waits {
			return true
		}
	}
	return false
}

// stops says whether an action stops a run to wait on a person: a
// Director's answer or a player's roll.
func stops(a ActionDef) bool { return a.Asks || a.Rolls != nil }

// stopAt is the first step at or after from that stops a run, or -1.
func (s *service) stopAt(rule Rule, from int) int {
	for i := from; i < len(rule.Then); i++ {
		if a, ok := s.reg.action(rule.Then[i].Action); ok && stops(a) {
			return i
		}
	}
	return -1
}

// waitsOnPurpose says whether a rule has an action that is the Director's
// call, so the drawer can say that is why it waits.
func (s *service) waitsOnPurpose(rule Rule) bool {
	for _, d := range rule.Then {
		if a, ok := s.reg.action(d.Action); ok && a.Waits {
			return true
		}
	}
	return false
}

// execute runs a rule's actions in order from step from and stops at the
// first that fails. Actions the server can't run are listed for the
// Director. payload is what set the rule off, kept with a held run so its
// note is the same later. A run that reaches an "Ask the Director" step or a
// player's roll stops there, held, and the step's index comes back as at
// (-1 otherwise).
func (s *service) execute(ctx context.Context, campaignID string, r Recipe, rule Rule, from int, scope string, payload json.RawMessage, v Viewer) (status string, lines []string, at int) {
	lines = []string{}
	note := s.note(r, rule, payload)
	for i := from; i < len(rule.Then); i++ {
		d := rule.Then[i]
		a, ok := s.reg.action(d.Action)
		if !ok {
			lines = append(lines, "An action this server no longer has was skipped.")
			continue
		}
		if stops(a) {
			return RunHeld, lines, i
		}
		if a.ClientRun || a.Run == nil {
			lines = append(lines, a.Label+": "+map[bool]string{true: "run in your browser", false: "for you to do"}[a.ClientRun]+".")
			continue
		}
		act := Act{RecipeID: r.ID, Args: d.Args, Scope: scope, By: v, Note: note}
		if d.Part != "" {
			p, ok := partByKey(r.Doc, d.Part)
			if !ok {
				lines = append(lines, a.Label+" did not run: its part is no longer in the recipe.")
				return RunFailed, lines, -1
			}
			act.Part = &p
		}
		line, err := a.Run(ctx, campaignID, act)
		if err != nil {
			lines = append(lines, a.Label+" did not run: "+plainError(err))
			return RunFailed, lines, -1
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return RunDone, lines, -1
}

// note names the rule in the line an owner writes to its own log. A rule
// set off by a named button says the button ("Dose, from Remnant need"), so
// the log reads as the Director's act rather than a rule acting alone.
func (s *service) note(r Recipe, rule Rule, payload json.RawMessage) string {
	if e, ok := s.reg.event(rule.When.Event); ok && e.PressArg != "" {
		if name := buttonName(payload, e.PressArg); name != "" {
			return name + ", from " + r.Name
		}
	}
	return "Rule from " + r.Name
}

// buttonName reads a named button's name from a rule's args or a press's
// payload, single-spaced; "" when it is missing, not text or too long to
// be a button.
func buttonName(raw json.RawMessage, arg string) string {
	var m map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return ""
	}
	s, _ := m[arg].(string)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > MaxNameChars {
		return ""
	}
	return s
}

// namedButtons lists the buttons the saved rules name for an event with a
// PressArg: each distinct name, compared without case, with the first
// spelling met and how many rules use it, in the order first met.
func namedButtons(recipes []Recipe, e EventDef) []Press {
	out := []Press{}
	at := map[string]int{}
	for _, r := range recipes {
		for _, rule := range r.Doc.Rules {
			if rule.When.Event != e.Name {
				continue
			}
			name := buttonName(rule.When.Args, e.PressArg)
			if name == "" {
				continue
			}
			k := strings.ToLower(name)
			if i, ok := at[k]; ok {
				out[i].Rules++
				continue
			}
			at[k] = len(out)
			out = append(out, Press{Event: e.Name, Label: name, Button: name, Arg: e.PressArg, Rules: 1})
		}
	}
	return out
}

// plainError is an error as the Director may read it: an owner's own
// message, never a raw internal one.
func plainError(err error) string {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code < 500 {
		return ae.Message
	}
	return "something went wrong."
}

func partByKey(doc Doc, key string) (Part, bool) {
	for _, p := range doc.Parts {
		if p.Key == key {
			return p, true
		}
	}
	return Part{}, false
}

// maxWhatChars is the stored "what" line's length.
const maxWhatChars = 200

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// what says in a line what set a rule off: the event, its part, its args.
func (s *service) what(doc Doc, rule Rule) string {
	label := rule.When.Event
	var fields []Field
	if e, ok := s.reg.event(rule.When.Event); ok {
		label, fields = e.Label, e.Fields
	}
	if p, ok := partByKey(doc, rule.When.Part); ok && rule.When.Part != "" {
		label += ": " + p.Label
	}
	var args map[string]any
	_ = json.Unmarshal(rule.When.Args, &args)
	for _, f := range fields {
		if v, ok := args[f.Name]; ok {
			label += fmt.Sprintf(", %s %v", strings.ToLower(f.Label), v)
		}
	}
	return label
}

func (s *service) view(r Run) RunView {
	out := RunView{ID: r.ID, Recipe: r.RecipeName, What: r.What, Status: r.Status, Lines: r.Lines, At: r.CreatedAt}
	if out.Lines == nil {
		out.Lines = []string{}
	}
	if r.DoneAt != nil {
		out.At = *r.DoneAt
	}
	rolling := s.waitsOnRoll(r)
	if r.Asked > 0 && !rolling {
		out.Ask = s.askDetail(r)
	}
	if r.Status != RunHeld {
		return out
	}
	out.RecipeID, out.Scope = r.RecipeID, r.Scope
	switch {
	case rolling:
		out.Kind, out.Roll = KindRoll, &RollWait{PlayerID: r.WaitFor}
		out.Waits = "Waits for the player's roll; only they can make it."
	case r.Asked > 0:
		out.Kind = KindAsk
		out.Waits = "Asks you before it goes on."
	case r.Rule.When.Test != "":
		out.Waits = "Only if: " + r.Rule.When.Test
	case r.Rule.Timing == TimingEncounterEnd:
		out.Waits = "Set to wait for the end of the encounter."
	case r.Rule.Timing == TimingHeld:
		out.Waits = "Set to wait for you."
	case s.waitsOnPurpose(r.Rule):
		out.Waits = "Part of it is your call, so it waits for you."
	default:
		out.Waits = "Part of it is yours to run."
	}
	for i, d := range r.Rule.Then {
		// A run that stopped part way has done the steps before the stop
		// (an answered ask among them), so only what is left is listed.
		if r.Asked > 0 && i < r.Asked-1 {
			continue
		}
		st := RunStep{Action: d.Action, Label: d.Action, Part: d.Part, Args: d.Args}
		if a, ok := s.reg.action(d.Action); ok {
			st.Label, st.Browser, st.ByHand, st.Rolls, st.Asks = a.Label, a.ClientRun, !a.ClientRun && a.Run == nil && !stops(a), a.Rolls != nil, a.Asks
		}
		out.Steps = append(out.Steps, st)
	}
	return out
}

// Drawer lists what the drawer shows about rules.
func (s *service) Drawer(ctx context.Context, campaignID string, v Viewer) (*Drawer, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	out := &Drawer{Presses: []Press{}, Waiting: []RunView{}, Recent: []RunView{}}
	recipes, err := s.repo.List(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	for _, e := range s.reg.presses() {
		p := Press{Event: e.Name, Label: e.Press}
		for _, r := range recipes {
			for _, rule := range r.Doc.Rules {
				if rule.When.Event == e.Name {
					p.Rules++
				}
			}
		}
		out.Presses = append(out.Presses, p)
	}
	for _, e := range s.reg.named() {
		out.Presses = append(out.Presses, namedButtons(recipes, e)...)
	}
	if s.runs == nil {
		return out, nil
	}
	for _, held := range []bool{true, false} {
		limit := recentRuns
		if held {
			limit = maxWaiting
		}
		rs, err := s.runs.ListRuns(ctx, campaignID, held, limit)
		if err != nil {
			return nil, errInternal(err)
		}
		for _, r := range rs {
			rv := s.view(r)
			s.fillAsk(ctx, campaignID, &rv, recipes)
			s.fillRoll(ctx, campaignID, &rv, r)
			if held {
				out.Waiting = append(out.Waiting, rv)
			} else {
				out.Recent = append(out.Recent, rv)
			}
		}
	}
	return out, nil
}

// Press sets off an event the Director starts from the drawer. button is ""
// for a registered press and a named button's name otherwise; a name no
// saved rule uses is not a button, so a stray request can't fire any text.
func (s *service) Press(ctx context.Context, campaignID, event, button string, v Viewer) (*Drawer, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	e, ok := s.reg.event(event)
	if !ok {
		return nil, errNotFound("button")
	}
	f := Fired{Event: e.Name}
	switch {
	case e.Press != "" && button == "":
		if e.Pressed != nil {
			if err := e.Pressed(ctx, campaignID, v); err != nil {
				return nil, errInternal(err)
			}
		}
	case e.PressArg != "" && button != "":
		recipes, err := s.repo.List(ctx, campaignID)
		if err != nil {
			return nil, errInternal(err)
		}
		want := strings.ToLower(strings.Join(strings.Fields(button), " "))
		var found *Press
		for _, p := range namedButtons(recipes, e) {
			if strings.ToLower(p.Button) == want {
				found = &p
				break
			}
		}
		if found == nil {
			return nil, errNotFound("button")
		}
		// The first spelling goes out, so the note and the run say the
		// button as the drawer shows it.
		if f.Payload, err = json.Marshal(map[string]string{e.PressArg: found.Button}); err != nil {
			return nil, errInternal(err)
		}
	default:
		return nil, errNotFound("button")
	}
	if err := s.Fire(ctx, campaignID, v, f); err != nil {
		return nil, errInternal(err)
	}
	return s.Drawer(ctx, campaignID, v)
}

// heldRun loads a run the Director is acting on.
func (s *service) heldRun(ctx context.Context, campaignID, id string, v Viewer) (Run, error) {
	if err := requireDirector(v); err != nil {
		return Run{}, err
	}
	if s.runs == nil || !validToken(id) {
		return Run{}, errNotFound("rule")
	}
	r, ok, err := s.runs.GetRun(ctx, campaignID, id)
	if err != nil {
		return Run{}, errInternal(err)
	}
	if !ok {
		return Run{}, errNotFound("rule")
	}
	if r.Status != RunHeld {
		return Run{}, errConflictMsg("this rule was already run or set aside")
	}
	return r, nil
}

// finish moves a run on from status from, refusing if someone else got
// there first.
func (s *service) finish(ctx context.Context, r Run, from, status string, lines []string, v Viewer) (*RunView, error) {
	now := time.Now().UTC()
	r.Status, r.Lines, r.DoneBy, r.DoneAt = status, lines, v.UserID, &now
	ok, err := s.runs.FinishRun(ctx, r, from)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflictMsg("this rule was already run or set aside")
	}
	out := s.view(r)
	return &out, nil
}

// RunHeld runs a waiting rule now, as it was when its event fired. A rule
// that is asking is answered instead, so Run can't skip the question.
func (s *service) RunHeld(ctx context.Context, campaignID, id string, v Viewer) (*RunView, error) {
	r, err := s.heldRun(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	if r.Asked > 0 {
		return nil, errConflictMsg("this rule is asking you; answer it instead")
	}
	recipe := Recipe{ID: r.RecipeID, Name: r.RecipeName}
	if got, ok, err := s.repo.Get(ctx, campaignID, r.RecipeID); err != nil {
		return nil, errInternal(err)
	} else if ok {
		recipe = got
	}
	// Claim it first, so two Directors pressing Run can't both run it.
	if _, err := s.finish(ctx, r, RunHeld, RunRunning, nil, v); err != nil {
		return nil, err
	}
	ctx, _ = withBudget(ctx)
	status, lines, at := s.execute(ctx, campaignID, recipe, r.Rule, 0, r.Scope, r.Payload, v)
	if status == RunHeld {
		r.Asked = at + 1
		s.setWaiter(ctx, &r)
	}
	out, err := s.finish(ctx, r, RunRunning, status, lines, v)
	if err == nil && status == RunHeld {
		s.announce(ctx, r, true)
	}
	return out, err
}

// Dismiss sets a waiting rule aside without running it. An ask set aside
// reads to its asker as a no; a roll set aside tells its player so.
func (s *service) Dismiss(ctx context.Context, campaignID, id string, v Viewer) (*RunView, error) {
	r, err := s.heldRun(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	if s.waitsOnRoll(r) {
		return s.setRollAside(ctx, r, v)
	}
	if r.Asked > 0 {
		return s.Answer(ctx, campaignID, id, v, AnswerInput{Answer: AnswerNo})
	}
	return s.finish(ctx, r, RunHeld, RunDismissed, []string{"Set aside."}, v)
}
