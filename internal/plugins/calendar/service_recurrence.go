// Package calendar - service_recurrence.go holds the service side of events
// that repeat by a rule and of "this one only" skips and moves: checking a
// rule against its calendar, expanding a month's repeating events into
// dates, keeping what a rule refers to from players who may not see it, the
// override writes, and the editor's "next few dates" preview.
//
// Visibility, in one place:
//   - A rule may only name a moon or an anchor event its author can see; a
//     hidden one answers as unknown, so a rule cannot probe for it.
//   - A viewer who can see a rule event sees its dates, even when they are
//     worked out from an anchor that viewer cannot see. The anchor itself is
//     never revealed: a non-author gets the rule stripped from the event
//     whenever it names a hidden moon or an anchor that viewer cannot see.
//   - Skipped occurrences (and a moved one's original date) go only to a
//     viewer who may edit the event; players never learn of them.
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// maxRecurrencePreviewCount bounds the editor's preview list.
const maxRecurrencePreviewCount = 10

// RecurrencePreviewInput is a draft rule to preview: the first Count dates
// it gives from Start, honouring the optional max-occurrences and end date
// exactly as a saved event would.
type RecurrencePreviewInput struct {
	Rule                     json.RawMessage
	Start                    DayDate
	Count                    int
	RecurrenceMaxOccurrences *int
	RecurrenceEnd            *DayDate
}

// RecurrencePreview is the preview's answer. Truncated means the scan hit
// its bound before finding Count dates (a rare rule, or one far away).
type RecurrencePreview struct {
	Dates     []DayDate `json:"dates"`
	Truncated bool      `json:"truncated"`
}

// canEditEvents reports whether v may edit events (Scribe and up), the
// audience that may see skipped occurrences.
func canEditEvents(v permissions.Viewer) bool {
	return v.IsSystem() || v.Role() >= permissions.RoleScribe
}

// loadRuleGeometry loads everything a rule expansion reads: months and
// weekdays, plus every moon and season, unfiltered (a rule on a hidden moon
// still has dates; who may see the rule is redactRuleRefs's concern).
func (s *calendarService) loadRuleGeometry(ctx context.Context, cal *Calendar) error {
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return err
	}
	var err error
	if cal.Moons, err = s.calRepo.GetMoons(ctx, cal.ID); err != nil {
		return fmt.Errorf("load moons: %w", err)
	}
	if cal.Seasons, err = s.calRepo.GetSeasons(ctx, cal.ID); err != nil {
		return fmt.Errorf("load seasons: %w", err)
	}
	return nil
}

// visibleAnchors returns, of the events with the given ids on cal, the ones
// v may see, by id: the same per-event and not-yet-announced gates a single
// event read applies.
func (s *calendarService) visibleAnchors(ctx context.Context, cal *Calendar, campaignID string, ids []string, v permissions.Viewer) (map[string]Event, error) {
	stored, err := s.eventRepo.GetEventsByIDs(ctx, cal.ID, ids)
	if err != nil {
		return nil, fmt.Errorf("get anchor events: %w", err)
	}
	visible := stored[:0]
	for _, e := range stored {
		if e.CalendarID == cal.ID && eventVisibleToViewer(e, v) {
			visible = append(visible, e)
		}
	}
	if !v.SkipsPerUserRules() {
		if visible, err = s.dropUnannouncedFutureEvents(ctx, cal, campaignID, visible); err != nil {
			return nil, err
		}
	}
	out := make(map[string]Event, len(visible))
	for _, e := range visible {
		out[e.ID] = e
	}
	return out, nil
}

