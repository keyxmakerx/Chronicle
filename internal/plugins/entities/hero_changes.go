package entities

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// hero_changes.go decides when a player may change their own hero's builder
// picks, and what rings the Directors' bell. They are two different sets and
// stay two functions:
//
//   - What locks (heroLockedChanges): a builder step or what a step's entry
//     buys. The Director sets the times in the campaign settings
//     (campaigns.HeroChangeRule): some days after the hero is made, some days
//     after each new level from a level on, or both, or any time when there is
//     no rule. A Director can also reopen one hero for some days. Outside
//     those times the writers refuse a change to a pick.
//   - What rings (heroBellChanges): the locked set plus every other key a
//     claimed owner may write: short identity texts, the story texts and the
//     lists of names. Those are always stored; they only ring.
//
// Only a change counts: filling a blank finishes the hero rather than
// changing it, so it is always allowed and rings nothing. Only a claimed
// owner acting as a Player rings; the callers judge that, since a Director's
// or Scribe's edit never reaches these functions with a change to report.
//
// A player's burst of changes stays one unread row per Director (the bell
// adapter folds it); this file only says what changed.

// HeroLevelKey is the field that holds a hero's level in the game systems
// that have one. A system without it never opens a level window.
const HeroLevelKey = "level"

// HeroReopenMaxDays bounds how long a Director may reopen one hero.
const HeroReopenMaxDays = campaigns.HeroChangeMaxDays

// HeroChangeState is what the window remembers about one hero: the newest
// level it reached and when, and a Director's reopening.
type HeroChangeState struct {
	EntityID      string
	Level         int
	LevelAt       *time.Time
	ReopenedUntil *time.Time
}

// HeroChangeStore owns the hero_change_state table.
type HeroChangeStore interface {
	// Get returns one hero's state; a hero with none gives a zero state.
	Get(ctx context.Context, entityID string) (HeroChangeState, error)
	// List returns the campaign's states by entity id.
	List(ctx context.Context, campaignID string) (map[string]HeroChangeState, error)
	// RecordLevel notes that the hero reached level at at.
	RecordLevel(ctx context.Context, campaignID, entityID string, level int, at time.Time) error
	// Reopen opens the hero's picks until until, whatever the rule says.
	Reopen(ctx context.Context, campaignID, entityID string, until time.Time, by string) error
}

// HeroChangeRules reads and stores the campaign's rule.
type HeroChangeRules interface {
	HeroChangeRule(ctx context.Context, campaignID string) (*campaigns.HeroChangeRule, error)
	SaveHeroChangeRule(ctx context.Context, campaignID string, rule *campaigns.HeroChangeRule) error
}

// HeroChangeNotifier tells the campaign's Directors that a player changed
// their hero, or raised its level themselves. A failure is logged by the
// caller; the change stands.
type HeroChangeNotifier interface {
	HeroChanged(ctx context.Context, e *Entity, userID string, changes []HeroPickChange) error
	LevelRaised(ctx context.Context, e *Entity, userID string, level int) error
}

// HeroChangeKind says how a changed field is judged and worded.
type HeroChangeKind string

// The kinds of change the bell tells of. Only HeroChangePick is ever locked.
const (
	// HeroChangePick is a builder step or what a step's entry buys.
	HeroChangePick HeroChangeKind = "pick"
	// HeroChangeText is a short identity text such as pronouns.
	HeroChangeText HeroChangeKind = "text"
	// HeroChangeList is a list of names (skills, languages, perks).
	HeroChangeList HeroChangeKind = "list"
	// HeroChangeStory is the player's long text. Its words never leave the
	// page: a change carries no From or To.
	HeroChangeStory HeroChangeKind = "story"
	// HeroChangeNumber is a number the system's play block asks the bell to
	// tell of (Victories, XP), From and To written as numbers.
	HeroChangeNumber HeroChangeKind = "number"
)

// HeroPickChange is one field a player changed, as the Directors read it.
// A zero Kind reads like a pick.
type HeroPickChange struct {
	FieldKey string
	Label    string
	From     string
	To       string
	Kind     HeroChangeKind
	// WasNames and NowNames are, for a list, the names it held before and
	// holds now, in their own spelling.
	WasNames []string
	NowNames []string
}

// locked reports whether the Director's times apply to this change.
func (c HeroPickChange) locked() bool { return c.Kind == HeroChangePick }

