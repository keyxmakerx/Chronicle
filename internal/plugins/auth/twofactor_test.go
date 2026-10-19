package auth

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

// twoFactorRig is an auth service over miniredis whose repo keeps the
// two-factor columns, so a test can walk the whole flow.
type twoFactorRig struct {
	svc  *authService
	repo *mockUserRepo
	mail *mockMailSender
	// hash is every account's stored password hash; a test swaps it to
	// stand for a password change.
	hash string
}

func newTwoFactorRig(t *testing.T) *twoFactorRig {
	t.Helper()
	hash, err := hashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	repo := &mockUserRepo{}
	r := &twoFactorRig{repo: repo, hash: hash}
	repo.findByIDFn = func(_ context.Context, id string) (*User, error) {
		return &User{ID: id, Email: id + "@example.com", DisplayName: id, PasswordHash: r.hash,
			TOTPSecret: repo.totpSecret[id], TOTPEnabled: repo.totpEnabled[id]}, nil
	}
	repo.findByEmailFn = func(ctx context.Context, email string) (*User, error) {
		return repo.findByIDFn(ctx, strings.TrimSuffix(email, "@example.com"))
	}
	svc, _ := newTestAuthServiceWithRedis(t, repo)
	ConfigureTwoFactor(svc, "test-site-secret-of-at-least-thirty-two")
	mail := &mockMailSender{}
	ConfigureMailSender(svc, mail, "https://example.com")
	r.svc, r.mail = svc, mail
	return r
}

// turnOn enables two-factor for id and returns the raw secret and codes.
func (r *twoFactorRig) turnOn(t *testing.T, id string) (string, []string) {
	t.Helper()
	ctx := context.Background()
	setup, err := r.svc.BeginTwoFactor(ctx, id, "correct horse")
	if err != nil {
		t.Fatalf("BeginTwoFactor: %v", err)
	}
	secret := strings.ReplaceAll(setup.Secret, " ", "")
	code, _ := totp.GenerateCode(secret, time.Now())
	codes, err := r.svc.EnableTwoFactor(ctx, id, code)
	if err != nil {
		t.Fatalf("EnableTwoFactor: %v", err)
	}
	enabledWith.Store(secret, code)
	return secret, codes
}

// enabledWith maps each secret to the code turnOn spent on it.
var enabledWith sync.Map

// unusedCode is a code the service accepts that turnOn has not spent: this
// step's code, or the next step's when a step boundary has passed since
// turnOn and this step's code is the spent one. Either stays accepted if
// one more boundary passes before the service checks it.
func unusedCode(secret string) string {
	now := time.Now()
	c, _ := totp.GenerateCode(secret, now)
	if used, _ := enabledWith.Load(secret); used == c {
		c, _ = totp.GenerateCode(secret, now.Add(30*time.Second))
	}
	return c
}

func (r *twoFactorRig) passwordStep(t *testing.T, id, device string) string {
	t.Helper()
	_, _, err := r.svc.Login(context.Background(), LoginInput{Email: id + "@example.com", Password: "correct horse", TrustedDevice: device})
	var need *TwoFactorRequired
	if !errors.As(err, &need) {
		t.Fatalf("Login error = %v, want TwoFactorRequired", err)
	}
	return need.Challenge
}

