// RSVP action emails + the standalone HTML pages the emailed links land on
// (C-CAL-RSVP-P1).
//
// Split from rsvp_handler.go because this is presentation, not routing: the
// email body and the two token pages are plain strings with no echo.Context, so
// they are directly unit-testable (rsvp_email_test.go) without a request.
//
// Every interpolated value is html.EscapeString'd. Event names and campaign
// names are operator-authored free text, and these strings are assembled by
// concatenation rather than a template engine, so escaping is the ONLY thing
// standing between a "<img onerror=…>" event name and an injected email body —
// the same C-SCHED-P3 0c sweep the sessions invite carries.
package calendar

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// startInviteFanOut kicks off the invite fan-out without blocking the request.
//
// Background because SMTP is slow and remote: the operator's "Collect RSVPs"
// click must return immediately, and a dead mail server must not turn a UI
// toggle into a timeout. context.WithTimeout (not the request context) because
// the request's context is cancelled the moment the handler returns.
// `actorUserID` is the operator who armed the gate. It is carried in because the
// per-recipient floor's send log records WHO mailed each recipient
// (calendar_schedule_asks.actor_user_id, NOT NULL) — see fanOutInvites.
func (h *RSVPHandler) startInviteFanOut(campaignID, campaignName string, cal *Calendar, evt *Event, actorUserID string) {
	if h.members == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		h.fanOutInvites(ctx, campaignID, campaignName, cal, evt, actorUserID)
	}()
}

