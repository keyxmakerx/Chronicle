package entity_notes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Repository defines the data access interface for entity notes.
// All read methods enforce the audience ACL inline; callers cannot
// accidentally bypass it by issuing a different query.
type Repository interface {
	// Create inserts a new note. ID, CreatedAt, UpdatedAt must be set
	// by the service before this call.
	Create(ctx context.Context, note *Note) error

	// FindByID returns a note by ID with the audience ACL applied.
	// Returns (nil, nil) if the note exists but the viewer cannot see
	// it — that's distinct from a real "not found" because the caller
	// shouldn't leak existence to unprivileged readers.
	FindByID(ctx context.Context, id string, viewer ViewerContext) (*Note, error)

	// FindByIDForAuthor returns a note ONLY if author_user_id matches.
	// Used by Update/Delete which restrict mutations to the author.
	FindByIDForAuthor(ctx context.Context, id, authorUserID string) (*Note, error)

	// ListByEntity returns notes on the entity that the viewer can see,
	// newest first. Pinned notes float to the top.
	ListByEntity(ctx context.Context, entityID string, viewer ViewerContext) ([]Note, error)

	// Update overwrites a note's mutable fields. The service must have
	// verified author ownership before calling this.
	Update(ctx context.Context, note *Note) error

	// Delete removes a note by ID. Caller responsible for author check.
	Delete(ctx context.Context, id string) error
}

// ViewerContext bundles the audience-relevant facts about the viewer
// plus the campaign they're acting in. Built once at the handler
// boundary from CampaignContext, then passed through to the repo so
// SQL parameters stay simple and the ACL is computed in one place.
type ViewerContext struct {
	UserID      string // viewer's user_id (matches author_user_id when reading own notes)
	CampaignID  string // campaign the viewer is acting in (FK target on writes)
	IsOwner     bool   // viewer has RoleOwner on this campaign
	IsScribe    bool   // viewer has RoleScribe (Co-DM equivalent) on this campaign
	IsDMGranted bool   // viewer has the per-user IsDmGranted flag set
}

// CanSeeDMScribe reports whether the viewer's role/grants admit
// `dm_scribe` notes. RoleOwner > RoleScribe in the campaign role enum,
// so owners qualify implicitly.
func (v ViewerContext) CanSeeDMScribe() bool {
	return v.IsOwner || v.IsScribe || v.IsDMGranted
}

// CanSeeDMOnly reports whether the viewer's role/grants admit
// `dm_only` notes. Scribe alone does NOT qualify — you have to be
// the Owner or be explicitly DM-granted. Per the docstring on
// campaigns.IsDmGranted, the flag's whole purpose is "dm_only
// visibility without role promotion."
func (v ViewerContext) CanSeeDMOnly() bool {
	return v.IsOwner || v.IsDMGranted
}

const noteColumns = `id, entity_id, campaign_id, author_user_id, audience,
	shared_with, title, body, body_html, pinned, created_at, updated_at`

// noteACLFilter is the WHERE-fragment that enforces audience visibility.
// Five parameters follow whatever WHERE prefix the caller provides:
//   1. viewer's user_id (matches author_user_id for "own notes" branch)
//   2. viewer's user_id (matches JSON_CONTAINS for `custom` audience)
//   3. viewer.CanSeeDMScribe() boolean
//   4. viewer.CanSeeDMOnly()   boolean
//
// The order matters — read it once, then never reorder without updating
// every call site. The pure-Go mirror NotePassesACL pins the same logic
// in service_test.go so a regression in EITHER the SQL or the Go code
// (someone editing this filter without updating the helper, or vice
// versa) can be caught by `go test`. Without that mirror, the headline
// privacy invariant ("Owner cannot read another user's private note")
// has no automated test — manual MariaDB checks don't survive refactors.
const noteACLFilter = `(
    author_user_id = ?
    OR audience = 'everyone'
    OR (audience = 'custom'    AND shared_with IS NOT NULL AND JSON_CONTAINS(shared_with, JSON_QUOTE(?), '$'))
    OR (audience = 'dm_scribe' AND ?)
    OR (audience = 'dm_only'   AND ?)
)`

// NotePassesACL is the pure-Go mirror of noteACLFilter. The repo's SQL
// is the production filter; this function exists so tests can exercise
// the audience matrix in process. The two MUST stay in lockstep — any
// change to the SQL must be reflected here, and vice versa.
//
// Used by service_test.go's full audience×viewer matrix. If you're
// reading this because a security review caught a leak, check that
// the SQL and this function still tell the same story for every
// (audience, author, viewer-flags) combination.
func NotePassesACL(note *Note, viewer ViewerContext) bool {
	if note == nil {
		return false
	}
	// Author always sees own notes regardless of audience.
	if note.AuthorUserID == viewer.UserID {
		return true
	}
	switch note.Audience {
	case AudienceEveryone:
		return true
	case AudienceCustom:
		for _, id := range note.SharedWith {
			if id == viewer.UserID {
				return true
			}
		}
		return false
	case AudienceDMScribe:
		return viewer.CanSeeDMScribe()
	case AudienceDMOnly:
		return viewer.CanSeeDMOnly()
	case AudiencePrivate:
		// Already handled by author check above; reach here only if
		// viewer is NOT the author, in which case private = no access.
		return false
	}
	// Unknown audience defaults to deny (defense in depth — DB enum
	// should have rejected it long before this).
	return false
}