func TestTwoFactorSetup(t *testing.T) {
	r := newTwoFactorRig(t)
	ctx := context.Background()

	_, err := r.svc.BeginTwoFactor(ctx, "mara", "wrong")
	assertAppError(t, err, http.StatusBadRequest)

	setup, err := r.svc.BeginTwoFactor(ctx, "mara", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(setup.QR, "data:image/png;base64,") {
		t.Fatalf("QR = %.40q", setup.QR)
	}
	if r.repo.totpEnabled["mara"] {
		t.Fatal("two-factor came on before the code was confirmed")
	}

	_, err = r.svc.EnableTwoFactor(ctx, "mara", "000000")
	assertAppError(t, err, http.StatusBadRequest)

	secret := strings.ReplaceAll(setup.Secret, " ", "")
	code, _ := totp.GenerateCode(secret, time.Now())
	codes, err := r.svc.EnableTwoFactor(ctx, "mara", code)
	if err != nil {
		t.Fatal(err)
	}
	if !r.repo.totpEnabled["mara"] {
		t.Fatal("two-factor not on")
	}
	stored := *r.repo.totpSecret["mara"]
	if strings.Contains(stored, secret) {
		t.Fatal("secret stored in the clear")
	}
	if opened, err := r.svc.openSecret(stored); err != nil || opened != secret {
		t.Fatalf("stored secret opens to %q, %v", opened, err)
	}
	if len(codes) != recoveryCodeCount {
		t.Fatalf("got %d recovery codes", len(codes))
	}
	st, _ := r.svc.TwoFactorStatus(ctx, "mara")
	if !st.Enabled || st.CodesLeft != recoveryCodeCount {
		t.Fatalf("status = %+v", st)
	}
	_, err = r.svc.BeginTwoFactor(ctx, "mara", "correct horse")
	assertAppError(t, err, http.StatusConflict)
}

func TestTwoFactorLogin(t *testing.T) {
	r := newTwoFactorRig(t)
	ctx := context.Background()
	secret, _ := r.turnOn(t, "mara")

	token, user, err := r.svc.Login(ctx, LoginInput{Email: "mara@example.com", Password: "correct horse"})
	var need *TwoFactorRequired
	if !errors.As(err, &need) || token != "" {
		t.Fatalf("Login = %q, %v; want no session and a code step", token, err)
	}
	if user == nil || need.Challenge == "" {
		t.Fatal("no challenge")
	}

	_, err = r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: "made-up", Code: unusedCode(secret)})
	assertAppError(t, err, http.StatusUnauthorized)

	code := unusedCode(secret)
	res, err := r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: need.Challenge, Code: code, Remember: true})
	if err != nil {
		t.Fatalf("CompleteTwoFactorLogin: %v", err)
	}
	if res.SessionToken == "" || res.TrustedDevice == "" {
		t.Fatalf("result = %+v", res)
	}
	if _, err := r.svc.ValidateSession(ctx, res.SessionToken); err != nil {
		t.Fatalf("session not valid: %v", err)
	}

	// The challenge is single-use.
	_, err = r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: need.Challenge, Code: code})
	assertAppError(t, err, http.StatusUnauthorized)

	// A code can't be used twice, even inside its time window.
	ch := r.passwordStep(t, "mara", "")
	_, err = r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: ch, Code: code})
	assertAppError(t, err, http.StatusUnauthorized)

	// The remembered device skips the code step, but only for its owner.
	if tok, _, err := r.svc.Login(ctx, LoginInput{Email: "mara@example.com", Password: "correct horse", TrustedDevice: res.TrustedDevice}); err != nil || tok == "" {
		t.Fatalf("trusted device still asked for a code: %v", err)
	}
	r.turnOn(t, "sam")
	r.passwordStep(t, "sam", res.TrustedDevice)

	// Changing the password forgets remembered devices and ends a
	// half-finished sign-in.
	pending := r.passwordStep(t, "mara", "")
	if _, err := r.svc.ChangePassword(ctx, "mara", "correct horse", "correct horse", "", ""); err != nil {
		t.Fatal(err)
	}
	r.passwordStep(t, "mara", res.TrustedDevice)
	newHash, _ := hashPassword("correct horse")
	r.hash = newHash
	_, err = r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: pending, Code: unusedCode(secret)})
	assertAppError(t, err, http.StatusUnauthorized)
	if !strings.Contains(err.Error(), "start again") {
		t.Fatalf("challenge from before the password change: %v", err)
	}
}

