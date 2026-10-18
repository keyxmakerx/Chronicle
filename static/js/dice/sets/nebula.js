// ==================== Nebula resin ====================
// Dark resin with a nebula poured into it. The clouds are a real volume: a short ray march through
// the die's own shape, so they have depth, parallax as the die turns, and bend a little at each facet
// the way light does entering resin. Stars are an instanced mesh (so the Gather show can pull them
// into the number) with a few glints on top. The number is solid pale paint on the face.
import {
  flakeLayout, hexA, mix, mulberry, paintNumeral, registerSet, strokeW, share
} from '../engine.js';

// ---------- Colours ----------
// cloud: the two nebula colours; stars: the star colours; stretch: how far the noise is pulled along
// each axis (aurora curtains); lens: how much of the volume is cloud, how bright its cores run and how dark the dust lanes are.
var NEBULA = [
  { key: 'orion', name: 'Orion', note: 'Magenta and blue clouds, pale pink numbers',
    cloud: ['#e8209a', '#2f5cff'], stars: ['#ffffff', '#dfe6ff', '#ffd9f0', '#b7caff'], lens: [1.0, 1.0, 1.0], stretch: [1, 1, 1],
    ink: '#ffe6f6', burn: '#ff6fd0', shadow: '#7a2bb0', rim: '#c04ad0', base: { grad: ['#1b1034', '#07040f'], rough: 0.26 } },
  { key: 'carina', name: 'Carina', note: 'Orange and teal clouds, cream numbers',
    cloud: ['#ff7a18', '#0fb5b0'], stars: ['#ffffff', '#fff0d0', '#c8fff6', '#ffd9a8'], lens: [1.0, 1.05, 1.1], stretch: [1, 1, 1],
    ink: '#fff0dc', burn: '#ffae5a', shadow: '#0f7f84', rim: '#ff9a40', base: { grad: ['#102226', '#050a0c'], rough: 0.26 } },
  { key: 'veil', name: 'Veil', note: 'Green and violet clouds, mint numbers',
    cloud: ['#2fe27c', '#8a40ff'], stars: ['#ffffff', '#e4ffe9', '#eadcff', '#b4ffd2'], lens: [1.0, 1.0, 0.9], stretch: [1, 1, 1],
    ink: '#e8ffee', burn: '#8dffb4', shadow: '#2a8a4f', rim: '#5ae09a', base: { grad: ['#0f2218', '#040a07'], rough: 0.26 } },
  { key: 'void', name: 'Void', note: 'Black resin, white stars, pale numbers',
    cloud: ['#4a5890', '#7d86a8'], stars: ['#ffffff', '#ffffff', '#e8eeff', '#fff4e2'], lens: [0.5, 0.55, 1.3], stretch: [1, 1, 1],
    ink: '#f5f7ff', burn: '#cfdcff', shadow: '#26293a', rim: '#8e98c0', base: { grad: ['#0e0f16', '#020204'], rough: 0.24 } },
  { key: 'aurora', name: 'Aurora', note: 'Green and violet curtains of light, pale mint numbers',
    cloud: ['#35ffb0', '#b050ff'], stars: ['#ffffff', '#d8fff0', '#f0dcff', '#bffff0'], lens: [0.85, 1.1, 0.9], stretch: [1.0, 0.3, 1.0],
    ink: '#e4fff6', burn: '#6bffd0', shadow: '#1f8a78', rim: '#5af0c0', base: { grad: ['#0a1d20', '#030809'], rough: 0.26 } }
];
NEBULA.forEach(function (c) { c.flakes = c.stars; c.alpha = -0.2; c.tint = c.shadow; });

