package campaigns

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func accessTestCampaign(ids ...string) *Campaign {
	s, _ := json.Marshal(CampaignSettings{DmGrantIDs: ids})
	return &Campaign{ID: "c1", Settings: string(s), SidebarConfig: "{}"}
}

func TestMemberAccess(t *testing.T) {
	c := accessTestCampaign("granted")
	cases := []struct {
		name      string
		m         CampaignMember
		wantMenu  string
		wantLabel string
	}{
		{"owner", CampaignMember{UserID: "o", Role: RoleOwner}, AccessPlayer, "Owner"},
		{"owner with a grant is still Owner", CampaignMember{UserID: "granted", Role: RoleOwner}, AccessPlayer, "Owner"},
		{"scribe with grant is Co-DM", CampaignMember{UserID: "granted", Role: RoleScribe}, AccessCoDM, "Co-DM"},
		{"scribe without grant", CampaignMember{UserID: "s", Role: RoleScribe}, AccessScribe, "Scribe"},
		{"player with grant keeps reading as player in the menu", CampaignMember{UserID: "granted", Role: RolePlayer}, AccessPlayer, "Player with DM access"},
		{"plain player", CampaignMember{UserID: "p", Role: RolePlayer}, AccessPlayer, "Player"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MemberAccess(c, tc.m); got != tc.wantMenu {
				t.Fatalf("MemberAccess = %q, want %q", got, tc.wantMenu)
			}
			if got := MemberAccessLabel(c, tc.m); got != tc.wantLabel {
				t.Fatalf("MemberAccessLabel = %q, want %q", got, tc.wantLabel)
			}
		})
	}
}

func TestMemberAccessNote(t *testing.T) {
	c := accessTestCampaign("granted")
	if MemberAccessNote(c, CampaignMember{UserID: "granted", Role: RolePlayer}) == "" {
		t.Fatal("player with a grant should get a note")
	}
	for _, m := range []CampaignMember{
		{UserID: "granted", Role: RoleScribe},
		{UserID: "p", Role: RolePlayer},
	} {
		if MemberAccessNote(c, m) != "" {
			t.Fatalf("unexpected note for %+v", m)
		}
	}
}

func TestSetMemberAccess(t *testing.T) {
	cases := []struct {
		name       string
		access     string
		member     *CampaignMember // nil: not a member
		grants     []string
		stale      string // a granted id whose account is gone
		missingCmp bool
		wantCode   int
		wantRole   *Role // nil: no role write expected
		wantGrants []string
		wantWrite  bool // settings written
	}{
		{name: "invalid choice", access: "wizard", member: &CampaignMember{Role: RolePlayer}, wantCode: http.StatusBadRequest},
		{name: "owner is a refused target", access: AccessPlayer, member: &CampaignMember{Role: RoleOwner}, wantCode: http.StatusBadRequest},
		{name: "not a member", access: AccessScribe, member: nil, wantCode: http.StatusNotFound},
		{name: "campaign missing", access: AccessScribe, member: &CampaignMember{Role: RolePlayer}, missingCmp: true, wantCode: http.StatusNotFound},
		{name: "player to codm sets scribe and adds the grant", access: AccessCoDM, member: &CampaignMember{Role: RolePlayer},
			wantRole: ptrRole(RoleScribe), wantGrants: []string{"t"}, wantWrite: true},
		{name: "codm to player drops the grant and sets player", access: AccessPlayer, member: &CampaignMember{Role: RoleScribe}, grants: []string{"other", "t"},
			wantRole: ptrRole(RolePlayer), wantGrants: []string{"other"}, wantWrite: true},
		{name: "codm to scribe drops the grant, role unchanged", access: AccessScribe, member: &CampaignMember{Role: RoleScribe}, grants: []string{"t"},
			wantGrants: []string{}, wantWrite: true},
		{name: "scribe to codm adds the grant, role unchanged", access: AccessCoDM, member: &CampaignMember{Role: RoleScribe},
			wantGrants: []string{"t"}, wantWrite: true},
		{name: "already codm changes nothing", access: AccessCoDM, member: &CampaignMember{Role: RoleScribe}, grants: []string{"t"}},
		// A grant left by a deleted account must not block a revoke.
		{name: "codm to player with a stale grant still revokes", access: AccessPlayer, member: &CampaignMember{Role: RoleScribe}, grants: []string{"gone", "t"}, stale: "gone",
			wantRole: ptrRole(RolePlayer), wantGrants: []string{"gone"}, wantWrite: true},
		{name: "player to codm drops a stale grant", access: AccessCoDM, member: &CampaignMember{Role: RolePlayer}, grants: []string{"gone"}, stale: "gone",
			wantRole: ptrRole(RoleScribe), wantGrants: []string{"t"}, wantWrite: true},
		{name: "plain player to player changes nothing", access: AccessPlayer, member: &CampaignMember{Role: RolePlayer}},
		{name: "player with a legacy grant chosen player replaces the grant", access: AccessPlayer, member: &CampaignMember{Role: RolePlayer}, grants: []string{"t"},
			wantGrants: []string{}, wantWrite: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var roleWrites []Role
			var written *CampaignSettings
			repo := &mockCampaignRepo{
				findMemberFn: func(_ context.Context, _, id string) (*CampaignMember, error) {
					if tc.member == nil || (tc.stale != "" && id == tc.stale) {
						return nil, apperror.NewNotFound("member not found")
					}
					m := *tc.member
					m.UserID = id
					return &m, nil
				},
				findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
					if tc.missingCmp {
						return nil, nil
					}
					return accessTestCampaign(tc.grants...), nil
				},
				updateMemberRoleFn: func(_ context.Context, _, _ string, r Role) error {
					roleWrites = append(roleWrites, r)
					return nil
				},
				updateSettingsFn: func(_ context.Context, _, js string) error {
					var s CampaignSettings
					if err := json.Unmarshal([]byte(js), &s); err != nil {
						t.Fatal(err)
					}
					written = &s
					return nil
				},
			}
			svc := newTestCampaignService(repo, nil).(*campaignService)
			err := svc.SetMemberAccess(context.Background(), "c1", "t", tc.access)
			if tc.wantCode != 0 {
				assertAppError(t, err, tc.wantCode)
				if len(roleWrites) > 0 || written != nil {
					t.Fatalf("refused call still wrote: roles %v settings %v", roleWrites, written)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantRole == nil && len(roleWrites) != 0 {
				t.Fatalf("unexpected role writes %v", roleWrites)
			}
			if tc.wantRole != nil && (len(roleWrites) != 1 || roleWrites[0] != *tc.wantRole) {
				t.Fatalf("role writes %v, want [%v]", roleWrites, *tc.wantRole)
			}
			if (written != nil) != tc.wantWrite {
				t.Fatalf("settings written = %v, want %v", written != nil, tc.wantWrite)
			}
			if written != nil && !sameIDs(written.DmGrantIDs, tc.wantGrants) {
				t.Fatalf("grants %v, want %v", written.DmGrantIDs, tc.wantGrants)
			}
		})
	}
}

func ptrRole(r Role) *Role { return &r }

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
