package entities

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// errStandingFull is returned by SaveReason when the standing already holds
// MaxStandingReasons; the service turns it into a message for the Director.
var errStandingFull = errors.New("standing holds the most reasons it can")

// StandingReasonWrite is a reason plus the standing it joins; SaveReason
// creates the standing on the first reason.
type StandingReasonWrite struct {
	CampaignID string
	EntityID   string
	SubjectKey string
	Level      int
	Reason     StandingReason
}

// StandingRepository owns the entity_standings, entity_standing_reasons and
// entity_standing_settings tables. It decides nothing about who may see what
// except one thing the SQL itself enforces: a read asked for a player never
// selects a dm_only reason.
type StandingRepository interface {
	// PlayersSee reports the page's switch; a page with none lets players see.
	PlayersSee(ctx context.Context, entityID string) (bool, error)
	SetPlayersSee(ctx context.Context, campaignID, entityID, userID string, see bool) error
	// List returns the page's standings, oldest first, bounded.
	List(ctx context.Context, campaignID, entityID string) ([]StandingRow, error)
	// Find returns one standing, or nil when the subject has none yet.
	Find(ctx context.Context, entityID, subjectKey string) (*StandingRow, error)
	// ReasonsFor returns each standing's newest reasons (at most perStanding)
	// and the count of reasons the caller may see. With includeDMOnly false the
	// DM-only reasons are not selected or counted.
	ReasonsFor(ctx context.Context, standingIDs []string, includeDMOnly bool, perStanding int) (map[string][]StandingReason, map[string]int, error)
	// SaveReason sets the standing's word and adds the reason in one
	// transaction, creating the standing if needed.
	SaveReason(ctx context.Context, w StandingReasonWrite) error
	// DeleteReason removes one reason of that page's subject; false when it
	// was not there.
	DeleteReason(ctx context.Context, entityID, subjectKey, reasonID string) (bool, error)
	// Names returns the live pages of the campaign among ids.
	Names(ctx context.Context, campaignID string, ids []string) (map[string]string, error)
	// Characters returns the campaign's playable characters, by name.
	Characters(ctx context.Context, campaignID string, limit int) ([]StandingSubjectRef, error)
}

type standingRepository struct {
	db *sql.DB
}

// NewStandingRepository creates the MariaDB repository for standings.
func NewStandingRepository(db *sql.DB) StandingRepository {
	return &standingRepository{db: db}
}

func (r *standingRepository) PlayersSee(ctx context.Context, entityID string) (bool, error) {
	var see bool
	err := r.db.QueryRowContext(ctx,
		`SELECT players_see FROM entity_standing_settings WHERE entity_id = ?`, entityID).Scan(&see)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading standing setting: %w", err)
	}
	return see, nil
}

