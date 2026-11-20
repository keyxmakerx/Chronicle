// ==================== Banded agate ====================
// A die cut from one agate nodule and polished. The stone grew in rings round a hollow, so the bands
// are shells round a point inside the die, worked out from each point's place in the stone: every
// face cuts the same rings at its own angle, and they join across the edges. The hollow itself is
// lined with small crystals that glitter. The number is engraved and filled with metal.
import {
  hexA, mix, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// bands: the three colours the rings run through; line: the thin pale rings between them.
var GOLD = { color: '#efc257', hi: '#fff0b8', lo: '#9a6c14', rough: 0.24 }, SILVER = { color: '#e9edf4', hi: '#ffffff', lo: '#8d98ad', rough: 0.2 };
var AGATE = [
  { key: 'lace', name: 'Blue lace and silver', note: 'Pale blue and white rings, silver numbers', burn: '#a8d8ff', ink: '#e9edf4',
    bands: ['#7fa8dc', '#cfe2f6', '#4f78ba'], line: '#ffffff', metal: SILVER, base: { grad: ['#b9d2ee', '#4f78ba'], rough: 0.1 } },
  { key: 'fire', name: 'Fire agate and gold', note: 'Red, orange and cream rings, gold numbers', burn: '#ffb45c', ink: '#efc257',
    bands: ['#b8321a', '#f0a04a', '#6e1408'], line: '#ffe8c4', metal: GOLD, base: { grad: ['#e0703a', '#6e1408'], rough: 0.1 } },
  { key: 'moss', name: 'Moss agate and gold', note: 'Deep green and pale jade rings, gold numbers', burn: '#c8ff9a', ink: '#efc257',
    bands: ['#2f6a48', '#a8d4b0', '#123a26'], line: '#eef8e8', metal: GOLD, base: { grad: ['#5f9a74', '#123a26'], rough: 0.1 } },
  { key: 'botswana', name: 'Botswana pink and silver', note: 'Grey, pink and white rings, silver numbers', burn: '#ffc8d8', ink: '#f2f2f6',
    bands: ['#8a8088', '#e8b4b8', '#4a4450'], line: '#ffffff', metal: SILVER, base: { grad: ['#c8a8ae', '#4a4450'], rough: 0.1 } },
  { key: 'grape', name: 'Grape agate and gold', note: 'Violet and lilac rings, gold numbers', burn: '#e0b4ff', ink: '#efc257',
    bands: ['#5c2f9a', '#c8a8ee', '#2a1050'], line: '#f4eaff', metal: GOLD, base: { grad: ['#9068c8', '#2a1050'], rough: 0.1 } },
  { key: 'onyx', name: 'Sardonyx and gold', note: 'Black and white rings, gold numbers', burn: '#ffd97a', ink: '#efc257',
    bands: ['#17171b', '#e8e4dc', '#000000'], line: '#ffffff', metal: GOLD, base: { grad: ['#6a686c', '#0a0a0c'], rough: 0.1 } }
];

var AGATE_GLSL = [
  'float agH( vec3 p ) { return fract( sin( dot( p, vec3( 127.1, 311.7, 74.7 ) ) ) * 43758.5453 ); }',
  'float agN( vec3 p ) { vec3 i = floor( p ), f = fract( p ); f = f * f * ( 3.0 - 2.0 * f );',
  '  return mix( mix( mix( agH( i ), agH( i + vec3( 1.0, 0.0, 0.0 ) ), f.x ), mix( agH( i + vec3( 0.0, 1.0, 0.0 ) ), agH( i + vec3( 1.0, 1.0, 0.0 ) ), f.x ), f.y ),',
  '    mix( mix( agH( i + vec3( 0.0, 0.0, 1.0 ) ), agH( i + vec3( 1.0, 0.0, 1.0 ) ), f.x ), mix( agH( i + vec3( 0.0, 1.0, 1.0 ) ), agH( i + vec3( 1.0, 1.0, 1.0 ) ), f.x ), f.y ), f.z ); }'
].join('\n');

// The face image marks where the stone shows with its alpha: the stone is drawn part see-through and
// the number solid, and the shader paints the rings only where the image lets it.
function agateShader(s, c) {
  var m = this, col = function (h) { return m.emissive.clone().set(h); };
  s.uniforms.uAgA = { value: col(c.bands[0]) }; s.uniforms.uAgB = { value: col(c.bands[1]) }; s.uniforms.uAgC = { value: col(c.bands[2]) }; s.uniforms.uAgL = { value: col(c.line) };
  s.uniforms.uAgS = { value: m.userData.agScale || 1 }; s.uniforms.uAgO = { value: m.userData.agAt || m.emissive.clone().set(0x000000) };
  s.uniforms.uAgHd = { value: m.userData.agHd ? 1 : 0 };
  s.vertexShader = 'varying vec3 vAgP;\n' + s.vertexShader.replace('#include <begin_vertex>', '#include <begin_vertex>\n  vAgP = position;');
  s.fragmentShader = 'uniform vec3 uAgA, uAgB, uAgC, uAgL, uAgO; uniform float uAgS, uAgHd; varying vec3 vAgP;\n' + AGATE_GLSL + '\n' +
    s.fragmentShader.replace('#include <lights_physical_fragment>', [
      'float agShow = 1.0 - smoothstep( 0.72, 0.94, sampledDiffuseColor.a );',
      'vec3 agP = vAgP * uAgS;',
      // Distance from the hollow, squashed and bent, so the rings are wavy shells and never true spheres.
      'float agD = length( ( agP - uAgO ) * vec3( 1.0, 1.3, 0.82 ) ) + ( agN( agP * 1.5 + 2.0 ) - 0.5 ) * 0.5 + ( agN( agP * 4.1 + 9.0 ) - 0.5 ) * 0.12;',
      'float agT = agD * 5.2;',
      // Ring widths are uneven: the stone grew fast in some seasons and slow in others.
      'float agW = agN( vec3( agT * 1.7, 3.0, 7.0 ) ), agV = agN( vec3( agT * 4.6, 11.0, 1.0 ) );',
      'vec3 agCol = mix( uAgA, uAgB, smoothstep( 0.35, 0.65, agW ) );',
      'agCol = mix( agCol, uAgC, smoothstep( 0.55, 0.8, agV ) * 0.85 );',
      'float agLine = smoothstep( 0.86, 0.98, 0.5 + 0.5 * sin( agT * 9.0 + agW * 9.0 ) );',
      'agCol = mix( agCol, uAgL, agLine * 0.75 );',
      // The hollow at the heart: a crust of small crystals, each catching the light its own way.
      'float agCore = 1.0 - smoothstep( 0.2, 0.3, agD );',
      'float agSp = agH( floor( agP * 34.0 ) );',
      'agCol = mix( agCol, uAgL * ( 0.5 + 0.6 * agSp ), agCore );',
      'diffuseColor.rgb = mix( diffuseColor.rgb, agCol * diffuse, agShow );',
      'roughnessFactor = mix( roughnessFactor, mix( 0.06, 0.55, agSp ), agCore * agShow * uAgHd );',
      '#include <lights_physical_fragment>'].join('\n'));
}

registerSet({
  key: 'agate', name: 'Banded agate', note: 'Polished agate whose rings run round a crystal hollow inside the stone and join across every edge. Engraved and filled with metal.',
  show: 'molten', bev: 0.07, see: null, relief: 1.2,
  colours: AGATE,
  paint: function (g, lay, c) {
    var cv = lay.cv, w = strokeW(lay), m = c.metal;
    g.clearRect(0, 0, cv, cv);
    g.fillStyle = hexA(mix(c.bands[0], c.bands[1], 0.5), 0.6); g.fillRect(0, 0, cv, cv);
    // A dark bed under the number keeps it readable over the busiest rings.
    g.save(); try { g.filter = 'blur(' + (cv / 256 * 2.2) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = 'rgba(0,0,0,.55)'; paintNumeral(g, lay, lay.style, w * 2.3); g.restore();
    g.fillStyle = g.strokeStyle = 'rgba(0,0,0,.95)'; paintNumeral(g, lay, lay.style, w * 1.62);
    var gr = g.createLinearGradient(0, lay.cy - lay.h * 0.6, 0, lay.cy + lay.h * 0.6); gr.addColorStop(0, m.hi); gr.addColorStop(0.45, m.color); gr.addColorStop(1, m.lo);
    g.fillStyle = g.strokeStyle = gr; paintNumeral(g, lay, lay.style, w * 1.08);
    g.fillStyle = g.strokeStyle = hexA(m.hi, 0.75); g.save(); g.translate(0, -w * 0.2); paintNumeral(g, lay, lay.style, w * 0.24); g.restore();
  },
  // Polished stone, not metal; the number's fill is metal.
  surface: function (g, lay, c) {
    var cv = lay.cv, w = strokeW(lay);
    g.fillStyle = 'rgb(0,' + Math.round(c.base.rough * 255) + ',0)'; g.fillRect(0, 0, cv, cv);
    g.fillStyle = g.strokeStyle = 'rgb(0,' + Math.round(c.metal.rough * 255 + 26) + ',200)'; paintNumeral(g, lay, lay.style, w);
  },
  height: function (g, lay) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay);
    g.fillStyle = '#fff'; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (0.8 * k) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = '#5a5a5a'; paintNumeral(g, lay, lay.style, w * 1.4);
    g.fillStyle = g.strokeStyle = '#9a9a9a'; paintNumeral(g, lay, lay.style, w * 0.8);
    g.restore();
  },
  rim: function (c) { return { col: hexA(mix(c.bands[0], c.bands[1], 0.5), 0.6), rough: 0.1, metal: 0 }; },
  physical: function (c, q) { return { clearcoat: q.detail === 'basic' ? 0 : 1, clearcoatRoughness: 0.05, envMapIntensity: 1.2, userData: { agHd: q.detail !== 'basic' } }; },
  shader: agateShader,
  build: function (T, die) {
    // Each stone has its hollow somewhere of its own, off to one side of the middle.
    var a = Math.random() * 6.28, b = Math.random() * 2 - 1, r = Math.sqrt(1 - b * b) * 0.42, at = new T.Color(); at.r = Math.cos(a) * r; at.g = b * 0.42; at.b = Math.sin(a) * r;
    die.mats.forEach(function (m) { m.userData.agScale = 2.2 / die.size; m.userData.agAt = at; });
    return { parts: [], tick: null, dispose: function () {} };
  },
  cover: function (c) { return c.key === 'lace' ? 0.5 : 0.2; }
});
