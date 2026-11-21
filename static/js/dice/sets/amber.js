// ==================== Amber ====================
// Fossil resin, clouded and warm, with things caught in it when it was still sap: strings of bubbles,
// a leaf, a gnat, a winged seed. They hang inside the die at their own depths and angles, so they
// slide past one another as it turns. The heart of the stone holds a deeper glow of its own colour.
import {
  canvas, hexA, mix, mulberry, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// tint: the resin (it also colours the shadow it casts); alpha: how much it hides; deep: the colour of
// its clouds and its heart; caught: the colour of the things inside.
var AMBER = [
  { key: 'honey', name: 'Honey amber', note: 'Clear gold resin, dark inclusions, ivory numbers', tint: '#f0a51e', alpha: 0.34, deep: '#b85c00', caught: '#2a1206', ink: '#fff6dc', burn: '#ffd05a', flame: '#ffb02a',
    base: { grad: ['#ffd98a', '#c97a10'], rough: 0.06, coat: 1 } },
  { key: 'cognac', name: 'Cognac amber', note: 'Deep red-brown resin, ivory numbers', tint: '#b8501a', alpha: 0.5, deep: '#5e1a04', caught: '#1c0802', ink: '#ffe9c4', burn: '#ff9a4a', flame: '#ff7a2a',
    base: { grad: ['#e08a4a', '#7a2c08'], rough: 0.06, coat: 1 } },
  { key: 'cherry', name: 'Cherry amber', note: 'Dark red resin, gold numbers', tint: '#9a1c1c', alpha: 0.56, deep: '#3c0406', caught: '#140203', ink: '#ffd98a', burn: '#ff6a5a', flame: '#ff4a3a',
    base: { grad: ['#d04a44', '#5a0c0e'], rough: 0.06, coat: 1 } },
  { key: 'green', name: 'Green amber', note: 'Olive-green resin, ivory numbers', tint: '#8fa01c', alpha: 0.4, deep: '#3c5206', caught: '#141c04', ink: '#fbffe0', burn: '#d8ff6a', flame: '#b8f03a',
    base: { grad: ['#cfe07a', '#5c7010'], rough: 0.06, coat: 1 } },
  { key: 'blue', name: 'Blue amber', note: 'Rare amber that shines blue in the light, white numbers', tint: '#2a6ad0', alpha: 0.44, deep: '#0a2a7a', caught: '#040c24', ink: '#f0f8ff', burn: '#7ac4ff', flame: '#5aa8ff',
    base: { grad: ['#8fc0ff', '#16409a'], rough: 0.06, coat: 1 } },
  { key: 'butter', name: 'Butterscotch amber', note: 'Milky yellow resin, brown numbers', tint: '#f0d078', alpha: 0.72, deep: '#d09a2a', caught: '#4a2a0a', ink: '#4a2408', keyline: '#fff6d8', burn: '#ff9a2a', flame: '#ff8a1a',
    base: { grad: ['#fff0c0', '#d8a83a'], rough: 0.08, coat: 1 } }
];

// ---------- The things caught inside ----------
// One image, four cells: a bubble, a leaf, a gnat, a winged seed.
var amberTexCache = {};
function amberTex(T, c) {
  if (amberTexCache[c.key]) return amberTexCache[c.key];
  var S = 128, cvs = canvas(S * 2), g = cvs.getContext('2d'), ink = c.caught, i;
  // The bubble: a bright ring, brighter on the side the light comes from, and a glint.
  var gr = g.createRadialGradient(S * 0.5, S * 0.5, S * 0.24, S * 0.5, S * 0.5, S * 0.44);
  gr.addColorStop(0, 'rgba(255,250,230,0.05)'); gr.addColorStop(0.75, 'rgba(255,250,230,0.5)'); gr.addColorStop(1, 'rgba(255,250,230,0)');
  g.fillStyle = gr; g.fillRect(0, 0, S, S);
  g.fillStyle = 'rgba(255,255,255,.9)'; g.beginPath(); g.ellipse(S * 0.36, S * 0.34, S * 0.07, S * 0.04, -0.7, 0, 7); g.fill();
  // The leaf: a blade with a midrib and veins.
  g.save(); g.translate(S * 1.5, S * 0.5); g.rotate(0.5);
  g.fillStyle = hexA(ink, 0.86); g.beginPath(); g.moveTo(-S * 0.42, 0); g.quadraticCurveTo(0, -S * 0.3, S * 0.42, 0); g.quadraticCurveTo(0, S * 0.3, -S * 0.42, 0); g.fill();
  g.strokeStyle = hexA(mix(ink, c.tint, 0.6), 0.9); g.lineWidth = 1.6; g.beginPath(); g.moveTo(-S * 0.44, 0); g.lineTo(S * 0.4, 0);
  for (i = -3; i <= 3; i++) { g.moveTo(i * S * 0.1, 0); g.lineTo(i * S * 0.1 + S * 0.08, -S * 0.1 * (1 - Math.abs(i) / 5)); g.moveTo(i * S * 0.1, 0); g.lineTo(i * S * 0.1 + S * 0.08, S * 0.1 * (1 - Math.abs(i) / 5)); }
  g.stroke(); g.restore();
  // The gnat: head, body in two parts, six legs, two clear wings.
  g.save(); g.translate(S * 0.5, S * 1.5); g.rotate(-0.4);
  g.fillStyle = hexA(mix(ink, c.tint, 0.5), 0.4); g.beginPath(); g.ellipse(-S * 0.06, -S * 0.2, S * 0.27, S * 0.1, -0.5, 0, 7); g.ellipse(-S * 0.06, S * 0.2, S * 0.27, S * 0.1, 0.5, 0, 7); g.fill();
  g.strokeStyle = hexA(ink, 0.9); g.lineWidth = 1.5; g.lineCap = 'round'; g.beginPath();
  for (i = -1; i <= 1; i++) { g.moveTo(i * S * 0.07, 0); g.quadraticCurveTo(i * S * 0.12, -S * 0.16, i * S * 0.2 + S * 0.04, -S * 0.3); g.moveTo(i * S * 0.07, 0); g.quadraticCurveTo(i * S * 0.12, S * 0.16, i * S * 0.2 + S * 0.04, S * 0.3); }
  g.moveTo(S * 0.24, -S * 0.03); g.lineTo(S * 0.38, -S * 0.12); g.moveTo(S * 0.24, S * 0.03); g.lineTo(S * 0.38, S * 0.12); g.stroke();
  g.fillStyle = hexA(ink, 0.94); g.beginPath(); g.ellipse(-S * 0.13, 0, S * 0.18, S * 0.06, 0, 0, 7); g.ellipse(S * 0.09, 0, S * 0.09, S * 0.07, 0, 0, 7); g.ellipse(S * 0.22, 0, S * 0.05, S * 0.05, 0, 0, 7); g.fill();
  g.restore();
  // The winged seed: a nut and one long papery wing.
  g.save(); g.translate(S * 1.5, S * 1.5); g.rotate(-0.7);
  g.fillStyle = hexA(mix(ink, c.tint, 0.35), 0.55); g.beginPath(); g.moveTo(-S * 0.2, -S * 0.05); g.quadraticCurveTo(S * 0.2, -S * 0.26, S * 0.44, -S * 0.02); g.quadraticCurveTo(S * 0.2, S * 0.14, -S * 0.2, S * 0.07); g.fill();
  g.strokeStyle = hexA(ink, 0.5); g.lineWidth = 1; g.beginPath(); for (i = 0; i < 6; i++) { g.moveTo(-S * 0.16, 0); g.quadraticCurveTo(S * 0.1, -S * 0.16 + i * S * 0.05, S * 0.4, -S * 0.05 + i * S * 0.02); } g.stroke();
  g.fillStyle = hexA(ink, 0.94); g.beginPath(); g.ellipse(-S * 0.28, 0, S * 0.13, S * 0.09, 0.2, 0, 7); g.fill();
  g.restore();
  var t = new T.CanvasTexture(cvs); t.colorSpace = T.SRGBColorSpace || t.colorSpace; t.anisotropy = 4;
  amberTexCache[c.key] = t; return t;
}
var amberGlow = null;
function amberGlowTex(T) {
  if (amberGlow) return amberGlow;
  var cvs = canvas(128), g = cvs.getContext('2d'), gr = g.createRadialGradient(64, 64, 0, 64, 64, 64);
  gr.addColorStop(0, 'rgba(255,255,255,1)'); gr.addColorStop(0.4, 'rgba(255,255,255,.5)'); gr.addColorStop(1, 'rgba(255,255,255,0)');
  g.fillStyle = gr; g.fillRect(0, 0, 128, 128);
  amberGlow = new T.CanvasTexture(cvs); return amberGlow;
}
var amberCount = 0;
// Flat cards set at every angle inside the die: each shows one cell of the image.
function amberBuild(T, die, q) {
  var c = die.th, hd = q.detail !== 'basic', rnd = mulberry(++amberCount * 613 + die.sides * 17), R = die.rest, pos = [], uv = [], idx = [], n = 0;
  var qn = new T.Quaternion(), e = new T.Euler(), v = new T.Vector3();
  function card(cell, size, reach) {
    var a = rnd() * 6.2832, z = rnd() * 2 - 1, s = Math.sqrt(1 - z * z), d = Math.cbrt(rnd()) * reach;
    var cx = Math.cos(a) * s * d, cy = z * d, cz = Math.sin(a) * s * d, u0 = (cell % 2) * 0.5, v0 = cell < 2 ? 0.5 : 0;
    e.set(rnd() * 6.28, rnd() * 6.28, rnd() * 6.28); qn.setFromEuler(e);
    [[-1, -1, 0, 0], [1, -1, 1, 0], [1, 1, 1, 1], [-1, 1, 0, 1]].forEach(function (k) {
      v.set(k[0] * size, k[1] * size, 0).applyQuaternion(qn); pos.push(cx + v.x, cy + v.y, cz + v.z); uv.push(u0 + k[2] * 0.5, v0 + k[3] * 0.5);
    });
    idx.push(n, n + 1, n + 2, n, n + 2, n + 3); n += 4;
  }
  // One or two large things near the middle, then bubbles in strings all through.
  var big = [1, 2, 3].sort(function () { return rnd() - 0.5; });
  card(big[0], R * 0.36, R * 0.2); if (hd) card(big[1], R * 0.27, R * 0.4);
  for (var i = 0, nb = hd ? 26 : 12; i < nb; i++) card(0, R * (0.035 + rnd() * rnd() * 0.11), R * 0.72);
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setAttribute('uv', new T.Float32BufferAttribute(uv, 2)); geo.setIndex(idx);
  var mat = new T.MeshBasicMaterial({ map: amberTex(T, c), transparent: true, depthWrite: false, side: T.DoubleSide });
  var mesh = new T.Mesh(geo, mat); mesh.renderOrder = 1.5; mesh.frustumCulled = false;
  // The heart: a soft ball of the resin's deeper colour, which gives the stone its depth.
  var gm = new T.SpriteMaterial({ map: amberGlowTex(T), color: new T.Color(c.deep), transparent: true, depthWrite: false, opacity: 0.62 });
  var glow = new T.Sprite(gm); glow.scale.setScalar(R * 2.1); glow.renderOrder = 1.4;
  var t = rnd() * 20;
  return { parts: [glow, mesh], tick: function (dt) { t += Math.min(dt, 0.05); gm.opacity = 0.58 + 0.08 * Math.sin(t * 0.9); }, dispose: function () { geo.dispose(); mat.dispose(); gm.dispose(); } };
}

registerSet({
  key: 'amber', name: 'Amber', note: 'Clouded fossil resin with bubbles, a leaf, a gnat or a winged seed caught inside, hanging at their own depths as the die turns.',
  show: 'kindle', bev: 0.085, see: 'glass', dense: true,
  colours: AMBER,
  paint: function (g, lay, c, seed) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay) * 1.06, rnd = mulberry(seed);
    g.clearRect(0, 0, cv, cv);
    g.globalAlpha = c.alpha; g.fillStyle = mix(c.tint, '#000000', 0.2); g.fillRect(0, 0, cv, cv); g.globalAlpha = 1;
    // Clouds: the resin is thicker in drifts, and those hide a little more.
    g.save(); try { g.filter = 'blur(' + (11 * k) + 'px)'; } catch (e) {}
    for (var i = 0; i < 7; i++) { g.fillStyle = hexA(rnd() > 0.4 ? c.deep : mix(c.tint, '#ffffff', 0.4), 0.22); g.beginPath(); g.ellipse(rnd() * cv, rnd() * cv, (16 + rnd() * 30) * k, (8 + rnd() * 14) * k, rnd() * 3.14, 0, 7); g.fill(); }
    g.restore();
    // The number: cut in and filled with paint, with a dark line round it so it reads over a pale page.
    g.lineJoin = 'round';
    g.fillStyle = g.strokeStyle = hexA(c.keyline || mix(c.deep, '#000000', 0.6), 0.92); paintNumeral(g, lay, lay.style, w * 1.6);
    g.fillStyle = g.strokeStyle = c.ink; paintNumeral(g, lay, lay.style, w);
  },
  surface: function (g, lay) {
    var w = strokeW(lay) * 1.06;
    g.fillStyle = g.strokeStyle = 'rgb(0,120,0)'; paintNumeral(g, lay, lay.style, w);
  },
  height: function (g, lay) {
    var w = strokeW(lay) * 1.06;
    g.fillStyle = g.strokeStyle = '#8a8a8a'; paintNumeral(g, lay, lay.style, w * 1.5);
    g.fillStyle = g.strokeStyle = '#6a6a6a'; paintNumeral(g, lay, lay.style, w);
  },
  rim: function (c) { return { col: hexA(mix(c.tint, '#000000', 0.3), Math.min(0.9, c.alpha + 0.16)), rough: 0.05, metal: 0 }; },
  physical: function (c, q) { return { clearcoat: q.detail === 'basic' ? 0 : 0.5, clearcoatRoughness: 0.08, envMapIntensity: 1.1 }; },
  // Seen through the die, the far side's numbers are mirrored and would crowd the ones that matter:
  // on the far side's copy they drop to a faint ghost.
  shader: function (s, c) {
    if (this.side !== 1) return;
    s.uniforms.uFarA = { value: c.alpha };
    s.fragmentShader = 'uniform float uFarA;\n' + s.fragmentShader.replace('float glassSpec',
      'float farKeep = smoothstep( uFarA + 0.12, uFarA + 0.4, diffuseColor.a );\ndiffuseColor.a = mix( diffuseColor.a, uFarA * 0.8, farKeep );\nfloat glassSpec');
  },
  build: amberBuild,
  cover: function () { return 0.5; }
});
