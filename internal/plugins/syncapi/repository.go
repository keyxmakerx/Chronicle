package syncapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SyncAPIRepository defines the data access contract for the sync API.
type SyncAPIRepository interface {
	// API key management.
	CreateKey(ctx context.Context, key *APIKey) error
	FindKeyByID(ctx context.Context, id int) (*APIKey, error)
	FindKeyByPrefix(ctx context.Context, prefix string) (*APIKey, error)
	ListKeysByUser(ctx context.Context, userID string) ([]APIKey, error)
	ListKeysByCampaign(ctx context.Context, campaignID string) ([]APIKey, error)
	ListAllKeys(ctx context.Context, limit, offset int) ([]APIKey, int, error)
	UpdateKeyActive(ctx context.Context, id int, active bool) error
	UpdateKeyLastUsed(ctx context.Context, id int, ip string) error
	DeleteKey(ctx context.Context, id int) error

	// Request logging.
	LogRequest(ctx context.Context, log *APIRequestLog) error
	ListRequestLogs(ctx context.Context, filter RequestLogFilter) ([]APIRequestLog, int, error)
	GetRequestTimeSeries(ctx context.Context, since time.Time, interval string) ([]TimeSeriesPoint, error)
	GetTopIPs(ctx context.Context, since time.Time, limit int) ([]TopEntry, error)
	GetTopPaths(ctx context.Context, since time.Time, limit int) ([]TopEntry, error)
	GetTopKeys(ctx context.Context, since time.Time, limit int) ([]TopEntry, error)

	// Security events.
	LogSecurityEvent(ctx context.Context, event *SecurityEvent) error
	ListSecurityEvents(ctx context.Context, filter SecurityEventFilter) ([]SecurityEvent, int, error)
	ResolveSecurityEvent(ctx context.Context, id int64, adminID string) error
	GetSecurityTimeSeries(ctx context.Context, since time.Time) ([]TimeSeriesPoint, error)

	// IP blocklist.
	AddIPBlock(ctx context.Context, block *IPBlock) error
	RemoveIPBlock(ctx context.Context, id int) error
	ListIPBlocks(ctx context.Context) ([]IPBlock, error)
	IsIPBlocked(ctx context.Context, ip string) (bool, error)

	// Statistics.
	GetStats(ctx context.Context, since time.Time) (*APIStats, error)
	GetCampaignStats(ctx context.Context, campaignID string, since time.Time) (*APIStats, error)
}

// syncAPIRepository implements SyncAPIRepository with MariaDB.
type syncAPIRepository struct {
	db *sql.DB
}

// NewSyncAPIRepository creates a new sync API repository.
func NewSyncAPIRepository(db *sql.DB) SyncAPIRepository {
	return &syncAPIRepository{db: db}
}

// --- API Key Management ---

// CreateKey inserts a new API key.
func (r *syncAPIRepository) CreateKey(ctx context.Context, key *APIKey) error {
	permsJSON, _ := json.Marshal(key.Permissions)
	var ipJSON []byte
	if len(key.IPAllowlist) > 0 {
		ipJSON, _ = json.Marshal(key.IPAllowlist)
	}

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO api_keys (key_hash, key_prefix, name, user_id, campaign_id, permissions, ip_allowlist, rate_limit, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key.KeyHash, key.KeyPrefix, key.Name, key.UserID, key.CampaignID,
		permsJSON, ipJSON, key.RateLimit, key.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("creating api key: %w", err)
	}
	id, _ := result.LastInsertId()
	key.ID = int(id)
	return nil
}

// FindKeyByID retrieves an API key by its ID.
func (r *syncAPIRepository) FindKeyByID(ctx context.Context, id int) (*APIKey, error) {
	return r.scanKey(r.db.QueryRowContext(ctx,
		`SELECT id, key_hash, key_prefix, name, user_id, campaign_id, permissions, ip_allowlist,
		        rate_limit, is_active, last_used_at, last_used_ip, expires_at, created_at, updated_at
		 FROM api_keys WHERE id = ?`, id))
}

// FindKeyByPrefix retrieves an API key by its prefix (for auth lookup).
func (r *syncAPIRepository) FindKeyByPrefix(ctx context.Context, prefix string) (*APIKey, error) {
	return r.scanKey(r.db.QueryRowContext(ctx,
		`SELECT id, key_hash, key_prefix, name, user_id, campaign_id, permissions, ip_allowlist,
		        rate_limit, is_active, last_used_at, last_used_ip, expires_at, created_at, updated_at
		 FROM api_keys WHERE key_prefix = ?`, prefix))
}

