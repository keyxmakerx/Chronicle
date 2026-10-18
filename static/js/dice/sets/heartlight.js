// ==================== Heartlight dice ====================
// Clear crystal with a knot of magic at its centre. The die keeps its normal shape; the knot is a few
// thin strands of streaming light woven round each other, with sparks orbiting and a soft glow, built
// once per die and driven on the graphics card from a handful of numbers. It spins up and stretches
// while the die tumbles and settles to a slow drift at rest. The numbers are frosted into the glass and
// lit faintly from the core.
import {
  canvas, hexA, mix, mulberry, paintNumeral, polyPath, registerSet, strokePaths, strokeW, share
} from '../engine.js';

// `tint` is the crystal's pale colour (it also colours the shadow the glass casts); `core` is the dye of the
// knot (dark enough to read on a white page),
// `hot` is the colour its light runs to, and `burn` is what a show lights the number with.
var heartColours = [
  { key: 'violet', name: 'Arcane violet', note: 'Lilac crystal, a violet knot', tint: '#cfbfff', alpha: 0.11, ink: '#f3edff', burn: '#b996ff', core: '#6a2fe8', hot: '#eadcff',
    base: { grad: ['#f1ecff', '#cfbfff'], rough: 0.05, coat: 1 } },
  { key: 'fey', name: 'Fey green', note: 'Mint crystal, a green knot', tint: '#a6e8c2', alpha: 0.11, ink: '#effff4', burn: '#7dffb0', core: '#0c9a50', hot: '#d8ffe4',
    base: { grad: ['#ebfbf2', '#a6e8c2'], rough: 0.05, coat: 1 } },
  { key: 'sun', name: 'Sun gold', note: 'Honey crystal, a golden knot', tint: '#ffdb88', alpha: 0.11, ink: '#fff8e3', burn: '#ffd05a', core: '#dd7a00', hot: '#fff1bd',
    base: { grad: ['#fff8e6', '#ffdb88'], rough: 0.05, coat: 1 } },
  { key: 'abyss', name: 'Abyss blue', note: 'Pale blue crystal, a deep blue knot', tint: '#a6cbff', alpha: 0.12, ink: '#ecf5ff', burn: '#66bdff', core: '#1050f0', hot: '#c3e6ff',
    base: { grad: ['#eaf3ff', '#a6cbff'], rough: 0.05, coat: 1 } },
  { key: 'blood', name: 'Blood red', note: 'Rose crystal, a crimson knot', tint: '#ffa8b2', alpha: 0.11, ink: '#fff0f0', burn: '#ff5a5a', core: '#cc0f28', hot: '#ffd2c4',
    base: { grad: ['#ffeced', '#ffa8b2'], rough: 0.05, coat: 1 } }
];

// Each die gets its own phase, so a table of them does not pulse as one.
var heartCount = 0;

// The number: strokes with round ends for the halo and keyline (paintNumeral's butt ends would leave the
// ends of every stroke without an outline), or paintNumeral itself for the classic font.
function heartStrokes(g, lay, w, round) {
  if (lay.style === 'classic') { paintNumeral(g, lay, lay.style, w); return; }
  g.lineWidth = w; g.lineJoin = round ? 'round' : 'miter'; g.miterLimit = 3; g.lineCap = round ? 'round' : 'butt';
  strokePaths(g, lay.strokes);
  if (lay.label === '6' || lay.label === '9') { g.beginPath(); g.arc(lay.cx, lay.cy + lay.h * 0.68, w * (round ? 0.5 : 0.62), 0, 7); g.fill(); }
}

