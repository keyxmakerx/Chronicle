package connector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

// A paused key is refused from its next call and works again once resumed;
// a change of what it may do applies from its next call, on the same key.
func TestPauseAndChange_ApplyFromTheNextCall(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()
	raw, c, err := svc.Connect(ctx, "camp-1", "owner-1", aiexport.PrivacyModeSafe, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Pause(ctx, "camp-1", "owner-1", c.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrKeyPaused) {
		t.Fatalf("paused key: err = %v, want ErrKeyPaused", err)
	}
	if err := svc.Resume(ctx, "camp-1", "owner-1", c.ID); err != nil {
		t.Fatal(err)
	}
	got, _, err := svc.Authenticate(ctx, "camp-1", raw)
	if err != nil || got.PausedAt != nil {
		t.Fatalf("resumed key: %+v, %v", got, err)
	}
	was, err := svc.ChangeAccess(ctx, "camp-1", "owner-1", c.ID, aiexport.PrivacyModePermitted, false)
	if err != nil {
		t.Fatal(err)
	}
	if was.Privacy != aiexport.PrivacyModeSafe || !was.CanAsk {
		t.Errorf("returned %+v, want the settings before the change", was)
	}
	got, _, err = svc.Authenticate(ctx, "camp-1", raw)
	if err != nil || got.Privacy != aiexport.PrivacyModePermitted || got.CanAsk || got.KeyHash != hashKey(raw) {
		t.Errorf("changed key: %+v, %v", got, err)
	}
}

// Only the owner changes a key, only the key the page shows, and only while
// it works; Everything is not offered.
func TestPauseAndChange_Refusals(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		campaign string
		user     string
		id       func(first, second int64) int64
		setup    func(svc *Service, now *time.Time)
		privacy  aiexport.PrivacyMode
		wantCode int
	}{
		{"not the owner", "camp-1", "player-1", func(_, s int64) int64 { return s }, nil, aiexport.PrivacyModeSafe, 403},
		{"another campaign's owner", "camp-1", "owner-2", func(_, s int64) int64 { return s }, nil, aiexport.PrivacyModeSafe, 403},
		{"another campaign's key", "camp-2", "owner-2", func(_, s int64) int64 { return s }, nil, aiexport.PrivacyModeSafe, 404},
		{"an earlier key", "camp-1", "owner-1", func(f, _ int64) int64 { return f }, nil, aiexport.PrivacyModeSafe, 404},
		{"a key that ran out", "camp-1", "owner-1", func(_, s int64) int64 { return s },
			func(_ *Service, now *time.Time) { *now = now.Add(KeyLifetime + time.Hour) }, aiexport.PrivacyModeSafe, 400},
		{"everything", "camp-1", "owner-1", func(_, s int64) int64 { return s }, nil, aiexport.PrivacyModeEverything, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, _, now := newTestService()
			_, first, _ := svc.Connect(ctx, "camp-1", "owner-1", aiexport.PrivacyModeSafe, false)
			_, second, _ := svc.Connect(ctx, "camp-1", "owner-1", aiexport.PrivacyModeSafe, false)
			if tt.setup != nil {
				tt.setup(svc, now)
			}
			id := tt.id(first.ID, second.ID)
			calls := map[string]error{
				"change": func() error {
					_, err := svc.ChangeAccess(ctx, tt.campaign, tt.user, id, tt.privacy, true)
					return err
				}(),
			}
			if tt.privacy != aiexport.PrivacyModeEverything {
				calls["pause"] = svc.Pause(ctx, tt.campaign, tt.user, id)
				calls["resume"] = svc.Resume(ctx, tt.campaign, tt.user, id)
			}
			for what, err := range calls {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tt.wantCode {
					t.Errorf("%s: err = %v, want code %d", what, err, tt.wantCode)
				}
			}
			for _, c := range repo.conns {
				if c.PausedAt != nil || c.CanAsk {
					t.Errorf("key %d changed: %+v", c.ID, c)
				}
			}
		})
	}
}
