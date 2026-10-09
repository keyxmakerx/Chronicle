/**
 * give_box.js -- the Give, Share and Move boxes (Chronicle.GiveBox).
 *
 * The server renders the boxes (internal/plugins/armory/give_box.templ,
 * share_box.templ, move_dialog.templ); this file opens and shuts them and sends what was picked.
 * On a character page a box opens in place: it grows out of a .ag-fold in the
 * "Items and money" panel (a line's Share and Move boxes under the line, on
 * the Stashes page too), as the roll-table editor does. On an Armory card
 * the ... button opens a .ag-pop popover whose "Give to..." turns into the
 * same box. One box is open at a time.
 *
 * A box holding an unsent choice does not close on a click outside or on
 * Escape: it shakes and shows its warn bar, whose Discard throws the choice
 * away. Cancel and the close button always close.
 *
 * Every control inside a server-rendered box calls in here through an inline
 * handler built Go-side (giveBoxCall), so nothing depends on a script inside
 * a swapped fragment. The outside-click and Escape listeners live only while
 * a box is open.
 */
(function () {
  'use strict';

  var S = null;      // the open box: {host, box, kind: 'fold'|'pop', btn, dirty}
  var busy = false;  // a box is loading; ignore further opens until it has
  var ICON_RE = /^fa-[a-z0-9-]{1,40}$/;

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function reduced() {
    var h = document.documentElement;
    return matchMedia('(prefers-reduced-motion: reduce)').matches || !!document.querySelector('.nav-rm, html.rm') ||
      h.hasAttribute('data-cz-reduce') || h.getAttribute('data-view-motion') === 'calm';
  }
  function notify(msg, kind) { if (window.Chronicle && Chronicle.notify) Chronicle.notify(msg, kind || 'error'); }

  // Opens or shuts a .ag-fold by animating its height to the content's.
  function fold(el, open, done) {
    var finished = false;
    function fin() {
      if (finished) return;
      finished = true;
      el.removeEventListener('transitionend', onEnd);
      if (open) el.style.height = 'auto';
      if (done) done();
    }
    function onEnd(e) { if (e.target === el && e.propertyName === 'height') fin(); }
    if (reduced()) { el.style.height = open ? 'auto' : '0px'; el.classList.toggle('is-open', open); finished = true; if (done) done(); return; }
    var from = el.getBoundingClientRect().height;
    el.style.height = 'auto';
    var to = open ? el.scrollHeight : 0;
    el.style.height = from + 'px';
    el.getBoundingClientRect();
    el.classList.toggle('is-open', open);
    el.style.height = to + 'px';
    el.addEventListener('transitionend', onEnd);
    // No transition fires when the height does not change.
    setTimeout(fin, 420);
  }

  // A box with an unsent choice shakes and says so instead of closing.
  function nag(box) {
    if (!box) return;
    var w = box.querySelector('.ag-warn');
    if (w) { w.classList.remove('is-on'); void w.offsetWidth; w.classList.add('is-on'); }
    box.classList.remove('is-nagging'); void box.offsetWidth; box.classList.add('is-nagging');
  }

  function toast(msg) {
    var old = document.querySelector('.ag-toast');
    if (old) old.remove();
    var t = document.createElement('div');
    t.className = 'ag-toast';
    t.setAttribute('role', 'status');
    t.innerHTML = '<i class="fa-solid fa-circle-check"></i><span></span>';
    t.querySelector('span').textContent = msg;
    document.body.appendChild(t);
    setTimeout(function () { t.remove(); }, 3200);
  }

  function outside(e) {
    if (!S) return;
    if (!S.host.isConnected) { forget(); return; }
    if (S.host.contains(e.target) || (S.btn && S.btn.contains(e.target))) return;
    close(false);
  }
  function onKey(e) {
    if (e.key !== 'Escape' || !S) return;
    var btn = S.btn;
    if (close(false) && btn && btn.isConnected) btn.focus();
  }
  function listen(on) {
    var m = on ? 'addEventListener' : 'removeEventListener';
    document[m]('mousedown', outside, true);
    document[m]('keydown', onKey, true);
  }
  // A swap took the box away (its panel reloaded): let it go quietly.
  function forget() { S = null; listen(false); }

  // close shuts the open box. Unless forced, an unsent choice nags instead
  // and close returns false. after runs once the box has gone.
  function close(force, after) {
    if (!S) { if (after) after(); return true; }
    if (!S.host.isConnected) { forget(); if (after) after(); return true; }
    if (!force && S.dirty) { nag(S.box); return false; }
    var s = S;
    forget();
    if (s.btn) s.btn.setAttribute('aria-expanded', 'false');
    if (s.kind === 'fold') {
      fold(s.host, false, function () { s.host.innerHTML = ''; if (after) after(); });
    } else {
      s.host.hidden = true;
      s.host.innerHTML = '';
      if (after) after();
    }
    return true;
  }

  // load fetches a server box into host through htmx, so its hx- attributes
  // (the search box) are live, and resolves with the box element or null.
  function load(url, host) {
    if (!window.htmx) return Promise.resolve(null);
    busy = true;
    return Promise.resolve(htmx.ajax('GET', url, { target: host, swap: 'innerHTML' }))
      .then(function () { busy = false; return host.querySelector('[data-box]'); },
        function () { busy = false; return null; });
  }

  function boxOf(el) { return el && el.closest('[data-box]'); }
  function mine(box) { return S && S.box === box ? S : null; }

  // ---- the Give box ----

  // The list the pick comes from: the visible one.
  function picked(box) {
    var lists = box.querySelectorAll('[data-list]');
    for (var i = 0; i < lists.length; i++) {
      if (lists[i].hidden) continue;
      var r = lists[i].querySelector('input[name=ag-pick]:checked');
      if (r) return r;
    }
    return null;
  }

  function hint(box) {
    var r = picked(box);
    var give = box.querySelector('[data-give]');
    var h = box.querySelector('[data-hint]');
    if (give) give.disabled = !r;
    if (box.hasAttribute('data-move-box')) moveReady(box);
    if (!h) return;
    var text = r && r.getAttribute('data-say');
    if (!text) { h.innerHTML = ''; return; }
    var icon = r.getAttribute('data-say-icon') || '';
    h.innerHTML = '<i class="fa-solid ' + (ICON_RE.test(icon) ? icon : 'fa-circle-info') + '"></i><span></span>';
    h.querySelector('span').textContent = text;
  }

  function warnOff(box) { var w = box.querySelector('.ag-warn'); if (w) w.classList.remove('is-on'); }

  function tab(btn) {
    var box = boxOf(btn); if (!box) return;
    var kind = btn.getAttribute('data-tab');
    box.setAttribute('data-kind', kind);
    box.querySelectorAll('[data-tab]').forEach(function (b) { b.setAttribute('aria-selected', b === btn ? 'true' : 'false'); });
    var s = box.querySelector('.ag-search'); if (s) s.hidden = kind !== 'items';
    box.querySelectorAll('[data-list]').forEach(function (l) { l.hidden = l.getAttribute('data-list') !== kind; });
    var q = box.querySelector('[data-qty]'); if (q) q.style.visibility = kind === 'map' ? 'hidden' : '';
    box.querySelectorAll('input[name=ag-pick]').forEach(function (r) { r.checked = false; });
    var st = mine(box); if (st) st.dirty = false;
    warnOff(box);
    hint(box);
  }

  function pick(input) {
    var box = boxOf(input); if (!box) return;
    var st = mine(box); if (st) st.dirty = true;
    warnOff(box);
    hint(box);
  }

  // After a search the pick may have gone from the list with the rows.
  function sync(box) {
    var st = mine(box); if (st) st.dirty = !!picked(box);
    hint(box);
  }

  // search asks the server for the items matching what was typed (it caps
  // the list and says when there are more) and swaps the list. Only the
  // latest answer is used.
  function search(input) {
    var box = boxOf(input); if (!box) return;
    var list = box.querySelector('[data-list="items"]');
    var url = input.getAttribute('data-search');
    if (!list || !url || url.indexOf('/campaigns/') !== 0) return;
    clearTimeout(box._agT);
    box._agT = setTimeout(function () {
      var seq = (box._agSeq = (box._agSeq || 0) + 1);
      Chronicle.apiFetch(url + '&q=' + encodeURIComponent(input.value.trim()), { headers: { 'HX-Request': 'true', Accept: 'text/html' } })
        .then(function (r) { return r.ok ? r.text() : Promise.reject(r); })
        .then(function (html) {
          if (seq !== box._agSeq) return;
          list.innerHTML = html;
          sync(box);
        })
        .catch(function () { if (seq === box._agSeq) notify('Could not search the Armory. Try again.'); });
    }, 250);
  }

  function clampQty(v) {
    var n = Math.floor(+v);
    if (!isFinite(n) || n < 1) n = 1;
    return Math.min(1000000, n);
  }

  function step(btn) {
    var box = boxOf(btn); if (!box) return;
    var i = box.querySelector('#ag-q'); if (!i) return;
    var max = +i.getAttribute('max') || 1000000;
    i.value = Math.min(max, clampQty(clampQty(i.value) + (+btn.getAttribute('data-q') || 0)));
    pick(i);
  }

  function cancel() { close(true); }
  function discard() { close(true); }

  function errorOf(r) {
    return r.json().then(function (j) { return (j && j.message) || 'Could not do that. Try again.'; },
      function () { return 'Could not do that. Try again.'; });
  }

  function trigger(r, name) {
    try { return (JSON.parse(r.headers.get('HX-Trigger') || '{}') || {})[name] || null; } catch (e) { return null; }
  }

  function give(btn) {
    var box = boxOf(btn); if (!box || btn.disabled) return;
    var r = picked(box); if (!r) return;
    var field = r.getAttribute('data-field');
    if (['item_id', 'map_id', 'character_id'].indexOf(field) < 0) return;
    var fd = new FormData();
    fd.append(field, r.value);
    if (box.getAttribute('data-character')) fd.append('character_id', box.getAttribute('data-character'));
    if (box.getAttribute('data-item')) fd.append('item_id', box.getAttribute('data-item'));
    var q = box.querySelector('#ag-q');
    fd.append('quantity', field === 'map_id' ? '1' : String(clampQty(q ? q.value : 1)));
    btn.disabled = true;
    Chronicle.apiFetch(box.getAttribute('data-post'), { method: 'POST', body: fd, headers: { 'HX-Request': 'true' } })
      .then(function (resp) {
        if (!resp.ok) return errorOf(resp).then(function (m) { btn.disabled = false; notify(m); });
        var g = trigger(resp, 'armory-given') || {};
        var st = mine(box); if (st) st.dirty = false;
        var panel = panelOf(box);
        // The panels reload once the box has folded away, then the line lands.
        close(true, function () { reload(panel ? panel.id : '', g.itemId); });
        if (g.message) toast(g.message);
      })
      .catch(function () { btn.disabled = false; notify('Network error. Try again.'); });
  }

  // reload tells every armory panel on the page to reload (armory-moved), and
  // when the one the give was made from comes back, flashes the line the
  // item went to. The listener lasts only until then.
  function reload(panelId, itemId, holders) {
    if (!window.htmx) return;
    var old = panelId && document.getElementById(panelId);
    if (old && itemId) {
      var stop = function () { document.body.removeEventListener('htmx:afterSettle', on); };
      var on = function () {
        var now = document.getElementById(panelId);
        if (!now || now === old) return;
        stop();
        flash(now, itemId, holders);
      };
      document.body.addEventListener('htmx:afterSettle', on);
      setTimeout(stop, 5000);
    }
    htmx.trigger(document.body, 'armory-moved');
  }

  // flash marks the line that changed. A give names its line; a move names
  // the item (or 'money') and the holders it left and reached, so another
  // stash holding the same item is left alone.
  function flash(section, key, holders) {
    var rows = section.querySelectorAll(key === 'money' ? '[data-money-row]' : '[data-item-id]');
    for (var i = 0; i < rows.length; i++) {
      var row = rows[i];
      if (key !== 'money' && row.getAttribute('data-item-id') !== key) continue;
      if (holders && holders.indexOf(row.getAttribute('data-holder')) < 0) continue;
      row.classList.add('ag-landed');
      if (!holders) return;
    }
  }

  // open toggles a box that folds out of the character panel: the Give box
  // (the header's "Give an item") or a line's Share box ("Share...").
  function open(btn, url) {
    var f, host;
    if (btn.hasAttribute('data-move')) {
      host = btn.closest('[data-move-host]');
      f = host && host.querySelector('[data-move-fold]');
    } else if (btn.hasAttribute('data-share')) {
      f = btn.closest('li') && btn.closest('li').querySelector('[data-share-fold]');
    } else {
      f = btn.closest('section') && btn.closest('section').querySelector('[data-give-fold]');
    }
    if (!f || busy) return;
    if (S && S.btn === btn) { close(false); return; }
    if (S && !close(false)) return;
    f.style.height = '0px';
    btn.setAttribute('aria-busy', 'true');
    load(url, f).then(function (box) {
      btn.removeAttribute('aria-busy');
      if (!box) { notify('Could not open that. Try again.'); return; }
      if (S) return;
      S = { host: f, box: box, kind: 'fold', btn: btn, dirty: false };
      btn.setAttribute('aria-expanded', 'true');
      listen(true);
      fold(f, true, function () {
        // The Give box starts in its search, as the mockup does; the Share box
        // takes focus itself so Tab reaches the first player without a ring
        // showing on a mouse open.
        var s = box.querySelector('#ag-search') || box.querySelector('input[name=ag-pick]') || box;
        s.focus({ preventScroll: true });
      });
    });
  }

  // ---- the Share box ----

  function tick(input) {
    var box = boxOf(input); if (!box) return;
    var st = mine(box); if (st) st.dirty = true;
    warnOff(box);
  }

  function save(btn) {
    var box = boxOf(btn); if (!box || btn.disabled) return;
    var fd = new FormData();
    box.querySelectorAll('input[name=user]:checked').forEach(function (c) { fd.append('user', c.value); });
    btn.disabled = true;
    Chronicle.apiFetch(box.getAttribute('data-post'), { method: 'POST', body: fd, headers: { 'HX-Request': 'true' } })
      .then(function (resp) {
        if (!resp.ok) return errorOf(resp).then(function (m) { btn.disabled = false; notify(m); });
        var g = trigger(resp, 'armory-shared') || {};
        var st = mine(box); if (st) st.dirty = false;
        var panel = panelOf(box);
        close(true, function () { reload(panel ? panel.id : '', g.itemId); });
        if (g.message) toast(g.message);
      })
      .catch(function () { btn.disabled = false; notify('Network error. Try again.'); });
  }

  // ---- the Move box ----

  // The panel a box lives in: the character's, or the Stashes page body.
  function panelOf(box) { return box.closest('section[id^="armory-panel-"], #armory-stashes'); }

  // Move stays disabled until a destination is chosen and the amount is
  // something the box can send.
  function moveReady(box) {
    var go = box.querySelector('[data-move-go]'); if (!go) return;
    var ok = !!picked(box);
    var q = box.querySelector('#ag-q');
    if (q) { var n = +q.value, max = +q.getAttribute('max') || 1000000; ok = ok && n >= 1 && n <= max && Math.floor(n) === n; }
    var a = box.querySelector('#ag-amount');
    if (a) ok = ok && a.value.trim() !== '';
    go.disabled = !ok || !!box._mvBusy;
  }

  function showErr(box, msg) {
    var bar = box.querySelector('.ag-warn[data-err]');
    if (!bar) { notify(msg); return; }
    bar.querySelector('span').textContent = msg;
    bar.classList.remove('is-on'); void bar.offsetWidth; bar.classList.add('is-on');
  }

  // move posts the choice to the moves route. While it waits the button shows
  // a spinner; on success it says so as the box folds away and the line it
  // landed on flashes; on failure the amber bar carries the server's words.
  function move(el) {
    var box = boxOf(el); if (!box) return;
    var go = box.querySelector('[data-move-go]');
    if (!go || go.disabled || box._mvBusy) return;
    var r = picked(box); if (!r) return;
    var kind = box.getAttribute('data-kind');
    var fd = new FormData();
    fd.append('kind', kind);
    fd.append('item_id', box.getAttribute('data-item-id') || '');
    fd.append('from_kind', box.getAttribute('data-from-kind') || '');
    fd.append('from_id', box.getAttribute('data-from-id') || '');
    fd.append('to', r.value);
    if (kind === 'money') {
      fd.append('amount', (box.querySelector('#ag-amount') || {}).value || '');
    } else {
      var q = box.querySelector('#ag-q');
      fd.append('quantity', q ? String(clampQty(q.value)) : '1');
    }
    var errBar = box.querySelector('.ag-warn[data-err]'); if (errBar) errBar.classList.remove('is-on');
    var label = go.getAttribute('data-label') || 'Move';
    box._mvBusy = true;
    go.disabled = true;
    go.innerHTML = '<span class="fx-spin" aria-hidden="true"></span><span></span>';
    go.lastChild.textContent = go.getAttribute('data-busy-label') || 'Moving…';
    function restore() { box._mvBusy = false; go.textContent = label; moveReady(box); }
    Chronicle.apiFetch(box.getAttribute('data-post'), { method: 'POST', body: fd, headers: { 'HX-Request': 'true' } })
      .then(function (resp) {
        if (!resp.ok) return errorOf(resp).then(function (m) { restore(); showErr(box, m); });
        var note = trigger(resp, 'chronicle:notify') || {};
        var st = mine(box); if (st) st.dirty = false;
        go.innerHTML = '<i class="fa-solid fa-check" aria-hidden="true"></i><span></span>';
        go.lastChild.textContent = label === 'Move' ? 'Moved' : 'Asked';
        var panel = panelOf(box);
        var holders = [box.getAttribute('data-from-kind') + ':' + box.getAttribute('data-from-id'), r.value];
        close(true, function () { reload(panel ? panel.id : '', kind === 'money' ? 'money' : box.getAttribute('data-item-id'), holders); });
        if (note.message) toast(note.message);
      })
      .catch(function () { restore(); showErr(box, 'Network error. Try again.'); });
  }

  // ---- the Armory card's menu ----

  function card(btn, o) {
    var pop = document.getElementById(o.pop);
    if (!pop || busy) return;
    if (S && S.btn === btn) { close(false); return; }
    if (S && !close(false)) return;
    pop.innerHTML = '';
    var menu = document.createElement('div');
    menu.className = 'ag-menu';
    function entry(icon, label, fn) {
      var b = document.createElement('button');
      b.type = 'button';
      b.setAttribute('role', 'menuitem');
      b.innerHTML = '<i class="fa-solid ' + icon + '"></i>';
      b.appendChild(document.createTextNode(label));
      b.addEventListener('click', fn);
      menu.appendChild(b);
      return b;
    }
    if (o.collections) entry('fa-layer-group', 'Add to collection…', function () { collections(pop, o); });
    if (o.give) {
      entry('fa-gift', 'Give to…', function () {
        load(o.give, pop).then(function (box) {
          if (!box || !S || S.host !== pop) return;
          S.box = box;
          pop.style.animation = 'none'; void pop.offsetWidth; pop.style.animation = '';
          // The menu entry that had focus is gone; keep focus in the popover.
          pop.focus({ preventScroll: true });
        });
      });
    }
    pop.appendChild(menu);
    pop.hidden = false;
    btn.setAttribute('aria-expanded', 'true');
    S = { host: pop, box: pop, kind: 'pop', btn: btn, dirty: false };
    listen(true);
    var f = menu.querySelector('button'); if (f) f.focus({ preventScroll: true });
  }

  // collections fills the popover with the collections the item can go in;
  // each box adds or removes it, ticking and counting from the server's
  // answer rather than assuming success.
  function collections(pop, o) {
    var base = '/campaigns/' + encodeURIComponent(o.campaign) + '/armory/';
    pop.innerHTML = '<div class="p-2 max-h-72 overflow-y-auto text-sm text-fg-muted">Loading…</div>';
    var wrap = pop.firstChild;
    function fail(msg) {
      var m = wrap.querySelector('[data-coll-error]'); if (m) m.textContent = msg;
      notify(msg);
    }
    function syncOption(c) {
      var sel = document.querySelectorAll('select[name=instance] option');
      for (var i = 0; i < sel.length; i++) if (sel[i].value === String(c.id)) sel[i].textContent = c.name + ' (' + c.itemCount + ')';
    }
    function row(c) {
      var l = document.createElement('label');
      l.className = 'flex items-center gap-2 px-2 py-1.5 rounded hover:bg-surface-alt cursor-pointer text-sm text-fg';
      var cb = document.createElement('input'); cb.type = 'checkbox'; cb.checked = !!c.hasItem;
      var n = document.createElement('span'); n.className = 'flex-1 truncate'; n.textContent = c.name;
      var k = document.createElement('span'); k.className = 'text-xs text-fg-muted'; k.textContent = String(c.itemCount);
      cb.addEventListener('change', function () {
        var want = cb.checked;
        if (cb.getAttribute('aria-busy') === 'true') return;
        cb.setAttribute('aria-busy', 'true');
        var req = want
          ? Chronicle.apiFetch(base + 'instances/' + c.id + '/items', { method: 'POST', body: { entity_id: o.item } })
          : Chronicle.apiFetch(base + 'instances/' + c.id + '/items/' + encodeURIComponent(o.item), { method: 'DELETE' });
        req.then(function (r) {
          if (r.ok) { c.hasItem = want; c.itemCount = Math.max(0, c.itemCount + (want ? 1 : -1)); k.textContent = String(c.itemCount); syncOption(c); return; }
          cb.checked = !want;
          return errorOf(r).then(fail);
        }).catch(function () { cb.checked = !want; fail('Could not update the collection.'); })
          .then(function () { cb.removeAttribute('aria-busy'); });
      });
      l.appendChild(cb); l.appendChild(n); l.appendChild(k);
      return l;
    }
    Chronicle.apiFetch(base + 'items/' + encodeURIComponent(o.item) + '/collections')
      .then(function (r) { if (!r.ok) throw new Error('load'); return r.json(); })
      .then(function (list) {
        wrap.className = 'p-2 max-h-72 overflow-y-auto';
        wrap.innerHTML = '<div class="px-2 pb-1 text-xs font-semibold text-fg-secondary">Add to collection</div>';
        if (!list.length) {
          var e = document.createElement('p');
          e.className = 'px-2 py-1 text-xs text-fg-muted';
          e.textContent = 'No collections yet. Create one with the gear button.';
          wrap.appendChild(e);
        }
        list.forEach(function (c) { wrap.appendChild(row(c)); });
        var m = document.createElement('p');
        m.setAttribute('data-coll-error', '1'); m.setAttribute('role', 'alert'); m.className = 'px-2 text-xs text-red-500';
        wrap.appendChild(m);
        var f = wrap.querySelector('input'); if (f) f.focus({ preventScroll: true });
      })
      .catch(function () { wrap.textContent = 'Could not load collections.'; });
  }

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.GiveBox = {
    open: open, move: move, card: card, tab: tab, pick: pick, step: step,
    search: search, cancel: cancel, discard: discard, give: give, tick: tick, save: save,
    _internal: { clampQty: clampQty, esc: esc, close: close, toast: toast }
  };
})();
