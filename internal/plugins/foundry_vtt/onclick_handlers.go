package foundry_vtt

import (
	"fmt"
	"text/template"

	"github.com/a-h/templ"
)

// onclick-handler builders.
//
// C-FMC-8 architectural rule: every interactive element inside an
// HTMX-swapped fragment uses an INLINE IIFE in its onclick attribute,
// NOT a templ `script` helper.
//
// Why: templ `script` helpers emit a <script>function __templ_X(){...}</script>
// tag alongside the button + a button onclick that references
// __templ_X(...) by name. When the fragment is loaded via hx-get
// and innerHTML-swapped, browsers do NOT reliably execute the
// <script> tag synchronously with the surrounding HTML. The button
// can be clicked before the function is defined, raising:
//
//	Uncaught ReferenceError: __templ_X is not defined
//
// PRs #302, #303, C-FMC-5c (admin buttons), and C-FMC-6's owner_tab
// (Save Pin / Rotate Token) all hit this. Earlier doc comments
// claimed templ scripts were HTMX-swap-safe — that claim was
// load-bearing wrong and is corrected here.
//
// The fix: build the JS body in Go and inject it directly into the
// onclick attribute via templ.ComponentScript with an empty Function
// (no <script> tag) and the IIFE in Call. Templ writes Call straight
// into the onclick attribute value WITHOUT HTML-escaping — so the
// JS body MUST NOT contain literal double quotes (they would close
// the onclick="..." attribute early). Use single-quoted JS strings
// throughout; JSEscapeString-escape interpolated values so they
// can't break out of the surrounding ''.
//
// Validation: the inline_iife_test.go contract test renders every
// helper and asserts the output (a) starts with `(function(`, (b)
// contains no literal " character, and (c) doesn't contain
// `__templ_` runtime references. If the contract regresses, those
// tests fail loudly.

// inlineOnClick wraps a JS body in a ComponentScript with empty
// Function (no <script> tag emitted) so the body renders directly
// into the onclick attribute. Name is required by templ's script
// deduplication but is irrelevant here since Function is empty.
func inlineOnClick(name, jsBody string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     name,
		Function: "",
		Call:     jsBody,
	}
}

// jsStr returns a JS-string literal containing the given value,
// suitable for direct embedding in an inline IIFE that lives in an
// onclick attribute delimited by double quotes.
//
// Wraps with SINGLE quotes (the attribute uses double); content is
// JS-escaped so literal apostrophes, backslashes, and control chars
// become \uXXXX escapes that can't terminate the surrounding ''.
//
// Example: jsStr("v0'1.10") → "'v0\\u00271.10'"
//          jsStr("v0.1.10") → "'v0.1.10'"
func jsStr(s string) string {
	return "'" + template.JSEscapeString(s) + "'"
}

// notifyCampaignOnClick returns the inline IIFE for the per-campaign
// "Notify" button. Posts to the notify endpoint; surfaces success
// or failure via Chronicle.notify. Doesn't reload — notify is a
// side-effect (audit event + optional SMTP), no DOM change to
// reflect.
func notifyCampaignOnClick(campaignID, version string) templ.ComponentScript {
	cid := jsStr(campaignID)
	ver := jsStr(version)
	body := fmt.Sprintf(
		`(function(){`+
			`if(!window.confirm('Notify campaign owner that '+%s+' is available?'))return;`+
			`Chronicle.apiFetch('/admin/foundry-vtt/version/'+encodeURIComponent(%s)+'/notify/'+encodeURIComponent(%s),{method:'POST'})`+
			`.then(function(){window.Chronicle.notify('Notified campaign '+%s,'success');})`+
			`.catch(function(err){window.Chronicle.notify('Notify failed: '+((err&&err.message)||''),'error');});`+
			`})()`,
		ver, ver, cid, cid)
	return inlineOnClick("fvtt_notifyCampaign", body)
}

// forcePinCampaignOnClick returns the inline IIFE for the per-
// campaign "Force-update" button. Confirm dialog is firmer
// (destructive override of owner's pin); reloads page on success
// so the updated state reflects.
func forcePinCampaignOnClick(campaignID, version string) templ.ComponentScript {
	cid := jsStr(campaignID)
	ver := jsStr(version)
	body := fmt.Sprintf(
		`(function(){`+
			// "owner\'s" — backslash-escape the apostrophe so it
			// doesn't close the surrounding single-quoted JS string.
			// Backslash is literal in Go raw strings, so the output
			// contains \' which JS interprets as a literal '.
			`if(!window.confirm('Force-update campaign '+%s+' to '+%s+'? This overrides the owner\'s current pin.'))return;`+
			`Chronicle.apiFetch('/admin/foundry-vtt/version/'+encodeURIComponent(%s)+'/force-pin/'+encodeURIComponent(%s),{method:'POST'})`+
			`.then(function(){window.Chronicle.notify('Force-updated to '+%s,'success');setTimeout(function(){window.location.reload();},600);})`+
			`.catch(function(err){window.Chronicle.notify('Force-update failed: '+((err&&err.message)||''),'error');});`+
			`})()`,
		cid, ver, ver, cid, ver)
	return inlineOnClick("fvtt_forcePinCampaign", body)
}

