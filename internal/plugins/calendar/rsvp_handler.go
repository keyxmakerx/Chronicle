// Event RSVP HTTP surface (C-CAL-RSVP-P1): two campaign-scoped API endpoints, a
// Scribe+ collection toggle, and the public single-use token flow reached from
// action emails.
//
// Cross-plugin dependencies are narrow interfaces declared HERE and injected by
// post-construction setters from internal/app/routes.go (house rule 8). The
// calendar plugin gains no compile-time edge into sessions — which the wire
// contract (internal/wire/plugin_import_guard_test.go) forbids outright — and
// every one of them is nil-safe: with no SMTP configured and no adapters wired,
// in-app RSVP still works end to end.
package calendar

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// MailSender sends email notifications. Wraps the SMTP service interface.
// Copied verbatim from the sessions plugin's identical seam
// (sessions/handler.go:21-24) rather than shared, so neither plugin imports the
// other; the concrete smtp service satisfies both.
type MailSender interface {
	SendHTMLMail(ctx context.Context, to []string, subject, plainBody, htmlBody string) error
	IsConfigured(ctx context.Context) bool
}

// RSVPNotifier writes in-app (bell) notifications.
//
// The notification TYPE vocabulary belongs to the notifications store's owner,
// so it is not a parameter here: the adapter in internal/app/routes.go supplies
// the type constant. The calendar only says who, what, and where to link.
type RSVPNotifier interface {
	NotifyRSVP(ctx context.Context, userIDs []string, campaignID, message, link string) error
}

// AvailabilityExceptionWriter is the scheduler slice the "Out this week" action
// needs.
//
// SELF-WRITE ONLY. userID is never taken from anything the caller controls —
// every call site passes the id from the redeemed token or the authenticated
// session, so one member can never mark another unavailable. Timezone
// resolution stays on the adapter side: the calendar has no business knowing a
// member's IANA zone.
type AvailabilityExceptionWriter interface {
	// ExceptionDates returns the YYYY-MM-DD dates on which the member already
	// has any availability exception. Used to SKIP hand-authored days.
	ExceptionDates(ctx context.Context, campaignID, userID string) ([]string, error)
	// MarkDaysUnavailable writes a full-day (0–1440) 'unavailable' exception for
	// each supplied date.
	MarkDaysUnavailable(ctx context.Context, campaignID, userID string, dates []string) error
	// OfferAvailableWindows records TEMPORARY availability the member is offering
	// for specific dates. ADDITIVE: the implementation composes the rest of each
	// day around the offer, so saying "I could also do Tuesday evening" can never
	// leave the member less available than before.
	OfferAvailableWindows(ctx context.Context, campaignID, userID string, windows []RSVPAvailabilityWindow) error
}

// RSVPMemberDirectory is the campaigns read the RSVP flows need: the roster (for
// email fan-out + display names) and the co-DM grant (so a co-DM's emailed link
// to a dm_only event still resolves). campaigns.CampaignService satisfies it.
type RSVPMemberDirectory interface {
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
	IsUserDmGranted(ctx context.Context, campaignID, userID string) (bool, error)
}

// RSVPHandler processes event-RSVP HTTP requests.
type RSVPHandler struct {
	svc          RSVPService
	members      RSVPMemberDirectory
	mailer       MailSender
	notifier     RSVPNotifier
	availability AvailabilityExceptionWriter
	baseURL      string // Application base URL for emailed action links.
}

// NewRSVPHandler creates the RSVP handler. Every cross-plugin seam is wired
// afterwards via the SetX setters below.
func NewRSVPHandler(svc RSVPService) *RSVPHandler {
	return &RSVPHandler{svc: svc}
}

// SetMemberDirectory wires the campaigns roster + co-DM grant reads.
func (h *RSVPHandler) SetMemberDirectory(d RSVPMemberDirectory) { h.members = d }

// SetMailSender wires the SMTP sender for action emails. Nil or unconfigured
// disables the email fan-out only — in-app RSVP is unaffected.
func (h *RSVPHandler) SetMailSender(ms MailSender, baseURL string) {
	h.mailer = ms
	h.baseURL = baseURL
}

// SetRSVPNotifier wires the in-app bell notifier.
func (h *RSVPHandler) SetRSVPNotifier(n RSVPNotifier) { h.notifier = n }

// SetAvailabilityWriter wires the scheduler write used by "Out this week".
func (h *RSVPHandler) SetAvailabilityWriter(w AvailabilityExceptionWriter) { h.availability = w }

// --- Campaign-scoped API ---

