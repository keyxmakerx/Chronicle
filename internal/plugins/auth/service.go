package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/argon2"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// sessionKeyPrefix is the Redis key prefix for session data.
const sessionKeyPrefix = "session:"

// userSessionsKeyPrefix is the Redis key prefix for the set of session tokens
// belonging to a user. Used to invalidate all sessions on password reset.
const userSessionsKeyPrefix = "user_sessions:"

// sessionTokenBytes is the number of random bytes in a session token.
// 32 bytes = 256 bits of entropy, hex-encoded to 64 characters.
const sessionTokenBytes = 32

// resetTokenBytes is the number of random bytes in a password reset token.
const resetTokenBytes = 32

// resetTokenExpiry is how long a password reset link stays valid.
const resetTokenExpiry = 1 * time.Hour

// sessionRevalidateInterval is how often sessions are checked against the
// database to detect user deletions, disablements, or privilege changes
// that occurred outside the normal service layer (e.g., direct DB edits,
// database wipe while Redis persists). 5 minutes balances security
// responsiveness against database load.
const sessionRevalidateInterval = 5 * time.Minute

// argon2id parameters tuned for a self-hosted application running on
// modest hardware (2-4 CPU cores, 2-4 GB RAM). These follow OWASP
// recommendations for argon2id: memory=64MB, iterations=3, parallelism=4.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MB in KiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// MailSender sends email for password reset and other auth-related flows.
// Matches smtp.MailService to avoid importing the smtp package directly.
type MailSender interface {
	SendMail(ctx context.Context, to []string, subject, body string) error
	IsConfigured(ctx context.Context) bool
}

// AuthService defines the business logic contract for authentication.
// Handlers call these methods -- they never touch the repository directly.
type AuthService interface {
	Register(ctx context.Context, input RegisterInput) (*User, error)
	Login(ctx context.Context, input LoginInput) (token string, user *User, err error)
	ValidateSession(ctx context.Context, token string) (*Session, error)
	DestroySession(ctx context.Context, token string) error

	// Password reset flow.
	InitiatePasswordReset(ctx context.Context, email string) error
	ValidateResetToken(ctx context.Context, token string) (email string, err error)
	ResetPassword(ctx context.Context, token, newPassword string) error

	// User profile.
	GetUser(ctx context.Context, userID string) (*User, error)
	UpdateTimezone(ctx context.Context, userID, timezone string) error
	UpdateDisplayName(ctx context.Context, userID, displayName string) error
	UpdateAvatarPath(ctx context.Context, userID string, avatarPath *string) error
	ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error

	// Email change with verification.
	RequestEmailChange(ctx context.Context, userID, newEmail, currentPassword string) error
	ConfirmEmailChange(ctx context.Context, token string) error

	// Admin session management.
	ListAllSessions(ctx context.Context) ([]SessionInfo, error)
	DestroyAllUserSessions(ctx context.Context, userID string) (int, error)

	// DestroySessionByHash finds a session by the SHA-256 hash of its token
	// and destroys it. Used by the admin dashboard to avoid exposing raw tokens.
	DestroySessionByHash(ctx context.Context, tokenHash string) error

	// Re-authentication for sensitive operations.
	ConfirmReauth(ctx context.Context, userID, password string) error
	IsReauthValid(ctx context.Context, userID string) (bool, error)
}

// authService implements AuthService with argon2id hashing and Redis sessions.
type authService struct {
	repo       UserRepository
	redis      *redis.Client
	mail       MailSender
	baseURL    string
	sessionTTL time.Duration
}

// NewAuthService creates a new auth service with the given dependencies.
func NewAuthService(repo UserRepository, rdb *redis.Client, sessionTTL time.Duration) AuthService {
	return &authService{
		repo:       repo,
		redis:      rdb,
		sessionTTL: sessionTTL,
	}
}

// ConfigureMailSender wires a mail sender into the auth service for password
// reset emails. Called from routes.go after both services are initialized.
func ConfigureMailSender(svc AuthService, mail MailSender, baseURL string) {
	if s, ok := svc.(*authService); ok {
		s.mail = mail
		s.baseURL = baseURL
	}
}

