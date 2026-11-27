package systems

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	// maxMarksPerCall bounds one batch so "mark the rest" can cover a long
	// list without one request holding a transaction open indefinitely.
	maxMarksPerCall = 200
	maxRuleReason   = 300
	maxRequestNote  = 300
	// suggestedShownFor is how long a Director's "choose another" note stays
	// on the player's list before it stops being shown.
	suggestedShownFor = 30 * 24 * time.Hour
)

// ChoiceHero is a character page a player plays, as the rules read it: the
// fields are the page's text values by key.
type ChoiceHero struct {
	ID          string
	Name        string
	OwnerUserID string
	Fields      map[string]string
}

// ChoiceMember is a campaign member, for naming the person a rule is about.
type ChoiceMember struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
}

// ChoiceHeroes is what the rules need from the entities and campaigns
// plugins, implemented at the composition root so this package imports
// neither.
type ChoiceHeroes interface {
	// Heroes lists the campaign's character pages that a player plays.
	Heroes(ctx context.Context, campaignID string) ([]ChoiceHero, error)
	// Members lists the campaign's members.
	Members(ctx context.Context, campaignID string) ([]ChoiceMember, error)
	// SetField writes one text field of a hero page the player owns, as a
	// Director would, and reports whether it did (false when the page is gone
	// or no longer theirs).
	SetField(ctx context.Context, campaignID, entityID, ownerUserID, fieldKey, value string) (bool, error)
}

// ChoiceMark is one mark in a request: the state of an entry (or, with Key
// "*", of a whole source) for everyone or for one player.
type ChoiceMark struct {
	// Source is "package" or "campaign".
	Source string `json:"source"`
	// Key is an entry's key (Choice.Key) or "*" for the whole source.
	Key string `json:"key"`
	// Target is "" for everyone, else a player's account id.
	Target string `json:"target"`
	// State is open, ask_first, not_offered, required, or clear to remove
	// the mark. Open for everyone is the same as clear.
	State string `json:"state"`
	// Reason is shown to the player it applies to. Absent keeps the stored
	// reason; present (even "") replaces it.
	Reason *string `json:"reason"`
}

// ChoiceMarksInput is a batch of marks on one pick list.
type ChoiceMarksInput struct {
	FieldKey string       `json:"fieldKey"`
	Marks    []ChoiceMark `json:"marks"`
}

// BrokenHero is a hero that holds an entry a rule keeps off their player's
// list.
type BrokenHero struct {
	HeroID     string `json:"heroId"`
	HeroName   string `json:"heroName"`
	PlayerID   string `json:"playerId"`
	PlayerName string `json:"playerName"`
	HeroBreak
}

// HeroBreak is one rule a hero already breaks. Kind is "not_offered" (the
// entry is kept off the list) or "off_list" (a required short list does not
// include it).
type HeroBreak struct {
	FieldKey string `json:"fieldKey"`
	Value    string `json:"value"`
	Kind     string `json:"kind"`
	Reason   string `json:"reason,omitempty"`
}

// Kinds of HeroBreak.
const (
	BreakNotOffered = "not_offered"
	BreakOffList    = "off_list"
)

// MarksCheck is the dry run of a batch of marks.
type MarksCheck struct {
	// Count is how many heroes would newly break.
	Count  int          `json:"count"`
	Heroes []BrokenHero `json:"heroes"`
}

// ChoiceRequestInput is a player's pick of an ask-first entry.
type ChoiceRequestInput struct {
	CampaignID string
	UserID     string
	// EntityID and EntityName name the hero the pick is for.
	EntityID   string
	EntityName string
	FieldKey   string
	EntryName  string
}

// ChoiceRequestOutcome is what a write answers with instead of storing the
// pick: the request now waits for a Director.
type ChoiceRequestOutcome struct {
	ID        int64  `json:"id"`
	FieldKey  string `json:"fieldKey"`
	EntryName string `json:"entryName"`
	Status    string `json:"status"`
}

