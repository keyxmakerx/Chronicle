// ==================== Neon ====================
// Black gloss with glass tubes of light bent into the numbers, and a thinner tube of a second colour
// running round every face. The tubes are lit from inside: they glow on a dark page and a bright one
// alike, hum a little, and now and then one flickers the way a real sign does.
import {
  hexA, mix, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// tube: the number's light; edge: the light round the face; body: the gloss behind them.
var NEON = [
  { key: 'arcade', name: 'Pink and cyan', note: 'Pink numbers, cyan edges', burn: '#ff7ad0', ink: '#ff5ac4',
    tube: '#ff3fb8', edge: '#2fe6ff', body: '#0c0a12', base: { grad: ['#3a1638', '#07060a'], rough: 0.14 } },
  { key: 'ice', name: 'Ice blue and white', note: 'Blue numbers, white edges', burn: '#8fd0ff', ink: '#5ab8ff',
    tube: '#3fa8ff', edge: '#e8f4ff', body: '#080b12', base: { grad: ['#14284a', '#05070a'], rough: 0.14 } },
  { key: 'toxic', name: 'Acid green and violet', note: 'Green numbers, violet edges', burn: '#b8ff7a', ink: '#8aff4a',
    tube: '#6aff2a', edge: '#b05aff', body: '#090c0a', base: { grad: ['#1c3a16', '#050705'], rough: 0.14 } },
  { key: 'sunset', name: 'Orange and magenta', note: 'Orange numbers, magenta edges', burn: '#ffb45c', ink: '#ff9a3a',
    tube: '#ff8a1a', edge: '#ff2f8a', body: '#100a08', base: { grad: ['#4a2210', '#0a0605'], rough: 0.14 } },
  { key: 'ultraviolet', name: 'Violet and gold', note: 'Violet numbers, gold edges', burn: '#c8a0ff', ink: '#b07aff',
    tube: '#9a4aff', edge: '#ffc83a', body: '#0b0912', base: { grad: ['#2a1650', '#06050a'], rough: 0.14 } },
  { key: 'ghost', name: 'White and red', note: 'White numbers, red edges', burn: '#ffffff', ink: '#f4f6ff',
    tube: '#eef2ff', edge: '#ff2a3a', body: '#0a0a0c', base: { grad: ['#34343c', '#060607'], rough: 0.14 } }
];

function neonInset(lay, k) { var cx = lay.cv / 2, cy = lay.cv / 2; return lay.inner.map(function (p) { return [cx + (p[0] - cx) * k, cy + (p[1] - cy) * k]; }); }
// The edge tube: the face's outline moved in, with its corners rounded as bent glass is.
function neonEdge(g, lay, k) {
  var pts = neonInset(lay, 0.86), n = pts.length, r = 9 * k;
  g.beginPath();
  for (var i = 0; i < n; i++) {
    var a = pts[i], b = pts[(i + 1) % n], c = pts[(i + 2) % n];
    if (!i) g.moveTo((a[0] + b[0]) / 2, (a[1] + b[1]) / 2);
    g.arcTo(b[0], b[1], (b[0] + c[0]) / 2, (b[1] + c[1]) / 2, r); g.lineTo((b[0] + c[0]) / 2, (b[1] + c[1]) / 2);
  }
  g.closePath();
}
// One lit tube: its colour spilled on the gloss round it, the tube, and the white-hot gas down its middle.
function neonTube(g, col, k, wide, path) {
  g.save(); try { g.filter = 'blur(' + (5 * k) + 'px)'; } catch (e) {}
  g.strokeStyle = g.fillStyle = hexA(col, 0.75); path(wide * 2.6); g.restore();
  g.strokeStyle = g.fillStyle = col; path(wide);
  g.strokeStyle = g.fillStyle = mix(col, '#ffffff', 0.78); path(wide * 0.42);
}

// The face image marks how brightly each point is lit with its alpha: gloss is solid, a tube is the
// most see-through, and its spilled light falls between. The body is never see-through, so the alpha
// is free to carry this.
function neonShader(s) {
  var m = this;
  s.uniforms.uNeT = m.userData.neT; s.uniforms.uNeGain = m.userData.neGain; s.uniforms.uNeOff = { value: m.userData.neOff || 0 };
  s.fragmentShader = 'uniform float uNeT, uNeGain, uNeOff;\n' +
    s.fragmentShader.replace('#include <lights_physical_fragment>', [
      'float neLit = clamp( ( 1.0 - sampledDiffuseColor.a ) / 0.42, 0.0, 1.0 );',
      // A steady hum, and a rare stutter: for a few frames every so often the gas half fails.
      'float neTick = floor( uNeT * 14.0 + uNeOff );',
      'float neDrop = step( 0.985, fract( sin( neTick * 12.9898 + uNeOff ) * 43758.5453 ) );',
      'float neHum = ( 0.94 + 0.06 * sin( uNeT * 50.0 + uNeOff ) ) * ( 1.0 - 0.55 * neDrop );',
      'totalEmissiveRadiance += sampledDiffuseColor.rgb * neLit * neLit * uNeGain * neHum * diffuse.g;',
      // Unlit, a tube is clear glass over the gloss: its own colour is kept low so the light is what shows.
      'diffuseColor.rgb *= 1.0 - 0.6 * neLit;',
      '#include <lights_physical_fragment>'].join('\n'));
}

registerSet({
  key: 'neon', name: 'Neon', note: 'Black gloss with the numbers in glowing glass tube and a second colour running round every face. The tubes hum, and now and then one flickers.',
  show: 'inscribe', bev: 0.06, see: null, relief: 1.4,
  colours: NEON,
  paint: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay);
    g.fillStyle = c.body; g.fillRect(0, 0, cv, cv);
    g.lineJoin = 'round'; g.lineCap = 'round';
    function edge(wd) { g.lineWidth = wd; neonEdge(g, lay, k); g.stroke(); }
    function num(wd) { paintNumeral(g, lay, lay.style, wd); }
    neonTube(g, c.edge, k, 1.7 * k, edge);
    neonTube(g, c.tube, k, w * 0.74, num);
    // Now mark the light in the alpha: cut the image part-way through wherever a tube or its spill lies.
    g.globalCompositeOperation = 'destination-out';
    g.save(); try { g.filter = 'blur(' + (4.5 * k) + 'px)'; } catch (e) {}
    g.strokeStyle = g.fillStyle = 'rgba(0,0,0,.2)'; edge(1.7 * k * 2.4); num(w * 1.7); g.restore();
    g.strokeStyle = g.fillStyle = 'rgba(0,0,0,.3)'; edge(1.7 * k); num(w * 0.74);
    g.globalCompositeOperation = 'source-over';
  },
  // Gloss everywhere; the tubes are smooth glass too.
  surface: function (g, lay, c) {
    g.fillStyle = 'rgb(0,' + Math.round(c.base.rough * 255) + ',0)'; g.fillRect(0, 0, lay.cv, lay.cv);
  },
  // The tubes stand off the face.
  height: function (g, lay) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay);
    g.fillStyle = '#707070'; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (1.1 * k) + 'px)'; } catch (e) {}
    g.lineJoin = 'round'; g.lineCap = 'round'; g.strokeStyle = g.fillStyle = '#ffffff';
    g.lineWidth = 1.9 * k; neonEdge(g, lay, k); g.stroke();
    paintNumeral(g, lay, lay.style, w * 0.74);
    g.restore();
  },
  rim: function (c) { return { col: mix(c.body, c.edge, 0.16), rough: 0.12, metal: 0 }; },
  physical: function (c, q) { return { clearcoat: q.detail === 'basic' ? 0 : 1, clearcoatRoughness: 0.06, envMapIntensity: 1.1, userData: { neT: { value: 0 }, neGain: { value: 1.5 } } }; },
  shader: neonShader,
  build: function (T, die) {
    var t = Math.random() * 40, s = 0, off = Math.random() * 90;
    die.mats.forEach(function (m) { m.userData.neOff = off; });
    var uT = die.mat.userData.neT, uG = die.mat.userData.neGain;
    return { parts: [], tick: function (dt, spin) {
      dt = Math.min(dt, 0.05); t += dt; uT.value = t;
      // A tumbling die burns a little brighter, and eases back as it comes to rest.
      var want = Math.min(1, Math.max(0, spin / 10)); s += (want - s) * (1 - Math.exp(-dt * 3));
      uG.value = 1.5 + 0.9 * s;
    }, dispose: function () {} };
  }
});