// Register creates a new user account. It validates uniqueness, hashes the
// password with argon2id, generates a UUID, and persists the user.
func (s *authService) Register(ctx context.Context, input RegisterInput) (*User, error) {
	// Check if email is already taken before doing expensive hashing.
	exists, err := s.repo.EmailExists(ctx, input.Email)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("checking email: %w", err))
	}
	if exists {
		return nil, apperror.NewConflict("registration failed — please try a different email or log in")
	}

	// Hash the password with argon2id (memory-hard, GPU-resistant).
	hash, err := hashPassword(input.Password)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("hashing password: %w", err))
	}

	// The very first user to register becomes the site admin automatically.
	isAdmin := false
	userCount, err := s.repo.CountUsers(ctx)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("counting users: %w", err))
	}
	if userCount == 0 {
		isAdmin = true
	}

	user := &User{
		ID:           generateUUID(),
		Email:        strings.ToLower(strings.TrimSpace(input.Email)),
		DisplayName:  strings.TrimSpace(input.DisplayName),
		PasswordHash: hash,
		IsAdmin:      isAdmin,
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating user: %w", err))
	}

	slog.Info("user registered",
		slog.String("user_id", user.ID),
		slog.String("email", user.Email),
		slog.Bool("is_admin", user.IsAdmin),
	)

	return user, nil
}

// loginThrottleMax is the maximum number of failed login attempts per email
// before throttling kicks in. Uses progressive delay after this threshold.
const loginThrottleMax = 10

// loginThrottleWindow is how long failed attempt counters persist in Redis.
const loginThrottleWindow = 15 * time.Minute

// loginThrottleMaxDelay caps the progressive delay applied after exceeding
// loginThrottleMax failures. The delay doubles each attempt (2s, 4s, 8s, ...)
// but never exceeds this ceiling.
const loginThrottleMaxDelay = 5 * time.Minute

// loginFailureKeyPrefix is the Redis key prefix for per-email failure counters.
// Uses SHA-256 of the email to avoid storing plaintext emails in Redis.
const loginFailureKeyPrefix = "login_failures:"

// Login authenticates a user by email and password. On success it creates a
// new session in Redis and returns the session token for the cookie.
//
// Per-email throttling: after 10 failed attempts within 15 minutes, further
// login attempts are rejected regardless of password correctness. This
// defends against credential stuffing from distributed IPs that bypass
// per-IP rate limiting.
func (s *authService) Login(ctx context.Context, input LoginInput) (string, *User, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))

	// Apply progressive delay when the failure count exceeds the threshold.
	// Instead of hard-rejecting, we slow down attempts to frustrate brute-force
	// attacks while still allowing legitimate users to eventually authenticate.
	if delay := s.loginThrottleDelay(ctx, email); delay > 0 {
		slog.Info("login throttle delay applied",
			slog.String("email_hash", loginFailureKey(email)),
			slog.Duration("delay", delay),
		)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", nil, ctx.Err()
		}
	}

	// Find user by email. Returns apperror.NotFound if no match.
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		// Don't reveal whether the email exists -- use generic message.
		var appErr *apperror.AppError
		if isNotFound(err, &appErr) {
			s.recordLoginFailure(ctx, email)
			return "", nil, apperror.NewUnauthorized("invalid email or password")
		}
		return "", nil, apperror.NewInternal(fmt.Errorf("finding user: %w", err))
	}

	// Block disabled accounts from logging in.
	if user.IsDisabled {
		return "", nil, apperror.NewForbidden("your account has been disabled")
	}

	// Verify the password against the stored argon2id hash.
	if !verifyPassword(input.Password, user.PasswordHash) {
		s.recordLoginFailure(ctx, email)
		return "", nil, apperror.NewUnauthorized("invalid email or password")
	}

	// Successful login — clear any failure counter.
	s.clearLoginFailures(ctx, email)

	// Create a new session in Redis with client metadata.
	token, err := s.createSession(ctx, user, input.IP, input.UserAgent)
	if err != nil {
		return "", nil, apperror.NewInternal(fmt.Errorf("creating session: %w", err))
	}

	// Update the user's last login timestamp (fire-and-forget, non-critical).
	if err := s.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		slog.Warn("failed to update last login",
			slog.String("user_id", user.ID),
			slog.Any("error", err),
		)
	}

	slog.Info("user logged in",
		slog.String("user_id", user.ID),
		slog.String("email", user.Email),
	)

	return token, user, nil
}

