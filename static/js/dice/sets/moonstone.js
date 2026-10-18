// ---------- Moonstone ----------
// A translucent milky stone: a pale, cloudy heart and greyer, glassier edges and corners where the light
// passes through. A blue sheen floats inside it. The sheen and the clouds are looked up in noise at points
// pushed back along the line of sight (the sheen shallow, the clouds deeper), so they slide against the
// surface as the die turns and drift slowly when it is still. The sheen tints the stone and adds a little
// light; it never goes white. The number is engraved: a dark cut line, a metal fill, a lit lip.
import {
  hexA, isLight, mix, mulberry, paintNumeral, polyPath, registerSet, strokeW
} from '../engine.js';

// ---------- Noise ----------
var MOON_GLSL = [
  'float mnH( vec3 p ) { return fract( sin( dot( p, vec3( 127.1, 311.7, 74.7 ) ) ) * 43758.5453 ); }',
  'float mnN( vec3 p ) { vec3 i = floor( p ), f = fract( p ); f = f * f * ( 3.0 - 2.0 * f );',
  '  float a = mnH( i ), b = mnH( i + vec3( 1.0, 0.0, 0.0 ) ), c = mnH( i + vec3( 0.0, 1.0, 0.0 ) ), d = mnH( i + vec3( 1.0, 1.0, 0.0 ) );',
  '  float e = mnH( i + vec3( 0.0, 0.0, 1.0 ) ), f1 = mnH( i + vec3( 1.0, 0.0, 1.0 ) ), g = mnH( i + vec3( 0.0, 1.0, 1.0 ) ), h = mnH( i + vec3( 1.0, 1.0, 1.0 ) );',
  '  return mix( mix( mix( a, b, f.x ), mix( c, d, f.x ), f.y ), mix( mix( e, f1, f.x ), mix( g, h, f.x ), f.y ), f.z ); }'
].join('\n');

// ---------- Colours ----------
// base.grad: on a pale stone [the milky heart, the greyer glassy edge]; on a dark stone [the paler edge, the
// deeper heart]. sheen: its colour, how much it tints the stone, how
// much light it adds (a black stone needs light; a pale one would only clip), and whether it is a rainbow.
var MOON_COLOURS = [
  { key: 'blue', name: 'Blue moonstone', note: 'Milky white with a blue sheen, silver numbers', burn: '#9cc4ff', ink: '#cdd4e0',
    base: { grad: ['#d9e0eb', '#8e9db6'], rough: 0.36 }, metal: { col: '#dfe5ef', hi: '#ffffff', lo: '#8f9bb2' }, sheen: { col: '#5a8eff', tint: 0.8, glow: 0.1 }, veil: '#9ab4dc', cut: 'rgba(22,30,54,.94)' },
  { key: 'peach', name: 'Peach moonstone', note: 'Warm cream with a peach sheen, gold numbers', burn: '#ffbe8a', ink: '#d9a547',
    base: { grad: ['#ecd8c6', '#b4937f'], rough: 0.36 }, metal: { col: '#dcae4e', hi: '#fbe39a', lo: '#9a6e18' }, sheen: { col: '#ff8458', tint: 0.85, glow: 0.1 }, veil: '#e8a88a', cut: 'rgba(52,26,14,.94)' },
  { key: 'rainbow', name: 'Rainbow moonstone', note: 'White stone, a sheen of every colour, silver numbers', burn: '#d9c6ff', ink: '#d3d8e4',
    base: { grad: ['#dfdfe8', '#9895ab'], rough: 0.34 }, metal: { col: '#dfe4ee', hi: '#ffffff', lo: '#8f9ab0' }, sheen: { col: '#8fb4ff', tint: 0.85, glow: 0.1, rainbow: true }, veil: '#b4b0d0', cut: 'rgba(28,26,50,.94)' },
  { key: 'grey', name: 'Grey moonstone', note: 'Smoky grey with a pale blue sheen, bright silver numbers', burn: '#c6d4ee', ink: '#eef0f4',
    base: { grad: ['#b9bdc6', '#5c616c'], rough: 0.36 }, metal: { col: '#f2f4f8', hi: '#ffffff', lo: '#b0b8c9' }, sheen: { col: '#7ea2e6', tint: 0.75, glow: 0.12 }, veil: '#6c7488', cut: 'rgba(14,18,30,.9)' },
  { key: 'black', name: 'Black moonstone', note: 'Dark stone, a bright blue sheen, silver numbers', burn: '#8db3ff', ink: '#e3e8f2',
    base: { grad: ['#2c2e37', '#090a0f'], rough: 0.32 }, metal: { col: '#e6ebf5', hi: '#ffffff', lo: '#9ba5ba' }, sheen: { col: '#6f9cff', tint: 0.0, glow: 0.3 }, veil: '#566078', cut: 'rgba(0,0,0,.92)' }
];