// fanOutInvites notifies every campaign member who can SEE the event.
//
// Two gates, in this order:
//  1. MEMBERS ONLY — the roster, never "everyone with campaign access". A public
//     campaign's anonymous visitors are not invited to anything.
//  2. VISIBILITY — each member is tested with the event's own visibility rules
//     before ANY channel is used. A dm_only event must not put its title in a
//     Player's inbox or bell; that is the same leak class the entity-ties lane
//     just closed, and email is the one channel you cannot retract.
//
// The in-app notification is sent to everyone who passes both gates. The EMAIL
// additionally needs a configured SMTP service and a member email address —
// with neither, in-app RSVP still works end to end (nil-safe by design).
// ── THE 24h PER-RECIPIENT FLOOR, REUSED AND NOT REINVENTED ─────────────────
// C-CALV4-GAMEREADY §4 [GR-6]. Toggling Collect RSVPs off and then on re-mailed
// the ENTIRE roster with no cooldown of any kind — and §4 makes this toggle far
// easier to reach (it moves from the legacy V2 drawer into the v4 day card),
// which makes that hazard worse rather than better.
//
// So this loop now reads the SAME floor the schedule-ask path already ships and
// the audit verified working: the same repository read (RecentlyAskedRecipients),
// the same SKIP-don't-refuse semantics, the same silence. A member mailed inside
// the window is passed over; everyone else is invited normally, so a second
// arming after somebody joins mails the new member and nobody else.
//
// THE 6h CAMPAIGN COOLDOWN IS DELIBERATELY NOT APPLIED. That one REFUSES the
// whole action, and refusing to arm the operator's own go/no-go gate because
// they armed something six hours ago is the polish-over-playability trade this
// slice forbids.
//
// A CONSEQUENCE, STATED RATHER THAN DISCOVERED: because both limits are read off
// one log, recording invite sends here means an arming also starts the 6h
// cooldown on the separate "Ask availability" control. That is the honest
// reading of a table whose stated purpose is "when was this roster last mailed"
// — the roster WAS just mailed — and it only ever affects the control that
// refuses, never this one.
//
// THE FLOOR APPLIES TO EMAIL ONLY. The in-app bell is not rate-limited: it is
// free, retractable and already honest, and suppressing it would hide a real
// state change from a member who is looking at the product.
func (h *RSVPHandler) fanOutInvites(ctx context.Context, campaignID, campaignName string, cal *Calendar, evt *Event, actorUserID string) {
	members, err := h.members.ListMembers(ctx, campaignID)
	if err != nil {
		slog.Warn("calendar rsvp: member list failed for invite fan-out",
			slog.Any("error", err), slog.String("campaign_id", campaignID))
		return
	}

	mailOK := h.mailer != nil && h.mailer.IsConfigured(ctx)

	// Read once, outside the loop. A FAILED read degrades to an EMPTY skip set
	// — the invite still goes out — because the operator's gate must arm even
	// when the bookkeeping is unavailable, and the worst case is the duplicate
	// email this floor exists to avoid rather than a party nobody invited.
	var skip map[string]bool
	if mailOK {
		skip, err = h.svc.RecentlyAskedRecipients(ctx, campaignID)
		if err != nil {
			slog.Warn("calendar rsvp: per-recipient floor unreadable; inviting everyone",
				slog.Any("error", err), slog.String("campaign_id", campaignID))
			skip = nil
		}
	}

	var notifyIDs []string

	for _, m := range members {
		if !h.memberMaySeeEvent(ctx, campaignID, m, evt) {
			continue
		}
		notifyIDs = append(notifyIDs, m.UserID)

		if !mailOK || m.Email == "" || skip[m.UserID] {
			continue
		}
		tokens, err := h.svc.MintActionTokens(ctx, evt.ID, m.UserID)
		if err != nil {
			slog.Warn("calendar rsvp: token mint failed",
				slog.Any("error", err), slog.String("user_id", m.UserID))
			continue
		}
		subject, plain, htmlBody := h.renderInviteEmail(campaignName, cal, evt, tokens)
		if err := h.mailer.SendHTMLMail(ctx, []string{m.Email}, subject, plain, htmlBody); err != nil {
			slog.Warn("calendar rsvp: invite email failed",
				slog.Any("error", err), slog.String("event_id", evt.ID))
			continue
		}
		// RECORDED ONLY AFTER THE MAILER TOOK IT WITHOUT ERROR — the log's own
		// invariant (migration 015): nothing is recorded when nothing was sent,
		// because a floor must never suppress mail to somebody who never got any.
		if err := h.svc.RecordScheduleAsk(ctx, campaignID, evt.ID, m.UserID, actorUserID); err != nil {
			slog.Warn("calendar rsvp: recording the invite send failed",
				slog.Any("error", err), slog.String("user_id", m.UserID))
		}
	}

	if h.notifier != nil && len(notifyIDs) > 0 {
		msg := fmt.Sprintf("RSVPs are open for %q", trimForDisplay(evt.Name, 80))
		if err := h.notifier.NotifyRSVP(ctx, notifyIDs, campaignID, msg, rsvpEventLink(campaignID, evt)); err != nil {
			slog.Warn("calendar rsvp: invite notification failed",
				slog.Any("error", err), slog.String("event_id", evt.ID))
		}
	}
}

// memberMaySeeEvent is THE per-recipient visibility gate, and there is exactly
// one of it.
//
// Lifted out of fanOutInvites' loop by C-CALV4-RSVP-P8B so the schedule ask can
// REUSE it rather than re-derive it. A second, subtly different visibility path
// is how the entity-ties leak happened, and email is the one channel you cannot
// retract: a dm_only event must not put its title in a Player's inbox, whether
// that inbox is reached by the OFF→ON fan-out or by the asking email.
//
// Co-DM promotion first (a co-DM sees dm_only content), then the event's own
// visibility rules. It is nil-safe on the directory: with no roster seam wired
// nobody is promoted, and CanUserViewEvent still decides.
func (h *RSVPHandler) memberMaySeeEvent(ctx context.Context, campaignID string, m campaigns.CampaignMember, evt *Event) bool {
	role := int(m.Role)
	if h.members != nil {
		if granted, err := h.members.IsUserDmGranted(ctx, campaignID, m.UserID); err == nil && granted {
			role = 3 // campaigns.RoleOwner — co-DMs see dm_only content.
		}
	}
	return h.svc.CanUserViewEvent(evt, role, m.UserID)
}

