package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

// mockUserRepo implements UserRepository for testing.
type mockUserRepo struct {
	createFn             func(ctx context.Context, user *User) error
	findByIDFn           func(ctx context.Context, id string) (*User, error)
	findByEmailFn        func(ctx context.Context, email string) (*User, error)
	emailExistsFn        func(ctx context.Context, email string) (bool, error)
	updateLastLoginFn    func(ctx context.Context, id string) error
	updatePasswordFn     func(ctx context.Context, userID, passwordHash string) error
	createResetTokenFn   func(ctx context.Context, userID, email, tokenHash string, expiresAt time.Time) error
	findResetTokenFn     func(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error)
	markResetTokenUsedFn func(ctx context.Context, tokenHash string) error
	listUsersFn          func(ctx context.Context, offset, limit int) ([]User, int, error)
	updateIsAdminFn      func(ctx context.Context, id string, isAdmin bool) error
	countUsersFn         func(ctx context.Context) (int, error)
	countAdminsFn        func(ctx context.Context) (int, error)
	updateIsDisabledFn   func(ctx context.Context, id string, isDisabled bool) error
}

func (m *mockUserRepo) Create(ctx context.Context, user *User) error {
	if m.createFn != nil {
		return m.createFn(ctx, user)
	}
	return nil
}

func (m *mockUserRepo) FindByID(ctx context.Context, id string) (*User, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("user not found")
}

func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*User, error) {
	if m.findByEmailFn != nil {
		return m.findByEmailFn(ctx, email)
	}
	return nil, apperror.NewNotFound("user not found")
}

func (m *mockUserRepo) EmailExists(ctx context.Context, email string) (bool, error) {
	if m.emailExistsFn != nil {
		return m.emailExistsFn(ctx, email)
	}
	return false, nil
}

func (m *mockUserRepo) UpdateLastLogin(ctx context.Context, id string) error {
	if m.updateLastLoginFn != nil {
		return m.updateLastLoginFn(ctx, id)
	}
	return nil
}

func (m *mockUserRepo) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	if m.updatePasswordFn != nil {
		return m.updatePasswordFn(ctx, userID, passwordHash)
	}
	return nil
}

func (m *mockUserRepo) CreateResetToken(ctx context.Context, userID, email, tokenHash string, expiresAt time.Time) error {
	if m.createResetTokenFn != nil {
		return m.createResetTokenFn(ctx, userID, email, tokenHash, expiresAt)
	}
	return nil
}

func (m *mockUserRepo) FindResetToken(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
	if m.findResetTokenFn != nil {
		return m.findResetTokenFn(ctx, tokenHash)
	}
	return "", "", time.Time{}, nil, apperror.NewNotFound("token not found")
}

func (m *mockUserRepo) MarkResetTokenUsed(ctx context.Context, tokenHash string) error {
	if m.markResetTokenUsedFn != nil {
		return m.markResetTokenUsedFn(ctx, tokenHash)
	}
	return nil
}

func (m *mockUserRepo) ListUsers(ctx context.Context, offset, limit int) ([]User, int, error) {
	if m.listUsersFn != nil {
		return m.listUsersFn(ctx, offset, limit)
	}
	return nil, 0, nil
}

func (m *mockUserRepo) UpdateIsAdmin(ctx context.Context, id string, isAdmin bool) error {
	if m.updateIsAdminFn != nil {
		return m.updateIsAdminFn(ctx, id, isAdmin)
	}
	return nil
}

func (m *mockUserRepo) CountUsers(ctx context.Context) (int, error) {
	if m.countUsersFn != nil {
		return m.countUsersFn(ctx)
	}
	return 0, nil
}

func (m *mockUserRepo) CountAdmins(ctx context.Context) (int, error) {
	if m.countAdminsFn != nil {
		return m.countAdminsFn(ctx)
	}
	return 0, nil
}

func (m *mockUserRepo) UpdateIsDisabled(ctx context.Context, id string, isDisabled bool) error {
	if m.updateIsDisabledFn != nil {
		return m.updateIsDisabledFn(ctx, id, isDisabled)
	}
	return nil
}

func (m *mockUserRepo) UpdateTimezone(ctx context.Context, userID, timezone string) error {
	return nil
}

// --- Mock Mail Sender ---

