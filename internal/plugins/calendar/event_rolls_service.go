// Package calendar - event_rolls_service.go is the rules for event rolls:
// who sees which night, rolling ahead (one night or a whole year), rolling a
// night that arrived unrolled, locks and telling players.
//
// A player learns nothing about a roll until a night they were told about
// arrives, and then only that night's player text. Every write is the
// Director's (owner or granted co-DM), checked here as well as on the route.
package calendar

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// EventRollService is the roll half of the calendar service. It is its own
// interface so CalendarService and its fakes elsewhere stay as they are;
// calendarService implements both and the handler reaches this one by a
// type assertion.
type EventRollService interface {
	// GetEventRoll returns an event's roll and its nights in year. A night
	// that arrived unrolled is rolled first when the roll says so.
	GetEventRoll(ctx context.Context, eventID, calendarID, campaignID string, year int, v permissions.Viewer) (*EventRollView, error)
	// ListMonthRolls returns, by event id, the nights of every rolling event
	// the viewer can see in one month.
	ListMonthRolls(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) (map[string][]RollNightView, error)
	// SetEventRoll creates or replaces an event's roll; nil removes it and
	// every stored night. Returns the roll as GetEventRoll would for year.
	SetEventRoll(ctx context.Context, eventID, calendarID, campaignID string, cfg *RollConfig, year int, v permissions.Viewer) (*EventRollView, error)
	// RollEventNights rolls nights ahead: every night of a year (leaving a
	// past night that has a result), or the given dates. Locked nights keep
	// their result.
	RollEventNights(ctx context.Context, eventID, calendarID, campaignID string, in RollNightsInput, v permissions.Viewer) (*EventRollView, error)
	// LockEventRollNights locks or unlocks stored nights.
	LockEventRollNights(ctx context.Context, eventID, calendarID, campaignID string, dates []DayDate, locked bool, v permissions.Viewer) (int, error)
	// TellEventRollNight says whether players are told one night; nil goes
	// back to what its result says.
	TellEventRollNight(ctx context.Context, eventID, calendarID, campaignID string, date DayDate, told *bool, v permissions.Viewer) error
	// ClearEventRollNights deletes a year's stored nights from today on,
	// keeping the locked ones when keepLocked.
	ClearEventRollNights(ctx context.Context, eventID, calendarID, campaignID string, year int, keepLocked bool, v permissions.Viewer) (int, error)
}

// RollNightsInput says which nights to roll ahead.
type RollNightsInput struct {
	// Year rolls every night of that year, when Dates is empty.
	Year int `json:"year"`
	// Dates rolls just these nights; each must be a night the event lands on.
	Dates []DayDate `json:"dates,omitempty"`
	// OnlyUnrolled keeps a night that already has a result.
	OnlyUnrolled bool `json:"only_unrolled,omitempty"`
}

// RollTable is a rolling table as an event roll reads it.
type RollTable struct {
	Name    string
	Entries []RollTableEntry
}

// RollTableEntry is one entry of a rolling table.
type RollTableEntry struct {
	Name, Brief string
	Weight      float64
}

// RollTableSource finds one of a campaign's rolling tables. The rolltables
// plugin provides it through an adapter wired in internal/app, so this
// plugin never reaches its storage.
type RollTableSource interface {
	// RollTable returns the table, or (nil, nil) when the campaign has none
	// with that id.
	RollTable(ctx context.Context, campaignID, tableID string) (*RollTable, error)
}

// SetEventRollRepository injects event roll storage. Optional wiring like
// SetEntityVisibilityGate: unset, every roll method answers that rolls are
// unavailable rather than failing in some other way.
func (s *calendarService) SetEventRollRepository(r EventRollRepository) { s.rollRepo = r }

// SetRollTableSource injects the rolling-table lookup a table roll needs.
// Unset, a table roll cannot be set or rolled.
func (s *calendarService) SetRollTableSource(t RollTableSource) { s.rollTables = t }

