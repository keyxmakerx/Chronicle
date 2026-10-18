// ==================== Verdant cage ====================
// An open metal frame in the die's shape, vines inlaid in its grooves, round an orb that floats and
// turns inside. Each face is an opaque metal plate with windows cut clean through (see: 'cutout') so the
// orb shows; the number sits on a solid, darker band that runs across the plate to the frame.
// The plate's look is drawn into the shared face atlas; the orb is the set's own part (build).
import {
  canvas, dot, hexA, mix, mulberry, paintNumeral, polyPath, registerSet, strokeW, share
} from '../engine.js';

// vine: the inlaid plant. stem is the woody stem, leaf runs dark (at the stalk) to light (where the light
// catches it), style is the leaf's shape ('ivy' three-lobed, 'oval' pointed, 'thorn' narrow, with thorns on
// the stem), berry (optional) hangs small clusters at some leaf joints.
// fey: the colour of the plant the Overgrow show grows; it glows in burn.
var CAGE_COLOURS = [
  { key: 'bronze', name: 'Bronze and jade', note: 'Dark antique bronze, jade ivy, a moonstone orb',
    burn: '#8dffc0', ink: '#f6e6c0', grow: '#3f9a66', band: ['#24170c', '#0f0904'], liner: '#6e5232', tex: 'hammer',
    base: { grad: ['#7f5c36', '#382414'], rough: 0.4 },
    vine: { stem: '#4f4a2c', leaf: ['#173d2a', '#3a8060', '#8cc9a5'], style: 'ivy' },
    fey: '#2e8a5c',
    orb: { kind: 'moon', col: '#8e9ec4', glow: '#8fb8ff', k: 0.3, rough: 0.2, iri: 0.5, sheen: '#7fa8ff', ior: 1.55, env: 1.1 } },
  { key: 'iron', name: 'Black iron and thorn', note: 'Blackened iron, blood-red thorn vines, a ruby orb',
    burn: '#ff5a4a', ink: '#f8eadd', grow: '#a3202b', band: ['#141317', '#060607'], liner: '#4c4e57', tex: 'hammer',
    base: { grad: ['#46474f', '#1a1b20'], rough: 0.44 },
    vine: { stem: '#3e1d16', leaf: ['#3a0910', '#821a22', '#cc5054'], style: 'thorn' },
    fey: '#8f1c27',
    orb: { kind: 'ruby', col: '#d8606c', glow: '#ff2a3c', k: 0.4, rough: 0.07, iri: 0, sheen: '#ff6a7a', ior: 1.77, env: 1.2 } },
  { key: 'silver', name: 'Silver and moss', note: 'Old, darkened silver, moss-green ivy, a pearl orb',
    burn: '#d8ff9a', ink: '#f6f8f0', grow: '#5f8a2e', band: ['#1e2823', '#0c120e'], liner: '#7d838f', tex: 'brush',
    base: { grad: ['#b3b8c1', '#5a5f6a'], rough: 0.36 },
    vine: { stem: '#4a5030', leaf: ['#2c3f17', '#5f822e', '#adc672'], style: 'ivy' },
    fey: '#5f8a2e',
    orb: { kind: 'pearl', col: '#cfc6c0', glow: '#ffe6f2', k: 0.14, rough: 0.22, iri: 1, sheen: '#ffc8e4', ior: 1.5, env: 1.0 } },
  { key: 'gold', name: 'Gold and emerald', note: 'Old, dark gold, emerald leaves, an amber orb',
    burn: '#ffd76a', ink: '#fff0c0', grow: '#1a7a45', band: ['#0e1f16', '#050d08'], liner: '#8a6c34', tex: 'brush',
    base: { grad: ['#ad873a', '#553810'], rough: 0.34 },
    vine: { stem: '#38452a', leaf: ['#0a3a20', '#1c7444', '#63b986'], style: 'oval' },
    fey: '#1a7a45',
    orb: { kind: 'amber', col: '#e09a3c', glow: '#ff9a1a', k: 0.4, rough: 0.1, iri: 0, sheen: '#ffd27a', ior: 1.55, env: 1.1 } },
  { key: 'pewter', name: 'Pewter and nightshade', note: 'Dark pewter, violet nightshade with black berries, an amethyst orb',
    burn: '#caa2ff', ink: '#f4eeff', grow: '#5a2f8a', band: ['#16142a', '#080712'], liner: '#5e6578', tex: 'hammer',
    base: { grad: ['#737a8d', '#2f3443'], rough: 0.38 },
    vine: { stem: '#3d2f47', leaf: ['#2a1645', '#5b3488', '#a07ccf'], style: 'oval', berry: '#1c1029' },
    fey: '#5a2f8a',
    orb: { kind: 'amethyst', col: '#a67fe0', glow: '#b57bff', k: 0.4, rough: 0.08, iri: 0, sheen: '#d2b0ff', ior: 1.55, env: 1.2 } }
];

