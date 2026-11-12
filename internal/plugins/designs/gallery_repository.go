// Hand-written SQL for the server gallery.
//
// SECURITY: these are plain lookups. Which entries a reader may see is decided
// in the service; Browse is the one query that filters by status itself,
// because it is the only listing every Owner on the server can reach.
package designs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// GalleryRepository is the persistence boundary for the gallery.
type GalleryRepository interface {
	Insert(ctx context.Context, e *GalleryEntry) error
	// Get returns nil when the entry does not exist.
	Get(ctx context.Context, id string) (*GalleryEntry, error)
	BySource(ctx context.Context, publisher, sourceDesignID string) (*GalleryEntry, error)
	// Save writes the mutable fields of an entry, including its state.
	Save(ctx context.Context, e *GalleryEntry) error
	Delete(ctx context.Context, id string) error
	// Browse lists visible entries only, newest first, with the total.
	Browse(ctx context.Context, q BrowseQuery) ([]GalleryEntry, int, error)
	ByPublisher(ctx context.Context, publisher string) ([]GalleryEntry, error)
	ByStatus(ctx context.Context, statuses []string, limit, offset int) ([]GalleryEntry, int, error)
	// DuplicateExists is true when the publisher has another live entry with
	// the same data hash.
	DuplicateExists(ctx context.Context, publisher, hash, exceptID string) (bool, error)
	// RefusedExists is true when the publisher has another entry with this data
	// hash that an admin refused or took down.
	RefusedExists(ctx context.Context, publisher, hash, exceptID string) (bool, error)
	// FlagReported sends a still-visible entry back to review with these
	// reasons, touching only status and reasons. It reports whether it did.
	FlagReported(ctx context.Context, id string, reasons []string) (bool, error)
	CountEvents(ctx context.Context, user string, since time.Time) (int, error)
	AddEvent(ctx context.Context, user string) error
	// AddReport returns false when this reporter already reported the entry.
	AddReport(ctx context.Context, entryID, reporter, reason, note string) (bool, error)
	Reports(ctx context.Context, entryID string) ([]GalleryReport, error)
	ReportedBy(ctx context.Context, reporter string, ids []string) (map[string]bool, error)
	ClearReports(ctx context.Context, entryID string) error
}

// ErrGalleryDuplicate is returned by Insert when the publisher already has an
// entry for that library design (the unique key), so a race reads as a conflict.
var ErrGalleryDuplicate = errors.New("gallery entry already exists")

type sqlGalleryRepository struct{ db *sql.DB }

// NewGalleryRepository creates a MariaDB-backed gallery repository.
func NewGalleryRepository(db *sql.DB) GalleryRepository { return &sqlGalleryRepository{db: db} }

const galleryCols = `g.id, g.publisher_user_id, COALESCE(g.source_design_id,''), g.kind, g.system_id, g.name, g.note, g.maker_name, g.data, g.data_hash, g.status, g.reasons, COALESCE(g.reviewed_by,''), g.published_at, g.updated_at`

func scanGallery(s scanner) (*GalleryEntry, error) {
	var e GalleryEntry
	var data, reasons string
	if err := s.Scan(&e.ID, &e.PublisherUserID, &e.SourceDesignID, &e.Kind, &e.SystemID, &e.Name, &e.Note, &e.MakerName, &data, &e.DataHash, &e.Status, &reasons, &e.ReviewedBy, &e.PublishedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	e.Data = json.RawMessage(data)
	if err := json.Unmarshal([]byte(reasons), &e.Reasons); err != nil || e.Reasons == nil {
		e.Reasons = []string{}
	}
	return &e, nil
}

func reasonsJSON(r []string) string {
	if r == nil {
		r = []string{}
	}
	b, _ := json.Marshal(r)
	return string(b)
}

func (r *sqlGalleryRepository) Insert(ctx context.Context, e *GalleryEntry) error {
	var src any
	if e.SourceDesignID != "" {
		src = e.SourceDesignID
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO design_gallery (id, publisher_user_id, source_design_id, kind, system_id, name, note, maker_name, data, data_hash, status, reasons)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.PublisherUserID, src, e.Kind, e.SystemID, e.Name, e.Note, e.MakerName, string(e.Data), e.DataHash, e.Status, reasonsJSON(e.Reasons))
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		return ErrGalleryDuplicate
	}
	if err != nil {
		return fmt.Errorf("inserting gallery entry: %w", err)
	}
	return nil
}