// SetEventRSVPAPI upserts the CALLER'S OWN RSVP.
// POST /campaigns/:id/calendars/:calId/events/:eid/rsvp — RolePlayer.
//
// The user id comes from the session, never the body: there is no wire shape by
// which one member can answer for another.
func (h *RSVPHandler) SetEventRSVPAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)

	evt, _, err := h.svc.EventContext(ctx, c.Param("eid"), cc.Campaign.ID)
	if err != nil {
		return err
	}

	var req SetRSVPRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// `action` is the in-app twin of the emailed action links. Both branches
	// still write only the CALLER'S own row.
	notice := ""
	switch req.Action {
	case RSVPActionSuggest:
		note := ""
		if req.Note != nil {
			note = *req.Note
		}
		var err error
		notice, err = h.applySuggestion(ctx, cc.Campaign.ID, evt, userID, cc.VisibilityRole(), note, req.Windows)
		if err != nil {
			return err
		}
	case RSVPActionOutWeek:
		if err := h.svc.SetMyRSVP(ctx, evt, userID, cc.VisibilityRole(),
			SetRSVPRequest{Status: RSVPNo, Note: req.Note}); err != nil {
			return err
		}
		_, cal, err := h.svc.EventContext(ctx, evt.ID, cc.Campaign.ID)
		if err != nil {
			return err
		}
		notice = h.applyOutThisWeek(ctx, cal, evt, userID)
		h.notifyOwnerOfResponse(ctx, cc.Campaign.ID, evt, userID, RSVPNo)
	default:
		if err := h.svc.SetMyRSVP(ctx, evt, userID, cc.VisibilityRole(), req); err != nil {
			return err
		}
		h.notifyOwnerOfResponse(ctx, cc.Campaign.ID, evt, userID, req.Status)
	}

	// THE FRAGMENT BRANCH (WG-6). Same route, same method, same role gate — the
	// only thing that changes is the representation, which is the house
	// convention (CLAUDE.md, "HTMX detection") and strictly less surface than a
	// second route performing the same write. The JSON below is untouched, so
	// static/js/availability.js and the Foundry-facing /api/v1 group keep
	// working exactly as they did.
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK,
			benchTileView(h.benchSessionFragment(c, cc, evt, userID, notice)))
	}

	summary, err := h.svc.Summary(ctx, evt, userID, cc.VisibilityRole(), cc.CanControlWorldState())
	if err != nil {
		return err
	}
	h.decorateResponders(ctx, cc.Campaign.ID, summary)
	if notice != "" {
		return c.JSON(http.StatusOK, struct {
			*EventRSVPSummary
			Notice string `json:"notice"`
		}{summary, notice})
	}
	return c.JSON(http.StatusOK, summary)
}

// benchSessionFragment re-renders the Bench's session tile after a write.
//
// ── WHAT LEAVES THE SERVER HERE, FIELD BY FIELD (§8.3 item 1) ──────────────
//
//	the event's name        — the caller passed its id and CanUserViewEvent
//	                          has already allowed them to see it
//	how far off it is       — derived from the event they can see
//	`N of M answered`       — two integers, both recomputed below from the
//	                          MEMBERSHIP-FILTERED roster
//	the caller's own answer — their own data, which Summary already returns
//	                          to them as my_status
//
// A FRAGMENT IS EASIER TO OVER-FILL THAN A JSON BODY, because it looks like
// markup rather than like data. So this one is deliberately POORER than the
// JSON response it replaces: it carries no responder names, no notes and no
// per-person breakdown, all of which Summary returns to an Owner/co-DM. There
// is no role branch here at all — nothing on this tile is gated because nothing
// on it is privileged.
//
// THE TALLY IS RECOMPUTED, NEVER READ BACK (§6). Summary.Counts is raw rows and
// would count somebody who has left the campaign; this walks the roster and
// counts members holding a row, exactly as the panel does.
func (h *RSVPHandler) benchSessionFragment(c echo.Context, cc *campaigns.CampaignContext,
	evt *Event, userID, notice string) BenchTile {
	ctx := c.Request().Context()
	in := benchSessionTileInput{
		// IsGM matches buildBench's own gate exactly, so the tile a write
		// returns is the tile the page would have rendered — a Director gets
		// their chipped Nudge back, not a player's answer form.
		IsGM:       permissions.CanSeeDmOnly(cc.VisibilityRole()),
		CampaignID: cc.Campaign.ID,
		CalendarID: evt.CalendarID,
		EventID:    evt.ID,
		Name:       evt.Name,
		When:       "next up",
		CSRFToken:  middleware.GetCSRFToken(c),
		Notice:     notice,
	}
	answers, err := h.svc.AnswersByUser(ctx, evt, userID, cc.VisibilityRole())
	if err != nil {
		slog.Warn("bench rsvp fragment: answers read failed",
			slog.String("event_id", evt.ID), slog.Any("error", err))
	}
	in.MyStatus = answers[userID]
	if h.members != nil {
		if members, merr := h.members.ListMembers(ctx, cc.Campaign.ID); merr == nil {
			in.Total = len(members)
			for _, m := range members {
				if _, ok := answers[m.UserID]; ok {
					in.Answered++
				}
			}
		}
	}
	return benchSessionTileLive(in)
}

// GetEventRSVPsAPI returns counts for everyone who can see the event, plus the
// per-person breakdown for Owner/co-DM only.
// GET /campaigns/:id/calendars/:calId/events/:eid/rsvps — RolePlayer.
//
// The detail gate is CanControlWorldState() (Owner OR DM-grantee) — the same
// Owner/co-DM capability the calendar already uses for world-state control, and
// the mirror of the scheduler overlay's density-for-all / identity-for-the-DM
// split (sessions/availability_model.go WeekOverlay.IncludeDetail).
func (h *RSVPHandler) GetEventRSVPsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)

	evt, _, err := h.svc.EventContext(ctx, c.Param("eid"), cc.Campaign.ID)
	if err != nil {
		return err
	}
	summary, err := h.svc.Summary(ctx, evt, userID, cc.VisibilityRole(), cc.CanControlWorldState())
	if err != nil {
		return err
	}
	h.decorateResponders(ctx, cc.Campaign.ID, summary)
	return c.JSON(http.StatusOK, summary)
}

