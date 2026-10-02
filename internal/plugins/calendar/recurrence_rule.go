// Package calendar - recurrence_rule.go defines the rule a RecurrenceByRule
// event repeats by ("every full moon of Luna", "the 3rd Kingsday of each
// month", "2 days after the Festival of Masks") and its strict parsing.
//
// A rule is a list of conditions a day must ALL meet, so a "custom" rule is
// just more conditions, never a new kind. Parsing here checks shape only;
// what a rule may reference in its calendar (a moon, a season, another
// event) is the service's check, because it needs the calendar and the
// author.
package calendar

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Rule condition kinds. The set is closed: an unknown kind is a 400, never
// a condition that silently matches nothing.
const (
	RuleMoonPhase  = "moon_phase"   // the day a moon reaches a phase
	RuleWeekday    = "weekday"      // any of the listed weekdays (0-based)
	RuleNthWeekday = "nth_weekday"  // the nth (or last) of a weekday in its month
	RuleDayOfMonth = "day_of_month" // a day of the month (or the last)
	RuleMonths     = "months"       // any of the listed months (1-based)
	RuleSeason     = "season"       // any day inside a season
	RuleAfterEvent = "after_event"  // N days after (negative: before) each occurrence of another event
	// RuleMonth, RuleSeasonStart and RuleRelativeToEvent are the single-value
	// forms the editor offers: one month, a season's first day, and the
	// anchor event's own dates (after_event with zero days; the rule's
	// offset_days then reads "2 days after").
	RuleMonth           = "month"
	RuleSeasonStart     = "season_start"
	RuleRelativeToEvent = "relative_to_event"
	ruleLastOfMonth     = -1 // the "last" sentinel for nth_weekday.n and day_of_month.day
)

// Moon phase names a moon_phase condition accepts, and the point of the
// cycle each one is (0 new, 0.25 first quarter, 0.5 full, 0.75 last
// quarter) — the same turning points Moon.MoonPhaseName centres its names on.
var rulePhaseTargets = map[string]float64{
	"new":           0,
	"first_quarter": 0.25,
	"full":          0.5,
	"last_quarter":  0.75,
}

// Bounds on a rule. They keep a stored rule small and an expansion cheap;
// none of them is near what a real calendar needs.
const (
	maxRuleJSONBytes  = 4096
	maxRuleConditions = 6
	maxRuleEvery      = 99
	// maxRuleShiftDays bounds offset_days and after_event's days, so a scan
	// never has to walk far outside the range it was asked about.
	maxRuleShiftDays = 365
	maxRuleListLen   = 200 // weekdays/months lists; matches maxCalendarMonths
	maxRuleNth       = 5
)

// RecurrenceRule is what a RecurrenceByRule event repeats by: the days every
// condition in Match holds, taking every Every-th one (default 1), each
// moved by OffsetDays (negative moves earlier).
type RecurrenceRule struct {
	Match      []RuleCondition `json:"match"`
	Every      int             `json:"every,omitempty"`
	OffsetDays int             `json:"offset_days,omitempty"`
}

// RuleCondition is one tagged condition. Only the fields its Kind names may
// be set; the pointers are the fields where zero is a real value (weekday 0,
// "0 days after").
type RuleCondition struct {
	Kind     string `json:"kind"`
	MoonID   int    `json:"moon_id,omitempty"`
	Phase    string `json:"phase,omitempty"`
	Weekdays []int  `json:"weekdays,omitempty"`
	N        int    `json:"n,omitempty"`
	Weekday  *int   `json:"weekday,omitempty"`
	Day      int    `json:"day,omitempty"`
	Months   []int  `json:"months,omitempty"`
	Month    int    `json:"month,omitempty"`
	SeasonID int    `json:"season_id,omitempty"`
	EventID  string `json:"event_id,omitempty"`
	Days     *int   `json:"days,omitempty"`
}

// every returns the rule's step, defaulting an absent one to 1.
func (r *RecurrenceRule) every() int {
	if r.Every < 1 {
		return 1
	}
	return r.Every
}

// anchorIDs returns the event ids the rule's after_event conditions name.
func (r *RecurrenceRule) anchorIDs() []string {
	if r == nil {
		return nil
	}
	var ids []string
	for _, c := range r.Match {
		if (c.Kind == RuleAfterEvent || c.Kind == RuleRelativeToEvent) && c.EventID != "" {
			ids = append(ids, c.EventID)
		}
	}
	return ids
}

// hasAfterEvent reports whether the rule depends on another event.
func (r *RecurrenceRule) hasAfterEvent() bool { return len(r.anchorIDs()) > 0 }

// usesRule reports whether e is expanded by its rule rather than by
// OccursOn: it repeats, its type is RecurrenceByRule, and it has a rule.
func (e *Event) usesRule() bool {
	return e.IsRecurring && e.RecurrenceType != nil && *e.RecurrenceType == RecurrenceByRule && e.RecurrenceRule != nil
}