// ---------- Plate geometry, shared by the colour, roughness and height passes ----------
function cageHash(s) { var h = 2166136261; for (var i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = Math.imul(h, 16777619); } return h >>> 0; }
// A convex polygon pulled in by d (canvas px) along its corner bisectors.
function cageInset(poly, d) {
  var n = poly.length, out = [];
  for (var k = 0; k < n; k++) {
    var p = poly[k], a = poly[(k + n - 1) % n], b = poly[(k + 1) % n];
    var e1x = a[0] - p[0], e1y = a[1] - p[1], e2x = b[0] - p[0], e2y = b[1] - p[1], l1 = Math.hypot(e1x, e1y) || 1, l2 = Math.hypot(e2x, e2y) || 1;
    e1x /= l1; e1y /= l1; e2x /= l2; e2y /= l2;
    var bx = e1x + e2x, by = e1y + e2y, bl = Math.hypot(bx, by) || 1; bx /= bl; by /= bl;
    var s = d / Math.max(0.25, Math.abs(e1x * by - e1y * bx));
    out.push([p[0] + bx * s, p[1] + by * s]);
  }
  return out;
}
// Equal steps along a polyline (closed or open): { pts: [{ x, y, s }], total }.
function cageResample(pts, step, closed) {
  var P = closed ? pts.concat([pts[0]]) : pts, cum = [0], i;
  for (i = 1; i < P.length; i++) cum.push(cum[i - 1] + Math.hypot(P[i][0] - P[i - 1][0], P[i][1] - P[i - 1][1]));
  var total = cum[cum.length - 1], n = Math.max(2, Math.round(total / step)), out = [], j = 1;
  for (i = 0; i <= n; i++) {
    var s = total * i / n;
    while (j < cum.length - 1 && cum[j] < s) j++;
    var f = (s - cum[j - 1]) / ((cum[j] - cum[j - 1]) || 1);
    out.push({ x: P[j - 1][0] + (P[j][0] - P[j - 1][0]) * f, y: P[j - 1][1] + (P[j][1] - P[j - 1][1]) * f, s: s });
  }
  if (closed) out.pop();
  return { pts: out, total: total };
}
// A stem wandering either side of its anchor line, held on the line where it starts. Two uneven waves
// make the sway, so it never repeats like a drawn zigzag. Each point carries its tangent and how far
// along the stem it is (f, 0 at the root), which sets the stem's taper.
function cageWobble(rs, amp, lam, ph, ph2, ends) {
  var pts = rs.pts, n = pts.length, out = [];
  for (var i = 0; i < n; i++) {
    var a = pts[Math.max(0, i - 1)], b = pts[Math.min(n - 1, i + 1)];
    var tx = b.x - a.x, ty = b.y - a.y, tl = Math.hypot(tx, ty) || 1; tx /= tl; ty /= tl;
    var env = Math.min(1, pts[i].s / ends); env = env * env * (3 - 2 * env);
    var s = pts[i].s, d = amp * env * (0.72 * Math.sin(6.2832 * s / lam + ph) + 0.38 * Math.sin(6.2832 * s / (lam * 0.43) + ph2));
    out.push({ x: pts[i].x - ty * d, y: pts[i].y + tx * d, tx: tx, ty: ty, s: s, f: s / (rs.total || 1) });
  }
  // Tangents of the swaying line itself, so leaves leave the stem along its real direction.
  for (i = 0; i < n; i++) {
    var p = out[Math.max(0, i - 1)], q = out[Math.min(n - 1, i + 1)], dx = q.x - p.x, dy = q.y - p.y, dl = Math.hypot(dx, dy) || 1;
    out[i].tx = dx / dl; out[i].ty = dy / dl;
  }
  return out;
}
// Leaves at uneven steps along a stem, mostly alternating sides, leaning forward; smaller towards the
// growing tip. Each leaf sits on a short stalk. Thorny stems also get hooked thorns between the leaves.
function cageSprigs(geo, st, lam, fw, rnd, style, berry, cx, cy) {
  var n = st.length, total = st[n - 1].s, s = lam * (0.3 + rnd() * 0.3), side = rnd() < 0.5 ? 1 : -1, i = 0, count = 0;
  var thin = style === 'thorn', wide = style === 'ivy';
  while (s < total - fw * 0.6) {
    while (i < n - 1 && st[i].s < s) i++;
    // Leaves on the window side are full size and reach out over the opening; those on the face's edge
    // side stay small and lie along the stem, so the face's edge never cuts one off.
    var q = st[i], nx = -q.ty, ny = q.tx, inward = ((cx - q.x) * nx + (cy - q.y) * ny) * side > 0;
    var size = fw * (wide ? 1.7 : thin ? 1.75 : 1.95) * (inward ? 1 : 0.62) * (1 - 0.38 * q.f) * (0.8 + rnd() * 0.4);
    var ang = (inward ? (wide ? 1.0 : 0.8) : 0.42) + rnd() * 0.4, c = Math.cos(ang), sn = Math.sin(ang) * side, dx = q.tx * c + nx * sn, dy = q.ty * c + ny * sn, stalk = size * (wide ? 0.38 : 0.22);
    geo.leaves.push({ sx: q.x, sy: q.y, x: q.x + dx * stalk, y: q.y + dy * stalk, dx: dx, dy: dy, L: size, W: size * (wide ? 0.95 : thin ? 0.36 : 0.5),
      bend: (rnd() - 0.5) * 0.5, tone: rnd(), shape: style, side: side });
    if (berry && count % 3 === 1) {
      // Two or three berries hang from the leaf joint on the other side.
      var bx = q.x - dx * fw * 0.1 - nx * side * fw * 0.32, by = q.y - dy * fw * 0.1 - ny * side * fw * 0.32, br = fw * 0.13;
      for (var b = 0; b < 3; b++) geo.berries.push({ x: bx + Math.cos(b * 2.1 + rnd()) * br * 1.3, y: by + Math.sin(b * 2.1 + rnd()) * br * 1.3, r: br * (0.8 + rnd() * 0.35) });
    }
    if (thin) {
      // Thorns between this leaf and the next, hooked back towards the root.
      for (var k = 1; k <= 2; k++) {
        var j = Math.min(n - 1, i + Math.round(lam * 0.17 * k / ((total / n) || 1))), p = st[j], ts = (k % 2 ? -side : side), px = -p.ty * ts, py = p.tx * ts, tl = fw * (0.34 + rnd() * 0.12) * (1 - 0.4 * p.f);
        geo.thorns.push({ x: p.x, y: p.y, tx: p.tx, ty: p.ty, px: px, py: py, L: tl });
      }
    }
    s += lam * (0.36 + rnd() * 0.3) * (thin ? 1.3 : 1); side = rnd() < 0.82 ? -side : side; count++;
  }
}
// A tendril: leaves the stem heading (ang), coils tighter as it goes.
function cageCurl(x, y, ang, len, dir) {
  var steps = 26, ds = len / steps, pts = [], th = ang;
  for (var i = 0; i <= steps; i++) {
    var t = i / steps; pts.push({ x: x, y: y, t: t });
    x += Math.cos(th) * ds; y += Math.sin(th) * ds;
    th += dir * (0.8 + 14 * t * t) / len * ds;
  }
  return pts;
}

