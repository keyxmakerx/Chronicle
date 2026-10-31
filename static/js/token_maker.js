/**
 * token_maker.js -- the token maker (Chronicle.tokenMaker): a creature's map
 * token made from its picture, in one of thirty frames and any colour.
 *
 * boot.js holds a stub that loads this file on the first open, and this file
 * loads the frames (token_frames.js) and its styles the same way, so a page
 * pays for them only when someone makes a token. The maker pops out over the
 * page from the control that opened it and gives focus back to it on close.
 * The picker shows still previews made ahead of time (one sprite), so only the
 * frame being edited is drawn live.
 *
 * A picture with a painted checkerboard or flat backdrop behind it (what art
 * tools often give for "no background") is noticed on open, and the maker
 * offers to clear it (token_cutout.js does the pixel work). The wand on the
 * stage holds the same clearing by hand: a strength and Erase and Restore
 * brushes. The cleared copy is what the token is drawn from.
 *
 * Save draws the token at TOKEN_PX as a PNG: the media sanitiser keeps a PNG's
 * transparency, while a WebP comes back as a JPEG with dark corners. A moving
 * frame is saved as its still picture. The page's own picture is never
 * changed, so the token can be made again from it at any time.
 *
 *   Chronicle.tokenMaker.open({
 *     campaignId, entityId, name,  // whose token
 *     art,   // URL of the picture to start from (same origin), or ''
 *     from,  // the element it grows out of and hands focus back to
 *     save,  // 'field': write the page's token_image field here; or a
 *            // function(mediaId) for a caller that saves it itself
 *     onClose // optional, after it closes, saved or not: a caller whose
 *            // redraw replaced `from` puts focus back itself
 *   });
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var TOKEN_PX = 512, TOKEN_FIELD = 'token_image', STORE = 'chronicle.tokenMaker', STORE_BG = 'chronicle.tokenMaker.bg';
  // The background is worked out on a copy at most this many pixels across;
  // the token is 512, so nothing it needs is lost.
  var CUT_PX = 1600;
  var STATIC_URL = /^\/static\/[A-Za-z0-9_.\-\/]+\.(js|css|webp|png)(\?v=[A-Za-z0-9._\-]+)?$/;
  var FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';
  var COLOURS = [['', 'Original'], ['#b02a24', 'Red'], ['#e07a1c', 'Orange'], ['#d4a62a', 'Gold'], ['#3c9a4a', 'Green'], ['#2a8f9a', 'Teal'],
    ['#2f5fc8', 'Blue'], ['#7a3fb8', 'Purple'], ['#c8408a', 'Pink'], ['#ecebe6', 'White'], ['#26262a', 'Black']];
  var HINT = 'Inside the frame: move it. Frame corners: resize it. Outside: move the picture. Picture corners or the wheel: resize the picture.';
  // The previews sprite: tiles of PV_TILE pixels, PV_COLS to a row, in the engine's design order.
  var PV_COLS = 8;

  var F = null;    // window.ChronicleTokenFrames once loaded
  var cur = null;  // the open maker, or null

  function assets() {
    var t = document.getElementById('chronicle-token-maker-src'), a = {};
    ['engine', 'css', 'previews', 'cutout'].forEach(function (k) {
      var v = t && t.getAttribute('data-' + k);
      a[k] = v && STATIC_URL.test(v) && v.indexOf('..') === -1 ? v : '';
    });
    return a;
  }

  var engineLoad = null;
  function loadEngine() {
    if (window.ChronicleTokenFrames) { F = window.ChronicleTokenFrames; return Promise.resolve(); }
    engineLoad = engineLoad || new Promise(function (resolve, reject) {
      var url = assets().engine, s = document.createElement('script');
      if (!url) { reject(new Error('no frames source')); return; }
      s.src = url;
      s.onload = function () { if (window.ChronicleTokenFrames) { F = window.ChronicleTokenFrames; resolve(); } else reject(new Error('frames did not register')); };
      s.onerror = function () { reject(new Error('could not load the frames')); };
      document.head.appendChild(s);
    });
    engineLoad.catch(function () { engineLoad = null; });
    return engineLoad;
  }

  var cutoutLoad = null;
  function loadCutout() {
    if (window.ChronicleTokenCutout) return Promise.resolve(window.ChronicleTokenCutout);
    cutoutLoad = cutoutLoad || new Promise(function (resolve, reject) {
      var url = assets().cutout, s = document.createElement('script');
      if (!url) { reject(new Error('no cutout source')); return; }
      s.src = url;
      s.onload = function () { if (window.ChronicleTokenCutout) resolve(window.ChronicleTokenCutout); else reject(new Error('cutout did not register')); };
      s.onerror = function () { reject(new Error('could not load the cutout')); };
      document.head.appendChild(s);
    });
    cutoutLoad.catch(function () { cutoutLoad = null; });
    return cutoutLoad;
  }

  // The maker opens once its styles are in, so it never shows unstyled; a
  // stylesheet that fails to load still lets it open.
  var stylesLoad = null;
  function loadStyles() {
    stylesLoad = stylesLoad || new Promise(function (resolve) {
      var url = assets().css;
      if (!url) { resolve(); return; }
      var l = document.createElement('link');
      l.rel = 'stylesheet'; l.href = url; l.setAttribute('data-token-maker-css', '');
      l.onload = l.onerror = function () { resolve(); };
      setTimeout(resolve, 8000);
      document.head.appendChild(l);
    });
    return stylesLoad;
  }

  function loadPicture(url) {
    return new Promise(function (resolve, reject) {
      if (!url) { resolve(null); return; }
      var img = new Image();
      img.onload = function () { resolve(img); };
      img.onerror = function () { reject(new Error('picture')); };
      img.src = url;
    });
  }

  function remembered() {
    try { return JSON.parse(localStorage.getItem(STORE) || '{}') || {}; } catch (e) { return {}; }
  }
  function remember(st) {
    try { localStorage.setItem(STORE, JSON.stringify({ frame: st.frame, colour: st.colour, shape: st.shape })); } catch (e) { /* private window */ }
  }

  function motionOK() { return !(C.reducedMotion && C.reducedMotion()); }
  function clock() { return window.MotionRest ? window.MotionRest.now() : performance.now() / 1000; }
  function resting() { return !!(window.MotionRest && window.MotionRest.still()); }

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; });
  }
  function design(id) {
    for (var i = 0; i < F.designs.length; i++) if (F.designs[i].id === id) return F.designs[i];
    return F.designs[F.designs.length - 1];
  }
  function groupOf(id) {
    for (var i = 0; i < F.groups.length; i++) if (F.groups[i].ids.indexOf(id) !== -1) return F.groups[i].id;
    return '';
  }

  /* ---------- The sheet ---------- */

  function build(o) {
    var el = document.createElement('div');
    el.className = 'tkm';
    el.setAttribute('role', 'dialog');
    el.setAttribute('aria-modal', 'true');
    el.setAttribute('aria-labelledby', 'tkm-title');
    var sw = COLOURS.map(function (c) {
      return '<button type="button" class="tkm-sw' + (c[0] ? '' : ' is-original') + '" data-tkm-colour="' + c[0] + '" data-n="' + c[1] + '"' +
        (c[0] ? ' style="--sw:' + c[0] + '"' : '') + ' aria-label="' + c[1] + '" aria-pressed="false"></button>';
    }).join('') + '<label class="tkm-sw is-any" title="Any colour"><input type="color" value="#2d6f86" data-tkm-any aria-label="Any colour"></label>';
    el.innerHTML =
      '<div class="tkm-bg"></div>' +
      '<div class="tkm-p" tabindex="-1">' +
        '<div class="tkm-top">' +
          '<h2 class="tkm-title" id="tkm-title">Token for ' + esc(o.name || 'this page') + '</h2>' +
          '<button type="button" class="tkm-x" data-tkm-close aria-label="Close the token maker"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button>' +
        '</div>' +
        '<div class="tkm-body">' +
          '<div class="tkm-work">' +
            '<div class="tkm-stage">' +
              '<canvas class="tkm-surface" width="1100" height="1100" tabindex="0" aria-label="Work surface. Drag the frame or the picture; arrow keys move the frame, plus and minus resize it."></canvas>' +
              '<p class="tkm-wait" data-tkm-wait><i class="fa-solid fa-circle-notch fa-spin" aria-hidden="true"></i> Getting the frames ready…</p>' +
              '<button type="button" class="tkm-wand" data-tkm-wand aria-expanded="false" aria-controls="tkm-bgd" aria-label="Background: clear it or touch it up" title="Background" hidden>' +
                '<i class="fa-solid fa-wand-magic-sparkles" aria-hidden="true"></i></button>' +
              '<div class="tkm-bgd" id="tkm-bgd" role="group" aria-labelledby="tkm-bgd-l" data-tkm-bgd hidden>' +
                '<div class="tkm-bgd-top"><p class="tkm-bgd-l" id="tkm-bgd-l">Background</p>' +
                  '<button type="button" class="tkm-ico" data-tkm-bgd-close aria-label="Close the background tools"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>' +
                '<p class="tkm-bgd-s" data-tkm-bgs></p>' +
                '<div class="tkm-bgd-row"><button type="button" class="tkm-nb is-go" data-tkm-bgrm>Clear the background</button>' +
                  '<button type="button" class="tkm-nq" data-tkm-bgback hidden>Put it all back</button></div>' +
                '<p class="tkm-bgd-k" id="tkm-str-l">Strength</p>' +
                '<div class="tkm-dseg" role="group" aria-labelledby="tkm-str-l" data-tkm-str>' +
                  '<button type="button" data-v="light" aria-pressed="false">Light</button><button type="button" data-v="medium" aria-pressed="true">Medium</button><button type="button" data-v="strong" aria-pressed="false">Strong</button></div>' +
                '<p class="tkm-bgd-k" id="tkm-brush-l">Touch up</p>' +
                '<div class="tkm-dseg" role="group" aria-labelledby="tkm-brush-l" data-tkm-brush>' +
                  '<button type="button" data-v="" aria-pressed="true">Move</button><button type="button" data-v="erase" aria-pressed="false">Erase</button><button type="button" data-v="restore" aria-pressed="false">Restore</button></div>' +
                '<label class="tkm-dsize">Brush size <input type="range" min="8" max="90" step="1" value="26" data-tkm-bsize disabled></label>' +
              '</div>' +
              '<div class="tkm-offer" role="group" aria-label="Background" data-tkm-offer hidden>' +
                '<p class="tkm-offer-t" data-tkm-offer-t></p>' +
                '<div class="tkm-offer-b" data-tkm-ask><button type="button" class="tkm-nb is-go" data-tkm-yes>Remove it</button><button type="button" class="tkm-nb" data-tkm-no>Keep it</button></div>' +
                '<div class="tkm-offer-b" data-tkm-did hidden><button type="button" class="tkm-nq" data-tkm-undo>Undo</button><button type="button" class="tkm-nq" data-tkm-adjust>Adjust</button>' +
                  '<button type="button" class="tkm-ico" data-tkm-offer-x aria-label="Hide this note"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>' +
              '</div>' +
            '</div>' +
            '<p class="tkm-hint" data-tkm-hint>' + HINT + '</p>' +
            '<div class="tkm-tools">' +
              '<label class="tkm-slide">Picture <input type="range" min="10" max="400" step="1" value="100" data-tkm-zoom data-tkm-pic disabled></label>' +
              '<button type="button" class="btn-secondary btn-sm" data-tkm-fill data-tkm-pic disabled>Fill the frame</button>' +
              '<button type="button" class="btn-secondary btn-sm" data-tkm-flip data-tkm-pic aria-pressed="false" disabled>Flip</button>' +
              '<label class="btn-secondary btn-sm tkm-file"><span data-tkm-filel>Another picture</span><input type="file" accept="image/png,image/jpeg,image/webp" data-tkm-file disabled></label>' +
            '</div>' +
          '</div>' +
          '<div class="tkm-side">' +
            '<section class="tkm-sec"><p class="tkm-label" id="tkm-shape-l">Shape</p>' +
              '<div class="tkm-seg" role="group" aria-labelledby="tkm-shape-l" data-tkm-shape>' +
                '<button type="button" data-v="round" aria-pressed="true">Round</button><button type="button" data-v="square" aria-pressed="false">Square</button>' +
              '</div></section>' +
            '<section class="tkm-sec"><p class="tkm-label" id="tkm-frame-l">Frame</p>' +
              '<div class="tkm-groups" role="group" aria-labelledby="tkm-frame-l" data-tkm-groups></div>' +
              '<div class="tkm-frames" data-tkm-frames></div>' +
              '<p class="tkm-note" data-tkm-note></p>' +
              '<button type="button" class="tkm-quiet" data-tkm-motion hidden>Pause motion</button>' +
            '</section>' +
            '<section class="tkm-sec" data-tkm-colours><p class="tkm-label" id="tkm-colour-l">Colour</p>' +
              '<div class="tkm-swatches" role="group" aria-labelledby="tkm-colour-l">' + sw + '</div>' +
              '<p class="tkm-note" data-tkm-cname>Original: each frame in its own colours.</p></section>' +
            '<section class="tkm-sec"><p class="tkm-label">On the map</p>' +
              '<div class="tkm-map" data-tkm-map></div>' +
              '<p class="tkm-note">One, two and three squares.</p></section>' +
          '</div>' +
        '</div>' +
        '<div class="tkm-foot">' +
          '<p class="tkm-msg" role="status" aria-live="polite" data-tkm-msg></p>' +
          '<button type="button" class="btn-secondary" data-tkm-close>Cancel</button>' +
          '<button type="button" class="btn-primary" data-tkm-save disabled>Save as the token</button>' +
        '</div>' +
      '</div>';
    var map = el.querySelector('[data-tkm-map]');
    [50, 100, 150].forEach(function (px) {
      var c = document.createElement('canvas'), d = Math.min(2, window.devicePixelRatio || 1);
      c.width = c.height = Math.round(px * d); c.style.width = c.style.height = px + 'px';
      c.setAttribute('aria-label', 'The token at ' + (px / 50) + (px === 50 ? ' square' : ' squares'));
      c.setAttribute('role', 'img');
      map.appendChild(c);
    });
    return el;
  }

  // The panel grows out of the control that opened it, as dm_sheet.js does.
  function setOrigin(panel, from) {
    var p = panel.getBoundingClientRect();
    if (!from || !from.getBoundingClientRect || !p.width || !p.height) return false;
    var r = from.getBoundingClientRect();
    panel.style.setProperty('--ct', Math.max(0, r.top - p.top) + 'px');
    panel.style.setProperty('--cl', Math.max(0, r.left - p.left) + 'px');
    panel.style.setProperty('--cr', Math.max(0, p.right - r.right) + 'px');
    panel.style.setProperty('--cb', Math.max(0, p.bottom - r.bottom) + 'px');
    return true;
  }

  function q(sel) { return cur && cur.el.querySelector(sel); }
  function msg(text, kind) {
    var m = q('[data-tkm-msg]');
    if (!m) return;
    m.textContent = text || '';
    m.className = 'tkm-msg' + (kind ? ' is-' + kind : '');
  }

  /* ---------- Work surface: the crop-tool look of the signed design ---------- */

  var HANDLE = 22;
  function iw(st) { return st.img.naturalWidth || st.img.width; }
  function ih(st) { return st.img.naturalHeight || st.img.height; }
  function picRect(st) { var i = st.place.img; return { x: i.x - iw(st) * i.s / 2, y: i.y - ih(st) * i.s / 2, w: iw(st) * i.s, h: ih(st) * i.s }; }
  function frRect(st) { var f = st.place.fr; return { x: f.x - f.w / 2, y: f.y - f.h / 2, w: f.w, h: f.h }; }
  function corners(r) { return [[r.x, r.y], [r.x + r.w, r.y], [r.x, r.y + r.h], [r.x + r.w, r.y + r.h]]; }
  function keepFrameIn(st) {
    var W = F.W, fr = st.place.fr, k = Math.min(1, W * 0.98 / fr.w);
    fr.w = Math.max(60, fr.w * k); fr.h = fr.w;
    fr.x = Math.min(W - fr.w / 2, Math.max(fr.w / 2, fr.x));
    fr.y = Math.min(W - fr.h / 2, Math.max(fr.h / 2, fr.y));
  }
  function baseScale(st) { return Math.min(F.W * 0.9 / iw(st), F.W * 0.9 / ih(st)); }

  function opts(st, t, still) {
    return { img: st.img, place: st.place, frame: st.frame, colour: st.colour, shape: st.shape, flip: st.flip, name: st.name, t: t, still: still, clear: st.img !== st.orig };
  }
  function still(st) { return !st.playing || !design(st.frame).anim; }

  function drawWork(st) {
    var c = st.surface, g = c.getContext('2d'), W = c.width, pr = picRect(st), fr = frRect(st), L = 34, t = clock();
    g.clearRect(0, 0, W, W);
    g.save(); g.translate(st.place.img.x, st.place.img.y); if (st.flip) g.scale(-1, 1);
    g.drawImage(st.img, -pr.w / 2, -pr.h / 2, pr.w, pr.h); g.restore();
    g.save(); g.beginPath(); g.rect(0, 0, W, W); g.rect(fr.x, fr.y, fr.w, fr.h);
    g.fillStyle = 'rgba(10,10,14,.62)'; g.fill('evenodd'); g.restore();
    var tk = Math.min(1, F.MASTER / fr.w), tw = Math.max(1, Math.round(fr.w * tk));
    if (st.tmp.width !== tw) { st.tmp.width = tw; st.tmp.height = tw; } // resizing reallocates; render clears it anyway
    F.render(st.tmp.getContext('2d'), tw, tw, opts(st, t, still(st)));
    g.drawImage(st.tmp, fr.x, fr.y, fr.w, fr.h);
    g.save(); g.strokeStyle = 'rgba(255,255,255,.28)'; g.lineWidth = 1.5; g.beginPath();
    for (var k = 1; k < 3; k++) {
      g.moveTo(fr.x + fr.w * k / 3, fr.y); g.lineTo(fr.x + fr.w * k / 3, fr.y + fr.h);
      g.moveTo(fr.x, fr.y + fr.h * k / 3); g.lineTo(fr.x + fr.w, fr.y + fr.h * k / 3);
    }
    g.stroke();
    g.strokeStyle = 'rgba(255,255,255,.7)'; g.strokeRect(fr.x, fr.y, fr.w, fr.h);
    g.strokeStyle = '#fff'; g.lineWidth = 6; g.lineCap = 'square'; g.beginPath();
    corners(fr).forEach(function (p, i) {
      var sx = i % 2 ? -1 : 1, sy = i > 1 ? -1 : 1;
      g.moveTo(p[0] + sx * L, p[1]); g.lineTo(p[0], p[1]); g.lineTo(p[0], p[1] + sy * L);
    });
    g.stroke(); g.restore();
    g.save(); g.setLineDash([6, 8]); g.strokeStyle = 'rgba(255,255,255,.45)'; g.lineWidth = 1.5; g.strokeRect(pr.x, pr.y, pr.w, pr.h); g.restore();
    corners(pr).forEach(function (p) {
      g.beginPath(); g.arc(p[0], p[1], 9, 0, Math.PI * 2); g.fillStyle = '#fff'; g.fill();
      g.lineWidth = 2; g.strokeStyle = 'rgba(0,0,0,.5)'; g.stroke();
    });
    if (st.brush && st.brushAt) {
      g.save(); g.beginPath(); g.arc(st.brushAt.x, st.brushAt.y, st.brushR, 0, Math.PI * 2);
      g.lineWidth = 4; g.strokeStyle = 'rgba(0,0,0,.55)'; g.stroke();
      g.lineWidth = 2; g.strokeStyle = st.brush === 'erase' ? '#fff' : 'rgba(170,220,255,.95)'; g.stroke(); g.restore();
    }
  }
  function drawMap(st) {
    var t = clock(), s = still(st);
    Array.prototype.forEach.call(st.el.querySelectorAll('[data-tkm-map] canvas'), function (c) {
      F.render(c.getContext('2d'), c.width, c.height, opts(st, t, s));
    });
  }
  function draw(st) {
    if (!st.img) return;
    drawWork(st); drawMap(st); syncZoom(st);
  }
  function syncZoom(st) {
    var z = st.el.querySelector('[data-tkm-zoom]');
    if (z) z.value = Math.round(st.place.img.s / baseScale(st) * 100);
  }
  function changed(st) {
    if (!st.dirty) { st.dirty = true; if (C.markDirty) C.markDirty('token-maker'); }
    draw(st); play(st);
  }

  // The frame under the pointer in the picker is drawn live with this picture,
  // over its still preview; every other preview stays a still.
  function drawHot(st) {
    if (!st.hot || !st.img) return;
    var c = st.hot.canvas, o = opts(st, clock(), !st.playing);
    o.frame = st.hot.id;
    F.render(c.getContext('2d'), c.width, c.height, o);
  }
  function moving(st) {
    return !!(st.img && st.playing && (design(st.frame).anim || (st.hot && design(st.hot.id).anim)));
  }

  // A moving frame plays at up to thirty frames a second and the map tokens and
  // the hovered preview at fifteen, and drawing never takes more than about half
  // the time, so the page stays responsive on a slow machine. The loop rests
  // with MotionRest and starts again on its wake.
  function tick(ts) {
    var st = cur;
    if (!st || st.closing) return;
    st.raf = 0;
    if (!moving(st) || resting()) return;
    if (ts >= st.nextTick) {
      var t0 = performance.now(), work = design(st.frame).anim;
      if (work) drawWork(st);
      if (++st.ticks % 2) { if (work) drawMap(st); if (st.hot && design(st.hot.id).anim) drawHot(st); }
      st.nextTick = ts + Math.max(30, (performance.now() - t0) * 2);
    }
    st.raf = requestAnimationFrame(tick);
  }
  function play(st) { if (st && !st.raf && !st.closing && moving(st)) st.raf = requestAnimationFrame(tick); }
  if (window.MotionRest) window.MotionRest.onWake(function () { play(cur); });

  /* ---------- Pointer: what was grabbed decides what moves ---------- */

  function toWork(st, e) { var r = st.surface.getBoundingClientRect(); return { x: (e.clientX - r.left) * F.W / r.width, y: (e.clientY - r.top) * F.W / r.height }; }
  function hitCorner(r, pt) {
    var c = corners(r);
    for (var i = 0; i < 4; i++) if (Math.abs(pt.x - c[i][0]) <= HANDLE && Math.abs(pt.y - c[i][1]) <= HANDLE) return true;
    return false;
  }
  function inside(r, pt) { return pt.x >= r.x && pt.x <= r.x + r.w && pt.y >= r.y && pt.y <= r.y + r.h; }
  function grab(st, pt) {
    var fr = frRect(st), pr = picRect(st);
    if (hitCorner(fr, pt)) return 'frame-size';
    if (hitCorner(pr, pt)) return 'pic-size';
    if (inside(fr, pt)) return 'frame-move';
    return 'pic-move';
  }
  var CURSOR = { 'frame-size': 'nwse-resize', 'pic-size': 'nwse-resize', 'frame-move': 'move', 'pic-move': 'grab' };

  function bindSurface(st) {
    var c = st.surface, drag = null, pinch = null, pts = new Map();
    c.addEventListener('pointermove', function (e) {
      if (!st.img) return;
      var pt = toWork(st, e);
      if (st.brush && !pinch) {
        if (drag) pts.set(e.pointerId, pt);
        st.brushAt = pt;
        if (drag && drag.kind === 'paint') { paintStroke(st, drag.last, pt); drag.last = pt; changed(st); }
        else { c.style.cursor = 'none'; drawWork(st); }
        return;
      }
      if (!drag) { c.style.cursor = CURSOR[grab(st, pt)]; return; }
      pts.set(e.pointerId, pt);
      var p = st.place;
      if (pts.size === 2 && pinch) {
        var a = Array.from(pts.values()), d = Math.hypot(a[0].x - a[1].x, a[0].y - a[1].y);
        p.img.s = Math.max(0.02, pinch.s * d / pinch.d); changed(st); return;
      }
      var dx = pt.x - drag.last.x, dy = pt.y - drag.last.y; drag.last = pt;
      if (drag.kind === 'frame-move') { p.fr.x += dx; p.fr.y += dy; keepFrameIn(st); }
      else if (drag.kind === 'pic-move') { p.img.x += dx; p.img.y += dy; }
      else if (drag.kind === 'frame-size') { p.fr.w = Math.max(60, 2 * Math.max(Math.abs(pt.x - p.fr.x), Math.abs(pt.y - p.fr.y))); keepFrameIn(st); }
      else if (drag.kind === 'pic-size') { p.img.s = Math.max(0.02, Math.hypot(pt.x - p.img.x, pt.y - p.img.y) / (Math.hypot(iw(st), ih(st)) / 2)); }
      changed(st);
    });
    c.addEventListener('pointerdown', function (e) {
      if (!st.img) return;
      c.setPointerCapture(e.pointerId);
      var pt = toWork(st, e); pts.set(e.pointerId, pt);
      if (pts.size === 2) { var a = Array.from(pts.values()); pinch = { d: Math.hypot(a[0].x - a[1].x, a[0].y - a[1].y), s: st.place.img.s }; return; }
      hideDone(st);
      if (st.brush && st.work) {
        drag = { kind: 'paint', last: pt }; st.brushAt = pt;
        paintStroke(st, pt, pt); changed(st);
        return;
      }
      drag = { kind: grab(st, pt), last: pt };
      if (drag.kind === 'pic-move') c.style.cursor = 'grabbing';
    });
    function up(e) { pts.delete(e.pointerId); if (pts.size < 2) pinch = null; if (!pts.size) drag = null; }
    c.addEventListener('pointerup', up); c.addEventListener('pointercancel', up);
    c.addEventListener('pointerleave', function () { if (st.brushAt && !drag) { st.brushAt = null; drawWork(st); } });
    // Zoom is gentle: one wheel tick is a small step.
    c.addEventListener('wheel', function (e) {
      if (!st.img) return;
      e.preventDefault(); var i = st.place.img; i.s = Math.max(0.02, i.s * (e.deltaY < 0 ? 1.05 : 1 / 1.05)); changed(st);
    }, { passive: false });
    c.addEventListener('keydown', function (e) {
      if (!st.img) return;
      var fr = st.place.fr, step = e.shiftKey ? 40 : 10, used = true;
      if (e.key === 'ArrowLeft') fr.x -= step; else if (e.key === 'ArrowRight') fr.x += step;
      else if (e.key === 'ArrowUp') fr.y -= step; else if (e.key === 'ArrowDown') fr.y += step;
      else if (e.key === '+' || e.key === '=') fr.w *= 1.05;
      else if (e.key === '-') fr.w /= 1.05; else used = false;
      if (used) { e.preventDefault(); keepFrameIn(st); changed(st); }
    });
  }

  /* ---------- The picker: a group, then one of its frames ---------- */

  // A frame's still preview is one tile of the sprite, in the engine's design order.
  function sprite(el, id) {
    var url = assets().previews, i = -1, rows = Math.ceil(F.designs.length / PV_COLS);
    for (var k = 0; k < F.designs.length; k++) if (F.designs[k].id === id) i = k;
    if (!url || i < 0) return;
    el.style.backgroundImage = 'url("' + url + '")';
    el.style.backgroundSize = (PV_COLS * 100) + '% ' + (rows * 100) + '%';
    el.style.backgroundPosition = ((i % PV_COLS) / (PV_COLS - 1) * 100) + '% ' + (rows > 1 ? Math.floor(i / PV_COLS) / (rows - 1) * 100 : 0) + '%';
  }
  function known(id) { return F.designs.some(function (d) { return d.id === id; }); }

  function buildGroups(st) {
    st.el.querySelector('[data-tkm-groups]').innerHTML = F.groups.map(function (g) {
      return '<button type="button" class="tkm-chip" data-g="' + esc(g.id) + '" title="' + esc(g.line) + '" aria-pressed="false">' + esc(g.name) + '</button>';
    }).join('');
  }
  function showGroup(st, gid) {
    var g = F.groups.filter(function (x) { return x.id === gid; })[0] || F.groups[0];
    st.group = g.id;
    Array.prototype.forEach.call(st.el.querySelectorAll('[data-tkm-groups] button'), function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-g') === g.id)); });
    var box = st.el.querySelector('[data-tkm-frames]');
    unhot(st);
    box.innerHTML = g.ids.filter(known).concat(['none']).map(function (id) {
      return '<button type="button" class="tkm-fr" data-v="' + esc(id) + '" aria-pressed="' + (id === st.frame) + '">' +
        '<span class="tkm-pv" aria-hidden="true"></span><span class="tkm-fr-n">' + esc(design(id).name) + '</span></button>';
    }).join('');
    Array.prototype.forEach.call(box.querySelectorAll('.tkm-fr'), function (b) { sprite(b.firstChild, b.getAttribute('data-v')); });
  }
  function chooseFrame(st, id) {
    st.frame = known(id) ? id : 'none';
    var g = groupOf(st.frame);
    if (g && g !== st.group) showGroup(st, g);
    Array.prototype.forEach.call(st.el.querySelectorAll('[data-tkm-frames] .tkm-fr'), function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-v') === st.frame)); });
    syncFrame(st);
  }
  function syncFrame(st) {
    var d = design(st.frame), note = d.blurb, mb = st.el.querySelector('[data-tkm-motion]');
    if (d.roundOnly && st.shape === 'square') note += ' Round only.';
    if (d.anim && !motionOK()) note += ' It holds still while motion is set to Calm or Off, and it is saved as a still picture either way.';
    else if (d.anim) note += ' The token is saved as its still picture.';
    st.el.querySelector('[data-tkm-note]').textContent = note;
    mb.hidden = !d.anim || !motionOK();
    mb.textContent = st.playing ? 'Pause motion' : 'Play motion';
    st.el.querySelector('[data-tkm-colours]').hidden = d.id === 'none';
  }

  // A moving frame that the pointer rests on (or keyboard focus reaches) plays
  // in its tile with this picture and colour; the still frames keep their
  // previews, which already show them as they are. Resting first, as a peek
  // does, means sweeping across the picker starts nothing.
  var HOT_MS = 250;
  function hot(st, btn) {
    var id = btn && btn.getAttribute('data-v');
    if (st.hot && st.hot.id === id) return;
    unhot(st);
    if (!st.img || !id || !design(id).anim || !st.playing) return;
    st.hotTimer = setTimeout(function () {
      if (cur !== st || !btn.isConnected) return;
      btn.querySelector('.tkm-pv').appendChild(st.hotCanvas);
      st.hot = { id: id, canvas: st.hotCanvas };
      drawHot(st); play(st);
    }, HOT_MS);
  }
  function unhot(st) {
    clearTimeout(st.hotTimer);
    st.hot = null;
    if (st.hotCanvas.parentNode) st.hotCanvas.parentNode.removeChild(st.hotCanvas);
  }

  /* ---------- Background: noticed, cleared and touched up ---------- */

  // Whether each picture's offer was answered, so it is asked once: the
  // newest hundred, by picture.
  function bgAnswers() {
    try { var v = JSON.parse(localStorage.getItem(STORE_BG) || '[]'); return Array.isArray(v) ? v : []; } catch (e) { return []; }
  }
  function bgAnswer(key) {
    var all = bgAnswers();
    for (var i = 0; i < all.length; i++) if (all[i] && all[i].k === key) return all[i];
    return null;
  }
  function bgRemember(st) {
    if (!st.bgKey) return;
    var all = bgAnswers().filter(function (a) { return a && a.k !== st.bgKey; });
    all.unshift(st.cut.auto ? { k: st.bgKey, a: 'remove', s: st.cut.strength } : { k: st.bgKey, a: 'keep' });
    try { localStorage.setItem(STORE_BG, JSON.stringify(all.slice(0, 100))); } catch (e) { /* private window */ }
  }
  function pictureKey(url) {
    try { var u = new URL(url, location.href); return u.pathname; } catch (e) { return ''; }
  }

  function freshCut() { return { auto: false, strength: 'medium', paint: null, painted: false, canvas: null, out: null }; }

  // The maker draws whichever picture is current; swapping keeps the framing,
  // since the copy may be smaller than the picture.
  function swapImg(st, img, k) {
    if (st.img === img) return;
    st.place.img.s *= st.imgK / k;
    st.img = img; st.imgK = k;
  }

  // A working copy of the picture, read once, for the cutout to look at. A
  // picture whose pixels cannot be read leaves the wand off.
  function prepCutout(st, img, key) {
    loadCutout().then(function (T) {
      if (cur !== st || st.orig !== img) return;
      var w0 = img.naturalWidth || img.width, h0 = img.naturalHeight || img.height, k = Math.min(1, CUT_PX / Math.max(w0, h0));
      var c = document.createElement('canvas'), w = Math.max(1, Math.round(w0 * k)), h = Math.max(1, Math.round(h0 * k)), px;
      c.width = w; c.height = h;
      try { var g = c.getContext('2d', { willReadFrequently: true }); g.drawImage(img, 0, 0, w, h); px = g.getImageData(0, 0, w, h); } catch (e) { return; }
      st.work = { px: px, k: w / w0, T: T };
      st.bg = T.detect(px);
      st.bgKey = key || '';
      st.el.querySelector('[data-tkm-wand]').hidden = false;
      var was = key ? bgAnswer(key) : null;
      if (was && was.a === 'remove' && st.bg.kind !== 'clear') {
        setAuto(st, true, T.strengths.indexOf(was.s) >= 0 ? was.s : 'medium', true);
        showOffer(st, 'again');
      } else if (!was && st.bg.sure) {
        showOffer(st, 'ask');
      }
      syncBg(st);
    }).catch(function () { /* the wand stays off; the maker works as before */ });
  }

  function cutActive(st) { return st.cut.auto || st.cut.painted; }

  // Draws the cleared copy for a rectangle of it (all of it by default) and
  // makes it, or the untouched picture, the one the token is drawn from.
  function recompose(st, x0, y0, x1, y1) {
    var W = st.work, cut = st.cut;
    if (!W) return;
    if (!cutActive(st)) { swapImg(st, st.orig, 1); return; }
    var w = W.px.width, h = W.px.height;
    if (!cut.canvas) {
      cut.canvas = document.createElement('canvas'); cut.canvas.width = w; cut.canvas.height = h;
      cut.out = cut.canvas.getContext('2d').createImageData(w, h);
      x0 = undefined;
    }
    if (x0 === undefined) { x0 = 0; y0 = 0; x1 = w; y1 = h; }
    x0 = Math.max(0, x0); y0 = Math.max(0, y0); x1 = Math.min(w, x1); y1 = Math.min(h, y1);
    if (x1 <= x0 || y1 <= y0) return;
    var auto = cut.auto ? W.T.mask(W.px, st.bg, cut.strength) : null;
    W.T.compose(W.px, st.bg, auto, cut.paint, cut.out, x0, y0, x1, y1);
    cut.canvas.getContext('2d').putImageData(cut.out, 0, 0, x0, y0, x1 - x0, y1 - y0);
    swapImg(st, cut.canvas, W.k);
  }

  // A change to the whole picture fades in over the old one, unless motion is Off.
  function fadeFrom(st) {
    if (st.el.classList.contains('is-still') || motionLevel() === 'off') return;
    var c = st.surface, ghost = document.createElement('canvas');
    ghost.width = c.width; ghost.height = c.height; ghost.className = 'tkm-ghost';
    ghost.getContext('2d').drawImage(c, 0, 0);
    c.parentNode.insertBefore(ghost, c.nextSibling);
    requestAnimationFrame(function () { requestAnimationFrame(function () { ghost.classList.add('is-gone'); }); });
    setTimeout(function () { ghost.remove(); }, 400);
  }

  // Clears the backdrop (or puts it back) at a strength. quiet is for
  // repeating the person's earlier answer on open, which is not a new edit.
  function setAuto(st, on, strength, quiet) {
    if (!st.work || st.bg.kind === 'clear') return;
    if (!quiet) fadeFrom(st);
    st.cut.auto = !!on;
    if (strength) st.cut.strength = strength;
    recompose(st);
    syncBg(st);
    if (quiet) draw(st); else { bgRemember(st); changed(st); }
  }
  function putBack(st) {
    fadeFrom(st);
    st.cut.auto = false; st.cut.paint = null; st.cut.painted = false;
    recompose(st);
    syncBg(st); bgRemember(st); changed(st);
  }

  // Erase or Restore along a stroke, in the copy's pixels.
  function paintStroke(st, a, b) {
    var W = st.work, cut = st.cut;
    if (!W) return;
    var w = W.px.width, h = W.px.height;
    if (!cut.paint) cut.paint = new Uint8Array(w * h);
    if (!cut.painted) { cut.painted = true; recompose(st); syncBg(st); }
    var p = st.place.img, s = p.s, v = st.brush === 'erase' ? 1 : 2, r = Math.max(1, st.brushR / s);
    function src(pt) { var dx = (pt.x - p.x) / s; return { x: (st.flip ? -dx : dx) + w / 2, y: (pt.y - p.y) / s + h / 2 }; }
    var A = src(a), B = src(b), len = Math.hypot(B.x - A.x, B.y - A.y), n = Math.max(1, Math.ceil(len / (r / 3)));
    var x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
    for (var k = 0; k <= n; k++) {
      var cx = A.x + (B.x - A.x) * k / n, cy = A.y + (B.y - A.y) * k / n;
      var ya = Math.max(0, Math.floor(cy - r)), yb = Math.min(h - 1, Math.ceil(cy + r)), xa = Math.max(0, Math.floor(cx - r)), xb = Math.min(w - 1, Math.ceil(cx + r));
      for (var y = ya; y <= yb; y++) for (var x = xa; x <= xb; x++) {
        var dx = x + 0.5 - cx, dy = y + 0.5 - cy;
        if (dx * dx + dy * dy <= r * r) cut.paint[y * w + x] = v;
      }
      if (xa <= xb && ya <= yb) { x0 = Math.min(x0, xa); y0 = Math.min(y0, ya); x1 = Math.max(x1, xb + 1); y1 = Math.max(y1, yb + 1); }
    }
    if (x1 > x0) recompose(st, x0, y0, x1, y1);
  }

  var OFFER_TEXT = {
    checker: 'This picture has a painted checkerboard behind it, so it won’t be see-through on the map.',
    flat: 'This picture has a plain backdrop behind it, so it won’t be see-through on the map.',
    done: 'Background removed.',
    again: 'Background removed, as you chose last time.'
  };
  // The note over the stage: the offer, then what was done with Undo, which
  // stays until the person moves on to framing or opens the wand.
  function showOffer(st, mode) {
    var box = st.el.querySelector('[data-tkm-offer]');
    st.offer = mode;
    if (!mode) { box.classList.remove('is-on'); box.hidden = true; return; }
    st.el.querySelector('[data-tkm-offer-t]').textContent = mode === 'ask' ? OFFER_TEXT[st.bg.kind] || OFFER_TEXT.flat : OFFER_TEXT[mode];
    st.el.querySelector('[data-tkm-ask]').hidden = mode !== 'ask';
    st.el.querySelector('[data-tkm-did]').hidden = mode === 'ask';
    if (box.hidden) {
      box.hidden = false;
      requestAnimationFrame(function () { requestAnimationFrame(function () { box.classList.add('is-on'); }); });
    }
  }
  function hideDone(st) { if (st.offer === 'done' || st.offer === 'again') showOffer(st, ''); }

  var KIND_NAME = { checker: 'the painted checkerboard', flat: 'the plain backdrop', busy: 'the colour round the edge' };
  function syncBg(st) {
    var el = st.el, cut = st.cut, bg = st.bg || { kind: 'busy' }, line;
    if (cut.auto) line = 'Cleared ' + (KIND_NAME[bg.kind] || KIND_NAME.busy) + '. Erase or Restore to touch up the edge.';
    else if (bg.kind === 'clear') line = 'This picture is already see-through round its edge. Erase paints away anything else.';
    else if (bg.kind === 'checker') line = 'There is a painted checkerboard behind this picture.';
    else if (bg.kind === 'flat') line = 'There is a plain backdrop behind this picture.';
    else line = 'No plain backdrop was found. Clearing takes what matches the colour round the edge.';
    el.querySelector('[data-tkm-bgs]').textContent = line;
    var rm = el.querySelector('[data-tkm-bgrm]');
    rm.hidden = cut.auto || bg.kind === 'clear';
    el.querySelector('[data-tkm-bgback]').hidden = !cutActive(st);
    Array.prototype.forEach.call(el.querySelectorAll('[data-tkm-str] button'), function (b) {
      b.disabled = !cut.auto; b.setAttribute('aria-pressed', String(b.getAttribute('data-v') === cut.strength));
    });
    Array.prototype.forEach.call(el.querySelectorAll('[data-tkm-brush] button'), function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-v') === st.brush)); });
    el.querySelector('[data-tkm-bsize]').disabled = !st.brush;
    el.querySelector('[data-tkm-bgd]').classList.toggle('is-brushing', !!st.brush);
    el.querySelector('[data-tkm-wand]').classList.toggle('is-on', cutActive(st));
    el.querySelector('[data-tkm-hint]').textContent = st.brush ?
      (st.brush === 'erase' ? 'Paint over what should go. ' : 'Paint over what should come back. ') + 'Choose Move to frame the token again.' : HINT;
    st.surface.style.cursor = st.brush ? 'none' : '';
  }
  function setBrush(st, v) {
    st.brush = v === 'erase' || v === 'restore' ? v : '';
    if (!st.brush) st.brushAt = null;
    syncBg(st); if (st.img) drawWork(st);
  }

  // The wand's drawer hangs from the wand, over the top of the stage.
  function bgDrawer(st, open) {
    var d = st.el.querySelector('[data-tkm-bgd]'), wand = st.el.querySelector('[data-tkm-wand]');
    if (open === !d.hidden) return;
    wand.setAttribute('aria-expanded', String(open));
    if (open) {
      hideDone(st);
      d.hidden = false;
      requestAnimationFrame(function () { requestAnimationFrame(function () { d.classList.add('is-on'); }); });
      var f = d.querySelector('button:not([hidden]):not([disabled])');
      if (f) f.focus({ preventScroll: true });
    } else {
      d.classList.remove('is-on'); d.hidden = true;
      setBrush(st, '');
    }
  }

  /* ---------- Controls ---------- */

  function setColour(st, value, name) {
    st.colour = value;
    Array.prototype.forEach.call(st.el.querySelectorAll('[data-tkm-colour]'), function (b) { b.setAttribute('aria-pressed', String(name !== '' && b.getAttribute('data-tkm-colour') === value)); });
    st.el.querySelector('[data-tkm-cname]').textContent = value ? (name || 'Your colour') : 'Original: each frame in its own colours.';
  }
  function setShape(st, v) {
    st.shape = v === 'square' ? 'square' : 'round';
    Array.prototype.forEach.call(st.el.querySelectorAll('[data-tkm-shape] button'), function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-v') === st.shape)); });
  }
  function fill(st) {
    var fr = st.place.fr;
    st.place.img = { x: fr.x, y: fr.y, s: Math.max(fr.w / iw(st), fr.h / ih(st)) };
  }

  function bindControls(st) {
    var el = st.el;
    el.addEventListener('click', function (e) {
      var t = e.target, b;
      if (t.closest('[data-tkm-close]')) { close(); return; }
      if (t.closest('[data-tkm-save]')) { save(); return; }
      if (!st.ready) return;
      if ((b = t.closest('[data-tkm-groups] button'))) {
        showGroup(st, b.getAttribute('data-g'));
        var first = el.querySelector('[data-tkm-frames] .tkm-fr');
        if (first && groupOf(st.frame) !== st.group) { chooseFrame(st, first.getAttribute('data-v')); changed(st); }
        return;
      }
      if ((b = t.closest('.tkm-fr'))) { chooseFrame(st, b.getAttribute('data-v')); changed(st); return; }
      if ((b = t.closest('[data-tkm-shape] button'))) { setShape(st, b.getAttribute('data-v')); syncFrame(st); changed(st); return; }
      if ((b = t.closest('[data-tkm-colour]'))) { setColour(st, b.getAttribute('data-tkm-colour'), b.getAttribute('data-n')); changed(st); return; }
      if (!st.img) return;
      if (t.closest('[data-tkm-yes]')) { setAuto(st, true); showOffer(st, 'done'); return; }
      if (t.closest('[data-tkm-no]')) { showOffer(st, ''); bgRemember(st); st.surface.focus({ preventScroll: true }); return; }
      if (t.closest('[data-tkm-undo]')) { putBack(st); showOffer(st, ''); st.surface.focus({ preventScroll: true }); return; }
      if (t.closest('[data-tkm-adjust]')) { bgDrawer(st, true); return; }
      if (t.closest('[data-tkm-offer-x]')) { showOffer(st, ''); st.surface.focus({ preventScroll: true }); return; }
      if (t.closest('[data-tkm-wand]')) { bgDrawer(st, st.el.querySelector('[data-tkm-bgd]').hidden); return; }
      if (t.closest('[data-tkm-bgd-close]')) { bgDrawer(st, false); st.el.querySelector('[data-tkm-wand]').focus({ preventScroll: true }); return; }
      if (t.closest('[data-tkm-bgrm]')) { setAuto(st, true); return; }
      if (t.closest('[data-tkm-bgback]')) { putBack(st); return; }
      if ((b = t.closest('[data-tkm-str] button'))) { setAuto(st, true, b.getAttribute('data-v')); return; }
      if ((b = t.closest('[data-tkm-brush] button'))) { setBrush(st, b.getAttribute('data-v')); return; }
      if (t.closest('[data-tkm-fill]')) { fill(st); changed(st); return; }
      if ((b = t.closest('[data-tkm-flip]'))) { st.flip = !st.flip; b.setAttribute('aria-pressed', String(st.flip)); changed(st); return; }
      if (t.closest('[data-tkm-motion]')) {
        st.playing = !st.playing; F.motion(st.playing); if (!st.playing) unhot(st); syncFrame(st); draw(st); play(st);
      }
    });
    el.querySelector('[data-tkm-any]').addEventListener('input', function (e) {
      if (st.ready && /^#[0-9a-f]{6}$/i.test(e.target.value)) { setColour(st, e.target.value, ''); changed(st); }
    });
    el.querySelector('[data-tkm-zoom]').addEventListener('input', function (e) {
      if (!st.img) return;
      st.place.img.s = baseScale(st) * Number(e.target.value) / 100; changed(st);
    });
    el.querySelector('[data-tkm-bsize]').addEventListener('input', function (e) {
      st.brushR = Number(e.target.value) || 26;
      var c = st.surface;
      if (st.img) { st.brushAt = st.brushAt || { x: c.width / 2, y: c.height / 2 }; drawWork(st); }
    });
    el.querySelector('[data-tkm-file]').addEventListener('change', function (e) {
      var f = e.target.files && e.target.files[0];
      e.target.value = '';
      if (!f) return;
      if (!/^image\/(png|jpeg|webp|gif)$/.test(f.type)) { msg('That file is not a picture. Choose a PNG, JPEG or WebP.', 'error'); return; }
      var url = URL.createObjectURL(f);
      loadPicture(url).then(function (img) {
        if (cur !== st) { URL.revokeObjectURL(url); return; }
        if (st.url) URL.revokeObjectURL(st.url);
        st.url = url;
        usePicture(st, img, 'file:' + f.name + ':' + f.size + ':' + f.lastModified); changed(st);
        msg('');
      }).catch(function () {
        URL.revokeObjectURL(url);
        msg('That picture could not be opened. Try another one.', 'error');
      });
    });
    var frames = el.querySelector('[data-tkm-frames]');
    frames.addEventListener('pointerover', function (e) { hot(st, e.target.closest('.tkm-fr')); });
    frames.addEventListener('focusin', function (e) { hot(st, e.target.closest('.tkm-fr')); });
    frames.addEventListener('pointerleave', function () { unhot(st); });
    frames.addEventListener('focusout', function (e) { if (!frames.contains(e.relatedTarget)) unhot(st); });
    el.addEventListener('keydown', function (e) {
      if (e.key !== 'Tab') return;
      var f = Array.prototype.filter.call(el.querySelectorAll(FOCUSABLE), function (n) { return !n.closest('[hidden]') && n.getClientRects().length && getComputedStyle(n).visibility !== 'hidden'; });
      if (!f.length) { e.preventDefault(); st.panel.focus(); return; }
      var first = f[0], last = f[f.length - 1], a = document.activeElement;
      if (e.shiftKey && (a === first || a === st.panel)) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && a === last) { e.preventDefault(); first.focus(); }
    });
  }

  // The first drawing of a material works it out at full size, which takes a
  // moment, so the wait note is painted before it starts.
  function usePicture(st, img, key) {
    st.img = st.orig = img; st.imgK = 1;
    st.work = null; st.bg = null; st.bgKey = ''; st.cut = freshCut(); st.brush = ''; st.brushAt = null;
    showOffer(st, '');
    if (!st.el.querySelector('[data-tkm-bgd]').hidden) bgDrawer(st, false);
    st.el.querySelector('[data-tkm-wand]').hidden = true;
    st.place = F.place(img);
    keepFrameIn(st);
    st.el.querySelector('[data-tkm-filel]').textContent = 'Another picture';
    Array.prototype.forEach.call(st.el.querySelectorAll('[data-tkm-pic]'), function (n) { n.disabled = false; });
    setTimeout(function () {
      if (cur !== st || st.img !== img) return;
      draw(st);
      st.el.querySelector('[data-tkm-wait]').hidden = true;
      st.el.querySelector('[data-tkm-save]').disabled = false;
      play(st);
      prepCutout(st, img, key);
    }, 30);
  }
  function noPicture(st, text) {
    var w = st.el.querySelector('[data-tkm-wait]');
    w.hidden = false;
    w.textContent = text;
    st.el.querySelector('[data-tkm-filel]').textContent = 'Choose a picture';
  }

  /* ---------- Opening and closing ---------- */

  // Calm fades the maker in; Off and a device asking for less motion show it at once.
  function motionLevel() {
    var h = document.documentElement;
    if (h.getAttribute('data-motion') === 'off' || (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches)) return 'off';
    return motionOK() ? 'full' : 'calm';
  }
  function sameOrigin(url) {
    try { return !!url && new URL(url, location.href).origin === location.origin; } catch (e) { return false; }
  }

  function onEscape(e) {
    if (e.key !== 'Escape' || !cur || e.defaultPrevented) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    var d = cur.el.querySelector('[data-tkm-bgd]');
    if (d && !d.hidden && !cur.el.querySelector('.ask-slip.is-on')) {
      bgDrawer(cur, false);
      cur.el.querySelector('[data-tkm-wand]').focus({ preventScroll: true });
      return;
    }
    close();
  }
  function onFocusIn(e) {
    if (cur && !cur.el.contains(e.target)) cur.panel.focus({ preventScroll: true });
  }

  var opening = false;
  function open(o) {
    o = o || {};
    if (cur || opening) return;
    opening = true;
    var from = o.from || document.activeElement;
    loadStyles().then(function () {
      opening = false;
      if (cur) return;
      var el = build(o), panel = el.querySelector('.tkm-p'), mem = remembered();
      var hc = document.createElement('canvas'), d = Math.min(2, window.devicePixelRatio || 1);
      hc.width = hc.height = Math.round(64 * d);
      hc.className = 'tkm-hot';
      var st = cur = {
        o: o, el: el, panel: panel, from: from, surface: el.querySelector('.tkm-surface'), tmp: document.createElement('canvas'), hotCanvas: hc,
        img: null, orig: null, imgK: 1, work: null, bg: null, bgKey: '', cut: freshCut(), offer: '', brush: '', brushAt: null, brushR: 26,
        place: null, url: '', group: '', hot: null, hotTimer: 0, frame: 'none', colour: '', shape: 'round', flip: false, name: o.name || '',
        playing: motionOK(), ready: false, dirty: false, saving: false, closing: false, raf: 0, ticks: 0, nextTick: 0, inert: []
      };
      document.body.appendChild(el);
      // The page underneath keeps its place but leaves the tab order and the reader's tree.
      Array.prototype.forEach.call(document.body.children, function (n) {
        if (n !== el && !n.inert && n.tagName !== 'SCRIPT') { n.inert = true; st.inert.push(n); }
      });
      document.documentElement.classList.add('tkm-lock');
      var lvl = motionLevel();
      if (lvl === 'full' && setOrigin(panel, from)) {
        requestAnimationFrame(function () { requestAnimationFrame(function () { el.classList.add('is-open'); }); });
      } else if (lvl === 'calm') {
        el.classList.add('is-calm');
        requestAnimationFrame(function () { requestAnimationFrame(function () { el.classList.add('is-open'); }); });
      } else {
        el.classList.add('is-open', 'is-still');
      }
      window.addEventListener('keydown', onEscape, true);
      document.addEventListener('focusin', onFocusIn);
      bindControls(st);
      panel.focus({ preventScroll: true });

      var art = sameOrigin(o.art) ? o.art : '';
      Promise.all([loadEngine(), loadPicture(art).catch(function () { return null; })]).then(function (r) {
        if (cur !== st) return;
        st.ready = true;
        st.el.querySelector('[data-tkm-file]').disabled = false;
        F.motion(st.playing);
        buildGroups(st);
        bindSurface(st);
        setShape(st, mem.shape);
        setColour(st, /^#[0-9a-f]{6}$/i.test(mem.colour || '') ? mem.colour : '', nameOf(mem.colour));
        var first = F.designs[0] ? F.designs[0].id : 'none';
        chooseFrame(st, known(mem.frame) ? mem.frame : first);
        if (!st.group) showGroup(st, F.groups[0].id);
        if (r[1]) usePicture(st, r[1], pictureKey(art));
        else noPicture(st, art ? 'The page picture could not be opened. Choose a picture to make the token from.' : 'This page has no picture yet. Choose one to make the token from.');
      }).catch(function () {
        if (cur !== st) return;
        noPicture(st, 'The frames could not be loaded. Close this and try again.');
      });
    });
  }
  function nameOf(hex) {
    for (var i = 0; i < COLOURS.length; i++) if (COLOURS[i][0] === hex) return COLOURS[i][1];
    return '';
  }

  // Closing with unsaved work asks in place first; force closes as it is.
  function close(force) {
    var st = cur;
    if (!st || st.closing) return;
    if (!force && st.saving) return;
    if (!force && st.dirty) {
      if (C.askSlip) {
        C.askSlip(st.panel, { ask: 'Discard this token?', detail: 'The frame and framing you chose have not been saved.', discard: 'Discard the token', onDiscard: function () { close(true); } });
        return;
      }
      if (!window.confirm('Discard this token? The frame and framing you chose have not been saved.')) return;
    }
    st.closing = true;
    cur = null;
    clearTimeout(st.hotTimer);
    if (st.raf) cancelAnimationFrame(st.raf);
    if (C.markClean) C.markClean('token-maker');
    window.removeEventListener('keydown', onEscape, true);
    document.removeEventListener('focusin', onFocusIn);
    st.inert.forEach(function (n) { n.inert = false; });
    document.documentElement.classList.remove('tkm-lock');
    if (st.url) URL.revokeObjectURL(st.url);
    var done = function () {
      st.el.remove();
      if (st.from && st.from.isConnected && st.from.focus) st.from.focus({ preventScroll: true });
      if (typeof st.o.onClose === 'function') st.o.onClose();
    };
    if (st.el.classList.contains('is-still') || !st.el.classList.contains('is-open')) { done(); return; }
    st.el.classList.remove('is-open');
    setTimeout(done, 180);
  }

  /* ---------- Saving ---------- */

  function toBlob(canvas) {
    return new Promise(function (resolve, reject) {
      try { canvas.toBlob(function (b) { if (b) resolve(b); else reject(new Error('blank')); }, 'image/png'); } catch (e) { reject(e); }
    });
  }
  function refused(r, fallback) {
    if (r.ok) return r.json().catch(function () { return {}; });
    return r.json().catch(function () { return {}; }).then(function (b) { throw new Error((b && b.message) || fallback); });
  }

  function save() {
    var st = cur;
    if (!st || !st.img || st.saving) return;
    var o = st.o, btn = st.el.querySelector('[data-tkm-save]'), label = btn.textContent;
    var toField = typeof o.save !== 'function';
    if (toField && (!o.campaignId || !o.entityId)) { msg('This token has nowhere to go. Close the maker and open it again from the page.', 'error'); return; }
    st.saving = true;
    btn.disabled = true;
    btn.innerHTML = '<i class="fa-solid fa-circle-notch fa-spin" aria-hidden="true"></i> Saving…';
    msg('');
    var c = document.createElement('canvas');
    c.width = c.height = TOKEN_PX;
    var made;
    try {
      F.render(c.getContext('2d'), TOKEN_PX, TOKEN_PX, opts(st, 0, true));
      made = toBlob(c);
    } catch (e) { made = Promise.reject(e); }
    made.catch(function (e) {
      // A picture from another site leaves the canvas unreadable.
      throw new Error(e && e.name === 'SecurityError' ? 'This picture comes from another site, so it cannot be made into a token. Choose another picture.' : 'The token could not be drawn. Try again.');
    }).then(function (blob) {
      var fd = new FormData();
      fd.append('file', blob, 'token.png');
      fd.append('usage_type', 'entity_image');
      if (o.campaignId) fd.append('campaign_id', o.campaignId);
      return C.apiFetch('/media/upload', { method: 'POST', body: fd }).then(function (r) { return refused(r, 'The token did not upload. Check your connection and try again.'); });
    }).then(function (m) {
      if (!m || !m.id) throw new Error('The token did not upload. Try again.');
      if (!toField) return Promise.resolve(o.save(m.id, m)).then(function () { return m; });
      var url = '/campaigns/' + encodeURIComponent(o.campaignId) + '/entities/' + encodeURIComponent(o.entityId) + '/fields';
      return C.apiFetch(url, { method: 'PUT', body: { fields_patch: (function () { var p = {}; p[TOKEN_FIELD] = m.id; return p; })() } })
        .then(function (r) { return refused(r, 'The token uploaded but could not be put on the page. Try again.'); })
        .then(function () { return m; });
    }).then(function (m) {
      remember(st);
      st.dirty = false;
      var ev = new CustomEvent('chronicle:token-saved', { bubbles: true, detail: { entityId: o.entityId || '', mediaId: m.id, url: m.url || '', field: toField ? TOKEN_FIELD : '' } });
      (st.from && st.from.isConnected ? st.from : document).dispatchEvent(ev);
      st.saving = false;
      close(true);
      if (C.notify) C.notify('Token saved.', 'success');
    }).catch(function (e) {
      st.saving = false;
      if (cur !== st) return;
      btn.disabled = false;
      btn.textContent = label;
      msg(e instanceof TypeError || !(e && e.message) ? 'The token was not saved. Check your connection and try again.' : e.message, 'error');
    });
  }

  C.tokenMaker = { open: open };
})();
