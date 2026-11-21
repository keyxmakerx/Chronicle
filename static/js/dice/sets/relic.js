// ==================== Relic coin ====================
// Dice struck like old coins. Each face is a coin's field with a raised rim, a ring of beads inside
// it and the number standing up in relief, as if hammered from a die. Centuries of handling have
// polished the high places bright and left tarnish in the low ones.
import {
  hexA, mix, mulberry, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// coin: [polished high places, the body, deep shade]; patina: what has gathered in the low places.
var RELIC = [
  { key: 'bronze', name: 'Bronze and verdigris', note: 'Brown bronze, green in the low places', burn: '#ffcf7a', ink: '#e6b877',
    coin: ['#f0c98c', '#a8733a', '#4a2c10'], patina: '#3f9a84', metal: { color: '#e6b877', hi: '#ffe6bd', lo: '#7a4c1c', rough: 0.3 }, base: { grad: ['#c08a4c', '#5a3614'], rough: 0.4 } },
  { key: 'silver', name: 'Tarnished silver', note: 'Bright silver, black in the low places', burn: '#d8e6ff', ink: '#e6eaf0',
    coin: ['#ffffff', '#b4bac6', '#474c58'], patina: '#16171c', metal: { color: '#e6eaf0', hi: '#ffffff', lo: '#7b8494', rough: 0.26 }, base: { grad: ['#c6ccd8', '#565c68'], rough: 0.36 } },
  { key: 'gold', name: 'Hoard gold', note: 'Yellow gold, warm shadow in the low places', burn: '#fff0a0', ink: '#f6d470',
    coin: ['#fff4c0', '#e2b03c', '#7a5208'], patina: '#6a3c08', metal: { color: '#f6d470', hi: '#fff6cf', lo: '#94680f', rough: 0.24 }, base: { grad: ['#eec050', '#8a5c0c'], rough: 0.32 } },
  { key: 'iron', name: 'Iron and rust', note: 'Dark iron, rust in the low places', burn: '#ff9a5c', ink: '#c9ced6',
    coin: ['#c9ced6', '#5c6068', '#1c1d21'], patina: '#a8481c', metal: { color: '#c9ced6', hi: '#f2f4f8', lo: '#4c5058', rough: 0.4 }, base: { grad: ['#74787f', '#232428'], rough: 0.5 } },
  { key: 'copper', name: 'Copper and sea green', note: 'Red copper, pale green in the low places', burn: '#ffb48a', ink: '#f0a070',
    coin: ['#ffc8a4', '#c2643a', '#541f0c'], patina: '#7fd0b4', metal: { color: '#f0a070', hi: '#ffd8c0', lo: '#8a401c', rough: 0.28 }, base: { grad: ['#d47a4e', '#5e2410'], rough: 0.38 } },
  { key: 'orichalc', name: 'Sunken orichalcum', note: 'Pale rose metal, deep blue in the low places', burn: '#ffd8e8', ink: '#f4c6c2',
    coin: ['#fff0ea', '#e2a39c', '#6e3840'], patina: '#1c3f8a', metal: { color: '#f4c6c2', hi: '#fff4f0', lo: '#8e5058', rough: 0.26 }, base: { grad: ['#e8b0a8', '#6e3840'], rough: 0.34 } }
];

function relSeed(lay) { return lay.cell ? lay.cell.x * 23 + lay.cell.y * 3 + lay.cv : lay.cv; }
function relInset(lay, k) { var cx = lay.cv / 2, cy = lay.cv / 2; return lay.inner.map(function (p) { return [cx + (p[0] - cx) * k, cy + (p[1] - cy) * k]; }); }
function relTrace(g, pts) { g.beginPath(); pts.forEach(function (p, i) { if (i) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.closePath(); }
function relClear(lay, x, y, pad) { return !lay.sub || lay.sub.every(function (s) { return Math.hypot(x - s.cx, y - s.cy) > s.h * 0.8 + pad; }); }
// Everything that stands up from the field, drawn with whatever fill and stroke are set; grow widens
// it, for the shadow and the tarnish that gather round raised work.
function relStruck(g, lay, k, grow) {
  var w = strokeW(lay), pts = relInset(lay, 0.81);
  g.lineJoin = 'round';
  g.lineWidth = (6 + grow) * k; relTrace(g, relInset(lay, 0.96)); g.stroke();
  pts.forEach(function (a, i) {
    var b = pts[(i + 1) % pts.length], dx = b[0] - a[0], dy = b[1] - a[1], L = Math.hypot(dx, dy), n = Math.max(3, Math.round(L / (6.4 * k)));
    for (var j = 0; j < n; j++) { var t = j / n, x = a[0] + dx * t, y = a[1] + dy * t; if (relClear(lay, x, y, 3 * k)) { g.beginPath(); g.arc(x, y, (1.75 + grow * 0.4) * k, 0, 7); g.fill(); } }
  });
  paintNumeral(g, lay, lay.style, w * 1.16 + grow * k * 0.9);
}

registerSet({
  key: 'relic', name: 'Relic coin', note: 'Struck like old coins: a raised rim, a ring of beads, and the number standing up in relief, worn bright on top with tarnish in the low places.',
  show: 'molten', bev: 0.05, see: null, relief: 2.6,
  colours: RELIC,
  paint: function (g, lay, c, seed) {
    var cv = lay.cv, k = cv / 256, rnd = mulberry(seed);
    g.fillStyle = mix(c.coin[1], c.coin[2], 0.25); g.fillRect(0, 0, cv, cv);
    // The field is not even: hammer marks and wear.
    g.save(); try { g.filter = 'blur(' + (7 * k) + 'px)'; } catch (e) {}
    for (var i = 0; i < 12; i++) { g.fillStyle = hexA(rnd() > 0.5 ? c.coin[1] : c.coin[2], 0.4); g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (10 + rnd() * 24) * k, 0, 7); g.fill(); }
    // Tarnish lies in patches, and thickest round the raised work.
    for (var q = 0; q < 6; q++) { g.fillStyle = hexA(c.patina, 0.4); g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (10 + rnd() * 20) * k, 0, 7); g.fill(); }
    g.restore();
    g.save(); try { g.filter = 'blur(' + (2.4 * k) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = hexA(c.patina, 0.95); relStruck(g, lay, k, 5.5); g.restore();
    g.fillStyle = g.strokeStyle = hexA(c.coin[2], 0.9); relStruck(g, lay, k, 1.6);
    // The raised work, polished by handling: bright toward the light, deeper away from it.
    var gr = g.createLinearGradient(0, 0, cv, cv); gr.addColorStop(0, c.coin[0]); gr.addColorStop(0.55, mix(c.coin[0], c.coin[1], 0.6)); gr.addColorStop(1, c.coin[1]);
    g.fillStyle = g.strokeStyle = gr; relStruck(g, lay, k, 0);
    g.save(); g.translate(-0.8 * k, -0.9 * k); g.globalAlpha = 0.5; g.fillStyle = g.strokeStyle = c.coin[0]; relStruck(g, lay, k, -2.6); g.restore();
    // Fine scratches from a long life in purses.
    g.lineWidth = 0.45 * k; g.lineCap = 'round';
    for (var j = 0; j < 34; j++) { var x = rnd() * cv, y = rnd() * cv, a = rnd() * 6.28, l = (4 + rnd() * 16) * k; g.strokeStyle = hexA(rnd() > 0.4 ? c.coin[0] : c.coin[2], 0.3); g.beginPath(); g.moveTo(x, y); g.lineTo(x + Math.cos(a) * l, y + Math.sin(a) * l); g.stroke(); }
  },
  // Metal throughout: the field dull, the tarnish duller and less metal, the raised work polished.
  surface: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, rnd = mulberry(relSeed(lay));
    g.fillStyle = 'rgb(0,' + Math.round((c.base.rough + 0.16) * 255) + ',165)'; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (2.4 * k) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = 'rgb(0,190,60)'; relStruck(g, lay, k, 5.5); g.restore();
    g.fillStyle = g.strokeStyle = 'rgb(0,' + Math.round(c.metal.rough * 255) + ',215)'; relStruck(g, lay, k, 0);
    for (var j = 0; j < 20; j++) { g.fillStyle = 'rgb(0,60,215)'; g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (0.5 + rnd()) * k, 0, 7); g.fill(); }
  },
  height: function (g, lay) {
    var cv = lay.cv, k = cv / 256, rnd = mulberry(relSeed(lay) + 5);
    g.fillStyle = '#5a5a5a'; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (5 * k) + 'px)'; } catch (e) {}
    for (var i = 0; i < 20; i++) { g.fillStyle = rnd() > 0.5 ? 'rgba(255,255,255,.14)' : 'rgba(0,0,0,.2)'; g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (6 + rnd() * 14) * k, 0, 7); g.fill(); }
    g.restore();
    // Struck relief has soft shoulders: a wide low swell under a narrower high one.
    g.save(); try { g.filter = 'blur(' + (1.8 * k) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = '#a8a8a8'; relStruck(g, lay, k, 1.4); g.restore();
    g.save(); try { g.filter = 'blur(' + (0.9 * k) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = '#ffffff'; relStruck(g, lay, k, -1.2); g.restore();
  },
  rim: function (c) { return { col: mix(c.coin[0], c.coin[1], 0.6), rough: c.base.rough, metal: 0.85 }; },
  physical: function () { return { envMapIntensity: 1.5 }; }
});
