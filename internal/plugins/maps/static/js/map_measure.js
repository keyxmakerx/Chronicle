/**
 * map_measure.js -- the Measure tool on a Chronicle map.
 *
 * Everyone who can see a map can measure on it, players included. A path is
 * laid by clicking each turn; double-click or Esc finishes it, Backspace drops
 * the last point and a second Esc clears it (touch screens get a Finish
 * button). A running distance shows at each turn, the total at the end, and
 * the panel lists the total and every leg.
 *
 * Measuring is the viewer's own: the path is never saved, sent or broadcast.
 * The only thing that reaches the server is the map's scale, which the owner
 * or a member with DM access sets once (PUT .../maps/:mid/measure, refused for
 * anyone else by MapService.SetMeasureScale) and everyone then measures with.
 *
 * Two kinds of map:
 *  - A hex map (grid type hex, with its layer showing) snaps every point to a
 *    hex centre, highlights the hexes along the path and reads out hexes,
 *    miles and days on foot from the layer's miles per hex and per day. The
 *    hex maths and the trip wording are map_hexes.js's own (hexLine,
 *    distance, tripSummary), not a copy.
 *  - Any other map uses the stored scale: a line between two points (percent
 *    of the picture) and its real length, so a bigger upload of the same
 *    picture keeps it.
 *
 * The pure parts are at the top and exported for test/js/map_measure.test.mjs;
 * the browser part needs Leaflet and window.chronicleMap.
 */
