package connector

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type undoRepository struct{ db *sql.DB }

// NewUndoRepository creates the SQL behind Undo.
func NewUndoRepository(db *sql.DB) UndoStore { return &undoRepository{db: db} }

func (r *undoRepository) SaveBefores(ctx context.Context, campaignID, userID string, id int64, status string, befores []Before) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return dbError("keeping befores", err)
	}
	defer func() { _ = tx.Rollback() }()
	// The request must be this person's, in this campaign, and still in the
	// status the caller holds it in; FOR UPDATE keeps it from being pruned
	// between the check and the inserts.
	var found int64
	err = tx.QueryRowContext(ctx,
		`SELECT q.id`+ofPerson+` AND q.id = ? AND q.status = ? FOR UPDATE`,
		campaignID, userID, id, status).Scan(&found)
	if err != nil {
		return dbError("keeping befores", err)
	}
	for _, b := range befores {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO ai_request_befores
			   (request_id, campaign_id, kind, target_id, target_name, action, before_json, after_hash, created_at, keep_until)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, campaignID, b.Kind, b.TargetID, clip(b.TargetName, 255), b.Action, b.BeforeJSON, b.AfterHash,
			b.CreatedAt.UTC(), b.KeepUntil.UTC()); err != nil {
			return dbError("keeping befores", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return dbError("keeping befores", err)
	}
	return nil
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func (r *undoRepository) Summaries(ctx context.Context, campaignID, userID string, ids []int64, now time.Time) (map[int64]UndoSummary, error) {
	out := map[int64]UndoSummary{}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := []any{campaignID, userID}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT q.id, q.undone_at`+ofPerson+` AND q.id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, dbError("reading undo state", err)
	}
	for rows.Next() {
		var id int64
		var undone sql.NullTime
		if err := rows.Scan(&id, &undone); err != nil {
			rows.Close()
			return nil, dbError("reading undo state", err)
		}
		s := UndoSummary{}
		if undone.Valid {
			t := undone.Time
			s.UndoneAt = &t
		}
		out[id] = s
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, dbError("reading undo state", err)
	}
	rows.Close()

	// Only the person's own requests found above are asked about, so another
	// person's befores are never read by passing their request id.
	if len(out) == 0 {
		return out, nil
	}
	args = []any{now.UTC(), campaignID}
	marks = strings.TrimSuffix(strings.Repeat("?,", len(out)), ",")
	for id := range out {
		args = append(args, id)
	}
	brows, err := r.db.QueryContext(ctx,
		`SELECT id, request_id, kind, target_id, target_name, action, after_hash
		   FROM ai_request_befores
		  WHERE undone_at IS NULL AND keep_until > ? AND campaign_id = ? AND request_id IN (`+marks+`)
		  ORDER BY id`, args...)
	if err != nil {
		return nil, dbError("reading befores", err)
	}
	defer brows.Close()
	for brows.Next() {
		var b Before
		if err := brows.Scan(&b.ID, &b.RequestID, &b.Kind, &b.TargetID, &b.TargetName, &b.Action, &b.AfterHash); err != nil {
			return nil, dbError("reading befores", err)
		}
		s := out[b.RequestID]
		s.Left = append(s.Left, b)
		out[b.RequestID] = s
	}
	if err := brows.Err(); err != nil {
		return nil, dbError("reading befores", err)
	}
	return out, nil
}

func (r *undoRepository) Claim(ctx context.Context, campaignID, userID string, id int64, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE ai_connection_requests q JOIN ai_connections c ON c.id = q.connection_id
		 SET q.undone_at = ?
		 WHERE q.campaign_id = ? AND c.user_id = ? AND q.id = ? AND q.status = ? AND q.undone_at IS NULL`,
		at.UTC(), campaignID, userID, id, RequestAnswered)
	if err != nil {
		return false, dbError("claiming undo", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, dbError("claiming undo", err)
	}
	return n == 1, nil
}

func (r *undoRepository) Release(ctx context.Context, campaignID, userID string, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE ai_connection_requests q JOIN ai_connections c ON c.id = q.connection_id
		 SET q.undone_at = NULL
		 WHERE q.campaign_id = ? AND c.user_id = ? AND q.id = ?`, campaignID, userID, id)
	if err != nil {
		return dbError("releasing undo", err)
	}
	return nil
}

func (r *undoRepository) Live(ctx context.Context, campaignID, userID string, id int64, now time.Time) ([]Before, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT b.id, b.request_id, b.campaign_id, b.kind, b.target_id, b.target_name, b.action, b.before_json, b.after_hash
		   FROM ai_request_befores b
		   JOIN ai_connection_requests q ON q.id = b.request_id
		   JOIN ai_connections c ON c.id = q.connection_id
		  WHERE b.campaign_id = ? AND c.user_id = ? AND b.request_id = ? AND b.undone_at IS NULL AND b.keep_until > ?
		  ORDER BY b.id DESC`, campaignID, userID, id, now.UTC())
	if err != nil {
		return nil, dbError("reading befores", err)
	}
	defer rows.Close()
	var out []Before
	for rows.Next() {
		var b Before
		var js sql.NullString
		if err := rows.Scan(&b.ID, &b.RequestID, &b.CampaignID, &b.Kind, &b.TargetID, &b.TargetName, &b.Action, &js, &b.AfterHash); err != nil {
			return nil, dbError("reading befores", err)
		}
		if js.Valid {
			v := js.String
			b.BeforeJSON = &v
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError("reading befores", err)
	}
	return out, nil
}

func (r *undoRepository) MarkUndone(ctx context.Context, beforeID int64, userID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE ai_request_befores SET undone_at = ?, undone_by = ? WHERE id = ? AND undone_at IS NULL`,
		at.UTC(), userID, beforeID)
	if err != nil {
		return dbError("marking undone", err)
	}
	return nil
}

func (r *undoRepository) Purge(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM ai_request_befores WHERE keep_until < ?`, now.UTC())
	if err != nil {
		return 0, dbError("purging befores", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
