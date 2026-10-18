// ==================== Gothic dice ====================
// Blackened iron tracery with stained glass between the ribs. Every face is a small cathedral window:
// an iron frame following the face's outline, pointed lancets reaching into its corners, round or
// foiled lights in the iron between them, and the number on a plate of its own at the centre. The panes
// are translucent glass (alpha well below 1) with a lead line round each; the iron is opaque.
// The number is a sharp serif built from the stroke numerals: each open stroke end flares into a
// wedge, drawn in silver with a dark edge so it reads over anything.
import {
  canvas, hexA, isLight, mix, mulberry, registerSet, strokePaths, strokeW
} from '../engine.js';

// Colours. `glass` lists the two colours the lancets and round lights take in turn, then the ground between them; `ga` is how opaque the
// glass is; `iron`, `wear` and `plate` are the frame, its bright worn edges and the number's plate;
// `trim` is the fine metal line round the plate; `ink` is the number and `edge` its dark outline; `tint`
// and `alpha` give the coloured shadow the glass throws; `glow` is how much of its own light the glass
// carries (daylight through the pane), `burn` is the colour a lit number takes.
var gothColours = [
  { key: 'cathedral', name: 'Cathedral', note: 'Ruby, sapphire and gold glass in blackened iron', burn: '#ffc66b', ink: '#eceaf0', edge: '#0c0b0f',
    tint: '#ff4a64', alpha: 0.18, base: { grad: ['#55535c', '#1b1a1f'], rough: 0.52 },
    iron: '#4a4850', wear: '#a9a7b3', plate: '#2b292f', trim: '#d2ab52', glass: ['#b5142e', '#e6a91f', '#1f55c2'], ga: 0.6, glow: 0.3 },
  { key: 'nightshade', name: 'Nightshade', note: 'Violet and green glass in iron gone blue-black', burn: '#c490ff', ink: '#ece6f7', edge: '#0b0a12',
    tint: '#a35cff', alpha: 0.18, base: { grad: ['#4f4b62', '#19182a'], rough: 0.52 },
    iron: '#45435a', wear: '#a3a0c2', plate: '#272538', trim: '#b9a4e8', glass: ['#1e9a5c', '#c04ed0', '#5a2db0'], ga: 0.6, glow: 0.3 },
  { key: 'ember', name: 'Ember', note: 'Amber and red glass in scorched, rust-dark iron', burn: '#ff9a4a', ink: '#f5eadb', edge: '#120907',
    tint: '#ff8a30', alpha: 0.18, base: { grad: ['#5c4a44', '#211513'], rough: 0.54 },
    iron: '#544039', wear: '#c0a58e', plate: '#33211c', trim: '#e0a24a', glass: ['#f2b52a', '#e9780f', '#b81f16'], ga: 0.6, glow: 0.34 },
  { key: 'frost', name: 'Frost chapel', note: 'Pale blue and white glass in pewter', burn: '#bfe8ff', ink: '#1e2636', edge: '#f4f8fc',
    tint: '#8cc4ee', alpha: 0.16, base: { grad: ['#c9cfd9', '#8d95a3'], rough: 0.46 },
    iron: '#8c93a2', wear: '#eef2f7', plate: '#b7bdc9', trim: '#4b5668', glass: ['#e4f1fa', '#4f93cf', '#8ec5ee'], ga: 0.62, glow: 0.26 },
  { key: 'verdigris', name: 'Verdigris', note: 'Teal and amber glass in weathered green bronze', burn: '#8ffbd8', ink: '#f1efe0', edge: '#07100d',
    tint: '#2ab89c', alpha: 0.18, base: { grad: ['#4d6a60', '#17261f'], rough: 0.5 },
    iron: '#476459', wear: '#a6cdb8', plate: '#223630', trim: '#d4b25a', glass: ['#e0961c', '#2b6fb0', '#139a86'], ga: 0.6, glow: 0.3 }
];

