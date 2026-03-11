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
    if (level === 'categories') {
      deactivateCategoryReorg();
    } else {
      deactivateEntityReorg();
    }

    active = false;
    level = null;
    document.body.classList.remove('sidebar-reorg-active');

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
        sidebarConfig = data || { entity_type_order: [], hidden_type_ids: [] };
        if (!sidebarConfig.entity_type_order) sidebarConfig.entity_type_order = [];
        if (!sidebarConfig.hidden_type_ids) sidebarConfig.hidden_type_ids = [];
        renderCategoryReorgUI();
      })
      .catch(function () {
        sidebarConfig = { entity_type_order: [], hidden_type_ids: [] };
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
        var isHidden = (sidebarConfig.hidden_type_ids || []).indexOf(typeId) !== -1;
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
    this.classList.add('opacity-40');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', this.getAttribute('data-entity-type-id'));
    // Prevent drill navigation during drag.
    e.stopPropagation();
  }

  function onCatDragOver(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
  }

  function onCatDragEnter(e) {
    e.preventDefault();
    if (this !== catDragSrc) {
      this.classList.add('sidebar-reorg-drop-target');
    }
  }

  function onCatDragLeave() {
    this.classList.remove('sidebar-reorg-drop-target');
  }

  function onCatDrop(e) {
    e.preventDefault();
    e.stopPropagation();
    this.classList.remove('sidebar-reorg-drop-target');

    if (catDragSrc && catDragSrc !== this) {
      // Reorder in DOM.
      var parent = catDragSrc.parentNode;
      var items = Array.from(parent.querySelectorAll('.sidebar-category-link'));
      var fromIdx = items.indexOf(catDragSrc);
      var toIdx = items.indexOf(this);

      if (fromIdx < toIdx) {
        parent.insertBefore(catDragSrc, this.nextSibling);
      } else {
        parent.insertBefore(catDragSrc, this);
      }

      saveCategoryOrder();
    }
  }

  function onCatDragEnd() {
    this.classList.remove('opacity-40');
    var items = document.querySelectorAll('#sidebar-cat-list .sidebar-category-link');
    items.forEach(function (item) {
      item.classList.remove('sidebar-reorg-drop-target');
    });
  }

  /**
   * Read category order from DOM and save to API.
   */
  function saveCategoryOrder() {
    var items = document.querySelectorAll('#sidebar-cat-list .sidebar-category-link');
    var order = [];
    items.forEach(function (item) {
      var id = parseInt(item.getAttribute('data-entity-type-id') || '0', 10);
      if (id) order.push(id);
    });

    sidebarConfig.entity_type_order = order;
    saveSidebarConfig();
  }

  /**
   * Toggle visibility of a category and save.
   */
  function toggleCategoryVisibility(typeId) {
    var idx = (sidebarConfig.hidden_type_ids || []).indexOf(typeId);
    if (idx === -1) {
      sidebarConfig.hidden_type_ids.push(typeId);
    } else {
      sidebarConfig.hidden_type_ids.splice(idx, 1);
    }
    saveSidebarConfig();

    // Update UI immediately.
    var link = document.querySelector('#sidebar-cat-list .sidebar-category-link[data-entity-type-id="' + typeId + '"]');
    if (link) {
      var isNowHidden = sidebarConfig.hidden_type_ids.indexOf(typeId) !== -1;
      var toggle = link.querySelector('.reorg-visibility-toggle');
      if (toggle) {
        toggle.title = isNowHidden ? 'Show in sidebar' : 'Hide from sidebar';
        toggle.innerHTML = '<i class="fa-solid ' + (isNowHidden ? 'fa-eye-slash text-gray-500' : 'fa-eye text-gray-400') + '"></i>';
      }
      if (isNowHidden) {
        link.classList.add('opacity-40');
      } else {
        link.classList.remove('opacity-40');
      }
    }
  }

  /**
   * Save sidebar config to server.
   */
  function saveSidebarConfig() {
    if (!configEndpoint || !sidebarConfig) return;

    Chronicle.apiFetch(configEndpoint, {
      method: 'PUT',
      body: {
        entity_type_order: sidebarConfig.entity_type_order || [],
        hidden_type_ids: sidebarConfig.hidden_type_ids || [],
        custom_sections: sidebarConfig.custom_sections || [],
        custom_links: sidebarConfig.custom_links || []
      }
    })
      .then(function (res) {
        if (res.ok) {
          Chronicle.notify('Sidebar order saved', 'success');
        } else {
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
      link.classList.remove('opacity-40');

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

  /**
   * Activate entity reordering (signals sidebar_tree.js via data attribute).
   */
  function activateEntityReorg() {
    var tree = document.getElementById('sidebar-entity-tree');
    if (tree) {
      tree.setAttribute('data-reorg-active', 'true');
      // Dispatch custom event for sidebar_tree.js to pick up.
      document.dispatchEvent(new CustomEvent('chronicle:reorg-changed', {
        detail: { active: true }
      }));
    }
  }

  /**
   * Deactivate entity reordering.
   */
  function deactivateEntityReorg() {
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

    // Exit reorg mode on navigation.
    window.addEventListener('chronicle:navigated', function () {
      if (active) deactivate();
    });

    // Exit reorg mode when drilling in/out changes context.
    var observer = new MutationObserver(function (mutations) {
      if (!active) return;
      mutations.forEach(function (m) {
        if (m.attributeName === 'class') {
          var newLevel = getCurrentLevel();
          if (newLevel !== level) {
            deactivate();
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
