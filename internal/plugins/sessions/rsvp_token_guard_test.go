package sessions

// The session RSVP token (/rsvp/:token) — the three things it must do that it
// was not doing.
//
//  1. RE-CHECK MEMBERSHIP. Its two sibling token routes both do:
//     /proposals/respond/:token calls isCampaignMember on BOTH halves, and
//     /calendar-rsvp/:token states the invariant outright — "a link cannot
//     outlive the access that justified it". This one skipped it, so a player
//     removed from the campaign kept a working "Going" link for the token's full
//     7-day life, writing `accepted` into session_attendees for a campaign they
//     were no longer part of. The Director's "3/5 going" counted a non-member
//     and the attendee list rendered their name to the party.
//
//  2. BE SINGLE-USE FOR REAL. MarkRSVPTokenUsed had no `used_at IS NULL`
//     predicate and no RowsAffected check, and the service applied before
//     consuming — so two submissions that both validated both applied, and
//     neither could tell it had lost.
//
//  3. NOT 404 ON A ROW THAT IS SITTING RIGHT THERE. See the attendee upsert
//     guards below.

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// tokenHandler builds a handler whose token resolves to (sessionID, userID) in
// campaign camp-1, with the given roster.
func tokenHandler(t *testing.T, roster []string, applied *string) *Handler {
	t.Helper()
	future := time.Now().UTC().Add(time.Hour)
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{Token: "rt", SessionID: "s1", UserID: "ex-member", Action: RSVPAccepted, ExpiresAt: future}, nil
		},
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, CampaignID: "camp-1"}, nil
		},
		updateAttendeeStatusFn: func(_ context.Context, _, uid, status string) error {
			*applied = uid + ":" + status
			return nil
		},
		markRSVPTokenUsedFn: func(_ context.Context, _ string) error { return nil },
	}
	members := make([]campaigns.CampaignMember, 0, len(roster))
	for _, id := range roster {
		members = append(members, campaigns.CampaignMember{UserID: id})
	}
	h := &Handler{svc: NewSessionService(repo, nil)}
	h.SetMemberLister(&stubMemberLister{members: members})
	return h
}

func rsvpTokenCtx(method string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(method, "/rsvp/rt", nil), rec)
	c.SetParamNames("token")
	c.SetParamValues("rt")
	return c, rec
}

// TestRSVPToken_RemovedMemberIsRefused covers BOTH halves. The GET matters as
// much as the POST: showing a removed player a "Confirm — Going" button that
// then fails is its own defect.
func TestRSVPToken_RemovedMemberIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		call   func(*Handler, echo.Context) error
	}{
		{"GET confirm page", http.MethodGet, (*Handler).RedeemRSVPToken},
		{"POST apply", http.MethodPost, (*Handler).ApplyRSVPToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var applied string
			// The roster no longer contains "ex-member".
			h := tokenHandler(t, []string{"still-here"}, &applied)
			c, rec := rsvpTokenCtx(tc.method)
			if err := tc.call(h, c); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if applied != "" {
				t.Errorf("a removed member's emailed link still wrote %q", applied)
			}
			if !strings.Contains(rec.Body.String(), "no longer a member") {
				t.Errorf("the page does not say why it was refused; body=%s", rec.Body.String())
			}
		})
	}
}

// TestRSVPToken_CurrentMemberStillWorks is the other half of the same coin: the
// recheck must not break the ordinary case. Without this a "fix" that refuses
// everybody would look green.
func TestRSVPToken_CurrentMemberStillWorks(t *testing.T) {
	var applied string
	h := tokenHandler(t, []string{"ex-member", "still-here"}, &applied)
	c, rec := rsvpTokenCtx(http.MethodPost)
	if err := h.ApplyRSVPToken(c); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if applied != "ex-member:"+RSVPAccepted {
		t.Fatalf("a current member's link did not apply (applied=%q); body=%s", applied, rec.Body.String())
	}
}

// TestRSVPToken_FailsClosedWithoutARoster pins the direction of the failure: a
// nil member lister (or a lookup error) must DENY, never wave the link through.
func TestRSVPToken_FailsClosedWithoutARoster(t *testing.T) {
	var applied string
	future := time.Now().UTC().Add(time.Hour)
	h := &Handler{svc: NewSessionService(&mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{Token: "rt", SessionID: "s1", UserID: "u1", Action: RSVPAccepted, ExpiresAt: future}, nil
		},
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, CampaignID: "camp-1"}, nil
		},
		updateAttendeeStatusFn: func(_ context.Context, _, uid, status string) error {
			applied = uid + ":" + status
			return nil
		},
		markRSVPTokenUsedFn: func(_ context.Context, _ string) error { return nil },
	}, nil)} // no memberLister wired at all
	c, _ := rsvpTokenCtx(http.MethodPost)
	if err := h.ApplyRSVPToken(c); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if applied != "" {
		t.Errorf("with no roster available the link still applied %q — it must fail closed", applied)
	}
}

