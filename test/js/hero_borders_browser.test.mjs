// hero_borders_browser.test.mjs — the card borders of the hero creator's stage look.
//
// Runs hero_borders.js, hero_stage.js and hero_creator.js in a real Chromium
// against a stubbed plan whose entries carry a look ({ border, colour }), the
// mount carrying data-look="stage" and the library as data-borders, with the
// real pictures served. It pins what a bordered card wears (data-trim, --k,
// --trim-img, the nine-slice numbers), the picture the colouring makes (a
// blob: for a border with a mask, the served picture for one without, the
// corner, the dyed scales), that one colour of one border is made once, what
// stays plain (an unknown border, a bad colour, no look), that the classic
// creator never draws a border, that the two properties never show as facts,
// that the small label reads against the real pixels behind it, that the foot
// line stays inside the frame's padding, no sideways scroll, that destroy()
// revokes what was made, and that a picture that fails leaves the plain one.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const C = '/campaigns/c1';
const DRAFT = 'chronicle:hero-draft:c1';
const STATIC = join(root, 'internal/plugins/entities/static');
const file = (rel) => readFileSync(join(STATIC, rel));

// The library as the page hands it over. The Go package is the source of
// truth; the test below that reads its source keeps this table honest.
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
// patch: { key: { part: address } } replaces a served address, to hand the script one it must refuse.
const libJSON = (patch = {}) => JSON.stringify(Object.fromEntries(Object.entries(LIB).map(([k, v]) => [k, { ...Object.fromEntries(Object.entries(v).map(([p, x]) => [p, URLS.includes(p) ? '/borders/' + x + '?v=abc123' : x])), ...(patch[k] || {}) }])));
const DEFAULT = { 'carved-stone': '#e0a030', fire: '#d9622b', 'great-rune': '#b48cf2', mechanical: '#6fa8dc', scales: '#2fb5a4', 'rune-metal': '#3fc8d4', vine: '#6fae4f', ice: '#8fd8ff' };

const ch = (name, look, props = {}) => ({ key: name.toLowerCase(), slug: name.toLowerCase(), name, summary: name + ' summary, a line or two of words.', source: 'package', properties: props, ...(look ? { look } : {}) });
const KEYS = Object.keys(LIB);
const plan = (choices) => () => ({ systemName: 'Draw Steel', steps: [{ fieldKey: 'ancestry', label: 'Ancestry', choices }, { fieldKey: 'class', label: 'Class', choices: [ch('Fury'), ch('Censor')] }] });
const colourOf = (k, col) => col || DEFAULT[k] || '#6fa8dc';
const every = (col) => plan(KEYS.map((k) => ch('B ' + k, { border: k, colour: colourOf(k, col) }, { speed: 5 })));
const MAIN = plan([
  ch('Fire', { border: 'fire', colour: '#d9622b' }, { speed: 5 }), ch('Rune', { border: 'rune-metal', colour: '#3fc8d4' }), ch('Great', { border: 'great-rune', colour: '#b48cf2' }),
  ch('Stone', { border: 'carved-stone', colour: '#e0a030' }), ch('Scale', { border: 'scales', colour: '#2fb5a4' }), ch('Frost', { border: 'ice', colour: '#8fd8ff' }),
  ch('Lava', { border: 'lava', colour: '#ff0000' }), ch('Red', { border: 'fire', colour: 'red' }), ch('Short', { border: 'fire', colour: '#f00' }), ch('Plain'),
]);

const LIGHT = ':root{--color-bg-primary:#f9fafb;--color-card-bg:#fff;--color-bg-tertiary:#f3f4f6;--color-border:#e5e7eb;--color-text-primary:#111827;--color-text-secondary:#4b5563;--dur-micro:120ms;--dur-standard:200ms;--dur-large:280ms;--ease-out:cubic-bezier(.16,1,.3,1);--z-popup:50;--z-sheet:60}';
const DARK = 'html.dark{--color-bg-primary:#111827;--color-card-bg:#1f2937;--color-bg-tertiary:#374151;--color-border:#374151;--color-text-primary:#f9fafb;--color-text-secondary:#c4c9d2}';

function page({ accent = '#6366f1', look = true, dark = false, borders = true, patch }) {
  return `<!doctype html><html data-gm-term="Director"${dark ? ' class="dark"' : ''}><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>${LIGHT}${DARK}:root{--color-accent:${accent}}*,::before,::after{box-sizing:border-box}body{margin:0;font-family:sans-serif}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style>
<link rel="stylesheet" href="/hero_creator.css">${look ? '<link rel="stylesheet" href="/hero_stage.css"><link rel="stylesheet" href="/hero_borders.css">' : ''}</head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true"${look ? ' data-look="stage"' : ''}${borders ? " data-borders='" + libJSON(patch) + "'" : ''}><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { register: (n, d) => { window.hcImpl = d; document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)); }, escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director' };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_borders.js"></script><script src="/hero_stage.js"></script><script src="/hero_creator.js"></script></body></html>`;
}