// maxRollYearsPerCall bounds how many years one roll-ahead call may touch,
// since each year is a full expansion of the event's repeat.
const maxRollYearsPerCall = 5

func (s *calendarService) rollsReady() error {
	if s.rollRepo == nil {
		return apperror.NewInternalMessage("event rolls are not available", nil)
	}
	return nil
}

func requireRollDirector(v permissions.Viewer) error {
	if !v.SkipsPerUserRules() {
		return apperror.NewForbidden("only the campaign owner or a granted co-DM may change a roll")
	}
	return nil
}

// rollTarget is a visible event with its calendar's geometry loaded and an
// expander ready for its dates.
type rollTarget struct {
	cal *Calendar
	evt *Event
	x   *expander
}

func (s *calendarService) rollTargetFor(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) (*rollTarget, error) {
	if err := s.rollsReady(); err != nil {
		return nil, err
	}
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	evt, err := s.eventInCalendarForViewer(ctx, eventID, calendarID, v)
	if err != nil {
		return nil, err
	}
	if err := s.loadRuleGeometry(ctx, cal); err != nil {
		return nil, err
	}
	anchors := map[string]*Event{}
	ids := []string{evt.ID}
	if evt.usesRule() {
		if aIDs := evt.RecurrenceRule.anchorIDs(); len(aIDs) > 0 {
			stored, err := s.eventRepo.GetEventsByIDs(ctx, cal.ID, aIDs)
			if err != nil {
				return nil, fmt.Errorf("get anchor events: %w", err)
			}
			for i := range stored {
				anchors[stored[i].ID] = &stored[i]
				ids = append(ids, stored[i].ID)
			}
		}
	}
	overrides, err := s.eventRepo.ListOverridesForEvents(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list occurrence overrides: %w", err)
	}
	return &rollTarget{cal: cal, evt: evt, x: newExpander(cal, anchors, overrides)}, nil
}

// yearSpan is the first and last day of year in cal.
func yearSpan(cal *Calendar, year int) (DayDate, DayDate, bool) {
	n := len(cal.Months)
	if n == 0 {
		return DayDate{}, DayDate{}, false
	}
	return DayDate{Year: year, Month: 1, Day: 1}, DayDate{Year: year, Month: n, Day: cal.MonthDays(n-1, year)}, true
}

// dates returns the nights the event lands on in [from, to], skipped
// occurrences left out and moved ones at their new date.
func (t *rollTarget) dates(from, to DayDate) ([]DayDate, bool) {
	if !t.evt.isRepeating() {
		d := DayDate{Year: t.evt.Year, Month: t.evt.Month, Day: t.evt.Day}
		if keepNonRepeating(*t.evt, from, to) && d.inRange(from, to) {
			return []DayDate{d}, false
		}
		return nil, false
	}
	occ, truncated := t.x.occurrences(t.evt, from, to, 0)
	out := make([]DayDate, 0, len(occ))
	for _, o := range occ {
		if !o.Skipped {
			out = append(out, DayDate{Year: o.Year, Month: o.Month, Day: o.Day})
		}
	}
	return out, truncated
}

func calToday(cal *Calendar) DayDate {
	return DayDate{Year: cal.CurrentYear, Month: cal.CurrentMonth, Day: cal.CurrentDay}
}

// rollOne makes one night's result.
func (s *calendarService) rollOne(ctx context.Context, campaignID string, cfg *RollConfig, d DayDate, source string, table *RollTable) (RollNight, error) {
	n := RollNight{Year: d.Year, Month: d.Month, Day: d.Day, Source: source}
	switch cfg.Source {
	case RollSourceDice:
		expr, err := ParseDice(cfg.Dice)
		if err != nil {
			return n, err
		}
		total, faces, err := expr.Roll()
		if err != nil {
			return n, err
		}
		n.Total, n.Faces = &total, faces
	case RollSourceTable:
		if table == nil {
			var err error
			if table, err = s.loadRollTable(ctx, campaignID, cfg.TableID); err != nil {
				return n, err
			}
		}
		e, err := pickEntry(table.Entries)
		if err != nil {
			return n, err
		}
		n.Entry = e.Name
	}
	return n, nil
}

