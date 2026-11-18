package layouts

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/a-h/templ"
)

// ManageSentenceFunc words a Manage page's own desk sentence ("4 people: you,
// 1 scribe and 2 players.") for the frame to draw under the page's title. The
// campaigns plugin registers it at wiring time: layouts cannot import it, and
// the sentence must come from the same provider data the desk reads, so the
// page opened by address and the page reached from the desk start the same
// way. It returns nil when there is nothing true to say (a viewer who is not
// the owner, a provider that failed), which draws no sentence at all.
type ManageSentenceFunc func(ctx context.Context, line string) templ.Component

var manageSentence atomic.Pointer[ManageSentenceFunc]

// SetManageSentence registers the resolver the frame draws sentences with.
func SetManageSentence(f ManageSentenceFunc) {
	if f == nil {
		manageSentence.Store(nil)
		return
	}
	manageSentence.Store(&f)
}

// manageSentenceFor returns the sentence for a page's desk line, or nil.
func manageSentenceFor(ctx context.Context, line string) templ.Component {
	if line == "" {
		return nil
	}
	f := manageSentence.Load()
	if f == nil {
		return nil
	}
	return (*f)(ctx, line)
}

// manageBackCall is the "Manage" word's click handler. It hands the click to
// static/js/desk_carry.js, loading that file on first use: the frame arrives by
// boosted swaps where a script tag may never run, and no other page needs the
// script.
func manageBackCall() templ.ComponentScript {
	src := AssetURL("/static/js/desk_carry.js")
	q := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", ``, "\r", ``, `<`, `\x3c`).Replace(src)
	return templ.ComponentScript{
		Name:     "manageBack",
		Function: "",
		// A plain click would navigate before the script has loaded, so the
		// first press is held and handed over with "late" set: the script then
		// plays the move, or goes to the link itself when it cannot.
		Call: `(function(a,e){var C=window.Chronicle=window.Chronicle||{};var late=0;` +
			`if(!C.deskCarry&&e.button===0&&!e.metaKey&&!e.ctrlKey&&!e.shiftKey&&!e.altKey&&!a.target){e.preventDefault();late=1;}` +
			`function go(){if(C.deskCarry)C.deskCarry.back(a,e,late);}` +
			`if(C.deskCarry){go();return;}` +
			`var s=document.querySelector('script[data-desk-carry-js]');` +
			`if(!s){s=document.createElement('script');s.src='` + q + `';s.setAttribute('data-desk-carry-js','');document.head.appendChild(s);}` +
			`s.addEventListener('load',go);})(this,event)`,
	}
}
