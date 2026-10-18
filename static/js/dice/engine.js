/**
 * engine.js — the thrown dice behind Chronicle.dice (static/js/dice.js), as an ES module.
 *
 * dice.js decides every value and every crit before anything is drawn; this module only shows them.
 * three.js draws and cannon-es simulates, both loaded on the first throw. The throw is simulated
 * ahead, then each die is turned by one of its own symmetries so the decided face is the one that
 * lands: the shape is identical after that turn, so the motion is still true physics.
 *
 * Twelve sets of dice, each with its own colours and its own way of lighting the number that lands
 * (a show). Jewelled, Confetti glass and Acrylic are built here; the other nine sets live in
 * ./sets/<key>.js and the crit effects in ./crits/<key>.js, each imported the first time a roll
 * needs it. Those files import what they share from this module by its bare URL, which is why
 * dice.js imports it without a version query: every importer has to name the same URL, or the
 * browser would run two copies with two registries.
 */

// ---------- Libraries ----------
// three.js (with its post-processing add-ons, bundled as one module) and cannon-es, from the
// hashed URLs dice.js passes in. Only same-origin /static paths are ever imported.
var LIB_URL = /^\/static\/[A-Za-z0-9_.\-\/]+\.js(\?v=[A-Za-z0-9._\-]+)?$/;
var LIBS = { three: '/static/vendor/three-dice.min.js', cannon: '/static/vendor/cannon-es.js' };
function configure(o) {
  ['three', 'cannon'].forEach(function (k) { if (o && typeof o[k] === 'string' && LIB_URL.test(o[k]) && o[k].indexOf('..') < 0) LIBS[k] = o[k]; });
}
var LIB = null;
function libs() {
  if (!LIB) {
    LIB = Promise.all([import(LIBS.three), import(LIBS.cannon)]).then(function (m) { return { T: m[0], C: m[1] }; });
    // A failed load must not stick: the next roll tries again.
    LIB.catch(function () { LIB = null; });
  }
  return LIB;
}

var PX = 46;           // screen pixels per world unit on the table
var STEP = 1 / 120;    // fixed physics step, so a recorded throw replays exactly
var FOV = 28;
var TAN = Math.tan(FOV * Math.PI / 360);
var BEV = 0.075;       // how far the metal rim runs in from each edge, in die radii

// ---------- Looks ----------
// A look is a base stone and, for the jewelled looks, a contrasting metal for the rims and the numbers.
// burn is the colour the landed number glows.
var THEMES = [
  { key: 'onyx', jewel: '#c8102e', name: 'Onyx and silver', note: 'Polished black stone, bright silver rims', group: 'jewel',
    base: { grad: ['#2a2b30', '#060608'], veins: { color: '#9aa0ab', n: 5, w: 1.2, a: 0.22 }, rough: 0.28, coat: 1 },
    metal: { color: '#e4e7ec', rough: 0.2 }, burn: '#a9dcff' },
  { key: 'ivory', jewel: '#0b7a4b', name: 'Ivory and gold', note: 'Warm ivory, polished gold', group: 'jewel',
    base: { grad: ['#fbf5e6', '#d9ccb0'], streaks: { color: '#b9a682', n: 60, a: 0.12 }, rough: 0.42, coat: 0.6 },
    metal: { color: '#f0c25e', rough: 0.24 }, burn: '#ff6a1a' },
  { key: 'marble', jewel: '#1f3fbf', name: 'Marble and black iron', note: 'White marble, dark iron', group: 'jewel',
    base: { grad: ['#fbfbfa', '#dcdcd9'], veins: { color: '#8b8d93', n: 7, w: 1.6, a: 0.42 }, rough: 0.3, coat: 0.9 },
    metal: { color: '#3b3e45', rough: 0.34 }, burn: '#ff2a2a' },
  { key: 'lapis', jewel: '#b8102a', name: 'Lapis and gold', note: 'Deep blue flecked with gold', group: 'jewel',
    base: { grad: ['#2d4fb0', '#0d1d55'], flecks: { color: '#e9c46a', n: 140 }, veins: { color: '#c9d3ee', n: 3, w: 1, a: 0.18 }, rough: 0.32, coat: 0.8 },
    metal: { color: '#eabf5c', rough: 0.22 }, burn: '#ffd36b' },
  { key: 'jade', jewel: '#6d28c9', name: 'Jade and bronze', note: 'Mottled green jade, old bronze', group: 'jewel',
    base: { grad: ['#4fa37b', '#164f38'], mottle: { color: '#b9e3c8', n: 26, a: 0.18 }, rough: 0.3, coat: 0.7 },
    metal: { color: '#c48a52', rough: 0.3 }, burn: '#7dffb0' },
  { key: 'garnet', jewel: '#eef3fb', name: 'Garnet and rose gold', note: 'Dark red stone, soft rose gold', group: 'jewel',
    base: { grad: ['#8e1b2c', '#33060f'], mottle: { color: '#ff7a8c', n: 18, a: 0.1 }, veins: { color: '#f2a0a8', n: 3, w: 1, a: 0.14 }, rough: 0.26, coat: 1 },
    metal: { color: '#efbfae', rough: 0.24 }, burn: '#ff6b8a' },
  { key: 'amethyst', jewel: '#0e8f6a', name: 'Amethyst and silver', note: 'Violet crystal, cool silver', group: 'jewel',
    base: { grad: ['#7a4cc2', '#24104a'], mottle: { color: '#d7b8ff', n: 22, a: 0.14 }, rough: 0.24, coat: 1 },
    metal: { color: '#dfe3ea', rough: 0.18 }, burn: '#d39bff' }
];
// A light stone needs its face dimmed under a burning number, or the glow washes out to white.
function isLight(th) { var n = parseInt(th.base.grad[0].slice(1), 16); return (0.3 * (n >> 16 & 255) + 0.59 * (n >> 8 & 255) + 0.11 * (n & 255)) / 255 > 0.62; }

// The colours' contrasting colour: the metal on the jewelled colours, the ink on the carved ones.
function accentOf(th) { return th.metal ? th.metal.color : th.ink; }
// The stone set at a jewelled die's corners: a deep jewel tone against the body and the metal.
function jewelOf(th) { return th.jewel || mix(th.burn, '#000000', 0.25); }
function accentRough(th) { return th.metal ? th.metal.rough : 0.3; }

// ---------- Geometry: vertices, then faces found as the convex hull ----------
function v3(a) { return { x: a[0], y: a[1], z: a[2] }; }
function sub(a, b) { return { x: a.x - b.x, y: a.y - b.y, z: a.z - b.z }; }
function add(a, b) { return { x: a.x + b.x, y: a.y + b.y, z: a.z + b.z }; }
function mul(a, s) { return { x: a.x * s, y: a.y * s, z: a.z * s }; }
function dot(a, b) { return a.x * b.x + a.y * b.y + a.z * b.z; }
function cross(a, b) { return { x: a.y * b.z - a.z * b.y, y: a.z * b.x - a.x * b.z, z: a.x * b.y - a.y * b.x }; }
function len(a) { return Math.sqrt(dot(a, a)); }
function norm(a) { var l = len(a) || 1; return mul(a, 1 / l); }

function solidVerts(sides) {
  var P = (1 + Math.sqrt(5)) / 2, I = 1 / P, out = [], s, t;
  if (sides === 4) { out = [[1, 1, 1], [1, -1, -1], [-1, 1, -1], [-1, -1, 1]]; }
  else if (sides === 6) { for (s = 0; s < 8; s++) out.push([s & 1 ? 1 : -1, s & 2 ? 1 : -1, s & 4 ? 1 : -1]); }
  else if (sides === 8) { out = [[1, 0, 0], [-1, 0, 0], [0, 1, 0], [0, -1, 0], [0, 0, 1], [0, 0, -1]]; }
  else if (sides === 12) {
    for (s = 0; s < 8; s++) out.push([s & 1 ? 1 : -1, s & 2 ? 1 : -1, s & 4 ? 1 : -1]);
    for (t = 0; t < 4; t++) { var a = t & 1 ? 1 : -1, b = t & 2 ? 1 : -1; out.push([0, a * I, b * P], [a * I, b * P, 0], [a * P, 0, b * I]); }
  } else if (sides === 20) {
    for (t = 0; t < 4; t++) { var c = t & 1 ? 1 : -1, d = t & 2 ? 1 : -1; out.push([0, c, d * P], [c, d * P, 0], [c * P, 0, d]); }
  } else if (sides === 10) {
    // Pentagonal trapezohedron: a zigzag ring of ten and two apexes, apex height chosen so each kite is flat.
    var k = 0.13, h = k * (1 + Math.cos(Math.PI / 5)) / (1 - Math.cos(Math.PI / 5));
    for (s = 0; s < 10; s++) { var g = s * Math.PI / 5; out.push([Math.cos(g), s % 2 ? -k : k, Math.sin(g)]); }
    out.push([0, h, 0], [0, -h, 0]);
  }
  var pts = out.map(v3), r = 0;
  pts.forEach(function (p) { r = Math.max(r, len(p)); });
  // A d4 is all point and little body: a quarter larger, so it sits on the page as big as the others.
  if (sides === 4) r /= 1.25;
  return pts.map(function (p) { return mul(p, 1 / r); });
}

function hull(v) {
  var faces = [], seen = {}, n = v.length, EPS = 1e-5;
  for (var i = 0; i < n; i++) for (var j = i + 1; j < n; j++) for (var k = j + 1; k < n; k++) {
    var nn = cross(sub(v[j], v[i]), sub(v[k], v[i]));
    if (len(nn) < 1e-9) continue;
    nn = norm(nn);
    var d = dot(nn, v[i]), pos = 0, neg = 0, on = [];
    for (var m = 0; m < n; m++) { var sd = dot(nn, v[m]) - d; if (sd > EPS) pos++; else if (sd < -EPS) neg++; else on.push(m); }
    if (pos && neg) continue;
    if (pos) nn = mul(nn, -1);
    var key = on.slice().sort(function (a, b) { return a - b; }).join(',');
    if (seen[key]) continue;
    seen[key] = 1;
    var c = { x: 0, y: 0, z: 0 };
    on.forEach(function (q) { c = add(c, v[q]); });
    c = mul(c, 1 / on.length);
    var u = norm(sub(v[on[0]], c)), w = cross(nn, u);
    on.sort(function (a, b) {
      var pa = sub(v[a], c), pb = sub(v[b], c);
      return Math.atan2(dot(pa, w), dot(pa, u)) - Math.atan2(dot(pb, w), dot(pb, u));
    });
    faces.push({ idx: on, n: nn, c: c });
  }
  return faces;
}

// Opposite faces add up the way real dice do: N + 1, so 1 sits under 20.
function numberFaces(faces, sides) {
  var used = [], val = 1;
  faces.forEach(function (f) { f.value = 0; });
  faces.forEach(function (f, i) {
    if (used[i]) return;
    var best = -1, bd = 2;
    faces.forEach(function (g, j) { if (j !== i && !used[j]) { var dd = dot(f.n, g.n); if (dd < bd) { bd = dd; best = j; } } });
    used[i] = used[best] = 1;
    f.value = val; faces[best].value = sides + 1 - val; val++;
  });
}

var SOLIDS = {};
function solid(sides) {
  if (SOLIDS[sides]) return SOLIDS[sides];
  // The percentile die is a d10 whose faces read 00 to 90 in tens.
  if (sides === 100) {
    var ten = solid(10);
    SOLIDS[100] = { sides: 100, v: ten.v, faces: ten.faces.map(function (f) { var v = f.value % 10 * 10; return Object.assign({}, f, { value: v, label: v ? String(v) : '00' }); }) };
    return SOLIDS[100];
  }
  var v = solidVerts(sides), f = hull(v);
  // A d4 is read at its top point, so each face carries the number of the corner it does not touch:
  // the face it rests on names the corner pointing up.
  if (sides === 4) f.forEach(function (face) { for (var q = 0; q < 4; q++) if (face.idx.indexOf(q) < 0) face.value = q + 1; });
  else numberFaces(f, sides);
  f.forEach(function (face) {
    // Upright for the number: towards the face's first vertex (a d20's point, a d10's apex).
    var up = norm(sub(v[face.idx[0]], face.c));
    if (sides === 6) up = norm(sub(mul(add(v[face.idx[0]], v[face.idx[1]]), 0.5), face.c));
    face.up = up; face.right = cross(up, face.n);
    var R = 0, inr = 9;
    face.idx.forEach(function (q, qi) {
      R = Math.max(R, len(sub(v[q], face.c)));
      var a = v[q], b = v[face.idx[(qi + 1) % face.idx.length]], e = norm(sub(b, a));
      var toC = sub(face.c, a), perp = sub(toC, mul(e, dot(toC, e)));
      inr = Math.min(inr, len(perp));
    });
    face.R = R; face.inr = inr;
  });
  SOLIDS[sides] = { sides: sides, v: v, faces: f };
  return SOLIDS[sides];
}

// ---------- Faces: rune numerals as carved strokes ----------
// Each digit is a few straight strokes on a 0..1 box, like letters cut with a chisel.
var DIGITS = {
  '0': [[[.5, 0], [.95, .28], [.95, .72], [.5, 1], [.05, .72], [.05, .28], [.5, 0]]],
  '1': [[[.18, .26], [.58, 0], [.58, 1]], [[.3, 1], [.86, 1]]],
  '2': [[[.05, .24], [.5, 0], [.95, .24], [.95, .42], [.05, 1], [.95, 1]]],
  '3': [[[.06, .04], [.92, 0], [.4, .44], [.95, .66], [.62, 1], [.05, .92]]],
  '4': [[[.72, 1], [.72, 0], [.04, .68], [.98, .68]]],
  '5': [[[.94, 0], [.14, 0], [.06, .44], [.62, .36], [.95, .66], [.58, 1], [.05, .9]]],
  '6': [[[.86, 0], [.24, .32], [.05, .7], [.48, 1], [.95, .76], [.62, .44], [.1, .6]]],
  '7': [[[.04, 0], [.96, 0], [.34, 1]], [[.4, .52], [.88, .52]]],
  '8': [[[.5, 0], [.92, .24], [.5, .48], [.08, .24], [.5, 0]], [[.5, .48], [.96, .74], [.5, 1], [.04, .74], [.5, .48]]],
  '9': [[[.9, .4], [.38, .56], [.05, .3], [.5, 0], [.95, .28], [.78, 1]]]
};
// Rune-like border marks. They are drawn in the style of runes and spell nothing.
var MARKS = [
  [[[0, 0], [0, 1]], [[0, .18], [.5, 0]], [[0, .44], [.5, .26]]],
  [[[0, 1], [0, 0], [.5, .36], [.5, 1]]],
  [[[0, 0], [0, 1]], [[0, .26], [.45, .5], [0, .74]]],
  [[[.45, 0], [0, .5], [.45, 1]]],
  [[[0, 0], [.5, 1]], [[.5, 0], [0, 1]]],
  [[[0, 0], [0, 1]], [[.5, 0], [.5, 1]], [[0, .34], [.5, .62]]],
  [[[0, 1], [0, 0], [.5, .25], [0, .5]]],
  [[[.25, 0], [.25, 1]], [[0, .36], [.5, .64]]],
  [[[.25, 0], [.25, 1]]],
  [[[0, 0], [0, 1], [.5, .7]], [[0, .3], [.5, 0]]]
];

function polyLen(pl) { var L = 0; for (var i = 1; i < pl.length; i++) L += Math.hypot(pl[i][0] - pl[i - 1][0], pl[i][1] - pl[i - 1][1]); return L; }

// Lays out the label's strokes in canvas pixels, centred on (cx, cy).
function strokesFor(text, cx, cy, h) {
  var cw = 0.62 * h, gap = 0.18 * h, total = text.length * cw + (text.length - 1) * gap, x0 = cx - total / 2, y0 = cy - h / 2, out = [];
  for (var i = 0; i < text.length; i++) {
    (DIGITS[text[i]] || []).forEach(function (pl) {
      out.push(pl.map(function (p) { return [x0 + i * (cw + gap) + p[0] * cw, y0 + p[1] * h]; }));
    });
  }
  return out;
}

function hexA(hex, a) {
  var n = parseInt(hex.slice(1), 16);
  return 'rgba(' + (n >> 16 & 255) + ',' + (n >> 8 & 255) + ',' + (n & 255) + ',' + a + ')';
}
function mix(h1, h2, t) {
  var a = parseInt(h1.slice(1), 16), b = parseInt(h2.slice(1), 16);
  var r = Math.round((a >> 16 & 255) * (1 - t) + (b >> 16 & 255) * t), g = Math.round((a >> 8 & 255) * (1 - t) + (b >> 8 & 255) * t), bl = Math.round((a & 255) * (1 - t) + (b & 255) * t);
  return '#' + ((1 << 24) + (r << 16) + (g << 8) + bl).toString(16).slice(1);
}
function strokePaths(g, paths) { paths.forEach(function (pl) { g.beginPath(); g.moveTo(pl[0][0], pl[0][1]); for (var i = 1; i < pl.length; i++) g.lineTo(pl[i][0], pl[i][1]); g.stroke(); }); }


function mulberry(a) { return function () { a |= 0; a = a + 0x6D2B79F5 | 0; var t = Math.imul(a ^ a >>> 15, 1 | a); t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296; }; }
function canvas(w, h) { var c = document.createElement('canvas'); c.width = w; c.height = h || w; return c; }

// ---------- The bevel: rounded rims between faces, split so every face is a piece of its own ----------
// Each face is inset by BEV; the strip between two inset faces is a quadratic fillet, and the gap at each
// corner is a small cap. The strip and cap triangles belong to whichever face they sit nearer, so the die
// can come apart face by face (Unsealed, Eldritch grasp) and still be whole when it rests.
var BEVELS = {};
function bevel(sd, bev) {
  bev = bev || BEV;
  var bk = sd.sides + '|' + bev;
  if (BEVELS[bk]) return BEVELS[bk];
  var v = sd.v, F = sd.faces, ins = [], rim = F.map(function () { return { p: [], n: [] }; });
  F.forEach(function (f) {
    var m = f.idx.length;
    ins.push(f.idx.map(function (q, k) {
      var a = v[q], e1 = norm(sub(v[f.idx[(k + m - 1) % m]], a)), e2 = norm(sub(v[f.idx[(k + 1) % m]], a));
      var bis = norm(add(e1, e2)), sh = len(cross(e1, bis));
      return add(a, mul(bis, bev / Math.max(0.2, sh)));
    }));
  });
  function cornerOf(fi, q) { return ins[fi][F[fi].idx.indexOf(q)]; }
  function tri(owner, a, b, c, na, nb, nc) {
    var nn = cross(sub(b, a), sub(c, a)), avg = add(add(na, nb), nc);
    var R = rim[owner];
    if (dot(nn, avg) < 0) { var t = b; b = c; c = t; var tn = nb; nb = nc; nc = tn; }
    R.p.push(a, b, c); R.n.push(na, nb, nc);
  }
  function bez(a, c, b, t) { var u = 1 - t; return add(add(mul(a, u * u), mul(c, 2 * u * t)), mul(b, t * t)); }
  function nlerp(a, b, t) { return norm(add(mul(a, 1 - t), mul(b, t))); }
  // The edge's control point: the point on the original edge nearest the two inset corners' midpoint.
  function ctrl(a, b, pa, pb) { var e = norm(sub(b, a)), mid = mul(add(pa, pb), 0.5); return add(a, mul(e, dot(sub(mid, a), e))); }
  var TS = [0, 0.25, 0.5, 0.75, 1], edges = {}, curves = {};
  F.forEach(function (f, fi) { f.idx.forEach(function (a, k) { var b = f.idx[(k + 1) % f.idx.length], key = Math.min(a, b) + '-' + Math.max(a, b); (edges[key] = edges[key] || []).push({ fi: fi, a: a, b: b }); }); });
  Object.keys(edges).forEach(function (key) {
    var E = edges[key]; if (E.length !== 2) return;
    var fa = E[0].fi, fb = E[1].fi, a = E[0].a, b = E[0].b, nA = F[fa].n, nB = F[fb].n;
    var pa0 = cornerOf(fa, a), pb0 = cornerOf(fa, b), pa1 = cornerOf(fb, a), pb1 = cornerOf(fb, b);
    var ca = ctrl(v[a], v[b], pa0, pa1), cb = ctrl(v[a], v[b], pb0, pb1);
    var rowA = TS.map(function (t) { return bez(pa0, ca, pa1, t); }), rowB = TS.map(function (t) { return bez(pb0, cb, pb1, t); });
    for (var i = 0; i < 4; i++) {
      var tm = (TS[i] + TS[i + 1]) / 2, own = tm < 0.5 ? fa : fb, n0 = nlerp(nA, nB, TS[i]), n1 = nlerp(nA, nB, TS[i + 1]);
      tri(own, rowA[i], rowB[i], rowB[i + 1], n0, n0, n1);
      tri(own, rowA[i], rowB[i + 1], rowA[i + 1], n0, n1, n1);
    }
    // The strip's two ends, kept for the corner caps: points from face fa's corner to face fb's.
    (curves[a] = curves[a] || []).push({ f0: fa, f1: fb, pts: rowA, nA: nA, nB: nB });
    (curves[b] = curves[b] || []).push({ f0: fa, f1: fb, pts: rowB, nA: nA, nB: nB });
  });
  Object.keys(curves).forEach(function (q) {
    var list = curves[q].slice(), vq = v[q], chain = [], cur = list.shift(), start = cur.f0, guard = 0;
    chain.push(cur);
    while (list.length && guard++ < 20) {
      var last = chain[chain.length - 1], to = last.flip ? last.f0 : last.f1, i;
      for (i = 0; i < list.length; i++) if (list[i].f0 === to || list[i].f1 === to) break;
      if (i === list.length) break;
      var nx = list.splice(i, 1)[0]; nx.flip = nx.f1 === to; chain.push(nx);
    }
    var ring = [], cen = { x: 0, y: 0, z: 0 };
    chain.forEach(function (c) {
      for (var k = 0; k < 4; k++) {
        var t0 = c.flip ? 1 - TS[k] : TS[k], t1 = c.flip ? 1 - TS[k + 1] : TS[k + 1], tm = (t0 + t1) / 2;
        ring.push({ a: c.pts[c.flip ? 4 - k : k], b: c.pts[c.flip ? 3 - k : k + 1], na: nlerp(c.nA, c.nB, t0), nb: nlerp(c.nA, c.nB, t1), own: tm < 0.5 ? c.f0 : c.f1 });
      }
    });
    ring.forEach(function (s) { cen = add(cen, s.a); });
    cen = mul(cen, 1 / ring.length);
    var tip = add(cen, mul(sub(vq, cen), 0.45)), nt = norm(vq);
    ring.forEach(function (s) { tri(s.own, tip, s.a, s.b, nt, s.na, s.nb); });
  });
  BEVELS[bk] = { ins: ins, rim: rim };
  return BEVELS[bk];
}

