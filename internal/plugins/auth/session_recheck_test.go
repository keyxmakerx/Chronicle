package auth

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestForceSessionRevalidation pins the post-restore guarantee: a session
// that was fresh a moment ago is re-checked on its next request, so one whose
// user is missing from the restored database ends, one whose user remains
// keeps working with the restored details, and no session gains lifetime.
func TestForceSessionRevalidation(t *testing.T) {
	repo := &mockUserRepo{
		findByIDFn: func(_ context.Context, id string) (*User, error) {
			if id == "gone" {
				return nil, apperror.NewNotFound("user not found")
			}
			return &User{ID: id, Email: id + "@example.com", DisplayName: "Restored " + id, IsAdmin: false}, nil
		},
	}
	svc, mr := newTestAuthServiceWithRedis(t, repo)

	fresh := time.Now()
	gone := seedSession(t, svc, mr, Session{UserID: "gone", IsAdmin: true, LastValidated: fresh})
	demoted := seedSession(t, svc, mr, Session{UserID: "kept", IsAdmin: true, Name: "Old", LastValidated: fresh})

	// Without the nudge both sessions are inside the revalidation window and
	// would be trusted as they are.
	if s, err := svc.ValidateSession(context.Background(), gone); err != nil || !s.IsAdmin {
		t.Fatalf("precondition: fresh session should be trusted, got %v %v", s, err)
	}

	n, err := ForceSessionRevalidation(context.Background(), svc)
	if err != nil || n != 2 {
		t.Fatalf("ForceSessionRevalidation = %d, %v; want 2, nil", n, err)
	}

	if _, err := svc.ValidateSession(context.Background(), gone); err == nil {
		t.Error("a session whose user is not in the restored database survived")
	}
	s, err := svc.ValidateSession(context.Background(), demoted)
	if err != nil {
		t.Fatalf("session of a surviving user was dropped: %v", err)
	}
	if s.IsAdmin || s.Name != "Restored kept" {
		t.Errorf("session kept stale details: admin=%v name=%q", s.IsAdmin, s.Name)
	}
	if ttl := mr.TTL(sessionKeyPrefix + demoted); ttl <= 0 || ttl > svc.sessionTTL {
		t.Errorf("ttl after the nudge = %v, want within (0, %v]", ttl, svc.sessionTTL)
	}
}

// TestForceSessionRevalidationIgnoresOtherKeys pins that only session keys are
// touched and that a service without Redis is a no-op.
func TestForceSessionRevalidationIgnoresOtherKeys(t *testing.T) {
	svc, mr := newTestAuthServiceWithRedis(t, &mockUserRepo{})
	_ = mr.Set("user_sessions:u1", "keep")
	_ = mr.Set("last_seen:u1", "keep")
	if n, err := ForceSessionRevalidation(context.Background(), svc); err != nil || n != 0 {
		t.Fatalf("= %d, %v; want 0, nil", n, err)
	}
	if v, _ := mr.Get("user_sessions:u1"); v != "keep" {
		t.Error("user session index was modified")
	}
	if n, err := ForceSessionRevalidation(context.Background(), NewAuthService(&mockUserRepo{}, nil, 0)); err != nil || n != 0 {
		t.Errorf("nil redis = %d, %v; want 0, nil", n, err)
	}
}