// mockMailSender implements MailSender for testing.
type mockMailSender struct {
	sendMailFn     func(ctx context.Context, to []string, subject, body string) error
	isConfiguredFn func(ctx context.Context) bool
	// Capture fields for assertions.
	lastTo      []string
	lastSubject string
	lastBody    string
	sendCount   int
}

func (m *mockMailSender) SendMail(ctx context.Context, to []string, subject, body string) error {
	m.lastTo = to
	m.lastSubject = subject
	m.lastBody = body
	m.sendCount++
	if m.sendMailFn != nil {
		return m.sendMailFn(ctx, to, subject, body)
	}
	return nil
}

func (m *mockMailSender) IsConfigured(ctx context.Context) bool {
	if m.isConfiguredFn != nil {
		return m.isConfiguredFn(ctx)
	}
	return true
}

// --- Test Helpers ---

// newTestAuthService creates an authService with a mock repo and no Redis
// (password reset tests don't need Redis; register/login Redis paths are
// tested separately in integration tests).
func newTestAuthService(repo *mockUserRepo) *authService {
	return &authService{
		repo:       repo,
		sessionTTL: 24 * time.Hour,
	}
}

// assertAppError checks that err is an *apperror.AppError with the expected code.
func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %d, got nil", expectedCode)
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperror.AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// --- Register Tests ---

func TestRegister_Success(t *testing.T) {
	repo := &mockUserRepo{
		emailExistsFn: func(ctx context.Context, email string) (bool, error) {
			return false, nil
		},
		countUsersFn: func(ctx context.Context) (int, error) {
			return 1, nil // Not the first user.
		},
		createFn: func(ctx context.Context, user *User) error {
			if user.Email != "alice@example.com" {
				t.Errorf("expected email alice@example.com, got %s", user.Email)
			}
			if user.DisplayName != "Alice" {
				t.Errorf("expected display name Alice, got %s", user.DisplayName)
			}
			if user.IsAdmin {
				t.Error("expected non-admin user")
			}
			if user.PasswordHash == "" {
				t.Error("expected password hash to be set")
			}
			return nil
		},
	}

	svc := newTestAuthService(repo)
	user, err := svc.Register(context.Background(), RegisterInput{
		Email:       "Alice@Example.com",
		DisplayName: "Alice",
		Password:    "secure-password-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID == "" {
		t.Error("expected user ID to be generated")
	}
}

func TestRegister_FirstUserBecomesAdmin(t *testing.T) {
	repo := &mockUserRepo{
		emailExistsFn: func(ctx context.Context, email string) (bool, error) {
			return false, nil
		},
		countUsersFn: func(ctx context.Context) (int, error) {
			return 0, nil // First user.
		},
	}

	svc := newTestAuthService(repo)
	user, err := svc.Register(context.Background(), RegisterInput{
		Email:       "admin@example.com",
		DisplayName: "Admin",
		Password:    "secure-password-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !user.IsAdmin {
		t.Error("expected first user to be admin")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	repo := &mockUserRepo{
		emailExistsFn: func(ctx context.Context, email string) (bool, error) {
			return true, nil
		},
	}

	svc := newTestAuthService(repo)
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:       "taken@example.com",
		DisplayName: "Test",
		Password:    "secure-password-123",
	})
	assertAppError(t, err, 409)
}

func TestRegister_EmailCheckError(t *testing.T) {
	repo := &mockUserRepo{
		emailExistsFn: func(ctx context.Context, email string) (bool, error) {
			return false, errors.New("db connection lost")
		},
	}

	svc := newTestAuthService(repo)
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:       "test@example.com",
		DisplayName: "Test",
		Password:    "secure-password-123",
	})
	assertAppError(t, err, 500)
}

func TestRegister_CreateError(t *testing.T) {
	repo := &mockUserRepo{
		emailExistsFn: func(ctx context.Context, email string) (bool, error) {
			return false, nil
		},
		countUsersFn: func(ctx context.Context) (int, error) {
			return 1, nil
		},
		createFn: func(ctx context.Context, user *User) error {
			return errors.New("db write error")
		},
	}

	svc := newTestAuthService(repo)
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:       "test@example.com",
		DisplayName: "Test",
		Password:    "secure-password-123",
	})
	assertAppError(t, err, 500)
}