// loginFailureKey returns the Redis key for tracking login failures for an email.
// Uses SHA-256 hash to avoid storing plaintext email addresses in Redis.
func loginFailureKey(email string) string {
	h := sha256.Sum256([]byte(email))
	return loginFailureKeyPrefix + hex.EncodeToString(h[:])
}

// loginThrottleDelay returns the progressive delay to apply before processing
// a login attempt for the given email. Returns 0 if the failure count is below
// the threshold. After loginThrottleMax failures, delay doubles each attempt:
// 2s, 4s, 8s, 16s, ... capped at loginThrottleMaxDelay (5 min).
// Fails open on Redis errors (logs warning, returns 0).
func (s *authService) loginThrottleDelay(ctx context.Context, email string) time.Duration {
	if s.redis == nil {
		return 0
	}
	count, err := s.redis.Get(ctx, loginFailureKey(email)).Int()
	if err != nil {
		// Key doesn't exist or Redis error — no delay.
		return 0
	}
	if count < loginThrottleMax {
		return 0
	}
	// Progressive backoff: 2^(count - threshold) * 2 seconds.
	exponent := count - loginThrottleMax
	delay := 2 * time.Second
	for i := 0; i < exponent; i++ {
		delay *= 2
		if delay >= loginThrottleMaxDelay {
			return loginThrottleMaxDelay
		}
	}
	return delay
}

// recordLoginFailure increments the failed attempt counter for an email.
// The counter auto-expires after the throttle window.
func (s *authService) recordLoginFailure(ctx context.Context, email string) {
	if s.redis == nil {
		return
	}
	key := loginFailureKey(email)
	pipe := s.redis.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, loginThrottleWindow)
	if _, err := pipe.Exec(ctx); err != nil {
		slog.Warn("failed to record login failure",
			slog.String("key", key),
			slog.Any("error", err),
		)
	}
}

// clearLoginFailures removes the failure counter on successful login.
func (s *authService) clearLoginFailures(ctx context.Context, email string) {
	if s.redis == nil {
		return
	}
	if err := s.redis.Del(ctx, loginFailureKey(email)).Err(); err != nil {
		slog.Warn("failed to clear login failures",
			slog.Any("error", err),
		)
	}
}

// ValidateSession looks up a session token in Redis and returns the session
// data if it exists and hasn't expired. Periodically revalidates the session
// against the database to detect user deletions, disablements, or privilege
// changes (e.g., admin flag revoked, database wiped while Redis persists).
func (s *authService) ValidateSession(ctx context.Context, token string) (*Session, error) {
	key := sessionKeyPrefix + token

	data, err := s.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, apperror.NewUnauthorized("session expired or invalid")
	}
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("reading session from Redis: %w", err))
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("unmarshaling session: %w", err))
	}

	// Periodically revalidate against the database. Zero-value LastValidated
	// (from sessions created before this field existed) triggers immediately.
	if time.Since(session.LastValidated) > sessionRevalidateInterval {
		if err := s.revalidateSession(ctx, key, token, &session); err != nil {
			return nil, err
		}
	}

	return &session, nil
}

// revalidateSession checks the database to ensure the session's user still
// exists, is not disabled, and has up-to-date privileges. On failure it
// destroys the session. On success it updates the session in Redis with fresh
// data, preserving the original TTL.
func (s *authService) revalidateSession(ctx context.Context, key, token string, session *Session) error {
	user, err := s.repo.FindByID(ctx, session.UserID)
	if err != nil {
		// Distinguish "user not found" from transient DB errors.
		var appErr *apperror.AppError
		if errorAs(err, &appErr) && appErr.Code == 404 {
			// User was deleted -- destroy the stale session.
			_ = s.DestroySession(ctx, token)
			return apperror.NewUnauthorized("session expired or invalid")
		}
		// Transient DB error -- log and allow the request through with stale
		// data. Revalidation will be retried on the next request since
		// LastValidated was not updated.
		slog.Warn("session revalidation failed, allowing stale session",
			slog.String("user_id", session.UserID),
			slog.Any("error", err),
		)
		return nil
	}

	// User exists but has been disabled.
	if user.IsDisabled {
		_ = s.DestroySession(ctx, token)
		return apperror.NewUnauthorized("session expired or invalid")
	}

	// Sync session fields with current DB state.
	session.IsAdmin = user.IsAdmin
	session.Email = user.Email
	session.Name = user.DisplayName
	session.LastValidated = time.Now().UTC()

	// Write the updated session back to Redis, preserving the original TTL
	// so revalidation does not extend the session lifetime.
	remainingTTL, err := s.redis.TTL(ctx, key).Result()
	if err != nil || remainingTTL <= 0 {
		// Key expired or error -- session will naturally expire.
		return nil
	}

	data, err := json.Marshal(session)
	if err != nil {
		slog.Warn("failed to marshal updated session",
			slog.String("user_id", session.UserID),
			slog.Any("error", err),
		)
		return nil
	}

	if err := s.redis.Set(ctx, key, data, remainingTTL).Err(); err != nil {
		slog.Warn("failed to write revalidated session to Redis",
			slog.String("user_id", session.UserID),
			slog.Any("error", err),
		)
	}

	return nil
}

