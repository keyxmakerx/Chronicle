/**
 * Rumours
 *
 * The entity page's "Rumours" block (internal/plugins/rumours/
 * rumours_block.templ): what the party has heard about a page, who said it,
 * how sure they were, what the Director knows is true and what the party has
 * decided to believe. The Director's truth and the party's belief sit together
 * as one joined mark at the bottom of each rumour.
 *
 * The server decides what a viewer may see and do: the block's JSON holds no
 * truth for anyone but the Director, the Director-only controls are not
 * rendered for anyone else, and every write is re-checked by the service. This
 * file makes no permission decision; it shows the controls the server sent and
 * asks the server for every change. Text reaches the DOM only through
 * textContent. Motion follows html[data-motion] (calm fades, off shows the end
 * state) and a device asking for reduced motion is always off.
 *
 * The controls the server renders carry inline IIFE handlers that call
 * window.ChronicleRumours (the block can arrive by an HTMX swap). Controls this
 * file creates itself (menu items, filters) are wired as they are created.
 * The only document listeners are the Escape key and the click-off for a menu,
 * and they live only while a menu, the Add form or an edit is open.
 */
(function () {
  'use strict';

  var INL = 2; // rumours shown before the rest fold away
  var PAGE = 10; // earlier rumours revealed at a time
  var PAGED_AT = 24; // from this many rumours the earlier ones come in pages
  var QUOTE_MAX = 280;
  var WHO_MAX = 80;
  var SURE = ['Heard it', 'Fairly sure', 'Swears by it'];
  var EASE_OUT = 'cubic-bezier(.16,1,.3,1)';
  var EASE_IN = 'cubic-bezier(.4,0,1,1)';
  var EASE_STD = 'cubic-bezier(.2,0,0,1)';

  // ---- Pure helpers (unit-tested in test/js/rumours.test.mjs) ----

  /** motionOf: how much motion the page allows ('full' | 'calm' | 'off'). */
  function motionOf(attr, reduced) {
    if (reduced) return 'off';
    return attr === 'calm' || attr === 'off' ? attr : 'full';
  }

  /** plural: "1 earlier rumour", "3 earlier rumours". */
  function plural(n, word) { return n + ' ' + word + (n === 1 ? '' : 's'); }

  /** sureWord: the words for a sureness step; out of range reads as the middle. */
  function sureWord(n) { return n >= 1 && n <= 3 && n === Math.floor(n) ? SURE[n - 1] : SURE[1]; }

  /** viewKind: which of the three looks the viewer has, from the flags the server set. */
  function viewKind(dm, mark) { return dm ? 'dm' : mark ? 'scribe' : 'player'; }

  /**
   * filterSet: the filters offered. They name what the viewer can see: the
   * Director's truth, or the party's belief.
   */
  function filterSet(kind) {
    return kind === 'dm'
      ? [['all', 'All'], ['open', 'Open'], ['true', 'True'], ['false', 'False']]
      : [['all', 'All'], ['true', 'Thinks true'], ['false', 'Thinks false'], ['unsure', 'Unsure']];
  }

  /** filterValue: the value of a rumour a filter compares against. */
  function filterValue(kind, it) { return kind === 'dm' ? it.truth || 'open' : it.belief || 'none'; }

  /** paged: whether the earlier rumours come in pages of PAGE. */
  function paged(n) { return n >= PAGED_AT; }

  /**
   * wanted: which rumours are drawn. Folded: the newest INL. Unfolded: the
   * rest too, all at once for a short list, or a page at a time for a long one.
   * A filter only applies unfolded. left is how many matching rumours are not
   * drawn yet.
   */
  function wanted(items, kind, st) {
    var L = items;
    if (st.unfolded && st.filter !== 'all') {
      L = items.filter(function (i) { return filterValue(kind, i) === st.filter; });
    }
    var cap = st.unfolded ? (paged(items.length) ? INL + st.shown : L.length) : INL;
    return { list: L.slice(0, cap), total: L.length, left: Math.max(0, L.length - cap) };
  }

  /** pairState: how the Director's truth compares with the party's belief. */
  function pairState(truth, belief) {
    var t = truth || 'open';
    if (!belief) return 'none';
    if (t !== 'open' && belief !== t) return 'differ';
    if (belief === t) return 'agree';
    return 'marked';
  }

  /** pairTip: the Director's hover line for the joined mark. */
  function pairTip(it) {
    var t = it.truth || 'open';
    return 'You: ' + t + '. ' + (it.belief ? 'Party (marked by ' + (it.beliefBy || 'a player') + '): ' + it.belief + '.' : 'Party: not marked yet.');
  }

  /**
   * tailLabel: the words on the fold's button. mode is 'open' (folded),
   * 'more' (unfolded with more to reveal) or 'close'.
   */
  function tailLabel(mode, o) {
    if (mode === 'open') return plural(o.earlier, 'earlier rumour');
    if (mode === 'more') {
      var base = o.filter === 'all' ? INL : 0;
      var nshow = o.shown - base;
      var ntot = o.total - base;
      var lab = o.filter === 'all' ? 'earlier rumours' : o.filter + ' rumours';
      return nshow + ' of ' + ntot + ' ' + lab + ' · ' + (o.left > PAGE ? 'Show ' + PAGE + ' more (' + o.left + ' left)' : 'Show the last ' + o.left);
    }
    return 'Show fewer';
  }

  /**
   * cleanText: trims, folds inner whitespace runs, and refuses control
   * characters. Returns { text, ok }; ok is false for control characters or a
   * text over max (never cut quietly). Mirrors cleanText in model.go.
   */
  function cleanText(s, max) {
    var str = String(s == null ? '' : s);
    if (/[\u0000-\u0008\u000e-\u001f\u007f-\u009f]/.test(str)) return { text: '', ok: false };
    var t = str.replace(/\s+/g, ' ').replace(/^ | $/g, '');
    return { text: t, ok: Array.from(t).length <= max };
  }

  /** validate: the message to show for a rumour that cannot be saved, or ''. */
  function validate(quote, who) {
    var q = cleanText(quote, QUOTE_MAX);
    if (!q.ok && !q.text) return 'Use plain text for the rumour.';
    if (!q.text) return 'Write what was said first.';
    if (!q.ok) return 'Keep the rumour to ' + QUOTE_MAX + ' characters or fewer.';
    if (!cleanText(who, WHO_MAX).ok) return 'Keep who said it to ' + WHO_MAX + ' characters or fewer, in plain text.';
    return '';
  }

  /** escapeShouldClose: Escape closes, unless a modifier is held. */
  function escapeShouldClose(e) {
    if (!e || e.key !== 'Escape') return false;
    return !(e.ctrlKey || e.metaKey || e.altKey);
  }

  /** replaceItem: the items with one swapped for the server's newer answer. */
  function replaceItem(items, fresh) {
    var out = (items || []).slice();
    for (var i = 0; i < out.length; i++) {
      if (out[i].id === fresh.id) { out[i] = fresh; return out; }
    }
    out.unshift(fresh);
    return out;
  }

  /** dropItem: the items without one. */
  function dropItem(items, id) { return (items || []).filter(function (i) { return i.id !== id; }); }

  /**
   * cutNotice: the line that says a bound cut the list off, or ''. total is
   * how many the page holds, sent how many the server sent.
   */
  function cutNotice(total, sent) {
    return total > sent ? 'Showing the newest ' + sent + ' of ' + total + ' rumours on this page.' : '';
  }

  var api = {
    motionOf: motionOf, plural: plural, sureWord: sureWord, viewKind: viewKind, filterSet: filterSet,
    filterValue: filterValue, paged: paged, wanted: wanted, pairState: pairState, pairTip: pairTip,
    tailLabel: tailLabel, cleanText: cleanText, validate: validate, escapeShouldClose: escapeShouldClose,
    replaceItem: replaceItem, dropItem: dropItem, cutNotice: cutNotice, INL: INL, PAGE: PAGE
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof window === 'undefined' || typeof document === 'undefined') return;

  // ---- Browser part ----

  function level() {
    var h = document.documentElement;
    var reduced = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    var attr = h.getAttribute('data-motion') || h.getAttribute('data-view-motion');
    if (h.hasAttribute('data-cz-reduce') && attr !== 'off') attr = 'calm';
    return motionOf(attr, reduced);
  }

  function rootOf(el) { return el && el.closest ? el.closest('.rm') : null; }
  function $(c, s) { return c.querySelector(s); }
  function $$(c, s) { return Array.prototype.slice.call(c.querySelectorAll(s)); }
  function mk(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function icon(name, extra) {
    var i = mk('i', 'fa-solid fa-' + name + (extra ? ' ' + extra : ''));
    i.setAttribute('aria-hidden', 'true');
    return i;
  }
  function cap(s) { return s.charAt(0).toUpperCase() + s.slice(1); }

  function stateOf(root) {
    if (!root.__rm) {
      var data = { rumours: [] };
      var s = root.querySelector('script[type="application/json"]');
      try { data = JSON.parse(s ? s.textContent : '{}') || data; } catch (e) { /* an unreadable block shows nothing to change */ }
      var dm = root.getAttribute('data-dm') === '1';
      var mark = root.getAttribute('data-mark') === '1';
      root.__rm = {
        items: (data.rumours || []).slice(), total: data.total || 0,
        dm: dm, add: root.getAttribute('data-add') === '1', mark: mark, kind: viewKind(dm, mark),
        unfolded: false, filter: 'all', shown: PAGE, editing: null, busy: false,
        menuOpener: null, menuTok: 0, addOpen: false, popAnim: [], onKey: null, onAway: null
      };
    }
    return root.__rm;
  }

  function say(root, msg) { var l = $(root, '[data-rm-live]'); if (l) l.textContent = msg; }

  // ---- Talking to the server ----

  function http(url, opts) {
    if (window.Chronicle && window.Chronicle.apiFetch) return window.Chronicle.apiFetch(url, opts);
    opts = opts || {};
    return fetch(url, { method: opts.method || 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'Content-Type': 'application/json' }, body: opts.body ? JSON.stringify(opts.body) : undefined });
  }

  function base(root) {
    return '/campaigns/' + encodeURIComponent(root.getAttribute('data-campaign')) + '/entities/' + encodeURIComponent(root.getAttribute('data-entity')) + '/rumours';
  }

  // Resolves with the parsed body (null for 204), or rejects with an Error
  // whose message is the server's own words when it sent any.
  function readJSON(res) {
    if (res.status === 204) return null;
    if (res.ok) return res.json();
    return res.json().then(function (b) { throw new Error((b && b.message) || 'status ' + res.status); },
      function () { throw new Error('status ' + res.status); });
  }

  function friendly(err, fallback) {
    return err && err.message && !/^status /.test(err.message) ? err.message : fallback;
  }

  function byId(st, id) {
    for (var i = 0; i < st.items.length; i++) if (st.items[i].id === id) return st.items[i];
    return null;
  }

  // ---- Rows ----

  function setBelief(node, it) {
    node.textContent = '';
    node.appendChild(document.createTextNode('Party thinks: '));
    node.appendChild(mk('b', null, it.belief || 'unmarked'));
  }

  function fill(root, a, it) {
    var st = stateOf(root);
    a.setAttribute('data-id', it.id);
    a.setAttribute('data-belief', it.belief || '');
    var q = $(a, '.rm-quote');
    q.textContent = it.quote;
    q.title = it.quote;
    $(a, '.rm-who').textContent = it.who;
    $(a, '.rm-when').textContent = it.when;
    $$(a, '.rm-dots i').forEach(function (d, i) { d.className = i < it.sure ? 'on' : ''; });
    $(a, '.rm-sl').textContent = sureWord(it.sure);
    var by = $(a, '.rm-by');
    by.textContent = '';
    if (it.addedBy) {
      var dot = mk('span', null, '·');
      dot.setAttribute('aria-hidden', 'true');
      by.appendChild(dot);
      by.appendChild(mk('span', null, 'added by ' + it.addedBy));
    }
    var marks = $(a, '[data-rm-marks]');
    marks.hidden = !st.mark && !it.belief;
    marks.setAttribute('data-bel', it.belief ? '1' : '0');
    var title = it.belief ? 'Marked by ' + (it.beliefBy || 'a player') : '';
    var bel = $(a, '[data-belief]');
    if (bel) {
      setBelief(bel, it);
      bel.title = it.belief ? title + '. Press to change what the party thinks.' : 'Press to mark what the party thinks.';
    }
    $$(a, '[data-bel-static]').forEach(function (s) { setBelief(s, it); s.hidden = !it.belief; s.title = title; });
    var pair = $(a, '[data-pair]');
    if (pair) {
      var tb = $(pair, '[data-truth]');
      var pb = $(pair, '[data-pbel]');
      var tr = it.truth || 'open';
      tb.setAttribute('data-v', tr);
      tb.textContent = cap(tr);
      var ps = pairState(tr, it.belief);
      pair.setAttribute('data-s', ps);
      pb.textContent = '';
      if (ps === 'none') pb.textContent = 'party: –';
      else if (ps === 'agree') { pb.appendChild(icon('check')); pb.appendChild(mk('span', null, 'agrees')); }
      else pb.textContent = 'party: ' + it.belief;
      tb.setAttribute('aria-label', 'What is true: ' + tr + '. Press to change. Only you see this.');
      pb.setAttribute('aria-label', it.belief
        ? 'Party thinks ' + it.belief + ', marked by ' + (it.beliefBy || 'a player') + '. Press to set for the party.'
        : 'Party has not marked this. Press to set for the party.');
      $(pair, '.rm-pt').textContent = pairTip(it);
    }
    var more = $(a, '[data-more]');
    if (more) {
      more.setAttribute('aria-disabled', it.canEdit ? 'false' : 'true');
      more.tabIndex = it.canEdit ? 0 : -1;
    }
  }

  function newRow(root, it) {
    var tpl = $(root, 'template[data-rm-tpl]');
    var src = tpl && tpl.content ? tpl.content.firstElementChild : null;
    if (!src) return null;
    var a = document.importNode(src, true);
    fill(root, a, it);
    return a;
  }

  // Rows joining the list slide down from behind the row above, and leaving
  // ones fold closed. Calm fades; Off shows the end state.
  function emerge(el, open, i, done) {
    var L = level();
    var d = i * 30;
    function fin() {
      if (open) el.getAnimations({ subtree: true }).forEach(function (x) { x.cancel(); });
      el.style.overflow = '';
      el.style.opacity = '';
      if (!open) el.remove();
      if (done) done();
    }
    if (L === 'off' || !el.animate) { fin(); return; }
    if (L === 'calm') {
      var c = el.animate([{ opacity: open ? 0 : 1 }, { opacity: open ? 1 : 0 }], { duration: open ? 200 : 120, delay: d, easing: 'ease-out', fill: 'both' });
      c.onfinish = fin;
      return;
    }
    var cs = getComputedStyle(el);
    var h = el.offsetHeight;
    var pt = cs.paddingTop;
    var pb = cs.paddingBottom;
    el.style.overflow = 'hidden';
    var A = open ? { height: '0px', opacity: 0, paddingTop: '0px', paddingBottom: '0px' } : { height: h + 'px', opacity: 1, paddingTop: pt, paddingBottom: pb };
    var B = open ? { height: h + 'px', opacity: 1, paddingTop: pt, paddingBottom: pb } : { height: '0px', opacity: 0, paddingTop: '0px', paddingBottom: '0px' };
    var opt = { duration: open ? 200 : 160, delay: d, easing: open ? EASE_OUT : EASE_IN, fill: 'both' };
    var an = el.animate([A, B], opt);
    var hide = 'translateY(-70%)';
    Array.prototype.slice.call(el.children).forEach(function (k) {
      k.animate(open ? [{ transform: hide }, { transform: 'none' }] : [{ transform: 'none' }, { transform: hide }], opt);
    });
    an.onfinish = fin;
  }

  function glow(el) {
    el.classList.add('rm-glow');
    setTimeout(function () { el.classList.remove('rm-glow'); }, 1300);
  }

  // ---- The list: what is drawn, the fold, the filters ----

  function listOf(root) { return $(root, '[data-rm-list]'); }
  function rowFor(root, id) {
    var k = listOf(root).children;
    for (var i = 0; i < k.length; i++) if (k[i].getAttribute('data-id') === id && !k[i].__out) return k[i];
    return null;
  }

  function renderFilters(root) {
    var st = stateOf(root);
    var box = $(root, '[data-rm-filters]');
    box.textContent = '';
    filterSet(st.kind).forEach(function (f) {
      var b = mk('button', null, f[1]);
      b.type = 'button';
      b.setAttribute('data-f', f[0]);
      b.setAttribute('aria-pressed', f[0] === st.filter ? 'true' : 'false');
      b.appendChild(mk('span', 'rm-n', '0'));
      b.addEventListener('click', function () { setFilter(root, f[0]); });
      box.appendChild(b);
    });
  }

  function counts(root) {
    var st = stateOf(root);
    var c = { all: st.items.length };
    st.items.forEach(function (i) { var v = filterValue(st.kind, i); c[v] = (c[v] || 0) + 1; });
    $$(root, '[data-f]').forEach(function (b) { $(b, '.rm-n').textContent = c[b.getAttribute('data-f')] || 0; });
    $(root, '[data-rm-cnt]').textContent = String(Math.max(st.total, st.items.length));
  }

  function tailOf(root) { return $(root, '[data-rm-tail]'); }

  function ensureTail(root) {
    var t = tailOf(root);
    if (t) return t;
    t = mk('div', 'rm-tail');
    t.setAttribute('data-rm-tail', '');
    var open = mk('button', 'rm-quiet');
    open.type = 'button';
    open.setAttribute('data-tail', '');
    open.setAttribute('aria-controls', listOf(root).id);
    open.addEventListener('click', function () { tail(open); });
    var few = mk('button', 'rm-quiet');
    few.type = 'button';
    few.setAttribute('data-fewer', '');
    few.hidden = true;
    few.appendChild(document.createTextNode('Show fewer'));
    few.appendChild(icon('chevron-down', 'rm-chev rm-up'));
    few.addEventListener('click', function () { fewer(few); });
    t.appendChild(open);
    t.appendChild(few);
    return t;
  }

  function sync(root, instant) {
    var st = stateOf(root);
    var list = listOf(root);
    var w = wanted(st.items, st.kind, st);
    var keep = {};
    var gone = [];
    var born = [];
    w.list.forEach(function (it) { keep[it.id] = true; });
    $$(list, '.rm-rum').forEach(function (r) {
      if (r.__out || keep[r.getAttribute('data-id')]) return;
      if (instant) { r.remove(); return; }
      r.__out = true;
      gone.push(r);
    });
    var emp = $(list, '.rm-empty');
    if (emp) emp.remove();
    var tail = ensureTail(root);
    // The fold sits in the list before anything is placed relative to it.
    list.appendChild(tail);
    var prev = null;
    w.list.forEach(function (it) {
      var r = rowFor(root, it.id);
      if (!r) {
        r = newRow(root, it);
        if (!r) return;
        var ref = prev ? prev.nextSibling : list.firstChild;
        while (ref && ref.__out && ref !== tail) ref = ref.nextSibling;
        list.insertBefore(r, ref || null);
        born.push(r);
      }
      prev = r;
    });
    if (!w.list.length) {
      emp = mk('div', 'rm-empty', st.items.length ? 'No rumours here.' : 'No rumours here yet.');
      list.insertBefore(emp, tail);
    }
    list.appendChild(tail);
    var canFold = st.items.length > INL;
    tail.hidden = !canFold;
    var more = st.unfolded && w.left > 0;
    var tb = $(tail, '[data-tail]');
    var fb = $(tail, '[data-fewer]');
    var mode = !st.unfolded ? 'open' : more ? 'more' : 'close';
    tb.setAttribute('data-mode', mode);
    tb.setAttribute('aria-expanded', st.unfolded ? 'true' : 'false');
    fb.hidden = !more;
    var chev = icon('chevron-down', 'rm-chev');
    tb.textContent = '';
    if (mode === 'more') {
      var label = tailLabel('more', { filter: st.filter, shown: w.list.length, total: w.total, left: w.left });
      var parts = label.split(' · ');
      tb.appendChild(mk('span', 'rm-st', parts[0]));
      tb.appendChild(mk('span', 'rm-sep', '·')).setAttribute('aria-hidden', 'true');
      tb.appendChild(mk('span', null, parts[1]));
    } else {
      tb.appendChild(document.createTextNode(tailLabel(mode, { earlier: st.items.length - INL })));
    }
    tb.appendChild(chev);
    var gi = 0;
    gone.slice().reverse().forEach(function (r) {
      if (r.contains(document.activeElement) && !tail.hidden) tb.focus({ preventScroll: true });
      emerge(r, false, gi++);
    });
    born.forEach(function (r, i) { emerge(r, true, i); });
    var filt = $(root, '[data-rm-filters]');
    var show = st.unfolded && st.items.length > INL;
    if (filt.hidden === show) {
      filt.hidden = !show;
      if (show) { filt.classList.remove('rm-in'); void filt.offsetWidth; filt.classList.add('rm-in'); }
    }
    counts(root);
    var cut = $(root, '[data-rm-cut]');
    if (cut) cut.textContent = cutNotice(st.total, st.items.length);
  }

  function setFilter(root, f) {
    var st = stateOf(root);
    st.filter = f;
    st.shown = PAGE;
    $$(root, '[data-f]').forEach(function (x) { x.setAttribute('aria-pressed', x.getAttribute('data-f') === f ? 'true' : 'false'); });
    sync(root);
  }

  function setUnfolded(root, on) {
    var st = stateOf(root);
    if (st.unfolded === on) return;
    st.unfolded = on;
    st.shown = PAGE;
    if (!on && st.filter !== 'all') {
      st.filter = 'all';
      $$(root, '[data-f]').forEach(function (x) { x.setAttribute('aria-pressed', x.getAttribute('data-f') === 'all' ? 'true' : 'false'); });
    }
    sync(root);
  }

  function tail(elm) {
    var root = rootOf(elm);
    if (!root) return;
    var st = stateOf(root);
    var b = $(root, '[data-tail]');
    var mode = b.getAttribute('data-mode');
    if (mode === 'more') { st.shown += PAGE; sync(root); }
    else setUnfolded(root, mode === 'open');
    b.focus({ preventScroll: true });
  }

  function fewer(elm) {
    var root = rootOf(elm);
    if (!root) return;
    setUnfolded(root, false);
    $(root, '[data-tail]').focus({ preventScroll: true });
  }

  // ---- Keys and click-off: listeners live only while something is open ----

  function anyOpen(st) { return st.menuOpener || st.addOpen || st.editing; }

  function watch(root) {
    var st = stateOf(root);
    if (st.onKey) return;
    st.onKey = function (e) {
      if (!escapeShouldClose(e)) return;
      if (st.menuOpener) { e.preventDefault(); closeMenu(root, false); }
      else if (st.addOpen) { e.preventDefault(); closeAdd(root, false); }
      else if (st.editing) { e.preventDefault(); cancelEdit(root, true); }
    };
    st.onAway = function (e) {
      // A menu closes on a click off it and loses nothing; a panel (the Add
      // form, an edit) closes only on purpose, so this only ever closes a menu.
      // The opener is excluded: its own handler decides (it toggles).
      var menu = $(root, '[data-rm-menu]');
      if (st.menuOpener && !menu.contains(e.target) && !st.menuOpener.contains(e.target)) closeMenu(root, false);
    };
    document.addEventListener('keydown', st.onKey);
    document.addEventListener('click', st.onAway);
  }

  function unwatch(root) {
    var st = stateOf(root);
    if (anyOpen(st) || !st.onKey) return;
    document.removeEventListener('keydown', st.onKey);
    document.removeEventListener('click', st.onAway);
    st.onKey = null;
    st.onAway = null;
  }

  // ---- Tiny menus: open from the word that opened them ----

  function closeMenu(root, instant) {
    var st = stateOf(root);
    var menu = $(root, '[data-rm-menu]');
    if (menu.hidden) return;
    var o = st.menuOpener;
    st.menuOpener = null;
    st.menuTok++;
    var tok = st.menuTok;
    if (o) o.removeAttribute('aria-expanded');
    menu.classList.remove('rm-open');
    if (instant || level() === 'off') menu.hidden = true;
    else setTimeout(function () { if (tok === st.menuTok) menu.hidden = true; }, 130);
    if (o && o.isConnected && root.contains(document.activeElement) && document.activeElement !== document.body && !instant) o.focus({ preventScroll: true });
    unwatch(root);
  }

  function openMenu(root, btn, caption, opts, cur, pick) {
    var st = stateOf(root);
    // Pressing the word that opened a menu closes it.
    if (st.menuOpener === btn) { closeMenu(root, false); return; }
    closeMenu(root, true);
    var menu = $(root, '[data-rm-menu]');
    st.menuOpener = btn;
    st.menuTok++;
    btn.setAttribute('aria-expanded', 'true');
    menu.textContent = '';
    if (caption) menu.appendChild(mk('div', 'rm-cap', caption));
    opts.forEach(function (o) {
      var b = mk('button');
      b.type = 'button';
      b.setAttribute('role', cur !== null ? 'menuitemradio' : 'menuitem');
      if (cur !== null) b.setAttribute('aria-checked', o[0] === cur ? 'true' : 'false');
      b.appendChild(mk('span', null, o[1]));
      if (cur !== null) b.appendChild(icon('check'));
      b.addEventListener('click', function () { pick(o[0]); });
      menu.appendChild(b);
    });
    menu.hidden = false;
    var f = root.getBoundingClientRect();
    var r = btn.getBoundingClientRect();
    var w = menu.offsetWidth;
    var hh = menu.offsetHeight;
    var atLeft = r.left + r.width / 2 - f.left < f.width / 2;
    var left = Math.max(4, Math.min(atLeft ? r.left - f.left : r.right - f.left - w, f.width - w - 4));
    var top = r.bottom - f.top + 2;
    // Near the foot of the page the menu opens upward, off the top of the word.
    if (r.bottom + hh + 8 > window.innerHeight && r.top - hh - 8 > 0) top = r.top - f.top - hh - 2;
    menu.style.left = left + 'px';
    menu.style.top = top + 'px';
    menu.style.setProperty('--ox', (atLeft ? r.left - f.left - left + 8 : r.right - f.left - left - 8) + 'px');
    void menu.offsetWidth;
    menu.classList.add('rm-open');
    var first = $(menu, 'button[aria-checked=true]') || $(menu, 'button');
    if (first) first.focus({ preventScroll: true });
    watch(root);
  }

  function rowOf(el) { return el.closest('.rm-rum'); }

  // ---- Changes: ask the server, then show what it stored ----

  function rowError(root, a, msg) {
    var p = $(a, '.rm-rowerr');
    if (!p) {
      p = mk('p', 'rm-err rm-rowerr');
      p.setAttribute('role', 'alert');
      $(a, '.rm-body').appendChild(p);
    }
    p.textContent = msg;
    say(root, msg);
  }

  function clearRowError(a) { var p = $(a, '.rm-rowerr'); if (p) p.remove(); }

  function adopt(root, fresh) {
    var st = stateOf(root);
    st.items = replaceItem(st.items, fresh);
    var a = rowFor(root, fresh.id);
    if (a) fill(root, a, fresh);
    counts(root);
    // A changed mark can move a rumour out of the filter it is shown under.
    if (st.unfolded && st.filter !== 'all') sync(root);
  }

  function put(root, a, path, body, ok, fail) {
    var st = stateOf(root);
    if (st.busy) return;
    st.busy = true;
    clearRowError(a);
    http(base(root) + '/' + encodeURIComponent(a.getAttribute('data-id')) + path, { method: 'PUT', body: body })
      .then(readJSON)
      .then(function (fresh) { st.busy = false; adopt(root, fresh); if (ok) ok(fresh); })
      .catch(function (err) {
        st.busy = false;
        rowError(root, a, friendly(err, 'That did not save. Try again.'));
        if (fail) fail();
      });
  }

  function truthMenu(btn) {
    var root = rootOf(btn);
    var a = rowOf(btn);
    var it = a && byId(stateOf(root), a.getAttribute('data-id'));
    if (!it) return;
    openMenu(root, btn, 'What is true (DM only)', [['true', 'True'], ['false', 'False'], ['open', 'Open']], it.truth || 'open', function (k) {
      closeMenu(root, false);
      put(root, a, '/truth', { truth: k }, function () { say(root, 'Marked ' + k + ' for the DM.'); });
    });
  }

  function markMenu(btn, caption) {
    var root = rootOf(btn);
    var a = rowOf(btn);
    var it = a && byId(stateOf(root), a.getAttribute('data-id'));
    if (!it) return;
    openMenu(root, btn, caption, [['true', 'True'], ['false', 'False'], ['unsure', 'Unsure']], it.belief || '', function (k) {
      closeMenu(root, false);
      put(root, a, '/belief', { belief: k }, function () { say(root, 'The party now thinks ' + k + '.'); });
    });
  }

  function pbelMenu(btn) { markMenu(btn, 'Set for the party'); }
  function beliefMenu(btn) { markMenu(btn, 'The party thinks'); }

  function moreMenu(btn) {
    var root = rootOf(btn);
    var a = rowOf(btn);
    var it = a && byId(stateOf(root), a.getAttribute('data-id'));
    if (!it || !it.canEdit) return;
    openMenu(root, btn, '', [['edit', 'Edit'], ['remove', 'Remove']], null, function (k) {
      closeMenu(root, false);
      if (k === 'edit') startEdit(root, a);
      else removeItem(root, a, it);
    });
  }

  function removeItem(root, a, it) {
    var st = stateOf(root);
    if (st.busy) return;
    st.busy = true;
    clearRowError(a);
    http(base(root) + '/' + encodeURIComponent(it.id), { method: 'DELETE' })
      .then(readJSON)
      .then(function () {
        st.busy = false;
        st.items = dropItem(st.items, it.id);
        st.total = Math.max(0, st.total - 1);
        sync(root);
        say(root, 'Rumour removed.');
      })
      .catch(function (err) { st.busy = false; rowError(root, a, friendly(err, 'That could not be removed. Try again.')); });
  }

  // ---- Edit in place ----

  function setESure(a, v) {
    $$(a, '[data-esure] button').forEach(function (x) { x.setAttribute('aria-checked', x.getAttribute('data-v') === String(v) ? 'true' : 'false'); });
  }

  function startEdit(root, a) {
    var st = stateOf(root);
    cancelEdit(root, false);
    var it = byId(st, a.getAttribute('data-id'));
    if (!it || !it.canEdit) return;
    st.editing = a;
    a.classList.add('rm-editing');
    $(a, '[data-e=quote]').value = it.quote;
    $(a, '[data-e=who]').value = it.who === 'Someone at the table' ? '' : it.who;
    setESure(a, it.sure);
    var err = $(a, '[data-rm-eerr]');
    err.hidden = true;
    $(a, '[data-rm-editor]').hidden = false;
    if (window.Chronicle && window.Chronicle.markDirty) window.Chronicle.markDirty('rumours-edit-' + root.getAttribute('data-entity'));
    var q = $(a, '[data-e=quote]');
    q.focus();
    q.select();
    watch(root);
  }

  function cancelEdit(root, back) {
    var st = stateOf(root);
    if (!st.editing) return;
    var a = st.editing;
    st.editing = null;
    a.classList.remove('rm-editing');
    $(a, '[data-rm-editor]').hidden = true;
    var it = byId(st, a.getAttribute('data-id'));
    if (it) fill(root, a, it);
    var m = $(a, '[data-more]');
    if (back && m && m.getAttribute('aria-disabled') !== 'true') m.focus({ preventScroll: true });
    unwatch(root);
    if (window.Chronicle && window.Chronicle.markClean) window.Chronicle.markClean('rumours-edit-' + root.getAttribute('data-entity'));
  }

  function editSave(btn) {
    var root = rootOf(btn);
    var st = stateOf(root);
    var a = st.editing;
    if (!a || st.busy) return;
    var quote = $(a, '[data-e=quote]');
    var who = $(a, '[data-e=who]');
    var err = $(a, '[data-rm-eerr]');
    var msg = validate(quote.value, who.value);
    if (msg) { err.textContent = msg; err.hidden = false; quote.focus(); return; }
    err.hidden = true;
    var c = $(a, '[data-esure] [aria-checked=true]');
    var save = $(a, '[data-save]');
    var old = save.textContent;
    save.setAttribute('aria-disabled', 'true');
    save.textContent = 'Saving…';
    st.busy = true;
    http(base(root) + '/' + encodeURIComponent(a.getAttribute('data-id')), {
      method: 'PUT', body: { quote: cleanText(quote.value, QUOTE_MAX).text, who: cleanText(who.value, WHO_MAX).text, sure: c ? parseInt(c.getAttribute('data-v'), 10) : 2 }
    })
      .then(readJSON)
      .then(function (fresh) {
        st.busy = false;
        save.removeAttribute('aria-disabled');
        save.textContent = old;
        st.items = replaceItem(st.items, fresh);
        cancelEdit(root, true);
        adopt(root, fresh);
        say(root, 'Rumour saved.');
      })
      .catch(function (e2) {
        st.busy = false;
        save.removeAttribute('aria-disabled');
        save.textContent = old;
        err.textContent = friendly(e2, 'That did not save. Try again.');
        err.hidden = false;
      });
  }

  // Enter saves from either field; the rumours are short enough to be one line.
  function editKey(el, e) {
    if (!e || e.key !== 'Enter' || e.isComposing) return;
    e.preventDefault();
    editSave(el);
  }

  function dblEdit(q) {
    var root = rootOf(q);
    if (!root) return;
    var a = rowOf(q);
    var st = stateOf(root);
    var it = a && byId(st, a.getAttribute('data-id'));
    if (it && it.canEdit) { closeMenu(root, true); startEdit(root, a); }
  }

  // ---- The Add form: the control becomes the form (container transform) ----

  function cancelAnims(st) { st.popAnim.forEach(function (x) { x.cancel(); }); st.popAnim = []; }

  function popParts(root) {
    var pop = $(root, '[data-rm-pop]');
    return { pop: pop, inner: $(pop, '.rm-pop-in'), content: $(pop, '.rm-pop-c'), btn: $(root, '[data-rm-add]') };
  }

  function placeAdd(root) {
    var p = popParts(root);
    var f = root.getBoundingClientRect();
    var o = p.btn.getBoundingClientRect();
    var w = Math.min(340, f.width - 16);
    p.pop.style.width = w + 'px';
    p.pop.style.left = Math.max(8, Math.min(o.right - f.left - w, f.width - w - 8)) + 'px';
    p.pop.style.top = Math.max(0, o.top - f.top) + 'px';
  }

  function clipFrom(root, radius) {
    var p = popParts(root);
    var pr = p.pop.getBoundingClientRect();
    var o = p.btn.getBoundingClientRect();
    return 'inset(' + (o.top - pr.top) + 'px ' + (pr.right - o.right) + 'px ' + (pr.bottom - o.bottom) + 'px ' + (o.left - pr.left) + 'px round ' + radius + 'px)';
  }

  function validAdd(root) {
    var p = popParts(root);
    var ok = !!$(p.pop, '[data-a=quote]').value.trim();
    $(p.pop, '[data-ok]').setAttribute('aria-disabled', ok ? 'false' : 'true');
    return ok;
  }

  function addInput(el) {
    var root = rootOf(el);
    if (!root) return;
    validAdd(root);
    if (window.Chronicle && window.Chronicle.markDirty) window.Chronicle.markDirty('rumours-add-' + root.getAttribute('data-entity'));
  }

  function sure(el) {
    var root = rootOf(el);
    if (!root) return;
    $$(el.parentNode, 'button').forEach(function (x) { x.setAttribute('aria-checked', x === el ? 'true' : 'false'); });
  }

  function esure(el, v) {
    var a = rowOf(el);
    if (a) setESure(a, v);
  }

  function addToggle(btn) {
    var root = rootOf(btn);
    if (!root) return;
    stateOf(root).addOpen ? closeAdd(root, false) : openAdd(root);
  }

  function openAdd(root) {
    var st = stateOf(root);
    closeMenu(root, true);
    cancelEdit(root, false);
    var p = popParts(root);
    var q = $(p.pop, '[data-a=quote]');
    q.value = '';
    $(p.pop, '[data-a=who]').value = '';
    $$(p.pop, '[data-sure] button').forEach(function (x) { x.setAttribute('aria-checked', x.getAttribute('data-v') === '2' ? 'true' : 'false'); });
    $(p.pop, '[data-rm-err]').hidden = true;
    validAdd(root);
    cancelAnims(st);
    st.addOpen = true;
    p.pop.hidden = false;
    placeAdd(root);
    p.btn.setAttribute('aria-expanded', 'true');
    watch(root);
    var L = level();
    function focus1() { q.focus({ preventScroll: true }); }
    if (L === 'off' || !p.pop.animate) { p.btn.style.visibility = 'hidden'; focus1(); return; }
    if (L === 'calm') {
      st.popAnim.push(p.pop.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 200, easing: 'ease-out' }));
      st.popAnim.push(p.btn.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 120, fill: 'forwards' }));
      setTimeout(focus1, 30);
      return;
    }
    // The button itself grows into the form: the form starts clipped to the
    // button's box and opens to its own, while the button fades out and the
    // form's content fades in after the first 40%.
    var radius = parseFloat(getComputedStyle(p.inner).borderTopLeftRadius) || 14;
    var panelBg = getComputedStyle(p.inner).backgroundColor;
    var from = clipFrom(root, 8);
    var to = 'inset(0px 0px 0px 0px round ' + radius + 'px)';
    var pageBg = getComputedStyle(root).getPropertyValue('--rm-page').trim() || 'transparent';
    st.popAnim.push(p.inner.animate([{ clipPath: from, backgroundColor: pageBg }, { clipPath: to, backgroundColor: panelBg }], { duration: 300, easing: EASE_STD }));
    st.popAnim.push(p.content.animate([{ opacity: 0 }, { opacity: 0, offset: 0.4 }, { opacity: 1 }], { duration: 300, easing: 'linear' }));
    st.popAnim.push(p.btn.animate([{ opacity: 1 }, { opacity: 0, offset: 0.4 }, { opacity: 0 }], { duration: 300, fill: 'forwards' }));
    setTimeout(focus1, 60);
  }

  function closeAdd(root, instant) {
    var st = stateOf(root);
    if (!st.addOpen) return;
    st.addOpen = false;
    var p = popParts(root);
    var L = instant ? 'off' : level();
    var had = p.pop.contains(document.activeElement);
    cancelAnims(st);
    if (window.Chronicle && window.Chronicle.markClean) window.Chronicle.markClean('rumours-add-' + root.getAttribute('data-entity'));
    function done() {
      if (st.addOpen) return;
      cancelAnims(st);
      p.pop.hidden = true;
      p.btn.style.visibility = '';
      p.btn.setAttribute('aria-expanded', 'false');
      if (had && !p.btn.hidden) p.btn.focus({ preventScroll: true });
      unwatch(root);
    }
    if (L === 'off' || !p.pop.animate) { done(); return; }
    if (L === 'calm') {
      var a1 = p.pop.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 120, easing: 'ease-in', fill: 'forwards' });
      st.popAnim.push(a1);
      st.popAnim.push(p.btn.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 120, fill: 'both' }));
      a1.onfinish = done;
      return;
    }
    var radius = parseFloat(getComputedStyle(p.inner).borderTopLeftRadius) || 14;
    var panelBg = getComputedStyle(p.inner).backgroundColor;
    var pageBg = getComputedStyle(root).getPropertyValue('--rm-page').trim() || 'transparent';
    var a = p.inner.animate([
      { clipPath: 'inset(0px 0px 0px 0px round ' + radius + 'px)', backgroundColor: panelBg },
      { clipPath: clipFrom(root, 8), backgroundColor: pageBg }
    ], { duration: 260, easing: EASE_STD, fill: 'forwards' });
    st.popAnim.push(a);
    st.popAnim.push(p.content.animate([{ opacity: 1 }, { opacity: 0, offset: 0.4 }, { opacity: 0 }], { duration: 260, easing: 'linear', fill: 'forwards' }));
    st.popAnim.push(p.btn.animate([{ opacity: 0 }, { opacity: 0, offset: 0.6 }, { opacity: 1 }], { duration: 260, fill: 'forwards' }));
    a.onfinish = done;
  }

  function addSave(btn) {
    var root = rootOf(btn);
    var st = stateOf(root);
    if (!root || st.busy) return;
    var p = popParts(root);
    var err = $(p.pop, '[data-rm-err]');
    var quote = $(p.pop, '[data-a=quote]');
    var who = $(p.pop, '[data-a=who]');
    var msg = validate(quote.value, who.value);
    if (msg) { err.textContent = msg; err.hidden = false; quote.focus(); return; }
    err.hidden = true;
    var c = $(p.pop, '[data-sure] [aria-checked=true]');
    var ok = $(p.pop, '[data-ok]');
    var old = ok.textContent;
    ok.setAttribute('aria-disabled', 'true');
    ok.textContent = '';
    ok.appendChild(icon('circle-notch', 'fa-spin'));
    ok.appendChild(document.createTextNode('Adding…'));
    st.busy = true;
    http(base(root), {
      method: 'POST', body: { quote: cleanText(quote.value, QUOTE_MAX).text, who: cleanText(who.value, WHO_MAX).text, sure: c ? parseInt(c.getAttribute('data-v'), 10) : 2 }
    })
      .then(readJSON)
      .then(function (fresh) {
        st.busy = false;
        ok.textContent = old;
        ok.removeAttribute('aria-disabled');
        st.items = replaceItem(st.items, fresh);
        st.total += 1;
        closeAdd(root, false);
        sync(root);
        var r = rowFor(root, fresh.id);
        if (r) glow(r);
        var cnt = $(root, '[data-rm-cnt]');
        cnt.classList.remove('rm-bump');
        void cnt.offsetWidth;
        cnt.classList.add('rm-bump');
        say(root, 'Rumour added.');
      })
      .catch(function (e2) {
        st.busy = false;
        ok.textContent = old;
        validAdd(root);
        err.textContent = friendly(e2, 'That did not save. Try again.');
        err.hidden = false;
      });
  }

  function addClose(el) {
    var root = rootOf(el);
    if (root) closeAdd(root, false);
  }

  window.ChronicleRumours = {
    addToggle: addToggle, addClose: addClose, addInput: addInput, addSave: addSave, sure: sure, esure: esure,
    tail: tail, fewer: fewer, truthMenu: truthMenu, pbelMenu: pbelMenu, beliefMenu: beliefMenu, moreMenu: moreMenu,
    editSave: editSave, editCancel: function (el) { var r = rootOf(el); if (r) cancelEdit(r, true); }, editKey: editKey, dblEdit: dblEdit
  };

  // The block mounts as a widget so its script loads on sight (ADR-063). On
  // mount the script takes over from the server's resting rows: it fills every
  // row from the JSON and draws the folded list, so what the server drew and
  // what this draws are one thing.
  function mount(root) {
    if (!root || root.__rmMounted) return;
    root.__rmMounted = true;
    var st = stateOf(root);
    renderFilters(root);
    $$(listOf(root), '.rm-rum').forEach(function (a) {
      var it = byId(st, a.getAttribute('data-id'));
      if (it) fill(root, a, it); else a.remove();
    });
    sync(root, true);
  }

  if (window.Chronicle && window.Chronicle.register) {
    window.Chronicle.register('rumours', {
      init: function (node) { mount(rootOf(node) || node); },
      destroy: function (node) {
        var r = rootOf(node) || node;
        if (r.__rm) {
          if (r.__rm.onKey) document.removeEventListener('keydown', r.__rm.onKey);
          if (r.__rm.onAway) document.removeEventListener('click', r.__rm.onAway);
        }
      }
    });
  }
})();
