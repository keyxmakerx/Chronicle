// hero_border_picker_browser.test.mjs — the border picker's samples and colours.
//
// Runs hero_border_picker.js with hero_borders.js in a real Chromium on a bare
// page that loads hero_creator.css, hero_borders.css and hero_border_picker.css
// and draws pickers through window.ChronicleBorderPicker, with the real
// pictures served. It pins the wall (None, the game system's sample, four
// groups, sixteen samples, one pressed), the colour a sample previews, what a
// press asks for, the colours block, sync without a redraw, hostile data that
// never reaches the markup, the field and wall sizes at 360 wide, the keyboard,
// text contrast in light and dark, and bind's undo.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing. PICKER_SHOTS=<dir> also writes
// the two review screenshots.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const STATIC = join(root, 'internal/plugins/entities/static');
const file = (rel) => readFileSync(join(STATIC, rel));

const mk = (img, extra = {}) => ({ img, ...extra });
const LIB = {
  'carved-stone': mk('earth.webp', { corner: 'carved-stone-corner.webp', cornerMask: 'carved-stone-corner-mask.webp', peak: 0.93, gain: 1.3 }),
  earth: mk('earth.webp'),
  fire: mk('fire.webp', { mask: 'fire-mask.webp', peak: 1, gain: 1.3 }),
  ice: mk('ice.webp'),
  storm: mk('storm.webp', { mask: 'storm-mask.webp', peak: 1, gain: 1.3 }),
  glass: mk('glass.webp'),
  'glass-smoke': mk('glass-smoke.webp'),
  'glass-sea': mk('glass-sea.webp'),
  'glass-rose': mk('glass-rose.webp'),
  'glass-midnight': mk('glass-midnight.webp'),
  'rune-metal': mk('rune-metal.webp', { mask: 'rune-metal-mask.webp', peak: 0.96, gain: 1.3 }),
  'great-rune': mk('great-rune.webp', { mask: 'great-rune-mask.webp', peak: 0, gain: 0.85 }),
  mechanical: mk('mechanical.webp', { mask: 'mechanical-mask.webp', corner: 'mechanical-corners.webp', cornerMask: 'mechanical-corners-mask.webp', ground: 'mechanical-steam.webp', peak: 0, gain: 1, glow: 1.4 }),
  'thorn-glass': mk('thorn-glass.webp', { mask: 'thorn-glass-mask.webp', peak: 0.92, gain: 1.3 }),
  scales: mk('scales.webp', { over: 'scales-boss.webp', dye: true }),
  vine: mk('vine.webp', { mask: 'vine-mask.webp', peak: 0.74, gain: 1.3 }),
};
const URLS = ['img', 'mask', 'over', 'corner', 'cornerMask', 'ground'];
const libJSON = JSON.stringify(Object.fromEntries(Object.entries(LIB).map(([k, v]) => [k, Object.fromEntries(Object.entries(v).map(([p, x]) => [p, URLS.includes(p) ? '/borders/' + x + '?v=abc123' : x]))])));

// What cardborders.PickerJSON hands the page; the last test keeps it honest against the Go source.
const GROUPS = [
  { name: 'Elements', keys: ['earth', 'fire', 'ice', 'storm'] },
  { name: 'Stone and metal', keys: ['carved-stone', 'rune-metal', 'great-rune', 'mechanical'] },
  { name: 'Glass', keys: ['glass', 'glass-smoke', 'glass-sea', 'glass-rose', 'glass-midnight'] },
  { name: 'Wild', keys: ['thorn-glass', 'scales', 'vine'] },
];
const INFO = {
  'carved-stone': ['Carved stone', '#e0a030'], earth: ['Earth', '#b08a5a'], fire: ['Fire', '#d9622b'], ice: ['Ice', '#8fd8ff'], storm: ['Storm', '#6fa8dc'],
  glass: ['Glass', '#58c4d8'], 'glass-smoke': ['Smoke glass', '#9aa3ad'], 'glass-sea': ['Sea glass', '#4fb8a8'], 'glass-rose': ['Rose glass', '#e58fb0'], 'glass-midnight': ['Midnight glass', '#a78bfa'],
  'rune-metal': ['Rune metal', '#3fc8d4'], 'great-rune': ['Great rune', '#b48cf2'], mechanical: ['Boiler works', '#6fa8dc'],
  'thorn-glass': ['Thorn glass', '#c4243a'], scales: ['Scales', '#2fb5a4'], vine: ['Vine and wood', '#6fae4f'],
};
const KEYS = Object.keys(INFO);
const PICK = { groups: GROUPS, borders: Object.fromEntries(KEYS.map((k) => [k, { name: INFO[k][0], colour: INFO[k][1], tints: !!(LIB[k].mask || LIB[k].cornerMask || LIB[k].dye) }])) };

const LIGHT = ':root{--color-bg-primary:#f9fafb;--color-bg-secondary:#f3f4f6;--color-card-bg:#fff;--color-bg-tertiary:#f3f4f6;--color-border:#e5e7eb;--color-text-primary:#111827;--color-text-secondary:#4b5563;--color-accent-rgb:99 102 241;--dur-micro:120ms;--dur-standard:200ms;--ease-out:cubic-bezier(.16,1,.3,1)}';
const DARK = 'html.dark{--color-bg-primary:#111827;--color-bg-secondary:#1f2937;--color-card-bg:#1f2937;--color-bg-tertiary:#374151;--color-border:#374151;--color-text-primary:#f9fafb;--color-text-secondary:#c4c9d2}';