// A convex polygon with its corners rounded by quadratic curves of reach rr (canvas px).
function cageRound(poly, rr) {
  var n = poly.length, out = [];
  for (var k = 0; k < n; k++) {
    var p = poly[k], a = poly[(k + n - 1) % n], b = poly[(k + 1) % n];
    var d1 = Math.hypot(a[0] - p[0], a[1] - p[1]) || 1, d2 = Math.hypot(b[0] - p[0], b[1] - p[1]) || 1, r1 = Math.min(rr, d1 * 0.4), r2 = Math.min(rr, d2 * 0.4);
    var s = [p[0] + (a[0] - p[0]) / d1 * r1, p[1] + (a[1] - p[1]) / d1 * r1], e = [p[0] + (b[0] - p[0]) / d2 * r2, p[1] + (b[1] - p[1]) / d2 * r2];
    for (var i = 0; i <= 6; i++) {
      var t = i / 6, u = 1 - t;
      out.push([u * u * s[0] + 2 * u * t * p[0] + t * t * e[0], u * u * s[1] + 2 * u * t * p[1] + t * t * e[1]]);
    }
  }
  return out;
}
// Everything about a face's plate that more than one pass needs, made once per face.
//   core    the window's outline (the face pulled in by the frame's width); clip is the frame vine's line
//   bands   the solid bands that carry the numbers, running to the frame: { x, y, rot, u0, u1, v0, v1, r }
//           in a frame turned by rot about (x, y); a d4's three run out to its corners
//   win     the opening, as a canvas whose alpha is the hole
//   stems, leaves, curls, thorns, berries   the vines (their shapes depend on the colour's vine)
function cageGeom(lay, c) {
  if (lay._cage) return lay._cage;
  var cv = lay.cv, k = cv / 256, inner = lay.inner, n = inner.length, BIG = cv * 0.9, V = c.vine;
  var rnd = mulberry(cageHash(lay.label + '|' + n + '|' + cv + '|' + Math.round(lay.poly[0][0] * 7) + '|' + Math.round(lay.poly[0][1] * 7)));
  var fw = Math.max(0.04 * cv, Math.min(0.075 * cv, lay.r * 0.24)), gw = fw * 0.4;
  var geo = { fw: fw, gw: gw, k: k, rnd: rnd, stems: [], leaves: [], curls: [], thorns: [], berries: [], knots: [], bands: [], dimples: [] };
  geo.core = cageInset(inner, fw); geo.clip = cageInset(inner, fw * 0.5);
  // A band is the number's own extent plus a margin, turned with the number, then run out to the frame
  // along its long side (a d4's corner numbers run out to their corner).
  var nums = lay.sub || [lay];
  nums.forEach(function (L) {
    var rot = L.rot || 0, cs = Math.cos(rot), sn = Math.sin(rot), u0 = 1e9, u1 = -1e9, v0 = 1e9, v1 = -1e9, w = strokeW(L), m = Math.max(3.5 * k, 0.08 * L.h);
    L.strokes.forEach(function (pl) {
      pl.forEach(function (p) {
        var dx = p[0] - L.cx, dy = p[1] - L.cy, u = dx * cs + dy * sn, v = -dx * sn + dy * cs;
        if (u < u0) u0 = u; if (u > u1) u1 = u; if (v < v0) v0 = v; if (v > v1) v1 = v;
      });
    });
    // A 6 or 9 carries a mark under it.
    if (!lay.sub && (L.label === '6' || L.label === '9')) v1 = Math.max(v1, L.h * 0.68 + w * 0.62);
    var hw = (u1 - u0) / 2 + w * 0.6 + m, hh = (v1 - v0) / 2 + w * 0.6 + m, uc = (u0 + u1) / 2, vc = (v0 + v1) / 2;
    var b = { x: L.cx + uc * cs - vc * sn, y: L.cy + uc * sn + vc * cs, rot: rot, u0: -hw, u1: hw, v0: -hh, v1: hh, r: Math.min(hw, hh) * 0.45 };
    if (lay.sub) b.v0 = -BIG; else if (hh > hw * 0.95) { b.v0 = -BIG; b.v1 = BIG; } else { b.u0 = -BIG; b.u1 = BIG; }
    geo.bands.push(b);
  });
  geo.win = cageWindow(lay, geo);
  var wd = geo.win.getContext('2d').getImageData(0, 0, cv, cv).data;
  function open(x, y) { x = Math.round(x); y = Math.round(y); return x >= 0 && y >= 0 && x < cv && y < cv && wd[(y * cv + x) * 4 + 3] > 140; }
  // Hammered dimples, shared by the colour and height passes.
  for (var d = 0; d < 40; d++) geo.dimples.push({ x: rnd() * cv, y: rnd() * cv, r: (7 + rnd() * 12) * k * 1.5 });
  // The vine roots at one point on the frame and grows both ways from it, part of the way round, never
  // the whole loop: a plant laid along the metal, not an outline traced round each window.
  var loop = cageResample(cageRound(geo.clip, fw * 2.4), 2 * k, true), N = loop.pts.length, root = Math.floor(rnd() * N);
  var cx = 0, cy = 0; inner.forEach(function (p) { cx += p[0] / n; cy += p[1] / n; });
  [1, -1].forEach(function (dir, r) {
    var frac = r ? 0.2 + rnd() * 0.16 : 0.42 + rnd() * 0.16, m = Math.round(frac * N), pts = [];
    for (var j = 0; j <= m; j++) { var lp = loop.pts[((root + dir * j) % N + N) % N]; pts.push([lp.x, lp.y]); }
    var rs = cageResample(pts, 1.6 * k, false), lam = fw * (3 + rnd() * 0.9);
    var st = cageWobble(rs, fw * 0.17, lam, rnd() * 6.28, rnd() * 6.28, fw * 0.7);
    geo.stems.push({ pts: st, w: gw * (r ? 1.15 : 1.4) });
    cageSprigs(geo, st, lam, fw, rnd, V.style, V.berry, cx, cy);
    // The growing tip coils into a tendril, out over a window if there is room.
    var e = st[st.length - 1], nx = -e.ty, ny = e.tx;
    if ((cx - e.x) * nx + (cy - e.y) * ny < 0) { nx = -nx; ny = -ny; }
    var lean = open(e.x + nx * fw * 1.6, e.y + ny * fw * 1.6) ? 0.7 : 0.15, h = Math.atan2(e.ty + ny * lean, e.tx + nx * lean), cd = rnd() < 0.5 ? 1 : -1;
    geo.curls.push({ pts: cageCurl(e.x, e.y, h, fw * 2.3, cd), w: gw * (r ? 0.7 : 0.8) });
  });
  geo.knots.push({ x: loop.pts[root].x, y: loop.pts[root].y, r: gw * 0.95 });
  // A couple of fine tendrils leave the older stem for windows that have the room.
  var main = geo.stems[0].pts, cnt = 0, dir = 1;
  for (var i = Math.round(main.length * 0.2); i < main.length * 0.8 && cnt < 2; i += Math.max(1, Math.round(fw * 2.6 / (1.6 * k)))) {
    var q = main[i], qx = -q.ty, qy = q.tx;
    if ((cx - q.x) * qx + (cy - q.y) * qy < 0) { qx = -qx; qy = -qy; }
    if (open(q.x + qx * fw * 1.4, q.y + qy * fw * 1.4) && open(q.x + qx * fw * 3, q.y + qy * fw * 3) && open(q.x + qx * fw * 2 + q.tx * fw, q.y + qy * fw * 2 + q.ty * fw)) {
      geo.curls.push({ pts: cageCurl(q.x + qx * fw * 0.2, q.y + qy * fw * 0.2, Math.atan2(qy, qx) + dir * 0.5, fw * 2.4, dir), w: gw * 0.5 }); cnt++; dir = -dir;
    }
  }
  lay._cage = geo;
  return geo;
}

// The window: where the plate is cut clean through. The face pulled in by the frame, less the bands,
// with its corners rounded off (blur, then threshold). Alpha is the opening.
function cageWindow(lay, geo) {
  var cv = lay.cv, m = canvas(cv), g = m.getContext('2d');
  g.fillStyle = '#fff'; polyPath(g, geo.core); g.fill();
  g.globalCompositeOperation = 'destination-out'; g.fillStyle = '#000';
  geo.bands.forEach(function (p) { cageRR(g, p, 0); g.fill(); });
  var b = canvas(cv), bg = b.getContext('2d');
  try { bg.filter = 'blur(' + (cv * 0.019) + 'px)'; } catch (e) {}
  bg.drawImage(m, 0, 0); bg.filter = 'none';
  var id = bg.getImageData(0, 0, cv, cv), d = id.data;
  for (var i = 0; i < d.length; i += 4) {
    var t = Math.max(0, Math.min(1, (d[i + 3] / 255 - 0.42) / 0.16));
    d[i] = d[i + 1] = d[i + 2] = 255; d[i + 3] = Math.round(t * t * (3 - 2 * t) * 255);
  }
  bg.putImageData(id, 0, 0);
  return b;
}
// A band's rounded rectangle, grown by `grow`.
function cageRR(g, p, grow) {
  var u0 = p.u0 - grow, u1 = p.u1 + grow, v0 = p.v0 - grow, v1 = p.v1 + grow, c = Math.cos(p.rot), s = Math.sin(p.rot);
  var r = Math.max(1, Math.min(p.r + grow, (u1 - u0) / 2, (v1 - v0) / 2));
  function T(u, v) { return [p.x + u * c - v * s, p.y + u * s + v * c]; }
  var pts = [[u0, v0], [u1, v0], [u1, v1], [u0, v1]];
  g.beginPath();
  for (var i = 0; i < 4; i++) {
    var a = pts[i], b = pts[(i + 1) % 4], pr = pts[(i + 3) % 4], dx = b[0] - a[0], dy = b[1] - a[1], dl = Math.hypot(dx, dy), px = a[0] - pr[0], py = a[1] - pr[1], pl = Math.hypot(px, py);
    var s0 = T(a[0] - px / pl * r, a[1] - py / pl * r), c0 = T(a[0], a[1]), s1 = T(a[0] + dx / dl * r, a[1] + dy / dl * r);
    if (i) g.lineTo(s0[0], s0[1]); else g.moveTo(s0[0], s0[1]);
    g.quadraticCurveTo(c0[0], c0[1], s1[0], s1[1]);
  }
  g.closePath();
}