// DestroySession removes a session from Redis, effectively logging the user out.
// Also removes the token from the user's session tracking set.
func (s *authService) DestroySession(ctx context.Context, token string) error {
	key := sessionKeyPrefix + token

	// Look up the session to get the user ID for set cleanup.
	data, err := s.redis.Get(ctx, key).Bytes()
	if err == nil {
		var session Session
		if jsonErr := json.Unmarshal(data, &session); jsonErr == nil {
			s.redis.SRem(ctx, userSessionsKeyPrefix+session.UserID, token)
		}
	}

	if err := s.redis.Del(ctx, key).Err(); err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting session from Redis: %w", err))
	}

	return nil
}

// createSession generates a random session token, stores the session data in
// Redis with the configured TTL, and returns the token. IP and userAgent are
// stored alongside the session for the admin active sessions view.
func (s *authService) createSession(ctx context.Context, user *User, ip, userAgent string) (string, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", fmt.Errorf("generating session token: %w", err)
	}

	now := time.Now().UTC()
	session := Session{
		UserID:        user.ID,
		Email:         user.Email,
		Name:          user.DisplayName,
		IsAdmin:       user.IsAdmin,
		IP:            ip,
		UserAgent:     userAgent,
		CreatedAt:     now,
		LastValidated: now,
	}

	data, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("marshaling session: %w", err)
	}

	key := sessionKeyPrefix + token
	if err := s.redis.Set(ctx, key, data, s.sessionTTL).Err(); err != nil {
		return "", fmt.Errorf("storing session in Redis: %w", err)
	}

	// Track this session token in the user's session set so we can invalidate
	// all sessions on password reset. The set has the same TTL as the session.
	userSetKey := userSessionsKeyPrefix + user.ID
	s.redis.SAdd(ctx, userSetKey, token)
	s.redis.Expire(ctx, userSetKey, s.sessionTTL)

	return token, nil
}

// --- Admin Session Management ---

// ListAllSessions scans Redis for all active sessions and returns them with
// metadata. Used by the admin security dashboard. Suitable for self-hosted
// instances with modest user counts (typically < 1000 sessions).
func (s *authService) ListAllSessions(ctx context.Context) ([]SessionInfo, error) {
	if s.redis == nil {
		return nil, nil
	}

	var sessions []SessionInfo
	var cursor uint64
	for {
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, sessionKeyPrefix+"*", 100).Result()
		if err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scanning sessions: %w", err))
		}

		for _, key := range keys {
			token := strings.TrimPrefix(key, sessionKeyPrefix)

			data, err := s.redis.Get(ctx, key).Bytes()
			if err != nil {
				continue // Session expired between scan and get.
			}

			var session Session
			if err := json.Unmarshal(data, &session); err != nil {
				continue
			}

			ttl, _ := s.redis.TTL(ctx, key).Result()

			hint := token
			if len(hint) > 8 {
				hint = hint[:8]
			}

			sessions = append(sessions, SessionInfo{
				Token:     token,
				TokenHash: hashToken(token),
				TokenHint: hint,
				UserID:    session.UserID,
				Email:     session.Email,
				Name:      session.Name,
				IsAdmin:   session.IsAdmin,
				IP:        session.IP,
				UserAgent: session.UserAgent,
				CreatedAt: session.CreatedAt,
				TTL:       ttl,
			})
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return sessions, nil
}