// renderInviteEmail builds the subject + plain-text + HTML bodies for one
// recipient. Structure follows the campaigns invite email
// (campaigns/invite_service.go): a small header, a detail card, then the action
// buttons — inline styles only, because email clients strip <style> blocks.
func (h *RSVPHandler) renderInviteEmail(campaignName string, cal *Calendar, evt *Event, tokens map[string]string) (subject, plain, htmlBody string) {
	name := trimForDisplay(evt.Name, 120)
	dateLine := rsvpDateLine(cal, evt)
	calName := ""
	if cal != nil {
		calName = cal.Name
	}
	subject = fmt.Sprintf("RSVP: %s — %s", name, campaignName)

	link := func(action string) string {
		return fmt.Sprintf("%s/calendar-rsvp/%s", h.baseURL, tokens[action])
	}

	var pb strings.Builder
	fmt.Fprintf(&pb, "You're invited to respond to a calendar event.\n\nEvent: %s\nCampaign: %s\nWhen: %s\n",
		name, campaignName, dateLine)
	if calName != "" {
		fmt.Fprintf(&pb, "Calendar: %s\n", calName)
	}
	pb.WriteString("\n")
	pb.WriteString(rsvpEmailActionsPlain(link))
	pb.WriteString("\n" + rsvpEmailSingleUseNotePlain)
	plain = pb.String()

	calRow := ""
	if calName != "" {
		calRow = rsvpEmailDetailRow("Calendar", calName)
	}

	htmlBody = rsvpEmailDoc("🗓️", "Are you coming?",
		rsvpEmailCard(`<h2 style="font-size:16px;margin:0 0 8px">`+escapeAttr(name)+`</h2>
  `+rsvpEmailDetailRow("Campaign", campaignName)+`
  `+rsvpEmailDetailRow("When", dateLine)+`
  `+calRow)+"\n"+
			rsvpEmailActionsHTML(link)+
			rsvpEmailSingleUseNoteHTML)

	return subject, plain, htmlBody
}

// --- shared email primitives ------------------------------------------------
//
// LIFTED TO PACKAGE LEVEL by C-CALV4-RSVP-P8B §6, from the closures that used
// to live inside renderInviteEmail. Two emails now leave this file — the invite
// and the schedule ask — and they must not drift about what a button looks
// like, what an action is called, or what promise the footer makes about a
// link. A copied closure is a promise that only holds until somebody edits one
// copy.
//
// Inline styles only, everywhere: email clients strip <style> blocks. Every
// interpolated value goes through escapeAttr — these strings are assembled by
// concatenation, and campaign names, event names and display names are all
// operator- or member-authored free text.

// rsvpEmailDoc is the document shell: the centred glyph + heading, then body.
func rsvpEmailDoc(glyph, heading, inner string) string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body style="font-family:system-ui,-apple-system,sans-serif;max-width:480px;margin:0 auto;padding:20px;color:#333">
<div style="text-align:center;margin-bottom:24px">
  <div style="font-size:32px;margin-bottom:8px">` + glyph + `</div>
  <h1 style="font-size:20px;margin:0">` + escapeAttr(heading) + `</h1>
</div>
` + inner + `
</body></html>`
}

// rsvpEmailCard is the grey detail block both emails open with.
func rsvpEmailCard(inner string) string {
	return `<div style="background:#f8f9fa;border-radius:8px;padding:20px;margin-bottom:24px">
  ` + inner + `
</div>`
}

// rsvpEmailDetailRow is one "Label: value" line inside a card. The VALUE is
// escaped; the label is a literal supplied by this package.
func rsvpEmailDetailRow(label, value string) string {
	return `<p style="margin:4px 0;color:#666;font-size:14px"><strong>` + label + `:</strong> ` +
		escapeAttr(value) + `</p>`
}

// rsvpEmailParagraph is one body sentence outside the card.
func rsvpEmailParagraph(text string) string {
	return `<p style="margin:0 0 12px;font-size:14px;line-height:1.5">` + escapeAttr(text) + `</p>`
}

// rsvpEmailPrimaryButton is the one big call to action. Distinct from the RSVP
// answer buttons below only in that it stands alone.
func rsvpEmailPrimaryButton(href, label, bg string) string {
	return `<a href="` + escapeAttr(href) + `" style="display:inline-block;padding:10px 22px;background:` + bg +
		`;color:#fff;text-decoration:none;border-radius:6px;font-weight:600;margin:0 6px 8px">` +
		escapeAttr(label) + `</a>`
}