// checkRule validates a parsed rule for an event on cal (geometry loaded by
// loadRuleGeometry). selfID is the event being saved, empty on create.
// Anchors must be events on this calendar the author can see, never the
// event itself, and never an event that itself repeats relative to
// another; and an event that rules the author can see repeat relative to
// may not start repeating relative to a third. That one level keeps every
// chain finite.
func (s *calendarService) checkRule(ctx context.Context, cal *Calendar, campaignID, selfID string, rule *RecurrenceRule, author permissions.Viewer) error {
	visibleMoon := func(m Moon) bool { return author.SkipsPerUserRules() || !m.HiddenFromPlayers }
	if err := rule.validateAgainstCalendar(cal, visibleMoon); err != nil {
		return err
	}
	ids := rule.anchorIDs()
	if len(ids) == 0 {
		return nil
	}
	anchors, err := s.visibleAnchors(ctx, cal, campaignID, ids, author)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id == selfID {
			return apperror.NewValidation("an event cannot repeat relative to itself")
		}
		a, ok := anchors[id]
		if !ok {
			return apperror.NewValidation("event_id is not an event of this calendar")
		}
		if a.RecurrenceRule != nil && a.RecurrenceType != nil && *a.RecurrenceType == RecurrenceByRule && a.RecurrenceRule.hasAfterEvent() {
			return apperror.NewValidation(fmt.Sprintf("%q itself repeats relative to another event; repeat relative to that one instead", a.Name))
		}
	}
	if selfID == "" {
		return nil
	}
	// Only dependents the writer can see may refuse the write. Refusing
	// because of a hidden one would tell a Scribe that a Director-only event
	// repeats relative to this one. A hidden dependent is instead left to
	// the one-level guard at read time: its anchor now follows another
	// event, so that condition matches nothing until the Director fixes
	// it, and saving it again is refused with a message naming this anchor.
	ruleEvents, err := s.eventRepo.ListRuleEvents(ctx, cal.ID)
	if err != nil {
		return fmt.Errorf("list rule events: %w", err)
	}
	var dependents []Event
	for _, e := range ruleEvents {
		if e.ID == selfID || e.RecurrenceRule == nil || !e.usesRuleType() {
			continue
		}
		for _, id := range e.RecurrenceRule.anchorIDs() {
			if id == selfID && eventVisibleToViewer(e, author) {
				dependents = append(dependents, e)
				break
			}
		}
	}
	if len(dependents) > 0 && !author.SkipsPerUserRules() {
		if dependents, err = s.dropUnannouncedFutureEvents(ctx, cal, campaignID, dependents); err != nil {
			return err
		}
	}
	if len(dependents) > 0 {
		return apperror.NewValidation("other events repeat relative to this one, so it cannot repeat relative to another event")
	}
	return nil
}

// validateEventRule is the create/update check for an event's merged
// recurrence: a "rule" type needs a rule, and a rule that is new or newly
// in use is checked against the calendar.
func (s *calendarService) validateEventRule(ctx context.Context, calendarID, campaignID, selfID string, evt *Event, recheck bool, author permissions.Viewer) error {
	if evt.RecurrenceType != nil && *evt.RecurrenceType == RecurrenceByRule && evt.RecurrenceRule == nil {
		return apperror.NewValidation("recurrence_type \"rule\" needs a recurrence_rule")
	}
	if evt.RecurrenceRule == nil || !recheck {
		return nil
	}
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return err
	}
	if err := s.loadRuleGeometry(ctx, cal); err != nil {
		return err
	}
	return s.checkRule(ctx, cal, campaignID, selfID, evt.RecurrenceRule, author)
}

