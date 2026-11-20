/**
 * bulk_actions.js -- Entity Bulk Selection & Actions
 *
 * Adds a tick box to each [data-entity-row] (List row or Gallery tile) and a
 * floating action bar with bulk operations (category, add a tag, move under
 * another page, who sees them, the Trash), each picked from a list and each
 * with Undo. Also remembers the List or Gallery view per browser.
 *
 * Mount: data-widget="bulk-actions"
 * Config:
 *   data-view-key     - localStorage key holding "list" or "gallery"
 *   data-campaign-id  - Campaign UUID
 *   data-csrf-token   - CSRF token
 *   data-role         - User's campaign role (int)
 *   data-can-hide     - "true" for the owner and Co-DMs, who alone hide pages
 *   data-entity-types - JSON array of entity types [{id, name, icon, color}]
 *   data-tags         - JSON array of tags [{id, name, color}]; when empty,
 *                       the Tags list loads GET /campaigns/:id/tags
 */
(function () {
  'use strict';

  var UNDO_KEY = 'chronicle-bulk-undo';

  // After a change reloads the list, offer to put it back. Undo replays the
  // reverse of what the server said it changed, then reloads again.
  function offerUndo(campaignId) {
    var raw = null;
    try { raw = sessionStorage.getItem(UNDO_KEY); sessionStorage.removeItem(UNDO_KEY); } catch (e) { return; }
    if (!raw) return;
    var pending;
    try { pending = JSON.parse(raw); } catch (e) { return; }
    if (!pending || pending.campaignId !== campaignId || typeof pending.message !== 'string') return;
    var steps = Array.isArray(pending.undo) ? pending.undo : [];
    if (!steps.length) { Chronicle.notify(pending.message, 'success'); return; }

    Chronicle.notify(pending.message, 'success', { duration: 10000, action: { label: 'Undo', onClick: function (btn) {
      btn.disabled = true;
      steps.reduce(function (chain, step) {
        return chain.then(function () {
          if (typeof step.path !== 'string' || step.path.charAt(0) !== '/') throw new Error('bad step');
          return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(campaignId) + step.path, { method: 'POST', body: step.body })
            .then(function (res) { if (!res.ok) throw new Error('undo ' + res.status); });
        });
      }, Promise.resolve()).then(function () {
        window.location.reload();
      }).catch(function () {
        btn.disabled = false;
        Chronicle.notify('Some of it couldn\u2019t be put back. Check the pages, or the Trash.', 'error');
      });
    } } });
  }

  // The list opens in the view this browser last chose. The server draws the
  // List; a stored Gallery is applied here, and the tabs follow it. Storage
  // can be refused (private windows), and then the List stays.
  function applyStoredView(el) {
    var key = el.dataset.viewKey;
    if (!key) return;
    var v = null;
    try { v = localStorage.getItem(key); } catch (e) { return; }
    if (v !== 'list' && v !== 'gallery') return;
    el.setAttribute('data-view', v);
    el.querySelectorAll('[data-view-tab]').forEach(function (t) {
      t.setAttribute('aria-selected', t.getAttribute('data-view') === v ? 'true' : 'false');
    });
  }

  Chronicle.register('bulk-actions', {
    init: function (el) {
      var campaignId = el.dataset.campaignId;
      var csrfToken = el.dataset.csrfToken || '';
      var role = parseInt(el.dataset.role, 10) || 0;
      var canHide = el.dataset.canHide === 'true';
      var entityTypes = [];
      var tags = [];

      try { entityTypes = JSON.parse(el.dataset.entityTypes || '[]'); } catch (e) { /* */ }
      try { tags = JSON.parse(el.dataset.tags || '[]'); } catch (e) { /* */ }

      // The view (List or Gallery) is remembered for everyone, players too.
      applyStoredView(el);

      // Only Scribes+ can use bulk actions.
      if (role < 2) return;

      var selected = {};
      var actionBar = null;

      // --- The tick boxes ---

      // Each page is a [data-entity-row] (a row of the List, a tile of the
      // Gallery). Its box is put in as the row's first child, so the server
      // draws no box for a list nobody may change. CSS keeps the box out of
      // sight until the pointer or the keyboard is on its row, or Select is on.
      function nameCheckbox(input, host) {
        var t = host.querySelector('.el-name');
        var name = t ? t.textContent.trim() : '';
        input.setAttribute('aria-label', name ? 'Select ' + name : 'Select page');
      }

      function injectCheckboxes() {
        el.querySelectorAll('[data-entity-row]').forEach(function (row) {
          if (row.querySelector('.el-pick')) return;
          var entityId = row.dataset.entityId;
          if (!entityId) return;

          var box = document.createElement('label');
          box.className = 'el-pick';
          var input = document.createElement('input');
          input.type = 'checkbox';
          input.setAttribute('data-bulk-id', entityId);
          box.appendChild(input);
          nameCheckbox(input, row);
          input.checked = !!selected[entityId];
          input.addEventListener('change', function () { toggleSelect(entityId, this.checked); });
          row.insertBefore(box, row.firstChild);
          if (selected[entityId]) row.setAttribute('data-picked', '');
        });
      }

      // The same page is a row and a tile; the boxes the person can see are
      // the ones in the view that is showing.
      function visibleBoxes() {
        return Array.prototype.slice.call(el.querySelectorAll('[data-bulk-id]')).filter(function (cb) {
          return cb.offsetParent !== null;
        });
      }

      // Keeps both views' boxes for one page in step.
      function setBoxes(entityId, isSelected) {
        el.querySelectorAll('[data-bulk-id]').forEach(function (cb) {
          if (cb.dataset.bulkId === entityId) cb.checked = isSelected;
        });
      }

      // --- Selection State ---

      // Shift-click ticks every box between the last one ticked and this one.
      var lastTicked = null;
      var shiftHeld = false;
      el.addEventListener('click', function (e) {
        if (e.target.matches && e.target.matches('[data-bulk-id]')) shiftHeld = e.shiftKey;
      }, true);

      function toggleSelect(entityId, isSelected) {
        if (shiftHeld && lastTicked && lastTicked !== entityId) {
          shiftHeld = false;
          var boxes = visibleBoxes();
          var ids = boxes.map(function (cb) { return cb.dataset.bulkId; });
          var a = ids.indexOf(lastTicked), b = ids.indexOf(entityId);
          if (a !== -1 && b !== -1) {
            boxes.slice(Math.min(a, b), Math.max(a, b) + 1).forEach(function (cb) {
              setBoxes(cb.dataset.bulkId, isSelected);
              if (isSelected) selected[cb.dataset.bulkId] = true; else delete selected[cb.dataset.bulkId];
            });
            lastTicked = entityId;
            updateActionBar();
            updateCardHighlights();
            return;
          }
        }
        shiftHeld = false;
        lastTicked = entityId;
        setBoxes(entityId, isSelected);
        if (isSelected) {
          selected[entityId] = true;
        } else {
          delete selected[entityId];
        }
        updateActionBar();
        updateCardHighlights();
      }

      function getSelectedIds() {
        return Object.keys(selected);
      }

      function clearSelection() {
        selected = {};
        el.querySelectorAll('[data-bulk-id]').forEach(function (cb) {
          cb.checked = false;
        });
        updateActionBar();
        updateCardHighlights();
      }

      // A picked page's row or tile carries data-picked, which CSS tints.
      function updateCardHighlights() {
        el.querySelectorAll('[data-entity-row]').forEach(function (row) {
          if (selected[row.dataset.entityId]) row.setAttribute('data-picked', '');
          else row.removeAttribute('data-picked');
        });
      }

      // --- Action Bar ---
      //
      // Each change is a quiet word that opens a small searchable list
      // beside it (Type, Tags, Who sees), so nothing asks for a typed id.
      // A change applies at once; the page reloads with an Undo message
      // that puts back exactly what changed.

      // The Select word reads Done while the boxes are on or any page is
      // picked (a pick made with the pointer on desktop included), so it
      // always offers the way out.
      function syncSelectWord(count) {
        var w = el.querySelector('.el-select-word');
        if (!w) return;
        var on = count > 0 || el.hasAttribute('data-selecting');
        w.textContent = on ? 'Done' : 'Select';
        w.setAttribute('aria-pressed', on ? 'true' : 'false');
      }

      function updateActionBar() {
        var count = getSelectedIds().length;
        syncSelectWord(count);
        if (count === 0) {
          closePicker();
          if (actionBar) { actionBar.remove(); actionBar = null; }
          return;
        }

        if (!actionBar) {
          actionBar = document.createElement('div');
          actionBar.className = 'bulk-action-bar fixed left-1/2 -translate-x-1/2 z-50 bg-surface border border-edge rounded-xl shadow-elev-hover px-4 py-2 w-max flex flex-wrap items-center gap-x-4 text-sm max-w-[calc(100vw-2rem)]';
          actionBar.setAttribute('role', 'toolbar');
          actionBar.setAttribute('aria-label', 'Change the selected pages');
          document.body.appendChild(actionBar);
          actionBar.addEventListener('click', onBarClick);
        }

        var isOwner = role >= 3;
        var word = function (act, label) {
          return '<button type="button" class="quiet-word" data-bulk-action="' + act + '" aria-haspopup="listbox" aria-expanded="false">' + label + ' <i class="fa-solid fa-chevron-down text-[10px] ml-1" aria-hidden="true"></i></button>';
        };
        var onPage = visibleBoxes().length;
        actionBar.innerHTML =
          '<span class="text-fg font-medium tabular-nums" aria-live="polite">' + count + ' selected</span>' +
          (onPage > count ? '<button type="button" class="quiet-word" data-bulk-action="all">Select all ' + onPage + '</button>' : '') +
          word('type', 'Type') +
          word('add-tags', 'Tags') +
          word('move', 'Move') +
          (canHide ? word('visibility', 'Who sees') : '') +
          (isOwner ? '<button type="button" class="quiet-word" data-bulk-action="delete">Trash</button>' : '') +
          '<button type="button" class="btn-ghost btn-icon text-fg-muted" data-bulk-action="clear" aria-label="Clear selection" title="Clear selection">' +
          '<i class="fa-solid fa-xmark" aria-hidden="true"></i></button>';
      }

      function onBarClick(e) {
        var btn = e.target.closest('[data-bulk-action]');
        if (!btn) return;
        var act = btn.getAttribute('data-bulk-action');
        if (act === 'clear') { clearSelection(); return; }
        if (act === 'all') {
          visibleBoxes().forEach(function (cb) { setBoxes(cb.dataset.bulkId, true); selected[cb.dataset.bulkId] = true; });
          updateActionBar();
          updateCardHighlights();
          return;
        }
        if (act === 'delete') { trashSelected(); return; }
        if (picker && picker.anchor === btn) { closePicker(); return; }
        if (act === 'type') openPicker(btn, typeOptions, applyType, 'Find a category');
        if (act === 'add-tags') loadTags().then(function () { openPicker(btn, tagOptions, applyTag, 'Find or make a tag', true); });
        if (act === 'visibility') openPicker(btn, visibilityOptions, applyVisibility);
        if (act === 'move') { moveResults = []; openPicker(btn, moveOptions, applyMove, 'Find a page to move them under', false, searchMove); }
      }

      // --- The picker ---

      var picker = null;

      function typeOptions() {
        return entityTypes.map(function (t) { return { id: t.id, label: t.name }; });
      }
      function tagOptions() {
        return tags.map(function (t) { return { id: t.id, label: t.name }; });
      }
      function visibilityOptions() {
        return [
          { id: 'public', label: 'Players can see them' },
          { id: 'private', label: 'Hidden from players' }
        ];
      }

      // Move lists pages from the campaign search as you type, so it never
      // loads every page; the top level is always offered.
      var moveResults = [];
      var moveTimer = null;
      var moveSeq = 0;
      function moveOptions() {
        var out = [{ id: '', label: 'Top level, under no page', keep: true }];
        moveResults.forEach(function (r) {
          if (!selected[r.id]) out.push({ id: r.id, name: r.name, label: r.type_name ? r.name + ' · ' + r.type_name : r.name });
        });
        return out;
      }
      function searchMove(q, rerender) {
        clearTimeout(moveTimer);
        var seq = ++moveSeq;
        if (q.length < 2) { moveResults = []; rerender(); return; }
        moveTimer = setTimeout(function () {
          Chronicle.apiFetch('/campaigns/' + encodeURIComponent(campaignId) + '/entities/search?q=' + encodeURIComponent(q))
            .then(function (res) { if (!res.ok) throw new Error('search ' + res.status); return res.json(); })
            .then(function (data) {
              if (seq !== moveSeq) return;
              moveResults = Array.isArray(data.results) ? data.results : [];
              rerender();
            })
            .catch(function () { if (seq === moveSeq) { moveResults = []; rerender(); } });
        }, 200);
      }

      var tagsLoaded = false;
      function loadTags() {
        if (tagsLoaded) return Promise.resolve();
        return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(campaignId) + '/tags')
          .then(function (res) { if (!res.ok) throw new Error('tags ' + res.status); return res.json(); })
          .then(function (list) { tags = Array.isArray(list) ? list : []; tagsLoaded = true; })
          .catch(function () { Chronicle.notify('Could not load the tags. Try again.', 'error'); throw new Error('no tags'); });
      }

      function openPicker(anchor, optionsFn, onPick, findLabel, canMake, onQuery) {
        closePicker();
        var box = document.createElement('div');
        box.className = 'bulk-picker fixed z-50 bg-surface border border-edge rounded-lg shadow-elev-hover p-1 w-64 max-h-72 flex flex-col';
        var list = document.createElement('div');
        list.className = 'overflow-y-auto';
        list.setAttribute('role', 'listbox');
        var find = null;
        if (findLabel) {
          find = document.createElement('input');
          find.type = 'text';
          find.className = 'w-full px-2 py-1.5 mb-1 text-sm bg-transparent border-0 border-b border-edge text-fg focus:outline-none focus:border-accent';
          find.setAttribute('aria-label', findLabel);
          find.placeholder = findLabel;
          box.appendChild(find);
        }
        box.appendChild(list);
        document.body.appendChild(box);

        var rect = anchor.getBoundingClientRect();
        box.style.left = Math.max(8, Math.min(rect.left, window.innerWidth - 264)) + 'px';
        box.style.bottom = (window.innerHeight - rect.top + 8) + 'px';
        anchor.setAttribute('aria-expanded', 'true');

        var active = 0;
        var shown = [];
        function render() {
          var q = find ? find.value.trim() : '';
          var ql = q.toLowerCase();
          // A list that searches as you type is already matched by the server.
          shown = optionsFn().filter(function (o) { return o.keep || onQuery || !ql || o.label.toLowerCase().indexOf(ql) !== -1; });
          var exact = shown.some(function (o) { return o.label.toLowerCase() === ql; });
          if (canMake && q && !exact) shown.push({ make: q, label: 'Make the tag “' + q + '”' });
          if (active >= shown.length) active = Math.max(0, shown.length - 1);
          list.innerHTML = shown.length ? shown.map(function (o, i) {
            return '<button type="button" role="option" data-i="' + i + '" aria-selected="' + (i === active) + '" class="block w-full text-left px-2 py-1.5 rounded text-sm ' +
              (i === active ? 'bg-accent/10 text-fg' : 'text-fg-body hover:bg-surface-alt') + (o.make ? ' italic' : '') + '">' + Chronicle.escapeHtml(o.label) + '</button>';
          }).join('') : '<p class="px-2 py-1.5 text-sm text-fg-muted">Nothing matches.</p>';
        }
        function pick(i) {
          var o = shown[i];
          if (!o) return;
          closePicker();
          if (o.make) {
            makeTag(o.make).then(function (t) { if (t) onPick(t.id, t.name); });
          } else {
            onPick(o.id, o.label, o);
          }
        }
        list.addEventListener('click', function (e) {
          var b = e.target.closest('[data-i]');
          if (b) pick(parseInt(b.getAttribute('data-i'), 10));
        });
        box.addEventListener('keydown', function (e) {
          if (e.key === 'ArrowDown') { e.preventDefault(); active = Math.min(active + 1, shown.length - 1); render(); }
          else if (e.key === 'ArrowUp') { e.preventDefault(); active = Math.max(active - 1, 0); render(); }
          else if (e.key === 'Enter') { e.preventDefault(); pick(active); }
          else if (e.key === 'Escape') { e.preventDefault(); closePicker(); anchor.focus(); }
        });
        if (find) find.addEventListener('input', function () {
          active = 0;
          if (onQuery) onQuery(find.value.trim(), render);
          render();
        });
        render();
        picker = { box: box, anchor: anchor };
        (find || list.querySelector('[data-i]') || box).focus();
        setTimeout(function () { document.addEventListener('mousedown', outside, true); }, 0);
      }

      function outside(e) {
        if (picker && !picker.box.contains(e.target) && !picker.anchor.contains(e.target)) closePicker();
      }

      function closePicker() {
        if (!picker) return;
        picker.anchor.setAttribute('aria-expanded', 'false');
        picker.box.remove();
        picker = null;
        document.removeEventListener('mousedown', outside, true);
      }

      function makeTag(name) {
        return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(campaignId) + '/tags', {
          method: 'POST',
          body: { name: name, color: '#6b7280' }
        }).then(function (res) {
          if (!res.ok) throw new Error('tag ' + res.status);
          return res.json();
        }).then(function (t) {
          tags.push(t);
          return t;
        }).catch(function () {
          Chronicle.notify('Could not make that tag.', 'error');
          return null;
        });
      }

      // --- Changes, each with its Undo ---

      function post(path, body) {
        return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(campaignId) + path, { method: 'POST', body: body })
          .then(function (res) { if (!res.ok) throw new Error(path + ' ' + res.status); return res.json(); });
      }

      function pages(n) { return n === 1 ? '1 page' : n + ' pages'; }

      function applyType(typeId, label) {
        var ids = getSelectedIds();
        post('/entities/bulk-type', { entity_ids: ids, entity_type_id: typeId }).then(function (data) {
          var byType = {};
          Object.keys(data.previous || {}).forEach(function (eid) {
            var t = data.previous[eid];
            (byType[t] = byType[t] || []).push(eid);
          });
          var undo = Object.keys(byType).map(function (t) {
            return { path: '/entities/bulk-type', body: { entity_ids: byType[t], entity_type_id: parseInt(t, 10) } };
          });
          done('Moved ' + pages(data.updated || 0) + ' to ' + label + '.', undo);
        }).catch(function () { Chronicle.notify('The category didn’t change. Try again.', 'error'); });
      }

      function applyTag(tagId, label) {
        var ids = getSelectedIds();
        post('/entities/bulk-tags', { entity_ids: ids, tag_ids: [tagId], action: 'add' }).then(function (data) {
          var gained = Object.keys(data.added || {});
          var undo = gained.length ? [{ path: '/entities/bulk-tags', body: { entity_ids: gained, tag_ids: [tagId], action: 'remove' } }] : [];
          done('Tagged ' + pages(gained.length) + ' ' + label + '.', undo);
        }).catch(function () { Chronicle.notify('The tag wasn’t added. Try again.', 'error'); });
      }

      function applyMove(parentId, label, opt) {
        var ids = getSelectedIds();
        post('/entities/bulk-move', { entity_ids: ids, parent_id: parentId || null }).then(function (data) {
          var byPlace = {};
          Object.keys(data.previous || {}).forEach(function (eid) {
            var p = data.previous[eid] || {};
            var key = JSON.stringify([p.parentId || null, p.parentNodeId || null]);
            (byPlace[key] = byPlace[key] || []).push(eid);
          });
          var undo = Object.keys(byPlace).map(function (key) {
            var place = JSON.parse(key);
            return { path: '/entities/bulk-move', body: { entity_ids: byPlace[key], parent_id: place[0], parent_node_id: place[1] } };
          });
          var n = data.moved || 0;
          done(parentId ? 'Moved ' + pages(n) + ' under ' + ((opt && opt.name) || label) + '.' : 'Moved ' + pages(n) + ' to the top level.', undo);
        }).catch(function () { Chronicle.notify('The pages didn’t move. Try again.', 'error'); });
      }

      function applyVisibility(choice) {
        var ids = getSelectedIds();
        var makePrivate = choice === 'private';
        post('/entities/bulk-visibility', { entity_ids: ids, private: makePrivate }).then(function (data) {
          var changed = data.toggledIds || [];
          var undo = changed.length ? [{ path: '/entities/bulk-visibility', body: { entity_ids: changed, private: !makePrivate } }] : [];
          done((makePrivate ? 'Hid ' : 'Showed ') + pages(changed.length) + (makePrivate ? ' from players.' : ' to players.'), undo);
        }).catch(function () { Chronicle.notify('Who sees them didn’t change. Try again.', 'error'); });
      }

      // Trash is undoable, so it doesn't ask first.
      function trashSelected() {
        var ids = getSelectedIds();
        if (ids.length === 0) return;
        post('/entities/bulk-delete', { entity_ids: ids }).then(function (data) {
          var undo = (data.deletedIds || []).map(function (eid) {
            return { path: '/trash/' + encodeURIComponent(eid) + '/restore' };
          });
          done('Moved ' + pages(undo.length) + ' to the Trash.', undo);
        }).catch(function () { Chronicle.notify('The pages weren’t moved to the Trash. Try again.', 'error'); });
      }

      // The list reloads to show the change; the Undo message waits for it.
      function done(message, undo) {
        try {
          sessionStorage.setItem(UNDO_KEY, JSON.stringify({ campaignId: campaignId, message: message, undo: undo }));
        } catch (e) { /* no Undo without storage; the change stands */ }
        clearSelection();
        window.location.reload();
      }

      offerUndo(campaignId);

      // Done on the Select word puts the picks down.
      el.addEventListener('bulk:clear', clearSelection);

      // --- Init ---

      injectCheckboxes();

      // Re-inject after HTMX swaps (pagination, filtering).
      el._bulkAfterSettle = function (e) {
        if (el.contains(e.detail.target)) {
          injectCheckboxes();
        }
      };
      document.addEventListener('htmx:afterSettle', el._bulkAfterSettle);
    },

    destroy: function (el) {
      var bar = document.querySelector('.bulk-action-bar');
      if (bar) bar.remove();
      document.querySelectorAll('.bulk-picker').forEach(function (p) { p.remove(); });
      if (el && el._bulkAfterSettle) {
        document.removeEventListener('htmx:afterSettle', el._bulkAfterSettle);
        el._bulkAfterSettle = null;
      }
    }
  });
})();
