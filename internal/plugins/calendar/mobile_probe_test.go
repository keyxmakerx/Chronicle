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
		at += 260
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
	budget := at + 1500
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
  // A box counts when it overflows AND its computed overflow on that axis is
  // not 'visible' — which is the only definition a thumb can tell apart.
  function scrollers(rootSel) {
    var root = rootSel ? el(rootSel) : document.body;
    if (!root) return [];
    var out = [];
    all(rootSel ? rootSel + ' *' : '*').forEach(function (n) {
      var cs = getComputedStyle(n);
      var vy = cs.overflowY !== 'visible' && cs.overflowY !== 'clip';
      var vx = cs.overflowX !== 'visible' && cs.overflowX !== 'clip';
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