func TestRegister_EmailNormalization(t *testing.T) {
	var capturedEmail string
	repo := &mockUserRepo{
		emailExistsFn: func(ctx context.Context, email string) (bool, error) {
			return false, nil
		},
		countUsersFn: func(ctx context.Context) (int, error) {
			return 1, nil
		},
		createFn: func(ctx context.Context, user *User) error {
			capturedEmail = user.Email
			return nil
		},
	}

	svc := newTestAuthService(repo)
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:       "  Alice@EXAMPLE.com  ",
		DisplayName: "Alice",
		Password:    "secure-password-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedEmail != "alice@example.com" {
		t.Errorf("expected normalized email alice@example.com, got %s", capturedEmail)
	}
}

// --- Password Hashing Tests ---

func TestHashAndVerifyPassword(t *testing.T) {
	password := "my-secret-password-123"

	hash, err := hashPassword(password)
	if err != nil {
		t.Fatalf("hashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	// Correct password should verify.
	if !verifyPassword(password, hash) {
		t.Error("expected correct password to verify")
	}

	// Wrong password should not verify.
	if verifyPassword("wrong-password", hash) {
		t.Error("expected wrong password to fail verification")
	}
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"empty string", ""},
		{"random text", "not-a-hash"},
		{"too few parts", "$argon2id$v=19$m=65536"},
		{"corrupted salt", "$argon2id$v=19$m=65536,t=3,p=4$!!!invalid$aGFzaA"},
		{"corrupted hash", "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$!!!invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if verifyPassword("password", tt.hash) {
				t.Error("expected invalid hash to fail verification")
			}
		})
	}
}

func TestHashPassword_UniqueSalts(t *testing.T) {
	hash1, err := hashPassword("same-password")
	if err != nil {
		t.Fatalf("hashPassword failed: %v", err)
	}
	hash2, err := hashPassword("same-password")
	if err != nil {
		t.Fatalf("hashPassword failed: %v", err)
	}
	if hash1 == hash2 {
		t.Error("expected different salts to produce different hashes")
	}
}

// --- Password Reset Tests ---

func TestInitiatePasswordReset_Success(t *testing.T) {
	var capturedTokenHash string
	mail := &mockMailSender{}
	repo := &mockUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*User, error) {
			return &User{ID: "user-123", Email: "alice@example.com"}, nil
		},
		createResetTokenFn: func(ctx context.Context, userID, email, tokenHash string, expiresAt time.Time) error {
			if userID != "user-123" {
				t.Errorf("expected user-123, got %s", userID)
			}
			if email != "alice@example.com" {
				t.Errorf("expected alice@example.com, got %s", email)
			}
			if tokenHash == "" {
				t.Error("expected non-empty token hash")
			}
			capturedTokenHash = tokenHash
			// Verify expiry is roughly 1 hour from now.
			untilExpiry := time.Until(expiresAt)
			if untilExpiry < 55*time.Minute || untilExpiry > 65*time.Minute {
				t.Errorf("expected expiry ~1 hour, got %v", untilExpiry)
			}
			return nil
		},
	}

	svc := newTestAuthService(repo)
	svc.mail = mail
	svc.baseURL = "https://chronicle.example.com"

	err := svc.InitiatePasswordReset(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify email was sent.
	if mail.sendCount != 1 {
		t.Errorf("expected 1 email sent, got %d", mail.sendCount)
	}
	if len(mail.lastTo) != 1 || mail.lastTo[0] != "alice@example.com" {
		t.Errorf("expected email to alice@example.com, got %v", mail.lastTo)
	}
	if mail.lastSubject == "" {
		t.Error("expected non-empty subject")
	}

	// Token hash in DB should not be empty.
	if capturedTokenHash == "" {
		t.Error("expected token hash to be stored")
	}
}

func TestInitiatePasswordReset_UnknownEmail(t *testing.T) {
	repo := &mockUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*User, error) {
			return nil, apperror.NewNotFound("user not found")
		},
	}
	mail := &mockMailSender{}

	svc := newTestAuthService(repo)
	svc.mail = mail

	// Should return nil (no error) to prevent email enumeration.
	err := svc.InitiatePasswordReset(context.Background(), "unknown@example.com")
	if err != nil {
		t.Fatalf("expected nil error for unknown email, got: %v", err)
	}

	// No email should have been sent.
	if mail.sendCount != 0 {
		t.Errorf("expected no emails sent for unknown user, got %d", mail.sendCount)
	}
}

