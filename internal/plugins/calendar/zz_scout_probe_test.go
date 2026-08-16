package calendar

// SCOUTING PROBE — not a fix, not a guard. It asserts the CURRENT behaviour so
// the finding can point at a run rather than at an argument.
//
// ONE PROBE IS LEFT HERE ON PURPOSE. Its defect is DELIBERATELY NOT FIXED:
// removing an RSVP note needs a control that does not exist (SuggestTime refuses
// an empty note by design, so there is no write path that could clear the
// column) plus a product decision about whether answering "Going" should clear a
// note left with an earlier "maybe". That is feature work, not a defect fix, and
// a half-fix that let an empty note through SuggestTime would break the
// "add a time OR a note" validation that stops empty submissions.
//
// Every other probe in this file has been deleted: its defect is fixed and a
// real guard now stands in its place (rsvp_week_zone_test.go,
// rsvp_suggest_zone_test.go, rsvp_collection_off_test.go,
// rsvp_collect_floor_test.go).

import (
	"context"
	"testing"
)

// PROBE 6 — an RSVP note, once written, can never be removed by the member.
//
// normalizeRSVPNote (rsvp_service.go:335) maps "" and "   " to nil, and
// UpsertRSVP's ON DUPLICATE KEY does note = COALESCE(VALUES(note), note)
// (rsvp_repository.go:90), so a nil note PRESERVES the stored one. SuggestTime
// refuses an empty note outright. There is no other write path to the column
// and no DELETE. The Owner's Responders[].Note keeps showing it forever.
func TestProbe_RSVPNoteCannotBeCleared(t *testing.T) {
	var wroteNote *string
	var noteWrites int
	repo := &mockRSVPRepo{
		upsertFn: func(_ context.Context, r *EventRSVP) error { wroteNote = r.Note; return nil },
		setNoteFn: func(_ context.Context, _, _, note string) error {
			noteWrites++
			return nil
		},
	}
	evt := testEvent(nil)
	svc := newTestRSVPService(repo, evt, testCalendar(nil))

	// The member tries to clear it by re-answering with an empty note.
	if err := svc.SetMyRSVP(context.Background(), evt, "u1", 1,
		SetRSVPRequest{Status: RSVPYes, Note: probeStr("")}); err != nil {
		t.Fatalf("SetMyRSVP: %v", err)
	}
	if wroteNote != nil {
		t.Fatalf("probe stale: an empty note now reaches the repository as %q", *wroteNote)
	}

	// ...and by whitespace.
	wroteNote = probeStr("sentinel")
	if err := svc.SetMyRSVP(context.Background(), evt, "u1", 1,
		SetRSVPRequest{Status: RSVPYes, Note: probeStr("   ")}); err != nil {
		t.Fatalf("SetMyRSVP: %v", err)
	}
	if wroteNote != nil {
		t.Fatalf("probe stale: whitespace now reaches the repository")
	}

	// ...and through the suggestion path, which refuses empty outright.
	err := svc.SuggestTime(context.Background(), evt, "u1", 1, "")
	if err == nil {
		t.Fatal("probe stale: SuggestTime now accepts an empty note")
	}
	if noteWrites != 0 {
		t.Fatalf("probe stale: %d note writes reached the repo", noteWrites)
	}
	t.Logf("CONFIRMED: every clearing attempt is a no-op (SuggestTime refuses: %v); "+
		"COALESCE(VALUES(note), note) then keeps the old text on the Owner's list forever", err)
}

func probeStr(s string) *string { return &s }
