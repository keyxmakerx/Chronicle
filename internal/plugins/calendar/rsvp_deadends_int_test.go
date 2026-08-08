// rsvp_deadends_int_test.go — C-CALV4-GAMEREADY §5, against a REAL MariaDB.
//
// WHY THESE CLAIMS NEEDED A DATABASE AND NOT A FAKE. Every dead end in §5 is a
// claim about a ROW: `calendar_event_rsvp_tokens.used_at` being NULL or not,
// and `calendar_event_rsvps` holding an answer the product then refuses to
// admit it has. A mock repository can be told that MarkRSVPTokenUsed was
// called; only a database can answer "and what does the row say now" — which is
// the exact question the audit's stateful probe asked, and the exact question
// the shipped suite never asked. `make test-db-up` provides the server; the
// scratch-schema helper (bench_month_cursor_test.go) provides the isolation.
//
// The suite SKIPS rather than fails with no server, per the house convention.
package calendar

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- fixture -----------------------------------------------------------------

// rsvpIntFixture is one campaign, one calendar, one COLLECTING event and one
// member who has been emailed their action links — the minimum shape in which
// an emailed RSVP link is a real thing rather than a struct.
type rsvpIntFixture struct {
	db         *sql.DB
	campaignID string
	userID     string
	// ownerID is the event's author — the person [GR-9]'s notification is for.
	ownerID  string
	eventID  string
	h        *RSVPHandler
	rsvpRepo RSVPRepository
}

func newRSVPIntFixture(t *testing.T) *rsvpIntFixture {
	t.Helper()
	db := newCalendarScratchSchema(t)
	ctx := context.Background()

	campaignID, cal := calTestSeedNavCalendar(t, db)
	calRepo := NewCalendarRepository(db)

	// The event's AUTHOR is a real row, because notifyOwnerOfResponse addresses
	// created_by: a fixture without one cannot tell "the Director was notified"
	// apart from "there was no Director to notify", which is exactly the
	// distinction [GR-9] turns on.
	ownerID := calTestID(t)
	if _, err := db.Exec(
		`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		ownerID, ownerID+"@example.test", "The Director", "x"); err != nil {
		t.Fatalf("seeding the event author: %v", err)
	}

	evt := &Event{
		ID: calTestID(t), CalendarID: cal.ID, Name: "Harvest Feast",
		Year: 1523, Month: 1, Day: 14, Visibility: storageVisibilityEveryone,
		CreatedBy: &ownerID,
	}
	if err := calRepo.CreateEvent(ctx, evt); err != nil {
		t.Fatalf("create event: %v", err)
	}

	rsvpRepo := NewRSVPRepository(db)
	// The opt-in the whole token flow is gated on: resolveToken refuses a link
	// to an event that is not collecting.
	if err := rsvpRepo.SetCollectRSVPs(ctx, evt.ID, true); err != nil {
		t.Fatalf("set collect_rsvps: %v", err)
	}

	// A real member row, because tokens carry a FK onto users(id).
	userID := calTestID(t)
	if _, err := db.Exec(
		`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.test", "Ari", "x"); err != nil {
		t.Fatalf("seeding a member: %v", err)
	}

	h := NewRSVPHandler(NewRSVPService(rsvpRepo, calRepo))
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: userID, Role: campaigns.RolePlayer, DisplayName: "Ari", Email: "ari@example.test"},
	}})
	h.SetRSVPNotifier(&mockNotifier{})

	return &rsvpIntFixture{
		db: db, campaignID: campaignID, userID: userID, ownerID: ownerID,
		eventID: evt.ID, h: h, rsvpRepo: rsvpRepo,
	}
}

// restoreRoster puts the one-member directory back after a test has emptied it
// to stand in for a member who has left the campaign.
func (f *rsvpIntFixture) restoreRoster() {
	f.h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: f.userID, Role: campaigns.RolePlayer, DisplayName: "Ari", Email: "ari@example.test"},
	}})
}

// mintOne mints the real action-token set and returns the one for `action`.
func (f *rsvpIntFixture) mintOne(t *testing.T, action string) string {
	t.Helper()
	toks, err := f.h.svc.MintActionTokens(context.Background(), f.eventID, f.userID)
	if err != nil {
		t.Fatalf("minting action tokens: %v", err)
	}
	tok := toks[action]
	if tok == "" {
		t.Fatalf("no %q token was minted; got %d actions", action, len(toks))
	}
	return tok
}

// usedAt reads the single-use column straight out of the table. It is the whole
// point of this file: the assertion is about the ROW, not about a call count.
func (f *rsvpIntFixture) usedAt(t *testing.T, token string) *string {
	t.Helper()
	var used sql.NullString
	err := f.db.QueryRow(
		`SELECT used_at FROM calendar_event_rsvp_tokens WHERE token = ?`, token).Scan(&used)
	if err != nil {
		t.Fatalf("reading used_at for the token: %v", err)
	}
	if !used.Valid {
		return nil
	}
	return &used.String
}

// --- [GR-7] the suggest token survives a rejected submit ---------------------

