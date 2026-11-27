package homebrew

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// A held run that stopped at a step waits for one of these.
const (
	KindAsk  = "ask"
	KindRoll = "roll"
)

// What a player sees of a roll step, beside the ask states.
const (
	AskRoll     = "roll"      // waiting for the player's roll
	AskRolled   = "rolled"    // rolled at or over the target; the rule went on
	AskMissed   = "missed"    // rolled under the target; the rule stopped
	AskSetAside = "set_aside" // a Director set the waiting roll aside
)

// Bell kinds for a roll step.
const (
	BellRoll   = "homebrew_roll"   // to the player: a roll waits on you
	BellRolled = "homebrew_rolled" // to the Directors: the player rolled
)

// RollResult is a player's roll at a "rolls first" step as stored with the
// run: what was rolled, what it came to, and whether it met the target.
type RollResult struct {
	Label   string `json:"label,omitempty"`
	Formula string `json:"formula"`
	Target  int    `json:"target"`
	Total   int    `json:"total"`
	Made    bool   `json:"made"`
	RollID  string `json:"rollId,omitempty"`
}

// RollWait is a run waiting on a player's roll, as the drawer and the
// Foundry module show it. Problem says why that player can't roll it now
// (their hero has no such number), in the owner's words.
type RollWait struct {
	Player   string `json:"player,omitempty"`
	PlayerID string `json:"playerId,omitempty"`
	Label    string `json:"label,omitempty"`
	Formula  string `json:"formula,omitempty"`
	Target   int    `json:"target,omitempty"`
	Problem  string `json:"problem,omitempty"`
}

// RollView is a roll step as its player sees it: the offer while it waits,
// then what it came to.
type RollView struct {
	Label   string `json:"label,omitempty"`
	Formula string `json:"formula,omitempty"`
	Target  int    `json:"target"`
	Total   *int   `json:"total,omitempty"`
	Made    bool   `json:"made,omitempty"`
	Problem string `json:"problem,omitempty"`
}

// Players says who plays a page: the member who claimed it, or "". It is
// filled by the app over the entities plugin.
type Players interface {
	PlayerOf(ctx context.Context, campaignID, pageID string) string
}

// WithPlayers sets who finds the player of a run's page, so a roll waits on
// the hero's player even when a Director's press set the rule off.
func WithPlayers(p Players) Option { return func(s *service) { s.players = p } }

// rollStep is the roll action a run stopped at, if it stopped at one.
func (s *service) rollStep(r Run) (ActionDef, Do, bool) {
	if r.Asked <= 0 || r.Asked > len(r.Rule.Then) {
		return ActionDef{}, Do{}, false
	}
	d := r.Rule.Then[r.Asked-1]
	a, ok := s.reg.action(d.Action)
	if !ok || a.Rolls == nil {
		return ActionDef{}, Do{}, false
	}
	return a, d, true
}

// waitsOnRoll says whether a run is held for a player's roll.
func (s *service) waitsOnRoll(r Run) bool {
	_, _, ok := s.rollStep(r)
	return ok && r.Status == RunHeld
}

// setWaiter names who a run that just stopped waits on. A roll waits on the
// player of the page the event happened on, or else the person who set the
// rule off; an ask waits on the Directors.
func (s *service) setWaiter(ctx context.Context, r *Run) {
	r.WaitFor = ""
	if _, _, ok := s.rollStep(*r); !ok {
		return
	}
	if s.players != nil && r.Scope != "" {
		r.WaitFor = s.players.PlayerOf(ctx, r.CampaignID, r.Scope)
	}
	if r.WaitFor == "" {
		r.WaitFor = r.CreatedBy
	}
}

// announce tells people a run has stopped: the Directors of an ask, the
// player of a roll. ring false skips the player's bell, for a roll the
// asker has just heard about in their answer's bell.
func (s *service) announce(ctx context.Context, r Run, ring bool) {
	if _, _, ok := s.rollStep(r); !ok {
		s.asked(ctx, r)
		return
	}
	if s.live != nil {
		s.live.AskChanged(r.CampaignID, AskEvent{ID: r.ID, State: AskRoll, Asker: r.WaitFor})
	}
	if !ring || s.bell == nil || r.WaitFor == "" {
		return
	}
	msg := "Your Director's house rule asks you to roll first"
	if w := askWords(r.Rule, r.Asked-1); w != "" {
		msg += ": " + w
	}
	n := Notice{Users: []string{r.WaitFor}, Kind: BellRoll, Message: clip(msg, maxWhatChars),
		Link: "/campaigns/" + r.CampaignID + "/homebrew/asks"}
	if err := s.bell.Ring(ctx, r.CampaignID, n); err != nil {
		slog.Warn("homebrew: roll notice not written", slog.Any("error", err))
	}
}