// rsvpEmailSecondaryLink is a text link: the two richer RSVP actions render
// this way so the card does not read as five equals.
func rsvpEmailSecondaryLink(href, label string) string {
	return `<a href="` + escapeAttr(href) + `" style="color:#6366f1;text-decoration:underline;margin:0 8px">` +
		escapeAttr(label) + `</a>`
}

// rsvpEmailActionColours pairs each primary answer with its button colour. It
// is keyed by action rather than positional so it cannot drift out of step with
// rsvpEmailActions, which stays the single ordered action set.
var rsvpEmailActionColours = map[string]string{
	RSVPActionYes:   "#22c55e",
	RSVPActionMaybe: "#f59e0b",
	RSVPActionNo:    "#ef4444",
}

// rsvpEmailActionsHTML renders the five action links in rsvpEmailActions order:
// the three answers as buttons, the two richer actions as secondary links.
// `link` maps an action to its already-minted token URL.
func rsvpEmailActionsHTML(link func(action string) string) string {
	var buttons, secondary strings.Builder
	for _, a := range rsvpEmailActions {
		if bg, primary := rsvpEmailActionColours[a]; primary {
			buttons.WriteString(rsvpEmailPrimaryButton(link(a), rsvpActionLabel(a), bg))
			continue
		}
		secondary.WriteString(rsvpEmailSecondaryLink(link(a), rsvpActionLabel(a)))
	}
	return `<div style="text-align:center;margin-bottom:20px">
  ` + buttons.String() + `
</div>
<div style="text-align:center;margin-bottom:24px;font-size:13px">
  ` + secondary.String() + `
</div>`
}

// rsvpEmailActionsPlain is the same five links for the plain-text body, in the
// same order, so the two representations cannot disagree about what is on offer.
func rsvpEmailActionsPlain(link func(action string) string) string {
	var b strings.Builder
	for _, a := range rsvpEmailActions {
		fmt.Fprintf(&b, "%s: %s\n", rsvpActionLabel(a), link(a))
	}
	return b.String()
}

// rsvpEmailSingleUseNoteHTML / …Plain are THE promise the emailed action links
// make, stated once so both emails make the same one. The tokens really are
// per-action, single-use and 7-day (rsvpTokenTTL, rsvp_model.go), and a
// re-minted set inherits every one of those properties unchanged.
const rsvpEmailSingleUseNoteHTML = `
<p style="text-align:center;color:#999;font-size:12px">Each link works once and expires in 7 days.</p>`

const rsvpEmailSingleUseNotePlain = "These links expire in 7 days and each one can be used once.\n"

// --- Standalone pages for the public token routes ---
//
// Self-contained (no external stylesheet, no CDN request): these render for a
// logged-out recipient straight out of an email client, so the fewer moving
// parts the better.

// rsvpPageShell wraps a card body in the shared standalone document.
func rsvpPageShell(title, accent, body string) string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + escapeAttr(title) + ` - Chronicle</title>
<style>body{font-family:system-ui,-apple-system,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f8f9fa}
.card{text-align:center;padding:2.5rem;border-radius:12px;background:#fff;box-shadow:0 2px 12px rgba(0,0,0,.08);max-width:420px;width:calc(100% - 2rem)}
.dot{width:44px;height:44px;border-radius:50%;margin:0 auto 1rem;background:` + accent + `}
h1{font-size:1.25rem;margin:0 0 .5rem;color:#111}
p{color:#666;margin:0 0 1.25rem;font-size:.92rem;line-height:1.5}
textarea,input[type=date],input[type=time]{font:inherit;width:100%;box-sizing:border-box;padding:.55rem;border:1px solid #d4d4d8;border-radius:8px;background:#fff;color:#111}
textarea{margin-bottom:1rem}
.wrow{text-align:left;margin-bottom:.7rem}
.wrow label,.notelabel{display:block;font-size:.78rem;font-weight:600;color:#52525b;margin-bottom:.3rem}
.wgrid{display:grid;grid-template-columns:1.4fr 1fr 1fr;gap:.4rem}
.prob{color:#b91c1c;font-weight:600}
a{color:#4f46e5}
@media (max-width:420px){.wgrid{grid-template-columns:1fr}}
button{font:inherit;font-weight:600;padding:.65rem 1.6rem;border:0;border-radius:8px;background:#6366f1;color:#fff;cursor:pointer}</style>
</head><body><div class="card">` + body + `</div></body></html>`
}

