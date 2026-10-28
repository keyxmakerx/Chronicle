package rolls

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Audience says who a live roll may reach. Exactly one shape is used:
// Everyone; DirectorsOnly; or Users, which Directors also receive unless
// Strict is set.
type Audience struct {
	Everyone      bool
	DirectorsOnly bool
	Users         []string
	Strict        bool
}

// Publisher sends a roll to the campaign's live connections. It is
// implemented in internal/app over the WebSocket hub, which drops the message
// for anyone outside the audience before it is sent.
type Publisher interface {
	RollMade(campaignID string, v View, a Audience)
}

// Service is the roll log's business logic. The rules, in one place:
//   - Chronicle's own rolls are decided here, never by a browser.
//   - Public rolls are everyone's. A Director-only roll is the Directors' and
//     the roller's. A blind roll is the Directors'; its roller is told only
//     that it was made. A self roll is the roller's alone, Directors included.
//   - The log a viewer reads and the live messages they receive follow the
//     same rule (the repository's visibleTo and audienceOf below).
type Service interface {
	// Table makes a roll asked for at the Table tab.
	Table(ctx context.Context, campaignID string, v Viewer, in TableInput) (*View, error)
	// Report records a roll another app decided. A replay of the same origin
	// id records nothing and returns the recorded roll's id with duplicate.
	Report(ctx context.Context, campaignID string, in ReportInput) (id string, duplicate bool, err error)
	// Log adds a roll another plugin made on the server.
	Log(ctx context.Context, campaignID string, in LogInput) error
	// List is a page of the log as the viewer may see it, oldest first.
	List(ctx context.Context, campaignID string, v Viewer, before string, limit int) (*Page, error)
	// Settings is the table's dice look.
	Settings(ctx context.Context, campaignID string) (*Settings, error)
	// SetSettings changes the table's dice look. Directors only.
	SetSettings(ctx context.Context, campaignID string, v Viewer, in Settings) (*Settings, error)
}

type service struct {
	repo  Repository
	pub   Publisher
	roll  func(int) int
	now   func() time.Time
	limit *limiter
}

// NewService creates the roll log service. pub may be nil (nothing is sent
// live).
func NewService(repo Repository, pub Publisher) Service {
	return &service{repo: repo, pub: pub, roll: cryptoRoll, now: time.Now, limit: newLimiter(tableBurst, tableWindow)}
}

// How many Table rolls one person may make in a window, so a held key or a
// script can't flood everyone's log and screen.
const (
	tableBurst  = 30
	tableWindow = time.Minute
)

func errInvalid(msg string) error { return apperror.NewBadRequest(msg) }

// clip trims s and cuts it to max characters.
func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

func validMode(m string) bool {
	return m == ModePublic || m == ModeGM || m == ModeBlind || m == ModeSelf
}

func (s *service) Table(ctx context.Context, campaignID string, v Viewer, in TableInput) (*View, error) {
	if v.UserID == "" {
		return nil, apperror.NewForbidden("sign in to roll")
	}
	terms, shown, ok := parseFormula(in.Formula)
	if !ok {
		return nil, errInvalid("Write the dice like 2d10+2, d20 or 4d6.")
	}
	mode := in.Mode
	if mode == "" {
		mode = ModePublic
	}
	if !validMode(mode) {
		return nil, errInvalid("Pick who sees the roll.")
	}
	// A Director's blind roll would hide it from no one who can see it, so
	// it is a Director-only roll.
	if v.Director && mode == ModeBlind {
		mode = ModeGM
	}
	if !s.limit.allow(campaignID+"/"+v.UserID, s.now()) {
		return nil, apperror.NewTooManyRequests("That's a lot of rolls at once. Wait a moment and roll again.")
	}
	dice, mod, total := throw(terms, s.roll)
	label := clip(in.Label, LabelMax)
	if label == "" {
		label = shown
	}
	r := Roll{
		ID: uuid.New().String(), CampaignID: campaignID, Origin: OriginChronicle, Source: SourceTable,
		RollerID: v.UserID, RollerName: clip(v.Name, NameMax), Label: label, Formula: shown,
		Dice: dice, Modifier: mod, Total: total, Mode: mode, CreatedAt: s.now().UTC(),
	}
	if _, _, err := s.repo.Insert(ctx, r); err != nil {
		return nil, err
	}
	s.publish(r, v.Director)
	out := viewOf(r, v)
	return &out, nil
}

