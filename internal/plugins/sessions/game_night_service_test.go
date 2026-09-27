package sessions

// Tests for the two most novel-and-risky pieces of the game-night RSVP build
// (issue #741 Part C): that a repeating game night keeps one answer PER
// NIGHT rather than one shared answer for the whole series, and that the
// "suggest another time" email link validates the suggestion BEFORE
// consuming its single-use token (the historical bug: a bad submission burned
// the link with nothing to show for it).

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// --- per-occurrence RSVP independence -------------------------------------

// TestUpdateRSVPDetailed_OccurrencesAreIndependent pins that answering for one
// night of a recurring series writes to THAT night's row only — a mock repo
// asserting each call's occurrence_date key stays exact is the same style
// this package already uses for its RSVP-token ordering guarantees
// (rsvp_token_guard_test.go), and it exercises the same service code a real
// database would.
func TestUpdateRSVPDetailed_OccurrencesAreIndependent(t *testing.T) {
	written := map[string]string{} // occurrenceDate -> status
	notes := map[string]*string{}  // occurrenceDate -> note
	rt := RecurrenceWeekly
	base := "2026-01-06"
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, IsRecurring: true, RecurrenceType: &rt, ScheduledDate: &base}, nil
		},
		upsertOccurrenceRSVPFn: func(_ context.Context, _, _, occurrenceDate, status string, note *string) error {
			written[occurrenceDate] = status
			notes[occurrenceDate] = note
			return nil
		},
	}
	svc := NewSessionService(repo, nil, nil)

	night1 := "2026-01-06"
	night2 := "2026-01-13" // the following week's occurrence
	if err := svc.UpdateRSVPDetailed(context.Background(), "s1", "u1", RSVPAccepted, &night1, patch.Absent[string]()); err != nil {
		t.Fatalf("answering night 1: %v", err)
	}
	if err := svc.UpdateRSVPDetailed(context.Background(), "s1", "u1", RSVPDeclined, &night2, patch.Of("can't make it")); err != nil {
		t.Fatalf("answering night 2: %v", err)
	}

	if written[night1] != RSVPAccepted {
		t.Errorf("night 1 status = %q, want %q", written[night1], RSVPAccepted)
	}
	if written[night2] != RSVPDeclined {
		t.Errorf("night 2 status = %q, want %q — answering night 2 must not touch night 1's row", written[night2], RSVPDeclined)
	}
	if notes[night1] != nil {
		t.Errorf("night 1 note = %v, want nil — night 2's note must not leak onto night 1", notes[night1])
	}
	if notes[night2] == nil || *notes[night2] != "can't make it" {
		t.Errorf("night 2 note = %v, want %q", notes[night2], "can't make it")
	}
	if len(written) != 2 {
		t.Fatalf("expected exactly 2 distinct occurrence rows written, got %d: %v", len(written), written)
	}
}

// TestUpdateRSVPDetailed_NonRecurringUsesAttendeesTable pins that a
// NON-recurring session's answer still goes through the exact old path
// (session_attendees, via UpdateAttendeeStatus) — this build is additive, so
// UpdateRSVPDetailed must never route a plain one-off session into the new
// per-occurrence table.
func TestUpdateRSVPDetailed_NonRecurringUsesAttendeesTable(t *testing.T) {
	var attendeeCalled, occurrenceCalled bool
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, IsRecurring: false}, nil
		},
		updateAttendeeStatusFn: func(_ context.Context, _, _, _ string) error {
			attendeeCalled = true
			return nil
		},
		upsertOccurrenceRSVPFn: func(_ context.Context, _, _, _, _ string, _ *string) error {
			occurrenceCalled = true
			return nil
		},
	}
	svc := NewSessionService(repo, nil, nil)
	if err := svc.UpdateRSVPDetailed(context.Background(), "s1", "u1", RSVPAccepted, nil, patch.Absent[string]()); err != nil {
		t.Fatalf("UpdateRSVPDetailed: %v", err)
	}
	if !attendeeCalled {
		t.Error("a non-recurring session's answer must still write to session_attendees")
	}
	if occurrenceCalled {
		t.Error("a non-recurring session's answer must never write to session_occurrence_rsvps")
	}
}

// --- "suggest another time": validate before consume ----------------------