// WaitingChoiceRequest is a waiting request as a Director reads it.
type WaitingChoiceRequest struct {
	ChoiceRequest
	FieldLabel string `json:"fieldLabel"`
	PlayerName string `json:"playerName"`
}

// AllowOutcome says what allowing a request did.
type AllowOutcome struct {
	// HeroUpdated is false when the hero page is gone or no longer the
	// player's; the player can then pick the entry themselves.
	HeroUpdated bool `json:"heroUpdated"`
}

// HeroWaitingPick is one blank on a hero that waits for a Director.
type HeroWaitingPick struct {
	ID        int64  `json:"id"`
	FieldKey  string `json:"fieldKey"`
	EntryName string `json:"entryName"`
}

// ChoiceRuleService owns the hero-choice rules: reading them for a viewer,
// the Directors' screens, and the picks that wait on a Director. It never
// sees a request; callers say who is asking through EntryActor.
type ChoiceRuleService interface {
	ChoiceLimiter

	// Rules lists a pick list's stored rules (every list when fieldKey is
	// ""). Directors only.
	Rules(ctx context.Context, campaignID, fieldKey string, actor EntryActor) ([]ChoiceRule, error)
	// SetMarks writes a batch of marks and returns the list's rules after.
	// Directors only.
	SetMarks(ctx context.Context, campaignID string, actor EntryActor, in ChoiceMarksInput) ([]ChoiceRule, error)
	// CheckMarks says which heroes the batch would newly break, writing
	// nothing. Directors only.
	CheckMarks(ctx context.Context, campaignID string, actor EntryActor, in ChoiceMarksInput) (*MarksCheck, error)
	// DirectorEntries is a pick list's every entry with its marks, for the
	// Directors' screen; asUser shows it as that player gets it.
	DirectorEntries(ctx context.Context, campaignID string, actor EntryActor, fieldKey, asUser string) (*DirectorEntryList, error)
	// RuleCountForEntry counts the rules naming a campaign entry, for the
	// delete warning. Directors only.
	RuleCountForEntry(ctx context.Context, campaignID string, actor EntryActor, entryID int64) (int, error)
	// ForgetEntry removes the rules naming a deleted campaign entry.
	ForgetEntry(ctx context.Context, campaignID, systemID, fieldKey string, entryID int64) (int, error)

	// HeroBreaks lists the rules a hero already breaks. Never changes the hero.
	HeroBreaks(ctx context.Context, campaignID string, hero ChoiceHero) ([]HeroBreak, error)

	// HeroWaiting lists the picks one player has waiting on a Director for
	// one hero, so the sheet can show each blank as waiting.
	HeroWaiting(ctx context.Context, campaignID, userID, entityID string) ([]HeroWaitingPick, error)
	// RequestChoice records a player's pick of an ask-first entry.
	RequestChoice(ctx context.Context, in ChoiceRequestInput) (*ChoiceRequestOutcome, error)
	// WaitingRequests lists the picks waiting on a Director. Directors only.
	WaitingRequests(ctx context.Context, campaignID string, actor EntryActor) ([]WaitingChoiceRequest, error)
	// AllowRequest gives the player the entry (a per-player open mark) and
	// sets the hero's field as a Director would. Directors only.
	AllowRequest(ctx context.Context, campaignID string, actor EntryActor, id int64) (*AllowOutcome, error)
	// SuggestAnother closes a request with a note for the player; the step
	// stays empty. Directors only.
	SuggestAnother(ctx context.Context, campaignID string, actor EntryActor, id int64, message string) error
}

type choiceRuleService struct {
	repo     ChoiceRuleRepository
	entries  SystemEntryRepository
	choices  CharacterChoiceService
	manifest EntryManifestResolver
	heroes   ChoiceHeroes
	now      func() time.Time
}

// ChoiceRuleOption adds an optional dependency to the service.
type ChoiceRuleOption func(*choiceRuleService)