async function open(t, o = {}) {
  const { width = 390, height = 844, p = MAIN, draft = null, failPictures = [], mouse = false } = o;
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport: { width, height }, reducedMotion: 'reduce', hasTouch: !mouse && width < 900, isMobile: false });
  const pg = await ctx.newPage();
  const errors = [];
  pg.on('pageerror', (e) => errors.push(String(e)));
  await pg.addInitScript(({ draft, key }) => {
    if (draft) localStorage.setItem(key, JSON.stringify(draft));
    // What the colouring makes and gives back, and how many pictures it draws.
    window.__made = 0; window.__revoked = 0; window.__blobs = 0;
    const mkUrl = URL.createObjectURL.bind(URL), rev = URL.revokeObjectURL.bind(URL), toBlob = HTMLCanvasElement.prototype.toBlob;
    URL.createObjectURL = (b) => { window.__made++; return mkUrl(b); };
    URL.revokeObjectURL = (u) => { window.__revoked++; return rev(u); };
    HTMLCanvasElement.prototype.toBlob = function (...a) { window.__blobs++; return toBlob.apply(this, a); };
  }, { draft, key: DRAFT });
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), u = new URL(req.url()), path = u.pathname;
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    const asset = { '/hero_creator.js': 'js/hero_creator.js', '/hero_stage.js': 'js/hero_stage.js', '/hero_borders.js': 'js/hero_borders.js', '/hero_rules_lines.js': 'js/hero_rules_lines.js' }[path];
    if (asset) return route.fulfill({ contentType: 'text/javascript', body: file(asset) });
    const css = { '/hero_creator.css': 'css/hero_creator.css', '/hero_stage.css': 'css/hero_stage.css', '/hero_borders.css': 'css/hero_borders.css' }[path];
    if (css) return route.fulfill({ contentType: 'text/css', body: file(css) });
    if (path.startsWith('/borders/')) {
      const name = path.slice('/borders/'.length);
      if (failPictures.includes(name)) return route.abort();
      return route.fulfill({ contentType: 'image/webp', body: file('img/borders/' + name) });
    }
    if (path === C + '/characters/new/plan') return ok(p());
    if (path.startsWith(C + '/entities/') || path === C + '/characters') return route.fulfill({ contentType: 'text/html', body: '<p>there</p>' });
    return route.fulfill({ contentType: 'text/html', body: page(o) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  return { pg, errors };
}

// Every card of an entry in the step on screen, on a phone or a computer.
const cards = (desk) => (desk ? '.hc-gcard[data-i]' : '.hc-hand .hc-card[data-i]');
const idle = (pg, ms = 150) => pg.waitForTimeout(ms);
const trimOf = (pg, sel) => pg.evaluate((s) => Array.from(document.querySelectorAll(s)).map((c) => ({
  name: c.querySelector('h3').textContent, trim: c.getAttribute('data-trim'), k: c.style.getPropertyValue('--k').trim(),
  img: c.style.getPropertyValue('--trim-img'), corner: c.style.getPropertyValue('--trim-corner'),
})), sel);
// Waits until the card's picture (or corner) is a made one.
const madeOn = (pg, desk, name, prop) => pg.waitForFunction(({ sel, name, prop }) => {
  const c = Array.from(document.querySelectorAll(sel)).find((x) => x.querySelector('h3').textContent === name);
  return !!c && c.style.getPropertyValue(prop).includes('blob:');
}, { sel: cards(desk), name, prop }, { timeout: 8000 });

const sizes = [['phone', 390, false], ['computer', 1280, true]];

for (const [label, width, desk] of sizes) {
  test(`${label}: an entry with a look wears its border`, { skip }, async (t) => {
    const { pg, errors } = await open(t, { width });
    const all = await trimOf(pg, cards(desk));
    const fire = all.find((c) => c.name === 'Fire');
    assert.equal(fire.trim, 'fire');
    assert.equal(fire.k, '#d9622b');
    assert.match(fire.img, /^url\("\/borders\/fire\.webp\?v=abc123"\)$|^url\("blob:/);
    const sel = `${cards(desk)}[data-trim="fire"]`;
    const s = await pg.locator(sel).first().evaluate((e) => { const c = getComputedStyle(e); return { slice: c.borderImageSlice, repeat: c.borderImageRepeat, outset: c.borderImageOutset, width: c.borderImageWidth, over: c.overflow }; });
    assert.equal(s.slice, '132');
    assert.equal(s.repeat, 'round');
    assert.equal(s.outset, '5px');
    assert.equal(s.width, desk ? '30px' : '44px');
    assert.equal(s.over, 'visible');
    const g = await pg.locator(`${cards(desk)}[data-trim="great-rune"]`).first().evaluate((e) => getComputedStyle(e).borderImageRepeat);
    assert.equal(g, 'stretch');
    assert.equal(errors.length, 0, errors.join('\n'));
  });

  test(`${label}: a border with a mask is made into a blob, one without keeps the served picture`, { skip }, async (t) => {
    const { pg, errors } = await open(t, { width });
    await madeOn(pg, desk, 'Fire', '--trim-img');
    await madeOn(pg, desk, 'Rune', '--trim-img');
    await madeOn(pg, desk, 'Great', '--trim-img');
    const by = Object.fromEntries((await trimOf(pg, cards(desk))).map((c) => [c.name, c]));
    for (const n of ['Fire', 'Rune', 'Great']) assert.match(by[n].img, /^url\("blob:/, n + ' is coloured');
    assert.match(by.Frost.img, /^url\("\/borders\/ice\.webp\?v=abc123"\)$/, 'ice has no mask, so it keeps the picture');
    // Carved stone: the ring is the served earth picture, the corner is made.
    await madeOn(pg, desk, 'Stone', '--trim-corner');
    const stone = Object.fromEntries((await trimOf(pg, cards(desk))).map((c) => [c.name, c])).Stone;
    assert.match(stone.img, /^url\("\/borders\/earth\.webp\?v=abc123"\)$/);
    assert.match(stone.corner, /^url\("blob:/);
    // Scales are dyed whole.
    await madeOn(pg, desk, 'Scale', '--trim-img');
    assert.match((await trimOf(pg, cards(desk))).find((c) => c.name === 'Scale').img, /^url\("blob:/);
    assert.equal(errors.length, 0, errors.join('\n'));
  });

  test(`${label}: an unknown border, a bad colour and no look are plain or take the default`, { skip }, async (t) => {
    const { pg } = await open(t, { width });
    await idle(pg);
    const by = Object.fromEntries((await trimOf(pg, cards(desk))).map((c) => [c.name, c]));
    assert.equal(by.Lava.trim, null, 'an unknown border is a plain card');
    assert.equal(by.Plain.trim, null, 'no look is a plain card');
    assert.equal(by.Red.trim, null, 'a colour that is not six hex digits leaves the card plain');
    assert.equal(by.Short.trim, null);
    for (const n of ['Lava', 'Plain', 'Red', 'Short']) assert.equal(by[n].img, '', n + ' has no border picture');
    const plain = await pg.evaluate((sel) => getComputedStyle(Array.from(document.querySelectorAll(sel)).find((x) => x.querySelector('h3').textContent === 'Plain')).borderImageSource, cards(desk));
    assert.equal(plain, 'none');
  });

  test(`${label}: the classic creator never writes data-trim, even when the plan has looks`, { skip }, async (t) => {
    const { pg } = await open(t, { width, look: false });
    await idle(pg);
    assert.equal(await pg.locator('.hc').getAttribute('data-look'), null);
    assert.ok((await pg.locator(cards(desk)).count()) >= 9);
    assert.equal(await pg.locator('[data-trim]').count(), 0);
    assert.equal(await pg.locator('[style*="--trim"]').count(), 0);
    assert.equal(await pg.evaluate(() => window.__blobs), 0, 'nothing is coloured');
  });

  test(`${label}: card_border and card_colour never show as facts`, { skip }, async (t) => {
    const { pg } = await open(t, { width });
    await idle(pg);
    const body = await pg.locator('.hc').innerText();
    assert.doesNotMatch(body, /card[ _]border|card[ _]colour/i);
    const cl = await open(t, { width, look: false });
    await idle(cl.pg);
    assert.doesNotMatch(await cl.pg.locator('.hc').innerText(), /card[ _]border|card[ _]colour/i);
    // The entry's sheet lists its facts too.
    const first = pg.locator(cards(desk)).first();
    await first.click();
    if (!desk && !(await pg.locator('.hc-sheet').count())) await first.click();
    await idle(pg, 300);
    assert.doesNotMatch(await pg.locator('.hc').innerText(), /card[ _]border|card[ _]colour/i);
  });

  test(`${label}: one colouring job for two cards of the same border and colour`, { skip }, async (t) => {
    const same = plan([ch('A', { border: 'fire', colour: '#d9622b' }), ch('B', { border: 'fire', colour: '#d9622b' }), ch('C', { border: 'fire', colour: '#D9622B' })]);
    const { pg } = await open(t, { width, p: same });
    await madeOn(pg, desk, 'A', '--trim-img');
    await madeOn(pg, desk, 'B', '--trim-img');
    await madeOn(pg, desk, 'C', '--trim-img');
    await idle(pg);
    assert.equal(await pg.evaluate(() => window.__blobs), 1, 'one picture is drawn');
    assert.equal(await pg.evaluate(() => window.__made), 1);
    const imgs = (await trimOf(pg, cards(desk))).filter((c) => c.trim).map((c) => c.img);
    assert.equal(new Set(imgs).size, 1, 'and all three wear it');
    const diff = plan([ch('A', { border: 'fire', colour: '#d9622b' }), ch('B', { border: 'fire', colour: '#112233' })]);
    const two = await open(t, { width, p: diff });
    await madeOn(two.pg, desk, 'B', '--trim-img');
    await madeOn(two.pg, desk, 'A', '--trim-img');
    assert.equal(await two.pg.evaluate(() => window.__blobs), 2, 'another colour is another job');
  });

  test(`${label}: destroy() revokes what it made`, { skip }, async (t) => {
    const { pg } = await open(t, { width });
    await madeOn(pg, desk, 'Stone', '--trim-corner');
    await madeOn(pg, desk, 'Scale', '--trim-img');
    await pg.waitForFunction(() => window.__made >= 5, null, { timeout: 8000 });
    await idle(pg, 300);
    const made = await pg.evaluate(() => window.__made);
    assert.ok(made >= 5, 'fire, rune, great, the stone corner and the scales: ' + made);
    assert.equal(await pg.evaluate(() => window.__revoked), 0, 'nothing is given back while the page is in use');
    await pg.evaluate(() => window.hcImpl.destroy(document.querySelector('.hc')));
    assert.equal(await pg.evaluate(() => window.__revoked), made, 'each made picture is revoked once');
  });

  test(`${label}: a picture that fails to load leaves the plain border and throws nothing`, { skip }, async (t) => {
    const { pg, errors } = await open(t, { width, failPictures: ['fire-mask.webp', 'scales.webp'] });
    await madeOn(pg, desk, 'Rune', '--trim-img');
    await idle(pg, 300);
    const by = Object.fromEntries((await trimOf(pg, cards(desk))).map((c) => [c.name, c]));
    assert.match(by.Fire.img, /^url\("\/borders\/fire\.webp\?v=abc123"\)$/, 'the plain picture stays');
    assert.equal(by.Fire.trim, 'fire');
    assert.match(by.Scale.img, /^url\("\/borders\/scales\.webp/);
    assert.match(by.Rune.img, /^url\("blob:/, 'the others are not held up');
    assert.deepEqual(errors, []);
  });
}

// --- Without the library or the script ---

test('a missing or unreadable library, or no script, means plain cards and no error', { skip }, async (t) => {
  const none = await open(t, { borders: false });
  await idle(none.pg);
  assert.equal(await none.pg.locator('[data-trim]').count(), 0);
  assert.deepEqual(none.errors, []);
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  for (const mode of ['broken', 'noscript']) {
    const pg = await (await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' })).newPage();
    const errs = [];
    pg.on('pageerror', (e) => errs.push(String(e)));
    await pg.route('http://t.test/**', (route) => {
      const path = new URL(route.request().url()).pathname;
      const js = { '/hero_creator.js': 'js/hero_creator.js', '/hero_stage.js': 'js/hero_stage.js', '/hero_rules_lines.js': 'js/hero_rules_lines.js' }[path] || (mode === 'broken' && path === '/hero_borders.js' ? 'js/hero_borders.js' : null);
      if (js) return route.fulfill({ contentType: 'text/javascript', body: file(js) });
      if (path === '/hero_borders.js') return route.fulfill({ contentType: 'text/javascript', body: '' });
      const css = { '/hero_creator.css': 'css/hero_creator.css', '/hero_stage.css': 'css/hero_stage.css', '/hero_borders.css': 'css/hero_borders.css' }[path];
      if (css) return route.fulfill({ contentType: 'text/css', body: file(css) });
      if (path === C + '/characters/new/plan') return route.fulfill({ contentType: 'application/json', body: JSON.stringify(MAIN()) });
      let html = page({});
      if (mode === 'broken') html = html.replace(/data-borders='[^']*'/, "data-borders='{not json'");
      return route.fulfill({ contentType: 'text/html', body: html });
    });
    await pg.goto('http://t.test/');
    await pg.waitForSelector('.hc-app');
    assert.equal(await pg.locator('[data-trim]').count(), 0, mode);
    assert.ok((await pg.locator('.hc-hand .hc-card[data-i]').count()) >= 9, mode + ': the cards are still there');
    assert.deepEqual(errs, [], mode);
  }
});

// --- The label reads against the real pixels behind it ---

// The label is photographed three times: in black, in white and with no ink.
// A pixel the black and white pictures disagree on is one a letter covers,
// whatever is behind it; the picture with no ink says what is behind it
// there. The worst of those against the label's own colour is the ratio.
// Pixels of the label's box that no letter covers are not text and are left
// out, so a frame that comes close to the words does not count against them.
async function labelRatios(pg, desk, count) {
  const out = [];
  for (let i = 0; i < count; i++) {
    const card = pg.locator(cards(desk)).nth(i);
    await card.scrollIntoViewIfNeeded();
    await idle(pg, 60);
    const lab = card.locator('.hc-top > span:first-child');
    const colour = await lab.evaluate((e) => { const cx = document.createElement('canvas').getContext('2d', { willReadFrequently: true }); cx.fillStyle = '#000'; cx.fillStyle = getComputedStyle(e).color; cx.fillRect(0, 0, 1, 1); const d = cx.getImageData(0, 0, 1, 1).data; return [d[0], d[1], d[2]]; });
    // The creator fades colour changes; the three pictures need each ink at once.
    const inked = async (ink) => { await lab.evaluate((e, ink) => { e.style.setProperty('transition', 'none', 'important'); e.style.setProperty('color', ink, 'important'); }, ink); return (await lab.screenshot()).toString('base64'); };
    const shots = { black: await inked('#000'), white: await inked('#fff'), bare: await inked('transparent') };
    await lab.evaluate((e) => { e.style.removeProperty('color'); e.style.removeProperty('transition'); });
    const got = await pg.evaluate(async ({ shots, fg }) => {
      const px = async (b64) => {
        const im = new Image(); im.src = 'data:image/png;base64,' + b64; await im.decode();
        const cv = document.createElement('canvas'); cv.width = im.width; cv.height = im.height;
        const cx = cv.getContext('2d', { willReadFrequently: true }); cx.drawImage(im, 0, 0);
        return cx.getImageData(0, 0, cv.width, cv.height).data;
      };
      const lum = (c) => { const v = c.map((x) => { x /= 255; return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4); }); return 0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2]; };
      const K = await px(shots.black), W = await px(shots.white), B = await px(shots.bare), lf = lum(fg);
      let min = 99, letters = 0;
      for (let p = 0; p < B.length; p += 4) {
        // black against white differs by 765 where a letter fills the pixel; a third of that is a letter's edge
        if (Math.abs(K[p] - W[p]) + Math.abs(K[p + 1] - W[p + 1]) + Math.abs(K[p + 2] - W[p + 2]) < 255) continue;
        letters++;
        const lb = lum([B[p], B[p + 1], B[p + 2]]);
        min = Math.min(min, (Math.max(lf, lb) + 0.05) / (Math.min(lf, lb) + 0.05));
      }
      return { min, letters };
    }, { shots, fg: colour });
    out.push({ card: await card.locator('h3').textContent(), ratio: Math.round(got.min * 100) / 100, letters: got.letters });
  }
  return out;
}

for (const colour of ['#c4243a', '#facc15', '#f8fafc', '#1e293b', '#22c55e']) {
  for (const [label, width, desk] of sizes) {
    for (const dark of [false, true]) {
      test(`${label}, ${dark ? 'dark' : 'light'}: the small label reads at 4.5:1 on a ${colour} border`, { skip }, async (t) => {
        const { pg } = await open(t, { width, dark, p: every(colour) });
        await pg.waitForFunction((n) => window.__made >= n, 8, { timeout: 15000 });
        await idle(pg, 400);
        const rs = await labelRatios(pg, desk, KEYS.length);
        for (const r of rs) {
          assert.ok(r.letters > 150, `${r.card}: the label's letters were not found (${r.letters} pixels)`);
          assert.ok(r.ratio >= 4.5, `${r.card}: ${r.ratio}`);
        }
      });
    }
  }
}

// --- The words stay inside the frame ---

test('"Read the rules" sits wholly inside the padding of a bordered card on a 360x640 phone', { skip }, async (t) => {
  const { pg } = await open(t, { width: 360, height: 640, p: every() });
  await idle(pg, 300);
  const n = KEYS.length, bad = [];
  for (let i = 0; i < n; i++) {
    await pg.evaluate((j) => { const h = document.querySelector('[data-hand]'), c = h.querySelectorAll('.hc-card[data-i]')[j]; h.scrollLeft = c.offsetLeft - (h.clientWidth - c.offsetWidth) / 2; }, i);
    await idle(pg, 250);
    const r = await pg.evaluate((j) => {
      const c = document.querySelectorAll('.hc-hand .hc-card[data-i]')[j], more = c.querySelector('.hc-more'), cs = getComputedStyle(c), a = c.getBoundingClientRect(), m = more.getBoundingClientRect();
      const b = (p) => parseFloat(cs['border' + p + 'Width']) || 0, pd = (p) => parseFloat(cs['padding' + p]) || 0;
      const inner = { top: a.top + b('Top') + pd('Top'), bottom: a.bottom - b('Bottom') - pd('Bottom'), left: a.left + b('Left') + pd('Left'), right: a.right - b('Right') - pd('Right') };
      return { name: c.querySelector('h3').textContent, trim: c.dataset.trim, up: m.top - inner.top, down: inner.bottom - m.bottom, l: m.left - inner.left, r: inner.right - m.right };
    }, i);
    if (!r.trim) continue;
    if (r.up < -0.5 || r.down < -0.5 || r.l < -0.5 || r.r < -0.5) bad.push(r);
  }
  assert.deepEqual(bad, []);
  // The classic card is not held to the frame's padding: nothing changed for it.
  const cl = await open(t, { width: 360, height: 640, look: false });
  assert.equal(await cl.pg.locator('[data-trim]').count(), 0);
});

// --- A card reads whole as the room shrinks ---

// Where each part of a card sits against the card's padding box, for the card
// at index j (brought to the middle of the hand first). The hand fits only the
// cards near its middle, in the frame after it scrolls, so the card is read two
// frames later: reading it at once raced that frame on a busy machine.
const insideOf = (pg, j) => pg.evaluate(async (n) => {
  const h = document.querySelector('[data-hand]'), c = h.querySelectorAll('.hc-card[data-i]')[n];
  h.scrollLeft = c.offsetLeft - (h.clientWidth - c.offsetWidth) / 2;
  await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
  const cs = getComputedStyle(c), pt = parseFloat(cs.paddingTop), pb = parseFloat(cs.paddingBottom);
  const top = c.querySelector('.hc-top'), h3 = c.querySelector('h3'), more = c.querySelector('.hc-more');
  return { name: h3.textContent, foot: c.clientHeight - pb - (more.offsetTop + more.offsetHeight), label: top.offsetTop - pt, nameTop: h3.offsetTop - pt, nameGap: more.offsetTop - (h3.offsetTop + h3.offsetHeight) };
}, j);

const roomState = (pg) => pg.evaluate(() => {
  const app = document.querySelector('.hc-app'), body = document.querySelector('.hc-body'), stg = document.querySelector('[data-stage]'), hand = document.querySelector('.hc-hand');
  const intro = document.querySelector('.hc-intro p');
  return { short: app.classList.contains('is-short'), introShown: !!intro && getComputedStyle(intro).display !== 'none', tucked: stg.classList.contains('is-tuck'), figuresShown: stg.querySelector('.hc-sstw').getBoundingClientRect().height > 20,
    scrolls: body.scrollHeight > body.clientHeight + 1, hand: Math.round(hand.clientHeight), page: document.documentElement.scrollWidth <= document.documentElement.clientWidth && app.scrollWidth <= app.clientWidth };
});

for (const room of [400, 450, 505, 540, 579, 580, 592, 616, 640, 700]) {
  test(`a card of every border reads whole in a room of ${room} on a 360 wide phone`, { skip }, async (t) => {
    const { pg, errors } = await open(t, { width: 360, height: room, p: every() });
    await idle(pg, 300);
    const bad = [];
    for (let i = 0; i < KEYS.length; i++) {
      const r = await insideOf(pg, i);
      await idle(pg, 60);
      const again = await insideOf(pg, i);
      if (again.foot < -1 || again.label < -1 || again.nameTop < -1 || again.nameGap < -1) bad.push(again);
      assert.equal(r.name, again.name);
    }
    assert.deepEqual(bad, []);
    const st = await roomState(pg);
    assert.ok(st.page, 'no sideways scroll');
    // The page gives way in order: the intro line, then the figures, then the body scrolls.
    if (room >= 630) assert.deepEqual([st.short, st.introShown, st.tucked, st.figuresShown, st.scrolls], [false, true, false, true, false], 'a roomy phone gives nothing up');
    if (room === 616) assert.deepEqual([st.short, st.introShown, st.tucked, st.figuresShown, st.scrolls], [true, false, false, true, false], 'only the intro line goes');
    if (room === 540) assert.deepEqual([st.short, st.tucked, st.figuresShown, st.scrolls], [true, true, false, false], 'the figures go next');
    if (room === 400) assert.deepEqual([st.short, st.tucked, st.scrolls], [true, true, true], 'last, the body scrolls');
    assert.deepEqual(errors, []);
  });
}

test('on a 400 room the body scrolls to the position line under the cards', { skip }, async (t) => {
  const { pg } = await open(t, { width: 360, height: 400, p: every() });
  await idle(pg, 300);
  const r = await pg.evaluate(() => {
    const body = document.querySelector('.hc-body'), under = document.querySelector('.hc-under');
    const before = under.getBoundingClientRect().bottom > window.innerHeight;
    body.scrollTop = body.scrollHeight;
    const u = under.getBoundingClientRect(), b = body.getBoundingClientRect();
    return { before, inBody: u.top >= b.top - 1 && u.bottom <= b.bottom + 1, inWindow: u.bottom <= window.innerHeight + 1, oy: getComputedStyle(body).overflowY };
  });
  assert.deepEqual(r, { before: true, inBody: true, inWindow: true, oy: 'auto' });
});

// A finger put down at a point and moved in steps, as a swipe is, then lifted.
async function swipe(pg, x, y, dx, dy) {
  const cdp = await pg.context().newCDPSession(pg);
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] });
  for (let i = 1; i <= 10; i++) {
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: x + dx * i / 10, y: y + dy * i / 10 }] });
    await pg.waitForTimeout(16);
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await pg.waitForTimeout(400);
}
const scrolled = (pg) => pg.evaluate(() => ({
  body: Math.round(document.querySelector('.hc-body').scrollTop), hand: Math.round(document.querySelector('.hc-hand').scrollLeft), page: Math.round(window.scrollY),
}));
// The middle of the card at the middle of the hand, where a thumb lands.
const midCard = (pg) => pg.evaluate(() => {
  const h = document.querySelector('.hc-hand').getBoundingClientRect(), mid = h.left + h.width / 2;
  const c = Array.from(document.querySelectorAll('.hc-hand .hc-card[data-i]')).map((x) => x.getBoundingClientRect()).sort((a, b) => Math.abs(a.left + a.width / 2 - mid) - Math.abs(b.left + b.width / 2 - mid))[0];
  return { x: Math.round(c.left + c.width / 2), y: Math.round(Math.min(c.top + c.height / 2, window.innerHeight - 40)) };
});

for (const room of [450, 400]) {
  test(`in a room of ${room} a swipe up that starts on a card scrolls the body, and the page behind stays`, { skip }, async (t) => {
    const { pg } = await open(t, { width: 360, height: room, p: every() });
    await idle(pg, 300);
    // A page behind that could move if the swipe reached it.
    await pg.evaluate(() => { const d = document.createElement('div'); d.style.height = '3000px'; document.body.appendChild(d); });
    const at = await midCard(pg), before = await scrolled(pg);
    assert.deepEqual([before.body, before.page], [0, 0]);
    await swipe(pg, at.x, at.y, 0, -160);
    const up = await scrolled(pg);
    assert.ok(up.body > 0, 'the body moved under a finger that started on a card: ' + JSON.stringify(up));
    assert.equal(up.hand, before.hand, 'an up swipe does not move the hand');
    // Past the body's end the swipe stops there.
    await swipe(pg, at.x, at.y, 0, -160);
    await swipe(pg, at.x, at.y, 0, -160);
    const end = await scrolled(pg);
    const max = await pg.evaluate(() => { const b = document.querySelector('.hc-body'); return b.scrollHeight - b.clientHeight; });
    assert.ok(end.body >= max - 1, 'the body reaches its end: ' + end.body + ' of ' + max);
    assert.equal(end.page, 0, 'the page behind did not move');
  });
}

test('in a room of 450 a sideways swipe on a card still moves the hand and not the body', { skip }, async (t) => {
  const { pg } = await open(t, { width: 360, height: 450, p: every() });
  await idle(pg, 300);
  const at = await midCard(pg), before = await scrolled(pg);
  await swipe(pg, at.x, at.y, -170, 0);
  const after = await scrolled(pg);
  assert.ok(after.hand > before.hand, 'the hand moved on: ' + JSON.stringify([before, after]));
  assert.deepEqual([after.body, after.page], [before.body, 0]);
});

test('growing the window back gives the intro line and the open figures back', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 360, height: 540, p: every() });
  await idle(pg, 300);
  let st = await roomState(pg);
  assert.deepEqual([st.short, st.tucked], [true, true]);
  await pg.setViewportSize({ width: 360, height: 400 });
  await idle(pg, 300);
  st = await roomState(pg);
  assert.equal(st.scrolls, true);
  await pg.setViewportSize({ width: 360, height: 844 });
  await idle(pg, 400);
  st = await roomState(pg);
  assert.deepEqual([st.short, st.introShown, st.tucked, st.figuresShown, st.scrolls], [false, true, false, true, false]);
  assert.equal(await pg.evaluate(() => document.querySelector('[data-hand]').style.minHeight), '');
  assert.deepEqual(errors, []);
});

test('a step change settles the room again: bordered cards tuck the stage, plain ones do not need to', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 360, height: 540, p: every() });
  await idle(pg, 300);
  assert.deepEqual(await pg.evaluate(() => [document.querySelector('[data-stage]').classList.contains('is-tuck'), document.querySelector('.hc-app').classList.contains('is-short')]), [true, true]);
  const go = async (n) => { await pg.click('.hc-sbtn'); await idle(pg, 200); await pg.click('.hc-drape [data-go="' + n + '"]'); await idle(pg, 400); };
  await go(1);
  const st1 = await roomState(pg);
  assert.deepEqual([st1.short, st1.tucked, st1.figuresShown, st1.scrolls], [true, false, true, false], 'plain cards fit with the figures open');
  await go(0);
  const st0 = await roomState(pg);
  assert.deepEqual([st0.short, st0.tucked, st0.scrolls], [true, true, false], 'and back on the bordered step');
  assert.deepEqual(errors, []);
});

for (const room of [400, 500, 592]) {
  test(`a plain card of the stage look reads whole in a room of ${room}`, { skip }, async (t) => {
    const { pg, errors } = await open(t, { width: 360, height: room, p: plan(['Elf', 'Gnome', 'Orc', 'Human'].map((n) => ch(n, null, { speed: 5 }))) });
    await idle(pg, 300);
    for (let i = 0; i < 4; i++) {
      const r = await insideOf(pg, i);
      assert.ok(r.foot >= -1 && r.label >= -1 && r.nameTop >= -1 && r.nameGap >= -1, JSON.stringify(r));
    }
    assert.ok((await roomState(pg)).page);
    assert.deepEqual(errors, []);
  });
}

test('making room does not loop with the size watcher', { skip }, async (t) => {
  const { pg } = await open(t, { width: 360, height: 400, p: every() });
  await pg.evaluate(() => {
    window.__fits = 0;
    const set = CSSStyleDeclaration.prototype.setProperty;
    CSSStyleDeclaration.prototype.setProperty = function (k, ...a) { if (k === '--hc-h') window.__fits++; return set.call(this, k, ...a); };
  });
  await pg.setViewportSize({ width: 360, height: 420 });
  await idle(pg, 300);
  const first = await pg.evaluate(() => window.__fits);
  await idle(pg, 600);
  assert.ok(first >= 1 && first <= 3, 'a resize fits a few times: ' + first);
  assert.equal(await pg.evaluate(() => window.__fits), first, 'and then rests');
});

test('no sideways scroll at 360 wide with bordered cards', { skip }, async (t) => {
  const { pg } = await open(t, { width: 360, height: 640, p: every() });
  await idle(pg, 300);
  const fits = () => pg.evaluate(() => ({ page: document.documentElement.scrollWidth <= document.documentElement.clientWidth, app: document.querySelector('.hc-app').scrollWidth <= document.querySelector('.hc-app').clientWidth }));
  assert.deepEqual(await fits(), { page: true, app: true }, 'the hand');
  await pg.evaluate(() => { const h = document.querySelector('[data-hand]'); h.scrollLeft = h.scrollWidth; });
  await idle(pg, 250);
  assert.deepEqual(await fits(), { page: true, app: true }, 'the hand at its far end');
  await pg.click('[data-mode="list"]');
  assert.deepEqual(await fits(), { page: true, app: true }, 'the list');
});

// --- Boiler works ---

const BOILER = plan([ch('Boiler', { border: 'mechanical', colour: '#6fa8dc' }, { speed: 5 }), ch('Plain')]);
const pseudoOf = (pg, sel, pseudo) => pg.locator(sel).first().evaluate((e, ps) => { const c = getComputedStyle(e, ps); return { slice: c.borderImageSlice, repeat: c.borderImageRepeat, source: c.borderImageSource, top: c.borderTopWidth, bottom: c.borderBottomWidth, left: c.borderLeftWidth, right: c.borderRightWidth }; }, pseudo);

for (const [label, width, desk] of sizes) {
  test(`${label}: a Boiler works card has its steam ground and its coloured pictures`, { skip }, async (t) => {
    const { pg, errors } = await open(t, { width, p: BOILER });
    const sel = `${cards(desk)}[data-trim="mechanical"]`;
    assert.equal(await pg.locator(sel).count(), 1);
    const ground = await pg.locator(sel).evaluate((e) => ({ prop: e.style.getPropertyValue('--trim-ground').trim(), bg: getComputedStyle(e).backgroundImage }));
    assert.equal(ground.prop, 'url("/borders/mechanical-steam.webp?v=abc123")');
    assert.ok(ground.bg.includes('/borders/mechanical-steam.webp?v=abc123'), 'the steam is among the card\'s backgrounds: ' + ground.bg);
    await madeOn(pg, desk, 'Boiler', '--trim-img');
    await madeOn(pg, desk, 'Boiler', '--trim-corner');
    const c = (await trimOf(pg, cards(desk))).find((x) => x.name === 'Boiler');
    assert.match(c.img, /^url\("blob:/);
    assert.match(c.corner, /^url\("blob:/);
    assert.equal(errors.length, 0, errors.join('\n'));
  });

  test(`${label}: the Boiler works corner sheet is a nine-slice drawn as a second border image`, { skip }, async (t) => {
    const { pg } = await open(t, { width, p: BOILER });
    const sel = `${cards(desk)}[data-trim="mechanical"]`;
    await madeOn(pg, desk, 'Boiler', '--trim-corner');
    const b = await pseudoOf(pg, sel, '::before');
    assert.equal(b.slice, '240 265');
    assert.equal(b.repeat, 'stretch');
    assert.match(b.source, /^url\("blob:/);
    const [v, h] = desk ? ['43px', '48px'] : ['64px', '70px'];
    assert.deepEqual([b.top, b.bottom, b.left, b.right], [v, v, h, h]);
  });
}

// The film curve, written out from its published form (Narkowicz-style ACES fit
// with the RRT/ODT matrices) as an independent check on the script's own.
const ACES_IN = [[0.59719, 0.35458, 0.04823], [0.076, 0.90834, 0.01566], [0.0284, 0.13383, 0.83777]];
const ACES_OUT = [[1.60475, -0.53108, -0.07367], [-0.10208, 1.10813, -0.00605], [-0.00327, -0.07276, 1.07602]];
const mul = (m, v) => m.map((r) => r[0] * v[0] + r[1] * v[1] + r[2] * v[2]);
const toLin = (c) => { c /= 255; return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4; };
const toSrgb = (v) => (v <= 0.0031308 ? v * 12.92 : 1.055 * v ** (1 / 2.4) - 0.055);
const acesShown = (rgb, strength) => {
  const lin = rgb.map((c) => toLin(c) * strength / 0.6);
  const fit = mul(ACES_IN, lin).map((v) => (v * (v + 0.0245786) - 0.000090537) / (v * (0.983729 * v + 0.4329510) + 0.238081));
  return mul(ACES_OUT, fit).map((v) => toSrgb(Math.min(1, Math.max(0, v))));
};
const hexRgb = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16));
// What the glow colouring leaves of a colour, as a share of 255 per channel.
const glowFactors = (hex, strength) => { const w = acesShown([255, 255, 255], strength), g = acesShown(hexRgb(hex), strength); return g.map((v, i) => v / w[i]); };

// The made picture against the served one and its mask: at the mask's brightest
// pixel and at the darkest base pixel the mask half-covers, so a clamp at white
// cannot hide a wrong factor.
async function samplePixels(pg, { made, base, mask }) {
  return pg.evaluate(async ({ made, base, mask }) => {
    const load = (u) => new Promise((ok, no) => { const i = new Image(); i.onload = () => ok(i); i.onerror = () => no(new Error(u)); i.src = u; });
    const [M, B, K] = await Promise.all([load(made), load(base), load(mask)]);
    const w = B.naturalWidth, h = B.naturalHeight;
    const px = (im) => { const c = document.createElement('canvas'); c.width = w; c.height = h; const x = c.getContext('2d', { willReadFrequently: true }); x.drawImage(im, 0, 0, w, h); return x.getImageData(0, 0, w, h).data; };
    const m = px(M), b = px(B), k = px(K);
    let top = -1, mid = -1, topA = 0, midBase = 999;
    for (let i = 0; i < k.length; i += 4) {
      if (b[i + 3] !== 255) continue;
      if (k[i] > topA) { topA = k[i]; top = i; }
      const dark = Math.max(b[i], b[i + 1], b[i + 2]);
      if (k[i] >= 100 && k[i] <= 200 && dark < midBase) { midBase = dark; mid = i; }
    }
    const at = (i) => ({ made: [m[i], m[i + 1], m[i + 2]], base: [b[i], b[i + 1], b[i + 2]], mask: k[i] });
    return { size: [w, h], top: top < 0 ? null : at(top), mid: mid < 0 ? null : at(mid) };
  }, { made, base, mask });
}

// Makes the border at the given colour and hands back what the colouring did at the two sample pixels.
async function sampleOf(t, { key, colour, name }) {
  const { pg } = await open(t, { width: 390, p: plan([ch('Only', { border: key, colour }, { speed: 5 })]) });
  await madeOn(pg, false, 'Only', '--trim-img');
  const made = (await trimOf(pg, cards(false)))[0].img.match(/url\("(blob:[^"]+)"\)/)[1];
  const dir = '/borders/';
  return samplePixels(pg, { made, base: dir + LIB[key].img + '?v=abc123', mask: dir + LIB[key].mask + '?v=abc123' });
}

const within = (got, want, tol, msg) => got.forEach((g, i) => assert.ok(Math.abs(g - want[i]) <= tol, `${msg}: channel ${i} is ${g}, wanted ${want[i].toFixed(1)}`));

test('the film curve written here keeps white white and lifts the weaker channels of a strong colour', () => {
  within(glowFactors('#ffffff', 1.4), [1, 1, 1], 1e-9, 'white');
  within(glowFactors('#6fa8dc', 1.4), [0.69, 0.86, 0.96], 0.01, 'blue');
  within(glowFactors('#c4243a', 1.4), [0.99, 0.17, 0.29], 0.01, 'red');
});

for (const colour of ['#6fa8dc', '#c4243a', '#ffffff']) {
  test(`Boiler works colouring at ${colour}: the made frame is the base plus the mask times the glowed colour`, { skip }, async (t) => {
    const f = glowFactors(colour, 1.4);
    const got = await sampleOf(t, { key: 'mechanical', colour });
    assert.ok(got.top && got.mid, 'the mask has pixels to sample');
    for (const which of ['top', 'mid']) {
      const s = got[which], want = s.base.map((b, i) => Math.min(255, b + s.mask * f[i]));
      within(s.made, want, 3, `${which} pixel (mask ${s.mask}, base ${s.base})`);
      // The test must be able to tell a wrong factor: the glowed colour adds something on at least one channel.
      assert.ok(want.some((w, i) => w - s.base[i] > 20), which + ' pixel is lit by the colour');
    }
  });
}

test('a border without glow is coloured as before: base plus mask times colour times the peak and gain scale', { skip }, async (t) => {
  const col = '#d9622b', rgb = hexRgb(col), k = 1.3 / Math.max(...rgb, 13);
  const got = await sampleOf(t, { key: 'fire', colour: col });
  for (const which of ['top', 'mid']) {
    const s = got[which], want = s.base.map((b, i) => Math.min(255, b + s.mask * rgb[i] * k));
    within(s.made, want, 3, `${which} pixel (mask ${s.mask}, base ${s.base})`);
  }
});

test('phone: a bordered card in the hand is layout-contained only, a plain one keeps paint containment', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390, p: BOILER });
  const contain = (sel) => pg.locator(sel).first().evaluate((e) => getComputedStyle(e).contain);
  assert.equal(await contain('.hc-hand .hc-card[data-trim]'), 'layout');
  assert.equal(await contain('.hc-hand .hc-card[data-i]:not([data-trim])'), 'layout paint');
});

// A corner piece lies over the frame, outside the card's padding box. The strip
// between the card's border-box corner and its padding-box corner is where
// paint containment would cut it, so the test photographs a few pixels of it
// with the piece and without it.
for (const key of ['carved-stone', 'mechanical']) {
  test(`phone: the ${key} corner piece is painted outside the card's padding box`, { skip }, async (t) => {
    const { pg } = await open(t, { width: 390, p: plan([ch('Only', { border: key, colour: colourOf(key) }, { speed: 5 })]) });
    await madeOn(pg, false, 'Only', '--trim-corner');
    await idle(pg, 300);
    const sel = '.hc-hand .hc-card[data-trim]';
    await pg.locator(sel).scrollIntoViewIfNeeded();
    const r = await pg.locator(sel).evaluate((e) => {
      const b = e.getBoundingClientRect(), cs = getComputedStyle(e), pb = e.querySelector('.hc-top').getBoundingClientRect();
      return { left: b.left, top: b.top, bl: parseFloat(cs.borderLeftWidth), bt: parseFloat(cs.borderTopWidth), inset: pb.left - b.left };
    });
    // The piece starts fw + 5px outside the padding box (21px on a phone) and the padding box starts bl in from the border box.
    const lo = Math.max(2, Math.ceil(r.bl - 16)), size = 6, x = Math.round(r.left + lo + 2), y = Math.round(r.top + Math.max(2, Math.ceil(r.bt - 16)) + 2);
    assert.ok(lo + 2 + size < r.bl, `the sample (${lo + 2}..${lo + 2 + size}) lies in the border strip (${r.bl}px)`);
    const clip = { x, y, width: size, height: size };
    const withPiece = await pg.screenshot({ clip });
    await pg.addStyleTag({ content: `.hc-hand .hc-card[data-trim]::before { display: none !important; }` });
    await idle(pg, 100);
    const without = await pg.screenshot({ clip });
    assert.notDeepEqual(withPiece, without, 'the piece shows in the strip outside the padding box');
  });
}

// A corner piece's own box stays inside the card's padding box at the right and
// the bottom. A box that passed them would count as the card's content running
// over, and the fit would give up the summary and the facts to make room for it.
const FACTS = { size: '1M', speed: 5, stability: 0 };
// Facts that give the hero no figures, so a short phone has room for more than a name.
const QUIET = { languages: 'Caelian', skill: 'Lore' };
const only = (key, props, name = 'Entry') => plan([ch(name, { border: key, colour: colourOf(key) }, props)]);
// What the fitted card in front shows, and how far its content runs past it.
const front = (pg) => pg.locator('.hc-hand .hc-card[data-i]').first().evaluate((c) => {
  const sum = c.querySelector('.hc-sum'), dl = c.querySelector('.hc-facts'), b = getComputedStyle(c, '::before'), hand = c.closest('.hc-hand');
  return {
    over: c.scrollHeight - c.clientHeight, wide: c.scrollWidth - c.clientWidth,
    sum: sum.hidden ? 0 : Math.round(sum.getBoundingClientRect().height), facts: dl && !dl.hidden ? Array.from(dl.children).filter((d) => !d.hidden).length : 0,
    name: getComputedStyle(c.querySelector('h3')).fontSize,
    piece: b.content === 'none' ? null : { height: parseFloat(b.height), bottom: parseFloat(b.bottom), right: parseFloat(b.right), outset: b.borderImageOutset },
    below: hand.getBoundingClientRect().bottom - c.getBoundingClientRect().bottom,
  };
});

for (const key of ['mechanical', 'carved-stone']) {
  for (const [width, height, props, facts] of [[390, 844, FACTS, true], [360, 640, QUIET, false]]) {
    test(`phone ${width}x${height}: a ${key} card keeps the summary and facts a card without a corner piece keeps`, { skip }, async (t) => {
      const { pg } = await open(t, { width, height, p: only(key, props) });
      await idle(pg, 300);
      const got = await front(pg);
      const { pg: plain } = await open(t, { width, height, p: only('fire', props) });
      await idle(plain, 300);
      const want = await front(plain);
      assert.ok(got.over <= 1 && got.wide <= 0, `nothing runs past the card (${got.over}px down, ${got.wide}px across)`);
      assert.ok(got.sum > 0, 'the summary shows');
      if (facts) assert.ok(got.facts >= 1, 'at least one fact shows');
      assert.deepEqual({ sum: got.sum, facts: got.facts, name: got.name }, { sum: want.sum, facts: want.facts, name: want.name }, 'the same as a Fire card of the same words');
    });
  }
}

// On a short phone a card is shorter than the worked corner of Carved stone. The
// piece ends at the padding box's foot, inside the frame's bottom side, and the
// four instruments of Boiler works reach no further than the frame's own outer
// edge, which the hand's padding leaves room for.
for (const height of [640, 540, 450, 400]) {
  test(`phone 360x${height}: a corner piece stays inside a short card`, { skip }, async (t) => {
    const { pg } = await open(t, { width: 360, height, p: only('carved-stone', FACTS) });
    await idle(pg, 300);
    const stone = await front(pg);
    assert.ok(stone.piece.height < 300, `the piece (${stone.piece.height}px) is shorter than its whole 300px`);
    assert.ok(Math.abs(stone.piece.bottom) <= 0.5, `it ends at the padding box's foot (${stone.piece.bottom}px from it)`);
    assert.ok(stone.over <= 1 && stone.wide <= 0, `nothing runs past the card (${stone.over}px down, ${stone.wide}px across)`);
    const { pg: other } = await open(t, { width: 360, height, p: only('mechanical', FACTS) });
    await idle(other, 300);
    const boiler = await front(other);
    assert.deepEqual([boiler.piece.bottom, boiler.piece.right, boiler.piece.outset], [0, 0, '21px'], 'the instruments are drawn from the padding box out to the frame\'s outer edge');
    assert.ok(boiler.over <= 1 && boiler.wide <= 0, `nothing runs past the card (${boiler.over}px down, ${boiler.wide}px across)`);
    assert.ok(boiler.below >= 5, `the hand keeps ${boiler.below}px under the card for the 5px the frame overhangs`);
  });
}

test('phone 390x844: on a tall card the Carved stone corner piece is whole', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390, height: 844, p: only('carved-stone', FACTS) });
  await idle(pg, 300);
  const stone = await front(pg);
  assert.equal(stone.piece.height, 300);
  assert.ok(stone.piece.bottom > 28, 'it ends above the bottom side');
});

