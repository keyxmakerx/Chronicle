// ---------- Crit fail: Eldritch grasp ----------
import {
  clamp01, cracksFor, cutPaths, dimmer, drawBurn, drop, env4, glow, glowFace, glowStrokes, in3,
  inout, mulberry, out3, registerCrit, smooth, splitDie, spray, starTex, strokeW, tintDie, wave,
  wholeDie, writhe
} from '../engine.js';

// ---------- Eldritch grasp, on the page ----------
// The showcase's grasp, brought onto the page: the same tendrils, portal and timing, with the camera
// fixed overhead and the room's own lights. Basic draws thinner meshes and fewer tendrils.
var GRASP_VEIN = '#b45cff', GRASP_BLOOD = '#ff2a1f';

// ---------- A detailed tendril ----------
// A real lit surface, not a flat shader: wet clearcoat, ringed muscle in the geometry itself, a mottled
// hide, veins that pulse toward the tip, and two rows of suckers along the underside.
var GRASP_NOISE = [
  'float hh(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }',
  'float nn(vec2 p){ vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f); return mix(mix(hh(i), hh(i + vec2(1.0, 0.0)), u.x), mix(hh(i + vec2(0.0, 1.0)), hh(i + vec2(1.0, 1.0)), u.x), u.y); }',
  'float fb(vec2 p){ float s = 0.0, a = 0.5; for (int i = 0; i < 5; i++) { s += a * nn(p); p = p * 2.03 + 1.7; a *= 0.5; } return s; }'].join('\n');