// DestroyAllUserSessions removes all active sessions for a user from Redis.
// Returns the number of sessions destroyed. Used by admin force-logout.
func (s *authService) DestroyAllUserSessions(ctx context.Context, userID string) (int, error) {
	if s.redis == nil {
		return 0, nil
	}

	userSetKey := userSessionsKeyPrefix + userID
	tokens, err := s.redis.SMembers(ctx, userSetKey).Result()
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("listing user sessions: %w", err))
	}

	for _, token := range tokens {
		s.redis.Del(ctx, sessionKeyPrefix+token)
	}
	s.redis.Del(ctx, userSetKey)

	return len(tokens), nil
}

// --- Password Reset ---

// InitiatePasswordReset generates a reset token, stores its hash in the DB,
// and sends a reset link via email. Always returns nil to avoid leaking whether
// the email exists (timing-safe: we always do the same work).
func (s *authService) InitiatePasswordReset(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))

	// Per-email rate limit: max 3 reset requests per 15 minutes.
	// Silently succeed when rate-limited to avoid leaking email existence.
	if s.redis != nil {
		rateLimitKey := "reset_rate:" + email
		count, _ := s.redis.Incr(ctx, rateLimitKey).Result()
		if count == 1 {
			s.redis.Expire(ctx, rateLimitKey, 15*time.Minute)
		}
		if count > 3 {
			slog.Debug("password reset rate-limited", slog.String("email", email))
			return nil
		}
	}

	// Always generate a token regardless of whether the email exists.
	// This prevents timing side-channel attacks that could reveal email existence
	// by comparing response times (token generation + email send vs early return).
	tokenBytes := make([]byte, resetTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return apperror.NewInternal(fmt.Errorf("generating reset token: %w", err))
	}
	plainToken := hex.EncodeToString(tokenBytes)
	tokenHash := hashToken(plainToken)
	expiresAt := time.Now().UTC().Add(resetTokenExpiry)

	// Look up user. If not found, log and return nil (don't reveal existence).
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		slog.Debug("password reset requested for unknown email", slog.String("email", email))
		return nil
	}

	// Store hashed token in DB.
	if err := s.repo.CreateResetToken(ctx, user.ID, user.Email, tokenHash, expiresAt); err != nil {
		return apperror.NewInternal(fmt.Errorf("storing reset token: %w", err))
	}

	// Send the email with the plaintext token in the link.
	if s.mail != nil && s.mail.IsConfigured(ctx) {
		link := fmt.Sprintf("%s/reset-password?token=%s", s.baseURL, plainToken)
		body := fmt.Sprintf(
			"A password reset was requested for your Chronicle account.\n\n"+
				"Click the link below to set a new password:\n%s\n\n"+
				"This link expires in 1 hour. If you did not request this, you can safely ignore this email.",
			link,
		)
		if err := s.mail.SendMail(ctx, []string{user.Email}, "Password Reset — Chronicle", body); err != nil {
			slog.Warn("failed to send password reset email",
				slog.String("email", user.Email),
				slog.Any("error", err),
			)
		}
	} else {
		slog.Warn("SMTP not configured; password reset email not sent — user will not receive the reset link",
			slog.String("email", user.Email),
		)
	}

	slog.Info("password reset initiated",
		slog.String("user_id", user.ID),
		slog.String("email", user.Email),
	)

	return nil
}

// ValidateResetToken checks that a reset token is valid, unused, and unexpired.
// Returns the associated email address on success.
func (s *authService) ValidateResetToken(ctx context.Context, token string) (string, error) {
	tokenHash := hashToken(token)

	_, email, expiresAt, usedAt, err := s.repo.FindResetToken(ctx, tokenHash)
	if err != nil {
		return "", apperror.NewBadRequest("invalid or expired reset link")
	}
	if usedAt != nil {
		return "", apperror.NewBadRequest("this reset link has already been used")
	}
	if time.Now().UTC().After(expiresAt) {
		return "", apperror.NewBadRequest("this reset link has expired")
	}

	return email, nil
}

