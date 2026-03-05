/**
 * timeline_viz.js -- D3.js Interactive Timeline Visualization Widget
 *
 * Renders an interactive SVG timeline with:
 *   - Center spine ruler with multi-tier ticks (primary/secondary/tertiary)
 *   - Horizontal time axis with 6 zoom levels (era→day)
 *   - Event markers with zoom-level-dependent visual styles
 *   - Range bars for multi-day events (horizontal colored bars)
 *   - Event clustering at low zoom (era/century) with count badges
 *   - Category-based event icons at year/month zoom
 *   - Calendar era background bands with watermark labels
 *   - Mini-map overview strip with viewport indicator
 *   - Pan/drag via d3.zoom, scroll wheel zoom
 *   - Tooltips on hover (event name, date, entity, category)
 *   - Color-coded events (per-link override or timeline default color)
 *   - Entity group swim-lanes (when groups exist)
 *   - Clickable zoom level buttons, zoom fit, search/filter bar
 *   - Event detail panel on click
 *   - Event connections: SVG lines/arrows between related events
 *   - Create-from-timeline: double-click empty space to create event at that date
 *
 * Zoom levels and visual styles:
 *   Era     — Small dots with subtle glow (clustered when dense)
 *   Century — Small circles with glow effect (clustered when dense)
 *   Decade  — Medium circles with category color coding
 *   Year    — Circles/icons with labels, date annotations
 *   Month   — Pill-shaped markers with label backgrounds
 *   Day     — Card-style markers with full event detail
 *
 * Mount: <div data-widget="timeline-viz"
 *             data-campaign-id="..."
 *             data-timeline-id="..."
 *             data-timeline-color="..."
 *             data-api-url="...">
 *
 * Requires D3.js v7. If D3 is not yet loaded when the widget mounts (e.g.
 * during HTMX navigation), it is loaded dynamically from the CDN.
 *
 * Load-order safe: if boot.js hasn't executed yet (Chronicle undefined),
 * registration is deferred until DOMContentLoaded.
 */
