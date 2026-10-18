package fieldrules

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// World answers what this plugin needs to know about pages and eras, which
// belong to other plugins. It is implemented in internal/app.
type World interface {
	// PlaceName is the name of a page of the campaign, and whether it exists.
	PlaceName(ctx context.Context, campaignID, entityID string) (string, bool)
	// EraNames lists the eras of the campaign's main calendar by id.
	EraNames(ctx context.Context, campaignID string, v Viewer) map[int]string
	// PlaceChain is the page followed by its parents, nearest first. It is
	// not narrowed to the viewer: it only decides which words apply.
	PlaceChain(ctx context.Context, campaignID, entityID string) []string
	// CurrentEra is the era the main calendar's date is in for this viewer,
	// or 0 (none, or one still hidden from them).
	CurrentEra(ctx context.Context, campaignID string, v Viewer) int
}

// Service is the homebrew rules and local words. Only the Director changes
// them; pages read what a rule gives and which words apply.
type Service interface {
	Rules(ctx context.Context, campaignID string, v Viewer) ([]Rule, error)
	SaveRule(ctx context.Context, campaignID string, in RuleInput, v Viewer) (*Rule, error)
	DeleteRule(ctx context.Context, campaignID string, id int, v Viewer) error
	// RuleNamed finds a rule by name, case-insensitively; nil when none.
	RuleNamed(ctx context.Context, campaignID, name string) (*Rule, error)
	// RuleExists reports whether the campaign has that rule.
	RuleExists(ctx context.Context, campaignID string, id int) bool
	// CheckRule and CheckWord are SaveRule's and SaveWord's checks without
	// the write, for a review screen.
	CheckRule(in RuleInput) error
	CheckWord(ctx context.Context, campaignID string, in WordInput, v Viewer) error
	// Eras is every era on the campaign's calendar by id, hidden ones too.
	Eras(ctx context.Context, campaignID string, v Viewer) map[int]string

	Words(ctx context.Context, campaignID string, v Viewer) ([]LocalWord, error)
	SaveWord(ctx context.Context, campaignID string, in WordInput, v Viewer) (*LocalWord, error)
	DeleteWord(ctx context.Context, campaignID string, id int, v Viewer) error

	// Evaluate gives, per rule id, what the rule gives for a page's field
	// values. A rule that is gone or matches nothing gives "".
	Evaluate(ctx context.Context, campaignID string, ruleIDs []int, data map[string]any) map[int]string
	// WordsFor is the local words a viewer sees on one page, keyed by the
	// lower-cased usual word (see Rename).
	WordsFor(ctx context.Context, campaignID, entityID string, v Viewer) map[string]string
}

type service struct {
	repo  Repository
	world World
}

// NewService creates the service.
func NewService(repo Repository, world World) Service {
	return &service{repo: repo, world: world}
}

// notDirector refuses anyone but the Director (or a granted co-Director).
func notDirector() error { return apperror.NewForbidden("only the Director can change homebrew tools") }

