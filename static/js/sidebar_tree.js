/**
 * sidebar_tree.js -- Collapsible Tree + Drag-and-Drop for Sidebar Entity List
 *
 * Transforms the flat entity list rendered by SidebarEntityList into a
 * collapsible tree using data-parent-id attributes. Supports:
 *   - Collapsible folders (entities with children)
 *   - Drag-and-drop reordering within the same level
 *   - Drag-and-drop reparenting (drop onto an entity to nest it)
 *   - Collapse state persisted in localStorage per campaign
 *
 * Listens for HTMX afterSwap events on #sidebar-cat-results to re-initialize
 * whenever the entity list is refreshed.
 */
(function () {
  'use strict';

  var INDENT_PX = 16;
  var STORAGE_KEY_PREFIX = 'chronicle-tree-collapsed-';

  /**
   * Initialize the tree for a freshly loaded entity list.
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

    // Build tree relationships.
    Object.keys(nodes).forEach(function (id) {
      var node = nodes[id];
      if (node.parentId && nodes[node.parentId]) {
        nodes[node.parentId].children.push(node);
      } else {
        rootIds.push(id);
      }
    });

    // Sort children by sort_order, then name.
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
    } catch (e) { /* ignore */ }

    function saveCollapsed() {
      try {
        localStorage.setItem(storageKey, JSON.stringify(collapsedSet));
      } catch (e) { /* ignore */ }
    }

    // Clear container and re-render as tree.
    container.innerHTML = '';

    function renderNode(node, depth) {
      var hasChildren = node.children.length > 0;
      var isCollapsed = !!collapsedSet[node.id];

      // Create wrapper div for the tree item.
      var wrapper = document.createElement('div');
      wrapper.className = 'sidebar-tree-node';
      wrapper.setAttribute('data-entity-id', node.id);
      if (node.parentId) wrapper.setAttribute('data-parent-id', node.parentId);
      wrapper.setAttribute('data-depth', depth);

      // Clone the original link element.
      var link = node.el.cloneNode(true);
      link.style.paddingLeft = (16 + depth * INDENT_PX) + 'px';

      // Add toggle button for items with children.
      if (hasChildren) {
        var toggle = document.createElement('span');
        toggle.className = 'sidebar-tree-toggle inline-flex items-center justify-center w-3 h-3 mr-1 cursor-pointer text-gray-500 hover:text-gray-300 transition-colors shrink-0';
        toggle.innerHTML = isCollapsed
          ? '<i class="fa-solid fa-chevron-right text-[8px]"></i>'
          : '<i class="fa-solid fa-chevron-down text-[8px]"></i>';
        toggle.addEventListener('click', function (e) {
          e.preventDefault();
          e.stopPropagation();
          isCollapsed = !isCollapsed;
          if (isCollapsed) {
            collapsedSet[node.id] = true;
            toggle.innerHTML = '<i class="fa-solid fa-chevron-right text-[8px]"></i>';
          } else {
            delete collapsedSet[node.id];
            toggle.innerHTML = '<i class="fa-solid fa-chevron-down text-[8px]"></i>';
          }
          saveCollapsed();
          // Toggle visibility of children container.
          var childContainer = wrapper.querySelector('.sidebar-tree-children');
          if (childContainer) {
            childContainer.style.display = isCollapsed ? 'none' : 'block';
          }
        });
        // Insert toggle before the color dot.
        link.insertBefore(toggle, link.firstChild);
      } else if (depth > 0) {
        // Add spacer for leaf nodes at depth > 0 to align with toggled siblings.
        var spacer = document.createElement('span');
        spacer.className = 'inline-block w-3 mr-1 shrink-0';
        link.insertBefore(spacer, link.firstChild);
      }

      wrapper.appendChild(link);

      // Render children.
      if (hasChildren) {
        sortChildren(node.children);
        var childContainer = document.createElement('div');
        childContainer.className = 'sidebar-tree-children';
        childContainer.style.display = isCollapsed ? 'none' : 'block';
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
  }

  /**
   * Setup drag-and-drop for reordering and reparenting.
   */
  function setupDragAndDrop(container, campaignId) {
    var dragSrcId = null;
    var dropIndicator = null;

    // Create drop indicator line.
    dropIndicator = document.createElement('div');
    dropIndicator.className = 'sidebar-drop-indicator';
    dropIndicator.style.cssText = 'display:none; height:2px; background:#6366f1; margin:0 8px; border-radius:1px; pointer-events:none;';

    container.addEventListener('dragstart', function (e) {
      var item = e.target.closest('.sidebar-tree-item');
      if (!item) return;
      dragSrcId = item.getAttribute('data-entity-id');
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', dragSrcId);
      item.style.opacity = '0.4';
    });

    container.addEventListener('dragend', function (e) {
      var item = e.target.closest('.sidebar-tree-item');
      if (item) item.style.opacity = '';
      dragSrcId = null;
      hideDropIndicator();
      clearDropTargets(container);
    });

    container.addEventListener('dragover', function (e) {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';

      var target = e.target.closest('.sidebar-tree-node');
      if (!target) return;

      var targetId = target.getAttribute('data-entity-id');
      if (targetId === dragSrcId) return;

      clearDropTargets(container);

      // Determine drop position: top half = insert before, bottom half = nest inside.
      var rect = target.getBoundingClientRect();
      var midY = rect.top + rect.height / 2;

      if (e.clientY < midY) {
        // Insert before — show indicator line above.
        showDropIndicator(target, 'before');
      } else {
        // Drop onto — highlight as parent.
        target.classList.add('sidebar-drop-target');
      }
    });

    container.addEventListener('dragleave', function (e) {
      var target = e.target.closest('.sidebar-tree-node');
      if (target) target.classList.remove('sidebar-drop-target');
    });

    container.addEventListener('drop', function (e) {
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
      var midY = rect.top + rect.height / 2;

      if (e.clientY < midY) {
        // Reorder: place before target (same parent).
        var targetParentId = target.getAttribute('data-parent-id') || null;
        var sortOrder = calculateSortOrder(target, 'before');
        reorderEntity(campaignId, droppedId, targetParentId, sortOrder);
      } else {
        // Reparent: nest inside target.
        // Place as first child (sort_order 0).
        reorderEntity(campaignId, droppedId, targetId, 0);
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
      var targets = el.querySelectorAll('.sidebar-drop-target');
      for (var i = 0; i < targets.length; i++) {
        targets[i].classList.remove('sidebar-drop-target');
      }
    }
  }

  /**
   * Calculate the sort order for an entity being dropped relative to a target.
   * Looks at sibling sort_order values to place the entity in the right position.
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
        var targetOrder = parseInt(targetNode.querySelector('.sidebar-tree-item')?.getAttribute('data-sort-order') || '0', 10);
        return Math.max(0, targetOrder - 1);
      }
      // Place between previous sibling and target.
      var prevItem = siblings[targetIdx - 1].querySelector('.sidebar-tree-item');
      var targetItem = targetNode.querySelector('.sidebar-tree-item');
      var prevOrder = parseInt(prevItem?.getAttribute('data-sort-order') || '0', 10);
      var targetOrder2 = parseInt(targetItem?.getAttribute('data-sort-order') || '0', 10);
      // If there's room between them, use the midpoint. Otherwise use target's order
      // (server will re-normalize).
      if (targetOrder2 > prevOrder + 1) {
        return Math.floor((prevOrder + targetOrder2) / 2);
      }
      return targetOrder2;
    }
    return 0;
  }

  /**
   * Send reorder/reparent request to the API.
   */
  function reorderEntity(campaignId, entityId, newParentId, sortOrder) {
    var body = {
      parent_id: newParentId || null,
      sort_order: sortOrder
    };

    fetch('/campaigns/' + campaignId + '/entities/' + entityId + '/reorder', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    })
    .then(function (resp) {
      if (!resp.ok) throw new Error('Reorder failed');
      // Refresh the sidebar entity list.
      var results = document.getElementById('sidebar-cat-results');
      if (results) {
        var loader = results.querySelector('[hx-get]');
        if (loader) {
          htmx.trigger(loader, 'load');
        } else {
          // Trigger a refresh by finding the search input and clearing it.
          var searchInput = document.querySelector('#sidebar-cat-content input[name="q"]');
          if (searchInput) {
            searchInput.value = '';
            htmx.trigger(searchInput, 'keyup');
          }
        }
      }
    })
    .catch(function (err) {
      console.error('sidebar_tree: reorder failed', err);
    });
  }

  // Add CSS for drop target highlighting.
  var style = document.createElement('style');
  style.textContent = '.sidebar-drop-target { background-color: rgba(99, 102, 241, 0.15) !important; outline: 1px dashed rgba(99, 102, 241, 0.5); outline-offset: -1px; }';
  document.head.appendChild(style);

  // Listen for HTMX content swaps to re-initialize tree.
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