// ---------- Face drawing ----------
// Veils (soft clouded bands), feathers (the fine branching lines moonstone is known for) and a few specks.
function moonBody(g, lay, c, seed) {
  var cv = lay.cv, k = cv / 256, rnd = mulberry(seed), dark = !isLight(c);
  // A pale stone is milky at the heart and greyer and glassier toward the edges and corners; a dark one
  // is deeper at the heart and paler at the edge.
  var gr = g.createRadialGradient(cv / 2, cv / 2, cv * 0.04, cv / 2, cv / 2, cv * 0.52);
  if (dark) { gr.addColorStop(0, mix(c.base.grad[1], c.base.grad[0], 0.3)); gr.addColorStop(0.7, mix(c.base.grad[1], c.base.grad[0], 0.85)); gr.addColorStop(1, c.base.grad[0]); }
  else { gr.addColorStop(0, c.base.grad[0]); gr.addColorStop(0.4, c.base.grad[0]); gr.addColorStop(1, c.base.grad[1]); }
  g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
  g.save(); try { g.filter = 'blur(' + (9 * k) + 'px)'; } catch (e) {}
  for (var i = 0; i < 6; i++) {
    g.strokeStyle = hexA(i % 2 ? c.veil : '#ffffff', dark ? 0.2 : (i % 2 ? 0.26 : 0.4)); g.lineWidth = (14 + rnd() * 30) * k; g.lineCap = 'round';
    var x = rnd() * cv, y = rnd() * cv; g.beginPath(); g.moveTo(x, y); g.bezierCurveTo(rnd() * cv, rnd() * cv, rnd() * cv, rnd() * cv, rnd() * cv, rnd() * cv); g.stroke();
  }
  g.restore();
  // Silky banding: very fine long lines, the layering that makes the sheen.
  g.lineWidth = 0.7 * k;
  for (var s = 0; s < (dark ? 0 : 46); s++) {
    g.strokeStyle = rnd() > 0.5 ? 'rgba(255,255,255,' + (dark ? 0.05 : 0.1) + ')' : hexA(c.veil, 0.1);
    var yy = rnd() * cv; g.beginPath(); g.moveTo(0, yy); g.bezierCurveTo(cv * 0.3, yy + (rnd() - 0.5) * 24 * k, cv * 0.7, yy + (rnd() - 0.5) * 24 * k, cv, yy + (rnd() - 0.5) * 12 * k); g.stroke();
  }
  // Feathers: a spine with short barbs on both sides, faint.
  g.save(); polyPath(g, lay.inner); g.clip(); g.lineCap = 'round';
  for (var f = 0; f < 3; f++) {
    var px = rnd() * cv, py = rnd() * cv, ang = rnd() * 6.28, len = 6 + Math.floor(rnd() * 5);
    for (var j = 0; j < len; j++) {
      var nx = px + Math.cos(ang) * 11 * k, ny = py + Math.sin(ang) * 11 * k, bl = (5 + rnd() * 7) * k;
      g.strokeStyle = hexA(c.veil, 0.42); g.lineWidth = 0.9 * k;
      g.beginPath(); g.moveTo(px, py); g.lineTo(nx, ny); g.stroke();
      g.strokeStyle = hexA(dark ? '#ffffff' : c.veil, dark ? 0.16 : 0.32); g.lineWidth = 0.7 * k;
      g.beginPath(); g.moveTo(px, py); g.lineTo(px + Math.cos(ang + 0.9) * bl, py + Math.sin(ang + 0.9) * bl); g.moveTo(px, py); g.lineTo(px + Math.cos(ang - 0.9) * bl, py + Math.sin(ang - 0.9) * bl); g.stroke();
      px = nx; py = ny; ang += (rnd() - 0.5) * 0.5;
    }
  }
  for (var d = 0; d < 14; d++) { g.fillStyle = hexA(dark ? '#ffffff' : '#6a5a4a', dark ? 0.1 : 0.12 + rnd() * 0.1); var ds = (0.5 + rnd() * 1.2) * k; g.fillRect(rnd() * cv, rnd() * cv, ds, ds); }
  g.restore();
  // The face sits down into its setting.
  g.save(); polyPath(g, lay.inner); g.clip();
  g.strokeStyle = hexA(c.base.grad[1], 0.32); g.lineWidth = cv * 0.06; try { g.filter = 'blur(' + (cv * 0.02) + 'px)'; } catch (e) {}
  polyPath(g, lay.inner); g.stroke(); g.restore();
}

