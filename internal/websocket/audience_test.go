package websocket

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// TestRequiresDM_AudienceGate locks in the broadcast gate's behavior:
// a Message with RequiresDM=true is delivered only to clients that pass
// permissions.CanSeeDmOnly(role, isDmGranted) — i.e. campaign Owners
// and DM-granted non-Owners. Everyone else is silently dropped.
//
// We test the predicate directly rather than spinning up a real Hub
// because the broadcast loop also tangles I/O (json.Marshal, channel
// sends) that aren't relevant to the security contract. The predicate
// match is the security gate; this test is the regression guard
// against accidentally inverting it.
func TestRequiresDM_AudienceGate(t *testing.T) {
	cases := []struct {
		name        string
		role        int
		isDmGranted bool
		requiresDM  bool
		wantDeliver bool
	}{
		{
			name:        "everyone msg → player receives",
			role:        permissions.RolePlayer,
			requiresDM:  false,
			wantDeliver: true,
		},
		{
			name:        "DM-only msg → player blocked",
			role:        permissions.RolePlayer,
			requiresDM:  true,
			wantDeliver: false,
		},
		{
			name:        "DM-only msg → owner receives",
			role:        permissions.RoleOwner,
			requiresDM:  true,
			wantDeliver: true,
		},
		{
			name:        "DM-only msg → DM-granted player receives",
			role:        permissions.RolePlayer,
			isDmGranted: true,
			requiresDM:  true,
			wantDeliver: true,
		},
		{
			name:        "DM-only msg → scribe blocked unless dm-granted",
			role:        permissions.RoleScribe,
			requiresDM:  true,
			wantDeliver: false,
		},
		{
			name:        "DM-only msg → scribe with grant receives",
			role:        permissions.RoleScribe,
			isDmGranted: true,
			requiresDM:  true,
			wantDeliver: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// This mirrors the inline gate in hub.go's broadcast loop.
			// If the broadcast gate logic ever changes, both this test
			// and hub.go must change in lockstep — that's the point.
			delivered := !tc.requiresDM || permissions.CanSeeDmOnly(tc.role, tc.isDmGranted)
			if delivered != tc.wantDeliver {
				t.Errorf("delivered=%v, want %v", delivered, tc.wantDeliver)
			}
		})
	}
}

// TestMessage_RequiresDMJSONOmitted ensures the new field is omitted
// from JSON when false so existing consumers don't see a noisy new
// key on every message.
func TestMessage_RequiresDMJSONOmitted(t *testing.T) {
	msg := NewMessage(MsgEntityCreated, "camp-1", "ent-1", nil)
	data, err := msg.Encode()
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if got := string(data); got != `{"type":"entity.created","campaignId":"camp-1","resourceId":"ent-1"}` {
		t.Errorf("RequiresDM=false should omit the field; got %s", got)
	}
}
