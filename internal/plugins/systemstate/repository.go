// Hand-written SQL for entity_system_state.
//
// SECURITY: every method takes campaignID and filters on it, so a lookup with
// an entity id from another campaign cannot match. MariaDB has no row-level
// security, so this scoping and the service's entity check are the guard.
package systemstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Repository is the persistence boundary for system state.
type Repository interface {
	// Get returns the stored row, or nil when there is none.
	Get(ctx context.Context, campaignID, entityID, systemID, key string) (*State, error)
	// Upsert writes the row in one statement. A nil half keeps the stored half
	// (and is stored as {} when the row is new); a non-nil half replaces it.
	Upsert(ctx context.Context, w Write) error
}

// Write is one upsert. GM and Public are nil to keep the stored half.
type Write struct {
	CampaignID string
	EntityID   string
	SystemID   string
	Key        string
	GM         []byte
	Public     []byte
	// UpdatedBy is empty when there is no acting user.
	UpdatedBy string
}

type sqlRepository struct {
	db *sql.DB
}

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository {
	return &sqlRepository{db: db}
}

func (r *sqlRepository) Get(ctx context.Context, campaignID, entityID, systemID, key string) (*State, error) {
	var gm, pub []byte
	var updated time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT gm_data, public_data, updated_at
		   FROM entity_system_state
		  WHERE campaign_id = ? AND entity_id = ? AND system_id = ? AND state_key = ?`,
		campaignID, entityID, systemID, key).Scan(&gm, &pub, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get system state: %w", err)
	}
	return &State{GM: gm, Public: pub, UpdatedAt: &updated}, nil
}

func (r *sqlRepository) Upsert(ctx context.Context, w Write) error {
	// A single INSERT ... ON DUPLICATE KEY UPDATE keeps the partial-update
	// contract atomic: the IF(?, ...) flags pick, per half, between the new
	// value and the stored one, so two concurrent writers to different halves
	// cannot clobber each other through a read-modify-write.
	gm, pub := []byte(`{}`), []byte(`{}`)
	setGM, setPub := w.GM != nil, w.Public != nil
	if setGM {
		gm = w.GM
	}
	if setPub {
		pub = w.Public
	}
	var by any
	if w.UpdatedBy != "" {
		by = w.UpdatedBy
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO entity_system_state
		     (entity_id, campaign_id, system_id, state_key, gm_data, public_data, updated_by, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NOW())
		 ON DUPLICATE KEY UPDATE
		     gm_data     = IF(?, VALUES(gm_data), gm_data),
		     public_data = IF(?, VALUES(public_data), public_data),
		     updated_by  = VALUES(updated_by),
		     updated_at  = NOW()`,
		w.EntityID, w.CampaignID, w.SystemID, w.Key, string(gm), string(pub), by,
		setGM, setPub)
	if err != nil {
		return fmt.Errorf("upsert system state: %w", err)
	}
	return nil
}