func (r *sqlGalleryRepository) one(ctx context.Context, where string, args ...any) (*GalleryEntry, error) {
	e, err := scanGallery(r.db.QueryRowContext(ctx, `SELECT `+galleryCols+` FROM design_gallery g WHERE `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading gallery entry: %w", err)
	}
	return e, nil
}

func (r *sqlGalleryRepository) Get(ctx context.Context, id string) (*GalleryEntry, error) {
	return r.one(ctx, `g.id = ?`, id)
}

func (r *sqlGalleryRepository) BySource(ctx context.Context, publisher, src string) (*GalleryEntry, error) {
	return r.one(ctx, `g.publisher_user_id = ? AND g.source_design_id = ?`, publisher, src)
}

func (r *sqlGalleryRepository) Save(ctx context.Context, e *GalleryEntry) error {
	var by, at any
	if e.ReviewedBy != "" {
		by, at = e.ReviewedBy, time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE design_gallery SET name = ?, note = ?, maker_name = ?, data = ?, data_hash = ?, system_id = ?, status = ?, reasons = ?,
		   reviewed_by = ?, reviewed_at = ? WHERE id = ?`,
		e.Name, e.Note, e.MakerName, string(e.Data), e.DataHash, e.SystemID, e.Status, reasonsJSON(e.Reasons), by, at, e.ID)
	if err != nil {
		return fmt.Errorf("saving gallery entry: %w", err)
	}
	return nil
}

func (r *sqlGalleryRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM design_gallery WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting gallery entry: %w", err)
	}
	return nil
}

func (r *sqlGalleryRepository) list(ctx context.Context, where string, args []any, limit, offset int) ([]GalleryEntry, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM design_gallery g WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting gallery entries: %w", err)
	}
	q := `SELECT ` + galleryCols + ` FROM design_gallery g WHERE ` + where + ` ORDER BY g.published_at DESC, g.id`
	a := args
	if limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		a = append(append([]any{}, args...), limit, offset)
	}
	rows, err := r.db.QueryContext(ctx, q, a...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing gallery entries: %w", err)
	}
	defer rows.Close()
	var out []GalleryEntry
	for rows.Next() {
		e, err := scanGallery(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning gallery entry: %w", err)
		}
		out = append(out, *e)
	}
	return out, total, rows.Err()
}

func (r *sqlGalleryRepository) Browse(ctx context.Context, q BrowseQuery) ([]GalleryEntry, int, error) {
	where, args := `g.status = 'visible'`, []any{}
	if !q.AnySys {
		where += ` AND g.system_id = ?`
		args = append(args, q.SystemID)
	}
	if len(q.Kinds) > 0 {
		where += ` AND g.kind IN (` + strings.TrimSuffix(strings.Repeat("?,", len(q.Kinds)), ",") + `)`
		for _, k := range q.Kinds {
			args = append(args, k)
		}
	}
	if q.Prefix != "" {
		where += ` AND g.kind LIKE ? ESCAPE '|'`
		args = append(args, likeEscape(q.Prefix)+"%")
	}
	if q.Text != "" {
		where += ` AND (g.name LIKE ? ESCAPE '|' OR g.maker_name LIKE ? ESCAPE '|')`
		like := "%" + likeEscape(q.Text) + "%"
		args = append(args, like, like)
	}
	return r.list(ctx, where, args, q.Limit, q.Offset)
}

func (r *sqlGalleryRepository) ByPublisher(ctx context.Context, publisher string) ([]GalleryEntry, error) {
	out, _, err := r.list(ctx, `g.publisher_user_id = ?`, []any{publisher}, 0, 0)
	return out, err
}

func (r *sqlGalleryRepository) ByStatus(ctx context.Context, statuses []string, limit, offset int) ([]GalleryEntry, int, error) {
	args := make([]any, len(statuses))
	for i, st := range statuses {
		args[i] = st
	}
	return r.list(ctx, `g.status IN (`+strings.TrimSuffix(strings.Repeat("?,", len(statuses)), ",")+`)`, args, limit, offset)
}

func (r *sqlGalleryRepository) RefusedExists(ctx context.Context, publisher, hash, exceptID string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM design_gallery WHERE publisher_user_id = ? AND data_hash = ? AND id <> ? AND status IN ('rejected','removed')`,
		publisher, hash, exceptID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking refused gallery data: %w", err)
	}
	return n > 0, nil
}

func (r *sqlGalleryRepository) FlagReported(ctx context.Context, id string, reasons []string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE design_gallery SET status = 'pending', reasons = ? WHERE id = ? AND status = 'visible'`, reasonsJSON(reasons), id)
	if err != nil {
		return false, fmt.Errorf("flagging reported entry: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sqlGalleryRepository) DuplicateExists(ctx context.Context, publisher, hash, exceptID string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM design_gallery WHERE publisher_user_id = ? AND data_hash = ? AND id <> ? AND status <> 'removed'`,
		publisher, hash, exceptID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking duplicate gallery data: %w", err)
	}
	return n > 0, nil
}