// ---------- Drawing ----------
// Colours derived from a colour's metal and vine, made once.
function cageTones(c) {
  if (c._cg) return c._cg;
  var V = c.vine;
  c._cg = { polish: mix(c.base.grad[0], '#ffffff', 0.22), groove: mix(c.base.grad[1], '#000000', 0.62),
    bark: mix(V.stem, '#000000', 0.25), barkLit: mix(V.stem, '#ffffff', 0.2), edge: mix(V.leaf[0], '#000000', 0.45) };
  return c._cg;
}
// A window's shape as a flat colour, for shadows and for the height map's openings.
function cageTint(mask, colour) {
  var t = canvas(mask.width, mask.height), g = t.getContext('2d');
  g.drawImage(mask, 0, 0); g.globalCompositeOperation = 'source-in'; g.fillStyle = colour; g.fillRect(0, 0, t.width, t.height);
  return t;
}
// A tapering stroke as one filled outline, so a see-through colour never doubles up where pieces overlap.
// pts are { x, y } and hw(i) the half-width at each; off shifts the whole ribbon sideways (a highlight).
function cageRibbon(g, pts, hw, off) {
  var n = pts.length, L = [], R = [], i;
  if (n < 2) return;
  for (i = 0; i < n; i++) {
    var a = pts[Math.max(0, i - 1)], b = pts[Math.min(n - 1, i + 1)], tx = b.x - a.x, ty = b.y - a.y, tl = Math.hypot(tx, ty) || 1, nx = -ty / tl, ny = tx / tl, h = hw(i), o = (off || 0) * h;
    L.push([pts[i].x + nx * (h + o), pts[i].y + ny * (h + o)]); R.push([pts[i].x - nx * (h - o), pts[i].y - ny * (h - o)]);
  }
  g.beginPath(); g.moveTo(L[0][0], L[0][1]);
  for (i = 1; i < n; i++) g.lineTo(L[i][0], L[i][1]);
  for (i = n - 1; i >= 0; i--) g.lineTo(R[i][0], R[i][1]);
  g.closePath(); g.fill();
  // A round root, so a stem starts as a stem and not a cut-off bar.
  g.beginPath(); g.arc(pts[0].x, pts[0].y, Math.max(0.3, hw(0) - Math.abs(off || 0) * hw(0)), 0, 7); g.fill();
}
function cageStems(g, geo, mul, off) {
  geo.stems.forEach(function (s) { cageRibbon(g, s.pts, function (i) { return s.w * mul * 0.5 * (1 - 0.55 * s.pts[i].f); }, off); });
}
function cageCurls(g, geo, mul) {
  geo.curls.forEach(function (c) { cageRibbon(g, c.pts, function (i) { return Math.max(0.25, c.w * mul * 0.5 * (1 - 0.8 * c.pts[i].t)); }); });
}
// A point on a leaf: u runs from its base (0) to its tip (1), v across it in half-widths; bend curves it.
function cageLeafPt(l, u, v, grow) {
  var L = l.L + grow * 2, W = l.W + grow, nx = -l.dy, ny = l.dx, vv = v * W + l.bend * u * u * l.L * 0.5;
  return [l.x - l.dx * grow + l.dx * u * L + nx * vv, l.y - l.dy * grow + l.dy * u * L + ny * vv];
}
// Ivy: three lobes and a heart-shaped base. The others: a pointed oval (narrow for thorn vines).
var CAGE_IVY = [[0.1, 0], [-0.06, 0.22, 0.1, 0.4], [0.2, 0.62, 0.42, 0.5], [0.44, 0.3, 0.57, 0.24], [0.8, 0.2, 1, 0],
  [0.8, -0.2, 0.57, -0.24], [0.44, -0.3, 0.42, -0.5], [0.2, -0.62, 0.1, -0.4], [-0.06, -0.22, 0.1, 0]];
