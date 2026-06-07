// repository.go — hand-written SQL for widget_bindings (C-WIDGET-BINDING-P1-SPINE).
//
// SECURITY (precedent refinement #3): campaignID is pushed down to EVERY
// signature so an unscoped read is unrepresentable — there is no method that
// reads a binding without a campaign filter. MariaDB has no row-level-security
// backstop, so this repository + the service are the only line of defense
// against cross-campaign access.
package widgetbindings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Repository is the persistence boundary for widget bindings. Every method is
// campaign-scoped by signature.
type Repository interface {
	// Upsert creates or updates the binding for (campaign, host, widget_type).
	// One binding per host per widget type (calendar is singleton-per-host).
	Upsert(ctx context.Context, b *WidgetBinding) error
	// GetForHost returns the binding for an exact host + widget type, or nil.
	GetForHost(ctx context.Context, campaignID, hostType, hostID, widgetType string) (*WidgetBinding, error)
	// DeleteForHostWidget removes a host's binding for a widget type (no-op if absent).
	DeleteForHostWidget(ctx context.Context, campaignID, hostType, hostID, widgetType string) error
	// ListByCampaign returns all bindings in a campaign (the integrity sweep input).
	ListByCampaign(ctx context.Context, campaignID string) ([]WidgetBinding, error)
	// ListForInstance returns bindings pointing at a specific instance (the
	// delete-hook input when that instance is deleted).
	ListForInstance(ctx context.Context, campaignID, widgetType, instanceID string) ([]WidgetBinding, error)
	// DeleteByID removes one binding (campaign-scoped).
	DeleteByID(ctx context.Context, campaignID, id string) error
}

type sqlRepository struct {
	db *sql.DB
}

// NewRepository creates a MariaDB-backed binding repository.
func NewRepository(db *sql.DB) Repository {
	return &sqlRepository{db: db}
}

const bindingCols = `id, campaign_id, host_type, host_id, widget_type, instance_id, created_at, updated_at`

func (r *sqlRepository) Upsert(ctx context.Context, b *WidgetBinding) error {
	// ON DUPLICATE KEY (uq on campaign_id, host_type, host_id, widget_type)
	// updates the instance — re-binding a host to a new instance is idempotent.
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO widget_bindings (`+bindingCols+`)
		 VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
		 ON DUPLICATE KEY UPDATE instance_id = VALUES(instance_id), updated_at = NOW()`,
		b.ID, b.CampaignID, b.HostType, b.HostID, b.WidgetType, b.InstanceID)
	if err != nil {
		return fmt.Errorf("upsert widget binding: %w", err)
	}
	return nil
}

func (r *sqlRepository) GetForHost(ctx context.Context, campaignID, hostType, hostID, widgetType string) (*WidgetBinding, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+bindingCols+` FROM widget_bindings
		 WHERE campaign_id = ? AND host_type = ? AND host_id = ? AND widget_type = ?`,
		campaignID, hostType, hostID, widgetType)
	b, err := scanBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

func (r *sqlRepository) DeleteForHostWidget(ctx context.Context, campaignID, hostType, hostID, widgetType string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM widget_bindings
		 WHERE campaign_id = ? AND host_type = ? AND host_id = ? AND widget_type = ?`,
		campaignID, hostType, hostID, widgetType)
	return err
}

func (r *sqlRepository) ListByCampaign(ctx context.Context, campaignID string) ([]WidgetBinding, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+bindingCols+` FROM widget_bindings WHERE campaign_id = ? ORDER BY widget_type, host_type, host_id`,
		campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBindings(rows)
}

func (r *sqlRepository) ListForInstance(ctx context.Context, campaignID, widgetType, instanceID string) ([]WidgetBinding, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+bindingCols+` FROM widget_bindings
		 WHERE campaign_id = ? AND widget_type = ? AND instance_id = ?`,
		campaignID, widgetType, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBindings(rows)
}

func (r *sqlRepository) DeleteByID(ctx context.Context, campaignID, id string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM widget_bindings WHERE campaign_id = ? AND id = ?`, campaignID, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBinding(s rowScanner) (*WidgetBinding, error) {
	var b WidgetBinding
	if err := s.Scan(&b.ID, &b.CampaignID, &b.HostType, &b.HostID, &b.WidgetType, &b.InstanceID, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}

func scanBindings(rows *sql.Rows) ([]WidgetBinding, error) {
	var out []WidgetBinding
	for rows.Next() {
		b, err := scanBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}