// TestRSVPSuggestToken_SurvivesRejection_Integration is the audit's probe P2,
// turned into a guard, against the real table.
//
// THE MEASURED DEFECT: GET the suggest link, POST a partially-filled row (date
// and from, NO to) with an empty note — exactly what the page invites, since
// every field looks optional and the textarea is labelled "(optional)" — and
// the server refuses it. `used_at` was ALREADY SET at that moment. Correcting
// the row and resubmitting answered "this RSVP link is invalid or has expired",
// and so did re-opening the link from the email. One incomplete form
// permanently destroyed a player's only way in.
//
// FOUR STEPS, IN THE ORDER A PLAYER LIVES THEM, because the third is the one
// the shipped suite could not have: refuse, ROW STILL NULL, correct and
// RESUBMIT THE SAME LINK, row now spent.
func TestRSVPSuggestToken_SurvivesRejection_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("RSVP dead-end integration tests require a database; skipped under -short")
	}
	f := newRSVPIntFixture(t)
	token := f.mintOne(t, RSVPActionSuggest)

	// 1. THE INCOMPLETE FORM. A date and a start with no end is dropped by
	//    parseOfferedWindows, so with an empty note there is nothing to record.
	rec := serveToken(f.h, http.MethodPost, token, "w0date=2026-08-05&w0from=18%3A00&note=")
	body := rec.Body.String()
	if !strings.Contains(body, "a time that would work") {
		t.Fatalf("the incomplete submission should have been refused; body = %q", body)
	}

	// 2. THE ROW. This is the assertion the fake-backed suite could not make.
	if got := f.usedAt(t, token); got != nil {
		t.Fatalf("a REFUSED suggestion consumed the link: used_at = %q, want NULL", *got)
	}

	// 3. THE SAME LINK, CORRECTED. This is the step that used to answer "this
	//    RSVP link is invalid or has expired".
	rec2 := serveToken(f.h, http.MethodPost, token,
		"w0date=2026-08-05&w0from=18%3A00&w0to=22%3A30&note=")
	if !strings.Contains(rec2.Body.String(), "Response recorded") {
		t.Fatalf("the corrected resubmission must be accepted on the SAME link; body = %q",
			rec2.Body.String())
	}

	// 4. SINGLE-USE IS NOT WEAKENED — it moved, it did not go away.
	if got := f.usedAt(t, token); got == nil {
		t.Fatal("an ACCEPTED suggestion must consume the link: used_at is still NULL")
	}
	rec3 := serveToken(f.h, http.MethodPost, token, "note=and+again")
	if !strings.Contains(rec3.Body.String(), "invalid or has expired") &&
		!strings.Contains(rec3.Body.String(), "already answered") {
		t.Errorf("a spent suggest link must not be redeemable a second time; body = %q",
			rec3.Body.String())
	}
}

// --- [GR-9] the emailed "Out this week" reaches the Director -----------------

// TestRSVPOutWeek_NotifiesAndRecords_Integration ties [GR-9]'s notification to
// the ROW the Director's count is computed from.
//
// The unit guard (TestRSVPToken_OutWeekNotifiesOwner) pins that the notifier
// fires on both surfaces. What it cannot show is the thing that made the
// Director's arithmetic wrong: the decline WAS being written to
// calendar_event_rsvps as `no` the whole time, so the tally silently moved while
// the one person who needed to act on it was never told. Notification and row
// are asserted together here for that reason.
func TestRSVPOutWeek_NotifiesAndRecords_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("RSVP dead-end integration tests require a database; skipped under -short")
	}
	f := newRSVPIntFixture(t)
	notifier := &mockNotifier{}
	f.h.SetRSVPNotifier(notifier)

	token := f.mintOne(t, RSVPActionOutWeek)
	body := serveToken(f.h, http.MethodPost, token, "").Body.String()
	if !strings.Contains(body, "not attending") {
		t.Fatalf("the member should be told they are marked out; body = %q", body)
	}

	if len(notifier.userIDs) != 1 || notifier.userIDs[0] != f.ownerID {
		t.Errorf("the emailed \"Out this week\" must reach the Director — it is the one "+
			"decline most likely to cancel the session; notified %v, want [%s]",
			notifier.userIDs, f.ownerID)
	}
	stored, err := f.rsvpRepo.GetUserRSVP(context.Background(), f.eventID, f.userID)
	if err != nil || stored == nil {
		t.Fatalf("the decline must be on record; got %+v err=%v", stored, err)
	}
	if stored.Status != RSVPNo {
		t.Errorf("an \"out this week\" is a decline: status = %q, want %q", stored.Status, RSVPNo)
	}
}

// --- [GR-8] a spent link states the answer and offers a way back -------------

