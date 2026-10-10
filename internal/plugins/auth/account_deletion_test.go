package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

type fakeDeletionHooks struct {
	owned []OwnedCampaignRef
	left  []string
}

func (f *fakeDeletionHooks) OwnedCampaigns(context.Context, string) ([]OwnedCampaignRef, error) {
	return f.owned, nil
}

func (f *fakeDeletionHooks) LeaveAllCampaigns(_ context.Context, userID string) error {
	f.left = append(f.left, userID)
	return nil
}

func TestDeleteOwnAccount(t *testing.T) {
	hash, err := hashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		in       DeleteAccountInput
		isAdmin  bool
		admins   int
		owned    []OwnedCampaignRef
		wantCode int // 0 = success
	}{
		{"deletes", DeleteAccountInput{Password: "correct horse", Confirm: "DELETE"}, false, 1, nil, 0},
		{"confirm word required", DeleteAccountInput{Password: "correct horse", Confirm: "delete"}, false, 1, nil, http.StatusBadRequest},
		{"wrong password", DeleteAccountInput{Password: "nope", Confirm: "DELETE"}, false, 1, nil, http.StatusBadRequest},
		{"last admin refused", DeleteAccountInput{Password: "correct horse", Confirm: "DELETE"}, true, 1, nil, http.StatusConflict},
		{"one of several admins allowed", DeleteAccountInput{Password: "correct horse", Confirm: "DELETE"}, true, 2, nil, 0},
		{"owns a campaign", DeleteAccountInput{Password: "correct horse", Confirm: "DELETE"}, false, 1,
			[]OwnedCampaignRef{{ID: "c1", Name: "Ashen Marches", MemberCount: 5}}, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUserRepo{
				findByIDFn: func(_ context.Context, id string) (*User, error) {
					return &User{ID: id, Email: "mara@example.com", PasswordHash: hash, IsAdmin: tt.isAdmin}, nil
				},
				countAdminsFn: func(context.Context) (int, error) { return tt.admins, nil },
			}
			svc := newTestAuthService(repo)
			hooks := &fakeDeletionHooks{owned: tt.owned}
			ConfigureAccountDeletion(svc, hooks)
			var revoked, after []string
			OnSessionsRevoked(svc, func(_ context.Context, id string) { revoked = append(revoked, id) })
			OnAccountDeleted(svc, func(_ context.Context, id string) { after = append(after, id) })

			err := svc.DeleteOwnAccount(context.Background(), "u1", tt.in)
			if tt.wantCode != 0 {
				assertAppError(t, err, tt.wantCode)
				if len(repo.anonymized) != 0 || len(hooks.left) != 0 || len(after) != 0 {
					t.Fatalf("a refused delete changed something: anonymized=%v left=%v after=%v", repo.anonymized, hooks.left, after)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(hooks.left) != 1 || len(revoked) != 1 || len(after) != 1 {
				t.Fatalf("left=%v revoked=%v after=%v", hooks.left, revoked, after)
			}
			if len(repo.anonymized) != 1 || !strings.Contains(repo.anonymized[0], "deleted+u1@deleted.invalid|"+DeletedDisplayName) {
				t.Fatalf("anonymized = %v", repo.anonymized)
			}
		})
	}
}

func TestDeleteOwnAccountRefusedWhenUnwired(t *testing.T) {
	svc := newTestAuthService(&mockUserRepo{})
	err := svc.DeleteOwnAccount(context.Background(), "u1", DeleteAccountInput{Password: "x", Confirm: "DELETE"})
	if err == nil {
		t.Fatal("deleting without the campaign steps wired must fail")
	}
}