func TestInitiatePasswordReset_TokenStorageError(t *testing.T) {
	repo := &mockUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*User, error) {
			return &User{ID: "user-123", Email: "alice@example.com"}, nil
		},
		createResetTokenFn: func(ctx context.Context, userID, email, tokenHash string, expiresAt time.Time) error {
			return errors.New("db error")
		},
	}

	svc := newTestAuthService(repo)
	err := svc.InitiatePasswordReset(context.Background(), "alice@example.com")
	assertAppError(t, err, 500)
}

func TestInitiatePasswordReset_NoMailSender(t *testing.T) {
	var tokenStored bool
	repo := &mockUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*User, error) {
			return &User{ID: "user-123", Email: "alice@example.com"}, nil
		},
		createResetTokenFn: func(ctx context.Context, userID, email, tokenHash string, expiresAt time.Time) error {
			tokenStored = true
			return nil
		},
	}

	// No mail sender configured -- should still succeed (token stored, no email).
	svc := newTestAuthService(repo)
	err := svc.InitiatePasswordReset(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tokenStored {
		t.Error("expected token to be stored even without mail sender")
	}
}

func TestInitiatePasswordReset_EmailNormalization(t *testing.T) {
	var capturedEmail string
	repo := &mockUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*User, error) {
			capturedEmail = email
			return &User{ID: "user-123", Email: email}, nil
		},
		createResetTokenFn: func(ctx context.Context, userID, email, tokenHash string, expiresAt time.Time) error {
			return nil
		},
	}

	svc := newTestAuthService(repo)
	_ = svc.InitiatePasswordReset(context.Background(), "  ALICE@Example.COM  ")
	if capturedEmail != "alice@example.com" {
		t.Errorf("expected normalized email, got %s", capturedEmail)
	}
}

func TestValidateResetToken_Valid(t *testing.T) {
	plainToken := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	expectedHash := hashToken(plainToken)

	repo := &mockUserRepo{
		findResetTokenFn: func(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
			if tokenHash != expectedHash {
				t.Errorf("expected hash %s, got %s", expectedHash, tokenHash)
			}
			return "user-123", "alice@example.com", time.Now().Add(30 * time.Minute), nil, nil
		},
	}

	svc := newTestAuthService(repo)
	email, err := svc.ValidateResetToken(context.Background(), plainToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %s", email)
	}
}

func TestValidateResetToken_NotFound(t *testing.T) {
	repo := &mockUserRepo{
		findResetTokenFn: func(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
			return "", "", time.Time{}, nil, apperror.NewNotFound("not found")
		},
	}

	svc := newTestAuthService(repo)
	_, err := svc.ValidateResetToken(context.Background(), "invalid-token")
	assertAppError(t, err, 400)
}

func TestValidateResetToken_AlreadyUsed(t *testing.T) {
	usedAt := time.Now().Add(-10 * time.Minute)
	repo := &mockUserRepo{
		findResetTokenFn: func(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
			return "user-123", "alice@example.com", time.Now().Add(30 * time.Minute), &usedAt, nil
		},
	}

	svc := newTestAuthService(repo)
	_, err := svc.ValidateResetToken(context.Background(), "some-token")
	assertAppError(t, err, 400)
}

func TestValidateResetToken_Expired(t *testing.T) {
	repo := &mockUserRepo{
		findResetTokenFn: func(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
			return "user-123", "alice@example.com", time.Now().Add(-10 * time.Minute), nil, nil
		},
	}

	svc := newTestAuthService(repo)
	_, err := svc.ValidateResetToken(context.Background(), "expired-token")
	assertAppError(t, err, 400)
}

