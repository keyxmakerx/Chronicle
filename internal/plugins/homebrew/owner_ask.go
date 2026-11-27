package homebrew

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// An owner's ask is a player's move that an owner holds back for the
// Director, such as a card that asks first. It is the "Ask the Director" step
// with no recipe of its own: the run's first step is the ask, and every saved
// rule on the owner's event adds its steps after it, so they run on Yes and a
// "Yes, and…" can change them. The owner hears the answer through
// EventAsk.Answered and does the move itself, so there is one ask and one
// place it is answered, wherever the Director answers it from.

// askKeyField is where an owner's ask keeps its key in the run's payload.
const askKeyField = "askKey"

// EventAsk is what an owner gives for an event that is its own ask.
type EventAsk struct {
	// Owner names who asks where the drawer shows a recipe's name.
	Owner string
	// Words says the ask to one reader: who wants it and what they want to
	// do. reader is a Director in the drawer and the API, and the asker in
	// their own list, so an owner leaves out what that reader may not see.
	Words func(ctx context.Context, campaignID string, payload json.RawMessage, reader Viewer) AskWords
	// Answered hears the Director's first answer, after the ask stopped
	// waiting and before any later step runs. On Yes an error stops the run
	// there, with the owner's words, and nothing after the ask runs.
	Answered func(ctx context.Context, campaignID string, a AskAnswered) error
}

// AskWords is an ask as one reader may read it. An empty Who leaves the
// asker named by their membership.
type AskWords struct {
	Who  string
	What string
}

// AskAnswered is a Director's answer as the owner hears it.
type AskAnswered struct {
	Kind    string // AnswerYes, AnswerNo or AnswerOther
	Reply   string
	Payload json.RawMessage
	Scope   string
	// Asker is who set the ask off; By is the Director who answered.
	Asker string
	By    Viewer
}

// OwnerAsk is an owner's ask as it is put to the Director. Key is the
// owner's own name for what waits (a card's copy), unique while it waits.
type OwnerAsk struct {
	Event   string
	Key     string
	Scope   string
	Payload json.RawMessage
}

func checkEventAsk(e EventDef) error {
	if e.Ask == nil {
		return nil
	}
	switch {
	case e.Ask.Answered == nil || e.Ask.Words == nil:
		return fmt.Errorf("homebrew: event %q is an ask but has no Words or Answered", e.Name)
	case len(e.PartKinds) > 0 || e.Press != "" || e.Pressed != nil || e.PressArg != "" || e.Preview != nil:
		return fmt.Errorf("homebrew: event %q is an ask, so it can't come from a part, be a button or have a preview", e.Name)
	}
	return nil
}

// ownerAsk is the owner of a run that is an owner's ask, or nil.
func (s *service) ownerAsk(r Run) *EventAsk {
	if r.RecipeID != "" || r.Asked == 0 {
		return nil
	}
	if e, ok := s.reg.event(r.Rule.When.Event); ok {
		return e.Ask
	}
	return nil
}

// askKey reads an owner's ask key from a run's payload.
func askKey(payload json.RawMessage) string {
	var m map[string]any
	if len(payload) == 0 || json.Unmarshal(payload, &m) != nil {
		return ""
	}
	k, _ := m[askKeyField].(string)
	return k
}

// withAskKey puts the key into the payload, which must be a JSON object.
func withAskKey(payload json.RawMessage, key string) (json.RawMessage, error) {
	m := map[string]any{}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &m); err != nil {
			return nil, fmt.Errorf("homebrew: an ask's payload must be an object: %w", err)
		}
	}
	m[askKeyField] = key
	return json.Marshal(m)
}

// ownerWords asks the owner how a run reads to reader.
func (s *service) ownerWords(ctx context.Context, campaignID string, own *EventAsk, r Run, reader Viewer) AskWords {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("homebrew: an owner's ask words panicked", slog.Any("panic", p), slog.String("campaign_id", campaignID))
		}
	}()
	return own.Words(ctx, campaignID, r.Payload, reader)
}

