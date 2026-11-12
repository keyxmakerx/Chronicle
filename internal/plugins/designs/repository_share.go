package designs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

func (r *sqlRepository) Share(ctx context.Context, id string) (string, bool, []Owner, error) {
	var token sql.NullString
	var on bool
	err := r.db.QueryRowContext(ctx, `SELECT link_token, link_on FROM design_shares WHERE design_id = ?`, id).Scan(&token, &on)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, nil, fmt.Errorf("reading share: %w", err)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT o.user_id, COALESCE(u.display_name,'') FROM design_share_owners o
		   JOIN users u ON u.id = o.user_id WHERE o.design_id = ? ORDER BY u.display_name, o.user_id`, id)
	if err != nil {
		return "", false, nil, fmt.Errorf("listing named owners: %w", err)
	}
	defer rows.Close()
	owners := []Owner{}
	for rows.Next() {
		var o Owner
		if err := rows.Scan(&o.UserID, &o.Name); err != nil {
			return "", false, nil, fmt.Errorf("scanning named owner: %w", err)
		}
		owners = append(owners, o)
	}
	return token.String, on, owners, rows.Err()
}

func (r *sqlRepository) SetLink(ctx context.Context, id, token string, on bool) error {
	var tok any
	if token != "" {
		tok = token
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO design_shares (design_id, link_token, link_on) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE link_token = VALUES(link_token), link_on = VALUES(link_on)`, id, tok, on)
	if err != nil {
		return fmt.Errorf("setting link: %w", err)
	}
	return nil
}

func (r *sqlRepository) FindByToken(ctx context.Context, token string) (*Design, bool, error) {
	if token == "" {
		return nil, false, nil
	}
	var id string
	var on bool
	err := r.db.QueryRowContext(ctx, `SELECT design_id, link_on FROM design_shares WHERE link_token = ?`, token).Scan(&id, &on)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("finding link: %w", err)
	}
	d, err := r.Get(ctx, id)
	return d, on, err
}

func (r *sqlRepository) AddOwner(ctx context.Context, designID, userID string) error {
	if _, err := r.db.ExecContext(ctx, `INSERT IGNORE INTO design_share_owners (design_id, user_id) VALUES (?, ?)`, designID, userID); err != nil {
		return fmt.Errorf("adding named owner: %w", err)
	}
	return nil
}

func (r *sqlRepository) RemoveOwner(ctx context.Context, designID, userID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM design_share_owners WHERE design_id = ? AND user_id = ?`, designID, userID); err != nil {
		return fmt.Errorf("removing named owner: %w", err)
	}
	return nil
}

func (r *sqlRepository) IsNamedOwner(ctx context.Context, designID, userID string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM design_share_owners WHERE design_id = ? AND user_id = ?`, designID, userID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking named owner: %w", err)
	}
	return n > 0, nil
}

func (r *sqlRepository) StopSharing(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("stopping share: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE design_shares SET link_on = FALSE, link_token = NULL WHERE design_id = ?`, id); err != nil {
		return fmt.Errorf("turning link off: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM design_share_owners WHERE design_id = ?`, id); err != nil {
		return fmt.Errorf("clearing named owners: %w", err)
	}
	return tx.Commit()
}

func (r *sqlRepository) SharedWith(ctx context.Context, userID, systemID, kind, exceptCampaignID string) ([]SharedDesign, error) {
	q := `SELECT d.id, d.kind, d.name, d.maker_name, d.campaign_id
	        FROM design_share_owners o
	        JOIN design_library d ON d.id = o.design_id
	       WHERE o.user_id = ? AND d.system_id = ? AND d.campaign_id <> ?`
	args := []any{userID, systemID, exceptCampaignID}
	if kind != "" {
		q += ` AND d.kind = ?`
		args = append(args, kind)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY o.added_at DESC, d.id LIMIT 200`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing shared designs: %w", err)
	}
	defer rows.Close()
	out := []SharedDesign{}
	for rows.Next() {
		var s SharedDesign
		if err := rows.Scan(&s.ID, &s.Kind, &s.Name, &s.MakerName, &s.campaignID); err != nil {
			return nil, fmt.Errorf("scanning shared design: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// likeEscape makes user text literal inside a LIKE pattern.
func likeEscape(s string) string {
	return strings.NewReplacer(`|`, `||`, `%`, `|%`, `_`, `|_`).Replace(s)
}

func (r *sqlRepository) SearchOwners(ctx context.Context, query, exceptUserID string, limit int) ([]OwnerMatch, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT u.id, u.display_name, c.settings
		   FROM users u
		   JOIN campaign_members m ON m.user_id = u.id AND m.role = 'owner'
		   JOIN campaigns c ON c.id = m.campaign_id AND c.deleted_at IS NULL AND c.archived_at IS NULL
		  WHERE u.is_disabled = FALSE AND u.id <> ? AND u.display_name LIKE ? ESCAPE '|'
		  ORDER BY u.display_name, u.id, c.id LIMIT 400`, exceptUserID, "%"+likeEscape(query)+"%")
	if err != nil {
		return nil, fmt.Errorf("searching owners: %w", err)
	}
	defer rows.Close()
	var out []OwnerMatch
	at := map[string]int{}
	for rows.Next() {
		var uid, name, settings string
		if err := rows.Scan(&uid, &name, &settings); err != nil {
			return nil, fmt.Errorf("scanning owner: %w", err)
		}
		i, ok := at[uid]
		if !ok {
			if len(out) >= limit {
				continue
			}
			i = len(out)
			at[uid] = i
			out = append(out, OwnerMatch{UserID: uid, DisplayName: name, Systems: []string{}})
		}
		if sys := systemOf(settings); sys != "" && !slices.Contains(out[i].Systems, sys) {
			out[i].Systems = append(out[i].Systems, sys)
		}
	}
	return out, rows.Err()
}

// systemOf reads the game system out of a campaign's settings JSON; a
// campaign with unreadable settings has none.
func systemOf(settings string) string {
	var s struct {
		SystemID string `json:"system_id"`
	}
	if json.Unmarshal([]byte(settings), &s) != nil {
		return ""
	}
	return s.SystemID
}