func (s *calendarService) loadRollTable(ctx context.Context, campaignID, tableID string) (*RollTable, error) {
	if s.rollTables == nil {
		return nil, apperror.NewValidation("rolling tables are not available here")
	}
	t, err := s.rollTables.RollTable(ctx, campaignID, tableID)
	if err != nil {
		return nil, fmt.Errorf("get rolling table: %w", err)
	}
	if t == nil || len(t.Entries) == 0 {
		return nil, apperror.NewValidation("that rolling table is gone or has no entries")
	}
	return t, nil
}

// pickEntry draws one entry by weight, with crypto/rand like a dice roll.
// Weights are scaled to whole thousandths; a weight that rounds to nothing
// still counts as the smallest share, so no entry silently disappears.
func pickEntry(entries []RollTableEntry) (RollTableEntry, error) {
	if len(entries) == 0 {
		return RollTableEntry{}, apperror.NewValidation("that rolling table has no entries")
	}
	shares := make([]int64, len(entries))
	var total int64
	for i, e := range entries {
		w := int64(math.Round(e.Weight * 1000))
		if w < 1 {
			w = 1
		}
		shares[i] = w
		total += w
	}
	r, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return RollTableEntry{}, fmt.Errorf("draw from table: %w", err)
	}
	pick := r.Int64()
	for i, w := range shares {
		if pick < w {
			return entries[i], nil
		}
		pick -= w
	}
	return entries[len(entries)-1], nil
}

// rollArrived rolls, for a roll that rolls on the night, every night in
// dates that has arrived (within rollOnTheNightWindow of today) and has no
// stored result. stored is updated in place. A failure to roll is not the
// reader's problem: the night stays unrolled and the read goes on.
func (s *calendarService) rollArrived(ctx context.Context, cal *Calendar, campaignID, eventID string, cfg *RollConfig, dates []DayDate, stored map[DayDate]RollNight) {
	if !cfg.OnTheNight {
		return
	}
	today := calToday(cal)
	earliest, ok := shiftDate(cal, today, -rollOnTheNightWindow)
	if !ok {
		earliest = today
	}
	var fresh []RollNight
	var table *RollTable
	for _, d := range dates {
		if _, have := stored[d]; have || !d.inRange(earliest, today) {
			continue
		}
		if cfg.Source == RollSourceTable && table == nil {
			t, err := s.loadRollTable(ctx, campaignID, cfg.TableID)
			if err != nil {
				return
			}
			table = t
		}
		n, err := s.rollOne(ctx, campaignID, cfg, d, RollNightOnDay, table)
		if err != nil {
			return
		}
		fresh = append(fresh, n)
	}
	if len(fresh) == 0 {
		return
	}
	if err := s.rollRepo.PutNights(ctx, eventID, fresh, true); err != nil {
		return
	}
	// Re-read rather than trust fresh: another reader may have stored the
	// same night first, and theirs is the one that stands.
	from, to := fresh[0].date(), fresh[len(fresh)-1].date()
	got, err := s.rollRepo.ListNights(ctx, eventID, from, to)
	if err != nil {
		return
	}
	for _, n := range got {
		stored[n.date()] = n
	}
}

