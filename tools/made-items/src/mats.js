// Materials by what they are. The recipe gives each one its colour; the kind
// of stuff decides how it takes light. A metal has no colour of its own and
// tints what it mirrors; wood, hide, cloth and bone keep their colour and only
// catch a glint; clear things are drawn as a far wall, a tint and a shine.
import * as THREE from 'three';
import { makeNoise, fbm, tex, rng } from './util.js';

let cache = null;
function maps() {
  if (cache) return cache;
  const n1 = makeNoise(11, 64), n2 = makeNoise(23, 64), n3 = makeNoise(37, 64);
  const g = v => Math.max(0, Math.min(1, v));
  cache = {
    brushedRough: tex(512, 512, (u, v) => .42 + (fbm(n1, u * 220, v * 3, 3) - .5) * .1 + (fbm(n2, u * 6, v * 6, 4) - .5) * .2 + Math.pow(n3(u * 64, v * 64), 14) * 1.5),
    brushedBump: tex(512, 512, (u, v) => .5 + (fbm(n1, u * 220, v * 3, 3) - .5) * .25 - Math.pow(n3(u * 64, v * 64), 14) * 2),
    steelColour: tex(256, 256, (u, v) => { const s = Math.max(0, fbm(n2, u * 5, v * 5, 4) - .56) * 1.6; return [1 - s * .3, 1 - s * .36, 1 - s * .46]; }, { srgb: true }),
    castRough: tex(512, 512, (u, v) => .3 + (fbm(n2, u * 9, v * 9, 4) - .5) * .7 + (fbm(n1, u * 90, v * 90, 2) - .5) * .25, {
      draw(ctx, w, h) { const r = rng(5); ctx.globalAlpha = .35; ctx.lineWidth = .6; for (let i = 0; i < 260; i++) { const x = r() * w, y = r() * h, a = r() * Math.PI, l = 8 + r() * 40; ctx.strokeStyle = r() > .5 ? '#fff' : '#555'; ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x + Math.cos(a) * l, y + Math.sin(a) * l); ctx.stroke(); } }
    }),
    castBump: tex(256, 256, (u, v) => .5 + (fbm(n3, u * 40, v * 40, 3) - .5) * .6),
    castColour: tex(256, 256, (u, v) => { const s = Math.max(0, fbm(n1, u * 7, v * 7, 4) - .5) * 1.6; return [1 - s * .32, 1 - s * .4, 1 - s * .55]; }, { srgb: true }),
    // Hammered iron: dents and scale left by the forge.
    forgedBump: tex(256, 256, (u, v) => .5 + (fbm(n2, u * 14, v * 14, 3) - .5) * 1.1 + (fbm(n3, u * 50, v * 50, 2) - .5) * .3),
    forgedRough: tex(256, 256, (u, v) => .55 + (fbm(n1, u * 12, v * 12, 4) - .5) * .7),
    hideBump: tex(512, 512, (u, v) => .5 + (fbm(n1, u * 60, v * 60, 3) - .5) * .9 + (fbm(n2, u * 14, v * 14, 3) - .5) * .4),
    hideRough: tex(256, 256, (u, v) => .62 + (fbm(n2, u * 10, v * 10, 4) - .5) * .5),
    hideColour: tex(256, 256, (u, v) => { const c = .82 + (fbm(n3, u * 6, v * 6, 4) - .5) * .45; return g(c); }, { srgb: true }),
    // Wood: long grain streaks, growth rings drifting across, a few darker pores.
    woodColour: tex(512, 512, (u, v) => { const w = fbm(n2, u * 3, v * 1.2, 3), ringv = Math.sin((u * 26 + w * 9)) * .5 + .5, st = fbm(n1, u * 120, v * 4, 3); return g(.62 + ringv * .22 + (st - .5) * .34); }, { srgb: true }),
    woodBump: tex(512, 512, (u, v) => .5 + (fbm(n1, u * 120, v * 4, 3) - .5) * .7 + (Math.sin(u * 26 + fbm(n2, u * 3, v * 1.2, 3) * 9) * .12)),
    woodRough: tex(256, 256, (u, v) => .6 + (fbm(n1, u * 60, v * 3, 3) - .5) * .4),
    // Cloth: a plain weave seen as fine crossing ridges, with slubs.
    clothBump: tex(512, 512, (u, v) => { const a = Math.sin(u * 420) * .5 + .5, b = Math.sin(v * 420) * .5 + .5; return .5 + (a * b - .25) * .9 + (fbm(n2, u * 30, v * 30, 2) - .5) * .5; }),
    clothColour: tex(256, 256, (u, v) => g(.84 + (fbm(n1, u * 8, v * 8, 4) - .5) * .32), { srgb: true }),
    // Stone and bone: clouding, pits and hairline cracks.
    rockBump: tex(512, 512, (u, v) => .5 + (fbm(n1, u * 10, v * 10, 5) - .5) * 1.2 - Math.pow(n3(u * 48, v * 48), 9) * 1.2),
    rockColour: tex(256, 256, (u, v) => g(.78 + (fbm(n2, u * 6, v * 6, 5) - .5) * .5 - Math.pow(n3(u * 40, v * 40), 8) * .3), { srgb: true }),
    boneColour: tex(512, 512, (u, v) => { const cl = fbm(n1, u * 8, v * 14, 4), band = (Math.sin(v * 150 + cl * 8) * .5 + .5) * .05; const s = Math.max(0, cl - .5) * 1.2; return [g(.95 - s * .18 - band), g(.93 - s * .26 - band), g(.9 - s * .36 - band * 1.2)]; }, { srgb: true }),
    boneBump: tex(512, 512, (u, v) => .55 + (fbm(n1, u * 50, v * 8, 3) - .5) * .3 + (fbm(n2, u * 9, v * 9, 3) - .5) * .25),
    // Baked crust: blisters and flour, darker where it rose toward the heat.
    crustColour: tex(256, 256, (u, v) => { const c = fbm(n1, u * 9, v * 9, 4), f = Math.pow(fbm(n2, u * 30, v * 30, 2), 3) * 1.4; return [g(.8 + (c - .5) * .5 + f * .3), g(.74 + (c - .5) * .6 + f * .34), g(.66 + (c - .5) * .7 + f * .4)]; }, { srgb: true }),
    crustBump: tex(256, 256, (u, v) => .5 + (fbm(n1, u * 18, v * 18, 4) - .5) * 1.3),
    paperColour: tex(256, 256, (u, v) => { const c = fbm(n2, u * 5, v * 5, 4); return [g(.96 - (c - .4) * .2), g(.94 - (c - .4) * .26), g(.88 - (c - .4) * .4)]; }, { srgb: true }),
    fine: tex(256, 256, (u, v) => .5 + (fbm(n3, u * 80, v * 80, 2) - .5) * .6),
  };
  return cache;
}
const C = hex => new THREE.Color(hex);
function tag(mat, label, extra) { mat.userData.label = label || ''; if (extra) Object.assign(mat.userData, extra); return mat; }

