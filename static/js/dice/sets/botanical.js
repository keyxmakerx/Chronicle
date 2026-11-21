// ==================== Pressed flowers ====================
// Clear casting resin with a small garden set inside: pressed blossoms, fern tips, loose petals and
// flakes of metal leaf, each lying at its own depth and angle. Nothing is painted on the faces but
// the number, so the flowers show through every side and slide past one another as the die turns.
import {
  canvas, hexA, mix, mulberry, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// tint and alpha: the resin; bloom: [large flower, its heart, small flower, loose petal]; leaf: the
// greenery; foil: the metal leaf; fey and grow: the plant the Overgrow show grows.
var BOTANICAL = [
  { key: 'meadow', name: 'Wild meadow', note: 'Clear resin, daisies and forget-me-nots, gold leaf', tint: '#eef6f2', alpha: 0.12, ink: '#1e2a22', keyline: '#ffffff', burn: '#b8ffb0', fey: '#3f9a4a', grow: '#3f9a4a',
    bloom: ['#ffffff', '#f2b21c', '#5a8ef0', '#ff9ac0'], leaf: '#4f9a3c', foil: ['#fff0b0', '#e6b84a'], base: { grad: ['#f4faf6', '#cfe4d8'], rough: 0.05, coat: 1 } },
  { key: 'rose', name: 'Rose garden', note: 'Blush resin, pink and red roses, gold leaf', tint: '#ffd9e0', alpha: 0.16, ink: '#fff6f0', keyline: '#6a1428', burn: '#ff9ab8', fey: '#b83a5a', grow: '#3f8a4a',
    bloom: ['#f0527a', '#ffd0dc', '#b8142f', '#ffb0c4'], leaf: '#3f8a4a', foil: ['#fff0b0', '#e6b84a'], base: { grad: ['#fff0f3', '#f4b8c6'], rough: 0.05, coat: 1 } },
  { key: 'lavender', name: 'Lavender field', note: 'Clear resin, violet sprigs, silver leaf', tint: '#ece6fa', alpha: 0.13, ink: '#2a1c4a', keyline: '#ffffff', burn: '#c8a8ff', fey: '#7a52c8', grow: '#5c8a52',
    bloom: ['#8f62e0', '#f0e0ff', '#c9a8f6', '#6a3fc0'], leaf: '#6f9a62', foil: ['#ffffff', '#cfd6e2'], base: { grad: ['#f4f0fc', '#d2c4ee'], rough: 0.05, coat: 1 } },
  { key: 'fern', name: 'Fern hollow', note: 'Pale green resin, ferns and white stars, copper leaf', tint: '#dff2e0', alpha: 0.16, ink: '#12301c', keyline: '#f4fff2', burn: '#9affb0', fey: '#2f7a3c', grow: '#2f7a3c',
    bloom: ['#f6fff0', '#d8c84a', '#ffffff', '#bfe8a8'], leaf: '#2f7a3c', foil: ['#ffd0b0', '#d9824a'], base: { grad: ['#ecf8ec', '#b8dcbc'], rough: 0.05, coat: 1 } },
  { key: 'autumn', name: 'Autumn walk', note: 'Warm resin, orange and red leaves, gold leaf', tint: '#ffe6c4', alpha: 0.18, ink: '#3a1a08', keyline: '#fff4e0', burn: '#ffb45c', fey: '#c8601c', grow: '#c8601c',
    bloom: ['#e8701c', '#ffd23f', '#b8281c', '#f0a030'], leaf: '#a8641c', foil: ['#fff0b0', '#e6b84a'], base: { grad: ['#fff4e2', '#f0c690'], rough: 0.05, coat: 1 } },
  { key: 'night', name: 'Night garden', note: 'Smoked resin, white moonflowers, silver leaf', tint: '#1c2038', alpha: 0.5, ink: '#f2f4ff', keyline: '#05060c', burn: '#bcd0ff', fey: '#8fa8e8', grow: '#5a7ac0',
    bloom: ['#f4f6ff', '#bcd0ff', '#d8c8ff', '#ffffff'], leaf: '#6a8ab8', foil: ['#ffffff', '#cfd6e2'], base: { grad: ['#3a4068', '#0c0e1c'], rough: 0.05, coat: 1 } }
];

// ---------- The garden ----------
// One image, eight cells: large flower, small flower, fern tip, leaf sprig, petal, petal, metal leaf, bud.
var botTexCache = {};
function botTex(T, c) {
  if (botTexCache[c.key]) return botTexCache[c.key];
  var S = 128, cvs = canvas(S * 4), g = cvs.getContext('2d'), i, a, B = c.bloom;
  function cell(n, fn) { g.save(); g.translate((n % 4) * S + S / 2, Math.floor(n / 4) * S + S / 2); fn(); g.restore(); }
  function petalRing(n, r0, r1, wd, col, edge) {
    for (var j = 0; j < n; j++) {
      g.save(); g.rotate(j / n * 6.2832);
      var gr = g.createLinearGradient(r0, 0, r1, 0); gr.addColorStop(0, edge); gr.addColorStop(0.35, col); gr.addColorStop(1, mix(col, '#ffffff', 0.25));
      g.fillStyle = gr; g.beginPath(); g.moveTo(r0, 0); g.bezierCurveTo(r0 + (r1 - r0) * 0.4, -wd, r1 * 0.96, -wd * 0.8, r1, 0); g.bezierCurveTo(r1 * 0.96, wd * 0.8, r0 + (r1 - r0) * 0.4, wd, r0, 0); g.fill();
      g.strokeStyle = hexA(mix(col, '#000000', 0.45), 0.5); g.lineWidth = 0.8; g.stroke();
      g.beginPath(); g.moveTo(r0, 0); g.lineTo(r1 * 0.8, 0); g.stroke();
      g.restore();
    }
  }
  // A large flower with many narrow petals and a seeded heart.
  cell(0, function () {
    petalRing(14, S * 0.1, S * 0.46, S * 0.075, B[0], mix(B[0], B[1], 0.5));
    g.fillStyle = B[1]; g.beginPath(); g.arc(0, 0, S * 0.13, 0, 7); g.fill();
    for (i = 0; i < 26; i++) { a = i * 2.4; var d = Math.sqrt(i / 26) * S * 0.11; g.fillStyle = hexA(mix(B[1], '#000000', 0.45), 0.7); g.beginPath(); g.arc(Math.cos(a) * d, Math.sin(a) * d, 1.4, 0, 7); g.fill(); }
  });
  // A small five-petalled flower.
  cell(1, function () {
    petalRing(5, S * 0.06, S * 0.4, S * 0.2, B[2], mix(B[2], '#ffffff', 0.5));
    g.fillStyle = B[1]; g.beginPath(); g.arc(0, 0, S * 0.08, 0, 7); g.fill();
  });
  // A fern tip: a stem with paired leaflets that shorten toward the point.
  cell(2, function () {
    g.rotate(-0.3); g.strokeStyle = mix(c.leaf, '#000000', 0.3); g.lineWidth = 2; g.lineCap = 'round'; g.beginPath(); g.moveTo(-S * 0.44, 0); g.quadraticCurveTo(0, -S * 0.08, S * 0.44, 0); g.stroke();
    for (i = 0; i < 11; i++) {
      var u = i / 10, x = -S * 0.4 + u * S * 0.8, y = -S * 0.08 * (1 - (2 * u - 1) * (2 * u - 1)) * 0.5, l = S * 0.24 * (1 - u * 0.8);
      [1, -1].forEach(function (sg) { g.fillStyle = mix(c.leaf, '#ffffff', 0.1 + u * 0.2); g.beginPath(); g.moveTo(x, y); g.quadraticCurveTo(x + l * 0.5, y + sg * l * 0.35, x + l * 0.45, y + sg * l); g.quadraticCurveTo(x - l * 0.1, y + sg * l * 0.5, x, y); g.fill(); });
    }
  });
  // A sprig: three leaves on a stalk.
  cell(3, function () {
    g.strokeStyle = mix(c.leaf, '#000000', 0.3); g.lineWidth = 2; g.beginPath(); g.moveTo(-S * 0.4, S * 0.2); g.quadraticCurveTo(0, S * 0.1, S * 0.3, -S * 0.3); g.stroke();
    [[-S * 0.12, S * 0.13, -1.3, 0.3], [S * 0.08, 0, 0.3, 0.32], [S * 0.3, -S * 0.3, -0.7, 0.26]].forEach(function (q) {
      g.save(); g.translate(q[0], q[1]); g.rotate(q[2]); var L = S * q[3];
      g.fillStyle = c.leaf; g.beginPath(); g.moveTo(0, 0); g.quadraticCurveTo(L * 0.5, -L * 0.42, L, 0); g.quadraticCurveTo(L * 0.5, L * 0.42, 0, 0); g.fill();
      g.strokeStyle = hexA(mix(c.leaf, '#ffffff', 0.5), 0.7); g.lineWidth = 1; g.beginPath(); g.moveTo(0, 0); g.lineTo(L * 0.9, 0); g.stroke(); g.restore();
    });
  });
  // Loose petals.
  [4, 5].forEach(function (n) { cell(n, function () {
    var col = n === 4 ? B[3] : B[0]; g.rotate(n);
    var gr = g.createLinearGradient(-S * 0.36, 0, S * 0.36, 0); gr.addColorStop(0, mix(col, '#ffffff', 0.4)); gr.addColorStop(1, col);
    g.fillStyle = gr; g.beginPath(); g.moveTo(-S * 0.38, 0); g.bezierCurveTo(-S * 0.1, -S * 0.34, S * 0.34, -S * 0.26, S * 0.38, 0); g.bezierCurveTo(S * 0.34, S * 0.26, -S * 0.1, S * 0.34, -S * 0.38, 0); g.fill();
    g.strokeStyle = hexA(mix(col, '#000000', 0.4), 0.4); g.lineWidth = 0.8; for (i = -2; i <= 2; i++) { g.beginPath(); g.moveTo(-S * 0.34, 0); g.quadraticCurveTo(0, i * S * 0.1, S * 0.32, i * S * 0.07); g.stroke(); }
  }); });
  // Metal leaf: a torn scrap, bright with creases.
  cell(6, function () {
    var rnd = mulberry(77), pts = [];
    for (i = 0; i < 9; i++) { a = i / 9 * 6.2832; var r = S * (0.22 + rnd() * 0.22); pts.push([Math.cos(a) * r, Math.sin(a) * r]); }
    var gr = g.createLinearGradient(-S * 0.4, -S * 0.4, S * 0.4, S * 0.4); gr.addColorStop(0, c.foil[0]); gr.addColorStop(0.5, c.foil[1]); gr.addColorStop(0.62, c.foil[0]); gr.addColorStop(1, mix(c.foil[1], '#000000', 0.25));
    g.fillStyle = gr; g.beginPath(); pts.forEach(function (p, j) { if (j) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.closePath(); g.fill();
    g.strokeStyle = hexA('#ffffff', 0.6); g.lineWidth = 1; g.beginPath(); g.moveTo(-S * 0.2, -S * 0.1); g.lineTo(S * 0.16, S * 0.2); g.moveTo(-S * 0.05, -S * 0.25); g.lineTo(S * 0.2, -S * 0.02); g.stroke();
  });
  // A bud on a short stalk.
  cell(7, function () {
    g.strokeStyle = mix(c.leaf, '#000000', 0.3); g.lineWidth = 2.4; g.beginPath(); g.moveTo(-S * 0.4, S * 0.26); g.quadraticCurveTo(-S * 0.1, S * 0.2, S * 0.08, -S * 0.02); g.stroke();
    g.fillStyle = B[2]; g.beginPath(); g.ellipse(S * 0.18, -S * 0.14, S * 0.2, S * 0.13, -0.75, 0, 7); g.fill();
    g.fillStyle = c.leaf; g.beginPath(); g.ellipse(S * 0.07, -S * 0.02, S * 0.1, S * 0.07, -0.75, 0, 7); g.fill();
  });
  var t = new T.CanvasTexture(cvs); t.colorSpace = T.SRGBColorSpace || t.colorSpace; t.anisotropy = 4;
  botTexCache[c.key] = t; return t;
}
var botCount = 0;
// Flat pressed things set at every angle inside the die: each card shows one cell of the image.
function botBuild(T, die, q) {
  var c = die.th, hd = q.detail !== 'basic', rnd = mulberry(++botCount * 421 + die.sides * 29), R = die.rest, pos = [], uv = [], idx = [], n = 0;
  var qn = new T.Quaternion(), e = new T.Euler(), v = new T.Vector3();
  function card(cell, size, reach) {
    var a = rnd() * 6.2832, z = rnd() * 2 - 1, s = Math.sqrt(1 - z * z), d = Math.cbrt(rnd()) * reach;
    var cx = Math.cos(a) * s * d, cy = z * d, cz = Math.sin(a) * s * d, u0 = (cell % 4) * 0.25, v0 = cell < 4 ? 0.75 : 0.5;
    e.set(rnd() * 6.28, rnd() * 6.28, rnd() * 6.28); qn.setFromEuler(e);
    [[-1, -1, 0, 0], [1, -1, 1, 0], [1, 1, 1, 1], [-1, 1, 0, 1]].forEach(function (k) {
      v.set(k[0] * size, k[1] * size, 0).applyQuaternion(qn); pos.push(cx + v.x, cy + v.y, cz + v.z); uv.push(u0 + k[2] * 0.25, v0 + k[3] * 0.25);
    });
    idx.push(n, n + 1, n + 2, n, n + 2, n + 3); n += 4;
  }
  // Greenery deepest, then the flowers, then petals and metal leaf nearest the faces.
  var i;
  for (i = 0; i < (hd ? 4 : 2); i++) card(rnd() > 0.5 ? 2 : 3, R * (0.42 + rnd() * 0.14), R * 0.3);
  for (i = 0; i < (hd ? 3 : 2); i++) card(0, R * (0.36 + rnd() * 0.14), R * 0.4);
  for (i = 0; i < (hd ? 5 : 2); i++) card(rnd() > 0.35 ? 1 : 7, R * (0.24 + rnd() * 0.12), R * 0.56);
  for (i = 0; i < (hd ? 8 : 4); i++) card(rnd() > 0.5 ? 4 : 5, R * (0.1 + rnd() * 0.08), R * 0.7);
  for (i = 0; i < (hd ? 9 : 4); i++) card(6, R * (0.05 + rnd() * 0.08), R * 0.74);
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setAttribute('uv', new T.Float32BufferAttribute(uv, 2)); geo.setIndex(idx);
  var mat = new T.MeshBasicMaterial({ map: botTex(T, c), transparent: true, depthWrite: false, side: T.DoubleSide, alphaTest: 0.04 });
  var mesh = new T.Mesh(geo, mat); mesh.renderOrder = 1.5; mesh.frustumCulled = false;
  return { parts: [mesh], tick: null, dispose: function () { geo.dispose(); mat.dispose(); } };
}

registerSet({
  key: 'botanical', name: 'Pressed flowers', note: 'Clear resin with pressed blossoms, fern tips, petals and metal leaf set inside, showing through every side.',
  show: 'overgrow', bev: 0.07, see: 'glass',
  colours: BOTANICAL,
  paint: function (g, lay, c) {
    var cv = lay.cv, w = strokeW(lay) * 1.08;
    g.clearRect(0, 0, cv, cv);
    g.globalAlpha = c.alpha; g.fillStyle = mix(c.tint, '#000000', 0.25); g.fillRect(0, 0, cv, cv); g.globalAlpha = 1;
    // The number is painted on the face: a keyline so it reads over the flowers and the page, then the ink.
    g.lineJoin = 'round';
    g.fillStyle = g.strokeStyle = hexA(c.keyline, 0.94); paintNumeral(g, lay, lay.style, w * 1.75);
    g.fillStyle = g.strokeStyle = c.ink; paintNumeral(g, lay, lay.style, w * 1.04);
  },
  surface: function (g, lay) {
    var w = strokeW(lay) * 1.08;
    g.fillStyle = g.strokeStyle = 'rgb(0,130,0)'; paintNumeral(g, lay, lay.style, w * 1.75);
  },
  height: function (g, lay) {
    var w = strokeW(lay) * 1.08;
    g.fillStyle = g.strokeStyle = '#9a9a9a'; paintNumeral(g, lay, lay.style, w * 1.75);
  },
  rim: function (c) { return { col: hexA(mix(c.tint, '#000000', 0.5), Math.max(0.24, c.alpha + 0.1)), rough: 0.04, metal: 0 }; },
  physical: function (c, q) { return { clearcoat: q.detail === 'basic' ? 0 : 0.4, clearcoatRoughness: 0.06, envMapIntensity: 1.05 }; },
  // Seen through the die, the far side's numbers are mirrored and would crowd the ones that matter:
  // on the far side's copy they drop to a faint ghost.
  shader: function (s, c) {
    if (this.side !== 1) return;
    s.uniforms.uFarA = { value: c.alpha };
    s.fragmentShader = 'uniform float uFarA;\n' + s.fragmentShader.replace('float glassSpec',
      'float farKeep = smoothstep( uFarA + 0.12, uFarA + 0.4, diffuseColor.a );\ndiffuseColor.a = mix( diffuseColor.a, uFarA * 0.8, farKeep );\nfloat glassSpec');
  },
  build: botBuild,
  cover: function (c) { return c.key === 'night' ? 0.2 : 0.55; }
});