// The face image: almost clear (alpha low) with a faint frosted lip round the rim, and the number frosted
// in: a glow of the core's colour, a dark keyline so it reads on a white page, then the pale fill.
function heartPaint(g, lay, c) {
  var cv = lay.cv, w = strokeW(lay) * 1.08, OFF = cv * 3;
  g.clearRect(0, 0, cv, cv);
  g.fillStyle = mix(c.tint, '#000000', 0.25); g.fillRect(0, 0, cv, cv);
  var a = g.getImageData(0, 0, cv, cv), mk = canvas(cv), mg = mk.getContext('2d');
  mg.fillStyle = '#000'; mg.fillRect(0, 0, cv, cv);
  mg.strokeStyle = 'rgba(255,255,255,.3)'; mg.lineWidth = cv * 0.03; polyPath(mg, lay.inner); mg.stroke();
  var m = mg.getImageData(0, 0, cv, cv).data, base = Math.round(c.alpha * 255);
  for (var i = 0; i < a.data.length; i += 4) a.data[i + 3] = Math.max(base, m[i]);
  g.putImageData(a, 0, 0);
  // Glow: the shape is drawn off the canvas and only its blurred shadow lands on it, which every browser
  // draws the same (a canvas blur filter is missing in some).
  g.save(); g.shadowColor = hexA(c.burn, 0.5); g.shadowBlur = w * 1.0; g.shadowOffsetX = OFF; g.translate(-OFF, 0);
  g.strokeStyle = g.fillStyle = '#000'; heartStrokes(g, lay, w * 1.4, true); g.restore();
  g.strokeStyle = g.fillStyle = hexA(mix(c.core, '#000000', 0.7), 0.95); heartStrokes(g, lay, w * 1.7, true);
  // The keyline is cut away under the number, so the fill can stay a little see-through.
  g.globalCompositeOperation = 'destination-out'; g.strokeStyle = g.fillStyle = '#000'; heartStrokes(g, lay, w * 1.04, false);
  g.globalCompositeOperation = 'source-over';
  g.globalAlpha = 0.95; g.strokeStyle = g.fillStyle = c.ink; heartStrokes(g, lay, w, false); g.globalAlpha = 1;
}

// The knot, in unit space (strands reach out to about 1). Each strand is a torus knot, tilted its own way,
// thicker and thinner along its length like beads of light.
var HEART_STRANDS = [
  { p: 2, q: 3, R: 0.62, r: 0.38, tube: 0.18, lobes: 3, rot: [0.5, 0.2, 0.0], seg: 120 },
  { p: 3, q: 2, R: 0.56, r: 0.44, tube: 0.135, lobes: 2, rot: [1.6, 0.8, 0.9], seg: 96 },
  { p: 1, q: 0, R: 1.2, r: 0, tube: 0.08, lobes: 4, rot: [1.0, 0.4, 0.0], seg: 72 },
  { p: 2, q: 5, R: 0.7, r: 0.3, tube: 0.095, lobes: 5, rot: [2.5, 1.9, 0.4], seg: 120 },
  { p: 1, q: 0, R: 1.35, r: 0, tube: 0.06, lobes: 6, rot: [2.2, 0.3, 0.8], seg: 72 }
];
var heartGeoCache = {};
// One shared geometry per quality: a tube along each strand (its u along the strand and which strand it
// is ride in attributes), and one quad each for the glow and every orbiting spark.
function heartGeo(T, hd) {
  var key = hd ? 'hd' : 'basic';
  if (heartGeoCache[key]) return heartGeoCache[key];
  var strands = hd ? HEART_STRANDS : HEART_STRANDS.slice(0, 3), rs = hd ? 6 : 5;
  var pos = [], nor = [], au = [], ast = [], idx = [];
  strands.forEach(function (s, si) {
    var rot = new T.Matrix4().makeRotationFromEuler(new T.Euler(s.rot[0], s.rot[1], s.rot[2])), curve = new T.Curve(), N = hd ? s.seg : Math.round(s.seg * 0.7);
    curve.getPoint = function (t, tg) {
      var f = t * 6.2831853, rr = s.R + s.r * Math.cos(s.q * f);
      return (tg || new T.Vector3()).set(rr * Math.cos(s.p * f), rr * Math.sin(s.p * f), s.r * Math.sin(s.q * f)).applyMatrix4(rot);
    };
    var fr = curve.computeFrenetFrames(N, true), base = pos.length / 3, P = new T.Vector3(), Nn = new T.Vector3();
    for (var i = 0; i <= N; i++) {
      var u = i / N, sn = Math.sin(u * 6.2831853 * s.lobes), rad = s.tube * (0.55 + 0.45 * sn * sn);
      curve.getPoint(u % 1, P);
      for (var j = 0; j < rs; j++) {
        var an = j / rs * 6.2831853, cs = Math.cos(an), sw = Math.sin(an);
        Nn.copy(fr.normals[i]).multiplyScalar(cs).addScaledVector(fr.binormals[i], sw);
        pos.push(P.x + Nn.x * rad, P.y + Nn.y * rad, P.z + Nn.z * rad); nor.push(Nn.x, Nn.y, Nn.z); au.push(u); ast.push(si);
      }
    }
    for (var i2 = 0; i2 < N; i2++) for (var j2 = 0; j2 < rs; j2++) {
      var a = base + i2 * rs + j2, b = base + i2 * rs + (j2 + 1) % rs, c2 = base + (i2 + 1) * rs + j2, d = base + (i2 + 1) * rs + (j2 + 1) % rs;
      idx.push(a, c2, b, b, c2, d);
    }
  });
  var knot = new T.BufferGeometry();
  knot.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); knot.setAttribute('normal', new T.Float32BufferAttribute(nor, 3));
  knot.setAttribute('aU', new T.Float32BufferAttribute(au, 1)); knot.setAttribute('aSt', new T.Float32BufferAttribute(ast, 1));
  knot.setIndex(idx); share(knot);
  // Quad 0 is the glow; the rest are sparks. aA: orbit radius, angular speed, phase, tilt. aB: size,
  // twinkle speed, twinkle phase, second tilt. aK: 0 glow, 1 spark.
  var n = hd ? 26 : 13, rnd = mulberry(4417), qp = [], qa = [], qb = [], qk = [], qi = [], corners = [[-1, -1], [1, -1], [1, 1], [-1, 1]];
  for (var sk = 0; sk <= n; sk++) {
    var spark = sk > 0, A = spark ? [0.75 + rnd() * 0.9, (0.7 + rnd() * 1.8) * (rnd() < 0.5 ? -1 : 1), rnd() * 6.28, rnd() * 3.14] : [0, 0, 0, 0];
    var Bv = spark ? [0.045 + rnd() * rnd() * 0.12, 2 + rnd() * 4, rnd() * 6.28, rnd() * 3.14] : [2.1, 0, 0, 0];
    corners.forEach(function (cn) { qp.push(cn[0], cn[1], 0); qa.push(A[0], A[1], A[2], A[3]); qb.push(Bv[0], Bv[1], Bv[2], Bv[3]); qk.push(spark ? 1 : 0); });
    qi.push(sk * 4, sk * 4 + 1, sk * 4 + 2, sk * 4, sk * 4 + 2, sk * 4 + 3);
  }
  var sp = new T.BufferGeometry();
  sp.setAttribute('position', new T.Float32BufferAttribute(qp, 3)); sp.setAttribute('aA', new T.Float32BufferAttribute(qa, 4));
  sp.setAttribute('aB', new T.Float32BufferAttribute(qb, 4)); sp.setAttribute('aK', new T.Float32BufferAttribute(qk, 1));
  sp.setIndex(qi); share(sp);
  heartGeoCache[key] = { knot: knot, sparks: sp };
  return heartGeoCache[key];
}

