// ==================== Damascus steel ====================
// Pattern-welded steel: two steels folded together, ground flat and etched, so the layers show as
// flowing light and dark bands. The bands are worked out from each point's place inside the die, not
// drawn face by face, so they run on over every edge the way layers in a real billet do. The etched
// layers sit a hair lower and duller; the bright ones stay polished. The number is cut deep and
// filled with a second metal, inside a fine inlaid line that follows the face.
import {
  hexA, mix, paintNumeral, polyPath, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// steel: [the etched layers, the bright layers]; flow: how tightly the bands fold (higher is busier);
// star marks star iron, which has straight crossed plates in place of folds.
var DAMASCUS = [
  { key: 'watered', name: 'Watered steel and gold', note: 'Grey steel in flowing layers, gold numbers', burn: '#ffcf6b', ink: '#efc257',
    steel: ['#4c515b', '#c9ced7'], flow: 1, metal: { color: '#efc257', hi: '#fff0b8', lo: '#9a6c14', rough: 0.24 }, base: { grad: ['#8b9099', '#3a3d44'], rough: 0.3 } },
  { key: 'blued', name: 'Blued steel and silver', note: 'Heat-blued layers, violet to peacock, silver numbers', burn: '#8fc8ff', ink: '#e6ebf3',
    steel: ['#232c78', '#5a9cf0'], heat: '#7a3fb0', flow: 1.15, metal: { color: '#e6ebf3', hi: '#ffffff', lo: '#8d98ad', rough: 0.2 }, base: { grad: ['#2c4a9a', '#10153a'], rough: 0.28 } },
  { key: 'mokume', name: 'Copper and silver mokume', note: 'Copper and silver worked like wood grain, silver numbers', burn: '#ffb07a', ink: '#f0f1f3',
    steel: ['#a4532a', '#e3ddd4'], flow: 1.5, metal: { color: '#f0f1f3', hi: '#ffffff', lo: '#9a9ea8', rough: 0.22 }, base: { grad: ['#c98a62', '#6a3418'], rough: 0.3 } },
  { key: 'black', name: 'Black damascus and copper', note: 'Deep-etched black layers, copper numbers', burn: '#ff9a5c', ink: '#e88f58',
    steel: ['#17181d', '#727884'], flow: 1.1, metal: { color: '#e88f58', hi: '#ffd2b0', lo: '#8a431c', rough: 0.26 }, base: { grad: ['#33363d', '#08090b'], rough: 0.34 } },
  { key: 'star', name: 'Star iron and gold', note: 'Meteorite iron, crossed plates of pale metal, gold numbers', burn: '#ffe08a', ink: '#f2c860',
    steel: ['#7f858e', '#eef1f4'], flow: 1, star: true, metal: { color: '#f2c860', hi: '#fff3c4', lo: '#a07418', rough: 0.24 }, base: { grad: ['#a9aeb6', '#555a62'], rough: 0.32 } },
  { key: 'rose', name: 'Rose bronze and ivory', note: 'Rose and pale bronze layers, ivory numbers', burn: '#ffc3a6', ink: '#fbf1dc',
    steel: ['#8a4a44', '#f0c2a8'], flow: 1.25, metal: { color: '#fbf1dc', hi: '#ffffff', lo: '#b89c78', rough: 0.26 }, base: { grad: ['#d49a86', '#6e3833'], rough: 0.3 } }
];

// ---------- The layers ----------
var DAMASCUS_GLSL = [
  'float dmH( vec3 p ) { return fract( sin( dot( p, vec3( 127.1, 311.7, 74.7 ) ) ) * 43758.5453 ); }',
  'float dmN( vec3 p ) { vec3 i = floor( p ), f = fract( p ); f = f * f * ( 3.0 - 2.0 * f );',
  '  return mix( mix( mix( dmH( i ), dmH( i + vec3( 1.0, 0.0, 0.0 ) ), f.x ), mix( dmH( i + vec3( 0.0, 1.0, 0.0 ) ), dmH( i + vec3( 1.0, 1.0, 0.0 ) ), f.x ), f.y ),',
  '    mix( mix( dmH( i + vec3( 0.0, 0.0, 1.0 ) ), dmH( i + vec3( 1.0, 0.0, 1.0 ) ), f.x ), mix( dmH( i + vec3( 0.0, 1.0, 1.0 ) ), dmH( i + vec3( 1.0, 1.0, 1.0 ) ), f.x ), f.y ), f.z ); }',
  // Folded layers: flat sheets bent by two sizes of noise, so they pool into eyes and run in ladders.
  'float dmFold( vec3 p, float flow ) {',
  '  float w = dmN( p * 1.1 ) * 2.2 + dmN( p * 2.7 + 5.0 ) * 0.8 * flow;',
  '  float a = ( p.y * 1.3 + p.x * 0.45 + p.z * 0.25 ) * 11.0 * flow + w * 7.0;',
  // Two rhythms laid over each other, so no two layers are the same width.
  '  return sin( a ) * 0.62 + sin( a * 2.3 + 1.3 + w ) * 0.38; }',
  // Star iron: three families of straight plates crossing at the angles the crystal sets.
  'float dmStar( vec3 p ) {',
  '  float a = sin( dot( p, vec3( 0.82, 0.57, 0.0 ) ) * 26.0 + dmN( p * 0.9 ) * 3.0 );',
  '  float b = sin( dot( p, vec3( -0.41, 0.57, 0.71 ) ) * 23.0 + dmN( p * 0.8 + 9.0 ) * 3.0 );',
  '  float c = sin( dot( p, vec3( -0.41, 0.57, -0.71 ) ) * 29.0 + dmN( p * 1.0 + 4.0 ) * 3.0 );',
  '  float pick = dmN( p * 0.75 + 2.0 );',
  '  return pick < 0.36 ? a : pick < 0.68 ? b : c; }'
].join('\n');

// The face image marks where the layers show with its alpha: the steel is drawn part see-through, the
// number and the inlaid line solid, and the shader paints the layers only where the image lets it.
function damascusShader(s, c) {
  var m = this;
  s.uniforms.uDmA = { value: m.emissive.clone().set(c.steel[0]) }; s.uniforms.uDmB = { value: m.emissive.clone().set(c.steel[1]) };
  s.uniforms.uDmHeat = { value: m.emissive.clone().set(c.heat || c.steel[1]) };
  s.uniforms.uDmFlow = { value: c.flow }; s.uniforms.uDmS = { value: m.userData.dmScale || 1 };
  s.uniforms.uDmHd = { value: m.userData.dmHd ? 1 : 0 }; s.uniforms.uDmStar = { value: c.star ? 1 : 0 };
  m.userData.dmS = s.uniforms.uDmS;
  s.vertexShader = 'varying vec3 vDmP;\n' + s.vertexShader.replace('#include <begin_vertex>', '#include <begin_vertex>\n  vDmP = position;');
  s.fragmentShader = 'uniform vec3 uDmA, uDmB, uDmHeat; uniform float uDmFlow, uDmS, uDmHd, uDmStar; varying vec3 vDmP;\n' + DAMASCUS_GLSL + '\n' +
    s.fragmentShader.replace('#include <lights_physical_fragment>', [
      'float dmShow = 1.0 - smoothstep( 0.72, 0.94, sampledDiffuseColor.a );',
      'vec3 dmP = vDmP * uDmS;',
      'float dmW = uDmStar > 0.5 ? dmStar( dmP ) : dmFold( dmP, uDmFlow );',
      // A second, finer set of layers inside each band, as a billet folded many times shows.
      'float dmFine = uDmHd > 0.5 ? sin( dmW * 9.0 + dmN( dmP * 6.0 ) * 2.0 ) : 0.0;',
      'float dmL = smoothstep( -0.22, 0.22, dmW + dmFine * 0.2 );',
      'vec3 dmCol = mix( uDmA, uDmB, dmL );',
      // Heat colour gathers where the bright layers thin out.
      'dmCol = mix( dmCol, uDmHeat, ( 1.0 - abs( dmW ) ) * 0.35 * step( 0.5, dmL ) );',
      'diffuseColor.rgb = mix( diffuseColor.rgb, dmCol * diffuse, dmShow );',
      'roughnessFactor = mix( roughnessFactor, mix( 0.62, 0.3, dmL ), dmShow );',
      // Etched layers are dull grey oxide, not mirror: less metal there keeps them from going black under a dark room.
      'metalnessFactor = mix( metalnessFactor, mix( 0.35, 0.85, dmL ), dmShow );',
      '#include <lights_physical_fragment>'].join('\n'));
}

// The inlaid line: the face's own outline, moved in toward its middle.
function damascusFillet(g, lay, k) {
  var cx = lay.cv / 2, cy = lay.cv / 2;
  g.beginPath();
  lay.inner.forEach(function (p, i) { var x = cx + (p[0] - cx) * k, y = cy + (p[1] - cy) * k; if (i) g.lineTo(x, y); else g.moveTo(x, y); });
  g.closePath();
}
function damascusMetal(g, lay, m) {
  var gr = g.createLinearGradient(0, lay.cy - lay.h * 0.6, 0, lay.cy + lay.h * 0.6);
  gr.addColorStop(0, m.hi); gr.addColorStop(0.45, m.color); gr.addColorStop(1, m.lo);
  return gr;
}

registerSet({
  key: 'damascus', name: 'Damascus steel', note: 'Folded steel etched to show its layers, which flow on over every edge. The number is cut deep and filled with a second metal.',
  show: 'molten', bev: 0.05, see: null, relief: 1.3,
  colours: DAMASCUS,
  paint: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), m = c.metal;
    g.clearRect(0, 0, cv, cv);
    g.fillStyle = hexA(mix(c.steel[0], c.steel[1], 0.5), 0.6); g.fillRect(0, 0, cv, cv);
    // The inlaid line, with the cut it sits in.
    g.lineJoin = 'miter';
    g.strokeStyle = 'rgba(0,0,0,.9)'; g.lineWidth = 3.4 * k; damascusFillet(g, lay, 0.88); g.stroke();
    g.strokeStyle = m.color; g.lineWidth = 1.5 * k; damascusFillet(g, lay, 0.88); g.stroke();
    // The number: a dark cut wider than the metal, then the metal, then a bright lip on its upper edge.
    g.fillStyle = g.strokeStyle = 'rgba(0,0,0,.95)'; paintNumeral(g, lay, lay.style, w * 1.42);
    g.fillStyle = g.strokeStyle = damascusMetal(g, lay, m); paintNumeral(g, lay, lay.style, w * 0.98);
    g.fillStyle = g.strokeStyle = hexA(m.hi, 0.75); g.save(); g.translate(0, -w * 0.2); paintNumeral(g, lay, lay.style, w * 0.24); g.restore();
  },
  // All metal. The number's fill is kept a little short of a mirror, so the room's dark floor can't empty it.
  surface: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay);
    g.fillStyle = 'rgb(0,' + Math.round(c.base.rough * 255) + ',255)'; g.fillRect(0, 0, cv, cv);
    g.strokeStyle = 'rgb(0,60,255)'; g.lineWidth = 1.5 * k; damascusFillet(g, lay, 0.88); g.stroke();
    g.fillStyle = g.strokeStyle = 'rgb(0,' + Math.round(c.metal.rough * 255 + 26) + ',200)'; paintNumeral(g, lay, lay.style, w);
  },
  height: function (g, lay) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay);
    g.fillStyle = '#fff'; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (0.8 * k) + 'px)'; } catch (e) {}
    g.strokeStyle = '#8c8c8c'; g.lineWidth = 3 * k; damascusFillet(g, lay, 0.88); g.stroke();
    g.fillStyle = g.strokeStyle = '#5a5a5a'; paintNumeral(g, lay, lay.style, w * 1.4);
    g.fillStyle = g.strokeStyle = '#9a9a9a'; paintNumeral(g, lay, lay.style, w * 0.8);
    g.restore();
  },
  // The rounded edges show the layers too: see-through like the steel, so the shader works there.
  rim: function (c) { return { col: hexA(mix(c.steel[0], c.steel[1], 0.5), 0.6), rough: 0.3, metal: 1 }; },
  physical: function (c, q) { return { envMapIntensity: 1.5, userData: { dmHd: q.detail !== 'basic' } }; },
  shader: damascusShader,
  build: function (T, die) {
    // The layers are laid out for a die of one size, so a small die shows the same billet, smaller.
    die.mats.forEach(function (m) { m.userData.dmScale = 2.2 / die.size; });
    return { parts: [], tick: null, dispose: function () {} };
  }
});