// SetRSVPCollectionAPI flips the per-event "Collect RSVPs" opt-in.
// PUT /campaigns/:id/calendars/:calId/events/:eid/rsvp-collection — RoleScribe.
//
// Turning collection ON is the invite moment: it fans out action emails and
// bell notifications to the members who can SEE the event. Turning it off is
// silent. The fan-out runs in the background so a slow SMTP server never holds
// the operator's click.
func (h *RSVPHandler) SetRSVPCollectionAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	evt, cal, err := h.svc.EventContext(ctx, c.Param("eid"), cc.Campaign.ID)
	if err != nil {
		return err
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if err := h.svc.SetCollection(ctx, evt.ID, req.Enabled); err != nil {
		return err
	}

	if req.Enabled && !evt.CollectRSVPs {
		// Reflect the new state on the copy the fan-out reads, so recipients
		// aren't invited to an event that still says it isn't collecting.
		evt.CollectRSVPs = true
		h.startInviteFanOut(cc.Campaign.ID, cc.Campaign.Name, cal, evt)
	}
	return c.JSON(http.StatusOK, map[string]any{"collect_rsvps": req.Enabled})
}

// mailNotConfiguredLine is spec ledger item 11's own wording, VERBATIM and
// SHARED. It is the one sentence Chronicle says when a mail server has not been
// configured, and it is a single package-level constant precisely so a future
// edit cannot make the Bench and this endpoint disagree about what an
// unconfigured mail server means. Never a silent success; never "we emailed
// them" when nothing was sent.
const mailNotConfiguredLine = "Email is not configured on this server — answers still work in-app; nobody was emailed."

// AskAvailabilityAPI mails the campaign's roster the schedule question.
// POST /campaigns/:id/calendar/ask — RoleScribe ([PB-3]).
//
// ── IT IS A WRITE, SO IT IS A POST ─────────────────────────────────────────
// Sending mail to a roster from a GET is one link prefetcher away from a
// fan-out nobody clicked. It rides the global CSRF middleware (not under /api/
// or /ws), accepting either the header or the csrf_token field the Bench's form
// sends.
//
// ── THE CALLER NAMES NOBODY ────────────────────────────────────────────────
// The audience is derived server-side from ListMembers(campaignID), full stop.
// There is no wire shape by which a caller names a recipient, an actor or a
// campaign: the campaign comes from :id (already authorised by
// RequireCampaignAccess) and the actor from the session. The single accepted
// parameter is an optional event_id — the session the panel already resolved —
// and it is RE-RESOLVED through EventContext, which is the existing
// campaign-scoping check, so an event id from another campaign 404s.
//
// ── ONE EMAIL, TWO SECTIONS ([PB-1]) ───────────────────────────────────────
// The schedule ask leads and needs no session. The five existing RSVP action
// links ride along only when the event is collecting AND this recipient may see
// it, re-minted through the EXISTING MintActionTokens — so both emails make the
// same promises with the same expiry and the same single-use property.
//
// ── THE RESPONSE IS A COUNT ────────────────────────────────────────────────
// "asking N members; M have no email address on file" — a number, never a list.
// It carries no member identities, no addresses and no per-recipient outcome,
// and it cannot: the send is backgrounded, so per-recipient results do not exist
// yet when this returns.
func (h *RSVPHandler) AskAvailabilityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	campaignID := cc.Campaign.ID
	actorID := auth.GetUserID(c)

	if h.members == nil {
		return apperror.NewConflict("this campaign's roster is unavailable, so nobody was emailed")
	}

	// THE CAMPAIGN COOLDOWN, and it is checked before anything is resolved or
	// minted. 429 with the human sentence rather than a bare code: the Bench
	// disables the control with the same words, so a caller who gets here has
	// bypassed the UI and still deserves to be told why.
	state, err := h.svc.ScheduleAskState(ctx, campaignID)
	if err != nil {
		return err
	}
	if !state.Ready {
		return apperror.NewTooManyRequests(scheduleAskCooldownLine(state))
	}

	// NEVER A SILENT SUCCESS. With no mail server there is nothing to send, and
	// saying so is the whole point of ledger item 11. 409 rather than 500: the
	// server is working correctly and is simply not configured to do this — an
	// admin action, not a Chronicle failure.
	if h.mailer == nil || !h.mailer.IsConfigured(ctx) {
		return apperror.NewConflict(mailNotConfiguredLine)
	}

	// The optional session, re-resolved through the existing campaign-scoping
	// check. A cross-campaign id 404s here, exactly as it does everywhere else
	// in this plugin, and a non-collecting event simply carries no RSVP half.
	var evt *Event
	var cal *Calendar
	if eid := strings.TrimSpace(c.FormValue("event_id")); eid != "" {
		evt, cal, err = h.svc.EventContext(ctx, eid, campaignID)
		if err != nil {
			return err
		}
		if !evt.CollectRSVPs {
			evt, cal = nil, nil
		}
	}

	members, err := h.members.ListMembers(ctx, campaignID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	// The per-recipient floor SKIPS, it does not refuse: a second ask after
	// somebody joins mails the new member and nobody else.
	skip, err := h.svc.RecentlyAskedRecipients(ctx, campaignID)
	if err != nil {
		return err
	}

	// THE AUDIENCE IS THE WHOLE ROSTER — not "people who have not saved
	// availability", because OverlayMember has no has-pattern signal and
	// "never told us" is indistinguishable from "busy all week". A
	// solicitation must not become an accusation.
	recipients := make([]scheduleAskRecipient, 0, len(members))
	noAddress := 0
	for _, m := range members {
		if m.Email == "" {
			noAddress++ // counted, never named.
			continue
		}
		if skip[m.UserID] {
			continue
		}
		recipients = append(recipients, scheduleAskRecipient{
			Member:      m,
			MaySeeEvent: evt != nil && h.memberMaySeeEvent(ctx, campaignID, m, evt),
		})
	}

	h.startScheduleAskFanOut(scheduleAskEmailInput{
		CampaignID:   campaignID,
		CampaignName: cc.Campaign.Name,
		ActorName:    h.displayName(ctx, campaignID, actorID),
		Event:        evt,
		Calendar:     cal,
	}, actorID, recipients)

	line := scheduleAskSentLine(len(recipients), noAddress)
	if middleware.IsHTMX(c) {
		// THE FRAGMENT IS NOT RICHER THAN THE DATA. It is the same sentence the
		// JSON carries, rendered where the control was, so the Director sees the
		// outcome without the page having to reload to learn the cooldown began.
		return middleware.Render(c, http.StatusOK, benchAskResultView(line))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"asking":   len(recipients),
		"no_email": noAddress,
		"message":  line,
	})
}

