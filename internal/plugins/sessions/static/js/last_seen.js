/**
 * Last seen
 *
 * The entity page's "Last seen" block (internal/plugins/sessions/
 * last_seen_block.templ) is one quiet line: when the party last met this page,
 * and a count of nights. This file is the behaviour behind its inline
 * handlers:
 *
 *   - A click on the line grows a sheet out of it (a container transform) and
 *     the sheet rolls back into the line on close. It floats over the page, so
 *     nothing below moves. It has a visible Close and closes on Escape; a click
 *     outside does not close it (a panel closes only on purpose).
 *   - The sheet lists every night the page was seen, newest first, each with
 *     where it came from, behind an "Only seen / Every night" toggle and a
 *     strip of game nights.
 *   - For the Director only (the server does not render those controls for
 *     anyone else, and the sightings route does not send a player the data):
 *     suggested nights to add or dismiss, "Add a sighting", and Remove.
 *
 * Data: GET sessions/sightings?entity= (what the viewer may know),
 * GET sessions/sightings/nights (the played nights), PUT and DELETE
 * sessions/:sid/sightings (the Director's answers). This file makes no
 * permission decision; it draws what the server sent. Text reaches the DOM only
 * through textContent. Motion follows html[data-motion] (calm fades, off shows
 * the end state) and a device asking for reduced motion is always off.
 *
 * The handlers are inline IIFEs (the block can arrive by an HTMX swap), so this
 * exposes window.ChronicleLastSeen. The one document keydown listener (Escape)
 * lives only while a sheet is open.
 */
