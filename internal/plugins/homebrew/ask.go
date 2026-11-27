package homebrew

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// AskAction is the "Ask the Director" step. A run that reaches it stops,
// having done the steps before it, and waits in the drawer's "Waiting on
// you" until a Director answers Yes (the steps after it run, perhaps with
// the Director's changes), No, or with a written reply (they don't).
const AskAction = "ask-director"

// Answers a Director can give an ask.
const (
	AnswerYes   = "yes"
	AnswerNo    = "no"
	AnswerOther = "other"
)

// What an asker sees of their ask.
const (
	AskWaiting   = "waiting"
	AskTakenBack = "taken_back"
)

const (
	// MaxAskChars bounds the words the recipe author gives the ask.
	MaxAskChars = 80
	// MaxReplyChars bounds a Director's written reply.
	MaxReplyChars = 200
	// myAsks is how many of their own asks a player is shown.
	myAsks = 20
)

// Bell kinds. The words are the bell's notification types.
const (
	BellAsked    = "homebrew_ask"
	BellAnswered = "homebrew_ask_answered"
)

// askArgs is the ask step's own args: what the asker wants to do, in the
// author's words, so the Director reads "Mira wants to draw a card".
type askArgs struct {
	Ask string `json:"ask,omitempty"`
}

// AskDirector is the ask step as the registry holds it. The app registers
// it alongside every other owner's actions.
func AskDirector() ActionDef {
	return ActionDef{Name: AskAction, Label: "Ask the Director", Asks: true,
		Fields: []Field{{Name: "ask", Label: "They want to", Type: FieldText, Optional: true}},
		Check:  checkAskArgs}
}

// checkAskArgs keeps the ask's words one short line. "wants to" and "to" at
// the start are dropped, since the drawer says "<name> wants to" itself.
func checkAskArgs(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var a askArgs
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return nil, errInvalid("the ask takes only what they want to do")
	}
	w := strings.Join(strings.Fields(a.Ask), " ")
	for _, lead := range []string{"wants to ", "to "} {
		if len(w) > len(lead) && strings.EqualFold(w[:len(lead)], lead) {
			w = w[len(lead):]
		}
	}
	w, err := cleanText("what they want to do", w, MaxAskChars)
	if err != nil {
		return nil, err
	}
	return json.Marshal(askArgs{Ask: w})
}

// Answer is a Director's answer to an ask, as stored on the run.
type Answer struct {
	Kind  string `json:"kind"`
	Reply string `json:"reply,omitempty"`
	// Changes say, one line each, what the Director changed in the steps
	// after the ask ("Copies: 2 (was 1)").
	Changes []string `json:"changes,omitempty"`
}

// AnswerInput is a Director's answer as sent. Steps change the args or part
// of steps after the ask, and only on Yes.
type AnswerInput struct {
	Answer string     `json:"answer"`
	Reply  string     `json:"reply,omitempty"`
	Steps  []StepEdit `json:"steps,omitempty"`
}

// StepEdit changes one step after the ask. A nil Part or empty Args keeps
// what the rule says.
type StepEdit struct {
	Step int             `json:"step"`
	Part *string         `json:"part,omitempty"`
	Args json.RawMessage `json:"args,omitempty"`
}

// AskDetail is what the drawer and the API show of a run that asked.
type AskDetail struct {
	Asker   string `json:"asker"`
	AskerID string `json:"askerId,omitempty"`
	Ask     string `json:"ask,omitempty"`
	// Step is the ask's index among the run's steps.
	Step int `json:"step"`
	// Later are the steps after the ask, each with what an answer may change.
	Later  []AskStep `json:"later"`
	Answer *Answer   `json:"answer,omitempty"`
}

// AskStep is one step after an ask, with the fields and parts the Director
// may change while answering: the action's own Fields and the recipe's parts
// of its PartKinds, so any action offers itself as a condition.
type AskStep struct {
	Step   int             `json:"step"`
	Action string          `json:"action"`
	Label  string          `json:"label"`
	Part   string          `json:"part,omitempty"`
	Args   json.RawMessage `json:"args,omitempty"`
	Fields []Field         `json:"fields,omitempty"`
	Parts  []PartChoice    `json:"parts,omitempty"`
}

