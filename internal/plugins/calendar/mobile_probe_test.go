package calendar

// mobile_probe_test.go — C-CALV4-MOBILE, the phone pass.
//
// ── WHY THIS FILE EXISTS ──────────────────────────────────────────────────
//
// The readiness audit's mobile lane returned NOT-READY, the only one of five
// that did, and every finding in it is a RENDERED number: a Ledger showing 41
// of the 220 pixels its five rows need, a Save button that leaves the viewport
// when a keyboard opens, a page that scrolls out from under an open sheet.
// None of those can be asserted on a string. They are measured here, in a real
// headless Chromium, over the real BenchPage output, the real stylesheets and
// the real module — the same substitute rig every other probe in this package
// uses.
//
// ── HOW A PHONE WIDTH IS REACHED, AND IT IS SAID ON EVERY ARTEFACT ────────
//
// [MOB-10] SIGNED: "every phone artefact states on its face how it reached a
// phone width." This Chromium (/opt/pw-browsers/chromium-1194) CLAMPS the
// headless window to a 500px minimum, so `--window-size=390,844` yields
// innerWidth 500 — the substitution that made the rejected evidence set worth
// nothing.
//
// EVERY MEASUREMENT IN THIS FILE IS TAKEN IN A **NESTED BROWSING CONTEXT**
// (an iframe), never in a device-metrics override. That choice is stated in
// every t.Logf header this file emits, because the two are NOT equivalent: an
// iframe has its own layout viewport, its own `innerWidth`, its own `100vw`
// and its own media-query resolution — but it has NO visual viewport of its
// own and no URL bar, so `dvh` in a frame resolves to the frame's box rather
// than to a phone's retracting-chrome behaviour. Where that distinction is
// load-bearing ([MOB-2]'s `dvh` swap) the probe says so at the assertion.
//
// A RESIZE IN A FRAME IS A REAL RESIZE. Changing the iframe's inline/block
// size from the parent re-lays-out the child and fires a genuine `resize`
// event in the child's window, which is exactly the event a software keyboard
// produces on Android Chrome. That is what the shrink arms below use.
//
// ── WHAT IS MEASURED, BY RULING ───────────────────────────────────────────
//
//	[MOB-1]  the Ledger's floor + the Block's narrow-tier cap
//	[MOB-2]  the sheet's geometry, under a shrink and under a rotation
//	[MOB-3]  the page lock and — ruled harder — its release
//	[MOB-4]  the scroller census and overscroll containment
//	[MOB-5]  the RSVP overview's horizontal drag
//	[MOB-8]  long weeks, and that a tenday grows no scroller it does not need
//	[MOB-9]  `.edmorph` absent from the settled open state
//
// Browser-gated only — no env gate — because these five are registered in
// tools/check-browser-probes.sh and a registered probe that skips itself by
// default is a silent pass wearing a name.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// mobileMinWindowPx is the headless clamp this rig exists to defeat. It is the
// same number daycard_floors_probe_test.go measured and named.
const mobileMinWindowPx = 500

// mobileWidths are the three phone widths every acceptance row in
// C-CALV4-MOBILE is stated at.
var mobileWidths = []int{390, 375, 360}

// mobilePagePadPx is the app shell's own horizontal padding at phone width
// (<main class="px-3 …">), the same number schedule_shots_test.go pins as
// schedulePagePadNarrow. The Block's host width is the viewport less twice
// this, which is why a "390px phone" is a 366px Block.
const mobilePagePadPx = 12

// ── the step protocol ──────────────────────────────────────────────────────
//
// A probe run is a list of steps. Each step optionally runs a line of script
// in the PARENT (which is how the frame is resized — the child cannot resize
// its own layout viewport, exactly as a page cannot open a keyboard) and then
// posts one command into the CHILD, whose reply is appended to the result
// array. Delays are virtual-time milliseconds.
type mobileStep struct {
	// Outer runs in the parent document. `f` is the iframe element.
	Outer string
	// Op is posted to the child; the child answers with one JSON object.
	Op string
	// Delay is how long to wait after Outer before posting Op.
	Delay int
}

type mobileReply map[string]any

func (r mobileReply) num(k string) float64 {
	if v, ok := r[k].(float64); ok {
		return v
	}
	return 0
}

func (r mobileReply) str(k string) string {
	if v, ok := r[k].(string); ok {
		return v
	}
	return ""
}

func (r mobileReply) boolean(k string) bool {
	if v, ok := r[k].(bool); ok {
		return v
	}
	return false
}

var mobileResultRe = regexp.MustCompile(`(?s)<pre id="mob">(.*?)</pre>`)

// mobileDrive writes the inner page, wraps it in a frame of the requested
// size, runs the steps and returns one reply per step.
func mobileDrive(t *testing.T, chrome, inner string, w, h int, steps []mobileStep) []mobileReply {
	t.Helper()
	dir := filepath.Dir(inner)
	outer := filepath.Join(dir, fmt.Sprintf("frame-%d-%d-%s", w, h, filepath.Base(inner)))

	var js strings.Builder
	at := 300
	for i, s := range steps {
		at += s.Delay
		fmt.Fprintf(&js, "setTimeout(function(){ %s; post(%q); }, %d);\n",
			s.Outer, s.Op, at)
		at += 300
		_ = i
	}

	frame := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0;background:#222}` +
		fmt.Sprintf(`#f{inline-size:%dpx;block-size:%dpx;border:0;display:block}`, w, h) +
		`</style></head><body><pre id="mob">[]</pre>` +
		`<iframe id="f" src="` + filepath.Base(inner) + `"></iframe>` +
		`<script>` +
		`var f=document.getElementById('f');var out=[];` +
		`function flush(){document.getElementById('mob').textContent=JSON.stringify(out);}` +
		`window.addEventListener('message',function(e){out.push(e.data);flush();});` +
		`function post(op){ if(f.contentWindow) f.contentWindow.postMessage(op,'*'); }` +
		`function size(a,b){ f.style.inlineSize=a+'px'; f.style.blockSize=b+'px'; }` +
		js.String() +
		`</script></body></html>`
	if err := os.WriteFile(outer, []byte(frame), 0o644); err != nil {
		t.Fatalf("write frame page: %v", err)
	}

	win := w + 80
	if win < mobileMinWindowPx {
		win = mobileMinWindowPx
	}
	// HEADROOM. The steps run on VIRTUAL time, so this is deterministic rather
	// than a race — but a truncated budget would drop the tail replies and
	// mobileDrive would fatal on the count rather than on a measurement, which
	// reads as a flake and is not one.
	budget := at + 3000
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", win, h+400),
		fmt.Sprintf("--virtual-time-budget=%d", budget),
		"--dump-dom", "file://"+outer,
	)
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium dump-dom at %dx%d: %v", w, h, err)
	}
	m := mobileResultRe.FindSubmatch(raw)
	if m == nil {
		t.Fatalf(`no <pre id="mob"> in the dump at %dx%d`, w, h)
	}
	body := string(m[1])
	for _, sub := range [][2]string{{"&quot;", `"`}, {"&amp;", "&"}, {"&lt;", "<"}, {"&gt;", ">"}, {"&#39;", "'"}} {
		body = strings.ReplaceAll(body, sub[0], sub[1])
	}
	var wire []string
	if err := json.Unmarshal([]byte(body), &wire); err != nil {
		t.Fatalf("frame payload at %dx%d is not a JSON array: %v\n%s", w, h, err, body)
	}
	out := make([]mobileReply, 0, len(wire))
	for _, s := range wire {
		var r mobileReply
		if err := json.Unmarshal([]byte(s), &r); err != nil {
			t.Fatalf("reply %q is not JSON: %v", s, err)
		}
		out = append(out, r)
	}
	if len(out) != len(steps) {
		t.Fatalf("asked for %d steps and got %d replies — the child stopped answering:\n%s",
			len(steps), len(out), body)
	}
	return out
}

// mobileInnerPage is the measurement page: the real BenchPage output, the real
// four stylesheets, the real module, plus the command listener. It carries the
// app shell's own phone padding so the Block's host width is the product's.
func mobileInnerPage(t *testing.T, data BenchData, extraCSS string) string {
	return mobileInnerPageWith(t, data, extraCSS, "")
}

// mobileInnerPageWith is the same page plus one script injected AFTER the
// module and BEFORE the ops listener. It exists for the edit-mode arm, which
// needs the capture rig's stubbed GET installed before a record is loaded —
// the same substitution daycard_screenshot_gen_test.go discloses, and the only
// fabrication anywhere in this file: the RECORD is canned, the chrome that
// draws it is the shipped chrome.
func mobileInnerPageWith(t *testing.T, data BenchData, extraCSS, extraScript string) string {
	t.Helper()
	css := benchCSS(t) + benchBlockSheet(t) + dayCardCSS(t) + extraCSS
	mod := readRepoFile(t, "internal/plugins/calendar/static/js/calendar_daycard.js")
	vis := readRepoFile(t, "internal/plugins/calendar/static/js/cal_visibility.js")
	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0;background:#f9fafb;` +
		`font-family:ui-sans-serif,system-ui,-apple-system,sans-serif}` +
		fmt.Sprintf(`.mob-main{padding:0 %dpx}`, mobilePagePadPx) +
		css +
		`</style></head><body><div class="mob-main">` +
		benchStripLinks(renderBench(t, data)) +
		`</div>` +
		`<script>` + vis + `</script><script>` + mod + `</script>` +
		`<script>` + extraScript + `</script>` +
		`<script>` + mobileOpsScript + `</script>` +
		`</body></html>`
}

// mobileWriteInner writes the inner page into a temp dir and returns its path.
func mobileWriteInner(t *testing.T, name, page string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(page), 0o644); err != nil {
		t.Fatalf("write inner page: %v", err)
	}
	return p
}

// mobileNeedChromium is the browser gate. It is the ONLY gate: no env var
// hides these probes, because they are registered in
// tools/check-browser-probes.sh and a registered probe that skips by default
// is the silent pass that guard exists to kill.
func mobileNeedChromium(t *testing.T) string {
	t.Helper()
	chrome := benchFindChromium()
	if chrome == "" {
		t.Skip("mobile probe: no Chromium binary found (set CHROMIUM_BIN)")
	}
	return chrome
}

// mobileHeader prints the browsing-context disclosure [MOB-10] requires on the
// face of every artefact. It is printed by every probe in this file, first.
func mobileHeader(t *testing.T, what string, w, h int) {
	t.Helper()
	t.Logf("── %s · %dx%d ─────────────────────────────────────────", what, w, h)
	t.Logf("   BROWSING CONTEXT: NESTED (iframe). Not a CDP device-metrics override.")
	t.Logf("   headless Chromium clamps its window to %dpx, so a bare --window-size=%d "+
		"would have measured %dpx wearing the name %d.", mobileMinWindowPx, w, mobileMinWindowPx, w)
}

