/**
 * Standing
 *
 * The entity page's "Standing" block (internal/plugins/entities/
 * standing_block.templ) is a word, five steps and one quiet line. This file is
 * the behaviour behind its inline handlers:
 *
 *   - A click on the word, the steps or the "Why:" line drapes the reasons down
 *     over the page from the line (a clip reveal in CSS; nothing below moves).
 *     It has a visible Close and Escape closes it; a click outside does not (a
 *     panel closes only on purpose).
 *   - The subject menu switches between the party and the characters.
 *   - The Director's "Add a reason" row turns into its form in place; Save
 *     adds the reason (and may move the word), the row's x removes one, and a
 *     switch decides whether players see the standing at all.
 *
 * The server decides what a viewer may see: the block's JSON holds no DM-only
 * reason for a player, and every write is the Director's and re-checked by the
 * service. This file makes no permission decision. Text reaches the DOM only
 * through textContent. Motion follows html[data-motion] (calm fades, off shows
 * the end state) and a device asking for reduced motion is always off.
 *
 * The handlers are inline IIFEs (the block can arrive by an HTMX swap), so this
 * exposes window.ChronicleStanding and adds no delegated listener. The one
 * document keydown listener (Escape) and the menu's click-off listener live
 * only while a drape or menu is open.
 */