func (s *service) Report(ctx context.Context, campaignID string, in ReportInput) (string, bool, error) {
	if in.Origin != OriginFoundry {
		return "", false, errInvalid("unknown origin")
	}
	id := strings.TrimSpace(in.OriginID)
	if id == "" || len(id) > OriginIDMax {
		return "", false, errInvalid("originId is required, at most 80 characters")
	}
	// A self roll has no audience Chronicle can name for certain, so it
	// stays in the app it was made in.
	if in.Mode != ModePublic && in.Mode != ModeGM && in.Mode != ModeBlind {
		return "", false, errInvalid(`mode must be "public", "gm" or "blind"`)
	}
	if len(in.Dice) > MaxDice {
		return "", false, errInvalid("too many dice")
	}
	sum := 0
	for _, d := range in.Dice {
		if d.Sides < 2 || d.Sides > MaxSides || d.Value < 1 || d.Value > d.Sides {
			return "", false, errInvalid("each die needs sides 2 to 1000 and a value it can show")
		}
		sum += d.Value
	}
	if in.Total < -MaxTotal || in.Total > MaxTotal {
		return "", false, errInvalid("total is out of range")
	}
	name := clip(in.RollerName, NameMax)
	if name == "" {
		name = "Someone in Foundry"
	}
	r := Roll{
		ID: uuid.New().String(), CampaignID: campaignID, Origin: in.Origin, OriginID: id, Source: SourceFoundry,
		RollerID: in.RollerID, RollerName: name, Label: clip(in.Label, LabelMax), Formula: clip(in.Formula, FormulaMax),
		Dice: in.Dice, Modifier: in.Total - sum, Total: in.Total, Mode: in.Mode, CreatedAt: s.now().UTC(),
	}
	if r.Label == "" {
		r.Label = r.Formula
	}
	got, dup, err := s.repo.Insert(ctx, r)
	if err != nil || dup {
		return got, dup, err
	}
	s.publish(r, false)
	return got, false, nil
}

func (s *service) Log(ctx context.Context, campaignID string, in LogInput) error {
	mode := in.Mode
	if mode == "" {
		mode = ModePublic
	}
	if !validMode(mode) {
		return errInvalid("unknown roll mode")
	}
	r := Roll{
		ID: uuid.New().String(), CampaignID: campaignID, Origin: OriginChronicle, Source: in.Source,
		RollerID: in.RollerID, RollerName: clip(in.RollerName, NameMax), Label: clip(in.Label, LabelMax),
		Formula: clip(in.Formula, FormulaMax), Dice: in.Dice, Modifier: in.Modifier, Total: in.Total,
		Outcome: clip(in.Outcome, OutcomeMax), Why: clip(in.Why, WhyMax), From: clip(in.From, FromMax),
		Mode: mode, CreatedAt: s.now().UTC(),
	}
	if _, _, err := s.repo.Insert(ctx, r); err != nil {
		return err
	}
	s.publish(r, false)
	return nil
}

func (s *service) List(ctx context.Context, campaignID string, v Viewer, before string, limit int) (*Page, error) {
	if limit < 1 {
		limit = DefaultPage
	}
	if limit > MaxPage {
		limit = MaxPage
	}
	rows, err := s.repo.List(ctx, campaignID, v, before, limit+1)
	if err != nil {
		return nil, err
	}
	page := &Page{Rolls: []View{}}
	if len(rows) > limit {
		page.More, rows = true, rows[:limit]
	}
	for i := len(rows) - 1; i >= 0; i-- {
		if visible(rows[i], v) {
			page.Rolls = append(page.Rolls, viewOf(rows[i], v))
		}
	}
	return page, nil
}