// buildRollView shapes an event's year for v.
func (s *calendarService) buildRollView(ctx context.Context, t *rollTarget, campaignID string, cfg *RollConfig, year int, v permissions.Viewer) (*EventRollView, error) {
	out := &EventRollView{EventID: t.evt.ID, Nights: []RollNightView{}}
	director := v.SkipsPerUserRules()
	if cfg == nil && !director {
		return out, nil
	}
	from, to, ok := yearSpan(t.cal, year)
	if !ok {
		return out, nil
	}
	dates, truncated := t.dates(from, to)
	if cfg == nil {
		// An event with no roll yet still lists its nights, so the Director
		// adding one sees which nights it will cover before saving.
		out.Upcoming, out.Truncated = dates, truncated
		return out, nil
	}
	got, err := s.rollRepo.ListNights(ctx, t.evt.ID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list roll nights: %w", err)
	}
	stored := make(map[DayDate]RollNight, len(got))
	for _, n := range got {
		stored[n.date()] = n
	}
	s.rollArrived(ctx, t.cal, campaignID, t.evt.ID, cfg, dates, stored)

	today := calToday(t.cal)
	var briefs map[string]string
	if director && cfg.Source == RollSourceTable && s.rollTables != nil {
		if tb, err := s.rollTables.RollTable(ctx, campaignID, cfg.TableID); err == nil && tb != nil {
			briefs = make(map[string]string, len(tb.Entries))
			for _, e := range tb.Entries {
				briefs[e.Name] = e.Brief
			}
		}
	}
	for _, d := range dates {
		n, have := stored[d]
		if !have {
			if director {
				out.Upcoming = append(out.Upcoming, d)
			}
			continue
		}
		if !director && today.before(d) {
			continue
		}
		nv, ok := cfg.view(n, director)
		if !ok {
			continue
		}
		if briefs != nil && n.Entry != "" {
			nv.Director = briefs[n.Entry]
		}
		out.Nights = append(out.Nights, nv)
	}
	if director {
		c := *cfg
		out.Config = &c
		out.Truncated = truncated
	}
	return out, nil
}

// GetEventRoll: see EventRollService.
func (s *calendarService) GetEventRoll(ctx context.Context, eventID, calendarID, campaignID string, year int, v permissions.Viewer) (*EventRollView, error) {
	t, err := s.rollTargetFor(ctx, eventID, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	cfg, err := s.rollRepo.GetConfig(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("get roll: %w", err)
	}
	return s.buildRollView(ctx, t, campaignID, cfg, year, v)
}

// ListMonthRolls: see EventRollService. It reads the month exactly as the
// calendar does, so a roll's nights reach only a viewer who can see the
// event on those dates.
func (s *calendarService) ListMonthRolls(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) (map[string][]RollNightView, error) {
	if err := s.rollsReady(); err != nil {
		return nil, err
	}
	events, err := s.ListEventsForMonth(ctx, calendarID, campaignID, year, month, v)
	if err != nil {
		return nil, err
	}
	out := map[string][]RollNightView{}
	ids := make([]string, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}
	cfgs, err := s.rollRepo.ConfigsFor(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list rolls: %w", err)
	}
	if len(cfgs) == 0 {
		return out, nil
	}
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return nil, err
	}
	if month < 1 || month > len(cal.Months) {
		return out, nil
	}
	from := DayDate{Year: year, Month: month, Day: 1}
	to := DayDate{Year: year, Month: month, Day: cal.MonthDays(month-1, year)}
	director := v.SkipsPerUserRules()
	today := calToday(cal)
	for _, e := range events {
		cfg := cfgs[e.ID]
		if cfg == nil {
			continue
		}
		var dates []DayDate
		if e.hasExpansion() {
			for _, o := range e.Occurrences {
				if !o.Skipped {
					dates = append(dates, DayDate{Year: o.Year, Month: o.Month, Day: o.Day})
				}
			}
		} else if d := (DayDate{Year: e.Year, Month: e.Month, Day: e.Day}); d.inRange(from, to) {
			dates = []DayDate{d}
		}
		if len(dates) == 0 {
			continue
		}
		got, err := s.rollRepo.ListNights(ctx, e.ID, from, to)
		if err != nil {
			return nil, fmt.Errorf("list roll nights: %w", err)
		}
		stored := make(map[DayDate]RollNight, len(got))
		for _, n := range got {
			stored[n.date()] = n
		}
		s.rollArrived(ctx, cal, campaignID, e.ID, cfg, dates, stored)
		var views []RollNightView
		for _, d := range dates {
			n, have := stored[d]
			if !have || (!director && today.before(d)) {
				continue
			}
			if nv, ok := cfg.view(n, director); ok {
				views = append(views, nv)
			}
		}
		if len(views) > 0 {
			out[e.ID] = views
		}
	}
	return out, nil
}

