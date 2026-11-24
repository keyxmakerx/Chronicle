package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/fieldrules"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

// fieldRulesWorld gives the homebrew rules plugin the pages and eras it
// names, which belong to the entities and calendar plugins.
type fieldRulesWorld struct {
	pages     entities.EntityService
	calendars calendar.CalendarService
	eras      calendar.EraVersionService
	// trackers is set once the trackers plugin is up; nil reads no trackers.
	trackers trackers.Service
}

func (w *fieldRulesWorld) PlaceName(ctx context.Context, campaignID, entityID string) (string, bool) {
	e, err := w.pages.GetByID(ctx, entityID)
	if err != nil || e == nil || e.CampaignID != campaignID {
		return "", false
	}
	return e.Name, true
}

// EraNames is the calendar as the viewer sees it; only a Director asks, so
// hidden eras are included.
func (w *fieldRulesWorld) EraNames(ctx context.Context, campaignID string, v fieldrules.Viewer) map[int]string {
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
func (w *fieldRulesWorld) PlaceChain(ctx context.Context, campaignID, entityID string) []string {
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
func (w *fieldRulesWorld) CurrentEra(ctx context.Context, campaignID string, v fieldrules.Viewer) int {
	pe, err := w.eras.PageEras(ctx, campaignID, permissions.RequestViewer(v.Role, v.UserID))
	if err != nil || pe == nil || pe.Current == nil {
		return 0
	}
	if pe.Calendar != nil && pe.Calendar.EraIsSecret(pe.Current) && !v.IsDM() {
		return 0
	}
	return pe.Current.ID
}

// Today reads the main calendar as the viewer sees it, so a moon hidden from
// players or an event they can't see never moves a result they are shown.
func (w *fieldRulesWorld) Today(ctx context.Context, campaignID string, v fieldrules.Viewer) fieldrules.Situation {
	var out fieldrules.Situation
	pv := permissions.RequestViewer(v.Role, v.UserID)
	c, err := w.calendars.GetDefaultCalendarForViewer(ctx, campaignID, pv)
	if err != nil || c == nil || c.CurrentMonth < 1 || c.CurrentDay < 1 {
		return out
	}
	y, m, d := c.CurrentYear, c.CurrentMonth, c.CurrentDay
	abs := c.CurrentDayIndex()
	out.DayPart = c.DayPart(y, m, d, c.CurrentHour, c.CurrentMinute)
	out.Moons = map[string]fieldrules.MoonNow{}
	for i := range c.Moons {
		mo := &c.Moons[i]
		out.Moons[strings.ToLower(strings.TrimSpace(mo.Name))] = fieldrules.MoonNow{Day: mo.CycleDay(abs), Words: mo.PhaseWords(abs)}
	}
	events, err := w.calendars.ListEventsForMonth(ctx, c.ID, campaignID, y, m, pv)
	if err != nil {
		slog.Warn("fieldrules: cannot read today's events", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return out
	}
	var kinds map[int]calendar.EventKind
	out.EventTags = map[string]bool{}
	for _, e := range events {
		if !eventOn(e, y, m, d) {
			continue
		}
		out.EventTags[strings.ToLower(strings.TrimSpace(e.Name))] = true
		if e.KindID == nil {
			continue
		}
		if kinds == nil {
			kinds = map[int]calendar.EventKind{}
			list, err := w.calendars.ListEventKinds(ctx, campaignID)
			if err != nil {
				slog.Warn("fieldrules: cannot read event kinds", slog.String("campaign_id", campaignID), slog.Any("error", err))
			}
			for _, k := range list {
				kinds[k.ID] = k
			}
		}
		if k, ok := kinds[*e.KindID]; ok {
			out.EventTags[strings.ToLower(k.Slug)] = true
			out.EventTags[strings.ToLower(strings.TrimSpace(k.Name))] = true
		}
	}
	return out
}

// PageTrackers reads the page's trackers as the viewer may see them, so a
// hidden tracker never moves a result a player is shown.
func (w *fieldRulesWorld) PageTrackers(ctx context.Context, campaignID, entityID string, v fieldrules.Viewer) map[string]float64 {
	if w.trackers == nil {
		return nil
	}
	tv := trackers.Viewer{UserID: v.UserID, MemberRole: v.Role, VisibilityRole: v.Role, Director: v.IsDM()}
	pv, err := w.trackers.Page(ctx, campaignID, entityID, tv)
	if err != nil || pv == nil {
		return nil
	}
	out := make(map[string]float64, len(pv.Trackers))
	for _, t := range pv.Trackers {
		out[strings.ToLower(strings.TrimSpace(t.Name))] = float64(t.Value)
	}
	return out
}

// fieldWordsAdapter gives pages the rules' results and local words.
type fieldWordsAdapter struct{ svc fieldrules.Service }

func (a fieldWordsAdapter) Evaluate(ctx context.Context, campaignID, entityID string, ruleIDs []int, data map[string]any, role int, userID string) map[int]string {
	return a.svc.Evaluate(ctx, campaignID, entityID, ruleIDs, data, fieldrules.Viewer{UserID: userID, Role: role})
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
		out.Rows = append(out.Rows, records.RuleRow{Is: row.Is, From: row.From, To: row.To, EventTag: row.EventTag, Time: row.Time, Gives: row.Gives})
	}
	for _, a := range r.Adjust {
		out.Adjust = append(out.Adjust, records.RuleAdjust{Tracker: a.Tracker, Op: a.Op, Value: a.Value, Add: a.Add, Min: a.Min, Max: a.Max})
	}
	return out
}

func fromAIRule(r records.HomebrewRule) fieldrules.RuleInput {
	in := fieldrules.RuleInput{ID: r.ID, Name: r.Name, Follows: r.Follows}
	for _, row := range r.Rows {
		in.Rows = append(in.Rows, fieldrules.RuleRow{Is: row.Is, From: row.From, To: row.To, EventTag: row.EventTag, Time: row.Time, Gives: row.Gives})
	}
	for _, a := range r.Adjust {
		in.Adjust = append(in.Adjust, fieldrules.RuleAdjust{Tracker: a.Tracker, Op: a.Op, Value: a.Value, Add: a.Add, Min: a.Min, Max: a.Max})
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