// offer asks the step's owner what the player would roll now.
func (s *service) offer(ctx context.Context, r Run, player Viewer) (RollOffer, error) {
	a, d, ok := s.rollStep(r)
	if !ok {
		return RollOffer{}, errNotFound("roll")
	}
	return a.Rolls.Offer(ctx, r.CampaignID, Act{RecipeID: r.RecipeID, Args: d.Args, Scope: r.Scope, By: player})
}

// fillRoll names the player a waiting roll is for and what they would roll.
func (s *service) fillRoll(ctx context.Context, campaignID string, rv *RunView, r Run) {
	if rv.Roll == nil {
		return
	}
	rv.Roll.Player = s.nameOf(ctx, campaignID, r.WaitFor)
	o, err := s.offer(ctx, r, Viewer{UserID: r.WaitFor})
	if err != nil {
		rv.Roll.Problem = plainError(err)
		rv.Waits = "Waits for " + rv.Roll.Player + "'s roll. " + rv.Roll.Problem
		return
	}
	rv.Roll.Label, rv.Roll.Formula, rv.Roll.Target = o.Label, o.Formula, o.Target
	rv.Waits = fmt.Sprintf("Waits for %s to roll %s, %d or more. Only they can roll it.", rv.Roll.Player, rollName(o), o.Target)
}

// rollName is a roll as a sentence names it: "Might (2d10+2)" or "2d10".
func rollName(o RollOffer) string {
	if o.Label == "" {
		return o.Formula
	}
	return o.Label + " (" + o.Formula + ")"
}

// rollLine is the run's line for a roll made.
func rollLine(who string, got RollResult) string {
	what := rollName(RollOffer{Label: got.Label, Formula: got.Formula})
	if got.Made {
		return fmt.Sprintf("%s rolled %d on %s, needing %d: the rule goes on.", who, got.Total, what, got.Target)
	}
	return fmt.Sprintf("%s rolled %d on %s, under %d: the rule stops here.", who, got.Total, what, got.Target)
}