// ResetPassword validates the token, hashes the new password, updates the
// user's password, and marks the token as used.
func (s *authService) ResetPassword(ctx context.Context, token, newPassword string) error {
	tokenHash := hashToken(token)

	userID, _, expiresAt, usedAt, err := s.repo.FindResetToken(ctx, tokenHash)
	if err != nil {
		return apperror.NewBadRequest("invalid or expired reset link")
	}
	if usedAt != nil {
		return apperror.NewBadRequest("this reset link has already been used")
	}
	if time.Now().UTC().After(expiresAt) {
		return apperror.NewBadRequest("this reset link has expired")
	}

	// Hash the new password.
	hash, err := hashPassword(newPassword)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("hashing new password: %w", err))
	}

	// Update password in DB.
	if err := s.repo.UpdatePassword(ctx, userID, hash); err != nil {
		return apperror.NewInternal(fmt.Errorf("updating password: %w", err))
	}

	// Mark token as used so it can't be reused.
	if err := s.repo.MarkResetTokenUsed(ctx, tokenHash); err != nil {
		slog.Warn("failed to mark reset token as used", slog.Any("error", err))
	}

	// Invalidate all existing sessions for this user. If an attacker stole a
	// session, the legitimate user resetting their password revokes the attacker's access.
	s.destroyUserSessions(ctx, userID)

	slog.Info("password reset completed", slog.String("user_id", userID))
	return nil
}

// hashToken returns the hex-encoded SHA-256 hash of a plaintext token.
// We store the hash in the DB so a DB leak doesn't expose valid tokens.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// destroyUserSessions removes all active sessions for a user from Redis.
// Called on password reset to invalidate any compromised sessions.
func (s *authService) destroyUserSessions(ctx context.Context, userID string) {
	if s.redis == nil {
		return
	}

	userSetKey := userSessionsKeyPrefix + userID
	tokens, err := s.redis.SMembers(ctx, userSetKey).Result()
	if err != nil {
		slog.Warn("failed to list user sessions for invalidation",
			slog.String("user_id", userID),
			slog.Any("error", err),
		)
		return
	}

	for _, token := range tokens {
		s.redis.Del(ctx, sessionKeyPrefix+token)
	}
	s.redis.Del(ctx, userSetKey)

	if len(tokens) > 0 {
		slog.Info("invalidated user sessions on password reset",
			slog.String("user_id", userID),
			slog.Int("session_count", len(tokens)),
		)
	}
}

// --- User Profile ---

// GetUser retrieves a user by ID. Used by the account settings page.
func (s *authService) GetUser(ctx context.Context, userID string) (*User, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateTimezone sets the user's IANA timezone. Validates the timezone string
// against Go's timezone database before persisting.
func (s *authService) UpdateTimezone(ctx context.Context, userID, timezone string) error {
	if timezone != "" {
		if _, err := time.LoadLocation(timezone); err != nil {
			return apperror.NewBadRequest("invalid timezone: " + timezone)
		}
	}
	return s.repo.UpdateTimezone(ctx, userID, timezone)
}

// UpdateDisplayName sets the user's display name with validation.
func (s *authService) UpdateDisplayName(ctx context.Context, userID, displayName string) error {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return apperror.NewBadRequest("display name is required")
	}
	if len(displayName) < 2 {
		return apperror.NewBadRequest("display name must be at least 2 characters")
	}
	if len(displayName) > 100 {
		return apperror.NewBadRequest("display name must be at most 100 characters")
	}
	return s.repo.UpdateDisplayName(ctx, userID, displayName)
}

// UpdateAvatarPath sets or clears the user's avatar image path.
func (s *authService) UpdateAvatarPath(ctx context.Context, userID string, avatarPath *string) error {
	return s.repo.UpdateAvatarPath(ctx, userID, avatarPath)
}

// ChangePassword verifies the current password and sets a new one.
func (s *authService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify current password.
	if !verifyPassword(currentPassword, user.PasswordHash) {
		return apperror.NewBadRequest("current password is incorrect")
	}

	// Validate new password.
	if len(newPassword) < 8 {
		return apperror.NewBadRequest("new password must be at least 8 characters")
	}
	if len(newPassword) > 128 {
		return apperror.NewBadRequest("new password must be at most 128 characters")
	}

	// Hash and store.
	hash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing new password: %w", err)
	}
	if err := s.repo.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}

	// Invalidate all existing sessions so any compromised session is terminated.
	// The caller is responsible for creating a fresh session for the current user.
	s.destroyUserSessions(ctx, userID)
	return nil
}