function graspTendril(st, o) {
  var T = st.T, hd = st.q.detail !== 'basic', SEG = hd ? 150 : 80, SID = hd ? 22 : 12, NV = (SEG + 1) * (SID + 1);
  var pos = new Float32Array(NV * 3), nrm = new Float32Array(NV * 3), au = new Float32Array(NV), aa = new Float32Array(NV), idx = [];
  for (var k = 0; k < SEG; k++) for (var j = 0; j < SID; j++) { var a = k * (SID + 1) + j, b = a + 1, c = a + SID + 1, d = c + 1; idx.push(a, c, b, b, c, d); }
  for (k = 0; k <= SEG; k++) for (j = 0; j <= SID; j++) { au[k * (SID + 1) + j] = k / SEG; aa[k * (SID + 1) + j] = j / SID; }
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.BufferAttribute(pos, 3)); geo.setAttribute('normal', new T.BufferAttribute(nrm, 3));
  geo.setAttribute('aU', new T.BufferAttribute(au, 1)); geo.setAttribute('aA', new T.BufferAttribute(aa, 1)); geo.setIndex(idx);
  var uni = { uTime: { value: 0 }, uGlow: { value: 1 }, uVein: { value: new T.Color(GRASP_VEIN) }, uHot: { value: new T.Color(GRASP_BLOOD) }, uSpot: { value: new T.Color('#7a3a62') } };
  var mat = new T.MeshPhysicalMaterial({ color: '#4c2444', roughness: 0.42, metalness: 0, clearcoat: 1, clearcoatRoughness: 0.12, sheen: 1, sheenColor: new T.Color('#b070c0'), sheenRoughness: 0.5, envMapIntensity: 0.9 });
  mat.onBeforeCompile = function (sh) {
    Object.assign(sh.uniforms, uni);
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nattribute float aU; attribute float aA; varying float vU; varying float vA;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvU = aU; vA = aA;');
    sh.fragmentShader = sh.fragmentShader.replace('#include <common>', '#include <common>\nuniform float uTime, uGlow; uniform vec3 uVein, uHot, uSpot; varying float vU; varying float vA;\n' + GRASP_NOISE)
      .replace('#include <color_fragment>', [
        '#include <color_fragment>',
        // Mottled hide: dark plum with bruise-coloured patches, paler toward the underside.
        'vec2 hp = vec2(vU * 46.0, vA * 7.0); float mott = fb(hp * 0.9);',
        'diffuseColor.rgb = mix(diffuseColor.rgb, uSpot, smoothstep(0.5, 0.75, mott) * 0.7);',
        'float under = smoothstep(0.35, 0.0, abs(vA - 0.5));',
        'diffuseColor.rgb = mix(diffuseColor.rgb, vec3(0.42, 0.24, 0.34), under * 0.55);'].join('\n'))
      .replace('#include <roughnessmap_fragment>', '#include <roughnessmap_fragment>\nroughnessFactor = clamp(roughnessFactor + (fb(vec2(vU * 90.0, vA * 14.0)) - 0.5) * 0.35, 0.15, 1.0);')
      .replace('#include <emissivemap_fragment>', [
        '#include <emissivemap_fragment>',
        // Veins: thin branching lines along the length, with a pulse running from the root to the tip.
        'float vl = abs(fb(vec2(vU * 18.0, vA * 9.0 + sin(vU * 31.0) * 0.4)) - 0.5);',
        'float vein = smoothstep(0.035, 0.0, vl) * (1.0 - under);',
        'float pulse = pow(0.5 + 0.5 * sin(vU * 26.0 - uTime * 8.0), 4.0);',
        'vec3 vv = mix(uVein, uHot, pow(pulse, 2.0) * 0.6);',
        'totalEmissiveRadiance += vv * vein * (0.6 + 3.4 * pulse) * uGlow * smoothstep(0.0, 0.12, vU);',
        // Light under the skin at the edges, as if lit from inside.
        'float fr = pow(1.0 - abs(dot(normal, normalize(vViewPosition))), 3.0);',
        'totalEmissiveRadiance += uVein * fr * 0.35 * uGlow;'].join('\n'));
  };
  var mesh = new T.Mesh(geo, mat); mesh.frustumCulled = false; mesh.castShadow = true; mesh.receiveShadow = true;

  // Suckers: little cups in two rows down the underside.
  var NS = 2 * (hd ? 26 : 16), cupGeo = new T.LatheGeometry([[0, -0.15], [0.55, -0.1], [0.95, 0.12], [1, 0.32], [0.82, 0.38], [0.62, 0.2], [0.3, 0.12], [0, 0.1]].map(function (p) { return new T.Vector2(p[0], p[1]); }), 14);
  cupGeo.computeVertexNormals();
  var cupMat = new T.MeshPhysicalMaterial({ color: '#b98aa6', roughness: 0.38, clearcoat: 1, clearcoatRoughness: 0.1, sheen: 0.5, sheenColor: new T.Color('#ffb0d8'), emissive: new T.Color('#3a0a2a'), emissiveIntensity: 0.6 });
  var cups = new T.InstancedMesh(cupGeo, cupMat, NS); cups.frustumCulled = false; cups.castShadow = true;
  var grp = new T.Group(); grp.add(mesh, cups);

  var curve = new T.CatmullRomCurve3([new T.Vector3(), new T.Vector3(0, 1, 0)], false, 'centripetal'), P = [], Tn = [], Ns = [], Bs = [], RR = [];
  for (k = 0; k <= SEG; k++) { P.push(new T.Vector3()); Tn.push(new T.Vector3()); Ns.push(new T.Vector3()); Bs.push(new T.Vector3()); RR.push(0); }
  var N = new T.Vector3(), B = new T.Vector3(), tmp = new T.Vector3(), m4 = new T.Matrix4(), q = new T.Quaternion(), sc = new T.Vector3(), up = new T.Vector3(0, 1, 0);
  var r0 = o.r, ph = o.ph || 0;
  return { group: grp, mesh: mesh, uni: uni, update: function (ctrl, upto, thick, time) {
    curve.points = ctrl; curve.updateArcLengths();
    for (var k = 0; k <= SEG; k++) P[k].copy(curve.getPointAt(Math.max(0, Math.min(1, k / SEG * upto))));
    for (k = 0; k <= SEG; k++) { var a = P[Math.max(0, k - 1)], b = P[Math.min(SEG, k + 1)]; Tn[k].subVectors(b, a); if (Tn[k].lengthSq() < 1e-12) Tn[k].set(0, 1, 0); Tn[k].normalize(); }
    // Parallel-transported frames, so the suckers stay in a line along the underside as it bends.
    N.set(1, 0, 0); if (Math.abs(N.dot(Tn[0])) > 0.9) N.set(0, 0, 1); N.cross(Tn[0]).normalize();
    for (k = 0; k <= SEG; k++) {
      if (k) { B.crossVectors(Tn[k - 1], Tn[k]); if (B.lengthSq() > 1e-12) { B.normalize(); N.applyAxisAngle(B, Math.acos(Math.max(-1, Math.min(1, Tn[k - 1].dot(Tn[k]))))); } }
      B.crossVectors(Tn[k], N).normalize(); Ns[k].copy(N); Bs[k].copy(B);
      var u = k / SEG;
      // Taper to a fine tip, ringed muscle, and a slow swallowing bulge that runs up the length.
      var taper = Math.pow(1 - u, 0.9) * 0.94 + 0.03, ring = 1 + 0.045 * Math.sin(u * 140) * (1 - u), gulp = 1 + 0.2 * Math.exp(-Math.pow((u - ((time * 0.45 + ph) % 1.4) + 0.2) * 7, 2));
      var rr = r0 * thick * taper * ring * gulp; RR[k] = rr;
      for (var j = 0; j <= SID; j++) {
        var an = j / SID * Math.PI * 2, cs = Math.cos(an), sn = Math.sin(an), o3 = (k * (SID + 1) + j) * 3;
        // Flattened underside, like a real arm: the side the suckers sit on is broader and flatter.
        var flat = 1 - 0.18 * Math.max(0, -cs);
        tmp.copy(N).multiplyScalar(cs * flat).addScaledVector(B, sn);
        var nl = tmp.length(); nrm[o3] = tmp.x / nl; nrm[o3 + 1] = tmp.y / nl; nrm[o3 + 2] = tmp.z / nl;
        pos[o3] = P[k].x + tmp.x * rr; pos[o3 + 1] = P[k].y + tmp.y * rr; pos[o3 + 2] = P[k].z + tmp.z * rr;
      }
    }
    geo.attributes.position.needsUpdate = true; geo.attributes.normal.needsUpdate = true;
    geo.computeBoundingSphere();
    // Underside is the -N side; two rows either side of it.
    var per = NS / 2;
    for (var i = 0; i < NS; i++) {
      var row = i < per ? -1 : 1, u2 = 0.2 + 0.74 * ((i % per) + (row > 0 ? 0.5 : 0)) / per, kk = Math.round(u2 * SEG);
      var ang = Math.PI + row * 0.42, rr2 = RR[kk];
      tmp.copy(Ns[kk]).multiplyScalar(Math.cos(ang) * 0.82).addScaledVector(Bs[kk], Math.sin(ang)).normalize();
      var shown = u2 < upto * 0.98 ? 1 : 0, size = rr2 * 0.36 * shown;
      q.setFromUnitVectors(up, tmp); sc.set(size, size, size);
      m4.compose(tmp.clone().multiplyScalar(rr2 * 0.93).add(P[kk]), q, sc); cups.setMatrixAt(i, m4);
    }
    cups.instanceMatrix.needsUpdate = true;
    uni.uTime.value = time;
  } };
}