// scheduleAskSentLine states the outcome as NUMBERS. Members with no address on
// file are counted and never named (§5): the actor is a Scribe+ who can see the
// roster page anyway, but this endpoint is not the place that hands out a list
// of who can and cannot be reached by email.
func scheduleAskSentLine(asking, noAddress int) string {
	line := fmt.Sprintf("Asking %s now.", pluralMembers(asking))
	if asking == 0 {
		line = "Nobody was emailed."
	}
	if noAddress > 0 {
		line += fmt.Sprintf(" %s no email address on file.", pluralHave(noAddress))
	}
	return line
}

// scheduleAskCooldownLine is the campaign cooldown's refusal, in words. The
// Bench prints the same sentence beside a disabled control, so the limit is
// visible BEFORE the click rather than discovered by hitting it.
func scheduleAskCooldownLine(state ScheduleAskState) string {
	hours := int(math.Ceil(state.RetryAfter.Hours()))
	if hours < 1 {
		hours = 1
	}
	return fmt.Sprintf("Asked %s ago. You can ask again in %s.",
		humanAgo(time.Since(state.LastAskedAt)), pluralHours(hours))
}

// humanAgo renders an elapsed duration the way a person would say it. Coarse on
// purpose: this is a cooldown readout, not a timestamp.
func humanAgo(d time.Duration) string {
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins < 1 {
			return "less than a minute"
		}
		return pluralUnit(mins, "minute")
	}
	return pluralUnit(int(d.Hours()), "hour")
}

func pluralMembers(n int) string { return pluralUnit(n, "member") }
func pluralHours(n int) string   { return pluralUnit(n, "hour") }

func pluralHave(n int) string {
	if n == 1 {
		return "1 member has"
	}
	return fmt.Sprintf("%d members have", n)
}

func pluralUnit(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// decorateResponders fills display names on a detail summary.
//
// Names are resolved through the campaigns roster (rule 8) rather than a JOIN on
// the core users table, which also means a member who has LEFT the campaign
// simply drops out of the list instead of lingering as a bare id.
func (h *RSVPHandler) decorateResponders(ctx context.Context, campaignID string, s *EventRSVPSummary) {
	if s == nil || len(s.Responders) == 0 || h.members == nil {
		return
	}
	members, err := h.members.ListMembers(ctx, campaignID)
	if err != nil {
		return
	}
	byID := make(map[string]campaigns.CampaignMember, len(members))
	for _, m := range members {
		byID[m.UserID] = m
	}
	kept := s.Responders[:0]
	for _, r := range s.Responders {
		m, ok := byID[r.UserID]
		if !ok {
			continue // no longer a member
		}
		r.DisplayName = m.DisplayName
		if r.DisplayName == "" {
			r.DisplayName = "Member"
		}
		kept = append(kept, r)
	}
	s.Responders = kept
}

// notifyOwnerOfResponse pings the event's author that someone answered.
// Fire-and-forget: a notification failure must never fail the RSVP write.
func (h *RSVPHandler) notifyOwnerOfResponse(ctx context.Context, campaignID string, evt *Event, responderID, status string) {
	if h.notifier == nil || evt.CreatedBy == nil || *evt.CreatedBy == "" || *evt.CreatedBy == responderID {
		return
	}
	name := h.displayName(ctx, campaignID, responderID)
	msg := fmt.Sprintf("%s responded %q to %q", name, status, evt.Name)
	if err := h.notifier.NotifyRSVP(ctx, []string{*evt.CreatedBy}, campaignID, msg, rsvpEventLink(campaignID, evt)); err != nil {
		slog.Warn("calendar rsvp: owner notification failed", slog.Any("error", err), slog.String("event_id", evt.ID))
	}
}

// displayName resolves one member's name, falling back to a neutral label.
func (h *RSVPHandler) displayName(ctx context.Context, campaignID, userID string) string {
	if h.members == nil {
		return "A player"
	}
	members, err := h.members.ListMembers(ctx, campaignID)
	if err != nil {
		return "A player"
	}
	for _, m := range members {
		if m.UserID == userID && m.DisplayName != "" {
			return m.DisplayName
		}
	}
	return "A player"
}

// rsvpEventLink is the in-app URL an RSVP notification points at: the V2
// calendar shell for the event's own calendar.
func rsvpEventLink(campaignID string, evt *Event) string {
	return fmt.Sprintf("/campaigns/%s/calendar/v2/%s", campaignID, evt.CalendarID)
}

// rsvpDateLine renders the human date line shared by the emails and the token
// pages: the event's own (possibly fantasy) date, plus its time when set.
func rsvpDateLine(cal *Calendar, evt *Event) string {
	// Event.Month is a 1-based index into cal.Months — the same convention every
	// other render helper uses (calendar_v2_helpers.go: cal.Months[Month-1]).
	// Bounds-checked here because this runs on the emailed path, where a
	// months-edited calendar must degrade to a readable label, not panic.
	monthName := fmt.Sprintf("month %d", evt.Month)
	if cal != nil && evt.Month >= 1 && evt.Month <= len(cal.Months) && cal.Months[evt.Month-1].Name != "" {
		monthName = cal.Months[evt.Month-1].Name
	}
	line := fmt.Sprintf("%s %d, %d", monthName, evt.Day, evt.Year)
	if t := evt.FormatTime(); t != "" {
		line += " at " + t
	} else if evt.AllDay {
		line += " (all day)"
	}
	return line
}

// rsvpActionLabel is the human label for an emailed action.
func rsvpActionLabel(action string) string {
	switch action {
	case RSVPActionYes:
		return "Going"
	case RSVPActionMaybe:
		return "Maybe"
	case RSVPActionNo:
		return "Can't make it"
	case RSVPActionOutWeek:
		return "Out this week"
	case RSVPActionSuggest:
		return "Suggest another time"
	default:
		return "Respond"
	}
}

// trimForDisplay bounds an interpolated string so a long event name can't blow
// out an email subject or a notification message.
func trimForDisplay(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max-1]) + "…"
}

