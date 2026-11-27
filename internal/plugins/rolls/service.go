package rolls

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
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
	// Record is Log that also returns the new roll's id, for a caller that
	// tells its own client which log row its roll became.
	Record(ctx context.Context, campaignID string, in LogInput) (string, error)
	// List is a page of the log as the viewer may see it, oldest first.
	List(ctx context.Context, campaignID string, v Viewer, before string, limit int) (*Page, error)
	// Settings is the campaign's dice: the table's look and the 3D dice picks.
	Settings(ctx context.Context, campaignID string) (*Settings, error)
	// SetSettings changes the campaign's dice, partially: only the fields sent
	// change. Directors only.
	SetSettings(ctx context.Context, campaignID string, v Viewer, in UpdateSettingsInput) (*Settings, error)
	// MyDice is the viewer's own dice pick and the dice their rolls are
	// thrown in right now.
	MyDice(ctx context.Context, campaignID string, v Viewer) (*MyDice, error)
	// SetMyDice replaces the viewer's own pick, checked against what the
	// campaign offers. The zero pick follows the campaign's dice again.
	SetMyDice(ctx context.Context, campaignID string, v Viewer, p DicePick) (*MyDice, error)
}

type service struct {
	repo  Repository
	pub   Publisher
	roll  func(int) int
	now   func() time.Time
	limit *limiter
	crits CritSource
	chars CharacterSource
}

// NewService creates the roll log service. pub may be nil (nothing is sent
// live); crits may be nil (the default crit rules apply); chars may be nil
// (no roll is ever made as a hero).
func NewService(repo Repository, pub Publisher, crits CritSource, chars CharacterSource) Service {
	return &service{repo: repo, pub: pub, crits: crits, chars: chars, roll: cryptoRoll, now: time.Now, limit: newLimiter(tableBurst, tableWindow)}
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
	charID := s.rollAs(ctx, campaignID, v, in.CharacterID, mode)
	dice, mod, total := throw(terms, s.roll)
	markCrits(s.critRules(ctx, campaignID), dice, termGroups(terms))
	label := clip(in.Label, LabelMax)
	if label == "" {
		label = shown
	}
	r := Roll{
		ID: uuid.New().String(), CampaignID: campaignID, Origin: OriginChronicle, Source: SourceTable,
		RollerID: v.UserID, RollerName: clip(v.Name, NameMax), Label: label, Formula: shown,
		Dice: dice, Modifier: mod, Total: total, Mode: mode, CharacterID: charID, CreatedAt: s.now().UTC(),
		RollerDice: s.rollerDice(ctx, campaignID, v.UserID),
	}
	if _, _, err := s.repo.Insert(ctx, r); err != nil {
		return nil, err
	}
	s.publish(r, v.Director)
	out := viewOf(r, v)
	return &out, nil
}

