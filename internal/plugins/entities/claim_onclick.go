package entities

// claim_onclick.go holds the inline handlers of the claim banner. The banner
// can arrive inside an HTMX-swapped page, where a script tag or a delegated
// listener may never run, so each control carries its own self-contained
// handler built from fixed text (no user text reaches the JS).

// claimSubmitJS runs the claim without leaving the page. The button shows a
// spinner while the request is out; on success the banner becomes "Yours" and
// flashes, and the page reloads only after the flash so the rest of the sheet
// picks up the new owner; on failure the amber bar carries the server's words
// and "Try again" resubmits. The request carries HX-Request so the server
// answers with a redirect header rather than a redirect, which fetch would
// follow into the whole page.
const claimSubmitJS = `(function(form,e){e.preventDefault();` +
	`var box=form.closest('[data-claim-state]'),wrap=form.closest('[data-claim-wrap]');` +
	`var btn=form.querySelector('button[type=submit]'),warn=wrap.querySelector('[data-claim-warn]');` +
	`if(btn.disabled)return;` +
	`var label=btn.innerHTML;warn.classList.remove('is-on');` +
	`btn.disabled=true;btn.innerHTML='<span class="fx-spin" aria-hidden="true"></span><span>Claiming…</span>';` +
	`function fail(m){btn.disabled=false;btn.innerHTML=label;` +
	`warn.querySelector('[data-claim-msg]').textContent=m||'Couldn’t claim this character. Nothing changed.';` +
	`void warn.offsetWidth;warn.classList.add('is-on');}` +
	`Chronicle.apiFetch(form.getAttribute('hx-post'),{method:'POST',body:new FormData(form),headers:{'HX-Request':'true'}}).then(function(r){` +
	`if(!r.ok)return r.json().then(function(j){fail(j&&j.message);},function(){fail();});` +
	`box.className='mb-4 rounded-md border border-edge bg-surface-alt px-4 py-2.5 flex items-center gap-3 ag-landed';` +
	`box.setAttribute('data-claim-state','claimed');` +
	`box.innerHTML='<i class="fa-solid fa-user-check text-accent"></i><p class="flex-1 text-sm text-fg">This character is yours. It is on your My Characters page.</p><span class="fx-pill is-yours">Yours</span>';` +
	`Chronicle.notify('Claimed. It is on your My Characters page.','success');` +
	`setTimeout(function(){location.reload();},1300);` +
	`}).catch(function(){fail('Network error. Try again.');});` +
	`})(this,event)`

// claimSubmitCall is the claim form's submit handler.
var claimSubmitCall = inlineHandler("claim_submit", claimSubmitJS)

// claimRetryCall resubmits the claim form from the amber bar's "Try again".
var claimRetryCall = inlineHandler("claim_retry",
	`(function(b){var f=b.closest('[data-claim-wrap]').querySelector('form');`+
		`if(f.requestSubmit)f.requestSubmit();else f.submit();})(this)`)
