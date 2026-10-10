/**
 * sidebar_row_menu.js -- the campaign sidebar's row menu
 *
 * One menu of a row's actions, opened four ways: right-click, a long press
 * on a touch screen, the row's dots button, or Shift+F10 / the Menu key on a
 * focused row. What it offers depends on who is looking:
 *
 * - The owner, on an app, category or link: pin it for everyone (or unpin),
 *   hide it from players (or show it), move it up or down, and Edit sidebar.
 *   Each saves at once through sidebar_editor.js's quickChange, the same
 *   PUT /sidebar-config the editor's Save sends.
 * - A player or scribe: pin it for themselves (or unpin), the row's own pin.
 * - Any row that is a link: open it in a new tab. A row with nothing else
 *   offers the owner Edit sidebar.
 *
 * Rows in a category's page panel keep their own folder menu
 * (sidebar_tree.js); the editor's rows have their own buttons, so the menu
 * stays shut while the owner edits.
 */
(function () {
  'use strict';

  var LONG_PRESS_MS = 550;
  var EASE_OUT = 'cubic-bezier(.16,1,.3,1)';

  // --- What the menu offers (pure; exported for tests) ---------------------

  /**
   * The menu's entries for one row. info: key (the row's nav key), href
   * (where the row goes, "" for a category), model (the owner's arrangement,
   * or null for anyone else), pin (null when the viewer has no pin of their
   * own on the row, else whether it is pinned), editor (Chronicle.navEditor).
   * Returns a list of {id, label, icon, disabled} and {sep: true}.
   */
  function itemsFor(info) {
    var out = [];
    // Right-click replaces the browser's own menu, so its two link actions
    // come along.
    if (info.href) {
      out.push({ id: 'open-tab', label: 'Open in new tab', icon: 'fa-arrow-up-right-from-square' });
      out.push({ id: 'copy-link', label: 'Copy link', icon: 'fa-link' });
    }
    var ed = info.editor;
    var loc = info.model && ed ? ed.findRow(info.model, info.key) : null;
    if (loc) {
      var pinned = loc.sec.id === 'pinned';
      out.push({ id: 'pin-all', label: pinned ? 'Unpin' : 'Pin to top for everyone', icon: 'fa-thumbtack' });
      out.push(loc.row.hidden
        ? { id: 'show', label: 'Show to players', icon: 'fa-eye' }
        : { id: 'hide', label: 'Hide from players', icon: 'fa-eye-slash' });
      out.push({ sep: true });
      out.push({ id: 'up', label: 'Move up', icon: 'fa-arrow-up', disabled: !canStep(ed, info.model, info.key, -1) });
      out.push({ id: 'down', label: 'Move down', icon: 'fa-arrow-down', disabled: !canStep(ed, info.model, info.key, 1) });
    } else if (info.pin !== null && info.pin !== undefined) {
      out.push({ id: 'pin-me', label: info.pin ? 'Unpin' : 'Pin for me', icon: 'fa-thumbtack' });
    }
    if (info.model) {
      if (out.length) out.push({ sep: true });
      out.push({ id: 'edit', label: 'Edit sidebar', icon: 'fa-pencil' });
    }
    return out;
  }

  /** Moves the row one place within its own section. The editor's arrow
   *  keys also cross into the next section, but here that would only repeat
   *  Pin. Returns false when it cannot move. */
  function stepWithin(ed, draft, key, dir) {
    var loc = ed.findRow(draft, key);
    if (!loc) return false;
    var from = loc.sec.id, to = ed.stepRow(draft, key, dir);
    return !!to && to.id === from;
  }

  function canStep(ed, model, key, dir) {
    return stepWithin(ed, JSON.parse(JSON.stringify(model)), key, dir);
  }

  // --- The page --------------------------------------------------------------

  var M = null; // the open menu: { el, items, info, opener, row, x, y }

  function editor() { return window.Chronicle && Chronicle.navEditor; }

  function reduced() {
    var q = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
    return !!(q && q.matches) || document.documentElement.classList.contains('nav-rm');
  }

  /** The sidebar row under el, or null for anything the menu leaves alone. */
  function rowFrom(el) {
    if (!el || !el.closest || document.body.classList.contains('nav-editing')) return null;
    if (el.closest('#sidebar-drill, #sidebar-nav-edit')) return null;
    var row = el.closest('#sidebar .nav-row[data-nav-key]');
    if (!row) {
      var wrap = el.closest('#sidebar .nav-rw');
      row = wrap && wrap.querySelector(':scope > .nav-row[data-nav-key]');
    }
    return row || null;
  }

  function infoFor(row) {
    var wrap = row.parentElement && row.parentElement.classList.contains('nav-rw') ? row.parentElement : null;
    var pinBtn = wrap && wrap.querySelector(':scope > [data-nav-pin]');
    var ed = editor();
    var navEl = document.getElementById('sidebar-nav');
    var href = row.tagName === 'A' ? row.getAttribute('href') || '' : '';
    return {
      key: row.getAttribute('data-nav-key'),
      href: href === '#' ? '' : href,
      model: ed && navEl ? ed.readModel(navEl) : null,
      pin: pinBtn ? pinBtn.getAttribute('aria-pressed') === 'true' : null,
      pinBtn: pinBtn,
      editor: ed,
      name: (row.querySelector('.nav-lb') || row).textContent.trim()
    };
  }

  function el(tag, cls, attrs) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (attrs) Object.keys(attrs).forEach(function (k) { n.setAttribute(k, attrs[k]); });
    return n;
  }

  // Entries are built with DOM calls: a row's name is typed by the owner.
  function draw(items, label) {
    var menu = el('div', 'rowmenu', { role: 'menu', 'aria-label': label, tabindex: '-1' });
    items.forEach(function (it, i) {
      if (it.sep) { menu.appendChild(el('div', 'rowmenu-sep', { role: 'separator' })); return; }
      var b = el('button', 'rowmenu-item', { type: 'button', role: 'menuitem', 'data-idx': String(i), tabindex: '-1' });
      if (it.disabled) { b.disabled = true; b.setAttribute('aria-disabled', 'true'); }
      var ic = el('i', 'fa-solid ' + it.icon, { 'aria-hidden': 'true' });
      var tx = el('span');
      tx.textContent = it.label;
      b.appendChild(ic);
      b.appendChild(tx);
      menu.appendChild(b);
    });
    return menu;
  }

  /** Places the menu at (x, y), kept inside the window. */
  function place(menu, x, y) {
    var r = menu.getBoundingClientRect();
    menu.style.left = Math.max(8, Math.min(x, window.innerWidth - r.width - 8)) + 'px';
    menu.style.top = Math.max(8, Math.min(y, window.innerHeight - r.height - 8)) + 'px';
  }

  function enabled() {
    return M ? Array.prototype.filter.call(M.el.querySelectorAll('.rowmenu-item'), function (b) { return !b.disabled; }) : [];
  }

  function focusAt(i) {
    var list = enabled();
    if (!list.length) return;
    list[(i + list.length) % list.length].focus();
  }

  function open(row, x, y, opener) {
    close(false);
    var info = infoFor(row);
    var items = itemsFor(info);
    if (!items.length) return false;
    var menu = draw(items, info.name ? info.name + ' actions' : 'Row actions');
    document.body.appendChild(menu);
    place(menu, x, y);
    menu.classList.add('is-open');
    if (!reduced() && menu.animate) {
      menu.animate([{ opacity: 0, transform: 'translateY(-4px) scale(.97)' }, { opacity: 1, transform: 'none' }],
        { duration: 130, easing: EASE_OUT });
    }
    M = { el: menu, items: items, info: info, opener: opener || null, row: row };
    if (M.opener) M.opener.setAttribute('aria-expanded', 'true');
    focusAt(0);
    return true;
  }

  function close(returnFocus) {
    if (!M) return;
    var m = M;
    M = null;
    if (m.el.parentNode) m.el.parentNode.removeChild(m.el);
    if (m.opener) m.opener.setAttribute('aria-expanded', 'false');
    if (returnFocus) {
      var back = m.opener && document.contains(m.opener) ? m.opener : m.row;
      if (back && document.contains(back)) back.focus({ preventScroll: true });
    }
  }

  function run(id, info) {
    var ed = info.editor;
    var nm = info.name;
    switch (id) {
      case 'open-tab':
        window.open(info.href, '_blank', 'noopener');
        break;
      case 'copy-link':
        if (navigator.clipboard) {
          navigator.clipboard.writeText(new URL(info.href, window.location.href).href).then(function () {
            if (Chronicle.notify) Chronicle.notify('Link copied.', 'success');
          }, function () {
            if (Chronicle.notify) Chronicle.notify('The link could not be copied.', 'error');
          });
        }
        break;
      case 'pin-all':
        ed.quickChange(info.key, function (d) { return !!ed.togglePin(d, info.key); },
          ed.findRow(info.model, info.key).sec.id === 'pinned' ? nm + ' is unpinned.' : nm + ' is pinned to the top for everyone.');
        break;
      case 'hide':
      case 'show':
        ed.quickChange(info.key, function (d) {
          var loc = ed.findRow(d, info.key);
          if (!loc) return false;
          loc.row.hidden = id === 'hide';
          return true;
        }, id === 'hide' ? nm + ' is hidden from players. It still opens from a link.' : nm + ' shows to players again.');
        break;
      case 'up':
      case 'down':
        ed.quickChange(info.key, function (d) { return stepWithin(ed, d, info.key, id === 'up' ? -1 : 1); },
          nm + ' moved ' + id + '.');
        break;
      case 'pin-me':
        if (info.pinBtn) ed.togglePersonalPin(info.pinBtn);
        break;
      case 'edit':
        document.dispatchEvent(new CustomEvent('chronicle:toggle-sidebar-editor'));
        break;
    }
  }

  function choose(i) {
    if (!M) return;
    var it = M.items[i];
    if (!it || it.sep || it.disabled) return;
    var info = M.info;
    // Focus goes back to the row first: the change may redraw the sidebar,
    // and the editor and pins move focus on from there.
    close(true);
    run(it.id, info);
  }

  // --- Wiring ----------------------------------------------------------------

  document.addEventListener('contextmenu', function (e) {
    var row = rowFrom(e.target);
    if (row && open(row, e.clientX, e.clientY)) e.preventDefault();
  });

  document.addEventListener('click', function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    if (justLongPressed) { justLongPressed = false; e.preventDefault(); e.stopPropagation(); return; }
    var item = M && t.closest('.rowmenu-item');
    if (item && M.el.contains(item)) { e.preventDefault(); choose(parseInt(item.getAttribute('data-idx'), 10)); return; }
    var more = t.closest('#sidebar [data-nav-more]');
    if (more) {
      e.preventDefault();
      e.stopPropagation();
      if (M && M.opener === more) { close(true); return; }
      var row = rowFrom(more);
      var r = more.getBoundingClientRect();
      if (row) open(row, r.left, r.bottom + 4, more);
      return;
    }
    if (M && !M.el.contains(t)) close(false);
  }, true);

  document.addEventListener('pointerdown', function (e) {
    if (M && e.target && !M.el.contains(e.target) && !(e.target.closest && e.target.closest('[data-nav-more]'))) close(false);
  }, true);

  // A long press on a touch screen opens the menu where the finger is; the
  // click that follows the lift is swallowed so the row does not also open.
  var lpTimer = null, lpStart = null, justLongPressed = false;
  document.addEventListener('pointerdown', function (e) {
    justLongPressed = false;
    if (e.pointerType !== 'touch') return;
    var row = rowFrom(e.target);
    if (!row) return;
    lpStart = { x: e.clientX, y: e.clientY };
    clearTimeout(lpTimer);
    lpTimer = setTimeout(function () {
      if (open(row, lpStart.x, lpStart.y)) {
        justLongPressed = true;
        if (navigator.vibrate) { try { navigator.vibrate(12); } catch (err) { /* not allowed */ } }
      }
    }, LONG_PRESS_MS);
  }, { passive: true });
  ['pointerup', 'pointercancel', 'pointermove'].forEach(function (type) {
    document.addEventListener(type, function (e) {
      if (type === 'pointermove' && lpStart && Math.abs(e.clientX - lpStart.x) + Math.abs(e.clientY - lpStart.y) < 10) return;
      clearTimeout(lpTimer);
    }, { passive: true });
  });

  document.addEventListener('keydown', function (e) {
    // A long press whose lift brought no click must not swallow the next
    // click a key makes.
    justLongPressed = false;
    if (M) {
      var list = enabled(), at = list.indexOf(document.activeElement);
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(true); }
      else if (e.key === 'ArrowDown') { e.preventDefault(); focusAt(at + 1); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); focusAt(at < 0 ? -1 : at - 1); }
      else if (e.key === 'Home') { e.preventDefault(); focusAt(0); }
      else if (e.key === 'End') { e.preventDefault(); focusAt(-1); }
      else if (e.key === 'Tab') { close(false); }
      else if ((e.key === 'Enter' || e.key === ' ') && at >= 0) {
        e.preventDefault();
        choose(parseInt(list[at].getAttribute('data-idx'), 10));
      }
      return;
    }
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      var row = rowFrom(document.activeElement);
      if (!row) return;
      var r = row.getBoundingClientRect();
      if (open(row, r.right - 4, r.top)) e.preventDefault();
    }
  }, true);

  // A boosted page swap or a scroll leaves the menu pointing at nothing.
  window.addEventListener('resize', function () { close(false); });
  document.addEventListener('scroll', function (e) {
    if (M && !(e.target && e.target.nodeType === 1 && M.el.contains(e.target))) close(false);
  }, true);
  document.addEventListener('htmx:beforeSwap', function () { close(false); });

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.navRowMenu = { itemsFor: itemsFor };
})();