// ---------- Small geometry ----------
function gothArea(p) {
  var a = 0, n = p.length;
  for (var i = 0; i < n; i++) { var q = p[(i + 1) % n]; a += p[i][0] * q[1] - q[0] * p[i][1]; }
  return a / 2;
}
// A convex polygon moved in by d (out when d is negative): each edge slid along its normal, corners
// found again where neighbouring edges meet.
function gothInset(poly, d) {
  var n = poly.length, s = gothArea(poly) >= 0 ? 1 : -1, ln = [], i;
  for (i = 0; i < n; i++) {
    var a = poly[i], b = poly[(i + 1) % n], ex = b[0] - a[0], ey = b[1] - a[1], L = Math.hypot(ex, ey) || 1;
    ln.push({ x: a[0] - ey / L * d * s, y: a[1] + ex / L * d * s, dx: ex, dy: ey });
  }
  var out = [];
  for (i = 0; i < n; i++) {
    var p = ln[(i + n - 1) % n], q = ln[i], den = p.dx * q.dy - p.dy * q.dx;
    var t = ((q.x - p.x) * q.dy - (q.y - p.y) * q.dx) / (den || 1e-9);
    out.push([p.x + p.dx * t, p.y + p.dy * t]);
  }
  return out;
}
// Distance from a point to a convex polygon's outline: negative inside, positive outside.
function gothDist(poly, x, y) {
  var n = poly.length, s = gothArea(poly) >= 0 ? 1 : -1, best = 1e9, inside = true;
  for (var i = 0; i < n; i++) {
    var a = poly[i], b = poly[(i + 1) % n], ex = b[0] - a[0], ey = b[1] - a[1], l2 = ex * ex + ey * ey || 1e-9;
    var t = Math.max(0, Math.min(1, ((x - a[0]) * ex + (y - a[1]) * ey) / l2));
    var d = Math.hypot(a[0] + ex * t - x, a[1] + ey * t - y);
    if (d < best) best = d;
    if (s * (ex * (y - a[1]) - ey * (x - a[0])) < 0) inside = false;
  }
  return inside ? -best : best;
}
// A pointed arch: base centre (ox, oy), axis (ux, uy) to the apex, height H, half-width wb at the base.
// Two arcs whose centres lie on the base line; returns the outline from the right base corner over the
// apex to the left one.
function gothArch(ox, oy, ux, uy, H, wb, m) {
  var vx = -uy, vy = ux, e = (H * H - wb * wb) / (2 * wb), R = wb + e, pts = [], i, s, hw;
  for (i = 0; i <= m; i++) { s = H * i / m; hw = Math.sqrt(Math.max(0, R * R - s * s)) - e; pts.push([ox + ux * s + vx * hw, oy + uy * s + vy * hw]); }
  for (i = m - 1; i >= 0; i--) { s = H * i / m; hw = Math.sqrt(Math.max(0, R * R - s * s)) - e; pts.push([ox + ux * s - vx * hw, oy + uy * s - vy * hw]); }
  return pts;
}
// A foil: n round lobes round a centre that just touch their neighbours, drawn as one outline.
function gothFoil(cx, cy, r, n, a0) {
  if (n < 3 || r < 5) { var o = []; for (var i = 0; i < 20; i++) o.push([cx + Math.cos(i / 20 * 6.2832) * r, cy + Math.sin(i / 20 * 6.2832) * r]); return o; }
  var sn = Math.sin(Math.PI / n), rl = r * sn / (1 + sn), dl = r - rl, pts = [], j, k, th, a, span = Math.PI + 2 * Math.PI / n;
  for (j = 0; j < n; j++) {
    th = a0 + j * 2 * Math.PI / n;
    var lx = cx + Math.cos(th) * dl, ly = cy + Math.sin(th) * dl;
    for (k = 0; k <= 9; k++) { a = th - span / 2 + span * k / 9; pts.push([lx + Math.cos(a) * rl, ly + Math.sin(a) * rl]); }
  }
  return pts;
}
function gothPath(g, pts) { g.beginPath(); for (var i = 0; i < pts.length; i++) { if (i) g.lineTo(pts[i][0], pts[i][1]); else g.moveTo(pts[i][0], pts[i][1]); } g.closePath(); }
function gothBox(pts) {
  var x0 = 1e9, y0 = 1e9, x1 = -1e9, y1 = -1e9;
  pts.forEach(function (p) { if (p[0] < x0) x0 = p[0]; if (p[0] > x1) x1 = p[0]; if (p[1] < y0) y0 = p[1]; if (p[1] > y1) y1 = p[1]; });
  return { x0: x0, y0: y0, x1: x1, y1: y1, w: x1 - x0, h: y1 - y0, cx: (x0 + x1) / 2, cy: (y0 + y1) / 2 };
}

// ---------- The plan of one face ----------
// Worked out once from the face's outline and the number's room, then drawn three times (colour, surface
// and height) so the three always agree about where the iron and the glass are.
// The number's extent in its own frame (turned by rot, as a d4's corner numbers are), grown by pad.
function gothPlate(strokes, cx, cy, rot, pad, tipT, tipB, extra) {
  var s = Math.sin(rot || 0), c = Math.cos(rot || 0), r0 = 1e9, r1 = -1e9, d0 = 1e9, d1 = -1e9;
  function see(x, y) { var dx = x - cx, dy = y - cy, r = dx * c + dy * s, d = -dx * s + dy * c; if (r < r0) r0 = r; if (r > r1) r1 = r; if (d < d0) d0 = d; if (d > d1) d1 = d; }
  strokes.forEach(function (pl) { pl.forEach(function (p) { see(p[0], p[1]); }); });
  (extra || []).forEach(function (p) { see(p[0], p[1]); });
  // Centred on the number's own centre, so the plate and the lights round it stay symmetrical.
  var rc = 0, dc = 0, hw = Math.max(Math.abs(r0), Math.abs(r1)) + pad, hh = Math.max(Math.abs(d0), Math.abs(d1)) + pad;
  // A 6 or 9 carries a dot below it; the plate then follows the whole mark instead of being centred.
  if (extra && extra.length) { dc = (d0 + d1) / 2; hh = (d1 - d0) / 2 + pad; }
  function map(r, d) { var rr = r + rc, dd = d + dc; return [cx + rr * c - dd * s, cy + rr * s + dd * c]; }
  // A pointed top when there is height for a true pointed arch (it must rise at least its half-width),
  // otherwise a cut corner.
  var ch = Math.min(hw, hh) * 0.24;
  var top = tipT >= hw * 1.05 ? gothArch(0, -hh, 0, -1, tipT, hw, 8) : [[hw, -hh + ch], [hw - ch, -hh], [-hw + ch, -hh], [-hw, -hh + ch]];
  var bot = tipB >= hw * 1.05 ? gothArch(0, hh, 0, 1, tipB, hw, 8) : [[-hw, hh - ch], [-hw + ch, hh], [hw - ch, hh], [hw, hh - ch]];
  return { poly: top.concat(bot).map(function (p) { return map(p[0], p[1]); }), hw: hw, hh: hh, cx: map(0, 0)[0], cy: map(0, 0)[1], rot: rot || 0 };
}
// Where a stroke ends in the open and not against another stroke: the number's wedge serifs go there.
function gothFlares(strokes, w) {
  var out = [];
  function nearOther(p, self, skipSeg) {
    for (var i = 0; i < strokes.length; i++) {
      var pl = strokes[i];
      for (var k = 1; k < pl.length; k++) {
        if (i === self && k === skipSeg) continue;
        var a = pl[k - 1], b = pl[k], ex = b[0] - a[0], ey = b[1] - a[1], l2 = ex * ex + ey * ey || 1e-9;
        var t = Math.max(0, Math.min(1, ((p[0] - a[0]) * ex + (p[1] - a[1]) * ey) / l2));
        if (Math.hypot(a[0] + ex * t - p[0], a[1] + ey * t - p[1]) < w * 0.75) return true;
      }
    }
    return false;
  }
  strokes.forEach(function (pl, i) {
    var n = pl.length; if (n < 2) return;
    if (Math.hypot(pl[0][0] - pl[n - 1][0], pl[0][1] - pl[n - 1][1]) < 1) return;
    if (!nearOther(pl[0], i, 1)) out.push({ p: pl[0], q: pl[1] });
    if (!nearOther(pl[n - 1], i, n - 1)) out.push({ p: pl[n - 1], q: pl[n - 2] });
  });
  return out;
}

