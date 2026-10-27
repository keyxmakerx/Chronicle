/**
 * sidebar_tree.js -- Collapsible tree for the sidebar's category panel
 *
 * Transforms the flat entity list rendered by SidebarEntityList into a
 * collapsible tree using data-parent-id attributes. No folder/page icons —
 * a folder is its chevron, a semibold name and a page count; a page is its
 * plain name (see renderNode). Supports:
 *   - Collapsible folders, each with a chevron and its own page count
 *   - Vertical guide lines for visual hierarchy
 *   - Smooth CSS transitions for collapse/expand
 *   - Collapse state persisted in localStorage per campaign
 *   - Each list's own order: the category's top level, a folder, or a page
 *     with pages under it sorts A to Z, by hand or newest first
 *     (data-root-order / data-child-order). In a list kept by hand, someone
 *     who may edit gets up and down arrows on each row and Alt+Up/Down on a
 *     focused row; the row menu offers the same moves and the order choice.
 *   - Extra listings (data-place-key rows): the same page under a second
 *     parent, a leaf with an "also here" label. They are added from the
 *     page's own place line and never move on their own.
 *
 * Listens for HTMX afterSwap events on #sidebar-cat-results to re-initialize
 * whenever the entity list is refreshed.
 */
(function () {
  'use strict';

  var INDENT_PX = 14;
  var STORAGE_KEY_PREFIX = 'chronicle-tree-collapsed-';

  // Folders first, then pages. Within each group the list's order decides:
  // 'hand' by sort_order (the name breaks a tie), 'new' newest first, and
  // anything else A to Z (sort_order breaks a tie between two same-named
  // rows). Pure over its arguments, so it is hoisted out of initTree() for
  // the test suite.
  function sortChildren(childNodes, mode) {
    childNodes.sort(function (a, b) {
      var aFolder = a.el.hasAttribute('data-is-folder'), bFolder = b.el.hasAttribute('data-is-folder');
      if (aFolder !== bFolder) return aFolder ? -1 : 1;
      var nameA = (a.el.getAttribute('data-entity-name') || '').toLowerCase();
      var nameB = (b.el.getAttribute('data-entity-name') || '').toLowerCase();
      var byName = nameA === nameB ? 0 : (nameA < nameB ? -1 : 1);
      if (mode === 'hand') return (a.sortOrder - b.sortOrder) || byName;
      if (mode === 'new') return ((b.created || 0) - (a.created || 0)) || byName;
      return byName || (a.sortOrder - b.sortOrder);
    });
  }

  // A folder's own count badge: every page (not a folder) in its subtree, at
  // any depth — a folder contributes nothing itself, only what it holds.
  function countPages(node) {
    if (node.isPlace) return 0;
    var n = node.isNode ? 0 : 1;
    node.children.forEach(function (c) { n += countPages(c); });
    return n;
  }


  /**
   * Initialize the tree for a freshly loaded entity list.
   * Reads the flat list of .sidebar-tree-item links, builds a parent-child
   * graph, and re-renders them as a nested tree with toggle buttons and icons.
   */
  function initTree() {
    var container = document.getElementById('sidebar-entity-tree');
    if (!container) return;

    var campaignId = container.getAttribute('data-campaign-id') || '';
    var canEdit = container.getAttribute('data-can-edit') === 'true';
    var items = container.querySelectorAll('.sidebar-tree-item');
    if (!items.length) return;

    // Build lookup: id -> { el, parentId, sortOrder, children[], isNode }
    // Items can be entities (data-entity-id) or folder nodes (data-node-id).
    // Entities can be parented under entities (data-parent-id) or folder nodes
    // (data-parent-node-id). Folder nodes use data-parent-id for their parent.
    var nodes = {};
    var rootIds = [];

    items.forEach(function (el) {
      // An extra listing of a page is keyed by page~parent: the page id alone
      // names its real row, and one page can be listed under many parents.
      var placeKey = el.getAttribute('data-place-key');
      var id = placeKey || el.getAttribute('data-entity-id') || el.getAttribute('data-node-id');
      if (!id) return;
      // Resolve parent: entities may have parent_id (entity) or parent_node_id (folder).
      var parentId = el.getAttribute('data-parent-node-id') || el.getAttribute('data-parent-id') || null;
      var sortOrder = parseInt(el.getAttribute('data-sort-order') || '0', 10);
      var created = parseInt(el.getAttribute('data-created') || '0', 10);
      var isNode = el.hasAttribute('data-node-id');

      nodes[id] = {
        el: el,
        id: id,
        parentId: parentId,
        sortOrder: sortOrder,
        created: created,
        children: [],
        isNode: isNode,
        isPlace: !!placeKey
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
     * Render a single tree node: clones the link element, adds indent, a
     * toggle button (for parents) or a same-size spacer (for leaves), a
     * folder's own page count, and the move arrows when the list it sits in
     * (listMode) is kept by hand.
     */
    function renderNode(node, depth, listMode) {
      var hasChildren = node.children.length > 0;
      var isFolder = node.el.hasAttribute('data-is-folder');
      var isCollapsed = !!collapsedSet[node.id];

      // Create wrapper div for the tree node.
      var wrapper = document.createElement('div');
      wrapper.className = 'sidebar-tree-node';
      // Use the correct ID attribute based on node type.
      if (node.isNode) {
        wrapper.setAttribute('data-node-id', node.id);
      } else if (node.isPlace) {
        // No data-entity-id: a move or a sibling count must not treat a
        // listing as the page itself.
        wrapper.setAttribute('data-place-key', node.id);
        wrapper.setAttribute('data-place-entity', node.el.getAttribute('data-place-entity') || '');
      } else {
        wrapper.setAttribute('data-entity-id', node.id);
      }
      if (node.parentId) wrapper.setAttribute('data-parent-id', node.parentId);
      wrapper.setAttribute('data-depth', String(depth));
      if (node.el.hasAttribute('data-sidebar-hidden')) {
        wrapper.setAttribute('data-sidebar-hidden', 'true');
      }

      // Clone the original element and apply indentation.
      var link = node.el.cloneNode(true);
      link.style.paddingLeft = (10 + depth * INDENT_PX) + 'px';

      // Folder nodes from the template are already <div> elements.
      // Only convert <a> to <div> if it's an entity with is_folder
      // (legacy compat). Pure folder nodes (data-node-id) are already divs.
      if (isFolder && link.tagName === 'A') {
        var div = document.createElement('div');
        div.className = link.className;
        div.style.cssText = link.style.cssText;
        div.innerHTML = link.innerHTML;
        ['data-entity-id', 'data-node-id', 'data-parent-id', 'data-sort-order', 'data-entity-name', 'data-sidebar-hidden', 'data-is-folder', 'data-child-order'].forEach(function (attr) {
          if (link.hasAttribute(attr)) div.setAttribute(attr, link.getAttribute(attr));
        });
        div.classList.add('sidebar-tree-item', 'cursor-default');
        link = div;
      }

      // No folder/page icon (the tree is chevron + name + count only).
      if (hasChildren || isFolder) {
        link.setAttribute('data-has-children', 'true');
      }

      // Add toggle button (chevron) for items with children; a leaf gets a
      // same-size spacer so labels still line up at every depth. aria-expanded
      // on the row itself (not the button) drives the chevron's CSS rotation.
      if (hasChildren) {
        wrapper.setAttribute('data-has-children', 'true');
        link.setAttribute('aria-expanded', String(!isCollapsed));
        var toggle = document.createElement('span');
        toggle.className = 'tree-toggle sidebar-tree-toggle';
        toggle.setAttribute('data-collapsed', String(isCollapsed));
        toggle.innerHTML = '<i class="fa-solid fa-chevron-down" aria-hidden="true"></i>';

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

          link.setAttribute('aria-expanded', String(!isCollapsed));
          toggle.setAttribute('data-collapsed', String(isCollapsed));

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

        link.insertBefore(toggle, link.firstChild);
      } else {
        var spacer = document.createElement('span');
        spacer.className = 'tree-toggle tree-spacer';
        link.insertBefore(spacer, link.firstChild);
      }

      // A folder's count: pages in its subtree, at any depth.
      if (isFolder) {
        var countEl = document.createElement('span');
        countEl.className = 'tree-n';
        countEl.textContent = String(countPages(node));
        link.appendChild(countEl);
      }

      // Set guide line position for nested nodes via inline style.
      if (depth > 0) {
        wrapper.style.setProperty('--guide-left', (11 + (depth - 1) * INDENT_PX) + 'px');
        // Use the CSS ::before pseudo-element positioned by --guide-left.
        wrapper.style.position = 'relative';
      }

      wrapper.appendChild(link);

      // The row menu's dots (sidebar_row_menu.js). A sibling of the row, not
      // inside it, since a page row is a link. A folder offers nothing to
      // someone who cannot edit, so it gets none.
      if (!isFolder || canEdit) {
        var more = document.createElement('button');
        more.type = 'button';
        more.className = 'nav-more tree-more';
        more.setAttribute('data-tree-more', '');
        more.setAttribute('aria-haspopup', 'menu');
        more.setAttribute('aria-expanded', 'false');
        more.setAttribute('aria-label', 'More actions for ' + (node.el.getAttribute('data-entity-name') || ''));
        more.title = 'More actions';
        more.innerHTML = '<i class="fa-solid fa-ellipsis" aria-hidden="true"></i>';
        wrapper.appendChild(more);
        wrapper.classList.add('has-more');
      }

      // A listing follows its page, so it never moves on its own.
      if (canEdit && listMode === 'hand' && !node.isPlace) {
        var steps = document.createElement('span');
        steps.className = 'tree-step';
        steps.appendChild(stepButton(-1, node.el.getAttribute('data-entity-name') || ''));
        steps.appendChild(stepButton(1, node.el.getAttribute('data-entity-name') || ''));
        wrapper.appendChild(steps);
        wrapper.classList.add('has-steps');
      }

      // Render children into a collapsible container.
      if (hasChildren) {
        var childMode = node.el.getAttribute('data-child-order') || 'az';
        sortChildren(node.children, childMode);
        var childContainer = document.createElement('div');
        childContainer.className = 'sidebar-tree-children';
        childContainer.setAttribute('data-order', childMode);
        if (isCollapsed) {
          childContainer.style.maxHeight = '0';
          childContainer.setAttribute('data-collapsed', 'true');
        }
        node.children.forEach(function (child) {
          renderNode(child, depth + 1, childMode);
          childContainer.appendChild(child._wrapper);
        });
        wrapper.appendChild(childContainer);
      }

      node._wrapper = wrapper;
      container.appendChild(wrapper);
    }

    // Sort and render root nodes.
    var roots = rootIds.map(function (id) { return nodes[id]; });
    var rootMode = container.getAttribute('data-root-order') || 'az';
    sortChildren(roots, rootMode);
    roots.forEach(function (node) {
      renderNode(node, 0, rootMode);
    });
    refreshSteps(container);

    markCurrent(container);

    // The rows are fresh clones, which htmx has never seen: process them so
    // they inherit the drill panel's boost and open in place.
    if (window.htmx) htmx.process(container);

    // Stagger fade-in for tree nodes (brief reveal animation).
    var treeNodes = container.querySelectorAll('.sidebar-tree-node');
    treeNodes.forEach(function (nd, i) {
      nd.style.opacity = '0';
      nd.style.transform = 'translateX(-4px)';
      nd.style.transition = 'opacity 150ms ease, transform 150ms ease';
      setTimeout(function () {
        nd.style.opacity = '1';
        nd.style.transform = 'translateX(0)';
      }, Math.min(i * 20, 300));
    });

    document.dispatchEvent(new CustomEvent('chronicle:sidebar-tree'));
  }

  /** Marks the row for the page now showing. Boosted navigation keeps the
   *  tree, so this runs again after each page swap, not only on build. */
  function markCurrent(container) {
    if (!container) return;
    var currentPath = window.location.pathname;
    container.querySelectorAll('a.sidebar-tree-item').forEach(function (a) {
      if (a.getAttribute('href') === currentPath) a.setAttribute('aria-current', 'page');
      else a.removeAttribute('aria-current');
    });
  }

  // --- Order: lists kept by hand ---------------------------------------------

  function stepButton(dir, name) {
    var b = document.createElement('button');
    b.type = 'button';
    b.className = 'tree-step-btn';
    b.setAttribute('data-step', String(dir));
    b.setAttribute('aria-label', 'Move ' + name + (dir < 0 ? ' up' : ' down'));
    b.title = dir < 0 ? 'Move up (Alt+Up)' : 'Move down (Alt+Down)';
    b.innerHTML = '<i class="fa-solid ' + (dir < 0 ? 'fa-arrow-up' : 'fa-arrow-down') + '" aria-hidden="true"></i>';
    return b;
  }

  /** How the list a row sits in is ordered: 'az', 'hand' or 'new'. */
  function listOrder(list) {
    if (!list) return 'az';
    var mode = list.id === 'sidebar-entity-tree' ? list.getAttribute('data-root-order') : list.getAttribute('data-order');
    return mode || 'az';
  }

  /** The rows a move steps over: the row's siblings of its own kind (a
   *  folder among folders, a page among pages), never an extra listing. */
  function stepSiblings(wrapper) {
    var list = wrapper && wrapper.parentElement;
    if (!list) return [];
    var folder = wrapper.hasAttribute('data-node-id');
    return Array.prototype.filter.call(list.children, function (w) {
      return w.classList && w.classList.contains('sidebar-tree-node') &&
        !w.hasAttribute('data-place-key') && w.hasAttribute('data-node-id') === folder;
    });
  }

  function canStep(wrapper, dir) {
    if (!wrapper || wrapper.hasAttribute('data-place-key') || listOrder(wrapper.parentElement) !== 'hand') return false;
    var sibs = stepSiblings(wrapper);
    var i = sibs.indexOf(wrapper);
    return i >= 0 && !!sibs[i + dir];
  }

  /** Turns off the arrows that would step past either end of their list. */
  function refreshSteps(root) {
    if (!root) return;
    root.querySelectorAll('.tree-step-btn').forEach(function (b) {
      b.disabled = !canStep(b.closest('.sidebar-tree-node'), parseInt(b.getAttribute('data-step'), 10));
    });
  }

  // Moves are saved one after another, so quick clicks land in the order
  // they were made.
  var moveQueue = Promise.resolve();

  /**
   * Moves a row one place up (dir -1) or down (dir 1) in a list kept by
   * hand: at once on screen, then saved. focusEl, when given, keeps focus
   * on the control that made the move, so its arrows stay under the hand
   * that is still clicking them. Returns false when the row cannot move.
   */
  function stepRow(wrapper, dir, focusEl) {
    var tree = document.getElementById('sidebar-entity-tree');
    if (!tree || !canStep(wrapper, dir)) return false;
    var sibs = stepSiblings(wrapper);
    var other = sibs[sibs.indexOf(wrapper) + dir];
    var list = wrapper.parentElement;
    // The neighbour moves, not the row: taking the row out of the page, even
    // for a moment, would take focus off whatever in it was focused.
    list.insertBefore(other, dir < 0 ? wrapper.nextSibling : wrapper);
    refreshSteps(list);
    if (focusEl) {
      // At the end of the list its arrow turns off; the other one takes focus.
      if (focusEl.disabled) focusEl = wrapper.querySelector(':scope > .tree-step > .tree-step-btn:not(:disabled)') || wrapper.querySelector(':scope > .sidebar-tree-item');
      if (focusEl && document.activeElement !== focusEl) focusEl.focus({ preventScroll: true });
    }
    if (wrapper.scrollIntoView) wrapper.scrollIntoView({ block: 'nearest' });

    var folder = wrapper.hasAttribute('data-node-id');
    var idAttr = folder ? 'data-node-id' : 'data-entity-id';
    var link = wrapper.querySelector(':scope > .sidebar-tree-item');
    var name = link ? link.getAttribute('data-entity-name') || '' : '';
    announce(name + (dir < 0 ? ' moved up.' : ' moved down.'));
    var url = '/campaigns/' + encodeURIComponent(tree.getAttribute('data-campaign-id') || '') + '/sidebar-order/move';
    var body = { kind: folder ? 'folder' : 'page', id: wrapper.getAttribute(idAttr), next_to: other.getAttribute(idAttr), after: dir > 0 };
    moveQueue = moveQueue.then(function () {
      return Chronicle.apiFetch(url, { method: 'PUT', body: body }).then(function (res) {
        if (!res.ok) throw new Error(String(res.status));
      });
    }).catch(function () {
      if (window.Chronicle && Chronicle.notify) Chronicle.notify(name + ' could not be moved. The list shows the saved order again.', 'error');
      refreshSidebarTree();
    });
    return true;
  }

  /** Says a move out loud for a screen reader, since nothing else changes. */
  function announce(text) {
    var live = document.getElementById('sidebar-tree-live');
    if (!live) {
      live = document.createElement('div');
      live.id = 'sidebar-tree-live';
      live.className = 'sr-only';
      live.setAttribute('aria-live', 'polite');
      document.body.appendChild(live);
    }
    live.textContent = text;
  }

  /**
   * Saves how a list is ordered (kind 'category', 'folder' or 'page'; mode
   * 'az', 'hand' or 'new') and redraws the tree, then runs then() once the
   * new rows are drawn.
   */
  function setOrder(kind, id, mode, then) {
    var tree = document.getElementById('sidebar-entity-tree');
    if (!tree) return Promise.resolve(false);
    var url = '/campaigns/' + encodeURIComponent(tree.getAttribute('data-campaign-id') || '') + '/sidebar-order';
    return Chronicle.apiFetch(url, { method: 'PUT', body: { kind: kind, id: id, mode: mode } }).then(function (res) {
      if (!res.ok) throw new Error(String(res.status));
      if (then) {
        document.addEventListener('chronicle:sidebar-tree', function once() {
          document.removeEventListener('chronicle:sidebar-tree', once);
          then();
        });
      }
      refreshSidebarTree();
      return true;
    }).catch(function () {
      if (window.Chronicle && Chronicle.notify) Chronicle.notify('The order could not be changed.', 'error');
      refreshSidebarTree();
      return false;
    });
  }

  document.addEventListener('click', function (e) {
    var b = e.target && e.target.closest ? e.target.closest('#sidebar-entity-tree .tree-step-btn') : null;
    if (!b) return;
    e.preventDefault();
    e.stopPropagation();
    stepRow(b.closest('.sidebar-tree-node'), parseInt(b.getAttribute('data-step'), 10), b);
  });

  // Alt+Up and Alt+Down move the focused row. A text field keeps its keys.
  document.addEventListener('keydown', function (e) {
    if (!e.altKey || e.ctrlKey || e.metaKey || e.shiftKey || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return;
    var t = e.target;
    if (!t || !t.closest || t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)) return;
    var wrapper = t.closest('#sidebar-entity-tree .sidebar-tree-node');
    var tree = document.getElementById('sidebar-entity-tree');
    if (!wrapper || !tree || tree.getAttribute('data-can-edit') !== 'true') return;
    if (listOrder(wrapper.parentElement) !== 'hand' || wrapper.hasAttribute('data-place-key')) return;
    e.preventDefault();
    stepRow(wrapper, e.key === 'ArrowUp' ? -1 : 1, t);
  });

  // The category's own order picker, drawn above its tree.
  document.addEventListener('change', function (e) {
    var sel = e.target;
    if (!sel || !sel.matches || !sel.matches('[data-tree-order]')) return;
    var tree = document.getElementById('sidebar-entity-tree');
    if (!tree) return;
    setOrder('category', tree.getAttribute('data-entity-type-id') || '', sel.value, function () {
      var again = document.querySelector('[data-tree-order]');
      if (again) again.focus({ preventScroll: true });
    });
  });

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
   * Start inline rename of a folder node or a page. Replaces the name span
   * with an input field. Enter/blur saves, Escape cancels. A page saves
   * through the metadata route, which changes only its name.
   */
  function startInlineRename(link, nodeId, campaignId, entityId) {
    var nameSpan = link.querySelector('.tree-lb');
    if (!nameSpan || nameSpan._renaming) return;
    nameSpan._renaming = true;

    var oldName = nameSpan.textContent;
    var input = document.createElement('input');
    input.type = 'text';
    input.value = oldName;
    // Larger font + padding for mobile usability; scrollIntoView ensures
    // the input stays visible when the virtual keyboard appears.
    input.className = 'bg-transparent border-b border-accent text-sm text-fg w-full outline-none py-1';
    input.style.minWidth = '60px';
    input.style.fontSize = '14px'; // Prevents iOS zoom on focus.

    nameSpan.textContent = '';
    nameSpan.appendChild(input);
    input.focus();
    input.select();
    // Scroll into view so virtual keyboard doesn't cover the input.
    setTimeout(function () { input.scrollIntoView({ block: 'center', behavior: 'smooth' }); }, 100);

    // Removing the input fires blur, which would run save() again; the
    // first Enter, Escape or blur settles the rename and the rest do nothing.
    var settled = false;

    function save() {
      if (settled) return;
      settled = true;
      var newName = input.value.trim();
      if (!newName || newName === oldName) {
        restore();
        return;
      }
      var url = entityId
        ? '/campaigns/' + campaignId + '/entities/' + entityId + '/metadata'
        : '/campaigns/' + campaignId + '/sidebar-nodes/' + nodeId;
      Chronicle.apiFetch(url, {
        method: 'PUT',
        body: { name: newName }
      }).then(function (res) {
        if (res.ok) {
          nameSpan.textContent = newName;
          link.setAttribute('data-entity-name', newName);
        } else {
          nameSpan.textContent = oldName;
          if (Chronicle.notify) Chronicle.notify('The name could not be changed.', 'error');
        }
        nameSpan._renaming = false;
      }).catch(function () {
        nameSpan.textContent = oldName;
        nameSpan._renaming = false;
      });
    }

    function cancel() {
      if (settled) return;
      settled = true;
      restore();
    }

    function restore() {
      nameSpan.textContent = oldName;
      nameSpan._renaming = false;
    }

    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') { e.preventDefault(); save(); }
      if (e.key === 'Escape') { e.preventDefault(); cancel(); }
    });
    input.addEventListener('blur', save);
  }

  // Guide lines: each nested row's line sits under its parent's chevron.
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

  // Tree re-init on HTMX swaps is handled by the single listener near the
  // bottom of this file (alongside observeLoadMore). A second
  // htmx:afterSwap handler would run initTree twice, doubling the chevrons.

  // --- Load More (pagination for large categories) ---

  // Listen for the "Load more" button click in the sidebar entity list.
  // Fetches the next page, extracts new items from the response HTML,
  // appends them to the tree container, and re-initializes the tree.
  document.addEventListener('sidebar:load-more', function (e) {
    var sentinel = e.target.closest('#sidebar-load-more');
    if (!sentinel) return;

    var page = parseInt(sentinel.dataset.page || '2', 10);
    var total = parseInt(sentinel.dataset.total || '0', 10);
    var loaded = parseInt(sentinel.dataset.loaded || '0', 10);

    // Find the search input to get the base URL.
    var searchInput = document.querySelector('#sidebar-cat-content input[name="q"]');
    if (!searchInput) return;

    var baseUrl = searchInput.getAttribute('hx-get') || '';
    var url = baseUrl + '&page=' + page;
    if (searchInput.value) url += '&q=' + encodeURIComponent(searchInput.value);

    // Show loading state.
    var btn = sentinel.querySelector('button');
    if (btn) btn.textContent = 'Loading...';

    // This endpoint answers JSON or the HTML fragment by Accept
    // (SearchAPI's wantsJSON) — apiFetch defaults to Accept: application/json,
    // so this overrides it to get the HTML this handler actually parses.
    Chronicle.apiFetch(url, { headers: { 'HX-Request': 'true', 'Accept': 'text/html' } })
      .then(function (res) { return res.ok ? res.text() : ''; })
      .then(function (html) {
        if (!html) return;

        // Parse the response HTML to extract new tree items.
        var parser = new DOMParser();
        var doc = parser.parseFromString(html, 'text/html');
        var newTree = doc.getElementById('sidebar-entity-tree');
        if (!newTree) return;

        // Append new items to the existing tree container.
        var container = document.getElementById('sidebar-entity-tree');
        if (!container) return;

        var newItems = newTree.querySelectorAll('.sidebar-tree-item');
        var addedCount = 0;
        newItems.forEach(function (item) {
          container.appendChild(item.cloneNode(true));
          addedCount++;
        });

        // Update or remove the sentinel.
        var newLoaded = loaded + addedCount;
        if (newLoaded >= total || addedCount === 0) {
          sentinel.remove();
        } else {
          sentinel.dataset.page = String(page + 1);
          sentinel.dataset.loaded = String(newLoaded);
          if (btn) btn.textContent = 'Load more (' + newLoaded + ' of ' + total + ')';
        }

        // Re-build the tree with all items.
        initTree();
      })
      .catch(function () {
        if (btn) btn.textContent = 'Failed to load — click to retry';
      });
  });

  // Auto-trigger load-more via IntersectionObserver when the sentinel
  // scrolls into view (infinite scroll within the drill panel).
  var loadMoreObserver = null;

  function observeLoadMore() {
    if (loadMoreObserver) loadMoreObserver.disconnect();

    var sentinel = document.getElementById('sidebar-load-more');
    if (!sentinel) return;

    var scrollContainer = sentinel.closest('.overflow-y-auto');
    if (!scrollContainer) return;

    loadMoreObserver = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) {
          sentinel.dispatchEvent(new CustomEvent('sidebar:load-more', { bubbles: true }));
        }
      });
    }, { root: scrollContainer, threshold: 0.1 });

    loadMoreObserver.observe(sentinel);
  }

  // Observe after tree init and HTMX swaps.
  document.addEventListener('htmx:afterSwap', function (e) {
    if (e.detail.target && (
      e.detail.target.id === 'sidebar-cat-results' ||
      e.detail.target.id === 'sidebar-cat-content'
    )) {
      setTimeout(function () { initTree(); observeLoadMore(); }, 10);
    }
  });

  document.addEventListener('htmx:afterSettle', function (e) {
    if (e.detail.target && e.detail.target.id === 'main-content') {
      markCurrent(document.getElementById('sidebar-entity-tree'));
    }
  });

  // Initialize on DOM ready.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { initTree(); observeLoadMore(); });
  } else {
    initTree();
    observeLoadMore();
  }

  // The row menu renames rows, reloads the tree, moves rows and changes a
  // list's order through these.
  window.Chronicle = window.Chronicle || {};
  window.Chronicle.sidebarTree = {
    rename: startInlineRename,
    refresh: refreshSidebarTree,
    step: stepRow,
    canStep: canStep,
    listOrder: listOrder,
    setOrder: setOrder
  };

  // Test-only hook: exposes the pure helpers so the node:test suites can
  // assert them without a real browser DOM. `module` is undefined when
  // loaded via <script>, so this is a no-op in the browser.
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
      sortChildren: sortChildren,
      countPages: countPages,
      startInlineRename: startInlineRename
    };
  }
})();