function cageLeafPath(g, l, grow) {
  var P = function (u, v) { return cageLeafPt(l, u, v, grow || 0); };
  g.beginPath();
  if (l.shape === 'ivy') {
    var p = P(CAGE_IVY[0][0], CAGE_IVY[0][1]); g.moveTo(p[0], p[1]);
    for (var i = 1; i < CAGE_IVY.length; i++) { var q = CAGE_IVY[i], c = P(q[0], q[1]), e = P(q[2], q[3]); g.quadraticCurveTo(c[0], c[1], e[0], e[1]); }
  } else {
    var a = P(0, 0), c1 = P(0.16, 1.2), c2 = P(0.6, 1.1), t = P(1, 0), c3 = P(0.62, -1.0), c4 = P(0.14, -1.1);
    g.moveTo(a[0], a[1]); g.bezierCurveTo(c1[0], c1[1], c2[0], c2[1], t[0], t[1]); g.bezierCurveTo(c3[0], c3[1], c4[0], c4[1], a[0], a[1]);
  }
  g.closePath();
}
function cageLine(g, a, b, c) { g.beginPath(); g.moveTo(a[0], a[1]); if (c) g.quadraticCurveTo(c[0], c[1], b[0], b[1]); else g.lineTo(b[0], b[1]); g.stroke(); }
// A painted leaf: darker at the stalk, the half facing the light a shade lighter, fine veins, a dark rim.
function cageLeafPaint(g, l, c, k) {
  var V = c.vine, T = cageTones(c), dark = V.leaf[0], lite = V.leaf[2];
  // Some leaves younger and brighter, some older and darker.
  var base = l.tone < 0.3 ? mix(V.leaf[1], dark, 0.32) : l.tone > 0.78 ? mix(V.leaf[1], lite, 0.22) : V.leaf[1];
  var P = function (u, v) { return cageLeafPt(l, u, v, 0); }, b0 = P(0, 0), tip = P(1, 0);
  var gr = g.createLinearGradient(b0[0], b0[1], tip[0], tip[1]); gr.addColorStop(0, mix(base, dark, 0.6)); gr.addColorStop(0.55, base); gr.addColorStop(1, mix(base, lite, 0.18));
  cageLeafPath(g, l, 0); g.fillStyle = gr; g.fill();
  g.save(); cageLeafPath(g, l, 0); g.clip();
  // Light comes from the face's upper left.
  var lit = (-l.dy * -0.6 + l.dx * -0.8) > 0 ? 1 : -1, h0 = P(-0.3, 0), h1 = P(1.3, 0), h2 = P(1.3, lit * 3), h3 = P(-0.3, lit * 3);
  g.beginPath(); g.moveTo(h0[0], h0[1]); g.lineTo(h1[0], h1[1]); g.lineTo(h2[0], h2[1]); g.lineTo(h3[0], h3[1]); g.closePath();
  g.fillStyle = hexA(lite, 0.26); g.fill();
  g.lineCap = 'round'; g.strokeStyle = hexA(lite, 0.62); g.lineWidth = Math.max(0.7, 0.85 * k);
  cageLine(g, P(0.04, 0), P(0.9, 0), P(0.5, 0));
  g.strokeStyle = hexA(lite, 0.36); g.lineWidth = Math.max(0.5, 0.6 * k);
  if (l.shape === 'ivy') { cageLine(g, P(0.14, 0), P(0.4, 0.42), P(0.26, 0.3)); cageLine(g, P(0.14, 0), P(0.4, -0.42), P(0.26, -0.3)); cageLine(g, P(0.1, 0), P(0.1, 0.32), P(0.02, 0.18)); cageLine(g, P(0.1, 0), P(0.1, -0.32), P(0.02, -0.18)); }
  else [0.24, 0.44, 0.64].forEach(function (u) { cageLine(g, P(u, 0), P(u + 0.2, 0.62), P(u + 0.08, 0.4)); cageLine(g, P(u, 0), P(u + 0.2, -0.62), P(u + 0.08, -0.4)); });
  g.restore();
  cageLeafPath(g, l, 0); g.strokeStyle = hexA(T.edge, 0.85); g.lineWidth = Math.max(0.6, 0.75 * k); g.stroke();
}
function cageThornPath(g, t, grow) {
  var w = t.L * 0.32 + grow, a = [t.x - t.tx * w, t.y - t.ty * w], b = [t.x + t.tx * w, t.y + t.ty * w], tip = [t.x + t.px * (t.L + grow) - t.tx * t.L * 0.45, t.y + t.py * (t.L + grow) - t.ty * t.L * 0.45];
  var m = [t.x + t.px * t.L * 0.45, t.y + t.py * t.L * 0.45];
  g.beginPath(); g.moveTo(a[0], a[1]); g.quadraticCurveTo(m[0] - t.tx * w * 0.3, m[1] - t.ty * w * 0.3, tip[0], tip[1]); g.quadraticCurveTo(m[0] + t.tx * w, m[1] + t.ty * w, b[0], b[1]); g.closePath();
}
// Every piece of the vine in one flat colour, grown by `grow` canvas px: the groove it lies in, and the
// height and surface passes.
function cageVineMask(g, geo, grow, fill) {
  g.fillStyle = fill;
  geo.stems.forEach(function (s) { cageRibbon(g, s.pts, function (i) { return s.w * 0.5 * (1 - 0.55 * s.pts[i].f) + grow; }); });
  geo.curls.forEach(function (c) { cageRibbon(g, c.pts, function (i) { return Math.max(0.25, c.w * 0.5 * (1 - 0.8 * c.pts[i].t)) + grow * 0.6; }); });
  geo.leaves.forEach(function (l) { cageLeafPath(g, l, grow); g.fill(); g.lineWidth = geo.gw * 0.45 + grow * 2; g.strokeStyle = fill; g.lineCap = 'round'; cageLine(g, [l.sx, l.sy], [l.x, l.y]); });
  geo.thorns.forEach(function (t) { cageThornPath(g, t, grow); g.fill(); });
  geo.berries.forEach(function (b) { g.beginPath(); g.arc(b.x, b.y, b.r + grow, 0, 7); g.fill(); });
  geo.knots.forEach(function (n) { g.beginPath(); g.arc(n.x, n.y, n.r + grow, 0, 7); g.fill(); });
}
// The plate's metal: a gradient, a little tarnish, then hammered dimples or brush lines.
function cageMetal(g, lay, c, geo) {
  var cv = lay.cv, k = geo.k, rnd = mulberry(cageHash(c.key + '|' + lay.label + '|' + cv)), i;
  var gr = g.createLinearGradient(cv * 0.12, cv * 0.04, cv * 0.9, cv * 0.96);
  gr.addColorStop(0, c.base.grad[0]); gr.addColorStop(1, c.base.grad[1]); g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
  g.save(); try { g.filter = 'blur(' + (13 * k) + 'px)'; } catch (e) {}
  for (i = 0; i < 9; i++) { g.fillStyle = hexA(c.base.grad[1], 0.2 + rnd() * 0.2); g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (26 + rnd() * 44) * k, 0, 7); g.fill(); }
  g.restore();
  if (c.tex === 'brush') {
    g.lineCap = 'round';
    for (i = 0; i < 420; i++) {
      var x = rnd() * cv, y = rnd() * cv, ln = (40 + rnd() * 120) * k;
      g.strokeStyle = rnd() > 0.5 ? 'rgba(255,255,255,' + (0.03 + rnd() * 0.05) + ')' : 'rgba(0,0,0,' + (0.06 + rnd() * 0.07) + ')';
      g.lineWidth = (0.5 + rnd() * 0.9) * k; g.beginPath(); g.moveTo(x, y); g.lineTo(x + ln * 0.96, y - ln * 0.28); g.stroke();
    }
  } else {
    geo.dimples.forEach(function (d) {
      var rg = g.createRadialGradient(d.x + d.r * 0.3, d.y + d.r * 0.3, 0, d.x, d.y, d.r);
      rg.addColorStop(0, 'rgba(255,255,255,.05)'); rg.addColorStop(0.75, 'rgba(0,0,0,0)'); rg.addColorStop(1, 'rgba(0,0,0,.05)');
      g.fillStyle = rg; g.beginPath(); g.arc(d.x, d.y, d.r, 0, 7); g.fill();
    });
  }
}
// The band the number sits on: a polished border round a dark enamelled field.
function cageBands(g, lay, c, geo) {
  var T = cageTones(c), bw = Math.max(2.5, geo.k * 2.8), cv = lay.cv;
  var fg = g.createLinearGradient(0, lay.cy - cv * 0.3, 0, lay.cy + cv * 0.3); fg.addColorStop(0, c.band[0]); fg.addColorStop(1, c.band[1]);
  g.save(); polyPath(g, geo.clip); g.clip();
  geo.bands.forEach(function (p) {
    cageRR(g, p, 0); g.fillStyle = T.polish; g.fill();
    cageRR(g, p, -bw); g.fillStyle = fg; g.fill();
    // A hairline just inside the border, like a cut.
    cageRR(g, p, -bw - geo.k * 3); g.strokeStyle = 'rgba(255,255,255,.1)'; g.lineWidth = Math.max(1, geo.k * 0.9); g.stroke();
  });
  g.restore();
}
function cageNumber(g, lay, c) {
  var w = strokeW(lay) * 1.06;
  g.save(); g.translate(0, w * 0.14); g.fillStyle = g.strokeStyle = 'rgba(0,0,0,.8)'; paintNumeral(g, lay, lay.style, w * 1.6); g.restore();
  g.fillStyle = g.strokeStyle = c.ink; paintNumeral(g, lay, lay.style, w);
}
function cagePaint(g, lay, c) {
  var geo = cageGeom(lay, c), k = geo.k, T = cageTones(c), V = c.vine;
  cageMetal(g, lay, c, geo);
  // The groove the vine is set into: a narrow dark channel just wider than the vine.
  cageVineMask(g, geo, 1.4 * k, T.groove);
  // The face's own edge: a dark line where the plate meets its rim.
  g.save(); polyPath(g, lay.inner); g.clip(); g.strokeStyle = 'rgba(0,0,0,.42)'; g.lineWidth = 7 * k; try { g.filter = 'blur(' + (2.5 * k) + 'px)'; } catch (e) {}
  polyPath(g, lay.inner); g.stroke(); g.restore();
  cageBands(g, lay, c, geo);
  // A d4's corner numbers reach the tapering tip: the face's edge trims them like the rest of the plate.
  g.save(); polyPath(g, lay.inner); g.clip(); cageNumber(g, lay, c); g.restore();
  // Shade the metal round each window, then cut it.
  g.save(); g.globalCompositeOperation = 'source-atop'; g.globalAlpha = 0.7; try { g.filter = 'blur(' + (5 * k) + 'px)'; } catch (e) {}
  g.drawImage(cageTint(geo.win, 'rgb(6,4,2)'), 0, 0); g.restore();
  g.save(); g.globalCompositeOperation = 'destination-out'; g.drawImage(geo.win, 0, 0); g.restore();
  // The vine, over its groove; leaves and tendrils may reach out across a window.
  g.save(); polyPath(g, lay.inner); g.clip();
  g.fillStyle = T.bark; cageCurls(g, geo, 1); cageStems(g, geo, 1);
  // Bark: a lit side, and fine twists across the stem.
  g.fillStyle = hexA(T.barkLit, 0.75); cageStems(g, geo, 0.42, -0.9);
  g.strokeStyle = hexA(mix(V.stem, '#000000', 0.5), 0.45); g.lineWidth = Math.max(0.5, 0.55 * k); g.lineCap = 'round';
  geo.stems.forEach(function (s) {
    for (var i = 3; i < s.pts.length; i += Math.max(2, Math.round(s.w * 1.1 / (1.6 * k)))) {
      var p = s.pts[i], h = s.w * 0.5 * (1 - 0.55 * p.f), nx = -p.ty, ny = p.tx;
      cageLine(g, [p.x + nx * h * 0.8 - p.tx * h * 0.6, p.y + ny * h * 0.8 - p.ty * h * 0.6], [p.x - nx * h * 0.8 + p.tx * h * 0.6, p.y - ny * h * 0.8 + p.ty * h * 0.6]);
    }
  });
  geo.thorns.forEach(function (t) { cageThornPath(g, t, 0); g.fillStyle = T.bark; g.fill(); g.strokeStyle = hexA(T.barkLit, 0.5); g.lineWidth = Math.max(0.5, 0.5 * k); g.stroke(); });
  g.strokeStyle = T.bark; g.lineCap = 'round';
  geo.leaves.forEach(function (l) { g.lineWidth = geo.gw * 0.42; cageLine(g, [l.sx, l.sy], [l.x, l.y]); });
  geo.leaves.forEach(function (l) { cageLeafPaint(g, l, c, k); });
  geo.berries.forEach(function (b) {
    var rg = g.createRadialGradient(b.x - b.r * 0.35, b.y - b.r * 0.35, 0, b.x, b.y, b.r); rg.addColorStop(0, mix(V.berry, '#ffffff', 0.35)); rg.addColorStop(0.45, V.berry); rg.addColorStop(1, mix(V.berry, '#000000', 0.5));
    g.fillStyle = rg; g.beginPath(); g.arc(b.x, b.y, b.r, 0, 7); g.fill();
  });
  geo.knots.forEach(function (n) { g.fillStyle = T.bark; g.beginPath(); g.arc(n.x, n.y, n.r, 0, 7); g.fill(); g.fillStyle = hexA(T.barkLit, 0.5); g.beginPath(); g.arc(n.x - n.r * 0.25, n.y - n.r * 0.25, n.r * 0.45, 0, 7); g.fill(); });
  g.restore();
}
function cageHeight(g, lay, c) {
  var geo = cageGeom(lay, c), cv = lay.cv, k = geo.k, sc = g.getTransform ? g.getTransform().a : 1, win = geo.win;
  // Hammering: shallow dents. Brushing: faint fine ridges.
  if (c.tex === 'brush') {
    var rnd = mulberry(cageHash('b' + c.key + lay.label)); g.lineCap = 'round';
    for (var i = 0; i < 260; i++) {
      var x = rnd() * cv, y = rnd() * cv, ln = (40 + rnd() * 110) * k; g.strokeStyle = rnd() > 0.5 ? '#ececec' : '#f8f8f8'; g.lineWidth = 0.9 * k;
      g.beginPath(); g.moveTo(x, y); g.lineTo(x + ln * 0.96, y - ln * 0.28); g.stroke();
    }
  } else {
    geo.dimples.forEach(function (d) {
      var rg = g.createRadialGradient(d.x, d.y, 0, d.x, d.y, d.r); rg.addColorStop(0, '#dcdcdc'); rg.addColorStop(1, '#ffffff');
      g.fillStyle = rg; g.beginPath(); g.arc(d.x, d.y, d.r, 0, 7); g.fill();
    });
  }
  g.save(); try { g.filter = 'blur(' + (0.7 * k * sc) + 'px)'; } catch (e) {}
  // The groove: cut well below the plate.
  cageVineMask(g, geo, 1.4 * k, '#4a4a4a');
  g.restore();
  // Windows: the plate falls away at their edges.
  g.save(); try { g.filter = 'blur(' + (2.6 * k * sc) + 'px)'; } catch (e) {}
  g.drawImage(cageTint(win, '#000'), 0, 0); g.restore();
  // Bands: a recessed field in a raised border, the number standing out of it.
  g.save(); try { g.filter = 'blur(' + (0.8 * k * sc) + 'px)'; } catch (e) {}
  g.save(); polyPath(g, geo.clip); g.clip();
  geo.bands.forEach(function (p) { cageRR(g, p, 0); g.fillStyle = '#fff'; g.fill(); cageRR(g, p, -Math.max(2.5, k * 2.8)); g.fillStyle = '#b4b4b4'; g.fill(); });
  g.restore();
  g.fillStyle = g.strokeStyle = '#e4e4e4'; paintNumeral(g, lay, lay.style, strokeW(lay) * 1.06);
  g.restore();
  // The vine stands in its groove almost level with the plate: round stems, leaves domed either side of
  // a sunken midrib, berries as beads.
  g.save(); polyPath(g, lay.inner); g.clip(); try { g.filter = 'blur(' + (0.9 * k * sc) + 'px)'; } catch (e) {}
  cageVineMask(g, geo, 0, '#a8a8a8');
  g.fillStyle = '#cfcfcf'; cageStems(g, geo, 0.45); cageCurls(g, geo, 0.45);
  geo.leaves.forEach(function (l) {
    var m = cageLeafPt(l, 0.45, 0, 0), rg = g.createRadialGradient(m[0], m[1], 0, m[0], m[1], l.L * 0.55);
    rg.addColorStop(0, 'rgba(214,214,214,1)'); rg.addColorStop(1, 'rgba(214,214,214,0)');
    g.save(); cageLeafPath(g, l, 0); g.clip(); g.fillStyle = rg; g.fillRect(m[0] - l.L, m[1] - l.L, l.L * 2, l.L * 2); g.restore();
    g.strokeStyle = '#949494'; g.lineWidth = Math.max(0.7, 0.8 * k); g.lineCap = 'round'; cageLine(g, cageLeafPt(l, 0.04, 0, 0), cageLeafPt(l, 0.88, 0, 0), cageLeafPt(l, 0.5, 0, 0));
  });
  geo.berries.forEach(function (b) { var rg = g.createRadialGradient(b.x, b.y, 0, b.x, b.y, b.r); rg.addColorStop(0, '#e8e8e8'); rg.addColorStop(1, '#a0a0a0'); g.fillStyle = rg; g.beginPath(); g.arc(b.x, b.y, b.r, 0, 7); g.fill(); });
  g.restore();
  // The height pass is the last to read the plate's geometry; its window image is the heavy part.
  geo.win.width = 0; lay._cage = null;
}
function cageSurface(g, lay, c) {
  var geo = cageGeom(lay, c), cv = lay.cv, k = geo.k, R = function (r, m) { return 'rgb(0,' + Math.round(r * 255) + ',' + Math.round(m * 255) + ')'; };
  g.fillStyle = R(c.base.rough, 1); g.fillRect(0, 0, cv, cv);
  // The groove: dark, rough, bare metal.
  cageVineMask(g, geo, 1.4 * k, R(0.72, 1));
  // Bands: a half-metal enamel; the number a satin paint.
  g.save(); polyPath(g, geo.clip); g.clip();
  geo.bands.forEach(function (p) { cageRR(g, p, -Math.max(2.5, k * 2.8)); g.fillStyle = R(0.5, 0.5); g.fill(); });
  g.restore();
  g.fillStyle = g.strokeStyle = R(0.55, 0.06); paintNumeral(g, lay, lay.style, strokeW(lay) * 1.06);
  // The vine: dull bark, waxy leaves, glossy berries.
  g.save(); polyPath(g, lay.inner); g.clip();
  cageVineMask(g, geo, 0, R(0.62, 0));
  geo.leaves.forEach(function (l) { cageLeafPath(g, l, 0); g.fillStyle = R(0.34, 0); g.fill(); });
  geo.berries.forEach(function (b) { g.fillStyle = R(0.14, 0); g.beginPath(); g.arc(b.x, b.y, b.r, 0, 7); g.fill(); });
  g.restore();
}

