// Hand-written SQL for recipes, who knows them, material tags, benches and
// settings.
//
// SECURITY: every statement filters on campaign_id, so an id from another
// campaign never reads or changes anything here.
package crafting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// errDuplicate is a unique-key clash: a recipe or bench name already taken.
var errDuplicate = errors.New("duplicate name")

// Repository is the persistence boundary for crafting.
type Repository interface {
	GetSettings(ctx context.Context, campaignID string) (Settings, error)
	SaveSettings(ctx context.Context, campaignID string, s Settings) error

	ListRecipes(ctx context.Context, campaignID string) ([]Recipe, error)
	GetRecipe(ctx context.Context, campaignID, id string) (Recipe, bool, error)
	// CreateRecipe stores r with its known list; errDuplicate when the name
	// is taken.
	CreateRecipe(ctx context.Context, campaignID, createdBy string, r Recipe) error
	// UpdateRecipe writes everything but the rules and looks if the stored
	// version is still expected, and replaces the known list; ok is false on
	// a lost race. errDuplicate when the new name is taken.
	UpdateRecipe(ctx context.Context, campaignID, userID string, r Recipe, expected int) (ok bool, err error)
	// UpdateRules writes the rules and looks on the same terms.
	UpdateRules(ctx context.Context, campaignID, id string, rules []Rule, looks map[string]*Look, expected int) (ok bool, err error)
	// SetDraft clears or sets the draft flag, bumping the version.
	SetDraft(ctx context.Context, campaignID, id string, draft bool) error
	DeleteRecipe(ctx context.Context, campaignID, id string) error
	// KnownBy lists the recipes each of the characters knows.
	KnownBy(ctx context.Context, campaignID string, characterIDs []string) (map[string][]string, error)

	// ListTags returns each tagged item's tags, in no particular order.
	ListTags(ctx context.Context, campaignID string) (map[string][]string, error)
	// SetTags replaces one item's tags.
	SetTags(ctx context.Context, campaignID, itemID string, tags []string) error

	ListBenches(ctx context.Context, campaignID string) ([]Bench, error)
	GetBench(ctx context.Context, campaignID, id string) (Bench, bool, error)
	// CreateBench returns errDuplicate when the name is taken.
	CreateBench(ctx context.Context, campaignID string, b Bench) error
	// UpdateBench renames a bench or changes its kind; ok false on a lost
	// race, errDuplicate on a name clash.
	UpdateBench(ctx context.Context, campaignID string, b Bench, expected int) (ok bool, err error)
	// SaveArrangement stores one look's arrangement ("desk" or "journal").
	SaveArrangement(ctx context.Context, campaignID, id, skin string, state json.RawMessage, expected int) (ok bool, err error)
	DeleteBench(ctx context.Context, campaignID, id string) error
}

type sqlRepository struct{ db *sql.DB }

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository { return &sqlRepository{db: db} }

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// ---------- Settings ----------

func (r *sqlRepository) GetSettings(ctx context.Context, campaignID string) (Settings, error) {
	s := DefaultSettings()
	err := r.db.QueryRowContext(ctx,
		`SELECT skin, difficulty, bane, skip_steps FROM crafting_settings WHERE campaign_id = ?`, campaignID,
	).Scan(&s.Skin, &s.Difficulty, &s.Bane, &s.Skip)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return s, fmt.Errorf("get crafting settings: %w", err)
	}
	return s, nil
}

