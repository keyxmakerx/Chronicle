// Package calendar - structure_repository.go is CalendarRepository's
// structure save: everything an owner's structure edit writes, in one
// transaction, so a failure part-way leaves the calendar exactly as it was.
package calendar

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// monthPositionColumns are the columns that store a month by its position
// in the calendar, remapped when months move. Each pair is a
// package-controlled literal, never caller input.
var monthPositionColumns = []struct{ table, column string }{
	{"calendar_events", "month"},
	{"calendar_events", "end_month"},
	{"calendar_events", "recurrence_end_month"},
	{"calendar_eras", "start_month"},
	{"calendar_eras", "end_month"},
}

// queryStructureRows runs query for calendarID and scans each row with scan.
func queryStructureRows[T any](ctx context.Context, ex dbExecutor, query, calendarID string, scan func(interface{ Scan(...any) error }) (*T, error)) ([]T, error) {
	rows, err := ex.QueryContext(ctx, query, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// loadStructureState reads the state a structure save is planned from
// through ex. lock takes the calendar row FOR UPDATE, so inside a
// transaction a second save on the same calendar waits for this one.
func loadStructureState(ctx context.Context, ex dbExecutor, calendarID string, lock bool) (*StructureState, error) {
	q := `SELECT ` + calendarCols + ` FROM calendars WHERE id = ?`
	if lock {
		q += ` FOR UPDATE`
	}
	cal, err := scanCalendar(ex.QueryRowContext(ctx, q, calendarID))
	if err != nil {
		return nil, fmt.Errorf("load calendar: %w", err)
	}
	if cal == nil {
		return nil, apperror.NewNotFound("calendar not found")
	}
	if cal.Months, err = queryStructureRows(ctx, ex,
		`SELECT `+monthCols+` FROM calendar_months WHERE calendar_id = ? ORDER BY sort_order`, calendarID, scanMonth); err != nil {
		return nil, fmt.Errorf("load months: %w", err)
	}
	if cal.Weekdays, err = queryStructureRows(ctx, ex,
		`SELECT `+weekdayCols+` FROM calendar_weekdays WHERE calendar_id = ? ORDER BY sort_order`, calendarID, scanWeekday); err != nil {
		return nil, fmt.Errorf("load weekdays: %w", err)
	}
	if cal.Moons, err = queryStructureRows(ctx, ex,
		`SELECT `+moonCols+` FROM calendar_moons WHERE calendar_id = ? ORDER BY id`, calendarID, scanMoon); err != nil {
		return nil, fmt.Errorf("load moons: %w", err)
	}
	if cal.Seasons, err = queryStructureRows(ctx, ex,
		`SELECT `+seasonCols+` FROM calendar_seasons WHERE calendar_id = ? ORDER BY id`, calendarID, scanSeason); err != nil {
		return nil, fmt.Errorf("load seasons: %w", err)
	}
	if cal.Eras, err = queryStructureRows(ctx, ex,
		`SELECT `+eraCols+` FROM calendar_eras WHERE calendar_id = ? ORDER BY id`, calendarID, scanEra); err != nil {
		return nil, fmt.Errorf("load eras: %w", err)
	}
	events, err := queryStructureRows(ctx, ex,
		`SELECT id, name, year, month, day, end_year, end_month, end_day,
		        recurrence_end_year, recurrence_end_month, recurrence_end_day
		 FROM calendar_events WHERE calendar_id = ? ORDER BY id`, calendarID,
		func(sc interface{ Scan(...any) error }) (*Event, error) {
			var e Event
			err := sc.Scan(&e.ID, &e.Name, &e.Year, &e.Month, &e.Day, &e.EndYear, &e.EndMonth, &e.EndDay,
				&e.RecurrenceEndYear, &e.RecurrenceEndMonth, &e.RecurrenceEndDay)
			return &e, err
		})
	if err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}
	weather, err := queryStructureRows(ctx, ex,
		`SELECT year, month, day FROM calendar_weather_days WHERE calendar_id = ? ORDER BY year, month, day`, calendarID,
		func(sc interface{ Scan(...any) error }) (*DayDate, error) {
			var d DayDate
			err := sc.Scan(&d.Year, &d.Month, &d.Day)
			return &d, err
		})
	if err != nil {
		return nil, fmt.Errorf("load day weather: %w", err)
	}
	// Only a rule event's rule is read: a rule left on an event of another
	// type is dormant and is never expanded.
	rules, err := queryStructureRows(ctx, ex,
		`SELECT id, name, recurrence_rule FROM calendar_events
		 WHERE calendar_id = ? AND recurrence_type = 'rule' AND recurrence_rule IS NOT NULL ORDER BY id`, calendarID,
		func(sc interface{ Scan(...any) error }) (*StructureRule, error) {
			var r StructureRule
			var raw []byte
			err := sc.Scan(&r.EventID, &r.Name, &raw)
			r.Raw = string(raw)
			return &r, err
		})
	if err != nil {
		return nil, fmt.Errorf("load repeat rules: %w", err)
	}
	overrides, err := queryStructureRows(ctx, ex,
		`SELECT o.event_id, o.occurrence_year, o.occurrence_month, o.occurrence_day, o.new_month
		 FROM calendar_event_overrides o JOIN calendar_events e ON e.id = o.event_id
		 WHERE e.calendar_id = ? ORDER BY o.event_id, o.occurrence_year, o.occurrence_month, o.occurrence_day`, calendarID,
		func(sc interface{ Scan(...any) error }) (*StructureOverride, error) {
			var o StructureOverride
			err := sc.Scan(&o.EventID, &o.Year, &o.Month, &o.Day, &o.NewMonth)
			return &o, err
		})
	if err != nil {
		return nil, fmt.Errorf("load overrides: %w", err)
	}
	return &StructureState{Calendar: cal, Events: events, WeatherDays: weather, Rules: rules, Overrides: overrides}, nil
}

// GetStructureState reads the state a structure save is planned from,
// unlocked, for the preview.
func (r *calendarRepo) GetStructureState(ctx context.Context, calendarID string) (*StructureState, error) {
	return loadStructureState(ctx, r.db, calendarID, false)
}

// ApplyStructure plans and writes a structure save in one transaction.
// The calendar row is locked first and the plan is made from state read
// through the same transaction, so two saves on one calendar cannot both
// be planned against the same old state. Months and weekdays are replaced
// outright (nothing stores their row ids). An existing moon keeps its row
// and only its name and cycle change, so its hidden flag, colour, phase
// offset, look and phase rows survive; an existing season likewise keeps
// its colour, description and weather effect. Ids from the input that are
// not this calendar's are inserted fresh, never updated.
func (r *calendarRepo) ApplyStructure(ctx context.Context, calendarID string, plan func(*StructureState) (*StructureWrite, error)) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin structure tx: %w", err)
	}
	defer tx.Rollback()

	st, err := loadStructureState(ctx, tx, calendarID, true)
	if err != nil {
		return err
	}
	w, err := plan(st)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE calendars SET leap_year_every = ?, current_month = ?, current_day = ? WHERE id = ?`,
		w.LeapYearEvery, w.CurrentMonth, w.CurrentDay, calendarID,
	); err != nil {
		return fmt.Errorf("update calendar: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_months WHERE calendar_id = ?`, calendarID); err != nil {
		return fmt.Errorf("delete months: %w", err)
	}
	for _, m := range w.Months {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_months (calendar_id, name, days, sort_order, is_intercalary, leap_year_days)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			calendarID, m.Name, m.Days, m.SortOrder, m.IsIntercalary, m.LeapYearDays,
		); err != nil {
			return fmt.Errorf("insert month %q: %w", m.Name, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_weekdays WHERE calendar_id = ?`, calendarID); err != nil {
		return fmt.Errorf("delete weekdays: %w", err)
	}
	for _, wd := range w.Weekdays {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_weekdays (calendar_id, name, sort_order, is_rest_day) VALUES (?, ?, ?, ?)`,
			calendarID, wd.Name, wd.SortOrder, wd.IsRestDay,
		); err != nil {
			return fmt.Errorf("insert weekday %q: %w", wd.Name, err)
		}
	}

	if err := applyStructureMoons(ctx, tx, calendarID, w.Moons); err != nil {
		return fmt.Errorf("apply moons: %w", err)
	}
	if err := applyStructureSeasons(ctx, tx, calendarID, w.Seasons); err != nil {
		return fmt.Errorf("apply seasons: %w", err)
	}
	if err := remapMonthPositions(ctx, tx, calendarID, w.MonthRemap); err != nil {
		return fmt.Errorf("remap months: %w", err)
	}
	if err := remapWeatherDays(ctx, tx, calendarID, w.MonthRemap); err != nil {
		return fmt.Errorf("remap day weather: %w", err)
	}
	for _, u := range w.RuleUpdates {
		if _, err := tx.ExecContext(ctx,
			`UPDATE calendar_events SET recurrence_rule = ? WHERE id = ? AND calendar_id = ?`,
			u.JSON, u.EventID, calendarID,
		); err != nil {
			return fmt.Errorf("rewrite repeat rule: %w", err)
		}
	}
	if err := remapOverrides(ctx, tx, calendarID, w.MonthRemap); err != nil {
		return fmt.Errorf("remap overrides: %w", err)
	}
	return tx.Commit()
}

