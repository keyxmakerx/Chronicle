/**
 * Dashboard Editor Widget
 *
 * Visual drag-and-drop dashboard layout editor for campaigns and categories.
 * Uses a 12-column grid row/column/block system. Owners can add, remove,
 * reorder, and configure dashboard blocks via a palette panel.
 *
 * Mount: data-widget="dashboard-editor"
 * Config:
 *   data-endpoint    - GET/PUT/DELETE endpoint for dashboard layout JSON
 *   data-campaign-id - Campaign UUID
 *   data-csrf-token  - CSRF token
 *   data-block-types - (optional) JSON array of block type objects to override palette
 */
(function () {
  'use strict';

  /** Default block types for campaign dashboards. Can be overridden per-widget
   *  via the data-block-types attribute for category dashboards. */
  var DEFAULT_BLOCK_TYPES = [
    { type: 'welcome_banner', label: 'Welcome Banner', icon: 'fa-flag',       desc: 'Campaign name & description' },
    { type: 'category_grid',  label: 'Category Grid',  icon: 'fa-grid-2',     desc: 'Quick-nav entity type grid' },
    { type: 'recent_pages',   label: 'Recent Pages',   icon: 'fa-clock',      desc: 'Recently updated entities' },
    { type: 'entity_list',    label: 'Entity List',     icon: 'fa-list',       desc: 'Filtered list by category' },
    { type: 'text_block',     label: 'Text Block',      icon: 'fa-align-left', desc: 'Custom rich text / HTML' },
    { type: 'pinned_pages',   label: 'Pinned Pages',    icon: 'fa-thumbtack',  desc: 'Hand-picked entity cards' },
    { type: 'calendar_preview', label: 'Calendar',     icon: 'fa-calendar-days', desc: 'Upcoming calendar events' }
  ];

  /** Column layout presets for adding new rows. */
  var COL_PRESETS = [
    { label: 'Full Width',       widths: [12] },
    { label: '2 Equal Columns',  widths: [6, 6] },
    { label: 'Wide + Sidebar',   widths: [8, 4] },
    { label: 'Sidebar + Wide',   widths: [4, 8] },
    { label: '3 Equal Columns',  widths: [4, 4, 4] }
  ];

  /**
   * Generate a short random ID.
   */
  function genId() {
    return 'db_' + Math.random().toString(36).substr(2, 8);
  }

  /**
   * Escape HTML to prevent XSS.
   */
  function esc(text) {
    var d = document.createElement('div');
    d.textContent = String(text || '');
    return d.innerHTML;
  }

  /**
   * Get CSRF token from cookie.
   */
  function getCsrf() {
    var m = document.cookie.match('(?:^|; )chronicle_csrf=([^;]*)');
    return m ? decodeURIComponent(m[1]) : '';
  }

  Chronicle.register('dashboard-editor', {
    init: function (el, config) {
      this.el = el;
      this.endpoint = config.endpoint;
      this.campaignId = config.campaignId;
      this.csrfToken = config.csrfToken;
      this.layout = null; // null = default layout in use
      this.dirty = false;
      this.dragState = null; // { type, blockType } or { type, rowIdx, colIdx, blockIdx }

      // Allow per-widget block palette override (e.g., category dashboards).
      if (config.blockTypes) {
        try {
          this.blockTypes = JSON.parse(config.blockTypes);
        } catch (e) {
          this.blockTypes = DEFAULT_BLOCK_TYPES;
        }
      } else {
        this.blockTypes = DEFAULT_BLOCK_TYPES;
      }

      this.load();
    },

    /**
     * Fetch existing layout from server.
     */
    load: function () {
      var self = this;
      fetch(this.endpoint, {
        headers: { 'Accept': 'application/json' },
        credentials: 'same-origin'
      })
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          return res.json();
        })
        .then(function (data) {
          self.layout = data; // null means default layout
          self.render();
        })
        .catch(function () {
          self.layout = null;
          self.render();
        });
    },

    /**
     * Save layout to server.
     */
    save: function (callback) {
      var self = this;
      if (!this.layout) {
        // Nothing to save — layout is default.
        if (callback) callback();
        return;
      }

      fetch(this.endpoint, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        credentials: 'same-origin',
        body: JSON.stringify(this.layout)
      })
        .then(function (res) {
          if (!res.ok) {
            Chronicle.notify('Failed to save dashboard layout', 'error');
          } else {
            self.dirty = false;
            Chronicle.notify('Dashboard layout saved', 'success');
          }
          if (callback) callback();
        })
        .catch(function () {
          Chronicle.notify('Failed to save dashboard layout', 'error');
          if (callback) callback();
        });
    },

    /**
     * Reset layout to default.
     */
    resetLayout: function () {
      var self = this;
      if (!confirm('Reset to default dashboard? This will remove your custom layout.')) return;

      fetch(this.endpoint, {
        method: 'DELETE',
        headers: { 'X-CSRF-Token': getCsrf() },
        credentials: 'same-origin'
      })
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          self.layout = null;
          self.dirty = false;
          Chronicle.notify('Dashboard reset to default', 'success');
          self.render();
        })
        .catch(function () {
          Chronicle.notify('Failed to reset dashboard', 'error');
        });
    },

    /**
     * Ensure we have a mutable layout object (create empty if null).
     */
    ensureLayout: function () {
      if (!this.layout) {
        this.layout = { rows: [] };
      }
    },

    /**
     * Render the full editor UI: palette + canvas.
     */
    render: function () {
      var self = this;
      var h = '';

      // Toolbar.
      h += '<div class="flex items-center justify-between mb-4">';
      h += '<div class="flex items-center gap-2">';
      h += '<button type="button" class="btn-save btn-primary text-sm px-3 py-1.5" disabled>Save Layout</button>';
      if (this.layout) {
        h += '<button type="button" class="btn-reset btn-secondary text-sm px-3 py-1.5">Reset to Default</button>';
      }
      h += '</div>';
      if (!this.layout) {
        h += '<span class="text-xs text-fg-muted italic">Using default dashboard layout</span>';
      }
      h += '</div>';

      // Two-panel layout: palette + canvas.
      h += '<div class="grid grid-cols-12 gap-4">';

      // Palette (left).
      h += '<div class="col-span-3">';
      h += '<div class="card p-3 space-y-2 sticky top-4">';
      h += '<h4 class="text-xs font-semibold text-fg-secondary uppercase tracking-wider mb-2">Blocks</h4>';
      self.blockTypes.forEach(function (bt) {
        h += '<div class="palette-block flex items-center gap-2 px-2 py-1.5 rounded border border-edge bg-surface-raised cursor-grab hover:border-accent/50 transition-colors text-sm" draggable="true" data-block-type="' + bt.type + '">';
        h += '<i class="fa-solid ' + bt.icon + ' text-xs text-fg-muted w-4 text-center"></i>';
        h += '<div>';
        h += '<div class="text-fg font-medium text-xs">' + esc(bt.label) + '</div>';
        h += '<div class="text-[10px] text-fg-muted">' + esc(bt.desc) + '</div>';
        h += '</div>';
        h += '</div>';
      });

      h += '<hr class="border-edge my-2"/>';
      h += '<h4 class="text-xs font-semibold text-fg-secondary uppercase tracking-wider mb-2">Add Row</h4>';
      COL_PRESETS.forEach(function (p) {
        h += '<button type="button" class="add-row-btn w-full text-left flex items-center gap-2 px-2 py-1.5 rounded border border-edge bg-surface-raised hover:border-accent/50 transition-colors text-xs" data-widths=\'' + JSON.stringify(p.widths) + '\'>';
        h += '<i class="fa-solid fa-plus text-[10px] text-fg-muted w-4 text-center"></i>';
        h += '<span class="text-fg font-medium">' + esc(p.label) + '</span>';
        h += '</button>';
      });
      h += '</div>';
      h += '</div>';

      // Canvas (right).
      h += '<div class="col-span-9">';
      h += '<div class="canvas space-y-3 min-h-[200px]">';

      if (!this.layout || !this.layout.rows || this.layout.rows.length === 0) {
        h += '<div class="border-2 border-dashed border-edge rounded-lg p-8 text-center">';
        h += '<i class="fa-solid fa-grip text-3xl text-fg-muted mb-3"></i>';
        h += '<p class="text-sm text-fg-muted">No custom layout yet.</p>';
        h += '<p class="text-xs text-fg-secondary mt-1">Add a row from the palette, then drag blocks into columns.</p>';
        h += '</div>';
      } else {
        this.layout.rows.forEach(function (row, ri) {
          h += self.renderRow(row, ri);
        });
      }

      h += '</div>';
      h += '</div>';

      h += '</div>'; // grid

      this.el.innerHTML = h;
      this.bindEvents();
    },

    /**
     * Render a single row with its columns and blocks.
     */
    renderRow: function (row, rowIdx) {
      var self = this;
      var h = '';
      h += '<div class="dash-row border border-edge rounded-lg bg-surface p-2" data-row-idx="' + rowIdx + '">';

      // Row header with controls.
      h += '<div class="flex items-center justify-between mb-2 px-1">';
      h += '<span class="text-[10px] text-fg-muted font-mono">Row ' + (rowIdx + 1) + '</span>';
      h += '<div class="flex items-center gap-1">';
      if (rowIdx > 0) {
        h += '<button type="button" class="move-row-btn p-1 text-fg-muted hover:text-fg" data-row="' + rowIdx + '" data-dir="-1" title="Move up"><i class="fa-solid fa-chevron-up text-[10px]"></i></button>';
      }
      if (this.layout && rowIdx < this.layout.rows.length - 1) {
        h += '<button type="button" class="move-row-btn p-1 text-fg-muted hover:text-fg" data-row="' + rowIdx + '" data-dir="1" title="Move down"><i class="fa-solid fa-chevron-down text-[10px]"></i></button>';
      }
      h += '<button type="button" class="delete-row-btn p-1 text-red-400 hover:text-red-300" data-row="' + rowIdx + '" title="Delete row"><i class="fa-solid fa-trash text-[10px]"></i></button>';
      h += '</div>';
      h += '</div>';

      // Columns (using CSS grid with 12-column template).
      h += '<div class="grid grid-cols-12 gap-2">';
      row.columns.forEach(function (col, ci) {
        h += '<div class="dash-col col-span-' + col.width + ' min-h-[60px] border border-dashed border-edge/50 rounded p-1.5 transition-colors" data-row-idx="' + rowIdx + '" data-col-idx="' + ci + '">';

        if (col.blocks.length === 0) {
          h += '<div class="drop-zone flex items-center justify-center h-full text-[10px] text-fg-muted">';
          h += '<span>Drop block here</span>';
          h += '</div>';
        } else {
          col.blocks.forEach(function (block, bi) {
            h += self.renderBlock(block, rowIdx, ci, bi);
          });
        }

        h += '</div>';
      });
      h += '</div>';

      h += '</div>';
      return h;
    },

    /**
     * Render a single block inside a column.
     */
    renderBlock: function (block, rowIdx, colIdx, blockIdx) {
      var bt = this.blockTypes.find(function (b) { return b.type === block.type; });
      var label = bt ? bt.label : block.type;
      var icon = bt ? bt.icon : 'fa-puzzle-piece';

      var h = '';
      h += '<div class="dash-block flex items-center gap-2 px-2 py-1.5 rounded bg-surface-raised border border-edge text-xs cursor-grab mb-1" draggable="true" data-row="' + rowIdx + '" data-col="' + colIdx + '" data-block="' + blockIdx + '">';
      h += '<i class="fa-solid ' + icon + ' text-fg-muted w-3 text-center text-[10px]"></i>';
      h += '<span class="flex-1 text-fg font-medium truncate">' + esc(label) + '</span>';

      // Config indicator for blocks with config.
      if (block.config && Object.keys(block.config).length > 0) {
        h += '<span class="text-[9px] text-accent" title="Has config"><i class="fa-solid fa-gear"></i></span>';
      }

      // Config + delete buttons.
      h += '<button type="button" class="config-block-btn p-0.5 text-fg-muted hover:text-fg" data-row="' + rowIdx + '" data-col="' + colIdx + '" data-block="' + blockIdx + '" title="Configure"><i class="fa-solid fa-sliders text-[10px]"></i></button>';
      h += '<button type="button" class="delete-block-btn p-0.5 text-red-400 hover:text-red-300" data-row="' + rowIdx + '" data-col="' + colIdx + '" data-block="' + blockIdx + '" title="Remove"><i class="fa-solid fa-xmark text-[10px]"></i></button>';
      h += '</div>';
      return h;
    },

    /**
     * Bind all events after render.
     */
    bindEvents: function () {
      var self = this;

      // Save button.
      var saveBtn = this.el.querySelector('.btn-save');
      if (saveBtn) {
        saveBtn.disabled = !this.dirty;
        saveBtn.addEventListener('click', function () {
          self.save(function () { self.render(); });
        });
      }

      // Reset button.
      var resetBtn = this.el.querySelector('.btn-reset');
      if (resetBtn) {
        resetBtn.addEventListener('click', function () {
          self.resetLayout();
        });
      }

      // Add row buttons.
      this.el.querySelectorAll('.add-row-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
          var widths = JSON.parse(btn.dataset.widths);
          self.addRow(widths);
        });
      });

      // Delete row buttons.
      this.el.querySelectorAll('.delete-row-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
          var ri = parseInt(btn.dataset.row);
          self.deleteRow(ri);
        });
      });

      // Move row buttons.
      this.el.querySelectorAll('.move-row-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
          var ri = parseInt(btn.dataset.row);
          var dir = parseInt(btn.dataset.dir);
          self.moveRow(ri, dir);
        });
      });

      // Delete block buttons.
      this.el.querySelectorAll('.delete-block-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
          var ri = parseInt(btn.dataset.row);
          var ci = parseInt(btn.dataset.col);
          var bi = parseInt(btn.dataset.block);
          self.deleteBlock(ri, ci, bi);
        });
      });

      // Config block buttons.
      this.el.querySelectorAll('.config-block-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
          var ri = parseInt(btn.dataset.row);
          var ci = parseInt(btn.dataset.col);
          var bi = parseInt(btn.dataset.block);
          self.configBlock(ri, ci, bi);
        });
      });

      // Drag from palette.
      this.el.querySelectorAll('.palette-block').forEach(function (block) {
        block.addEventListener('dragstart', function (e) {
          self.dragState = { type: 'palette', blockType: block.dataset.blockType };
          e.dataTransfer.effectAllowed = 'copy';
        });
        block.addEventListener('dragend', function () {
          self.dragState = null;
          self.clearDropHighlights();
        });
      });

      // Drag existing blocks.
      this.el.querySelectorAll('.dash-block').forEach(function (block) {
        block.addEventListener('dragstart', function (e) {
          self.dragState = {
            type: 'move',
            rowIdx: parseInt(block.dataset.row),
            colIdx: parseInt(block.dataset.col),
            blockIdx: parseInt(block.dataset.block)
          };
          e.dataTransfer.effectAllowed = 'move';
        });
        block.addEventListener('dragend', function () {
          self.dragState = null;
          self.clearDropHighlights();
        });
      });

      // Drop targets (columns).
      this.el.querySelectorAll('.dash-col').forEach(function (col) {
        col.addEventListener('dragover', function (e) {
          if (!self.dragState) return;
          e.preventDefault();
          e.dataTransfer.dropEffect = self.dragState.type === 'palette' ? 'copy' : 'move';
          col.classList.add('border-accent', 'bg-accent/5');
          col.classList.remove('border-edge/50');
        });
        col.addEventListener('dragleave', function () {
          col.classList.remove('border-accent', 'bg-accent/5');
          col.classList.add('border-edge/50');
        });
        col.addEventListener('drop', function (e) {
          e.preventDefault();
          col.classList.remove('border-accent', 'bg-accent/5');
          col.classList.add('border-edge/50');

          var ri = parseInt(col.dataset.rowIdx);
          var ci = parseInt(col.dataset.colIdx);

          if (self.dragState.type === 'palette') {
            self.addBlock(ri, ci, self.dragState.blockType);
          } else if (self.dragState.type === 'move') {
            self.moveBlock(self.dragState.rowIdx, self.dragState.colIdx, self.dragState.blockIdx, ri, ci);
          }
          self.dragState = null;
        });
      });
    },

    /**
     * Remove drop highlight classes from all columns.
     */
    clearDropHighlights: function () {
      this.el.querySelectorAll('.dash-col').forEach(function (col) {
        col.classList.remove('border-accent', 'bg-accent/5');
        col.classList.add('border-edge/50');
      });
    },

    /**
     * Add a new row with the given column widths.
     */
    addRow: function (widths) {
      this.ensureLayout();
      var cols = widths.map(function (w) {
        return { id: genId(), width: w, blocks: [] };
      });
      this.layout.rows.push({ id: genId(), columns: cols });
      this.dirty = true;
      this.render();
    },

    /**
     * Delete a row by index.
     */
    deleteRow: function (rowIdx) {
      if (!this.layout) return;
      this.layout.rows.splice(rowIdx, 1);
      this.dirty = true;
      if (this.layout.rows.length === 0) {
        // No rows left — save as empty to persist the custom layout state.
      }
      this.render();
    },

    /**
     * Move a row up (-1) or down (+1).
     */
    moveRow: function (rowIdx, direction) {
      if (!this.layout) return;
      var newIdx = rowIdx + direction;
      if (newIdx < 0 || newIdx >= this.layout.rows.length) return;
      var rows = this.layout.rows;
      var tmp = rows[rowIdx];
      rows[rowIdx] = rows[newIdx];
      rows[newIdx] = tmp;
      this.dirty = true;
      this.render();
    },

    /**
     * Add a block to a specific column.
     */
    addBlock: function (rowIdx, colIdx, blockType) {
      if (!this.layout) return;
      var col = this.layout.rows[rowIdx].columns[colIdx];
      col.blocks.push({ id: genId(), type: blockType, config: {} });
      this.dirty = true;
      this.render();
    },

    /**
     * Delete a block from a column.
     */
    deleteBlock: function (rowIdx, colIdx, blockIdx) {
      if (!this.layout) return;
      this.layout.rows[rowIdx].columns[colIdx].blocks.splice(blockIdx, 1);
      this.dirty = true;
      this.render();
    },

    /**
     * Move a block from one column to another (or reorder within same column).
     */
    moveBlock: function (fromRow, fromCol, fromBlock, toRow, toCol) {
      if (!this.layout) return;
      var block = this.layout.rows[fromRow].columns[fromCol].blocks.splice(fromBlock, 1)[0];
      this.layout.rows[toRow].columns[toCol].blocks.push(block);
      this.dirty = true;
      this.render();
    },

    /**
     * Show a config dialog for a block. Different block types have different
     * config options. Uses a simple prompt-based approach for now.
     */
    configBlock: function (rowIdx, colIdx, blockIdx) {
      if (!this.layout) return;
      var block = this.layout.rows[rowIdx].columns[colIdx].blocks[blockIdx];
      var cfg = block.config || {};

      switch (block.type) {
        case 'recent_pages':
          var limit = prompt('Number of recent pages to show (4-12):', cfg.limit || '8');
          if (limit !== null) {
            var n = parseInt(limit);
            if (n >= 1 && n <= 50) {
              block.config.limit = n;
              this.dirty = true;
              this.render();
            }
          }
          break;

        case 'category_grid':
          var cols = prompt('Number of columns (2-6):', cfg.columns || '4');
          if (cols !== null) {
            var c = parseInt(cols);
            if (c >= 1 && c <= 12) {
              block.config.columns = c;
              this.dirty = true;
              this.render();
            }
          }
          break;

        case 'text_block':
          var content = prompt('Enter text/HTML content:', cfg.content || '');
          if (content !== null) {
            block.config.content = content;
            this.dirty = true;
            this.render();
          }
          break;

        case 'entity_list':
          var etId = prompt('Entity type ID to filter by:', cfg.entity_type_id || '');
          if (etId !== null) {
            block.config.entity_type_id = etId;
            var listLimit = prompt('Number of entities (4-20):', cfg.limit || '8');
            if (listLimit !== null) {
              block.config.limit = parseInt(listLimit) || 8;
            }
            this.dirty = true;
            this.render();
          }
          break;

        case 'entity_grid':
          var gridCols = prompt('Number of columns (2-6):', cfg.columns || '4');
          if (gridCols !== null) {
            var gc = parseInt(gridCols);
            if (gc >= 1 && gc <= 12) {
              block.config.columns = gc;
              this.dirty = true;
              this.render();
            }
          }
          break;

        case 'calendar_preview':
          var calLimit = prompt('Number of upcoming events to show (1-20):', cfg.limit || '5');
          if (calLimit !== null) {
            var cl = parseInt(calLimit);
            if (cl >= 1 && cl <= 20) {
              block.config.limit = cl;
              this.dirty = true;
              this.render();
            }
          }
          break;

        default:
          Chronicle.notify('This block has no configurable options', 'info');
          break;
      }
    },

    destroy: function () {
      this.el.innerHTML = '';
    }
  });
})();