// HeroLock is whether a hero's picks may change now, as the builder and the
// Directors' list show it. Until is set when the window closes at a known
// time; Next says what opens it again when it is shut.
type HeroLock struct {
	Open     bool       `json:"open"`
	Until    *time.Time `json:"until,omitempty"`
	Reopened bool       `json:"reopened,omitempty"`
	Next     string     `json:"next,omitempty"`
	// NextLevel is the level that opens a shut window again; 0 when no
	// level does.
	NextLevel int `json:"nextLevel,omitempty"`
	// Ruled is false when the campaign has no rule, so the builder needs no
	// line at all.
	Ruled bool `json:"ruled"`
}

// DaysLeft is the whole days until the window closes, counting a part day;
// 0 when it has no end.
func (l HeroLock) DaysLeft(now time.Time) int {
	if l.Until == nil {
		return 0
	}
	return int(math.Ceil(l.Until.Sub(now).Hours() / 24))
}

// heroChangeWindow decides the lock from the rule, what the window remembers,
// when the hero was made and its level now. A nil rule is any time.
func heroChangeWindow(rule *campaigns.HeroChangeRule, st HeroChangeState, made time.Time, level int, name string, now time.Time) HeroLock {
	if rule == nil {
		return HeroLock{Open: true}
	}
	lock := HeroLock{Ruled: true}
	consider := func(t time.Time) {
		if t.After(now) {
			lock.Open = true
			if lock.Until == nil || t.After(*lock.Until) {
				tt := t
				lock.Until = &tt
			}
		}
	}
	if rule.AfterMakingDays > 0 && !made.IsZero() {
		consider(made.Add(days(rule.AfterMakingDays)))
	}
	from := max(rule.FromLevel, 2)
	if rule.AfterLevel && level >= from && st.Level == level && st.LevelAt != nil {
		consider(st.LevelAt.Add(days(rule.AfterLevelDays)))
	}
	if st.ReopenedUntil != nil && st.ReopenedUntil.After(now) {
		consider(*st.ReopenedUntil)
		lock.Reopened = lock.Until != nil && lock.Until.Equal(*st.ReopenedUntil)
	}
	if lock.Open {
		return lock
	}
	if name == "" {
		name = "your hero"
	}
	if rule.AfterLevel {
		lock.NextLevel = max(level+1, from)
		lock.Next = fmt.Sprintf("Your picks open again for %s when %s reaches level %d. Your Director can open them sooner.",
			dayWord(rule.AfterLevelDays), name, lock.NextLevel)
	} else {
		lock.Next = "Your picks are set. Your Director can open them again."
	}
	return lock
}

func days(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }

// dayWord is "1 day" or "N days".
func dayWord(n int) string {
	if n == 1 {
		return "1 day"
	}
	return strconv.Itoa(n) + " days"
}

// heroLevel reads a hero's level, which a sheet may hold as a number or as
// text; 0 when it has none.
func heroLevel(fields map[string]any) int {
	switch v := fields[HeroLevelKey].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	}
	return 0
}

// heroLockedChanges lists the picks a patch changes that the Director's times
// lock: builder steps and what an entry lets the hero buy, each whose stored
// value is not blank and differs from what the patch writes. Filling a blank
// is not a change, and neither is a multi step gaining the picks it is still
// short of (a conduit's second domain): finishing a hero is not changing it.
// A story or list key is never in this set whatever the plan says, so the
// lock cannot widen to them.
func heroLockedChanges(e *Entity, patch map[string]any, picks builderPicks, labels map[string]string) []HeroPickChange {
	var out []HeroPickChange
	for k, v := range patch {
		if ownerStoryFieldKeys[k] || ownerListFieldKeys[k] {
			continue
		}
		step, isOptions := strings.CutSuffix(k, heroOptionsSuffix)
		if !isOptions {
			step = k
		}
		if _, on := picks[step]; !on {
			continue
		}
		var old any
		if e != nil {
			old = e.FieldsData[k]
		}
		was, now := pickText(old, isOptions), pickText(v, isOptions)
		if was == "" || strings.EqualFold(was, now) {
			continue
		}
		if !isOptions && fillsMulti(picks[step], e, patch, was, now) {
			continue
		}
		label := labels[step]
		if label == "" {
			label = step
		}
		if isOptions {
			label += " choices"
		}
		out = append(out, HeroPickChange{FieldKey: k, Label: label, From: was, To: now, Kind: HeroChangePick})
	}
	slices.SortFunc(out, func(a, b HeroPickChange) int { return strings.Compare(a.FieldKey, b.FieldKey) })
	return out
}

