/**
 * Template Editor Widget
 *
 * Visual drag-and-drop page template editor for entity types.
 * Uses a 12-column grid system with rows, columns, and blocks.
 * Shows animated drop indicators between blocks during drag.
 *
 * Mount: data-widget="template-editor"
 * Config:
 *   data-endpoint   - GET/PUT endpoint for layout JSON
 *   data-layout     - Initial layout JSON
 *   data-fields     - Entity type field definitions JSON
 *   data-entity-type-name - Display name
 *   data-csrf-token - CSRF token
 */
Chronicle.register('template-editor', {
  init(el) {
    this.el = el;
    this.endpoint = el.dataset.endpoint;
    this.entityTypeName = el.dataset.entityTypeName;
    this.csrfToken = el.dataset.csrfToken;
    this.fields = JSON.parse(el.dataset.fields || '[]');
    this.layout = JSON.parse(el.dataset.layout || '{"rows":[]}');
    this.dirty = false;
    // Track current drop indicator position.
    this.dropIndicator = null;
    this.dropTarget = null;

    // Ensure layout has rows.
    if (!this.layout.rows || this.layout.rows.length === 0) {
      this.layout = this.defaultLayout();
    }

    this.render();
    this.bindSave();
  },

  /** Available block types that can be dragged from the palette. */
  blockTypes: [
    { type: 'title',        label: 'Title',        icon: 'fa-heading',       desc: 'Entity name and actions' },
    { type: 'image',        label: 'Image',         icon: 'fa-image',         desc: 'Header image with upload' },
    { type: 'entry',        label: 'Rich Text',     icon: 'fa-align-left',    desc: 'Main content editor' },
    { type: 'attributes',   label: 'Attributes',    icon: 'fa-list',          desc: 'Custom field values' },
    { type: 'details',      label: 'Details',       icon: 'fa-info-circle',   desc: 'Metadata and dates' },
    { type: 'tags',         label: 'Tags',          icon: 'fa-tags',          desc: 'Tag picker widget' },
    { type: 'relations',    label: 'Relations',     icon: 'fa-link',          desc: 'Entity relation links' },
    { type: 'divider',      label: 'Divider',       icon: 'fa-minus',         desc: 'Horizontal separator' },
    { type: 'calendar',     label: 'Calendar',      icon: 'fa-calendar-days', desc: 'Entity calendar events' },
    { type: 'two_column',   label: '2 Columns',     icon: 'fa-columns',       desc: 'Side-by-side columns', container: true },
    { type: 'three_column', label: '3 Columns',     icon: 'fa-table-columns', desc: 'Three equal columns', container: true },
    { type: 'tabs',         label: 'Tabs',          icon: 'fa-folder',        desc: 'Tabbed content sections', container: true },
    { type: 'section',      label: 'Section',       icon: 'fa-caret-down',    desc: 'Collapsible accordion', container: true },
  ],

  /** Container block types that can hold sub-blocks inside them. */
  containerTypes: ['two_column', 'three_column', 'tabs', 'section'],

  /** Width presets for two_column blocks (left/right out of 12). */
  twoColPresets: [
    { label: '50 / 50', left: 6, right: 6 },
    { label: '33 / 67', left: 4, right: 8 },
    { label: '67 / 33', left: 8, right: 4 },
  ],

  /** Column layout presets for quick row configuration. */
  colPresets: [
    { label: '1 Column',  widths: [12] },
    { label: '2 Columns', widths: [6, 6] },
    { label: 'Wide + Sidebar', widths: [8, 4] },
    { label: 'Sidebar + Wide', widths: [4, 8] },
    { label: '3 Columns', widths: [4, 4, 4] },
  ],

  /** Height presets for block minimum height control. */
  heightPresets: [
    { value: 'auto', label: 'Auto' },
    { value: 'sm',   label: 'Small',  px: '150px' },
    { value: 'md',   label: 'Medium', px: '300px' },
    { value: 'lg',   label: 'Large',  px: '500px' },
    { value: 'xl',   label: 'X-Large', px: '700px' },
  ],

  /** Visibility options for block access control. */
  visibilityOptions: [
    { value: 'everyone', label: 'Everyone',  icon: 'fa-globe' },
    { value: 'dm_only',  label: 'DM Only',   icon: 'fa-lock' },
  ],

  defaultLayout() {
    return {
      rows: [{
        id: this.uid('row'),
        columns: [
          { id: this.uid('col'), width: 8, blocks: [
            { id: this.uid('blk'), type: 'title', config: {} },
            { id: this.uid('blk'), type: 'entry', config: {} },
          ]},
          { id: this.uid('col'), width: 4, blocks: [
            { id: this.uid('blk'), type: 'image', config: {} },
            { id: this.uid('blk'), type: 'attributes', config: {} },
            { id: this.uid('blk'), type: 'details', config: {} },
          ]},
        ],
      }],
    };
  },

  uid(prefix) {
    return prefix + '-' + Math.random().toString(36).substr(2, 6);
  },

  /** Create a draggable palette item for a block type definition. */
  createPaletteItem(bt) {
    const item = document.createElement('div');
    item.className = 'flex items-center gap-2 px-3 py-2 mb-1 bg-white dark:bg-gray-700 border border-gray-200 dark:border-gray-600 rounded-md cursor-grab hover:border-indigo-300 dark:hover:border-indigo-500 hover:shadow-sm transition-all text-sm';
    item.draggable = true;
    item.innerHTML = `
      <i class="fa-solid ${bt.icon} w-4 text-gray-400 dark:text-gray-500 text-center"></i>
      <div>
        <div class="font-medium text-gray-700 dark:text-gray-200">${bt.label}</div>
        <div class="text-[10px] text-gray-400 dark:text-gray-500">${bt.desc}</div>
      </div>
    `;
    item.addEventListener('dragstart', (e) => {
      e.dataTransfer.setData('text/plain', JSON.stringify({ source: 'palette', type: bt.type }));
      e.dataTransfer.effectAllowed = 'copyMove';
      item.classList.add('opacity-50');
    });
    item.addEventListener('dragend', () => {
      item.classList.remove('opacity-50');
      this.clearDropIndicator();
    });
    return item;
  },

  /** Return default config for a newly created block of the given type. */
  defaultBlockConfig(type) {
    switch (type) {
      case 'two_column':
        return { left_width: 6, right_width: 6, left: [], right: [] };
      case 'three_column':
        return { widths: [4, 4, 4], columns: [[], [], []] };
      case 'tabs':
        return { tabs: [{ label: 'Tab 1', blocks: [] }, { label: 'Tab 2', blocks: [] }] };
      case 'section':
        return { title: 'Section', collapsed: false, blocks: [] };
      default:
        return {};
    }
  },

  /** Check whether a block type is a container that holds sub-blocks. */
  isContainer(type) {
    return this.containerTypes.includes(type);
  },

  render() {
    this.el.innerHTML = '';
    this.el.className = 'flex h-full overflow-hidden';

    // Palette panel.
    const palette = document.createElement('div');
    palette.className = 'w-56 bg-gray-50 dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 p-4 overflow-y-auto shrink-0';
    palette.innerHTML = `
      <h3 class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-3">Components</h3>
    `;

    // Separate content blocks from layout/container blocks for grouped display.
    const contentBlocks = this.blockTypes.filter(bt => !bt.container);
    const layoutBlocks = this.blockTypes.filter(bt => bt.container);

    contentBlocks.forEach(bt => {
      palette.appendChild(this.createPaletteItem(bt));
    });

    // Layout blocks section header.
    const layoutHeader = document.createElement('h3');
    layoutHeader.className = 'text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-3 mt-5';
    layoutHeader.textContent = 'Layout Blocks';
    palette.appendChild(layoutHeader);

    layoutBlocks.forEach(bt => {
      palette.appendChild(this.createPaletteItem(bt));
    });

    // Row presets section.
    const presetSection = document.createElement('div');
    presetSection.className = 'mt-6';
    presetSection.innerHTML = '<h3 class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-3">Row Layouts</h3>';
    this.colPresets.forEach(preset => {
      const btn = document.createElement('button');
      btn.className = 'flex items-center gap-2 w-full px-3 py-2 mb-1 bg-white dark:bg-gray-700 border border-gray-200 dark:border-gray-600 rounded-md hover:border-indigo-300 dark:hover:border-indigo-500 hover:shadow-sm transition-all text-sm text-left';
      const preview = preset.widths.map(w => {
        const pct = Math.round(w / 12 * 100);
        return `<div class="h-3 bg-gray-300 dark:bg-gray-500 rounded-sm" style="width:${pct}%"></div>`;
      }).join('<div class="w-0.5"></div>');
      btn.innerHTML = `
        <div class="flex gap-0.5 w-12 shrink-0">${preview}</div>
        <span class="text-gray-700 dark:text-gray-200">${preset.label}</span>
      `;
      btn.addEventListener('click', () => this.addRow(preset.widths));
      presetSection.appendChild(btn);
    });
    palette.appendChild(presetSection);

    this.el.appendChild(palette);

    // Canvas area.
    const canvas = document.createElement('div');
    canvas.className = 'flex-1 overflow-y-auto p-6 bg-gray-100 dark:bg-gray-900';
    this.canvas = canvas;
    this.renderCanvas();
    this.el.appendChild(canvas);
  },

  renderCanvas() {
    this.canvas.innerHTML = '';

    if (this.layout.rows.length === 0) {
      this.canvas.innerHTML = `
        <div class="flex flex-col items-center justify-center h-full text-gray-400 dark:text-gray-500">
          <i class="fa-solid fa-table-cells-large text-4xl mb-3"></i>
          <p class="text-sm">Click a row layout on the left to get started</p>
        </div>
      `;
      return;
    }

    this.layout.rows.forEach((row, rowIdx) => {
      const rowEl = document.createElement('div');
      rowEl.className = 'mb-4 group/row';
      rowEl.dataset.rowIdx = rowIdx;

      // Row toolbar.
      const toolbar = document.createElement('div');
      toolbar.className = 'flex items-center gap-2 mb-1 opacity-0 group-hover/row:opacity-100 transition-opacity';
      toolbar.innerHTML = `
        <span class="text-[10px] text-gray-400 dark:text-gray-500 font-mono">Row ${rowIdx + 1}</span>
        <div class="flex-1"></div>
      `;

      // Column layout picker for this row.
      this.colPresets.forEach(preset => {
        const btn = document.createElement('button');
        btn.className = 'p-1 hover:bg-gray-200 dark:hover:bg-gray-700 rounded transition-colors';
        btn.title = preset.label;
        const isActive = JSON.stringify(row.columns.map(c => c.width)) === JSON.stringify(preset.widths);
        const preview = preset.widths.map(w => {
          const pct = Math.round(w / 12 * 100);
          const color = isActive ? 'bg-indigo-400' : 'bg-gray-300 dark:bg-gray-500';
          return `<div class="h-2 ${color} rounded-sm" style="width:${pct}%"></div>`;
        }).join('<div class="w-px"></div>');
        btn.innerHTML = `<div class="flex gap-px w-8">${preview}</div>`;
        btn.addEventListener('click', () => this.changeRowLayout(rowIdx, preset.widths));
        toolbar.appendChild(btn);
      });

      // Delete row button.
      const delBtn = document.createElement('button');
      delBtn.className = 'p-1 text-gray-300 dark:text-gray-600 hover:text-red-500 dark:hover:text-red-400 transition-colors ml-1';
      delBtn.title = 'Delete row';
      delBtn.innerHTML = '<i class="fa-solid fa-trash-can text-xs"></i>';
      delBtn.addEventListener('click', () => this.deleteRow(rowIdx));
      toolbar.appendChild(delBtn);

      // Move buttons.
      if (rowIdx > 0) {
        const upBtn = document.createElement('button');
        upBtn.className = 'p-1 text-gray-300 dark:text-gray-600 hover:text-gray-600 dark:hover:text-gray-300 transition-colors';
        upBtn.title = 'Move up';
        upBtn.innerHTML = '<i class="fa-solid fa-chevron-up text-xs"></i>';
        upBtn.addEventListener('click', () => this.moveRow(rowIdx, -1));
        toolbar.appendChild(upBtn);
      }
      if (rowIdx < this.layout.rows.length - 1) {
        const downBtn = document.createElement('button');
        downBtn.className = 'p-1 text-gray-300 dark:text-gray-600 hover:text-gray-600 dark:hover:text-gray-300 transition-colors';
        downBtn.title = 'Move down';
        downBtn.innerHTML = '<i class="fa-solid fa-chevron-down text-xs"></i>';
        downBtn.addEventListener('click', () => this.moveRow(rowIdx, 1));
        toolbar.appendChild(downBtn);
      }

      rowEl.appendChild(toolbar);

      // Columns grid.
      const grid = document.createElement('div');
      grid.className = 'grid gap-3';
      grid.style.gridTemplateColumns = row.columns.map(c => `${c.width}fr`).join(' ');

      row.columns.forEach((col, colIdx) => {
        const colEl = document.createElement('div');
        colEl.className = 'te-column bg-white dark:bg-gray-800 border-2 border-dashed border-gray-200 dark:border-gray-600 rounded-lg min-h-[80px] p-2 transition-colors relative';
        colEl.dataset.rowIdx = rowIdx;
        colEl.dataset.colIdx = colIdx;

        // Column header.
        const colHeader = document.createElement('div');
        colHeader.className = 'text-[10px] text-gray-300 dark:text-gray-600 font-mono mb-1 px-1';
        colHeader.textContent = `${col.width}/12`;
        colEl.appendChild(colHeader);

        // Render blocks with drop zones between them.
        col.blocks.forEach((block, blockIdx) => {
          const blockEl = this.renderBlock(block, rowIdx, colIdx, blockIdx);
          colEl.appendChild(blockEl);
        });

        // Column-level drag events for drop position tracking.
        colEl.addEventListener('dragover', (e) => {
          e.preventDefault();
          e.dataTransfer.dropEffect = 'move';
          colEl.classList.add('border-indigo-400');
          this.updateDropIndicator(e, colEl, rowIdx, colIdx);
        });
        colEl.addEventListener('dragleave', (e) => {
          if (!colEl.contains(e.relatedTarget)) {
            colEl.classList.remove('border-indigo-400');
            this.clearDropIndicator();
          }
        });
        colEl.addEventListener('drop', (e) => {
          e.preventDefault();
          colEl.classList.remove('border-indigo-400');
          const insertIdx = this.dropTarget ? this.dropTarget.insertIdx : col.blocks.length;
          this.clearDropIndicator();
          this.handleDrop(e, rowIdx, colIdx, insertIdx);
        });

        grid.appendChild(colEl);
      });

      rowEl.appendChild(grid);
      this.canvas.appendChild(rowEl);
    });
  },

  /** Compute where the drop indicator should appear based on mouse Y position. */
  updateDropIndicator(e, colEl, rowIdx, colIdx) {
    // Use only direct child .te-block elements to avoid matching sub-blocks inside containers.
    const blockEls = Array.from(colEl.children).filter(c => c.classList.contains('te-block'));
    let insertIdx = blockEls.length; // Default: append at end.
    let indicatorY = null;
    let referenceEl = null;

    for (let i = 0; i < blockEls.length; i++) {
      const rect = blockEls[i].getBoundingClientRect();
      const midY = rect.top + rect.height / 2;
      if (e.clientY < midY) {
        insertIdx = i;
        referenceEl = blockEls[i];
        break;
      }
    }

    // Only update if position changed.
    if (this.dropTarget &&
        this.dropTarget.rowIdx === rowIdx &&
        this.dropTarget.colIdx === colIdx &&
        this.dropTarget.insertIdx === insertIdx) {
      return;
    }

    this.clearDropIndicator();
    this.dropTarget = { rowIdx, colIdx, insertIdx };

    // Create the indicator line.
    const indicator = document.createElement('div');
    indicator.className = 'te-drop-indicator';
    indicator.style.cssText = 'height: 3px; background: #6366f1; border-radius: 2px; margin: 2px 4px; transition: opacity 0.15s ease; opacity: 0; position: relative;';
    // Animated glow effect.
    indicator.innerHTML = '<div style="position:absolute;inset:-2px 0;background:#6366f1;opacity:0.2;border-radius:4px;animation:te-pulse 1s ease-in-out infinite"></div>';

    if (referenceEl) {
      colEl.insertBefore(indicator, referenceEl);
    } else {
      colEl.appendChild(indicator);
    }

    // Fade in.
    requestAnimationFrame(() => { indicator.style.opacity = '1'; });
    this.dropIndicator = indicator;
  },

  /** Remove the current drop indicator from the DOM. */
  clearDropIndicator() {
    if (this.dropIndicator && this.dropIndicator.parentNode) {
      this.dropIndicator.remove();
    }
    this.dropIndicator = null;
    this.dropTarget = null;
  },

  renderBlock(block, rowIdx, colIdx, blockIdx) {
    // Container blocks get a specialized expanded renderer.
    if (this.isContainer(block.type)) {
      return this.renderContainerBlock(block, rowIdx, colIdx, blockIdx);
    }

    // Ensure config exists.
    if (!block.config) block.config = {};

    const bt = this.blockTypes.find(b => b.type === block.type) || { label: block.type, icon: 'fa-cube' };
    const el = document.createElement('div');
    el.className = 'te-block flex items-center gap-2 px-3 py-2 mb-1 bg-gray-50 dark:bg-gray-700 border border-gray-200 dark:border-gray-600 rounded group/block cursor-grab hover:border-indigo-300 dark:hover:border-indigo-500 transition-colors';
    el.draggable = true;
    el.dataset.blockIdx = blockIdx;

    const curHeight = block.config.minHeight || 'auto';
    const curVisibility = block.config.visibility || 'everyone';
    const visIcon = curVisibility === 'dm_only' ? 'fa-lock' : '';

    // DM-only blocks get a subtle red-ish border to stand out in the editor.
    if (curVisibility === 'dm_only') {
      el.classList.add('border-amber-400', 'dark:border-amber-600');
    }

    el.innerHTML = `
      <i class="fa-solid fa-grip-vertical text-gray-300 dark:text-gray-500 text-xs"></i>
      <i class="fa-solid ${bt.icon} w-4 text-gray-400 dark:text-gray-500 text-center text-sm"></i>
      <span class="text-sm font-medium text-gray-700 dark:text-gray-200 flex-1">${bt.label}</span>
      ${curVisibility === 'dm_only' ? '<i class="fa-solid fa-lock text-amber-500 text-[10px]" title="DM Only"></i>' : ''}
      <select class="te-block-vis opacity-0 group-hover/block:opacity-100 text-[10px] bg-transparent text-gray-400 dark:text-gray-500 border border-gray-300 dark:border-gray-600 rounded px-1 py-0.5 cursor-pointer hover:text-gray-600 dark:hover:text-gray-300 transition-all" title="Visibility">
        ${this.visibilityOptions.map(v => `<option value="${v.value}" ${v.value === curVisibility ? 'selected' : ''}>${v.label}</option>`).join('')}
      </select>
      <select class="te-block-height opacity-0 group-hover/block:opacity-100 text-[10px] bg-transparent text-gray-400 dark:text-gray-500 border border-gray-300 dark:border-gray-600 rounded px-1 py-0.5 cursor-pointer hover:text-gray-600 dark:hover:text-gray-300 transition-all" title="Block height">
        ${this.heightPresets.map(h => `<option value="${h.value}" ${h.value === curHeight ? 'selected' : ''}>${h.label}</option>`).join('')}
      </select>
      <button class="te-block-del opacity-0 group-hover/block:opacity-100 text-gray-300 dark:text-gray-500 hover:text-red-500 dark:hover:text-red-400 transition-all p-0.5" title="Remove">
        <i class="fa-solid fa-xmark text-xs"></i>
      </button>
    `;

    // Bind visibility change.
    const visSelect = el.querySelector('.te-block-vis');
    visSelect.addEventListener('change', (e) => {
      e.stopPropagation();
      const val = e.target.value;
      if (val === 'everyone') {
        delete block.config.visibility;
      } else {
        block.config.visibility = val;
      }
      this.markDirty();
      this.renderCanvas();
    });
    visSelect.addEventListener('mousedown', (e) => e.stopPropagation());

    // Bind height change.
    const heightSelect = el.querySelector('.te-block-height');
    heightSelect.addEventListener('change', (e) => {
      e.stopPropagation();
      const val = e.target.value;
      if (val === 'auto') {
        delete block.config.minHeight;
      } else {
        block.config.minHeight = val;
      }
      this.markDirty();
    });
    // Prevent drag on select interaction.
    heightSelect.addEventListener('mousedown', (e) => e.stopPropagation());

    this.bindBlockDrag(el, block, rowIdx, colIdx, blockIdx);
    this.bindBlockDelete(el, rowIdx, colIdx, blockIdx);
    this.bindBlockPreview(el, block);

    return el;
  },

  /** Bind drag-start/drag-end events to a block element for canvas reordering. */
  bindBlockDrag(el, block, rowIdx, colIdx, blockIdx) {
    el.addEventListener('dragstart', (e) => {
      e.stopPropagation();
      e.dataTransfer.setData('text/plain', JSON.stringify({
        source: 'canvas',
        rowIdx, colIdx, blockIdx,
        block,
      }));
      e.dataTransfer.effectAllowed = 'move';
      el.classList.add('opacity-50');
    });
    el.addEventListener('dragend', () => {
      el.classList.remove('opacity-50');
      this.clearDropIndicator();
    });
  },

  /** Bind the delete button click to remove a block from the layout. */
  bindBlockDelete(el, rowIdx, colIdx, blockIdx) {
    el.querySelector('.te-block-del').addEventListener('click', (e) => {
      e.stopPropagation();
      this.layout.rows[rowIdx].columns[colIdx].blocks.splice(blockIdx, 1);
      this.markDirty();
      this.renderCanvas();
    });
  },

  /**
   * Bind right-click context menu on a block to show a visual preview overlay.
   * The preview shows a mock-up of how the block will look on an entity page.
   */
  bindBlockPreview(el, block) {
    const editor = this;
    el.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      e.stopPropagation();
      editor.showBlockPreview(block, e.clientX, e.clientY);
    });
    // Also support click with a small preview icon.
    const previewBtn = document.createElement('button');
    previewBtn.className = 'te-preview-btn opacity-0 group-hover/block:opacity-100 text-gray-300 dark:text-gray-500 hover:text-indigo-500 dark:hover:text-indigo-400 transition-all p-0.5 mr-1';
    previewBtn.title = 'Preview appearance';
    previewBtn.innerHTML = '<i class="fa-solid fa-eye text-xs"></i>';
    previewBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      const rect = el.getBoundingClientRect();
      editor.showBlockPreview(block, rect.left + rect.width / 2, rect.top);
    });
    // Insert before delete button.
    const delBtn = el.querySelector('.te-block-del');
    if (delBtn) {
      delBtn.parentNode.insertBefore(previewBtn, delBtn);
    }
  },

  /**
   * Show a preview overlay for a block type, displaying a mock-up of
   * how the block will render on an actual entity page.
   */
  showBlockPreview(block, x, y) {
    // Remove any existing preview.
    this.closeBlockPreview();

    const bt = this.blockTypes.find(b => b.type === block.type) || { label: block.type, icon: 'fa-cube', desc: '' };

    // Create overlay backdrop.
    const backdrop = document.createElement('div');
    backdrop.className = 'te-preview-backdrop';
    backdrop.style.cssText = 'position:fixed;inset:0;z-index:9998;background:rgba(0,0,0,0.3);';
    backdrop.addEventListener('click', () => this.closeBlockPreview());

    // Create preview panel.
    const panel = document.createElement('div');
    panel.className = 'te-preview-panel';
    panel.style.cssText = 'position:fixed;z-index:9999;background:var(--color-card-bg,#fff);border:1px solid var(--color-border,#e5e7eb);border-radius:12px;box-shadow:0 20px 60px rgba(0,0,0,0.2);max-width:480px;width:90%;overflow:hidden;';

    // Center the panel on screen.
    panel.style.left = '50%';
    panel.style.top = '50%';
    panel.style.transform = 'translate(-50%, -50%)';

    // Header.
    const header = document.createElement('div');
    header.style.cssText = 'padding:12px 16px;border-bottom:1px solid var(--color-border-light,#f3f4f6);display:flex;align-items:center;gap:8px;';
    header.innerHTML = `
      <i class="fa-solid ${bt.icon}" style="color:var(--color-text-muted);font-size:14px;"></i>
      <span style="font-weight:600;font-size:14px;color:var(--color-text-primary);">${bt.label} Preview</span>
      <span style="flex:1"></span>
      <span style="font-size:11px;color:var(--color-text-muted);padding:2px 8px;border-radius:4px;background:var(--color-bg-tertiary);">Mock preview</span>
    `;

    // Preview content area with highlighted border.
    const content = document.createElement('div');
    content.style.cssText = 'padding:16px;';

    const preview = document.createElement('div');
    preview.style.cssText = 'border:2px solid #6366f1;border-radius:8px;padding:16px;background:var(--color-bg-secondary,#fff);position:relative;';

    // Highlight indicator.
    const indicator = document.createElement('div');
    indicator.style.cssText = 'position:absolute;top:-10px;left:12px;background:#6366f1;color:white;font-size:10px;font-weight:600;padding:1px 8px;border-radius:4px;';
    indicator.textContent = bt.label;
    preview.appendChild(indicator);

    // Mock content based on block type.
    preview.appendChild(this.createBlockMockup(block.type));

    content.appendChild(preview);

    // Footer with description.
    const footer = document.createElement('div');
    footer.style.cssText = 'padding:12px 16px;border-top:1px solid var(--color-border-light,#f3f4f6);font-size:12px;color:var(--color-text-secondary);';
    footer.textContent = bt.desc;

    panel.appendChild(header);
    panel.appendChild(content);
    panel.appendChild(footer);

    document.body.appendChild(backdrop);
    document.body.appendChild(panel);

    this._previewBackdrop = backdrop;
    this._previewPanel = panel;

    // Close on Escape.
    this._previewEscHandler = (e) => {
      if (e.key === 'Escape') this.closeBlockPreview();
    };
    document.addEventListener('keydown', this._previewEscHandler);
  },

  /** Close the block preview overlay. */
  closeBlockPreview() {
    if (this._previewBackdrop) {
      this._previewBackdrop.remove();
      this._previewBackdrop = null;
    }
    if (this._previewPanel) {
      this._previewPanel.remove();
      this._previewPanel = null;
    }
    if (this._previewEscHandler) {
      document.removeEventListener('keydown', this._previewEscHandler);
      this._previewEscHandler = null;
    }
  },

  /** Create a static mockup of a block type for the preview overlay. */
  createBlockMockup(type) {
    const mock = document.createElement('div');
    mock.style.color = 'var(--color-text-body,#374151)';

    switch (type) {
      case 'title':
        mock.innerHTML = `
          <div style="display:flex;align-items:center;justify-content:space-between;">
            <div style="font-size:24px;font-weight:700;color:var(--color-text-primary,#111827);">Entity Name</div>
            <div style="display:flex;gap:6px;">
              <span style="padding:4px 12px;font-size:12px;background:var(--color-bg-tertiary);border:1px solid var(--color-border);border-radius:6px;color:var(--color-text-secondary);">Edit</span>
              <span style="padding:4px 12px;font-size:12px;background:#dc2626;color:white;border-radius:6px;">Delete</span>
            </div>
          </div>
        `;
        break;

      case 'image':
        mock.innerHTML = `
          <div style="background:var(--color-bg-tertiary);border-radius:8px;height:140px;display:flex;align-items:center;justify-content:center;color:var(--color-text-muted);">
            <i class="fa-solid fa-image" style="font-size:32px;opacity:0.4;"></i>
          </div>
        `;
        break;

      case 'entry':
        mock.innerHTML = `
          <div style="border:1px solid var(--color-border);border-radius:8px;overflow:hidden;">
            <div style="padding:8px 12px;border-bottom:1px solid var(--color-border-light);display:flex;justify-content:space-between;align-items:center;">
              <span style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;color:var(--color-text-secondary);">Entry</span>
              <span style="font-size:11px;color:var(--color-text-muted);"><i class="fa-solid fa-pen" style="font-size:10px"></i> Edit</span>
            </div>
            <div style="padding:16px;font-size:13px;line-height:1.6;">
              <p style="margin:0 0 8px;">Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore.</p>
              <p style="margin:0;color:var(--color-text-secondary);">Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.</p>
            </div>
          </div>
        `;
        break;

      case 'attributes':
        var fieldMocks = this.fields.length > 0
          ? this.fields.slice(0, 4).map(f => `
              <div style="margin-bottom:8px;">
                <div style="font-size:10px;font-weight:500;text-transform:uppercase;letter-spacing:0.05em;color:var(--color-text-secondary);">${f.label}</div>
                <div style="font-size:13px;color:var(--color-text-primary);margin-top:2px;">${f.type === 'checkbox' ? 'Yes' : 'Sample value'}</div>
              </div>
            `).join('')
          : `<div style="font-size:12px;color:var(--color-text-muted);">No fields defined</div>`;

        mock.innerHTML = `
          <div style="border:1px solid var(--color-border);border-radius:8px;padding:12px;">
            <div style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;color:var(--color-text-secondary);margin-bottom:10px;">Attributes</div>
            ${fieldMocks}
          </div>
        `;
        break;

      case 'details':
        mock.innerHTML = `
          <div style="border:1px solid var(--color-border);border-radius:8px;padding:12px;">
            <div style="display:flex;align-items:center;gap:6px;margin-bottom:10px;">
              <span style="padding:2px 10px;border-radius:999px;background:#6366f1;color:white;font-size:12px;font-weight:500;"><i class="fa-solid fa-user" style="font-size:10px;margin-right:4px;"></i> Character</span>
              <span style="font-size:12px;color:var(--color-text-secondary);">Warrior</span>
            </div>
            <div style="border-top:1px solid var(--color-border-light);padding-top:8px;font-size:11px;color:var(--color-text-muted);">
              <div>Created Jan 1, 2026</div>
              <div>Updated Feb 20, 2026</div>
            </div>
          </div>
        `;
        break;

      case 'tags':
        mock.innerHTML = `
          <div style="display:flex;flex-wrap:wrap;gap:4px;">
            <span style="padding:2px 8px;border-radius:999px;font-size:11px;font-weight:500;background:#6366f122;color:#6366f1;">Important</span>
            <span style="padding:2px 8px;border-radius:999px;font-size:11px;font-weight:500;background:#22c55e22;color:#22c55e;">Ally</span>
            <span style="padding:2px 8px;border-radius:999px;font-size:11px;font-weight:500;background:#f4393e22;color:#f43f5e;">Villain</span>
            <span style="padding:2px 8px;border-radius:999px;font-size:11px;font-weight:500;border:1px dashed var(--color-border);color:var(--color-text-muted);font-size:11px;"><i class="fa-solid fa-plus" style="font-size:9px"></i> Tag</span>
          </div>
        `;
        break;

      case 'relations':
        mock.innerHTML = `
          <div style="border:1px solid var(--color-border);border-radius:8px;overflow:hidden;">
            <div style="padding:6px 10px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;color:var(--color-text-secondary);border-bottom:1px solid var(--color-border-light);">
              <i class="fa-solid fa-link" style="font-size:9px"></i> Allied With
            </div>
            <div style="padding:8px 10px;display:flex;align-items:center;gap:6px;">
              <span style="width:24px;height:24px;border-radius:50%;background:#22c55e;display:flex;align-items:center;justify-content:center;"><i class="fa-solid fa-user" style="font-size:10px;color:white;"></i></span>
              <span style="font-size:13px;font-weight:500;color:var(--color-text-primary);">Another Entity</span>
            </div>
          </div>
        `;
        break;

      case 'divider':
        mock.innerHTML = `
          <hr style="border:none;border-top:1px solid var(--color-border);margin:8px 0;" />
        `;
        break;

      default:
        mock.innerHTML = `<div style="font-size:13px;color:var(--color-text-muted);">Preview not available for this block type.</div>`;
    }

    return mock;
  },

  /**
   * Render a container block (two_column, three_column, tabs, section).
   * Container blocks display an expanded visual with drop zones for sub-blocks,
   * plus configuration controls for their layout properties.
   */
  renderContainerBlock(block, rowIdx, colIdx, blockIdx) {
    const bt = this.blockTypes.find(b => b.type === block.type) || { label: block.type, icon: 'fa-cube' };
    const el = document.createElement('div');
    el.className = 'te-block te-container-block mb-1 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 overflow-hidden group/block';
    el.draggable = true;
    el.dataset.blockIdx = blockIdx;

    // Container header bar with drag handle, label, config, and delete.
    const header = document.createElement('div');
    header.className = 'flex items-center gap-2 px-3 py-2 bg-indigo-50 dark:bg-indigo-900/30 border-b border-indigo-100 dark:border-indigo-800 cursor-grab';
    header.innerHTML = `
      <i class="fa-solid fa-grip-vertical text-indigo-300 dark:text-indigo-600 text-xs"></i>
      <i class="fa-solid ${bt.icon} w-4 text-indigo-400 dark:text-indigo-500 text-center text-sm"></i>
      <span class="text-sm font-semibold text-indigo-700 dark:text-indigo-300 flex-1">${bt.label}</span>
    `;

    // Config controls specific to the container type (inserted into header).
    const configArea = document.createElement('div');
    configArea.className = 'flex items-center gap-1';
    this.renderContainerConfig(configArea, block, rowIdx, colIdx, blockIdx);

    // Delete button.
    const delBtn = document.createElement('button');
    delBtn.className = 'te-block-del text-indigo-300 dark:text-indigo-600 hover:text-red-500 dark:hover:text-red-400 transition-all p-0.5 ml-1';
    delBtn.title = 'Remove';
    delBtn.innerHTML = '<i class="fa-solid fa-xmark text-xs"></i>';
    configArea.appendChild(delBtn);
    header.appendChild(configArea);
    el.appendChild(header);

    // Container body with sub-block drop zones.
    const body = document.createElement('div');
    body.className = 'p-2';
    this.renderContainerBody(body, block, rowIdx, colIdx, blockIdx);
    el.appendChild(body);

    this.bindBlockDrag(el, block, rowIdx, colIdx, blockIdx);
    delBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      this.layout.rows[rowIdx].columns[colIdx].blocks.splice(blockIdx, 1);
      this.markDirty();
      this.renderCanvas();
    });

    // Right-click preview for container blocks.
    el.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      e.stopPropagation();
      this.showBlockPreview(block, e.clientX, e.clientY);
    });

    return el;
  },

  /**
   * Render container-specific configuration controls into the header area.
   * Two-column gets a width preset dropdown, tabs gets add/rename/remove,
   * section gets a title input and collapse toggle.
   */
  renderContainerConfig(container, block, rowIdx, colIdx, blockIdx) {
    switch (block.type) {
      case 'two_column':
        this.renderTwoColConfig(container, block, rowIdx, colIdx, blockIdx);
        break;
      case 'tabs':
        this.renderTabsConfig(container, block, rowIdx, colIdx, blockIdx);
        break;
      case 'section':
        this.renderSectionConfig(container, block, rowIdx, colIdx, blockIdx);
        break;
      // three_column has no config -- always equal widths.
    }
  },

  /** Width preset selector for two_column blocks. */
  renderTwoColConfig(container, block, rowIdx, colIdx, blockIdx) {
    const select = document.createElement('select');
    select.className = 'text-xs border border-indigo-200 dark:border-indigo-700 rounded px-1 py-0.5 bg-white dark:bg-gray-700 text-indigo-700 dark:text-indigo-300 focus:outline-none focus:ring-1 focus:ring-indigo-400';
    select.title = 'Column widths';
    this.twoColPresets.forEach(preset => {
      const opt = document.createElement('option');
      opt.value = `${preset.left}:${preset.right}`;
      opt.textContent = preset.label;
      if (block.config.left_width === preset.left && block.config.right_width === preset.right) {
        opt.selected = true;
      }
      select.appendChild(opt);
    });
    select.addEventListener('change', (e) => {
      e.stopPropagation();
      const [left, right] = e.target.value.split(':').map(Number);
      block.config.left_width = left;
      block.config.right_width = right;
      this.markDirty();
      this.renderCanvas();
    });
    // Prevent drag when interacting with the select.
    select.addEventListener('mousedown', (e) => e.stopPropagation());
    container.appendChild(select);
  },

  /** Tab management controls: add tab button. Rename/remove on individual tabs in body. */
  renderTabsConfig(container, block, rowIdx, colIdx, blockIdx) {
    const addBtn = document.createElement('button');
    addBtn.className = 'text-xs text-indigo-500 dark:text-indigo-400 hover:text-indigo-700 dark:hover:text-indigo-300 px-1.5 py-0.5 border border-indigo-200 dark:border-indigo-700 rounded hover:bg-indigo-50 dark:hover:bg-indigo-900/30 transition-colors';
    addBtn.title = 'Add tab';
    addBtn.innerHTML = '<i class="fa-solid fa-plus text-[10px]"></i> Tab';
    addBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      const tabNum = block.config.tabs.length + 1;
      block.config.tabs.push({ label: `Tab ${tabNum}`, blocks: [] });
      this.markDirty();
      this.renderCanvas();
    });
    addBtn.addEventListener('mousedown', (e) => e.stopPropagation());
    container.appendChild(addBtn);
  },

  /** Section title input and collapse toggle. */
  renderSectionConfig(container, block, rowIdx, colIdx, blockIdx) {
    const input = document.createElement('input');
    input.type = 'text';
    input.value = block.config.title || 'Section';
    input.className = 'text-xs border border-indigo-200 dark:border-indigo-700 rounded px-1.5 py-0.5 bg-white dark:bg-gray-700 text-indigo-700 dark:text-indigo-300 w-28 focus:outline-none focus:ring-1 focus:ring-indigo-400';
    input.title = 'Section title';
    input.addEventListener('change', (e) => {
      e.stopPropagation();
      block.config.title = e.target.value || 'Section';
      this.markDirty();
    });
    input.addEventListener('mousedown', (e) => e.stopPropagation());
    input.addEventListener('keydown', (e) => e.stopPropagation());
    container.appendChild(input);

    const collapseBtn = document.createElement('button');
    const isCollapsed = block.config.collapsed;
    collapseBtn.className = 'text-xs px-1.5 py-0.5 border border-indigo-200 dark:border-indigo-700 rounded hover:bg-indigo-50 dark:hover:bg-indigo-900/30 transition-colors ' +
      (isCollapsed ? 'text-gray-400 dark:text-gray-500' : 'text-indigo-500 dark:text-indigo-400');
    collapseBtn.title = isCollapsed ? 'Default: collapsed' : 'Default: expanded';
    collapseBtn.innerHTML = isCollapsed
      ? '<i class="fa-solid fa-chevron-right text-[10px]"></i>'
      : '<i class="fa-solid fa-chevron-down text-[10px]"></i>';
    collapseBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      block.config.collapsed = !block.config.collapsed;
      this.markDirty();
      this.renderCanvas();
    });
    collapseBtn.addEventListener('mousedown', (e) => e.stopPropagation());
    container.appendChild(collapseBtn);
  },

  /**
   * Render the body content of a container block, including sub-block
   * drop zones for each slot (columns, tabs, or section content).
   */
  renderContainerBody(body, block, rowIdx, colIdx, blockIdx) {
    switch (block.type) {
      case 'two_column':
        this.renderTwoColBody(body, block, rowIdx, colIdx, blockIdx);
        break;
      case 'three_column':
        this.renderThreeColBody(body, block, rowIdx, colIdx, blockIdx);
        break;
      case 'tabs':
        this.renderTabsBody(body, block, rowIdx, colIdx, blockIdx);
        break;
      case 'section':
        this.renderSectionBody(body, block, rowIdx, colIdx, blockIdx);
        break;
    }
  },

  /** Render two side-by-side drop zones for a two_column block. */
  renderTwoColBody(body, block, rowIdx, colIdx, blockIdx) {
    const leftW = block.config.left_width || 6;
    const rightW = block.config.right_width || 6;
    const grid = document.createElement('div');
    grid.className = 'grid gap-2';
    grid.style.gridTemplateColumns = `${leftW}fr ${rightW}fr`;

    const leftZone = this.createSubBlockZone(
      block.config.left || [], `${leftW}/12`,
      rowIdx, colIdx, blockIdx, 'left'
    );
    const rightZone = this.createSubBlockZone(
      block.config.right || [], `${rightW}/12`,
      rowIdx, colIdx, blockIdx, 'right'
    );

    grid.appendChild(leftZone);
    grid.appendChild(rightZone);
    body.appendChild(grid);
  },

  /** Render three equal drop zones for a three_column block. */
  renderThreeColBody(body, block, rowIdx, colIdx, blockIdx) {
    const widths = block.config.widths || [4, 4, 4];
    if (!block.config.columns) block.config.columns = [[], [], []];
    const grid = document.createElement('div');
    grid.className = 'grid gap-2';
    grid.style.gridTemplateColumns = widths.map(w => `${w}fr`).join(' ');

    widths.forEach((w, i) => {
      const zone = this.createSubBlockZone(
        block.config.columns[i] || [], `${w}/12`,
        rowIdx, colIdx, blockIdx, `col_${i}`
      );
      grid.appendChild(zone);
    });

    body.appendChild(grid);
  },

  /** Render tabbed interface with tab headers and switchable content panes. */
  renderTabsBody(body, block, rowIdx, colIdx, blockIdx) {
    if (!block.config.tabs || block.config.tabs.length === 0) {
      block.config.tabs = [{ label: 'Tab 1', blocks: [] }];
    }

    // Track active tab index on the block config for UI state.
    if (block.config._activeTab === undefined) block.config._activeTab = 0;
    if (block.config._activeTab >= block.config.tabs.length) block.config._activeTab = 0;

    // Tab header bar.
    const tabBar = document.createElement('div');
    tabBar.className = 'flex items-center border-b border-gray-200 dark:border-gray-600 mb-2 gap-0.5';

    block.config.tabs.forEach((tab, tabIdx) => {
      const isActive = tabIdx === block.config._activeTab;
      const tabBtn = document.createElement('div');
      tabBtn.className = 'flex items-center gap-1 px-3 py-1.5 text-xs font-medium rounded-t cursor-pointer border border-b-0 transition-colors ' +
        (isActive
          ? 'bg-white dark:bg-gray-800 text-indigo-700 dark:text-indigo-300 border-gray-200 dark:border-gray-600 -mb-px'
          : 'bg-gray-50 dark:bg-gray-700 text-gray-500 dark:text-gray-400 border-transparent hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-600');

      // Editable tab label.
      const labelSpan = document.createElement('span');
      labelSpan.textContent = tab.label;
      labelSpan.className = 'cursor-pointer';
      labelSpan.title = 'Click to select, double-click to rename';
      labelSpan.addEventListener('click', (e) => {
        e.stopPropagation();
        block.config._activeTab = tabIdx;
        this.renderCanvas();
      });
      labelSpan.addEventListener('dblclick', (e) => {
        e.stopPropagation();
        const newLabel = prompt('Tab label:', tab.label);
        if (newLabel !== null && newLabel.trim() !== '') {
          tab.label = newLabel.trim();
          this.markDirty();
          this.renderCanvas();
        }
      });
      labelSpan.addEventListener('mousedown', (e) => e.stopPropagation());
      tabBtn.appendChild(labelSpan);

      // Remove tab button (only if more than one tab).
      if (block.config.tabs.length > 1) {
        const removeBtn = document.createElement('button');
        removeBtn.className = 'text-gray-300 dark:text-gray-500 hover:text-red-500 dark:hover:text-red-400 transition-colors ml-1';
        removeBtn.title = 'Remove tab';
        removeBtn.innerHTML = '<i class="fa-solid fa-xmark text-[10px]"></i>';
        removeBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          block.config.tabs.splice(tabIdx, 1);
          if (block.config._activeTab >= block.config.tabs.length) {
            block.config._activeTab = block.config.tabs.length - 1;
          }
          this.markDirty();
          this.renderCanvas();
        });
        removeBtn.addEventListener('mousedown', (e) => e.stopPropagation());
        tabBtn.appendChild(removeBtn);
      }

      tabBar.appendChild(tabBtn);
    });

    body.appendChild(tabBar);

    // Active tab content pane.
    const activeTab = block.config.tabs[block.config._activeTab];
    if (!activeTab.blocks) activeTab.blocks = [];
    const pane = this.createSubBlockZone(
      activeTab.blocks, null,
      rowIdx, colIdx, blockIdx, `tab_${block.config._activeTab}`
    );
    body.appendChild(pane);
  },

  /** Render collapsible section with title bar and content drop zone. */
  renderSectionBody(body, block, rowIdx, colIdx, blockIdx) {
    if (!block.config.blocks) block.config.blocks = [];

    // Section title preview.
    const titleBar = document.createElement('div');
    titleBar.className = 'flex items-center gap-2 px-2 py-1.5 bg-gray-50 dark:bg-gray-700 rounded mb-2 text-sm text-gray-600 dark:text-gray-300';
    const collapseIcon = block.config.collapsed ? 'fa-chevron-right' : 'fa-chevron-down';
    titleBar.innerHTML = `
      <i class="fa-solid ${collapseIcon} text-xs text-gray-400 dark:text-gray-500"></i>
      <span class="font-medium">${block.config.title || 'Section'}</span>
      <span class="text-[10px] text-gray-400 dark:text-gray-500 ml-auto">${block.config.collapsed ? 'collapsed by default' : 'expanded by default'}</span>
    `;
    body.appendChild(titleBar);

    // Content drop zone.
    const zone = this.createSubBlockZone(
      block.config.blocks, null,
      rowIdx, colIdx, blockIdx, 'content'
    );
    body.appendChild(zone);
  },

  /**
   * Create a drop zone element for sub-blocks within a container.
   * Handles drag-and-drop, rendering of existing sub-blocks, and
   * drop indicators within the zone.
   *
   * @param {Array} blocks - Reference to the sub-block array in the config.
   * @param {string|null} label - Optional label shown in the zone header.
   * @param {number} rowIdx - Parent row index.
   * @param {number} colIdx - Parent column index.
   * @param {number} blockIdx - Parent container block index.
   * @param {string} slot - Slot identifier within the container (e.g. 'left', 'right', 'tab_0').
   */
  createSubBlockZone(blocks, label, rowIdx, colIdx, blockIdx, slot) {
    const zone = document.createElement('div');
    zone.className = 'te-subzone border-2 border-dashed border-gray-200 dark:border-gray-600 rounded min-h-[48px] p-1.5 transition-colors relative';
    zone.dataset.containerRow = rowIdx;
    zone.dataset.containerCol = colIdx;
    zone.dataset.containerBlock = blockIdx;
    zone.dataset.containerSlot = slot;

    if (label) {
      const lbl = document.createElement('div');
      lbl.className = 'text-[9px] text-gray-300 dark:text-gray-600 font-mono mb-0.5 px-0.5';
      lbl.textContent = label;
      zone.appendChild(lbl);
    }

    // Render existing sub-blocks.
    blocks.forEach((subBlock, subIdx) => {
      const subEl = this.renderSubBlock(subBlock, rowIdx, colIdx, blockIdx, slot, subIdx);
      zone.appendChild(subEl);
    });

    // Empty state hint.
    if (blocks.length === 0) {
      const hint = document.createElement('div');
      hint.className = 'te-subzone-hint text-[10px] text-gray-300 dark:text-gray-600 text-center py-2 italic';
      hint.textContent = 'Drop blocks here';
      zone.appendChild(hint);
    }

    // Drag-and-drop events for the sub-block zone.
    zone.addEventListener('dragover', (e) => {
      e.preventDefault();
      e.stopPropagation();
      e.dataTransfer.dropEffect = 'move';
      zone.classList.add('border-indigo-400', 'bg-indigo-50/30');
      this.updateSubZoneDropIndicator(e, zone, rowIdx, colIdx, blockIdx, slot);
    });
    zone.addEventListener('dragleave', (e) => {
      if (!zone.contains(e.relatedTarget)) {
        zone.classList.remove('border-indigo-400', 'bg-indigo-50/30');
        this.clearDropIndicator();
      }
    });
    zone.addEventListener('drop', (e) => {
      e.preventDefault();
      e.stopPropagation();
      zone.classList.remove('border-indigo-400', 'bg-indigo-50/30');
      const insertIdx = this.dropTarget ? this.dropTarget.insertIdx : blocks.length;
      this.clearDropIndicator();
      this.handleSubBlockDrop(e, rowIdx, colIdx, blockIdx, slot, insertIdx);
    });

    return zone;
  },

  /** Render a single sub-block inside a container zone. */
  renderSubBlock(subBlock, rowIdx, colIdx, blockIdx, slot, subIdx) {
    const bt = this.blockTypes.find(b => b.type === subBlock.type) || { label: subBlock.type, icon: 'fa-cube' };
    const el = document.createElement('div');
    el.className = 'te-sub-block flex items-center gap-1.5 px-2 py-1 mb-0.5 bg-gray-50 dark:bg-gray-700 border border-gray-200 dark:border-gray-600 rounded text-xs group/sub cursor-grab hover:border-indigo-300 dark:hover:border-indigo-500 transition-colors';
    el.draggable = true;
    el.dataset.subIdx = subIdx;
    el.innerHTML = `
      <i class="fa-solid fa-grip-vertical text-gray-300 dark:text-gray-500 text-[10px]"></i>
      <i class="fa-solid ${bt.icon} w-3 text-gray-400 dark:text-gray-500 text-center text-[10px]"></i>
      <span class="font-medium text-gray-600 dark:text-gray-300 flex-1">${bt.label}</span>
      <button class="te-sub-del opacity-0 group-hover/sub:opacity-100 text-gray-300 dark:text-gray-500 hover:text-red-500 dark:hover:text-red-400 transition-all p-0.5" title="Remove">
        <i class="fa-solid fa-xmark text-[10px]"></i>
      </button>
    `;

    // Drag sub-block within or between zones.
    el.addEventListener('dragstart', (e) => {
      e.stopPropagation();
      e.dataTransfer.setData('text/plain', JSON.stringify({
        source: 'subblock',
        rowIdx, colIdx, blockIdx, slot, subIdx,
        block: subBlock,
      }));
      e.dataTransfer.effectAllowed = 'move';
      el.classList.add('opacity-50');
    });
    el.addEventListener('dragend', () => {
      el.classList.remove('opacity-50');
      this.clearDropIndicator();
    });

    // Delete sub-block.
    el.querySelector('.te-sub-del').addEventListener('click', (e) => {
      e.stopPropagation();
      const subBlocks = this.getSubBlockArray(rowIdx, colIdx, blockIdx, slot);
      if (subBlocks) {
        subBlocks.splice(subIdx, 1);
        this.markDirty();
        this.renderCanvas();
      }
    });

    return el;
  },

  /**
   * Resolve the sub-block array reference for a given container slot.
   * Returns the array of blocks stored in the container's config at the
   * specified slot (e.g. 'left', 'right', 'col_0', 'tab_1', 'content').
   */
  getSubBlockArray(rowIdx, colIdx, blockIdx, slot) {
    const block = this.layout.rows[rowIdx].columns[colIdx].blocks[blockIdx];
    if (!block || !block.config) return null;

    switch (block.type) {
      case 'two_column':
        if (slot === 'left') return block.config.left;
        if (slot === 'right') return block.config.right;
        return null;
      case 'three_column':
        if (slot.startsWith('col_')) {
          const i = parseInt(slot.split('_')[1], 10);
          return block.config.columns?.[i] || null;
        }
        return null;
      case 'tabs':
        if (slot.startsWith('tab_')) {
          const i = parseInt(slot.split('_')[1], 10);
          return block.config.tabs?.[i]?.blocks || null;
        }
        return null;
      case 'section':
        if (slot === 'content') return block.config.blocks;
        return null;
      default:
        return null;
    }
  },

  /** Compute drop indicator position within a sub-block zone. */
  updateSubZoneDropIndicator(e, zoneEl, rowIdx, colIdx, blockIdx, slot) {
    const subEls = zoneEl.querySelectorAll('.te-sub-block');
    let insertIdx = subEls.length;
    let referenceEl = null;

    for (let i = 0; i < subEls.length; i++) {
      const rect = subEls[i].getBoundingClientRect();
      const midY = rect.top + rect.height / 2;
      if (e.clientY < midY) {
        insertIdx = i;
        referenceEl = subEls[i];
        break;
      }
    }

    // Only update if position changed.
    if (this.dropTarget &&
        this.dropTarget.containerBlock === blockIdx &&
        this.dropTarget.slot === slot &&
        this.dropTarget.insertIdx === insertIdx) {
      return;
    }

    this.clearDropIndicator();
    this.dropTarget = { rowIdx, colIdx, containerBlock: blockIdx, slot, insertIdx };

    const indicator = document.createElement('div');
    indicator.className = 'te-drop-indicator';
    indicator.style.cssText = 'height: 2px; background: #6366f1; border-radius: 1px; margin: 1px 2px; transition: opacity 0.15s ease; opacity: 0; position: relative;';
    indicator.innerHTML = '<div style="position:absolute;inset:-1px 0;background:#6366f1;opacity:0.2;border-radius:3px;animation:te-pulse 1s ease-in-out infinite"></div>';

    if (referenceEl) {
      zoneEl.insertBefore(indicator, referenceEl);
    } else {
      zoneEl.appendChild(indicator);
    }

    requestAnimationFrame(() => { indicator.style.opacity = '1'; });
    this.dropIndicator = indicator;
  },

  /**
   * Handle a drop event inside a container sub-block zone.
   * Supports drops from the palette, from the main canvas, and from other sub-block zones.
   */
  handleSubBlockDrop(e, targetRowIdx, targetColIdx, targetBlockIdx, targetSlot, insertIdx) {
    let data;
    try {
      data = JSON.parse(e.dataTransfer.getData('text/plain'));
    } catch { return; }

    const targetBlocks = this.getSubBlockArray(targetRowIdx, targetColIdx, targetBlockIdx, targetSlot);
    if (!targetBlocks) return;

    // Do not allow dropping container blocks inside other containers (prevents nesting).
    const dropType = data.type || (data.block && data.block.type);
    if (dropType && this.isContainer(dropType)) return;

    if (data.source === 'palette') {
      // New block from palette.
      const newBlock = { id: this.uid('blk'), type: data.type, config: {} };
      targetBlocks.splice(insertIdx, 0, newBlock);
    } else if (data.source === 'subblock') {
      // Moving between sub-block zones.
      const srcBlocks = this.getSubBlockArray(data.rowIdx, data.colIdx, data.blockIdx, data.slot);
      if (!srcBlocks) return;

      const sameZone = data.rowIdx === targetRowIdx &&
                       data.colIdx === targetColIdx &&
                       data.blockIdx === targetBlockIdx &&
                       data.slot === targetSlot;

      srcBlocks.splice(data.subIdx, 1);
      let adjustedIdx = insertIdx;
      if (sameZone && data.subIdx < insertIdx) adjustedIdx--;
      targetBlocks.splice(adjustedIdx, 0, data.block);
    } else if (data.source === 'canvas') {
      // Moving a top-level block into a container zone.
      // Do not allow moving container blocks into sub-zones.
      if (data.block && this.isContainer(data.block.type)) return;
      const srcBlocks = this.layout.rows[data.rowIdx].columns[data.colIdx].blocks;
      srcBlocks.splice(data.blockIdx, 1);
      targetBlocks.splice(insertIdx, 0, data.block);
    }

    this.markDirty();
    this.renderCanvas();
  },

  handleDrop(e, targetRowIdx, targetColIdx, insertIdx) {
    let data;
    try {
      data = JSON.parse(e.dataTransfer.getData('text/plain'));
    } catch { return; }

    if (data.source === 'palette') {
      // Add new block from palette at the indicated position.
      // Container blocks get their default config with sub-block arrays.
      const config = this.defaultBlockConfig(data.type);
      const block = { id: this.uid('blk'), type: data.type, config };
      this.layout.rows[targetRowIdx].columns[targetColIdx].blocks.splice(insertIdx, 0, block);
    } else if (data.source === 'canvas') {
      // Moving within the same column -- adjust index if moving down.
      const sameCol = data.rowIdx === targetRowIdx && data.colIdx === targetColIdx;
      const srcBlocks = this.layout.rows[data.rowIdx].columns[data.colIdx].blocks;
      // Remove from source first.
      srcBlocks.splice(data.blockIdx, 1);
      // If same column and the source was above the target, adjust index.
      let adjustedIdx = insertIdx;
      if (sameCol && data.blockIdx < insertIdx) {
        adjustedIdx--;
      }
      this.layout.rows[targetRowIdx].columns[targetColIdx].blocks.splice(adjustedIdx, 0, data.block);
    } else if (data.source === 'subblock') {
      // Moving a sub-block out of a container into the main canvas.
      const srcBlocks = this.getSubBlockArray(data.rowIdx, data.colIdx, data.blockIdx, data.slot);
      if (srcBlocks) {
        srcBlocks.splice(data.subIdx, 1);
        this.layout.rows[targetRowIdx].columns[targetColIdx].blocks.splice(insertIdx, 0, data.block);
      }
    }

    this.markDirty();
    this.renderCanvas();
  },

  addRow(widths) {
    const rowId = this.uid('row');
    const columns = widths.map((w) => ({
      id: this.uid('col'),
      width: w,
      blocks: [],
    }));
    this.layout.rows.push({ id: rowId, columns });
    this.markDirty();
    this.renderCanvas();
  },

  changeRowLayout(rowIdx, widths) {
    const row = this.layout.rows[rowIdx];
    const allBlocks = row.columns.flatMap(c => c.blocks);

    // Redistribute blocks: put them all in the first column.
    row.columns = widths.map((w, i) => ({
      id: row.columns[i]?.id || this.uid('col'),
      width: w,
      blocks: i === 0 ? allBlocks : [],
    }));

    this.markDirty();
    this.renderCanvas();
  },

  deleteRow(rowIdx) {
    this.layout.rows.splice(rowIdx, 1);
    this.markDirty();
    this.renderCanvas();
  },

  moveRow(rowIdx, direction) {
    const newIdx = rowIdx + direction;
    if (newIdx < 0 || newIdx >= this.layout.rows.length) return;
    const rows = this.layout.rows;
    [rows[rowIdx], rows[newIdx]] = [rows[newIdx], rows[rowIdx]];
    this.markDirty();
    this.renderCanvas();
  },

  // Find the save button, scoping to the nearest [data-te-container] first
  // so HTMX-swapped fragments use their own button rather than a stale global.
  findSaveBtn() {
    const container = this.el.closest('[data-te-container]');
    return (container && container.querySelector('#te-save-btn'))
        || document.getElementById('te-save-btn');
  },

  // Find the save status element with the same scoping strategy.
  findSaveStatus() {
    const container = this.el.closest('[data-te-container]');
    return (container && container.querySelector('#te-save-status'))
        || document.getElementById('te-save-status');
  },

  markDirty() {
    this.dirty = true;
    const status = this.findSaveStatus();
    if (status) status.textContent = 'Unsaved changes';
    const btn = this.findSaveBtn();
    if (btn) btn.classList.add('animate-pulse');
  },

  bindSave() {
    const btn = this.findSaveBtn();
    if (btn) {
      btn.addEventListener('click', () => this.save());
    }

    // Ctrl+S / Cmd+S shortcut. Store reference for cleanup in destroy().
    this.keydownHandler = (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        this.save();
      }
    };
    document.addEventListener('keydown', this.keydownHandler);
  },

  /**
   * Strip transient UI-only properties (prefixed with _) from the layout
   * before saving. These are used for editor state (e.g. _activeTab) and
   * should not be persisted to the database.
   */
  cleanLayoutForSave(layout) {
    return JSON.parse(JSON.stringify(layout, (key, value) => {
      if (key.startsWith('_')) return undefined;
      return value;
    }));
  },

  async save() {
    const btn = this.findSaveBtn();
    const status = this.findSaveStatus();
    if (btn) {
      btn.disabled = true;
      btn.textContent = 'Saving...';
      btn.classList.remove('animate-pulse');
    }

    try {
      // Strip transient UI state before sending to the server.
      const cleanLayout = this.cleanLayoutForSave(this.layout);
      const res = await fetch(this.endpoint, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': this.csrfToken,
        },
        body: JSON.stringify({ layout: cleanLayout }),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.message || 'Failed to save');
      }

      this.dirty = false;
      if (status) status.textContent = 'Saved';
      setTimeout(() => { if (status && !this.dirty) status.textContent = ''; }, 2000);
    } catch (err) {
      if (status) status.textContent = 'Error: ' + err.message;
      if (status) status.classList.add('text-red-500');
      setTimeout(() => {
        if (status) { status.textContent = ''; status.classList.remove('text-red-500'); }
      }, 4000);
    } finally {
      if (btn) {
        btn.disabled = false;
        btn.textContent = 'Save Template';
      }
    }
  },

  // Clean up when HTMX swaps this widget out (called by boot.js destroyElement).
  destroy(el) {
    if (this.keydownHandler) {
      document.removeEventListener('keydown', this.keydownHandler);
      this.keydownHandler = null;
    }
    el.innerHTML = '';
    this.layout = null;
    this.canvas = null;
    this.fields = null;
    this.el = null;
  },
});

// Inject styles for drop indicator animations and container block visuals.
if (!document.getElementById('te-styles')) {
  const style = document.createElement('style');
  style.id = 'te-styles';
  style.textContent = [
    '@keyframes te-pulse { 0%, 100% { opacity: 0.2; } 50% { opacity: 0.4; } }',
    // Container blocks should not shrink their sub-block zones on drag.
    '.te-container-block .te-subzone { min-height: 36px; }',
    // Subtle background tint for container zones so they stand out from the column bg.
    '.te-container-block .te-subzone { background: rgba(249,250,251,0.5); }',
    '.dark .te-container-block .te-subzone { background: rgba(55,65,81,0.3); }',
    // Hide the empty-state hint when a drop indicator is showing.
    '.te-subzone.border-indigo-400 .te-subzone-hint { display: none; }',
    // Prevent container block drag handle from interfering with child interactions.
    '.te-container-block > .p-2 { cursor: default; }',
  ].join('\n');
  document.head.appendChild(style);
}