func (s *service) Settings(ctx context.Context, campaignID string) (*Settings, error) {
	look, err := s.repo.Look(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if look == "" {
		look = LookQuick
	}
	return &Settings{Look: look}, nil
}

func (s *service) SetSettings(ctx context.Context, campaignID string, v Viewer, in Settings) (*Settings, error) {
	if !v.Director {
		return nil, apperror.NewForbidden("only the Director sets the table's dice")
	}
	if in.Look != LookQuick && in.Look != LookRune && in.Look != LookNone {
		return nil, errInvalid("Pick quick 2D, rune 3D or numbers only.")
	}
	if err := s.repo.SetLook(ctx, campaignID, in.Look); err != nil {
		return nil, err
	}
	return &Settings{Look: in.Look}, nil
}

// visible is the repository's visibleTo again, so a repository that ever
// returned too much still could not leak a roll.
func visible(r Roll, v Viewer) bool {
	mine := r.RollerID != "" && r.RollerID == v.UserID
	switch r.Mode {
	case ModePublic:
		return true
	case ModeGM, ModeBlind:
		return mine || v.Director
	default:
		return mine
	}
}

// viewOf shapes a roll for one viewer: a blind roll's roller who is not a
// Director gets it without its result.
func viewOf(r Roll, v Viewer) View {
	out := View{
		ID: r.ID, Origin: r.Origin, OriginID: r.OriginID, Source: r.Source, RollerID: r.RollerID, RollerName: r.RollerName,
		Label: r.Label, Formula: r.Formula, Why: r.Why, From: r.From, Mode: r.Mode, CreatedAt: r.CreatedAt,
		Mine: r.RollerID != "" && r.RollerID == v.UserID,
	}
	if r.Mode == ModeBlind && !v.Director {
		out.Hidden = true
		return out
	}
	mod, total := r.Modifier, r.Total
	out.Dice, out.Modifier, out.Total, out.Outcome = r.Dice, &mod, &total, r.Outcome
	return out
}

// publish sends the roll live to its audience. rollerIsDirector says the
// roller already receives the full roll as a Director, so a blind roll needs
// no stub for them.
func (s *service) publish(r Roll, rollerIsDirector bool) {
	if s.pub == nil {
		return
	}
	full := viewOf(r, Viewer{Director: true})
	full.Mine = false
	switch r.Mode {
	case ModePublic:
		s.pub.RollMade(r.CampaignID, full, Audience{Everyone: true})
	case ModeGM:
		if r.RollerID == "" {
			s.pub.RollMade(r.CampaignID, full, Audience{DirectorsOnly: true})
		} else {
			s.pub.RollMade(r.CampaignID, full, Audience{Users: []string{r.RollerID}})
		}
	case ModeBlind:
		s.pub.RollMade(r.CampaignID, full, Audience{DirectorsOnly: true})
		if r.RollerID != "" && !rollerIsDirector {
			stub := viewOf(r, Viewer{})
			stub.Mine = false
			s.pub.RollMade(r.CampaignID, stub, Audience{Users: []string{r.RollerID}, Strict: true})
		}
	case ModeSelf:
		if r.RollerID != "" {
			s.pub.RollMade(r.CampaignID, full, Audience{Users: []string{r.RollerID}, Strict: true})
		}
	default:
		slog.Warn("rolls: a roll with an unknown mode was not sent", slog.String("mode", r.Mode))
	}
}

// limiter counts recent actions per key in a fixed window.
type limiter struct {
	mu     sync.Mutex
	burst  int
	window time.Duration
	seen   map[string][]time.Time
	swept  time.Time
}

func newLimiter(burst int, window time.Duration) *limiter {
	return &limiter{burst: burst, window: window, seen: map[string][]time.Time{}}
}

// allow records an action for key at now and reports whether it is within
// the burst. Old keys are swept once a window so the map stays small.
func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-l.window)
	if now.Sub(l.swept) > l.window {
		l.swept = now
		for k, ts := range l.seen {
			if len(ts) == 0 || ts[len(ts)-1].Before(cut) {
				delete(l.seen, k)
			}
		}
	}
	ts := l.seen[key]
	i := 0
	for i < len(ts) && ts[i].Before(cut) {
		i++
	}
	ts = ts[i:]
	if len(ts) >= l.burst {
		l.seen[key] = ts
		return false
	}
	l.seen[key] = append(ts, now)
	return true
}
