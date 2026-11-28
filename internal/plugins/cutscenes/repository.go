// Hand-written SQL for cut scenes and their showings.
//
// SECURITY: every statement filters on campaign_id (directly, or through the
// showing for its viewers), so an id from another campaign never reads or
// changes anything here.
package cutscenes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Repository is the store behind the service.
type Repository interface {
	ListScenes(ctx context.Context, campaignID string) ([]Scene, error)
	// GetScene answers NotFound for a scene that is not in the campaign.
	GetScene(ctx context.Context, campaignID, id string) (*Scene, error)
	// CreateScene answers Conflict when the campaign has a scene of that name.
	CreateScene(ctx context.Context, campaignID, createdBy string, s Scene) error
	// UpdateScene answers NotFound for a missing scene and Conflict for a
	// name another scene has.
	UpdateScene(ctx context.Context, campaignID string, s Scene) error
	DeleteScene(ctx context.Context, campaignID, id string) error

	// CreateShowing stores the showing and its viewers in one transaction.
	CreateShowing(ctx context.Context, s Showing) error
	// GetShowing answers NotFound for a showing that is not in the campaign.
	GetShowing(ctx context.Context, campaignID, id string) (*Showing, error)
	// StopShowing stamps stopped_at once; a second stop changes nothing.
	StopShowing(ctx context.Context, campaignID, id string, at time.Time) error
	// RecentShowings lists the campaign's newest showings, viewers included.
	RecentShowings(ctx context.Context, campaignID string, limit int) ([]Showing, error)
	// ViewerShowings lists the showings userID is a viewer of whose window
	// is still open at now, newest first, each with only that viewer.
	ViewerShowings(ctx context.Context, campaignID, userID string, now time.Time) ([]Showing, error)
	// MarkWatched stamps watched_at the first time only.
	MarkWatched(ctx context.Context, campaignID, showingID, userID string, at time.Time) error
	// ReleaseMoment marks moment i of the showing let go; again changes
	// nothing.
	ReleaseMoment(ctx context.Context, campaignID, showingID string, i int) error

	// SaveAnswer stores a viewer's answer at a moment, replacing their own
	// while it waits; one the Director already decided answers Conflict.
	SaveAnswer(ctx context.Context, campaignID string, a Answer) error
	// ListAnswers lists a showing's answers, oldest first; userID "" for
	// everyone's.
	ListAnswers(ctx context.Context, campaignID, showingID, userID string) ([]Answer, error)
	// GetAnswer answers NotFound for an answer that is not the showing's.
	GetAnswer(ctx context.Context, campaignID, showingID, answerID string) (*Answer, error)
	// ResolveAnswer decides a waiting answer once; a second decision answers
	// Conflict.
	ResolveAnswer(ctx context.Context, campaignID, answerID, status, line, pageID string, at time.Time) error
	// ReopenAnswer puts an answer the Director added back to waiting, for
	// when the page refused the line after the answer was claimed.
	ReopenAnswer(ctx context.Context, campaignID, answerID string) error
}

type sqlRepository struct{ db *sql.DB }

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository { return &sqlRepository{db: db} }

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func nameTaken(name string) error {
	return apperror.NewConflict(fmt.Sprintf("a scene called %q already exists", name))
}

// sceneColumns are read in this order by scanScene.
const sceneColumns = "id, name, background, overlays, `text`, COALESCE(moments, ''), ending, updated_at"

type scanner interface{ Scan(dest ...any) error }

func scanScene(row scanner) (Scene, error) {
	var s Scene
	var bg, ov, tx, mo, end string
	if err := row.Scan(&s.ID, &s.Name, &bg, &ov, &tx, &mo, &end, &s.UpdatedAt); err != nil {
		return s, err
	}
	// A layer that no longer parses reads as empty rather than breaking the
	// list; the editor then shows it as unset and the next save repairs it.
	_ = json.Unmarshal([]byte(bg), &s.Background)
	_ = json.Unmarshal([]byte(ov), &s.Overlays)
	_ = json.Unmarshal([]byte(tx), &s.Text)
	_ = json.Unmarshal([]byte(end), &s.Ending)
	if mo != "" {
		_ = json.Unmarshal([]byte(mo), &s.Moments)
	}
	if s.Overlays == nil {
		s.Overlays = []Overlay{}
	}
	if s.Text == nil {
		s.Text = []TextLine{}
	}
	return s, nil
}