// isRepeating reports whether e has occurrences beyond its own date, so
// skipping or moving one of them means something.
func (e *Event) isRepeating() bool {
	if !e.IsRecurring || e.RecurrenceType == nil {
		return false
	}
	switch *e.RecurrenceType {
	case RecurrenceByRule:
		return e.RecurrenceRule != nil
	case RecurrenceWeekly, RecurrenceBiWeekly, RecurrenceMonthly, RecurrenceCustom, RecurrenceYearly:
		return true
	}
	return false
}

// ParseRecurrenceRule decodes and shape-checks a rule. A nil, empty or JSON
// null input is "no rule" (nil, nil). Unknown keys, unknown kinds, fields a
// kind does not use, and out-of-bounds values are all validation errors.
func ParseRecurrenceRule(raw []byte) (*RecurrenceRule, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if len(trimmed) > maxRuleJSONBytes {
		return nil, apperror.NewValidation(fmt.Sprintf("recurrence_rule is larger than %d bytes", maxRuleJSONBytes))
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var r RecurrenceRule
	if err := dec.Decode(&r); err != nil {
		return nil, apperror.NewValidation("recurrence_rule is not a valid rule: " + err.Error())
	}
	if dec.More() {
		return nil, apperror.NewValidation("recurrence_rule has trailing data")
	}
	if err := r.validateShape(); err != nil {
		return nil, err
	}
	return &r, nil
}

// validateShape checks everything about a rule that needs no calendar.
func (r *RecurrenceRule) validateShape() error {
	if len(r.Match) == 0 {
		return apperror.NewValidation("recurrence_rule needs at least one condition in match")
	}
	if len(r.Match) > maxRuleConditions {
		return apperror.NewValidation(fmt.Sprintf("recurrence_rule may have at most %d conditions", maxRuleConditions))
	}
	if r.Every < 0 || r.Every > maxRuleEvery {
		return apperror.NewValidation(fmt.Sprintf("recurrence_rule.every must be between 1 and %d", maxRuleEvery))
	}
	if r.OffsetDays < -maxRuleShiftDays || r.OffsetDays > maxRuleShiftDays {
		return apperror.NewValidation(fmt.Sprintf("recurrence_rule.offset_days must be within %d days", maxRuleShiftDays))
	}
	for i := range r.Match {
		if err := r.Match[i].validateShape(); err != nil {
			return apperror.NewValidation(fmt.Sprintf("recurrence_rule.match[%d]: %s", i, apperror.SafeMessage(err)))
		}
	}
	return nil
}

// validateShape checks one condition's kind and the fields that kind uses,
// refusing any field it does not.
func (c *RuleCondition) validateShape() error {
	var allowed map[string]bool
	switch c.Kind {
	case RuleMoonPhase:
		allowed = map[string]bool{"moon_id": true, "phase": true}
		if c.MoonID <= 0 {
			return apperror.NewValidation("moon_phase needs a moon_id")
		}
		if _, ok := rulePhaseTargets[c.Phase]; !ok {
			return apperror.NewValidation("phase must be new, first_quarter, full or last_quarter")
		}
	case RuleWeekday:
		// One weekday ("weekday") or several ("weekdays"), not both.
		allowed = map[string]bool{"weekdays": true, "weekday": true}
		switch {
		case c.Weekday != nil && c.Weekdays != nil:
			return apperror.NewValidation("weekday takes weekday or weekdays, not both")
		case c.Weekday != nil:
			if *c.Weekday < 0 {
				return apperror.NewValidation("weekday must be 0 or more")
			}
		default:
			if err := checkIndexList("weekdays", c.Weekdays, 0); err != nil {
				return err
			}
		}
	case RuleNthWeekday:
		allowed = map[string]bool{"n": true, "weekday": true}
		if c.N != ruleLastOfMonth && (c.N < 1 || c.N > maxRuleNth) {
			return apperror.NewValidation("n must be 1 to 5, or -1 for the last")
		}
		if c.Weekday == nil || *c.Weekday < 0 {
			return apperror.NewValidation("nth_weekday needs a weekday")
		}
	case RuleDayOfMonth:
		allowed = map[string]bool{"day": true}
		if c.Day != ruleLastOfMonth && (c.Day < 1 || c.Day > maxCalendarMonthDays) {
			return apperror.NewValidation("day must be a day of the month, or -1 for the last")
		}
	case RuleMonths:
		allowed = map[string]bool{"months": true}
		if err := checkIndexList("months", c.Months, 1); err != nil {
			return err
		}
	case RuleMonth:
		allowed = map[string]bool{"month": true}
		if c.Month < 1 {
			return apperror.NewValidation("month needs a month (1-based)")
		}
	case RuleSeason, RuleSeasonStart:
		allowed = map[string]bool{"season_id": true}
		if c.SeasonID <= 0 {
			return apperror.NewValidation("season needs a season_id")
		}
	case RuleAfterEvent:
		allowed = map[string]bool{"event_id": true, "days": true}
		if c.EventID == "" || len(c.EventID) > 36 {
			return apperror.NewValidation("after_event needs an event_id")
		}
		if c.Days == nil {
			return apperror.NewValidation("after_event needs days (negative for before)")
		}
		if *c.Days < -maxRuleShiftDays || *c.Days > maxRuleShiftDays {
			return apperror.NewValidation(fmt.Sprintf("days must be within %d", maxRuleShiftDays))
		}
	case RuleRelativeToEvent:
		allowed = map[string]bool{"event_id": true}
		if c.EventID == "" || len(c.EventID) > 36 {
			return apperror.NewValidation("relative_to_event needs an event_id")
		}
	default:
		return apperror.NewValidation(fmt.Sprintf("unknown condition kind %q", c.Kind))
	}
	set := map[string]bool{
		"moon_id": c.MoonID != 0, "phase": c.Phase != "", "weekdays": c.Weekdays != nil,
		"n": c.N != 0, "weekday": c.Weekday != nil, "day": c.Day != 0, "months": c.Months != nil, "month": c.Month != 0,
		"season_id": c.SeasonID != 0, "event_id": c.EventID != "", "days": c.Days != nil,
	}
	for field, present := range set {
		if present && !allowed[field] {
			return apperror.NewValidation(fmt.Sprintf("%s does not take %s", c.Kind, field))
		}
	}
	return nil
}

// checkIndexList requires a non-empty, bounded list of indices >= min with
// no repeats.
func checkIndexList(field string, list []int, min int) error {
	if len(list) == 0 {
		return apperror.NewValidation(field + " must list at least one")
	}
	if len(list) > maxRuleListLen {
		return apperror.NewValidation(fmt.Sprintf("%s may list at most %d", field, maxRuleListLen))
	}
	seen := make(map[int]bool, len(list))
	for _, v := range list {
		if v < min {
			return apperror.NewValidation(fmt.Sprintf("%s has an out-of-range entry %d", field, v))
		}
		if seen[v] {
			return apperror.NewValidation(fmt.Sprintf("%s lists %d twice", field, v))
		}
		seen[v] = true
	}
	return nil
}

// validateAgainstCalendar checks the parts of a shape-valid rule that
// depend on the calendar's own structure: weekday and month numbers it
// actually has, and moons and seasons that belong to it. visibleMoon decides
// whether the author may name a moon (a hidden one answers as unknown).
// after_event references are the service's check (they need a query).
func (r *RecurrenceRule) validateAgainstCalendar(cal *Calendar, visibleMoon func(Moon) bool) error {
	wl := cal.WeekLength()
	for i, c := range r.Match {
		bad := func(msg string) error {
			return apperror.NewValidation(fmt.Sprintf("recurrence_rule.match[%d]: %s", i, msg))
		}
		switch c.Kind {
		case RuleMoonPhase:
			found := false
			for _, m := range cal.Moons {
				if m.ID == c.MoonID && visibleMoon(m) {
					found = true
					break
				}
			}
			if !found {
				return bad("moon_id is not a moon of this calendar")
			}
		case RuleWeekday:
			if c.Weekday != nil && *c.Weekday >= wl {
				return bad("weekday is not in this calendar's week")
			}
			for _, w := range c.Weekdays {
				if w >= wl {
					return bad("weekdays has an index this calendar's week does not have")
				}
			}
		case RuleNthWeekday:
			if *c.Weekday >= wl {
				return bad("weekday is not in this calendar's week")
			}
		case RuleMonths:
			for _, m := range c.Months {
				if m > len(cal.Months) {
					return bad("months has a month this calendar does not have")
				}
			}
		case RuleMonth:
			if c.Month > len(cal.Months) {
				return bad("month is not a month of this calendar")
			}
		case RuleSeason, RuleSeasonStart:
			found := false
			for _, s := range cal.Seasons {
				if s.ID == c.SeasonID {
					found = true
					break
				}
			}
			if !found {
				return bad("season_id is not a season of this calendar")
			}
		}
	}
	return nil
}

// ruleColumn scans calendar_events.recurrence_rule. A stored value that no
// longer parses is logged and read as no rule rather than failing the whole
// read: one bad row must not blank a month for everyone.
type ruleColumn struct{ dst **RecurrenceRule }

// Scan implements sql.Scanner.
func (rc ruleColumn) Scan(src any) error {
	*rc.dst = nil
	var raw []byte
	switch v := src.(type) {
	case nil:
		return nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("recurrence_rule: unexpected column type %T", src)
	}
	var r RecurrenceRule
	if err := json.Unmarshal(raw, &r); err != nil {
		slog.Warn("calendar: unreadable recurrence_rule ignored", slog.Any("error", err))
		return nil
	}
	*rc.dst = &r
	return nil
}

// ruleValue is a rule's column value: NULL for none, its JSON otherwise.
func ruleValue(r *RecurrenceRule) driver.Valuer { return ruleValuer{r} }

type ruleValuer struct{ r *RecurrenceRule }

// Value implements driver.Valuer.
func (v ruleValuer) Value() (driver.Value, error) {
	if v.r == nil {
		return nil, nil
	}
	b, err := json.Marshal(v.r)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}
