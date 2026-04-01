/**
 * bulk_actions.js -- Entity Bulk Selection & Actions
 *
 * Adds multi-select checkboxes to entity cards/rows and a floating
 * action bar with bulk operations (change type, add/remove tags,
 * toggle visibility, delete).
 *
 * Mount: data-widget="bulk-actions"
 * Config:
 *   data-campaign-id  - Campaign UUID
 *   data-csrf-token   - CSRF token
 *   data-role         - User's campaign role (int)
 *   data-entity-types - JSON array of entity types [{id, name, icon, color}]
 *   data-tags         - JSON array of tags [{id, name, color}]
 */
(function () {
  'use strict';

  Chronicle.register('bulk-actions', {
    init: function (el) {
      var campaignId = el.dataset.campaignId;
      var csrfToken = el.dataset.csrfToken || '';
      var role = parseInt(el.dataset.role, 10) || 0;
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

      function toggleSelect(entityId, isSelected) {
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

      function updateActionBar() {
        var count = getSelectedIds().length;
        if (count === 0) {
          if (actionBar) { actionBar.remove(); actionBar = null; }
          return;
        }

        if (!actionBar) {
          actionBar = document.createElement('div');
          actionBar.className = 'fixed bottom-4 left-1/2 -translate-x-1/2 z-50 bg-surface border border-edge rounded-xl shadow-xl px-4 py-3 flex items-center gap-3 text-sm';
          document.body.appendChild(actionBar);
        }

        var isOwner = role >= 3;
        actionBar.innerHTML =
          '<span class="text-fg font-medium">' + count + ' selected</span>' +
          '<div class="w-px h-5 bg-edge"></div>' +
          '<button type="button" class="px-3 py-1.5 rounded-md text-xs font-medium bg-surface-alt text-fg hover:bg-accent/10 hover:text-accent transition-colors" data-bulk-action="type">' +
          '<i class="fa-solid fa-tag mr-1"></i>Change Type</button>' +
          '<button type="button" class="px-3 py-1.5 rounded-md text-xs font-medium bg-surface-alt text-fg hover:bg-accent/10 hover:text-accent transition-colors" data-bulk-action="add-tags">' +
          '<i class="fa-solid fa-tags mr-1"></i>Add Tags</button>' +
          '<button type="button" class="px-3 py-1.5 rounded-md text-xs font-medium bg-surface-alt text-fg hover:bg-accent/10 hover:text-accent transition-colors" data-bulk-action="visibility">' +
          '<i class="fa-solid fa-eye mr-1"></i>Visibility</button>' +
          (isOwner
            ? '<button type="button" class="px-3 py-1.5 rounded-md text-xs font-medium bg-surface-alt text-fg hover:bg-rose-500/10 hover:text-rose-500 transition-colors" data-bulk-action="delete">' +
              '<i class="fa-solid fa-trash mr-1"></i>Delete</button>'
            : '') +
          '<div class="w-px h-5 bg-edge"></div>' +
          '<button type="button" class="text-fg-muted hover:text-fg transition-colors" data-bulk-action="clear" title="Clear selection">' +
          '<i class="fa-solid fa-xmark"></i></button>';

        // Bind action handlers.
        actionBar.querySelector('[data-bulk-action="type"]').addEventListener('click', showTypeMenu);
        actionBar.querySelector('[data-bulk-action="add-tags"]').addEventListener('click', showTagMenu);
        actionBar.querySelector('[data-bulk-action="visibility"]').addEventListener('click', toggleVisibility);
        if (actionBar.querySelector('[data-bulk-action="delete"]')) {
          actionBar.querySelector('[data-bulk-action="delete"]').addEventListener('click', confirmDelete);
        }
        actionBar.querySelector('[data-bulk-action="clear"]').addEventListener('click', clearSelection);
      }

      // --- Bulk Actions ---

      function showTypeMenu() {
        var ids = getSelectedIds();
        if (ids.length === 0) return;

        // Simple prompt-based type selection (could be upgraded to dropdown later).
        var typeNames = entityTypes.map(function (t) { return t.id + ': ' + t.name; }).join('\n');
        var input = prompt('Enter entity type ID to change to:\n\n' + typeNames);
        if (!input) return;
        var typeId = parseInt(input, 10);
        if (isNaN(typeId)) return;

        Chronicle.apiFetch('/campaigns/' + campaignId + '/entities/bulk-type', {
          method: 'POST',
          body: { entity_ids: ids, entity_type_id: typeId }
        }).then(function (res) {
          if (res.ok) {
            Chronicle.notify('Updated ' + ids.length + ' entities', 'success');
            clearSelection();
            window.location.reload();
          } else {
            Chronicle.notify('Failed to update types', 'error');
          }
        });
      }

      function showTagMenu() {
        var ids = getSelectedIds();
        if (ids.length === 0) return;

        var tagNames = tags.map(function (t) { return t.id + ': ' + t.name; }).join('\n');
        var input = prompt('Enter tag IDs to add (comma-separated):\n\n' + tagNames);
        if (!input) return;
        var tagIds = input.split(',').map(function (s) { return parseInt(s.trim(), 10); }).filter(function (n) { return !isNaN(n); });
        if (tagIds.length === 0) return;

        Chronicle.apiFetch('/campaigns/' + campaignId + '/entities/bulk-tags', {
          method: 'POST',
          body: { entity_ids: ids, tag_ids: tagIds, action: 'add' }
        }).then(function (res) {
          if (res.ok) {
            Chronicle.notify('Tags added to ' + ids.length + ' entities', 'success');
            clearSelection();
            window.location.reload();
          } else {
            Chronicle.notify('Failed to add tags', 'error');
          }
        });
      }

      function toggleVisibility() {
        var ids = getSelectedIds();
        if (ids.length === 0) return;

        var makePrivate = confirm('Make ' + ids.length + ' entities private?\n\nClick OK for Private, Cancel for Public.');

        Chronicle.apiFetch('/campaigns/' + campaignId + '/entities/bulk-visibility', {
          method: 'POST',
          body: { entity_ids: ids, private: makePrivate }
        }).then(function (res) {
          if (res.ok) {
            Chronicle.notify((makePrivate ? 'Made private: ' : 'Made public: ') + ids.length + ' entities', 'success');
            clearSelection();
            window.location.reload();
          } else {
            Chronicle.notify('Failed to update visibility', 'error');
          }
        });
      }

      function confirmDelete() {
        var ids = getSelectedIds();
        if (ids.length === 0) return;
        if (!confirm('Delete ' + ids.length + ' entities? This cannot be undone.')) return;

        Chronicle.apiFetch('/campaigns/' + campaignId + '/entities/bulk-delete', {
          method: 'POST',
          body: { entity_ids: ids }
        }).then(function (res) {
          if (res.ok) {
            Chronicle.notify('Deleted ' + ids.length + ' entities', 'success');
            clearSelection();
            window.location.reload();
          } else {
            Chronicle.notify('Failed to delete entities', 'error');
          }
        });
      }

      // --- Init ---

      injectCheckboxes();

      // Re-inject after HTMX swaps (pagination, filtering).
      document.addEventListener('htmx:afterSettle', function (e) {
        if (el.contains(e.detail.target)) {
          injectCheckboxes();
        }
      });
    },

    destroy: function () {
      var bar = document.querySelector('.bulk-action-bar');
      if (bar) bar.remove();
    }
  });
})();