// ListKeysByUser returns all API keys owned by a user.
func (r *syncAPIRepository) ListKeysByUser(ctx context.Context, userID string) ([]APIKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, key_hash, key_prefix, name, user_id, campaign_id, permissions, ip_allowlist,
		        rate_limit, is_active, last_used_at, last_used_ip, expires_at, created_at, updated_at
		 FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing keys by user: %w", err)
	}
	defer rows.Close()
	return r.scanKeys(rows)
}

// ListKeysByCampaign returns all API keys for a campaign.
func (r *syncAPIRepository) ListKeysByCampaign(ctx context.Context, campaignID string) ([]APIKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, key_hash, key_prefix, name, user_id, campaign_id, permissions, ip_allowlist,
		        rate_limit, is_active, last_used_at, last_used_ip, expires_at, created_at, updated_at
		 FROM api_keys WHERE campaign_id = ? ORDER BY created_at DESC`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing keys by campaign: %w", err)
	}
	defer rows.Close()
	return r.scanKeys(rows)
}

// ListAllKeys returns all API keys with pagination (admin).
func (r *syncAPIRepository) ListAllKeys(ctx context.Context, limit, offset int) ([]APIKey, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting api keys: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, key_hash, key_prefix, name, user_id, campaign_id, permissions, ip_allowlist,
		        rate_limit, is_active, last_used_at, last_used_ip, expires_at, created_at, updated_at
		 FROM api_keys ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing all keys: %w", err)
	}
	defer rows.Close()

	keys, err := r.scanKeys(rows)
	return keys, total, err
}

// UpdateKeyActive enables or disables an API key.
func (r *syncAPIRepository) UpdateKeyActive(ctx context.Context, id int, active bool) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE api_keys SET is_active = ? WHERE id = ?`, active, id)
	if err != nil {
		return fmt.Errorf("updating key active: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("api key not found")
	}
	return nil
}

// UpdateKeyLastUsed records the last usage time and IP.
func (r *syncAPIRepository) UpdateKeyLastUsed(ctx context.Context, id int, ip string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE api_keys SET last_used_at = NOW(), last_used_ip = ? WHERE id = ?`, ip, id)
	if err != nil {
		return fmt.Errorf("updating key last used: %w", err)
	}
	return nil
}

// DeleteKey permanently removes an API key.
func (r *syncAPIRepository) DeleteKey(ctx context.Context, id int) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting api key: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("api key not found")
	}
	return nil
}

// --- Request Logging ---

// LogRequest records an API request.
func (r *syncAPIRepository) LogRequest(ctx context.Context, log *APIRequestLog) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO api_request_log (api_key_id, campaign_id, user_id, method, path, status_code,
		 ip_address, user_agent, request_size, response_size, duration_ms, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.APIKeyID, log.CampaignID, log.UserID, log.Method, log.Path, log.StatusCode,
		log.IPAddress, log.UserAgent, log.RequestSize, log.ResponseSize, log.DurationMs, log.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("logging api request: %w", err)
	}
	return nil
}

// ListRequestLogs returns filtered request logs with pagination.
func (r *syncAPIRepository) ListRequestLogs(ctx context.Context, filter RequestLogFilter) ([]APIRequestLog, int, error) {
	where, args := buildLogFilter(filter)

	var total int
	countQuery := `SELECT COUNT(*) FROM api_request_log` + where
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting request logs: %w", err)
	}

	query := `SELECT id, api_key_id, campaign_id, user_id, method, path, status_code,
	          ip_address, user_agent, request_size, response_size, duration_ms, error_message, created_at
	          FROM api_request_log` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing request logs: %w", err)
	}
	defer rows.Close()

	var logs []APIRequestLog
	for rows.Next() {
		var l APIRequestLog
		if err := rows.Scan(&l.ID, &l.APIKeyID, &l.CampaignID, &l.UserID, &l.Method, &l.Path,
			&l.StatusCode, &l.IPAddress, &l.UserAgent, &l.RequestSize, &l.ResponseSize,
			&l.DurationMs, &l.ErrorMessage, &l.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning request log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// GetRequestTimeSeries returns request counts bucketed by interval.
func (r *syncAPIRepository) GetRequestTimeSeries(ctx context.Context, since time.Time, interval string) ([]TimeSeriesPoint, error) {
	// Use DATE_FORMAT for grouping by hour or day.
	var format string
	switch interval {
	case "hour":
		format = "%Y-%m-%d %H:00:00"
	case "day":
		format = "%Y-%m-%d 00:00:00"
	default:
		format = "%Y-%m-%d %H:00:00"
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT DATE_FORMAT(created_at, ?) as bucket, COUNT(*) as cnt
		 FROM api_request_log WHERE created_at >= ?
		 GROUP BY bucket ORDER BY bucket`, format, since)
	if err != nil {
		return nil, fmt.Errorf("getting request time series: %w", err)
	}
	defer rows.Close()

	var points []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		var ts string
		if err := rows.Scan(&ts, &p.Count); err != nil {
			return nil, fmt.Errorf("scanning time series point: %w", err)
		}
		p.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts)
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetTopIPs returns the most active IPs by request count.
func (r *syncAPIRepository) GetTopIPs(ctx context.Context, since time.Time, limit int) ([]TopEntry, error) {
	return r.getTopEntries(ctx,
		`SELECT ip_address, COUNT(*) as cnt FROM api_request_log
		 WHERE created_at >= ? GROUP BY ip_address ORDER BY cnt DESC LIMIT ?`,
		since, limit)
}

