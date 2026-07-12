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
      '.avail-note{font-size:12.5px;color:var(--color-text-secondary,#6b7280);margin:10px 2px 0}';
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
    var prev = el('button', 'avail-chip'); prev.type = 'button'; prev.innerHTML = '<i class="fa-solid fa-chevron-left"></i>';
    prev.setAttribute('aria-label', 'Previous week');
    prev.addEventListener('click', function () { self.weekStart = addDays(self.weekStart, -7); self.loadOverlay(); });
    var next = el('button', 'avail-chip'); next.type = 'button'; next.innerHTML = '<i class="fa-solid fa-chevron-right"></i>';
    next.setAttribute('aria-label', 'Next week');
    next.addEventListener('click', function () { self.weekStart = addDays(self.weekStart, 7); self.loadOverlay(); });
    var wlabel = el('span', 'text-sm font-semibold text-fg'); wlabel.setAttribute('data-week-label', '');
    bar.appendChild(prev); bar.appendChild(wlabel); bar.appendChild(next);

    var legend = el('span', 'avail-legend ml-auto');
    legend.innerHTML = '<span><span style="display:inline-block;width:12px;height:12px;border-radius:3px;background:rgba(34,163,90,.5);border:1px solid var(--color-border,#e5e7eb)"></span> greener = more free</span><span>★ = everyone free &amp; keen</span>';
    bar.appendChild(legend);
    panel.appendChild(bar);

    var chips = el('div', 'avail-toolbar'); chips.setAttribute('data-ov-chips', ''); panel.appendChild(chips);
    var host = el('div'); host.setAttribute('data-ov-grid', ''); panel.appendChild(host);
    var note = el('p', 'avail-note');
    note.textContent = this.canDetail
      ? 'The deepest green is the best window. Each slim lane is one player (solid = prefers), shown in your timezone.'
      : 'Deeper green means more of the group can play then. Times are in your timezone.';
    panel.appendChild(note);

    this.loadOverlay();
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
    this.announce('Group availability for week of ' + first.date);
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