// PartChoice is a part a step may be pointed at.
type PartChoice struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// AskView is an ask as its asker sees it: their own words, the state, and
// what came back. It never carries the recipe, the rule or its lines, which
// are the Director's.
type AskView struct {
	ID      string    `json:"id"`
	Ask     string    `json:"ask,omitempty"`
	State   string    `json:"state"`
	Reply   string    `json:"reply,omitempty"`
	Changes []string  `json:"changes,omitempty"`
	At      time.Time `json:"at"`
	// CanTakeBack is true while the ask waits.
	CanTakeBack bool `json:"canTakeBack,omitempty"`
}

// AskPeople names people and says who the Directors are. It is filled by
// the app over the campaigns service.
type AskPeople interface {
	Name(ctx context.Context, campaignID, userID string) string
	Directors(ctx context.Context, campaignID string) ([]string, error)
}

// Notice is one bell entry.
type Notice struct {
	Users   []string
	Kind    string
	Message string
	Detail  string
	Link    string
}

// Bell writes bell entries through the notification service.
type Bell interface {
	Ring(ctx context.Context, campaignID string, n Notice) error
}

// AskEvent is a live message about an ask: it was made (State waiting) or
// answered or taken back. It carries the run's id and state only; a screen
// that hears it reads the ask again through its own route. Asker is who the
// message also reaches, besides the Directors.
type AskEvent struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Asker string `json:"-"`
}

// Live publishes ask events to connected screens.
type Live interface {
	AskChanged(campaignID string, e AskEvent)
}

// Option configures the service.
type Option func(*service)

// WithAskPeople sets who names the asker and lists the Directors.
func WithAskPeople(p AskPeople) Option { return func(s *service) { s.people = p } }

// WithBell sets where ask notices are written.
func WithBell(b Bell) Option { return func(s *service) { s.bell = b } }

// WithLive sets who carries live ask messages.
func WithLive(l Live) Option { return func(s *service) { s.live = l } }

// askWords reads the ask step's words from a rule.
func askWords(rule Rule, step int) string {
	if step < 0 || step >= len(rule.Then) {
		return ""
	}
	var a askArgs
	_ = json.Unmarshal(rule.Then[step].Args, &a)
	return a.Ask
}

// upperFirst capitalises a sentence's first letter for a heading.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

func (s *service) nameOf(ctx context.Context, campaignID, userID string) string {
	if s.people != nil && userID != "" {
		if n := s.people.Name(ctx, campaignID, userID); n != "" {
			return n
		}
	}
	return "A player"
}

// askDetail is the part of a run's view that does not need the recipe.
func (s *service) askDetail(r Run) *AskDetail {
	d := &AskDetail{AskerID: r.CreatedBy, Ask: askWords(r.Rule, r.Asked-1), Step: r.Asked - 1, Later: []AskStep{}, Answer: r.Answer}
	for i := r.Asked; i < len(r.Rule.Then); i++ {
		st := r.Rule.Then[i]
		as := AskStep{Step: i, Action: st.Action, Label: st.Action, Part: st.Part, Args: st.Args}
		if a, ok := s.reg.action(st.Action); ok {
			as.Label, as.Fields = a.Label, a.Fields
		}
		d.Later = append(d.Later, as)
	}
	return d
}

// fillAsk names the asker and lists, for each step after a waiting ask, the
// recipe's parts it may be pointed at.
func (s *service) fillAsk(ctx context.Context, campaignID string, rv *RunView, recipes []Recipe) {
	if rv.Ask == nil {
		return
	}
	rv.Ask.Asker = s.nameOf(ctx, campaignID, rv.Ask.AskerID)
	if rv.Status != RunHeld {
		return
	}
	var doc *Doc
	for i := range recipes {
		if recipes[i].ID == rv.RecipeID {
			doc = &recipes[i].Doc
		}
	}
	if doc == nil {
		return
	}
	for i, st := range rv.Ask.Later {
		a, ok := s.reg.action(st.Action)
		if !ok || len(a.PartKinds) == 0 {
			continue
		}
		for _, p := range doc.Parts {
			if listHas(a.PartKinds, p.Kind) {
				rv.Ask.Later[i].Parts = append(rv.Ask.Later[i].Parts, PartChoice{Key: p.Key, Label: p.Label})
			}
		}
	}
}