// GetTopPaths returns the most requested API paths.
func (r *syncAPIRepository) GetTopPaths(ctx context.Context, since time.Time, limit int) ([]TopEntry, error) {
	return r.getTopEntries(ctx,
		`SELECT path, COUNT(*) as cnt FROM api_request_log
		 WHERE created_at >= ? GROUP BY path ORDER BY cnt DESC LIMIT ?`,
		since, limit)
}

// GetTopKeys returns the most active API keys by request count.
func (r *syncAPIRepository) GetTopKeys(ctx context.Context, since time.Time, limit int) ([]TopEntry, error) {
	return r.getTopEntries(ctx,
		`SELECT CONCAT(k.key_prefix, ' - ', k.name), COUNT(*) as cnt
		 FROM api_request_log l JOIN api_keys k ON k.id = l.api_key_id
		 WHERE l.created_at >= ? GROUP BY l.api_key_id ORDER BY cnt DESC LIMIT ?`,
		since, limit)
}

// --- Security Events ---

// LogSecurityEvent records a security event.
func (r *syncAPIRepository) LogSecurityEvent(ctx context.Context, event *SecurityEvent) error {
	var detailsJSON []byte
	if event.Details != nil {
		detailsJSON, _ = json.Marshal(event.Details)
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO api_security_events (event_type, api_key_id, campaign_id, ip_address, user_agent, details)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		event.EventType, event.APIKeyID, event.CampaignID, event.IPAddress, event.UserAgent, detailsJSON,
	)
	if err != nil {
		return fmt.Errorf("logging security event: %w", err)
	}
	return nil
}

// ListSecurityEvents returns filtered security events with pagination.
func (r *syncAPIRepository) ListSecurityEvents(ctx context.Context, filter SecurityEventFilter) ([]SecurityEvent, int, error) {
	where, args := buildSecurityFilter(filter)

	var total int
	countQuery := `SELECT COUNT(*) FROM api_security_events` + where
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting security events: %w", err)
	}

	query := `SELECT id, event_type, api_key_id, campaign_id, ip_address, user_agent, details,
	          resolved, resolved_by, resolved_at, created_at
	          FROM api_security_events` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing security events: %w", err)
	}
	defer rows.Close()

	var events []SecurityEvent
	for rows.Next() {
		var e SecurityEvent
		var detailsRaw []byte
		var resolvedAt sql.NullTime
		var resolvedBy sql.NullString
		if err := rows.Scan(&e.ID, &e.EventType, &e.APIKeyID, &e.CampaignID, &e.IPAddress,
			&e.UserAgent, &detailsRaw, &e.Resolved, &resolvedBy, &resolvedAt, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning security event: %w", err)
		}
		if len(detailsRaw) > 0 {
			json.Unmarshal(detailsRaw, &e.Details)
		}
		if resolvedAt.Valid {
			e.ResolvedAt = &resolvedAt.Time
		}
		if resolvedBy.Valid {
			e.ResolvedBy = &resolvedBy.String
		}
		events = append(events, e)
	}
	return events, total, rows.Err()
}

// ResolveSecurityEvent marks a security event as resolved by an admin.
func (r *syncAPIRepository) ResolveSecurityEvent(ctx context.Context, id int64, adminID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE api_security_events SET resolved = 1, resolved_by = ?, resolved_at = NOW() WHERE id = ?`,
		adminID, id)
	if err != nil {
		return fmt.Errorf("resolving security event: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("security event not found")
	}
	return nil
}

// GetSecurityTimeSeries returns security event counts by hour.
func (r *syncAPIRepository) GetSecurityTimeSeries(ctx context.Context, since time.Time) ([]TimeSeriesPoint, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DATE_FORMAT(created_at, '%Y-%m-%d %H:00:00') as bucket, COUNT(*) as cnt
		 FROM api_security_events WHERE created_at >= ?
		 GROUP BY bucket ORDER BY bucket`, since)
	if err != nil {
		return nil, fmt.Errorf("getting security time series: %w", err)
	}
	defer rows.Close()

	var points []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		var ts string
		if err := rows.Scan(&ts, &p.Count); err != nil {
			return nil, fmt.Errorf("scanning security time series: %w", err)
		}
		p.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts)
		points = append(points, p)
	}
	return points, rows.Err()
}

