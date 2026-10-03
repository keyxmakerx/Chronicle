package notes

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// An app grant is a player's permission for an app outside Chronicle (the
// Foundry notebook) to use that player's notes in one campaign. It never
// widens what the player can do: every request still runs as the player at
// their live campaign role, so leaving the campaign ends it.

// appGrantTokenPrefix marks a notes grant token so it is recognisable in a
// leak report and never mistaken for a sync API key.
const appGrantTokenPrefix = "cnt_"

// appGrantIdleLimit ends a grant nobody has used for this long. A notebook
// in regular use never reaches it.
const appGrantIdleLimit = 90 * 24 * time.Hour

// appGrantTouchEvery limits last_used_at writes to one per grant per window,
// so reading notes doesn't write a row on every request.
const appGrantTouchEvery = 5 * time.Minute

// maxAppGrantsPerUser caps live grants per player per campaign. Each Allow
// makes one; a player who allows from many browsers keeps the newest.
const maxAppGrantsPerUser = 10

// AppGrant is one player's grant, as shown back to them. The token is never
// part of it.
type AppGrant struct {
	ID         string     `json:"id"`
	CampaignID string     `json:"campaignId"`
	UserID     string     `json:"userId"`
	Origin     string     `json:"origin"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"-"`
}

// usable reports whether the grant may still authenticate at now.
func (g *AppGrant) usable(now time.Time) bool {
	if g.RevokedAt != nil {
		return false
	}
	last := g.CreatedAt
	if g.LastUsedAt != nil && g.LastUsedAt.After(last) {
		last = *g.LastUsedAt
	}
	return now.Sub(last) < appGrantIdleLimit
}

// AppGrantRepository is the data access for notes_app_grants.
type AppGrantRepository interface {
	Create(ctx context.Context, g *AppGrant, tokenHash string) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*AppGrant, error)
	ListLive(ctx context.Context, campaignID, userID string) ([]AppGrant, error)
	Revoke(ctx context.Context, id string, at time.Time) error
	Touch(ctx context.Context, id string, at time.Time) error
	RevokeAllForUser(ctx context.Context, userID string, at time.Time) error
}

// AppGrantService issues, checks and revokes app grants.
type AppGrantService interface {
	// Issue makes a grant for userID in campaignID and returns the token,
	// which is not stored and cannot be recovered. origin must already have
	// been checked against the allowed origins by the caller.
	Issue(ctx context.Context, campaignID, userID, origin string) (token string, g *AppGrant, err error)

	// Authenticate returns the live grant for token, or an Unauthorized error.
	Authenticate(ctx context.Context, token string) (*AppGrant, error)

	// List returns the player's live grants in the campaign, newest first.
	List(ctx context.Context, campaignID, userID string) ([]AppGrant, error)

	// Revoke ends one of the player's own grants. Someone else's grant, or
	// one in another campaign, is NotFound.
	Revoke(ctx context.Context, campaignID, userID, grantID string) error

	// RevokeAllForUser ends every grant the user holds, in every campaign.
	// Runs when all the user's sessions are destroyed (password reset or
	// change, force sign-out), so a grant never outlives them.
	RevokeAllForUser(ctx context.Context, userID string) error
}

type appGrantService struct {
	repo AppGrantRepository
	now  func() time.Time
}

// NewAppGrantService creates the app grant service.
func NewAppGrantService(repo AppGrantRepository) AppGrantService {
	return &appGrantService{repo: repo, now: time.Now}
}

// hashAppGrantToken is the stored form of a token. The token carries 256
// random bits, so a plain SHA-256 is enough; a slow hash would only slow
// down every notes request.
func hashAppGrantToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newAppGrantToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating notes grant token: %w", err)
	}
	return appGrantTokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *appGrantService) Issue(ctx context.Context, campaignID, userID, origin string) (string, *AppGrant, error) {
	if campaignID == "" || userID == "" || origin == "" {
		return "", nil, apperror.NewBadRequest("campaign, user and origin are required")
	}
	token, err := newAppGrantToken()
	if err != nil {
		return "", nil, apperror.NewInternal(err)
	}
	now := s.now().UTC().Truncate(time.Second)
	g := &AppGrant{
		ID:         generateID(),
		CampaignID: campaignID,
		UserID:     userID,
		Origin:     origin,
		CreatedAt:  now,
	}
	if err := s.repo.Create(ctx, g, hashAppGrantToken(token)); err != nil {
		return "", nil, apperror.NewInternal(err)
	}

	// Keep the newest few; the oldest past the cap are revoked so a grant
	// left in a forgotten browser doesn't stay valid forever.
	live, err := s.repo.ListLive(ctx, campaignID, userID)
	if err == nil && len(live) > maxAppGrantsPerUser {
		for _, old := range live[maxAppGrantsPerUser:] {
			_ = s.repo.Revoke(ctx, old.ID, now)
		}
	}
	return token, g, nil
}