// rsvpResultPage reports what happened AND offers a way back into the product.
//
// IT USED TO BE TERMINAL — "what happened, nothing to click" — and that was the
// whole of C-CALV4-GAMEREADY §5 [GR-8]'s second dead end: the page contained no
// `<a>` at all and never mentioned `/campaigns/`, so a member who answered from
// their inbox landed on a card with no route anywhere. The success page is the
// one every answering member sees, so it is the one that most needed a door.
//
// `backHref` EMPTY IS A DECISION, NOT A DEFAULT. The generic invalid-link page
// is rendered for viewers who may no longer be members and may no longer see
// the event, and its contract is that it discloses NOTHING — not the title, and
// not which campaign the link belonged to. So it passes "" and keeps its shape
// exactly as the audit verified it.
func rsvpResultPage(title, message string, success bool, backHref string) string {
	accent := "#ef4444"
	if success {
		accent = "#22c55e"
	}
	back := ""
	if backHref != "" {
		back = `<p><a href="` + escapeAttr(backHref) + `">Go to the schedule to change your answer</a></p>`
	}
	return rsvpPageShell(title, accent,
		`<div class="dot"></div><h1>`+escapeAttr(title)+`</h1><p>`+escapeAttr(message)+`</p>`+back)
}

// rsvpSchedulePath is the one destination the token pages link back to, and it
// is the SHIPPED campaign schedule page (`GET /campaigns/:id/schedule`,
// routes.go) — which already owns the tri-state answer control, so a member who
// wants to change their mind lands on the surface that can do it rather than on
// a second inline form nobody has to maintain.
func rsvpSchedulePath(campaignID string) string {
	return "/campaigns/" + campaignID + "/schedule"
}

// rsvpConfirmPage is the GET interstitial: a POST form the recipient must
// submit. Because a mail scanner or link prefetcher issues a GET and never a
// POST, this page is what stops an automated fetch from recording an RSVP.
//
// csrfToken rides a hidden field: these POSTs are NOT under the CSRF-exempt
// /api/ or /ws prefixes, and the GET that rendered this page already passed
// through the middleware and minted the cookie, so the double-submit matches.
func rsvpConfirmPage(title, message, actionURL, confirmLabel, csrfToken string) string {
	return rsvpPageShell(title, "#6366f1",
		`<div class="dot"></div><h1>`+escapeAttr(title)+`</h1><p>`+escapeAttr(message)+`</p>`+
			`<form method="POST" action="`+escapeAttr(actionURL)+`">`+
			`<input type="hidden" name="csrf_token" value="`+escapeAttr(csrfToken)+`">`+
			`<button type="submit">`+escapeAttr(confirmLabel)+`</button></form>`)
}

// rsvpSuggestFormRows is how many date/time rows the emailed suggestion form
// offers. Three is enough to express "any of these evenings" without turning an
// email landing page into a scheduler.
const rsvpSuggestFormRows = 3

