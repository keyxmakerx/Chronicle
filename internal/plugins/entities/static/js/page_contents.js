/**
 * Contents
 *
 * The entity page's "Contents" block (internal/plugins/entities/
 * contents_block.templ) lists the page's own headings as jump links. On a
 * page with room it is a rail in the left margin of the page area: an icon
 * over a strip of marks, one mark per section, the current one lit. On a
 * narrow page it is a labelled "Contents" button. Either opens the same panel,
 * a sheet that slides out over the page text with every heading in it.
 *
 * Room is measured, not guessed. The page area (#main-content) sits to the
 * right of the navigation sidebar, which is open, folded to icons or hidden,
 * and changes width while it slides. The rail lives in a gutter this file
 * reserves on the left of that page area, never left of the sidebar's right
 * edge, and it gives way to the button when the page area is too narrow for
 * the gutter plus readable text (a phone, or a small window with the sidebar
 * open). placement() is the one rule; it is unit-tested.
 *
 * Headings come from the rendered page text, so a viewer is only ever offered
 * what their page already shows (the server leaves Director-only text out of a
 * player's page). Fewer than four headings means the block draws nothing.
 * Text reaches the DOM only through textContent. Motion follows
 * html[data-motion] (calm fades, off shows the end state) and a device asking
 * for reduced motion is always off.
 *
 * Handlers are inline IIFEs (the block can arrive by an HTMX swap), so this
 * exposes window.ChronicleContents and adds no delegated listener. The
 * listeners it does add (scroll, resize, observers, and Escape while the panel
 * is open) are on the page area or the window and are removed in destroy.
 */