func (s *service) Rules(ctx context.Context, campaignID string, v Viewer) ([]Rule, error) {
	if !v.IsDM() {
		return nil, notDirector()
	}
	out, err := s.repo.ListRules(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if out == nil {
		out = []Rule{}
	}
	return out, nil
}

// cleanRule trims and checks a rule; the message names the row a person
// would fix.
func cleanRule(in RuleInput) (Rule, error) {
	r := Rule{ID: in.ID, Name: strings.TrimSpace(in.Name), Follows: strings.TrimSpace(in.Follows)}
	switch {
	case r.Name == "":
		return r, apperror.NewBadRequest("a rule needs a name")
	case utf8.RuneCountInString(r.Name) > MaxNameLen:
		return r, apperror.NewBadRequest(fmt.Sprintf("a rule's name can be at most %d characters", MaxNameLen))
	case r.Follows == "":
		return r, apperror.NewBadRequest("say which field the rule follows")
	case utf8.RuneCountInString(r.Follows) > maxFollowLen:
		return r, apperror.NewBadRequest("the followed field's key is too long")
	case len(in.Rows) == 0:
		return r, apperror.NewBadRequest("a rule needs at least one row")
	case len(in.Rows) > MaxRuleRows:
		return r, apperror.NewBadRequest(fmt.Sprintf("a rule can have at most %d rows", MaxRuleRows))
	}
	for i, row := range in.Rows {
		n := i + 1
		row.Is = strings.TrimSpace(row.Is)
		row.Gives = strings.TrimSpace(row.Gives)
		ranged := row.From != nil || row.To != nil
		switch {
		case row.Gives == "":
			return r, apperror.NewBadRequest(fmt.Sprintf("row %d needs what it gives", n))
		case utf8.RuneCountInString(row.Gives) > MaxGivesLen || utf8.RuneCountInString(row.Is) > maxIsLen:
			return r, apperror.NewBadRequest(fmt.Sprintf("row %d is too long", n))
		case ranged && row.Is != "":
			return r, apperror.NewBadRequest(fmt.Sprintf("row %d matches a value or a range of numbers, not both", n))
		case !ranged && row.Is == "":
			return r, apperror.NewBadRequest(fmt.Sprintf("row %d needs the value it matches", n))
		case row.From != nil && row.To != nil && *row.From > *row.To:
			return r, apperror.NewBadRequest(fmt.Sprintf("row %d's range starts after it ends", n))
		}
		r.Rows = append(r.Rows, row)
	}
	return r, nil
}

// CleanRule checks a rule and returns it trimmed, the way it would be saved.
// It reads nothing, so a rule kept elsewhere (a Workshop recipe) is held to
// the same limits.
func CleanRule(in RuleInput) (Rule, error) { return cleanRule(in) }

func (s *service) CheckRule(in RuleInput) error {
	_, err := cleanRule(in)
	return err
}

func (s *service) CheckWord(ctx context.Context, campaignID string, in WordInput, v Viewer) error {
	if !v.IsDM() {
		return notDirector()
	}
	_, err := s.cleanWord(ctx, campaignID, in, v)
	return err
}

func (s *service) Eras(ctx context.Context, campaignID string, v Viewer) map[int]string {
	if !v.IsDM() {
		return map[int]string{}
	}
	return s.world.EraNames(ctx, campaignID, v)
}

func (s *service) SaveRule(ctx context.Context, campaignID string, in RuleInput, v Viewer) (*Rule, error) {
	if !v.IsDM() {
		return nil, notDirector()
	}
	r, err := cleanRule(in)
	if err != nil {
		return nil, err
	}
	if r.ID == 0 {
		n, err := s.repo.CountRules(ctx, campaignID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if n >= MaxRules {
			return nil, apperror.NewBadRequest(fmt.Sprintf("a campaign can have at most %d rules", MaxRules))
		}
		r.ID, err = s.repo.InsertRule(ctx, campaignID, r)
		if err != nil {
			return nil, ruleErr(err, r.Name)
		}
	} else {
		cur, err := s.repo.GetRule(ctx, campaignID, r.ID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if cur == nil {
			return nil, apperror.NewNotFound("rule not found")
		}
		if _, err := s.repo.UpdateRule(ctx, campaignID, r); err != nil {
			return nil, ruleErr(err, r.Name)
		}
	}
	saved, err := s.repo.GetRule(ctx, campaignID, r.ID)
	if err != nil || saved == nil {
		return nil, apperror.NewInternal(fmt.Errorf("reading saved rule: %v", err))
	}
	return saved, nil
}

func ruleErr(err error, name string) error {
	if errors.Is(err, errDuplicate) {
		return apperror.NewConflict("there is already a rule called " + name)
	}
	return apperror.NewInternal(err)
}

func (s *service) DeleteRule(ctx context.Context, campaignID string, id int, v Viewer) error {
	if !v.IsDM() {
		return notDirector()
	}
	ok, err := s.repo.DeleteRule(ctx, campaignID, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !ok {
		return apperror.NewNotFound("rule not found")
	}
	return nil
}

func (s *service) RuleNamed(ctx context.Context, campaignID, name string) (*Rule, error) {
	rules, err := s.repo.ListRules(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	for i := range rules {
		if strings.EqualFold(rules[i].Name, strings.TrimSpace(name)) {
			return &rules[i], nil
		}
	}
	return nil, nil
}

func (s *service) RuleExists(ctx context.Context, campaignID string, id int) bool {
	r, err := s.repo.GetRule(ctx, campaignID, id)
	return err == nil && r != nil
}

func (s *service) Words(ctx context.Context, campaignID string, v Viewer) ([]LocalWord, error) {
	if !v.IsDM() {
		return nil, notDirector()
	}
	words, err := s.repo.ListWords(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	eras := s.world.EraNames(ctx, campaignID, v)
	out := make([]LocalWord, 0, len(words))
	for _, w := range words {
		if w.EraID != 0 {
			name, ok := eras[w.EraID]
			if !ok {
				continue // its era was removed; it can never apply
			}
			w.EraName = name
		}
		out = append(out, w)
	}
	return out, nil
}

func (s *service) cleanWord(ctx context.Context, campaignID string, in WordInput, v Viewer) (LocalWord, error) {
	w := LocalWord{ID: in.ID, Word: strings.TrimSpace(in.Word), Local: strings.TrimSpace(in.Local),
		PlaceID: strings.TrimSpace(in.PlaceID), EraID: in.EraID, DMOnly: in.DMOnly}
	switch {
	case w.Word == "" || w.Local == "":
		return w, apperror.NewBadRequest("a local word needs the usual word and what it is called")
	case utf8.RuneCountInString(w.Word) > MaxNameLen || utf8.RuneCountInString(w.Local) > MaxNameLen:
		return w, apperror.NewBadRequest(fmt.Sprintf("a word can be at most %d characters", MaxNameLen))
	case (w.PlaceID == "") == (w.EraID == 0):
		return w, apperror.NewBadRequest("say where the word is used: one place or one era")
	}
	if w.PlaceID != "" {
		name, ok := s.world.PlaceName(ctx, campaignID, w.PlaceID)
		if !ok {
			return w, apperror.NewBadRequest("that place is not a page of this campaign")
		}
		w.PlaceName = name
	} else {
		name, ok := s.world.EraNames(ctx, campaignID, v)[w.EraID]
		if !ok {
			return w, apperror.NewBadRequest("that era is not on this campaign's calendar")
		}
		w.EraName = name
	}
	return w, nil
}

func (s *service) SaveWord(ctx context.Context, campaignID string, in WordInput, v Viewer) (*LocalWord, error) {
	if !v.IsDM() {
		return nil, notDirector()
	}
	w, err := s.cleanWord(ctx, campaignID, in, v)
	if err != nil {
		return nil, err
	}
	if w.ID == 0 {
		n, err := s.repo.CountWords(ctx, campaignID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if n >= MaxWords {
			return nil, apperror.NewBadRequest(fmt.Sprintf("a campaign can have at most %d local words", MaxWords))
		}
		if w.ID, err = s.repo.InsertWord(ctx, campaignID, w); err != nil {
			return nil, wordErr(err, w)
		}
		return &w, nil
	}
	if !s.wordExists(ctx, campaignID, w.ID) {
		return nil, apperror.NewNotFound("local word not found")
	}
	if _, err := s.repo.UpdateWord(ctx, campaignID, w); err != nil {
		return nil, wordErr(err, w)
	}
	return &w, nil
}

func (s *service) wordExists(ctx context.Context, campaignID string, id int) bool {
	words, err := s.repo.ListWords(ctx, campaignID)
	if err != nil {
		return false
	}
	for _, w := range words {
		if w.ID == id {
			return true
		}
	}
	return false
}

func wordErr(err error, w LocalWord) error {
	if errors.Is(err, errDuplicate) {
		where := w.PlaceName
		if where == "" {
			where = w.EraName
		}
		return apperror.NewConflict(w.Word + " already has a local name in " + where)
	}
	return apperror.NewInternal(err)
}

func (s *service) DeleteWord(ctx context.Context, campaignID string, id int, v Viewer) error {
	if !v.IsDM() {
		return notDirector()
	}
	ok, err := s.repo.DeleteWord(ctx, campaignID, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !ok {
		return apperror.NewNotFound("local word not found")
	}
	return nil
}

func (s *service) Evaluate(ctx context.Context, campaignID string, ruleIDs []int, data map[string]any) map[int]string {
	out := map[int]string{}
	if len(ruleIDs) == 0 {
		return out
	}
	rules, err := s.repo.ListRules(ctx, campaignID)
	if err != nil {
		// A failed read shows the fields without their rule, never an error page.
		slog.Warn("fieldrules: cannot read rules", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return out
	}
	want := make(map[int]bool, len(ruleIDs))
	for _, id := range ruleIDs {
		want[id] = true
	}
	for i := range rules {
		if want[rules[i].ID] {
			out[rules[i].ID] = rules[i].Gives(data[rules[i].Follows])
		}
	}
	return out
}

func (s *service) WordsFor(ctx context.Context, campaignID, entityID string, v Viewer) map[string]string {
	words, err := s.repo.ListWords(ctx, campaignID)
	if err != nil {
		slog.Warn("fieldrules: cannot read local words", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	if len(words) == 0 {
		return nil
	}
	var places []string
	if entityID != "" {
		places = s.world.PlaceChain(ctx, campaignID, entityID)
	}
	return ChooseWords(words, places, s.world.CurrentEra(ctx, campaignID, v), v.IsDM())
}
