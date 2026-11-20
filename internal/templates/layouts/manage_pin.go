package layouts

import (
	"context"
	"strings"

	"github.com/a-h/templ"
)

// manageToolOfLine maps a desk line to the Manage tool it opens, so the desk
// and the frame pin the same key the sidebar's Pinned group stores. A line
// that opens no tool of its own (a blank, the end-of-campaign line) is absent.
var manageToolOfLine = map[string]string{
	"people":   "manage:members",
	"changes":  "manage:player-changes",
	"game":     "manage:extensions",
	"look":     "manage:customize",
	"media":    "manage:media",
	"settings": "manage:settings",
	"tables":   "manage:roll-tables",
	"foundry":  "manage:foundry",
	"ai":       "manage:ai",
	"trash":    "manage:trash",
}

// ManageToolOfLine returns the pin key and the tool's name for a desk line, or
// empty strings when the line has no tool to pin.
func ManageToolOfLine(line string) (key, name string) {
	key, ok := manageToolOfLine[line]
	if !ok {
		return "", ""
	}
	for _, r := range ManageToolRows("", true, true) {
		if r.Key == key {
			return key, r.Label
		}
	}
	return "", ""
}

// ManagePinned reports whether the owner already pinned the tool.
func ManagePinned(ctx context.Context, key string) bool { return navPinnedHere(ctx, key) }

// managePinCall is the pin's click handler. It hands the press to
// static/js/manage_pin.js, loading it on first use: the desk and the frame
// arrive by boosted swaps, where a script tag may never run, and only these two
// places need the script.
func managePinCall() templ.ComponentScript {
	src := AssetURL("/static/js/manage_pin.js")
	q := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", ``, "\r", ``, `<`, `\x3c`).Replace(src)
	return templ.ComponentScript{
		Name:     "managePin",
		Function: "",
		Call: `(function(b,e){var C=window.Chronicle=window.Chronicle||{};` +
			`function go(){if(C.managePin)C.managePin.toggle(b,e);}` +
			`if(C.managePin){go();return;}` +
			`var s=document.querySelector('script[data-manage-pin-js]');` +
			`if(!s){s=document.createElement('script');s.src='` + q + `';s.setAttribute('data-manage-pin-js','');document.head.appendChild(s);}` +
			`s.addEventListener('load',go);})(this,event)`,
	}
}

// manageFindCall is the Find button's click handler. It hands the press to
// static/js/manage_find.js, loading it on first use for the same reason the pin
// does: the frame arrives by boosted swaps, where a script tag may never run.
func manageFindCall() templ.ComponentScript {
	src := AssetURL("/static/js/manage_find.js")
	q := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", ``, "\r", ``, `<`, `\x3c`).Replace(src)
	return templ.ComponentScript{
		Name:     "manageFind",
		Function: "",
		Call: `(function(b,e){var C=window.Chronicle=window.Chronicle||{};` +
			`function go(){if(C.manageFind)C.manageFind.open(b,e);}` +
			`if(C.manageFind){go();return;}` +
			`var s=document.querySelector('script[data-manage-find-js]');` +
			`if(!s){s=document.createElement('script');s.src='` + q + `';s.setAttribute('data-manage-find-js','');document.head.appendChild(s);}` +
			`s.addEventListener('load',go);})(this,event)`,
	}
}

type manageFindFieldKey struct{}

// WithManageFindField marks a render as one whose page draws its own Find
// field (ManageFindField), so the frame leaves its header button out.
func WithManageFindField(ctx context.Context) context.Context {
	return context.WithValue(ctx, manageFindFieldKey{}, true)
}

// ManageFindEligible reports whether the viewer gets Find at all: the
// signed-in owner who is not previewing the campaign as a player. The list
// behind it is owner-only on the server too; this only avoids offering a
// refused button.
func ManageFindEligible(ctx context.Context) bool {
	return IsAuthenticated(ctx) && GetCampaignRole(ctx) >= 3 && !IsViewingAsPlayer(ctx)
}

// ManageFindShown reports whether the frame's header draws the Find button:
// the viewer gets Find, and the page has not drawn a field of its own.
func ManageFindShown(ctx context.Context) bool {
	own, _ := ctx.Value(manageFindFieldKey{}).(bool)
	return !own && ManageFindEligible(ctx)
}