// ---------- The orb ----------
// Stones drawn as an equirectangular image (it wraps, so blobs are drawn at both seams): the pattern
// turns with the orb, and doubles as the inner glow's mask.
var CAGE_ORBTEX = {};
function cageOrbTex(T, kind) {
  if (CAGE_ORBTEX[kind]) return CAGE_ORBTEX[kind];
  var W = 256, H = 128, cv = canvas(W, H), g = cv.getContext('2d'), rnd = mulberry(cageHash('orb' + kind)), i;
  var P = {
    moon: { base: '#c2cfe6', blobs: [['#ffffff', 0.6, 22, 64], ['#7f9fe6', 0.32, 20, 60], ['#e9f0ff', 0.5, 14, 40], ['#4d6fbd', 0.3, 16, 48]], n: 46 },
    ruby: { base: '#7c0719', blobs: [['#e0203e', 0.6, 16, 56], ['#2e0108', 0.55, 14, 50], ['#ff7a8a', 0.3, 8, 24]], n: 40 },
    pearl: { base: '#eee7dc', blobs: [['#ffc4de', 0.55, 18, 54], ['#bfe6ff', 0.55, 18, 54], ['#fff0c4', 0.5, 14, 40], ['#ffffff', 0.5, 12, 34]], n: 44 },
    amber: { base: '#dd9420', blobs: [['#ffd868', 0.65, 16, 56], ['#8f4605', 0.6, 16, 50], ['#ff9a2a', 0.4, 10, 30]], n: 40 },
    amethyst: { base: '#6a2db3', blobs: [['#c9a2ff', 0.55, 16, 56], ['#2c0d5c', 0.6, 14, 50], ['#e4ccff', 0.3, 8, 24]], n: 40 }
  }[kind] || { base: '#ffffff', blobs: [], n: 0 };
  g.fillStyle = P.base; g.fillRect(0, 0, W, H);
  g.save(); try { g.filter = 'blur(5px)'; } catch (e) {}
  for (i = 0; i < P.n; i++) {
    var b = P.blobs[i % P.blobs.length], x = rnd() * W, y = (0.12 + rnd() * 0.76) * H, r = b[2] + rnd() * (b[3] - b[2]);
    g.fillStyle = hexA(b[0], b[1] * (0.6 + rnd() * 0.6));
    [-W, 0, W].forEach(function (o) { g.beginPath(); g.ellipse(x + o, y, r, r * (0.45 + rnd() * 0.4), rnd() * 3, 0, 7); g.fill(); });
  }
  g.restore();
  if (kind === 'ruby' || kind === 'amethyst') {
    // Silk and growth zoning: fine light needles, and bands across the stone.
    g.lineWidth = 1; g.lineCap = 'round';
    for (i = 0; i < 70; i++) { var x0 = rnd() * W, y0 = rnd() * H, a = rnd() < 0.5 ? 0.5 : 2.1, l = 8 + rnd() * 22; g.strokeStyle = kind === 'ruby' ? 'rgba(255,150,160,.2)' : 'rgba(235,215,255,.18)'; g.beginPath(); g.moveTo(x0, y0); g.lineTo(x0 + Math.cos(a) * l, y0 + Math.sin(a) * l); g.stroke(); }
  }
  if (kind === 'amber') {
    // Bubbles and flecks trapped in the resin.
    for (i = 0; i < 26; i++) {
      var bx = rnd() * W, by = (0.1 + rnd() * 0.8) * H, br = 1.5 + rnd() * 4;
      g.strokeStyle = 'rgba(255,238,180,.6)'; g.fillStyle = 'rgba(255,214,120,.25)'; g.lineWidth = 1; g.beginPath(); g.arc(bx, by, br, 0, 7); g.fill(); g.stroke();
    }
    for (i = 0; i < 14; i++) { g.fillStyle = 'rgba(70,32,4,.5)'; g.beginPath(); g.arc(rnd() * W, rnd() * H, 0.8 + rnd() * 1.6, 0, 7); g.fill(); }
  }
  if (kind === 'pearl') {
    // Nacre: faint bands of colour that run round the pearl.
    for (i = 0; i < 9; i++) { g.fillStyle = hexA(['#ffd6ec', '#d4f1ff', '#fff3c8'][i % 3], 0.14); g.fillRect(0, rnd() * H, W, 3 + rnd() * 9); }
  }
  var t = new T.CanvasTexture(cv); t.colorSpace = T.SRGBColorSpace; t.wrapS = T.RepeatWrapping; t.anisotropy = 4; share(t);
  CAGE_ORBTEX[kind] = t;
  return t;
}
var CAGE_ORBGEO = {};
function cageOrbGeo(T, hd) {
  var k = hd ? 'hd' : 'basic';
  if (!CAGE_ORBGEO[k]) { CAGE_ORBGEO[k] = hd ? new T.SphereGeometry(1, 40, 28) : new T.SphereGeometry(1, 22, 15); share(CAGE_ORBGEO[k]); }
  return CAGE_ORBGEO[k];
}
// A stone, not a ball of coloured plastic: a hard gloss over its own pattern, a bright core that falls
// off towards the edge (light from inside), and in HD the film and sheen real stones show.
function cageOrbMat(T, c, hd) {
  var o = c.orb, tex = cageOrbTex(T, o.kind);
  var p = { color: o.col, map: tex, emissive: o.glow, emissiveMap: tex, emissiveIntensity: o.k, roughness: o.rough, metalness: 0, ior: o.ior, specularIntensity: 1, envMapIntensity: o.env };
  if (hd) Object.assign(p, { clearcoat: 0.7, clearcoatRoughness: 0.1, iridescence: o.iri, iridescenceIOR: 1.35, iridescenceThicknessRange: [180, 520], sheen: 0.7, sheenRoughness: 0.45, sheenColor: new T.Color(o.sheen) });
  var m = new T.MeshPhysicalMaterial(p);
  m.onBeforeCompile = function (s) {
    s.fragmentShader = s.fragmentShader.replace('#include <emissivemap_fragment>',
      '#include <emissivemap_fragment>\n\tfloat cgNV = clamp( dot( normal, normalize( vViewPosition ) ), 0.0, 1.0 );\n\ttotalEmissiveRadiance *= 0.2 + 1.2 * pow( cgNV, 1.7 );');
  };
  m.customProgramCacheKey = function () { return 'cage:orb:' + (hd ? 'hd' : 'basic'); };
  return m;
}
// The orb floats in the middle of the die and turns on its own: it keeps its own bearing in the room
// while the die tumbles round it, turns faster the harder the die spins, and settles to a slow turn.
function cageBuild(T, die, q) {
  var hd = q.detail !== 'basic', sd = die.sd, c = die.th, inr = 9;
  sd.faces.forEach(function (f) { inr = Math.min(inr, dot(f.c, f.n)); });
  inr *= die.size;
  var R = inr * (die.sides === 4 ? 0.84 : 0.72), mat = cageOrbMat(T, c, hd), orb = new T.Mesh(cageOrbGeo(T, hd), mat);
  orb.scale.setScalar(R); orb.castShadow = false; orb.receiveShadow = false;
  // What the plates' inside needs to know: how far the nearest face is from the middle.
  die.mat.userData.cg = { size: inr };
  var world = new T.Quaternion().setFromEuler(new T.Euler(0.5, 0.2, 0.3)), qa = new T.Quaternion(), qg = new T.Quaternion(), axis = new T.Vector3(0.28, 1, 0.18).normalize();
  var rate = 0.5, t = (cageHash(c.key + die.sides) % 100) / 7, drift = die.size * 0.012;
  die.cage = { orb: orb, mat: mat, base: c.orb.k };
  return {
    parts: [orb],
    tick: function (dt, spin) {
      t += dt;
      rate += ((0.5 + Math.min(spin, 14) * 0.3) - rate) * Math.min(1, dt * (spin > 0.2 ? 5 : 1.1));
      qa.setFromAxisAngle(axis, rate * dt); world.premultiply(qa).normalize();
      die.group.getWorldQuaternion(qg);
      orb.quaternion.copy(qg).invert().multiply(world);
      orb.position.set(Math.sin(t * 0.8) * drift, Math.sin(t * 1.1 + 1) * drift, Math.sin(t * 0.7 + 2) * drift);
    },
    dispose: function () { mat.dispose(); }
  };
}