// momentsColumn is the moments column: NULL for a scene without any, so a scene
// saved before moments existed and one saved after without them read alike.
func momentsColumn(s Scene) (any, error) {
	if len(s.Moments) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(s.Moments)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func layers(s Scene) (bg, ov, tx, end []byte, err error) {
	if bg, err = json.Marshal(s.Background); err != nil {
		return
	}
	if s.Overlays == nil {
		s.Overlays = []Overlay{}
	}
	if ov, err = json.Marshal(s.Overlays); err != nil {
		return
	}
	if s.Text == nil {
		s.Text = []TextLine{}
	}
	if tx, err = json.Marshal(s.Text); err != nil {
		return
	}
	end, err = json.Marshal(s.Ending)
	return
}

func (r *sqlRepository) ListScenes(ctx context.Context, campaignID string) ([]Scene, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+sceneColumns+` FROM cutscenes WHERE campaign_id = ? ORDER BY name`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing cut scenes: %w", err)
	}
	defer rows.Close()
	out := []Scene{}
	for rows.Next() {
		s, err := scanScene(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning cut scene: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *sqlRepository) GetScene(ctx context.Context, campaignID, id string) (*Scene, error) {
	s, err := scanScene(r.db.QueryRowContext(ctx, `SELECT `+sceneColumns+` FROM cutscenes WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("scene not found")
	}
	if err != nil {
		return nil, fmt.Errorf("reading cut scene: %w", err)
	}
	return &s, nil
}

func (r *sqlRepository) CreateScene(ctx context.Context, campaignID, createdBy string, s Scene) error {
	bg, ov, tx, end, err := layers(s)
	if err != nil {
		return fmt.Errorf("encoding cut scene: %w", err)
	}
	mo, err := momentsColumn(s)
	if err != nil {
		return fmt.Errorf("encoding cut scene: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		"INSERT INTO cutscenes (id, campaign_id, name, background, overlays, `text`, moments, ending, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		s.ID, campaignID, s.Name, string(bg), string(ov), string(tx), mo, string(end), createdBy)
	if isDuplicate(err) {
		return nameTaken(s.Name)
	}
	if err != nil {
		return fmt.Errorf("creating cut scene: %w", err)
	}
	return nil
}

func (r *sqlRepository) UpdateScene(ctx context.Context, campaignID string, s Scene) error {
	bg, ov, tx, end, err := layers(s)
	if err != nil {
		return fmt.Errorf("encoding cut scene: %w", err)
	}
	mo, err := momentsColumn(s)
	if err != nil {
		return fmt.Errorf("encoding cut scene: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		"UPDATE cutscenes SET name = ?, background = ?, overlays = ?, `text` = ?, moments = ?, ending = ? WHERE campaign_id = ? AND id = ?",
		s.Name, string(bg), string(ov), string(tx), mo, string(end), campaignID, s.ID)
	if isDuplicate(err) {
		return nameTaken(s.Name)
	}
	if err != nil {
		return fmt.Errorf("saving cut scene: %w", err)
	}
	// MariaDB counts changed rows, so an unchanged save reports 0; tell a
	// missing scene apart by looking.
	if n, _ := res.RowsAffected(); n == 0 {
		var one int
		err := r.db.QueryRowContext(ctx, `SELECT 1 FROM cutscenes WHERE campaign_id = ? AND id = ?`, campaignID, s.ID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return apperror.NewNotFound("scene not found")
		}
		if err != nil {
			return fmt.Errorf("checking cut scene: %w", err)
		}
	}
	return nil
}

func (r *sqlRepository) DeleteScene(ctx context.Context, campaignID, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM cutscenes WHERE campaign_id = ? AND id = ?`, campaignID, id)
	if err != nil {
		return fmt.Errorf("deleting cut scene: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("scene not found")
	}
	return nil
}

func (r *sqlRepository) CreateShowing(ctx context.Context, s Showing) error {
	snap, err := json.Marshal(s.Scene)
	if err != nil {
		return fmt.Errorf("encoding showing: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting showing: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var sceneID any
	if s.SceneID != "" {
		sceneID = s.SceneID
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO cutscene_showings (id, campaign_id, scene_id, scene, shown_by, shown_at, rewatch_until) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.CampaignID, sceneID, string(snap), s.ShownBy, s.ShownAt.UTC(), s.RewatchUntil.UTC()); err != nil {
		return fmt.Errorf("creating showing: %w", err)
	}
	for _, v := range s.Viewers {
		if _, err := tx.ExecContext(ctx, `INSERT INTO cutscene_showing_viewers (showing_id, user_id) VALUES (?, ?)`, s.ID, v.UserID); err != nil {
			return fmt.Errorf("adding showing viewer: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing showing: %w", err)
	}
	return nil
}

const showingColumns = "s.id, s.campaign_id, s.scene_id, s.scene, s.shown_by, s.shown_at, s.rewatch_until, s.stopped_at, s.released"

func scanShowing(row scanner, extra ...any) (Showing, error) {
	var sh Showing
	var sceneID sql.NullString
	var snap string
	var stopped sql.NullTime
	dest := append([]any{&sh.ID, &sh.CampaignID, &sceneID, &snap, &sh.ShownBy, &sh.ShownAt, &sh.RewatchUntil, &stopped, &sh.Released}, extra...)
	if err := row.Scan(dest...); err != nil {
		return sh, err
	}
	sh.SceneID = sceneID.String
	if stopped.Valid {
		t := stopped.Time
		sh.StoppedAt = &t
	}
	_ = json.Unmarshal([]byte(snap), &sh.Scene)
	return sh, nil
}

func (r *sqlRepository) GetShowing(ctx context.Context, campaignID, id string) (*Showing, error) {
	sh, err := scanShowing(r.db.QueryRowContext(ctx, `SELECT `+showingColumns+` FROM cutscene_showings s WHERE s.campaign_id = ? AND s.id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("showing not found")
	}
	if err != nil {
		return nil, fmt.Errorf("reading showing: %w", err)
	}
	list := []Showing{sh}
	if err := r.attachViewers(ctx, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// attachViewers fills each showing's viewers with one query.
func (r *sqlRepository) attachViewers(ctx context.Context, list []Showing) error {
	if len(list) == 0 {
		return nil
	}
	idx := make(map[string]int, len(list))
	args := make([]any, 0, len(list))
	for i, s := range list {
		idx[s.ID] = i
		args = append(args, s.ID)
	}
	q := `SELECT showing_id, user_id, watched_at FROM cutscene_showing_viewers WHERE showing_id IN (?` + strings.Repeat(", ?", len(list)-1) + `) ORDER BY showing_id, user_id`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("listing showing viewers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid string
		var v ShowingViewer
		var w sql.NullTime
		if err := rows.Scan(&sid, &v.UserID, &w); err != nil {
			return fmt.Errorf("scanning showing viewer: %w", err)
		}
		if w.Valid {
			t := w.Time
			v.WatchedAt = &t
		}
		if i, ok := idx[sid]; ok {
			list[i].Viewers = append(list[i].Viewers, v)
		}
	}
	return rows.Err()
}

func (r *sqlRepository) StopShowing(ctx context.Context, campaignID, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cutscene_showings SET stopped_at = ? WHERE campaign_id = ? AND id = ? AND stopped_at IS NULL`, at.UTC(), campaignID, id)
	if err != nil {
		return fmt.Errorf("stopping showing: %w", err)
	}
	return nil
}

func (r *sqlRepository) RecentShowings(ctx context.Context, campaignID string, limit int) ([]Showing, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+showingColumns+` FROM cutscene_showings s WHERE s.campaign_id = ? ORDER BY s.shown_at DESC, s.id LIMIT ?`, campaignID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing showings: %w", err)
	}
	defer rows.Close()
	out := []Showing{}
	for rows.Next() {
		sh, err := scanShowing(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning showing: %w", err)
		}
		out = append(out, sh)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return out, r.attachViewers(ctx, out)
}

func (r *sqlRepository) ViewerShowings(ctx context.Context, campaignID, userID string, now time.Time) ([]Showing, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+showingColumns+`, v.watched_at
		FROM cutscene_showings s JOIN cutscene_showing_viewers v ON v.showing_id = s.id
		WHERE s.campaign_id = ? AND v.user_id = ? AND s.rewatch_until > ?
		ORDER BY s.shown_at DESC, s.id`, campaignID, userID, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("listing viewer showings: %w", err)
	}
	defer rows.Close()
	out := []Showing{}
	for rows.Next() {
		var w sql.NullTime
		sh, err := scanShowing(rows, &w)
		if err != nil {
			return nil, fmt.Errorf("scanning viewer showing: %w", err)
		}
		v := ShowingViewer{UserID: userID}
		if w.Valid {
			t := w.Time
			v.WatchedAt = &t
		}
		sh.Viewers = []ShowingViewer{v}
		out = append(out, sh)
	}
	return out, rows.Err()
}

func (r *sqlRepository) MarkWatched(ctx context.Context, campaignID, showingID, userID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cutscene_showing_viewers v JOIN cutscene_showings s ON s.id = v.showing_id
		SET v.watched_at = ? WHERE s.campaign_id = ? AND v.showing_id = ? AND v.user_id = ? AND v.watched_at IS NULL`,
		at.UTC(), campaignID, showingID, userID)
	if err != nil {
		return fmt.Errorf("marking showing watched: %w", err)
	}
	return nil
}

func (r *sqlRepository) ReleaseMoment(ctx context.Context, campaignID, showingID string, i int) error {
	if i < 0 || i >= 31 {
		return apperror.NewBadRequest("no such moment")
	}
	_, err := r.db.ExecContext(ctx, `UPDATE cutscene_showings SET released = released | ? WHERE campaign_id = ? AND id = ?`, 1<<uint(i), campaignID, showingID)
	if err != nil {
		return fmt.Errorf("letting a moment go: %w", err)
	}
	return nil
}

func answerDecided() error {
	return apperror.NewConflict("Your Director has already read this answer.")
}

func (r *sqlRepository) SaveAnswer(ctx context.Context, campaignID string, a Answer) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting answer: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var id, status string
	err = tx.QueryRowContext(ctx, `SELECT a.id, a.status FROM cutscene_moment_answers a JOIN cutscene_showings s ON s.id = a.showing_id
		WHERE s.campaign_id = ? AND a.showing_id = ? AND a.moment = ? AND a.user_id = ? FOR UPDATE`,
		campaignID, a.ShowingID, a.Moment, a.UserID).Scan(&id, &status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// The showing is read through its campaign, so an id from another
		// campaign inserts nothing.
		res, err := tx.ExecContext(ctx, "INSERT INTO cutscene_moment_answers (id, showing_id, moment, user_id, way, `text`, status, created_at) "+
			"SELECT ?, s.id, ?, ?, ?, ?, ?, ? FROM cutscene_showings s WHERE s.campaign_id = ? AND s.id = ?",
			a.ID, a.Moment, a.UserID, a.Way, a.Text, AnswerWaiting, a.CreatedAt.UTC(), campaignID, a.ShowingID)
		if isDuplicate(err) {
			return answerDecided()
		}
		if err != nil {
			return fmt.Errorf("saving answer: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return apperror.NewNotFound("showing not found")
		}
	case err != nil:
		return fmt.Errorf("reading answer: %w", err)
	case status != AnswerWaiting:
		return answerDecided()
	default:
		if _, err := tx.ExecContext(ctx, "UPDATE cutscene_moment_answers SET way = ?, `text` = ?, created_at = ? WHERE id = ?",
			a.Way, a.Text, a.CreatedAt.UTC(), id); err != nil {
			return fmt.Errorf("saving answer: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing answer: %w", err)
	}
	return nil
}

const answerColumns = "a.id, a.showing_id, a.moment, a.user_id, a.way, a.`text`, a.status, a.line, a.page_id, a.created_at, a.resolved_at"

func scanAnswer(row scanner) (Answer, error) {
	var a Answer
	var line, page sql.NullString
	var resolved sql.NullTime
	if err := row.Scan(&a.ID, &a.ShowingID, &a.Moment, &a.UserID, &a.Way, &a.Text, &a.Status, &line, &page, &a.CreatedAt, &resolved); err != nil {
		return a, err
	}
	a.Line, a.PageID = line.String, page.String
	if resolved.Valid {
		t := resolved.Time
		a.ResolvedAt = &t
	}
	return a, nil
}

func (r *sqlRepository) ListAnswers(ctx context.Context, campaignID, showingID, userID string) ([]Answer, error) {
	q := `SELECT ` + answerColumns + ` FROM cutscene_moment_answers a JOIN cutscene_showings s ON s.id = a.showing_id
		WHERE s.campaign_id = ? AND a.showing_id = ?`
	args := []any{campaignID, showingID}
	if userID != "" {
		q += ` AND a.user_id = ?`
		args = append(args, userID)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY a.created_at, a.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing answers: %w", err)
	}
	defer rows.Close()
	out := []Answer{}
	for rows.Next() {
		a, err := scanAnswer(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning answer: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *sqlRepository) GetAnswer(ctx context.Context, campaignID, showingID, answerID string) (*Answer, error) {
	a, err := scanAnswer(r.db.QueryRowContext(ctx, `SELECT `+answerColumns+` FROM cutscene_moment_answers a JOIN cutscene_showings s ON s.id = a.showing_id
		WHERE s.campaign_id = ? AND a.showing_id = ? AND a.id = ?`, campaignID, showingID, answerID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("answer not found")
	}
	if err != nil {
		return nil, fmt.Errorf("reading answer: %w", err)
	}
	return &a, nil
}

func (r *sqlRepository) ResolveAnswer(ctx context.Context, campaignID, answerID, status, line, pageID string, at time.Time) error {
	var l, p any
	if line != "" {
		l = line
	}
	if pageID != "" {
		p = pageID
	}
	res, err := r.db.ExecContext(ctx, `UPDATE cutscene_moment_answers a JOIN cutscene_showings s ON s.id = a.showing_id
		SET a.status = ?, a.line = ?, a.page_id = ?, a.resolved_at = ?
		WHERE s.campaign_id = ? AND a.id = ? AND a.status = ?`, status, l, p, at.UTC(), campaignID, answerID, AnswerWaiting)
	if err != nil {
		return fmt.Errorf("deciding answer: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewConflict("This answer was already decided.")
	}
	return nil
}

func (r *sqlRepository) ReopenAnswer(ctx context.Context, campaignID, answerID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cutscene_moment_answers a JOIN cutscene_showings s ON s.id = a.showing_id
		SET a.status = ?, a.line = NULL, a.page_id = NULL, a.resolved_at = NULL
		WHERE s.campaign_id = ? AND a.id = ? AND a.status IN (?, ?)`, AnswerWaiting, campaignID, answerID, AnswerAdded, AnswerChanged)
	if err != nil {
		return fmt.Errorf("reopening answer: %w", err)
	}
	return nil
}