// mobileOpsScript is the child's command listener — the whole of the in-page
// measurement. One `op` in, one JSON object out, posted to the parent.
//
// It reads the LIVE layout only: getBoundingClientRect, clientHeight,
// scrollHeight, getComputedStyle. It never re-derives a number from a
// stylesheet, because a declaration is not a used value — `min-height` loses
// to a `height`, to a flex shrink and to four other things that decide a box.
const mobileOpsScript = `
(function () {
  function el(s) { return document.querySelector(s); }
  function all(s) { return Array.prototype.slice.call(document.querySelectorAll(s)); }
  function box(node) {
    if (!node) return null;
    var r = node.getBoundingClientRect();
    return {
      top: Math.round(r.top), bottom: Math.round(r.bottom),
      left: Math.round(r.left), right: Math.round(r.right),
      w: Math.round(r.width), h: Math.round(r.height),
      ch: node.clientHeight, sh: node.scrollHeight,
      cw: node.clientWidth, sw: node.scrollWidth
    };
  }
  function ov(node, prop) { return node ? getComputedStyle(node)[prop] : ''; }
  function settle() {
    if (!document.getAnimations) return;
    document.getAnimations().forEach(function (a) {
      try { a.finish(); } catch (e) { /* a discrete step has nothing to finish */ }
    });
  }
  function reply(o) { o.innerWidth = window.innerWidth; o.innerHeight = window.innerHeight;
    parent.postMessage(JSON.stringify(o), '*'); }

  // rowsFullyInside counts LEDGER ROWS ENTIRELY INSIDE THEIR SCROLL WINDOW.
  // "0 of 5 visible" is the founding measurement and it is a count of boxes,
  // not a ratio of heights.
  function rowsInside(rowsBox) {
    if (!rowsBox) return { rows: 0, inside: 0, clipped: 0 };
    var wr = rowsBox.getBoundingClientRect();
    var rows = Array.prototype.slice.call(rowsBox.querySelectorAll('.lrow'));
    var inside = 0;
    rows.forEach(function (r) {
      var b = r.getBoundingClientRect();
      if (b.top >= wr.top - 0.5 && b.bottom <= wr.bottom + 0.5) inside++;
    });
    return { rows: rows.length, inside: inside, clipped: rows.length - inside };
  }

  // scrollers is the CENSUS: every element that can actually absorb a swipe.
  //
  // THE DEFINITION IS A THUMB'S, NOT A LAYOUT ENGINE'S, and both halves of it
  // are load-bearing. A box counts when (1) it overflows on that axis, (2) its
  // computed overflow there is 'auto' or 'scroll' — an 'overflow: hidden' box
  // clips and cannot be swiped at all, so counting it would inflate the census
  // with boxes no finger can reach — and (3) it is at least 24px on that axis.
  // Without (3) the census returns 132 regions at 390, almost all of them the
  // 1px 'span.vh' screen-reader clips that every surface here uses; a 1px box
  // is not one of the forty-pixel bands the operator's complaint is about.
  function scrollable(v) { return v === 'auto' || v === 'scroll'; }
  function scrollers(rootSel) {
    var root = rootSel ? el(rootSel) : document.body;
    if (!root) return [];
    var out = [];
    all(rootSel ? rootSel + ' *' : '*').forEach(function (n) {
      var cs = getComputedStyle(n);
      var vy = scrollable(cs.overflowY) && n.clientHeight >= 24;
      var vx = scrollable(cs.overflowX) && n.clientWidth >= 24;
      if (vy && n.scrollHeight - n.clientHeight > 1) {
        out.push({ sel: name(n), axis: 'y', c: n.clientHeight, s: n.scrollHeight,
          contain: cs.overscrollBehaviorY });
      }
      if (vx && n.scrollWidth - n.clientWidth > 1) {
        out.push({ sel: name(n), axis: 'x', c: n.clientWidth, s: n.scrollWidth,
          contain: cs.overscrollBehaviorX });
      }
    });
    return out;
  }
  function name(n) {
    var s = n.tagName.toLowerCase();
    if (n.className && typeof n.className === 'string') {
      s += '.' + n.className.trim().split(/\s+/).join('.');
    }
    return s;
  }

  function fold() {
    var d = document.scrollingElement || document.documentElement;
    return { sw: d.scrollWidth, cw: d.clientWidth, top: d.scrollTop };
  }

  function firstCell() { return el('[data-bench-block] [data-day][data-day-ord]'); }

  var ops = {
    // ── [MOB-1] ──────────────────────────────────────────────────────────
    zones: function () {
      var host = el('.cal-block-host');
      var lr = el('.cal-block-host .lrows');
      var r = rowsInside(lr);
      return {
        host: box(host), block: box('.cal-block-host .block' ? el('.cal-block-host .block') : null),
        np: box(el('.cal-block-host .np')), body: box(el('.cal-block-host .body')),
        inst: box(el('.cal-block-host .inst')), grid: box(el('.cal-block-host .grid')),
        ledger: box(el('.cal-block-host .ledger')), lrows: box(lr),
        shelf: box(el('.cal-block-host .shelf')), sp2: box(el('.cal-block-host .shelf .sp2')),
        rows: r.rows, rowsInside: r.inside, rowsClipped: r.clipped,
        blockMaxH: ov(el('.cal-block-host .block'), 'maxHeight'),
        lrowsMinH: ov(lr, 'minHeight'),
        lrowH: (function () { var x = el('.cal-block-host .lrow'); return x ? x.getBoundingClientRect().height : 0; })(),
        lrowsOverscroll: ov(lr, 'overscrollBehaviorY'),
        blocks: all('.cal-block-host .block').length
      };
    },
    // every .lrows on the page, so the second (real-world) Block is measured too
    lrowsAll: function () {
      return { list: all('.cal-block-host .lrows').map(function (n) {
        var r = rowsInside(n);
        return { ch: n.clientHeight, sh: n.scrollHeight, rows: r.rows, inside: r.inside,
          minH: getComputedStyle(n).minHeight, contain: getComputedStyle(n).overscrollBehaviorY };
      }) };
    },
    // Block height with a day SELECTED, for the declared-height invariance.
    selectDay: function () {
      var c = firstCell();
      var pick = el('.cal-block-host input.daypick');
      if (pick) pick.click();
      else if (c) c.click();
      settle();
      return { block: box(el('.cal-block-host .block')), picked: !!pick };
    },
    blockHeight: function () { return { block: box(el('.cal-block-host .block')) }; },
    // Shelf pane parity: click each tab and report the ZONE's height (the one
    // the host page feels) alongside the pane's own, plus the Block's.
    shelfPanes: function () {
      var tabs = all('.cal-block-host .shelf .st [role="tab"], .cal-block-host .shelf .st button, .cal-block-host .shelf .st label');
      var seen = [];
      tabs.forEach(function (tb) {
        try { tb.click(); } catch (e) { /* not a control */ }
        settle();
        var sh = el('.cal-block-host .shelf');
        var sp = el('.cal-block-host .shelf .sp2');
        var bl = el('.cal-block-host .block');
        seen.push({ label: (tb.textContent || '').trim().slice(0, 20),
          h: sh ? Math.round(sh.getBoundingClientRect().height) : -1,
          pane: sp ? Math.round(sp.getBoundingClientRect().height) : -1,
          paneScroll: sp ? sp.scrollHeight : -1,
          block: bl ? Math.round(bl.getBoundingClientRect().height) : -1 });
      });
      return { panes: seen };
    },

    // ── [MOB-4] / [MOB-8] ────────────────────────────────────────────────
    census: function () {
      return { bench: scrollers('.cal-bench'), page: fold(),
        instOX: ov(el('.cal-block-host .inst'), 'overflowX'),
        instOY: ov(el('.cal-block-host .inst'), 'overflowY'),
        inst: box(el('.cal-block-host .inst')), grid: box(el('.cal-block-host .grid')),
        instContainX: ov(el('.cal-block-host .inst'), 'overscrollBehaviorX'),
        weekLen: (function () { var g = el('.cal-block-host .grid');
          return g ? g.getAttribute('data-week-len') : ''; })(),
        headers: (function () {
          var hs = all('.cal-block-host .grid .hd b');
          var last = hs.length ? hs[hs.length - 1].getBoundingClientRect() : null;
          var zero = 0;
          all('.cal-block-host .cell').forEach(function (c) {
            if (c.getBoundingClientRect().width < 1) zero++;
          });
          return { count: hs.length, lastRight: last ? Math.round(last.right) : 0, zeroCells: zero };
        })() };
    },
    ovwrap: function () {
      var w = el('.cal-bench .rsvp .ovwrap');
      var names = all('.cal-bench .rsvp .ovgrid .nm, .cal-bench .rsvp .mrow .nm').map(function (n) {
        return { t: (n.textContent || '').trim(), cw: n.clientWidth, sw: n.scrollWidth };
      });
      return { wrap: box(w), grid: box(el('.cal-bench .rsvp .ovgrid')),
        contain: ov(w, 'overscrollBehaviorX'), page: fold(),
        zone: !!el('.cal-bench .rsvp [data-rsvp-zone-missing], .cal-bench .rsvp .zonewarn'),
        text: (function () { var p = el('.cal-bench .rsvp'); return p ? (p.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 900) : ''; })(),
        names: names };
    },

    // ── [MOB-2] / [MOB-9] ────────────────────────────────────────────────
    openCard: function () {
      var c = firstCell(); if (c) c.click(); settle();
      return { card: box(el('.cal-daycard')), open: !!el('.cal-daycard[data-dc-shown]') };
    },
    openEditor: function () {
      var d = el('[data-dc-new]'); if (d) d.click(); settle();
      return { ok: !!el('.cal-dayeditor[data-dc-shown]') };
    },
    // EDIT MODE, which is the only mode Delete exists in: the module reveals
    // [data-de-delete] only once a record has an id, because a DELETE cannot
    // name an event the POST has not returned one for.
    openEditRow: function () {
      var cells = all('[data-bench-block] [data-day][data-day-ord]');
      var best = null, bestN = 0;
      cells.forEach(function (c) {
        c.click();
        var n = document.querySelectorAll('[data-cal-daycard] [data-dc-edit]').length;
        if (n > bestN) { bestN = n; best = c; }
      });
      if (best) {
        best.click();
        var door = el('[data-cal-daycard] [data-dc-edit]');
        if (door) door.click();
      }
      settle();
      return { ok: !!el('.cal-dayeditor[data-dc-shown]'), doors: bestN };
    },
    sheet: function () {
      settle();
      var root = el('.cal-dayeditor');
      var save = el('[data-de-save]');
      var cs = root ? getComputedStyle(root) : null;
      return {
        open: !!(root && root.hasAttribute('data-dc-shown')),
        inlineTop: root ? (root.style.top || '') : '',
        inlineWidth: root ? (root.style.width || '') : '',
        inlineLeft: root ? (root.style.left || '') : '',
        dcsheet: !!(root && root.classList.contains('dcsheet')),
        edmorph: !!(root && root.classList.contains('edmorph')),
        clear: root ? root.getAttribute('data-dc-clear') : '',
        root: box(root), save: box(save), edbody: box(el('.cal-dayeditor .ed-body')),
        edbodyMax: ov(el('.cal-dayeditor .ed-body'), 'maxBlockSize'),
        edbodyContain: ov(el('.cal-dayeditor .ed-body'), 'overscrollBehaviorY'),
        tdur: cs ? cs.transitionDuration : '', tprop: cs ? cs.transitionProperty : '',
        saveInside: (function () {
          if (!save) return false;
          var r = save.getBoundingClientRect();
          return r.top >= 0 && r.bottom <= window.innerHeight + 0.5;
        })()
      };
    },
    card: function () {
      settle();
      var root = el('.cal-daycard');
      return { root: box(root), dcsheet: !!(root && root.classList.contains('dcsheet')),
        inlineTop: root ? (root.style.top || '') : '',
        rowsContain: ov(el('.cal-daycard .dc-rows'), 'overscrollBehaviorY') };
    },

    // ── [MOB-3] ──────────────────────────────────────────────────────────
    preScroll: function () {
      window.scrollTo(0, 120);
      var d = document.scrollingElement || document.documentElement;
      return { top: d.scrollTop, bodyPos: getComputedStyle(document.body).position };
    },
    push: function () {
      window.scrollBy(0, 400);
      var d = document.scrollingElement || document.documentElement;
      return { top: d.scrollTop, htmlOverflow: getComputedStyle(document.documentElement).overflow,
        bodyOverflow: getComputedStyle(document.body).overflow,
        bodyPos: getComputedStyle(document.body).position,
        bodyTop: document.body.style.top || '' };
    },
    after: function () {
      var d = document.scrollingElement || document.documentElement;
      return { top: d.scrollTop, bodyPos: getComputedStyle(document.body).position,
        bodyTop: document.body.style.top || '',
        edOpen: !!el('.cal-dayeditor[data-dc-shown]'),
        cardOpen: !!el('.cal-daycard[data-dc-shown]') };
    },
    closeEscape: function () {
      var root = el('.cal-dayeditor[data-dc-shown]') || el('.cal-daycard[data-dc-shown]');
      var ev = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true });
      (root || document).dispatchEvent(ev);
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
      settle(); return { did: 'escape' };
    },
    closeX: function () {
      var x = el('.cal-dayeditor [data-de-cancel]'); if (x) x.click(); settle();
      return { did: 'x', found: !!x };
    },
    closeCancel: function () {
      var btns = all('.cal-dayeditor [data-de-cancel]');
      var b = btns.length > 1 ? btns[1] : btns[0]; if (b) b.click(); settle();
      return { did: 'cancel', found: !!b };
    },
    closeSave: function () {
      // The teardown path, driven directly: a save in a file:// page has no
      // server to answer it, so this exercises the SAME hide the module runs.
      var root = el('.cal-dayeditor');
      if (root && root.hidePopover) { try { root.hidePopover(); } catch (e) {} }
      settle(); return { did: 'save-teardown' };
    },
    closeOutside: function () {
      document.body.click();
      document.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
      document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      settle(); return { did: 'outside' };
    },
    closeCard: function () {
      var root = el('.cal-daycard');
      if (root && root.hidePopover) { try { root.hidePopover(); } catch (e) {} }
      settle(); return { did: 'card-teardown' };
    },
    // ── [MOB-7] the NAMED LIST, measured by selector ────────────────────
    targets: function () {
      settle();
      var named = [
        ['RSVP Yes / No / Maybe', '.cal-bench .rsvpb .btn'],
        ['Ask →', '.cal-bench .rsvp .mrow .btn'],
        ['the ribbon tile arrow', '.cal-bench .tile .eb .ar'],
        ['the Block Layers invoker', '.cal-block-host .icb.layers'],
        ['the Ledger Month tab', '.cal-block-host .ltab'],
        ['editor Save', '.cal-dayeditor [data-de-save]'],
        ['editor Cancel / ✕', '.cal-dayeditor [data-de-cancel]'],
        ['editor Delete', '.cal-dayeditor [data-de-delete]'],
        ['a day cell', '.cal-block-host .cell']
      ];
      var out = [];
      named.forEach(function (pair) {
        var boxes = all(pair[1]).filter(function (n) {
          var r = n.getBoundingClientRect();
          return r.width > 0 || r.height > 0;
        }).map(function (n) {
          var r = n.getBoundingClientRect();
          return { t: (n.textContent || '').trim().slice(0, 16),
            w: Math.round(r.width * 10) / 10, h: Math.round(r.height * 10) / 10 };
        });
        var minH = 1e9, minW = 1e9;
        boxes.forEach(function (b) { if (b.h < minH) minH = b.h; if (b.w < minW) minW = b.w; });
        out.push({ label: pair[0], sel: pair[1], n: boxes.length,
          minW: boxes.length ? minW : 0, minH: boxes.length ? minH : 0, boxes: boxes });
      });
      // THE ✕/DELETE GAP. They live in different rows of the same sheet, so
      // the distance is measured as a real edge-to-edge gap rather than
      // assumed from the DOM order.
      var x = el('.cal-dayeditor .ed-head [data-de-cancel]');
      var del = el('.cal-dayeditor [data-de-delete]');
      var gap = -1;
      if (x && del) {
        var a = x.getBoundingClientRect(), b = del.getBoundingClientRect();
        var dy = Math.max(0, Math.max(a.top - b.bottom, b.top - a.bottom));
        var dx = Math.max(0, Math.max(a.left - b.right, b.left - a.right));
        gap = Math.round(Math.max(dx, dy));
      }
      // C-CALV4-DAYCARD-WDWRAP, answered by measurement rather than carried:
      // the booking says the weekday strip wraps "6 + 4" at phone width. The
      // rows are counted from DISTINCT TOP EDGES, and "outside" is a stop whose
      // right edge crosses its own container's.
      var wd = el('.cal-dayeditor .wdpick');
      var strip = { stops: 0, rows: 0, outside: 0, right: 0 };
      if (wd) {
        var wr = wd.getBoundingClientRect();
        var tops = {};
        all('.cal-dayeditor .wdpick button').forEach(function (b) {
          var r = b.getBoundingClientRect();
          if (r.width <= 0 && r.height <= 0) return;
          strip.stops++;
          tops[Math.round(r.top)] = 1;
          if (r.right > wr.right + 0.5 || r.left < wr.left - 0.5) strip.outside++;
        });
        strip.rows = Object.keys(tops).length;
        strip.right = Math.round(wr.right);
      }
      return { list: out, xDeleteGap: gap, wdstrip: strip, deleteVisible: !!(del &&
        del.getBoundingClientRect().height > 0) };
    },
    noop: function () { return { did: 'noop', page: fold() }; }
  };

  window.addEventListener('message', function (e) {
    var op = String(e.data || '');
    var fn = ops[op];
    var out;
    try { out = fn ? fn() : { error: 'no op ' + op }; }
    catch (err) { out = { error: String(err && err.message || err), op: op }; }
    out.op = op;
    reply(out);
  });
})();
`

