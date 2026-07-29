// rsvp_ask_test.go — the schedule ask: the email, the fan-out, and §9's
// security review as executable assertions (C-CALV4-RSVP-P8B stage 3).
//
// The route's security section is written here rather than only in prose,
// because "who may call this" and "what may this reveal" are claims, and a
// claim about an endpoint that mails other people is worth exactly as much as
// its test.
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- fixtures ---------------------------------------------------------------

// askHandler builds an RSVPHandler with a roster, a mailer and a repo, wired
// exactly as internal/app/routes.go wires the real one.
func askHandler(t *testing.T, evt *Event, dir *mockMemberDir, mailer *mockMailer, repo *mockRSVPRepo) *RSVPHandler {
	t.Helper()
	h := NewRSVPHandler(newTestRSVPService(repo, evt, testCalendar(nil)))
	h.SetMemberDirectory(dir)
	if mailer != nil {
		h.SetMailSender(mailer, "https://chronicle.example.test")
	}
	return h
}

// askRoster is the standard five-member fixture: three reachable, one with no
// address on file, and one co-DM.
func askRoster() *mockMemberDir {
	return &mockMemberDir{
		members: []campaigns.CampaignMember{
			{UserID: "owner", Role: campaigns.RoleOwner, DisplayName: "Imix", Email: "owner@example.test"},
			{UserID: "player", Role: campaigns.RolePlayer, DisplayName: "Ari", Email: "ari@example.test"},
			{UserID: "codm", Role: campaigns.RolePlayer, DisplayName: "Bryn", Email: "bryn@example.test"},
			{UserID: "noaddr", Role: campaigns.RolePlayer, DisplayName: "Tam"},
		},
		dmGrant: map[string]bool{"codm": true},
	}
}

// callAsk invokes AskAvailabilityAPI directly with a populated campaign
// context, the way the middleware stack would leave it. form carries whatever
// the caller tries to submit.
func callAsk(t *testing.T, h *RSVPHandler, actorID string, form url.Values) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/calendar/ask",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Vale of Ash"},
		MemberRole: campaigns.RoleOwner,
		IsMember:   true,
	})
	c.Set("auth_user_id", actorID)
	return rec, h.AskAvailabilityAPI(c)
}

// askJSON decodes the endpoint's response body.
func askJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON (%v): %s", err, rec.Body.String())
	}
	return out
}

