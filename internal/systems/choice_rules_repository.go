package systems

import (
	"context"
	"database/sql"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ChoiceRuleRepository is the data access contract for hero-choice rules and
// the picks waiting on a Director. Every method is scoped by campaign.
type ChoiceRuleRepository interface {
	// List returns the system's rules, for one field when fieldKey is set.
	List(ctx context.Context, campaignID, systemID, fieldKey string) ([]ChoiceRule, error)
	// ListForViewer returns the rules that apply to one viewer: those for
	// everyone and those for userID (every field when fieldKey is "").
	ListForViewer(ctx context.Context, campaignID, systemID, fieldKey, userID string) ([]ChoiceRule, error)
	// Apply writes a batch of marks in one transaction: upserts replace the
	// row for the same (field, entry, target), deletes remove it.
	Apply(ctx context.Context, upserts []ChoiceRule, deletes []ChoiceRule) error
	// CountForEntry counts the rules naming one entry, for the delete warning.
	CountForEntry(ctx context.Context, campaignID, systemID, fieldKey, source, key string) (int, error)
	// DeleteForEntry removes every rule naming an entry, and withdraws the
	// requests waiting on it; it returns how many rules went.
	DeleteForEntry(ctx context.Context, campaignID, systemID, fieldKey, source, key string) (int, error)

	// UpsertWaiting records a waiting request, replacing the same player's
	// waiting request for the same hero and field.
	UpsertWaiting(ctx context.Context, r *ChoiceRequest) error
	// GetRequest returns one request; apperror not-found when absent.
	GetRequest(ctx context.Context, campaignID string, id int64) (*ChoiceRequest, error)
	// ListRequests returns the system's requests in one status, oldest first.
	ListRequests(ctx context.Context, campaignID, systemID, status string) ([]ChoiceRequest, error)
	// UserRequests returns one player's waiting requests and their recently
	// suggested ones for one field, newest first.
	UserRequests(ctx context.Context, campaignID, systemID, userID, fieldKey string, since time.Time) ([]ChoiceRequest, error)
	// HeroWaiting returns the waiting requests one player made for one hero.
	HeroWaiting(ctx context.Context, campaignID, userID, entityID string) ([]ChoiceRequest, error)
	// Close moves a waiting request to a decided status; false when it was
	// no longer waiting.
	Close(ctx context.Context, campaignID string, id int64, status, message, decidedBy string) (bool, error)
}

type choiceRuleRepository struct{ db *sql.DB }

// NewChoiceRuleRepository creates the MariaDB-backed repository.
func NewChoiceRuleRepository(db *sql.DB) ChoiceRuleRepository { return &choiceRuleRepository{db: db} }

const choiceRuleColumns = `id, campaign_id, system_id, field_key, entry_source, entry_key, entry_name,
	target_user_id, state, reason, created_by, created_at, updated_at`

func scanChoiceRule(s rowScanner) (ChoiceRule, error) {
	var r ChoiceRule
	err := s.Scan(&r.ID, &r.CampaignID, &r.SystemID, &r.FieldKey, &r.Source, &r.Key, &r.Name,
		&r.Target, &r.State, &r.Reason, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (r *choiceRuleRepository) queryRules(ctx context.Context, q string, args ...any) ([]ChoiceRule, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []ChoiceRule{}
	for rows.Next() {
		rule, err := scanChoiceRule(rows)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		out = append(out, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (r *choiceRuleRepository) List(ctx context.Context, campaignID, systemID, fieldKey string) ([]ChoiceRule, error) {
	q := `SELECT ` + choiceRuleColumns + ` FROM campaign_choice_rules WHERE campaign_id = ? AND system_id = ?`
	args := []any{campaignID, systemID}
	if fieldKey != "" {
		q += ` AND field_key = ?`
		args = append(args, fieldKey)
	}
	return r.queryRules(ctx, q+` ORDER BY field_key, entry_source, entry_key, target_user_id`, args...)
}

func (r *choiceRuleRepository) ListForViewer(ctx context.Context, campaignID, systemID, fieldKey, userID string) ([]ChoiceRule, error) {
	q := `SELECT ` + choiceRuleColumns + ` FROM campaign_choice_rules
		WHERE campaign_id = ? AND system_id = ? AND target_user_id IN ('', ?)`
	args := []any{campaignID, systemID, userID}
	if fieldKey != "" {
		q += ` AND field_key = ?`
		args = append(args, fieldKey)
	}
	return r.queryRules(ctx, q+` ORDER BY field_key, entry_source, entry_key, target_user_id`, args...)
}

func (r *choiceRuleRepository) Apply(ctx context.Context, upserts []ChoiceRule, deletes []ChoiceRule) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, d := range deletes {
		if _, err := tx.ExecContext(ctx, `DELETE FROM campaign_choice_rules
			WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND entry_source = ? AND entry_key = ? AND target_user_id = ?`,
			d.CampaignID, d.SystemID, d.FieldKey, d.Source, d.Key, d.Target); err != nil {
			return apperror.NewInternal(err)
		}
	}
	for _, u := range upserts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_choice_rules
			(campaign_id, system_id, field_key, entry_source, entry_key, entry_name, target_user_id, state, reason, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE entry_name = VALUES(entry_name), state = VALUES(state), reason = VALUES(reason)`,
			u.CampaignID, u.SystemID, u.FieldKey, u.Source, u.Key, u.Name, u.Target, u.State, u.Reason, u.CreatedBy); err != nil {
			return apperror.NewInternal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (r *choiceRuleRepository) CountForEntry(ctx context.Context, campaignID, systemID, fieldKey, source, key string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_choice_rules
		WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND entry_source = ? AND entry_key = ?`,
		campaignID, systemID, fieldKey, source, key).Scan(&n)
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	return n, nil
}

func (r *choiceRuleRepository) DeleteForEntry(ctx context.Context, campaignID, systemID, fieldKey, source, key string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM campaign_choice_rules
		WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND entry_source = ? AND entry_key = ?`,
		campaignID, systemID, fieldKey, source, key)
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	n, _ := res.RowsAffected()
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_choice_requests SET status = 'withdrawn', decided_at = CURRENT_TIMESTAMP
		WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND entry_source = ? AND entry_key = ? AND status = 'waiting'`,
		campaignID, systemID, fieldKey, source, key); err != nil {
		return 0, apperror.NewInternal(err)
	}
	if err := tx.Commit(); err != nil {
		return 0, apperror.NewInternal(err)
	}
	return int(n), nil
}

const choiceRequestColumns = `id, campaign_id, system_id, field_key, entry_source, entry_key, entry_name,
	user_id, entity_id, entity_name, status, message, decided_by, created_at, decided_at`

func scanChoiceRequest(s rowScanner) (ChoiceRequest, error) {
	var q ChoiceRequest
	var decided sql.NullTime
	err := s.Scan(&q.ID, &q.CampaignID, &q.SystemID, &q.FieldKey, &q.Source, &q.Key, &q.Name,
		&q.UserID, &q.EntityID, &q.EntityName, &q.Status, &q.Message, &q.DecidedBy, &q.CreatedAt, &decided)
	if decided.Valid {
		t := decided.Time
		q.DecidedAt = &t
	}
	return q, err
}

func (r *choiceRuleRepository) queryRequests(ctx context.Context, q string, args ...any) ([]ChoiceRequest, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []ChoiceRequest{}
	for rows.Next() {
		req, err := scanChoiceRequest(rows)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		out = append(out, req)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (r *choiceRuleRepository) UpsertWaiting(ctx context.Context, q *ChoiceRequest) error {
	// Looked up by id under a row lock rather than judged by an UPDATE's
	// affected rows: the driver reports changed rows, so re-sending the same
	// pick within a second would change nothing and insert a duplicate. The
	// locking read also makes a second concurrent pick wait for the first.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM campaign_choice_requests
		WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND user_id = ? AND entity_id = ? AND status = 'waiting'
		ORDER BY id DESC LIMIT 1 FOR UPDATE`,
		q.CampaignID, q.SystemID, q.FieldKey, q.UserID, q.EntityID).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		ins, err := tx.ExecContext(ctx, `INSERT INTO campaign_choice_requests
			(campaign_id, system_id, field_key, entry_source, entry_key, entry_name, user_id, entity_id, entity_name)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			q.CampaignID, q.SystemID, q.FieldKey, q.Source, q.Key, q.Name, q.UserID, q.EntityID, q.EntityName)
		if err != nil {
			return apperror.NewInternal(err)
		}
		id, _ = ins.LastInsertId()
	case err != nil:
		return apperror.NewInternal(err)
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE campaign_choice_requests
			SET entry_source = ?, entry_key = ?, entry_name = ?, entity_name = ?, created_at = CURRENT_TIMESTAMP
			WHERE id = ?`,
			q.Source, q.Key, q.Name, q.EntityName, id); err != nil {
			return apperror.NewInternal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return apperror.NewInternal(err)
	}
	q.ID = id
	q.Status = RequestWaiting
	return nil
}

func (r *choiceRuleRepository) GetRequest(ctx context.Context, campaignID string, id int64) (*ChoiceRequest, error) {
	q, err := scanChoiceRequest(r.db.QueryRowContext(ctx,
		`SELECT `+choiceRequestColumns+` FROM campaign_choice_requests WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("request not found")
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return &q, nil
}

func (r *choiceRuleRepository) ListRequests(ctx context.Context, campaignID, systemID, status string) ([]ChoiceRequest, error) {
	return r.queryRequests(ctx, `SELECT `+choiceRequestColumns+` FROM campaign_choice_requests
		WHERE campaign_id = ? AND system_id = ? AND status = ? ORDER BY created_at, id`, campaignID, systemID, status)
}

func (r *choiceRuleRepository) UserRequests(ctx context.Context, campaignID, systemID, userID, fieldKey string, since time.Time) ([]ChoiceRequest, error) {
	return r.queryRequests(ctx, `SELECT `+choiceRequestColumns+` FROM campaign_choice_requests
		WHERE campaign_id = ? AND system_id = ? AND user_id = ? AND field_key = ?
		  AND (status = 'waiting' OR (status = 'suggested' AND decided_at >= ?))
		ORDER BY id DESC`, campaignID, systemID, userID, fieldKey, since)
}

func (r *choiceRuleRepository) Close(ctx context.Context, campaignID string, id int64, status, message, decidedBy string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE campaign_choice_requests
		SET status = ?, message = ?, decided_by = ?, decided_at = CURRENT_TIMESTAMP
		WHERE campaign_id = ? AND id = ? AND status = 'waiting'`, status, message, decidedBy, campaignID, id)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *choiceRuleRepository) HeroWaiting(ctx context.Context, campaignID, userID, entityID string) ([]ChoiceRequest, error) {
	return r.queryRequests(ctx, `SELECT `+choiceRequestColumns+` FROM campaign_choice_requests
		WHERE campaign_id = ? AND user_id = ? AND entity_id = ? AND status = 'waiting' ORDER BY id`, campaignID, userID, entityID)
}
