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
	// Replace revokes the connections c's maker has in the campaign and
	// stores c in their place, in one transaction holding the campaign's
	// row, so one person never has two live keys there. It returns c's id.
	Replace(ctx context.Context, c *Connection) (int64, error)
	// RevokeAll revokes every connection of the campaign that is not revoked.
	RevokeAll(ctx context.Context, campaignID string, at time.Time) error
	// RevokeMadeBy revokes every connection userID made in the campaign
	// that is not revoked.
	RevokeMadeBy(ctx context.Context, campaignID, userID string, at time.Time) error
	// RevokeOne revokes the campaign's connection id when it is not revoked.
	RevokeOne(ctx context.Context, campaignID string, id int64, at time.Time) error
	// RevokeAllMadeBy revokes every connection userID made, in every
	// campaign, that is not revoked.
	RevokeAllMadeBy(ctx context.Context, userID string, at time.Time) error
	// SetPaused pauses (at set) or resumes (nil) the campaign's connection id
	// while it is not revoked.
	SetPaused(ctx context.Context, campaignID string, id int64, at *time.Time) error
	// SetAccess changes what the campaign's connection id may see and
	// whether it may ask, while it is not revoked.
	SetAccess(ctx context.Context, campaignID string, id int64, privacy aiexport.PrivacyMode, canAsk bool) error
	// Current is userID's newest connection in the campaign that is not
	// revoked (it may have expired), or nil.
	Current(ctx context.Context, campaignID, userID string) (*Connection, error)
	// PlayerKeys lists the campaign's player keys that are not revoked,
	// with each maker's name, for the owner.
	PlayerKeys(ctx context.Context, campaignID string, now, since time.Time) ([]KeyRow, error)
	// FindByHash is the connection with this key hash whose maker's account
	// is not disabled, or nil. Revoked and expired keys are returned too;
	// the service decides.
	FindByHash(ctx context.Context, keyHash string) (*Connection, error)
	// MarkUsed stamps last_used_at, and the client's name when given.
	MarkUsed(ctx context.Context, id int64, at time.Time, clientName string) error
	// AddCall logs one tool call.
	AddCall(ctx context.Context, call *Call) error
	// RecentCalls lists the calls userID's keys made in the campaign,
	// newest first.
	RecentCalls(ctx context.Context, campaignID, userID string, limit int) ([]Call, error)
	// PruneCalls keeps the newest keep of those calls.
	PruneCalls(ctx context.Context, campaignID, userID string, keep int) error

	// AddRequest stores a request unless the key's maker already has max
	// waiting or the same changes (byte for byte) are already waiting,
	// checked in one transaction holding the campaign's row. It returns the
	// id, ErrAlreadyWaiting or ErrTooManyWaiting.
	AddRequest(ctx context.Context, r *Request, max int) (int64, error)
	// Requests lists what userID's AI asked for in the campaign, in one
	// status, newest first, without the blocks. Each person's requests are
	// theirs alone: a player's wait for that player, the owner's for the
	// owner.
	Requests(ctx context.Context, campaignID, userID, status string, limit int) ([]Request, error)
	// Request is one of userID's requests with its blocks, or nil.
	Request(ctx context.Context, campaignID, userID string, id int64) (*Request, error)
	// FinishRequest moves one of userID's waiting requests to status. It
	// reports whether the request was still waiting.
	FinishRequest(ctx context.Context, campaignID, userID string, id int64, status, outcome string, at time.Time) (bool, error)
	// PruneRequests keeps userID's newest keep requests that are no longer
	// waiting.
	PruneRequests(ctx context.Context, campaignID, userID string, keep int) error
}

type repository struct{ db *sql.DB }

// NewRepository creates the connector's repository.
func NewRepository(db *sql.DB) Repository { return &repository{db: db} }

func dbError(what string, err error) error {
	return apperror.NewInternal(fmt.Errorf("ai connector: %s: %w", what, err))
}