// ── [MOB-1] — the founding measurement ────────────────────────────────────
//
// BASELINE, from the readiness audit at 390x664: `.cal-block-host .lrows`
// clientHeight 41 against scrollHeight 220 — five 44px rows, ZERO fully
// visible. A second probe at host width 358 measured 14 against 82, with the
// empty-state sentence sheared mid-word.
//
// TARGET, ruled: rows fully inside the scroll window >= 3 and clientHeight
// >= 132, at 390 AND 375 AND 360. 132 is 3 x 44, where 44 is the sheet's own
// `.cal-block-host .lrow { height: 44px }`.
//
// AND THE INVARIANCE THE CAP WAS BUYING IS RE-BOUGHT, NOT DROPPED
// (calendar-block.css §SIZE CLASS: "Declared heights are INVARIANT so dropping
// a Block into any host page can never shove that page around"). Two rows
// below prove it by measurement: the Block's height is identical with a day
// selected and with none, and the Shelf's `.sp2` is identical across its panes.
func TestMobileProbe_TheLedgerShowsThreeRowsOnAPhone(t *testing.T) {
	chrome := mobileNeedChromium(t)
	page := mobileInnerPage(t, benchFxShotData(DayCardMount{
		CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}), "")
	inner := mobileWriteInner(t, "ledger.html", page)

	for _, w := range mobileWidths {
		h := mobileHeightFor(w)
		mobileHeader(t, "[MOB-1] the Ledger", w, h)
		r := mobileDrive(t, chrome, inner, w, h, []mobileStep{
			{Op: "zones"}, {Op: "lrowsAll"}, {Op: "blockHeight"},
			{Op: "selectDay"}, {Op: "shelfPanes"},
		})
		zones, lall, before, sel, panes := r[0], r[1], r[2], r[3], r[4]

		if int(zones.num("innerWidth")) != w {
			t.Fatalf("asked for a %dpx viewport and measured in %.0fpx — the frame did not "+
				"take and every number below is about a viewport nobody asked for",
				w, zones.num("innerWidth"))
		}
		lrows := mobileBox(zones, "lrows")
		body := mobileBox(zones, "body")
		block := mobileBox(zones, "block")
		t.Logf("   .block %.0fx%.0f (max-height %s) · .np %.0f · .body %.0f (client %.0f / scroll %.0f) · .inst %.0f · .shelf %.0f",
			block["w"], block["h"], zones.str("blockMaxH"), mobileBox(zones, "np")["h"],
			body["h"], body["ch"], body["sh"], mobileBox(zones, "inst")["h"],
			mobileBox(zones, "shelf")["h"])
		t.Logf("   .lrows client %.0f / scroll %.0f (min-height %s) · rows %.0f · FULLY INSIDE %.0f · clipped %.0f · .lrow height %.0f",
			lrows["ch"], lrows["sh"], zones.str("lrowsMinH"), zones.num("rows"),
			zones.num("rowsInside"), zones.num("rowsClipped"), zones.num("lrowH"))

		if got := int(zones.num("rowsInside")); got < 3 {
			t.Errorf("%dpx: %d Ledger row(s) fully inside the scroll window, ruled >= 3 "+
				"(client %.0f / scroll %.0f over %.0f rows). [MOB-1]: this is the list of "+
				"WHAT IS HAPPENING and a GM is holding this phone at a table",
				w, got, lrows["ch"], lrows["sh"], zones.num("rows"))
		}
		if lrows["ch"] < float64(mobileLedgerFloorPx) {
			t.Errorf("%dpx: .lrows clientHeight %.0f, ruled >= %d (= 3 x %d, the sheet's own "+
				".lrow height). Baseline was 41.", w, lrows["ch"], mobileLedgerFloorPx,
				mobileLedgerFloorPx/3)
		}
		// The Block stops scrolling INTERNALLY; the page scrolls instead.
		if body["sh"]-body["ch"] > 1 {
			t.Errorf("%dpx: .body still scrolls internally — client %.0f vs scroll %.0f. "+
				"[MOB-1]/[MOB-4]: on a phone the PAGE is the scroller",
				w, body["ch"], body["sh"])
		}
		// EVERY Ledger on the page, not just the first: the audit's second probe
		// caught the real-world Block at 46/82 while the first read 41/220.
		list, _ := lall["list"].([]any)
		for i, raw := range list {
			m, _ := raw.(map[string]any)
			t.Logf("   .lrows#%d client %.0f / scroll %.0f · rows %.0f · fully inside %.0f · min-height %v · overscroll %v",
				i, mobileF(m, "ch"), mobileF(m, "sh"), mobileF(m, "rows"), mobileF(m, "inside"),
				m["minH"], m["contain"])
			if mobileF(m, "rows") > 0 && mobileF(m, "inside") < 3 && mobileF(m, "rows") >= 3 {
				t.Errorf("%dpx: Ledger #%d shows %.0f of %.0f rows fully — [MOB-1] rules >= 3",
					w, i, mobileF(m, "inside"), mobileF(m, "rows"))
			}
		}

		// ── the declared-height invariance, re-bought rather than dropped ──
		hBefore := mobileBox(before, "block")["h"]
		hAfter := mobileBox(sel, "block")["h"]
		t.Logf("   declared-height invariance: no day selected %.0f · day selected %.0f (delta %.0f)",
			hBefore, hAfter, hAfter-hBefore)
		if hAfter != hBefore {
			t.Errorf("%dpx: the Block is %.0f tall with no day selected and %.0f with one — "+
				"calendar-block.css §SIZE CLASS: \"Declared heights are INVARIANT so dropping "+
				"a Block into any host page can never shove that page around\". The cap used "+
				"to buy this; [MOB-1] re-buys it with the .lrows floor and it must hold at 0px",
				w, hBefore, hAfter)
		}

		// THE SUBJECT OF THIS ROW IS THE ZONE, NOT THE PANE, AND THE SWAP IS
		// DELIBERATE. The acceptance row names `.sp2`, but a 132px floor on
		// `.sp2` (or on `.spane`) makes the Almanac pane measure exactly 132
		// of 132 and TestProbe_ShelfGeometryIsInvariant then says — correctly
		// — that the scroller has stopped scrolling. That guard is not
		// weakened to admit this slice. What the acceptance row is FOR is
		// stated in its own second sentence, "switching tabs must not move the
		// host page", and the box the host page feels is `.shelf`. Both
		// numbers are reported at every width so the substitution is visible
		// rather than silent.
		pl, _ := panes["panes"].([]any)
		var first, firstBlock float64 = -1, -1
		for _, raw := range pl {
			m, _ := raw.(map[string]any)
			hh, bh := mobileF(m, "h"), mobileF(m, "block")
			t.Logf("   shelf pane %q — .shelf %.0f · .sp2 %.0f (scroll %.0f) · .block %.0f",
				m["label"], hh, mobileF(m, "pane"), mobileF(m, "paneScroll"), bh)
			if first < 0 {
				first, firstBlock = hh, bh
				continue
			}
			if hh != first {
				t.Errorf("%dpx: switching to the %q Shelf pane moves .shelf from %.0f to %.0f — "+
					"switching a tab must not shove the host page", w, m["label"], first, hh)
			}
			if bh != firstBlock {
				t.Errorf("%dpx: switching to the %q Shelf pane moves the BLOCK from %.0f to %.0f",
					w, m["label"], firstBlock, bh)
			}
		}
	}

	// ── THE DESKTOP CONTROL. The FULL tier is byte-unchanged and this is the
	//    row that proves the container query did not leak upward.
	mobileHeader(t, "[MOB-1] CONTROL · desktop", 1280, 900)
	r := mobileDrive(t, chrome, inner, 1280, 900, []mobileStep{{Op: "zones"}})
	z := r[0]
	lr := mobileBox(z, "lrows")
	t.Logf("   CONTROL 1280x900: .lrows client %.0f / scroll %.0f · rows %.0f · fully inside %.0f · min-height %s · .block max-height %s",
		lr["ch"], lr["sh"], z.num("rows"), z.num("rowsInside"), z.str("lrowsMinH"), z.str("blockMaxH"))
	if z.num("rowsInside") < 3 {
		t.Errorf("the desktop control regressed: %.0f of %.0f rows fully visible at 1280",
			z.num("rowsInside"), z.num("rows"))
	}
}