// o: { state, opts, dark, hostStyle, noSys, pick, apply }. noSys leaves state.sys undefined.
function page(o) {
  const init = JSON.stringify({ state: o.state || { look: null }, opts: o.opts || { lay: 'wall' }, noSys: !('sys' in (o.state || {})), pick: o.pick || PICK, apply: !!o.apply });
  return `<!doctype html><html${o.dark ? ' class="dark"' : ''}><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>${LIGHT}${DARK}:root{--color-accent:#6366f1}*,::before,::after{box-sizing:border-box}body{margin:0;font-family:sans-serif;background:var(--color-bg-primary);color:var(--color-text-primary)}</style>
<link rel="stylesheet" href="/hero_creator.css"><link rel="stylesheet" href="/hero_borders.css"><link rel="stylesheet" href="/hero_border_picker.css"></head>
<body><button id="before" type="button">before</button><div id="host" style="${o.hostStyle || ''}"></div><button id="after" type="button">after</button>
<script>window.Chronicle = { escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])) };</script>
<script src="/hero_borders.js"></script><script src="/hero_border_picker.js"></script>
<script>
var INIT = ${init.replace(/</g, '\\u003c')}, host = document.getElementById('host');
window.B = window.ChronicleHeroBorders(${libJSON}, document);
window.P = window.ChronicleBorderPicker(INIT.pick, window.B);
window.S = INIT.state; if (INIT.noSys) delete window.S.sys;
window.OPTS = INIT.opts; window.calls = []; window.APPLY = INIT.apply;
window.draw = function () { host.innerHTML = P.html(S, OPTS); };
window.draw();
window.off = P.bind(host, { state: function () { return S; }, change: function (pk, look, merge) { calls.push({ look: look, merge: merge }); if (window.APPLY) { S.look = look; P.sync(pk, S); } } });
</script></body></html>`;
}

