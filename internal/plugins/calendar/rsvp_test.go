package calendar

// Event RSVP tests (C-CAL-RSVP-P1).
//
// Hand-written struct-of-function-fields mocks, standard-library `testing`,
// table-driven where the cases are uniform — the house convention (no testify,
// no sqlmock: nothing in this repo executes real SQL).
//
// The security-shaped behaviours get their own named tests rather than table
// rows, because each is a distinct claim a reviewer should be able to find:
// visibility gating, self-write-only, single-use tokens, GET-never-applies, and
// "suggest another time" NOT minting a slot proposal.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- mocks ---

type mockRSVPRepo struct {
	upsertFn      func(ctx context.Context, r *EventRSVP) error
	setNoteFn     func(ctx context.Context, eventID, userID, note string) error
	getUserFn     func(ctx context.Context, eventID, userID string) (*EventRSVP, error)
	listFn        func(ctx context.Context, eventID string) ([]EventRSVP, error)
	setCollectFn  func(ctx context.Context, eventID string, enabled bool) error
	createTokenFn func(ctx context.Context, t *EventRSVPToken) error
	findTokenFn   func(ctx context.Context, token string) (*EventRSVPToken, error)
	markUsedFn    func(ctx context.Context, token string) error
	// Schedule-ask send bookkeeping (C-CALV4-RSVP-P8B).
	recordAskFn func(ctx context.Context, a *ScheduleAsk) error
	lastAskFn   func(ctx context.Context, campaignID string) (time.Time, error)
	recentAskFn func(ctx context.Context, campaignID string, since time.Time) ([]string, error)
}

func (m *mockRSVPRepo) UpsertRSVP(ctx context.Context, r *EventRSVP) error {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, r)
	}
	return nil
}
func (m *mockRSVPRepo) SetRSVPNote(ctx context.Context, eventID, userID, note string) error {
	if m.setNoteFn != nil {
		return m.setNoteFn(ctx, eventID, userID, note)
	}
	return nil
}
func (m *mockRSVPRepo) GetUserRSVP(ctx context.Context, eventID, userID string) (*EventRSVP, error) {
	if m.getUserFn != nil {
		return m.getUserFn(ctx, eventID, userID)
	}
	return nil, nil
}
func (m *mockRSVPRepo) ListRSVPsForEvent(ctx context.Context, eventID string) ([]EventRSVP, error) {
	if m.listFn != nil {
		return m.listFn(ctx, eventID)
	}
	return nil, nil
}
func (m *mockRSVPRepo) SetCollectRSVPs(ctx context.Context, eventID string, enabled bool) error {
	if m.setCollectFn != nil {
		return m.setCollectFn(ctx, eventID, enabled)
	}
	return nil
}
func (m *mockRSVPRepo) CreateRSVPToken(ctx context.Context, t *EventRSVPToken) error {
	if m.createTokenFn != nil {
		return m.createTokenFn(ctx, t)
	}
	return nil
}
func (m *mockRSVPRepo) FindRSVPToken(ctx context.Context, token string) (*EventRSVPToken, error) {
	if m.findTokenFn != nil {
		return m.findTokenFn(ctx, token)
	}
	return nil, nil
}
func (m *mockRSVPRepo) MarkRSVPTokenUsed(ctx context.Context, token string) error {
	if m.markUsedFn != nil {
		return m.markUsedFn(ctx, token)
	}
	return nil
}
func (m *mockRSVPRepo) RecordScheduleAsk(ctx context.Context, a *ScheduleAsk) error {
	if m.recordAskFn != nil {
		return m.recordAskFn(ctx, a)
	}
	return nil
}

// LastScheduleAskAt defaults to the zero time — "nobody has ever been asked" —
// so every pre-existing fixture behaves as a campaign with no send history.
func (m *mockRSVPRepo) LastScheduleAskAt(ctx context.Context, campaignID string) (time.Time, error) {
	if m.lastAskFn != nil {
		return m.lastAskFn(ctx, campaignID)
	}
	return time.Time{}, nil
}
func (m *mockRSVPRepo) ScheduleAskRecipientsSince(ctx context.Context, campaignID string, since time.Time) ([]string, error) {
	if m.recentAskFn != nil {
		return m.recentAskFn(ctx, campaignID, since)
	}
	return nil, nil
}

type mockEventLookup struct {
	evt *Event
	cal *Calendar
}

func (m *mockEventLookup) GetEvent(_ context.Context, _ string) (*Event, error) { return m.evt, nil }
func (m *mockEventLookup) GetByID(_ context.Context, _ string) (*Calendar, error) {
	return m.cal, nil
}

type mockMemberDir struct {
	members []campaigns.CampaignMember
	dmGrant map[string]bool
	err     error
}

func (m *mockMemberDir) ListMembers(_ context.Context, _ string) ([]campaigns.CampaignMember, error) {
	return m.members, m.err
}
func (m *mockMemberDir) IsUserDmGranted(_ context.Context, _, userID string) (bool, error) {
	return m.dmGrant[userID], nil
}

type mockMailer struct {
	configured bool
	sent       []string // one entry per recipient address
	bodies     []string
}

