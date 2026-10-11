package campaigns

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestHandoverStatus(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		lastSeen time.Time
		months   int
		wantOpen bool
		wantAt   time.Time
	}{
		{"never allowed", now.AddDate(-2, 0, 0), 0, false, time.Time{}},
		{"not away long enough", now.AddDate(0, -2, 0), 3, false, now.AddDate(0, 1, 0)},
		{"exactly at the limit", now.AddDate(0, -3, 0), 3, true, now},
		{"long gone", now.AddDate(-1, -1, 0), 12, true, now.AddDate(0, -1, 0)},
		{"never seen at all", time.Time{}, 6, false, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := handoverStatus(OwnerPresence{OwnerID: "o", LastSeen: tt.lastSeen}, tt.months, now)
			if st.Open != tt.wantOpen {
				t.Errorf("Open = %v, want %v", st.Open, tt.wantOpen)
			}
			if !st.OpensAt.Equal(tt.wantAt) {
				t.Errorf("OpensAt = %v, want %v", st.OpensAt, tt.wantAt)
			}
		})
	}
}

func TestSetHandoverAfterOnlyTakesTheOfferedChoices(t *testing.T) {
	var saved string
	repo := &mockCampaignRepo{updateSettingsFn: func(_ context.Context, _, s string) error { saved = s; return nil }}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	for _, m := range []int{-1, 1, 24} {
		assertAppError(t, svc.SetHandoverAfter(context.Background(), "camp-1", m), 400)
	}
	if err := svc.SetHandoverAfter(context.Background(), "camp-1", 6); err != nil {
		t.Fatalf("SetHandoverAfter(6): %v", err)
	}
	if want := `"handover_after_months":6`; !strings.Contains(saved, want) {
		t.Errorf("saved settings %s, want them to hold %s", saved, want)
	}
}

// handoverRig is a campaign owned by "owner" with a player, a guest and an
// optional pending transfer, recording what a hand-over changes.
type handoverRig struct {
	repo        *mockCampaignRepo
	transferred []string
	deleted     []string
	roles       map[string]Role
	settings    string
}

func newHandoverRig(months int, away time.Duration) *handoverRig {
	r := &handoverRig{roles: map[string]Role{"owner": RoleOwner, "player": RolePlayer, "guest": RolePlayer}}
	settings := fmt.Sprintf(`{"handover_after_months":%d}`, months)
	r.repo = &mockCampaignRepo{
		ownerPresence: map[string]OwnerPresence{
			"camp-1": {OwnerID: "owner", OwnerName: "Olive", LastSeen: time.Now().UTC().Add(-away)},
		},
		findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
			return &Campaign{ID: id, Name: "The Long Road", Settings: settings, SidebarConfig: "{}"}, nil
		},
		listMembersFn: func(_ context.Context, _ string) ([]CampaignMember, error) {
			return []CampaignMember{
				{UserID: "owner", Role: RoleOwner, DisplayName: "Olive"},
				{UserID: "player", Role: RolePlayer, DisplayName: "Pat"},
				{UserID: "guest", Role: RolePlayer, DisplayName: "Gil", IsGuest: true},
			}, nil
		},
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{UserID: userID, Role: r.roles[userID]}, nil
		},
		updateMemberRoleFn: func(_ context.Context, _, userID string, role Role) error {
			r.roles[userID] = role
			return nil
		},
		findTransferByCampaignFn: func(_ context.Context, _ string) (*OwnershipTransfer, error) {
			return &OwnershipTransfer{ID: "pending-1"}, nil
		},
		deleteTransferFn: func(_ context.Context, id string) error {
			r.deleted = append(r.deleted, id)
			return nil
		},
		updateSettingsFn: func(_ context.Context, _, js string) error {
			r.settings = js
			return nil
		},
		transferOwnershipFn: func(_ context.Context, _, from, to string) error {
			r.transferred = append(r.transferred, from+">"+to)
			r.roles[from], r.roles[to] = RoleScribe, RoleOwner
			return nil
		},
	}
	return r
}

func TestAdminHandOver(t *testing.T) {
	const month = 31 * 24 * time.Hour
	tests := []struct {
		name     string
		months   int
		away     time.Duration
		to       string
		wantCode int
	}{
		{"owner never allowed it", 0, 24 * month, "player", 403},
		{"owner not away long enough", 6, 2 * month, "player", 403},
		{"to the owner themselves", 3, 4 * month, "owner", 400},
		{"to someone outside the campaign", 3, 4 * month, "stranger", 400},
		{"to a guest", 3, 4 * month, "guest", 400},
		{"to a member, once allowed", 3, 4 * month, "player", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rig := newHandoverRig(tt.months, tt.away)
			svc := newTestCampaignService(rig.repo, &mockUserFinder{})
			_, err := svc.AdminHandOver(context.Background(), "camp-1", tt.to)
			if tt.wantCode != 0 {
				assertAppError(t, err, tt.wantCode)
				if len(rig.transferred) != 0 || len(rig.deleted) != 0 {
					t.Errorf("a refused hand-over changed things: transferred %v, deleted %v", rig.transferred, rig.deleted)
				}
				return
			}
			if err != nil {
				t.Fatalf("AdminHandOver: %v", err)
			}
			if len(rig.transferred) != 1 || rig.transferred[0] != "owner>player" {
				t.Errorf("transferred %v, want [owner>player]", rig.transferred)
			}
			if len(rig.deleted) != 1 || rig.deleted[0] != "pending-1" {
				t.Errorf("pending transfer not cancelled: %v", rig.deleted)
			}
			if got := MemberAccess(&Campaign{Settings: rig.settings}, CampaignMember{UserID: "owner", Role: rig.roles["owner"]}); got != AccessCoDM {
				t.Errorf("old owner's access = %q, want Co-DM (settings %s)", got, rig.settings)
			}
		})
	}
}
