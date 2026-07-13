package sessions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// sched_p3_test.go — C-SCHED-P3. Pins the item-0 security fixes (0a token
// closed+membership recheck, 0b GET-confirm/POST-apply, 0c email HTML escaping)
// and the P3-proper confirm-winner → session flow, the scheduled-time field, and
// recurrence parity. Service-level where possible; the token routes are exercised
// at the handler level (they carry no campaign middleware) with lightweight stubs.

// --- stubs for handler-level token tests ---

type stubMemberLister struct{ members []campaigns.CampaignMember }

func (s *stubMemberLister) ListMembers(_ context.Context, _ string) ([]campaigns.CampaignMember, error) {
	return s.members, nil
}

type stubUserDir struct{ tz string }

func (s *stubUserDir) GetUser(_ context.Context, userID string) (*auth.User, error) {
	tz := s.tz
	return &auth.User{ID: userID, DisplayName: "Player", Timezone: &tz}, nil
}

type captureMailer struct{ lastHTML string }

func (m *captureMailer) SendHTMLMail(_ context.Context, _ []string, _, _, htmlBody string) error {
	m.lastHTML = htmlBody
	return nil
}
func (m *captureMailer) IsConfigured(_ context.Context) bool { return true }

// --- P3: scheduled-time field ---

func TestFormatScheduledDate_LearnsTime(t *testing.T) {
	date := "2028-03-08"
	clock := "19:30"
	cases := []struct {
		name string
		s    Session
		want string
	}{
		{"date + time", Session{ScheduledDate: &date, ScheduledTime: &clock}, "Wed, Mar 8, 2028 · 7:30 PM"},
		{"date only", Session{ScheduledDate: &date}, "Wed, Mar 8, 2028"},
		{"no date", Session{}, ""},
	}
	for _, tc := range cases {
		if got := tc.s.FormatScheduledDate(); got != tc.want {
			t.Errorf("%s: FormatScheduledDate() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// --- P3: confirm-winner flow ---

func TestConfirmProposalWinner_ClosesAndCreatesSession(t *testing.T) {
	// Winning slot: 2026-07-18 23:00 UTC → America/New_York (EDT, UTC-4) = 19:00.
	startUTC := time.Date(2026, 7, 18, 23, 0, 0, 0, time.UTC)
	var closedPID, closedOID string
	var created *Session
	repo := &mockSessionRepo{
		getProposalFn: func(_ context.Context, _, _ string) (*SlotProposal, []SlotProposalOption, error) {
			return &SlotProposal{ID: "p1", CampaignID: "c1", Title: "The Dragon's Lair", Status: ProposalOpen},
				[]SlotProposalOption{{ID: "o1", ProposalID: "p1", StartsAtUTC: startUTC, EndsAtUTC: startUTC.Add(3 * time.Hour)}}, nil
		},
		setProposalWinnerAndCloseFn: func(_ context.Context, pid, oid string) error { closedPID, closedOID = pid, oid; return nil },
		createFn:                    func(_ context.Context, _ string, s *Session) error { created = s; return nil },
	}
	svc := NewSessionService(repo, nil)

	session, err := svc.ConfirmProposalWinner(context.Background(), "c1", "p1", "o1", "dm-user", "America/New_York")
	if err != nil {
		t.Fatalf("ConfirmProposalWinner: %v", err)
	}
	if closedPID != "p1" || closedOID != "o1" {
		t.Errorf("winner/close = (%q,%q), want (p1,o1)", closedPID, closedOID)
	}
	if created == nil {
		t.Fatal("expected a session to be created")
	}
	if created.Name != "The Dragon's Lair" {
		t.Errorf("session name = %q, want the proposal title", created.Name)
	}
	if created.ScheduledDate == nil || *created.ScheduledDate != "2026-07-18" {
		t.Errorf("scheduled_date = %v, want 2026-07-18 (confirmer zone)", created.ScheduledDate)
	}
	if created.ScheduledTime == nil || *created.ScheduledTime != "19:00" {
		t.Errorf("scheduled_time = %v, want 19:00 (23:00 UTC in EDT)", created.ScheduledTime)
	}
	if created.Status != StatusPlanned {
		t.Errorf("status = %q, want planned", created.Status)
	}
	if session == nil || session.ID != created.ID {
		t.Error("returned session should be the created one")
	}
}

func TestConfirmProposalWinner_RejectsClosedProposal(t *testing.T) {
	closed := false
	repo := &mockSessionRepo{
		getProposalFn: func(_ context.Context, _, _ string) (*SlotProposal, []SlotProposalOption, error) {
			return &SlotProposal{ID: "p1", CampaignID: "c1", Status: ProposalClosed},
				[]SlotProposalOption{{ID: "o1", ProposalID: "p1"}}, nil
		},
		setProposalWinnerAndCloseFn: func(_ context.Context, _, _ string) error { closed = true; return nil },
	}
	svc := NewSessionService(repo, nil)
	if _, err := svc.ConfirmProposalWinner(context.Background(), "c1", "p1", "o1", "dm", "UTC"); err == nil {
		t.Error("expected rejection when confirming an already-closed proposal")
	}
	if closed {
		t.Error("a closed proposal must not be re-confirmed")
	}
}

func TestConfirmProposalWinner_ConcurrentCloseNoDuplicateSession(t *testing.T) {
	// Simulate a concurrent confirm having won the close: the conditional close
	// matched zero rows → errProposalAlreadyClosed. The winner must NOT create a
	// second session.
	created := false
	repo := &mockSessionRepo{
		getProposalFn: func(_ context.Context, _, _ string) (*SlotProposal, []SlotProposalOption, error) {
			return &SlotProposal{ID: "p1", CampaignID: "c1", Status: ProposalOpen},
				[]SlotProposalOption{{ID: "o1", ProposalID: "p1", StartsAtUTC: time.Now().UTC(), EndsAtUTC: time.Now().UTC().Add(time.Hour)}}, nil
		},
		setProposalWinnerAndCloseFn: func(_ context.Context, _, _ string) error { return errProposalAlreadyClosed },
		createFn:                    func(_ context.Context, _ string, _ *Session) error { created = true; return nil },
	}
	svc := NewSessionService(repo, nil)
	if _, err := svc.ConfirmProposalWinner(context.Background(), "c1", "p1", "o1", "dm", "UTC"); err == nil {
		t.Error("expected an already-confirmed rejection on a lost close race")
	}
	if created {
		t.Error("a lost close race must NOT create a duplicate session")
	}
}

// --- P3: recurrence parity — the wall-clock time carries to the next occurrence ---

func TestGenerateNextOccurrence_CarriesScheduledTime(t *testing.T) {
	date := "2026-07-18"
	clock := "19:00"
	weekly := RecurrenceWeekly
	var next *Session
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, _ string) (*Session, error) {
			return &Session{
				ID: "s1", CampaignID: "c1", Name: "Weekly Game", Status: StatusPlanned,
				ScheduledDate: &date, ScheduledTime: &clock,
				IsRecurring: true, RecurrenceType: &weekly, RecurrenceInterval: 1,
			}, nil
		},
		updateFn:        func(_ context.Context, _ *Session) error { return nil },
		createFn:        func(_ context.Context, _ string, s *Session) error { next = s; return nil },
		listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) { return nil, nil },
	}
	svc := NewSessionService(repo, nil)

	if _, err := svc.UpdateSession(context.Background(), "s1", UpdateSessionInput{
		Name: "Weekly Game", Status: StatusCompleted,
		ScheduledDate: &date, ScheduledTime: &clock,
		IsRecurring: true, RecurrenceType: &weekly, RecurrenceInterval: 1,
	}); err != nil {
		t.Fatalf("UpdateSession(complete): %v", err)
	}
	if next == nil {
		t.Fatal("completing a recurring session should generate the next occurrence")
	}
	if next.ScheduledDate == nil || *next.ScheduledDate != "2026-07-25" {
		t.Errorf("next date = %v, want 2026-07-25 (weekly +7)", next.ScheduledDate)
	}
	if next.ScheduledTime == nil || *next.ScheduledTime != "19:00" {
		t.Errorf("next time = %v, want 19:00 preserved (recurrence parity)", next.ScheduledTime)
	}
}

// --- 0b: proposal token GET renders a confirm page (no apply); POST applies ---

func proposalTokenHandler(applied *bool, members []campaigns.CampaignMember) *Handler {
	future := time.Now().UTC().Add(time.Hour)
	repo := &mockSessionRepo{
		findProposalTokenFn: func(_ context.Context, _ string) (*SlotProposalToken, error) {
			return &SlotProposalToken{Token: "tok", OptionID: "o1", UserID: "u1", Response: ResponseYes, ExpiresAt: future}, nil
		},
		findOptionFn:       func(_ context.Context, _ string) (*SlotProposalOption, error) { return &SlotProposalOption{ID: "o1", ProposalID: "p1", StartsAtUTC: time.Now().UTC(), EndsAtUTC: time.Now().UTC().Add(time.Hour)}, nil },
		findProposalByIDFn: func(_ context.Context, _ string) (*SlotProposal, error) { return &SlotProposal{ID: "p1", CampaignID: "c1", Title: "Game", Status: ProposalOpen}, nil },
		upsertProposalResponseFn: func(_ context.Context, _ *SlotProposalResponse) error { *applied = true; return nil },
		markProposalTokenUsedFn:  func(_ context.Context, _ string) error { return nil },
	}
	return &Handler{svc: NewSessionService(repo, nil), memberLister: &stubMemberLister{members: members}, userDir: &stubUserDir{tz: "UTC"}}
}

func tokenCtx(method, token string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/proposals/respond/"+token, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("token")
	c.SetParamValues(token)
	return c, rec
}

func TestProposalToken_GetConfirmsPost_Applies(t *testing.T) {
	member := []campaigns.CampaignMember{{UserID: "u1"}}

	// GET is a pure read: renders a POST form, records nothing (0b).
	applied := false
	h := proposalTokenHandler(&applied, member)
	c, rec := tokenCtx(http.MethodGet, "tok")
	if err := h.RedeemProposalToken(c); err != nil {
		t.Fatalf("GET RedeemProposalToken: %v", err)
	}
	if applied {
		t.Error("GET must NOT apply the response (mail-prefetch safety)")
	}
	if !strings.Contains(rec.Body.String(), `method="POST"`) {
		t.Errorf("GET should render a POST confirm form; body=%s", rec.Body.String())
	}
	// The form must carry the CSRF field or the real POST would 403 under the
	// global CSRF middleware (the token POST routes aren't exempt).
	if !strings.Contains(rec.Body.String(), `name="csrf_token"`) {
		t.Errorf("confirm form must include the csrf_token field; body=%s", rec.Body.String())
	}

	// POST applies (0b).
	applied = false
	h = proposalTokenHandler(&applied, member)
	c, _ = tokenCtx(http.MethodPost, "tok")
	if err := h.ApplyProposalToken(c); err != nil {
		t.Fatalf("POST ApplyProposalToken: %v", err)
	}
	if !applied {
		t.Error("POST must apply the response")
	}
}

// --- 0a: proposal token rejects a user who is no longer a member ---

func TestProposalToken_NonMemberRejected(t *testing.T) {
	applied := false
	h := proposalTokenHandler(&applied, nil) // no members → u1 is not a member
	c, rec := tokenCtx(http.MethodPost, "tok")
	if err := h.ApplyProposalToken(c); err != nil {
		t.Fatalf("POST ApplyProposalToken: %v", err)
	}
	if applied {
		t.Error("a non-member's token must not apply (0a membership recheck)")
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "member") {
		t.Errorf("expected a not-a-member message; body=%s", rec.Body.String())
	}
}

// --- 0b: RSVP token GET renders a confirm page (no apply) ---

func TestRSVPToken_GetDoesNotApply(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	applied := false
	repo := &mockSessionRepo{
		findRSVPTokenFn:        func(_ context.Context, _ string) (*RSVPToken, error) { return &RSVPToken{Token: "rt", SessionID: "s1", UserID: "u1", Action: RSVPAccepted, ExpiresAt: future}, nil },
		updateAttendeeStatusFn: func(_ context.Context, _, _, _ string) error { applied = true; return nil },
		markRSVPTokenUsedFn:    func(_ context.Context, _ string) error { return nil },
	}
	h := &Handler{svc: NewSessionService(repo, nil)}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/rsvp/rt", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("token")
	c.SetParamValues("rt")
	if err := h.RedeemRSVPToken(c); err != nil {
		t.Fatalf("GET RedeemRSVPToken: %v", err)
	}
	if applied {
		t.Error("GET /rsvp/:token must NOT apply the RSVP (mail-prefetch safety)")
	}
	if !strings.Contains(rec.Body.String(), `method="POST"`) || !strings.Contains(rec.Body.String(), `name="csrf_token"`) {
		t.Errorf("GET should render a POST confirm form with a CSRF field; body=%s", rec.Body.String())
	}

	// POST applies.
	applied = false
	c2 := e.NewContext(httptest.NewRequest(http.MethodPost, "/rsvp/rt", nil), httptest.NewRecorder())
	c2.SetParamNames("token")
	c2.SetParamValues("rt")
	if err := h.ApplyRSVPToken(c2); err != nil {
		t.Fatalf("POST ApplyRSVPToken: %v", err)
	}
	if !applied {
		t.Error("POST /rsvp/:token must apply the RSVP")
	}
}

// --- 0c: email HTML escapes operator-authored strings ---

func TestProposalEmail_EscapesTitleAndCampaign(t *testing.T) {
	mailer := &captureMailer{}
	repo := &mockSessionRepo{
		createProposalTokenFn: func(_ context.Context, _ *SlotProposalToken) error { return nil },
	}
	h := &Handler{svc: NewSessionService(repo, nil), mailer: mailer, userDir: &stubUserDir{tz: "UTC"}, baseURL: "https://x.test"}

	proposal := &SlotProposal{ID: "p1", Title: `<script>alert(1)</script>`}
	options := []ProposalOptionView{{Option: SlotProposalOption{ID: "o1", StartsAtUTC: time.Now().UTC(), EndsAtUTC: time.Now().UTC().Add(time.Hour)}}}
	h.sendProposalEmail(context.Background(), "c1", `<b>Evil Campaign</b>`, proposal, options, campaigns.CampaignMember{UserID: "u1", Email: "a@b.test"})

	if strings.Contains(mailer.lastHTML, "<script>alert(1)</script>") {
		t.Error("raw proposal title leaked into the email HTML (XSS)")
	}
	if !strings.Contains(mailer.lastHTML, "&lt;script&gt;") {
		t.Error("proposal title should be HTML-escaped")
	}
	if strings.Contains(mailer.lastHTML, "<b>Evil Campaign</b>") {
		t.Error("raw campaign name leaked into the email HTML (XSS)")
	}
}

func TestRSVPEmail_EscapesSessionName(t *testing.T) {
	mailer := &captureMailer{}
	repo := &mockSessionRepo{
		createRSVPTokenFn: func(_ context.Context, _ *RSVPToken) error { return nil },
	}
	h := &Handler{svc: NewSessionService(repo, nil), mailer: mailer, baseURL: "https://x.test"}

	date := "2026-07-18"
	session := &Session{ID: "s1", Name: `<img src=x onerror=alert(1)>`, ScheduledDate: &date}
	h.sendRSVPEmails(context.Background(), session, `<b>Camp</b>`, []campaigns.CampaignMember{{UserID: "u1", Email: "a@b.test"}})

	if strings.Contains(mailer.lastHTML, "<img src=x onerror=alert(1)>") {
		t.Error("raw session name leaked into the RSVP email HTML (XSS)")
	}
	if !strings.Contains(mailer.lastHTML, "&lt;img") {
		t.Error("session name should be HTML-escaped")
	}
}
