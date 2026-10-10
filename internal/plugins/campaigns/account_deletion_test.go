package campaigns

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestLeaveAllForDeletedAccount(t *testing.T) {
	tests := []struct {
		name        string
		owned       []OwnedCampaign
		memberships []string
		wantCode    int
		wantRemoved []string
	}{
		{"leaves every campaign", nil, []string{"c1", "c2"}, 0, []string{"c1", "c2"}},
		{"refuses while owning one", []OwnedCampaign{{ID: "c9", Name: "Mine"}}, []string{"c1", "c9"}, http.StatusConflict, nil},
		{"no memberships is fine", nil, nil, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var removed []string
			repo := &mockCampaignRepo{
				ownedByUser:       tt.owned,
				memberCampaignIDs: tt.memberships,
				findMemberFn: func(_ context.Context, campaignID, userID string) (*CampaignMember, error) {
					return &CampaignMember{CampaignID: campaignID, UserID: userID, Role: RolePlayer}, nil
				},
				removeMemberFn: func(_ context.Context, campaignID, _ string) error {
					removed = append(removed, campaignID)
					return nil
				},
			}
			svc := newTestCampaignService(repo, &mockUserFinder{})
			err := svc.LeaveAllForDeletedAccount(context.Background(), "u1")
			if tt.wantCode != 0 {
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != tt.wantCode {
					t.Fatalf("err = %v, want code %d", err, tt.wantCode)
				}
				if len(removed) != 0 || len(repo.transfersCancelledFor) != 0 {
					t.Fatalf("a refused leave changed something: removed=%v", removed)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(removed) != len(tt.wantRemoved) || len(repo.transfersCancelledFor) != 1 {
				t.Fatalf("removed %v (want %v), transfers cancelled %v", removed, tt.wantRemoved, repo.transfersCancelledFor)
			}
		})
	}
}