// ---------- Keeping the other crit dice in view ----------
// o.clear lists the dice to keep in view (x, z, radius on the page). The layers drawn on the page, the dim
// patch and the portal's stain, leave a soft hole round each; whatever travels out from the die (smoke,
// tendrils, the pieces) is steered round them instead. Every helper is a no-op for an empty list, so a
// grasp played alone is exactly as it was.
var GRASP_CLEAR = [
  'uniform vec3 uClr[8];',
  'float grClear(vec2 w){ float m = 1.0;',
  ' for (int i = 0; i < 8; i++) { float r = uClr[i].z; if (r > 0.0) m *= smoothstep(r * 0.6, r * 1.2, length(w - uClr[i].xy)); }',
  ' return m; }'].join('\n');
// The eight nearest to `at`, as the vec3s the shaders read (radius 0 is an unused slot).
function graspSpots(st, clear, at) {
  var T = st.T, out = [], k, near = (clear || []).slice().sort(function (a, b) { return Math.hypot(a.x - at.x, a.z - at.z) - Math.hypot(b.x - at.x, b.z - at.z); });
  for (k = 0; k < 8; k++) out.push(near[k] ? new T.Vector3(near[k].x, near[k].z, near[k].r) : new T.Vector3());
  return out;
}
// The page's darkening patch with a hole round each die to keep in view; with none it is the engine's own.
function graspDimmer(st, at, radius, clr) {
  var m = dimmer(st, at, radius);
  if (!clr) return m;
  m.material.onBeforeCompile = function (sh) {
    sh.uniforms.uClr = { value: clr };
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nvarying vec2 vHw;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvHw = (modelMatrix * vec4(transformed, 1.0)).xz;');
    sh.fragmentShader = sh.fragmentShader.replace('#include <common>', '#include <common>\nuniform vec3 uClr[8]; varying vec2 vHw;')
      .replace('#include <color_fragment>', '#include <color_fragment>\nfor (int i = 0; i < 8; i++) { float cr = uClr[i].z; if (cr > 0.0) diffuseColor.a *= smoothstep(cr * 0.6, cr * 1.2, length(vHw - uClr[i].xy)); }');
  };
  return m;
}
// An angle out of c, turned aside if the way it points runs into a die to keep in view.
function graspSteer(clear, c, a, rnd) {
  for (var pass = 0; pass < 3; pass++) {
    var moved = false;
    for (var i = 0; i < clear.length; i++) {
      var ox = clear[i].x - c.x, oz = clear[i].z - c.z, d = Math.hypot(ox, oz), r = clear[i].r * 0.8;
      if (d <= r) continue;
      var b = Math.atan2(oz, ox), half = Math.asin(Math.min(1, r / d)) + 0.08, df = Math.atan2(Math.sin(a - b), Math.cos(a - b));
      if (Math.abs(df) < half) { a = b + (df < 0 ? -1 : 1) * (half + 0.05 + rnd() * 0.12); moved = true; }
    }
    if (!moved) break;
  }
  return a;
}
// How far from c a ray at angle a may run before it reaches the space kept for a die to keep in view.
function graspReach(clear, c, a, want, floor) {
  var dx = Math.cos(a), dz = Math.sin(a), best = want;
  for (var i = 0; i < clear.length; i++) {
    var ox = clear[i].x - c.x, oz = clear[i].z - c.z, r = clear[i].r * 0.8, al = ox * dx + oz * dz, pp = ox * ox + oz * oz - al * al;
    if (al <= 0 || pp >= r * r || ox * ox + oz * oz <= r * r) continue;
    best = Math.min(best, al - Math.sqrt(r * r - pp));
  }
  return Math.max(floor, best);
}
// A random point round c, between radii r0 and r1, outside the space kept for the other dice.
function graspAround(clear, c, r0, r1, r) {
  var a, rr, x, z, k, i, ok;
  for (k = 0; k < 8; k++) {
    a = r() * 6.28; rr = r0 + r() * (r1 - r0); x = c.x + Math.cos(a) * rr; z = c.z + Math.sin(a) * rr; ok = true;
    for (i = 0; i < clear.length && ok; i++) if (Math.hypot(x - clear[i].x, z - clear[i].z) < clear[i].r * 0.9) ok = false;
    if (ok) break;
  }
  return [x, z];
}