// SetEventRoll: see EventRollService.
func (s *calendarService) SetEventRoll(ctx context.Context, eventID, calendarID, campaignID string, cfg *RollConfig, year int, v permissions.Viewer) (*EventRollView, error) {
	if err := requireRollDirector(v); err != nil {
		return nil, err
	}
	t, err := s.rollTargetFor(ctx, eventID, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		if err := s.rollRepo.DeleteConfig(ctx, eventID); err != nil {
			return nil, fmt.Errorf("delete roll: %w", err)
		}
		return s.buildRollView(ctx, t, campaignID, nil, year, v)
	}
	if err := cfg.Normalize(); err != nil {
		return nil, err
	}
	if cfg.Source == RollSourceTable {
		if _, err := s.loadRollTable(ctx, campaignID, cfg.TableID); err != nil {
			return nil, err
		}
	}
	if err := s.rollRepo.SetConfig(ctx, eventID, *cfg); err != nil {
		return nil, fmt.Errorf("set roll: %w", err)
	}
	return s.buildRollView(ctx, t, campaignID, cfg, year, v)
}

// RollEventNights: see EventRollService.
func (s *calendarService) RollEventNights(ctx context.Context, eventID, calendarID, campaignID string, in RollNightsInput, v permissions.Viewer) (*EventRollView, error) {
	if err := requireRollDirector(v); err != nil {
		return nil, err
	}
	t, err := s.rollTargetFor(ctx, eventID, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	cfg, err := s.rollRepo.GetConfig(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("get roll: %w", err)
	}
	if cfg == nil {
		return nil, apperror.NewValidation("this event has no roll yet")
	}
	if len(in.Dates) > maxRollNightsPerCall {
		return nil, apperror.NewValidation(fmt.Sprintf("roll at most %d nights at once", maxRollNightsPerCall))
	}

	// The nights to roll, each checked against the event's own dates.
	years := map[int]bool{}
	if len(in.Dates) == 0 {
		years[in.Year] = true
	}
	for _, d := range in.Dates {
		years[d.Year] = true
	}
	if len(years) > maxRollYearsPerCall {
		return nil, apperror.NewValidation(fmt.Sprintf("roll nights in at most %d years at once", maxRollYearsPerCall))
	}
	valid := map[DayDate]bool{}
	var all []DayDate
	for y := range years {
		from, to, ok := yearSpan(t.cal, y)
		if !ok {
			return nil, apperror.NewValidation("this calendar has no months")
		}
		ds, _ := t.dates(from, to)
		for _, d := range ds {
			valid[d] = true
			all = append(all, d)
		}
	}
	targets := all
	if len(in.Dates) == 0 {
		// A whole-year roll leaves a night that has already happened alone
		// once it has a result: players may have been told it.
		from, to, _ := yearSpan(t.cal, in.Year)
		got, err := s.rollRepo.ListNights(ctx, eventID, from, to)
		if err != nil {
			return nil, fmt.Errorf("list roll nights: %w", err)
		}
		have := make(map[DayDate]bool, len(got))
		for _, n := range got {
			have[n.date()] = true
		}
		today := calToday(t.cal)
		targets = targets[:0:0]
		for _, d := range all {
			if !d.before(today) || !have[d] {
				targets = append(targets, d)
			}
		}
	}
	if len(in.Dates) > 0 {
		targets = targets[:0:0]
		for _, d := range in.Dates {
			if !valid[d] {
				return nil, apperror.NewValidation(fmt.Sprintf("the event does not land on %d-%d-%d", d.Year, d.Month, d.Day))
			}
			targets = append(targets, d)
		}
		// Rolling a named night that is locked is refused rather than
		// skipped, so the caller is not told it was rolled.
		for _, d := range targets {
			got, err := s.rollRepo.ListNights(ctx, eventID, d, d)
			if err != nil {
				return nil, fmt.Errorf("list roll nights: %w", err)
			}
			if len(got) > 0 && got[0].Locked {
				return nil, apperror.NewValidation("that night is locked; unlock it to roll it again")
			}
		}
	}

	var table *RollTable
	if cfg.Source == RollSourceTable {
		if table, err = s.loadRollTable(ctx, campaignID, cfg.TableID); err != nil {
			return nil, err
		}
	}
	nights := make([]RollNight, 0, len(targets))
	for _, d := range targets {
		n, err := s.rollOne(ctx, campaignID, cfg, d, RollNightAhead, table)
		if err != nil {
			return nil, err
		}
		nights = append(nights, n)
	}
	if err := s.rollRepo.PutNights(ctx, eventID, nights, in.OnlyUnrolled); err != nil {
		return nil, fmt.Errorf("store roll nights: %w", err)
	}
	year := in.Year
	if len(in.Dates) > 0 {
		year = in.Dates[0].Year
	}
	return s.buildRollView(ctx, t, campaignID, cfg, year, v)
}

// LockEventRollNights: see EventRollService.
func (s *calendarService) LockEventRollNights(ctx context.Context, eventID, calendarID, campaignID string, dates []DayDate, locked bool, v permissions.Viewer) (int, error) {
	if err := requireRollDirector(v); err != nil {
		return 0, err
	}
	if len(dates) > maxRollNightsPerCall {
		return 0, apperror.NewValidation(fmt.Sprintf("lock at most %d nights at once", maxRollNightsPerCall))
	}
	if _, err := s.rollTargetFor(ctx, eventID, calendarID, campaignID, v); err != nil {
		return 0, err
	}
	n, err := s.rollRepo.LockNights(ctx, eventID, dates, locked)
	if err != nil {
		return 0, fmt.Errorf("lock roll nights: %w", err)
	}
	return n, nil
}

// TellEventRollNight: see EventRollService.
func (s *calendarService) TellEventRollNight(ctx context.Context, eventID, calendarID, campaignID string, date DayDate, told *bool, v permissions.Viewer) error {
	if err := requireRollDirector(v); err != nil {
		return err
	}
	if _, err := s.rollTargetFor(ctx, eventID, calendarID, campaignID, v); err != nil {
		return err
	}
	if _, err := s.rollRepo.SetTold(ctx, eventID, date, told); err != nil {
		return fmt.Errorf("tell roll night: %w", err)
	}
	return nil
}

// ClearEventRollNights: see EventRollService.
func (s *calendarService) ClearEventRollNights(ctx context.Context, eventID, calendarID, campaignID string, year int, keepLocked bool, v permissions.Viewer) (int, error) {
	if err := requireRollDirector(v); err != nil {
		return 0, err
	}
	t, err := s.rollTargetFor(ctx, eventID, calendarID, campaignID, v)
	if err != nil {
		return 0, err
	}
	from, to, ok := yearSpan(t.cal, year)
	if !ok {
		return 0, nil
	}
	got, err := s.rollRepo.ListNights(ctx, eventID, from, to)
	if err != nil {
		return 0, fmt.Errorf("list roll nights: %w", err)
	}
	// Nights that have already happened stay: players may have been told.
	today := calToday(t.cal)
	dates := make([]DayDate, 0, len(got))
	for _, n := range got {
		if !n.date().before(today) {
			dates = append(dates, n.date())
		}
	}
	n, err := s.rollRepo.ClearNights(ctx, eventID, dates, keepLocked)
	if err != nil {
		return 0, fmt.Errorf("clear roll nights: %w", err)
	}
	return n, nil
}