func (s *appGrantService) Authenticate(ctx context.Context, token string) (*AppGrant, error) {
	if !strings.HasPrefix(token, appGrantTokenPrefix) {
		return nil, apperror.NewUnauthorized("notes access not allowed")
	}
	g, err := s.repo.FindByTokenHash(ctx, hashAppGrantToken(token))
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == 404 {
			return nil, apperror.NewUnauthorized("notes access not allowed")
		}
		return nil, apperror.NewInternal(err)
	}
	now := s.now().UTC()
	if !g.usable(now) {
		return nil, apperror.NewUnauthorized("notes access not allowed")
	}
	if g.LastUsedAt == nil || now.Sub(*g.LastUsedAt) >= appGrantTouchEvery {
		// Best effort: a failed touch must not fail the request.
		_ = s.repo.Touch(ctx, g.ID, now.Truncate(time.Second))
	}
	return g, nil
}

func (s *appGrantService) List(ctx context.Context, campaignID, userID string) ([]AppGrant, error) {
	all, err := s.repo.ListLive(ctx, campaignID, userID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	now := s.now().UTC()
	out := make([]AppGrant, 0, len(all))
	for _, g := range all {
		if g.usable(now) {
			out = append(out, g)
		}
	}
	return out, nil
}

func (s *appGrantService) Revoke(ctx context.Context, campaignID, userID, grantID string) error {
	live, err := s.repo.ListLive(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	for _, g := range live {
		if g.ID == grantID {
			if err := s.repo.Revoke(ctx, g.ID, s.now().UTC().Truncate(time.Second)); err != nil {
				return apperror.NewInternal(err)
			}
			return nil
		}
	}
	return apperror.NewNotFound("connection not found")
}

func (s *appGrantService) RevokeAllForUser(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	if err := s.repo.RevokeAllForUser(ctx, userID, s.now().UTC().Truncate(time.Second)); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// --- repository ---

type appGrantRepository struct {
	db *sql.DB
}

// NewAppGrantRepository creates the MariaDB-backed app grant repository.
func NewAppGrantRepository(db *sql.DB) AppGrantRepository {
	return &appGrantRepository{db: db}
}

const appGrantColumns = `id, campaign_id, user_id, origin, created_at, last_used_at, revoked_at`

// appGrantColumnsG is appGrantColumns qualified for a query joining users.
const appGrantColumnsG = `g.id, g.campaign_id, g.user_id, g.origin, g.created_at, g.last_used_at, g.revoked_at`

func scanAppGrant(row interface{ Scan(...any) error }) (*AppGrant, error) {
	var g AppGrant
	var lastUsed, revoked sql.NullTime
	if err := row.Scan(&g.ID, &g.CampaignID, &g.UserID, &g.Origin, &g.CreatedAt, &lastUsed, &revoked); err != nil {
		return nil, err
	}
	if lastUsed.Valid {
		t := lastUsed.Time
		g.LastUsedAt = &t
	}
	if revoked.Valid {
		t := revoked.Time
		g.RevokedAt = &t
	}
	return &g, nil
}

func (r *appGrantRepository) Create(ctx context.Context, g *AppGrant, tokenHash string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notes_app_grants (id, campaign_id, user_id, token_hash, origin, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		g.ID, g.CampaignID, g.UserID, tokenHash, g.Origin, g.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating notes app grant: %w", err)
	}
	return nil
}

func (r *appGrantRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*AppGrant, error) {
	// A disabled account's grants stop at once, as its sessions do.
	g, err := scanAppGrant(r.db.QueryRowContext(ctx,
		`SELECT `+appGrantColumnsG+` FROM notes_app_grants g
		 JOIN users u ON u.id = g.user_id
		 WHERE g.token_hash = ? AND u.is_disabled = FALSE`, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("grant not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding notes app grant: %w", err)
	}
	return g, nil
}

func (r *appGrantRepository) ListLive(ctx context.Context, campaignID, userID string) ([]AppGrant, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+appGrantColumns+` FROM notes_app_grants
		 WHERE campaign_id = ? AND user_id = ? AND revoked_at IS NULL
		 ORDER BY created_at DESC, id DESC`, campaignID, userID)
	if err != nil {
		return nil, fmt.Errorf("listing notes app grants: %w", err)
	}
	defer rows.Close()
	var out []AppGrant
	for rows.Next() {
		g, err := scanAppGrant(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning notes app grant: %w", err)
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (r *appGrantRepository) Revoke(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notes_app_grants SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, at, id)
	if err != nil {
		return fmt.Errorf("revoking notes app grant: %w", err)
	}
	return nil
}

func (r *appGrantRepository) Touch(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notes_app_grants SET last_used_at = ? WHERE id = ?`, at, id)
	if err != nil {
		return fmt.Errorf("touching notes app grant: %w", err)
	}
	return nil
}

func (r *appGrantRepository) RevokeAllForUser(ctx context.Context, userID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notes_app_grants SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, at, userID)
	if err != nil {
		return fmt.Errorf("revoking notes app grants for user: %w", err)
	}
	return nil
}