func (m *mockMailer) IsConfigured(_ context.Context) bool { return m.configured }
func (m *mockMailer) SendHTMLMail(_ context.Context, to []string, _, _, htmlBody string) error {
	m.sent = append(m.sent, to...)
	m.bodies = append(m.bodies, htmlBody)
	return nil
}

type mockNotifier struct {
	userIDs  []string
	messages []string
}

func (m *mockNotifier) NotifyRSVP(_ context.Context, userIDs []string, _, message, _ string) error {
	m.userIDs = append(m.userIDs, userIDs...)
	m.messages = append(m.messages, message)
	return nil
}

type mockAvailability struct {
	existing []string
	written  []string
	forUser  string
	offered  []RSVPAvailabilityWindow
	offerErr error
}

func (m *mockAvailability) OfferAvailableWindows(_ context.Context, _, userID string, w []RSVPAvailabilityWindow) error {
	if m.offerErr != nil {
		return m.offerErr
	}
	m.forUser = userID
	m.offered = append(m.offered, w...)
	return nil
}

func (m *mockAvailability) ExceptionDates(_ context.Context, _, _ string) ([]string, error) {
	return m.existing, nil
}
func (m *mockAvailability) MarkDaysUnavailable(_ context.Context, _, userID string, dates []string) error {
	m.forUser = userID
	m.written = append(m.written, dates...)
	return nil
}

// --- fixtures ---

func testEvent(mut func(*Event)) *Event {
	owner := "owner-1"
	e := &Event{
		ID: "evt-1", CalendarID: "cal-1", Name: "Harvest Feast",
		Year: 2026, Month: 7, Day: 27,
		Visibility: "everyone", CollectRSVPs: true, CreatedBy: &owner,
	}
	if mut != nil {
		mut(e)
	}
	return e
}

func testCalendar(mut func(*Calendar)) *Calendar {
	c := &Calendar{
		ID: "cal-1", CampaignID: "camp-1", Name: "Harptos",
		Months: []Month{{Name: "Flamerule", SortOrder: 0}, {Name: "Eleasis", SortOrder: 1},
			{Name: "Eleint", SortOrder: 2}, {Name: "Marpenoth", SortOrder: 3},
			{Name: "Uktar", SortOrder: 4}, {Name: "Nightal", SortOrder: 5},
			{Name: "Hammer", SortOrder: 6}},
	}
	if mut != nil {
		mut(c)
	}
	return c
}

func newTestRSVPService(repo RSVPRepository, evt *Event, cal *Calendar) RSVPService {
	return NewRSVPService(repo, &mockEventLookup{evt: evt, cal: cal})
}

// --- visibility + opt-in gating ---

