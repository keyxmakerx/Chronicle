// ==================== Kintsugi ====================
// Glazed pottery, broken and mended with gold. The breaks are worked out from each point's place
// inside the die, so a seam that leaves one face carries on across the edge onto the next, the way a
// real break runs through a pot. The glaze is smooth and deep; the gold stands in it bright. Some
// glazes also craze: a fine net of hairline cracks under the surface.
import {
  hexA, isLight, mix, mulberry, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// glaze: [where it pools, where it thins]; seam: the mending metal [light, body, shade]; num: the
// number's colour; craze: how strongly the hairline net shows (0 for none).
var KINTSUGI = [
  { key: 'porcelain', name: 'White porcelain and gold', note: 'White glaze, gold seams, indigo numbers', burn: '#ff8a1a', ink: '#1f2f7a',
    glaze: ['#f4f1e8', '#ffffff'], seam: ['#fff0b0', '#e9bb4a', '#946a12'], num: '#1f2f7a', craze: 0.0, base: { grad: ['#fbf9f2', '#d8d3c4'], rough: 0.12 } },
  { key: 'celadon', name: 'Celadon and gold', note: 'Pale green crackled glaze, gold seams, gold numbers', burn: '#ffe08a', ink: '#e9bb4a',
    glaze: ['#8fbfa6', '#cfe6d6'], seam: ['#fff0b0', '#e9bb4a', '#946a12'], num: '#7a5408', craze: 0.55, base: { grad: ['#b9d9c6', '#6f9f88'], rough: 0.12 } },
  { key: 'raku', name: 'Black raku and gold', note: 'Black glaze, gold seams, gold numbers', burn: '#ffcf6b', ink: '#f0c558',
    glaze: ['#0f1012', '#2b2d33'], seam: ['#fff0b0', '#f0c558', '#946a12'], num: '#f0c558', craze: 0.0, base: { grad: ['#33363d', '#08090b'], rough: 0.16 } },
  { key: 'cobalt', name: 'Cobalt and gold', note: 'Deep blue glaze, gold seams, gold numbers', burn: '#ffd98a', ink: '#f0c558',
    glaze: ['#122a8c', '#2f55c8'], seam: ['#fff0b0', '#f0c558', '#946a12'], num: '#f6d57a', craze: 0.0, base: { grad: ['#2f55c8', '#0b1a5c'], rough: 0.12 } },
  { key: 'oxblood', name: 'Oxblood and silver', note: 'Dark red glaze, silver seams, silver numbers', burn: '#ffd0c0', ink: '#e8ecf2',
    glaze: ['#5c0d10', '#a3201f'], seam: ['#ffffff', '#dfe4ec', '#7e8798'], num: '#eef1f6', craze: 0.25, base: { grad: ['#a3201f', '#3d0709'], rough: 0.12 } },
  { key: 'robin', name: 'Robin egg and copper', note: 'Speckled blue-green glaze, copper seams, copper numbers', burn: '#ffb07a', ink: '#c56a34',
    glaze: ['#63b8b4', '#a7dcd6'], seam: ['#ffd9bd', '#d9783e', '#7a3814'], num: '#7a3210', craze: 0.3, speck: '#3a2a20', base: { grad: ['#a7dcd6', '#4a9a96'], rough: 0.12 } }
];

// ---------- The breaks ----------
var KINTSUGI_GLSL = [
  'vec3 ktH( vec3 p ) { p = vec3( dot( p, vec3( 127.1, 311.7, 74.7 ) ), dot( p, vec3( 269.5, 183.3, 246.1 ) ), dot( p, vec3( 113.5, 271.9, 124.6 ) ) ); return fract( sin( p ) * 43758.5453 ); }',
  'float ktN( vec3 p ) { vec3 i = floor( p ), f = fract( p ); f = f * f * ( 3.0 - 2.0 * f );',
  '  return mix( mix( mix( ktH( i ).x, ktH( i + vec3( 1.0, 0.0, 0.0 ) ).x, f.x ), mix( ktH( i + vec3( 0.0, 1.0, 0.0 ) ).x, ktH( i + vec3( 1.0, 1.0, 0.0 ) ).x, f.x ), f.y ),',
  '    mix( mix( ktH( i + vec3( 0.0, 0.0, 1.0 ) ).x, ktH( i + vec3( 1.0, 0.0, 1.0 ) ).x, f.x ), mix( ktH( i + vec3( 0.0, 1.0, 1.0 ) ).x, ktH( i + vec3( 1.0, 1.0, 1.0 ) ).x, f.x ), f.y ), f.z ); }',
  // How far a point is from the nearest wall between two shards: the pot broke into the cells of this net.
  'float ktCell( vec3 p ) { vec3 i = floor( p ), f = fract( p ); float a = 8.0, b = 8.0;',
  '  for ( int x = -1; x <= 1; x++ ) for ( int y = -1; y <= 1; y++ ) for ( int z = -1; z <= 1; z++ ) {',
  '    vec3 o = vec3( float( x ), float( y ), float( z ) ); vec3 d = o + ktH( i + o ) - f; float q = dot( d, d );',
  '    if ( q < a ) { b = a; a = q; } else if ( q < b ) { b = q; } }',
  '  return sqrt( b ) - sqrt( a ); }'
].join('\n');

// The face image marks where a break may show with its alpha: the glaze is drawn part see-through,
// the number solid, so a seam never runs over a number.
function kintsugiShader(s, c) {
  var m = this, col = function (h) { return m.emissive.clone().set(h); };
  s.uniforms.uKtHi = { value: col(c.seam[0]) }; s.uniforms.uKtMid = { value: col(c.seam[1]) }; s.uniforms.uKtLo = { value: col(c.seam[2]) };
  s.uniforms.uKtS = { value: m.userData.ktScale || 1 }; s.uniforms.uKtOff = { value: m.userData.ktOff || 0 };
  s.uniforms.uKtCraze = { value: c.craze || 0 }; s.uniforms.uKtHd = { value: m.userData.ktHd ? 1 : 0 };
  s.vertexShader = 'varying vec3 vKtP;\n' + s.vertexShader.replace('#include <begin_vertex>', '#include <begin_vertex>\n  vKtP = position;');
  s.fragmentShader = 'uniform vec3 uKtHi, uKtMid, uKtLo; uniform float uKtS, uKtOff, uKtCraze, uKtHd; varying vec3 vKtP;\n' + KINTSUGI_GLSL + '\n' +
    s.fragmentShader.replace('#include <lights_physical_fragment>', [
      'float ktShow = 1.0 - smoothstep( 0.72, 0.94, sampledDiffuseColor.a );',
      'vec3 ktP = vKtP * uKtS + uKtOff;',
      // A break is never straight: the net is bent a little before it is read.
      'vec3 ktQ = ktP * 0.74 + ( vec3( ktN( ktP * 2.3 ), ktN( ktP * 2.3 + 7.0 ), ktN( ktP * 2.3 + 13.0 ) ) - 0.5 ) * 0.3;',
      'float ktD = ktCell( ktQ );',
      // The seam swells and thins along its run, as lacquer laid by hand does.
      'float ktW = 0.026 + 0.022 * ktN( ktP * 3.1 + 3.0 );',
      'float ktSeam = ( 1.0 - smoothstep( ktW * 0.72, ktW, ktD ) ) * ktShow;',
      'float ktLip = smoothstep( ktW * 0.1, ktW * 0.8, ktD );',
      'vec3 ktGold = mix( uKtHi, mix( uKtMid, uKtLo, ktLip * ktLip ), 0.35 + 0.65 * ktLip );',
      // Crazing: a much finer net, shown only as a faint darker line under the glaze.
      'float ktFine = uKtCraze > 0.0 && uKtHd > 0.5 ? 1.0 - smoothstep( 0.0, 0.05, ktCell( ktP * 6.5 + 4.0 ) ) : 0.0;',
      'diffuseColor.rgb *= 1.0 - ktFine * uKtCraze * 0.45 * ktShow * ( 1.0 - ktSeam );',
      'diffuseColor.rgb = mix( diffuseColor.rgb, ktGold * diffuse, ktSeam );',
      'roughnessFactor = mix( roughnessFactor, 0.26, ktSeam );',
      'metalnessFactor = mix( metalnessFactor, 0.92, ktSeam );',
      '#include <lights_physical_fragment>'].join('\n'));
}

registerSet({
  key: 'kintsugi', name: 'Kintsugi', note: 'Glazed pottery mended with gold. The seams run on across every edge, and no two dice break the same way.',
  show: 'inscribe', bev: 0.09, see: null, relief: 1.1,
  colours: KINTSUGI,
  paint: function (g, lay, c, seed) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), rnd = mulberry(seed);
    g.clearRect(0, 0, cv, cv);
    var gr = g.createRadialGradient(lay.cx, lay.cy, lay.r * 0.1, lay.cx, lay.cy, lay.r * 1.5);
    gr.addColorStop(0, hexA(c.glaze[1], 0.6)); gr.addColorStop(1, hexA(c.glaze[0], 0.6));
    g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
    // Glaze pools: soft patches a shade deeper or paler.
    g.save(); try { g.filter = 'blur(' + (8 * k) + 'px)'; } catch (e) {}
    for (var i = 0; i < 7; i++) { g.fillStyle = hexA(rnd() > 0.5 ? c.glaze[0] : c.glaze[1], 0.3); g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (16 + rnd() * 26) * k, 0, 7); g.fill(); }
    g.restore();
    if (c.speck) for (var j = 0; j < 46; j++) { g.fillStyle = hexA(c.speck, 0.2 + rnd() * 0.35); g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (0.5 + rnd() * rnd() * 2.2) * k, 0, 7); g.fill(); }
    // The number, brushed on under the glaze: a soft halo where the colour bled, then the stroke.
    g.save(); try { g.filter = 'blur(' + (1.6 * k) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = hexA(c.num, 0.4); paintNumeral(g, lay, lay.style, w * 1.5); g.restore();
    g.fillStyle = g.strokeStyle = c.num; paintNumeral(g, lay, lay.style, w * 1.06);
    g.fillStyle = g.strokeStyle = hexA(mix(c.num, '#ffffff', 0.5), 0.5); g.save(); g.translate(-w * 0.1, -w * 0.14); paintNumeral(g, lay, lay.style, w * 0.2); g.restore();
  },
  // Glaze everywhere: glass-smooth and not metal. The shader makes the seams metal on its own.
  surface: function (g, lay, c) {
    g.fillStyle = 'rgb(0,' + Math.round(c.base.rough * 255) + ',0)'; g.fillRect(0, 0, lay.cv, lay.cv);
  },
  // The glaze runs thick, so the number only just shows as a swell under it.
  height: function (g, lay) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay);
    g.fillStyle = '#808080'; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (2.2 * k) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = '#a8a8a8'; paintNumeral(g, lay, lay.style, w * 1.1); g.restore();
  },
  rim: function (c) { return { col: hexA(mix(c.glaze[0], c.glaze[1], 0.4), 0.6), rough: c.base.rough, metal: 0 }; },
  physical: function (c, q) { return { clearcoat: q.detail === 'basic' ? 0 : 0.7, clearcoatRoughness: 0.06, envMapIntensity: 1.2, userData: { ktHd: q.detail !== 'basic' } }; },
  shader: kintsugiShader,
  build: function (T, die) {
    // Each die breaks its own way; the net is laid out for a die of one size, so a small die has as many shards as a big one.
    var off = Math.random() * 40;
    die.mats.forEach(function (m) { m.userData.ktScale = 2.2 / die.size; m.userData.ktOff = off; });
    return { parts: [], tick: null, dispose: function () {} };
  },
  cover: function (c) { return isLight(c) ? 0.65 : 0; }
});