// AskFor puts an owner's ask to the Directors and returns the waiting run's
// id. v is the asker with a Director's standing, as an owner's Fire is
// called, since the rules that wait behind the ask are the Director's own. A
// second ask under a key that already waits answers that one's id.
func (s *service) AskFor(ctx context.Context, campaignID string, v Viewer, a OwnerAsk) (string, error) {
	if err := requireDirector(v); err != nil {
		return "", err
	}
	if s.runs == nil {
		return "", nil
	}
	def, ok := s.reg.event(a.Event)
	if !ok || def.Ask == nil {
		return "", fmt.Errorf("homebrew: %q is not an owner's ask", a.Event)
	}
	if !validToken(a.Key) {
		return "", fmt.Errorf("homebrew: an owner's ask needs a key")
	}
	payload, err := withAskKey(a.Payload, a.Key)
	if err != nil {
		return "", err
	}
	held, err := s.runs.ListRuns(ctx, campaignID, true, maxHeldRuns)
	if err != nil {
		return "", fmt.Errorf("homebrew: list held runs: %w", err)
	}
	for _, h := range held {
		if h.RecipeID == "" && h.Asked == 1 && h.Rule.When.Event == a.Event && askKey(h.Payload) == a.Key {
			return h.ID, nil
		}
	}
	if len(held) >= maxHeldRuns {
		return "", errConflictMsg("too many rules already wait for you")
	}
	run := Run{ID: s.newID(), CampaignID: campaignID, RecipeName: clip(def.Ask.Owner, MaxNameChars), What: clip(def.Label, maxWhatChars),
		Scope: a.Scope, Payload: payload, Status: RunHeld, Lines: []string{}, CreatedBy: v.UserID, CreatedAt: time.Now().UTC(), Asked: 1}
	words := s.ownerWords(ctx, campaignID, def.Ask, run, v)
	args, err := json.Marshal(askArgs{Ask: words.What})
	if err != nil {
		return "", err
	}
	if args, err = checkAskArgs(ctx, campaignID, args); err != nil {
		args = json.RawMessage(`{}`)
	}
	run.Rule = Rule{When: When{Event: a.Event}, Timing: TimingNow, Then: []Do{{Action: AskAction, Args: args}}}
	if run.Rule.Then, err = s.gather(ctx, campaignID, def, payload, run.Rule.Then); err != nil {
		return "", err
	}
	if err := s.saveRun(ctx, run); err != nil {
		return "", err
	}
	s.asked(ctx, run)
	return run.ID, nil
}

// gather appends, after the ask, the steps of every saved rule the ask's
// event meets, each marked with its recipe so its part and log note are that
// recipe's. A rule's timing and test give way to the ask: the Director's Yes
// is what lets its steps run.
func (s *service) gather(ctx context.Context, campaignID string, def EventDef, payload json.RawMessage, then []Do) ([]Do, error) {
	recipes, err := s.repo.List(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("homebrew: list recipes: %w", err)
	}
	for _, r := range recipes {
		for _, rule := range r.Doc.Rules {
			if rule.When.Event != def.Name || rule.When.Part != "" {
				continue
			}
			if def.Match != nil {
				ok, err := def.Match(rule.When.Args, payload)
				if err != nil {
					return nil, fmt.Errorf("homebrew: match %s: %w", def.Name, err)
				}
				if !ok {
					continue
				}
			}
			for _, d := range rule.Then {
				if len(then) > maxRunSteps {
					slog.Warn("homebrew: an ask gathered more steps than a run may take; the rest were left out",
						slog.String("campaign_id", campaignID), slog.String("event", def.Name))
					return then, nil
				}
				d.Recipe = r.ID
				then = append(then, d)
			}
		}
	}
	return then, nil
}