func TestSetMyRSVP_Gating(t *testing.T) {
	dmOnly := testEvent(func(e *Event) { e.Visibility = "dm_only" })
	closed := testEvent(func(e *Event) { e.CollectRSVPs = false })

	tests := []struct {
		name    string
		evt     *Event
		role    int
		status  string
		wantErr bool
	}{
		{"player answers an everyone event", testEvent(nil), int(campaigns.RolePlayer), RSVPYes, false},
		{"player cannot answer a dm_only event", dmOnly, int(campaigns.RolePlayer), RSVPYes, true},
		{"scribe cannot answer a dm_only event either", dmOnly, int(campaigns.RoleScribe), RSVPYes, true},
		{"owner can answer a dm_only event", dmOnly, int(campaigns.RoleOwner), RSVPYes, false},
		{"collection off is rejected", closed, int(campaigns.RolePlayer), RSVPYes, true},
		{"bogus status is rejected", testEvent(nil), int(campaigns.RolePlayer), "sure", true},
		{"empty status is rejected", testEvent(nil), int(campaigns.RolePlayer), "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestRSVPService(&mockRSVPRepo{}, tt.evt, testCalendar(nil))
			err := svc.SetMyRSVP(context.Background(), tt.evt, "u1", tt.role, SetRSVPRequest{Status: tt.status})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestSetMyRSVP_WritesOnlyCallerRow pins that the row written carries the userID
// the SERVICE was handed — there is no field on the request that can redirect it.
func TestSetMyRSVP_WritesOnlyCallerRow(t *testing.T) {
	var got *EventRSVP
	repo := &mockRSVPRepo{upsertFn: func(_ context.Context, r *EventRSVP) error { got = r; return nil }}
	svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))

	if err := svc.SetMyRSVP(context.Background(), testEvent(nil), "player-9", int(campaigns.RolePlayer),
		SetRSVPRequest{Status: RSVPMaybe}); err != nil {
		t.Fatalf("SetMyRSVP: %v", err)
	}
	if got == nil || got.UserID != "player-9" {
		t.Fatalf("expected the row to be written for player-9, got %+v", got)
	}
	if got.Status != RSVPMaybe {
		t.Errorf("status = %q, want %q", got.Status, RSVPMaybe)
	}
}

func TestNormalizeRSVPNote(t *testing.T) {
	long := strings.Repeat("x", maxRSVPNoteLen+1)
	blank := "   "
	ok := "  any evening after 8  "

	if n, err := normalizeRSVPNote(nil); err != nil || n != nil {
		t.Errorf("nil note: got (%v, %v), want (nil, nil)", n, err)
	}
	if n, err := normalizeRSVPNote(&blank); err != nil || n != nil {
		t.Errorf("blank note must normalize to nil so it never blanks a stored note; got (%v, %v)", n, err)
	}
	if n, err := normalizeRSVPNote(&ok); err != nil || n == nil || *n != "any evening after 8" {
		t.Errorf("note should be trimmed; got (%v, %v)", n, err)
	}
	if _, err := normalizeRSVPNote(&long); err == nil {
		t.Errorf("a note over %d runes must be rejected", maxRSVPNoteLen)
	}
}

// --- counts + detail gating ---

func TestSummary_CountsForAll_DetailForDirectorOnly(t *testing.T) {
	note := "try Sundays"
	repo := &mockRSVPRepo{listFn: func(_ context.Context, _ string) ([]EventRSVP, error) {
		return []EventRSVP{
			{UserID: "u1", Status: RSVPYes},
			{UserID: "u2", Status: RSVPYes},
			{UserID: "u3", Status: RSVPMaybe, Note: &note},
			{UserID: "u4", Status: RSVPNo},
		}, nil
	}}
	svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))
	evt := testEvent(nil)

	player, err := svc.Summary(context.Background(), evt, "u3", int(campaigns.RolePlayer), false)
	if err != nil {
		t.Fatalf("Summary(player): %v", err)
	}
	if player.Counts.Yes != 2 || player.Counts.Maybe != 1 || player.Counts.No != 1 {
		t.Errorf("counts = %+v, want 2/1/1", player.Counts)
	}
	if len(player.Responders) != 0 {
		t.Errorf("a Player must never receive the per-person breakdown; got %d responders", len(player.Responders))
	}
	// A member always sees their OWN answer — it is their own data.
	if player.MyStatus != RSVPMaybe || player.MyNote == nil || *player.MyNote != note {
		t.Errorf("caller's own answer must round-trip; got status=%q note=%v", player.MyStatus, player.MyNote)
	}

	director, err := svc.Summary(context.Background(), evt, "owner-1", int(campaigns.RoleOwner), true)
	if err != nil {
		t.Fatalf("Summary(director): %v", err)
	}
	if len(director.Responders) != 4 {
		t.Errorf("Owner/co-DM must receive the per-person breakdown; got %d responders", len(director.Responders))
	}
}

func TestSummary_HiddenEventIsNotFound(t *testing.T) {
	evt := testEvent(func(e *Event) { e.Visibility = "dm_only" })
	svc := newTestRSVPService(&mockRSVPRepo{}, evt, testCalendar(nil))
	if _, err := svc.Summary(context.Background(), evt, "u1", int(campaigns.RolePlayer), false); err == nil {
		t.Error("a Player reading RSVPs on a dm_only event must be refused (and told only 'not found')")
	}
}

// --- tokens ---

func futureToken(action string) *EventRSVPToken {
	return &EventRSVPToken{
		Token: "tok", EventID: "evt-1", UserID: "u1", Action: action,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
}

func TestValidateToken_RejectsSpentAndExpired(t *testing.T) {
	used := time.Now().UTC().Add(-time.Minute)
	tests := []struct {
		name string
		tok  *EventRSVPToken
	}{
		{"unknown token", nil},
		{"already used", &EventRSVPToken{Token: "tok", UsedAt: &used, ExpiresAt: time.Now().UTC().Add(time.Hour)}},
		{"expired", &EventRSVPToken{Token: "tok", ExpiresAt: time.Now().UTC().Add(-time.Hour)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRSVPRepo{findTokenFn: func(_ context.Context, _ string) (*EventRSVPToken, error) {
				return tt.tok, nil
			}}
			svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))
			_, err := svc.ValidateToken(context.Background(), "tok")
			if err == nil {
				t.Fatal("expected rejection")
			}
			// Every failure mode must be indistinguishable to an unauthenticated
			// caller, or the route becomes an oracle for token existence.
			if !strings.Contains(err.Error(), rsvpBadTokenMsg) {
				t.Errorf("error must be the generic message %q; got %q", rsvpBadTokenMsg, err.Error())
			}
		})
	}
}

// TestApplyToken_ConsumesBeforeWriting pins single-use: when the atomic
// "consume if unused" UPDATE loses the race, nothing is written.
func TestApplyToken_ConsumesBeforeWriting(t *testing.T) {
	wrote := false
	repo := &mockRSVPRepo{
		findTokenFn: func(_ context.Context, _ string) (*EventRSVPToken, error) { return futureToken(RSVPActionYes), nil },
		markUsedFn:  func(_ context.Context, _ string) error { return errRSVPTokenSpent },
		upsertFn:    func(_ context.Context, _ *EventRSVP) error { wrote = true; return nil },
	}
	svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))

	if _, err := svc.ApplyToken(context.Background(), "tok"); err == nil {
		t.Fatal("a token that lost the consume race must be refused")
	}
	if wrote {
		t.Error("no RSVP may be written when the token was already spent")
	}
}