// ---------- Material ----------
function moonShader(s, c) {
  var sh = c.sheen, m = this;
  s.uniforms.uMoonT = { value: 0 }; s.uniforms.uMoonCol = { value: m.emissive.clone().set(sh.col) };
  var pale = isLight(c);
  s.uniforms.uMoonEdge = { value: m.emissive.clone().set(pale ? mix(c.base.grad[1], '#000000', 0.12) : mix(c.base.grad[0], '#ffffff', 0.35)) };
  s.uniforms.uMoonTint = { value: sh.tint }; s.uniforms.uMoonGlow = { value: sh.glow }; s.uniforms.uMoonRainbow = { value: sh.rainbow ? 1 : 0 };
  s.uniforms.uMoonHd = { value: m.userData.moonHd ? 1 : 0 }; s.uniforms.uMoonMilk = { value: pale ? 0.6 : 0.1 }; s.uniforms.uMoonSharp = { value: pale ? 1 : 1.7 }; s.uniforms.uMoonDepth = { value: pale ? 1 : 0.5 }; s.uniforms.uMoonS = { value: m.userData.moonScale || 1 };
  m.userData.moonT = s.uniforms.uMoonT;
  s.vertexShader = 'varying vec3 vMoonP; varying vec3 vMoonV;\n' + s.vertexShader.replace('#include <begin_vertex>',
    '#include <begin_vertex>\n  vMoonP = position;\n  vMoonV = normalize( ( cameraPosition - ( modelMatrix * vec4( position, 1.0 ) ).xyz ) * mat3( modelMatrix ) );');
  s.fragmentShader = 'uniform float uMoonT, uMoonTint, uMoonGlow, uMoonRainbow, uMoonHd, uMoonMilk, uMoonSharp, uMoonDepth, uMoonS; uniform vec3 uMoonCol, uMoonEdge; varying vec3 vMoonP; varying vec3 vMoonV;\n' + MOON_GLSL + '\n' +
    s.fragmentShader.replace('#include <lights_physical_fragment>', [
      'vec3 mnV = normalize( vMoonV ), mP = vMoonP * uMoonS;',
      'float mnv = clamp( dot( normal, normalize( vViewPosition ) ), 0.0, 1.0 );',
      // The sheen layer lies below the surface: the look-up point slides back along the line of sight.
      'vec3 mp = mP * 0.9 - mnV * 0.6;',
      'float mn1 = mnN( mp * 1.5 + vec3( 0.0, uMoonT * 0.04, uMoonT * 0.026 ) );',
      'float mn2 = uMoonHd > 0.5 ? mnN( mp * 3.4 + 4.1 ) : 0.5;',
      'float mph = dot( mp, vec3( 1.3, 0.9, 1.1 ) ) * 1.7 + mn1 * 3.2 + mn2 * 0.9;',
      'float msh = smoothstep( 0.25, 0.9, 0.5 + 0.5 * sin( mph + uMoonT * 0.1 ) ) * smoothstep( 0.18, 0.65, mnN( mp * 0.7 + 9.0 ) ) * ( 0.95 + 0.35 * ( 1.0 - mnv ) );',
      // A second sheen layer lies deeper: against the first it shifts with the view, which is what makes the glow float.
      'if ( uMoonHd > 0.5 ) {',
      '  vec3 mp2 = mP * 0.9 - mnV * 1.3;',
      '  float m2 = smoothstep( 0.35, 0.9, 0.5 + 0.5 * sin( dot( mp2, vec3( 0.8, 1.3, 0.6 ) ) * 2.1 + mnN( mp2 * 1.2 + 17.0 ) * 4.0 - uMoonT * 0.07 ) ) * smoothstep( 0.25, 0.7, mnN( mp2 * 0.8 + 31.0 ) );',
      '  msh = max( msh, m2 * 0.7 );',
      '}',
      'msh = pow( msh, uMoonSharp );',
      'vec3 mcol = uMoonRainbow > 0.5 ? mix( vec3( 0.8 ), 0.5 + 0.5 * cos( 6.2832 * ( mph * 0.07 + mn1 * 0.6 + vec3( 0.0, 0.33, 0.67 ) ) ), 0.85 ) : uMoonCol;',
      // Clouds and fibres inside the stone, looked up deep along the line of sight so they sit under the surface.
      'vec3 mw = mP * 0.9 - mnV * 0.95;',
      'float mcl = mnN( mw * 2.0 + 1.3 ), mfb = uMoonHd > 0.5 ? mnN( vec3( mw.x * 1.1, mw.y * 4.8, mw.z * 1.1 ) + 3.7 ) : 0.5;',
      'diffuseColor.rgb *= 1.0 - uMoonDepth * ( 0.22 * ( 1.0 - mcl ) + 0.14 * ( 1.0 - mfb ) );',
      'diffuseColor.rgb = mix( diffuseColor.rgb, diffuseColor.rgb * mcol * 1.1, msh * uMoonTint );',
      // Thin stone at the edges passes the light through: pale stones go greyer and glassier there, dark ones lighter.
      'diffuseColor.rgb = mix( diffuseColor.rgb, uMoonEdge, pow( 1.0 - mnv, 1.8 ) * uMoonMilk );',
      'totalEmissiveRadiance += diffuseColor.rgb * 0.04 * diffuse.g + mcol * msh * uMoonGlow * diffuse.g;',
      '#include <lights_physical_fragment>'].join('\n'));
}