// TestValidateAndRecordSuggestion_ValidatesBeforeConsuming mirrors
// rsvp_token_guard_test.go's TestRSVPToken_ConsumesBeforeItApplies — a
// deterministic call-order assertion — but for the OPPOSITE historical bug:
// here validation must happen BEFORE the token is consumed, not after.
func TestValidateAndRecordSuggestion_ValidatesBeforeConsuming(t *testing.T) {
	var order []string
	future := time.Now().UTC().Add(time.Hour)
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{Token: "st", SessionID: "s1", UserID: "u1", Action: RSVPActionSuggest, ExpiresAt: future}, nil
		},
		markRSVPTokenUsedFn: func(_ context.Context, _ string) error {
			order = append(order, "consume")
			return nil
		},
		createRescheduleSuggestionFn: func(_ context.Context, _ *RescheduleSuggestion) error {
			order = append(order, "record")
			return nil
		},
	}
	svc := NewSessionService(repo, nil, nil)

	validDate := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
	if _, err := svc.ValidateAndRecordSuggestion(context.Background(), "st", validDate, nil, nil); err != nil {
		t.Fatalf("ValidateAndRecordSuggestion: %v", err)
	}
	if len(order) != 2 || order[0] != "consume" || order[1] != "record" {
		t.Fatalf("call order = %v, want [consume record]", order)
	}
}

// TestValidateAndRecordSuggestion_InvalidDateNeverConsumesTheToken is the
// fix itself: a malformed or past date must leave the single-use token
// untouched, so the member can correct their input and resubmit the SAME
// link, instead of burning it on a bad first try.
func TestValidateAndRecordSuggestion_InvalidDateNeverConsumesTheToken(t *testing.T) {
	for _, tc := range []struct {
		name string
		date string
	}{
		{"malformed date", "not-a-date"},
		{"past date", time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var consumed, recorded bool
			future := time.Now().UTC().Add(time.Hour)
			repo := &mockSessionRepo{
				findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
					return &RSVPToken{Token: "st", SessionID: "s1", UserID: "u1", Action: RSVPActionSuggest, ExpiresAt: future}, nil
				},
				markRSVPTokenUsedFn: func(_ context.Context, _ string) error {
					consumed = true
					return nil
				},
				createRescheduleSuggestionFn: func(_ context.Context, _ *RescheduleSuggestion) error {
					recorded = true
					return nil
				},
			}
			svc := NewSessionService(repo, nil, nil)
			if _, err := svc.ValidateAndRecordSuggestion(context.Background(), "st", tc.date, nil, nil); err == nil {
				t.Fatal("expected a validation error for an invalid suggested date")
			}
			if consumed {
				t.Error("an invalid suggestion must not consume the single-use token")
			}
			if recorded {
				t.Error("an invalid suggestion must not be recorded")
			}
		})
	}
}

// TestValidateAndRecordSuggestion_WrongTokenActionRejected pins that a plain
// accept/decline/tentative token can never be replayed through the suggest
// flow (it has no suggested date/time of its own to validate against).
func TestValidateAndRecordSuggestion_WrongTokenActionRejected(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{Token: "rt", SessionID: "s1", UserID: "u1", Action: RSVPAccepted, ExpiresAt: future}, nil
		},
		markRSVPTokenUsedFn: func(_ context.Context, _ string) error {
			t.Error("a wrong-action token must not be consumed")
			return nil
		},
	}
	svc := NewSessionService(repo, nil, nil)
	if _, err := svc.ValidateAndRecordSuggestion(context.Background(), "rt", "2099-01-01", nil, nil); err == nil {
		t.Fatal("expected an error when a non-suggest token is used on the suggest flow")
	}
}

// TestValidateAndRecordSuggestion_LosingTheConsumeDoesNotRecord is the
// suggest-flow's twin of TestRSVPToken_LosingTheConsumeDoesNotApply: a lost
// race on the atomic used_at guard must stop before the suggestion is ever
// written.
func TestValidateAndRecordSuggestion_LosingTheConsumeDoesNotRecord(t *testing.T) {
	recorded := false
	future := time.Now().UTC().Add(time.Hour)
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{Token: "st", SessionID: "s1", UserID: "u1", Action: RSVPActionSuggest, ExpiresAt: future}, nil
		},
		markRSVPTokenUsedFn: func(_ context.Context, _ string) error { return ErrRSVPTokenSpent },
		createRescheduleSuggestionFn: func(_ context.Context, _ *RescheduleSuggestion) error {
			recorded = true
			return nil
		},
	}
	svc := NewSessionService(repo, nil, nil)
	validDate := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
	_, err := svc.ValidateAndRecordSuggestion(context.Background(), "st", validDate, nil, nil)
	if !errors.Is(err, ErrRSVPTokenSpent) {
		t.Fatalf("expected ErrRSVPTokenSpent, got %v", err)
	}
	if recorded {
		t.Error("the loser of the consume race must not record a suggestion")
	}
}