func TestApplyToken_ActionMapping(t *testing.T) {
	tests := []struct {
		action     string
		wantStatus string
		wantNote   bool
	}{
		{RSVPActionYes, RSVPYes, false},
		{RSVPActionMaybe, RSVPMaybe, false},
		{RSVPActionNo, RSVPNo, false},
		{RSVPActionOutWeek, RSVPNo, false}, // "out this week" is a decline...
		// ...but "suggest" carries NO status and no note write at this layer: the
		// note + windows arrive with the POST, so the handler owns that effect.
		{RSVPActionSuggest, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			var gotStatus string
			gotNote := false
			repo := &mockRSVPRepo{
				findTokenFn: func(_ context.Context, _ string) (*EventRSVPToken, error) { return futureToken(tt.action), nil },
				upsertFn:    func(_ context.Context, r *EventRSVP) error { gotStatus = r.Status; return nil },
				setNoteFn:   func(_ context.Context, _, _, _ string) error { gotNote = true; return nil },
			}
			svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))
			if _, err := svc.ApplyToken(context.Background(), "tok"); err != nil {
				t.Fatalf("ApplyToken(%s): %v", tt.action, err)
			}
			if gotStatus != tt.wantStatus {
				t.Errorf("status written = %q, want %q", gotStatus, tt.wantStatus)
			}
			if gotNote != tt.wantNote {
				t.Errorf("note written = %v, want %v", gotNote, tt.wantNote)
			}
		})
	}
}

func TestMintActionTokens_OnePerActionSingleUse(t *testing.T) {
	var created []*EventRSVPToken
	repo := &mockRSVPRepo{createTokenFn: func(_ context.Context, tok *EventRSVPToken) error {
		created = append(created, tok)
		return nil
	}}
	svc := newTestRSVPService(repo, testEvent(nil), testCalendar(nil))

	tokens, err := svc.MintActionTokens(context.Background(), "evt-1", "u1")
	if err != nil {
		t.Fatalf("MintActionTokens: %v", err)
	}
	if len(tokens) != len(rsvpEmailActions) {
		t.Fatalf("expected one token per action (%d), got %d", len(rsvpEmailActions), len(tokens))
	}
	seen := map[string]bool{}
	for _, tok := range created {
		if len(tok.Token) != 64 {
			t.Errorf("token width = %d, want 64 (the CHAR(64) column)", len(tok.Token))
		}
		if seen[tok.Token] {
			t.Error("tokens must be unique across actions")
		}
		seen[tok.Token] = true
		if d := time.Until(tok.ExpiresAt); d > rsvpTokenTTL+time.Minute || d < rsvpTokenTTL-time.Minute {
			t.Errorf("token TTL = %v, want ~%v", d, rsvpTokenTTL)
		}
	}
}

// --- "out this week" week resolution ---

func TestRSVPWeekDates(t *testing.T) {
	// 2026-07-27 is a Monday; 2026-07-29 a Wednesday.
	now := time.Date(2026, 7, 29, 15, 4, 0, 0, time.UTC)

	realTime := testCalendar(func(c *Calendar) {
		c.Mode = ModeRealLife
		c.TracksRealTime = true
	})
	// A real-time calendar's Y/M/D IS the Gregorian date, so the event's own
	// week is derivable — here, an event two weeks out.
	evtFuture := testEvent(func(e *Event) { e.Year, e.Month, e.Day = 2026, 8, 12 }) // Wed 2026-08-12

	monday, dates := rsvpWeekDates(realTime, evtFuture, now)
	if monday != "2026-08-10" {
		t.Errorf("real-time calendar must use the EVENT's week; monday = %q, want 2026-08-10", monday)
	}
	if len(dates) != 7 || dates[0] != "2026-08-10" || dates[6] != "2026-08-16" {
		t.Errorf("week = %v, want Mon 2026-08-10 .. Sun 2026-08-16", dates)
	}

	// A fantasy calendar has no real-world date, so the fallback is the week
	// containing the redemption moment — the scheduler's own "out this week"
	// meaning. The user is always TOLD which week, so this can't silently
	// block the wrong one.
	fantasyMonday, fantasyDates := rsvpWeekDates(testCalendar(nil), evtFuture, now)
	if fantasyMonday != "2026-07-27" {
		t.Errorf("fantasy calendar must fall back to the current week; monday = %q, want 2026-07-27", fantasyMonday)
	}
	if len(fantasyDates) != 7 || fantasyDates[6] != "2026-08-02" {
		t.Errorf("fallback week = %v, want Mon 2026-07-27 .. Sun 2026-08-02", fantasyDates)
	}
}

