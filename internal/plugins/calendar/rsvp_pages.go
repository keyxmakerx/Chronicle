package calendar

import "html"

// rsvpBaseURL is the authed per-event RSVP route prefix used by the in-app panel.
func rsvpBaseURL(campaignID, calID, eventID string) string {
	return "/campaigns/" + campaignID + "/calendars/" + calID + "/events/" + eventID
}

// rsvpPanelTargetID is the DOM id of the in-app panel wrapper (hx-swap target).
func rsvpPanelTargetID(eventID string) string { return "rsvp-panel-" + eventID }

// rsvpBtnClass highlights the caller's current choice: the active status button
// gets the primary style, the rest the secondary style.
func rsvpBtnClass(myStatus, status string) string {
	if myStatus == status {
		return "btn-primary text-xs"
	}
	return "btn-secondary text-xs"
}

// rsvpActionVals is the hx-vals JSON body for an in-app RSVP action button.
func rsvpActionVals(action string) string { return `{"action":"` + action + `"}` }

// rsvpStatusLabel renders an RSVP status for the counts/facepile UI.
func rsvpStatusLabel(status string) string {
	switch status {
	case RSVPStatusYes:
		return "Going"
	case RSVPStatusMaybe:
		return "Maybe"
	case RSVPStatusNo:
		return "Can't make it"
	default:
		return status
	}
}

// Standalone HTML pages for the public emailed-token RSVP flow (C-CAL-RSVP-P1).
// Mirrors the sessions plugin's token pages (sessions/handler.go:557-602): plain
// self-contained pages (no app layout, no auth) with the GET-confirm/POST-apply
// split + CSRF double-submit. All interpolated values are escaped; the form
// action is a same-origin token path. The hidden CSRF field name matches
// middleware.csrfFormField ("csrf_token") because these POST routes ride the
// global CSRF middleware (only /api/* and /ws are exempt): the GET already
// minted the cookie, so the double-submit matches on POST.

// rsvpTokenActionLabel renders a token action for the confirm interstitial.
func rsvpTokenActionLabel(action string) string {
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

// rsvpTokenAppliedMessage is the success line after a token is applied.
func rsvpTokenAppliedMessage(action string) string {
	switch action {
	case RSVPActionYes:
		return "You're marked as going. You can close this page."
	case RSVPActionMaybe:
		return "You're marked as maybe. You can close this page."
	case RSVPActionNo:
		return "You're marked as not attending. You can close this page."
	case RSVPActionOutWeek:
		return "You're marked out for this week — your availability was updated. You can close this page."
	case RSVPActionSuggest:
		return "Thanks — your suggestion was sent to the organizer. You can close this page."
	default:
		return "Your response has been recorded. You can close this page."
	}
}

// rsvpTokenPageHead is the shared <head> + card CSS for the token pages.
const rsvpTokenPageHead = `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>RSVP - Chronicle</title>
<style>body{font-family:system-ui,-apple-system,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f8f9fa;color:#333}
.card{text-align:center;padding:2.5rem;border-radius:12px;background:#fff;box-shadow:0 2px 12px rgba(0,0,0,.08);max-width:420px;width:90%}
.icon{font-size:2.5rem;margin-bottom:1rem}h1{font-size:1.25rem;margin:0 0 .5rem}
p{color:#666;margin:0 0 1.25rem;font-size:.95rem}
textarea{width:100%;box-sizing:border-box;min-height:96px;padding:.6rem;border:1px solid #ddd;border-radius:8px;font:inherit;margin-bottom:1rem}
button{font:inherit;font-weight:600;padding:.65rem 1.6rem;border:0;border-radius:8px;background:#6366f1;color:#fff;cursor:pointer}</style></head><body>`

// rsvpTokenResultHTML is the terminal success/failure page.
func rsvpTokenResultHTML(title, message string, success bool) string {
	icon := "✗"
	color := "#ef4444"
	if success {
		icon = "✓"
		color = "#22c55e"
	}
	return rsvpTokenPageHead +
		`<div class="card"><div class="icon" style="color:` + color + `">` + icon + `</div>` +
		`<h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(message) + `</p></div></body></html>`
}

// rsvpTokenConfirmHTML is the GET interstitial: a single POST button the user
// must click to apply (defeats mail-scanner prefetch — a scanner issues a GET).
func rsvpTokenConfirmHTML(title, message, actionURL, confirmLabel, csrfToken string) string {
	return rsvpTokenPageHead +
		`<div class="card"><div class="icon" style="color:#6366f1">📅</div>` +
		`<h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(message) + `</p>` +
		`<form method="POST" action="` + html.EscapeString(actionURL) + `">` +
		`<input type="hidden" name="csrf_token" value="` + html.EscapeString(csrfToken) + `">` +
		`<button type="submit">` + html.EscapeString(confirmLabel) + `</button></form>` +
		`</div></body></html>`
}

// rsvpSuggestFormHTML is the GET page for the "suggest another time" action: a
// free-text box POSTed as the RSVP note. Same CSRF double-submit as the confirm
// page.
func rsvpSuggestFormHTML(actionURL, csrfToken string) string {
	return rsvpTokenPageHead +
		`<div class="card"><div class="icon" style="color:#6366f1">✎</div>` +
		`<h1>Suggest another time</h1>` +
		`<p>Tell the organizer when would work better. This is sent as a note — it does not change the event.</p>` +
		`<form method="POST" action="` + html.EscapeString(actionURL) + `">` +
		`<input type="hidden" name="csrf_token" value="` + html.EscapeString(csrfToken) + `">` +
		`<textarea name="note" placeholder="e.g. Any evening after 8pm, or the following weekend" maxlength="500"></textarea>` +
		`<button type="submit">Send suggestion</button></form>` +
		`</div></body></html>`
}
