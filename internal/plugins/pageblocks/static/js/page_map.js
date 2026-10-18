/**
 * page_map.js: the Map page block, a small hand-drawn map on a page.
 *
 * Read mode shows only the map: pointing at or tabbing to a marker opens the
 * one hover card (Chronicle.hovercard), a page's card through
 * entity_tooltip.js or a few words of the marker's own, and a click on the
 * map opens it bigger. A Director edits it in place: the pen swaps the
 * heading row for the tools, a marker list sits beside the map and a
 * settings section of fixed height sits under it, so selecting never moves
 * the page. A held shape or marker rides on a transform and lands on the grid
 * when let go; nothing else redraws while it is held.
 *
 * The content comes from data-endpoint, so a player never receives a DM-only
 * marker. User text is escaped before it goes into any markup. The pure
 * helpers are exposed as window.PageMapHelpers for the node tests.
 *
 * Styles live in static/css/page_map.css, scoped under .pm.
 */
(function () {
  'use strict';

  // The grid is fixed: every map is 40 by 26 cells, drawn 10 units a cell.
  var GW = 40, GH = 26, U = 10, VW = GW * U, VH = GH * U;
  // The eight named colours, in the order of the --map-c-0..7 tokens.
  var COLORS = ['sky', 'teal', 'orchid', 'rose', 'green', 'violet', 'cyan', 'lime'];
  var BGS = ['none', 'dots', 'grid', 'hex'];
  var BG_NAMES = { none: 'None', dots: 'Dots', grid: 'Grid', hex: 'Hexes' };
  var ICONS = ['location-dot', 'house', 'chess-rook', 'anchor', 'church', 'door-open', 'skull', 'gem', 'star', 'eye', 'fire', 'crown'];
  var ICON_NAMES = {
    'location-dot': 'Place', house: 'House', 'chess-rook': 'Tower', anchor: 'Harbor', church: 'Temple',
    'door-open': 'Door', skull: 'Danger', gem: 'Treasure', star: 'Quest', eye: 'Secret', fire: 'Camp', crown: 'Ruler'
  };
  // Font Awesome 6 solid code points, so a marker's icon can be an SVG
  // <text> in the icon font the page already loads (the timeline does the same).
  var ICON_GLYPH = {
    'location-dot': '', house: '', 'chess-rook': '', anchor: '', church: '',
    'door-open': '', skull: '', gem: '', star: '', eye: '', fire: '', crown: ''
  };
  var RECTS = ['room', 'area', 'water', 'danger'];
  var LINES = ['wall', 'path'];
  var KINDS = RECTS.concat(LINES, ['label']);
  // Drawing order: grounds first, then rooms, then lines and words on top.
  var ORDER = { area: 0, water: 1, danger: 1, room: 2, path: 3, wall: 4, label: 5 };
  // The server's limits, mirrored so the editor stops short of a refusal.
  var LIM = { shapes: 200, pins: 200, name: 80, text: 500, sketch: 20000, search: 2, shown: 20 };

  // ---------- Pure helpers ----------
  function uid(prefix) { return prefix + '_' + Math.random().toString(36).slice(2, 10); }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }
  function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }
  function snap(v, step) { return Math.round(v / step) * step; }
  function num(v, d) { v = Number(v); return isFinite(v) ? v : d; }
  function str(v) { return typeof v === 'string' ? v : (v == null ? '' : String(v)); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function isHex(c) { return /^#[0-9a-f]{6}$/i.test(str(c)); }
  // amberish mirrors the server's refusal of a saturated colour in the
  // amber hue band: amber is the Director's mark, so a marker that wore it
  // would read as hidden from players.
  function amberish(hex) {
    if (!isHex(hex)) return false;
    var r = parseInt(hex.slice(1, 3), 16) / 255, g = parseInt(hex.slice(3, 5), 16) / 255, b = parseInt(hex.slice(5, 7), 16) / 255;
    var mx = Math.max(r, g, b), mn = Math.min(r, g, b);
    if (mx === 0 || mx === mn) return false;
    var sat = (mx - mn) / mx, hue;
    if (mx === r) hue = ((g - b) / (mx - mn)) % 6;
    else if (mx === g) hue = (b - r) / (mx - mn) + 2;
    else hue = (r - g) / (mx - mn) + 4;
    hue *= 60;
    if (hue < 0) hue += 360;
    return hue >= 30 && hue <= 50 && sat >= 0.35 && mx >= 0.25;
  }
  // validColor keeps the three kinds a colour may be: "" (the theme's own),
  // one of the eight names, or #rrggbb outside amber. Anything else falls
  // back to the theme.
  function validColor(c) {
    c = str(c).trim().toLowerCase();
    return COLORS.indexOf(c) >= 0 || (isHex(c) && !amberish(c)) ? c : '';
  }
  // colorVal is what goes into a style attribute: a token for a name, the
  // hex itself, or "" for the theme's colour. Only validated values pass.
  function colorVal(c) {
    c = validColor(c);
    if (!c) return '';
    var i = COLORS.indexOf(c);
    return i >= 0 ? 'var(--map-c-' + i + ')' : c;
  }
  // inkFor says whether a marker's glyph needs dark or light ink on a colour
  // the DM picked freely; named colours and the theme carry their own ink.
  function inkFor(c) {
    c = validColor(c);
    if (!isHex(c)) return '';
    var r = parseInt(c.slice(1, 3), 16) / 255, g = parseInt(c.slice(3, 5), 16) / 255, b = parseInt(c.slice(5, 7), 16) / 255;
    var lin = function (v) { return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); };
    var L = 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
    return L > 0.4 ? 'dark' : 'light';
  }

  function isPin(o) { return !!o && !o.t; }
  function baseX(o) { return o.pts ? o.pts[0][0] : o.x; }
  function baseY(o) { return o.pts ? o.pts[0][1] : o.y; }

  // limits is how far a thing may travel before any part of it leaves the grid.
  function limits(o) {
    if (o.pts) {
      var xs = o.pts.map(function (p) { return p[0]; }), ys = o.pts.map(function (p) { return p[1]; });
      return { x0: -Math.min.apply(0, xs), x1: GW - Math.max.apply(0, xs), y0: -Math.min.apply(0, ys), y1: GH - Math.max.apply(0, ys) };
    }
    if (isPin(o)) return { x0: 0.5 - o.x, x1: GW - 0.5 - o.x, y0: 0.5 - o.y, y1: GH - 0.5 - o.y };
    if (o.t === 'label') return { x0: -o.x, x1: GW - 2 - o.x, y0: 1 - o.y, y1: GH - o.y };
    return { x0: -o.x, x1: GW - o.w - o.x, y0: -o.y, y1: GH - o.h - o.y };
  }

  // settle is where a move lands: on the nearest grid point (half a cell for
  // markers), never past the grid's edge.
  function settle(o, dx, dy) {
    var L = limits(o), step = isPin(o) ? 0.5 : 1;
    return {
      dx: clamp(snap(dx + baseX(o), step) - baseX(o), L.x0, L.x1),
      dy: clamp(snap(dy + baseY(o), step) - baseY(o), L.y0, L.y1)
    };
  }
  function moveBy(o, dx, dy) {
    if (o.pts) o.pts = o.pts.map(function (p) { return [p[0] + dx, p[1] + dy]; });
    else { o.x += dx; o.y += dy; }
    return o;
  }

  // bbox is a shape's outline in drawing units, for the selection frame.
  function bbox(s) {
    if (s.pts) {
      var xs = s.pts.map(function (p) { return p[0] * U; }), ys = s.pts.map(function (p) { return p[1] * U; });
      var x = Math.min.apply(0, xs), y = Math.min.apply(0, ys);
      return { x: x, y: y, w: Math.max.apply(0, xs) - x, h: Math.max.apply(0, ys) - y };
    }
    if (s.t === 'label') return { x: s.x * U, y: s.y * U - 12, w: Math.max(30, str(s.name).length * 6.2), h: 15 };
    return { x: s.x * U, y: s.y * U, w: s.w * U, h: s.h * U };
  }

  // visiblePins is what this viewer gets to see on the map.
  function visiblePins(pins, dm) {
    return (pins || []).filter(function (p) { return !p.hidden && (dm || !p.dm); });
  }
  // pinNumbers counts the numbered markers. Markers players see come first,
  // so a player's 1, 2, 3 are the Director's 1, 2, 3; DM-only ones follow.
  function pinNumbers(pins, dm) {
    var out = {}, n = 0;
    var vis = visiblePins(pins, dm).filter(function (p) { return p.style === 'number'; });
    vis.forEach(function (p) { if (!p.dm) out[p.id] = ++n; });
    vis.forEach(function (p) { if (p.dm) out[p.id] = ++n; });
    return out;
  }

  function isBlank(doc) { return !doc || (!doc.shapes.length && !doc.pins.length); }

  function emptyDoc() { return { grid: { w: GW, h: GH }, background: 'none', shapes: [], pins: [] }; }

  function point(p) {
    if (!p || p.length < 2) return null;
    var x = num(p[0], NaN), y = num(p[1], NaN);
    return isFinite(x) && isFinite(y) ? [clamp(x, 0, GW), clamp(y, 0, GH)] : null;
  }

  // normalise fills what a short or older document lacks and drops what
  // cannot be drawn, so drawing never meets an undefined list.
  function normalise(d) {
    d = d && typeof d === 'object' ? d : {};
    var out = emptyDoc();
    out.background = BGS.indexOf(d.background) >= 0 ? d.background : 'none';
    (Array.isArray(d.shapes) ? d.shapes : []).forEach(function (s) {
      if (!s || KINDS.indexOf(s.t) < 0) return;
      var o = { id: str(s.id) || uid('s'), t: s.t, name: str(s.name), color: validColor(s.color) };
      if (LINES.indexOf(s.t) >= 0) {
        var pts = (Array.isArray(s.pts) ? s.pts : []).map(point).filter(Boolean);
        if (pts.length < 2) return;
        o.pts = pts;
      } else if (s.t === 'label') {
        o.x = clamp(num(s.x, 0), 0, GW); o.y = clamp(num(s.y, 1), 0, GH);
      } else {
        o.x = clamp(num(s.x, 0), 0, GW); o.y = clamp(num(s.y, 0), 0, GH);
        o.w = clamp(num(s.w, 1), 1, GW - o.x || 1); o.h = clamp(num(s.h, 1), 1, GH - o.y || 1);
      }
      out.shapes.push(o);
    });
    (Array.isArray(d.pins) ? d.pins : []).forEach(function (p) {
      if (!p) return;
      var o = {
        id: str(p.id) || uid('p'),
        x: clamp(num(p.x, 0.5), 0.5, GW - 0.5), y: clamp(num(p.y, 0.5), 0.5, GH - 0.5),
        name: str(p.name), style: p.style === 'icon' ? 'icon' : 'number',
        icon: ICONS.indexOf(p.icon) >= 0 ? p.icon : 'location-dot',
        entityId: str(p.entityId), page: str(p.page), text: str(p.text),
        dm: !!p.dm, color: validColor(p.color)
      };
      if (p.entityName) o.entityName = str(p.entityName);
      if (p.entityUrl) o.entityUrl = str(p.entityUrl);
      if (p.hidden) o.hidden = true;
      out.pins.push(o);
    });
    return out;
  }

  // toWire is the document as the server takes it: the page's name and
  // address are the server's own and are not sent back, and a label whose
  // words are being retyped waits until it has some.
  function toWire(doc) {
    var d = clone(doc);
    d.grid = { w: GW, h: GH };
    d.shapes = d.shapes.filter(function (s) { return s.t !== 'label' || str(s.name).trim(); });
    d.pins.forEach(function (p) { delete p.entityName; delete p.entityUrl; delete p.hidden; });
    return d;
  }

  // drawn is the shape a drag from (sx,sy) to (ex,ey) makes with a tool,
  // before it lands on the grid.
  function drawn(tool, sx, sy, ex, ey) {
    ex = clamp(ex, 0, GW); ey = clamp(ey, 0, GH);
    if (LINES.indexOf(tool) >= 0) return { id: 'tmp', t: tool, pts: [[sx, sy], [Math.round(ex), Math.round(ey)]], name: '', color: '' };
    return { id: 'tmp', t: tool, x: Math.min(sx, ex), y: Math.min(sy, ey), w: Math.abs(ex - sx), h: Math.abs(ey - sy), name: '', color: '' };
  }
  // land snaps a drawn shape to whole cells; a shape with no size is no shape.
  function land(s) {
    if (!s) return null;
    s = clone(s);
    if (s.pts) {
      s.pts = s.pts.map(function (p) { return [Math.round(p[0]), Math.round(p[1])]; });
      return s.pts[0][0] !== s.pts[1][0] || s.pts[0][1] !== s.pts[1][1] ? s : null;
    }
    var x1 = Math.round(s.x + s.w), y1 = Math.round(s.y + s.h);
    s.x = Math.round(s.x); s.y = Math.round(s.y); s.w = x1 - s.x; s.h = y1 - s.y;
    return s.w >= 1 && s.h >= 1 ? s : null;
  }

  // previewURL is the address the shared hover card reads for a page.
  function previewURL(campaignId, entityId) {
    if (!campaignId || !entityId) return '';
    return '/campaigns/' + encodeURIComponent(campaignId) + '/entities/' + encodeURIComponent(entityId) + '/preview';
  }
  // campaignOf reads the campaign from the block's endpoint when the mount
  // does not name it.
  function campaignOf(endpoint) {
    var m = /\/campaigns\/([^/]+)\//.exec(str(endpoint));
    return m ? decodeURIComponent(m[1]) : '';
  }
  // highlight splits a name around what was typed, for the picker's <mark>.
  function highlight(name, q) {
    name = str(name); q = str(q).trim().toLowerCase();
    var i = q ? name.toLowerCase().indexOf(q) : -1;
    if (i < 0) return [name, '', ''];
    return [name.slice(0, i), name.slice(i, i + q.length), name.slice(i + q.length)];
  }

  var H = {
    GW: GW, GH: GH, U: U, COLORS: COLORS, BGS: BGS, ICONS: ICONS, ICON_GLYPH: ICON_GLYPH, KINDS: KINDS, LIM: LIM,
    clamp: clamp, snap: snap, amberish: amberish, validColor: validColor, colorVal: colorVal, inkFor: inkFor, limits: limits,
    settle: settle, moveBy: moveBy, bbox: bbox, visiblePins: visiblePins, pinNumbers: pinNumbers,
    isBlank: isBlank, emptyDoc: emptyDoc, normalise: normalise, toWire: toWire, drawn: drawn, land: land,
    previewURL: previewURL, campaignOf: campaignOf, highlight: highlight, esc: esc
  };
  if (typeof window !== 'undefined') window.PageMapHelpers = H;
  if (typeof Chronicle === 'undefined' || !Chronicle.register) return;

  // ---------- Server ----------
  function api(S, method, path, body, extra) {
    var url = S.cfg.endpoint + (path || '') + (method === 'GET' && !path ? '?kind=' + encodeURIComponent(S.kind) : '');
    var opts = { method: method, body: body, csrfToken: S.cfg.csrfToken };
    if (extra && extra.keepalive) opts.keepalive = true;
    return Chronicle.apiFetch(url, opts).then(function (res) {
      return res.text().then(function (txt) {
        var data = null;
        try { data = txt ? JSON.parse(txt) : null; } catch (e) { data = null; }
        if (!res.ok) {
          var err = new Error((data && (data.message || data.error)) || 'Something went wrong. Try again.');
          err.status = res.status;
          throw err;
        }
        return data;
      });
    });
  }

  // calm is true when the device or the campaign asks for less motion; a
  // held thing then lands at once instead of settling.
  function calm() {
    var root = document.documentElement;
    if (root.hasAttribute('data-motion') || root.hasAttribute('data-cz-reduce')) return true;
    return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }
  // still is true when nothing may move at all: Off, or the device asks for
  // reduced motion. Calm still fades.
  function still() {
    if (document.documentElement.getAttribute('data-motion') === 'off') return true;
    return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }
  function typingIn(t) {
    return !!(t && t.closest && (t.closest('input, textarea, select, [contenteditable="true"]') || t.isContentEditable));
  }

  var count = 0;

  Chronicle.register('page_map', {
    init: function (el, cfg) {
      var S = {
        el: el, cfg: cfg, kind: cfg.kind || 'page_map', id: 'pm' + (++count),
        campaignId: str(cfg.campaignId) || campaignOf(cfg.endpoint),
        doc: emptyDoc(), version: 0, canEdit: false, loaded: false,
        seq: 0, dirty: false, saveT: 0, saving: Promise.resolve(), status: '', statusErr: false,
        show: {}, stash: {}, hot: null, drag: null, pop: null, insts: [], unbind: [], dead: false
      };
      el.classList.add('pm');
      el._pm = S;
      S.small = new Inst(S, el, false);
      S.onKey = function (ev) { onKey(S, ev); };
      document.addEventListener('keydown', S.onKey);
      if (Chronicle.hovercard) S.unbind.push(Chronicle.hovercard.bind(el, '[data-pin-text]', function (t) { return noteCard(S, t); }));
      load(S);
    },
    destroy: function (el) {
      var S = el._pm; if (!S) return;
      if (S.dirty) flush(S, true);
      S.dead = true;
      clearTimeout(S.saveT);
      document.removeEventListener('keydown', S.onKey);
      S.unbind.forEach(function (u) { u(); });
      if (S.pop && S.pop.root.parentNode) S.pop.root.parentNode.removeChild(S.pop.root);
      if (Chronicle.markClean) Chronicle.markClean(S.id);
      el._pm = null;
    }
  });

  function load(S) {
    api(S, 'GET').then(function (v) {
      take(S, v);
      S.loaded = true;
      S.small.frame();
    }).catch(function () {
      // A block that cannot load is left out; the rest of the page is fine.
      S.el.textContent = '';
    });
  }

  function take(S, v) {
    v = v || {};
    S.doc = normalise(v.data);
    S.version = v.version || 0;
    S.canEdit = !!v.canEdit;
    S.show = {}; S.stash = {};
  }

  function setStatus(S, msg, isErr) {
    S.status = msg; S.statusErr = !!isErr;
    S.insts.forEach(function (i) {
      var line = i.el.querySelector('.pm-status');
      if (line) { line.textContent = msg; line.classList.toggle('err', !!isErr); }
    });
  }

  // ---------- Saving ----------
  // Every change waits a moment and then the whole map saves at once; saves
  // go one at a time so each carries the version the last one returned.
  function changed(S) {
    S.seq++;
    S.dirty = true;
    if (Chronicle.markDirty) Chronicle.markDirty(S.id);
    if (S.statusErr) setStatus(S, '');
    clearTimeout(S.saveT);
    S.saveT = setTimeout(function () { flush(S); }, 700);
  }

  function flush(S, leaving) {
    clearTimeout(S.saveT); S.saveT = 0;
    if (!S.dirty) return S.saving;
    S.dirty = false;
    var run = function () {
      if (S.dead && !leaving) return null;
      var seq = S.seq;
      setStatus(S, 'Saving…');
      return api(S, 'PUT', '', { kind: S.kind, data: toWire(S.doc), version: S.version }, { keepalive: leaving }).then(function (v) {
        if (S.dead) return;
        S.version = (v && v.version) || S.version;
        merge(S, v && v.data);
        if (seq === S.seq && !S.dirty) {
          if (Chronicle.markClean) Chronicle.markClean(S.id);
          setStatus(S, 'Saved');
          setTimeout(function () { if (!S.dead && S.status === 'Saved') setStatus(S, ''); }, 2000);
        }
      }, function (err) { if (!S.dead) saveFailed(S, err); });
    };
    S.saving = S.saving.then(run, run);
    return S.saving;
  }

  // merge takes what only the server knows (a linked page's name and
  // address) without redrawing anything, so a save never disturbs a drag.
  function merge(S, data) {
    if (!data || !Array.isArray(data.pins)) return;
    var byId = {};
    data.pins.forEach(function (p) { if (p && p.id) byId[p.id] = p; });
    S.doc.pins.forEach(function (p) {
      var s = byId[p.id];
      if (!s || str(s.entityId) !== p.entityId) return;
      if (s.entityName) p.entityName = str(s.entityName);
      if (s.entityUrl) p.entityUrl = str(s.entityUrl);
    });
  }

  function saveFailed(S, err) {
    if (err.status === 409) {
      var msg = 'Someone else changed this map. Showing their version.';
      return api(S, 'GET').then(function (v) {
        take(S, v);
        S.dirty = false; S.drag = null;
        if (Chronicle.markClean) Chronicle.markClean(S.id);
        S.insts.forEach(function (i) { i.sel = null; i.drape = null; i.temp = null; });
        frameAll(S);
        setStatus(S, msg, true);
      }).catch(function () { setStatus(S, msg, true); });
    }
    // Kept as unsaved, so leaving the page still warns; the next change tries again.
    S.dirty = true;
    setStatus(S, 'Not saved: ' + err.message, true);
    return null;
  }

  // ---------- One map view ----------
  // The page holds the small view; Bigger mounts a second view of the same
  // map in a dialog. Both draw from S.doc.
  function Inst(S, el, big) {
    this.S = S; this.el = el; this.big = big;
    this.editing = false; this.tool = 'select'; this.marker = 'number'; this.icon = 'location-dot';
    this.sel = null; this.drape = null; this.temp = null; this.svg = null; this.layer = null;
    this.key = S.id + (big ? 'b' : 'a');
    this.pq = { i: 0, list: [], total: 0, q: '', timer: 0, token: 0 };
    S.insts.push(this);
    wire(this);
  }

  function frameAll(S) { S.insts.forEach(function (i) { if (i.live()) i.frame(); }); }
  function paintAll(S) { S.insts.forEach(function (i) { if (i.live() && i.layer) i.paint(); }); }

  Inst.prototype.live = function () {
    return this.el.isConnected && (!this.big || !!(this.S.pop && !this.S.pop.root.hidden));
  };
  Inst.prototype.find = function (id) {
    var d = this.S.doc;
    return d.pins.filter(function (q) { return q.id === id; })[0] || d.shapes.filter(function (q) { return q.id === id; })[0] || null;
  };
  Inst.prototype.selected = function () { return this.sel ? this.find(this.sel) : null; };

  function toolBtn(t, id, icon, label) {
    return '<button type="button" class="pm-icon" data-tool="' + id + '" aria-pressed="' + (t.tool === id) + '" aria-label="' + label + '" title="' + label + '"><i class="fa-solid fa-' + icon + '" aria-hidden="true"></i></button>';
  }

  function headHTML(t) {
    var S = t.S;
    if (t.editing) {
      return '<div class="pm-head pm-tools" role="toolbar" aria-label="Map tools">' +
        toolBtn(t, 'select', 'arrow-pointer', 'Select and move') + '<span class="pm-tsep"></span>' +
        toolBtn(t, 'room', 'square', 'Room') + toolBtn(t, 'area', 'vector-square', 'Area') + toolBtn(t, 'water', 'water', 'Water') +
        toolBtn(t, 'danger', 'triangle-exclamation', 'Danger zone') + toolBtn(t, 'wall', 'grip-lines', 'Wall') +
        toolBtn(t, 'path', 'route', 'Path') + toolBtn(t, 'label', 'font', 'Label') + '<span class="pm-tsep"></span>' +
        '<button type="button" class="pm-icon" data-mk="number" aria-pressed="' + (t.tool === 'pin' && t.marker === 'number') + '" aria-label="Numbered marker" title="Numbered marker"><span class="pm-one" aria-hidden="true">1</span></button>' +
        '<button type="button" class="pm-icon" data-mk="' + t.icon + '" aria-pressed="' + (t.tool === 'pin' && t.marker === 'icon') + '" aria-label="Icon marker" title="Icon marker (pick its icon after placing)"><i class="fa-solid fa-' + t.icon + '" aria-hidden="true"></i></button>' +
        '<span class="pm-sp"></span><span class="pm-status' + (S.statusErr ? ' err' : '') + '" role="status" aria-live="polite">' + esc(S.status) + '</span>' +
        '<button type="button" class="pm-quiet" data-act="text">From text</button>' +
        (t.big ? '' : '<button type="button" class="pm-icon" data-act="done" aria-label="Done editing the map" title="Done"><i class="fa-solid fa-check" aria-hidden="true"></i></button>') +
        '</div>';
    }
    if (t.big) return '';
    return '<div class="pm-head"><h3 class="cb-head pm-title">Map</h3><span class="pm-sp"></span>' +
      (S.canEdit ? '<button type="button" class="pm-icon" data-act="pen" aria-label="Edit the map" title="Edit the map"><i class="fa-solid fa-pen" aria-hidden="true"></i></button>' : '') +
      '<button type="button" class="pm-icon" data-act="bigger" aria-label="Open the map bigger" title="Bigger"><i class="fa-solid fa-up-right-and-down-left-from-center" aria-hidden="true"></i></button></div>';
  }

  // frame draws the whole view. It runs when the view changes state (a tool,
  // editing on or off), never while something is held.
  Inst.prototype.frame = function () {
    var t = this, S = t.S, el = t.el;
    if (!S.canEdit && isBlank(S.doc)) {
      // Nothing to read and nothing to edit: draw nothing.
      el.textContent = ''; t.svg = t.layer = null;
      if (!t.big) el.hidden = true;
      return;
    }
    el.hidden = false;
    var keep = focusKey(el);
    var blank = isBlank(S.doc);
    var bm = blank ? '<div class="pm-blank"><div><b>No map yet</b>' + (t.editing ? 'Pick a shape and drag on the grid, or ' : 'Draw one here, or ') +
      '<button type="button" class="pm-quiet" data-act="' + (t.editing ? 'text' : 'pen') + '">' + (t.editing ? 'paste one from the AI' : 'start drawing') + '</button>.</div></div>' : '';
    el.classList.toggle('pm-editing', t.editing);
    el.innerHTML = headHTML(t) +
      '<div class="pm-body"><div class="pm-main"><div class="pm-wrap' + (blank && !t.editing ? ' blank' : '') + '">' +
      '<svg class="pm-svg' + (t.editing ? ' edit tool-' + t.tool : '') + '" viewBox="0 0 ' + VW + ' ' + VH + '" role="group" aria-label="Map"></svg>' + bm +
      '</div><p class="pm-cap"></p></div>' +
      // The marker list is an editing aid only; readers just point at the markers.
      (t.editing ? '<div class="pm-side"><div class="pm-side-in"><p class="pm-side-h">Markers</p><ul class="pm-legend" aria-label="Markers"></ul></div></div>' : '') +
      '</div>' + (t.editing ? '<section class="pm-slot" aria-label="Selected"></section>' : '');
    t.svg = el.querySelector('svg');
    t.svg.innerHTML = '<g class="pm-gridg">' + gridSVG(t) + '</g><g class="pm-layer"></g>';
    t.layer = t.svg.querySelector('.pm-layer');
    t._lg = t._cap = null;
    t.paint();
    t.drawDrape();
    restoreFocus(el, keep);
  };

  // A redraw replaces the control that had focus; the same control gets it back.
  function focusKey(el) {
    var a = document.activeElement;
    if (!a || !el.contains(a)) return null;
    var keys = ['data-tool', 'data-mk', 'data-act', 'data-setbg'];
    for (var i = 0; i < keys.length; i++) {
      if (a.hasAttribute(keys[i])) return '[' + keys[i] + '="' + a.getAttribute(keys[i]) + '"]';
    }
    return null;
  }
  function restoreFocus(el, sel) {
    if (!sel) return;
    var n = el.querySelector(sel);
    if (n) n.focus();
  }

  // The background is one pattern over the map. With none, an editor still
  // gets a faint grid to place things by; readers get a plain map.
  function gridSVG(t) {
    var kind = t.S.doc.background || 'none', id = 'pmbg' + t.key, pat, faint = false;
    if (kind === 'none') { if (!t.editing) return ''; kind = 'grid'; faint = true; }
    if (kind === 'dots') pat = '<circle class="pm-bgdot" cx="' + U / 2 + '" cy="' + U / 2 + '" r=".9"/>';
    else if (kind === 'hex') {
      var s = U * 0.8, h = s * Math.sqrt(3);
      pat = '<path class="pm-bgline" d="M0 ' + h / 2 + 'L' + s / 2 + ' 0H' + 1.5 * s + 'L' + 2 * s + ' ' + h / 2 + 'L' + 1.5 * s + ' ' + h + 'H' + s / 2 + 'ZM' + 2 * s + ' ' + h / 2 + 'H' + 3 * s + '"/>';
      return '<defs><pattern id="' + id + '" patternUnits="userSpaceOnUse" width="' + 3 * s + '" height="' + h + '">' + pat + '</pattern></defs><rect width="' + VW + '" height="' + VH + '" fill="url(#' + id + ')"/>';
    } else pat = '<path class="pm-bgline' + (faint ? ' faint' : '') + '" d="M' + U + ' 0V' + U + 'H0"/>';
    // Dots sit on the grid points things snap to, so the pattern starts half a cell early.
    var off = kind === 'dots' ? ' x="' + -U / 2 + '" y="' + -U / 2 + '"' : '';
    return '<defs><pattern id="' + id + '" patternUnits="userSpaceOnUse"' + off + ' width="' + U + '" height="' + U + '">' + pat + '</pattern></defs><rect width="' + VW + '" height="' + VH + '" fill="url(#' + id + ')"/>';
  }
  function redrawBg(S) {
    S.insts.forEach(function (i) {
      var g = i.live() && i.svg && i.svg.querySelector('.pm-gridg');
      if (g) g.innerHTML = gridSVG(i);
    });
  }

  function tintCls(o) { return colorVal(o.color) ? ' tinted' : ''; }
  function tintSty(o, v) { var c = colorVal(o.color); return c ? ' style="' + v + ':' + c + '"' : ''; }

  function shapeSVG(s, sel) {
    var o, n = esc(s.name);
    if (s.t === 'label') {
      o = '<text class="pm-sh pm-sh-label' + tintCls(s) + '" data-id="' + esc(s.id) + '"' + tintSty(s, '--tint') + ' x="' + s.x * U + '" y="' + s.y * U + '">' + n + '</text>';
    } else if (s.pts) {
      var d = s.pts.map(function (p, i) { return (i ? 'L' : 'M') + p[0] * U + ' ' + p[1] * U; }).join(' ');
      o = '<g class="pm-sh' + tintCls(s) + '" data-id="' + esc(s.id) + '"' + tintSty(s, '--tint') + '><path class="pm-sh-' + s.t + '" d="' + d + '"/><path class="pm-sh-hit" d="' + d + '"/></g>';
    } else {
      o = '<g class="pm-sh' + tintCls(s) + '" data-id="' + esc(s.id) + '"' + tintSty(s, '--tint') + '><rect class="pm-sh-' + s.t + '" x="' + s.x * U + '" y="' + s.y * U + '" width="' + s.w * U + '" height="' + s.h * U + '" rx="' + (s.t === 'area' ? 6 : 2) + '"/>' +
        (n ? '<text class="pm-sh-name" x="' + (s.x * U + 5) + '" y="' + (s.y * U + 11) + '">' + n + '</text>' : '') + '</g>';
    }
    if (sel) {
      var b = bbox(s);
      o += '<rect class="pm-selbox" x="' + (b.x - 3) + '" y="' + (b.y - 3) + '" width="' + (b.w + 6) + '" height="' + (b.h + 6) + '"/>';
      if (!s.pts && s.t !== 'label') o += '<circle class="pm-hd" data-hd="' + esc(s.id) + '" cx="' + (s.x + s.w) * U + '" cy="' + (s.y + s.h) * U + '" r="5"/>';
    }
    return o;
  }

  // cardAttrs make a marker (or its list row) a hover-card trigger: a linked
  // page goes through entity_tooltip.js, which watches the whole document;
  // a marker with words goes through this widget's own binding.
  function cardAttrs(S, p) {
    var url = p.entityId ? previewURL(S.campaignId, p.entityId) : '';
    if (url) {
      var href = /^\/(?![\/\\])/.test(str(p.entityUrl)) ? p.entityUrl : '';
      return ' data-entity-preview="' + esc(url) + '"' + (href ? ' href="' + esc(href) + '"' : '');
    }
    return ' data-pin-text=""';
  }

  // TODO(#1447): each style kit gives the marker its own shape (Sharp a
  // square, Ornate a diamond, Flowing a pebble); until the kits land every
  // kit draws the disc.
  function pinSVG(t, p, nums) {
    var S = t.S, x = p.x * U, y = p.y * U, r = 9, ink = inkFor(p.color);
    var cls = 'pm-pin' + tintCls(p) + (p.dm ? ' dm' : '') + (t.editing && t.sel === p.id ? ' selp' : '') + (S.hot === p.id ? ' hot' : '') + (ink ? ' ink-' + ink : '');
    var inner = p.style === 'number'
      ? '<text class="pm-num" x="' + x + '" y="' + y + '">' + (nums[p.id] || '') + '</text>'
      : '<text class="pm-glyph" x="' + x + '" y="' + y + '">' + ICON_GLYPH[p.icon] + '</text>';
    return '<g class="' + cls + '" data-pin="' + esc(p.id) + '"' + tintSty(p, '--pc') + (p.dm ? ' data-dm-only=""' : '') +
      (t.editing ? '' : cardAttrs(S, p)) +
      ' tabindex="0" role="button" aria-label="' + esc((p.name || 'Marker') + (p.dm ? ', DM only' : '')) + '">' +
      '<circle class="pm-hit" cx="' + x + '" cy="' + y + '" r="16"/><circle class="pm-halo" cx="' + x + '" cy="' + y + '" r="' + (r + 4) + '"/>' +
      '<circle class="pm-disc" cx="' + x + '" cy="' + y + '" r="' + r + '"/>' + inner + '</g>';
  }

  function svgBody(t) {
    var S = t.S, nums = pinNumbers(S.doc.pins, S.canEdit);
    var shapes = S.doc.shapes.slice().sort(function (a, b) { return ORDER[a.t] - ORDER[b.t]; });
    // A shape being resized is drawn from its live outline instead of its saved one.
    if (t.temp && t.temp.ghost) shapes = shapes.map(function (s) { return s.id === t.temp.id ? t.temp : s; });
    return shapes.map(function (s) { return shapeSVG(s, t.editing && t.sel === s.id); }).join('') +
      (t.temp && !t.temp.ghost ? shapeSVG(t.temp, false) : '') +
      visiblePins(S.doc.pins, S.canEdit).map(function (p) { return pinSVG(t, p, nums); }).join('');
  }

  function capText(t) {
    if (t.editing) {
      if (t.tool === 'select') return 'Select a shape or marker to move it. Pull a corner to resize. Delete removes it.';
      if (t.tool === 'pin') return 'Click the map to place the marker.';
      if (t.tool === 'label') return 'Click where the words go.';
      return 'Drag on the map to draw it.';
    }
    return isBlank(t.S.doc) ? '' : 'Hover a marker for its card. Click the map to open it bigger.';
  }

  function legendHTML(t) {
    var S = t.S, nums = pinNumbers(S.doc.pins, S.canEdit);
    var html = visiblePins(S.doc.pins, S.canEdit).map(function (p) {
      var mk = p.style === 'number' ? '<span>' + (nums[p.id] || '') + '</span>' : '<i class="fa-solid fa-' + p.icon + '" aria-hidden="true"></i>';
      var ink = inkFor(p.color);
      var kind = p.dm ? '<span class="pm-lk cb-dm">DM only</span>' : '<span class="pm-lk">' + (p.entityId ? 'page' : 'note') + '</span>';
      return '<li class="' + (p.dm ? 'dm' : '') + tintCls(p) + (ink ? ' ink-' + ink : '') + '"' + tintSty(p, '--pc') + (p.dm ? ' data-dm-only=""' : '') + '>' +
        '<button type="button" data-lg="' + esc(p.id) + '"' + cardAttrs(S, p) + '><span class="pm-mk">' + mk + '</span><span class="pm-lname">' + esc(p.name || 'Marker') + '</span>' + kind + '</button></li>';
    }).join('');
    return html || '<li class="pm-lgnone">Pick 1 or the icon above the map, then click the map.</li>';
  }

  // paint redraws the map layer; the caption and the marker list change only
  // when their words do, so the page around the map never reflows mid-gesture.
  Inst.prototype.paint = function (mapOnly) {
    if (!this.layer) return;
    this.layer.innerHTML = svgBody(this);
    if (mapOnly) return;
    var cap = this.el.querySelector('.pm-cap'), ct = capText(this);
    if (cap && ct !== this._cap) { cap.textContent = ct; this._cap = ct; }
    var lg = this.el.querySelector('.pm-legend');
    if (lg) {
      var html = legendHTML(this);
      if (html !== this._lg) { lg.innerHTML = html; this._lg = html; }
      lightRows(this.S, lg);
    }
  };

  // ---------- Settings under the map ----------
  function swatches(o) {
    var c = validColor(o.color), custom = isHex(c);
    return '<div class="pm-fld">Colour<div class="pm-sw" role="group" aria-label="Colour">' +
      '<button type="button" class="theme" data-setcolor="" aria-pressed="' + !c + '" aria-label="The theme\'s colour" title="The theme\'s colour"></button>' +
      COLORS.map(function (k, i) {
        var label = k[0].toUpperCase() + k.slice(1);
        return '<button type="button" data-setcolor="' + k + '" style="--c:var(--map-c-' + i + ')" aria-pressed="' + (c === k) + '" aria-label="' + label + '" title="' + label + '"></button>';
      }).join('') +
      '<label class="cust' + (custom ? ' on' : '') + '" title="Any colour"><input type="color" data-f="color" value="' + (custom ? c : '#888888') + '" aria-label="Any colour"></label></div>' +
      '<p class="pm-cerr" role="status"></p></div>';
  }

  function showOf(S, p) { return S.show[p.id] || (p.entityId ? 'page' : 'text'); }

  function pickerHTML(t, p) {
    var id = t.key + '-pl';
    return '<div class="pm-fld"><label for="' + t.key + '-pq">Page</label><div class="pm-pick">' +
      '<input id="' + t.key + '-pq" data-pq role="combobox" aria-expanded="false" aria-autocomplete="list" aria-controls="' + id + '" autocomplete="off" spellcheck="false" maxlength="120"' +
      ' value="' + esc(p.entityName || p.page) + '" placeholder="Search for a page">' +
      '<ul class="pm-plist" id="' + id + '" role="listbox" aria-label="Pages" hidden></ul></div></div>';
  }

  Inst.prototype.drawDrape = function () {
    var t = this, S = t.S, slot = t.el.querySelector('.pm-slot');
    if (!slot) return;
    if (!t.editing) { slot.innerHTML = ''; return; }
    var o = t.selected();
    if (t.sel && !o) t.sel = null;
    if (t.drape === 'text') {
      slot.innerHTML = '<div class="pm-drape"><label class="pm-fld grow" for="' + t.key + '-sk">Paste the AI\'s map, or edit the lines' +
        '<textarea id="' + t.key + '-sk" class="pm-sketch" spellcheck="false" maxlength="' + LIM.sketch + '" disabled placeholder="room &quot;The house&quot; 17,10 to 23,16&#10;pin &quot;The door&quot; 20,16 icon door-open: It sticks."></textarea></label>' +
        '<div class="pm-drow"><p class="pm-skerr" role="status"></p><span class="pm-sp"></span><button type="button" class="pm-quiet" data-act="closedrape">Cancel</button>' +
        '<button type="button" class="pm-btn primary" data-act="drawtext" disabled>Draw it</button></div></div>';
      loadSketch(t);
      return;
    }
    if (o && isPin(o)) {
      var p = o, nm = t.key + '-st', sn = t.key + '-sh', show = showOf(S, p);
      slot.innerHTML = '<div class="pm-drape" data-for="' + esc(p.id) + '"><div class="pm-sgrid">' +
        '<div class="pm-scol"><label class="pm-fld">Marker name<input data-f="name" maxlength="' + LIM.name + '" value="' + esc(p.name) + '"></label>' +
        swatches(p) + '<label class="pm-chk"><input type="checkbox" data-f="dm"' + (p.dm ? ' checked' : '') + '>Only the DM team sees it</label></div>' +
        '<div class="pm-scol"><div class="pm-fld">Looks like<div class="pm-radios">' +
        '<label><input type="radio" name="' + nm + '" data-f="style" value="number"' + (p.style === 'number' ? ' checked' : '') + '>Number</label>' +
        '<label><input type="radio" name="' + nm + '" data-f="style" value="icon"' + (p.style === 'icon' ? ' checked' : '') + '>Icon</label></div></div>' +
        (p.style === 'icon' ? '<div class="pm-igrid" role="group" aria-label="Icon">' + ICONS.map(function (ic) {
          return '<button type="button" class="pm-icon" data-seticon="' + ic + '" aria-pressed="' + (p.icon === ic) + '" aria-label="' + ICON_NAMES[ic] + '" title="' + ICON_NAMES[ic] + '"><i class="fa-solid fa-' + ic + '" aria-hidden="true"></i></button>';
        }).join('') + '</div>' : '') + '</div>' +
        '<div class="pm-scol"><div class="pm-fld">On hover, show<div class="pm-radios">' +
        '<label><input type="radio" name="' + sn + '" data-f="show" value="page"' + (show === 'page' ? ' checked' : '') + '>A page</label>' +
        '<label><input type="radio" name="' + sn + '" data-f="show" value="text"' + (show === 'text' ? ' checked' : '') + '>Some text</label></div></div>' +
        (show === 'page' ? pickerHTML(t, p)
          : '<label class="pm-fld grow">Text<textarea data-f="text" rows="2" maxlength="' + LIM.text + '">' + esc(p.text) + '</textarea></label>') + '</div></div>' +
        '<div class="pm-drow"><button type="button" class="pm-btn danger" data-act="remove">Remove marker</button><span class="pm-sp"></span><button type="button" class="pm-quiet" data-act="closedrape">Done</button></div></div>';
      return;
    }
    if (o) {
      var s = o;
      slot.innerHTML = '<div class="pm-drape" data-for="' + esc(s.id) + '"><div class="pm-sgrid">' +
        '<div class="pm-scol"><label class="pm-fld">' + (s.t === 'label' ? 'Words' : 'Name') + '<input data-f="name" maxlength="' + LIM.name + '" value="' + esc(s.name) + '"></label></div>' +
        (s.t === 'label' ? '' : '<div class="pm-scol">' + swatches(s) + '</div>') +
        (RECTS.indexOf(s.t) >= 0 ? '<div class="pm-scol"><label class="pm-fld">Kind<select data-f="t">' + RECTS.map(function (k) {
          return '<option value="' + k + '"' + (k === s.t ? ' selected' : '') + '>' + (k === 'danger' ? 'Danger zone' : k[0].toUpperCase() + k.slice(1)) + '</option>';
        }).join('') + '</select></label></div>' : '') + '</div>' +
        '<div class="pm-drow"><button type="button" class="pm-btn danger" data-act="remove">Remove</button><span class="pm-sp"></span><button type="button" class="pm-quiet" data-act="closedrape">Done</button></div></div>';
      return;
    }
    slot.innerHTML = '<div class="pm-drape empty"><div class="pm-emptyin"><div class="pm-fld">Map background<div class="pm-seg" role="group" aria-label="Map background">' +
      BGS.map(function (k) { return '<button type="button" data-setbg="' + k + '" aria-pressed="' + (S.doc.background === k) + '">' + BG_NAMES[k] + '</button>'; }).join('') +
      '</div></div><p class="pm-hint"><b>Nothing selected.</b> Click a shape or marker on the map, or a marker in the list, to name it, colour it, change it or remove it here.</p></div></div>';
  };

  // ---------- From text ----------
  function loadSketch(t) {
    var S = t.S;
    api(S, 'GET', '/sketch').then(function (v) {
      var ta = t.drape === 'text' && t.el.querySelector('.pm-sketch');
      if (!ta) return;
      ta.value = str(v && v.text);
      ta.disabled = false;
      t.el.querySelector('[data-act="drawtext"]').disabled = false;
      ta.focus();
    }).catch(function (err) {
      var ta = t.drape === 'text' && t.el.querySelector('.pm-sketch');
      if (!ta) return;
      ta.disabled = false;
      t.el.querySelector('[data-act="drawtext"]').disabled = false;
      t.el.querySelector('.pm-skerr').textContent = 'The map\'s lines did not load (' + err.message + '). You can still paste new ones.';
    });
  }

  function warningText(w) { return typeof w === 'string' ? w : str(w && (w.message || w.text)); }

  function drawText(t, button) {
    var S = t.S, ta = t.el.querySelector('.pm-sketch'), errLine = t.el.querySelector('.pm-skerr');
    if (!ta) return;
    button.disabled = true; button.textContent = 'Drawing…';
    api(S, 'POST', '/sketch', { text: ta.value }).then(function (r) {
      var warns = (r && Array.isArray(r.warnings) ? r.warnings : []).map(warningText).filter(Boolean);
      var doc = normalise(r && r.data);
      if (isBlank(doc)) {
        button.disabled = false; button.textContent = 'Draw it';
        errLine.textContent = warns[0] || 'There is nothing to draw in those lines.';
        return;
      }
      S.doc = doc; S.show = {}; S.stash = {};
      S.insts.forEach(function (i) { i.sel = null; i.drape = null; i.temp = null; i.tool = 'select'; });
      frameAll(S);
      if (warns.length) {
        var cap = t.el.querySelector('.pm-cap');
        if (cap) cap.textContent = 'Drawn, leaving out ' + warns.length + ' line' + (warns.length > 1 ? 's' : '') + ': ' + warns.join(' ');
      }
      changed(S);
      flush(S);
    }).catch(function (err) {
      button.disabled = false; button.textContent = 'Draw it';
      errLine.textContent = err.message;
    });
  }

  // ---------- Hover: lighting and cards ----------
  // Lighting a marker only toggles a class, so the marker under the pointer
  // (or holding keyboard focus) is never replaced while its card opens.
  function setHot(S, id) {
    if (S.drag || id === S.hot) return;
    S.hot = id;
    S.insts.forEach(function (i) {
      if (!i.svg) return;
      i.svg.querySelectorAll('[data-pin]').forEach(function (g) { g.classList.toggle('hot', g.getAttribute('data-pin') === id); });
      var lg = i.el.querySelector('.pm-legend');
      if (lg) lightRows(S, lg);
    });
  }
  function lightRows(S, lg) {
    lg.querySelectorAll('[data-lg]').forEach(function (b) { b.classList.toggle('hot', b.getAttribute('data-lg') === S.hot); });
  }

  function pinOf(S, trigger) {
    var id = trigger.getAttribute('data-pin') || trigger.getAttribute('data-lg');
    return S.doc.pins.filter(function (p) { return p.id === id; })[0] || null;
  }
  function noteCard(S, trigger) {
    var p = pinOf(S, trigger);
    if (!p) return null;
    return { kind: (p.dm ? 'DM only · ' : '') + 'Note', title: p.name || 'Marker', text: p.text || 'No words yet.' };
  }

  // ---------- Events ----------
  // Everything is delegated from the view's own element once, so a redraw
  // never stacks listeners and nothing outlives an HTMX swap.
  function wire(t) {
    var el = t.el, S = t.S;
    el.addEventListener('click', function (ev) { onClick(t, ev); });
    el.addEventListener('input', function (ev) {
      if (ev.target.matches && ev.target.matches('[data-pq]')) { t.pq.i = 0; pickerShow(t, ev.target); return; }
      onField(t, ev);
    });
    el.addEventListener('change', function (ev) { onField(t, ev); });
    el.addEventListener('focusin', function (ev) {
      if (ev.target.matches && ev.target.matches('[data-pq]')) { t.pq.i = 0; ev.target.select(); pickerShow(t, ev.target); return; }
      var g = ev.target.closest && ev.target.closest('[data-pin],[data-lg]');
      if (g && (!t.editing || g.hasAttribute('data-lg'))) setHot(S, g.getAttribute('data-pin') || g.getAttribute('data-lg'));
    });
    el.addEventListener('focusout', function (ev) {
      if (ev.target.matches && ev.target.matches('[data-pq]')) { pickerHide(t, ev.target); return; }
      if (ev.target.closest && ev.target.closest('[data-pin],[data-lg]')) setHot(S, null);
    });
    el.addEventListener('keydown', function (ev) { if (ev.target.matches && ev.target.matches('[data-pq]')) pickerKey(t, ev); });
    el.addEventListener('pointerover', function (ev) {
      var row = ev.target.closest('[data-lg]');
      if (row) { setHot(S, row.getAttribute('data-lg')); return; }
      var g = ev.target.closest('[data-pin]');
      if (g && !t.editing) setHot(S, g.getAttribute('data-pin'));
    });
    el.addEventListener('pointerout', function (ev) {
      var n = ev.target.closest('[data-lg],[data-pin]');
      if (n && !n.contains(ev.relatedTarget)) setHot(S, null);
    });
    el.addEventListener('pointerdown', function (ev) {
      var li = ev.target.closest('[data-pick]');
      if (li) { ev.preventDefault(); pickerPick(t, Number(li.getAttribute('data-pick'))); return; }
      if (t.svg && t.svg.contains(ev.target)) onDown(t, ev);
    });
    el.addEventListener('pointermove', function (ev) { if (S.drag && S.drag.inst === t) onMove(t, ev); });
    el.addEventListener('pointerup', function (ev) { if (S.drag && S.drag.inst === t) onUp(t, ev); });
    el.addEventListener('pointercancel', function (ev) { if (S.drag && S.drag.inst === t) onUp(t, ev); });
  }

  function onClick(t, ev) {
    var S = t.S;
    // Reading, not editing: a click on the map (not a marker) opens it bigger.
    if (!t.editing && t.svg && t.svg.contains(ev.target)) {
      if (!t.big && !isBlank(S.doc) && !ev.target.closest('[data-pin]')) openBig(S, t.el.querySelector('[data-act="bigger"]') || t.svg);
      return;
    }
    var row = t.editing && ev.target.closest('[data-lg]');
    if (row) { t.sel = row.getAttribute('data-lg'); t.drape = null; t.drawDrape(); t.paint(); return; }
    var b = ev.target.closest('[data-act],[data-tool],[data-mk],[data-seticon],[data-setcolor],[data-setbg]');
    if (!b || !t.el.contains(b)) return;
    var a = b.getAttribute('data-act');
    if (a === 'pen') { t.editing = true; t.tool = isBlank(S.doc) ? 'room' : 'select'; t.sel = null; t.drape = null; t.frame(); focusFirstTool(t); return; }
    if (a === 'done') { stopEditing(t); return; }
    if (a === 'bigger') { openBig(S, b); return; }
    if (a === 'text') { t.drape = 'text'; t.sel = null; t.drawDrape(); t.paint(); return; }
    if (a === 'closedrape') { t.drape = null; t.sel = null; t.drawDrape(); t.paint(); return; }
    if (a === 'drawtext') { drawText(t, b); return; }
    if (a === 'remove') { removeSelected(t); return; }
    if (b.hasAttribute('data-tool')) { t.tool = b.getAttribute('data-tool'); t.sel = null; t.drape = null; t.frame(); return; }
    if (b.hasAttribute('data-mk')) {
      var mk = b.getAttribute('data-mk');
      t.tool = 'pin';
      if (mk === 'number') t.marker = 'number'; else { t.marker = 'icon'; t.icon = ICONS.indexOf(mk) >= 0 ? mk : 'location-dot'; }
      t.sel = null; t.drape = null; t.frame(); return;
    }
    if (b.hasAttribute('data-setbg')) {
      var bg = b.getAttribute('data-setbg');
      if (BGS.indexOf(bg) < 0 || bg === S.doc.background) return;
      S.doc.background = bg; redrawBg(S);
      t.el.querySelectorAll('[data-setbg]').forEach(function (x) { x.setAttribute('aria-pressed', String(x === b)); });
      changed(S); return;
    }
    var o = t.selected();
    if (b.hasAttribute('data-setcolor')) {
      if (!o) return;
      o.color = validColor(b.getAttribute('data-setcolor'));
      t.drawDrape(); restoreFocus(t.el, '[data-setcolor="' + o.color + '"]'); paintAll(S); changed(S); return;
    }
    if (b.hasAttribute('data-seticon') && o && isPin(o)) {
      var ic = b.getAttribute('data-seticon');
      if (ICONS.indexOf(ic) < 0) return;
      o.icon = ic; t.icon = ic;
      t.drawDrape(); restoreFocus(t.el, '[data-seticon="' + ic + '"]'); paintAll(S); changed(S);
    }
  }

  function focusFirstTool(t) {
    var n = t.el.querySelector('[data-tool][aria-pressed="true"]') || t.el.querySelector('[data-tool]');
    if (n) n.focus();
  }

  function stopEditing(t) {
    flush(t.S);
    t.editing = false; t.sel = null; t.drape = null; t.temp = null; t.tool = 'select';
    t.frame();
    var pen = t.el.querySelector('[data-act="pen"]');
    if (pen) pen.focus();
  }

  function removeSelected(t) {
    var S = t.S, id = t.sel;
    if (!id) return;
    S.doc.shapes = S.doc.shapes.filter(function (s) { return s.id !== id; });
    S.doc.pins = S.doc.pins.filter(function (p) { return p.id !== id; });
    delete S.show[id]; delete S.stash[id];
    S.insts.forEach(function (i) { if (i.sel === id) i.sel = null; });
    S.insts.forEach(function (i) { if (i.live() && i.layer) { i.drawDrape(); i.paint(); } });
    if (isBlank(S.doc)) frameAll(S);
    changed(S);
  }

  function onField(t, ev) {
    var f = ev.target.getAttribute && ev.target.getAttribute('data-f');
    if (!f) return;
    var S = t.S, o = t.selected();
    if (!o) return;
    var v = ev.target.type === 'checkbox' ? ev.target.checked : ev.target.value;
    // A radio's, checkbox's or select's input event repeats its change, and a
    // text field's change repeats its input; act on one of each.
    var choice = ev.target.type === 'radio' || ev.target.type === 'checkbox' || ev.target.tagName === 'SELECT';
    if (ev.type === 'input' && choice) return;
    if (ev.type === 'change' && (f === 'name' || f === 'text')) return;
    if (f === 'name') o.name = str(v).slice(0, LIM.name);
    else if (f === 'text') o.text = str(v).slice(0, LIM.text);
    else if (f === 'dm') o.dm = !!v;
    else if (f === 'color') {
      var cerr = t.el.querySelector('.pm-cerr');
      if (amberish(v)) {
        // Said in words; the colour stays what it was.
        if (cerr) cerr.textContent = 'That is too close to amber, which marks what only the DM sees. Pick another.';
        return;
      }
      if (cerr) cerr.textContent = '';
      o.color = validColor(v);
    }
    else if (f === 'style') o.style = v === 'icon' ? 'icon' : 'number';
    else if (f === 't') { if (RECTS.indexOf(v) < 0) return; o.t = v; }
    else if (f === 'show') setShow(S, o, v === 'page' ? 'page' : 'text');
    else return;
    // A choice that changes which settings show redraws them, keeping focus.
    if (ev.type === 'change' && (f === 'style' || f === 'show' || f === 'color')) {
      t.drawDrape();
      restoreFocus(t.el, '[data-f="' + f + '"]' + (f === 'color' ? '' : '[value="' + (f === 'show' ? showOf(S, o) : o.style) + '"]'));
    }
    paintAll(S);
    changed(S);
  }

  // A marker shows a page or some text. Switching to text sets the page
  // aside (not lost) so switching back in this visit brings it back.
  function setShow(S, p, mode) {
    S.show[p.id] = mode;
    if (mode === 'text' && p.entityId) {
      S.stash[p.id] = { entityId: p.entityId, entityName: p.entityName, entityUrl: p.entityUrl, page: p.page };
      p.entityId = ''; delete p.entityName; delete p.entityUrl;
    } else if (mode === 'page' && !p.entityId && S.stash[p.id]) {
      var k = S.stash[p.id];
      p.entityId = k.entityId; p.page = k.page;
      if (k.entityName) p.entityName = k.entityName;
      if (k.entityUrl) p.entityUrl = k.entityUrl;
    }
  }

  // ---------- Page picker ----------
  // Every page in the campaign, found by typing: the search asks the server
  // once the typing rests, so hundreds of pages are as quick as five.
  function pickerList(t) { return t.el.querySelector('.pm-plist'); }

  function pickerShow(t, inp) {
    var ul = pickerList(t), pq = t.pq;
    if (!ul) return;
    var q = inp.value.trim(), p = t.selected();
    var current = !!(p && q && q === (p.entityName || p.page));
    var rows;
    if (current || q.length < LIM.search) {
      pq.list = []; pq.total = 0;
      rows = '<li class="pm-more" aria-disabled="true">Type ' + (current ? 'a page\'s name' : 'two or more letters') + ' to search.</li>';
    } else if (pq.q !== q) {
      rows = ul.hidden ? '<li class="pm-more" aria-disabled="true">Searching…</li>' : null;
      pickerSearch(t, inp, q);
    } else {
      pq.i = clamp(pq.i, 0, Math.max(0, pq.list.length - 1));
      rows = pq.list.map(function (r, i) {
        var parts = highlight(r.name, q);
        return '<li role="option" id="' + t.key + '-o' + i + '" data-pick="' + i + '" aria-selected="' + (i === pq.i) + '">' +
          '<span class="pm-pname">' + esc(parts[0]) + (parts[1] ? '<mark>' + esc(parts[1]) + '</mark>' : '') + esc(parts[2]) + '</span>' +
          (r.type_name ? '<small>' + esc(r.type_name) + '</small>' : '') + '</li>';
      }).join('') +
        (pq.total > pq.list.length ? '<li class="pm-more" aria-disabled="true">' + (pq.total - pq.list.length) + ' more, keep typing</li>' : '') +
        (!pq.list.length ? '<li class="pm-more" aria-disabled="true">No page by that name.</li>' : '');
    }
    if (rows !== null) ul.innerHTML = rows;
    ul.hidden = false;
    inp.setAttribute('aria-expanded', 'true');
    if (pq.list.length) {
      inp.setAttribute('aria-activedescendant', t.key + '-o' + pq.i);
      var cur = ul.querySelector('#' + t.key + '-o' + pq.i);
      if (cur && cur.scrollIntoView) cur.scrollIntoView({ block: 'nearest' });
    } else inp.removeAttribute('aria-activedescendant');
  }

  function pickerSearch(t, inp, q) {
    var pq = t.pq, S = t.S;
    clearTimeout(pq.timer);
    pq.timer = setTimeout(function () {
      var my = ++pq.token;
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(S.campaignId) + '/entities/search?q=' + encodeURIComponent(q) + '&pages=1', {
        headers: { Accept: 'application/json' }
      }).then(function (res) {
        if (!res.ok) throw new Error('search ' + res.status);
        return res.json();
      }).then(function (data) {
        if (my !== pq.token || document.activeElement !== inp) return;
        var all = (data && Array.isArray(data.results) ? data.results : []).filter(function (r) { return r && r.id; });
        pq.list = all.slice(0, LIM.shown);
        pq.total = Math.max(num(data && data.total, all.length), pq.list.length);
        pq.q = q; pq.i = 0;
        pickerShow(t, inp);
      }).catch(function () {
        if (my !== pq.token) return;
        var ul = pickerList(t);
        if (ul) ul.innerHTML = '<li class="pm-more" aria-disabled="true">The search did not answer. Try again.</li>';
      });
    }, 220);
  }

  function pickerHide(t, inp) {
    var ul = pickerList(t), p = t.selected();
    clearTimeout(t.pq.timer); t.pq.token++;
    t.pq.q = '';
    if (ul) ul.hidden = true;
    inp.setAttribute('aria-expanded', 'false');
    inp.removeAttribute('aria-activedescendant');
    if (p) inp.value = p.entityName || p.page;
  }

  function pickerPick(t, i) {
    var p = t.selected(), r = t.pq.list[i], inp = t.el.querySelector('[data-pq]');
    if (!p || !isPin(p) || !r || !inp) return;
    p.entityId = str(r.id); p.entityName = str(r.name); p.page = str(r.name);
    if (r.url) p.entityUrl = str(r.url);
    delete t.S.stash[p.id];
    inp.value = p.entityName;
    pickerHide(t, inp);
    paintAll(t.S);
    changed(t.S);
  }

  function pickerKey(t, ev) {
    var inp = ev.target, ul = pickerList(t), open = ul && !ul.hidden, pq = t.pq;
    if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') {
      ev.preventDefault();
      if (open && pq.list.length) pq.i = clamp(pq.i + (ev.key === 'ArrowDown' ? 1 : -1), 0, pq.list.length - 1);
      pickerShow(t, inp);
    } else if (ev.key === 'Enter') {
      ev.preventDefault();
      if (open && pq.list[pq.i]) pickerPick(t, pq.i);
    } else if (ev.key === 'Escape' && open) {
      // This Escape closes the list only; the next one clears the selection.
      ev.preventDefault();
      ev.stopPropagation();
      pickerHide(t, inp);
    }
  }

  // ---------- Pointer work on the map ----------
  function gp(svg, ev) {
    var pt = svg.createSVGPoint(); pt.x = ev.clientX; pt.y = ev.clientY;
    var r = pt.matrixTransform(svg.getScreenCTM().inverse());
    return { x: r.x / U, y: r.y / U };
  }

  function onDown(t, ev) {
    var S = t.S, svg = t.svg;
    // A thing still settling from the last move lands before another is held.
    if (!t.editing || ev.button > 0 || S.settling) return;
    var at = gp(svg, ev), hd = ev.target.closest('[data-hd]'), pg = ev.target.closest('[data-pin]'), sh = ev.target.closest('.pm-sh');
    if (t.tool === 'select') {
      // Nothing is redrawn while a thing is held: it rides on a transform and
      // lands on the grid when let go, so the rest of the page holds still.
      if (hd) {
        var rs = t.find(hd.getAttribute('data-hd'));
        if (rs) S.drag = { inst: t, kind: 'resize', o: rs, node: hd };
      } else if (pg || sh) {
        var node = pg || sh, o = t.find(pg ? pg.getAttribute('data-pin') : sh.getAttribute('data-id'));
        if (o) {
          S.drag = { inst: t, kind: 'move', o: o, node: node, sx: at.x, sy: at.y, dx: 0, dy: 0, lim: limits(o) };
          node.classList.add('lifted');
        }
      } else if (t.sel || t.drape) { t.sel = null; t.drape = null; t.drawDrape(); t.paint(); }
      if (S.drag) svg.classList.add('dragging');
    } else if (full(t, t.tool === 'pin' ? 'pins' : 'shapes')) {
      ev.preventDefault();
      return;
    } else if (t.tool === 'pin') {
      var np = {
        id: uid('p'), x: clamp(snap(at.x, 0.5), 0.5, GW - 0.5), y: clamp(snap(at.y, 0.5), 0.5, GH - 0.5),
        name: 'Marker ' + (S.doc.pins.length + 1), style: t.marker, icon: t.marker === 'icon' ? t.icon : 'location-dot',
        entityId: '', page: '', text: '', dm: false, color: ''
      };
      S.doc.pins.push(np); S.show[np.id] = 'text';
      t.sel = np.id; t.tool = 'select'; frameAll(S); selectName(t); changed(S);
      ev.preventDefault();
      return;
    } else if (t.tool === 'label') {
      var nl = { id: uid('s'), t: 'label', x: clamp(Math.round(at.x), 0, GW - 2), y: clamp(Math.round(at.y), 1, GH), name: 'Label' };
      S.doc.shapes.push(nl);
      t.sel = nl.id; t.tool = 'select'; frameAll(S); selectName(t); changed(S);
      ev.preventDefault();
      return;
    } else {
      S.drag = { inst: t, kind: 'draw', sx: clamp(Math.round(at.x), 0, GW), sy: clamp(Math.round(at.y), 0, GH) };
    }
    if (S.drag) {
      if (Chronicle.hovercard) Chronicle.hovercard.close();
      try { svg.setPointerCapture(ev.pointerId); } catch (e) { /* the pointer already left */ }
    }
    ev.preventDefault();
  }

  // full says so in the caption when the map holds all it may.
  function full(t, what) {
    var n = t.S.doc[what].length;
    if (n < LIM[what]) return false;
    var cap = t.el.querySelector('.pm-cap');
    if (cap) cap.textContent = 'This map holds ' + LIM[what] + ' ' + (what === 'pins' ? 'markers' : 'shapes') + ', its most. Remove one to add another.';
    return true;
  }

  function selectName(t) {
    var nm = t.el.querySelector('.pm-drape input[data-f="name"]');
    if (nm) { nm.focus(); nm.select(); }
  }

  // A move asks for one frame at a time; the latest pointer position wins.
  function onMove(t, ev) {
    var S = t.S;
    S.drag.last = gp(t.svg, ev);
    if (S.drag.raf) return;
    S.drag.raf = requestAnimationFrame(function () {
      var d = S.drag;
      if (!d || d.inst !== t) return;
      d.raf = 0;
      if (d.last) step(t, d, d.last);
    });
  }

  function step(t, d, at) {
    var o = d.o;
    if (d.kind === 'move') {
      var L = d.lim;
      d.dx = clamp(at.x - d.sx, L.x0, L.x1); d.dy = clamp(at.y - d.sy, L.y0, L.y1);
      d.node.setAttribute('transform', 'translate(' + d.dx * U + ' ' + d.dy * U + ')');
    } else if (d.kind === 'resize') {
      t.temp = { id: o.id, t: o.t, x: o.x, y: o.y, w: clamp(at.x - o.x, 1, GW - o.x), h: clamp(at.y - o.y, 1, GH - o.y), name: o.name, color: o.color, ghost: true };
      t.paint(true);
    } else if (d.kind === 'draw') {
      t.temp = drawn(t.tool, d.sx, d.sy, at.x, at.y);
      t.paint(true);
    }
  }

  function onUp(t, ev) {
    var S = t.S, d = S.drag;
    S.drag = null;
    if (d.raf) cancelAnimationFrame(d.raf);
    // The last position may not have had its frame yet.
    if (ev.type === 'pointerup' && d.last) step(t, d, d.last);
    t.svg.classList.remove('dragging');
    if (d.kind === 'draw') {
      var s = land(t.temp);
      t.temp = null; t.tool = 'select';
      if (s) { s.id = uid('s'); S.doc.shapes.push(s); t.sel = s.id; changed(S); }
      frameAll(S);
      return;
    }
    if (d.kind === 'resize') {
      var g = t.temp; t.temp = null;
      if (g) { d.o.w = clamp(Math.round(g.w), 1, GW - d.o.x); d.o.h = clamp(Math.round(g.h), 1, GH - d.o.y); changed(S); }
      paintAll(S);
      return;
    }
    // A move lands on the nearest grid point with a short settle; a press
    // without a move just selects.
    var o = d.o, fresh = t.sel !== o.id;
    var moved = ev.type === 'pointerup' && (Math.abs(d.dx) > 0.15 || Math.abs(d.dy) > 0.15);
    t.sel = o.id; t.drape = null;
    if (!moved) {
      d.node.classList.remove('lifted'); d.node.removeAttribute('transform');
      if (fresh) t.drawDrape();
      t.paint();
      return;
    }
    var to = settle(o, d.dx, d.dy);
    var commit = function () {
      moveBy(o, to.dx, to.dy);
      if (fresh) t.drawDrape();
      paintAll(S);
      changed(S);
    };
    if (calm()) { commit(); return; }
    d.node.style.transition = 'transform 120ms cubic-bezier(.2,.8,.2,1)';
    d.node.setAttribute('transform', 'translate(' + to.dx * U + ' ' + to.dy * U + ')');
    S.settling = setTimeout(function () { S.settling = 0; if (!S.dead) commit(); }, 130);
  }

  // ---------- Keys ----------
  // The keys act only in this map's own view: the open Bigger dialog, or the
  // small map while it is being edited and focus is in it (or nowhere).
  function activeInst(S) {
    if (S.pop && !S.pop.root.hidden) return S.pop.inst;
    var a = document.activeElement;
    if (S.small.editing && (!a || a === document.body || S.el.contains(a))) return S.small;
    return null;
  }

  function onKey(S, ev) {
    if (S.dead || ev.defaultPrevented || !S.loaded) return;
    var t = activeInst(S);
    if (!t) return;
    var bigOpen = S.pop && !S.pop.root.hidden;
    if (ev.key === 'Tab' && bigOpen) { trapTab(S, ev); return; }
    if (ev.key === 'Escape') {
      if (S.drag) return;
      if (t.editing && (t.drape || t.sel)) { t.drape = null; t.sel = null; t.drawDrape(); t.paint(); ev.preventDefault(); return; }
      if (bigOpen) { closeBig(S); ev.preventDefault(); return; }
      if (t.editing) { stopEditing(t); ev.preventDefault(); }
      return;
    }
    if (ev.ctrlKey || ev.metaKey || ev.altKey || typingIn(ev.target)) return;
    if ((ev.key === 'Delete' || ev.key === 'Backspace') && t.editing && t.sel && !S.drag) {
      ev.preventDefault();
      removeSelected(t);
    }
  }

  function trapTab(S, ev) {
    var box = S.pop.sheet;
    var f = Array.prototype.filter.call(box.querySelectorAll('button, input, select, textarea, [tabindex="0"]'), function (n) {
      // SVG markers have no offsetParent at all (undefined), so they count as shown.
      return !n.disabled && n.offsetParent !== null;
    });
    if (!f.length) return;
    var first = f[0], last = f[f.length - 1];
    if (ev.shiftKey && document.activeElement === first) { ev.preventDefault(); last.focus(); }
    else if (!ev.shiftKey && document.activeElement === last) { ev.preventDefault(); first.focus(); }
    else if (!box.contains(document.activeElement)) { ev.preventDefault(); first.focus(); }
  }

  // ---------- Bigger: the same map grown out of the small one ----------
  function ensurePop(S) {
    if (S.pop) return S.pop;
    var root = document.createElement('div');
    root.className = 'pm-pop';
    root.hidden = true;
    root.setAttribute('role', 'dialog');
    root.setAttribute('aria-modal', 'true');
    root.setAttribute('aria-labelledby', S.id + '-popt');
    root.innerHTML = '<div class="pm-scrim"></div><div class="pm-bigsheet"><div class="pm-bighead"><h3 class="pm-bigtitle" id="' + S.id + '-popt">Map</h3>' +
      '<span class="pm-sp"></span><button type="button" class="pm-btn" data-popdone>Done</button></div><div class="pm pm-big"></div></div>';
    document.body.appendChild(root);
    var sheet = root.querySelector('.pm-bigsheet'), mapEl = root.querySelector('.pm-big');
    S.pop = { root: root, sheet: sheet, inst: null, opener: null };
    S.pop.inst = new Inst(S, mapEl, true);
    root.querySelector('[data-popdone]').addEventListener('click', function () { closeBig(S); });
    if (Chronicle.hovercard) S.unbind.push(Chronicle.hovercard.bind(mapEl, '[data-pin-text]', function (t) { return noteCard(S, t); }));
    return S.pop;
  }

  function openBig(S, from) {
    var pop = ensurePop(S), r = (from || S.el).getBoundingClientRect();
    pop.opener = from && from.focus ? from : null;
    pop.sheet.style.setProperty('--pm-ox', (r.left + r.width / 2) / window.innerWidth * 100 + '%');
    pop.sheet.style.setProperty('--pm-oy', (r.top + r.height / 2) / window.innerHeight * 100 + '%');
    if (Chronicle.hovercard) Chronicle.hovercard.close();
    setHot(S, null);
    if (S.small.editing) { S.small.sel = null; S.small.drape = null; }
    pop.root.classList.remove('out');
    pop.root.hidden = false;
    var B = pop.inst;
    B.editing = S.canEdit; B.tool = 'select'; B.sel = null; B.drape = null; B.temp = null;
    B.frame();
    pop.root.querySelector('[data-popdone]').focus();
  }

  function closeBig(S) {
    var pop = S.pop;
    if (!pop || pop.root.hidden) return;
    flush(S);
    if (Chronicle.hovercard) Chronicle.hovercard.close();
    setHot(S, null);
    var done = function () {
      pop.root.hidden = true; pop.root.classList.remove('out');
      pop.inst.el.textContent = ''; pop.inst.svg = pop.inst.layer = null;
      S.small.frame();
      if (pop.opener && pop.opener.isConnected) pop.opener.focus();
      else { var b = S.el.querySelector('[data-act="bigger"]'); if (b) b.focus(); }
    };
    if (still()) { done(); return; }
    // Done shrinks it back into the block it grew from.
    pop.root.classList.add('out');
    setTimeout(done, 150);
  }
})();
