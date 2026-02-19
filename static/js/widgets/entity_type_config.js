/**
 * entity_type_config.js -- Unified Entity Type Configuration Widget
 *
 * Combines sidebar ordering, visibility toggling, color picking, and layout
 * editing into a single cohesive UI. Replaces the separate sidebar_config
 * and layout_builder widgets.
 *
 * Config attributes:
 *   data-sidebar-endpoint  -- API for sidebar config (GET/PUT)
 *   data-layout-base       -- Base URL for entity type APIs (e.g., /campaigns/:id/entity-types)
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
      var layouts = {};       // etID -> { sections: [] }
      var fieldsByType = {};   // etID -> []
      var expandedType = null; // Currently expanded entity type ID.
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
          var expanded = expandedType === t.id;

          // Main row.
          html += '<div class="et-config-item border border-gray-200 rounded-md' +
            (hidden ? ' opacity-50' : '') +
            (expanded ? ' border-gray-400 ring-1 ring-gray-300' : '') +
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

          // Expand/collapse layout button.
          html += '<button type="button" class="toggle-expand p-1 mr-1.5 text-xs rounded hover:bg-gray-100 transition-colors" data-type-id="' + t.id + '" title="' +
            (expanded ? 'Collapse layout' : 'Edit layout') + '">';
          html += '<i class="fa-solid ' + (expanded ? 'fa-chevron-up' : 'fa-sliders') + ' text-gray-400"></i>';
          html += '</button>';

          // Visibility toggle.
          html += '<button type="button" class="toggle-visibility p-1 text-xs rounded hover:bg-gray-100 transition-colors" data-type-id="' + t.id + '" title="' +
            (hidden ? 'Show in sidebar' : 'Hide from sidebar') + '">';
          html += '<i class="fa-solid ' + (hidden ? 'fa-eye-slash text-gray-400' : 'fa-eye text-gray-600') + '"></i>';
          html += '</button>';

          html += '</div>'; // end header

          // Expandable layout section.
          if (expanded) {
            html += '<div class="et-config-layout px-3 pb-3 border-t border-gray-100 mt-0 pt-3">';
            html += renderLayout(t);
            html += '</div>';
          }

          html += '</div>'; // end item
        });

        html += '</div>'; // end list
        html += '<p class="text-xs text-gray-400 mt-3">Drag to reorder sidebar. Click the color circle to change. Click <i class="fa-solid fa-sliders text-xs"></i> to edit profile layout.</p>';

        el.innerHTML = html;
        bindEvents();
      }

      // --- Layout Rendering ---

      function renderLayout(t) {
        var layout = layouts[t.id];
        if (!layout) {
          return '<p class="text-xs text-gray-500 italic">Loading layout...</p>';
        }

        var leftSections = layout.sections.filter(function (s) { return s.column === 'left'; });
        var rightSections = layout.sections.filter(function (s) { return s.column !== 'left'; });

        var html = '<div class="grid grid-cols-2 gap-3">';

        // Left column.
        html += '<div>';
        html += '<h4 class="text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1.5">Sidebar</h4>';
        html += '<div class="layout-column min-h-[60px] border border-dashed border-gray-200 rounded-md p-1.5 space-y-1" data-column="left" data-type-id="' + t.id + '">';
        leftSections.forEach(function (s) { html += layoutSectionItem(s); });
        html += '</div></div>';

        // Right column.
        html += '<div>';
        html += '<h4 class="text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1.5">Main</h4>';
        html += '<div class="layout-column min-h-[60px] border border-dashed border-gray-200 rounded-md p-1.5 space-y-1" data-column="right" data-type-id="' + t.id + '">';
        rightSections.forEach(function (s) { html += layoutSectionItem(s); });
        html += '</div></div>';

        html += '</div>';
        return html;
      }

      function layoutSectionItem(section) {
        var icon = section.type === 'entry' ? 'fa-align-left' :
                   section.type === 'posts' ? 'fa-comments' : 'fa-list';

        return '<div class="layout-section-item flex items-center px-2 py-1.5 rounded border border-gray-200 bg-white cursor-grab select-none text-xs" ' +
          'draggable="true" data-section-key="' + escapeAttr(section.key) + '">' +
          '<i class="fa-solid fa-grip-vertical text-gray-300 mr-1.5 text-[10px]"></i>' +
          '<i class="fa-solid ' + icon + ' text-gray-400 mr-1.5 text-[10px]"></i>' +
          '<span class="flex-1 text-gray-600">' + escapeHtml(section.label) + '</span>' +
          '<span class="text-[10px] text-gray-400">' + escapeHtml(section.type) + '</span>' +
          '</div>';
      }

      // --- Default layout builder ---

      function buildDefaultSections(fieldDefs) {
        var sections = [];
        var sectionMap = {};
        fieldDefs.forEach(function (f) {
          var sec = f.section || 'Details';
          if (!sectionMap[sec]) {
            sectionMap[sec] = true;
            sections.push({
              key: sec.toLowerCase().replace(/[^a-z0-9]+/g, '-'),
              label: sec,
              type: 'fields',
              column: 'left'
            });
          }
        });
        sections.push({ key: 'entry', label: 'Main Entry', type: 'entry', column: 'right' });
        return sections;
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

        // Expand/collapse toggles.
        el.querySelectorAll('.toggle-expand').forEach(function (btn) {
          btn.addEventListener('click', function (e) {
            e.stopPropagation();
            var typeID = parseInt(btn.getAttribute('data-type-id'), 10);
            toggleExpand(typeID);
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

        // Layout section drag events.
        bindLayoutDragEvents();
      }

      function bindLayoutDragEvents() {
        var sectionItems = el.querySelectorAll('.layout-section-item');
        var columns = el.querySelectorAll('.layout-column');

        sectionItems.forEach(function (item) {
          item.addEventListener('dragstart', function (e) {
            e.stopPropagation(); // Don't trigger parent drag.
            item.classList.add('opacity-40');
            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('text/plain', 'section:' + item.getAttribute('data-section-key'));
          });
          item.addEventListener('dragend', function () {
            item.classList.remove('opacity-40');
            columns.forEach(function (col) { col.classList.remove('border-blue-400', 'bg-blue-50'); });
          });
        });

        columns.forEach(function (col) {
          col.addEventListener('dragover', function (e) {
            // Only accept layout section drags (not entity type reorder).
            var data = e.dataTransfer.types;
            e.preventDefault();
            e.dataTransfer.dropEffect = 'move';
          });
          col.addEventListener('dragenter', function (e) {
            e.preventDefault();
            col.classList.add('border-blue-400', 'bg-blue-50');
          });
          col.addEventListener('dragleave', function (e) {
            if (e.target === col) col.classList.remove('border-blue-400', 'bg-blue-50');
          });
          col.addEventListener('drop', function (e) {
            e.preventDefault();
            e.stopPropagation();
            col.classList.remove('border-blue-400', 'bg-blue-50');

            var rawData = e.dataTransfer.getData('text/plain');
            if (!rawData.startsWith('section:')) return;
            var sectionKey = rawData.substring(8);
            var typeID = parseInt(col.getAttribute('data-type-id'), 10);

            var escapedKey = typeof CSS !== 'undefined' && CSS.escape ? CSS.escape(sectionKey) : sectionKey;
            var draggedEl = el.querySelector('.layout-section-item[data-section-key="' + escapedKey + '"]');
            if (!draggedEl) return;

            col.appendChild(draggedEl);
            updateLayoutFromDOM(typeID);
            saveLayout(typeID);
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

      function toggleExpand(typeID) {
        if (expandedType === typeID) {
          expandedType = null;
          render();
          return;
        }

        expandedType = typeID;

        // Load layout if not cached.
        if (!layouts[typeID]) {
          render(); // Show "Loading..."
          fetch(layoutBase + '/' + typeID + '/layout', {
            headers: { 'Accept': 'application/json' },
            credentials: 'same-origin'
          })
            .then(function (res) {
              if (!res.ok) throw new Error('HTTP ' + res.status);
              return res.json();
            })
            .then(function (data) {
              var layout = (data.layout && data.layout.sections) ? data.layout : { sections: [] };
              fieldsByType[typeID] = data.fields || [];

              if (layout.sections.length === 0) {
                layout.sections = buildDefaultSections(fieldsByType[typeID]);
              }
              layouts[typeID] = layout;
              render();
            })
            .catch(function () {
              layouts[typeID] = { sections: [{ key: 'entry', label: 'Main Entry', type: 'entry', column: 'right' }] };
              render();
            });
        } else {
          render();
        }
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

      function updateLayoutFromDOM(typeID) {
        var layout = layouts[typeID];
        if (!layout) return;

        var newSections = [];
        var columns = el.querySelectorAll('.layout-column[data-type-id="' + typeID + '"]');
        columns.forEach(function (col) {
          var colName = col.getAttribute('data-column');
          col.querySelectorAll('.layout-section-item').forEach(function (item) {
            var key = item.getAttribute('data-section-key');
            var original = layout.sections.find(function (s) { return s.key === key; });
            if (original) {
              newSections.push({ key: original.key, label: original.label, type: original.type, column: colName });
            }
          });
        });
        layout.sections = newSections;
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

      function saveLayout(typeID) {
        var layout = layouts[typeID];
        if (!layout) return;

        fetch(layoutBase + '/' + typeID + '/layout', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCSRF() },
          credentials: 'same-origin',
          body: JSON.stringify({ layout: layout })
        })
          .then(function (res) {
            if (!res.ok) console.error('[entity-type-config] Layout save failed: HTTP ' + res.status);
          })
          .catch(function (err) {
            console.error('[entity-type-config] Layout save error:', err);
          });
      }
    },

    destroy: function (el) {
      el.innerHTML = '';
    }
  });
})();
