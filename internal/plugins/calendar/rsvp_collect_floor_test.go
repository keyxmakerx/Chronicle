package calendar

// Arming "Collect RSVPs" must report whether mail is going out FOR THIS CALL,
// not whether a mail server exists.
//
// THE DEFECT: the response computed `emailed: fannedOut && mailer.IsConfigured`
// and never consulted the 24h per-recipient floor the fan-out applies. The floor
// reads the SAME calendar_schedule_asks log the "Ask availability" control
// writes, so the two features silently suppress each other: a Director who
// clicks "Ask availability" in the morning (mails the whole roster, writes a row
// per recipient) and arms Collect RSVPs that afternoon sends ZERO emails — while
// the response says collect_rsvps:true, emailed:true and carries no notice at
// all (mailNotConfiguredLine only covers the no-SMTP branch). The operator reads
// "the party has been invited" and stops checking. That is verbatim the failure
// [GR-10] was written to close, reopened one file over by [GR-6].

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// floorRepo returns a repo whose per-recipient floor already covers the given
// user ids — i.e. they were emailed inside the last 24 hours.
func floorRepo(inFloor ...string) *mockRSVPRepo {
	ids := append([]string(nil), inFloor...)
	return &mockRSVPRepo{recentAskFn: func(_ context.Context, _ string, _ time.Time) ([]string, error) {
		return ids, nil
	}}
}

// floorRoster is a two-player roster with addresses.
func floorRoster() *mockMemberDir {
	return &mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "player-1", Email: "p1@example.test", Role: campaigns.RolePlayer},
		{UserID: "player-2", Email: "p2@example.test", Role: campaigns.RolePlayer},
	}}
}

// TestCollectRSVPs_DoesNotClaimEmailWhenTheFloorSkipsEveryone is the headline.
func TestCollectRSVPs_DoesNotClaimEmailWhenTheFloorSkipsEveryone(t *testing.T) {
	evt := testEvent(func(e *Event) { e.CollectRSVPs = false })
	repo := floorRepo("player-1", "player-2")
	h := NewRSVPHandler(newTestRSVPService(repo, evt, testCalendar(nil)))
	h.SetMemberDirectory(floorRoster())
	mailer := &mockMailer{configured: true}
	h.SetMailSender(mailer, "https://chronicle.example.test")

	body := serveCollectionPUT(t, h, `{"enabled":true}`)

	if got, _ := body["emailed"].(bool); got {
		t.Error("emailed = true while every recipient is inside the 24h per-recipient " +
			"floor — the operator is told the party was invited and nobody was")
	}
	notice, ok := body["notice"].(string)
	if !ok {
		t.Fatalf("no notice explaining why nobody was emailed; body = %+v", body)
	}
	if !strings.Contains(notice, "24 hours") {
		t.Errorf("the notice does not name the reason (the 24h limit), so the operator's "+
			"next move is to go and check the mail server: %q", notice)
	}
	// The arming itself must still succeed — in-app answering works regardless.
	if got, _ := body["collect_rsvps"].(bool); !got {
		t.Error("the arming was refused; the fix is to stop CLAIMING what did not happen, " +
			"not to stop doing what did")
	}
}

// TestCollectRSVPs_ClaimsEmailWhenSomebodyIsActuallyMailed is the other half. A
// "fix" that always answers false would satisfy the test above.
func TestCollectRSVPs_ClaimsEmailWhenSomebodyIsActuallyMailed(t *testing.T) {
	evt := testEvent(func(e *Event) { e.CollectRSVPs = false })
	// Only one of the two was mailed recently.
	repo := floorRepo("player-1")
	h := NewRSVPHandler(newTestRSVPService(repo, evt, testCalendar(nil)))
	h.SetMemberDirectory(floorRoster())
	h.SetMailSender(&mockMailer{configured: true}, "https://chronicle.example.test")

	body := serveCollectionPUT(t, h, `{"enabled":true}`)
	if got, _ := body["emailed"].(bool); !got {
		t.Error("emailed = false while player-2 is outside the floor and will be mailed")
	}
	if _, ok := body["notice"]; ok {
		t.Errorf("a notice was attached to a send that IS happening; body = %+v", body)
	}
}

// TestCollectRSVPs_NoAddressesIsItsOwnReason keeps the notices diagnostic rather
// than generic: "nobody has an email address" and "everyone was mailed today"
// send the operator to two different places.
func TestCollectRSVPs_NoAddressesIsItsOwnReason(t *testing.T) {
	evt := testEvent(func(e *Event) { e.CollectRSVPs = false })
	h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, evt, testCalendar(nil)))
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "player-1", Role: campaigns.RolePlayer}, // no address
	}})
	h.SetMailSender(&mockMailer{configured: true}, "https://chronicle.example.test")

	body := serveCollectionPUT(t, h, `{"enabled":true}`)
	if got, _ := body["emailed"].(bool); got {
		t.Error("emailed = true with no addressable recipient")
	}
	notice, _ := body["notice"].(string)
	if !strings.Contains(notice, "email address") {
		t.Errorf("notice = %q, want it to name the missing addresses", notice)
	}
	if strings.Contains(notice, "24 hours") {
		t.Errorf("notice blames the rate limit for a missing-address problem: %q", notice)
	}
}

// TestInvitePlan_MatchesWhatTheFanOutSends is the anti-drift pin. The plan and
// the fan-out are two readings of one rule, which is exactly the shape that
// drifts, so they are measured against each other rather than trusted.
func TestInvitePlan_MatchesWhatTheFanOutSends(t *testing.T) {
	for _, tc := range []struct {
		name    string
		roster  []campaigns.CampaignMember
		skipped []string
	}{
		{"nobody skipped", floorRoster().members, nil},
		{"one skipped", floorRoster().members, []string{"player-1"}},
		{"all skipped", floorRoster().members, []string{"player-1", "player-2"}},
		{"one has no address", []campaigns.CampaignMember{
			{UserID: "player-1", Email: "p1@example.test", Role: campaigns.RolePlayer},
			{UserID: "player-2", Role: campaigns.RolePlayer},
		}, nil},
		{"empty roster", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evt := testEvent(func(e *Event) { e.CollectRSVPs = true })
			cal := testCalendar(nil)
			repo := floorRepo(tc.skipped...)
			h := NewRSVPHandler(newTestRSVPService(repo, evt, cal))
			h.SetMemberDirectory(&mockMemberDir{members: tc.roster})
			mailer := &mockMailer{configured: true}
			h.SetMailSender(mailer, "https://chronicle.example.test")

			ctx := context.Background()
			plan := h.planInviteFanOut(ctx, "camp-1", evt, true)
			h.fanOutInvites(ctx, "camp-1", "Vale of Ash", cal, evt, "gm-1")

			// Give the (synchronous here) send a beat in case a future change
			// backgrounds part of it.
			time.Sleep(10 * time.Millisecond)
			mailer.mu.Lock()
			sent := len(mailer.sent)
			mailer.mu.Unlock()

			if plan.WillEmail != sent {
				t.Errorf("the plan says %d will be emailed; the fan-out sent %d — the report "+
					"and the send have drifted apart", plan.WillEmail, sent)
			}
		})
	}
}
