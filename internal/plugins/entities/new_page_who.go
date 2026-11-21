package entities

import (
	"strconv"

	"github.com/a-h/templ"
)

// The two sentences Create page can say about who will see the new page, and
// the quiet word that flips between them. They describe what will be saved:
// newPageWho is given the state the server resolved, never a default of its
// own, so the words and the saved page cannot disagree.
const (
	newPageHiddenLine = "Players won’t see this page until you show it."
	newPageShownLine  = "Everyone in the campaign will see this page."
	newPageShowWord   = "Show it to players"
	newPageHideWord   = "Hide it from players"
)

// newPageWhoLine is the sentence for the state the page will be saved in.
func newPageWhoLine(isPrivate bool) string {
	if isPrivate {
		return newPageHiddenLine
	}
	return newPageShownLine
}

// newPageWhoWord is the quiet word that changes the state.
func newPageWhoWord(isPrivate bool) string {
	if isPrivate {
		return newPageShowWord
	}
	return newPageHideWord
}

// newPageWhoToggleJS flips the hidden is_private field and the words around
// it together. The form arrives by htmx and inside a dialog, so the handler
// is inline and built here from fixed text.
func newPageWhoToggleJS() templ.ComponentScript {
	return inlineHandler("newPageWhoToggle", `(function(b){`+
		`var w=b.closest('[data-np-who]'),i=w.querySelector('input[name=is_private]'),`+
		`t=w.querySelector('[data-np-who-text]'),l=w.querySelector('[data-np-who-lock]');`+
		`var hide=i.value!=='true';`+
		`i.value=hide?'true':'false';`+
		`t.textContent=hide?`+strconv.Quote(newPageHiddenLine)+`:`+strconv.Quote(newPageShownLine)+`;`+
		`b.textContent=hide?`+strconv.Quote(newPageShowWord)+`:`+strconv.Quote(newPageHideWord)+`;`+
		`l.hidden=!hide;`+
		`})(this)`)
}

// newPageCloseJS closes the sheet the form sits in. A dialog's own close
// puts focus back on what opened it.
func newPageCloseJS() templ.ComponentScript {
	return inlineHandler("newPageClose", `(function(b){var n=window.Chronicle&&window.Chronicle.newPage;if(n){n.requestClose();return;}var d=b.closest('dialog');if(d)d.close();})(this)`)
}