const connectionColumns = `c.id, c.campaign_id, c.user_id, c.key_hash, c.key_hint, c.privacy,
	c.can_ask, c.client_name, c.created_at, c.expires_at, c.last_used_at, c.revoked_at, c.paused_at, c.for_player`

func scanConnection(row interface{ Scan(...any) error }) (*Connection, error) {
	var c Connection
	var privacy string
	var lastUsed, revoked, paused sql.NullTime
	if err := row.Scan(&c.ID, &c.CampaignID, &c.UserID, &c.KeyHash, &c.KeyHint, &privacy,
		&c.CanAsk, &c.ClientName, &c.CreatedAt, &c.ExpiresAt, &lastUsed, &revoked, &paused, &c.ForPlayer); err != nil {
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
		`UPDATE ai_connections SET revoked_at = ? WHERE campaign_id = ? AND user_id = ? AND revoked_at IS NULL`,
		c.CreatedAt.UTC(), c.CampaignID, c.UserID); err != nil {
		return 0, dbError("revoking connections", err)
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO ai_connections (campaign_id, user_id, key_hash, key_hint, privacy, can_ask, created_at, expires_at, for_player)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.CampaignID, c.UserID, c.KeyHash, c.KeyHint, c.Privacy.String(), c.CanAsk, c.CreatedAt.UTC(), c.ExpiresAt.UTC(), c.ForPlayer)
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

func (r *repository) RevokeOne(ctx context.Context, campaignID string, id int64, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE ai_connections SET revoked_at = ? WHERE campaign_id = ? AND id = ? AND revoked_at IS NULL`,
		at.UTC(), campaignID, id); err != nil {
		return dbError("revoking connection", err)
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

func (r *repository) Current(ctx context.Context, campaignID, userID string) (*Connection, error) {
	c, err := scanConnection(r.db.QueryRowContext(ctx,
		`SELECT `+connectionColumns+` FROM ai_connections c
		 WHERE c.campaign_id = ? AND c.user_id = ? AND c.revoked_at IS NULL
		 ORDER BY c.id DESC LIMIT 1`, campaignID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, dbError("reading connection", err)
	}
	return c, nil
}

func (r *repository) PlayerKeys(ctx context.Context, campaignID string, now, since time.Time) ([]KeyRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+connectionColumns+keyRowColumns+`
		 FROM ai_connections c
		 JOIN campaigns cp ON cp.id = c.campaign_id
		 LEFT JOIN users u ON u.id = c.user_id
		 WHERE c.campaign_id = ? AND c.for_player = TRUE AND c.revoked_at IS NULL AND c.expires_at > ?
		 ORDER BY c.last_used_at IS NULL, c.last_used_at DESC, c.id DESC
		 LIMIT 200`, since.UTC(), campaignID, now.UTC())
	if err != nil {
		return nil, dbError("listing player keys", err)
	}
	return scanKeyRows(rows)
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

func (r *repository) RecentCalls(ctx context.Context, campaignID, userID string, limit int) ([]Call, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT k.id, k.connection_id, k.campaign_id, k.called_at, k.tool, k.summary, k.note, k.ok
		 FROM ai_connection_calls k JOIN ai_connections c ON c.id = k.connection_id
		 WHERE k.campaign_id = ? AND c.user_id = ? ORDER BY k.id DESC LIMIT ?`, campaignID, userID, limit)
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

func (r *repository) PruneCalls(ctx context.Context, campaignID, userID string, keep int) error {
	// The cut-off is the id of the keep-th newest call; MariaDB can't LIMIT
	// inside an IN subquery, so it is read first.
	var cut int64
	err := r.db.QueryRowContext(ctx,
		`SELECT k.id FROM ai_connection_calls k JOIN ai_connections c ON c.id = k.connection_id
		 WHERE k.campaign_id = ? AND c.user_id = ? ORDER BY k.id DESC LIMIT 1 OFFSET ?`,
		campaignID, userID, keep-1).Scan(&cut)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return dbError("pruning calls", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`DELETE k FROM ai_connection_calls k JOIN ai_connections c ON c.id = k.connection_id
		 WHERE k.campaign_id = ? AND c.user_id = ? AND k.id < ?`, campaignID, userID, cut); err != nil {
		return dbError("pruning calls", err)
	}
	return nil
}

// ErrTooManyWaiting: the key's maker already has as many requests waiting
// as they may.
var ErrTooManyWaiting = errors.New("too many requests are waiting for review")

// ErrAlreadyWaiting: the key's maker already has these exact changes
// waiting. An AI app that timed out and called again, or an AI that
// resends to be sure, would otherwise leave the owner two copies to review.
var ErrAlreadyWaiting = errors.New("these changes are already waiting for review")

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
	var same int
	if err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ai_connection_requests q JOIN ai_connections c ON c.id = q.connection_id
		 WHERE q.campaign_id = ? AND q.status = ? AND q.body = ?
		   AND c.user_id = (SELECT user_id FROM ai_connections WHERE id = ?)`,
		req.CampaignID, RequestWaiting, req.Body, req.ConnectionID).Scan(&same); err != nil {
		return 0, dbError("checking requests", err)
	}
	if same > 0 {
		err = ErrAlreadyWaiting
		return 0, err
	}
	var waiting int
	if err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ai_connection_requests q JOIN ai_connections c ON c.id = q.connection_id
		 WHERE q.campaign_id = ? AND q.status = ?
		   AND c.user_id = (SELECT user_id FROM ai_connections WHERE id = ?)`,
		req.CampaignID, RequestWaiting, req.ConnectionID).Scan(&waiting); err != nil {
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

const requestColumns = `q.id, q.campaign_id, q.connection_id, q.app_name, q.summary, q.counts, q.blocks, q.status, q.outcome, q.created_at, q.decided_at`

// ofPerson joins a request to the key that sent it, so each person sees only
// what their own AI asked for.
const ofPerson = ` FROM ai_connection_requests q JOIN ai_connections c ON c.id = q.connection_id WHERE q.campaign_id = ? AND c.user_id = ?`

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

func (r *repository) Requests(ctx context.Context, campaignID, userID, status string, limit int) ([]Request, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+requestColumns+ofPerson+` AND q.status = ? ORDER BY q.id DESC LIMIT ?`, campaignID, userID, status, limit)
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

func (r *repository) Request(ctx context.Context, campaignID, userID string, id int64) (*Request, error) {
	var body string
	q, err := scanRequest(r.db.QueryRowContext(ctx,
		`SELECT `+requestColumns+`, q.body`+ofPerson+` AND q.id = ?`,
		campaignID, userID, id), &body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, dbError("reading request", err)
	}
	q.Body = body
	return q, nil
}

func (r *repository) FinishRequest(ctx context.Context, campaignID, userID string, id int64, status, outcome string, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE ai_connection_requests q JOIN ai_connections c ON c.id = q.connection_id
		 SET q.status = ?, q.outcome = ?, q.decided_at = ?
		 WHERE q.campaign_id = ? AND c.user_id = ? AND q.id = ? AND q.status = ?`,
		status, outcome, at.UTC(), campaignID, userID, id, RequestWaiting)
	if err != nil {
		return false, dbError("finishing request", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, dbError("finishing request", err)
	}
	return n == 1, nil
}

func (r *repository) PruneRequests(ctx context.Context, campaignID, userID string, keep int) error {
	var cut int64
	err := r.db.QueryRowContext(ctx,
		`SELECT q.id`+ofPerson+` AND q.status <> ?
		 ORDER BY q.id DESC LIMIT 1 OFFSET ?`, campaignID, userID, RequestWaiting, keep-1).Scan(&cut)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return dbError("pruning requests", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`DELETE q FROM ai_connection_requests q JOIN ai_connections c ON c.id = q.connection_id
		 WHERE q.campaign_id = ? AND c.user_id = ? AND q.status <> ? AND q.id < ?`,
		campaignID, userID, RequestWaiting, cut); err != nil {
		return dbError("pruning requests", err)
	}
	return nil
}