// mobileHeightFor pairs each phone width with the screen height it actually
// ships on, so a "390" arm is a real 390x664 and not a 390-wide desktop.
func mobileHeightFor(w int) int {
	switch w {
	case 375:
		return 667
	case 360:
		return 640
	default:
		return 664
	}
}

// mobileLedgerFloorPx is [MOB-1]'s floor, DERIVED not invented: three of the
// sheet's own `.cal-block-host .lrow { height: 44px }`. The stylesheet writes
// the same arithmetic in a comment rather than a bare literal.
const mobileLedgerFloorPx = 3 * 44

func mobileBox(r mobileReply, k string) map[string]float64 {
	out := map[string]float64{}
	m, _ := r[k].(map[string]any)
	for _, f := range []string{"top", "bottom", "left", "right", "w", "h", "ch", "sh", "cw", "sw"} {
		out[f] = mobileF(m, f)
	}
	return out
}

func mobileF(m map[string]any, k string) float64 {
	if m == nil {
		return 0
	}
	if v, ok := m[k].(float64); ok {
		return v
	}
	return 0
}

// ── [MOB-2] + [MOB-9] — the sheet's geometry, and the morph that must not
//    re-trigger ─────────────────────────────────────────────────────────────
//
// BASELINE, measured: at 390x664 the open editor carried an inline
// `top: 106px; width: 390px` written once at open time. Shrink the viewport to
// 390x380 with a real resize — what a software keyboard does — and the rect
// was UNCHANGED at y[106..464] with Save at y[426..456] against a 380px
// viewport. 84px of the sheet, including the entire footer, below the fold of
// a `position: fixed` box that does not scroll with the page. Rotating to
// 844x390 left `width: 390px` on an 844px viewport.
//
// TARGET, ruled: no inline `top`, no inline `width`, and `[data-de-save]`
// entirely inside the viewport after BOTH a keyboard-shaped shrink and a
// rotation, at all three widths.
//
// [MOB-9] rides the same run because it is [MOB-2]'s only motion consequence:
// `.edmorph` transitions inline-size / block-size / translate / opacity, and
// handing the sheet's geometry to CSS means the browser re-resolves it on
// every viewport change. If `.edmorph` were still on the element when a
// keyboard opened, the reposition would ANIMATE — a 200ms slide while the user
// is trying to type. The sheet already claims the invariant in a comment
// ("`.edmorph` IS PRESENT ONLY WHILE THE MORPH IS IN FLIGHT"); this turns the
// claim into two numbers.
func TestMobileProbe_TheSheetStaysReachableWhenTheViewportShrinks(t *testing.T) {
	chrome := mobileNeedChromium(t)
	page := mobileInnerPage(t, benchFxShotData(DayCardMount{
		CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}), "")
	inner := mobileWriteInner(t, "sheet.html", page)

	for _, arm := range []struct{ w, h, shrinkH, rotW, rotH int }{
		{390, 664, 380, 844, 390},
		{375, 667, 360, 812, 375},
		{360, 640, 340, 800, 360},
	} {
		mobileHeader(t, "[MOB-2] the sheet under a keyboard and a rotation", arm.w, arm.h)
		r := mobileDrive(t, chrome, inner, arm.w, arm.h, []mobileStep{
			{Op: "openCard"},
			{Op: "openEditor", Delay: 400},
			{Op: "sheet", Delay: 400},
			{Op: "sheet", Delay: 300, Outer: fmt.Sprintf("size(%d,%d)", arm.w, arm.shrinkH)},
			{Op: "sheet", Delay: 300, Outer: fmt.Sprintf("size(%d,%d)", arm.rotW, arm.rotH)},
		})
		open, shrunk, rotated := r[2], r[3], r[4]

		if !open.boolean("open") {
			t.Fatalf("%dpx: the editor was NOT open when the probe measured — every number "+
				"below would be a number about nothing", arm.w)
		}
		for _, s := range []struct {
			label string
			vh    int
			rep   mobileReply
		}{
			{"OPEN", arm.h, open},
			{fmt.Sprintf("KEYBOARD (%dx%d)", arm.w, arm.shrinkH), arm.shrinkH, shrunk},
			{fmt.Sprintf("ROTATED (%dx%d)", arm.rotW, arm.rotH), arm.rotH, rotated},
		} {
			root, save := mobileBox(s.rep, "root"), mobileBox(s.rep, "save")
			t.Logf("   %s — viewport %.0fx%.0f · sheet %.0fx%.0f at y[%.0f..%.0f] · Save y[%.0f..%.0f] · inline top %q width %q · .dcsheet %v · .edmorph %v · data-dc-clear %q",
				s.label, s.rep.num("innerWidth"), s.rep.num("innerHeight"),
				root["w"], root["h"], root["top"], root["bottom"],
				save["top"], save["bottom"], s.rep.str("inlineTop"), s.rep.str("inlineWidth"),
				s.rep.boolean("dcsheet"), s.rep.boolean("edmorph"), s.rep.str("clear"))

			if s.rep.str("inlineTop") != "" {
				t.Errorf("%s: the sheet still carries an inline top of %q — [MOB-2] hands the "+
					"geometry to CSS precisely so a stale pixel cannot survive a viewport change",
					s.label, s.rep.str("inlineTop"))
			}
			if s.rep.str("inlineWidth") != "" {
				t.Errorf("%s: the sheet still carries an inline width of %q", s.label,
					s.rep.str("inlineWidth"))
			}
			if !s.rep.boolean("saveInside") {
				t.Errorf("%s: Save's rect is y[%.0f..%.0f] against a %.0fpx viewport — it is "+
					"NOT reachable, and a position:fixed box cannot be scrolled to. This is "+
					"the finding: a GM taps Save at a table and there is no Save",
					s.label, save["top"], save["bottom"], s.rep.num("innerHeight"))
			}
			// [MOB-9], both assertions, at every viewport state.
			if s.rep.boolean("edmorph") {
				t.Errorf("%s: `.edmorph` is STILL ON the settled open editor. It transitions "+
					"inline-size / block-size / translate / opacity, so the reposition a "+
					"keyboard causes would animate — a 200ms slide while the user is typing",
					s.label)
			}
			if d := s.rep.str("tdur"); d != "" && !mobileAllZeroDurations(d) {
				t.Errorf("%s: transition-duration on the settled editor resolves to %q over "+
					"%q, not 0s — [MOB-9] rules the resting editor carries no transition at all",
					s.label, d, s.rep.str("tprop"))
			}
		}
		// The sheet is the WHOLE width after a rotation — the stale-width hat of
		// the same bug (audit finding 40).
		rw := mobileBox(rotated, "root")["w"]
		if int(rw) != arm.rotW {
			t.Errorf("rotated to %dx%d: the sheet renders %.0f wide, not %d — the stale inline "+
				"width survived the rotation", arm.rotW, arm.rotH, rw, arm.rotW)
		}
		// [DC-3]'s honesty channel still describes the box that renders.
		ro := mobileBox(open, "root")
		t.Logf("   [DC-3] the reported rect vs the RENDERED box at %dx%d: data-dc-clear=%q, "+
			"rendered y[%.0f..%.0f] x[%.0f..%.0f]", arm.w, arm.h, open.str("clear"),
			ro["top"], ro["bottom"], ro["left"], ro["right"])
		if open.str("clear") == "" {
			t.Errorf("%dpx: data-dc-clear is absent — [DC-3] SIGNED makes the occlusion "+
				"report a build-time flag and retiring it along with the pixel is how a "+
				"signature gets un-signed quietly", arm.w)
		}
		if int(ro["bottom"]) != arm.h {
			t.Errorf("%dpx: the sheet's bottom edge is %.0f against a %d viewport — a bottom "+
				"sheet that is not flush to the bottom is not the shape the sheet declares",
				arm.w, ro["bottom"], arm.h)
		}
		if mx := open.str("edbodyMax"); !strings.Contains(mx, "px") {
			t.Errorf("%dpx: .ed-body's max-block-size resolves to %q", arm.w, mx)
		} else {
			t.Logf("   .ed-body max-block-size resolves to %s (declared min(70dvh, 620px); "+
				"NOTE: in a NESTED BROWSING CONTEXT dvh equals the frame's own box, so this "+
				"number is the frame's 70%%, not a phone's retracting-chrome 70%%)", mx)
		}
	}

	// ── THE DAY CARD'S CONTROL, WHICH IS WHAT MADE THIS A DEFECT RATHER THAN
	//    A LIMITATION. In the audit's own run the card moved 568 -> 285 under
	//    the same shrink while the editor did not move at all. Under [MOB-2]
	//    the card does not need the module's resize listener to do it: the CSS
	//    keeps it flush to the bottom edge by construction.
	mobileHeader(t, "[MOB-2] CONTROL · the day card under the same shrink", 390, 664)
	r := mobileDrive(t, chrome, inner, 390, 664, []mobileStep{
		{Op: "openCard"},
		{Op: "card", Delay: 400},
		{Op: "card", Delay: 300, Outer: "size(390,380)"},
	})
	before, after := mobileBox(r[1], "root"), mobileBox(r[2], "root")
	t.Logf("   card at 664: top %.0f bottom %.0f · at 380: top %.0f bottom %.0f · inline top %q / %q",
		before["top"], before["bottom"], after["top"], after["bottom"],
		r[1].str("inlineTop"), r[2].str("inlineTop"))
	if int(after["bottom"]) != 380 {
		t.Errorf("the day card's bottom edge is %.0f against a 380px viewport — the control "+
			"that used to work has stopped working", after["bottom"])
	}
	if after["top"] >= before["top"] {
		t.Errorf("the day card did not move up under the shrink: %.0f -> %.0f",
			before["top"], after["top"])
	}
}