test('computer: the Carved stone corner piece is not cut short on a card of the usual height', { skip }, async (t) => {
  const { pg } = await open(t, { width: 1280, p: only('carved-stone', FACTS) });
  await idle(pg, 300);
  const got = await pg.locator('.hc-gcard[data-trim="carved-stone"]').first().evaluate((c) => { const b = getComputedStyle(c, '::before'); return { height: parseFloat(b.height), bottom: parseFloat(b.bottom), over: c.scrollHeight - c.clientHeight }; });
  // The whole piece is 900/132 of the frame's 30px on a computer.
  assert.ok(30 * 900 / 132 - got.height <= 2, `the piece is ${got.height}px of ${(30 * 900 / 132).toFixed(1)}`);
  assert.ok(got.bottom >= 0 && got.over <= 1, `it does not pass the padding box (${got.bottom}px inside, ${got.over}px over)`);
});

// A name of one long word cannot wrap. Down to the smallest size it is set
// smaller; past that it breaks, and no letter of it lies outside the name's own
// box. A bordered card has no paint containment to hide one that did.
const LONG_NAME = 'Abcdefghijklmnopqrstuvwxyzabcdefghijklmn';
for (const [width, height] of [[360, 844], [360, 640]]) {
  test(`phone ${width}x${height}: a forty-letter name breaks inside its card`, { skip }, async (t) => {
    const p = plan([ch(LONG_NAME, { border: 'fire', colour: '#d9622b' }, FACTS), ch(LONG_NAME + 'o', { border: 'mechanical', colour: '#6fa8dc' }, FACTS), ch(LONG_NAME + 'p', null, FACTS)]);
    const { pg, errors } = await open(t, { width, height, p });
    await idle(pg, 300);
    const got = await pg.evaluate(() => Array.from(document.querySelectorAll('.hc-hand .hc-card[data-i]')).map((c) => {
      const h3 = c.querySelector('h3'), sum = c.querySelector('.hc-sum'), r = document.createRange();
      r.selectNodeContents(h3);
      const ink = r.getBoundingClientRect(), box = h3.getBoundingClientRect(), card = c.getBoundingClientRect(), cs = getComputedStyle(c);
      return {
        trim: c.getAttribute('data-trim') || 'plain', px: parseFloat(getComputedStyle(h3).fontSize), lines: Math.round(box.height / parseFloat(getComputedStyle(h3).lineHeight)),
        pastName: Math.round(ink.right - box.right), pastCard: Math.round(ink.right - (card.right - parseFloat(cs.borderRightWidth) - parseFloat(cs.paddingRight))),
        nameWide: h3.scrollWidth - h3.clientWidth, sumWide: sum.hidden ? 0 : sum.scrollWidth - sum.clientWidth, over: c.scrollHeight - c.clientHeight,
      };
    }));
    assert.deepEqual(got.map((g) => g.trim), ['fire', 'mechanical', 'plain']);
    for (const g of got) {
      assert.ok(g.pastName <= 0 && g.pastCard <= 0 && g.nameWide <= 1, `${g.trim}: the name stays in its box (${g.pastName}px past the name's box, ${g.pastCard}px past the card's words)`);
      assert.ok(g.px >= 14, `${g.trim}: the name is ${g.px}px, never under 14`);
      // Two lines, unless the name is already at the smallest size: there it
      // takes the lines it needs, and how many depends on how wide the face
      // this machine draws with sets forty letters.
      assert.ok(g.lines >= 1 && (g.lines <= 2 || g.px === 14), `${g.trim}: the name takes ${g.lines} lines at ${g.px}px`);
      assert.ok(g.sumWide <= 1, `${g.trim}: no word of the summary is cut (${g.sumWide}px)`);
      assert.ok(g.over <= 1, `${g.trim}: nothing runs past the card (${g.over}px)`);
    }
    assert.equal(errors.length, 0, errors.join('\n'));
  });
}