async function open(t, o = {}) {
  const { width = 390, height = 900 } = o;
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport: { width, height }, reducedMotion: 'reduce', hasTouch: false });
  const pg = await ctx.newPage();
  const errors = [];
  pg.on('pageerror', (e) => errors.push(String(e)));
  await pg.route('http://t.test/**', (route) => {
    const path = new URL(route.request().url()).pathname;
    const js = { '/hero_borders.js': 'js/hero_borders.js', '/hero_border_picker.js': 'js/hero_border_picker.js' }[path];
    if (js) return route.fulfill({ contentType: 'text/javascript', body: file(js) });
    const css = { '/hero_creator.css': 'css/hero_creator.css', '/hero_borders.css': 'css/hero_borders.css', '/hero_border_picker.css': 'css/hero_border_picker.css' }[path];
    if (css) return route.fulfill({ contentType: 'text/css', body: file(css) });
    if (path.startsWith('/borders/')) return route.fulfill({ contentType: 'image/webp', body: file('img/borders/' + path.slice('/borders/'.length)) });
    return route.fulfill({ contentType: 'text/html', body: page(o) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hcw-pk');
  return { pg, errors };
}

const pressed = (pg) => pg.evaluate(() => Array.from(document.querySelectorAll('.hcw-t[aria-pressed="true"]')).map((b) => b.getAttribute('data-b')));
const calls = (pg) => pg.evaluate(() => window.calls);
const last = async (pg) => (await calls(pg)).slice(-1)[0];
const tileK = (pg) => pg.evaluate(() => Object.fromEntries(Array.from(document.querySelectorAll('.hcw-t[data-b]')).map((b) => [b.getAttribute('data-b'), (b.querySelector('.hc-gcard').style.getPropertyValue('--k') || '').trim()])));

// --- The wall ---

test('the wall: None first, four groups in order, sixteen samples, one pressed', { skip }, async (t) => {
  const { pg, errors } = await open(t, { state: { look: { border: 'fire', colour: '#d9622b' } } });
  const info = await pg.evaluate(() => ({
    root: document.querySelector('.hcw-pk').outerHTML.slice(0, 80),
    cls: document.querySelector('.hcw-pk').className, lay: document.querySelector('.hcw-pk').getAttribute('data-lay'), hasPk: document.querySelector('.hcw-pk').hasAttribute('data-pk'),
    keys: Array.from(document.querySelectorAll('.hcw-t')).map((b) => b.getAttribute('data-b')),
    heads: Array.from(document.querySelectorAll('.hcw-g h4')).map((h) => h.textContent),
    groups: Array.from(document.querySelectorAll('.hcw-g')).map((g) => Array.from(g.querySelectorAll('.hcw-t')).map((b) => [b.getAttribute('data-b'), b.querySelector('.hcw-tn').textContent])),
    types: Array.from(document.querySelectorAll('.hcw-t')).map((b) => b.type),
    trims: Array.from(document.querySelectorAll('.hcw-t')).map((b) => b.querySelector('.hcw-sm > .hc-gcard').getAttribute('data-trim')),
  }));
  assert.match(info.cls, /\bhcw-pk\b.*\bhc\b/);
  assert.equal(info.lay, 'wall');
  assert.ok(info.hasPk);
  assert.equal(info.keys[0], '', 'None is first');
  assert.ok(!info.keys.includes('@sys'), 'no game system sample when sys is undefined');
  assert.deepEqual(info.heads, ['Elements', 'Stone and metal', 'Glass', 'Wild']);
  assert.deepEqual(info.groups[0], [['', 'None']]);
  GROUPS.forEach((g, i) => assert.deepEqual(info.groups[i + 1], g.keys.map((k) => [k, INFO[k][0]])));
  assert.equal(info.keys.length - 1, 16);
  assert.ok(info.types.every((x) => x === 'button'));
  assert.deepEqual(info.trims, [null, ...GROUPS.flatMap((g) => g.keys)]);
  assert.deepEqual(await pressed(pg), ['fire']);
  assert.deepEqual(errors, []);
});

test('the wall: no border presses None; the game system sample is there only when sys is not undefined', { skip }, async (t) => {
  let { pg } = await open(t, { state: { look: null } });
  assert.deepEqual(await pressed(pg), ['']);
  ({ pg } = await open(t, { state: { look: null, sys: { border: 'ice', colour: '#8fd8ff' } }, opts: { lay: 'wall', sysLabel: 'Draw Steel’s' } }));
  const sys = await pg.evaluate(() => { const b = document.querySelector('.hcw-t[data-b="@sys"]'); return { name: b.querySelector('.hcw-tn').textContent, trim: b.querySelector('.hc-gcard').getAttribute('data-trim'), order: Array.from(b.parentNode.children).indexOf(b) }; });
  assert.deepEqual(sys, { name: 'Draw Steel’s', trim: 'ice', order: 1 });
  assert.deepEqual(await pressed(pg), [''], 'a card with no border while the system gives one: None');
  ({ pg } = await open(t, { state: { look: { border: 'ice', colour: '#8fd8ff' }, sys: { border: 'ice', colour: '#8fd8ff' } } }));
  assert.deepEqual((await pressed(pg)).sort(), ['@sys', 'ice'], 'the system sample and its own border are both the card');
  ({ pg } = await open(t, { state: { look: null, sys: null }, opts: { lay: 'wall', none: 'No border' } }));
  const nul = await pg.evaluate(() => { const b = document.querySelector('.hcw-t[data-b="@sys"]'); return { has: !!b, trim: b.querySelector('.hc-gcard').hasAttribute('data-trim'), none: document.querySelector('.hcw-t[data-b=""] .hcw-tn').textContent }; });
  assert.deepEqual(nul, { has: true, trim: false, none: 'No border' });
  assert.deepEqual(await pressed(pg), ['@sys'], 'sys null and no border: exactly one sample, the system’s');
});

test('a chosen colour that is not the border’s own shows on every sample; its own does not', { skip }, async (t) => {
  let { pg } = await open(t, { state: { look: { border: 'fire', colour: '#112233' } } });
  const k = await tileK(pg);
  assert.equal(k[''], '');
  for (const key of KEYS) assert.equal(k[key], '#112233', key);
  ({ pg } = await open(t, { state: { look: { border: 'fire', colour: '#D9622B' } } }));
  const own = await tileK(pg);
  for (const key of KEYS) assert.equal(own[key], INFO[key][1], key + ' in its own colour');
  ({ pg } = await open(t, { state: { look: null } }));
  assert.equal((await tileK(pg)).fire, '#d9622b');
});

// --- Pressing ---

test('pressing a border keeps a chosen colour and otherwise takes its own', { skip }, async (t) => {
  let { pg } = await open(t, { state: { look: { border: 'fire', colour: '#112233' } } });
  await pg.click('.hcw-t[data-b="ice"]');
  assert.deepEqual(await last(pg), { look: { border: 'ice', colour: '#112233' }, merge: false });
  ({ pg } = await open(t, { state: { look: { border: 'fire', colour: '#d9622b' } } }));
  await pg.click('.hcw-t[data-b="ice"]');
  assert.deepEqual(await last(pg), { look: { border: 'ice', colour: '#8fd8ff' }, merge: false });
  ({ pg } = await open(t, { state: { look: null } }));
  await pg.click('.hcw-t[data-b="vine"]');
  assert.deepEqual(await last(pg), { look: { border: 'vine', colour: '#6fae4f' }, merge: false });
});

test('pressing None gives null; the game system sample gives sys, also when sys is null', { skip }, async (t) => {
  let { pg } = await open(t, { state: { look: { border: 'fire', colour: '#d9622b' }, sys: { border: 'ice', colour: '#abcdef' } } });
  await pg.click('.hcw-t[data-b=""]');
  assert.deepEqual(await last(pg), { look: null, merge: false });
  await pg.click('.hcw-t[data-b="@sys"]');
  assert.deepEqual(await last(pg), { look: { border: 'ice', colour: '#abcdef' }, merge: false });
  ({ pg } = await open(t, { state: { look: { border: 'fire', colour: '#d9622b' }, sys: null } }));
  await pg.click('.hcw-t[data-b="@sys"]');
  assert.deepEqual(await last(pg), { look: null, merge: false });
});

test('colours: own swatch, a named colour, and typing in the colour input', { skip }, async (t) => {
  const { pg } = await open(t, { state: { look: { border: 'fire', colour: '#112233' } } });
  await pg.click('.hcw-c--own');
  assert.deepEqual(await last(pg), { look: { border: 'fire', colour: '#d9622b' }, merge: false });
  await pg.click('.hcw-c[data-c="#e0a030"]');
  assert.deepEqual(await last(pg), { look: { border: 'fire', colour: '#e0a030' }, merge: false });
  await pg.locator('input[data-any]').evaluate((el) => { el.value = '#123456'; el.dispatchEvent(new Event('input', { bubbles: true })); });
  assert.deepEqual(await last(pg), { look: { border: 'fire', colour: '#123456' }, merge: true });
  assert.equal((await calls(pg)).length, 3);
  const names = await pg.evaluate(() => [window.P.COLOURS.length, window.P.COLOURS[0], window.P.COLOURS[10]]);
  assert.deepEqual(names, [11, ['#b3243a', 'Crimson'], ['#8a8f98', 'Iron']]);
});

// --- The colours ---

test('the colours: none before a border, the note on a border that keeps its own tint, the heading text', { skip }, async (t) => {
  let { pg } = await open(t, { state: { look: null } });
  assert.equal(await pg.locator('.hcw-c').count(), 0);
  assert.equal(await pg.locator('.hcw-cg .hcw-lab').textContent(), 'Colour');
  assert.equal(await pg.locator('.hcw-cg .hcw-note').textContent(), 'Pick a border first; then it can take a colour.');
  ({ pg } = await open(t, { state: { look: { border: 'ice', colour: '#8fd8ff' } } }));
  assert.equal(await pg.locator('.hcw-c').count(), 13, 'own, eleven named, any');
  assert.equal(await pg.locator('.hcw-c--own[aria-pressed="true"]').count(), 1);
  assert.equal(await pg.locator('.hcw-cgh small').textContent(), 'Its own');
  assert.equal(await pg.locator('.hcw-note').textContent(), 'Ice keeps its own tint. Your colour shows in the card’s small label and its glow.');
  ({ pg } = await open(t, { state: { look: { border: 'fire', colour: '#e0a030' } } }));
  assert.equal(await pg.locator('.hcw-note').count(), 0, 'fire takes colour: no note');
  assert.equal(await pg.locator('.hcw-cgh small').textContent(), 'Gold');
  assert.deepEqual(await pg.evaluate(() => Array.from(document.querySelectorAll('.hcw-c[aria-pressed="true"]')).map((b) => b.getAttribute('data-c'))), ['#e0a030']);
  ({ pg } = await open(t, { state: { look: { border: 'fire', colour: '#123456' } } }));
  assert.equal(await pg.locator('.hcw-cgh small').textContent(), '#123456');
  assert.equal(await pg.locator('.hcw-c--any').getAttribute('data-set'), '');
  assert.equal(await pg.locator('.hcw-c[aria-pressed="true"]').count(), 0, 'a colour of the input’s own is not a named one');
});

test('the helpers: names, own colour, words, same', { skip }, async (t) => {
  const { pg } = await open(t, {});
  const r = await pg.evaluate(() => {
    const P = window.P;
    return [P.colourName('#E0A030'), P.colourName('#ABCDEF'), P.isOwn({ border: 'fire', colour: '#D9622B' }), P.isOwn({ border: 'fire', colour: '#d9622c' }), P.isOwn(null), P.isOwn({ border: 'nope', colour: '#d9622b' }),
      P.words(null), P.words({ border: 'rune-metal', colour: '#3fc8d4' }), P.words({ border: 'rune-metal', colour: '#e0a030' }), P.words({ border: 'rune-metal', colour: '#123456' }), P.words({ border: 'nope', colour: '#123456' }),
      P.same(null, null), P.same({ border: 'fire', colour: '#AABBCC' }, { border: 'fire', colour: '#aabbcc' }), P.same({ border: 'fire', colour: '#aabbcc' }, { border: 'ice', colour: '#aabbcc' }), P.same(null, { border: 'fire', colour: '#aabbcc' })];
  });
  assert.deepEqual(r, ['Gold', '#abcdef', true, false, false, false, 'No border', 'Rune metal', 'Rune metal, gold', 'Rune metal, #123456', 'No border', true, true, false, false]);
});

test('paint and sample put a look on an element and take it off again', { skip }, async (t) => {
  const { pg, errors } = await open(t, {});
  const r = await pg.evaluate(() => {
    const el = document.createElement('div'); document.body.appendChild(el);
    const get = () => ({ trim: el.getAttribute('data-trim'), k: el.style.getPropertyValue('--k'), img: el.style.getPropertyValue('--trim-img'), corner: el.style.getPropertyValue('--trim-corner'), ground: el.style.getPropertyValue('--trim-ground') });
    window.P.paint(el, { border: 'mechanical', colour: '#6fa8dc' });
    const a = get();
    window.P.paint(el, { border: 'ice', colour: '#8FD8FF' });
    const b = get();
    window.P.paint(el, null);
    const c = get();
    window.P.paint(el, { border: 'fire', colour: 'nope' });
    const d = get();
    return { a, b, c, d, s: window.P.sample({ border: 'ice', colour: '#8fd8ff' }), p: window.P.sample(null) };
  });
  assert.equal(r.a.trim, 'mechanical');
  assert.match(r.a.img, /borders\/mechanical\.webp/);
  assert.ok(!r.a.img.includes('&quot;'), 'the quotes are real quotes, not the attribute’s entities');
  assert.match(r.a.corner, /mechanical-corners\.webp/);
  assert.match(r.a.ground, /mechanical-steam\.webp/);
  assert.equal(r.b.trim, 'ice');
  assert.equal(r.b.k, '#8fd8ff');
  assert.equal(r.b.corner, '', 'the old border’s corner is gone');
  assert.equal(r.b.ground, '', 'and its ground');
  assert.deepEqual(r.c, { trim: null, k: '', img: '', corner: '', ground: '' });
  assert.equal(r.d.trim, 'fire');
  assert.equal(r.d.k, '#d9622b', 'a colour that is not hex becomes the border’s own');
  assert.match(r.s, /^<span class="hcw-sm"><span class="hc-gcard" data-trim="ice" style="--k:#8fd8ff;/);
  assert.equal(r.p, '<span class="hcw-sm"><span class="hc-gcard"></span></span>');
  assert.deepEqual(errors, []);
});

// --- sync ---

test('sync updates pressed states and samples without replacing the root', { skip }, async (t) => {
  const { pg, errors } = await open(t, { state: { look: { border: 'fire', colour: '#d9622b' }, sys: { border: 'ice', colour: '#8fd8ff' } } });
  await pg.evaluate(() => { document.querySelector('.hcw-pk').__mine = 1; document.querySelector('.hcw-t[data-b="vine"]').__mine = 1; document.querySelector('[data-wall]').__mine = 1; });
  await pg.evaluate(() => { window.S.look = { border: 'vine', colour: '#445566' }; window.P.sync(document.querySelector('.hcw-pk'), window.S); });
  assert.deepEqual(await pressed(pg), ['vine']);
  const k = await tileK(pg);
  for (const key of KEYS) assert.equal(k[key], '#445566', key);
  const kept = await pg.evaluate(() => [document.querySelector('.hcw-pk').__mine, document.querySelector('.hcw-t[data-b="vine"]').__mine, document.querySelector('[data-wall]').__mine, document.querySelectorAll('.hcw-pk').length]);
  assert.deepEqual(kept, [1, 1, 1, 1]);
  // The colours follow: another border redraws them, and back to none shows the line.
  assert.equal(await pg.locator('.hcw-cgh small').textContent(), '#445566');
  assert.equal(await pg.locator('.hcw-note').count(), 0);
  await pg.evaluate(() => { window.S.look = { border: 'glass', colour: '#58c4d8' }; window.P.sync(document.querySelector('.hcw-pk'), window.S); });
  assert.equal(await pg.locator('.hcw-note').count(), 1, 'glass keeps its own tint');
  assert.equal(await pg.locator('.hcw-cgh small').textContent(), 'Its own');
  await pg.evaluate(() => { window.S.look = { border: 'ice', colour: '#8fd8ff' }; window.P.sync(document.querySelector('.hcw-pk'), window.S); });
  assert.deepEqual((await pressed(pg)).sort(), ['@sys', 'ice']);
  await pg.evaluate(() => { window.S.look = null; window.P.sync(document.querySelector('.hcw-pk'), window.S); });
  assert.deepEqual(await pressed(pg), ['']);
  assert.equal(await pg.locator('.hcw-c').count(), 0);
  assert.equal(await pg.locator('.hcw-cg .hcw-note').textContent(), 'Pick a border first; then it can take a colour.');
  assert.equal(Object.values(await tileK(pg)).filter(Boolean).length, 17, 'the sixteen and the system\u2019s');
  assert.deepEqual(errors, []);
});

test('sync keeps the colour input while it is focused, and the keyboard where it was', { skip }, async (t) => {
  const { pg, errors } = await open(t, { state: { look: { border: 'fire', colour: '#112233' } } });
  await pg.evaluate(() => { const i = document.querySelector('input[data-any]'); i.__mine = 1; i.parentNode.__mine = 1; document.querySelector('[data-cg]').__mine = 1; i.focus(); });
  await pg.evaluate(() => { window.S.look = { border: 'fire', colour: '#445566' }; window.P.sync(document.querySelector('.hcw-pk'), window.S); });
  const r = await pg.evaluate(() => {
    const i = document.querySelector('input[data-any]');
    return { same: i.__mine === 1 && i.parentNode.__mine === 1 && document.querySelector('[data-cg]').__mine === 1, focused: document.activeElement === i, head: document.querySelector('[data-cname]').textContent,
      c: i.parentNode.style.getPropertyValue('--c'), set: i.parentNode.hasAttribute('data-set'), pressed: document.querySelectorAll('.hcw-c[aria-pressed="true"]').length };
  });
  assert.deepEqual(r, { same: true, focused: true, head: '#445566', c: '#445566', set: true, pressed: 0 });
  // Pressing a named swatch from the keyboard redraws nothing it needs and keeps the focus on it.
  await pg.evaluate(() => { window.APPLY = true; });
  await pg.focus('.hcw-c[data-c="#6fae4f"]');
  await pg.keyboard.press('Enter');
  assert.equal(await pg.evaluate(() => document.activeElement && document.activeElement.getAttribute('data-c')), '#6fae4f');
  assert.deepEqual(await pg.evaluate(() => Array.from(document.querySelectorAll('.hcw-c[aria-pressed="true"]')).map((b) => b.getAttribute('data-c'))), ['#6fae4f']);
  // A new border draws the colours again and a swatch that had the keyboard keeps it.
  await pg.focus('.hcw-c[data-c="#6fae4f"]');
  await pg.evaluate(() => { window.S.look = { border: 'ice', colour: '#6fae4f' }; window.P.sync(document.querySelector('.hcw-pk'), window.S); });
  assert.equal(await pg.evaluate(() => document.activeElement && document.activeElement.getAttribute('data-c')), '#6fae4f');
  assert.deepEqual(errors, []);
});

// --- Hostile data ---

test('a colour or key that is not the library’s never reaches the markup', { skip }, async (t) => {
  const evil = ['red;background:url(x)', '"><img src=x onerror=1>', '#12345', "#123456;x:'", 'javascript:1'];
  const { pg, errors } = await open(t, {});
  for (const bad of evil) {
    const r = await pg.evaluate((bad) => {
      const st = { look: { border: 'fire', colour: bad }, sys: { border: 'fire', colour: bad } };
      const h = window.P.html(st, { lay: 'field', top: true, split: true });
      const box = document.createElement('div'); box.innerHTML = h; document.body.appendChild(box);
      const out = { h, imgs: box.querySelectorAll('img').length, bad: Array.from(box.querySelectorAll('[style]')).map((e) => e.getAttribute('style')).filter((x) => !/^(--c:#[0-9a-f]{6}|--k:#[0-9a-f]{6};--trim-img:url\("[\w\-./?=%:]+"\)(;--trim-corner:url\("[\w\-./?=%:]+"\))?(;--trim-ground:url\("[\w\-./?=%:]+"\))?)$/.test(x)),
        pressed: Array.from(box.querySelectorAll('.hcw-t[aria-pressed="true"]')).map((b) => b.getAttribute('data-b')), colours: Array.from(box.querySelectorAll('.hcw-c[aria-pressed="true"]')).map((b) => b.getAttribute('data-c')), val: box.querySelector('input[data-any]').value };
      box.remove();
      return out;
    }, bad);
    assert.ok(!r.h.includes(bad), bad + ' is in the markup');
    assert.ok(!r.h.includes('onerror'), bad);
    assert.equal(r.imgs, 0, bad + ' made an element');
    assert.deepEqual(r.bad, [], bad + ' reached a style attribute');
    assert.deepEqual(r.pressed.sort(), ['@sys', 'fire']);
    assert.deepEqual(r.colours, ['']);
    assert.equal(r.val, '#d9622b', 'the border’s own colour stands in');
  }
  assert.deepEqual(errors, []);
});

test('a border key that is not in the library draws no pressed sample and no error; a hostile name is escaped', { skip }, async (t) => {
  let { pg, errors } = await open(t, { state: { look: { border: 'lava', colour: '#ff0000' } } });
  assert.deepEqual(await pressed(pg), []);
  assert.equal(await pg.locator('.hcw-t').count(), 17);
  assert.equal(await pg.locator('.hcw-c').count(), 0);
  await pg.evaluate(() => window.P.sync(document.querySelector('.hcw-pk'), window.S));
  assert.deepEqual(await pressed(pg), []);
  assert.deepEqual(errors, []);
  const evilPick = JSON.parse(JSON.stringify(PICK));
  evilPick.borders.fire.name = '<b id="x">Fire</b><img src=x onerror=1>';
  evilPick.borders.ice.colour = 'red;background:url(x)';
  evilPick.groups[0].name = '<i>Elements</i>';
  evilPick.groups[0].keys.push('"><script>1</script>');
  ({ pg, errors } = await open(t, { pick: evilPick, state: { look: { border: 'fire', colour: '#d9622b' } } }));
  assert.equal(await pg.locator('#x, .hcw-pk img, .hcw-pk i, .hcw-pk script, .hcw-pk b').count(), 0);
  assert.equal(await pg.locator('.hcw-t[data-b="fire"] .hcw-tn').textContent(), '<b id="x">Fire</b><img src=x onerror=1>');
  assert.equal(await pg.locator('.hcw-t').count(), 17, 'a listed key that is no border is left out');
  await pg.click('.hcw-t[data-b="ice"]');
  assert.equal((await last(pg)).look.border, 'ice');
  assert.match((await last(pg)).look.colour, /^#[0-9a-f]{6}$/);
  assert.deepEqual(errors, []);
});

// --- Sizes ---

for (const dark of [false, true]) {
  test(`field layout at 360 wide${dark ? ' (dark)' : ''}: every sample and swatch is at least 44 by 44`, { skip }, async (t) => {
    const { pg } = await open(t, { width: 360, dark, state: { look: { border: 'fire', colour: '#e0a030' } }, opts: { lay: 'field', top: true }, hostStyle: 'padding:0 16px' });
    const boxes = await pg.evaluate(() => Array.from(document.querySelectorAll('.hcw-t, .hcw-c')).map((e) => { const r = e.getBoundingClientRect(); return [e.className, Math.round(r.width * 10) / 10, Math.round(r.height * 10) / 10]; }));
    assert.equal(boxes.length, 17 + 13);
    for (const [c, w, h] of boxes) assert.ok(w >= 44 && h >= 44, `${c} is ${w} by ${h}`);
    assert.ok(await pg.evaluate(() => document.documentElement.scrollWidth <= 360), 'no sideways scroll');
    assert.deepEqual(await pg.evaluate(() => Array.from(document.querySelectorAll('.hcw-tn')).filter((n) => n.scrollWidth > n.clientWidth).map((n) => n.textContent)), [], 'a form shows whole names');
    assert.equal(await pg.evaluate(() => document.querySelector('.hcw-pk').firstElementChild.hasAttribute('data-cg')), true, 'top: the colours come first');
  });
}

for (const width of [360, 1100]) {
  test(`wall layout in a 360 wide panel (page ${width} wide): no sideways scroll, names whole`, { skip }, async (t) => {
    const { pg } = await open(t, { width, state: { look: { border: 'fire', colour: '#d9622b' } }, opts: { lay: 'wall' }, hostStyle: 'width:360px;padding:14px 16px' });
    const r = await pg.evaluate(() => ({
      page: document.documentElement.scrollWidth, host: document.getElementById('host').scrollWidth, hostW: document.getElementById('host').clientWidth,
      cut: Array.from(document.querySelectorAll('.hcw-tn')).filter((n) => n.scrollWidth > n.clientWidth).map((n) => n.textContent + ' ' + n.scrollWidth + '>' + n.clientWidth),
      out: Array.from(document.querySelectorAll('.hcw-t, .hcw-c, .hcw-g h4')).filter((e) => e.getBoundingClientRect().right > 360.5).length,
      hs: Array.from(document.querySelectorAll('.hcw-g h4')).filter((h) => h.scrollWidth > h.clientWidth).length,
    }));
    assert.ok(r.page <= width, 'the page does not scroll sideways');
    assert.ok(r.host <= r.hostW, 'nor the panel');
    assert.deepEqual(r.cut, [], 'no name is cut');
    assert.equal(r.out, 0);
    assert.equal(r.hs, 0);
  });
}

test('on a phone the wall’s targets are a finger’s', { skip }, async (t) => {
  const { pg } = await open(t, { width: 360, state: { look: { border: 'fire', colour: '#d9622b' } }, opts: { lay: 'wall', split: true, top: true }, hostStyle: 'height:600px;display:flex;flex-direction:column' });
  const r = await pg.evaluate(() => {
    const sw = Array.from(document.querySelectorAll('.hcw-c')).map((e) => e.getBoundingClientRect()), wall = document.querySelector('[data-wall]'), cs = document.querySelector('.hcw-cs');
    return { sw: sw.map((b) => [b.width, b.height]), wallScrolls: wall.scrollHeight > wall.clientHeight, wallOverflow: getComputedStyle(wall).overflowY, csOverflow: getComputedStyle(cs).overflowX, head: getComputedStyle(document.querySelector('.hcw-cgh')).display, page: document.documentElement.scrollWidth };
  });
  assert.ok(r.sw.every(([w, h]) => w === 44 && h === 44));
  assert.equal(r.wallOverflow, 'auto');
  assert.ok(r.wallScrolls, 'the wall scrolls by itself');
  assert.equal(r.csOverflow, 'auto', 'the colours are one row that scrolls sideways');
  assert.equal(r.head, 'none');
  assert.ok(r.page <= 360);
});

// --- The keyboard ---

test('keyboard: Tab reaches every sample and swatch in order; Enter and Space press; the focus ring shows', { skip }, async (t) => {
  const { pg } = await open(t, { state: { look: { border: 'fire', colour: '#e0a030' }, sys: null }, opts: { lay: 'wall' } });
  const want = await pg.evaluate(() => Array.from(document.querySelectorAll('.hcw-t, .hcw-c[data-c], input[data-any]')).map((e) => e.getAttribute('data-b') !== null ? 'b:' + e.getAttribute('data-b') : e.getAttribute('data-c') !== null ? 'c:' + e.getAttribute('data-c') : 'any'));
  assert.equal(want.length, 18 + 12 + 1);
  await pg.focus('#before');
  const got = [], rings = [];
  for (let i = 0; i < want.length; i++) {
    await pg.keyboard.press('Tab');
    got.push(await pg.evaluate(() => { const e = document.activeElement; return e.getAttribute('data-b') !== null ? 'b:' + e.getAttribute('data-b') : e.getAttribute('data-c') !== null ? 'c:' + e.getAttribute('data-c') : e.hasAttribute('data-any') ? 'any' : e.id || e.tagName; }));
    if (i === 3 || i === 20 || i === want.length - 1) rings.push(await pg.evaluate(() => { const e = document.activeElement, s = getComputedStyle(e.matches('input') ? e.parentNode : e); return [s.outlineStyle, parseFloat(s.outlineWidth)]; }));
  }
  assert.deepEqual(got, want);
  for (const [style, w] of rings) assert.ok(style !== 'none' && w >= 2, `outline ${style} ${w}`);
  await pg.keyboard.press('Tab');
  assert.equal(await pg.evaluate(() => document.activeElement.id), 'after');
  const n = (await calls(pg)).length;
  await pg.focus('.hcw-t[data-b="storm"]');
  await pg.keyboard.press('Enter');
  assert.deepEqual((await last(pg)).look, { border: 'storm', colour: '#e0a030' });
  await pg.focus('.hcw-t[data-b="ice"]');
  await pg.keyboard.press('Space');
  assert.deepEqual((await last(pg)).look, { border: 'ice', colour: '#e0a030' });
  await pg.focus('.hcw-c[data-c="#4aa3e0"]');
  await pg.keyboard.press('Space');
  assert.deepEqual((await last(pg)).look, { border: 'fire', colour: '#4aa3e0' });
  assert.equal((await calls(pg)).length, n + 3);
});

// --- Light and dark ---

// Text colour against what is behind it: the element's own and its parents' backgrounds laid over each other.
const CONTRAST = () => {
  const parse = (s) => {
    const m = s.match(/rgba?\(([^)]+)\)/);
    if (!m) return null;
    const p = m[1].split(/[ ,/]+/).filter(Boolean).map(Number);
    return [p[0], p[1], p[2], p.length > 3 ? p[3] : 1];
  };
  const lum = (c) => { const v = c.map((x) => { x /= 255; return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4); }); return 0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2]; };
  const behind = (el) => {
    const layers = [];
    for (let e = el; e; e = e.parentElement) { const c = parse(getComputedStyle(e).backgroundColor); if (c && c[3] > 0) layers.push(c); if (c && c[3] === 1) break; }
    let base = [255, 255, 255];
    layers.reverse().forEach((c) => { base = base.map((b, i) => b * (1 - c[3]) + c[i] * c[3]); });
    return base;
  };
  return Array.from(document.querySelectorAll('.hcw-tn, .hcw-g h4, .hcw-lab, .hcw-cgh small, .hcw-note')).map((el) => {
    const fg = parse(getComputedStyle(el).color), bg = behind(el), l1 = lum(fg), l2 = lum(bg);
    return { what: el.className + ' ' + el.textContent.slice(0, 20), ratio: Math.round(((Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05)) * 100) / 100 };
  });
};
for (const dark of [false, true]) {
  for (const lay of ['wall', 'field']) {
    test(`${lay}, ${dark ? 'dark' : 'light'}: sample names, headings and notes reach 4.5:1`, { skip }, async (t) => {
      const { pg } = await open(t, { dark, width: 360, state: { look: { border: 'ice', colour: '#112233' }, sys: { border: 'fire', colour: '#d9622b' } }, opts: { lay, sysLabel: 'Draw Steel’s' } });
      await pg.waitForTimeout(250);
      const rs = await pg.evaluate(CONTRAST);
      assert.ok(rs.length >= 25);
      for (const r of rs) assert.ok(r.ratio >= 4.5, `${r.what}: ${r.ratio}`);
      assert.ok(rs.some((r) => /hcw-g/.test(r.what) === false && /Ice/.test(r.what)), 'the pressed name was measured');
      assert.ok(rs.some((r) => /Pick|keeps/.test(r.what)));
    });
  }
}

// --- bind ---

test('bind: the function it returns removes the listeners; two pickers share one set', { skip }, async (t) => {
  const { pg } = await open(t, { state: { look: { border: 'fire', colour: '#d9622b' } } });
  // A second picker in the same container answers to the same listeners.
  await pg.evaluate(() => { host.insertAdjacentHTML('beforeend', window.P.html({ look: null }, { lay: 'field' })); });
  await pg.locator('.hcw-pk').nth(1).locator('.hcw-t[data-b="vine"]').click();
  assert.equal((await calls(pg)).length, 1);
  assert.equal(await pg.evaluate(() => window.calls[0].look.border), 'vine');
  await pg.evaluate(() => window.off());
  await pg.click('.hcw-t[data-b="ice"]');
  await pg.click('.hcw-c--own');
  await pg.locator('input[data-any]').evaluate((el) => { el.value = '#123456'; el.dispatchEvent(new Event('input', { bubbles: true })); });
  assert.equal((await calls(pg)).length, 1, 'a press after it does nothing');
});

// --- The Go list and this table agree ---

test('the test table matches the Go library’s names, colours and groups', () => {
  const src = readFileSync(join(root, 'internal/plugins/entities/cardborders/cardborders.go'), 'utf8');
  for (const k of KEYS) {
    const line = src.split('\n').find((l) => l.includes(`{Key: "${k}",`));
    assert.ok(line, 'Go lacks ' + k);
    const group = GROUPS.find((g) => g.keys.includes(k)).name;
    assert.ok(line.includes(`Name: "${INFO[k][0]}", Colour: "${INFO[k][1]}", Group: "${group}"`), k + ' differs: ' + line);
  }
  for (const g of GROUPS) {
    const lit = `{"${g.name}", []string{${g.keys.map((k) => `"${k}"`).join(', ')}}}`;
    assert.ok(src.includes(lit), 'Go group ' + g.name);
  }
  assert.equal(GROUPS.flatMap((g) => g.keys).length, 16);
  for (const f of Object.values(LIB).flatMap((v) => URLS.map((p) => v[p]).filter(Boolean))) assert.ok(existsSync(join(STATIC, 'img/borders', f)), f);
});

// --- Pictures for review ---

test('screenshots for review', { skip: skip || !process.env.PICKER_SHOTS }, async (t) => {
  const dir = process.env.PICKER_SHOTS;
  mkdirSync(dir, { recursive: true });
  const panel = 'width:360px;height:700px;display:flex;flex-direction:column;background:var(--color-bg-secondary);border:1px solid var(--color-border)';
  let { pg } = await open(t, { width: 420, height: 740, hostStyle: panel, state: { look: { border: 'rune-metal', colour: '#e0a030' }, sys: { border: 'rune-metal', colour: '#3fc8d4' } }, opts: { lay: 'wall', split: true, sysLabel: 'Draw Steel’s' } });
  await pg.waitForTimeout(1500);
  await pg.screenshot({ path: join(dir, 'picker-wall-360-light.png') });
  ({ pg } = await open(t, { width: 360, height: 1500, dark: true, hostStyle: 'padding:16px', state: { look: { border: 'rune-metal', colour: '#e0a030' } }, opts: { lay: 'field', top: true } }));
  await pg.waitForTimeout(1500);
  await pg.screenshot({ path: join(dir, 'picker-field-360-dark.png'), fullPage: true });
});
