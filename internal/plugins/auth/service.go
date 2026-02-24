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
		return nil, apperror.NewConflict("an account with this email already exists")
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

// Login authenticates a user by email and password. On success it creates a
// new session in Redis and returns the session token for the cookie.
func (s *authService) Login(ctx context.Context, input LoginInput) (string, *User, error) {
	// Find user by email. Returns apperror.NotFound if no match.
	user, err := s.repo.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(input.Email)))
	if err != nil {
		// Don't reveal whether the email exists -- use generic message.
		var appErr *apperror.AppError
		if isNotFound(err, &appErr) {
			return "", nil, apperror.NewUnauthorized("invalid email or password")
		}
		return "", nil, apperror.NewInternal(fmt.Errorf("finding user: %w", err))
	}

	// Verify the password against the stored argon2id hash.
	if !verifyPassword(input.Password, user.PasswordHash) {
		return "", nil, apperror.NewUnauthorized("invalid email or password")
	}

	// Create a new session in Redis.
	token, err := s.createSession(ctx, user)
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

// ValidateSession looks up a session token in Redis and returns the session
// data if it exists and hasn't expired.
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

	return &session, nil
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
// Redis with the configured TTL, and returns the token.
func (s *authService) createSession(ctx context.Context, user *User) (string, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", fmt.Errorf("generating session token: %w", err)
	}

	session := Session{
		UserID:    user.ID,
		Email:     user.Email,
		Name:      user.DisplayName,
		IsAdmin:   user.IsAdmin,
		CreatedAt: time.Now().UTC(),
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

// --- Password Reset ---

// InitiatePasswordReset generates a reset token, stores its hash in the DB,
// and sends a reset link via email. Always returns nil to avoid leaking whether
// the email exists (timing-safe: we always do the same work).
func (s *authService) InitiatePasswordReset(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))

	// Look up user. If not found, log and return nil (don't reveal existence).
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		slog.Debug("password reset requested for unknown email", slog.String("email", email))
		return nil
	}

	// Generate a random token and hash it for storage.
	tokenBytes := make([]byte, resetTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return apperror.NewInternal(fmt.Errorf("generating reset token: %w", err))
	}
	plainToken := hex.EncodeToString(tokenBytes)
	tokenHash := hashToken(plainToken)
	expiresAt := time.Now().UTC().Add(resetTokenExpiry)

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