func TestTwoFactorRecoveryCodes(t *testing.T) {
	r := newTwoFactorRig(t)
	ctx := context.Background()
	_, codes := r.turnOn(t, "mara")

	shape := regexp.MustCompile(`^[a-z2-9]{4}-[a-z2-9]{4}-[a-z2-9]{4}$`)
	seen := map[string]bool{}
	for _, c := range codes {
		if !shape.MatchString(c) || seen[c] {
			t.Fatalf("bad or repeated code %q", c)
		}
		seen[c] = true
	}

	// Typed in capitals without the dash still matches, once.
	typed := strings.ToUpper(strings.ReplaceAll(codes[0], "-", " "))
	ch := r.passwordStep(t, "mara", "")
	if _, err := r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: ch, Code: typed}); err != nil {
		t.Fatalf("recovery code refused: %v", err)
	}
	ch = r.passwordStep(t, "mara", "")
	_, err := r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: ch, Code: codes[0]})
	assertAppError(t, err, http.StatusUnauthorized)

	st, _ := r.svc.TwoFactorStatus(ctx, "mara")
	if st.CodesLeft != recoveryCodeCount-1 {
		t.Fatalf("codes left = %d", st.CodesLeft)
	}

	// New codes need a code as well as the password, and replace the old ones.
	_, err = r.svc.RegenerateRecoveryCodes(ctx, "mara", "correct horse", "")
	assertAppError(t, err, http.StatusBadRequest)
	fresh, err := r.svc.RegenerateRecoveryCodes(ctx, "mara", "correct horse", codes[3])
	if err != nil {
		t.Fatal(err)
	}
	ch = r.passwordStep(t, "mara", "")
	_, err = r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: ch, Code: codes[1]})
	assertAppError(t, err, http.StatusUnauthorized)
	if _, err := r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: ch, Code: fresh[0]}); err != nil {
		t.Fatalf("new code refused: %v", err)
	}
}

func TestTwoFactorLockAfterFiveWrongCodes(t *testing.T) {
	r := newTwoFactorRig(t)
	ctx := context.Background()
	secret, _ := r.turnOn(t, "mara")
	ch := r.passwordStep(t, "mara", "")
	for i := 0; i < twoFactorMaxFailures-1; i++ {
		_, err := r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: ch, Code: "000000"})
		assertAppError(t, err, http.StatusUnauthorized)
	}
	_, err := r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: ch, Code: "000000"})
	assertAppError(t, err, http.StatusTooManyRequests)

	// Locked: a right code is refused too, and a fresh password step doesn't reset it.
	ch = r.passwordStep(t, "mara", "")
	_, err = r.svc.CompleteTwoFactorLogin(ctx, TwoFactorLoginInput{Challenge: ch, Code: unusedCode(secret)})
	assertAppError(t, err, http.StatusTooManyRequests)
}

func TestTwoFactorTurnOff(t *testing.T) {
	r := newTwoFactorRig(t)
	ctx := context.Background()
	secret, _ := r.turnOn(t, "mara")

	assertAppError(t, r.svc.DisableTwoFactor(ctx, "mara", "wrong", unusedCode(secret)), http.StatusBadRequest)
	assertAppError(t, r.svc.DisableTwoFactor(ctx, "mara", "correct horse", ""), http.StatusBadRequest)
	assertAppError(t, r.svc.DisableTwoFactor(ctx, "mara", "correct horse", "000000"), http.StatusUnauthorized)
	if err := r.svc.DisableTwoFactor(ctx, "mara", "correct horse", unusedCode(secret)); err != nil {
		t.Fatal(err)
	}
	if r.repo.totpEnabled["mara"] || r.repo.totpSecret["mara"] != nil {
		t.Fatal("two-factor still on")
	}
	if n, _ := r.repo.CountRecoveryCodes(ctx, "mara"); n != 0 {
		t.Fatalf("%d recovery codes left", n)
	}
	if tok, _, err := r.svc.Login(ctx, LoginInput{Email: "mara@example.com", Password: "correct horse"}); err != nil || tok == "" {
		t.Fatalf("password alone should sign in now: %v", err)
	}
}