// --- single-use: the ORDER, pinned deterministically ------------------------

// TestRSVPToken_ConsumesBeforeItApplies is the structural guarantee behind the
// single-use promise, and it is deterministic — unlike the concurrency test
// below, which can only ever observe a race it happens to lose.
//
// The old order was apply-then-consume with a consume that could not report
// having consumed nothing, so the loser of a race applied anyway. Consuming
// first makes the atomic `used_at IS NULL` UPDATE the gate.
func TestRSVPToken_ConsumesBeforeItApplies(t *testing.T) {
	var order []string
	future := time.Now().UTC().Add(time.Hour)
	svc := &sessionService{repo: &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{Token: "rt", SessionID: "s1", UserID: "u1", Action: RSVPAccepted, ExpiresAt: future}, nil
		},
		updateAttendeeStatusFn: func(_ context.Context, _, _, _ string) error {
			order = append(order, "apply")
			return nil
		},
		markRSVPTokenUsedFn: func(_ context.Context, _ string) error {
			order = append(order, "consume")
			return nil
		},
	}}
	if _, err := svc.ApplyRSVPToken(context.Background(), "rt"); err != nil {
		t.Fatalf("ApplyRSVPToken: %v", err)
	}
	if len(order) != 2 || order[0] != "consume" || order[1] != "apply" {
		t.Fatalf("call order = %v, want [consume apply] — applying first lets the loser of a "+
			"race write before the token is spent", order)
	}
}

// TestRSVPToken_LosingTheConsumeDoesNotApply is the other half: when the
// consume reports the token was already spent, nothing may be written.
func TestRSVPToken_LosingTheConsumeDoesNotApply(t *testing.T) {
	applied := false
	future := time.Now().UTC().Add(time.Hour)
	svc := &sessionService{repo: &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{Token: "rt", SessionID: "s1", UserID: "u1", Action: RSVPAccepted, ExpiresAt: future}, nil
		},
		updateAttendeeStatusFn: func(_ context.Context, _, _, _ string) error { applied = true; return nil },
		markRSVPTokenUsedFn:    func(_ context.Context, _ string) error { return ErrRSVPTokenSpent },
	}}
	_, err := svc.ApplyRSVPToken(context.Background(), "rt")
	if err == nil {
		t.Fatal("losing the consume race was reported as success")
	}
	if applied {
		t.Error("the loser of the consume race still wrote an RSVP")
	}
	if !strings.Contains(err.Error(), "already been used") {
		t.Errorf("error = %v, want the member-facing 'already been used' message", err)
	}
}

// --- row-level: single-use, and the attendee upsert -------------------------

// TestDB_SessionRSVPTokenCannotBeSpentTwiceConcurrently is the race the missing
// `used_at IS NULL` predicate left open. Both goroutines validate before either
// consumes; exactly one may apply.
//
// Run repeatedly so a non-interleaving run cannot pass for coverage.
func TestDB_SessionRSVPTokenCannotBeSpentTwiceConcurrently(t *testing.T) {
	if testing.Short() {
		t.Skip("row-level test")
	}
	db := newScratchDB(t)
	campID, userID := seedCampaign(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil)
	ctx := context.Background()

	for attempt := 0; attempt < 40; attempt++ {
		sessID := seedSession(t, db, campID, userID, "Session 41")
		if err := repo.AddAttendee(ctx, sessID, userID, RSVPInvited); err != nil {
			t.Fatalf("invite: %v", err)
		}
		accept, _, err := svc.CreateRSVPTokens(ctx, sessID, userID)
		if err != nil {
			t.Fatalf("CreateRSVPTokens: %v", err)
		}

		var wg sync.WaitGroup
		results := make([]error, 2)
		gate := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-gate
				_, results[i] = svc.ApplyRSVPToken(ctx, accept)
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
			t.Fatalf("attempt %d: %d of 2 concurrent redemptions of ONE single-use link succeeded, want exactly 1 (%v)",
				attempt, ok, results)
		}
	}
}