// mobileAllZeroDurations reports whether every comma-separated duration in a
// computed `transition-duration` is zero. A resting editor must carry none.
func mobileAllZeroDurations(d string) bool {
	for _, part := range strings.Split(d, ",") {
		p := strings.TrimSpace(part)
		if p != "0s" && p != "0ms" && p != "" {
			return false
		}
	}
	return true
}

// ── [MOB-3] — the lock, and the release ruled harder than the lock ────────
//
// BASELINE, measured with the day card open AND with the editor open, at
// 390x664, 360x640 and 390x844 — six arms, six identical results:
//
//	window.scrollBy(0, 400)  ->  document.scrollingElement.scrollTop  0 -> 400
//	computed overflow on <html> and on <body>: "visible"
//
// Because the sheet is position:fixed it stays pinned while the calendar
// behind it scrolls away, so the card can end up describing a day that is no
// longer on screen.
//
// THE RELEASE IS THE HALF THAT MATTERS MORE, and it is proven on every exit
// path this surface has, including the pathological one: open the card, open
// the editor from it (the card closes AS the editor opens), close the editor.
// A page left locked is a phone on a page that will not scroll, with no
// visible cause and no way out but a reload — which is strictly worse than the
// defect being fixed.
func TestMobileProbe_ThePageIsLockedBehindASheetAndReleasedOnEveryExit(t *testing.T) {
	chrome := mobileNeedChromium(t)
	page := mobileInnerPage(t, benchFxShotData(DayCardMount{
		CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}), "")
	inner := mobileWriteInner(t, "lock.html", page)

	// ── the lock, six arms: card and editor, at three widths ──────────────
	for _, w := range mobileWidths {
		h := mobileHeightFor(w)
		mobileHeader(t, "[MOB-3] the lock", w, h)

		card := mobileDrive(t, chrome, inner, w, h, []mobileStep{
			{Op: "preScroll"}, {Op: "openCard", Delay: 200}, {Op: "push", Delay: 300},
		})
		ed := mobileDrive(t, chrome, inner, w, h, []mobileStep{
			{Op: "preScroll"}, {Op: "openCard", Delay: 200},
			{Op: "openEditor", Delay: 400}, {Op: "push", Delay: 400},
		})
		for _, arm := range []struct {
			label string
			pre   mobileReply
			push  mobileReply
		}{
			{"day card open", card[0], card[2]},
			{"editor open", ed[0], ed[3]},
		} {
			t.Logf("   %s — pre-open scrollTop %.0f · after scrollBy(0,400) scrollTop %.0f · body position %q top %q · html overflow %q",
				arm.label, arm.pre.num("top"), arm.push.num("top"),
				arm.push.str("bodyPos"), arm.push.str("bodyTop"), arm.push.str("htmlOverflow"))
			// THE RULED NUMBER IS ZERO, and that is the position:fixed form
			// rather than a second opinion about it: with <body> taken out of
			// flow the document has nothing left to scroll, so
			// `scrollingElement.scrollTop` reads 0 and the page's real position
			// is carried by `body { top: -Npx }`. The baseline here was 400.
			if arm.push.num("top") != 0 {
				t.Errorf("%dpx, %s: scrollBy(0,400) left scrollTop at %.0f, ruled 0 — the day "+
					"the sheet describes slides out from under it", w, arm.label,
					arm.push.num("top"))
			}
			if want := fmt.Sprintf("%.0fpx", -arm.pre.num("top")); arm.push.str("bodyTop") != want {
				t.Errorf("%dpx, %s: body top is %q, want %q — the lock must hold the page "+
					"WHERE IT WAS, not snap it to the top", w, arm.label,
					arm.push.str("bodyTop"), want)
			}
			if arm.push.str("bodyPos") != "fixed" {
				t.Errorf("%dpx, %s: body position is %q, not fixed — [MOB-3] rules the "+
					"position:fixed form because `overflow:hidden` on <html> is only safe if "+
					"iOS 16 support is not needed", w, arm.label, arm.push.str("bodyPos"))
			}
		}
	}

	// ── THE RELEASE, ON EVERY EXIT PATH ───────────────────────────────────
	mobileHeader(t, "[MOB-3] the release, five exit paths plus the pathological arm", 390, 664)
	for _, exit := range []struct {
		label string
		open  []mobileStep
		close string
	}{
		{"Escape", nil, "closeEscape"},
		{"the head ✕", nil, "closeX"},
		{"Cancel", nil, "closeCancel"},
		// SAVE'S TEARDOWN IS DRIVEN AS A UA-INITIATED CLOSE, AND THAT IS THE
		// POINT OF THE ROW RATHER THAN A SHORTCUT AROUND IT. A file:// page has
		// no server, so a real Save cannot complete; `hidePopover()` is exactly
		// the shape of close the module's own functions never see, which is why
		// [MOB-3] rules the release onto the `toggle` event. If the lock lifts
		// here it lifts on the real Save too, because the real Save ends in the
		// same hide.
		{"Save (UA teardown)", nil, "closeSave"},
	} {
		steps := []mobileStep{
			{Op: "preScroll"},
			{Op: "openCard", Delay: 200},
			{Op: "openEditor", Delay: 400},
			{Op: "push", Delay: 400},
			{Op: exit.close, Delay: 200},
			{Op: "after", Delay: 400},
		}
		r := mobileDrive(t, chrome, inner, 390, 664, steps)
		pre, locked, after := r[0], r[3], r[5]
		t.Logf("   exit %-16s — locked scrollTop %.0f (body %q) · after close scrollTop %.0f (body %q) · pre-open was %.0f · editor still open %v",
			exit.label, locked.num("top"), locked.str("bodyPos"), after.num("top"),
			after.str("bodyPos"), pre.num("top"), after.boolean("edOpen"))
		if after.str("bodyPos") == "fixed" {
			t.Errorf("exit %q: the page is STILL LOCKED after the sheet closed (body position "+
				"%q). A lock never released leaves a phone on a page that will not scroll, "+
				"with no visible cause and no way out but a reload — [MOB-3] rules this the "+
				"worse bug", exit.label, after.str("bodyPos"))
		}
		if d := after.num("top") - pre.num("top"); d > 1 || d < -1 {
			t.Errorf("exit %q: the scroll position came back as %.0f against a pre-open %.0f — "+
				"the stored offset is the whole reason the position:fixed form is affordable",
				exit.label, after.num("top"), pre.num("top"))
		}
	}

	// THE FIFTH PATH IS THE CARD'S, NOT THE EDITOR'S, AND THE PROBE SAYS SO
	// RATHER THAN PRETENDING OTHERWISE. Measured from the module: the document
	// click handler dismisses the CARD when the click lands outside
	// `[data-cal-daycard]`, and the editor — popover="manual" with explicit
	// controls — has no outside-click dismissal at all. Asserting one on the
	// editor would be asserting a path that does not exist.
	oc := mobileDrive(t, chrome, inner, 390, 664, []mobileStep{
		{Op: "preScroll"},
		{Op: "openCard", Delay: 200},
		{Op: "push", Delay: 300},
		{Op: "closeOutside", Delay: 200},
		{Op: "after", Delay: 400},
	})
	t.Logf("   exit %-16s — locked scrollTop %.0f (body %q) · after close scrollTop %.0f (body %q) · pre-open was %.0f · card still open %v",
		"outside click (card)", oc[2].num("top"), oc[2].str("bodyPos"),
		oc[4].num("top"), oc[4].str("bodyPos"), oc[0].num("top"), oc[4].boolean("cardOpen"))
	if oc[4].str("bodyPos") == "fixed" {
		t.Errorf("exit outside-click: the page is STILL LOCKED after the card closed (body "+
			"position %q)", oc[4].str("bodyPos"))
	}
	if d := oc[4].num("top") - oc[0].num("top"); d > 1 || d < -1 {
		t.Errorf("exit outside-click: the scroll came back as %.0f against a pre-open %.0f",
			oc[4].num("top"), oc[0].num("top"))
	}

	// THE PATHOLOGICAL ARM. Card -> editor (the card closes as the editor
	// opens) -> close. Exactly one release, and the page comes back. A
	// reference count that went negative is what this row exists to catch.
	r := mobileDrive(t, chrome, inner, 390, 664, []mobileStep{
		{Op: "preScroll"},
		{Op: "openCard", Delay: 200},
		{Op: "push", Delay: 200},
		{Op: "openEditor", Delay: 400},
		{Op: "push", Delay: 300},
		{Op: "closeX", Delay: 200},
		{Op: "after", Delay: 400},
		{Op: "push", Delay: 200},
	})
	pre, cardLocked, edLocked, after, freeAgain := r[0], r[2], r[4], r[6], r[7]
	t.Logf("   PATHOLOGICAL card→editor→close — pre %.0f · card-locked %.0f (%q) · editor-locked %.0f (%q) · after %.0f (%q) · scrolls again to %.0f",
		pre.num("top"), cardLocked.num("top"), cardLocked.str("bodyPos"),
		edLocked.num("top"), edLocked.str("bodyPos"), after.num("top"),
		after.str("bodyPos"), freeAgain.num("top"))
	if cardLocked.str("bodyPos") != "fixed" || edLocked.str("bodyPos") != "fixed" {
		t.Errorf("the handover dropped the lock mid-flight: card %q then editor %q",
			cardLocked.str("bodyPos"), edLocked.str("bodyPos"))
	}
	if after.str("bodyPos") == "fixed" {
		t.Error("card→editor→close left the page locked — the handover released once too few")
	}
	if d := after.num("top") - pre.num("top"); d > 1 || d < -1 {
		t.Errorf("card→editor→close restored the scroll to %.0f against a pre-open %.0f",
			after.num("top"), pre.num("top"))
	}
	if freeAgain.num("top") <= after.num("top") {
		t.Errorf("the page does not scroll again after the sheets closed: scrollBy(0,400) "+
			"left it at %.0f", freeAgain.num("top"))
	}
}