// rsvpSuggestPage is the "suggest another time" form: structured
// date + from + to rows PLUS an optional free-text note.
//
// The rows are the point. A note tells the Director something they have to read
// and re-key; a row becomes real temporary availability in the scheduler, so it
// shows up in the overlay and counts toward the computed best window. Either is
// accepted, so a member with a vague answer is never blocked — the server
// requires at least one of the two.
//
// Plain HTML, no JavaScript: this renders for a possibly-logged-out member in
// whatever browser their email client hands off to. `type="date"` / `type="time"`
// degrade to text inputs on anything that doesn't support them, and the parser
// simply skips a row it can't read.
//
// `errMsg` is the RE-RENDER's reason ([GR-7], C-CALV4-GAMEREADY §5) and is
// empty on the first render. A refused submission comes BACK HERE rather than
// dead-ending on the failure page, because the token that carried it is
// deliberately still unspent — the member fixes the row and sends it again.
//
// Same GET-renders / POST-applies split as the confirm page.
func rsvpSuggestPage(detail, actionURL, csrfToken, errMsg string) string {
	var rows strings.Builder
	for i := 0; i < rsvpSuggestFormRows; i++ {
		idx := fmt.Sprint(i)
		label := "Another time that works"
		if i == 0 {
			label = "A time that works for you"
		}
		rows.WriteString(
			`<div class="wrow"><label for="w` + idx + `date">` + escapeAttr(label) + `</label>` +
				`<div class="wgrid">` +
				`<input type="date" id="w` + idx + `date" name="w` + idx + `date" aria-label="Date">` +
				`<input type="time" name="w` + idx + `from" aria-label="From" placeholder="from">` +
				`<input type="time" name="w` + idx + `to" aria-label="To" placeholder="to">` +
				`</div></div>`)
	}

	// The reason renders ABOVE the form and inside `role="alert"`, so a member
	// on a screen reader hears why the send did not go through rather than
	// finding an unchanged form and guessing.
	problem := ""
	if errMsg != "" {
		problem = `<p class="prob" role="alert">` + escapeAttr(errMsg) + `</p>`
	}

	return rsvpPageShell("Suggest another time", "#6366f1",
		`<div class="dot"></div><h1>When could you make it?</h1>`+
			`<p>`+escapeAttr(detail)+`<br>Add any times that would work — they'll be added to your `+
			`availability so the organiser can see them on the schedule.</p>`+
			problem+
			`<form method="POST" action="`+escapeAttr(actionURL)+`">`+
			`<input type="hidden" name="csrf_token" value="`+escapeAttr(csrfToken)+`">`+
			rows.String()+
			`<label for="note" class="notelabel">Anything else? (optional)</label>`+
			`<textarea id="note" name="note" rows="3" maxlength="`+fmt.Sprint(maxRSVPNoteLen)+
			`" placeholder="e.g. any evening after 8pm, or Sunday afternoon"></textarea>`+
			`<button type="submit">Send</button></form>`)
}

// --- the schedule ask (C-CALV4-RSVP-P8B) ------------------------------------

// scheduleAskEmailInput is one recipient's copy of the asking email.
//
// ONE EMAIL, ONE SEND, TWO SECTIONS ([PB-1] signed (b)). The subject and the
// primary call to action are the SCHEDULE ASK — the operator's own sentence,
// and the half that needs no session at all, so the email is valid in a
// campaign that has scheduled nothing, which is precisely the campaign that
// most needs asking. The five existing RSVP action links ride the SAME email
// when a collecting session is resolvable AND this recipient may see it.
//
// Event/Calendar/Tokens are therefore all-or-nothing and are set by the caller
// only after the visibility gate has passed for THIS member. This renderer does
// not re-derive that gate — a second implementation of it is exactly the failure
// class the fan-out's own header warns about.
type scheduleAskEmailInput struct {
	CampaignID   string
	CampaignName string
	// ActorName is the Director's display name, resolved through the ROSTER
	// (RSVPHandler.displayName), never a users JOIN.
	ActorName string
	Event     *Event
	Calendar  *Calendar
	Tokens    map[string]string
}

// scheduleAskAvailabilityPath is the deep link the primary CTA carries: the
// SHIPPED availability page, which is already a painted weekly grid with a
// per-day exceptions editor under it.
//
// A PLAIN DEEP LINK, NOT A TOKENED ONE ([PB-2] signed (i)). The grid is a
// client-rendered surface over six authenticated JSON endpoints, so "a tokened
// link to the grid" would mean token-authenticating all six or minting a
// session from an emailed link — the largest authorisation-surface change
// anyone has proposed in calendar-v4, for a page whose whole content is the
// member's own data. The directive's one-time-token pattern IS honoured in this
// same email: the RSVP action links are that pattern, they work from a
// logged-out inbox, and they are unchanged. What makes the deep link land is
// stage 1's destination preservation, not a new credential.
func scheduleAskAvailabilityPath(campaignID string) string {
	return "/campaigns/" + campaignID + "/availability"
}

