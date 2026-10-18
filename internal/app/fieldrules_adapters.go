package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/fieldrules"
)

// fieldRulesWorld gives the homebrew rules plugin the pages and eras it
// names, which belong to the entities and calendar plugins.
type fieldRulesWorld struct {
	pages     entities.EntityService
	calendars calendar.CalendarService
	eras      calendar.EraVersionService
}

func (w fieldRulesWorld) PlaceName(ctx context.Context, campaignID, entityID string) (string, bool) {
	e, err := w.pages.GetByID(ctx, entityID)
	if err != nil || e == nil || e.CampaignID != campaignID {
		return "", false
	}
	return e.Name, true
}

// EraNames is the calendar as the viewer sees it; only a Director asks, so
// hidden eras are included.
func (w fieldRulesWorld) EraNames(ctx context.Context, campaignID string, v fieldrules.Viewer) map[int]string {
	out := map[int]string{}
	cal, err := w.calendars.GetDefaultCalendarForViewer(ctx, campaignID, permissions.RequestViewer(v.Role, v.UserID))
	if err != nil || cal == nil {
		return out
	}
	for _, e := range cal.Eras {
		out[e.ID] = e.Name
	}
	return out
}

// PlaceChain reads parents unfiltered on purpose: it only decides which local
// words apply, and a word is shown or not by its own DM-only flag, so no
// hidden page is named by it.
func (w fieldRulesWorld) PlaceChain(ctx context.Context, campaignID, entityID string) []string {
	chain := []string{entityID}
	parents, err := w.pages.GetAncestors(ctx, entityID, permissions.RoleOwner, "")
	if err != nil {
		return chain
	}
	for _, p := range parents {
		if p.CampaignID == campaignID {
			chain = append(chain, p.ID)
		}
	}
	return chain
}

// CurrentEra uses the viewer's own calendar view, so an era still hidden
// from players never renames anything for them.
func (w fieldRulesWorld) CurrentEra(ctx context.Context, campaignID string, v fieldrules.Viewer) int {
	pe, err := w.eras.PageEras(ctx, campaignID, permissions.RequestViewer(v.Role, v.UserID))
	if err != nil || pe == nil || pe.Current == nil {
		return 0
	}
	if pe.Calendar != nil && pe.Calendar.EraIsSecret(pe.Current) && !v.IsDM() {
		return 0
	}
	return pe.Current.ID
}

// fieldWordsAdapter gives pages the rules' results and local words.
type fieldWordsAdapter struct{ svc fieldrules.Service }

func (a fieldWordsAdapter) Evaluate(ctx context.Context, campaignID string, ruleIDs []int, data map[string]any) map[int]string {
	return a.svc.Evaluate(ctx, campaignID, ruleIDs, data)
}

func (a fieldWordsAdapter) WordsFor(ctx context.Context, campaignID, entityID string, role int, userID string) map[string]string {
	return a.svc.WordsFor(ctx, campaignID, entityID, fieldrules.Viewer{UserID: userID, Role: role})
}

func (a fieldWordsAdapter) Rename(words map[string]string, text string) (string, bool) {
	return fieldrules.Rename(words, text)
}

func (a fieldWordsAdapter) RuleExists(ctx context.Context, campaignID string, ruleID int) bool {
	return a.svc.RuleExists(ctx, campaignID, ruleID)
}

// aiHomebrewAdapter gives AI Import the homebrew rules and local words, as
// the operator running the import.
type aiHomebrewAdapter struct{ svc fieldrules.Service }

var _ records.HomebrewAPI = aiHomebrewAdapter{}

func homebrewViewer(a records.Actor) fieldrules.Viewer {
	return fieldrules.Viewer{UserID: a.UserID, Role: a.Role}
}

func toAIRule(r fieldrules.Rule) records.HomebrewRule {
	out := records.HomebrewRule{ID: r.ID, Name: r.Name, Follows: r.Follows}
	for _, row := range r.Rows {
		out.Rows = append(out.Rows, records.RuleRow{Is: row.Is, From: row.From, To: row.To, Gives: row.Gives})
	}
	return out
}

func fromAIRule(r records.HomebrewRule) fieldrules.RuleInput {
	in := fieldrules.RuleInput{ID: r.ID, Name: r.Name, Follows: r.Follows}
	for _, row := range r.Rows {
		in.Rows = append(in.Rows, fieldrules.RuleRow{Is: row.Is, From: row.From, To: row.To, Gives: row.Gives})
	}
	return in
}

func fromAIWord(w records.HomebrewWord) fieldrules.WordInput {
	return fieldrules.WordInput{ID: w.ID, Word: w.Word, Local: w.Local, PlaceID: w.PlaceID, EraID: w.EraID, DMOnly: w.DMOnly}
}

func (a aiHomebrewAdapter) Rules(ctx context.Context, campaignID string, act records.Actor) ([]records.HomebrewRule, error) {
	rules, err := a.svc.Rules(ctx, campaignID, homebrewViewer(act))
	if err != nil {
		return nil, err
	}
	out := make([]records.HomebrewRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, toAIRule(r))
	}
	return out, nil
}

func (a aiHomebrewAdapter) RuleNamed(ctx context.Context, campaignID, name string) (*records.HomebrewRule, error) {
	r, err := a.svc.RuleNamed(ctx, campaignID, name)
	if err != nil || r == nil {
		return nil, err
	}
	out := toAIRule(*r)
	return &out, nil
}

func (a aiHomebrewAdapter) CheckRule(in records.HomebrewRule) error {
	return a.svc.CheckRule(fromAIRule(in))
}

func (a aiHomebrewAdapter) SaveRule(ctx context.Context, campaignID string, in records.HomebrewRule, act records.Actor) error {
	_, err := a.svc.SaveRule(ctx, campaignID, fromAIRule(in), homebrewViewer(act))
	return err
}

func (a aiHomebrewAdapter) DeleteRule(ctx context.Context, campaignID string, id int, act records.Actor) error {
	return a.svc.DeleteRule(ctx, campaignID, id, homebrewViewer(act))
}

func (a aiHomebrewAdapter) Words(ctx context.Context, campaignID string, act records.Actor) ([]records.HomebrewWord, error) {
	words, err := a.svc.Words(ctx, campaignID, homebrewViewer(act))
	if err != nil {
		return nil, err
	}
	out := make([]records.HomebrewWord, 0, len(words))
	for _, w := range words {
		out = append(out, records.HomebrewWord{ID: w.ID, Word: w.Word, Local: w.Local, PlaceID: w.PlaceID,
			PlaceName: w.PlaceName, EraID: w.EraID, EraName: w.EraName, DMOnly: w.DMOnly})
	}
	return out, nil
}

func (a aiHomebrewAdapter) CheckWord(ctx context.Context, campaignID string, in records.HomebrewWord, act records.Actor) error {
	return a.svc.CheckWord(ctx, campaignID, fromAIWord(in), homebrewViewer(act))
}

func (a aiHomebrewAdapter) SaveWord(ctx context.Context, campaignID string, in records.HomebrewWord, act records.Actor) error {
	_, err := a.svc.SaveWord(ctx, campaignID, fromAIWord(in), homebrewViewer(act))
	return err
}

func (a aiHomebrewAdapter) DeleteWord(ctx context.Context, campaignID string, id int, act records.Actor) error {
	return a.svc.DeleteWord(ctx, campaignID, id, homebrewViewer(act))
}

func (a aiHomebrewAdapter) Eras(ctx context.Context, campaignID string, act records.Actor) map[int]string {
	return a.svc.Eras(ctx, campaignID, homebrewViewer(act))
}