// asked tells the Directors a run is asking and every open screen that it
// is waiting. It never fails the firing that made it.
func (s *service) asked(ctx context.Context, r Run) {
	if s.live != nil {
		s.live.AskChanged(r.CampaignID, AskEvent{ID: r.ID, State: AskWaiting, Asker: r.CreatedBy})
	}
	if s.bell == nil || s.people == nil {
		return
	}
	dirs, err := s.people.Directors(ctx, r.CampaignID)
	if err != nil {
		slog.Warn("homebrew: could not list the Directors to tell of an ask", slog.Any("error", err))
		return
	}
	var to []string
	for _, id := range dirs {
		if id != "" && id != r.CreatedBy {
			to = append(to, id)
		}
	}
	if len(to) == 0 {
		return
	}
	msg := s.nameOf(ctx, r.CampaignID, r.CreatedBy)
	if w := askWords(r.Rule, r.Asked-1); w != "" {
		msg += " wants to " + w
	} else {
		msg += " asks you about " + r.RecipeName
	}
	n := Notice{Users: to, Kind: BellAsked, Message: clip(msg, maxWhatChars), Link: "/campaigns/" + r.CampaignID + "/dm-screen"}
	if err := s.bell.Ring(ctx, r.CampaignID, n); err != nil {
		slog.Warn("homebrew: ask notice not written", slog.Any("error", err))
	}
}

// answered tells the asker what came back and every open screen that the ask
// no longer waits. Nobody is rung about their own act.
func (s *service) answered(ctx context.Context, r Run, state, by string) {
	if s.live != nil {
		s.live.AskChanged(r.CampaignID, AskEvent{ID: r.ID, State: state, Asker: r.CreatedBy})
	}
	if s.bell == nil || r.CreatedBy == "" || r.CreatedBy == by || r.Answer == nil {
		return
	}
	w := askWords(r.Rule, r.Asked-1)
	if w == "" {
		w = "your ask"
	}
	var msg string
	switch r.Answer.Kind {
	case AnswerYes:
		msg = "Your Director said yes: " + w
		if len(r.Answer.Changes) > 0 {
			msg = "Your Director said yes, with changes: " + w
		}
	case AnswerNo:
		msg = "Your Director said no: " + w
	default:
		msg = "Your Director replied: " + r.Answer.Reply
	}
	detail := strings.Join(r.Answer.Changes, " ")
	if r.Answer.Kind != AnswerOther && r.Answer.Reply != "" {
		detail = strings.TrimSpace(detail + " " + r.Answer.Reply)
	}
	n := Notice{Users: []string{r.CreatedBy}, Kind: BellAnswered, Message: clip(msg, maxWhatChars), Detail: detail,
		Link: "/campaigns/" + r.CampaignID + "/homebrew/asks"}
	if err := s.bell.Ring(ctx, r.CampaignID, n); err != nil {
		slog.Warn("homebrew: answer notice not written", slog.Any("error", err))
	}
}

// askingRun loads a run that is waiting for an answer.
func (s *service) askingRun(ctx context.Context, campaignID, id string) (Run, error) {
	if s.runs == nil || !validToken(id) {
		return Run{}, errNotFound("ask")
	}
	r, ok, err := s.runs.GetRun(ctx, campaignID, id)
	if err != nil {
		return Run{}, errInternal(err)
	}
	if !ok || r.Asked == 0 {
		return Run{}, errNotFound("ask")
	}
	if r.Status != RunHeld {
		return Run{}, errConflictMsg("this ask was already answered or taken back")
	}
	return r, nil
}