// TestApplyOutThisWeek_SkipsHandAuthoredDays mirrors the client-side rule in
// static/js/availability.js fireOutWeek: a date that already carries ANY
// exception is left completely alone.
func TestApplyOutThisWeek_SkipsHandAuthoredDays(t *testing.T) {
	restore := nowUTC
	nowUTC = func() time.Time { return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC) }
	defer func() { nowUTC = restore }()

	avail := &mockAvailability{existing: []string{"2026-07-29", "2026-08-01"}}
	h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, testEvent(nil), testCalendar(nil)))
	h.SetAvailabilityWriter(avail)

	msg := h.applyOutThisWeek(context.Background(), testCalendar(nil), testEvent(nil), "player-9")

	if avail.forUser != "player-9" {
		t.Errorf("availability must be written for the ACTING user only; got %q", avail.forUser)
	}
	if len(avail.written) != 5 {
		t.Fatalf("expected 5 of 7 days written (2 already customised); got %d: %v", len(avail.written), avail.written)
	}
	for _, d := range avail.written {
		if d == "2026-07-29" || d == "2026-08-01" {
			t.Errorf("hand-authored day %s must be left untouched", d)
		}
	}
	if !strings.Contains(msg, "2026-07-27") {
		t.Errorf("the resolved week must be named back to the member; got %q", msg)
	}
	if !strings.Contains(msg, "left alone") {
		t.Errorf("skipped days must be reported; got %q", msg)
	}
}

func TestApplyOutThisWeek_NilWriterDegradesGracefully(t *testing.T) {
	h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, testEvent(nil), testCalendar(nil)))
	msg := h.applyOutThisWeek(context.Background(), testCalendar(nil), testEvent(nil), "u1")
	if msg == "" || !strings.Contains(msg, "not attending") {
		t.Errorf("with no scheduler wired the RSVP must still be reported as recorded; got %q", msg)
	}
}

// --- public token routes ---

func newTokenHandler(t *testing.T, action string, repo *mockRSVPRepo) (*RSVPHandler, *mockNotifier) {
	t.Helper()
	if repo.findTokenFn == nil {
		repo.findTokenFn = func(_ context.Context, _ string) (*EventRSVPToken, error) { return futureToken(action), nil }
	}
	h := NewRSVPHandler(newTestRSVPService(repo, testEvent(nil), testCalendar(nil)))
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "u1", Role: campaigns.RolePlayer, DisplayName: "Ari", Email: "ari@example.test"},
	}})
	n := &mockNotifier{}
	h.SetRSVPNotifier(n)
	return h, n
}

