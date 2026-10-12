/**
 * map_shapes.js -- diagram-style shapes on a Chronicle map: rooms, areas,
 * water and walls (boxes), arrows and speech bubbles, which the DM places from
 * the Shapes palette and then selects, moves, resizes, renames and deletes.
 *
 * Every shape is an ordinary drawing (see internal/plugins/maps/drawing_annotation.go):
 *   room | area | water | wall   points [top-left, bottom-right] in map percent;
 *                                text_content is the optional name (a wall has none)
 *   arrow                        points [tail, tip]
 *   callout                      points [where the bubble's tail points]; text_content is its text
 * What a box looks like is decided here, from theme tokens, so it follows the
 * light and dark themes; the stored colours are not used for the four boxes.
 * The server validates and withholds shapes like any drawing (DM-only ones
 * never reach players), so this file is only what is on screen. User text is
 * only ever set with textContent or an input's value.
 *
 * The pure geometry lives at the top and is exported for tests; the browser
 * part needs Leaflet and the drawing module (map_drawing_tools.js), which owns
 * the layers and the API calls and hands this module what it needs.
 */
(function () {
  'use strict';

  var BOX_TYPES = ['room', 'area', 'water', 'wall'];
  var SHAPE_TYPES = BOX_TYPES.concat(['arrow', 'callout']);
  // Mirrors minBoxExtent on the server: the smallest width or height, in map
  // percent, a box may have.
  var MIN_EXTENT = 0.5;
  var MAX_TEXT = 500;
  // The palette, in the signed design's order. key is what the rail sends,
  // type the drawing_type it creates.
  var KINDS = [
    { key: 'room', type: 'room', label: 'Room', hint: 'Drag out a room, or click to drop one' },
    { key: 'area', type: 'area', label: 'Area', hint: 'Drag out an area, or click to drop one' },
    { key: 'water', type: 'water', label: 'Water', hint: 'Drag out water, or click to drop some' },
    { key: 'wall', type: 'wall', label: 'Wall', hint: 'Drag out a wall, or click to drop one' },
    { key: 'arrow', type: 'arrow', label: 'Arrow', hint: 'Drag from the tail to the tip' },
    { key: 'bubble', type: 'callout', label: 'Bubble', hint: 'Click where the speech bubble points' }
  ];
  // Size of a click-placed shape, in grid cells (width, height).
  var DEFAULT_CELLS = { room: [3, 2], area: [4, 3], water: [4, 3], wall: [4, 0.5] };
  var HANDLES = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w'];
  // The colour a box is saved with. The viewer ignores it for these kinds, but
  // the record carries a valid hex like every drawing.
  var BOX_STROKE = '#334155';

  // ---- Pure geometry ----

  function clamp(v, lo, hi) { return Math.min(hi, Math.max(lo, v)); }
  function round3(v) { return Math.round(v * 1000) / 1000; }
  function isNum(v) { return typeof v === 'number' && isFinite(v); }

  function isBoxType(t) { return BOX_TYPES.indexOf(t) !== -1; }
  function isShapeType(t) { return SHAPE_TYPES.indexOf(t) !== -1; }

  function kindOf(key) {
    for (var i = 0; i < KINDS.length; i++) if (KINDS[i].key === key) return KINDS[i];
    return null;
  }

  // typeLabel is the word for a shape in the bar and in screen-reader labels.
  function typeLabel(t) {
    switch (t) {
      case 'room': return 'Room';
      case 'area': return 'Area';
      case 'water': return 'Water';
      case 'wall': return 'Wall';
      case 'arrow': return 'Arrow';
      case 'callout': return 'Speech bubble';
    }
    return 'Shape';
  }

  // fieldLabel names the text field a shape has, or '' for one with no text.
  function fieldLabel(t) {
    if (t === 'callout') return 'Bubble text';
    if (t === 'room' || t === 'area' || t === 'water') return 'Name';
    return '';
  }

  // gridStep is one grid cell in map percent on each axis. Squares are
  // measured as if the map were 1000 wide (see the viewer's grid), so a cell
  // is the same share of the width on every map. Shapes snap to it whether or
  // not the grid is drawn, so a blank map still lays out neatly.
  function gridStep(gridSize, mapW, mapH) {
    var size = isNum(gridSize) && gridSize > 0 ? gridSize : 50;
    var w = mapW > 0 ? mapW : 1000;
    var h = mapH > 0 ? mapH : 1000;
    var cell = size * w / 1000;
    return { x: Math.max(0.5, round3(cell / w * 100)), y: Math.max(0.5, round3(cell / h * 100)) };
  }

  // stepFor is the snap step for a kind: a wall is thinner than a cell, so it
  // snaps to half of one.
  function stepFor(type, step) {
    return type === 'wall' ? { x: step.x / 2, y: step.y / 2 } : step;
  }

  function snap(v, step, on) {
    if (!on || !(step > 0)) return round3(v);
    return round3(Math.round(v / step) * step);
  }

  // boxOf reads a box's two corners as a sorted rectangle, or null when the
  // points are not exactly two usable coordinate pairs.
  function boxOf(points) {
    if (!Array.isArray(points) || points.length !== 2) return null;
    var a = points[0], b = points[1];
    if (!a || !b || !isNum(a.x) || !isNum(a.y) || !isNum(b.x) || !isNum(b.y)) return null;
    return { x1: Math.min(a.x, b.x), y1: Math.min(a.y, b.y), x2: Math.max(a.x, b.x), y2: Math.max(a.y, b.y) };
  }

  function pointsOfBox(b) {
    return [{ x: round3(b.x1), y: round3(b.y1) }, { x: round3(b.x2), y: round3(b.y2) }];
  }

  // minSize is the smallest a box of this kind may be dragged to: one snap
  // step, never under the server's floor.
  function minSize(type, step) {
    var s = stepFor(type, step);
    return { w: Math.max(MIN_EXTENT, s.x), h: Math.max(MIN_EXTENT, s.y) };
  }

  // placeBox turns a drag from a to b (map percent) into the new box. A click
  // (tiny) drops the kind's default size with its top-left at the click. A
  // drag is snapped and grown to the minimum size toward the side it was
  // dragged to, then kept on the map.
  function placeBox(type, a, b, tiny, step, snapOn) {
    var s = stepFor(type, step);
    var min = minSize(type, step);
    var cells = DEFAULT_CELLS[type] || [2, 2];
    var x1, y1, x2, y2;
    if (tiny) {
      x1 = snap(a.x, s.x, snapOn); y1 = snap(a.y, s.y, snapOn);
      x2 = x1 + cells[0] * step.x; y2 = y1 + cells[1] * step.y;
    } else {
      var ax = snap(a.x, s.x, snapOn), ay = snap(a.y, s.y, snapOn);
      var bx = snap(b.x, s.x, snapOn), by = snap(b.y, s.y, snapOn);
      x1 = Math.min(ax, bx); x2 = Math.max(ax, bx);
      y1 = Math.min(ay, by); y2 = Math.max(ay, by);
      if (x2 - x1 < min.w) { if (b.x < a.x) x1 = x2 - min.w; else x2 = x1 + min.w; }
      if (y2 - y1 < min.h) { if (b.y < a.y) y1 = y2 - min.h; else y2 = y1 + min.h; }
    }
    return fitBox({ x1: x1, y1: y1, x2: x2, y2: y2 });
  }

  // fitBox slides a box back onto the map without changing its size.
  function fitBox(b) {
    var w = Math.min(100, b.x2 - b.x1), h = Math.min(100, b.y2 - b.y1);
    var x1 = clamp(b.x1, 0, 100 - w), y1 = clamp(b.y1, 0, 100 - h);
    return { x1: round3(x1), y1: round3(y1), x2: round3(x1 + w), y2: round3(y1 + h) };
  }

  // moveBox moves a box by a drag delta, snapping its top-left corner and
  // keeping the whole box on the map.
  function moveBox(b, dx, dy, step, snapOn) {
    var w = b.x2 - b.x1, h = b.y2 - b.y1;
    var x1 = clamp(snap(b.x1 + dx, step.x, snapOn), 0, 100 - w);
    var y1 = clamp(snap(b.y1 + dy, step.y, snapOn), 0, 100 - h);
    return { x1: round3(x1), y1: round3(y1), x2: round3(x1 + w), y2: round3(y1 + h) };
  }

  // resizeBox drags the named handle (n, s, e, w or a corner such as se) to a
  // map point. The opposite edges stay put and the box never shrinks under min
  // or leaves the map.
  function resizeBox(b, handle, p, step, snapOn, min) {
    var n = { x1: b.x1, y1: b.y1, x2: b.x2, y2: b.y2 };
    if (handle.indexOf('w') !== -1) n.x1 = clamp(snap(p.x, step.x, snapOn), 0, n.x2 - min.w);
    if (handle.indexOf('e') !== -1) n.x2 = clamp(snap(p.x, step.x, snapOn), n.x1 + min.w, 100);
    if (handle.indexOf('n') !== -1) n.y1 = clamp(snap(p.y, step.y, snapOn), 0, n.y2 - min.h);
    if (handle.indexOf('s') !== -1) n.y2 = clamp(snap(p.y, step.y, snapOn), n.y1 + min.h, 100);
    return { x1: round3(n.x1), y1: round3(n.y1), x2: round3(n.x2), y2: round3(n.y2) };
  }

  // shiftPoints moves every point by a drag delta. The first point is snapped,
  // the rest keep their distance from it, and the whole shape stays on the map.
  function shiftPoints(points, dx, dy, step, snapOn) {
    if (!Array.isArray(points) || !points.length) return points;
    var minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
    points.forEach(function (p) {
      minX = Math.min(minX, p.x); maxX = Math.max(maxX, p.x);
      minY = Math.min(minY, p.y); maxY = Math.max(maxY, p.y);
    });
    var sx = snap(points[0].x + dx, step.x, snapOn) - points[0].x;
    var sy = snap(points[0].y + dy, step.y, snapOn) - points[0].y;
    sx = clamp(sx, -minX, 100 - maxX);
    sy = clamp(sy, -minY, 100 - maxY);
    return points.map(function (p) { return { x: round3(p.x + sx), y: round3(p.y + sy) }; });
  }

  // moveEnd drags one end of an arrow (0 tail, 1 tip) to a map point. It
  // returns null when that would put both ends on the same spot.
  function moveEnd(points, which, p, step, snapOn) {
    var q = { x: clamp(snap(p.x, step.x, snapOn), 0, 100), y: clamp(snap(p.y, step.y, snapOn), 0, 100) };
    var other = points[which === 0 ? 1 : 0];
    if (Math.abs(q.x - other.x) < 1e-6 && Math.abs(q.y - other.y) < 1e-6) return null;
    var out = [points[0], points[1]];
    out[which] = { x: round3(q.x), y: round3(q.y) };
    return out;
  }

  // placeArrow turns a drag from a to b into an arrow's two ends. A click
  // drops a short arrow pointing right (or left, at the right edge).
  function placeArrow(a, b, tiny, step, snapOn) {
    var tail = { x: clamp(snap(a.x, step.x, snapOn), 0, 100), y: clamp(snap(a.y, step.y, snapOn), 0, 100) };
    var tip;
    if (tiny) {
      var len = 2 * step.x;
      tip = { x: tail.x + len <= 100 ? tail.x + len : tail.x - len, y: tail.y };
    } else {
      tip = { x: clamp(snap(b.x, step.x, snapOn), 0, 100), y: clamp(snap(b.y, step.y, snapOn), 0, 100) };
      if (Math.abs(tip.x - tail.x) < 1e-6 && Math.abs(tip.y - tail.y) < 1e-6) {
        tip = { x: tail.x + step.x <= 100 ? tail.x + step.x : tail.x - step.x, y: tail.y };
      }
    }
    return [{ x: round3(tail.x), y: round3(tail.y) }, { x: round3(tip.x), y: round3(tip.y) }];
  }

  // placePoint is where a bubble's tail points: the click, snapped.
  function placePoint(p, step, snapOn) {
    return [{ x: clamp(snap(p.x, step.x, snapOn), 0, 100), y: clamp(snap(p.y, step.y, snapOn), 0, 100) }];
  }

  var ARROW_KEYS = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] };

  // nudgePoints is the keyboard move: an arrow key moves the shape one step;
  // with Shift it instead resizes a box from its bottom-right corner, or moves
  // an arrow's tip. Returns the new points, or null for no change.
  function nudgePoints(d, key, shift, step) {
    var dir = ARROW_KEYS[key];
    if (!dir || !d || !Array.isArray(d.points)) return null;
    var s = stepFor(d.drawing_type, step);
    var dx = dir[0] * s.x, dy = dir[1] * s.y;
    var out = null;
    if (isBoxType(d.drawing_type)) {
      var b = boxOf(d.points);
      if (!b) return null;
      out = pointsOfBox(shift ? resizeBox(b, 'se', { x: b.x2 + dx, y: b.y2 + dy }, s, false, minSize(d.drawing_type, step)) : moveBox(b, dx, dy, s, false));
    } else if (d.drawing_type === 'arrow' && shift) {
      out = moveEnd(d.points, 1, { x: d.points[1].x + dx, y: d.points[1].y + dy }, s, false);
    } else {
      out = shiftPoints(d.points, dx, dy, s, false);
    }
    if (!out || JSON.stringify(out) === JSON.stringify(d.points)) return null;
    return out;
  }

  // handlePos is where a box handle sits, as a map point.
  function handlePos(b, h) {
    var x = h.indexOf('w') !== -1 ? b.x1 : h.indexOf('e') !== -1 ? b.x2 : (b.x1 + b.x2) / 2;
    var y = h.indexOf('n') !== -1 ? b.y1 : h.indexOf('s') !== -1 ? b.y2 : (b.y1 + b.y2) / 2;
    return { x: x, y: y };
  }

  // handlesFor lists the handles a box shows. The edge handles go away on a
  // box too small on screen to tell them from the corners.
  function handlesFor(pxW, pxH) {
    return HANDLES.filter(function (h) {
      if (h.length === 2) return true;
      if (h === 'n' || h === 's') return pxW >= 56;
      return pxH >= 56;
    });
  }

  // bodyFor builds the create request for a shape the palette placed. Colours
  // come from the rail for arrows and bubbles (as the Draw tools do); boxes
  // carry a fixed valid one.
  function bodyFor(key, points, rail) {
    var k = kindOf(key);
    if (!k) return null;
    var o = { type: k.type, points: points };
    if (isBoxType(k.type)) {
      o.opts = { stroke_color: BOX_STROKE, stroke_width: 2 };
      if (k.type === 'room') o.opts.text_content = 'Room';
    } else if (k.type === 'callout') {
      o.opts = { stroke_color: rail && rail.color, stroke_width: 2, text_content: 'Note' };
    } else {
      o.opts = { stroke_color: rail && rail.color, stroke_width: rail && rail.width };
    }
    return o;
  }

  var pure = {
    BOX_TYPES: BOX_TYPES, SHAPE_TYPES: SHAPE_TYPES, KINDS: KINDS, HANDLES: HANDLES, MIN_EXTENT: MIN_EXTENT,
    MAX_TEXT: MAX_TEXT, BOX_STROKE: BOX_STROKE,
    isBoxType: isBoxType, isShapeType: isShapeType, kindOf: kindOf, typeLabel: typeLabel, fieldLabel: fieldLabel,
    gridStep: gridStep, stepFor: stepFor, snap: snap, boxOf: boxOf, pointsOfBox: pointsOfBox, minSize: minSize,
    placeBox: placeBox, fitBox: fitBox, moveBox: moveBox, resizeBox: resizeBox, shiftPoints: shiftPoints,
    moveEnd: moveEnd, placeArrow: placeArrow, placePoint: placePoint, nudgePoints: nudgePoints,
    handlePos: handlePos, handlesFor: handlesFor, bodyFor: bodyFor
  };

  // ---- Browser-only part ----

  if (typeof window === 'undefined' || typeof document === 'undefined') {
    if (typeof module !== 'undefined' && module.exports) module.exports = pure;
    return;
  }

  var styleAdded = false;
  function addStyle() {
    if (styleAdded || document.getElementById('mp-shapes-style')) { styleAdded = true; return; }
    styleAdded = true;
    var s = document.createElement('style');
    s.id = 'mp-shapes-style';
    // Colours are theme tokens with a fallback, so a shape follows light and
    // dark. The accent is the map page's own; the dashboard widget has none and
    // falls back to the site accent.
    s.textContent = [
      '.mp-sheet { fill: var(--color-card-bg, #fff); fill-opacity: 1; stroke: var(--color-border, #d1d5db); stroke-width: 1px; vector-effect: non-scaling-stroke; }',
      '.mp-shp { vector-effect: non-scaling-stroke; }',
      '.mp-shp-room { fill: var(--color-card-bg, #fff); fill-opacity: 0.92; stroke: var(--color-text-primary, #111827); stroke-width: 2px; }',
      '.mp-shp-area { fill: var(--mp-accent, rgb(var(--color-accent-rgb, 99 102 241))); fill-opacity: 0.14; stroke: var(--mp-accent, rgb(var(--color-accent-rgb, 99 102 241))); stroke-width: 1.5px; stroke-dasharray: 6 4; }',
      '.mp-shp-water { fill: rgb(59 130 246); fill-opacity: 0.3; stroke: rgb(59 130 246); stroke-width: 1.5px; }',
      '.mp-shp-wall { fill: var(--color-text-primary, #111827); fill-opacity: 0.92; stroke: none; }',
      '.mp-shp-hit { stroke-linecap: round; }',
      '.mp-shp-live { cursor: pointer; }',
      '.mp-shp-live:focus-visible { outline: 2px solid var(--mp-accent, #6366f1); outline-offset: 2px; }',
      '.mp-shp-selected { cursor: move; }',
      '.mp-shp-lbl-icon { background: none; border: 0; pointer-events: none; }',
      '.mp-shp-lbl { position: absolute; left: 0; top: 0; box-sizing: border-box; padding: 3px 6px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 13px/1.3 system-ui, sans-serif; color: var(--color-text-primary, #111827); text-shadow: 0 0 3px var(--color-card-bg, #fff), 0 0 3px var(--color-card-bg, #fff); pointer-events: none; }',
      '.mp-shp-sel { fill: none; stroke: var(--mp-accent, #6366f1); stroke-width: 2px; stroke-dasharray: 5 3; vector-effect: non-scaling-stroke; pointer-events: none; }',
      '.mp-shp-h-icon { background: none; border: 0; }',
      '.mp-shp-h { box-sizing: border-box; width: 14px; height: 14px; border-radius: 4px; background: var(--color-card-bg, #fff); border: 2px solid var(--mp-accent, #6366f1); touch-action: none; }',
      '.mp-shp-h[data-handle="mv"], .mp-shp-h[data-handle="a"], .mp-shp-h[data-handle="b"] { border-radius: 50%; cursor: move; }',
      '.mp-shp-h[data-handle="nw"], .mp-shp-h[data-handle="se"] { cursor: nwse-resize; }',
      '.mp-shp-h[data-handle="ne"], .mp-shp-h[data-handle="sw"] { cursor: nesw-resize; }',
      '.mp-shp-h[data-handle="n"], .mp-shp-h[data-handle="s"] { cursor: ns-resize; }',
      '.mp-shp-h[data-handle="e"], .mp-shp-h[data-handle="w"] { cursor: ew-resize; }',
      '.mp-shpbar { left: 50%; bottom: 16px; transform: translateX(-50%); display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 6px 12px; padding: 8px 12px; max-width: calc(100% - 24px); font-size: 12px; animation: mp-shp-in var(--dur-standard, 200ms) var(--ease-out, cubic-bezier(0.16, 1, 0.3, 1)) both; }',
      '.mp-shpbar[hidden] { display: none; }',
      '.mp-shpbar [hidden] { display: none !important; }',
      '.mp-shpbar label { display: flex; align-items: center; gap: 6px; color: var(--color-text-secondary); }',
      '.mp-shpbar input[type=text], .mp-shpbar textarea { font: inherit; font-size: 13px; width: 220px; max-width: 100%; padding: 4px 8px; border: 1px solid var(--color-border); border-radius: 6px; background: var(--color-card-bg); color: var(--color-text-primary); resize: none; }',
      '.mp-shpbar button { border: 1px solid var(--color-border); background: var(--color-card-bg); border-radius: 7px; padding: 4px 10px; font-size: 12px; color: var(--color-text-body); }',
      '.mp-shpbar button:hover { background: var(--color-bg-tertiary); }',
      '.mp-shpbar .mp-shp-del { color: var(--color-danger-strong, #dc2626); border-color: var(--color-danger-strong, #dc2626); }',
      '.mp-shpbar .mp-shp-hint { order: 9; flex-basis: 100%; text-align: center; color: var(--color-text-secondary); }',
      '@keyframes mp-shp-in { from { opacity: 0; transform: translateX(-50%) translateY(6px); } to { opacity: 1; transform: translateX(-50%); } }',
      '@keyframes mp-shp-fade { from { opacity: 0; } to { opacity: 1; } }',
      '.mp-shp-in { animation: mp-shp-fade var(--dur-standard, 200ms) var(--ease-out, cubic-bezier(0.16, 1, 0.3, 1)) both; }',
      '@media (prefers-reduced-motion: reduce) { .mp-shpbar, .mp-shp-in { animation: none; } }'
    ].join('\n');
    document.head.appendChild(s);
  }

  function el(tag, cls, attrs) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (attrs) for (var k in attrs) if (Object.prototype.hasOwnProperty.call(attrs, k)) e.setAttribute(k, attrs[k]);
    return e;
  }

  function labelPane(map) {
    var pane = map.getPane('mpShapeLabels');
    if (!pane) {
      pane = map.createPane('mpShapeLabels');
      pane.style.zIndex = 408;
      pane.style.pointerEvents = 'none';
    }
    return 'mpShapeLabels';
  }

  // renderBox returns the Leaflet layer for a room, area, water or wall: the
  // rectangle, plus its name inside the top-left corner when it has one and the
  // box is big enough on screen to show it. env: { map, toLatLng, interactive,
  // animate }. Returns null for a box too damaged to draw.
  function renderBox(d, env) {
    addStyle();
    var b = boxOf(d && d.points);
    if (!b || !isBoxType(d.drawing_type)) return null;
    var tl = env.toLatLng({ x: b.x1, y: b.y1 });
    var br = env.toLatLng({ x: b.x2, y: b.y2 });
    var rect = L.rectangle([tl, br], {
      className: 'mp-shp mp-shp-' + d.drawing_type,
      interactive: env.interactive !== false,
      // Colours come from the stylesheet; these only apply where it is absent.
      color: BOX_STROKE, weight: d.drawing_type === 'wall' ? 0 : 2, fillOpacity: 0.3
    });
    var parts = [rect];
    var name = d.text_content == null ? '' : String(d.text_content);
    var label = null, lbl = null;
    if (d.drawing_type !== 'wall' && name.trim()) {
      lbl = el('div', 'mp-shp-lbl');
      lbl.textContent = name;
      var fs = Number(d.font_size);
      if (fs >= 8 && fs <= 48) lbl.style.fontSize = fs + 'px';
      label = L.marker(tl, {
        pane: labelPane(env.map),
        icon: L.divIcon({ className: 'mp-shp-lbl-icon', html: lbl, iconSize: [0, 0], iconAnchor: [0, 0] }),
        interactive: false, keyboard: false
      });
      label._mpSkip = true;
      parts.push(label);
    }
    var group = L.featureGroup(parts);
    var map = null;
    // The label takes the box's size on screen, and hides when there is no room.
    function fit() {
      if (!map || !lbl) return;
      var a = map.latLngToLayerPoint(tl), c = map.latLngToLayerPoint(br);
      var w = Math.abs(c.x - a.x), h = Math.abs(c.y - a.y);
      lbl.style.width = Math.max(0, w) + 'px';
      lbl.style.height = Math.max(0, h) + 'px';
      lbl.style.display = w < 40 || h < 22 ? 'none' : 'block';
    }
    group.on('add', function (e) {
      if (e.target !== group) return;
      map = env.map;
      map.on('zoom viewreset', fit);
      fit();
      if (env.animate && rect.getElement()) {
        rect.getElement().classList.add('mp-shp-in');
        if (lbl) lbl.classList.add('mp-shp-in');
      }
    });
    group.on('remove', function (e) {
      if (e.target !== group) return;
      if (map) map.off('zoom viewreset', fit);
      map = null;
    });
    return group;
  }

  // ---- The controller ----

  // attach adds select, move, resize, rename and delete to the shapes the
  // drawing module draws. opts:
  //   toLatLng(pt), toPercent(latlng)   map percent <-> Leaflet
  //   step()                            one grid cell in map percent {x, y}
  //   canEdit(d), canDelete(d)          the server's rule for this viewer
  //   canDmOnly                         whether the viewer may hide shapes from players
  //   replace(d)                        re-draw a drawing from new data
  //   patch(id, fields)                 PUT only the changed fields -> Promise<bool>
  //   remove(id, latlng)                ask to delete (the drawing module's confirm)
  //   beforeSelect()                    drop any other selection (a picture)
  function attach(map, opts) {
    addStyle();
    var items = {};
    var selectedId = null;
    var live = false;
    var destroyed = false;
    var replacing = null;
    var dragging = false;
    var lastDragEnd = 0;
    var ui = null;
    var bar = null, refs = null;
    var textBase = null;
    var nudgeBase = null, nudgeTimer = null;

    if (!map.getPane('mpShapeUI')) {
      var pane = map.createPane('mpShapeUI');
      pane.style.zIndex = 620;
    }

    function clone(o) { return JSON.parse(JSON.stringify(o)); }
    function step() { var s = opts.step ? opts.step() : null; return s && s.x > 0 && s.y > 0 ? s : { x: 5, y: 5 }; }
    function pctOf(e) {
      var t = e.touches && e.touches[0] ? e.touches[0] : (e.changedTouches && e.changedTouches[0]) || e;
      return opts.toPercent(map.mouseEventToLatLng({ clientX: t.clientX, clientY: t.clientY }));
    }
    function pointOf(e) {
      var t = e.touches && e.touches[0] ? e.touches[0] : (e.changedTouches && e.changedTouches[0]) || e;
      return { x: t.clientX, y: t.clientY };
    }

    // ---- wiring ----

    function describe(item) {
      var d = item.d;
      var name = d.text_content ? ': ' + d.text_content : '';
      return typeLabel(d.drawing_type) + name + '. Press Enter to select it.';
    }

    function applyAria(item) {
      var on = live && opts.canEdit(item.d);
      item.els.forEach(function (e) {
        if (on) {
          e.setAttribute('tabindex', '0');
          e.setAttribute('role', 'button');
          e.setAttribute('aria-label', describe(item));
          e.setAttribute('aria-pressed', selectedId === item.id ? 'true' : 'false');
        } else {
          e.removeAttribute('tabindex');
          e.removeAttribute('role');
          e.removeAttribute('aria-label');
          e.removeAttribute('aria-pressed');
        }
        e.classList.toggle('mp-shp-live', !!on);
        e.classList.toggle('mp-shp-selected', !!on && selectedId === item.id);
      });
    }

    function gather(layer, out) {
      if (layer._mpSkip) return;
      if (layer.getElement) { var e = layer.getElement(); if (e) out.push(e); }
      if (layer.eachLayer) layer.eachLayer(function (l) { gather(l, out); });
    }

    // collect finds the layer's elements once it is on the page and gives them
    // their press and key handlers.
    function collect(item) {
      var els = [];
      gather(item.layer, els);
      item.els = els;
      els.forEach(function (e) {
        if (e.__mpShp) return;
        e.__mpShp = true;
        L.DomEvent.on(e, 'mousedown touchstart', function (ev) { onPress(ev, item); });
        e.addEventListener('keydown', function (ev) {
          if ((ev.key === 'Enter' || ev.key === ' ') && live && opts.canEdit(item.d)) {
            ev.preventDefault();
            ev.stopPropagation();
            select(item.id);
          }
        });
      });
      applyAria(item);
    }

    // An unselected shape lets the map pan under a press and only a clean click
    // picks it, so a large area never traps the hand; a selected one is dragged.
    function onPress(ev, item) {
      if (!live || selectedId !== item.id || !opts.canEdit(item.d)) return;
      if (ev.type === 'mousedown' && ev.button !== 0) return;
      L.DomEvent.stopPropagation(ev);
      L.DomEvent.preventDefault(ev);
      beginMove(ev, item.id);
    }

    function addArrowHit(layer, d) {
      var pts = d.points || [];
      if (pts.length < 2) return null;
      var hit = L.polyline([opts.toLatLng(pts[0]), opts.toLatLng(pts[1])], {
        className: 'mp-shp-hit', weight: 18, opacity: 0, interactive: true, lineCap: 'round'
      });
      layer.addLayer(hit);
      return hit;
    }

    function wire(layer, d) {
      if (destroyed || !d || !isShapeType(d.drawing_type)) return;
      var item = { id: d.id, d: d, layer: layer, els: [] };
      items[d.id] = item;
      if (d.drawing_type === 'arrow' && opts.canEdit(d) && layer.addLayer) addArrowHit(layer, d);
      layer.on('add', function (e) { if (!destroyed) collect(item); });
      layer.on('remove', function (e) {
        if (e.target !== layer || replacing === item.id) return;
        item.els = [];
        if (selectedId === item.id) deselect();
      });
      layer.on('click', function (e) {
        if (!live || !opts.canEdit(item.d)) return;
        L.DomEvent.stopPropagation(e);
        if (selectedId !== item.id) select(item.id);
      });
      if (layer._map) collect(item);
    }

    function unwire(id) {
      if (selectedId === id) deselect();
      delete items[id];
    }

    function reset() {
      deselect();
      items = {};
    }

    // ---- selection ----

    function select(id, o) {
      var it = items[id];
      if (!it || !opts.canEdit(it.d)) return false;
      if (selectedId !== id) {
        if (opts.beforeSelect) opts.beforeSelect();
        flushNudge();
        var prev = selectedId ? items[selectedId] : null;
        selectedId = id;
        if (prev) applyAria(prev);
        applyAria(it);
        showSelection();
        syncBar();
      }
      if (o && o.focus) {
        // A shape just placed by a drag is selected while the button that
        // ended the drag is still coming up; its click is not a click away.
        lastDragEnd = Date.now();
        focusField();
      }
      return true;
    }

    function deselect() {
      var was = selectedId !== null;
      flushNudge();
      var prev = selectedId ? items[selectedId] : null;
      selectedId = null;
      textBase = null;
      clearUI();
      if (bar) { bar.hidden = true; refs.hintFor = null; }
      if (prev) applyAria(prev);
      return was;
    }

    // ---- the selection's outline and handles ----

    function clearUI() {
      if (ui && ui.group) map.removeLayer(ui.group);
      ui = null;
    }

    function boundsOf(b) { return L.latLngBounds(opts.toLatLng({ x: b.x1, y: b.y1 }), opts.toLatLng({ x: b.x2, y: b.y2 })); }

    function pxSize(b) {
      var a = map.latLngToLayerPoint(opts.toLatLng({ x: b.x1, y: b.y1 }));
      var c = map.latLngToLayerPoint(opts.toLatLng({ x: b.x2, y: b.y2 }));
      return { w: Math.abs(c.x - a.x), h: Math.abs(c.y - a.y) };
    }

    function addHandle(name, pt) {
      var m = L.marker(opts.toLatLng(pt), {
        pane: 'mpShapeUI', keyboard: false, interactive: true, zIndexOffset: 1000,
        icon: L.divIcon({ className: 'mp-shp-h-icon', html: '<div class="mp-shp-h" data-handle="' + name + '"></div>', iconSize: [14, 14], iconAnchor: [7, 7] })
      });
      m.on('add', function () {
        var e = m.getElement();
        if (!e || e.__mpShp) return;
        e.__mpShp = true;
        e.setAttribute('aria-hidden', 'true');
        L.DomEvent.on(e, 'mousedown touchstart', function (ev) {
          if (ev.type === 'mousedown' && ev.button !== 0) return;
          L.DomEvent.stopPropagation(ev);
          L.DomEvent.preventDefault(ev);
          beginHandle(ev, name);
        });
      });
      ui.group.addLayer(m);
      ui.handles[name] = m;
    }

    function showSelection() {
      clearUI();
      var it = selectedId ? items[selectedId] : null;
      if (!it) return;
      var d = it.d, pts = d.points || [];
      ui = { group: L.layerGroup().addTo(map), handles: {}, outline: null, key: '' };
      if (isBoxType(d.drawing_type)) {
        var b = boxOf(pts);
        if (!b) return;
        ui.outline = L.rectangle(boundsOf(b), { pane: 'mpShapeUI', className: 'mp-shp-sel', interactive: false });
        ui.group.addLayer(ui.outline);
        var px = pxSize(b);
        var names = handlesFor(px.w, px.h);
        ui.key = names.join();
        names.forEach(function (h) { addHandle(h, handlePos(b, h)); });
      } else if (d.drawing_type === 'arrow' && pts.length === 2) {
        addHandle('a', pts[0]);
        addHandle('b', pts[1]);
      } else if (pts.length) {
        addHandle('mv', pts[0]);
      }
    }

    // updateSelection moves the existing handles to the shape's new place, or
    // rebuilds them when the set of handles has changed.
    function updateSelection() {
      var it = selectedId ? items[selectedId] : null;
      if (!it) return;
      if (!ui) { showSelection(); return; }
      var d = it.d, pts = d.points || [];
      if (isBoxType(d.drawing_type)) {
        var b = boxOf(pts);
        if (!b) return;
        var px = pxSize(b);
        if (handlesFor(px.w, px.h).join() !== ui.key) { showSelection(); return; }
        ui.outline.setBounds(boundsOf(b));
        Object.keys(ui.handles).forEach(function (h) { ui.handles[h].setLatLng(opts.toLatLng(handlePos(b, h))); });
      } else if (d.drawing_type === 'arrow' && pts.length === 2) {
        ui.handles.a.setLatLng(opts.toLatLng(pts[0]));
        ui.handles.b.setLatLng(opts.toLatLng(pts[1]));
      } else if (ui.handles.mv && pts.length) {
        ui.handles.mv.setLatLng(opts.toLatLng(pts[0]));
      }
    }
    function onZoomEnd() { if (!destroyed && selectedId) updateSelection(); }
    map.on('zoomend', onZoomEnd);

    // ---- changing a shape ----

    // swap re-draws the selected shape from new data (a live preview, or the
    // saved state put back), keeping keyboard focus on it.
    function swap(id, nd) {
      var it = items[id];
      var hadFocus = !!it && it.els.indexOf(document.activeElement) !== -1;
      replacing = id;
      try { opts.replace(nd); } finally { replacing = null; }
      var now = items[id];
      if (hadFocus && now && now.els[0]) { try { now.els[0].focus({ preventScroll: true }); } catch (e) { /* not focusable */ } }
      if (selectedId === id) { updateSelection(); syncBar(); }
    }

    function preview(id, fields) {
      var it = items[id];
      if (!it) return;
      swap(id, Object.assign({}, it.d, fields));
    }

    // commit saves what was previewed. Only the changed fields travel (the
    // endpoint is a partial update); if the server refuses, the shape goes back
    // to how it was.
    function commit(id, before, fields) {
      if (!items[id]) return;
      Promise.resolve(opts.patch(id, fields)).then(function (ok) {
        if (!ok && items[id]) swap(id, before);
      }, function () {
        if (items[id]) swap(id, before);
      });
    }

    // drag follows the pointer until it is released; mouse and touch both
    // route through here.
    function drag(startEvent, onMove, onEnd) {
      var moved = false;
      var s = pointOf(startEvent);
      dragging = true;
      function move(e) {
        var p = pointOf(e);
        if (!moved && Math.abs(p.x - s.x) + Math.abs(p.y - s.y) < 3) return;
        moved = true;
        if (e.cancelable) e.preventDefault();
        onMove(e);
      }
      function up(e) {
        window.removeEventListener('mousemove', move);
        window.removeEventListener('mouseup', up);
        window.removeEventListener('touchmove', move);
        window.removeEventListener('touchend', up);
        window.removeEventListener('touchcancel', up);
        dragging = false;
        // The click that ends a real drag is not a click away; a press that did
        // not move is just a click.
        if (moved) lastDragEnd = Date.now();
        onEnd(moved);
      }
      window.addEventListener('mousemove', move);
      window.addEventListener('mouseup', up);
      window.addEventListener('touchmove', move, { passive: false });
      window.addEventListener('touchend', up);
      window.addEventListener('touchcancel', up);
    }

    function beginMove(startEvent, id) {
      var it = items[id];
      if (!it) return;
      var d0 = clone(it.d);
      var sp = pctOf(startEvent);
      var type = d0.drawing_type;
      var st = stepFor(type, step());
      drag(startEvent, function (e) {
        var p = pctOf(e);
        var dx = p.x - sp.x, dy = p.y - sp.y;
        var on = !e.altKey;
        var b = isBoxType(type) ? boxOf(d0.points) : null;
        var pts = b ? pointsOfBox(moveBox(b, dx, dy, st, on)) : shiftPoints(d0.points, dx, dy, st, on);
        preview(id, { points: pts });
      }, function (moved) {
        if (moved && items[id]) commit(id, d0, { points: items[id].d.points });
      });
    }

    function beginHandle(startEvent, handle) {
      var id = selectedId;
      var it = items[id];
      if (!it) return;
      var d0 = clone(it.d);
      var type = d0.drawing_type;
      var st = stepFor(type, step());
      drag(startEvent, function (e) {
        var p = pctOf(e);
        var on = !e.altKey;
        var pts = null;
        if (isBoxType(type)) {
          var b = boxOf(d0.points);
          if (b) pts = pointsOfBox(resizeBox(b, handle, p, st, on, minSize(type, step())));
        } else if (type === 'arrow') {
          pts = moveEnd(d0.points, handle === 'a' ? 0 : 1, p, st, on);
        } else {
          pts = placePoint(p, st, on);
        }
        if (pts) preview(id, { points: pts });
      }, function (moved) {
        if (moved && items[id]) commit(id, d0, { points: items[id].d.points });
      });
    }

    // Keyboard moves land one step at a time; the save waits for a pause so a
    // held key is one request, not dozens.
    function flushNudge() {
      if (!nudgeBase) return;
      clearTimeout(nudgeTimer);
      var base = nudgeBase;
      nudgeBase = null;
      if (items[base.id]) commit(base.id, base, { points: items[base.id].d.points });
    }

    function nudge(key, shift) {
      var it = selectedId ? items[selectedId] : null;
      if (!it) return false;
      var pts = nudgePoints(it.d, key, shift, step());
      if (!pts) return true;
      if (!nudgeBase) nudgeBase = clone(it.d);
      preview(it.id, { points: pts });
      clearTimeout(nudgeTimer);
      nudgeTimer = setTimeout(flushNudge, 350);
      return true;
    }

    // ---- the bar: name or bubble text, hide from players, delete ----

    function buildBar() {
      if (bar) return;
      var host = map.getContainer().parentNode;
      bar = el('div', 'mp-fl mp-shpbar', { role: 'toolbar', 'aria-label': 'Shape' });
      bar.hidden = true;
      var field = el('label');
      var fieldName = el('span');
      var line = el('input', '', { type: 'text', maxlength: String(MAX_TEXT), autocomplete: 'off' });
      var area = el('textarea', '', { rows: '1', maxlength: String(MAX_TEXT) });
      field.appendChild(fieldName);
      field.appendChild(line);
      field.appendChild(area);
      bar.appendChild(field);

      var hideWrap = el('label');
      var hide = el('input', '', { type: 'checkbox' });
      hideWrap.appendChild(hide);
      hideWrap.appendChild(document.createTextNode('Hide from players'));
      bar.appendChild(hideWrap);

      var hint = el('span', 'mp-shp-hint', { role: 'status' });
      bar.appendChild(hint);
      var del = el('button', 'mp-shp-del', { type: 'button' });
      del.textContent = 'Delete';
      var done = el('button', '', { type: 'button', 'aria-label': 'Done' });
      done.textContent = 'Done';
      bar.appendChild(del);
      bar.appendChild(done);

      // Keys typed in the bar belong to it, not to the map's shortcuts.
      ['keydown', 'mousedown', 'pointerdown', 'click', 'dblclick', 'wheel', 'touchstart', 'contextmenu'].forEach(function (ev) {
        bar.addEventListener(ev, function (e) { e.stopPropagation(); });
      });
      [line, area].forEach(function (f) {
        f.addEventListener('input', function () { onText(f, false); });
        f.addEventListener('change', function () { onText(f, true); });
      });
      line.addEventListener('keydown', function (e) {
        if (e.key === 'Enter' && !e.isComposing) { e.preventDefault(); line.blur(); }
      });
      area.addEventListener('keydown', function (e) {
        // Enter saves; Shift+Enter starts a new line, as in the bubble editor.
        if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); area.blur(); }
      });
      hide.addEventListener('change', function () {
        var it = selectedId ? items[selectedId] : null;
        if (!it) return;
        var before = clone(it.d);
        var vis = hide.checked ? 'dm_only' : 'everyone';
        preview(it.id, { visibility: vis });
        commit(it.id, before, { visibility: vis });
      });
      del.addEventListener('click', function () {
        var it = selectedId ? items[selectedId] : null;
        if (it && opts.canDelete(it.d)) opts.remove(it.id, anchorOf(it.d));
      });
      done.addEventListener('click', function () { deselect(); });
      host.appendChild(bar);
      refs = { field: field, name: fieldName, line: line, area: area, hideWrap: hideWrap, hide: hide, hint: hint, del: del };
    }

    function anchorOf(d) {
      var p = (d.points || [])[0];
      return p ? opts.toLatLng(p) : null;
    }

    function activeField() {
      var it = selectedId ? items[selectedId] : null;
      if (!it || !refs) return null;
      return it.d.drawing_type === 'callout' ? refs.area : refs.line;
    }

    function syncBar() {
      var it = selectedId ? items[selectedId] : null;
      if (!it) { if (bar) bar.hidden = true; return; }
      buildBar();
      var d = it.d;
      var label = fieldLabel(d.drawing_type);
      var multi = d.drawing_type === 'callout';
      bar.hidden = false;
      refs.field.hidden = !label;
      refs.name.textContent = label;
      refs.line.hidden = multi;
      refs.area.hidden = !multi;
      var f = multi ? refs.area : refs.line;
      f.setAttribute('aria-label', label || 'Text');
      // Do not overwrite what is being typed.
      if (document.activeElement !== f) f.value = d.text_content == null ? '' : String(d.text_content);
      refs.hideWrap.hidden = !opts.canDmOnly;
      refs.hide.checked = d.visibility === 'dm_only';
      refs.del.hidden = !opts.canDelete(d);
      // Said once per selection: this line is a live region, and a move
      // re-syncs the bar on every frame.
      if (refs.hintFor !== it.id) {
        refs.hintFor = it.id;
        refs.hint.textContent = typeLabel(d.drawing_type) + ' selected. Arrow keys move it' +
          (isBoxType(d.drawing_type) ? ', Shift and arrows resize it' : d.drawing_type === 'arrow' ? ', Shift and arrows move the tip' : '') + '.';
      }
    }

    // onText previews the text as it is typed and saves it when the field is
    // left. A bubble cannot be saved empty, so an emptied one goes back; a box
    // name can be cleared.
    function onText(f, final) {
      var it = selectedId ? items[selectedId] : null;
      if (!it) return;
      if (!textBase || textBase.id !== it.id) textBase = clone(it.d);
      var bubble = it.d.drawing_type === 'callout';
      var v = f.value;
      var trimmed = v.trim();
      if (!final) {
        if (bubble && !trimmed) return;
        preview(it.id, { text_content: trimmed ? v : null });
        return;
      }
      var base = textBase;
      textBase = null;
      if (bubble && !trimmed) {
        f.value = base.text_content == null ? '' : String(base.text_content);
        if (items[it.id]) swap(it.id, base);
        return;
      }
      var next = trimmed ? trimmed : null;
      if (next === (base.text_content == null ? null : base.text_content)) {
        if (items[it.id] && trimmed !== v) preview(it.id, { text_content: next });
        return;
      }
      preview(it.id, { text_content: next });
      commit(it.id, base, { text_content: next });
    }

    function focusField() {
      var f = activeField();
      if (!f || f.hidden) return;
      try { f.focus(); if (f.select) f.select(); } catch (e) { /* not focusable */ }
    }

    // ---- keyboard ----

    function typing(t) {
      return !!(t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable));
    }
    function onKey(e) {
      if (destroyed) return;
      if (!map.getContainer().isConnected) { document.removeEventListener('keydown', onKey, true); return; }
      if (!selectedId || !live || e.ctrlKey || e.metaKey || e.altKey) return;
      if (typing(e.target) || (e.target.closest && e.target.closest('.mp-ann-pop, #mp-sheet, .mp-ms-pop, #marker-modal'))) return;
      if (ARROW_KEYS[e.key]) {
        // Arrow keys would otherwise pan the map.
        if (nudge(e.key, e.shiftKey)) { e.preventDefault(); e.stopPropagation(); }
      }
    }
    document.addEventListener('keydown', onKey, true);

    // A click on empty map drops the selection; a click on a shape, a handle or
    // the bar does not, and neither does the click that ends a drag.
    function onMapClick(e) {
      if (!selectedId || Date.now() - lastDragEnd < 300) return;
      var t = e.originalEvent && e.originalEvent.target;
      if (t && t.closest && t.closest('.mp-shp-h, .mp-shpbar')) return;
      for (var id in items) {
        if (items[id].els.some(function (x) { return t && x.contains && x.contains(t); })) return;
      }
      deselect();
    }
    map.on('click', onMapClick);

    return {
      // render draws a shape the drawing module asks for; wire gives it the
      // select/move behaviour. Arrows and bubbles are drawn by the annotation
      // module and only wired.
      renderBox: function (d, env) {
        return renderBox(d, Object.assign({ map: map, toLatLng: opts.toLatLng, interactive: true }, env || {}));
      },
      wire: wire,
      unwire: unwire,
      reset: reset,
      // selectMode(false) is called while another tool is active: shapes then
      // let every click through and nothing stays selected.
      selectMode: function (on) {
        live = !!on;
        if (!live) deselect();
        for (var id in items) applyAria(items[id]);
      },
      select: select,
      selectedId: function () { return selectedId; },
      escape: function () { return deselect(); },
      deleteSelected: function () {
        var it = selectedId ? items[selectedId] : null;
        if (!it || !opts.canDelete(it.d)) return false;
        opts.remove(it.id, anchorOf(it.d));
        return true;
      },
      busy: function () { return dragging || selectedId !== null; },
      destroy: function () {
        if (destroyed) return;
        flushNudge();
        destroyed = true;
        deselect();
        map.off('click', onMapClick);
        map.off('zoomend', onZoomEnd);
        document.removeEventListener('keydown', onKey, true);
        if (bar && bar.parentNode) bar.parentNode.removeChild(bar);
        bar = null;
      }
    };
  }

  var api = Object.assign({ attach: attach, renderBox: renderBox, ensureStyle: addStyle }, pure);
  window.ChronicleMapShapes = api;
})();
