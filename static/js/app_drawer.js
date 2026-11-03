/**
 * app_drawer.js -- the app drawer beside the campaign sidebar
 *
 * The grid on the Apps heading and the All apps row open it. It grows out
 * beside the sidebar from the thing clicked (200 ms in, 120 ms out), only
 * fades under Calm, and simply appears under Off. Esc closes it and puts
 * focus back where it came from; so does a click outside it.
 *
 * A tile opens its app. Its menu (right-click, a long press, the dots or
 * Shift+F10) is the sidebar's row menu (sidebar_row_menu.js), which hands
 * the drawer's own actions back to act() here: pin an app to the sidebar or
 * keep it in the drawer only (the owner), pin it for yourself (a player),
 * turn an app on or off (the owner). Each saves at once through the
 * sidebar editor's quickChange or the member's own pins, folds the row in
 * with the Glow, and offers Undo in the toast.
 *
 * The owner can also drag a tile onto the Apps section: a dashed gap opens
 * where it will land, and letting go pins it there. Right-click stays the
 * main way, on touch too; a touch never starts a drag.
 *
 * Typing in the find line filters the tiles. Single-key shortcuts ("/" for
 * the find line) never fire while focus is in a field.
 */
(function () {
  'use strict';

  var GROW_IN = 200, GROW_OUT = 120, FOLD_MS = 200;
  var EASE_OUT = 'cubic-bezier(.16,1,.3,1)', EASE_IN = 'cubic-bezier(.4,0,1,1)';
  var DRAG_SLOP = 6, GAP_REST = 110;

  // --- Small helpers -----------------------------------------------------------

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }
  function drawer() { return document.getElementById('app-drawer'); }
  function isOpen() { var d = drawer(); return !!d && !d.hidden && !d.classList.contains('is-closing'); }
  function editor() { return window.Chronicle && Chronicle.navEditor; }
  function menuOpen() { return !!(window.Chronicle && Chronicle.navRowMenu && Chronicle.navRowMenu.isOpen()); }

  /** "full", "calm" or "off", as html[data-motion] and the device say. */
  function motion() {
    var h = document.documentElement, m = h.getAttribute('data-motion');
    var rm = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (m === 'off' || rm) return 'off';
    if (m === 'calm' || h.classList.contains('nav-rm')) return 'calm';
    return 'full';
  }

  function play(node, frames, opts) {
    if (!node || !node.animate) return null;
    try { return node.animate(frames, opts); } catch (e) { return null; }
  }

  function done(anim, fn) {
    if (!anim) { fn(); return; }
    anim.onfinish = fn;
    anim.oncancel = fn;
  }

  /** A field, a text area or editable content, where keys are typing. */
  function typing(t) {
    if (!t || t.nodeType !== 1) return false;
    var tag = t.tagName;
    return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || t.isContentEditable;
  }

  function cssEsc(s) {
    return window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/["\\]/g, '\\$&');
  }

  function notify(msg, type, undo) {
    if (!window.Chronicle || !Chronicle.notify) return;
    Chronicle.notify(msg, type || 'success', undo ? { duration: 7000, action: { label: 'Undo', onClick: undo } } : undefined);
  }

  // --- Opening and closing -----------------------------------------------------

  var from = null; // what opened the drawer, for focus and aria-expanded

  /** Places the drawer beside the sidebar, its top level with the Apps
   *  heading, growing from the opener's height. On a narrow screen it lies
   *  over the sidebar instead. */
  function place(d, opener) {
    var sb = document.getElementById('sidebar');
    var sr = sb ? sb.getBoundingClientRect() : { right: 0 };
    var head = $('[data-nav-section="apps"] .nav-gh-wrap');
    var hr = head ? head.getBoundingClientRect() : (opener ? opener.getBoundingClientRect() : { top: 64 });
    var or = opener ? opener.getBoundingClientRect() : hr;
    var narrow = window.innerWidth < 640 || sr.right + 300 > window.innerWidth;
    var left = narrow ? 8 : sr.right + 8;
    var top = Math.max(8, Math.min(hr.top - 6, window.innerHeight - 480));
    d.style.left = left + 'px';
    d.style.right = narrow ? '8px' : '';
    d.style.top = top + 'px';
    d.style.maxHeight = Math.max(240, window.innerHeight - top - 16) + 'px';
    d.style.transformOrigin = (narrow ? '24px ' : '0 ') + Math.max(12, or.top + or.height / 2 - top) + 'px';
  }

  function setExpanded(on) {
    $$('[data-apd-open]').forEach(function (b) { b.setAttribute('aria-expanded', String(on)); });
  }

  function open(opener) {
    var d = drawer();
    if (!d) return;
    if (isOpen()) { close(true); return; }
    from = opener || null;
    d.classList.remove('is-closing');
    place(d, opener);
    d.hidden = false;
    setExpanded(true);
    var m = motion();
    if (m === 'full') {
      play(d, [{ opacity: 0, transform: 'scale(.94) translateX(-6px)' }, { opacity: 1, transform: 'none' }],
        { duration: GROW_IN, easing: EASE_OUT });
    } else if (m === 'calm') {
      play(d, [{ opacity: 0 }, { opacity: 1 }], { duration: GROW_IN, easing: 'linear' });
    }
    var find = $('[data-apd-find]', d);
    if (find) find.focus({ preventScroll: true });
  }

  function close(refocus) {
    var d = drawer();
    if (!d || d.hidden) return;
    setExpanded(false);
    endDrag(null);
    var back = refocus && from && document.contains(from) ? from : null;
    from = null;
    var finish = function () {
      d.classList.remove('is-closing');
      d.hidden = true;
      resetFind(d);
    };
    var m = motion();
    if (m === 'off') finish();
    else {
      d.classList.add('is-closing');
      done(play(d, m === 'full'
        ? [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.96) translateX(-4px)' }]
        : [{ opacity: 1 }, { opacity: 0 }], { duration: GROW_OUT, easing: EASE_IN, fill: 'forwards' }), finish);
    }
    if (back) back.focus({ preventScroll: true });
  }

  // --- Finding -------------------------------------------------------------------

  function filter(d, q) {
    q = String(q || '').trim().toLowerCase();
    var any = false;
    $$('[data-apd-group]', d).forEach(function (g) {
      var shown = 0;
      $$('.apd-cell', g).forEach(function (c) {
        var t = c.querySelector('[data-apd-tile]');
        var hit = !q || (t && (t.getAttribute('data-apd-find') || '').indexOf(q) >= 0);
        c.hidden = !hit;
        if (hit) shown++;
      });
      g.hidden = shown === 0;
      if (shown) any = true;
    });
    var empty = $('[data-apd-empty]', d);
    if (empty) empty.hidden = any;
  }

  function resetFind(d) {
    var f = $('[data-apd-find]', d);
    if (f && f.value) { f.value = ''; filter(d, ''); }
  }

  /** The tiles the viewer can reach now, in reading order. */
  function visibleTiles(d) {
    return $$('.apd-cell:not([hidden]) [data-apd-tile]', d).map(function (t) {
      return t.tagName === 'A' ? t : t.querySelector('button, a');
    }).filter(Boolean);
  }

  /** How many tiles sit side by side in the grid. */
  function columns(d) {
    var cells = $$('.apd-cell:not([hidden])', d);
    if (cells.length < 2) return 1;
    var top = cells[0].getBoundingClientRect().top, n = 0;
    for (var i = 0; i < cells.length; i++) {
      if (Math.abs(cells[i].getBoundingClientRect().top - top) > 4) break;
      n++;
    }
    return Math.max(1, n);
  }

  // --- Joining the sidebar: Fold and Glow ---------------------------------------

  /** The sidebar row for an app key, wrapper included. */
  function sidebarRow(key) {
    var r = $('#sidebar .nav-row[data-nav-key="' + cssEsc(key) + '"]');
    if (!r) return null;
    return r.parentElement && r.parentElement.classList.contains('nav-rw') ? r.parentElement : r;
  }

  /** Folds a row that joined the sidebar open, and glows it. Calm opens it
   *  at once and keeps the glow; Off keeps only the glow's end. */
  function joined(key) {
    var w = sidebarRow(key);
    if (!w) return;
    if (w.scrollIntoView) w.scrollIntoView({ block: 'nearest' });
    var m = motion();
    if (m === 'full') {
      var h = w.scrollHeight;
      w.style.overflow = 'hidden';
      done(play(w, [{ height: '0px', opacity: 0 }, { height: h + 'px', opacity: 1 }],
        { duration: FOLD_MS, easing: EASE_OUT }), function () { w.style.overflow = ''; });
    }
    if (m !== 'off') {
      var row = w.classList.contains('nav-row') ? w : w.querySelector('.nav-row');
      if (row) {
        row.classList.remove('apd-glow');
        void row.offsetWidth;
        row.classList.add('apd-glow');
        setTimeout(function () { row.classList.remove('apd-glow'); }, 1300);
      }
    }
  }

  /** Glows a tile in the drawer, after it moved there (an app turned on). */
  function glowTile(key) {
    var t = $('#app-drawer [data-apd-tile][data-nav-key="' + cssEsc(key) + '"]');
    if (!t || motion() === 'off') return;
    if (t.scrollIntoView) t.scrollIntoView({ block: 'nearest' });
    t.classList.add('apd-glow');
    setTimeout(function () { t.classList.remove('apd-glow'); }, 1300);
  }

  // --- Actions (the row menu hands these back) ----------------------------------

  /** Where a row sits now: its section and the key after it, for Undo. */
  function placeOf(key) {
    var ed = editor(), model = ed && ed.readModel(document.getElementById('sidebar-nav'));
    var loc = model && ed.findRow(model, key);
    if (!loc) return null;
    var next = loc.sec.items[loc.index + 1];
    return { sec: loc.sec.id, before: next ? next.key : null };
  }

  /** Pins an app to the sidebar for everyone, in Apps before beforeKey (or
   *  at the end). */
  function pinSide(key, name, beforeKey) {
    var ed = editor();
    if (!ed) return;
    ed.quickChange(key, function (d) { return ed.placeBefore(d, key, 'apps', beforeKey); },
      name + ' is pinned to everyone’s sidebar.').then(function (ok) {
      if (!ok) return;
      joined(key);
      notify(name + ' is pinned to everyone’s sidebar.', 'success', function () {
        ed.quickChange(key, function (d) { return ed.keepInDrawer(d, key); }, name + ' is back in the drawer only.');
      });
    });
  }

  /** Keeps an app in the drawer only: out of every sidebar, nothing lost. */
  function keep(key, name) {
    var ed = editor();
    if (!ed) return;
    var was = placeOf(key);
    ed.quickChange(key, function (d) { return ed.keepInDrawer(d, key); },
      name + ' is in the drawer only.').then(function (ok) {
      if (!ok) return;
      notify(name + ' is in the drawer only. Nothing in it changed.', 'success', was && function () {
        ed.quickChange(key, function (d) { return ed.placeBefore(d, key, was.sec, was.before); },
          name + ' is back in the sidebar.').then(function (back) { if (back) joined(key); });
      });
    });
  }

  /** A player's own pin, from the drawer. */
  function pinMe(key, name, on) {
    var ed = editor();
    if (!ed) return;
    ed.setPersonalPin(key, on, name).then(function (ok) {
      if (!ok) return;
      if (on) joined(key);
      notify(on ? name + ' is pinned to your sidebar.' : name + ' is unpinned.', 'success', function () {
        ed.setPersonalPin(key, !on, name).then(function (back) { if (back && !on) joined(key); });
      });
    });
  }

  /** Turns an app's addons on or off, then redraws the sidebar and tiles. */
  function toggle(info, enable) {
    var ids = String(enable ? info.onIds : info.offIds).split(',').filter(Boolean);
    if (!ids.length || !info.base) return;
    var chain = Promise.resolve();
    ids.forEach(function (id) {
      chain = chain.then(function () {
        return Chronicle.apiFetch(info.base + '/addons/' + encodeURIComponent(id) + '/toggle?action=' + (enable ? 'enable' : 'disable'),
          { method: 'PUT' }).then(function (res) {
          if (!res.ok) throw new Error(String(res.status));
        });
      });
    });
    chain.then(function () {
      var ed = editor();
      return ed && ed.refresh ? ed.refresh() : (window.location.reload(), false);
    }).then(function () {
      if (enable) {
        glowTile(info.key);
        notify(info.name + ' is on. Players get it the next time they open a page.', 'success');
      } else {
        notify(info.name + ' is off. Its pages stay.', 'success', function () {
          var again = $('#app-drawer [data-apd-tile][data-nav-key="' + cssEsc(info.key) + '"]');
          var ids2 = again && again.getAttribute('data-apd-on-ids');
          if (ids2) toggle({ key: info.key, name: info.name, base: info.base, onIds: ids2 }, true);
        });
      }
    }, function () {
      notify(enable ? info.name + ' could not be turned on.' : info.name + ' could not be turned off.', 'error');
    });
  }

  /** Runs one of the drawer's menu choices; false leaves it to the menu. */
  function act(id, info) {
    switch (id) {
      case 'open':
        close(false);
        if (info.node) info.node.click();
        return true;
      case 'open-tab':
        if (info.href) window.open(info.href, '_blank', 'noopener');
        return true;
      case 'pin-side': pinSide(info.key, info.name, null); return true;
      case 'keep-drawer': keep(info.key, info.name); return true;
      case 'pin-me': pinMe(info.key, info.name, !info.personal); return true;
      case 'turn-on': toggle(info, true); return true;
      case 'turn-off-yes': toggle(info, false); return true;
      case 'features':
        if (info.base) window.location.assign(info.base + '/extensions');
        return true;
    }
    return false;
  }

  // --- Dragging a tile onto the Apps section (the owner) ------------------------

  var D = null; // { tile, key, name, x, y, on, ghost, gap, before, wait, dropEnd }

  function appsBody() {
    var b = document.getElementById('nav-b-apps');
    return b && !b.hidden ? b : null;
  }

  /** The Apps rows a gap can open before, in order. */
  function appsRows(body) {
    return Array.prototype.filter.call(body.children, function (n) {
      return n.querySelector && (n.matches('.nav-row[data-nav-key]') || n.querySelector(':scope > .nav-row[data-nav-key]'));
    });
  }

  function rowKey(n) {
    var r = n.matches('.nav-row[data-nav-key]') ? n : n.querySelector(':scope > .nav-row[data-nav-key]');
    return r ? r.getAttribute('data-nav-key') : null;
  }

  function startDrag() {
    D.on = true;
    var r = D.tile.getBoundingClientRect();
    var g = D.tile.cloneNode(true);
    g.removeAttribute('href');
    g.removeAttribute('data-apd-tile');
    g.removeAttribute('data-nav-key');
    g.className = 'apd-tile apd-ghost';
    g.setAttribute('aria-hidden', 'true');
    g.style.width = r.width + 'px';
    g.style.left = r.left + 'px';
    g.style.top = r.top + 'px';
    document.body.appendChild(g);
    D.ghost = g;
    D.tile.classList.add('is-lifted');
    drawer().classList.add('is-dragging');
    var gap = document.createElement('div');
    gap.className = 'apd-gap';
    gap.setAttribute('aria-hidden', 'true');
    gap.textContent = D.name;
    D.gap = gap;
  }

  /** Puts the gap before row (null: the end), folding it open the first time. */
  function putGap(body, before) {
    var end = body.querySelector(':scope > .nav-apd-all');
    var at = before || end;
    if (D.gap.parentNode === body && D.gap.nextSibling === at) return;
    var first = !D.gap.parentNode;
    body.insertBefore(D.gap, at);
    D.before = before ? rowKey(before) : null;
    if (first && motion() === 'full') {
      play(D.gap, [{ height: '0px', opacity: 0 }, { height: D.gap.offsetHeight + 'px', opacity: 1 }], { duration: FOLD_MS, easing: EASE_OUT });
    }
  }

  function moveDrag(x, y) {
    var tilt = motion() === 'full' ? ' rotate(2deg)' : '';
    D.ghost.style.transform = 'translate(' + (x - D.x) + 'px,' + (y - D.y) + 'px)' + tilt;
    var sec = $('[data-nav-section="apps"]');
    var sr = sec ? sec.getBoundingClientRect() : null;
    var over = sr && x >= sr.left && x <= sr.right && y >= sr.top - 8 && y <= sr.bottom + 8;
    var body = appsBody();
    if (sec) sec.classList.toggle('apd-drop', !!over && !body);
    D.dropEnd = !!over && !body;
    if (!over || !body) {
      clearTimeout(D.wait);
      if (D.gap.parentNode) D.gap.parentNode.removeChild(D.gap);
      D.before = null;
      return;
    }
    var rows = appsRows(body), before = null;
    for (var i = 0; i < rows.length; i++) {
      var rr = rows[i].getBoundingClientRect();
      if (y < rr.top + rr.height / 2) { before = rows[i]; break; }
    }
    // The gap moves only once the pointer rests, so the rows it pushes
    // never push it back.
    clearTimeout(D.wait);
    if (!D.gap.parentNode) putGap(body, before);
    else D.wait = setTimeout(function () { if (D && D.gap) putGap(body, before); }, GAP_REST);
  }

  function endDrag(e) {
    if (!D) return;
    var d = D;
    D = null;
    clearTimeout(d.wait);
    if (!d.on) return;
    // The click that follows letting go must not open the app.
    d.tile.setAttribute('data-apd-dragged', '');
    setTimeout(function () { d.tile.removeAttribute('data-apd-dragged'); }, 0);
    var dv = drawer();
    if (dv) dv.classList.remove('is-dragging');
    var sec = $('[data-nav-section="apps"]');
    if (sec) sec.classList.remove('apd-drop');
    var landed = e && (d.gap.parentNode || d.dropEnd);
    if (d.gap.parentNode) d.gap.parentNode.removeChild(d.gap);
    d.tile.classList.remove('is-lifted');
    if (landed) {
      d.ghost.remove();
      pinSide(d.key, d.name, d.before);
      return;
    }
    // Not dropped on the Apps section: the tile glides home.
    var r = d.tile.getBoundingClientRect(), g = d.ghost;
    if (motion() === 'full') {
      done(play(g, [{ transform: g.style.transform }, { transform: 'translate(' + (r.left - parseFloat(g.style.left)) + 'px,' + (r.top - parseFloat(g.style.top)) + 'px)' }],
        { duration: 200, easing: EASE_OUT, fill: 'forwards' }), function () { g.remove(); });
    } else {
      g.remove();
    }
  }

  // --- Wiring ----------------------------------------------------------------------

  document.addEventListener('click', function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    var opener = t.closest('[data-apd-open]');
    if (opener) { e.preventDefault(); open(opener); return; }
    if (t.closest('#app-drawer [data-apd-close]')) { close(true); return; }
    var on = t.closest('#app-drawer [data-apd-turnon]');
    if (on && on.tagName === 'BUTTON') {
      var tile = on.closest('[data-apd-tile]'), lb = tile && tile.querySelector('.apd-tlabel');
      var d = drawer();
      toggle({ key: tile.getAttribute('data-nav-key'), name: lb ? lb.textContent.trim() : '',
        base: d.getAttribute('data-nav-base'), onIds: tile.getAttribute('data-apd-on-ids') || '' }, true);
      return;
    }
    var link = t.closest('#app-drawer a[data-apd-tile]');
    if (link) {
      if (link.hasAttribute('data-apd-dragged')) { e.preventDefault(); e.stopPropagation(); return; }
      // A plain click opens the app here; the drawer steps aside for it.
      if (!e.metaKey && !e.ctrlKey && !e.shiftKey && e.button === 0) close(false);
    }
  });

  // A click outside closes it, but not one on its own menu or on an opener.
  document.addEventListener('pointerdown', function (e) {
    if (!isOpen() || menuOpen()) return;
    var t = e.target;
    if (!t || !t.closest) return;
    if (t.closest('#app-drawer, [data-apd-open], .rowmenu, #chronicle-toasts')) return;
    close(false);
  });

  document.addEventListener('input', function (e) {
    if (e.target && e.target.matches && e.target.matches('#app-drawer [data-apd-find]')) filter(drawer(), e.target.value);
  });

  document.addEventListener('keydown', function (e) {
    if (!isOpen() || menuOpen()) return;
    var d = drawer(), t = e.target;
    if (e.key === 'Escape') {
      var f = $('[data-apd-find]', d);
      if (t === f && f.value) { e.preventDefault(); f.value = ''; filter(d, ''); return; }
      e.preventDefault();
      close(true);
      return;
    }
    if (e.key === '/' && !typing(t) && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault();
      var find = $('[data-apd-find]', d);
      if (find) find.focus();
      return;
    }
    if (!d.contains(t)) return;
    var tiles = visibleTiles(d), i = tiles.indexOf(t);
    if (t.matches('[data-apd-find]') && e.key === 'ArrowDown' && tiles.length) {
      e.preventDefault();
      tiles[0].focus();
      return;
    }
    if (i < 0) return;
    var cols = columns(d), to = -1;
    if (e.key === 'ArrowRight') to = i + 1;
    else if (e.key === 'ArrowLeft') to = i - 1;
    else if (e.key === 'ArrowDown') to = i + cols;
    else if (e.key === 'ArrowUp') to = i - cols;
    else if (e.key === 'Home') to = 0;
    else if (e.key === 'End') to = tiles.length - 1;
    else return;
    e.preventDefault();
    if (to < 0 && e.key === 'ArrowUp') { var fl = $('[data-apd-find]', d); if (fl) fl.focus(); return; }
    tiles[Math.max(0, Math.min(tiles.length - 1, to))].focus();
  });

  // Dragging is for the owner, with a mouse or pen, from a tile kept in the
  // drawer only; a touch opens the menu on a long press instead.
  document.addEventListener('pointerdown', function (e) {
    if (e.button !== 0 || e.pointerType === 'touch') return;
    var tile = e.target && e.target.closest && e.target.closest('#app-drawer a[data-apd-tile]');
    var d = drawer();
    if (!tile || !d || !d.hasAttribute('data-apd-owner') || tile.hasAttribute('data-apd-in')) return;
    var lb = tile.querySelector('.apd-tlabel');
    D = { tile: tile, key: tile.getAttribute('data-nav-key'), name: lb ? lb.textContent.trim() : '', x: e.clientX, y: e.clientY, on: false };
  });
  document.addEventListener('pointermove', function (e) {
    if (!D) return;
    if (!D.on) {
      if (Math.abs(e.clientX - D.x) + Math.abs(e.clientY - D.y) < DRAG_SLOP) return;
      startDrag();
    }
    e.preventDefault();
    moveDrag(e.clientX, e.clientY);
  });
  document.addEventListener('pointerup', function (e) { endDrag(e); });
  document.addEventListener('pointercancel', function () { endDrag(null); });
  // A link would start the browser's own drag of its address.
  document.addEventListener('dragstart', function (e) {
    if (e.target && e.target.closest && e.target.closest('#app-drawer [data-apd-tile]')) e.preventDefault();
  });

  // After the tiles are redrawn, focus finds its tile again.
  var lastKey = null;
  document.addEventListener('focusin', function (e) {
    var t = e.target && e.target.closest && e.target.closest('#app-drawer [data-apd-tile]');
    if (t) lastKey = t.getAttribute('data-nav-key');
  });
  document.addEventListener('chronicle:app-drawer-refreshed', function () {
    var d = drawer();
    if (!d || d.hidden) return;
    var f = $('[data-apd-find]', d);
    if (f && f.value) filter(d, f.value);
    if (d.contains(document.activeElement) || !lastKey) return;
    var t = $('[data-apd-tile][data-nav-key="' + cssEsc(lastKey) + '"]', d);
    var target = t && (t.tagName === 'A' ? t : t.querySelector('button, a'));
    if (target) target.focus({ preventScroll: true });
  });

  window.addEventListener('resize', function () { if (isOpen()) place(drawer(), from); });
  document.addEventListener('htmx:beforeSwap', function (e) {
    if (e.detail && e.detail.target && e.detail.target.id === 'main-content') close(false);
  });

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.appDrawer = { open: open, close: close, act: act, filter: filter, joined: joined };
})();
