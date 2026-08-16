package calendar

// Two smaller RSVP honesty rules that both come down to "do not report a state
// the system is not in".
//
//  1. A PARTIAL "out this week" must say which days it managed. There is no
//     transaction spanning the week — the writer loops one day-replace per date,
//     any of which can fail on the per-user exception cap or a transient DB
//     error — so a failure on day 4 of 7 leaves days 1-3 committed. Reporting
//     "your calendar availability could not be updated" at a member who is
//     half-blocked, naming no days, leaves them believing they are free on days
//     the Director's overlay now shows as unavailable. The RSVP itself is
//     already recorded, so the two halves disagree.
//
//  2. Arming Collect RSVPs must fan out on the WRITE's answer, not on a
//     separately-read event row. Two Scribes arming at the same moment both read
//     collect_rsvps=0, both believed they had caused the transition, and both
//     started an invite fan-out — the 24h per-recipient floor absorbs nothing in
//     a tight race, because its rows are written only after each send returns.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestApplyOutThisWeek_PartialWriteNamesTheDaysItManaged(t *testing.T) {
	restore := nowUTC
	nowUTC = func() time.Time { return time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC) }
	defer func() { nowUTC = restore }()

	// Day 4 of 7 fails (the per-user exception cap).
	avail := &mockAvailability{zone: "Europe/London", failAfter: 3}
	h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, testEvent(nil), testCalendar(nil)))
	h.SetAvailabilityWriter(avail)

	msg := h.applyOutThisWeek(context.Background(), testCalendar(nil), testEvent(nil), "player-9")

	if strings.Contains(msg, "could not be updated — please set it by hand") {
		t.Fatalf("a member with 3 of 7 days already committed was told nothing happened: %q", msg)
	}
	for _, day := range []string{"2026-08-17", "2026-08-18", "2026-08-19"} {
		if !strings.Contains(msg, day) {
			t.Errorf("the message does not name the committed day %s: %q", day, msg)
		}
	}
	if !strings.Contains(msg, "by hand") {
		t.Errorf("the message does not tell them the rest needs doing by hand: %q", msg)
	}
}

// TestApplyOutThisWeek_TotalFailureStillSaysSo keeps the original message for
// the case it was written for — nothing committed at all.
func TestApplyOutThisWeek_TotalFailureStillSaysSo(t *testing.T) {
	restore := nowUTC
	nowUTC = func() time.Time { return time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC) }
	defer func() { nowUTC = restore }()

	avail := &mockAvailability{zone: "Europe/London", failAfter: 0, failAll: true}
	h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, testEvent(nil), testCalendar(nil)))
	h.SetAvailabilityWriter(avail)

	msg := h.applyOutThisWeek(context.Background(), testCalendar(nil), testEvent(nil), "player-9")
	if !strings.Contains(msg, "could not be updated") {
		t.Errorf("a total failure no longer says so: %q", msg)
	}
}

// TestCollectRSVPs_LoserOfAConcurrentArmingDoesNotFanOut pins that the fan-out
// rides the write's own answer.
func TestCollectRSVPs_LoserOfAConcurrentArmingDoesNotFanOut(t *testing.T) {
	// The event row this request read still says "not collecting" — but by the
	// time the UPDATE lands, the other Scribe has already flipped it, so the
	// conditional write changes nothing.
	evt := testEvent(func(e *Event) { e.CollectRSVPs = false })
	repo := &mockRSVPRepo{collectAlreadySet: true}
	h := NewRSVPHandler(newTestRSVPService(repo, evt, testCalendar(nil)))
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "player-1", Email: "p1@example.test", Role: campaigns.RolePlayer},
	}})
	mailer := &mockMailer{configured: true}
	h.SetMailSender(mailer, "https://chronicle.example.test")

	body := serveCollectionPUT(t, h, `{"enabled":true}`)

	if got, _ := body["emailed"].(bool); got {
		t.Error("the losing arming claimed it emailed the roster; the winner already did, " +
			"and a second fan-out double-mails everybody")
	}
	if got, _ := body["collect_rsvps"].(bool); !got {
		t.Error("the losing arming reported the wrong state; collection IS on")
	}
}

// TestCollectRSVPs_WinnerOfAnArmingStillFansOut is the other half — a fix that
// never fans out would satisfy the test above.
func TestCollectRSVPs_WinnerOfAnArmingStillFansOut(t *testing.T) {
	evt := testEvent(func(e *Event) { e.CollectRSVPs = false })
	h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, evt, testCalendar(nil)))
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "player-1", Email: "p1@example.test", Role: campaigns.RolePlayer},
	}})
	h.SetMailSender(&mockMailer{configured: true}, "https://chronicle.example.test")

	body := serveCollectionPUT(t, h, `{"enabled":true}`)
	if got, _ := body["emailed"].(bool); !got {
		t.Errorf("the arming that DID flip the column did not fan out; body=%+v", body)
	}
}