func serveToken(h *RSVPHandler, method, tokenStr string, form string) *httptest.ResponseRecorder {
	e := echo.New()
	var req *http.Request
	if method == http.MethodPost {
		req = httptest.NewRequest(method, "/calendar-rsvp/"+tokenStr, strings.NewReader(form))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	} else {
		req = httptest.NewRequest(method, "/calendar-rsvp/"+tokenStr, nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("token")
	c.SetParamValues(tokenStr)
	if method == http.MethodPost {
		_ = h.ApplyEventRSVPToken(c)
	} else {
		_ = h.RedeemEventRSVPToken(c)
	}
	return rec
}

// TestToken_GetDoesNotApply is the anti-prefetch pin: a mail scanner issuing a
// GET must record nothing. Only the POST writes.
func TestToken_GetDoesNotApply(t *testing.T) {
	wrote := false
	repo := &mockRSVPRepo{upsertFn: func(_ context.Context, _ *EventRSVP) error { wrote = true; return nil }}
	h, _ := newTokenHandler(t, RSVPActionYes, repo)

	rec := serveToken(h, http.MethodGet, "tok", "")
	if wrote {
		t.Fatal("GET on the token route must NOT record an RSVP (mail-scanner defence)")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<form method=\"POST\"") {
		t.Error("GET must render a POST confirm form")
	}
	if !strings.Contains(body, `name="csrf_token"`) {
		t.Error("the confirm form must carry the CSRF double-submit hidden field")
	}

	rec2 := serveToken(h, http.MethodPost, "tok", "")
	if !wrote {
		t.Error("POST must record the RSVP")
	}
	if !strings.Contains(rec2.Body.String(), "Response recorded") {
		t.Errorf("POST should confirm success; body = %q", rec2.Body.String())
	}
}

// TestToken_NonMemberRefused pins fail-closed re-checking at redemption: a
// recipient who has since left the campaign gets the generic invalid-link page,
// which also never discloses the event's title.
func TestToken_NonMemberRefused(t *testing.T) {
	repo := &mockRSVPRepo{}
	h, _ := newTokenHandler(t, RSVPActionYes, repo)
	h.SetMemberDirectory(&mockMemberDir{members: nil}) // no longer a member

	rec := serveToken(h, http.MethodGet, "tok", "")
	body := rec.Body.String()
	if strings.Contains(body, "Harvest Feast") {
		t.Error("the event title must not leak to a non-member")
	}
	if !strings.Contains(body, "invalid or has expired") {
		t.Errorf("expected the generic invalid-link page; got %q", body)
	}
}

// TestToken_SuggestWritesNoteAndNotifiesOwner pins the explicitly REJECTED
// branch: suggesting a time must not create a slot proposal (proposal creation
// is Scribe+ by ruling), only a note plus an owner notification.
func TestToken_SuggestWritesNoteAndNotifiesOwner(t *testing.T) {
	gotNote := ""
	upserted := false
	repo := &mockRSVPRepo{
		setNoteFn: func(_ context.Context, _, _, note string) error { gotNote = note; return nil },
		upsertFn:  func(_ context.Context, _ *EventRSVP) error { upserted = true; return nil },
	}
	h, notifier := newTokenHandler(t, RSVPActionSuggest, repo)

	rec := serveToken(h, http.MethodPost, "tok", "note=Sundays+after+4")
	if gotNote != "Sundays after 4" {
		t.Errorf("note = %q, want %q", gotNote, "Sundays after 4")
	}
	if upserted {
		t.Error("suggesting a time must not overwrite the member's status")
	}
	if len(notifier.userIDs) != 1 || notifier.userIDs[0] != "owner-1" {
		t.Errorf("the event owner must be notified; got %v", notifier.userIDs)
	}
	if !strings.Contains(notifier.messages[0], "Sundays after 4") {
		t.Errorf("the notification should carry the suggestion; got %q", notifier.messages[0])
	}
	if !strings.Contains(rec.Body.String(), "sent to the organiser") {
		t.Errorf("the member should be told it was sent; body = %q", rec.Body.String())
	}
}

func TestToken_EmptySuggestionRejected(t *testing.T) {
	repo := &mockRSVPRepo{}
	h, _ := newTokenHandler(t, RSVPActionSuggest, repo)
	rec := serveToken(h, http.MethodPost, "tok", "note=")
	if !strings.Contains(rec.Body.String(), "RSVP Failed") {
		t.Errorf("an empty suggestion with no times must be refused; body = %q", rec.Body.String())
	}
}

// --- temporary offered availability (C-CAL-RSVP-P2) ---

// TestToken_SuggestWithWindowsWritesAvailability is the point of the feature: a
// member who can't make the proposed time offers real windows from the EMAIL,
// and those become schedulable availability rather than prose.
func TestToken_SuggestWithWindowsWritesAvailability(t *testing.T) {
	gotNote := ""
	repo := &mockRSVPRepo{setNoteFn: func(_ context.Context, _, _, note string) error { gotNote = note; return nil }}
	h, notifier := newTokenHandler(t, RSVPActionSuggest, repo)
	avail := &mockAvailability{}
	h.SetAvailabilityWriter(avail)

	// Row 0 complete; row 1 half-filled (no end time); row 2 empty.
	form := "w0date=2026-08-05&w0from=18%3A00&w0to=22%3A30" +
		"&w1date=2026-08-07&w1from=19%3A00" +
		"&note="
	rec := serveToken(h, http.MethodPost, "tok", form)

	if len(avail.offered) != 1 {
		t.Fatalf("expected exactly the one complete row to be offered; got %+v", avail.offered)
	}
	w := avail.offered[0]
	if w.OnDate != "2026-08-05" || w.StartMinute != 18*60 || w.EndMinute != 22*60+30 {
		t.Errorf("window parsed wrong: %+v", w)
	}
	if avail.forUser != "u1" {
		t.Errorf("availability must be written for the token's own user; got %q", avail.forUser)
	}
	// With no note supplied, one is synthesised so the Director's response list
	// shows the offer rather than a blank.
	if !strings.Contains(gotNote, "2026-08-05 18:00–22:30") {
		t.Errorf("note should carry the offered window; got %q", gotNote)
	}
	if len(notifier.messages) != 1 || !strings.Contains(notifier.messages[0], "18:00") {
		t.Errorf("owner notification should carry the offered time; got %v", notifier.messages)
	}
	if !strings.Contains(rec.Body.String(), "added to your availability") {
		t.Errorf("the member should be told their times were saved; body = %q", rec.Body.String())
	}
}

func TestToken_SuggestWindowsMalformedRowsDropped(t *testing.T) {
	repo := &mockRSVPRepo{}
	h, _ := newTokenHandler(t, RSVPActionSuggest, repo)
	avail := &mockAvailability{}
	h.SetAvailabilityWriter(avail)

	// Backwards range, junk time, and a missing date — all skipped; the note
	// still carries the submission through.
	form := "w0date=2026-08-05&w0from=22%3A00&w0to=18%3A00" +
		"&w1date=2026-08-06&w1from=notatime&w1to=20%3A00" +
		"&w2from=18%3A00&w2to=20%3A00" +
		"&note=any+evening+really"
	rec := serveToken(h, http.MethodPost, "tok", form)

	if len(avail.offered) != 0 {
		t.Errorf("no malformed row should be written; got %+v", avail.offered)
	}
	if !strings.Contains(rec.Body.String(), "sent to the organiser") {
		t.Errorf("the note must still go through; body = %q", rec.Body.String())
	}
}

// TestSuggestOffer_AvailabilityFailureStillRecordsAnswer pins that the RSVP note
// and the owner notification — the promise this flow makes — survive a scheduler
// that is absent or erroring.
func TestSuggestOffer_AvailabilityFailureStillRecordsAnswer(t *testing.T) {
	for _, tc := range []struct {
		name  string
		avail *mockAvailability
		want  string
	}{
		{"no scheduler wired", nil, "set up on this instance"},
		{"scheduler errors", &mockAvailability{offerErr: errRSVPTokenSpent}, "be added to your availability"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			noted := false
			repo := &mockRSVPRepo{setNoteFn: func(_ context.Context, _, _, _ string) error { noted = true; return nil }}
			h, notifier := newTokenHandler(t, RSVPActionSuggest, repo)
			if tc.avail != nil {
				h.SetAvailabilityWriter(tc.avail)
			}
			rec := serveToken(h, http.MethodPost, "tok", "w0date=2026-08-05&w0from=18%3A00&w0to=22%3A00&note=")
			if !noted {
				t.Error("the RSVP note must still be recorded")
			}
			if len(notifier.messages) != 1 {
				t.Error("the owner must still be notified")
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("member should be told the times weren't saved; body = %q", rec.Body.String())
			}
		})
	}
}

func TestParseHHMM(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		{"18:00", 1080, true},
		{"00:00", 0, true},
		{"23:59", 1439, true},
		{"09:05:00", 545, true}, // browsers that render seconds
		{"24:00", 0, false},
		{"18:60", 0, false},
		{"18", 0, false},
		{"", 0, false},
		{"aa:bb", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseHHMM(tt.in)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("parseHHMM(%q) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestApplySuggestion_RejectsEmptyAndOverLongOffers(t *testing.T) {
	h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, testEvent(nil), testCalendar(nil)))
	h.SetMemberDirectory(&mockMemberDir{})

	if _, err := h.applySuggestion(context.Background(), "camp-1", testEvent(nil), "u1",
		int(campaigns.RolePlayer), "   ", nil); err == nil {
		t.Error("a blank note with no windows must be refused")
	}

	many := make([]RSVPAvailabilityWindow, maxRSVPWindows+1)
	if _, err := h.applySuggestion(context.Background(), "camp-1", testEvent(nil), "u1",
		int(campaigns.RolePlayer), "", many); err == nil {
		t.Errorf("more than %d windows must be refused", maxRSVPWindows)
	}
}

func TestToken_CollectionOffRefused(t *testing.T) {
	repo := &mockRSVPRepo{}
	h := NewRSVPHandler(newTestRSVPService(repo,
		testEvent(func(e *Event) { e.CollectRSVPs = false }), testCalendar(nil)))
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{{UserID: "u1", Role: campaigns.RolePlayer}}})
	repo.findTokenFn = func(_ context.Context, _ string) (*EventRSVPToken, error) { return futureToken(RSVPActionYes), nil }

	rec := serveToken(h, http.MethodGet, "tok", "")
	if !strings.Contains(rec.Body.String(), "no longer collecting") {
		t.Errorf("a link for an event that stopped collecting must be refused; body = %q", rec.Body.String())
	}
}

