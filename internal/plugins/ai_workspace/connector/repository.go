package connector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

// Repository is the SQL behind connections and their call log. Every
// statement that reads or changes a campaign's rows is scoped by campaign_id.
type Repository interface {
	// Replace revokes the campaign's connections and stores c in their
	// place, in one transaction holding the campaign's row, so two at once
	// can't both stay live. It returns c's id.
	Replace(ctx context.Context, c *Connection) (int64, error)
	// RevokeAll revokes every connection of the campaign that is not revoked.
	RevokeAll(ctx context.Context, campaignID string, at time.Time) error
	// Current is the campaign's newest connection that is not revoked (it
	// may have expired), or nil.
	Current(ctx context.Context, campaignID string) (*Connection, error)
	// FindByHash is the connection with this key hash whose maker's account
	// is not disabled, or nil. Revoked and expired keys are returned too;
	// the service decides.
	FindByHash(ctx context.Context, keyHash string) (*Connection, error)
	// MarkUsed stamps last_used_at, and the client's name when given.
	MarkUsed(ctx context.Context, id int64, at time.Time, clientName string) error
	// AddCall logs one tool call.
	AddCall(ctx context.Context, call *Call) error
	// RecentCalls lists the campaign's newest calls first.
	RecentCalls(ctx context.Context, campaignID string, limit int) ([]Call, error)
	// PruneCalls keeps the campaign's newest keep calls.
	PruneCalls(ctx context.Context, campaignID string, keep int) error
}

type repository struct{ db *sql.DB }

// NewRepository creates the connector's repository.
func NewRepository(db *sql.DB) Repository { return &repository{db: db} }

func dbError(what string, err error) error {
	return apperror.NewInternal(fmt.Errorf("ai connector: %s: %w", what, err))
}

const connectionColumns = `c.id, c.campaign_id, c.user_id, c.key_hash, c.key_hint, c.privacy,
	c.client_name, c.created_at, c.expires_at, c.last_used_at, c.revoked_at`

func scanConnection(row interface{ Scan(...any) error }) (*Connection, error) {
	var c Connection
	var privacy string
	var lastUsed, revoked sql.NullTime
	if err := row.Scan(&c.ID, &c.CampaignID, &c.UserID, &c.KeyHash, &c.KeyHint, &privacy,
		&c.ClientName, &c.CreatedAt, &c.ExpiresAt, &lastUsed, &revoked); err != nil {
		return nil, err
	}
	c.Privacy = parsePrivacy(privacy)
	if lastUsed.Valid {
		t := lastUsed.Time
		c.LastUsedAt = &t
	}
	if revoked.Valid {
		t := revoked.Time
		c.RevokedAt = &t
	}
	return &c, nil
}

// parsePrivacy reads a stored mode. Anything unknown reads as Safe, so a bad
// row can only narrow what the AI sees.
func parsePrivacy(s string) aiexport.PrivacyMode {
	if s == aiexport.PrivacyModePermitted.String() {
		return aiexport.PrivacyModePermitted
	}
	return aiexport.PrivacyModeSafe
}

func (r *repository) Replace(ctx context.Context, c *Connection) (id int64, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, dbError("starting connection change", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var locked string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM campaigns WHERE id = ? FOR UPDATE`, c.CampaignID).Scan(&locked); err != nil {
		return 0, dbError("locking campaign", err)
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE ai_connections SET revoked_at = ? WHERE campaign_id = ? AND revoked_at IS NULL`,
		c.CreatedAt.UTC(), c.CampaignID); err != nil {
		return 0, dbError("revoking connections", err)
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO ai_connections (campaign_id, user_id, key_hash, key_hint, privacy, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.CampaignID, c.UserID, c.KeyHash, c.KeyHint, c.Privacy.String(), c.CreatedAt.UTC(), c.ExpiresAt.UTC())
	if err != nil {
		return 0, dbError("creating connection", err)
	}
	if id, err = res.LastInsertId(); err != nil {
		return 0, dbError("reading connection id", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, dbError("saving connection", err)
	}
	return id, nil
}

func (r *repository) RevokeAll(ctx context.Context, campaignID string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE ai_connections SET revoked_at = ? WHERE campaign_id = ? AND revoked_at IS NULL`,
		at.UTC(), campaignID); err != nil {
		return dbError("revoking connections", err)
	}
	return nil
}

func (r *repository) Current(ctx context.Context, campaignID string) (*Connection, error) {
	c, err := scanConnection(r.db.QueryRowContext(ctx,
		`SELECT `+connectionColumns+` FROM ai_connections c
		 WHERE c.campaign_id = ? AND c.revoked_at IS NULL
		 ORDER BY c.id DESC LIMIT 1`, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, dbError("reading connection", err)
	}
	return c, nil
}

func (r *repository) FindByHash(ctx context.Context, keyHash string) (*Connection, error) {
	c, err := scanConnection(r.db.QueryRowContext(ctx,
		`SELECT `+connectionColumns+` FROM ai_connections c
		 WHERE c.key_hash = ?
		   AND EXISTS (SELECT 1 FROM users u WHERE u.id = c.user_id AND u.is_disabled = FALSE)`, keyHash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, dbError("finding connection", err)
	}
	return c, nil
}

func (r *repository) MarkUsed(ctx context.Context, id int64, at time.Time, clientName string) error {
	var err error
	if clientName != "" {
		_, err = r.db.ExecContext(ctx,
			`UPDATE ai_connections SET last_used_at = ?, client_name = ? WHERE id = ?`, at.UTC(), clientName, id)
	} else {
		_, err = r.db.ExecContext(ctx, `UPDATE ai_connections SET last_used_at = ? WHERE id = ?`, at.UTC(), id)
	}
	if err != nil {
		return dbError("marking connection used", err)
	}
	return nil
}

func (r *repository) AddCall(ctx context.Context, call *Call) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO ai_connection_calls (connection_id, campaign_id, called_at, tool, summary, note, ok)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		call.ConnectionID, call.CampaignID, call.At.UTC(), call.Tool, call.Summary, call.Note, call.OK); err != nil {
		return dbError("logging call", err)
	}
	return nil
}

func (r *repository) RecentCalls(ctx context.Context, campaignID string, limit int) ([]Call, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, connection_id, campaign_id, called_at, tool, summary, note, ok
		 FROM ai_connection_calls WHERE campaign_id = ? ORDER BY id DESC LIMIT ?`, campaignID, limit)
	if err != nil {
		return nil, dbError("listing calls", err)
	}
	defer rows.Close()
	var out []Call
	for rows.Next() {
		var c Call
		if err := rows.Scan(&c.ID, &c.ConnectionID, &c.CampaignID, &c.At, &c.Tool, &c.Summary, &c.Note, &c.OK); err != nil {
			return nil, dbError("reading call", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError("listing calls", err)
	}
	return out, nil
}

func (r *repository) PruneCalls(ctx context.Context, campaignID string, keep int) error {
	// The cut-off is the id of the keep-th newest call; MariaDB can't LIMIT
	// inside an IN subquery, so it is read first.
	var cut int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM ai_connection_calls WHERE campaign_id = ? ORDER BY id DESC LIMIT 1 OFFSET ?`,
		campaignID, keep-1).Scan(&cut)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return dbError("pruning calls", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM ai_connection_calls WHERE campaign_id = ? AND id < ?`, campaignID, cut); err != nil {
		return dbError("pruning calls", err)
	}
	return nil
}
