/**
 * layout_builder.js -- Entity Type Layout Builder Widget
 *
 * Mounts on a data-widget="layout-builder" element. Renders a two-column
 * layout editor that lets campaign owners customize entity profile page
 * sections. Sections can be dragged between left (sidebar) and right (main)
 * columns, reordered, and toggled.
 *
 * Config attributes:
 *   data-endpoint="/campaigns/:id/entity-types/:etid/layout"  -- API endpoint
 *   data-entity-type-name="Character"  -- Display name
 *
 * The widget fetches the current layout and field definitions on init,
 * renders the two-column editor, and saves on every change.
 */
(function () {
  'use strict';

  Chronicle.register('layout-builder', {
    init: function (el, config) {
      var endpoint = config.endpoint;
      if (!endpoint) {
        console.error('[layout-builder] Missing data-endpoint');
        return;
      }

      var typeName = config.entityTypeName || 'Entity';
      var layout = { sections: [] };
      var fields = [];

      // Fetch current layout from server.
      fetch(endpoint, {
        headers: { 'Accept': 'application/json' },
        credentials: 'same-origin'
      })
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          return res.json();
        })
        .then(function (data) {
          layout = (data.layout && data.layout.sections) ? data.layout : { sections: [] };
          fields = data.fields || [];

          // If layout is empty, create default sections based on fields.
          if (layout.sections.length === 0) {
            layout.sections = buildDefaultSections(fields);
          }

          render();
        })
        .catch(function (err) {
          console.error('[layout-builder] Failed to fetch layout:', err);
          el.innerHTML = '<p class="text-sm text-red-600">Failed to load layout.</p>';
        });

      /**
       * Build default sections from field definitions.
       */
      function buildDefaultSections(fieldDefs) {
        var sections = [];

        // Group fields by section name.
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

        // Main entry always in right column.
        sections.push({
          key: 'entry',
          label: 'Main Entry',
          type: 'entry',
          column: 'right'
        });

        return sections;
      }

      /**
       * Render the two-column layout editor.
       */
      function render() {
        var leftSections = layout.sections.filter(function (s) { return s.column === 'left'; });
        var rightSections = layout.sections.filter(function (s) { return s.column !== 'left'; });

        var html = '<div class="grid grid-cols-2 gap-4">';

        // Left column (sidebar).
        html += '<div class="space-y-2">';
        html += '<h3 class="text-xs font-semibold uppercase tracking-wider text-gray-500 mb-2">Left Column (Sidebar)</h3>';
        html += '<div class="layout-column min-h-[100px] border-2 border-dashed border-gray-200 rounded-lg p-2 space-y-1" data-column="left">';
        leftSections.forEach(function (s) {
          html += sectionItem(s);
        });
        html += '</div>';
        html += '</div>';

        // Right column (main).
        html += '<div class="space-y-2">';
        html += '<h3 class="text-xs font-semibold uppercase tracking-wider text-gray-500 mb-2">Right Column (Main)</h3>';
        html += '<div class="layout-column min-h-[100px] border-2 border-dashed border-gray-200 rounded-lg p-2 space-y-1" data-column="right">';
        rightSections.forEach(function (s) {
          html += sectionItem(s);
        });
        html += '</div>';
        html += '</div>';

        html += '</div>';
        html += '<p class="text-xs text-gray-500 mt-2">Drag sections between columns to customize the ' + escapeHtml(typeName) + ' profile layout.</p>';

        el.innerHTML = html;

        // Bind drag events to section items and columns.
        bindDragEvents();
      }

      /**
       * Render a single section item.
       */
      function sectionItem(section) {
        var icon = section.type === 'entry' ? 'fa-align-left' :
                   section.type === 'posts' ? 'fa-comments' : 'fa-list';

        return '<div class="layout-section-item flex items-center px-3 py-2 rounded-md border border-gray-200 bg-white cursor-grab select-none text-sm" ' +
          'draggable="true" data-section-key="' + escapeAttr(section.key) + '">' +
          '<span class="drag-handle mr-2 text-gray-400"><i class="fa-solid fa-grip-vertical text-xs"></i></span>' +
          '<span class="w-4 h-4 mr-2 flex items-center justify-center">' +
          '<i class="fa-solid ' + icon + ' text-xs text-gray-500"></i>' +
          '</span>' +
          '<span class="flex-1 text-gray-700">' + escapeHtml(section.label) + '</span>' +
          '<span class="text-xs text-gray-400">' + escapeHtml(section.type) + '</span>' +
          '</div>';
      }

      /**
       * Bind drag-and-drop events to section items and columns.
       */
      function bindDragEvents() {
        var items = el.querySelectorAll('.layout-section-item');
        var columns = el.querySelectorAll('.layout-column');

        items.forEach(function (item) {
          item.addEventListener('dragstart', function (e) {
            item.classList.add('opacity-50');
            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('text/plain', item.getAttribute('data-section-key'));
          });

          item.addEventListener('dragend', function () {
            item.classList.remove('opacity-50');
            columns.forEach(function (col) {
              col.classList.remove('border-blue-400', 'bg-blue-50');
            });
          });
        });

        columns.forEach(function (col) {
          col.addEventListener('dragover', function (e) {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'move';
          });

          col.addEventListener('dragenter', function (e) {
            e.preventDefault();
            col.classList.add('border-blue-400', 'bg-blue-50');
          });

          col.addEventListener('dragleave', function (e) {
            // Only remove highlight if leaving the column itself.
            if (e.target === col) {
              col.classList.remove('border-blue-400', 'bg-blue-50');
            }
          });

          col.addEventListener('drop', function (e) {
            e.preventDefault();
            col.classList.remove('border-blue-400', 'bg-blue-50');

            var sectionKey = e.dataTransfer.getData('text/plain');
            var targetColumn = col.getAttribute('data-column');

            // Find the dragged element.
            var escapedKey = typeof CSS !== 'undefined' && CSS.escape ? CSS.escape(sectionKey) : sectionKey;
            var draggedEl = el.querySelector('[data-section-key="' + escapedKey + '"]');
            if (!draggedEl) return;

            // Move the element to this column.
            col.appendChild(draggedEl);

            // Update layout from DOM.
            updateLayoutFromDOM();
            save();
          });
        });
      }

      /**
       * Rebuild layout.sections from current DOM state.
       */
      function updateLayoutFromDOM() {
        var newSections = [];

        var columns = el.querySelectorAll('.layout-column');
        columns.forEach(function (col) {
          var colName = col.getAttribute('data-column');
          var items = col.querySelectorAll('.layout-section-item');
          items.forEach(function (item) {
            var key = item.getAttribute('data-section-key');
            // Find original section data.
            var original = layout.sections.find(function (s) { return s.key === key; });
            if (original) {
              newSections.push({
                key: original.key,
                label: original.label,
                type: original.type,
                column: colName
              });
            }
          });
        });

        layout.sections = newSections;
      }

      /**
       * Save layout to server.
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
          body: JSON.stringify({ layout: layout })
        })
          .then(function (res) {
            if (!res.ok) console.error('[layout-builder] Save returned HTTP ' + res.status);
          })
          .catch(function (err) {
            console.error('[layout-builder] Save failed:', err);
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
       * Escape a string for safe use in an HTML attribute value.
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
