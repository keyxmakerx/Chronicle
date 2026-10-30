package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// A password re-confirmation covers only the session that made it, and wrong
// passwords are limited per account whichever session or address sends them.
func TestReauth_SessionScopedAndThrottled(t *testing.T) {
	hash, err := hashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	repo := &mockUserRepo{}
	repo.findByIDFn = func(_ context.Context, id string) (*User, error) {
		return &User{ID: id, PasswordHash: hash}, nil
	}
	svc, _ := newTestAuthServiceWithRedis(t, repo)
	ctx := context.Background()

	if err := svc.ConfirmReauth(ctx, "admin", "session-a", "correct horse"); err != nil {
		t.Fatalf("right password refused: %v", err)
	}
	if ok, _ := svc.IsReauthValid(ctx, "admin", "session-a"); !ok {
		t.Fatal("the confirming session isn't re-confirmed")
	}
	if ok, _ := svc.IsReauthValid(ctx, "admin", "session-b"); ok {
		t.Fatal("another session of the same account rides the confirmation")
	}
	if ok, _ := svc.IsReauthValid(ctx, "admin", ""); ok {
		t.Fatal("a request with no session token counts as re-confirmed")
	}

	for i := 0; i < reauthMaxFailures; i++ {
		session := []string{"session-b", "session-c"}[i%2]
		if err := svc.ConfirmReauth(ctx, "admin", session, "wrong"); err == nil {
			t.Fatal("wrong password accepted")
		}
	}
	err = svc.ConfirmReauth(ctx, "admin", "session-d", "correct horse")
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusTooManyRequests {
		t.Fatalf("after %d wrong passwords the right one still runs: %v", reauthMaxFailures, err)
	}
	if ok, _ := svc.IsReauthValid(ctx, "admin", "session-d"); ok {
		t.Fatal("a paused account was re-confirmed")
	}
}