// AnswerFor answers the owner's ask waiting under key, as Answer does. found
// is false when none waits there: an ask from before the owner asked through
// the Workshop, or one already answered, which the owner then handles alone.
func (s *service) AnswerFor(ctx context.Context, campaignID, event, key string, v Viewer, in AnswerInput) (*RunView, bool, error) {
	if err := requireDirector(v); err != nil {
		return nil, false, err
	}
	if s.runs == nil || key == "" {
		return nil, false, nil
	}
	held, err := s.runs.ListRuns(ctx, campaignID, true, maxHeldRuns)
	if err != nil {
		return nil, false, errInternal(err)
	}
	for _, r := range held {
		if r.RecipeID == "" && r.Asked == 1 && r.Rule.When.Event == event && askKey(r.Payload) == key {
			out, err := s.Answer(ctx, campaignID, r.ID, v, in)
			return out, true, err
		}
	}
	return nil, false, nil
}

// tellOwner hands the owner the first answer to its ask. A panic is the
// owner's failure, reported as one.
func (s *service) tellOwner(ctx context.Context, own *EventAsk, r Run, kind, reply string, by Viewer) (err error) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("homebrew: an owner's answer panicked", slog.Any("panic", p), slog.String("campaign_id", r.CampaignID))
			err = fmt.Errorf("homebrew: the owner's answer failed")
		}
	}()
	return own.Answered(ctx, r.CampaignID, AskAnswered{Kind: kind, Reply: reply, Payload: r.Payload, Scope: r.Scope, Asker: r.CreatedBy, By: by})
}

// stepRecipes finds the recipe each step of a run belongs to: the run's own,
// or, in an owner's ask, the one the step names. A recipe deleted since is
// an empty one, so a step on its part fails with the usual line.
type stepRecipes struct {
	s    *service
	ctx  context.Context
	cid  string
	base Recipe
	got  map[string]Recipe
}

func (s *service) stepRecipes(ctx context.Context, campaignID string, base Recipe) *stepRecipes {
	return &stepRecipes{s: s, ctx: ctx, cid: campaignID, base: base, got: map[string]Recipe{}}
}

func (sr *stepRecipes) of(d Do) Recipe {
	if d.Recipe == "" || d.Recipe == sr.base.ID {
		return sr.base
	}
	if r, ok := sr.got[d.Recipe]; ok {
		return r
	}
	r := Recipe{ID: d.Recipe}
	if validToken(d.Recipe) {
		if got, ok, err := sr.s.repo.Get(sr.ctx, sr.cid, d.Recipe); err == nil && ok {
			r = got
		}
	}
	sr.got[d.Recipe] = r
	return r
}

// CloseFor ends the owner's ask waiting under key when what it asked about
// is gone some other way (a Director took the card back, discarded or played
// it). Nothing runs and the owner is not told, since it closed it; the run
// is withdrawn with the owner's line, and screens hear it stopped waiting.
// found is false when no ask waits there.
func (s *service) CloseFor(ctx context.Context, campaignID, event, key string, v Viewer, line string) (bool, error) {
	if err := requireDirector(v); err != nil {
		return false, err
	}
	if s.runs == nil || key == "" {
		return false, nil
	}
	held, err := s.runs.ListRuns(ctx, campaignID, true, maxHeldRuns)
	if err != nil {
		return false, errInternal(err)
	}
	for _, r := range held {
		if r.RecipeID != "" || r.Asked != 1 || r.Rule.When.Event != event || askKey(r.Payload) != key {
			continue
		}
		if line == "" {
			line = "Closed before an answer."
		}
		if _, err := s.finish(ctx, r, RunHeld, RunWithdrawn, append(r.Lines, clip(line, maxWhatChars)), v); err != nil {
			// Answered in the same instant: that answer stands.
			return false, nil
		}
		s.answered(ctx, r, AskTakenBack, v.UserID)
		return true, nil
	}
	return false, nil
}
