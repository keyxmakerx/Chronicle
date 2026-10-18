// Package calendar - event_roll_repository.go persists event rolls: each
// event's roll settings (calendar_event_rolls) and each night's stored
// result (calendar_event_roll_nights). Who may see which night is a service
// rule; this file only reads and writes the stored state.
package calendar

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// EventRollRepository defines persistence for event rolls.
type EventRollRepository interface {
	// GetConfig returns an event's roll, or (nil, nil) when it has none.
	GetConfig(ctx context.Context, eventID string) (*RollConfig, error)
	// ConfigsFor returns the rolls of the given events, by event id; an event
	// with no roll is absent.
	ConfigsFor(ctx context.Context, eventIDs []string) (map[string]*RollConfig, error)
	// SetConfig creates or replaces an event's roll.
	SetConfig(ctx context.Context, eventID string, cfg RollConfig) error
	// DeleteConfig removes an event's roll and every stored night.
	DeleteConfig(ctx context.Context, eventID string) error

	// ListNights returns the stored nights in [from, to], in date order.
	ListNights(ctx context.Context, eventID string, from, to DayDate) ([]RollNight, error)
	// PutNights stores results in one transaction. A locked night is never
	// replaced. With onlyNew, a night that already has a result is kept too,
	// so two readers rolling the same arrived night store one result. A
	// written night follows its result's own "tell players" again.
	PutNights(ctx context.Context, eventID string, nights []RollNight, onlyNew bool) error
	// LockNights sets the lock on stored nights and returns how many changed.
	LockNights(ctx context.Context, eventID string, dates []DayDate, locked bool) (int, error)
	// SetTold sets whether players are told one night (nil: as its result
	// says) and returns how many rows changed.
	SetTold(ctx context.Context, eventID string, date DayDate, told *bool) (int, error)
	// ClearNights deletes the stored nights on the given dates, keeping the
	// locked ones when keepLocked, and returns how many were deleted.
	ClearNights(ctx context.Context, eventID string, dates []DayDate, keepLocked bool) (int, error)
}

type eventRollRepo struct {
	db *sql.DB
}

// NewEventRollRepository creates a MariaDB-backed event roll repository.
func NewEventRollRepository(db *sql.DB) EventRollRepository {
	return &eventRollRepo{db: db}
}

func (r *eventRollRepo) GetConfig(ctx context.Context, eventID string) (*RollConfig, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT config FROM calendar_event_rolls WHERE event_id = ?`, eventID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg RollConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("decode roll for event %s: %w", eventID, err)
	}
	return &cfg, nil
}

func (r *eventRollRepo) ConfigsFor(ctx context.Context, eventIDs []string) (map[string]*RollConfig, error) {
	out := map[string]*RollConfig{}
	if len(eventIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(eventIDs))
	marks := make([]byte, 0, 2*len(eventIDs))
	for i, id := range eventIDs {
		args[i] = id
		if i > 0 {
			marks = append(marks, ',')
		}
		marks = append(marks, '?')
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT event_id, config FROM calendar_event_rolls WHERE event_id IN (`+string(marks)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var cfg RollConfig
		if json.Unmarshal(raw, &cfg) != nil {
			continue // a row that no longer decodes reads as no roll
		}
		out[id] = &cfg
	}
	return out, rows.Err()
}

func (r *eventRollRepo) SetConfig(ctx context.Context, eventID string, cfg RollConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO calendar_event_rolls (event_id, config) VALUES (?, ?)
		 ON DUPLICATE KEY UPDATE config = VALUES(config)`, eventID, string(raw))
	return err
}

func (r *eventRollRepo) DeleteConfig(ctx context.Context, eventID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_event_roll_nights WHERE event_id = ?`, eventID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_event_rolls WHERE event_id = ?`, eventID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *eventRollRepo) ListNights(ctx context.Context, eventID string, from, to DayDate) ([]RollNight, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT year, month, day, total, faces, entry, source, locked, told
		 FROM calendar_event_roll_nights
		 WHERE event_id = ? AND (year, month, day) >= (?, ?, ?) AND (year, month, day) <= (?, ?, ?)
		 ORDER BY year, month, day`,
		eventID, from.Year, from.Month, from.Day, to.Year, to.Month, to.Day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RollNight
	for rows.Next() {
		var n RollNight
		var total sql.NullInt64
		var faces, entry sql.NullString
		var told sql.NullBool
		if err := rows.Scan(&n.Year, &n.Month, &n.Day, &total, &faces, &entry, &n.Source, &n.Locked, &told); err != nil {
			return nil, err
		}
		if total.Valid {
			t := int(total.Int64)
			n.Total = &t
		}
		if faces.Valid {
			n.Faces = decodeFaces(&faces.String)
		}
		n.Entry = entry.String
		if told.Valid {
			b := told.Bool
			n.Told = &b
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *eventRollRepo) PutNights(ctx context.Context, eventID string, nights []RollNight, onlyNew bool) error {
	if len(nights) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	q := `INSERT INTO calendar_event_roll_nights (event_id, year, month, day, total, faces, entry, source, told)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL)`
	if onlyNew {
		q = `INSERT IGNORE INTO calendar_event_roll_nights (event_id, year, month, day, total, faces, entry, source, told)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL)`
	} else {
		// Each assignment reads locked, which is never assigned here, so the
		// order of the SET list does not matter.
		q += ` ON DUPLICATE KEY UPDATE
		   total  = IF(locked, total, VALUES(total)),
		   faces  = IF(locked, faces, VALUES(faces)),
		   entry  = IF(locked, entry, VALUES(entry)),
		   source = IF(locked, source, VALUES(source)),
		   told   = IF(locked, told, NULL)`
	}
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, n := range nights {
		var entry *string
		if n.Entry != "" {
			e := n.Entry
			entry = &e
		}
		if _, err := stmt.ExecContext(ctx, eventID, n.Year, n.Month, n.Day, n.Total, encodeFaces(n.Faces), entry, n.Source); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *eventRollRepo) LockNights(ctx context.Context, eventID string, dates []DayDate, locked bool) (int, error) {
	return r.eachDate(ctx, `UPDATE calendar_event_roll_nights SET locked = ? WHERE event_id = ? AND year = ? AND month = ? AND day = ?`,
		dates, func(d DayDate) []any { return []any{locked, eventID, d.Year, d.Month, d.Day} })
}

func (r *eventRollRepo) SetTold(ctx context.Context, eventID string, date DayDate, told *bool) (int, error) {
	return r.eachDate(ctx, `UPDATE calendar_event_roll_nights SET told = ? WHERE event_id = ? AND year = ? AND month = ? AND day = ?`,
		[]DayDate{date}, func(d DayDate) []any { return []any{told, eventID, d.Year, d.Month, d.Day} })
}

func (r *eventRollRepo) ClearNights(ctx context.Context, eventID string, dates []DayDate, keepLocked bool) (int, error) {
	q := `DELETE FROM calendar_event_roll_nights WHERE event_id = ? AND year = ? AND month = ? AND day = ?`
	if keepLocked {
		q += ` AND locked = FALSE`
	}
	return r.eachDate(ctx, q, dates, func(d DayDate) []any { return []any{eventID, d.Year, d.Month, d.Day} })
}

// eachDate runs one statement per date in a transaction and sums the rows
// it changed.
func (r *eventRollRepo) eachDate(ctx context.Context, q string, dates []DayDate, args func(DayDate) []any) (int, error) {
	if len(dates) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()
	changed := 0
	for _, d := range dates {
		res, err := stmt.ExecContext(ctx, args(d)...)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		changed += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}