(function () {
  'use strict';

  var MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  var MAX_CELL_PX = 35;

  // ---- Pure helpers (unit-tested in test/js/last_seen.test.mjs) ----

  /** motionOf: how much motion the page allows ('full' | 'calm' | 'off'). */
  function motionOf(attr, reduced) {
    if (reduced) return 'off';
    return attr === 'calm' || attr === 'off' ? attr : 'full';
  }

  function parseDate(date) {
    var m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date || '');
    return m ? Date.UTC(+m[1], +m[2] - 1, +m[3]) : NaN;
  }

  /**
   * agoText: how long before `nowMs` a night (YYYY-MM-DD) was. The same rule
   * as sessions.AgoText on the server, so the line reads alike before and
   * after the Director changes it.
   */
  function agoText(date, nowMs) {
    var t = parseDate(date);
    if (isNaN(t)) return '';
    var n = new Date(nowMs);
    var today = Date.UTC(n.getFullYear(), n.getMonth(), n.getDate());
    var days = Math.round((today - t) / 86400000);
    if (days < 0) return 'coming up';
    if (days === 0) return 'today';
    if (days === 1) return 'yesterday';
    if (days < 14) return days + ' days ago';
    var weeks = Math.round(days / 7);
    if (weeks < 9) return weeks + ' weeks ago';
    var months = Math.round(days / 30.44);
    if (months < 24) return months + ' months ago';
    return Math.round(days / 365.25) + ' years ago';
  }

  /** fmtDate: "10 Oct 2026" for a YYYY-MM-DD night; '' for a series with no night. */
  function fmtDate(date) {
    var t = parseDate(date);
    if (isNaN(t)) return '';
    var d = new Date(t);
    return d.getUTCDate() + ' ' + MON[d.getUTCMonth()] + ' ' + d.getUTCFullYear();
  }

  /** nightsLabel: "1 night" or "N nights". */
  function nightsLabel(n) {
    return n === 1 ? '1 night' : n + ' nights';
  }

  /** itemKey: a night is one session on one date; a series-wide link has no date. */
  function itemKey(it) {
    return it.sessionId + '|' + (it.date || '');
  }

  // cmpNewest orders latest night first; a series with no night sorts last
  // (its date is empty), then by name so the order is stable.
  function cmpNewest(a, b) {
    var da = a.date || '';
    var db = b.date || '';
    if (da !== db) return da < db ? 1 : -1;
    var na = a.sessionName || '';
    var nb = b.sessionName || '';
    return na < nb ? -1 : na > nb ? 1 : 0;
  }

  /** newestFirst: a copy of the list, latest night first. */
  function newestFirst(list) {
    return list.slice().sort(cmpNewest);
  }

  /** rowTitle: a night's name; a series with no night says so. */
  function rowTitle(it) {
    return it.date ? it.sessionName : 'Seen during ' + it.sessionName + ', night not set';
  }

  /** sourceText: where a sighting came from, in small muted words. */
  function sourceText(it, dm, userId) {
    // How a night was recorded is the Director's business; a player is only
    // told that the party was there.
    if (!dm) return 'From the game night';
    if (it.source === 'manual') {
      return dm && it.addedBy && it.addedBy === userId ? 'Added by you' : 'Added by the DM';
    }
    if (it.source === 'checkin') return 'From the game night check-in';
    return 'From the game night';
  }

  /**
   * isStored: whether Remove can clear it. A link row lives on the game night
   * itself, so Remove there only points at the night.
   */
  function isStored(it) {
    return it.source === 'checkin' || it.source === 'manual';
  }

  /** lineParts: what the line says, from the newest sighting. */
  function lineParts(seen, nowMs) {
    var list = newestFirst(seen || []);
    if (!list.length) return { what: 'Not seen yet', when: '' };
    var top = list[0];
    if (!top.date) return { what: 'during ' + top.sessionName + ', night not set', when: '' };
    return { what: top.sessionName, when: agoText(top.date, nowMs) };
  }

  /** suggestLabel: the Director's prompt over the suggested nights. */
  function suggestLabel(n, name) {
    return n + (n === 1 ? ' night mentions ' : ' nights mention ') + name + '. Add?';
  }

  /**
   * allRows: every night for "Every night", newest first. A night with no
   * answer is a 'none' row; a seen one keeps its sighting. Nights the page
   * has an answer for but the played list lacks (a link on a night still ahead,
   * or past the list's bound) are kept so nothing seen disappears.
   */
  function allRows(nights, seen) {
    var byKey = {};
    var rows = [];
    (seen || []).forEach(function (s) {
      byKey[itemKey(s)] = true;
      rows.push({ state: 'seen', it: s });
    });
    (nights || []).forEach(function (n) {
      if (byKey[itemKey(n)]) return;
      rows.push({ state: 'none', it: n });
    });
    return rows.sort(function (a, b) { return cmpNewest(a.it, b.it); });
  }

  /**
   * stripBlocks: the nights, newest first, squeezed into at most `cells`
   * blocks when they outnumber what fits. Each block counts the seen nights it
   * holds. `flags` is one boolean per night (seen or not).
   */
  function stripBlocks(flags, cells) {
    var total = flags.length;
    var n = Math.max(1, Math.min(total, cells));
    var out = [];
    if (!total) return out;
    for (var b = 0; b < n; b++) {
      var from = Math.floor(b * total / n);
      var to = Math.floor((b + 1) * total / n);
      var count = 0;
      for (var k = from; k < to; k++) if (flags[k]) count++;
      out.push({ from: from, to: to, size: to - from, count: count });
    }
    return out;
  }

  /** blockLevel: how dark a strip block is, by the share of its nights seen. */
  function blockLevel(count, size) {
    if (!count) return 0;
    if (size === 1 || count / size > 0.7) return 3;
    return count / size > 0.4 ? 2 : 1;
  }

  /** escapeShouldClose: Escape closes, unless a modifier is held. */
  function escapeShouldClose(e) {
    if (!e || e.key !== 'Escape') return false;
    return !(e.ctrlKey || e.metaKey || e.altKey);
  }

  /** nightPageURL: the game night's page; a series with no night opens its own page. */
  function nightPageURL(campaignId, it) {
    var base = '/campaigns/' + encodeURIComponent(campaignId) + '/game-nights?session=' + encodeURIComponent(it.sessionId);
    return it.date ? base + '&date=' + encodeURIComponent(it.date) : base;
  }

  var api = {
    motionOf: motionOf,
    agoText: agoText,
    fmtDate: fmtDate,
    nightsLabel: nightsLabel,
    itemKey: itemKey,
    newestFirst: newestFirst,
    rowTitle: rowTitle,
    sourceText: sourceText,
    isStored: isStored,
    lineParts: lineParts,
    suggestLabel: suggestLabel,
    allRows: allRows,
    stripBlocks: stripBlocks,
    blockLevel: blockLevel,
    escapeShouldClose: escapeShouldClose,
    nightPageURL: nightPageURL
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof window === 'undefined' || typeof document === 'undefined') return;

  // ---- Browser part ----

  var SHEET_MS = 300;

  function level() {
    var reduced = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    return motionOf(document.documentElement.getAttribute('data-motion'), reduced);
  }

  function rootOf(el) { return el && el.closest ? el.closest('.ls') : null; }
  function $(root, sel) { return root.querySelector(sel); }

  function make(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  }

  function icon(name) {
    var i = make('i', 'fa-solid ' + name);
    i.setAttribute('aria-hidden', 'true');
    return i;
  }

  function dmTag() { return make('span', 'ls-dmtag', 'DM'); }

  // Per-block state lives on the block element, so two blocks on a page do not
  // share an open sheet.
  function stateOf(root) {
    if (!root.__ls) {
      root.__ls = {
        open: false, mode: 'seen', sugOpen: false, formOpen: false, data: null, nights: null,
        nightsFailed: false, loading: false, error: '', undo: null, onKey: null, tipTimer: 0, wired: false, busy: false
      };
    }
    return root.__ls;
  }

  function isDM(root) { return root.getAttribute('data-dm') === '1'; }
  function campaignBase(root) { return '/campaigns/' + encodeURIComponent(root.getAttribute('data-campaign')) + '/sessions'; }

  // ---- Talking to the server ----

  function call(url, opts) {
    if (window.Chronicle && window.Chronicle.apiFetch) return window.Chronicle.apiFetch(url, opts);
    opts = opts || {};
    return fetch(url, { method: opts.method || 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'Content-Type': 'application/json' }, body: opts.body ? JSON.stringify(opts.body) : undefined });
  }

  function readJSON(res) {
    if (!res.ok) return Promise.reject(new Error('status ' + res.status));
    return res.json();
  }

  function fetchSightings(root) {
    return call(campaignBase(root) + '/sightings?entity=' + encodeURIComponent(root.getAttribute('data-entity'))).then(readJSON);
  }

  function fetchNights(root) {
    return call(campaignBase(root) + '/sightings/nights').then(readJSON);
  }

  function writeSighting(root, it, state, source) {
    return call(campaignBase(root) + '/' + encodeURIComponent(it.sessionId) + '/sightings', {
      method: 'PUT',
      body: { entityId: root.getAttribute('data-entity'), date: it.date, state: state, source: source || 'manual' }
    }).then(function (res) { if (!res.ok) throw new Error('status ' + res.status); });
  }

  function clearSighting(root, it) {
    return call(campaignBase(root) + '/' + encodeURIComponent(it.sessionId) + '/sightings?entity=' +
      encodeURIComponent(root.getAttribute('data-entity')) + '&date=' + encodeURIComponent(it.date), { method: 'DELETE' })
      .then(function (res) { if (!res.ok) throw new Error('status ' + res.status); });
  }

  // ---- Drawing ----

  function seenList(st) { return newestFirst((st.data && st.data.seen) || []); }
  function sugList(st) { return newestFirst((st.data && st.data.suggestions) || []); }

  function renderLine(root) {
    var st = stateOf(root);
    var seen = seenList(st);
    var p = lineParts(seen, Date.now());
    $(root, '[data-ls-what]').textContent = p.what;
    $(root, '[data-ls-when]').textContent = p.when;
    $(root, '[data-ls-count]').textContent = nightsLabel(seen.length);
    var tog = $(root, '[data-ls-tog]');
    tog.setAttribute('aria-label', nightsLabel(seen.length) + ' seen, ' + (st.open ? 'close' : 'open') + ' the list');
    var rev = $(root, '[data-ls-rev]');
    if (rev) {
      rev.textContent = '';
      var n = sugList(st).length;
      if (n) {
        rev.appendChild(dmTag());
        rev.appendChild(document.createTextNode(n + ' to review'));
      }
    }
    measure(root);
  }

  // The sheet starts as the line's own rectangle, so its height is measured.
  function measure(root) {
    var line = $(root, '.ls-line');
    if (line) $(root, '.ls-wrap').style.setProperty('--lh', line.offsetHeight + 'px');
  }

  function renderSummary(root) {
    var st = stateOf(root);
    var seen = seenList(st).length;
    var total = st.nights ? st.nights.length : 0;
    var txt = total ? seen + ' of ' + total + ' nights' + (st.mode === 'all' ? ', seen ones marked' : '') : nightsLabel(seen);
    $(root, '[data-ls-sum]').textContent = txt;
  }

  function sourceLine(root, it) {
    var line = make('div', 'ls-rsrc');
    line.appendChild(make('span', '', sourceText(it, isDM(root), root.getAttribute('data-user'))));
    var a = make('a', 'ls-lnk', 'Open game night');
    a.setAttribute('href', nightPageURL(root.getAttribute('data-campaign'), it));
    line.appendChild(a);
    var tools = make('span', 'ls-sp');
    if (isDM(root)) {
      var rm = make('button', 'ls-q ls-mute', 'Remove');
      rm.type = 'button';
      rm.setAttribute('aria-label', 'Remove ' + rowTitle(it) + ' from Last seen');
      rm.setAttribute('data-dm-only', '');
      rm.addEventListener('click', function () { onRemove(root, it, rm.closest('.ls-nrow')); });
      tools.appendChild(rm);
    }
    line.appendChild(tools);
    return line;
  }

  function seenRow(root, it) {
    var row = make('li', 'ls-nrow');
    row.setAttribute('data-key', itemKey(it));
    var r1 = make('div', 'ls-r1');
    r1.appendChild(make('span', 'ls-ti', rowTitle(it)));
    if (it.date) r1.appendChild(make('span', 'ls-dt', fmtDate(it.date)));
    row.appendChild(r1);
    row.appendChild(sourceLine(root, it));
    return row;
  }

  function noneRow(it) {
    var row = make('li', 'ls-nrow ls-no');
    row.setAttribute('data-key', itemKey(it));
    row.appendChild(make('span', '', it.sessionName));
    row.appendChild(make('span', 'ls-dt', fmtDate(it.date) + ' · not seen'));
    return row;
  }

  function nightOptions(select, nights) {
    select.textContent = '';
    nights.forEach(function (n) {
      var o = make('option', '', n.sessionName + ', ' + fmtDate(n.date));
      o.value = itemKey(n);
      select.appendChild(o);
    });
  }

  function suggestionRow(root, it) {
    var st = stateOf(root);
    var row = make('li', 'ls-nrow ls-sg');
    row.setAttribute('data-key', itemKey(it));
    var r1 = make('div', 'ls-r1');
    r1.appendChild(make('span', 'ls-ti', rowTitle(it)));
    if (it.date) r1.appendChild(make('span', 'ls-dt', fmtDate(it.date)));
    row.appendChild(r1);
    var src = make('div', 'ls-rsrc');
    src.appendChild(make('span', '', 'Mentioned on this game night'));
    var tools = make('span', 'ls-sp');
    if (it.date) {
      var add = make('button', 'ls-q', 'Add');
      add.type = 'button';
      add.setAttribute('aria-label', 'Add ' + it.sessionName + ', ' + fmtDate(it.date) + ' as a sighting');
      add.addEventListener('click', function () { answer(root, row, it, 'seen'); });
      var no = make('button', 'ls-q ls-mute', 'Not seen');
      no.type = 'button';
      no.setAttribute('aria-label', it.sessionName + ', ' + fmtDate(it.date) + ' is not a sighting');
      no.addEventListener('click', function () { answer(root, row, it, 'not_seen'); });
      tools.appendChild(add);
      tools.appendChild(no);
    } else {
      // A series-wide mention names no night, so the Director picks which one.
      var mine = (st.nights || []).filter(function (n) { return n.sessionId === it.sessionId; });
      if (mine.length) {
        var sel = make('select', 'ls-pick');
        sel.setAttribute('aria-label', 'Which night of ' + it.sessionName);
        nightOptions(sel, mine);
        var go = make('button', 'ls-q', 'Add');
        go.type = 'button';
        go.addEventListener('click', function () {
          var chosen = null;
          mine.forEach(function (n) { if (itemKey(n) === sel.value) chosen = n; });
          if (chosen) answer(root, row, chosen, 'seen');
        });
        tools.appendChild(sel);
        tools.appendChild(go);
      } else {
        tools.appendChild(make('span', 'ls-none', 'No played night yet'));
      }
    }
    src.appendChild(tools);
    row.appendChild(src);
    return row;
  }

  function renderSuggestions(root) {
    var st = stateOf(root);
    if (!isDM(root)) return;
    var sugs = sugList(st);
    var tog = $(root, '[data-ls-sugtog]');
    var none = $(root, '[data-ls-nosug]');
    tog.hidden = !sugs.length;
    none.hidden = !!sugs.length;
    if (sugs.length) $(root, '[data-ls-suglbl]').textContent = suggestLabel(sugs.length, root.getAttribute('data-entity-name'));
    tog.setAttribute('aria-expanded', st.sugOpen && sugs.length ? 'true' : 'false');
  }

  function renderRows(root, joined) {
    var st = stateOf(root);
    var scroll = $(root, '.ls-scroll');
    scroll.textContent = '';
    if (st.loading) { scroll.appendChild(make('p', 'ls-empty', 'Loading the nights')); return; }
    if (st.error) {
      var err = make('p', 'ls-empty');
      err.appendChild(document.createTextNode(st.error + ' '));
      var retry = make('button', 'ls-q', 'Try again');
      retry.type = 'button';
      retry.addEventListener('click', function () { load(root); });
      err.appendChild(retry);
      scroll.appendChild(err);
      return;
    }
    var sugs = isDM(root) ? sugList(st) : [];
    if (sugs.length) {
      var wrap = make('div', 'ls-sugg' + (st.sugOpen ? ' ls-on' : ''));
      wrap.setAttribute('data-ls-sugg', '');
      wrap.setAttribute('data-dm-only', '');
      var inner = make('div');
      var box = make('div', 'ls-sugg-in');
      var head = make('div', 'ls-sugg-h');
      head.appendChild(dmTag());
      head.appendChild(make('span', '', 'Only you see these. Nothing joins until you add it.'));
      var dated = sugs.filter(function (s) { return s.date; });
      if (dated.length > 1) {
        var all = make('button', 'ls-q ls-sp', 'Add all ' + dated.length);
        all.type = 'button';
        all.addEventListener('click', function () { addAll(root, dated); });
        head.appendChild(all);
      }
      box.appendChild(head);
      var ul = make('ul', 'ls-rows');
      sugs.forEach(function (s) { ul.appendChild(suggestionRow(root, s)); });
      box.appendChild(ul);
      inner.appendChild(box);
      wrap.appendChild(inner);
      if (!st.sugOpen) wrap.setAttribute('aria-hidden', 'true');
      scroll.appendChild(wrap);
    }
    if (st.undo) scroll.appendChild(undoRow(root));

    var rows;
    if (st.mode === 'all') rows = allRows(st.nights || [], seenList(st));
    else rows = seenList(st).map(function (s) { return { state: 'seen', it: s }; });
    if (!rows.length) {
      scroll.appendChild(make('p', 'ls-empty', 'Nobody has met ' + root.getAttribute('data-entity-name') +
        ' yet. Nights where this page is linked as encountered or key fill this in by themselves.'));
      return;
    }
    var list = make('ul', 'ls-rows');
    rows.forEach(function (r) {
      var li = r.state === 'seen' ? seenRow(root, r.it) : noneRow(r.it);
      if (joined && joined.indexOf(itemKey(r.it)) >= 0) li.classList.add('ls-join');
      list.appendChild(li);
    });
    scroll.appendChild(list);
  }

  function undoRow(root) {
    var st = stateOf(root);
    var row = make('div', 'ls-nrow ls-undo');
    row.setAttribute('role', 'status');
    row.appendChild(make('span', '', rowTitle(st.undo) + ' left out of Last seen.'));
    var b = make('button', 'ls-q ls-sp', 'Undo');
    b.type = 'button';
    b.addEventListener('click', function () {
      var it = st.undo;
      st.undo = null;
      run(root, writeSighting(root, it, 'seen', it.source), { join: [itemKey(it)] });
    });
    row.appendChild(b);
    return row;
  }

  function renderStrip(root) {
    var st = stateOf(root);
    var strip = $(root, '.ls-strip');
    var W = strip.clientWidth;
    if (W <= 0) return;
    var seen = seenList(st).filter(function (s) { return s.date; });
    var seenKeys = {};
    seen.forEach(function (s) { seenKeys[itemKey(s)] = true; });
    var nights = st.nights && st.nights.length ? st.nights : seen;
    var flags = nights.map(function (n) { return !!seenKeys[itemKey(n)]; });
    strip.textContent = '';
    if (!nights.length) return;
    var cells = Math.max(1, Math.floor((W + 3) / MAX_CELL_PX));
    stripBlocks(flags, cells).forEach(function (b) {
      var first = nights[b.from];
      var last = nights[b.to - 1];
      var target = null;
      for (var k = b.from; k < b.to; k++) if (flags[k]) { target = nights[k]; break; }
      if (!target && st.mode === 'all') target = first;
      var lv = blockLevel(b.count, b.size);
      var cell = make('button', 'ls-cell' + (target ? ' ls-go' : '') + (lv ? ' ls-lv' + lv : ''));
      cell.type = 'button';
      var name = b.size === 1 ? first.sessionName + ', ' + fmtDate(first.date) : fmtDate(last.date) + ' to ' + fmtDate(first.date);
      var status = b.size === 1 ? (b.count ? 'seen' : 'not seen') : b.count + ' of ' + b.size + ' seen';
      cell.setAttribute('data-name', name);
      cell.setAttribute('data-status', status);
      cell.setAttribute('aria-label', name + ', ' + status + (target ? '. Go to this night' : ''));
      if (target) cell.setAttribute('data-key', itemKey(target));
      else { cell.setAttribute('aria-disabled', 'true'); cell.tabIndex = -1; }
      cell.appendChild(make('i'));
      strip.appendChild(cell);
    });
  }

  function renderAll(root, opts) {
    renderLine(root);
    renderSummary(root);
    renderSuggestions(root);
    renderRows(root, opts && opts.join);
    renderStrip(root);
  }

  // ---- Loading and answering ----

  function load(root) {
    var st = stateOf(root);
    st.loading = true;
    st.error = '';
    renderRows(root);
    // The nights list only decorates the strip, "Every night" and the pickers,
    // so a failure there leaves the rest working.
    var nights = fetchNights(root).then(function (r) { st.nights = r.nights || []; st.nightsFailed = false; },
      function () { st.nights = st.nights || []; st.nightsFailed = true; });
    var sightings = fetchSightings(root).then(function (d) { st.data = d; });
    return Promise.all([sightings, nights]).then(function () {
      st.loading = false;
      renderAll(root);
    }, function () {
      st.loading = false;
      st.error = 'Could not load the nights.';
      renderRows(root);
    });
  }

  // Refetches after a write so the sheet shows what the server now holds
  // (a removed stored row can leave its link behind, a suggestion can move).
  function run(root, promise, opts) {
    var st = stateOf(root);
    if (st.busy) return Promise.resolve();
    st.busy = true;
    return promise.then(function () { return fetchSightings(root); }).then(function (d) {
      st.data = d;
      st.busy = false;
      renderAll(root, opts);
      if (opts && opts.join && opts.join.length) flash(root, opts.join[0]);
    }, function () {
      st.busy = false;
      st.error = 'That did not save. Nothing changed.';
      renderRows(root);
    });
  }

  function leaveThen(row, fn) {
    if (!row || level() === 'off') { fn(); return; }
    row.classList.add('ls-leave');
    setTimeout(fn, level() === 'calm' ? 200 : 210);
  }

  function answer(root, row, it, state) {
    var join = state === 'seen' ? [itemKey(it)] : null;
    leaveThen(row, function () { run(root, writeSighting(root, it, state, 'manual'), { join: join }); });
  }

  function addAll(root, items) {
    var st = stateOf(root);
    var rows = [].slice.call($(root, '.ls-scroll').querySelectorAll('.ls-sg'));
    rows.forEach(function (r, i) {
      if (level() === 'full') r.style.animationDelay = (i * 30) + 'ms';
      if (level() !== 'off') r.classList.add('ls-leave');
    });
    var keys = items.map(itemKey);
    setTimeout(function () {
      var chain = Promise.resolve();
      items.forEach(function (it) { chain = chain.then(function () { return writeSighting(root, it, 'seen', 'manual'); }); });
      st.sugOpen = false;
      run(root, chain, { join: keys });
    }, level() === 'off' ? 0 : 260);
  }

  function onRemove(root, it, row) {
    var st = stateOf(root);
    if (!isStored(it)) {
      // A link lives on the game night itself, so Remove changes nothing here:
      // it says where the link is and offers the night.
      showLinkNote(root, row, it);
      return;
    }
    leaveThen(row, function () {
      st.undo = it;
      run(root, clearSighting(root, it));
    });
  }

  function showLinkNote(root, row, it) {
    var existing = row.querySelector('.ls-linknote');
    if (existing) { existing.remove(); return; }
    var note = make('p', 'ls-linknote');
    note.setAttribute('role', 'status');
    note.appendChild(document.createTextNode('This comes from a link on the game night, so the link stays there. '));
    var a = make('a', 'ls-lnk', 'Open the game night');
    a.setAttribute('href', nightPageURL(root.getAttribute('data-campaign'), it));
    note.appendChild(a);
    note.appendChild(document.createTextNode(' to change it.'));
    row.appendChild(note);
  }

  function flash(root, key) {
    var rows = $(root, '.ls-scroll').querySelectorAll('.ls-nrow');
    for (var i = 0; i < rows.length; i++) {
      if (rows[i].getAttribute('data-key') === key) { glow(rows[i]); return; }
    }
  }

  function glow(row) {
    row.style.transition = 'none';
    row.classList.add('ls-glow');
    void row.offsetWidth;
    setTimeout(function () {
      row.style.transition = 'background-color 1.2s, box-shadow 1.2s';
      row.classList.remove('ls-glow');
      setTimeout(function () { row.style.transition = ''; }, 1300);
    }, level() === 'off' ? 1200 : 150);
  }

  function jump(root, key) {
    var scroll = $(root, '.ls-scroll');
    var rows = scroll.querySelectorAll('.ls-nrow');
    for (var i = 0; i < rows.length; i++) {
      if (rows[i].getAttribute('data-key') !== key) continue;
      var top = rows[i].getBoundingClientRect().top - scroll.getBoundingClientRect().top + scroll.scrollTop - 4;
      if (typeof scroll.scrollTo === 'function') scroll.scrollTo({ top: top, behavior: level() === 'full' ? 'smooth' : 'auto' });
      else scroll.scrollTop = top;
      glow(rows[i]);
      return;
    }
  }

  // ---- The strip's peek ----

  function wireStrip(root) {
    var st = stateOf(root);
    if (st.wired) return;
    st.wired = true;
    var strip = $(root, '.ls-strip');
    var tip = $(root, '.ls-tip');
    var dh = $(root, '.ls-dh');
    function hide() { clearTimeout(st.tipTimer); tip.classList.remove('ls-show'); }
    function show(cell, now) {
      clearTimeout(st.tipTimer);
      st.tipTimer = setTimeout(function () {
        tip.textContent = '';
        tip.appendChild(document.createTextNode(cell.getAttribute('data-name') + ' '));
        tip.appendChild(make('span', '', '· ' + cell.getAttribute('data-status')));
        var sr = strip.getBoundingClientRect();
        var cr = cell.getBoundingClientRect();
        var hr = dh.getBoundingClientRect();
        var w = tip.offsetWidth;
        var cx = cr.left - hr.left + cr.width / 2;
        tip.style.left = Math.max(w / 2, Math.min(dh.clientWidth - w / 2, cx)) + 'px';
        tip.style.top = (sr.top - hr.top + cr.height + 2) + 'px';
        tip.classList.add('ls-show');
      }, (now || level() === 'off') ? 0 : 250);
    }
    strip.addEventListener('mouseover', function (e) { var c = e.target.closest('.ls-cell'); if (c) show(c); });
    strip.addEventListener('mouseout', function (e) { if (e.target.closest('.ls-cell')) hide(); });
    strip.addEventListener('focusin', function (e) { var c = e.target.closest('.ls-cell'); if (c) show(c, true); });
    strip.addEventListener('focusout', hide);
    strip.addEventListener('click', function (e) {
      var c = e.target.closest('.ls-cell');
      if (!c || c.getAttribute('aria-disabled')) return;
      hide();
      jump(root, c.getAttribute('data-key'));
    });
    if (window.ResizeObserver) new ResizeObserver(function () { if (st.open) renderStrip(root); }).observe(strip);
  }

  // ---- The sheet ----

  function setOpen(root, on) {
    var st = stateOf(root);
    if (on === st.open) return;
    st.open = on;
    if (!on) closeForm(root, true);
    measure(root);
    var drape = $(root, '.ls-drape');
    $(root, '[data-ls-tog]').setAttribute('aria-expanded', on ? 'true' : 'false');
    drape.classList.toggle('ls-on', on);
    if (on) {
      wireStrip(root);
      if (!st.data && !st.loading) load(root);
      else renderAll(root);
      if (!st.onKey) {
        st.onKey = function (e) {
          if (!root.isConnected) { document.removeEventListener('keydown', st.onKey); st.onKey = null; return; }
          if (!st.open || !escapeShouldClose(e)) return;
          e.preventDefault();
          if (st.formOpen) { closeForm(root); var ao = $(root, '[data-ls-addopen]'); if (ao) ao.focus(); return; }
          setOpen(root, false);
          $(root, '[data-ls-tog]').focus();
        };
        document.addEventListener('keydown', st.onKey);
      }
      setTimeout(function () { renderStrip(root); }, 0);
    } else {
      if (st.onKey) { document.removeEventListener('keydown', st.onKey); st.onKey = null; }
      st.undo = null;
    }
    renderLine(root);
  }

  function toggle(el) {
    var root = rootOf(el);
    if (root) setOpen(root, !stateOf(root).open);
  }

  function close(el) {
    var root = rootOf(el);
    if (!root) return;
    setOpen(root, false);
    $(root, '[data-ls-tog]').focus();
  }

  function mode(el, m) {
    var root = rootOf(el);
    if (!root) return;
    var st = stateOf(root);
    if (m === st.mode) return;
    st.mode = m;
    [].forEach.call(root.querySelectorAll('.ls-seg button'), function (b) {
      b.setAttribute('aria-pressed', b.getAttribute('data-v') === m ? 'true' : 'false');
    });
    closeForm(root, true);
    renderAll(root);
    $(root, '.ls-scroll').scrollTop = 0;
  }

  function suggestions(el) {
    var root = rootOf(el);
    if (!root) return;
    var st = stateOf(root);
    st.sugOpen = !st.sugOpen;
    var wrap = $(root, '[data-ls-sugg]');
    if (wrap) {
      wrap.classList.toggle('ls-on', st.sugOpen);
      wrap.setAttribute('aria-hidden', st.sugOpen ? 'false' : 'true');
    }
    $(root, '[data-ls-sugtog]').setAttribute('aria-expanded', st.sugOpen ? 'true' : 'false');
    if (st.sugOpen) $(root, '.ls-scroll').scrollTop = 0;
  }

  // ---- Add a sighting: the word becomes the form ----

  function placeForm(root) {
    var wrap = $(root, '.ls-wrap');
    var form = $(root, '[data-ls-addf]');
    var open = $(root, '[data-ls-addopen]');
    var wr = wrap.getBoundingClientRect();
    var o = open.getBoundingClientRect();
    var W = Math.min(340, wr.width - 8);
    form.style.width = W + 'px';
    var H = form.offsetHeight;
    var right = o.right - wr.left;
    var left = Math.max(0, right - W);
    var top = o.top - wr.top;
    form.style.left = left + 'px';
    form.style.top = top + 'px';
    var radius = '8px';
    form.style.setProperty('--cs', 'inset(' + Math.max(0, 0) + 'px ' + Math.max(0, left + W - right) + 'px ' +
      Math.max(0, H - o.height) + 'px ' + Math.max(0, (o.left - wr.left) - left) + 'px round ' + radius + ')');
  }

  function fillForm(root) {
    var st = stateOf(root);
    var select = $(root, '[data-ls-night]');
    var seenKeys = {};
    seenList(st).forEach(function (s) { seenKeys[itemKey(s)] = true; });
    var open = (st.nights || []).filter(function (n) { return !seenKeys[itemKey(n)]; });
    select.textContent = '';
    if (!open.length) {
      var o = make('option', '', st.nightsFailed ? 'The nights could not be loaded' : 'Every night already counts');
      o.value = '';
      select.appendChild(o);
    } else {
      nightOptions(select, open);
    }
    select.disabled = !open.length;
    $(root, '[data-ls-addgo]').disabled = !open.length;
    root.__ls.formNights = open;
  }

  function addOpen(el) {
    var root = rootOf(el);
    if (!root) return;
    var st = stateOf(root);
    if (st.formOpen || !st.open) return;
    st.formOpen = true;
    fillForm(root);
    placeForm(root);
    var form = $(root, '[data-ls-addf]');
    void form.offsetWidth;
    $(root, '[data-ls-addopen]').setAttribute('aria-expanded', 'true');
    form.classList.add('ls-on');
    setTimeout(function () { if (st.formOpen) $(root, '[data-ls-night]').focus(); }, level() === 'off' ? 0 : 120);
  }

  function closeForm(root, now) {
    var st = stateOf(root);
    if (!st.formOpen) return;
    st.formOpen = false;
    var form = $(root, '[data-ls-addf]');
    if (!form) return;
    if (now === true) form.style.transition = 'none';
    form.classList.remove('ls-on');
    var open = $(root, '[data-ls-addopen]');
    if (open) open.setAttribute('aria-expanded', 'false');
    if (now === true) { void form.offsetWidth; form.style.transition = ''; }
  }

  function addClose(el) {
    var root = rootOf(el);
    if (!root) return;
    closeForm(root);
    var open = $(root, '[data-ls-addopen]');
    if (open) open.focus();
  }

  function addSave(el, e) {
    if (e && e.preventDefault) e.preventDefault();
    var root = rootOf(el);
    if (!root) return;
    var st = stateOf(root);
    var key = $(root, '[data-ls-night]').value;
    var chosen = null;
    (st.formNights || []).forEach(function (n) { if (itemKey(n) === key) chosen = n; });
    if (!chosen) return;
    closeForm(root);
    var open = $(root, '[data-ls-addopen]');
    if (open) open.focus();
    setTimeout(function () {
      st.undo = null;
      run(root, writeSighting(root, chosen, 'seen', 'manual'), { join: [itemKey(chosen)] });
    }, level() === 'off' ? 0 : Math.round(SHEET_MS * 0.6));
  }

  window.ChronicleLastSeen = {
    toggle: toggle, close: close, mode: mode, suggestions: suggestions,
    addOpen: addOpen, addClose: addClose, addSave: addSave
  };

  // The block mounts as a widget only so its script loads on sight (ADR-063);
  // every handler is inline.
  if (window.Chronicle && window.Chronicle.register) {
    window.Chronicle.register('last-seen', {
      init: function (el) { var r = rootOf(el) || el; if (r.querySelector) measure(r); },
      destroy: function (el) {
        var r = rootOf(el) || el;
        if (r.__ls && r.__ls.onKey) document.removeEventListener('keydown', r.__ls.onKey);
      }
    });
  }
})();
