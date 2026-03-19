/**
 * sidebar_tree.js -- Collapsible Tree + Drag-and-Drop for Sidebar Entity List
 *
 * Transforms the flat entity list rendered by SidebarEntityList into a
 * collapsible tree using data-parent-id attributes. Supports:
 *   - Collapsible folders (entities with children get folder icons)
 *   - Leaf nodes get page icons
 *   - Vertical guide lines for visual hierarchy
 *   - Smooth CSS transitions for collapse/expand
 *   - Drag-and-drop reordering within the same level
 *   - Drag-and-drop reparenting (drop onto an entity to nest it)
 *   - Visual feedback distinguishing reorder vs reparent operations
 *   - Collapse state persisted in localStorage per campaign
 *
 * Listens for HTMX afterSwap events on #sidebar-cat-results to re-initialize
 * whenever the entity list is refreshed.
 */
(function () {
  'use strict';

  var INDENT_PX = 14;
  var STORAGE_KEY_PREFIX = 'chronicle-tree-collapsed-';

  // Track the current container for the reorg-changed listener (IIFE-scoped
  // so the listener is registered once, not per initTree call).
  var currentTreeContainer = null;

  /**
   * Initialize the tree for a freshly loaded entity list.
   * Reads the flat list of .sidebar-tree-item links, builds a parent-child
   * graph, and re-renders them as a nested tree with toggle buttons and icons.
   */
  function initTree() {
    var container = document.getElementById('sidebar-entity-tree');
    if (!container) return;

    var campaignId = container.getAttribute('data-campaign-id') || '';
    var items = container.querySelectorAll('.sidebar-tree-item');
    if (!items.length) return;

    // Build lookup: entityId -> { el, parentId, sortOrder, children[] }
    var nodes = {};
    var rootIds = [];

    items.forEach(function (el) {
      var id = el.getAttribute('data-entity-id');
      var parentId = el.getAttribute('data-parent-id') || null;
      var sortOrder = parseInt(el.getAttribute('data-sort-order') || '0', 10);

      nodes[id] = {
        el: el,
        id: id,
        parentId: parentId,
        sortOrder: sortOrder,
        children: []
      };
    });

    // Build tree relationships — link children to their parents.
    // If a node's parent isn't in the current list (e.g. different entity type),
    // clear the stale parentId so it becomes a true root node. This prevents
    // reorder operations from accidentally reparenting to an invisible entity.
    Object.keys(nodes).forEach(function (id) {
      var node = nodes[id];
      if (node.parentId && nodes[node.parentId]) {
        nodes[node.parentId].children.push(node);
      } else {
        node.parentId = null;
        rootIds.push(id);
      }
    });

    // Sort children by sort_order, then alphabetically by name.
    function sortChildren(childNodes) {
      childNodes.sort(function (a, b) {
        if (a.sortOrder !== b.sortOrder) return a.sortOrder - b.sortOrder;
        var nameA = (a.el.getAttribute('data-entity-name') || '').toLowerCase();
        var nameB = (b.el.getAttribute('data-entity-name') || '').toLowerCase();
        return nameA < nameB ? -1 : nameA > nameB ? 1 : 0;
      });
    }

    // Load collapsed state from localStorage.
    var storageKey = STORAGE_KEY_PREFIX + campaignId;
    var collapsedSet = {};
    try {
      var stored = localStorage.getItem(storageKey);
      if (stored) collapsedSet = JSON.parse(stored);
    } catch (e) { /* ignore corrupt localStorage */ }

    function saveCollapsed() {
      try {
        localStorage.setItem(storageKey, JSON.stringify(collapsedSet));
      } catch (e) { /* ignore quota errors */ }
    }

    // Clear container and re-render as tree.
    container.innerHTML = '';

    /**
     * Render a single tree node: clones the link element, adds indent,
     * toggle button (for parents) or spacer (for nested leaves), and
     * swaps the icon to folder/page as appropriate.
     */
    function renderNode(node, depth) {
      var hasChildren = node.children.length > 0;
      var isFolder = node.el.hasAttribute('data-is-folder');
      var showAsFolder = hasChildren || isFolder;
      var isCollapsed = !!collapsedSet[node.id];

      // Create wrapper div for the tree node.
      var wrapper = document.createElement('div');
      wrapper.className = 'sidebar-tree-node';
      wrapper.setAttribute('data-entity-id', node.id);
      if (node.parentId) wrapper.setAttribute('data-parent-id', node.parentId);
      wrapper.setAttribute('data-depth', String(depth));
      // Propagate sidebar-hidden flag to wrapper for reorg script.
      if (node.el.hasAttribute('data-sidebar-hidden')) {
        wrapper.setAttribute('data-sidebar-hidden', 'true');
      }

      // Clone the original link element and apply indentation.
      var link = node.el.cloneNode(true);
      link.style.paddingLeft = (10 + depth * INDENT_PX) + 'px';

      // Folder entities are organizational — not navigable as pages.
      // Replace the <a> with a <div> so clicking doesn't navigate away.
      if (isFolder) {
        var div = document.createElement('div');
        div.className = link.className;
        div.style.cssText = link.style.cssText;
        div.innerHTML = link.innerHTML;
        // Copy data attributes needed by drag-and-drop.
        ['data-entity-id', 'data-parent-id', 'data-sort-order', 'data-entity-name', 'data-sidebar-hidden', 'data-is-folder'].forEach(function (attr) {
          if (link.hasAttribute(attr)) div.setAttribute(attr, link.getAttribute(attr));
        });
        div.classList.add('sidebar-tree-item', 'cursor-default');
        link = div;
      }

      // Swap the default page icon based on whether this node is a folder.
      var iconEl = link.querySelector('.sidebar-tree-icon i');
      if (iconEl) {
        if (showAsFolder) {
          // Folder icon: open when expanded, closed when collapsed.
          iconEl.className = isCollapsed
            ? 'fa-solid fa-folder text-[10px]'
            : 'fa-solid fa-folder-open text-[10px]';
          link.setAttribute('data-has-children', 'true');
        }
        // Leaves keep the default fa-file-lines icon from the template.
      }

      // Add toggle button (chevron) for items with children.
      if (hasChildren) {
        var toggle = document.createElement('span');
        toggle.className = 'sidebar-tree-toggle inline-flex items-center justify-center w-4 h-4 cursor-pointer text-gray-500 hover:text-gray-300 shrink-0';
        toggle.setAttribute('data-collapsed', String(isCollapsed));
        // Use a single right-chevron that rotates via CSS transform.
        toggle.innerHTML = '<i class="fa-solid fa-chevron-right text-[7px]"></i>';
        if (!isCollapsed) {
          toggle.style.transform = 'rotate(90deg)';
        }

        toggle.addEventListener('click', function (e) {
          e.preventDefault();
          e.stopPropagation();
          isCollapsed = !isCollapsed;

          // Update collapsed state and persist.
          if (isCollapsed) {
            collapsedSet[node.id] = true;
          } else {
            delete collapsedSet[node.id];
          }
          saveCollapsed();

          // Rotate toggle chevron.
          toggle.style.transform = isCollapsed ? 'rotate(0deg)' : 'rotate(90deg)';
          toggle.setAttribute('data-collapsed', String(isCollapsed));

          // Swap folder icon between open/closed.
          var folderIcon = link.querySelector('.sidebar-tree-icon i');
          if (folderIcon) {
            folderIcon.className = isCollapsed
              ? 'fa-solid fa-folder text-[10px]'
              : 'fa-solid fa-folder-open text-[10px]';
          }

          // Animate children container collapse/expand.
          var childContainer = wrapper.querySelector('.sidebar-tree-children');
          if (childContainer) {
            if (isCollapsed) {
              // Collapse: set explicit height then animate to 0.
              childContainer.style.maxHeight = childContainer.scrollHeight + 'px';
              // Force reflow so the browser registers the starting height.
              childContainer.offsetHeight; // eslint-disable-line no-unused-expressions
              childContainer.setAttribute('data-collapsed', 'true');
              childContainer.style.maxHeight = '0';
            } else {
              // Expand: animate from 0 to scrollHeight, then remove max-height.
              childContainer.setAttribute('data-collapsed', 'false');
              childContainer.style.maxHeight = childContainer.scrollHeight + 'px';
              setTimeout(function () {
                childContainer.style.maxHeight = '';
              }, 220);
            }
          }
        });

        // Insert toggle before the icon span.
        var iconSpan = link.querySelector('.sidebar-tree-icon');
        if (iconSpan) {
          link.insertBefore(toggle, iconSpan);
        } else {
          link.insertBefore(toggle, link.firstChild);
        }
      } else if (depth > 0) {
        // Leaf nodes at depth > 0 get a spacer to align with toggled siblings.
        var spacer = document.createElement('span');
        spacer.className = 'inline-block w-4 shrink-0';
        var iconSpan2 = link.querySelector('.sidebar-tree-icon');
        if (iconSpan2) {
          link.insertBefore(spacer, iconSpan2);
        } else {
          link.insertBefore(spacer, link.firstChild);
        }
      }

      // Set guide line position for nested nodes via inline style.
      if (depth > 0) {
        wrapper.style.setProperty('--guide-left', (11 + (depth - 1) * INDENT_PX) + 'px');
        // Use the CSS ::before pseudo-element positioned by --guide-left.
        wrapper.style.position = 'relative';
      }

      wrapper.appendChild(link);

      // Render children into a collapsible container.
      if (hasChildren) {
        sortChildren(node.children);
        var childContainer = document.createElement('div');
        childContainer.className = 'sidebar-tree-children';
        if (isCollapsed) {
          childContainer.style.maxHeight = '0';
          childContainer.setAttribute('data-collapsed', 'true');
        }
        node.children.forEach(function (child) {
          renderNode(child, depth + 1);
          childContainer.appendChild(child._wrapper);
        });
        wrapper.appendChild(childContainer);
      }

      node._wrapper = wrapper;
      container.appendChild(wrapper);
    }

    // Sort and render root nodes.
    var roots = rootIds.map(function (id) { return nodes[id]; });
    sortChildren(roots);
    roots.forEach(function (node) {
      renderNode(node, 0);
    });

    // --- Drag and Drop ---
    setupDragAndDrop(container, campaignId);

    // Preserve reorg state across HTMX tree re-inits. If reorg mode is
    // still active (body class set by sidebar_reorg.js), re-apply the
    // data attribute and drag handles on the newly created container.
    // This handles both the case where the old container had the attribute
    // and when the tree was replaced by an HTMX swap during reorg mode.
    if (container.hasAttribute('data-reorg-active') || document.body.classList.contains('sidebar-reorg-active')) {
      container.setAttribute('data-reorg-active', 'true');
      updateDraggable(container, true);
    }
  }

  /**
   * Check if entity reorg mode is active.
   */
  function isReorgActive(container) {
    return container.hasAttribute('data-reorg-active');
  }

  /**
   * Toggle draggable state on tree items based on reorg mode.
   * When enabled, also adds entity visibility toggles (eye icons).
   */
  function updateDraggable(container, enabled, hiddenEntityIds) {
    var hiddenSet = {};
    if (hiddenEntityIds) {
      hiddenEntityIds.forEach(function (id) { hiddenSet[id] = true; });
    }

    var nodes = container.querySelectorAll('.sidebar-tree-node');
    nodes.forEach(function (node) {
      var item = node.querySelector('.sidebar-tree-item');
      if (item) {
        var entityId = node.getAttribute('data-entity-id');
        if (enabled) {
          item.setAttribute('draggable', 'true');
          // Add drag handle if not present.
          if (!node.querySelector('.reorg-drag-handle')) {
            var handle = document.createElement('span');
            handle.className = 'reorg-drag-handle w-3 h-3 flex items-center justify-center shrink-0 text-gray-500 cursor-grab mr-1';
            handle.innerHTML = '<i class="fa-solid fa-grip-vertical text-[8px]"></i>';
            item.insertBefore(handle, item.firstChild);
          }
          // Add entity visibility toggle if not present.
          if (entityId && !node.querySelector('.reorg-entity-visibility')) {
            var isHidden = !!hiddenSet[entityId] || node.hasAttribute('data-sidebar-hidden');
            var eyeBtn = document.createElement('button');
            eyeBtn.type = 'button';
            eyeBtn.className = 'reorg-entity-visibility ml-auto p-0.5 text-[10px] rounded hover:bg-white/10 transition-colors shrink-0';
            eyeBtn.setAttribute('data-entity-id', entityId);
            eyeBtn.title = isHidden ? 'Show in sidebar' : 'Hide from sidebar';
            eyeBtn.innerHTML = '<i class="fa-solid ' + (isHidden ? 'fa-eye-slash text-gray-500' : 'fa-eye text-gray-400') + '"></i>';
            eyeBtn.addEventListener('click', function (e) {
              e.preventDefault();
              e.stopPropagation();
              document.dispatchEvent(new CustomEvent('chronicle:toggle-entity-visibility', {
                detail: { entityId: entityId }
              }));
            });
            item.appendChild(eyeBtn);
          }
        } else {
          item.removeAttribute('draggable');
          var handle = node.querySelector('.reorg-drag-handle');
          if (handle) handle.remove();
          var eyeToggle = node.querySelector('.reorg-entity-visibility');
          if (eyeToggle) eyeToggle.remove();
        }
      }
    });
  }

  /**
   * Setup drag-and-drop for reordering and reparenting.
   *
   * Drop zones are determined by mouse position relative to the target node:
   *   - Top third: reorder (insert before target, same parent)
   *   - Bottom two-thirds: reparent (nest inside target)
   *
   * Visual feedback differs: reorder shows an indigo line between items,
   * reparent highlights the target with a left-border accent.
   *
   * Drag events only fire when reorg mode is active (data-reorg-active).
   */
  function setupDragAndDrop(container, campaignId) {
    var dragSrcId = null;
    var dropIndicator = null;

    // Create drop indicator line element (reused across drag operations).
    dropIndicator = document.createElement('div');
    dropIndicator.className = 'sidebar-drop-indicator';
    dropIndicator.style.display = 'none';

    // Store container reference for the IIFE-scoped reorg listener.
    currentTreeContainer = container;

    // Check initial state (in case reorg was active before tree init).
    if (isReorgActive(container)) {
      updateDraggable(container, true);
    }

    container.addEventListener('dragstart', function (e) {
      if (!isReorgActive(container)) return;
      var item = e.target.closest('.sidebar-tree-item');
      if (!item) return;
      dragSrcId = item.getAttribute('data-entity-id');
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', dragSrcId);
      // Fade the source item to indicate it's being dragged.
      setTimeout(function () { item.style.opacity = '0.35'; }, 0);
    });

    container.addEventListener('dragend', function (e) {
      var item = e.target.closest('.sidebar-tree-item');
      if (item) item.style.opacity = '';
      dragSrcId = null;
      hideDropIndicator();
      clearDropTargets(container);
    });

    container.addEventListener('dragover', function (e) {
      if (!isReorgActive(container)) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';

      var target = e.target.closest('.sidebar-tree-node');
      if (!target) return;

      var targetId = target.getAttribute('data-entity-id');
      if (targetId === dragSrcId) return;

      clearDropTargets(container);

      // Determine drop position: top third = reorder, bottom two-thirds = reparent.
      var rect = target.getBoundingClientRect();
      var thirdY = rect.top + rect.height / 3;

      if (e.clientY < thirdY) {
        // Reorder: insert before — show indicator line above target.
        showDropIndicator(target, 'before');
      } else {
        // Reparent: nest inside — highlight target with accent border.
        target.classList.add('sidebar-drop-reparent');
      }
    });

    container.addEventListener('dragleave', function (e) {
      var target = e.target.closest('.sidebar-tree-node');
      if (target) {
        target.classList.remove('sidebar-drop-target');
        target.classList.remove('sidebar-drop-reparent');
      }
    });

    container.addEventListener('drop', function (e) {
      if (!isReorgActive(container)) return;
      e.preventDefault();
      hideDropIndicator();
      clearDropTargets(container);

      var droppedId = e.dataTransfer.getData('text/plain');
      if (!droppedId) return;

      var target = e.target.closest('.sidebar-tree-node');
      if (!target) return;

      var targetId = target.getAttribute('data-entity-id');
      if (targetId === droppedId) return;

      var rect = target.getBoundingClientRect();
      var thirdY = rect.top + rect.height / 3;

      if (e.clientY < thirdY) {
        // Reorder: place before target (same parent).
        var targetParentId = target.getAttribute('data-parent-id') || null;
        var sortOrder = calculateSortOrder(target, 'before');
        reorderEntity(campaignId, droppedId, targetParentId, sortOrder);
      } else {
        // Reparent zone: behavior depends on whether target is a folder.
        var targetHasChildren = !!target.querySelector('[data-has-children]');
        if (targetHasChildren) {
          // Target is already a folder — add dragged entity as child.
          reorderEntity(campaignId, droppedId, targetId, 0);
        } else {
          // Target is a leaf — show menu to create a new folder
          // containing both the dragged and target entities.
          showReparentMenu(e.clientX, e.clientY, droppedId, target, campaignId);
        }
      }
    });

    function showDropIndicator(targetNode, position) {
      if (position === 'before') {
        targetNode.parentNode.insertBefore(dropIndicator, targetNode);
        dropIndicator.style.display = 'block';
      }
    }

    function hideDropIndicator() {
      dropIndicator.style.display = 'none';
      if (dropIndicator.parentNode) {
        dropIndicator.parentNode.removeChild(dropIndicator);
      }
    }

    function clearDropTargets(el) {
      var targets = el.querySelectorAll('.sidebar-drop-target, .sidebar-drop-reparent');
      for (var i = 0; i < targets.length; i++) {
        targets[i].classList.remove('sidebar-drop-target');
        targets[i].classList.remove('sidebar-drop-reparent');
      }
    }

    // --- Touch Drag-and-Drop for mobile entity reordering ---
    var TOUCH_THRESHOLD = 10;
    var touchState = { src: null, srcId: null, ghost: null, startX: 0, startY: 0, started: false };

    container.addEventListener('touchstart', function (e) {
      if (!isReorgActive(container)) return;
      var item = e.target.closest('.sidebar-tree-item');
      if (!item) return;
      var touch = e.touches[0];
      touchState.src = item.closest('.sidebar-tree-node');
      touchState.srcId = item.getAttribute('data-entity-id');
      touchState.startX = touch.clientX;
      touchState.startY = touch.clientY;
      touchState.started = false;
    }, { passive: false });

    container.addEventListener('touchmove', function (e) {
      if (!touchState.src) return;
      var touch = e.touches[0];
      var dx = touch.clientX - touchState.startX;
      var dy = touch.clientY - touchState.startY;

      if (!touchState.started) {
        if (Math.abs(dx) + Math.abs(dy) < TOUCH_THRESHOLD) return;
        touchState.started = true;
        e.preventDefault();
        touchState.ghost = touchState.src.cloneNode(true);
        touchState.ghost.style.cssText = 'position:fixed;pointer-events:none;z-index:9999;opacity:0.7;width:' + touchState.src.offsetWidth + 'px';
        document.body.appendChild(touchState.ghost);
        touchState.src.style.opacity = '0.3';
      }

      if (touchState.started) {
        e.preventDefault();
        touchState.ghost.style.left = touch.clientX + 'px';
        touchState.ghost.style.top = (touch.clientY - 16) + 'px';

        clearDropTargets(container);
        hideDropIndicator();
        var el = document.elementFromPoint(touch.clientX, touch.clientY);
        var target = el ? el.closest('.sidebar-tree-node') : null;
        if (target && target !== touchState.src) {
          var rect = target.getBoundingClientRect();
          var thirdY = rect.top + rect.height / 3;
          if (touch.clientY < thirdY) {
            showDropIndicator(target, 'before');
          } else {
            target.classList.add('sidebar-drop-reparent');
          }
        }
      }
    }, { passive: false });

    container.addEventListener('touchend', function (e) {
      if (!touchState.src || !touchState.started) {
        touchState.src = null;
        return;
      }

      hideDropIndicator();
      clearDropTargets(container);

      var lastTouch = e.changedTouches[0];
      var el = document.elementFromPoint(lastTouch.clientX, lastTouch.clientY);
      var target = el ? el.closest('.sidebar-tree-node') : null;

      if (target && target !== touchState.src) {
        var targetId = target.getAttribute('data-entity-id');
        var rect = target.getBoundingClientRect();
        var thirdY = rect.top + rect.height / 3;

        if (lastTouch.clientY < thirdY) {
          var targetParentId = target.getAttribute('data-parent-id') || null;
          var sortOrder = calculateSortOrder(target, 'before');
          reorderEntity(campaignId, touchState.srcId, targetParentId, sortOrder);
        } else {
          var targetHasChildren = !!target.querySelector('[data-has-children]');
          if (targetHasChildren) {
            reorderEntity(campaignId, touchState.srcId, targetId, 0);
          } else {
            showReparentMenu(lastTouch.clientX, lastTouch.clientY, touchState.srcId, target, campaignId);
          }
        }
      }

      // Cleanup.
      if (touchState.ghost && touchState.ghost.parentNode) {
        touchState.ghost.parentNode.removeChild(touchState.ghost);
      }
      if (touchState.src) touchState.src.style.opacity = '';
      touchState.src = null;
      touchState.ghost = null;
      touchState.started = false;
    });
  }

  /**
   * Calculate the sort order for an entity being dropped relative to a target.
   * Looks at sibling sort_order values to place the entity in the right position.
   * If there's no room between siblings, returns the target's order (server
   * will re-normalize the sequence on save).
   */
  function calculateSortOrder(targetNode, position) {
    var siblings = targetNode.parentNode.querySelectorAll(':scope > .sidebar-tree-node');
    var targetIdx = -1;
    for (var i = 0; i < siblings.length; i++) {
      if (siblings[i] === targetNode) { targetIdx = i; break; }
    }

    if (position === 'before') {
      if (targetIdx === 0) {
        // Placing before the first sibling: use target's order - 1 (min 0).
        var targetEl = targetNode.querySelector('.sidebar-tree-item');
        var targetOrder = parseInt(targetEl ? targetEl.getAttribute('data-sort-order') : '0', 10) || 0;
        return Math.max(0, targetOrder - 1);
      }
      // Place between previous sibling and target.
      var prevItem = siblings[targetIdx - 1].querySelector('.sidebar-tree-item');
      var targetItem = targetNode.querySelector('.sidebar-tree-item');
      var prevOrder = parseInt(prevItem ? prevItem.getAttribute('data-sort-order') : '0', 10) || 0;
      var targetOrder2 = parseInt(targetItem ? targetItem.getAttribute('data-sort-order') : '0', 10) || 0;
      // Use midpoint if there's room, otherwise server re-normalizes.
      if (targetOrder2 > prevOrder + 1) {
        return Math.floor((prevOrder + targetOrder2) / 2);
      }
      return targetOrder2;
    }
    return 0;
  }

  /**
   * Refresh the sidebar entity list via HTMX to reflect updated hierarchy.
   */
  function refreshSidebarTree() {
    var results = document.getElementById('sidebar-cat-results');
    if (results) {
      var searchInput = document.querySelector('#sidebar-cat-content input[name="q"]');
      if (searchInput) {
        var loadUrl = searchInput.getAttribute('hx-get');
        if (loadUrl) {
          htmx.ajax('GET', loadUrl, { target: results, swap: 'innerHTML' });
        }
      }
    }
  }

  /**
   * Send reorder/reparent request to the API. On success, refreshes the
   * sidebar entity list via HTMX to reflect the new ordering.
   */
  function reorderEntity(campaignId, entityId, newParentId, sortOrder) {
    var body = {
      parent_id: newParentId || null,
      sort_order: sortOrder
    };

    Chronicle.apiFetch('/campaigns/' + campaignId + '/entities/' + entityId + '/reorder', {
      method: 'PUT',
      body: body
    })
    .then(function (resp) {
      if (!resp.ok) {
        return resp.text().then(function (body) {
          console.error('sidebar_tree: reorder failed', resp.status, body);
          throw new Error('Reorder failed: ' + resp.status);
        });
      }
      refreshSidebarTree();
    })
    .catch(function (err) {
      console.error('sidebar_tree: reorder failed', err);
    });
  }

  // -----------------------------------------------------------------------
  // Reparent Menu — shown when dropping a leaf onto another leaf.
  // Instead of making the target the parent, creates a new folder entity
  // and groups both items under it.
  // -----------------------------------------------------------------------

  /**
   * Remove any existing reparent context menu from the DOM.
   */
  function removeReparentMenu() {
    var existing = document.getElementById('sidebar-reparent-menu');
    if (existing) existing.remove();
    document.removeEventListener('mousedown', onReparentMenuOutsideClick);
  }

  /**
   * Dismiss handler: close menu when clicking outside it.
   */
  function onReparentMenuOutsideClick(e) {
    var menu = document.getElementById('sidebar-reparent-menu');
    if (menu && !menu.contains(e.target)) {
      removeReparentMenu();
    }
  }

  /**
   * Show a context menu with options for grouping two entities into a folder.
   * Appears at the drop position when a leaf entity is dropped onto another
   * leaf entity. The original target entity is never made the folder itself —
   * a new folder is created to contain both.
   */
  function showReparentMenu(x, y, droppedId, targetNode, campaignId) {
    removeReparentMenu();

    var menu = document.createElement('div');
    menu.id = 'sidebar-reparent-menu';
    menu.className = 'fixed z-[9999] bg-surface border border-border rounded-lg shadow-xl py-1 min-w-[180px] text-sm';

    // Clamp position to stay within viewport.
    var menuW = 190;
    var menuH = 80;
    var posX = Math.min(x, window.innerWidth - menuW - 8);
    var posY = Math.min(y, window.innerHeight - menuH - 8);
    menu.style.left = posX + 'px';
    menu.style.top = posY + 'px';

    // Option 1: New page as folder (a real entity with content).
    var opt1 = document.createElement('button');
    opt1.type = 'button';
    opt1.className = 'w-full px-3 py-2 text-left flex items-center gap-2 text-fg hover:bg-surface-hover transition-colors';
    opt1.innerHTML = '<i class="fa-solid fa-file-lines text-xs text-fg-muted w-4 text-center"></i> New page as folder';
    opt1.addEventListener('click', function () {
      removeReparentMenu();
      createGroupFolder(campaignId, droppedId, targetNode, 'New Page');
    });

    // Option 2: Empty folder (organizational only).
    var opt2 = document.createElement('button');
    opt2.type = 'button';
    opt2.className = 'w-full px-3 py-2 text-left flex items-center gap-2 text-fg hover:bg-surface-hover transition-colors';
    opt2.innerHTML = '<i class="fa-solid fa-folder text-xs text-fg-muted w-4 text-center"></i> New empty folder';
    opt2.addEventListener('click', function () {
      removeReparentMenu();
      createGroupFolder(campaignId, droppedId, targetNode, 'New Folder', true);
    });

    menu.appendChild(opt1);
    menu.appendChild(opt2);
    document.body.appendChild(menu);

    // Close on outside click (delayed to avoid the current event).
    setTimeout(function () {
      document.addEventListener('mousedown', onReparentMenuOutsideClick);
    }, 0);
  }

  /**
   * Create a new folder entity and group both the dragged and target
   * entities under it. The folder is placed where the target was
   * (same parent, same sort order), then both items are reparented.
   *
   * Flow: create folder → position folder → reparent target → reparent dragged → refresh.
   */
  function createGroupFolder(campaignId, droppedId, targetNode, folderName, isFolder) {
    var targetId = targetNode.getAttribute('data-entity-id');
    var targetParentId = targetNode.getAttribute('data-parent-id') || null;
    var targetItem = targetNode.querySelector('.sidebar-tree-item');
    var targetSortOrder = parseInt(targetItem ? targetItem.getAttribute('data-sort-order') : '0', 10) || 0;

    // Read entity type from the tree container.
    var tree = document.getElementById('sidebar-entity-tree');
    var entityTypeId = parseInt(tree ? tree.getAttribute('data-entity-type-id') : '0', 10) || 0;

    // Step 1: Create the folder entity.
    Chronicle.apiFetch('/campaigns/' + campaignId + '/entities/quick-create', {
      method: 'POST',
      body: { name: folderName, entity_type_id: entityTypeId, is_folder: !!isFolder }
    })
    .then(function (res) {
      if (!res.ok) throw new Error('Failed to create folder');
      return res.json();
    })
    .then(function (folder) {
      // Step 2: Position the folder where the target was.
      return Chronicle.apiFetch('/campaigns/' + campaignId + '/entities/' + folder.id + '/reorder', {
        method: 'PUT',
        body: { parent_id: targetParentId, sort_order: targetSortOrder }
      }).then(function (res) {
        if (!res.ok) throw new Error('Failed to position folder');
        return folder;
      });
    })
    .then(function (folder) {
      // Step 3: Reparent the target entity under the new folder.
      return Chronicle.apiFetch('/campaigns/' + campaignId + '/entities/' + targetId + '/reorder', {
        method: 'PUT',
        body: { parent_id: folder.id, sort_order: 0 }
      }).then(function (res) {
        if (!res.ok) throw new Error('Failed to reparent target');
        return folder;
      });
    })
    .then(function (folder) {
      // Step 4: Reparent the dragged entity under the new folder.
      return Chronicle.apiFetch('/campaigns/' + campaignId + '/entities/' + droppedId + '/reorder', {
        method: 'PUT',
        body: { parent_id: folder.id, sort_order: 1 }
      }).then(function (res) {
        if (!res.ok) throw new Error('Failed to reparent dragged entity');
      });
    })
    .then(function () {
      // Step 5: Refresh the sidebar tree.
      refreshSidebarTree();
      Chronicle.notify('Folder created', 'success');
    })
    .catch(function (err) {
      console.error('sidebar_tree: create group folder failed', err);
      Chronicle.notify('Failed to create folder', 'error');
    });
  }

  // Single IIFE-scoped listener for reorg mode changes. Always re-queries
  // the DOM to avoid stale container references after HTMX swaps.
  document.addEventListener('chronicle:reorg-changed', function (e) {
    var container = document.getElementById('sidebar-entity-tree');
    if (container) {
      // Keep currentTreeContainer in sync.
      currentTreeContainer = container;
      var hiddenIds = (e.detail && e.detail.hiddenEntityIds) || [];
      updateDraggable(container, e.detail && e.detail.active, hiddenIds);
    }
  });

  // Listen for entity visibility changes and update the eye toggle + opacity.
  document.addEventListener('chronicle:entity-visibility-changed', function (e) {
    if (!e.detail || !e.detail.entityId) return;
    var node = document.querySelector('.sidebar-tree-node[data-entity-id="' + e.detail.entityId + '"]');
    if (!node) return;

    var item = node.querySelector('.sidebar-tree-item');
    var btn = node.querySelector('.reorg-entity-visibility');
    if (e.detail.hidden) {
      node.setAttribute('data-sidebar-hidden', 'true');
      if (item) item.classList.add('opacity-40');
      if (btn) {
        btn.title = 'Show in sidebar';
        btn.innerHTML = '<i class="fa-solid fa-eye-slash text-gray-500"></i>';
      }
    } else {
      node.removeAttribute('data-sidebar-hidden');
      if (item) item.classList.remove('opacity-40');
      if (btn) {
        btn.title = 'Hide from sidebar';
        btn.innerHTML = '<i class="fa-solid fa-eye text-gray-400"></i>';
      }
    }
  });

  // Inject CSS for guide lines (uses custom property set per-node).
  var style = document.createElement('style');
  style.textContent = [
    '.sidebar-tree-node[data-depth]:not([data-depth="0"])::before {',
    '  content: "";',
    '  position: absolute;',
    '  top: 0;',
    '  bottom: 0;',
    '  left: var(--guide-left, 11px);',
    '  width: 1px;',
    '  background: rgba(75, 85, 99, 0.2);',
    '  pointer-events: none;',
    '}'
  ].join('\n');
  document.head.appendChild(style);

  // Listen for HTMX content swaps to re-initialize tree after list refreshes.
  document.addEventListener('htmx:afterSwap', function (e) {
    if (e.detail.target && (
      e.detail.target.id === 'sidebar-cat-results' ||
      e.detail.target.id === 'sidebar-cat-content'
    )) {
      setTimeout(initTree, 10);
    }
  });

  // Initialize on DOM ready.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initTree);
  } else {
    initTree();
  }
})();
