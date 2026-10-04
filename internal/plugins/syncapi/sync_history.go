package syncapi

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Directions a sync event can take. "link" is the connection itself
// (connect, disconnect, a catch-up run), not a change to anything.
const (
	DirToChronicle = "to_chronicle"
	DirToFoundry   = "to_foundry"
	DirLink        = "link"
)

// Who wrote a history row: Chronicle for calls it received, the client for
// what only it can see.
const (
	reportedByChronicle = "chronicle"
	reportedByClient    = "client"
)

// historyRetention is how long history rows are kept.
const historyRetention = 90 * 24 * time.Hour

// SyncEvent is one row of the sync history. Children are the steps of a
// catch-up run, filled in on read.
type SyncEvent struct {
	ID           int64       `json:"id"`
	ParentID     *int64      `json:"parentId,omitempty"`
	OccurredAt   time.Time   `json:"at"`
	Direction    string      `json:"direction"`
	ReportedBy   string      `json:"reportedBy"`
	Kind         string      `json:"kind"`
	ResourceID   string      `json:"resourceId,omitempty"`
	ResourceName string      `json:"name"`
	Action       string      `json:"action"`
	Call         string      `json:"call"`
	Status       string      `json:"status"`
	OK           bool        `json:"ok"`
	DurationMs   int         `json:"durationMs"`
	Message      string      `json:"message,omitempty"`
	Was          string      `json:"was,omitempty"`
	UserID       *string     `json:"-"`
	UserName     string      `json:"who,omitempty"`
	APIKeyID     *int        `json:"-"`
	Children     []SyncEvent `json:"children,omitempty"`
}

// SyncHistoryFilter narrows a history read. Before pages backwards by id.
type SyncHistoryFilter struct {
	Before     int64
	After      int64
	Limit      int
	Direction  string
	FailedOnly bool
	Query      string
}

