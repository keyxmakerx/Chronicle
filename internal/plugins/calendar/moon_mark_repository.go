package calendar

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// MoonMarkRepository stores manual moons: each moon's mode and state words
// on calendar_moons, and its marks in calendar_moon_marks. Every write is
// scoped to the moon's calendar, so an id from another calendar changes
// nothing.
type MoonMarkRepository interface {
	// MarksFor returns the marks of the calendar's moons, by moon id, in no
	// particular order (Calendar.indexMoonMarks sorts them).
	MarksFor(ctx context.Context, calendarID string) (map[int][]MoonMark, error)
	// CountMarks is how many marks one moon has.
	CountMarks(ctx context.Context, moonID int) (int, error)
	// SetMode writes a moon's phase mode and state words; false when the
	// moon is not in the calendar.
	SetMode(ctx context.Context, calendarID string, moonID int, mode string, names map[string]string) (bool, error)
	// PutMark stores or replaces the mark on its day.
	PutMark(ctx context.Context, moonID int, mark MoonMark) error
	// DeleteMark removes the mark on a day; false when there was none.
	DeleteMark(ctx context.Context, moonID int, d DayDate) (bool, error)
}

type moonMarkRepo struct{ db *sql.DB }

// NewMoonMarkRepository returns the MariaDB moon mark store.
func NewMoonMarkRepository(db *sql.DB) MoonMarkRepository { return &moonMarkRepo{db: db} }

func (r *moonMarkRepo) MarksFor(ctx context.Context, calendarID string) (map[int][]MoonMark, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT mk.moon_id, mk.year, mk.month, mk.day, mk.state, mk.shown
		 FROM calendar_moon_marks mk
		 JOIN calendar_moons mo ON mo.id = mk.moon_id
		 WHERE mo.calendar_id = ? AND mo.phase_mode = ?`, calendarID, MoonManual)
	if err != nil {
		return nil, fmt.Errorf("list moon marks: %w", err)
	}
	defer rows.Close()
	out := map[int][]MoonMark{}
	for rows.Next() {
		var id int
		var k MoonMark
		if err := rows.Scan(&id, &k.Year, &k.Month, &k.Day, &k.State, &k.Shown); err != nil {
			return nil, fmt.Errorf("scan moon mark: %w", err)
		}
		out[id] = append(out[id], k)
	}
	return out, rows.Err()
}

func (r *moonMarkRepo) CountMarks(ctx context.Context, moonID int) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM calendar_moon_marks WHERE moon_id = ?`, moonID).Scan(&n)
	return n, err
}

func (r *moonMarkRepo) SetMode(ctx context.Context, calendarID string, moonID int, mode string, names map[string]string) (bool, error) {
	var namesJSON any
	if len(names) > 0 {
		b, err := json.Marshal(names)
		if err != nil {
			return false, err
		}
		namesJSON = string(b)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE calendar_moons SET phase_mode = ?, state_names = ? WHERE id = ? AND calendar_id = ?`,
		mode, namesJSON, moonID, calendarID); err != nil {
		return false, fmt.Errorf("set moon mode: %w", err)
	}
	// A DSN without clientFoundRows counts changed rows, so existence is
	// checked on its own rather than read from RowsAffected.
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM calendar_moons WHERE id = ? AND calendar_id = ?)`, moonID, calendarID).Scan(&exists)
	return exists, err
}

func (r *moonMarkRepo) PutMark(ctx context.Context, moonID int, k MoonMark) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_moon_marks (moon_id, year, month, day, state, shown) VALUES (?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE state = VALUES(state), shown = VALUES(shown)`,
		moonID, k.Year, k.Month, k.Day, k.State, k.Shown)
	if err != nil {
		return fmt.Errorf("put moon mark: %w", err)
	}
	return nil
}

func (r *moonMarkRepo) DeleteMark(ctx context.Context, moonID int, d DayDate) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM calendar_moon_marks WHERE moon_id = ? AND year = ? AND month = ? AND day = ?`,
		moonID, d.Year, d.Month, d.Day)
	if err != nil {
		return false, fmt.Errorf("delete moon mark: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
