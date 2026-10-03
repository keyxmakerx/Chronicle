package entities

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// PageSafetyRepository stores what lets a world page be undone: its version
// history, its place in the Trash, and the text revision the editor checks
// so one person's save can't silently replace another's.
type PageSafetyRepository interface {
	// LatestVersion returns a page's newest version, or nil when it has none.
	LatestVersion(ctx context.Context, entityID string) (*EntityVersion, error)
	InsertVersion(ctx context.Context, v *EntityVersion) error
	// ReplaceVersionContent overwrites a version's title, text and
	// updated_at; used when a save joins the author's open history row.
	ReplaceVersionContent(ctx context.Context, v *EntityVersion) error
	// ListVersions returns a page's versions newest first, with text.
	ListVersions(ctx context.Context, entityID string, limit int) ([]EntityVersion, error)
	// FindVersion returns one version of entityID; NotFound when the id
	// belongs to another page.
	FindVersion(ctx context.Context, entityID, versionID string) (*EntityVersion, error)
	PruneVersions(ctx context.Context, entityID string, keep int) error

	// EntryRev returns a live page's text revision.
	EntryRev(ctx context.Context, entityID string) (int, error)
	// SaveEntryAtRev writes the text only if the stored revision is still
	// baseRev, and returns the new revision; ok is false when someone else
	// saved first.
	SaveEntryAtRev(ctx context.Context, entityID, entryJSON, entryHTML, searchText string, baseRev int) (newRev int, ok bool, err error)

	// TrashSubtree moves a live page and its live sub-pages to the Trash as
	// one item, and returns the ids that moved, the page first.
	TrashSubtree(ctx context.Context, campaignID, rootID, userID string, at time.Time) ([]string, error)
	// ListTrash returns the campaign's Trash items, newest first.
	ListTrash(ctx context.Context, campaignID string) ([]TrashItem, error)
	// FindTrashItem returns one Trash item; NotFound when it isn't in the
	// campaign's Trash.
	FindTrashItem(ctx context.Context, campaignID, rootID string) (*TrashItem, error)
	// RestoreTrashItem brings a Trash item's pages back, and returns how many.
	RestoreTrashItem(ctx context.Context, campaignID, rootID string) (int, error)
	// PurgeTrashedBefore deletes for good the pages trashed before cutoff.
	PurgeTrashedBefore(ctx context.Context, cutoff time.Time) (int, error)
}

type pageSafetyRepository struct {
	db *sql.DB
}

// NewPageSafetyRepository creates the MariaDB page-safety repository.
func NewPageSafetyRepository(db *sql.DB) PageSafetyRepository {
	return &pageSafetyRepository{db: db}
}

const versionColumns = `v.id, v.entity_id, v.user_id, v.kind, v.name, v.entry, v.entry_html,
	v.created_at, v.updated_at, COALESCE(u.display_name, '')`

func scanVersion(scan func(dest ...any) error) (*EntityVersion, error) {
	v := &EntityVersion{}
	if err := scan(&v.ID, &v.EntityID, &v.UserID, &v.Kind, &v.Name, &v.Entry, &v.EntryHTML,
		&v.CreatedAt, &v.UpdatedAt, &v.UserName); err != nil {
		return nil, err
	}
	return v, nil
}

func (r *pageSafetyRepository) LatestVersion(ctx context.Context, entityID string) (*EntityVersion, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+versionColumns+`
		FROM entity_versions v LEFT JOIN users u ON u.id = v.user_id
		WHERE v.entity_id = ?
		ORDER BY v.created_at DESC, v.updated_at DESC LIMIT 1`, entityID)
	v, err := scanVersion(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading latest page version: %w", err)
	}
	return v, nil
}

func (r *pageSafetyRepository) InsertVersion(ctx context.Context, v *EntityVersion) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO entity_versions
		(id, entity_id, user_id, kind, name, entry, entry_html, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.EntityID, v.UserID, v.Kind, v.Name, v.Entry, v.EntryHTML, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting page version: %w", err)
	}
	return nil
}

func (r *pageSafetyRepository) ReplaceVersionContent(ctx context.Context, v *EntityVersion) error {
	_, err := r.db.ExecContext(ctx, `UPDATE entity_versions
		SET name = ?, entry = ?, entry_html = ?, updated_at = ? WHERE id = ?`,
		v.Name, v.Entry, v.EntryHTML, v.UpdatedAt, v.ID)
	if err != nil {
		return fmt.Errorf("updating page version: %w", err)
	}
	return nil
}

