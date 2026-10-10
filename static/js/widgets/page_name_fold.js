/**
 * page_name_fold.js -- the Edit name fold (Chronicle.PageHeader.name*).
 *
 * The server renders the fold (internal/plugins/entities/name_fold.templ); this
 * file opens and shuts it, searches the campaign's pages for the parent, and
 * saves. It grows out of the page header in place, and a save updates the
 * heading on the page without reloading it.
 *
 * Only what changed is sent (the metadata endpoint is a partial update: an
 * absent key keeps the stored value, null clears it). A claimed player's fold
 * holds the name alone, which is all the server takes from them.
 *
 * A fold with unsaved changes does not close on a click outside or on Escape:
 * it shakes and shows its amber bar, whose Discard throws the changes away.
 * Cancel and the close button always close. Every control calls in here
 * through an inline handler built Go-side, so nothing depends on a script
 * inside a swapped fragment; the outside-click and Escape listeners live only
 * while the fold is open.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var H = (C.PageHeader = C.PageHeader || {});
  var DIRTY_ID = 'page-name-fold';

  var S = null;          // the open fold: {host, box, trigger}
  var saving = false;
  var searchTimer = 0;
  var searchSeq = 0;

  function reduced() {
    return matchMedia('(prefers-reduced-motion: reduce)').matches ||
      !!document.querySelector('.nav-rm, html.rm, html[data-cz-reduce], html[data-view-motion="calm"]');
  }
  function $(root, sel) { return root.querySelector(sel); }
  function boxOf(el) { return el && el.closest('[data-box]'); }

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
    if (reduced()) {
      el.style.height = open ? 'auto' : '0px';
      el.classList.toggle('is-open', open);
      finished = true;
      if (done) done();
      return;
    }
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

  function nag(box) {
    var w = $(box, '.ag-warn');
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
  H._toast = toast;

  // ---- what the form holds against what is saved ----

  function structure(box) { return box.getAttribute('data-structure') === 'true'; }

  function current(box) {
    var name = $(box, '[data-f="name"]');
    var label = $(box, '[data-f="label"]');
    var radio = $(box, 'input[name="page-parent"]:checked');
    return {
      name: name ? name.value.trim() : '',
      label: label ? label.value.trim() : null,
      parent: radio ? radio.value : null
    };
  }

  // changes lists only the keys that differ from what is saved, shaped for
  // the metadata endpoint. An emptied descriptor or parent is sent as null.
  function changes(box) {
    var c = current(box), out = {};
    if (c.name !== box.getAttribute('data-name')) out.name = c.name;
    if (structure(box)) {
      if (c.label !== null && c.label !== box.getAttribute('data-label')) out.type_label = c.label === '' ? null : c.label;
      if (c.parent !== null && c.parent !== box.getAttribute('data-parent-id')) out.parent_id = c.parent === '' ? null : c.parent;
    }
    return out;
  }

  function isDirty(box) { return Object.keys(changes(box)).length > 0; }

  function setSaveState(box, state, label) {
    var b = $(box, '[data-save]');
    if (!b) return;
    b.classList.toggle('is-loading', state === 'loading');
    b.classList.toggle('is-ok', state === 'ok');
    $(b, '[data-save-label]').textContent = label || 'Save';
  }

  function showError(box, msg) {
    var e = $(box, '[data-err]');
    if (!e) return;
    e.textContent = msg || '';
    e.hidden = !msg;
  }

  // sync keeps Save, the dirty registry and the amber bar in step with the form.
  function sync(box) {
    var dirty = isDirty(box);
    var save = $(box, '[data-save]');
    if (save && !saving) save.disabled = !dirty || !current(box).name;
    if (dirty) { if (C.markDirty) C.markDirty(DIRTY_ID); }
    else {
      if (C.markClean) C.markClean(DIRTY_ID);
      var w = $(box, '.ag-warn');
      if (w) w.classList.remove('is-on');
    }
    return dirty;
  }

  // reset puts the form back to what is saved.
  function reset(box) {
    var name = $(box, '[data-f="name"]'), label = $(box, '[data-f="label"]');
    if (name) name.value = box.getAttribute('data-name') || '';
    if (label) label.value = box.getAttribute('data-label') || '';
    var q = $(box, '[data-parent-q]');
    if (q) q.value = '';
    box.setAttribute('data-sel-id', box.getAttribute('data-parent-id') || '');
    box.setAttribute('data-sel-name', box.getAttribute('data-parent-name') || '');
    showError(box, '');
    setSaveState(box, 'idle');
    var w = $(box, '.ag-warn');
    if (w) w.classList.remove('is-on');
    renderParents(box, null);
    sync(box);
  }

  // ---- the parent list ----

  function row(box, id, name, selected) {
    var l = document.createElement('label');
    l.className = 'ag-row';
    var r = document.createElement('input');
    r.type = 'radio';
    r.name = 'page-parent';
    r.value = id;
    r.checked = selected;
    r.setAttribute('data-name', name);
    r.addEventListener('change', function () { onPick(box, r); });
    var s = document.createElement('span');
    s.className = 'ag-name';
    var b = document.createElement('b');
    b.textContent = name;
    s.appendChild(b);
    l.appendChild(r);
    l.appendChild(s);
    return l;
  }

  function onPick(box, radio) {
    box.setAttribute('data-sel-id', radio.value);
    box.setAttribute('data-sel-name', radio.getAttribute('data-name') || '');
    sync(box);
  }

  // renderParents rebuilds the list: "No parent", the chosen page, then the
  // search results. The chosen page stays listed so searching never loses it.
  function renderParents(box, results, failed) {
    var list = $(box, '[data-parent-list]');
    if (!list) return;
    var selId = box.getAttribute('data-sel-id') || '';
    var selName = box.getAttribute('data-sel-name') || '';
    var self = box.getAttribute('data-self-id');
    list.textContent = '';
    list.appendChild(row(box, '', 'No parent', selId === ''));
    if (selId !== '') list.appendChild(row(box, selId, selName || "A page you can't see", true));
    if (results === null) return;
    var shown = 0;
    results.forEach(function (it) {
      if (!it || it.id === self || it.id === selId) return;
      list.appendChild(row(box, it.id, it.name || 'Untitled', false));
      shown++;
    });
    if (failed || (!shown && results.length === 0)) {
      var m = document.createElement('div');
      m.className = 'ag-more';
      m.textContent = failed ? 'Couldn’t search the pages. Try again.' : 'No pages match.';
      list.appendChild(m);
    }
  }

  function loadParents(box, q) {
    var url = box.getAttribute('data-search-endpoint');
    if (!url || !C.apiFetch) return;
    var seq = ++searchSeq;
    C.apiFetch(url + '?q=' + encodeURIComponent(q), { headers: { Accept: 'application/json' } })
      .then(function (res) { return res.ok ? res.json() : Promise.reject(new Error('search')); })
      .then(function (data) {
        if (seq !== searchSeq || !box.isConnected) return;
        renderParents(box, (data && data.results) || []);
      })
      .catch(function () {
        if (seq !== searchSeq || !box.isConnected) return;
        renderParents(box, [], true);
      });
  }

  H.parentSearch = function (el) {
    var box = boxOf(el);
    if (!box) return;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(function () {
      var q = el.value.trim();
      // The search needs two letters; fewer shows the first pages by name.
      loadParents(box, q.length >= 2 ? q : '');
    }, 250);
  };

  // ---- open and close ----

  function outside(e) {
    if (!S) return;
    if (!S.host.isConnected) { forget(); return; }
    if (S.host.contains(e.target) || (S.trigger && S.trigger.contains(e.target))) return;
    close(false);
  }
  function onKey(e) {
    if (e.key !== 'Escape' || !S) return;
    var t = S.trigger;
    if (close(false) && t && t.isConnected) t.focus();
  }
  function listen(on) {
    var m = on ? 'addEventListener' : 'removeEventListener';
    document[m]('mousedown', outside, true);
    document[m]('keydown', onKey, true);
  }
  function forget() {
    S = null;
    listen(false);
    if (C.markClean) C.markClean(DIRTY_ID);
  }

  // close shuts the fold. Unless forced, unsaved changes nag instead and
  // close returns false.
  function close(force, after) {
    if (!S) { if (after) after(); return true; }
    if (!S.host.isConnected) { forget(); if (after) after(); return true; }
    if (!force && !saving && isDirty(S.box)) { nag(S.box); return false; }
    var s = S;
    forget();
    if (s.trigger) s.trigger.setAttribute('aria-expanded', 'false');
    fold(s.host, false, function () {
      s.host.setAttribute('aria-hidden', 'true');
      s.host.setAttribute('inert', '');
      reset(s.box);
      if (after) after();
    });
    return true;
  }

  H.nameFold = function (el) {
    var host = document.getElementById('page-name-fold');
    if (!host) return;
    var box = $(host, '[data-box]');
    if (S && S.host === host) { close(false); return; }
    reset(box);
    S = { host: host, box: box, trigger: el };
    el.setAttribute('aria-expanded', 'true');
    host.removeAttribute('inert');
    host.setAttribute('aria-hidden', 'false');
    listen(true);
    fold(host, true);
    if (structure(box)) loadParents(box, '');
    var n = $(box, '[data-f="name"]');
    if (n && !('ontouchstart' in window)) setTimeout(function () { n.focus({ preventScroll: true }); }, 30);
  };

  H._fold = { changes: changes, isDirty: isDirty };

  H.nameClose = function () { close(true); };
  H.nameDiscard = function () { close(true); };
  H.nameInput = function (el) { var b = boxOf(el); if (b) { showError(b, ''); setSaveState(b, 'idle'); sync(b); } };

  // ---- save ----

  function applyName(box, id, name) {
    var all = document.querySelectorAll('[data-page-h1],[data-page-crumb]');
    for (var i = 0; i < all.length; i++) all[i].textContent = name;
    var stars = document.querySelectorAll('[data-favorite-toggle]');
    for (var j = 0; j < stars.length; j++) {
      if (stars[j].getAttribute('data-favorite-toggle') === id) stars[j].setAttribute('data-entity-name', name);
    }
    var old = box.getAttribute('data-name') || '';
    if (old && document.title.indexOf(old) === 0) document.title = name + document.title.slice(old.length);
    // A system widget that draws the name itself can listen for this.
    document.dispatchEvent(new CustomEvent('chronicle:page-renamed', { bubbles: true, detail: { entityId: id, name: name } }));
  }

  H.nameSave = function (el) {
    var box = boxOf(el);
    if (!box || saving) return;
    var body = changes(box);
    if (!Object.keys(body).length) return;
    if (body.name === '') { showError(box, 'A page needs a name.'); return; }
    saving = true;
    el.disabled = true;
    showError(box, '');
    setSaveState(box, 'loading', 'Saving…');
    C.apiFetch(box.getAttribute('data-endpoint'), { method: 'PUT', body: body })
      .then(function (res) {
        if (res.ok) return res.json().catch(function () { return {}; });
        return res.json().catch(function () { return {}; }).then(function (b) {
          throw new Error((b && (b.message || b.error)) || 'Couldn’t save. Nothing changed.');
        });
      })
      .then(function (data) {
        var c = current(box);
        var savedName = (data && data.name) || c.name;
        applyName(box, box.getAttribute('data-self-id'), savedName);
        box.setAttribute('data-name', savedName);
        if (c.label !== null) box.setAttribute('data-label', c.label);
        if (c.parent !== null) {
          box.setAttribute('data-parent-id', c.parent);
          box.setAttribute('data-parent-name', box.getAttribute('data-sel-name') || '');
        }
        saving = false;
        if (C.markClean) C.markClean(DIRTY_ID);
        setSaveState(box, 'ok', 'Saved');
        toast('Saved.');
        setTimeout(function () { close(true); }, reduced() ? 0 : 700);
      })
      .catch(function (err) {
        saving = false;
        setSaveState(box, 'idle');
        showError(box, err && err.message ? err.message : 'Couldn’t save. Nothing changed.');
        sync(box);
      });
  };
})();
