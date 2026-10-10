/**
 * page_places.js -- the "Also listed under" line (Chronicle.PagePlaces).
 *
 * The server renders the line (internal/plugins/entities/place_line.templ) and
 * answers every add or remove with a fresh copy of it; this file opens the page
 * picker, searches the campaign's pages, and swaps the line for the reply. A
 * listing only changes where the page shows in the tree: the page is the same
 * one, edited once, and removing a listing never deletes it.
 *
 * Every control calls in through an inline handler built Go-side, so the line
 * keeps working after the server swaps it. The outside-click and Escape
 * listeners live only while the picker is open.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var P = (C.PagePlaces = C.PagePlaces || {});

  var open = null;      // the root of the line whose picker is open
  var searchTimer = 0;
  var searchSeq = 0;

  /**
   * The pages worth offering: not the page itself, not its real home, not a
   * place it is already listed in, and nothing without an id. The server
   * refuses the rest (a page below it, another campaign) and says why.
   * Pure, so it is unit tested.
   */
  function pickable(results, selfId, realParentId, listedIds) {
    var skip = {};
    skip[selfId] = true;
    if (realParentId) skip[realParentId] = true;
    (listedIds || []).forEach(function (id) { if (id) skip[id] = true; });
    var out = [];
    var seen = {};
    (results || []).forEach(function (it) {
      if (!it || !it.id || skip[it.id] || seen[it.id]) return;
      seen[it.id] = true;
      out.push({ id: it.id, name: it.name || 'Untitled', type: it.type_name || '' });
    });
    return out;
  }

  function reduced() {
    return matchMedia('(prefers-reduced-motion: reduce)').matches ||
      !!document.querySelector('.nav-rm, html.rm, html[data-cz-reduce], html[data-view-motion="calm"]');
  }

  function rootOf(el) { return el && el.closest('[data-places]'); }
  function $(root, sel) { return root.querySelector(sel); }
  function listed(root) {
    return (root.getAttribute('data-listed') || '').split(',').filter(Boolean);
  }

  function toast(msg, type) {
    if (C.notify) C.notify(msg, type || 'info');
  }

  function showError(root, msg) {
    var e = $(root, '[data-err]');
    if (!e) return;
    e.textContent = msg || '';
    e.hidden = !msg;
  }

  // ---- the picker ----

  function row(name, type, onPick) {
    var b = document.createElement('button');
    b.type = 'button';
    b.className = 'pl-row';
    b.setAttribute('role', 'option');
    var n = document.createElement('span');
    n.className = 'pl-row-name';
    n.textContent = name;
    b.appendChild(n);
    if (type) {
      var t = document.createElement('span');
      t.className = 'pl-row-type';
      t.textContent = type;
      b.appendChild(t);
    }
    b.addEventListener('click', onPick);
    return b;
  }

  function message(list, text) {
    var m = document.createElement('div');
    m.className = 'pl-more';
    m.textContent = text;
    list.appendChild(m);
  }

  function render(root, results, failed) {
    var list = $(root, '[data-list]');
    if (!list) return;
    list.textContent = '';
    if (failed) { message(list, 'Couldn’t search the pages. Try again.'); return; }
    var items = pickable(results, root.getAttribute('data-self-id'),
      root.getAttribute('data-parent-id'), listed(root));
    if (!items.length) { message(list, 'No pages match.'); return; }
    items.forEach(function (it) {
      list.appendChild(row(it.name, it.type, function () { pick(root, it.id); }));
    });
  }

  function load(root, q) {
    var url = root.getAttribute('data-search-endpoint');
    if (!url || !C.apiFetch) return;
    var seq = ++searchSeq;
    C.apiFetch(url + '?q=' + encodeURIComponent(q), { headers: { Accept: 'application/json' } })
      .then(function (res) { return res.ok ? res.json() : Promise.reject(new Error('search')); })
      .then(function (data) {
        if (seq !== searchSeq || !root.isConnected) return;
        render(root, (data && data.results) || [], false);
      })
      .catch(function () {
        if (seq !== searchSeq || !root.isConnected) return;
        render(root, [], true);
      });
  }

  P.search = function (el) {
    var root = rootOf(el);
    if (!root) return;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(function () {
      var q = el.value.trim();
      // The search needs two letters; fewer shows the first pages by name.
      load(root, q.length >= 2 ? q : '');
    }, 250);
  };

  function outside(e) {
    if (!open) return;
    if (!open.isConnected) { forget(); return; }
    if (open.contains(e.target)) return;
    P.close();
  }
  function onKey(e) {
    if (e.key !== 'Escape' || !open) return;
    var add = $(open, '[data-add]');
    P.close();
    if (add) add.focus();
  }
  function listen(on) {
    var m = on ? 'addEventListener' : 'removeEventListener';
    document[m]('mousedown', outside, true);
    document[m]('keydown', onKey, true);
  }
  function forget() { open = null; listen(false); }

  P.close = function () {
    if (!open) return;
    var root = open;
    forget();
    var pop = $(root, '[data-pop]');
    var add = $(root, '[data-add]');
    if (add) add.setAttribute('aria-expanded', 'false');
    if (pop) pop.hidden = true;
  };

  P.toggle = function (el) {
    var root = rootOf(el);
    if (!root) return;
    if (open === root) { P.close(); return; }
    P.close();
    var pop = $(root, '[data-pop]');
    if (!pop) return;
    open = root;
    showError(root, '');
    pop.hidden = false;
    el.setAttribute('aria-expanded', 'true');
    listen(true);
    var q = $(root, '[data-q]');
    if (q) {
      q.value = '';
      if (!('ontouchstart' in window)) setTimeout(function () { q.focus({ preventScroll: true }); }, 30);
    }
    load(root, '');
  };

  // ---- add and remove ----

  // swap replaces the line with the server's fresh copy of it.
  function swap(root, html) {
    var tpl = document.createElement('template');
    tpl.innerHTML = html.trim();
    var fresh = tpl.content.firstElementChild;
    if (!fresh) { root.remove(); return null; }
    var focusAdd = root.contains(document.activeElement);
    root.replaceWith(fresh);
    if (focusAdd) {
      var add = fresh.querySelector('[data-add]');
      if (add) add.focus({ preventScroll: true });
    }
    return fresh;
  }

  function failure(res) {
    return res.json().catch(function () { return {}; }).then(function (b) {
      throw new Error((b && (b.message || b.error)) || 'Couldn’t save. Nothing changed.');
    });
  }

  function pick(root, parentId) {
    if (root.getAttribute('data-busy')) return;
    root.setAttribute('data-busy', '1');
    showError(root, '');
    C.apiFetch(root.getAttribute('data-endpoint'), {
      method: 'POST',
      body: { parent_id: parentId }
    })
      .then(function (res) { return res.ok ? res.text() : failure(res); })
      .then(function (html) {
        P.close();
        swap(root, html);
        toast('Listed there as well. It is the same page.', 'success');
      })
      .catch(function (err) {
        root.removeAttribute('data-busy');
        showError(root, err && err.message ? err.message : 'Couldn’t save. Nothing changed.');
      });
  }

  P.remove = function (el) {
    var root = rootOf(el);
    var pid = el.getAttribute('data-parent-id');
    if (!root || !pid || root.getAttribute('data-busy')) return;
    root.setAttribute('data-busy', '1');
    var chip = el.closest('.pl-chip');
    if (chip) chip.classList.add('is-leaving');
    C.apiFetch(root.getAttribute('data-endpoint') + '/' + encodeURIComponent(pid), {
      method: 'DELETE'
    })
      .then(function (res) { return res.ok ? res.text() : failure(res); })
      .then(function (html) {
        // Let the chip finish its short fade before the line is replaced.
        setTimeout(function () {
          swap(root, html);
          toast('Removed. The page itself is still there.', 'success');
        }, reduced() ? 0 : 140);
      })
      .catch(function (err) {
        root.removeAttribute('data-busy');
        if (chip) chip.classList.remove('is-leaving');
        toast(err && err.message ? err.message : 'Couldn’t remove that place.', 'error');
      });
  };

  // Test-only hook: `module` is undefined when loaded via <script>.
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { pickable: pickable };
  }
})();
