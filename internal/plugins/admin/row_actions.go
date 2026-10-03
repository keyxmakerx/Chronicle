package admin

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// rowAction is one action of a list row. The People and Campaigns lists are
// declared once here and rendered twice (inline in the desktop table, in the
// "⋯" menu on the phone card), so the two layouts can never drift apart in
// request, confirm text or CSRF handling.
type rowAction struct {
	// Label is the desktop text; MenuLabel the phone-menu text (risky items
	// end in "…" because they ask first).
	Label, MenuLabel string
	// Method is the htmx verb: put, post or delete.
	Method, URL string
	// Confirm is the hx-confirm text; empty means the action runs at once.
	Confirm string
	// Tone picks the colour: "", "danger", "accent", "ok" or "warn".
	Tone string
	// Vals is a JSON object of extra form fields (the Join-as role).
	Vals string
	// Icon renders the desktop button as an icon with Label as its tooltip.
	Icon bool
	// Aria overrides the accessible name where Label alone is ambiguous.
	Aria string
	// DividerBefore separates the destructive tail of a menu.
	DividerBefore bool
}

// attrs builds the htmx attributes. CSRF travels both as a header and a form
// field, exactly as the forms it replaces did, and every value is escaped by
// templ rather than spliced into markup.
func (a rowAction) attrs(csrf string) templ.Attributes {
	hdr, _ := json.Marshal(map[string]string{"X-CSRF-Token": csrf})
	m := templ.Attributes{"hx-" + a.Method: a.URL, "hx-headers": string(hdr)}
	if a.Confirm != "" {
		m["hx-confirm"] = a.Confirm
	}
	if a.Vals != "" {
		m["hx-vals"] = a.Vals
	}
	return m
}

// rowToneClass is the inline (desktop) colour for a tone.
func rowToneClass(tone string) string {
	switch tone {
	case "danger":
		return "link-danger"
	case "accent":
		return "text-accent hover:text-accent-hover"
	case "ok":
		return "text-emerald-600 hover:text-emerald-800 dark:text-emerald-400 dark:hover:text-emerald-300"
	case "warn":
		return "text-orange-600 hover:text-orange-800 dark:text-orange-400 dark:hover:text-orange-300"
	}
	return "text-fg-muted hover:text-fg"
}

// rowMenuToneClass is the menu-item colour for a tone; destructive items are red.
func rowMenuToneClass(tone string) string {
	switch tone {
	case "danger":
		return "text-red-600 dark:text-red-400"
	case "accent":
		return "text-accent"
	case "warn":
		return "text-orange-600 dark:text-orange-400"
	case "ok":
		return "text-emerald-600 dark:text-emerald-400"
	}
	return "text-fg-body"
}

// userRowActions lists the actions of a People row. An admin cannot be
// disabled from here, matching the table's existing rule.
func userRowActions(u auth.User) []rowAction {
	var acts []rowAction
	if u.IsAdmin {
		acts = append(acts, rowAction{Label: "Remove Admin", MenuLabel: "Remove admin…", Method: "put",
			URL: fmt.Sprintf("/admin/users/%s/admin", u.ID), Confirm: "Remove admin privileges from this user?", Tone: "danger"})
	} else {
		acts = append(acts, rowAction{Label: "Make Admin", MenuLabel: "Make admin…", Method: "put",
			URL: fmt.Sprintf("/admin/users/%s/admin", u.ID), Confirm: "Grant admin privileges to this user?", Tone: "accent"})
	}
	if !u.IsAdmin {
		if u.IsDisabled {
			acts = append(acts, rowAction{Label: "Enable", MenuLabel: "Enable account…", Method: "put",
				URL: fmt.Sprintf("/admin/security/users/%s/enable", u.ID), Confirm: "Re-enable this user account?", Tone: "ok"})
		} else {
			acts = append(acts, rowAction{Label: "Disable", MenuLabel: "Disable account…", Method: "put",
				URL: fmt.Sprintf("/admin/security/users/%s/disable", u.ID), Confirm: "Disable this user account? They will be immediately logged out.", Tone: "warn"})
		}
	}
	acts = append(acts, rowAction{Label: "Force logout all sessions", MenuLabel: "Force logout…", Method: "post",
		URL: fmt.Sprintf("/admin/security/users/%s/force-logout", u.ID), Confirm: "Force logout all sessions for this user?",
		Icon: true, Aria: "Force logout all sessions for " + u.DisplayName})
	return acts
}

// campaignJoinActions are the "Join as" choices; ownership transfers, so it asks.
func campaignJoinActions(c campaigns.Campaign, csrf string) []rowAction {
	url := fmt.Sprintf("/admin/campaigns/%s/join", c.ID)
	vals := func(role string) string {
		b, _ := json.Marshal(map[string]string{"role": role, "csrf_token": csrf})
		return string(b)
	}
	return []rowAction{
		{Label: "Player", MenuLabel: "Join as player", Method: "post", URL: url, Vals: vals("player")},
		{Label: "Scribe", MenuLabel: "Join as scribe", Method: "post", URL: url, Vals: vals("scribe")},
		{Label: "Owner (transfers)", MenuLabel: "Join as owner (transfers)…", Method: "post", URL: url, Vals: vals("owner"), Tone: "warn",
			Confirm: "Join as Owner? Ownership moves to you and the current owner loses it."},
	}
}

// campaignRowActions are the non-join actions of a Campaigns row.
func campaignRowActions(c campaigns.Campaign) []rowAction {
	return []rowAction{
		{Label: "Leave", MenuLabel: "Leave campaign…", Method: "delete", URL: fmt.Sprintf("/admin/campaigns/%s/leave", c.ID),
			Confirm: "Leave this campaign? You will lose the access you gave yourself."},
		{Label: "Delete", MenuLabel: "Delete campaign…", Method: "delete", URL: fmt.Sprintf("/admin/campaigns/%s", c.ID),
			Confirm: "Permanently delete this campaign and all its data?", Tone: "danger"},
	}
}

// campaignCardActions is the phone menu: join choices first, then the rest,
// with the destructive tail set apart.
func campaignCardActions(c campaigns.Campaign, csrf string) []rowAction {
	acts := append(campaignJoinActions(c, csrf), campaignRowActions(c)...)
	acts[len(acts)-1].DividerBefore = true
	return acts
}

// userCardActions is the phone menu: the destructive account actions (disable,
// force logout) sit after a divider.
func userCardActions(u auth.User) []rowAction {
	acts := userRowActions(u)
	if len(acts) > 1 {
		acts[1].DividerBefore = true
	}
	return acts
}

// avatarInitial is the first letter of a name for the card avatar.
func avatarInitial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		return string(unicode.ToUpper(r))
	}
	return "?"
}
