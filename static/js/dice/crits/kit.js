// ---------- What the newer crits share ----------
// The start and the end every crit owes the die, flat pictures laid on the page, and the tarot cards
// two of the crits deal. A crit's own file holds only what is its own.
import {
  canvas, drop, glowNumeral, mulberry, numberOnto, paintNumeral, shadeDie, strokePaths, strokeW, strokesFor, tintDie
} from '../engine.js';

// The frame round a crit: where the die landed, a list of everything the crit adds, and one way out
// that puts the die back exactly as it was with its number lit. A skipped crit takes the same way out.
function critScene(st, die, fi) {
  var T = st.T, g = die.group;
  var x = { T: T, hd: st.q.detail !== 'basic', g: g, s: die.size, p0: g.position.clone(), q0: g.quaternion.clone(),
    c: new T.Vector3(g.position.x, 0, g.position.z), own: [], rnd: mulberry(1 + Math.floor(Math.random() * 1e6)), done: false };
  x.add = function (o) { st.scene.add(o); x.own.push(o); return o; };
  // Cuts the die with planes: what lies behind one is not drawn. A material other dice use too is cut
  // only while this die's own meshes draw with it, so the planes never reach the other dice. Parts whose
  // own shader cannot be cut (set stones) are listed in x.loose, for the crit to hide when the cut passes.
  var clipped = [], hooks = []; x.loose = [];
  x.showLoose = function (on) { x.loose.forEach(function (ob) { ob.visible = on; }); };
  function matsOf(ob) { return ob.material ? (Array.isArray(ob.material) ? ob.material : [ob.material]) : []; }
  x.clip = function (planes) {
    var others = [];
    st.dice.forEach(function (d) { if (d !== die) d.group.traverse(function (ob) { matsOf(ob).forEach(function (m) { others.push(m); }); }); });
    g.traverse(function (ob) {
      var shared = [];
      matsOf(ob).forEach(function (m) {
        if (m.isShaderMaterial && !m.clipping) { if (x.loose.indexOf(ob) < 0) x.loose.push(ob); return; }
        if (clipped.indexOf(m) >= 0 || shared.indexOf(m) >= 0) return;
        if (others.indexOf(m) >= 0) shared.push(m); else { m.clippingPlanes = planes; clipped.push(m); }
      });
      if (!shared.length) return;
      var h = { ob: ob, bOwn: Object.prototype.hasOwnProperty.call(ob, 'onBeforeRender'), aOwn: Object.prototype.hasOwnProperty.call(ob, 'onAfterRender'), b: ob.onBeforeRender, a: ob.onAfterRender };
      ob.onBeforeRender = function (r, sc, cam, geo, mat) { if (shared.indexOf(mat) >= 0) mat.clippingPlanes = planes; h.b.apply(this, arguments); };
      ob.onAfterRender = function (r, sc, cam, geo, mat) { h.a.apply(this, arguments); if (shared.indexOf(mat) >= 0) mat.clippingPlanes = null; };
      hooks.push(h);
    });
  };
  x.end = function (color) {
    g.position.copy(x.p0); g.quaternion.copy(x.q0); g.scale.setScalar(1); g.visible = true;
    clipped.forEach(function (m) { m.clippingPlanes = null; }); clipped = []; x.showLoose(true);
    hooks.forEach(function (h) {
      if (h.bOwn) h.ob.onBeforeRender = h.b; else delete h.ob.onBeforeRender;
      if (h.aOwn) h.ob.onAfterRender = h.a; else delete h.ob.onAfterRender;
    }); hooks = [];
    shadeDie(die, 1); tintDie(die, color, 0);
    die.faces.forEach(function (fc, i) { if (i !== fi && fc.mat) fc.mat.emissiveIntensity = 0; });
    glowNumeral(die, fi, color);
    drop(st, x.own); x.done = true;
  };
  return x;
}

