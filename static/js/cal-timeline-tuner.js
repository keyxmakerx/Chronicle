// cal-timeline-tuner.js — C-TIMELINE-V2-DESIGN-1-TUNER.
//
// The "FM Tuner" timeline showcase: a radio-dial time axis through the
// middle of the canvas, swim-lanes above and below, era gradient bands
// behind everything, hover-revealed connection arcs, adaptive zoom from
// millennia to days, and a restrained atmospheric backdrop.
//
// Browser IIFE, INIT_BLOCKS-style per-block try/catch (a throw in one
// block can't take down the rest). Mock-driven — reads the dataset from
// the #cal-tuner-data attribute (single source of truth with the templ).
// Raw SVG + CSS transforms; NO D3, NO vendor library (audit §7 decision).
//
// Pure, DOM-free helpers are exposed on `window.__tuner*` so the headless
// node tests can exercise the adaptive-tick model, the registry hooks,
// the backdrop restraint rules, the lane grouping, and the cursor-sync
// DOM-event protocol without a real browser. Visual fidelity is the
// operator's local gate.
//
// Cross-refs:
//   - dispatches/chronicle/C-TIMELINE-V2-DESIGN-1-TUNER.md (this dispatch)
//   - decisions/2026-06-05-rendering-canvas-css-exemption.md (CSS canvas)

