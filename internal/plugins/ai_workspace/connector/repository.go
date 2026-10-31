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
	// RevokeMadeBy revokes every connection userID made in the campaign
	// that is not revoked.
	RevokeMadeBy(ctx context.Context, campaignID, userID string, at time.Time) error
	// RevokeAllMadeBy revokes every connection userID made, in every
	// campaign, that is not revoked.
	RevokeAllMadeBy(ctx context.Context, userID string, at time.Time) error
	// SetPaused pauses (at set) or resumes (nil) the campaign's connection id
	// while it is not revoked.
	SetPaused(ctx context.Context, campaignID string, id int64, at *time.Time) error
	// SetAccess changes what the campaign's connection id may see and
	// whether it may ask, while it is not revoked.
	SetAccess(ctx context.Context, campaignID string, id int64, privacy aiexport.PrivacyMode, canAsk bool) error
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

	// AddRequest stores a request unless the campaign already has max
	// waiting, checked in one transaction holding the campaign's row. It
	// returns the id, or ErrTooManyWaiting.
	AddRequest(ctx context.Context, r *Request, max int) (int64, error)
	// Requests lists the campaign's requests in one status, newest first,
	// without their blocks.
	Requests(ctx context.Context, campaignID, status string, limit int) ([]Request, error)
	// Request is one of the campaign's requests with its blocks, or nil.
	Request(ctx context.Context, campaignID string, id int64) (*Request, error)
	// FinishRequest moves a waiting request to status. It reports whether
	// the request was still waiting.
	FinishRequest(ctx context.Context, campaignID string, id int64, status, outcome string, at time.Time) (bool, error)
	// PruneRequests keeps the campaign's newest keep requests that are no
	// longer waiting.
	PruneRequests(ctx context.Context, campaignID string, keep int) error
}

type repository struct{ db *sql.DB }

// NewRepository creates the connector's repository.
func NewRepository(db *sql.DB) Repository { return &repository{db: db} }

func dbError(what string, err error) error {
	return apperror.NewInternal(fmt.Errorf("ai connector: %s: %w", what, err))
}

const connectionColumns = `c.id, c.campaign_id, c.user_id, c.key_hash, c.key_hint, c.privacy,
	c.can_ask, c.client_name, c.created_at, c.expires_at, c.last_used_at, c.revoked_at, c.paused_at`

func scanConnection(row interface{ Scan(...any) error }) (*Connection, error) {
	var c Connection
	var privacy string
	var lastUsed, revoked, paused sql.NullTime
	if err := row.Scan(&c.ID, &c.CampaignID, &c.UserID, &c.KeyHash, &c.KeyHint, &privacy,
		&c.CanAsk, &c.ClientName, &c.CreatedAt, &c.ExpiresAt, &lastUsed, &revoked, &paused); err != nil {
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
	if paused.Valid {
		t := paused.Time
		c.PausedAt = &t
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
		`INSERT INTO ai_connections (campaign_id, user_id, key_hash, key_hint, privacy, can_ask, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.CampaignID, c.UserID, c.KeyHash, c.KeyHint, c.Privacy.String(), c.CanAsk, c.CreatedAt.UTC(), c.ExpiresAt.UTC())
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

func (r *repository) RevokeMadeBy(ctx context.Context, campaignID, userID string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE ai_connections SET revoked_at = ? WHERE campaign_id = ? AND user_id = ? AND revoked_at IS NULL`,
		at.UTC(), campaignID, userID); err != nil {
		return dbError("revoking connections", err)
	}
	return nil
}

func (r *repository) RevokeAllMadeBy(ctx context.Context, userID string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE ai_connections SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`,
		at.UTC(), userID); err != nil {
		return dbError("revoking connections", err)
	}
	return nil
}

func (r *repository) SetPaused(ctx context.Context, campaignID string, id int64, at *time.Time) error {
	var v any
	if at != nil {
		v = at.UTC()
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE ai_connections SET paused_at = ? WHERE campaign_id = ? AND id = ? AND revoked_at IS NULL`,
		v, campaignID, id); err != nil {
		return dbError("pausing connection", err)
	}
	return nil
}