// ---------- The floor: the stain that spreads, opens into a portal, and leaves a scorch ----------
function graspPortal(st, at, R, clr) {
  var T = st.T, uni = { uClr: { value: clr || graspSpots(st, null, at) }, uTime: { value: 0 }, uStain: { value: 0 }, uOpen: { value: 0 }, uScorch: { value: 0 }, uR: { value: R }, uVein: { value: new T.Color(GRASP_VEIN) }, uHot: { value: new T.Color(GRASP_BLOOD) } };
  var mat = new T.ShaderMaterial({ uniforms: uni, transparent: true, depthWrite: false, toneMapped: false,
    vertexShader: 'varying vec2 vP; varying vec2 vW; void main(){ vP = position.xy; vW = (modelMatrix * vec4(position, 1.0)).xz; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }',
    fragmentShader: [
      'uniform float uTime, uStain, uOpen, uScorch, uR; uniform vec3 uVein, uHot; varying vec2 vP; varying vec2 vW;', GRASP_NOISE, GRASP_CLEAR,
      'void main(){',
      ' float r = length(vP), a = atan(vP.y, vP.x);',
      ' float edgeN = fb(vec2(a * 3.0, uTime * 0.25)) * 0.5 + fb(vP * 2.2) * 0.5;',
      ' float R = uR * uStain * (0.78 + 0.45 * edgeN);',
      ' float inside = smoothstep(R, R - 0.25, r);',
      // Veins crawling out ahead of the stain.
      ' float vl = abs(fb(vP * 3.4 + vec2(0.0, uTime * 0.05)) - 0.5);',
      ' float veins = smoothstep(0.03, 0.0, vl) * smoothstep(R + 0.6, R - 0.05, r) * (1.0 - inside * 0.6) * step(0.001, uStain);',
      ' float rimGlow = exp(-pow((r - R) * 9.0, 2.0)) * step(0.001, uStain);',
      ' vec3 col = vec3(0.0); float al = inside * 0.92;',
      ' col += uVein * veins * 1.6 + mix(uVein, uHot, 0.4) * rimGlow * 1.3;',
      ' al = max(al, max(veins, rimGlow) * 0.9);',
      // The portal: a tunnel looking down, filaments spiralling inward.
      ' float P = uR * 0.82 * uOpen;',
      ' if (uOpen > 0.0 && r < P + 0.2) {',
      '  float pr = r / max(P, 0.001); float depth = 0.35 / max(pr, 0.04);',
      '  vec2 tc = vec2(a / 6.2832 * 3.0 + depth * 0.15 - uTime * 0.12, depth - uTime * 0.9);',
      '  float sw = fb(tc * vec2(4.0, 1.5)); float fil = smoothstep(0.62, 0.9, sw);',
      '  vec3 deep = mix(vec3(0.02, 0.0, 0.05), vec3(0.0), smoothstep(0.6, 0.0, pr));',
      '  vec3 pc = deep + mix(uVein, uHot, smoothstep(0.4, 1.0, pr) * 0.5) * fil * (0.4 + 2.2 * smoothstep(0.1, 1.0, pr));',
      '  float lip = exp(-pow((pr - 1.0) * 6.0, 2.0)); pc += mix(uVein, vec3(1.0, 0.85, 1.0), 0.25) * lip * 2.4;',
      '  float m = smoothstep(P + 0.18, P - 0.02, r);',
      '  col = mix(col, pc, m); al = max(al, m);',
      ' }',
      // The scorch left behind: a dark burn with embers dying in it.
      ' if (uScorch > 0.0) { float sR = uR * 0.75 * (0.85 + 0.3 * edgeN); float s = smoothstep(sR, sR * 0.4, r);',
      '  float em = smoothstep(0.7, 0.95, fb(vP * 6.0 + uTime * 0.15)) * s;',
      '  col = mix(col, vec3(0.012, 0.006, 0.004) + uHot * em * 1.6 * uScorch, s * uScorch); al = max(al, s * 0.85 * uScorch); }',
      ' gl_FragColor = vec4(col, al * grClear(vW)); }'].join('\n') });
  var m = new T.Mesh(new T.PlaneGeometry(R * 3.4, R * 3.4), mat);
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.008, at.z); m.renderOrder = 2; st.scene.add(m);
  return { mesh: m, uni: uni };
}

