// rsvp_ask_log_test.go — the persisted halves of the schedule-ask rate limit
// (C-CALV4-RSVP-P8B stage 2, ruling [PB-4]).
//
// Two limits, two questions, one table:
//
//	per CAMPAIGN, 6h    refuses the whole send, and the refusal is a SENTENCE
//	                    the Bench prints, not a bare 429.
//	per RECIPIENT, 24h  skips that member and lets the rest of the send
//	                    proceed, so a second ask after somebody joins mails the
//	                    new member and nobody else.
//
// The third layer (per-actor, 10/hour, in memory) is not persisted and is
// tested at the route.
package calendar

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// errTestDB stands in for a repository/database failure.
var errTestDB = errors.New("db unavailable")

// TestScheduleAskState_CampaignCooldown is the campaign-level readout: never
// asked is askable; inside the window reports how long is left, to the whole
// hour the Bench prints; outside it is askable again.
func TestScheduleAskState_CampaignCooldown(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name string
		// last is the most recent send; the zero value means "never asked".
		last      time.Time
		wantReady bool
		// wantRetryHours is the remaining wait ROUNDED UP to whole hours —
		// the number the Bench prints, because a limit must never promise to
		// lift sooner than it does.
		wantRetryHours int
	}{
		{name: "never asked is askable", last: time.Time{}, wantReady: true},
		{name: "asked two hours ago waits four more", last: now.Add(-2 * time.Hour), wantRetryHours: 4},
		{name: "asked one minute ago waits nearly six", last: now.Add(-time.Minute), wantRetryHours: 6},
		{name: "asked six hours ago is askable again", last: now.Add(-6 * time.Hour), wantReady: true},
		{name: "asked a week ago is askable", last: now.Add(-7 * 24 * time.Hour), wantReady: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRSVPRepo{lastAskFn: func(context.Context, string) (time.Time, error) {
				return tt.last, nil
			}}
			svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))
			st, err := svc.ScheduleAskState(context.Background(), "camp-1")
			if err != nil {
				t.Fatalf("ScheduleAskState: %v", err)
			}
			if st.Ready != tt.wantReady {
				t.Fatalf("Ready = %v, want %v (retry %v)", st.Ready, tt.wantReady, st.RetryAfter)
			}
			if tt.wantReady {
				if !st.LastAskedAt.Equal(tt.last) {
					t.Errorf("LastAskedAt = %v, want %v", st.LastAskedAt, tt.last)
				}
				if st.RetryAfter != 0 {
					t.Errorf("an askable campaign must report no wait, got %v", st.RetryAfter)
				}
				return
			}
			if got := int(math.Ceil(st.RetryAfter.Hours())); got != tt.wantRetryHours {
				t.Errorf("RetryAfter = %v (%d hours rounded up), want %d", st.RetryAfter, got, tt.wantRetryHours)
			}
		})
	}
}

// TestScheduleAskState_ReadFailureRefusesTheSend pins the fail-closed direction.
// If the cooldown cannot be read we do not know whether this roster was mailed
// twenty minutes ago, and an unretractable email is the wrong thing to guess
// about.
func TestScheduleAskState_ReadFailureRefusesTheSend(t *testing.T) {
	repo := &mockRSVPRepo{lastAskFn: func(context.Context, string) (time.Time, error) {
		return time.Time{}, errTestDB
	}}
	svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))
	if _, err := svc.ScheduleAskState(context.Background(), "camp-1"); err == nil {
		t.Fatal("a cooldown read failure must refuse the send, not fall through to askable")
	}
}