// drainSends waits for the background fan-out to finish its work. The send is
// deliberately backgrounded (a dead mail server must not turn a button into a
// timeout), so the tests have to wait for it rather than assume it ran.
func drainSends(t *testing.T, m *mockMailer, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		// attempts, not len(sent): a fan-out whose every send FAILS still has to
		// be waited for before the "no rows were written" assertion is honest.
		n := m.attempts
		m.mu.Unlock()
		if n >= want {
			// Give a stray extra send a moment to show up so an over-send fails.
			time.Sleep(30 * time.Millisecond)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t.Fatalf("background fan-out sent %d emails, want %d: %v", len(m.sent), want, m.sent)
}

// --- the email --------------------------------------------------------------

// TestScheduleAskEmail_AsksInTheOperatorsOwnTerms pins the content contract:
// who is asking, the three sentences about how the grid actually works, the
// deep link, and the "why you got this" line. No unsubscribe link is invented.
func TestScheduleAskEmail_AsksInTheOperatorsOwnTerms(t *testing.T) {
	h := askHandler(t, testEvent(nil), askRoster(), &mockMailer{configured: true}, &mockRSVPRepo{})
	subject, plain, htmlBody := h.renderScheduleAskEmail(scheduleAskEmailInput{
		CampaignID: "camp-1", CampaignName: "Vale of Ash", ActorName: "Imix",
	})

	if !strings.Contains(subject, "Vale of Ash") || !strings.Contains(subject, "When can you play") {
		t.Errorf("subject must name the campaign and ask the question; got %q", subject)
	}
	for _, body := range map[string]string{"plain": plain, "html": htmlBody} {
		for _, want := range []string{
			"Imix",                       // who is asking, resolved through the roster
			"Vale of Ash",                // what about
			"it repeats every week",      // normal hours, set once
			"mark that day on its own",   // the odd week off, separately
			"edit a single day directly", // "or they could just type everything in"
			"member of this campaign",    // why you got this
		} {
			if !strings.Contains(body, want) {
				t.Errorf("the %s body does not say %q", body, want)
			}
		}
		if !strings.Contains(body, "https://chronicle.example.test/campaigns/camp-1/availability") {
			t.Errorf("the %s body is missing the availability deep link", body)
		}
		// NO UNSUBSCRIBE IS INVENTED. Chronicle has no notification preferences
		// at all (booked as C-NOTIFY-PREFS); a dead link would be worse than
		// the honest sentence above.
		if strings.Contains(strings.ToLower(body), "unsubscribe") {
			t.Errorf("the %s body invented an unsubscribe link", body)
		}
	}
	// With no session attached there is no RSVP half and no promise about links.
	if strings.Contains(plain, "expires in 7 days") || strings.Contains(plain, "calendar-rsvp") {
		t.Error("a schedule ask with no session must carry no RSVP action links")
	}
}

// TestScheduleAskEmail_RSVPSectionRidesAlongWhenAttached is [PB-1](b): the same
// email additionally carries the five EXISTING action links, in the one ordered
// action set, with the same 7-day single-use promise the invite email prints.
func TestScheduleAskEmail_RSVPSectionRidesAlongWhenAttached(t *testing.T) {
	h := askHandler(t, testEvent(nil), askRoster(), &mockMailer{configured: true}, &mockRSVPRepo{})
	_, plain, htmlBody := h.renderScheduleAskEmail(scheduleAskEmailInput{
		CampaignID: "camp-1", CampaignName: "Vale of Ash", ActorName: "Imix",
		Event: testEvent(nil), Calendar: testCalendar(nil),
		Tokens: map[string]string{
			RSVPActionYes: "t1", RSVPActionMaybe: "t2", RSVPActionNo: "t3",
			RSVPActionOutWeek: "t4", RSVPActionSuggest: "t5",
		},
	})
	for _, tok := range []string{"t1", "t2", "t3", "t4", "t5"} {
		want := "https://chronicle.example.test/calendar-rsvp/" + tok
		if !strings.Contains(htmlBody, want) || !strings.Contains(plain, want) {
			t.Errorf("both bodies must carry action link %s", want)
		}
	}
	if !strings.Contains(plain, "expire in 7 days") {
		t.Error("the plain body must state the 7-day single-use promise")
	}
	if !strings.Contains(htmlBody, "expires in 7 days") {
		t.Error("the HTML body must state the 7-day single-use promise")
	}
	// The schedule ask still LEADS: it is the subject and the primary CTA.
	if strings.Index(plain, "availability") > strings.Index(plain, "calendar-rsvp") {
		t.Error("the schedule ask must lead; the RSVP section rides along beneath it")
	}
}

// TestScheduleAskEmail_EscapesOperatorText is the concatenation guard, run over
// BOTH bodies: a crafted campaign name, event name and display name are inert.
func TestScheduleAskEmail_EscapesOperatorText(t *testing.T) {
	evt := testEvent(func(e *Event) { e.Name = `Feast <img src=x onerror="alert(1)">` })
	h := askHandler(t, evt, askRoster(), &mockMailer{configured: true}, &mockRSVPRepo{})
	_, plain, htmlBody := h.renderScheduleAskEmail(scheduleAskEmailInput{
		CampaignID:   "camp-1",
		CampaignName: `Vale <img src=x onerror="alert(1)"> of "Ash"`,
		ActorName:    `<script>alert(2)</script>`,
		Event:        evt, Calendar: testCalendar(nil),
		Tokens: map[string]string{RSVPActionYes: "t1"},
	})
	// Assert on the DANGEROUS form: `onerror=` alone also matches the harmless
	// escaped rendering, so testing for it would pass a leak.
	for _, bad := range []string{"<img src=x", `onerror="alert`, "<script>"} {
		if strings.Contains(htmlBody, bad) {
			t.Errorf("the HTML body carries live markup %q", bad)
		}
	}
	if !strings.Contains(htmlBody, "&lt;img") || !strings.Contains(htmlBody, "&lt;script&gt;") {
		t.Error("expected the escaped forms in the HTML body")
	}
	// The plain body is not HTML, so it carries the raw text — that is correct
	// and is asserted so nobody "fixes" it into double-escaped noise.
	if !strings.Contains(plain, "<script>alert(2)</script>") {
		t.Error("the plain-text body should carry the raw text, unescaped")
	}
}

// --- the fan-out ------------------------------------------------------------

// TestScheduleAskFanOut_RSVPHalfIsPerRecipientVisibility is the leak pin: a
// dm_only event's TITLE must not reach a Player's inbox, even though the
// schedule ask itself goes to the whole roster. The Player still gets asked;
// they simply get the email without the session in it.
func TestScheduleAskFanOut_RSVPHalfIsPerRecipientVisibility(t *testing.T) {
	evt := testEvent(func(e *Event) { e.Visibility = "dm_only"; e.Name = "Secret Council" })
	dir := askRoster()
	mailer := &mockMailer{configured: true}
	h := askHandler(t, evt, dir, mailer, &mockRSVPRepo{})

	rec, err := callAsk(t, h, "owner", url.Values{"event_id": {"evt-1"}})
	if err != nil {
		t.Fatalf("AskAvailabilityAPI: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	drainSends(t, mailer, 3)

	mailer.mu.Lock()
	defer mailer.mu.Unlock()
	for i, to := range mailer.sent {
		hasTitle := strings.Contains(mailer.bodies[i], "Secret Council") ||
			strings.Contains(mailer.plains[i], "Secret Council")
		switch to {
		case "owner@example.test", "bryn@example.test": // owner + co-DM
			if !hasTitle {
				t.Errorf("%s may see the dm_only event and should have got the RSVP half", to)
			}
		case "ari@example.test": // a Player
			if hasTitle {
				t.Errorf("a dm_only event's title reached a Player's inbox (%s)", to)
			}
			// …but they were still ASKED. The audience is the whole roster.
			if !strings.Contains(mailer.plains[i], "availability") {
				t.Errorf("%s must still receive the schedule ask", to)
			}
		default:
			t.Errorf("unexpected recipient %s", to)
		}
	}
}

// TestScheduleAsk_CountsAreNumbersNotNames is §9.1 + §9.9: the response is a
// count, it names nobody, and a member with no address on file is counted
// rather than listed.
func TestScheduleAsk_CountsAreNumbersNotNames(t *testing.T) {
	mailer := &mockMailer{configured: true}
	h := askHandler(t, testEvent(nil), askRoster(), mailer, &mockRSVPRepo{})

	rec, err := callAsk(t, h, "owner", url.Values{})
	if err != nil {
		t.Fatalf("AskAvailabilityAPI: %v", err)
	}
	body := askJSON(t, rec)
	if body["asking"] != float64(3) {
		t.Errorf("asking = %v, want 3 (four members, one with no address)", body["asking"])
	}
	if body["no_email"] != float64(1) {
		t.Errorf("no_email = %v, want 1", body["no_email"])
	}
	raw := rec.Body.String()
	for _, identity := range []string{"Tam", "noaddr", "ari@example.test", "owner@example.test", "Ari"} {
		if strings.Contains(raw, identity) {
			t.Errorf("the response leaked an identity (%q): %s", identity, raw)
		}
	}
	if len(body) != 3 {
		t.Errorf("the response carries %d fields (%v); it is a count, a count and a sentence", len(body), body)
	}
	drainSends(t, mailer, 3)
}

// TestScheduleAsk_TwoSendsInsideTheCooldownProduceOneFanOut is §9.4's headline
// assertion, plus the shape of the refusal: a human sentence and a 429, never a
// bare code.
func TestScheduleAsk_TwoSendsInsideTheCooldownProduceOneFanOut(t *testing.T) {
	var lastAsk time.Time
	repo := &mockRSVPRepo{
		lastAskFn: func(context.Context, string) (time.Time, error) { return lastAsk, nil },
		recordAskFn: func(_ context.Context, a *ScheduleAsk) error {
			lastAsk = a.SentAt // the goroutine's write is what starts the cooldown
			return nil
		},
	}
	mailer := &mockMailer{configured: true}
	h := askHandler(t, testEvent(nil), askRoster(), mailer, repo)

	if _, err := callAsk(t, h, "owner", url.Values{}); err != nil {
		t.Fatalf("first ask: %v", err)
	}
	drainSends(t, mailer, 3)

	_, err := callAsk(t, h, "owner", url.Values{})
	if err == nil {
		t.Fatal("a second ask inside the cooldown must be refused")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusTooManyRequests {
		t.Fatalf("refusal = %v, want a 429", err)
	}
	if !strings.Contains(appErr.Message, "You can ask again in") {
		t.Errorf("the refusal must be a sentence the operator can act on; got %q", appErr.Message)
	}
	mailer.mu.Lock()
	defer mailer.mu.Unlock()
	if len(mailer.sent) != 3 {
		t.Errorf("two sends inside the cooldown produced %d emails, want exactly one fan-out of 3", len(mailer.sent))
	}
}

// TestScheduleAsk_FloorSkipsTheAskedAndReachesTheNewJoiner is the other half of
// §9.4: the per-recipient floor SKIPS, so a legitimate second ask after
// somebody joins mails the new member and nobody else.
func TestScheduleAsk_FloorSkipsTheAskedAndReachesTheNewJoiner(t *testing.T) {
	repo := &mockRSVPRepo{
		recentAskFn: func(context.Context, string, time.Time) ([]string, error) {
			return []string{"owner", "player", "codm"}, nil // everyone but the new joiner
		},
	}
	dir := askRoster()
	dir.members = append(dir.members, campaigns.CampaignMember{
		UserID: "newbie", Role: campaigns.RolePlayer, DisplayName: "Nis", Email: "nis@example.test",
	})
	mailer := &mockMailer{configured: true}
	h := askHandler(t, testEvent(nil), dir, mailer, repo)

	rec, err := callAsk(t, h, "owner", url.Values{})
	if err != nil {
		t.Fatalf("AskAvailabilityAPI: %v", err)
	}
	if got := askJSON(t, rec)["asking"]; got != float64(1) {
		t.Errorf("asking = %v, want 1 — only the newly joined member is outside the floor", got)
	}
	drainSends(t, mailer, 1)
	mailer.mu.Lock()
	defer mailer.mu.Unlock()
	if len(mailer.sent) != 1 || mailer.sent[0] != "nis@example.test" {
		t.Errorf("the second ask must reach only the new member; got %v", mailer.sent)
	}
}

// TestScheduleAsk_NothingRecordedWhenNothingSent covers all three ways a send
// can fail to happen. Each writes NO row, because a cooldown must never lock
// out a campaign that received no mail.
func TestScheduleAsk_NothingRecordedWhenNothingSent(t *testing.T) {
	t.Run("SMTP unconfigured refuses with the verbatim sentence", func(t *testing.T) {
		rows := 0
		repo := &mockRSVPRepo{recordAskFn: func(context.Context, *ScheduleAsk) error { rows++; return nil }}
		h := askHandler(t, testEvent(nil), askRoster(), &mockMailer{configured: false}, repo)

		_, err := callAsk(t, h, "owner", url.Values{})
		if err == nil {
			t.Fatal("an unconfigured mail server must not report a silent success")
		}
		if !strings.Contains(err.Error(), mailNotConfiguredLine) {
			t.Errorf("the refusal must be ledger item 11's own sentence; got %v", err)
		}
		if rows != 0 {
			t.Errorf("%d ask rows written with no mail server configured", rows)
		}
	})

	t.Run("no mail sender wired at all", func(t *testing.T) {
		rows := 0
		repo := &mockRSVPRepo{recordAskFn: func(context.Context, *ScheduleAsk) error { rows++; return nil }}
		h := askHandler(t, testEvent(nil), askRoster(), nil, repo)
		if _, err := callAsk(t, h, "owner", url.Values{}); err == nil {
			t.Fatal("with no mailer wired the ask must be refused, not silently dropped")
		}
		if rows != 0 {
			t.Errorf("%d ask rows written with no mailer wired", rows)
		}
	})

	t.Run("a send error writes no row", func(t *testing.T) {
		rows := 0
		repo := &mockRSVPRepo{recordAskFn: func(context.Context, *ScheduleAsk) error { rows++; return nil }}
		mailer := &mockMailer{configured: true, sendErr: errors.New("smtp: connection refused")}
		h := askHandler(t, testEvent(nil), askRoster(), mailer, repo)

		if _, err := callAsk(t, h, "owner", url.Values{}); err != nil {
			t.Fatalf("a failing mail server must not fail the request: %v", err)
		}
		drainSends(t, mailer, 3)
		if rows != 0 {
			t.Errorf("%d ask rows written for sends that all failed", rows)
		}
	})
}

// --- §9.2 / §9.3: what the caller may name ----------------------------------

// TestScheduleAsk_CallerNamesNobody is §9.2's "there must be no wire shape by
// which the caller names a recipient". Every hostile field is ignored: the
// audience comes from ListMembers, the actor from the session, the campaign
// from the authorised path parameter.
func TestScheduleAsk_CallerNamesNobody(t *testing.T) {
	mailer := &mockMailer{configured: true}
	var recorded []ScheduleAsk
	repo := &mockRSVPRepo{recordAskFn: func(_ context.Context, a *ScheduleAsk) error {
		recorded = append(recorded, *a)
		return nil
	}}
	h := askHandler(t, testEvent(nil), askRoster(), mailer, repo)

	rec, err := callAsk(t, h, "owner", url.Values{
		"recipient":         {"victim@elsewhere.test"},
		"recipient_user_id": {"victim"},
		"to":                {"victim@elsewhere.test"},
		"campaign_id":       {"some-other-campaign"},
		"actor_user_id":     {"someone-else"},
		"email":             {"victim@elsewhere.test"},
	})
	if err != nil {
		t.Fatalf("AskAvailabilityAPI: %v", err)
	}
	if got := askJSON(t, rec)["asking"]; got != float64(3) {
		t.Errorf("asking = %v, want the roster's 3 — body fields must be ignored", got)
	}
	drainSends(t, mailer, 3)

	mailer.mu.Lock()
	defer mailer.mu.Unlock()
	for _, to := range mailer.sent {
		if strings.Contains(to, "elsewhere.test") {
			t.Fatalf("a body-supplied recipient was mailed: %v", mailer.sent)
		}
	}
	for _, r := range recorded {
		if r.CampaignID != "camp-1" {
			t.Errorf("a body-supplied campaign id reached the log: %+v", r)
		}
		if r.ActorUserID != "owner" {
			t.Errorf("a body-supplied actor id reached the log: %+v", r)
		}
	}
}

// TestScheduleAsk_CrossCampaignEventIs404 is §9.3: the ONE accepted parameter
// is re-resolved through the existing campaign-scoping check.
func TestScheduleAsk_CrossCampaignEventIs404(t *testing.T) {
	// The event's calendar belongs to another campaign.
	otherCal := testCalendar(func(c *Calendar) { c.CampaignID = "camp-999" })
	h := NewRSVPHandler(NewRSVPService(&mockRSVPRepo{}, &mockEventLookup{evt: testEvent(nil), cal: otherCal}))
	h.SetMemberDirectory(askRoster())
	h.SetMailSender(&mockMailer{configured: true}, "https://chronicle.example.test")

	_, err := callAsk(t, h, "owner", url.Values{"event_id": {"evt-1"}})
	if err == nil {
		t.Fatal("an event id from another campaign must not resolve")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusNotFound {
		t.Fatalf("cross-campaign event_id = %v, want 404", err)
	}
}

// TestScheduleAsk_NonCollectingEventCarriesNoRSVPHalf: naming a real event of
// this campaign that is NOT collecting does not conjure an RSVP section — the
// ask still goes out, without action links nobody asked to receive.
func TestScheduleAsk_NonCollectingEventCarriesNoRSVPHalf(t *testing.T) {
	evt := testEvent(func(e *Event) { e.CollectRSVPs = false })
	mailer := &mockMailer{configured: true}
	minted := 0
	repo := &mockRSVPRepo{createTokenFn: func(context.Context, *EventRSVPToken) error { minted++; return nil }}
	h := askHandler(t, evt, askRoster(), mailer, repo)

	if _, err := callAsk(t, h, "owner", url.Values{"event_id": {"evt-1"}}); err != nil {
		t.Fatalf("AskAvailabilityAPI: %v", err)
	}
	drainSends(t, mailer, 3)
	if minted != 0 {
		t.Errorf("%d tokens minted for an event that is not collecting RSVPs", minted)
	}
	mailer.mu.Lock()
	defer mailer.mu.Unlock()
	for _, b := range mailer.plains {
		if strings.Contains(b, "calendar-rsvp") {
			t.Error("an event that is not collecting must contribute no action links")
		}
	}
}

// --- §9.4: the per-actor limiter -------------------------------------------

// TestScheduleAskActorLimiter_TenPerHourPerUser pins the third layer: it is
// per-USER (two actors do not share a budget), it answers 429 with Retry-After,
// and it is not an import of the bestiary package — this is the calendar's own.
func TestScheduleAskActorLimiter_TenPerHourPerUser(t *testing.T) {
	mw := calendarUserRateLimit(scheduleAskActorBurst, time.Hour)
	handler := mw(func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	call := func(userID string) *httptest.ResponseRecorder {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/calendar/ask", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("auth_user_id", userID)
		if err := handler(c); err != nil {
			t.Fatalf("limiter: %v", err)
		}
		return rec
	}

	for i := 1; i <= scheduleAskActorBurst; i++ {
		if rec := call("gm-1"); rec.Code != http.StatusOK {
			t.Fatalf("request %d of %d was refused (%d); the budget is per hour", i, scheduleAskActorBurst, rec.Code)
		}
	}
	rec := call("gm-1")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request %d = %d, want 429", scheduleAskActorBurst+1, rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("the 429 must carry Retry-After — a limit the client cannot wait out is a wall")
	}
	// A DIFFERENT actor is unaffected: a table shares an IP far more often than
	// it shares an account, which is why this is per-user and not per-IP.
	if rec := call("gm-2"); rec.Code != http.StatusOK {
		t.Errorf("a second actor was caught by the first actor's budget (%d)", rec.Code)
	}
}