// mobileLongWeekData builds the same Bench with a week of `n` weekdays. The
// production fixture is a TENDAY, which §CORRECTIONS proved is CLEAN at every
// phone width — the anecdote "a tenday loses days on a narrow phone" is
// refuted by arithmetic (300px of grid floor against a 388px body at 390 and
// 358 at 360). The real threshold is week-len >= 13 at 390 and >= 12 at 360,
// so the long arm is measured at 20 — the length the audit's own probe lost
// four day cells at.
func mobileLongWeekData(t *testing.T, n int) BenchData {
	t.Helper()
	data := benchFxData(true, true)
	cal := benchFxTypedCalendar()
	cal.Weekdays = nil
	for i := 0; i < n; i++ {
		cal.Weekdays = append(cal.Weekdays, Weekday{Name: fmt.Sprintf("W%02d", i+1)})
	}
	d := projectBlock(BlockProjectionInput{
		Calendar: &cal, Events: benchFxShotEvents(),
		Viewer:     BlockViewer{UserID: "u-1", Role: 3},
		MonthIndex: cal.CurrentMonth - 1, Year: cal.CurrentYear,
		MoonCap:    benchMoonCap,
	})
	d.Layers = benchBlockLayers(blockLayerPrefs{})
	data.Primary = &BenchBlock{Data: d, Manage: benchManage(&cal, "cal-harptos", "camp-1")}
	return data
}

// ── [MOB-4] + [MOB-8] — the scroller census, the containment, and the one
//    horizontal scroller this slice adds ───────────────────────────────────
//
// BASELINE, measured at 390x664 inside a document of scrollHeight 2251:
//
//	.cal-block-host .lrows          41 / 220   y 732–773
//	.cal-block-host .shelf .sp2     75 / 222   y 808–883
//	a second .lrows                 46 /  82   y 1401–1447
//	(+ .cal-dayeditor .ed-body 465 / 1229 once the editor opens)
//
// and `overscroll-behavior` returned ZERO hits across all four calendar
// stylesheets, as did `touch-action`. A vertical swipe on the middle third of
// the screen therefore did something different depending on which 40-pixel
// band the finger landed in.
//
// DECLARING CONTAINMENT ON FIVE NESTED SCROLLERS IS A PATCH; REDUCING FIVE TO
// TWO IS THE FIX, AND BOTH SHIP. [MOB-1] already paid for most of the census:
// with the desktop ration lifted, `.body` and the Ledger stop being scroll
// windows carved out of 520 pixels.
func TestMobileProbe_TheScrollerCensusAndTheLongWeek(t *testing.T) {
	chrome := mobileNeedChromium(t)
	mount := DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}
	inner := mobileWriteInner(t, "census.html", mobileInnerPage(t, benchFxShotData(mount), ""))

	// ── (a) THE CENSUS ────────────────────────────────────────────────────
	mobileHeader(t, "[MOB-4] the scroller census, Bench idle", 390, 664)
	r := mobileDrive(t, chrome, inner, 390, 664, []mobileStep{{Op: "census"}})
	list, _ := r[0]["bench"].([]any)
	t.Logf("   %d scrolling region(s) inside .cal-bench:", len(list))
	for _, raw := range list {
		m, _ := raw.(map[string]any)
		t.Logf("      %v  axis %v  client %.0f / scroll %.0f  overscroll-behavior %v",
			m["sel"], m["axis"], mobileF(m, "c"), mobileF(m, "s"), m["contain"])
		if m["contain"] != "contain" {
			t.Errorf("the scroller %v declares overscroll-behavior-%v %q — a region that "+
				"chains its swipe to the page is one of the bands [MOB-4] exists to remove",
				m["sel"], m["axis"], m["contain"])
		}
	}
	if len(list) > 2 {
		t.Errorf("%d scrolling regions inside .cal-bench at 390, ruled <= 2 (baseline 3). "+
			"On a phone the PAGE is the scroller; a region earns its own only by being a "+
			"list genuinely longer than the screen", len(list))
	}

	// ── (b) THE CONTAINMENT, DECLARED IN ALL FOUR SHEETS ──────────────────
	for _, sheet := range []struct{ file, name string }{
		{"calendar-block.css", "the Block"},
		{"calendar-daycard.css", "the card and the editor"},
		{"calendar-bench.css", "the Bench"},
		{"calendar-schedule.css", "/schedule"},
	} {
		code := readRepoFile(t, filepath.Join("static", "css", sheet.file))
		n := strings.Count(code, "overscroll-behavior")
		t.Logf("   %s — `overscroll-behavior` declarations in %s: %d (baseline 0)",
			sheet.name, sheet.file, n)
		if n == 0 {
			t.Errorf("%s declares no overscroll-behavior at all — [MOB-4](b) rules the count "+
				"non-zero in all four sheets", sheet.file)
		}
		// The DECLARATION form, so a comment naming the refused property (and
		// this slice writes one, on purpose) is not mistaken for shipping it.
		if strings.Contains(code, "touch-action:") {
			t.Errorf("%s declares `touch-action` — [MOB-9b] REFUSES that property by name: on "+
				"the month grid it claims the gesture and kills page scrolling over the "+
				"calendar, which on a phone is a worse trade than losing drag-create",
				sheet.file)
		}
	}

	// ── [MOB-8] THE LONG WEEK ─────────────────────────────────────────────
	long := mobileWriteInner(t, "week20.html", mobileInnerPage(t, mobileLongWeekData(t, 20), ""))
	ten := mobileWriteInner(t, "week10.html", mobileInnerPage(t, mobileLongWeekData(t, 10), ""))
	for _, w := range []int{390, 375, 360} {
		mobileHeader(t, "[MOB-8] week-len 20", w, mobileHeightFor(w))
		lr := mobileDrive(t, chrome, long, w, mobileHeightFor(w), []mobileStep{{Op: "census"}})[0]
		inst, grid := mobileBox(lr, "inst"), mobileBox(lr, "grid")
		hd, _ := lr["headers"].(map[string]any)
		t.Logf("   week-len %v — .inst client %.0f / scroll %.0f (overflow-x %s, overflow-y %s, overscroll-x %s) · .grid %.0f · %v weekday headers, last right edge %.0f · zero-width cells %.0f",
			lr["weekLen"], inst["cw"], inst["sw"], lr.str("instOX"), lr.str("instOY"),
			lr.str("instContainX"), grid["w"], mobileF(hd, "count"),
			mobileF(hd, "lastRight"), mobileF(hd, "zeroCells"))

		// REACHABILITY IS THE ASSERTION, NOT OVERFLOW. A box whose content is
		// wider than itself is only a defect when the content cannot be
		// reached, and `overflow: hidden` — the shipped state — is exactly the
		// case where it cannot: the pixels exist in scrollWidth and no gesture
		// arrives at them. So the two facts are asserted TOGETHER.
		overflows := inst["sw"] > inst["cw"]+1
		reachable := lr.str("instOX") == "auto" || lr.str("instOX") == "scroll"
		if !overflows {
			t.Errorf("%dpx, week-len 20: .inst reports scrollWidth %.0f against clientWidth "+
				"%.0f — a 20-day week needs 600px of grid, so this measurement is not of the "+
				"case the ruling is about", w, inst["sw"], inst["cw"])
		}
		if overflows && !reachable {
			t.Errorf("%dpx, week-len 20: %.0fpx of grid inside a %.0fpx box whose overflow-x "+
				"is %q — the days past the edge exist in scrollWidth and NO GESTURE REACHES "+
				"THEM. Days a GM cannot reach are DATA LOSS, and data loss outranks gesture "+
				"friction", w, inst["sw"], inst["cw"], lr.str("instOX"))
		}
		if lr.str("instContainX") != "contain" {
			t.Errorf("%dpx: the deliberate horizontal scroller declares overscroll-behavior-x "+
				"%q, ruled `contain`", w, lr.str("instContainX"))
		}
		// The block axis must NOT have become a scroller as a side effect: CSS
		// resolves a visible/non-visible overflow pair to `auto`.
		if inst["sh"]-inst["ch"] > 1 {
			t.Errorf("%dpx: .inst also scrolls VERTICALLY (%.0f in %.0f) — the horizontal "+
				"scroller has grown a second axis nobody asked for", w, inst["sh"], inst["ch"])
		}
		if z := mobileF(hd, "zeroCells"); z > 0 {
			t.Errorf("%dpx, week-len 20: %.0f day cell(s) measure zero-width", w, z)
		}

		// A TENDAY GROWS NO SCROLLER IT DOES NOT NEED.
		tr := mobileDrive(t, chrome, ten, w, mobileHeightFor(w), []mobileStep{{Op: "census"}})[0]
		ti := mobileBox(tr, "inst")
		t.Logf("   week-len %v (a tenday) — .inst client %.0f / scroll %.0f · .grid %.0f",
			tr["weekLen"], ti["cw"], ti["sw"], mobileBox(tr, "grid")["w"])
		if ti["sw"] > ti["cw"]+1 {
			t.Errorf("%dpx: a TENDAY grew a horizontal scroller (%.0f in %.0f). A tenday's "+
				"grid floor is 300px against a body of 388 at 390 and 358 at 360 — it fits, "+
				"and a surface that sprouts a drag it does not need is its own defect",
				w, ti["sw"], ti["cw"])
		}
	}

	// ── AND THE PAGE FOLD SURVIVES IT ─────────────────────────────────────
	//
	// The one thing this slice could most easily break: at 360, 375, 390 and
	// 414 the page has never dragged sideways in any measured state, and
	// [MOB-8] is the only ruling that adds a horizontal scroller.
	mobileHeader(t, "[MOB-8] the page fold, every width x every state", 0, 0)
	for _, w := range []int{360, 375, 390, 414} {
		h := mobileHeightFor(w)
		if w == 414 {
			h = 736
		}
		for _, st := range []struct {
			label string
			steps []mobileStep
			pick  int
		}{
			{"Bench idle", []mobileStep{{Op: "noop"}}, 0},
			{"day card open", []mobileStep{{Op: "openCard"}, {Op: "noop", Delay: 300}}, 1},
			{"editor open", []mobileStep{{Op: "openCard"}, {Op: "openEditor", Delay: 400},
				{Op: "noop", Delay: 400}}, 2},
		} {
			for _, src := range []struct{ name, path string }{
				{"tenday", ten}, {"week-len 20", long},
			} {
				rr := mobileDrive(t, chrome, src.path, w, h, st.steps)[st.pick]
				pg, _ := rr["page"].(map[string]any)
				sw, cw := mobileF(pg, "sw"), mobileF(pg, "cw")
				t.Logf("   %dx%d · %-14s · %-11s — document scrollWidth %.0f vs clientWidth %.0f",
					w, h, st.label, src.name, sw, cw)
				if sw != cw {
					t.Errorf("%dpx, %s, %s: the DOCUMENT drags sideways (scrollWidth %.0f vs "+
						"clientWidth %.0f). [MOB-8]'s scroller is bounded and this row is what "+
						"catches it going wrong", w, st.label, src.name, sw, cw)
				}
			}
		}
	}
}

