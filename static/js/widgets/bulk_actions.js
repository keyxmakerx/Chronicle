/**
 * bulk_actions.js -- Entity Bulk Selection & Actions
 *
 * Adds multi-select checkboxes to entity cards/rows and a floating
 * action bar with bulk operations (category, add a tag, who sees them, the
 * Trash), each picked from a list and each with Undo.
 *
 * Mount: data-widget="bulk-actions"
 * Config:
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

    var btnId = 'undo-bulk-' + Date.now();
    Chronicle.notify(Chronicle.escapeHtml(pending.message) +
      ' <button type="button" id="' + btnId + '" class="underline font-medium ml-1">Undo</button>',
      'success', { html: true, duration: 10000 });
    var btn = document.getElementById(btnId);
    if (!btn) return;
    btn.addEventListener('click', function () {
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

      // Only Scribes+ can use bulk actions.
      if (role < 2) return;

      var selected = {};
      var actionBar = null;

      // --- Inject Checkboxes ---

      function injectCheckboxes() {
        // Grid view cards.
        el.querySelectorAll('[data-entity-preview]').forEach(function (card) {
          if (card.querySelector('.bulk-checkbox')) return;
          var entityId = extractEntityId(card);
          if (!entityId) return;

          var cb = document.createElement('div');
          cb.className = 'bulk-checkbox absolute top-2 left-2 z-10 opacity-0 group-hover:opacity-100 transition-opacity';
          cb.innerHTML = '<input type="checkbox" class="w-4 h-4 accent-accent rounded border-edge cursor-pointer" data-bulk-id="' + entityId + '"/>';
          cb.addEventListener('click', function (e) { e.preventDefault(); e.stopPropagation(); });
          cb.querySelector('input').addEventListener('change', function (e) {
            e.stopPropagation();
            toggleSelect(entityId, this.checked);
          });

          // Make card relative for absolute positioning.
          card.style.position = 'relative';
          card.insertBefore(cb, card.firstChild);
        });

        // Table view rows.
        el.querySelectorAll('tr[data-entity-id]').forEach(function (row) {
          if (row.querySelector('.bulk-checkbox')) return;
          var entityId = row.dataset.entityId;
          if (!entityId) return;

          var td = document.createElement('td');
          td.className = 'bulk-checkbox px-2 py-2 w-8';
          td.innerHTML = '<input type="checkbox" class="w-4 h-4 accent-accent rounded border-edge cursor-pointer" data-bulk-id="' + entityId + '"/>';
          td.querySelector('input').addEventListener('change', function () {
            toggleSelect(entityId, this.checked);
          });
          row.insertBefore(td, row.firstChild);
        });

        // Add select-all checkbox to table header if table exists.
        var thead = el.querySelector('thead tr');
        if (thead && !thead.querySelector('.bulk-select-all')) {
          var th = document.createElement('th');
          th.className = 'bulk-select-all px-2 py-2.5 w-8';
          th.innerHTML = '<input type="checkbox" class="w-4 h-4 accent-accent rounded border-edge cursor-pointer" title="Select all"/>';
          th.querySelector('input').addEventListener('change', function () {
            var checked = this.checked;
            el.querySelectorAll('[data-bulk-id]').forEach(function (cb) {
              cb.checked = checked;
              toggleSelect(cb.dataset.bulkId, checked);
            });
          });
          thead.insertBefore(th, thead.firstChild);
        }
      }

      function extractEntityId(card) {
        var href = card.getAttribute('href') || '';
        var match = href.match(/\/entities\/([a-f0-9-]+)/);
        return match ? match[1] : null;
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
          var boxes = Array.prototype.slice.call(el.querySelectorAll('[data-bulk-id]'));
          var ids = boxes.map(function (cb) { return cb.dataset.bulkId; });
          var a = ids.indexOf(lastTicked), b = ids.indexOf(entityId);
          if (a !== -1 && b !== -1) {
            boxes.slice(Math.min(a, b), Math.max(a, b) + 1).forEach(function (cb) {
              cb.checked = isSelected;
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
        var selectAll = el.querySelector('.bulk-select-all input');
        if (selectAll) selectAll.checked = false;
        updateActionBar();
        updateCardHighlights();
      }

      function updateCardHighlights() {
        el.querySelectorAll('[data-entity-preview]').forEach(function (card) {
          var entityId = extractEntityId(card);
          if (entityId && selected[entityId]) {
            card.classList.add('ring-2', 'ring-accent/50');
            var cb = card.querySelector('.bulk-checkbox');
            if (cb) cb.classList.add('opacity-100');
          } else {
            card.classList.remove('ring-2', 'ring-accent/50');
          }
        });
      }

      // --- Action Bar ---
      //
      // Each change is a quiet word that opens a small searchable list
      // beside it (Type, Tags, Who sees), so nothing asks for a typed id.
      // A change applies at once; the page reloads with an Undo message
      // that puts back exactly what changed.

      function updateActionBar() {
        var count = getSelectedIds().length;
        if (count === 0) {
          closePicker();
          if (actionBar) { actionBar.remove(); actionBar = null; }
          return;
        }

        if (!actionBar) {
          actionBar = document.createElement('div');
          actionBar.className = 'bulk-action-bar fixed bottom-4 left-1/2 -translate-x-1/2 z-50 bg-surface border border-edge rounded-xl shadow-xl px-4 py-2 w-max flex flex-wrap items-center gap-4 text-sm max-w-[calc(100vw-2rem)]';
          actionBar.setAttribute('role', 'toolbar');
          actionBar.setAttribute('aria-label', 'Change the selected pages');
          document.body.appendChild(actionBar);
          actionBar.addEventListener('click', onBarClick);
        }

        var isOwner = role >= 3;
        var word = function (act, label) {
          return '<button type="button" class="quiet-word" data-bulk-action="' + act + '" aria-haspopup="listbox" aria-expanded="false">' + label + ' <i class="fa-solid fa-chevron-down text-[10px] ml-1" aria-hidden="true"></i></button>';
        };
        var onPage = el.querySelectorAll('[data-bulk-id]').length;
        actionBar.innerHTML =
          '<span class="text-fg font-medium tabular-nums" aria-live="polite">' + count + ' selected</span>' +
          (onPage > count ? '<button type="button" class="quiet-word" data-bulk-action="all">Select all ' + onPage + '</button>' : '') +
          word('type', 'Type') +
          word('add-tags', 'Tags') +
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
          el.querySelectorAll('[data-bulk-id]').forEach(function (cb) { cb.checked = true; selected[cb.dataset.bulkId] = true; });
          updateActionBar();
          updateCardHighlights();
          return;
        }
        if (act === 'delete') { trashSelected(); return; }
        if (picker && picker.anchor === btn) { closePicker(); return; }
        if (act === 'type') openPicker(btn, typeOptions, applyType, 'Find a category');
        if (act === 'add-tags') loadTags().then(function () { openPicker(btn, tagOptions, applyTag, 'Find or make a tag', true); });
        if (act === 'visibility') openPicker(btn, visibilityOptions, applyVisibility);
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

      var tagsLoaded = false;
      function loadTags() {
        if (tagsLoaded) return Promise.resolve();
        return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(campaignId) + '/tags')
          .then(function (res) { if (!res.ok) throw new Error('tags ' + res.status); return res.json(); })
          .then(function (list) { tags = Array.isArray(list) ? list : []; tagsLoaded = true; })
          .catch(function () { Chronicle.notify('Could not load the tags. Try again.', 'error'); throw new Error('no tags'); });
      }

      function openPicker(anchor, optionsFn, onPick, findLabel, canMake) {
        closePicker();
        var box = document.createElement('div');
        box.className = 'bulk-picker fixed z-50 bg-surface border border-edge rounded-lg shadow-xl p-1 w-64 max-h-72 flex flex-col';
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
          shown = optionsFn().filter(function (o) { return !ql || o.label.toLowerCase().indexOf(ql) !== -1; });
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
            onPick(o.id, o.label);
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
        if (find) find.addEventListener('input', function () { active = 0; render(); });
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