// --- the Bench write path (C-CALV4-RSVP-P8 §8, WG-6) ------------------------
//
// ZERO NEW ROUTES. The Bench's session tile posts, form-encoded, to the
// EXISTING Player+ endpoint, and the handler content-negotiates: an HTMX
// request gets the tile back, everything else gets the unchanged JSON. These
// tests are §8.3's security review in executable form — each heading it names
// is one assertion below, so a reviewer can read the claim and the proof
// together.

// serveRSVPWrite drives SetEventRSVPAPI the way a browser would.
func serveRSVPWrite(h *RSVPHandler, body string, contentType string, htmx bool,
	role campaigns.Role, userID string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost,
		"/campaigns/camp-1/calendars/cal-1/events/evt-1/rsvp", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, contentType)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId", "eid")
	c.SetParamValues("camp-1", "cal-1", "evt-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsMember: true,
	})
	c.Set("auth_user_id", userID)
	return rec, h.SetEventRSVPAPI(c)
}

func newBenchWriteHandler(repo RSVPRepository) *RSVPHandler {
	h := NewRSVPHandler(newTestRSVPService(repo, testEvent(nil), testCalendar(nil)))
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "u1", Role: campaigns.RolePlayer, DisplayName: "Ari"},
		{UserID: "u2", Role: campaigns.RolePlayer, DisplayName: "Bo"},
		{UserID: "u3", Role: campaigns.RoleOwner, DisplayName: "Cass"},
	}})
	return h
}

// §8.3 item 2 — WHO MAY CALL IT, and the part a route gate cannot express:
// THE USER ID COMES FROM THE SESSION, NEVER THE BODY. Proven by SUPPLYING one:
// the request carries user_id, userId AND user fields naming somebody else, and
// the row still lands on the authenticated caller.
func TestBenchWrite_BodySuppliedUserIDIsIgnored(t *testing.T) {
	for _, body := range []struct {
		name, payload, ctype string
	}{
		{"form", "status=yes&user_id=victim&userId=victim&UserID=victim", echo.MIMEApplicationForm},
		{"json", `{"status":"yes","user_id":"victim","userId":"victim","UserID":"victim"}`, echo.MIMEApplicationJSON},
	} {
		t.Run(body.name, func(t *testing.T) {
			var got *EventRSVP
			repo := &mockRSVPRepo{upsertFn: func(_ context.Context, r *EventRSVP) error { got = r; return nil }}
			h := newBenchWriteHandler(repo)

			if _, err := serveRSVPWrite(h, body.payload, body.ctype, false,
				campaigns.RolePlayer, "caller-1"); err != nil {
				t.Fatalf("write: %v", err)
			}
			if got == nil {
				t.Fatal("nothing was written")
			}
			if got.UserID != "caller-1" {
				t.Errorf("the row landed on %q — a body field redirected the write", got.UserID)
			}
		})
	}
}

