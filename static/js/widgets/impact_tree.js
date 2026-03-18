/**
 * impact_tree.js — Renders a collapsible tree diagram for system impact preview.
 *
 * Reads tree data from `data-tree` attribute (JSON), builds a nested tree
 * with CSS-drawn connection lines, icons, and collapsible sections.
 *
 * Mount: `<div data-widget="impact_tree" data-tree="...">`
 *
 * @module impact_tree
 */
(function () {
  'use strict';

  /** Icon map for node types when no explicit icon is set. */
  const DEFAULT_ICONS = {
    root: 'fa-cube',
    section: 'fa-folder',
    category: 'fa-list',
    preset: 'fa-user',
    field: 'fa-circle',
    foundry: 'fa-dice-d20',
    warning: 'fa-triangle-exclamation',
  };

  /** Color classes for node types. */
  const TYPE_COLORS = {
    root: 'text-accent',
    section: 'text-fg',
    category: 'text-blue-600 dark:text-blue-400',
    preset: 'text-purple-600 dark:text-purple-400',
    field: 'text-fg-muted',
    foundry: 'text-orange-600 dark:text-orange-400',
    warning: 'text-yellow-600 dark:text-yellow-400',
  };

  /**
   * Renders a single tree node and its children recursively.
   * @param {Object} node - TreeNode object.
   * @param {number} depth - Current nesting depth.
   * @returns {HTMLElement}
   */
  function renderNode(node, depth) {
    const li = document.createElement('li');
    li.className = 'relative';

    const row = document.createElement('div');
    row.className = 'flex items-center gap-2 py-1 px-2 rounded hover:bg-gray-50 dark:hover:bg-gray-800 cursor-default';

    // Collapse toggle for nodes with children.
    const hasChildren = node.children && node.children.length > 0;
    if (hasChildren) {
      const toggle = document.createElement('button');
      toggle.type = 'button';
      toggle.className = 'text-fg-muted hover:text-fg text-xs w-4 h-4 flex items-center justify-center flex-shrink-0';
      toggle.innerHTML = '<i class="fas fa-chevron-down text-[10px]"></i>';
      toggle.setAttribute('aria-label', 'Toggle');
      toggle.addEventListener('click', () => {
        const childList = li.querySelector(':scope > ul');
        if (childList) {
          const collapsed = childList.classList.toggle('hidden');
          toggle.innerHTML = collapsed
            ? '<i class="fas fa-chevron-right text-[10px]"></i>'
            : '<i class="fas fa-chevron-down text-[10px]"></i>';
        }
      });
      row.appendChild(toggle);
    } else {
      const spacer = document.createElement('span');
      spacer.className = 'w-4 flex-shrink-0';
      row.appendChild(spacer);
    }

    // Icon.
    const icon = document.createElement('i');
    const iconClass = node.icon || DEFAULT_ICONS[node.type] || 'fa-circle';
    const colorClass = TYPE_COLORS[node.type] || 'text-fg-muted';
    icon.className = `fas ${iconClass} ${colorClass} text-sm flex-shrink-0`;
    row.appendChild(icon);

    // Label.
    const label = document.createElement('span');
    label.className = depth === 0
      ? 'text-sm font-semibold text-fg'
      : 'text-sm text-fg';
    label.textContent = node.label;
    row.appendChild(label);

    // Badge.
    if (node.badge) {
      const badge = document.createElement('span');
      const badgeColor = node.type === 'warning'
        ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400'
        : 'bg-gray-100 text-fg-muted dark:bg-gray-800';
      badge.className = `text-xs px-1.5 py-0.5 rounded ${badgeColor}`;
      badge.textContent = node.badge;
      row.appendChild(badge);
    }

    li.appendChild(row);

    // Children.
    if (hasChildren) {
      const childList = document.createElement('ul');
      childList.className = 'ml-4 border-l border-gray-200 dark:border-gray-700 pl-2 space-y-0.5';

      // Auto-collapse field-level nodes for large trees.
      if (node.type === 'preset' && node.children.length > 8) {
        childList.classList.add('hidden');
        const toggle = row.querySelector('button');
        if (toggle) {
          toggle.innerHTML = '<i class="fas fa-chevron-right text-[10px]"></i>';
        }
      }

      for (const child of node.children) {
        childList.appendChild(renderNode(child, depth + 1));
      }
      li.appendChild(childList);
    }

    return li;
  }

  /**
   * Mounts the impact tree widget.
   * @param {HTMLElement} el - Container element with data-tree attribute.
   */
  function mount(el) {
    const rawData = el.getAttribute('data-tree');
    if (!rawData) return;

    let tree;
    try {
      tree = JSON.parse(rawData);
    } catch (e) {
      el.textContent = 'Failed to parse tree data';
      return;
    }

    const root = document.createElement('ul');
    root.className = 'space-y-0.5';
    root.appendChild(renderNode(tree, 0));

    el.innerHTML = '';
    el.appendChild(root);
  }

  // Register with Chronicle widget system.
  if (window.Chronicle && Chronicle.registerWidget) {
    Chronicle.registerWidget('impact_tree', mount);
  } else {
    // Fallback: auto-mount on DOMContentLoaded.
    document.addEventListener('DOMContentLoaded', () => {
      document.querySelectorAll('[data-widget="impact_tree"]').forEach(mount);
    });
  }
})();
