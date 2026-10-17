package sessions

import (
	"context"
	"database/sql"
	"fmt"
)

// SightingRepository stores per-night sightings (session_night_sightings) and
// reads the session links a page's Last seen merges with them.
type SightingRepository interface {
	// UpsertSighting writes one night's answer for one page; the key is
	// (session, occurrence date, entity), so answering again replaces it.
	UpsertSighting(ctx context.Context, s SightingRow) error
	DeleteSighting(ctx context.Context, sessionID, occurrenceDate, entityID string) error
	// ListSightingsForEntity returns the page's rows on live sessions of the
	// campaign.
	ListSightingsForEntity(ctx context.Context, campaignID, entityID string) ([]SightingRow, error)
	// ListLinksForEntity returns the page's session_entities links on live
	// sessions of the campaign, with each session's date and recurrence.
	ListLinksForEntity(ctx context.Context, campaignID, entityID string) ([]SightingLink, error)
	// ListSightingsForNight returns every answer stored for one night, as
	// entity id to state.
	ListSightingsForNight(ctx context.Context, sessionID, occurrenceDate string) (map[string]string, error)
}

// SightingRow is one stored answer, with its session's name.
type SightingRow struct {
	SessionID      string
	SessionName    string
	OccurrenceDate string
	EntityID       string
	State          string
	Source         string
	AddedBy        string
}

// SightingLink is one session_entities link on a live session.
type SightingLink struct {
	SessionID     string
	SessionName   string
	Role          string
	ScheduledDate string
	IsRecurring   bool
}

type sightingRepository struct {
	db *sql.DB
}

// NewSightingRepository creates the sightings repository.
func NewSightingRepository(db *sql.DB) SightingRepository {
	return &sightingRepository{db: db}
}

func (r *sightingRepository) UpsertSighting(ctx context.Context, s SightingRow) error {
	var addedBy any
	if s.AddedBy != "" {
		addedBy = s.AddedBy
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO session_night_sightings (session_id, occurrence_date, entity_id, state, source, added_by)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE state = VALUES(state), source = VALUES(source), added_by = VALUES(added_by)`,
		s.SessionID, s.OccurrenceDate, s.EntityID, s.State, s.Source, addedBy)
	if err != nil {
		return fmt.Errorf("upserting sighting: %w", err)
	}
	return nil
}

func (r *sightingRepository) DeleteSighting(ctx context.Context, sessionID, occurrenceDate, entityID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM session_night_sightings WHERE session_id = ? AND occurrence_date = ? AND entity_id = ?`,
		sessionID, occurrenceDate, entityID)
	if err != nil {
		return fmt.Errorf("deleting sighting: %w", err)
	}
	return nil
}

func (r *sightingRepository) ListSightingsForEntity(ctx context.Context, campaignID, entityID string) ([]SightingRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT ns.session_id, s.name, DATE_FORMAT(ns.occurrence_date, '%Y-%m-%d'), ns.entity_id,
		       ns.state, ns.source, COALESCE(ns.added_by, '')
		FROM session_night_sightings ns
		INNER JOIN sessions s ON s.id = ns.session_id AND s.deleted_at IS NULL
		WHERE s.campaign_id = ? AND ns.entity_id = ?`,
		campaignID, entityID)
	if err != nil {
		return nil, fmt.Errorf("listing sightings: %w", err)
	}
	defer rows.Close()
	var out []SightingRow
	for rows.Next() {
		var s SightingRow
		if err := rows.Scan(&s.SessionID, &s.SessionName, &s.OccurrenceDate, &s.EntityID, &s.State, &s.Source, &s.AddedBy); err != nil {
			return nil, fmt.Errorf("scanning sighting: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *sightingRepository) ListLinksForEntity(ctx context.Context, campaignID, entityID string) ([]SightingLink, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT se.session_id, s.name, se.role, COALESCE(DATE_FORMAT(s.scheduled_date, '%Y-%m-%d'), ''), s.is_recurring
		FROM session_entities se
		INNER JOIN sessions s ON s.id = se.session_id AND s.deleted_at IS NULL
		WHERE s.campaign_id = ? AND se.entity_id = ?`,
		campaignID, entityID)
	if err != nil {
		return nil, fmt.Errorf("listing session links: %w", err)
	}
	defer rows.Close()
	var out []SightingLink
	for rows.Next() {
		var l SightingLink
		if err := rows.Scan(&l.SessionID, &l.SessionName, &l.Role, &l.ScheduledDate, &l.IsRecurring); err != nil {
			return nil, fmt.Errorf("scanning session link: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *sightingRepository) ListSightingsForNight(ctx context.Context, sessionID, occurrenceDate string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT entity_id, state FROM session_night_sightings WHERE session_id = ? AND occurrence_date = ?`,
		sessionID, occurrenceDate)
	if err != nil {
		return nil, fmt.Errorf("listing a night's sightings: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, state string
		if err := rows.Scan(&id, &state); err != nil {
			return nil, fmt.Errorf("scanning a night's sighting: %w", err)
		}
		out[id] = state
	}
	return out, rows.Err()
}
