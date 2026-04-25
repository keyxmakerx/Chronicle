/**
 * sidebar_editor.js -- Unified Sidebar Edit Mode
 *
 * Replaces sidebar_reorg.js and sidebar_layout_editor.js with a single
 * inline editing experience. One pencil button triggers edit mode for
 * all sidebar items (dashboard, addons, categories, sub-types, sections,
 * links). At entity level (drilled into a category), it signals
 * sidebar_tree.js to enable drag-and-drop on the entity tree.
 *
 * Items become directly draggable (no grip handles). Eye toggles,
 * edit/delete actions appear inline on hover.
 *
 * API: PUT /campaigns/:id/sidebar-config
 * Events: chronicle:toggle-sidebar-editor, chronicle:reorg-changed,
 *         chronicle:toggle-entity-visibility, chronicle:toggle-node-visibility
 */
(function () {
  'use strict';

  var TOUCH_THRESHOLD = 10;

  var KNOWN_ADDONS = [
    { slug: 'notes', label: 'Journal', icon: 'fa-book-open' },
    { slug: 'npcs', label: 'NPCs', icon: 'fa-users' },
    { slug: 'armory', label: 'Armory', icon: 'fa-shield-halved' }
  ];

  // State.
  var active = false;
  var level = null; // 'global' or 'entities'
  var campaignId = null;
  var config = null;
  var items = [];
  var entityTypes = [];
  var endpoint = null;

  // Touch drag state.
  var touchDrag = { src: null, ghost: null, startX: 0, startY: 0, started: false };

  // --- Helpers ---

  function isDrilled() {
    var panel = document.getElementById('sidebar-category');
    return panel && panel.classList.contains('sidebar-drill-active');
  }

  function getCampaignId() {
    if (campaignId) return campaignId;
    var el = document.querySelector('[data-campaign-id]');
    if (el) campaignId = el.dataset.campaignId;
    return campaignId;
  }

  function getEndpoint() {
    if (endpoint) return endpoint;
    var cid = getCampaignId();
    if (cid) endpoint = '/campaigns/' + cid + '/sidebar-config';
    return endpoint;
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

  // Returns true if the item is a sub-type category (has a parent_type_id).
  function isSubType(item) {
    if (item.type !== 'category') return false;
    for (var i = 0; i < entityTypes.length; i++) {
      if (entityTypes[i].id === item.type_id) return !!entityTypes[i].parent_type_id;
    }
    return false;
  }

  function injectMissing(existingItems, types) {
    var hasDashboard = false, hasAllPages = false;
    var addonSlugs = {}, categoryIds = {};

    existingItems.forEach(function (item) {
      if (item.type === 'dashboard') hasDashboard = true;
      if (item.type === 'all_pages') hasAllPages = true;
      if (item.type === 'addon') addonSlugs[item.slug] = true;
      if (item.type === 'category') categoryIds[item.type_id] = true;
    });

    if (!hasDashboard) existingItems.unshift({ type: 'dashboard', visible: true });

    KNOWN_ADDONS.forEach(function (addon) {
      if (!addonSlugs[addon.slug]) {
        existingItems.push({ type: 'addon', slug: addon.slug, label: addon.label, icon: addon.icon, visible: true });
      }
    });

    // Only add TOP-LEVEL entity types. Sub-category entity_types
    // (parent_type_id != null) are template variants of their parent, not
    // navigable collections, and must never appear in the sidebar config.
    types.forEach(function (et) {
      if (et.parent_type_id) return;
      if (!categoryIds[et.id]) {
        existingItems.push({ type: 'category', type_id: et.id, visible: true });
      }
    });

    if (!hasAllPages) existingItems.push({ type: 'all_pages', visible: true });
    return existingItems;
  }

  function generateDefaults(types) {
    var defaults = [{ type: 'dashboard', visible: true }];
    KNOWN_ADDONS.forEach(function (a) {
      defaults.push({ type: 'addon', slug: a.slug, label: a.label, icon: a.icon, visible: true });
    });
    // Only top-level types; sub-categories are template variants.
    var parents = types.filter(function (t) { return !t.parent_type_id; });
    parents.forEach(function (parent) {
      defaults.push({ type: 'category', type_id: parent.id, visible: true });
    });
    defaults.push({ type: 'all_pages', visible: true });
    return defaults;
  }

  // --- Toggle Edit Mode ---

  function toggle() {
    if (active) {
      deactivate();
    } else {
      activate();
    }
  }

  function activate() {
    active = true;
    level = isDrilled() ? 'entities' : 'global';
    document.body.classList.add('sidebar-reorg-active');
    updateButton(true);

    if (level === 'global') {
      activateGlobal();
    } else {
      activateEntities();
    }
  }

  function deactivate() {
    if (level === 'global') {
      deactivateGlobal();
    } else {
      deactivateEntities();
    }
    active = false;
    level = null;
    document.body.classList.remove('sidebar-reorg-active');
    updateButton(false);
  }

  function updateButton(isActive) {
    var btns = document.querySelectorAll('[data-sidebar-edit-toggle]');
    btns.forEach(function (btn) {
      var icon = btn.querySelector('i');
      if (isActive) {
        btn.classList.add('bg-accent/20', 'text-accent');
        btn.classList.remove('text-gray-500');
        btn.title = 'Done editing';
        if (icon) { icon.className = 'fa-solid fa-check text-[10px]'; }
      } else {
        btn.classList.remove('bg-accent/20', 'text-accent');
        btn.classList.add('text-gray-500');
        btn.title = 'Edit sidebar';
        if (icon) { icon.className = 'fa-solid fa-pencil text-[10px]'; }
      }
    });
  }

  // --- Global Level (Categories, Addons, Sections, Links) ---

  var editPanel = null;

  function activateGlobal() {
    var ep = getEndpoint();
    if (!ep) return;

    // Try to get entity types from sidebar data attribute.
    var etEl = document.querySelector('[data-sidebar-entity-types]');
    if (etEl) {
      try { entityTypes = JSON.parse(etEl.dataset.sidebarEntityTypes); } catch (e) { /* ignore */ }
    }

    Chronicle.apiFetch(ep)
      .then(function (res) { return res.ok ? res.json() : {}; })
      .then(function (data) {
        config = data || {};
        if (config.items && config.items.length > 0) {
          // Strip any persisted sub-category items from older configs — they
          // are now template variants and must not appear in the editor.
          var filtered = config.items.filter(function (item) {
            if (item.type !== 'category') return true;
            for (var i = 0; i < entityTypes.length; i++) {
              if (entityTypes[i].id === item.type_id) return !entityTypes[i].parent_type_id;
            }
            return true;
          });
          items = injectMissing(filtered, entityTypes);
        } else {
          items = generateDefaults(entityTypes);
        }
        renderEditPanel();
      })
      .catch(function () { renderEditPanel(); });
  }

  var configChanged = false;

  function deactivateGlobal() {
    if (editPanel) {
      editPanel.remove();
      editPanel = null;
    }
    // Reload page to reflect sidebar changes (order, visibility, sections).
    if (configChanged) {
      configChanged = false;
      window.location.reload();
      return;
    }
    // Restore original sidebar content if no changes were made.
    var catList = document.getElementById('sidebar-cat-list');
    if (catList) {
      Array.from(catList.children).forEach(function (child) {
        // Only unhide children that were visible before edit mode.
        if (child.dataset.editWasHidden === 'yes') {
          // Was already hidden — leave it hidden.
        } else {
          child.style.display = '';
        }
        delete child.dataset.editWasHidden;
      });
    }
  }

  function renderEditPanel() {
    // Remove existing panel if any.
    if (editPanel) editPanel.remove();

    var catList = document.getElementById('sidebar-cat-list');
    if (!catList) return;

    editPanel = document.createElement('div');
    editPanel.id = 'sidebar-edit-panel';
    editPanel.className = 'px-2 py-2 space-y-1';

    items.forEach(function (item, idx) {
      var row = createEditRow(item, idx);
      editPanel.appendChild(row);
    });

    // Add section/link buttons.
    var actions = document.createElement('div');
    actions.className = 'flex gap-2 px-1 pt-2 border-t border-gray-700/50 mt-2';
    actions.innerHTML =
      '<button type="button" class="text-[10px] text-fg-muted hover:text-accent transition-colors" data-add-section>' +
      '<i class="fa-solid fa-plus mr-1"></i>Section</button>' +
      '<button type="button" class="text-[10px] text-fg-muted hover:text-accent transition-colors" data-add-link>' +
      '<i class="fa-solid fa-plus mr-1"></i>Link</button>';

    actions.querySelector('[data-add-section]').addEventListener('click', function () {
      var label = prompt('Section label:');
      if (!label || !label.trim()) return;
      items.push({ type: 'section', id: 'sec_' + Math.random().toString(36).substr(2, 8), label: label.trim(), visible: true });
      saveConfig();
      renderEditPanel();
    });

    actions.querySelector('[data-add-link]').addEventListener('click', function () {
      var label = prompt('Link label:');
      if (!label || !label.trim()) return;
      var url = prompt('URL:');
      if (!url || !url.trim()) return;
      items.push({ type: 'link', id: 'lnk_' + Math.random().toString(36).substr(2, 8), label: label.trim(), url: url.trim(), icon: 'fa-link', visible: true });
      saveConfig();
      renderEditPanel();
    });

    editPanel.appendChild(actions);

    // Hide original sidebar content, append edit panel.
    // Track which children were already hidden so we don't unhide them on restore.
    Array.from(catList.children).forEach(function (child) {
      if (child.id !== 'sidebar-edit-panel') {
        if (!child.dataset.editWasHidden) {
          child.dataset.editWasHidden = child.style.display === 'none' ? 'yes' : 'no';
        }
        child.style.display = 'none';
      }
    });
    catList.appendChild(editPanel);
  }

  function createEditRow(item, idx) {
    var vis = item.visible !== false;
    var label = Chronicle.escapeHtml(getItemLabel(item));
    var icon = getItemIcon(item);
    var color = getItemColor(item);

    var row = document.createElement('div');
    var subType = isSubType(item);
    row.className = 'sidebar-edit-item flex items-center gap-1.5 px-2 py-1.5 rounded-md group transition-all cursor-grab' +
      (vis ? '' : ' opacity-40') +
      (subType ? ' ml-4' : '');
    row.draggable = true;
    row.dataset.editIdx = idx;

    // Nesting is derived from the entity-type parent_type_id, not a
    // per-item flag — sub-types are always rendered nested in their
    // parent's drill panel. The visual ml-4 indent above already
    // communicates the parent-child relationship in the editor.
    var iconStyle = color ? ' style="color:' + Chronicle.escapeAttr(color) + '"' : '';
    row.innerHTML =
      '<span class="w-4 h-4 flex items-center justify-center shrink-0"' + iconStyle + '>' +
      '<i class="fa-solid ' + Chronicle.escapeHtml(icon) + ' text-[10px]"></i></span>' +
      '<span class="flex-1 text-[11px] text-sidebar-text truncate">' + label + '</span>' +
      '<span class="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">' +
      (item.type === 'section' || item.type === 'link'
        ? '<button type="button" class="w-5 h-5 flex items-center justify-center rounded text-[9px] text-fg-muted hover:text-fg" data-action="edit" title="Edit">' +
          '<i class="fa-solid fa-pen"></i></button>' +
          '<button type="button" class="w-5 h-5 flex items-center justify-center rounded text-[9px] text-fg-muted hover:text-rose-400" data-action="delete" title="Remove">' +
          '<i class="fa-solid fa-trash"></i></button>'
        : '') +
      '<button type="button" class="w-5 h-5 flex items-center justify-center rounded text-[9px] text-fg-muted hover:text-fg" data-action="toggle" title="' + (vis ? 'Hide' : 'Show') + '">' +
      '<i class="fa-solid ' + (vis ? 'fa-eye' : 'fa-eye-slash') + '"></i></button>' +
      '</span>';

    // Event handlers.
    row.querySelector('[data-action="toggle"]').addEventListener('click', function (e) {
      e.stopPropagation();
      items[idx].visible = !items[idx].visible;
      saveConfig();
      renderEditPanel();
    });

    var editBtn = row.querySelector('[data-action="edit"]');
    if (editBtn) {
      editBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        editItem(idx);
      });
    }

    var delBtn = row.querySelector('[data-action="delete"]');
    if (delBtn) {
      delBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        if (!confirm('Remove "' + getItemLabel(items[idx]) + '"?')) return;
        items.splice(idx, 1);
        saveConfig();
        renderEditPanel();
      });
    }

    // Desktop drag-and-drop.
    row.addEventListener('dragstart', function (e) {
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', String(idx));
      row.classList.add('opacity-40');
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
      var fromIdx = parseInt(e.dataTransfer.getData('text/plain'), 10);
      var toIdx = idx;
      if (isNaN(fromIdx) || fromIdx === toIdx) return;
      var moved = items.splice(fromIdx, 1)[0];
      items.splice(toIdx, 0, moved);
      saveConfig();
      renderEditPanel();
    });

    row.addEventListener('dragend', function () {
      row.classList.remove('opacity-40');
      // Clean up any lingering highlight on all rows.
      if (editPanel) {
        editPanel.querySelectorAll('.sidebar-edit-item').forEach(function (r) {
          r.classList.remove('ring-1', 'ring-accent/50', 'opacity-40');
        });
      }
    });

    // Touch drag-and-drop.
    var startX = 0, startY = 0, dragging = false;
    row.addEventListener('touchstart', function (e) {
      var touch = e.touches[0];
      startX = touch.clientX;
      startY = touch.clientY;
      dragging = false;
    }, { passive: true });

    row.addEventListener('touchmove', function (e) {
      var touch = e.touches[0];
      if (!dragging && Math.abs(touch.clientY - startY) > TOUCH_THRESHOLD) {
        dragging = true;
        touchDrag.src = idx;
        touchDrag.ghost = row.cloneNode(true);
        touchDrag.ghost.style.cssText = 'position:fixed;pointer-events:none;opacity:0.7;z-index:9999;width:' + row.offsetWidth + 'px;';
        document.body.appendChild(touchDrag.ghost);
        row.classList.add('opacity-40');
      }
      if (dragging) {
        e.preventDefault();
        touchDrag.ghost.style.left = (touch.clientX - row.offsetWidth / 2) + 'px';
        touchDrag.ghost.style.top = (touch.clientY - 20) + 'px';
        // Highlight drop target.
        editPanel.querySelectorAll('.sidebar-edit-item').forEach(function (r) {
          r.classList.remove('ring-1', 'ring-accent/50');
        });
        var target = document.elementFromPoint(touch.clientX, touch.clientY);
        if (target) {
          var targetRow = target.closest('.sidebar-edit-item');
          if (targetRow && targetRow !== row) targetRow.classList.add('ring-1', 'ring-accent/50');
        }
      }
    }, { passive: false });

    row.addEventListener('touchend', function (e) {
      if (touchDrag.ghost) { touchDrag.ghost.remove(); touchDrag.ghost = null; }
      row.classList.remove('opacity-40');
      editPanel.querySelectorAll('.sidebar-edit-item').forEach(function (r) {
        r.classList.remove('ring-1', 'ring-accent/50');
      });
      if (dragging && touchDrag.src !== null) {
        var touch = e.changedTouches[0];
        var target = document.elementFromPoint(touch.clientX, touch.clientY);
        if (target) {
          var targetRow = target.closest('.sidebar-edit-item');
          if (targetRow) {
            var toIdx = parseInt(targetRow.dataset.editIdx, 10);
            if (!isNaN(toIdx) && toIdx !== touchDrag.src) {
              var moved = items.splice(touchDrag.src, 1)[0];
              items.splice(toIdx, 0, moved);
              saveConfig();
              renderEditPanel();
            }
          }
        }
      }
      touchDrag.src = null;
      dragging = false;
    });

    return row;
  }

  function editItem(idx) {
    var item = items[idx];
    var label = prompt('Label:', item.label || '');
    if (label === null) return;
    item.label = label.trim();
    if (item.type === 'link') {
      var url = prompt('URL:', item.url || '');
      if (url !== null) item.url = url.trim();
      var icon = prompt('Icon (e.g. fa-globe):', item.icon || '');
      if (icon !== null) item.icon = icon.trim();
    }
    saveConfig();
    renderEditPanel();
  }

  function saveConfig() {
    var ep = getEndpoint();
    if (!ep) return;
    configChanged = true;

    Chronicle.apiFetch(ep, {
      method: 'PUT',
      body: {
        items: items,
        hidden_entity_ids: (config && config.hidden_entity_ids) || [],
        hidden_node_ids: (config && config.hidden_node_ids) || []
      }
    }).then(function (res) {
      if (!res.ok) Chronicle.notify('Failed to save sidebar', 'error');
    }).catch(function () {
      Chronicle.notify('Failed to save sidebar', 'error');
    });
  }

  // --- Entity Level (Drilled Into Category) ---

  function activateEntities() {
    var ep = getEndpoint();
    if (!ep) return;

    // Fetch config for visibility data.
    Chronicle.apiFetch(ep)
      .then(function (res) { return res.ok ? res.json() : {}; })
      .then(function (data) {
        config = data || {};

        // Set data-reorg-active on tree element (sidebar_tree.js checks this).
        var tree = document.getElementById('sidebar-entity-tree');
        if (tree) {
          tree.setAttribute('data-reorg-active', 'true');
          document.dispatchEvent(new CustomEvent('chronicle:reorg-changed', {
            detail: {
              active: true,
              hiddenEntityIds: config.hidden_entity_ids || [],
              hiddenNodeIds: config.hidden_node_ids || []
            }
          }));
        } else {
          // Tree not loaded yet (HTMX lazy load). Wait for it.
          document.addEventListener('htmx:afterSwap', function onSwap() {
            var t = document.getElementById('sidebar-entity-tree');
            if (t && active) {
              t.setAttribute('data-reorg-active', 'true');
              setTimeout(function () {
                document.dispatchEvent(new CustomEvent('chronicle:reorg-changed', {
                  detail: {
                    active: true,
                    hiddenEntityIds: (config && config.hidden_entity_ids) || [],
                    hiddenNodeIds: (config && config.hidden_node_ids) || []
                  }
                }));
              }, 50);
              document.removeEventListener('htmx:afterSwap', onSwap);
            }
          });
        }
      });
  }

  function deactivateEntities() {
    var tree = document.getElementById('sidebar-entity-tree');
    if (tree) {
      tree.removeAttribute('data-reorg-active');
    }
    document.dispatchEvent(new CustomEvent('chronicle:reorg-changed', {
      detail: { active: false }
    }));
  }

  // --- Visibility Events from sidebar_tree.js ---

  document.addEventListener('chronicle:toggle-entity-visibility', function (e) {
    if (!config) return;
    var entityId = e.detail && e.detail.entityId;
    if (!entityId) return;

    if (!config.hidden_entity_ids) config.hidden_entity_ids = [];
    var idx = config.hidden_entity_ids.indexOf(entityId);
    if (idx >= 0) {
      config.hidden_entity_ids.splice(idx, 1);
    } else {
      config.hidden_entity_ids.push(entityId);
    }

    saveConfig();
    document.dispatchEvent(new CustomEvent('chronicle:entity-visibility-changed', {
      detail: { entityId: entityId, hidden: idx < 0 }
    }));
  });

  document.addEventListener('chronicle:toggle-node-visibility', function (e) {
    if (!config) return;
    var nodeId = e.detail && e.detail.nodeId;
    if (!nodeId) return;

    if (!config.hidden_node_ids) config.hidden_node_ids = [];
    var idx = config.hidden_node_ids.indexOf(nodeId);
    if (idx >= 0) {
      config.hidden_node_ids.splice(idx, 1);
    } else {
      config.hidden_node_ids.push(nodeId);
    }

    saveConfig();
    document.dispatchEvent(new CustomEvent('chronicle:node-visibility-changed', {
      detail: { nodeId: nodeId, hidden: idx < 0 }
    }));
  });

  // --- Drill State Observer ---
  // When user drills in/out while edit mode is active, switch levels.

  var observer = new MutationObserver(function () {
    if (!active) return;
    var newLevel = isDrilled() ? 'entities' : 'global';
    if (newLevel !== level) {
      if (level === 'global') deactivateGlobal();
      else if (level === 'entities') deactivateEntities();
      level = newLevel;
      if (level === 'global') activateGlobal();
      else activateEntities();
    }
  });

  var sidebarEl = document.getElementById('sidebar-category');
  if (sidebarEl) {
    observer.observe(sidebarEl, { attributes: true, attributeFilter: ['class'] });
  }

  // --- Event Listeners ---

  document.addEventListener('chronicle:toggle-sidebar-editor', function () {
    toggle();
  });

  // Exit edit mode on navigation.
  document.addEventListener('chronicle:navigated', function () {
    if (active) deactivate();
  });

  // Also exit on HTMX navigation.
  document.addEventListener('htmx:beforeRequest', function (e) {
    if (active && e.detail && e.detail.boosted) deactivate();
  });
})();
