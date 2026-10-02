// onclick_handlers.go builds the inline IIFE click handlers for the armory's
// HTMX-swapped fragments (item card menu, collection rename). Templ script
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

// collectionMenuOnClick toggles the item card's "Add to collection" popover.
// It lazily loads the collections that hold the item, and each checkbox adds
// or removes the item through the existing instance-item routes, ticking and
// counting from the server's answer rather than assuming success. Focus and
// Escape handling are scoped to the popover (no document listeners) so a swap
// of the gallery cannot leave a stray handler behind.
func collectionMenuOnClick(campaignID, entityID, popoverID string) templ.ComponentScript {
	body := fmt.Sprintf(
		`(function(btn){`+
			`var pop=document.getElementById(%[3]s);if(!pop)return;`+
			`var base='/campaigns/'+encodeURIComponent(%[1]s)+'/armory/';var eid=%[2]s;`+
			`function close(refocus){pop.hidden=true;btn.setAttribute('aria-expanded','false');if(refocus)btn.focus();}`+
			`if(!pop.hidden){close(true);return;}`+
			`pop.hidden=false;btn.setAttribute('aria-expanded','true');pop.textContent='Loading...';`+
			`pop.onkeydown=function(e){if(e.key==='Escape'){e.stopPropagation();close(true);}};`+
			`pop.onfocusout=function(){setTimeout(function(){var a=document.activeElement;if(!pop.hidden&&a!==btn&&!pop.contains(a))close(false);},0);};`+
			`function fail(msg){var m=pop.querySelector('[data-coll-error]');if(m){m.textContent=msg;}if(window.Chronicle&&Chronicle.notify)Chronicle.notify(msg,'error');}`+
			`function syncOption(c){var o=document.querySelector('select[name=instance] option[value=\''+c.id+'\']');if(o)o.textContent=c.name+' ('+c.itemCount+')';}`+
			`function row(c){`+
			`var l=document.createElement('label');l.className='flex items-center gap-2 px-2 py-1.5 rounded hover:bg-surface-alt cursor-pointer text-sm text-fg';`+
			`var cb=document.createElement('input');cb.type='checkbox';cb.checked=!!c.hasItem;`+
			`var n=document.createElement('span');n.className='flex-1 truncate';n.textContent=c.name;`+
			`var k=document.createElement('span');k.className='text-xs text-fg-muted';k.textContent=String(c.itemCount);`+
			`cb.onchange=function(){var want=cb.checked;if(cb.getAttribute('aria-busy')==='true')return;cb.setAttribute('aria-busy','true');`+
			`var req=want?Chronicle.apiFetch(base+'instances/'+c.id+'/items',{method:'POST',body:{entity_id:eid}}):Chronicle.apiFetch(base+'instances/'+c.id+'/items/'+encodeURIComponent(eid),{method:'DELETE'});`+
			`req.then(function(r){if(r.ok){c.hasItem=want;c.itemCount=Math.max(0,c.itemCount+(want?1:-1));k.textContent=String(c.itemCount);syncOption(c);return;}`+
			`cb.checked=!want;return r.json().then(function(j){fail(j.message||'Could not update the collection.');},function(){fail('Could not update the collection.');});})`+
			`.catch(function(){cb.checked=!want;fail('Could not update the collection.');})`+
			`.then(function(){cb.removeAttribute('aria-busy');});};`+
			`l.appendChild(cb);l.appendChild(n);l.appendChild(k);return l;}`+
			`function render(list){pop.textContent='';var h=document.createElement('div');h.className='px-2 pb-1 text-xs font-semibold text-fg-secondary';h.textContent='Add to collection';pop.appendChild(h);`+
			`if(!list.length){var e=document.createElement('p');e.className='px-2 py-1 text-xs text-fg-muted';e.textContent='No collections yet. Create one with the gear button.';pop.appendChild(e);}`+
			`list.forEach(function(c){pop.appendChild(row(c));});`+
			`var m=document.createElement('p');m.setAttribute('data-coll-error','1');m.setAttribute('role','alert');m.className='px-2 text-xs text-red-500';pop.appendChild(m);`+
			`var f=pop.querySelector('input');if(f)f.focus();else pop.focus();}`+
			`Chronicle.apiFetch(base+'items/'+encodeURIComponent(eid)+'/collections').then(function(r){if(!r.ok)throw new Error('load');return r.json();}).then(render)`+
			`.catch(function(){pop.textContent='Could not load collections.';pop.focus();});`+
			`})(this)`,
		jsStr(campaignID), jsStr(entityID), jsStr(popoverID))
	return inlineOnClick("armory_collectionMenu", body)
}

// renameInstanceOnClick swaps a collection row's label for an inline name
// input. The update route replaces every field, so the row's current
// description, icon and colour ride along with the new name; a validation
// failure is shown under the input instead of failing silently.
func renameInstanceOnClick(campaignID string, instanceID int, name, description, icon, color string) templ.ComponentScript {
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
			`Chronicle.apiFetch(url,{method:'PUT',body:{name:n,description:%[4]s,icon:%[5]s,color:%[6]s}}).then(function(r){`+
			`if(r.ok){var t=label.querySelector('[data-inst-name]');if(t)t.textContent=n;`+
			`var o=document.querySelector('select[name=instance] option[value=\''+%[2]d+'\']');if(o){var m=o.textContent.trim().match(/\(\d+\)$/);o.textContent=n+(m?' '+m[0]:'');}done();return;}`+
			`return r.json().then(function(j){err.textContent=j.message||'Rename failed.';input.focus();},function(){err.textContent='Rename failed.';});})`+
			`.catch(function(){err.textContent='Network error. Try again.';})`+
			`.then(function(){save.disabled=false;});};`+
			`})(this)`,
		jsStr(campaignID), instanceID, jsStr(name), jsStr(description), jsStr(icon), jsStr(color))
	return inlineOnClick("armory_renameInstance", body)
}

// instanceDescription dereferences the optional description for the rename
// payload, where an absent one must be sent as empty rather than dropped.
func instanceDescription(inst InventoryInstance) string {
	if inst.Description == nil {
		return ""
	}
	return *inst.Description
}