// The strands are drawn as soft tubes: opaque along the middle, fading to the edge, a dye colour for
// cover (so they show on a white page) with pulses of hot light streaming along them.
var HEART_KNOT_VS = [
  'attribute float aU; attribute float aSt;',
  'varying vec3 vN; varying vec3 vV; varying float vU; varying float vSt;',
  'void main() {',
  '  vec4 mv = modelViewMatrix * vec4( position, 1.0 );',
  '  vN = normalize( normalMatrix * normal ); vV = normalize( -mv.xyz ); vU = aU; vSt = aSt;',
  '  gl_Position = projectionMatrix * mv;',
  '}'
].join('\n');
var HEART_KNOT_FS = [
  'uniform float uFlow; uniform float uGain; uniform vec3 uCore; uniform vec3 uHot;',
  'varying vec3 vN; varying vec3 vV; varying float vU; varying float vSt;',
  'void main() {',
  '  float f = abs( dot( normalize( vN ), normalize( vV ) ) );',
  '  float edge = smoothstep( 0.0, 0.42, f );',
  '  float s1 = pow( 0.5 + 0.5 * sin( 6.2831853 * ( vU * 2.0 - uFlow ) + vSt * 2.1 ), 9.0 );',
  '  float s2 = pow( 0.5 + 0.5 * sin( 6.2831853 * ( vU * 5.0 + uFlow * 1.7 ) + vSt * 4.3 ), 20.0 );',
  '  float hot = clamp( ( s1 * 0.75 + s2 * 1.0 ) * uGain, 0.0, 1.0 );',
  '  vec3 body = uCore * ( 0.55 + 0.55 * f );',
  '  vec3 col = mix( body, uHot, hot * ( 0.35 + 0.45 * f ) );',
  '  float a = edge * ( 0.94 + 0.06 * hot );',
  '  gl_FragColor = vec4( col, 1.0 );',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '  gl_FragColor = vec4( gl_FragColor.rgb * a, a );',
  '}'
].join('\n');
// The glow and the sparks: billboards in view space, placed on the card from a few numbers.
var HEART_SPARK_VS = [
  'uniform float uOrb; uniform float uT; uniform float uGlow; uniform float uScale;',
  'attribute vec4 aA; attribute vec4 aB; attribute float aK;',
  'varying vec2 vC; varying float vK; varying float vTw;',
  'void main() {',
  '  vec3 o = vec3( 0.0 ); float tw = 1.0;',
  '  if ( aK > 0.5 ) {',
  '    float an = aA.z + uOrb * aA.y;',
  '    o = vec3( cos( an ), 0.0, sin( an ) ) * aA.x * uScale;',
  '    float c1 = cos( aA.w ), s1 = sin( aA.w ), c2 = cos( aB.w ), s2 = sin( aB.w );',
  '    o = vec3( o.x, o.y * c1 - o.z * s1, o.y * s1 + o.z * c1 );',
  '    o = vec3( o.x * c2 - o.y * s2, o.x * s2 + o.y * c2, o.z );',
  '    tw = 0.5 + 0.5 * sin( uT * aB.y + aB.z );',
  '  }',
  '  vec4 mv = modelViewMatrix * vec4( o, 1.0 );',
  '  float sz = aB.x * uScale * ( aK > 0.5 ? 0.5 + 0.7 * tw : uGlow );',
  '  mv.xy += position.xy * sz;',
  '  vC = position.xy; vK = aK; vTw = tw;',
  '  gl_Position = projectionMatrix * mv;',
  '}'
].join('\n');
var HEART_SPARK_FS = [
  'uniform float uGain; uniform vec3 uCore; uniform vec3 uHot;',
  'varying vec2 vC; varying float vK; varying float vTw;',
  'void main() {',
  '  float d = length( vC ); if ( d > 1.0 ) discard;',
  '  vec3 col; float a;',
  '  if ( vK < 0.5 ) { float s = exp( -d * d * 4.0 ); col = mix( uCore, uHot, s * s * 0.55 ); a = s * 0.4 * uGain; }',
  '  else { float s = 1.0 - smoothstep( 0.0, 1.0, d ); col = mix( uCore, uHot, s * s ); a = ( 0.3 + 0.7 * s ) * ( 0.35 + 0.65 * vTw ); }',
  '  gl_FragColor = vec4( col, 1.0 );',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '  gl_FragColor = vec4( gl_FragColor.rgb * a, a );',
  '}'
].join('\n');