func (r *sqlGalleryRepository) CountEvents(ctx context.Context, user string, since time.Time) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM design_gallery_events WHERE user_id = ? AND created_at >= ?`, user, since).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting gallery events: %w", err)
	}
	return n, nil
}

// AddEvent records an attempt and forgets this user's events older than two
// days, which no rule reads, so the table stays small.
func (r *sqlGalleryRepository) AddEvent(ctx context.Context, user string) error {
	if _, err := r.db.ExecContext(ctx, `INSERT INTO design_gallery_events (user_id) VALUES (?)`, user); err != nil {
		return fmt.Errorf("recording gallery event: %w", err)
	}
	_, _ = r.db.ExecContext(ctx, `DELETE FROM design_gallery_events WHERE user_id = ? AND created_at < DATE_SUB(NOW(), INTERVAL 2 DAY)`, user)
	return nil
}

func (r *sqlGalleryRepository) AddReport(ctx context.Context, entryID, reporter, reason, note string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `INSERT IGNORE INTO design_gallery_reports (entry_id, reporter_user_id, reason, note) VALUES (?, ?, ?, ?)`, entryID, reporter, reason, note)
	if err != nil {
		return false, fmt.Errorf("adding report: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *sqlGalleryRepository) Reports(ctx context.Context, entryID string) ([]GalleryReport, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT reporter_user_id, reason, note, created_at FROM design_gallery_reports WHERE entry_id = ? ORDER BY created_at, reporter_user_id`, entryID)
	if err != nil {
		return nil, fmt.Errorf("listing reports: %w", err)
	}
	defer rows.Close()
	out := []GalleryReport{}
	for rows.Next() {
		var g GalleryReport
		if err := rows.Scan(&g.ReporterUserID, &g.Reason, &g.Note, &g.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning report: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *sqlGalleryRepository) ReportedBy(ctx context.Context, reporter string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	args := []any{reporter}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT entry_id FROM design_gallery_reports WHERE reporter_user_id = ? AND entry_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading reports: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning report: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (r *sqlGalleryRepository) ClearReports(ctx context.Context, entryID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM design_gallery_reports WHERE entry_id = ?`, entryID); err != nil {
		return fmt.Errorf("clearing reports: %w", err)
	}
	return nil
}
