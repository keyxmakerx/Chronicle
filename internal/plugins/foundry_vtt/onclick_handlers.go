package foundry_vtt

import (
	"fmt"
	"text/template"

	"github.com/a-h/templ"
)

// onclick-handler builders.
//
// Rule: every interactive element inside an HTMX-swapped fragment uses an
// inline IIFE in its onclick attribute, never a templ `script` helper — a
// script helper's separate <script> tag isn't guaranteed to execute before
// an hx-get innerHTML swap lets the button be clicked ("__templ_X is not
// defined"). Build the JS body in Go and inject it via
// templ.ComponentScript with an empty Function and the IIFE in Call, which
// templ writes into onclick without HTML-escaping: use single-quoted JS
// strings and JSEscapeString-escape interpolated values, never a literal
// double quote.
//
// onclick_handlers_test.go pins the contract: every helper's output starts
// with `(function(`, contains no literal `"`, and never references
// `__templ_`.

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

// rotateTokenOnClick returns the inline IIFE for the owner's
// "Rotate Token" button. Confirm dialog because rotation invalidates
// every previously-issued install URL.
func rotateTokenOnClick(campaignID string) templ.ComponentScript {
	cid := jsStr(campaignID)
	body := fmt.Sprintf(
		`(function(){`+
			`if(!window.confirm('Rotate the install URL? The one saved in Foundry stops working until you reinstall the module there with the new URL. Players are not affected.'))return;`+
			`Chronicle.apiFetch('/campaigns/'+encodeURIComponent(%s)+'/foundry-vtt/token/rotate',{method:'POST'})`+
			`.then(function(){window.Chronicle.notify('Install URL rotated. Reinstall the module in Foundry with the new URL.','success');setTimeout(function(){window.location.reload();},600);})`+
			`.catch(function(err){window.Chronicle.notify('Rotate failed: '+((err&&err.message)||''),'error');});`+
			`})()`,
		cid)
	return inlineOnClick("fvtt_rotateToken", body)
}

// dismissAutoPinBannerOnClick returns the inline IIFE for the admin
// banner's "Dismiss" button: calls the dismiss endpoint and hides the
// banner DOM element.
func dismissAutoPinBannerOnClick() templ.ComponentScript {
	body := `(function(){` +
		`Chronicle.apiFetch('/admin/foundry-vtt/autopin-banner/dismiss',{method:'POST'})` +
		`.then(function(){var b=document.getElementById('fvtt-autopin-banner');if(b)b.style.display='none';})` +
		`.catch(function(err){window.Chronicle.notify('Dismiss failed: '+((err&&err.message)||''),'error');});` +
		`})()`
	return inlineOnClick("fvtt_dismissAutoPinBanner", body)
}

// showAffectedCampaignsOnClick returns the inline IIFE for the banner's
// "Show affected campaigns" button: scroll + click the
// `fvtt-campaigns-trigger-<sanitized-version>` button on /admin/packages
// (its hx-get fetches the campaigns-using-version fragment). That button
// only exists once the Versions list is HTMX-expanded, so if it isn't in
// the DOM yet, click the Versions trigger (found via its
// `data-fvtt-versions-trigger` attribute, so the server needn't know the
// package ID at render time) and retry on `htmx:afterSwap`.
func showAffectedCampaignsOnClick(previousVersion string) templ.ComponentScript {
	// sanitizeForID(version) replaces dots/plus/slash with hyphens —
	// must match the packages.templ sanitizeForID helper. Calling
	// that helper would create an inter-plugin import. The IDs are
	// version strings so the replacement is straightforward; pre-
	// compute on the server side for the inline IIFE.
	sanitized := sanitizeVersionForDOMID(previousVersion)
	id := template.JSEscapeString("fvtt-campaigns-trigger-" + sanitized)
	body := fmt.Sprintf(
		`(function(){`+
			`var id='%s';`+
			// Fast path: campaigns trigger already in DOM → scroll + click.
			`var existing=document.getElementById(id);`+
			`if(existing){existing.scrollIntoView({behavior:'smooth',block:'center'});existing.click();return;}`+
			// Stage 1: locate the foundry-module Versions trigger.
			`var v=document.querySelector('[data-fvtt-versions-trigger]');`+
			`if(!v){window.Chronicle.notify('Versions list could not be located on this page; reload and try again.','error');return;}`+
			// Stage 2: wire a one-shot listener on the swap target,
			// then click the Versions trigger to load the list.
			`var targetSel=v.getAttribute('hx-target');`+
			`var target=targetSel?document.querySelector(targetSel):null;`+
			`var stage2=function(){var b=document.getElementById(id);if(b){b.scrollIntoView({behavior:'smooth',block:'center'});b.click();}else{window.Chronicle.notify('Version row not found after expanding the list; the version may have been pruned.','error');}};`+
			`if(target){var onSwap=function(){target.removeEventListener('htmx:afterSwap',onSwap);stage2();};target.addEventListener('htmx:afterSwap',onSwap);}`+
			// Fallback timer: if no swap target was wired, still attempt
			// stage 2 after a beat in case HTMX completed faster than us.
			`else{setTimeout(stage2,600);}`+
			`v.click();`+
			`})()`,
		id)
	return inlineOnClick("fvtt_showAffected", body)
}

// sanitizeVersionForDOMID mirrors packages.sanitizeForID's behavior for
// the foundry-module version strings the banner needs to target.
// Duplicated locally to avoid a cross-plugin import; must stay in
// lock-step with packages/templ_helpers.go's sanitizeForID or
// showAffectedCampaignsOnClick silently fails to find its target.
func sanitizeVersionForDOMID(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '.', '+', '/':
			out = append(out, '-')
		default:
			out = append(out, c)
		}
	}
	return string(out)
}