// WithChoiceHeroes lets the rules read heroes and members and set a hero's
// field. Without it the dry run finds no heroes, names are blank and Allow
// records the player's open mark without touching a hero.
func WithChoiceHeroes(h ChoiceHeroes) ChoiceRuleOption {
	return func(s *choiceRuleService) { s.heroes = h }
}

// NewChoiceRuleService wires the service. choices is read with the rules
// switched off (WithChoiceUnlimited) to see every entry.
func NewChoiceRuleService(repo ChoiceRuleRepository, entries SystemEntryRepository, choices CharacterChoiceService, manifest EntryManifestResolver, opts ...ChoiceRuleOption) ChoiceRuleService {
	s := &choiceRuleService{repo: repo, entries: entries, choices: choices, manifest: manifest, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Limit implements ChoiceLimiter. A Director or a context that asked for the
// list as it is passes untouched; everyone else gets the list the rules leave
// them. A failing read is an error rather than an unfiltered list: a limit
// that quietly stops holding is worse than a list that fails to load.
func (s *choiceRuleService) Limit(ctx context.Context, campaignID string, l *ChoiceList) error {
	// A visitor is never exempt and has no account, whatever else the context
	// says (choice_visitor.go).
	visitor := choiceVisitor(ctx)
	if l == nil || l.SystemID == "" || (!visitor && (ChoiceViewerIsDirector(ctx) || choiceUnlimited(ctx))) {
		return nil
	}
	viewer := ChoiceFor(ctx)
	if visitor {
		viewer = ""
	}
	rows, err := s.repo.ListForViewer(ctx, campaignID, l.SystemID, l.FieldKey, viewer)
	if err != nil {
		return err
	}
	res := applyChoiceRules(l.Choices, rows, viewer)
	l.Choices, l.Excluded, l.Rules = res.Kept, res.Excluded, res.fieldRules()
	if viewer == "" {
		return nil
	}
	reqs, err := s.repo.UserRequests(ctx, campaignID, l.SystemID, viewer, l.FieldKey, s.now().Add(-suggestedShownFor))
	if err != nil {
		return err
	}
	var waiting, suggested *RequestView
	for _, q := range reqs {
		v := &RequestView{ID: q.ID, EntryName: q.Name, EntityID: q.EntityID, Message: q.Message, CreatedAt: q.CreatedAt}
		if q.Status == RequestWaiting && waiting == nil {
			waiting = v
		}
		if q.Status == RequestSuggested && suggested == nil {
			suggested = v
		}
	}
	if waiting != nil {
		suggested = nil
	}
	if waiting != nil || suggested != nil {
		if l.Rules == nil {
			l.Rules = &FieldRules{}
		}
		l.Rules.Waiting, l.Rules.Suggested = waiting, suggested
	}
	return nil
}

func (s *choiceRuleService) system(ctx context.Context, campaignID string) (*SystemManifest, error) {
	var m *SystemManifest
	if s.manifest != nil {
		m = s.manifest(ctx, campaignID)
	}
	if m == nil {
		return nil, apperror.NewValidation("This campaign has no game system, so there is nothing to limit.")
	}
	if len(m.ID) > 64 {
		return nil, apperror.NewValidation("This game system's id is too long to hold hero rules.")
	}
	return m, nil
}

func (s *choiceRuleService) checkField(m *SystemManifest, fieldKey string) error {
	if !ValidChoiceFieldKey(fieldKey) || !isCharacterStringField(m, fieldKey) {
		return apperror.NewValidation("That is not a pick list of this game system's character sheet.")
	}
	return nil
}

// directorList is a pick list as a Director sees it, rules aside: every entry
// of the package and the campaign, hidden ones included.
func (s *choiceRuleService) directorList(ctx context.Context, campaignID, fieldKey string) (*ChoiceList, error) {
	return s.choices.CharacterChoiceList(WithChoiceUnlimited(WithChoiceViewer(ctx, true)), campaignID, fieldKey)
}

// playerList is a pick list as one player reads it, rules aside: only the
// entries that player can see. An empty userID is a player with no account,
// who sees what everyone sees.
func (s *choiceRuleService) playerList(ctx context.Context, campaignID, fieldKey, userID string) (*ChoiceList, error) {
	c := WithChoiceViewerUser(WithChoiceViewer(WithChoiceUnlimited(ctx), false), userID)
	return s.choices.CharacterChoiceList(c, campaignID, fieldKey)
}

func (s *choiceRuleService) members(ctx context.Context, campaignID string) []ChoiceMember {
	if s.heroes == nil {
		return nil
	}
	ms, err := s.heroes.Members(ctx, campaignID)
	if err != nil {
		slog.Warn("choice rules: listing members failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	return ms
}

func memberNames(ms []ChoiceMember) map[string]string {
	out := make(map[string]string, len(ms))
	for _, m := range ms {
		out[m.UserID] = m.Name
	}
	return out
}

func (s *choiceRuleService) Rules(ctx context.Context, campaignID, fieldKey string, actor EntryActor) ([]ChoiceRule, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	if fieldKey != "" && !ValidChoiceFieldKey(fieldKey) {
		return nil, apperror.NewValidation("invalid field key")
	}
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil {
		return []ChoiceRule{}, nil
	}
	return s.repo.List(ctx, campaignID, m.ID, fieldKey)
}

func (s *choiceRuleService) manifestOrNil(ctx context.Context, campaignID string) *SystemManifest {
	if s.manifest == nil {
		return nil
	}
	return s.manifest(ctx, campaignID)
}

// plannedMarks is a validated batch: what to write and delete, and the
// field's rules as they would stand after.
type plannedMarks struct {
	systemID string
	fieldKey string
	upserts  []ChoiceRule
	deletes  []ChoiceRule
	before   []ChoiceRule
	after    []ChoiceRule
}

// planMarks validates a batch against the Directors' view of the list and
// works out the rows it changes. SetMarks and CheckMarks both use it, so the
// dry run can never accept what the save would refuse.
func (s *choiceRuleService) planMarks(ctx context.Context, campaignID string, actor EntryActor, in ChoiceMarksInput) (*plannedMarks, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	m, err := s.system(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if err := s.checkField(m, in.FieldKey); err != nil {
		return nil, err
	}
	if len(in.Marks) == 0 || len(in.Marks) > maxMarksPerCall {
		return nil, apperror.NewValidation(fmt.Sprintf("Send between 1 and %d marks at a time.", maxMarksPerCall))
	}
	list, err := s.directorList(ctx, campaignID, in.FieldKey)
	if err != nil {
		return nil, err
	}
	entries := map[ruleRef]Choice{}
	for _, c := range list.Choices {
		if c.Key != "" {
			entries[ruleRef{source: c.Source, key: c.Key}] = c
		}
	}
	// A standard entry a rewrite replaced is off the list, but its rows still
	// hold for the rewrite, so a mark followed there must find an entry.
	for _, c := range list.Choices {
		std := ruleRef{source: ChoiceSourcePackage, key: c.StandsFor}
		if _, listed := entries[std]; c.Origin == OriginRewritten && c.StandsFor != "" && !listed {
			entries[std] = c
		}
	}
	existing, err := s.repo.List(ctx, campaignID, m.ID, in.FieldKey)
	if err != nil {
		return nil, err
	}
	byRef := map[ruleRef]ChoiceRule{}
	for _, r := range existing {
		byRef[ruleRef{r.Source, r.Key, r.Target}] = r
	}
	members := s.members(ctx, campaignID)
	known := memberNames(members)

	plan := &plannedMarks{systemID: m.ID, fieldKey: in.FieldKey, before: existing}
	final := map[ruleRef]ChoiceRule{}
	for k, v := range byRef {
		final[k] = v
	}
	// A later mark for the same entry and target replaces an earlier one in
	// the batch, so each row is written once.
	order := []ruleRef{}
	changes := map[ruleRef]*ChoiceRule{}
	for _, mk := range in.Marks {
		ref, state, err := normalizeMark(mk)
		if err != nil {
			return nil, err
		}
		ref = followRewrite(ref, entries, byRef)
		if ref.target != "" && len(members) > 0 {
			if _, ok := known[ref.target]; !ok {
				return nil, apperror.NewValidation("That person isn't a member of this campaign.")
			}
		}
		if _, seen := changes[ref]; !seen {
			order = append(order, ref)
		}
		if state == "clear" {
			changes[ref] = nil
			delete(final, ref)
			continue
		}
		name := ""
		if ref.key != RuleGroupKey {
			c, ok := entries[ruleRef{source: ref.source, key: ref.key}]
			if !ok {
				return nil, apperror.NewValidation("That entry isn't on this list.")
			}
			name = c.Name
			if had, ok := byRef[ref]; ok && had.Name != "" {
				name = had.Name
			}
			if state == RuleRequired {
				if err := s.checkRequirable(ctx, campaignID, in.FieldKey, ref, c); err != nil {
					return nil, err
				}
			}
		}
		reason := byRef[ref].Reason
		if mk.Reason != nil {
			reason = strings.TrimSpace(*mk.Reason)
			if utf8.RuneCountInString(reason) > maxRuleReason {
				return nil, apperror.NewValidation(fmt.Sprintf("A reason can be at most %d characters.", maxRuleReason))
			}
		}
		row := ChoiceRule{
			CampaignID: campaignID, SystemID: m.ID, FieldKey: in.FieldKey,
			Source: ref.source, Key: ref.key, Name: name, Target: ref.target,
			State: state, Reason: reason, CreatedBy: actor.UserID,
		}
		changes[ref] = &row
		final[ref] = row
	}
	for _, ref := range order {
		if row := changes[ref]; row != nil {
			plan.upserts = append(plan.upserts, *row)
		} else if _, had := byRef[ref]; had {
			plan.deletes = append(plan.deletes, byRef[ref])
		}
	}
	for _, r := range final {
		plan.after = append(plan.after, r)
	}
	sort.Slice(plan.after, func(i, j int) bool {
		a, b := plan.after[i], plan.after[j]
		return a.Source+"|"+a.Key+"|"+a.Target < b.Source+"|"+b.Key+"|"+b.Target
	})
	return plan, nil
}

// followRewrite points a mark on a rewrite at the standard entry's row when
// the rewrite has no row of its own for that target and the standard entry
// does: the Directors' view shows that row on the rewrite (the evaluator
// lets it hold there), so clearing or editing it there must reach it.
func followRewrite(ref ruleRef, entries map[ruleRef]Choice, byRef map[ruleRef]ChoiceRule) ruleRef {
	if ref.key == RuleGroupKey {
		return ref
	}
	if _, own := byRef[ref]; own {
		return ref
	}
	c, ok := entries[ruleRef{source: ref.source, key: ref.key}]
	if !ok || c.Origin != OriginRewritten || c.StandsFor == "" {
		return ref
	}
	std := ruleRef{source: ChoiceSourcePackage, key: c.StandsFor, target: ref.target}
	if _, has := byRef[std]; has {
		return std
	}
	return ref
}

// normalizeMark trims and checks the shape of one mark. Open for everyone is
// the default, so it clears.
func normalizeMark(mk ChoiceMark) (ruleRef, string, error) {
	ref := ruleRef{source: strings.TrimSpace(mk.Source), key: strings.TrimSpace(mk.Key), target: strings.TrimSpace(mk.Target)}
	if ref.source != ChoiceSourcePackage && ref.source != ChoiceSourceCampaign {
		return ref, "", apperror.NewValidation("A mark names an entry of the game or of this campaign.")
	}
	if ref.key == "" || len(ref.key) > 100 {
		return ref, "", apperror.NewValidation("A mark names an entry.")
	}
	if len(ref.target) > 36 {
		return ref, "", apperror.NewValidation("A mark is for everyone or for one player.")
	}
	state := strings.ToLower(strings.TrimSpace(mk.State))
	switch state {
	case RuleOpen:
		if ref.target == "" {
			state = "clear"
		}
	case RuleAskFirst, RuleNotOffered, RuleRequired, "clear":
	default:
		return ref, "", apperror.NewValidation("A mark is open, ask first, not offered or required.")
	}
	return ref, state, nil
}

// checkRequirable refuses a required mark that would show an entry to
// someone who cannot read it: a rule never widens what a player sees.
func (s *choiceRuleService) checkRequirable(ctx context.Context, campaignID, fieldKey string, ref ruleRef, c Choice) error {
	if ref.target != "" {
		pl, err := s.playerList(ctx, campaignID, fieldKey, ref.target)
		if err != nil {
			return err
		}
		for _, pc := range pl.Choices {
			if (pc.Source == ref.source && pc.Key == ref.key) || (pc.Source == c.Source && pc.Key == c.Key) {
				return nil
			}
		}
		return apperror.NewValidation(fmt.Sprintf("That player can't see %s yet, so it can't be required of them. Make it visible to them first.", c.Name))
	}
	if c.Source == ChoiceSourceCampaign && (c.Visibility != EntryVisibilityEveryone || c.Status != "") {
		return apperror.NewValidation(fmt.Sprintf("Players can't see %s yet, so it can't be required of them. Make it visible to everyone first.", c.Name))
	}
	return nil
}

func (s *choiceRuleService) SetMarks(ctx context.Context, campaignID string, actor EntryActor, in ChoiceMarksInput) ([]ChoiceRule, error) {
	plan, err := s.planMarks(ctx, campaignID, actor, in)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Apply(ctx, plan.upserts, plan.deletes); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, campaignID, plan.systemID, plan.fieldKey)
}

func (s *choiceRuleService) CheckMarks(ctx context.Context, campaignID string, actor EntryActor, in ChoiceMarksInput) (*MarksCheck, error) {
	plan, err := s.planMarks(ctx, campaignID, actor, in)
	if err != nil {
		return nil, err
	}
	out := &MarksCheck{Heroes: []BrokenHero{}}
	if s.heroes == nil {
		return out, nil
	}
	heroes, err := s.heroes.Heroes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	names := memberNames(s.members(ctx, campaignID))
	lists := map[string][]Choice{}
	seen := map[string]bool{}
	for _, h := range heroes {
		value := strings.TrimSpace(h.Fields[plan.fieldKey])
		if h.OwnerUserID == "" || value == "" {
			continue
		}
		list, ok := lists[h.OwnerUserID]
		if !ok {
			pl, err := s.playerList(ctx, campaignID, plan.fieldKey, h.OwnerUserID)
			if err != nil {
				return nil, err
			}
			list = pl.Choices
			lists[h.OwnerUserID] = list
		}
		if breakFor(list, plan.before, h.OwnerUserID, plan.fieldKey, value) != nil {
			continue
		}
		if b := breakFor(list, plan.after, h.OwnerUserID, plan.fieldKey, value); b != nil {
			out.Heroes = append(out.Heroes, BrokenHero{
				HeroID: h.ID, HeroName: h.Name, PlayerID: h.OwnerUserID, PlayerName: names[h.OwnerUserID], HeroBreak: *b,
			})
			if !seen[h.ID] {
				seen[h.ID] = true
				out.Count++
			}
		}
	}
	return out, nil
}

// breakFor says whether a hero's value for a field breaks the rules for its
// player, given the list that player can read. A value that names a kept
// entry is fine (an ask-first entry the hero holds counts as allowed). One
// that names a kept-off entry breaks; so does any other value when a required
// short list exists, since it is not on it. An empty value holds nothing.
func breakFor(list []Choice, rows []ChoiceRule, owner, fieldKey, value string) *HeroBreak {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	res := applyChoiceRules(list, rows, owner)
	if !res.Limited || choiceByName(res.Kept, value) != nil {
		return nil
	}
	if c := choiceByName(res.Excluded, value); c != nil {
		if r := indexRules(rows, owner).resolve(*c, owner); r.State == RuleNotOffered {
			return &HeroBreak{FieldKey: fieldKey, Value: value, Kind: BreakNotOffered, Reason: ruleReason(r.Rule)}
		}
		return &HeroBreak{FieldKey: fieldKey, Value: value, Kind: BreakOffList}
	}
	if res.Required {
		return &HeroBreak{FieldKey: fieldKey, Value: value, Kind: BreakOffList}
	}
	return nil
}

func (s *choiceRuleService) HeroBreaks(ctx context.Context, campaignID string, hero ChoiceHero) ([]HeroBreak, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil || hero.OwnerUserID == "" {
		return nil, nil
	}
	rows, err := s.repo.ListForViewer(ctx, campaignID, m.ID, "", hero.OwnerUserID)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	byField := map[string][]ChoiceRule{}
	for _, r := range rows {
		byField[r.FieldKey] = append(byField[r.FieldKey], r)
	}
	fields := make([]string, 0, len(byField))
	for k := range byField {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	var out []HeroBreak
	for _, f := range fields {
		value := strings.TrimSpace(hero.Fields[f])
		if value == "" {
			continue
		}
		pl, err := s.playerList(ctx, campaignID, f, hero.OwnerUserID)
		if err != nil {
			return nil, err
		}
		if b := breakFor(pl.Choices, byField[f], hero.OwnerUserID, f, value); b != nil {
			out = append(out, *b)
		}
	}
	return out, nil
}

func (s *choiceRuleService) RuleCountForEntry(ctx context.Context, campaignID string, actor EntryActor, entryID int64) (int, error) {
	if err := requireDirector(actor); err != nil {
		return 0, err
	}
	e, err := s.entries.Get(ctx, campaignID, entryID)
	if err != nil {
		return 0, err
	}
	return s.repo.CountForEntry(ctx, campaignID, e.SystemID, e.FieldKey, ChoiceSourceCampaign, fmt.Sprint(e.ID))
}

func (s *choiceRuleService) ForgetEntry(ctx context.Context, campaignID, systemID, fieldKey string, entryID int64) (int, error) {
	return s.repo.DeleteForEntry(ctx, campaignID, systemID, fieldKey, ChoiceSourceCampaign, fmt.Sprint(entryID))
}

// presetFieldLabel is the label a character preset gives a field, or its key.
func presetFieldLabel(m *SystemManifest, key string) string {
	for _, p := range m.EntityPresets {
		for _, f := range p.Fields {
			if f.Key == key && strings.TrimSpace(f.Label) != "" {
				return strings.TrimSpace(f.Label)
			}
		}
	}
	return key
}

func (s *choiceRuleService) RequestChoice(ctx context.Context, in ChoiceRequestInput) (*ChoiceRequestOutcome, error) {
	if in.UserID == "" {
		return nil, apperror.NewForbidden("sign in to ask your Director")
	}
	// Read the list as the player gets it, rules on: the entry must really be
	// one they have to ask for, whatever the caller believed.
	c := WithChoiceViewerUser(WithChoiceViewer(ctx, false), in.UserID)
	list, err := s.choices.CharacterChoiceList(c, in.CampaignID, in.FieldKey)
	if err != nil {
		return nil, err
	}
	ch := choiceByName(list.Choices, in.EntryName)
	if ch == nil || ch.State != ChoiceStateAskFirst {
		return nil, apperror.NewValidation("That entry doesn't need your Director's yes.")
	}
	req := &ChoiceRequest{
		CampaignID: in.CampaignID, SystemID: list.SystemID, FieldKey: in.FieldKey,
		Source: ch.Source, Key: ch.Key, Name: ch.Name, UserID: in.UserID,
		EntityID: in.EntityID, EntityName: in.EntityName,
	}
	if err := s.repo.UpsertWaiting(ctx, req); err != nil {
		return nil, err
	}
	return &ChoiceRequestOutcome{ID: req.ID, FieldKey: in.FieldKey, EntryName: ch.Name, Status: RequestWaiting}, nil
}

func (s *choiceRuleService) WaitingRequests(ctx context.Context, campaignID string, actor EntryActor) ([]WaitingChoiceRequest, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil {
		return []WaitingChoiceRequest{}, nil
	}
	reqs, err := s.repo.ListRequests(ctx, campaignID, m.ID, RequestWaiting)
	if err != nil {
		return nil, err
	}
	names := memberNames(s.members(ctx, campaignID))
	out := make([]WaitingChoiceRequest, 0, len(reqs))
	for _, q := range reqs {
		out = append(out, WaitingChoiceRequest{ChoiceRequest: q, FieldLabel: presetFieldLabel(m, q.FieldKey), PlayerName: names[q.UserID]})
	}
	return out, nil
}

func (s *choiceRuleService) AllowRequest(ctx context.Context, campaignID string, actor EntryActor, id int64) (*AllowOutcome, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	req, err := s.repo.GetRequest(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if req.Status != RequestWaiting {
		return nil, apperror.NewConflict("That request has already been answered.")
	}
	// The entry must still be there for this player to be given it.
	pl, err := s.playerList(ctx, campaignID, req.FieldKey, req.UserID)
	if err != nil {
		return nil, err
	}
	var entry *Choice
	for i := range pl.Choices {
		if pl.Choices[i].Source == req.Source && pl.Choices[i].Key == req.Key {
			entry = &pl.Choices[i]
		}
	}
	if entry == nil {
		return nil, apperror.NewValidation("That entry isn't on the list any more, so it can't be allowed.")
	}
	// Open for this player, ahead of any mark for everyone: "Allow for Pat".
	if err := s.repo.Apply(ctx, []ChoiceRule{{
		CampaignID: campaignID, SystemID: req.SystemID, FieldKey: req.FieldKey,
		Source: req.Source, Key: req.Key, Name: entry.Name, Target: req.UserID,
		State: RuleOpen, CreatedBy: actor.UserID,
	}}, nil); err != nil {
		return nil, err
	}
	out := &AllowOutcome{}
	if s.heroes != nil && req.EntityID != "" {
		ok, err := s.heroes.SetField(ctx, campaignID, req.EntityID, req.UserID, req.FieldKey, entry.Name)
		if err != nil {
			return nil, err
		}
		out.HeroUpdated = ok
	}
	if _, err := s.repo.Close(ctx, campaignID, id, RequestAllowed, "", actor.UserID); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *choiceRuleService) SuggestAnother(ctx context.Context, campaignID string, actor EntryActor, id int64, message string) error {
	if err := requireDirector(actor); err != nil {
		return err
	}
	message = strings.TrimSpace(message)
	if utf8.RuneCountInString(message) > maxRequestNote {
		return apperror.NewValidation(fmt.Sprintf("A note can be at most %d characters.", maxRequestNote))
	}
	if _, err := s.repo.GetRequest(ctx, campaignID, id); err != nil {
		return err
	}
	ok, err := s.repo.Close(ctx, campaignID, id, RequestSuggested, message, actor.UserID)
	if err != nil {
		return err
	}
	if !ok {
		return apperror.NewConflict("That request has already been answered.")
	}
	return nil
}

func (s *choiceRuleService) HeroWaiting(ctx context.Context, campaignID, userID, entityID string) ([]HeroWaitingPick, error) {
	if userID == "" || entityID == "" {
		return nil, nil
	}
	reqs, err := s.repo.HeroWaiting(ctx, campaignID, userID, entityID)
	if err != nil {
		return nil, err
	}
	out := make([]HeroWaitingPick, 0, len(reqs))
	for _, q := range reqs {
		out = append(out, HeroWaitingPick{ID: q.ID, FieldKey: q.FieldKey, EntryName: q.Name})
	}
	return out, nil
}