// A picture drawn once and used as a texture.
function picture(T, w, h, draw) {
  var c = canvas(w, h), g = c.getContext('2d'); draw(g, w, h);
  var t = new T.CanvasTexture(c); t.colorSpace = T.SRGBColorSpace; t.anisotropy = 4; return t;
}
// A picture lying flat on the page. Light pictures add to the page; the rest cover it.
function flat(T, tex, w, h, o) {
  o = o || {};
  var m = new T.Mesh(new T.PlaneGeometry(w, h), new T.MeshBasicMaterial({ map: tex, color: o.color == null ? 0xffffff : o.color, transparent: true, opacity: o.opacity == null ? 1 : o.opacity, depthWrite: false,
    side: T.DoubleSide, blending: o.add ? T.AdditiveBlending : T.NormalBlending, toneMapped: false }));
  m.rotation.x = -Math.PI / 2; m.position.y = o.y == null ? 0.02 : o.y; if (o.order != null) m.renderOrder = o.order;
  return m;
}
// The landed face's number as a picture of its own, upright on the page: for a crit that takes the die
// away and still owes the table its result. It glows the way the die's own number does; o.solid draws
// it in one flat colour instead, for a dark number on a pale ground.
function numeral(T, die, fi, color, o) {
  var lay = die.faces[fi].lay;
  // A d4's face carries three small numbers, none of them the roll: its picture is the roll alone.
  if (lay.sub) { var h = lay.r * 0.96; lay = Object.assign({}, lay, { sub: null, h: h, strokes: strokesFor(lay.label, lay.cx, lay.cy, h) }); }
  var tex = picture(T, lay.cv, lay.cv, function (g) {
    if (!(o && o.solid)) numberOnto(g, lay, color);
    else if (lay.style === 'classic') { g.fillStyle = color; paintNumeral(g, lay, 'classic'); }
    else { g.lineJoin = 'round'; g.lineCap = 'round'; g.strokeStyle = color; g.lineWidth = strokeW(lay) * 1.2; strokePaths(g, lay.strokes); }
  }), size = lay.cv / lay.s * die.size;
  return flat(T, tex, size, size, o);
}
// A soft round patch, for glows, pools and shadows.
var SOFT = null;
function softTex(T) {
  if (SOFT) return SOFT;
  SOFT = picture(T, 128, 128, function (g) { var gr = g.createRadialGradient(64, 64, 0, 64, 64, 64); gr.addColorStop(0, 'rgba(255,255,255,1)'); gr.addColorStop(0.5, 'rgba(255,255,255,.55)'); gr.addColorStop(1, 'rgba(255,255,255,0)'); g.fillStyle = gr; g.fillRect(0, 0, 128, 128); });
  SOFT.userData.shared = true; return SOFT;
}