// expandMonth narrows a month read's candidates to the events that land in
// (year, month) and fills each repeating event's Occurrences. Anchors and
// overrides are read in one query each, never per event. cal must carry
// Months and Weekdays already.
func (s *calendarService) expandMonth(ctx context.Context, cal *Calendar, events []Event, year, month int, v permissions.Viewer) ([]Event, error) {
	from := DayDate{Year: year, Month: month, Day: 1}
	to := DayDate{Year: year, Month: month, Day: cal.MonthDays(month-1, year)}

	var repeatingIDs, anchorIDs []string
	anyRule := false
	for i := range events {
		e := &events[i]
		if !e.isRepeating() {
			continue
		}
		repeatingIDs = append(repeatingIDs, e.ID)
		if e.usesRule() {
			anyRule = true
			anchorIDs = append(anchorIDs, e.RecurrenceRule.anchorIDs()...)
		}
	}
	if len(repeatingIDs) == 0 {
		return dropDormantRuleEvents(events, from, to), nil
	}
	if anyRule {
		var err error
		if cal.Moons, err = s.calRepo.GetMoons(ctx, cal.ID); err != nil {
			return nil, fmt.Errorf("load moons: %w", err)
		}
		if cal.Seasons, err = s.calRepo.GetSeasons(ctx, cal.ID); err != nil {
			return nil, fmt.Errorf("load seasons: %w", err)
		}
	}
	anchors := map[string]*Event{}
	if len(anchorIDs) > 0 {
		stored, err := s.eventRepo.GetEventsByIDs(ctx, cal.ID, anchorIDs)
		if err != nil {
			return nil, fmt.Errorf("get anchor events: %w", err)
		}
		for i := range stored {
			anchors[stored[i].ID] = &stored[i]
		}
	}
	overrideIDs := append(append([]string{}, repeatingIDs...), anchorIDs...)
	overrides, err := s.eventRepo.ListOverridesForEvents(ctx, overrideIDs)
	if err != nil {
		return nil, fmt.Errorf("list occurrence overrides: %w", err)
	}

	x := newExpander(cal, anchors, overrides)
	editor := canEditEvents(v)
	out := events[:0]
	for _, e := range events {
		if !e.isRepeating() {
			if keepNonRepeating(e, from, to) {
				out = append(out, e)
			}
			continue
		}
		if to.Day < 1 {
			continue // a month the calendar does not have this year
		}
		occ, truncated := x.occurrences(&e, from, to, 0)
		if !editor {
			occ = forPlayers(occ)
		}
		// A truncated event stays, with what was found: its dates are
		// unknown, not absent, and dropping it would hide it silently.
		if len(occ) == 0 && !truncated {
			continue
		}
		if occ == nil {
			occ = []Occurrence{}
		}
		e.Occurrences = occ
		e.OccurrencesTruncated = truncated
		out = append(out, e)
	}
	return out, nil
}

// keepNonRepeating keeps a non-repeating candidate, except a "rule" event
// with no rule, which the repository widens in from every month but which
// only has its own date.
func keepNonRepeating(e Event, from, to DayDate) bool {
	if e.IsRecurring && e.RecurrenceType != nil && *e.RecurrenceType == RecurrenceByRule {
		return DayDate{Year: e.Year, Month: e.Month, Day: e.Day}.inRange(from, to)
	}
	return true
}

// dropDormantRuleEvents applies keepNonRepeating to a list with no
// repeating events in it.
func dropDormantRuleEvents(events []Event, from, to DayDate) []Event {
	out := events[:0]
	for _, e := range events {
		if keepNonRepeating(e, from, to) {
			out = append(out, e)
		}
	}
	return out
}

// forPlayers drops skipped occurrences and a moved one's original date.
func forPlayers(occ []Occurrence) []Occurrence {
	out := occ[:0]
	for _, o := range occ {
		if o.Skipped {
			continue
		}
		o.MovedFrom = nil
		out = append(out, o)
	}
	return out
}

