package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SecurityEventRepository defines the data access contract for security events.
type SecurityEventRepository interface {
	// Log inserts a new security event into the database.
	Log(ctx context.Context, event *SecurityEvent) error

	// List returns paginated security events, most recent first. Optional
	// eventType filter narrows results to a specific event type.
	List(ctx context.Context, eventType string, limit, offset int) ([]SecurityEvent, int, error)

	// GetStats returns aggregate security statistics for the dashboard.
	GetStats(ctx context.Context) (*SecurityStats, error)

	// CountRecentByIP returns the number of events from a specific IP in
	// the given duration. Useful for detecting brute-force attacks.
	CountRecentByIP(ctx context.Context, ip string, eventType string, since time.Duration) (int, error)
}

// securityEventRepository implements SecurityEventRepository with MariaDB.
type securityEventRepository struct {
	db *sql.DB
}

// NewSecurityEventRepository creates a new repository backed by the given DB.
func NewSecurityEventRepository(db *sql.DB) SecurityEventRepository {
	return &securityEventRepository{db: db}
}

// Log inserts a new security event. Details are serialized to JSON.
func (r *securityEventRepository) Log(ctx context.Context, event *SecurityEvent) error {
	query := `INSERT INTO security_events (event_type, user_id, actor_id, ip_address, user_agent, details, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	var detailsJSON []byte
	if event.Details != nil {
		var err error
		detailsJSON, err = json.Marshal(event.Details)
		if err != nil {
			return fmt.Errorf("marshaling security event details: %w", err)
		}
	}

	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	// Use NULL for empty user/actor IDs (foreign key compatibility).
	var userID, actorID any
	if event.UserID != "" {
		userID = event.UserID
	}
	if event.ActorID != "" {
		actorID = event.ActorID
	}

	result, err := r.db.ExecContext(ctx, query,
		event.EventType, userID, actorID,
		event.IPAddress, event.UserAgent,
		detailsJSON, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting security event: %w", err)
	}

	id, _ := result.LastInsertId()
	event.ID = id
	return nil
}

// List returns paginated security events with user/actor display names.
func (r *securityEventRepository) List(ctx context.Context, eventType string, limit, offset int) ([]SecurityEvent, int, error) {
	// Count total matching events.
	countQuery := `SELECT COUNT(*) FROM security_events`
	countArgs := []any{}
	if eventType != "" {
		countQuery += ` WHERE event_type = ?`
		countArgs = append(countArgs, eventType)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting security events: %w", err)
	}

	// Fetch events with joined user names.
	query := `SELECT se.id, se.event_type, COALESCE(se.user_id, ''), COALESCE(se.actor_id, ''),
	                 se.ip_address, COALESCE(se.user_agent, ''), se.details, se.created_at,
	                 COALESCE(u.display_name, '') AS user_name,
	                 COALESCE(a.display_name, '') AS actor_name
	          FROM security_events se
	          LEFT JOIN users u ON u.id = se.user_id
	          LEFT JOIN users a ON a.id = se.actor_id`

	args := []any{}
	if eventType != "" {
		query += ` WHERE se.event_type = ?`
		args = append(args, eventType)
	}

	query += ` ORDER BY se.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing security events: %w", err)
	}
	defer rows.Close()

	var events []SecurityEvent
	for rows.Next() {
		var e SecurityEvent
		var detailsJSON sql.NullString
		if err := rows.Scan(
			&e.ID, &e.EventType, &e.UserID, &e.ActorID,
			&e.IPAddress, &e.UserAgent, &detailsJSON, &e.CreatedAt,
			&e.UserName, &e.ActorName,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning security event: %w", err)
		}

		if detailsJSON.Valid && detailsJSON.String != "" {
			if jsonErr := json.Unmarshal([]byte(detailsJSON.String), &e.Details); jsonErr != nil {
				e.Details = map[string]any{"_parse_error": "invalid JSON"}
			}
		}

		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating security events: %w", err)
	}

	return events, total, nil
}

// GetStats returns aggregate security statistics for the admin dashboard.
func (r *securityEventRepository) GetStats(ctx context.Context) (*SecurityStats, error) {
	stats := &SecurityStats{}

	// Total events.
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM security_events`).Scan(&stats.TotalEvents); err != nil {
		return nil, fmt.Errorf("counting security events: %w", err)
	}

	// Failed logins in last 24 hours.
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM security_events WHERE event_type = ? AND created_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR)`,
		EventLoginFailed,
	).Scan(&stats.FailedLogins24h); err != nil {
		return nil, fmt.Errorf("counting failed logins: %w", err)
	}

	// Successful logins in last 24 hours.
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM security_events WHERE event_type = ? AND created_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR)`,
		EventLoginSuccess,
	).Scan(&stats.SuccessfulLogins24h); err != nil {
		return nil, fmt.Errorf("counting successful logins: %w", err)
	}

	// Disabled users.
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_disabled = TRUE`).Scan(&stats.DisabledUsers); err != nil {
		return nil, fmt.Errorf("counting disabled users: %w", err)
	}

	// Unique IPs in last 24 hours.
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT ip_address) FROM security_events WHERE created_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR) AND ip_address != ''`,
	).Scan(&stats.UniqueIPs24h); err != nil {
		return nil, fmt.Errorf("counting unique IPs: %w", err)
	}

	return stats, nil
}

// CountRecentByIP returns the number of events from a specific IP in the
// given time window. Used to detect brute-force attempts.
func (r *securityEventRepository) CountRecentByIP(ctx context.Context, ip string, eventType string, since time.Duration) (int, error) {
	query := `SELECT COUNT(*) FROM security_events
	          WHERE ip_address = ? AND event_type = ?
	          AND created_at >= DATE_SUB(NOW(), INTERVAL ? SECOND)`

	var count int
	if err := r.db.QueryRowContext(ctx, query, ip, eventType, int(since.Seconds())).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting recent events by IP: %w", err)
	}

	return count, nil
}