// --- Email Change ---

// emailVerifyTokenBytes is the number of random bytes in an email verification token.
const emailVerifyTokenBytes = 32

// emailVerifyExpiry is how long an email verification link stays valid.
const emailVerifyExpiry = 24 * time.Hour

// RequestEmailChange initiates an email change by verifying the user's password,
// checking uniqueness of the new email, generating a verification token, and
// sending a verification link to the NEW email address.
func (s *authService) RequestEmailChange(ctx context.Context, userID, newEmail, currentPassword string) error {
	newEmail = strings.ToLower(strings.TrimSpace(newEmail))
	if newEmail == "" {
		return apperror.NewBadRequest("email address is required")
	}

	// Verify current password first.
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !verifyPassword(currentPassword, user.PasswordHash) {
		return apperror.NewBadRequest("current password is incorrect")
	}

	// Don't allow changing to the same email.
	if newEmail == user.Email {
		return apperror.NewBadRequest("this is already your current email address")
	}

	// Check if the new email is already taken.
	exists, err := s.repo.EmailExists(ctx, newEmail)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("checking email: %w", err))
	}
	if exists {
		return apperror.NewConflict("an account with this email already exists")
	}

	// Generate verification token.
	tokenBytes := make([]byte, emailVerifyTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return apperror.NewInternal(fmt.Errorf("generating verification token: %w", err))
	}
	plainToken := hex.EncodeToString(tokenBytes)
	tokenHash := hashToken(plainToken)
	expiresAt := time.Now().UTC().Add(emailVerifyExpiry)

	// Store pending email and token hash.
	if err := s.repo.SetPendingEmail(ctx, userID, newEmail, tokenHash, expiresAt); err != nil {
		return apperror.NewInternal(fmt.Errorf("storing pending email: %w", err))
	}

	// Send verification email to the NEW address.
	if s.mail != nil && s.mail.IsConfigured(ctx) {
		link := fmt.Sprintf("%s/account/email/verify?token=%s", s.baseURL, plainToken)
		body := fmt.Sprintf(
			"An email change was requested for your Chronicle account.\n\n"+
				"Click the link below to confirm your new email address:\n%s\n\n"+
				"This link expires in 24 hours. If you did not request this change, you can safely ignore this email.",
			link,
		)
		if err := s.mail.SendMail(ctx, []string{newEmail}, "Verify Your New Email — Chronicle", body); err != nil {
			slog.Warn("failed to send email verification",
				slog.String("user_id", userID),
				slog.String("new_email", newEmail),
				slog.Any("error", err),
			)
			return apperror.NewInternal(fmt.Errorf("sending verification email: %w", err))
		}
	} else {
		slog.Warn("SMTP not configured; email verification link not sent",
			slog.String("user_id", userID),
			slog.String("new_email", newEmail),
		)
		return apperror.NewBadRequest("SMTP is not configured — cannot send verification email")
	}

	slog.Info("email change requested",
		slog.String("user_id", userID),
		slog.String("new_email", newEmail),
	)
	return nil
}

// ConfirmEmailChange validates the verification token and updates the user's email.
// Invalidates all sessions to force re-login with the new email.
func (s *authService) ConfirmEmailChange(ctx context.Context, token string) error {
	tokenHash := hashToken(token)

	userID, pendingEmail, expiresAt, err := s.repo.FindByEmailVerifyToken(ctx, tokenHash)
	if err != nil {
		return apperror.NewBadRequest("invalid or expired verification link")
	}
	if time.Now().UTC().After(expiresAt) {
		return apperror.NewBadRequest("this verification link has expired")
	}

	// Final uniqueness check (another user could have taken the email in the meantime).
	exists, err := s.repo.EmailExists(ctx, pendingEmail)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("checking email: %w", err))
	}
	if exists {
		return apperror.NewConflict("this email address is now taken by another account")
	}

	// Update the email and clear pending fields.
	if err := s.repo.ConfirmEmailChange(ctx, userID, pendingEmail); err != nil {
		return apperror.NewInternal(fmt.Errorf("confirming email change: %w", err))
	}

	// Invalidate all sessions — user must re-login with the new email.
	s.destroyUserSessions(ctx, userID)

	slog.Info("email change confirmed",
		slog.String("user_id", userID),
		slog.String("new_email", pendingEmail),
	)
	return nil
}