// nowUTC is the clock the RSVP flows read. A package-level var so tests can
// pin the "out this week" week without waiting for a real Monday.
var nowUTC = func() time.Time { return time.Now().UTC() }

// escapeAttr is a tiny local alias making the escaping intent explicit at every
// interpolation site in the standalone token pages below.
func escapeAttr(s string) string { return html.EscapeString(s) }

// --- Public single-use token flow (/calendar-rsvp/:token) ---
//
// Mounted at the ROOT, distinct from sessions' /rsvp/:token, and deliberately
// outside every campaign group: an emailed link has no campaign in its path and
// its recipient may be logged out, so the token is the sole credential and this
// handler does its own membership + visibility checks. It therefore also sits
// outside RequireAddon — a member who was emailed a link must still be able to
// answer it, and the token is worthless if the event is gone.
//
// GET  = pure read confirm interstitial (a mail scanner's prefetch changes
//        nothing) and, for "suggest another time", the free-text form.
// POST = the state change. Both halves ride the GLOBAL CSRF middleware (only
//        /api/* and /ws are exempt — internal/app/app.go:160), so the GET mints
//        the cookie and plants the matching hidden field for the POST's
//        double-submit. Without it the confirm click would 403.

// rsvpDeadTokenPage is what BOTH token routes render when resolveToken refuses.
//
// C-CALV4-GAMEREADY §5 [GR-8]. THE MEASURED DEAD END: after a successful POST,
// a browser refresh, a double-tap, a mail client's prefetch, or simply
// re-opening the link to check "did that go through?" rendered
//
//	RSVP Failed
//	this RSVP link is invalid or has expired
//
// The answer HAD been recorded. The page just said it failed — and a player who
// reads that tells their GM the RSVP system is broken, and the GM believes them,
// because the product said so. Session zero then goes on debugging a system
// that is working perfectly.
//
// So: when the link is spent, an answer stands on record for it, and the SAME
// membership + event-visibility re-check resolveToken runs still passes, state
// the answer and offer the schedule. Everything else keeps the generic page.
//
// THE GATE IS DELIBERATELY THE FULL ONE, NOT A CHEAPER LOOKALIKE. Widening by a
// single condition is a leak: a member removed from the campaign, and an event
// flipped to dm_only after the invite went out, must still get the generic page
// that never discloses the title. That is why this re-runs memberRole and
// CanUserViewEvent rather than trusting the token, and why a failure at ANY step
// falls through to the same generic page rather than explaining itself.
func (h *RSVPHandler) rsvpDeadTokenPage(ctx context.Context, tokenStr string, cause error) string {
	generic := rsvpResultPage("RSVP Failed", apperror.UserMessage(cause, rsvpBadTokenMsg), false, "")

	tok, answer, err := h.svc.AnsweredToken(ctx, tokenStr)
	if err != nil {
		return generic
	}
	evt, cal, err := h.svc.EventContext(ctx, tok.EventID, "")
	if err != nil || !evt.CollectRSVPs {
		return generic
	}
	role, ok := h.memberRole(ctx, cal.CampaignID, tok.UserID)
	if !ok || !h.svc.CanUserViewEvent(evt, role, tok.UserID) {
		return generic
	}

	return rsvpResultPage("You've already answered",
		fmt.Sprintf("You're down as %q for %s.",
			rsvpStatusLabel(answer.Status), trimForDisplay(evt.Name, 80)),
		true, rsvpSchedulePath(cal.CampaignID))
}

// rsvpStatusLabel names a STORED status for a human. It is deliberately separate
// from rsvpActionLabel, which names an ACTION: "Out this week" is an action that
// stores the status "no", and telling a member they are down as "Out this week"
// when the row says `no` would be reporting the button they pressed rather than
// the answer the Director will count.
func rsvpStatusLabel(status string) string {
	switch status {
	case RSVPYes:
		return "Going"
	case RSVPMaybe:
		return "Maybe"
	case RSVPNo:
		return "Not going"
	default:
		return "Answered"
	}
}

