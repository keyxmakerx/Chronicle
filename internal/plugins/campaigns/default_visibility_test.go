// default_visibility_test.go pins the shared resolution table for
// CampaignSettings.DefaultVisibility, used by every entity-creation path
// (web form, shop quick-create, REST POST /entities, batch-sync, bestiary
// import) so a campaign's default visibility is honoured everywhere.
//
// The three-state parameter is load-bearing: collapsing "absent" into
// "false" would silently ignore a private default, and collapsing "explicit
// false" into "absent" would override a client's deliberate is_private:false.
package campaigns

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

func TestCampaignSettings_DefaultsToPrivate(t *testing.T) {
	cases := []struct {
		name string
		vis  string
		want bool
	}{
		{"unset means everyone", "", false},
		{"dm_only starts hidden", "dm_only", true},
		{"private starts hidden", "private", true},
		{"explicit public value is not private", "public", false},
		{"unknown value is not private", "sideways", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := CampaignSettings{DefaultVisibility: tc.vis}
			if got := s.DefaultsToPrivate(); got != tc.want {
				t.Errorf("DefaultsToPrivate() = %v, want %v (DefaultVisibility=%q)", got, tc.want, tc.vis)
			}
		})
	}
}

func TestCampaignSettings_ResolveNewEntityPrivacy(t *testing.T) {
	cases := []struct {
		name      string
		vis       string
		requested patch.Field[bool]
		want      bool
	}{
		{
			name:      "absent under a dm_only default starts private",
			vis:       "dm_only",
			requested: patch.Absent[bool](),
			want:      true,
		},
		{
			name:      "absent under a private default starts private",
			vis:       "private",
			requested: patch.Absent[bool](),
			want:      true,
		},
		{
			name:      "absent under no default stays public",
			vis:       "",
			requested: patch.Absent[bool](),
			want:      false,
		},
		{
			// The distinction the whole three-state type exists for: a
			// client that deliberately says "public" is obeyed, and is
			// NOT quietly upgraded by the campaign default.
			name:      "explicit false under a dm_only default stays public",
			vis:       "dm_only",
			requested: patch.Of(false),
			want:      false,
		},
		{
			name:      "explicit true under no default is private",
			vis:       "",
			requested: patch.Of(true),
			want:      true,
		},
		{
			name:      "explicit true under a dm_only default is private",
			vis:       "dm_only",
			requested: patch.Of(true),
			want:      true,
		},
		{
			// Create has nothing stored to clear, so null has no "clear"
			// reading; it falls through to the campaign default rather
			// than silently meaning public.
			name:      "explicit null falls through to the campaign default",
			vis:       "dm_only",
			requested: patch.Null[bool](),
			want:      true,
		},
		{
			name:      "explicit null with no default is public",
			vis:       "",
			requested: patch.Null[bool](),
			want:      false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := CampaignSettings{DefaultVisibility: tc.vis}
			if got := s.ResolveNewEntityPrivacy(tc.requested); got != tc.want {
				t.Errorf("ResolveNewEntityPrivacy() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestNewPagePrivacy pins that a Scribe's new page is always open to players,
// whatever they asked for or the campaign defaults to, while the owner and
// Co-DMs keep the shared resolution.
func TestNewPagePrivacy(t *testing.T) {
	hiddenByDefault := &Campaign{Settings: `{"default_visibility":"dm_only"}`}
	cases := []struct {
		name      string
		cc        *CampaignContext
		requested patch.Field[bool]
		want      bool
	}{
		{"scribe asking for hidden", &CampaignContext{Campaign: hiddenByDefault, MemberRole: RoleScribe}, patch.Of(true), false},
		{"scribe under a hidden default", &CampaignContext{Campaign: hiddenByDefault, MemberRole: RoleScribe}, patch.Absent[bool](), false},
		{"co-dm asking for hidden", &CampaignContext{Campaign: hiddenByDefault, MemberRole: RoleScribe, IsDmGranted: true}, patch.Of(true), true},
		{"owner under a hidden default", &CampaignContext{Campaign: hiddenByDefault, MemberRole: RoleOwner}, patch.Absent[bool](), true},
		{"owner asking for open", &CampaignContext{Campaign: hiddenByDefault, MemberRole: RoleOwner}, patch.Of(false), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cc.NewPagePrivacy(tc.requested); got != tc.want {
				t.Errorf("NewPagePrivacy() = %v, want %v", got, tc.want)
			}
		})
	}
}