// rollAs keeps the hero a Table roll names only when the viewer may roll as
// it in this mode. Foundry posts the roll as that hero's actor, so a player
// naming someone else's hero would speak as them, and a public roll naming a
// hidden page would show it to the table; such a roll, or one whose hero
// can't be checked, is still made, under the roller's own name.
func (s *service) rollAs(ctx context.Context, campaignID string, v Viewer, characterID, mode string) string {
	id := strings.TrimSpace(characterID)
	if id == "" || len(id) > CharacterIDMax || s.chars == nil {
		return ""
	}
	ok, err := s.chars.MayRollAs(ctx, campaignID, v, id, mode)
	if err != nil {
		slog.Warn("rolls: cannot check the hero a roll names; rolling without it", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return ""
	}
	if !ok {
		return ""
	}
	return id
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
	// Copied so judging never writes into the caller's slice.
	dice := append([]Die(nil), in.Dice...)
	markCrits(s.critRules(ctx, campaignID), dice, runGroups(dice))
	name := clip(in.RollerName, NameMax)
	if name == "" {
		name = "Someone in Foundry"
	}
	r := Roll{
		ID: uuid.New().String(), CampaignID: campaignID, Origin: in.Origin, OriginID: id, Source: SourceFoundry,
		RollerID: in.RollerID, RollerName: name, Label: clip(in.Label, LabelMax), Formula: clip(in.Formula, FormulaMax),
		Dice: dice, Modifier: in.Total - sum, Total: in.Total, Mode: in.Mode, CreatedAt: s.now().UTC(),
		RollerDice: s.rollerDice(ctx, campaignID, in.RollerID),
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
	_, err := s.Record(ctx, campaignID, in)
	return err
}

func (s *service) Record(ctx context.Context, campaignID string, in LogInput) (string, error) {
	mode := in.Mode
	if mode == "" {
		mode = ModePublic
	}
	if !validMode(mode) {
		return "", errInvalid("unknown roll mode")
	}
	dice := append([]Die(nil), in.Dice...)
	markCrits(s.critRules(ctx, campaignID), dice, runGroups(dice))
	r := Roll{
		ID: uuid.New().String(), CampaignID: campaignID, Origin: OriginChronicle, Source: in.Source,
		RollerID: in.RollerID, RollerName: clip(in.RollerName, NameMax), Label: clip(in.Label, LabelMax),
		Formula: clip(in.Formula, FormulaMax), Dice: dice, Modifier: in.Modifier, Total: in.Total,
		Outcome: clip(in.Outcome, OutcomeMax), Why: clip(in.Why, WhyMax), From: clip(in.From, FromMax),
		Mode: mode, CreatedAt: s.now().UTC(), RollerDice: s.rollerDice(ctx, campaignID, in.RollerID),
	}
	if _, _, err := s.repo.Insert(ctx, r); err != nil {
		return "", err
	}
	s.publish(r, false)
	return r.ID, nil
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
	st, err := s.repo.Settings(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if st.DiceOffered == nil {
		st.DiceOffered = []string{}
	}
	if st.Look == "" {
		st.Look = LookQuick
	}
	return &st, nil
}

// diceColourSlug is the shape of a colour key. Whether it names a colour of
// the chosen set is the picker's to know (the catalogue lives in the browser);
// the engine falls back on a colour its set doesn't have.
var diceColourSlug = regexp.MustCompile(`^[a-z0-9-]{1,24}$`)

func oneOf(v string, list []string) bool {
	for _, k := range list {
		if v == k {
			return true
		}
	}
	return false
}

// checkChoice refuses a present value that is neither empty (the default) nor
// in list.
func checkChoice(f patch.Field[string], list []string, msg string) error {
	if v, ok := f.Get(); ok && v != "" && !oneOf(v, list) {
		return errInvalid(msg)
	}
	return nil
}

func (s *service) SetSettings(ctx context.Context, campaignID string, v Viewer, in UpdateSettingsInput) (*Settings, error) {
	if !v.Director {
		return nil, apperror.NewForbidden("only the Director sets the table's dice")
	}
	if l, ok := in.Look.Get(); ok && l != LookQuick && l != LookRune && l != LookNone {
		return nil, errInvalid("Pick quick 2D, rune 3D or numbers only.")
	}
	if err := checkChoice(in.DiceSet, DiceSetKeys, "Pick one of the dice sets."); err != nil {
		return nil, err
	}
	if c, ok := in.DiceColour.Get(); ok && c != "" && !diceColourSlug.MatchString(c) {
		return nil, errInvalid("That dice colour isn't one of the set's.")
	}
	if err := checkChoice(in.CritSucc, CritSuccKeys, "Pick one of the crit success animations."); err != nil {
		return nil, err
	}
	if err := checkChoice(in.CritFail, CritFailKeys, "Pick one of the crit fail animations."); err != nil {
		return nil, err
	}
	if q, ok := in.Quality.Get(); ok && q != "" && q != QualityAuto && q != QualityBasic && q != QualityHD {
		return nil, errInvalid("Pick auto, basic or HD detail.")
	}

	offered, err := cleanOffered(in.DiceOffered)
	if err != nil {
		return nil, err
	}
	cur, err := s.repo.Settings(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if cur.Look == "" {
		cur.Look = LookQuick
	}
	next := Settings{
		Look:        in.Look.Val(cur.Look),
		DiceSet:     in.DiceSet.Val(cur.DiceSet),
		DiceColour:  in.DiceColour.Val(cur.DiceColour),
		CritSucc:    in.CritSucc.Val(cur.CritSucc),
		CritFail:    in.CritFail.Val(cur.CritFail),
		Quality:     in.Quality.Val(cur.Quality),
		PlayersPick: in.PlayersPick.Val(cur.PlayersPick),
		DiceOffered: cur.DiceOffered,
	}
	if in.DiceOffered.Present() && !in.DiceOffered.IsNull() {
		next.DiceOffered = offered
	}
	if next.DiceOffered == nil {
		next.DiceOffered = []string{}
	}
	// The campaign's own die is always on offer, whichever of the two changed.
	own := next.DiceSet
	if own == "" {
		own = DiceSetKeys[0]
	}
	if len(next.DiceOffered) > 0 && !oneOf(own, next.DiceOffered) {
		return nil, errInvalid("The campaign's own dice set has to be one of the sets on offer.")
	}
	// Only what the body carried is written, so a stale read of the rest can't
	// undo someone else's change.
	var changed []string
	for _, c := range []struct {
		key string
		set bool
	}{
		{"look", has(in.Look)}, {"diceSet", has(in.DiceSet)}, {"diceColour", has(in.DiceColour)},
		{"diceOffered", has(in.DiceOffered)}, {"critSucc", has(in.CritSucc)}, {"critFail", has(in.CritFail)}, {"quality", has(in.Quality)}, {"playersPick", has(in.PlayersPick)},
	} {
		if c.set {
			changed = append(changed, c.key)
		}
	}
	if err := s.repo.SaveSettings(ctx, campaignID, next, changed); err != nil {
		return nil, err
	}
	return &next, nil
}

// cleanOffered checks a list of offered sets and returns it in catalogue
// order without repeats. Every set listed (or none) means every set is on
// offer, which is stored as the empty list.
func cleanOffered(f patch.Field[[]string]) ([]string, error) {
	list, ok := f.Get()
	if !ok {
		return nil, nil
	}
	for _, k := range list {
		if !oneOf(k, DiceSetKeys) {
			return nil, errInvalid("Pick offered dice from the dice sets.")
		}
	}
	out := []string{}
	for _, k := range DiceSetKeys {
		if oneOf(k, list) {
			out = append(out, k)
		}
	}
	if len(out) == len(DiceSetKeys) {
		return []string{}, nil
	}
	return out, nil
}

// isOffered is whether players may pick set; an empty offer list offers every
// set.
func isOffered(st Settings, set string) bool {
	return len(st.DiceOffered) == 0 || oneOf(set, st.DiceOffered)
}

// effectiveDice is the dice a member's rolls are thrown in, on every screen:
// the campaign's, with the member's own pick laid over it field by field
// while the campaign lets players pick. A stored pick that no longer counts
// (picking turned off, its set taken off the offer, a name the engine no
// longer has) is skipped, never deleted, so the campaign's dice show and
// widening the offer again brings the pick back. A set that is skipped takes
// its colour with it, since a colour belongs to its set.
func effectiveDice(st Settings, p DicePick) DiceLook {
	out := DiceLook{Set: st.DiceSet, Colour: st.DiceColour, CritSucc: st.CritSucc, CritFail: st.CritFail}
	if !st.PlayersPick {
		return out
	}
	if p.Set != "" && oneOf(p.Set, DiceSetKeys) && isOffered(st, p.Set) && (p.Colour == "" || diceColourSlug.MatchString(p.Colour)) {
		out.Set, out.Colour, out.Own = p.Set, p.Colour, true
	}
	if p.CritSucc != "" && oneOf(p.CritSucc, CritSuccKeys) {
		out.CritSucc, out.Own = p.CritSucc, true
	}
	if p.CritFail != "" && oneOf(p.CritFail, CritFailKeys) {
		out.CritFail, out.Own = p.CritFail, true
	}
	return out
}

// rollerDice settles the dice a new roll is thrown in. A roll with no roller
// (a Foundry user nobody linked) is thrown in the campaign's dice. When the
// settings or the pick can't be read the roll is still made, carrying no
// dice, and screens throw it in the campaign's.
func (s *service) rollerDice(ctx context.Context, campaignID, rollerID string) *DiceLook {
	st, err := s.repo.Settings(ctx, campaignID)
	if err != nil {
		slog.Warn("rolls: cannot read the campaign's dice; the roll carries none", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	var p DicePick
	if rollerID != "" {
		if p, err = s.repo.DicePick(ctx, campaignID, rollerID); err != nil {
			slog.Warn("rolls: cannot read the roller's dice; using the campaign's", slog.String("campaign_id", campaignID), slog.Any("error", err))
			p = DicePick{}
		}
	}
	look := effectiveDice(st, p)
	return &look
}

func (s *service) MyDice(ctx context.Context, campaignID string, v Viewer) (*MyDice, error) {
	if v.UserID == "" {
		return nil, apperror.NewForbidden("sign in to pick your dice")
	}
	st, err := s.repo.Settings(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.DicePick(ctx, campaignID, v.UserID)
	if err != nil {
		return nil, err
	}
	return &MyDice{Pick: p, Effective: effectiveDice(st, p), MayPick: st.PlayersPick}, nil
}

// SetMyDice stores the viewer's own pick. Only the viewer's own row is ever
// written, the campaign must let players pick, and every name must be one the
// engine draws, the set one on offer. The zero pick is always allowed, so
// anyone can go back to the campaign's dice.
func (s *service) SetMyDice(ctx context.Context, campaignID string, v Viewer, p DicePick) (*MyDice, error) {
	if v.UserID == "" {
		return nil, apperror.NewForbidden("sign in to pick your dice")
	}
	st, err := s.repo.Settings(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	// A colour belongs to its set; without a set the campaign's colour stands.
	if p.Set == "" {
		p.Colour = ""
	}
	if !p.IsZero() {
		if !st.PlayersPick {
			return nil, apperror.NewForbidden("Your Director has set everyone’s dice.")
		}
		if p.Set != "" && (!oneOf(p.Set, DiceSetKeys) || !isOffered(st, p.Set)) {
			return nil, errInvalid("Pick one of the dice sets on offer.")
		}
		if p.Colour != "" && !diceColourSlug.MatchString(p.Colour) {
			return nil, errInvalid("That dice colour isn't one of the set's.")
		}
		if p.CritSucc != "" && !oneOf(p.CritSucc, CritSuccKeys) {
			return nil, errInvalid("Pick one of the crit success animations.")
		}
		if p.CritFail != "" && !oneOf(p.CritFail, CritFailKeys) {
			return nil, errInvalid("Pick one of the crit fail animations.")
		}
	}
	if err := s.repo.SaveDicePick(ctx, campaignID, v.UserID, p); err != nil {
		return nil, err
	}
	return &MyDice{Pick: p, Effective: effectiveDice(st, p), MayPick: st.PlayersPick}, nil
}

// has is whether a field carries a value to write: an explicit null keeps the
// stored one (Field.Val), so it writes nothing.
func has[T any](f patch.Field[T]) bool { _, ok := f.Get(); return ok }

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
		Label: r.Label, Formula: r.Formula, Why: r.Why, From: r.From, Mode: r.Mode, CharacterID: r.CharacterID,
		CreatedAt: r.CreatedAt, Mine: r.RollerID != "" && r.RollerID == v.UserID,
	}
	if r.Mode == ModeBlind && !v.Director {
		out.Hidden = true
		return out
	}
	mod, total := r.Modifier, r.Total
	out.Dice, out.Modifier, out.Total, out.Outcome = r.Dice, &mod, &total, r.Outcome
	out.RollerDice = r.RollerDice
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