// Roll makes the roll a run waits on, as the player it waits on. The roll
// is the step owner's, made on the server; this compares its total with the
// target. At or over it, the rest of the rule runs; under it, the run ends.
// Nobody else may roll it, Directors included: they may set it aside.
func (s *service) Roll(ctx context.Context, campaignID, id string, v Viewer) (*AskView, error) {
	if v.UserID == "" {
		return nil, errForbidden("only the player this roll waits on can make it")
	}
	r, err := s.askingRun(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	a, d, ok := s.rollStep(r)
	if !ok {
		return nil, errConflictMsg("this rule is waiting for a Director's answer, not a roll")
	}
	if r.WaitFor == "" || r.WaitFor != v.UserID {
		return nil, errForbidden("only the player this roll waits on can make it")
	}
	recipe := Recipe{ID: r.RecipeID, Name: r.RecipeName}
	if got, ok, err := s.repo.Get(ctx, campaignID, r.RecipeID); err != nil {
		return nil, errInternal(err)
	} else if ok {
		recipe = got
	}
	// Claim it first, so a second press can't roll again.
	held := r.Lines
	if _, err := s.finish(ctx, r, RunHeld, RunRunning, held, v); err != nil {
		return nil, errConflictMsg("this roll was already made or set aside")
	}
	player := Viewer{UserID: v.UserID, VisibilityRole: v.VisibilityRole, Name: s.nameOf(ctx, campaignID, v.UserID)}
	got, err := a.Rolls.Roll(ctx, campaignID, Act{RecipeID: r.RecipeID, Args: d.Args, Scope: r.Scope, By: player, Note: s.note(recipe, r.Rule, r.Payload)})
	if err != nil {
		// Nothing was decided: the roll waits again for another press.
		if _, perr := s.runs.FinishRun(ctx, r, RunRunning); perr != nil {
			slog.Warn("homebrew: a roll that failed could not wait again", slog.String("run", r.ID), slog.Any("error", perr))
		}
		return nil, err
	}
	res := RollResult{Label: got.Label, Formula: got.Formula, Target: got.Target, Total: got.Total, Made: got.Total >= got.Target, RollID: got.RollID}
	if r.Answer == nil {
		r.Answer = &Answer{}
	}
	r.Answer.Roll = &res
	lines := append(append([]string{}, held...), rollLine(player.Name, res))
	status := RunMissed
	if res.Made {
		// The rest of the rule is the Director's, set off under their
		// standing as every owner's firing is; the player only rolled.
		rule := Viewer{UserID: v.UserID, Name: player.Name, Director: true, VisibilityRole: permissions.RoleOwner}
		ctx, _ = withBudget(ctx)
		var more []string
		var at int
		status, more, at = s.execute(ctx, campaignID, recipe, r.Rule, r.Asked, r.Scope, r.Payload, rule)
		lines = append(lines, more...)
		if status == RunHeld {
			r.Asked = at + 1
			s.setWaiter(ctx, &r)
		}
	}
	if _, err := s.finish(ctx, r, RunRunning, status, lines, v); err != nil {
		return nil, err
	}
	r.Status, r.Lines = status, lines
	state := AskMissed
	if res.Made {
		state = AskRolled
	}
	s.rolled(ctx, r, state, res, v.UserID, player.Name)
	if status == RunHeld {
		s.announce(ctx, r, r.WaitFor != v.UserID)
	}
	now := time.Now().UTC()
	r.DoneAt = &now
	out := s.askView(ctx, r, v)
	return &out, nil
}

// rolled tells every open screen the roll was made and rings the Directors
// with what it came to; the player saw it on their own screen.
func (s *service) rolled(ctx context.Context, r Run, state string, res RollResult, rollerID, who string) {
	if s.live != nil {
		s.live.AskChanged(r.CampaignID, AskEvent{ID: r.ID, State: state, Asker: rollerID})
	}
	if s.bell == nil || s.people == nil {
		return
	}
	dirs, err := s.people.Directors(ctx, r.CampaignID)
	if err != nil {
		slog.Warn("homebrew: could not list the Directors to tell of a roll", slog.Any("error", err))
		return
	}
	var to []string
	for _, id := range dirs {
		if id != "" && id != rollerID {
			to = append(to, id)
		}
	}
	if len(to) == 0 {
		return
	}
	n := Notice{Users: to, Kind: BellRolled, Message: clip(rollLine(who, res), maxWhatChars),
		Detail: "From the " + r.RecipeName + " recipe", Link: "/campaigns/" + r.CampaignID + "/dm-screen"}
	if err := s.bell.Ring(ctx, r.CampaignID, n); err != nil {
		slog.Warn("homebrew: roll result notice not written", slog.Any("error", err))
	}
}

// setRollAside ends a run waiting on a player's roll without the roll, and
// tells the player.
func (s *service) setRollAside(ctx context.Context, r Run, v Viewer) (*RunView, error) {
	out, err := s.finish(ctx, r, RunHeld, RunDismissed, append(append([]string{}, r.Lines...), "Set aside before the roll."), v)
	if err != nil {
		return nil, err
	}
	if s.live != nil {
		s.live.AskChanged(r.CampaignID, AskEvent{ID: r.ID, State: AskSetAside, Asker: r.WaitFor})
	}
	if s.bell != nil && r.WaitFor != "" && r.WaitFor != v.UserID {
		msg := "Your Director set aside the roll"
		if w := askWords(r.Rule, r.Asked-1); w != "" {
			msg += ": " + w
		}
		n := Notice{Users: []string{r.WaitFor}, Kind: BellAnswered, Message: clip(msg, maxWhatChars),
			Link: "/campaigns/" + r.CampaignID + "/homebrew/asks"}
		if err := s.bell.Ring(ctx, r.CampaignID, n); err != nil {
			slog.Warn("homebrew: set-aside notice not written", slog.Any("error", err))
		}
	}
	return out, nil
}