// ---------- A little 3D noise, shared by every die ----------
// Four independent periodic value-noise channels, three octaves each, stretched to the full 0..1 so
// the shader can threshold them. One fetch gives four different noises.
var nebNoiseTex = null;
function nebNoise(T) {
  if (nebNoiseTex) return nebNoiseTex;
  var N = 32, data = new Uint8Array(N * N * N * 4), rnd = mulberry(913), oct = [[4, 0.55], [8, 0.3], [16, 0.15]], ch, k, i, x, y, z;
  function sm(t) { return t * t * (3 - 2 * t); }
  for (ch = 0; ch < 4; ch++) {
    var lat = oct.map(function (o) { var L = new Float32Array(o[0] * o[0] * o[0]); for (var j = 0; j < L.length; j++) L[j] = rnd(); return L; });
    var vals = new Float32Array(N * N * N), lo = 9, hi = -9;
    for (z = 0; z < N; z++) for (y = 0; y < N; y++) for (x = 0; x < N; x++) {
      var v = 0;
      for (k = 0; k < 3; k++) {
        var f = oct[k][0], L = lat[k], px = x / N * f, py = y / N * f, pz = z / N * f;
        var ix = Math.floor(px), iy = Math.floor(py), iz = Math.floor(pz), fx = sm(px - ix), fy = sm(py - iy), fz = sm(pz - iz);
        var x0 = ix % f, x1 = (ix + 1) % f, y0 = iy % f, y1 = (iy + 1) % f, z0 = iz % f, z1 = (iz + 1) % f;
        function at(a, b, c) { return L[(c * f + b) * f + a]; }
        var a0 = at(x0, y0, z0) + (at(x1, y0, z0) - at(x0, y0, z0)) * fx, a1 = at(x0, y1, z0) + (at(x1, y1, z0) - at(x0, y1, z0)) * fx;
        var b0 = at(x0, y0, z1) + (at(x1, y0, z1) - at(x0, y0, z1)) * fx, b1 = at(x0, y1, z1) + (at(x1, y1, z1) - at(x0, y1, z1)) * fx;
        var c0 = a0 + (a1 - a0) * fy, c1 = b0 + (b1 - b0) * fy;
        v += oct[k][1] * (c0 + (c1 - c0) * fz);
      }
      vals[(z * N + y) * N + x] = v; if (v < lo) lo = v; if (v > hi) hi = v;
    }
    for (i = 0; i < vals.length; i++) data[i * 4 + ch] = Math.round((vals[i] - lo) / (hi - lo) * 255);
  }
  var t = new T.Data3DTexture(data, N, N, N);
  t.format = T.RGBAFormat; t.type = T.UnsignedByteType; t.minFilter = t.magFilter = T.LinearFilter; t.generateMipmaps = false;
  t.wrapS = t.wrapT = t.wrapR = T.RepeatWrapping; t.unpackAlignment = 1; t.needsUpdate = true; share(t);
  nebNoiseTex = t;
  return t;
}