// fillsMulti reports whether now keeps every pick a multi step holds in was,
// in any order, and adds at most what the hero's allowance leaves. Foundry
// sends a hero's subclass items in its own order, so the same set reordered
// is no change either.
func fillsMulti(st builderStep, e *Entity, patch map[string]any, was, now string) bool {
	if !st.multi {
		return false
	}
	before, after := heroMultiNames(was), heroMultiNames(now)
	if len(after) < len(before) || len(after) > st.multiMax(e, patch) {
		return false
	}
	kept := make(map[string]bool, len(after))
	for _, n := range after {
		kept[strings.ToLower(n)] = true
	}
	for _, n := range before {
		if !kept[strings.ToLower(n)] {
			return false
		}
	}
	return true
}

// heroBellChanges lists every change a claimed owner's patch makes that rings
// the bell: the locked set, and each identity, story and list key whose stored
// value is not blank and differs. The bell never names a key the type marks
// GM-only or lacks, so a write that slipped such a key past a door still
// leaves it out. typeFields nil means the type is unknown and only the owner
// keys themselves limit it.
func heroBellChanges(e *Entity, patch map[string]any, picks builderPicks, stepLabel map[string]string, typeFields []FieldDefinition) []HeroPickChange {
	out := heroLockedChanges(e, patch, picks, stepLabel)
	if e == nil {
		return out
	}
	gmOnly, onType := map[string]bool{}, map[string]bool{}
	fieldLabel := map[string]string{}
	field := map[string]*FieldDefinition{}
	for i := range typeFields {
		f := typeFields[i]
		field[f.Key] = &typeFields[i]
		onType[f.Key] = true
		fieldLabel[f.Key] = strings.TrimSpace(f.Label)
		if f.GMOnly {
			gmOnly[f.Key] = true
		}
	}
	for k, v := range patch {
		var kind HeroChangeKind
		switch {
		case ownerListFieldKeys[k]:
			kind = HeroChangeList
		case ownerStoryFieldKeys[k]:
			kind = HeroChangeStory
		case ownerIdentityFieldKeys[k]:
			if _, isStep := picks[k]; isStep {
				continue // judged above, as a pick
			}
			kind = HeroChangeText
		case playBellNumber(field[k]):
			if _, isStep := picks[k]; isStep {
				continue
			}
			kind = HeroChangeNumber
		default:
			continue
		}
		if gmOnly[k] || (typeFields != nil && !onType[k]) {
			continue
		}
		// A story is compared as written, so a change of case rings; every
		// other kind is also dropped when a client merely resends the stored
		// value, since Foundry resends the whole mapped set.
		if kind != HeroChangeStory && sentBackUnchanged(e, k, v) {
			continue
		}
		ch, changed := bellChange(kind, k, e.FieldsData[k], v)
		if !changed {
			continue
		}
		switch kind {
		case HeroChangeList:
			ch.Label = ownerListLabels[k]
		default:
			ch.Label = fieldLabel[k]
			if ch.Label == "" {
				ch.Label = capitalFirst(k)
			}
		}
		out = append(out, ch)
	}
	slices.SortFunc(out, func(a, b HeroPickChange) int { return strings.Compare(a.FieldKey, b.FieldKey) })
	return out
}