// DestroySessionByHash finds a session by the SHA-256 hash of its token
// and destroys it. Prevents exposing raw session tokens in admin UI URLs.
func (s *authService) DestroySessionByHash(ctx context.Context, tokenHash string) error {
	if s.redis == nil {
		return apperror.NewInternal(fmt.Errorf("redis not available"))
	}

	// Scan all sessions to find the one matching the hash.
	var cursor uint64
	for {
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, sessionKeyPrefix+"*", 100).Result()
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("scanning sessions: %w", err))
		}

		for _, key := range keys {
			token := strings.TrimPrefix(key, sessionKeyPrefix)
			if hashToken(token) == tokenHash {
				return s.DestroySession(ctx, token)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return apperror.NewNotFound("session not found")
}

// --- Password Hashing (argon2id) ---

// hashPassword creates an argon2id hash of the given password. The output
// format is: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
// This format is compatible with most argon2 libraries and allows self-
// contained verification without separate salt storage.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// Encode to the standard PHC string format.
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64Salt, b64Hash)

	return encoded, nil
}

// verifyPassword checks a plaintext password against an argon2id hash string.
// Returns true if the password matches.
func verifyPassword(password, encodedHash string) bool {
	// Parse the encoded hash to extract parameters, salt, and hash.
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	// Compute the hash of the provided password with the same parameters.
	computedHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expectedHash)))

	// Constant-time comparison to prevent timing attacks.
	return subtle.ConstantTimeCompare(expectedHash, computedHash) == 1
}

// --- Helpers ---

// generateUUID creates a new v4 UUID string using crypto/rand.
// Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
func generateUUID() string {
	uuid := make([]byte, 16)
	_, _ = rand.Read(uuid)

	// Set version (4) and variant (RFC 4122) bits.
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant RFC 4122

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// generateSessionToken creates a cryptographically random hex-encoded token.
func generateSessionToken() (string, error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// isNotFound checks if an error is an apperror.NotFound type.
func isNotFound(err error, target **apperror.AppError) bool {
	if err == nil {
		return false
	}
	var appErr *apperror.AppError
	if ok := errorAs(err, &appErr); ok && appErr.Code == 404 {
		*target = appErr
		return true
	}
	return false
}

// errorAs is a thin wrapper around type assertion for AppError.
func errorAs(err error, target **apperror.AppError) bool {
	ae, ok := err.(*apperror.AppError)
	if ok {
		*target = ae
	}
	return ok
}

// --- Re-Authentication for Sensitive Operations ---

// reauthKeyPrefix is the Redis key prefix for re-authentication confirmations.
// Keys are short-lived (5 minutes) and set after the admin re-enters their password.
const reauthKeyPrefix = "reauth_confirmed:"

// reauthWindow is how long a re-authentication confirmation remains valid.
// After this window, the admin must re-enter their password for the next
// sensitive operation.
const reauthWindow = 5 * time.Minute

// ConfirmReauth validates the admin's password and sets a short-lived Redis key
// that allows sensitive operations for the reauthWindow duration. Returns an
// error if the password is incorrect.
func (s *authService) ConfirmReauth(ctx context.Context, userID, password string) error {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("finding user for reauth: %w", err))
	}

	if !verifyPassword(password, user.PasswordHash) {
		return apperror.NewUnauthorized("incorrect password")
	}

	// Set the reauth confirmation key in Redis with short TTL.
	key := reauthKeyPrefix + userID
	if err := s.redis.Set(ctx, key, "1", reauthWindow).Err(); err != nil {
		return apperror.NewInternal(fmt.Errorf("storing reauth confirmation: %w", err))
	}

	return nil
}

// IsReauthValid checks whether the user has recently confirmed their password
// for sensitive operations. Returns true if a valid reauth confirmation exists
// in Redis within the reauthWindow.
func (s *authService) IsReauthValid(ctx context.Context, userID string) (bool, error) {
	key := reauthKeyPrefix + userID
	exists, err := s.redis.Exists(ctx, key).Result()
	if err != nil {
		return false, apperror.NewInternal(fmt.Errorf("checking reauth status: %w", err))
	}
	return exists > 0, nil
}