// notifyOlderOnClick returns the inline IIFE for the mass-notify
// version-level action. Surfaces the notified count.
func notifyOlderOnClick(version string) templ.ComponentScript {
	ver := jsStr(version)
	body := fmt.Sprintf(
		`(function(){`+
			`if(!window.confirm('Notify EVERY campaign with a pin older than '+%s+'?'))return;`+
			`Chronicle.apiFetch('/admin/foundry-vtt/version/'+encodeURIComponent(%s)+'/notify-older',{method:'POST'})`+
			`.then(function(resp){var n=(resp&&typeof resp.notified==='number')?resp.notified:0;window.Chronicle.notify('Notified '+n+' campaign(s).','success');})`+
			`.catch(function(err){window.Chronicle.notify('Mass-notify failed: '+((err&&err.message)||''),'error');});`+
			`})()`,
		ver, ver)
	return inlineOnClick("fvtt_notifyOlder", body)
}

// forcePinOlderOnClick returns the inline IIFE for the mass force-
// pin version-level action. Confirm is sterner; reloads on success.
func forcePinOlderOnClick(version string) templ.ComponentScript {
	ver := jsStr(version)
	body := fmt.Sprintf(
		`(function(){`+
			`if(!window.confirm('Force-update EVERY campaign with a pin older than '+%s+'? This overrides every affected owner\'s pin. Confirm only if rolling out a critical update.'))return;`+
			`Chronicle.apiFetch('/admin/foundry-vtt/version/'+encodeURIComponent(%s)+'/force-pin-older',{method:'POST'})`+
			`.then(function(resp){var n=(resp&&typeof resp.pinned==='number')?resp.pinned:0;window.Chronicle.notify('Force-updated '+n+' campaign(s) to '+%s,'success');setTimeout(function(){window.location.reload();},600);})`+
			`.catch(function(err){window.Chronicle.notify('Mass force-update failed: '+((err&&err.message)||''),'error');});`+
			`})()`,
		ver, ver, ver)
	return inlineOnClick("fvtt_forcePinOlder", body)
}

// pinSaveOnClick returns the inline IIFE for the owner's "Save Pin"
// button. Reads the selected value from the version dropdown and
// PUTs to the pin endpoint.
func pinSaveOnClick(campaignID string) templ.ComponentScript {
	cid := jsStr(campaignID)
	body := fmt.Sprintf(
		`(function(){`+
			`var sel=document.getElementById('fvtt-pin-selector');if(!sel)return;`+
			`var version=sel.value;`+
			`Chronicle.apiFetch('/campaigns/'+encodeURIComponent(%s)+'/foundry-vtt/pin',{method:'PUT',body:JSON.stringify({version:version}),headers:{'Content-Type':'application/json'}})`+
			`.then(function(){window.Chronicle.notify(version?('Pinned to '+version):'Set to auto-update','success');setTimeout(function(){window.location.reload();},600);})`+
			`.catch(function(err){window.Chronicle.notify('Pin failed: '+((err&&err.message)||''),'error');});`+
			`})()`,
		cid)
	return inlineOnClick("fvtt_pinSave", body)
}

// rotateTokenOnClick returns the inline IIFE for the owner's
// "Rotate Token" button. Confirm dialog because rotation invalidates
// every previously-issued install URL.
func rotateTokenOnClick(campaignID string) templ.ComponentScript {
	cid := jsStr(campaignID)
	body := fmt.Sprintf(
		`(function(){`+
			`if(!window.confirm('Rotate the install token? Every player will need to reinstall with the new URL.'))return;`+
			`Chronicle.apiFetch('/campaigns/'+encodeURIComponent(%s)+'/foundry-vtt/token/rotate',{method:'POST'})`+
			`.then(function(){window.Chronicle.notify('Token rotated. All players will need to reinstall.','success');setTimeout(function(){window.location.reload();},600);})`+
			`.catch(function(err){window.Chronicle.notify('Rotate failed: '+((err&&err.message)||''),'error');});`+
			`})()`,
		cid)
	return inlineOnClick("fvtt_rotateToken", body)
}

// dismissAutoPinBannerOnClick returns the inline IIFE for the
// admin banner's "Dismiss" button. Calls the dismiss endpoint +
// hides the banner DOM element. Added in C-FMC-8 along with the
// banner itself (deferred from C-FMC-6).
func dismissAutoPinBannerOnClick() templ.ComponentScript {
	body := `(function(){` +
		`Chronicle.apiFetch('/admin/foundry-vtt/autopin-banner/dismiss',{method:'POST'})` +
		`.then(function(){var b=document.getElementById('fvtt-autopin-banner');if(b)b.style.display='none';})` +
		`.catch(function(err){window.Chronicle.notify('Dismiss failed: '+((err&&err.message)||''),'error');});` +
		`})()`
	return inlineOnClick("fvtt_dismissAutoPinBanner", body)
}
