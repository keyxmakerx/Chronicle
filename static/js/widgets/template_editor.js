/**
 * Template Editor Widget
 *
 * Visual drag-and-drop page template editor for entity types.
 * Uses a 12-column grid system with rows, columns, and blocks.
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

    // Ensure layout has rows.
    if (!this.layout.rows || this.layout.rows.length === 0) {
      this.layout = this.defaultLayout();
    }

    this.render();
    this.bindSave();
  },

  /** Available block types that can be dragged from the palette. */
  blockTypes: [
    { type: 'title',      label: 'Title',      icon: 'fa-heading',    desc: 'Entity name and actions' },
    { type: 'image',      label: 'Image',       icon: 'fa-image',      desc: 'Header image with upload' },
    { type: 'entry',      label: 'Rich Text',   icon: 'fa-align-left', desc: 'Main content editor' },
    { type: 'attributes', label: 'Attributes',  icon: 'fa-list',       desc: 'Custom field values' },
    { type: 'details',    label: 'Details',     icon: 'fa-info-circle', desc: 'Metadata and dates' },
    { type: 'divider',    label: 'Divider',     icon: 'fa-minus',      desc: 'Horizontal separator' },
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

  render() {
    this.el.innerHTML = '';
    this.el.className = 'flex h-full overflow-hidden';

    // Palette panel.
    const palette = document.createElement('div');
    palette.className = 'w-56 bg-gray-50 border-r border-gray-200 p-4 overflow-y-auto shrink-0';
    palette.innerHTML = `
      <h3 class="text-xs font-semibold uppercase tracking-wider text-gray-500 mb-3">Components</h3>
    `;
    this.blockTypes.forEach(bt => {
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
      item.addEventListener('dragend', () => item.classList.remove('opacity-50'));
      palette.appendChild(item);
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
        colEl.className = 'bg-white border-2 border-dashed border-gray-200 rounded-lg min-h-[80px] p-2 transition-colors';
        colEl.dataset.rowIdx = rowIdx;
        colEl.dataset.colIdx = colIdx;

        // Column header.
        const colHeader = document.createElement('div');
        colHeader.className = 'text-[10px] text-gray-300 font-mono mb-1 px-1';
        colHeader.textContent = `${col.width}/12`;
        colEl.appendChild(colHeader);

        // Render blocks.
        col.blocks.forEach((block, blockIdx) => {
          const blockEl = this.renderBlock(block, rowIdx, colIdx, blockIdx);
          colEl.appendChild(blockEl);
        });

        // Drop zone setup.
        colEl.addEventListener('dragover', (e) => {
          e.preventDefault();
          e.dataTransfer.dropEffect = 'move';
          colEl.classList.add('border-indigo-400', 'bg-indigo-50');
        });
        colEl.addEventListener('dragleave', (e) => {
          if (!colEl.contains(e.relatedTarget)) {
            colEl.classList.remove('border-indigo-400', 'bg-indigo-50');
          }
        });
        colEl.addEventListener('drop', (e) => {
          e.preventDefault();
          colEl.classList.remove('border-indigo-400', 'bg-indigo-50');
          this.handleDrop(e, rowIdx, colIdx);
        });

        grid.appendChild(colEl);
      });

      rowEl.appendChild(grid);
      this.canvas.appendChild(rowEl);
    });
  },

  renderBlock(block, rowIdx, colIdx, blockIdx) {
    const bt = this.blockTypes.find(b => b.type === block.type) || { label: block.type, icon: 'fa-cube' };
    const el = document.createElement('div');
    el.className = 'flex items-center gap-2 px-3 py-2 mb-1 bg-gray-50 border border-gray-200 rounded group/block cursor-grab hover:border-indigo-300 transition-colors';
    el.draggable = true;
    el.innerHTML = `
      <i class="fa-solid fa-grip-vertical text-gray-300 text-xs"></i>
      <i class="fa-solid ${bt.icon} w-4 text-gray-400 text-center text-sm"></i>
      <span class="text-sm font-medium text-gray-700 flex-1">${bt.label}</span>
      <button class="te-block-del opacity-0 group-hover/block:opacity-100 text-gray-300 hover:text-red-500 transition-all p-0.5" title="Remove">
        <i class="fa-solid fa-xmark text-xs"></i>
      </button>
    `;

    // Drag from canvas (move).
    el.addEventListener('dragstart', (e) => {
      e.dataTransfer.setData('text/plain', JSON.stringify({
        source: 'canvas',
        rowIdx, colIdx, blockIdx,
        block,
      }));
      e.dataTransfer.effectAllowed = 'move';
      el.classList.add('opacity-50');
    });
    el.addEventListener('dragend', () => el.classList.remove('opacity-50'));

    // Delete block.
    el.querySelector('.te-block-del').addEventListener('click', (e) => {
      e.stopPropagation();
      this.layout.rows[rowIdx].columns[colIdx].blocks.splice(blockIdx, 1);
      this.markDirty();
      this.renderCanvas();
    });

    return el;
  },

  handleDrop(e, targetRowIdx, targetColIdx) {
    let data;
    try {
      data = JSON.parse(e.dataTransfer.getData('text/plain'));
    } catch { return; }

    if (data.source === 'palette') {
      // Add new block from palette.
      const block = { id: this.uid('blk'), type: data.type, config: {} };
      this.layout.rows[targetRowIdx].columns[targetColIdx].blocks.push(block);
    } else if (data.source === 'canvas') {
      // Move existing block.
      const srcBlocks = this.layout.rows[data.rowIdx].columns[data.colIdx].blocks;
      srcBlocks.splice(data.blockIdx, 1);
      this.layout.rows[targetRowIdx].columns[targetColIdx].blocks.push(data.block);
    }

    this.markDirty();
    this.renderCanvas();
  },

  addRow(widths) {
    const rowId = this.uid('row');
    const columns = widths.map((w, i) => ({
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

  async save() {
    const btn = document.getElementById('te-save-btn');
    const status = document.getElementById('te-save-status');
    if (btn) {
      btn.disabled = true;
      btn.textContent = 'Saving...';
      btn.classList.remove('animate-pulse');
    }

    try {
      const res = await fetch(this.endpoint, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ layout: this.layout }),
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