func TestAdminDisableTwoFactorEmailsThePerson(t *testing.T) {
	r := newTwoFactorRig(t)
	ctx := context.Background()
	assertAppError(t, r.svc.AdminDisableTwoFactor(ctx, "mara"), http.StatusBadRequest)

	r.turnOn(t, "mara")
	if err := r.svc.AdminDisableTwoFactor(ctx, "mara"); err != nil {
		t.Fatal(err)
	}
	if r.repo.totpEnabled["mara"] {
		t.Fatal("two-factor still on")
	}
	if r.mail.sendCount != 1 || r.mail.lastTo[0] != "mara@example.com" || !strings.Contains(r.mail.lastBody, "switched off") {
		t.Fatalf("mail = %d to %v: %q", r.mail.sendCount, r.mail.lastTo, r.mail.lastBody)
	}
}

func TestDeleteAccountAsksForCodeWhenTwoFactorIsOn(t *testing.T) {
	r := newTwoFactorRig(t)
	ctx := context.Background()
	_, codes := r.turnOn(t, "mara")
	ConfigureAccountDeletion(r.svc, &fakeDeletionHooks{})

	assertAppError(t, r.svc.DeleteOwnAccount(ctx, "mara", DeleteAccountInput{Password: "correct horse", Confirm: "DELETE"}), http.StatusBadRequest)
	assertAppError(t, r.svc.DeleteOwnAccount(ctx, "mara", DeleteAccountInput{Password: "correct horse", Confirm: "DELETE", Code: "000000"}), http.StatusUnauthorized)
	if len(r.repo.anonymized) != 0 {
		t.Fatal("deleted without a right code")
	}
	if err := r.svc.DeleteOwnAccount(ctx, "mara", DeleteAccountInput{Password: "correct horse", Confirm: "DELETE", Code: codes[2]}); err != nil {
		t.Fatalf("delete with a recovery code: %v", err)
	}
}

func TestRecoveryCodeNormalizing(t *testing.T) {
	tests := []struct{ in, want string }{
		{"k7qe-2m9x-b4fz", "k7qe2m9xb4fz"},
		{"K7QE 2M9X", "k7qe2m9x"},
		{" k7qe-2m9x ", "k7qe2m9x"},
	}
	for _, tt := range tests {
		if got := normalizeRecoveryCode(tt.in); got != tt.want {
			t.Errorf("normalizeRecoveryCode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestForgotPasswordPageWithoutEmail(t *testing.T) {
	tests := []struct {
		name      string
		mailReady bool
		want      string
		wantForm  bool
	}{
		{"email set up", true, "send a reset link", true},
		{"no email", false, "isn't set up to send email", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := ForgotPasswordPage("tok", "", "", tt.mailReady).Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			if !strings.Contains(html, tt.want) {
				t.Errorf("page lacks %q", tt.want)
			}
			if got := strings.Contains(html, `action="/forgot-password"`); got != tt.wantForm {
				t.Errorf("form shown = %v, want %v", got, tt.wantForm)
			}
		})
	}

	svc := newTestAuthService(&mockUserRepo{})
	if svc.CanEmailResetLinks(context.Background()) {
		t.Error("no mail sender wired, yet reset links count as sendable")
	}
	ConfigureMailSender(svc, &mockMailSender{isConfiguredFn: func(context.Context) bool { return false }}, "")
	if svc.CanEmailResetLinks(context.Background()) {
		t.Error("unconfigured email counts as sendable")
	}
}

// Changing the password ends every session, so the caller must get a fresh
// one back or the next click signs them out.
func TestChangePasswordKeepsCallerSignedIn(t *testing.T) {
	r := newTwoFactorRig(t)
	ctx := context.Background()
	old, _, err := r.svc.Login(ctx, LoginInput{Email: "mara@example.com", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := r.svc.ChangePassword(ctx, "mara", "correct horse", "another horse", "1.2.3.4", "ua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.ValidateSession(ctx, old); err == nil {
		t.Fatal("old session survived the password change")
	}
	sess, err := r.svc.ValidateSession(ctx, token)
	if err != nil || sess.UserID != "mara" {
		t.Fatalf("fresh session = %+v, %v", sess, err)
	}
}