// redactRuleRefs strips the rule from any event whose rule names a moon or
// an anchor event v may not see, so a non-author never learns that a hidden
// moon or event exists. The event's dates are untouched. Authors are never
// redacted. Costs nothing when no event carries such a reference.
func (s *calendarService) redactRuleRefs(ctx context.Context, cal *Calendar, campaignID string, events []Event, v permissions.Viewer) error {
	if v.SkipsPerUserRules() {
		return nil
	}
	var anchorIDs []string
	needMoons := false
	for _, e := range events {
		if e.RecurrenceRule == nil {
			continue
		}
		anchorIDs = append(anchorIDs, e.RecurrenceRule.anchorIDs()...)
		for _, c := range e.RecurrenceRule.Match {
			if c.Kind == RuleMoonPhase {
				needMoons = true
			}
		}
	}
	if len(anchorIDs) == 0 && !needMoons {
		return nil
	}
	shownMoon := map[int]bool{}
	if needMoons {
		moons, err := s.calRepo.GetMoons(ctx, cal.ID)
		if err != nil {
			return fmt.Errorf("load moons: %w", err)
		}
		for _, m := range moons {
			if !m.HiddenFromPlayers {
				shownMoon[m.ID] = true
			}
		}
	}
	var shownAnchor map[string]Event
	if len(anchorIDs) > 0 {
		var err error
		if shownAnchor, err = s.visibleAnchors(ctx, cal, campaignID, anchorIDs, v); err != nil {
			return err
		}
	}
	for i := range events {
		r := events[i].RecurrenceRule
		if r == nil {
			continue
		}
		for _, c := range r.Match {
			hidden := false
			switch c.Kind {
			case RuleMoonPhase:
				hidden = !shownMoon[c.MoonID]
			case RuleAfterEvent, RuleRelativeToEvent:
				_, ok := shownAnchor[c.EventID]
				hidden = !ok
			}
			if hidden {
				events[i].RecurrenceRule = nil
				break
			}
		}
	}
	return nil
}

// SetOccurrenceOverride skips or moves one occurrence of a repeating event.
// The date must be one the event's own repeat actually produces; a move's
// new date must exist in the calendar, differ from the original, and not
// already hold another occurrence of the event. Gated like editing the
// event: the route admits Scribes and up, and the event must be visible to v.
func (s *calendarService) SetOccurrenceOverride(ctx context.Context, eventID, calendarID, campaignID string, occ DayDate, input OccurrenceOverrideInput, v permissions.Viewer) (*OccurrenceOverride, error) {
	cal, evt, x, err := s.overrideContext(ctx, eventID, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	if err := validDate(cal, occ, "occurrence"); err != nil {
		return nil, err
	}
	if nat, _ := x.natural(evt, occ, occ, 1, 0); len(nat) == 0 {
		return nil, apperror.NewValidation("the event does not happen on that date")
	}
	o := OccurrenceOverride{EventID: evt.ID, Year: occ.Year, Month: occ.Month, Day: occ.Day, Action: input.Action}
	switch input.Action {
	case OverrideSkip:
	case OverrideMove:
		target := DayDate{Year: input.Year, Month: input.Month, Day: input.Day}
		if err := validDate(cal, target, "new date"); err != nil {
			return nil, err
		}
		if target == occ {
			return nil, apperror.NewValidation("the new date is the occurrence's own date")
		}
		// Another occurrence already on the target (its own, or one moved
		// there) would double the event up on one day.
		others := x.overrides[evt.ID][:0:0]
		for _, ov := range x.overrides[evt.ID] {
			if ov.date() != occ {
				others = append(others, ov)
			}
		}
		x.overrides[evt.ID] = others
		existing, _ := x.occurrences(evt, target, target, 0)
		for _, e := range existing {
			if !e.Skipped {
				return nil, apperror.NewConflict("the event already happens on the new date")
			}
		}
		o.NewYear, o.NewMonth, o.NewDay = &target.Year, &target.Month, &target.Day
	default:
		return nil, apperror.NewValidation("action must be \"skip\" or \"move\"")
	}
	if err := s.eventRepo.SetOccurrenceOverride(ctx, o); err != nil {
		return nil, fmt.Errorf("set occurrence override: %w", err)
	}
	return &o, nil
}

// DeleteOccurrenceOverride puts one skipped or moved occurrence back. Same
// gate as SetOccurrenceOverride; NotFound when there was nothing to undo.
func (s *calendarService) DeleteOccurrenceOverride(ctx context.Context, eventID, calendarID, campaignID string, occ DayDate, v permissions.Viewer) error {
	if _, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v); err != nil {
		return err
	}
	if _, err := s.eventInCalendarForViewer(ctx, eventID, calendarID, v); err != nil {
		return err
	}
	found, err := s.eventRepo.DeleteOccurrenceOverride(ctx, eventID, occ.Year, occ.Month, occ.Day)
	if err != nil {
		return fmt.Errorf("delete occurrence override: %w", err)
	}
	if !found {
		return apperror.NewNotFound("no change stored for that occurrence")
	}
	return nil
}