// ---------- Tarot cards ----------
// A card is drawn the way a printed deck is: a border, a numeral, one bold figure and a title. `grim`
// cards are printed on dark stock in bone and red; the rest on ivory in gold and blue.
var CARD_W = 256, CARD_H = 420;
function cardFrame(g, grim, numeral, title) {
  var W = CARD_W, H = CARD_H, paper = grim ? '#16131c' : '#f3ead2', ink = grim ? '#d9cfc0' : '#1c2a66', gold = grim ? '#a8242c' : '#c9962c';
  g.fillStyle = paper; g.beginPath(); g.roundRect(0, 0, W, H, 18); g.fill();
  var gr = g.createRadialGradient(W / 2, H / 2, 40, W / 2, H / 2, 300); gr.addColorStop(0, 'rgba(255,255,255,' + (grim ? 0.06 : 0.25) + ')'); gr.addColorStop(1, 'rgba(0,0,0,' + (grim ? 0.4 : 0.14) + ')');
  g.fillStyle = gr; g.fill();
  g.strokeStyle = gold; g.lineWidth = 5; g.beginPath(); g.roundRect(12, 12, W - 24, H - 24, 10); g.stroke();
  g.lineWidth = 1.5; g.beginPath(); g.roundRect(21, 21, W - 42, H - 42, 6); g.stroke();
  g.fillStyle = ink; g.textAlign = 'center'; g.textBaseline = 'middle';
  g.font = '600 26px Georgia, "Times New Roman", serif'; g.fillText(numeral, W / 2, 48);
  g.font = '600 21px Georgia, "Times New Roman", serif'; g.fillText(title, W / 2, H - 46);
  g.strokeStyle = gold; g.lineWidth = 1.5; g.beginPath(); g.moveTo(52, 68); g.lineTo(W - 52, 68); g.moveTo(52, H - 68); g.lineTo(W - 52, H - 68); g.stroke();
  return { ink: ink, gold: gold, paper: paper };
}
function rays(g, x, y, r0, r1, n, wide) {
  for (var i = 0; i < n; i++) { var a = i / n * 6.2832, r = i % 2 ? r1 * 0.78 : r1; g.beginPath(); g.moveTo(x + Math.cos(a - wide) * r0, y + Math.sin(a - wide) * r0); g.lineTo(x + Math.cos(a) * r, y + Math.sin(a) * r); g.lineTo(x + Math.cos(a + wide) * r0, y + Math.sin(a + wide) * r0); g.closePath(); g.fill(); }
}
function star(g, x, y, r, n) {
  g.beginPath(); for (var i = 0; i < n * 2; i++) { var a = i / (n * 2) * 6.2832 - Math.PI / 2, rr = i % 2 ? r * 0.42 : r; if (i) g.lineTo(x + Math.cos(a) * rr, y + Math.sin(a) * rr); else g.moveTo(x + Math.cos(a) * rr, y + Math.sin(a) * rr); } g.closePath(); g.fill();
}
var CARDS = {
  back: function (g) {
    var W = CARD_W, H = CARD_H;
    g.fillStyle = '#141c4a'; g.beginPath(); g.roundRect(0, 0, W, H, 18); g.fill();
    g.strokeStyle = '#d6a942'; g.lineWidth = 5; g.beginPath(); g.roundRect(12, 12, W - 24, H - 24, 10); g.stroke();
    g.save(); g.beginPath(); g.roundRect(22, 22, W - 44, H - 44, 6); g.clip();
    g.strokeStyle = 'rgba(214,169,66,.55)'; g.lineWidth = 1.2;
    for (var i = -12; i < 20; i++) { g.beginPath(); g.moveTo(i * 28, 0); g.lineTo(i * 28 + H, H); g.moveTo(i * 28 + H, 0); g.lineTo(i * 28, H); g.stroke(); }
    g.restore();
    g.fillStyle = '#141c4a'; g.beginPath(); g.arc(W / 2, H / 2, 62, 0, 7); g.fill(); g.strokeStyle = '#d6a942'; g.lineWidth = 3; g.stroke();
    g.fillStyle = '#f0c95a'; star(g, W / 2, H / 2, 44, 8);
  },
  sun: function (g) {
    var k = cardFrame(g, false, 'XIX', 'THE SUN'), x = CARD_W / 2, y = 205;
    g.fillStyle = k.gold; rays(g, x, y, 60, 118, 24, 0.09);
    var gr = g.createRadialGradient(x - 16, y - 16, 6, x, y, 62); gr.addColorStop(0, '#fff6c4'); gr.addColorStop(1, '#f0b62a');
    g.fillStyle = gr; g.beginPath(); g.arc(x, y, 58, 0, 7); g.fill(); g.strokeStyle = '#a8741a'; g.lineWidth = 3; g.stroke();
  },
  star: function (g) {
    var k = cardFrame(g, false, 'XVII', 'THE STAR'), x = CARD_W / 2, y = 196;
    g.fillStyle = k.gold; star(g, x, y, 78, 8);
    g.fillStyle = k.ink; [[60, 120, 13], [196, 118, 13], [52, 262, 11], [204, 266, 11], [128, 310, 14], [92, 96, 7], [170, 300, 7]].forEach(function (p) { star(g, p[0], p[1], p[2], 4); });
  },
  wheel: function (g) {
    var k = cardFrame(g, false, 'X', 'FORTUNE'), x = CARD_W / 2, y = 205;
    g.strokeStyle = k.gold; g.lineWidth = 9; g.beginPath(); g.arc(x, y, 92, 0, 7); g.stroke();
    g.strokeStyle = k.ink; g.lineWidth = 3; g.beginPath(); g.arc(x, y, 76, 0, 7); g.stroke();
    for (var i = 0; i < 8; i++) { var a = i / 8 * 6.2832; g.lineWidth = i % 2 ? 2.5 : 5; g.beginPath(); g.moveTo(x, y); g.lineTo(x + Math.cos(a) * 76, y + Math.sin(a) * 76); g.stroke(); }
    g.fillStyle = k.gold; g.beginPath(); g.arc(x, y, 20, 0, 7); g.fill();
  },
  moon: function (g) {
    var k = cardFrame(g, true, 'XVIII', 'THE MOON'), x = CARD_W / 2, y = 190;
    g.fillStyle = k.ink; g.beginPath(); g.arc(x, y, 72, 0, 7); g.fill();
    g.fillStyle = k.paper; g.beginPath(); g.arc(x + 30, y - 12, 62, 0, 7); g.fill();
    g.fillStyle = k.gold; for (var i = 0; i < 9; i++) { g.beginPath(); g.ellipse(60 + i * 17, 300 + (i % 2) * 14, 3, 9, 0, 0, 7); g.fill(); }
  },
  death: function (g) {
    var k = cardFrame(g, true, 'XIII', 'DEATH'), x = CARD_W / 2, y = 190;
    g.fillStyle = k.ink; g.beginPath(); g.ellipse(x, y - 10, 62, 68, 0, 0, 7); g.fill(); g.fillRect(x - 38, y + 34, 76, 44);
    g.fillStyle = k.paper; g.beginPath(); g.ellipse(x - 25, y - 4, 17, 21, 0, 0, 7); g.ellipse(x + 25, y - 4, 17, 21, 0, 0, 7); g.fill();
    g.beginPath(); g.moveTo(x, y + 18); g.lineTo(x - 9, y + 38); g.lineTo(x + 9, y + 38); g.closePath(); g.fill();
    for (var i = -2; i <= 2; i++) g.fillRect(x + i * 14 - 1.5, y + 50, 3, 28);
  },
  tower: function (g) {
    var k = cardFrame(g, true, 'XVI', 'THE TOWER'), x = CARD_W / 2;
    g.fillStyle = k.ink; g.beginPath(); g.moveTo(x - 38, 330); g.lineTo(x - 28, 150); g.lineTo(x + 28, 150); g.lineTo(x + 38, 330); g.closePath(); g.fill();
    for (var i = -2; i <= 2; i += 2) g.fillRect(x + i * 15 - 9, 128, 18, 26);
    g.fillStyle = k.paper; g.fillRect(x - 8, 190, 16, 28); g.fillRect(x - 8, 252, 16, 28);
    g.strokeStyle = k.gold; g.lineWidth = 7; g.lineJoin = 'miter'; g.beginPath(); g.moveTo(206, 84); g.lineTo(160, 132); g.lineTo(184, 138); g.lineTo(136, 186); g.stroke();
    g.fillStyle = k.gold; [[70, 150], [58, 210], [196, 230], [84, 280]].forEach(function (p) { g.beginPath(); g.moveTo(p[0], p[1] - 12); g.quadraticCurveTo(p[0] + 9, p[1], p[0], p[1] + 10); g.quadraticCurveTo(p[0] - 9, p[1], p[0], p[1] - 12); g.fill(); });
  }
};
var cardTexCache = {};
function cardTex(T, name) {
  if (!cardTexCache[name]) { cardTexCache[name] = picture(T, CARD_W, CARD_H, CARDS[name]); cardTexCache[name].userData.shared = true; }
  return cardTexCache[name];
}
// One card on the page. turn runs 0 (face down) to 1 (face up): it lifts off the page as it goes
// over, as a card flipped by a finger does. h is the card's height in world units.
function tarot(T, name, h) {
  var w = h * CARD_W / CARD_H, root = new T.Group(), flip = new T.Group(), geo = new T.PlaneGeometry(w, h);
  var front = new T.Mesh(geo, new T.MeshBasicMaterial({ map: cardTex(T, name), transparent: true, depthWrite: false, toneMapped: false }));
  var back = new T.Mesh(geo, new T.MeshBasicMaterial({ map: cardTex(T, 'back'), transparent: true, depthWrite: false, toneMapped: false }));
  // Face down to begin with: the back looks up, the face looks at the page.
  back.rotation.x = -Math.PI / 2; front.rotation.x = Math.PI / 2; front.rotation.z = Math.PI;
  var shade = flat(T, softTex(T), w * 1.5, h * 1.3, { color: 0x000000, opacity: 0.35, y: 0.012 });
  flip.add(front, back); root.add(shade, flip); root.renderOrder = 5;
  return { root: root, front: front, back: back, w: w, h: h,
    turn: function (k) { flip.rotation.z = Math.PI * k; flip.position.y = 0.06 + Math.sin(Math.PI * k) * w * 0.55; shade.material.opacity = 0.35 * (1 - 0.5 * Math.sin(Math.PI * k)); },
    fade: function (a) { front.material.opacity = a; back.material.opacity = a; shade.material.opacity = 0.35 * a; } };
}

export { CARD_H, CARD_W, cardTex, critScene, flat, numeral, picture, softTex, tarot };