// Everything about the knot's life: the strands and sparks, their uniforms, and the spring that makes
// the knot stretch while the die spins and settle when it stops.
function heartBuild(T, die, q) {
  var hd = q.detail !== 'basic', c = die.th, G = heartGeo(T, hd), rnd = mulberry(++heartCount * 977 + die.sides * 31 + 5);
  var Rc = die.rest * 0.44, core = new T.Color(c.core), hot = new T.Color(c.hot);
  var shared = { uT: { value: 0 }, uGain: { value: 1 }, uCore: { value: core }, uHot: { value: hot } };
  var kmat = new T.ShaderMaterial({ vertexShader: HEART_KNOT_VS, fragmentShader: HEART_KNOT_FS, transparent: true, depthWrite: false, premultipliedAlpha: true,
    uniforms: { uFlow: { value: 0 }, uGain: shared.uGain, uCore: shared.uCore, uHot: shared.uHot } });
  var smat = new T.ShaderMaterial({ vertexShader: HEART_SPARK_VS, fragmentShader: HEART_SPARK_FS, transparent: true, depthWrite: false, premultipliedAlpha: true,
    uniforms: { uOrb: { value: 0 }, uT: shared.uT, uGlow: { value: 1 }, uScale: { value: Rc }, uGain: shared.uGain, uCore: shared.uCore, uHot: shared.uHot } });
  var knot = new T.Mesh(G.knot, kmat), sparks = new T.Mesh(G.sparks, smat);
  // Between the far side of the glass (order 1) and the near side (order 2), so the near faces lie over it.
  knot.frustumCulled = sparks.frustumCulled = false; knot.renderOrder = 1.5; sparks.renderOrder = 1.6;
  var axis = new T.Group(), spinG = new T.Group();
  spinG.scale.setScalar(Rc); axis.add(spinG); spinG.add(knot);
  var t = rnd() * 50, flow = rnd(), orb = rnd() * 6.28, ang = rnd() * 6.28, pre = rnd() * 6.28, s = 0, sx = 1, sv = 0, seed = rnd() * 6.28;
  var number = die.mat.userData.heartU || null, numberT = die.mat.userData.heartT || null;
  axis.rotation.set(0.4 + rnd(), rnd() * 6.28, 0);
  return {
    parts: [axis, sparks],
    tick: function (dt, spin) {
      dt = Math.min(dt, 0.05); t += dt;
      // How hard the die is tumbling, 0 to 1: quick to wake, slow to settle.
      var want = Math.min(1, Math.max(0, spin / 10));
      s += (want - s) * (1 - Math.exp(-dt * (want > s ? 9 : 2.4)));
      // A spring: the knot stretches while it spins and, springing back, overshoots a touch as it settles.
      sv += ((1 + 0.38 * s - sx) * 70 - sv * 7) * dt; sx += sv * dt;
      var xy = 1 / Math.sqrt(sx);
      spinG.scale.set(Rc * xy, Rc * xy, Rc * sx);
      ang += (0.32 + 8 * s) * dt; spinG.rotation.z = ang;
      pre += (0.18 + 1.6 * s) * dt; axis.rotation.x = 0.7 + 0.35 * Math.sin(pre * 0.7); axis.rotation.y = pre;
      flow += (0.1 + 1.5 * s) * dt; orb += (0.35 + 2.6 * s) * dt;
      var gain = 0.78 + 0.4 * s + 0.06 * Math.sin(t * 1.9 + seed);
      kmat.uniforms.uFlow.value = flow; smat.uniforms.uOrb.value = orb; smat.uniforms.uGlow.value = 1 + 0.18 * s;
      shared.uT.value = t; shared.uGain.value = gain;
      // The core lights the frosted numbers: steadier at rest, brighter and flickering while it spins.
      var lit = 0.32 + 0.3 * s + 0.05 * Math.sin(t * 2.3 + seed) + 0.05 * s * Math.sin(t * 17);
      if (number) { number.value = lit; numberT.value = t; }
    },
    dispose: function () { kmat.dispose(); smat.dispose(); }
  };
}

