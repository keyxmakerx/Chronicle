/*
 * availability.js — availability scheduler client (C-SCHED-P1).
 *
 * Renders two views on the availability page shell:
 *   1. "My availability" paint grid — a 7-day × 24-hour recurring pattern the
 *      member paints (available / preferred / erase), saved zone-local.
 *   2. "Group availability / Team heatmap" overlay — the signed encoding:
 *      a green DENSITY UNDERLAY per hour (deeper = more free), a COUNT gutter
 *      (n / N, ★ = full house AND all prefer), and — for the DM only — slim
 *      OPAQUE per-member lanes so identity never blends into the wash. Rendered
 *      in the DM's zone with an explicit zone label.
 *
 * ES5 style (var + function expressions, no arrow/template literals) to match
 * static/js/boot.js. Uses Chronicle.apiFetch / Chronicle.escapeHtml.
 */
(function () {
  'use strict';

  // Display order is Monday-first; storage day_of_week is 0=Sun..6=Sat.
  var DISPLAY_DAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
  var DISPLAY_DOW = [1, 2, 3, 4, 5, 6, 0]; // column index -> stored day_of_week
  var HOURS = 24;
  var STATE_AVAIL = 'available';
  var STATE_PREFER = 'preferred';

  // A compact curated IANA list for the member's zone selector. The member's
  // stored zone and the browser-detected zone are merged in at init.
  var COMMON_TZ = [
    'UTC', 'America/New_York', 'America/Chicago', 'America/Denver',
    'America/Los_Angeles', 'America/Anchorage', 'America/Phoenix',
    'America/Toronto', 'America/Sao_Paulo', 'Europe/London', 'Europe/Dublin',
    'Europe/Paris', 'Europe/Berlin', 'Europe/Madrid', 'Europe/Rome',
    'Europe/Athens', 'Europe/Moscow', 'Africa/Johannesburg', 'Asia/Dubai',
    'Asia/Kolkata', 'Asia/Bangkok', 'Asia/Singapore', 'Asia/Shanghai',
    'Asia/Tokyo', 'Australia/Sydney', 'Pacific/Auckland'
  ];

  function $(sel, root) { return (root || document).querySelector(sel); }
  function el(tag, cls) { var e = document.createElement(tag); if (cls) e.className = cls; return e; }
  function pad2(n) { return (n < 10 ? '0' : '') + n; }

  function hourLabel(h) {
    var s = h < 12 ? 'AM' : 'PM';
    var hh = h % 12; if (hh === 0) hh = 12;
    return hh + ' ' + s;
  }

  // --- date helpers (UTC-anchored civil-date math; no wall-clock involved) ---
  function isoOf(d) { return d.getUTCFullYear() + '-' + pad2(d.getUTCMonth() + 1) + '-' + pad2(d.getUTCDate()); }
  function parseISO(s) { var p = String(s).split('-'); return new Date(Date.UTC(+p[0], +p[1] - 1, +p[2])); }
  function addDays(d, n) { var c = new Date(d.getTime()); c.setUTCDate(c.getUTCDate() + n); return c; }
  function mondayOf(d) {
    var wd = d.getUTCDay();          // 0=Sun..6=Sat
    var off = (wd + 6) % 7;          // days since Monday
    return addDays(d, -off);
  }
  function todayUTC() { var n = new Date(); return new Date(Date.UTC(n.getFullYear(), n.getMonth(), n.getDate())); }
  function firstOfMonth(d) { return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), 1)); }
  var MONTH_NAMES = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];

  // Steep perceptual ramp (signed mockup densityBg): near-nothing for 1 free,
  // unmistakable for a full house — the best windows must jump out.
  function densityAlpha(n, total) {
    if (!n) return 0;
    if (n >= total) return 0.85;
    var denom = Math.max(total - 1, 1);
    return 0.05 + 0.62 * Math.pow((n - 1) / denom, 1.35);
  }
  function heatRGBA(n, total) {
    var a = densityAlpha(n, total);
    if (a <= 0) return 'transparent';
    return 'rgba(34,163,90,' + a.toFixed(2) + ')';
  }

  function detectTZ() {
    try { return Intl.DateTimeFormat().resolvedOptions().timeZone || ''; } catch (e) { return ''; }
  }

  // Inject scoped styles once.
  function injectStyles() {
    if ($('#avail-styles')) return;
    var css =
      '[data-availability-root]{--avail-hpx:26px}' +
      '.avail-toolbar{display:flex;flex-wrap:wrap;align-items:center;gap:10px 14px;margin-bottom:12px}' +
      '.avail-seg{display:inline-flex;background:var(--color-bg-tertiary,#f3f4f6);border:1px solid var(--color-border,#e5e7eb);border-radius:9px;padding:2px}' +
      '.avail-seg button{border:0;background:none;cursor:pointer;padding:6px 12px;border-radius:7px;font-weight:600;font-size:12.5px;color:var(--color-text-secondary,#6b7280)}' +
      '.avail-seg button[aria-pressed="true"]{background:var(--color-bg-secondary,#fff);color:var(--color-text-primary,#111);box-shadow:0 1px 2px rgba(16,24,40,.1)}' +
      '.avail-grid{display:grid;grid-template-columns:56px repeat(7,1fr);border:1px solid var(--color-border,#e5e7eb);border-radius:12px;overflow:hidden;background:var(--color-border,#e5e7eb);gap:1px;user-select:none}' +
      '.avail-grid .hcell{background:var(--color-bg-secondary,#fff);font:600 10px/1 inherit;color:var(--color-text-muted,#9ca3af);text-align:right;padding:3px 6px 0 0}' +
      '.avail-grid .dhead{background:var(--color-bg-secondary,#fff);text-align:center;font:700 11.5px/1.3 inherit;color:var(--color-text-body,#374151);padding:7px 2px}' +
      '.avail-grid .corner{background:var(--color-bg-secondary,#fff)}' +
      '.avail-cell{background:var(--color-bg-secondary,#fff);height:var(--avail-hpx);border:0;padding:0;cursor:pointer;position:relative}' +
      '.avail-cell:hover{outline:2px solid var(--color-accent,#6366f1);outline-offset:-2px}' +
      '.avail-cell[data-state="available"]{background:rgba(34,163,90,.42)}' +
      '.avail-cell[data-state="preferred"]{background:rgba(34,163,90,.82);box-shadow:inset 0 0 0 2px var(--gold,#d69e0a)}' +
      '.avail-cell:focus-visible{outline:2px solid var(--color-accent,#6366f1);outline-offset:-2px;z-index:2}' +
      '.ov-daycol{position:relative;background:var(--color-bg-secondary,#fff);border-left:1px solid var(--color-border-light,#f3f4f6)}' +
      '.ov-band{position:absolute;left:0;right:0;pointer-events:none}' +
      '.ov-bn{position:absolute;top:50%;right:3px;transform:translateY(-50%);font:700 9.5px/1 inherit;color:var(--color-text-primary,#111);text-shadow:0 0 3px var(--color-bg-secondary,#fff),0 0 2px var(--color-bg-secondary,#fff)}' +
      '.ov-bn.gold{color:var(--gold,#d69e0a)}' +
      '.ov-lane{position:absolute;border-radius:3px;border-left:2px solid;overflow:hidden}' +
      '.ov-lane.pref{box-shadow:inset 0 0 0 1px rgba(0,0,0,.15)}' +
      '.avail-chip{display:inline-flex;align-items:center;gap:6px;border:1px solid var(--color-border,#e5e7eb);background:var(--color-bg-secondary,#fff);color:var(--color-text-body,#374151);font:600 12px/1 inherit;padding:5px 10px;border-radius:999px;cursor:pointer}' +
      '.avail-chip[aria-pressed="false"]{opacity:.5;text-decoration:line-through}' +
      '.avail-chip .dot{width:9px;height:9px;border-radius:999px}' +
      '.avail-legend{display:inline-flex;align-items:center;gap:6px 12px;font:600 11.5px/1 inherit;color:var(--color-text-secondary,#6b7280);flex-wrap:wrap}' +
      '.avail-note{font-size:12.5px;color:var(--color-text-secondary,#6b7280);margin:10px 2px 0}' +
      // month↔week morph stage (signed concept mockup encoding)
      '.ov-stage{position:relative}' +
      '.ov-view{transition:opacity .34s ease, transform .34s cubic-bezier(.32,.72,.33,1);transform-origin:top center}' +
      '.ov-view.hidden{display:none}' +
      '.ov-view.leave{opacity:0;transform:scale(.975)}' +
      '.ov-view.enter{opacity:0;transform:scaleY(.55);transform-origin:var(--ox,50%) top}' +
      '@media (prefers-reduced-motion:reduce){.ov-view{transition:none}}' +
      '.ov-month{display:grid;grid-template-columns:repeat(7,1fr);gap:1px;border:1px solid var(--color-border,#e5e7eb);border-radius:12px;overflow:hidden;background:var(--color-border,#e5e7eb)}' +
      '.ov-mhead{background:var(--color-bg-secondary,#fff);text-align:center;padding:7px 2px;font:700 11px/1 inherit;color:var(--color-text-secondary,#6b7280)}' +
      '.ov-mday{background:var(--color-bg-secondary,#fff);min-height:82px;padding:5px 6px 6px;cursor:pointer;border:0;display:flex;flex-direction:column;gap:4px;text-align:left}' +
      '.ov-mday:hover{background:var(--color-bg-tertiary,#f3f4f6)}' +
      '.ov-mday.out{opacity:.4}' +
      '.ov-mday .dn{font:700 11px/1 inherit;color:var(--color-text-body,#374151);display:flex;justify-content:space-between;align-items:center}' +
      '.ov-mday .pk{font:600 9px/1 inherit;color:var(--color-text-muted,#9ca3af)}' +
      '.ov-mini{display:flex;flex-direction:column;gap:1px;flex:1;min-height:38px;border-radius:3px;overflow:hidden}' +
      '.ov-mini i{flex:1;background:var(--color-bg-tertiary,#f3f4f6)}' +
      // slot builder
      '.ov-daycol.building{cursor:crosshair}' +
      '.ov-sel{position:absolute;left:1px;right:1px;background:rgba(99,102,241,.28);border:1.5px solid var(--color-accent,#6366f1);border-radius:4px;pointer-events:none}' +
      '.ov-slotchip{display:inline-flex;align-items:center;gap:8px;background:var(--color-bg-secondary,#fff);border:1px solid var(--color-border,#e5e7eb);border-radius:8px;padding:5px 10px;font:600 12px/1 inherit}' +
      '.ov-slotchip button{border:0;background:none;color:var(--color-text-muted,#9ca3af);cursor:pointer;font-size:13px}';
    var st = el('style'); st.id = 'avail-styles'; st.textContent = css;
    document.head.appendChild(st);
  }

  // ================= controller =================
  function AvailabilityApp(root) {
    this.root = root;
    this.campaignID = root.getAttribute('data-campaign-id');
    this.userID = root.getAttribute('data-user-id');
    this.canDetail = root.getAttribute('data-can-detail') === 'true';
    this.tz = root.getAttribute('data-tz') || detectTZ() || 'UTC';
    this.live = $('[data-avail-live]', root);
    // grid[displayCol 0..6][hour 0..23] = '' | 'available' | 'preferred'
    this.grid = [];
    this.tool = STATE_AVAIL;
    this.painting = false;
    this.paintValue = '';
    this.weekStart = mondayOf(todayUTC());
    this.overlayData = null;
    this.excluded = {};
    // Month↔week + slot builder (C-SCHED-P2).
    this.scale = 'week';                       // 'week' | 'month'
    this.monthAnchor = firstOfMonth(todayUTC());
    this.weekCache = {};                       // weekStartISO -> overlay payload
    this.building = false;                     // DM slot-builder active
    this.selectedSlots = [];                   // [{date, startMinute, endMinute}]
  }

  AvailabilityApp.prototype.announce = function (msg) {
    if (!this.live) return;
    this.live.textContent = '';
    var live = this.live;
    setTimeout(function () { live.textContent = msg; }, 30);
  };

  AvailabilityApp.prototype.init = function () {
    injectStyles();
    this.bindTabs();
    this.renderMine();
    this.announce('Availability editor ready');
    // Deep-link: /availability?tab=overlay jumps straight to the group view
    // (the "New proposal" entry point).
    if (/[?&]tab=overlay/.test(window.location.search)) this.showTab('overlay');
  };

  AvailabilityApp.prototype.bindTabs = function () {
    var self = this;
    var tabs = this.root.querySelectorAll('[data-avail-tab]');
    Array.prototype.forEach.call(tabs, function (t) {
      t.addEventListener('click', function () { self.showTab(t.getAttribute('data-avail-tab')); });
    });
  };

  AvailabilityApp.prototype.showTab = function (which) {
    var tabs = this.root.querySelectorAll('[data-avail-tab]');
    Array.prototype.forEach.call(tabs, function (t) {
      var on = t.getAttribute('data-avail-tab') === which;
      t.setAttribute('aria-selected', on ? 'true' : 'false');
      t.classList.toggle('bg-surface-raised', on);
      t.classList.toggle('text-fg', on);
    });
    $('[data-avail-panel="mine"]', this.root).hidden = (which !== 'mine');
    $('[data-avail-panel="overlay"]', this.root).hidden = (which !== 'overlay');
    if (which === 'overlay' && !this.overlayData) this.renderOverlayShell();
  };

  // ---------------- MY AVAILABILITY (paint grid) ----------------
  AvailabilityApp.prototype.emptyGrid = function () {
    var g = [];
    for (var c = 0; c < 7; c++) { g[c] = []; for (var h = 0; h < HOURS; h++) g[c][h] = ''; }
    return g;
  };

  AvailabilityApp.prototype.renderMine = function () {
    var self = this;
    var panel = $('[data-avail-panel="mine"]', this.root);
    panel.innerHTML = '';
    this.grid = this.emptyGrid();

    // Toolbar: timezone + paint tool + save.
    var bar = el('div', 'avail-toolbar');
    var tzWrap = el('label'); tzWrap.className = 'text-sm text-fg-secondary';
    tzWrap.appendChild(document.createTextNode('Your timezone '));
    var tzSel = el('select'); tzSel.className = 'ml-1 text-sm border border-edge rounded-md px-2 py-1 bg-surface';
    var zones = COMMON_TZ.slice();
    [this.tz, detectTZ()].forEach(function (z) { if (z && zones.indexOf(z) < 0) zones.unshift(z); });
    zones.forEach(function (z) {
      var o = el('option'); o.value = z; o.textContent = z; if (z === self.tz) o.selected = true; tzSel.appendChild(o);
    });
    tzSel.addEventListener('change', function () { self.tz = tzSel.value; });
    tzWrap.appendChild(tzSel);
    bar.appendChild(tzWrap);

    var seg = el('div', 'avail-seg'); seg.setAttribute('role', 'group'); seg.setAttribute('aria-label', 'Paint tool');
    [[STATE_AVAIL, 'Available'], [STATE_PREFER, 'Preferred'], ['', 'Erase']].forEach(function (t) {
      var b = el('button'); b.type = 'button'; b.textContent = t[1];
      b.setAttribute('aria-pressed', t[0] === self.tool ? 'true' : 'false');
      b.addEventListener('click', function () {
        self.tool = t[0];
        Array.prototype.forEach.call(seg.querySelectorAll('button'), function (x) { x.setAttribute('aria-pressed', 'false'); });
        b.setAttribute('aria-pressed', 'true');
      });
      seg.appendChild(b);
    });
    bar.appendChild(seg);

    var save = el('button', 'btn-primary text-sm'); save.type = 'button';
    save.innerHTML = '<i class="fa-solid fa-floppy-disk mr-1"></i> Save';
    save.addEventListener('click', function () { self.saveMine(save); });
    bar.appendChild(save);

    var status = el('span', 'text-xs text-fg-muted'); status.setAttribute('data-mine-status', '');
    bar.appendChild(status);
    panel.appendChild(bar);

    // Grid.
    var grid = el('div', 'avail-grid'); grid.setAttribute('role', 'grid');
    grid.appendChild(el('div', 'corner'));
    DISPLAY_DAYS.forEach(function (d) { var h = el('div', 'dhead'); h.textContent = d; grid.appendChild(h); });
    for (var h = 0; h < HOURS; h++) {
      var hc = el('div', 'hcell'); hc.textContent = hourLabel(h); grid.appendChild(hc);
      for (var c = 0; c < 7; c++) {
        (function (col, hour) {
          var cell = el('button', 'avail-cell'); cell.type = 'button';
          cell.setAttribute('data-col', col); cell.setAttribute('data-hour', hour);
          cell.setAttribute('aria-label', DISPLAY_DAYS[col] + ' ' + hourLabel(hour) + ', unavailable');
          cell.addEventListener('mousedown', function (e) { e.preventDefault(); self.beginPaint(cell); });
          cell.addEventListener('mouseenter', function () { if (self.painting) self.applyPaint(cell); });
          cell.addEventListener('click', function () { if (!self.painting) self.togglePaint(cell); });
          cell.addEventListener('keydown', function (e) {
            if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); self.togglePaint(cell); }
          });
          grid.appendChild(cell);
        })(c, h);
      }
    }
    document.addEventListener('mouseup', function () { self.painting = false; });
    panel.appendChild(grid);

    var note = el('p', 'avail-note');
    note.textContent = 'Click or drag to paint the hours you can play. This repeats every week. Times are in your timezone.';
    panel.appendChild(note);

    // One-off exceptions (C-SCHED-P2 0c) live below the recurring grid.
    var excHost = el('div'); excHost.setAttribute('data-exc-host', ''); excHost.style.marginTop = '18px';
    panel.appendChild(excHost);

    this.loadMine(status);
  };

  AvailabilityApp.prototype.cellAt = function (col, hour) {
    return $('.avail-cell[data-col="' + col + '"][data-hour="' + hour + '"]', this.root);
  };

  AvailabilityApp.prototype.setCell = function (cell, state) {
    var col = +cell.getAttribute('data-col'), hour = +cell.getAttribute('data-hour');
    this.grid[col][hour] = state;
    if (state) cell.setAttribute('data-state', state); else cell.removeAttribute('data-state');
    var label = state ? state : 'unavailable';
    cell.setAttribute('aria-label', DISPLAY_DAYS[col] + ' ' + hourLabel(hour) + ', ' + label);
    cell.setAttribute('aria-pressed', state ? 'true' : 'false');
  };

  AvailabilityApp.prototype.beginPaint = function (cell) {
    this.painting = true;
    var col = +cell.getAttribute('data-col'), hour = +cell.getAttribute('data-hour');
    // Toggle semantics on the seed cell: painting the same state erases it.
    this.paintValue = (this.grid[col][hour] === this.tool) ? '' : this.tool;
    this.setCell(cell, this.paintValue);
  };
  AvailabilityApp.prototype.applyPaint = function (cell) { this.setCell(cell, this.paintValue); };
  AvailabilityApp.prototype.togglePaint = function (cell) {
    var col = +cell.getAttribute('data-col'), hour = +cell.getAttribute('data-hour');
    this.setCell(cell, this.grid[col][hour] === this.tool ? '' : this.tool);
  };

  AvailabilityApp.prototype.loadMine = function (status) {
    var self = this;
    status.textContent = 'Loading…';
    Chronicle.apiFetch('/campaigns/' + this.campaignID + '/availability/mine')
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (data) {
        if (!data) { status.textContent = ''; return; }
        if (data.tz) { self.tz = data.tz; var sel = $('.avail-toolbar select', self.root); if (sel) sel.value = data.tz; }
        (data.blocks || []).forEach(function (b) {
          var col = DISPLAY_DOW.indexOf(b.dayOfWeek);
          if (col < 0) return;
          for (var h = Math.floor(b.startMinute / 60); h < Math.ceil(b.endMinute / 60) && h < HOURS; h++) {
            var cell = self.cellAt(col, h); if (cell) self.setCell(cell, b.state);
          }
        });
        status.textContent = '';
        self.renderExceptions();
      })
      .catch(function () { status.textContent = 'Could not load your availability.'; });
  };

  // Convert the painted grid into merged contiguous blocks (per day + state).
  AvailabilityApp.prototype.gridToBlocks = function () {
    var blocks = [];
    for (var col = 0; col < 7; col++) {
      var dow = DISPLAY_DOW[col];
      var run = null;
      for (var h = 0; h <= HOURS; h++) {
        var st = h < HOURS ? this.grid[col][h] : '';
        if (run && run.state === st) { run.end = h + 1; continue; }
        if (run) { blocks.push({ dayOfWeek: dow, startMinute: run.start * 60, endMinute: run.end * 60, state: run.state }); }
        run = st ? { state: st, start: h, end: h + 1 } : null;
      }
    }
    return blocks;
  };

  AvailabilityApp.prototype.saveMine = function (btn) {
    var self = this;
    var status = $('[data-mine-status]', this.root);
    btn.disabled = true; status.textContent = 'Saving…';
    Chronicle.apiFetch('/campaigns/' + this.campaignID + '/availability/mine', {
      method: 'PUT',
      body: { tz: this.tz, blocks: this.gridToBlocks() }
    }).then(function (r) {
      btn.disabled = false;
      if (r.ok) { status.textContent = 'Saved.'; self.announce('Availability saved'); self.overlayData = null; }
      else { r.json().then(function (j) { status.textContent = (j && j.error) || 'Save failed.'; }).catch(function () { status.textContent = 'Save failed.'; }); }
    }).catch(function () { btn.disabled = false; status.textContent = 'Save failed.'; });
  };

  // ---------------- TEAM OVERLAY (heatmap) ----------------
  AvailabilityApp.prototype.renderOverlayShell = function () {
    var self = this;
    var panel = $('[data-avail-panel="overlay"]', this.root);
    panel.innerHTML = '';

    var bar = el('div', 'avail-toolbar');

    // Month/Week scale toggle (signed concept: month grid ↔ week heatmap).
    var scale = el('div', 'avail-seg'); scale.setAttribute('role', 'group'); scale.setAttribute('aria-label', 'Calendar scale');
    [['month', 'Month'], ['week', 'Week']].forEach(function (opt) {
      var b = el('button'); b.type = 'button'; b.textContent = opt[1]; b.setAttribute('data-ov-scale', opt[0]);
      b.setAttribute('aria-pressed', opt[0] === self.scale ? 'true' : 'false');
      b.addEventListener('click', function () { self.setScale(opt[0]); });
      scale.appendChild(b);
    });
    bar.appendChild(scale);

    var prev = el('button', 'avail-chip'); prev.type = 'button'; prev.innerHTML = '<i class="fa-solid fa-chevron-left"></i>';
    prev.setAttribute('aria-label', 'Previous');
    prev.addEventListener('click', function () { self.step(-1); });
    var next = el('button', 'avail-chip'); next.type = 'button'; next.innerHTML = '<i class="fa-solid fa-chevron-right"></i>';
    next.setAttribute('aria-label', 'Next');
    next.addEventListener('click', function () { self.step(1); });
    var wlabel = el('span', 'text-sm font-semibold text-fg'); wlabel.setAttribute('data-week-label', '');
    bar.appendChild(prev); bar.appendChild(wlabel); bar.appendChild(next);

    // DM slot builder toggle (owner detail only).
    if (this.canDetail) {
      var build = el('button', 'btn-secondary text-sm'); build.type = 'button'; build.setAttribute('data-ov-build-toggle', '');
      build.innerHTML = '<i class="fa-solid fa-wand-magic-sparkles mr-1"></i> Propose times';
      build.addEventListener('click', function () { self.toggleBuilder(); });
      bar.appendChild(build);
    }

    var legend = el('span', 'avail-legend ml-auto');
    legend.innerHTML = '<span><span style="display:inline-block;width:12px;height:12px;border-radius:3px;background:rgba(34,163,90,.5);border:1px solid var(--color-border,#e5e7eb)"></span> greener = more free</span><span>★ = everyone free &amp; keen</span>';
    bar.appendChild(legend);
    panel.appendChild(bar);

    // Slot-builder tray (DM): selected slots + create button.
    if (this.canDetail) {
      var tray = el('div'); tray.setAttribute('data-ov-builder', ''); tray.hidden = true;
      tray.style.cssText = 'margin-bottom:12px;padding:10px 12px;border:1px dashed var(--color-accent,#6366f1);border-radius:10px';
      panel.appendChild(tray);
    }

    var chips = el('div', 'avail-toolbar'); chips.setAttribute('data-ov-chips', ''); panel.appendChild(chips);

    // Stage holds both views for the morph.
    var stage = el('div', 'ov-stage');
    var monthView = el('section', 'ov-view hidden'); monthView.setAttribute('data-ov-monthview', '');
    var monthHost = el('div'); monthHost.setAttribute('data-ov-month', ''); monthView.appendChild(monthHost);
    var weekView = el('section', 'ov-view'); weekView.setAttribute('data-ov-weekview', '');
    var host = el('div'); host.setAttribute('data-ov-grid', ''); weekView.appendChild(host);
    stage.appendChild(monthView); stage.appendChild(weekView);
    panel.appendChild(stage);

    var note = el('p', 'avail-note');
    note.textContent = this.canDetail
      ? 'The deepest green is the best window. Each slim lane is one player (solid = prefers), shown in your timezone. Switch to Month for the big picture, or Propose times to build a poll.'
      : 'Deeper green means more of the group can play then. Times are in your timezone.';
    panel.appendChild(note);

    if (this.scale === 'month') { monthView.classList.remove('hidden'); weekView.classList.add('hidden'); this.renderMonth(); }
    else { this.loadOverlay(); }
  };

  // setScale switches month↔week with the signed zoom-and-unfold morph, honoring
  // reduced-motion. Week keeps its current weekStart; month uses monthAnchor.
  AvailabilityApp.prototype.setScale = function (which, originWd) {
    if (which === this.scale) return;
    var self = this;
    this.scale = which;
    Array.prototype.forEach.call(this.root.querySelectorAll('[data-ov-scale]'), function (b) {
      b.setAttribute('aria-pressed', b.getAttribute('data-ov-scale') === which ? 'true' : 'false');
    });
    var monthView = $('[data-ov-monthview]', this.root), weekView = $('[data-ov-weekview]', this.root);
    var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    if (which === 'week') {
      this.loadOverlay();
      var ox = (originWd != null) ? Math.round(((originWd + 0.5) / 7) * 100) : 50;
      weekView.style.setProperty('--ox', ox + '%');
      if (reduce) { monthView.classList.add('hidden'); weekView.classList.remove('hidden'); this.announce('Week view'); return; }
      weekView.classList.remove('hidden'); weekView.classList.add('enter'); void weekView.offsetWidth;
      monthView.classList.add('leave'); weekView.classList.remove('enter');
      setTimeout(function () { monthView.classList.add('hidden'); monthView.classList.remove('leave'); }, 340);
      this.announce('Week view');
    } else {
      this.renderMonth();
      if (reduce) { weekView.classList.add('hidden'); monthView.classList.remove('hidden'); this.announce('Month view'); return; }
      monthView.classList.remove('hidden'); monthView.classList.add('leave'); void monthView.offsetWidth; monthView.classList.remove('leave');
      weekView.classList.add('leave');
      setTimeout(function () { weekView.classList.add('hidden'); weekView.classList.remove('leave'); }, 340);
      this.announce('Month view');
    }
  };

  // step advances the visible range by one unit of the current scale.
  AvailabilityApp.prototype.step = function (dir) {
    if (this.scale === 'month') {
      this.monthAnchor = new Date(Date.UTC(this.monthAnchor.getUTCFullYear(), this.monthAnchor.getUTCMonth() + dir, 1));
      this.renderMonth();
    } else {
      this.weekStart = addDays(this.weekStart, dir * 7);
      this.loadOverlay();
    }
  };

  AvailabilityApp.prototype.loadOverlay = function () {
    var self = this;
    var host = $('[data-ov-grid]', this.root);
    host.innerHTML = '<p class="text-sm text-fg-muted p-4">Loading…</p>';
    var url = '/campaigns/' + this.campaignID + '/availability/overlay?week=' +
      encodeURIComponent(isoOf(this.weekStart)) + '&tz=' + encodeURIComponent(this.tz);
    Chronicle.apiFetch(url).then(function (r) { return r.ok ? r.json() : null; }).then(function (data) {
      if (!data) { host.innerHTML = '<p class="text-sm text-fg-muted p-4">Could not load the group overlay.</p>'; return; }
      self.overlayData = data;
      self.weekCache[isoOf(self.weekStart)] = data;
      self.tz = data.viewerTz || self.tz;
      self.renderChips();
      self.renderOverlay();
      var tl = $('[data-tz-label-text]', self.root); if (tl) tl.textContent = data.viewerTz;
    }).catch(function () { host.innerHTML = '<p class="text-sm text-fg-muted p-4">Could not load the group overlay.</p>'; });
  };

  AvailabilityApp.prototype.includedMembers = function () {
    var self = this;
    return (this.overlayData.members || []).filter(function (m) { return !self.excluded[m.userId]; });
  };

  AvailabilityApp.prototype.renderChips = function () {
    var self = this;
    var wrap = $('[data-ov-chips]', this.root);
    wrap.innerHTML = '';
    if (!this.canDetail || !this.overlayData.members || !this.overlayData.members.length) return;
    this.overlayData.members.forEach(function (m) {
      var on = !self.excluded[m.userId];
      var b = el('button', 'avail-chip'); b.type = 'button'; b.setAttribute('aria-pressed', on ? 'true' : 'false');
      var dot = el('span', 'dot'); dot.style.background = m.color; b.appendChild(dot);
      var name = el('span'); name.textContent = m.name + (m.role === 'DM' ? ' (DM)' : ''); b.appendChild(name);
      b.addEventListener('click', function () {
        self.excluded[m.userId] = !self.excluded[m.userId];
        b.setAttribute('aria-pressed', self.excluded[m.userId] ? 'false' : 'true');
        self.renderOverlay();
      });
      wrap.appendChild(b);
    });
  };

  // Count free/prefer for one hour cell, honoring chip exclusions when we have
  // per-member detail; falling back to the server aggregate otherwise.
  AvailabilityApp.prototype.hourCounts = function (dayHour, includedIDs, total) {
    if (this.canDetail && dayHour.freeIds) {
      var free = 0, prefer = 0;
      dayHour.freeIds.forEach(function (id) { if (includedIDs[id]) free++; });
      (dayHour.preferIds || []).forEach(function (id) { if (includedIDs[id]) prefer++; });
      return { free: free, prefer: prefer, total: total };
    }
    return { free: dayHour.free, prefer: dayHour.prefer, total: total };
  };

  AvailabilityApp.prototype.renderOverlay = function () {
    var self = this;
    var data = this.overlayData;
    var host = $('[data-ov-grid]', this.root);
    var wlabel = $('[data-week-label]', this.root);
    var first = data.days[0], last = data.days[data.days.length - 1];
    if (wlabel) wlabel.textContent = 'Week of ' + first.date + ' – ' + last.date;

    var includedIDs = {}, total;
    if (this.canDetail) {
      this.includedMembers().forEach(function (m) { includedIDs[m.userId] = true; });
      total = this.includedMembers().length;
    } else {
      total = data.totalMembers;
    }
    if (total < 1) total = data.totalMembers || 1;

    var hpx = 26;
    var grid = el('div');
    grid.style.display = 'grid';
    grid.style.gridTemplateColumns = '56px repeat(7,1fr)';
    grid.style.border = '1px solid var(--color-border,#e5e7eb)';
    grid.style.borderRadius = '12px';
    grid.style.overflow = 'hidden';
    grid.style.background = 'var(--color-bg-secondary,#fff)';

    // header row
    grid.appendChild(el('div'));
    data.days.forEach(function (d) {
      var dh = el('div'); dh.style.textAlign = 'center'; dh.style.padding = '7px 2px';
      dh.style.font = '700 11.5px/1.3 inherit'; dh.style.color = 'var(--color-text-body,#374151)';
      dh.style.borderBottom = '1px solid var(--color-border,#e5e7eb)';
      dh.innerHTML = DISPLAY_DAYS[(d.weekday + 6) % 7] + '<br><span style="font-weight:500;color:var(--color-text-muted,#9ca3af);font-size:10px">' + Chronicle.escapeHtml(d.date.slice(5)) + '</span>';
      grid.appendChild(dh);
    });

    // axis + day columns
    var axis = el('div'); axis.style.position = 'relative';
    for (var h = 0; h < HOURS; h++) {
      var hl = el('div'); hl.style.height = hpx + 'px'; hl.style.font = '600 10px/1 inherit';
      hl.style.color = 'var(--color-text-muted,#9ca3af)'; hl.style.textAlign = 'right'; hl.style.padding = '2px 6px 0 0';
      hl.textContent = hourLabel(h); axis.appendChild(hl);
    }
    grid.appendChild(axis);

    data.days.forEach(function (day, di) {
      var col = el('div', 'ov-daycol'); col.style.height = (HOURS * hpx) + 'px';
      for (var h = 0; h < HOURS; h++) {
        var cnt = self.hourCounts(day.hours[h], includedIDs, total);
        if (cnt.free > 0) {
          var band = el('div', 'ov-band'); band.style.top = (h * hpx) + 'px'; band.style.height = hpx + 'px';
          band.style.background = heatRGBA(cnt.free, total);
          var full = cnt.free >= total, allKeen = full && cnt.prefer >= total;
          var bn = el('span', 'ov-bn' + (allKeen ? ' gold' : ''));
          bn.textContent = allKeen ? '★' : (full ? cnt.free + '/' + total : String(cnt.free));
          band.appendChild(bn);
          band.setAttribute('title', cnt.free + ' of ' + total + ' free at ' + hourLabel(h));
          col.appendChild(band);
        }
      }
      // slim opaque per-member lanes (owner detail only)
      if (self.canDetail) {
        var inc = self.includedMembers();
        var n = Math.max(inc.length, 1);
        inc.forEach(function (m, idx) {
          (m.lanes || []).forEach(function (ln) {
            if (ln.day !== di) return;
            var lane = el('div', 'ov-lane' + (ln.state === STATE_PREFER ? ' pref' : ''));
            lane.style.top = (ln.start / 60 * hpx + 1) + 'px';
            lane.style.height = Math.max(((ln.end - ln.start) / 60 * hpx - 2), 3) + 'px';
            lane.style.left = (2 + idx * (82 / n)) + '%';
            lane.style.width = Math.max(82 / n - 1.5, 5) + '%';
            lane.style.borderLeftColor = m.color;
            // OPAQUE mix so player colour never blends into the green wash.
            lane.style.background = 'color-mix(in srgb, ' + m.color + ' ' + (ln.state === STATE_PREFER ? '58%' : '34%') + ', var(--color-bg-secondary,#fff))';
            lane.setAttribute('title', m.name + ' · ' + hourLabel(Math.floor(ln.start / 60)) + '–' + hourLabel(Math.floor(ln.end / 60)) + (ln.state === STATE_PREFER ? ' (prefers)' : ''));
            col.appendChild(lane);
          });
        });
      }
      grid.appendChild(col);
    });

    host.innerHTML = '';
    host.appendChild(grid);
    if (this.building) this.attachBuilder(grid);
    this.announce('Group availability for week of ' + first.date);
  };

  // ---------------- MONTH VIEW ----------------
  // loadWeekData fetches (and caches) one week's overlay payload, then calls cb.
  AvailabilityApp.prototype.loadWeekData = function (weekStartISO, cb) {
    var self = this;
    if (this.weekCache[weekStartISO]) { cb(this.weekCache[weekStartISO]); return; }
    var url = '/campaigns/' + this.campaignID + '/availability/overlay?week=' +
      encodeURIComponent(weekStartISO) + '&tz=' + encodeURIComponent(this.tz);
    Chronicle.apiFetch(url).then(function (r) { return r.ok ? r.json() : null; }).then(function (data) {
      if (data) self.weekCache[weekStartISO] = data;
      cb(data);
    }).catch(function () { cb(null); });
  };

  AvailabilityApp.prototype.renderMonth = function () {
    var self = this;
    var host = $('[data-ov-month]', this.root);
    if (!host) return;
    var wlabel = $('[data-week-label]', this.root);
    var y = this.monthAnchor.getUTCFullYear(), mo = this.monthAnchor.getUTCMonth();
    if (wlabel) wlabel.textContent = MONTH_NAMES[mo] + ' ' + y;

    // Monday-first weeks spanning the month.
    var first = new Date(Date.UTC(y, mo, 1));
    var gridStart = mondayOf(first);
    var last = new Date(Date.UTC(y, mo + 1, 0));
    var weeks = [];
    for (var w = gridStart; w <= mondayOf(last); w = addDays(w, 7)) weeks.push(new Date(w.getTime()));

    host.innerHTML = '<p class="text-sm text-fg-muted p-4">Loading…</p>';

    // Fetch every visible week, then build a date -> hour-counts map.
    var pending = weeks.length, byDate = {}, total = this.overlayData ? this.overlayData.totalMembers : 0;
    weeks.forEach(function (wk) {
      self.loadWeekData(isoOf(wk), function (data) {
        if (data) {
          total = total || data.totalMembers;
          (data.days || []).forEach(function (d) {
            byDate[d.date] = d.hours.map(function (h) { return h.free; });
          });
        }
        if (--pending === 0) self.buildMonthGrid(host, y, mo, gridStart, weeks.length, byDate, total || 1);
      });
    });
  };

  AvailabilityApp.prototype.buildMonthGrid = function (host, y, mo, gridStart, weekCount, byDate, total) {
    var self = this;
    var grid = el('div', 'ov-month');
    DISPLAY_DAYS.forEach(function (dn) { var h = el('div', 'ov-mhead'); h.textContent = dn; grid.appendChild(h); });
    for (var i = 0; i < weekCount * 7; i++) {
      (function (idx) {
        var date = addDays(gridStart, idx);
        var iso = isoOf(date);
        var inMonth = date.getUTCMonth() === mo;
        var counts = byDate[iso] || [];
        var peak = counts.length ? Math.max.apply(null, counts) : 0;
        var cell = el('button', 'ov-mday' + (inMonth ? '' : ' out')); cell.type = 'button';
        cell.setAttribute('aria-label', iso + ' — up to ' + peak + ' of ' + total + ' free');
        var dn = el('div', 'dn');
        dn.innerHTML = '<span>' + date.getUTCDate() + '</span>' + (peak ? '<span class="pk">' + peak + ' free</span>' : '');
        cell.appendChild(dn);
        var mini = el('div', 'ov-mini'); mini.setAttribute('aria-hidden', 'true');
        for (var h = 0; h < HOURS; h++) {
          var seg = el('i'); var n = counts[h] || 0;
          if (n > 0) seg.style.background = heatRGBA(n, total);
          mini.appendChild(seg);
        }
        cell.appendChild(mini);
        cell.addEventListener('click', function () {
          self.weekStart = mondayOf(date);
          self.setScale('week', (date.getUTCDay() + 6) % 7);
        });
        grid.appendChild(cell);
      })(i);
    }
    host.innerHTML = '';
    host.appendChild(grid);
    this.announce('Month view ' + MONTH_NAMES[mo] + ' ' + y);
  };

  // ---------------- DM SLOT BUILDER ----------------
  // Click/drag on the week grid's day columns to select 1–5 candidate slots,
  // then create a proposal. Slots are viewer-zone wall-clocks (date + minutes);
  // the server resolves them to UTC instants (DST-correct).
  AvailabilityApp.prototype.toggleBuilder = function () {
    this.building = !this.building;
    var btn = $('[data-ov-build-toggle]', this.root);
    if (btn) btn.classList.toggle('!bg-accent', this.building);
    var tray = $('[data-ov-builder]', this.root);
    if (tray) tray.hidden = !this.building;
    if (this.building && this.scale !== 'week') { this.setScale('week'); }
    this.renderBuilderTray();
    if (this.overlayData) this.renderOverlay();
    this.announce(this.building ? 'Proposal builder on — drag to pick candidate slots' : 'Proposal builder off');
  };

  AvailabilityApp.prototype.attachBuilder = function (grid) {
    var self = this;
    var cols = grid.querySelectorAll('.ov-daycol');
    var hpx = 26;
    Array.prototype.forEach.call(cols, function (col, di) {
      col.classList.add('building');
      var day = self.overlayData.days[di];
      var drag = null, selEl = null;
      function minuteAt(e) {
        var rect = col.getBoundingClientRect();
        var yy = Math.max(0, Math.min(rect.height, e.clientY - rect.top));
        return Math.round(yy / hpx) * 60; // snap to the hour
      }
      col.addEventListener('mousedown', function (e) {
        e.preventDefault();
        drag = { start: minuteAt(e) };
        selEl = el('div', 'ov-sel'); col.appendChild(selEl);
      });
      col.addEventListener('mousemove', function (e) {
        if (!drag || !selEl) return;
        var cur = minuteAt(e);
        var a = Math.min(drag.start, cur), b = Math.max(drag.start, cur);
        selEl.style.top = (a / 60 * hpx) + 'px';
        selEl.style.height = Math.max((b - a) / 60 * hpx, hpx) + 'px';
      });
      col.addEventListener('mouseup', function (e) {
        if (!drag) return;
        var cur = minuteAt(e);
        var a = Math.min(drag.start, cur), b = Math.max(drag.start, cur);
        if (b <= a) b = a + 60;
        if (b > 1440) b = 1440;
        if (selEl && selEl.parentNode) selEl.parentNode.removeChild(selEl);
        drag = null; selEl = null;
        self.addSlot(day.date, a, b);
      });
    });
  };

  AvailabilityApp.prototype.addSlot = function (date, startMinute, endMinute) {
    if (this.selectedSlots.length >= 5) { this.announce('Up to 5 slots'); return; }
    this.selectedSlots.push({ date: date, startMinute: startMinute, endMinute: endMinute });
    this.renderBuilderTray();
    this.announce('Added slot ' + date + ' ' + hourLabel(startMinute / 60) + '–' + hourLabel(endMinute / 60));
  };

  AvailabilityApp.prototype.renderBuilderTray = function () {
    var self = this;
    var tray = $('[data-ov-builder]', this.root);
    if (!tray) return;
    tray.innerHTML = '';
    var head = el('div', 'flex items-center gap-2 mb-2');
    head.innerHTML = '<span class="text-sm font-semibold text-fg"><i class="fa-solid fa-wand-magic-sparkles mr-1"></i>Build a proposal</span><span class="text-xs text-fg-muted">Drag on a day to pick up to 5 candidate slots.</span>';
    tray.appendChild(head);

    var chips = el('div', 'flex flex-wrap items-center gap-2 mb-2');
    if (!this.selectedSlots.length) {
      var none = el('span', 'text-xs text-fg-muted'); none.textContent = 'No slots picked yet.'; chips.appendChild(none);
    }
    this.selectedSlots.forEach(function (s, i) {
      var chip = el('span', 'ov-slotchip');
      var lbl = el('span'); lbl.textContent = s.date + ' · ' + hourLabel(s.startMinute / 60) + '–' + hourLabel(s.endMinute / 60);
      chip.appendChild(lbl);
      var x = el('button'); x.type = 'button'; x.innerHTML = '<i class="fa-solid fa-xmark"></i>'; x.setAttribute('aria-label', 'Remove slot');
      x.addEventListener('click', function () { self.selectedSlots.splice(i, 1); self.renderBuilderTray(); });
      chip.appendChild(x); chips.appendChild(chip);
    });
    tray.appendChild(chips);

    var row = el('div', 'flex flex-wrap items-center gap-2');
    var title = el('input'); title.type = 'text'; title.placeholder = 'Proposal title (e.g. Next session)';
    title.className = 'text-sm border border-edge rounded-md px-2 py-1 bg-surface flex-1'; title.setAttribute('data-ov-proptitle', '');
    var create = el('button', 'btn-primary text-sm'); create.type = 'button'; create.innerHTML = '<i class="fa-solid fa-paper-plane mr-1"></i> Send proposal';
    create.disabled = this.selectedSlots.length < 1;
    create.addEventListener('click', function () { self.submitProposal(create); });
    var st = el('span', 'text-xs text-fg-muted'); st.setAttribute('data-ov-propstatus', '');
    row.appendChild(title); row.appendChild(create); row.appendChild(st);
    tray.appendChild(row);
  };

  AvailabilityApp.prototype.submitProposal = function (btn) {
    var self = this;
    var titleEl = $('[data-ov-proptitle]', this.root);
    var st = $('[data-ov-propstatus]', this.root);
    var title = titleEl ? titleEl.value.trim() : '';
    if (!title) { if (st) st.textContent = 'Add a title.'; return; }
    if (!this.selectedSlots.length) { if (st) st.textContent = 'Pick at least one slot.'; return; }
    btn.disabled = true; if (st) st.textContent = 'Sending…';
    Chronicle.apiFetch('/campaigns/' + this.campaignID + '/proposals', {
      method: 'POST',
      body: { title: title, tz: this.tz, options: this.selectedSlots }
    }).then(function (r) {
      return r.ok ? r.json() : r.json().then(function (j) { throw new Error((j && j.error) || 'Failed'); });
    }).then(function (res) {
      self.selectedSlots = [];
      self.building = false;
      var tray = $('[data-ov-builder]', self.root); if (tray) tray.hidden = true;
      if (self.overlayData) self.renderOverlay();
      if (window.Chronicle && Chronicle.notify) Chronicle.notify('Proposal sent — players notified', 'success');
      if (res && res.link) window.location.href = res.link;
    }).catch(function (e) {
      btn.disabled = false;
      if (st) st.textContent = (e && e.message) || 'Failed to send.';
    });
  };

  // ---------------- EXCEPTIONS (one-off, compose-the-day) ----------------
  // C-SCHED-P2 0c: exceptions REPLACE the whole day in storage, so the editor
  // COMPOSES the day — it pre-fills from the recurring pattern, so marking one
  // hour busy re-sends the rest instead of erasing it.

  // recurringDayState returns the recurring grid state for a real date's weekday
  // as a 24-entry array ('' | available | preferred).
  AvailabilityApp.prototype.recurringDayState = function (dateISO) {
    var d = parseISO(dateISO);
    var col = DISPLAY_DOW.indexOf(d.getUTCDay()); // storage dow -> display column
    var out = [];
    for (var h = 0; h < HOURS; h++) out[h] = (col >= 0 && this.grid[col]) ? (this.grid[col][h] || '') : '';
    return out;
  };

  AvailabilityApp.prototype.renderExceptions = function () {
    var self = this;
    var host = $('[data-exc-host]', this.root);
    if (!host) return;
    host.innerHTML = '';

    var head = el('div', 'flex flex-wrap items-center justify-between gap-2 mb-2');
    var h3 = el('h2', 'text-sm font-semibold text-fg');
    h3.innerHTML = '<i class="fa-solid fa-calendar-day mr-1"></i> One-off changes';
    head.appendChild(h3);
    var picker = el('div', 'flex items-center gap-2');
    var date = el('input'); date.type = 'date'; date.className = 'text-sm border border-edge rounded-md px-2 py-1 bg-surface';
    date.value = isoOf(todayUTC());
    var editBtn = el('button', 'btn-secondary text-sm'); editBtn.type = 'button';
    editBtn.innerHTML = '<i class="fa-solid fa-pen mr-1"></i> Edit this date';
    editBtn.addEventListener('click', function () { if (date.value) self.openDayEditor(date.value); });
    picker.appendChild(date); picker.appendChild(editBtn);
    head.appendChild(picker);
    host.appendChild(head);

    var help = el('p', 'avail-note'); help.style.marginTop = '0';
    help.textContent = 'Override a specific date without touching your weekly pattern. The editor starts from your usual week for that day, so trimming one hour keeps the rest.';
    host.appendChild(help);

    var list = el('div'); list.setAttribute('data-exc-list', ''); list.style.marginTop = '10px';
    host.appendChild(list);

    var editor = el('div'); editor.setAttribute('data-exc-editor', ''); editor.style.marginTop = '10px';
    host.appendChild(editor);

    this.loadExceptions();
  };

  AvailabilityApp.prototype.loadExceptions = function () {
    var self = this;
    var list = $('[data-exc-list]', this.root);
    if (!list) return;
    Chronicle.apiFetch('/campaigns/' + this.campaignID + '/availability/exceptions')
      .then(function (r) { return r.ok ? r.json() : []; })
      .then(function (excs) {
        // Group exceptions by date so the day is shown as one editable entry.
        var byDate = {};
        (excs || []).forEach(function (e) { (byDate[e.onDate] = byDate[e.onDate] || []).push(e); });
        var dates = Object.keys(byDate).sort();
        list.innerHTML = '';
        if (!dates.length) { return; }
        dates.forEach(function (dt) {
          var row = el('div', 'flex items-center justify-between gap-2 text-sm bg-surface-alt border border-edge rounded-md px-3 py-1.5 mb-1.5');
          var lbl = el('span', 'text-fg-secondary'); lbl.textContent = dt + ' — customized (' + byDate[dt].length + ' block' + (byDate[dt].length !== 1 ? 's' : '') + ')';
          row.appendChild(lbl);
          var actions = el('span', 'flex items-center gap-2');
          var edit = el('button', 'text-accent text-xs font-semibold'); edit.type = 'button'; edit.textContent = 'Edit';
          edit.addEventListener('click', function () { self.openDayEditor(dt); });
          var clr = el('button', 'text-fg-muted text-xs font-semibold'); clr.type = 'button'; clr.textContent = 'Revert to weekly';
          clr.addEventListener('click', function () { self.saveDayException(dt, [], clr); });
          actions.appendChild(edit); actions.appendChild(clr);
          row.appendChild(actions);
          list.appendChild(row);
        });
      })
      .catch(function () { /* non-fatal */ });
  };

  // openDayEditor builds a single-day, 24-hour compose editor pre-filled with the
  // day's EFFECTIVE availability (existing exception rows if any, else the
  // recurring pattern), so saving re-sends the whole day.
  AvailabilityApp.prototype.openDayEditor = function (dateISO) {
    var self = this;
    var editor = $('[data-exc-editor]', this.root);
    if (!editor) return;
    editor.innerHTML = '<p class="text-sm text-fg-muted">Loading…</p>';

    // Fetch existing exception blocks for the date; fall back to recurring.
    Chronicle.apiFetch('/campaigns/' + this.campaignID + '/availability/exceptions')
      .then(function (r) { return r.ok ? r.json() : []; })
      .then(function (excs) {
        var day = self.recurringDayState(dateISO);
        var forDate = (excs || []).filter(function (e) { return e.onDate === dateISO; });
        if (forDate.length) {
          // Compose from the stored exception set (unavailable punches holes).
          for (var h = 0; h < HOURS; h++) day[h] = '';
          forDate.forEach(function (e) {
            if (e.state === 'unavailable') return; // hole
            for (var h = Math.floor(e.startMinute / 60); h < Math.ceil(e.endMinute / 60) && h < HOURS; h++) day[h] = e.state;
          });
        }
        self.buildDayEditor(editor, dateISO, day);
      })
      .catch(function () { self.buildDayEditor(editor, dateISO, self.recurringDayState(dateISO)); });
  };

  AvailabilityApp.prototype.buildDayEditor = function (editor, dateISO, dayState) {
    var self = this;
    editor.innerHTML = '';
    var card = el('div', 'bg-surface border border-edge rounded-lg p-3');
    var title = el('div', 'flex items-center justify-between mb-2');
    var t = el('span', 'text-sm font-semibold text-fg'); t.textContent = 'Editing ' + dateISO;
    title.appendChild(t);
    var close = el('button', 'text-fg-muted text-xs'); close.type = 'button'; close.innerHTML = '<i class="fa-solid fa-xmark"></i>';
    close.addEventListener('click', function () { editor.innerHTML = ''; });
    title.appendChild(close);
    card.appendChild(title);

    var seg = el('div', 'avail-seg'); seg.style.marginBottom = '8px'; var tool = { v: STATE_AVAIL };
    [[STATE_AVAIL, 'Available'], [STATE_PREFER, 'Preferred'], ['', 'Busy']].forEach(function (opt) {
      var b = el('button'); b.type = 'button'; b.textContent = opt[1];
      b.setAttribute('aria-pressed', opt[0] === tool.v ? 'true' : 'false');
      b.addEventListener('click', function () {
        tool.v = opt[0];
        Array.prototype.forEach.call(seg.querySelectorAll('button'), function (x) { x.setAttribute('aria-pressed', 'false'); });
        b.setAttribute('aria-pressed', 'true');
      });
      seg.appendChild(b);
    });
    card.appendChild(seg);

    // 24 hour cells in a compact single-day strip.
    var strip = el('div'); strip.style.display = 'grid'; strip.style.gridTemplateColumns = 'repeat(12,1fr)'; strip.style.gap = '3px';
    var cells = [];
    var painting = { on: false, val: '' };
    for (var h = 0; h < HOURS; h++) {
      (function (hour) {
        var c = el('button', 'avail-cell'); c.type = 'button'; c.style.height = '30px'; c.style.borderRadius = '4px';
        c.title = hourLabel(hour);
        c.setAttribute('aria-label', hourLabel(hour) + ', ' + (dayState[hour] || 'busy'));
        if (dayState[hour]) c.setAttribute('data-state', dayState[hour]);
        var lab = el('span'); lab.style.cssText = 'position:absolute;bottom:1px;left:2px;font-size:8px;color:var(--color-text-muted,#9ca3af)'; lab.textContent = hour;
        c.appendChild(lab);
        function setC(v) { dayState[hour] = v; if (v) c.setAttribute('data-state', v); else c.removeAttribute('data-state'); c.setAttribute('aria-label', hourLabel(hour) + ', ' + (v || 'busy')); }
        c.addEventListener('mousedown', function (e) { e.preventDefault(); painting.on = true; painting.val = (dayState[hour] === tool.v) ? '' : tool.v; setC(painting.val); });
        c.addEventListener('mouseenter', function () { if (painting.on) setC(painting.val); });
        c.addEventListener('click', function () { if (!painting.on) setC(dayState[hour] === tool.v ? '' : tool.v); });
        cells.push(c); strip.appendChild(c);
      })(h);
    }
    document.addEventListener('mouseup', function () { painting.on = false; });
    card.appendChild(strip);

    var actions = el('div', 'flex items-center gap-2 mt-3');
    var save = el('button', 'btn-primary text-sm'); save.type = 'button'; save.innerHTML = '<i class="fa-solid fa-floppy-disk mr-1"></i> Save this date';
    save.addEventListener('click', function () { self.saveDayException(dateISO, self.dayStateToBlocks(dayState), save); });
    var st = el('span', 'text-xs text-fg-muted'); st.setAttribute('data-exc-editor-status', '');
    actions.appendChild(save); actions.appendChild(st);
    card.appendChild(actions);

    editor.appendChild(card);
  };

  // dayStateToBlocks merges a 24-entry day into contiguous blocks for the save.
  AvailabilityApp.prototype.dayStateToBlocks = function (dayState) {
    var blocks = [], run = null;
    for (var h = 0; h <= HOURS; h++) {
      var st = h < HOURS ? (dayState[h] || '') : '';
      if (run && run.state === st) { run.end = h + 1; continue; }
      if (run) blocks.push({ startMinute: run.start * 60, endMinute: run.end * 60, state: run.state });
      run = st ? { state: st, start: h, end: h + 1 } : null;
    }
    return blocks;
  };

  AvailabilityApp.prototype.saveDayException = function (dateISO, blocks, btn) {
    var self = this;
    if (btn) btn.disabled = true;
    var st = $('[data-exc-editor-status]', this.root);
    if (st) st.textContent = 'Saving…';
    Chronicle.apiFetch('/campaigns/' + this.campaignID + '/availability/exceptions', {
      method: 'PUT',
      body: { onDate: dateISO, tz: this.tz, blocks: blocks }
    }).then(function (r) {
      if (btn) btn.disabled = false;
      if (r.ok) {
        if (st) st.textContent = 'Saved.';
        self.announce('Saved changes for ' + dateISO);
        self.overlayData = null;
        var editor = $('[data-exc-editor]', self.root); if (editor) editor.innerHTML = '';
        self.loadExceptions();
      } else {
        r.json().then(function (j) { if (st) st.textContent = (j && j.error) || 'Save failed.'; }).catch(function () { if (st) st.textContent = 'Save failed.'; });
      }
    }).catch(function () { if (btn) btn.disabled = false; if (st) st.textContent = 'Save failed.'; });
  };

  // ================= boot =================
  function boot() {
    var root = document.querySelector('[data-availability-root]');
    if (!root || root.getAttribute('data-avail-booted')) return;
    root.setAttribute('data-avail-booted', '1');
    new AvailabilityApp(root).init();
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
  // Re-boot after HTMX swaps that replace the page body.
  document.body && document.body.addEventListener('htmx:afterSettle', boot);
})();