// mobileSchedulePage builds /schedule's real surface inside the app shell's
// own phone padding, plus the ops listener. It is the same head+body pair the
// fidelity harness renders, so this is a measurement of the product and not of
// a harness.
func mobileSchedulePage(t *testing.T, isGM bool) string {
	t.Helper()
	data := scheduleShotData(isGM, "week")
	var head, body strings.Builder
	if err := scheduleHead(data).Render(context.Background(), &head); err != nil {
		t.Fatalf("render schedule head: %v", err)
	}
	if err := scheduleBody(data).Render(context.Background(), &body); err != nil {
		t.Fatalf("render schedule body: %v", err)
	}
	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0;background:#f9fafb;` +
		`font-family:ui-sans-serif,system-ui,-apple-system,sans-serif}` +
		fmt.Sprintf(`.shotwrap{padding:%dpx}`, schedulePagePadNarrow) +
		fmt.Sprintf(`@media (min-width:%dpx){.shotwrap{padding:%dpx}}`,
			schedulePagePadBreak, schedulePagePadWide) +
		scheduleCSS(t, "calendar-bench.css") + scheduleCSS(t, "calendar-schedule.css") +
		`</style></head><body>` +
		`<div class="cal-bench cal-schedule shotwrap">` + head.String() + body.String() + `</div>` +
		`<script>` + mobileScheduleOpsScript + `</script>` +
		`</body></html>`
}

// ── [MOB-5] + [MOB-6] — the operator's own gate panel, and the page a player
//    uses to say when they are free ─────────────────────────────────────────
//
// [MOB-5] BASELINE, measured on the player arm AND the GM arm with the repo's
// own RSVP fixture: `.cal-bench .rsvp .ovwrap` client 362 against scroll 520 at
// 390 — 158px of sideways drag — and client 332 against 520 at 360, 188px. The
// document's own scrollWidth never moved and nothing was clipped away, so this
// is a GESTURE TRAP rather than data loss: a 7-day availability grid you must
// drag sideways inside a page that scrolls vertically, on the panel the
// operator's go/no-go gate names.
//
// [MOB-6] BASELINE, four widths: `.sc-wrap` overflow-x `hidden` with 370/370 at
// 414 and 346/346 at 390, but `auto` with 331/338 at 375 and 316/338 at 360 —
// 7px and 22px, each exactly `338 - budget`.
func TestMobileProbe_TheRSVPOverviewAndTheScheduleFitThePhone(t *testing.T) {
	chrome := mobileNeedChromium(t)

	// ── [MOB-5], both arms, three widths — six rows ───────────────────────
	for _, arm := range []struct {
		label string
		data  BenchData
	}{
		{"player", benchFxDataRsvp(false, false)},
		{"GM", benchFxDataRsvp(true, true)},
	} {
		inner := mobileWriteInner(t, "rsvp-"+arm.label+".html", mobileInnerPage(t, arm.data, ""))
		for _, w := range mobileWidths {
			h := mobileHeightFor(w)
			mobileHeader(t, "[MOB-5] the RSVP overview · "+arm.label+" arm", w, h)
			r := mobileDrive(t, chrome, inner, w, h, []mobileStep{{Op: "ovwrap"}})[0]
			wrap, grid := mobileBox(r, "wrap"), mobileBox(r, "grid")
			pg, _ := r["page"].(map[string]any)
			drag := wrap["sw"] - wrap["cw"]
			t.Logf("   .ovwrap client %.0fx%.0f · scroll %.0fx%.0f · DRAG %.0fpx · .ovgrid %.0f wide · overscroll-x %s · document %.0f/%.0f",
				wrap["cw"], wrap["h"], wrap["sw"], wrap["h"], drag, grid["w"],
				r.str("contain"), mobileF(pg, "sw"), mobileF(pg, "cw"))
			if wrap["cw"] == 0 {
				t.Fatalf("%dpx, %s arm: no .ovwrap was rendered — this measurement is of "+
					"nothing", w, arm.label)
			}
			if drag > 0 {
				t.Errorf("%dpx, %s arm: the overview needs %.0fpx of sideways drag (client "+
					"%.0f, scroll %.0f). [MOB-5] rules ZERO at 360, 375 and 390, on both "+
					"arms — a diagonal-gesture trap on the panel the operator's own gate "+
					"names", w, arm.label, drag, wrap["cw"], wrap["sw"])
			}
			// NOTHING WAS DROPPED TO GET THERE: every lane still shows all
			// seven days, and no identity cell has content wider than its box.
			names, _ := r["names"].([]any)
			for _, raw := range names {
				m, _ := raw.(map[string]any)
				if mobileF(m, "sw")-mobileF(m, "cw") > 1 {
					t.Errorf("%dpx, %s arm: the identity cell %q is clipped (%.0f of %.0f) — "+
						"the phone form may narrow the column, never abbreviate what is in it",
						w, arm.label, m["t"], mobileF(m, "cw"), mobileF(m, "sw"))
				}
			}
			// THE SIGNED PAIR STAYS AT EVERY WIDTH, AND "EVERY WIDTH" IS
			// MEASURED AGAINST THE DESKTOP RATHER THAN AGAINST A LITERAL.
			// `Ask →` is GM-only by audience ([GR-15]), so asserting it beside
			// `zone not set` on a player's page would assert a control that is
			// deliberately absent. What the signed law forbids is the phone
			// DROPPING something the desktop shows: "the repair may never be
			// the thing that disappears on the smallest screen".
			narrowTxt := r.str("text")
			wide := mobileDrive(t, chrome, inner, 1280, 900, []mobileStep{{Op: "ovwrap"}})[0]
			wideTxt := wide.str("text")
			for _, needle := range []string{"zone not set", "Ask"} {
				if strings.Contains(wideTxt, needle) && !strings.Contains(narrowTxt, needle) {
					t.Errorf("%dpx, %s arm: %q is on the panel at 1280 and gone at %d — the "+
						"repair may never be the thing that disappears on the smallest screen",
						w, arm.label, needle, w)
				}
			}
			t.Logf("   the signed pair at %d vs 1280 — `zone not set` %v/%v · `Ask` %v/%v",
				w, strings.Contains(narrowTxt, "zone not set"),
				strings.Contains(wideTxt, "zone not set"),
				strings.Contains(narrowTxt, "Ask"), strings.Contains(wideTxt, "Ask"))
		}
	}

	// ── [MOB-6] /schedule, four widths ────────────────────────────────────
	sched := mobileWriteInner(t, "schedule.html", mobileSchedulePage(t, true))
	for _, w := range []int{414, schedulePhoneViewport, scheduleMidPhoneViewport,
		scheduleNarrowViewport} {
		h := 736
		if w != 414 {
			h = mobileHeightFor(w)
		}
		mobileHeader(t, "[MOB-6] /schedule", w, h)
		r := mobileDrive(t, chrome, sched, w, h, []mobileStep{{Op: "schedule"}})[0]
		wraps, _ := r["wraps"].([]any)
		if len(wraps) == 0 {
			t.Fatalf("%dpx: no .sc-wrap rendered — this measurement is of nothing", w)
		}
		for i, raw := range wraps {
			m, _ := raw.(map[string]any)
			drag := mobileF(m, "sw") - mobileF(m, "cw")
			t.Logf("   .sc-wrap#%d — overflow-x %v · client %.0f · scroll %.0f · drag %.0fpx · overscroll-x %v",
				i, m["ox"], mobileF(m, "cw"), mobileF(m, "sw"), drag, m["contain"])
			if drag > 0 {
				t.Errorf("%dpx: .sc-wrap#%d drags %.0fpx sideways (client %.0f, scroll %.0f) — "+
					"the availability matrix was tuned to exactly one phone and this is the "+
					"other ones", w, i, drag, mobileF(m, "cw"), mobileF(m, "sw"))
			}
		}
		// The paint targets a player presses to say when they are free.
		pk, _ := r["pk"].(map[string]any)
		t.Logf("   .sc-pk — %v measured · smallest %.0fx%.0f · widest line count in a day row %.0f",
			mobileF(pk, "n"), mobileF(pk, "minW"), mobileF(pk, "minH"), mobileF(pk, "maxLines"))
		if mobileF(pk, "n") > 0 {
			if mobileF(pk, "minW") < 44 || mobileF(pk, "minH") < 44 {
				t.Errorf("%dpx: the smallest availability paint target is %.0fx%.0f against "+
					"the 44px platform floor (baseline 24x39 at 375 and 360)",
					w, mobileF(pk, "minW"), mobileF(pk, "minH"))
			}
			// A time axis that wraps three ways stops being an axis.
			if w == scheduleNarrowViewport && mobileF(pk, "maxLines") > 2 {
				t.Errorf("STOP-AND-FLAG at %dpx: a paint row wraps to %.0f lines. [MOB-6] "+
					"rules more than two a flag, because a time axis that wraps three ways "+
					"stops being an axis", w, mobileF(pk, "maxLines"))
			}
		}
		pg, _ := r["page"].(map[string]any)
		t.Logf("   document scrollWidth %.0f vs clientWidth %.0f",
			mobileF(pg, "sw"), mobileF(pg, "cw"))
		if mobileF(pg, "sw") != mobileF(pg, "cw") {
			t.Errorf("%dpx: /schedule's page fold broke — scrollWidth %.0f vs clientWidth %.0f",
				w, mobileF(pg, "sw"), mobileF(pg, "cw"))
		}
	}
}