func (r *standingRepository) SetPlayersSee(ctx context.Context, campaignID, entityID, userID string, see bool) error {
	var by any
	if userID != "" {
		by = userID
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO entity_standing_settings (entity_id, campaign_id, players_see, updated_by)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE players_see = VALUES(players_see), updated_by = VALUES(updated_by)`,
		entityID, campaignID, see, by)
	if err != nil {
		return fmt.Errorf("saving standing setting: %w", err)
	}
	return nil
}

const standingSelect = `SELECT id, campaign_id, entity_id, subject_key, level, COALESCE(updated_by, ''), created_at, updated_at
	FROM entity_standings`

func scanStanding(sc interface{ Scan(...any) error }) (StandingRow, error) {
	var s StandingRow
	err := sc.Scan(&s.ID, &s.CampaignID, &s.EntityID, &s.SubjectKey, &s.Level, &s.UpdatedBy, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *standingRepository) List(ctx context.Context, campaignID, entityID string) ([]StandingRow, error) {
	rows, err := r.db.QueryContext(ctx,
		standingSelect+` WHERE entity_id = ? AND campaign_id = ? ORDER BY created_at, id LIMIT ?`,
		entityID, campaignID, maxStandingSubjects)
	if err != nil {
		return nil, fmt.Errorf("listing standings: %w", err)
	}
	defer rows.Close()
	var out []StandingRow
	for rows.Next() {
		s, err := scanStanding(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning standing: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *standingRepository) Find(ctx context.Context, entityID, subjectKey string) (*StandingRow, error) {
	s, err := scanStanding(r.db.QueryRowContext(ctx,
		standingSelect+` WHERE entity_id = ? AND subject_key = ?`, entityID, subjectKey))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding standing: %w", err)
	}
	return &s, nil
}

// inList builds "?,?,?" and its args for an IN clause.
func inList(ids []string) (string, []any) {
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args[i] = id
	}
	return strings.Join(marks, ","), args
}

func (r *standingRepository) ReasonsFor(ctx context.Context, standingIDs []string, includeDMOnly bool, perStanding int) (map[string][]StandingReason, map[string]int, error) {
	reasons := map[string][]StandingReason{}
	counts := map[string]int{}
	if len(standingIDs) == 0 {
		return reasons, counts, nil
	}
	marks, args := inList(standingIDs)
	// The filter is part of the SQL, not applied afterwards, so a player's read
	// never even loads a DM-only reason.
	vis := ` AND dm_only = 0`
	if includeDMOnly {
		vis = ``
	}

	crows, err := r.db.QueryContext(ctx,
		`SELECT standing_id, COUNT(*) FROM entity_standing_reasons WHERE standing_id IN (`+marks+`)`+vis+` GROUP BY standing_id`, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("counting standing reasons: %w", err)
	}
	for crows.Next() {
		var id string
		var n int
		if err := crows.Scan(&id, &n); err != nil {
			crows.Close()
			return nil, nil, fmt.Errorf("scanning reason count: %w", err)
		}
		counts[id] = n
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return nil, nil, err
	}
	crows.Close()

	qargs := append(append([]any{}, args...), perStanding)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, standing_id, direction, reason, when_label, dm_only, COALESCE(created_by, ''), created_at FROM (
		   SELECT r.*, ROW_NUMBER() OVER (PARTITION BY standing_id ORDER BY created_at DESC, id DESC) AS rn
		   FROM entity_standing_reasons r WHERE standing_id IN (`+marks+`)`+strings.ReplaceAll(vis, "dm_only", "r.dm_only")+`
		 ) t WHERE rn <= ? ORDER BY standing_id, created_at DESC, id DESC`, qargs...)
	if err != nil {
		return nil, nil, fmt.Errorf("listing standing reasons: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var x StandingReason
		if err := rows.Scan(&x.ID, &x.StandingID, &x.Direction, &x.Text, &x.When, &x.DMOnly, &x.CreatedBy, &x.CreatedAt); err != nil {
			return nil, nil, fmt.Errorf("scanning standing reason: %w", err)
		}
		reasons[x.StandingID] = append(reasons[x.StandingID], x)
	}
	return reasons, counts, rows.Err()
}

func (r *standingRepository) SaveReason(ctx context.Context, w StandingReasonWrite) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting standing write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var by any
	if w.Reason.CreatedBy != "" {
		by = w.Reason.CreatedBy
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO entity_standings (id, campaign_id, entity_id, subject_key, level, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE level = VALUES(level), updated_by = VALUES(updated_by)`,
		uuid.NewString(), w.CampaignID, w.EntityID, w.SubjectKey, w.Level, by); err != nil {
		return fmt.Errorf("saving standing: %w", err)
	}
	// Lock the standing so two reasons added at once cannot both slip under the cap.
	var standingID string
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM entity_standings WHERE entity_id = ? AND subject_key = ? FOR UPDATE`,
		w.EntityID, w.SubjectKey).Scan(&standingID); err != nil {
		return fmt.Errorf("locking standing: %w", err)
	}
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM entity_standing_reasons WHERE standing_id = ?`, standingID).Scan(&n); err != nil {
		return fmt.Errorf("counting standing reasons: %w", err)
	}
	if n >= MaxStandingReasons {
		return errStandingFull
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO entity_standing_reasons (id, standing_id, direction, reason, when_label, dm_only, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		w.Reason.ID, standingID, w.Reason.Direction, w.Reason.Text, w.Reason.When, w.Reason.DMOnly, by); err != nil {
		return fmt.Errorf("saving standing reason: %w", err)
	}
	return tx.Commit()
}

func (r *standingRepository) DeleteReason(ctx context.Context, entityID, subjectKey, reasonID string) (bool, error) {
	// Joined to the standing so an id from another page or subject deletes nothing.
	res, err := r.db.ExecContext(ctx,
		`DELETE r FROM entity_standing_reasons r
		 INNER JOIN entity_standings s ON s.id = r.standing_id
		 WHERE r.id = ? AND s.entity_id = ? AND s.subject_key = ?`, reasonID, entityID, subjectKey)
	if err != nil {
		return false, fmt.Errorf("removing standing reason: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *standingRepository) Names(ctx context.Context, campaignID string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	marks, args := inList(ids)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name FROM entities WHERE campaign_id = ? AND deleted_at IS NULL AND id IN (`+marks+`)`,
		append([]any{campaignID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("reading standing subjects: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scanning standing subject: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}

func (r *standingRepository) Characters(ctx context.Context, campaignID string, limit int) ([]StandingSubjectRef, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name FROM entities WHERE campaign_id = ? AND deleted_at IS NULL AND playable = 1 ORDER BY name, id LIMIT ?`,
		campaignID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing characters for standing: %w", err)
	}
	defer rows.Close()
	var out []StandingSubjectRef
	for rows.Next() {
		var s StandingSubjectRef
		if err := rows.Scan(&s.Key, &s.Name); err != nil {
			return nil, fmt.Errorf("scanning character: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