// --- DB-backed: real rows, not mocks ---------------------------------------

// TestDB_OccurrenceRSVPsAreIndependentAcrossNights is the row-level twin of
// TestUpdateRSVPDetailed_OccurrencesAreIndependent above: it proves the
// UNIQUE KEY (session_id, user_id, occurrence_date) on session_occurrence_rsvps
// really does keep two nights of the same series in separate rows against a
// real database, not just in a mock's call log.
func TestDB_OccurrenceRSVPsAreIndependentAcrossNights(t *testing.T) {
	if testing.Short() {
		t.Skip("row-level test")
	}
	db := newScratchDB(t)
	campID, ownerID := seedCampaign(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil, nil)
	ctx := context.Background()

	sessID := seedSession(t, db, campID, ownerID, "Weekly Game Night")
	rt := RecurrenceWeekly
	if _, err := db.ExecContext(ctx,
		`UPDATE sessions SET is_recurring = 1, recurrence_type = ?, scheduled_date = ? WHERE id = ?`,
		rt, "2026-01-06", sessID); err != nil {
		t.Fatalf("marking session recurring: %v", err)
	}
	player := seedUser(t, db, "Bo")

	night1 := "2026-01-06"
	night2 := "2026-01-13"
	if err := svc.UpdateRSVPDetailed(ctx, sessID, player, RSVPAccepted, &night1, patch.Of("bringing snacks")); err != nil {
		t.Fatalf("answering night 1: %v", err)
	}
	if err := svc.UpdateRSVPDetailed(ctx, sessID, player, RSVPDeclined, &night2, patch.Absent[string]()); err != nil {
		t.Fatalf("answering night 2: %v", err)
	}

	rsvps1, err := svc.ListOccurrenceAttendees(ctx, sessID, night1)
	if err != nil {
		t.Fatalf("listing night 1: %v", err)
	}
	rsvps2, err := svc.ListOccurrenceAttendees(ctx, sessID, night2)
	if err != nil {
		t.Fatalf("listing night 2: %v", err)
	}
	if len(rsvps1) != 1 || rsvps1[0].Status != RSVPAccepted {
		t.Fatalf("night 1 rsvps = %+v, want exactly one accepted row", rsvps1)
	}
	if rsvps1[0].Note == nil || *rsvps1[0].Note != "bringing snacks" {
		t.Errorf("night 1 note = %v, want %q", rsvps1[0].Note, "bringing snacks")
	}
	if len(rsvps2) != 1 || rsvps2[0].Status != RSVPDeclined {
		t.Fatalf("night 2 rsvps = %+v, want exactly one declined row — it must not have been overwritten by night 1's answer", rsvps2)
	}
	if rsvps2[0].Note != nil {
		t.Errorf("night 2 note = %v, want nil — night 1's note must not have leaked across", rsvps2[0].Note)
	}
}

// TestDB_SuggestionTokenCannotBeSpentTwiceConcurrently is the suggest-flow's
// row-level twin of TestDB_SessionRSVPTokenCannotBeSpentTwiceConcurrently:
// exactly one of two concurrent submissions of the SAME single-use suggest
// link may succeed.
func TestDB_SuggestionTokenCannotBeSpentTwiceConcurrently(t *testing.T) {
	if testing.Short() {
		t.Skip("row-level test")
	}
	db := newScratchDB(t)
	campID, ownerID := seedCampaign(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil, nil)
	ctx := context.Background()

	for attempt := 0; attempt < 10; attempt++ {
		sessID := seedSession(t, db, campID, ownerID, "Session")
		player := seedUser(t, db, "Bo")
		tok := &RSVPToken{
			Token:     newDBID(t),
			SessionID: sessID,
			UserID:    player,
			Action:    RSVPActionSuggest,
			ExpiresAt: time.Now().UTC().Add(time.Hour),
		}
		if err := repo.CreateRSVPToken(ctx, tok); err != nil {
			t.Fatalf("creating suggest token: %v", err)
		}

		validDate := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
		var wg sync.WaitGroup
		results := make([]error, 2)
		gate := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-gate
				_, results[i] = svc.ValidateAndRecordSuggestion(ctx, tok.Token, validDate, nil, nil)
			}(i)
		}
		close(gate)
		wg.Wait()

		ok := 0
		for _, err := range results {
			if err == nil {
				ok++
			}
		}
		if ok != 1 {
			t.Fatalf("attempt %d: %d of 2 concurrent submissions of ONE single-use suggest link succeeded, want exactly 1 (%v)",
				attempt, ok, results)
		}
	}
}