// mobileScheduleOpsScript is /schedule's own measurement. It is a second,
// smaller listener rather than a branch inside the Bench's, because the two
// pages share no elements and a single script would be half dead on each.
const mobileScheduleOpsScript = `
(function () {
  function all(s) { return Array.prototype.slice.call(document.querySelectorAll(s)); }
  function reply(o) { o.innerWidth = window.innerWidth; o.innerHeight = window.innerHeight;
    parent.postMessage(JSON.stringify(o), '*'); }
  window.addEventListener('message', function (e) {
    var out = { op: String(e.data || '') };
    try {
      var d = document.scrollingElement || document.documentElement;
      out.page = { sw: d.scrollWidth, cw: d.clientWidth };
      out.wraps = all('.cal-schedule .sc-wrap').map(function (n) {
        var cs = getComputedStyle(n);
        return { cw: n.clientWidth, sw: n.scrollWidth, ox: cs.overflowX,
          contain: cs.overscrollBehaviorX };
      });
      // THE PAINT TARGETS, plus the LINE COUNT of the rows they wrap inside.
      // The line count is derived from the distinct top edges in a row rather
      // than from a height division, because the row's own height is exactly
      // what wrapping changes.
      var minW = 1e9, minH = 1e9, n = 0, maxLines = 0;
      all('.cal-schedule .sc-paintgrid .sc-row').forEach(function (row) {
        var tops = {};
        all2(row, '.sc-pk').forEach(function (b) {
          var r = b.getBoundingClientRect();
          if (r.width <= 0 && r.height <= 0) return;
          n++;
          if (r.width < minW) minW = r.width;
          if (r.height < minH) minH = r.height;
          tops[Math.round(r.top)] = 1;
        });
        var lines = Object.keys(tops).length;
        if (lines > maxLines) maxLines = lines;
      });
      out.pk = { n: n, minW: n ? minW : 0, minH: n ? minH : 0, maxLines: maxLines };
    } catch (err) { out.error = String(err && err.message || err); }
    reply(out);
  });
  function all2(root, s) { return Array.prototype.slice.call(root.querySelectorAll(s)); }
})();
`

// ── [MOB-7] — the tap floor, on a named list, in the block axis ───────────
//
// BASELINE at 390x664, scoped to the calendar's own surfaces: 46 of the
// editor's 50 visible controls under 44px, smallest 24x24 (the head ✕); the
// RSVP trio Yes 37x24 / No 33x24 / Maybe 52x24; `Ask →` 53x24; the Block's
// Layers invoker 28x28; the Ledger's Month tab 49x22; the ribbon tile's `→`
// link 10x19 — the smallest target measured anywhere on the page. The repo's
// own TestDayCardFloorsProbe was green on all of it, because it measures
// against 24.
//
// THE LIST IS SHORT ON PURPOSE and the other thirty-odd are not chased: these
// are the controls a person hits under time pressure at a table. The 24px
// floor stands unchanged for dense desktop chrome, which the 1280 control row
// at the end of this test proves rather than promises.
func TestMobileProbe_TheTapFloorAtAPhoneWidth(t *testing.T) {
	chrome := mobileNeedChromium(t)
	// EDIT MODE and the GM arm, because two of the named controls exist only
	// there: `[data-de-delete]` is hidden until a record has an id, and the
	// tie/roster chrome the editor's dense rows sit in only renders on a
	// loaded record. The stubbed GET is the capture rig's own, disclosed here
	// as it is disclosed there.
	inner := mobileWriteInner(t, "targets.html",
		mobileInnerPageWith(t, benchFxShotData(DayCardMount{
			CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
			CanRestrict: true, CampaignID: "camp-1"}), "", daycardStubEditRecord))
	// THE GM's RSVP PANEL IS ITS OWN PAGE: `Ask →` is a Director's control on
	// the roster and the editor fixture does not carry an RSVP panel at all.
	gmInner := mobileWriteInner(t, "targets-gm.html",
		mobileInnerPage(t, benchFxDataRsvp(true, true), ""))
	// THE PLAYER ARM IS A SECOND PAGE, because the RSVP answer trio is a
	// player's control and a GM's page does not carry it at all — the absence
	// is in the data ([BR2-7]), so there is no width at which a GM sees one.
	playerInner := mobileWriteInner(t, "targets-player.html",
		mobileInnerPage(t, benchFxDataRsvp(false, false), ""))

	steps := []mobileStep{
		{Op: "openCard"},
		{Op: "openEditRow", Delay: 500},
		{Op: "targets", Delay: 900},
	}
	panelSteps := []mobileStep{{Op: "targets"}}

	desktop := mobileDrive(t, chrome, inner, 1280, 900, steps)[2]
	deskMin := map[string][2]float64{}
	dl, _ := desktop["list"].([]any)
	for _, path := range []string{playerInner, gmInner} {
		if pd, ok := mobileDrive(t, chrome, path, 1280, 900,
			panelSteps)[0]["list"].([]any); ok {
			dl = append(dl, pd...)
		}
	}
	// ONLY RENDERED CONTROLS ARE RECORDED. The two pages overlap by label —
	// the GM's editor page has no RSVP trio and the player's page has no
	// editor — so folding a 0x0 "not rendered" row over a real measurement
	// would invent a regression out of an absence.
	for _, raw := range dl {
		m, _ := raw.(map[string]any)
		if mobileF(m, "n") == 0 {
			continue
		}
		deskMin[fmt.Sprint(m["label"])] = [2]float64{mobileF(m, "minW"), mobileF(m, "minH")}
	}

	for _, w := range mobileWidths {
		h := mobileHeightFor(w)
		mobileHeader(t, "[MOB-7] the tap floor", w, h)
		r := mobileDrive(t, chrome, inner, w, h, steps)[2]
		list, _ := r["list"].([]any)
		if len(list) == 0 {
			t.Fatalf("%dpx: nothing measured", w)
		}
		// The two RSVP pages contribute the controls the editor fixture does
		// not carry: the answer trio (a player's) and `Ask →` (a Director's).
		for _, panel := range []struct {
			path string
			want string
		}{
			{playerInner, "RSVP Yes / No / Maybe"},
			{gmInner, "Ask →"},
		} {
			pr := mobileDrive(t, chrome, panel.path, w, h, panelSteps)[0]
			pl, _ := pr["list"].([]any)
			for _, raw := range pl {
				m, _ := raw.(map[string]any)
				if fmt.Sprint(m["label"]) == panel.want && mobileF(m, "n") > 0 {
					list = append(list, raw)
				}
			}
		}
		for _, raw := range list {
			m, _ := raw.(map[string]any)
			label := fmt.Sprint(m["label"])
			n, minW, minH := mobileF(m, "n"), mobileF(m, "minW"), mobileF(m, "minH")
			t.Logf("   %-26s %-40s n=%.0f smallest %.1fx%.1f", label, m["sel"], n, minW, minH)
			if n == 0 {
				// A control this arm does not render is reported, never silently
				// counted as passing.
				t.Logf("      (not rendered in this arm — not asserted)")
				continue
			}
			// THE DAY CELL IS THE ONE REFUSAL, AND IT IS A CLOSED QUESTION.
			if label == "a day cell" {
				if minH < float64(daycardTouchFloorPx) {
					t.Errorf("%dpx: a day cell's BLOCK axis is %.1f, under %d — this axis is "+
						"reachable and is asserted", w, minH, daycardTouchFloorPx)
				}
				t.Logf("      REFUSED AS AN INLINE TARGET, WITH THE ARITHMETIC: a cell's "+
					"inline size is gridWidth / week-len; the grid measures 364px at a 390 "+
					"viewport, so a ten-day week gives 36.4px — the %.1f measured here — and "+
					"44px would need week-len <= 8 (364 / 8 = 45.5). No CSS reaches it "+
					"without dropping days, and dropping days is [MOB-8]'s data loss "+
					"arriving by another door. The BLOCK axis is %.1f and passes.", minW, minH)
				continue
			}
			if minH < float64(daycardTouchFloorPx) {
				t.Errorf("%dpx: %s measures %.1fx%.1f — the block axis is under the %dpx "+
					"platform floor. [MOB-7] names this control; it is one a person hits "+
					"under time pressure at a table", w, label, minW, minH, daycardTouchFloorPx)
			}
		}

		// THE ✕ AND DELETE ARE HELD APART, and the report says whether the
		// editor lane's Delete CONFIRMATION has shipped — because if it has
		// not, this separation is the only protection there is.
		// C-CALV4-DAYCARD-WDWRAP: a booking retired by measurement.
		if wd, ok := r["wdstrip"].(map[string]any); ok && mobileF(wd, "stops") > 0 {
			t.Logf("   C-CALV4-DAYCARD-WDWRAP — the weekday strip: %.0f stops, %.0f row(s), "+
				"%.0f outside the box, .wdpick right edge %.0f. The booking said it wraps "+
				"\"6 + 4\"; it does not.",
				mobileF(wd, "stops"), mobileF(wd, "rows"), mobileF(wd, "outside"),
				mobileF(wd, "right"))
			if mobileF(wd, "outside") > 0 {
				t.Errorf("%dpx: %.0f weekday stop(s) render outside the strip's own box",
					w, mobileF(wd, "outside"))
			}
		}

		gap := r.num("xDeleteGap")
		t.Logf("   ✕ ↔ Delete edge-to-edge gap: %.0fpx (ruled >= 8) · Delete rendered %v",
			gap, r.boolean("deleteVisible"))
		if r.boolean("deleteVisible") && gap >= 0 && gap < 8 {
			t.Errorf("%dpx: the head ✕ and Delete are %.0fpx apart — a 24x24 ✕ beside a "+
				"one-click Delete is a destroyed event one pixel of thumb-slop away", w, gap)
		}
	}

	// ── NOTHING ABOVE 640 MOVED, and this is the row that proves it ───────
	mobileHeader(t, "[MOB-7] CONTROL · desktop, the same named list", 1280, 900)
	after := mobileDrive(t, chrome, inner, 1280, 900, steps)[2]
	al, _ := after["list"].([]any)
	for _, path := range []string{playerInner, gmInner} {
		if pa, ok := mobileDrive(t, chrome, path, 1280, 900,
			panelSteps)[0]["list"].([]any); ok {
			al = append(al, pa...)
		}
	}
	for _, raw := range al {
		m, _ := raw.(map[string]any)
		label := fmt.Sprint(m["label"])
		if mobileF(m, "n") == 0 {
			continue
		}
		got := [2]float64{mobileF(m, "minW"), mobileF(m, "minH")}
		t.Logf("   %-26s smallest %.1fx%.1f at 1280", label, got[0], got[1])
		if was, ok := deskMin[label]; ok && was != got {
			t.Errorf("the desktop measurement of %s moved from %.1fx%.1f to %.1fx%.1f — "+
				"[MOB-7] rules that nothing above 640 changes", label,
				was[0], was[1], got[0], got[1])
		}
	}
}
