// registration_gate_test.go — the beta registration gate (B-R4): the auth
// service enforces open / invite-only / closed, and the first-user-admin
// bootstrap always works regardless of mode.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// fakeRegPolicy returns a fixed registration mode.
type fakeRegPolicy struct{ mode string }

func (f fakeRegPolicy) GetRegistrationMode(_ context.Context) (string, error) { return f.mode, nil }

// errRegPolicy simulates a failed settings read (DB outage).
type errRegPolicy struct{}

func (errRegPolicy) GetRegistrationMode(_ context.Context) (string, error) {
	return "", errors.New("settings unavailable")
}

// fakeInviteChecker treats any non-empty token as valid when `valid` is set
// (email-agnostic — the email-binding behavior is covered separately).
type fakeInviteChecker struct{ valid bool }

func (f fakeInviteChecker) IsRegistrationInviteValid(_ context.Context, token, _ string) bool {
	return f.valid && token != ""
}

// TestRegister_RegistrationGate is the mode × visitor matrix: the first user on
// an empty instance always registers (and becomes admin) in every mode; open
// admits anyone; invite admits only a valid invite; closed admits no one.
func TestRegister_RegistrationGate(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		userCount   int
		inviteToken string
		inviteValid bool
		wantErr     bool
		wantAdmin   bool
	}{
		// First user (empty instance) — always allowed, always admin, ALL modes.
		{name: "open first-user", mode: "open", userCount: 0, wantAdmin: true},
		{name: "invite first-user", mode: "invite", userCount: 0, wantAdmin: true},
		{name: "closed first-user", mode: "closed", userCount: 0, wantAdmin: true},

		// Open mode — anyone registers (not admin once one exists).
		{name: "open stranger", mode: "open", userCount: 3},
		{name: "open with token (ignored)", mode: "open", userCount: 3, inviteToken: "x", inviteValid: false},

		// Invite mode — only a valid invite.
		{name: "invite + valid invite", mode: "invite", userCount: 3, inviteToken: "good", inviteValid: true},
		{name: "invite stranger (no token)", mode: "invite", userCount: 3, wantErr: true},
		{name: "invite stranger (bad token)", mode: "invite", userCount: 3, inviteToken: "bad", inviteValid: false, wantErr: true},

		// Closed mode — no one (even a valid invite).
		{name: "closed + valid invite still blocked", mode: "closed", userCount: 3, inviteToken: "good", inviteValid: true, wantErr: true},
		{name: "closed stranger", mode: "closed", userCount: 3, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var created *User
			repo := &mockUserRepo{
				countUsersFn:  func(context.Context) (int, error) { return tt.userCount, nil },
				emailExistsFn: func(context.Context, string) (bool, error) { return false, nil },
				createFn:      func(_ context.Context, u *User) error { created = u; return nil },
			}
			svc := newTestAuthService(repo)
			svc.regPolicy = fakeRegPolicy{mode: tt.mode}
			svc.inviteChecker = fakeInviteChecker{valid: tt.inviteValid}

			user, err := svc.Register(context.Background(), RegisterInput{
				Email:       "x@example.com",
				DisplayName: "X",
				Password:    "password123",
				InviteToken: tt.inviteToken,
			})

			if tt.wantErr {
				assertAppError(t, err, http.StatusForbidden)
				if created != nil {
					t.Errorf("gate blocked registration but a user was still created")
				}
				return
			}
			if err != nil {
				t.Fatalf("Register: unexpected error: %v", err)
			}
			if user == nil || created == nil {
				t.Fatalf("expected a user to be created")
			}
			if created.IsAdmin != tt.wantAdmin {
				t.Errorf("IsAdmin = %v, want %v (first-user bootstrap)", created.IsAdmin, tt.wantAdmin)
			}
		})
	}
}

// emailBoundChecker mimics the real adapter: the invite is issued to
// invitedEmail, so a non-empty registering email must match it (case-insensitive).
type emailBoundChecker struct{ invitedEmail string }

func (c emailBoundChecker) IsRegistrationInviteValid(_ context.Context, token, email string) bool {
	if token == "" {
		return false
	}
	return email == "" || strings.EqualFold(strings.TrimSpace(email), c.invitedEmail)
}