func (r *sqlRepository) SaveSettings(ctx context.Context, campaignID string, s Settings) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO crafting_settings (campaign_id, skin, difficulty, bane, skip_steps) VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE skin = VALUES(skin), difficulty = VALUES(difficulty), bane = VALUES(bane), skip_steps = VALUES(skip_steps)`,
		campaignID, s.Skin, s.Difficulty, s.Bane, s.Skip)
	if err != nil {
		return fmt.Errorf("save crafting settings: %w", err)
	}
	return nil
}

// ---------- Recipes ----------

const recipeColumns = `id, name, template, bench_kind, COALESCE(result_item_id, ''), result_qty, goal, stat, prerequisite, source,
	cost_cents, cost_currency, who, materials, steps, colors, rules, looks, draft, draft_prompt, sort_order, version, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanRecipe(s rowScanner) (Recipe, error) {
	var (
		r                                      Recipe
		cents                                  int64
		materials, steps, colors, rules, looks string
	)
	if err := s.Scan(&r.ID, &r.Name, &r.Template, &r.Bench, &r.Result.ItemID, &r.Result.Qty, &r.Goal, &r.Stat,
		&r.Prerequisite, &r.Source, &cents, &r.Cost.Currency, &r.Who, &materials, &steps, &colors, &rules, &looks,
		&r.Draft, &r.DraftPrompt, &r.SortOrder, &r.Version, &r.UpdatedAt); err != nil {
		return r, err
	}
	r.Cost.Amount = float64(cents) / 100
	r.Materials, r.Steps, r.Rules, r.Looks, r.Known = []Material{}, []Step{}, []Rule{}, map[string]*Look{}, []string{}
	// The columns are written only by this package, cleaned; a value that
	// will not decode reads as empty rather than failing the whole list.
	_ = json.Unmarshal([]byte(materials), &r.Materials)
	_ = json.Unmarshal([]byte(steps), &r.Steps)
	_ = json.Unmarshal([]byte(rules), &r.Rules)
	_ = json.Unmarshal([]byte(looks), &r.Looks)
	if colors != "" {
		_ = json.Unmarshal([]byte(colors), &r.Colors)
	}
	return r, nil
}

