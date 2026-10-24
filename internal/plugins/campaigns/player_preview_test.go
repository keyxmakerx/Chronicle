package campaigns

import "testing"

// PlayerPreview draws as a player without touching the context it came from,
// which still decides access for the rest of the request.
func TestCampaignContext_PlayerPreview(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "c1"}, MemberRole: RoleOwner, IsDmGranted: true, IsSiteAdmin: true, IsMember: true}
	pv := cc.PlayerPreview()
	if pv.MemberRole != RolePlayer || pv.IsDmGranted || pv.IsSiteAdmin {
		t.Errorf("preview = role %d, dm %v, admin %v; want a plain player", pv.MemberRole, pv.IsDmGranted, pv.IsSiteAdmin)
	}
	if !pv.IsMember || pv.Campaign != cc.Campaign {
		t.Error("preview should keep membership and the campaign")
	}
	if cc.MemberRole != RoleOwner || !cc.IsDmGranted || !cc.IsSiteAdmin {
		t.Error("PlayerPreview changed the original context")
	}
}
