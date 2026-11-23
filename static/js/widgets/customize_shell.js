/**
 * customize_shell.js: Customize's frame (customize_shell.templ).
 *
 * Customize keeps ONE draft for all its areas: a JSON object of named
 * sections ("look", "categories", ...). The server keeps it until the owner
 * presses Save or Discard (GET/PUT/DELETE /campaigns/:id/customize/draft),
 * so leaving the page loses nothing. Save asks the server to apply every
 * section (POST .../customize/publish); each section is applied by the
 * plugin that owns its data, and a section that fails stays in the draft.
 *
 * The frame also runs the wall's behaviour (Customize's first page): it scales
 * each picture to its tile, lights the part a shared picture changes in the
 * others, and, at module level, grows a pressed picture into its workshop
 * and back (see "The grow" below).
 *
 * Areas talk to the frame through the page, not through each other:
 *   - An area changes its section with Chronicle.customizeSet(key, value)
 *     (value null removes the section). A value may carry `_changes`, a list
 *     of { label, href } the save bar lists; the server ignores `_`-keys.
 *   - The frame announces every change it did not get from that call (Undo,
 *     Redo, Discard, Save) as a `chronicle:customize-draft` event on
 *     document, detail { reason, draft, keys, applied }. Areas redraw from
 *     detail.draft.
 *   - The current draft is always on the frame's data-draft attribute, so an
 *     area can read it whenever it mounts (Chronicle.customizeDraft()).
 *   - Chronicle.customizeFlush() resolves once the draft is kept, before an
 *     area opens a page that reads it.
 *   - An area with its own Undo and Redo buttons (the Customizer's page layout) calls
 *     Chronicle.customizeUndo / customizeRedo and reads customizeHistory();
 *     customizeSet(key, value, { step: true }) makes that change its own Undo
 *     step rather than joining a quick run of edits to the same section.
 */
