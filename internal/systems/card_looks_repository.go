package systems

import (
	"context"
	"database/sql"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/database"
)

// CardLookRepository is the data access contract for the looks an owner gave
// package entries. Every method is scoped by campaign.
type CardLookRepository interface {
	// List returns the rows of one field, keyed by entry key.
	List(ctx context.Context, campaignID, systemID, fieldKey string) (map[string]StoredLook, error)
	// Apply writes a batch in one transaction: upserts replace the row for the
	// same (field, entry), deletes remove it. A batch is all or nothing so an
	// owner's save never half-lands.
	Apply(ctx context.Context, upserts []CardLookRow, deletes []CardLookRow) error
	// DeleteDerived removes the rows of the derived entries (<slug>--...) of
	// one parent slug, in every field of the system; it returns how many went.
	DeleteDerived(ctx context.Context, campaignID, systemID, parentSlug string) (int, error)
}

type cardLookRepository struct{ db *sql.DB }

// NewCardLookRepository creates the MariaDB-backed repository.
func NewCardLookRepository(db *sql.DB) CardLookRepository { return &cardLookRepository{db: db} }

func (r *cardLookRepository) List(ctx context.Context, campaignID, systemID, fieldKey string) (map[string]StoredLook, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT entry_key, border, colour FROM campaign_card_looks
		  WHERE campaign_id = ? AND system_id = ? AND field_key = ?`, campaignID, systemID, fieldKey)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]StoredLook{}
	for rows.Next() {
		var key string
		var l StoredLook
		if err := rows.Scan(&key, &l.Border, &l.Colour); err != nil {
			return nil, apperror.NewInternal(err)
		}
		out[key] = l
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (r *cardLookRepository) Apply(ctx context.Context, upserts []CardLookRow, deletes []CardLookRow) error {
	if len(upserts) == 0 && len(deletes) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, u := range upserts {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO campaign_card_looks (campaign_id, system_id, field_key, entry_key, border, colour, updated_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE border = VALUES(border), colour = VALUES(colour), updated_by = VALUES(updated_by)`,
			u.CampaignID, u.SystemID, u.FieldKey, u.EntryKey, u.Border, u.Colour, u.UpdatedBy)
		if err != nil {
			return apperror.NewInternal(err)
		}
	}
	for _, d := range deletes {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM campaign_card_looks WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND entry_key = ?`,
			d.CampaignID, d.SystemID, d.FieldKey, d.EntryKey); err != nil {
			return apperror.NewInternal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// likeEscaper escapes the LIKE wildcards so a slug is matched literally, even
// though slugs are [a-z0-9-] today. It uses the house escape character, which
// means the same under every sql_mode.
var likeEscaper = strings.NewReplacer(
	database.LikeEscapeChar, database.LikeEscapeChar+database.LikeEscapeChar,
	"%", database.LikeEscapeChar+"%",
	"_", database.LikeEscapeChar+"_",
)

func (r *cardLookRepository) DeleteDerived(ctx context.Context, campaignID, systemID, parentSlug string) (int, error) {
	// An empty slug would make the prefix "--" and sweep unrelated keys.
	if parentSlug == "" {
		return 0, nil
	}
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM campaign_card_looks WHERE campaign_id = ? AND system_id = ? AND entry_key LIKE ? ESCAPE '`+database.LikeEscapeChar+`'`,
		campaignID, systemID, likeEscaper.Replace(parentSlug)+`--%`)
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