// ---------- Face textures ----------
// Per face: colour (the stone, its veins, the inlaid number), a roughness/metal map (the number is metal
// on the jewelled looks), and a normal map so the engraving catches the light as the die turns.
function faceLayout(sd, face, ins, label, cv, style, bev) {
  var s = cv * 0.47 / face.R, cx = cv / 2, cy = cv / 2;
  function px(p) { var d = sub(p, face.c); return [cx + dot(d, face.right) * s, cy - dot(d, face.up) * s]; }
  var poly = face.idx.map(function (q) { return px(sd.v[q]); }), inner = ins.map(px);
  if (sd.sides === 4) return faceLayoutD4(sd, face, poly, inner, cv, style, s, px);
  var r = (face.inr - Math.min(bev || BEV, 0.1) * 0.8) * s;
  var h = label.length > 1 ? 0.78 * r : 0.96 * r;
  if (style === 'classic') h *= 1.05;
  var ny = cy + (sd.sides === 20 ? r * 0.05 : 0);
  return { s: s, cv: cv, cx: cx, cy: ny, poly: poly, inner: inner, r: r, h: h, label: label, style: style, strokes: strokesFor(label, cx, ny, h), px: px };
}
// A d4 face: three numbers, one in each corner, each standing with its top toward its corner (the
// number of the corner a d4 is read by). lay.sub holds each one as a layout of its own, so a show can
// light just the number at the top point; lay.strokes holds all three for painting.
function faceLayoutD4(sd, face, poly, inner, cv, style, s, px) {
  var cx = cv / 2, cy = cv / 2, h = face.R * s * 0.36, all = [], sub = [];
  // Seen from straight above, a d4's faces slope steeply away, which squashes each number along the
  // slope to about a third: drawn half again as tall that way, they read close to their true shape.
  var tall = 1.5;
  face.idx.forEach(function (q, k) {
    var vx = poly[k][0] - cx, vy = poly[k][1] - cy, rot = Math.atan2(vx, -vy);
    var nx = cx + vx * 0.5, ny = cy + vy * 0.5, c = Math.cos(rot), sn = Math.sin(rot), label = String(q + 1);
    var strokes = strokesFor(label, 0, 0, h).map(function (pl) { return pl.map(function (p) { var y = p[1] * tall; return [nx + p[0] * c - y * sn, ny + p[0] * sn + y * c]; }); });
    all = all.concat(strokes);
    sub.push({ s: s, cv: cv, cx: nx, cy: ny, poly: poly, inner: inner, r: h * 0.6, h: h, label: label, style: style, strokes: strokes, px: px, rot: rot, corner: q });
  });
  return { s: s, cv: cv, cx: cx, cy: cy, poly: poly, inner: inner, r: face.inr * s * 0.9, h: h, label: String(face.value), style: style, strokes: all, px: px, sub: sub };
}
function polyPath(g, poly) { g.beginPath(); poly.forEach(function (p, k) { if (k) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.closePath(); }
function numeralFont(lay) { return '700 ' + Math.round(lay.h * 1.08) + 'px Cinzel, Georgia, "Times New Roman", serif'; }

// Paints the numeral into a context in one flat style; colour, metal and height maps each call it.
function paintNumeral(g, lay, style, w) {
  if (style === 'classic' && lay.sub) { lay.sub.forEach(function (sl) { paintNumeral(g, sl, style, w); }); return; }
  if (style === 'classic' && lay.rot) {
    g.save(); g.translate(lay.cx, lay.cy); g.rotate(lay.rot); g.translate(-lay.cx, -lay.cy);
    paintNumeral(g, Object.assign({}, lay, { rot: 0 }), style, w); g.restore(); return;
  }
  if (style === 'classic') {
    g.textAlign = 'center'; g.textBaseline = 'middle'; g.font = numeralFont(lay);
    g.fillText(lay.label, lay.cx, lay.cy + lay.h * 0.04);
    if (lay.label === '6' || lay.label === '9') g.fillRect(lay.cx - lay.h * 0.24, lay.cy + lay.h * 0.6, lay.h * 0.48, Math.max(2, lay.h * 0.07));
    return;
  }
  g.lineJoin = 'miter'; g.miterLimit = 3; g.lineCap = 'butt'; g.lineWidth = w;
  strokePaths(g, lay.strokes);
  if (lay.label === '6' || lay.label === '9') { g.beginPath(); g.arc(lay.cx, lay.cy + lay.h * 0.68, w * 0.62, 0, 7); g.fill(); }
}
function strokeW(lay) { return Math.max(2, lay.h * 0.13) * (lay.wk || 1); }

function paintStone(g, cv, th, rnd) {
  var b = th.base, grd = g.createRadialGradient(cv * 0.38, cv * 0.32, cv * 0.05, cv / 2, cv / 2, cv * 0.75);
  grd.addColorStop(0, b.grad[0]); grd.addColorStop(1, b.grad[1]);
  g.fillStyle = grd; g.fillRect(0, 0, cv, cv);
  var k = cv / 256, i;
  if (b.mottle) {
    for (i = 0; i < b.mottle.n; i++) {
      var x = rnd() * cv, y = rnd() * cv, r = (10 + rnd() * 40) * k, gr = g.createRadialGradient(x, y, 0, x, y, r);
      gr.addColorStop(0, hexA(b.mottle.color, b.mottle.a * (0.4 + rnd() * 0.6))); gr.addColorStop(1, hexA(b.mottle.color, 0));
      g.fillStyle = gr; g.fillRect(x - r, y - r, r * 2, r * 2);
    }
  }
  if (b.streaks) {
    g.strokeStyle = hexA(b.streaks.color, b.streaks.a); g.lineWidth = 0.7 * k;
    for (i = 0; i < b.streaks.n; i++) { var yy = rnd() * cv; g.beginPath(); g.moveTo(0, yy); g.bezierCurveTo(cv * 0.3, yy + (rnd() - 0.5) * 20 * k, cv * 0.7, yy + (rnd() - 0.5) * 20 * k, cv, yy + (rnd() - 0.5) * 10 * k); g.stroke(); }
  }
  if (b.veins) {
    g.save(); try { g.filter = 'blur(' + (0.7 * k) + 'px)'; } catch (e) {}
    for (i = 0; i < b.veins.n; i++) {
      var px = rnd() * cv, py = rnd() * cv, ang = rnd() * 6.28, wd = b.veins.w * k * (0.5 + rnd());
      g.strokeStyle = hexA(b.veins.color, b.veins.a * (0.5 + rnd() * 0.5)); g.lineWidth = wd; g.lineCap = 'round';
      g.beginPath(); g.moveTo(px, py);
      for (var s = 0; s < 9; s++) { ang += (rnd() - 0.5) * 0.9; px += Math.cos(ang) * 22 * k; py += Math.sin(ang) * 22 * k; g.lineTo(px, py); }
      g.stroke();
      if (rnd() > 0.5) { g.lineWidth = wd * 0.4; g.beginPath(); g.moveTo(px, py); g.lineTo(px + (rnd() - 0.5) * 60 * k, py + (rnd() - 0.5) * 60 * k); g.stroke(); }
    }
    g.restore();
  }
  if (b.flecks) {
    for (i = 0; i < b.flecks.n; i++) { g.fillStyle = hexA(b.flecks.color, 0.5 + rnd() * 0.5); var fs = (0.6 + rnd() * 1.6) * k; g.fillRect(rnd() * cv, rnd() * cv, fs, fs); }
  }
  if (b.grain) {
    for (i = 0; i < 420; i++) { g.fillStyle = rnd() > 0.5 ? 'rgba(255,255,255,' + rnd() * 0.06 + ')' : 'rgba(0,0,0,' + rnd() * 0.08 + ')'; g.fillRect(rnd() * cv, rnd() * cv, (1 + rnd() * 2) * k, (1 + rnd() * 2) * k); }
  }
}

// A jewelled face: the stone, a soft shadow just inside the rim, then the number inlaid in metal.
function drawColour(g, lay, th, seed) {
  var cv = lay.cv, rnd = mulberry(seed), w = strokeW(lay), acc = accentOf(th);
  paintStone(g, cv, th, rnd);
  g.save(); polyPath(g, lay.inner); g.clip();
  g.strokeStyle = 'rgba(0,0,0,.28)'; g.lineWidth = cv * 0.07; try { g.filter = 'blur(' + (cv * 0.02) + 'px)'; } catch (e) {}
  polyPath(g, lay.inner); g.stroke(); g.restore();
  // The cut the metal sits in shows as a thin dark line, then the metal itself.
  g.fillStyle = 'rgba(0,0,0,.55)'; g.strokeStyle = 'rgba(0,0,0,.55)';
  paintNumeral(g, lay, lay.style, w * 1.28);
  g.fillStyle = acc; g.strokeStyle = acc;
  paintNumeral(g, lay, lay.style, w);
}
function heightToNormal(src, strength) {
  var W = src.width, H = src.height, sd = src.getContext('2d').getImageData(0, 0, W, H).data;
  var out = canvas(W, H), og = out.getContext('2d'), od = og.createImageData(W, H), d = od.data;
  function h(x, y) { x = x < 0 ? 0 : x >= W ? W - 1 : x; y = y < 0 ? 0 : y >= H ? H - 1 : y; return sd[(y * W + x) * 4] / 255; }
  for (var y = 0; y < H; y++) for (var x = 0; x < W; x++) {
    var dx = (h(x + 1, y) - h(x - 1, y)) * strength, dy = (h(x, y + 1) - h(x, y - 1)) * strength;
    var nx = -dx, ny = dy, nz = 1, l = Math.sqrt(nx * nx + ny * ny + 1), o = (y * W + x) * 4;
    d[o] = (nx / l * 0.5 + 0.5) * 255; d[o + 1] = (ny / l * 0.5 + 0.5) * 255; d[o + 2] = (nz / l * 0.5 + 0.5) * 255; d[o + 3] = 255;
  }
  og.putImageData(od, 0, 0);
  return out;
}
function faceOf(die, value) { for (var i = 0; i < die.faces.length; i++) if (die.faces[i].value === value) return i; return 0; }
// Puts every piece back where it belongs, after an effect has moved them.
function reassemble(die) { die.faces.forEach(function (fc) { fc.piece.position.copy(fc.c); fc.piece.quaternion.identity(); fc.piece.scale.setScalar(1); fc.piece.visible = true; }); }

// ---------- The table: a see-through canvas over the page ----------
// What the device offers, read once. Outside a browser (the unit tests import this module) there is
// no WebGL.
var QUALITY = (function () {
  var nav = typeof navigator !== 'undefined' ? navigator : {}, gl = false;
  var low = (nav.hardwareConcurrency || 4) <= 2 || (nav.deviceMemory || 8) <= 2;
  try { if (typeof document !== 'undefined') { var c = document.createElement('canvas'); gl = !!(c.getContext('webgl2') || c.getContext('webgl')); } } catch (e) { gl = false; }
  return { gl: gl, low: low };
})();
// Objects every table reuses (face images, common shapes and materials), listed as they are made so
// that they can be let go of every renderer that drew them (disposeStage).
var SHARED = new Set();
function share(o) { o.userData.shared = true; SHARED.add(o); return o; }
var STAGES = new Set();
var SHADOWTEX = null;
function shadowTex(T) {
  if (SHADOWTEX) return SHADOWTEX;
  var c = canvas(128), g = c.getContext('2d'), gr = g.createRadialGradient(64, 64, 0, 64, 64, 64);
  gr.addColorStop(0, 'rgba(0,0,0,1)'); gr.addColorStop(0.45, 'rgba(0,0,0,.55)'); gr.addColorStop(1, 'rgba(0,0,0,0)');
  g.fillStyle = gr; g.fillRect(0, 0, 128, 128);
  SHADOWTEX = new T.CanvasTexture(c); share(SHADOWTEX);
  return SHADOWTEX;
}
// The soft dark patch right under a die, which the shadow map is too coarse to give. It tightens as
// the die comes down, which is most of what makes a die look like it's touching the table.
function addContact(st, die) {
  var T = st.T, m = new T.Mesh(new T.PlaneGeometry(1, 1), new T.MeshBasicMaterial({ map: shadowTex(T), color: 0x000000, transparent: true, opacity: 0.5, depthWrite: false }));
  m.rotation.x = -Math.PI / 2; m.renderOrder = -1; st.scene.add(m);
  st.contact.push({ m: m, die: die });
}

// ---------- The throw: simulated ahead, then replayed ----------
function simulate(C, st, dice, from, rank) {
  var best = null;
  for (var attempt = 0; attempt < 5; attempt++) {
    var world = new C.World({ gravity: new C.Vec3(0, -42, 0) }); world.allowSleep = true;
    var dm = new C.Material('die'), wm = new C.Material('wall');
    world.addContactMaterial(new C.ContactMaterial(dm, wm, { friction: 0.32, restitution: 0.42 }));
    world.addContactMaterial(new C.ContactMaterial(dm, dm, { friction: 0.2, restitution: 0.5 }));
    var inset = Math.min(1.1, Math.min(st.hw, st.hh) * 0.18);
    function wall(p, ax, ang) { var b = new C.Body({ mass: 0, material: wm, shape: new C.Plane() }); b.position.set(p[0], p[1], p[2]); b.quaternion.setFromAxisAngle(new C.Vec3(ax[0], ax[1], ax[2]), ang); world.addBody(b); }
    wall([0, 0, 0], [1, 0, 0], -Math.PI / 2);
    wall([-st.hw + inset, 0, 0], [0, 1, 0], Math.PI / 2); wall([st.hw - inset, 0, 0], [0, 1, 0], -Math.PI / 2);
    wall([0, 0, -st.hh + inset + (st.padTop || 0) / PX], [0, 1, 0], 0); wall([0, 0, st.hh - inset], [0, 1, 0], Math.PI);
    var rnd = Math.random, bodies = [];
    dice.forEach(function (d, i) {
      var b = new C.Body({ mass: 1, material: dm, shape: d.shape, sleepSpeedLimit: 0.25, sleepTimeLimit: 0.35, linearDamping: 0.12, angularDamping: 0.14 });
      var sx, sz, vx, vz, sp = Math.min(st.hw, st.hh) * (1.8 + rnd() * 0.7), lim = d.size * 1.3;
      if (from) {
        sx = from.x + (i % 3 - 1) * d.size * 1.7; sz = from.z + Math.floor(i / 3) * d.size * 1.7;
        var dx = -sx * (0.7 + rnd() * 0.5) + (rnd() - 0.5) * st.hw * 0.6, dz = -sz * (0.7 + rnd() * 0.5) + (rnd() - 0.5) * st.hh * 0.6, dl = Math.hypot(dx, dz) || 1;
        vx = dx / dl * sp; vz = dz / dl * sp;
      } else {
        sx = -st.hw + inset + lim + (i % 4) * d.size * 1.6; sz = st.hh - inset - lim - Math.floor(i / 4) * d.size * 1.8;
        vx = sp * (0.9 + rnd() * 0.4); vz = -sp * (0.55 + rnd() * 0.4);
      }
      sx = Math.max(-st.hw + inset + lim, Math.min(st.hw - inset - lim, sx));
      sz = Math.max(-st.hh + inset + (st.padTop || 0) / PX + lim, Math.min(st.hh - inset - lim, sz));
      b.position.set(sx, d.size * 2.2 + rnd() * d.size * 1.5, sz);
      b.quaternion.setFromEuler(rnd() * 6.28, rnd() * 6.28, rnd() * 6.28);
      b.velocity.set(vx, -2, vz);
      b.angularVelocity.set((rnd() - 0.5) * 30, (rnd() - 0.5) * 30, (rnd() - 0.5) * 30);
      world.addBody(b); bodies.push(b);
    });
    var MAX = 840, frames = dice.map(function () { return new Float32Array(MAX * 7); }), steps = 0;
    for (; steps < MAX; steps++) {
      world.step(STEP);
      bodies.forEach(function (b, i) { var o = steps * 7, f = frames[i]; f[o] = b.position.x; f[o + 1] = b.position.y; f[o + 2] = b.position.z; f[o + 3] = b.quaternion.x; f[o + 4] = b.quaternion.y; f[o + 5] = b.quaternion.z; f[o + 6] = b.quaternion.w; });
      if (steps > 50 && bodies.every(function (b) { return b.sleepState === C.Body.SLEEPING; })) { steps++; break; }
    }
    var landed = bodies.map(function (b, i) { return landedFace(dice[i], b.quaternion); });
    var worst = Math.min.apply(null, landed.map(function (l) { return l.dot; }));
    // A throw still rocking when time runs out ranks below any that settled.
    var settled = steps < MAX, res = { frames: frames, steps: Math.min(steps, MAX), landed: landed, worst: worst, score: worst - (settled ? 0 : 1) };
    // A caller may rank throws further (a crit's die should land where its effect has room); 0 is ideal.
    var extra = rank ? rank(res) : 0; res.score += extra;
    if (!best || res.score > best.score) best = res;
    if (settled && worst > 0.97 && extra > -0.05) break;
  }
  return best;
}
function landedFace(die, q) {
  // The world's up, seen from inside the die: rotate (0,1,0) by the inverse quaternion.
  var r = { x: -q.x, y: -q.y, z: -q.z }, u = { x: 0, y: 1, z: 0 }, t = mul(cross(r, u), 2);
  var up = add(add(u, mul(t, q.w)), cross(r, t));
  var best = -2, at = 0, k = die.sides === 4 ? -1 : 1;
  // A d4 has no top face: the face it rests on, pointing down, is the one that names the roll.
  die.faces.forEach(function (fc, i) { var d = dot(fc.f.n, up) * k; if (d > best) { best = d; at = i; } });
  return { face: at, dot: best };
}
// One of the die's own rotations that carries face `from` onto face `to`. Where several do, take the
// one that leaves the number most upright on screen (screen up is -z), so the roll reads at a glance.
function symmetry(T, sd, from, to, qSim) {
  var fa = sd.faces[from], fb = sd.faces[to], best = null, bestScore = -9;
  function basis(f, vi) {
    var t = norm(sub(sd.v[vi], f.c)), z = cross(t, f.n);
    return new T.Matrix4().makeBasis(new T.Vector3(t.x, t.y, t.z), new T.Vector3(f.n.x, f.n.y, f.n.z), new T.Vector3(z.x, z.y, z.z));
  }
  var Ainv = basis(fa, fa.idx[0]).transpose(), tmp = new T.Vector3(), upA = new T.Vector3(fa.up.x, fa.up.y, fa.up.z);
  for (var k = 0; k < fb.idx.length; k++) {
    var R = basis(fb, fb.idx[k]).multiply(Ainv), ok = true;
    for (var i = 0; i < sd.v.length && ok; i++) {
      tmp.set(sd.v[i].x, sd.v[i].y, sd.v[i].z).applyMatrix4(R);
      ok = sd.v.some(function (p) { return Math.abs(p.x - tmp.x) + Math.abs(p.y - tmp.y) + Math.abs(p.z - tmp.z) < 1e-3; });
    }
    if (!ok) continue;
    var q = new T.Quaternion().setFromRotationMatrix(R);
    var score = qSim ? -upA.clone().applyQuaternion(q).applyQuaternion(qSim).z : 0;
    if (score > bestScore) { bestScore = score; best = q; }
  }
  return best || new T.Quaternion();
}
// ---------- Effects ----------
// Each effect is { update(dt) -> still running }. Effects only start once the dice have settled.
function out3(t) { t = Math.max(0, Math.min(1, t)); return 1 - Math.pow(1 - t, 3); }
function in3(t) { t = Math.max(0, Math.min(1, t)); return t * t * t; }
function inout(t) { t = Math.max(0, Math.min(1, t)); return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2; }

function clamp01(t) { return Math.max(0, Math.min(1, t)); }
function smooth(a, b, t) { t = clamp01((t - a) / (b - a)); return t * t * (3 - 2 * t); }
// 0 before a, up over a..b, holds, down over c..d.
function env4(t, a, b, c, d) { return t < b ? smooth(a, b, t) : t < c ? 1 : 1 - smooth(c, d, t); }

var SPRITE = null;
function spriteTex(T) {
  if (SPRITE) return SPRITE;
  var c = canvas(64), g = c.getContext('2d'), gr = g.createRadialGradient(32, 32, 0, 32, 32, 32);
  gr.addColorStop(0, 'rgba(255,255,255,1)'); gr.addColorStop(0.35, 'rgba(255,255,255,.6)'); gr.addColorStop(1, 'rgba(255,255,255,0)');
  g.fillStyle = gr; g.fillRect(0, 0, 64, 64);
  SPRITE = new T.CanvasTexture(c); share(SPRITE);
  return SPRITE;
}
// A lens star: a hot point with long thin rays, the shape cameras give a bright light.
var STAR = null;
function starTex(T) {
  if (STAR) return STAR;
  var c = canvas(256), g = c.getContext('2d');
  g.translate(128, 128); g.globalCompositeOperation = 'lighter';
  [[0, 1, 4], [Math.PI / 2, 1, 4], [Math.PI / 4, 0.55, 2.4], [-Math.PI / 4, 0.55, 2.4]].forEach(function (r) {
    g.save(); g.rotate(r[0]);
    var gr = g.createLinearGradient(-128, 0, 128, 0);
    gr.addColorStop(0, 'rgba(255,255,255,0)'); gr.addColorStop(0.5, 'rgba(255,255,255,' + r[1] + ')'); gr.addColorStop(1, 'rgba(255,255,255,0)');
    g.fillStyle = gr; g.fillRect(-128, -r[2] / 2, 256, r[2]); g.restore();
  });
  var rg = g.createRadialGradient(0, 0, 0, 0, 0, 60); rg.addColorStop(0, 'rgba(255,255,255,1)'); rg.addColorStop(0.2, 'rgba(255,255,255,.5)'); rg.addColorStop(1, 'rgba(255,255,255,0)');
  g.fillStyle = rg; g.fillRect(-60, -60, 120, 120);
  STAR = new T.CanvasTexture(c); share(STAR);
  return STAR;
}
function smokeTex(T) {
  var c = canvas(64), g = c.getContext('2d'), rnd = mulberry(5);
  for (var i = 0; i < 9; i++) {
    var x = 20 + rnd() * 24, y = 20 + rnd() * 24, r = 10 + rnd() * 14, gr = g.createRadialGradient(x, y, 0, x, y, r);
    gr.addColorStop(0, 'rgba(255,255,255,.35)'); gr.addColorStop(1, 'rgba(255,255,255,0)'); g.fillStyle = gr; g.fillRect(0, 0, 64, 64);
  }
  var t = new T.CanvasTexture(c); share(t); return t;
}
var SMOKE = null;
function rgb(T, hex) { return new T.Color(hex); }
// Particles. o.blend 'add' glows; 'normal' is for smoke and ash, which must darken.
function spray(st, o) {
  var T = st.T, n = QUALITY.low ? Math.ceil(o.n / 2) : o.n, rnd = Math.random;
  var pos = new Float32Array(n * 3), col = new Float32Array(n * 4), vel = new Float32Array(n * 3), age = new Float32Array(n), life = new Float32Array(n);
  var colors = (o.colors || [o.color]).map(function (h) { return rgb(T, h); });
  for (var i = 0; i < n; i++) {
    var s = o.spawn(i, rnd);
    pos.set(s.p, i * 3); vel.set(s.v, i * 3);
    age[i] = -(s.delay || 0); life[i] = o.life[0] + rnd() * (o.life[1] - o.life[0]);
    var c = colors[i % colors.length]; col[i * 4] = c.r; col[i * 4 + 1] = c.g; col[i * 4 + 2] = c.b; col[i * 4 + 3] = 0;
  }
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.BufferAttribute(pos, 3));
  geo.setAttribute('color', new T.BufferAttribute(col, 4));
  if (o.smoke && !SMOKE) SMOKE = smokeTex(T);
  var mat = new T.PointsMaterial({ size: st.ptSize(o.size), map: o.smoke ? SMOKE : spriteTex(T), transparent: true, depthWrite: false, vertexColors: true, sizeAttenuation: true,
    blending: o.blend === 'normal' ? T.NormalBlending : T.AdditiveBlending, toneMapped: false });
  var pts = new T.Points(geo, mat); pts.frustumCulled = false; st.scene.add(pts);
  var g = o.g || 0, drag = o.drag || 0, pull = o.pull;
  return { update: function (dt) {
    var alive = false;
    for (var i = 0; i < n; i++) {
      age[i] += dt;
      if (age[i] < 0) { col[i * 4 + 3] = 0; alive = true; continue; }
      var k = age[i] / life[i];
      if (k >= 1) { col[i * 4 + 3] = 0; continue; }
      alive = true;
      var j = i * 3;
      vel[j + 1] -= g * dt;
      if (pull) { vel[j] += (pull.x - pos[j]) * pull.k * dt; vel[j + 1] += (pull.y - pos[j + 1]) * pull.k * dt; vel[j + 2] += (pull.z - pos[j + 2]) * pull.k * dt; }
      if (o.swirl) { var dx = pos[j] - o.swirl.x, dz = pos[j + 2] - o.swirl.z; vel[j] += -dz * o.swirl.k * dt; vel[j + 2] += dx * o.swirl.k * dt; }
      var dd = Math.max(0, 1 - drag * dt); vel[j] *= dd; vel[j + 1] *= dd; vel[j + 2] *= dd;
      pos[j] += vel[j] * dt; pos[j + 1] += vel[j + 1] * dt; pos[j + 2] += vel[j + 2] * dt;
      if (!o.noFloor && pos[j + 1] < 0.02) { pos[j + 1] = 0.02; vel[j + 1] *= -0.25; vel[j] *= 0.6; vel[j + 2] *= 0.6; }
      col[i * 4 + 3] = (o.alpha || 1) * (k < 0.15 ? k / 0.15 : 1 - (k - 0.15) / 0.85);
    }
    geo.attributes.position.needsUpdate = true; geo.attributes.color.needsUpdate = true;
    if (!alive) { st.scene.remove(pts); geo.dispose(); mat.dispose(); }
    return alive;
  } };
}
function worldOf(die, local) { die.group.updateMatrixWorld(true); return local.clone().applyMatrix4(die.group.matrixWorld); }
// A shockwave: a soft band that is brightest at its leading edge and fades inward, never a flat hoop.
var WAVETEX = null;
function waveTex(T) {
  if (WAVETEX) return WAVETEX;
  var c = canvas(256, 256), g = c.getContext('2d'), gr = g.createRadialGradient(128, 128, 0, 128, 128, 128);
  gr.addColorStop(0, 'rgba(255,255,255,0)'); gr.addColorStop(0.6, 'rgba(255,255,255,0)'); gr.addColorStop(0.9, 'rgba(255,255,255,0.55)');
  gr.addColorStop(0.97, 'rgba(255,255,255,1)'); gr.addColorStop(1, 'rgba(255,255,255,0)');
  g.fillStyle = gr; g.fillRect(0, 0, 256, 256);
  WAVETEX = new T.CanvasTexture(c); WAVETEX.colorSpace = T.SRGBColorSpace; share(WAVETEX); return WAVETEX;
}
function wave(st, at, color, from, to, dur, dark) {
  var T = st.T, m = new T.Mesh(new T.PlaneGeometry(2, 2), new T.MeshBasicMaterial({ map: waveTex(T), color: color, transparent: true, opacity: 0.85, depthWrite: false, side: T.DoubleSide, blending: dark ? T.NormalBlending : T.AdditiveBlending, toneMapped: false }));
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.03, at.z); st.scene.add(m);
  var t = 0;
  return { update: function (dt) { t += dt; var k = Math.min(1, t / dur), s = from + (to - from) * out3(k); m.scale.set(s, s, s); m.material.opacity = (dark ? 0.6 : 0.9) * (1 - k) * (1 - k); if (k >= 1) { st.scene.remove(m); m.geometry.dispose(); m.material.dispose(); return false; } return true; } };
}
// A billboard of light. Additive, so it only ever brightens what is under it.
function glow(st, tex, color, size) {
  var T = st.T, sp = new T.Sprite(new T.SpriteMaterial({ map: tex, color: color, transparent: true, opacity: 0, depthWrite: false, blending: T.AdditiveBlending, toneMapped: false }));
  sp.scale.set(size, size, 1); return sp;
}
// Darkens the table round the die while magic plays, the way a scene dims for a spell on screen.
// Without it, light effects vanish on a white page.
function dimmer(st, at, radius) {
  var T = st.T, m = new T.Mesh(new T.PlaneGeometry(radius * 2, radius * 2), new T.MeshBasicMaterial({ map: shadowTex(T), color: 0x05040a, transparent: true, opacity: 0, depthWrite: false }));
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.004, at.z); m.renderOrder = -2; st.scene.add(m);
  return m;
}
function drop(st, list) { list.forEach(function (o) { if (!o) return; if (o.parent) o.parent.remove(o); o.traverse(function (x) { if (x.geometry) x.geometry.dispose(); if (x.material) { if (x.material.map && !x.material.map.userData.shared) x.material.map.dispose(); x.material.dispose(); } }); }); }

// The burn, drawn into the glow map. p runs 0..1 along the strokes like a fuse.
function drawBurn(g, lay, p, color, calm) {
  var cv = lay.cv;
  g.fillStyle = '#000'; g.fillRect(0, 0, cv, cv);
  if (p <= 0) return;
  var w = strokeW(lay);
  if (lay.style === 'classic' || calm) {
    g.globalAlpha = Math.min(1, p);
    if (lay.style === 'classic') {
      g.shadowColor = color; g.shadowBlur = lay.h * 0.16; g.fillStyle = color; paintNumeral(g, lay, 'classic', w);
      g.shadowBlur = 0; g.fillStyle = mix(color, '#ffffff', 0.55); paintNumeral(g, lay, 'classic', w);
    } else glowStrokes(g, lay.strokes, w, color);
    g.globalAlpha = 1; return;
  }
  var lens = lay.strokes.map(polyLen), total = lens.reduce(function (a, b) { return a + b; }, 0), done = total * p, acc = 0, lit = [];
  lay.strokes.forEach(function (pl, i) {
    var L = lens[i], take = Math.max(0, Math.min(L, done - acc)); acc += L;
    if (take <= 0) return;
    var out = [pl[0]], run = 0;
    for (var k = 1; k < pl.length; k++) {
      var sl = Math.hypot(pl[k][0] - pl[k - 1][0], pl[k][1] - pl[k - 1][1]);
      if (run + sl <= take) { out.push(pl[k]); run += sl; continue; }
      var f = (take - run) / sl; out.push([pl[k - 1][0] + (pl[k][0] - pl[k - 1][0]) * f, pl[k - 1][1] + (pl[k][1] - pl[k - 1][1]) * f]); break;
    }
    lit.push(out);
  });
  glowStrokes(g, lit, w, color);
  if (p < 1 && lit.length) { var last = lit[lit.length - 1], tip = last[last.length - 1]; g.fillStyle = '#fff'; g.shadowColor = color; g.shadowBlur = w * 3; g.beginPath(); g.arc(tip[0], tip[1], w * 0.7, 0, 7); g.fill(); g.shadowBlur = 0; }
}
function glowStrokes(g, paths, w, color) {
  g.lineJoin = 'miter'; g.miterLimit = 3; g.lineCap = 'round';
  // A tight halo: a wide one spreads over the whole face and washes the stone out.
  g.shadowColor = color; g.shadowBlur = w * 1.5; g.strokeStyle = hexA(color, 0.6); g.lineWidth = w * 1.35; strokePaths(g, paths);
  g.strokeStyle = color; g.shadowBlur = w * 0.6; g.lineWidth = w * 1.1; strokePaths(g, paths);
  g.shadowBlur = 0; g.strokeStyle = mix(color, '#ffffff', 0.6); g.lineWidth = w * 0.45; strokePaths(g, paths);
}

// ---------- Cinematic extras ----------
// A light from the magic itself. It lights the die and its pieces from inside; it casts no shadow,
// because on a see-through table every shadow it threw would be a dark wedge across the page.
function castLight(st, light) { return light; }
// A light that follows env(t) for dur seconds, then removes itself.
function fxLight(st, pos, color, dur, env, range) {
  var T = st.T, l = castLight(st, new T.PointLight(color, 0, range, 1.4)), t = 0;
  l.position.copy(pos); st.scene.add(l);
  return { light: l, update: function (dt) { t += dt; l.intensity = Math.max(0, env(t)); if (t >= dur) { st.scene.remove(l); if (l.shadow && l.shadow.map) l.shadow.map.dispose(); return false; } return true; } };
}

// Draws a face's number as light into g, without clearing it.
function numberOnto(g, lay, color) {
  if (lay.style === 'classic') { g.shadowColor = color; g.shadowBlur = lay.h * 0.16; g.fillStyle = color; paintNumeral(g, lay, 'classic'); g.shadowBlur = 0; g.fillStyle = mix(color, '#ffffff', 0.55); paintNumeral(g, lay, 'classic'); }
  else glowStrokes(g, lay.strokes, strokeW(lay), color);
}
function cutPaths(paths, p) {
  return paths.map(function (pl) {
    var L = polyLen(pl), take = L * p, out = [pl[0]], run = 0;
    for (var k = 1; k < pl.length; k++) {
      var sl = Math.hypot(pl[k][0] - pl[k - 1][0], pl[k][1] - pl[k - 1][1]);
      if (run + sl <= take) { out.push(pl[k]); run += sl; continue; }
      var f = (take - run) / sl; out.push([pl[k - 1][0] + (pl[k][0] - pl[k - 1][0]) * f, pl[k - 1][1] + (pl[k][1] - pl[k - 1][1]) * f]); break;
    }
    return out;
  }).filter(function (pl) { return pl.length > 1; });
}
function cracksFor(fc, rnd) {
  var poly = fc.lay.poly, out = [], cx = fc.lay.cv / 2, cy = cx, j = fc.lay.cv / 256 * 26;
  for (var k = 0; k < 3; k++) {
    var e = Math.floor(rnd() * poly.length), a = poly[e], b = poly[(e + 1) % poly.length], u = 0.2 + rnd() * 0.6;
    var x = a[0] + (b[0] - a[0]) * u, y = a[1] + (b[1] - a[1]) * u, pl = [[x, y]], steps = 5;
    for (var s2 = 1; s2 <= steps; s2++) { var f = s2 / steps * (0.55 + rnd() * 0.3); pl.push([x + (cx - x) * f + (rnd() - 0.5) * j, y + (cy - y) * f + (rnd() - 0.5) * j]); }
    out.push(pl);
    var m = pl[2]; out.push([m, [m[0] + (rnd() - 0.5) * j * 2.3, m[1] + (rnd() - 0.5) * j * 2.3]]);
  }
  return out;
}
// Waving control points from a to b: a sideways S-curve that travels, and a lift in the middle.
function writhe(T, a, b, n, amp, lift, t, ph) {
  var out = [], dir = new T.Vector3().subVectors(b, a), L = dir.length() || 1, side = new T.Vector3(-dir.z, 0, dir.x).normalize(), up = new T.Vector3(0, 1, 0);
  for (var i = 0; i < n; i++) {
    var u = i / (n - 1), w = Math.sin(u * Math.PI * 1.6 - t * 3.2 + ph) * amp * L * Math.sin(u * Math.PI);
    out.push(new T.Vector3().copy(a).lerp(b, u).addScaledVector(side, w).addScaledVector(up, Math.sin(u * Math.PI) * lift));
  }
  return out;
}