(function () {
  'use strict';

  var EVENT = 'chronicle:customize-draft';
  var SAVE_DELAY = 700;      // ms of quiet before the draft is sent
  var COALESCE = 1200;       // edits to one section this close share one Undo step
  var MAX_UNDO = 100;

  function parse(s, dflt) {
    try { var v = JSON.parse(s || ''); return v == null ? dflt : v; } catch (e) { return dflt; }
  }
  function esc(s) { return Chronicle.escapeHtml(s); }
  function typing(t) {
    if (!t || !t.closest) return false;
    return !!t.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"]');
  }

  // Shared with the areas: read the draft, and change a section of it. An
  // area may load before the frame's script; its changes wait in a queue
  // the frame drains when it mounts.
  // Areas whose script loaded first queue their changes here.
  Chronicle._customizeQueue = Chronicle._customizeQueue || [];
  var active = null;
  Chronicle.customizeDraft = function () {
    if (active) return active.read();
    var el = document.querySelector('[data-widget="customize-shell"]');
    return el ? parse(el.getAttribute('data-draft'), {}) : {};
  };
  Chronicle.customizeSet = function (key, value, opts) {
    if (active) active.set(key, value, opts);
    else Chronicle._customizeQueue.push([key, value, opts]);
  };
  Chronicle.customizeUndo = function () { if (active) active.undo(); };
  Chronicle.customizeRedo = function () { if (active) active.redo(); };
  Chronicle.customizeHistory = function () {
    return active ? active.history() : { undo: false, redo: false };
  };
  // customizeToast shows a note in the frame's toast stack; undoFn adds an
  // Undo button.
  Chronicle.customizeToast = function (msg, bad, undoFn) { if (active) active.toast(msg, bad, undoFn); };
  // customizeFlush resolves once the draft as it stands is kept on the
  // server, for an area about to open a page that reads it.
  Chronicle.customizeFlush = function () {
    return active ? active.flush() : Promise.resolve(true);
  };
  // The last frame's toast stack. An area that reads itself again after a
  // Save (a category does) swaps the whole frame, toast stack included, a
  // moment after "Saved." appears; the new frame takes over the notes still
  // showing so they stay long enough to read.
  var lastToasts = null;


  /* ---------- The grow ----------
   * Pressing a picture on the wall flies a copy of it (the "fly") from its
   * tile to where the workshop's stage will be, in the same frame as the
   * press, while the wall steps back; the request runs meanwhile. The copy is
   * laid out at its landing size with the miniature scaled to that width and
   * played from the tile with a transform, so it stays sharp as it grows. It
   * holds there, over the stage, until the workshop has drawn (every widget
   * in [data-czs-stage] mounted and no [aria-busy="true"] left in it, or
   * 1.5 s at most), then
   * cross-fades into it. "Everything" and browser Back step the workshop back
   * at once and, when the wall arrives, fly the picture from the stage into
   * its tile. Calm fades the picture in place instead of flying it; Off and
   * a device asking for reduced motion swap at once. Focus lands on the
   * workshop's name going in and on the tile you came from coming back.
   * html[data-czs-fly] keeps the arriving content hidden until its move
   * starts (customize_wall.css), so nothing is drawn first and then moves. */
  var G = document.documentElement;
  var growFrom = '', fly = null, back = null, lastBar = null;
  var READY_CAP = 1500;      // ms after the swap the fly waits for the workshop at most
  var STALE = 10000;         // ms after which a move with no swap is given up
  function motionLevel() {
    var m = G.getAttribute('data-motion');
    if (m === 'off' || (window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches)) return 'off';
    if (m === 'calm' || G.hasAttribute('data-cz-reduce') || G.getAttribute('data-view-motion') === 'calm') return 'calm';
    return 'full';
  }
  function cssMs(name, dflt) {
    var v = parseFloat(getComputedStyle(G).getPropertyValue(name));
    return isFinite(v) && v > 0 ? v : dflt;
  }
  function cssEase(name, dflt) { return getComputedStyle(G).getPropertyValue(name).trim() || dflt; }
  function keyAttr(k) { return String(k || '').replace(/[^a-z0-9-]/gi, ''); }
  // The wall picture a workshop returns to: the one pressed to get here, or
  // the one its own address names after a reload.
  function returnKey(frame) {
    var own = frame ? frame.getAttribute('data-wall-key') : '';
    var look = frame && frame.getAttribute('data-area') === 'look';
    if (look && growFrom && !/^(cat|home|app)-/.test(growFrom)) return growFrom;
    return own || growFrom || 'frame';
  }
  function tileFor(wall, key) {
    return wall ? wall.querySelector('.czw-tile[data-czw-key="' + keyAttr(key) + '"]') : null;
  }
  // Where a workshop's stage sits: below the bar, across the content column,
  // down to the foot of the window. On the wall there is no bar yet, so its
  // height is the one last measured, or the frame's own --czs-bar-h.
  function stageBox() {
    var main = document.getElementById('main-content'), czs = document.querySelector('.czs');
    var m = main ? main.getBoundingClientRect() : { top: 0 }, c = czs ? czs.getBoundingClientRect() : { left: 0, width: window.innerWidth };
    var bar = (lastBar && lastBar.w === window.innerWidth && lastBar.h) || (czs && parseFloat(getComputedStyle(czs).getPropertyValue('--czs-bar-h'))) || 50;
    var top = Math.max(0, m.top) + bar;
    return { left: c.left, top: top, width: c.width, height: Math.max(0, window.innerHeight - top) };
  }
  // The copy that flies: the tile's miniature in a picture laid out at width w
  // at (x, y), which scales the miniature to that width itself.
  function ghostOf(tile, x, y, w) {
    var mini = tile && tile.querySelector('.czw-mini');
    if (!mini) return null;
    var g = document.createElement('div');
    g.className = 'czw-pic czw-fly';
    g.setAttribute('aria-hidden', 'true');
    g.style.left = x + 'px'; g.style.top = y + 'px'; g.style.width = w + 'px';
    g.appendChild(mini.cloneNode(true));
    g.firstChild.style.setProperty('--s', (w / 720).toFixed(4));
    document.body.appendChild(g);
    return g;
  }
  function flipFrom(r, g) {
    var w = g.offsetWidth || 1, x = parseFloat(g.style.left), y = parseFloat(g.style.top);
    return 'translate(' + (r.left - x) + 'px,' + (r.top - y) + 'px) scale(' + (r.width / w) + ')';
  }
  // Puts everything a move touched back and forgets it; safe to call twice.
  function flyClear(m) {
    m = m || fly;
    if (!m) return;
    if (m === fly) fly = null;
    if (m === back) back = null;
    cancelAnimationFrame(m.raf); clearTimeout(m.cap); clearTimeout(m.stale);
    m.anims.forEach(function (a) { try { a.cancel(); } catch (e) { /* already gone */ } });
    m.anims = [];
    if (m.ghost) m.ghost.remove();
    if (m.pic) m.pic.style.visibility = '';
    if (m.wall) { m.wall.style.pointerEvents = ''; m.wall.style.transformOrigin = ''; }
    if (!fly && !back) G.removeAttribute('data-czs-fly');
  }
  function play(m, el, frames, opts) {
    if (!el || !el.animate) return null;
    var a = el.animate(frames, opts);
    m.anims.push(a);
    return a;
  }
  // Moves keyboard focus without scrolling the page, keeping the target in
  // view of the scrolling column.
  function focusQuiet(el) {
    if (!el) return;
    try { el.focus({ preventScroll: true }); } catch (e) { el.focus(); }
    var main = document.getElementById('main-content');
    if (!main || !main.contains(el)) return;
    var r = el.getBoundingClientRect(), m = main.getBoundingClientRect();
    if (r.top < m.top + 8) main.scrollTop -= m.top + 8 - r.top;
    else if (r.bottom > m.bottom - 8) main.scrollTop += Math.min(r.top - m.top - 8, r.bottom - m.bottom + 8);
  }
  function focusLost() {
    var a = document.activeElement;
    return !a || a === document.body || !a.isConnected;
  }

  // Opening: a press on a picture starts the fly at once; htmx sends the
  // request from the same click.
  function grow(tile) {
    flyClear(fly); flyClear(back);
    growFrom = tile.getAttribute('data-czw-key') || '';
    var lv = motionLevel();
    if (lv === 'off') return;
    var wall = tile.closest('[data-czw]'), pic = tile.querySelector('[data-czw-pic]');
    var to = stageBox(), from = pic && pic.getBoundingClientRect();
    var g = from && from.width ? ghostOf(tile, to.left, to.top, to.width) : null;
    if (!g) return;
    var m = fly = { kind: 'grow', elt: tile, ghost: g, pic: pic, wall: wall, anims: [], raf: 0, cap: 0, stale: 0, landed: false, swapped: false, settled: false };
    G.setAttribute('data-czs-fly', 'grow');
    wall.style.pointerEvents = 'none';
    var ease = cssEase('--ease-out', 'ease-out'), a;
    if (lv === 'full') {
      var dur = cssMs('--dur-slide', 360) * 0.92;
      pic.style.visibility = 'hidden';
      // The wall steps back about the middle of what shows of it.
      var wr = wall.getBoundingClientRect();
      wall.style.transformOrigin = (window.innerWidth / 2 - wr.left) + 'px ' + (window.innerHeight / 2 - wr.top) + 'px';
      play(m, wall, [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(0.97)' }], { duration: dur, easing: ease, fill: 'forwards' });
      a = play(m, g, [{ transform: flipFrom(from, g) }, { transform: 'none' }], { duration: dur, easing: ease, fill: 'both' });
    } else {
      var calm = cssMs('--dur-standard', 200);
      play(m, wall, [{ opacity: 1 }, { opacity: 0 }], { duration: calm, easing: ease, fill: 'forwards' });
      a = play(m, g, [{ opacity: 0 }, { opacity: 1 }], { duration: calm, easing: ease, fill: 'both' });
    }
    var landed = function () { m.landed = true; };
    if (a) { a.onfinish = landed; } else landed();
    m.stale = setTimeout(function () { flyClear(m); }, STALE);
  }
  // The workshop has drawn: the picture fades into it.
  function reveal(m) {
    if (m !== fly) return;
    cancelAnimationFrame(m.raf); clearTimeout(m.cap);
    var stage = document.querySelector('.czs-is-workshop [data-czs-stage]');
    var dur = cssMs('--dur-standard', 200), ease = cssEase('--ease-out', 'ease-out');
    G.removeAttribute('data-czs-fly');
    if (stage && motionLevel() !== 'off') play(m, stage, [{ opacity: 0 }, { opacity: 1 }], { duration: dur * 0.6, easing: ease });
    var a = play(m, m.ghost, [{ opacity: 1 }, { opacity: 0 }], { duration: dur, easing: ease, fill: 'forwards' });
    // The stage keeps animation: none, or its own settle would play again.
    var done = function () { flyClear(m); };
    if (a) { a.onfinish = done; a.oncancel = done; } else done();
  }
  function waitReady(m) {
    var t0 = 0;
    var tick = function () {
      if (m !== fly) return;
      var stage = document.querySelector('.czs-is-workshop [data-czs-stage]');
      if (!stage) { flyClear(m); return; }
      // A widget whose script is still on the way counts as loading; one
      // frame after mounting, its own loading mark has had its chance.
      var waiting = Chronicle.isMounted && Array.prototype.some.call(stage.querySelectorAll('[data-widget]'), function (w) { return !Chronicle.isMounted(w); });
      if (waiting) t0 = 0;
      else if (m.landed && t0 && !stage.querySelector('[aria-busy="true"]')) { reveal(m); return; }
      else t0 = 1;
      m.raf = requestAnimationFrame(tick);
    };
    m.raf = requestAnimationFrame(tick);
    m.cap = setTimeout(function () { if (m === fly) reveal(m); }, READY_CAP);
  }

  // Closing: the workshop steps back at once, as feedback while the wall is
  // fetched; the picture flies home once it arrives.
  function stepBack(frame) {
    flyClear(fly); flyClear(back);
    var stage = frame && frame.querySelector('[data-czs-stage]'), lv = motionLevel();
    if (!stage) return;
    var top = frame.querySelector('[data-czs-top]'), r = stage.getBoundingClientRect();
    var t = Math.max(r.top, top ? top.getBoundingClientRect().bottom : r.top);
    var m = back = { kind: 'shrink', key: returnKey(frame), rect: { left: r.left, top: t, width: r.width }, lv: lv, anims: [], raf: 0, cap: 0, stale: 0, t: Date.now() };
    if (lv === 'off') return;
    G.setAttribute('data-czs-fly', 'shrink');
    var dur = cssMs('--dur-micro', 120), ease = cssEase('--ease-out', 'ease-out');
    play(m, stage, lv === 'full' ? [{ opacity: 1, transform: 'none' }, { opacity: 0.6, transform: 'scale(0.985)' }] : [{ opacity: 1 }, { opacity: 0.6 }],
      { duration: dur, easing: ease, fill: 'forwards' });
    m.stale = setTimeout(function () { flyClear(m); }, STALE);
  }
  // The wall is in: focus the tile you came from and fly its picture home.
  function homeward(m) {
    var wall = document.querySelector('.czs-is-wall [data-czw]');
    flyClear(m);
    if (!wall) return;
    var tile = tileFor(wall, m.key) || tileFor(wall, 'frame');
    if (tile) focusQuiet(tile);
    if (m.lv === 'off' || !tile) return;
    var pic = tile.querySelector('[data-czw-pic]'), to = pic && pic.getBoundingClientRect();
    var g = to && to.width ? ghostOf(tile, m.rect.left, m.rect.top, m.rect.width) : null;
    var n = back = { kind: 'home', ghost: g, pic: pic, wall: wall, anims: [], raf: 0, cap: 0, stale: 0 };
    G.setAttribute('data-czs-fly', 'shrink');
    var a;
    if (m.lv === 'full' && g) {
      var dur = cssMs('--dur-large', 280) * 0.9, ease = cssEase('--ease-in-out', 'ease-in-out');
      pic.style.visibility = 'hidden';
      var wr = wall.getBoundingClientRect();
      wall.style.transformOrigin = (window.innerWidth / 2 - wr.left) + 'px ' + (window.innerHeight / 2 - wr.top) + 'px';
      play(n, wall, [{ opacity: 0, transform: 'scale(0.97)' }, { opacity: 1, transform: 'none' }], { duration: dur, easing: ease });
      a = play(n, g, [{ transform: 'none' }, { transform: flipFrom(to, g) }], { duration: dur, easing: ease, fill: 'forwards' });
    } else {
      var calm = cssMs('--dur-standard', 200), ce = cssEase('--ease-out', 'ease-out');
      play(n, wall, [{ opacity: 0 }, { opacity: 1 }], { duration: calm, easing: ce });
      a = play(n, g, [{ opacity: 1 }, { opacity: 0 }], { duration: calm, easing: ce, fill: 'forwards' });
    }
    // The wall's own fade now holds it, so the hiding rule can go.
    G.setAttribute('data-czs-fly', 'home');
    var done = function () { flyClear(n); };
    if (a) { a.onfinish = done; a.oncancel = done; } else done();
  }

  document.addEventListener('click', function (e) {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || !e.target.closest) return;
    var tile = e.target.closest('.czs-is-wall .czw-tile[href]');
    if (tile) { grow(tile); return; }
    var link = e.target.closest('.czs-is-workshop [data-czs-back]');
    if (link) stepBack(link.closest('.czs'));
  }, true);
  // Browser Back from a part: the same step back, then the same way home.
  window.addEventListener('popstate', function () {
    if (fly) flyClear(fly);
    var frame = document.querySelector('.czs-is-workshop');
    if (frame) stepBack(frame); else flyClear(back);
  }, true);
  document.addEventListener('htmx:afterSwap', function (e) {
    var main = document.getElementById('main-content');
    if (!e.detail || e.detail.target !== main) return;
    if (fly && document.querySelector('.czs-is-workshop')) {
      fly.swapped = true;
      clearTimeout(fly.stale);
      // The stage's own settle would replay once the hiding rule lifts.
      var st = document.querySelector('.czs-is-workshop [data-czs-stage]');
      if (st) st.style.animation = 'none';
    } else if (fly) flyClear(fly);
    if (back && back.kind === 'shrink') {
      var m = back;
      // After boot.js puts the column back at its top, before the first paint.
      if (document.querySelector('.czs-is-wall')) Promise.resolve().then(function () { homeward(m); });
      else flyClear(m);
    }
  });
  document.addEventListener('htmx:afterSettle', function (e) {
    var main = document.getElementById('main-content');
    if (!e.detail || e.detail.target !== main) return;
    var frame = document.querySelector('.czs-is-workshop');
    if (!frame) return;
    // Going in, focus lands on the workshop's name once its widgets have mounted.
    requestAnimationFrame(function () {
      if (focusLost()) focusQuiet(frame.querySelector('.czs-catname'));
    });
    if (fly && fly.swapped) waitReady(fly);
  });
  document.addEventListener('htmx:historyRestore', function () {
    if (back && back.kind === 'shrink' && document.querySelector('.czs-is-wall')) { homeward(back); return; }
    flyClear(fly); flyClear(back);
    var frame = document.querySelector('.czs-is-workshop');
    if (frame) requestAnimationFrame(function () { if (focusLost()) focusQuiet(frame.querySelector('.czs-catname')); });
  });
  ['htmx:responseError', 'htmx:sendError', 'htmx:timeout', 'htmx:swapError'].forEach(function (t) {
    document.addEventListener(t, function () { flyClear(fly); flyClear(back); });
  });
  // Escape mid-move: before the swap it stops the request and the page stays;
  // after it, the workshop shows at once. Either way no copy is left behind.
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape' || (!fly && !back)) return;
    if (fly && !fly.swapped && window.htmx) htmx.trigger(fly.elt, 'htmx:abort');
    if (back && back.kind === 'shrink' && window.htmx) {
      var l = document.querySelector('.czs-is-workshop [data-czs-back]');
      if (l) htmx.trigger(l, 'htmx:abort');
    }
    flyClear(fly); flyClear(back);
  }, true);
  Chronicle._customizeGrow = { clear: function () { flyClear(fly); flyClear(back); }, active: function () { return !!(fly || back); } };

  function mount(el) {
    var CID = el.getAttribute('data-campaign-id');
    var CSRF = el.getAttribute('data-csrf');
    var BASE = '/campaigns/' + encodeURIComponent(CID) + '/customize';
    var areas = parse(el.getAttribute('data-areas'), []);
    var finds = parse(el.getAttribute('data-find'), []);
    var draft = parse(el.getAttribute('data-draft'), {});
    var version = Number(el.getAttribute('data-version')) || 0;
    var past = [], future = [];
    var lastKey = null, lastAt = 0;
    var rev = 0, sentRev = 0, saveT = 0, inflight = null, retryT = 0;
    var problem = null;        // 'conflict' | 'offline' | null
    var publishing = false;
    var failed = [];           // [{ key, message }] from the last Save
    var alive = true;
    var listeners = [];

    function on(t, type, fn, o) { t.addEventListener(type, fn, o); listeners.push([t, type, fn, o]); }
    function $(sel) { return el.querySelector(sel); }
    function $$(sel) { return Array.prototype.slice.call(el.querySelectorAll(sel)); }
    function snapshot() { return JSON.stringify(draft); }
    function sections() { return Object.keys(draft).filter(function (k) { return k.charAt(0) !== '_'; }); }
    function areaFor(key) { return areas.filter(function (a) { return a.section === key; })[0]; }

    /* ---------- Changing the draft ---------- */
    function announce(reason, keys, extra) {
      el.setAttribute('data-draft', snapshot());
      var detail = { reason: reason, draft: parse(snapshot(), {}), keys: keys || [] };
      if (extra) Object.keys(extra).forEach(function (k) { detail[k] = extra[k]; });
      document.dispatchEvent(new CustomEvent(EVENT, { detail: detail }));
      paint();
    }
    function changedKeys(a, b) {
      var A = parse(a, {}), B = parse(b, {}), out = [];
      Object.keys(A).concat(Object.keys(B)).forEach(function (k) {
        if (out.indexOf(k) < 0 && JSON.stringify(A[k]) !== JSON.stringify(B[k])) out.push(k);
      });
      return out;
    }
    function set(key, value, opts) {
      if (!alive || typeof key !== 'string' || !key) return;
      var before = snapshot();
      var empty = value == null || (typeof value === 'object' && !Object.keys(value).some(function (k) { return k.charAt(0) !== '_'; }));
      if (empty) delete draft[key];
      else draft[key] = parse(JSON.stringify(value), null);
      if (snapshot() === before) return;
      var now = Date.now();
      var step = !!(opts && opts.step);
      if (step || !(key === lastKey && now - lastAt < COALESCE && past.length)) {
        past.push(before);
        if (past.length > MAX_UNDO) past.shift();
      }
      future = [];
      lastKey = key; lastAt = now;
      failed = failed.filter(function (f) { return f.key !== key; });
      el.setAttribute('data-draft', snapshot());
      changed();
      paint();
    }
    function restore(to, reason) {
      var before = snapshot();
      draft = parse(to, {});
      lastKey = null;
      announce(reason, changedKeys(before, to));
      changed();
    }
    function undo() {
      if (!past.length || publishing) return;
      future.push(snapshot());
      restore(past.pop(), 'undo');
    }
    function redo() {
      if (!future.length || publishing) return;
      past.push(snapshot());
      restore(future.pop(), 'redo');
    }

    /* ---------- Keeping the draft on the server ---------- */
    function changed() {
      rev++;
      Chronicle.markDirty('customize-draft');
      clearTimeout(saveT);
      saveT = setTimeout(function () { send(false); }, SAVE_DELAY);
    }
    function api(method, path, body, keepalive) {
      var opts = { method: method, csrfToken: CSRF };
      if (body !== undefined) opts.body = body;
      if (keepalive) opts.keepalive = true;
      return Chronicle.apiFetch(BASE + path, opts).then(function (res) {
        return res.json().then(function (b) { return { ok: res.ok, status: res.status, body: b }; },
          function () { return { ok: res.ok, status: res.status, body: null }; });
      });
    }
    // send keeps the server's copy in step; it resolves once what was
    // current when it was called is stored (or has failed).
    function send(keepalive) {
      clearTimeout(saveT);
      if (problem === 'conflict') return Promise.resolve(false);
      if (inflight) return inflight.then(function () { return rev === sentRev ? true : send(keepalive); });
      if (rev === sentRev) return Promise.resolve(true);
      var mine = rev, body = { version: version, draft: parse(snapshot(), {}) };
      inflight = api('PUT', '/draft', body, keepalive).then(function (r) {
        inflight = null;
        if (r.ok) {
          version = (r.body && typeof r.body.version === 'number') ? r.body.version : version;
          el.setAttribute('data-version', String(version));
          sentRev = mine;
          if (problem === 'offline') problem = null;
          if (rev === sentRev) Chronicle.markClean('customize-draft');
          if (alive) paint();
          return true;
        }
        if (r.status === 409) { problem = 'conflict'; if (alive) paint(); return false; }
        return offline();
      }, function () { inflight = null; return offline(); });
      return inflight;
    }
    function offline() {
      problem = 'offline';
      if (alive) {
        paint();
        clearTimeout(retryT);
        retryT = setTimeout(function () { send(false); }, 5000);
      }
      return false;
    }

    /* ---------- Save and Discard ---------- */
    function labels() {
      var out = [];
      sections().forEach(function (k) {
        var sec = draft[k] || {}, ch = Array.isArray(sec._changes) ? sec._changes : [];
        if (ch.length) ch.forEach(function (c) { if (c && c.label) out.push({ key: k, label: String(c.label), href: c.href || '' }); });
        else {
          var a = areaFor(k);
          out.push({ key: k, label: a ? a.label : k, href: a ? a.href : '' });
        }
      });
      return out;
    }
    function publish() {
      if (publishing || problem === 'conflict' || !sections().length) return;
      publishing = true; paint();
      send(false).then(function (ok) {
        if (!ok) { publishing = false; paint(); toast('Not saved. ' + problemText(), true); return null; }
        return api('POST', '/publish', { version: version });
      }).then(function (r) {
        if (!r) return;
        publishing = false;
        if (!r.ok) {
          if (r.status === 409) problem = 'conflict';
          paint();
          toast('Not saved. ' + ((r.body && (r.body.message || r.body.error)) || problemText()), true);
          return;
        }
        var b = r.body || {};
        draft = b.draft && typeof b.draft === 'object' ? b.draft : {};
        version = typeof b.version === 'number' ? b.version : 0;
        el.setAttribute('data-version', String(version));
        rev = sentRev = 0;
        past = []; future = []; lastKey = null;
        failed = Array.isArray(b.failed) ? b.failed : [];
        Chronicle.markClean('customize-draft');
        announce('publish', b.applied || [], { applied: b.applied || [], failed: failed });
        if (failed.length) {
          var a = areaFor(failed[0].key);
          toast('Some changes were not saved. ' + (a ? a.label + ': ' : '') + failed[0].message, true);
        } else {
          toast('Saved. Everyone in the campaign now sees your changes.');
        }
      }, function () {
        publishing = false; paint();
        toast('Not saved. Chronicle could not be reached; your draft is kept.', true);
      });
    }
    function discard() {
      if (publishing || !sections().length) return;
      var kept = snapshot();
      clearTimeout(saveT);
      var go = inflight ? inflight.then(function () {}) : Promise.resolve();
      go.then(function () {
        return api('DELETE', '/draft?version=' + encodeURIComponent(version));
      }).then(function (r) {
        if (!r.ok && r.status !== 404) {
          if (r.status === 409) { problem = 'conflict'; paint(); }
          toast('Not thrown away. ' + problemText(), true);
          return;
        }
        version = 0; rev = sentRev = 0; failed = [];
        el.setAttribute('data-version', '0');
        past = []; future = [];
        Chronicle.markClean('customize-draft');
        restoreQuiet({}, 'discard');
        toast('Changes thrown away.', false, function () {
          // Bring the draft back as a new one.
          version = 0;
          restoreQuiet(parse(kept, {}), 'restore');
          rev++; send(false);
        });
      }, function () { toast('Not thrown away. Chronicle could not be reached.', true); });
    }
    function restoreQuiet(to, reason) {
      var before = snapshot();
      draft = to;
      lastKey = null;
      announce(reason, changedKeys(before, snapshot()));
    }
    function problemText() {
      if (problem === 'conflict') return 'This draft was changed in another tab or window. Reload to see the latest.';
      if (problem === 'offline') return 'Chronicle could not be reached. Your changes are kept on this page and sent again shortly.';
      return 'Try again.';
    }

    /* ---------- The save bar, the rail and toasts ---------- */
    var bar = $('[data-czs-savebar]');
    function paint() {
      var secs = sections();
      $$('[data-czs-area]').forEach(function (a) {
        var area = areas.filter(function (x) { return x.id === a.getAttribute('data-czs-area'); })[0];
        var dot = a.querySelector('.czs-dot');
        if (dot) dot.hidden = !(area && area.section && secs.indexOf(area.section) >= 0);
      });
      var dotsElsewhere = document.querySelectorAll('[data-czs-area-dot]');
      Array.prototype.forEach.call(dotsElsewhere, function (d) {
        var area = areas.filter(function (x) { return x.id === d.getAttribute('data-czs-area-dot'); })[0];
        d.hidden = !(area && area.section && secs.indexOf(area.section) >= 0);
      });
      // The bar stays while there is something to redo: Redo has no other
      // button on a phone. Save and Discard empty the redo history, so it
      // still goes away after either.
      var show = secs.length > 0 || publishing || problem === 'conflict' || future.length > 0;
      bar.hidden = !show;
      if (!show) return;
      var ls = labels(), n = ls.length;
      var what = $('[data-czs-what]'), sdot = bar.querySelector('.czs-sdot');
      sdot.classList.toggle('is-bad', !!problem || failed.length > 0);
      sdot.hidden = !secs.length && !problem && !publishing && !failed.length;
      // With nothing waiting the words open no list, so they are not a control then.
      what.disabled = !secs.length && !publishing && problem !== 'conflict';
      if (problem === 'conflict') what.innerHTML = '<b>Draft changed elsewhere.</b> Reload to see it.';
      else if (publishing) what.innerHTML = '<span class="czs-spin" aria-hidden="true"></span> <b>Saving…</b>';
      else if (!secs.length) what.innerHTML = '<b>No changes</b> waiting' + (problem === 'offline' ? ', not yet kept' : '');
      else {
        what.innerHTML = '<b>' + n + (n === 1 ? ' change' : ' changes') + '</b> waiting' +
          (problem === 'offline' ? ', not yet kept' : '') + ': ' + esc(ls.map(function (l) { return l.label; }).join(', '));
      }
      $('[data-czs-undo]').disabled = !past.length || publishing;
      $('[data-czs-redo]').disabled = !future.length || publishing;
      var dis = $('[data-czs-discard]'), save = $('[data-czs-save]');
      dis.disabled = publishing || !secs.length;
      save.disabled = publishing || problem === 'conflict' || !secs.length;
      save.textContent = problem === 'conflict' ? 'Reload' : (publishing ? 'Saving…' : 'Save changes');
      if (problem === 'conflict') { save.disabled = false; save.setAttribute('data-reload', ''); }
      else save.removeAttribute('data-reload');
    }
    var toasts = $('[data-czs-toasts]');
    if (toasts && lastToasts && lastToasts !== toasts && !lastToasts.isConnected) {
      // A note with Undo is left behind: its Undo belongs to the old frame.
      while (lastToasts.firstChild) {
        var t0 = lastToasts.firstChild;
        if (t0.nodeType !== 1 || t0.hasAttribute('data-undo')) { t0.remove(); continue; }
        t0.classList.add('is-carried');
        toasts.appendChild(t0);
      }
    }
    if (toasts) lastToasts = toasts;
    function toast(msg, bad, undoFn) {
      if (!toasts) return;
      var t = document.createElement('div');
      t.className = 'czs-toast' + (bad ? ' is-bad' : '');
      if (undoFn) t.setAttribute('data-undo', '');
      t.innerHTML = '<span></span>' + (undoFn ? '<button type="button">Undo</button>' : '') +
        (bad ? '<button type="button" aria-label="Dismiss">Close</button>' : '');
      t.firstChild.textContent = msg;
      var btns = t.querySelectorAll('button');
      if (undoFn) btns[0].addEventListener('click', function () { undoFn(); t.remove(); });
      if (bad) btns[btns.length - 1].addEventListener('click', function () { t.remove(); });
      toasts.appendChild(t);
      // Errors stay until dismissed; everything else leaves on its own.
      if (!bad) setTimeout(function () { t.remove(); }, undoFn ? 6500 : 3200);
    }

    /* ---------- What is waiting ---------- */
    var pop = null;
    function closePop() {
      if (!pop) return;
      pop.remove(); pop = null;
      document.removeEventListener('pointerdown', popOff, true);
    }
    function popOff(e) { if (pop && !pop.contains(e.target) && !e.target.closest('[data-czs-what]')) closePop(); }
    function openChanges(anchor) {
      if (pop) { closePop(); return; }
      var ls = labels();
      // With only a redo waiting there is no list to show.
      if (!ls.length) return;
      pop = document.createElement('div');
      pop.className = 'czs-pop';
      pop.setAttribute('role', 'dialog');
      pop.setAttribute('aria-label', 'Changes waiting');
      var fail = {};
      failed.forEach(function (f) { fail[f.key] = f.message; });
      pop.innerHTML = '<div class="czs-changes"><h3>Waiting to be saved</h3>' + ls.map(function (l) {
        return '<a href="' + esc(l.href || '#') + '"><i class="fa-solid fa-circle-dot" aria-hidden="true"></i><span>' + esc(l.label) +
          (fail[l.key] ? '<span class="czs-fail">Not saved: ' + esc(fail[l.key]) + '</span>' : '') + '</span></a>';
      }).join('') + '</div>';
      document.body.appendChild(pop);
      var r = anchor.getBoundingClientRect();
      pop.style.left = Math.max(12, Math.min(r.left, innerWidth - pop.offsetWidth - 12)) + 'px';
      pop.style.top = Math.max(12, r.top - pop.offsetHeight - 10) + 'px';
      pop.style.transformOrigin = 'bottom left';
      setTimeout(function () { document.addEventListener('pointerdown', popOff, true); }, 0);
      var first = pop.querySelector('a');
      if (first) first.focus();
      pop.addEventListener('keydown', function (e) { if (e.key === 'Escape') { closePop(); anchor.focus(); } });
    }

    /* ---------- Find a setting ---------- */
    // The field itself is shared with the Manage desk (find_palette.js); this
    // frame only says what it may find and when "/" is its own.
    var palette = null;
    function openFind() {
      if (palette || !Chronicle.findPalette) return;
      palette = Chronicle.findPalette.open({
        finds: finds,
        label: 'Find a setting',
        placeholder: 'Find a setting, like \u201cfont\u201d or \u201cPlaces\u201d',
        onClose: function () { palette = null; }
      });
    }

    /* ---------- The wall ----------
     * Each miniature is drawn 720 wide and scaled to its tile. Where the
     * browser can divide lengths the CSS does it from the tile; elsewhere the
     * width is read here. Hovering or focusing a shared picture lights the part
     * it changes in every other picture (touch has no hover). */
    var wall = $('[data-czw]'), picRO = null;
    if (wall) {
      var cssFit = window.CSS && CSS.supports && CSS.supports('transform', 'scale(calc(10px / 2px))');
      if (!cssFit) {
        var fit = function (pic) {
          var m = pic.querySelector('.czw-mini');
          if (m && pic.clientWidth) m.style.setProperty('--s', (pic.clientWidth / 720).toFixed(4));
        };
        var pics = Array.prototype.slice.call(wall.querySelectorAll('[data-czw-pic]'));
        pics.forEach(fit);
        if (window.ResizeObserver) {
          picRO = new ResizeObserver(function (es) { es.forEach(function (x) { fit(x.target); }); });
          pics.forEach(function (p) { picRO.observe(p); });
        }
      }
      var lit = function (e, onTile) {
        if (e.pointerType === 'touch') return;
        var t = e.target.closest && e.target.closest('.czw-tile[data-czw-shares]');
        if (!t) return;
        if (onTile) wall.setAttribute('data-hl', t.getAttribute('data-czw-shares')); else wall.removeAttribute('data-hl');
      };
      on(wall, 'pointerover', function (e) { lit(e, true); });
      on(wall, 'pointerout', function (e) { lit(e, false); });
      on(wall, 'focusin', function (e) { lit(e, true); });
      on(wall, 'focusout', function (e) { lit(e, false); });
    }

    /* ---------- Wiring ---------- */
    on(bar, 'click', function (e) {
      var b = e.target.closest('button');
      if (!b) return;
      if (b.hasAttribute('data-czs-undo')) undo();
      else if (b.hasAttribute('data-czs-redo')) redo();
      else if (b.hasAttribute('data-czs-discard')) discard();
      else if (b.hasAttribute('data-czs-save')) { if (b.hasAttribute('data-reload')) location.reload(); else publish(); }
      else if (b.hasAttribute('data-czs-what')) openChanges(b);
    });
    on($('[data-czs-find]'), 'click', openFind);

    /* ---------- The bar and the category drawer ----------
     * A category's name drapes the drawer of every category down from the
     * bar. Escape, the drawer's Close and a click outside put it away. */
    var topBar = $('[data-czs-top]'), openDr = null;
    // Content that pins itself under the bar (the Look's example) needs its height.
    var topRO = null;
    function setTopH() { if (topBar && topBar.offsetHeight) { el.style.setProperty('--czs-top-h', topBar.offsetHeight + 'px'); lastBar = { w: window.innerWidth, h: topBar.offsetHeight }; } }
    setTopH();
    if (topBar && window.ResizeObserver) { topRO = new ResizeObserver(setTopH); topRO.observe(topBar); }
    on(window, 'resize', setTopH);
    // On a narrow screen the tabs scroll; start with the open area's in view.
    // A category's bar keeps its name in view when it can, and otherwise
    // starts at a whole part, so no word is cut at its left edge. The page
    // layout calls it again once its picker has narrowed the row.
    function revealTab() {
      // On a phone the tabs have a row of their own, which is what scrolls.
      var row = topBar && topBar.querySelector('.czs-tabrow');
      var strip = row && row.clientWidth ? row : topBar && topBar.querySelector('.czs-tabs-top'), cur = strip && strip.querySelector('.czs-tab[aria-current]');
      if (!cur || strip.scrollWidth <= strip.clientWidth) return;
      var w = cur.closest('.czs-tabw') || cur, x0 = strip.offsetLeft;
      if (strip === row) { strip.scrollLeft = Math.max(0, w.offsetLeft - x0 - 12); return; }
      if (!strip.hasAttribute('data-czs-catbar')) { strip.scrollLeft = Math.max(0, w.offsetLeft - x0 - 12); return; }
      // The row lands with a whole item at its left edge. Near the end the
      // row could not scroll that far, so a tail of room is added after the
      // last item rather than leaving a cut-off word at the left.
      var right = w.offsetLeft - x0 + w.offsetWidth;
      var parts = Array.prototype.slice.call(strip.querySelectorAll(':scope > .czs-back-all, :scope > .czs-tabw, :scope > .czs-tabrow > *'));
      // "Everything" stays pinned at the row's start, so the landing leaves
      // room for it and the first item is the one after it.
      var pin = strip.querySelector('.czs-back-all'), pinW = pin ? pin.offsetWidth : 0, first = pin ? 1 : 0;
      for (var i = first; i < parts.length; i++) {
        var left = Math.max(0, parts[i].offsetLeft - x0);
        if (right - left <= strip.clientWidth - pinW || parts[i] === w) {
          // Measured from the items, not scrollWidth, which can still hold
          // the room the last call added.
          var end = parts[parts.length - 1], at = Math.max(0, left - pinW);
          var tail = Math.max(0, at - (end.offsetLeft - x0 + end.offsetWidth - strip.clientWidth));
          strip.style.paddingRight = tail ? tail + 'px' : '';
          strip.scrollLeft = at;
          // The new room only counts once a frame has been drawn with it.
          if (tail) requestAnimationFrame(function () { requestAnimationFrame(function () { strip.scrollLeft = at; }); });
          return;
        }
      }
    }
    revealTab();
    // Widths settle once the web fonts arrive; land on a whole item again.
    // Widths settle as fonts and styles arrive, so the row lands again
    // whenever its items change size, until the person moves it themselves.
    // A row that scrolls sideways fades on the side that has more beyond it,
    // so a cut word never reads as the end of the row. Rows are drawn by
    // several scripts, so this watches the frame rather than each row.
    var CUE_ROWS = '.czs-tabs-top, .czs-tabrow, .czs-tabs, .czp .parts-row, .czp .lk-row, .cpe-dock';
    function cueRow(r) {
      var max = r.scrollWidth - r.clientWidth, x = Math.abs(r.scrollLeft);
      r.classList.toggle('cue-l', x > 1);
      r.classList.toggle('cue-r', x < max - 1);
    }
    function cueAll() { Array.prototype.forEach.call(el.querySelectorAll(CUE_ROWS), cueRow); }
    var cueRaf = 0;
    function cueSoon() { if (!cueRaf) cueRaf = requestAnimationFrame(function () { cueRaf = 0; if (alive) cueAll(); }); }
    on(el, 'scroll', function (e) { if (e.target.matches && e.target.matches(CUE_ROWS)) cueRow(e.target); }, true);
    on(window, 'resize', cueSoon);
    var cueMO = window.MutationObserver ? new MutationObserver(cueSoon) : null;
    if (cueMO) cueMO.observe(el, { childList: true, subtree: true });
    cueSoon();

    var rowRO = null, rowStrip = topBar && topBar.querySelector('[data-czs-catbar]');
    if (rowStrip && window.ResizeObserver) {
      rowRO = new ResizeObserver(function () { if (alive && rowRO) revealTab(); });
      rowRO.observe(rowStrip);
      Array.prototype.forEach.call(rowStrip.querySelectorAll(':scope > *, .czs-tabrow > *'), function (k) { rowRO.observe(k); });
      var settle = function () { if (rowRO) { rowRO.disconnect(); rowRO = null; } };
      ['pointerdown', 'wheel', 'touchstart', 'keydown'].forEach(function (t) { on(rowStrip, t, settle, { passive: true }); });
    }
    Chronicle.customizeRevealTab = revealTab;
    function drawerFor(id) { return $('[data-czs-drawer-for="' + id + '"]'); }
    function chevFor(id) { return $('[data-czs-drawer="' + id + '"]'); }
    function placeDrawer(id) {
      var d = drawerFor(id), w = chevFor(id) && chevFor(id).closest('.czs-tabw');
      if (!d || !w) return;
      var left = w.getBoundingClientRect().left - topBar.getBoundingClientRect().left;
      d.style.left = Math.max(8, Math.min(left, topBar.clientWidth - d.offsetWidth - 8)) + 'px';
    }
    function closeDrawer(focus) {
      if (!openDr) return;
      var id = openDr;
      openDr = null;
      var d = drawerFor(id), c = chevFor(id);
      if (d) d.hidden = true;
      if (c) { c.setAttribute('aria-expanded', 'false'); if (focus) c.focus(); }
    }
    function openDrawer(id) {
      if (openDr === id) { closeDrawer(true); return; }
      closeDrawer(false);
      var d = drawerFor(id), c = chevFor(id);
      if (!d || !c) return;
      closePop();
      openDr = id;
      d.hidden = false;
      c.setAttribute('aria-expanded', 'true');
      placeDrawer(id);
      var first = d.querySelector('a[aria-current="page"]') || d.querySelector('a');
      if (first) first.focus();
    }
    if (topBar) {
      // Capture, so a Look tab changes the address's # before htmx would
      // follow the link.
      on(topBar, 'click', function (e) {
        var c = e.target.closest('[data-czs-drawer]');
        if (c) { e.preventDefault(); openDrawer(c.getAttribute('data-czs-drawer')); return; }
        if (e.target.closest('[data-czs-drawer-close]')) { e.preventDefault(); closeDrawer(true); return; }
        // A part on this same page (a Look room) changes the address's #
        // only, which the area follows.
        var a = e.target.closest('.czs-drawer a, .czs-tab[data-czs-room]');
        if (a && a.hash && a.pathname === location.pathname) {
          e.preventDefault(); e.stopPropagation();
          closeDrawer(false);
          if (location.hash === a.hash) window.dispatchEvent(new HashChangeEvent('hashchange'));
          else location.hash = a.hash;
          return;
        }
        if (a && a.closest('.czs-drawer')) closeDrawer(false);
      }, true);
      on(document, 'pointerdown', function (e) {
        if (openDr && !e.target.closest('.czs-drawer, [data-czs-drawer]')) closeDrawer(false);
      }, true);
      on(document, 'keydown', function (e) {
        if (e.key === 'Escape' && openDr) { e.preventDefault(); closeDrawer(true); }
      });
      on(window, 'resize', function () { if (openDr) placeDrawer(openDr); });
      // An area that keeps its open part in the page (the Look's rooms)
      // names it, and which of its parts hold unsaved changes: its tabs in the
      // bar carry the open mark and the dots.
      on(document, 'chronicle:customize-part', function (e) {
        var dt = (e && e.detail) || {};
        Array.prototype.forEach.call(topBar.querySelectorAll('.czs-tab[data-czs-room]'), function (a) {
          var k = a.getAttribute('data-czs-room');
          if (k === dt.key) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
          var dot = a.querySelector('.czs-dot');
          if (dot) dot.hidden = !(dt.changed && dt.changed.indexOf(k) >= 0);
        });
        revealTab();
      });
    }
    on(document, 'keydown', function (e) {
      if (e.defaultPrevented || palette) return;
      var mod = e.ctrlKey || e.metaKey;
      if (!mod && !e.altKey && e.key === '/' && !typing(e.target)) { e.preventDefault(); openFind(); return; }
      if (!mod || typing(e.target)) return;
      var k = e.key.toLowerCase();
      if (k === 'z' && !e.shiftKey) { if (past.length) { e.preventDefault(); undo(); } }
      else if ((k === 'z' && e.shiftKey) || k === 'y') { if (future.length) { e.preventDefault(); redo(); } }
    });
    // Ctrl+S on a page with nothing else to save (keyboard_shortcuts.js).
    on(window, 'chronicle:save', publish);
    on(window, 'pagehide', function () { if (rev !== sentRev) send(true); });

    var api_ = {
      read: function () { return parse(snapshot(), {}); }, set: set, flush: function () { return send(false); },
      undo: undo, redo: redo, toast: toast,
      history: function () { return { undo: past.length > 0 && !publishing, redo: future.length > 0 && !publishing }; }
    };
    active = api_;
    Chronicle._customizeQueue.splice(0).forEach(function (q) { set(q[0], q[1], q[2]); });
    paint();
    if (location.hash === '#find') openFind();

    return function destroy() {
      alive = false;
      if (active === api_) active = null;
      // Whatever has not reached the server goes now, even as the page
      // changes under it.
      if (rev !== sentRev && problem !== 'conflict') send(true);
      clearTimeout(saveT); clearTimeout(retryT);
      closePop();
      if (palette) { palette.remove(); palette = null; }
      listeners.forEach(function (l) { l[0].removeEventListener(l[1], l[2], l[3]); });
      if (rowRO) { rowRO.disconnect(); rowRO = null; }
      if (topRO) { topRO.disconnect(); topRO = null; }
      if (cueMO) cueMO.disconnect();
      if (picRO) picRO.disconnect();
    };
  }

  Chronicle.register('customize-shell', {
    init: function (el) { el._czsDestroy = mount(el); },
    destroy: function (el) {
      if (el._czsDestroy) el._czsDestroy();
      el._czsDestroy = null;
    }
  });
})();
