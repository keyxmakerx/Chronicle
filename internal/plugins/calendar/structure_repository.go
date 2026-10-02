// Package calendar - structure_repository.go is CalendarRepository's
// structure save: everything an owner's structure edit writes, in one
// transaction, so a failure part-way leaves the calendar exactly as it was.
package calendar

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

// ApplyStructure writes a planned structure save. Months and weekdays are
// replaced outright (nothing stores their row ids). An existing moon keeps
// its row and only its name and cycle change, so its hidden flag, colour,
// phase offset, look and phase rows survive; an existing season likewise
// keeps its colour, description and weather effect. Ids from the input
// that are not this calendar's are inserted fresh, never updated.
func (r *calendarRepo) ApplyStructure(ctx context.Context, calendarID string, w StructureWrite) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin structure tx: %w", err)
	}
	defer tx.Rollback()

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