// Answer answers a waiting ask. Yes runs the steps after it, with any of
// them changed as the Director said; each change goes through the step
// owner's own Check, exactly as a saved recipe does, and a refusal leaves
// the ask waiting with nothing run. No and a written reply run nothing more.
// The run moves on from held only once, so two Directors can't both answer.
func (s *service) Answer(ctx context.Context, campaignID, id string, v Viewer, in AnswerInput) (*RunView, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	r, err := s.askingRun(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	reply, err := cleanText("your reply", in.Reply, MaxReplyChars)
	if err != nil {
		return nil, err
	}
	switch in.Answer {
	case AnswerNo, AnswerOther:
		if len(in.Steps) > 0 {
			return nil, errInvalid("only a yes can change what happens next")
		}
		status, line := RunDeclined, "Answered no."
		if in.Answer == AnswerOther {
			if reply == "" {
				return nil, errInvalid("write the reply the player gets")
			}
			status, line = RunReplied, "Replied: "+reply
		}
		r.Answer = &Answer{Kind: in.Answer, Reply: reply}
		out, err := s.finish(ctx, r, RunHeld, status, append(r.Lines, line), v)
		if err != nil {
			return nil, err
		}
		s.answered(ctx, r, in.Answer, v.UserID)
		return out, nil
	case AnswerYes:
	default:
		return nil, errInvalid("answer yes, no or other")
	}

	recipe := Recipe{ID: r.RecipeID, Name: r.RecipeName}
	if got, ok, err := s.repo.Get(ctx, campaignID, r.RecipeID); err != nil {
		return nil, errInternal(err)
	} else if ok {
		recipe = got
	}
	rule, changes, err := s.applyEdits(ctx, campaignID, recipe.Doc, r, in.Steps)
	if err != nil {
		return nil, err
	}
	r.Rule, r.Answer = rule, &Answer{Kind: AnswerYes, Reply: reply, Changes: changes}
	// Claim it first, so a second answer finds it already taken.
	if _, err := s.finish(ctx, r, RunHeld, RunRunning, r.Lines, v); err != nil {
		return nil, err
	}
	s.answered(ctx, r, AnswerYes, v.UserID)
	ctx, _ = withBudget(ctx)
	status, lines, at := s.execute(ctx, campaignID, recipe, r.Rule, r.Asked, r.Scope, r.Payload, v)
	lines = append(append(append([]string{}, r.Lines...), "Answered yes."), lines...)
	if status == RunHeld {
		r.Asked = at + 1
	}
	out, err := s.finish(ctx, r, RunRunning, status, lines, v)
	if err == nil && status == RunHeld {
		s.asked(ctx, r)
	}
	return out, err
}

// applyEdits returns the run's rule with the Director's changes to the steps
// after the ask, checked as a saved recipe's steps are, and a line for each
// field or part that changed.
func (s *service) applyEdits(ctx context.Context, campaignID string, doc Doc, r Run, edits []StepEdit) (Rule, []string, error) {
	rule := r.Rule
	rule.Then = append([]Do(nil), r.Rule.Then...)
	if len(edits) == 0 {
		return rule, nil, nil
	}
	kinds := partKindsByKey(doc)
	seen := map[int]bool{}
	var changes []string
	for _, e := range edits {
		if e.Step < r.Asked || e.Step >= len(rule.Then) || seen[e.Step] {
			return Rule{}, nil, errInvalid("an answer can change each step after the ask once")
		}
		seen[e.Step] = true
		was := rule.Then[e.Step]
		d := was
		if e.Part != nil {
			d.Part = *e.Part
		}
		if len(e.Args) > 0 {
			d.Args = e.Args
		}
		got, err := checkStep(ctx, s.reg, campaignID, fmt.Sprintf("step %d", e.Step+1), d, kinds)
		if err != nil {
			return Rule{}, nil, err
		}
		rule.Then[e.Step] = got
		changes = append(changes, s.changeLines(doc, was, got)...)
	}
	return rule, changes, nil
}

// changeLines says what differs between a step as the rule had it and as the
// Director answered it, field by field in the action's own words.
func (s *service) changeLines(doc Doc, was, now Do) []string {
	var out []string
	a, _ := s.reg.action(now.Action)
	if was.Part != now.Part {
		label := now.Part
		if p, ok := partByKey(doc, now.Part); ok {
			label = p.Label
		}
		out = append(out, a.Label+": on "+label)
	}
	var before, after map[string]any
	_ = json.Unmarshal(was.Args, &before)
	_ = json.Unmarshal(now.Args, &after)
	for _, f := range a.Fields {
		b, n := argText(before[f.Name]), argText(after[f.Name])
		if b == n {
			continue
		}
		line := f.Label + ": " + n
		if b != "any" {
			line += " (was " + b + ")"
		}
		out = append(out, line)
	}
	return out
}

func argText(v any) string {
	switch x := v.(type) {
	case nil:
		return "any"
	case string:
		if x == "" {
			return "any"
		}
		return x
	case float64:
		return fmt.Sprintf("%g", x)
	}
	return fmt.Sprint(v)
}

// TakeBack lets the person who set off an ask withdraw it while it waits.
// Nobody else may, Directors included: a Director answers instead.
func (s *service) TakeBack(ctx context.Context, campaignID, id string, v Viewer) (*AskView, error) {
	if v.UserID == "" {
		return nil, errForbidden("only the person who asked can take it back")
	}
	r, err := s.askingRun(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if r.CreatedBy != v.UserID {
		return nil, errForbidden("only the person who asked can take it back")
	}
	now := time.Now().UTC()
	r.Status, r.Lines, r.DoneBy, r.DoneAt = RunWithdrawn, append(r.Lines, "Taken back by the player."), v.UserID, &now
	ok, err := s.runs.FinishRun(ctx, r, RunHeld)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflictMsg("this ask was already answered or taken back")
	}
	s.answered(ctx, r, AskTakenBack, v.UserID)
	out := askView(r)
	return &out, nil
}

// askView is a run as its asker sees it.
func askView(r Run) AskView {
	out := AskView{ID: r.ID, Ask: askWords(r.Rule, r.Asked-1), At: r.CreatedAt}
	if r.DoneAt != nil {
		out.At = *r.DoneAt
	}
	switch {
	case r.Status == RunHeld:
		out.State, out.CanTakeBack = AskWaiting, true
	case r.Status == RunWithdrawn:
		out.State = AskTakenBack
	case r.Answer != nil:
		out.State, out.Reply, out.Changes = r.Answer.Kind, r.Answer.Reply, r.Answer.Changes
	default:
		// Moving on from a yes: the Director has answered and it runs.
		out.State = AnswerYes
	}
	return out
}

// MyAsks lists the viewer's own asks, waiting ones and what came back, newest
// first. It is the only read of runs open to players, and it shows them
// nothing but their own words and the answer.
func (s *service) MyAsks(ctx context.Context, campaignID string, v Viewer) ([]AskView, error) {
	out := []AskView{}
	if v.UserID == "" || s.runs == nil {
		return out, nil
	}
	rs, err := s.runs.ListAsks(ctx, campaignID, v.UserID, myAsks)
	if err != nil {
		return nil, errInternal(err)
	}
	for _, r := range rs {
		out = append(out, askView(r))
	}
	return out, nil
}

// Asks lists the asks waiting for a Director, oldest first so the first
// asked is answered first, as the drawer and the Foundry module show them.
func (s *service) Asks(ctx context.Context, campaignID string, v Viewer) ([]RunView, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	out := []RunView{}
	if s.runs == nil {
		return out, nil
	}
	rs, err := s.runs.ListRuns(ctx, campaignID, true, maxWaiting)
	if err != nil {
		return nil, errInternal(err)
	}
	recipes, err := s.repo.List(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i].Asked == 0 {
			continue
		}
		rv := s.view(rs[i])
		s.fillAsk(ctx, campaignID, &rv, recipes)
		out = append(out, rv)
	}
	return out, nil
}