(function () {
  'use strict';

  var MIN_HEADINGS = 4;       // a shorter page gets no Contents
  var RAIL_W = 32;            // the strip and the icon
  var RAIL_PAD = 8;           // space between the page area's edge and the rail
  var GUTTER = RAIL_W + RAIL_PAD; // reserved on the page text's left in rail mode
  var MIN_TEXT = 520;         // text width the page keeps beside the gutter
  var PHONE_MAX = 767;        // under this the sidebar is off-canvas, so: button
  var PANEL_W = 300;
  var ICON_H = 44;            // the strip starts under the icon
  var ALIGN_LIMIT = 12;       // up to this many names sit level with their marks
  var FIND_FROM = 12;         // the panel offers Find above this many headings
  var READ_LINE = 60;         // how far below the top a heading counts as "being read"
  var PEEK_MS = 200;
  var EASE = 'cubic-bezier(.16,1,.3,1)';

  // ---- Pure helpers (unit-tested in test/js/page_contents.test.mjs) ----

  /** motionOf: how much motion the page allows ('full' | 'calm' | 'off'). */
  function motionOf(attr, reduced) {
    if (reduced) return 'off';
    return attr === 'calm' || attr === 'off' ? attr : 'full';
  }

  /**
   * collectHeadings: the entries of the Contents from heading-like items
   * ({level, text, hidden, dm}). A heading with no words is skipped (a
   * heading that was all Director-only text arrives empty for a player), and
   * so is one that is not on screen (another tab, a folded block, the player
   * view hiding Director text). Each entry keeps its place in `items`.
   */
  function collectHeadings(items) {
    var out = [];
    for (var i = 0; i < items.length; i++) {
      var it = items[i];
      if (!it || it.hidden) continue;
      var text = String(it.text || '').replace(/\s+/g, ' ').trim();
      if (!text) continue;
      out.push({ index: i, level: it.level, text: text, dm: !!it.dm });
    }
    return out;
  }

  /** shouldShow: a page needs at least four headings to get a Contents. */
  function shouldShow(entries) {
    return entries.length >= MIN_HEADINGS;
  }

  /** signature: changes when the list of names changes, so a rebuild is only done then. */
  function signature(entries) {
    return entries.map(function (e) { return (e.dm ? '!' : '') + e.text; }).join('\n');
  }

  /**
   * activeIndex: the section being read. `tops` are the headings' tops
   * relative to the top of the scrolling area. The last heading at or above
   * the read line wins; before the first heading it is the first; at the very
   * end of the page it is the last, even when the last section is too short to
   * reach the line.
   */
  function activeIndex(tops, atEnd) {
    if (!tops.length) return -1;
    if (atEnd) return tops.length - 1;
    var cur = 0;
    for (var i = 0; i < tops.length; i++) if (tops[i] <= READ_LINE) cur = i;
    return cur;
  }

  /**
   * marksGeometry: where each section's mark sits on a strip. `tops` are the
   * headings' offsets from the top of the whole scrolling content, `total` is
   * that content's height, `end` is where the last section stops, and
   * `stripH` is the strip's height. A mark is at least 3px tall and leaves a
   * 2px gap before the next.
   */
  function marksGeometry(tops, end, total, stripH) {
    var H = Math.max(0, stripH - 12);
    var sc = H / Math.max(1, total);
    return tops.map(function (t, i) {
      var stop = i + 1 < tops.length ? tops[i + 1] : end;
      return { t: 6 + t * sc, h: Math.max(3, (stop - t) * sc - 2) };
    });
  }

  /** markAt: the section a pointer height (from the strip's top) points at. */
  function markAt(geo, y) {
    var r = 0;
    for (var i = 0; i < geo.length; i++) if (geo[i].t - 1 <= y) r = i;
    return r;
  }

  /**
   * alignRows: where each name sits so it is level with its mark, or null when
   * they cannot all fit (then the names are a plain scrolling list and a
   * hovered name lights its mark). Rows are 32px tall and never overlap.
   */
  function alignRows(geo, stripH, phone) {
    if (phone || !geo.length || geo.length > ALIGN_LIMIT) return null;
    var pos = [], prev = -32;
    for (var i = 0; i < geo.length; i++) {
      var p = Math.max(0, geo[i].t + geo[i].h / 2 - 16, prev + 32);
      pos.push(p);
      prev = p;
    }
    return prev + 32 <= stripH + 4 ? pos : null;
  }

  /**
   * placement: rail or button, and where the rail goes. Everything is in
   * viewport pixels. `main` is the page area's rect, `sidebar` the navigation
   * sidebar's rect (null when there is none), `view` the window size.
   *
   * The rail's left edge is the page area's left edge plus a small pad, and
   * never less than the sidebar's right edge plus the same pad, so it cannot
   * overlap the sidebar whatever width the sidebar is in or is sliding
   * through. When the page area cannot hold the gutter and MIN_TEXT of text
   * the answer is the button. Under PHONE_MAX the sidebar is off-canvas and
   * the answer is always the button.
   */
  function placement(main, sidebar, view) {
    var sbRight = sidebar && sidebar.width > 0 && sidebar.right > 0 ? sidebar.right : 0;
    var panelLeft = Math.max(main.left, sbRight);
    var top = Math.max(main.top, 0);
    var bottom = Math.min(main.bottom, view.height);
    var out = {
      mode: 'button',
      railLeft: panelLeft + RAIL_PAD,
      panelLeft: panelLeft,
      top: top,
      height: Math.max(0, bottom - top),
      panelWidth: Math.max(0, Math.min(PANEL_W, main.right - panelLeft))
    };
    if (view.width <= PHONE_MAX) return out;
    var room = main.right - panelLeft;
    if (room - GUTTER < MIN_TEXT) return out;
    out.mode = 'rail';
    return out;
  }

  /** matchRows: which rows a Find query keeps (all of them for an empty query). */
  function matchRows(entries, query) {
    var q = String(query || '').trim().toLowerCase();
    return entries.map(function (e) { return !q || e.text.toLowerCase().indexOf(q) >= 0; });
  }

  /** tipText: the rail's tip for a section: its name and where it is in the list. */
  function tipText(entry, i, n) {
    return { name: entry.text, dm: entry.dm, count: (i + 1) + ' of ' + n };
  }

  /** escapeShouldClose: Escape closes, unless a modifier is held. */
  function escapeShouldClose(e) {
    if (!e || e.key !== 'Escape') return false;
    return !(e.ctrlKey || e.metaKey || e.altKey);
  }

  /** stepKey: the strip's new highlighted section for a key, or -1 if the key is not one of its own. */
  function stepKey(key, cur, n) {
    if (!n) return -1;
    if (key === 'Home') return 0;
    if (key === 'End') return n - 1;
    if (key === 'ArrowDown') return Math.min(n - 1, cur + 1);
    if (key === 'ArrowUp') return Math.max(0, cur - 1);
    return -1;
  }

  var api = {
    motionOf: motionOf,
    collectHeadings: collectHeadings,
    shouldShow: shouldShow,
    signature: signature,
    activeIndex: activeIndex,
    marksGeometry: marksGeometry,
    markAt: markAt,
    alignRows: alignRows,
    placement: placement,
    matchRows: matchRows,
    tipText: tipText,
    escapeShouldClose: escapeShouldClose,
    stepKey: stepKey,
    MIN_HEADINGS: MIN_HEADINGS,
    MIN_TEXT: MIN_TEXT,
    GUTTER: GUTTER
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof window === 'undefined' || typeof document === 'undefined') return;

  // ---- The block ----

  function level() {
    var reduced = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    return motionOf(document.documentElement.getAttribute('data-motion'), reduced);
  }
  function $(root, sel) { return root.querySelector(sel); }
  function rootOf(el) { return el && el.closest ? el.closest('.ct') : null; }
  function stateOf(root) { return root && root.__ct; }
  function bodyOf(root) { return root.parentNode && root.closest ? root.closest('#main-content') : null; }

  // The heading elements of the page's text. Only the rich-text editor's own
  // headings count: the page's other blocks have headings of their own that
  // are not part of the story.
  function pageHeadings(main) {
    return [].slice.call(main.querySelectorAll('[data-widget="editor"] h1, [data-widget="editor"] h2, [data-widget="editor"] h3'));
  }
  function itemOf(h) {
    var secret = h.querySelector('[data-secret]');
    var dm = !!(h.closest('[data-dm-only]') || (secret && (secret.textContent || '').trim() === (h.textContent || '').trim()));
    return {
      level: Number(h.tagName.charAt(1)),
      text: h.textContent,
      hidden: !h.getClientRects().length,
      dm: dm
    };
  }

  function init(root) {
    if (root.__ct) return;
    var shell = $(root, 'template[data-ct-shell]');
    if (!shell || !shell.content) return;
    root.appendChild(shell.content.cloneNode(true));
    var main = bodyOf(root);
    var st = root.__ct = {
      root: root, main: main, heads: [], entries: [], sig: null, geo: [], active: -1,
      mode: 'off', open: false, lock: 0, kb: -1, peekT: 0, icoT: 0, pending: 0,
      rows: [], marks: [], obs: [], onScroll: null, onResize: null, onKey: null
    };
    st.rail = $(root, '.ct-rail');
    st.strip = $(root, '.ct-strip');
    st.lit = $(root, '.ct-lit');
    st.tip = $(root, '.ct-tip');
    st.ptip = $(root, '.ct-ptip');
    st.cico = $(root, '.ct-cico');
    st.btn = $(root, '.ct-btn');
    st.pn = $(root, '.ct-pn');
    st.pl = $(root, '.ct-pl');
    st.fld = $(root, '.ct-fld');
    st.empty = $(root, '.ct-empty');
    // No page area (a page outside the app shell) still gets the button.
    st.sidebar = document.getElementById('sidebar');

    var schedule = function () {
      if (st.pending) clearTimeout(st.pending);
      st.pending = setTimeout(function () { st.pending = 0; refresh(st); }, 250);
    };
    st.schedule = schedule;

    if (main) {
      // The page text arrives after the page (the editor fetches it), and tabs
      // and the player view show and hide whole blocks, so watch for both. The
      // block's own changes are ignored, or it would answer itself.
      var mo = new MutationObserver(function (list) {
        for (var i = 0; i < list.length; i++) if (!root.contains(list[i].target)) { schedule(); return; }
      });
      mo.observe(main, { childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ['hidden', 'class'] });
      st.obs.push(mo);
      var po = new MutationObserver(schedule);
      po.observe(document.documentElement, { attributes: true, attributeFilter: ['data-view-as-player'] });
      st.obs.push(po);
      if (window.ResizeObserver) {
        var ro = new ResizeObserver(function () { relayout(st); });
        ro.observe(main);
        if (st.sidebar) ro.observe(st.sidebar);
        st.obs.push(ro);
      }
      st.onScroll = function () { if (Date.now() >= st.lock) setActive(st, derive(st), false); };
      main.addEventListener('scroll', st.onScroll, { passive: true });
    }
    st.onResize = function () { relayout(st); };
    window.addEventListener('resize', st.onResize);
    refresh(st);
  }

  function destroy(root) {
    var st = stateOf(root);
    if (!st) return;
    st.obs.forEach(function (o) { o.disconnect(); });
    if (st.pending) clearTimeout(st.pending);
    clearTimeout(st.peekT);
    clearTimeout(st.icoT);
    if (st.main && st.onScroll) st.main.removeEventListener('scroll', st.onScroll);
    window.removeEventListener('resize', st.onResize);
    if (st.onKey) document.removeEventListener('keydown', st.onKey);
    if (st.main) st.main.classList.remove('ct-railed');
    root.__ct = null;
  }

  // refresh re-reads the headings; the list and marks are rebuilt only when the
  // names changed, the geometry always.
  function refresh(st) {
    if (!st.root.isConnected) return;
    var heads = st.main ? pageHeadings(st.main) : [];
    var entries = collectHeadings(heads.map(itemOf));
    var show = shouldShow(entries);
    var sig = show ? signature(entries) : '';
    st.heads = entries.map(function (e) { return heads[e.index]; });
    st.entries = entries;
    if (show && sig !== st.sig) buildLists(st);
    st.sig = sig;
    st.visible = show;
    if (!show && st.open) closePanel(st, false);
    relayout(st);
  }

  function buildLists(st) {
    [].forEach.call(st.strip.querySelectorAll('.ct-mk'), function (m) { m.parentNode.removeChild(m); });
    st.marks = st.entries.map(function (e) {
      var m = document.createElement('span');
      m.className = 'ct-mk' + (e.dm ? ' ct-dm' : '');
      st.strip.insertBefore(m, st.lit);
      return m;
    });
    st.pl.textContent = '';
    st.rows = st.entries.map(function (e, i) {
      var li = document.createElement('li');
      var a = document.createElement('button');
      a.type = 'button';
      a.className = 'ct-row' + (e.dm ? ' ct-dm' : '');
      a.setAttribute('data-i', String(i));
      var pm = document.createElement('span');
      pm.className = 'ct-pm';
      var nm = document.createElement('span');
      nm.className = 'ct-nm';
      nm.textContent = e.text;
      a.appendChild(pm);
      a.appendChild(nm);
      if (e.dm) {
        var tag = document.createElement('span');
        tag.className = 'ct-dmtag';
        tag.textContent = 'DM';
        a.appendChild(tag);
      }
      a.addEventListener('click', function () { go(st, i, true); });
      a.addEventListener('mouseover', function () { hoverMark(st, i); });
      a.addEventListener('mouseout', function () { hoverMark(st, -1); });
      li.appendChild(a);
      st.pl.appendChild(li);
      return li;
    });
    st.pn.classList.toggle('ct-many', st.entries.length > FIND_FROM);
    st.fld.value = '';
    filterRows(st);
    st.active = -1;
  }

  function hoverMark(st, i) {
    st.marks.forEach(function (m, k) { m.classList.toggle('ct-hov', k === i); });
  }

  function filterRows(st) {
    var keep = matchRows(st.entries, st.fld.value), any = false;
    st.rows.forEach(function (li, i) { li.hidden = !keep[i]; if (keep[i]) any = true; });
    st.empty.hidden = any || !st.entries.length;
  }

  // The window the fixed pieces are measured against: the page area, or the
  // whole window when there is none.
  function areaRect(st) {
    if (st.main) return st.main.getBoundingClientRect();
    return { left: 0, right: window.innerWidth, top: 0, bottom: window.innerHeight, width: window.innerWidth };
  }

  // fixedAt puts a position:fixed piece at viewport coordinates even when an
  // ancestor (a transformed page transition) makes fixed positioning relative
  // to itself: it measures where (0,0) really is and compensates.
  function fixedAt(el, left, top) {
    el.style.left = '0px';
    el.style.top = '0px';
    var r = el.getBoundingClientRect();
    el.style.left = (left - r.left) + 'px';
    el.style.top = (top - r.top) + 'px';
  }

  function relayout(st) {
    if (!st.root.isConnected) return;
    var root = st.root;
    if (!st.visible) {
      setMode(st, 'off');
      return;
    }
    var area = areaRect(st);
    var sb = st.sidebar && st.sidebar.getBoundingClientRect();
    var p = placement(area, sb || null, { width: window.innerWidth, height: window.innerHeight });
    if (!st.main) p.mode = 'button';
    st.place = p;
    setMode(st, p.mode);
    if (p.mode === 'rail') {
      st.rail.style.height = p.height + 'px';
      fixedAt(st.rail, p.railLeft, p.top);
      st.pn.style.height = p.height + 'px';
    } else {
      // A button-mode panel hangs from the top of the page area and is as tall
      // as what is on screen.
      st.pn.style.height = p.height + 'px';
    }
    st.pn.style.width = p.panelWidth + 'px';
    fixedAt(st.pn, p.panelLeft, p.top);
    // Geometry of the marks, against the whole scrolling content.
    if (p.mode === 'rail') measureMarks(st);
    setActive(st, derive(st), true);
  }

  function setMode(st, mode) {
    var root = st.root;
    if (mode !== st.mode) {
      st.mode = mode;
      root.setAttribute('data-ct', mode);
      if (mode !== 'rail' && st.open) { /* the panel stays open across a mode flip */ }
    }
    // The gutter is held while a rail is wanted, so the text does not move
    // under the reader when the list is rebuilt.
    if (st.main) st.main.classList.toggle('ct-railed', mode === 'rail');
    if (st.cico) st.cico.tabIndex = mode === 'rail' ? 0 : -1;
    if (st.strip) st.strip.tabIndex = mode === 'rail' ? 0 : -1;
    if (mode === 'off' && st.open) closePanel(st, false);
  }

  function contentTops(st) {
    var base = st.main ? st.main.getBoundingClientRect().top - st.main.scrollTop : 0;
    return st.heads.map(function (h) { return h.getBoundingClientRect().top - base; });
  }

  function measureMarks(st) {
    var tops = contentTops(st);
    var total = st.main ? st.main.scrollHeight : document.documentElement.scrollHeight;
    var story = st.heads.length ? st.heads[st.heads.length - 1].closest('[data-widget="editor"]') : null;
    var base = st.main ? st.main.getBoundingClientRect().top - st.main.scrollTop : 0;
    var end = story ? story.getBoundingClientRect().bottom - base : total;
    var stripH = Math.max(0, st.place.height - ICON_H);
    st.geo = marksGeometry(tops, Math.max(end, tops[tops.length - 1] + 1), total, stripH);
    st.marks.forEach(function (m, i) {
      if (!st.geo[i]) return;
      m.style.top = st.geo[i].t + 'px';
      m.style.height = st.geo[i].h + 'px';
    });
    var pos = alignRows(st.geo, stripH, false);
    st.pl.classList.toggle('ct-al', !!pos);
    st.rows.forEach(function (li, i) { li.style.top = pos ? pos[i] + 'px' : ''; });
    placeLit(st, true);
  }

  function derive(st) {
    if (!st.heads.length || !st.main) return st.heads.length ? 0 : -1;
    var base = st.main.getBoundingClientRect().top;
    var tops = st.heads.map(function (h) { return h.getBoundingClientRect().top - base; });
    var atEnd = st.main.scrollTop + st.main.clientHeight >= st.main.scrollHeight - 2;
    return activeIndex(tops, atEnd);
  }

  function placeLit(st, instant) {
    var g = st.geo[st.active];
    if (!g || st.mode !== 'rail') { st.lit.style.opacity = '0'; return; }
    st.lit.classList.toggle('ct-dm', !!st.entries[st.active].dm);
    function put() {
      st.lit.style.top = g.t + 'px';
      st.lit.style.height = g.h + 'px';
      st.lit.style.opacity = '1';
    }
    if (instant) {
      st.lit.style.transition = 'none';
      put();
      void st.lit.offsetWidth;
      st.lit.style.transition = '';
      return;
    }
    if (level() === 'calm') { st.lit.style.opacity = '0'; setTimeout(put, 130); } else put();
  }

  function setActive(st, i, instant) {
    if (i < 0) { st.active = -1; return; }
    var changed = i !== st.active;
    st.active = i;
    if (changed || instant) placeLit(st, instant);
    var prev = st.pl.querySelector('.ct-sel');
    if (prev) { prev.classList.remove('ct-sel'); prev.removeAttribute('aria-current'); }
    var cur = st.rows[i] && st.rows[i].firstChild;
    if (cur) { cur.classList.add('ct-sel'); cur.setAttribute('aria-current', 'location'); }
  }

  function go(st, i, keepOpen) {
    var h = st.heads[i];
    if (!h || !st.main) return;
    var lv = level();
    var top = h.getBoundingClientRect().top - st.main.getBoundingClientRect().top + st.main.scrollTop - 8;
    st.lock = lv === 'full' ? Date.now() + 900 : 0;
    setActive(st, i, false);
    st.main.scrollTo({ top: top, behavior: lv === 'full' ? 'smooth' : 'auto' });
    // Glow: where you landed (the Glow move; it ends clear, with no change to the page).
    if (lv !== 'off' && h.animate) {
      h.animate([{ backgroundColor: 'rgb(var(--color-accent-rgb, 99 102 241) / .16)' }, { backgroundColor: 'transparent' }], { duration: 1200, easing: 'ease-out' });
    }
    if (!keepOpen) closePanel(st, false);
  }

  // ---- The tip on the rail ----

  function showTip(st, i) {
    var e = st.entries[i], g = st.geo[i];
    if (!e || !g) return;
    hoverMark(st, i);
    if (st.open) return;
    var t = tipText(e, i, st.entries.length);
    st.tip.textContent = '';
    var nm = document.createElement('span');
    nm.textContent = t.name;
    st.tip.appendChild(nm);
    if (t.dm) {
      var tag = document.createElement('span');
      tag.className = 'ct-dmtag';
      tag.textContent = 'DM';
      st.tip.appendChild(tag);
    }
    var small = document.createElement('small');
    small.textContent = t.count;
    st.tip.appendChild(small);
    st.tip.style.top = Math.max(16, Math.min(st.strip.clientHeight - 16, g.t + g.h / 2)) + 'px';
    st.tip.classList.add('ct-on');
  }
  function hideTip(st) {
    clearTimeout(st.peekT);
    st.tip.classList.remove('ct-on');
    hoverMark(st, -1);
  }
  function stripY(st, e) { return e.clientY - st.strip.getBoundingClientRect().top; }

  // ---- The panel ----

  function openPanel(st) {
    if (st.open || !st.visible) return;
    st.open = true;
    hidePeek(st);
    hideTip(st);
    st.pn.classList.add('ct-show');
    st.root.classList.add('ct-isopen');
    [st.cico, st.btn].forEach(function (b) { b.setAttribute('aria-expanded', 'true'); });
    var cur = st.pl.querySelector('.ct-sel');
    if (cur && !st.pl.classList.contains('ct-al')) {
      var li = cur.parentNode;
      st.pl.scrollTop = Math.max(0, li.offsetTop - st.pl.clientHeight / 2 + li.offsetHeight / 2);
    }
    if (level() === 'full') {
      [].forEach.call(st.pl.querySelectorAll('.ct-row'), function (a, i) {
        if (a.animate && i < 16) a.animate([{ opacity: 0, transform: 'translateX(-8px)' }, { opacity: 1, transform: 'none' }], { duration: 240, delay: 90 + i * 22, easing: EASE, fill: 'backwards' });
      });
    }
    st.onKey = function (e) {
      if (escapeShouldClose(e)) { e.stopPropagation(); closePanel(st, true); }
    };
    document.addEventListener('keydown', st.onKey);
    setTimeout(function () {
      if (!st.open) return;
      var target = cur || st.pn.querySelector('.ct-x');
      if (target && target.focus) target.focus({ preventScroll: true });
    }, level() === 'full' ? 60 : 0);
  }

  function closePanel(st, refocus) {
    if (!st.open) return;
    st.open = false;
    st.pn.classList.remove('ct-show');
    st.root.classList.remove('ct-isopen');
    [st.cico, st.btn].forEach(function (b) { b.setAttribute('aria-expanded', 'false'); });
    hoverMark(st, -1);
    if (st.onKey) { document.removeEventListener('keydown', st.onKey); st.onKey = null; }
    if (refocus) {
      var back = st.mode === 'rail' ? st.cico : st.btn;
      if (back && back.focus) back.focus({ preventScroll: true });
    }
  }

  function hidePeek(st) { clearTimeout(st.icoT); st.ptip.classList.remove('ct-on'); }

  // ---- Handlers (called from the block's inline attributes) ----

  var H = {
    toggle: function (el) {
      var st = stateOf(rootOf(el));
      if (!st) return;
      hidePeek(st);
      if (st.open) closePanel(st, true); else openPanel(st);
    },
    close: function (el) { var st = stateOf(rootOf(el)); if (st) closePanel(st, true); },
    panelKey: function (el, e) {
      var st = stateOf(rootOf(el));
      if (st && st.open && escapeShouldClose(e)) { e.stopPropagation(); closePanel(st, true); }
    },
    filter: function (el) { var st = stateOf(rootOf(el)); if (st) filterRows(st); },
    icoEnter: function (el) {
      var st = stateOf(rootOf(el));
      if (!st || st.mode !== 'rail') return;
      clearTimeout(st.icoT);
      st.icoT = setTimeout(function () { if (!st.open) st.ptip.classList.add('ct-on'); }, 250);
    },
    icoFocus: function (el) {
      var st = stateOf(rootOf(el));
      if (st && st.mode === 'rail' && !st.open && el.matches(':focus-visible')) st.ptip.classList.add('ct-on');
    },
    icoLeave: function (el) { var st = stateOf(rootOf(el)); if (st) hidePeek(st); },
    stripMove: function (el, e) {
      var st = stateOf(rootOf(el));
      if (!st || !st.geo.length) return;
      var i = markAt(st.geo, stripY(st, e));
      if (st.open || st.tip.classList.contains('ct-on')) { showTip(st, i); return; }
      clearTimeout(st.peekT);
      st.peekT = setTimeout(function () { showTip(st, i); }, level() === 'off' ? 0 : PEEK_MS);
    },
    stripLeave: function (el) {
      var st = stateOf(rootOf(el));
      if (!st) return;
      if (document.activeElement === st.strip && st.strip.matches(':focus-visible')) return;
      hideTip(st);
    },
    stripClick: function (el, e) {
      var st = stateOf(rootOf(el));
      if (st && st.geo.length) go(st, markAt(st.geo, stripY(st, e)), st.open);
    },
    stripFocus: function (el) {
      var st = stateOf(rootOf(el));
      if (!st || !el.matches(':focus-visible')) return;
      st.kb = Math.max(0, st.active);
      showTip(st, st.kb);
    },
    stripKey: function (el, e) {
      var st = stateOf(rootOf(el));
      if (!st) return;
      var n = stepKey(e.key, st.kb, st.entries.length);
      if (n >= 0) { e.preventDefault(); st.kb = n; showTip(st, n); }
      else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (st.entries[st.kb]) go(st, st.kb, st.open); }
      else if (e.key === 'Escape' && !st.open) hideTip(st);
    }
  };

  window.ChronicleContents = H;

  // The block mounts as a widget so its script loads on sight (ADR-063) and so
  // the page area is released when a swap removes it.
  if (window.Chronicle && window.Chronicle.register) {
    window.Chronicle.register('page-contents', {
      init: function (el) { init(el); },
      destroy: function (el) { destroy(el); }
    });
  }
})();