// RedeemEventRSVPToken renders the confirm interstitial.
// GET /calendar-rsvp/:token
func (h *RSVPHandler) RedeemEventRSVPToken(c echo.Context) error {
	ctx := c.Request().Context()
	tokenStr := c.Param("token")

	tok, evt, cal, err := h.resolveToken(ctx, tokenStr)
	if err != nil {
		return c.HTML(http.StatusOK, h.rsvpDeadTokenPage(ctx, tokenStr, err))
	}

	action := escapeAttr("/calendar-rsvp/" + tokenStr)
	csrf := middleware.GetCSRFToken(c)
	detail := evt.Name + " — " + rsvpDateLine(cal, evt)

	if tok.Action == RSVPActionSuggest {
		return c.HTML(http.StatusOK, rsvpSuggestPage(detail, action, csrf, ""))
	}

	msg := fmt.Sprintf("You're responding %q to %s. Tap below to confirm.", rsvpActionLabel(tok.Action), detail)
	if tok.Action == RSVPActionOutWeek {
		_, dates := rsvpWeekDates(cal, evt, nowUTC())
		msg = fmt.Sprintf("This declines %s and marks you unavailable for the whole week of %s "+
			"(days you've already customised are left alone). Tap below to confirm.", detail, dates[0])
	}
	return c.HTML(http.StatusOK, rsvpConfirmPage("Confirm your response", msg, action,
		"Confirm — "+rsvpActionLabel(tok.Action), csrf))
}

// ApplyEventRSVPToken consumes the token and applies its action.
// POST /calendar-rsvp/:token
func (h *RSVPHandler) ApplyEventRSVPToken(c echo.Context) error {
	ctx := c.Request().Context()
	tokenStr := c.Param("token")

	tok, evt, cal, err := h.resolveToken(ctx, tokenStr)
	if err != nil {
		return c.HTML(http.StatusOK, h.rsvpDeadTokenPage(ctx, tokenStr, err))
	}

	// ── VALIDATE BEFORE CONSUMING ─────────────────────────────────────────
	// C-CALV4-GAMEREADY §5 [GR-7]. "Suggest another time" is the ONE action
	// whose write can still be REFUSED after the token has resolved:
	// applySuggestion rejects a submission carrying neither a usable window nor
	// a note. This handler used to call ApplyToken FIRST, which marks the token
	// used — so a partially-filled row (date + from, no `to`, with an empty
	// note) was refused with the link ALREADY DEAD. Correcting the row and
	// resubmitting answered "this RSVP link is invalid or has expired", and so
	// did re-opening the link from the email: one incomplete form permanently
	// destroyed a player's only way in.
	//
	// So the suggest branch runs the write first and consumes ONLY on success.
	// That is the same shape as the existing GET-never-applies rule, moved one
	// step earlier — nothing is spent until something happened.
	//
	// SINGLE-USE IS NOT WEAKENED. ApplyToken still owns consumption and still
	// consumes atomically (`used_at IS NULL`); it is safe to run LAST for this
	// action specifically because it returns early for `suggest` (rsvp_service.go)
	// — it marks the token used and writes no status — so the reorder changes
	// only WHEN the link dies, never whether it can be spent twice. Two
	// concurrent submits still land exactly one consume; the loser is told the
	// link is spent, and the suggestion the winner wrote is on record.
	if tok.Action == RSVPActionSuggest {
		// The token flow's role is the member's real campaign role, resolved in
		// resolveToken; re-resolve rather than assume, so the visibility gate
		// inside SuggestTime is the same one every other path uses.
		role, _ := h.memberRole(ctx, cal.CampaignID, tok.UserID)
		msg, err := h.applySuggestion(ctx, cal.CampaignID, evt, tok.UserID, role,
			c.FormValue("note"), parseOfferedWindows(c))
		if err != nil {
			// THE FORM COMES BACK, carrying the reason. Not a dead end: the
			// token is still live, which is the entire point of the reorder.
			return c.HTML(http.StatusOK, rsvpSuggestPage(
				evt.Name+" — "+rsvpDateLine(cal, evt),
				escapeAttr("/calendar-rsvp/"+tokenStr),
				middleware.GetCSRFToken(c),
				apperror.UserMessage(err, "Please add a time that would work, or a short note.")))
		}
		if _, err := h.svc.ApplyToken(ctx, tokenStr); err != nil {
			return c.HTML(http.StatusOK, h.rsvpDeadTokenPage(ctx, tokenStr, err))
		}
		// THE SUCCESS PAGE CARRIES THE SAME DOOR ([GR-8]): a member who answers
		// from their inbox should not land somewhere with no route onward.
		return c.HTML(http.StatusOK, rsvpResultPage("Response recorded", msg, true,
			rsvpSchedulePath(cal.CampaignID)))
	}

	applied, err := h.svc.ApplyToken(ctx, tokenStr)
	if err != nil {
		return c.HTML(http.StatusOK, h.rsvpDeadTokenPage(ctx, tokenStr, err))
	}

	message := fmt.Sprintf("Your response to %q was recorded.", trimForDisplay(evt.Name, 80))
	switch applied.Action {
	case RSVPActionOutWeek:
		message = h.applyOutThisWeek(ctx, cal, evt, tok.UserID)
		// C-CALV4-GAMEREADY §5 [GR-9]. This branch used to return without
		// notifying, while `default:` (yes/maybe/no) notified — so the emailed
		// "Out this week" told the Director NOTHING, and it is the one decline
		// most likely to cancel the session. The Director saw four
		// notifications, counted four, and arrived to a table of three.
		//
		// The asymmetry was never a design choice: the IN-APP branch of
		// SetEventRSVPAPI has always called this, with this same RSVPNo, so the
		// intent was already written down one file over. The two surfaces are
		// now pinned in ONE test (TestRSVPToken_OutWeekNotifiesOwner) precisely
		// because being pinned in different places is how they drifted.
		h.notifyOwnerOfResponse(ctx, cal.CampaignID, evt, tok.UserID, RSVPNo)
	default:
		h.notifyOwnerOfResponse(ctx, cal.CampaignID, evt, tok.UserID, statusForAction(applied.Action))
	}
	return c.HTML(http.StatusOK, rsvpResultPage("Response recorded", message, true,
		rsvpSchedulePath(cal.CampaignID)))
}