// ==================== Rune dice ====================
// Twelve sets of dice, each with its own colours and its own way of showing off the number that lands.
// Jewelled, Confetti glass and Acrylic are built here; the other sets register themselves with
// registerSet. Any crit animation plays on any die.

// Colours for the confetti glass: the glass's tint, the flakes inside, and the ink of the numbers.
var GLASS = [
  { key: 'clear', name: 'Clear prism', note: 'Water-clear, rainbow flakes, ink numbers', tint: '#eef4ff', alpha: 0.14, flakes: ['#ff4f9a', '#ffd23f', '#3fd7ff', '#7cff6b', '#b98cff', '#ffffff'], ink: '#191c2a', keyline: 'rgba(255,255,255,.95)', burn: '#bfe8ff' },
  { key: 'smoke', name: 'Smoke and gold', note: 'Smoked glass, gold leaf', tint: '#3a3634', alpha: 0.42, flakes: ['#f3c45a', '#ffe08a', '#c98f2e'], ink: '#f3d27a', burn: '#ffcf6b' },
  { key: 'sea', name: 'Sea glass', note: 'Pale teal, silver and blue', tint: '#7fd6c6', alpha: 0.26, flakes: ['#e9f2f7', '#9fd8ff', '#3b8fe0', '#c7fff1'], ink: '#ffffff', burn: '#a8fff0' },
  { key: 'rose', name: 'Rose quartz', note: 'Blush pink, gold and white', tint: '#ffb3c7', alpha: 0.28, flakes: ['#ffd36b', '#ffffff', '#ff8fb0'], ink: '#ffffff', burn: '#ffc1d6' },
  { key: 'midnight', name: 'Midnight', note: 'Deep blue, silver stars', tint: '#1d2d7a', alpha: 0.5, flakes: ['#e8eef9', '#b9c8ff', '#ffffff'], ink: '#e8eef9', burn: '#9fc0ff' }
];
// Colours for the acrylic: the body, a pearly swirl through it, and the paint in the numbers.
var ACRYLIC = [
  { key: 'oxblood', name: 'Oxblood and cream', note: 'Deep wine red, cream numbers', base: ['#7a1322', '#3a050d'], swirl: '#c2414f', ink: '#f4e8cf', pop: '#ffd27a', burn: '#ffd27a' },
  { key: 'bone', name: 'Bone and black', note: 'Warm bone, black numbers', base: ['#f3ecdc', '#d8ccb2'], swirl: '#ffffff', ink: '#16130f', pop: '#c8102e', burn: '#ff5a3a' },
  { key: 'forest', name: 'Forest pearl', note: 'Pearly green, gold numbers', base: ['#2f7a55', '#123a28'], swirl: '#9be3bf', ink: '#f0c75e', pop: '#fff1b8', burn: '#ffe08a' },
  { key: 'royal', name: 'Royal violet', note: 'Pearly violet, white numbers', base: ['#5b2fb0', '#22104f'], swirl: '#c7a6ff', ink: '#ffffff', pop: '#ffd36b', burn: '#e3c4ff' },
  { key: 'ink', name: 'Ink and gold', note: 'Gloss black, gold numbers', base: ['#24252b', '#050506'], swirl: '#4a4d58', ink: '#e9c15a', pop: '#fff0b0', burn: '#ffd36b' },
  { key: 'coral', name: 'Coral', note: 'Warm coral, white numbers', base: ['#ff7a5c', '#c93c2a'], swirl: '#ffc2a8', ink: '#ffffff', pop: '#2a1a5e', burn: '#fff1e0' }
];
var KINDS = {
  jewelled: { key: 'jewelled', name: 'Jewelled', note: 'Polished stone inlaid with metal, a cut gem at every corner', highlight: 'Molten', show: 'molten',
    highlightWhat: 'The metal in the number runs molten, white-hot, then cools to a bright gleam.', colours: THEMES.filter(function (t) { return t.group === 'jewel'; }), bev: 0.075 },
  confetti: { key: 'confetti', name: 'Confetti glass', note: 'Clear glass with flakes of foil suspended inside', highlight: 'Gather', show: 'gather', see: 'glass',
    highlightWhat: 'The flakes inside swirl up and gather into the number, just under its face.', colours: GLASS, bev: 0.06 },
  acrylic: { key: 'acrylic', name: 'Acrylic', note: 'Deep, glossy acrylic with a pearl swirl, crisp painted numbers', highlight: 'Enamel', show: 'enamel',
    highlightWhat: 'The die gives a little hop and the number rises out of the face in glossy enamel.', colours: ACRYLIC, bev: 0.13 }
};
function kindOf(k) { return KINDS[k] || KINDS.jewelled; }
// The sets built in their own files. A set is a kind of die like the three above, plus the hooks that
// draw and build it (see registerSet's notes); the shared code asks only for what it needs.
var SETS = {};
//   key, name, note, show (a SHOWS key), bev, colours: [{ key, name, note, burn, ink, base: { grad: [light, dark], rough } }]
//   paint(g, lay, c, seed, sd)   the face's colour image, number included (alpha is read when see is set)
//   surface(g, lay, c)           optional: roughness in green, metalness in blue, red left at 0
//   height(g, lay, c)            optional: white is high
//   relief                       optional: how strongly the height map bends the light (1 is the default)
//   rim(c)                       { col, rough, metal } for the rounded edges
//   see                          null, 'glass' (blended, far side drawn) or 'cutout' (holes, alpha below 0.5)
//   dense                        a glass set dark enough to cast a solid die's shadow, not a tinted glow
//   physical(c, q, back)         optional extra MeshPhysicalMaterial settings
//   shader(s, c)                 optional extra onBeforeCompile work on the body (this is the material)
//   build(T, die, q)             optional: { parts: [meshes in the die's space], tick(dt, spin), dispose() }
//   cover(c)                     optional: how much a show's light covers the face (light faces need some)
function registerSet(def) {
  var sh = SHOWS[def.show] || {};
  def.highlight = def.highlight || sh.name || def.show; def.highlightWhat = def.highlightWhat || sh.what || '';
  SETS[def.key] = def; KINDS[def.key] = def;
}
// Every colour object answers the questions the shared code asks of it (base.grad, burn, key).
function coloursOf(kind, key) {
  var list = kindOf(kind).colours, c = list[0];
  for (var i = 0; i < list.length; i++) if (list[i].key === key) c = list[i];
  if (kind === 'confetti' && !c.base) c.base = { grad: [mix(c.tint, '#ffffff', 0.3), c.tint], rough: 0.05, coat: 1 };
  if (kind === 'acrylic' && !c.base.grad) c.base = { grad: c.base, rough: 0.2, coat: 1 };
  if (SETS[kind] && SETS[kind].prep && !c._prep) { SETS[kind].prep(c); c._prep = true; }
  return c;
}

// ---------- The room the dice reflect ----------
// A small photo studio: dark floor, pale ceiling, four soft boxes, none straight overhead (the camera
// looks straight down, so a box overhead would sit as glare on every top face, right on the number).
function studioScene(T) {
  var sc = new T.Scene(), c = canvas(4, 256), g = c.getContext('2d'), gr = g.createLinearGradient(0, 0, 0, 256);
  gr.addColorStop(0, '#4a4c52'); gr.addColorStop(0.42, '#bdbcb8'); gr.addColorStop(0.5, '#6d6e73'); gr.addColorStop(0.56, '#2a2b30'); gr.addColorStop(1, '#141518');
  g.fillStyle = gr; g.fillRect(0, 0, 4, 256);
  var t = new T.CanvasTexture(c); t.colorSpace = T.SRGBColorSpace;
  sc.add(new T.Mesh(new T.SphereGeometry(50, 32, 16), new T.MeshBasicMaterial({ map: t, side: T.BackSide })));
  [[-30, 24, -20, 22, 2.8], [28, 20, 18, 14, 3], [0, 18, -38, 20, 2.5], [-18, 10, 30, 10, 1.6]].forEach(function (b) {
    var m = new T.Mesh(new T.PlaneGeometry(b[3], b[3] * 0.6), new T.MeshBasicMaterial({ color: new T.Color(b[4], b[4], b[4] * 0.97), side: T.DoubleSide }));
    m.position.set(b[0], b[1], b[2]); m.lookAt(0, 0, 0); sc.add(m);
  });
  return sc;
}
// One environment for the whole page: the blurred version lights the dice, a sharp cube is what the
// gems see inside themselves.
var ENV = null;
function environment(T, renderer) {
  if (ENV && ENV.renderer === renderer) return ENV;
  var sc = studioScene(T), pm = new T.PMREMGenerator(renderer), pmrem = pm.fromScene(sc, 0.03).texture;
  var rt = new T.WebGLCubeRenderTarget(128, { type: T.HalfFloatType }), cc = new T.CubeCamera(0.1, 100, rt);
  cc.update(renderer, sc);
  pm.dispose(); sc.traverse(function (o) { if (o.geometry) o.geometry.dispose(); if (o.material) { if (o.material.map) o.material.map.dispose(); o.material.dispose(); } });
  ENV = { renderer: renderer, pmrem: pmrem, cube: rt.texture, rt: rt };
  return ENV;
}

// ---------- Face drawings ----------
// Numbers are drawn large and with a dark keyline round them, because a player reads them from
// straight above at arm's length, often against glare.
function layout(sd, f, ins, label, cv, style, bev) {
  var lay = faceLayout(sd, f, ins, label, cv, style, bev);
  // A d4's corner numbers are sized by their own layout, which leaves no room to grow them.
  if (lay.sub) { lay.wk = 1.2; lay.sub.forEach(function (sl) { sl.wk = 1.2; }); return lay; }
  var k = sd.sides === 20 ? 1.12 : sd.sides === 12 || sd.sides === 10 || sd.sides === 100 ? 1.06 : 1.0;
  // Strokes a fifth heavier than the base layout's: thin metal lines are the hardest part to read from above.
  lay.h *= k; lay.wk = 1.2; lay.strokes = strokesFor(label, lay.cx, lay.cy, lay.h);
  return lay;
}
// Acrylic: a deep gloss body with a slow pearly swirl, the number cut in and filled with paint.
function drawAcrylic(g, lay, c, seed) {
  var cv = lay.cv, rnd = mulberry(seed), w = strokeW(lay) * 1.08, k = cv / 256;
  var gr = g.createRadialGradient(cv * 0.36, cv * 0.3, cv * 0.04, cv / 2, cv / 2, cv * 0.78);
  gr.addColorStop(0, c.base.grad[0]); gr.addColorStop(1, c.base.grad[1]); g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
  g.save(); try { g.filter = 'blur(' + (6 * k) + 'px)'; } catch (e) {}
  for (var i = 0; i < 9; i++) {
    g.strokeStyle = hexA(c.swirl, 0.07 + rnd() * 0.12); g.lineWidth = (8 + rnd() * 26) * k; g.lineCap = 'round';
    var x = rnd() * cv, y = rnd() * cv; g.beginPath(); g.moveTo(x, y);
    g.bezierCurveTo(rnd() * cv, rnd() * cv, rnd() * cv, rnd() * cv, rnd() * cv, rnd() * cv); g.stroke();
  }
  g.restore();
  // The cut: a dark lip on the far side of every stroke reads as depth from above.
  g.strokeStyle = g.fillStyle = 'rgba(0,0,0,.55)'; g.save(); g.translate(0, w * 0.18); paintNumeral(g, lay, lay.style, w * 1.32); g.restore();
  g.strokeStyle = g.fillStyle = c.ink; paintNumeral(g, lay, lay.style, w);
}
// Confetti glass: mostly clear (alpha low), tinted; the number painted solid on the surface.
function drawGlass(g, lay, c) {
  var cv = lay.cv, w = strokeW(lay) * 1.08;
  g.clearRect(0, 0, cv, cv);
  // Glass has almost no colour of its own to reflect: its tint shows as a darkening filter, not a
  // pale veil, which is what made clear glass read as frosted.
  g.fillStyle = mix(c.tint, '#000000', 0.5); g.globalAlpha = 1; g.fillRect(0, 0, cv, cv);
  // Alpha comes from a second pass: clear body, opaque number, a slightly frostier rim of the face.
  var a = g.getImageData(0, 0, cv, cv), mk = canvas(cv), mg = mk.getContext('2d');
  mg.fillStyle = '#000'; mg.fillRect(0, 0, cv, cv);
  mg.strokeStyle = 'rgba(255,255,255,.35)'; mg.lineWidth = cv * 0.05; polyPath(mg, lay.inner); mg.stroke();
  mg.fillStyle = mg.strokeStyle = '#fff'; paintNumeral(mg, lay, lay.style, w * 1.75);
  var m = mg.getImageData(0, 0, cv, cv).data, base = Math.round(c.alpha * 255);
  for (var i = 0; i < a.data.length; i += 4) a.data[i + 3] = Math.max(base, m[i]);
  g.putImageData(a, 0, 0);
  // Number: a dark keyline then the ink, so it reads over whatever is behind the glass.
  g.strokeStyle = g.fillStyle = c.keyline || 'rgba(10,10,14,.9)'; paintNumeral(g, lay, lay.style, w * 1.7);
  g.strokeStyle = g.fillStyle = c.ink; paintNumeral(g, lay, lay.style, w);
}
// Roughness (green) and metalness (blue), per kind.
function drawSurface(g, lay, c, half, kind) {
  var cv = lay.cv * half, w = strokeW(lay), set = SETS[kind];
  if (set) {
    g.fillStyle = 'rgb(0,' + Math.round(((c.base && c.base.rough) || 0.4) * 255) + ',0)'; g.fillRect(0, 0, cv, cv);
    if (set.surface) { g.save(); g.scale(half, half); set.surface(g, lay, c); g.restore(); }
    return;
  }
  var body = kind === 'acrylic' ? [0.2, 0] : kind === 'confetti' ? [0.05, 0] : [Math.max(0.34, c.base.rough), 0];
  var num = kind === 'acrylic' ? [0.55, 0] : kind === 'confetti' ? [0.5, 0] : [Math.max(0.3, accentRough(c) + 0.1), 210];
  g.fillStyle = 'rgb(0,' + Math.round(body[0] * 255) + ',' + body[1] + ')'; g.fillRect(0, 0, cv, cv);
  g.save(); g.scale(half, half);
  var s = 'rgb(0,' + Math.round(num[0] * 255) + ',' + num[1] + ')'; g.fillStyle = g.strokeStyle = s;
  paintNumeral(g, lay, lay.style, w * (kind === 'jewelled' ? 1 : 1.08));
  if (kind === 'jewelled') { g.restore(); return; }
  g.restore();
}
function drawHeight(g, lay, half, kind, c) {
  var cv = lay.cv * half, w = strokeW(lay), set = SETS[kind];
  if (set) {
    g.fillStyle = '#fff'; g.fillRect(0, 0, cv, cv);
    if (set.height) { g.save(); g.scale(half, half); set.height(g, lay, c); g.restore(); }
    return;
  }
  g.fillStyle = '#fff'; g.fillRect(0, 0, cv, cv);
  g.save(); g.scale(half, half); try { g.filter = 'blur(' + (0.9 / half) + 'px)'; } catch (e) {}
  g.fillStyle = g.strokeStyle = kind === 'jewelled' ? '#5a5a5a' : '#8a8a8a';
  paintNumeral(g, lay, lay.style, w * 1.15); g.restore();
}

// The rim (and a jewelled die's bezels): one material per kind. Red in the surface map marks it,
// so a tint can light the rims without lighting the faces.
var RIMPATCH = 0.06;
function rimLook(kind, c) {
  if (SETS[kind]) return SETS[kind].rim(c);
  // The metal rims a little rougher than the numbers: a polished rod would carry a long white streak.
  if (kind === 'jewelled') return { col: accentOf(c), rough: Math.max(0.4, accentRough(c) + 0.18), metal: 1 };
  if (kind === 'confetti') return { col: hexA(mix(c.tint, '#000000', 0.5), Math.max(0.24, c.alpha + 0.08)), rough: 0.04, metal: 0 };
  return { col: mix(c.base.grad[0], c.base.grad[1], 0.4), rough: 0.16, metal: 0 };
}
function rimUV(lay) { var L = lay.cell, h = lay.cv * RIMPATCH / 2; return [(L.x + h) / L.W, 1 - (L.y + h) / L.H]; }

// One atlas per kind of die: every face drawn into one image, so a die is a single draw, and every
// identical die on the table shares the same images. Only the most recent are kept: one d20's set is
// several megabytes of canvas.
var ATLAS = {}, ATLAS_KEPT = [], ATLAS_MAX = 24;
// A die already on the table keeps the images it holds; the cache just stops handing them out.
function dropAtlas(key) {
  var a = ATLAS[key]; if (!a) return;
  [a.map, a.srf, a.nrm].forEach(function (t) { SHARED.delete(t); });
  delete ATLAS[key]; ATLAS_KEPT.splice(ATLAS_KEPT.indexOf(key), 1);
}
function atlas(T, sd, kind, c, style, cv, bev) {
  var key = [sd.sides, kind, c.key, style, cv].join('|');
  if (ATLAS[key]) { ATLAS_KEPT.splice(ATLAS_KEPT.indexOf(key), 1); ATLAS_KEPT.push(key); return ATLAS[key]; }
  var n = sd.faces.length, cols = Math.ceil(Math.sqrt(n)), rows = Math.ceil(n / cols), half = 0.5;
  var bv = bevel(sd, bev), W = cols * cv, H = rows * cv;
  var col = canvas(W, H), srf = canvas(W * half, H * half), hgt = canvas(W * half, H * half);
  var cg = col.getContext('2d'), sg = srf.getContext('2d'), hg = hgt.getContext('2d'), lays = [];
  var tmp = canvas(cv), tg = tmp.getContext('2d'), ts = canvas(cv * half), tsg = ts.getContext('2d'), th = canvas(cv * half), thg = th.getContext('2d');
  sd.faces.forEach(function (f, fi) {
    var lay = layout(sd, f, bv.ins[fi], f.label || String(f.value), cv, style, bev), x = (fi % cols) * cv, y = Math.floor(fi / cols) * cv;
    lay.cell = { x: x, y: y, W: W, H: H }; if (lay.sub) lay.sub.forEach(function (sl) { sl.cell = lay.cell; });
    tg.clearRect(0, 0, cv, cv);
    if (SETS[kind]) { tg.save(); SETS[kind].paint(tg, lay, c, sd.sides * 53 + fi * 11 + 5, sd); tg.restore(); }
    else if (kind === 'jewelled') drawColour(tg, lay, c, sd.sides * 97 + fi * 13 + 7);
    else if (kind === 'acrylic') drawAcrylic(tg, lay, c, sd.sides * 31 + fi * 7);
    else drawGlass(tg, lay, c);
    // The cell's corner, outside the face, is the rim's colour: the rim samples one point here.
    var P = Math.ceil(cv * RIMPATCH), rm = rimLook(kind, c);
    tg.clearRect(0, 0, P, P); tg.fillStyle = rm.col; tg.fillRect(0, 0, P, P);
    cg.drawImage(tmp, x, y);
    tsg.clearRect(0, 0, cv * half, cv * half); drawSurface(tsg, lay, c, half, kind);
    tsg.fillStyle = 'rgb(255,' + Math.round(rm.rough * 255) + ',' + Math.round(rm.metal * 255) + ')'; tsg.fillRect(0, 0, Math.ceil(P * half), Math.ceil(P * half));
    sg.drawImage(ts, x * half, y * half);
    thg.clearRect(0, 0, cv * half, cv * half); drawHeight(thg, lay, half, kind, c); hg.drawImage(th, x * half, y * half);
    lays.push(lay);
  });
  function tex(cn, srgb) { var t = new T.CanvasTexture(cn); if (srgb) t.colorSpace = T.SRGBColorSpace; t.anisotropy = 8; share(t); return t; }
  var nrm = heightToNormal(hgt, 3.0 * cv / 256 * ((SETS[kind] && SETS[kind].relief) || 1));
  ATLAS[key] = { map: tex(col, true), srf: tex(srf), nrm: tex(nrm), lays: lays, bv: bv, cols: cols, rows: rows, W: W, H: H };
  ATLAS_KEPT.push(key); if (ATLAS_KEPT.length > ATLAS_MAX) dropAtlas(ATLAS_KEPT[0]);
  return ATLAS[key];
}

// ---------- Building a die ----------
// A whole die is a handful of draws: one body mesh (every face, its rim and a jewelled die's bezels,
// all textured from the shared atlas), its gems as one instanced mesh, and on the glass die the flakes
// and the far side of the glass. The per-face pieces that effects pull apart are built only when an
// effect first splits a die, so a table of a dozen dice stays cheap.

// The body of one kind of die at one size, shared by every die like it: one buffer, and the range of
// it each face owns (its face, its share of the rim, its bezels), so a face can be cut out as a piece.
var BODY = {};
function bodyGeo(T, sd, atl, size, kind) {
  var key = [sd.sides, kind, size, atl.W].join('|');
  if (BODY[key]) return BODY[key];
  var bv = atl.bv, pos = [], nor = [], uv = [], ranges = [], gems = [], owner = {};
  var tor = kind === 'jewelled' ? new T.TorusGeometry(1.02, 0.16, 6, 16).toNonIndexed() : null;
  var Y = new T.Vector3(0, 1, 0), X = new T.Vector3(1, 0, 0), v = new T.Vector3(), nm = new T.Matrix3();
  sd.faces.forEach(function (f, fi) {
    var lay = atl.lays[fi], cell = lay.cell, start = pos.length / 3, ins = bv.ins[fi], n = ins.length, ru = rimUV(lay);
    function P(p) { pos.push(p.x * size, p.y * size, p.z * size); }
    function UV(px) { uv.push((cell.x + px[0]) / cell.W, 1 - (cell.y + px[1]) / cell.H); }
    for (var k = 0; k < n; k++) {
      P(f.c); UV([lay.cv / 2, lay.cv / 2]); P(ins[k]); UV(lay.inner[k]); P(ins[(k + 1) % n]); UV(lay.inner[(k + 1) % n]);
      nor.push(f.n.x, f.n.y, f.n.z, f.n.x, f.n.y, f.n.z, f.n.x, f.n.y, f.n.z);
    }
    var R = bv.rim[fi];
    R.p.forEach(function (p, j) { P(p); nor.push(R.n[j].x, R.n[j].y, R.n[j].z); uv.push(ru[0], ru[1]); });
    if (tor) f.idx.forEach(function (q) {
      if (owner[q] != null) return; owner[q] = fi;
      // A gem set into the corner, its table facing straight out from the die's centre.
      var vq = sd.v[q], out = norm(vq), r = size * 0.105, at = new T.Vector3(vq.x, vq.y, vq.z).multiplyScalar(0.93 * size);
      var qq = new T.Quaternion().setFromUnitVectors(Y, new T.Vector3(out.x, out.y, out.z));
      gems.push({ fi: fi, m: new T.Matrix4().compose(at.clone().add(new T.Vector3(0, r * 0.05, 0).applyQuaternion(qq)), qq, new T.Vector3(r, r, r)) });
      var bm = new T.Matrix4().compose(at.clone().add(new T.Vector3(0, r * 0.04, 0).applyQuaternion(qq)), qq.clone().multiply(new T.Quaternion().setFromAxisAngle(X, Math.PI / 2)), new T.Vector3(r, r, r));
      nm.getNormalMatrix(bm);
      var tp = tor.attributes.position, tn = tor.attributes.normal;
      for (var i = 0; i < tp.count; i++) {
        v.fromBufferAttribute(tp, i).applyMatrix4(bm); pos.push(v.x, v.y, v.z);
        v.fromBufferAttribute(tn, i).applyMatrix3(nm).normalize(); nor.push(v.x, v.y, v.z); uv.push(ru[0], ru[1]);
      }
    });
    ranges.push([start, pos.length / 3 - start]);
  });
  if (tor) tor.dispose();
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3));
  geo.setAttribute('normal', new T.Float32BufferAttribute(nor, 3));
  geo.setAttribute('uv', new T.Float32BufferAttribute(uv, 2));
  geo.computeBoundingSphere(); share(geo);
  BODY[key] = { geo: geo, ranges: ranges, gems: gems, pieces: [], over: [], size: size };
  return BODY[key];
}
// One face's share of the body, moved so the face's centre is its origin.
function pieceGeo(T, body, sd, fi) {
  if (body.pieces[fi]) return body.pieces[fi];
  var r = body.ranges[fi], c = sd.faces[fi].c, s = body.size, A = body.geo.attributes;
  var p = A.position.array.slice(r[0] * 3, (r[0] + r[1]) * 3);
  for (var i = 0; i < p.length; i += 3) { p[i] -= c.x * s; p[i + 1] -= c.y * s; p[i + 2] -= c.z * s; }
  var g = new T.BufferGeometry();
  g.setAttribute('position', new T.BufferAttribute(p, 3));
  g.setAttribute('normal', new T.BufferAttribute(A.normal.array.slice(r[0] * 3, (r[0] + r[1]) * 3), 3));
  g.setAttribute('uv', new T.BufferAttribute(A.uv.array.slice(r[0] * 2, (r[0] + r[1]) * 2), 2));
  g.computeBoundingSphere(); share(g);
  body.pieces[fi] = g;
  return g;
}
// The glow layer over one face: the face's own polygon, mapped 0..1 onto that face's glow canvas.
function overlayGeo(T, body, sd, atl, fi) {
  if (body.over[fi]) return body.over[fi];
  var f = sd.faces[fi], ins = atl.bv.ins[fi], lay = atl.lays[fi], s = body.size, n = ins.length, pos = [], uv = [];
  function P(p) { pos.push((p.x - f.c.x) * s, (p.y - f.c.y) * s, (p.z - f.c.z) * s); }
  for (var k = 0; k < n; k++) {
    P(f.c); uv.push(0.5, 0.5);
    P(ins[k]); uv.push(lay.inner[k][0] / lay.cv, 1 - lay.inner[k][1] / lay.cv);
    P(ins[(k + 1) % n]); uv.push(lay.inner[(k + 1) % n][0] / lay.cv, 1 - lay.inner[(k + 1) % n][1] / lay.cv);
  }
  var g = new T.BufferGeometry();
  g.setAttribute('position', new T.Float32BufferAttribute(pos, 3));
  g.setAttribute('uv', new T.Float32BufferAttribute(uv, 2));
  share(g);
  body.over[fi] = g;
  return g;
}
// A brilliant cut: a flat table, a sloped crown and a deep pavilion, sixteen facets round.
var GEMGEO = null;
function gemGeometry(T) {
  if (GEMGEO) return GEMGEO;
  var pts = [[0, -0.86], [1, -0.03], [1, 0.03], [0.56, 0.34], [0, 0.34]].map(function (p) { return new T.Vector2(p[0], p[1]); });
  GEMGEO = new T.LatheGeometry(pts, 16).toNonIndexed(); GEMGEO.computeVertexNormals(); share(GEMGEO);
  return GEMGEO;
}
// Flakes of foil: small irregular five-sided chips.
var FLAKEGEO = null;
function flakeGeometry(T) {
  if (FLAKEGEO) return FLAKEGEO;
  FLAKEGEO = new T.CircleGeometry(1, 5); share(FLAKEGEO);
  return FLAKEGEO;
}
// Where the flakes sit inside a die: anywhere clear of the faces, each one owned by its nearest face.
var FLAKES = {};
function flakeLayout(T, sd, size, count, seed) {
  var key = [sd.sides, size, count, seed].join('|');
  if (FLAKES[key]) return FLAKES[key];
  var rnd = mulberry(seed), out = [], guard = 0, q = new T.Quaternion(), e = new T.Euler();
  while (out.length < count && guard++ < count * 40) {
    var p = { x: rnd() * 2 - 1, y: rnd() * 2 - 1, z: rnd() * 2 - 1 }, best = -9, own = 0, ok = true;
    for (var i = 0; i < sd.faces.length; i++) {
      var f = sd.faces[i], d = dot(p, f.n), lim = dot(f.c, f.n) - 0.14;
      if (d > lim) { ok = false; break; }
      if (d - lim > best) { best = d - lim; own = i; }
    }
    if (!ok) continue;
    var s = size * (0.035 + rnd() * 0.035);
    e.set(rnd() * 6.28, rnd() * 6.28, rnd() * 6.28); q.setFromEuler(e);
    out.push({ fi: own, m: new T.Matrix4().compose(new T.Vector3(p.x * size, p.y * size, p.z * size), q.clone(), new T.Vector3(s, s * (0.6 + rnd() * 0.6), s)), pick: rnd() });
  }
  FLAKES[key] = out;
  return out;
}

