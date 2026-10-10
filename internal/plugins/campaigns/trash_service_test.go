package campaigns

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// userFinderByID answers FindUserByID with a fixed name, for the "deleted by"
// copy MoveToTrash takes.
type userFinderByID struct {
	mockUserFinder
	name string
	err  error
}

func (f *userFinderByID) FindUserByID(_ context.Context, id string) (*MemberUser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &MemberUser{ID: id, DisplayName: f.name}, nil
}

func TestMoveToTrash(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		finder   *userFinderByID
		repoErr  error
		wantBy   string
		wantName string
		wantCode int
		wantRevo bool
	}{
		{"copies the person's name", "u-1", &userFinderByID{name: "alex"}, nil, "u-1", "alex", 0, true},
		{"unknown person is still recorded by id", "u-2", &userFinderByID{err: errors.New("gone")}, nil, "u-2", "", 0, true},
		{"system action has no person", "", &userFinderByID{name: "alex"}, nil, "", "", 0, true},
		{"already in the trash is not found and drops nothing", "u-1", &userFinderByID{name: "alex"}, apperror.NewNotFound("campaign not found"), "u-1", "alex", http.StatusNotFound, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBy, gotName string
			var gotAt time.Time
			repo := &mockCampaignRepo{
				moveToTrashFn: func(_ context.Context, _, by, name string, at time.Time) error {
					gotBy, gotName, gotAt = by, name, at
					return tc.repoErr
				},
			}
			revoker := &mockConnRevoker{}
			svc := &campaignService{repo: repo, users: tc.finder, connRevoker: revoker}

			err := svc.MoveToTrash(context.Background(), "camp-1", tc.userID)
			if tc.wantCode != 0 {
				assertAppError(t, err, tc.wantCode)
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotBy != tc.wantBy || gotName != tc.wantName {
				t.Errorf("recorded by=%q name=%q, want by=%q name=%q", gotBy, gotName, tc.wantBy, tc.wantName)
			}
			if gotAt.IsZero() || gotAt.Location() != time.UTC {
				t.Errorf("deleted_at = %v, want a UTC time", gotAt)
			}
			if got := len(revoker.revokedCampaigns) == 1; got != tc.wantRevo {
				t.Errorf("sockets dropped = %v, want %v", got, tc.wantRevo)
			}
		})
	}
}

func TestPurgeTrashed(t *testing.T) {
	tests := []struct {
		name        string
		claim       bool
		claimErr    error
		purgeErr    error
		cleanerErr  error
		wantPurged  bool
		wantErr     bool
		wantOrder   []string
		wantHook    bool
		wantCleaner bool
	}{
		{"unclaimed campaign is left alone", false, nil, nil, nil, false, false, []string{"claim"}, false, false},
		{"claim error stops everything", false, errors.New("db down"), nil, nil, false, true, []string{"claim"}, false, false},
		{"claimed: files, then the row (hook fires between)", true, nil, nil, nil, true, false, []string{"claim", "media", "sql"}, true, true},
		{"media failure does not stop the delete", true, nil, nil, errors.New("disk"), true, false, []string{"claim", "media", "sql"}, true, true},
		{"another purger finished first is success", true, nil, apperror.NewNotFound("campaign not found"), nil, true, false, []string{"claim", "media", "sql"}, true, true},
		{"sql failure is reported and retried later", true, nil, errors.New("db down"), nil, false, true, []string{"claim", "media", "sql"}, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var order []string
			repo := &mockCampaignRepo{
				claimFn: func(context.Context, string, time.Time) (bool, error) {
					order = append(order, "claim")
					return tc.claim, tc.claimErr
				},
				purgeFn: func(context.Context, string) error {
					order = append(order, "sql")
					return tc.purgeErr
				},
			}
			cleaner := &mockMediaCleaner{deleteFn: func(context.Context, string) (int, error) {
				order = append(order, "media")
				return 2, tc.cleanerErr
			}}
			hook := &mockHookDispatcher{}
			svc := NewCampaignService(repo, &mockUserFinder{}, nil, nil, "http://localhost:8080")
			svc.SetMediaCleaner(cleaner)
			svc.SetHookDispatcher(hook)

			purged, err := svc.PurgeTrashed(context.Background(), "camp-9", time.Time{})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if purged != tc.wantPurged {
				t.Errorf("purged = %v, want %v", purged, tc.wantPurged)
			}
			if !reflect.DeepEqual(order, tc.wantOrder) {
				t.Errorf("order = %v, want %v", order, tc.wantOrder)
			}
			if hook.called != tc.wantHook || cleaner.called != tc.wantCleaner {
				t.Errorf("hook=%v cleaner=%v, want hook=%v cleaner=%v", hook.called, cleaner.called, tc.wantHook, tc.wantCleaner)
			}
			if tc.wantHook && hook.campaignID != "camp-9" {
				t.Errorf("hook got campaign %q", hook.campaignID)
			}
		})
	}
}

// TestPurgeTrashed_NilCleanerAndDispatcher keeps the unwired case working: a
// service built without a media cleaner or hook dispatcher still purges.
func TestPurgeTrashed_NilCleanerAndDispatcher(t *testing.T) {
	svc := NewCampaignService(&mockCampaignRepo{}, &mockUserFinder{}, nil, nil, "http://localhost:8080")
	purged, err := svc.PurgeTrashed(context.Background(), "camp-1", time.Time{})
	if err != nil || !purged {
		t.Fatalf("purged=%v err=%v, want true,nil", purged, err)
	}
}

func TestRestoreFromTrash_PassesRepoRefusal(t *testing.T) {
	repo := &mockCampaignRepo{restoreFn: func(context.Context, string) error {
		return apperror.NewNotFound("that campaign is not in the trash any more")
	}}
	svc := NewCampaignService(repo, &mockUserFinder{}, nil, nil, "http://localhost:8080")
	assertAppError(t, svc.RestoreFromTrash(context.Background(), "camp-1"), http.StatusNotFound)
}
