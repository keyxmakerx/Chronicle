package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// What happened to a run.
const (
	RunDone      = "done"
	RunHeld      = "held" // waits in the Homebrew drawer for the Director
	RunFailed    = "failed"
	RunDismissed = "dismissed"
	RunStopped   = "stopped" // rules kept setting each other off
	RunRunning   = "running" // claimed by a Director pressing Run
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
}

// Press is an event the Director sets off from the drawer.
type Press struct {
	Event string `json:"event"`
	Label string `json:"label"`
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
			if s.holds(rule) {
				run.Status = RunHeld
			} else {
				run.Status, run.Lines = s.execute(ctx, campaignID, r, rule, f.Scope, v)
				now := run.CreatedAt
				run.DoneBy, run.DoneAt = v.UserID, &now
			}
			if err := s.saveRun(ctx, run); err != nil {
				return err
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
// carries a test only a person can judge, or does something the server
// can't.
func (s *service) holds(rule Rule) bool {
	if rule.Timing != TimingNow || rule.When.Test != "" {
		return true
	}
	for _, d := range rule.Then {
		if a, ok := s.reg.action(d.Action); !ok || a.ClientRun || a.Run == nil {
			return true
		}
	}
	return false
}

// execute runs a rule's actions in order and stops at the first that fails.
// Actions the server can't run are listed for the Director.
func (s *service) execute(ctx context.Context, campaignID string, r Recipe, rule Rule, scope string, v Viewer) (string, []string) {
	lines := []string{}
	for _, d := range rule.Then {
		a, ok := s.reg.action(d.Action)
		if !ok {
			lines = append(lines, "An action this server no longer has was skipped.")
			continue
		}
		if a.ClientRun || a.Run == nil {
			lines = append(lines, a.Label+": "+map[bool]string{true: "run in your browser", false: "for you to do"}[a.ClientRun]+".")
			continue
		}
		act := Act{RecipeID: r.ID, Args: d.Args, Scope: scope, By: v, Note: "Rule from " + r.Name}
		if d.Part != "" {
			p, ok := partByKey(r.Doc, d.Part)
			if !ok {
				lines = append(lines, a.Label+" did not run: its part is no longer in the recipe.")
				return RunFailed, lines
			}
			act.Part = &p
		}
		line, err := a.Run(ctx, campaignID, act)
		if err != nil {
			lines = append(lines, a.Label+" did not run: "+plainError(err))
			return RunFailed, lines
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return RunDone, lines
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
	if r.Status != RunHeld {
		return out
	}
	out.RecipeID, out.Scope = r.RecipeID, r.Scope
	switch {
	case r.Rule.When.Test != "":
		out.Waits = "Only if: " + r.Rule.When.Test
	case r.Rule.Timing == TimingEncounterEnd:
		out.Waits = "Set to wait for the end of the encounter."
	case r.Rule.Timing == TimingHeld:
		out.Waits = "Set to wait for you."
	default:
		out.Waits = "Part of it is yours to run."
	}
	for _, d := range r.Rule.Then {
		st := RunStep{Action: d.Action, Label: d.Action, Part: d.Part, Args: d.Args}
		if a, ok := s.reg.action(d.Action); ok {
			st.Label, st.Browser, st.ByHand = a.Label, a.ClientRun, !a.ClientRun && a.Run == nil
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
			if held {
				out.Waiting = append(out.Waiting, s.view(r))
			} else {
				out.Recent = append(out.Recent, s.view(r))
			}
		}
	}
	return out, nil
}

// Press sets off an event the Director starts from the drawer.
func (s *service) Press(ctx context.Context, campaignID, event string, v Viewer) (*Drawer, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	e, ok := s.reg.event(event)
	if !ok || e.Press == "" {
		return nil, errNotFound("button")
	}
	if err := s.Fire(ctx, campaignID, v, Fired{Event: e.Name}); err != nil {
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

// RunHeld runs a waiting rule now, as it was when its event fired.
func (s *service) RunHeld(ctx context.Context, campaignID, id string, v Viewer) (*RunView, error) {
	r, err := s.heldRun(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
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
	status, lines := s.execute(ctx, campaignID, recipe, r.Rule, r.Scope, v)
	return s.finish(ctx, r, RunRunning, status, lines, v)
}

// Dismiss sets a waiting rule aside without running it.
func (s *service) Dismiss(ctx context.Context, campaignID, id string, v Viewer) (*RunView, error) {
	r, err := s.heldRun(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	return s.finish(ctx, r, RunHeld, RunDismissed, []string{"Set aside."}, v)
}