// parseOfferedWindows reads the emailed suggestion form's date/from/to rows.
//
// The form is plain HTML with NO JavaScript — it renders inside whatever browser
// an email client hands off to, possibly for a logged-out member — so the rows
// are a fixed set of indexed fields and a row is simply skipped unless all three
// parts are present and coherent. Malformed input is DROPPED rather than
// erroring: the note still gets through, and the member is told what was saved.
func parseOfferedWindows(c echo.Context) []RSVPAvailabilityWindow {
	out := make([]RSVPAvailabilityWindow, 0, rsvpSuggestFormRows)
	for i := 0; i < rsvpSuggestFormRows; i++ {
		date := strings.TrimSpace(c.FormValue(fmt.Sprintf("w%ddate", i)))
		from := strings.TrimSpace(c.FormValue(fmt.Sprintf("w%dfrom", i)))
		to := strings.TrimSpace(c.FormValue(fmt.Sprintf("w%dto", i)))
		if date == "" || from == "" || to == "" {
			continue
		}
		start, ok1 := parseHHMM(from)
		end, ok2 := parseHHMM(to)
		if !ok1 || !ok2 || start >= end {
			continue
		}
		out = append(out, RSVPAvailabilityWindow{OnDate: date, StartMinute: start, EndMinute: end})
	}
	return out
}

