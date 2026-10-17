/**
 * dice.js — Chronicle.dice, thrown 3D dice for any roll on the site.
 *
 *   Chronicle.dice.roll(groups, opts) -> Promise<[{ label, values }]>
 *
 * groups: [{ label, dice: [{ sides }] }], sides 4, 6, 8, 10, 12 or 20;
 *   { sides: 10, tens: true } is the percentile tens die (00, 10 … 90).
 * values: dN gives 1..N; the tens die gives 0, 10 … 90 (00 is 0).
 * opts: from (an element or client {x, y} the throw starts at),
 *   describe(group, values) (the words read out to screen readers),
 *   still (set the dice down instead of throwing them), results (values
 *   decided by the caller, one array per group), crit ('succ' | 'fail').
 *
 * The values are decided before anything is drawn, so Calm, Off and a
 * browser without WebGL resolve at once with the same numbers a throw would
 * show. The throw is simulated ahead with cannon-es, then each die is turned
 * by one of its own symmetries so the decided face is the one that lands;
 * the shape is identical after that turn, so the motion is still true
 * physics. three.js and cannon-es are vendored and load on the first throw.
 */
(function () {
  'use strict';

  var W = typeof window !== 'undefined' ? window : globalThis;
  var SELF = typeof document !== 'undefined' ? document.currentScript : null;
  var THREE_SRC = (SELF && SELF.getAttribute('data-three-src')) || '/static/vendor/three.module.min.js';
  var CANNON_SRC = (SELF && SELF.getAttribute('data-cannon-src')) || '/static/vendor/cannon-es.js';

  var LIB = null;
  function libs() {
    if (!LIB) {
      LIB = Promise.all([import(THREE_SRC), import(CANNON_SRC)])
        .then(function (m) { return { T: m[0], C: m[1] }; });
      // A failed load must not stick: the next roll tries again.
      LIB.catch(function () { LIB = null; });
    }
    return LIB;
  }

  var SIDES = [4, 6, 8, 10, 12, 20];
  var PX = 46;           // screen pixels per world unit on the table
  var STEP = 1 / 120;    // fixed physics step, so a recorded throw replays exactly
  var FOV = 28;
  var TAN = Math.tan(FOV * Math.PI / 360);
  var CV = 256;          // face texture size

  // ---------- Values: decided first, from the browser's crypto source ----------

  function rand(n) {
    var c = W.crypto;
    if (c && c.getRandomValues) {
      // Rejection sampling keeps every face equally likely.
      var buf = new Uint32Array(1), lim = Math.floor(4294967296 / n) * n;
      do { c.getRandomValues(buf); } while (buf[0] >= lim);
      return buf[0] % n + 1;
    }
    return Math.floor(Math.random() * n) + 1;
  }
  function randomValue(die) { return die.tens ? (rand(10) - 1) * 10 : rand(die.sides); }
  function validValue(die, v) {
    if (typeof v !== 'number' || v !== Math.floor(v)) return false;
    return die.tens ? v >= 0 && v <= 90 && v % 10 === 0 : v >= 1 && v <= die.sides;
  }

  // Checks the groups and settles every value, throwing on anything the
  // dice could never show.
  function decide(groups, results) {
    if (!Array.isArray(groups)) throw new TypeError('Chronicle.dice.roll: groups must be an array');
    return groups.map(function (g, gi) {
      if (!g || !Array.isArray(g.dice)) throw new TypeError('Chronicle.dice.roll: group ' + gi + ' needs a dice array');
      var given = results && results[gi];
      return {
        label: g.label == null ? '' : String(g.label),
        values: g.dice.map(function (d, di) {
          if (!d || SIDES.indexOf(d.sides) < 0) throw new TypeError('Chronicle.dice.roll: unsupported die d' + (d && d.sides));
          if (d.tens && d.sides !== 10) throw new TypeError('Chronicle.dice.roll: only a d10 can be a tens die');
          if (given && given[di] !== undefined) {
            if (!validValue(d, given[di])) throw new RangeError('Chronicle.dice.roll: ' + given[di] + ' is not a face of that die');
            return given[di];
          }
          return randomValue(d);
        })
      };
    });
  }

  // A die's face numbering and the value a face means. A d4 reads its top
  // corner, so its face value is the corner opposite the face it rests on.
  function faceValue(die, face) { return die.tens ? (face.value % 10) * 10 : face.value; }
  function faceLabel(die, face) { return die.tens ? (face.value === 10 ? '00' : face.value + '0') : String(face.value); }
  function faceFor(sd, die, value) {
    for (var i = 0; i < sd.faces.length; i++) if (faceValue(die, sd.faces[i]) === value) return i;
    return 0;
  }

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
    if (sides === 4) out = [[1, 1, 1], [1, -1, -1], [-1, 1, -1], [-1, -1, 1]];
    else if (sides === 6) { for (s = 0; s < 8; s++) out.push([s & 1 ? 1 : -1, s & 2 ? 1 : -1, s & 4 ? 1 : -1]); }
    else if (sides === 8) out = [[1, 0, 0], [-1, 0, 0], [0, 1, 0], [0, -1, 0], [0, 0, 1], [0, 0, -1]];
    else if (sides === 12) {
      for (s = 0; s < 8; s++) out.push([s & 1 ? 1 : -1, s & 2 ? 1 : -1, s & 4 ? 1 : -1]);
      for (t = 0; t < 4; t++) { var a = t & 1 ? 1 : -1, b = t & 2 ? 1 : -1; out.push([0, a * I, b * P], [a * I, b * P, 0], [a * P, 0, b * I]); }
    } else if (sides === 20) {
      for (t = 0; t < 4; t++) { var c = t & 1 ? 1 : -1, d = t & 2 ? 1 : -1; out.push([0, c, d * P], [c, d * P, 0], [c * P, 0, d]); }
    } else if (sides === 10) {
      // Pentagonal trapezohedron: a zigzag ring of ten and two apexes, the
      // apex height chosen so each kite is flat.
      var k = 0.13, h = k * (1 + Math.cos(Math.PI / 5)) / (1 - Math.cos(Math.PI / 5));
      for (s = 0; s < 10; s++) { var g = s * Math.PI / 5; out.push([Math.cos(g), s % 2 ? -k : k, Math.sin(g)]); }
      out.push([0, h, 0], [0, -h, 0]);
    }
    var pts = out.map(v3), r = 0;
    pts.forEach(function (p) { r = Math.max(r, len(p)); });
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

  // Opposite faces add up the way real dice do, N + 1, so 1 sits under 20.
  function numberFaces(faces, sides, nv) {
    if (sides === 4) {
      faces.forEach(function (f) {
        for (var q = 0; q < nv; q++) if (f.idx.indexOf(q) < 0) f.value = q + 1;
      });
      return;
    }
    var used = [], val = 1;
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
    var v = solidVerts(sides), f = hull(v);
    numberFaces(f, sides, v.length);
    f.forEach(function (face) {
      // Upright for the number: towards the face's first vertex (a d20's
      // point, a d10's apex), or the middle of an edge on a d6.
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

  // The face that counts once the die is still: the one facing up, or for a
  // d4 the one it rests on. q is the die's rotation (x, y, z, w).
  function landedFace(sd, q) {
    // The world's up seen from inside the die: (0,1,0) turned by q's inverse.
    var r = { x: -q.x, y: -q.y, z: -q.z }, u = { x: 0, y: 1, z: 0 }, t = mul(cross(r, u), 2);
    var up = add(add(u, mul(t, q.w)), cross(r, t));
    if (sd.sides === 4) up = mul(up, -1);
    var best = -2, at = 0;
    sd.faces.forEach(function (f, i) { var d = dot(f.n, up); if (d > best) { best = d; at = i; } });
    return { face: at, dot: best };
  }

  // One of the die's own rotations that carries face `from` onto face `to`.
  // Where several do, take the one that leaves the number most upright on
  // screen (screen up is -z), so the roll reads at a glance.
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
      var score = qSim && sd.sides !== 4 ? -upA.clone().applyQuaternion(q).applyQuaternion(qSim).z : 0;
      if (score > bestScore) { bestScore = score; best = q; }
    }
    return best || new T.Quaternion();
  }

  function bodyShape(C, sd, size) {
    return new C.ConvexPolyhedron({
      vertices: sd.v.map(function (p) { return new C.Vec3(p.x * size, p.y * size, p.z * size); }),
      faces: sd.faces.map(function (f) { return f.idx.slice(); })
    });
  }

  // ---------- The throw: simulated ahead, then replayed ----------

  // table: { hw, hh, padTop } in world units and pixels; dice: [{ sd, shape,
  // size }]; from: a world {x, z} or null. Keeps the attempt whose dice sit
  // flattest, so none ends cocked against a wall.
  function simulate(C, table, dice, from) {
    var best = null, top = (table.padTop || 0) / PX;
    for (var attempt = 0; attempt < 5; attempt++) {
      var world = new C.World({ gravity: new C.Vec3(0, -42, 0) }); world.allowSleep = true;
      var dm = new C.Material('die'), wm = new C.Material('wall');
      world.addContactMaterial(new C.ContactMaterial(dm, wm, { friction: 0.32, restitution: 0.42 }));
      world.addContactMaterial(new C.ContactMaterial(dm, dm, { friction: 0.2, restitution: 0.5 }));
      var inset = Math.min(1.1, Math.min(table.hw, table.hh) * 0.18);
      var wall = function (p, ax, ang) {
        var b = new C.Body({ mass: 0, material: wm, shape: new C.Plane() });
        b.position.set(p[0], p[1], p[2]); b.quaternion.setFromAxisAngle(new C.Vec3(ax[0], ax[1], ax[2]), ang); world.addBody(b);
      };
      wall([0, 0, 0], [1, 0, 0], -Math.PI / 2);
      wall([-table.hw + inset, 0, 0], [0, 1, 0], Math.PI / 2); wall([table.hw - inset, 0, 0], [0, 1, 0], -Math.PI / 2);
      wall([0, 0, -table.hh + inset + top], [0, 1, 0], 0); wall([0, 0, table.hh - inset], [0, 1, 0], Math.PI);
      var rnd = Math.random, bodies = [];
      dice.forEach(function (d, i) {
        var b = new C.Body({ mass: 1, material: dm, shape: d.shape, sleepSpeedLimit: 0.25, sleepTimeLimit: 0.35, linearDamping: 0.12, angularDamping: 0.14 });
        var sx, sz, vx, vz, sp = Math.min(table.hw, table.hh) * (1.8 + rnd() * 0.7), lim = d.size * 1.3;
        if (from) {
          sx = from.x + (i % 3 - 1) * d.size * 1.7; sz = from.z + Math.floor(i / 3) * d.size * 1.7;
          var dx = -sx * (0.7 + rnd() * 0.5) + (rnd() - 0.5) * table.hw * 0.6, dz = -sz * (0.7 + rnd() * 0.5) + (rnd() - 0.5) * table.hh * 0.6, dl = Math.hypot(dx, dz) || 1;
          vx = dx / dl * sp; vz = dz / dl * sp;
        } else {
          sx = -table.hw + inset + lim + (i % 4) * d.size * 1.6; sz = table.hh - inset - lim - Math.floor(i / 4) * d.size * 1.8;
          vx = sp * (0.9 + rnd() * 0.4); vz = -sp * (0.55 + rnd() * 0.4);
        }
        sx = Math.max(-table.hw + inset + lim, Math.min(table.hw - inset - lim, sx));
        sz = Math.max(-table.hh + inset + top + lim, Math.min(table.hh - inset - lim, sz));
        b.position.set(sx, d.size * 2.2 + rnd() * d.size * 1.5, sz);
        b.quaternion.setFromEuler(rnd() * 6.28, rnd() * 6.28, rnd() * 6.28);
        b.velocity.set(vx, -2, vz);
        b.angularVelocity.set((rnd() - 0.5) * 30, (rnd() - 0.5) * 30, (rnd() - 0.5) * 30);
        world.addBody(b); bodies.push(b);
      });
      var MAX = 840, frames = dice.map(function () { return new Float32Array(MAX * 7); }), steps = 0;
      for (; steps < MAX; steps++) {
        world.step(STEP);
        bodies.forEach(function (b, i) {
          var o = steps * 7, f = frames[i];
          f[o] = b.position.x; f[o + 1] = b.position.y; f[o + 2] = b.position.z;
          f[o + 3] = b.quaternion.x; f[o + 4] = b.quaternion.y; f[o + 5] = b.quaternion.z; f[o + 6] = b.quaternion.w;
        });
        if (steps > 50 && bodies.every(function (b) { return b.sleepState === C.Body.SLEEPING; })) { steps++; break; }
      }
      var landed = bodies.map(function (b, i) { return landedFace(dice[i].sd, b.quaternion); });
      var worst = Math.min.apply(null, landed.map(function (l) { return l.dot; }));
      // A throw still rocking when time runs out ranks below any that settled.
      var settled = steps < MAX, res = { frames: frames, steps: Math.min(steps, MAX), landed: landed, worst: worst, score: worst - (settled ? 0 : 1) };
      if (!best || res.score > best.score) best = res;
      if (settled && worst > 0.97) break;
    }
    return best;
  }

  // ---------- Faces ----------

  // The neutral look every roll uses until a campaign picks one in Customize.
  var LOOK = { body: ['#f6f1e7', '#d8cdb8'], ink: '#2b2620', rough: 0.5, metal: 0.04 };

  function mulberry(a) {
    return function () { a |= 0; a = a + 0x6D2B79F5 | 0; var t = Math.imul(a ^ a >>> 15, 1 | a); t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296; };
  }

  function faceLayout(sd, face) {
    var s = CV * 0.47 / face.R, cx = CV / 2, cy = CV / 2;
    var poly = face.idx.map(function (q) { var p = sub(sd.v[q], face.c); return [cx + dot(p, face.right) * s, cy - dot(p, face.up) * s]; });
    return { s: s, cx: cx, cy: cy + (sd.sides === 20 ? face.inr * s * 0.04 : 0), poly: poly, r: face.inr * s };
  }

  function number(g, text, x, y, h, ink) {
    g.textAlign = 'center'; g.textBaseline = 'middle';
    g.font = '700 ' + Math.round(h) + 'px Inter, system-ui, sans-serif';
    g.fillStyle = 'rgba(0,0,0,.22)'; g.fillText(text, x, y + 2);
    g.fillStyle = ink; g.fillText(text, x, y);
    // A bar under 6 and 9 says which way is up.
    if (text === '6' || text === '9') g.fillRect(x - h * 0.22, y + h * 0.48, h * 0.44, Math.max(3, h * 0.07));
  }

  function drawFace(g, sd, die, fi, lay, look) {
    var face = sd.faces[fi], rnd = mulberry(sd.sides * 97 + fi * 13 + 7);
    var grd = g.createRadialGradient(CV * 0.42, CV * 0.36, 10, CV / 2, CV / 2, CV * 0.72);
    grd.addColorStop(0, look.body[0]); grd.addColorStop(1, look.body[1]);
    g.fillStyle = grd; g.fillRect(0, 0, CV, CV);
    for (var i = 0; i < 260; i++) {
      g.fillStyle = rnd() > 0.5 ? 'rgba(255,255,255,' + rnd() * 0.06 + ')' : 'rgba(0,0,0,' + rnd() * 0.05 + ')';
      g.fillRect(rnd() * CV, rnd() * CV, 1 + rnd() * 2, 1 + rnd() * 2);
    }
    // A soft bevel along the face's edges.
    g.beginPath(); lay.poly.forEach(function (p, k) { if (k) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.closePath();
    g.lineJoin = 'round'; g.strokeStyle = 'rgba(255,255,255,.35)'; g.lineWidth = 10; g.stroke();
    g.strokeStyle = 'rgba(0,0,0,.18)'; g.lineWidth = 3; g.stroke();
    if (sd.sides === 4) {
      // Each corner carries its own number, upright towards that corner, so
      // whichever face is read the top corner shows the roll.
      face.idx.forEach(function (q, k) {
        var p = lay.poly[k], ang = Math.atan2(p[0] - lay.cx, -(p[1] - lay.cy));
        g.save(); g.translate(lay.cx + (p[0] - lay.cx) * 0.52, lay.cy + (p[1] - lay.cy) * 0.52); g.rotate(ang);
        number(g, String(q + 1), 0, 0, lay.r * 0.85, look.ink);
        g.restore();
      });
      return;
    }
    var label = faceLabel(die, face);
    number(g, label, lay.cx, lay.cy, label.length > 1 ? 0.95 * lay.r : 1.15 * lay.r, look.ink);
  }

  var TEX = {};
  function faceTexture(T, sd, die, fi, lay, look) {
    var key = sd.sides + (die.tens ? 't' : '') + '|' + fi;
    if (TEX[key]) return TEX[key];
    var c = document.createElement('canvas'); c.width = c.height = CV;
    drawFace(c.getContext('2d'), sd, die, fi, lay, look);
    var t = new T.CanvasTexture(c); t.colorSpace = T.SRGBColorSpace; t.anisotropy = 4; t.userData.shared = true;
    TEX[key] = t;
    return t;
  }

  function makeDie(T, C, spec, size) {
    var sd = solid(spec.sides), group = new T.Group();
    sd.faces.forEach(function (f, fi) {
      var lay = faceLayout(sd, f), tex = faceTexture(T, sd, spec, fi, lay, LOOK);
      var pos = [], uv = [], nr = [], n = f.idx.length;
      var put = function (p, u) { pos.push(p.x * size, p.y * size, p.z * size); uv.push(u[0] / CV, 1 - u[1] / CV); nr.push(f.n.x, f.n.y, f.n.z); };
      for (var k = 0; k < n; k++) {
        put(f.c, [CV / 2, CV / 2]); put(sd.v[f.idx[k]], lay.poly[k]); put(sd.v[f.idx[(k + 1) % n]], lay.poly[(k + 1) % n]);
      }
      var geo = new T.BufferGeometry();
      geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3));
      geo.setAttribute('uv', new T.Float32BufferAttribute(uv, 2));
      geo.setAttribute('normal', new T.Float32BufferAttribute(nr, 3));
      var mesh = new T.Mesh(geo, new T.MeshStandardMaterial({ map: tex, roughness: LOOK.rough, metalness: LOOK.metal }));
      mesh.castShadow = true;
      group.add(mesh);
    });
    return { spec: spec, sd: sd, group: group, shape: bodyShape(C, sd, size), size: size, rest: dot(sd.faces[0].c, sd.faces[0].n) * size };
  }

  // ---------- The table: a transparent canvas over the page ----------

  var GL = null;
  function hasGL() {
    if (GL === null) {
      try { var c = document.createElement('canvas'); GL = !!(c.getContext('webgl2') || c.getContext('webgl')); } catch (e) { GL = false; }
    }
    return GL;
  }
  function lowEnd() { var n = W.navigator || {}; return (n.hardwareConcurrency || 4) <= 2 || (n.deviceMemory || 8) <= 2; }

  // Matches MotionLevel: html[data-motion] is "calm" or "off" (theme.js sets it).
  function motion() {
    var m = document.documentElement.getAttribute('data-motion');
    if (m === 'off' || m === 'calm') return m;
    if (W.matchMedia && W.matchMedia('(prefers-reduced-motion: reduce)').matches) return 'off';
    return 'full';
  }

  function makeStage(T) {
    var Wd = Math.max(120, W.innerWidth), Ht = Math.max(120, W.innerHeight);
    var canvas = document.createElement('canvas');
    canvas.className = 'chronicle-dice';
    canvas.setAttribute('aria-hidden', 'true');
    canvas.style.cssText = 'position:fixed;inset:0;width:100vw;height:100vh;z-index:900;pointer-events:none;transition:opacity 320ms ease';
    document.body.appendChild(canvas);
    var renderer = new T.WebGLRenderer({ canvas: canvas, alpha: true, antialias: true });
    renderer.setPixelRatio(Math.min(W.devicePixelRatio || 1, lowEnd() ? 1 : 2));
    renderer.setSize(Wd, Ht, false);
    renderer.shadowMap.enabled = true; renderer.shadowMap.type = T.PCFSoftShadowMap;
    renderer.outputColorSpace = T.SRGBColorSpace;
    var scene = new T.Scene(), hw = Wd / 2 / PX, hh = Ht / 2 / PX, dist = hh / TAN;
    var cam = new T.PerspectiveCamera(FOV, Wd / Ht, 1, dist * 2);
    cam.position.set(0, dist, 0); cam.up.set(0, 0, -1); cam.lookAt(0, 0, 0);
    scene.add(new T.HemisphereLight(0xffffff, 0x8890a0, 1.15));
    var sun = new T.DirectionalLight(0xffffff, 2.1); sun.position.set(-hw * 0.5, dist * 0.6, -hh * 0.35); sun.castShadow = true;
    var map = lowEnd() ? 1024 : 2048;
    sun.shadow.mapSize.set(map, map);
    var sc = sun.shadow.camera; sc.left = -hw * 1.3; sc.right = hw * 1.3; sc.top = hh * 1.3; sc.bottom = -hh * 1.3; sc.near = 1; sc.far = dist * 2; sun.shadow.radius = 4;
    scene.add(sun);
    var floor = new T.Mesh(new T.PlaneGeometry(hw * 4, hh * 4), new T.ShadowMaterial({ opacity: 0.28 }));
    floor.rotation.x = -Math.PI / 2; floor.receiveShadow = true; scene.add(floor);
    // Dice land below the top bar, never under it.
    var bar = document.getElementById('app-topbar'), padTop = bar ? Math.max(0, bar.getBoundingClientRect().bottom) : 0;
    return { renderer: renderer, scene: scene, cam: cam, canvas: canvas, hw: hw, hh: hh, padTop: padTop, dead: false,
      toWorld: function (px, py) { return { x: (px - Wd / 2) / PX, z: (py - Ht / 2) / PX }; } };
  }

  function disposeStage(st) {
    if (!st || st.dead) return;
    st.dead = true;
    st.scene.traverse(function (o) {
      if (o.geometry) o.geometry.dispose();
      if (o.material) { if (o.material.map && !o.material.map.userData.shared) o.material.map.dispose(); o.material.dispose(); }
    });
    st.renderer.dispose();
    try { st.renderer.forceContextLoss(); } catch (e) { /* already gone */ }
    st.canvas.remove();
  }

  // ---------- Announcing the result ----------

  function announce(text) {
    if (!text || typeof document === 'undefined') return;
    var live = document.getElementById('chronicle-dice-live');
    if (!live) {
      live = document.createElement('div');
      live.id = 'chronicle-dice-live'; live.className = 'sr-only';
      live.setAttribute('aria-live', 'polite');
      document.body.appendChild(live);
    }
    live.textContent = '';
    // A fresh node each time, so the same total twice is still read out.
    setTimeout(function () { live.textContent = text; }, 30);
  }
  function describeAll(groups, out, describe) {
    return out.map(function (r, i) {
      if (describe) return describe(groups[i], r.values);
      return (r.label ? r.label + ': ' : '') + r.values.join(', ');
    }).filter(Boolean).join('. ');
  }

  function fromPoint(from) {
    if (!from) return null;
    if (typeof from.getBoundingClientRect === 'function') {
      var r = from.getBoundingClientRect();
      return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
    }
    return typeof from.x === 'number' && typeof from.y === 'number' ? from : null;
  }

  // ---------- One roll ----------

  var current = null;
  function stop() { if (current) { var c = current; current = null; c.cancel(); } }

  function roll(groups, opts) {
    opts = opts || {};
    var out;
    try { out = decide(groups, opts.results); } catch (e) { return Promise.reject(e); }
    var specs = [], want = [];
    (groups || []).forEach(function (g, gi) { g.dice.forEach(function (d, di) { specs.push(d); want.push(out[gi].values[di]); }); });
    var m = typeof document === 'undefined' ? 'off' : motion();
    var told = describeAll(groups, out, opts.describe);
    // TODO(#1442): opts.crit is accepted now and drawn once the crit
    // effects are signed off.
    if (!specs.length || m === 'off' || !hasGL()) {
      announce(told);
      return Promise.resolve(out);
    }
    var still = !!opts.still || m === 'calm';
    var landed = new Promise(function (resolve) {
      libs().then(function (L) { throwDice(L, specs, want, still, fromPoint(opts.from), resolve); })
        .catch(function () { resolve(); });
    });
    // Calm and still rolls do not wait for the drawing.
    if (still) { announce(told); return Promise.resolve(out); }
    return landed.then(function () { announce(told); return out; });
  }

  function throwDice(L, specs, want, still, at, landed) {
    var T = L.T, C = L.C;
    stop();
    var st = makeStage(T);
    var size = Math.max(0.55, Math.min(1.05, Math.min(st.hw, st.hh) / 5.5));
    var dice = specs.map(function (s) { var d = makeDie(T, C, s, size); st.scene.add(d.group); return d; });
    var from = at ? st.toWorld(at.x, at.y) : null;
    var plan = null, fix = [];
    if (!still) {
      plan = simulate(C, st, dice, from);
      dice.forEach(function (d, i) {
        var f = plan.frames[i], k = (plan.steps - 1) * 7;
        fix.push(symmetry(T, d.sd, faceFor(d.sd, d.spec, want[i]), plan.landed[i].face, new T.Quaternion(f[k + 3], f[k + 4], f[k + 5], f[k + 6])));
      });
    } else {
      // Set down by hand: the decided face up (a d4 rests on it), turned so
      // the number reads upright, in a row near where the roll came from.
      // Rows wrap and are kept whole on the table.
      var gap = size * 2.5, per = Math.max(1, Math.min(dice.length, Math.floor(st.hw * 1.7 / gap))), rows = Math.ceil(dice.length / per);
      var spanX = (per - 1) * gap / 2, spanZ = (rows - 1) * gap / 2;
      var limX = Math.max(0, st.hw * 0.8 - spanX - size), limZ = Math.max(0, st.hh * 0.6 - spanZ - size);
      var cx = from ? Math.max(-limX, Math.min(limX, from.x * 0.6)) : 0;
      var cz = from ? Math.max(-limZ, Math.min(limZ, from.z * 0.6)) : 0;
      dice.forEach(function (d, i) {
        var f = d.sd.faces[faceFor(d.sd, d.spec, want[i])];
        var q = new T.Quaternion().setFromUnitVectors(new T.Vector3(f.n.x, f.n.y, f.n.z), new T.Vector3(0, d.sd.sides === 4 ? -1 : 1, 0));
        var fu = new T.Vector3(f.up.x, f.up.y, f.up.z).applyQuaternion(q), yaw = d.sd.sides === 4 ? 0 : Math.atan2(fu.x, -fu.z);
        q.premultiply(new T.Quaternion().setFromAxisAngle(new T.Vector3(0, 1, 0), yaw + (i % 2 ? -0.18 : 0.14)));
        d.group.quaternion.copy(q);
        var row = Math.floor(i / per), inRow = row < rows - 1 ? per : dice.length - row * per;
        d.group.position.set(cx + (i % per - (inRow - 1) / 2) * gap, d.rest, cz + row * gap - spanZ);
      });
    }
    var t0 = performance.now(), holdUntil = 0, skip = false, raf = 0, ended = false, qs = new T.Quaternion();
    var onSkip = function () { skip = true; };
    // A click or key press anywhere hurries the dice; it never blocks the page.
    setTimeout(function () { W.addEventListener('pointerdown', onSkip, true); W.addEventListener('keydown', onSkip, true); }, 0);
    var unlisten = function () { W.removeEventListener('pointerdown', onSkip, true); W.removeEventListener('keydown', onSkip, true); };
    var me = { cancel: function () { ended = true; cancelAnimationFrame(raf); unlisten(); disposeStage(st); landed(); } };
    current = me;
    var place = function (idx) {
      dice.forEach(function (d, i) {
        var f = plan.frames[i], k = Math.min(idx, plan.steps - 1) * 7;
        d.group.position.set(f[k], f[k + 1], f[k + 2]);
        d.group.quaternion.copy(qs.set(f[k + 3], f[k + 4], f[k + 5], f[k + 6]).multiply(fix[i]));
      });
    };
    var finish = function () {
      ended = true; unlisten();
      if (current === me) current = null;
      st.canvas.style.opacity = '0';
      setTimeout(function () { disposeStage(st); }, 360);
    };
    var frame = function (now) {
      if (ended) return;
      if (!holdUntil) {
        var idx = plan && !skip ? Math.floor((now - t0) / 1000 / STEP) : Infinity;
        if (plan) place(idx);
        if (!plan || idx >= plan.steps - 1) { holdUntil = now + (skip ? 250 : 1400); landed(); }
      } else if (skip && holdUntil - now > 250) holdUntil = now + 250;
      st.renderer.render(st.scene, st.cam);
      if (holdUntil && now >= holdUntil) { finish(); return; }
      raf = requestAnimationFrame(frame);
    };
    raf = requestAnimationFrame(frame);
  }

  W.Chronicle = W.Chronicle || {};
  W.Chronicle.dice = {
    roll: roll,
    stop: stop,
    // Exposed for test/js/dice.test.mjs.
    _internal: { decide: decide, solid: solid, landedFace: landedFace, symmetry: symmetry, simulate: simulate,
      bodyShape: bodyShape, faceFor: faceFor, faceValue: faceValue, faceLabel: faceLabel }
  };
})();