test('computer: a forty-letter name breaks inside its card, bordered or plain', { skip }, async (t) => {
  const { pg } = await open(t, { width: 1280, p: plan([ch(LONG_NAME, { border: 'fire', colour: '#d9622b' }, FACTS), ch(LONG_NAME + 'o', null, FACTS)]) });
  await idle(pg, 300);
  const got = await pg.evaluate(() => Array.from(document.querySelectorAll('.hc-gcard[data-i]')).map((c) => {
    const h3 = c.querySelector('h3'), r = document.createRange();
    r.selectNodeContents(h3);
    return { trim: c.getAttribute('data-trim') || 'plain', past: Math.round(r.getBoundingClientRect().right - h3.getBoundingClientRect().right), wide: c.scrollWidth - c.clientWidth };
  }));
  assert.deepEqual(got.map((g) => g.trim), ['fire', 'plain']);
  for (const g of got) assert.ok(g.past <= 0 && g.wide <= 0, `${g.trim}: the name stays in its card (${g.past}px past its box, the card ${g.wide}px wider inside than out)`);
});

// Sixteen letters are too wide at the name's full size and fit at the smallest
// in a narrow face and in a wide one alike.
test('phone 390x844: a one-word name that fits at a smaller size is set smaller, not broken', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390, height: 844, p: only('fire', FACTS, 'Abcdefghijklmnop') });
  await idle(pg, 300);
  const h3 = await pg.locator('.hc-hand .hc-card[data-i] h3').first().evaluate((h) => ({ px: parseFloat(getComputedStyle(h).fontSize), wrap: h.style.overflowWrap, lines: Math.round(h.getBoundingClientRect().height / parseFloat(getComputedStyle(h).lineHeight)), wide: h.scrollWidth - h.clientWidth }));
  assert.ok(h3.px >= 14 && h3.px < 27, `the name is set at ${h3.px}px`);
  assert.deepEqual([h3.wrap, h3.lines], ['', 1], 'on one line, unbroken');
  assert.ok(h3.wide <= 1);
});