function gothPlan(sd, lay, c, seed) {
  var cv = lay.cv, k = cv / 256, sides = sd.sides, tri = sides === 4 || sides === 8 || sides === 20, rnd = mulberry(seed * 7 + 3);
  var rib = cv * (tri ? 0.022 : 0.025), fw = cv * (tri ? 0.027 : 0.031), P = { variant: seed % 3, cv: cv, k: k, rib: rib, fw: fw, lead: cv * 0.0065, ring: cv * 0.0085, sides: sides, seed: seed, rnd: rnd };
  var Q = gothInset(lay.inner, fw), G = [lay.cx, lay.cy], w = strokeW(lay);
  P.Q = Q; P.G = G; P.w = w;
  // Plates: one under the number, or under each of a d4's three corner numbers. Each is a small window
  // of its own, pointed at the top.
  P.plates = [];
  var pad = w * 0.9 + lay.h * 0.04;
  if (lay.sub) {
    lay.sub.forEach(function (sl) {
      var p0 = gothPlate(sl.strokes, sl.cx, sl.cy, sl.rot, pad, 0, 0, null);
      P.plates.push(gothPlate(sl.strokes, sl.cx, sl.cy, sl.rot, pad, p0.hw * 1.25, 0, null));
    });
  } else {
    var extra = lay.label === '6' || lay.label === '9' ? [[lay.cx, lay.cy + lay.h * 0.68 + w * 0.62]] : null;
    P.plates.push(gothPlate(lay.strokes, lay.cx, lay.cy, 0, pad, 0, 0, extra));
    var pl = P.plates[0], tip = pl.hw * 1.25;
    // The point of the arch stops short of the frame.
    while (tip >= pl.hw * 1.05 && gothDist(Q, pl.cx, pl.cy - pl.hh - tip) > -rib * 1.5) tip -= 2;
    P.plates[0] = gothPlate(lay.strokes, lay.cx, lay.cy, 0, pad, tip, 0, extra);
  }
  P.flares = gothFlares(lay.strokes, w);
  P.lancets = []; P.foils = []; P.spokes = [];
  var sepP = rib * 1.1, sepL = rib * 1.25;
  function nearPlate(x, y) { var d = 1e9; P.plates.forEach(function (p) { d = Math.min(d, gothDist(p.poly, x, y)); }); return d; }
  function nearLights(x, y) { var d = 1e9; P.lancets.concat(P.foils).forEach(function (l) { d = Math.min(d, gothDist(l.poly, x, y)); }); return d; }
  function nearSpokes(x, y) {
    var d = 1e9;
    P.spokes.forEach(function (s) {
      var ex = s[1][0] - s[0][0], ey = s[1][1] - s[0][1], l2 = ex * ex + ey * ey || 1e-9, t = Math.max(0, Math.min(1, ((x - s[0][0]) * ex + (y - s[0][1]) * ey) / l2));
      d = Math.min(d, Math.hypot(s[0][0] + ex * t - x, s[0][1] + ey * t - y));
    });
    return d;
  }
  // Lancets: a pointed light reaching from the plate into each corner, as big as fits.
  function lancetFit(c0, asp, H) {
    var wb = H / asp, tb = c0.tb0, bx, by;
    // The base's corners must clear the plate too: slide it out until they do.
    for (;;) {
      bx = G[0] + c0.ux * tb; by = G[1] + c0.uy * tb;
      if (nearPlate(bx + c0.vx * wb, by + c0.vy * wb) >= sepP && nearPlate(bx - c0.vx * wb, by - c0.vy * wb) >= sepP) break;
      tb += 3; if (tb + H > c0.Dv) return null;
    }
    var poly = gothArch(bx, by, c0.ux, c0.uy, H, wb, 6);
    for (var i = 0; i < poly.length; i++) {
      if (gothDist(Q, poly[i][0], poly[i][1]) > -rib * 0.1) return null;
      if (P.lancets.length && nearLights(poly[i][0], poly[i][1]) < sepL) return null;
    }
    return { poly: gothArch(bx, by, c0.ux, c0.uy, H, wb, 12), H: H, wb: wb, tb: tb, ux: c0.ux, uy: c0.uy };
  }
  var cands = Q.map(function (v) {
    var dx = v[0] - G[0], dy = v[1] - G[1], Dv = Math.hypot(dx, dy), c0 = { v: v, Dv: Dv, ux: dx / Dv, uy: dy / Dv, tb0: 0, best: null };
    c0.vx = -c0.uy; c0.vy = c0.ux;
    // Where the middle of the lancet's base first clears the plate.
    while (c0.tb0 < Dv && nearPlate(G[0] + c0.ux * c0.tb0, G[1] + c0.uy * c0.tb0) < sepP) c0.tb0 += 2;
    if (lay.sub) return c0;
    [1.3, 1.8, 2.4].forEach(function (asp) {
      var lo = 8 * k, hi = c0.Dv - c0.tb0, found = null;
      for (var it = 0; it < 9; it++) { var mid = (lo + hi) / 2, f = lancetFit(c0, asp, mid); if (f) { found = f; lo = mid; } else hi = mid; }
      if (found && (!c0.best || found.H * found.wb > c0.best.H * c0.best.wb)) { c0.best = found; c0.asp = asp; }
    });
    return c0;
  });
  // One corner with far more room than the rest would make the face lopsided: cap each lancet at
  // half as big again as the middle one.
  var hs = cands.filter(function (c0) { return c0.best; }).map(function (c0) { return c0.best.H; }).sort(function (a, b) { return a - b; });
  var cap = hs.length ? hs[Math.floor(hs.length / 2)] * 1.5 : 0;
  cands.forEach(function (c0) {
    var ln = c0.best && c0.best.H > cap ? lancetFit(c0, c0.asp, cap) || c0.best : c0.best;
    if (ln && ln.H > 14 * k && ln.wb > 5 * k) {
      P.lancets.push(ln);
      P.spokes.push([G, [G[0] + ln.ux * ln.tb, G[1] + ln.uy * ln.tb]], [[G[0] + ln.ux * (ln.tb + ln.H), G[1] + ln.uy * (ln.tb + ln.H)], c0.v]);
    } else P.spokes.push([G, c0.v]);
  });
  // Round or foiled lights set in the glass between the ribs, largest first. The room at a point is
  // the nearest of the frame, plate, spokes and lights, each less the iron that borders it.
  var lobes = sides === 6 || sides === 10 || sides === 100 ? 4 : sides === 12 ? 5 : 3, rmin = cv * 0.03, rmax = cv * 0.15, a0 = rnd() * 6.28;
  var want = lay.sub ? 4 : Q.length, box = gothBox(Q);
  function room(x, y, floor) {
    var dq = -gothDist(Q, x, y) - rib * 0.55;
    if (dq <= floor) return 0;
    var r = Math.min(dq, rmax, nearPlate(x, y) - sepP * 1.15);
    if (r <= floor) return 0;
    r = Math.min(r, nearSpokes(x, y) - rib * 1.1);
    if (r <= floor) return 0;
    return Math.min(r, nearLights(x, y) - sepL * 0.95);
  }
  for (var n = 0; n < want; n++) {
    var bx = 0, by = 0, br = 0, st = Math.max(3, Math.round(cv / 64)), x, y, r;
    // A coarse scan centred on the number (so a symmetrical face gets symmetrical lights), then a
    // finer one round the best place.
    for (y = G[1] - Math.ceil((G[1] - box.y0) / st) * st; y < box.y1; y += st) for (x = G[0] - Math.ceil((G[0] - box.x0) / st) * st; x < box.x1; x += st) {
      r = room(x, y, br); if (r > br) { br = r; bx = x; by = y; }
    }
    var cx0 = bx, cy0 = by;
    for (y = cy0 - st; y <= cy0 + st; y += 1) for (x = cx0 - st; x <= cx0 + st; x += 1) { r = room(x, y, br); if (r > br) { br = r; bx = x; by = y; } }
    if (br < rmin) break;
    var ln = br < 14 * k ? 0 : lobes, la = Math.atan2(by - G[1], bx - G[0]);
    P.foils.push({ poly: gothFoil(bx, by, br, ln, la), cx: bx, cy: by, r: br, n: ln, a: la });
  }
  // Rivets along the frame: one at each corner.
  P.rivets = gothInset(lay.inner, fw * 0.5).map(function (v) { return [v[0], v[1], fw * 0.24]; });
  return P;
}

