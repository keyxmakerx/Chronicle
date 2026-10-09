package entities

import (
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// cast_onclick.go holds the inline handlers of the owner's list controls on the
// Characters page. The page arrives as an HTMX fragment, where a script tag or
// a delegated listener may never run, so each control carries its own
// self-contained handler, built here from fixed text and integer IDs only.

// inlineHandler wraps a JS body in a ComponentScript with an empty Function so
// templ writes only the call into the attribute and emits no script tag.
func inlineHandler(name, body string) templ.ComponentScript {
	return templ.ComponentScript{Name: name, Function: "", Call: body}
}

// castFoldJS opens the named box in the band's fold, or closes the fold when
// box is empty or already open. Boxes are "add" and "rm-<typeID>"; one shows
// at a time. The fold grows to the box's height and returns to auto so a
// search that shortens the list can still resize it.
const castFoldJS = `(function(el,want){` +
	`var s=el.closest('[data-band]'),f=s.querySelector('.ag-fold');` +
	`var cur=f.getAttribute('data-open')||'';if(want&&cur===want)want='';` +
	`var boxes=f.querySelectorAll('[data-box]');` +
	`for(var i=0;i<boxes.length;i++){boxes[i].hidden=boxes[i].getAttribute('data-box')!==want;}` +
	`var add=s.querySelector('.ag-add');if(add)add.setAttribute('aria-expanded',want==='add'?'true':'false');` +
	`f.setAttribute('data-open',want);` +
	`var still=window.matchMedia('(prefers-reduced-motion: reduce)').matches;` +
	`if(want){f.classList.add('is-open');f.style.height=still?'auto':f.scrollHeight+'px';` +
	`if(!still){setTimeout(function(){if(f.getAttribute('data-open'))f.style.height='auto';},350);}` +
	`var q=f.querySelector('[data-box='+JSON.stringify(want)+'] input[type=search]');if(q)q.focus({preventScroll:true});}` +
	`else{if(!still){f.style.height=f.scrollHeight+'px';f.getBoundingClientRect();}f.style.height='0px';f.classList.remove('is-open');}` +
	`})(this,`

// castFoldCall returns the click handler that opens (or closes) a box.
func castFoldCall(box string) templ.ComponentScript {
	return inlineHandler("cast_fold_"+box, castFoldJS+"'"+box+"')")
}

// castFoldRemoveCall opens the in-place "stop showing this type" box of a chip.
func castFoldRemoveCall(typeID int) templ.ComponentScript {
	return castFoldCall("rm-" + strconv.Itoa(typeID))
}

// castPickCall runs when a radio row is picked: it enables Save and shows the
// row's sentence under the list.
var castPickCall = inlineHandler("cast_pick",
	`(function(r){var b=r.closest('[data-box]');`+
		`var save=b.querySelector('[data-save]');if(save)save.disabled=false;`+
		`var h=b.querySelector('[data-hint]');`+
		`if(h){h.innerHTML='<i class="fa-solid fa-eye"></i><span></span>';h.lastChild.textContent=r.getAttribute('data-say')||'';}`+
		`})(this)`)

// castSearchCall filters the add box's rows by what was typed.
var castSearchCall = inlineHandler("cast_search",
	`(function(i){var b=i.closest('[data-box]'),q=i.value.trim().toLowerCase();`+
		`var rows=b.querySelectorAll('.ag-row'),n=0;`+
		`for(var k=0;k<rows.length;k++){var m=!q||rows[k].textContent.toLowerCase().indexOf(q)>=0;`+
		`rows[k].style.display=m?'':'none';if(m)n++;}`+
		`var none=b.querySelector('[data-nomatch]');if(none)none.hidden=n>0;`+
		`})(this)`)

// castPickSay is the sentence under the add list once a type is picked: what
// the band will show, and that visibility still applies.
func castPickSay(kind CharacterListKind, o CastTypeOption) string {
	var what string
	if kind == CharacterListCharacters {
		what = "Claimed " + o.Name + " pages join the party."
	} else {
		what = "Every " + o.Name + " page shows here"
		if len(o.With) > 0 {
			what += ", " + strings.Join(o.With, ", ") + " too"
		}
		what += "."
	}
	return what + " Players still only see the pages they’re allowed to."
}