// An address the script cannot put safely in a style leaves the card plain.
for (const [part, key, bad] of [
  ['ground', 'mechanical', '/borders/a b.webp'],
  ['ground', 'mechanical', '/borders/a".webp'],
  ['ground', 'mechanical', '/borders/a).webp'],
  ['img', 'fire', '/borders/a b.webp'],
  ['corner', 'mechanical', '/borders/a b.webp'],
]) {
  test(`an unsafe ${part} address (${bad}) leaves a ${key} card plain`, { skip }, async (t) => {
    const p = plan([ch('Only', { border: key, colour: colourOf(key) }, { speed: 5 }), ch('Other', { border: 'earth', colour: '#b08a5a' })]);
    const { pg, errors } = await open(t, { width: 390, p, patch: { [key]: { [part]: bad } } });
    await idle(pg, 300);
    const by = Object.fromEntries((await trimOf(pg, cards(false))).map((c) => [c.name, c]));
    assert.equal(by.Only.trim, null);
    assert.equal(by.Only.img, '');
    assert.equal(await pg.locator(`${cards(false)}[style*="--trim-ground"]`).count(), 0);
    assert.equal(by.Other.trim, 'earth', 'a safe border is not held up');
    assert.deepEqual(errors, []);
  });
}

// --- The Go list and this table agree ---