func (r *sqlRepository) ListRecipes(ctx context.Context, campaignID string) ([]Recipe, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+recipeColumns+` FROM crafting_recipes WHERE campaign_id = ? ORDER BY sort_order, name`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	defer rows.Close()
	out := []Recipe{}
	for rows.Next() {
		rec, err := scanRecipe(rows)
		if err != nil {
			return nil, fmt.Errorf("scan recipe: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	known, err := r.knownFor(ctx, campaignID, "")
	if err != nil {
		return nil, err
	}
	for i := range out {
		if k := known[out[i].ID]; k != nil {
			out[i].Known = k
		}
	}
	return out, nil
}

func (r *sqlRepository) GetRecipe(ctx context.Context, campaignID, id string) (Recipe, bool, error) {
	rec, err := scanRecipe(r.db.QueryRowContext(ctx,
		`SELECT `+recipeColumns+` FROM crafting_recipes WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Recipe{}, false, nil
	}
	if err != nil {
		return Recipe{}, false, fmt.Errorf("get recipe: %w", err)
	}
	known, err := r.knownFor(ctx, campaignID, id)
	if err != nil {
		return Recipe{}, false, err
	}
	if k := known[id]; k != nil {
		rec.Known = k
	}
	return rec, true, nil
}

// knownFor maps recipe id to the characters who know it, for one recipe or
// (with recipeID "") the whole campaign.
func (r *sqlRepository) knownFor(ctx context.Context, campaignID, recipeID string) (map[string][]string, error) {
	q := `SELECT recipe_id, character_id FROM crafting_known WHERE campaign_id = ?`
	args := []any{campaignID}
	if recipeID != "" {
		q += ` AND recipe_id = ?`
		args = append(args, recipeID)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY created_at, character_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list known: %w", err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var rid, cid string
		if err := rows.Scan(&rid, &cid); err != nil {
			return nil, fmt.Errorf("scan known: %w", err)
		}
		out[rid] = append(out[rid], cid)
	}
	return out, rows.Err()
}

func costCents(c Cost) int64 { return int64(math.Round(c.Amount * 100)) }

func colorsJSON(c Colors) string {
	if c == (Colors{}) {
		return ""
	}
	return mustJSON(c)
}

func (r *sqlRepository) CreateRecipe(ctx context.Context, campaignID, createdBy string, rec Recipe) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("create recipe: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	_, err = tx.ExecContext(ctx,
		`INSERT INTO crafting_recipes (id, campaign_id, name, template, bench_kind, result_item_id, result_qty, goal, stat,
			prerequisite, source, cost_cents, cost_currency, who, materials, steps, colors, rules, looks, draft, draft_prompt,
			sort_order, created_by)
		 SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			COALESCE((SELECT MAX(sort_order) + 1 FROM crafting_recipes WHERE campaign_id = ?), 0), ?`,
		rec.ID, campaignID, rec.Name, rec.Template, rec.Bench, nullString(rec.Result.ItemID), rec.Result.Qty, rec.Goal, rec.Stat,
		rec.Prerequisite, rec.Source, costCents(rec.Cost), rec.Cost.Currency, rec.Who, mustJSON(rec.Materials), mustJSON(rec.Steps),
		colorsJSON(rec.Colors), mustJSON(rec.Rules), mustJSON(rec.Looks), rec.Draft, rec.DraftPrompt, campaignID, nullString(createdBy))
	if isDuplicate(err) {
		return errDuplicate
	}
	if err != nil {
		return fmt.Errorf("create recipe: %w", err)
	}
	if err := writeKnown(ctx, tx, campaignID, rec.ID, createdBy, rec.Known); err != nil {
		return err
	}
	return tx.Commit()
}

// writeKnown replaces a recipe's known list inside tx, keeping the rows of
// characters still on it so their dates stay.
func writeKnown(ctx context.Context, tx *sql.Tx, campaignID, recipeID, userID string, known []string) error {
	q := `DELETE FROM crafting_known WHERE campaign_id = ? AND recipe_id = ?`
	args := []any{campaignID, recipeID}
	if len(known) > 0 {
		q += ` AND character_id NOT IN (?` + strings.Repeat(",?", len(known)-1) + `)`
		for _, id := range known {
			args = append(args, id)
		}
	}
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("clear known: %w", err)
	}
	for _, id := range known {
		if _, err := tx.ExecContext(ctx,
			`INSERT IGNORE INTO crafting_known (recipe_id, character_id, campaign_id, added_by) VALUES (?, ?, ?, ?)`,
			recipeID, id, campaignID, nullString(userID)); err != nil {
			return fmt.Errorf("add known: %w", err)
		}
	}
	return nil
}

func (r *sqlRepository) UpdateRecipe(ctx context.Context, campaignID, userID string, rec Recipe, expected int) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("update recipe: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	res, err := tx.ExecContext(ctx,
		`UPDATE crafting_recipes SET name = ?, template = ?, bench_kind = ?, result_item_id = ?, result_qty = ?, goal = ?, stat = ?,
			prerequisite = ?, source = ?, cost_cents = ?, cost_currency = ?, who = ?, materials = ?, steps = ?, colors = ?,
			version = version + 1
		 WHERE campaign_id = ? AND id = ? AND version = ?`,
		rec.Name, rec.Template, rec.Bench, nullString(rec.Result.ItemID), rec.Result.Qty, rec.Goal, rec.Stat,
		rec.Prerequisite, rec.Source, costCents(rec.Cost), rec.Cost.Currency, rec.Who, mustJSON(rec.Materials), mustJSON(rec.Steps),
		colorsJSON(rec.Colors), campaignID, rec.ID, expected)
	if isDuplicate(err) {
		return false, errDuplicate
	}
	if err != nil {
		return false, fmt.Errorf("update recipe: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	if err := writeKnown(ctx, tx, campaignID, rec.ID, userID, rec.Known); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (r *sqlRepository) UpdateRules(ctx context.Context, campaignID, id string, rules []Rule, looks map[string]*Look, expected int) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE crafting_recipes SET rules = ?, looks = ?, version = version + 1 WHERE campaign_id = ? AND id = ? AND version = ?`,
		mustJSON(rules), mustJSON(looks), campaignID, id, expected)
	if err != nil {
		return false, fmt.Errorf("update recipe rules: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sqlRepository) SetDraft(ctx context.Context, campaignID, id string, draft bool) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE crafting_recipes SET draft = ?, version = version + 1 WHERE campaign_id = ? AND id = ?`, draft, campaignID, id); err != nil {
		return fmt.Errorf("set recipe draft: %w", err)
	}
	return nil
}

func (r *sqlRepository) DeleteRecipe(ctx context.Context, campaignID, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM crafting_recipes WHERE campaign_id = ? AND id = ?`, campaignID, id); err != nil {
		return fmt.Errorf("delete recipe: %w", err)
	}
	return nil
}

func (r *sqlRepository) KnownBy(ctx context.Context, campaignID string, characterIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(characterIDs) == 0 {
		return out, nil
	}
	args := []any{campaignID}
	for _, id := range characterIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT character_id, recipe_id FROM crafting_known WHERE campaign_id = ? AND character_id IN (?`+
			strings.Repeat(",?", len(characterIDs)-1)+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("known by: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, rid string
		if err := rows.Scan(&cid, &rid); err != nil {
			return nil, fmt.Errorf("scan known by: %w", err)
		}
		out[cid] = append(out[cid], rid)
	}
	return out, rows.Err()
}

// ---------- Material tags ----------

func (r *sqlRepository) ListTags(ctx context.Context, campaignID string) (map[string][]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT item_id, tag FROM crafting_material_tags WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list material tags: %w", err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var id, tag string
		if err := rows.Scan(&id, &tag); err != nil {
			return nil, fmt.Errorf("scan material tag: %w", err)
		}
		out[id] = append(out[id], tag)
	}
	return out, rows.Err()
}

func (r *sqlRepository) SetTags(ctx context.Context, campaignID, itemID string, tags []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set material tags: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	if _, err := tx.ExecContext(ctx, `DELETE FROM crafting_material_tags WHERE campaign_id = ? AND item_id = ?`, campaignID, itemID); err != nil {
		return fmt.Errorf("clear material tags: %w", err)
	}
	for _, t := range tags {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO crafting_material_tags (item_id, tag, campaign_id) VALUES (?, ?, ?)`, itemID, t, campaignID); err != nil {
			return fmt.Errorf("add material tag: %w", err)
		}
	}
	return tx.Commit()
}

// ---------- Benches ----------

func scanBench(s rowScanner) (Bench, error) {
	var (
		b             Bench
		desk, journal sql.NullString
	)
	if err := s.Scan(&b.ID, &b.Name, &b.Kind, &desk, &journal, &b.SortOrder, &b.Version); err != nil {
		return b, err
	}
	if desk.Valid && desk.String != "" {
		b.Desk = json.RawMessage(desk.String)
	}
	if journal.Valid && journal.String != "" {
		b.Journal = json.RawMessage(journal.String)
	}
	return b, nil
}

const benchColumns = `id, name, kind, desk, journal, sort_order, version`

func (r *sqlRepository) ListBenches(ctx context.Context, campaignID string) ([]Bench, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+benchColumns+` FROM crafting_benches WHERE campaign_id = ? ORDER BY sort_order, name`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list benches: %w", err)
	}
	defer rows.Close()
	out := []Bench{}
	for rows.Next() {
		b, err := scanBench(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bench: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *sqlRepository) GetBench(ctx context.Context, campaignID, id string) (Bench, bool, error) {
	b, err := scanBench(r.db.QueryRowContext(ctx,
		`SELECT `+benchColumns+` FROM crafting_benches WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Bench{}, false, nil
	}
	if err != nil {
		return Bench{}, false, fmt.Errorf("get bench: %w", err)
	}
	return b, true, nil
}

func (r *sqlRepository) CreateBench(ctx context.Context, campaignID string, b Bench) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO crafting_benches (id, campaign_id, name, kind, sort_order)
		 SELECT ?, ?, ?, ?, COALESCE((SELECT MAX(sort_order) + 1 FROM crafting_benches WHERE campaign_id = ?), 0)`,
		b.ID, campaignID, b.Name, b.Kind, campaignID)
	if isDuplicate(err) {
		return errDuplicate
	}
	if err != nil {
		return fmt.Errorf("create bench: %w", err)
	}
	return nil
}

func (r *sqlRepository) UpdateBench(ctx context.Context, campaignID string, b Bench, expected int) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE crafting_benches SET name = ?, kind = ?, version = version + 1 WHERE campaign_id = ? AND id = ? AND version = ?`,
		b.Name, b.Kind, campaignID, b.ID, expected)
	if isDuplicate(err) {
		return false, errDuplicate
	}
	if err != nil {
		return false, fmt.Errorf("update bench: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sqlRepository) SaveArrangement(ctx context.Context, campaignID, id, skin string, state json.RawMessage, expected int) (bool, error) {
	col := "desk"
	if skin == "journal" {
		col = "journal"
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE crafting_benches SET `+col+` = ?, version = version + 1 WHERE campaign_id = ? AND id = ? AND version = ?`,
		string(state), campaignID, id, expected)
	if err != nil {
		return false, fmt.Errorf("save bench arrangement: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sqlRepository) DeleteBench(ctx context.Context, campaignID, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM crafting_benches WHERE campaign_id = ? AND id = ?`, campaignID, id); err != nil {
		return fmt.Errorf("delete bench: %w", err)
	}
	return nil
}
