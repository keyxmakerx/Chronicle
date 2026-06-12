/**
 * sidebar_reorg.js -- Sidebar Reorg Mode Controller
 *
 * Adds a toggle button in the sidebar that activates "reorg mode" for inline
 * reordering of categories or entities. Context-aware: at the category level
 * it enables drag-to-reorder for entity type icons; when drilled into a
 * category it signals sidebar_tree.js to enable entity D&D.
 *
 * Category reorder uses the existing PUT /campaigns/:id/sidebar-config API.
 * Entity reorder uses the existing PUT /campaigns/:id/entities/:eid/reorder API.
 *
 * Touch support: implements touchstart/touchmove/touchend for mobile D&D.
 */
(function () {
  'use strict';

  var TOUCH_THRESHOLD = 10; // px movement before starting a touch drag

  // State.
  var active = false;
  var level = null; // 'categories' or 'entities'
  var campaignId = null;

  // Cached sidebar config for category reordering.
  var sidebarConfig = null;
  var configEndpoint = null;

  // Escape-key cleanup ref.
  var escKeyHandler = null;

  // Debounced category-reorder save state.
  var saveTimer = null;
  // Snapshot taken at drag-start for one-level undo.
  var undoItems = null;

  // Touch drag state.
  var touchDrag = {
    src: null,
    ghost: null,
    startX: 0,
    startY: 0,
    started: false
  };

  /**
   * Detect whether the sidebar is currently drilled into a category.
   */
  function isDrilled() {
    var panel = document.getElementById('sidebar-category');
    return panel && panel.classList.contains('sidebar-drill-active');
  }

  /**
   * Get the current reorg level based on sidebar drill state.
   */
  function getCurrentLevel() {
    return isDrilled() ? 'entities' : 'categories';
  }

  /**
   * Toggle reorg mode on/off.
   */
  function toggle() {
    if (active) {
      deactivate();
    } else {
      activate();
    }
  }

  /**
   * Activate reorg mode for the current sidebar level.
   */
  function activate() {
    active = true;
    level = getCurrentLevel();
    document.body.classList.add('sidebar-reorg-active');

    var btn = document.getElementById('sidebar-reorg-toggle');
    if (btn) {
      btn.classList.add('bg-accent/20', 'text-accent');
      btn.title = 'Done reordering';
      var icon = btn.querySelector('i');
      if (icon) icon.className = 'fa-solid fa-check text-[10px]';
    }

    // Sync visual state on any secondary reorg toggle buttons (e.g. drill panel).
    document.querySelectorAll('[data-reorg-toggle]').forEach(function (b) {
      b.classList.add('bg-accent/20', 'text-accent');
      b.title = 'Done reordering';
      var i = b.querySelector('i');
      if (i) i.className = 'fa-solid fa-check text-[10px]';
    });

    // Escape exits reorg mode without saving any in-flight debounce.
    escKeyHandler = function (e) { if (e.key === 'Escape') deactivate(); };
    document.addEventListener('keydown', escKeyHandler);

    if (level === 'categories') {
      activateCategoryReorg();
    } else {
      activateEntityReorg();
    }
  }

  /**
   * Deactivate reorg mode and clean up.
   */
  function deactivate() {
    // Remove escape handler and cancel any pending debounced save.
    if (escKeyHandler) {
      document.removeEventListener('keydown', escKeyHandler);
      escKeyHandler = null;
    }
    clearTimeout(saveTimer);
    saveTimer = null;

    if (level === 'categories') {
      deactivateCategoryReorg();
    } else {
      deactivateEntityReorg();
    }

    active = false;
    level = null;
    document.body.classList.remove('sidebar-reorg-active', 'sidebar-reorg-dragging');

    var btn = document.getElementById('sidebar-reorg-toggle');
    if (btn) {
      btn.classList.remove('bg-accent/20', 'text-accent');
      btn.title = 'Reorder sidebar';
      var icon = btn.querySelector('i');
      if (icon) icon.className = 'fa-solid fa-grip-vertical text-[10px]';
    }

    // Reset visual state on any secondary reorg toggle buttons.
    document.querySelectorAll('[data-reorg-toggle]').forEach(function (b) {
      b.classList.remove('bg-accent/20', 'text-accent');
      b.title = 'Reorder pages';
      var i = b.querySelector('i');
      if (i) i.className = 'fa-solid fa-grip-vertical text-[10px]';
    });

    cleanupTouchDrag();
  }

  // -----------------------------------------------------------------------
  // Category Reorg Mode
  // -----------------------------------------------------------------------

  var catDragSrc = null;

  /**
   * Activate category reordering in the sidebar icon list.
   * Adds drag handles and visibility toggles to each category link.
   */
  function activateCategoryReorg() {
    var catList = document.getElementById('sidebar-cat-list');
    if (!catList) return;

    catList.setAttribute('data-reorg-active', 'true');

    // Determine config endpoint from campaign ID.
    var btn = document.getElementById('sidebar-reorg-toggle');
    campaignId = btn ? btn.getAttribute('data-campaign-id') : null;
    if (!campaignId) return;
    configEndpoint = '/campaigns/' + campaignId + '/sidebar-config';

    // Fetch current sidebar config.
    Chronicle.apiFetch(configEndpoint)
      .then(function (res) { return res.ok ? res.json() : null; })
      .then(function (data) {
        sidebarConfig = data || {};
        if (!sidebarConfig.items) sidebarConfig.items = [];
        if (!sidebarConfig.hidden_entity_ids) sidebarConfig.hidden_entity_ids = [];
        if (!sidebarConfig.hidden_node_ids) sidebarConfig.hidden_node_ids = [];
        renderCategoryReorgUI();
      })
      .catch(function () {
        sidebarConfig = { items: [], hidden_entity_ids: [], hidden_node_ids: [] };
        renderCategoryReorgUI();
      });
  }

  /**
   * Add drag handles and eye toggles to category links.
   */
  function renderCategoryReorgUI() {
    var links = document.querySelectorAll('#sidebar-cat-list .sidebar-category-link');
    links.forEach(function (link) {
      link.setAttribute('draggable', 'true');

      // Inert-link: suppress navigation clicks while reorg mode is active (iOS jiggle-mode semantics).
      if (!link._reorgClickInert) {
        link._reorgClickInert = function (e) { e.preventDefault(); e.stopPropagation(); };
        link.addEventListener('click', link._reorgClickInert, true);
      }

      // Add drag handle if not already present.
      if (!link.querySelector('.reorg-drag-handle')) {
        var handle = document.createElement('span');
        handle.className = 'reorg-drag-handle w-4 h-4 flex items-center justify-center shrink-0 text-gray-500 cursor-grab mr-1';
        handle.innerHTML = '<i class="fa-solid fa-grip-vertical text-[9px]"></i>';
        link.insertBefore(handle, link.firstChild);
      }

      // Add visibility toggle if not already present.
      var typeId = parseInt(link.getAttribute('data-entity-type-id') || '0', 10);
      if (typeId && !link.querySelector('.reorg-visibility-toggle')) {
        // Determine visibility from unified items model; fall back to legacy hidden_type_ids.
        var foundItem = null;
        (sidebarConfig.items || []).forEach(function (it) {
          if (it.type === 'category' && it.type_id === typeId) foundItem = it;
        });
        var isHidden = foundItem ? !foundItem.visible : (sidebarConfig.hidden_type_ids || []).indexOf(typeId) !== -1;
        var toggle = document.createElement('button');
        toggle.type = 'button';
        toggle.className = 'reorg-visibility-toggle ml-auto p-1 text-xs rounded hover:bg-white/10 transition-colors';
        toggle.setAttribute('data-type-id', String(typeId));
        toggle.title = isHidden ? 'Show in sidebar' : 'Hide from sidebar';
        toggle.innerHTML = '<i class="fa-solid ' + (isHidden ? 'fa-eye-slash text-gray-500' : 'fa-eye text-gray-400') + '"></i>';
        toggle.addEventListener('click', function (e) {
          e.preventDefault();
          e.stopPropagation();
          toggleCategoryVisibility(typeId);
        });
        link.appendChild(toggle);

        if (isHidden) {
          link.classList.add('opacity-40');
        }
      }

      // Category drag events — store refs on element for cleanup.
      link._reorgDragStart = onCatDragStart.bind(link);
      link._reorgDragOver = onCatDragOver.bind(link);
      link._reorgDragEnter = onCatDragEnter.bind(link);
      link._reorgDragLeave = onCatDragLeave.bind(link);
      link._reorgDrop = onCatDrop.bind(link);
      link._reorgDragEnd = onCatDragEnd.bind(link);
      link.addEventListener('dragstart', link._reorgDragStart);
      link.addEventListener('dragover', link._reorgDragOver);
      link.addEventListener('dragenter', link._reorgDragEnter);
      link.addEventListener('dragleave', link._reorgDragLeave);
      link.addEventListener('drop', link._reorgDrop);
      link.addEventListener('dragend', link._reorgDragEnd);

      // Touch events for mobile — store refs for cleanup.
      link._reorgTouchStart = onCatTouchStart.bind(link);
      link._reorgTouchMove = onCatTouchMove.bind(link);
      link._reorgTouchEnd = onCatTouchEnd.bind(link);
      link.addEventListener('touchstart', link._reorgTouchStart, { passive: false });
      link.addEventListener('touchmove', link._reorgTouchMove, { passive: false });
      link.addEventListener('touchend', link._reorgTouchEnd);
    });
  }

  function onCatDragStart(e) {
    catDragSrc = this;
    // Snapshot items for one-level undo before the drag mutates anything.
    undoItems = (sidebarConfig && sidebarConfig.items) ? sidebarConfig.items.slice() : null;
    this.classList.add('opacity-40', 'sidebar-reorg-drag-lifting');
    document.body.classList.add('sidebar-reorg-dragging');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', this.getAttribute('data-entity-type-id'));
    // Prevent drill navigation during drag.
    e.stopPropagation();
  }

  function onCatDragOver(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (this === catDragSrc) return;
    // Show 2-px insertion line above or below based on cursor Y position.
    var rect = this.getBoundingClientRect();
    var isTopHalf = e.clientY < rect.top + rect.height / 2;
    this.classList.toggle('sidebar-reorg-insert-before', isTopHalf);
    this.classList.toggle('sidebar-reorg-insert-after', !isTopHalf);
  }

  function onCatDragEnter(e) {
    e.preventDefault();
    if (this === catDragSrc) return;
    // Clear indicators on other rows so only the hovered row shows one.
    document.querySelectorAll('#sidebar-cat-list .sidebar-category-link').forEach(function (l) {
      if (l !== catDragSrc) {
        l.classList.remove('sidebar-reorg-insert-before', 'sidebar-reorg-insert-after');
      }
    });
  }

  function onCatDragLeave() {
    this.classList.remove('sidebar-reorg-insert-before', 'sidebar-reorg-insert-after');
  }

  function onCatDrop(e) {
    e.preventDefault();
    e.stopPropagation();

    // Clear all insertion indicators.
    document.querySelectorAll('#sidebar-cat-list .sidebar-category-link').forEach(function (l) {
      l.classList.remove('sidebar-reorg-insert-before', 'sidebar-reorg-insert-after', 'sidebar-reorg-drop-target');
    });

    if (catDragSrc && catDragSrc !== this) {
      // Insert before or after based on cursor Y half.
      var rect = this.getBoundingClientRect();
      var insertBefore = e.clientY < rect.top + rect.height / 2;
      if (insertBefore) {
        this.parentNode.insertBefore(catDragSrc, this);
      } else {
        this.parentNode.insertBefore(catDragSrc, this.nextSibling);
      }

      saveCategoryOrder();
    }
  }

  function onCatDragEnd() {
    this.classList.remove('opacity-40', 'sidebar-reorg-drag-lifting');
    document.body.classList.remove('sidebar-reorg-dragging');
    document.querySelectorAll('#sidebar-cat-list .sidebar-category-link').forEach(function (item) {
      item.classList.remove('sidebar-reorg-drop-target', 'sidebar-reorg-insert-before', 'sidebar-reorg-insert-after');
    });
  }

  /**
   * Read category order from DOM and persist via the unified items model.
   * If items is empty (legacy config), bootstraps from DOM + legacy visibility.
   * Category slots in items are reordered in-place; non-category items keep
   * their positions.
   */
  function saveCategoryOrder() {
    var domLinks = document.querySelectorAll('#sidebar-cat-list .sidebar-category-link');
    var domTypeIds = [];
    domLinks.forEach(function (el) {
      var id = parseInt(el.getAttribute('data-entity-type-id') || '0', 10);
      if (id) domTypeIds.push(id);
    });

    if (!sidebarConfig.items || !sidebarConfig.items.length) {
      // Bootstrap unified model from DOM + legacy hidden_type_ids.
      var legacyHidden = sidebarConfig.hidden_type_ids || [];
      sidebarConfig.items = domTypeIds.map(function (id) {
        return { type: 'category', type_id: id, visible: legacyHidden.indexOf(id) === -1 };
      });
    } else {
      // Reorder category slots to match DOM order, preserving non-category items.
      var byTypeId = {};
      sidebarConfig.items.forEach(function (item) {
        if (item.type === 'category' && item.type_id) byTypeId[item.type_id] = item;
      });
      var catSlots = [];
      sidebarConfig.items.forEach(function (item, i) {
        if (item.type === 'category') catSlots.push(i);
      });
      var newItems = sidebarConfig.items.slice();
      domTypeIds.forEach(function (typeId, i) {
        if (i < catSlots.length) {
          newItems[catSlots[i]] = byTypeId[typeId] || { type: 'category', type_id: typeId, visible: true };
        }
      });
      sidebarConfig.items = newItems;
    }

    scheduledCategorySave();
  }

  /**
   * Toggle visibility of a category type in the unified items model and save.
   */
  function toggleCategoryVisibility(typeId) {
    if (!sidebarConfig.items) sidebarConfig.items = [];

    var item = null;
    sidebarConfig.items.forEach(function (it) {
      if (it.type === 'category' && it.type_id === typeId) item = it;
    });
    if (item) {
      item.visible = !item.visible;
    } else {
      // Not yet in unified model — add it as hidden.
      item = { type: 'category', type_id: typeId, visible: false };
      sidebarConfig.items.push(item);
    }
    saveSidebarConfig();

    // Update UI immediately.
    var isNowHidden = !item.visible;
    var link = document.querySelector('#sidebar-cat-list .sidebar-category-link[data-entity-type-id="' + typeId + '"]');
    if (link) {
      var toggleBtn = link.querySelector('.reorg-visibility-toggle');
      if (toggleBtn) {
        toggleBtn.title = isNowHidden ? 'Show in sidebar' : 'Hide from sidebar';
        toggleBtn.innerHTML = '<i class="fa-solid ' + (isNowHidden ? 'fa-eye-slash text-gray-500' : 'fa-eye text-gray-400') + '"></i>';
      }
      if (isNowHidden) {
        link.classList.add('opacity-40');
      } else {
        link.classList.remove('opacity-40');
      }
    }
  }

  /**
   * Debounce wrapper for category reorder saves — fires one PUT 400ms after the
   * last drop, then shows the undo toast.
   */
  function scheduledCategorySave() {
    clearTimeout(saveTimer);
    var capturedUndo = undoItems;  // snapshot from drag-start
    saveTimer = setTimeout(function () {
      saveTimer = null;
      persistCategoryReorder(capturedUndo);
    }, 400);
  }

  /**
   * Persist the current items array and show the undo toast on success.
   * Only used for category reorder (not for visibility or entity toggles).
   */
  function persistCategoryReorder(prevItems) {
    if (!configEndpoint || !sidebarConfig) return;

    Chronicle.apiFetch(configEndpoint, {
      method: 'PUT',
      body: {
        items: sidebarConfig.items || [],
        hidden_entity_ids: sidebarConfig.hidden_entity_ids || [],
        hidden_node_ids: sidebarConfig.hidden_node_ids || []
      }
    })
      .then(function (res) {
        if (res.ok) {
          showUndoToast(prevItems);
        } else {
          Chronicle.notify('Failed to save sidebar order', 'error');
        }
      })
      .catch(function () {
        Chronicle.notify('Failed to save sidebar order', 'error');
      });
  }

  /**
   * Show a transient toast with an Undo button. Dismisses after 5 s.
   * Only called after a successful category reorder save.
   */
  function showUndoToast(prevItems) {
    var existing = document.getElementById('sidebar-reorg-undo-toast');
    if (existing && existing.parentNode) existing.parentNode.removeChild(existing);

    var toast = document.createElement('div');
    toast.id = 'sidebar-reorg-undo-toast';
    toast.className = 'sidebar-reorg-undo-toast';

    var msg = document.createTextNode('Sidebar order saved ');
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'sidebar-reorg-undo-btn';
    btn.textContent = 'Undo';
    toast.appendChild(msg);
    toast.appendChild(btn);
    document.body.appendChild(toast);

    var dismissed = false;
    function dismiss() {
      if (dismissed) return;
      dismissed = true;
      if (toast.parentNode) toast.parentNode.removeChild(toast);
    }

    btn.addEventListener('click', function () {
      dismiss();
      undoReorder(prevItems);
    });

    setTimeout(dismiss, 5000);
  }

  /**
   * Restore the category order to prevItems, re-render the DOM, and save immediately.
   */
  function undoReorder(prevItems) {
    if (!prevItems) return;
    undoItems = null;
    sidebarConfig.items = prevItems.slice();
    reorderCatListDOM(sidebarConfig.items);
    saveSidebarConfig();
  }

  /**
   * Re-sort the #sidebar-cat-list DOM links to match the type_id order in items.
   */
  function reorderCatListDOM(items) {
    var catList = document.getElementById('sidebar-cat-list');
    if (!catList) return;
    var typeIdOrder = [];
    (items || []).forEach(function (item) {
      if (item.type === 'category' && item.type_id) typeIdOrder.push(item.type_id);
    });
    var links = Array.from(catList.querySelectorAll('.sidebar-category-link'));
    typeIdOrder.forEach(function (typeId) {
      for (var i = 0; i < links.length; i++) {
        if (parseInt(links[i].getAttribute('data-entity-type-id') || '0', 10) === typeId) {
          catList.appendChild(links[i]);
          break;
        }
      }
    });
  }

  /**
   * Save sidebar config to server immediately (for visibility toggles and undo).
   * Sends only items + per-entity/node visibility so the server's load-merge-write
   * can preserve legacy fields (entity_type_order etc.) set by other writers.
   */
  function saveSidebarConfig() {
    if (!configEndpoint || !sidebarConfig) return;

    Chronicle.apiFetch(configEndpoint, {
      method: 'PUT',
      body: {
        items: sidebarConfig.items || [],
        hidden_entity_ids: sidebarConfig.hidden_entity_ids || [],
        hidden_node_ids: sidebarConfig.hidden_node_ids || []
      }
    })
      .then(function (res) {
        if (!res.ok) {
          Chronicle.notify('Failed to save sidebar order', 'error');
        }
      })
      .catch(function () {
        Chronicle.notify('Failed to save sidebar order', 'error');
      });
  }

  /**
   * Clean up category reorg UI.
   */
  function deactivateCategoryReorg() {
    var catList = document.getElementById('sidebar-cat-list');
    if (catList) catList.removeAttribute('data-reorg-active');

    var links = document.querySelectorAll('#sidebar-cat-list .sidebar-category-link');
    links.forEach(function (link) {
      link.removeAttribute('draggable');
      link.classList.remove('opacity-40', 'sidebar-reorg-insert-before', 'sidebar-reorg-insert-after', 'sidebar-reorg-drag-lifting');

      // Remove inert-link click suppressor.
      if (link._reorgClickInert) {
        link.removeEventListener('click', link._reorgClickInert, true);
        link._reorgClickInert = null;
      }

      // Remove drag handles and visibility toggles.
      var handle = link.querySelector('.reorg-drag-handle');
      if (handle) handle.remove();
      var toggle = link.querySelector('.reorg-visibility-toggle');
      if (toggle) toggle.remove();

      // Remove all event listeners added during activation.
      if (link._reorgDragStart) {
        link.removeEventListener('dragstart', link._reorgDragStart);
        link.removeEventListener('dragover', link._reorgDragOver);
        link.removeEventListener('dragenter', link._reorgDragEnter);
        link.removeEventListener('dragleave', link._reorgDragLeave);
        link.removeEventListener('drop', link._reorgDrop);
        link.removeEventListener('dragend', link._reorgDragEnd);
      }
      if (link._reorgTouchStart) {
        link.removeEventListener('touchstart', link._reorgTouchStart);
        link.removeEventListener('touchmove', link._reorgTouchMove);
        link.removeEventListener('touchend', link._reorgTouchEnd);
      }
    });
  }

  // -----------------------------------------------------------------------
  // Entity Reorg Mode
  // -----------------------------------------------------------------------

  // Pending HTMX listener for entity tree that hasn't loaded yet.
  var pendingTreeHandler = null;

  /**
   * Activate entity reordering (signals sidebar_tree.js via data attribute).
   * If the entity tree hasn't loaded yet (HTMX lazy load), waits for the
   * afterSwap event and retries.
   */
  function activateEntityReorg() {
    // Clean up any pending handler from a previous attempt.
    if (pendingTreeHandler) {
      document.removeEventListener('htmx:afterSwap', pendingTreeHandler);
      pendingTreeHandler = null;
    }

    // Ensure we have the campaign ID and config endpoint.
    if (!campaignId) {
      var btn = document.getElementById('sidebar-reorg-toggle');
      campaignId = btn ? btn.getAttribute('data-campaign-id') : null;
    }
    if (campaignId && !configEndpoint) {
      configEndpoint = '/campaigns/' + campaignId + '/sidebar-config';
    }

    // Fetch sidebar config so we know which entities are hidden.
    var configReady = Promise.resolve();
    if (configEndpoint && !sidebarConfig) {
      configReady = Chronicle.apiFetch(configEndpoint)
        .then(function (res) { return res.ok ? res.json() : null; })
        .then(function (data) {
          sidebarConfig = data || {};
          if (!sidebarConfig.hidden_entity_ids) sidebarConfig.hidden_entity_ids = [];
        })
        .catch(function () {
          sidebarConfig = { hidden_entity_ids: [] };
        });
    }

    configReady.then(function () {
      var tree = document.getElementById('sidebar-entity-tree');
      if (tree) {
        tree.setAttribute('data-reorg-active', 'true');
        document.dispatchEvent(new CustomEvent('chronicle:reorg-changed', {
          detail: { active: true, hiddenEntityIds: (sidebarConfig && sidebarConfig.hidden_entity_ids) || [], hiddenNodeIds: (sidebarConfig && sidebarConfig.hidden_node_ids) || [] }
        }));
        return;
      }

      // Tree not loaded yet — wait for HTMX to deliver it.
      pendingTreeHandler = function (e) {
        if (e.detail.target && (
          e.detail.target.id === 'sidebar-cat-results' ||
          e.detail.target.id === 'sidebar-cat-content'
        )) {
          document.removeEventListener('htmx:afterSwap', pendingTreeHandler);
          pendingTreeHandler = null;
          // Give sidebar_tree.js time to run initTree() first.
          setTimeout(function () {
            var tree = document.getElementById('sidebar-entity-tree');
            if (tree && active) {
              tree.setAttribute('data-reorg-active', 'true');
              document.dispatchEvent(new CustomEvent('chronicle:reorg-changed', {
                detail: { active: true, hiddenEntityIds: (sidebarConfig && sidebarConfig.hidden_entity_ids) || [], hiddenNodeIds: (sidebarConfig && sidebarConfig.hidden_node_ids) || [] }
              }));
            }
          }, 50);
        }
      };
      document.addEventListener('htmx:afterSwap', pendingTreeHandler);
    });
  }

  /**
   * Deactivate entity reordering.
   */
  function deactivateEntityReorg() {
    // Cancel any pending HTMX handler waiting for tree load.
    if (pendingTreeHandler) {
      document.removeEventListener('htmx:afterSwap', pendingTreeHandler);
      pendingTreeHandler = null;
    }

    var tree = document.getElementById('sidebar-entity-tree');
    if (tree) {
      tree.removeAttribute('data-reorg-active');
      document.dispatchEvent(new CustomEvent('chronicle:reorg-changed', {
        detail: { active: false }
      }));
    }
  }

  // -----------------------------------------------------------------------
  // Touch Drag-and-Drop (Categories)
  // -----------------------------------------------------------------------

  function onCatTouchStart(e) {
    if (!active || level !== 'categories') return;
    var touch = e.touches[0];
    touchDrag.src = this;
    touchDrag.startX = touch.clientX;
    touchDrag.startY = touch.clientY;
    touchDrag.started = false;
  }

  function onCatTouchMove(e) {
    if (!touchDrag.src) return;
    var touch = e.touches[0];
    var dx = touch.clientX - touchDrag.startX;
    var dy = touch.clientY - touchDrag.startY;

    // Start drag after threshold.
    if (!touchDrag.started) {
      if (Math.abs(dx) + Math.abs(dy) < TOUCH_THRESHOLD) return;
      touchDrag.started = true;
      e.preventDefault();

      // Create ghost element.
      touchDrag.ghost = touchDrag.src.cloneNode(true);
      touchDrag.ghost.className = 'sidebar-reorg-touch-ghost';
      touchDrag.ghost.style.position = 'fixed';
      touchDrag.ghost.style.pointerEvents = 'none';
      touchDrag.ghost.style.zIndex = '9999';
      touchDrag.ghost.style.opacity = '0.7';
      touchDrag.ghost.style.width = touchDrag.src.offsetWidth + 'px';
      document.body.appendChild(touchDrag.ghost);

      touchDrag.src.classList.add('opacity-30');
    }

    if (touchDrag.started) {
      e.preventDefault();
      touchDrag.ghost.style.left = touch.clientX + 'px';
      touchDrag.ghost.style.top = (touch.clientY - 16) + 'px';

      // Highlight drop target.
      var target = document.elementFromPoint(touch.clientX, touch.clientY);
      if (target) target = target.closest('.sidebar-category-link');

      var links = document.querySelectorAll('#sidebar-cat-list .sidebar-category-link');
      links.forEach(function (l) { l.classList.remove('sidebar-reorg-drop-target'); });
      if (target && target !== touchDrag.src) {
        target.classList.add('sidebar-reorg-drop-target');
      }
    }
  }

  function onCatTouchEnd(e) {
    if (!touchDrag.src || !touchDrag.started) {
      cleanupTouchDrag();
      return;
    }

    // Find drop target.
    var lastTouch = e.changedTouches[0];
    var target = document.elementFromPoint(lastTouch.clientX, lastTouch.clientY);
    if (target) target = target.closest('.sidebar-category-link');

    if (target && target !== touchDrag.src) {
      var parent = touchDrag.src.parentNode;
      var items = Array.from(parent.querySelectorAll('.sidebar-category-link'));
      var fromIdx = items.indexOf(touchDrag.src);
      var toIdx = items.indexOf(target);

      if (fromIdx < toIdx) {
        parent.insertBefore(touchDrag.src, target.nextSibling);
      } else {
        parent.insertBefore(touchDrag.src, target);
      }
      saveCategoryOrder();
    }

    cleanupTouchDrag();
  }

  /**
   * Clean up touch drag ghost and state.
   */
  function cleanupTouchDrag() {
    if (touchDrag.ghost && touchDrag.ghost.parentNode) {
      touchDrag.ghost.parentNode.removeChild(touchDrag.ghost);
    }
    if (touchDrag.src) {
      touchDrag.src.classList.remove('opacity-30');
    }

    var links = document.querySelectorAll('#sidebar-cat-list .sidebar-category-link');
    links.forEach(function (l) { l.classList.remove('sidebar-reorg-drop-target'); });

    touchDrag.src = null;
    touchDrag.ghost = null;
    touchDrag.started = false;
  }

  // -----------------------------------------------------------------------
  // Entity Visibility Toggle
  // -----------------------------------------------------------------------

  /**
   * Toggle sidebar visibility of an individual entity.
   * Called from sidebar_tree.js when the user clicks an entity eye toggle.
   */
  function toggleEntityVisibility(entityId) {
    if (!sidebarConfig) sidebarConfig = {};
    if (!sidebarConfig.hidden_entity_ids) sidebarConfig.hidden_entity_ids = [];

    var idx = sidebarConfig.hidden_entity_ids.indexOf(entityId);
    if (idx === -1) {
      sidebarConfig.hidden_entity_ids.push(entityId);
    } else {
      sidebarConfig.hidden_entity_ids.splice(idx, 1);
    }
    saveSidebarConfig();

    var isNowHidden = sidebarConfig.hidden_entity_ids.indexOf(entityId) !== -1;
    return isNowHidden;
  }

  /**
   * Toggle sidebar visibility of a folder node.
   * Called from sidebar_tree.js when the user clicks a folder eye toggle.
   */
  function toggleNodeVisibility(nodeId) {
    if (!sidebarConfig) sidebarConfig = {};
    if (!sidebarConfig.hidden_node_ids) sidebarConfig.hidden_node_ids = [];

    var idx = sidebarConfig.hidden_node_ids.indexOf(nodeId);
    if (idx === -1) {
      sidebarConfig.hidden_node_ids.push(nodeId);
    } else {
      sidebarConfig.hidden_node_ids.splice(idx, 1);
    }
    saveSidebarConfig();

    return sidebarConfig.hidden_node_ids.indexOf(nodeId) !== -1;
  }

  // -----------------------------------------------------------------------
  // Initialization and event binding
  // -----------------------------------------------------------------------

  function init() {
    var btn = document.getElementById('sidebar-reorg-toggle');
    if (!btn) return;

    btn.addEventListener('click', function (e) {
      e.preventDefault();
      e.stopPropagation();
      toggle();
    });

    // Allow other buttons (e.g. drill panel) to toggle reorg via custom event.
    document.addEventListener('chronicle:toggle-reorg', function () {
      toggle();
    });

    // Listen for entity visibility toggle requests from sidebar_tree.js.
    document.addEventListener('chronicle:toggle-entity-visibility', function (e) {
      if (!e.detail || !e.detail.entityId) return;
      var isNowHidden = toggleEntityVisibility(e.detail.entityId);
      document.dispatchEvent(new CustomEvent('chronicle:entity-visibility-changed', {
        detail: { entityId: e.detail.entityId, hidden: isNowHidden }
      }));
    });

    // Listen for folder node visibility toggle requests from sidebar_tree.js.
    document.addEventListener('chronicle:toggle-node-visibility', function (e) {
      if (!e.detail || !e.detail.nodeId) return;
      var isNowHidden = toggleNodeVisibility(e.detail.nodeId);
      document.dispatchEvent(new CustomEvent('chronicle:node-visibility-changed', {
        detail: { nodeId: e.detail.nodeId, hidden: isNowHidden }
      }));
    });

    // Exit reorg mode on navigation.
    window.addEventListener('chronicle:navigated', function () {
      if (active) deactivate();
    });

    // When drilling in/out, deactivate reorg so the user re-activates
    // at the new level. But only deactivate on transitions where the
    // level actually changed AND the user didn't just activate reorg.
    var observer = new MutationObserver(function (mutations) {
      if (!active) return;
      mutations.forEach(function (m) {
        if (m.attributeName === 'class') {
          var newLevel = getCurrentLevel();
          if (newLevel !== level) {
            // Switching from categories to entities (drill-in):
            // deactivate category reorg and switch to entity reorg.
            if (newLevel === 'entities' && level === 'categories') {
              deactivateCategoryReorg();
              level = 'entities';
              activateEntityReorg();
            } else if (newLevel === 'categories' && level === 'entities') {
              // Drilling out: deactivate entirely.
              deactivate();
            }
          }
        }
      });
    });

    var catPanel = document.getElementById('sidebar-category');
    if (catPanel) {
      observer.observe(catPanel, { attributes: true, attributeFilter: ['class'] });
    }
  }

  // Initialize on DOM ready.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