func (r *pageSafetyRepository) ListVersions(ctx context.Context, entityID string, limit int) ([]EntityVersion, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+versionColumns+`
		FROM entity_versions v LEFT JOIN users u ON u.id = v.user_id
		WHERE v.entity_id = ?
		ORDER BY v.created_at DESC, v.updated_at DESC LIMIT ?`, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing page versions: %w", err)
	}
	defer rows.Close()
	var out []EntityVersion
	for rows.Next() {
		v, err := scanVersion(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning page version: %w", err)
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *pageSafetyRepository) FindVersion(ctx context.Context, entityID, versionID string) (*EntityVersion, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+versionColumns+`
		FROM entity_versions v LEFT JOIN users u ON u.id = v.user_id
		WHERE v.id = ? AND v.entity_id = ?`, versionID, entityID)
	v, err := scanVersion(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("version not found")
	}
	if err != nil {
		return nil, fmt.Errorf("reading page version: %w", err)
	}
	return v, nil
}

// PruneVersions keeps the newest `keep` versions. The cut-off is read first
// because MariaDB can't LIMIT inside an IN subquery.
func (r *pageSafetyRepository) PruneVersions(ctx context.Context, entityID string, keep int) error {
	var cutoff time.Time
	err := r.db.QueryRowContext(ctx, `SELECT created_at FROM entity_versions
		WHERE entity_id = ? ORDER BY created_at DESC LIMIT 1 OFFSET ?`, entityID, keep-1).Scan(&cutoff)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("finding version prune point: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM entity_versions
		WHERE entity_id = ? AND created_at < ?`, entityID, cutoff); err != nil {
		return fmt.Errorf("pruning page versions: %w", err)
	}
	return nil
}

func (r *pageSafetyRepository) EntryRev(ctx context.Context, entityID string) (int, error) {
	var rev int
	err := r.db.QueryRowContext(ctx, `SELECT entry_rev FROM entities
		WHERE id = ? AND deleted_at IS NULL`, entityID).Scan(&rev)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, apperror.NewNotFound("entity not found")
	}
	if err != nil {
		return 0, fmt.Errorf("reading entry revision: %w", err)
	}
	return rev, nil
}

// SaveEntryAtRev is one conditional UPDATE, so two saves racing on the same
// revision can't both win.
func (r *pageSafetyRepository) SaveEntryAtRev(ctx context.Context, entityID, entryJSON, entryHTML, searchText string, baseRev int) (int, bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE entities
		SET entry = ?, entry_html = ?, search_text = ?, entry_rev = entry_rev + 1, updated_at = NOW()
		WHERE id = ? AND entry_rev = ? AND deleted_at IS NULL`,
		entryJSON, entryHTML, searchText, entityID, baseRev)
	if err != nil {
		return 0, false, fmt.Errorf("saving entity entry: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("checking rows affected: %w", err)
	}
	rev, err := r.EntryRev(ctx, entityID)
	if err != nil {
		return 0, false, err
	}
	return rev, n == 1, nil
}

// maxTrashDepth bounds the sub-page walk, as FindAncestors bounds its own.
const maxTrashDepth = 50

func (r *pageSafetyRepository) TrashSubtree(ctx context.Context, campaignID, rootID, userID string, at time.Time) ([]string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting trash transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit

	// Collect ids first: MariaDB won't UPDATE a table its own subquery reads.
	// Sub-pages already in the Trash keep their own item.
	rows, err := tx.QueryContext(ctx, `WITH RECURSIVE sub AS (
			SELECT id, 0 AS depth FROM entities
			WHERE id = ? AND campaign_id = ? AND deleted_at IS NULL
			UNION ALL
			SELECT e.id, sub.depth + 1 FROM entities e
			INNER JOIN sub ON e.parent_id = sub.id
			WHERE e.deleted_at IS NULL AND sub.depth < ?
		) SELECT id FROM sub ORDER BY depth`, rootID, campaignID, maxTrashDepth)
	if err != nil {
		return nil, fmt.Errorf("collecting sub-pages: %w", err)
	}
	var ids []string
	var idArgs []any
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning sub-page id: %w", err)
		}
		ids = append(ids, id)
		idArgs = append(idArgs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("collecting sub-pages: %w", err)
	}
	if len(ids) == 0 {
		return nil, apperror.NewNotFound("entity not found")
	}

	var by any
	if userID != "" {
		by = userID
	}
	args := append([]any{at, by, rootID, campaignID}, idArgs...)
	if _, err := tx.ExecContext(ctx, `UPDATE entities
		SET deleted_at = ?, deleted_by = ?, trash_root_id = ?
		WHERE campaign_id = ? AND deleted_at IS NULL AND id IN (`+sqlPlaceholders(len(ids))+`)`, args...); err != nil {
		return nil, fmt.Errorf("moving pages to trash: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing trash: %w", err)
	}
	return ids, nil
}

