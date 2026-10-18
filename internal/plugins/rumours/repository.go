package rumours

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	// errPageFull is returned by Insert when the page already holds MaxRumours;
	// the service turns it into a message for the writer.
	errPageFull = errors.New("page holds the most rumours it can")
	// errNoPage is returned by Insert when the page is not in that campaign.
	errNoPage = errors.New("page not found in this campaign")
)

// Repository owns the page_rumours table. It decides nothing about who may do
// what, except one thing the SQL itself enforces: a read made without
// withTruth never selects the truth column, so a read for a Scribe or a player
// cannot carry it even if a caller forgets to drop it afterwards.
type Repository interface {
	// List returns the page's rumours, newest first, at most limit, and how
	// many the page holds in all.
	List(ctx context.Context, campaignID, entityID string, withTruth bool, limit int) ([]Rumour, int, error)
	// Get returns one rumour of that page, or nil when it is not there.
	Get(ctx context.Context, campaignID, entityID, id string, withTruth bool) (*Rumour, error)
	// Insert adds a rumour unless the page already holds max; it also checks
	// the page is in the campaign, under a lock that serialises two adds.
	Insert(ctx context.Context, r Rumour, max int) error
	// UpdateText changes what was said, who said it and how sure they were.
	UpdateText(ctx context.Context, campaignID, entityID, id, quote, source string, sure int) (bool, error)
	// SetTruth records the Director's verdict.
	SetTruth(ctx context.Context, campaignID, entityID, id, truth string) (bool, error)
	// SetBelief records what the party thinks and who marked it.
	SetBelief(ctx context.Context, campaignID, entityID, id, belief, byUser string, byDM bool) (bool, error)
	// Delete removes a rumour of that page; false when it was not there.
	Delete(ctx context.Context, campaignID, entityID, id string) (bool, error)
}

type repository struct {
	db *sql.DB
}

// NewRepository creates the MariaDB repository for rumours.
func NewRepository(db *sql.DB) Repository {
	return &repository{db: db}
}

// selectRumour is the column list of a read. The truth column is named only
// when the caller may have it; otherwise a constant empty string stands in.
func selectRumour(withTruth bool) string {
	truth := `''`
	if withTruth {
		truth = `r.truth`
	}
	return `SELECT r.id, r.campaign_id, r.entity_id, r.quote, r.source, r.sure, ` + truth + `, r.belief,
		COALESCE(r.belief_by, ''), COALESCE(bu.display_name, ''), r.belief_by_dm, r.by_dm, COALESCE(r.created_by, ''), COALESCE(au.display_name, ''), r.created_at
		FROM page_rumours r
		LEFT JOIN users au ON au.id = r.created_by
		LEFT JOIN users bu ON bu.id = r.belief_by`
}

func scanRumour(sc interface{ Scan(...any) error }) (Rumour, error) {
	var r Rumour
	var at time.Time
	err := sc.Scan(&r.ID, &r.CampaignID, &r.EntityID, &r.Quote, &r.Source, &r.Sure, &r.Truth, &r.Belief,
		&r.BeliefBy, &r.BeliefName, &r.BeliefByDM, &r.ByDM, &r.CreatedBy, &r.AuthorName, &at)
	r.CreatedAt = at
	return r, err
}