// overrideContext resolves the calendar (with rule geometry), the event (a
// visible, repeating one) and an expander loaded with its anchors and
// overrides, for the override writes.
func (s *calendarService) overrideContext(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) (*Calendar, *Event, *expander, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, nil, nil, err
	}
	evt, err := s.eventInCalendarForViewer(ctx, eventID, calendarID, v)
	if err != nil {
		return nil, nil, nil, err
	}
	if !evt.isRepeating() {
		return nil, nil, nil, apperror.NewValidation("only a repeating event has occurrences to skip or move")
	}
	if err := s.loadRuleGeometry(ctx, cal); err != nil {
		return nil, nil, nil, err
	}
	anchors := map[string]*Event{}
	ids := []string{evt.ID}
	if evt.usesRule() {
		if aIDs := evt.RecurrenceRule.anchorIDs(); len(aIDs) > 0 {
			stored, err := s.eventRepo.GetEventsByIDs(ctx, cal.ID, aIDs)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("get anchor events: %w", err)
			}
			for i := range stored {
				anchors[stored[i].ID] = &stored[i]
				ids = append(ids, stored[i].ID)
			}
		}
	}
	overrides, err := s.eventRepo.ListOverridesForEvents(ctx, ids)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list occurrence overrides: %w", err)
	}
	return cal, evt, newExpander(cal, anchors, overrides), nil
}

// validDate refuses a date the calendar does not have.
func validDate(cal *Calendar, d DayDate, what string) error {
	if d.Month < 1 || d.Month > len(cal.Months) || d.Day < 1 || d.Day > cal.MonthDays(d.Month-1, d.Year) {
		return apperror.NewValidation(what + " is not a date in this calendar")
	}
	return nil
}