test('the test library matches the Go library', () => {
  const src = readFileSync(join(root, 'internal/plugins/entities/cardborders/cardborders.go'), 'utf8');
  for (const [k, v] of Object.entries(LIB)) {
    assert.ok(src.includes(`Key: "${k}"`), 'Go lacks ' + k);
    for (const p of URLS) if (v[p]) assert.ok(src.includes(`"${v[p]}"`), `Go lacks ${v[p]}`);
    if (v.peak != null) assert.ok(src.includes(`Peak: num(${v.peak})`), k + ' peak');
    if (v.gain != null) assert.ok(src.includes(`Gain: num(${v.gain})`), k + ' gain');
    if (v.glow != null) assert.ok(src.includes(`Glow: num(${v.glow})`), k + ' glow');
    for (const p of URLS) if (v[p]) assert.ok(existsSync(join(STATIC, 'img/borders', v[p])), v[p] + ' is shipped');
  }
  assert.equal((src.match(/\{Key: "/g) || []).length, Object.keys(LIB).length);
  // Both ways: an entry the Go side gives a ground or glow that this table lacks is drift too.
  assert.equal((src.match(/Ground: "/g) || []).length, Object.values(LIB).filter((v) => v.ground).length, 'grounds');
  assert.equal((src.match(/Glow: num\(/g) || []).length, Object.values(LIB).filter((v) => v.glow != null).length, 'glows');
});
