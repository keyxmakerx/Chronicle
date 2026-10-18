package campaigns

import "testing"

// TestRedactMemberEmails pins who sees an address in the member roster JSON:
// the owner sees all, anyone else only their own.
func TestRedactMemberEmails(t *testing.T) {
	tests := []struct {
		name    string
		viewer  string
		isOwner bool
		want    map[string]string
	}{
		{"owner sees every address", "u1", true, map[string]string{"u1": "a@x", "u2": "b@x", "u3": "c@x"}},
		{"player sees only their own", "u2", false, map[string]string{"u1": "", "u2": "b@x", "u3": ""}},
		{"guest-code joiner sees none of the others", "u3", false, map[string]string{"u1": "", "u2": "", "u3": "c@x"}},
		{"no viewer id sees none", "", false, map[string]string{"u1": "", "u2": "", "u3": ""}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			members := []CampaignMember{
				{UserID: "u1", Email: "a@x", Role: RoleOwner},
				{UserID: "u2", Email: "b@x", Role: RolePlayer},
				{UserID: "u3", Email: "c@x", Role: RolePlayer},
			}
			role := RolePlayer
			if tc.isOwner {
				role = RoleOwner
			}
			redactMemberEmails(members, role, tc.viewer)
			for _, m := range members {
				if m.Email != tc.want[m.UserID] {
					t.Errorf("%s: email = %q, want %q", m.UserID, m.Email, tc.want[m.UserID])
				}
			}
		})
	}
}