// PreviewRecurrence lists the first dates a draft rule gives from a start
// date, for the editor's "next five". It writes nothing and is gated like a
// read; what the rule may reference is checked for v exactly as a save
// would be, so a preview cannot probe for a hidden moon or event.
func (s *calendarService) PreviewRecurrence(ctx context.Context, calendarID, campaignID string, input RecurrencePreviewInput, v permissions.Viewer) (*RecurrencePreview, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	rule, err := ParseRecurrenceRule(input.Rule)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, apperror.NewValidation("recurrence_rule is required")
	}
	count := input.Count
	if count <= 0 {
		count = 5
	}
	if count > maxRecurrencePreviewCount {
		count = maxRecurrencePreviewCount
	}
	if err := s.loadRuleGeometry(ctx, cal); err != nil {
		return nil, err
	}
	if err := validDate(cal, input.Start, "start"); err != nil {
		return nil, err
	}
	if input.RecurrenceEnd != nil {
		if err := validDate(cal, *input.RecurrenceEnd, "recurrence end"); err != nil {
			return nil, err
		}
	}
	if err := s.checkRule(ctx, cal, campaignID, "", rule, v); err != nil {
		return nil, err
	}

	byRule := RecurrenceByRule
	draft := &Event{
		ID: "", Year: input.Start.Year, Month: input.Start.Month, Day: input.Start.Day,
		IsRecurring: true, RecurrenceType: &byRule, RecurrenceRule: rule,
		RecurrenceMaxOccurrences: input.RecurrenceMaxOccurrences,
	}
	if e := input.RecurrenceEnd; e != nil {
		draft.RecurrenceEndYear, draft.RecurrenceEndMonth, draft.RecurrenceEndDay = &e.Year, &e.Month, &e.Day
	}
	anchors := map[string]*Event{}
	if ids := rule.anchorIDs(); len(ids) > 0 {
		stored, err := s.eventRepo.GetEventsByIDs(ctx, cal.ID, ids)
		if err != nil {
			return nil, fmt.Errorf("get anchor events: %w", err)
		}
		for i := range stored {
			anchors[stored[i].ID] = &stored[i]
		}
	}
	overrides, err := s.eventRepo.ListOverridesForEvents(ctx, rule.anchorIDs())
	if err != nil {
		return nil, fmt.Errorf("list occurrence overrides: %w", err)
	}
	x := newExpander(cal, anchors, overrides)

	// From the earliest day an occurrence could fall (a negative offset can
	// put one before the start) through the scan bound.
	from := input.Start
	if rule.OffsetDays < 0 {
		if d, ok := shiftDate(cal, from, rule.OffsetDays); ok {
			from = d
		}
	}
	to, ok := shiftDate(cal, input.Start, x.capDays+max(rule.OffsetDays, 0))
	if !ok {
		return &RecurrencePreview{Dates: []DayDate{}}, nil
	}
	dates, truncated := x.natural(draft, from, to, count, 0)
	if dates == nil {
		dates = []DayDate{}
	}
	return &RecurrencePreview{Dates: dates, Truncated: truncated || len(dates) < count && !draftEnds(draft)}, nil
}

// draftEnds reports whether a preview draft stops by itself (a count or an
// end date), so fewer dates than asked for is complete, not cut short.
func draftEnds(e *Event) bool {
	return e.RecurrenceMaxOccurrences != nil || e.RecurrenceEndYear != nil
}