(function () {
  'use strict';

  // ── INIT_BLOCKS runner ─────────────────────────────────────────────
  var BLOCKS = [];
  function registerInitBlock(name, fn) { BLOCKS.push({ name: name, fn: fn }); }
  function runAll() {
    var res = {};
    BLOCKS.forEach(function (b) {
      try { b.fn(); res[b.name] = 'ok'; }
      catch (e) {
        res[b.name] = 'error: ' + (e && e.message);
        try { console.error('[tuner] init block ' + b.name + ' failed', e); } catch (ee) {}
      }
    });
    return res;
  }

  function reduced() {
    try { return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches); }
    catch (e) { return false; }
  }

  // ── Mock dataset (single source of truth) ──────────────────────────
  var DATA = null;
  function loadData() {
    if (DATA) return DATA;
    var node = document.getElementById('cal-tuner-data');
    var raw = node && node.getAttribute('data-cal-tuner-data');
    if (!raw) { DATA = {}; return DATA; }
    try { DATA = JSON.parse(raw); } catch (e) { DATA = {}; }
    return DATA;
  }

  // ── Calendar geometry: date → absolute day index and back ──────────
  // The mock approximates uniform months (days_per_month / months_per_year),
  // which is exact for Harptos (12 × 30) and good enough for positioning.
  function geom() {
    var d = DATA || {};
    return { dpm: d.days_per_month || 30, mpy: d.months_per_year || 12 };
  }
  function dayIndex(y, m, day) {
    var g = geom();
    return (y * g.mpy + (Math.max(1, m || 1) - 1)) * g.dpm + (Math.max(1, day || 1) - 1);
  }
  function fromDayIndex(idx) {
    var g = geom(), dpy = g.mpy * g.dpm;
    var y = Math.floor(idx / dpy); var rem = idx - y * dpy;
    var m = Math.floor(rem / g.dpm) + 1; var day = rem - (m - 1) * g.dpm + 1;
    return { year: y, month: m, day: day };
  }
  function dateKey(y, m, day) { return y + '-' + m + '-' + day; }

  // ── Adaptive tick model (§A "Zoom levels") ─────────────────────────
  // Given pixels-per-day, return the tick granularity. The thresholds
  // mirror the dispatch table exactly.
  function tickModel(pxPerDay) {
    if (pxPerDay > 200) return { level: 'max-in', major: 'days', minor: 'hours' };
    if (pxPerDay >= 50) return { level: 'high-in', major: 'weeks', minor: 'days' };
    if (pxPerDay >= 10) return { level: 'medium-in', major: 'months', minor: 'weeks' };
    if (pxPerDay >= 1) return { level: 'default', major: 'years', minor: 'months' };
    if (pxPerDay >= 0.1) return { level: 'medium-out', major: 'decades', minor: 'years' };
    if (pxPerDay >= 0.01) return { level: 'high-out', major: 'centuries', minor: 'decades' };
    return { level: 'max-out', major: 'millennia', minor: 'centuries' };
  }
  // Days-per-unit for tick spacing (approximate, mock-uniform calendar).
  function unitDays(unit) {
    var g = geom(), dpy = g.mpy * g.dpm;
    switch (unit) {
      case 'hours': return 1 / (DATA && DATA.calendar && DATA.calendar.hours_per_day || 24);
      case 'days': return 1;
      case 'weeks': return 7;
      case 'months': return g.dpm;
      case 'years': return dpy;
      case 'decades': return dpy * 10;
      case 'centuries': return dpy * 100;
      case 'millennia': return dpy * 1000;
      default: return dpy;
    }
  }
  var ZOOM_LABELS = {
    'max-in': 'Day view', 'high-in': 'Week view', 'medium-in': 'Month view',
    'default': 'Year view', 'medium-out': 'Decade view', 'high-out': 'Century view',
    'max-out': 'Millennium view'
  };

  // ── Effect registries (§A overlays + §J2 backdrop) ─────────────────
  // Self-contained for the showcase (the production Almanac registries are
  // engine-coupled and page-separation forbids cross-loading). Each MUST
  // entry implements timelineAxisRender + timelineBackdropRender; TBD-ish
  // entries fall back to a generic dot / tint.
  var WEATHER_GLYPHS = { rain: '☔', thunderstorm: '⛈', snow: '❄', fog: '🌫', cloudy: '☁', clear: '☀' };
  var CELESTIAL_GLYPHS = { 'meteor-shower': '☄', 'eclipse-solar': '🌑', 'eclipse-lunar': '🌕', 'comet': '☄', 'aurora': '✦' };
  var WEATHER_MUST = ['rain', 'snow', 'thunderstorm', 'fog', 'cloudy'];
  var CELESTIAL_MUST = ['meteor-shower', 'eclipse-solar', 'eclipse-lunar'];

  function makeEntry(id, glyph, must) {
    return {
      id: id,
      name: id.replace(/-/g, ' '),
      tier: must ? 'must' : 'tbd',
      axisGlyph: glyph || '•',
      // timelineAxisRender — append a small glyph at (x,y) in the
      // container. Returns the created node (or null if no container).
      timelineAxisRender: function (opts) {
        opts = opts || {};
        var c = opts.container; if (!c || !c.appendChild) return null;
        var el = document.createElement('span');
        el.className = 'cal-timeline-tuner-glyph cal-timeline-tuner-glyph--' + (opts.kind || 'weather');
        el.textContent = must ? (glyph || '•') : '•';
        el.title = (this.name) + (opts.date ? ' · ' + opts.date : '');
        if (typeof opts.x === 'number') el.style.left = opts.x + 'px';
        c.appendChild(el);
        return el;
      },
      // timelineBackdropRender — fill the backdrop container for this
      // effect at the given opacity (MUST entries paint a themed gradient
      // via the data-fx attribute; TBD entries a generic tint).
      timelineBackdropRender: function (opts) {
        opts = opts || {};
        var c = opts.container; if (!c || !c.appendChild) return null;
        var el = document.createElement('div');
        el.className = 'cal-timeline-tuner-backdrop-fill';
        el.setAttribute('data-fx', must ? id : 'generic');
        if (!must) el.style.background = 'oklch(0.40 0.02 260 / 0.5)';
        c.appendChild(el);
        return el;
      }
    };
  }

  var WEATHER_EFFECTS = {};
  var CELESTIAL_EFFECTS = {};
  function buildRegistries() {
    WEATHER_MUST.concat(['clear']).forEach(function (id) {
      WEATHER_EFFECTS[id] = makeEntry(id, WEATHER_GLYPHS[id], WEATHER_MUST.indexOf(id) !== -1);
    });
    CELESTIAL_MUST.concat(['comet', 'aurora']).forEach(function (id) {
      CELESTIAL_EFFECTS[id] = makeEntry(id, CELESTIAL_GLYPHS[id], CELESTIAL_MUST.indexOf(id) !== -1);
    });
  }
  buildRegistries();

  // ── Backdrop restraint plan (§J2, binding) ─────────────────────────
  // Pure resolver: given a date key, decide what the atmospheric backdrop
  // renders. Weather always renders if present; non-routine celestial
  // always render; sun + moons (the sky-band) render ONLY on special-moon
  // days (eclipses / supermoons / blood moons). No daily sun/moon noise.
  function backdropPlan(key) {
    var d = DATA || {};
    var dw = d.day_weather || {};
    var ce = d.celestial_events || {};
    var special = d.special_moon_days || [];
    var celest = (ce[key] || []).map(function (c) { return c.type; });
    var skyBand = special.indexOf(key) !== -1;
    return {
      weather: dw[key] || null,
      celestial: celest,
      skyBand: skyBand,
      sunMoon: skyBand, // sun + moons gate strictly on special-moon days
      active: !!(dw[key] || celest.length || skyBand)
    };
  }

  // ── Swim-lane grouping (§D) ────────────────────────────────────────
  // Pure: partition events into lanes by entity / category / tier.
  // Empty lanes are dropped; lanes preserve dataset order.
  function groupLanes(events, mode) {
    events = events || [];
    var d = DATA || {};
    var lanes = [], index = {};
    function lane(key, label, color) {
      if (!index[key]) { index[key] = { key: key, label: label, color: color || '', events: [] }; lanes.push(index[key]); }
      return index[key];
    }
    if (mode === 'category') {
      (d.categories || []).forEach(function (c) { lane(c.id, c.name, c.color); });
      events.forEach(function (e) { (index[e.category] || lane(e.category, e.category)).events.push(e); });
    } else if (mode === 'tier') {
      (d.tiers || []).forEach(function (t) { lane(t.id, t.name, t.color); });
      events.forEach(function (e) { (index[e.tier] || lane(e.tier, e.tier)).events.push(e); });
    } else { // entity (default)
      (d.entities || []).forEach(function (en) { lane(en.id, en.name, en.color); });
      events.forEach(function (e) {
        (e.entities || []).forEach(function (eid) { (index[eid] || lane(eid, eid)).events.push(e); });
      });
    }
    return lanes.filter(function (l) { return l.events.length > 0; });
  }

  // ── Connection model (§F) ──────────────────────────────────────────
  // Pure resolvers for hover-reveal + show-all toggle.
  function connectionsFor(eventId) {
    return (DATA && DATA.connections || []).filter(function (c) {
      return c.source === eventId || c.target === eventId;
    });
  }
  function arcDashClass(type) {
    if (type === 'related') return 'cal-timeline-tuner-arc--related';
    if (type === 'mentioned') return 'cal-timeline-tuner-arc--mentioned';
    return ''; // caused / co-occurs render solid
  }
  function entityColor(id) {
    var e = (DATA && DATA.entities || []).filter(function (x) { return x.id === id; })[0];
    return e ? e.color : 'oklch(0.7 0.04 250)';
  }

  // ── Cursor-sync DOM-event protocol (§J1) ───────────────────────────
  // Tuner emits cal:cursor-change when its needle moves and listens for
  // the same event from sibling widgets (loop-prevented by sourceWidgetId).
  function makeCursorSync(applyExternalDate) {
    var selfId = 'tuner-' + Math.random().toString(36).slice(2, 9);
    var sync = { selfId: selfId, type: 'timeline', lastExternal: null, lastEmitted: null };
    var emitPending = null, emitTimer = null;
    function rawEmit(name, detail) {
      detail = detail || {}; detail.sourceWidgetId = selfId; detail.sourceWidgetType = 'timeline';
      sync.lastEmitted = detail;
      try { document.dispatchEvent(new CustomEvent(name, { detail: detail })); } catch (e) {}
    }
    // Throttle cursor-change emits to ~50ms during a drag (§N event-storm),
    // but always flush the final position.
    sync.emitCursorChange = function (date, opts) {
      var detail = { date: date };
      if (opts && opts.immediate) { if (emitTimer) { clearTimeout(emitTimer); emitTimer = null; } rawEmit('cal:cursor-change', detail); return; }
      emitPending = detail;
      if (emitTimer) return;
      emitTimer = setTimeout(function () { emitTimer = null; if (emitPending) { rawEmit('cal:cursor-change', emitPending); emitPending = null; } }, 50);
    };
    sync.emitEventCreate = function (eventId, date) { rawEmit('cal:event-create', { eventId: eventId, date: date }); };
    sync.emitDateJump = function (date) { rawEmit('cal:date-jump', { date: date }); };
    // Single handler used for both real DOM events and direct test calls.
    sync._handle = function (detail) {
      if (!detail || detail.sourceWidgetId === selfId) return false; // loop-prevention
      sync.lastExternal = detail;
      if (detail.date && typeof applyExternalDate === 'function') applyExternalDate(detail.date);
      return true;
    };
    try {
      document.addEventListener('cal:cursor-change', function (ev) { sync._handle(ev && ev.detail); });
      document.addEventListener('cal:date-jump', function (ev) { sync._handle(ev && ev.detail); });
    } catch (e) {}
    return sync;
  }

  // Expose the pure helpers immediately (independent of DOM wiring) so the
  // headless tests can reach them even on a stubbed document.
  window.__tunerTickModel = tickModel;
  window.__tunerUnitDays = unitDays;
  window.__tunerRegistries = { weather: WEATHER_EFFECTS, celestial: CELESTIAL_EFFECTS };
  window.__tunerBackdropPlan = backdropPlan;
  window.__tunerGroupLanes = groupLanes;
  window.__tunerConnectionsFor = connectionsFor;
  window.__tunerDayIndex = dayIndex;
  window.__tunerFromDayIndex = fromDayIndex;

  // ════════════════════════════════════════════════════════════════════
  // DOM wiring — every block is null-safe so the page (and the node DOM
  // stub) never throws when an element is absent.
  // ════════════════════════════════════════════════════════════════════

  // Shared mutable view state for the interactive blocks.
  var STATE = {
    pxPerDay: 2.5,          // default zoom → "Year view"
    originDay: 0,           // day index at canvas x=0
    cursorDay: 0,           // current needle day index
    group: 'entity',
    showAll: false,
    backdrop: true,
    hovered: null
  };
  var EL = {};
  var CURSOR_SYNC = null;

  function q(sel) { try { return document.querySelector(sel); } catch (e) { return null; } }
  function canvasWidth() {
    if (EL.canvas && EL.canvas.getBoundingClientRect) {
      var r = EL.canvas.getBoundingClientRect(); if (r && r.width) return r.width;
    }
    return 1080;
  }
  function dayToX(idx) { return (idx - STATE.originDay) * STATE.pxPerDay; }
  function xToDay(x) { return STATE.originDay + x / STATE.pxPerDay; }

  function fmtDate(idx) {
    var dt = fromDayIndex(Math.round(idx));
    var epoch = (DATA && DATA.calendar && DATA.calendar.epoch_name) || '';
    var tm = tickModel(STATE.pxPerDay);
    if (tm.major === 'days' || tm.major === 'weeks') return 'Day ' + dt.day + ', M' + dt.month + ' ' + dt.year + ' ' + epoch;
    if (tm.major === 'months') return 'M' + dt.month + ' ' + dt.year + ' ' + epoch;
    return dt.year + ' ' + epoch;
  }

  function eraAt(year) {
    var eras = (DATA && DATA.eras) || [];
    for (var i = 0; i < eras.length; i++) {
      var e = eras[i];
      var end = e.end_year || 99999;
      if (year >= e.start_year && year <= end) return e;
    }
    return null;
  }

  // ── Block: data — parse + seed cursor at "today" ───────────────────
  registerInitBlock('data', function () {
    loadData();
    STATE.cursorDay = dayIndex(DATA.current_year || 0, DATA.current_month || 1, DATA.current_day || 1);
    STATE.originDay = STATE.cursorDay - (canvasWidth() / 2) / STATE.pxPerDay;
  });

  // ── Block: refs — cache element handles ────────────────────────────
  registerInitBlock('refs', function () {
    EL.root = q('[data-tuner-root]');
    EL.canvas = q('[data-tuner-canvas]');
    EL.ticks = q('[data-tuner-ticks]');
    EL.overlays = q('[data-tuner-axis-overlays]');
    EL.axis = q('[data-tuner-axis]');
    EL.eraBands = q('[data-tuner-era-bands]');
    EL.backdrop = q('[data-tuner-backdrop]');
    EL.lanesUpper = q('[data-tuner-lanes-upper]');
    EL.lanesLower = q('[data-tuner-lanes-lower]');
    EL.conns = q('[data-tuner-connections]');
    EL.needle = q('[data-tuner-needle]');
    EL.cursorChip = q('[data-tuner-cursor-chip]');
    EL.cursorDate = q('[data-tuner-cursor-date]');
    EL.eraName = q('[data-tuner-era-name]');
    EL.zoomLabel = q('[data-tuner-zoom-label]');
    EL.legend = q('[data-tuner-era-legend]');
    EL.popup = q('[data-tuner-popup]');
  });

  // ── Render: ticks + axis labels (adaptive, §A) ─────────────────────
  function renderTicks() {
    if (!EL.ticks) return;
    EL.ticks.innerHTML = '';
    var tm = tickModel(STATE.pxPerDay);
    var w = canvasWidth();
    var majorDays = unitDays(tm.major), minorDays = unitDays(tm.minor);
    var startDay = STATE.originDay, endDay = STATE.originDay + w / STATE.pxPerDay;
    // Minor ticks first (drawn shorter / subtler).
    var firstMinor = Math.ceil(startDay / minorDays) * minorDays;
    for (var md = firstMinor; md <= endDay; md += minorDays) {
      var mx = dayToX(md);
      var minor = document.createElement('span');
      minor.className = 'cal-timeline-tuner-tick cal-timeline-tuner-tick--minor';
      minor.style.left = mx + 'px';
      EL.ticks.appendChild(minor);
    }
    // Major ticks + labels.
    var firstMajor = Math.ceil(startDay / majorDays) * majorDays;
    for (var jd = firstMajor; jd <= endDay; jd += majorDays) {
      var jx = dayToX(jd), dt = fromDayIndex(Math.round(jd));
      var t = document.createElement('span');
      t.className = 'cal-timeline-tuner-tick cal-timeline-tuner-tick--major';
      t.style.left = jx + 'px';
      EL.ticks.appendChild(t);
      var lbl = document.createElement('span');
      lbl.className = 'cal-timeline-tuner-tick-label';
      lbl.style.left = jx + 'px';
      lbl.textContent = (tm.major === 'months') ? ('M' + dt.month) : String(dt.year);
      EL.ticks.appendChild(lbl);
    }
    if (EL.zoomLabel) EL.zoomLabel.textContent = ZOOM_LABELS[tm.level] || 'Year view';
  }

  // ── Render: registry overlays on the axis (§A) ─────────────────────
  function renderOverlays() {
    if (!EL.overlays) return;
    EL.overlays.innerHTML = '';
    var d = DATA || {}, dw = d.day_weather || {}, ce = d.celestial_events || {};
    var startDay = STATE.originDay, endDay = STATE.originDay + canvasWidth() / STATE.pxPerDay;
    Object.keys(dw).forEach(function (key) {
      var p = key.split('-'), idx = dayIndex(+p[0], +p[1], +p[2]);
      if (idx < startDay || idx > endDay) return;
      var fx = WEATHER_EFFECTS[dw[key]]; if (!fx) return;
      fx.timelineAxisRender({ container: EL.overlays, x: dayToX(idx), kind: 'weather', date: key });
    });
    Object.keys(ce).forEach(function (key) {
      var p = key.split('-'), idx = dayIndex(+p[0], +p[1], +p[2]);
      if (idx < startDay || idx > endDay) return;
      (ce[key] || []).forEach(function (c) {
        var fx = CELESTIAL_EFFECTS[c.type]; if (!fx) return;
        fx.timelineAxisRender({ container: EL.overlays, x: dayToX(idx), kind: 'celestial', date: key });
      });
    });
  }

  // ── Render: era gradient bands + legend (§C) ───────────────────────
  function renderEras() {
    if (!EL.eraBands) return;
    EL.eraBands.innerHTML = '';
    if (EL.legend) EL.legend.innerHTML = '';
    var g = geom(), dpy = g.mpy * g.dpm;
    ((DATA && DATA.eras) || []).forEach(function (e) {
      var startIdx = e.start_year * dpy;
      var endIdx = (e.end_year || (fromDayIndex(STATE.originDay + canvasWidth() / STATE.pxPerDay).year + 50)) * dpy;
      var x0 = dayToX(startIdx), x1 = dayToX(endIdx);
      var band = document.createElement('div');
      band.className = 'cal-timeline-tuner-era-band';
      band.setAttribute('data-era-id', e.id);
      band.style.left = x0 + 'px';
      band.style.width = Math.max(0, x1 - x0) + 'px';
      band.style.setProperty('--band-from', e.color);
      band.style.setProperty('--band-to', e.color);
      EL.eraBands.appendChild(band);
      var wm = document.createElement('div');
      wm.className = 'cal-timeline-tuner-era-watermark';
      wm.style.left = ((x0 + x1) / 2) + 'px';
      wm.style.transform = 'translate(-50%, -50%)';
      wm.textContent = e.name;
      EL.eraBands.appendChild(wm);
      if (EL.legend) {
        var sw = document.createElement('span');
        sw.className = 'cal-timeline-tuner-legend-swatch';
        sw.style.background = e.color;
        sw.textContent = e.name;
        sw.addEventListener('click', function () { jumpToDay(startIdx + (endIdx - startIdx) / 2); });
        EL.legend.appendChild(sw);
      }
    });
  }

  // ── Render: swim-lanes + event cards (§D/§E) ───────────────────────
  function tierClass(t) { return t === 'major' ? 'major' : (t === 'detail' ? 'detail' : 'standard'); }
  function catIcon(id) {
    var c = (DATA && DATA.categories || []).filter(function (x) { return x.id === id; })[0];
    return c ? (c.name || '').charAt(0) : '•';
  }
  function renderLanes() {
    if (!EL.lanesUpper || !EL.lanesLower) return;
    EL.lanesUpper.innerHTML = ''; EL.lanesLower.innerHTML = '';
    var lanes = groupLanes((DATA && DATA.events) || [], STATE.group);
    var half = Math.ceil(lanes.length / 2);
    lanes.forEach(function (lane, i) {
      var upper = i < half;
      var host = upper ? EL.lanesUpper : EL.lanesLower;
      var laneIdx = upper ? i : (i - half);
      var laneEl = document.createElement('div');
      laneEl.className = 'cal-timeline-tuner-lane';
      var y = upper ? (laneIdx * 96 + 6) : (laneIdx * 96 + 6);
      laneEl.style[upper ? 'bottom' : 'top'] = (laneIdx * 96 + 4) + 'px';
      laneEl.style.height = '92px';
      var label = document.createElement('span');
      label.className = 'cal-timeline-tuner-lane-label';
      label.textContent = lane.label;
      laneEl.appendChild(label);
      lane.events.forEach(function (ev) {
        var idx = dayIndex(ev.year, ev.month, ev.day);
        var card = document.createElement('div');
        card.className = 'cal-timeline-tuner-card cal-timeline-tuner-card--' + tierClass(ev.tier);
        card.setAttribute('data-event-id', ev.id);
        card.style.left = dayToX(idx) + 'px';
        card.style[upper ? 'bottom' : 'top'] = '24px';
        if (lane.color) card.style.borderLeftColor = lane.color;
        var title = document.createElement('div');
        title.className = 'cal-timeline-tuner-card-title';
        title.textContent = ev.name;
        card.appendChild(title);
        if (ev.tier !== 'detail') {
          var sub = document.createElement('div');
          sub.className = 'cal-timeline-tuner-card-sub';
          sub.textContent = 'M' + ev.month + ' ' + ev.year;
          card.appendChild(sub);
        }
        card.addEventListener('mouseenter', function () { setHover(ev.id); });
        card.addEventListener('mouseleave', function () { setHover(null); });
        card.addEventListener('click', function (e) { e.stopPropagation(); openPopup(ev, card); });
        laneEl.appendChild(card);
      });
      host.appendChild(laneEl);
    });
  }

  // ── Render: connection arcs (§F) ───────────────────────────────────
  function cardCenter(eventId) {
    var card = EL.canvas && EL.canvas.querySelector
      ? EL.canvas.querySelector('[data-event-id="' + eventId + '"]') : null;
    if (!card || !card.getBoundingClientRect || !EL.canvas) return null;
    var cr = card.getBoundingClientRect(), pr = EL.canvas.getBoundingClientRect();
    return { x: cr.left - pr.left + cr.width / 2, y: cr.top - pr.top + cr.height / 2 };
  }
  function renderConnections() {
    if (!EL.conns) return;
    EL.conns.innerHTML = '';
    var conns = (DATA && DATA.connections) || [];
    var visible = STATE.showAll
      ? conns
      : (STATE.hovered ? connectionsFor(STATE.hovered) : []);
    var hoveredSet = STATE.hovered ? connectionsFor(STATE.hovered) : [];
    visible.forEach(function (c) {
      var a = cardCenter(c.source), b = cardCenter(c.target);
      if (!a || !b) return;
      var midY = (a.y + b.y) / 2 - 40;
      var path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      path.setAttribute('d', 'M ' + a.x + ' ' + a.y + ' Q ' + ((a.x + b.x) / 2) + ' ' + midY + ' ' + b.x + ' ' + b.y);
      path.setAttribute('class', 'cal-timeline-tuner-arc ' + arcDashClass(c.type));
      path.style.stroke = entityColor(c.entity_id);
      if (STATE.showAll && STATE.hovered && hoveredSet.indexOf(c) === -1) {
        path.setAttribute('class', path.getAttribute('class') + ' is-faded');
      }
      if (!reduced()) path.classList && path.classList.add('is-drawing');
      EL.conns.appendChild(path);
    });
  }

  function setHover(id) {
    STATE.hovered = id;
    renderConnections();
    if (EL.canvas && EL.canvas.querySelectorAll) {
      Array.prototype.forEach.call(EL.canvas.querySelectorAll('[data-event-id]'), function (card) {
        var related = id && (card.getAttribute('data-event-id') === id ||
          connectionsFor(id).some(function (c) { return c.source === card.getAttribute('data-event-id') || c.target === card.getAttribute('data-event-id'); }));
        if (card.classList) card.classList.toggle('is-dim', !!id && !related);
      });
    }
  }

  // ── Render: atmospheric backdrop (§J2) ─────────────────────────────
  function renderBackdrop() {
    if (!EL.backdrop) return;
    EL.backdrop.innerHTML = '';
    if (!STATE.backdrop) { if (EL.backdrop.classList) EL.backdrop.classList.remove('is-active'); return; }
    var dt = fromDayIndex(Math.round(STATE.cursorDay));
    var plan = backdropPlan(dateKey(dt.year, dt.month, dt.day));
    if (!plan.active) { if (EL.backdrop.classList) EL.backdrop.classList.remove('is-active'); return; }
    // Weather first (bottom), celestial on top — §J2 precedence.
    if (plan.weather && WEATHER_EFFECTS[plan.weather]) {
      WEATHER_EFFECTS[plan.weather].timelineBackdropRender({ container: EL.backdrop, opacity: 1 });
    }
    plan.celestial.forEach(function (type) {
      if (CELESTIAL_EFFECTS[type]) CELESTIAL_EFFECTS[type].timelineBackdropRender({ container: EL.backdrop, opacity: 1 });
    });
    // Special-moon-day: the full sky-band (sun + moons) — the rare render.
    if (plan.sunMoon) {
      var sky = document.createElement('div'); sky.className = 'cal-timeline-tuner-backdrop-sky';
      EL.backdrop.appendChild(sky);
      var sun = document.createElement('div'); sun.className = 'cal-timeline-tuner-backdrop-sun';
      EL.backdrop.appendChild(sun);
      var moon = document.createElement('div'); moon.className = 'cal-timeline-tuner-backdrop-moon';
      moon.style.left = '64%'; moon.style.top = '34%';
      EL.backdrop.appendChild(moon);
    }
    if (EL.backdrop.classList) EL.backdrop.classList.add('is-active');
  }

  // ── Needle + cursor (§B) ───────────────────────────────────────────
  function renderNeedle() {
    if (EL.needle) EL.needle.style.left = dayToX(STATE.cursorDay) + 'px';
    var label = fmtDate(STATE.cursorDay);
    if (EL.cursorChip) EL.cursorChip.textContent = label;
    if (EL.cursorDate) EL.cursorDate.textContent = label;
    var era = eraAt(fromDayIndex(Math.round(STATE.cursorDay)).year);
    if (EL.eraName) EL.eraName.textContent = era ? era.name : '—';
  }

  function renderAll() {
    renderEras(); renderBackdrop(); renderLanes(); renderTicks();
    renderOverlays(); renderNeedle(); renderConnections();
  }

  function setCursorDay(idx, opts) {
    STATE.cursorDay = idx;
    renderNeedle(); renderBackdrop();
    if (CURSOR_SYNC) {
      var dt = fromDayIndex(Math.round(idx));
      CURSOR_SYNC.emitCursorChange({ year: dt.year, month: dt.month, day: dt.day }, opts);
    }
  }
  function jumpToDay(idx) { setCursorDay(idx, { immediate: true }); }

  // Apply an external sibling's cursor change to our needle (§J1).
  function applyExternalDate(date) {
    if (!date) return;
    var idx = (typeof date === 'object')
      ? dayIndex(date.year, date.month || 1, date.day || 1)
      : Math.round(+date);
    if (isNaN(idx)) return;
    STATE.cursorDay = idx;
    renderNeedle(); renderBackdrop();
  }

  // ── Zoom centered on a screen x (§G) ───────────────────────────────
  function zoomAt(screenX, factor) {
    var dayUnderCursor = xToDay(screenX);
    STATE.pxPerDay = Math.max(0.001, Math.min(600, STATE.pxPerDay * factor));
    STATE.originDay = dayUnderCursor - screenX / STATE.pxPerDay;
    renderAll();
  }

  // ── Block: interactions — pan/zoom/scrub/keys ──────────────────────
  registerInitBlock('interactions', function () {
    if (!EL.canvas) return;
    var dragging = false, lastX = 0, draggingNeedle = false;

    EL.canvas.addEventListener('wheel', function (ev) {
      ev.preventDefault();
      if (ev.shiftKey) { STATE.originDay += (ev.deltaY) / STATE.pxPerDay; renderAll(); return; }
      var rect = EL.canvas.getBoundingClientRect();
      zoomAt(ev.clientX - rect.left, ev.deltaY < 0 ? 1.15 : 1 / 1.15);
    }, { passive: false });

    EL.canvas.addEventListener('pointerdown', function (ev) {
      var rect = EL.canvas.getBoundingClientRect();
      var x = ev.clientX - rect.left;
      // Needle hit-test (within ~8px of the needle) → scrub.
      if (Math.abs(x - dayToX(STATE.cursorDay)) < 8) {
        draggingNeedle = true; if (EL.needle && EL.needle.classList) EL.needle.classList.add('is-dragging');
        ev.stopPropagation(); return;
      }
      dragging = true; lastX = ev.clientX;
    });
    document.addEventListener('pointermove', function (ev) {
      if (draggingNeedle) {
        var rect = EL.canvas.getBoundingClientRect();
        setCursorDay(xToDay(ev.clientX - rect.left));
        return;
      }
      if (!dragging) return;
      STATE.originDay -= (ev.clientX - lastX) / STATE.pxPerDay; lastX = ev.clientX; renderAll();
    });
    document.addEventListener('pointerup', function () {
      if (draggingNeedle) { setCursorDay(STATE.cursorDay, { immediate: true }); if (EL.needle && EL.needle.classList) EL.needle.classList.remove('is-dragging'); }
      dragging = false; draggingNeedle = false;
    });

    // Click on the axis → jump the needle there (§B).
    if (EL.axis) EL.axis.addEventListener('click', function (ev) {
      var rect = EL.canvas.getBoundingClientRect();
      jumpToDay(xToDay(ev.clientX - rect.left));
    });

    // Keyboard (§G).
    if (EL.root) EL.root.addEventListener('keydown', function (ev) {
      switch (ev.key) {
        case 'ArrowLeft': STATE.originDay -= 40 / STATE.pxPerDay; renderAll(); break;
        case 'ArrowRight': STATE.originDay += 40 / STATE.pxPerDay; renderAll(); break;
        case 'ArrowUp': case 'ArrowDown': toggleShowAll(); break;
        case '+': case '=': zoomAt(canvasWidth() / 2, 1.2); break;
        case '-': case '_': zoomAt(canvasWidth() / 2, 1 / 1.2); break;
        case 'Home': jumpToDay(dayIndex(DATA.current_year, DATA.current_month, DATA.current_day)); break;
        case 'Escape': closePopup(); break;
        default: return;
      }
      ev.preventDefault();
    });
  });

  // ── Block: chrome controls ─────────────────────────────────────────
  function toggleShowAll() {
    STATE.showAll = !STATE.showAll;
    var cb = q('[data-tuner-show-all]'); if (cb) cb.checked = STATE.showAll;
    renderConnections();
  }
  registerInitBlock('controls', function () {
    var zin = q('[data-tuner-zoom-in]'); if (zin) zin.addEventListener('click', function () { zoomAt(canvasWidth() / 2, 1.2); });
    var zout = q('[data-tuner-zoom-out]'); if (zout) zout.addEventListener('click', function () { zoomAt(canvasWidth() / 2, 1 / 1.2); });
    var fit = q('[data-tuner-fit]'); if (fit) fit.addEventListener('click', zoomToFit);
    var grp = q('[data-tuner-group]'); if (grp) grp.addEventListener('change', function () { STATE.group = grp.value; renderLanes(); renderConnections(); });
    var all = q('[data-tuner-show-all]'); if (all) all.addEventListener('change', function () { STATE.showAll = all.checked; renderConnections(); });
    var bd = q('[data-tuner-backdrop-toggle]'); if (bd) bd.addEventListener('change', function () { STATE.backdrop = bd.checked; renderBackdrop(); });
  });

  function zoomToFit() {
    var evs = (DATA && DATA.events) || []; if (!evs.length) return;
    var min = Infinity, max = -Infinity;
    evs.forEach(function (e) { var i = dayIndex(e.year, e.month, e.day); if (i < min) min = i; if (i > max) max = i; });
    var span = Math.max(1, max - min);
    STATE.pxPerDay = Math.max(0.001, Math.min(600, (canvasWidth() * 0.9) / span));
    STATE.originDay = min - (canvasWidth() * 0.05) / STATE.pxPerDay;
    renderAll();
  }

  // ── Block: popup (§E shared event detail) ──────────────────────────
  function openPopup(ev, card) {
    if (!EL.popup) return;
    var set = function (sel, txt) { var n = EL.popup.querySelector(sel); if (n) n.textContent = txt; };
    set('[data-tuner-popup-title]', ev.name);
    set('[data-tuner-popup-date]', 'M' + ev.month + ' ' + ev.day + ', ' + ev.year + ' ' + ((DATA.calendar && DATA.calendar.epoch_name) || ''));
    set('[data-tuner-popup-meta]', (ev.category || '') + ' · ' + (ev.tier || ''));
    set('[data-tuner-popup-desc]', ev.description || '');
    var ents = EL.popup.querySelector('[data-tuner-popup-entities]');
    if (ents) {
      ents.innerHTML = '';
      (ev.entities || []).forEach(function (eid) {
        var en = (DATA.entities || []).filter(function (x) { return x.id === eid; })[0];
        var chip = document.createElement('span');
        chip.className = 'cal-timeline-tuner-entity-chip';
        chip.style.background = (en && en.color) || 'oklch(0.4 0.02 260)';
        chip.textContent = (en && en.name) || eid;
        ents.appendChild(chip);
      });
    }
    var expanded = EL.popup.querySelector('[data-tuner-popup-expanded]');
    if (expanded) expanded.hidden = true;
    EL.popup.hidden = false;
    if (card && card.getBoundingClientRect && EL.canvas) {
      var cr = card.getBoundingClientRect(), pr = EL.canvas.getBoundingClientRect();
      EL.popup.style.left = Math.min(pr.width - 270, Math.max(8, cr.left - pr.left)) + 'px';
      EL.popup.style.top = Math.max(8, cr.top - pr.top - 8) + 'px';
    }
    if (EL.popup.classList) EL.popup.classList.add('is-open');
  }
  function closePopup() {
    if (!EL.popup) return;
    if (EL.popup.classList) EL.popup.classList.remove('is-open');
    EL.popup.hidden = true;
  }
  registerInitBlock('popup', function () {
    if (!EL.popup) return;
    var close = EL.popup.querySelector('[data-tuner-popup-close]'); if (close) close.addEventListener('click', closePopup);
    var expand = EL.popup.querySelector('[data-tuner-popup-expand]');
    if (expand) expand.addEventListener('click', function () {
      var ex = EL.popup.querySelector('[data-tuner-popup-expanded]'); if (ex) ex.hidden = !ex.hidden;
    });
    if (EL.cursorChip) EL.cursorChip.addEventListener('click', function () {
      var input = prompt && prompt('Jump to year:', String(fromDayIndex(Math.round(STATE.cursorDay)).year));
      if (input != null && input !== '') { var y = parseInt(input, 10); if (!isNaN(y)) jumpToDay(dayIndex(y, 1, 1)); }
    });
  });

  // ── Block: cursor-sync (§J1) ───────────────────────────────────────
  registerInitBlock('cursor-sync', function () {
    CURSOR_SYNC = makeCursorSync(applyExternalDate);
    window.__tunerCursorSync = CURSOR_SYNC;
  });

  // ── Block: first paint ─────────────────────────────────────────────
  registerInitBlock('paint', function () { renderAll(); });

  // Expose state + render for tests/debug.
  window.__tunerState = STATE;
  window.__tunerRenderAll = renderAll;
  window.__tunerSetCursorDay = setCursorDay;

  // ── Init trigger ───────────────────────────────────────────────────
  function init() {
    if (window.__calTunerInited) return;
    window.__tunerResults = runAll();
    window.__calTunerInited = true;
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
})();
