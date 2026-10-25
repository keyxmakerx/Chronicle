/**
 * sidebar_row_menu.js -- the campaign sidebar's row menu
 *
 * One menu of a row's actions, opened four ways: right-click, a long press
 * on a touch screen, the row's dots button (apps and links; category rows
 * have none), or Shift+F10 / the Menu key on a focused row. What it offers depends on who is looking:
 *
 * - The owner, on an app, category or link: pin it for everyone (or unpin),
 *   hide it from players (or show it), move it up or down, and Edit sidebar.
 *   Each saves at once through sidebar_editor.js's quickChange, the same
 *   PUT /sidebar-config the editor's Save sends.
 * - A player or scribe: pin it for themselves (or unpin), the row's own pin.
 * - Any row that is a link: open it in a new tab. A category, whose click
 *   only peeks into the drill panel, also offers its own page. A row with
 *   nothing else offers the owner Edit sidebar.
 *
 * Rows in a category's page panel (sidebar_tree.js) get the same menu:
 *
 * - A page, for anyone: open it, open it in a new tab, copy its link.
 * - A page, for a scribe or the owner: a new page inside it, a new folder
 *   beside it, rename it, move it to a folder.
 * - A folder, for a scribe or the owner: a new folder inside it, rename it,
 *   move it; the owner can also delete it, after a second step in the menu.
 *   Its pages go back to the top level, as the route says.
 *
 * The editor's rows have their own buttons, so the menu stays shut while
 * the owner edits.
 */