// TestRSVPResult_SpentTokenStatesTheAnswer is the audit's second dead end, and
// the reason it is a table blocker rather than polish.
//
// THE MEASURED DEFECT: after a successful POST, a second POST — a browser
// refresh, a double-tap, a mail client prefetch, or a member simply checking
// "did that go through?" — rendered "RSVP Failed / this RSVP link is invalid or
// has expired". The answer WAS on record. The page just said it failed, and the
// page contained no `<a>` at all, so there was nowhere to go and check. A player
// who sees that tells their GM the RSVP system is broken, the GM believes them
// because the product said so, and session zero goes on debugging something that
// works.
//
// THREE CASES IN ONE TEST, AND THE TWO NEGATIVES ARE THE POINT. Widening the
// answer page by a single condition is a disclosure: the audit verified that a
// REMOVED MEMBER and a `dm_only`-FLIPPED EVENT get a generic invalid-link page
// that never leaks the title, and that behaviour is preserved exactly. Both
// negatives redeem the SAME spent token as the positive, so the only thing that
// differs between "you're down as Going" and "invalid or has expired" is the
// re-check — which is what makes them a control rather than three unrelated
// assertions.
func TestRSVPResult_SpentTokenStatesTheAnswer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("RSVP dead-end integration tests require a database; skipped under -short")
	}
	f := newRSVPIntFixture(t)
	token := f.mintOne(t, RSVPActionYes)

	// The first redemption: the real one, which really records "yes".
	first := serveToken(f.h, http.MethodPost, token, "")
	if !strings.Contains(first.Body.String(), "Response recorded") {
		t.Fatalf("the first redemption should succeed; body = %q", first.Body.String())
	}
	// The success page's own door ([GR-8]: "the same link goes on the SUCCESS
	// page") — the page every answering member actually sees.
	if !strings.Contains(first.Body.String(), "/campaigns/"+f.campaignID+"/schedule") {
		t.Errorf("the SUCCESS page must offer a route back into the product; body = %q",
			first.Body.String())
	}
	// The row really is there — so anything the second visit says about it is a
	// statement about persisted state, not about a render.
	stored, err := f.rsvpRepo.GetUserRSVP(context.Background(), f.eventID, f.userID)
	if err != nil || stored == nil || stored.Status != RSVPYes {
		t.Fatalf("the answer must be on record; got %+v err=%v", stored, err)
	}

	t.Run("the refresh states the answer instead of claiming failure", func(t *testing.T) {
		rec := serveToken(f.h, http.MethodPost, token, "")
		body := rec.Body.String()
		if strings.Contains(body, "RSVP Failed") {
			t.Errorf("a spent link with an answer on record must NOT report failure; body = %q", body)
		}
		if !strings.Contains(body, "already answered") || !strings.Contains(body, "Going") {
			t.Errorf("the page must STATE the answer on record; body = %q", body)
		}
		if !strings.Contains(body, "/campaigns/"+f.campaignID+"/schedule") {
			t.Errorf("the page must offer a way back to change it; body = %q", body)
		}
	})

	t.Run("re-opening the emailed link (GET) says the same thing", func(t *testing.T) {
		// The member's most likely action is not a re-POST — it is clicking the
		// link in the email again, which is a GET.
		body := serveToken(f.h, http.MethodGet, token, "").Body.String()
		if !strings.Contains(body, "already answered") {
			t.Errorf("re-opening a spent link must state the answer too; body = %q", body)
		}
	})

	t.Run("a REMOVED MEMBER still gets the generic page", func(t *testing.T) {
		f.h.SetMemberDirectory(&mockMemberDir{members: nil})
		t.Cleanup(f.restoreRoster)
		body := serveToken(f.h, http.MethodPost, token, "").Body.String()
		if strings.Contains(body, "already answered") || strings.Contains(body, "Harvest Feast") {
			t.Errorf("a removed member must not be told the answer or the title; body = %q", body)
		}
		if !strings.Contains(body, "invalid or has expired") {
			t.Errorf("a removed member must get the GENERIC page; body = %q", body)
		}
	})

	t.Run("a dm_only-FLIPPED EVENT still gets the generic page", func(t *testing.T) {
		if _, err := f.db.Exec(
			`UPDATE calendar_events SET visibility = ? WHERE id = ?`,
			storageVisibilityDMOnly, f.eventID); err != nil {
			t.Fatalf("flipping the event to dm_only: %v", err)
		}
		t.Cleanup(func() {
			if _, err := f.db.Exec(`UPDATE calendar_events SET visibility = ? WHERE id = ?`,
				storageVisibilityEveryone, f.eventID); err != nil {
				t.Logf("restoring visibility: %v", err)
			}
		})
		body := serveToken(f.h, http.MethodPost, token, "").Body.String()
		if strings.Contains(body, "already answered") || strings.Contains(body, "Harvest Feast") {
			t.Errorf("an event flipped to dm_only must not leak its title or the answer; body = %q", body)
		}
		if !strings.Contains(body, "invalid or has expired") {
			t.Errorf("a flipped event must get the GENERIC page; body = %q", body)
		}
	})

	t.Run("a token that was never minted gets the generic page", func(t *testing.T) {
		body := serveToken(f.h, http.MethodPost,
			strings.Repeat("a", 64), "").Body.String()
		if !strings.Contains(body, "invalid or has expired") {
			t.Errorf("an unknown token must stay indistinguishable from a spent one; body = %q", body)
		}
	})
}
