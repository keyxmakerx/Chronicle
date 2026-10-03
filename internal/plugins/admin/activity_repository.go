package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ActivityRepository is the data-access contract for the admin change log.
type ActivityRepository interface {
	// Insert stores one entry; a zero CreatedAt is stamped with the current time.
	Insert(ctx context.Context, e *ActivityEntry) error

	// List returns entries matching the filter, newest first, with the actor's
	// display name joined in, plus the matching row count for pagination.
	List(ctx context.Context, f ActivityFilter, limit, offset int) ([]ActivityEntry, int, error)

	// Actors lists everyone with at least one logged change, by name.
	Actors(ctx context.Context) ([]ActivityActor, error)
}

type activityRepository struct {
	db *sql.DB
}

// NewActivityRepository creates the MariaDB-backed activity repository.
func NewActivityRepository(db *sql.DB) ActivityRepository {
	return &activityRepository{db: db}
}

// Insert stores one entry. Detail is serialized to JSON; an empty actor is
// stored as NULL so a system-made change is not tied to a fake user id.
func (r *activityRepository) Insert(ctx context.Context, e *ActivityEntry) error {
	var detail []byte
	if len(e.Detail) > 0 {
		var err error
		if detail, err = json.Marshal(e.Detail); err != nil {
			return fmt.Errorf("marshaling admin activity detail: %w", err)
		}
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	var actor any
	if e.ActorUserID != "" {
		actor = e.ActorUserID
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO admin_activity (actor_user_id, action, target_type, target_id, target_label, detail, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		actor, e.Action, e.TargetType, e.TargetID, e.TargetLabel, detail, e.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting admin activity: %w", err)
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

// List returns one page of entries, newest first. LEFT JOIN keeps rows whose
// actor account has since been deleted.
func (r *activityRepository) List(ctx context.Context, f ActivityFilter, limit, offset int) ([]ActivityEntry, int, error) {
	where, args := buildActivityWhere(f)
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_activity a`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting admin activity: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT a.id, a.actor_user_id, a.action, a.target_type, a.target_id, a.target_label,
		        a.detail, a.created_at, COALESCE(u.display_name, '')
		 FROM admin_activity a
		 LEFT JOIN users u ON u.id = a.actor_user_id`+where+`
		 ORDER BY a.created_at DESC, a.id DESC
		 LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing admin activity: %w", err)
	}
	defer rows.Close()

	var out []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		var actor sql.NullString
		var detail []byte
		if err := rows.Scan(&e.ID, &actor, &e.Action, &e.TargetType, &e.TargetID, &e.TargetLabel,
			&detail, &e.CreatedAt, &e.ActorName); err != nil {
			return nil, 0, fmt.Errorf("scanning admin activity: %w", err)
		}
		e.ActorUserID = actor.String
		if len(detail) > 0 {
			// A malformed detail blob must not hide the row itself.
			_ = json.Unmarshal(detail, &e.Detail)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating admin activity: %w", err)
	}
	return out, total, nil
}

// Actors returns each distinct actor once. Accounts deleted since are skipped:
// the filter has no name to show and the id would mean nothing to the reader.
func (r *activityRepository) Actors(ctx context.Context) ([]ActivityActor, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT u.id, u.display_name
		 FROM users u
		 WHERE u.id IN (SELECT DISTINCT actor_user_id FROM admin_activity WHERE actor_user_id IS NOT NULL)
		 ORDER BY u.display_name`)
	if err != nil {
		return nil, fmt.Errorf("listing admin activity actors: %w", err)
	}
	defer rows.Close()
	var out []ActivityActor
	for rows.Next() {
		var a ActivityActor
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			return nil, fmt.Errorf("scanning admin activity actor: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
