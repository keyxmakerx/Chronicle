// calendar_a11y_test.go — C-CAL-WORLDSTATE-PREPORT-HARDENING (Slice 2 — UX).
//
// Static guards on the a11y + reduced-motion hardening: the four popups are
// dialogs with focus management + a single Escape/Tab handler; the live
// reduced-motion listener + the frame-renderer static-seed; the demo-only
// window.prompt is flagged. Behavioural focus paths (focus-into/restore, the
// Tab trap) need a real DOM — covered structurally here + flagged for a
// jsdom/browser pass; the dispatch sanctions this fallback.

package demo

import (
	"strings"
	"testing"
)

func TestCalAlmanac_DialogA11y(t *testing.T) {
	html := renderAlmanac(t)
	// Each popup root is a labelled modal dialog (+ tabindex so it can take focus).
	for _, root := range []string{"data-cal-qv", "data-cal-create", "data-cal-editor", "data-cal-skypanel"} {
		// find the opening tag for this root and assert the a11y attrs are on it
		idx := strings.Index(html, root)
		if idx < 0 {
			t.Errorf("popup root %q missing", root)
			continue
		}
		// the <aside ...> open tag containing this root attr
		start := strings.LastIndex(html[:idx], "<aside")
		end := strings.Index(html[idx:], ">")
		if start < 0 || end < 0 {
			t.Errorf("could not bound the %q tag", root)
			continue
		}
		tag := html[start : idx+end]
		for _, attr := range []string{`role="dialog"`, `aria-modal="true"`, `tabindex="-1"`, "aria-label="} {
			if !strings.Contains(tag, attr) {
				t.Errorf("%s dialog missing %q", root, attr)
			}
		}
	}
	if n := strings.Count(html, `role="dialog"`); n < 4 {
		t.Errorf("expected ≥4 role=dialog popups; got %d", n)
	}
}

func TestCalAlmanac_DialogFocusJS(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"function openDialog(el, closeFn)", // focus-into + push
		"function closeDialog(el)",         // focus-restore to trigger
		"d.trigger.focus()",                // restore to the opener
		"registerInitBlock('dialog-a11y'",  // single global handler
		"DIALOG_STACK",                     // dialog stack (topmost)
		"ev.key === 'Tab'",                 // Tab focus-trap
		"ev.key === 'Escape'",              // Escape closes topmost
		"openDialog(qv, closeQuickview)", "openDialog(ed, closeEditor)",
		"openDialog(p, closeSkyPanel)", "openDialog(pop, closeCreatePopup)",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("dialog-a11y marker missing: %q", m)
		}
	}
	// The two drifted Escape listeners are gone (deduped into the single handler).
	for _, gone := range []string{
		"if (ev.key === 'Escape') { closeEditor(); closeQuickview(); closeSkyPanel(); }",
		"if (e.key === 'Escape') closeCreatePopup();",
	} {
		if strings.Contains(js, gone) {
			t.Errorf("duplicate Escape listener should be removed: %q", gone)
		}
	}
}

func TestCalAlmanac_ReducedMotionCompleteness(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"window.matchMedia('(prefers-reduced-motion: reduce)')", // (still used)
		"onRMChange",             // live change listener (was read-once)
		"mq.addEventListener",    // subscribe to the change
		"if (!drops.length) for", // rain/storm static seed (dt=0 not blank)
		"if (!fl.length) for",    // snow/ash static seed
	} {
		if !strings.Contains(js, m) {
			t.Errorf("reduced-motion-completeness marker missing: %q", m)
		}
	}
}

func TestCalAlmanac_PromptFlagged(t *testing.T) {
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "window.prompt('[Demo]") {
		t.Errorf("the window.prompt shortcut must be gated as clearly demo-only")
	}
	if !strings.Contains(js, "do NOT carry prompt() into") {
		t.Errorf("the window.prompt must be flagged for the production editor")
	}
}