// SyncHistoryRepository owns the sync_events table.
type SyncHistoryRepository interface {
	// Insert stores one event and its children, returning the parent's id.
	Insert(ctx context.Context, campaignID string, ev *SyncEvent) (int64, error)
	// List returns top-level events newest first, each with its children.
	List(ctx context.Context, campaignID string, f SyncHistoryFilter) ([]SyncEvent, error)
	// Get returns one row of this campaign, a step of a run included.
	Get(ctx context.Context, campaignID string, id int64) (*SyncEvent, error)
	// Window returns every row, steps included, within span of centre,
	// oldest first: at most perSide rows on each side, the closest kept, so
	// a busy stretch never pushes out the rows around the centre.
	Window(ctx context.Context, campaignID string, centre time.Time, span time.Duration, perSide int) ([]SyncEvent, error)
	// Latest returns the newest row since a time, or only the newest
	// failure, or nil when there is none.
	Latest(ctx context.Context, campaignID string, since time.Time, failedOnly bool) (*SyncEvent, error)
	// Failures returns this campaign's failed rows for one call and answer
	// since a time, newest first.
	Failures(ctx context.Context, campaignID, call, status string, since time.Time, limit int) ([]SyncEvent, error)
	// PruneOlderThan deletes rows older than cutoff.
	PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type syncHistoryRepo struct {
	db *sql.DB
}

// NewSyncHistoryRepository creates the MariaDB-backed history repository.
func NewSyncHistoryRepository(db *sql.DB) SyncHistoryRepository {
	return &syncHistoryRepo{db: db}
}

const insertSyncEvent = `INSERT INTO sync_events
	(campaign_id, parent_id, occurred_at, direction, reported_by, kind, resource_id, resource_name,
	 action, call_desc, status, ok, duration_ms, message, was_value, user_id, api_key_id)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func (r *syncHistoryRepo) Insert(ctx context.Context, campaignID string, ev *SyncEvent) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("begin sync event: %w", err))
	}
	defer func() { _ = tx.Rollback() }()

	id, err := insertEventTx(ctx, tx, campaignID, nil, ev)
	if err != nil {
		return 0, err
	}
	for i := range ev.Children {
		// A step is part of its run: same reporter, and its run's time when
		// it carries none.
		ch := &ev.Children[i]
		ch.ReportedBy = ev.ReportedBy
		if ch.OccurredAt.IsZero() {
			ch.OccurredAt = ev.OccurredAt
		}
		if _, err := insertEventTx(ctx, tx, campaignID, &id, ch); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("commit sync event: %w", err))
	}
	return id, nil
}

func insertEventTx(ctx context.Context, tx *sql.Tx, campaignID string, parent *int64, ev *SyncEvent) (int64, error) {
	res, err := tx.ExecContext(ctx, insertSyncEvent,
		campaignID, parent, ev.OccurredAt.UTC(), ev.Direction, ev.ReportedBy, ev.Kind, ev.ResourceID,
		ev.ResourceName, ev.Action, ev.Call, ev.Status, ev.OK, ev.DurationMs, ev.Message, ev.Was, ev.UserID, ev.APIKeyID)
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("insert sync event: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("sync event id: %w", err))
	}
	return id, nil
}

const selectSyncEvent = `SELECT e.id, e.parent_id, e.occurred_at, e.direction, e.reported_by, e.kind,
	e.resource_id, e.resource_name, e.action, e.call_desc, e.status, e.ok, e.duration_ms, e.message,
	e.was_value, e.user_id, COALESCE(u.display_name, ''), e.api_key_id
	FROM sync_events e LEFT JOIN users u ON u.id = e.user_id`

func (r *syncHistoryRepo) List(ctx context.Context, campaignID string, f SyncHistoryFilter) ([]SyncEvent, error) {
	where := []string{"e.campaign_id = ?", "e.parent_id IS NULL"}
	args := []any{campaignID}
	if f.Before > 0 {
		where = append(where, "e.id < ?")
		args = append(args, f.Before)
	}
	if f.After > 0 {
		where = append(where, "e.id > ?")
		args = append(args, f.After)
	}
	// A catch-up run matches when any of its steps does, so a filter never
	// hides a failure inside one.
	var match []string
	var matchArgs []any
	if f.Direction != "" {
		match = append(match, "x.direction = ?")
		matchArgs = append(matchArgs, f.Direction)
	}
	if f.FailedOnly {
		match = append(match, "x.ok = 0")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + escapeLike(q) + "%"
		match = append(match, "(x.resource_name LIKE ? OR x.action LIKE ? OR x.call_desc LIKE ? OR COALESCE(xu.display_name, '') LIKE ?)")
		matchArgs = append(matchArgs, like, like, like, like)
	}
	if len(match) > 0 {
		cond := strings.Join(match, " AND ")
		where = append(where, "EXISTS (SELECT 1 FROM sync_events x LEFT JOIN users xu ON xu.id = x.user_id"+
			" WHERE (x.id = e.id OR x.parent_id = e.id) AND "+cond+")")
		args = append(args, matchArgs...)
	}
	limit := f.Limit
	if limit < 1 || limit > 200 {
		limit = 50
	}
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, selectSyncEvent+" WHERE "+strings.Join(where, " AND ")+
		" ORDER BY e.id DESC LIMIT ?", args...)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("list sync events: %w", err))
	}
	events, err := scanSyncEvents(rows)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return events, nil
	}

	ids := make([]any, 0, len(events))
	marks := make([]string, 0, len(events))
	index := make(map[int64]int, len(events))
	for i, ev := range events {
		ids = append(ids, ev.ID)
		marks = append(marks, "?")
		index[ev.ID] = i
	}
	kids, err := r.db.QueryContext(ctx, selectSyncEvent+" WHERE e.parent_id IN ("+strings.Join(marks, ",")+") ORDER BY e.id", ids...)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("list sync event steps: %w", err))
	}
	children, err := scanSyncEvents(kids)
	if err != nil {
		return nil, err
	}
	for _, ch := range children {
		if i, ok := index[*ch.ParentID]; ok {
			events[i].Children = append(events[i].Children, ch)
		}
	}
	return events, nil
}

func scanSyncEvents(rows *sql.Rows) ([]SyncEvent, error) {
	defer rows.Close()
	out := []SyncEvent{}
	for rows.Next() {
		var ev SyncEvent
		var parent sql.NullInt64
		var user sql.NullString
		var key sql.NullInt64
		if err := rows.Scan(&ev.ID, &parent, &ev.OccurredAt, &ev.Direction, &ev.ReportedBy, &ev.Kind,
			&ev.ResourceID, &ev.ResourceName, &ev.Action, &ev.Call, &ev.Status, &ev.OK, &ev.DurationMs,
			&ev.Message, &ev.Was, &user, &ev.UserName, &key); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scan sync event: %w", err))
		}
		if parent.Valid {
			p := parent.Int64
			ev.ParentID = &p
		}
		if user.Valid {
			u := user.String
			ev.UserID = &u
		}
		if key.Valid {
			k := int(key.Int64)
			ev.APIKeyID = &k
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("read sync events: %w", err))
	}
	return out, nil
}

func (r *syncHistoryRepo) Get(ctx context.Context, campaignID string, id int64) (*SyncEvent, error) {
	rows, err := r.db.QueryContext(ctx, selectSyncEvent+" WHERE e.campaign_id = ? AND e.id = ?", campaignID, id)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("get sync event: %w", err))
	}
	events, err := scanSyncEvents(rows)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, apperror.NewNotFound("that history entry was not found")
	}
	return &events[0], nil
}

func (r *syncHistoryRepo) Window(ctx context.Context, campaignID string, centre time.Time, span time.Duration, perSide int) ([]SyncEvent, error) {
	before, err := r.db.QueryContext(ctx, selectSyncEvent+
		" WHERE e.campaign_id = ? AND e.occurred_at BETWEEN ? AND ? ORDER BY e.occurred_at DESC, e.id DESC LIMIT ?",
		campaignID, centre.Add(-span).UTC(), centre.UTC(), perSide)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("sync event window: %w", err))
	}
	earlier, err := scanSyncEvents(before)
	if err != nil {
		return nil, err
	}
	after, err := r.db.QueryContext(ctx, selectSyncEvent+
		" WHERE e.campaign_id = ? AND e.occurred_at > ? AND e.occurred_at <= ? ORDER BY e.occurred_at, e.id LIMIT ?",
		campaignID, centre.UTC(), centre.Add(span).UTC(), perSide)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("sync event window: %w", err))
	}
	later, err := scanSyncEvents(after)
	if err != nil {
		return nil, err
	}
	out := make([]SyncEvent, 0, len(earlier)+len(later))
	for i := len(earlier) - 1; i >= 0; i-- {
		out = append(out, earlier[i])
	}
	return append(out, later...), nil
}

func (r *syncHistoryRepo) Latest(ctx context.Context, campaignID string, since time.Time, failedOnly bool) (*SyncEvent, error) {
	q := selectSyncEvent + " WHERE e.campaign_id = ? AND e.occurred_at >= ?"
	if failedOnly {
		q += " AND e.ok = 0"
	}
	rows, err := r.db.QueryContext(ctx, q+" ORDER BY e.occurred_at DESC, e.id DESC LIMIT 1", campaignID, since.UTC())
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("latest sync event: %w", err))
	}
	events, err := scanSyncEvents(rows)
	if err != nil || len(events) == 0 {
		return nil, err
	}
	return &events[0], nil
}

func (r *syncHistoryRepo) Failures(ctx context.Context, campaignID, call, status string, since time.Time, limit int) ([]SyncEvent, error) {
	rows, err := r.db.QueryContext(ctx, selectSyncEvent+
		" WHERE e.campaign_id = ? AND e.ok = 0 AND e.call_desc = ? AND e.status = ? AND e.occurred_at >= ?"+
		" ORDER BY e.occurred_at DESC, e.id DESC LIMIT ?",
		campaignID, call, status, since.UTC(), limit)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("repeated sync failures: %w", err))
	}
	return scanSyncEvents(rows)
}

func (r *syncHistoryRepo) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM sync_events WHERE created_at < ?`, cutoff.UTC())
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("prune sync events: %w", err))
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// escapeLike makes a search term match literally inside LIKE.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