// ---------- The plates' material ----------
// The inside of the frame, seen through a window, is a plain brushed metal liner lit by the orb: it
// is the back of the same triangles, so it must not show the face art backwards or leave a hole.
function cageShader(s, c) {
  var m = this, Col = m.color.constructor, cg = m.userData.cg || { size: 1 };
  s.uniforms.uCgLiner = { value: new Col(c.liner) };
  s.uniforms.uCgGlow = { value: new Col(c.orb.glow) };
  s.uniforms.uCgSize = { value: cg.size };
  s.uniforms.uCgK = { value: c.orb.k };
  s.vertexShader = 'varying vec3 vCgLocal;\n' + s.vertexShader.replace('#include <begin_vertex>', '#include <begin_vertex>\n\tvCgLocal = position;');
  s.fragmentShader = 'uniform vec3 uCgLiner; uniform vec3 uCgGlow; uniform float uCgSize; uniform float uCgK;\nvarying vec3 vCgLocal;\n' + s.fragmentShader
    .replace('#include <map_fragment>',
      '#include <map_fragment>\n\tbool cgBack = ! gl_FrontFacing;\n\tfloat cgD = max( length( vCgLocal ) / uCgSize, 1.0 );\n' +
      '\tif ( cgBack ) {\n\t\tfloat cgB = 0.5 + 0.5 * sin( dot( vCgLocal, vec3( 0.71, 0.24, 0.66 ) ) / uCgSize * 160.0 );\n' +
      '\t\tdiffuseColor.rgb = uCgLiner * diffuse * ( 0.8 + 0.2 * cgB );\n\t\tdiffuseColor.a = 1.0;\n\t}')
    .replace('#include <roughnessmap_fragment>', '#include <roughnessmap_fragment>\n\tif ( cgBack ) roughnessFactor = 0.46;')
    .replace('#include <metalnessmap_fragment>', '#include <metalnessmap_fragment>\n\tif ( cgBack ) metalnessFactor = 0.7;')
    .replace('#include <normal_fragment_maps>', '#include <normal_fragment_maps>\n\tif ( cgBack ) normal = nonPerturbedNormal;')
    .replace('#include <opaque_fragment>',
      'if ( cgBack ) outgoingLight += diffuseColor.rgb * uCgGlow * uCgK * 2.6 * pow( 1.0 / cgD, 2.4 );\n#include <opaque_fragment>');
}

registerSet({
  key: 'cage', name: 'Verdant cage', show: 'overgrow', bev: 0.065,
  note: 'An open frame of dark metal with living vines inlaid in its grooves, round an orb that floats and turns inside',
  colours: CAGE_COLOURS,
  paint: function (g, lay, c) { cagePaint(g, lay, c); },
  surface: function (g, lay, c) { cageSurface(g, lay, c); },
  height: function (g, lay, c) { cageHeight(g, lay, c); },
  relief: 1,
  // The frame's edges are the most of the metal anyone sees: kept dark and a little worn.
  rim: function (c) { return { col: mix(c.base.grad[0], c.base.grad[1], 0.6), rough: 0.48, metal: 1 }; },
  see: 'cutout',
  // Alpha to coverage softens the cut edges of the windows under multisampling.
  physical: function () { return { envMapIntensity: 1.1, specularIntensity: 1, alphaToCoverage: true }; },
  shader: cageShader,
  build: cageBuild,
  cover: function () { return 0; }
});