// bellChange judges one non-pick key. A value it cannot read is no change,
// since reporting "everything removed" for a malformed write would be wrong.
func bellChange(kind HeroChangeKind, k string, old, v any) (HeroPickChange, bool) {
	ch := HeroPickChange{FieldKey: k, Kind: kind}
	switch kind {
	case HeroChangeNumber:
		// Unset reads as 0, so a first award says "0 → 1". A value that is
		// not a number is no change; the door refuses it anyway.
		was, okWas := playNumber(old)
		if old != nil && !okWas {
			return ch, false
		}
		now, okNow := playNumber(v)
		if v != nil && !okNow {
			return ch, false
		}
		if was == now {
			return ch, false
		}
		ch.From, ch.To = playNumberText(was), playNumberText(now)
	case HeroChangeList:
		was, okWas := listNames(old)
		now, okNow := listNames(v)
		if !okWas || !okNow || len(was) == 0 || sameNames(was, now) {
			return ch, false
		}
		ch.WasNames, ch.NowNames = was, now
	default:
		if v != nil {
			if _, isText := v.(string); !isText {
				return ch, false
			}
		}
		oldText, _ := old.(string)
		was, now := strings.TrimSpace(oldText), ""
		if s, ok := v.(string); ok {
			now = strings.TrimSpace(s)
		}
		same := was == now
		if kind == HeroChangeText {
			same = strings.EqualFold(was, now)
		}
		if was == "" || same {
			return ch, false
		}
		if kind == HeroChangeText {
			ch.From, ch.To = was, now
		}
	}
	return ch, true
}

// listNames reads a list as the names it holds, trimmed and distinct, in the
// order given: a JSON array as text, or a decoded array, whose entries are a
// name or an object with a name. Blank is no names. ok is false for text that
// is not a list, which has no names to compare.
func listNames(v any) (names []string, ok bool) {
	var raw []any
	switch t := v.(type) {
	case nil:
		return nil, true
	case string:
		if strings.TrimSpace(t) == "" {
			return nil, true
		}
		if err := json.Unmarshal([]byte(t), &raw); err != nil {
			return nil, false
		}
	case []any:
		raw = t
	case []string:
		for _, n := range t {
			raw = append(raw, n)
		}
	default:
		return nil, false
	}
	seen := map[string]bool{}
	for _, it := range raw {
		var name string
		switch n := it.(type) {
		case string:
			name = n
		case map[string]any:
			name, _ = n["name"].(string)
		}
		name = strings.TrimSpace(name)
		key := strings.ToLower(name)
		if name == "" || seen[key] {
			continue
		}
		seen[key] = true
		names = append(names, name)
	}
	return names, true
}

// sameNames reports whether two name lists hold the same names, ignoring
// order and case.
func sameNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, n := range a {
		set[strings.ToLower(n)] = true
	}
	for _, n := range b {
		if !set[strings.ToLower(n)] {
			return false
		}
	}
	return true
}

func capitalFirst(k string) string {
	if k == "" {
		return k
	}
	r, size := utf8.DecodeRuneInString(k)
	return string(unicode.ToUpper(r)) + k[size:]
}

