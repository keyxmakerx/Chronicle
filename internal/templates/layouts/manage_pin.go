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