func TestResetPassword_Success(t *testing.T) {
	var updatedHash string
	var tokenMarkedUsed bool

	repo := &mockUserRepo{
		findResetTokenFn: func(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
			return "user-123", "alice@example.com", time.Now().Add(30 * time.Minute), nil, nil
		},
		updatePasswordFn: func(ctx context.Context, userID, passwordHash string) error {
			if userID != "user-123" {
				t.Errorf("expected user-123, got %s", userID)
			}
			updatedHash = passwordHash
			return nil
		},
		markResetTokenUsedFn: func(ctx context.Context, tokenHash string) error {
			tokenMarkedUsed = true
			return nil
		},
	}

	svc := newTestAuthService(repo)
	err := svc.ResetPassword(context.Background(), "valid-token", "new-secure-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Password hash should have been updated.
	if updatedHash == "" {
		t.Error("expected password hash to be updated")
	}
	// Verify the new hash works with the new password.
	if !verifyPassword("new-secure-password", updatedHash) {
		t.Error("expected new password to verify against updated hash")
	}
	// Token should have been marked as used.
	if !tokenMarkedUsed {
		t.Error("expected token to be marked as used")
	}
}

func TestResetPassword_InvalidToken(t *testing.T) {
	repo := &mockUserRepo{
		findResetTokenFn: func(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
			return "", "", time.Time{}, nil, apperror.NewNotFound("not found")
		},
	}

	svc := newTestAuthService(repo)
	err := svc.ResetPassword(context.Background(), "bad-token", "new-password")
	assertAppError(t, err, 400)
}

func TestResetPassword_UsedToken(t *testing.T) {
	usedAt := time.Now().Add(-5 * time.Minute)
	repo := &mockUserRepo{
		findResetTokenFn: func(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
			return "user-123", "alice@example.com", time.Now().Add(30 * time.Minute), &usedAt, nil
		},
	}

	svc := newTestAuthService(repo)
	err := svc.ResetPassword(context.Background(), "used-token", "new-password")
	assertAppError(t, err, 400)
}

func TestResetPassword_ExpiredToken(t *testing.T) {
	repo := &mockUserRepo{
		findResetTokenFn: func(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
			return "user-123", "alice@example.com", time.Now().Add(-10 * time.Minute), nil, nil
		},
	}

	svc := newTestAuthService(repo)
	err := svc.ResetPassword(context.Background(), "expired-token", "new-password")
	assertAppError(t, err, 400)
}

func TestResetPassword_UpdatePasswordError(t *testing.T) {
	repo := &mockUserRepo{
		findResetTokenFn: func(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
			return "user-123", "alice@example.com", time.Now().Add(30 * time.Minute), nil, nil
		},
		updatePasswordFn: func(ctx context.Context, userID, passwordHash string) error {
			return errors.New("db write error")
		},
	}

	svc := newTestAuthService(repo)
	err := svc.ResetPassword(context.Background(), "valid-token", "new-password")
	assertAppError(t, err, 500)
}

// --- Hash Token Tests ---

func TestHashToken_Deterministic(t *testing.T) {
	token := "test-token-12345"
	hash1 := hashToken(token)
	hash2 := hashToken(token)
	if hash1 != hash2 {
		t.Error("expected hashToken to be deterministic")
	}
}

func TestHashToken_DifferentInputs(t *testing.T) {
	hash1 := hashToken("token-a")
	hash2 := hashToken("token-b")
	if hash1 == hash2 {
		t.Error("expected different tokens to produce different hashes")
	}
}

func TestHashToken_Length(t *testing.T) {
	hash := hashToken("any-token")
	// SHA-256 = 32 bytes = 64 hex characters.
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(hash))
	}
}

// --- UUID Generation Tests ---

func TestGenerateUUID_Format(t *testing.T) {
	uuid := generateUUID()
	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx (36 chars).
	if len(uuid) != 36 {
		t.Errorf("expected 36-char UUID, got %d chars: %s", len(uuid), uuid)
	}
	if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
		t.Errorf("expected UUID format with dashes, got %s", uuid)
	}
	// Version nibble should be '4'.
	if uuid[14] != '4' {
		t.Errorf("expected version 4 UUID, got version %c", uuid[14])
	}
}

func TestGenerateUUID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		uuid := generateUUID()
		if seen[uuid] {
			t.Fatalf("UUID collision after %d iterations", i)
		}
		seen[uuid] = true
	}
}

// --- ConfigureMailSender Tests ---

func TestConfigureMailSender(t *testing.T) {
	repo := &mockUserRepo{}
	svc := newTestAuthService(repo)

	mail := &mockMailSender{}
	ConfigureMailSender(svc, mail, "https://example.com")

	if svc.mail != mail {
		t.Error("expected mail sender to be set")
	}
	if svc.baseURL != "https://example.com" {
		t.Errorf("expected baseURL https://example.com, got %s", svc.baseURL)
	}
}