// pickText is a pick as compared and shown: trimmed text, or for what an
// entry buys, its names sorted and joined.
func pickText(v any, list bool) string {
	s, _ := v.(string)
	s = strings.TrimSpace(s)
	if !list || s == "" {
		return s
	}
	var names []string
	if err := json.Unmarshal([]byte(s), &names); err != nil {
		return s
	}
	slices.SortFunc(names, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return strings.Join(names, ", ")
}

// stepLabels maps each step's field key to the label the builder shows.
func stepLabels(plan *HeroPlan) map[string]string {
	out := map[string]string{}
	if plan == nil {
		return out
	}
	for _, st := range plan.Steps {
		out[st.FieldKey] = st.Label
	}
	return out
}

// HeroChangeService ties the rule, the remembered state and the bell
// together. Every part is optional: with no rules store every hero is open,
// and with no notifier nothing rings.
type HeroChangeService struct {
	rules  HeroChangeRules
	store  HeroChangeStore
	notify HeroChangeNotifier
	now    func() time.Time
}

// NewHeroChangeService builds the service.
func NewHeroChangeService(rules HeroChangeRules, store HeroChangeStore, notify HeroChangeNotifier) *HeroChangeService {
	return &HeroChangeService{rules: rules, store: store, notify: notify, now: time.Now}
}

// Rule returns the campaign's rule; nil is any time.
func (s *HeroChangeService) Rule(ctx context.Context, campaignID string) (*campaigns.HeroChangeRule, error) {
	if s == nil || s.rules == nil {
		return nil, nil
	}
	return s.rules.HeroChangeRule(ctx, campaignID)
}

// SaveRule stores the campaign's rule; nil is any time.
func (s *HeroChangeService) SaveRule(ctx context.Context, campaignID string, rule *campaigns.HeroChangeRule) error {
	if s == nil || s.rules == nil {
		return apperror.NewInternal(errors.New("hero change rules not wired"))
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	return s.rules.SaveHeroChangeRule(ctx, campaignID, rule)
}

// Lock is one hero's lock now.
func (s *HeroChangeService) Lock(ctx context.Context, e *Entity) (HeroLock, error) {
	rule, err := s.Rule(ctx, e.CampaignID)
	if err != nil || rule == nil {
		return HeroLock{Open: true}, err
	}
	var st HeroChangeState
	if s.store != nil {
		if st, err = s.store.Get(ctx, e.ID); err != nil {
			return HeroLock{}, err
		}
	}
	return heroChangeWindow(rule, st, e.CreatedAt, heroLevel(e.FieldsData), e.Name, s.now()), nil
}

// Locks is the lock of each of a campaign's heroes, read in one pass.
func (s *HeroChangeService) Locks(ctx context.Context, campaignID string, heroes []Entity) (map[string]HeroLock, error) {
	out := make(map[string]HeroLock, len(heroes))
	rule, err := s.Rule(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	states := map[string]HeroChangeState{}
	if rule != nil && s.store != nil {
		if states, err = s.store.List(ctx, campaignID); err != nil {
			return nil, err
		}
	}
	now := s.now()
	for i := range heroes {
		e := &heroes[i]
		out[e.ID] = heroChangeWindow(rule, states[e.ID], e.CreatedAt, heroLevel(e.FieldsData), e.Name, now)
	}
	return out, nil
}

// CheckPicks returns the changes a claimed owner's patch makes for the bell,
// refused when it changes a pick and the hero's picks are shut now. A list,
// story or identity change is never refused: it is stored and only rings.
func (s *HeroChangeService) CheckPicks(ctx context.Context, e *Entity, patch map[string]any, picks builderPicks, plan *HeroPlan, typeFields []FieldDefinition) ([]HeroPickChange, error) {
	changes := heroBellChanges(e, patch, picks, stepLabels(plan), typeFields)
	if s == nil || !anyLocked(changes) {
		return changes, nil
	}
	lock, err := s.Lock(ctx, e)
	if err != nil {
		return nil, err
	}
	if !lock.Open {
		return nil, apperror.NewForbidden("Your hero's picks are locked right now. " + lock.Next)
	}
	return changes, nil
}

func anyLocked(changes []HeroPickChange) bool {
	return slices.ContainsFunc(changes, HeroPickChange.locked)
}

// HeldPicks is CheckPicks key by key, for writers that keep what passes and
// report the rest (Foundry, a player's AI): when the hero's picks are shut,
// each changed pick is held with the reason; the changes that are not picks
// are stored, so they are still returned for the bell. The caller passes only
// the keys it would still store, so a key held for another reason is not
// counted twice.
func (s *HeroChangeService) HeldPicks(ctx context.Context, e *Entity, patch map[string]any, plan *HeroPlan, typeFields []FieldDefinition) (map[string]string, []HeroPickChange, error) {
	changes := heroBellChanges(e, patch, builderPicksFrom(plan), stepLabels(plan), typeFields)
	if s == nil || !anyLocked(changes) {
		return nil, changes, nil
	}
	lock, err := s.Lock(ctx, e)
	if err != nil {
		return nil, nil, err
	}
	if lock.Open {
		return nil, changes, nil
	}
	held := map[string]string{}
	var stored []HeroPickChange
	for _, ch := range changes {
		if ch.locked() {
			held[ch.FieldKey] = "Your hero's picks are locked right now. " + lock.Next
			continue
		}
		stored = append(stored, ch)
	}
	return held, stored, nil
}

// Changed rings the Directors' bell for a change a player saved.
func (s *HeroChangeService) Changed(ctx context.Context, e *Entity, userID string, changes []HeroPickChange) {
	if s == nil || s.notify == nil || len(changes) == 0 {
		return
	}
	if err := s.notify.HeroChanged(ctx, e, userID, changes); err != nil {
		slog.Warn("hero change bell failed", slog.String("entity_id", e.ID), slog.Any("error", err))
	}
}

// HeroLevelRaised is the level patch gives the hero when that is higher than
// the stored one; 0 when patch leaves the level alone, keeps it or lowers it.
func HeroLevelRaised(e *Entity, patch map[string]any) int {
	if e == nil {
		return 0
	}
	if _, ok := patch[HeroLevelKey]; !ok {
		return 0
	}
	if now := heroLevel(patch); now > heroLevel(e.FieldsData) {
		return now
	}
	return 0
}

// LevelRaised rings the Directors' bell for a level a player gave their own
// hero. The level-up still counts, since tables that level outside Chronicle
// rely on it; the bell is so it is seen. 0 rings nothing.
func (s *HeroChangeService) LevelRaised(ctx context.Context, e *Entity, userID string, level int) {
	if s == nil || s.notify == nil || e == nil || level <= 0 || userID == "" {
		return
	}
	if err := s.notify.LevelRaised(ctx, e, userID, level); err != nil {
		slog.Warn("hero level bell failed", slog.String("entity_id", e.ID), slog.Any("error", err))
	}
}

// Reopen opens one hero's picks for some days from now.
func (s *HeroChangeService) Reopen(ctx context.Context, e *Entity, n int, by string) error {
	if s == nil || s.store == nil {
		return apperror.NewInternal(errors.New("hero change store not wired"))
	}
	if n < 1 || n > HeroReopenMaxDays {
		return apperror.NewBadRequest(fmt.Sprintf("reopen for 1 to %d days", HeroReopenMaxDays))
	}
	return s.store.Reopen(ctx, e.CampaignID, e.ID, s.now().Add(days(n)), by)
}

// FieldsChanged notes a claimed hero's new level, so a level window opens
// from the moment it was reached. It is a field observer and never blocks
// the write.
func (s *HeroChangeService) FieldsChanged(ctx context.Context, e *Entity, oldFields, newFields map[string]any) error {
	if s == nil || s.store == nil || e == nil || e.OwnerUserID == nil {
		return nil
	}
	was, now := heroLevel(oldFields), heroLevel(newFields)
	if now <= was || now < 2 {
		return nil
	}
	return s.store.RecordLevel(ctx, e.CampaignID, e.ID, now, s.now())
}

// heroChangeRepository is the MariaDB HeroChangeStore.
type heroChangeRepository struct {
	db *sql.DB
}

// NewHeroChangeRepository creates the MariaDB store for hero change state.
func NewHeroChangeRepository(db *sql.DB) HeroChangeStore {
	return &heroChangeRepository{db: db}
}

func (r *heroChangeRepository) Get(ctx context.Context, entityID string) (HeroChangeState, error) {
	st := HeroChangeState{EntityID: entityID}
	var levelAt, until sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT level, level_at, reopened_until FROM hero_change_state WHERE entity_id = ?`, entityID).
		Scan(&st.Level, &levelAt, &until)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("reading hero change state: %w", err)
	}
	st.LevelAt, st.ReopenedUntil = nullTime(levelAt), nullTime(until)
	return st, nil
}

func (r *heroChangeRepository) List(ctx context.Context, campaignID string) (map[string]HeroChangeState, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT entity_id, level, level_at, reopened_until FROM hero_change_state WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing hero change state: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]HeroChangeState{}
	for rows.Next() {
		var st HeroChangeState
		var levelAt, until sql.NullTime
		if err := rows.Scan(&st.EntityID, &st.Level, &levelAt, &until); err != nil {
			return nil, fmt.Errorf("scanning hero change state: %w", err)
		}
		st.LevelAt, st.ReopenedUntil = nullTime(levelAt), nullTime(until)
		out[st.EntityID] = st
	}
	return out, rows.Err()
}

func (r *heroChangeRepository) RecordLevel(ctx context.Context, campaignID, entityID string, level int, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO hero_change_state (entity_id, campaign_id, level, level_at) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE level = VALUES(level), level_at = VALUES(level_at)`,
		entityID, campaignID, level, at.UTC())
	if err != nil {
		return fmt.Errorf("recording hero level: %w", err)
	}
	return nil
}

func (r *heroChangeRepository) Reopen(ctx context.Context, campaignID, entityID string, until time.Time, by string) error {
	var byArg any
	if by != "" {
		byArg = by
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO hero_change_state (entity_id, campaign_id, reopened_until, reopened_by) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE reopened_until = VALUES(reopened_until), reopened_by = VALUES(reopened_by)`,
		entityID, campaignID, until.UTC(), byArg)
	if err != nil {
		return fmt.Errorf("reopening hero picks: %w", err)
	}
	return nil
}

func nullTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