// --- IP Blocklist ---

// AddIPBlock adds an IP to the blocklist.
func (r *syncAPIRepository) AddIPBlock(ctx context.Context, block *IPBlock) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO api_ip_blocklist (ip_address, reason, blocked_by, expires_at)
		 VALUES (?, ?, ?, ?)`,
		block.IPAddress, block.Reason, block.BlockedBy, block.ExpiresAt)
	if err != nil {
		return fmt.Errorf("adding ip block: %w", err)
	}
	id, _ := result.LastInsertId()
	block.ID = int(id)
	return nil
}

// RemoveIPBlock removes an IP from the blocklist.
func (r *syncAPIRepository) RemoveIPBlock(ctx context.Context, id int) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM api_ip_blocklist WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("removing ip block: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("ip block not found")
	}
	return nil
}

// ListIPBlocks returns all blocked IPs.
func (r *syncAPIRepository) ListIPBlocks(ctx context.Context) ([]IPBlock, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, ip_address, reason, blocked_by, expires_at, created_at
		 FROM api_ip_blocklist ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing ip blocks: %w", err)
	}
	defer rows.Close()

	var blocks []IPBlock
	for rows.Next() {
		var b IPBlock
		var expiresAt sql.NullTime
		if err := rows.Scan(&b.ID, &b.IPAddress, &b.Reason, &b.BlockedBy, &expiresAt, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning ip block: %w", err)
		}
		if expiresAt.Valid {
			b.ExpiresAt = &expiresAt.Time
		}
		blocks = append(blocks, b)
	}
	return blocks, rows.Err()
}

// IsIPBlocked checks if an IP is on the blocklist (including unexpired entries).
func (r *syncAPIRepository) IsIPBlocked(ctx context.Context, ip string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM api_ip_blocklist
		 WHERE ip_address = ? AND (expires_at IS NULL OR expires_at > NOW()))`, ip).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking ip blocked: %w", err)
	}
	return exists, nil
}

// --- Statistics ---

// GetStats returns aggregated API statistics since a given time.
func (r *syncAPIRepository) GetStats(ctx context.Context, since time.Time) (*APIStats, error) {
	stats := &APIStats{}

	// Request stats.
	r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END), 0),
		        COUNT(DISTINCT ip_address), COALESCE(AVG(duration_ms), 0)
		 FROM api_request_log WHERE created_at >= ?`, since).
		Scan(&stats.TotalRequests, &stats.TotalErrors, &stats.UniqueIPs, &stats.AvgResponseTimeMs)

	// Key count.
	r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE is_active = 1`).
		Scan(&stats.ActiveKeys)

	// Security event counts.
	r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN resolved = 0 THEN 1 ELSE 0 END), 0)
		 FROM api_security_events WHERE created_at >= ?`, since).
		Scan(&stats.SecurityEvents, &stats.UnresolvedEvents)

	// Blocked IPs.
	r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_ip_blocklist WHERE expires_at IS NULL OR expires_at > NOW()`).
		Scan(&stats.BlockedIPs)

	return stats, nil
}

// GetCampaignStats returns API statistics scoped to a campaign.
func (r *syncAPIRepository) GetCampaignStats(ctx context.Context, campaignID string, since time.Time) (*APIStats, error) {
	stats := &APIStats{}

	r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END), 0),
		        COUNT(DISTINCT ip_address), COALESCE(AVG(duration_ms), 0)
		 FROM api_request_log WHERE campaign_id = ? AND created_at >= ?`, campaignID, since).
		Scan(&stats.TotalRequests, &stats.TotalErrors, &stats.UniqueIPs, &stats.AvgResponseTimeMs)

	r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_keys WHERE campaign_id = ? AND is_active = 1`, campaignID).
		Scan(&stats.ActiveKeys)

	return stats, nil
}

// --- Helpers ---