// The three sentences of the ask, matching what the shipped page actually does
// (availability.templ's own header, and the per-day exceptions editor beneath
// it). Stated as constants because the email must not describe a page that
// works differently.
const (
	scheduleAskLead      = "Paint the hours you can play — it repeats every week, so you set your normal hours once."
	scheduleAskException = "If one week is different, mark that day on its own; the odd week does not disturb your normal hours."
	scheduleAskTypeItIn  = "You can also edit a single day directly, if typing it in is easier than painting it."
	// scheduleAskWhy is the last line of every copy. There is no unsubscribe
	// link and none is invented: Chronicle has no notification preferences at
	// all, which C-NOTIFY-PREFS is booked to fix — inventing a dead link here
	// would be worse than saying plainly why the mail arrived.
	scheduleAskWhy = "You received this because you are a member of this campaign."
)

// renderScheduleAskEmail builds the subject + plain + HTML bodies of the asking
// email — a SIBLING of renderInviteEmail in this file, sharing its helpers, its
// single escaping convention and its one ordered action set.
func (h *RSVPHandler) renderScheduleAskEmail(in scheduleAskEmailInput) (subject, plain, htmlBody string) {
	campaign := trimForDisplay(in.CampaignName, 120)
	actor := trimForDisplay(in.ActorName, 80)
	gridURL := h.baseURL + scheduleAskAvailabilityPath(in.CampaignID)

	subject = fmt.Sprintf("When can you play? — %s", campaign)

	// The RSVP half is attached only when the caller resolved a visible,
	// collecting session AND minted this recipient's tokens.
	withRSVP := in.Event != nil && len(in.Tokens) > 0
	link := func(action string) string {
		return fmt.Sprintf("%s/calendar-rsvp/%s", h.baseURL, in.Tokens[action])
	}

	var pb strings.Builder
	fmt.Fprintf(&pb, "%s is trying to find a time the table can play, and would like to know your schedule.\n\n", actor)
	fmt.Fprintf(&pb, "Campaign: %s\n\n", campaign)
	fmt.Fprintf(&pb, "%s\n%s\n%s\n\n", scheduleAskLead, scheduleAskException, scheduleAskTypeItIn)
	fmt.Fprintf(&pb, "Set your availability: %s\n", gridURL)
	if withRSVP {
		fmt.Fprintf(&pb, "\nWhile you're here — there is a session on the books:\n\nEvent: %s\nWhen: %s\n\n",
			trimForDisplay(in.Event.Name, 120), rsvpDateLine(in.Calendar, in.Event))
		pb.WriteString(rsvpEmailActionsPlain(link))
		pb.WriteString("\n" + rsvpEmailSingleUseNotePlain)
	}
	fmt.Fprintf(&pb, "\n%s\n", scheduleAskWhy)
	plain = pb.String()

	body := rsvpEmailCard(
		`<h2 style="font-size:16px;margin:0 0 8px">`+escapeAttr(campaign)+`</h2>
  `+rsvpEmailDetailRow("Asked by", actor)) + "\n" +
		rsvpEmailParagraph(scheduleAskLead) + "\n" +
		rsvpEmailParagraph(scheduleAskException) + "\n" +
		rsvpEmailParagraph(scheduleAskTypeItIn) + "\n" +
		`<div style="text-align:center;margin:20px 0 24px">
  ` + rsvpEmailPrimaryButton(gridURL, "Set your availability", "#6366f1") + `
</div>`

	if withRSVP {
		body += "\n" + rsvpEmailCard(
			`<h2 style="font-size:16px;margin:0 0 8px">`+escapeAttr(trimForDisplay(in.Event.Name, 120))+`</h2>
  `+rsvpEmailDetailRow("When", rsvpDateLine(in.Calendar, in.Event))) + "\n" +
			rsvpEmailActionsHTML(link) + rsvpEmailSingleUseNoteHTML
	}
	body += "\n" + `<p style="text-align:center;color:#999;font-size:12px">` + escapeAttr(scheduleAskWhy) + `</p>`

	htmlBody = rsvpEmailDoc("🗓️", "When can you play?", body)
	return subject, plain, htmlBody
}