// ---------- The effect ----------
// The 1 burns in; cracks of red light spread over the stone while the die shivers and dark wisps rise;
// it bursts, the pieces hang in the air, and tendrils come up out of a portal in the table, each
// coiling round a piece and dragging it under. The die comes back up whole, the 1 still burning, and
// leaves a scorch that fades. Timed on e, the effect's own clock, which slows across the burst.
var GRASP_BURST = 2.9;
function fxGrasp(st, die, fi, th, o) {
  var T = st.T, hd = st.q.detail !== 'basic', s = die.size, k6 = s / 0.72, g = die.group;
  var c = g.position.clone(), qLand = g.quaternion.clone(), e = 0, own = [], sprays = {}, burst = false, reformed = false, done = false, lastP = '';
  var rnd = mulberry(91), cracks = die.faces.map(function (fc, i) { return cracksFor(fc, mulberry(300 + i)); });
  var faces = die.faces.map(function (fc, i) { return glowFace(die, i); });
  // With other crit dice to keep in view (o.clear) the darkness and the portal leave a hole round each, the
  // smoke and tendrils steer round them, and the key light is dimmed far less, since it lights them too.
  var clear = o && o.clear || [], clr = clear.length ? graspSpots(st, clear, c) : null, sunK = clear.length ? 0.12 : 0.4;
  var portal = graspPortal(st, c, 1.25 * k6, clr); own.push(portal.mesh);
  var dim = graspDimmer(st, c, 3.4 * k6, clr); own.push(dim);
  // One light under the die in HD (cracks, then the portal's violet) and the flash of the burst; Basic
  // keeps only the one.
  var under = new T.PointLight(GRASP_BLOOD, 0, 8 * k6, 2), flashL = hd ? new T.PointLight('#ffd0c0', 0, 9 * k6, 2) : null;
  st.scene.add(under); own.push(under); if (flashL) { st.scene.add(flashL); own.push(flashL); }
  var sun0 = st.sun.intensity, tends = [], NT = hd ? 7 : 5;
  for (var i = 0; i < NT; i++) {
    var td = graspTendril(st, { r: s * (0.3 + rnd() * 0.06), ph: rnd() * 1.4 });
    td.group.visible = false; st.scene.add(td.group); own.push(td.group);
    var ta = graspSteer(clear, c, i / NT * Math.PI * 2 + (rnd() - 0.5) * 0.5, rnd);
    tends.push({ td: td, a: ta, ring: (0.3 + rnd() * 0.35) * k6, L: graspReach(clear, c, ta, (2.0 + rnd() * 0.8) * k6, s * 1.4), h: (0.8 + rnd() * 0.9) * k6, ph: rnd() * 6, d: i * 0.07, piece: null });
  }
  var pieces = null, star = null;
  function hover(pc) {
    var k = out3((e - GRASP_BURST) / 0.9), p = pc.from.clone().addScaledVector(pc.dir, pc.out * k);
    p.y = Math.min(2.1 * k6, Math.max(s * 0.45, pc.from.y + pc.lift * k)) + Math.sin(e * 1.7 + pc.ph) * 0.06 * k * k6;
    return p;
  }
  function doBurst() {
    burst = true; g.position.copy(c);
    var parts = splitDie(st, die);
    pieces = parts.map(function (p, i) {
      var r = mulberry(500 + i), dv = new T.Vector3(p.dir.x, 0, p.dir.z).normalize();
      return { fc: p.fc, m: p.m, from: p.m.position.clone(), dir: dv, out: graspReach(clear, c, Math.atan2(dv.z, dv.x), s * (1.4 + r() * 1.3), s * 0.7), lift: s * (0.5 + r() * 1.0) + Math.max(0, p.dir.y) * s * 0.8,
        ax: new T.Vector3(r() - 0.5, r() - 0.5, r() - 0.5).normalize(), spin: 0.8 + r() * 2.2, ph: r() * 6, by: null, tv: 4.7 + r() * 0.5 };
    });
    tends.forEach(function (td) {
      var best = null, bd = -2, d = new T.Vector3(Math.cos(td.a), 0, Math.sin(td.a));
      pieces.forEach(function (pc) { if (pc.by) return; var dd = pc.dir.dot(d) + pc.lift * 0.1; if (dd > bd) { bd = dd; best = pc; } });
      if (best) { best.by = td; td.piece = best; }
    });
    var cc = [c.x, c.y, c.z];
    st.fx.push(spray(st, { n: hd ? 160 : 70, colors: [GRASP_BLOOD, GRASP_VEIN, '#ffb48a', '#ffffff'], size: s * 0.08, life: [0.5, 1.5], drag: 1.2, g: 6,
      spawn: function (i, r) { var a = r() * 6.28, sp = (2 + r() * 7) * k6; return { p: cc, v: [Math.cos(a) * sp, 1 + r() * 5, Math.sin(a) * sp] }; } }));
    st.fx.push(spray(st, { n: hd ? 40 : 18, colors: ['#3b2c4a', '#2a1f36', '#4a3550'], size: s * 1.6, life: [1.4, 2.6], drag: 1.4, g: -0.5, blend: 'normal', smoke: true, alpha: 0.55,
      spawn: function (i, r) { var a = graspSteer(clear, c, r() * 6.28, r), sp = (0.8 + r() * 2.4) * k6; return { p: cc, v: [Math.cos(a) * sp, 0.3 + r() * 0.9, Math.sin(a) * sp] }; } }));
    st.fx.push(wave(st, c, GRASP_BLOOD, s * 0.6, s * 5, 0.7));
    st.fx.push(wave(st, c, GRASP_VEIN, s * 0.3, s * 3.4, 1.0));
    star = glow(st, starTex(T), '#ffd6cc', s * 5); star.position.copy(c).setY(c.y + 0.2); st.scene.add(star); own.push(star);
  }
  function reform() {
    reformed = true; wholeDie(die);
    faces.forEach(function (f) { f.clear(); f.commit(); });
  }
  // The cracks (and the burning 1) on every face's light layer, redrawn only when they change.
  function paintCracks(p, fade) {
    var key = Math.round(p * 40) + '|' + Math.round(fade * 24);
    if (key === lastP) return; lastP = key;
    die.faces.forEach(function (fc, i) {
      var f = faces[i]; f.clear();
      if (p > 0 && fade > 0) { f.g.globalAlpha = fade; glowStrokes(f.g, cutPaths(cracks[i], p), 3.2 * fc.lay.cv / 256, i % 3 ? GRASP_VEIN : GRASP_BLOOD); f.g.globalAlpha = 1; }
      if (i === fi) glowStrokes(f.g, fc.lay.strokes, strokeW(fc.lay), GRASP_BLOOD);
      f.commit();
    });
  }
  function slowmo() { return 1 - 0.7 * env4(e, GRASP_BURST - 0.05, GRASP_BURST + 0.05, GRASP_BURST + 0.45, GRASP_BURST + 0.9); }
  function seq(dt) {
    var fc1 = die.faces[fi], f1 = faces[fi];
    if (e < 1.0) { drawBurn(f1.g, f1.lay, clamp01((e - 0.15) / 0.75), GRASP_BLOOD); f1.commit(1.6); }
    if (e >= 1.0 && e < GRASP_BURST) {
      var p = smooth(1.0, 2.6, e); paintCracks(p, 1);
      die.faces.forEach(function (fc) { fc.mat.emissiveIntensity = 1.3 + p * 2 + (p > 0.6 ? Math.sin(e * 51) * 0.5 : 0); });
      tintDie(die, GRASP_BLOOD, p * 0.9);
      var j = s * 0.05 * smooth(1.8, 2.8, e);
      g.position.set(c.x + (Math.random() - 0.5) * j, c.y + (Math.random() - 0.5) * j * 0.4, c.z + (Math.random() - 0.5) * j);
      if (!sprays.wisps && e > 1.5) { sprays.wisps = 1;
        st.fx.push(spray(st, { n: hd ? 46 : 20, colors: ['#4a3a5c', '#3a2d48'], size: s * 1.1, life: [1.6, 2.6], drag: 0.4, g: -0.35, blend: 'normal', smoke: true, alpha: 0.4,
          spawn: function (i, r) { var w = graspAround(clear, c, 0.3 * k6, 1.2 * k6, r); return { p: [w[0], 0.05, w[1]], v: [(r() - 0.5) * 0.2, 0.25 + r() * 0.4, (r() - 0.5) * 0.2], delay: r() * 1.2 }; } })); }
    }
    if (e >= GRASP_BURST && !burst) doBurst();
    var U = portal.uni; U.uTime.value = e;
    U.uStain.value = smooth(1.2, 2.9, e) * (1 - smooth(7.6, 8.4, e));
    U.uOpen.value = e < GRASP_BURST ? 0 : out3((e - GRASP_BURST) / 0.55) * (1 - smooth(7.0, 7.9, e));
    U.uScorch.value = smooth(7.5, 8.5, e) * (1 - smooth(9.0, 9.9, e));
    dim.material.opacity = 0.55 * smooth(1.2, 2.6, e) * (1 - smooth(8.2, 9.6, e));
    under.position.copy(c).setY(burst ? 0.45 : c.y + 0.25);
    under.color.set(e < GRASP_BURST ? GRASP_BLOOD : GRASP_VEIN);
    under.intensity = e < GRASP_BURST ? 9 * smooth(1.0, 2.7, e) * (0.75 + Math.random() * 0.5) : 16 * U.uOpen.value * (0.85 + 0.15 * Math.sin(e * 13));
    if (flashL) { flashL.position.copy(c).setY(c.y + 0.6); flashL.intensity = e >= GRASP_BURST ? 70 * Math.exp(-(e - GRASP_BURST) * 9) : 0; }
    else if (e >= GRASP_BURST && e < GRASP_BURST + 0.4) under.intensity += 40 * Math.exp(-(e - GRASP_BURST) * 9);
    st.sun.intensity = sun0 * (1 - sunK * smooth(1.8, 3.0, e) * (1 - smooth(7.6, 8.8, e)));
    if (star) { var sk = e - GRASP_BURST; star.material.opacity = sk < 0.05 ? sk / 0.05 : Math.exp(-(sk - 0.05) * 5); var ss = s * (4 + sk * 6); star.scale.set(ss, ss, 1); }
    if (burst) tends.forEach(function (td) {
      var e2 = e - td.d, grow = smooth(GRASP_BURST + 0.15, GRASP_BURST + 0.95, e2), retract = smooth(6.1, 6.9, e2);
      td.td.group.visible = grow > 0.005 && retract < 0.995;
      if (!td.td.group.visible) return;
      var dir = new T.Vector3(Math.cos(td.a), 0, Math.sin(td.a)), root = c.clone().addScaledVector(dir, td.ring).setY((-1.4 - 1.2 * retract) * k6);
      var emerge = c.clone().addScaledVector(dir, td.ring * 1.1).setY(0.05 - 1.1 * retract * k6);
      var free = c.clone().addScaledVector(dir, td.L * (0.6 + 0.4 * grow)).setY(td.h * grow + 0.3);
      var tip = free, pc = td.piece, coil = null;
      if (pc) {
        var rk = inout((e2 - 3.45) / 0.75), hp = pc.m.position;
        tip = free.clone().lerp(hp, rk);
        if (rk >= 1) { var ca = e * 7 + td.ph; coil = [0, 1, 2].map(function (q) { var an = ca + q * 2.1; return hp.clone().add(new T.Vector3(Math.cos(an) * s * 0.42, (q - 1) * s * 0.18, Math.sin(an) * s * 0.42)); }); }
      }
      var ctrl = [root, emerge].concat(writhe(T, emerge, tip, 6, 0.16, s * 0.9 * (1 - retract), e2, td.ph).slice(1));
      if (coil) ctrl = ctrl.concat(coil);
      td.td.update(ctrl, Math.max(0.002, grow * (1 - retract)), 0.55 + 0.45 * grow, e2);
      // Brighter veins than in the showcase: over a lit page, not a dark table, they need it to read.
      td.td.uni.uGlow.value = 1.1 + 0.45 * Math.sin(e * 3 + td.ph) + 0.5 * smooth(4.9, 5.3, e2);
    });
    if (burst && !reformed) pieces.forEach(function (pc) {
      var hp = hover(pc), dq = new T.Quaternion();
      if (pc.by) pc.m.position.copy(hp.lerp(c.clone().setY(-0.9 * k6), clamp01(in3((e - 4.95 - pc.by.d) / 1.25))));
      else {
        var vk = in3((e - pc.tv) / 1.5), rel = hp.clone().sub(c), r0 = Math.hypot(rel.x, rel.z), a0 = Math.atan2(rel.z, rel.x) + vk * 5;
        pc.m.position.set(c.x + Math.cos(a0) * r0 * (1 - vk), hp.y * (1 - vk) - vk * 0.9 * k6, c.z + Math.sin(a0) * r0 * (1 - vk));
      }
      pc.m.quaternion.premultiply(dq.setFromAxisAngle(pc.ax, pc.spin * dt * (1 + (e > 4.9 ? 2 : 0))));
      pc.fc.mat.emissiveIntensity = 1.5 + Math.sin(e * 9 + pc.ph) * 0.4;
      // Pieces below the table are hidden: the page has no hole for them to sink into.
      pc.m.visible = pc.m.position.y > -s * 0.4;
    });
    if (burst && !sprays.motes && e > 3.3) { sprays.motes = 1;
      st.fx.push(spray(st, { n: hd ? 120 : 50, colors: [GRASP_VEIN, '#e0b8ff', GRASP_BLOOD], size: s * 0.06, life: [1.2, 2.2], drag: 0.3, g: 0, noFloor: true, pull: { x: c.x, y: -0.3, z: c.z, k: 1.6 }, swirl: { x: c.x, z: c.z, k: 2.4 },
        spawn: function (i, r) { var w = graspAround(clear, c, 1.2 * k6, 3.0 * k6, r); return { p: [w[0], 0.2 + r() * 2.0, w[1]], v: [0, 0, 0], delay: r() * 2.6 }; } })); }
    if (e >= 6.75 && !reformed) reform();
    if (reformed) {
      var k2 = out3((e - 6.75) / 1.3);
      g.position.set(c.x, c.y - 2.0 * k6 * (1 - k2), c.z);
      g.quaternion.copy(qLand).premultiply(new T.Quaternion().setFromAxisAngle(new T.Vector3(0, 1, 0), (1 - k2) * 4));
      var fade = 1 - smooth(7.8, 9.2, e); paintCracks(1, fade);
      die.faces.forEach(function (fc, i) { fc.mat.emissiveIntensity = i === fi ? 1.8 : 1.2 * fade + 0.0001; });
      tintDie(die, GRASP_BLOOD, 0.9 * fade);
      if (!sprays.embers && e > 7.6) { sprays.embers = 1;
        st.fx.push(spray(st, { n: hd ? 60 : 26, colors: [GRASP_BLOOD, '#ff8a3a', '#ffcf8a'], size: s * 0.05, life: [1.2, 2.6], drag: 0.5, g: -0.6,
          spawn: function (i, r) { var a = r() * 6.28, rr = r() * 0.9 * k6; return { p: [c.x + Math.cos(a) * rr, 0.04, c.z + Math.sin(a) * rr], v: [(r() - 0.5) * 0.3, 0.3 + r() * 0.6, (r() - 0.5) * 0.3], delay: r() * 1.5 }; } })); }
    }
  }
  return { update: function (dt) {
    if (done) return false;
    var d2 = dt * 1.15 * slowmo(); e += d2; seq(d2);
    if (e > 9.9) {
      // The end: whole, exactly where it landed, the 1 lit red, everything else gone.
      done = true; g.position.copy(c); g.quaternion.copy(qLand); st.sun.intensity = sun0; tintDie(die, GRASP_BLOOD, 0);
      die.faces.forEach(function (fc, i) { if (i !== fi) fc.mat.emissiveIntensity = 0; });
      drop(st, own);
      return false;
    }
    return true;
  } };
}
// ---------- The echo ----------
// The same tendril on another die. It comes up out of a portal (the grand's, so it reaches across the page
// to this die, or a small one of its own beside the die when played alone), winds round the die and sinks
// back, and the number burns red where it was. No lights: the tendril is a lit surface, and the die's own
// tint and the red numeral carry the glow, so it costs no recompile when it starts.
function fxGraspEcho(st, die, fi, th, e) {
  var T = st.T, hd = st.q.detail !== 'basic', g = die.group, s = die.size, k6 = s / 0.72, t = 0, END = 1.95, i, j;
  var rnd = mulberry(6007 + Math.floor(Math.random() * 1e6)), own = [];
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), y0 = p0.y, c = new T.Vector3(p0.x, 0, p0.z), FROM = !!(e && e.from);

  // Where the tendril comes up: on the grand's side of the page a little way out from its portal's centre
  // (inside it once it has opened), or beside this die. The wind round the die starts facing that point.
  var E0 = new T.Vector3(), ang = rnd() * 6.283, sgn = rnd() < 0.5 ? -1 : 1;
  if (FROM) {
    var gp = e.from.group.position, dx = c.x - gp.x, dz = c.z - gp.z, dl = Math.hypot(dx, dz) || 1, off = Math.min(1.0 * k6, dl * 0.35);
    E0.set(gp.x + dx / dl * off, 0, gp.z + dz / dl * off);
  } else E0.set(c.x + Math.cos(ang) * s * 1.75, 0, c.z + Math.sin(ang) * s * 1.75);
  var aE = Math.atan2(E0.z - c.z, E0.x - c.x), Rc = s * 1.08;

  var portal = graspPortal(st, E0, 0.5 * k6, null); portal.mesh.renderOrder = 60; own.push(portal.mesh);
  var dim = graspDimmer(st, c, 3.2 * s, null); own.push(dim);
  var td = graspTendril(st, { r: s * 0.3, ph: rnd() * 1.4 }); td.group.visible = false; st.scene.add(td.group); own.push(td.group);

  // The path: out of the ground, across the page, then round the die from low to high. Points are reused.
  var NR = 10, NC = 8, ctrl = [], pt;
  for (i = 0; i < 2 + NR + NC; i++) ctrl.push(new T.Vector3());
  var CS = new T.Vector3(), side = new T.Vector3(), PH = rnd() * 6;
  function path(tt, retract) {
    ctrl[0].set(E0.x, (-1.4 - 1.2 * retract) * k6, E0.z); ctrl[1].set(E0.x, 0.05 - 0.9 * retract * k6, E0.z);
    CS.set(c.x + Math.cos(aE) * Rc, y0 * 0.5, c.z + Math.sin(aE) * Rc);
    var rx = CS.x - E0.x, rz = CS.z - E0.z, L = Math.hypot(rx, rz) || 1, amp = 0.09 * (1 - 0.7 * retract);
    side.set(-rz / L, 0, rx / L);
    for (i = 0; i < NR; i++) {
      var u = i / (NR - 1), w = Math.sin(u * Math.PI * 1.6 - tt * 3.2 + PH) * amp * L * Math.sin(u * Math.PI), lift = Math.sin(u * Math.PI) * s * 0.8;
      ctrl[2 + i].set(E0.x + rx * u + side.x * w, 0.05 + (CS.y - 0.05) * u + lift, E0.z + rz * u + side.z * w);
    }
    for (j = 0; j < NC; j++) {
      var k = j / (NC - 1), an = aE + sgn * (j * 0.85 + Math.sin(tt * 3 + j) * 0.04), rr = Rc * (1 - 0.1 * k) * (1 + 0.03 * Math.sin(tt * 9 + j));
      ctrl[2 + NR + j].set(c.x + Math.cos(an) * rr, y0 * (0.5 + 0.7 * k), c.z + Math.sin(an) * rr);
    }
  }
  function chord(a, b) { var sum = 0; for (var q = a + 1; q < b; q++) sum += ctrl[q].distanceTo(ctrl[q - 1]); return sum; }

  // The number burns red as the tendril closes in and stays lit.
  var gf = glowFace(die, fi), lastP = -1;
  function burn(p, k) { var key = Math.round(p * 24); if (key !== lastP) { lastP = key; drawBurn(gf.g, gf.lay, p, GRASP_BLOOD); } gf.commit(k); }
  var puffed = false;
  return { update: function (dt) {
    t += dt;
    var reachK = smooth(FROM ? 0.2 : 0.24, FROM ? 0.74 : 0.62, t), coilK = smooth(FROM ? 0.7 : 0.58, FROM ? 1.28 : 1.16, t), retract = smooth(1.4, 1.88, t);
    path(t, retract);
    var Lr = chord(1, 2 + NR) + 0.001, Lc = chord(1 + NR, 2 + NR + NC), f1 = Lr / (Lr + Lc), upto = (f1 * reachK + (1 - f1) * coilK) * (1 - retract);
    td.group.visible = upto > 0.004;
    if (td.group.visible) {
      td.update(ctrl, Math.max(0.002, upto), 0.55 + 0.4 * reachK, t);
      td.uni.uGlow.value = 1.1 + 0.45 * Math.sin(t * 9) + 0.5 * smooth(0.8, 1.2, t);
    }
    // The mouth it comes through: opens fast, closes behind it.
    var U = portal.uni; U.uTime.value = t * 1.2;
    U.uStain.value = smooth(0, 0.3, t) * (1 - smooth(1.7, 1.94, t));
    U.uOpen.value = out3((t - 0.06) / 0.4) * (1 - smooth(1.55, 1.85, t));
    dim.material.opacity = 0.4 * smooth(0, 0.5, t) * (1 - smooth(1.5, 1.94, t));
    // The grip: the die shivers a little while it is wound, and glows red through its edges.
    var grip = smooth(0.85, 1.15, t) * (1 - smooth(1.3, 1.5, t)), jit = s * 0.018 * grip;
    g.position.set(p0.x + (rnd() - 0.5) * jit, p0.y, p0.z + (rnd() - 0.5) * jit);
    tintDie(die, GRASP_BLOOD, 0.5 * smooth(0.3, 0.9, t) * (1 - smooth(1.5, 1.9, t)));
    burn(smooth(0.3, 0.95, t), 1.4 + 0.4 * Math.sin(t * 11) * grip + 0.4 * smooth(0.3, 0.95, t));
    if (!puffed && t >= 0.85) { puffed = true;
      st.fx.push(spray(st, { n: hd ? 22 : 10, colors: [GRASP_BLOOD, '#ff8a3a', GRASP_VEIN], size: s * 0.05, life: [0.4, 0.7], drag: 0.6, g: -0.6,
        spawn: function (k, r) { var a = r() * 6.28, rr = r() * s * 1.0; return { p: [c.x + Math.cos(a) * rr, 0.1 + r() * y0 * 1.6, c.z + Math.sin(a) * rr], v: [(r() - 0.5) * 0.6, 0.4 + r() * 0.8, (r() - 0.5) * 0.6], delay: r() * 0.25 }; } })); }
    if (t >= END) {
      g.position.copy(p0); g.quaternion.copy(q0); tintDie(die, GRASP_BLOOD, 0);
      drawBurn(gf.g, gf.lay, 1, GRASP_BLOOD); gf.commit(1.8);
      td.group.children.forEach(function (m) { if (m.isInstancedMesh) m.dispose(); });
      drop(st, own);
      return false;
    }
    return true;
  } };
}
// beat: the die bursts and the portal starts to open (seconds of real time, the slow-motion included);
// peak: the pieces have been dragged under and the tendrils are drawing back.
registerCrit('grasp', { name: 'Eldritch grasp', kind: 'fail', what: 'Red cracks spread over the die and it bursts. Tendrils rise out of a portal in the page, each drags a piece under, and the die comes back up whole with the 1 burning.', fx: fxGrasp,
  beat: 2.55, peak: 6.6, echo: fxGraspEcho, echoWhat: 'A tendril reaches across the page to each other 1, curls round the die and sinks back.' });