// scanKey scans a single API key row.
func (r *syncAPIRepository) scanKey(row *sql.Row) (*APIKey, error) {
	k := &APIKey{}
	var permsRaw, ipRaw []byte
	var lastUsedAt sql.NullTime
	var lastUsedIP sql.NullString
	var expiresAt sql.NullTime

	err := row.Scan(&k.ID, &k.KeyHash, &k.KeyPrefix, &k.Name, &k.UserID, &k.CampaignID,
		&permsRaw, &ipRaw, &k.RateLimit, &k.IsActive,
		&lastUsedAt, &lastUsedIP, &expiresAt, &k.CreatedAt, &k.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("api key not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scanning api key: %w", err)
	}

	if len(permsRaw) > 0 {
		json.Unmarshal(permsRaw, &k.Permissions)
	}
	if len(ipRaw) > 0 {
		json.Unmarshal(ipRaw, &k.IPAllowlist)
	}
	if lastUsedAt.Valid {
		k.LastUsedAt = &lastUsedAt.Time
	}
	if lastUsedIP.Valid {
		k.LastUsedIP = &lastUsedIP.String
	}
	if expiresAt.Valid {
		k.ExpiresAt = &expiresAt.Time
	}
	return k, nil
}

// scanKeys scans multiple API key rows.
func (r *syncAPIRepository) scanKeys(rows *sql.Rows) ([]APIKey, error) {
	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var permsRaw, ipRaw []byte
		var lastUsedAt sql.NullTime
		var lastUsedIP sql.NullString
		var expiresAt sql.NullTime

		if err := rows.Scan(&k.ID, &k.KeyHash, &k.KeyPrefix, &k.Name, &k.UserID, &k.CampaignID,
			&permsRaw, &ipRaw, &k.RateLimit, &k.IsActive,
			&lastUsedAt, &lastUsedIP, &expiresAt, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning api key: %w", err)
		}

		if len(permsRaw) > 0 {
			json.Unmarshal(permsRaw, &k.Permissions)
		}
		if len(ipRaw) > 0 {
			json.Unmarshal(ipRaw, &k.IPAllowlist)
		}
		if lastUsedAt.Valid {
			k.LastUsedAt = &lastUsedAt.Time
		}
		if lastUsedIP.Valid {
			k.LastUsedIP = &lastUsedIP.String
		}
		if expiresAt.Valid {
			k.ExpiresAt = &expiresAt.Time
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// getTopEntries is a generic helper for top-N queries.
func (r *syncAPIRepository) getTopEntries(ctx context.Context, query string, since time.Time, limit int) ([]TopEntry, error) {
	rows, err := r.db.QueryContext(ctx, query, since, limit)
	if err != nil {
		return nil, fmt.Errorf("getting top entries: %w", err)
	}
	defer rows.Close()

	var entries []TopEntry
	for rows.Next() {
		var e TopEntry
		if err := rows.Scan(&e.Label, &e.Count); err != nil {
			return nil, fmt.Errorf("scanning top entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// buildLogFilter constructs a WHERE clause for request log queries.
func buildLogFilter(f RequestLogFilter) (string, []any) {
	where := ""
	var args []any
	conditions := []string{}

	if f.APIKeyID != nil {
		conditions = append(conditions, "api_key_id = ?")
		args = append(args, *f.APIKeyID)
	}
	if f.CampaignID != nil {
		conditions = append(conditions, "campaign_id = ?")
		args = append(args, *f.CampaignID)
	}
	if f.IPAddress != nil {
		conditions = append(conditions, "ip_address = ?")
		args = append(args, *f.IPAddress)
	}
	if f.StatusCode != nil {
		conditions = append(conditions, "status_code = ?")
		args = append(args, *f.StatusCode)
	}
	if f.Since != nil {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, *f.Since)
	}

	if len(conditions) > 0 {
		where = " WHERE "
		for i, c := range conditions {
			if i > 0 {
				where += " AND "
			}
			where += c
		}
	}
	return where, args
}

// buildSecurityFilter constructs a WHERE clause for security event queries.
func buildSecurityFilter(f SecurityEventFilter) (string, []any) {
	where := ""
	var args []any
	conditions := []string{}

	if f.EventType != nil {
		conditions = append(conditions, "event_type = ?")
		args = append(args, *f.EventType)
	}
	if f.IPAddress != nil {
		conditions = append(conditions, "ip_address = ?")
		args = append(args, *f.IPAddress)
	}
	if f.Resolved != nil {
		conditions = append(conditions, "resolved = ?")
		args = append(args, *f.Resolved)
	}
	if f.Since != nil {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, *f.Since)
	}

	if len(conditions) > 0 {
		where = " WHERE "
		for i, c := range conditions {
			if i > 0 {
				where += " AND "
			}
			where += c
		}
	}
	return where, args
}
