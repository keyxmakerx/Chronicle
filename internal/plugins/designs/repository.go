// Hand-written SQL for the design library and its shares.
//
// SECURITY: a row is only ever as visible as the service lets it be. Every
// method here is a plain lookup; the service decides who may see the result
// and returns NotFound for a design the reader may not reach.
package designs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Repository is the persistence boundary for designs.
type Repository interface {
	// List is a campaign's library, newest first; kind "" lists every kind.
	List(ctx context.Context, campaignID, kind string) ([]Design, error)
	// Get returns nil when the design does not exist, whatever campaign it is in.
	Get(ctx context.Context, id string) (*Design, error)
	Count(ctx context.Context, campaignID string) (int, error)
	Insert(ctx context.Context, d *Design) error
	Update(ctx context.Context, id, name, note string) error
	// Delete removes the design and, by cascade, its share state.
	Delete(ctx context.Context, id string) error

	// Share state. SetLink upserts the share row; token is kept when on is false
	// only if the caller passes it, so an off link can be cleared.
	Share(ctx context.Context, id string) (token string, on bool, owners []Owner, err error)
	SetLink(ctx context.Context, id, token string, on bool) error
	// FindByToken returns the design behind a link token, or nil, and whether
	// that link is on.
	FindByToken(ctx context.Context, token string) (*Design, bool, error)
	AddOwner(ctx context.Context, designID, userID string) error
	RemoveOwner(ctx context.Context, designID, userID string) error
	IsNamedOwner(ctx context.Context, designID, userID string) (bool, error)
	// StopSharing turns the link off, forgets its token and removes every
	// named owner, in one transaction.
	StopSharing(ctx context.Context, id string) error
	// SharedWith lists designs named to the user on a game system, leaving out
	// the campaign they are being listed for.
	SharedWith(ctx context.Context, userID, systemID, kind, exceptCampaignID string) ([]SharedDesign, error)
	// SearchOwners finds users who own at least one live campaign, by display
	// name. It reads the users and campaign_members tables read-only, because
	// no service offers a cross-campaign owner search.
	SearchOwners(ctx context.Context, query, exceptUserID string, limit int) ([]OwnerMatch, error)
}

type sqlRepository struct{ db *sql.DB }

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository { return &sqlRepository{db: db} }

const designCols = `d.id, d.campaign_id, d.kind, d.system_id, d.name, d.note, COALESCE(d.maker_user_id,''), d.maker_name, d.data, d.origin, COALESCE(d.origin_ref,''), d.created_at, d.updated_at`

// sharedExpr is true while a design has a working link or a named owner.
const sharedExpr = `(EXISTS (SELECT 1 FROM design_shares s WHERE s.design_id = d.id AND s.link_on) OR EXISTS (SELECT 1 FROM design_share_owners o WHERE o.design_id = d.id))`

type scanner interface{ Scan(dest ...any) error }

func scanDesign(s scanner, withShared bool) (*Design, error) {
	var d Design
	var data string
	dest := []any{&d.ID, &d.CampaignID, &d.Kind, &d.SystemID, &d.Name, &d.Note, &d.MakerUserID, &d.MakerName, &data, &d.Origin, &d.OriginRef, &d.CreatedAt, &d.UpdatedAt}
	if withShared {
		dest = append(dest, &d.Shared)
	}
	if err := s.Scan(dest...); err != nil {
		return nil, err
	}
	d.Data = json.RawMessage(data)
	return &d, nil
}

func (r *sqlRepository) List(ctx context.Context, campaignID, kind string) ([]Design, error) {
	q := `SELECT ` + designCols + `, ` + sharedExpr + ` FROM design_library d WHERE d.campaign_id = ?`
	args := []any{campaignID}
	if kind != "" {
		q += ` AND d.kind = ?`
		args = append(args, kind)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY d.created_at DESC, d.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing designs: %w", err)
	}
	defer rows.Close()
	var out []Design
	for rows.Next() {
		d, err := scanDesign(rows, true)
		if err != nil {
			return nil, fmt.Errorf("scanning design: %w", err)
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (r *sqlRepository) Get(ctx context.Context, id string) (*Design, error) {
	d, err := scanDesign(r.db.QueryRowContext(ctx,
		`SELECT `+designCols+`, `+sharedExpr+` FROM design_library d WHERE d.id = ?`, id), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting design: %w", err)
	}
	return d, nil
}

func (r *sqlRepository) Count(ctx context.Context, campaignID string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM design_library WHERE campaign_id = ?`, campaignID).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting designs: %w", err)
	}
	return n, nil
}

func (r *sqlRepository) Insert(ctx context.Context, d *Design) error {
	var maker, ref any
	if d.MakerUserID != "" {
		maker = d.MakerUserID
	}
	if d.OriginRef != "" {
		ref = d.OriginRef
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO design_library (id, campaign_id, kind, system_id, name, note, maker_user_id, maker_name, data, origin, origin_ref)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.CampaignID, d.Kind, d.SystemID, d.Name, d.Note, maker, d.MakerName, string(d.Data), d.Origin, ref)
	if err != nil {
		return fmt.Errorf("inserting design: %w", err)
	}
	return nil
}

func (r *sqlRepository) Update(ctx context.Context, id, name, note string) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE design_library SET name = ?, note = ? WHERE id = ?`, name, note, id); err != nil {
		return fmt.Errorf("updating design: %w", err)
	}
	return nil
}

func (r *sqlRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM design_library WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting design: %w", err)
	}
	return nil
}