func (r *repository) SetAccess(ctx context.Context, campaignID string, id int64, privacy aiexport.PrivacyMode, canAsk bool) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE ai_connections SET privacy = ?, can_ask = ? WHERE campaign_id = ? AND id = ? AND revoked_at IS NULL`,
		privacy.String(), canAsk, campaignID, id); err != nil {
		return dbError("changing connection", err)
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

// ErrTooManyWaiting: the campaign already has as many requests waiting as
// it may.
var ErrTooManyWaiting = errors.New("too many requests are waiting for the owner")

func (r *repository) AddRequest(ctx context.Context, req *Request, max int) (id int64, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, dbError("starting request", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var locked string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM campaigns WHERE id = ? FOR UPDATE`, req.CampaignID).Scan(&locked); err != nil {
		return 0, dbError("locking campaign", err)
	}
	var waiting int
	if err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ai_connection_requests WHERE campaign_id = ? AND status = ?`,
		req.CampaignID, RequestWaiting).Scan(&waiting); err != nil {
		return 0, dbError("counting requests", err)
	}
	if waiting >= max {
		err = ErrTooManyWaiting
		return 0, err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO ai_connection_requests
		   (campaign_id, connection_id, app_name, summary, counts, blocks, body, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.CampaignID, req.ConnectionID, req.AppName, req.Summary, req.Counts, req.Blocks, req.Body,
		RequestWaiting, req.CreatedAt.UTC())
	if err != nil {
		return 0, dbError("saving request", err)
	}
	if id, err = res.LastInsertId(); err != nil {
		return 0, dbError("reading request id", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, dbError("saving request", err)
	}
	return id, nil
}

const requestColumns = `id, campaign_id, connection_id, app_name, summary, counts, blocks, status, outcome, created_at, decided_at`

func scanRequest(row interface{ Scan(...any) error }, body *string) (*Request, error) {
	var q Request
	var decided sql.NullTime
	dest := []any{&q.ID, &q.CampaignID, &q.ConnectionID, &q.AppName, &q.Summary, &q.Counts, &q.Blocks,
		&q.Status, &q.Outcome, &q.CreatedAt, &decided}
	if body != nil {
		dest = append(dest, body)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	if decided.Valid {
		t := decided.Time
		q.DecidedAt = &t
	}
	return &q, nil
}

func (r *repository) Requests(ctx context.Context, campaignID, status string, limit int) ([]Request, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+requestColumns+` FROM ai_connection_requests
		 WHERE campaign_id = ? AND status = ? ORDER BY id DESC LIMIT ?`, campaignID, status, limit)
	if err != nil {
		return nil, dbError("listing requests", err)
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		q, err := scanRequest(rows, nil)
		if err != nil {
			return nil, dbError("reading request", err)
		}
		out = append(out, *q)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError("listing requests", err)
	}
	return out, nil
}

func (r *repository) Request(ctx context.Context, campaignID string, id int64) (*Request, error) {
	var body string
	q, err := scanRequest(r.db.QueryRowContext(ctx,
		`SELECT `+requestColumns+`, body FROM ai_connection_requests WHERE campaign_id = ? AND id = ?`,
		campaignID, id), &body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, dbError("reading request", err)
	}
	q.Body = body
	return q, nil
}

func (r *repository) FinishRequest(ctx context.Context, campaignID string, id int64, status, outcome string, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE ai_connection_requests SET status = ?, outcome = ?, decided_at = ?
		 WHERE campaign_id = ? AND id = ? AND status = ?`,
		status, outcome, at.UTC(), campaignID, id, RequestWaiting)
	if err != nil {
		return false, dbError("finishing request", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, dbError("finishing request", err)
	}
	return n == 1, nil
}

func (r *repository) PruneRequests(ctx context.Context, campaignID string, keep int) error {
	var cut int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM ai_connection_requests WHERE campaign_id = ? AND status <> ?
		 ORDER BY id DESC LIMIT 1 OFFSET ?`, campaignID, RequestWaiting, keep-1).Scan(&cut)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return dbError("pruning requests", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM ai_connection_requests WHERE campaign_id = ? AND status <> ? AND id < ?`,
		campaignID, RequestWaiting, cut); err != nil {
		return dbError("pruning requests", err)
	}
	return nil
}
