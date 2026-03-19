/**
 * sidebar_layout_editor.js -- Unified Sidebar Layout Editor
 *
 * Replaces the separate entity-type-config and sidebar-nav-editor widgets
 * with a single drag-and-drop editor that controls ALL sidebar items:
 * dashboard, addon links, categories, custom sections, and custom links.
 *
 * Mounts on: data-widget="sidebar-layout-editor"
 * Attributes:
 *   data-endpoint     - sidebar config API URL
 *   data-campaign-id  - campaign ID
 *   data-entity-types - JSON array of entity types [{id, name, name_plural, icon, color}]
 */
(function () {
  'use strict';

  // Known addon definitions for generating default items.
  var KNOWN_ADDONS = [
    { slug: 'notes', label: 'Journal', icon: 'fa-book-open' },
    { slug: 'npcs', label: 'NPCs', icon: 'fa-users' },
    { slug: 'armory', label: 'Armory', icon: 'fa-shield-halved' }
  ];

  Chronicle.register('sidebar-layout-editor', {
    init: function (el) {
      var endpoint = el.dataset.endpoint;
      var campaignId = el.dataset.campaignId;
      var entityTypes = [];
      try { entityTypes = JSON.parse(el.dataset.entityTypes || '[]'); } catch (e) { /* ignore */ }

      var config = null;
      var items = [];

      // Load current config.
      Chronicle.apiFetch(endpoint)
        .then(function (res) { return res.ok ? res.json() : {}; })
        .then(function (data) {
          config = data || {};
          if (config.items && config.items.length > 0) {
            items = config.items;
          } else {
            // Generate default items from entity types and addons.
            items = generateDefaults(entityTypes);
          }
          render();
        })
        .catch(function () { render(); });

      function generateDefaults(types) {
        var defaults = [];
        defaults.push({ type: 'dashboard', visible: true });

        KNOWN_ADDONS.forEach(function (addon) {
          defaults.push({ type: 'addon', slug: addon.slug, label: addon.label, icon: addon.icon, visible: true });
        });

        types.forEach(function (et) {
          defaults.push({ type: 'category', type_id: et.id, visible: true });
        });

        defaults.push({ type: 'all_pages', visible: true });
        return defaults;
      }

      function getItemLabel(item) {
        switch (item.type) {
          case 'dashboard': return 'Dashboard';
          case 'all_pages': return 'All Pages';
          case 'addon': return item.label || item.slug;
          case 'category':
            for (var i = 0; i < entityTypes.length; i++) {
              if (entityTypes[i].id === item.type_id) return entityTypes[i].name_plural || entityTypes[i].name;
            }
            return 'Category #' + item.type_id;
          case 'section': return item.label || 'Section';
          case 'link': return item.label || item.url || 'Link';
          default: return item.type;
        }
      }

      function getItemIcon(item) {
        switch (item.type) {
          case 'dashboard': return 'fa-home';
          case 'all_pages': return 'fa-layer-group';
          case 'addon': return item.icon || 'fa-puzzle-piece';
          case 'category':
            for (var i = 0; i < entityTypes.length; i++) {
              if (entityTypes[i].id === item.type_id) return entityTypes[i].icon || 'fa-folder';
            }
            return 'fa-folder';
          case 'section': return 'fa-grip-lines';
          case 'link': return item.icon || 'fa-link';
          default: return 'fa-circle';
        }
      }

      function getItemColor(item) {
        if (item.type === 'category') {
          for (var i = 0; i < entityTypes.length; i++) {
            if (entityTypes[i].id === item.type_id) return entityTypes[i].color || '';
          }
        }
        return '';
      }

      function render() {
        var html = '<div class="space-y-1" id="sidebar-layout-list">';

        items.forEach(function (item, idx) {
          var label = Chronicle.escapeHtml(getItemLabel(item));
          var icon = getItemIcon(item);
          var color = getItemColor(item);
          var vis = item.visible !== false;
          var iconColor = color ? 'style="color:' + Chronicle.escapeHtml(color) + '"' : '';

          html += '<div class="flex items-center gap-2 px-3 py-2 rounded-md bg-surface-raised border border-border/50 group ' +
            (vis ? '' : 'opacity-40') + '" draggable="true" data-idx="' + idx + '">';

          // Drag handle.
          html += '<span class="cursor-grab text-fg-muted"><i class="fa-solid fa-grip-vertical text-xs"></i></span>';

          // Icon.
          html += '<span class="w-5 h-5 flex items-center justify-center shrink-0" ' + iconColor + '>';
          html += '<i class="fa-solid ' + Chronicle.escapeHtml(icon) + ' text-xs"></i></span>';

          // Label.
          html += '<span class="flex-1 text-sm text-fg truncate">' + label + '</span>';

          // Type badge.
          html += '<span class="text-[10px] text-fg-muted uppercase">' + Chronicle.escapeHtml(item.type) + '</span>';

          // Edit button for sections and links.
          if (item.type === 'section' || item.type === 'link') {
            html += '<button type="button" class="p-1 text-fg-muted hover:text-fg transition-colors" data-edit="' + idx + '" title="Edit">';
            html += '<i class="fa-solid fa-pen text-[10px]"></i></button>';
          }

          // Visibility toggle.
          html += '<button type="button" class="p-1 text-fg-muted hover:text-fg transition-colors" data-toggle="' + idx + '" title="' + (vis ? 'Hide' : 'Show') + '">';
          html += '<i class="fa-solid ' + (vis ? 'fa-eye' : 'fa-eye-slash') + ' text-xs"></i></button>';

          // Delete button for sections and links.
          if (item.type === 'section' || item.type === 'link') {
            html += '<button type="button" class="p-1 text-fg-muted hover:text-red-400 transition-colors" data-delete="' + idx + '" title="Remove">';
            html += '<i class="fa-solid fa-trash text-[10px]"></i></button>';
          }

          html += '</div>';
        });

        html += '</div>';

        // Add buttons.
        html += '<div class="flex gap-2 mt-3">';
        html += '<button type="button" id="sle-add-section" class="btn btn-sm btn-secondary"><i class="fa-solid fa-grip-lines mr-1.5"></i> Add Section</button>';
        html += '<button type="button" id="sle-add-link" class="btn btn-sm btn-secondary"><i class="fa-solid fa-link mr-1.5"></i> Add Link</button>';
        html += '</div>';

        el.innerHTML = html;
        bindEvents();
        bindDragDrop();
      }

      function bindEvents() {
        // Visibility toggles.
        el.querySelectorAll('[data-toggle]').forEach(function (btn) {
          btn.addEventListener('click', function () {
            var idx = parseInt(btn.dataset.toggle, 10);
            items[idx].visible = !items[idx].visible;
            save();
            render();
          });
        });

        // Delete buttons.
        el.querySelectorAll('[data-delete]').forEach(function (btn) {
          btn.addEventListener('click', function () {
            var idx = parseInt(btn.dataset.delete, 10);
            if (!confirm('Remove "' + getItemLabel(items[idx]) + '"?')) return;
            items.splice(idx, 1);
            save();
            render();
          });
        });

        // Edit buttons.
        el.querySelectorAll('[data-edit]').forEach(function (btn) {
          btn.addEventListener('click', function () {
            var idx = parseInt(btn.dataset.edit, 10);
            editItem(idx);
          });
        });

        // Add section.
        var addSec = el.querySelector('#sle-add-section');
        if (addSec) {
          addSec.addEventListener('click', function () {
            var label = prompt('Section label:');
            if (!label || !label.trim()) return;
            items.push({
              type: 'section',
              id: 'sec_' + Math.random().toString(36).substr(2, 8),
              label: label.trim(),
              visible: true
            });
            save();
            render();
          });
        }

        // Add link.
        var addLink = el.querySelector('#sle-add-link');
        if (addLink) {
          addLink.addEventListener('click', function () {
            var label = prompt('Link label:');
            if (!label || !label.trim()) return;
            var url = prompt('URL (e.g. /path or https://...):');
            if (!url || !url.trim()) return;
            items.push({
              type: 'link',
              id: 'lnk_' + Math.random().toString(36).substr(2, 8),
              label: label.trim(),
              url: url.trim(),
              icon: 'fa-link',
              visible: true
            });
            save();
            render();
          });
        }
      }

      function editItem(idx) {
        var item = items[idx];
        var label = prompt('Label:', item.label || '');
        if (label === null) return;
        item.label = label.trim();
        if (item.type === 'link') {
          var url = prompt('URL:', item.url || '');
          if (url !== null) item.url = url.trim();
          var icon = prompt('Icon (FontAwesome class, e.g. fa-globe):', item.icon || '');
          if (icon !== null) item.icon = icon.trim();
        }
        save();
        render();
      }

      var dragSrcIdx = null;

      function bindDragDrop() {
        var list = el.querySelector('#sidebar-layout-list');
        if (!list) return;

        var rows = list.querySelectorAll('[data-idx]');
        rows.forEach(function (row) {
          row.addEventListener('dragstart', function (e) {
            dragSrcIdx = parseInt(row.dataset.idx, 10);
            row.classList.add('opacity-40');
            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('text/plain', String(dragSrcIdx));
          });

          row.addEventListener('dragover', function (e) {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'move';
            row.classList.add('ring-1', 'ring-accent/50');
          });

          row.addEventListener('dragleave', function () {
            row.classList.remove('ring-1', 'ring-accent/50');
          });

          row.addEventListener('drop', function (e) {
            e.preventDefault();
            row.classList.remove('ring-1', 'ring-accent/50');
            var toIdx = parseInt(row.dataset.idx, 10);
            if (dragSrcIdx === null || dragSrcIdx === toIdx) return;

            // Move item.
            var moved = items.splice(dragSrcIdx, 1)[0];
            items.splice(toIdx, 0, moved);
            dragSrcIdx = null;
            save();
            render();
          });

          row.addEventListener('dragend', function () {
            row.classList.remove('opacity-40');
            dragSrcIdx = null;
          });
        });
      }

      function save() {
        Chronicle.apiFetch(endpoint, {
          method: 'PUT',
          body: {
            items: items,
            hidden_entity_ids: (config && config.hidden_entity_ids) || []
          }
        })
        .then(function (res) {
          if (res.ok) {
            Chronicle.notify('Sidebar updated', 'success');
          } else {
            Chronicle.notify('Failed to save sidebar', 'error');
          }
        })
        .catch(function () {
          Chronicle.notify('Failed to save sidebar', 'error');
        });
      }
    }
  });
})();
