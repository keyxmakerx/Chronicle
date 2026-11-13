// Shared helpers: seeded noise, canvas textures and lofted geometry.
import * as THREE from 'three';

export function rng(seed) { let s = seed >>> 0; return function () { s = (Math.imul(s, 1664525) + 1013904223) >>> 0; return s / 4294967296; }; }

// Tileable value noise on an n by n lattice.
export function makeNoise(seed, n) {
  n = n || 64; const r = rng(seed), g = new Float32Array(n * n);
  for (let i = 0; i < g.length; i++) g[i] = r();
  const sm = t => t * t * (3 - 2 * t);
  return function (x, y) {
    x = ((x % n) + n) % n; y = ((y % n) + n) % n;
    const x0 = Math.floor(x), y0 = Math.floor(y), x1 = (x0 + 1) % n, y1 = (y0 + 1) % n, fx = sm(x - x0), fy = sm(y - y0);
    const a = g[y0 * n + x0], b = g[y0 * n + x1], c = g[y1 * n + x0], d = g[y1 * n + x1];
    return a + (b - a) * fx + (c - a) * fy + (a - b - c + d) * fx * fy;
  };
}
export function fbm(noise, x, y, oct) { let a = 0, amp = .5, f = 1; for (let i = 0; i < (oct || 4); i++) { a += amp * noise(x * f, y * f); amp *= .5; f *= 2; } return a; }

// A canvas texture filled per pixel. fn(u, v) returns a grey 0..1 or [r, g, b] 0..1.
export function tex(w, h, fn, opts) {
  opts = opts || {};
  const c = document.createElement('canvas'); c.width = w; c.height = h;
  const ctx = c.getContext('2d'), im = ctx.createImageData(w, h), d = im.data;
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
    const v = fn(x / w, y / h, x, y), i = (y * w + x) * 4;
    if (typeof v === 'number') { d[i] = d[i + 1] = d[i + 2] = Math.max(0, Math.min(255, v * 255)); }
    else { d[i] = Math.max(0, Math.min(255, v[0] * 255)); d[i + 1] = Math.max(0, Math.min(255, v[1] * 255)); d[i + 2] = Math.max(0, Math.min(255, v[2] * 255)); }
    d[i + 3] = 255;
  }
  ctx.putImageData(im, 0, 0);
  if (opts.draw) opts.draw(ctx, w, h);
  const t = new THREE.CanvasTexture(c);
  t.wrapS = t.wrapT = THREE.RepeatWrapping; t.anisotropy = 8; t.flipY = false;
  if (opts.srgb) t.colorSpace = THREE.SRGBColorSpace;
  if (opts.repeat) t.repeat.set(opts.repeat[0], opts.repeat[1]);
  return t;
}

// Skin a stack of rings (arrays of Vector3, all the same length) into a mesh.
// sharp keeps a hard edge between neighbouring ring segments while staying
// smooth along the stack, which is what a ground blade needs.
export function loft(rings, opts) {
  opts = opts || {};
  const R = rings.length, n = rings[0].length, pos = [], uv = [], idx = [];
  if (opts.sharp) {
    for (let i = 0; i < R; i++) for (let j = 0; j < n; j++) {
      const a = rings[i][j], b = rings[i][(j + 1) % n];
      pos.push(a.x, a.y, a.z, b.x, b.y, b.z); uv.push(j / n, i / (R - 1), (j + 1) / n, i / (R - 1));
    }
    for (let i = 0; i < R - 1; i++) for (let j = 0; j < n; j++) {
      const a = (i * n + j) * 2, b = a + 1, c = ((i + 1) * n + j) * 2, d = c + 1;
      idx.push(a, b, c, b, d, c);
    }
  } else {
    for (let i = 0; i < R; i++) for (let j = 0; j <= n; j++) { const p = rings[i][j % n]; pos.push(p.x, p.y, p.z); uv.push(j / n, i / (R - 1)); }
    for (let i = 0; i < R - 1; i++) for (let j = 0; j < n; j++) {
      const a = i * (n + 1) + j, b = a + 1, c = a + n + 1, d = c + 1;
      idx.push(a, b, c, b, d, c);
    }
  }
  const g = new THREE.BufferGeometry();
  g.setAttribute('position', new THREE.Float32BufferAttribute(pos, 3));
  g.setAttribute('uv', new THREE.Float32BufferAttribute(uv, 2));
  g.setIndex(idx);
  g.computeVertexNormals();
  if (!opts.sharp) { // weld the normals across the UV seam
    const nm = g.attributes.normal;
    for (let i = 0; i < R; i++) {
      const a = i * (n + 1), b = a + n;
      const x = nm.getX(a) + nm.getX(b), y = nm.getY(a) + nm.getY(b), z = nm.getZ(a) + nm.getZ(b), l = Math.hypot(x, y, z) || 1;
      nm.setXYZ(a, x / l, y / l, z / l); nm.setXYZ(b, x / l, y / l, z / l);
    }
  }
  if (opts.flip) { const ix = g.index.array; for (let i = 0; i < ix.length; i += 3) { const t = ix[i]; ix[i] = ix[i + 1]; ix[i + 1] = t; } g.computeVertexNormals(); }
  return g;
}

// A ring of points around a centre, in the plane spanned by unit vectors ax and az.
export function ring(c, ax, az, pts) { return pts.map(p => new THREE.Vector3().copy(c).addScaledVector(ax, p[0]).addScaledVector(az, p[1])); }

// A tube along a list of points with a per-point radius; frames are carried
// along the path so the tube never twists on itself.
export function tube(points, radius, sides, closed) {
  const n = points.length, rings = [], up = new THREE.Vector3(0, 0, 1);
  let prevN = null;
  for (let i = 0; i < n; i++) {
    const a = points[closed ? (i - 1 + n) % n : Math.max(0, i - 1)], b = points[closed ? (i + 1) % n : Math.min(n - 1, i + 1)];
    const t = new THREE.Vector3().subVectors(b, a).normalize();
    let nn = prevN ? prevN.clone().addScaledVector(t, -prevN.dot(t)) : new THREE.Vector3().crossVectors(t, Math.abs(t.dot(up)) > .9 ? new THREE.Vector3(1, 0, 0) : up);
    nn.normalize(); prevN = nn;
    const bn = new THREE.Vector3().crossVectors(t, nn);
    const r = typeof radius === 'function' ? radius(i / (n - 1)) : radius, pts = [];
    for (let j = 0; j < sides; j++) { const th = j / sides * Math.PI * 2; pts.push([Math.cos(th) * r, Math.sin(th) * r]); }
    rings.push(ring(points[i], nn, bn, pts));
  }
  if (closed) rings.push(rings[0]);
  return loft(rings);
}
