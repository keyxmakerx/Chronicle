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
	eventID    string
	h          *RSVPHandler
	rsvpRepo   RSVPRepository
}

func newRSVPIntFixture(t *testing.T) *rsvpIntFixture {
	t.Helper()
	db := newCalendarScratchSchema(t)
	ctx := context.Background()

	campaignID, cal := calTestSeedNavCalendar(t, db)
	calRepo := NewCalendarRepository(db)

	evt := &Event{
		ID: calTestID(t), CalendarID: cal.ID, Name: "Harvest Feast",
		Year: 1523, Month: 1, Day: 14, Visibility: storageVisibilityEveryone,
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
		db: db, campaignID: campaignID, userID: userID, eventID: evt.ID,
		h: h, rsvpRepo: rsvpRepo,
	}
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