type repository struct {
	db *sql.DB
}

// NewRepository constructs a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, n *Note) error {
	sharedWithJSON, err := encodeSharedWith(n.SharedWith)
	if err != nil {
		return err
	}
	body, err := encodeBody(n.Body)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO entity_notes (
			id, entity_id, campaign_id, author_user_id, audience,
			shared_with, title, body, body_html, pinned
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.EntityID, n.CampaignID, n.AuthorUserID, string(n.Audience),
		sharedWithJSON, nullableString(n.Title), body, nullableString(n.BodyHTML), n.Pinned,
	)
	return err
}

func (r *repository) FindByID(ctx context.Context, id string, viewer ViewerContext) (*Note, error) {
	query := `SELECT ` + noteColumns + ` FROM entity_notes
		WHERE id = ? AND ` + noteACLFilter
	row := r.db.QueryRowContext(ctx, query,
		id,
		viewer.UserID,
		viewer.UserID,
		viewer.CanSeeDMScribe(),
		viewer.CanSeeDMOnly(),
	)
	n, err := scanRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return n, err
}

func (r *repository) FindByIDForAuthor(ctx context.Context, id, authorUserID string) (*Note, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+noteColumns+` FROM entity_notes WHERE id = ? AND author_user_id = ?`,
		id, authorUserID,
	)
	n, err := scanRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return n, err
}

func (r *repository) ListByEntity(ctx context.Context, entityID string, viewer ViewerContext) ([]Note, error) {
	query := `SELECT ` + noteColumns + ` FROM entity_notes
		WHERE entity_id = ? AND ` + noteACLFilter + `
		ORDER BY pinned DESC, updated_at DESC`
	rows, err := r.db.QueryContext(ctx, query,
		entityID,
		viewer.UserID,
		viewer.UserID,
		viewer.CanSeeDMScribe(),
		viewer.CanSeeDMOnly(),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]Note, 0)
	for rows.Next() {
		n, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

func (r *repository) Update(ctx context.Context, n *Note) error {
	sharedWithJSON, err := encodeSharedWith(n.SharedWith)
	if err != nil {
		return err
	}
	body, err := encodeBody(n.Body)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE entity_notes
		    SET audience = ?, shared_with = ?, title = ?, body = ?, body_html = ?, pinned = ?
		  WHERE id = ?`,
		string(n.Audience), sharedWithJSON, nullableString(n.Title), body,
		nullableString(n.BodyHTML), n.Pinned, n.ID,
	)
	return err
}

func (r *repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM entity_notes WHERE id = ?`, id)
	return err
}

// --- helpers ---

func encodeSharedWith(ids []string) (any, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode shared_with: %w", err)
	}
	return string(b), nil
}

func encodeBody(body json.RawMessage) (any, error) {
	if len(body) == 0 {
		return nil, nil
	}
	// Validate it's parseable JSON to keep junk out of the column.
	var probe any
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, fmt.Errorf("body is not valid JSON: %w", err)
	}
	return string(body), nil
}

// nullableString returns nil for empty strings so DEFAULT NULL columns
// stay NULL rather than getting an empty-string sentinel.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func scanRow(row *sql.Row) (*Note, error) {
	var n Note
	var sharedWith, title, body, bodyHTML sql.NullString
	if err := row.Scan(
		&n.ID, &n.EntityID, &n.CampaignID, &n.AuthorUserID, &n.Audience,
		&sharedWith, &title, &body, &bodyHTML, &n.Pinned,
		&n.CreatedAt, &n.UpdatedAt,
	); err != nil {
		return nil, err
	}
	hydrate(&n, sharedWith, title, body, bodyHTML)
	return &n, nil
}

func scanRows(rows *sql.Rows) (*Note, error) {
	var n Note
	var sharedWith, title, body, bodyHTML sql.NullString
	if err := rows.Scan(
		&n.ID, &n.EntityID, &n.CampaignID, &n.AuthorUserID, &n.Audience,
		&sharedWith, &title, &body, &bodyHTML, &n.Pinned,
		&n.CreatedAt, &n.UpdatedAt,
	); err != nil {
		return nil, err
	}
	hydrate(&n, sharedWith, title, body, bodyHTML)
	return &n, nil
}

// hydrate fills the note's nullable string fields and decodes JSON.
// SharedWith always lands as a non-nil slice (possibly empty) so the
// JSON response is shape-stable for frontend consumers.
func hydrate(n *Note, sharedWith, title, body, bodyHTML sql.NullString) {
	n.SharedWith = []string{}
	if sharedWith.Valid && sharedWith.String != "" {
		_ = json.Unmarshal([]byte(sharedWith.String), &n.SharedWith)
		if n.SharedWith == nil {
			n.SharedWith = []string{}
		}
	}
	if title.Valid {
		n.Title = title.String
	}
	if body.Valid && body.String != "" {
		n.Body = json.RawMessage(body.String)
	}
	if bodyHTML.Valid {
		n.BodyHTML = bodyHTML.String
	}
}