// ---------- The volume ----------
var NEB_VS = [
  'varying vec3 vP; varying vec3 vO;',
  'void main() {',
  '  vP = position; vO = ( inverse( modelMatrix ) * vec4( cameraPosition, 1.0 ) ).xyz;',
  '  gl_Position = projectionMatrix * modelViewMatrix * vec4( position, 1.0 );',
  '}'
].join('\n');
// The ray enters the die's inset shape through one facet, bends as it goes into the resin, and is
// marched to the far side. Cloud density is thresholded noise; filaments come from where a second noise
// crosses its middle value, dust lanes absorb, and bright cores glow where the cloud is thickest.
var NEB_FS = [
  'precision highp sampler3D;',
  'uniform sampler3D uNoise; uniform vec4 uPl[NP];',
  'uniform vec3 uA; uniform vec3 uB; uniform vec3 uSeed; uniform vec3 uStretch; uniform vec3 uLens;',
  'uniform float uTime; uniform float uSize;',
  'varying vec3 vP; varying vec3 vO;',
  'float nbHash( vec2 p ) { return fract( sin( dot( p, vec2( 12.9898, 78.233 ) ) ) * 43758.5453 ); }',
  'void main() {',
  '  vec3 rd = normalize( vP - vO );',
  '  float tin = 0.0, tout = 1e5; vec3 nIn = -rd;',
  '  for ( int i = 0; i < NP; i++ ) {',
  '    vec4 pl = uPl[ i ]; float dn = dot( pl.xyz, rd ), t = ( pl.w - dot( pl.xyz, vO ) ) / dn;',
  '    if ( dn < 0.0 ) { if ( t > tin ) { tin = t; nIn = pl.xyz; } } else { tout = min( tout, t ); }',
  '  }',
  '  if ( tin >= tout ) discard;',
  '  vec3 pe = vO + rd * tin, rd2 = refract( rd, nIn, 0.74 );',
  '  if ( dot( rd2, rd2 ) < 0.5 ) rd2 = rd;',
  '  float t2 = 1e5;',
  '  for ( int i = 0; i < NP; i++ ) {',
  '    vec4 pl = uPl[ i ]; float dn = dot( pl.xyz, rd2 );',
  '    if ( dn > 0.0 ) t2 = min( t2, ( pl.w - dot( pl.xyz, pe ) ) / dn );',
  '  }',
  '  float ds = t2 / float( STEPS ), j = nbHash( gl_FragCoord.xy );',
  '  vec3 acc = vec3( 0.0 ); float T = 1.0;',
  '  for ( int i = 0; i < STEPS; i++ ) {',
  '    vec3 p = pe + rd2 * ( ( float( i ) + j ) * ds );',
  '    vec3 q = p / uSize * uStretch * 0.55 + uSeed + vec3( uTime * 0.02, uTime * 0.013, uTime * 0.017 );',
  '    vec4 a = texture( uNoise, q ), b = texture( uNoise, q * 2.3 + 0.37 );',
  '    float fb = a.r * 0.62 + b.r * 0.38;',
  '    float body = smoothstep( 0.46, 0.72, fb );',
  '    float wisp = 1.0 - abs( a.g * 2.0 - 1.0 ); wisp = wisp * wisp * wisp * wisp;',
  '    float dens = body * ( 0.12 + 1.1 * wisp ) * uLens.x;',
  '    float hue = smoothstep( 0.4, 0.6, a.b * 0.75 + b.g * 0.25 );',
  '    float dust = smoothstep( 0.5, 0.78, b.b * 0.65 + a.a * 0.35 ) * smoothstep( 0.3, 0.55, fb );',
  '    vec3 col = mix( uA, uB, hue );',
  '    float core = dens * dens;',
  '    vec3 em = col * ( 0.35 + 1.8 * core * uLens.y ) * dens + vec3( 0.9 ) * core * dens * 0.2 * uLens.y;',
  '    float sig = dens * 1.5 + dust * 5.0 * uLens.z + 0.55;',
  '    float k = exp( -sig * ds / uSize * 2.2 );',
  '    acc += T * em * ( 1.0 - k ) / max( sig, 0.001 ) * 5.0;',
  '    T *= k;',
  '  }',
  '  gl_FragColor = vec4( acc, 1.0 - T );',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');

// A glint: a hot point with four thin rays, camera-facing, that twinkles slowly.
var NEB_GVS = [
  'varying vec2 vUv; varying float vPh;',
  'void main() {',
  '  vUv = position.xy; vPh = float( gl_InstanceID );',
  '  vec4 mv = modelViewMatrix * instanceMatrix * vec4( 0.0, 0.0, 0.0, 1.0 );',
  '  mv.xy += position.xy * length( ( instanceMatrix * vec4( 1.0, 0.0, 0.0, 0.0 ) ).xyz );',
  '  gl_Position = projectionMatrix * mv;',
  '}'
].join('\n');
var NEB_GFS = [
  'uniform float uTime; uniform vec3 uCol; varying vec2 vUv; varying float vPh;',
  'void main() {',
  '  vec2 p = vUv; float r = length( p );',
  '  float core = exp( -r * r * 26.0 );',
  '  float ray = exp( -abs( p.x ) * 22.0 ) * exp( -abs( p.y ) * 3.2 ) + exp( -abs( p.y ) * 22.0 ) * exp( -abs( p.x ) * 3.2 );',
  '  float tw = 0.5 + 0.5 * sin( uTime * 1.7 + vPh * 6.3 );',
  '  vec3 c = uCol * ( core * 1.6 + ray * ( 0.5 + 1.1 * tw ) ) * ( 1.0 - smoothstep( 0.8, 1.0, r ) );',
  '  gl_FragColor = vec4( c, 1.0 );',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');

var nebStarGeo = null, nebStarMat = null, nebQuad = null;
function nebStarGeometry(T) {
  if (nebStarGeo) return nebStarGeo;
  nebStarGeo = new T.IcosahedronGeometry(1, 0); share(nebStarGeo);
  return nebStarGeo;
}
// Stars are bright enough to bloom in HD; the colour multiplies each one's own colour.
function nebStarMaterial(T) {
  if (nebStarMat) return nebStarMat;
  nebStarMat = new T.MeshBasicMaterial({ color: new T.Color(1.2, 1.2, 1.2), transparent: true, depthWrite: false, blending: T.AdditiveBlending }); share(nebStarMat);
  return nebStarMat;
}

// Stars sit anywhere clear of the faces, mostly pinpoints and a few larger. Each belongs to its nearest
// face, so a face that comes apart carries its own stars.
function nebStarList(T, die, hd) {
  var base = flakeLayout(T, die.sd, die.size, hd ? 260 : 120, die.sides * 13 + 5), rnd = mulberry(die.sides * 29 + 11), s = die.size, out = [];
  var p = new T.Vector3(), q = new T.Quaternion(), sc = new T.Vector3();
  base.forEach(function (fl) {
    fl.m.decompose(p, q, sc);
    var r = s * (0.008 + 0.03 * Math.pow(rnd(), 3.5));
    out.push({ fi: fl.fi, m: new T.Matrix4().compose(p.clone(), q.clone(), new T.Vector3(r, r, r)), pick: fl.pick, r: r });
  });
  return out;
}

registerSet({
  key: 'nebula', name: 'Nebula resin', show: 'gather', bev: 0.07, see: 'glass', dense: true, relief: 0,
  note: 'Dark resin with drifting coloured clouds and pinpoint stars. The number in solid pale colour floating in the resin.',
  highlightWhat: 'The stars inside drift together and trace the number just under its face.',
  colours: NEBULA,

  // The face: a dark, mostly clear skin over the volume; the number is solid pale paint with a dark
  // halo and keyline, so a bright cloud behind it never eats the strokes.
  paint: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay) * 1.08;
    g.clearRect(0, 0, cv, cv);
    var gr = g.createRadialGradient(cv * 0.38, cv * 0.32, cv * 0.04, cv / 2, cv / 2, cv * 0.8);
    gr.addColorStop(0, hexA(c.base.grad[0], 0.34)); gr.addColorStop(1, hexA(c.base.grad[1], 0.5));
    g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (3.2 * k) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = 'rgba(3,2,8,.9)'; paintNumeral(g, lay, lay.style, w * 2.5); g.restore();
    g.fillStyle = g.strokeStyle = 'rgb(6,4,12)'; paintNumeral(g, lay, lay.style, w * 1.6);
    g.fillStyle = g.strokeStyle = c.ink; paintNumeral(g, lay, lay.style, w);
  },
  surface: function (g, lay) { g.fillStyle = g.strokeStyle = 'rgb(0,120,0)'; paintNumeral(g, lay, lay.style, strokeW(lay) * 1.5); },
  rim: function (c) { return { col: hexA(mix(c.base.grad[1], c.rim, 0.12), 0.4), rough: 0.4, metal: 0 }; },
  physical: function (c, q) { var hd = q.detail !== 'basic'; return { clearcoat: hd ? 0.5 : 0, clearcoatRoughness: 0.12, envMapIntensity: 0.9 }; },
  cover: function () { return 0; },

  build: function (T, die, q) {
    var hd = q.detail !== 'basic', c = die.th, s = die.size, f;
    // The clouds.
    var np = die.sd.faces.length, planes = die.sd.faces.map(function (fc) { return new T.Vector4(fc.n.x, fc.n.y, fc.n.z, (fc.c.x * fc.n.x + fc.c.y * fc.n.y + fc.c.z * fc.n.z) * s * 0.965); });
    var u = {
      uNoise: { value: nebNoise(T) }, uPl: { value: planes }, uA: { value: new T.Color(c.cloud[0]) }, uB: { value: new T.Color(c.cloud[1]) },
      uSeed: { value: new T.Vector3(die.sides * 0.31, die.sides * 0.17, die.sides * 0.23) }, uStretch: { value: new T.Vector3(c.stretch[0], c.stretch[1], c.stretch[2]) },
      uLens: { value: new T.Vector3(c.lens[0], c.lens[1], c.lens[2]) }, uTime: { value: 0 }, uSize: { value: s }
    };
    var vm = new T.ShaderMaterial({ vertexShader: NEB_VS, fragmentShader: NEB_FS, uniforms: u, defines: { NP: np, STEPS: hd ? 18 : 10 }, transparent: true, depthWrite: false,
      blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor });
    var vol = new T.Mesh(die.body.geo, vm); vol.renderOrder = 1.5; vol.frustumCulled = false;

    // The stars.
    var list = nebStarList(T, die, hd), stars = new T.InstancedMesh(nebStarGeometry(T), nebStarMaterial(T), list.length), col = new T.Color();
    list.forEach(function (fl, i) { stars.setMatrixAt(i, fl.m); stars.setColorAt(i, col.set(c.stars[Math.floor(fl.pick * c.stars.length)])); });
    stars.frustumCulled = false; stars.renderOrder = 1.7;
    die.flakes = stars; die.flakeList = list; die.parts.push(stars);

    // A few glints, on the larger stars.
    var gn = hd ? 9 : 5, rnd = mulberry(die.sides * 41 + 3), gm = new T.Matrix4(), gq = new T.Quaternion(), big = list.slice().sort(function (a, b) { return b.r - a.r; }).slice(0, gn * 2);
    var gmat = new T.ShaderMaterial({ vertexShader: NEB_GVS, fragmentShader: NEB_GFS, uniforms: { uTime: { value: 0 }, uCol: { value: new T.Color('#ffffff').lerp(new T.Color(c.cloud[0]), 0.15) } },
      transparent: true, depthWrite: false, blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneFactor, blendSrcAlpha: T.ZeroFactor, blendDstAlpha: T.OneFactor });
    if (!nebQuad) { nebQuad = new T.PlaneGeometry(2, 2); share(nebQuad); }
    var glints = new T.InstancedMesh(nebQuad, gmat, gn), pp = new T.Vector3(), ps = new T.Vector3();
    for (f = 0; f < gn; f++) {
      var pick = big.splice(Math.floor(rnd() * big.length), 1)[0], r = s * (0.07 + rnd() * 0.07);
      pick.m.decompose(pp, gq, ps); glints.setMatrixAt(f, gm.compose(pp, gq.identity(), ps.set(r, r, r)));
    }
    glints.renderOrder = 1.6; glints.frustumCulled = false;

    var t = 0;
    return {
      parts: [vol, stars, glints],
      tick: function (dt, spin) {
        t += dt * (1 + Math.min(spin, 12) * 0.35); u.uTime.value = t; gmat.uniforms.uTime.value = t;
        // The volume belongs to the body: when a crit splits the die into face pieces it goes with it.
        vol.visible = glints.visible = die.main.visible;
      },
      dispose: function () { vm.dispose(); gmat.dispose(); glints.dispose(); }
    };
  }
});