func (r *repository) List(ctx context.Context, campaignID, entityID string, withTruth bool, limit int) ([]Rumour, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM page_rumours WHERE entity_id = ? AND campaign_id = ?`, entityID, campaignID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting rumours: %w", err)
	}
	rows, err := r.db.QueryContext(ctx,
		selectRumour(withTruth)+` WHERE r.entity_id = ? AND r.campaign_id = ? ORDER BY r.created_at DESC, r.id DESC LIMIT ?`,
		entityID, campaignID, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("listing rumours: %w", err)
	}
	defer rows.Close()
	var out []Rumour
	for rows.Next() {
		x, err := scanRumour(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning rumour: %w", err)
		}
		out = append(out, x)
	}
	return out, total, rows.Err()
}

func (r *repository) Get(ctx context.Context, campaignID, entityID, id string, withTruth bool) (*Rumour, error) {
	x, err := scanRumour(r.db.QueryRowContext(ctx,
		selectRumour(withTruth)+` WHERE r.id = ? AND r.entity_id = ? AND r.campaign_id = ?`, id, entityID, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding rumour: %w", err)
	}
	return &x, nil
}

func (r *repository) Insert(ctx context.Context, x Rumour, max int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting rumour write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the page row so two adds at once cannot both slip under the cap;
	// the same read proves the page is in this campaign.
	var lock string
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM entities WHERE id = ? AND campaign_id = ? FOR UPDATE`, x.EntityID, x.CampaignID).Scan(&lock); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNoPage
		}
		return fmt.Errorf("locking page: %w", err)
	}
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM page_rumours WHERE entity_id = ?`, x.EntityID).Scan(&n); err != nil {
		return fmt.Errorf("counting rumours: %w", err)
	}
	if n >= max {
		return errPageFull
	}
	var by any
	if x.CreatedBy != "" {
		by = x.CreatedBy
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO page_rumours (id, campaign_id, entity_id, quote, source, sure, truth, belief, by_dm, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		x.ID, x.CampaignID, x.EntityID, x.Quote, x.Source, x.Sure, TruthOpen, x.ByDM, by); err != nil {
		return fmt.Errorf("saving rumour: %w", err)
	}
	return tx.Commit()
}

// changed reports whether a statement touched a row.
func changed(res sql.Result, err error, what string) (bool, error) {
	if err != nil {
		return false, fmt.Errorf("%s: %w", what, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("%s: %w", what, err)
	}
	return n > 0, nil
}

func (r *repository) UpdateText(ctx context.Context, campaignID, entityID, id, quote, source string, sure int) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE page_rumours SET quote = ?, source = ?, sure = ? WHERE id = ? AND entity_id = ? AND campaign_id = ?`,
		quote, source, sure, id, entityID, campaignID)
	return changedOrThere(ctx, r.db, res, err, "updating rumour", campaignID, entityID, id)
}

func (r *repository) SetTruth(ctx context.Context, campaignID, entityID, id, truth string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE page_rumours SET truth = ? WHERE id = ? AND entity_id = ? AND campaign_id = ?`,
		truth, id, entityID, campaignID)
	return changedOrThere(ctx, r.db, res, err, "setting rumour truth", campaignID, entityID, id)
}

func (r *repository) SetBelief(ctx context.Context, campaignID, entityID, id, belief, byUser string, byDM bool) (bool, error) {
	var by any
	if byUser != "" {
		by = byUser
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE page_rumours SET belief = ?, belief_by = ?, belief_by_dm = ? WHERE id = ? AND entity_id = ? AND campaign_id = ?`,
		belief, by, byDM, id, entityID, campaignID)
	return changedOrThere(ctx, r.db, res, err, "setting rumour belief", campaignID, entityID, id)
}

// changedOrThere is changed with a fallback: an UPDATE that wrote the value
// already stored reports zero rows on MariaDB, which is not "not found", so a
// zero count is settled by asking whether the rumour exists.
func changedOrThere(ctx context.Context, db *sql.DB, res sql.Result, err error, what, campaignID, entityID, id string) (bool, error) {
	ok, err := changed(res, err, what)
	if err != nil || ok {
		return ok, err
	}
	var one int
	qerr := db.QueryRowContext(ctx,
		`SELECT 1 FROM page_rumours WHERE id = ? AND entity_id = ? AND campaign_id = ?`, id, entityID, campaignID).Scan(&one)
	if errors.Is(qerr, sql.ErrNoRows) {
		return false, nil
	}
	if qerr != nil {
		return false, fmt.Errorf("%s: %w", what, qerr)
	}
	return true, nil
}

func (r *repository) Delete(ctx context.Context, campaignID, entityID, id string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM page_rumours WHERE id = ? AND entity_id = ? AND campaign_id = ?`, id, entityID, campaignID)
	return changed(res, err, "removing rumour")
}