registerSet({
  key: 'heartlight', name: 'Heartlight', note: 'Clear crystal with a knot of magic swirling at its centre, the numbers frosted into the glass', show: 'inscribe', bev: 0.045, see: 'glass',
  colours: heartColours,
  paint: heartPaint,
  // Frosted numbers are matte; the crystal round them stays glossy.
  surface: function (g, lay) {
    var w = strokeW(lay) * 1.08;
    g.strokeStyle = g.fillStyle = 'rgb(0,170,0)'; heartStrokes(g, lay, w * 1.7, true);
    g.strokeStyle = g.fillStyle = 'rgb(0,215,0)'; heartStrokes(g, lay, w, false);
  },
  // The numbers are etched a little below the face.
  height: function (g, lay) {
    var w = strokeW(lay) * 1.08;
    g.strokeStyle = g.fillStyle = '#b4b4b4'; heartStrokes(g, lay, w * 1.7, true);
    g.strokeStyle = g.fillStyle = '#7e7e7e'; heartStrokes(g, lay, w, false);
  },
  rim: function (c) { return { col: hexA(mix(c.tint, '#000000', 0.5), 0.3), rough: 0.03, metal: 0 }; },
  // A lit number over a white page needs some colour of its own to cover it.
  cover: function () { return 0.5; },
  physical: function (c, q) {
    return { clearcoat: q.detail === 'basic' ? 0 : 0.4, clearcoatRoughness: 0.05, envMapIntensity: 1.1, userData: { heartU: { value: 0.32 }, heartT: { value: 0 } } };
  },
  // Light from the core reaches the frosted number: added where the image is nearly opaque (the number),
  // before the glass adds its own opacity. On the far side's copy, seen mirrored through the glass, the
  // numbers drop to a faint ghost and the halo and frosted lip go, so only the top face's number reads.
  shader: function (s, c) {
    var Col = this.color.constructor, back = this.side === 1;
    s.uniforms.uHeart = this.userData.heartU; s.uniforms.uHeartT = this.userData.heartT; s.uniforms.uHeartCol = { value: new Col(c.burn) };
    var lit = back ?
      'float heartKeep = smoothstep( 0.2, 0.5, diffuseColor.a );\ndiffuseColor.a = mix( ' + c.alpha.toFixed(3) + ', diffuseColor.a * 0.2, heartKeep );\n' :
      'float heartNum = smoothstep( 0.62, 0.9, diffuseColor.a );\nfloat heartFlick = 0.75 + 0.25 * sin( vMapUv.x * 34.0 + vMapUv.y * 21.0 + uHeartT * 1.7 ) * sin( vMapUv.y * 29.0 - vMapUv.x * 17.0 - uHeartT * 1.2 );\noutgoingLight += uHeartCol * heartNum * uHeart * heartFlick;\n';
    s.fragmentShader = 'uniform float uHeart; uniform float uHeartT; uniform vec3 uHeartCol;\n' + s.fragmentShader.replace('float glassSpec', lit + 'float glassSpec');
  },
  build: heartBuild
});