// TestRegister_InviteEmailBindingSingleUse pins the review fix: an email-scoped
// invite yields at most one account — the invited address registers once, a
// different address cannot consume the invite, and the invited address cannot
// re-register (email uniqueness). This closes the "one invite → unlimited
// accounts" hole.
func TestRegister_InviteEmailBindingSingleUse(t *testing.T) {
	created := map[string]bool{}
	repo := &mockUserRepo{
		countUsersFn:  func(context.Context) (int, error) { return 3, nil }, // not the first user
		emailExistsFn: func(_ context.Context, email string) (bool, error) { return created[strings.ToLower(email)], nil },
		createFn:      func(_ context.Context, u *User) error { created[strings.ToLower(u.Email)] = true; return nil },
	}
	svc := newTestAuthService(repo)
	svc.regPolicy = fakeRegPolicy{mode: "invite"}
	svc.inviteChecker = emailBoundChecker{invitedEmail: "alice@x.com"}

	reg := func(email string) error {
		_, err := svc.Register(context.Background(), RegisterInput{
			Email: email, DisplayName: "U", Password: "password123", InviteToken: "T",
		})
		return err
	}

	if err := reg("alice@x.com"); err != nil {
		t.Fatalf("the invited email must register: %v", err)
	}
	if err := reg("mallory@evil.com"); err == nil {
		t.Error("a different email must not consume the invite")
	} else {
		assertAppError(t, err, http.StatusForbidden)
	}
	if err := reg("alice@x.com"); err == nil {
		t.Error("the invited email must not re-register (one invite → one account)")
	}
}

// TestRegister_FailsClosedOnPolicyError pins the review fix: a settings-read
// error blocks a non-first-user registration (a lockdown control must not
// silently revert to open), while the first-user bootstrap still succeeds.
func TestRegister_FailsClosedOnPolicyError(t *testing.T) {
	repo := &mockUserRepo{
		countUsersFn:  func(context.Context) (int, error) { return 2, nil },
		emailExistsFn: func(context.Context, string) (bool, error) { return false, nil },
		createFn:      func(context.Context, *User) error { return nil },
	}
	svc := newTestAuthService(repo)
	svc.regPolicy = errRegPolicy{}

	if _, err := svc.Register(context.Background(), RegisterInput{
		Email: "x@x.com", DisplayName: "X", Password: "password123",
	}); err == nil {
		t.Error("a settings-read error must fail closed for a non-first user")
	} else {
		assertAppError(t, err, http.StatusForbidden)
	}

	repo.countUsersFn = func(context.Context) (int, error) { return 0, nil }
	if _, err := svc.Register(context.Background(), RegisterInput{
		Email: "admin@x.com", DisplayName: "A", Password: "password123",
	}); err != nil {
		t.Errorf("first-user bootstrap must survive a settings-read error: %v", err)
	}
}

// TestRegister_GateFailsOpenWithoutPolicy pins the safety default: with no
// policy/checker wired, registration behaves exactly as before (open), so a
// partial or absent wiring can never lock out an instance.
func TestRegister_GateFailsOpenWithoutPolicy(t *testing.T) {
	repo := &mockUserRepo{
		countUsersFn:  func(context.Context) (int, error) { return 5, nil },
		emailExistsFn: func(context.Context, string) (bool, error) { return false, nil },
		createFn:      func(context.Context, *User) error { return nil },
	}
	svc := newTestAuthService(repo) // no regPolicy, no inviteChecker
	if _, err := svc.Register(context.Background(), RegisterInput{
		Email: "y@example.com", DisplayName: "Y", Password: "password123",
	}); err != nil {
		t.Fatalf("registration must stay open when no gate is wired, got: %v", err)
	}
}

// TestRegistrationStatus covers the register-page decision helper: first user is
// always allowed; strangers are blocked in invite/closed; a valid invite passes.
func TestRegistrationStatus(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		userCount   int
		inviteToken string
		inviteValid bool
		wantAllowed bool
	}{
		{name: "closed first-user allowed (bootstrap)", mode: "closed", userCount: 0, wantAllowed: true},
		{name: "open stranger allowed", mode: "open", userCount: 2, wantAllowed: true},
		{name: "invite stranger blocked", mode: "invite", userCount: 2, wantAllowed: false},
		{name: "invite valid token allowed", mode: "invite", userCount: 2, inviteToken: "good", inviteValid: true, wantAllowed: true},
		{name: "closed stranger blocked", mode: "closed", userCount: 2, wantAllowed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUserRepo{countUsersFn: func(context.Context) (int, error) { return tt.userCount, nil }}
			svc := newTestAuthService(repo)
			svc.regPolicy = fakeRegPolicy{mode: tt.mode}
			svc.inviteChecker = fakeInviteChecker{valid: tt.inviteValid}

			mode, allowed, err := svc.RegistrationStatus(context.Background(), tt.inviteToken)
			if err != nil {
				t.Fatalf("RegistrationStatus: %v", err)
			}
			if mode != tt.mode {
				t.Errorf("mode = %q, want %q", mode, tt.mode)
			}
			if allowed != tt.wantAllowed {
				t.Errorf("allowed = %v, want %v", allowed, tt.wantAllowed)
			}
		})
	}
}