// THE `form:` TAGS ARE THE POINT (ledger #20). Without them Echo binds a
// form-encoded body by capitalised field name, so `status=yes` binds to nothing
// and the ENUM gate rejects an empty status. This is the regression that would
// silently return the Bench's tile to a dead control.
func TestBenchWrite_FormEncodedBodyBinds(t *testing.T) {
	for _, status := range []string{RSVPYes, RSVPNo, RSVPMaybe} {
		var got *EventRSVP
		repo := &mockRSVPRepo{upsertFn: func(_ context.Context, r *EventRSVP) error { got = r; return nil }}
		h := newBenchWriteHandler(repo)

		if _, err := serveRSVPWrite(h, "status="+status+"&csrf_token=t",
			echo.MIMEApplicationForm, false, campaigns.RolePlayer, "u1"); err != nil {
			t.Fatalf("status %q: %v", status, err)
		}
		if got == nil || got.Status != status {
			t.Errorf("form status=%q bound to %+v — SetRSVPRequest needs its form tags", status, got)
		}
	}
}

// §8.3 item 1 — WHAT THE FRAGMENT EXPOSES. It must not be RICHER than the JSON
// it replaces, and a fragment is easier to over-fill because it looks like
// markup rather than data. The JSON returns the per-person breakdown to an
// Owner/co-DM; the fragment returns nobody's name at all.
func TestBenchWrite_HTMXFragmentIsPoorerThanTheJSON(t *testing.T) {
	repo := &mockRSVPRepo{listFn: func(_ context.Context, _ string) ([]EventRSVP, error) {
		return []EventRSVP{
			{UserID: "u1", Status: RSVPYes},
			{UserID: "u2", Status: RSVPNo},
			{UserID: "departed", Status: RSVPYes},
		}, nil
	}}
	h := newBenchWriteHandler(repo)

	rec, err := serveRSVPWrite(h, "status=yes", echo.MIMEApplicationForm, true,
		campaigns.RoleOwner, "u3")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-bench-tile="session"`) {
		t.Fatalf("an HTMX write must answer with the session tile; got %q", body)
	}
	// No names, no notes, no per-person breakdown — none of which the tile has
	// any use for and all of which the JSON response carries for this role.
	for _, leaked := range []string{"Ari", "Bo", "Cass", "u1", "u2", "departed", "responders"} {
		if strings.Contains(body, leaked) {
			t.Errorf("the fragment leaked %q — it must be poorer than the JSON, not richer", leaked)
		}
	}
	// And the tally is RECOMPUTED from the membership-filtered roster: the
	// departed row is stored, and it must not be in the count.
	if !strings.Contains(body, "2 of 3 answered") {
		t.Errorf("fragment tally is wrong — a departed member's stored row reached it: %q", body)
	}
}

// The JSON consumers are NOT retired. static/js/availability.js and the
// Foundry-facing /api/v1 group are separate consumers of this endpoint, and a
// non-HTMX write must still get the summary shape back.
func TestBenchWrite_NonHTMXStillGetsJSON(t *testing.T) {
	h := newBenchWriteHandler(&mockRSVPRepo{})
	rec, err := serveRSVPWrite(h, `{"status":"yes"}`, echo.MIMEApplicationJSON, false,
		campaigns.RolePlayer, "u1")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"event_id"`) || !strings.Contains(body, `"counts"`) {
		t.Errorf("a non-HTMX write must still return EventRSVPSummary; got %q", body)
	}
	if strings.Contains(body, "data-bench-tile") {
		t.Error("a non-HTMX write returned markup")
	}
}

// §8.3 item 4 — CSRF. The write is NOT under /api/ and NOT /ws, which are the
// only two prefixes internal/middleware/csrf.go skips, so the global
// double-submit guard covers it. Proven against the real middleware rather than
// asserted: a POST with no token is rejected before the handler runs.
func TestBenchWrite_TokenlessPostIsRejectedByCSRF(t *testing.T) {
	e := echo.New()
	guarded := middleware.CSRF()(func(c echo.Context) error {
		return c.String(http.StatusOK, "handler ran")
	})

	req := httptest.NewRequest(http.MethodPost,
		"/campaigns/camp-1/calendars/cal-1/events/evt-1/rsvp", strings.NewReader("status=yes"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	if err := guarded(e.NewContext(req, rec)); err == nil && rec.Code < 400 {
		t.Fatalf("a token-less POST to the RSVP path was allowed through (code %d, body %q)",
			rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "handler ran") {
		t.Error("the handler ran without a CSRF token")
	}
}
