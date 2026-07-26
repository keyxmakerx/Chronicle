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
)

// startInviteFanOut kicks off the invite fan-out without blocking the request.
//
// Background because SMTP is slow and remote: the operator's "Collect RSVPs"
// click must return immediately, and a dead mail server must not turn a UI
// toggle into a timeout. context.WithTimeout (not the request context) because
// the request's context is cancelled the moment the handler returns.
func (h *RSVPHandler) startInviteFanOut(campaignID, campaignName string, cal *Calendar, evt *Event) {
	if h.members == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		h.fanOutInvites(ctx, campaignID, campaignName, cal, evt)
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
func (h *RSVPHandler) fanOutInvites(ctx context.Context, campaignID, campaignName string, cal *Calendar, evt *Event) {
	members, err := h.members.ListMembers(ctx, campaignID)
	if err != nil {
		slog.Warn("calendar rsvp: member list failed for invite fan-out",
			slog.Any("error", err), slog.String("campaign_id", campaignID))
		return
	}

	mailOK := h.mailer != nil && h.mailer.IsConfigured(ctx)
	var notifyIDs []string

	for _, m := range members {
		role := int(m.Role)
		if granted, err := h.members.IsUserDmGranted(ctx, campaignID, m.UserID); err == nil && granted {
			role = 3 // campaigns.RoleOwner — co-DMs see dm_only content.
		}
		if !h.svc.CanUserViewEvent(evt, role, m.UserID) {
			continue
		}
		notifyIDs = append(notifyIDs, m.UserID)

		if !mailOK || m.Email == "" {
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
	for _, a := range rsvpEmailActions {
		fmt.Fprintf(&pb, "%s: %s\n", rsvpActionLabel(a), link(a))
	}
	pb.WriteString("\nThese links expire in 7 days and each one can be used once.\n")
	plain = pb.String()

	// Button row. Yes/Maybe/No are the primary answers; the two richer actions
	// render as secondary text links so the card doesn't read as five equals.
	btn := func(action, bg string) string {
		return `<a href="` + escapeAttr(link(action)) + `" style="display:inline-block;padding:10px 22px;background:` + bg +
			`;color:#fff;text-decoration:none;border-radius:6px;font-weight:600;margin:0 6px 8px">` +
			escapeAttr(rsvpActionLabel(action)) + `</a>`
	}
	secondary := func(action string) string {
		return `<a href="` + escapeAttr(link(action)) + `" style="color:#6366f1;text-decoration:underline;margin:0 8px">` +
			escapeAttr(rsvpActionLabel(action)) + `</a>`
	}

	calRow := ""
	if calName != "" {
		calRow = `<p style="margin:4px 0;color:#666;font-size:14px"><strong>Calendar:</strong> ` +
			escapeAttr(calName) + `</p>`
	}

	htmlBody = `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body style="font-family:system-ui,-apple-system,sans-serif;max-width:480px;margin:0 auto;padding:20px;color:#333">
<div style="text-align:center;margin-bottom:24px">
  <div style="font-size:32px;margin-bottom:8px">🗓️</div>
  <h1 style="font-size:20px;margin:0">Are you coming?</h1>
</div>
<div style="background:#f8f9fa;border-radius:8px;padding:20px;margin-bottom:24px">
  <h2 style="font-size:16px;margin:0 0 8px">` + escapeAttr(name) + `</h2>
  <p style="margin:4px 0;color:#666;font-size:14px"><strong>Campaign:</strong> ` + escapeAttr(campaignName) + `</p>
  <p style="margin:4px 0;color:#666;font-size:14px"><strong>When:</strong> ` + escapeAttr(dateLine) + `</p>
  ` + calRow + `
</div>
<div style="text-align:center;margin-bottom:20px">
  ` + btn(RSVPActionYes, "#22c55e") + btn(RSVPActionMaybe, "#f59e0b") + btn(RSVPActionNo, "#ef4444") + `
</div>
<div style="text-align:center;margin-bottom:24px;font-size:13px">
  ` + secondary(RSVPActionOutWeek) + secondary(RSVPActionSuggest) + `
</div>
<p style="text-align:center;color:#999;font-size:12px">Each link works once and expires in 7 days.</p>
</body></html>`

	return subject, plain, htmlBody
}

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
textarea{font:inherit;width:100%;box-sizing:border-box;padding:.6rem;border:1px solid #d4d4d8;border-radius:8px;margin-bottom:1rem}
button{font:inherit;font-weight:600;padding:.65rem 1.6rem;border:0;border-radius:8px;background:#6366f1;color:#fff;cursor:pointer}</style>
</head><body><div class="card">` + body + `</div></body></html>`
}

// rsvpResultPage is the terminal page: what happened, nothing to click.
func rsvpResultPage(title, message string, success bool) string {
	accent := "#ef4444"
	if success {
		accent = "#22c55e"
	}
	return rsvpPageShell(title, accent,
		`<div class="dot"></div><h1>`+escapeAttr(title)+`</h1><p>`+escapeAttr(message)+`</p>`)
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

// rsvpSuggestPage is the "suggest another time" free-text form. Same GET-renders
// / POST-applies split as the confirm page; the note rides the POST body.
func rsvpSuggestPage(detail, actionURL, csrfToken string) string {
	return rsvpPageShell("Suggest another time", "#6366f1",
		`<div class="dot"></div><h1>Suggest another time</h1>`+
			`<p>`+escapeAttr(detail)+`<br>What times would work better for you?</p>`+
			`<form method="POST" action="`+escapeAttr(actionURL)+`">`+
			`<input type="hidden" name="csrf_token" value="`+escapeAttr(csrfToken)+`">`+
			`<textarea name="note" rows="4" maxlength="`+fmt.Sprint(maxRSVPNoteLen)+
			`" placeholder="e.g. any evening after 8pm, or Sunday afternoon" required></textarea>`+
			`<button type="submit">Send suggestion</button></form>`)
}