(function () {
  'use strict';

  // The units a scale can be given in: the stored value, its long name and
  // its short form for the chips. The server allows exactly these
  // (MeasureUnits in display_settings.go).
  var UNITS = {
    miles: ['miles', 'mi'],
    km: ['kilometres', 'km'],
    feet: ['feet', 'ft'],
    leagues: ['leagues', 'lea']
  };
  var UNIT_ORDER = ['miles', 'km', 'feet', 'leagues'];
  // The server's cap on a scale's length.
  var MAX_LENGTH = 100000;
  // Points closer than this on screen are one point: a double-click lands two
  // clicks on the same spot before it finishes the path.
  var SAME_SPOT_PX = 4;
  // A scale line shorter than this on screen is a stray click, not a drag.
  var MIN_SCALE_PX = 12;

  // hexMath is map_hexes.js's pure maths, read from the loaded module (the
  // viewer loads it on every hex map). Tests hand it in with useHexMath.
  var givenHexMath = null;
  function hexMath() {
    if (givenHexMath) return givenHexMath;
    if (typeof window !== 'undefined' && window.ChronicleMapHexes && window.ChronicleMapHexes.math) return window.ChronicleMapHexes.math;
    return null;
  }

  // ---- Pure maths ----

  // unitsPerPixel is how many of the scale's units one map pixel covers, or
  // null when there is no usable scale. mapW and mapH are the picture's size,
  // because the scale's ends are stored in percent.
  function unitsPerPixel(scale, mapW, mapH) {
    if (!scale || !scale.a || !scale.b || !UNITS[scale.unit]) return null;
    var len = Number(scale.length);
    if (!(len > 0) || !isFinite(len)) return null;
    var px = Math.hypot((scale.b.x - scale.a.x) / 100 * mapW, (scale.b.y - scale.a.y) / 100 * mapH);
    if (!(px > 0)) return null;
    return len / px;
  }

  // legLengths is each leg of a path of map-pixel points, in the scale's units.
  function legLengths(points, perPx) {
    var out = [];
    for (var i = 1; i < points.length; i++) {
      out.push(Math.hypot(points[i].x - points[i - 1].x, points[i].y - points[i - 1].y) * perPx);
    }
    return out;
  }

  // running is the distance so far at each turn after the first.
  function running(legs) {
    var out = [], t = 0;
    for (var i = 0; i < legs.length; i++) { t += legs[i]; out.push(t); }
    return out;
  }

  function sum(list) { return list.reduce(function (t, x) { return t + (x || 0); }, 0); }

  // fmt is a distance as people read it: whole numbers from 10 up (with
  // thousands separators from 100), one decimal below.
  function fmt(n) {
    if (!isFinite(n)) return '';
    if (n >= 100) return Math.round(n).toLocaleString('en-US');
    if (n >= 10) return String(Math.round(n));
    return String(Math.round(n * 10) / 10);
  }

  // dropRepeats removes consecutive points that land on the same spot (within
  // eps in whatever units x and y are in), keeping the first of each run.
  function dropRepeats(points, eps) {
    var out = [];
    points.forEach(function (p) {
      var last = out[out.length - 1];
      if (last && Math.hypot(p.x - last.x, p.y - last.y) < eps) return;
      out.push(p);
    });
    return out;
  }

  // hexLegs is the number of hexes in each leg of a path of hexes.
  function hexLegs(hexes) {
    var hx = hexMath(), out = [];
    for (var i = 1; i < hexes.length; i++) out.push(hx.distance(hexes[i - 1], hexes[i]));
    return out;
  }

  // hexRoute is the hexes the path walks through, joints counted once, so its
  // length minus one is the path's length in hexes (tripSummary's reading).
  function hexRoute(hexes) {
    var hx = hexMath(), out = [];
    for (var i = 0; i < hexes.length; i++) {
      if (i === 0) { out.push(hexes[0]); continue; }
      hx.hexLine(hexes[i - 1], hexes[i]).slice(1).forEach(function (h) { out.push(h); });
    }
    return out;
  }

  // hexCells is the distinct hexes along the route, for the highlight.
  function hexCells(route) {
    var seen = {}, out = [];
    route.forEach(function (h) {
      var k = h.col + ',' + h.row;
      if (seen[k]) return;
      seen[k] = 1;
      out.push(h);
    });
    return out;
  }

  // hexReadout is the hex map's readout for a path of hexes: "N hexes · M
  // miles" and "About D days on foot" (halves, at least half a day), from the
  // layer's travel figures, worded as Plan a trip words them.
  function hexReadout(hexes, perHex, perDay) {
    var hx = hexMath();
    var route = hexRoute(hexes);
    var s = hx.tripSummary(route, { milesPerHex: perHex, milesPerDay: perDay });
    if (s) return { hexes: s.hexes, miles: s.miles, days: s.days, head: s.head, about: s.about };
    return { hexes: 0, miles: 0, days: 0, head: '0 hexes · 0 miles', about: 'About 0 days on foot' };
  }

  // hexChip is a running total at a turn on a hex map.
  function hexChip(n, perHex) { return n + (n === 1 ? ' hex' : ' hexes') + ' · ' + (n * perHex) + ' mi'; }

  // unitChip is a running total at a turn on a picture map.
  function unitChip(n, unit) { return fmt(n) + ' ' + (UNITS[unit] ? UNITS[unit][1] : ''); }

  // scaleBody is what "Save scale" sends: the line's ends in percent of the
  // picture (clamped onto it and rounded, so a drag that ends a hair off the
  // edge still saves) and the typed length. Null when the length is not a
  // number the server would take, or the ends are one spot.
  function scaleBody(a, b, length, unit) {
    var n = Number(length);
    if (!(n > 0) || !isFinite(n) || n > MAX_LENGTH || !UNITS[unit]) return null;
    function pc(v) { return Math.round(Math.max(0, Math.min(100, v)) * 1000) / 1000; }
    var A = { x: pc(a.x), y: pc(a.y) }, B = { x: pc(b.x), y: pc(b.y) };
    if (Math.hypot(B.x - A.x, B.y - A.y) < 0.1) return null;
    return { a: A, b: B, length: n, unit: unit };
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
      UNITS: UNITS, UNIT_ORDER: UNIT_ORDER, MAX_LENGTH: MAX_LENGTH, SAME_SPOT_PX: SAME_SPOT_PX,
      unitsPerPixel: unitsPerPixel, legLengths: legLengths, running: running, sum: sum, fmt: fmt,
      dropRepeats: dropRepeats, hexLegs: hexLegs, hexRoute: hexRoute, hexCells: hexCells,
      hexReadout: hexReadout, hexChip: hexChip, unitChip: unitChip, scaleBody: scaleBody,
      useHexMath: function (m) { givenHexMath = m; }
    };
  }
  if (typeof window === 'undefined') return;

  // ---- Browser part ----
  var coarseQuery = window.matchMedia ? window.matchMedia('(pointer: coarse)') : null;
  // touch is whether the main pointer is a finger: there is no hover or
  // double-click, so the hints say "tap" and point at the Finish button.
  function touch() { return !!(coarseQuery && coarseQuery.matches); }

  // el builds an element. Strings become text nodes, never markup, so nothing
  // here can inject HTML whatever a label holds.
  function el(tag, cls, kids) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    (kids || []).forEach(function (k) {
      if (k === null || k === undefined || k === false) return;
      n.appendChild(typeof k === 'string' ? document.createTextNode(k) : k);
    });
    return n;
  }
  function icon(name) { var i = el('i', 'fa-solid ' + name); i.setAttribute('aria-hidden', 'true'); return i; }
  function kbd(k) { return el('kbd', '', [k]); }
  function button(cls, kids, label) {
    var b = el('button', cls, kids);
    b.type = 'button';
    if (label) b.setAttribute('aria-label', label);
    return b;
  }
  // grow replays the Grow move on a panel that has just appeared. Calm and Off
  // are handled by the app's motion rules (input.css), reduced motion by ours.
  function grow(node) {
    node.classList.remove('mp-ms-grow');
    void node.offsetWidth;
    node.classList.add('mp-ms-grow');
  }

  function attach(ctx) {
    if (!ctx || ctx.measure || !ctx.map || typeof L === 'undefined') return ctx && ctx.measure;
    var map = ctx.map;
    var W = ctx.imageW, H = ctx.imageH;
    var panel = document.getElementById('mp-measure');
    var hintEl = document.getElementById('mp-hint');
    var wrap = document.getElementById('map-wrap');
    var container = map.getContainer();
    var canScale = !!ctx.canSetScale;
    var scaleURL = '/campaigns/' + encodeURIComponent(ctx.campaignID) + '/maps/' + encodeURIComponent(ctx.mapID) + '/measure';

    var active = false;
    var mode = 'measure';   // 'measure' | 'scale' (dragging out the scale line)
    var pts = [];           // the path's points in map pixels, {x, y, h?} (h: the hex on a hex map)
    var live = null;        // where the pointer is, while a path is being laid
    var done = false;       // the path was finished
    var sl = null;          // the scale line being drawn, {a, b} in map pixels
    var dragging = false;
    var pop = null;         // the "How long is that line?" box
    var saving = false;
    var raf = 0;
    var lastPointer = '';   // the kind of pointer that last touched the map

    var pane = map.getPane('mpMeasure') || map.createPane('mpMeasure');
    // Above the pins (600) so the path reads over them, below their names and
    // cards (650, 700).
    pane.style.zIndex = 610;
    pane.style.pointerEvents = 'none';
    var renderer = L.svg({ pane: 'mpMeasure', padding: 0.5 });
    var group = L.layerGroup().addTo(map);

    function ll(p) { return L.latLng(H - p.y, p.x); }
    function fromLL(latlng) { return { x: latlng.lng, y: H - latlng.lat }; }
    function onPicture(p) { return p.x >= 0 && p.y >= 0 && p.x <= W && p.y <= H; }
    function clampPic(p) { return { x: Math.max(0, Math.min(W, p.x)), y: Math.max(0, Math.min(H, p.y)) }; }
    function scale() { var D = ctx.getDisplay ? ctx.getDisplay() : null; return (D && D.measure) || null; }
    function hexInfo() { return ctx.hexes && ctx.hexes.measureInfo ? ctx.hexes.measureInfo() : null; }
    function isHex() { return !!hexInfo() && !!hexMath(); }
    function perPx() { return unitsPerPixel(scale(), W, H); }
    function measurable() { return isHex() || perPx() !== null; }

    // snap puts a point on its hex's centre on a hex map; null when the point
    // lies outside the hex field (a field pinned to a picture), so a click
    // there adds nothing.
    function snap(p) {
      var info = hexInfo(), hx = hexMath();
      if (!info || !hx) return { x: p.x, y: p.y };
      var lay = info.layout;
      var c = hx.hexAtMap(lay, p.x, p.y);
      if (!c) return null;
      var cc = hx.center(lay.geo, c.col, c.row);
      return { x: lay.xf.x + cc[0] * lay.xf.s, y: lay.xf.y + cc[1] * lay.xf.s, h: c };
    }

    function path() {
      var a = pts.slice();
      // The pointer's spot is the next turn while laying a path; a tap leaves
      // it on the last point, which is no leg at all.
      var last = a[a.length - 1];
      if (!done && live && last && (live.x !== last.x || live.y !== last.y)) a.push(live);
      return a;
    }
    function legsOf(a) {
      if (a.length < 2) return [];
      if (isHex()) return hexLegs(a.map(function (p) { return p.h; }));
      var k = perPx();
      return k === null ? [] : legLengths(a, k);
    }

    // ---- Drawing on the map ----

    function hexRings(cells) {
      var info = hexInfo(), hx = hexMath(), lay = info.layout, r = lay.geo.r * lay.xf.s;
      return cells.map(function (c) {
        var cc = hx.center(lay.geo, c.col, c.row);
        var cx = lay.xf.x + cc[0] * lay.xf.s, cy = lay.xf.y + cc[1] * lay.xf.s, ring = [];
        for (var k = 0; k < 6; k++) {
          var t = Math.PI / 3 * k - Math.PI / 2;
          ring.push(ll({ x: cx + r * Math.cos(t), y: cy + r * Math.sin(t) }));
        }
        return [ring];
      });
    }

    function line(points, cls, dash) {
      var o = { renderer: renderer, interactive: false, className: cls, weight: cls === 'mp-ms-halo' || cls === 'mp-ms-sl-halo' ? 6 : 3, lineCap: 'round', lineJoin: 'round' };
      if (dash) o.dashArray = dash;
      return L.polyline(points, o);
    }

    function chip(p, text, total) {
      var span = el('span', 'mp-ms-chip' + (total ? ' mp-ms-tot' : ''), [text]);
      return L.marker(ll(p), {
        pane: 'mpMeasure', interactive: false, keyboard: false,
        icon: L.divIcon({ className: 'mp-ms-chipwrap', html: span, iconSize: null })
      });
    }

    function draw() {
      raf = 0;
      group.clearLayers();
      var a = path();
      if (a.length) {
        var hex = isHex();
        if (hex && a.length > 1 && a[0].h) {
          L.polygon(hexRings(hexCells(hexRoute(a.map(function (p) { return p.h; })))), {
            renderer: renderer, interactive: false, className: 'mp-ms-hexes', weight: 1.5
          }).addTo(group);
        }
        var fixed = (done ? a : a.slice(0, -1)).map(ll);
        if (fixed.length > 1) {
          line(fixed, 'mp-ms-halo').addTo(group);
          line(fixed, 'mp-ms-line').addTo(group);
        }
        if (!done && a.length > 1) {
          var seg = [ll(a[a.length - 2]), ll(a[a.length - 1])];
          line(seg, 'mp-ms-halo').addTo(group);
          line(seg, 'mp-ms-line', '7 6').addTo(group);
        }
        a.forEach(function (p, i) {
          if (!done && live && i === a.length - 1 && a.length > pts.length) return;
          L.circleMarker(ll(p), {
            renderer: renderer, interactive: false, className: 'mp-ms-pt', radius: i === 0 ? 6 : 4.5, weight: 2.5
          }).addTo(group);
        });
        // The distance so far at each turn, and the total at the end.
        if (a.length > 1 && measurable()) {
          var legs = legsOf(a), run = running(legs), sc = scale(), info = hexInfo();
          for (var j = 1; j < a.length; j++) {
            var text = hex ? hexChip(run[j - 1], info.perHex) : unitChip(run[j - 1], sc.unit);
            chip(a[j], text, j === a.length - 1).addTo(group);
          }
        }
      }
      // The scale line: a ruler with a tick across each end.
      if (sl) {
        var A = map.latLngToContainerPoint(ll(sl.a)), B = map.latLngToContainerPoint(ll(sl.b));
        var dx = B.x - A.x, dy = B.y - A.y, len = Math.hypot(dx, dy) || 1, nx = -dy / len * 7, ny = dx / len * 7;
        function back(x, y) { return map.containerPointToLatLng(L.point(x, y)); }
        var parts = [
          [ll(sl.a), ll(sl.b)],
          [back(A.x + nx, A.y + ny), back(A.x - nx, A.y - ny)],
          [back(B.x + nx, B.y + ny), back(B.x - nx, B.y - ny)]
        ];
        line(parts, 'mp-ms-sl-halo').addTo(group);
        line(parts, 'mp-ms-sl').addTo(group);
      }
      placePop();
    }
    function drawSoon() { if (!raf) raf = requestAnimationFrame(draw); }

    // ---- The hint bar ----

    function setHint(parts) {
      if (!hintEl) return;
      if (!parts) { hintEl.hidden = true; return; }
      while (hintEl.firstChild) hintEl.removeChild(hintEl.firstChild);
      parts.forEach(function (p) { hintEl.appendChild(typeof p === 'string' ? document.createTextNode(p) : p); });
      hintEl.hidden = false;
    }
    function hint() {
      if (!active) return;
      var t = touch();
      if (mode === 'scale') { setHint(pop ? null : ['Drag along something you know the length of ', kbd('Esc'), ' to cancel']); return; }
      if (!isHex() && !scale() && !canScale) { setHint(null); return; }
      if (done) setHint(t ? ['Tap to start a new path'] : ['Click to start a new path ', kbd('Esc'), ' to clear']);
      else if (pts.length) setHint(t ? ['Tap each turn, then Finish'] : ['Click each turn · double-click to finish ', kbd('Esc')]);
      else setHint(t ? ['Tap to start measuring'] : ['Click to start measuring ', kbd('Esc'), ' to stop']);
    }

    // ---- The panel ----

    function readout() {
      var a = path(), legs = legsOf(a), out = [];
      if (a.length < 2) {
        out.push(touch()
          ? el('p', '', ['Tap where the path starts, then each turn. Tap Finish when you’re done.'])
          : el('p', '', ['Click where the path starts, then each turn. Double-click or press ', kbd('Esc'), ' to finish.']));
        return out;
      }
      var box = el('div', 'mp-ms-readout');
      box.setAttribute('aria-live', 'polite');
      box.appendChild(el('span', 'mp-ms-lab', [done ? 'Total' : 'So far']));
      if (isHex()) {
        var info = hexInfo(), r = hexReadout(a.map(function (p) { return p.h; }), info.perHex, info.perDay);
        box.appendChild(el('b', '', [r.head]));
        box.appendChild(el('span', '', [r.about]));
      } else {
        var sc = scale();
        box.appendChild(el('b', '', [fmt(sum(legs)) + ' ' + UNITS[sc.unit][0]]));
        box.appendChild(el('span', '', [legs.length + (legs.length === 1 ? ' leg' : ' legs')]));
      }
      out.push(box);
      if (legs.length > 1) {
        var hex = isHex(), unit = hex ? '' : scale().unit;
        var list = el('div', 'mp-ms-legs', legs.map(function (n) {
          return el('span', 'mp-badge', [hex ? n + ' hx' : unitChip(n, unit)]);
        }));
        list.setAttribute('aria-label', 'Each leg');
        out.push(list);
      }
      return out;
    }

    function renderPanel() {
      if (!panel) return;
      if (!active) { panel.hidden = true; return; }
      var wasHidden = panel.hidden;
      var kids = [];
      var x = button('mp-ms-x', [icon('fa-xmark')], 'Stop measuring');
      x.addEventListener('click', function () { if (ctx.setTool) ctx.setTool('move'); });
      kids.push(el('div', 'mp-ms-head', [el('span', '', ['Measure']), x]));
      var sc = scale();
      if (mode === 'scale') {
        kids.push(el('p', '', ['Drag along something you know the length of, like a road or the scale bar drawn on the map. You’ll type its length next.']));
        var cancel = button('mp-cbtn', ['Cancel']);
        cancel.addEventListener('click', cancelScale);
        kids.push(el('div', 'mp-ms-row', [cancel]));
      } else if (!isHex() && !sc) {
        if (canScale) {
          var set = button('mp-cbtn mp-cbtn-primary', [icon('fa-ruler'), ' Set the scale']);
          set.addEventListener('click', startScale);
          kids.push(el('div', 'mp-ms-scalebox', [
            el('b', '', ['Set this map’s scale first']),
            el('p', '', ['Drag along something you know the length of, then type how long it is. You do this once; everyone who measures this map uses it.']),
            el('div', 'mp-ms-row', [set])
          ]));
        } else {
          kids.push(el('div', 'mp-ms-scalebox', [
            el('b', '', ['This map has no scale yet']),
            el('p', '', ['So distances can’t be shown. The owner or a member with DM access can set one; then measuring works here for everyone.'])
          ]));
        }
        if (pts.length > 1) {
          var clr = button('mp-cbtn', ['Clear the path']);
          clr.addEventListener('click', clear);
          kids.push(el('div', 'mp-ms-row', [clr]));
        }
      } else {
        readout().forEach(function (n) { kids.push(n); });
        var row = [];
        if (pts.length && !done) {
          var fin = button('mp-cbtn mp-cbtn-primary', [icon('fa-check'), ' Finish']);
          fin.addEventListener('click', finish);
          var und = button('mp-cbtn', [icon('fa-rotate-left'), ' Undo point']);
          und.addEventListener('click', undo);
          row.push(fin, und);
        }
        if (pts.length) {
          var c2 = button('mp-cbtn', ['Clear']);
          c2.addEventListener('click', clear);
          row.push(c2);
        }
        if (row.length) kids.push(el('div', 'mp-ms-row', row));
        if (isHex()) {
          var info = hexInfo();
          kids.push(el('p', 'mp-ms-note', ['Uses this map’s ' + info.perHex + ' miles a hex and ' + info.perDay + ' miles a day, the same figures as Plan a trip.']));
        } else {
          var note = el('p', 'mp-ms-note', ['Distances use the scale set for this map (' + UNITS[sc.unit][0] + ').']);
          if (canScale) {
            var ch = button('mp-ms-link', ['Change the scale']);
            ch.addEventListener('click', startScale);
            note.appendChild(document.createTextNode(' '));
            note.appendChild(ch);
          }
          kids.push(note);
        }
      }
      kids.push(el('div', 'mp-ms-private', [icon('fa-lock'), el('span', '', ['Only you see this. Nothing is saved.'])]));
      while (panel.firstChild) panel.removeChild(panel.firstChild);
      kids.forEach(function (k) { panel.appendChild(k); });
      panel.hidden = false;
      if (wasHidden) grow(panel);
    }

    function refresh() {
      // A path laid on hexes means nothing once the hexes go (and the reverse).
      if (pts.length && !!pts[0].h !== isHex()) { pts = []; live = null; done = false; }
      renderPanel();
      hint();
      draw();
    }

    // ---- Laying a path ----

    function add(p) {
      if (!onPicture(p)) return;
      var s = snap(p);
      if (!s) return;
      if (done) { pts = []; done = false; }
      pts.push(s);
      live = s;
      refresh();
    }

    // finish ends the path. The double-click that finishes it also clicked
    // twice on one spot, so points that land together on screen become one.
    function finish() {
      var kept = dropRepeats(pts.map(function (p) {
        var c = map.latLngToContainerPoint(ll(p));
        return { x: c.x, y: c.y, p: p };
      }), SAME_SPOT_PX).map(function (q) { return q.p; });
      pts = kept;
      done = pts.length > 1;
      if (!done) pts = [];
      live = null;
      refresh();
    }

    function undo() {
      if (done || !pts.length) return;
      pts.pop();
      if (!pts.length) live = null;
      refresh();
    }

    function clear() {
      pts = [];
      live = null;
      done = false;
      if (mode === 'scale') cancelScale(); else refresh();
    }

    function onClick(e) {
      if (!active || mode !== 'measure') return;
      add(fromLL(e.latlng));
    }
    // On a touch screen two quick taps at different turns would read as a
    // double-click and end the path early, so touch finishes with the button.
    function onDblClick() {
      if (lastPointer === 'touch') return;
      if (active && mode === 'measure' && pts.length && !done) finish();
    }
    function onMove(e) {
      if (!active || mode !== 'measure' || done || !pts.length) return;
      var s = snap(fromLL(e.latlng));
      if (!s || (live && live.x === s.x && live.y === s.y)) return;
      live = s;
      renderPanel();
      drawSoon();
    }

    // ---- Setting the scale (the owner or a member with DM access) ----

    function startScale() {
      if (!canScale) return;
      mode = 'scale';
      sl = null;
      pts = [];
      live = null;
      done = false;
      closePop();
      map.dragging.disable();
      refresh();
    }

    function cancelScale() {
      mode = 'measure';
      sl = null;
      dragging = false;
      closePop();
      map.dragging.enable();
      refresh();
    }

    function onPointerDown(e) {
      lastPointer = e.pointerType || '';
      if (!active || mode !== 'scale' || e.button !== 0) return;
      if (e.target.closest && e.target.closest('.leaflet-marker-icon, .leaflet-popup')) return;
      var p = clampPic(fromLL(map.mouseEventToLatLng(e)));
      closePop();
      sl = { a: p, b: p };
      dragging = true;
      try { container.setPointerCapture(e.pointerId); } catch (err) { /* capture is a nicety */ }
      e.preventDefault();
      hint();
      draw();
    }
    function onPointerMove(e) {
      if (!dragging) return;
      sl.b = clampPic(fromLL(map.mouseEventToLatLng(e)));
      drawSoon();
    }
    function onPointerUp() {
      if (!dragging) return;
      dragging = false;
      var A = map.latLngToContainerPoint(ll(sl.a)), B = map.latLngToContainerPoint(ll(sl.b));
      if (Math.hypot(B.x - A.x, B.y - A.y) < MIN_SCALE_PX) { sl = null; refresh(); return; }
      openPop();
    }

    function placePop() {
      if (!pop || !sl || !wrap) return;
      var B = map.latLngToContainerPoint(ll(sl.b)), r = wrap.getBoundingClientRect(), c = container.getBoundingClientRect();
      var bx = B.x + (c.left - r.left), by = B.y + (c.top - r.top);
      var x = Math.min(Math.max(8, bx + 12), r.width - pop.offsetWidth - 8);
      var y = Math.min(Math.max(8, by + 12), r.height - pop.offsetHeight - 8);
      pop.style.left = x + 'px';
      pop.style.top = y + 'px';
    }

    function closePop() {
      if (pop) { pop.remove(); pop = null; }
    }

    function openPop() {
      closePop();
      var sc = scale();
      var input = el('input', 'mp-input mp-ms-len');
      input.type = 'number';
      input.min = '0.1';
      input.max = String(MAX_LENGTH);
      input.step = 'any';
      input.value = '10';
      input.setAttribute('aria-label', 'Length');
      var sel = el('select', 'mp-input mp-ms-unit', UNIT_ORDER.map(function (u) {
        var o = el('option', '', [UNITS[u][0]]);
        o.value = u;
        return o;
      }));
      sel.value = sc && UNITS[sc.unit] ? sc.unit : 'miles';
      sel.setAttribute('aria-label', 'Unit');
      var save = button('mp-cbtn mp-cbtn-primary', ['Save scale']);
      var again = button('mp-cbtn', ['Draw again']);
      pop = el('div', 'mp-fl mp-ms-pop', [
        el('b', '', ['How long is that line?']),
        el('div', 'mp-ms-row', [input, sel]),
        el('small', '', ['Saved on this map for everyone who measures it.']),
        el('div', 'mp-ms-row', [save, again])
      ]);
      pop.setAttribute('role', 'dialog');
      pop.setAttribute('aria-label', 'How long is that line?');
      wrap.appendChild(pop);
      placePop();
      grow(pop);
      hint();
      input.focus();
      input.select();

      function doSave() {
        if (saving) return;
        var body = scaleBody(
          { x: sl.a.x / W * 100, y: sl.a.y / H * 100 },
          { x: sl.b.x / W * 100, y: sl.b.y / H * 100 },
          input.value, sel.value);
        if (!body) { input.setAttribute('aria-invalid', 'true'); input.focus(); return; }
        input.removeAttribute('aria-invalid');
        saving = true;
        save.disabled = true;
        Chronicle.apiFetch(scaleURL, { method: 'PUT', body: { scale: body } }).then(function (resp) {
          return resp.json().catch(function () { return {}; }).then(function (data) {
            if (!resp.ok) { Chronicle.notify(data.message || 'The scale could not be saved', 'error'); return; }
            if (ctx.onMeasureScale) ctx.onMeasureScale(data.scale || body);
            mode = 'measure';
            sl = null;
            closePop();
            map.dragging.enable();
            refresh();
            Chronicle.notify('Scale saved. Everyone who measures ' + (ctx.mapName || 'this map') + ' now uses it.', 'success');
          });
        }).catch(function () {
          Chronicle.notify('The scale could not be saved', 'error');
        }).then(function () {
          saving = false;
          save.disabled = false;
        });
      }
      save.addEventListener('click', doSave);
      input.addEventListener('keydown', function (e) { if (e.key === 'Enter') { e.preventDefault(); doSave(); } });
      again.addEventListener('click', function () { closePop(); sl = null; refresh(); });
      // Clicks inside the box are not clicks on the map.
      pop.addEventListener('pointerdown', function (e) { e.stopPropagation(); });
    }

    function onView() { if (sl || pts.length) drawSoon(); else placePop(); }

    map.on('click', onClick);
    map.on('dblclick', onDblClick);
    map.on('mousemove', onMove);
    map.on('zoomend moveend resize', onView);
    container.addEventListener('pointerdown', onPointerDown);
    container.addEventListener('pointermove', onPointerMove);
    container.addEventListener('pointerup', onPointerUp);
    container.addEventListener('pointercancel', onPointerUp);
    if (coarseQuery && coarseQuery.addEventListener) coarseQuery.addEventListener('change', function () { if (active) { renderPanel(); hint(); } });

    // setActive turns the tool on or off. A finished path stays on the map
    // after the tool is put down; an unfinished one and the scale line go.
    function setActive(on) {
      on = !!on;
      if (on === active) { if (on) refresh(); return; }
      active = on;
      if (!on) {
        if (mode === 'scale') { mode = 'measure'; sl = null; dragging = false; map.dragging.enable(); }
        closePop();
        live = null;
        if (!done) pts = [];
        container.style.cursor = '';
        if (panel) panel.hidden = true;
        draw();
        return;
      }
      container.style.cursor = 'crosshair';
      refresh();
    }

    var handle = {
      setActive: setActive,
      // addPoint takes a point from elsewhere (a click on a pin) as a turn.
      addPoint: function (latlng) { if (active && mode === 'measure') add(fromLL(latlng)); },
      // escape steps back: out of setting the scale, then finishes the path,
      // then clears it. False when there was nothing to step back from.
      escape: function () {
        if (!active) return false;
        if (mode === 'scale') { cancelScale(); return true; }
        if (pts.length && !done) { finish(); return true; }
        if (done) { clear(); return true; }
        return false;
      },
      // key handles Backspace, which drops the last point of an unfinished path.
      key: function (e) {
        if (!active || e.key !== 'Backspace' || mode !== 'measure' || done || !pts.length) return false;
        e.preventDefault();
        undo();
        return true;
      },
      // refresh redraws after something the readout depends on changed (the
      // scale, the hexes, the travel figures).
      refresh: function () { if (active || pts.length) refresh(); },
      destroy: function () {
        if (raf) cancelAnimationFrame(raf);
        map.off('click', onClick);
        map.off('dblclick', onDblClick);
        map.off('mousemove', onMove);
        map.off('zoomend moveend resize', onView);
        container.removeEventListener('pointerdown', onPointerDown);
        container.removeEventListener('pointermove', onPointerMove);
        container.removeEventListener('pointerup', onPointerUp);
        container.removeEventListener('pointercancel', onPointerUp);
        closePop();
        try { group.remove(); } catch (e) { /* map already gone */ }
        if (panel) panel.hidden = true;
      }
    };
    ctx.measure = handle;
    return handle;
  }

  window.ChronicleMapMeasure = { init: attach };
})();
