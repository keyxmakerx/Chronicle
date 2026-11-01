package connector

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type watchRepository struct{ db *sql.DB }

// NewWatchRepository creates the SQL behind the switches and alerts.
func NewWatchRepository(db *sql.DB) WatchRepository { return &watchRepository{db: db} }

func (r *watchRepository) ServerAccess(ctx context.Context) (ServerAccess, error) {
	var a ServerAccess
	var allowed bool
	err := r.db.QueryRowContext(ctx, `SELECT ai_off, players_allowed FROM ai_server_access WHERE id = 1`).Scan(&a.AIOff, &allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return ServerAccess{}, nil
	}
	if err != nil {
		return a, dbError("reading server switches", err)
	}
	a.PlayersBlocked = !allowed
	return a, nil
}

func (r *watchRepository) SetServerAccess(ctx context.Context, a ServerAccess, by string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO ai_server_access (id, ai_off, players_allowed, updated_at, updated_by) VALUES (1, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE ai_off = VALUES(ai_off), players_allowed = VALUES(players_allowed),
		   updated_at = VALUES(updated_at), updated_by = VALUES(updated_by)`,
		a.AIOff, !a.PlayersBlocked, at.UTC(), by); err != nil {
		return dbError("saving server switches", err)
	}
	return nil
}

func (r *watchRepository) CampaignAccess(ctx context.Context, campaignID string) (CampaignAccess, error) {
	var a CampaignAccess
	var cut sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT players_allowed, cut_off_at FROM ai_campaign_access WHERE campaign_id = ?`, campaignID).Scan(&a.PlayersAllowed, &cut)
	if errors.Is(err, sql.ErrNoRows) {
		return CampaignAccess{}, nil
	}
	if err != nil {
		return a, dbError("reading campaign switches", err)
	}
	if cut.Valid {
		t := cut.Time
		a.CutOffAt = &t
	}
	return a, nil
}

func (r *watchRepository) SetCutOff(ctx context.Context, campaignID string, at *time.Time, now time.Time) error {
	var v any
	if at != nil {
		v = at.UTC()
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO ai_campaign_access (campaign_id, cut_off_at, updated_at) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE cut_off_at = VALUES(cut_off_at), updated_at = VALUES(updated_at)`,
		campaignID, v, now.UTC()); err != nil {
		return dbError("saving campaign switch", err)
	}
	return nil
}

func (r *watchRepository) SetPlayersAllowed(ctx context.Context, campaignID string, allowed bool, now time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO ai_campaign_access (campaign_id, players_allowed, updated_at) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE players_allowed = VALUES(players_allowed), updated_at = VALUES(updated_at)`,
		campaignID, allowed, now.UTC()); err != nil {
		return dbError("saving players switch", err)
	}
	return nil
}

// seenAppsMax keeps seen_apps inside its column; once full, a new app is
// still warned about but no longer remembered.
const seenAppsMax = 255

func (r *watchRepository) NoteApp(ctx context.Context, connID int64, app string) (bool, error) {
	var seen string
	if err := r.db.QueryRowContext(ctx, `SELECT seen_apps FROM ai_connections WHERE id = ?`, connID).Scan(&seen); err != nil {
		return false, dbError("reading seen apps", err)
	}
	name := strings.ReplaceAll(app, ",", " ")
	var list []string
	if seen != "" {
		list = strings.Split(seen, ",")
	}
	for _, s := range list {
		if strings.EqualFold(s, name) {
			return false, nil
		}
	}
	next := strings.Join(append(list, name), ",")
	if len(next) <= seenAppsMax {
		if _, err := r.db.ExecContext(ctx, `UPDATE ai_connections SET seen_apps = ? WHERE id = ?`, next, connID); err != nil {
			return false, dbError("saving seen apps", err)
		}
	}
	return len(list) > 0, nil
}