// ---------- Drawing ----------
// The iron: a dark metal with forging unevenness, fine grain, pits with a lit lower lip, and a few
// scratches. The unevenness is worked out once as a list of places (three variants per image size) and
// drawn into the colour, surface and height images alike, so a pit looks like a pit in all of them; the
// images themselves are made once per colour and shared by every face and die.
var gothIronCache = {}, gothTexCache = {};
function gothIronSpec(cv, variant) {
  var key = cv + '|' + variant;
  if (gothIronCache[key]) return gothIronCache[key];
  var rnd = mulberry(variant * 977 + cv), k = cv / 256, sp = { blobs: [], grain: [], pits: [], scratches: [] }, i;
  for (i = 0; i < 26; i++) sp.blobs.push({ x: rnd() * cv, y: rnd() * cv, r: (14 + rnd() * 46) * k, lit: rnd() > 0.5, a: 0.06 + rnd() * 0.1 });
  for (i = 0; i < 900 * k; i++) sp.grain.push({ x: rnd() * cv, y: rnd() * cv, s: (0.8 + rnd() * 1.8) * k, lit: rnd() > 0.5, a: rnd() });
  for (i = 0; i < 70; i++) sp.pits.push({ x: rnd() * cv, y: rnd() * cv, r: (0.7 + rnd() * 1.7) * k });
  for (i = 0; i < 10; i++) sp.scratches.push({ x: rnd() * cv, y: rnd() * cv, a: rnd() * 6.28, L: (10 + rnd() * 34) * k, al: rnd() });
  gothIronCache[key] = sp;
  return sp;
}
function gothIronTex(P, c) {
  var key = ['col', c.key, P.cv, P.variant].join('|');
  if (gothTexCache[key]) return gothTexCache[key];
  var cv = P.cv, k = cv / 256, sp = gothIronSpec(cv, P.variant), cn = canvas(cv), g = cn.getContext('2d');
  g.fillStyle = c.iron; g.fillRect(0, 0, cv, cv);
  sp.blobs.forEach(function (b) {
    var gr = g.createRadialGradient(b.x, b.y, 0, b.x, b.y, b.r);
    gr.addColorStop(0, hexA(b.lit ? c.wear : '#000000', b.a)); gr.addColorStop(1, 'rgba(0,0,0,0)');
    g.fillStyle = gr; g.fillRect(b.x - b.r, b.y - b.r, b.r * 2, b.r * 2);
  });
  sp.grain.forEach(function (d) { g.fillStyle = d.lit ? 'rgba(255,255,255,' + d.a * 0.07 + ')' : 'rgba(0,0,0,' + d.a * 0.12 + ')'; g.fillRect(d.x, d.y, d.s, d.s); });
  sp.pits.forEach(function (d) {
    g.fillStyle = 'rgba(255,255,255,.16)'; g.beginPath(); g.arc(d.x + d.r * 0.35, d.y + d.r * 0.4, d.r, 0, 7); g.fill();
    g.fillStyle = 'rgba(0,0,0,.55)'; g.beginPath(); g.arc(d.x, d.y, d.r, 0, 7); g.fill();
  });
  g.lineCap = 'round';
  sp.scratches.forEach(function (d) { g.strokeStyle = hexA(c.wear, 0.1 + d.al * 0.12); g.lineWidth = 0.7 * k; g.beginPath(); g.moveTo(d.x, d.y); g.lineTo(d.x + Math.cos(d.a) * d.L, d.y + Math.sin(d.a) * d.L); g.stroke(); });
  gothTexCache[key] = cn;
  return cn;
}
// One pane of glass: uneven colour, faint streaks, seeds, and a darkening where it meets the lead.
function gothGlass(g, P, c, pane, rnd) {
  var bb = gothBox(pane.poly), k = P.k, col = pane.col, ga = c.ga, i, span = Math.max(bb.w, bb.h);
  var gr = g.createRadialGradient(bb.cx - bb.w * 0.12, bb.cy - bb.h * 0.18, 0, bb.cx, bb.cy, span * 0.72);
  gr.addColorStop(0, hexA(mix(col, '#ffffff', 0.28), ga * 0.92)); gr.addColorStop(0.55, hexA(col, ga)); gr.addColorStop(1, hexA(mix(col, '#000000', 0.32), Math.min(0.95, ga + 0.12)));
  g.fillStyle = gr; g.fillRect(bb.x0 - 2, bb.y0 - 2, bb.w + 4, bb.h + 4);
  for (i = 0; i < 12; i++) {
    var x = bb.x0 + rnd() * bb.w, y = bb.y0 + rnd() * bb.h, r = (7 + rnd() * 20) * k, lit = rnd() > 0.45, bl = g.createRadialGradient(x, y, 0, x, y, r);
    bl.addColorStop(0, hexA(lit ? mix(col, '#ffffff', 0.45) : mix(col, '#000000', 0.5), lit ? 0.18 : 0.22)); bl.addColorStop(1, 'rgba(0,0,0,0)');
    g.fillStyle = bl; g.fillRect(x - r, y - r, r * 2, r * 2);
  }
  g.lineCap = 'round';
  for (i = 0; i < 7; i++) {
    var sx = bb.x0 + rnd() * bb.w, sy = bb.y0 + rnd() * bb.h, a = rnd() * 6.28, L = (16 + rnd() * 42) * k;
    g.strokeStyle = rnd() > 0.4 ? 'rgba(255,255,255,' + (0.07 + rnd() * 0.1) + ')' : hexA(mix(col, '#000000', 0.6), 0.12 + rnd() * 0.1);
    g.lineWidth = (0.8 + rnd() * 2.6) * k; g.beginPath(); g.moveTo(sx, sy);
    g.bezierCurveTo(sx + Math.cos(a) * L * 0.4 + (rnd() - 0.5) * 10 * k, sy + Math.sin(a) * L * 0.4 + (rnd() - 0.5) * 10 * k, sx + Math.cos(a) * L * 0.7, sy + Math.sin(a) * L * 0.7 + (rnd() - 0.5) * 8 * k, sx + Math.cos(a) * L, sy + Math.sin(a) * L); g.stroke();
  }
  for (i = 0; i < 8; i++) {
    var bx = bb.x0 + rnd() * bb.w, by = bb.y0 + rnd() * bb.h, br = (0.7 + rnd() * 1.7) * k;
    g.fillStyle = 'rgba(255,255,255,.14)'; g.beginPath(); g.arc(bx, by, br, 0, 7); g.fill();
    g.strokeStyle = 'rgba(255,255,255,.5)'; g.lineWidth = 0.7 * k; g.beginPath(); g.arc(bx, by, br, 3.6, 5.4); g.stroke();
  }
}
// The number: strokes with wedge serifs at their free ends. grow thickens everything for the outline.
function gothNumeral(g, lay, P, grow) {
  var w = P.w, sw = w + 2 * grow;
  g.lineJoin = 'miter'; g.miterLimit = 3; g.lineCap = 'butt'; g.lineWidth = sw;
  strokePaths(g, lay.strokes);
  P.flares.forEach(function (f) {
    var dx = f.p[0] - f.q[0], dy = f.p[1] - f.q[1], L = Math.hypot(dx, dy) || 1, ux = dx / L, uy = dy / L, vx = -uy, vy = ux;
    var h1 = w * 0.74 + grow, h0 = sw / 2, back = w * 0.95 + grow * 0.5, ex = f.p[0] + ux * grow * 0.5, ey = f.p[1] + uy * grow * 0.5;
    g.beginPath();
    g.moveTo(ex + vx * h1, ey + vy * h1); g.lineTo(ex - vx * h1, ey - vy * h1);
    g.lineTo(ex - ux * back - vx * h0, ey - uy * back - vy * h0); g.lineTo(ex - ux * back + vx * h0, ey - uy * back + vy * h0);
    g.closePath(); g.fill();
  });
  if (lay.label === '6' || lay.label === '9') { g.beginPath(); g.arc(lay.cx, lay.cy + lay.h * 0.68, w * 0.62 + grow, 0, 7); g.fill(); }
}

