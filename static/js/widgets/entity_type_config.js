/**
 * entity_type_config.js -- Unified Entity Type Configuration Widget
 *
 * Combines sidebar ordering, visibility toggling, and color picking into
 * a single cohesive UI. Each entity type also gets a link to the visual
 * template editor page.
 *
 * Config attributes:
 *   data-sidebar-endpoint  -- API for sidebar config (GET/PUT)
 *   data-layout-base       -- Base URL for entity type routes (e.g., /campaigns/:id/entity-types)
 *   data-entity-types      -- JSON array of entity types from server
 */
(function () {
  'use strict';

  Chronicle.register('entity-type-config', {
    init: function (el, config) {
      var sidebarEndpoint = config.sidebarEndpoint;
      var layoutBase = config.layoutBase;

      if (!sidebarEndpoint || !layoutBase) {
        console.error('[entity-type-config] Missing data-sidebar-endpoint or data-layout-base');
        return;
      }

      // State.
      var entityTypes = [];
      var sidebarConfig = { entity_type_order: [], hidden_type_ids: [] };
      var dragSrcEl = null;

      // Parse entity types from data attribute.
      try {
        entityTypes = JSON.parse(el.getAttribute('data-entity-types') || '[]');
      } catch (e) {
        console.error('[entity-type-config] Invalid entity types JSON');
        return;
      }

      // Fetch sidebar config from server.
      fetch(sidebarEndpoint, {
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
          render();
        });

      // --- Helpers ---

      function getCSRF() {
        var m = document.cookie.match('(?:^|; )chronicle_csrf=([^;]*)');
        return m ? decodeURIComponent(m[1]) : '';
      }

      function escapeHtml(text) {
        var div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
      }

      function escapeAttr(text) {
        return String(text).replace(/[&"'<>]/g, function (c) {
          return { '&': '&amp;', '"': '&quot;', "'": '&#39;', '<': '&lt;', '>': '&gt;' }[c];
        });
      }

      // --- Ordering ---

      function getOrderedTypes() {
        var order = sidebarConfig.entity_type_order;
        if (!order || order.length === 0) return entityTypes.slice();

        var typeMap = {};
        entityTypes.forEach(function (t) { typeMap[t.id] = t; });

        var result = [];
        var seen = {};
        order.forEach(function (id) {
          if (typeMap[id]) { result.push(typeMap[id]); seen[id] = true; }
        });
        entityTypes.forEach(function (t) {
          if (!seen[t.id]) result.push(t);
        });
        return result;
      }

      function isHidden(typeID) {
        return (sidebarConfig.hidden_type_ids || []).indexOf(typeID) !== -1;
      }

      // --- Render ---

      function render() {
        var types = getOrderedTypes();
        var html = '<div class="et-config-list space-y-1">';

        types.forEach(function (t) {
          var hidden = isHidden(t.id);

          // Main row.
          html += '<div class="et-config-item border border-gray-200 rounded-md' +
            (hidden ? ' opacity-50' : '') +
            '" data-type-id="' + t.id + '">';

          // Header row: drag handle, color, icon, name, actions.
          html += '<div class="et-config-header flex items-center px-3 py-2.5 cursor-grab select-none" draggable="true">';

          // Drag handle.
          html += '<span class="drag-handle mr-2.5 text-gray-300 hover:text-gray-500"><i class="fa-solid fa-grip-vertical text-xs"></i></span>';

          // Color swatch (clickable).
          html += '<label class="relative mr-2.5 cursor-pointer" title="Change color">';
          html += '<span class="w-4 h-4 rounded-full block border border-gray-300" style="background-color: ' + escapeAttr(t.color || '#6b7280') + '"></span>';
          html += '<input type="color" class="color-picker absolute inset-0 w-full h-full opacity-0 cursor-pointer" data-type-id="' + t.id + '" value="' + escapeAttr(t.color || '#6b7280') + '"/>';
          html += '</label>';

          // Icon.
          html += '<span class="w-4 h-4 mr-2 flex items-center justify-center">';
          html += '<i class="fa-solid ' + escapeAttr(t.icon || 'fa-file') + ' text-xs" style="color: ' + escapeAttr(t.color || '#6b7280') + '"></i>';
          html += '</span>';

          // Name.
          html += '<span class="flex-1 text-sm font-medium text-gray-700">' + escapeHtml(t.name_plural || t.name) + '</span>';

          // Template editor link.
          html += '<a href="' + escapeAttr(layoutBase) + '/' + t.id + '/template" class="p-1 mr-1.5 text-xs rounded hover:bg-gray-100 transition-colors" title="Edit page template">';
          html += '<i class="fa-solid fa-table-cells-large text-gray-400"></i>';
          html += '</a>';

          // Visibility toggle.
          html += '<button type="button" class="toggle-visibility p-1 text-xs rounded hover:bg-gray-100 transition-colors" data-type-id="' + t.id + '" title="' +
            (hidden ? 'Show in sidebar' : 'Hide from sidebar') + '">';
          html += '<i class="fa-solid ' + (hidden ? 'fa-eye-slash text-gray-400' : 'fa-eye text-gray-600') + '"></i>';
          html += '</button>';

          html += '</div>'; // end header
          html += '</div>'; // end item
        });

        html += '</div>'; // end list
        html += '<p class="text-xs text-gray-400 mt-3">Drag to reorder sidebar. Click the color circle to change. Click <i class="fa-solid fa-table-cells-large text-xs"></i> to design page template.</p>';

        el.innerHTML = html;
        bindEvents();
      }

      // --- Events ---

      function bindEvents() {
        // Color pickers.
        el.querySelectorAll('.color-picker').forEach(function (input) {
          input.addEventListener('change', function () {
            var typeID = parseInt(input.getAttribute('data-type-id'), 10);
            var newColor = input.value;
            updateColor(typeID, newColor);
          });
        });

        // Visibility toggles.
        el.querySelectorAll('.toggle-visibility').forEach(function (btn) {
          btn.addEventListener('click', function (e) {
            e.stopPropagation();
            var typeID = parseInt(btn.getAttribute('data-type-id'), 10);
            toggleVisibility(typeID);
          });
        });

        // Drag events for sidebar reordering.
        var items = el.querySelectorAll('.et-config-header');
        items.forEach(function (header) {
          var item = header.parentElement;
          header.addEventListener('dragstart', function (e) {
            dragSrcEl = item;
            item.classList.add('opacity-40');
            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('text/plain', item.getAttribute('data-type-id'));
          });
          header.addEventListener('dragend', function () {
            item.classList.remove('opacity-40');
            el.querySelectorAll('.et-config-item').forEach(function (i) {
              i.classList.remove('border-blue-400');
            });
          });
        });

        el.querySelectorAll('.et-config-item').forEach(function (item) {
          item.addEventListener('dragover', function (e) {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'move';
          });
          item.addEventListener('dragenter', function (e) {
            e.preventDefault();
            if (item !== dragSrcEl) item.classList.add('border-blue-400');
          });
          item.addEventListener('dragleave', function (e) {
            if (e.target === item || e.target === item.querySelector('.et-config-header')) {
              item.classList.remove('border-blue-400');
            }
          });
          item.addEventListener('drop', function (e) {
            e.preventDefault();
            e.stopPropagation();
            item.classList.remove('border-blue-400');
            if (dragSrcEl && dragSrcEl !== item) {
              var list = el.querySelector('.et-config-list');
              var allItems = Array.from(list.querySelectorAll('.et-config-item'));
              var fromIdx = allItems.indexOf(dragSrcEl);
              var toIdx = allItems.indexOf(item);
              if (fromIdx < toIdx) {
                list.insertBefore(dragSrcEl, item.nextSibling);
              } else {
                list.insertBefore(dragSrcEl, item);
              }
              updateOrderFromDOM();
              saveSidebarConfig();
            }
          });
        });
      }

      // --- Actions ---

      function toggleVisibility(typeID) {
        var idx = (sidebarConfig.hidden_type_ids || []).indexOf(typeID);
        if (idx === -1) {
          sidebarConfig.hidden_type_ids = (sidebarConfig.hidden_type_ids || []).concat([typeID]);
        } else {
          sidebarConfig.hidden_type_ids = sidebarConfig.hidden_type_ids.filter(function (id) { return id !== typeID; });
        }
        saveSidebarConfig();
        render();
      }

      function updateColor(typeID, newColor) {
        // Optimistic update.
        entityTypes.forEach(function (t) {
          if (t.id === typeID) t.color = newColor;
        });

        fetch(layoutBase + '/' + typeID + '/color', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCSRF() },
          credentials: 'same-origin',
          body: JSON.stringify({ color: newColor })
        })
          .then(function (res) {
            if (!res.ok) console.error('[entity-type-config] Color save failed: HTTP ' + res.status);
          })
          .catch(function (err) {
            console.error('[entity-type-config] Color save error:', err);
          });

        render();
      }

      function updateOrderFromDOM() {
        var items = el.querySelectorAll('.et-config-item');
        sidebarConfig.entity_type_order = Array.from(items).map(function (item) {
          return parseInt(item.getAttribute('data-type-id'), 10);
        });
      }

      // --- Save ---

      function saveSidebarConfig() {
        fetch(sidebarEndpoint, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCSRF() },
          credentials: 'same-origin',
          body: JSON.stringify({
            entity_type_order: sidebarConfig.entity_type_order || [],
            hidden_type_ids: sidebarConfig.hidden_type_ids || []
          })
        })
          .then(function (res) {
            if (!res.ok) console.error('[entity-type-config] Sidebar save failed: HTTP ' + res.status);
          })
          .catch(function (err) {
            console.error('[entity-type-config] Sidebar save error:', err);
          });
      }
    },

    destroy: function (el) {
      el.innerHTML = '';
    }
  });
})();
