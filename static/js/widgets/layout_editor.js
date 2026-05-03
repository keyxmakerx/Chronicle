/**
 * Unified Layout Editor Widget
 *
 * Single drag-and-drop layout editor replacing both dashboard_editor.js and
 * template_editor.js. Used by layout_studio.js to edit campaign dashboards,
 * owner dashboards, category dashboards, and page templates through one UI.
 *
 * Mount: data-widget="layout-editor"
 * Config:
 *   data-endpoint       - GET/PUT/DELETE endpoint for layout JSON
 *   data-campaign-id    - Campaign UUID
 *   data-csrf-token     - CSRF token
 *   data-context        - "dashboard" or "template"
 *   data-features       - Comma-separated feature flags
 *   data-layout         - (optional) Initial layout JSON
 *   data-block-types    - (optional) Override palette block types JSON
 *   data-fields         - (optional) Entity type field definitions for previews
 *   data-role           - (optional) Role for dashboard layouts
 */
(function () {
  'use strict';

  // ── Constants ──────────────────────────────────────────────────────

  var COL_PRESETS = [
    { label: 'Full Width',       widths: [12] },
    { label: '2 Equal Columns',  widths: [6, 6] },
    { label: 'Wide + Sidebar',   widths: [8, 4] },
    { label: 'Sidebar + Wide',   widths: [4, 8] },
    { label: '3 Equal Columns',  widths: [4, 4, 4] },
  ];

  var HEIGHT_PRESETS = [
    { value: 'auto', label: 'Auto' },
    { value: 'sm',   label: 'Small',   px: '150px' },
    { value: 'md',   label: 'Medium',  px: '300px' },
    { value: 'lg',   label: 'Large',   px: '500px' },
    { value: 'xl',   label: 'X-Large', px: '700px' },
  ];

  var VISIBILITY_OPTIONS = [
    { value: 'everyone', label: 'Everyone', icon: 'fa-globe' },
    { value: 'dm_only',  label: 'DM Only',  icon: 'fa-lock' },
  ];

  var TWO_COL_PRESETS = [
    { label: '50 / 50', left: 6, right: 6 },
    { label: '33 / 67', left: 4, right: 8 },
    { label: '67 / 33', left: 8, right: 4 },
  ];

  var CONTAINER_TYPES = ['two_column', 'three_column', 'tabs', 'section'];

  /** Fallback block types when the API is unavailable. */
  var FALLBACK_DASHBOARD_BLOCKS = [
    { type: 'welcome_banner',   label: 'Welcome Banner',   icon: 'fa-flag',              desc: 'Campaign name & description' },
    { type: 'category_grid',    label: 'Category Grid',    icon: 'fa-grid-2',            desc: 'Quick-nav entity type grid' },
    { type: 'recent_pages',     label: 'Recent Pages',     icon: 'fa-clock',             desc: 'Recently updated entities' },
    { type: 'entity_list',      label: 'Entity List',      icon: 'fa-list',              desc: 'Filtered list by category' },
    { type: 'text_block',       label: 'Text Block',       icon: 'fa-align-left',        desc: 'Custom rich text / HTML' },
    { type: 'pinned_pages',     label: 'Pinned Pages',     icon: 'fa-thumbtack',         desc: 'Hand-picked entity cards' },
    { type: 'calendar_preview', label: 'Calendar',         icon: 'fa-calendar-days',     desc: 'Upcoming calendar events',       addon: 'calendar' },
    { type: 'timeline_preview', label: 'Timeline',         icon: 'fa-timeline',          desc: 'Timeline list with event counts', addon: 'timeline' },
    // map_preview retired — superseded by map_editor (entity templates)
    // and map_full (dashboards). See PR notes / routes.go map_editor block.
    { type: 'calendar_full',    label: 'Full Calendar',    icon: 'fa-calendar',          desc: 'Full interactive calendar grid', addon: 'calendar' },
    { type: 'timeline_full',    label: 'Full Timeline',    icon: 'fa-timeline',          desc: 'Full timeline D3 visualization', addon: 'timeline' },
    { type: 'relations_graph_full', label: 'Full Relations Graph', icon: 'fa-diagram-project', desc: 'Large entity relations graph', addon: 'relations' },
    { type: 'map_full',         label: 'Full Map',         icon: 'fa-map-location-dot',  desc: 'Full map with drawings & tokens', addon: 'maps' },
    { type: 'session_tracker',  label: 'Sessions',         icon: 'fa-dice-d20',          desc: 'Upcoming sessions with RSVP',    addon: 'sessions' },
    { type: 'activity_feed',    label: 'Activity Feed',    icon: 'fa-clock-rotate-left', desc: 'Recent campaign activity log' },
    { type: 'sync_status',      label: 'Foundry Sync',     icon: 'fa-plug',              desc: 'Foundry VTT sync status',        addon: 'foundry' },
  ];

  var FALLBACK_TEMPLATE_BLOCKS = [
    { type: 'title',        label: 'Title',          icon: 'fa-heading',       desc: 'Entity name and actions' },
    { type: 'image',        label: 'Image',          icon: 'fa-image',         desc: 'Header image with upload' },
    { type: 'entry',        label: 'Rich Text',      icon: 'fa-align-left',    desc: 'Main content editor' },
    { type: 'attributes',   label: 'Attributes',     icon: 'fa-list',          desc: 'Custom field values' },
    { type: 'details',      label: 'Details',        icon: 'fa-info-circle',   desc: 'Metadata and dates' },
    { type: 'tags',         label: 'Tags',           icon: 'fa-tags',          desc: 'Tag picker widget' },
    { type: 'relations',    label: 'Relations',      icon: 'fa-link',          desc: 'Entity relation links' },
    { type: 'divider',      label: 'Divider',        icon: 'fa-minus',         desc: 'Horizontal separator' },
    { type: 'shop_inventory', label: 'Shop Inventory', icon: 'fa-store',       desc: 'Shop items with prices' },
    { type: 'posts',        label: 'Posts',           icon: 'fa-layer-group',   desc: 'Sub-notes and additional content sections' },
    { type: 'text_block',   label: 'Text Block',      icon: 'fa-align-left',   desc: 'Custom static HTML content' },
    { type: 'two_column',   label: '2 Columns',       icon: 'fa-columns',      desc: 'Side-by-side columns',       container: true },
    { type: 'three_column', label: '3 Columns',       icon: 'fa-table-columns', desc: 'Three equal columns',       container: true },
    { type: 'tabs',         label: 'Tabs',            icon: 'fa-folder',        desc: 'Tabbed content sections',   container: true },
    { type: 'section',      label: 'Section',         icon: 'fa-caret-down',    desc: 'Collapsible accordion',     container: true },
  ];

  function uid(prefix) {
    return (prefix || 'le') + '_' + Math.random().toString(36).substr(2, 8);
  }

  // ── Widget Registration ────────────────────────────────────────────

  Chronicle.register('layout-editor', {

    // ── Init ───────────────────────────────────────────────────────

    init: function (el) {
      this.el = el;
      this.endpoint = el.dataset.endpoint;
      this.baseEndpoint = el.dataset.endpoint;
      this.campaignId = el.dataset.campaignId;
      this.csrfToken = el.dataset.csrfToken;
      this.context = el.dataset.context || 'dashboard'; // "dashboard" or "template"

      // Parse feature flags.
      var featureStr = el.dataset.features || '';
      this.features = {};
      var self = this;
      featureStr.split(',').forEach(function (f) {
        f = f.trim();
        if (f) self.features[f] = true;
      });

      // Parse optional initial layout.
      try {
        this.layout = el.dataset.layout ? JSON.parse(el.dataset.layout) : null;
      } catch (e) {
        this.layout = null;
      }

      // Parse optional fields for preview mockups.
      try {
        this.fields = JSON.parse(el.dataset.fields || '[]');
      } catch (e) {
        this.fields = [];
      }

      // For dashboard context with roles.
      this.role = el.dataset.role || '';
      if (this.role) this.endpoint = this._buildEndpoint();

      this.dirty = false;
      this.dragState = null;
      this.dropIndicator = null;
      this.dropTarget = null;
      this.blockTypes = [];
      this.canvas = null;

      // Template defaults if no layout is provided.
      if (this.context === 'template' && (!this.layout || !this.layout.rows || this.layout.rows.length === 0)) {
        this.layout = this._defaultTemplateLayout();
      }

      // Role-change listener for dashboard context.
      if (this.features.roles) {
        this._onRoleChange = function (e) {
          var newRole = e.detail && e.detail.role;
          if (newRole && newRole !== self.role) {
            if (self.dirty && !confirm('You have unsaved changes. Switch role and discard them?')) return;
            self.role = newRole;
            self.endpoint = self._buildEndpoint();
            self.dirty = false;
            self.load();
          }
        };
        el.addEventListener('role-change', this._onRoleChange);
      }

      // Load block types from API, then render.
      this._loadBlockTypes();

      // If dashboard with no initial layout, fetch from server.
      if (this.context === 'dashboard' && !this.layout) {
        this.load();
      } else {
        this.render();
        this._bindSave();
      }
    },

    _buildEndpoint: function () {
      if (this.role && this.role !== '') {
        var sep = this.baseEndpoint.indexOf('?') >= 0 ? '&' : '?';
        return this.baseEndpoint + sep + 'role=' + encodeURIComponent(this.role);
      }
      return this.baseEndpoint;
    },

    _defaultTemplateLayout: function () {
      return {
        rows: [{
          id: uid('row'),
          columns: [
            { id: uid('col'), width: 8, blocks: [
              { id: uid('blk'), type: 'title', config: {} },
              { id: uid('blk'), type: 'entry', config: {} },
            ]},
            { id: uid('col'), width: 4, blocks: [
              { id: uid('blk'), type: 'image', config: {} },
              { id: uid('blk'), type: 'attributes', config: {} },
              { id: uid('blk'), type: 'details', config: {} },
            ]},
          ],
        }],
      };
    },

    hasFeature: function (name) {
      return !!this.features[name];
    },

    isContainer: function (type) {
      var bt = this.blockTypes.find(function (b) { return b.type === type; });
      if (bt) return !!bt.container;
      return CONTAINER_TYPES.indexOf(type) >= 0;
    },

    defaultBlockConfig: function (type) {
      switch (type) {
        case 'two_column':   return { left_width: 6, right_width: 6, left: [], right: [] };
        case 'three_column': return { widths: [4, 4, 4], columns: [[], [], []] };
        case 'tabs':         return { tabs: [{ label: 'Tab 1', blocks: [] }, { label: 'Tab 2', blocks: [] }] };
        case 'section':      return { title: 'Section', collapsed: false, blocks: [] };
        default:             return {};
      }
    },

    // ── Block Type Loading ─────────────────────────────────────────

    _loadBlockTypes: function () {
      var self = this;
      if (!this.campaignId) {
        this.blockTypes = this._fallbackBlocks();
        return;
      }

      var ctx = this.context === 'template' ? 'template' : 'dashboard';
      var url = '/campaigns/' + this.campaignId + '/entity-types/block-types?context=' + ctx;

      Chronicle.apiFetch(url)
        .then(function (r) { return r.ok ? r.json() : []; })
        .then(function (types) {
          if (types && types.length > 0) {
            self.blockTypes = types.map(function (t) {
              var bt = {
                type: t.type, label: t.label, icon: t.icon, desc: t.description,
                container: !!t.container,
                singleton: !!t.singleton, // Carries BlockMeta.Singleton — drives the singleton drop guard.
              };
              if (t.addon) bt.addon = t.addon;
              if (t.widget_slug) bt.widget_slug = t.widget_slug;
              if (t.config_fields) bt.config_fields = t.config_fields;
              return bt;
            });
          } else {
            self.blockTypes = self._fallbackBlocks();
          }
          self.render();
        })
        .catch(function () {
          self.blockTypes = self._fallbackBlocks();
          self.render();
        });
    },

    _fallbackBlocks: function () {
      return this.context === 'template' ? FALLBACK_TEMPLATE_BLOCKS : FALLBACK_DASHBOARD_BLOCKS;
    },

    // ── Palette Rendering ─────────────────────────────────────────

    _createPaletteSectionEl: function (title, icon, open) {
      var section = document.createElement('div');
      section.className = 'palette-section mb-1';

      var toggle = document.createElement('button');
      toggle.type = 'button';
      toggle.className = 'palette-section-toggle w-full flex items-center gap-1.5 px-1 py-1.5 text-left hover:bg-surface-alt rounded transition-colors';
      toggle.innerHTML =
        '<i class="fa-solid fa-chevron-down text-[8px] text-fg-muted palette-chevron transition-transform' + (open ? '' : ' -rotate-90') + '"></i>' +
        '<i class="fa-solid ' + icon + ' text-[9px] text-fg-muted"></i>' +
        '<span class="text-[10px] font-semibold text-fg-secondary uppercase tracking-wider flex-1">' + Chronicle.escapeHtml(title) + '</span>';

      var content = document.createElement('div');
      content.className = 'palette-section-content space-y-1 mt-1';
      if (!open) content.style.display = 'none';

      toggle.addEventListener('click', function () {
        var hidden = content.style.display === 'none';
        content.style.display = hidden ? '' : 'none';
        toggle.querySelector('.palette-chevron').classList.toggle('-rotate-90', !hidden);
      });

      section.appendChild(toggle);
      section.appendChild(content);
      return section;
    },

    _createPaletteSection: function (title, icon, blocks, open) {
      var section = this._createPaletteSectionEl(title, icon, open);
      var content = section.querySelector('.palette-section-content');
      var self = this;
      blocks.forEach(function (bt) { content.appendChild(self._createPaletteItem(bt)); });
      return section;
    },

    _createPaletteItem: function (bt) {
      var self = this;
      var item = document.createElement('div');
      item.className = 'flex items-center gap-2 px-3 py-2 mb-1 bg-surface-raised border border-edge rounded-md cursor-grab hover:border-accent/50 hover:shadow-sm transition-all text-sm';
      item.draggable = true;

      var source = bt.addon || bt.widget_slug || '';
      var addonBadge = source
        ? '<span class="text-[8px] px-1 py-px rounded bg-surface-alt text-fg-muted border border-edge leading-tight shrink-0">' + Chronicle.escapeHtml(source) + '</span>'
        : '';
      item.innerHTML =
        '<i class="fa-solid ' + bt.icon + ' w-4 text-fg-muted text-center"></i>' +
        '<div class="flex-1 min-w-0">' +
          '<div class="flex items-center gap-1">' +
            '<span class="font-medium text-fg">' + Chronicle.escapeHtml(bt.label) + '</span>' +
            addonBadge +
          '</div>' +
          '<div class="text-[10px] text-fg-muted">' + Chronicle.escapeHtml(bt.desc) + '</div>' +
        '</div>';

      item.addEventListener('dragstart', function (e) {
        var dragData = { source: 'palette', type: bt.type };
        if (bt.widget_slug) dragData.widget_slug = bt.widget_slug;
        e.dataTransfer.setData('text/plain', JSON.stringify(dragData));
        e.dataTransfer.effectAllowed = 'copyMove';
        item.classList.add('opacity-50');
      });
      item.addEventListener('dragend', function () {
        item.classList.remove('opacity-50');
        self.clearDropIndicator();
      });
      return item;
    },

    // ── Main Render ────────────────────────────────────────────────

    render: function () {
      this.el.innerHTML = '';
      this.el.className = 'flex h-full overflow-hidden';

      // Palette panel.
      var palette = document.createElement('div');
      palette.className = 'w-56 bg-surface-alt border-r border-edge p-3 overflow-y-auto shrink-0';

      if (this.context === 'template') {
        this._buildTemplatePalette(palette);
      } else {
        this._buildDashboardPalette(palette);
      }
      this.el.appendChild(palette);

      // Canvas area.
      var canvas = document.createElement('div');
      canvas.className = 'flex-1 overflow-y-auto p-6 bg-surface-alt';
      this.canvas = canvas;
      this.renderCanvas();
      this.el.appendChild(canvas);
    },

    _buildTemplatePalette: function (palette) {
      var self = this;
      var contentBlocks = this.blockTypes.filter(function (bt) { return !bt.container && !bt.widget_slug; });
      var extWidgetBlocks = this.blockTypes.filter(function (bt) { return !!bt.widget_slug; });
      var layoutBlocks = this.blockTypes.filter(function (bt) { return bt.container; });

      palette.appendChild(this._createPaletteSection('Components', 'fa-cube', contentBlocks, true));
      if (extWidgetBlocks.length > 0) {
        palette.appendChild(this._createPaletteSection('Extensions', 'fa-puzzle-piece', extWidgetBlocks, false));
      }
      if (this.hasFeature('containers')) {
        palette.appendChild(this._createPaletteSection('Layout', 'fa-columns', layoutBlocks, false));
      }

      // Row presets section.
      var presetSection = this._createPaletteSectionEl('Row Layouts', 'fa-table-columns', false);
      var presetContent = presetSection.querySelector('.palette-section-content');
      COL_PRESETS.forEach(function (preset) {
        var btn = document.createElement('button');
        btn.className = 'flex items-center gap-2 w-full px-3 py-2 mb-1 bg-surface-raised border border-edge rounded-md hover:border-accent/50 hover:shadow-sm transition-all text-sm text-left';
        var preview = preset.widths.map(function (w) {
          var pct = Math.round(w / 12 * 100);
          return '<div class="h-3 bg-edge rounded-sm" style="width:' + pct + '%"></div>';
        }).join('<div class="w-0.5"></div>');
        btn.innerHTML = '<div class="flex gap-0.5 w-12 shrink-0">' + preview + '</div><span class="text-fg">' + Chronicle.escapeHtml(preset.label) + '</span>';
        btn.addEventListener('click', function () { self.addRow(preset.widths); });
        presetContent.appendChild(btn);
      });
      palette.appendChild(presetSection);

      // Preset load/save (template only).
      if (this.hasFeature('presets') && this.campaignId) {
        var presetActions = this._createPaletteSectionEl('Presets', 'fa-floppy-disk', false);
        var actContent = presetActions.querySelector('.palette-section-content');

        var loadBtn = document.createElement('button');
        loadBtn.className = 'flex items-center gap-2 w-full px-3 py-2 mb-1 bg-surface-raised border border-edge rounded-md hover:border-accent/50 hover:shadow-sm transition-all text-sm text-left';
        loadBtn.innerHTML = '<i class="fa-solid fa-download w-4 text-fg-muted text-center"></i><span class="text-fg">Load Preset</span>';
        loadBtn.addEventListener('click', function () { self.showLoadPresetMenu(loadBtn); });
        actContent.appendChild(loadBtn);

        var savePresetBtn = document.createElement('button');
        savePresetBtn.className = 'flex items-center gap-2 w-full px-3 py-2 bg-surface-raised border border-edge rounded-md hover:border-accent/50 hover:shadow-sm transition-all text-sm text-left';
        savePresetBtn.innerHTML = '<i class="fa-solid fa-floppy-disk w-4 text-fg-muted text-center"></i><span class="text-fg">Save as Preset</span>';
        savePresetBtn.addEventListener('click', function () { self.saveAsPreset(); });
        actContent.appendChild(savePresetBtn);

        palette.appendChild(presetActions);
      }
    },

    _buildDashboardPalette: function (palette) {
      var self = this;
      var coreBlocks = this.blockTypes.filter(function (bt) { return !bt.addon; });
      var addonBlocks = this.blockTypes.filter(function (bt) { return !!bt.addon; });

      palette.appendChild(this._createPaletteSection('Core Blocks', 'fa-cube', coreBlocks, true));
      if (addonBlocks.length > 0) {
        palette.appendChild(this._createPaletteSection('Addon Blocks', 'fa-puzzle-piece', addonBlocks, false));
      }

      // Add Row section.
      var rowSection = this._createPaletteSectionEl('Add Row', 'fa-columns', true);
      var rowContent = rowSection.querySelector('.palette-section-content');
      COL_PRESETS.forEach(function (p) {
        var btn = document.createElement('button');
        btn.className = 'w-full text-left flex items-center gap-2 px-2 py-1.5 rounded border border-edge bg-surface-raised hover:border-accent/50 transition-colors text-xs';
        btn.innerHTML = '<i class="fa-solid fa-plus text-[10px] text-fg-muted w-4 text-center"></i><span class="text-fg font-medium">' + Chronicle.escapeHtml(p.label) + '</span>';
        btn.addEventListener('click', function () { self.addRow(p.widths); });
        rowContent.appendChild(btn);
      });
      palette.appendChild(rowSection);
    },

    // ── Canvas Rendering ─────────────────────────────────────────

    renderCanvas: function () {
      this.canvas.innerHTML = '';
      var self = this;
      var rows = this.layout && this.layout.rows ? this.layout.rows : [];

      if (rows.length === 0) {
        this.canvas.innerHTML =
          '<div class="flex flex-col items-center justify-center h-full text-fg-muted">' +
            '<i class="fa-solid fa-table-cells-large text-4xl mb-3"></i>' +
            '<p class="text-sm">' + (this.context === 'template' ? 'Click a row layout on the left to get started' : 'No custom layout yet. Add a row from the palette.') + '</p>' +
          '</div>';
        return;
      }

      rows.forEach(function (row, rowIdx) {
        self.canvas.appendChild(self._renderRow(row, rowIdx));
      });
    },

    _renderRow: function (row, rowIdx) {
      var self = this;
      var rowEl = document.createElement('div');
      rowEl.className = 'mb-4 group/row';
      rowEl.dataset.rowIdx = rowIdx;

      // Row toolbar.
      var toolbar = document.createElement('div');
      toolbar.className = 'flex items-center gap-2 mb-1 opacity-0 group-hover/row:opacity-100 transition-opacity';
      toolbar.innerHTML = '<span class="text-[10px] text-fg-muted font-mono">Row ' + (rowIdx + 1) + '</span><div class="flex-1"></div>';

      // Column layout picker buttons.
      COL_PRESETS.forEach(function (preset) {
        var btn = document.createElement('button');
        btn.className = 'p-1 hover:bg-surface-alt rounded transition-colors';
        btn.title = preset.label;
        var isActive = JSON.stringify(row.columns.map(function (c) { return c.width; })) === JSON.stringify(preset.widths);
        var preview = preset.widths.map(function (w) {
          var pct = Math.round(w / 12 * 100);
          var color = isActive ? 'bg-accent' : 'bg-edge';
          return '<div class="h-2 ' + color + ' rounded-sm" style="width:' + pct + '%"></div>';
        }).join('<div class="w-px"></div>');
        btn.innerHTML = '<div class="flex gap-px w-8">' + preview + '</div>';
        btn.addEventListener('click', function () { self.changeRowLayout(rowIdx, preset.widths); });
        toolbar.appendChild(btn);
      });

      // Delete row.
      var delBtn = document.createElement('button');
      delBtn.className = 'p-1 text-fg-muted hover:text-red-500 dark:hover:text-red-400 transition-colors ml-1';
      delBtn.title = 'Delete row';
      delBtn.innerHTML = '<i class="fa-solid fa-trash-can text-xs"></i>';
      delBtn.addEventListener('click', function () { self.deleteRow(rowIdx); });
      toolbar.appendChild(delBtn);

      // Move buttons.
      if (rowIdx > 0) {
        var upBtn = document.createElement('button');
        upBtn.className = 'p-1 text-fg-muted hover:text-fg transition-colors';
        upBtn.title = 'Move up';
        upBtn.innerHTML = '<i class="fa-solid fa-chevron-up text-xs"></i>';
        upBtn.addEventListener('click', function () { self.moveRow(rowIdx, -1); });
        toolbar.appendChild(upBtn);
      }
      if (this.layout && rowIdx < this.layout.rows.length - 1) {
        var downBtn = document.createElement('button');
        downBtn.className = 'p-1 text-fg-muted hover:text-fg transition-colors';
        downBtn.title = 'Move down';
        downBtn.innerHTML = '<i class="fa-solid fa-chevron-down text-xs"></i>';
        downBtn.addEventListener('click', function () { self.moveRow(rowIdx, 1); });
        toolbar.appendChild(downBtn);
      }

      rowEl.appendChild(toolbar);

      // Columns grid.
      var grid = document.createElement('div');
      grid.className = 'grid gap-3';
      grid.style.gridTemplateColumns = row.columns.map(function (c) { return c.width + 'fr'; }).join(' ');

      row.columns.forEach(function (col, colIdx) {
        grid.appendChild(self._renderColumn(col, rowIdx, colIdx));
      });

      rowEl.appendChild(grid);
      return rowEl;
    },

    _renderColumn: function (col, rowIdx, colIdx) {
      var self = this;
      var colEl = document.createElement('div');
      colEl.className = 'le-column bg-surface border-2 border-dashed border-edge rounded-lg min-h-[80px] p-2 transition-colors relative';
      colEl.dataset.rowIdx = rowIdx;
      colEl.dataset.colIdx = colIdx;

      // Column header.
      var colHeader = document.createElement('div');
      colHeader.className = 'text-[10px] text-fg-muted font-mono mb-1 px-1';
      colHeader.textContent = col.width + '/12';
      colEl.appendChild(colHeader);

      // Render blocks.
      col.blocks.forEach(function (block, blockIdx) {
        colEl.appendChild(self.renderBlock(block, rowIdx, colIdx, blockIdx));
      });

      // Column-level drag events.
      colEl.addEventListener('dragover', function (e) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        colEl.classList.add('border-accent');
        self.updateDropIndicator(e, colEl, rowIdx, colIdx);
      });
      colEl.addEventListener('dragleave', function (e) {
        if (!colEl.contains(e.relatedTarget)) {
          colEl.classList.remove('border-accent');
          self.clearDropIndicator();
        }
      });
      colEl.addEventListener('drop', function (e) {
        e.preventDefault();
        colEl.classList.remove('border-accent');
        var insertIdx = self.dropTarget ? self.dropTarget.insertIdx : col.blocks.length;
        self.clearDropIndicator();
        self.handleDrop(e, rowIdx, colIdx, insertIdx);
      });

      return colEl;
    },

    // ── Block Rendering ────────────────────────────────────────────

    renderBlock: function (block, rowIdx, colIdx, blockIdx) {
      // Container blocks get specialized renderer (template context with containers feature).
      if (this.hasFeature('containers') && this.isContainer(block.type)) {
        return this.renderContainerBlock(block, rowIdx, colIdx, blockIdx);
      }

      if (!block.config) block.config = {};
      var bt = this.blockTypes.find(function (b) { return b.type === block.type; }) || { label: block.type, icon: 'fa-cube' };
      var el = document.createElement('div');
      el.className = 'le-block flex items-center gap-2 px-3 py-2 mb-1 bg-surface-raised border border-edge rounded group/block cursor-grab hover:border-accent/50 transition-colors';
      el.draggable = true;
      el.dataset.blockIdx = blockIdx;

      var html = '<i class="fa-solid fa-grip-vertical text-fg-muted text-xs"></i>' +
        '<i class="fa-solid ' + bt.icon + ' w-4 text-fg-muted text-center text-sm"></i>' +
        '<span class="text-sm font-medium text-fg flex-1">' + Chronicle.escapeHtml(bt.label) + '</span>';

      // Visibility controls (template with visibility feature).
      if (this.hasFeature('visibility')) {
        var curVis = block.config.visibility || 'everyone';
        if (curVis === 'dm_only') {
          el.classList.add('border-amber-400', 'dark:border-amber-600');
          html += '<i class="fa-solid fa-lock text-amber-500 text-[10px]" title="DM Only"></i>';
        }
        html += '<select class="le-block-vis opacity-0 group-hover/block:opacity-100 text-[10px] bg-transparent text-fg-muted border border-edge rounded px-1 py-0.5 cursor-pointer hover:text-fg transition-all" title="Visibility">';
        VISIBILITY_OPTIONS.forEach(function (v) {
          html += '<option value="' + v.value + '"' + (v.value === curVis ? ' selected' : '') + '>' + v.label + '</option>';
        });
        html += '</select>';
      }

      // Height controls (template with height feature).
      if (this.hasFeature('height')) {
        var curHeight = block.config.minHeight || 'auto';
        html += '<select class="le-block-height opacity-0 group-hover/block:opacity-100 text-[10px] bg-transparent text-fg-muted border border-edge rounded px-1 py-0.5 cursor-pointer hover:text-fg transition-all" title="Block height">';
        HEIGHT_PRESETS.forEach(function (h) {
          html += '<option value="' + h.value + '"' + (h.value === curHeight ? ' selected' : '') + '>' + h.label + '</option>';
        });
        html += '</select>';
      }

      // Config button (for blocks with config_fields or dashboard blocks).
      if (bt.config_fields && bt.config_fields.length > 0) {
        html += '<button class="le-config-btn opacity-0 group-hover/block:opacity-100 text-fg-muted hover:text-accent transition-all p-0.5" title="Configure"><i class="fa-solid fa-sliders text-xs"></i></button>';
      }

      // Delete button.
      html += '<button class="le-block-del opacity-0 group-hover/block:opacity-100 text-fg-muted hover:text-red-500 dark:hover:text-red-400 transition-all p-0.5" title="Remove"><i class="fa-solid fa-xmark text-xs"></i></button>';

      el.innerHTML = html;

      // Bind visibility change.
      var self = this;
      var visSelect = el.querySelector('.le-block-vis');
      if (visSelect) {
        visSelect.addEventListener('change', function (e) {
          e.stopPropagation();
          if (e.target.value === 'everyone') { delete block.config.visibility; }
          else { block.config.visibility = e.target.value; }
          self.markDirty();
          self.renderCanvas();
        });
        visSelect.addEventListener('mousedown', function (e) { e.stopPropagation(); });
      }

      // Bind height change.
      var heightSelect = el.querySelector('.le-block-height');
      if (heightSelect) {
        heightSelect.addEventListener('change', function (e) {
          e.stopPropagation();
          if (e.target.value === 'auto') { delete block.config.minHeight; }
          else { block.config.minHeight = e.target.value; }
          self.markDirty();
        });
        heightSelect.addEventListener('mousedown', function (e) { e.stopPropagation(); });
      }

      // Bind config button.
      var configBtn = el.querySelector('.le-config-btn');
      if (configBtn) {
        configBtn.addEventListener('click', function (e) {
          e.stopPropagation();
          self.showConfigDialog(block, bt);
        });
      }

      this._bindBlockDrag(el, block, rowIdx, colIdx, blockIdx);
      this._bindBlockDelete(el, rowIdx, colIdx, blockIdx);

      // Preview (template with preview feature).
      if (this.hasFeature('preview')) {
        this._bindBlockPreview(el, block);
      }

      return el;
    },

    _bindBlockDrag: function (el, block, rowIdx, colIdx, blockIdx) {
      var self = this;
      el.addEventListener('dragstart', function (e) {
        e.stopPropagation();
        e.dataTransfer.setData('text/plain', JSON.stringify({
          source: 'canvas', rowIdx: rowIdx, colIdx: colIdx, blockIdx: blockIdx, block: block,
        }));
        e.dataTransfer.effectAllowed = 'move';
        el.classList.add('opacity-50');
      });
      el.addEventListener('dragend', function () {
        el.classList.remove('opacity-50');
        self.clearDropIndicator();
      });
    },

    _bindBlockDelete: function (el, rowIdx, colIdx, blockIdx) {
      var self = this;
      el.querySelector('.le-block-del').addEventListener('click', function (e) {
        e.stopPropagation();
        self.layout.rows[rowIdx].columns[colIdx].blocks.splice(blockIdx, 1);
        self.markDirty();
        self.renderCanvas();
      });
    },

    _bindBlockPreview: function (el, block) {
      var self = this;
      el.addEventListener('contextmenu', function (e) {
        e.preventDefault();
        e.stopPropagation();
        self.showBlockPreview(block, e.clientX, e.clientY);
      });
      var previewBtn = document.createElement('button');
      previewBtn.className = 'le-preview-btn opacity-0 group-hover/block:opacity-100 text-fg-muted hover:text-accent transition-all p-0.5 mr-1';
      previewBtn.title = 'Preview appearance';
      previewBtn.innerHTML = '<i class="fa-solid fa-eye text-xs"></i>';
      previewBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        var rect = el.getBoundingClientRect();
        self.showBlockPreview(block, rect.left + rect.width / 2, rect.top);
      });
      var delBtn = el.querySelector('.le-block-del');
      if (delBtn) delBtn.parentNode.insertBefore(previewBtn, delBtn);
    },

    // ── Container Block Rendering ─────────────────────────────────

    renderContainerBlock: function (block, rowIdx, colIdx, blockIdx) {
      var bt = this.blockTypes.find(function (b) { return b.type === block.type; }) || { label: block.type, icon: 'fa-cube' };
      var el = document.createElement('div');
      el.className = 'le-block le-container-block mb-1 border border-edge rounded-lg bg-surface overflow-hidden group/block';
      el.draggable = true;
      el.dataset.blockIdx = blockIdx;

      // Header bar.
      var header = document.createElement('div');
      header.className = 'flex items-center gap-2 px-3 py-2 bg-accent/10 border-b border-accent/30 cursor-grab';
      header.innerHTML =
        '<i class="fa-solid fa-grip-vertical text-accent/40 text-xs"></i>' +
        '<i class="fa-solid ' + bt.icon + ' w-4 text-accent/60 text-center text-sm"></i>' +
        '<span class="text-sm font-semibold text-accent flex-1">' + Chronicle.escapeHtml(bt.label) + '</span>';

      var configArea = document.createElement('div');
      configArea.className = 'flex items-center gap-1';
      this._renderContainerConfig(configArea, block, rowIdx, colIdx, blockIdx);

      var delBtn = document.createElement('button');
      delBtn.className = 'le-block-del text-accent/40 hover:text-red-500 dark:hover:text-red-400 transition-all p-0.5 ml-1';
      delBtn.title = 'Remove';
      delBtn.innerHTML = '<i class="fa-solid fa-xmark text-xs"></i>';
      configArea.appendChild(delBtn);
      header.appendChild(configArea);
      el.appendChild(header);

      // Container body with sub-block zones.
      var body = document.createElement('div');
      body.className = 'p-2';
      this._renderContainerBody(body, block, rowIdx, colIdx, blockIdx);
      el.appendChild(body);

      this._bindBlockDrag(el, block, rowIdx, colIdx, blockIdx);
      var self = this;
      delBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        self.layout.rows[rowIdx].columns[colIdx].blocks.splice(blockIdx, 1);
        self.markDirty();
        self.renderCanvas();
      });

      if (this.hasFeature('preview')) {
        el.addEventListener('contextmenu', function (e) {
          e.preventDefault();
          e.stopPropagation();
          self.showBlockPreview(block, e.clientX, e.clientY);
        });
      }

      return el;
    },

    _renderContainerConfig: function (container, block, rowIdx, colIdx, blockIdx) {
      var self = this;
      switch (block.type) {
        case 'two_column':
          var select = document.createElement('select');
          select.className = 'text-xs border border-accent/30 rounded px-1 py-0.5 bg-surface-raised text-accent focus:outline-none focus:ring-1 focus:ring-accent';
          select.title = 'Column widths';
          TWO_COL_PRESETS.forEach(function (preset) {
            var opt = document.createElement('option');
            opt.value = preset.left + ':' + preset.right;
            opt.textContent = preset.label;
            if (block.config.left_width === preset.left && block.config.right_width === preset.right) opt.selected = true;
            select.appendChild(opt);
          });
          select.addEventListener('change', function (e) {
            e.stopPropagation();
            var parts = e.target.value.split(':').map(Number);
            block.config.left_width = parts[0];
            block.config.right_width = parts[1];
            self.markDirty();
            self.renderCanvas();
          });
          select.addEventListener('mousedown', function (e) { e.stopPropagation(); });
          container.appendChild(select);
          break;

        case 'tabs':
          var addBtn = document.createElement('button');
          addBtn.className = 'text-xs text-accent hover:text-accent-hover px-1.5 py-0.5 border border-accent/30 rounded hover:bg-accent/10 transition-colors';
          addBtn.title = 'Add tab';
          addBtn.innerHTML = '<i class="fa-solid fa-plus text-[10px]"></i> Tab';
          addBtn.addEventListener('click', function (e) {
            e.stopPropagation();
            block.config.tabs.push({ label: 'Tab ' + (block.config.tabs.length + 1), blocks: [] });
            self.markDirty();
            self.renderCanvas();
          });
          addBtn.addEventListener('mousedown', function (e) { e.stopPropagation(); });
          container.appendChild(addBtn);
          break;

        case 'section':
          var input = document.createElement('input');
          input.type = 'text';
          input.value = block.config.title || 'Section';
          input.className = 'text-xs border border-accent/30 rounded px-1.5 py-0.5 bg-surface-raised text-accent w-28 focus:outline-none focus:ring-1 focus:ring-accent';
          input.addEventListener('change', function (e) {
            e.stopPropagation();
            block.config.title = e.target.value || 'Section';
            self.markDirty();
          });
          input.addEventListener('mousedown', function (e) { e.stopPropagation(); });
          input.addEventListener('keydown', function (e) { e.stopPropagation(); });
          container.appendChild(input);

          var collapseBtn = document.createElement('button');
          var isCollapsed = block.config.collapsed;
          collapseBtn.className = 'text-xs px-1.5 py-0.5 border border-accent/30 rounded hover:bg-accent/10 transition-colors ' +
            (isCollapsed ? 'text-fg-muted' : 'text-accent');
          collapseBtn.title = isCollapsed ? 'Default: collapsed' : 'Default: expanded';
          collapseBtn.innerHTML = isCollapsed
            ? '<i class="fa-solid fa-chevron-right text-[10px]"></i>'
            : '<i class="fa-solid fa-chevron-down text-[10px]"></i>';
          collapseBtn.addEventListener('click', function (e) {
            e.stopPropagation();
            block.config.collapsed = !block.config.collapsed;
            self.markDirty();
            self.renderCanvas();
          });
          collapseBtn.addEventListener('mousedown', function (e) { e.stopPropagation(); });
          container.appendChild(collapseBtn);
          break;
      }
    },

    _renderContainerBody: function (body, block, rowIdx, colIdx, blockIdx) {
      var self = this;
      switch (block.type) {
        case 'two_column': {
          var leftW = block.config.left_width || 6;
          var rightW = block.config.right_width || 6;
          var grid = document.createElement('div');
          grid.className = 'grid gap-2';
          grid.style.gridTemplateColumns = leftW + 'fr ' + rightW + 'fr';
          grid.appendChild(this._createSubBlockZone(block.config.left || [], leftW + '/12', rowIdx, colIdx, blockIdx, 'left'));
          grid.appendChild(this._createSubBlockZone(block.config.right || [], rightW + '/12', rowIdx, colIdx, blockIdx, 'right'));
          body.appendChild(grid);
          break;
        }
        case 'three_column': {
          var widths = block.config.widths || [4, 4, 4];
          if (!block.config.columns) block.config.columns = [[], [], []];
          var g = document.createElement('div');
          g.className = 'grid gap-2';
          g.style.gridTemplateColumns = widths.map(function (w) { return w + 'fr'; }).join(' ');
          widths.forEach(function (w, i) {
            g.appendChild(self._createSubBlockZone(block.config.columns[i] || [], w + '/12', rowIdx, colIdx, blockIdx, 'col_' + i));
          });
          body.appendChild(g);
          break;
        }
        case 'tabs': {
          if (!block.config.tabs || block.config.tabs.length === 0) {
            block.config.tabs = [{ label: 'Tab 1', blocks: [] }];
          }
          if (block.config._activeTab === undefined) block.config._activeTab = 0;
          if (block.config._activeTab >= block.config.tabs.length) block.config._activeTab = 0;

          var tabBar = document.createElement('div');
          tabBar.className = 'flex items-center border-b border-edge mb-2 gap-0.5';

          block.config.tabs.forEach(function (tab, tabIdx) {
            var isActive = tabIdx === block.config._activeTab;
            var tabBtn = document.createElement('div');
            tabBtn.className = 'flex items-center gap-1 px-3 py-1.5 text-xs font-medium rounded-t cursor-pointer border border-b-0 transition-colors ' +
              (isActive ? 'bg-surface text-accent border-edge -mb-px' : 'bg-surface-raised text-fg-secondary border-transparent hover:text-fg hover:bg-surface-alt');

            var labelSpan = document.createElement('span');
            labelSpan.textContent = tab.label;
            labelSpan.className = 'cursor-pointer';
            labelSpan.title = 'Click to select, double-click to rename';
            labelSpan.addEventListener('click', function (e) {
              e.stopPropagation();
              block.config._activeTab = tabIdx;
              self.renderCanvas();
            });
            labelSpan.addEventListener('dblclick', function (e) {
              e.stopPropagation();
              var newLabel = prompt('Tab label:', tab.label);
              if (newLabel !== null && newLabel.trim() !== '') {
                tab.label = newLabel.trim();
                self.markDirty();
                self.renderCanvas();
              }
            });
            labelSpan.addEventListener('mousedown', function (e) { e.stopPropagation(); });
            tabBtn.appendChild(labelSpan);

            if (block.config.tabs.length > 1) {
              var removeBtn = document.createElement('button');
              removeBtn.className = 'text-fg-muted hover:text-red-500 dark:hover:text-red-400 transition-colors ml-1';
              removeBtn.title = 'Remove tab';
              removeBtn.innerHTML = '<i class="fa-solid fa-xmark text-[10px]"></i>';
              removeBtn.addEventListener('click', function (e) {
                e.stopPropagation();
                block.config.tabs.splice(tabIdx, 1);
                if (block.config._activeTab >= block.config.tabs.length) block.config._activeTab = block.config.tabs.length - 1;
                self.markDirty();
                self.renderCanvas();
              });
              removeBtn.addEventListener('mousedown', function (e) { e.stopPropagation(); });
              tabBtn.appendChild(removeBtn);
            }
            tabBar.appendChild(tabBtn);
          });
          body.appendChild(tabBar);

          var activeTab = block.config.tabs[block.config._activeTab];
          if (!activeTab.blocks) activeTab.blocks = [];
          body.appendChild(this._createSubBlockZone(activeTab.blocks, null, rowIdx, colIdx, blockIdx, 'tab_' + block.config._activeTab));
          break;
        }
        case 'section': {
          if (!block.config.blocks) block.config.blocks = [];
          var titleBar = document.createElement('div');
          titleBar.className = 'flex items-center gap-2 px-2 py-1.5 bg-surface-raised rounded mb-2 text-sm text-fg-secondary';
          var collapseIcon = block.config.collapsed ? 'fa-chevron-right' : 'fa-chevron-down';
          titleBar.innerHTML =
            '<i class="fa-solid ' + collapseIcon + ' text-xs text-fg-muted"></i>' +
            '<span class="font-medium">' + Chronicle.escapeHtml(block.config.title || 'Section') + '</span>' +
            '<span class="text-[10px] text-fg-muted ml-auto">' + (block.config.collapsed ? 'collapsed by default' : 'expanded by default') + '</span>';
          body.appendChild(titleBar);
          body.appendChild(this._createSubBlockZone(block.config.blocks, null, rowIdx, colIdx, blockIdx, 'content'));
          break;
        }
      }
    },

    // ── Sub-Block Zones (Container Drop Targets) ─────────────────

    _createSubBlockZone: function (blocks, label, rowIdx, colIdx, blockIdx, slot) {
      var self = this;
      var zone = document.createElement('div');
      zone.className = 'le-subzone border-2 border-dashed border-edge rounded min-h-[48px] p-1.5 transition-colors relative';
      zone.dataset.containerRow = rowIdx;
      zone.dataset.containerCol = colIdx;
      zone.dataset.containerBlock = blockIdx;
      zone.dataset.containerSlot = slot;

      if (label) {
        var lbl = document.createElement('div');
        lbl.className = 'text-[9px] text-fg-muted font-mono mb-0.5 px-0.5';
        lbl.textContent = label;
        zone.appendChild(lbl);
      }

      blocks.forEach(function (subBlock, subIdx) {
        zone.appendChild(self._renderSubBlock(subBlock, rowIdx, colIdx, blockIdx, slot, subIdx));
      });

      if (blocks.length === 0) {
        var hint = document.createElement('div');
        hint.className = 'le-subzone-hint text-[10px] text-fg-muted text-center py-2 italic';
        hint.textContent = 'Drop blocks here';
        zone.appendChild(hint);
      }

      zone.addEventListener('dragover', function (e) {
        e.preventDefault();
        e.stopPropagation();
        e.dataTransfer.dropEffect = 'move';
        zone.classList.add('border-accent', 'bg-accent/5');
        self._updateSubZoneDropIndicator(e, zone, rowIdx, colIdx, blockIdx, slot);
      });
      zone.addEventListener('dragleave', function (e) {
        if (!zone.contains(e.relatedTarget)) {
          zone.classList.remove('border-accent', 'bg-accent/5');
          self.clearDropIndicator();
        }
      });
      zone.addEventListener('drop', function (e) {
        e.preventDefault();
        e.stopPropagation();
        zone.classList.remove('border-accent', 'bg-accent/5');
        var insertIdx = self.dropTarget ? self.dropTarget.insertIdx : blocks.length;
        self.clearDropIndicator();
        self._handleSubBlockDrop(e, rowIdx, colIdx, blockIdx, slot, insertIdx);
      });

      return zone;
    },

    _renderSubBlock: function (subBlock, rowIdx, colIdx, blockIdx, slot, subIdx) {
      var self = this;
      var bt = this.blockTypes.find(function (b) { return b.type === subBlock.type; }) || { label: subBlock.type, icon: 'fa-cube' };
      var el = document.createElement('div');
      el.className = 'le-sub-block flex items-center gap-1.5 px-2 py-1 mb-0.5 bg-surface-raised border border-edge rounded text-xs group/sub cursor-grab hover:border-accent/50 transition-colors';
      el.draggable = true;
      el.dataset.subIdx = subIdx;
      el.innerHTML =
        '<i class="fa-solid fa-grip-vertical text-fg-muted text-[10px]"></i>' +
        '<i class="fa-solid ' + bt.icon + ' w-3 text-fg-muted text-center text-[10px]"></i>' +
        '<span class="font-medium text-fg-secondary flex-1">' + Chronicle.escapeHtml(bt.label) + '</span>' +
        '<button class="le-sub-del opacity-0 group-hover/sub:opacity-100 text-fg-muted hover:text-red-500 dark:hover:text-red-400 transition-all p-0.5" title="Remove"><i class="fa-solid fa-xmark text-[10px]"></i></button>';

      el.addEventListener('dragstart', function (e) {
        e.stopPropagation();
        e.dataTransfer.setData('text/plain', JSON.stringify({
          source: 'subblock', rowIdx: rowIdx, colIdx: colIdx, blockIdx: blockIdx, slot: slot, subIdx: subIdx, block: subBlock,
        }));
        e.dataTransfer.effectAllowed = 'move';
        el.classList.add('opacity-50');
      });
      el.addEventListener('dragend', function () {
        el.classList.remove('opacity-50');
        self.clearDropIndicator();
      });

      el.querySelector('.le-sub-del').addEventListener('click', function (e) {
        e.stopPropagation();
        var arr = self._getSubBlockArray(rowIdx, colIdx, blockIdx, slot);
        if (arr) {
          arr.splice(subIdx, 1);
          self.markDirty();
          self.renderCanvas();
        }
      });

      return el;
    },

    _getSubBlockArray: function (rowIdx, colIdx, blockIdx, slot) {
      var block = this.layout.rows[rowIdx].columns[colIdx].blocks[blockIdx];
      if (!block || !block.config) return null;
      switch (block.type) {
        case 'two_column':
          if (slot === 'left') return block.config.left;
          if (slot === 'right') return block.config.right;
          return null;
        case 'three_column':
          if (slot.indexOf('col_') === 0) {
            var i = parseInt(slot.split('_')[1], 10);
            return block.config.columns && block.config.columns[i] ? block.config.columns[i] : null;
          }
          return null;
        case 'tabs':
          if (slot.indexOf('tab_') === 0) {
            var ti = parseInt(slot.split('_')[1], 10);
            return block.config.tabs && block.config.tabs[ti] ? block.config.tabs[ti].blocks : null;
          }
          return null;
        case 'section':
          if (slot === 'content') return block.config.blocks;
          return null;
        default: return null;
      }
    },

    // ── Drop Indicator Logic ───────────────────────────────────────

    updateDropIndicator: function (e, colEl, rowIdx, colIdx) {
      var blockEls = Array.from(colEl.children).filter(function (c) { return c.classList.contains('le-block'); });
      var insertIdx = blockEls.length;
      var referenceEl = null;

      for (var i = 0; i < blockEls.length; i++) {
        var rect = blockEls[i].getBoundingClientRect();
        if (e.clientY < rect.top + rect.height / 2) {
          insertIdx = i;
          referenceEl = blockEls[i];
          break;
        }
      }

      if (this.dropTarget && this.dropTarget.rowIdx === rowIdx && this.dropTarget.colIdx === colIdx && this.dropTarget.insertIdx === insertIdx) return;

      this.clearDropIndicator();
      this.dropTarget = { rowIdx: rowIdx, colIdx: colIdx, insertIdx: insertIdx };

      var indicator = this._createIndicatorEl(3);
      if (referenceEl) colEl.insertBefore(indicator, referenceEl);
      else colEl.appendChild(indicator);

      requestAnimationFrame(function () { indicator.style.opacity = '1'; });
      this.dropIndicator = indicator;
    },

    _updateSubZoneDropIndicator: function (e, zoneEl, rowIdx, colIdx, blockIdx, slot) {
      var subEls = zoneEl.querySelectorAll('.le-sub-block');
      var insertIdx = subEls.length;
      var referenceEl = null;

      for (var i = 0; i < subEls.length; i++) {
        var rect = subEls[i].getBoundingClientRect();
        if (e.clientY < rect.top + rect.height / 2) {
          insertIdx = i;
          referenceEl = subEls[i];
          break;
        }
      }

      if (this.dropTarget && this.dropTarget.containerBlock === blockIdx && this.dropTarget.slot === slot && this.dropTarget.insertIdx === insertIdx) return;

      this.clearDropIndicator();
      this.dropTarget = { rowIdx: rowIdx, colIdx: colIdx, containerBlock: blockIdx, slot: slot, insertIdx: insertIdx };

      var indicator = this._createIndicatorEl(2);
      if (referenceEl) zoneEl.insertBefore(indicator, referenceEl);
      else zoneEl.appendChild(indicator);

      requestAnimationFrame(function () { indicator.style.opacity = '1'; });
      this.dropIndicator = indicator;
    },

    _createIndicatorEl: function (height) {
      var indicator = document.createElement('div');
      indicator.className = 'le-drop-indicator';
      indicator.style.cssText = 'height:' + height + 'px;background:#6366f1;border-radius:2px;margin:2px 4px;transition:opacity 0.15s ease;opacity:0;position:relative;';
      indicator.innerHTML = '<div style="position:absolute;inset:-2px 0;background:#6366f1;opacity:0.2;border-radius:4px;animation:le-pulse 1s ease-in-out infinite"></div>';
      return indicator;
    },

    clearDropIndicator: function () {
      if (this.dropIndicator && this.dropIndicator.parentNode) this.dropIndicator.remove();
      this.dropIndicator = null;
      this.dropTarget = null;
    },

    // ── Drop Handlers ──────────────────────────────────────────────

    // _isSingletonType reads the registry's `singleton` flag for a block
    // type. Falls back to false if the registry didn't load (palette is in
    // fallback mode) — matches server behavior where the registry is the
    // single source of truth.
    _isSingletonType: function (blockType) {
      var bt = this.blockTypes.find(function (b) { return b.type === blockType; });
      return !!(bt && bt.singleton);
    },

    // _countBlocksOfType walks the current layout and counts blocks of the
    // given type at every nesting level (top-level columns + container
    // sub-blocks). Used by the singleton drop guard.
    _countBlocksOfType: function (blockType) {
      var count = 0;
      var self = this;
      if (!this.layout || !this.layout.rows) return 0;
      this.layout.rows.forEach(function (row) {
        (row.columns || []).forEach(function (col) {
          (col.blocks || []).forEach(function (blk) {
            if (blk.type === blockType) count++;
            // Container blocks (two_column, tabs, section) hold sub-blocks;
            // walk those too so a singleton dropped inside a container
            // still counts.
            if (self.isContainer(blk.type)) {
              self._countSubBlocksOfType(blk, blockType, function (n) { count += n; });
            }
          });
        });
      });
      return count;
    },

    // _countSubBlocksOfType inspects the various sub-block storage shapes
    // (slots[], columns[].blocks[], tabs[].blocks[]) used by container
    // block configs. Defensive on shape — older container configs may not
    // have the array yet.
    _countSubBlocksOfType: function (containerBlock, blockType, addFn) {
      var cfg = containerBlock.config || {};
      var self = this;
      var walkArr = function (arr) {
        if (!Array.isArray(arr)) return;
        var n = 0;
        arr.forEach(function (b) {
          if (b && b.type === blockType) n++;
          if (b && self.isContainer(b.type)) self._countSubBlocksOfType(b, blockType, addFn);
        });
        if (n > 0) addFn(n);
      };
      // two_column / three_column store {columns: [{blocks: []}]}
      if (Array.isArray(cfg.columns)) {
        cfg.columns.forEach(function (c) { walkArr(c && c.blocks); });
      }
      // tabs stores {tabs: [{blocks: []}]}
      if (Array.isArray(cfg.tabs)) {
        cfg.tabs.forEach(function (t) { walkArr(t && t.blocks); });
      }
      // section stores {blocks: []}
      if (Array.isArray(cfg.blocks)) {
        walkArr(cfg.blocks);
      }
    },

    handleDrop: function (e, targetRowIdx, targetColIdx, insertIdx) {
      var data;
      try { data = JSON.parse(e.dataTransfer.getData('text/plain')); } catch (err) { return; }

      if (data.source === 'palette') {
        // Singleton guard: refuse the drop if this block type is declared
        // max-one-per-layout (BlockMeta.Singleton on the server) and a
        // copy is already on the canvas. The server-side ValidateLayout
        // would also reject it on save, but stopping here gives the
        // operator immediate visual feedback instead of an error toast
        // 30 seconds later when they finally hit Save.
        if (this._isSingletonType(data.type) && this._countBlocksOfType(data.type) >= 1) {
          var bt = this.blockTypes.find(function (b) { return b.type === data.type; });
          var label = (bt && bt.label) || data.type;
          if (Chronicle.notify) {
            Chronicle.notify('Only one ' + label + ' block is allowed per layout.', 'error');
          }
          return;
        }
        var config = this.defaultBlockConfig(data.type);
        if (data.widget_slug) config.widget_slug = data.widget_slug;
        var block = { id: uid('blk'), type: data.type, config: config };
        this._ensureLayout();
        this.layout.rows[targetRowIdx].columns[targetColIdx].blocks.splice(insertIdx, 0, block);
      } else if (data.source === 'canvas') {
        var sameCol = data.rowIdx === targetRowIdx && data.colIdx === targetColIdx;
        var srcBlocks = this.layout.rows[data.rowIdx].columns[data.colIdx].blocks;
        srcBlocks.splice(data.blockIdx, 1);
        var adjustedIdx = insertIdx;
        if (sameCol && data.blockIdx < insertIdx) adjustedIdx--;
        this.layout.rows[targetRowIdx].columns[targetColIdx].blocks.splice(adjustedIdx, 0, data.block);
      } else if (data.source === 'subblock') {
        var srcArr = this._getSubBlockArray(data.rowIdx, data.colIdx, data.blockIdx, data.slot);
        if (srcArr) {
          srcArr.splice(data.subIdx, 1);
          this.layout.rows[targetRowIdx].columns[targetColIdx].blocks.splice(insertIdx, 0, data.block);
        }
      }

      this.markDirty();
      this.renderCanvas();
    },

    _handleSubBlockDrop: function (e, targetRowIdx, targetColIdx, targetBlockIdx, targetSlot, insertIdx) {
      var data;
      try { data = JSON.parse(e.dataTransfer.getData('text/plain')); } catch (err) { return; }

      var targetBlocks = this._getSubBlockArray(targetRowIdx, targetColIdx, targetBlockIdx, targetSlot);
      if (!targetBlocks) return;

      // No nesting containers inside containers.
      var dropType = data.type || (data.block && data.block.type);
      if (dropType && this.isContainer(dropType)) return;

      if (data.source === 'palette') {
        // Same singleton guard as handleDrop — covers drops into
        // container slots (two_column, tabs, section).
        if (this._isSingletonType(data.type) && this._countBlocksOfType(data.type) >= 1) {
          var sbBt = this.blockTypes.find(function (b) { return b.type === data.type; });
          var sbLabel = (sbBt && sbBt.label) || data.type;
          if (Chronicle.notify) {
            Chronicle.notify('Only one ' + sbLabel + ' block is allowed per layout.', 'error');
          }
          return;
        }
        var cfg = {};
        if (data.widget_slug) cfg.widget_slug = data.widget_slug;
        targetBlocks.splice(insertIdx, 0, { id: uid('blk'), type: data.type, config: cfg });
      } else if (data.source === 'subblock') {
        var srcArr = this._getSubBlockArray(data.rowIdx, data.colIdx, data.blockIdx, data.slot);
        if (!srcArr) return;
        var sameZone = data.rowIdx === targetRowIdx && data.colIdx === targetColIdx && data.blockIdx === targetBlockIdx && data.slot === targetSlot;
        srcArr.splice(data.subIdx, 1);
        var adj = insertIdx;
        if (sameZone && data.subIdx < insertIdx) adj--;
        targetBlocks.splice(adj, 0, data.block);
      } else if (data.source === 'canvas') {
        if (data.block && this.isContainer(data.block.type)) return;
        this.layout.rows[data.rowIdx].columns[data.colIdx].blocks.splice(data.blockIdx, 1);
        targetBlocks.splice(insertIdx, 0, data.block);
      }

      this.markDirty();
      this.renderCanvas();
    },

    // ── Row Operations ─────────────────────────────────────────────

    _ensureLayout: function () {
      if (!this.layout) this.layout = { rows: [] };
    },

    addRow: function (widths) {
      this._ensureLayout();
      var columns = widths.map(function (w) { return { id: uid('col'), width: w, blocks: [] }; });
      this.layout.rows.push({ id: uid('row'), columns: columns });
      this.markDirty();
      this.renderCanvas();
    },

    changeRowLayout: function (rowIdx, widths) {
      var row = this.layout.rows[rowIdx];
      var allBlocks = [];
      row.columns.forEach(function (c) { allBlocks = allBlocks.concat(c.blocks); });
      row.columns = widths.map(function (w, i) {
        return { id: (row.columns[i] && row.columns[i].id) || uid('col'), width: w, blocks: i === 0 ? allBlocks : [] };
      });
      this.markDirty();
      this.renderCanvas();
    },

    deleteRow: function (rowIdx) {
      if (!this.layout) return;
      this.layout.rows.splice(rowIdx, 1);
      this.markDirty();
      this.renderCanvas();
    },

    moveRow: function (rowIdx, direction) {
      if (!this.layout) return;
      var newIdx = rowIdx + direction;
      if (newIdx < 0 || newIdx >= this.layout.rows.length) return;
      var tmp = this.layout.rows[rowIdx];
      this.layout.rows[rowIdx] = this.layout.rows[newIdx];
      this.layout.rows[newIdx] = tmp;
      this.markDirty();
      this.renderCanvas();
    },

    // ── Config Dialog ─────────────────────────────────────────────

    showConfigDialog: function (block, bt) {
      if (!bt.config_fields || bt.config_fields.length === 0) return;
      this._closeConfigDialog();

      var self = this;
      if (!block.config) block.config = {};

      // Backdrop.
      var backdrop = document.createElement('div');
      backdrop.className = 'le-config-backdrop';
      backdrop.style.cssText = 'position:fixed;inset:0;z-index:9998;background:rgba(0,0,0,0.4);';
      backdrop.addEventListener('click', function () { self._closeConfigDialog(); });

      // Panel.
      var panel = document.createElement('div');
      panel.className = 'le-config-panel';
      panel.style.cssText = 'position:fixed;z-index:9999;left:50%;top:50%;transform:translate(-50%,-50%);background:var(--color-card-bg,#fff);border:1px solid var(--color-border,#e5e7eb);border-radius:12px;box-shadow:0 20px 60px rgba(0,0,0,0.2);width:400px;max-width:90%;max-height:80vh;overflow-y:auto;';

      // Header.
      var header = document.createElement('div');
      header.style.cssText = 'padding:16px;border-bottom:1px solid var(--color-border-light,#f3f4f6);display:flex;align-items:center;gap:8px;';
      header.innerHTML = '<i class="fa-solid ' + bt.icon + '" style="color:var(--color-text-muted);"></i>' +
        '<span style="font-weight:600;font-size:14px;">Configure ' + Chronicle.escapeHtml(bt.label) + '</span>';
      panel.appendChild(header);

      // Form.
      var form = document.createElement('div');
      form.style.cssText = 'padding:16px;display:flex;flex-direction:column;gap:12px;';

      var fieldEls = {};
      bt.config_fields.forEach(function (field) {
        var wrapper = document.createElement('div');
        var labelEl = document.createElement('label');
        labelEl.style.cssText = 'display:block;font-size:12px;font-weight:500;color:var(--color-text-secondary);margin-bottom:4px;';
        labelEl.textContent = field.label;
        wrapper.appendChild(labelEl);

        var input;
        var currentVal = block.config[field.key] !== undefined ? block.config[field.key] : (field.default !== undefined ? field.default : '');

        switch (field.type) {
          case 'number':
            input = document.createElement('input');
            input.type = 'number';
            input.value = currentVal;
            if (field.min !== undefined && field.min !== null) input.min = field.min;
            if (field.max !== undefined && field.max !== null) input.max = field.max;
            input.className = 'w-full px-3 py-1.5 text-sm border border-edge rounded bg-surface text-fg focus:outline-none focus:ring-1 focus:ring-accent';
            break;

          case 'text':
            input = document.createElement('input');
            input.type = 'text';
            input.value = currentVal;
            input.className = 'w-full px-3 py-1.5 text-sm border border-edge rounded bg-surface text-fg focus:outline-none focus:ring-1 focus:ring-accent';
            break;

          case 'textarea':
            input = document.createElement('textarea');
            input.value = currentVal;
            input.rows = 4;
            input.className = 'w-full px-3 py-1.5 text-sm border border-edge rounded bg-surface text-fg focus:outline-none focus:ring-1 focus:ring-accent resize-y';
            break;

          case 'select':
            input = document.createElement('select');
            input.className = 'w-full px-3 py-1.5 text-sm border border-edge rounded bg-surface text-fg focus:outline-none focus:ring-1 focus:ring-accent';
            (field.options || []).forEach(function (opt) {
              var o = document.createElement('option');
              o.value = opt.value;
              o.textContent = opt.label;
              if (String(currentVal) === String(opt.value)) o.selected = true;
              input.appendChild(o);
            });
            break;

          case 'entity_type':
            input = document.createElement('select');
            input.className = 'w-full px-3 py-1.5 text-sm border border-edge rounded bg-surface text-fg focus:outline-none focus:ring-1 focus:ring-accent';
            var emptyOpt = document.createElement('option');
            emptyOpt.value = '';
            emptyOpt.textContent = '— Select entity type —';
            input.appendChild(emptyOpt);
            // Fetch entity types.
            if (self.campaignId) {
              Chronicle.apiFetch('/campaigns/' + self.campaignId + '/entity-types')
                .then(function (r) { return r.ok ? r.json() : []; })
                .then(function (types) {
                  (types || []).forEach(function (et) {
                    var o = document.createElement('option');
                    o.value = et.id;
                    o.textContent = et.name;
                    if (String(currentVal) === String(et.id)) o.selected = true;
                    input.appendChild(o);
                  });
                })
                .catch(function () {});
            }
            break;

          case 'map':
            // Renders the campaign's maps as a dropdown. Without this, the
            // map_preview / map_full block configs had no map_id picker —
            // the layout editor would skip the field, the renderer would
            // get an empty map_id, and the map widget would fall through
            // to the "Configure a map for this block. View all maps."
            // placeholder. Hits the v1 JSON API (the web /maps endpoint
            // is HTML-only).
            input = document.createElement('select');
            input.className = 'w-full px-3 py-1.5 text-sm border border-edge rounded bg-surface text-fg focus:outline-none focus:ring-1 focus:ring-accent';
            var mapEmptyOpt = document.createElement('option');
            mapEmptyOpt.value = '';
            mapEmptyOpt.textContent = '— Select map —';
            input.appendChild(mapEmptyOpt);
            if (self.campaignId) {
              Chronicle.apiFetch('/api/v1/campaigns/' + self.campaignId + '/maps')
                .then(function (r) { return r.ok ? r.json() : []; })
                .then(function (mapList) {
                  // ListMaps returns a bare array; older responses may
                  // wrap it in { data: [...] }. Handle both defensively
                  // since the response shape could change without
                  // breaking other callers.
                  var arr = Array.isArray(mapList) ? mapList : (mapList && mapList.data) || [];
                  arr.forEach(function (m) {
                    var o = document.createElement('option');
                    o.value = m.id;
                    o.textContent = m.name || '(unnamed map)';
                    if (String(currentVal) === String(m.id)) o.selected = true;
                    input.appendChild(o);
                  });
                })
                .catch(function () {});
            }
            break;

          default:
            input = document.createElement('input');
            input.type = 'text';
            input.value = currentVal;
            input.className = 'w-full px-3 py-1.5 text-sm border border-edge rounded bg-surface text-fg focus:outline-none focus:ring-1 focus:ring-accent';
        }

        wrapper.appendChild(input);
        form.appendChild(wrapper);
        fieldEls[field.key] = { el: input, field: field };
      });

      panel.appendChild(form);

      // Footer with Save/Cancel.
      var footer = document.createElement('div');
      footer.style.cssText = 'padding:12px 16px;border-top:1px solid var(--color-border-light,#f3f4f6);display:flex;justify-content:flex-end;gap:8px;';

      var cancelBtn = document.createElement('button');
      cancelBtn.className = 'btn-secondary text-sm px-4 py-1.5';
      cancelBtn.textContent = 'Cancel';
      cancelBtn.addEventListener('click', function () { self._closeConfigDialog(); });

      var saveBtn = document.createElement('button');
      saveBtn.className = 'btn-primary text-sm px-4 py-1.5';
      saveBtn.textContent = 'Apply';
      saveBtn.addEventListener('click', function () {
        // Read values from fields.
        Object.keys(fieldEls).forEach(function (key) {
          var fe = fieldEls[key];
          var val = fe.el.value;
          if (fe.field.type === 'number') {
            val = parseInt(val, 10);
            if (isNaN(val)) val = fe.field.default || 0;
          }
          if (val === '' || val === undefined) {
            delete block.config[key];
          } else {
            block.config[key] = val;
          }
        });
        self.markDirty();
        self.renderCanvas();
        self._closeConfigDialog();
      });

      footer.appendChild(cancelBtn);
      footer.appendChild(saveBtn);
      panel.appendChild(footer);

      document.body.appendChild(backdrop);
      document.body.appendChild(panel);
      this._configBackdrop = backdrop;
      this._configPanel = panel;

      this._configEscHandler = function (e) {
        if (e.key === 'Escape') self._closeConfigDialog();
      };
      document.addEventListener('keydown', this._configEscHandler);
    },

    _closeConfigDialog: function () {
      if (this._configBackdrop) { this._configBackdrop.remove(); this._configBackdrop = null; }
      if (this._configPanel) { this._configPanel.remove(); this._configPanel = null; }
      if (this._configEscHandler) { document.removeEventListener('keydown', this._configEscHandler); this._configEscHandler = null; }
    },

    // ── Block Preview (template context) ───────────────────────────

    showBlockPreview: function (block, x, y) {
      this._closeBlockPreview();
      var bt = this.blockTypes.find(function (b) { return b.type === block.type; }) || { label: block.type, icon: 'fa-cube', desc: '' };

      var backdrop = document.createElement('div');
      backdrop.className = 'le-preview-backdrop';
      backdrop.style.cssText = 'position:fixed;inset:0;z-index:9998;background:rgba(0,0,0,0.3);';
      var self = this;
      backdrop.addEventListener('click', function () { self._closeBlockPreview(); });

      var panel = document.createElement('div');
      panel.className = 'le-preview-panel';
      panel.style.cssText = 'position:fixed;z-index:9999;left:50%;top:50%;transform:translate(-50%,-50%);background:var(--color-card-bg,#fff);border:1px solid var(--color-border,#e5e7eb);border-radius:12px;box-shadow:0 20px 60px rgba(0,0,0,0.2);max-width:480px;width:90%;overflow:hidden;';

      // Header.
      var header = document.createElement('div');
      header.style.cssText = 'padding:12px 16px;border-bottom:1px solid var(--color-border-light,#f3f4f6);display:flex;align-items:center;gap:8px;';
      header.innerHTML = '<i class="fa-solid ' + bt.icon + '" style="color:var(--color-text-muted);font-size:14px;"></i>' +
        '<span style="font-weight:600;font-size:14px;color:var(--color-text-primary);">' + Chronicle.escapeHtml(bt.label) + ' Preview</span>' +
        '<span style="flex:1"></span>' +
        '<span style="font-size:11px;color:var(--color-text-muted);padding:2px 8px;border-radius:4px;background:var(--color-bg-tertiary);">Mock preview</span>';

      var content = document.createElement('div');
      content.style.cssText = 'padding:16px;';
      var preview = document.createElement('div');
      preview.style.cssText = 'border:2px solid #6366f1;border-radius:8px;padding:16px;background:var(--color-bg-secondary,#fff);position:relative;';
      var indicator = document.createElement('div');
      indicator.style.cssText = 'position:absolute;top:-10px;left:12px;background:#6366f1;color:white;font-size:10px;font-weight:600;padding:1px 8px;border-radius:4px;';
      indicator.textContent = bt.label;
      preview.appendChild(indicator);
      preview.appendChild(this._createBlockMockup(block.type));
      content.appendChild(preview);

      var footer = document.createElement('div');
      footer.style.cssText = 'padding:12px 16px;border-top:1px solid var(--color-border-light,#f3f4f6);font-size:12px;color:var(--color-text-secondary);';
      footer.textContent = bt.desc;

      panel.appendChild(header);
      panel.appendChild(content);
      panel.appendChild(footer);

      document.body.appendChild(backdrop);
      document.body.appendChild(panel);
      this._previewBackdrop = backdrop;
      this._previewPanel = panel;

      this._previewEscHandler = function (e) { if (e.key === 'Escape') self._closeBlockPreview(); };
      document.addEventListener('keydown', this._previewEscHandler);
    },

    _closeBlockPreview: function () {
      if (this._previewBackdrop) { this._previewBackdrop.remove(); this._previewBackdrop = null; }
      if (this._previewPanel) { this._previewPanel.remove(); this._previewPanel = null; }
      if (this._previewEscHandler) { document.removeEventListener('keydown', this._previewEscHandler); this._previewEscHandler = null; }
    },

    _createBlockMockup: function (type) {
      var mock = document.createElement('div');
      mock.style.color = 'var(--color-text-body,#374151)';
      switch (type) {
        case 'title':
          mock.innerHTML = '<div style="display:flex;align-items:center;justify-content:space-between;"><div style="font-size:24px;font-weight:700;color:var(--color-text-primary,#111827);">Entity Name</div><div style="display:flex;gap:6px;"><span style="padding:4px 12px;font-size:12px;background:var(--color-bg-tertiary);border:1px solid var(--color-border);border-radius:6px;color:var(--color-text-secondary);">Edit</span></div></div>';
          break;
        case 'image':
          mock.innerHTML = '<div style="background:var(--color-bg-tertiary);border-radius:8px;height:140px;display:flex;align-items:center;justify-content:center;color:var(--color-text-muted);"><i class="fa-solid fa-image" style="font-size:32px;opacity:0.4;"></i></div>';
          break;
        case 'entry':
          mock.innerHTML = '<div style="border:1px solid var(--color-border);border-radius:8px;overflow:hidden;"><div style="padding:8px 12px;border-bottom:1px solid var(--color-border-light);font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;color:var(--color-text-secondary);">Entry</div><div style="padding:16px;font-size:13px;line-height:1.6;"><p style="margin:0 0 8px;">Lorem ipsum dolor sit amet, consectetur adipiscing elit.</p><p style="margin:0;color:var(--color-text-secondary);">Ut enim ad minim veniam.</p></div></div>';
          break;
        case 'attributes':
          var fieldMocks = this.fields && this.fields.length > 0
            ? this.fields.slice(0, 4).map(function (f) {
                return '<div style="margin-bottom:8px;"><div style="font-size:10px;font-weight:500;text-transform:uppercase;letter-spacing:0.05em;color:var(--color-text-secondary);">' + Chronicle.escapeHtml(f.label) + '</div><div style="font-size:13px;color:var(--color-text-primary);margin-top:2px;">Sample value</div></div>';
              }).join('')
            : '<div style="font-size:12px;color:var(--color-text-muted);">No fields defined</div>';
          mock.innerHTML = '<div style="border:1px solid var(--color-border);border-radius:8px;padding:12px;"><div style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;color:var(--color-text-secondary);margin-bottom:10px;">Attributes</div>' + fieldMocks + '</div>';
          break;
        case 'tags':
          mock.innerHTML = '<div style="display:flex;flex-wrap:wrap;gap:4px;"><span style="padding:2px 8px;border-radius:999px;font-size:11px;font-weight:500;background:#6366f122;color:#6366f1;">Important</span><span style="padding:2px 8px;border-radius:999px;font-size:11px;font-weight:500;background:#22c55e22;color:#22c55e;">Ally</span></div>';
          break;
        case 'relations':
          mock.innerHTML = '<div style="border:1px solid var(--color-border);border-radius:8px;overflow:hidden;"><div style="padding:6px 10px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;color:var(--color-text-secondary);border-bottom:1px solid var(--color-border-light);"><i class="fa-solid fa-link" style="font-size:9px"></i> Allied With</div><div style="padding:8px 10px;display:flex;align-items:center;gap:6px;"><span style="width:24px;height:24px;border-radius:50%;background:#22c55e;display:flex;align-items:center;justify-content:center;"><i class="fa-solid fa-user" style="font-size:10px;color:white;"></i></span><span style="font-size:13px;font-weight:500;color:var(--color-text-primary);">Another Entity</span></div></div>';
          break;
        case 'divider':
          mock.innerHTML = '<hr style="border:none;border-top:1px solid var(--color-border);margin:8px 0;" />';
          break;
        default:
          mock.innerHTML = '<div style="font-size:13px;color:var(--color-text-muted);">Preview not available for this block type.</div>';
      }
      return mock;
    },

    // ── Preset System (template context) ───────────────────────────

    showLoadPresetMenu: function (anchorEl) {
      this._closePresetMenu();
      var self = this;

      var menu = document.createElement('div');
      menu.className = 'le-preset-menu absolute z-50 mt-1 w-56 bg-surface border border-edge rounded-lg shadow-lg overflow-hidden';
      menu.style.cssText = 'max-height:300px;overflow-y:auto;';
      menu.innerHTML = '<div class="px-3 py-2 text-xs text-fg-muted">Loading presets...</div>';

      var rect = anchorEl.getBoundingClientRect();
      var paletteRect = anchorEl.closest('.w-56').getBoundingClientRect();
      menu.style.position = 'fixed';
      menu.style.left = paletteRect.left + 'px';
      menu.style.top = (rect.bottom + 4) + 'px';
      menu.style.width = paletteRect.width + 'px';
      document.body.appendChild(menu);
      this._presetMenu = menu;

      this._presetMenuClickHandler = function (e) {
        if (!menu.contains(e.target) && e.target !== anchorEl) self._closePresetMenu();
      };
      setTimeout(function () { document.addEventListener('click', self._presetMenuClickHandler); }, 0);

      Chronicle.apiFetch('/campaigns/' + this.campaignId + '/layout-presets')
        .then(function (r) { return r.ok ? r.json() : []; })
        .then(function (presets) {
          menu.innerHTML = '';
          if (!presets || presets.length === 0) {
            menu.innerHTML = '<div class="px-3 py-2 text-xs text-fg-muted">No presets available</div>';
            return;
          }
          presets.forEach(function (preset) {
            var item = document.createElement('button');
            item.className = 'flex items-center gap-2 w-full px-3 py-2 text-sm text-left text-fg hover:bg-accent/10 transition-colors';
            item.innerHTML = '<i class="fa-solid ' + Chronicle.escapeHtml(preset.icon || 'fa-table-columns') + ' w-4 text-fg-muted text-center"></i>' +
              '<div><div class="font-medium">' + Chronicle.escapeHtml(preset.name) + '</div>' +
              (preset.description ? '<div class="text-[10px] text-fg-muted">' + Chronicle.escapeHtml(preset.description) + '</div>' : '') +
              '</div>' +
              (preset.is_builtin ? '<span class="ml-auto text-[9px] text-fg-muted border border-edge rounded px-1">Built-in</span>' : '');
            item.addEventListener('click', function () {
              self._closePresetMenu();
              self._loadPreset(preset);
            });
            menu.appendChild(item);
          });
        })
        .catch(function () {
          menu.innerHTML = '<div class="px-3 py-2 text-xs text-red-400">Failed to load presets</div>';
        });
    },

    _closePresetMenu: function () {
      if (this._presetMenu) { this._presetMenu.remove(); this._presetMenu = null; }
      if (this._presetMenuClickHandler) { document.removeEventListener('click', this._presetMenuClickHandler); this._presetMenuClickHandler = null; }
    },

    _loadPreset: function (preset) {
      if (!confirm('Replace current layout with "' + preset.name + '"? This cannot be undone.')) return;
      try {
        var layout = JSON.parse(preset.layout_json);
        if (layout && layout.rows) {
          this.layout = layout;
          this.markDirty();
          this.renderCanvas();
          var status = this._findSaveStatus();
          if (status) status.textContent = 'Preset loaded — save to apply';
        }
      } catch (e) {
        var st = this._findSaveStatus();
        if (st) st.textContent = 'Error loading preset';
      }
    },

    saveAsPreset: function () {
      var name = prompt('Preset name:');
      if (!name || !name.trim()) return;
      var self = this;
      var status = this._findSaveStatus();

      var cleanLayout = this._cleanLayoutForSave(this.layout);
      Chronicle.apiFetch('/campaigns/' + this.campaignId + '/layout-presets', {
        method: 'POST',
        body: { name: name.trim(), description: '', layout_json: JSON.stringify(cleanLayout), icon: 'fa-table-columns' },
      })
        .then(function (res) {
          if (!res.ok) throw new Error('Failed to save preset');
          if (status) status.textContent = 'Preset saved';
          setTimeout(function () { if (status && !self.dirty) status.textContent = ''; }, 2000);
        })
        .catch(function (err) {
          if (status) { status.textContent = 'Error: ' + err.message; status.classList.add('text-red-500'); }
          setTimeout(function () { if (status) { status.textContent = ''; status.classList.remove('text-red-500'); } }, 4000);
        });
    },

    // ── Save / Load ──────────────────────────────────────────────

    _findSaveBtn: function () {
      var container = this.el.closest('[data-te-container]');
      return (container && container.querySelector('#te-save-btn')) || document.getElementById('te-save-btn');
    },

    _findSaveStatus: function () {
      var container = this.el.closest('[data-te-container]');
      return (container && container.querySelector('#te-save-status')) || document.getElementById('te-save-status');
    },

    markDirty: function () {
      this.dirty = true;
      var status = this._findSaveStatus();
      if (status) status.textContent = 'Unsaved changes';
      var btn = this._findSaveBtn();
      if (btn) btn.classList.add('animate-pulse');
    },

    _bindSave: function () {
      var self = this;
      var btn = this._findSaveBtn();
      if (btn) btn.addEventListener('click', function () { self.save(); });

      this._keydownHandler = function (e) {
        if ((e.ctrlKey || e.metaKey) && e.key === 's') {
          e.preventDefault();
          self.save();
        }
      };
      document.addEventListener('keydown', this._keydownHandler);
    },

    _cleanLayoutForSave: function (layout) {
      return JSON.parse(JSON.stringify(layout, function (key, value) {
        if (key.charAt(0) === '_') return undefined;
        return value;
      }));
    },

    save: function (callback) {
      var self = this;
      if (!this.layout) {
        if (callback) callback();
        return;
      }

      var btn = this._findSaveBtn();
      var status = this._findSaveStatus();
      if (btn) { btn.disabled = true; btn.textContent = 'Saving...'; btn.classList.remove('animate-pulse'); }

      var cleanLayout = this._cleanLayoutForSave(this.layout);
      var body = this.context === 'template' ? { layout: cleanLayout } : cleanLayout;

      Chronicle.apiFetch(this.endpoint, { method: 'PUT', body: body })
        .then(function (res) {
          var label = self.context === 'template' ? 'template' : 'dashboard layout';
          if (!res.ok) {
            // Surface the server's specific message — ValidateLayout
            // returns operator-friendly text like "Only one Map Editor
            // block is allowed per layout." Generic "Failed to save"
            // hides that detail and was the source of multiple
            // mystery-failure bug reports.
            return res.json().then(
              function (body) {
                var msg = (body && (body.message || body.error)) || ('Failed to save ' + label);
                Chronicle.notify(msg, 'error');
              },
              function () {
                Chronicle.notify('Failed to save ' + label + ' (HTTP ' + res.status + ')', 'error');
              }
            );
          }
          self.dirty = false;
          var successLabel = self.context === 'template' ? 'Template saved' : 'Dashboard layout saved';
          if (status) { status.textContent = 'Saved'; setTimeout(function () { if (status && !self.dirty) status.textContent = ''; }, 2000); }
          Chronicle.notify(successLabel, 'success');
        })
        .catch(function (err) {
          var label = self.context === 'template' ? 'template' : 'dashboard layout';
          // Distinguish network failures from server-rejected saves so
          // the operator knows whether to retry vs. fix their layout.
          var msg = (err && err.message) ? ('Network error saving ' + label + ': ' + err.message) : ('Network error saving ' + label);
          Chronicle.notify(msg, 'error');
        })
        .finally(function () {
          if (btn) { btn.disabled = false; btn.textContent = self.context === 'template' ? 'Save Template' : 'Save Layout'; }
          if (callback) callback();
        });
    },

    load: function () {
      var self = this;
      Chronicle.apiFetch(this.endpoint)
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          return res.json();
        })
        .then(function (data) {
          self.layout = data;
          self.render();
          self._bindSave();
        })
        .catch(function () {
          self.layout = null;
          self.render();
          self._bindSave();
        });
    },

    resetLayout: function () {
      var self = this;
      if (!confirm('Reset to default? This will remove your custom layout.')) return;

      Chronicle.apiFetch(this.endpoint, { method: 'DELETE' })
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          self.layout = null;
          self.dirty = false;
          Chronicle.notify('Layout reset to default', 'success');
          self.render();
        })
        .catch(function () {
          Chronicle.notify('Failed to reset layout', 'error');
        });
    },

    // ── Destroy ────────────────────────────────────────────────────

    destroy: function (el) {
      if (this._keydownHandler) {
        document.removeEventListener('keydown', this._keydownHandler);
        this._keydownHandler = null;
      }
      if (this._onRoleChange) {
        this.el.removeEventListener('role-change', this._onRoleChange);
      }
      this._closePresetMenu();
      this._closeBlockPreview();
      this._closeConfigDialog();
      el.innerHTML = '';
      this.layout = null;
      this.canvas = null;
      this.fields = null;
      this.el = null;
    },
  });

  // ── Style Injection ────────────────────────────────────────────

  if (!document.getElementById('le-styles')) {
    var style = document.createElement('style');
    style.id = 'le-styles';
    style.textContent = [
      '@keyframes le-pulse { 0%, 100% { opacity: 0.2; } 50% { opacity: 0.4; } }',
      '.le-container-block .le-subzone { min-height: 36px; }',
      '.le-container-block .le-subzone { background: var(--color-bg-tertiary, rgba(249,250,251,0.5)); }',
      '.dark .le-container-block .le-subzone { background: var(--color-bg-tertiary, rgba(55,65,81,0.3)); }',
      '.le-subzone.border-accent .le-subzone-hint { display: none; }',
      '.le-container-block > .p-2 { cursor: default; }',
    ].join('\n');
    document.head.appendChild(style);
  }
})();