// kind: steel (ground and brushed), cast (poured and filed), forged (hammered, dark).
export function metal(hex, o) {
  o = o || {}; const t = maps(), kind = o.kind || 'steel', c = C(hex);
  const r = o.rough == null ? (kind === 'steel' ? .3 : kind === 'cast' ? .28 : .55) : o.rough;
  const m = new THREE.MeshPhysicalMaterial({ color: c, metalness: 1, roughness: Math.min(1, r / .5),
    roughnessMap: kind === 'steel' ? t.brushedRough : kind === 'cast' ? t.castRough : t.forgedRough,
    bumpMap: kind === 'steel' ? t.brushedBump : kind === 'cast' ? t.castBump : t.forgedBump, bumpScale: o.bump == null ? (kind === 'steel' ? .12 : kind === 'cast' ? .5 : .6) : o.bump,
    map: kind === 'steel' ? t.steelColour : t.castColour, side: o.double ? THREE.DoubleSide : THREE.FrontSide, flatShading: !!o.flat });
  return tag(m, o.label);
}
export function wood(hex, o) {
  o = o || {}; const t = maps();
  return tag(new THREE.MeshPhysicalMaterial({ color: C(hex).multiplyScalar(1.25), map: t.woodColour, metalness: 0, roughness: 1, roughnessMap: t.woodRough, bumpMap: t.woodBump, bumpScale: o.bump == null ? .7 : o.bump, clearcoat: o.polish || .12, clearcoatRoughness: .4 }), o.label);
}
export function leather(hex, o) {
  o = o || {}; const t = maps(), c = C(hex);
  return tag(new THREE.MeshPhysicalMaterial({ color: c.clone().multiplyScalar(1.15), map: t.hideColour, metalness: 0, roughness: 1, roughnessMap: t.hideRough, bumpMap: t.hideBump, bumpScale: o.bump == null ? 1.1 : o.bump, sheen: .3, sheenRoughness: .5, sheenColor: c.clone().lerp(C('#c9a184'), .6), side: o.double ? THREE.DoubleSide : THREE.FrontSide }), o.label);
}
export function cloth(hex, o) {
  o = o || {}; const t = maps(), c = C(hex);
  return tag(new THREE.MeshPhysicalMaterial({ color: c.clone().multiplyScalar(1.12), map: t.clothColour, metalness: 0, roughness: .95, bumpMap: t.clothBump, bumpScale: o.bump == null ? .5 : o.bump, sheen: o.sheen == null ? .8 : o.sheen, sheenRoughness: .45, sheenColor: c.clone().lerp(C('#ffffff'), .45), side: THREE.DoubleSide }), o.label);
}
export function bone(hex, o) {
  o = o || {}; const t = maps();
  return tag(new THREE.MeshPhysicalMaterial({ color: C(hex), map: t.boneColour, metalness: 0, roughness: o.rough == null ? .45 : o.rough, bumpMap: t.boneBump, bumpScale: .9, clearcoat: o.gloss == null ? .35 : o.gloss, clearcoatRoughness: .25, sheen: .3, sheenColor: 0xffe2b8, sheenRoughness: .6, side: o.double ? THREE.DoubleSide : THREE.FrontSide }), o.label);
}
export function rock(hex, o) {
  o = o || {}; const t = maps();
  return tag(new THREE.MeshPhysicalMaterial({ color: C(hex).multiplyScalar(1.2), map: t.rockColour, metalness: 0, roughness: o.rough == null ? .85 : o.rough, bumpMap: t.rockBump, bumpScale: o.bump == null ? 1.6 : o.bump, flatShading: !!o.flat }), o.label);
}
export function wax(hex, o) {
  o = o || {}; const c = C(hex), t = maps();
  return tag(new THREE.MeshPhysicalMaterial({ color: c, metalness: 0, roughness: .42, bumpMap: t.fine, bumpScale: .15, clearcoat: .3, clearcoatRoughness: .35, emissive: c, emissiveIntensity: o.lit == null ? .06 : o.lit }), o.label);
}
export function paper(hex, o) {
  o = o || {}; const t = maps();
  return tag(new THREE.MeshPhysicalMaterial({ color: C(hex || '#e9dfc6'), map: o.map || t.paperColour, metalness: 0, roughness: .9, bumpMap: t.fine, bumpScale: .25, side: THREE.DoubleSide }), o.label);
}
// Plain matte stuff: food, clay, cord. grain picks the surface relief.
export function matte(hex, o) {
  o = o || {}; const t = maps(), crust = o.grain === 'crust';
  return tag(new THREE.MeshPhysicalMaterial({ color: C(hex).multiplyScalar(crust ? 1.2 : 1), map: crust ? t.crustColour : o.grain === 'rock' ? t.rockColour : null, metalness: 0, roughness: o.rough == null ? .8 : o.rough, bumpMap: crust ? t.crustBump : o.grain === 'rock' ? t.rockBump : t.fine, bumpScale: o.bump == null ? (crust ? 1.2 : .3) : o.bump, clearcoat: o.wet || 0, clearcoatRoughness: .15, sheen: o.sheen || 0, sheenColor: C(hex).lerp(C('#ffffff'), .5), side: o.double ? THREE.DoubleSide : THREE.FrontSide, flatShading: !!o.flat }), o.label);
}
// A cut or grown crystal. The far faces are drawn from inside, mirroring the
// room in the stone's colour; the near faces add the tint and the white glints.
export function gem(hex, o) {
  o = o || {}; const c = C(hex), deep = c.clone().multiplyScalar(.5), vc = !!o.vc, W = C('#ffffff');
  const back = new THREE.MeshPhysicalMaterial({ color: vc ? W : c.clone().lerp(W, o.pale || .12), vertexColors: vc, metalness: 1, roughness: o.frost || .08, flatShading: !o.round, envMapIntensity: o.fire == null ? 3.2 : o.fire, side: THREE.BackSide, emissive: c, emissiveIntensity: o.lit == null ? .1 : o.lit });
  const tint = new THREE.MeshPhysicalMaterial({ color: vc ? W.clone().multiplyScalar(.6) : deep, vertexColors: vc, metalness: 0, roughness: .05, flatShading: !o.round, transparent: true, opacity: o.body == null ? .3 : o.body, depthWrite: false, envMapIntensity: .6 });
  const shine = new THREE.MeshPhysicalMaterial({ color: C('#ffffff').multiplyScalar(o.mirror == null ? .2 : o.mirror), metalness: 1, roughness: o.frost ? .12 : .02, flatShading: !o.round, transparent: true, blending: THREE.AdditiveBlending, depthWrite: false, envMapIntensity: 1.5 });
  return tag(back, o.label, { layers: [{ mat: tint, order: 1 }, { mat: shine, order: 2 }], edges: o.edges == null ? .5 : o.edges, inner: { hex: hex, power: o.inner == null ? .9 : o.inner } });
}
// Blown glass: nearly nothing straight on, a tint that deepens at the edges, hard glints.
export function glass(hex, o) {
  o = o || {}; const c = C(hex), k = o.tint == null ? .22 : o.tint;
  const back = new THREE.MeshPhysicalMaterial({ color: c, metalness: 0, roughness: .06, transparent: true, opacity: k, depthWrite: false, side: THREE.BackSide, envMapIntensity: 1.2 });
  const tint = new THREE.MeshPhysicalMaterial({ color: c, metalness: 0, roughness: .05, transparent: true, opacity: k * .8, depthWrite: false, envMapIntensity: .5 });
  const shine = new THREE.MeshPhysicalMaterial({ color: 0x000000, metalness: 0, roughness: .03, transparent: true, blending: THREE.AdditiveBlending, depthWrite: false, clearcoat: 1, clearcoatRoughness: .03, envMapIntensity: 1.5, ior: 1.52 });
  return tag(back, o.label, { layers: [{ mat: tint, order: 3 }, { mat: shine, order: 4 }], noshadow: true });
}
// What a vessel holds: dark in its depth, lit where light comes through it.
export function liquid(hex, o) {
  o = o || {}; const c = C(hex);
  return tag(new THREE.MeshPhysicalMaterial({ color: c.clone().multiplyScalar(.07), metalness: 0, roughness: .06, clearcoat: 1, clearcoatRoughness: .04, emissive: c, emissiveIntensity: o.lit == null ? .1 : o.lit, envMapIntensity: .8 }), o.label, { inner: { hex: hex, power: o.inner == null ? 1.5 : o.inner } });
}
// Things that give off light. power above 1 burns toward white.
export function glow(hex, power, o) {
  o = o || {};
  const m = new THREE.MeshBasicMaterial({ color: C(hex).multiplyScalar(power == null ? 3 : power), vertexColors: !!o.vc, transparent: !!o.add || o.opacity != null, opacity: o.opacity == null ? 1 : o.opacity, blending: o.add ? THREE.AdditiveBlending : THREE.NormalBlending, depthWrite: !o.add, side: o.double || o.add ? THREE.DoubleSide : THREE.FrontSide, fog: false });
  return tag(m, o.label, { glow: true, noshadow: true });
}
// Things only half here: seen as light with nothing solid behind it.
export function ghost(hex, opacity, o) {
  o = o || {};
  const m = new THREE.MeshBasicMaterial({ color: C(hex).multiplyScalar(o.power || 1), vertexColors: !!o.vc, transparent: true, opacity: opacity == null ? .2 : opacity, blending: THREE.AdditiveBlending, depthWrite: false, side: THREE.DoubleSide });
  return tag(m, o.label, { glow: true, noshadow: true });
}
export function dark() { return tag(new THREE.MeshStandardMaterial({ color: 0x0c0706, roughness: .95, metalness: 0, side: THREE.DoubleSide }), ''); }