// ---------- Materials ----------
// The rims glow on their own when a die is tinted: the surface map's red channel marks rim, not face.
function rimGlowHook(s) {
  s.fragmentShader = s.fragmentShader.replace('#include <emissivemap_fragment>',
    '#ifdef USE_EMISSIVEMAP\n\ttotalEmissiveRadiance *= texture2D( emissiveMap, vEmissiveMapUv ).r;\n#endif');
}
// Glass: the body is nearly clear, but its reflections are not. Where the surface reflects (a soft
// box, the rounded rim, a grazing angle), the glass turns opaque with that reflection, as real glass
// does; elsewhere the page shows through, tinted.
function glassHook(s) {
  s.fragmentShader = s.fragmentShader.replace('#include <emissivemap_fragment>',
    '#ifdef USE_EMISSIVEMAP\n\ttotalEmissiveRadiance *= texture2D( emissiveMap, vEmissiveMapUv ).r;\n#endif')
    .replace('#include <opaque_fragment>',
    'float glassSpec = dot( max( outgoingLight - totalDiffuse, vec3( 0.0 ) ), vec3( 0.3, 0.59, 0.11 ) );\n' +
    'float glassFres = pow( 1.0 - clamp( dot( normal, geometryViewDir ), 0.0, 1.0 ), 3.0 );\n' +
    'diffuseColor.a = clamp( diffuseColor.a + min( glassSpec * 0.55, 0.6 ) + glassFres * 0.3, 0.0, 1.0 );\n' +
    '#include <opaque_fragment>');
}
function acrylicHook(s) {
  s.uniforms.uEdge = { value: this.userData.edge };
  s.fragmentShader = 'uniform vec3 uEdge;\n' + s.fragmentShader.replace('#include <emissivemap_fragment>',
    '#ifdef USE_EMISSIVEMAP\n\ttotalEmissiveRadiance *= texture2D( emissiveMap, vEmissiveMapUv ).r;\n#endif\n' +
    '\ttotalEmissiveRadiance += uEdge * pow( 1.0 - clamp( dot( normal, normalize( vViewPosition ) ), 0.0, 1.0 ), 2.5 );');
}
// A die's body: textured from the atlas; one material per die, so a tint or a shade stays on that die.
function bodyMat(T, kind, atl, c, q, back) {
  var hd = q.detail !== 'basic', o = { map: atl.map, roughnessMap: atl.srf, metalnessMap: atl.srf, normalMap: atl.nrm, roughness: 1, metalness: 1,
    emissive: new T.Color(c.burn), emissiveMap: atl.srf, emissiveIntensity: 0, envMapIntensity: 1 };
  var set = SETS[kind];
  if (set) return setMat(T, set, o, c, q, back, hd);
  if (kind === 'jewelled') Object.assign(o, { clearcoat: 0.45, clearcoatRoughness: 0.28 });
  else if (kind === 'acrylic') Object.assign(o, { clearcoat: 1, clearcoatRoughness: 0.12, normalScale: new T.Vector2(0.7, 0.7), specularIntensity: 0.7,
    iridescence: hd ? 0.25 : 0, iridescenceIOR: 1.3, iridescenceThicknessRange: [120, 420] });
  else Object.assign(o, { transparent: true, clearcoat: hd ? 0.4 : 0, clearcoatRoughness: 0.06, ior: 1.5, specularIntensity: 1, envMapIntensity: 1.05,
    side: back ? T.BackSide : T.FrontSide, depthWrite: !back });
  var m = new T.MeshPhysicalMaterial(o);
  m.onBeforeCompile = kind === 'confetti' ? glassHook : kind === 'acrylic' ? acrylicHook : rimGlowHook;
  // Acrylic lets a little light through near its edges: a glow of its own lighter colour there.
  if (kind === 'acrylic') m.userData.edge = new T.Color(mix(c.base.grad[0], '#ffffff', 0.25)).multiplyScalar(0.55);
  if (back) m.color.setScalar(0.55);
  return m;
}
// A set's body: the shared atlas maps, the see-through handling its kind asks for, then its own settings.
function setMat(T, set, o, c, q, back, hd) {
  if (set.see === 'glass') Object.assign(o, { transparent: true, ior: 1.5, specularIntensity: 1, side: back ? T.BackSide : T.FrontSide, depthWrite: !back });
  // Holes cut clean through: anything under half alpha is not drawn, and the inside of the frame shows.
  if (set.see === 'cutout') Object.assign(o, { alphaTest: 0.5, side: T.DoubleSide });
  if (set.physical) Object.assign(o, set.physical(c, q, back));
  var m = new T.MeshPhysicalMaterial(o), own = set.shader;
  m.onBeforeCompile = function (sh) { (set.see === 'glass' ? glassHook : rimGlowHook)(sh); if (own) own.call(m, sh, c); };
  m.customProgramCacheKey = function () { return 'set:' + set.key + (back ? ':back' : ''); };
  if (back) m.color.setScalar(0.55);
  return m;
}
// Flakes: thin foil, a mirror for whatever the room shows, with a film of colour that shifts as the
// die turns.
var FLAKEMAT = {};
function flakeMat(T, hd) {
  var k = hd ? 'hd' : 'basic';
  if (FLAKEMAT[k]) return FLAKEMAT[k];
  FLAKEMAT[k] = new T.MeshPhysicalMaterial({ color: 0xffffff, metalness: 1, roughness: 0.24, side: T.DoubleSide, envMapIntensity: 1.7,
    iridescence: hd ? 0.9 : 0, iridescenceIOR: 1.6, iridescenceThicknessRange: [220, 620] });
  share(FLAKEMAT[k]);
  return FLAKEMAT[k];
}