// --- the schedule ask's fan-out ---------------------------------------------

// scheduleAskRecipient is one member the handler has already decided to mail:
// on the roster, with an address on file, and outside the per-recipient floor.
// The decision is made SYNCHRONOUSLY (so the response can state a count) and
// the sending is done in the background (so a dead mail server cannot turn a
// button into a timeout).
type scheduleAskRecipient struct {
	Member campaigns.CampaignMember
	// MaySeeEvent is this recipient's own answer to the visibility gate,
	// evaluated once by the handler through memberMaySeeEvent. Only these
	// recipients get the RSVP half of the email.
	MaySeeEvent bool
}

// startScheduleAskFanOut sends the asking email without blocking the request.
//
// Same shape and same reasoning as startInviteFanOut: context.WithTimeout over
// context.Background(), because the request's context dies the moment the
// handler returns, and because a dead mail server must not turn a UI control
// into a timeout. The consequence the caller must live with is that the
// response cannot report per-recipient results — so the actor is told "asking N
// members" and the cooldown rows are written HERE, per recipient, as each send
// succeeds.
func (h *RSVPHandler) startScheduleAskFanOut(in scheduleAskEmailInput, actorUserID string, recipients []scheduleAskRecipient) {
	if len(recipients) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		h.fanOutScheduleAsk(ctx, in, actorUserID, recipients)
	}()
}

// fanOutScheduleAsk mails one asking email per recipient and records each send.
//
// NOTHING IS RECORDED WHEN NOTHING WAS SENT. The ask row is written only after
// SendHTMLMail returns nil, never optimistically up front, so a mail server
// that refuses every message leaves the cooldown untouched and the Director can
// try again once it is fixed.
func (h *RSVPHandler) fanOutScheduleAsk(ctx context.Context, in scheduleAskEmailInput, actorUserID string, recipients []scheduleAskRecipient) {
	if h.mailer == nil {
		return
	}
	eventID := ""
	if in.Event != nil {
		eventID = in.Event.ID
	}

	for _, r := range recipients {
		copyIn := in
		// The RSVP half rides along ONLY for a recipient who may see the event.
		// Tokens are minted per recipient, so a member who cannot see it never
		// has a link to it in the first place.
		if in.Event == nil || !r.MaySeeEvent {
			copyIn.Event, copyIn.Calendar, copyIn.Tokens = nil, nil, nil
		} else {
			tokens, err := h.svc.MintActionTokens(ctx, in.Event.ID, r.Member.UserID)
			if err != nil {
				slog.Warn("calendar rsvp: schedule-ask token mint failed",
					slog.Any("error", err), slog.String("user_id", r.Member.UserID))
				// Not fatal: the schedule ask itself does not need a session, so
				// the email still goes out — just without the RSVP section.
				copyIn.Event, copyIn.Calendar, copyIn.Tokens = nil, nil, nil
			} else {
				copyIn.Tokens = tokens
			}
		}

		subject, plain, htmlBody := h.renderScheduleAskEmail(copyIn)
		if err := h.mailer.SendHTMLMail(ctx, []string{r.Member.Email}, subject, plain, htmlBody); err != nil {
			slog.Warn("calendar rsvp: schedule-ask email failed",
				slog.Any("error", err), slog.String("campaign_id", in.CampaignID))
			continue // no send, no row.
		}
		// One row per recipient actually handed to the mailer without error.
		// The event id recorded is the one the SEND was about, not the one this
		// recipient could see, so the log says what the campaign was asked with.
		if err := h.svc.RecordScheduleAsk(ctx, in.CampaignID, eventID, r.Member.UserID, actorUserID); err != nil {
			slog.Warn("calendar rsvp: schedule-ask bookkeeping failed",
				slog.Any("error", err), slog.String("campaign_id", in.CampaignID))
		}
	}
}