(function () {
  'use strict';

  var WORDS = ['Hostile', 'Wary', 'Neutral', 'Warm', 'Trusted'];
  var PAGE = 30;
  var WHY_WORDS = 4;
  var WHY_LIST_MAX = 8;
  var TOOLS_MIN = 9; // find and filter appear once there are more than 8
  var REASON_MAX = 90;
  var WHEN_MAX = 30;
  var EASE_OUT = 'cubic-bezier(.16,1,.3,1)';
  var EASE_IN = 'cubic-bezier(.4,0,1,1)';
  var MARK = { up: 'fa-arrow-up', down: 'fa-arrow-down', none: 'fa-minus' };
  var MARK_LABEL = { up: 'Raised their standing', down: 'Lowered their standing', none: 'No change to standing' };

  // ---- Pure helpers (unit-tested in test/js/standing.test.mjs) ----

  /** motionOf: how much motion the page allows ('full' | 'calm' | 'off'). */
  function motionOf(attr, reduced) {
    if (reduced) return 'off';
    return attr === 'calm' || attr === 'off' ? attr : 'full';
  }

  /**
   * whyText: the quiet line under the word. Mirrors StandingWhyText in
   * standing_view.go; both are pinned by the same cases.
   */
  function whyText(total, newest) {
    if (!total || total <= 0) return 'Why: no reasons shared yet';
    if (total > WHY_LIST_MAX) return 'Why: ' + total + ' reasons';
    var words = String(newest || '').split(/\s+/).filter(Boolean);
    var lead = String(newest || '');
    if (words.length > WHY_WORDS) lead = words.slice(0, WHY_WORDS).join(' ') + '…';
    return total === 1 ? 'Why: ' + lead : 'Why: ' + lead + ' and ' + (total - 1) + ' more';
  }

  /** wordFor: the word for a level; anything out of range reads as Neutral. */
  function wordFor(level) {
    return level >= 0 && level < WORDS.length && level === Math.floor(level) ? WORDS[level] : WORDS[2];
  }

  /** stepsLabel: the steps' label for a screen reader. */
  function stepsLabel(sub) {
    return sub && sub.set ? 'Standing: ' + wordFor(sub.level) + ', step ' + (sub.level + 1) + ' of 5' : 'Standing not set yet';
  }

  /**
   * cleanText: trims, folds inner whitespace runs, and refuses control
   * characters. Returns { text, ok }; ok is false for control characters or a
   * text over max (never cut quietly). Mirrors cleanStandingText in Go.
   */
  function cleanText(s, max) {
    var str = String(s == null ? '' : s);
    if (/[\u0000-\u0008\u000e-\u001f\u007f-\u009f]/.test(str)) return { text: '', ok: false };
    var t = str.replace(/\s+/g, ' ').replace(/^ | $/g, '');
    return { text: t, ok: Array.from(t).length <= max };
  }

  /** validateReason: the message to show for a reason that cannot be saved, or ''. */
  function validateReason(text, when) {
    var t = cleanText(text, REASON_MAX);
    if (!t.ok && !t.text) return 'Use plain text for the reason.';
    if (!t.text) return 'Say what happened first.';
    if (!t.ok) return 'Keep the reason to 90 characters or fewer.';
    if (!cleanText(when, WHEN_MAX).ok) return 'Keep the "when" to 30 characters or fewer.';
    return '';
  }

  /** suggestLevel: the word a reason of that effect would move the standing to. */
  function suggestLevel(level, dir) {
    var n = level + (dir === 'up' ? 1 : dir === 'down' ? -1 : 0);
    return Math.max(0, Math.min(4, n));
  }

  /** filterReasons: the reasons matching the search text and the effect filter. */
  function filterReasons(list, q, f) {
    var needle = String(q || '').toLowerCase();
    return (list || []).filter(function (r) {
      return (!f || f === 'all' || r.direction === f) && (!needle || String(r.text).toLowerCase().indexOf(needle) > -1);
    });
  }

  /** infoText: the drape header's small line. */
  function infoText(shown, total, filtering) {
    return filtering ? shown + ' of ' + total + ' · newest first' : 'Newest first';
  }

  /** moreLabel: the label of the button that reveals the next page of reasons. */
  function moreLabel(left) {
    return 'Show ' + Math.min(PAGE, left) + ' more (' + left + ' left)';
  }

  /**
   * visibleTotal: how many reasons count in the Director's "view as a player"
   * preview, which hides the Director-only ones the server also sent the
   * Director. Without the preview it is the server's own total.
   */
  function visibleTotal(sub, asPlayer) {
    if (!asPlayer) return sub.total;
    var hidden = (sub.reasons || []).filter(function (r) { return r.dmOnly; }).length;
    return Math.max(0, sub.total - hidden);
  }

  /** escapeShouldClose: Escape closes, unless a modifier is held. */
  function escapeShouldClose(e) {
    if (!e || e.key !== 'Escape') return false;
    return !(e.ctrlKey || e.metaKey || e.altKey);
  }

  /** replaceSubject: the subjects with one swapped for the server's newer answer. */
  function replaceSubject(subjects, fresh) {
    var out = (subjects || []).slice();
    for (var i = 0; i < out.length; i++) {
      if (out[i].key === fresh.key) { out[i] = fresh; return out; }
    }
    out.push(fresh);
    return out;
  }

  var api = {
    motionOf: motionOf,
    whyText: whyText,
    wordFor: wordFor,
    stepsLabel: stepsLabel,
    cleanText: cleanText,
    validateReason: validateReason,
    suggestLevel: suggestLevel,
    filterReasons: filterReasons,
    infoText: infoText,
    moreLabel: moreLabel,
    visibleTotal: visibleTotal,
    escapeShouldClose: escapeShouldClose,
    replaceSubject: replaceSubject,
    PAGE: PAGE
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

  function rootOf(el) { return el && el.closest ? el.closest('.sg') : null; }
  function $(root, sel) { return root.querySelector(sel); }
  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function asPlayer() { return document.documentElement.hasAttribute('data-view-as-player'); }
  function isDM(root) { return root.getAttribute('data-dm') === '1'; }

  function stateOf(root) {
    if (!root.__sg) {
      var data = { subjects: [] };
      var s = root.querySelector('script[type="application/json"]');
      try { data = JSON.parse(s ? s.textContent : '{}') || data; } catch (e) { /* an unreadable block shows nothing to change */ }
      if (!data.subjects) data.subjects = [];
      root.__sg = {
        data: data, cur: root.getAttribute('data-current') || '', open: false, shown: PAGE, q: '', f: 'all',
        dir: 'up', vis: true, formOpen: false, menuOpen: false, busy: false, onKey: null, onAway: null
      };
    }
    return root.__sg;
  }

  function current(root) {
    var st = stateOf(root);
    for (var i = 0; i < st.data.subjects.length; i++) if (st.data.subjects[i].key === st.cur) return st.data.subjects[i];
    return st.data.subjects[0] || { key: 'party', name: 'The party', set: false, level: 2, word: 'Neutral', total: 0, reasons: [] };
  }

  function reasonsOf(root) {
    var sub = current(root);
    return asPlayer() ? (sub.reasons || []).filter(function (r) { return !r.dmOnly; }) : (sub.reasons || []);
  }

  function say(root, msg) { var l = $(root, '[data-sg-live]'); if (l) l.textContent = msg; }

  // ---- Talking to the server ----

  function call(url, opts) {
    if (window.Chronicle && window.Chronicle.apiFetch) return window.Chronicle.apiFetch(url, opts);
    opts = opts || {};
    return fetch(url, { method: opts.method || 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'Content-Type': 'application/json' }, body: opts.body ? JSON.stringify(opts.body) : undefined });
  }

  function base(root) {
    return '/campaigns/' + encodeURIComponent(root.getAttribute('data-campaign')) + '/entities/' + encodeURIComponent(root.getAttribute('data-entity'));
  }

  // Resolves with the parsed body, or rejects with an Error whose message is
  // the server's own words when it sent any.
  function readJSON(res) {
    if (res.ok) return res.json();
    return res.json().then(function (b) { throw new Error((b && b.message) || 'status ' + res.status); },
      function () { throw new Error('status ' + res.status); });
  }

  // ---- Drawing ----

  function renderWord(root, animate) {
    var sub = current(root);
    var wrap = $(root, '[data-sg-word]');
    var text = sub.set ? wordFor(sub.level) : 'Not set yet';
    var olds = wrap.querySelectorAll('.sg-w');
    var lv = level();
    var old = olds[olds.length - 1];
    if (old && old.textContent === text) return;
    var nw = el('span', 'sg-w' + (sub.set ? '' : ' sg-unset'), text);
    if (!animate || lv === 'off' || !old) {
      wrap.textContent = '';
      wrap.appendChild(nw);
      return;
    }
    while (olds.length > 1) { wrap.removeChild(olds[0]); olds = wrap.querySelectorAll('.sg-w'); }
    wrap.appendChild(nw);
    nw.animate(lv === 'full' ? [{ opacity: 0, transform: 'translateY(6px)' }, { opacity: 1, transform: 'none' }] : [{ opacity: 0 }, { opacity: 1 }], { duration: 280, easing: EASE_OUT });
    var a = old.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 280, easing: EASE_OUT, fill: 'forwards' });
    a.onfinish = function () { if (old.parentNode) old.parentNode.removeChild(old); };
  }

  function renderSteps(root, instant) {
    var sub = current(root);
    var steps = $(root, '[data-sg-steps]');
    var thumb = $(root, '[data-sg-thumb]');
    steps.setAttribute('aria-label', stepsLabel(sub));
    thumb.hidden = !sub.set;
    if (instant) { thumb.style.transition = 'none'; thumb.style.setProperty('--i', String(sub.level)); void thumb.offsetWidth; thumb.style.transition = ''; }
    else thumb.style.setProperty('--i', String(sub.level));
    Array.prototype.forEach.call(root.querySelectorAll('.sg-dscale span'), function (s, i) {
      s.className = sub.set && i === sub.level ? 'sg-cur' : '';
    });
  }

  function renderWhy(root) {
    var list = reasonsOf(root);
    var total = visibleTotal(current(root), asPlayer());
    $(root, '[data-sg-wt]').textContent = whyText(total, list.length ? list[0].text : '');
    $(root, '[data-sg-cnt]').textContent = String(total);
  }

  function makeRow(root, r) {
    var li = el('li', 'sg-rs');
    var row = el('div', 'sg-lrow' + (r.dmOnly ? ' sg-dm' : ''));
    if (r.dmOnly) row.setAttribute('data-dm-only', '');
    var mk = el('span', 'sg-mk sg-' + (MARK[r.direction] ? r.direction : 'none'));
    mk.setAttribute('role', 'img');
    mk.setAttribute('aria-label', MARK_LABEL[r.direction] || MARK_LABEL.none);
    var ic = el('i', 'fa-solid ' + (MARK[r.direction] || MARK.none));
    ic.setAttribute('aria-hidden', 'true');
    mk.appendChild(ic);
    var tx = el('div', 'sg-tx');
    tx.appendChild(el('span', null, r.text));
    if (r.dmOnly) tx.appendChild(el('span', 'sg-dmtag cb-dm', 'DM'));
    row.appendChild(mk);
    row.appendChild(tx);
    row.appendChild(el('span', 'sg-wh', r.when || ''));
    if (isDM(root)) {
      var rm = el('button', 'sg-rm');
      rm.type = 'button';
      rm.setAttribute('aria-label', 'Remove reason: ' + r.text);
      var x = el('i', 'fa-solid fa-xmark');
      x.setAttribute('aria-hidden', 'true');
      rm.appendChild(x);
      rm.addEventListener('click', function () { removeReason(root, li, r); });
      row.appendChild(rm);
    } else {
      row.appendChild(el('span'));
    }
    li.appendChild(row);
    return li;
  }

  function renderList(root) {
    var st = stateOf(root);
    var list = $(root, '[data-sg-list]');
    var all = reasonsOf(root);
    var total = visibleTotal(current(root), asPlayer());
    var f = filterReasons(all, st.q, st.f);
    var n = Math.min(st.shown, f.length);
    list.textContent = '';
    $(root, '[data-sg-tools]').hidden = all.length < TOOLS_MIN;
    for (var i = 0; i < n; i++) list.appendChild(makeRow(root, f[i]));
    if (!f.length) list.appendChild(el('li', 'sg-empty', all.length ? 'Nothing matches that.' : 'No reasons shared yet.'));
    if (f.length > n) {
      var li = el('li', 'sg-more');
      var b = el('button', 'sg-quiet', moreLabel(f.length - n));
      b.type = 'button';
      b.addEventListener('click', function () {
        st.shown += PAGE;
        var y = list.scrollTop;
        renderList(root);
        list.scrollTop = y;
      });
      li.appendChild(b);
      list.appendChild(li);
    }
    var filtering = !!(st.q || st.f !== 'all');
    $(root, '[data-sg-info]').textContent = infoText(f.length, total, filtering);
    renderWhy(root);
  }

  function renderMenu(root) {
    var sub = current(root);
    var lab = $(root, '[data-sg-subjlab]');
    if (lab) lab.textContent = sub.name;
    Array.prototype.forEach.call(root.querySelectorAll('[data-sg-menu] button'), function (b) {
      b.setAttribute('aria-checked', b.getAttribute('data-key') === sub.key ? 'true' : 'false');
    });
    var drape = $(root, '[data-sg-drape]');
    drape.setAttribute('aria-label', 'Why ' + (root.getAttribute('data-entity-name') || 'this page') + ' feels this way about ' + sub.name);
    var forEl = $(root, '[data-sg-for]');
    if (forEl) forEl.textContent = 'for ' + sub.name;
  }

  function renderDMState(root) {
    var st = stateOf(root);
    var hidden = isDM(root) && st.data.playersSee === false;
    $(root, '[data-sg-srow]').classList.toggle('sg-dm', hidden);
    var tag = $(root, '[data-sg-dmtag]');
    if (tag) tag.hidden = !hidden;
    var see = $(root, '[data-sg-see]');
    if (see) {
      see.setAttribute('aria-checked', st.data.playersSee === false ? 'false' : 'true');
      var hint = $(root, '[data-sg-seehint]');
      if (hint) hint.hidden = st.data.playersSee !== false;
    }
  }

  function renderAll(root, opts) {
    opts = opts || {};
    renderWord(root, !!opts.animateWord);
    renderSteps(root, !!opts.instant);
    renderMenu(root);
    renderDMState(root);
    renderList(root);
  }

  // ---- The drape ----

  function onKey(root, e) {
    if (!escapeShouldClose(e)) return;
    var st = stateOf(root);
    if (st.formOpen) { e.preventDefault(); closeForm(root, true); }
    else if (st.menuOpen) { e.preventDefault(); closeMenu(root, true); }
    else if (st.open) { e.preventDefault(); closeDrape(root, true); }
  }

  function watchKeys(root) {
    var st = stateOf(root);
    if (st.onKey) return;
    st.onKey = function (e) { onKey(root, e); };
    document.addEventListener('keydown', st.onKey);
  }

  function unwatchKeys(root) {
    var st = stateOf(root);
    if (st.open || st.menuOpen || !st.onKey) return;
    document.removeEventListener('keydown', st.onKey);
    st.onKey = null;
  }

  function openDrape(root) {
    var st = stateOf(root);
    if (st.open) return;
    st.open = true;
    renderSteps(root, true);
    renderList(root);
    $(root, '[data-sg-drape]').classList.add('sg-on');
    $(root, '[data-sg-why]').setAttribute('aria-expanded', 'true');
    watchKeys(root);
  }

  function closeDrape(root, refocus) {
    var st = stateOf(root);
    if (!st.open) return;
    closeForm(root, false);
    st.open = false;
    $(root, '[data-sg-drape]').classList.remove('sg-on');
    var why = $(root, '[data-sg-why]');
    why.setAttribute('aria-expanded', 'false');
    unwatchKeys(root);
    if (refocus) why.focus();
  }

  function toggle(elm) {
    var root = rootOf(elm);
    if (!root) return;
    var st = stateOf(root);
    if (st.open) closeDrape(root, elm && elm.hasAttribute && elm.hasAttribute('data-sg-srow') ? false : true);
    else openDrape(root);
  }

  function close(elm) { var root = rootOf(elm); if (root) closeDrape(root, true); }

  // ---- The subject menu: closes on a click off it, unlike a panel ----

  function openMenu(root) {
    var st = stateOf(root);
    if (st.menuOpen) return;
    st.menuOpen = true;
    $(root, '[data-sg-menu]').classList.add('sg-on');
    $(root, '[data-sg-subjbtn]').setAttribute('aria-expanded', 'true');
    watchKeys(root);
    st.onAway = function (e) {
      var pick = root.querySelector('.sg-pick');
      if (pick && !pick.contains(e.target)) closeMenu(root, false);
    };
    document.addEventListener('click', st.onAway);
  }

  function closeMenu(root, refocus) {
    var st = stateOf(root);
    if (!st.menuOpen) return;
    st.menuOpen = false;
    $(root, '[data-sg-menu]').classList.remove('sg-on');
    var btn = $(root, '[data-sg-subjbtn]');
    btn.setAttribute('aria-expanded', 'false');
    if (st.onAway) { document.removeEventListener('click', st.onAway); st.onAway = null; }
    unwatchKeys(root);
    if (refocus) btn.focus();
  }

  function menu(elm) {
    var root = rootOf(elm);
    if (!root) return;
    stateOf(root).menuOpen ? closeMenu(root, false) : openMenu(root);
  }

  function resetFind(root) {
    var st = stateOf(root);
    st.q = ''; st.f = 'all'; st.shown = PAGE;
    var find = $(root, '[data-sg-find]');
    if (find) find.value = '';
    Array.prototype.forEach.call(root.querySelectorAll('.sg-tools .sg-seg button'), function (b) {
      b.setAttribute('aria-pressed', b.getAttribute('data-v') === 'all' ? 'true' : 'false');
    });
  }

  function settle(node) {
    var lv = level();
    if (lv === 'off' || !node || !node.animate) return;
    node.animate(lv === 'full' ? [{ opacity: 0, transform: 'translateY(6px)' }, { opacity: 1, transform: 'none' }] : [{ opacity: 0 }, { opacity: 1 }], { duration: 280, easing: EASE_OUT });
  }

  function subject(elm) {
    var root = rootOf(elm);
    if (!root) return;
    var st = stateOf(root);
    var key = elm.getAttribute('data-key');
    closeMenu(root, true);
    if (!key || key === st.cur) return;
    st.cur = key;
    resetFind(root);
    closeForm(root, false);
    renderAll(root, { instant: true });
    settle($(root, '[data-sg-srow]'));
    if (st.open) settle($(root, '[data-sg-list]'));
  }

  // ---- Find and filter ----

  function find(elm) {
    var root = rootOf(elm);
    if (!root) return;
    var st = stateOf(root);
    st.q = elm.value || '';
    st.shown = PAGE;
    renderList(root);
  }

  function filter(elm, v) {
    var root = rootOf(elm);
    if (!root) return;
    var st = stateOf(root);
    st.f = v;
    st.shown = PAGE;
    Array.prototype.forEach.call(root.querySelectorAll('.sg-tools .sg-seg button'), function (b) {
      b.setAttribute('aria-pressed', b === elm ? 'true' : 'false');
    });
    renderList(root);
  }

  // ---- Writes (the Director's) ----

  function apply(root, fresh, opts) {
    var st = stateOf(root);
    st.data.subjects = replaceSubject(st.data.subjects, fresh);
    renderAll(root, opts);
  }

  function removeReason(root, li, r) {
    var st = stateOf(root);
    if (st.busy) return;
    st.busy = true;
    var key = st.cur;
    call(base(root) + '/standing/' + encodeURIComponent(key) + '/reasons/' + encodeURIComponent(r.id), { method: 'DELETE' })
      .then(readJSON)
      .then(function (fresh) {
        st.busy = false;
        say(root, 'Reason removed.');
        var lv = level();
        var finish = function () { apply(root, fresh, {}); };
        if (lv === 'off' || !li.animate) { finish(); return; }
        var h = li.offsetHeight;
        li.style.overflow = 'hidden';
        var a = li.animate(lv === 'full' ? [{ height: h + 'px', opacity: 1 }, { height: '0px', opacity: 0 }] : [{ opacity: 1 }, { opacity: 0 }], { duration: 160, easing: EASE_IN, fill: 'forwards' });
        a.onfinish = finish;
      })
      .catch(function (err) { st.busy = false; say(root, 'Could not remove that reason. ' + (err && err.message ? err.message : '')); });
  }

  function growRow(row, fromH) {
    var lv = level();
    if (lv !== 'full' || !row.animate) return;
    var toH = row.offsetHeight;
    row.style.overflow = 'hidden';
    var a = row.animate([{ height: fromH + 'px' }, { height: toH + 'px' }], { duration: 200, easing: EASE_OUT });
    a.onfinish = a.oncancel = function () { row.style.overflow = ''; };
  }

  function setDir(root, v) {
    var st = stateOf(root);
    st.dir = v;
    Array.prototype.forEach.call(root.querySelectorAll('[data-sg-dir] button'), function (b) {
      b.setAttribute('aria-pressed', b.getAttribute('data-v') === v ? 'true' : 'false');
    });
    var sub = current(root);
    $(root, '[data-sg-w]').value = String(suggestLevel(sub.set ? sub.level : 2, v));
  }

  function dir(elm, v) { var root = rootOf(elm); if (root) setDir(root, v); }

  function setVis(root, v) {
    stateOf(root).vis = v;
    var b = $(root, '[data-sg-vis]');
    b.setAttribute('aria-checked', v ? 'true' : 'false');
    $(root, '[data-sg-vishint]').hidden = v;
  }

  function vis(elm) { var root = rootOf(elm); if (root) setVis(root, !stateOf(root).vis); }

  function addOpen(elm) {
    var root = rootOf(elm);
    if (!root) return;
    var st = stateOf(root);
    var row = $(root, '[data-sg-addrow]');
    var form = $(root, '[data-sg-form]');
    var btn = $(root, '[data-sg-add]');
    $(root, '[data-sg-s]').value = '';
    $(root, '[data-sg-n]').value = '';
    var err = $(root, '[data-sg-err]');
    err.classList.remove('sg-show');
    err.textContent = '';
    $(root, '[data-sg-s]').removeAttribute('aria-invalid');
    setVis(root, true);
    setDir(root, 'up');
    var h0 = row.offsetHeight;
    btn.hidden = true;
    form.hidden = false;
    row.classList.add('sg-open');
    btn.setAttribute('aria-expanded', 'true');
    st.formOpen = true;
    growRow(row, h0);
    if (row.scrollIntoView) row.scrollIntoView({ block: 'nearest' });
    setTimeout(function () { $(root, '[data-sg-s]').focus({ preventScroll: true }); }, 40);
  }

  function closeForm(root, refocus) {
    var st = stateOf(root);
    if (!st.formOpen) return;
    st.formOpen = false;
    var row = $(root, '[data-sg-addrow]');
    var form = $(root, '[data-sg-form]');
    var btn = $(root, '[data-sg-add]');
    var h0 = row.offsetHeight;
    form.hidden = true;
    btn.hidden = false;
    row.classList.remove('sg-open');
    btn.setAttribute('aria-expanded', 'false');
    growRow(row, h0);
    if (refocus) btn.focus();
  }

  function addCancel(elm) { var root = rootOf(elm); if (root) closeForm(root, true); }

  function addSave(elm, e) {
    if (e && e.preventDefault) e.preventDefault();
    var root = rootOf(elm);
    if (!root) return;
    var st = stateOf(root);
    if (st.busy) return;
    var sIn = $(root, '[data-sg-s]');
    var err = $(root, '[data-sg-err]');
    var msg = validateReason(sIn.value, $(root, '[data-sg-n]').value);
    if (msg) {
      err.textContent = msg;
      err.classList.add('sg-show');
      sIn.setAttribute('aria-invalid', 'true');
      sIn.focus();
      return;
    }
    err.classList.remove('sg-show');
    sIn.removeAttribute('aria-invalid');
    var before = current(root);
    var body = {
      direction: st.dir, text: cleanText(sIn.value, REASON_MAX).text, when: cleanText($(root, '[data-sg-n]').value, WHEN_MAX).text,
      dmOnly: !st.vis, level: parseInt($(root, '[data-sg-w]').value, 10)
    };
    var save = $(root, '[data-sg-save]');
    st.busy = true;
    save.disabled = true;
    call(base(root) + '/standing/' + encodeURIComponent(st.cur) + '/reasons', { method: 'POST', body: body })
      .then(readJSON)
      .then(function (fresh) {
        st.busy = false;
        save.disabled = false;
        resetFind(root);
        apply(root, fresh, { animateWord: !before.set || before.level !== fresh.level });
        var first = $(root, '[data-sg-list]').firstChild;
        if (first && first.classList && first.classList.contains('sg-rs') && level() !== 'off' && first.animate) {
          var h = first.scrollHeight;
          first.style.overflow = 'hidden';
          var a = first.animate(level() === 'full' ? [{ height: '0px', opacity: 0 }, { height: h + 'px', opacity: 1 }] : [{ opacity: 0 }, { opacity: 1 }], { duration: 200, easing: EASE_OUT });
          a.onfinish = a.oncancel = function () { first.style.overflow = ''; };
        }
        $(root, '[data-sg-list]').scrollTop = 0;
        closeForm(root, true);
        say(root, 'Reason added.');
      })
      .catch(function (e2) {
        st.busy = false;
        save.disabled = false;
        err.textContent = e2 && e2.message && !/^status /.test(e2.message) ? e2.message : 'That did not save. Try again.';
        err.classList.add('sg-show');
      });
  }

  function seeToggle(elm) {
    var root = rootOf(elm);
    if (!root) return;
    var st = stateOf(root);
    if (st.busy) return;
    var next = st.data.playersSee === false;
    st.busy = true;
    call(base(root) + '/standing-settings', { method: 'PUT', body: { playersSee: next } })
      .then(readJSON)
      .then(function (r) {
        st.busy = false;
        st.data.playersSee = !!r.playersSee;
        renderDMState(root);
        say(root, r.playersSee ? 'Players can see this standing.' : 'Only the DM sees this standing.');
      })
      .catch(function () { st.busy = false; say(root, 'Could not change who sees this standing.'); });
  }

  window.ChronicleStanding = {
    toggle: toggle, close: close, menu: menu, subject: subject, find: find, filter: filter,
    dir: dir, vis: vis, addOpen: addOpen, addCancel: addCancel, addSave: addSave, seeToggle: seeToggle
  };

  // The block mounts as a widget only so its script loads on sight (ADR-063);
  // every handler is inline. The server drew the first subject; the first
  // open draws the reasons.
  if (window.Chronicle && window.Chronicle.register) {
    window.Chronicle.register('standing', {
      init: function (node) {
        var r = rootOf(node) || node;
        if (r.querySelector && r.querySelector('[data-sg-list]')) renderDMState(r);
      },
      destroy: function (node) {
        var r = rootOf(node) || node;
        if (r.__sg) {
          if (r.__sg.onKey) document.removeEventListener('keydown', r.__sg.onKey);
          if (r.__sg.onAway) document.removeEventListener('click', r.__sg.onAway);
        }
      }
    });
  }
})();
