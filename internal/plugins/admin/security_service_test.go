package admin

// security_service_test.go pins that disabling a user account drops its
// live sockets in every campaign, after the account is disabled and its
// sessions are destroyed — an open socket is never rechecked otherwise.

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// Narrow test doubles: auth.UserRepository/AuthService are large, so
// embedding them means only the methods under test need overriding.

type fakeUserRepo struct {
	auth.UserRepository
	findByIDFn         func(ctx context.Context, id string) (*auth.User, error)
	updateIsDisabledFn func(ctx context.Context, id string, disabled bool) error
}

func (f *fakeUserRepo) FindByID(ctx context.Context, id string) (*auth.User, error) {
	return f.findByIDFn(ctx, id)
}

func (f *fakeUserRepo) UpdateIsDisabled(ctx context.Context, id string, disabled bool) error {
	if f.updateIsDisabledFn == nil {
		return nil
	}
	return f.updateIsDisabledFn(ctx, id, disabled)
}

type fakeAuthService struct {
	auth.AuthService
	destroyAllUserSessionsFn func(ctx context.Context, userID string) (int, error)
}

func (f *fakeAuthService) DestroyAllUserSessions(ctx context.Context, userID string) (int, error) {
	if f.destroyAllUserSessionsFn == nil {
		return 0, nil
	}
	return f.destroyAllUserSessionsFn(ctx, userID)
}

// mockConnRevoker records RevokeUserEverywhere calls for assertions.
type mockConnRevoker struct {
	revokedUserIDs []string
}

func (m *mockConnRevoker) RevokeUserEverywhere(userID string) {
	m.revokedUserIDs = append(m.revokedUserIDs, userID)
}

// activeUser is a plain, enabled, non-admin account — the only shape
// DisableUser should be able to act on.
func activeUser(id string) *auth.User {
	return &auth.User{ID: id, IsDisabled: false, IsAdmin: false}
}

// TestDisableUser_DropsLiveConnectionsEverywhere pins that RevokeUserEverywhere
// fires only after the disable write and session destroy both complete —
// a sequence log records the order, not just that all three happened.
func TestDisableUser_DropsLiveConnectionsEverywhere(t *testing.T) {
	var sequence []string
	repo := &fakeUserRepo{
		findByIDFn: func(context.Context, string) (*auth.User, error) {
			return activeUser("u-1"), nil
		},
		updateIsDisabledFn: func(_ context.Context, _ string, disabled bool) error {
			if disabled {
				sequence = append(sequence, "disabled")
			}
			return nil
		},
	}
	authSvc := &fakeAuthService{
		destroyAllUserSessionsFn: func(context.Context, string) (int, error) {
			sequence = append(sequence, "sessions_destroyed")
			return 2, nil
		},
	}
	revoker := &mockConnRevoker{}

	svc := NewSecurityService(nil, repo, authSvc)
	// Wrap the revoker's own call in the sequence log too, via a small
	// adapter, so ordering is observable from one slice.
	svc.SetConnectionRevoker(sequencingRevoker{revoker, &sequence})

	if err := svc.DisableUser(context.Background(), "u-1"); err != nil {
		t.Fatalf("DisableUser: %v", err)
	}

	if len(revoker.revokedUserIDs) != 1 || revoker.revokedUserIDs[0] != "u-1" {
		t.Errorf("revokedUserIDs = %v, want [u-1] — disabling an account must drop its "+
			"live sockets in every campaign", revoker.revokedUserIDs)
	}

	want := []string{"disabled", "sessions_destroyed", "revoked"}
	if len(sequence) != len(want) {
		t.Fatalf("sequence = %v, want %v", sequence, want)
	}
	for i, step := range want {
		if sequence[i] != step {
			t.Errorf("sequence[%d] = %q, want %q (full sequence: %v) — revocation must happen "+
				"after the account is disabled and its sessions are destroyed", i, sequence[i], step, sequence)
		}
	}
}

// sequencingRevoker wraps a mockConnRevoker to also append to a shared
// sequence log, so TestDisableUser_DropsLiveConnectionsEverywhere can
// assert ordering against the repo/session-destroy steps in one slice.
type sequencingRevoker struct {
	inner    *mockConnRevoker
	sequence *[]string
}

func (s sequencingRevoker) RevokeUserEverywhere(userID string) {
	*s.sequence = append(*s.sequence, "revoked")
	s.inner.RevokeUserEverywhere(userID)
}

// TestDisableUser_UnwiredRevokerStillDisables matches the fail-open
// convention used elsewhere (RevokeKey, DisableForCampaign, ...): the
// revoker is late-bound, so a nil revoker must not block the disable.
func TestDisableUser_UnwiredRevokerStillDisables(t *testing.T) {
	disabled := false
	repo := &fakeUserRepo{
		findByIDFn: func(context.Context, string) (*auth.User, error) {
			return activeUser("u-1"), nil
		},
		updateIsDisabledFn: func(context.Context, string, bool) error {
			disabled = true
			return nil
		},
	}
	authSvc := &fakeAuthService{}

	svc := NewSecurityService(nil, repo, authSvc)
	if err := svc.DisableUser(context.Background(), "u-1"); err != nil {
		t.Fatalf("unexpected error with no connection revoker wired: %v", err)
	}
	if !disabled {
		t.Error("account was not disabled even though the revoker was never wired")
	}
}

// TestDisableUser_AlreadyDisabled_DoesNotRevoke keeps the fix scoped: the
// existing already-disabled guard fires before anything else runs.
func TestDisableUser_AlreadyDisabled_DoesNotRevoke(t *testing.T) {
	repo := &fakeUserRepo{
		findByIDFn: func(context.Context, string) (*auth.User, error) {
			return &auth.User{ID: "u-1", IsDisabled: true}, nil
		},
	}
	revoker := &mockConnRevoker{}
	svc := NewSecurityService(nil, repo, &fakeAuthService{})
	svc.SetConnectionRevoker(revoker)

	err := svc.DisableUser(context.Background(), "u-1")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != 400 {
		t.Fatalf("expected a 400 AppError for an already-disabled user, got %v", err)
	}
	if len(revoker.revokedUserIDs) != 0 {
		t.Errorf("revokedUserIDs = %v, want none — nothing changed for an already-disabled account", revoker.revokedUserIDs)
	}
}

// TestDisableUser_AdminAccount_DoesNotRevoke keeps the fix scoped: the
// existing admin guard fires before anything else runs.
func TestDisableUser_AdminAccount_DoesNotRevoke(t *testing.T) {
	repo := &fakeUserRepo{
		findByIDFn: func(context.Context, string) (*auth.User, error) {
			return &auth.User{ID: "u-1", IsAdmin: true}, nil
		},
	}
	revoker := &mockConnRevoker{}
	svc := NewSecurityService(nil, repo, &fakeAuthService{})
	svc.SetConnectionRevoker(revoker)

	err := svc.DisableUser(context.Background(), "u-1")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != 400 {
		t.Fatalf("expected a 400 AppError for an admin account, got %v", err)
	}
	if len(revoker.revokedUserIDs) != 0 {
		t.Errorf("revokedUserIDs = %v, want none — an admin account was never disabled", revoker.revokedUserIDs)
	}
}