// A whole tooth or tusk: yellowed and rough at the root, glossy toward the point,
// with hairline cracks and growth bands. Its colour is its own, so it takes no recipe colour.
export function ivory(seed, o) {
  o = o || {}; const na = makeNoise(seed || 51, 64), nb = makeNoise((seed || 51) + 9, 64), r = rng(seed || 51), cracks = [];
  for (let i = 0; i < 26; i++) cracks.push({ u: r(), v0: r() * .7, len: .08 + r() * .35, w: .0025 + r() * .003, wob: r() * 6 });
  function crack(u, v) { let k = 0; for (const c of cracks) { if (v < c.v0 || v > c.v0 + c.len) continue; let du = Math.abs(u - (c.u + Math.sin(v * 40 + c.wob) * .006)); du = Math.min(du, 1 - du); if (du < c.w) k = Math.max(k, (1 - du / c.w) * Math.sin((v - c.v0) / c.len * Math.PI)); } return k; }
  const stain = o.stain == null ? 1 : o.stain;
  const colour = tex(512, 1024, (u, v) => {
    const cl = fbm(na, u * 8, v * 14, 4), root = Math.max(0, 1 - v / .22), tipw = Math.min(1, Math.max(0, (v - .25) / .7));
    let R = .80 + tipw * .14, G = .72 + tipw * .17, B = .52 + tipw * .30; const st = Math.max(0, cl - .45) * 1.4 * (1 - tipw * .7); R -= st * .16; G -= st * .22; B -= st * .28;
    const band = (Math.sin(v * 190 + cl * 8) * .5 + .5) * .035; R -= band; G -= band; B -= band * 1.2;
    const q = Math.min(1, Math.pow(root, 1.4) * (.6 + fbm(nb, u * 14, v * 30, 3) * .8) * 1.5) * stain; R = R * (1 - q) + .2 * q; G = G * (1 - q) + .06 * q; B = B * (1 - q) + .045 * q;
    const k = crack(u, v) * .8; return [R - k * .5, G - k * .5, B - k * .45];
  }, { srgb: true });
  const rough = tex(256, 512, (u, v) => .28 + Math.max(0, 1 - v / .25) * .5 + (fbm(nb, u * 10, v * 20, 4) - .5) * .3 + crack(u, v) * .4);
  const bump = tex(512, 1024, (u, v) => .6 - crack(u, v) * .6 + (fbm(na, u * 50, v * 8, 3) - .5) * .18 - Math.max(0, 1 - v / .2) * fbm(nb, u * 40, v * 60, 2) * .5);
  for (const t of [colour, rough, bump]) t.wrapT = THREE.ClampToEdgeWrapping;
  return tag(new THREE.MeshPhysicalMaterial({ color: C(o.tint || '#ffffff'), map: colour, metalness: 0, roughness: 1, roughnessMap: rough, bumpMap: bump, bumpScale: 1.6, clearcoat: .35, clearcoatRoughness: .25, sheen: .3, sheenColor: 0xffe2b8, sheenRoughness: .6 }), o.label);
}
// Fur as it is: many thin layers, each showing only the hairs long enough to reach it.
export function furLayers(hex, stripeHex, n, stripeFn) {
  const nz = makeNoise(71, 256), c = C(hex), s = C(stripeHex), out = [];
  const hair = tex(512, 512, (u, v) => { const a = nz(u * 256, v * 256), b = nz(u * 256 + 91, v * 256 + 37); return Math.min(1, Math.max(0, (a * .6 + b * .4))); });
  const col = tex(512, 512, (u, v) => { const k = stripeFn ? stripeFn(u, v) : 0, m = .75 + (fbm(nz, u * 20, v * 20, 3) - .5) * .5; return [(c.r * (1 - k) + s.r * k) * m, (c.g * (1 - k) + s.g * k) * m, (c.b * (1 - k) + s.b * k) * m].map(x => Math.pow(Math.max(0, x), 1 / 2.2)); }, { srgb: true });
  hair.repeat.set(5, 5); 
  for (let i = 0; i < n; i++) { const t = i / (n - 1); out.push(new THREE.MeshStandardMaterial({ map: col, color: new THREE.Color(.35 + .9 * t, .35 + .9 * t, .35 + .9 * t), alphaMap: i ? hair : null, alphaTest: i ? .3 + t * .52 : 0, roughness: .9, metalness: 0, side: THREE.DoubleSide })); out[i].userData = { label: '' }; }
  return out;
}

// Shell, scale and chitin: hard, glossy, and with a thin skin that splits light into colours.
export function shell(hex, o) {
  o = o || {}; const t = maps();
  return tag(new THREE.MeshPhysicalMaterial({ color: C(hex), map: t.boneColour, metalness: o.metal == null ? .35 : o.metal, roughness: o.rough == null ? .3 : o.rough, bumpMap: t.boneBump, bumpScale: o.bump == null ? .8 : o.bump, clearcoat: .8, clearcoatRoughness: .15, iridescence: o.irid == null ? 1 : o.irid, iridescenceIOR: 1.7, iridescenceThicknessRange: [200, 600], side: o.double ? THREE.DoubleSide : THREE.FrontSide }), o.label);
}
