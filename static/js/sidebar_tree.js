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
    Object.keys(nodes).forEach(function (id) {
      var node = nodes[id];
      if (node.parentId && nodes[node.parentId]) {
        nodes[node.parentId].children.push(node);
      } else {
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
      var isCollapsed = !!collapsedSet[node.id];

      // Create wrapper div for the tree node.
      var wrapper = document.createElement('div');
      wrapper.className = 'sidebar-tree-node';
      wrapper.setAttribute('data-entity-id', node.id);
      if (node.parentId) wrapper.setAttribute('data-parent-id', node.parentId);
      wrapper.setAttribute('data-depth', String(depth));

      // Clone the original link element and apply indentation.
      var link = node.el.cloneNode(true);
      link.style.paddingLeft = (12 + depth * INDENT_PX) + 'px';

      // Swap the default page icon based on whether this node has children.
      var iconEl = link.querySelector('.sidebar-tree-icon i');
      if (iconEl) {
        if (hasChildren) {
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
   */
  function setupDragAndDrop(container, campaignId) {
    var dragSrcId = null;
    var dropIndicator = null;

    // Create drop indicator line element (reused across drag operations).
    dropIndicator = document.createElement('div');
    dropIndicator.className = 'sidebar-drop-indicator';
    dropIndicator.style.display = 'none';

    container.addEventListener('dragstart', function (e) {
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
        // Reparent: nest inside target as first child (sort_order 0).
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
      var targets = el.querySelectorAll('.sidebar-drop-target, .sidebar-drop-reparent');
      for (var i = 0; i < targets.length; i++) {
        targets[i].classList.remove('sidebar-drop-target');
        targets[i].classList.remove('sidebar-drop-reparent');
      }
    }
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
        var targetOrder = parseInt(targetNode.querySelector('.sidebar-tree-item')?.getAttribute('data-sort-order') || '0', 10);
        return Math.max(0, targetOrder - 1);
      }
      // Place between previous sibling and target.
      var prevItem = siblings[targetIdx - 1].querySelector('.sidebar-tree-item');
      var targetItem = targetNode.querySelector('.sidebar-tree-item');
      var prevOrder = parseInt(prevItem?.getAttribute('data-sort-order') || '0', 10);
      var targetOrder2 = parseInt(targetItem?.getAttribute('data-sort-order') || '0', 10);
      // Use midpoint if there's room, otherwise server re-normalizes.
      if (targetOrder2 > prevOrder + 1) {
        return Math.floor((prevOrder + targetOrder2) / 2);
      }
      return targetOrder2;
    }
    return 0;
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

    var csrfToken = Chronicle.getCsrf();
    var headers = { 'Content-Type': 'application/json' };
    if (csrfToken) headers['X-CSRF-Token'] = csrfToken;

    fetch('/campaigns/' + campaignId + '/entities/' + entityId + '/reorder', {
      method: 'PUT',
      headers: headers,
      body: JSON.stringify(body)
    })
    .then(function (resp) {
      if (!resp.ok) throw new Error('Reorder failed');
      // Refresh the sidebar entity list to show updated hierarchy.
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
    })
    .catch(function (err) {
      console.error('sidebar_tree: reorder failed', err);
    });
  }

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
