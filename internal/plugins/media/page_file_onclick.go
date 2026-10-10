package media

import "github.com/a-h/templ"

// page_file_onclick.go holds the inline handlers of the Files section. The
// section arrives inside an HTMX-loaded fragment, where a script tag or a
// delegated listener may never run, so each control carries its own
// self-contained handler built from fixed text. The only values read at run
// time are the section's own data-base URL (made from two ids) and a row's
// data-id; no file name ever reaches the JS.

// inlineHandler wraps a JS body so templ writes only the call into the
// attribute and emits no script tag.
func inlineHandler(name, body string) templ.ComponentScript {
	return templ.ComponentScript{Name: name, Function: "", Call: body}
}

// pfReloadJS re-fetches the section in place after any change. It is shared
// text, spliced into the handlers below.
const pfReloadJS = `function reload(){htmx.ajax('GET',base,{target:s,swap:'outerHTML'});}`

// pfPickJS opens the file picker.
const pfPickJS = `(function(b){b.closest('[data-pf]').querySelector('[data-pf-input]').click();})(this)`

// pfUploadJS attaches the chosen file. The button shows a spinner while the
// request is out; the server's own words come back on a refusal (a type that
// is not allowed, a quota, a size).
const pfUploadJS = `(function(inp){var s=inp.closest('[data-pf]'),f=inp.files&&inp.files[0];if(!f)return;` +
	`var base=s.getAttribute('data-base'),btn=s.querySelector('[data-pf-attach]'),label=btn.innerHTML;` + pfReloadJS +
	`var fd=new FormData();fd.append('file',f);` +
	`var gm=s.querySelector('[data-pf-gm]');if(gm&&gm.checked)fd.append('gm_only','1');` +
	`btn.disabled=true;btn.innerHTML='<span class=fx-spin aria-hidden=true></span><span>Attaching…</span>';` +
	`function fail(m){btn.disabled=false;btn.innerHTML=label;inp.value='';Chronicle.notify(m||'Couldn’t attach that file. Nothing changed.','error');}` +
	`Chronicle.apiFetch(base,{method:'POST',body:fd}).then(function(r){` +
	`if(!r.ok)return r.json().then(function(j){fail(j&&j.message);},function(){fail();});` +
	`inp.value='';Chronicle.notify('File attached.','success');reload();` +
	`}).catch(function(){fail('Network error. Try again.');});})(this)`

// pfAskJS swaps a row's buttons for the "remove it?" question.
const pfAskJS = `(function(b){var r=b.closest('[data-pf-row]');r.querySelector('[data-pf-acts]').hidden=true;` +
	`var q=r.querySelector('[data-pf-ask]');q.hidden=false;q.querySelector('[data-pf-keep]').focus();})(this)`

// pfKeepJS puts the buttons back.
const pfKeepJS = `(function(b){var r=b.closest('[data-pf-row]');r.querySelector('[data-pf-ask]').hidden=true;` +
	`var a=r.querySelector('[data-pf-acts]');a.hidden=false;a.querySelector('[data-pf-remove]').focus();})(this)`

// pfRemoveJS removes the file for good (page files do not go to the Trash).
const pfRemoveJS = `(function(b){var r=b.closest('[data-pf-row]'),s=b.closest('[data-pf]'),base=s.getAttribute('data-base');` + pfReloadJS +
	`b.disabled=true;` +
	`Chronicle.apiFetch(base+'/'+r.getAttribute('data-id'),{method:'DELETE'}).then(function(x){` +
	`if(!x.ok){b.disabled=false;return x.json().then(function(j){Chronicle.notify((j&&j.message)||'Couldn’t remove that file.','error');},function(){Chronicle.notify('Couldn’t remove that file.','error');});}` +
	`r.classList.add('is-leaving');setTimeout(reload,220);` +
	`}).catch(function(){b.disabled=false;Chronicle.notify('Network error. Try again.','error');});})(this)`

// pfGMToggleJS flips a file between GM only and shown to everyone who can see
// the page.
const pfGMToggleJS = `(function(b){var r=b.closest('[data-pf-row]'),s=b.closest('[data-pf]'),base=s.getAttribute('data-base');` + pfReloadJS +
	`var on=r.getAttribute('data-gm')==='1';b.disabled=true;` +
	`Chronicle.apiFetch(base+'/'+r.getAttribute('data-id')+'/visibility',{method:'PUT',body:{gmOnly:!on}}).then(function(x){` +
	`if(!x.ok){b.disabled=false;return x.json().then(function(j){Chronicle.notify((j&&j.message)||'Couldn’t change that.','error');},function(){Chronicle.notify('Couldn’t change that.','error');});}` +
	`reload();}).catch(function(){b.disabled=false;Chronicle.notify('Network error. Try again.','error');});})(this)`

var (
	pfPickCall     = inlineHandler("pf_pick", pfPickJS)
	pfUploadCall   = inlineHandler("pf_upload", pfUploadJS)
	pfAskCall      = inlineHandler("pf_ask", pfAskJS)
	pfKeepCall     = inlineHandler("pf_keep", pfKeepJS)
	pfRemoveCall   = inlineHandler("pf_remove", pfRemoveJS)
	pfGMToggleCall = inlineHandler("pf_gm_toggle", pfGMToggleJS)
)
