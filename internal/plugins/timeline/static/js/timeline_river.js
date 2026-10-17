/**
 * timeline_river.js -- the time line page's River view: one line through
 * history that drags, wheel- and pinch-zooms and coasts, events as dots
 * coloured by kind, cause and effect drawn as arcs under the line, crowded
 * dots gathered into counted bubbles, the open event docked beside the line,
 * and the calendar's own month folding down under it. The signed design is
 * issue #677's round 5 mockup.
 *
 * Mount: <div data-widget="timeline-river" data-campaign-id data-timeline-id
 *             data-api-url data-calendar-id>
 *
 * Reads GET .../timelines/:tid/data (events, eras and connections, already
 * filtered for the viewer) and, when the time line follows a calendar, that
 * calendar's structure and the month's events from the calendar's own
 * endpoints. Date maths is the calendar page's (Chronicle.calendarDate), so
 * a day lands where the calendar puts it.
 */
(function () {
  var NS = 'http://www.w3.org/2000/svg';
  var ICON_X = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg>';
  var ICON_L = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m15 6-6 6 6 6"/></svg>';
  var ICON_R = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m9 6 6 6-6 6"/></svg>';

  function esc(s) { return Chronicle.escapeHtml(String(s == null ? '' : s)); }
  // A colour from data lands in a style attribute, so only a plain hex passes.
  function safeHex(c) { return typeof c === 'string' && /^#[0-9a-fA-F]{3,8}$/.test(c) ? c : null; }
  function hashHue(s) { var h = 0; for (var i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) % 360; return h; }
  function motion() { return document.documentElement.getAttribute('data-motion') || 'full'; }
  function coarse() { return !!(window.matchMedia && matchMedia('(pointer: coarse)').matches); }

  // Calendar geometry: the calendar's own maths when the time line follows
  // one, otherwise twelve thirty-day months, which is how standalone dates
  // were always placed.
  function Geo(cal) {
    var CD = cal && Chronicle.calendarDate;
    this.cal = CD ? cal : null;
    this.CD = CD;
    this.mc = this.cal ? CD.monthCount(cal) : 12;
    this._yl = {};
  }
  Geo.prototype.monthDays = function (y, m1) { return this.cal ? this.CD.monthDays(this.cal, m1 - 1, y) : 30; };
  Geo.prototype.yearLen = function (y) {
    if (this._yl[y] == null) { var n = 0; for (var m = 1; m <= this.mc; m++) n += this.monthDays(y, m); this._yl[y] = n || 360; }
    return this._yl[y];
  };
  Geo.prototype.monthName = function (m1) {
    var ms = this.cal && this.cal.months;
    return ms && ms[m1 - 1] && ms[m1 - 1].name ? ms[m1 - 1].name : 'Month ' + m1;
  };
  // t is a fractional year: year + days into it / days in it.
  Geo.prototype.t = function (y, m1, d) {
    var n = 0; m1 = Math.max(1, Math.min(this.mc, m1 || 1));
    for (var m = 1; m < m1; m++) n += this.monthDays(y, m);
    return y + (n + Math.max(1, d || 1) - 1) / this.yearLen(y);
  };
  Geo.prototype.parts = function (t) {
    var y = Math.floor(t + 1e-9), left = Math.round((t - y) * this.yearLen(y)), m = 1;
    while (m < this.mc && left >= this.monthDays(y, m)) { left -= this.monthDays(y, m); m++; }
    return { y: y, m: m, d: Math.min(this.monthDays(y, m), left + 1) };
  };
  Geo.prototype.dateText = function (t) { var p = this.parts(t); return this.monthName(p.m) + ' ' + p.d + ', ' + p.y; };

  // One River per mount; boot.js shares a widget's impl across mounts, so
  // the per-mount state lives on its own object.
  var River = {
    init: function (el, config) {
      this.el = el;
      this.cfg = config;
      this.st = { sel: null, docked: false, hover: null, q: '', fold: false, month: null, hidden: {}, stack: [] };
      this.view = { t0: 0, t1: 1 }; this.target = { t0: 0, t1: 1 }; this.vel = 0;
      this.W = 800; this.H = 380; this.Y = 210;
      this.nodes = {}; this.arcs = {}; this.rows = {}; this.lastCl = '';
      this.raf = null;
      this._renderShell();
      this._load();
    },

    destroy: function () {
      if (this.raf) cancelAnimationFrame(this.raf);
      if (this._ro) this._ro.disconnect();
      if (this._onKey) document.removeEventListener('keydown', this._onKey);
      clearTimeout(this._peekTimer);
      this.el.innerHTML = '';
    },

    _load: function () {
      var self = this, cfg = this.cfg;
      Chronicle.apiFetch(cfg.apiUrl).then(function (r) { if (!r.ok) throw new Error('The server answered ' + r.status + '.'); return r.json(); }).then(function (data) {
        if (!cfg.calendarId) return [data, null];
        return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(cfg.campaignId) + '/calendars/' + encodeURIComponent(cfg.calendarId))
          .then(function (r) { return r.ok ? r.json() : null; })
          .catch(function () { return null; })
          .then(function (cal) { return [data, cal]; });
      }).then(function (res) {
        self._setData(res[0], res[1]);
      }).catch(function (err) {
        self.$('stage').innerHTML = '<p class="tlr-empty">The time line could not load. ' + esc(err && err.message) + '</p>';
      });
    },

    $: function (part) { return this.el.querySelector('[data-tlr="' + part + '"]'); },

    _renderShell: function () {
      this.el.classList.add('tlr-wrap');
      this.el.innerHTML = '<div class="tlr">' +
        '<nav class="tlr-eras" data-tlr="eras" aria-label="Eras" hidden></nav>' +
        '<div class="tlr-tools">' +
          '<span class="tlr-seg" role="group" aria-label="Zoom to"><button type="button" data-fit="all">All</button><button type="button" data-fit="now">Today</button></span>' +
          '<button type="button" class="tlr-btn sq" data-zoom="0.5" aria-label="Zoom in"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg></button>' +
          '<button type="button" class="tlr-btn sq" data-zoom="2" aria-label="Zoom out"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M5 12h14"/></svg></button>' +
          '<span class="tlr-range" data-tlr="range" aria-live="polite"></span><span class="tlr-grow"></span>' +
          '<label class="tlr-find"><span class="tlr-sr">Find an event</span><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg><input data-tlr="q" type="search" placeholder="Find an event" autocomplete="off"><span class="tlr-kbd" aria-hidden="true">/</span></label>' +
        '</div>' +
        '<div class="tlr-kinds" data-tlr="kinds" role="group" aria-label="Kinds of event" hidden></div>' +
        '<div class="tlr-body" data-tlr="body">' +
          '<div class="tlr-main">' +
            '<div class="tlr-stage" data-tlr="stage" tabindex="0" aria-label="Time line. Arrow keys move along it, plus and minus zoom, Tab reaches each event."><svg data-tlr="svg"></svg><span class="tlr-hint">Drag to move · scroll or pinch to zoom · click an event</span></div>' +
            '<div class="tlr-overview" data-tlr="ov" aria-label="The whole history. Drag the window to move."><svg data-tlr="ovsvg"></svg></div>' +
            '<button type="button" class="tlr-monthlink" data-tlr="monthlink" data-act="cal" aria-expanded="false" hidden></button>' +
            '<div class="tlr-fold" data-tlr="fold"><div><div class="tlr-cal" data-tlr="cal" aria-label="Month"></div></div></div>' +
          '</div>' +
          '<aside class="tlr-side" aria-label="Event"><div class="in" data-tlr="side"></div></aside>' +
        '</div></div>';
      this.stage = this.$('stage'); this.svg = this.$('svg'); this.ov = this.$('ov'); this.ovsvg = this.$('ovsvg'); this.body = this.$('body'); this.side = this.$('side');
      this.svg.innerHTML = '<g data-l="back"></g><g data-l="ticks"></g><path class="river-base" style="fill:none;stroke:var(--tlr-line-strong);stroke-width:2"></path>' +
        '<path class="river-fill" pathLength="1" style="fill:none;stroke:var(--tlr-acc);stroke-width:2;stroke-dasharray:1"></path><g data-l="bars"></g><g data-l="arcs"></g><g data-l="labs"></g><g data-l="cl"></g><g data-l="dots"></g>';
      var g = {}, svg = this.svg;
      ['back', 'ticks', 'bars', 'arcs', 'labs', 'cl', 'dots'].forEach(function (k) { g[k] = svg.querySelector('[data-l="' + k + '"]'); });
      this.g = g; this.base = svg.querySelector('.river-base'); this.fill = svg.querySelector('.river-fill');
      this._bind();
    },

    // Shapes the viewer-filtered payload into what the line draws.
    _setData: function (data, cal) {
      var self = this, geo = this.geo = new Geo(cal);
      this.cal = geo.cal;
      var kinds = {}, kindOrder = [];
      var evs = (data.events || []).filter(function (e) { return e.event_year != null && e.event_year === e.event_year; }).map(function (e) {
        var kind = (e.event_category || '').trim() || 'Event';
        var col = safeHex(e.color_override);
        if (!kinds[kind]) { kinds[kind] = { name: kind, color: col || 'oklch(var(--tlr-kL) var(--tlr-kC) ' + hashHue(kind.toLowerCase()) + ')' }; kindOrder.push(kind); }
        var vis = e.visibility_override || e.event_visibility || 'everyone';
        var t = geo.t(e.event_year, e.event_month, e.event_day);
        var end = e.event_end_year != null ? geo.t(e.event_end_year, e.event_end_month || e.event_month, e.event_end_day || e.event_day) : null;
        return {
          id: e.source + ':' + e.event_id, name: e.label || e.event_name || 'Untitled', t: t, end: end && end > t ? end : null,
          kind: kind, color: col, dm: vis === 'dm_only', desc: e.event_description || '',
          page: e.event_entity_id && e.event_entity_name ? { id: e.event_entity_id, name: e.event_entity_name } : null
        };
      }).sort(function (a, b) { return a.t - b.t; });
      this.kinds = kinds; this.kindOrder = kindOrder;
      this.events = evs; this.byId = {};
      evs.forEach(function (e) { self.byId[e.id] = e; });
      // The server already dropped connections touching an event this viewer can't see.
      this.links = (data.connections || []).map(function (c) { return [c.source_type + ':' + c.source_id, c.target_type + ':' + c.target_id]; })
        .filter(function (l) { return self.byId[l[0]] && self.byId[l[1]] && l[0] !== l[1]; });
      this.eras = (data.eras || []).map(function (e) { return { name: e.name, a: e.start_year, b: e.end_year != null ? e.end_year + 1 : Infinity }; })
        .filter(function (e) { return e.a === e.a; }).sort(function (a, b) { return a.a - b.a; });
      this.today = cal && cal.current_year ? geo.t(cal.current_year, cal.current_month, cal.current_day) : null;
      var lo = evs.length ? evs[0].t : (this.today != null ? this.today : 0), hi = lo;
      evs.forEach(function (e) { hi = Math.max(hi, e.end || e.t); });
      if (this.today != null) { lo = Math.min(lo, this.today); hi = Math.max(hi, this.today); }
      var pad = Math.max(1, (hi - lo) * 0.06);
      this.MIN = lo - pad; this.MAX = hi + pad;
      this._buildNodes(); this._buildArcs(); this._buildEras(); this._buildKinds();
      if (!evs.length) this.stage.insertAdjacentHTML('beforeend', '<p class="tlr-empty">No events on this time line yet.</p>');
      var t0 = this.MIN, t1 = this.MAX;
      if (motion() === 'full') { var w = (t1 - t0) * 0.15; this.view = { t0: t0 - w, t1: t1 + w }; this.svg.classList.add('intro'); var svgEl = this.svg; setTimeout(function () { svgEl.classList.remove('intro'); }, 1800); }
      else this.view = { t0: t0, t1: t1 };
      this.target = { t0: this.view.t0, t1: this.view.t1 };
      this.draw(); this.renderCal();
      this.setTarget(t0, t1);
    },

    X: function (t) { return (t - this.view.t0) / (this.view.t1 - this.view.t0) * this.W; },
    span: function () { return this.view.t1 - this.view.t0; },
    shown: function () { var h = this.st.hidden; return this.events.filter(function (e) { return !h[e.kind]; }); },
    shownLinks: function () { var self = this, ok = {}; this.shown().forEach(function (e) { ok[e.id] = 1; }); return this.links.filter(function (l) { return ok[l[0]] && ok[l[1]]; }); },
    matches: function (e) { var q = this.st.q.trim().toLowerCase(); return !!q && (e.name + ' ' + e.desc).toLowerCase().indexOf(q) >= 0; },
    kindColor: function (e) { return e.color || this.kinds[e.kind].color; },
    whenText: function (e) {
      var g = this.geo, s = g.dateText(e.t) + (e.end ? ' to ' + g.dateText(e.end) : '');
      if (this.today != null && Math.abs(e.t - this.today) < 1e-9) s += ' · today';
      return s;
    },

    // Everything upstream and downstream of the picked event, so its whole story lights up.
    chain: function (id) {
      var links = this.shownLinks(), lit = {}, seen = {};
      function walk(x, dir) { links.forEach(function (l) { var from = dir > 0 ? l[0] : l[1], to = dir > 0 ? l[1] : l[0]; if (from === x && !seen[to + dir]) { seen[to + dir] = 1; lit[l[0] + '>' + l[1]] = 1; walk(to, dir); } }); }
      if (id) { walk(id, 1); walk(id, -1); }
      return lit;
    },

    _labHTML: function (e) {
      var name = e.name.length > 32 ? e.name.slice(0, 31) + '…' : e.name, date = this.geo.dateText(e.t);
      return '<line class="stem" x1="8" x2="8"/><text class="t-name" x="8" y="0">' + esc(name) + '</text><text class="t-date" x="8" y="14">' + esc(date) + '</text>' +
        (e.dm ? '<text class="t-date t-dm" x="8" y="14" dx="' + (date.length * 6 + 8) + '">Director only</text>' : '');
    },

    _buildNodes: function () {
      var self = this, hit = coarse() ? 22 : 16;
      this.g.dots.innerHTML = ''; this.g.labs.innerHTML = ''; this.nodes = {};
      this.events.forEach(function (e, i) {
        var d = document.createElementNS(NS, 'g');
        d.setAttribute('class', 'ev'); d.setAttribute('data-id', e.id); d.setAttribute('tabindex', '0'); d.setAttribute('role', 'button');
        d.setAttribute('aria-label', e.name + ', ' + self.whenText(e) + (e.dm ? ', Director only' : ''));
        d.style.setProperty('--k', self.kindColor(e));
        d.innerHTML = '<circle class="ring" r="14.5"/><circle class="halo" r="12.5"/><circle class="core" r="5.5" style="animation-delay:' + (380 + Math.min(i, 24) * 45) + 'ms"/><circle class="hit" r="' + hit + '"/>';
        self.g.dots.appendChild(d);
        var lab = document.createElementNS(NS, 'g');
        lab.setAttribute('class', 'elab off'); lab.innerHTML = self._labHTML(e);
        self.g.labs.appendChild(lab);
        var n = Math.min(32, e.name.length), dl = self.geo.dateText(e.t).length + (e.dm ? 15 : 0);
        self.nodes[e.id] = { dot: d, lab: lab, w: Math.min(230, Math.max(n * 6.7, dl * 6, 110) + 16) };
      });
    },

    _buildArcs: function () {
      var self = this;
      this.g.arcs.innerHTML = ''; this.arcs = {};
      this.links.forEach(function (l) {
        var p = document.createElementNS(NS, 'path'), dm = self.byId[l[0]].dm || self.byId[l[1]].dm;
        // A dashed Director arc keeps its dashes, so it does not take the 0..1 draw-in length.
        if (!dm) p.setAttribute('pathLength', '1');
        p.setAttribute('class', 'arc' + (dm ? ' dm' : ''));
        self.g.arcs.appendChild(p); self.arcs[l[0] + '>' + l[1]] = p;
      });
    },

    _buildEras: function () {
      var nav = this.$('eras');
      if (!this.eras.length) { nav.hidden = true; return; }
      nav.hidden = false;
      nav.innerHTML = '<span class="tlr-lbl">Eras</span>' + this.eras.map(function (e, i) {
        return '<a href="#" data-era="' + i + '">' + esc(e.name) + '<small>' + e.a + (e.b === Infinity ? ' on' : ' to ' + (e.b - 1)) + '</small></a>';
      }).join('') + '<span class="tlr-erastep"><button type="button" class="tlr-btn ghost" data-erastep="-1">Earlier era</button><button type="button" class="tlr-btn ghost" data-erastep="1">Later era</button></span>';
      this.eraLinks = Array.prototype.slice.call(nav.querySelectorAll('[data-era]'));
    },

    _buildKinds: function () {
      var box = this.$('kinds'), self = this;
      if (this.kindOrder.length < 2) { box.hidden = true; return; }
      box.hidden = false;
      box.innerHTML = '<span class="tlr-lbl" style="margin-right:4px">Kinds</span>' + this.kindOrder.map(function (k) {
        return '<button type="button" class="tlr-kind" data-kind="' + esc(k) + '" style="--k:' + self.kinds[k].color + '" aria-pressed="' + !self.st.hidden[k] + '"><i></i>' + esc(k) + '</button>';
      }).join('');
    },

    _eraFit: function (i) {
      var e = this.eras[i]; if (!e) return;
      this.setTarget(Math.max(this.MIN, e.a), Math.min(this.MAX, e.b));
    },

    _ticks: function () {
      var g = this.geo, out = [], W = this.W, ppy = W / this.span(), t0 = this.view.t0, t1 = this.view.t1, y, m, d, k, i;
      var ysteps = [1000, 500, 250, 100, 50, 25, 10, 5, 2, 1], ystep = 0;
      for (i = 0; i < ysteps.length; i++) if (ysteps[i] * ppy >= 62) ystep = ysteps[i];
      var avgLen = g.yearLen(Math.floor(t0)), mw = ppy / g.mc, dw = ppy / avgLen;
      if (dw * 1 >= 30 || dw * 2 >= 30 || dw * 5 >= 30) {
        var dstep = dw >= 30 ? 1 : dw * 2 >= 30 ? 2 : 5;
        for (y = Math.floor(t0); y <= Math.ceil(t1); y++) for (m = 1; m <= g.mc; m++) for (d = 1; d <= g.monthDays(y, m); d += (d === 1 ? dstep - 1 || 1 : dstep)) {
          var td = g.t(y, m, d); if (td < t0 || td > t1) continue;
          out.push({ x: this.X(td), t: d === 1 ? g.monthName(m) : String(d), major: d === 1 });
        }
      } else if (ystep <= 1 && mw >= 34) {
        var mstep = mw >= 48 ? 1 : mw * 2 >= 48 ? 2 : mw * 3 >= 48 ? 3 : 6;
        for (y = Math.floor(t0); y <= Math.ceil(t1); y++) for (m = 1; m <= g.mc; m += mstep) {
          var tm = g.t(y, m, 1); if (tm < t0 || tm > t1) continue;
          out.push({ x: this.X(tm), t: m === 1 ? g.monthName(1) + ' ' + y : g.monthName(m), major: m === 1 });
        }
      } else {
        ystep = ystep || 1000;
        for (k = Math.ceil(t0 / ystep); k <= Math.floor(t1 / ystep); k++) out.push({ x: this.X(k * ystep), t: String(k * ystep), major: (k * ystep) % (ystep * 5) === 0 });
      }
      return out;
    },

    _placeLabels: function (evs, rowsMax) {
      var self = this, st = this.st, taken = [], r; for (r = 0; r < rowsMax; r++) taken.push([]);
      var out = {};
      function pr(e) { return (e.id === st.sel ? 1000 : 0) + (e.id === st.hover ? 900 : 0) + (self.matches(e) ? 800 : 0) + (e.page ? 10 : 0); }
      evs.slice().sort(function (a, b) { return pr(b) - pr(a); }).forEach(function (e) {
        var x = self.X(e.t), w = self.nodes[e.id].w, W = self.W;
        if (x < 0 || x > W) return;
        var l = x - 8; if (l + w > W - 4) l = x + 8 - w; if (l < 4) l = 4;
        function fits(r) { return taken[r].every(function (s) { return l + w + 10 < s[0] || l > s[1] + 10; }); }
        // A label keeps its row while it still fits, so labels do not hop as you pan.
        var order = []; if (self.rows[e.id] != null && self.rows[e.id] < rowsMax) order.push(self.rows[e.id]);
        for (var q = 0; q < rowsMax; q++) if (order.indexOf(q) < 0) order.push(q);
        for (var k = 0; k < order.length; k++) { if (fits(order[k])) { taken[order[k]].push([l, l + w]); out[e.id] = { row: order[k], l: l, x: x }; self.rows[e.id] = order[k]; return; } }
      });
      return out;
    },

    // Dots closer than a dot's width gather into one counted bubble; the open event never joins one.
    _cluster: function (evs) {
      var self = this, W = this.W, Y = this.Y, groups = [], cur = null, hit = coarse() ? 22 : 16;
      evs.filter(function (e) { var x = self.X(e.t); return x > -20 && x < W + 20; }).forEach(function (e) {
        var x = self.X(e.t);
        if (e.id === self.st.sel) { groups.push([e]); cur = null; return; }
        if (cur && x - self.X(cur[cur.length - 1].t) < 12 && x - self.X(cur[0].t) < 40) cur.push(e); else { cur = [e]; groups.push(cur); }
      });
      var inCl = {}, h = '';
      groups.forEach(function (g) {
        if (g.length < 2) return;
        var a = g[0].t, b = g[g.length - 1].t, cx = (self.X(a) + self.X(b)) / 2, n = g.length, r = n < 10 ? 9 : n < 100 ? 11 : 13;
        var dm = g.every(function (e) { return e.dm; });
        g.forEach(function (e) { inCl[e.id] = 1; });
        h += '<g class="cl" data-a="' + a + '" data-b="' + b + '" tabindex="0" role="button" aria-label="' + n + ' events, ' + esc(self.geo.dateText(a)) + ' to ' + esc(self.geo.dateText(b)) + '. Zoom in." transform="translate(' + cx.toFixed(1) + ',' + Y + ')">' +
          '<circle class="hit" r="' + hit + '"/><circle class="bub" r="' + r + '"' + (dm ? ' style="stroke:var(--tlr-dm);stroke-dasharray:3 2.4"' : '') + '/><text text-anchor="middle" dy="3.6">' + n + '</text></g>';
      });
      if (h !== this.lastCl) { this.g.cl.innerHTML = h; this.lastCl = h; }
      return inCl;
    },

    draw: function () {
      if (!this.geo) return;
      var self = this, st = this.st, W = this.W = this.stage.clientWidth || 800, H = this.H = this.stage.clientHeight || 380, axis = 50, h = '';
      var Y = this.Y = Math.round(H * 0.56);
      this.svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
      this.eras.forEach(function (e, i) {
        var l = self.X(e.a), r = e.b === Infinity ? W : self.X(e.b); if (r < 0 || l > W) return;
        var cl = Math.max(0, l), cr = Math.min(W, r);
        if (l > 0) h += '<line x1="' + l + '" x2="' + l + '" y1="0" y2="' + H + '" style="stroke:var(--tlr-line)"/>';
        if (cr - cl > e.name.length * 7.6 + 24) h += '<text class="t-era" data-era-i="' + i + '" x="' + (cl + 10) + '" y="20">' + esc(e.name) + '</text>';
      });
      this.g.back.innerHTML = h;
      h = '<line x1="0" x2="' + W + '" y1="' + axis + '" y2="' + axis + '" style="stroke:var(--tlr-line)"/>';
      this._ticks().forEach(function (t) {
        if (t.x < 14 || t.x > W - 14) return;
        h += '<line x1="' + t.x + '" x2="' + t.x + '" y1="' + (axis - 5) + '" y2="' + axis + '" style="stroke:var(--tlr-line-strong)"/>' +
          '<text class="t-tick" x="' + t.x + '" y="' + (axis - 10) + '" text-anchor="middle"' + (t.major ? ' style="fill:var(--tlr-body);font-weight:600"' : '') + '>' + esc(t.t) + '</text>';
      });
      var tx = this.today != null ? this.X(this.today) : null;
      if (tx != null && tx >= 0 && tx <= W) h += '<line x1="' + tx + '" x2="' + tx + '" y1="' + axis + '" y2="' + H + '" style="stroke:var(--tlr-acc);stroke-width:1.5;stroke-dasharray:2 4;opacity:.6"/>' +
        '<rect x="' + (tx - 24) + '" y="' + (axis + 4) + '" width="48" height="17" rx="4" style="fill:var(--tlr-bg);stroke:var(--tlr-acc-link)"/><text class="t-today" x="' + tx + '" y="' + (axis + 16) + '" text-anchor="middle">TODAY</text>';
      this.g.ticks.innerHTML = h;
      this.base.setAttribute('d', 'M0 ' + Y + ' H' + W);
      // The drawn-in stretch of river is the history up to today.
      this.fill.setAttribute('d', tx == null ? 'M0 ' + Y : 'M' + Math.max(-4, this.X(this.MIN)) + ' ' + Y + ' H' + Math.min(W + 4, tx));
      var vis = this.shown(), lit = this.chain(st.sel), inCl = this._cluster(vis), bars = '';
      vis.forEach(function (e) { if (e.end) bars += '<line x1="' + self.X(e.t) + '" x2="' + self.X(e.end) + '" y1="' + Y + '" y2="' + Y + '" style="stroke:' + self.kindColor(e) + ';stroke-width:8;stroke-linecap:round;opacity:.28"/>'; });
      this.g.bars.innerHTML = bars;
      var visIds = {}; vis.forEach(function (e) { if (!inCl[e.id]) visIds[e.id] = 1; });
      Object.keys(this.arcs).forEach(function (k) {
        var ids = k.split('>'), a = self.byId[ids[0]], b = self.byId[ids[1]], p = self.arcs[k];
        if (!visIds[a.id] || !visIds[b.id]) { p.style.display = 'none'; p.classList.remove('lit'); return; }
        p.style.display = '';
        // An end far off screen is pinned just past the edge, so the arc leaves the frame cleanly.
        var x1 = Math.max(-80, self.X(a.end || a.t)), x2 = Math.min(W + 80, self.X(b.t)), d = Math.min(Math.max(20, H - Y - 30), Math.max(18, Math.abs(x2 - x1) * 0.42));
        p.setAttribute('d', 'M' + x1 + ' ' + (Y + 7) + ' C' + x1 + ' ' + (Y + d * 1.25) + ' ' + x2 + ' ' + (Y + d * 1.25) + ' ' + x2 + ' ' + (Y + 7));
        var on = !!lit[k];
        if (on !== p.classList.contains('lit')) { p.classList.toggle('lit', on); if (on) { p.style.animation = 'none'; void p.getBoundingClientRect(); p.style.animation = ''; } }
      });
      var placed = this._placeLabels(vis.filter(function (e) { return visIds[e.id]; }), Math.max(1, Math.floor((Y - axis - 54) / 38)));
      this.events.forEach(function (e) {
        var n = self.nodes[e.id], x = self.X(e.t), on = !!visIds[e.id];
        n.dot.setAttribute('transform', 'translate(' + x.toFixed(1) + ',' + Y + ')');
        var cls = 'ev' + (on ? '' : ' gone') + (e.dm ? ' dm' : '') + (self.today != null && e.t > self.today + 1e-9 ? ' later' : '') +
          (e.id === st.sel ? ' on' : '') + (e.id === st.hover ? ' hov' : '') + (self.matches(e) ? ' hit' : '') + (n.dot.classList.contains('glow') ? ' glow' : '');
        if (n.dot.getAttribute('class') !== cls) n.dot.setAttribute('class', cls);
        n.dot.setAttribute('tabindex', on ? '0' : '-1');
        var pl = placed[e.id];
        if (pl && on) {
          var ly = Y - 34 - pl.row * 38, stem = n.lab.querySelector('.stem'), sx = pl.x - pl.l;
          n.lab.setAttribute('transform', 'translate(' + pl.l.toFixed(1) + ',' + ly + ')');
          stem.setAttribute('x1', sx); stem.setAttribute('x2', sx); stem.setAttribute('y1', 18); stem.setAttribute('y2', Y - ly - 9);
          n.lab.setAttribute('class', 'elab' + (e.id === st.sel ? ' on' : '') + (e.dm ? ' dmo' : ''));
        } else n.lab.setAttribute('class', 'elab off');
      });
      this._drawOverview(vis);
      var g = this.geo, p0 = g.parts(this.view.t0), p1 = g.parts(this.view.t1);
      this.$('range').textContent = this.span() < 1.2 ? g.monthName(p0.m) + ' ' + p0.y + ' to ' + g.monthName(p1.m) + ' ' + p1.y : Math.round(this.view.t0) + ' to ' + Math.round(this.view.t1);
      this._markEra();
      this._placePeek();
    },

    _drawOverview: function (evs) {
      var self = this, w = this.ov.clientWidth || this.W, MIN = this.MIN, MAX = this.MAX;
      function p(t) { return (t - MIN) / (MAX - MIN) * w; }
      var h = '<line x1="0" x2="' + w + '" y1="16" y2="16" style="stroke:var(--tlr-line-strong);stroke-width:1.5"/>';
      evs.forEach(function (e) { h += '<circle cx="' + p(e.t).toFixed(1) + '" cy="16" r="2.5" style="fill:' + (e.id === self.st.sel ? 'var(--tlr-acc)' : e.dm ? 'var(--tlr-dm)' : 'var(--tlr-muted)') + '"/>'; });
      var a = Math.max(0, p(this.view.t0)), b = Math.min(w, p(this.view.t1));
      h += '<rect x="' + Math.min(a, w - 6) + '" y="3" width="' + Math.max(6, b - a) + '" height="26" rx="6" style="fill:var(--tlr-acc-soft);stroke:var(--tlr-acc);stroke-width:1.5"/>';
      this.ovsvg.setAttribute('viewBox', '0 0 ' + w + ' 32'); this.ovsvg.innerHTML = h;
    },

    _markEra: function () {
      if (!this.eraLinks) return;
      var c = (this.view.t0 + this.view.t1) / 2, eras = this.eras;
      this.eraLinks.forEach(function (a, i) { var on = c >= eras[i].a && c < eras[i].b; if ((a.getAttribute('aria-current') === 'true') !== on) a.setAttribute('aria-current', String(on)); });
    },

    // Peek: a hover card after a short rest, so sweeping across the line stays quiet.
    _placePeek: function () {
      var self = this, st = this.st, id = st.hover && st.hover !== st.sel ? st.hover : null;
      if (id !== this._peekFor) {
        if (this._peekEl) this._peekEl.remove();
        this._peekEl = null; this._peekFor = id; this._peekReady = null; clearTimeout(this._peekTimer);
        if (id) this._peekTimer = setTimeout(function () { self._peekReady = id; self._placePeek(); }, motion() === 'off' ? 0 : 250);
      }
      if (!id || this._peekReady !== id || !this.byId[id]) return;
      var e = this.byId[id];
      if (!this._peekEl) {
        this._peekEl = document.createElement('div'); this._peekEl.className = 'tlr-peek';
        this._peekEl.innerHTML = '<b>' + esc(e.name) + '</b>' + esc(this.whenText(e)) + (e.dm ? ' · <span class="d">Director only</span>' : '');
        this.stage.appendChild(this._peekEl);
      }
      this._peekEl.style.left = Math.max(130, Math.min(this.W - 130, this.X(e.t))) + 'px'; this._peekEl.style.top = (this.Y - 4) + 'px';
    },

    // Motion: the view eases toward its target and coasts after a drag; Calm settles faster, Off jumps.
    _clamp: function () {
      var tg = this.target, full = this.MAX - this.MIN, sp = Math.max(1 / 400, Math.min(full * 1.4, tg.t1 - tg.t0)), mid = (tg.t0 + tg.t1) / 2, pad = full * 0.2;
      tg.t0 = mid - sp / 2; tg.t1 = mid + sp / 2;
      if (tg.t0 < this.MIN - pad) { tg.t1 += this.MIN - pad - tg.t0; tg.t0 = this.MIN - pad; }
      if (tg.t1 > this.MAX + pad) { tg.t0 -= tg.t1 - this.MAX - pad; tg.t1 = this.MAX + pad; }
    },
    _kick: function () {
      var self = this;
      this._clamp();
      if (!this.raf) { this._last = performance.now(); this.raf = requestAnimationFrame(function (n) { self._tick(n); }); }
    },
    _tick: function (now) {
      var self = this, dt = Math.min(48, now - this._last), m = motion(), v = this.view, tg = this.target;
      this._last = now; this.raf = null;
      if (this.vel && m === 'full') { var sh = this.vel * dt; tg.t0 += sh; tg.t1 += sh; this._clamp(); this.vel *= Math.pow(0.9, dt / 16); if (Math.abs(this.vel * (this.W / this.span())) < 0.02) this.vel = 0; } else this.vel = 0;
      var k = m === 'off' ? 1 : 1 - Math.pow(m === 'calm' ? 0.55 : 0.78, dt / 16);
      // Ease in log space for zoom, so zooming feels even at every scale.
      var sc = Math.log(v.t1 - v.t0), tsc = Math.log(tg.t1 - tg.t0), c = (v.t0 + v.t1) / 2, tc = (tg.t0 + tg.t1) / 2;
      var nsc = sc + (tsc - sc) * k, nc = c + (tc - c) * k, ns = Math.exp(nsc);
      v.t0 = nc - ns / 2; v.t1 = nc + ns / 2;
      var done = Math.abs(nsc - tsc) < 1e-4 && Math.abs(nc - tc) < (tg.t1 - tg.t0) * 1e-4 && !this.vel;
      if (done) { v.t0 = tg.t0; v.t1 = tg.t1; }
      this.draw();
      if (!done) this.raf = requestAnimationFrame(function (n) { self._tick(n); });
    },
    setTarget: function (t0, t1) { this.target.t0 = t0; this.target.t1 = t1; this._kick(); },
    zoomAt: function (px, f) {
      var tg = this.target, at = tg.t0 + px / this.W * (tg.t1 - tg.t0), sp = (tg.t1 - tg.t0) * f, k = (at - tg.t0) / (tg.t1 - tg.t0);
      this.setTarget(at - k * sp, at - k * sp + sp);
    },
    reveal: function (e) {
      var tg = this.target, sp = tg.t1 - tg.t0, x = (e.t - tg.t0) / sp;
      if (x > 0.12 && x < 0.88) return;
      this.setTarget(e.t - sp / 2, e.t + sp / 2);
    },

    _restart: function (el, cls) { el.classList.remove(cls); void el.getBoundingClientRect(); el.classList.add(cls); setTimeout(function () { el.classList.remove(cls); }, 1300); },
    _remember: function (what) { this.st.stack = this.st.stack.filter(function (x) { return x !== what; }); this.st.stack.push(what); },
    _forget: function (what) { this.st.stack = this.st.stack.filter(function (x) { return x !== what; }); },

    // The panel and the month close only on purpose: Close, Escape, or the
    // thing that opened them; Escape closes the last one opened.
    openEvent: function (id, opts) {
      opts = opts || {};
      var e = this.byId[id]; if (!e) return;
      this.st.sel = id; this.st.docked = true; this._remember('side');
      var p = this.geo.parts(e.t); this.st.month = { y: p.y, m: p.m };
      if (opts.fly) this.reveal(e);
      this.body.classList.add('docked');
      this.renderSide(); this.renderCal(); this.draw();
      if (opts.glow) { this._restart(this.nodes[id].dot, 'glow'); var t = this.side.querySelector('.t'); if (t) this._restart(t, 'glow'); }
    },
    closeSide: function () {
      var was = this.st.sel;
      this.st.docked = false; this.st.sel = null; this._forget('side');
      this.body.classList.remove('docked'); this.renderSide(); this.renderCal(); this.draw();
      if (was && this.nodes[was]) this.nodes[was].dot.focus({ preventScroll: true });
    },
    setFold: function (open) {
      if (!this.cal) return;
      this.st.fold = open;
      if (open) this._remember('fold'); else this._forget('fold');
      if (open && !this.st.month) this.st.month = this._foldMonth();
      this.renderCal();
    },
    closeLast: function () {
      var top = this.st.stack[this.st.stack.length - 1];
      if (top === 'fold') { this.setFold(false); var b = this.el.querySelector('[data-act="cal"]'); if (b) b.focus(); return true; }
      if (top === 'side') { this.closeSide(); return true; }
      return false;
    },
    step: function (dir) {
      var list = this.shown(), sel = this.st.sel, i = -1;
      for (var k = 0; k < list.length; k++) if (list[k].id === sel) i = k;
      var n = list[Math.max(0, Math.min(list.length - 1, i + dir))];
      if (n && n.id !== sel) this.openEvent(n.id, { fly: true });
    },

    renderSide: function () {
      var self = this, e = this.st.sel && this.byId[this.st.sel], side = this.side;
      if (!e) { side.innerHTML = ''; side.className = 'in'; return; }
      side.className = 'in' + (e.dm ? ' dmo' : '');
      var links = this.shownLinks();
      var from = links.filter(function (l) { return l[1] === e.id; }).map(function (l) { return self.byId[l[0]]; });
      var to = links.filter(function (l) { return l[0] === e.id; }).map(function (l) { return self.byId[l[1]]; });
      function rel(lbl, arr) {
        return arr.length ? '<div class="sub"><span class="tlr-lbl">' + lbl + '</span>' + arr.map(function (x) {
          return '<button type="button" class="tlr-lrow' + (x.dm ? ' dmo' : '') + '" data-go="' + esc(x.id) + '">' + esc(x.name) + '<small>' + esc(self.geo.dateText(x.t)) + '</small></button>';
        }).join('') + '</div>' : '';
      }
      var later = this.today != null && e.t > this.today + 1e-9;
      side.innerHTML = '<div class="top"><span class="tlr-lbl">' + esc(e.kind) + (e.dm ? '<span class="tlr-dmtag">Director only</span>' : '') + (later ? '<span class="tlr-uptag">Coming up</span>' : '') + '</span>' +
        '<button type="button" class="tlr-btn ghost sq" data-act="close" aria-label="Close">' + ICON_X + '</button></div>' +
        '<div class="t"><h2>' + esc(e.name) + '</h2></div><div class="when">' + esc(this.whenText(e)) + '</div>' +
        (e.desc ? '<p class="desc">' + esc(e.desc) + '</p>' : '') +
        (e.page ? '<div class="pages"><span class="tlr-lbl">Linked page</span><br><a href="/campaigns/' + encodeURIComponent(this.cfg.campaignId) + '/entities/' + encodeURIComponent(e.page.id) + '">' + esc(e.page.name) + '</a></div>' : '') +
        rel('Came from', from) + rel('Led to', to) +
        '<div class="tlr-keys"><span class="tlr-kbd">←</span> <span class="tlr-kbd">→</span> next event · <span class="tlr-kbd">Esc</span> closes</div>';
    },

    // The month under the line is the calendar page's own month grid, in its
    // own signed look (calendar-view.css), with that month's calendar events.
    // The month opens from a quiet line right above where it unfolds, naming
    // the month it will show.
    _foldMonth: function () { var p = this.geo.parts(this.today != null ? this.today : this.view.t0); return { y: p.y, m: p.m }; },

    renderCal: function () {
      var self = this, fold = this.$('fold'), box = this.$('cal'), link = this.$('monthlink');
      fold.classList.toggle('open', !!this.st.fold);
      link.hidden = !this.cal;
      if (this.cal) {
        var LM = this.st.month || this._foldMonth();
        link.textContent = this.st.fold ? 'Hide the month' : 'Show ' + this.geo.monthName(LM.m) + ' ' + LM.y + ' on the calendar';
        link.setAttribute('aria-expanded', String(!!this.st.fold));
      }
      if (!this.st.fold || !this.st.month || !this.cal) return;
      var M = this.st.month, key = M.y + '-' + M.m;
      this._calEvents = this._calEvents || {};
      if (!this._calEvents[key]) {
        this._calEvents[key] = 'loading';
        Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.cfg.campaignId) + '/calendars/' + encodeURIComponent(this.cfg.calendarId) + '/events?year=' + M.y + '&month=' + M.m)
          .then(function (r) { return r.ok ? r.json() : []; }).catch(function () { return []; })
          .then(function (list) { self._calEvents[key] = Array.isArray(list) ? list : []; if (self.st.month && self.st.month.y === M.y && self.st.month.m === M.m) self.renderCal(); });
      }
      var list = Array.isArray(this._calEvents[key]) ? this._calEvents[key] : [];
      box.innerHTML = this._calHead(M) + this._calGrid(M, list) +
        '<p class="tlr-calnote">This is the calendar\'s own month. Days, weather and moons are edited on the calendar page.</p>';
    },

    _calHref: function (y, m, d) { return '/campaigns/' + encodeURIComponent(this.cfg.campaignId) + '/calendars/' + encodeURIComponent(this.cfg.calendarId) + '/view?date=' + y + '-' + m + '-' + (d || 1); },

    _calHead: function (M) {
      return '<div class="tlr-cal-h"><button type="button" class="tlr-btn ghost sq" data-month="-1" aria-label="Previous month">' + ICON_L + '</button>' +
        '<button type="button" class="tlr-btn ghost sq" data-month="1" aria-label="Next month">' + ICON_R + '</button>' +
        '<h3>' + esc(this.geo.monthName(M.m)) + ' ' + M.y + '</h3><a href="' + this._calHref(M.y, M.m, 1) + '">Open the calendar</a>' +
        '<button type="button" class="tlr-btn" data-act="closecal">Close</button></div>';
    },

    // Which calendar events fall on a day, by the calendar page's own rules.
    _onDay: function (list, y, m, d) {
      var CD = Chronicle.calendarDate, cal = this.cal;
      return list.filter(function (e) {
        if (CD.hasExpansion && CD.hasExpansion(e)) return CD.occurrenceOn(e, y, m, d) !== null;
        if (e.year === y && e.month === m && e.day === d) return true;
        if (e.is_recurring && CD.occursOn && CD.occursOn(cal, e, y, m, d)) return true;
        if (!e.is_recurring && CD.withinSpan && CD.withinSpan(cal, e, y, m, d)) return true;
        return false;
      });
    },

    _calGrid: function (M, list) {
      var self = this, CD = Chronicle.calendarDate, cal = this.cal, days = this.geo.monthDays(M.y, M.m);
      var mdef = (cal.months || [])[M.m - 1] || {}, inter = !!mdef.is_intercalary;
      var cols = inter ? Math.min(days, 10) : CD.weekLen(cal), lead = inter ? 0 : CD.gridLead(cal, M.y, M.m);
      var tp = this.today != null ? this.geo.parts(this.today) : null, sel = this.st.sel && this.byId[this.st.sel], sp = sel ? this.geo.parts(sel.t) : null;
      var own = {}; this.shown().forEach(function (e) { var p = self.geo.parts(e.t); if (p.y === M.y && p.m === M.m) (own[p.d] = own[p.d] || []).push(e); });
      var colour = Chronicle.calendarColor || function () { return null; };
      var dow = inter ? '' : '<div class="dow">' + CD.gridWeekdays(cal).map(function (w) { return '<span>' + esc(w.name || '') + '</span>'; }).join('') + '</div>';
      var cells = [], i;
      for (i = 0; i < lead; i++) cells.push(null);
      for (i = 1; i <= days; i++) cells.push(i);
      while (cells.length % cols) cells.push(null);
      var h = '<div class="wk">';
      cells.forEach(function (d, k) {
        if (k && k % cols === 0) h += '</div><div class="wk">';
        if (!d) { h += '<div class="day"></div>'; return; }
        var isToday = tp && tp.y === M.y && tp.m === M.m && tp.d === d, past = tp && !isToday && self.geo.t(M.y, M.m, d) < self.today;
        var calEvs = self._onDay(list, M.y, M.m, d), mine = (own[d] || []).slice(), marks = [];
        calEvs.forEach(function (ce) {
          var tid = 'calendar:' + ce.id, c = colour(ce.color || ce.kind_color);
          mine = mine.filter(function (x) { return x.id !== tid; });
          marks.push('<span class="mk' + (ce.visibility === 'dm_only' ? ' dir' : '') + (tid === self.st.sel ? ' on' : '') + '"' + (self.byId[tid] ? ' data-go="' + esc(tid) + '"' : '') + (c ? ' style="color:' + c + '"' : '') + ' title="' + esc(ce.name) + '">' + esc(ce.icon || ce.kind_icon || '●') + '</span>');
        });
        mine.forEach(function (x) { marks.push('<span class="mk' + (x.dm ? ' dir' : '') + (x.id === self.st.sel ? ' on' : '') + '" data-go="' + esc(x.id) + '" style="color:' + self.kindColor(x) + '" title="' + esc(x.name) + '">●</span>'); });
        var extra = marks.length > 4 ? '<span class="more">+' + (marks.length - 4) + '</span>' : '';
        var go = (own[d] || [])[0];
        h += '<a class="day' + (isToday ? ' today' : '') + (past ? ' past' : '') + (sp && sp.y === M.y && sp.m === M.m && sp.d === d ? ' sel' : '') + '" href="' + self._calHref(M.y, M.m, d) + '"' + (go ? ' data-go="' + esc(go.id) + '"' : '') + '>' +
          '<div class="dc"><span class="num">' + d + '</span>' + (marks.length ? '<div class="marks">' + marks.slice(0, 4).join('') + extra + '</div>' : '') + '</div></a>';
      });
      h += '</div>';
      return '<div class="cal-v5"><div class="cal" style="--cols:' + cols + '">' + dow + '<div class="stage"><div class="month">' + h + '</div></div></div></div>';
    },

    _bind: function () {
      var self = this, stage = this.stage, ov = this.ov, ptrs = {}, drag = null, pinch = null, samples = [];
      function zoomRange(a, b) { var pad = Math.max((b - a) * 0.7, 1 / 60); self.setTarget(a - pad, b + pad); }

      // Pointer: drag pans (and coasts on release), two fingers pinch, a still click picks.
      stage.addEventListener('pointerdown', function (ev) {
        if (!self.geo) return;
        stage.setPointerCapture(ev.pointerId); ptrs[ev.pointerId] = { x: ev.clientX, y: ev.clientY }; self.vel = 0;
        var ids = Object.keys(ptrs);
        if (ids.length === 2) { var a = ptrs[ids[0]], b = ptrs[ids[1]]; pinch = { d: Math.hypot(a.x - b.x, a.y - b.y) }; drag = null; return; }
        var t = ev.target.closest && ev.target.closest('.ev'), c = ev.target.closest && ev.target.closest('.cl'), era = ev.target.getAttribute && ev.target.getAttribute('data-era-i');
        drag = { x: ev.clientX, t0: self.target.t0, t1: self.target.t1, moved: false, id: t ? t.getAttribute('data-id') : null, cl: c ? [+c.getAttribute('data-a'), +c.getAttribute('data-b')] : null, era: era != null ? +era : null };
        samples = [{ x: ev.clientX, t: performance.now() }];
      });
      stage.addEventListener('pointermove', function (ev) {
        if (!self.geo) return;
        if (ptrs[ev.pointerId]) ptrs[ev.pointerId] = { x: ev.clientX, y: ev.clientY };
        if (pinch) {
          var ids = Object.keys(ptrs); if (ids.length < 2) return;
          var a = ptrs[ids[0]], b = ptrs[ids[1]], d = Math.hypot(a.x - b.x, a.y - b.y), r = stage.getBoundingClientRect();
          self.zoomAt((a.x + b.x) / 2 - r.left, pinch.d / Math.max(1, d)); pinch.d = d; return;
        }
        if (drag) {
          var dx = ev.clientX - drag.x;
          if (Math.abs(dx) > 4 && !drag.moved) { drag.moved = true; stage.classList.add('dragging'); self.st.hover = null; }
          if (drag.moved) {
            var sh = dx / self.W * (drag.t1 - drag.t0);
            self.target.t0 = drag.t0 - sh; self.target.t1 = drag.t1 - sh; self._clamp();
            self.view.t0 = self.target.t0; self.view.t1 = self.target.t1; self.draw();
            samples.push({ x: ev.clientX, t: performance.now() }); if (samples.length > 6) samples.shift();
          }
          return;
        }
        var t = ev.target.closest && ev.target.closest('.ev'), id = t ? t.getAttribute('data-id') : null;
        if (id !== self.st.hover) { self.st.hover = id; self.draw(); }
      });
      function endPtr(ev) {
        delete ptrs[ev.pointerId];
        if (pinch) { if (Object.keys(ptrs).length < 2) pinch = null; return; }
        if (drag && drag.moved && samples.length > 1) {
          var a = samples[0], b = samples[samples.length - 1], dtm = b.t - a.t;
          if (dtm > 0 && performance.now() - b.t < 80) { self.vel = -((b.x - a.x) / dtm) / self.W * self.span(); self._kick(); }
        }
        if (drag && !drag.moved) {
          // Clicking the open event again is one of the three ways to close it.
          if (drag.id) { if (drag.id === self.st.sel && self.st.docked) self.closeSide(); else self.openEvent(drag.id); }
          else if (drag.cl) zoomRange(drag.cl[0], drag.cl[1]);
          else if (drag.era != null) self._eraFit(drag.era);
        }
        drag = null; stage.classList.remove('dragging');
      }
      stage.addEventListener('pointerup', endPtr);
      stage.addEventListener('pointercancel', endPtr);
      stage.addEventListener('pointerleave', function () { if (!drag && self.st.hover) { self.st.hover = null; self.draw(); } });
      stage.addEventListener('wheel', function (ev) {
        if (!self.geo) return;
        ev.preventDefault(); self.vel = 0;
        var r = stage.getBoundingClientRect(), tg = self.target;
        if (Math.abs(ev.deltaX) > Math.abs(ev.deltaY)) { var sh = ev.deltaX / self.W * (tg.t1 - tg.t0); self.setTarget(tg.t0 + sh, tg.t1 + sh); return; }
        var dy = ev.deltaMode === 1 ? ev.deltaY * 16 : ev.deltaY;
        self.zoomAt(ev.clientX - r.left, Math.exp(dy * (ev.ctrlKey ? 0.012 : 0.0022)));
      }, { passive: false });
      stage.addEventListener('keydown', function (ev) {
        if (!self.geo) return;
        var t = ev.target.closest && ev.target.closest('.ev'), c = ev.target.closest && ev.target.closest('.cl'), tg = self.target, sp = tg.t1 - tg.t0;
        if ((ev.key === 'Enter' || ev.key === ' ') && t) { ev.preventDefault(); var id = t.getAttribute('data-id'); if (id === self.st.sel && self.st.docked) self.closeSide(); else self.openEvent(id); return; }
        if ((ev.key === 'Enter' || ev.key === ' ') && c) { ev.preventDefault(); zoomRange(+c.getAttribute('data-a'), +c.getAttribute('data-b')); return; }
        if ((ev.key === 'ArrowLeft' || ev.key === 'ArrowRight') && !self.st.docked) { ev.preventDefault(); var dir = ev.key === 'ArrowLeft' ? -1 : 1; self.setTarget(tg.t0 + dir * sp * 0.2, tg.t1 + dir * sp * 0.2); }
        if (ev.key === '+' || ev.key === '=') self.zoomAt(self.W / 2, 0.6);
        if (ev.key === '-') self.zoomAt(self.W / 2, 1.6);
      });
      stage.addEventListener('focusin', function (ev) { var t = ev.target.closest && ev.target.closest('.ev'); if (t) { self.st.hover = t.getAttribute('data-id'); self.draw(); } });
      stage.addEventListener('focusout', function () { if (self.st.hover) { self.st.hover = null; self.draw(); } });

      var ovDrag = false;
      function ovMove(ev) { var r = ov.getBoundingClientRect(), t = self.MIN + (ev.clientX - r.left) / r.width * (self.MAX - self.MIN), sp = self.target.t1 - self.target.t0; self.setTarget(t - sp / 2, t + sp / 2); }
      ov.addEventListener('pointerdown', function (ev) { if (!self.geo) return; ov.setPointerCapture(ev.pointerId); ovDrag = true; self.vel = 0; ovMove(ev); });
      ov.addEventListener('pointermove', function (ev) { if (ovDrag) ovMove(ev); });
      ov.addEventListener('pointerup', function () { ovDrag = false; });

      this.el.addEventListener('click', function (ev) {
        if (!self.geo) return;
        var t = ev.target.closest('button, a'); if (!t || !self.el.contains(t)) return;
        var d = t.dataset;
        if (d.era != null) { ev.preventDefault(); self._eraFit(+d.era); return; }
        if (d.go) {
          // A mark inside a day link opens its event here rather than following the link.
          var mk = ev.target.closest('[data-go]');
          ev.preventDefault(); self.openEvent(mk ? mk.getAttribute('data-go') : d.go, { fly: true, glow: true }); return;
        }
        if (d.erastep) {
          var c0 = (self.target.t0 + self.target.t1) / 2, ci = -1;
          self.eras.forEach(function (x, i) { if (c0 >= x.a && c0 < x.b) ci = i; });
          if (ci < 0) ci = c0 < (self.eras[0] || {}).a ? -1 : self.eras.length;
          self._eraFit(Math.max(0, Math.min(self.eras.length - 1, ci + (+d.erastep)))); return;
        }
        if (d.kind != null) {
          var k = d.kind; self.st.hidden[k] = !self.st.hidden[k]; t.setAttribute('aria-pressed', String(!self.st.hidden[k]));
          if (self.st.sel && self.st.hidden[self.byId[self.st.sel].kind]) self.closeSide();
          self.renderCal(); self.draw(); return;
        }
        if (d.act === 'close') { self.closeSide(); return; }
        if (d.act === 'cal') { self.setFold(!self.st.fold); return; }
        if (d.act === 'closecal') { self.setFold(false); var cb = self.el.querySelector('[data-act="cal"]'); if (cb) cb.focus(); return; }
        if (d.month) {
          var m = self.st.month.m + (+d.month), y = self.st.month.y;
          if (m < 1) { m = self.geo.mc; y--; } if (m > self.geo.mc) { m = 1; y++; }
          self.st.month = { y: y, m: m }; self.renderCal(); return;
        }
        if (d.fit === 'all') { self.setTarget(self.MIN, self.MAX); return; }
        if (d.fit === 'now') { var c = self.today != null ? self.today : (self.MIN + self.MAX) / 2, w = 1 / self.geo.mc; self.setTarget(c - w, c + w); return; }
        if (d.zoom) { self.zoomAt(self.W / 2, +d.zoom); return; }
      });
      var q = this.$('q');
      q.addEventListener('input', function () { self.st.q = q.value; self.draw(); });
      q.addEventListener('keydown', function (ev) {
        if (ev.key !== 'Enter' || !self.geo) return;
        var hit = self.shown().filter(function (e) { return self.matches(e); }).pop();
        if (hit) self.openEvent(hit.id, { fly: true, glow: true });
      });

      // Page keys: "/" finds, Escape closes the last thing opened, arrows step while an event is open.
      this._onKey = function (ev) {
        if (!self.geo || !self.el.isConnected) return;
        var tg = ev.target, typing = tg && (tg.tagName === 'INPUT' || tg.tagName === 'TEXTAREA' || tg.tagName === 'SELECT' || tg.isContentEditable);
        if (ev.key === 'Escape') {
          if (tg === q && q.value) return;
          if (self.closeLast()) ev.preventDefault();
          return;
        }
        if (typing || ev.metaKey || ev.ctrlKey || ev.altKey) return;
        if (ev.key === '/') { ev.preventDefault(); q.focus(); return; }
        if ((ev.key === 'ArrowLeft' || ev.key === 'ArrowRight') && self.st.docked && self.el.contains(tg)) {
          ev.preventDefault(); self.step(ev.key === 'ArrowLeft' ? -1 : 1);
          var n = self.nodes[self.st.sel]; if (n && stage.contains(tg)) n.dot.focus({ preventScroll: true });
        }
      };
      document.addEventListener('keydown', this._onKey);
      if (window.ResizeObserver) { this._ro = new ResizeObserver(function () { self.draw(); }); this._ro.observe(stage); }
    }
  };

  var mounts = new WeakMap();
  var _impl = {
    init: function (el, config) { var r = Object.create(River); mounts.set(el, r); r.init(el, config); },
    destroy: function (el) { var r = mounts.get(el); if (r) { r.destroy(); mounts.delete(el); } }
  };

  function _register() { Chronicle.register('timeline-river', _impl); }
  if (window.Chronicle && Chronicle.register) _register();
  else document.addEventListener('DOMContentLoaded', _register);
})();