func (r *watchRepository) RaiseAlert(ctx context.Context, a *Alert) error {
	var campaign, conn any
	if a.CampaignID != "" {
		campaign = a.CampaignID
	}
	if a.ConnectionID != 0 {
		conn = a.ConnectionID
	}
	// One open alert per key (or address) and kind: a repeat adds its hits.
	res, err := r.db.ExecContext(ctx,
		`UPDATE ai_connection_alerts SET hits = hits + ?, updated_at = ?, detail = ?
		 WHERE kind = ? AND settled_at IS NULL
		   AND ((? IS NOT NULL AND connection_id = ?) OR (? IS NULL AND connection_id IS NULL AND address = ?))`,
		a.Hits, a.UpdatedAt.UTC(), a.Detail, a.Kind, conn, conn, conn, a.Address)
	if err != nil {
		return dbError("updating alert", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO ai_connection_alerts (campaign_id, connection_id, kind, detail, address, hits, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		campaign, conn, a.Kind, a.Detail, a.Address, a.Hits, a.CreatedAt.UTC(), a.UpdatedAt.UTC()); err != nil {
		return dbError("saving alert", err)
	}
	return nil
}

const alertColumns = `id, campaign_id, connection_id, kind, detail, address, hits, created_at, updated_at, settled_at, settled_how`

func scanAlerts(rows *sql.Rows) ([]Alert, error) {
	defer rows.Close()
	var out []Alert
	for rows.Next() {
		var a Alert
		var campaign sql.NullString
		var conn sql.NullInt64
		var settled sql.NullTime
		if err := rows.Scan(&a.ID, &campaign, &conn, &a.Kind, &a.Detail, &a.Address, &a.Hits,
			&a.CreatedAt, &a.UpdatedAt, &settled, &a.SettledHow); err != nil {
			return nil, dbError("reading alert", err)
		}
		a.CampaignID, a.ConnectionID = campaign.String, conn.Int64
		if settled.Valid {
			t := settled.Time
			a.SettledAt = &t
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError("listing alerts", err)
	}
	return out, nil
}

func (r *watchRepository) OpenAlerts(ctx context.Context, campaignID string, limit int) ([]Alert, error) {
	var rows *sql.Rows
	var err error
	if campaignID == "" {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+alertColumns+` FROM ai_connection_alerts WHERE settled_at IS NULL ORDER BY id DESC LIMIT ?`, limit)
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+alertColumns+` FROM ai_connection_alerts WHERE campaign_id = ? AND settled_at IS NULL ORDER BY id DESC LIMIT ?`,
			campaignID, limit)
	}
	if err != nil {
		return nil, dbError("listing alerts", err)
	}
	return scanAlerts(rows)
}

func (r *watchRepository) RecentAlerts(ctx context.Context, since time.Time, limit int) ([]Alert, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+alertColumns+` FROM ai_connection_alerts WHERE updated_at >= ? ORDER BY updated_at DESC, id DESC LIMIT ?`,
		since.UTC(), limit)
	if err != nil {
		return nil, dbError("listing alerts", err)
	}
	return scanAlerts(rows)
}

func (r *watchRepository) SettleAlert(ctx context.Context, campaignID string, id int64, how string, at time.Time) (*Alert, error) {
	var rows *sql.Rows
	var err error
	if campaignID == "" {
		rows, err = r.db.QueryContext(ctx, `SELECT `+alertColumns+` FROM ai_connection_alerts WHERE id = ? AND settled_at IS NULL`, id)
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+alertColumns+` FROM ai_connection_alerts WHERE id = ? AND campaign_id = ? AND settled_at IS NULL`, id, campaignID)
	}
	if err != nil {
		return nil, dbError("reading alert", err)
	}
	list, err := scanAlerts(rows)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE ai_connection_alerts SET settled_at = ?, settled_how = ? WHERE id = ? AND settled_at IS NULL`, at.UTC(), how, id)
	if err != nil {
		return nil, dbError("settling alert", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	a := list[0]
	a.SettledAt, a.SettledHow = &at, how
	return &a, nil
}

func (r *watchRepository) ConnectionByID(ctx context.Context, id int64) (*Connection, error) {
	c, err := scanConnection(r.db.QueryRowContext(ctx, `SELECT `+connectionColumns+` FROM ai_connections c WHERE c.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, dbError("reading connection", err)
	}
	return c, nil
}

func (r *watchRepository) LiveKeys(ctx context.Context, now, since time.Time) ([]KeyRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+connectionColumns+keyRowColumns+`
		 FROM ai_connections c
		 JOIN campaigns cp ON cp.id = c.campaign_id
		 LEFT JOIN users u ON u.id = c.user_id
		 WHERE c.revoked_at IS NULL AND c.expires_at > ?
		 ORDER BY c.last_used_at IS NULL, c.last_used_at DESC, c.id DESC
		 LIMIT 500`, since.UTC(), now.UTC())
	if err != nil {
		return nil, dbError("listing keys", err)
	}
	return scanKeyRows(rows)
}

// keyRowColumns follow connectionColumns in a KeyRow query: the campaign's
// name, the maker's name and the key's calls since a time (the first ?).
const keyRowColumns = `, cp.name, COALESCE(NULLIF(u.display_name, ''), u.email, ''),
	   (SELECT COUNT(*) FROM ai_connection_calls k WHERE k.connection_id = c.id AND k.called_at >= ?),
	   (SELECT COUNT(*) FROM ai_connection_calls k WHERE k.connection_id = c.id)`

// scanKeyRows reads KeyRows, leaving out every key hash.
func scanKeyRows(rows *sql.Rows) ([]KeyRow, error) {
	defer rows.Close()
	var out []KeyRow
	for rows.Next() {
		var k KeyRow
		var privacy string
		var lastUsed, revoked, paused sql.NullTime
		if err := rows.Scan(&k.ID, &k.CampaignID, &k.UserID, &k.KeyHash, &k.KeyHint, &privacy,
			&k.CanAsk, &k.ClientName, &k.CreatedAt, &k.ExpiresAt, &lastUsed, &revoked, &paused, &k.ForPlayer,
			&k.CampaignName, &k.PersonName, &k.CallsToday, &k.CallsTotal); err != nil {
			return nil, dbError("reading key", err)
		}
		k.Privacy = parsePrivacy(privacy)
		if lastUsed.Valid {
			t := lastUsed.Time
			k.LastUsedAt = &t
		}
		if paused.Valid {
			t := paused.Time
			k.PausedAt = &t
		}
		k.KeyHash = ""
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError("listing keys", err)
	}
	return out, nil
}

func (r *watchRepository) CallsSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_connection_calls WHERE called_at >= ?`, since.UTC()).Scan(&n); err != nil {
		return 0, dbError("counting calls", err)
	}
	return n, nil
}
