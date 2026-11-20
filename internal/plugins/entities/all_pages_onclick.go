package entities

import (
	"strconv"

	"github.com/a-h/templ"
)

// all_pages_onclick.go holds the inline handlers of All pages' own controls.
// The list arrives as an HTMX fragment, where a script tag or a delegated
// listener may never run, so each control carries a self-contained handler
// built here from fixed text.

// allPagesViewKey is the browser's memory of which view the list is in. It
// is not the old cards/table key: that stored "grid" for everyone who never
// chose, which would now open the Gallery for them.
const allPagesViewKey = "chronicle_entities_view_v2"

// allPagesViewJS switches the list to one view, marks its tab, and remembers
// the choice in this browser. Storage can be refused; the view still changes.
func allPagesViewJS(view string) templ.ComponentScript {
	return inlineHandler("allPagesView", `(function(b){`+
		`var l=b.closest('#entity-list'),v=`+strconv.Quote(view)+`;`+
		`l.setAttribute('data-view',v);`+
		`var t=l.querySelectorAll('[data-view-tab]');`+
		`for(var i=0;i<t.length;i++){t[i].setAttribute('aria-selected',t[i]===b?'true':'false');}`+
		`try{localStorage.setItem(`+strconv.Quote(allPagesViewKey)+`,v);}catch(e){}`+
		`})(this)`)
}

// allPagesSelectJS turns the tick boxes on or off for touch, where a box that
// waits for a pointer is never seen. Done also clears what was picked.
func allPagesSelectJS() templ.ComponentScript {
	return inlineHandler("allPagesSelect", `(function(b){`+
		`var l=b.closest('#entity-list'),on=!l.hasAttribute('data-selecting')&&!l.querySelector('[data-entity-row][data-picked]');`+
		`if(on){l.setAttribute('data-selecting','');}else{l.removeAttribute('data-selecting');}`+
		`b.textContent=on?'Done':'Select';b.setAttribute('aria-pressed',on?'true':'false');`+
		`if(!on){l.dispatchEvent(new CustomEvent('bulk:clear'));}`+
		`})(this)`)
}

// allPagesCategoryJS goes to the chosen category's own list. The options'
// values are addresses this page wrote itself.
func allPagesCategoryJS() templ.ComponentScript {
	return inlineHandler("allPagesCategory", `(function(s){if(s.value){window.location.assign(s.value);}})(this)`)
}