(function () {
  'use strict';

  var LONG_PRESS_MS = 550;
  var EASE_OUT = 'cubic-bezier(.16,1,.3,1)';

  // --- What the menu offers (pure; exported for tests) ---------------------

  /**
   * The menu's entries for one row. info: key (the row's nav key), href
   * (where the row goes, "" for a category), page (a category's own page,
   * else ""), model (the owner's arrangement,
   * or null for anyone else), pin (null when the viewer has no pin of their
   * own on the row, else whether it is pinned), editor (Chronicle.navEditor).
   * Returns a list of {id, label, icon, disabled} and {sep: true}.
   */
  function itemsFor(info) {
    var out = [];
    // Right-click replaces the browser's own menu, so its two link actions
    // come along.
    if (info.page) out.push({ id: 'open-page', label: 'Open category page', icon: 'fa-arrow-right' });
    if (info.href || info.page) {
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

  /**
   * The menu's entries for a row in a category's page panel. info: kind
   * ("page", "place" for a page's extra listing, or "folder"), canEdit
   * (scribe or owner), canDelete (owner).
   */
  function treeItemsFor(info) {
    var out = [];
    if (info.kind !== 'folder') {
      out.push({ id: 'open', label: 'Open', icon: 'fa-arrow-right' });
      out.push({ id: 'open-tab', label: 'Open in new tab', icon: 'fa-arrow-up-right-from-square' });
      out.push({ id: 'copy-link', label: 'Copy link', icon: 'fa-link' });
    }
    // A listing follows its page: it is renamed or moved from the page's own row.
    if (!info.canEdit || info.kind === 'place') return out;
    if (out.length) out.push({ sep: true });
    if (info.kind === 'page') {
      out.push({ id: 'new-sub', label: 'New page inside', icon: 'fa-file-circle-plus' });
      out.push({ id: 'new-folder', label: 'New folder here', icon: 'fa-folder-plus' });
    } else {
      out.push({ id: 'new-folder', label: 'New folder inside', icon: 'fa-folder-plus' });
    }
    out.push({ id: 'rename', label: 'Rename', icon: 'fa-i-cursor' });
    out.push({ id: 'move', label: 'Move to folder…', icon: 'fa-folder-tree' });
    if (info.kind === 'folder' && info.canDelete) {
      out.push({ sep: true });
      out.push({ id: 'delete', label: 'Delete folder', icon: 'fa-trash-can', danger: true });
    }
    return out;
  }

  /**
   * The Move to folder list: the top level, then every folder in tree
   * order, indented by depth. folders: [{id, name, depth}] already without
   * the row itself and anything inside it. current is the row's folder id,
   * or "" at the top level, and is shown but cannot be chosen.
   */
  function moveItemsFor(folders, current) {
    var out = [{ id: 'back', label: 'Back', icon: 'fa-arrow-left' }, { sep: true }];
    out.push({ id: 'move-to', target: '', label: 'Top level', icon: 'fa-layer-group', disabled: current === '' });
    folders.forEach(function (f) {
      out.push({ id: 'move-to', target: f.id, label: f.name, icon: 'fa-folder', indent: f.depth, disabled: current === f.id });
    });
    return out;
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
    var tree = el.closest('#sidebar-entity-tree .sidebar-tree-node');
    if (tree) return tree;
    if (el.closest('#sidebar-drill, #sidebar-nav-edit')) return null;
    var row = el.closest('#sidebar .nav-row[data-nav-key]');
    if (!row) {
      var wrap = el.closest('#sidebar .nav-rw');
      row = wrap && wrap.querySelector(':scope > .nav-row[data-nav-key]');
    }
    return row || null;
  }

  /** A same-site path, or "" for anything that is not one. It is rebuilt
   *  segment by segment behind a fixed "/", each segment encoded, so no
   *  scheme, second host or markup can ride along. */
  function sitePath(p) {
    if (typeof p !== 'string' || p.charAt(0) !== '/') return '';
    var segs = p.split('/'), out = [];
    for (var i = 0; i < segs.length; i++) {
      if (segs[i]) out.push(encodeURIComponent(decodeSeg(segs[i])));
    }
    return out.length ? '/' + out.join('/') : '';
  }

  function decodeSeg(s) {
    try { return decodeURIComponent(s); } catch (e) { return s; }
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
      page: sitePath(row.getAttribute('data-nav-home')),
      model: ed && navEl ? ed.readModel(navEl) : null,
      pin: pinBtn ? pinBtn.getAttribute('aria-pressed') === 'true' : null,
      pinBtn: pinBtn,
      editor: ed,
      name: (row.querySelector('.nav-lb') || row).textContent.trim()
    };
  }

  function treeInfoFor(node) {
    var tree = node.closest('#sidebar-entity-tree');
    var link = node.querySelector(':scope > .sidebar-tree-item');
    var parent = node.getAttribute('data-parent-id') || '';
    var parentNode = parent && tree ? tree.querySelector('.sidebar-tree-node[data-node-id="' + cssEsc(parent) + '"]') : null;
    var kind = node.hasAttribute('data-node-id') ? 'folder' : node.hasAttribute('data-place-key') ? 'place' : 'page';
    return {
      tree: true,
      kind: kind,
      node: node,
      link: link,
      nodeId: node.getAttribute('data-node-id') || '',
      entityId: node.getAttribute('data-entity-id') || '',
      href: link && link.tagName === 'A' ? sitePath(link.getAttribute('href')) : '',
      // The folder the row sits in, "" when it is not in one.
      folder: parentNode ? parent : '',
      campaignId: tree ? tree.getAttribute('data-campaign-id') || '' : '',
      typeId: tree ? parseInt(tree.getAttribute('data-entity-type-id') || '0', 10) || 0 : 0,
      canEdit: !!tree && tree.getAttribute('data-can-edit') === 'true',
      canDelete: !!tree && tree.getAttribute('data-can-delete-folders') === 'true',
      name: link ? (link.getAttribute('data-entity-name') || link.textContent || '').trim() : ''
    };
  }

  function cssEsc(s) {
    return window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/["\\]/g, '\\$&');
  }

  /** The folders a row may move into: every folder but itself and the ones inside it. */
  function folderChoices(info) {
    var tree = info.node.closest('#sidebar-entity-tree');
    if (!tree) return [];
    var out = [];
    tree.querySelectorAll('.sidebar-tree-node[data-node-id]').forEach(function (f) {
      if (info.kind === 'folder' && info.node.contains(f)) return;
      var lb = f.querySelector(':scope > .sidebar-tree-item');
      out.push({
        id: f.getAttribute('data-node-id'),
        name: lb ? (lb.getAttribute('data-entity-name') || lb.textContent || '').trim() : '',
        depth: parseInt(f.getAttribute('data-depth') || '0', 10) || 0
      });
    });
    return out;
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
      if (it.danger) b.setAttribute('data-danger', '');
      if (it.indent) b.style.paddingLeft = (10 + it.indent * 14) + 'px';
      var ic = el('i', it.icon ? 'fa-solid ' + it.icon : '', { 'aria-hidden': 'true' });
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
    var info = row.classList.contains('sidebar-tree-node') ? treeInfoFor(row) : infoFor(row);
    return show(info.tree ? treeItemsFor(info) : itemsFor(info), info, row, x, y, opener);
  }

  /** Draws items as the open menu. A submenu (Move to folder, the delete
   *  check) is drawn the same way, in the same place. */
  function show(items, info, row, x, y, opener) {
    if (!items.length) return false;
    var menu = draw(items, info.name ? info.name + ' actions' : 'Row actions');
    document.body.appendChild(menu);
    place(menu, x, y);
    menu.classList.add('is-open');
    if (!reduced() && menu.animate) {
      menu.animate([{ opacity: 0, transform: 'translateY(-4px) scale(.97)' }, { opacity: 1, transform: 'none' }],
        { duration: 130, easing: EASE_OUT });
    }
    M = { el: menu, items: items, info: info, opener: opener || null, row: row, x: x, y: y };
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
      var back = m.opener && document.contains(m.opener) ? m.opener : (m.info.link || m.row);
      if (back && document.contains(back)) back.focus({ preventScroll: true });
    }
  }

  function run(id, info, it) {
    if (info.tree) { runTree(id, info, it); return; }
    var ed = info.editor;
    var nm = info.name;
    switch (id) {
      case 'open-page':
        window.location.assign(info.page);
        break;
      case 'open-tab':
        window.open(info.href || info.page, '_blank', 'noopener');
        break;
      case 'copy-link':
        if (navigator.clipboard) {
          navigator.clipboard.writeText(new URL(info.href || info.page, window.location.href).href).then(function () {
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
    var sub = submenu(it.id, info);
    if (sub) {
      var m = M;
      close(false);
      show(sub, info, m.row, m.x, m.y, m.opener);
      return;
    }
    // Focus goes back to the row first: the change may redraw the sidebar,
    // and the editor and pins move focus on from there.
    close(true);
    run(it.id, info, it);
  }

  /** The items a choice opens in place of the menu, or null. */
  function submenu(id, info) {
    if (id === 'move') return moveItemsFor(folderChoices(info), info.folder);
    if (id === 'back') return treeItemsFor(info);
    if (id === 'delete') {
      return [
        { id: 'back', label: 'Back', icon: 'fa-arrow-left' },
        { sep: true },
        { id: 'delete-yes', label: 'Delete "' + info.name + '"', icon: 'fa-trash-can', danger: true },
        { id: 'note', label: 'Its pages move to the top level.', icon: '', disabled: true }
      ];
    }
    return null;
  }

  function treeURL(info, tail) {
    return '/campaigns/' + encodeURIComponent(info.campaignId) + tail;
  }

  function treeSave(url, method, body, done, failed) {
    return Chronicle.apiFetch(url, { method: method, body: body }).then(function (res) {
      if (!res.ok) throw new Error(String(res.status));
      return res.status === 204 ? {} : res.json().catch(function () { return {}; });
    }).then(done, function () {
      if (Chronicle.notify) Chronicle.notify(failed, 'error');
    });
  }

  /** Reloads the panel's tree, then runs then() once the new rows are drawn. */
  function reloadTree(then) {
    var api = Chronicle.sidebarTree;
    if (!api) return;
    if (then) {
      document.addEventListener('chronicle:sidebar-tree', function once() {
        document.removeEventListener('chronicle:sidebar-tree', once);
        then();
      });
    }
    api.refresh();
  }

  function runTree(id, info, it) {
    var nm = info.name;
    switch (id) {
      case 'open':
        // The row's own click, so the page opens the way any sidebar click does.
        if (info.link) info.link.click();
        break;
      case 'open-tab':
      case 'copy-link':
        run(id, { href: info.href, page: '' });
        break;
      case 'new-sub':
        window.location.assign(treeURL(info, '/entities/new?type=' + info.typeId + '&parent_id=' + encodeURIComponent(info.entityId)));
        break;
      case 'new-folder': {
        var parent = info.kind === 'folder' ? info.nodeId : info.folder;
        treeSave(treeURL(info, '/sidebar-nodes'), 'POST', { name: 'New folder', entity_type_id: info.typeId }, function (f) {
          var placed = parent
            ? treeSave(treeURL(info, '/sidebar-nodes/' + encodeURIComponent(f.id) + '/reorder'), 'PUT', { parent_id: parent },
              function () {}, 'The folder was made at the top level.')
            : Promise.resolve();
          placed.then(function () {
            reloadTree(function () {
              var row = document.querySelector('#sidebar-entity-tree .sidebar-tree-node[data-node-id="' + cssEsc(f.id) + '"]');
              var lb = row && row.querySelector(':scope > .sidebar-tree-item');
              if (lb && Chronicle.sidebarTree) Chronicle.sidebarTree.rename(lb, f.id, info.campaignId);
            });
          });
        }, 'The folder could not be made.');
        break;
      }
      case 'rename':
        if (info.link && Chronicle.sidebarTree) {
          Chronicle.sidebarTree.rename(info.link, info.nodeId, info.campaignId, info.kind === 'page' ? info.entityId : '');
        }
        break;
      case 'move-to': {
        var to = it.target || null;
        var url = info.kind === 'folder'
          ? treeURL(info, '/sidebar-nodes/' + encodeURIComponent(info.nodeId) + '/reorder')
          : treeURL(info, '/entities/' + encodeURIComponent(info.entityId) + '/reorder');
        // A page goes to the end of its new folder; the route clamps the index.
        var body = info.kind === 'folder' ? { parent_id: to } : { parent_id: null, parent_node_id: to, sort_order: 1000000 };
        treeSave(url, 'PUT', body, function () {
          reloadTree();
          if (Chronicle.notify) Chronicle.notify(nm + ' moved to ' + it.label + '.', 'success');
        }, nm + ' could not be moved.');
        break;
      }
      case 'delete-yes':
        treeSave(treeURL(info, '/sidebar-nodes/' + encodeURIComponent(info.nodeId)), 'DELETE', undefined, function () {
          reloadTree();
          if (Chronicle.notify) Chronicle.notify(nm + ' is deleted. Its pages are at the top level.', 'success');
        }, nm + ' could not be deleted.');
        break;
    }
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
    var more = t.closest('#sidebar [data-nav-more], #sidebar [data-tree-more]');
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
    if (M && e.target && !M.el.contains(e.target) && !(e.target.closest && e.target.closest('[data-nav-more], [data-tree-more]'))) close(false);
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
  window.Chronicle.navRowMenu = { itemsFor: itemsFor, treeItemsFor: treeItemsFor, moveItemsFor: moveItemsFor, sitePath: sitePath };
})();