// TestDB_MarkRSVPTokenUsedReportsTheLoser is the deterministic, race-free
// statement of the same defect: consuming an already-spent token must be
// distinguishable from consuming a fresh one, or no caller can ever detect the
// loser of a race.
func TestDB_MarkRSVPTokenUsedReportsTheLoser(t *testing.T) {
	if testing.Short() {
		t.Skip("row-level test")
	}
	db := newScratchDB(t)
	campID, userID := seedCampaign(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil)
	ctx := context.Background()

	sessID := seedSession(t, db, campID, userID, "S")
	accept, _, err := svc.CreateRSVPTokens(ctx, sessID, userID)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}

	if err := repo.MarkRSVPTokenUsed(ctx, accept); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	var firstUsed sql.NullTime
	if err := db.QueryRow(`SELECT used_at FROM session_rsvp_tokens WHERE token=?`, accept).Scan(&firstUsed); err != nil {
		t.Fatalf("reading used_at: %v", err)
	}

	if err := repo.MarkRSVPTokenUsed(ctx, accept); err == nil {
		t.Fatal("consuming an ALREADY-SPENT token returned nil — the caller cannot detect a lost race")
	}

	// And the audit of WHEN the link was really spent must survive: the losing
	// consume must not overwrite used_at.
	var secondUsed sql.NullTime
	if err := db.QueryRow(`SELECT used_at FROM session_rsvp_tokens WHERE token=?`, accept).Scan(&secondUsed); err != nil {
		t.Fatalf("re-reading used_at: %v", err)
	}
	if !secondUsed.Valid || !secondUsed.Time.Equal(firstUsed.Time) {
		t.Errorf("used_at moved from %v to %v — the second consume overwrote the audit trail",
			firstUsed.Time, secondUsed.Time)
	}
}

// TestDB_SameStatusRSVPTwiceInOneSecondSucceeds pins the "flaky RSVP" 404.
//
// The old UPDATE reported RowsAffected==0 as "attendee not found". MariaDB
// counts CHANGED rows, not matched rows, and responded_at = NOW() has
// one-second resolution — so a double-tap of Going within one second changed
// nothing and 404'd on a row that was sitting right there. The loop makes the
// same-second case certain rather than hoping for it.
func TestDB_SameStatusRSVPTwiceInOneSecondSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("row-level test")
	}
	db := newScratchDB(t)
	campID, userID := seedCampaign(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil)
	ctx := context.Background()

	sessID := seedSession(t, db, campID, userID, "Session 41")
	if err := repo.AddAttendee(ctx, sessID, userID, RSVPInvited); err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := svc.UpdateRSVP(ctx, sessID, userID, RSVPAccepted); err != nil {
		t.Fatalf("first RSVP: %v", err)
	}
	for i := 0; i < 25; i++ { // certainly inside one second for at least one pair
		if err := svc.UpdateRSVP(ctx, sessID, userID, RSVPAccepted); err != nil {
			t.Fatalf("re-submitting the SAME status was refused with %v — a member who "+
				"double-taps Going is told their attendee row does not exist", err)
		}
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM session_attendees WHERE session_id=? AND user_id=?`,
		sessID, userID).Scan(&status); err != nil {
		t.Fatalf("attendee row missing: %v", err)
	}
	if status != RSVPAccepted {
		t.Errorf("stored status = %q, want %q", status, RSVPAccepted)
	}
}

// TestDB_MemberWhoJoinedLaterCanRSVP pins the late-joiner case.
//
// InviteAll runs only at session-creation time, and no route adds an attendee
// afterwards — so a player who joined the campaign on Tuesday had no row on
// last week's Saturday session, no RSVP control on the page, and a 404 if they
// posted anyway. The Director had no invite action either; the only escape was
// deleting and recreating the session, which loses its notes, recap and entity
// links.
func TestDB_MemberWhoJoinedLaterCanRSVP(t *testing.T) {
	if testing.Short() {
		t.Skip("row-level test")
	}
	db := newScratchDB(t)
	campID, ownerID := seedCampaign(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil)
	ctx := context.Background()

	sessID := seedSession(t, db, campID, ownerID, "Session 41")
	if err := svc.InviteAll(ctx, sessID, []string{ownerID}); err != nil {
		t.Fatalf("InviteAll: %v", err)
	}

	newcomer := seedUser(t, db, "Bo")
	if err := svc.UpdateRSVP(ctx, sessID, newcomer, RSVPAccepted); err != nil {
		t.Fatalf("a member who joined after the session was created could not RSVP: %v", err)
	}

	attendees, err := svc.ListAttendees(ctx, sessID)
	if err != nil {
		t.Fatalf("ListAttendees: %v", err)
	}
	if len(attendees) != 2 {
		t.Fatalf("attendee rows = %d, want 2 (the original member and the newcomer)", len(attendees))
	}
	var found bool
	for _, a := range attendees {
		if a.UserID == newcomer {
			found = true
			if a.Status != RSVPAccepted {
				t.Errorf("newcomer status = %q, want %q", a.Status, RSVPAccepted)
			}
		}
	}
	if !found {
		t.Error("the newcomer is still absent from the attendee list, so the page renders " +
			"no RSVP control for them and the 'N going / M invited' line ignores them")
	}
}
