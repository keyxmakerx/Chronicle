package entities

import "github.com/a-h/templ"

// practice_onclick.go holds the inline handlers of the practice shelf, the
// Directors' list and the practice page. They arrive as HTMX fragments, where
// a script tag or a delegated listener may never run, so every control
// carries its own self-contained handler built here from fixed text.

// practiceRevealJS opens or closes one named box inside the row that holds
// the clicked control: the confirm line of a yes or a throw-away, or the
// Director's note field. A trigger that carries data-pr-hide steps aside while
// its box is open so the box's own button is the only one with that word.
const practiceRevealJS = `(function(b,k){var r=b.closest('[data-pr-row]');if(!r)return;` +
	`var box=r.querySelector('[data-pr-box='+JSON.stringify(k)+']');if(!box)return;` +
	`var show=box.hidden;box.hidden=!show;` +
	`var ts=r.querySelectorAll('[data-pr-opens='+JSON.stringify(k)+']');` +
	`for(var i=0;i<ts.length;i++){ts[i].setAttribute('aria-expanded',show?'true':'false');if(ts[i].hasAttribute('data-pr-hide'))ts[i].hidden=show;}` +
	`if(show){var f=box.querySelector('input,button');if(f)f.focus();}` +
	`else{var t=r.querySelector('[data-pr-opens='+JSON.stringify(k)+']');if(t)t.focus();}` +
	`})(this,`

// practiceRevealCall returns the click handler that opens or closes a box.
func practiceRevealCall(box string) templ.ComponentScript {
	return inlineHandler("practice_reveal_"+box, practiceRevealJS+"'"+box+"')")
}