(function() {
var _impl = {
  /** CDN URL used to dynamically load D3 when it's not already available. */
  _d3Src: 'https://cdn.jsdelivr.net/npm/d3@7/dist/d3.min.js',

  /**
   * Target scale factors for each zoom level (midpoints of threshold ranges).
   * Used when clicking a zoom level button to animate to that level.
   */
  _zoomTargets: {
    era: 0.15, century: 0.55, decade: 1.4, year: 5, month: 16.5, day: 37
  },

  /**
   * Initialize the timeline visualization.
   * @param {HTMLElement} el - Mount point element.
   * @param {Object} config - Parsed data-* attributes.
   */
  init: function(el, config) {
    var self = this;
    this.el = el;
    this.config = config;
    this.apiUrl = config.apiUrl;
    this.timelineColor = config.timelineColor || '#6366f1';
    this.events = [];
    this.groups = [];
    this.eras = [];
    this.connections = [];
    this.timeline = null;
    this.tooltip = null;
    this.detailPanel = null;
    this.svg = null;
    this.zoom = null;
    this.currentTransform = null;
    this.searchQuery = '';
    this.currentZoomLevel = 'year';

    // Layout constants.
    this.margin = { top: 40, right: 40, bottom: 10, left: 40 };
    this.rowHeight = 36;
    this.laneHeight = 50;

    // Zoom level thresholds (scale factor k).
    this.zoomThresholds = {
      era:     { max: 0.3 },
      century: { min: 0.3, max: 0.8 },
      decade:  { min: 0.8, max: 2 },
      year:    { min: 2, max: 8 },
      month:   { min: 8, max: 25 },
      day:     { min: 25 }
    };

    // Visual config per zoom level.
    this.zoomStyles = {
      era:     { radius: 3,  showLabel: false, showDate: false, showEntity: false, strokeWidth: 1 },
      century: { radius: 4,  showLabel: false, showDate: false, showEntity: false, strokeWidth: 1.5 },
      decade:  { radius: 5,  showLabel: false, showDate: false, showEntity: false, strokeWidth: 2 },
      year:    { radius: 6,  showLabel: true,  showDate: false, showEntity: false, strokeWidth: 2 },
      month:   { radius: 8,  showLabel: true,  showDate: true,  showEntity: true,  strokeWidth: 2 },
      day:     { radius: 10, showLabel: true,  showDate: true,  showEntity: true,  strokeWidth: 2.5 }
    };

    // Build toolbar/container first so the user sees something immediately.
    this._buildDOM();

    // Ensure D3 is available before loading data. During HTMX navigation the
    // CDN script tag in the template may not have executed yet.
    if (typeof d3 !== 'undefined') {
      this.currentTransform = d3.zoomIdentity;
      this._loadData();
    } else {
      this._ensureD3(function() {
        self.currentTransform = d3.zoomIdentity;
        self._loadData();
      });
    }
  },

  /**
   * Dynamically load D3.js from the CDN if it hasn't been loaded yet.
   * Shows a loading indicator while waiting, and an error if it fails.
   * @param {Function} cb - Callback invoked once D3 is available.
   */
  _ensureD3: function(cb) {
    var self = this;

    // Show loading state in the SVG container area.
    this.svgContainer.innerHTML =
      '<div class="timeline-viz-empty">' +
        '<i class="fa-solid fa-spinner fa-spin text-2xl mb-3" style="color: var(--color-fg-muted)"></i>' +
        '<p class="text-sm" style="color: var(--color-fg-muted)">Loading visualization...</p>' +
      '</div>';

    // Check if another script element is already loading D3.
    var existing = document.querySelector('script[src="' + this._d3Src + '"]');
    if (existing) {
      // D3 script tag exists but hasn't finished loading. Wait for it.
      var attempts = 0;
      var poll = setInterval(function() {
        attempts++;
        if (typeof d3 !== 'undefined') {
          clearInterval(poll);
          cb();
        } else if (attempts > 100) {
          clearInterval(poll);
          self._showD3Error();
        }
      }, 100);
      return;
    }

    // Load D3 dynamically.
    var script = document.createElement('script');
    script.src = this._d3Src;
    script.onload = function() { cb(); };
    script.onerror = function() { self._showD3Error(); };
    document.head.appendChild(script);
  },

  /**
   * Show a user-friendly error when D3 fails to load.
   */
  _showD3Error: function() {
    this.svgContainer.innerHTML =
      '<div class="timeline-viz-empty">' +
        '<i class="fa-solid fa-triangle-exclamation text-3xl mb-3" style="color: var(--color-fg-muted)"></i>' +
        '<p style="font-weight: 600">Unable to load visualization library</p>' +
        '<p class="text-xs mt-1" style="color: var(--color-fg-muted)">' +
          'D3.js failed to load. Check your internet connection or try refreshing the page.</p>' +
      '</div>';
  },

  /**
   * Build the widget DOM structure (toolbar + SVG container + minimap + panels).
   */
  _buildDOM: function() {
    this.el.innerHTML = '';
    this.el.classList.add('timeline-viz-container');
    var self = this;

    // Toolbar.
    var toolbar = document.createElement('div');
    toolbar.className = 'timeline-viz-toolbar';
    toolbar.innerHTML =
      '<div class="timeline-viz-toolbar-left">' +
        '<button class="timeline-viz-btn" data-action="zoom-in" title="Zoom in">' +
          '<i class="fa-solid fa-plus"></i>' +
        '</button>' +
        '<button class="timeline-viz-btn" data-action="zoom-out" title="Zoom out">' +
          '<i class="fa-solid fa-minus"></i>' +
        '</button>' +
        '<button class="timeline-viz-btn" data-action="zoom-fit" title="Fit all events">' +
          '<i class="fa-solid fa-expand"></i>' +
        '</button>' +
        '<div class="timeline-viz-zoom-buttons">' +
          '<button class="timeline-viz-zoom-btn" data-zoom-level="era">Era</button>' +
          '<button class="timeline-viz-zoom-btn" data-zoom-level="century">Cen</button>' +
          '<button class="timeline-viz-zoom-btn" data-zoom-level="decade">Dec</button>' +
          '<button class="timeline-viz-zoom-btn active" data-zoom-level="year">Year</button>' +
          '<button class="timeline-viz-zoom-btn" data-zoom-level="month">Mon</button>' +
          '<button class="timeline-viz-zoom-btn" data-zoom-level="day">Day</button>' +
        '</div>' +
      '</div>' +
      '<div class="timeline-viz-toolbar-center">' +
        '<div class="timeline-viz-search-wrap">' +
          '<i class="fa-solid fa-search timeline-viz-search-icon"></i>' +
          '<input type="text" class="timeline-viz-search-input" placeholder="Filter events..."/>' +
          '<button class="timeline-viz-search-clear" data-action="search-clear" style="display:none">' +
            '<i class="fa-solid fa-xmark"></i>' +
          '</button>' +
        '</div>' +
      '</div>' +
      '<div class="timeline-viz-toolbar-right">' +
        '<span class="timeline-viz-range"></span>' +
        '<input type="number" class="timeline-viz-skip-input" placeholder="Year" title="Go to year"/>' +
        '<button class="timeline-viz-btn" data-action="skip-to" title="Go to year">' +
          '<i class="fa-solid fa-arrow-right"></i>' +
        '</button>' +
      '</div>';
    this.el.appendChild(toolbar);

    // SVG container with loading skeleton.
    var svgContainer = document.createElement('div');
    svgContainer.className = 'timeline-viz-svg-wrap';
    svgContainer.innerHTML =
      '<div class="timeline-viz-skeleton">' +
        '<div class="timeline-viz-skeleton-bar" style="width:70%; margin-left:15%"></div>' +
        '<div class="timeline-viz-skeleton-dots">' +
          '<span></span><span></span><span></span><span></span><span></span>' +
        '</div>' +
        '<div class="timeline-viz-skeleton-bar" style="width:50%; margin-left:25%"></div>' +
      '</div>';
    this.el.appendChild(svgContainer);
    this.svgContainer = svgContainer;

    // Mini-map container (below SVG).
    var minimapContainer = document.createElement('div');
    minimapContainer.className = 'timeline-viz-minimap';
    this.el.appendChild(minimapContainer);
    this.minimapContainer = minimapContainer;

    // Tooltip (uses opacity transition via .visible class).
    var tip = document.createElement('div');
    tip.className = 'timeline-viz-tooltip';
    document.body.appendChild(tip);
    this.tooltip = tip;

    // Detail panel (floating, shown on event click).
    var detail = document.createElement('div');
    detail.className = 'timeline-viz-detail';
    detail.innerHTML =
      '<button class="timeline-viz-detail-close"><i class="fa-solid fa-xmark"></i></button>' +
      '<div class="timeline-viz-detail-body"></div>';
    document.body.appendChild(detail);
    this.detailPanel = detail;

    // Close detail panel on button click or Escape key.
    detail.querySelector('.timeline-viz-detail-close').addEventListener('click', function() {
      self._hideDetail();
    });
    document.addEventListener('keydown', function(e) {
      if (e.key === 'Escape') self._hideDetail();
    });
    // Close on click outside.
    document.addEventListener('mousedown', function(e) {
      if (self.detailPanel.classList.contains('visible') &&
          !self.detailPanel.contains(e.target) &&
          !e.target.closest('.timeline-event')) {
        self._hideDetail();
      }
    });

    // Wire toolbar buttons.
    toolbar.addEventListener('click', function(e) {
      var btn = e.target.closest('[data-action]');
      if (btn) {
        var action = btn.getAttribute('data-action');
        if (action === 'zoom-in') self._zoomBy(1.5);
        else if (action === 'zoom-out') self._zoomBy(1 / 1.5);
        else if (action === 'zoom-fit') self._zoomFit();
        else if (action === 'skip-to') self._skipToYear();
        else if (action === 'search-clear') self._clearSearch();
        return;
      }
      // Zoom level buttons.
      var zoomBtn = e.target.closest('[data-zoom-level]');
      if (zoomBtn) {
        self._jumpToZoomLevel(zoomBtn.getAttribute('data-zoom-level'));
      }
    });

    // Enter key on skip input.
    var skipInput = toolbar.querySelector('.timeline-viz-skip-input');
    skipInput.addEventListener('keydown', function(e) {
      if (e.key === 'Enter') self._skipToYear();
    });

    // Search input.
    var searchInput = toolbar.querySelector('.timeline-viz-search-input');
    var clearBtn = toolbar.querySelector('.timeline-viz-search-clear');
    searchInput.addEventListener('input', function() {
      self.searchQuery = this.value.toLowerCase();
      clearBtn.style.display = self.searchQuery ? '' : 'none';
      self._applySearchFilter();
    });
  },

  /**
   * Fetch timeline data from the API and render.
   */
  _loadData: function() {
    var self = this;
    fetch(this.apiUrl)
      .then(function(r) { return r.json(); })
      .then(function(data) {
        self.timeline = data.timeline;
        self.groups = data.groups || [];
        self.eras = data.eras || [];
        self.connections = data.connections || [];

        // Filter out events with missing or NaN date fields. This guards
        // against omitted JSON fields (e.g. zero-value ints with omitempty)
        // that would produce NaN positions in the D3 scale.
        self.events = (data.events || []).filter(function(e) {
          var y = e.event_year;
          // Year 0 is valid; only reject undefined/null/NaN.
          if (y == null || y !== y) return false;
          // Default missing month/day to 1 (safe fallback).
          if (e.event_month == null || e.event_month !== e.event_month) e.event_month = 1;
          if (e.event_day == null || e.event_day !== e.event_day) e.event_day = 1;
          return true;
        });

        // Sort events by date.
        self.events.sort(function(a, b) {
          var ya = a.event_year, yb = b.event_year;
          if (ya !== yb) return ya - yb;
          var ma = a.event_month, mb = b.event_month;
          if (ma !== mb) return ma - mb;
          return a.event_day - b.event_day;
        });

        self._render();
      })
      .catch(function(err) {
        console.warn('[timeline-viz] Failed to load data:', err);
        self.svgContainer.innerHTML =
          '<div class="timeline-viz-empty">' +
            '<i class="fa-solid fa-triangle-exclamation text-3xl mb-3" style="color: var(--color-fg-muted)"></i>' +
            '<p style="font-weight: 600">Failed to load timeline data</p>' +
            '<p class="text-xs mt-1" style="color: var(--color-fg-muted)">' + self._escapeHTML(String(err)) + '</p>' +
          '</div>';
      });
  },

  /**
   * Main render: build scales, axes, and event markers.
   */
  _render: function() {
    if (this.events.length === 0) {
      this.svgContainer.innerHTML =
        '<div class="timeline-viz-empty">' +
          '<div class="timeline-viz-empty-line"></div>' +
          '<i class="fa-solid fa-calendar-plus text-4xl mb-3" style="color: var(--color-fg-muted)"></i>' +
          '<p style="font-weight: 600">No events to display</p>' +
          '<p class="text-xs mt-1" style="color: var(--color-fg-muted)">Link calendar events to see them on the timeline.</p>' +
          '<button class="timeline-viz-btn mt-4" style="width:auto; padding:6px 16px; font-size:12px" ' +
            'onclick="var m=document.getElementById(\'event-picker-modal\'); if(m) m.classList.remove(\'hidden\')">' +
            '<i class="fa-solid fa-link" style="margin-right:6px"></i>Link Events' +
          '</button>' +
        '</div>';
      this.minimapContainer.innerHTML = '';
      return;
    }

    var self = this;
    var container = this.svgContainer;
    var width = container.clientWidth || 800;
    var m = this.margin;

    // Reserve extra top space for era bar strip when eras exist.
    var eraBarHeight = (this.eras && this.eras.length > 0) ? 20 : 0;
    m.top = 40 + eraBarHeight;
    this.margin.top = m.top;
    this.eraBarHeight = eraBarHeight;

    // Compute year range.
    var minYear = d3.min(this.events, function(d) { return d.event_year; });
    var maxYear = d3.max(this.events, function(d) { return d.event_year; });
    minYear = minYear - 1;
    maxYear = maxYear + 1;

    // Build swim-lanes.
    var lanes = this._buildLanes();
    var laneCount = Math.max(lanes.length, 1);
    // Use a dynamic minimum content height so the SVG fills the container
    // instead of leaving empty whitespace below a tiny 50px content area.
    var minContentHeight = Math.max(laneCount * this.laneHeight, 240);
    var contentHeight = minContentHeight;
    var height = m.top + contentHeight + m.bottom;

    // Create SVG.
    container.innerHTML = '';
    var svg = d3.select(container)
      .append('svg')
      .attr('width', width)
      .attr('height', height)
      .attr('class', 'timeline-viz-svg');
    this.svg = svg;
    this.width = width;
    this.height = height;
    this.contentHeight = contentHeight;

    // Ruler Y position: center of the content area.
    this.rulerY = m.top + contentHeight / 2;

    // Defs: clip path with padding for day-level cards (extend above/below).
    var defs = svg.append('defs');
    var clipPad = 45;
    defs.append('clipPath')
      .attr('id', 'timeline-clip')
      .append('rect')
      .attr('x', m.left)
      .attr('y', m.top - clipPad)
      .attr('width', width - m.left - m.right)
      .attr('height', contentHeight + clipPad * 2);

    // Glow filter for low-zoom event dots.
    var glowFilter = defs.append('filter')
      .attr('id', 'event-glow')
      .attr('x', '-50%').attr('y', '-50%')
      .attr('width', '200%').attr('height', '200%');
    glowFilter.append('feDropShadow')
      .attr('dx', 0).attr('dy', 1)
      .attr('stdDeviation', 2)
      .attr('flood-opacity', 0.35);

    // Arrowhead marker for event connections.
    defs.append('marker')
      .attr('id', 'conn-arrow')
      .attr('viewBox', '0 0 10 10')
      .attr('refX', 9).attr('refY', 5)
      .attr('markerWidth', 6).attr('markerHeight', 6)
      .attr('orient', 'auto-start-reverse')
      .append('path')
      .attr('d', 'M 0 0 L 10 5 L 0 10 Z')
      .attr('fill', 'var(--color-accent, #6366f1)');

    // X scale: maps year values to pixel positions.
    var xScale = d3.scaleLinear()
      .domain([minYear, maxYear])
      .range([m.left, width - m.right]);
    this.xScale = xScale;
    this.xScaleOrig = xScale.copy();

    // Y scale: maps lane index to vertical position.
    var yScale = d3.scaleBand()
      .domain(d3.range(laneCount))
      .range([m.top, m.top + contentHeight])
      .padding(0.1);
    this.yScale = yScale;
    this.laneCount = laneCount;

    // Background.
    svg.append('rect')
      .attr('width', width)
      .attr('height', height)
      .attr('fill', 'var(--color-surface, #1a1b26)')
      .attr('rx', 8);

    // Era bands group (behind everything except background).
    this.eraBandGroup = svg.append('g').attr('class', 'timeline-era-bands');

    // Grid lines group (behind events, on top of eras).
    this.gridGroup = svg.append('g').attr('class', 'timeline-grid');

    // Swim-lane backgrounds.
    this.laneGroup = svg.append('g').attr('class', 'timeline-lanes');
    var laneData = lanes.length > 0 ? lanes : [{ name: 'Events', color: null, entityIDs: null }];
    this.laneGroup.selectAll('rect')
      .data(laneData)
      .enter()
      .append('rect')
      .attr('x', m.left)
      .attr('y', function(d, i) { return yScale(i); })
      .attr('width', width - m.left - m.right)
      .attr('height', yScale.bandwidth())
      .attr('fill', function(d, i) {
        return i % 2 === 0
          ? 'var(--color-surface-alt, #24253a)'
          : 'transparent';
      })
      .attr('opacity', lanes.length > 1 ? 1 : 0)
      .attr('rx', 4);

    // Lane labels.
    if (lanes.length > 1) {
      this.laneGroup.selectAll('text')
        .data(laneData)
        .enter()
        .append('text')
        .attr('x', m.left + 6)
        .attr('y', function(d, i) { return yScale(i) + 14; })
        .attr('class', 'timeline-lane-label')
        .text(function(d) { return d.name; });
    }

    // Ruler group (center spine with ticks).
    this.rulerGroup = svg.append('g')
      .attr('class', 'timeline-ruler')
      .attr('transform', 'translate(0,' + this.rulerY + ')');

    // Central ruler line (the "spine").
    this.rulerGroup.append('line')
      .attr('class', 'timeline-ruler-line')
      .attr('x1', m.left)
      .attr('x2', width - m.right)
      .attr('y1', 0).attr('y2', 0);

    // Connection lines group (behind events, clipped).
    this.connectionGroup = svg.append('g')
      .attr('clip-path', 'url(#timeline-clip)')
      .attr('class', 'timeline-connections');

    // Main content group (clipped).
    this.contentGroup = svg.append('g')
      .attr('clip-path', 'url(#timeline-clip)');

    // Assign lane index to each event.
    this._assignLanes(lanes);

    // Draw everything.
    this._drawEraBands();
    this._drawGrid();
    this._drawRulerTicks();
    this._drawEvents();
    this._drawConnections();

    // Setup zoom behavior.
    this.zoom = d3.zoom()
      .scaleExtent([0.1, 50])
      .translateExtent([[-Infinity, 0], [Infinity, height]])
      .on('zoom', function(event) {
        self.currentTransform = event.transform;
        self._onZoom();
      });

    svg.call(this.zoom);

    // Double-click on empty space to create event at that date.
    svg.on('dblclick.create', function(event) {
      // Only trigger if the click was not on an event marker.
      if (event.target.closest('.timeline-event')) return;
      var coords = d3.pointer(event);
      var yearFrac = self.xScale.invert(coords[0]);
      self._onCreateAtDate(yearFrac);
    });

    this._updateZoomLevel();
    this._drawMinimap();
    this._updateVisibleRange();
  },

  // ---- Swim-Lane Helpers ----

  /**
   * Build swim-lane definitions from entity groups.
   */
  _buildLanes: function() {
    if (!this.groups || this.groups.length === 0) {
      return [];
    }
    var lanes = [];
    for (var i = 0; i < this.groups.length; i++) {
      var g = this.groups[i];
      var entityIDs = {};
      if (g.members) {
        for (var j = 0; j < g.members.length; j++) {
          entityIDs[g.members[j].entity_id] = true;
        }
      }
      lanes.push({ name: g.name, color: g.color, entityIDs: entityIDs });
    }
    lanes.push({ name: 'Other', color: null, entityIDs: null });
    return lanes;
  },

  /**
   * Assign a lane index to each event based on its entity_id.
   */
  _assignLanes: function(lanes) {
    for (var i = 0; i < this.events.length; i++) {
      var evt = this.events[i];
      evt._lane = 0;
      if (lanes.length > 0 && evt.event_entity_id) {
        for (var j = 0; j < lanes.length; j++) {
          if (lanes[j].entityIDs && lanes[j].entityIDs[evt.event_entity_id]) {
            evt._lane = j;
            break;
          }
          if (j === lanes.length - 1) {
            evt._lane = j;
          }
        }
      }
    }

    var laneGroups = {};
    for (var i = 0; i < this.events.length; i++) {
      var lane = this.events[i]._lane;
      if (!laneGroups[lane]) laneGroups[lane] = [];
      laneGroups[lane].push(this.events[i]);
    }
    for (var lane in laneGroups) {
      var evts = laneGroups[lane];
      for (var k = 0; k < evts.length; k++) {
        evts[k]._subIndex = k;
        evts[k]._subCount = evts.length;
      }
    }
  },

  // ---- Date/Color Helpers ----

  /**
   * Convert event date to a fractional year for precise positioning.
   */
  _dateToYear: function(evt) {
    return evt.event_year + (evt.event_month - 1) / 12 + (evt.event_day - 1) / 365;
  },

  /**
   * Convert end date to fractional year for range bars.
   */
  _endDateToYear: function(evt) {
    if (!evt.event_end_year) return null;
    var ey = evt.event_end_year;
    var em = evt.event_end_month || evt.event_month || 1;
    var ed = evt.event_end_day || evt.event_day || 1;
    return ey + (em - 1) / 12 + (ed - 1) / 365;
  },

  /**
   * Get effective color for an event (override or timeline default).
   */
  _eventColor: function(evt) {
    if (evt.color_override) return evt.color_override;
    return this.timelineColor;
  },

  // ---- Zoom Level Helpers ----

  /**
   * Determine the current zoom level name from scale factor.
   */
  _getZoomLevel: function() {
    var k = this.currentTransform.k;
    if (k < 0.3) return 'era';
    if (k < 0.8) return 'century';
    if (k < 2) return 'decade';
    if (k < 8) return 'year';
    if (k < 25) return 'month';
    return 'day';
  },

  /**
   * Get visual style config for current zoom level.
   */
  _currentStyle: function() {
    return this.zoomStyles[this.currentZoomLevel];
  },

  // ---- Ruler Tick System (Center Spine) ----

  /**
   * Get primary tick spacing (in pixels) based on zoom level.
   */
  _primarySpacing: function() {
    var level = this.currentZoomLevel;
    if (level === 'era' || level === 'century') return 120;
    if (level === 'decade') return 80;
    if (level === 'year') return 60;
    if (level === 'month') return 50;
    return 70; // day
  },

  /**
   * Format a tick value as a label based on zoom level.
   */
  _tickLabel: function(d, level) {
    if (level === 'era' || level === 'century' || level === 'decade' || level === 'year') {
      return 'Y' + Math.round(d);
    }
    if (level === 'month') {
      var year = Math.floor(d);
      var monthFrac = (d - year) * 12;
      var month = Math.round(monthFrac) + 1;
      if (month <= 1 || month > 12) return 'Y' + year;
      return 'M' + month;
    }
    // Day level.
    var year = Math.floor(d);
    var dayFrac = (d - year) * 365;
    var month = Math.floor(dayFrac / 30) + 1;
    var day = Math.round(dayFrac % 30) + 1;
    if (day <= 1 && month <= 1) return 'Y' + year;
    return 'M' + month + ' D' + day;
  },

  /**
   * Compute secondary tick values (between primary ticks).
   */
  _secondaryTickValues: function(primaryTicks) {
    if (primaryTicks.length < 2) return [];
    var result = [];
    for (var i = 0; i < primaryTicks.length - 1; i++) {
      var mid = (primaryTicks[i] + primaryTicks[i + 1]) / 2;
      result.push(mid);
    }
    return result;
  },

  /**
   * Compute tertiary tick values (between primary and secondary ticks).
   */
  _tertiaryTickValues: function(primaryTicks, secondaryTicks) {
    if (primaryTicks.length < 2) return [];
    var all = primaryTicks.concat(secondaryTicks).sort(function(a, b) { return a - b; });
    var result = [];
    for (var i = 0; i < all.length - 1; i++) {
      var mid = (all[i] + all[i + 1]) / 2;
      result.push(mid);
    }
    return result;
  },

  /**
   * Draw the center ruler with multi-tier ticks.
   * Replaces the old _drawAxis() method.
   */
  _drawRulerTicks: function() {
    var self = this;
    var m = this.margin;
    var level = this.currentZoomLevel;
    var w = this.width - m.left - m.right;

    // Clear previous ticks.
    this.rulerGroup.selectAll('.ruler-tick').remove();

    // Primary ticks (tall, labeled).
    var primaryTicks = this.xScale.ticks(
      Math.max(2, Math.floor(w / this._primarySpacing()))
    );

    var primaryG = this.rulerGroup.selectAll('.ruler-tick-primary')
      .data(primaryTicks).enter().append('g')
      .attr('class', 'ruler-tick ruler-tick-primary')
      .attr('transform', function(d) { return 'translate(' + self.xScale(d) + ',0)'; });

    primaryG.append('line')
      .attr('y1', -14).attr('y2', 14)
      .attr('stroke', 'var(--color-fg-secondary, #a9b1d6)')
      .attr('stroke-width', 1.5)
      .attr('opacity', 0.5);

    primaryG.append('text')
      .attr('class', 'ruler-label ruler-label-primary')
      .attr('y', -20)
      .attr('text-anchor', 'middle')
      .text(function(d) { return self._tickLabel(d, level); });

    // Secondary ticks (medium, unlabeled).
    var secondaryTicks = this._secondaryTickValues(primaryTicks);

    this.rulerGroup.selectAll('.ruler-tick-secondary')
      .data(secondaryTicks).enter().append('g')
      .attr('class', 'ruler-tick ruler-tick-secondary')
      .attr('transform', function(d) { return 'translate(' + self.xScale(d) + ',0)'; })
      .append('line')
      .attr('y1', -8).attr('y2', 8)
      .attr('stroke', 'var(--color-fg-muted, #565f89)')
      .attr('stroke-width', 1)
      .attr('opacity', 0.3);

    // Tertiary ticks (short, very subtle).
    var tertiaryTicks = this._tertiaryTickValues(primaryTicks, secondaryTicks);

    this.rulerGroup.selectAll('.ruler-tick-tertiary')
      .data(tertiaryTicks).enter().append('g')
      .attr('class', 'ruler-tick ruler-tick-tertiary')
      .attr('transform', function(d) { return 'translate(' + self.xScale(d) + ',0)'; })
      .append('line')
      .attr('y1', -4).attr('y2', 4)
      .attr('stroke', 'var(--color-fg-muted, #565f89)')
      .attr('stroke-width', 0.5)
      .attr('opacity', 0.2);

    // Store primary ticks for grid reuse.
    this._lastPrimaryTicks = primaryTicks;
    this._lastSecondaryTicks = secondaryTicks;
  },

  // ---- Grid System ----

  /**
   * Draw vertical grid lines with alternating column bands.
   */
  _drawGrid: function() {
    var self = this;
    var m = this.margin;
    var contentHeight = this.contentHeight || (this.height - m.top - m.bottom);
    var w = this.width - m.left - m.right;

    // Clear previous grid.
    this.gridGroup.selectAll('*').remove();

    // Get tick positions from xScale.
    var majorTicks = this.xScale.ticks(
      Math.max(2, Math.floor(w / this._primarySpacing()))
    );

    // Alternating column bands (very subtle zebra striping).
    var bandPositions = [m.left].concat(majorTicks.map(function(d) {
      return self.xScale(d);
    })).concat([this.width - m.right]);

    for (var i = 0; i < bandPositions.length - 1; i++) {
      if (i % 2 === 0) {
        var bw = bandPositions[i + 1] - bandPositions[i];
        if (bw > 0) {
          this.gridGroup.append('rect')
            .attr('x', bandPositions[i])
            .attr('y', m.top)
            .attr('width', bw)
            .attr('height', contentHeight)
            .attr('fill', 'var(--color-fg, #c0caf5)')
            .attr('opacity', 0.02);
        }
      }
    }

    // Major grid lines at primary tick positions.
    this.gridGroup.selectAll('.grid-major')
      .data(majorTicks).enter().append('line')
      .attr('class', 'grid-major')
      .attr('x1', function(d) { return self.xScale(d); })
      .attr('x2', function(d) { return self.xScale(d); })
      .attr('y1', m.top)
      .attr('y2', m.top + contentHeight)
      .attr('stroke', 'var(--color-edge, #2a2b3d)')
      .attr('stroke-width', 1)
      .attr('opacity', 0.15);

    // Minor grid lines between major ticks.
    var minorTicks = this._secondaryTickValues(majorTicks);
    this.gridGroup.selectAll('.grid-minor')
      .data(minorTicks).enter().append('line')
      .attr('class', 'grid-minor')
      .attr('x1', function(d) { return self.xScale(d); })
      .attr('x2', function(d) { return self.xScale(d); })
      .attr('y1', m.top)
      .attr('y2', m.top + contentHeight)
      .attr('stroke', 'var(--color-edge, #2a2b3d)')
      .attr('stroke-dasharray', '2,6')
      .attr('stroke-width', 0.5)
      .attr('opacity', 0.08);
  },

  // ---- Era Background Bands ----

  /**
   * Draw calendar era bands as semi-transparent colored rectangles
   * with faint watermark labels.
   */
  _drawEraBands: function() {
    var self = this;
    if (!this.eras || this.eras.length === 0) {
      if (this.eraBandGroup) this.eraBandGroup.selectAll('*').remove();
      return;
    }

    this.eraBandGroup.selectAll('*').remove();

    var xScale = this.xScale;
    var barY = 8;   // top padding inside SVG
    var barH = 16;  // compact bar height

    this.eras.forEach(function(era) {
      var startYear = era.start_year;
      var endYear = era.end_year || xScale.domain()[1];

      // Skip eras completely outside visible domain.
      var domain = xScale.domain();
      if (endYear < domain[0] || startYear > domain[1]) return;

      var x1 = xScale(Math.max(startYear, domain[0]));
      var x2 = xScale(Math.min(endYear, domain[1]));
      var bandWidth = Math.max(x2 - x1, 1);
      var color = era.color || '#6366f1';

      // Colored bar rectangle at the top.
      self.eraBandGroup.append('rect')
        .attr('class', 'timeline-era-band')
        .attr('x', x1).attr('y', barY)
        .attr('width', bandWidth)
        .attr('height', barH)
        .attr('fill', color)
        .attr('opacity', 0.25)
        .attr('rx', 3);

      // Era name label (truncated to fit bar width).
      if (bandWidth > 30) {
        self.eraBandGroup.append('text')
          .attr('class', 'timeline-era-label')
          .attr('x', x1 + 6)
          .attr('y', barY + barH / 2)
          .attr('dominant-baseline', 'central')
          .attr('font-size', '10px')
          .attr('font-weight', '600')
          .attr('fill', color)
          .attr('opacity', 0.9)
          .text(function() {
            var maxChars = Math.floor(bandWidth / 6);
            return era.name.length > maxChars
              ? era.name.substring(0, maxChars - 1) + '\u2026'
              : era.name;
          });
      }
    });
  },

  // ---- Event Clustering ----

  /**
   * Cluster nearby events at low zoom levels to avoid visual clutter.
   * Returns the displayable data array (mix of real events and cluster objects).
   */
  _clusterEvents: function() {
    var level = this.currentZoomLevel;
    if (level !== 'era' && level !== 'century') {
      // No clustering at higher zoom levels.
      this._displayEvents = this.events;
      return this.events;
    }

    var threshold = (level === 'era') ? 20 : 15;
    var xScale = this.xScale;
    var self = this;
    var clusters = [];
    var used = {};

    for (var i = 0; i < this.events.length; i++) {
      if (used[i]) continue;
      var cluster = [this.events[i]];
      used[i] = true;
      var px = xScale(self._dateToYear(this.events[i]));

      for (var j = i + 1; j < this.events.length; j++) {
        if (used[j]) continue;
        var px2 = xScale(self._dateToYear(this.events[j]));
        if (Math.abs(px2 - px) < threshold) {
          cluster.push(this.events[j]);
          used[j] = true;
        }
      }

      if (cluster.length > 1) {
        // Create cluster pseudo-event with averaged position.
        var avgYear = 0;
        for (var c = 0; c < cluster.length; c++) {
          avgYear += self._dateToYear(cluster[c]);
        }
        avgYear /= cluster.length;
        clusters.push({
          _isCluster: true,
          _events: cluster,
          _count: cluster.length,
          _lane: cluster[0]._lane,
          _subIndex: cluster[0]._subIndex,
          event_year: cluster[0].event_year,
          event_month: cluster[0].event_month,
          event_day: cluster[0].event_day,
          _avgYear: avgYear,
          _minYear: d3.min(cluster, function(e) { return e.event_year; }),
          _maxYear: d3.max(cluster, function(e) { return e.event_year; })
        });
      } else {
        clusters.push(cluster[0]);
      }
    }

    this._displayEvents = clusters;
    return clusters;
  },

  // ---- Category Icons ----

  /**
   * Map event category to a Font Awesome unicode glyph.
   * Returns null if no icon mapping exists.
   */
  _categoryIcon: function(cat) {
    if (!cat) return null;
    var map = {
      holiday:  '\uf073',  // calendar-days
      battle:   '\uf132',  // shield-halved
      quest:    '\uf279',  // map
      birthday: '\uf1fd',  // cake-candles
      festival: '\uf72b',  // hat-wizard
      travel:   '\uf072',  // plane
      custom:   '\uf111'   // circle
    };
    return map[cat] || null;
  },

  /**
   * Map event category to a color for the category indicator dot.
   */
  _categoryColor: function(cat) {
    if (!cat) return 'transparent';
    var map = {
      holiday:  '#f59e0b',
      battle:   '#ef4444',
      quest:    '#22c55e',
      birthday: '#ec4899',
      festival: '#a855f7',
      travel:   '#3b82f6',
      custom:   '#6b7280'
    };
    return map[cat] || '#6b7280';
  },

  // ---- Event Drawing ----

  /**
   * Compute Y position for an event, using center-spine layout for
   * single-lane timelines and swim-lane centering for multi-lane.
   */
  _eventY: function(d) {
    if (this.laneCount <= 1) {
      // Single lane: alternate events above and below the ruler spine.
      var offset = (d._subIndex % 2 === 0) ? -28 : 28;
      return this.rulerY + offset;
    }
    // Multi-lane: center within swim-lane band.
    var laneY = this.yScale(d._lane) || this.margin.top;
    var bandH = this.yScale.bandwidth() || this.laneHeight;
    return laneY + bandH / 2;
  },

  /**
   * Draw event markers on the timeline.
   */
  _drawEvents: function() {
    var self = this;
    var xScale = this.xScale;
    var style = this._currentStyle();
    var level = this.currentZoomLevel;

    // Get display data (possibly clustered).
    var displayData = this._clusterEvents();

    var eventGroups = this.contentGroup.selectAll('.timeline-event')
      .data(displayData)
      .enter()
      .append('g')
      .attr('class', 'timeline-event')
      .attr('transform', function(d) {
        var x = d._isCluster
          ? xScale(d._avgYear)
          : xScale(self._dateToYear(d));
        var y = self._eventY(d);
        return 'translate(' + x + ',' + y + ')';
      })
      .style('cursor', 'pointer');

    // --- Range bars for multi-day events ---
    eventGroups.each(function(d) {
      if (d._isCluster) return;
      var endYearFrac = self._endDateToYear(d);
      if (endYearFrac === null) return;
      var g = d3.select(this);
      var startX = 0;
      var endX = xScale(endYearFrac) - xScale(self._dateToYear(d));
      if (endX < 3) return; // too small to show

      // Bar behind the dot.
      g.insert('rect', ':first-child')
        .attr('class', 'timeline-event-range-bar')
        .attr('x', 0).attr('y', -4)
        .attr('width', endX)
        .attr('height', 8)
        .attr('rx', 4)
        .attr('fill', self._eventColor(d))
        .attr('opacity', 0.3);

      // End cap circle.
      g.append('circle')
        .attr('class', 'timeline-event-range-end')
        .attr('cx', endX).attr('cy', 0)
        .attr('r', Math.max(style.radius * 0.5, 2))
        .attr('fill', self._eventColor(d))
        .attr('opacity', 0.5);
    });

    // --- Cluster badges ---
    eventGroups.each(function(d) {
      if (!d._isCluster) return;
      var g = d3.select(this);
      var label = d._count + ' events';
      var pillW = label.length * 6.5 + 20;

      g.append('rect')
        .attr('class', 'timeline-cluster-badge')
        .attr('x', -pillW / 2).attr('y', -11)
        .attr('width', pillW).attr('height', 22)
        .attr('rx', 11)
        .attr('fill', 'var(--color-surface-alt, #24253a)')
        .attr('stroke', 'var(--color-accent, #6366f1)')
        .attr('stroke-width', 1.5);
      g.append('text')
        .attr('class', 'timeline-cluster-label')
        .attr('text-anchor', 'middle')
        .attr('y', 4)
        .attr('fill', 'var(--color-fg, #c0caf5)')
        .attr('font-size', '10px')
        .attr('font-weight', '600')
        .text(label);
    });

    // --- Event dot (base marker) for non-cluster events ---
    var useGlow = (level === 'era' || level === 'century' || level === 'decade');
    eventGroups.filter(function(d) { return !d._isCluster; }).append('circle')
      .attr('class', 'timeline-event-dot')
      .attr('r', style.radius)
      .attr('fill', function(d) { return self._eventColor(d); })
      .attr('stroke', 'var(--color-surface, #1a1b26)')
      .attr('stroke-width', style.strokeWidth)
      .attr('filter', useGlow ? 'url(#event-glow)' : null);

    // --- Category icons (replace dots at year/month zoom) ---
    if (level === 'year' || level === 'month') {
      eventGroups.each(function(d) {
        if (d._isCluster) return;
        var icon = self._categoryIcon(d.event_category);
        if (!icon) return;
        var g = d3.select(this);
        // Hide the dot, show icon instead.
        g.select('.timeline-event-dot').attr('opacity', 0);
        g.append('text')
          .attr('class', 'timeline-event-icon')
          .attr('text-anchor', 'middle')
          .attr('dominant-baseline', 'central')
          .attr('font-family', '"Font Awesome 6 Free"')
          .attr('font-weight', 900)
          .attr('font-size', (style.radius * 1.6) + 'px')
          .attr('fill', self._eventColor(d))
          .text(icon);
      });
    }

    // Pulsing ring for highlighted (searched) events.
    eventGroups.filter(function(d) { return !d._isCluster; }).append('circle')
      .attr('class', 'timeline-event-highlight')
      .attr('r', style.radius + 4)
      .attr('fill', 'none')
      .attr('stroke', function(d) { return self._eventColor(d); })
      .attr('stroke-width', 1.5)
      .attr('opacity', 0)
      .attr('stroke-dasharray', '3,3');

    // Pill background for month zoom labels.
    var isMonth = (level === 'month');
    if (isMonth) {
      eventGroups.filter(function(d) { return !d._isCluster; }).append('rect')
        .attr('class', 'timeline-event-pill')
        .attr('rx', 4).attr('ry', 4)
        .attr('x', style.radius + 2)
        .attr('y', -13)
        .attr('width', function(d) {
          var label = d.label || d.event_name || '';
          return Math.min(label.length * 6.5 + 16, 160);
        })
        .attr('height', 20)
        .attr('fill', function(d) { return self._eventColor(d) + '20'; })
        .attr('stroke', function(d) { return self._eventColor(d) + '60'; })
        .attr('stroke-width', 1);
    }

    // Card markers at day zoom level (foreignObject with HTML cards).
    var isDay = (level === 'day');
    if (isDay) {
      eventGroups.filter(function(d) { return !d._isCluster; }).append('foreignObject')
        .attr('class', 'timeline-event-card-fo')
        .attr('width', 220).attr('height', 80)
        .attr('x', style.radius + 6).attr('y', -40)
        .append('xhtml:div')
        .attr('class', 'tl-viz-card')
        .html(function(d) {
          var label = self._escapeHTML(d.label || d.event_name || 'Untitled');
          var date = 'Y' + d.event_year + ' M' + d.event_month + ' D' + d.event_day;
          var entity = d.event_entity_name ? self._escapeHTML(d.event_entity_name) : '';
          var catHtml = '';
          if (d.event_category) {
            catHtml = '<span class="tl-viz-card-cat">' + self._escapeHTML(d.event_category) + '</span>';
          }
          return '<div class="tl-viz-card-name">' + label + '</div>' +
                 '<div class="tl-viz-card-date">' + date + '</div>' +
                 (entity ? '<div class="tl-viz-card-entity"><i class="fa-solid ' +
                   (d.event_entity_icon || 'fa-circle-dot') + ' tl-viz-card-entity-icon"></i>' + entity + '</div>' : '') +
                 catHtml;
        });
    }

    // Event name label (hidden at day zoom — cards replace it).
    eventGroups.filter(function(d) { return !d._isCluster; }).append('text')
      .attr('class', 'timeline-event-label')
      .attr('x', style.radius + (isMonth ? 10 : 4))
      .attr('y', -2)
      .text(function(d) { return d.label || d.event_name || ''; })
      .style('display', (style.showLabel && !isDay) ? null : 'none');

    // Date sub-label.
    eventGroups.filter(function(d) { return !d._isCluster; }).append('text')
      .attr('class', 'timeline-event-date')
      .attr('x', style.radius + 4)
      .attr('y', 10)
      .text(function(d) { return 'M' + d.event_month + ' D' + d.event_day; })
      .style('display', (style.showDate && !isDay) ? null : 'none');

    // Entity sub-label (only at month zoom — day zoom uses cards).
    eventGroups.filter(function(d) { return !d._isCluster; }).append('text')
      .attr('class', 'timeline-event-entity')
      .attr('x', style.radius + 4)
      .attr('y', 22)
      .text(function(d) { return d.event_entity_name || ''; })
      .style('display', (style.showEntity && level === 'month') ? null : 'none');

    // Category indicator dot (small colored dot next to main circle at decade+ zoom).
    eventGroups.filter(function(d) { return !d._isCluster; }).append('circle')
      .attr('class', 'timeline-event-cat-dot')
      .attr('cx', function() { return -(style.radius + 3); })
      .attr('cy', 0)
      .attr('r', 2.5)
      .attr('fill', function(d) {
        return self._categoryColor(d.event_category);
      })
      .style('display', function(d) {
        return d.event_category ? null : 'none';
      });

    // Hover/click handlers.
    eventGroups
      .on('mouseenter', function(event, d) {
        if (d._isCluster) return;
        self._showTooltip(event, d);
      })
      .on('mouseleave', function() { self._hideTooltip(); })
      .on('click', function(event, d) {
        if (d._isCluster) {
          // Zoom into the cluster's year range.
          self._zoomToRange(d._minYear - 1, d._maxYear + 1);
        } else {
          self._onEventClick(event, d);
        }
      });

    this.eventGroups = eventGroups;
  },

  // ---- Event Connection Lines ----

  /**
   * Build a lookup from event_id → event data for fast connection resolution.
   */
  _buildEventIndex: function() {
    var idx = {};
    var events = this._displayEvents || this.events;
    for (var i = 0; i < events.length; i++) {
      var e = events[i];
      if (!e._isCluster) {
        idx[e.event_id] = e;
      }
    }
    return idx;
  },

  /**
   * Draw SVG lines/arrows between connected events.
   * Lines curve upward (for left-to-right) using quadratic Bézier paths.
   */
  _drawConnections: function() {
    if (!this.connectionGroup) return;
    this.connectionGroup.selectAll('*').remove();

    if (!this.connections || this.connections.length === 0) return;

    var self = this;
    var xScale = this.xScale;
    var idx = this._buildEventIndex();

    this.connections.forEach(function(conn) {
      var srcEvt = idx[conn.source_id];
      var tgtEvt = idx[conn.target_id];
      if (!srcEvt || !tgtEvt) return;

      var x1 = xScale(self._dateToYear(srcEvt));
      var y1 = self._eventY(srcEvt);
      var x2 = xScale(self._dateToYear(tgtEvt));
      var y2 = self._eventY(tgtEvt);

      // Quadratic Bézier control point (arc above the line).
      var midX = (x1 + x2) / 2;
      var dist = Math.abs(x2 - x1);
      var arcHeight = Math.min(dist * 0.3, 60);
      var midY = Math.min(y1, y2) - arcHeight;

      var color = (conn.color) ? conn.color : 'var(--color-accent, #6366f1)';
      var dashArray = '';
      if (conn.style === 'dashed') dashArray = '8,4';
      else if (conn.style === 'dotted') dashArray = '3,3';

      var path = self.connectionGroup.append('path')
        .attr('d', 'M ' + x1 + ' ' + y1 + ' Q ' + midX + ' ' + midY + ' ' + x2 + ' ' + y2)
        .attr('fill', 'none')
        .attr('stroke', color)
        .attr('stroke-width', 1.5)
        .attr('stroke-opacity', 0.6)
        .attr('class', 'timeline-connection-line');

      if (dashArray) path.attr('stroke-dasharray', dashArray);

      // Add arrowhead for 'arrow' style.
      if (conn.style === 'arrow' || conn.style === '') {
        path.attr('marker-end', 'url(#conn-arrow)');
      }

      // Optional label at midpoint.
      if (conn.label) {
        var labelX = midX;
        var labelY = midY - 6;
        self.connectionGroup.append('text')
          .attr('x', labelX)
          .attr('y', labelY)
          .attr('text-anchor', 'middle')
          .attr('fill', color)
          .attr('font-size', '10px')
          .attr('font-weight', '500')
          .attr('opacity', 0.8)
          .attr('class', 'timeline-connection-label')
          .text(conn.label);
      }
    });
  },

  // ---- Mini-Map Overview Strip ----

  /**
   * Draw the mini-map overview strip below the main SVG.
   */
  _drawMinimap: function() {
    var self = this;
    var container = this.minimapContainer;
    if (!container) return;
    var width = container.clientWidth || this.width || 800;
    var height = 36;

    container.innerHTML = '';

    if (this.events.length === 0) return;

    var miniSvg = d3.select(container)
      .append('svg')
      .attr('width', width)
      .attr('height', height)
      .attr('class', 'timeline-viz-minimap-svg');

    // Background.
    miniSvg.append('rect')
      .attr('width', width).attr('height', height)
      .attr('fill', 'var(--color-surface-alt, #24253a)')
      .attr('rx', 0);

    // Mini scale spanning full event range.
    var minYear = d3.min(this.events, function(d) { return d.event_year; }) - 1;
    var maxYear = d3.max(this.events, function(d) { return d.event_year; }) + 1;
    this.miniScale = d3.scaleLinear()
      .domain([minYear, maxYear])
      .range([4, width - 4]);

    // Draw mini event dots.
    miniSvg.selectAll('.mini-dot')
      .data(this.events).enter()
      .append('circle')
      .attr('cx', function(d) { return self.miniScale(self._dateToYear(d)); })
      .attr('cy', height / 2)
      .attr('r', 1.5)
      .attr('fill', function(d) { return self._eventColor(d); })
      .attr('opacity', 0.6);

    // Subtle center line across minimap.
    miniSvg.append('line')
      .attr('x1', 4).attr('x2', width - 4)
      .attr('y1', height / 2).attr('y2', height / 2)
      .attr('stroke', 'var(--color-edge, #2a2b3d)')
      .attr('stroke-width', 1)
      .attr('opacity', 0.4);

    // Viewport rectangle (updated on zoom).
    this.miniViewport = miniSvg.append('rect')
      .attr('class', 'timeline-minimap-viewport')
      .attr('y', 2).attr('height', height - 4)
      .attr('rx', 3)
      .attr('fill', 'var(--color-accent, #6366f1)')
      .attr('opacity', 0.12)
      .attr('stroke', 'var(--color-accent, #6366f1)')
      .attr('stroke-width', 1)
      .attr('stroke-opacity', 0.4);

    this._updateMinimapViewport();

    // Click on minimap to jump.
    miniSvg.on('click', function(event) {
      var coords = d3.pointer(event);
      var targetYear = self.miniScale.invert(coords[0]);
      self._centerOnYear(targetYear);
    });

    this.miniSvg = miniSvg;
  },

  /**
   * Update the minimap viewport rectangle to reflect the current main view.
   */
  _updateMinimapViewport: function() {
    if (!this.miniViewport || !this.miniScale) return;
    var m = this.margin;
    var leftYear = this.xScale.invert(m.left);
    var rightYear = this.xScale.invert(this.width - m.right);
    var x1 = this.miniScale(leftYear);
    var x2 = this.miniScale(rightYear);
    this.miniViewport
      .attr('x', Math.max(0, x1))
      .attr('width', Math.max(4, x2 - x1));
  },

  // ---- Navigation Helpers ----

  /**
   * Center the view on a specific year (preserves current zoom scale).
   */
  _centerOnYear: function(year) {
    if (!this.svg || !this.zoom) return;
    var m = this.margin;
    var centerX = (this.width - m.left - m.right) / 2 + m.left;
    var k = this.currentTransform.k;
    var tx = centerX - k * this.xScaleOrig(year);

    this.svg.transition().duration(400).call(
      this.zoom.transform,
      d3.zoomIdentity.translate(tx, 0).scale(k)
    );
  },

  /**
   * Zoom to fit a specific year range.
   */
  _zoomToRange: function(minYear, maxYear) {
    if (!this.svg || !this.zoom) return;
    var m = this.margin;
    var w = this.width - m.left - m.right;
    var yearSpan = maxYear - minYear;
    if (yearSpan <= 0) yearSpan = 2;

    var scale = w / (this.xScaleOrig(maxYear) - this.xScaleOrig(minYear));
    var tx = m.left - scale * this.xScaleOrig(minYear);

    this.svg.transition().duration(500).call(
      this.zoom.transform,
      d3.zoomIdentity.translate(tx, 0).scale(scale)
    );
  },

  /**
   * Update the visible year range indicator in the toolbar.
   */
  _updateVisibleRange: function() {
    var m = this.margin;
    var leftYear = Math.round(this.xScale.invert(m.left));
    var rightYear = Math.round(this.xScale.invert(this.width - m.right));
    var rangeEl = this.el.querySelector('.timeline-viz-range');
    if (rangeEl) {
      rangeEl.textContent = 'Y' + leftYear + ' \u2013 Y' + rightYear;
    }
  },

  // ---- Zoom/Pan ----

  /**
   * Handle zoom/pan events: rescale axis, reposition events, update styles.
   */
  _onZoom: function() {
    var self = this;
    var t = this.currentTransform;

    // Rescale X axis only (horizontal pan/zoom).
    var newX = t.rescaleX(this.xScaleOrig);
    this.xScale = newX;

    // Detect zoom level change.
    var newLevel = this._getZoomLevel();
    var levelChanged = (newLevel !== this.currentZoomLevel);
    this.currentZoomLevel = newLevel;

    // Update ruler, grid, eras.
    this._drawRulerTicks();
    this._drawGrid();
    this._drawEraBands();

    // Update ruler spine line extent.
    this.rulerGroup.select('.timeline-ruler-line')
      .attr('x1', this.margin.left)
      .attr('x2', this.width - this.margin.right);

    // Reposition events.
    if (this.eventGroups) {
      this.eventGroups.attr('transform', function(d) {
        var x = d._isCluster
          ? newX(d._avgYear)
          : newX(self._dateToYear(d));
        var y = self._eventY(d);
        return 'translate(' + x + ',' + y + ')';
      });
    }

    // Update event visual styles based on zoom level.
    if (levelChanged) {
      // Redraw events to swap marker types (circle/pill/card) and clustering.
      this.contentGroup.selectAll('.timeline-event').remove();
      this._drawEvents();

      // Re-apply search filter after redraw.
      if (this.searchQuery) {
        this._applySearchFilter();
      }

      // Reposition after redraw.
      if (this.eventGroups) {
        this.eventGroups.attr('transform', function(d) {
          var x = d._isCluster
            ? newX(d._avgYear)
            : newX(self._dateToYear(d));
          var y = self._eventY(d);
          return 'translate(' + x + ',' + y + ')';
        });
      }
    }

    // Redraw connection lines (they depend on event positions).
    this._drawConnections();

    this._updateZoomLevel();
    this._updateMinimapViewport();
    this._updateVisibleRange();
  },

  /**
   * Update the active zoom level button in the toolbar.
   */
  _updateZoomLevel: function() {
    var btns = this.el.querySelectorAll('.timeline-viz-zoom-btn');
    var level = this.currentZoomLevel;
    for (var i = 0; i < btns.length; i++) {
      if (btns[i].getAttribute('data-zoom-level') === level) {
        btns[i].classList.add('active');
      } else {
        btns[i].classList.remove('active');
      }
    }
  },

  /**
   * Jump to a specific zoom level by animating to its target scale factor.
   * Preserves the current horizontal pan center.
   * @param {string} level - Zoom level name (era/century/decade/year/month/day).
   */
  _jumpToZoomLevel: function(level) {
    if (!this.svg || !this.zoom) return;
    var targetK = this._zoomTargets[level];
    if (targetK == null) return;

    var m = this.margin;
    var centerX = (this.width - m.left - m.right) / 2 + m.left;

    // Calculate the center year of the current view.
    var currentCenterYear = this.xScale.invert(centerX);

    // Compute new translation to keep the same center year.
    var tx = centerX - targetK * this.xScaleOrig(currentCenterYear);

    this.svg.transition().duration(400).call(
      this.zoom.transform,
      d3.zoomIdentity.translate(tx, 0).scale(targetK)
    );
  },

  /**
   * Programmatic zoom by a scale factor.
   */
  _zoomBy: function(factor) {
    if (!this.svg || !this.zoom) return;
    this.svg.transition().duration(300).call(
      this.zoom.scaleBy, factor
    );
  },

  /**
   * Zoom to fit all events in view.
   */
  _zoomFit: function() {
    if (!this.svg || !this.zoom || this.events.length === 0) return;
    var m = this.margin;
    var w = this.width - m.left - m.right;
    var minYear = d3.min(this.events, function(d) { return d.event_year; }) - 1;
    var maxYear = d3.max(this.events, function(d) { return d.event_year; }) + 1;
    var yearSpan = maxYear - minYear;
    if (yearSpan <= 0) yearSpan = 2;

    var scale = w / (this.xScaleOrig(maxYear) - this.xScaleOrig(minYear));
    var tx = m.left - scale * this.xScaleOrig(minYear);

    this.svg.transition().duration(500).call(
      this.zoom.transform,
      d3.zoomIdentity.translate(tx, 0).scale(scale)
    );
  },

  /**
   * Skip to a specific year (centers the view on that year).
   */
  _skipToYear: function() {
    var input = this.el.querySelector('.timeline-viz-skip-input');
    var year = parseInt(input.value, 10);
    if (isNaN(year)) return;
    this._centerOnYear(year);
    input.value = '';
  },

  // ---- Search/Filter ----

  /**
   * Apply search filter: highlight matching events, dim non-matching ones.
   */
  _applySearchFilter: function() {
    if (!this.eventGroups) return;
    var q = this.searchQuery;
    var self = this;

    this.eventGroups.each(function(d) {
      var g = d3.select(this);
      if (!q) {
        g.style('opacity', 1);
        g.select('.timeline-event-highlight').attr('opacity', 0);
        return;
      }

      var match;
      if (d._isCluster) {
        // A cluster matches if any of its events match.
        match = d._events.some(function(e) { return self._eventMatchesSearch(e, q); });
      } else {
        match = self._eventMatchesSearch(d, q);
      }
      g.style('opacity', match ? 1 : 0.15);
      g.select('.timeline-event-highlight')
        .attr('opacity', match ? 0.6 : 0);
    });
  },

  /**
   * Check if an event matches the search query.
   */
  _eventMatchesSearch: function(d, q) {
    var name = (d.label || d.event_name || '').toLowerCase();
    if (name.indexOf(q) !== -1) return true;
    var entity = (d.event_entity_name || '').toLowerCase();
    if (entity.indexOf(q) !== -1) return true;
    var cat = (d.event_category || '').toLowerCase();
    if (cat.indexOf(q) !== -1) return true;
    var desc = (d.event_description || '').toLowerCase();
    if (desc.indexOf(q) !== -1) return true;
    if (String(d.event_year).indexOf(q) !== -1) return true;
    return false;
  },

  /**
   * Clear the search filter.
   */
  _clearSearch: function() {
    var input = this.el.querySelector('.timeline-viz-search-input');
    input.value = '';
    this.searchQuery = '';
    this.el.querySelector('.timeline-viz-search-clear').style.display = 'none';
    this._applySearchFilter();
  },

  // ---- Tooltip/Detail Panel ----

  /**
   * Show tooltip for an event.
   */
  _showTooltip: function(event, d) {
    var tip = this.tooltip;
    var label = d.label || d.event_name || 'Untitled';
    var date = 'Y' + d.event_year + ' M' + d.event_month + ' D' + d.event_day;

    var html = '<div style="font-weight:700; font-size:13px; margin-bottom:4px">' +
      this._escapeHTML(label) + '</div>';
    html += '<div style="font-family:ui-monospace,monospace; font-size:11px; color:var(--color-fg-muted); margin-bottom:4px">' +
      date + '</div>';

    if (d.event_entity_name) {
      html += '<div style="display:inline-flex; align-items:center; gap:4px; font-size:12px; color:var(--color-fg-secondary); margin-bottom:3px">';
      if (d.event_entity_icon) {
        html += '<i class="fa-solid ' + d.event_entity_icon + '"></i>';
      }
      html += this._escapeHTML(d.event_entity_name) + '</div>';
    }
    if (d.event_category) {
      html += '<div><span class="timeline-viz-tooltip-cat">' + this._escapeHTML(d.event_category) + '</span></div>';
    }
    if (d.event_description) {
      var desc = d.event_description;
      if (desc.length > 120) desc = desc.substring(0, 120) + '...';
      html += '<div style="font-size:11px; color:var(--color-fg-muted); margin-top:4px; line-height:1.4">' +
        this._escapeHTML(desc) + '</div>';
    }

    tip.innerHTML = html;

    var x = event.pageX + 14;
    var y = event.pageY - 12;
    var tipW = tip.offsetWidth;
    if (x + tipW > window.innerWidth - 20) {
      x = event.pageX - tipW - 14;
    }
    tip.style.left = x + 'px';
    tip.style.top = y + 'px';
    tip.classList.add('visible');
  },

  /**
   * Hide the tooltip.
   */
  _hideTooltip: function() {
    this.tooltip.classList.remove('visible');
  },

  /**
   * Handle click on an event marker — show the detail panel.
   */
  _onEventClick: function(event, d) {
    this._hideTooltip();
    this._showDetail(event, d);
  },

  /**
   * Show the event detail panel positioned near the clicked event.
   * @param {MouseEvent} event - Click event for positioning.
   * @param {Object} d - Event data object.
   */
  _showDetail: function(event, d) {
    var panel = this.detailPanel;
    var body = panel.querySelector('.timeline-viz-detail-body');
    var label = d.label || d.event_name || 'Untitled';
    var date = 'Y' + d.event_year + ' M' + d.event_month + ' D' + d.event_day;
    var color = this._eventColor(d);

    // Show end date if range event.
    if (d.event_end_year) {
      date += ' \u2013 Y' + d.event_end_year;
      if (d.event_end_month) date += ' M' + d.event_end_month;
      if (d.event_end_day) date += ' D' + d.event_end_day;
    }

    var html = '<div class="timeline-viz-detail-color" style="background:' + color + '"></div>';
    html += '<div class="timeline-viz-detail-name">' + this._escapeHTML(label) + '</div>';
    html += '<div class="timeline-viz-detail-date">' + date + '</div>';

    if (d.event_entity_name) {
      html += '<div class="timeline-viz-detail-entity">';
      if (d.event_entity_icon) {
        html += '<i class="fa-solid ' + d.event_entity_icon + '"></i> ';
      }
      html += this._escapeHTML(d.event_entity_name) + '</div>';
    }

    if (d.event_category) {
      html += '<div class="timeline-viz-detail-cat">' + this._escapeHTML(d.event_category) + '</div>';
    }

    if (d.event_description) {
      html += '<div class="timeline-viz-detail-desc">' + this._escapeHTML(d.event_description) + '</div>';
    }

    // Source indicator.
    if (d.source === 'standalone') {
      html += '<div class="timeline-viz-detail-source"><i class="fa-solid fa-pen-fancy"></i> Standalone event</div>';
    } else {
      html += '<div class="timeline-viz-detail-source"><i class="fa-solid fa-calendar-days"></i> Calendar event</div>';
    }

    body.innerHTML = html;

    // Position near the click, staying within viewport.
    var x = event.pageX + 16;
    var y = event.pageY - 20;
    var panelW = 320;
    if (x + panelW > window.innerWidth - 20) {
      x = event.pageX - panelW - 16;
    }
    if (y + 300 > window.innerHeight) {
      y = window.innerHeight - 320;
    }
    if (y < 10) y = 10;

    panel.style.left = x + 'px';
    panel.style.top = y + 'px';
    panel.classList.add('visible');
  },

  /**
   * Hide the event detail panel.
   */
  _hideDetail: function() {
    if (this.detailPanel) {
      this.detailPanel.classList.remove('visible');
    }
  },

  // ---- Create From Timeline ----

  /**
   * Open the standalone event create modal with the date pre-filled from the
   * clicked position on the timeline. Triggered by double-click on empty space.
   * @param {number} yearFrac - Fractional year value from the D3 scale.
   */
  _onCreateAtDate: function(yearFrac) {
    var year = Math.floor(yearFrac);
    var monthFrac = (yearFrac - year) * 12;
    var month = Math.floor(monthFrac) + 1;
    var dayFrac = (monthFrac - Math.floor(monthFrac)) * 30;
    var day = Math.max(1, Math.round(dayFrac) + 1);
    if (month < 1) month = 1;
    if (month > 12) month = 12;
    if (day > 30) day = 30;

    // Try to find the standalone event create modal and populate date fields.
    var modal = document.getElementById('standalone-event-modal');
    if (!modal) return;

    // Fill date fields.
    var yearInput = modal.querySelector('[name="year"]');
    var monthInput = modal.querySelector('[name="month"]');
    var dayInput = modal.querySelector('[name="day"]');
    if (yearInput) yearInput.value = year;
    if (monthInput) monthInput.value = month;
    if (dayInput) dayInput.value = day;

    // Show the modal.
    modal.classList.remove('hidden');
  },

  // ---- Utilities ----

  /**
   * Escape HTML entities for safe insertion.
   */
  _escapeHTML: function(str) {
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  },

  /**
   * Destroy the widget and clean up.
   */
  destroy: function() {
    if (this.tooltip && this.tooltip.parentNode) {
      this.tooltip.parentNode.removeChild(this.tooltip);
    }
    if (this.detailPanel && this.detailPanel.parentNode) {
      this.detailPanel.parentNode.removeChild(this.detailPanel);
    }
    if (this.el) {
      this.el.innerHTML = '';
    }
  }
};

// Register with Chronicle's widget system. If boot.js hasn't executed yet
// (e.g. this script loaded as a non-defer tag in the body while boot.js is
// deferred in the head), wait for DOMContentLoaded when all defer scripts
// will have run.
function _register() { Chronicle.register('timeline-viz', _impl); }
if (typeof Chronicle !== 'undefined') {
  _register();
} else {
  document.addEventListener('DOMContentLoaded', _register);
}
})();