// TestRecentlyAskedRecipients_FloorSkipsNotRefuses proves the per-recipient
// floor is a SKIP: the member asked three hours ago is in the returned set, the
// member asked thirty hours ago is not, and a member who has never been asked
// is not — so a newly joined member is always reached.
func TestRecentlyAskedRecipients_FloorSkipsNotRefuses(t *testing.T) {
	var gotSince time.Time
	repo := &mockRSVPRepo{recentAskFn: func(_ context.Context, campaignID string, since time.Time) ([]string, error) {
		if campaignID != "camp-1" {
			t.Errorf("floor read scoped to %q, want camp-1", campaignID)
		}
		gotSince = since
		// The repository applies the window; the fake returns what it would.
		return []string{"asked-3h-ago"}, nil
	}}
	svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))

	skip, err := svc.RecentlyAskedRecipients(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("RecentlyAskedRecipients: %v", err)
	}
	if !skip["asked-3h-ago"] {
		t.Error("a member emailed inside the 24h floor must be skipped")
	}
	if skip["never-asked"] || skip["asked-30h-ago"] {
		t.Error("only members inside the floor are skipped — a newly joined member must be reached")
	}
	// The window handed to the repository is the signed 24 hours, measured back
	// from now rather than from the campaign's last send.
	if elapsed := time.Since(gotSince); elapsed < 23*time.Hour || elapsed > 25*time.Hour {
		t.Errorf("the floor window started %v ago, want ~24h", elapsed)
	}
}

// TestRecordScheduleAsk_OneRowPerRecipientActuallySent pins the write: the row
// carries who was mailed, who asked, and which session (if any) rode along, and
// it is stamped at write time — the goroutine writes it AS each send succeeds,
// never optimistically up front.
func TestRecordScheduleAsk_OneRowPerRecipientActuallySent(t *testing.T) {
	var rows []ScheduleAsk
	repo := &mockRSVPRepo{recordAskFn: func(_ context.Context, a *ScheduleAsk) error {
		rows = append(rows, *a)
		return nil
	}}
	svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))

	before := time.Now().UTC().Add(-time.Second)
	if err := svc.RecordScheduleAsk(context.Background(), "camp-1", "evt-1", "u1", "gm-1"); err != nil {
		t.Fatalf("RecordScheduleAsk: %v", err)
	}
	// A send with no session attached records the ask with no event.
	if err := svc.RecordScheduleAsk(context.Background(), "camp-1", "", "u2", "gm-1"); err != nil {
		t.Fatalf("RecordScheduleAsk (no event): %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("wrote %d rows, want one per recipient", len(rows))
	}
	if rows[0].ID == "" || rows[0].ID == rows[1].ID {
		t.Error("each ask row needs its own id")
	}
	if rows[0].EventID == nil || *rows[0].EventID != "evt-1" {
		t.Errorf("row 0 EventID = %v, want evt-1", rows[0].EventID)
	}
	if rows[1].EventID != nil {
		t.Errorf("a send with no session must record no event, got %v", rows[1].EventID)
	}
	for i, r := range rows {
		if r.CampaignID != "camp-1" || r.ActorUserID != "gm-1" {
			t.Errorf("row %d scoped wrong: %+v", i, r)
		}
		if r.SentAt.Before(before) || r.SentAt.After(time.Now().UTC().Add(time.Second)) {
			t.Errorf("row %d SentAt = %v, want stamped at write time", i, r.SentAt)
		}
	}
}

// TestRecordScheduleAsk_RefusesAnIncompleteRow is the "nothing is recorded when
// nothing was sent" invariant seen from below: the service will not write a row
// that cannot identify the campaign or the recipient it claims to be about.
func TestRecordScheduleAsk_RefusesAnIncompleteRow(t *testing.T) {
	wrote := false
	repo := &mockRSVPRepo{recordAskFn: func(context.Context, *ScheduleAsk) error {
		wrote = true
		return nil
	}}
	svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))
	for _, tc := range [][3]string{{"", "u1", "gm-1"}, {"camp-1", "", "gm-1"}} {
		if err := svc.RecordScheduleAsk(context.Background(), tc[0], "", tc[1], tc[2]); err == nil {
			t.Errorf("an incomplete ask row (campaign=%q recipient=%q) must be refused", tc[0], tc[1])
		}
	}
	if wrote {
		t.Error("an incomplete ask row reached the repository")
	}
}