const trashItemSelect = `SELECT e.id, e.name, et.name, et.icon, e.deleted_at, e.deleted_by,
		COALESCE(u.display_name, ''),
		(SELECT COUNT(*) FROM entities c WHERE c.trash_root_id = e.id AND c.id <> e.id)
	FROM entities e
	INNER JOIN entity_types et ON et.id = e.entity_type_id
	LEFT JOIN users u ON u.id = e.deleted_by
	WHERE e.campaign_id = ? AND e.deleted_at IS NOT NULL AND e.trash_root_id = e.id`

func scanTrashItem(scan func(dest ...any) error) (*TrashItem, error) {
	t := &TrashItem{}
	if err := scan(&t.ID, &t.Name, &t.TypeName, &t.TypeIcon, &t.DeletedAt, &t.DeletedBy,
		&t.DeletedByName, &t.SubPages); err != nil {
		return nil, err
	}
	return t, nil
}

func (r *pageSafetyRepository) ListTrash(ctx context.Context, campaignID string) ([]TrashItem, error) {
	rows, err := r.db.QueryContext(ctx, trashItemSelect+` ORDER BY e.deleted_at DESC, e.name`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing trash: %w", err)
	}
	defer rows.Close()
	var out []TrashItem
	for rows.Next() {
		t, err := scanTrashItem(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning trash item: %w", err)
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (r *pageSafetyRepository) FindTrashItem(ctx context.Context, campaignID, rootID string) (*TrashItem, error) {
	t, err := scanTrashItem(r.db.QueryRowContext(ctx, trashItemSelect+` AND e.id = ?`, campaignID, rootID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("page is not in the trash")
	}
	if err != nil {
		return nil, fmt.Errorf("reading trash item: %w", err)
	}
	return t, nil
}

func (r *pageSafetyRepository) RestoreTrashItem(ctx context.Context, campaignID, rootID string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("starting restore transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit

	res, err := tx.ExecContext(ctx, `UPDATE entities
		SET deleted_at = NULL, deleted_by = NULL, trash_root_id = NULL
		WHERE campaign_id = ? AND trash_root_id = ? AND deleted_at IS NOT NULL`, campaignID, rootID)
	if err != nil {
		return 0, fmt.Errorf("restoring pages: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return 0, apperror.NewNotFound("page is not in the trash")
	}
	// A restored page whose parent is still in the Trash (deleted on its own
	// later) would hang under a page nobody can see; put it at the top level.
	if _, err := tx.ExecContext(ctx, `UPDATE entities e
		INNER JOIN entities p ON p.id = e.parent_id
		SET e.parent_id = NULL
		WHERE e.id = ? AND p.deleted_at IS NOT NULL`, rootID); err != nil {
		return 0, fmt.Errorf("detaching restored page: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing restore: %w", err)
	}
	return int(n), nil
}

// purgeBatch keeps each purge statement short so it never holds locks long.
const purgeBatch = 200

func (r *pageSafetyRepository) PurgeTrashedBefore(ctx context.Context, cutoff time.Time) (int, error) {
	total := 0
	for {
		res, err := r.db.ExecContext(ctx, `DELETE FROM entities
			WHERE deleted_at IS NOT NULL AND deleted_at < ? LIMIT ?`, cutoff, purgeBatch)
		if err != nil {
			return total, fmt.Errorf("purging trash: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("checking rows affected: %w", err)
		}
		total += int(n)
		if n < purgeBatch {
			return total, nil
		}
	}
}

// sqlPlaceholders returns "?, ?, ..." for n arguments.
func sqlPlaceholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}