// Draws the whole face in one of three modes. look says how each part is filled in that mode.
function gothRender(g, lay, P, c, look) {
  var cv = P.cv, i, Q = P.Q, G = P.G, sec = [];
  g.fillStyle = look.iron; g.fillRect(0, 0, cv, cv);
  // The glass between the ribs: one pane per edge, then the lancets and the foils cut out of it.
  Q.forEach(function (v, n) { sec.push({ poly: [G, v, Q[(n + 1) % Q.length]], col: c.glass[2], kind: 'sector', i: n }); });
  var lights = [];
  P.lancets.forEach(function (l, n) { lights.push({ poly: l.poly, col: c.glass[(n + P.seed) % 2], kind: 'lancet', i: n }); });
  P.foils.forEach(function (f, n) { lights.push({ poly: f.poly, col: c.glass[(n + P.seed + 1) % 2], kind: 'foil', i: n }); });
  g.save(); gothPath(g, Q); g.clip();
  sec.forEach(function (pn) { g.save(); gothPath(g, pn.poly); g.clip(); look.glass(g, pn); g.restore(); });
  look.spokes(g, P.spokes, P.rib * 0.9);
  lights.forEach(function (pn) {
    // Cut the light out first, so the glass under it does not show through its own.
    g.save(); g.globalCompositeOperation = 'destination-out'; gothPath(g, pn.poly); g.fillStyle = '#000'; g.fill(); g.restore();
    g.save(); gothPath(g, pn.poly); g.clip(); look.glass(g, pn); g.restore();
  });
  if (look.shade) look.shade(g, lights);
  g.restore();
  g.lineJoin = 'round'; g.lineCap = 'round';
  lights.forEach(function (pn) { look.rib(g, pn, pn.kind === 'foil' ? P.rib * 0.8 : P.rib); });
  // A large foil is a small rose: a boss in the middle with a rib running out to each cusp.
  P.foils.forEach(function (f) { if (f.n >= 3 && f.r >= 22 * P.k) look.rose(g, f, P.rib * 0.5); });
  g.beginPath(); g.rect(-4, -4, cv + 8, cv + 8); Q.forEach(function (p, n) { if (n) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.closePath();
  look.frame(g);
  P.rivets.forEach(function (r) { look.rivet(g, r); });
  P.plates.forEach(function (pl) { g.save(); gothPath(g, Q); g.clip(); look.plate(g, pl); g.restore(); });
  look.numeral(g);
}

function gothColourLook(g, lay, P, c) {
  var rnd = P.rnd, cv = P.cv, k = P.k, pat = g.createPattern(gothIronTex(P, c), 'repeat'), lead = mix(c.iron, '#000000', 0.6);
  return {
    iron: pat,
    glass: function (gg, pn) { gothGlass(gg, P, c, pn, rnd); },
    spokes: function (gg, list, w) {
      gg.lineCap = 'butt'; gg.lineJoin = 'round';
      list.forEach(function (s) {
        gg.beginPath(); gg.moveTo(s[0][0], s[0][1]); gg.lineTo(s[1][0], s[1][1]);
        gg.strokeStyle = hexA(lead, 1); gg.lineWidth = w + P.lead * 2; gg.stroke();
        gg.strokeStyle = pat; gg.lineWidth = w; gg.stroke();
      });
    },
    shade: function (gg, lights) {
      // Glass pools darker against the lead: soft dark bands along the frame and round every light.
      var lines = [P.Q].concat(lights.map(function (l) { return l.poly; }));
      lines.forEach(function (pl) {
        for (var s = 0; s < 4; s++) { gg.strokeStyle = 'rgba(0,0,0,' + (0.06 + s * 0.03) + ')'; gg.lineWidth = (P.rib + (4 - s) * 3 * k); gothPath(gg, pl); gg.stroke(); }
      });
      gg.strokeStyle = hexA(lead, 1); gg.lineWidth = P.lead * 2; gothPath(gg, P.Q); gg.stroke();
      lights.forEach(function (l) { gg.lineWidth = P.rib + P.lead * 2; gothPath(gg, l.poly); gg.stroke(); });
    },
    rib: function (gg, pn, w) {
      gothPath(gg, pn.poly);
      gg.strokeStyle = pat; gg.lineWidth = w; gg.stroke();
      // Worn bright edges: broken, so it reads as metal rubbed through, not as a drawn line.
      gg.save(); gg.setLineDash([4 * k + rnd() * 18 * k, 7 * k + rnd() * 30 * k, 2 * k + rnd() * 9 * k, 9 * k + rnd() * 24 * k]);
      gg.strokeStyle = hexA(c.wear, 0.45); gg.lineWidth = w; gg.stroke(); gg.setLineDash([]);
      gg.strokeStyle = pat; gg.lineWidth = w - 2.4 * k; gg.stroke(); gg.restore();
    },
    rose: function (gg, f, w) {
      var sn = Math.sin(Math.PI / f.n), dl = f.r - f.r * sn / (1 + sn), cu = dl * Math.cos(Math.PI / f.n), j;
      gg.lineCap = 'butt';
      for (j = 0; j < f.n; j++) {
        var a = f.a + (j + 0.5) * 2 * Math.PI / f.n;
        gg.beginPath(); gg.moveTo(f.cx, f.cy); gg.lineTo(f.cx + Math.cos(a) * cu, f.cy + Math.sin(a) * cu);
        gg.strokeStyle = hexA(lead, 1); gg.lineWidth = w + P.lead * 1.6; gg.stroke(); gg.strokeStyle = pat; gg.lineWidth = w; gg.stroke();
      }
      gg.fillStyle = hexA(lead, 1); gg.beginPath(); gg.arc(f.cx, f.cy, f.r * 0.27 + P.lead, 0, 7); gg.fill();
      gg.fillStyle = pat; gg.beginPath(); gg.arc(f.cx, f.cy, f.r * 0.27, 0, 7); gg.fill();
      gg.strokeStyle = hexA(c.wear, 0.4); gg.lineWidth = 1.2 * k; gg.beginPath(); gg.arc(f.cx, f.cy, f.r * 0.24, 3.6, 5.3); gg.stroke();
    },
    frame: function (gg) {
      gg.fillStyle = pat; gg.fill('evenodd');
      // The frame's inner edge is bevelled: a bright worn line, broken, with a dark line at its foot.
      gg.save(); gg.clip('evenodd');
      gothPath(gg, P.Q);
      gg.strokeStyle = 'rgba(0,0,0,.6)'; gg.lineWidth = 3 * k; gg.stroke();
      gg.setLineDash([6 * k + rnd() * 20 * k, 9 * k + rnd() * 30 * k]); gg.strokeStyle = hexA(c.wear, 0.5); gg.lineWidth = 1.6 * k; gg.stroke(); gg.setLineDash([]);
      gg.restore();
    },
    rivet: function (gg, r) {
      gg.fillStyle = 'rgba(0,0,0,.5)'; gg.beginPath(); gg.arc(r[0] + r[2] * 0.2, r[1] + r[2] * 0.3, r[2] * 1.1, 0, 7); gg.fill();
      var rg = gg.createRadialGradient(r[0] - r[2] * 0.3, r[1] - r[2] * 0.35, 0, r[0], r[1], r[2]);
      rg.addColorStop(0, hexA(c.wear, 0.95)); rg.addColorStop(1, hexA(mix(c.iron, '#000000', 0.2), 1)); gg.fillStyle = rg; gg.beginPath(); gg.arc(r[0], r[1], r[2], 0, 7); gg.fill();
    },
    plate: function (gg, pl) {
      gothPath(gg, pl.poly); gg.fillStyle = c.plate; gg.fill();
      gg.globalAlpha = 0.28; gg.fillStyle = pat; gg.fill(); gg.globalAlpha = 1;
      gg.lineJoin = 'round';
      gg.strokeStyle = 'rgba(0,0,0,.7)'; gg.lineWidth = P.ring * 2.4; gg.stroke();
      gg.strokeStyle = c.trim; gg.lineWidth = P.ring; gg.stroke();
      gg.strokeStyle = hexA(mix(c.trim, '#ffffff', 0.5), 0.55); gg.lineWidth = P.ring * 0.35; gg.save(); gg.translate(-P.ring * 0.25, -P.ring * 0.3); gg.stroke(); gg.restore();
    },
    numeral: function (gg) {
      var gr = gg.createLinearGradient(0, lay.cy - lay.h * 0.6, 0, lay.cy + lay.h * 0.6);
      gr.addColorStop(0, mix(c.ink, '#ffffff', 0.3)); gr.addColorStop(0.5, c.ink); gr.addColorStop(1, mix(c.ink, '#000000', 0.22));
      var e = Math.max(2.4 * k, P.w * 0.3);
      gg.fillStyle = gg.strokeStyle = c.edge; gothNumeral(gg, lay, P, e);
      gg.fillStyle = gg.strokeStyle = gr; gothNumeral(gg, lay, P, 0);
    }
  };
}

// The surface map (roughness in green, metalness in blue) and the height map follow the same plan, so the
// glass is smooth and non-metal where the colour map has glass, and the iron, plate and number are metal.
// A repeating image of the iron's own unevenness: pits (rough, dull) and scratches (smooth, bright).
function gothMapTex(P, tag, base, pit, scratch, level) {
  var key = ['map', tag, P.cv, P.variant].join('|');
  if (gothTexCache[key]) return gothTexCache[key];
  var cv = P.cv, k = cv / 256, sp = gothIronSpec(cv, P.variant), cn = canvas(cv), g = cn.getContext('2d');
  g.fillStyle = base; g.fillRect(0, 0, cv, cv);
  sp.pits.forEach(function (d) { g.fillStyle = pit; g.beginPath(); g.arc(d.x, d.y, d.r, 0, 7); g.fill(); });
  g.lineCap = 'round'; g.strokeStyle = scratch; g.lineWidth = 0.7 * k;
  sp.scratches.forEach(function (d) { g.beginPath(); g.moveTo(d.x, d.y); g.lineTo(d.x + Math.cos(d.a) * d.L * 0.8, d.y + Math.sin(d.a) * d.L * 0.8); g.stroke(); });
  if (level) sp.grain.forEach(function (d, i) { if (i % 3) return; g.fillStyle = (d.lit ? 'rgba(255,255,255,' : 'rgba(0,0,0,') + d.a * 0.06 + ')'; g.fillRect(d.x, d.y, d.s, d.s); });
  gothTexCache[key] = cn;
  return cn;
}
function gothSurfaceLook(g, lay, P, c) {
  var rnd = P.rnd, k = P.k, ironS = 'rgb(0,135,150)', glassS = 'rgb(0,28,0)';
  var pat = g.createPattern(gothMapTex(P, 'srf', ironS, 'rgb(0,215,120)', 'rgb(0,95,255)', false), 'repeat');
  return {
    iron: pat,
    glass: function (gg, pn) { gg.fillStyle = glassS; gg.fillRect(0, 0, P.cv, P.cv); },
    spokes: function (gg, list, w) { gg.lineCap = 'butt'; gg.strokeStyle = pat; gg.lineWidth = w + P.lead * 2; list.forEach(function (s) { gg.beginPath(); gg.moveTo(s[0][0], s[0][1]); gg.lineTo(s[1][0], s[1][1]); gg.stroke(); }); },
    shade: null,
    rib: function (gg, pn, w) {
      gothPath(gg, pn.poly); gg.strokeStyle = pat; gg.lineWidth = w; gg.stroke();
      // Worn edges are smoother and brighter: rough 0.3, fully metal.
      gg.setLineDash([4 * k + rnd() * 18 * k, 7 * k + rnd() * 30 * k, 2 * k + rnd() * 9 * k, 9 * k + rnd() * 24 * k]);
      gg.strokeStyle = 'rgb(0,80,235)'; gg.lineWidth = w; gg.stroke(); gg.setLineDash([]);
      gg.strokeStyle = pat; gg.lineWidth = w - 2.4 * k; gg.stroke();
    },
    rose: function (gg, f, w) {
      var sn = Math.sin(Math.PI / f.n), dl = f.r - f.r * sn / (1 + sn), cu = dl * Math.cos(Math.PI / f.n), j;
      gg.lineCap = 'butt'; gg.strokeStyle = pat; gg.lineWidth = w + P.lead * 1.6;
      for (j = 0; j < f.n; j++) { var a = f.a + (j + 0.5) * 2 * Math.PI / f.n; gg.beginPath(); gg.moveTo(f.cx, f.cy); gg.lineTo(f.cx + Math.cos(a) * cu, f.cy + Math.sin(a) * cu); gg.stroke(); }
      gg.fillStyle = pat; gg.beginPath(); gg.arc(f.cx, f.cy, f.r * 0.27 + P.lead, 0, 7); gg.fill();
    },
    frame: function (gg) { gg.fillStyle = pat; gg.fill('evenodd'); },
    rivet: function (gg, r) { gg.fillStyle = 'rgb(0,70,230)'; gg.beginPath(); gg.arc(r[0], r[1], r[2], 0, 7); gg.fill(); },
    plate: function (gg, pl) {
      gothPath(gg, pl.poly); gg.fillStyle = 'rgb(0,125,160)'; gg.fill();
      gg.lineJoin = 'round'; gg.strokeStyle = 'rgb(0,60,240)'; gg.lineWidth = P.ring; gg.stroke();
    },
    numeral: function (gg) {
      var e = Math.max(2.4 * k, P.w * 0.3);
      gg.fillStyle = gg.strokeStyle = 'rgb(0,150,130)'; gothNumeral(gg, lay, P, e);
      gg.fillStyle = gg.strokeStyle = 'rgb(0,100,120)'; gothNumeral(gg, lay, P, 0);
    }
  };
}
function gothHeightLook(g, lay, P, c) {
  var rnd = P.rnd, k = P.k, sl = P.cv * 0.0045, web = 0.7, rib = 0.78, frame = 0.84, glassH = 0.26;
  function gray(v) { var n = Math.round(Math.max(0, Math.min(1, v)) * 255); return 'rgb(' + n + ',' + n + ',' + n + ')'; }
  var pat = g.createPattern(gothMapTex(P, 'web', gray(web), gray(web - 0.2), gray(web + 0.1), true), 'repeat');
  var patF = g.createPattern(gothMapTex(P, 'frame', gray(frame), gray(frame - 0.22), gray(frame + 0.08), true), 'repeat');
  // A rounded edge: bands of rising height drawn wide to narrow, so the slope is smooth in the normal map.
  function bevel(gg, draw, w, lo, hi) {
    for (var i = 0; i < 4; i++) { gg.lineWidth = w + (3 - i) * 2 * sl; gg.strokeStyle = gray(lo + (hi - lo) * (i + 1) / 4); draw(gg); gg.stroke(); }
  }
  return {
    iron: pat,
    glass: function (gg, pn) {
      gg.fillStyle = gray(glassH); gg.fillRect(0, 0, P.cv, P.cv);
      // Hand-made glass is never quite flat: slow swells that make the reflections wander.
      var bb = gothBox(pn.poly);
      for (var i = 0; i < 6; i++) {
        var x = bb.x0 + rnd() * bb.w, y = bb.y0 + rnd() * bb.h, r = (10 + rnd() * 26) * k, bl = gg.createRadialGradient(x, y, 0, x, y, r), up = rnd() > 0.5;
        bl.addColorStop(0, up ? 'rgba(255,255,255,.07)' : 'rgba(0,0,0,.07)'); bl.addColorStop(1, 'rgba(0,0,0,0)'); gg.fillStyle = bl; gg.fillRect(x - r, y - r, r * 2, r * 2);
      }
    },
    spokes: function (gg, list, w) { gg.lineCap = 'butt'; list.forEach(function (s) { bevel(gg, function (g2) { g2.beginPath(); g2.moveTo(s[0][0], s[0][1]); g2.lineTo(s[1][0], s[1][1]); }, w, glassH, rib); }); },
    shade: function (gg) { gg.save(); gothPath(gg, P.Q); gg.clip(); bevel(gg, function (g2) { gothPath(g2, P.Q); }, 0, glassH, frame - 0.02); gg.restore(); },
    rib: function (gg, pn, w) { gg.lineJoin = 'round'; bevel(gg, function (g2) { gothPath(g2, pn.poly); }, w, glassH, rib); },
    rose: function (gg, f, w) {
      var sn = Math.sin(Math.PI / f.n), dl = f.r - f.r * sn / (1 + sn), cu = dl * Math.cos(Math.PI / f.n), j;
      gg.lineCap = 'butt';
      for (j = 0; j < f.n; j++) {
        var a = f.a + (j + 0.5) * 2 * Math.PI / f.n;
        bevel(gg, function (g2) { g2.beginPath(); g2.moveTo(f.cx, f.cy); g2.lineTo(f.cx + Math.cos(a) * cu, f.cy + Math.sin(a) * cu); }, w, glassH, rib);
      }
      var rg = gg.createRadialGradient(f.cx, f.cy, 0, f.cx, f.cy, f.r * 0.3); rg.addColorStop(0, gray(0.92)); rg.addColorStop(0.8, gray(rib)); rg.addColorStop(1, gray(0.5));
      gg.fillStyle = rg; gg.beginPath(); gg.arc(f.cx, f.cy, f.r * 0.3, 0, 7); gg.fill();
    },
    frame: function (gg) { gg.fillStyle = patF; gg.fill('evenodd'); },
    rivet: function (gg, r) {
      var rg = gg.createRadialGradient(r[0], r[1], 0, r[0], r[1], r[2]); rg.addColorStop(0, gray(1)); rg.addColorStop(1, gray(frame + 0.02)); gg.fillStyle = rg; gg.beginPath(); gg.arc(r[0], r[1], r[2], 0, 7); gg.fill();
    },
    plate: function (gg, pl) {
      gothPath(gg, pl.poly); gg.fillStyle = gray(0.8); gg.fill();
      gg.lineJoin = 'round'; bevel(gg, function (g2) { gothPath(g2, pl.poly); }, P.ring, 0.74, 0.93);
    },
    numeral: function (gg) {
      var e = Math.max(2.4 * k, P.w * 0.3);
      gg.fillStyle = gg.strokeStyle = gray(0.9); gothNumeral(gg, lay, P, e);
      gg.fillStyle = gg.strokeStyle = gray(1); gothNumeral(gg, lay, P, 0);
    }
  };
}

registerSet({
  key: 'gothic', name: 'Gothic', note: 'Blackened iron tracery with stained glass between the ribs', show: 'kindle', bev: 0.065, see: 'glass', relief: 0.8,
  colours: gothColours,
  paint: function (g, lay, c, seed, sd) {
    var P = gothPlan(sd, lay, c, seed); lay._goth = P;
    gothRender(g, lay, P, c, gothColourLook(g, lay, P, c));
  },
  surface: function (g, lay, c) { var P = lay._goth; gothRender(g, lay, P, c, gothSurfaceLook(g, lay, P, c)); },
  height: function (g, lay, c) { var P = lay._goth; gothRender(g, lay, P, c, gothHeightLook(g, lay, P, c)); },
  rim: function (c) { return { col: mix(c.iron, '#ffffff', 0.04), rough: 0.44, metal: 1 }; },
  // The glass carries a little light of its own, as a window does in daylight: added only where the
  // surface is not metal, so the iron stays dark.
  shader: function (s, c) {
    s.uniforms.uGothGlow = { value: c.glow };
    s.fragmentShader = 'uniform float uGothGlow;\n' + s.fragmentShader.replace('vEmissiveMapUv ).r;\n#endif',
      'vEmissiveMapUv ).r;\n#endif\n\ttotalEmissiveRadiance += diffuseColor.rgb * ( 1.0 - metalnessFactor ) * uGothGlow;');
  },
  cover: function (c) { return isLight(c) ? 0.55 : 0; }
});