// ListOccurrenceOverrides returns every stored override on a calendar's
// events, by event id, for a system caller (the campaign export). Any other
// viewer is refused, like ListAllEventsForCalendar.
func (s *calendarService) ListOccurrenceOverrides(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (map[string][]OccurrenceOverride, error) {
	if !v.IsSystem() {
		return nil, apperror.NewForbidden("ListOccurrenceOverrides is system-only")
	}
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return nil, err
	}
	events, err := s.eventRepo.ListAllEvents(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	ids := make([]string, 0, len(events))
	for _, e := range events {
		if e.isRepeating() {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return map[string][]OccurrenceOverride{}, nil
	}
	out, err := s.eventRepo.ListOverridesForEvents(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list occurrence overrides: %w", err)
	}
	return out, nil
}

// importedRules carries a calendar-format import's repeat rules over to the
// new calendar: the exporting calendar's moon, season and event ids mapped
// to the re-created ones, and the new calendar's geometry to check each
// rule against. A rule that cannot be carried whole is dropped and its
// event imported as a one-off on its start date, with a warning: a rule
// left pointing at the wrong moon or event would put the event on wrong
// dates without anyone noticing.
type importedRules struct {
	cal     *Calendar // nil when no imported event repeats by a rule
	moons   map[int]int
	seasons map[int]int
	events  map[string]string
}

// newImportedRules maps ir's moon and season refs onto the moons and
// seasons ApplyImport just created on calendarID. They were inserted in
// list order into a fresh calendar, so sorted by id they line up with the
// import's; a count that does not match leaves that map empty.
func (s *calendarService) newImportedRules(ctx context.Context, calendarID string, ir *ImportResult) (*importedRules, error) {
	r := &importedRules{moons: map[int]int{}, seasons: map[int]int{}, events: map[string]string{}}
	anyRule := false
	for _, ee := range ir.Events {
		anyRule = anyRule || exportEventUsesRule(ee)
	}
	if !anyRule {
		return r, nil
	}
	cal, err := s.calRepo.GetByID(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return nil, apperror.NewNotFound("calendar not found")
	}
	if err := s.loadRuleGeometry(ctx, cal); err != nil {
		return nil, err
	}
	r.cal = cal

	moons := append([]Moon(nil), cal.Moons...)
	sort.Slice(moons, func(i, j int) bool { return moons[i].ID < moons[j].ID })
	if len(ir.MoonRefs) == len(ir.Moons) && len(moons) == len(ir.Moons) {
		for i, ref := range ir.MoonRefs {
			if ref != 0 {
				r.moons[ref] = moons[i].ID
			}
		}
	}
	seasons := append([]Season(nil), cal.Seasons...)
	sort.Slice(seasons, func(i, j int) bool { return seasons[i].ID < seasons[j].ID })
	if len(ir.SeasonRefs) == len(ir.Seasons) && len(seasons) == len(ir.Seasons) {
		for i, ref := range ir.SeasonRefs {
			if ref != 0 {
				r.seasons[ref] = seasons[i].ID
			}
		}
	}
	return r, nil
}

// exportEventUsesRule reports whether an imported event repeats by a rule.
func exportEventUsesRule(ee ExportEvent) bool {
	return ee.RecurrenceType != nil && *ee.RecurrenceType == RecurrenceByRule
}

// hasAnchor reports whether an imported event's rule names another event,
// so it must be created after that one.
func (r *importedRules) hasAnchor(ee ExportEvent) bool {
	if !exportEventUsesRule(ee) {
		return false
	}
	rule, err := ParseRecurrenceRule(ee.RecurrenceRule)
	return err == nil && rule != nil && rule.hasAfterEvent()
}

// prepare puts ee's rule on input with its ids remapped, or, when the rule
// is missing, unreadable or names something the import did not recreate,
// turns the event into a one-off.
func (r *importedRules) prepare(input *CreateEventInput, ee ExportEvent, ir *ImportResult) {
	input.RecurrenceRule = nil
	if !exportEventUsesRule(ee) {
		return
	}
	rule, err := ParseRecurrenceRule(ee.RecurrenceRule)
	if err == nil && rule != nil && r.cal != nil {
		if mapped, ok := rule.remapped(r.moons, r.seasons, r.events); ok {
			input.RecurrenceRule, _ = json.Marshal(mapped) // plain structs: cannot fail
			return
		}
	}
	input.IsRecurring, input.RecurrenceType = false, nil
	ir.Warnings = append(ir.Warnings, fmt.Sprintf(
		"event %q repeats by a rule that names something this import could not recreate; imported as a one-off on its start date", ee.Name))
}

// check holds a remapped rule to the calendar's own checks and turns the
// event into a one-off if it fails them. No viewer is involved: every moon
// in the file is the importer's own, and an event_id can only have been
// remapped to an event this import created in its first pass, which never
// itself follows another event, so the anchor checks a save makes already
// hold.
func (r *importedRules) check(evt *Event, ee ExportEvent, ir *ImportResult) {
	if evt.RecurrenceRule == nil {
		return
	}
	if err := evt.RecurrenceRule.validateAgainstCalendar(r.cal, func(Moon) bool { return true }); err != nil {
		evt.IsRecurring, evt.RecurrenceType, evt.RecurrenceRule = false, nil, nil
		ir.Warnings = append(ir.Warnings, fmt.Sprintf(
			"event %q has a repeat rule this calendar cannot use (%s); imported as a one-off on its start date", ee.Name, apperror.SafeMessage(err)))
	}
}

// created records an imported event's new id under its exported one. An
// event that itself follows another is never recorded, so no rule can be
// remapped onto it and a hand-edited file cannot build a chain.
func (r *importedRules) created(ee ExportEvent, evt *Event) {
	if ee.Ref != "" && !r.hasAnchor(ee) {
		r.events[ee.Ref] = evt.ID
	}
}
