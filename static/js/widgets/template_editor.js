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
    { type: 'divider',      label: 'Divider',       icon: 'fa-minus',         desc: 'Horizontal separator' },
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
    item.className = 'flex items-center gap-2 px-3 py-2 mb-1 bg-white border border-gray-200 rounded-md cursor-grab hover:border-indigo-300 hover:shadow-sm transition-all text-sm';
    item.draggable = true;
    item.innerHTML = `
      <i class="fa-solid ${bt.icon} w-4 text-gray-400 text-center"></i>
      <div>
        <div class="font-medium text-gray-700">${bt.label}</div>
        <div class="text-[10px] text-gray-400">${bt.desc}</div>
      </div>
    `;
    item.addEventListener('dragstart', (e) => {
      e.dataTransfer.setData('text/plain', JSON.stringify({ source: 'palette', type: bt.type }));
      e.dataTransfer.effectAllowed = 'copy';
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
    palette.className = 'w-56 bg-gray-50 border-r border-gray-200 p-4 overflow-y-auto shrink-0';
    palette.innerHTML = `
      <h3 class="text-xs font-semibold uppercase tracking-wider text-gray-500 mb-3">Components</h3>
    `;

    // Separate content blocks from layout/container blocks for grouped display.
    const contentBlocks = this.blockTypes.filter(bt => !bt.container);
    const layoutBlocks = this.blockTypes.filter(bt => bt.container);

    contentBlocks.forEach(bt => {
      palette.appendChild(this.createPaletteItem(bt));
    });

    // Layout blocks section header.
    const layoutHeader = document.createElement('h3');
    layoutHeader.className = 'text-xs font-semibold uppercase tracking-wider text-gray-500 mb-3 mt-5';
    layoutHeader.textContent = 'Layout Blocks';
    palette.appendChild(layoutHeader);

    layoutBlocks.forEach(bt => {
      palette.appendChild(this.createPaletteItem(bt));
    });

    // Row presets section.
    const presetSection = document.createElement('div');
    presetSection.className = 'mt-6';
    presetSection.innerHTML = '<h3 class="text-xs font-semibold uppercase tracking-wider text-gray-500 mb-3">Row Layouts</h3>';
    this.colPresets.forEach(preset => {
      const btn = document.createElement('button');
      btn.className = 'flex items-center gap-2 w-full px-3 py-2 mb-1 bg-white border border-gray-200 rounded-md hover:border-indigo-300 hover:shadow-sm transition-all text-sm text-left';
      const preview = preset.widths.map(w => {
        const pct = Math.round(w / 12 * 100);
        return `<div class="h-3 bg-gray-300 rounded-sm" style="width:${pct}%"></div>`;
      }).join('<div class="w-0.5"></div>');
      btn.innerHTML = `
        <div class="flex gap-0.5 w-12 shrink-0">${preview}</div>
        <span class="text-gray-700">${preset.label}</span>
      `;
      btn.addEventListener('click', () => this.addRow(preset.widths));
      presetSection.appendChild(btn);
    });
    palette.appendChild(presetSection);

    this.el.appendChild(palette);

    // Canvas area.
    const canvas = document.createElement('div');
    canvas.className = 'flex-1 overflow-y-auto p-6 bg-gray-100';
    this.canvas = canvas;
    this.renderCanvas();
    this.el.appendChild(canvas);
  },

  renderCanvas() {
    this.canvas.innerHTML = '';

    if (this.layout.rows.length === 0) {
      this.canvas.innerHTML = `
        <div class="flex flex-col items-center justify-center h-full text-gray-400">
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
        <span class="text-[10px] text-gray-400 font-mono">Row ${rowIdx + 1}</span>
        <div class="flex-1"></div>
      `;

      // Column layout picker for this row.
      this.colPresets.forEach(preset => {
        const btn = document.createElement('button');
        btn.className = 'p-1 hover:bg-gray-200 rounded transition-colors';
        btn.title = preset.label;
        const isActive = JSON.stringify(row.columns.map(c => c.width)) === JSON.stringify(preset.widths);
        const preview = preset.widths.map(w => {
          const pct = Math.round(w / 12 * 100);
          const color = isActive ? 'bg-indigo-400' : 'bg-gray-300';
          return `<div class="h-2 ${color} rounded-sm" style="width:${pct}%"></div>`;
        }).join('<div class="w-px"></div>');
        btn.innerHTML = `<div class="flex gap-px w-8">${preview}</div>`;
        btn.addEventListener('click', () => this.changeRowLayout(rowIdx, preset.widths));
        toolbar.appendChild(btn);
      });

      // Delete row button.
      const delBtn = document.createElement('button');
      delBtn.className = 'p-1 text-gray-300 hover:text-red-500 transition-colors ml-1';
      delBtn.title = 'Delete row';
      delBtn.innerHTML = '<i class="fa-solid fa-trash-can text-xs"></i>';
      delBtn.addEventListener('click', () => this.deleteRow(rowIdx));
      toolbar.appendChild(delBtn);

      // Move buttons.
      if (rowIdx > 0) {
        const upBtn = document.createElement('button');
        upBtn.className = 'p-1 text-gray-300 hover:text-gray-600 transition-colors';
        upBtn.title = 'Move up';
        upBtn.innerHTML = '<i class="fa-solid fa-chevron-up text-xs"></i>';
        upBtn.addEventListener('click', () => this.moveRow(rowIdx, -1));
        toolbar.appendChild(upBtn);
      }
      if (rowIdx < this.layout.rows.length - 1) {
        const downBtn = document.createElement('button');
        downBtn.className = 'p-1 text-gray-300 hover:text-gray-600 transition-colors';
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
        colEl.className = 'te-column bg-white border-2 border-dashed border-gray-200 rounded-lg min-h-[80px] p-2 transition-colors relative';
        colEl.dataset.rowIdx = rowIdx;
        colEl.dataset.colIdx = colIdx;

        // Column header.
        const colHeader = document.createElement('div');
        colHeader.className = 'text-[10px] text-gray-300 font-mono mb-1 px-1';
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

    const bt = this.blockTypes.find(b => b.type === block.type) || { label: block.type, icon: 'fa-cube' };
    const el = document.createElement('div');
    el.className = 'te-block flex items-center gap-2 px-3 py-2 mb-1 bg-gray-50 border border-gray-200 rounded group/block cursor-grab hover:border-indigo-300 transition-colors';
    el.draggable = true;
    el.dataset.blockIdx = blockIdx;
    el.innerHTML = `
      <i class="fa-solid fa-grip-vertical text-gray-300 text-xs"></i>
      <i class="fa-solid ${bt.icon} w-4 text-gray-400 text-center text-sm"></i>
      <span class="text-sm font-medium text-gray-700 flex-1">${bt.label}</span>
      <button class="te-block-del opacity-0 group-hover/block:opacity-100 text-gray-300 hover:text-red-500 transition-all p-0.5" title="Remove">
        <i class="fa-solid fa-xmark text-xs"></i>
      </button>
    `;

    this.bindBlockDrag(el, block, rowIdx, colIdx, blockIdx);
    this.bindBlockDelete(el, rowIdx, colIdx, blockIdx);

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
   * Render a container block (two_column, three_column, tabs, section).
   * Container blocks display an expanded visual with drop zones for sub-blocks,
   * plus configuration controls for their layout properties.
   */
  renderContainerBlock(block, rowIdx, colIdx, blockIdx) {
    const bt = this.blockTypes.find(b => b.type === block.type) || { label: block.type, icon: 'fa-cube' };
    const el = document.createElement('div');
    el.className = 'te-block te-container-block mb-1 border border-gray-300 rounded-lg bg-white overflow-hidden group/block';
    el.draggable = true;
    el.dataset.blockIdx = blockIdx;

    // Container header bar with drag handle, label, config, and delete.
    const header = document.createElement('div');
    header.className = 'flex items-center gap-2 px-3 py-2 bg-indigo-50 border-b border-indigo-100 cursor-grab';
    header.innerHTML = `
      <i class="fa-solid fa-grip-vertical text-indigo-300 text-xs"></i>
      <i class="fa-solid ${bt.icon} w-4 text-indigo-400 text-center text-sm"></i>
      <span class="text-sm font-semibold text-indigo-700 flex-1">${bt.label}</span>
    `;

    // Config controls specific to the container type (inserted into header).
    const configArea = document.createElement('div');
    configArea.className = 'flex items-center gap-1';
    this.renderContainerConfig(configArea, block, rowIdx, colIdx, blockIdx);

    // Delete button.
    const delBtn = document.createElement('button');
    delBtn.className = 'te-block-del text-indigo-300 hover:text-red-500 transition-all p-0.5 ml-1';
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
    select.className = 'text-xs border border-indigo-200 rounded px-1 py-0.5 bg-white text-indigo-700 focus:outline-none focus:ring-1 focus:ring-indigo-400';
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
    addBtn.className = 'text-xs text-indigo-500 hover:text-indigo-700 px-1.5 py-0.5 border border-indigo-200 rounded hover:bg-indigo-50 transition-colors';
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
    input.className = 'text-xs border border-indigo-200 rounded px-1.5 py-0.5 bg-white text-indigo-700 w-28 focus:outline-none focus:ring-1 focus:ring-indigo-400';
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
    collapseBtn.className = 'text-xs px-1.5 py-0.5 border border-indigo-200 rounded hover:bg-indigo-50 transition-colors ' +
      (isCollapsed ? 'text-gray-400' : 'text-indigo-500');
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
    tabBar.className = 'flex items-center border-b border-gray-200 mb-2 gap-0.5';

    block.config.tabs.forEach((tab, tabIdx) => {
      const isActive = tabIdx === block.config._activeTab;
      const tabBtn = document.createElement('div');
      tabBtn.className = 'flex items-center gap-1 px-3 py-1.5 text-xs font-medium rounded-t cursor-pointer border border-b-0 transition-colors ' +
        (isActive
          ? 'bg-white text-indigo-700 border-gray-200 -mb-px'
          : 'bg-gray-50 text-gray-500 border-transparent hover:text-gray-700 hover:bg-gray-100');

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
        removeBtn.className = 'text-gray-300 hover:text-red-500 transition-colors ml-1';
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
    titleBar.className = 'flex items-center gap-2 px-2 py-1.5 bg-gray-50 rounded mb-2 text-sm text-gray-600';
    const collapseIcon = block.config.collapsed ? 'fa-chevron-right' : 'fa-chevron-down';
    titleBar.innerHTML = `
      <i class="fa-solid ${collapseIcon} text-xs text-gray-400"></i>
      <span class="font-medium">${block.config.title || 'Section'}</span>
      <span class="text-[10px] text-gray-400 ml-auto">${block.config.collapsed ? 'collapsed by default' : 'expanded by default'}</span>
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
    zone.className = 'te-subzone border-2 border-dashed border-gray-200 rounded min-h-[48px] p-1.5 transition-colors relative';
    zone.dataset.containerRow = rowIdx;
    zone.dataset.containerCol = colIdx;
    zone.dataset.containerBlock = blockIdx;
    zone.dataset.containerSlot = slot;

    if (label) {
      const lbl = document.createElement('div');
      lbl.className = 'text-[9px] text-gray-300 font-mono mb-0.5 px-0.5';
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
      hint.className = 'te-subzone-hint text-[10px] text-gray-300 text-center py-2 italic';
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
    el.className = 'te-sub-block flex items-center gap-1.5 px-2 py-1 mb-0.5 bg-gray-50 border border-gray-200 rounded text-xs group/sub cursor-grab hover:border-indigo-300 transition-colors';
    el.draggable = true;
    el.dataset.subIdx = subIdx;
    el.innerHTML = `
      <i class="fa-solid fa-grip-vertical text-gray-300 text-[10px]"></i>
      <i class="fa-solid ${bt.icon} w-3 text-gray-400 text-center text-[10px]"></i>
      <span class="font-medium text-gray-600 flex-1">${bt.label}</span>
      <button class="te-sub-del opacity-0 group-hover/sub:opacity-100 text-gray-300 hover:text-red-500 transition-all p-0.5" title="Remove">
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

  markDirty() {
    this.dirty = true;
    const status = document.getElementById('te-save-status');
    if (status) status.textContent = 'Unsaved changes';
    const btn = document.getElementById('te-save-btn');
    if (btn) btn.classList.add('animate-pulse');
  },

  bindSave() {
    const btn = document.getElementById('te-save-btn');
    if (btn) {
      btn.addEventListener('click', () => this.save());
    }

    // Ctrl+S / Cmd+S shortcut.
    document.addEventListener('keydown', (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        this.save();
      }
    });
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
    const btn = document.getElementById('te-save-btn');
    const status = document.getElementById('te-save-status');
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
    // Hide the empty-state hint when a drop indicator is showing.
    '.te-subzone.border-indigo-400 .te-subzone-hint { display: none; }',
    // Prevent container block drag handle from interfering with child interactions.
    '.te-container-block > .p-2 { cursor: default; }',
  ].join('\n');
  document.head.appendChild(style);
}