// parseHHMM converts an <input type="time"> value into minutes from midnight.
// Accepts "HH:MM" and the "HH:MM:SS" some browsers emit when seconds are shown.
func parseHHMM(v string) (int, bool) {
	parts := strings.Split(v, ":")
	if len(parts) < 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// resolveToken validates a token and resolves the event + calendar behind it,
// re-checking that the recipient is STILL a campaign member and can STILL see
// the event.
//
// FAIL-CLOSED on every leg. Re-checking at redemption (not just at mint time)
// is what stops an emailed link from outliving the access that justified it:
// a member removed from the campaign, or an event flipped to dm_only after the
// invite went out, gets the same generic invalid-link page — which also never
// reveals the event's title to someone who may no longer see it.
func (h *RSVPHandler) resolveToken(ctx context.Context, tokenStr string) (*EventRSVPToken, *Event, *Calendar, error) {
	tok, err := h.svc.ValidateToken(ctx, tokenStr)
	if err != nil {
		return nil, nil, nil, err
	}
	// campaignID "" — the token has no campaign in its path; it is resolved
	// from the event's own calendar and then used for the membership check.
	evt, cal, err := h.svc.EventContext(ctx, tok.EventID, "")
	if err != nil {
		return nil, nil, nil, apperror.NewBadRequest(rsvpBadTokenMsg)
	}
	if !evt.CollectRSVPs {
		return nil, nil, nil, apperror.NewBadRequest("this event is no longer collecting RSVPs")
	}
	role, ok := h.memberRole(ctx, cal.CampaignID, tok.UserID)
	if !ok || !h.svc.CanUserViewEvent(evt, role, tok.UserID) {
		return nil, nil, nil, apperror.NewBadRequest(rsvpBadTokenMsg)
	}
	return tok, evt, cal, nil
}

// memberRole resolves the effective VISIBILITY role of a user in a campaign for
// the unauthenticated token flow — the equivalent of CampaignContext
// .VisibilityRole(), which needs a request context this route doesn't have.
//
// FAIL-CLOSED: a nil directory or any lookup error denies, mirroring the
// sessions token flow's isCampaignMember.
func (h *RSVPHandler) memberRole(ctx context.Context, campaignID, userID string) (int, bool) {
	if h.members == nil {
		return 0, false
	}
	members, err := h.members.ListMembers(ctx, campaignID)
	if err != nil {
		return 0, false
	}
	for _, m := range members {
		if m.UserID != userID {
			continue
		}
		// A co-DM sees dm_only content without holding the Owner role, exactly
		// as CampaignContext.VisibilityRole() resolves it.
		if granted, err := h.members.IsUserDmGranted(ctx, campaignID, userID); err == nil && granted {
			return int(campaigns.RoleOwner), true
		}
		return int(m.Role), true
	}
	return 0, false
}

// applyOutThisWeek writes the member's own full-day unavailable exceptions for
// the event's week and returns the message shown back to them.
//
// Mirrors static/js/availability.js fireOutWeek: list the member's existing
// exception dates first and write ONLY the days that have none, so a
// hand-authored day is never overwritten. The resolved week is always named in
// the result so the action can't silently block a week they didn't expect.
func (h *RSVPHandler) applyOutThisWeek(ctx context.Context, cal *Calendar, evt *Event, userID string) string {
	weekStart, dates := rsvpWeekDates(cal, evt, nowUTC())
	base := fmt.Sprintf("You're marked as not attending %q.", trimForDisplay(evt.Name, 80))
	if h.availability == nil {
		return base + " (Scheduler availability isn't available on this instance, so only the RSVP was recorded.)"
	}

	existing, err := h.availability.ExceptionDates(ctx, cal.CampaignID, userID)
	if err != nil {
		slog.Warn("calendar rsvp: reading availability exceptions failed",
			slog.Any("error", err), slog.String("campaign_id", cal.CampaignID))
		return base + " Your calendar availability could not be updated — please set it by hand."
	}
	has := make(map[string]bool, len(existing))
	for _, d := range existing {
		has[d] = true
	}
	toWrite := make([]string, 0, len(dates))
	skipped := 0
	for _, d := range dates {
		if has[d] {
			skipped++
			continue
		}
		toWrite = append(toWrite, d)
	}

	if len(toWrite) > 0 {
		if err := h.availability.MarkDaysUnavailable(ctx, cal.CampaignID, userID, toWrite); err != nil {
			slog.Warn("calendar rsvp: writing availability exceptions failed",
				slog.Any("error", err), slog.String("campaign_id", cal.CampaignID))
			return base + " Your calendar availability could not be updated — please set it by hand."
		}
	}

	msg := fmt.Sprintf("%s You're marked unavailable for %d of the 7 days in the week of %s.",
		base, len(toWrite), weekStart)
	if skipped > 0 {
		msg += fmt.Sprintf(" %d day(s) already had your own custom availability and were left alone.", skipped)
	}
	return msg
}

// applySuggestion is the shared "I can't make that — here's what would work"
// path for BOTH the in-app buttons and the emailed token, so the two surfaces
// can never drift apart.
//
// It accepts a free-text note, structured availability windows, or both, and
// requires at least one. The windows are the schedulable half: they become real
// `available` exceptions in the scheduler, so the Director's overlay and its
// computed best-window can actually use them, instead of prose somebody has to
// read and re-key. Writing availability is best-effort — the RSVP note and the
// owner notification are the promise this flow makes, and a scheduler that is
// absent or degraded must not swallow the member's answer.
func (h *RSVPHandler) applySuggestion(ctx context.Context, campaignID string, evt *Event,
	userID string, role int, note string, windows []RSVPAvailabilityWindow) (string, error) {

	if len(windows) > maxRSVPWindows {
		return "", apperror.NewBadRequest(fmt.Sprintf("at most %d time windows at once", maxRSVPWindows))
	}
	if strings.TrimSpace(note) == "" && len(windows) == 0 {
		return "", apperror.NewValidation("add a time that would work, or a short note")
	}

	// The note is what persists on the RSVP row. When the member gave only
	// windows, synthesise one so the Director sees something on the response
	// list rather than a blank.
	effectiveNote := strings.TrimSpace(note)
	if effectiveNote == "" {
		effectiveNote = "Could do: " + formatWindowList(windows)
	}
	if err := h.svc.SuggestTime(ctx, evt, userID, role, effectiveNote); err != nil {
		return "", err
	}

	notice := "Thanks — your suggestion was sent to the organiser."
	if len(windows) > 0 {
		if h.availability == nil {
			notice = "Thanks — your note was sent. (Scheduler availability isn't set up on this instance, so the times weren't saved to your calendar.)"
		} else if err := h.availability.OfferAvailableWindows(ctx, campaignID, userID, windows); err != nil {
			slog.Warn("calendar rsvp: writing offered availability failed",
				slog.Any("error", err), slog.String("campaign_id", campaignID))
			notice = "Thanks — your note was sent, but the times couldn't be added to your availability. You can set them by hand on the availability page."
		} else {
			notice = "Thanks — your times were added to your availability and sent to the organiser."
		}
	}

	h.notifySuggestion(ctx, campaignID, evt, userID, effectiveNote)
	return notice, nil
}

// formatWindowList renders offered windows for a note / notification.
func formatWindowList(windows []RSVPAvailabilityWindow) string {
	parts := make([]string, 0, len(windows))
	for _, w := range windows {
		parts = append(parts, w.FormatWindow())
	}
	return strings.Join(parts, ", ")
}

// notifySuggestion pings the event's owner that a member proposed a better time.
//
// It records a NOTE and a notification — nothing more. It deliberately does NOT
// mint a slot proposal: proposal creation is Scribe+ by ruling
// (sessions/routes.go:53), and a Player clicking a link in an email must not
// escalate into that capability.
func (h *RSVPHandler) notifySuggestion(ctx context.Context, campaignID string, evt *Event, userID, note string) {
	if h.notifier == nil || evt.CreatedBy == nil || *evt.CreatedBy == "" {
		return
	}
	name := h.displayName(ctx, campaignID, userID)
	msg := fmt.Sprintf("%s suggested another time for %q: %s",
		name, trimForDisplay(evt.Name, 60), trimForDisplay(note, 160))
	if err := h.notifier.NotifyRSVP(ctx, []string{*evt.CreatedBy}, campaignID, msg, rsvpEventLink(campaignID, evt)); err != nil {
		slog.Warn("calendar rsvp: suggestion notification failed", slog.Any("error", err), slog.String("event_id", evt.ID))
	}
}
