// instance_repository.go provides data access for inventory instances.
// Instances are standalone named item collections within a campaign.
package armory

import (
	"context"
	"database/sql"
	"fmt"
)

// InstanceRepository defines the data access contract for inventory instances.
type InstanceRepository interface {
	// Create inserts a new inventory instance and returns it with the generated ID.
	Create(ctx context.Context, campaignID, name, slug, description, icon, color string) (*InventoryInstance, error)

	// FindByID retrieves an instance by ID.
	FindByID(ctx context.Context, id int) (*InventoryInstance, error)

	// ListByCampaign returns all instances for a campaign with item counts.
	ListByCampaign(ctx context.Context, campaignID string) ([]InventoryInstance, error)

	// Update modifies an instance's mutable fields.
	Update(ctx context.Context, id int, name, slug, description, icon, color string) error

	// Delete removes an instance (cascade deletes inventory_items).
	Delete(ctx context.Context, id int) error

	// AddItem adds an entity to an inventory instance.
	AddItem(ctx context.Context, instanceID int, entityID string, quantity int) error

	// RemoveItem removes an entity from an inventory instance.
	RemoveItem(ctx context.Context, instanceID int, entityID string) error

	// CountInstanceItems returns the number of items in an instance.
	CountInstanceItems(ctx context.Context, instanceID int) (int, error)
}

// instanceRepository implements InstanceRepository with MariaDB.
type instanceRepository struct {
	db *sql.DB
}

// NewInstanceRepository creates a new instance repository.
func NewInstanceRepository(db *sql.DB) InstanceRepository {
	return &instanceRepository{db: db}
}

// Create inserts a new inventory instance.
func (r *instanceRepository) Create(ctx context.Context, campaignID, name, slug, description, icon, color string) (*InventoryInstance, error) {
	query := `INSERT INTO inventory_instances (campaign_id, name, slug, description, icon, color)
		VALUES (?, ?, ?, ?, ?, ?)`

	var desc *string
	if description != "" {
		desc = &description
	}

	result, err := r.db.ExecContext(ctx, query, campaignID, name, slug, desc, icon, color)
	if err != nil {
		return nil, fmt.Errorf("creating inventory instance: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting instance id: %w", err)
	}

	return r.FindByID(ctx, int(id))
}

// FindByID retrieves an instance by ID with its item count.
func (r *instanceRepository) FindByID(ctx context.Context, id int) (*InventoryInstance, error) {
	query := `SELECT i.id, i.campaign_id, i.name, i.slug, i.description,
		i.icon, i.color, i.sort_order, i.created_at, i.updated_at,
		(SELECT COUNT(*) FROM inventory_items ii WHERE ii.instance_id = i.id) AS item_count
		FROM inventory_instances i WHERE i.id = ?`

	inst := &InventoryInstance{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&inst.ID, &inst.CampaignID, &inst.Name, &inst.Slug, &inst.Description,
		&inst.Icon, &inst.Color, &inst.SortOrder, &inst.CreatedAt, &inst.UpdatedAt,
		&inst.ItemCount,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding instance %d: %w", id, err)
	}
	return inst, nil
}

// ListByCampaign returns all instances for a campaign ordered by sort_order, name.
func (r *instanceRepository) ListByCampaign(ctx context.Context, campaignID string) ([]InventoryInstance, error) {
	query := `SELECT i.id, i.campaign_id, i.name, i.slug, i.description,
		i.icon, i.color, i.sort_order, i.created_at, i.updated_at,
		(SELECT COUNT(*) FROM inventory_items ii WHERE ii.instance_id = i.id) AS item_count
		FROM inventory_instances i
		WHERE i.campaign_id = ?
		ORDER BY i.sort_order, i.name`

	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing instances: %w", err)
	}
	defer rows.Close()

	var instances []InventoryInstance
	for rows.Next() {
		inst := InventoryInstance{}
		if err := rows.Scan(
			&inst.ID, &inst.CampaignID, &inst.Name, &inst.Slug, &inst.Description,
			&inst.Icon, &inst.Color, &inst.SortOrder, &inst.CreatedAt, &inst.UpdatedAt,
			&inst.ItemCount,
		); err != nil {
			return nil, fmt.Errorf("scanning instance: %w", err)
		}
		instances = append(instances, inst)
	}
	return instances, rows.Err()
}

// Update modifies an instance's mutable fields.
func (r *instanceRepository) Update(ctx context.Context, id int, name, slug, description, icon, color string) error {
	var desc *string
	if description != "" {
		desc = &description
	}

	query := `UPDATE inventory_instances SET name = ?, slug = ?, description = ?, icon = ?, color = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, name, slug, desc, icon, color, id)
	if err != nil {
		return fmt.Errorf("updating instance %d: %w", id, err)
	}
	return nil
}

// Delete removes an inventory instance. FK cascade handles inventory_items cleanup.
func (r *instanceRepository) Delete(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM inventory_instances WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting instance %d: %w", id, err)
	}
	return nil
}

// AddItem links an entity to an inventory instance. Uses INSERT IGNORE to
// silently handle duplicates.
func (r *instanceRepository) AddItem(ctx context.Context, instanceID int, entityID string, quantity int) error {
	query := `INSERT INTO inventory_items (instance_id, entity_id, quantity)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE quantity = VALUES(quantity)`
	_, err := r.db.ExecContext(ctx, query, instanceID, entityID, quantity)
	if err != nil {
		return fmt.Errorf("adding item to instance: %w", err)
	}
	return nil
}

// RemoveItem unlinks an entity from an inventory instance.
func (r *instanceRepository) RemoveItem(ctx context.Context, instanceID int, entityID string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM inventory_items WHERE instance_id = ? AND entity_id = ?", instanceID, entityID)
	if err != nil {
		return fmt.Errorf("removing item from instance: %w", err)
	}
	return nil
}

// CountInstanceItems returns the number of items in an instance.
func (r *instanceRepository) CountInstanceItems(ctx context.Context, instanceID int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_items WHERE instance_id = ?", instanceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting instance items: %w", err)
	}
	return count, nil
}