// ---------- Gems ----------
// A cut stone is not a coloured shiny surface: most of what it shows is light that went in through
// the crown, bounced off two facets of the pavilion and came back out, darkened by the colour it
// passed through. So the shader traces that path, against a sharp copy of the room: a thin
// reflection on top, a dark-and-bright mosaic of the facets inside, and a white spark where a facet
// catches the key light. HD splits the light slightly by colour (the stone's fire).
var GEM_VS = [
  'varying vec3 vW; varying vec3 vAxis; varying vec3 vCtr;',
  'void main() {',
  '  mat4 m = modelMatrix;',
  '  #ifdef USE_INSTANCING',
  '  m = m * instanceMatrix;',
  '  #endif',
  '  vec4 w = m * vec4( position, 1.0 );',
  '  vW = w.xyz; vAxis = normalize( ( m * vec4( 0.0, 1.0, 0.0, 0.0 ) ).xyz ); vCtr = ( m * vec4( 0.0, 0.0, 0.0, 1.0 ) ).xyz;',
  '  gl_Position = projectionMatrix * viewMatrix * w;',
  '}'
].join('\n');
var GEM_FS = [
  'uniform samplerCube uEnv; uniform vec3 uColor; uniform vec3 uKey; uniform float uDepth;',
  'varying vec3 vW; varying vec3 vAxis; varying vec3 vCtr;',
  'float gemHash( float n ) { return fract( sin( n ) * 43758.5453 ); }',
  'vec3 gemEnv( vec3 d ) { return textureCube( uEnv, d ).rgb; }',
  'vec3 gemInside( vec3 V, vec3 N, vec3 ax, vec3 b1, vec3 b2, float a0, float eta ) {',
  '  vec3 t = refract( -V, N, eta );',
  '  vec3 r1 = cos( a0 + 3.14159 ) * b1 + sin( a0 + 3.14159 ) * b2, r2 = cos( a0 ) * b1 + sin( a0 ) * b2;',
  '  t = reflect( t, normalize( -ax * 0.755 + r1 * 0.656 ) );',
  '  t = reflect( t, normalize( -ax * 0.755 + r2 * 0.656 ) );',
  '  float up = dot( t, ax ); if ( up < 0.0 ) t -= ax * up * 2.0;',
  '  return normalize( t );',
  '}',
  'void main() {',
  '  vec3 N = normalize( cross( dFdx( vW ), dFdy( vW ) ) ), V = normalize( cameraPosition - vW );',
  '  if ( dot( N, V ) < 0.0 ) N = -N;',
  '  float NV = clamp( dot( N, V ), 0.0, 1.0 ), F = 0.077 + 0.923 * pow( 1.0 - NV, 5.0 );',
  '  vec3 R = reflect( -V, N ), col = gemEnv( R ) * F;',
  '  vec3 ax = normalize( vAxis ), b1 = normalize( cross( ax, abs( ax.y ) < 0.9 ? vec3( 0.0, 1.0, 0.0 ) : vec3( 1.0, 0.0, 0.0 ) ) ), b2 = cross( ax, b1 );',
  '  vec3 rel = vW - vCtr; rel -= ax * dot( rel, ax );',
  '  float seg = floor( ( atan( dot( rel, b2 ), dot( rel, b1 ) ) / 6.2831853 + 0.5 ) * 16.0 );',
  '  float a0 = ( seg + 0.5 ) / 16.0 * 6.2831853 - 3.14159265, ring = floor( dot( N, ax ) * 3.0 );',
  '  float path = uDepth * ( 0.4 + 1.4 * gemHash( seg * 7.13 + ring * 3.1 ) );',
  '  vec3 tint = pow( max( uColor, vec3( 0.002 ) ), vec3( path ) );',
  '  #ifdef GEM_FIRE',
  '  vec3 inner = vec3( gemEnv( gemInside( V, N, ax, b1, b2, a0, 1.0 / 1.76 ) ).r, gemEnv( gemInside( V, N, ax, b1, b2, a0, 1.0 / 1.77 ) ).g, gemEnv( gemInside( V, N, ax, b1, b2, a0, 1.0 / 1.79 ) ).b );',
  '  vec3 Ti = gemInside( V, N, ax, b1, b2, a0, 1.0 / 1.77 );',
  '  inner += pow( max( dot( Ti, uKey ), 0.0 ), 60.0 ) * 6.0;',
  '  #else',
  '  vec3 Ti = gemInside( V, N, ax, b1, b2, a0, 1.0 / 1.77 ), inner = gemEnv( Ti );',
  '  #endif',
  // Scintillation: each facet of the pavilion either sends back a light in the room or the dark under
  // the stone, and which one changes with the angle, so the stone twinkles as the die turns.
  '  vec3 qd = floor( Ti * 2.5 + 0.5 );',
  '  float lit = smoothstep( 0.35, 0.85, gemHash( dot( qd, vec3( 1.7, 9.2, 3.3 ) ) + seg * 0.37 + ring * 2.9 ) );',
  '  col += ( 1.0 - F ) * inner * tint * mix( 0.12, 3.2, lit );',
  '  col += pow( max( dot( R, uKey ), 0.0 ), 700.0 ) * 26.0;',
  '  gl_FragColor = vec4( col, 1.0 );',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');
// The direction of the stage's key light, shared by every gem.
var GEMKEY = { value: null };
var GEMMAT = {};
function gemMat(T, colour, env, fire) {
  var key = colour + '|' + (fire ? 1 : 0) + '|' + (env ? env.uuid : 'none');
  if (GEMMAT[key]) return GEMMAT[key];
  if (!GEMKEY.value) GEMKEY.value = new T.Vector3(-0.55, 0.62, -0.55).normalize();
  var m;
  if (env) {
    m = new T.ShaderMaterial({ vertexShader: GEM_VS, fragmentShader: GEM_FS, defines: fire ? { GEM_FIRE: '' } : {}, extensions: { derivatives: true },
      uniforms: { uEnv: { value: env }, uColor: { value: new T.Color(colour) }, uKey: GEMKEY, uDepth: { value: 1.25 } } });
  } else {
    m = new T.MeshPhysicalMaterial({ color: colour, metalness: 0, roughness: 0.04, flatShading: true, specularIntensity: 1, ior: 1.77, envMapIntensity: 1.5 });
  }
  share(m); m.userData.env = env;
  GEMMAT[key] = m;
  return m;
}

// ---------- The die ----------
// q: { detail: 'hd' | 'basic', gems: true for the gems' fire, env: the stage's environment() }.
// The returned die has the fields the physics and the crit effects use (group, faces[i].{n, c, value,
// rest, th, shape), so the physics and the crit effects work on it unchanged.
function makeDie(T, C, sides, size, kind, c, style, cv, q) {
  q = q || { detail: 'hd' };
  var K = kindOf(kind), sd = solid(sides), atl = atlas(T, sd, K.key, c, style, cv, K.bev), body = bodyGeo(T, sd, atl, size, K.key), see = K.see || null;
  var hd = q.detail !== 'basic', group = new T.Group(), mats = [], parts = [];
  var mat = bodyMat(T, K.key, atl, c, q, false); mats.push(mat);
  // Solid dice take shadows on themselves too: the gems and a raised number shade the faces under them.
  var main = new T.Mesh(body.geo, mat); main.castShadow = true; main.receiveShadow = K.key !== 'confetti'; main.layers.enable(1); parts.push(main);
  var back = null, gems = null, flakes = null, flakeList = null;
  if (see === 'glass' && K.key !== 'confetti') {
    back = new T.Mesh(body.geo, bodyMat(T, K.key, atl, c, q, true)); mats.push(back.material); back.renderOrder = 1; main.renderOrder = 2;
    if (!K.dense) { main.castShadow = false; main.layers.disable(1); }
    parts.unshift(back);
  }
  if (K.key === 'confetti') {
    // The far side of the glass, seen through the near side: its numbers backwards and dimmer, which is
    // most of what makes clear glass read as solid.
    back = new T.Mesh(body.geo, bodyMat(T, K.key, atl, c, q, true)); mats.push(back.material); back.renderOrder = 1; main.renderOrder = 2;
    main.castShadow = false; main.layers.disable(1); parts.unshift(back);
    flakeList = flakeLayout(T, sd, size, hd ? 260 : 120, sides * 7 + 3);
    flakes = new T.InstancedMesh(flakeGeometry(T), flakeMat(T, hd), flakeList.length);
    var col = new T.Color();
    flakeList.forEach(function (fl, i) { flakes.setMatrixAt(i, fl.m); flakes.setColorAt(i, col.set(c.flakes[Math.floor(fl.pick * c.flakes.length)])); });
    parts.push(flakes);
  }
  if (body.gems.length) {
    gems = new T.InstancedMesh(gemGeometry(T), gemMat(T, jewelOf(c), q.env && q.env.cube, q.gems !== false && hd), body.gems.length);
    body.gems.forEach(function (gm, i) { gems.setMatrixAt(i, gm.m); });
    gems.castShadow = true; gems.layers.enable(1); parts.push(gems);
  }
  parts.forEach(function (p) { group.add(p); });
  var faces = sd.faces.map(function (f, fi) {
    var piece = new T.Group(); piece.position.set(f.c.x * size, f.c.y * size, f.c.z * size); group.add(piece);
    return { f: f, piece: piece, lay: atl.lays[fi], value: f.value, n: new T.Vector3(f.n.x, f.n.y, f.n.z), c: new T.Vector3(f.c.x, f.c.y, f.c.z).multiplyScalar(size) };
  });
  var shape = new C.ConvexPolyhedron({ vertices: sd.v.map(function (p) { return new C.Vec3(p.x * size, p.y * size, p.z * size); }), faces: sd.faces.map(function (f) { return f.idx.slice(); }) });
  var die = { sd: sd, group: group, faces: faces, body: body, atl: atl, main: main, back: back, gems: gems, flakes: flakes, flakeList: flakeList, parts: parts,
    mat: mat, rimMat: mat, mats: mats, shape: shape, size: size, sides: sides, th: c, kind: K.key, q: q, T: T, see: see, dense: !!K.dense,
    rest: dot(sd.faces[0].c, sd.faces[0].n) * size };
  // A set's own pieces (a core, an orb, stars inside) live in the die's space and stay put when an
  // effect swaps the body for its face pieces; tick turns them each frame.
  if (K.build) {
    var b = K.build(T, die, q) || {};
    die.inner = b.parts || []; die.inner.forEach(function (p) { group.add(p); });
    die.tick = b.tick || null; die.innerDispose = b.dispose || null;
  }
  return die;
}
// Swap the gems between plain and fiery, when the quality changes mid-session.
function setGemFire(die, on) {
  if (!die.gems) return;
  die.gems.material = gemMat(die.T, jewelOf(die.th), die.q.env && die.q.env.cube, on);
  die._pieces && die._pieces.forEach(function (p) { if (p.gems) p.gems.material = die.gems.material; });
}
function disposeDie(die) {
  if (die.group.parent) die.group.parent.remove(die.group);
  if (die.innerDispose) die.innerDispose();
  die.mats.forEach(function (m) { m.dispose(); });
  die.faces.forEach(function (fc) { if (fc.gt) { fc.gt.dispose(); fc.over.material.dispose(); } });
  if (die.gems) die.gems.dispose(); if (die.flakes) die.flakes.dispose();
  (die._pieces || []).forEach(function (p) { if (p.gems) p.gems.dispose(); if (p.flakes) p.flakes.dispose(); });
}

// ---------- The crit-effect API ----------
// The calls every crit effect makes on a die, so every crit plays on every kind of die.

// Fill each face's piece with its own meshes, the first time a die comes apart.
function buildPieces(die) {
  if (die._pieces) return die._pieces;
  var T = die.T, body = die.body, sd = die.sd;
  die._pieces = die.faces.map(function (fc, fi) {
    var geo = pieceGeo(T, body, sd, fi), out = { meshes: [] };
    var m = new T.Mesh(geo, die.mat); m.castShadow = die.main.castShadow; m.receiveShadow = die.main.receiveShadow; m.layers.mask = die.main.layers.mask; m.renderOrder = die.main.renderOrder;
    if (die.back) { var b = new T.Mesh(geo, die.back.material); b.renderOrder = 1; out.meshes.push(b); }
    out.meshes.push(m);
    var inv = new T.Matrix4().makeTranslation(-fc.c.x, -fc.c.y, -fc.c.z), tmp = new T.Matrix4();
    function subset(list, geoOf, mat) {
      var mine = list.filter(function (g) { return g.fi === fi; }); if (!mine.length) return null;
      var im = new T.InstancedMesh(geoOf, mat, mine.length);
      mine.forEach(function (g, i) { im.setMatrixAt(i, tmp.multiplyMatrices(inv, g.m)); });
      im.castShadow = !die.back; if (!die.back) im.layers.enable(1); return { im: im, mine: mine };
    }
    if (die.gems) { var g = subset(body.gems, die.gems.geometry, die.gems.material); if (g) { out.gems = g.im; out.meshes.push(g.im); } }
    if (die.flakes) {
      var f = subset(die.flakeList, die.flakes.geometry, die.flakes.material);
      if (f) { var col = new T.Color(); f.mine.forEach(function (fl, i) { f.im.setColorAt(i, col.set(die.th.flakes[Math.floor(fl.pick * die.th.flakes.length)])); }); out.flakes = f.im; out.meshes.push(f.im); }
    }
    out.meshes.forEach(function (x) { x.visible = false; fc.piece.add(x); });
    return out;
  });
  return die._pieces;
}
function showPieces(die, on) {
  buildPieces(die).forEach(function (p) { p.meshes.forEach(function (x) { x.visible = on; }); });
  die.parts.forEach(function (x) { x.visible = !on; });
}
// Split the die into one piece per face, loose in the scene where they were.
// Returns [{ m: Object3D, fc, dir: outward world normal, home: world position }].
function splitDie(st, die) {
  var g = die.group; g.updateMatrixWorld(true); showPieces(die, true);
  var out = die.faces.map(function (fc) {
    st.scene.attach(fc.piece);
    return { m: fc.piece, fc: fc, dir: fc.n.clone().applyQuaternion(g.quaternion).normalize(), home: fc.piece.position.clone() };
  });
  g.visible = false; die._split = out; return out;
}
// Put the pieces back: the die is whole again, at die.group's transform.
function wholeDie(die) {
  if (die._split) die.faces.forEach(function (fc) { die.group.add(fc.piece); });
  reassemble(die); if (die._pieces) showPieces(die, false); die.group.visible = true; die._split = null;
}
// A face's light layer, made the first time it is asked for: a canvas over the face, added as light.
function glowLayer(die, fi) {
  var fc = die.faces[fi]; if (fc.over) return fc;
  var T = die.T, cv = fc.lay.cv;
  fc.gc = canvas(cv); fc.gg = fc.gc.getContext('2d'); fc.gg.fillStyle = '#000'; fc.gg.fillRect(0, 0, cv, cv);
  fc.gt = new T.CanvasTexture(fc.gc); fc.gt.colorSpace = T.SRGBColorSpace;
  // Light added on top of whatever the face is, without touching its alpha: on a glass die the page
  // still shows through round the number, and the glow spills onto it as light.
  var m = new T.MeshBasicMaterial({ map: fc.gt, transparent: true, depthWrite: false, polygonOffset: true, polygonOffsetFactor: -2, polygonOffsetUnits: -2,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneFactor, blendSrcAlpha: T.ZeroFactor, blendDstAlpha: T.OneFactor });
  m.color.setScalar(0);
  fc.over = new T.Mesh(overlayGeo(T, die.body, die.sd, die.atl, fi), m); fc.over.renderOrder = 3; fc.over.visible = false;
  fc.piece.add(fc.over);
  // Effects set fc.mat.emissiveIntensity; here that is the brightness of the light layer.
  fc.mat = { color: die.mat.color };
  Object.defineProperty(fc.mat, 'emissiveIntensity', { get: function () { return m.color.r; }, set: function (k) { m.color.setScalar(k); fc.over.visible = k > 0.001; } });
  return fc;
}
// The light drawn on one face, as a 2D canvas over that face: draw on g (black is no light), then commit(intensity).
function glowFace(die, fi) {
  var fc = glowLayer(die, fi), cv = fc.lay.cv;
  return { g: fc.gg, lay: fc.lay, cv: cv,
    clear: function () { fc.gg.fillStyle = '#000'; fc.gg.fillRect(0, 0, cv, cv); },
    commit: function (k) { fc.gt.needsUpdate = true; if (k != null) fc.mat.emissiveIntensity = k; } };
}
// The landed number drawn lit on a face's glow canvas, in a colour.
function glowNumeral(die, fi, color, alpha) { var gf = glowFace(die, fi); gf.clear(); gf.g.globalAlpha = alpha == null ? 1 : alpha; numberOnto(gf.g, gf.lay, color); gf.g.globalAlpha = 1; gf.commit(1.6); }
// Light the whole die's rims/metal in a colour (k = 0 turns it off).
function tintDie(die, color, k) { die.mat.emissive.set(color); die.mat.emissiveIntensity = k; if (die.back) { die.back.material.emissive.set(color); die.back.material.emissiveIntensity = k; } }
// Darken (k < 1) or brighten the body colour of every face; 1 is normal.
function shadeDie(die, k) { die.mat.color.setScalar(k); if (die.back) die.back.material.color.setScalar(0.55 * k); }
// A point on a face, from canvas px (x, y) in that face's drawing, to the die's local space.
function faceToLocal(die, fi, x, y) {
  var fc = die.faces[fi], f = fc.f, lay = fc.lay, h = lay.cv / 2, a = (x - h) / lay.s, b = (h - y) / lay.s;
  return new die.T.Vector3(f.c.x + f.right.x * a + f.up.x * b, f.c.y + f.right.y * a + f.up.y * b, f.c.z + f.right.z * a + f.up.z * b).multiplyScalar(die.size);
}

// ---------- The table ----------
// A see-through canvas over the page, with each costly part of the picture behind
// a switch (st.q), so quality can drop one part at a time, even mid-throw:
//   glow     the bloom pass, so lit numbers glow past their edges
//   contact  soft contact shadows rendered from under the table (off: a soft blob under each die)
//   gems     the gems' fire and sparkle
//   sharp    full pixel density and sharper faces (off: 1.25x and smaller face images)
//   soft     a finer shadow map
//   detail   'hd' or 'basic': flakes, pearl and coats on new dice
var LAYERS = ['glow', 'contact', 'gems', 'sharp', 'soft', 'detail'];
function qualityPreset(name) {
  var on = name !== 'basic';
  return { name: name, glow: on, contact: on, gems: on, sharp: on, soft: on, detail: on ? 'hd' : 'basic' };
}
function faceSize(q) { return q.sharp ? 384 : 256; }

// How long the table fades in and out: the --dur-large token, so the owner's Motion speed retimes it.
function fadeMs() {
  var v = typeof document !== 'undefined' ? getComputedStyle(document.documentElement).getPropertyValue('--dur-large').trim() : '', n = parseFloat(v);
  return isFinite(n) ? (/ms$/.test(v) ? n : n * 1000) : 280;
}

function makeStage(T, host, fixed, FX, q) {
  q = Object.assign({}, q || qualityPreset('hd'));
  var r = fixed ? { width: innerWidth, height: innerHeight } : host.getBoundingClientRect();
  var W = Math.max(120, Math.round(r.width)), H = Math.max(120, Math.round(r.height));
  var cnv = document.createElement('canvas');
  cnv.className = 'chronicle-dice';
  cnv.setAttribute('aria-hidden', 'true');
  cnv.style.cssText = (fixed ? 'position:fixed;inset:0;width:100vw;height:100vh;z-index:900;' : 'position:absolute;inset:0;width:100%;height:100%;') + 'pointer-events:none;transition:opacity ' + fadeMs() + 'ms ease';
  var renderer = new T.WebGLRenderer({ canvas: cnv, alpha: true, antialias: true, premultipliedAlpha: true });
  (fixed ? document.body : host).appendChild(cnv);
  function ratio() { return Math.min(window.devicePixelRatio || 1, q.sharp ? 2 : 1.25); }
  renderer.setPixelRatio(ratio()); renderer.setSize(W, H, false);
  renderer.shadowMap.enabled = true; renderer.shadowMap.type = T.PCFSoftShadowMap;
  renderer.outputColorSpace = T.SRGBColorSpace; renderer.localClippingEnabled = true;
  renderer.toneMapping = T.ACESFilmicToneMapping; renderer.toneMappingExposure = 1.05;
  var scene = new T.Scene(), hw = W / 2 / PX, hh = H / 2 / PX, dist = hh / TAN, env3 = environment(T, renderer);
  scene.environment = env3.pmrem;
  var cam = new T.PerspectiveCamera(FOV, W / H, 1, dist * 2);
  cam.position.set(0, dist, 0); cam.up.set(0, 0, -1); cam.lookAt(0, 0, 0);
  scene.add(new T.HemisphereLight(0xffffff, 0x6a7080, 0.55));
  // The key light sits about 45 degrees up, not overhead: overhead, its glare sits on every top face,
  // right where the number is read.
  var sun = new T.DirectionalLight(0xfff6ea, 2.6); sun.position.set(-hw * 0.6, dist * 0.3, -hh * 0.6); sun.castShadow = true;
  var sc = sun.shadow.camera; sc.left = -hw * 1.3; sc.right = hw * 1.3; sc.top = hh * 1.3; sc.bottom = -hh * 1.3; sc.near = 1; sc.far = dist * 2;
  sun.shadow.bias = -0.0004; sun.shadow.normalBias = 0.02;
  scene.add(sun);
  GEMKEY.value = sun.position.clone().normalize();
  var fill = new T.DirectionalLight(0xdfe8ff, 0.6); fill.position.set(hw * 0.6, dist * 0.4, hh * 0.5); scene.add(fill);
  var floor = new T.Mesh(new T.PlaneGeometry(hw * 4, hh * 4), new T.ShadowMaterial({ opacity: 0.3 }));
  floor.rotation.x = -Math.PI / 2; floor.receiveShadow = true; scene.add(floor);
  var st = { T: T, renderer: renderer, scene: scene, cam: cam, canvas: cnv, W: W, H: H, hw: hw, hh: hh, dist: dist, fx: [], dice: [], contact: [], dead: false,
    cine: false, composer: null, q: q, env3: env3, sun: sun, FX: FX,
    toWorld: function (px, py) { return { x: (px - W / 2) / PX, z: (py - H / 2) / PX }; },
    toScreen: function (v) { var p = v.clone().project(cam); return { x: (p.x + 1) / 2 * W, y: (1 - p.y) / 2 * H }; },
    ptSize: function (worldDiameter) { return worldDiameter / TAN; } };
  function shadows() {
    var n = q.soft ? 2048 : 1024;
    if (sun.shadow.mapSize.x !== n) { sun.shadow.mapSize.set(n, n); if (sun.shadow.map) { sun.shadow.map.dispose(); sun.shadow.map = null; } }
  }
  function glow() {
    if (q.glow && FX && !st.composer) {
      // Bloom on a see-through canvas: the glow lands where alpha is 0, and with premultiplied alpha the
      // page shows it as added light, so a burning number lights the page round the die.
      var pr = renderer.getPixelRatio(), rt = new T.WebGLRenderTarget(W * pr, H * pr, { type: T.HalfFloatType, samples: 4 });
      var comp = new FX.EffectComposer(renderer, rt), rp = new FX.RenderPass(scene, cam); rp.clearAlpha = 0; comp.addPass(rp);
      var bloom = new FX.UnrealBloomPass(new T.Vector2(W, H), 0.24, 0.35, 0.98);
      // The blur writes alpha 1 everywhere, which would paint the page black: add the glow's colour only.
      var bm = bloom.blendMaterial; bm.blending = T.CustomBlending; bm.blendSrc = T.OneFactor; bm.blendDst = T.OneFactor;
      bm.blendSrcAlpha = T.ZeroFactor; bm.blendDstAlpha = T.OneFactor;
      comp.addPass(bloom); comp.addPass(new FX.OutputPass());
      st.composer = comp;
    }
    st.cine = !!(q.glow && st.composer);
  }
  shadows(); glow();
  st.contactPass = makeContact(st);
  st.draw = function () {
    if (q.contact) st.contactPass.render();
    if (st.cine) st.composer.render(); else renderer.render(scene, cam);
  };
  // Turn one layer on or off now. Faces and flakes ('sharp', 'detail') apply to dice made after this.
  st.setLayer = function (name, on) {
    if (name === 'detail') { q.detail = on === true || on === 'hd' ? 'hd' : 'basic'; return; }
    q[name] = !!on;
    if (name === 'glow') glow();
    if (name === 'soft') shadows();
    if (name === 'sharp') { renderer.setPixelRatio(ratio()); renderer.setSize(W, H, false); if (st.composer) { st.composer.setPixelRatio(renderer.getPixelRatio()); st.composer.setSize(W, H); } }
    if (name === 'gems') st.dice.forEach(function (d) { setGemFire(d, q.gems && d.q.detail !== 'basic'); });
    if (name === 'contact') st.contactPass.show(q.contact);
  };
  STAGES.add(st);
  return st;
}

// Contact shadows: the dice seen from under the table as a depth image, blurred, laid on the table.
// They darken right where a die touches and fade as it rises, which a shadow map cannot do finely.
// Off (or in Basic), each die gets a soft blob instead.
function makeContact(st) {
  var T = st.T, R = st.renderer, hw = st.hw, hh = st.hh, res = 512, ar = hw / hh;
  var rw = Math.round(ar >= 1 ? res : res * ar), rh = Math.round(ar >= 1 ? res / ar : res);
  var rt = new T.WebGLRenderTarget(rw, rh), rtb = new T.WebGLRenderTarget(rw, rh);
  rt.texture.generateMipmaps = rtb.texture.generateMipmaps = false;
  var height = 2.4, grp = new T.Group(); st.scene.add(grp);
  var plane = new T.Mesh(new T.PlaneGeometry(hw * 2, hh * 2), new T.MeshBasicMaterial({ map: rt.texture, transparent: true, opacity: 0.92, depthWrite: false, toneMapped: false }));
  plane.rotation.x = -Math.PI / 2; plane.position.y = 0.004; plane.renderOrder = -1; grp.add(plane);
  // Looking straight up from the table, so its image lines up with the plane above it.
  var cam = new T.OrthographicCamera(-hw, hw, hh, -hh, 0, height); cam.rotation.x = Math.PI / 2; cam.layers.set(1); grp.add(cam);
  // The texture's rows run the other way to the plane's: flip the plane's v so they line up.
  var uvs = plane.geometry.attributes.uv; for (var i = 0; i < uvs.count; i++) uvs.setY(i, 1 - uvs.getY(i));
  // Nearest surface wins, so the bottom of a die (darkest) is never painted over by flakes above it.
  var depth = new T.MeshDepthMaterial();
  depth.onBeforeCompile = function (s) {
    s.fragmentShader = s.fragmentShader.replace('gl_FragColor = vec4( vec3( 1.0 - fragCoordZ ), opacity );', 'gl_FragColor = vec4( vec3( 0.0 ), pow( 1.0 - fragCoordZ, 2.2 ) * 0.9 );');
  };
  var FX = st.FX, hb = null, vb = null, quad = null;
  if (FX && FX.HorizontalBlurShader) {
    hb = new T.ShaderMaterial(FX.HorizontalBlurShader); vb = new T.ShaderMaterial(FX.VerticalBlurShader); hb.depthTest = vb.depthTest = false;
    quad = new T.Mesh(new T.PlaneGeometry(hw * 2, hh * 2), hb); quad.rotation.x = Math.PI / 2; quad.layers.set(1); quad.visible = false; grp.add(quad);
  }
  function blur(amount) {
    if (!quad) return;
    quad.visible = true;
    quad.material = hb; hb.uniforms.tDiffuse.value = rt.texture; hb.uniforms.h.value = amount / rw; R.setRenderTarget(rtb); R.render(quad, cam);
    quad.material = vb; vb.uniforms.tDiffuse.value = rtb.texture; vb.uniforms.v.value = amount / rh; R.setRenderTarget(rt); R.render(quad, cam);
    quad.visible = false;
  }
  var on = !!st.q.contact, blobs = [];
  function show(v) { on = v; plane.visible = v; st.contact.forEach(function (c) { c.m.visible = c.glass || !v; }); }
  show(on);
  return {
    show: show,
    render: function () {
      if (!on) return;
      var bg = st.scene.background, ov = st.scene.overrideMaterial, ca = R.getClearAlpha(), cc = R.getClearColor(new T.Color());
      plane.visible = false; st.scene.background = null; st.scene.overrideMaterial = depth;
      R.setClearColor(0x000000, 0); R.setRenderTarget(rt); R.clear(); R.render(st.scene, cam);
      st.scene.overrideMaterial = ov; blur(1.6); blur(0.8);
      R.setRenderTarget(null); R.setClearColor(cc, ca); st.scene.background = bg; plane.visible = true;
    },
    dispose: function () { rt.dispose(); rtb.dispose(); plane.geometry.dispose(); plane.material.dispose(); depth.dispose(); if (hb) { hb.dispose(); vb.dispose(); quad.geometry.dispose(); } }
  };
}
// A die joins the table with its blob, used when contact shadows are off. A glass die always has its
// blob (it is left out of the contact image, which would draw it as solid), lighter, plus a shadow
// tinted its own colour: light through coloured glass lands coloured.
function addDie(st, die) {
  st.scene.add(die.group); st.dice.push(die);
  addContact(st, die);
  var c = st.contact[st.contact.length - 1], T = st.T;
  c.glass = die.see === 'glass' && !die.dense; c.m.visible = c.glass || !st.q.contact;
  if (c.glass) {
    var tint = die.th.tint || die.th.shadow || die.th.base.grad[0];
    c.tint = new T.Mesh(new T.PlaneGeometry(1, 1), new T.MeshBasicMaterial({ map: shadowTex(T), color: new T.Color(mix(tint, '#000000', 0.45)), transparent: true, depthWrite: false, toneMapped: false }));
    c.tint.rotation.x = -Math.PI / 2; c.tint.renderOrder = -1; st.scene.add(c.tint);
  }
}
// Each frame: blobs tighten as a die comes down; a glass die's tinted shadow falls away from the key light.
function updateContact(st) {
  var sx = -st.sun.position.x / st.sun.position.y, sz = -st.sun.position.z / st.sun.position.y;
  st.contact.forEach(function (c) {
    var d = c.die, g = d.group, h = Math.max(0, g.position.y - d.rest), s = d.size, k = Math.min(1, h / (s * 4)), fade = d.group.visible ? (d.fade == null ? 1 : d.fade) : 0;
    c.m.position.set(g.position.x, 0.006, g.position.z);
    var w = s * (2.3 + k * 2.2); c.m.scale.set(w, w, 1);
    c.m.material.opacity = 0.62 * (1 - k) * fade * (c.glass ? 0.45 : 1);
    if (c.tint) {
      var y = g.position.y;
      c.tint.position.set(g.position.x + sx * y * 0.8, 0.005, g.position.z + sz * y * 0.8);
      var tw = s * (2.9 + k * 2); c.tint.scale.set(tw * 1.15, tw, 1);
      c.tint.material.opacity = (0.32 + (d.th.alpha == null ? 0.3 : d.th.alpha) * 0.5) * (1 - k * 0.6) * fade;
    }
  });
}
function removeDie(st, die) {
  st.contact = st.contact.filter(function (c) {
    if (c.die !== die) return true;
    [c.m, c.tint].forEach(function (m) { if (m) { m.parent.remove(m); m.material.dispose(); m.geometry.dispose(); } });
    return false;
  });
  st.dice = st.dice.filter(function (d) { return d !== die; });
  disposeDie(die);
}
function disposeStage(st) {
  if (!st || st.dead) return;
  st.dice.slice().forEach(function (d) { removeDie(st, d); });
  st.contactPass.dispose();
  st.dead = true;
  st.scene.traverse(function (o) {
    if (o.geometry && !o.geometry.userData.shared) o.geometry.dispose();
    var ms = o.material ? (Array.isArray(o.material) ? o.material : [o.material]) : [];
    ms.forEach(function (m) { if (!m.userData.shared) m.dispose(); });
  });
  if (st.composer) st.composer.dispose();
  // The environment is made per renderer; this renderer is going away, so its copy and the gems that
  // reflect it go too.
  var env = st.env3;
  Object.keys(GEMMAT).forEach(function (k) { if (GEMMAT[k].userData.env === env.cube) { SHARED.delete(GEMMAT[k]); GEMMAT[k].dispose(); delete GEMMAT[k]; } });
  env.pmrem.dispose(); env.rt.dispose(); if (ENV === env) ENV = null;
  st.renderer.dispose(); try { st.renderer.forceContextLoss(); } catch (e) {}
  st.canvas.remove();
  // With no table left, the shared objects are let go of every renderer that drew them; the next
  // table uploads them again.
  STAGES.delete(st);
  if (!STAGES.size) SHARED.forEach(function (o) { o.dispose(); });
}

// ---------- Showing off the number that lands ----------
// Every set marks its landed number with one of the shows, cheaply enough to play on a dozen dice at
// once: the face image is drawn once and the animation runs on the graphics card from a few numbers.
//   Molten   the metal runs white-hot along the strokes like a fuse, then cools to a gleam
//   Gather   flakes inside swirl up and settle round the number, just under its face
//   Enamel   the die hops and the number rises out of the face in glossy enamel
// A show is play(st, die, fi, lay, k) -> { update(dt) } and clear(die). lay is the number to show:
// usually the face's own, but a d4 shows the number at its top point on each of three faces, so a show
// runs once per place (k counts them; anything that moves the whole die does it only for k = 0).
var SHOWS = {};
function registerShow(key, def) { def.key = key; SHOWS[key] = def; }
// Where a die shows its landed number: its face, or on a d4 the top corner of the three faces it shows.
function resultSlots(die, fi) {
  if (die.sides !== 4) return [{ fi: fi, lay: die.faces[fi].lay }];
  var idx = die.faces[fi].f.idx, top = -1, out = [];
  for (var q = 0; q < 4; q++) if (idx.indexOf(q) < 0) top = q;
  die.faces.forEach(function (fc, j) { if (j !== fi && fc.lay.sub) fc.lay.sub.forEach(function (sl) { if (sl.corner === top) out.push({ fi: j, lay: sl }); }); });
  return out;
}

// The numeral as data, per face: red = how far along the strokes (0..1, divided by green), green =
// the numeral's shape, blue = a soft halo round it.
var HLDATA = {};
function numeralData(T, die, fi, lay) {
  lay = lay || die.faces[fi].lay;
  var key = [die.sides, die.kind, die.th.key, lay.cv, lay.style, fi, lay.corner == null ? '' : lay.corner].join('|');
  if (HLDATA[key]) return HLDATA[key];
  var cv = lay.cv, w = strokeW(lay), a = canvas(cv), ag = a.getContext('2d', { willReadFrequently: true });
  var b = canvas(cv), bg = b.getContext('2d', { willReadFrequently: true });
  ag.fillStyle = '#000'; ag.fillRect(0, 0, cv, cv); bg.fillStyle = '#000'; bg.fillRect(0, 0, cv, cv);
  var lens = lay.strokes.map(polyLen), total = lens.reduce(function (x, y) { return x + y; }, 0) || 1, acc = 0;
  ag.lineCap = 'round'; ag.lineJoin = 'round'; ag.lineWidth = w * 1.05;
  // Short pieces, each coloured with how far along the whole numeral it lies.
  lay.strokes.forEach(function (pl) {
    for (var k = 1; k < pl.length; k++) {
      var x0 = pl[k - 1][0], y0 = pl[k - 1][1], x1 = pl[k][0], y1 = pl[k][1], L = Math.hypot(x1 - x0, y1 - y0), n = Math.max(1, Math.ceil(L / 2));
      for (var j = 0; j < n; j++) {
        var p = (acc + L * (j + 0.5) / n) / total;
        ag.strokeStyle = 'rgb(' + Math.round(p * 255) + ',255,0)';
        ag.beginPath(); ag.moveTo(x0 + (x1 - x0) * j / n, y0 + (y1 - y0) * j / n); ag.lineTo(x0 + (x1 - x0) * (j + 1) / n, y0 + (y1 - y0) * (j + 1) / n); ag.stroke();
      }
      acc += L;
    }
  });
  if (lay.label === '6' || lay.label === '9') { ag.fillStyle = 'rgb(255,255,0)'; ag.beginPath(); ag.arc(lay.cx, lay.cy + lay.h * 0.68, w * 0.62, 0, 7); ag.fill(); }
  try { bg.filter = 'blur(' + (w * 0.9) + 'px)'; } catch (e) {}
  bg.strokeStyle = bg.fillStyle = '#fff'; paintNumeral(bg, lay, lay.style, w * 1.6);
  var A = ag.getImageData(0, 0, cv, cv), B = bg.getImageData(0, 0, cv, cv).data;
  for (var i = 0; i < A.data.length; i += 4) { A.data[i + 2] = B[i]; A.data[i + 3] = 255; }
  ag.putImageData(A, 0, 0);
  var t = new T.CanvasTexture(a); share(t); t.generateMipmaps = false; t.minFilter = T.LinearFilter;
  HLDATA[key] = { tex: t, total: total };
  return HLDATA[key];
}
// The point a fraction p of the way along the numeral's strokes, in face canvas px.
function strokeAt(lay, p) {
  var lens = lay.strokes.map(polyLen), total = lens.reduce(function (x, y) { return x + y; }, 0), want = total * Math.max(0, Math.min(1, p)), acc = 0;
  for (var i = 0; i < lay.strokes.length; i++) {
    var pl = lay.strokes[i];
    if (acc + lens[i] < want && i < lay.strokes.length - 1) { acc += lens[i]; continue; }
    for (var k = 1; k < pl.length; k++) {
      var L = Math.hypot(pl[k][0] - pl[k - 1][0], pl[k][1] - pl[k - 1][1]);
      if (acc + L >= want || k === pl.length - 1) { var f = L ? Math.max(0, Math.min(1, (want - acc) / L)) : 0; return [pl[k - 1][0] + (pl[k][0] - pl[k - 1][0]) * f, pl[k - 1][1] + (pl[k][1] - pl[k - 1][1]) * f, pl[k][0] - pl[k - 1][0], pl[k][1] - pl[k - 1][1]]; }
      acc += L;
    }
  }
  return [lay.cx, lay.cy, 1, 0];
}

// The light over the number. Added as light on dark dice; on light ones it also covers the face a
// little, so a hot orange still reads on ivory.
var HL_VS = 'varying vec2 vUv; void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4( position, 1.0 ); }';
var HL_FS = [
  'uniform sampler2D uData; uniform float uP; uniform float uT; uniform float uK; uniform float uHalo; uniform float uCover; uniform float uCool; uniform float uInk;',
  'uniform vec3 uHot; uniform vec3 uBurn; uniform vec3 uGleam;',
  'varying vec2 vUv;',
  'void main() {',
  '  vec4 d = texture2D( uData, vUv );',
  '  float m = d.g, prog = d.r / max( d.g, 0.004 ), since = uP - prog;',
  '  float lit = smoothstep( -0.015, 0.0, since ) * m, heat = exp( -max( since, 0.0 ) * uCool ) * step( 0.0, since );',
  '  vec3 col = mix( uGleam, mix( uBurn, uHot, smoothstep( 0.55, 1.0, heat ) ), smoothstep( 0.0, 0.45, heat ) );',
  '  float flick = 0.88 + 0.12 * sin( uT * 21.0 + prog * 37.0 ) * heat;',
  '  vec3 o = col * lit * uInk * ( 0.75 + 1.9 * heat ) * flick + uBurn * d.b * uHalo * smoothstep( 0.6, 1.0, uP ) * ( 0.4 + 0.8 * heat );',
  '  gl_FragColor = vec4( o * uK, clamp( lit * uCover * uK, 0.0, 1.0 ) );',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');
function hlLayer(die, fi, lay) {
  var fc = die.faces[fi], T = die.T, data = numeralData(T, die, fi, lay);
  if (fc.hl) { fc.hl.material.uniforms.uData.value = data.tex; return fc.hl; }
  var m = new T.ShaderMaterial({ vertexShader: HL_VS, fragmentShader: HL_FS, transparent: true, depthWrite: false, polygonOffset: true, polygonOffsetFactor: -3, polygonOffsetUnits: -3,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.ZeroFactor, blendDstAlpha: T.OneFactor,
    uniforms: { uData: { value: data.tex }, uP: { value: 0 }, uT: { value: 0 }, uK: { value: 0 }, uHalo: { value: 0.6 }, uCover: { value: 0 }, uCool: { value: 3.2 }, uInk: { value: 1 },
      uHot: { value: new T.Color('#fff4d8') }, uBurn: { value: new T.Color(die.th.burn) }, uGleam: { value: new T.Color(die.th.burn) } } });
  var mesh = new T.Mesh(overlayGeo(T, die.body, die.sd, die.atl, fi), m); mesh.renderOrder = 4; mesh.visible = false;
  fc.piece.add(mesh); fc.hl = mesh;
  return mesh;
}
// Light dice: how much the lit number covers the face under it (0 on dark dice, where light adds).
function coverOf(die) {
  var K = kindOf(die.kind); if (K.cover) return K.cover(die.th);
  return die.kind === 'jewelled' && isLight(die.th) ? 0.8 : (die.kind === 'acrylic' || SETS[die.kind]) && isLight(die.th) ? 0.6 : 0;
}

// Molten inlay: a fuse of white-hot metal runs the strokes, sparks spitting from its tip, then the
// whole number cools from orange to a steady gleam of the die's own glow colour.
function hlMolten(st, die, fi, lay) {
  lay = lay || die.faces[fi].lay;
  var T = st.T, mesh = hlLayer(die, fi, lay), u = mesh.material.uniforms, t = 0, run = 1.05, hd = st.q.detail !== 'basic';
  u.uHot.value.set('#fff4d8'); u.uGleam.value.set(mix(die.th.burn, accentOf(die.th), 0.35)); u.uCover.value = coverOf(die); u.uHalo.value = 0.7; u.uCool.value = 3.2; u.uInk.value = 1;
  mesh.visible = true;
  var s = die.size, n = hd ? 46 : 22;
  st.fx.push(spray(st, { n: n, colors: ['#fff6d8', die.th.burn, mix(die.th.burn, '#ffffff', 0.5)], size: s * 0.07, life: [0.25, 0.6], drag: 1.2, g: 9,
    spawn: function (i, r) {
      var p = i / n, at = strokeAt(lay, p), w = worldOf(die, faceToLocal(die, fi, at[0], at[1]));
      return { p: [w.x, w.y + 0.02, w.z], v: [(r() - 0.5) * 3.2, 1.5 + r() * 3, (r() - 0.5) * 3.2], delay: p * run };
    } }));
  return { update: function (dt) {
    t += dt;
    u.uT.value = t;
    u.uP.value = t < run ? inout(t / run) * 1.02 : 1.02 + (t - run) * 1.6;
    u.uK.value = Math.min(1, t / 0.08) * (t < run + 0.3 ? 1.15 : 1.15 - 0.3 * smooth(run + 0.3, run + 1.2, t));
    return t < run + 1.25;
  } };
}

// Gather: some of the flakes inside lift off their places and spiral up to the landed face, where
// they settle flat along both sides of every stroke, so the number is traced in foil from inside.
function hlGather(st, die, fi, lay, k) {
  lay = lay || die.faces[fi].lay;
  var T = st.T, fc = die.faces[fi], hd = st.q.detail !== 'basic', n = hd ? 130 : 64, list = die.flakeList, t = 0;
  if (!die.flakes || !list) return { update: function () { return false; } };
  // A d4 gathers three numbers at once: each takes its own share of the flakes.
  var share = die.sides === 4 ? 3 : 1;
  n = Math.floor(n / share);
  var rnd = mulberry(fi * 31 + 7), pick = [], used = die._gatherUsed || {}, guard = 0;
  while (pick.length < Math.min(n, list.length / share) && guard++ < 4000) { var i = Math.floor(rnd() * list.length); if (!used[i]) { used[i] = 1; pick.push(i); } }
  die._gatherUsed = used;
  var N = new T.Vector3(fc.n.x, fc.n.y, fc.n.z), w = strokeW(lay), zero = new T.Matrix4().makeScale(0, 0, 0), flat = new T.Quaternion().setFromUnitVectors(new T.Vector3(0, 0, 1), N);
  // The gathered foil catches a little of the die's glow, so the traced number stands out from the
  // flakes still scattered through the glass.
  var gm = new T.MeshPhysicalMaterial({ color: 0xffffff, metalness: 1, roughness: 0.2, side: T.DoubleSide, envMapIntensity: 1.8, emissive: new T.Color(die.th.burn), emissiveIntensity: 0,
    iridescence: hd ? 0.9 : 0, iridescenceIOR: 1.6, iridescenceThicknessRange: [220, 620] });
  var im = new T.InstancedMesh(die.flakes.geometry, gm, pick.length), col = new T.Color();
  var mv = [], P = new T.Vector3(), Q = new T.Quaternion(), S = new T.Vector3(), M = new T.Matrix4();
  pick.forEach(function (li, k) {
    var fl = list[li], p0 = new T.Vector3(), q0 = new T.Quaternion(), s0 = new T.Vector3(); fl.m.decompose(p0, q0, s0);
    var pr = (k + rnd()) / pick.length, at = strokeAt(lay, pr), dl = Math.hypot(at[2], at[3]) || 1, side = k % 2 ? 1 : -1, off = w * (0.8 + rnd() * 0.3) * side;
    var x = at[0] - at[3] / dl * off, y = at[1] + at[2] / dl * off, p1 = faceToLocal(die, fi, x, y).addScaledVector(N, -die.size * 0.06);
    var q1 = flat.clone().multiply(new T.Quaternion().setFromAxisAngle(new T.Vector3(0, 0, 1), rnd() * 6.28));
    mv.push({ p0: p0, q0: q0, s0: s0, p1: p1, q1: q1, delay: pr * 0.5 + rnd() * 0.12, spin: 1.2 + rnd() * 1.6 });
    die.flakes.setMatrixAt(li, zero);
    im.setColorAt(k, col.set(die.th.flakes[Math.floor(fl.pick * die.th.flakes.length)]));
  });
  die.flakes.instanceMatrix.needsUpdate = true;
  (die._gathered = die._gathered || []).push({ im: im, pick: pick });
  die.group.add(im);
  var mesh = hlLayer(die, fi, lay), u = mesh.material.uniforms; u.uP.value = 2; u.uCool.value = 50; u.uHalo.value = 0.9; u.uCover.value = 0; u.uK.value = 0; u.uInk.value = 0.2;
  u.uGleam.value.set(die.th.burn); mesh.visible = true;
  var axis = N, rel = new T.Vector3(), tmpQ = new T.Quaternion(), dur = 1.5;
  function place() {
    mv.forEach(function (f, k) {
      var e = inout(clamp01((t - f.delay) / 0.95));
      // Spiral about the face's axis: the angle unwinds as the flake arrives.
      P.lerpVectors(f.p0, f.p1, e);
      rel.copy(P); var along = rel.dot(axis); rel.addScaledVector(axis, -along);
      rel.applyQuaternion(tmpQ.setFromAxisAngle(axis, (1 - e) * f.spin * Math.PI)); P.copy(rel).addScaledVector(axis, along);
      Q.slerpQuaternions(f.q0, f.q1, e); S.copy(f.s0).multiplyScalar((1 - 0.35 * e) * (1 + 0.25 * Math.sin(e * Math.PI)));
      im.setMatrixAt(k, M.compose(P, Q, S));
    });
    im.instanceMatrix.needsUpdate = true;
  }
  place();
  return { update: function (dt) {
    t += dt; place();
    // A soft light behind the gathered foil once it has arrived, so the traced number pops.
    u.uK.value = 0.55 * smooth(0.8, 1.5, t); gm.emissiveIntensity = 0.45 * smooth(0.7, 1.4, t);
    u.uT.value = t;
    return t < dur + 0.2;
  } };
}

// Raised enamel: the die gives a small hop on landing and the number rises out of the face as
// rounded strokes of glossy enamel in the die's accent colour, with a ring on the table.
function enamelGeo(T, die, fi, lay) {
  var fc = die.faces[fi], r = strokeW(lay) * 0.8 / lay.s * die.size, parts = [];
  function local(px) { return faceToLocal(die, fi, px[0], px[1]).sub(fc.c); }
  function joint(p) { var g = new T.SphereGeometry(r, 12, 8).toNonIndexed(); g.translate(p.x, p.y, p.z); parts.push(g); }
  lay.strokes.forEach(function (pl) {
    var pts = pl.map(local);
    pts.forEach(joint);
    for (var k = 1; k < pts.length; k++) {
      var a = pts[k - 1], b = pts[k], L = a.distanceTo(b); if (L < 1e-5) continue;
      var g = new T.CylinderGeometry(r, r, L, 12, 1, true).toNonIndexed();
      g.applyQuaternion(new T.Quaternion().setFromUnitVectors(new T.Vector3(0, 1, 0), b.clone().sub(a).normalize()));
      g.translate((a.x + b.x) / 2, (a.y + b.y) / 2, (a.z + b.z) / 2); parts.push(g);
    }
  });
  if (lay.label === '6' || lay.label === '9') joint(local([lay.cx, lay.cy + lay.h * 0.68]));
  var n = 0; parts.forEach(function (g) { n += g.attributes.position.count; });
  var pos = new Float32Array(n * 3), nor = new Float32Array(n * 3), o = 0;
  parts.forEach(function (g) { pos.set(g.attributes.position.array, o * 3); nor.set(g.attributes.normal.array, o * 3); o += g.attributes.position.count; g.dispose(); });
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.BufferAttribute(pos, 3)); geo.setAttribute('normal', new T.BufferAttribute(nor, 3));
  return { geo: geo, r: r };
}
function hlEnamel(st, die, fi, lay, k) {
  lay = lay || die.faces[fi].lay;
  var T = st.T, fc = die.faces[fi], eg = enamelGeo(T, die, fi, lay), t = 0, y0 = die.group.position.y, s = die.size, lead = !k;
  var pop = die.th.pop || die.th.burn;
  var mat = new T.MeshPhysicalMaterial({ color: pop, roughness: 0.1, metalness: 0, clearcoat: 1, clearcoatRoughness: 0.02, specularIntensity: 1, emissive: new T.Color(pop), emissiveIntensity: 0.6 });
  var mesh = new T.Mesh(eg.geo, mat), N = new T.Vector3(fc.n.x, fc.n.y, fc.n.z); mesh.castShadow = true; fc.piece.add(mesh);
  (die._enamel = die._enamel || []).push(mesh);
  var base = die.group.position.clone();
  if (lead) st.fx.push(wave(st, { x: base.x, z: base.z }, die.th.burn, s * 0.9, s * 2.1, 0.7, isLight(die.th)));
  // k runs 0..1: the strokes come up out of the face, float clear of it for a moment, then settle
  // onto it standing proud. From above, rising reads as the number growing and its shadow parting.
  function set(k) {
    var up = Math.sin(Math.PI * Math.min(1, k)) * (1 - 0.35 * k);
    mesh.position.copy(N).multiplyScalar(-eg.r * 1.2 + eg.r * 2.1 * Math.min(1, k * 1.4) + s * 0.24 * up);
    mesh.scale.setScalar(1 + 0.22 * up);
  }
  set(0);
  return { update: function (dt) {
    t += dt;
    // The hop: up and down in a quarter second, with a little squash as it lands again.
    var hop = t < 0.34 ? Math.sin(Math.PI * t / 0.34) : 0;
    if (lead) die.group.position.y = y0 + hop * s * 0.32;
    var sq = t > 0.3 && t < 0.5 ? Math.sin(Math.PI * (t - 0.3) / 0.2) * 0.06 : 0; if (lead) die.group.scale.set(1 + sq * 0.5, 1 - sq, 1 + sq * 0.5);
    set(inout(clamp01((t - 0.16) / 0.85)));
    mat.emissiveIntensity = 0.06 + 0.7 * (1 - smooth(0.3, 1.1, t));
    if (t >= 1.4) { if (lead) { die.group.position.y = y0; die.group.scale.set(1, 1, 1); } set(1); return false; }
    return true;
  } };
}

registerShow('molten', { name: 'Molten', what: 'The number glows like fresh-poured metal, then cools to its resting colour.', play: hlMolten });
registerShow('gather', { name: 'Gather', what: 'The flakes or stars inside drift together and fill the number.', play: hlGather,
  clear: function (die) {
    (die._gathered || []).forEach(function (g) {
      g.pick.forEach(function (li) { die.flakes.setMatrixAt(li, die.flakeList[li].m); }); die.flakes.instanceMatrix.needsUpdate = true;
      if (g.im.parent) g.im.parent.remove(g.im); g.im.material.dispose(); g.im.dispose();
    });
    die._gathered = null; die._gatherUsed = null;
  } });
registerShow('enamel', { name: 'Enamel', what: 'Colour sweeps into the number and it rises out of the face in glossy enamel.', play: hlEnamel,
  clear: function (die) { (die._enamel || []).forEach(function (m) { if (m.parent) m.parent.remove(m); m.geometry.dispose(); m.material.dispose(); }); die._enamel = null; } });

// The show for a die's set, on every place its number shows; clearHighlight puts the die back as it
// was, for a preview that plays it again.
function highlight(st, die, fi, showKey) {
  var sh = SHOWS[showKey || kindOf(die.kind).show] || SHOWS.molten;
  var list = resultSlots(die, fi).map(function (sl, k) { return sh.play(st, die, sl.fi, sl.lay, k); });
  return { update: function (dt) {
    var on = false;
    for (var i = 0; i < list.length; i++) if (list[i]) { if (list[i].update(dt)) on = true; else list[i] = null; }
    return on;
  } };
}
function clearHighlight(die) {
  die.faces.forEach(function (fc) { if (fc.hl) { fc.hl.visible = false; fc.hl.material.uniforms.uK.value = 0; } });
  Object.keys(SHOWS).forEach(function (k) { if (SHOWS[k].clear) SHOWS[k].clear(die); });
}

// ---------- Inscribe: a point of light writes the number ----------
// A bright point hovers just over the face and travels the strokes in order, lifting between strokes,
// with a few fine sparks shed along its path. Behind it the number is etched in light: a hot thin line
// down each stroke with the colour of the glow along its flanks (the number's shape and stroke order come
// from numeralData). When the point has finished it
// flares and goes out, and the whole number settles to a soft glow. The point and its sparks are one
// Points draw; the etching is one layer over the face (about two draws a place).
//   th.burn   the colour of the light

// The point's journey as a table in time: where it is (face pixels), how far along the strokes it
// has written, and how high it hovers (it lifts between strokes). Strokes take time in proportion to
// their length; a hop between two takes a little of its own.
function inscribeTrack(lay, M) {
  var lens = lay.strokes.map(polyLen), total = lens.reduce(function (x, y) { return x + y; }, 0) || 1, n = lay.strokes.length, hop = n > 1 ? 0.05 : 0;
  var seg = [], t = 0, acc = 0;
  lay.strokes.forEach(function (pl, i) {
    var d = lens[i] / total * (1 - hop * (n - 1));
    seg.push({ t0: t, t1: t + d, pl: pl, p0: acc / total, p1: (acc + lens[i]) / total, hop: false }); t += d; acc += lens[i];
    if (i < n - 1) { seg.push({ t0: t, t1: t + hop, a: pl[pl.length - 1], b: lay.strokes[i + 1][0], p0: acc / total, p1: acc / total, hop: true }); t += hop; }
  });
  var X = new Float32Array(M), Y = new Float32Array(M), P = new Float32Array(M), Z = new Float32Array(M), s = 0;
  for (var j = 0; j < M; j++) {
    var x = j / (M - 1) * t;
    while (s < seg.length - 1 && seg[s].t1 < x) s++;
    var g = seg[s], f = g.t1 > g.t0 ? Math.max(0, Math.min(1, (x - g.t0) / (g.t1 - g.t0))) : 0;
    if (g.hop) { X[j] = g.a[0] + (g.b[0] - g.a[0]) * f; Y[j] = g.a[1] + (g.b[1] - g.a[1]) * f; Z[j] = Math.sin(Math.PI * f); }
    else {
      var want = f * polyLen(g.pl), pl = g.pl, run = 0, px = pl[0][0], py = pl[0][1];
      for (var k = 1; k < pl.length; k++) {
        var L = Math.hypot(pl[k][0] - pl[k - 1][0], pl[k][1] - pl[k - 1][1]);
        if (run + L >= want || k === pl.length - 1) { var u = L ? Math.max(0, Math.min(1, (want - run) / L)) : 0; px = pl[k - 1][0] + (pl[k][0] - pl[k - 1][0]) * u; py = pl[k - 1][1] + (pl[k][1] - pl[k - 1][1]) * u; break; }
        run += L;
      }
      X[j] = px; Y[j] = py; Z[j] = 0;
    }
    P[j] = g.p0 + (g.p1 - g.p0) * f;
  }
  return { x: X, y: Y, p: P, z: Z };
}

// ---------- Shaders ----------
var inscribeEtchVS = 'varying vec2 vUv; void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4( position, 1.0 ); }';
var inscribeEtchFS = [
  'uniform sampler2D uData; uniform float uP; uniform float uK; uniform float uSet; uniform float uSpill; uniform float uCover; uniform float uCool; uniform float uEr;',
  'uniform vec3 uHot; uniform vec3 uCol; uniform vec3 uGlow;',
  'varying vec2 vUv;',
  'void main() {',
  '  vec4 d = texture2D( uData, vUv );',
  '  float m = d.g, prog = d.r / max( d.g, 0.004 ), since = uP - prog;',
  '  float on = smoothstep( -0.008, 0.004, since ), lit = on * m, heat = exp( -max( since, 0.0 ) * uCool ) * step( 0.0, since );',
  // The fine line down the middle of each stroke: where the shape is still there a little way out on every side.
  '  float core = min( min( texture2D( uData, vUv + vec2( uEr, 0.0 ) ).g, texture2D( uData, vUv - vec2( uEr, 0.0 ) ).g ), min( texture2D( uData, vUv + vec2( 0.0, uEr ) ).g, texture2D( uData, vUv - vec2( 0.0, uEr ) ).g ) ) * on;',
  '  float h = d.b * ( 1.0 - m * 0.5 );',
  '  vec3 pen = mix( uCol, uHot, 0.3 + 0.7 * heat );',
  '  vec3 etched = pen * ( 0.7 + 1.3 * heat ) + uHot * core * ( 0.5 + 1.3 * heat );',
  '  vec3 soft = uGlow * ( 0.85 + 0.4 * core );',
  '  vec3 o = mix( etched, soft, uSet ) * lit + uCol * h * uSpill;',
  '  gl_FragColor = vec4( o * uK, clamp( ( lit + h * uSpill * 0.4 ) * uCover * uK, 0.0, 1.0 ) );',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');
var inscribePtVS = [
  'attribute float aSz; attribute float aA; attribute float aH; uniform float uScale; varying float vA; varying float vH;',
  'void main() {',
  '  vA = aA; vH = aH;',
  '  vec4 mv = modelViewMatrix * vec4( position, 1.0 );',
  '  gl_Position = projectionMatrix * mv;',
  '  gl_PointSize = aSz * uScale / max( -mv.z, 0.01 );',
  '  if ( aA < 0.002 ) gl_Position = vec4( 2.0, 2.0, 2.0, 1.0 );',
  '}'
].join('\n');
// The light is added (rgb) while a little of what is under it is covered (alpha), so the point still
// shows as a bright dot with a dim ring on a light face or page, and simply glows on a dark one.
var inscribePtFS = [
  'uniform vec3 uCol; uniform vec3 uHot; uniform float uDark; varying float vA; varying float vH;',
  'void main() {',
  '  vec2 q = gl_PointCoord * 2.0 - 1.0; float r = length( q );',
  '  if ( r > 1.0 ) discard;',
  '  float core = exp( -r * r * 16.0 ), halo = pow( max( 1.0 - r, 0.0 ), 2.2 );',
  '  float ray = vH * max( exp( -abs( q.x ) * 34.0 ) * ( 1.0 - abs( q.y ) ), exp( -abs( q.y ) * 34.0 ) * ( 1.0 - abs( q.x ) ) );',
  '  vec3 light = ( uHot * ( core * 1.6 + ray * 0.8 ) + uCol * halo * 0.8 ) * vA;',
  '  float dark = uDark * smoothstep( 1.0, 0.3, r ) * vA * ( 0.5 + 0.5 * vH );',
  '  gl_FragColor = vec4( light, dark );',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');

// ---------- The show ----------
function inscribeDrop(die, fi, corner) {
  if (!die._inscribe) return;
  die._inscribe = die._inscribe.filter(function (o) {
    if (o.fi !== fi || o.corner !== corner) return true;
    inscribeFree(o); return false;
  });
}
function inscribeFree(o) {
  o.meshes.forEach(function (m) { if (m.parent) m.parent.remove(m); if (!m.geometry.userData.shared) m.geometry.dispose(); m.material.dispose(); });
}
function inscribePlay(st, die, fi, lay, k) {
  lay = lay || die.faces[fi].lay;
  var T = st.T, fc = die.faces[fi], f = fc.f, th = die.th, hd = st.q.detail !== 'basic', size = die.size, pw = size / lay.s, wW = strokeW(lay) * pw;
  inscribeDrop(die, fi, lay.corner);
  // Over a pale face or clear glass, light added to white cannot show: cover a little, and keep the
  // settled glow darker than the page so the number still reads.
  var cover = Math.max(coverOf(die), die.see === 'glass' ? 0.5 : 0), col = new T.Color(th.burn), hot = new T.Color('#ffffff').lerp(col, 0.22), glow = new T.Color(mix(th.burn, '#ffffff', 0.3));
  col.multiplyScalar(1 - 0.3 * cover); glow.multiplyScalar(1 - 0.5 * cover);
  var WRITE = 0.86, SET = 0.5, END = WRITE + SET;
  // The etching.
  var em = new T.ShaderMaterial({ vertexShader: inscribeEtchVS, fragmentShader: inscribeEtchFS, transparent: true, depthWrite: false, polygonOffset: true, polygonOffsetFactor: -3, polygonOffsetUnits: -3,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.ZeroFactor, blendDstAlpha: T.OneFactor,
    uniforms: { uData: { value: numeralData(T, die, fi, lay).tex }, uP: { value: 0 }, uK: { value: 0 }, uSet: { value: 0 }, uSpill: { value: 0 }, uCover: { value: cover }, uCool: { value: 4.5 }, uEr: { value: strokeW(lay) * 0.3 / lay.cv },
      uHot: { value: hot }, uCol: { value: col }, uGlow: { value: glow } } });
  var etch = new T.Mesh(overlayGeo(T, die.body, die.sd, die.atl, fi), em); etch.renderOrder = 4; fc.piece.add(etch);
  // The point and its sparks: one Points, index 0 the point.
  var M = 120, tr = inscribeTrack(lay, M), n = hd ? 34 : 16, N = n + 1, rnd = mulberry(fi * 29 + (lay.corner || 0) * 7 + 5);
  var R = f.right, U = f.up, Nv = fc.n, cv2 = lay.cv / 2, loc = new Float32Array(M * 3);
  for (var j = 0; j < M; j++) {
    var X = (tr.x[j] - cv2) * pw, Y = (cv2 - tr.y[j]) * pw, Z = size * 0.05 + tr.z[j] * size * 0.1;
    loc[j * 3] = R.x * X + U.x * Y + Nv.x * Z; loc[j * 3 + 1] = R.y * X + U.y * Y + Nv.y * Z; loc[j * 3 + 2] = R.z * X + U.z * Y + Nv.z * Z;
  }
  var pos = new Float32Array(N * 3), sz = new Float32Array(N), al = new Float32Array(N), hd1 = new Float32Array(N); hd1[0] = 1;
  var sb = new Float32Array(n * 3), sv = new Float32Array(n * 3), born = new Float32Array(n), life = new Float32Array(n), big = new Float32Array(n);
  for (var i = 0; i < n; i++) {
    var tb = (i + rnd() * 0.9) / n, ff = Math.min(1, tb) * (M - 1), i0 = Math.min(M - 2, Math.floor(ff)), fr = ff - i0;
    for (var c = 0; c < 3; c++) sb[i * 3 + c] = loc[i0 * 3 + c] * (1 - fr) + loc[(i0 + 1) * 3 + c] * fr;
    var a0 = rnd() * 6.283, sp = size * (0.06 + rnd() * 0.14), up = size * (0.1 + rnd() * 0.3);
    sv[i * 3] = R.x * Math.cos(a0) * sp + U.x * Math.sin(a0) * sp + Nv.x * up; sv[i * 3 + 1] = R.y * Math.cos(a0) * sp + U.y * Math.sin(a0) * sp + Nv.y * up; sv[i * 3 + 2] = R.z * Math.cos(a0) * sp + U.z * Math.sin(a0) * sp + Nv.z * up;
    born[i] = tb * WRITE; life[i] = 0.28 + rnd() * 0.3; big[i] = wW * (0.3 + rnd() * 0.5); hd1[i + 1] = 0;
  }
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.BufferAttribute(pos, 3)); geo.setAttribute('aSz', new T.BufferAttribute(sz, 1)); geo.setAttribute('aA', new T.BufferAttribute(al, 1)); geo.setAttribute('aH', new T.BufferAttribute(hd1, 1));
  var pm = new T.ShaderMaterial({ vertexShader: inscribePtVS, fragmentShader: inscribePtFS, transparent: true, depthWrite: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor,
    uniforms: { uScale: { value: st.H * 0.5 * st.renderer.getPixelRatio() / TAN }, uCol: { value: col }, uHot: { value: hot }, uDark: { value: cover > 0 ? 0.5 : 0.34 } } });
  var pts = new T.Points(geo, pm); pts.frustumCulled = false; pts.renderOrder = 6; fc.piece.add(pts);
  (die._inscribe = die._inscribe || []).push({ fi: fi, corner: lay.corner, meshes: [etch, pts] });
  var u = em.uniforms, t = 0, headS = wW * 3.8, ph = rnd() * 6;
  function place(x) {
    var ff = Math.min(1, x) * (M - 1), i0 = Math.min(M - 2, Math.floor(ff)), fr = ff - i0;
    pos[0] = loc[i0 * 3] * (1 - fr) + loc[(i0 + 1) * 3] * fr; pos[1] = loc[i0 * 3 + 1] * (1 - fr) + loc[(i0 + 1) * 3 + 1] * fr; pos[2] = loc[i0 * 3 + 2] * (1 - fr) + loc[(i0 + 1) * 3 + 2] * fr;
    return tr.p[i0] * (1 - fr) + tr.p[i0 + 1] * fr;
  }
  function frame() {
    var x = t / WRITE, p = place(x), done = t >= WRITE, since = t - WRITE;
    // The point: grows in, shines steadily while it writes, flares as it finishes and goes out.
    var ramp = smooth(0, 0.07, t), flare = done ? 1 + 1.4 * Math.sin(Math.PI * Math.min(1, since / 0.2)) : 1, fade = done ? 1 - smooth(0.08, 0.26, since) : 1;
    sz[0] = headS * ramp * flare * (0.94 + 0.06 * Math.sin(t * 47 + ph)); al[0] = ramp * fade;
    for (var i = 0; i < n; i++) {
      var age = t - born[i], o = i + 1;
      if (age < 0 || age > life[i]) { al[o] = 0; sz[o] = 0; continue; }
      var q = age / life[i], d = (1 - Math.exp(-3 * age)) / 3;
      pos[o * 3] = sb[i * 3] + sv[i * 3] * d * 3; pos[o * 3 + 1] = sb[i * 3 + 1] + sv[i * 3 + 1] * d * 3; pos[o * 3 + 2] = sb[i * 3 + 2] + sv[i * 3 + 2] * d * 3;
      al[o] = (1 - q) * (1 - q) * 0.95; sz[o] = big[i] * (1 - 0.5 * q);
    }
    geo.attributes.position.needsUpdate = true; geo.attributes.aSz.needsUpdate = true; geo.attributes.aA.needsUpdate = true;
    // The etching: follows the point, then the whole number settles to a soft glow.
    var set = smooth(0, SET, since);
    u.uP.value = done ? 1.02 : p; u.uSet.value = set; u.uSpill.value = 0.12 * p + 0.88 * set;
    u.uK.value = smooth(0, 0.06, t) * (1.1 - 0.3 * set);
  }
  frame();
  return { update: function (dt) {
    t += dt; frame();
    if (t >= END) { al[0] = 0; for (var i = 1; i < N; i++) al[i] = 0; geo.attributes.aA.needsUpdate = true; pts.visible = false; return false; }
    return true;
  } };
}

registerShow('inscribe', { name: 'Inscribe', what: 'A point of light writes the number stroke by stroke, then the number glows softly.', play: inscribePlay,
  clear: function (die) { (die._inscribe || []).forEach(inscribeFree); die._inscribe = null; } });

// ---------- Kindle: light rises behind the number like a candle behind a window ----------
// One layer over the face. A pool of warm light comes up from the bottom of the number, with the
// number itself first a dark shape against it; then the number takes the light and burns bright,
// the flame flickers, and everything settles to a low ember. The face's own colours tint the light
// (a coloured pane glows its own colour). Warm by default; a set whose glow is not warm (cold fire)
// keeps more of its own colour. One draw a place, nothing per frame but a few numbers.
//   th.burn    pulls the light towards the set's own glow colour
//   th.flame   optional: the light's colour outright

var kindleVS = 'varying vec2 vUv; void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4( position, 1.0 ); }';
var kindleFS = [
  'uniform sampler2D uData; uniform sampler2D uMap; uniform vec4 uCell; uniform vec2 uC; uniform vec2 uR;',
  'uniform float uT; uniform float uRise; uniform float uField; uniform float uNum; uniform float uNumK; uniform float uHeat; uniform float uCover; uniform float uFlick; uniform float uK; uniform float uTint;',
  'uniform vec3 uCore; uniform vec3 uMid; uniform vec3 uEmber;',
  'varying vec2 vUv;',
  'void main() {',
  '  vec4 d = texture2D( uData, vUv );',
  '  float m = d.g, halo = d.b;',
  '  vec2 q = ( vUv - uC ) / uR;',
  // The flame's slow sway moves the pool a little to either side.
  '  q.x += 0.06 * sin( uT * 7.3 + q.y * 1.7 );',
  '  float r = length( q * vec2( 1.0, 1.12 ) );',
  '  float pool = smoothstep( 1.3, 0.0, r );',
  '  pool = pool * pool * ( 3.0 - 2.0 * pool );',
  // The light rises: a front climbs from below the number to above it, soft-edged.
  '  float front = mix( -1.4, 1.6, uRise );',
  '  float below = 1.0 - smoothstep( front - 0.6, front + 0.1, q.y );',
  '  float f = max( pool, halo * 1.2 ) * below * uField * uFlick;',
  '  vec3 warm = mix( uMid, uCore, smoothstep( 0.4, 1.3, f ) );',
  // A coloured pane takes the light as its own colour.
  '  vec3 face = texture2D( uMap, uCell.xy + vUv * uCell.zw ).rgb;',
  '  vec3 tint = face / max( max( face.r, max( face.g, face.b ) ), 0.08 );',
  '  vec3 lightCol = mix( warm, tint * max( warm.r, max( warm.g, warm.b ) ), uTint );',
  // The number blocks the light until the flame reaches it, so it is first a dark shape against it.
  '  float open = 1.0 - 0.94 * m * ( 1.0 - uNum );',
  '  vec3 light = lightCol * f * open;',
  '  vec3 num = mix( uEmber, uCore, uHeat ) * m * uNum * uNumK * ( 0.6 + 0.6 * pool ) * uFlick;',
  '  vec3 spill = mix( uEmber, uMid, uHeat ) * halo * uNum * ( 0.18 + 0.4 * uHeat );',
  '  vec3 o = light + num + spill;',
  '  float a = clamp( ( f * open * 0.9 + m * uNum * 0.8 + halo * uNum * 0.2 ) * uCover, 0.0, 1.0 );',
  '  gl_FragColor = vec4( o * uK, a * uK );',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');

function kindleDrop(die, fi, corner) {
  if (!die._kindle) return;
  die._kindle = die._kindle.filter(function (o) {
    if (o.fi !== fi || o.corner !== corner) return true;
    if (o.mesh.parent) o.mesh.parent.remove(o.mesh); o.mesh.material.dispose(); return false;
  });
}
// The light's three colours: a candle's, pulled towards the set's own glow by how un-warm that glow is.
function kindleColours(T, th) {
  var burn = new T.Color(th.burn), hsl = { h: 0, s: 0, l: 0 }; burn.getHSL(hsl);
  var warm = hsl.h < 0.17 || hsl.h > 0.94, w = th.flame ? 1 : warm ? 0.22 : 0.72;
  if (th.flame) { var fl = new T.Color(th.flame); return { core: fl.clone().lerp(new T.Color(0xffffff), 0.5), mid: fl, ember: fl.clone().multiplyScalar(0.55) }; }
  return {
    core: new T.Color('#ffe3a6').lerp(burn.clone().lerp(new T.Color(0xffffff), 0.6), w),
    mid: new T.Color('#ff9a3a').lerp(burn, w),
    ember: new T.Color('#c93c16').lerp(burn.clone().multiplyScalar(0.8), w)
  };
}
function kindlePlay(st, die, fi, lay, k) {
  lay = lay || die.faces[fi].lay;
  var T = st.T, fc = die.faces[fi], th = die.th, cv = lay.cv, atl = die.atl, cl = kindleColours(T, th);
  // Over a pale face or clear glass, light added to white cannot show: cover a little, and keep the lit
  // number deeper than the face so it still reads.
  var cover = Math.max(coverOf(die), die.see === 'glass' ? 0.5 : 0);
  if (cover > 0) { cl.core.lerp(cl.mid, 0.6); cl.mid.multiplyScalar(1 - 0.3 * cover); cl.ember.multiplyScalar(1 - 0.5 * cover); }
  kindleDrop(die, fi, lay.corner);
  // Where the number lies on the face, in the layer's own 0..1 frame.
  var x0 = cv, y0 = cv, x1 = 0, y1 = 0, w = strokeW(lay);
  lay.strokes.forEach(function (pl) { pl.forEach(function (p) { x0 = Math.min(x0, p[0]); y0 = Math.min(y0, p[1]); x1 = Math.max(x1, p[0]); y1 = Math.max(y1, p[1]); }); });
  if (lay.label === '6' || lay.label === '9') y1 = Math.max(y1, lay.cy + lay.h * 0.68 + w * 0.6);
  var cx = (x0 + x1) / 2 / cv, cy = 1 - (y0 + y1) / 2 / cv, rx = (x1 - x0) / 2 / cv * 1.5 + 0.05, ry = (y1 - y0) / 2 / cv * 1.5 + 0.05;
  var cell = lay.cell || { x: 0, y: 0, W: cv, H: cv };
  var mat = new T.ShaderMaterial({ vertexShader: kindleVS, fragmentShader: kindleFS, transparent: true, depthWrite: false, polygonOffset: true, polygonOffsetFactor: -3, polygonOffsetUnits: -3,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.ZeroFactor, blendDstAlpha: T.OneFactor,
    uniforms: { uData: { value: numeralData(T, die, fi, lay).tex }, uMap: { value: atl.map }, uCell: { value: new T.Vector4(cell.x / cell.W, 1 - (cell.y + cv) / cell.H, cv / cell.W, cv / cell.H) },
      uC: { value: new T.Vector2(cx, cy) }, uR: { value: new T.Vector2(rx, ry) }, uT: { value: 0 }, uRise: { value: 0 }, uField: { value: 0 }, uNum: { value: 0 }, uNumK: { value: 1.4 }, uHeat: { value: 1 },
      uCover: { value: cover }, uFlick: { value: 1 }, uK: { value: 1 }, uTint: { value: 0.55 }, uCore: { value: cl.core }, uMid: { value: cl.mid }, uEmber: { value: cl.ember } } });
  var mesh = new T.Mesh(overlayGeo(T, die.body, die.sd, die.atl, fi), mat); mesh.renderOrder = 4; fc.piece.add(mesh);
  (die._kindle = die._kindle || []).push({ fi: fi, corner: lay.corner, mesh: mesh });
  var u = mat.uniforms, t = 0, END = 1.45, ph = (fi * 1.7 + (lay.corner || 0)) % 6;
  function frame() {
    u.uT.value = t + ph;
    u.uRise.value = smooth(0.02, 0.78, t);
    u.uField.value = 1.0 * smooth(0, 0.5, t) * (1 - 0.7 * smooth(0.95, END, t));
    u.uNum.value = smooth(0.42, 0.92, t);
    u.uHeat.value = 1 - 0.62 * smooth(0.92, END, t);
    u.uNumK.value = 1.4 - 0.85 * smooth(0.92, END, t);
    // A candle: three quick wobbles that never line up, strongest while it burns, calm as an ember.
    var a = smooth(0.12, 0.5, t) * (1 - 0.8 * smooth(0.95, END, t)), tt = t + ph;
    u.uFlick.value = 1 + a * (0.1 * Math.sin(tt * 23.3) + 0.07 * Math.sin(tt * 37.1 + 2.1) + 0.05 * Math.sin(tt * 61.7 + 4.2));
  }
  frame();
  return { update: function (dt) {
    t += dt; frame();
    if (t >= END) { u.uFlick.value = 1; return false; }
    return true;
  } };
}

registerShow('kindle', { name: 'Kindle', what: 'Light rises behind the number like a candle behind a window, then settles to an ember.', play: kindlePlay,
  clear: function (die) { (die._kindle || []).forEach(function (o) { if (o.mesh.parent) o.mesh.parent.remove(o.mesh); o.mesh.material.dispose(); }); die._kindle = null; } });

// ---------- Overgrow: a fey vine wraps the number and lights it ----------
// A glowing vine grows along the number, first stroke to last, winding round each stroke the way ivy
// winds round a post: over the stroke, down behind it into the face, and over again. The number lights
// up just behind the growing tip, leaves unfurl with glowing veins, a flower opens at each stroke's end
// and sheds motes of light, then it all settles to a soft glow. The vine is built once per layout in the
// face's own pixel frame (X right, Y up, Z out of the face), so any die size or turn reuses it, and it
// grows on the graphics card: every ring, leaf and petal carries the moment it starts and the point it
// grows from, so a frame costs a few numbers. A soft ribbon under the vine, shifted away from the sun,
// keeps it sitting on the face instead of floating over it.
//   th.fey || th.grow   the plant's colour; it glows in th.burn

var overgrowCache = {};

// A stroke with its corners rounded off, so a tube can turn them: a short curve in place of each vertex.
function overgrowRound(pl, r) {
  var out = [pl[0]];
  for (var i = 1; i < pl.length - 1; i++) {
    var a = pl[i - 1], v = pl[i], b = pl[i + 1], ax = a[0] - v[0], ay = a[1] - v[1], bx = b[0] - v[0], by = b[1] - v[1];
    var la = Math.hypot(ax, ay), lb = Math.hypot(bx, by);
    if (la < 1e-6 || lb < 1e-6) { out.push(v); continue; }
    var d = Math.min(r, la * 0.45, lb * 0.45), p1x = v[0] + ax / la * d, p1y = v[1] + ay / la * d, p2x = v[0] + bx / lb * d, p2y = v[1] + by / lb * d;
    for (var k = 0; k <= 4; k++) { var t = k / 4, u = 1 - t; out.push([u * u * p1x + 2 * u * t * v[0] + t * t * p2x, u * u * p1y + 2 * u * t * v[1] + t * t * p2y]); }
  }
  out.push(pl[pl.length - 1]);
  return out;
}
// Points every ds along a polyline, as [x, y, distance from the start], and its length.
function overgrowResample(pts, ds) {
  var cum = [0], i;
  for (i = 1; i < pts.length; i++) cum.push(cum[i - 1] + Math.hypot(pts[i][0] - pts[i - 1][0], pts[i][1] - pts[i - 1][1]));
  var L = cum[cum.length - 1], n = Math.max(2, Math.round(L / ds)), out = [], seg = 0;
  for (i = 0; i <= n; i++) {
    var s = L * i / n;
    while (seg < pts.length - 2 && cum[seg + 1] < s) seg++;
    var span = cum[seg + 1] - cum[seg], f = span > 1e-9 ? (s - cum[seg]) / span : 0;
    out.push([pts[seg][0] + (pts[seg + 1][0] - pts[seg][0]) * f, pts[seg][1] + (pts[seg + 1][1] - pts[seg][1]) * f, s]);
  }
  return { pts: out, L: L };
}

// Growing geometry accumulated in plain arrays: position, normal, colour (r shade, g what it is: 0 stem,
// 1 leaf, 2 petal, 3 a flower's heart; b highlight), the point each piece grows from (aC), and when and how it grows (aG: start, span, overshoot).
function overgrowBuf(ribbon) {
  var b = { pos: [], nor: [], col: [], ac: [], ag: [], ah: [], idx: [], n: 0 };
  b.v = function (x, y, z, nx, ny, nz, c0, c1, c2, c3, cx, cy, cz, g0, g1, g2, h) {
    b.pos.push(x, y, z); b.nor.push(nx, ny, nz);
    if (ribbon) b.col.push(c0, c1, c2, c3); else b.col.push(c0, c1, c2);
    b.ac.push(cx, cy, cz); b.ag.push(g0, g1, g2); b.ah.push(h || 0);
    return b.n++;
  };
  b.tri = function (a, c, d) { b.idx.push(a, c, d); };
  b.geo = function (T) {
    var g = new T.BufferGeometry(), big = b.n > 65000;
    g.setAttribute('position', new T.Float32BufferAttribute(b.pos, 3));
    g.setAttribute('normal', new T.Float32BufferAttribute(b.nor, 3));
    g.setAttribute('color', new T.Float32BufferAttribute(b.col, ribbon ? 4 : 3));
    g.setAttribute('aC', new T.Float32BufferAttribute(b.ac, 3));
    g.setAttribute('aG', new T.Float32BufferAttribute(b.ag, 3));
    if (ribbon) g.setAttribute('aH', new T.Float32BufferAttribute(b.ah, 1));
    g.setIndex(big ? new T.Uint32BufferAttribute(b.idx, 1) : new T.Uint16BufferAttribute(b.idx, 1));
    g.computeBoundingSphere(); share(g);
    return g;
  };
  return b;
}

// A tube along a run of rings. R[i] is [x, y, z, radius, progress, shade]; span is how long the tip
// takes to thicken; the shadow ribbon follows the same rings, and fades where the tube dips behind the
// face. Returns nothing: it writes into B and S.
function overgrowTube(B, S, R, sides, span, leafness) {
  var n = R.length, i, j;
  for (i = 0; i < n; i++) {
    var a = R[Math.max(0, i - 1)], c = R[Math.min(n - 1, i + 1)], r = R[i];
    var tx = c[0] - a[0], ty = c[1] - a[1], tz = c[2] - a[2], tl = Math.hypot(tx, ty, tz) || 1; tx /= tl; ty /= tl; tz /= tl;
    // A ring across the tube; a tube standing straight up off the face (a seed, a flower's heart) rings the face's own axes.
    var bl = Math.hypot(tx, ty), bx = 1, by = 0, nx = 0, ny = 1, nz = 0;
    if (bl > 1e-4) { bx = ty / bl; by = -tx / bl; nx = by * tz; ny = -bx * tz; nz = bx * ty - by * tx; }
    var apex = r[3] < 0.02;
    for (j = 0; j < sides; j++) {
      var an = j / sides * Math.PI * 2, cs = Math.cos(an), sn = Math.sin(an);
      var ox = cs * nx + sn * bx, oy = cs * ny + sn * by, oz = cs * nz;
      // Darker underneath: the part of the tube against the face gets no light.
      var shade = r[5] * (0.52 + 0.48 * (0.5 + 0.5 * cs));
      if (apex) B.v(r[0], r[1], r[2], i ? tx : -tx, i ? ty : -ty, i ? tz : -tz, shade, leafness, 0, 0, r[0], r[1], r[2], r[4], span, 0);
      else B.v(r[0] + ox * r[3], r[1] + oy * r[3], r[2] + oz * r[3], ox, oy, oz, shade, leafness, 0.25 * (0.5 + 0.5 * cs), 0, r[0], r[1], r[2], r[4], span, 0);
    }
    var w = r[3] * 1.9 + 1.2, sh = 0.34 * clamp01((r[2] + r[3]) / (r[3] * 2 + 1e-6));
    S.v(r[0] - bx * w, r[1] - by * w, 0.3, 0, 0, 1, 0.02, 0.014, 0.03, 0, r[0], r[1], 0.3, r[4], span, 0, r[2]);
    S.v(r[0], r[1], 0.3, 0, 0, 1, 0.02, 0.014, 0.03, apex ? 0 : sh, r[0], r[1], 0.3, r[4], span, 0, r[2]);
    S.v(r[0] + bx * w, r[1] + by * w, 0.3, 0, 0, 1, 0.02, 0.014, 0.03, 0, r[0], r[1], 0.3, r[4], span, 0, r[2]);
  }
  var base = B.n - n * sides, sb = S.n - n * 3;
  for (i = 0; i < n - 1; i++) {
    for (j = 0; j < sides; j++) {
      var a0 = base + i * sides + j, b0 = base + i * sides + (j + 1) % sides, c0 = a0 + sides, d0 = b0 + sides;
      B.tri(a0, b0, c0); B.tri(b0, d0, c0);
    }
    var s0 = sb + i * 3;
    S.tri(s0, s0 + 3, s0 + 1); S.tri(s0 + 1, s0 + 3, s0 + 4); S.tri(s0 + 1, s0 + 4, s0 + 2); S.tri(s0 + 2, s0 + 4, s0 + 5);
  }
}

// A leaf: a pointed oval folded along its rib and curling up at the tip, on a short stalk at (bx, by, bz),
// pointing along angle ang. The same outline is laid flat on the face as its shadow. kind is 1 for a
// leaf, 2 for a petal; cup lifts the tip further (a petal cups).
function overgrowLeaf(B, S, bx, by, bz, ang, len, wid, rows, p0, span, rnd, zs, kind, cup) {
  var cols = 3, ca = Math.cos(ang), sa = Math.sin(ang), first = B.n, sfirst = S.n, r, c;
  kind = kind || 1; cup = cup || 1;
  var lift = len * (0.1 + rnd() * 0.07) * zs * cup, curl = len * (0.16 + rnd() * 0.12) * zs * cup, bend = (rnd() - 0.5) * 0.5;
  for (r = 0; r <= rows; r++) {
    var u = r / rows, wd = Math.sin(Math.PI * Math.pow(u, 0.72)) * (1 - 0.34 * u);
    for (c = 0; c < cols; c++) {
      var v = (c - 1), vv = v * wid * wd, uu = u * len + vv * bend * u;
      var x = bx + ca * uu - sa * vv, y = by + sa * uu + ca * vv;
      var z = bz + lift * u + curl * u * u + (1 - Math.abs(v)) * 0.34 * wid * wd * zs;
      var tone = 0.78 + (1 - Math.abs(v)) * 0.34 + u * 0.1, hi = (1 - Math.abs(v)) * 0.5 * Math.sin(Math.PI * u);
      B.v(x, y, z, 0, 0, 1, tone, kind, hi, 0, bx, by, bz, p0, span, 1);
      S.v(x, y, 0.3, 0, 0, 1, 0.02, 0.014, 0.03, wd > 0.02 ? 0.3 - Math.abs(v) * 0.14 : 0, bx, by, 0.3, p0, span, 1, z);
    }
  }
  for (r = 0; r < rows; r++) {
    for (c = 0; c < cols - 1; c++) {
      var a = first + r * cols + c, b = a + 1, d = a + cols, e = d + 1, sa0 = sfirst + r * cols + c, sb0 = sa0 + 1, sd0 = sa0 + cols, se0 = sd0 + 1;
      B.tri(a, d, b); B.tri(b, d, e); S.tri(sa0, sd0, sb0); S.tri(sb0, sd0, se0);
    }
  }
  // Normals from the folded surface, so the rib catches light along one side.
  var acc = [], i;
  for (i = 0; i < (rows + 1) * cols; i++) acc.push(0, 0, 0);
  for (r = 0; r < rows; r++) for (c = 0; c < cols - 1; c++) {
    var q = [r * cols + c, r * cols + c + 1, (r + 1) * cols + c, (r + 1) * cols + c + 1];
    [[0, 2, 1], [1, 2, 3]].forEach(function (t) {
      var i0 = (first + q[t[0]]) * 3, i1 = (first + q[t[1]]) * 3, i2 = (first + q[t[2]]) * 3, P = B.pos;
      var e1x = P[i1] - P[i0], e1y = P[i1 + 1] - P[i0 + 1], e1z = P[i1 + 2] - P[i0 + 2], e2x = P[i2] - P[i0], e2y = P[i2 + 1] - P[i0 + 1], e2z = P[i2 + 2] - P[i0 + 2];
      var nx = e1y * e2z - e1z * e2y, ny = e1z * e2x - e1x * e2z, nz = e1x * e2y - e1y * e2x;
      t.forEach(function (k) { acc[q[k] * 3] += nx; acc[q[k] * 3 + 1] += ny; acc[q[k] * 3 + 2] += nz; });
    });
  }
  for (i = 0; i < (rows + 1) * cols; i++) {
    var l = Math.hypot(acc[i * 3], acc[i * 3 + 1], acc[i * 3 + 2]) || 1, o = (first + i) * 3;
    B.nor[o] = acc[i * 3] / l; B.nor[o + 1] = acc[i * 3 + 1] / l; B.nor[o + 2] = acc[i * 3 + 2] / l;
  }
}

// A tendril: a thin tube that leaves the vine and winds into a spiral, growing as it goes. dir is +1 or
// -1 for the way it turns. Starts at radius r0 (the vine's, when it continues the vine) and tapers.
function overgrowCurl(B, S, bx, by, bz, ang, dir, R0, r0, rEnd, p0, pSpan, sides, turns, shade, zs) {
  var Phi = Math.PI * 2 * turns, steps = Math.max(8, Math.ceil(Phi / 0.42)), x = bx, y = by, h = ang, ring = [], len = 0;
  for (var i = 0; i <= steps; i++) {
    var f = i / steps, rho = R0 * (1 - 0.78 * f), dphi = Phi / steps;
    var rr = r0 + (rEnd - r0) * Math.pow(f, 0.8);
    ring.push([x, y, bz + zs * (0.3 * r0 * f + 1.1 * r0 * f * f), i === steps ? 0 : rr, p0 + pSpan * f, shade]);
    h += dir * dphi; x += Math.cos(h) * rho * dphi; y += Math.sin(h) * rho * dphi; len += rho * dphi;
  }
  overgrowTube(B, S, ring, sides, 0.035, 0);
  return len;
}

// The whole number's growth for one layout, as { vine, shade, reach, motes }; cached. motes are points
// in the face frame, each with the moment the growth reaches it, for the light the flowers shed.
function overgrowShape(T, lay, hd) {
  var w = strokeW(lay), cv2 = lay.cv / 2, seed = Math.round(lay.cv * 7 + lay.h * 13 + lay.cx * 3 + lay.cy * 5 + (lay.rot || 0) * 101 + String(lay.label).charCodeAt(0) * 131 + (lay.corner == null ? 0 : lay.corner * 17));
  var key = [lay.cv, lay.label, lay.h.toFixed(2), lay.wk, lay.cx.toFixed(1), lay.cy.toFixed(1), (lay.rot || 0).toFixed(3), hd ? 1 : 0, lay.strokes.length, seed].join('|');
  if (overgrowCache[key]) return overgrowCache[key];
  // A d4's numbers sit on faces seen at a slant, where anything raised off the face looks several times taller
  // than it is: keep the growth flatter and finer there.
  var flat = lay.corner != null, zs = flat ? 0.4 : 1, rT = w * (flat ? 0.18 : 0.21), sides = hd ? 7 : 5, rnd = mulberry(seed);
  // The wrap: one turn every lam along a stroke, swinging A to either side of it, its middle zc above the face.
  var lam = w * 4.0, A = w * 0.58, zc = A * 0.22, ds = Math.min(Math.max(rT * 1.1, 1.5), lam / 14) * (hd ? 1 : 1.4), paths = [], total = 0;
  lay.strokes.forEach(function (pl) {
    var S = overgrowResample(overgrowRound(pl, w * 0.8), ds);
    // Into the face frame, with the tangent at every point.
    S.pts = S.pts.map(function (p) { return [p[0] - cv2, cv2 - p[1], p[2]]; });
    paths.push({ S: S, off: total }); total += S.L;
  });
  total = Math.max(total, 1);
  var B = overgrowBuf(false), SH = overgrowBuf(true), reach = 1, leafSpan = 0.22, tubeSpan = 0.045, motes = [], flowers = 0;
  function upTo(p) { if (p > reach) reach = p; }
  // Leaves and flowers keep off the number: none may cover another stroke or point into the number's middle.
  // skip leaves out the end of the stroke a flower grows from: stroke pi, arc length outside s0..s1.
  var mx = lay.cx - cv2, my = cv2 - lay.cy, every = [];
  paths.forEach(function (P, pi) { P.S.pts.forEach(function (p, i) { if (i % 2 === 0) every.push([p[0], p[1], p[2], pi]); }); });
  function clear(x, y, r, skip) {
    for (var i = 0; i < every.length; i++) {
      var p = every[i];
      if (skip && p[3] === skip.pi && (p[2] < skip.s0 || p[2] > skip.s1)) continue;
      if (Math.hypot(p[0] - x, p[1] - y) < r) return false;
    }
    return true;
  }
  // A flower: five cupped petals round a bright heart, opening once the vine has passed.
  // A bud of light: a small bright bead on the vine.
  function overgrowBud(x, y, z, rb, p) {
    var rl = [];
    for (var j = 0; j <= 4; j++) { var fa = j / 4 * Math.PI; rl.push([x, y, z + Math.cos(fa) * rb, j === 0 || j === 4 ? 0 : Math.sin(fa) * rb, p + j * 0.004, 1]); }
    overgrowTube(B, SH, rl, 5, 0.06, 3); upTo(p + 0.08);
  }
  function flower(x, y, z, p, ang) {
    var Lp = w * (0.72 + rnd() * 0.1), rows = hd ? 4 : 3;
    for (var q = 0; q < 5; q++) overgrowLeaf(B, SH, x, y, z, ang + q * Math.PI * 0.4 + (rnd() - 0.5) * 0.2, Lp, Lp * 0.56, rows, p + q * 0.012, 0.2, rnd, zs, 2, 1.7);
    var rb = w * 0.3, rl = [];
    for (var j = 0; j <= 4; j++) { var fa = j / 4 * Math.PI; rl.push([x, y, z + rb * 0.4 + Math.cos(fa) * rb * 0.5, j === 0 || j === 4 ? 0 : Math.sin(fa) * rb, p + 0.07 + j * 0.003, 1]); }
    overgrowTube(B, SH, rl, hd ? 7 : 5, 0.08, 3);
    upTo(p + 0.27);
    for (j = 0; j < 4; j++) motes.push([x, y, z, p + 0.1 + j * 0.03]);
    flowers++;
  }
  paths.forEach(function (P, pi) {
    var pts = P.S.pts, L = P.S.L, n = pts.length, ring = [], ph = rnd() * 6.28, turn = rnd() < 0.5 ? 1 : -1, closed = Math.hypot(pts[0][0] - pts[n - 1][0], pts[0][1] - pts[n - 1][1]) < w * 1.2;
    var tang = [], i;
    for (i = 0; i < n; i++) {
      var a = pts[Math.max(0, i - 1)], c = pts[Math.min(n - 1, i + 1)], tl = Math.hypot(c[0] - a[0], c[1] - a[1]) || 1;
      tang.push([(c[0] - a[0]) / tl, (c[1] - a[1]) / tl]);
    }
    for (i = 0; i < n; i++) {
      var s = pts[i][2], env = smooth(0, w * 1.3, s) * smooth(0, w * 1.3, L - s), phi = turn * Math.PI * 2 * s / lam + ph, e = A * env * Math.sin(phi);
      var x = pts[i][0] - tang[i][1] * e, y = pts[i][1] + tang[i][0] * e, z = zs * (rT * 0.95 * (1 - env) + env * (zc + A * Math.cos(phi)));
      // Thin where it starts, a thumb thinner again near a tip, to a point where a closed stroke meets itself.
      var rad = rT * (1 + 0.08 * Math.sin(s / (w * 0.36))) * (0.45 + 0.55 * smooth(0, w * 1.4, s));
      var tipFrac = smooth(0, w * 2.2, L - s); rad *= closed ? (0.15 + 0.85 * tipFrac) : (0.6 + 0.4 * tipFrac);
      if (i === n - 1 && closed) rad = 0;
      ring.push([x, y, z, rad, (P.off + s) / total, 0.82 + 0.18 * Math.sin(s / (w * 0.9) + ph), phi, env]);
    }
    overgrowTube(B, SH, ring, sides, tubeSpan, 0);
    // Leaves, tendrils and buds of light come where the vine comes round the side of the stroke and
    // touches the face, pointing away from the number so it stays clear.
    var nth = 0;
    for (i = 1; i < n - 1; i++) {
      var a0 = Math.abs(Math.sin(ring[i - 1][6])), a1 = Math.abs(Math.sin(ring[i][6])), a2 = Math.abs(Math.sin(ring[i + 1][6]));
      if (!(a1 >= a0 && a1 > a2) || ring[i][7] < 0.6) continue;
      var R = ring[i], side = Math.sin(R[6]) > 0 ? 1 : -1, slot = nth++ % 3, p = R[4] + 0.004;
      var ang = Math.atan2(tang[i][1], tang[i][0]) + side * (1.0 + rnd() * 0.3);
      if (slot === 2 && nth % 2) { overgrowBud(R[0], R[1], R[2] + R[3] * 0.6, w * 0.17, p + 0.02); continue; }
      if (slot === 2 && !hd) continue;
      var Lf = w * (1.45 + rnd() * 0.35), tx = R[0] + Math.cos(ang) * Lf * 0.8, ty = R[1] + Math.sin(ang) * Lf * 0.8;
      if (Math.hypot(tx - mx, ty - my) < Math.hypot(R[0] - mx, R[1] - my) || !clear(tx, ty, w * 1.05, null) || !clear(R[0] + Math.cos(ang) * Lf * 0.45, R[1] + Math.sin(ang) * Lf * 0.45, w * 0.8, null)) continue;
      if (slot === 2) overgrowCurl(B, SH, R[0], R[1], R[2], ang, -side, w * (0.55 + rnd() * 0.2), rT * 0.5, rT * 0.12, p, 0.1, 5, 1.1, 0.9, zs);
      else overgrowLeaf(B, SH, R[0], R[1], R[2] - rT * 0.1, ang, Lf, Lf * 0.42, hd ? 5 : 4, p, leafSpan, rnd, zs);
      if (slot === 0 && nth % 3 === 1) motes.push([R[0], R[1], R[2], p + 0.05]);
    }
    // A flower blooms just past the end of an open stroke, or two thirds of the way round a closed one.
    if (flowers < 3) {
      var fx, fy, fp;
      if (!closed) { var t2 = tang[n - 1], E = ring[n - 1]; fx = E[0] + t2[0] * w * 0.95; fy = E[1] + t2[1] * w * 0.95; fp = E[4] + 0.01; }
      else {
        var k2 = Math.round(n * 0.66), nx = -tang[k2][1], ny = tang[k2][0];
        if (nx * (pts[k2][0] - mx) + ny * (pts[k2][1] - my) < 0) { nx = -nx; ny = -ny; }
        fx = pts[k2][0] + nx * w * 1.4; fy = pts[k2][1] + ny * w * 1.4; fp = ring[k2][4] + 0.01;
      }
      if (clear(fx, fy, w * 1.2, { pi: pi, s0: -1, s1: closed ? 1e9 : L - w * 3 })) flower(fx, fy, rT * 1.3 * zs, fp, rnd() * 6.28);
    }
  });
  // The dot that tells a 6 from a 9 buds last, as a small seed of light.
  if (lay.label === '6' || lay.label === '9') {
    var dx = lay.cx - cv2, dy = cv2 - (lay.cy + lay.h * 0.68), rb = w * 0.4, ringList = [];
    for (var q = 0; q <= 4; q++) { var fa = q / 4 * Math.PI, rr = Math.sin(fa) * rb; ringList.push([dx, dy, rb * 0.5 + Math.cos(fa) * rb * 0.6, q === 0 || q === 4 ? 0 : rr, 0.97 + q * 0.002, 0.95]); }
    overgrowTube(B, SH, ringList, hd ? 7 : 5, 0.04, 3);
  }
  overgrowCache[key] = { vine: B.geo(T), shade: SH.geo(T), reach: Math.max(reach, 1 + leafSpan) + 0.02, motes: motes };
  return overgrowCache[key];
}

// ---------- Materials ----------
// Each ring, leaf and petal scales out from the point it grows from once the growth front (uG) reaches
// its start; the tip is a bright, young light that settles to the plant's own colour.
var overgrowGrowSrc = [
  'vec3 transformed = vec3( position );',
  'float ogK = clamp( ( uG - aG.x ) / max( aG.y, 0.0001 ), 0.0, 1.0 );',
  'float ogE = ogK - 1.0;',
  'float ogF = aG.z > 0.5 ? 1.0 + 2.70158 * ogE * ogE * ogE + 1.70158 * ogE * ogE : ogK * ogK * ( 3.0 - 2.0 * ogK );',
  'ogF = ogK > 0.0 ? ogF : 0.0;',
  'transformed = aC + ( transformed - aC ) * ogF;'
].join('\n');
// The plant glows from within: a little along the stem, along each leaf's veins, and most in the
// flowers. uE holds those three strengths, so the show can swell them and let them settle.
function overgrowBodyHook(sh) {
  var ov = this.userData.ov;
  ['uG', 'uVine', 'uLeaf', 'uPetal', 'uHeart', 'uTip', 'uGlow', 'uE'].forEach(function (k) { sh.uniforms[k] = ov[k]; });
  sh.vertexShader = 'attribute vec3 aC; attribute vec3 aG; uniform float uG; varying float vYoung;\n' + sh.vertexShader.replace('#include <begin_vertex>', overgrowGrowSrc + '\nvYoung = 1.0 - ogK;');
  sh.fragmentShader = 'uniform vec3 uVine; uniform vec3 uLeaf; uniform vec3 uPetal; uniform vec3 uHeart; uniform vec3 uTip; uniform vec3 uGlow; uniform vec3 uE; varying float vYoung;\n' + sh.fragmentShader
    .replace('#include <color_fragment>',
      'float ogLeaf = clamp( vColor.g, 0.0, 1.0 ), ogPetal = clamp( vColor.g - 1.0, 0.0, 1.0 ), ogHeart = clamp( vColor.g - 2.0, 0.0, 1.0 );\n' +
      'vec3 ogCol = mix( mix( mix( uVine, uLeaf, ogLeaf ), uPetal, ogPetal ), uHeart, ogHeart ) * vColor.r * ( 1.0 + 0.6 * vColor.b * ( 1.0 - ogPetal ) );\n' +
      'diffuseColor.rgb = mix( ogCol, ogCol * 0.45 + uTip * 0.75, vYoung * 0.9 );')
    .replace('#include <emissivemap_fragment>', '#include <emissivemap_fragment>\n' +
      'totalEmissiveRadiance += uGlow * ( uE.x * ( 1.0 - ogLeaf ) * 0.22 + uE.y * vColor.b * ogLeaf * ( 1.0 - ogPetal ) * 2.4 + uE.z * ( ogPetal * ( 0.7 + vColor.b ) + ogHeart * 1.4 ) ) + uTip * vYoung * 2.0;');
}
function overgrowShadeHook(sh) {
  var ov = this.userData.ov;
  sh.uniforms.uG = ov.uG; sh.uniforms.uSun = ov.sun;
  sh.vertexShader = 'attribute vec3 aC; attribute vec3 aG; attribute float aH; uniform float uG; uniform vec2 uSun;\n' + sh.vertexShader.replace('#include <begin_vertex>', overgrowGrowSrc + '\ntransformed.xy += uSun * aH * ogF;');
}

// ---------- The show ----------
function overgrowDrop(die, fi, corner) {
  if (!die._overgrow) return;
  die._overgrow = die._overgrow.filter(function (o) {
    if (o.fi !== fi || o.corner !== corner) return true;
    o.meshes.forEach(function (m) { if (m.parent) m.parent.remove(m); m.material.dispose(); });
    return false;
  });
}
function overgrowPlay(st, die, fi, lay, k) {
  lay = lay || die.faces[fi].lay;
  var T = st.T, fc = die.faces[fi], f = fc.f, th = die.th, hd = st.q.detail !== 'basic', shape = overgrowShape(T, lay, hd);
  overgrowDrop(die, fi, lay.corner);
  var glow = th.burn, C = new T.Color(th.fey || th.grow || glow), G = new T.Color(glow), white = new T.Color(0xffffff);
  var ug = { value: 0 }, sun = { value: new T.Vector2() }, E = new T.Vector3();
  var ov = { uG: ug, sun: sun, uVine: { value: C.clone().multiplyScalar(0.34) }, uLeaf: { value: C.clone() }, uPetal: { value: G.clone().lerp(white, 0.35) },
    uHeart: { value: G.clone().lerp(white, 0.8) }, uTip: { value: G.clone().lerp(white, 0.6) }, uGlow: { value: G.clone() }, uE: { value: E } };
  var mat = new T.MeshStandardMaterial({ color: 0xffffff, vertexColors: true, roughness: 0.42, metalness: 0, envMapIntensity: 0.9 });
  mat.userData.ov = ov; mat.onBeforeCompile = overgrowBodyHook; mat.customProgramCacheKey = function () { return 'overgrow-fey'; };
  var shm = new T.MeshBasicMaterial({ vertexColors: true, transparent: true, depthWrite: false, side: T.DoubleSide, forceSinglePass: true, polygonOffset: true, polygonOffsetFactor: -2, polygonOffsetUnits: -2 });
  shm.userData.ov = ov; shm.onBeforeCompile = overgrowShadeHook; shm.customProgramCacheKey = function () { return 'overgrow-shade'; };
  var vine = new T.Mesh(shape.vine, mat), shade = new T.Mesh(shape.shade, shm);
  // The shapes are in the face's pixel frame: one matrix puts them on the face at the die's size.
  var R = new T.Vector3(f.right.x, f.right.y, f.right.z), U = new T.Vector3(f.up.x, f.up.y, f.up.z), N = fc.n, sc = die.size / lay.s;
  [vine, shade].forEach(function (m) {
    m.matrixAutoUpdate = false; m.matrix.makeBasis(R, U, N); m.matrix.scale(new T.Vector3(sc, sc, sc)); m.frustumCulled = false; fc.piece.add(m);
  });
  shade.renderOrder = 5;
  // The shadow falls away from the sun, in the face's own frame.
  var L = st.sun.position.clone().normalize().applyQuaternion(die.group.quaternion.clone().invert()), lz = Math.max(0.25, L.dot(N));
  sun.value.set(-L.dot(R) / lz, -L.dot(U) / lz);
  (die._overgrow = die._overgrow || []).push({ fi: fi, corner: lay.corner, meshes: [vine, shade] });
  // The number itself lights up just behind the vine's tip: a bright front that settles to a soft glow.
  var hl = hlLayer(die, fi, lay), u = hl.material.uniforms;
  // On a light die the light covers the face under the number, so it keeps its own deeper colour and
  // no wide halo, or the number would fade into the pale face round it.
  var cover = coverOf(die);
  u.uHot.value.set(mix(glow, '#ffffff', cover ? 0.45 : 0.7)); u.uGleam.value.set(cover ? glow : mix(glow, '#ffffff', 0.3)); u.uCover.value = cover; u.uHalo.value = cover ? 0.25 : 0.6; u.uCool.value = 2.6; u.uInk.value = 1; u.uK.value = 0; u.uP.value = 0;
  hl.visible = true;
  var t = 0, run = 1.25, end = 2.0, s = die.size, n = Math.round((hd ? 26 : 12) / (die.sides === 4 ? 2 : 1));
  // Motes of light rise from the flowers and drift off as the growth reaches them.
  if (shape.motes.length) {
    var cv2 = lay.cv / 2;
    st.fx.push(spray(st, { n: n, colors: [glow, mix(glow, '#ffffff', 0.6), '#ffffff'], size: s * 0.06, life: [0.6, 1.1], drag: 1.4, g: -0.5, noFloor: true,
      spawn: function (i, r) {
        var m = shape.motes[i % shape.motes.length], loc = faceToLocal(die, fi, m[0] + cv2, cv2 - m[1]).addScaledVector(N, m[2] * sc), w = worldOf(die, loc);
        return { p: [w.x + (r() - 0.5) * s * 0.12, w.y + 0.02, w.z + (r() - 0.5) * s * 0.12], v: [(r() - 0.5) * 1.4, 0.7 + r() * 1.2, (r() - 0.5) * 1.4], delay: Math.min(1.1, m[3]) * run * 0.9 + r() * 0.25 };
      } }));
  }
  ug.value = 0;
  function settle(x) { E.set(1.0 - 0.62 * x, 1.3 - 0.75 * x, 1.6 - 0.7 * x); }
  settle(0); E.multiplyScalar(0);
  return { update: function (dt) {
    t += dt;
    var x = clamp01(t / run), front = shape.reach * (0.8 * x + 0.2 * (1 - (1 - x) * (1 - x)));
    ug.value = front;
    settle(smooth(run * 0.85, end, t)); E.multiplyScalar(Math.min(1, t / 0.15));
    u.uT.value = t;
    u.uP.value = Math.max(0, front - 0.02) + Math.max(0, t - run) * 1.5;
    u.uK.value = Math.min(1, t / 0.1) * (1.15 - 0.5 * smooth(run, end, t));
    if (t >= end) { ug.value = shape.reach; settle(1); u.uK.value = 0.65; return false; }
    return true;
  } };
}

registerShow('overgrow', { name: 'Overgrow', what: 'A glowing fey vine winds round the number as it lights up, then flowers open and shed motes of light.', play: overgrowPlay,
  clear: function (die) {
    (die._overgrow || []).forEach(function (o) { o.meshes.forEach(function (m) { if (m.parent) m.parent.remove(m); m.material.dispose(); }); });
    die._overgrow = null;
  } });

// ---------- Choosing the quality ----------
// Auto works in three steps, so nobody waits for a test before their first roll:
//   1. hints: what the browser says about the device (graphics chip, cores, memory, screen)
//   2. a hidden test, once per device: a dozen dice drawn for about fifteen frames off screen when the
//      page is idle; the time per frame decides HD, HD without the two dearest layers, or Basic
//   3. during a throw, if frames still run long, the costliest layers drop one at a time
// A person who picks HD or Basic by hand gets exactly that, with no stepping down.
var QKEY = 'runedice.quality.v1';
function gpuName() {
  try {
    var c = document.createElement('canvas'), gl = c.getContext('webgl2') || c.getContext('webgl');
    if (!gl) return '';
    var ext = gl.getExtension('WEBGL_debug_renderer_info');
    var name = ext ? gl.getParameter(ext.UNMASKED_RENDERER_WEBGL) : gl.getParameter(gl.RENDERER);
    var lose = gl.getExtension('WEBGL_lose_context'); if (lose) lose.loseContext();
    return String(name || '');
  } catch (e) { return ''; }
}
function hints() {
  var gpu = gpuName(), soft = /swiftshader|llvmpipe|software|basic render/i.test(gpu);
  var phone = /Android|iPhone|iPad|Mobile/i.test(navigator.userAgent || '') || Math.min(screen.width, screen.height) < 600;
  var preset = !QUALITY.gl ? 'none' : soft || QUALITY.low ? 'basic' : phone ? 'mid' : 'hd';
  return { preset: preset, gpu: gpu, phone: phone, cores: navigator.hardwareConcurrency || 0, memory: navigator.deviceMemory || 0, how: 'hints' };
}
function cachedQuality() { try { var v = JSON.parse(localStorage.getItem(QKEY) || 'null'); return v && v.preset ? v : null; } catch (e) { return null; } }
function forgetQuality() { try { localStorage.removeItem(QKEY); } catch (e) {} }
// The layers for a preset name. 'mid' is HD without the glow and the contact shadows.
function layersOf(preset) {
  var q = qualityPreset(preset === 'basic' || preset === 'none' ? 'basic' : 'hd');
  if (preset === 'mid') { q.glow = false; q.contact = false; }
  q.name = preset; return q;
}
// What a roll should use for the person's setting: the layers, and whether it may step down.
function qualityFor(sel) {
  sel = sel || { mode: 'auto' };
  // A table held to Basic by its DM is Basic for everyone, whatever each person picked.
  if (sel.capBasic) return { q: layersOf('basic'), adapt: false };
  if (sel.mode === 'hd' || sel.mode === 'basic') return { q: layersOf(sel.mode), adapt: false };
  if (sel.mode === 'custom' && sel.layers) return { q: Object.assign(qualityPreset('hd'), sel.layers, { name: 'custom' }), adapt: false };
  var known = cachedQuality() || hints();
  return { q: layersOf(known.preset), adapt: true };
}
var DETECT = null;
// The hidden test. Resolves to { preset, ms, gpu, how: 'measured' }; cached so it runs once per device.
function detectQuality(force) {
  if (!force) { var c = cachedQuality(); if (c) return Promise.resolve(c); }
  if (DETECT) return DETECT;
  var h = hints();
  if (h.preset === 'none') return Promise.resolve(h);
  // A software renderer or a small device is Basic whatever a test would say, and drawing a dozen HD
  // dice there would hold the page for many seconds.
  if (h.preset === 'basic') { try { localStorage.setItem(QKEY, JSON.stringify(h)); } catch (e) {} return Promise.resolve(h); }
  var host = null, st = null, had = null;
  // The test's own table and face images go when it ends, however it ends: the images are tens of
  // megabytes.
  function clean() {
    if (st) disposeStage(st);
    if (host) host.remove();
    if (had) ATLAS_KEPT.filter(function (k) { return had.indexOf(k) < 0; }).forEach(dropAtlas);
    st = host = had = null;
  }
  DETECT = libs().then(function (L) {
    var T = L.T, C = L.C, kinds = ['jewelled', 'confetti', 'acrylic'];
    host = document.createElement('div');
    host.style.cssText = 'position:fixed;left:0;top:0;width:960px;height:600px;opacity:0;pointer-events:none;z-index:-1';
    document.body.appendChild(host);
    had = ATLAS_KEPT.slice();
    st = makeStage(T, host, false, T, qualityPreset('hd'));
    // A dozen dice in three looks: the same work to draw as a dozen different ones, from a quarter of
    // the images.
    for (var i = 0; i < 12; i++) {
      var k = kinds[i % 3], d = makeDie(T, C, 20, 0.9, k, coloursOf(k, kindOf(k).colours[0].key), 'rune', 384, { detail: 'hd', gems: true, env: st.env3 });
      d.group.position.set(-4.5 + (i % 4) * 3, 1.2, -2.6 + Math.floor(i / 4) * 2.6); d.group.rotation.set(i, i * 0.7, i * 1.3); addDie(st, d);
    }
    var gl = st.renderer.getContext(), px = new Uint8Array(4);
    // Each frame is made to finish before it is timed (a pixel is read back), so the time is the real
    // cost of drawing it. Only the drawing is timed: the wait for the screen's next refresh is not cost.
    function frame() { st.dice.forEach(function (d) { d.group.rotation.y += 0.05; }); updateContact(st); st.draw(); gl.readPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, px); }
    return new Promise(function (resolve, reject) {
      var n = 0, sum = 0, slow = 0;
      function step() {
        try {
          var a = performance.now(); frame(); var took = performance.now() - a;
          if (n >= 3) { sum += took; if (took > 66) slow++; }
          n++;
          // Two frames three times over the Basic line already answer the test; one may be a hiccup.
          if (n < 18 && slow < 2) { requestAnimationFrame(step); return; }
          var ms = sum / (n - 3);
          clean();
          var preset = ms <= 11 ? 'hd' : ms <= 22 ? 'mid' : 'basic';
          var out = { preset: preset, ms: Math.round(ms * 10) / 10, gpu: h.gpu, how: 'measured' };
          try { localStorage.setItem(QKEY, JSON.stringify(out)); } catch (e) {}
          DETECT = null; resolve(out);
        } catch (e) { reject(e); }
      }
      requestAnimationFrame(step);
    });
  }).catch(function () { clean(); DETECT = null; return h; });
  return DETECT;
}

// ---------- A roll ----------
// Up to a dozen dice of any sets thrown across the page with real physics. Every die lights its
// number its own way. The caller says which dice crit (the game system decides), and one throw
// plays one grand crit:
//   - on a die that counts (o.dice[i].counts, as with advantage's kept die); among those, or among
//     every crit die when none is marked, a success beats a failure, then the die nearest the
//     middle of the page takes it
//   - the other crit dice of the grand's kind echo it (its subdued twin, tied to the grand die),
//     nearest first, from the grand's big moment, a tenth of a second apart
//   - crit dice of the other kind play the subdued twin of the table's crit for that kind, once the
//     grand has peaked
// Every echo die is passed to the grand as a place to keep clear, so its number stays in view.
//
// o.dice: [{ sides (4, 6, 8, 10, 12, 20, or 100 for the percentile tens die), kind (a set key),
//   colour, value, crit ('succ' | 'fail' | null), counts, succFx, failFx }]. value is required: the
//   roller is handed its numbers and never picks them. succFx / failFx: that die's owner's crits.
// o.succFx / o.failFx: the table's crits. Every set and crit named must be loaded (loadSets,
//   loadCrits) before the call.
// o.quality: { mode: 'auto' | 'basic' | 'hd' | 'custom', layers, capBasic } (see qualityFor)
// o.host: an element to draw in; without one the dice are thrown over the whole page
// o.from: the point, in the host's pixels, the throw starts from; o.padTop: pixels kept clear at the top
// o.still: set the dice down where they rest, with no throw and no crits (Calm)
// o.skippable: a press anywhere skips the animations (see onSkip)
// o.onSettle(points): the dice have landed; points are where they rest, in the host's pixels
// o.onQuality(q, layer): Auto quality dropped a layer; o.hold: ms the dice rest; o.keep: never clear
// Resolves once the dice are gone with { values, drawn, crit, grand, echoes, q, stepped, simMs, fps }.
//
// A crit registers with registerCrit(key, { name, kind, what, fx(st, die, fi, th, o), beat, peak,
// echo(st, die, fi, th, e), echoWhat }). beat: seconds from its start to its big moment (when echoes
// begin); peak: when it is past its height (other-kind echoes start then; default beat + 0.8).
// o.clear: [{ x, z, r }] places (world units) the grand keeps in view; e: { from: the grand die or
// null, fromFi, order, grand: the grand's key }.
var CRITS = {};
function registerCrit(key, def) { def.key = key; CRITS[key] = def; }
function critList(kind) { return Object.keys(CRITS).filter(function (k) { return CRITS[k].kind === kind; }).map(function (k) { return Object.assign({ key: k }, CRITS[k]); }); }
var STEPDOWN = ['glow', 'contact', 'gems', 'sharp'];
// How long skipped or Calm dice stay on the page, in ms: longer than an unskipped roll's rest, since
// nobody watched them land.
var SKIPHOLD = 3000;

function stop(host) { if (host && host._chronicleDice) { var old = host._chronicleDice; host._chronicleDice = null; old.cancel(); } }
// Resolves once three.js and cannon-es are loaded, so a caller can wait for every file a roll needs.
function ready() { return libs().then(function () {}); }
function roll(o) {
  var host = o.host || document.body, fixed = !o.host;
  return libs().then(function (L) {
    var T = L.T, C = L.C;
    stop(host);
    var qp = qualityFor(o.quality), q = qp.q;
    var st = makeStage(T, host, fixed, T, q); st.padTop = o.padTop || 0;
    // A die or a shader that fails to build must not leave a canvas over the page.
    try { return throwOn(T, C, st, host, qp, o); } catch (e) { disposeStage(st); throw e; }
  });
}
function throwOn(T, C, st, host, qp, o) {
  var q = qp.q, fixed = !o.host;
  var n = o.dice.length, base = Math.max(0.55, Math.min(1.05, Math.min(st.hw, st.hh) / (fixed ? 7 : 3.3)));
  var size = o.size || base * (n > 4 ? Math.max(0.72, 1 - 0.035 * (n - 4)) : 1);
  var want = o.dice.map(function (d) { return d.value; });
  var dice = o.dice.map(function (d) {
    var die = makeDie(T, C, d.sides, size, d.kind, coloursOf(d.kind, d.colour), 'rune', faceSize(q), { detail: q.detail, gems: q.gems, env: st.env3 });
    addDie(st, die); return die;
  });
  var kinds = o.dice.map(function (d) { return d.crit === 'succ' || d.crit === 'fail' ? d.crit : null; });
  var crits = o.still ? [] : kinds.map(function (k, i) { return k ? i : -1; }).filter(function (i) { return i >= 0; });
  // The dice that could take the grand: the crit dice that count (all of them when none is
  // marked), and of those the successes when there are any.
  var first = crits.filter(function (i) { return o.dice[i].counts; });
  if (!first.length) first = crits.slice();
  var wins = first.filter(function (i) { return kinds[i] === 'succ'; });
  if (wins.length) first = wins;
  var crit = first.length ? kinds[first[0]] : null;
  var from = o.from ? st.toWorld(o.from.x, o.from.y) : null, t0p = performance.now(), plan = null, fix = [];
  if (o.still) {
    setDown(T, st, dice, want, from);
    // Calm fades the dice in where they rest; nothing travels.
    st.canvas.style.opacity = '0';
  } else {
    // A crit needs room: prefer a throw where a die that may take the grand rests away from the edges.
    var rank = first.length ? function (res) {
      var k = (res.steps - 1) * 7, best = 9;
      first.forEach(function (i) { var f = res.frames[i]; best = Math.min(best, Math.hypot(f[k] / st.hw, f[k + 2] / st.hh)); });
      return -Math.max(0, best - 0.45) * 0.8;
    } : null;
    plan = simulate(C, st, dice, from, rank);
    dice.forEach(function (d, i) {
      var f = plan.frames[i], k = (plan.steps - 1) * 7;
      fix.push(symmetry(T, d.sd, faceOf(d, want[i]), plan.landed[i].face, new T.Quaternion(f[k + 3], f[k + 4], f[k + 5], f[k + 6])));
    });
    // Draw the opening pose once before the clock starts. New sets compile their shaders and upload
    // their faces on their first draw, which can take a while, and the throw must not skip ahead for it.
    dice.forEach(function (d, i) {
      var f = plan.frames[i];
      d.group.position.set(f[0], f[1], f[2]); d.group.quaternion.set(f[3], f[4], f[5], f[6]).multiply(fix[i]);
    });
  }
  var simMs = performance.now() - t0p;
  updateContact(st); st.draw();
  return new Promise(function (resolve) {
    var t0 = performance.now(), last = t0, phase = o.still ? 'land' : 'throw', holdUntil = 0, skip = !!o.still, dismiss = false, raf = 0, ended = false, qs = new T.Quaternion();
    var times = [], lastChange = 0, frames = 0, stepped = [], allT = 0, allN = 0;
    // A press skips the animations, not the result: the dice stay a few seconds with their numbers
    // lit so everyone can read them, and a second press once they rest clears them. Typing in a
    // field never counts as a press.
    function onSkip(e) {
      var t = e.target; if (e.type === 'keydown' && t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
      if (skip && phase === 'hold') dismiss = true;
      skip = true;
    }
    if (o.skippable) setTimeout(function () { if (!ended) { window.addEventListener('pointerdown', onSkip, true); window.addEventListener('keydown', onSkip, true); } }, 0);
    function cleanup() { window.removeEventListener('pointerdown', onSkip, true); window.removeEventListener('keydown', onSkip, true); }
    function result() { return { values: want.slice(), drawn: true, crit: crit, grand: grandAt, echoes: echoAt.slice(), q: Object.assign({}, st.q), stepped: stepped.slice(), simMs: Math.round(simMs), fps: fps() }; }
    var grandAt = -1, echoAt = [], prevQ = dice.map(function () { return new T.Quaternion(); }), spins = dice.map(function () { return 0; });
    var me = { st: st, cancel: function () { ended = true; cancelAnimationFrame(raf); cleanup(); disposeStage(st); if (host._chronicleDice === me) host._chronicleDice = null; resolve(result()); } };
    host._chronicleDice = me;
    // Frames a second over the whole roll, so a step-down (which restarts the watch) doesn't hide it.
    function fps() { return allT > 0 ? Math.round(allN / allT) : 0; }
    function place(idx, dt) {
      dice.forEach(function (d, i) {
        var f = plan.frames[i], k = Math.min(idx, plan.steps - 1) * 7;
        prevQ[i].copy(d.group.quaternion);
        d.group.position.set(f[k], f[k + 1], f[k + 2]);
        d.group.quaternion.copy(qs.set(f[k + 3], f[k + 4], f[k + 5], f[k + 6]).multiply(fix[i]));
        // How fast the die turns, for sets whose insides respond to the tumble.
        spins[i] = dt > 0 ? 2 * Math.acos(Math.min(1, Math.abs(prevQ[i].dot(d.group.quaternion)))) / dt : 0;
      });
    }
    function land() {
      phase = 'fx';
      st.scene.updateMatrixWorld(true);
      dice.forEach(function (d, i) { spins[i] = 0; st.fx.push(highlight(st, d, faceOf(d, want[i]))); });
      if (crit) playCrits(st, dice, kinds, crits, first, o, function (g, e) { grandAt = g; echoAt = e; });
      if (o.onSettle) o.onSettle(dice.map(function (d) { return st.toScreen(d.group.position.clone().setY(d.size * 1.6)); }));
    }
    // Auto quality: when frames run long, drop the next costly layer, one at a time, mid-roll if need be.
    // The stage keeps its own copy of the layers (st.q), and that copy is what each step changes.
    function watch(dt) {
      frames++; times.push(dt); if (times.length > 20) times.shift();
      if (!qp.adapt || frames - lastChange < 18 || times.length < 12) return;
      // The median, so one long frame (a tab switch, a shader compiling as a crit starts) is no reason.
      var mid = times.slice().sort(function (a, b) { return a - b; })[times.length >> 1];
      if (mid <= 1 / 42) return;
      for (var i = 0; i < STEPDOWN.length; i++) if (st.q[STEPDOWN[i]]) {
        st.setLayer(STEPDOWN[i], false); stepped.push(STEPDOWN[i]); lastChange = frames; times.length = 0;
        if (o.onQuality) o.onQuality(Object.assign({}, st.q), STEPDOWN[i]);
        return;
      }
    }
    function finish() {
      ended = true; cleanup();
      if (o.keep) { resolve(result()); return; }
      st.canvas.style.opacity = '0';
      setTimeout(function () {
        if (!st.dead) disposeStage(st);
        if (host._chronicleDice === me) host._chronicleDice = null;
        resolve(result());
      }, fadeMs() + 40);
    }
    function frame(now) {
      if (ended) return;
      // A frame that fails ends the roll, so a broken effect never leaves a frozen canvas over the page.
      try { step(now); } catch (e) { me.cancel(); setTimeout(function () { throw e; }, 0); }
    }
    function step(now) {
      // A frame's timestamp can be older than the moment the throw was set up. Below zero, the first
      // frame would read before the recording and place the dice at NaN, and a NaN that reaches the
      // glow pass spreads across the whole picture.
      var real = Math.max(0, (now - last) / 1000), dt = Math.min(0.05, real); last = now;
      if (skip) dt = 5;
      if (phase === 'land') { st.canvas.style.opacity = '1'; land(); }
      if (phase === 'throw') {
        var idx = skip ? plan.steps : Math.max(0, Math.floor((now - t0) / 1000 / STEP));
        place(idx, real);
        if (idx >= plan.steps - 1) land();
      }
      if (phase === 'fx') {
        var running = st.fx.slice(); st.fx.length = 0;
        running.forEach(function (f) { if (f.update(dt)) st.fx.push(f); });
        if (!st.fx.length) { phase = 'hold'; holdUntil = now + (skip ? SKIPHOLD : (o.hold == null ? 1600 : o.hold)); }
      }
      dice.forEach(function (d, i) { if (d.tick) d.tick(dt, spins[i]); });
      updateContact(st);
      st.draw();
      if (frames > 1 && !skip) { watch(real); allT += real; allN++; }
      else frames++;
      if (phase === 'hold' && (now >= holdUntil || dismiss)) { finish(); return; }
      raf = requestAnimationFrame(frame);
    }
    raf = requestAnimationFrame(frame);
  });
}

// Calm: each die set down where it rests, the decided face up (a d4 rests on it), turned so the
// number reads upright, in rows near where the roll came from. Rows wrap and stay on the table.
function setDown(T, st, dice, want, from) {
  var size = dice.length ? dice[0].size : 1, gap = size * 2.5;
  var per = Math.max(1, Math.min(dice.length, Math.floor(st.hw * 1.7 / gap))), rows = Math.ceil(dice.length / per);
  var spanX = (per - 1) * gap / 2, spanZ = (rows - 1) * gap / 2;
  var limX = Math.max(0, st.hw * 0.8 - spanX - size), limZ = Math.max(0, st.hh * 0.6 - spanZ - size);
  var cx = from ? Math.max(-limX, Math.min(limX, from.x * 0.6)) : 0;
  var cz = from ? Math.max(-limZ, Math.min(limZ, from.z * 0.6)) : 0;
  dice.forEach(function (d, i) {
    var f = d.faces[faceOf(d, want[i])].f;
    var q = new T.Quaternion().setFromUnitVectors(new T.Vector3(f.n.x, f.n.y, f.n.z), new T.Vector3(0, d.sides === 4 ? -1 : 1, 0));
    var fu = new T.Vector3(f.up.x, f.up.y, f.up.z).applyQuaternion(q), yaw = d.sides === 4 ? 0 : Math.atan2(fu.x, -fu.z);
    q.premultiply(new T.Quaternion().setFromAxisAngle(new T.Vector3(0, 1, 0), yaw + (i % 2 ? -0.18 : 0.14)));
    d.group.quaternion.copy(q);
    var row = Math.floor(i / per), inRow = row < rows - 1 ? per : dice.length - row * per;
    d.group.position.set(cx + (i % per - (inRow - 1) / 2) * gap, d.rest, cz + row * gap - spanZ);
  });
}

// The crits for one throw, by the rules at the top of this section. done(grand index, echo indices).
function playCrits(st, dice, kinds, crits, first, o, done) {
  function at(i) { var p = dice[i].group.position; return { x: p.x, z: p.z }; }
  function mid(i) { var p = at(i); return Math.hypot(p.x / st.hw, p.z / st.hh); }
  var g = first.slice().sort(function (a, b) { return mid(a) - mid(b); })[0], gd = dice[g], kind = kinds[g], gfi = faceOf(gd, o.dice[g].value);
  var own = o.dice[g] || {}, key = (kind === 'succ' ? own.succFx || o.succFx : own.failFx || o.failFx);
  var pick = CRITS[key] || critList(kind)[0];
  if (!pick) return;
  var gp = at(g), others = crits.filter(function (i) { return i !== g; });
  function dist(i) { var p = at(i); return Math.hypot(p.x - gp.x, p.z - gp.z); }
  var same = others.filter(function (i) { return kinds[i] === kind; }).sort(function (a, b) { return dist(a) - dist(b); });
  var other = others.filter(function (i) { return kinds[i] !== kind; }).sort(function (a, b) { return dist(a) - dist(b); });
  var okey = kind === 'succ' ? o.failFx : o.succFx, opick = CRITS[okey] || critList(kind === 'succ' ? 'fail' : 'succ')[0];
  var clear = others.map(function (i) { var p = at(i); return { x: p.x, z: p.z, r: dice[i].size * 1.9 }; });
  var beat = pick.beat == null ? 1.2 : pick.beat, peak = pick.peak == null ? beat + 0.8 : pick.peak, t = -0.5, list = [];
  same.forEach(function (i, n) { list.push({ i: i, at: beat + 0.1 * n, c: pick, e: { from: gd, fromFi: gfi, order: n, grand: pick.key } }); });
  other.forEach(function (i, n) { list.push({ i: i, at: peak + 0.1 * n, c: opick, e: { from: null, fromFi: -1, order: n, grand: pick.key } }); });
  done(g, others);
  st.fx.push({ update: function (dt) {
    var was = t; t += dt;
    if (was < 0 && t >= 0) st.fx.push(pick.fx(st, gd, gfi, gd.th, { clear: clear, echoes: others.length }));
    list.forEach(function (x) {
      if (x.done || t < x.at) return; x.done = true;
      var d = dice[x.i], fi = faceOf(d, o.dice[x.i].value), fx = x.c && x.c.echo ? x.c.echo(st, d, fi, d.th, x.e) : echoPlain(st, d, fi, d.th);
      if (fx) st.fx.push(fx);
    });
    return list.some(function (x) { return !x.done; }) || t < 0;
  } });
}
// The echo for a crit that has no subdued twin of its own: the number flares and a ring runs out.
function echoPlain(st, die, fi, th) {
  var t = 0, fc = glowLayer(die, fi), base = die.group.position;
  glowNumeral(die, fi, th.burn, 1); fc.mat.emissiveIntensity = 0;
  st.fx.push(wave(st, { x: base.x, z: base.z }, th.burn, die.size * 0.8, die.size * 2.2, 0.8, isLight(th)));
  return { update: function (dt) { t += dt; fc.mat.emissiveIntensity = 2.2 * env4(t, 0, 0.15, 0.5, 1.4); return t < 1.4; } };
}

// ---------- Sets and crits, imported when first needed ----------
// Every set in the order a picker lists them; the first three are built in above.
var SET_KEYS = ['jewelled', 'confetti', 'acrylic', 'cage', 'heartlight', 'gothic', 'obsidian', 'moonstone', 'oak', 'nebula', 'bone', 'scale'];
var CRIT_KEYS = { succ: ['storm', 'phoenix', 'dawn', 'starfall', 'bloom', 'hoard'], fail: ['grasp', 'frost', 'shatter', 'hex', 'crows', 'abyss'] };
var LOADING = {};
// Only names on the lists above are ever imported, so a key read from a saved preference can't
// point the import anywhere else.
function load(dir, key) {
  var id = dir + '/' + key;
  if (!LOADING[id]) {
    LOADING[id] = import(new URL('./' + dir + '/' + key + '.js', import.meta.url).href);
    LOADING[id].catch(function () { delete LOADING[id]; });
  }
  return LOADING[id];
}
function loadSets(keys) {
  return Promise.all((keys || []).filter(function (k) { return SET_KEYS.indexOf(k) >= 0 && !KINDS[k]; }).map(function (k) { return load('sets', k); }));
}
function loadCrits(keys) {
  var all = CRIT_KEYS.succ.concat(CRIT_KEYS.fail);
  return Promise.all((keys || []).filter(function (k) { return all.indexOf(k) >= 0 && !CRITS[k]; }).map(function (k) { return load('crits', k); }));
}
// Everything a picker shows: every set with its colours and its show, and every crit by kind. It
// loads all of them, so it is for the pages where people choose dice, not for a roll.
function catalogue() {
  return Promise.all([loadSets(SET_KEYS), loadCrits(CRIT_KEYS.succ.concat(CRIT_KEYS.fail))]).then(function () {
    function crit(k) { var c = CRITS[k]; return { key: k, name: c.name, what: c.what || '', echoWhat: c.echoWhat || '' }; }
    return {
      sets: SET_KEYS.map(function (k) {
        var K = kindOf(k);
        return { key: k, name: K.name, note: K.note || '', show: K.highlight, showWhat: K.highlightWhat || '',
          colours: K.colours.map(function (c) {
            var cc = coloursOf(k, c.key);
            return { key: c.key, name: c.name, note: c.note || '', swatch: cc.base.grad.slice(), glow: cc.burn };
          }) };
      }),
      crits: { succ: CRIT_KEYS.succ.map(crit), fail: CRIT_KEYS.fail.map(crit) }
    };
  });
}
// Whether a key names a set, or a crit of that kind.
function hasSet(key) { return SET_KEYS.indexOf(key) >= 0; }
function hasCrit(key, kind) { return (CRIT_KEYS[kind] || []).indexOf(key) >= 0; }

// What the set and crit files use, and the API dice.js and the tests call.
export {
  CRIT_KEYS, GEMKEY, GEM_FS, GEM_VS, MARKS, PX, QUALITY, RIMPATCH, SET_KEYS, TAN, add,
  cachedQuality, canvas, catalogue, clamp01, configure, cracksFor, cross, cutPaths, detectQuality,
  dimmer, dot, drawBurn, drop, env4, faceOf, faceToLocal, flakeLayout, forgetQuality, fxLight,
  gemMat, glow, glowFace, glowLayer, glowNumeral, glowStrokes, hasCrit, hasSet, heightToNormal,
  hexA, in3, inout, isLight, kindOf, landedFace, len, loadCrits, loadSets, mix, mul, mulberry,
  norm, numberOnto, out3, paintNumeral, polyLen, polyPath, qualityFor, ready, registerCrit,
  registerSet, roll, setDown, shadeDie, share, simulate, smooth, solid, splitDie, spray, spriteTex,
  starTex, stop, strokePaths, strokeW, sub, symmetry, tintDie, wave, wholeDie, worldOf, writhe
};
