// onclick_handlers.go builds the inline IIFE click handlers for the armory's
// HTMX-swapped fragments (item card menu, collection rename, the Give box). Templ script
// helpers emit a sibling <script> that browsers do not reliably run after an
// innerHTML swap, so each handler is a self-contained expression rendered
// straight into the onclick attribute. The JS uses single quotes only (the
// attribute is double-quoted) and string values go through jsStr, so a
// collection name with quotes cannot break out of the attribute.
package armory

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/a-h/templ"
)

// inlineOnClick wraps a JS body in a ComponentScript with an empty Function so
// templ renders only the call into the attribute and emits no <script>.
func inlineOnClick(name, jsBody string) templ.ComponentScript {
	return templ.ComponentScript{Name: name, Function: "", Call: jsBody}
}

// jsStr returns a single-quoted JS string literal with the content escaped so
// quotes, backslashes and control characters cannot terminate it.
func jsStr(s string) string {
	return "'" + strings.ReplaceAll(template.JSEscapeString(s), `\"`, `\u0022`) + "'"
}

// collectionMenuOnClick opens the item card's menu: "Add to collection..."
// (the collections that hold the item, ticked and counted from the server's
// answers) and, with a giveURL, "Give to...", which turns the same popover
// into the Give box. Chronicle.GiveBox.card owns the popover; giveURL is empty
// for viewers who may not give and collections is false for those who may not
// edit collections, and each leaves its entry out.
func collectionMenuOnClick(campaignID, entityID, popoverID, giveURL string, collections bool) templ.ComponentScript {
	body := fmt.Sprintf(
		`(function(btn){if(window.Chronicle&&Chronicle.GiveBox)Chronicle.GiveBox.card(btn,{campaign:%s,item:%s,pop:%s,give:%s,collections:%t});})(this)`,
		jsStr(campaignID), jsStr(entityID), jsStr(popoverID), jsStr(giveURL), collections)
	return inlineOnClick("armory_collectionMenu", body)
}

// renameInstanceOnClick swaps a collection row's label for an inline name
// input. It sends only the name: the update route is partial, so the row's
// possibly stale description, icon and colour are left alone; a validation
// failure is shown under the input instead of failing silently.
func renameInstanceOnClick(campaignID string, instanceID int, name string) templ.ComponentScript {
	body := fmt.Sprintf(
		`(function(btn){`+
			`var row=btn.closest('[data-inst-row]');if(!row||row.querySelector('[data-rename-form]'))return;`+
			`var label=row.querySelector('[data-inst-label]');if(!label)return;`+
			`var url='/campaigns/'+encodeURIComponent(%[1]s)+'/armory/instances/'+%[2]d;`+
			`var form=document.createElement('form');form.setAttribute('data-rename-form','1');form.className='flex-1 flex flex-wrap items-center gap-2';`+
			`var input=document.createElement('input');input.type='text';input.maxLength=100;input.value=%[3]s;input.setAttribute('aria-label','Collection name');input.className='input text-sm flex-1 min-w-0';`+
			`var save=document.createElement('button');save.type='submit';save.className='btn-primary text-xs';save.textContent='Save';`+
			`var cancel=document.createElement('button');cancel.type='button';cancel.className='btn-secondary text-xs';cancel.textContent='Cancel';`+
			`var err=document.createElement('p');err.setAttribute('role','alert');err.className='basis-full text-xs text-red-500';`+
			`form.appendChild(input);form.appendChild(save);form.appendChild(cancel);form.appendChild(err);`+
			`label.hidden=true;label.parentNode.insertBefore(form,label.nextSibling);input.focus();input.select();`+
			`function done(){form.remove();label.hidden=false;btn.focus();}`+
			`cancel.onclick=done;`+
			`form.onkeydown=function(e){if(e.key==='Escape'){e.stopPropagation();done();}};`+
			`form.onsubmit=function(e){e.preventDefault();var n=input.value.trim();err.textContent='';`+
			`if(!n){err.textContent='Name is required.';input.focus();return;}`+
			`save.disabled=true;`+
			`Chronicle.apiFetch(url,{method:'PUT',body:{name:n}}).then(function(r){`+
			`if(r.ok){var t=label.querySelector('[data-inst-name]');if(t)t.textContent=n;`+
			`var o=document.querySelector('select[name=instance] option[value=\''+%[2]d+'\']');if(o){var m=o.textContent.trim().match(/\(\d+\)$/);o.textContent=n+(m?' '+m[0]:'');}done();return;}`+
			`return r.json().then(function(j){err.textContent=j.message||'Rename failed.';input.focus();},function(){err.textContent='Rename failed.';});})`+
			`.catch(function(){err.textContent='Network error. Try again.';})`+
			`.then(function(){save.disabled=false;});};`+
			`})(this)`,
		jsStr(campaignID), instanceID, jsStr(name))
	return inlineOnClick("armory_renameInstance", body)
}

// withdrawToggleOnClick opens (or closes) the inline "Withdraw this request?"
// confirm on a waiting purchase line. Opening hides the Withdraw link and
// moves focus to the confirm; closing restores both. Escape inside the confirm
// closes it too.
func withdrawToggleOnClick(open bool) templ.ComponentScript {
	body := fmt.Sprintf(
		`(function(el){`+
			`var w=el.closest('[data-withdraw]');if(!w)return;`+
			`var o=w.querySelector('[data-withdraw-open]'),c=w.querySelector('[data-withdraw-confirm]');if(!o||!c)return;`+
			`var open=%t;o.hidden=open;o.setAttribute('aria-expanded',open?'true':'false');c.hidden=!open;`+
			`if(open){c.onkeydown=function(e){if(e.key==='Escape'){e.stopPropagation();o.hidden=false;o.setAttribute('aria-expanded','false');c.hidden=true;o.focus();}};`+
			`var b=c.querySelector('button');if(b)b.focus();}else{o.focus();}`+
			`})(this)`, open)
	return inlineOnClick("armory_withdrawToggle", body)
}
