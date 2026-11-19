package campaigns

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestInviteBoxListsEverythingWaiting(t *testing.T) {
	now := time.Now()
	until := now.Add(48 * time.Hour)
	used := now.Add(-time.Hour)
	d := InviteBoxData{
		CampaignID: "c1", CSRF: "tok", Now: now, JoinURL: "https://x.test/join", Mode: "email", EmailReady: true,
		LinkURL: "https://x.test/join/abc",
		Invites: []Invite{{ID: "i1", Email: "nora@example.com", Role: "scribe", ExpiresAt: now.Add(6 * 24 * time.Hour)}},
		Codes: []GuestCode{
			{ID: "g1", Note: "Sam", ExpiresAt: now.Add(5 * 24 * time.Hour), CreatedAt: now},
			{ID: "g2", UsedBy: strPtr("u"), UsedAt: &used, UsedByName: "Visitor", VisitDays: 3, VisitUntil: &until, ExpiresAt: now.Add(time.Hour)},
		},
	}
	var b bytes.Buffer
	if err := InviteBox(d).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"Invite someone", "By email", "With a link", "With a code",
		"Waiting to be used", "nora@example.com", "Invite link",
		`hx-delete="/campaigns/c1/invite-box/invites/i1"`,
		`hx-delete="/campaigns/c1/invite-box/link"`,
		`hx-delete="/campaigns/c1/invite-box/codes/g1"`,
		`hx-post="/campaigns/c1/invite-box/codes/g2/player"`,
		`hx-post="/campaigns/c1/invite-box/codes/g2/end"`,
		"Join as a player", "Look around",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("box lacks %q", want)
		}
	}
	// An open code has no Make player; only a visitor does.
	if strings.Contains(out, "codes/g1/player") {
		t.Error("an unused code offers Make player")
	}
}

