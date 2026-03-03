/**
 * timeline_viz.js -- D3.js Interactive Timeline Visualization Widget
 *
 * Renders an interactive SVG timeline with:
 *   - Horizontal time axis with 6 zoom levels (era→day)
 *   - Event markers with zoom-level-dependent visual styles
 *   - Pan/drag via d3.zoom, scroll wheel zoom
 *   - Tooltips on hover (event name, date, entity, category)
 *   - Color-coded events (per-link override or timeline default color)
 *   - Entity group swim-lanes (when groups exist)
 *   - Skip-to-date input, zoom fit, search/filter bar
 *
 * Zoom levels and visual styles:
 *   Era     — Small dots with subtle glow
 *   Century — Small circles with glow effect
 *   Decade  — Medium circles with category color coding
 *   Year    — Circles with labels, date annotations
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
 */
Chronicle.register('timeline-viz', {
  /** CDN URL used to dynamically load D3 when it's not already available. */
  _d3Src: 'https://cdn.jsdelivr.net/npm/d3@7/dist/d3.min.js',

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
    this.timeline = null;
    this.tooltip = null;
    this.svg = null;
    this.zoom = null;
    this.currentTransform = null;
    this.searchQuery = '';
    this.currentZoomLevel = 'year';

    // Layout constants.
    this.margin = { top: 60, right: 40, bottom: 40, left: 40 };
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
          // ~10 seconds of polling.
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
   * Build the widget DOM structure (toolbar + SVG container).
   */
  _buildDOM: function() {
    this.el.innerHTML = '';
    this.el.classList.add('timeline-viz-container');

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
        '<span class="timeline-viz-zoom-label">Year</span>' +
      '</div>' +
      '<div class="timeline-viz-toolbar-divider"></div>' +
      '<div class="timeline-viz-toolbar-center">' +
        '<div class="timeline-viz-search-wrap">' +
          '<i class="fa-solid fa-search timeline-viz-search-icon"></i>' +
          '<input type="text" class="timeline-viz-search-input" placeholder="Filter events..."/>' +
          '<button class="timeline-viz-search-clear" data-action="search-clear" style="display:none">' +
            '<i class="fa-solid fa-xmark"></i>' +
          '</button>' +
        '</div>' +
      '</div>' +
      '<div class="timeline-viz-toolbar-divider"></div>' +
      '<div class="timeline-viz-toolbar-right">' +
        '<label class="timeline-viz-skip-label">Go to year:</label>' +
        '<input type="number" class="timeline-viz-skip-input" placeholder="Year"/>' +
        '<button class="timeline-viz-btn" data-action="skip-to">' +
          '<i class="fa-solid fa-arrow-right"></i>' +
        '</button>' +
      '</div>';
    this.el.appendChild(toolbar);

    // SVG container.
    var svgContainer = document.createElement('div');
    svgContainer.className = 'timeline-viz-svg-wrap';
    this.el.appendChild(svgContainer);
    this.svgContainer = svgContainer;

    // Tooltip (uses opacity transition via .visible class).
    var tip = document.createElement('div');
    tip.className = 'timeline-viz-tooltip';
    document.body.appendChild(tip);
    this.tooltip = tip;

    // Wire toolbar buttons.
    var self = this;
    toolbar.addEventListener('click', function(e) {
      var btn = e.target.closest('[data-action]');
      if (!btn) return;
      var action = btn.getAttribute('data-action');
      if (action === 'zoom-in') self._zoomBy(1.5);
      else if (action === 'zoom-out') self._zoomBy(1 / 1.5);
      else if (action === 'zoom-fit') self._zoomFit();
      else if (action === 'skip-to') self._skipToYear();
      else if (action === 'search-clear') self._clearSearch();
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
          '<div class="timeline-viz-empty">Failed to load timeline data.</div>';
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
      return;
    }

    var self = this;
    var container = this.svgContainer;
    var width = container.clientWidth || 800;
    var m = this.margin;

    // Compute year range.
    var minYear = d3.min(this.events, function(d) { return d.event_year; });
    var maxYear = d3.max(this.events, function(d) { return d.event_year; });
    minYear = minYear - 1;
    maxYear = maxYear + 1;

    // Build swim-lanes.
    var lanes = this._buildLanes();
    var laneCount = Math.max(lanes.length, 1);
    var contentHeight = laneCount * this.laneHeight;
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

    // Defs: clip path and glow filter.
    var defs = svg.append('defs');
    defs.append('clipPath')
      .attr('id', 'timeline-clip')
      .append('rect')
      .attr('x', m.left)
      .attr('y', m.top)
      .attr('width', width - m.left - m.right)
      .attr('height', contentHeight);

    // Glow filter for low-zoom event dots.
    var glowFilter = defs.append('filter')
      .attr('id', 'event-glow')
      .attr('x', '-50%').attr('y', '-50%')
      .attr('width', '200%').attr('height', '200%');
    glowFilter.append('feDropShadow')
      .attr('dx', 0).attr('dy', 1)
      .attr('stdDeviation', 2)
      .attr('flood-opacity', 0.35);

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

    // Background.
    svg.append('rect')
      .attr('width', width)
      .attr('height', height)
      .attr('fill', 'var(--color-surface, #1a1b26)')
      .attr('rx', 8);

    // Grid lines group (behind everything).
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

    // Axis group.
    this.axisGroup = svg.append('g')
      .attr('class', 'timeline-axis')
      .attr('transform', 'translate(0,' + m.top + ')');

    // Main content group (clipped).
    this.contentGroup = svg.append('g')
      .attr('clip-path', 'url(#timeline-clip)');

    // Assign lane index to each event.
    this._assignLanes(lanes);

    // Draw events.
    this._drawEvents();
    this._drawAxis();
    this._drawGrid();

    // Setup zoom behavior.
    this.zoom = d3.zoom()
      .scaleExtent([0.1, 50])
      .translateExtent([[-Infinity, 0], [Infinity, height]])
      .on('zoom', function(event) {
        self.currentTransform = event.transform;
        self._onZoom();
      });

    svg.call(this.zoom);
    this._updateZoomLevel();
  },

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

  /**
   * Convert event date to a fractional year for precise positioning.
   */
  _dateToYear: function(evt) {
    return evt.event_year + (evt.event_month - 1) / 12 + (evt.event_day - 1) / 365;
  },

  /**
   * Get effective color for an event (override or timeline default).
   */
  _eventColor: function(evt) {
    if (evt.color_override) return evt.color_override;
    return this.timelineColor;
  },

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

  /**
   * Draw event markers on the timeline.
   */
  _drawEvents: function() {
    var self = this;
    var xScale = this.xScale;
    var yScale = this.yScale;
    var style = this._currentStyle();

    var eventGroups = this.contentGroup.selectAll('.timeline-event')
      .data(this.events)
      .enter()
      .append('g')
      .attr('class', 'timeline-event')
      .attr('transform', function(d) {
        var x = xScale(self._dateToYear(d));
        var laneY = yScale(d._lane) || self.margin.top;
        var bandH = yScale.bandwidth() || self.laneHeight;
        var y = laneY + bandH / 2;
        return 'translate(' + x + ',' + y + ')';
      })
      .style('cursor', 'pointer');

    // Event circle — base marker (with glow at low zoom levels).
    var useGlow = (this.currentZoomLevel === 'era' || this.currentZoomLevel === 'century' || this.currentZoomLevel === 'decade');
    eventGroups.append('circle')
      .attr('class', 'timeline-event-dot')
      .attr('r', style.radius)
      .attr('fill', function(d) { return self._eventColor(d); })
      .attr('stroke', 'var(--color-surface, #1a1b26)')
      .attr('stroke-width', style.strokeWidth)
      .attr('filter', useGlow ? 'url(#event-glow)' : null);

    // Pulsing ring for highlighted (searched) events.
    eventGroups.append('circle')
      .attr('class', 'timeline-event-highlight')
      .attr('r', style.radius + 4)
      .attr('fill', 'none')
      .attr('stroke', function(d) { return self._eventColor(d); })
      .attr('stroke-width', 1.5)
      .attr('opacity', 0)
      .attr('stroke-dasharray', '3,3');

    // Pill background for month zoom labels.
    var isMonth = (this.currentZoomLevel === 'month');
    if (isMonth) {
      eventGroups.append('rect')
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
    var isDay = (this.currentZoomLevel === 'day');
    if (isDay) {
      eventGroups.append('foreignObject')
        .attr('class', 'timeline-event-card-fo')
        .attr('width', 180).attr('height', 62)
        .attr('x', style.radius + 6).attr('y', -31)
        .append('xhtml:div')
        .attr('class', 'tl-viz-card')
        .html(function(d) {
          var label = self._escapeHTML(d.label || d.event_name || 'Untitled');
          var date = 'M' + d.event_month + ' D' + d.event_day;
          var entity = d.event_entity_name ? self._escapeHTML(d.event_entity_name) : '';
          return '<div class="tl-viz-card-name">' + label + '</div>' +
                 '<div class="tl-viz-card-date">' + date + '</div>' +
                 (entity ? '<div class="tl-viz-card-entity">' + entity + '</div>' : '');
        });
    }

    // Event name label (hidden at day zoom — cards replace it).
    eventGroups.append('text')
      .attr('class', 'timeline-event-label')
      .attr('x', style.radius + (isMonth ? 10 : 4))
      .attr('y', -2)
      .text(function(d) { return d.label || d.event_name || ''; })
      .style('display', (style.showLabel && !isDay) ? null : 'none');

    // Date sub-label.
    eventGroups.append('text')
      .attr('class', 'timeline-event-date')
      .attr('x', style.radius + 4)
      .attr('y', 10)
      .text(function(d) { return 'M' + d.event_month + ' D' + d.event_day; })
      .style('display', (style.showDate && !isDay) ? null : 'none');

    // Entity sub-label (only at month zoom — day zoom uses cards).
    eventGroups.append('text')
      .attr('class', 'timeline-event-entity')
      .attr('x', style.radius + 4)
      .attr('y', 22)
      .text(function(d) { return d.event_entity_name || ''; })
      .style('display', (style.showEntity && this.currentZoomLevel === 'month') ? null : 'none');

    // Category indicator dot (small colored dot next to main circle at decade+ zoom).
    eventGroups.append('circle')
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
      .on('mouseenter', function(event, d) { self._showTooltip(event, d); })
      .on('mouseleave', function() { self._hideTooltip(); })
      .on('click', function(event, d) { self._onEventClick(event, d); });

    this.eventGroups = eventGroups;
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

  /**
   * Draw the time axis with zoom-level-aware tick formatting.
   */
  _drawAxis: function() {
    var self = this;
    var level = this.currentZoomLevel;
    var tickFormat, tickCount;

    if (level === 'era' || level === 'century') {
      tickFormat = function(d) { return 'Y' + Math.round(d); };
      tickCount = Math.max(2, Math.floor((self.width - self.margin.left - self.margin.right) / 120));
    } else if (level === 'decade') {
      tickFormat = function(d) { return 'Y' + Math.round(d); };
      tickCount = Math.max(2, Math.floor((self.width - self.margin.left - self.margin.right) / 80));
    } else if (level === 'year') {
      tickFormat = function(d) { return 'Y' + Math.round(d); };
      tickCount = Math.max(2, Math.floor((self.width - self.margin.left - self.margin.right) / 60));
    } else if (level === 'month') {
      tickFormat = function(d) {
        var year = Math.floor(d);
        var monthFrac = (d - year) * 12;
        var month = Math.round(monthFrac) + 1;
        if (month <= 1 || month > 12) return 'Y' + year;
        return 'M' + month;
      };
      tickCount = Math.max(4, Math.floor((self.width - self.margin.left - self.margin.right) / 50));
    } else {
      // Day level.
      tickFormat = function(d) {
        var year = Math.floor(d);
        var dayFrac = (d - year) * 365;
        var month = Math.floor(dayFrac / 30) + 1;
        var day = Math.round(dayFrac % 30) + 1;
        if (day <= 1 && month <= 1) return 'Y' + year;
        return 'M' + month + ' D' + day;
      };
      tickCount = Math.max(4, Math.floor((self.width - self.margin.left - self.margin.right) / 70));
    }

    var xAxis = d3.axisTop(this.xScale)
      .tickFormat(tickFormat)
      .ticks(tickCount);
    this.axisGroup.call(xAxis);
  },

  /**
   * Draw vertical grid lines.
   */
  _drawGrid: function() {
    var m = this.margin;
    var contentHeight = this.height - m.top - m.bottom;
    var ticks = this.xScale.ticks(
      Math.max(2, Math.floor((this.width - m.left - m.right) / 80))
    );

    this.gridGroup.selectAll('line').remove();
    this.gridGroup.selectAll('line')
      .data(ticks)
      .enter()
      .append('line')
      .attr('x1', this.xScale)
      .attr('x2', this.xScale)
      .attr('y1', m.top)
      .attr('y2', m.top + contentHeight)
      .attr('stroke', 'var(--color-edge, #2a2b3d)')
      .attr('stroke-dasharray', '2,4')
      .attr('stroke-width', 0.5);
  },

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

    // Update axis with level-aware formatting.
    this._drawAxis();
    this._drawGrid();

    // Get current style.
    var style = this._currentStyle();

    // Reposition events.
    this.eventGroups.attr('transform', function(d) {
      var x = newX(self._dateToYear(d));
      var laneY = self.yScale(d._lane) || self.margin.top;
      var bandH = self.yScale.bandwidth() || self.laneHeight;
      var y = laneY + bandH / 2;
      return 'translate(' + x + ',' + y + ')';
    });

    // Update event visual styles based on zoom level.
    if (levelChanged) {
      var isDay = (newLevel === 'day');
      var isMonth = (newLevel === 'month');
      var useGlow = (newLevel === 'era' || newLevel === 'century' || newLevel === 'decade');

      // Redraw events to swap marker types (circle/pill/card).
      this.contentGroup.selectAll('.timeline-event').remove();
      this._drawEvents();

      // Re-apply search filter after redraw.
      if (this.searchQuery) {
        this._applySearchFilter();
      }

      // Reposition after redraw.
      this.eventGroups.attr('transform', function(d) {
        var x = newX(self._dateToYear(d));
        var laneY = self.yScale(d._lane) || self.margin.top;
        var bandH = self.yScale.bandwidth() || self.laneHeight;
        var y = laneY + bandH / 2;
        return 'translate(' + x + ',' + y + ')';
      });
    }

    this._updateZoomLevel();
  },

  /**
   * Update the zoom level indicator in the toolbar.
   */
  _updateZoomLevel: function() {
    var label = this.el.querySelector('.timeline-viz-zoom-label');
    if (!label) return;
    var level = this.currentZoomLevel;
    var display = level.charAt(0).toUpperCase() + level.slice(1);
    label.textContent = display;
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

    if (!this.svg || !this.zoom) return;
    var m = this.margin;
    var centerX = (this.width - m.left - m.right) / 2 + m.left;
    var k = this.currentTransform.k;
    var tx = centerX - k * this.xScaleOrig(year);

    this.svg.transition().duration(500).call(
      this.zoom.transform,
      d3.zoomIdentity.translate(tx, 0).scale(k)
    );
    input.value = '';
  },

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
        // No filter: reset all to full opacity.
        g.style('opacity', 1);
        g.select('.timeline-event-highlight').attr('opacity', 0);
        return;
      }

      var match = self._eventMatchesSearch(d, q);
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
    // Match year.
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

  /**
   * Show tooltip for an event.
   */
  _showTooltip: function(event, d) {
    var tip = this.tooltip;
    var label = d.label || d.event_name || 'Untitled';
    var date = 'Y' + d.event_year + ' M' + d.event_month + ' D' + d.event_day;

    // Build structured tooltip content.
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

    // Position tooltip.
    var x = event.pageX + 14;
    var y = event.pageY - 12;
    var tipW = tip.offsetWidth;
    if (x + tipW > window.innerWidth - 20) {
      x = event.pageX - tipW - 14;
    }
    tip.style.left = x + 'px';
    tip.style.top = y + 'px';

    // Fade in via CSS transition.
    tip.classList.add('visible');
  },

  /**
   * Hide the tooltip with opacity fade-out.
   */
  _hideTooltip: function() {
    this.tooltip.classList.remove('visible');
  },

  /**
   * Handle click on an event marker.
   */
  _onEventClick: function(event, d) {
    // Open the event list section (Alpine.js managed).
    var container = this.el.closest('[x-data]');
    if (container && container.__x) {
      container.__x.$data.eventListOpen = true;
    }
  },

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
    if (this.el) {
      this.el.innerHTML = '';
    }
  }
});
