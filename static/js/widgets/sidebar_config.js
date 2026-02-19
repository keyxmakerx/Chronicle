/**
 * sidebar_config.js -- Drag-to-Reorder Sidebar Configuration Widget
 *
 * Mounts on a data-widget="sidebar-config" element. Renders a sortable list
 * of entity types that the campaign owner can reorder via drag-and-drop and
 * toggle visibility. Changes are persisted to the server via PUT.
 *
 * Config attributes:
 *   data-endpoint="/campaigns/:id/sidebar-config"  -- API endpoint
 *   data-csrf=""  -- CSRF token (auto-read from cookie if empty)
 *
 * The widget fetches the current config on init, renders the list, and
 * saves on every change (reorder or toggle).
 */
(function () {
  'use strict';

  Chronicle.register('sidebar-config', {
    init: function (el, config) {
      var endpoint = config.endpoint;
      if (!endpoint) {
        console.error('[sidebar-config] Missing data-endpoint');
        return;
      }

      // State.
      var entityTypes = [];
      var sidebarConfig = { entity_type_order: [], hidden_type_ids: [] };
      var dragSrcEl = null;

      // Parse entity types from data attribute (JSON array).
      try {
        entityTypes = JSON.parse(el.getAttribute('data-entity-types') || '[]');
      } catch (e) {
        console.error('[sidebar-config] Invalid entity types JSON');
        return;
      }

      // Fetch current config from server.
      fetch(endpoint, {
        headers: { 'Accept': 'application/json' },
        credentials: 'same-origin'
      })
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          return res.json();
        })
        .then(function (data) {
          sidebarConfig = data || { entity_type_order: [], hidden_type_ids: [] };
          if (!sidebarConfig.entity_type_order) sidebarConfig.entity_type_order = [];
          if (!sidebarConfig.hidden_type_ids) sidebarConfig.hidden_type_ids = [];
          render();
        })
        .catch(function () {
          // Fall back to defaults.
          render();
        });

      /**
       * Get ordered entity types (respecting sidebar config order).
       */
      function getOrderedTypes() {
        var order = sidebarConfig.entity_type_order;
        if (!order || order.length === 0) {
          return entityTypes.slice();
        }

        var typeMap = {};
        entityTypes.forEach(function (t) { typeMap[t.id] = t; });

        var result = [];
        var seen = {};
        order.forEach(function (id) {
          if (typeMap[id]) {
            result.push(typeMap[id]);
            seen[id] = true;
          }
        });

        // Append any types not in the order.
        entityTypes.forEach(function (t) {
          if (!seen[t.id]) {
            result.push(t);
          }
        });

        return result;
      }

      /**
       * Check if a type ID is hidden.
       */
      function isHidden(typeID) {
        return (sidebarConfig.hidden_type_ids || []).indexOf(typeID) !== -1;
      }

      /**
       * Render the sortable list.
       */
      function render() {
        var types = getOrderedTypes();
        var html = '<div class="sidebar-config-list space-y-1">';

        types.forEach(function (t) {
          var hidden = isHidden(t.id);
          html += '<div class="sidebar-config-item flex items-center px-3 py-2 rounded-md border border-gray-200 bg-white cursor-grab select-none' +
            (hidden ? ' opacity-50' : '') +
            '" draggable="true" data-type-id="' + t.id + '">' +
            '<span class="drag-handle mr-2 text-gray-400"><i class="fa-solid fa-grip-vertical text-xs"></i></span>' +
            '<span class="w-4 h-4 mr-2 flex items-center justify-center">' +
            '<i class="fa-solid ' + escapeAttr(t.icon || 'fa-file') + ' text-xs" style="color: ' + escapeAttr(t.color || '#6b7280') + '"></i>' +
            '</span>' +
            '<span class="flex-1 text-sm text-gray-700">' + escapeHtml(t.name_plural || t.name) + '</span>' +
            '<button type="button" class="toggle-visibility ml-2 p-1 text-xs rounded hover:bg-gray-100 transition-colors" data-type-id="' + t.id + '" title="' +
            (hidden ? 'Show in sidebar' : 'Hide from sidebar') + '">' +
            '<i class="fa-solid ' + (hidden ? 'fa-eye-slash text-gray-400' : 'fa-eye text-gray-600') + '"></i>' +
            '</button>' +
            '</div>';
        });

        html += '</div>';
        html += '<p class="text-xs text-gray-500 mt-2">Drag to reorder. Click the eye icon to show/hide types in the sidebar.</p>';

        el.innerHTML = html;

        // Bind drag events.
        var items = el.querySelectorAll('.sidebar-config-item');
        items.forEach(function (item) {
          item.addEventListener('dragstart', handleDragStart);
          item.addEventListener('dragover', handleDragOver);
          item.addEventListener('dragenter', handleDragEnter);
          item.addEventListener('dragleave', handleDragLeave);
          item.addEventListener('drop', handleDrop);
          item.addEventListener('dragend', handleDragEnd);
        });

        // Bind visibility toggles.
        var toggles = el.querySelectorAll('.toggle-visibility');
        toggles.forEach(function (btn) {
          btn.addEventListener('click', function (e) {
            e.preventDefault();
            var typeID = parseInt(btn.getAttribute('data-type-id'), 10);
            toggleVisibility(typeID);
          });
        });
      }

      function handleDragStart(e) {
        dragSrcEl = this;
        this.classList.add('opacity-50', 'border-accent');
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', this.getAttribute('data-type-id'));
      }

      function handleDragOver(e) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        return false;
      }

      function handleDragEnter(e) {
        e.preventDefault();
        this.classList.add('border-blue-400', 'bg-blue-50');
      }

      function handleDragLeave() {
        this.classList.remove('border-blue-400', 'bg-blue-50');
      }

      function handleDrop(e) {
        e.preventDefault();
        e.stopPropagation();

        this.classList.remove('border-blue-400', 'bg-blue-50');

        if (dragSrcEl !== this) {
          // Reorder in DOM.
          var list = el.querySelector('.sidebar-config-list');
          var items = Array.from(list.querySelectorAll('.sidebar-config-item'));
          var fromIdx = items.indexOf(dragSrcEl);
          var toIdx = items.indexOf(this);

          if (fromIdx < toIdx) {
            list.insertBefore(dragSrcEl, this.nextSibling);
          } else {
            list.insertBefore(dragSrcEl, this);
          }

          // Update config and save.
          updateOrderFromDOM();
          save();
        }

        return false;
      }

      function handleDragEnd() {
        this.classList.remove('opacity-50', 'border-accent');
        var items = el.querySelectorAll('.sidebar-config-item');
        items.forEach(function (item) {
          item.classList.remove('border-blue-400', 'bg-blue-50');
        });
      }

      /**
       * Read current order from DOM elements.
       */
      function updateOrderFromDOM() {
        var items = el.querySelectorAll('.sidebar-config-item');
        sidebarConfig.entity_type_order = Array.from(items).map(function (item) {
          return parseInt(item.getAttribute('data-type-id'), 10);
        });
      }

      /**
       * Toggle visibility of an entity type.
       */
      function toggleVisibility(typeID) {
        var idx = (sidebarConfig.hidden_type_ids || []).indexOf(typeID);
        if (idx === -1) {
          sidebarConfig.hidden_type_ids = (sidebarConfig.hidden_type_ids || []).concat([typeID]);
        } else {
          sidebarConfig.hidden_type_ids = sidebarConfig.hidden_type_ids.filter(function (id) {
            return id !== typeID;
          });
        }
        save();
        render();
      }

      /**
       * Save configuration to server.
       */
      function save() {
        var csrfMatch = document.cookie.match('(?:^|; )chronicle_csrf=([^;]*)');
        var csrf = csrfMatch ? decodeURIComponent(csrfMatch[1]) : '';

        fetch(endpoint, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': csrf
          },
          credentials: 'same-origin',
          body: JSON.stringify({
            entity_type_order: sidebarConfig.entity_type_order || [],
            hidden_type_ids: sidebarConfig.hidden_type_ids || []
          })
        })
          .then(function (res) {
            if (!res.ok) console.error('[sidebar-config] Save returned HTTP ' + res.status);
          })
          .catch(function (err) {
            console.error('[sidebar-config] Save failed:', err);
          });
      }

      /**
       * Escape HTML to prevent XSS.
       */
      function escapeHtml(text) {
        var div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
      }

      /**
       * Escape a string for use in an HTML attribute value.
       * Strips characters that could break out of class/style attributes.
       */
      function escapeAttr(text) {
        return String(text).replace(/[&"'<>]/g, function (c) {
          return { '&': '&amp;', '"': '&quot;', "'": '&#39;', '<': '&lt;', '>': '&gt;' }[c];
        });
      }
    },

    destroy: function (el) {
      el.innerHTML = '';
    }
  });
})();