// applyStructureMoons is upsertMoons narrowed to what the structure editor
// shows: an existing moon's name and cycle, nothing else.
func applyStructureMoons(ctx context.Context, ex dbExecutor, calendarID string, moons []MoonInput) error {
	existing, err := existingIDs(ctx, ex, "calendar_moons", calendarID)
	if err != nil {
		return err
	}
	keep := make(map[int]bool, len(moons))
	for _, m := range moons {
		if m.ID != nil && existing[*m.ID] {
			if _, err := ex.ExecContext(ctx,
				`UPDATE calendar_moons SET name = ?, cycle_days = ? WHERE id = ? AND calendar_id = ?`,
				m.Name, m.CycleDays, *m.ID, calendarID,
			); err != nil {
				return err
			}
			keep[*m.ID] = true
			continue
		}
		id, err := insertMoon(ctx, ex, calendarID, m)
		if err != nil {
			return err
		}
		keep[id] = true
	}
	return deleteUnkept(ctx, ex, "calendar_moons", calendarID, existing, keep)
}

// applyStructureSeasons upserts seasons by id: an existing season's name
// and dates change, a new one is inserted whole, a missing one is deleted.
func applyStructureSeasons(ctx context.Context, ex dbExecutor, calendarID string, seasons []Season) error {
	existing, err := existingIDs(ctx, ex, "calendar_seasons", calendarID)
	if err != nil {
		return err
	}
	keep := make(map[int]bool, len(seasons))
	for _, s := range seasons {
		if s.ID != 0 && existing[s.ID] {
			if _, err := ex.ExecContext(ctx,
				`UPDATE calendar_seasons SET name = ?, start_month = ?, start_day = ?, end_month = ?, end_day = ?
				 WHERE id = ? AND calendar_id = ?`,
				s.Name, s.StartMonth, s.StartDay, s.EndMonth, s.EndDay, s.ID, calendarID,
			); err != nil {
				return err
			}
			keep[s.ID] = true
			continue
		}
		res, err := ex.ExecContext(ctx,
			`INSERT INTO calendar_seasons (calendar_id, name, start_month, start_day, end_month, end_day, description, color, weather_effect)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			calendarID, s.Name, s.StartMonth, s.StartDay, s.EndMonth, s.EndDay, s.Description, s.Color, s.WeatherEffect,
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		keep[int(id)] = true
	}
	return deleteUnkept(ctx, ex, "calendar_seasons", calendarID, existing, keep)
}

// remapMonthPositions moves every stored month position in remap to its new
// value. One CASE per column reads each row's own old value, so a swap (3
// becomes 1 while 1 becomes 3) never chains through an intermediate value.
func remapMonthPositions(ctx context.Context, ex dbExecutor, calendarID string, remap map[int]int) error {
	if len(remap) == 0 {
		return nil
	}
	olds := make([]int, 0, len(remap))
	for o := range remap {
		olds = append(olds, o)
	}
	sort.Ints(olds)

	for _, pc := range monthPositionColumns {
		var cases strings.Builder
		args := make([]any, 0, len(olds)*3+1)
		for _, o := range olds {
			cases.WriteString(" WHEN ? THEN ?")
			args = append(args, o, remap[o])
		}
		args = append(args, calendarID)
		in := strings.TrimSuffix(strings.Repeat("?,", len(olds)), ",")
		for _, o := range olds {
			args = append(args, o)
		}
		query := `UPDATE ` + pc.table + ` SET ` + pc.column + ` = CASE ` + pc.column + cases.String() + ` END
		          WHERE calendar_id = ? AND ` + pc.column + ` IN (` + in + `)`
		if _, err := ex.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("%s.%s: %w", pc.table, pc.column, err)
		}
	}
	return nil
}

// remapWeatherDays moves day-weather readings with their month. Month is
// part of the table's primary key, so a swap cannot be one UPDATE: moved
// rows first take their target as a negative month (never a real one),
// then flip back. A reading left in a month with no counterpart stays put,
// except where a moved reading now claims its day; there the moved reading
// wins and the left-behind one is deleted, as the preview says.
func remapWeatherDays(ctx context.Context, ex dbExecutor, calendarID string, remap map[int]int) error {
	if len(remap) == 0 {
		return nil
	}
	olds := make([]int, 0, len(remap))
	for o := range remap {
		olds = append(olds, o)
	}
	sort.Ints(olds)
	var cases strings.Builder
	args := make([]any, 0, len(olds)*3+1)
	for _, o := range olds {
		cases.WriteString(" WHEN ? THEN ?")
		args = append(args, o, -remap[o])
	}
	args = append(args, calendarID)
	for _, o := range olds {
		args = append(args, o)
	}
	in := strings.TrimSuffix(strings.Repeat("?,", len(olds)), ",")
	if _, err := ex.ExecContext(ctx,
		`UPDATE calendar_weather_days SET month = CASE month`+cases.String()+` END
		 WHERE calendar_id = ? AND month IN (`+in+`)`, args...); err != nil {
		return fmt.Errorf("park moved readings: %w", err)
	}
	if _, err := ex.ExecContext(ctx,
		`DELETE w FROM calendar_weather_days w
		 JOIN calendar_weather_days n
		   ON n.calendar_id = w.calendar_id AND n.year = w.year AND n.day = w.day AND n.month = -w.month
		 WHERE w.calendar_id = ? AND w.month > 0`, calendarID); err != nil {
		return fmt.Errorf("drop displaced readings: %w", err)
	}
	if _, err := ex.ExecContext(ctx,
		`UPDATE calendar_weather_days SET month = -month WHERE calendar_id = ? AND month < 0`, calendarID); err != nil {
		return fmt.Errorf("place moved readings: %w", err)
	}
	return nil
}

// remapOverrides moves one-off repeat changes with their month. The
// occurrence month is part of the primary key, so it is parked on negative
// months and flipped back exactly like day weather; an override left in a
// month with no counterpart stays put unless a moved one now claims its
// date, where the moved one wins. new_month is not a key column and is one
// plain CASE.
func remapOverrides(ctx context.Context, ex dbExecutor, calendarID string, remap map[int]int) error {
	if len(remap) == 0 {
		return nil
	}
	olds := make([]int, 0, len(remap))
	for o := range remap {
		olds = append(olds, o)
	}
	sort.Ints(olds)
	in := strings.TrimSuffix(strings.Repeat("?,", len(olds)), ",")
	caseFor := func(col string, sign int) (string, []any) {
		var b strings.Builder
		args := make([]any, 0, len(olds)*2)
		for _, o := range olds {
			b.WriteString(" WHEN ? THEN ?")
			args = append(args, o, sign*remap[o])
		}
		return "CASE " + col + b.String() + " END", args
	}
	inArgs := func(args []any) []any {
		args = append(args, calendarID)
		for _, o := range olds {
			args = append(args, o)
		}
		return args
	}

	cs, args := caseFor("o.occurrence_month", -1)
	if _, err := ex.ExecContext(ctx,
		`UPDATE calendar_event_overrides o JOIN calendar_events e ON e.id = o.event_id
		 SET o.occurrence_month = `+cs+`
		 WHERE e.calendar_id = ? AND o.occurrence_month IN (`+in+`)`, inArgs(args)...); err != nil {
		return fmt.Errorf("park moved overrides: %w", err)
	}
	if _, err := ex.ExecContext(ctx,
		`DELETE o FROM calendar_event_overrides o
		 JOIN calendar_events e ON e.id = o.event_id
		 JOIN calendar_event_overrides n
		   ON n.event_id = o.event_id AND n.occurrence_year = o.occurrence_year
		  AND n.occurrence_day = o.occurrence_day AND n.occurrence_month = -o.occurrence_month
		 WHERE e.calendar_id = ? AND o.occurrence_month > 0`, calendarID); err != nil {
		return fmt.Errorf("drop displaced overrides: %w", err)
	}
	if _, err := ex.ExecContext(ctx,
		`UPDATE calendar_event_overrides o JOIN calendar_events e ON e.id = o.event_id
		 SET o.occurrence_month = -o.occurrence_month
		 WHERE e.calendar_id = ? AND o.occurrence_month < 0`, calendarID); err != nil {
		return fmt.Errorf("place moved overrides: %w", err)
	}

	cs, args = caseFor("o.new_month", 1)
	if _, err := ex.ExecContext(ctx,
		`UPDATE calendar_event_overrides o JOIN calendar_events e ON e.id = o.event_id
		 SET o.new_month = `+cs+`
		 WHERE e.calendar_id = ? AND o.new_month IN (`+in+`)`, inArgs(args)...); err != nil {
		return fmt.Errorf("remap move targets: %w", err)
	}
	return nil
}