// The engraving: a dark cut with the metal filled into it, shaded top to bottom, with a bright lip
// where the cut's far wall catches the light.
function moonNumber(g, lay, c, w) {
  var h = lay.h, y0 = lay.cy - h * 0.55, y1 = lay.cy + h * 0.55, m = c.metal;
  g.fillStyle = g.strokeStyle = 'rgba(0,0,0,.25)'; g.save(); g.translate(0, w * 0.25); paintNumeral(g, lay, lay.style, w * 2.0); g.restore();
  g.fillStyle = g.strokeStyle = c.cut; paintNumeral(g, lay, lay.style, w * 1.95);
  var gr = g.createLinearGradient(0, y0, 0, y1); gr.addColorStop(0, m.hi); gr.addColorStop(0.5, m.col); gr.addColorStop(1, m.lo);
  g.fillStyle = g.strokeStyle = gr; paintNumeral(g, lay, lay.style, w * 0.86);
  g.fillStyle = g.strokeStyle = hexA(m.hi, 0.6); g.save(); g.translate(0, -w * 0.2); paintNumeral(g, lay, lay.style, w * 0.28); g.restore();
}

registerSet({
  key: 'moonstone', name: 'Moonstone', note: 'Milky white stone with a blue sheen that drifts as the die turns. Engraved and filled in silver.', show: 'inscribe', bev: 0.065,
  colours: MOON_COLOURS,
  see: null, relief: 1,
  paint: function (g, lay, c, seed) {
    moonBody(g, lay, c, seed);
    moonNumber(g, lay, c, strokeW(lay));
  },
  surface: function (g, lay, c) {
    var w = strokeW(lay);
    g.fillStyle = g.strokeStyle = 'rgb(0,96,130)'; paintNumeral(g, lay, lay.style, w * 1.0);
  },
  height: function (g, lay, c) {
    var w = strokeW(lay);
    g.fillStyle = '#fff'; g.fillRect(0, 0, lay.cv, lay.cv);
    try { g.filter = 'blur(' + (1.0 * lay.cv / 256) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = '#7a7a7a'; paintNumeral(g, lay, lay.style, w * 1.85);
    g.fillStyle = g.strokeStyle = '#404040'; paintNumeral(g, lay, lay.style, w * 0.95);
  },
  rim: function (c) { return { col: isLight(c) ? mix(c.base.grad[1], c.base.grad[0], 0.5) : mix(c.base.grad[1], '#7a859c', 0.4), rough: 0.32, metal: 0.05 }; },
  physical: function (c, q) {
    var hd = q.detail !== 'basic';
    return { envMapIntensity: 0.9, specularIntensity: 0.8, userData: { moonHd: hd } };
  },
  shader: moonShader,
  build: function (T, die, q) {
    var t = 0;
    // The sheen and clouds are laid out for a die of one size, so a small die shows the same pattern small.
    die.mat.userData.moonScale = 2.2 / die.size;
    return { parts: [], tick: function (dt) { t += dt; var u = die.mat.userData.moonT; if (u) u.value = t; }, dispose: function () {} };
  }
});
