/**
 * dice_tray_stage.js: the 3D stage of the dice trays (Chronicle.diceTrayStage.Tray).
 *
 * The trays are flat drawn art (dice_tray_art.js); only the dice are 3D, from Chronicle's dice engine
 * (dice/engine.js): its sets, physics shapes, the turn that makes the decided face land, and each
 * set's landing show. Each drawn picture lies on a flat, unlit plane where the real wall stands, so
 * the dice pass behind the paper tower's front and show only through its window and arch. The camera
 * is orthographic, so the art stays flat and lines up with the physics at any size.
 *
 * The values are decided before a throw (the caller passes each die's value, or the stage decides
 * one with crypto), and the throw is simulated ahead and then turned so the decided face lands: a
 * tray never changes a result.
 *
 * Self-contained apart from the engine, so the Foundry module can load it with the art.
 */
(function () {
  'use strict';
  var W = typeof window !== 'undefined' ? window : globalThis;
  W.Chronicle = W.Chronicle || {};

var E = null, LIBS = null;
// The dice engine is the same module dice.js uses, named by the same bare URL so the browser runs one
// copy (see dice.ai.md). The hashed library URLs come from the layout's #chronicle-dice-src element, as
// dice.js reads them; a host page with no such element (Foundry) passes opts.engine and opts.libs.
function loadEngine(o) {
  o = o || {};
  var src = document.getElementById('chronicle-dice-src');
  var url = o.engine || '/static/js/dice/engine.js';
  var libs = o.libs || (src ? { three: src.getAttribute('data-three-src'), cannon: src.getAttribute('data-cannon-src') } : null);
  return import(url).then(function (mod) { if (libs && libs.three && libs.cannon) mod.configure(libs); E = mod; return mod; });
}
function ready(o) {
  if (!LIBS) LIBS = loadEngine(o).then(function (mod) { return mod.libs(); });
  LIBS.catch(function () { LIBS = null; });
  return LIBS;
}

var STEP = 1 / 180, GRAV = -85, ELEV = 32 * Math.PI / 180;
var SIZE = { tower: 0.56, bowl: 0.66, mat: 0.6 };
var A = W.Chronicle.diceTrayArt;
function rand() { var a = new Uint32Array(1); crypto.getRandomValues(a); return a[0] / 4294967296; }
function decide(sides) { return 1 + Math.floor(rand() * sides); }

// ---------- The bowl: a flat floor inside a ring of unseen wall, seen from straight above ----------
function bowlLayout(T) {
  var R = 3.2, s = { size: SIZE.bowl, R: R, S: 2 * (R + 1.05) }, B = [], N = 36;
  B.push(box(T, [6, 0.2, 6], [0, -0.2, 0], null, 'floor'));
  for (var i = 0; i < N; i++) {
    var an = i / N * Math.PI * 2;
    B.push(box(T, [0.4, 1.4, Math.PI * (R + 0.4) / N * 1.2], [Math.cos(an) * (R + 0.4), 1.1, Math.sin(an) * (R + 0.4)], qAxis(T, [0, 1, 0], -an), null));
  }
  // A gentle rise to the wall, unseen like the wall, so a die that ends against the side rolls back a little.
  var ph = 0.26;
  for (var j = 0; j < N; j++) {
    var aj = j / N * Math.PI * 2, q = qAxis(T, [0, 1, 0], -aj).multiply(qAxis(T, [0, 0, 1], ph));
    B.push(box(T, [0.62, 0.1, Math.PI * 2.9 / N * 1.25], [Math.cos(aj) * 2.85, 0.16 - 0.1 / Math.cos(ph), Math.sin(aj) * 2.85], q, 'floor'));
  }
  B.push(box(T, [6, 0.1, 6], [0, 3.2, 0], null, null));
  s.boxes = B;
  // In over the rim with a sideways run, so the dice go round and bounce off the sides.
  // In over the near rim, aimed across the bowl a little off centre, so the dice hit the far side,
  // come back off it and settle.
  s.spawn = function (i, n, r) {
    var an = 2.2 + i * 0.3 + (r() - 0.5) * 0.25, rr = R - 0.8, sp = 13 + r() * 3, aim = an + Math.PI + (r() - 0.5) * 0.9;
    return { p: [Math.cos(an) * rr, 1.1 + i * 0.3, Math.sin(an) * rr],
      v: [Math.cos(aim) * sp, -2, Math.sin(aim) * sp], w: [(r() - 0.5) * 30, (r() - 0.5) * 30, (r() - 0.5) * 30] };
  };
  s.rested = function (p) { return p.y < 0.9 && Math.hypot(p.x, p.z) < R - 0.1; };
  s.zone = { x0: -1.6, x1: 1.6, z0: -1.4, z1: 1.4 };
  // A smooth floor and a lively wall: the dice run and come back off the sides before they settle.
  s.grip = { floor: 0.06, floorBounce: 0.28, wallBounce: 0.5 };
  var e = s.S / 2;
  s.view = { dir: [0, 1, 0], up: [0, 0, -1], pts: [[-e, 0, -e], [e, 0, e], [-e, 0, e], [e, 0, -e]], margin: 0.02 };
  s.sun = [-6, 14, -5];
  return s;
}

// ---------- The table mat: the bowl's flat floor and unseen walls, made rectangular ----------
// Each roll comes in from the edge nearest its roller (lay.from, an angle on the mat) and rolls across.
function matLayout(T) {
  var hw = 4.6, hd = 3, s = { size: SIZE.mat, hw: hw, hd: hd, crowd: true }, B = [];
  B.push(box(T, [hw + 2, 0.2, hd + 2], [0, -0.2, 0], null, 'floor'));
  B.push(box(T, [0.5, 1.4, hd + 1], [-hw - 0.5, 1.1, 0], null, null)); B.push(box(T, [0.5, 1.4, hd + 1], [hw + 0.5, 1.1, 0], null, null));
  B.push(box(T, [hw + 1, 1.4, 0.5], [0, 1.1, -hd - 0.5], null, null)); B.push(box(T, [hw + 1, 1.4, 0.5], [0, 1.1, hd + 0.5], null, null));
  B.push(box(T, [hw + 1, 0.1, hd + 1], [0, 3.2, 0], null, null));
  s.boxes = B; s.from = Math.PI / 2;
  s.spawn = function (i, n, r) {
    var a = s.from + (r() - 0.5) * 0.5, dx = Math.cos(a), dz = Math.sin(a);
    var t = Math.min((hw - 0.6) / Math.max(1e-3, Math.abs(dx)), (hd - 0.6) / Math.max(1e-3, Math.abs(dz)));
    var off = (i - (n - 1) / 2) * 0.6, sp = 8 + r() * 4, aim = a + Math.PI + (r() - 0.5) * 0.7;
    return { p: [dx * t - dz * off, 0.9 + i * 0.25, dz * t + dx * off], v: [Math.cos(aim) * sp, -1, Math.sin(aim) * sp], w: [(r() - 0.5) * 30, (r() - 0.5) * 30, (r() - 0.5) * 30] };
  };
  s.rested = function (p) { return p.y < 0.9 && Math.abs(p.x) < hw - 0.1 && Math.abs(p.z) < hd - 0.1; };
  s.zone = { x0: -hw + 0.9, x1: hw - 0.9, z0: -hd + 0.9, z1: hd - 0.9 };
  s.grip = { floor: 0.1, floorBounce: 0.24, wallBounce: 0.4 };
  var ex = hw + 0.75, ez = hd + 0.75;
  s.view = { dir: [0, 1, 0], up: [0, 0, -1], pts: [[-ex, 0, -ez], [ex, 0, ez], [-ex, 0, ez], [ex, 0, -ez]], margin: 0.02 };
  s.sun = [-6, 14, -5];
  return s;
}

// The tower's view: from the front and a little above, so the paper front reads flat and the dice
// in the tray at its foot show their top faces.
function towerView(lay) {
  var d = lay.tower, tr = lay.tray, W = d.IW + d.t + A.OVER + 0.2, YT = d.H + A.CAP + A.MER + 0.15, zf = d.front + d.t, zb = d.Tz - d.ID - d.t;
  lay.view = { dir: [0, Math.sin(ELEV), Math.cos(ELEV)], up: [0, 1, 0], margin: 0.03,
    pts: [[d.Tx - W, YT, zf], [d.Tx + W, YT, zf], [d.Tx - W, d.H + A.CAP + 0.1, zb], [tr.x0 - 0.3, 0, tr.z1 + 0.3], [tr.x1 + 0.3, 0, tr.z1 + 0.3], [tr.x1 + 0.3, tr.wall, tr.z0 - 0.3], [tr.x0 - 0.3, 0, d.Tz - d.ID - d.t]] };
  lay.sun = [-5, 14, 7];
  return lay;
}

// ---------- The tray ----------
function Tray(opts) {
  this.opts = opts || {};
  this.style = this.opts.style || 'tower'; this.mode = 'solo';
  this.rolls = []; this.fx = []; this.ppu = 50;
  this.look = null;
}
Tray.prototype.init = function () {
  var self = this;
  return ready(self.opts).then(function (lib) {
    var T = self.T = lib.T, C = self.C = lib.C;
    var wrap = self.el = document.createElement('div'); wrap.className = 'tray3d' + (self.opts.cls ? ' ' + self.opts.cls : '');
    var cnv = self.canvas = document.createElement('canvas'); cnv.className = 'tray3d-canvas'; cnv.setAttribute('aria-hidden', 'true');
    var tags = self.tagLayer = document.createElement('div'); tags.className = 'tray3d-tags';
    wrap.appendChild(cnv); wrap.appendChild(tags);
    var r = self.renderer = new T.WebGLRenderer({ canvas: cnv, alpha: true, antialias: true, premultipliedAlpha: true });
    r.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    r.shadowMap.enabled = true; r.shadowMap.type = T.PCFSoftShadowMap;
    r.outputColorSpace = T.SRGBColorSpace; r.toneMapping = T.ACESFilmicToneMapping; r.toneMappingExposure = 1.05;
    self.env = E.environment(T, r);
    var sc = self.scene = new T.Scene(); sc.environment = self.env.pmrem;
    self.cam = new T.OrthographicCamera(-5, 5, 5, -5, 0.1, 300);
    sc.add(new T.HemisphereLight(0xffffff, 0x6a7080, 0.8));
    var sun = self.sun = new T.DirectionalLight(0xfff6ea, 2.4); sun.castShadow = true;
    sun.shadow.mapSize.set(2048, 2048); sun.shadow.bias = -0.0005; sun.shadow.normalBias = 0.03; sun.shadow.radius = 4;
    sc.add(sun); sc.add(sun.target);
    var fill = new T.DirectionalLight(0xdfe8ff, 0.7); fill.position.set(6, 6, 10); sc.add(fill);
    self.st = { T: T, scene: sc, sun: sun, cam: self.cam, fx: self.fx, q: { detail: 'hd', glow: false }, renderer: r, H: 400, ptSize: function (dia) { return dia * self.ppu; } };
    self.ro = new ResizeObserver(function () { self.resize(); });
    self.ro.observe(wrap);
    self.last = performance.now();
    var loop = function (now) { if (self.dead) return; self.frame(now); self.raf = requestAnimationFrame(loop); };
    self.raf = requestAnimationFrame(loop);
    return self;
  });
};
// Stops the loop and frees the WebGL context: the card is redrawn by the dock, and a page keeps only
// so many contexts.
Tray.prototype.destroy = function () {
  this.dead = true;
  if (this.raf) cancelAnimationFrame(this.raf);
  if (this.ro) this.ro.disconnect();
  if (this.rolls) this.clearAll(true);
  if (this.group) { this.scene.remove(this.group); disposeAll(this.group); }
  if (this.env && this.env.pmrem && this.env.pmrem.dispose) this.env.pmrem.dispose();
  if (this.renderer) { this.renderer.dispose(); if (this.renderer.forceContextLoss) this.renderer.forceContextLoss(); }
  if (this.el && this.el.parentNode) this.el.parentNode.removeChild(this.el);
};
// A world point on the flat mat or bowl, in pixels over the canvas: where a name tag or a flat die goes.
Tray.prototype.toPx = function (x, z) {
  var q = new this.T.Vector3(x, 0, z).project(this.cam);
  return { x: (q.x + 1) / 2 * this.W, y: (1 - q.y) / 2 * this.H };
};
Tray.prototype.mount = function (host) { if (this.el.parentNode !== host) host.appendChild(this.el); this.resize(); };
Tray.prototype.resize = function () {
  var w = Math.max(60, this.el.clientWidth), h = Math.max(60, this.el.clientHeight);
  if (w === this.W && h === this.H && this.lay) return;
  this.W = w; this.H = h; this.st.H = h;
  this.renderer.setSize(w, h, false);
  if (!this.lay) this.build();
  else this.fit();
};
// The orthographic camera along the layout's direction, framing its points with the window's shape.
Tray.prototype.fit = function () {
  var T = this.T, cam = this.cam, v = this.lay.view, dir = new T.Vector3().fromArray(v.dir).normalize();
  var P = v.pts.map(function (p) { return new T.Vector3().fromArray(p); }), look = new T.Vector3();
  P.forEach(function (p) { look.add(p); }); look.multiplyScalar(1 / P.length);
  cam.up.fromArray(v.up); cam.position.copy(look).addScaledVector(dir, 60); cam.lookAt(look); cam.updateMatrixWorld(true);
  var x0 = 1e9, x1 = -1e9, y0 = 1e9, y1 = -1e9, q = new T.Vector3();
  P.forEach(function (p) { q.copy(p).applyMatrix4(cam.matrixWorldInverse); x0 = Math.min(x0, q.x); x1 = Math.max(x1, q.x); y0 = Math.min(y0, q.y); y1 = Math.max(y1, q.y); });
  var cx = (x0 + x1) / 2, cy = (y0 + y1) / 2, hx = (x1 - x0) / 2 * (1 + v.margin), hy = (y1 - y0) / 2 * (1 + v.margin), asp = this.W / this.H;
  if (hx / hy < asp) hx = hy * asp; else hy = hx / asp;
  cam.near = 1; cam.far = 200;
  this.full = { l: cx - hx, r: cx + hx, t: cy + hy, b: cy - hy };
  this.content = { l: x0, r: x1, b: y0, t: y1 };
  this.setView(this.full);
};
Tray.prototype.setView = function (f) {
  var cam = this.cam; cam.left = f.l; cam.right = f.r; cam.top = f.t; cam.bottom = f.b; cam.updateProjectionMatrix();
  this.view = f; this.ppu = this.H / (f.t - f.b); this.idleDrawn = false;
};

// ---------- Building: the physics layout, the shadow the dice cast, and the drawn art ----------
Tray.prototype.build = function () {
  var T = this.T;
  this.clearAll(true);
  if (this.group) { this.scene.remove(this.group); disposeAll(this.group); this.art = null; }
  var lay;
  if (this.style === 'bowl') lay = bowlLayout(T);
  else if (this.style === 'mat') lay = matLayout(T);
  else lay = towerView(towerLayout(T));
  this.lay = lay;
  var g = this.group = new T.Group(); this.scene.add(g);
  this.sun.position.fromArray(lay.sun); this.sun.target.position.set(0, 0, 0);
  var ext = 8, sc = this.sun.shadow.camera;
  sc.left = -ext; sc.right = ext; sc.top = ext; sc.bottom = -ext; sc.near = 1; sc.far = 60; sc.updateProjectionMatrix();
  // The dice's shadows fall on an invisible sheet over the drawn floor.
  var catcher;
  if (this.style === 'tower') { var tr = lay.tray; catcher = new T.Mesh(new T.PlaneGeometry(tr.x1 - tr.x0, tr.z1 - tr.z0), new T.ShadowMaterial({ opacity: 0.3 })); catcher.position.set((tr.x0 + tr.x1) / 2, 0.003, (tr.z0 + tr.z1) / 2); }
  else if (this.style === 'mat') { catcher = new T.Mesh(new T.PlaneGeometry(lay.hw * 2 + 1, lay.hd * 2 + 1), new T.ShadowMaterial({ opacity: 0.3 })); catcher.position.y = 0.003; }
  else { catcher = new T.Mesh(new T.CircleGeometry(lay.R + 0.3, 64), new T.ShadowMaterial({ opacity: 0.3 })); catcher.position.y = 0.003; }
  catcher.rotation.x = -Math.PI / 2; catcher.receiveShadow = true; g.add(catcher);
  if (this.look) this.paint();
  if (this.W) this.fit();
};
function disposeAll(o) {
  o.traverse(function (m) {
    if (m.geometry) m.geometry.dispose();
    if (m.material) (Array.isArray(m.material) ? m.material : [m.material]).forEach(function (x) { if (x.map) x.map.dispose(); x.dispose(); });
  });
}
// Lays each drawn picture on its plane. Unlit and untoned, so the art shows exactly as painted.
Tray.prototype.paint = function () {
  var T = this.T, L = this.look, lay = this.lay, K = 100;
  if (this.art) { this.group.remove(this.art); disposeAll(this.art); }
  var a = this.art = new T.Group(); this.group.add(a); this.idleDrawn = false;
  function mat(c, o) { var t = new T.CanvasTexture(c); t.colorSpace = T.SRGBColorSpace; t.anisotropy = 4; return new T.MeshBasicMaterial(Object.assign({ map: t, toneMapped: false }, o || {})); }
  function plane(w, h, m, p, flat) { var me = new T.Mesh(new T.PlaneGeometry(w, h), m); me.position.set(p[0], p[1], p[2]); if (flat) me.rotation.x = -Math.PI / 2; a.add(me); return me; }
  if (this.style === 'mat') {
    var mw = (lay.hw + A.MAT_B + 0.25) * 2, md = (lay.hd + A.MAT_B + 0.25) * 2;
    plane(mw, md, mat(A.matArt(L, lay.hw, lay.hd, 100), { transparent: true, alphaTest: 0.02 }), [0, 0, 0], true);
    return;
  }
  if (this.style === 'bowl') {
    plane(lay.S, lay.S, mat(A.bowlArt(L, lay.R, 120, lay.S), { transparent: true, alphaTest: 0.02 }), [0, 0, 0], true);
    return;
  }
  var d = lay.tower, art = A.towerArt(L, Object.assign({ tray: lay.tray }, d), K), z = d.front + d.t;
  var f = art.front, o = art.over, i = art.inner, tp = art.top, fl = art.floor, rm = art.rims, lp = art.lip, bk = art.back;
  plane(f.x1 - f.x0, f.y1 - f.y0, mat(f.c, { alphaTest: 0.5 }), [(f.x0 + f.x1) / 2, (f.y0 + f.y1) / 2, z + 0.004]);
  plane(o.x1 - o.x0, o.y1 - o.y0, mat(o.c, { transparent: true, depthWrite: false }), [(o.x0 + o.x1) / 2, (o.y0 + o.y1) / 2, z + 0.008]).renderOrder = 10;
  plane(i.x1 - i.x0, i.y1 - i.y0, mat(i.c), [(i.x0 + i.x1) / 2, (i.y0 + i.y1) / 2, d.Tz - d.ID + 0.002]);
  plane(tp.x1 - tp.x0, tp.z1 - tp.z0, mat(tp.c, { alphaTest: 0.5 }), [(tp.x0 + tp.x1) / 2, tp.y, (tp.z0 + tp.z1) / 2], true);
  plane(fl.x1 - fl.x0, fl.z1 - fl.z0, mat(fl.c), [(fl.x0 + fl.x1) / 2, 0, (fl.z0 + fl.z1) / 2], true);
  plane(rm.x1 - rm.x0, rm.z1 - rm.z0, mat(rm.c, { alphaTest: 0.5 }), [(rm.x0 + rm.x1) / 2, rm.y + 0.002, (rm.z0 + rm.z1) / 2], true);
  plane(lp.x1 - lp.x0, lp.y1 - lp.y0, mat(lp.c), [(lp.x0 + lp.x1) / 2, (lp.y0 + lp.y1) / 2, lp.z]);
  plane(bk.x1 - bk.x0, bk.y1 - bk.y0, mat(bk.c), [(bk.x0 + bk.x1) / 2, (bk.y0 + bk.y1) / 2, bk.z + 0.002]);
  // Baffles and the exit ramp: strips of card, pale on top, darker on the edge that faces you.
  // Inside the tower it is dimmer than outside, and dimmer still down at the ramp.
  function set(dim) {
    var top = A.mix(A.mix(L.card2, L.inner, 0.3), '#000000', dim), edge = A.mix(A.mix(L.card2, L.edge, 0.55), '#000000', dim), side = A.mix(L.inner, '#000000', 0.25 + dim);
    return [side, side, top, side, edge, side].map(function (c) { return new T.MeshBasicMaterial({ color: c, toneMapped: false }); });
  }
  var baffle = set(0.05), ramp = set(0.12);
  lay.boxes.forEach(function (b) {
    if (b.look !== 'baffle' || b.hidden) return;
    var m = new T.Mesh(new T.BoxGeometry(b.h[0] * 2, b.h[1] * 2, b.h[2] * 2), b.p[1] < 2 ? ramp : baffle);
    m.position.set(b.p[0], b.p[1], b.p[2]); m.quaternion.copy(b.q); a.add(m);
  });
};

// ---------- Rolling ----------
// Rolls a list of dice: [{ sides, kind, colour, value }]. Options: calm (set down already settled), tag
// and color (a name tag over the dice, for the table tray), replace (a roller whose earlier dice make way). Resolves with the values when they land.
Tray.prototype.start = function (list, o) {
  var self = this, T = this.T;
  if (this.mode === 'solo') this.clearAll(); else this.trim(list.length);
  // Done here, not when the roll is asked for, so a roll still waiting its turn is replaced too.
  if (o.replace) this.leaveWho(o.replace);
  var dice = this.makeDice(list), want = list.map(function (d) { return d.value || decide(d.sides); });
  var roll = { dice: dice, want: want, tag: o.tag, color: o.color, who: o.who, state: 'wait' };
  if (o.from != null) this.lay.from = o.from;
  if (this.keepOnly && o.who && o.who !== this.keepOnly) roll.hideOnLand = true;
  dice.forEach(function (d) { self.scene.add(d.group); });
  this.addTag(roll);
  return new Promise(function (resolve) {
    roll.resolve = resolve;
    if (o.calm) {
      self.setDown(roll);
      dice.forEach(function (d) { fadeDie(d, 0); });
      roll.state = 'fadein'; roll.t0 = performance.now();
    } else {
      var plan = roll.plan = self.simulate(dice, null);
      roll.fix = dice.map(function (d, i) {
        var f = plan.frames[i], k = (plan.steps - 1) * 7;
        return E.symmetry(T, d.sd, E.faceOf(d, want[i]), plan.landed[i].face, new T.Quaternion(f[k + 3], f[k + 4], f[k + 5], f[k + 6]));
      });
      self.placeAt(roll, 0);
      roll.state = 'play'; roll.t0 = performance.now();
      if (self.style === 'tower') {
        var k = (plan.steps - 1) * 7, fin = plan.frames.map(function (f) { return new T.Vector3(f[k], f[k + 1], f[k + 2]); });
        roll.rest = fin;
      }
    }
    self.rolls.push(roll);
  });
};

// ---------- Each frame ----------
Tray.prototype.frame = function (now) {
  var self = this, dt = Math.min(0.05, Math.max(0, (now - this.last) / 1000)); this.last = now;
  if (!this.el.isConnected || !this.W || !this.lay) return;
  this.rolls.slice().forEach(function (r) {
    if (r.state === 'play') {
      var idx = Math.floor((now - r.t0) / 1000 / STEP * (self.speed || 1));
      self.placeAt(r, idx);
      if (idx >= r.plan.steps - 1) self.land(r);
    } else if (r.state === 'fadein') {
      var kf = Math.min(1, (now - r.t0) / 280);
      r.dice.forEach(function (d) { fadeDie(d, kf); });
      if (kf >= 1) self.land(r);
    } else if (r.state === 'leave') {
      var kl = 1 - Math.min(1, (now - r.t0) / 220);
      r.dice.forEach(function (d) { fadeDie(d, kl); });
      if (r.tagEl) r.tagEl.style.opacity = kl;
      if (kl <= 0) self.dropRoll(r);
    }
  });
  var run = this.fx.slice(); this.fx.length = 0;
  run.forEach(function (f) { if (f.update(dt)) self.fx.push(f); });
  this.rolls.forEach(function (r) { r.dice.forEach(function (d) { if (d.tick) d.tick(dt, 0); }); });
  this.placeTags();
  // Nothing moving and nothing changed: keep the last picture rather than drawing it again.
  var idle = !this.rolls.length && !this.fx.length;
  if (idle && this.idleDrawn) return;
  this.idleDrawn = idle;
  this.renderer.render(this.scene, this.cam);
};
Tray.prototype.setStyle = function (s) { if (s === this.style) return; this.style = s; this.chain = null; if (this.W) this.build(); else this.lay = null; };
Tray.prototype.setLook = function (L) { this.look = L; if (this.lay) this.paint(); };
Tray.prototype.setMode = function (m) { this.mode = m; };
// Fades out every roll not made by keep (the table mat with others' dice switched off).
Tray.prototype.showOthers = function () { this.keepOnly = null; };
Tray.prototype.hideOthers = function (keep) {
  var now = performance.now();
  // A roll still moving finishes first, so whoever waits on it hears the result, then fades.
  this.keepOnly = keep;
  this.rolls.forEach(function (r) { if (!r.who || r.who === keep) return; if (r.state === 'rest') { r.dice.forEach(function (d) { E.clearHighlight(d); }); r.state = 'leave'; r.t0 = now; } else r.hideOnLand = true; });
  this.idleDrawn = false;
};
// Fades out one roller's dice on the shared mat, so their next roll takes their place.
Tray.prototype.leaveWho = function (who) {
  var now = performance.now();
  this.rolls.forEach(function (r) {
    if (r.who !== who || r.state === 'leave') return;
    if (r.state === 'rest') { r.dice.forEach(function (d) { E.clearHighlight(d); }); r.state = 'leave'; r.t0 = now; } else r.hideOnLand = true;
  });
  this.idleDrawn = false;
};
// Where every die in the tray will rest: those already down, and this throw's.
Tray.prototype.allRest = function (extra) {
  var pts = extra.slice();
  this.rolls.forEach(function (r) { if (r.state === 'rest') r.dice.forEach(function (d) { pts.push(d.group.position.clone()); }); });
  return pts;
};
Tray.prototype.busy = function () { return this.rolls.some(function (r) { return r.state === 'play' || r.state === 'wait' || r.state === 'fadein'; }); };

// ---------- From the first pass: walls, physics, the recorded throw, landing, tags, clearing ----------
function qAxis(T, ax, ang) { return new T.Quaternion().setFromAxisAngle(new T.Vector3(ax[0], ax[1], ax[2]), ang); }
// A box: half sizes, centre, rotation (THREE quaternion), how it is drawn ('wall', 'floor', 'trim' or null for unseen).
function box(T, h, p, q, look) { return { h: h, p: p, q: q || new T.Quaternion(), look: look }; }
// A thin wall as drawn, with a much thicker one behind it for the physics (grown by g, in the box's own
// axes), so a fast die can never pass through or come to rest inside it.
function slab(B, T, h, p, q, look, g) {
  q = q || new T.Quaternion();
  var v = box(T, h, p, q, look); v.phys = false; B.push(v);
  var off = new T.Vector3(g[0] / 2, g[1] / 2, g[2] / 2).applyQuaternion(q);
  B.push(box(T, [h[0] + Math.abs(g[0]) / 2, h[1] + Math.abs(g[1]) / 2, h[2] + Math.abs(g[2]) / 2], [p[0] + off.x, p[1] + off.y, p[2] + off.z], q, look === 'wall' || look === 'rim' ? null : look, true));
  B[B.length - 1].look2 = look; B[B.length - 1].hidden = true;
}

function towerLayout(T) {
  var s = {}, t = 0.26, IW = 1.75, ID = 0.9, H = 7.6, Tx = -1.9, Tz = -1.35;
  var front = Tz + ID, AH = 1.55, AW = IW - 0.02;
  var tray = { x0: -4.5, x1: 4.6, z0: front + t, z1: 5.1, wall: 0.46 };
  s.size = SIZE.tower; s.H = H;
  var B = [];
  // The tower: two sides, a back, the front above and beside the arch (the glass is part of it).
  slab(B, T, [t / 2, H / 2, ID + t], [Tx - IW - t / 2, H / 2, Tz], null, 'wall', [-1.4, 0, 0]);
  slab(B, T, [t / 2, H / 2, ID + t], [Tx + IW + t / 2, H / 2, Tz], null, 'wall', [1.4, 0, 0]);
  slab(B, T, [IW + t, H / 2, t / 2], [Tx, H / 2, Tz - ID - t / 2], null, 'wall', [0, 0, -1.4]);
  B.push(box(T, [IW + t + 0.7, (H - AH) / 2, t / 2 + 0.7], [Tx, AH + (H - AH) / 2, front + t / 2 + 0.7], null, null));
  var post = (IW + t - AW) / 2;
  B.push(box(T, [post, AH / 2, t / 2], [Tx - AW - post, AH / 2, front + t / 2], null, null));
  B.push(box(T, [post, AH / 2, t / 2], [Tx + AW + post, AH / 2, front + t / 2], null, null));
  // Baffles: ramps across the full depth, sloping each way in turn, seen side-on through the window.
  var bl = 1.18, ang = 0.6;
  // dir -1 hangs from the left wall and runs down to the right; +1 the mirror. The high end sits in the wall.
  s.baffles = [[6.25, -1], [4.5, 1], [2.8, -1]];
  s.baffles.forEach(function (b) {
    var dir = b[1], cx = Tx + dir * (IW - bl * Math.cos(ang) + 0.05);
    slab(B, T, [bl, 0.07, ID + 0.12], [cx, b[0], Tz], qAxis(T, [0, 0, 1], dir * ang), 'baffle', [0, -0.12, 0]);
  });
  // The last ramp runs to the front, out through the arch, with a guide in each front corner.
  // Three pieces, each shallower than the last, so a die keeps its speed and tumbles out into the tray.
  [[Tz - ID - 0.1, 1.85, Tz - 0.1, 0.78], [Tz - 0.1, 0.78, Tz + 0.6, 0.24], [Tz + 0.6, 0.24, front + t + 0.5, 0.0]].forEach(function (r) {
    var len = Math.hypot(r[2] - r[0], r[3] - r[1]) / 2 + 0.05, a = Math.atan2(r[1] - r[3], r[2] - r[0]), n = 0.08;
    slab(B, T, [IW + 0.12, n, len], [Tx, (r[1] + r[3]) / 2 - n * Math.cos(a), (r[0] + r[2]) / 2 - n * Math.sin(a)], qAxis(T, [1, 0, 0], a), 'baffle', [0, -0.3, 0]);
  });
  // The tray.
  var cx = (tray.x0 + tray.x1) / 2, cz = (tray.z0 + tray.z1) / 2, hw = (tray.x1 - tray.x0) / 2, hd = (tray.z1 - tray.z0) / 2;
  slab(B, T, [hw + 0.24, 0.2, hd + 0.24], [cx, -0.2, cz], null, 'floor', [0, -1.4, 0]);
  B.push(box(T, [IW + t, 0.2, ID + t], [Tx, -0.2, Tz], null, 'wall'));
  // An unseen chimney above the open top: a die that bounces high off the first baffle falls back in.
  B.push(box(T, [t / 2, 1.6, ID + t], [Tx - IW - t / 2, H + 1.6, Tz], null, null));
  B.push(box(T, [t / 2, 1.6, ID + t], [Tx + IW + t / 2, H + 1.6, Tz], null, null));
  B.push(box(T, [IW + t, 1.6, t / 2], [Tx, H + 1.6, Tz - ID - t / 2], null, null));
  B.push(box(T, [IW + t, 1.6, t / 2], [Tx, H + 1.6, front + t / 2], null, null));
  slab(B, T, [0.12, tray.wall / 2, hd + 0.12], [tray.x0 - 0.12, tray.wall / 2, cz], null, 'rim', [-1.2, 0, 0]);
  slab(B, T, [0.12, tray.wall / 2, hd + 0.12], [tray.x1 + 0.12, tray.wall / 2, cz], null, 'rim', [1.2, 0, 0]);
  slab(B, T, [hw + 0.24, tray.wall / 2, 0.12], [cx, tray.wall / 2, tray.z1 + 0.12], null, 'rim', [0, 0, 1.2]);
  var bx0 = Tx + IW + t;
  B.push(box(T, [(tray.x1 - bx0) / 2 + 0.12, tray.wall / 2, 0.12], [(tray.x1 + bx0) / 2 + 0.06, tray.wall / 2, tray.z0 - 0.12], null, 'rim'));
  var lx1 = Tx - IW - t;
  if (lx1 > tray.x0) B.push(box(T, [(lx1 - tray.x0) / 2 + 0.12, tray.wall / 2, 0.12], [(lx1 + tray.x0) / 2 - 0.06, tray.wall / 2, tray.z0 - 0.12], null, 'rim'));
  // Above everything: a lid nobody sees, so a die can never leave the tray.
  B.push(box(T, [hw, 0.1, hd], [cx, 3.2, cz], null, null));
  s.boxes = B;
  s.tray = tray; s.tower = { Tx: Tx, Tz: Tz, IW: IW, ID: ID, H: H, t: t, AH: AH, AW: AW, front: front };
  s.spawn = function (i, n, r) {
    var k = i % 3, row = Math.floor(i / 3);
    return { p: [Tx - 0.9 + k * 0.42 + (r() - 0.5) * 0.15, H + 0.7 + row * 0.95 + k * 0.25, Tz + (r() - 0.5) * 0.7],
      v: [0.6 + r() * 0.5, -1.5, (r() - 0.5) * 0.6], w: [(r() - 0.5) * 22, (r() - 0.5) * 22, (r() - 0.5) * 22] };
  };
  // Leaving the arch a die picks up the speed the last ramp gives it, forward and mostly towards the
  // open end of the tray, each a little different, so a table's rolls spread out instead of piling up.
  // Stage 0 is at the arch's mouth (straight on, clear of its posts), stage 1 once it is out in the tray.
  s.kick = function (b, r, stage) {
    if (!stage) { b.velocity.set(b.velocity.x * 0.3, b.velocity.y + 0.5, Math.max(b.velocity.z, 0) * 0.5 + 5 + r() * 2.5); return; }
    var side = r() < 0.3 ? -(2 + r() * 6) : 3 + r() * 12.5;
    b.velocity.set(b.velocity.x + side, b.velocity.y + 0.8 + r() * 1.5, b.velocity.z + 0.5 + r() * 3);
    b.angularVelocity.set(b.angularVelocity.x + (r() - 0.5) * 14, b.angularVelocity.y + (r() - 0.5) * 6, b.angularVelocity.z - side * 0.9);
  };
  // A smooth tray floor, so the dice run on across it before they settle.
  s.grip = { floor: 0.07, floorBounce: 0.26, wallBounce: 0.4 };
  // On the tray floor, not on top of another die.
  s.rested = function (p) { return p.y < s.size * 1.15 && p.z > tray.z0 + 0.1 && p.x > tray.x0 && p.x < tray.x1 && p.z < tray.z1; };
  s.zone = { x0: tray.x0 + 0.6, x1: tray.x1 - 0.6, z0: tray.z0 + 1.3, z1: tray.z1 - 0.9 };
  var top = H + 1.6;
  s.frameFull = [[Tx - IW - t - 0.1, top, Tz], [Tx + IW + t, top, Tz], [tray.x0 - 0.2, 0, tray.z1 + 0.25], [tray.x1 + 0.25, 0, tray.z1 + 0.25], [tray.x1 + 0.25, tray.wall, tray.z0], [Tx - IW - t, 0, tray.z1]];
  s.frameRest = [[tray.x0, 0, tray.z1 + 0.2], [tray.x1, 0, tray.z1 + 0.2], [tray.x0, 2.6, tray.z0 - 0.4], [tray.x1, tray.wall, tray.z0 - 0.4]];
  s.camDir = [0.32, 0.66, 1]; s.camDirRest = [0.18, 1.35, 1];
  s.lookY = 0;
  return s;
}

// ---------- Physics ----------
function makeWorld(C, boxes, lay) {
  var gr = (lay && lay.grip) || {};
  var w = new C.World({ gravity: new C.Vec3(0, GRAV, 0) }); w.allowSleep = true;
  var dm = new C.Material('die'), wm = new C.Material('wall'), rm = new C.Material('ramp'), fm = new C.Material('floor');
  // cannon-es caps friction per contact point (from gravity, not the normal force), so a die on a face
  // with several points grips several times over: these are lower than real-world values for that reason.
  w.addContactMaterial(new C.ContactMaterial(dm, wm, { friction: 0.08, restitution: gr.wallBounce || 0.3 }));
  w.addContactMaterial(new C.ContactMaterial(dm, rm, { friction: 0.03, restitution: 0.25 }));
  w.addContactMaterial(new C.ContactMaterial(dm, fm, { friction: gr.floor || 0.16, restitution: gr.floorBounce || 0.22 }));
  w.addContactMaterial(new C.ContactMaterial(dm, dm, { friction: 0.06, restitution: 0.35 }));
  w.dm = dm; w.wm = wm;
  // The walls as three fixed bodies of many boxes (walls, ramps, floor), each with its own grip:
  // cheap, since each pair is rejected by its bounding sphere first.
  var bodies = { wall: new C.Body({ mass: 0, material: wm }), ramp: new C.Body({ mass: 0, material: rm }), floor: new C.Body({ mass: 0, material: fm }) };
  boxes.forEach(function (b) {
    if (b.phys === false) return;
    var lk = b.look2 || b.look, k = lk === 'baffle' ? 'ramp' : lk === 'floor' ? 'floor' : 'wall';
    bodies[k].addShape(new C.Box(new C.Vec3(b.h[0], b.h[1], b.h[2])), new C.Vec3(b.p[0], b.p[1], b.p[2]), new C.Quaternion(b.q.x, b.q.y, b.q.z, b.q.w));
  });
  Object.keys(bodies).forEach(function (k) { if (bodies[k].shapes.length) w.addBody(bodies[k]); });
  return w;
}

function dieBody(C, w, d) {
  return new C.Body({ mass: 1, material: w.dm, shape: d.shape, sleepSpeedLimit: 0.12, sleepTimeLimit: 0.5, linearDamping: 0.12, angularDamping: 0.22 });
}
// cannon-es can leave a sharp die (a d8 or d10 on its point) frozen on a slope, which a real die never
// does. A die that has stopped anywhere above the tray floor gets a small turn to send it on its way.
function unstick(bodies, r, lay) {
  bodies.forEach(function (b) {
    var high = lay.tower && b.position.y > 0.9;
    // In the tower a die high up that barely moves is stuck, even if it spins in place on a point.
    if ((b.position.y > 0.62 || !lay.rested(b.position)) && b.velocity.length() < 0.6 && (high || b.angularVelocity.length() < 1.5)) {
      b.wakeUp(); b.angularVelocity.set(b.angularVelocity.x + (r() - 0.5) * 9, b.angularVelocity.y + (r() - 0.5) * 3, b.angularVelocity.z + (r() - 0.5) * 9);
      // A pointed die can catch on a baffle or wedge against a wall; a small hop frees it.
      // On a baffle's low end it is sent on the way the baffle runs, off the end and down.
      if (high) {
        var run = 0; lay.baffles.forEach(function (bf) { if (Math.abs(bf[0] - b.position.y) < 0.9) run = -bf[1]; });
        b.velocity.set(b.velocity.x + run * 2.5 + (r() - 0.5) * 1.5, b.velocity.y + 1.2, b.velocity.z + (lay.tower.Tz - b.position.z) * 2.5);
      }
    }
  });
}
function addObstacles(C, w, list) {
  list.forEach(function (o) {
    var b = new C.Body({ mass: 0, material: w.dm, shape: o.shape });
    b.position.set(o.p.x, o.p.y, o.p.z); b.quaternion.set(o.q.x, o.q.y, o.q.z, o.q.w); w.addBody(b);
  });
}

// ---------- Fading a die in or out (Calm, and clearing the last roll) ----------
function fadeDie(die, k) {
  die.group.traverse(function (o) {
    if (!o.material || o.material.isShaderMaterial) return;
    (Array.isArray(o.material) ? o.material : [o.material]).forEach(function (m) {
      if (m.userData.op == null) { m.userData.op = m.opacity; m.userData.tr = m.transparent; }
      var tr = k < 1 ? true : m.userData.tr;
      if (m.transparent !== tr) { m.transparent = tr; m.needsUpdate = true; }
      m.opacity = m.userData.op * k;
    });
  });
}

Tray.prototype.makeDice = function (list) {
  var T = this.T, C = this.C, self = this, size = this.lay.size * (list.length > 6 ? 0.86 : 1);
  return list.map(function (d) {
    var kind = E.hasSet(d.kind) ? d.kind : 'jewelled', die = E.makeDie(T, C, d.sides, size, kind, E.coloursOf(kind, d.colour), 'rune', 384, { detail: 'hd', gems: true, env: self.env });
    die.group.traverse(function (o) { if (o.isMesh) o.castShadow = true; });
    return die;
  });
};
Tray.prototype.resting = function () {
  var out = [];
  this.rolls.forEach(function (r) { if (r.state === 'rest') r.dice.forEach(function (d) { out.push({ shape: d.shape, p: d.group.position, q: d.group.quaternion }); }); });
  return out;
};
// The throw, simulated ahead and kept frame by frame. A throw where a die stays on a baffle, leaves the
// tray or rests on an edge is thrown again (up to eight times; fourteen in the tower, where a crowded
// throw is thrown again too), so the one shown always lands clean.
Tray.prototype.simulate = function (dice, script) {
  var T = this.T, C = this.C, lay = this.lay, obst = this.resting(), best = null, MAX = Math.round(6 / STEP);
  for (var attempt = 0, tries = lay.kick ? 14 : lay.crowd ? 10 : 8; attempt < tries; attempt++) {
    var w = makeWorld(C, lay.boxes, lay); addObstacles(C, w, obst);
    var cup = null, pose = null;
        var r = Math.random, bodies = dice.map(function (d, i) {
      var b = dieBody(C, w, d), s = lay.spawn(i, dice.length, r);
      b.position.set(s.p[0], s.p[1], s.p[2]); b.quaternion.setFromEuler(r() * 6.28, r() * 6.28, r() * 6.28);
      b.velocity.set(s.v[0], s.v[1], s.v[2]); b.angularVelocity.set(s.w[0], s.w[1], s.w[2]);
      w.addBody(b); return b;
    });
    var kicked = dice.map(function () { return 0; }), exitZ = lay.tower ? [lay.tower.front + lay.tower.t + 0.05, lay.tower.front + lay.tower.t + 0.14] : 0;
    var frames = dice.map(function () { return new Float32Array(MAX * 7); }), cf = cup ? new Float32Array(MAX * 7) : null, steps = 0, sdone = !script;
    for (; steps < MAX; steps++) {
      w.step(STEP);
      if (steps % 12 === 0) unstick(bodies, r, lay);
      if (lay.kick) bodies.forEach(function (b, i) { if (kicked[i] < 2 && b.position.z > exitZ[kicked[i]] && b.position.y < 1.4) { lay.kick(b, r, kicked[i]); kicked[i]++; } });
      bodies.forEach(function (b, i) { var o = steps * 7, f = frames[i]; f[o] = b.position.x; f[o + 1] = b.position.y; f[o + 2] = b.position.z; f[o + 3] = b.quaternion.x; f[o + 4] = b.quaternion.y; f[o + 5] = b.quaternion.z; f[o + 6] = b.quaternion.w; });
      if (steps > 60 && sdone && bodies.every(function (b) { return b.sleepState === C.Body.SLEEPING; })) { steps++; break; }
    }
    // Play ends once nothing visibly moves any more, which comes a little before the bodies sleep.
    var end = Math.min(steps, MAX), still = end;
    for (var sI = end - 1; sI > 0; sI--) {
      var moved = frames.some(function (f) { var a = sI * 7, b2 = (sI - 1) * 7; return Math.abs(f[a] - f[b2]) + Math.abs(f[a + 1] - f[b2 + 1]) + Math.abs(f[a + 2] - f[b2 + 2]) > 0.0015 || Math.abs(f[a + 3] - f[b2 + 3]) + Math.abs(f[a + 4] - f[b2 + 4]) + Math.abs(f[a + 5] - f[b2 + 5]) + Math.abs(f[a + 6] - f[b2 + 6]) > 0.0015; });
      if (moved) { still = sI + 1; break; }
    }
    steps = Math.max(2, still);
    var qa = new T.Quaternion(), pa = new T.Vector3();
    var landed = frames.map(function (f, i) { var o = (steps - 1) * 7; return E.landedFace(dice[i], qa.set(f[o + 3], f[o + 4], f[o + 5], f[o + 6])); });
    var finals = frames.map(function (f) { var o = (steps - 1) * 7; return new T.Vector3(f[o], f[o + 1], f[o + 2]); });
    var worst = Math.min.apply(null, landed.map(function (l) { return l.dot; }));
    var out = finals.filter(function (p) { return !lay.rested(p); }).length;
    // How close this throw's dice end to each other and to dice already down: a crowded throw is
    // thrown again, so each roller's dice and name stay clear of the others'.
    var near = 1e9, others = obst.map(function (o) { return o.p; }), gap = lay.size * 2.5;
    finals.forEach(function (p, i) {
      finals.forEach(function (q, j) { if (j > i) near = Math.min(near, Math.hypot(p.x - q.x, p.z - q.z) + 0.6); });
      others.forEach(function (q) { near = Math.min(near, Math.hypot(p.x - q.x, p.z - q.z)); });
    });
    var crowd = (lay.kick || lay.crowd) && near < gap ? 1 - near / gap : 0;
    // Against the tray's front or far wall a die hides behind the rim, and at the arch's mouth it sits in
    // the tower's shade, so both count as crowded too.
    if (lay.kick) finals.forEach(function (p) { var zn = lay.zone; if (p.z > zn.z1 || p.x > zn.x1) crowd += 0.25; else if (p.z < zn.z0) crowd += 0.3; });
    var settled = end < MAX, res = { frames: frames, cup: cf, steps: Math.min(steps, MAX), landed: landed, score: worst - out - (settled ? 0 : 1) - crowd, attempts: attempt + 1 };
    if (!best || res.score > best.score) best = res;
    if (settled && !out && worst > 0.96 && !crowd) break;
  }
  return best;
};

// Rolls a list of dice: [{ sides, kind, colour, value }]. Options: calm (set down already settled),
// tag and color (a name tag over the dice, for the shared tray). Resolves with the values when they land.
Tray.prototype.roll = function (list, o) {
  var self = this; o = o || {};
  var sets = list.map(function (d) { return d.kind; }).filter(function (k) { return k; });
  var go = function () { return E.loadSets(sets).then(function () { return self.start(list, o); }); };
  this.chain = (this.chain || Promise.resolve()).then(go, go);
  return this.chain;
};

Tray.prototype.placeAt = function (roll, idx) {
  var qs = new this.T.Quaternion(), plan = roll.plan, k = Math.min(idx, plan.steps - 1) * 7;
  roll.dice.forEach(function (d, i) {
    var f = plan.frames[i];
    d.group.position.set(f[k], f[k + 1], f[k + 2]);
    d.group.quaternion.copy(qs.set(f[k + 3], f[k + 4], f[k + 5], f[k + 6]).multiply(roll.fix[i]));
  });
};
// Calm: each die set down where it rests, the decided face up and its number upright, in the free
// space nearest the middle of the tray.
Tray.prototype.setDown = function (roll) {
  var T = this.T, z = this.lay.zone, taken = this.resting().map(function (o) { return o.p.clone(); });
  var size = roll.dice[0].size, gap = size * 2.7, cx = (z.x0 + z.x1) / 2, cz = (z.z0 + z.z1) / 2, cand = [];
  for (var x = z.x0; x <= z.x1 + 1e-6; x += gap * 0.5) for (var y = z.z0; y <= z.z1 + 1e-6; y += gap * 0.5) cand.push(new T.Vector3(x, 0, y));
  cand.sort(function (a, b) { return Math.hypot(a.x - cx, a.z - cz) - Math.hypot(b.x - cx, b.z - cz); });
  roll.dice.forEach(function (d, i) {
    var at = cand.find(function (c) { return taken.every(function (t) { return Math.hypot(t.x - c.x, t.z - c.z) >= gap; }); }) || cand[i % cand.length];
    taken.push(at);
    var f = d.faces[E.faceOf(d, roll.want[i])].f;
    var q = new T.Quaternion().setFromUnitVectors(new T.Vector3(f.n.x, f.n.y, f.n.z), new T.Vector3(0, d.sides === 4 ? -1 : 1, 0));
    var fu = new T.Vector3(f.up.x, f.up.y, f.up.z).applyQuaternion(q), yaw = d.sides === 4 ? 0 : Math.atan2(fu.x, -fu.z);
    q.premultiply(new T.Quaternion().setFromAxisAngle(new T.Vector3(0, 1, 0), yaw + (i % 2 ? -0.16 : 0.12)));
    d.group.quaternion.copy(q); d.group.position.set(at.x, d.rest, at.z);
  });
};
Tray.prototype.land = function (roll) {
  var self = this; roll.state = 'rest';
  this.scene.updateMatrixWorld(true);
  // Each set's own way of lighting the landed number. A show that fails leaves the die unlit, never the tray broken.
  roll.dice.forEach(function (d, i) { try { self.fx.push(E.highlight(self.st, d, E.faceOf(d, roll.want[i]))); } catch (e) { console.warn(e); } });
  if (roll.resolve) roll.resolve({ values: roll.want.slice(), attempts: roll.plan ? roll.plan.attempts : 0 });
  if (roll.hideOnLand) { roll.state = 'leave'; roll.t0 = performance.now(); }
};

// ---------- Name tags over each roller's dice ----------
Tray.prototype.addTag = function (roll) {
  if (!roll.tag) return;
  var el = document.createElement('span'); el.className = 'tray3d-tag'; el.textContent = roll.tag;
  if (roll.color) el.style.setProperty('--tag', roll.color);
  this.tagLayer.appendChild(el); roll.tagEl = el;
};
Tray.prototype.placeTags = function () {
  var self = this, placed = [], others = [];
  this.rolls.forEach(function (r) { r.dice.forEach(function (d) { var q = d.group.position.clone().project(self.cam); others.push({ r: r, x: (q.x + 1) / 2 * self.W, y: (1 - q.y) / 2 * self.H, s: d.size * self.ppu * 0.6 }); }); });
  this.rolls.forEach(function (r) {
    if (!r.tagEl) return;
    // In the tower a roll's name shows once its dice are out of the arch; behind the paper they are unseen.
    var hidden = self.style === 'tower' && r.dice.some(function (d) { return d.group.position.z < -0.2; });
    r.tagEl.style.opacity = hidden ? '0' : '';
    if (hidden) return;
    // Over the roll's die that sits highest on screen. If that spot would cover any other die or another
    // name, it goes below, then beside, and only then stacks upwards.
    var best = null; r.dice.forEach(function (d) { var q = d.group.position.clone().project(self.cam); if (!best || q.y > best.y) best = q; });
    var bx = (best.x + 1) / 2 * self.W, by = (1 - best.y) / 2 * self.H, s = r.dice[0].size * self.ppu * 0.62;
    var w = r.tagEl.offsetWidth || 60, h = r.tagEl.offsetHeight || 20;
    function hits(x, y) {
      for (var i = 0; i < placed.length; i++) { var q = placed[i]; if (Math.abs(q.x - x) < (q.w + w) / 2 + 2 && Math.abs(q.y - y) < h + 2) return true; }
      for (var j = 0; j < others.length; j++) { var o = others[j]; if ((o.r !== r || o.x !== bx || o.y !== by) && Math.abs(o.x - x) < w / 2 + o.s && y > o.y - o.s && y - h < o.y + o.s) return true; }
      return false;
    }
    var spots = [[bx, by - s - 2], [bx, by + s + h + 2], [bx - s - w / 2 - 2, by + h / 2], [bx + s + w / 2 + 2, by + h / 2]], x = bx, y = by - s - 2;
    for (var k = 0; k < spots.length; k++) { if (!hits(spots[k][0], spots[k][1])) { x = spots[k][0]; y = spots[k][1]; break; } }
    if (k === spots.length) for (var pass = 0; pass < 12 && hits(x, y); pass++) y -= h + 2;
    y = Math.max(h, y);
    placed.push({ x: x, y: y, w: w });
    r.tagEl.style.transform = 'translate(' + Math.round(x) + 'px,' + Math.round(y) + 'px) translate(-50%,-100%)';
  });
};

// ---------- Clearing ----------
Tray.prototype.dropRoll = function (r) {
  var self = this;
  r.dice.forEach(function (d) { E.disposeDie(d); });
  if (r.tagEl && r.tagEl.parentNode) r.tagEl.parentNode.removeChild(r.tagEl);
  this.rolls = this.rolls.filter(function (x) { return x !== r; });
};
Tray.prototype.clearAll = function (now) {
  var self = this;
  if (this.live && now) { this.live = null; }
  this.rolls.slice().forEach(function (r) {
    if (r.state === 'live' && !now) return;
    if (r.resolve && r.state !== 'rest') r.resolve({ values: r.want.slice(), cancelled: true });
    if (now) self.dropRoll(r); else if (r.state !== 'leave') { r.dice.forEach(function (d) { E.clearHighlight(d); }); r.state = 'leave'; r.t0 = performance.now(); }
  });
  this.fx.length = 0;
};
// The shared tray holds the latest rolls, up to ten dice; the oldest go to make room.
Tray.prototype.trim = function (incoming) {
  var count = incoming, keep = [];
  for (var i = this.rolls.length - 1; i >= 0; i--) { var r = this.rolls[i]; if (r.state === 'leave') continue; count += r.dice.length; if (count > 10) { r.dice.forEach(function (d) { E.clearHighlight(d); }); r.state = 'leave'; r.t0 = performance.now(); } }
};

  W.Chronicle.diceTrayStage = { Tray: Tray, ready: ready, engine: function () { return E; } };
})();
