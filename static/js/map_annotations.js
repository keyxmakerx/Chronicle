/**
 * map_annotations.js -- arrows, highlighter strokes, numbered steps and speech
 * bubbles on a Chronicle map, and the small in-map editors the drawing tools
 * use (the text editor for labels and bubbles, and the delete confirm).
 *
 * Each annotation is a drawing (see internal/plugins/maps/drawing_annotation.go):
 *   arrow      points [tail, tip]; stroke_color, stroke_width (the head grows with it)
 *   highlight  points along the stroke; stroke_width, fill_alpha as opacity,
 *              multiplied over the map so the map shows through
 *   step       points [centre]; text_content is the number
 *   callout    points [where the tail points]; text_content is the text
 * The server validates and withholds them like any drawing, so this file is
 * only what is on screen. User text is only ever set with textContent.
 *
 * The pure geometry lives at the top and is exported for tests; the browser
 * part needs Leaflet.
 */
(function () {
  'use strict';

  var MAX_TEXT = 500;
  var MAX_STEP = 999;
  // The widest a bubble's text runs before it wraps, in screen pixels.
  var BUBBLE_TEXT_MAX = 220;
  var BUBBLE = { padX: 10, padY: 7, radius: 10, tailH: 12, tailBase: 14, tailInset: 16 };

  // ---- Pure geometry ----

  // arrowHead sizes an arrowhead in screen pixels from the line width, so a
  // thick arrow gets a bigger head. On a line too short for a full head the
  // head shrinks to fit. Returns the tip, the two barbs and the neck where the
  // shaft stops (so a round line cap never shows past the point).
  function arrowHead(tail, tip, width) {
    var w = Math.max(1, Number(width) || 1);
    var dx = tip.x - tail.x;
    var dy = tip.y - tail.y;
    var len = Math.sqrt(dx * dx + dy * dy);
    var head = Math.max(10, w * 3 + 6);
    if (len > 0 && head > len * 0.8) head = len * 0.8;
    var half = head * 0.6;
    if (len === 0) {
      return { tip: { x: tip.x, y: tip.y }, left: { x: tip.x, y: tip.y }, right: { x: tip.x, y: tip.y }, neck: { x: tip.x, y: tip.y }, length: 0 };
    }
    var ux = dx / len;
    var uy = dy / len;
    var bx = tip.x - ux * head;
    var by = tip.y - uy * head;
    // The shaft stops a little inside the head so the joint never shows a gap.
    var neckBack = head * 0.7;
    return {
      tip: { x: tip.x, y: tip.y },
      left: { x: bx - uy * half, y: by + ux * half },
      right: { x: bx + uy * half, y: by - ux * half },
      neck: { x: tip.x - ux * neckBack, y: tip.y - uy * neckBack },
      length: head
    };
  }

  // stepNumberOf reads a step's number, or 0 when it has none that is valid.
  function stepNumberOf(d) {
    if (!d || d.drawing_type !== 'step') return 0;
    var s = String(d.text_content == null ? '' : d.text_content).trim();
    if (!/^[0-9]{1,3}$/.test(s)) return 0;
    var n = parseInt(s, 10);
    return n >= 1 && n <= MAX_STEP ? n : 0;
  }

  // nextStepNumber continues from the highest step already on the map, so
  // deleting one never renumbers the rest and a gap is never refilled. It stops
  // at the server's cap.
  function nextStepNumber(drawings) {
    var max = 0;
    (drawings || []).forEach(function (d) { max = Math.max(max, stepNumberOf(d)); });
    return Math.min(MAX_STEP, max + 1);
  }

  function stepHint(n) {
    if (n >= MAX_STEP) return 'Click to place step ' + MAX_STEP;
    return 'Click to place step ' + n + ', then ' + (n + 1) + ' and on';
  }

  // bubbleLayout lays a speech bubble around text of the measured size. The
  // tail's point is at (0, 0), the click point; the box sits above it and a
  // little to the left, so the tail runs straight down its left side. Returns
  // the box, where the text goes and one outline path (box and tail together,
  // so the stroke has no seam).
  function bubbleLayout(textW, textH) {
    var b = BUBBLE;
    var minW = b.tailInset + b.tailBase + b.radius;
    var w = Math.max(minW, Math.ceil(Math.max(0, textW)) + 2 * b.padX);
    var h = Math.max(2 * b.radius, Math.ceil(Math.max(0, textH)) + 2 * b.padY);
    var x = -b.tailInset;
    var y = -b.tailH - h;
    var r = b.radius;
    var bottom = y + h;
    var path = [
      'M', x + r, y,
      'H', x + w - r,
      'Q', x + w, y, x + w, y + r,
      'V', bottom - r,
      'Q', x + w, bottom, x + w - r, bottom,
      'H', b.tailBase,
      'L', 0, 0,
      'L', 0, bottom,
      'H', x + r,
      'Q', x, bottom, x, bottom - r,
      'V', y + r,
      'Q', x, y, x + r, y,
      'Z'
    ].join(' ');
    return { x: x, y: y, w: w, h: h, text: { x: x + b.padX, y: y + b.padY }, path: path, tip: { x: 0, y: 0 } };
  }

  // highlightWidth turns the rail's line choice into a highlighter's width:
  // a highlighter is always broad, and the choice only scales it.
  function highlightWidth(lineWidth) {
    var w = Number(lineWidth) || 4;
    return Math.min(40, Math.max(12, Math.round(w * 4 + 2)));
  }

  function roundPct(v) { return Math.round(v * 100) / 100; }

  // fitStroke rounds a stroke's points and drops ones too close to the last
  // kept, widening the gap until the stroke fits the server's size ceiling.
  // The first and last points always stay.
  function fitStroke(points, maxBytes) {
    var limit = maxBytes || 9500;
    var gap = 0.15;
    var out = [];
    for (var tries = 0; tries < 12; tries++) {
      out = [];
      for (var i = 0; i < points.length; i++) {
        var p = { x: roundPct(points[i].x), y: roundPct(points[i].y) };
        var last = out[out.length - 1];
        if (last && i !== points.length - 1 && Math.abs(p.x - last.x) + Math.abs(p.y - last.y) < gap) continue;
        out.push(p);
      }
      if (JSON.stringify(out).length <= limit) return out;
      gap *= 1.6;
    }
    return out;
  }

  // ---- Browser-only part ----

  if (typeof window === 'undefined' || typeof document === 'undefined') {
    if (typeof module !== 'undefined' && module.exports) {
      module.exports = {
        MAX_TEXT: MAX_TEXT, MAX_STEP: MAX_STEP, BUBBLE: BUBBLE,
        arrowHead: arrowHead, nextStepNumber: nextStepNumber, stepNumberOf: stepNumberOf,
        stepHint: stepHint, bubbleLayout: bubbleLayout, highlightWidth: highlightWidth,
        fitStroke: fitStroke
      };
    }
    return;
  }

  var SVGNS = 'http://www.w3.org/2000/svg';
  var HEX = /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/;
  function safeColor(c, fallback) { return HEX.test(c || '') ? c : fallback; }

  var styleAdded = false;
  function addStyle() {
    if (styleAdded || document.getElementById('mp-annotations-style')) { styleAdded = true; return; }
    styleAdded = true;
    var s = document.createElement('style');
    s.id = 'mp-annotations-style';
    s.textContent = [
      '.mp-ann-icon { background: none; border: 0; }',
      '.mp-ann-step { box-sizing: border-box; width: 100%; height: 100%; border-radius: 50%; border: 2.5px solid #fff; color: #fff; display: flex; align-items: center; justify-content: center; font: 700 14px/1 system-ui, sans-serif; box-shadow: 0 1px 3px rgb(0 0 0 / 0.35); cursor: pointer; font-variant-numeric: tabular-nums; }',
      '.mp-ann-bubble { position: absolute; left: 0; top: 0; width: 0; height: 0; transform-origin: 0 0; }',
      // Leaflet lifts every svg in the map pane to z-index 200; the outline
      // must stay under its text.
      '.mp-ann-bubble > svg.mp-ann-outline { position: absolute; z-index: 0; overflow: visible; pointer-events: none; filter: drop-shadow(0 1px 2px rgb(0 0 0 / 0.3)); }',
      '.mp-ann-bubble-text { position: absolute; z-index: 1; left: 0; top: 0; width: max-content; max-width: ' + BUBBLE_TEXT_MAX + 'px; color: #111827; font: 13px/1.35 system-ui, sans-serif; white-space: pre-wrap; overflow-wrap: anywhere; cursor: pointer; }',
      '.mp-ann-measuring { visibility: hidden; }',
      // New shapes arrive quickly: discs and bubbles grow from their point,
      // lines fade in. Calm and Off are applied site-wide on html[data-motion].
      '@keyframes mp-ann-grow { from { opacity: 0; transform: scale(0.6); } to { opacity: 1; transform: none; } }',
      '@keyframes mp-ann-fade { from { opacity: 0; } to { opacity: 1; } }',
      '.mp-ann-in-grow { animation: mp-ann-grow var(--dur-standard, 200ms) var(--ease-out, cubic-bezier(0.16, 1, 0.3, 1)) both; }',
      '.mp-ann-in-fade { animation: mp-ann-fade var(--dur-standard, 200ms) var(--ease-out, cubic-bezier(0.16, 1, 0.3, 1)) both; }',
      // The in-map editor and confirm grow out of the point they belong to.
      '.mp-ann-pop { position: absolute; z-index: 1200; padding: 10px; display: flex; flex-direction: column; gap: 8px; width: 240px; font-size: 13px; transform-origin: var(--mp-pop-origin, 0 100%); animation: mp-ann-grow var(--dur-standard, 200ms) var(--ease-out, cubic-bezier(0.16, 1, 0.3, 1)) both; }',
      '.mp-ann-pop h4 { margin: 0; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--color-text-secondary); }',
      '.mp-ann-pop p { margin: 0; color: var(--color-text-body); line-height: 1.4; }',
      '.mp-ann-pop textarea { resize: vertical; min-height: 64px; max-height: 180px; }',
      '.mp-ann-pop .mp-ann-row { display: flex; align-items: center; justify-content: flex-end; gap: 6px; }',
      '.mp-ann-pop .mp-ann-count { margin-right: auto; font: 11px ui-monospace, monospace; color: var(--color-text-secondary); }',
      '.mp-ann-pop .mp-ann-count[data-near="true"] { color: var(--color-danger, #ef4444); }',
      // The map page resets button colour by id, so the coloured buttons name
      // it at the same strength.
      '#map-wrap .mp-ann-pop .mp-cbtn-primary, #map-wrap .mp-ann-pop .mp-cbtn-danger { color: #fff; }',
      '.mp-cbtn.mp-cbtn-danger { background: var(--color-danger-strong, #dc2626); border-color: var(--color-danger-strong, #dc2626); color: #fff; }',
      '.mp-cbtn.mp-cbtn-danger:hover:not(:disabled) { filter: brightness(1.08); }',
      '@media (prefers-reduced-motion: reduce) { .mp-ann-in-grow, .mp-ann-in-fade, .mp-ann-pop { animation: none; } }'
    ].join('\n');
    document.head.appendChild(s);
  }

  // The highlighter's own pane multiplies with what is under it (the map
  // picture, pictures on it and hex art), which a path inside the shared vector
  // pane cannot, because each pane is its own stacking context. It sits just
  // under the other drawings (400) and over the hexes (390).
  function highlightPane(map) {
    var pane = map.getPane('mpHighlight');
    if (!pane) {
      pane = map.createPane('mpHighlight');
      pane.style.zIndex = 395;
      pane.style.mixBlendMode = 'multiply';
    }
    return 'mpHighlight';
  }

  function animateIn(el, kind) {
    if (!el || !el.classList) return;
    el.classList.add(kind === 'grow' ? 'mp-ann-in-grow' : 'mp-ann-in-fade');
    el.addEventListener('animationend', function done() {
      el.classList.remove('mp-ann-in-grow', 'mp-ann-in-fade');
      el.removeEventListener('animationend', done);
    });
  }

  function svgEl(tag, attrs) {
    var e = document.createElementNS(SVGNS, tag);
    for (var k in attrs) if (Object.prototype.hasOwnProperty.call(attrs, k)) e.setAttribute(k, attrs[k]);
    return e;
  }

  // ---- Renderers ----

  // renderArrow keeps the head in screen pixels, so it is redrawn when the zoom
  // changes; the shaft and head are one group for events.
  function renderArrow(d, env) {
    var pts = d.points || [];
    if (pts.length < 2) return null;
    var color = safeColor(d.stroke_color, '#000000');
    var width = Math.max(1, Number(d.stroke_width) || 2);
    var a = env.toLatLng(pts[0]);
    var b = env.toLatLng(pts[1]);
    var shaft = L.polyline([a, b], { color: color, weight: width, lineCap: 'round', interactive: true });
    var head = L.polygon([b, b, b], { stroke: false, fill: true, fillColor: color, fillOpacity: 1, interactive: true });
    var group = L.featureGroup([shaft, head]);
    var map = null;
    function update() {
      if (!map) return;
      var g = arrowHead(map.latLngToLayerPoint(a), map.latLngToLayerPoint(b), width);
      shaft.setLatLngs([a, map.layerPointToLatLng(L.point(g.neck.x, g.neck.y))]);
      head.setLatLngs([g.tip, g.left, g.right].map(function (p) { return map.layerPointToLatLng(L.point(p.x, p.y)); }));
    }
    group.on('add', function () {
      map = env.map;
      map.on('zoomend', update);
      update();
      if (env.animate) { animateIn(shaft._path, 'fade'); animateIn(head._path, 'fade'); }
    });
    group.on('remove', function () { if (map) map.off('zoomend', update); map = null; });
    return group;
  }

  function renderHighlight(d, env) {
    var pts = d.points || [];
    if (pts.length < 2) return null;
    var alpha = Number(d.fill_alpha);
    if (!isFinite(alpha) || alpha <= 0) alpha = 0.35;
    var layer = L.polyline(pts.map(env.toLatLng), {
      pane: highlightPane(env.map),
      color: safeColor(d.stroke_color, '#facc15'),
      weight: Math.max(1, Number(d.stroke_width) || 18),
      opacity: Math.min(0.7, alpha),
      lineCap: 'round',
      lineJoin: 'round',
      interactive: true
    });
    if (env.animate) layer.on('add', function () { animateIn(layer._path, 'fade'); });
    return layer;
  }

  function renderStep(d, env) {
    var pts = d.points || [];
    var n = stepNumberOf(d);
    if (pts.length < 1 || !n) return null;
    var fs = Math.min(48, Math.max(8, Number(d.font_size) || 14));
    var size = Math.round(fs * 2);
    var disc = document.createElement('div');
    disc.className = 'mp-ann-step';
    disc.style.background = safeColor(d.stroke_color, '#7c3aed');
    disc.style.fontSize = fs + 'px';
    disc.textContent = String(n);
    if (env.animate) animateIn(disc, 'grow');
    return L.marker(env.toLatLng(pts[0]), {
      icon: L.divIcon({ className: 'mp-ann-icon', html: disc, iconSize: [size, size], iconAnchor: [size / 2, size / 2] }),
      interactive: true,
      keyboard: false
    });
  }

  // renderCallout measures the wrapped text once it is on the page, then draws
  // the outline around it; a bubble re-added after "hide drawings" measures
  // again, because nothing has a size while it is off the page.
  function renderCallout(d, env) {
    var pts = d.points || [];
    var text = d.text_content == null ? '' : String(d.text_content);
    if (pts.length < 1 || !text.trim()) return null;
    var color = safeColor(d.stroke_color, '#7c3aed');
    var wrap = document.createElement('div');
    wrap.className = 'mp-ann-bubble mp-ann-measuring';
    var svg = svgEl('svg', { 'class': 'mp-ann-outline' });
    var outline = svgEl('path', { fill: '#ffffff', stroke: color, 'stroke-width': '2', 'stroke-linejoin': 'round' });
    svg.appendChild(outline);
    var body = document.createElement('div');
    body.className = 'mp-ann-bubble-text';
    var fs = Number(d.font_size);
    if (fs >= 8 && fs <= 48) body.style.fontSize = fs + 'px';
    body.textContent = text;
    wrap.appendChild(svg);
    wrap.appendChild(body);
    var marker = L.marker(env.toLatLng(pts[0]), {
      icon: L.divIcon({ className: 'mp-ann-icon', html: wrap, iconSize: [0, 0], iconAnchor: [0, 0] }),
      interactive: true,
      keyboard: false
    });
    marker.on('add', function () {
      var lay = bubbleLayout(body.offsetWidth, body.offsetHeight);
      body.style.left = lay.text.x + 'px';
      body.style.top = lay.text.y + 'px';
      // The outline's own box, with room for the stroke on every side.
      svg.setAttribute('width', String(lay.w + 4));
      svg.setAttribute('height', String(lay.h + BUBBLE.tailH + 4));
      svg.setAttribute('viewBox', (lay.x - 2) + ' ' + (lay.y - 2) + ' ' + (lay.w + 4) + ' ' + (lay.h + BUBBLE.tailH + 4));
      svg.style.left = (lay.x - 2) + 'px';
      svg.style.top = (lay.y - 2) + 'px';
      outline.setAttribute('d', lay.path);
      wrap.classList.remove('mp-ann-measuring');
      if (env.animate) { env.animate = false; animateIn(wrap, 'grow'); }
    });
    return marker;
  }

  // render returns the Leaflet layer for an annotation drawing, or null for a
  // drawing it does not draw (or one too damaged to draw).
  function render(d, env) {
    addStyle();
    switch (d && d.drawing_type) {
      case 'arrow': return renderArrow(d, env);
      case 'highlight': return renderHighlight(d, env);
      case 'step': return renderStep(d, env);
      case 'callout': return renderCallout(d, env);
    }
    return null;
  }

  // ---- In-map editors ----

  var openPop = null;
  function closePop() {
    if (openPop) { var p = openPop; openPop = null; p.close(); }
  }

  // place puts a popover beside a map point, inside the map's frame: above the
  // point when there is room, otherwise below, and it grows from that corner.
  function place(pop, map, latlng) {
    var host = pop.parentNode;
    var hostRect = host.getBoundingClientRect();
    var mapRect = map.getContainer().getBoundingClientRect();
    var pt = latlng ? map.latLngToContainerPoint(latlng) : map.getSize().divideBy(2);
    var x = pt.x + (mapRect.left - hostRect.left);
    var y = pt.y + (mapRect.top - hostRect.top);
    var w = pop.offsetWidth;
    var h = pop.offsetHeight;
    var left = Math.max(8, Math.min(x - 12, hostRect.width - w - 8));
    var above = y - h - 14 >= 8;
    var top = above ? y - h - 14 : Math.min(y + 14, hostRect.height - h - 8);
    pop.style.left = left + 'px';
    pop.style.top = Math.max(8, top) + 'px';
    pop.style.setProperty('--mp-pop-origin', Math.max(0, x - left) + 'px ' + (above ? '100%' : '0'));
  }

  function hostFor(map) {
    var c = map.getContainer();
    return document.getElementById('map-wrap') && document.getElementById('map-wrap').contains(c) ? document.getElementById('map-wrap') : c.parentNode || c;
  }

  function button(label, cls) {
    var b = document.createElement('button');
    b.type = 'button';
    b.className = 'mp-cbtn' + (cls ? ' ' + cls : '');
    b.textContent = label;
    return b;
  }

  function popShell(map, title) {
    closePop();
    addStyle();
    var pop = document.createElement('div');
    pop.className = 'mp-fl mp-ann-pop';
    pop.setAttribute('role', 'dialog');
    pop.setAttribute('aria-label', title);
    var h = document.createElement('h4');
    h.textContent = title;
    pop.appendChild(h);
    // Clicks, wheel and keys inside belong to the popover, not the map or the
    // viewer's shortcuts.
    ['mousedown', 'pointerdown', 'click', 'dblclick', 'contextmenu', 'wheel', 'touchstart'].forEach(function (ev) {
      pop.addEventListener(ev, function (e) { e.stopPropagation(); });
    });
    return pop;
  }

  function openShell(pop, map, latlng, onClose) {
    var host = hostFor(map);
    host.appendChild(pop);
    place(pop, map, latlng);
    function away(e) { if (!pop.contains(e.target)) handle.close(); }
    function moved() { place(pop, map, latlng); }
    var handle = {
      close: function () {
        if (!pop.parentNode) return;
        document.removeEventListener('pointerdown', away, true);
        map.off('move zoom resize', moved);
        pop.parentNode.removeChild(pop);
        if (openPop === handle) openPop = null;
        if (onClose) onClose();
      }
    };
    // A click off the popover closes it, as every map popover does.
    setTimeout(function () { document.addEventListener('pointerdown', away, true); }, 0);
    map.on('move zoom resize', moved);
    openPop = handle;
    return handle;
  }

  // editText opens the in-map text editor at a point. onSubmit gets the
  // trimmed text and returns a promise of whether it was saved; the editor
  // stays open (with the text) when it was not.
  function editText(opts) {
    var map = opts.map;
    var pop = popShell(map, opts.title);
    var field = document.createElement(opts.multiline ? 'textarea' : 'input');
    field.className = 'mp-input';
    if (!opts.multiline) field.type = 'text';
    else field.rows = 3;
    field.maxLength = MAX_TEXT;
    field.placeholder = opts.placeholder || '';
    field.value = opts.value || '';
    field.setAttribute('aria-label', opts.title);
    pop.appendChild(field);
    var row = document.createElement('div');
    row.className = 'mp-ann-row';
    var count = document.createElement('span');
    count.className = 'mp-ann-count';
    var cancel = button('Cancel');
    var ok = button(opts.submitLabel || 'Save', 'mp-cbtn-primary');
    row.appendChild(count);
    row.appendChild(cancel);
    row.appendChild(ok);
    pop.appendChild(row);
    function sync() {
      var len = field.value.length;
      // The count only shows near the limit, where it helps.
      count.textContent = len > MAX_TEXT - 100 ? len + ' / ' + MAX_TEXT : '';
      count.setAttribute('data-near', len >= MAX_TEXT ? 'true' : 'false');
      ok.disabled = !field.value.trim();
    }
    field.addEventListener('input', sync);
    sync();
    var handle = openShell(pop, map, opts.latlng, opts.onCancel);
    var busy = false;
    function submit() {
      var v = field.value.trim();
      if (!v || busy) return;
      busy = true;
      ok.disabled = true;
      Promise.resolve(opts.onSubmit(v)).then(function (saved) {
        busy = false;
        if (saved) { opts.onCancel = null; handle.close(); } else sync();
      }, function () { busy = false; sync(); });
    }
    ok.addEventListener('click', submit);
    cancel.addEventListener('click', function () { handle.close(); });
    field.addEventListener('keydown', function (e) {
      e.stopPropagation();
      if (e.key === 'Escape') { e.preventDefault(); handle.close(); return; }
      // Enter saves; Shift+Enter starts a new line in a bubble.
      if (e.key === 'Enter' && !(opts.multiline && e.shiftKey) && !e.isComposing) { e.preventDefault(); submit(); }
    });
    field.focus();
    if (field.select) field.select();
    return handle;
  }

  // confirmAt asks, in the map, before something is removed. onConfirm runs
  // only on the explicit button.
  function confirmAt(opts) {
    var map = opts.map;
    var pop = popShell(map, opts.title || 'Delete');
    var p = document.createElement('p');
    p.textContent = opts.message;
    pop.appendChild(p);
    var row = document.createElement('div');
    row.className = 'mp-ann-row';
    var cancel = button('Cancel');
    var ok = button(opts.confirmLabel || 'Delete', 'mp-cbtn-danger');
    row.appendChild(cancel);
    row.appendChild(ok);
    pop.appendChild(row);
    var handle = openShell(pop, map, opts.latlng, null);
    cancel.addEventListener('click', function () { handle.close(); });
    ok.addEventListener('click', function () { handle.close(); opts.onConfirm(); });
    pop.addEventListener('keydown', function (e) {
      e.stopPropagation();
      if (e.key === 'Escape') { e.preventDefault(); handle.close(); }
    });
    cancel.focus();
    return handle;
  }

  window.ChronicleMapAnnotations = {
    TYPES: ['arrow', 'highlight', 'step', 'callout'],
    MAX_TEXT: MAX_TEXT,
    render: render,
    arrowHead: arrowHead,
    nextStepNumber: nextStepNumber,
    stepHint: stepHint,
    highlightWidth: highlightWidth,
    fitStroke: fitStroke,
    editText: editText,
    confirmAt: confirmAt,
    closePopover: closePop
  };
})();
