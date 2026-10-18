package sessions

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// NightEntryRepository stores which Journal note is whose entry for which
// game night (session_night_entries).
type NightEntryRepository interface {
	// InsertNightEntry records an entry; one already recorded for that
	// night and member is left as it is.
	InsertNightEntry(ctx context.Context, row NightEntryRow) error
	// ListNightEntries returns every entry recorded for one night.
	ListNightEntries(ctx context.Context, sessionID, occurrenceDate string) ([]NightEntryRow, error)
	// ListUserNightEntries returns userID's entries on the given sessions.
	ListUserNightEntries(ctx context.Context, userID string, sessionIDs []string) ([]NightEntryRow, error)
	// ListNightEntriesByNote returns the entries among noteIDs that belong
	// to a live game night of the campaign, with the night's name and time.
	ListNightEntriesByNote(ctx context.Context, campaignID string, noteIDs []string) ([]NightEntryNight, error)
}

// NightEntryRow is one recorded entry.
type NightEntryRow struct {
	SessionID string
	Date      string
	UserID    string
	NoteID    string
}

// NightEntryNight is an entry with the night it belongs to.
type NightEntryNight struct {
	NightEntryRow
	Name string
	Time string
	TZ   string
}

type nightEntryRepository struct {
	db *sql.DB
}

// NewNightEntryRepository creates the night entries repository.
func NewNightEntryRepository(db *sql.DB) NightEntryRepository {
	return &nightEntryRepository{db: db}
}

func (r *nightEntryRepository) InsertNightEntry(ctx context.Context, row NightEntryRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT IGNORE INTO session_night_entries (session_id, occurrence_date, user_id, note_id)
		VALUES (?, ?, ?, ?)`, row.SessionID, row.Date, row.UserID, row.NoteID)
	if err != nil {
		return fmt.Errorf("recording night entry: %w", err)
	}
	return nil
}

func (r *nightEntryRepository) ListNightEntries(ctx context.Context, sessionID, occurrenceDate string) ([]NightEntryRow, error) {
	return r.list(ctx, `
		SELECT session_id, DATE_FORMAT(occurrence_date, '%Y-%m-%d'), user_id, note_id
		FROM session_night_entries
		WHERE session_id = ? AND occurrence_date = ?
		ORDER BY created_at, user_id`, sessionID, occurrenceDate)
}

func (r *nightEntryRepository) ListUserNightEntries(ctx context.Context, userID string, sessionIDs []string) ([]NightEntryRow, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	args := []any{userID}
	for _, id := range sessionIDs {
		args = append(args, id)
	}
	return r.list(ctx, `
		SELECT session_id, DATE_FORMAT(occurrence_date, '%Y-%m-%d'), user_id, note_id
		FROM session_night_entries
		WHERE user_id = ? AND session_id IN (`+placeholders(len(sessionIDs))+`)`, args...)
}

func (r *nightEntryRepository) list(ctx context.Context, query string, args ...any) ([]NightEntryRow, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing night entries: %w", err)
	}
	defer rows.Close()
	var out []NightEntryRow
	for rows.Next() {
		var e NightEntryRow
		if err := rows.Scan(&e.SessionID, &e.Date, &e.UserID, &e.NoteID); err != nil {
			return nil, fmt.Errorf("scanning night entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// noteBatch bounds the ids bound in one query; the Journal asks about every
// note the viewer can see.
const noteBatch = 1000

func (r *nightEntryRepository) ListNightEntriesByNote(ctx context.Context, campaignID string, noteIDs []string) ([]NightEntryNight, error) {
	var out []NightEntryNight
	for len(noteIDs) > 0 {
		n := min(len(noteIDs), noteBatch)
		part, err := r.byNote(ctx, campaignID, noteIDs[:n])
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
		noteIDs = noteIDs[n:]
	}
	return out, nil
}

func (r *nightEntryRepository) byNote(ctx context.Context, campaignID string, noteIDs []string) ([]NightEntryNight, error) {
	args := []any{campaignID}
	for _, id := range noteIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.session_id, DATE_FORMAT(e.occurrence_date, '%Y-%m-%d'), e.user_id, e.note_id,
		       s.name, COALESCE(s.scheduled_time, ''), COALESCE(s.scheduled_tz, '')
		FROM session_night_entries e
		JOIN sessions s ON s.id = e.session_id
		WHERE s.campaign_id = ? AND s.deleted_at IS NULL
		  AND e.note_id IN (`+placeholders(len(noteIDs))+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing night entries by note: %w", err)
	}
	defer rows.Close()
	var out []NightEntryNight
	for rows.Next() {
		var e NightEntryNight
		if err := rows.Scan(&e.SessionID, &e.Date, &e.UserID, &e.NoteID, &e.Name, &e.Time, &e.TZ); err != nil {
			return nil, fmt.Errorf("scanning night entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// placeholders is "?, ?, ?" for n values.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}
