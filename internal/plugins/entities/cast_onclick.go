package entities

import (
	"html"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// cast_onclick.go holds the inline handlers of the owner's list controls on the
// Characters page. The page arrives as an HTMX fragment, where a script tag or
// a delegated listener may never run, so each control carries its own
// self-contained handler, built here from fixed text and integer IDs only.

// inlineHandler wraps a JS body in a ComponentScript with an empty Function so
// templ writes only the call into the attribute and emits no script tag. templ
// writes Call into the double-quoted attribute as is, so the body is
// HTML-escaped here: a double quote in it (markup the handler inserts) would
// otherwise end the attribute and print the rest of the script on the page.
func inlineHandler(name, body string) templ.ComponentScript {
	return templ.ComponentScript{Name: name, Function: "", Call: html.EscapeString(body)}
}

// castFoldJS opens the named box in the band's fold, or closes the fold when
// box is empty or already open. Boxes are "add", "players" and "rm-<typeID>";
// one shows at a time. The fold grows to the box's height and returns to auto
// so a search that shortens the list can still resize it.
const castFoldJS = `(function(el,want){` +
	`var s=el.closest('[data-band]'),f=s.querySelector('.ag-fold');` +
	`var cur=f.getAttribute('data-open')||'';if(want&&cur===want)want='';` +
	`var boxes=f.querySelectorAll('[data-box]');` +
	`for(var i=0;i<boxes.length;i++){boxes[i].hidden=boxes[i].getAttribute('data-box')!==want;}` +
	`var ops=s.querySelectorAll('[data-opens]');for(var j=0;j<ops.length;j++){ops[j].setAttribute('aria-expanded',ops[j].getAttribute('data-opens')===want?'true':'false');}` +
	`f.setAttribute('data-open',want);` +
	`var still=window.matchMedia('(prefers-reduced-motion: reduce)').matches;` +
	`if(want){f.classList.add('is-open');f.style.height=still?'auto':f.scrollHeight+'px';` +
	`if(!still){setTimeout(function(){if(f.getAttribute('data-open'))f.style.height='auto';},350);}` +
	`var q=f.querySelector('[data-box='+JSON.stringify(want)+'] input[type=search]');if(q)q.focus({preventScroll:true});}` +
	`else{if(!still){f.style.height=f.scrollHeight+'px';f.getBoundingClientRect();}f.style.height='0px';f.classList.remove('is-open');}` +
	`})(this,`

// castFoldStyle keeps a fold the server renders open at its natural height;
// a closed fold takes its zero height from the stylesheet.
func castFoldStyle(open string) string {
	if open == "" {
		return ""
	}
	return "height:auto"
}

// castIcon is the page type's icon for a row, or a plain person.
func castIcon(e Entity) string {
	if e.TypeIcon != "" {
		return e.TypeIcon
	}
	return "fa-user"
}

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

// castFinderJS opens the Players box's single finder under the clicked button,
// or closes it when that button's finder is already open. kind "give" puts it
// after the player list, "free" after the Free to pick section. The target
// (a user id, or "free") goes in the hidden "for" field the search sends, and
// an immediate search shows the starting list.
const castFinderJS = `(function(b,kind){` +
	`var box=b.closest('[data-box]'),f=box.querySelector('.pw-find');` +
	`var was=b.getAttribute('aria-expanded')==='true';` +
	`var all=box.querySelectorAll('.pw-give');for(var i=0;i<all.length;i++)all[i].setAttribute('aria-expanded','false');` +
	`if(was){f.hidden=true;return;}` +
	`var host=kind==='give'?box.querySelector('.pw-list'):b.closest('.pw-sec');` +
	`host.after(f);f.hidden=false;` +
	`f.querySelector('input[name=for]').value=b.getAttribute('data-who')||'free';` +
	`var q=f.querySelector('input[type=search]');q.value='';q.placeholder=b.getAttribute('data-ph')||'';` +
	`b.setAttribute('aria-expanded','true');` +
	`if(window.htmx)htmx.trigger(q,'search');` +
	`q.focus({preventScroll:true});` +
	`})(this,`

// castFinderCall returns the click handler of a Give or Add button.
func castFinderCall(kind string) templ.ComponentScript {
	return inlineHandler("cast_finder_"+kind, castFinderJS+"'"+kind+"')")
}

// castAskClaimCall shows the confirm row that belongs to the clicked Pick
// button and hides the others, so one question is open at a time.
var castAskClaimCall = inlineHandler("cast_ask_claim",
	`(function(b){var id=b.getAttribute('data-entity-id'),`+
		`rows=b.closest('.ag-body').querySelectorAll('.ag-confirm-row');`+
		`for(var i=0;i<rows.length;i++){rows[i].hidden=rows[i].getAttribute('data-for')!==id;}})(this)`)

// castCancelClaimCall closes the confirm row it sits in.
var castCancelClaimCall = inlineHandler("cast_cancel_claim",
	`(function(b){b.closest('.ag-confirm-row').hidden=true;})(this)`)
