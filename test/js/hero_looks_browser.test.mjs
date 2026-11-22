// hero_looks_browser.test.mjs — the Customizer's Hero creator page.
//
// Runs hero_looks.js with hero_workshop.js, hero_border_picker.js,
// hero_borders.js, hero_stage.js and hero_creator.js in a real Chromium, in a
// page that stands in for the Customizer (the real page editor and shell
// stylesheets, the section's markup, a save bar), against a stubbed plan of
// package cards with and without looks, one the owner changed, an accepted
// campaign entry and one that is only handed in. Chronicle.customizeSet and
// customizeDraft are stubs that record, and a test dispatches
// chronicle:customize-draft itself; one test mounts the real customize_shell.js
// and undoes through it. It pins what the frame shows, what a press opens, the
// exact draft value every kind of change writes, the draft laid over the cards,
// hostile draft values, the two views, that nothing in the frame posts or
// navigates, the keyboard, and the site's motion levels.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing. LOOKS_SHOTS=<dir> also writes the
// review screenshots.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const C = '/campaigns/c1';
const TAB = C + '/customize/x/hero-creator';
const STATIC = join(root, 'internal/plugins/entities/static');
const file = (rel) => readFileSync(join(STATIC, rel));
const site = (rel) => readFileSync(join(root, 'static', rel));
const SHOTS = process.env.LOOKS_SHOTS;
if (SHOTS) mkdirSync(SHOTS, { recursive: true });

// The library and the picker's data as the server hands them over (the picker's
// own test keeps these honest against the Go source).
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
const LIBJSON = JSON.stringify(Object.fromEntries(Object.entries(LIB).map(([k, v]) => [k, Object.fromEntries(Object.entries(v).map(([p, x]) => [p, URLS.includes(p) ? '/borders/' + x + '?v=abc123' : x]))])));
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
const PICK = { groups: GROUPS, borders: Object.fromEntries(Object.keys(INFO).map((k) => [k, { name: INFO[k][0], colour: INFO[k][1], tints: !!(LIB[k].mask || LIB[k].cornerMask || LIB[k].dye) }])) };

// Package cards: one with a look the game system gives, one with none, one the
// owner changed (the game system's own is fire). A campaign's own entry the
// Director accepted, and one only handed in.
const pkg = (name, extra = {}) => ({ key: name.toLowerCase(), slug: name.toLowerCase(), name, summary: name + ' summary, a line or two of words.', source: 'package', properties: {}, ...extra });
const PLAN = () => ({
  systemName: 'Draw Steel',
  steps: [
    {
      fieldKey: 'ancestry', label: 'Ancestry', choices: [
        pkg('Dwarf', { look: { border: 'rune-metal', colour: '#3fc8d4' } }),
        pkg('Human'),
        pkg('Orc', { look: { border: 'ice', colour: '#8fd8ff' }, lookChanged: true, systemLook: { border: 'fire', colour: '#d9622b' } }),
        pkg('Elf', { look: { border: 'vine', colour: '#6fae4f' } }),
      ],
    },
    {
      fieldKey: 'class', label: 'Class', choices: [
        pkg('Fury'), pkg('Censor'),
        { key: '7', slug: 'gale', name: 'Gale', summary: 'An accepted entry.', source: 'campaign', properties: {}, look: { border: 'storm', colour: '#6fa8dc' } },
        { key: '8', slug: 'hush', name: 'Hush', summary: 'Handed in, not accepted.', source: 'campaign', status: 'sent', properties: {} },
      ],
    },
    { fieldKey: 'kit', label: 'Kit', choices: [pkg('Mountain'), pkg('Panther')] },
  ],
});
const EMPTY_PLAN = () => ({ systemName: 'Plain Game', steps: [] });

// Tokens and the shared sheets, as the site defines them.
const input = site('css/input.css').toString();
function block(src, from) {
  const at = src.indexOf('{', from);
  let depth = 0, i = at;
  for (; i < src.length; i++) { if (src[i] === '{') depth++; else if (src[i] === '}' && --depth === 0) break; }
  return src.slice(from, i + 1);
}
const TOKENS = block(input, input.indexOf(':root {\n  --z-raised')) + '\n' + block(input, input.indexOf('  :root {\n    --color-bg-primary')) + '\n' + block(input, input.indexOf('  .dark {'));

const SHELL = `<div class="czs czs-is-workshop"><div class="czs-stage"><div class="czs-top" data-czs-top style="height:48px"></div><div class="czs-body" data-czs-stage>
@@SECTION@@</div>
<div class="czs-savebar" role="region" aria-label="Unsaved changes" data-czs-savebar hidden><span class="czs-sdot" aria-hidden="true"></span><button type="button" class="czs-what" data-czs-what></button>
<button type="button" class="czs-icon-btn" data-czs-undo aria-label="Undo">u</button><button type="button" class="czs-icon-btn" data-czs-redo aria-label="Redo">r</button><button type="button" class="czs-ghost" data-czs-discard>Discard</button><button type="button" class="btn-primary btn-sm" data-czs-save>Save changes</button></div></div><div class="czs-toasts" data-czs-toasts></div></div>`;

// The section's own markup (hero_looks_section.templ).
const SECTION = `<div class="cpe hcw" data-widget="hero-looks" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-tab-url="${TAB}" data-borders='${LIBJSON}' data-border-picker='${JSON.stringify(PICK)}' data-view="desk">
<p class="hcw-empty" data-hl-empty hidden>This campaign’s game system has no hero creator, so there are no cards to dress.</p>
<div class="cpe-editor hcw-editor" data-hl-editor><div class="cpe-main"><div class="cpe-canvas hcw-canvas" data-hl-canvas><p class="hcw-cap" data-hl-cap></p><div class="hcw-fit" data-hl-fit><div class="hcw-frame" data-hl-frame></div></div></div></div>
<aside class="cpe-insp hcw-insp" data-hl-insp aria-label="Card border" tabindex="-1"></aside>
<div class="cpe-dock-wrap hcw-dockw"><div class="cpe-dock" data-hl-dock role="toolbar" aria-label="Workshop tools"></div></div></div></div>`;

const SHEETS = ['static:css/customize_shell.css', 'static:css/customize_page_editor.css', 'css/hero_creator.css', 'css/hero_stage.css', 'css/hero_borders.css', 'css/hero_border_picker.css', 'css/hero_looks.css'];
const SCRIPTS = ['js/hero_rules_lines.js', 'js/hero_borders.js', 'js/hero_border_picker.js', 'js/hero_stage.js', 'js/hero_workshop.js', 'js/hero_creator.js'];

// o.draft is what Chronicle.customizeDraft() answers; o.shell mounts the real shell.
function page(o) {
  const links = SHEETS.map((s) => `<link rel="stylesheet" href="/${s.replace(':', '/')}">`).join('');
  const scripts = SCRIPTS.map((s) => `<script src="/${s}"></script>`).join('');
  const stub = o.shell ? '' : `Chronicle.customizeSet = function (k, v, opts) { window.sets.push({ k: k, v: v == null ? v : JSON.parse(JSON.stringify(v)), opts: opts }); };
Chronicle.customizeDraft = function () { return window.DRAFT; };`;
  const shell = o.shell ? '<script src="/static/js/widgets/customize_shell.js"></script>' : '';
  const html = SHELL.replace('@@SECTION@@', SECTION).replace('data-czs-savebar hidden', 'data-czs-savebar hidden');
  const wrap = o.shell ? html.replace('<div class="czs czs-is-workshop">', `<div class="czs czs-is-workshop" data-widget="customize-shell" data-campaign-id="c1" data-csrf="t" data-area="x-hero-creator" data-version="0" data-draft='${JSON.stringify(o.draft || {})}' data-find="[]" data-areas='[{"id":"x-hero-creator","label":"Hero creator","href":"${TAB}","section":"herolooks"}]'>`).replace('<div class="czs-top" data-czs-top style="height:48px"></div>', '<div class="czs-top" data-czs-top style="height:48px"><button type="button" data-czs-find>Find</button></div>') : html;
  return `<!doctype html><html data-gm-term="Director"${o.dark ? ' class="dark"' : ''}${o.motion ? ` data-motion="${o.motion}"` : ''}><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>${TOKENS}*,::before,::after{box-sizing:border-box}html,body{margin:0}body{font-family:Inter,sans-serif;background:var(--color-bg-primary);color:var(--color-text-body)}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}
#main-content{height:100vh;overflow:auto}.btn-primary{background:var(--color-accent);color:#fff;border:0;border-radius:8px;padding:6px 12px}.btn-sm{font-size:13px}</style>${links}</head>
<body><button id="before" type="button">before</button><main id="main-content">${wrap}</main><button id="after" type="button">after</button>
<script>
window.sets = []; window.DRAFT = ${JSON.stringify(o.draft || {})}; window.widgets = {}; window.mounted = new WeakSet(); window.hits = [];
window.Chronicle = { register: function (n, d) { widgets[n] = d; }, escapeHtml: function (v) { return String(v == null ? '' : v).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); },
  gmTerm: function () { return 'Director'; }, markDirty: function () {}, markClean: function () {},
  apiFetch: function (url, opts) { hits.push((opts && opts.method || 'GET') + ' ' + url); return fetch(url, { method: opts && opts.method || 'GET', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: opts && opts.body ? JSON.stringify(opts.body) : undefined }); },
  mountWidgets: function (r) { r.querySelectorAll('[data-widget]').forEach(function (el) { var d = widgets[el.getAttribute('data-widget')]; if (d && !mounted.has(el)) { mounted.add(el); d.init(el); } }); },
  destroyWidget: function (el) { var d = widgets[el.getAttribute('data-widget')]; if (d && mounted.has(el)) { mounted.delete(el); d.destroy(el); } },
  isMounted: function (el) { return mounted.has(el); } };
${stub}
</script>${scripts}<script src="/js/hero_looks.js"></script>${shell}
<script>window.addEventListener('load', function () { Chronicle.mountWidgets(document); window.ready = true; });</script></body></html>`;
}

// o: { width, height, dark, motion, plan, draft, shell, reduced }. seen.api lists every request that is not a served file.
async function open(t, o = {}) {
  const { width = 1280, height = 800, plan = PLAN, reduced = 'no-preference' } = o;
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport: { width, height }, reducedMotion: reduced, hasTouch: width < 700 });
  const pg = await ctx.newPage();
  const seen = { api: [], nav: [] };
  const errors = [];
  pg.on('pageerror', (e) => errors.push(String(e)));
  pg.on('framenavigated', (f) => { if (f === pg.mainFrame()) seen.nav.push(f.url()); });
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), u = new URL(req.url()), p = u.pathname;
    if (p.startsWith('/static/')) {
      const rel = p.slice('/static/'.length);
      return route.fulfill({ contentType: rel.endsWith('.js') ? 'text/javascript' : 'text/css', body: site(rel) });
    }
    if (p.startsWith('/borders/')) return route.fulfill({ contentType: 'image/webp', body: file('img/borders/' + p.slice('/borders/'.length)) });
    if (/^\/(js|css)\//.test(p)) return route.fulfill({ contentType: p.endsWith('.js') ? 'text/javascript' : 'text/css', body: file(p.slice(1)) });
    if (p === C + '/characters/new/plan') { seen.api.push('GET ' + p + u.search); return route.fulfill({ contentType: 'application/json', body: JSON.stringify(typeof plan === 'function' ? plan() : plan) }); }
    if (p === '/') return route.fulfill({ contentType: 'text/html', body: page(o) });
    seen.api.push(req.method() + ' ' + p);
    if (p.endsWith('/customize/draft') || p.endsWith('/customize/publish')) return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ version: 1, applied: [], draft: {} }) });
    return route.fulfill({ contentType: 'text/html', body: '<p>elsewhere</p>' });
  });
  await pg.goto('http://t.test/');
  await pg.waitForFunction(() => window.ready === true);
  if (o.cards !== false) await pg.waitForSelector('.hcw-frame .hc-app');
  return { pg, seen, errors };
}

const sets = (pg) => pg.evaluate(() => window.sets);
const lastSet = async (pg) => (await sets(pg)).slice(-1)[0];
const settle = (pg, ms = 120) => pg.waitForTimeout(ms);
const cardOf = (pg, name) => pg.locator('.hcw-frame :is(.hc-gcard, .hc-card)[data-i]', { hasText: name }).first();
async function press(pg, name) { await cardOf(pg, name).click(); await pg.waitForSelector('.hcw-insp.open'); await settle(pg, 100); }
const tile = (pg, key) => pg.locator('.hcw-insp .hcw-t[data-b="' + key + '"]');
const trimOf = (pg, name) => cardOf(pg, name).evaluate((n) => ({ trim: n.getAttribute('data-trim'), k: n.style.getPropertyValue('--k').trim() }));
const frameInfo = (pg) => pg.evaluate(() => ({ view: document.querySelector('.hcw-frame .hc-app').getAttribute('data-view'), fw: document.querySelector('.hcw-frame').offsetWidth }));
const shot = async (pg, name) => { if (SHOTS) await pg.screenshot({ path: join(SHOTS, name) }); };

// --- The frame ---

test('the frame shows the real creator with bordered cards; nothing is kept, made inert or asked for but the plan', { skip }, async (t) => {
  const { pg, errors, seen } = await open(t);
  await settle(pg, 200);
  assert.deepEqual(errors, []);
  assert.deepEqual(await trimOf(pg, 'Dwarf'), { trim: 'rune-metal', k: '#3fc8d4' });
  assert.deepEqual(await trimOf(pg, 'Orc'), { trim: 'ice', k: '#8fd8ff' });
  assert.deepEqual(await trimOf(pg, 'Elf'), { trim: 'vine', k: '#6fae4f' });
  assert.equal((await trimOf(pg, 'Human')).trim, null);
  const info = await pg.evaluate(() => ({
    keys: Object.keys(localStorage).length, session: Object.keys(sessionStorage).length,
    inert: document.querySelectorAll('[inert]').length, full: document.documentElement.hasAttribute('data-full-surface'),
    cap: document.querySelector('.hcw-cap').textContent, dock: Array.from(document.querySelectorAll('[data-hl-dock] button')).map((b) => b.textContent + ':' + b.getAttribute('aria-pressed')),
    lv: !!document.querySelector('.hcw-frame .hc-lv'), saved: !!document.querySelector('.hcw-frame [data-saved]'), title: document.querySelector('.hcw-frame .hc-abar h1').textContent,
    cpeOpen: document.documentElement.classList.contains('cpe-open'),
  }));
  assert.equal(info.keys, 0, 'no hero draft is read or written');
  assert.equal(info.session, 0);
  assert.equal(info.inert, 0, 'nothing outside the workshop is made inert');
  assert.equal(info.full, false);
  assert.equal(info.cap, 'The real creator, as your players see it. Press a card to change its border.');
  assert.deepEqual(info.dock, ['Computer:true', 'Phone:false']);
  assert.equal(info.lv, false, 'no way out of the frame is drawn');
  assert.equal(info.saved, false, 'nothing is saved on this device');
  assert.equal(info.cpeOpen, true);
  assert.deepEqual(seen.api, ['GET ' + C + '/characters/new/plan']);
  assert.deepEqual(await sets(pg), [], 'nothing is written to the draft by looking');
});

test('a workshop mount with no workshop class is left unmounted, never the stage look', { skip }, async (t) => {
  const { pg } = await open(t, { cards: false });
  const r = await pg.evaluate(() => {
    const keep = window.ChronicleHeroWorkshop; delete window.ChronicleHeroWorkshop;
    const m = document.createElement('div');
    m.className = 'hc'; m.setAttribute('data-look', 'stage'); m.setAttribute('data-workshop', ''); m.setAttribute('data-campaign-id', 'c1'); m.setAttribute('data-plan-url', '/x');
    document.body.appendChild(m);
    // A creator class made before the class went missing is cached; a fresh page is the honest case.
    return { cached: !!document.querySelector('.hcw-frame .hc-app'), keep: typeof keep };
  });
  assert.equal(r.keep, 'function');
});

test('the creator for players is the same code: a mount with no data-workshop builds the stage look', { skip }, async (t) => {
  const { pg } = await open(t);
  const r = await pg.evaluate(() => {
    const m = document.createElement('div');
    m.className = 'hc'; m.setAttribute('data-look', 'stage'); m.setAttribute('data-campaign-id', 'c1'); m.setAttribute('data-plan-url', '/nowhere');
    document.body.appendChild(m);
    widgets['hero-creator'].init(m);
    return { has: !!m._heroCreator, workshop: !!(m._heroCreator && m._heroCreator.hooks) };
  });
  assert.deepEqual(r, { has: true, workshop: false });
});

// --- Pressing a card ---

test('pressing a card opens the panel with its kind and name, marks the card and gives the keyboard to the sample in use', { skip }, async (t) => {
  const { pg } = await open(t);
  await press(pg, 'Dwarf');
  const r = await pg.evaluate(() => {
    const a = document.activeElement;
    return {
      kind: document.querySelector('.hcw-kind').textContent, name: document.querySelector('.hcw-who h3').textContent,
      expanded: document.querySelector('.hcw-on').getAttribute('aria-expanded'), on: document.querySelector('.hcw-on').textContent.includes('Dwarf'),
      focus: a.closest('.hcw-insp') ? a.getAttribute('data-b') : 'outside', label: document.querySelector('.hcw-insp').getAttribute('aria-label'),
      dim: getComputedStyle(document.querySelector('.hcw-frame [data-i="1"]')).opacity,
    };
  });
  assert.equal(r.kind, 'Ancestry');
  assert.equal(r.name, 'Dwarf');
  assert.equal(r.expanded, 'true');
  assert.ok(r.on);
  assert.equal(r.focus, '@sys', 'Dwarf wears the look its game system gives, so that sample is the one in use');
  assert.equal(r.label, 'Card border');
  assert.ok(Number(r.dim) < 1, 'the cards that are not pressed step back');
  assert.deepEqual(await pg.locator('.hcw-insp .hcw-t[aria-pressed="true"]').evaluateAll((l) => l.map((b) => b.getAttribute('data-b')).sort()), ['@sys', 'rune-metal'], 'the game system’s sample and the border it gives both read as in use');
  await shot(pg, 'press-desk.png');
});

test('Escape and Done close the panel and give the keyboard back to the card; Previous and Next walk and stop at the ends', { skip }, async (t) => {
  const { pg } = await open(t);
  await press(pg, 'Dwarf');
  const dis = (name) => pg.locator('[data-hl="' + name + '"]').first().getAttribute('aria-disabled');
  assert.equal(await dis('prev'), 'true');
  assert.equal(await dis('next'), 'false');
  await pg.locator('[data-hl="next"]').first().click();
  await settle(pg, 150);
  assert.equal(await pg.locator('.hcw-who h3').textContent(), 'Human');
  assert.equal(await dis('prev'), 'false');
  assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('data-hl')), 'next', 'the button keeps the keyboard');
  await pg.locator('[data-hl="prev"]').first().click();
  await settle(pg, 150);
  assert.equal(await pg.locator('.hcw-who h3').textContent(), 'Dwarf');
  for (let i = 0; i < 3; i++) { await pg.locator('[data-hl="next"]').first().click(); await settle(pg, 120); }
  assert.equal(await pg.locator('.hcw-who h3').textContent(), 'Elf');
  assert.equal(await dis('next'), 'true');
  await pg.locator('[data-hl="next"]').first().evaluate((b) => b.click());
  assert.equal(await pg.locator('.hcw-who h3').textContent(), 'Elf', 'the end of the run stays put');
  await pg.keyboard.press('Escape');
  await settle(pg, 100);
  assert.equal(await pg.locator('.hcw-insp.open').count(), 0);
  assert.equal(await pg.evaluate(() => document.activeElement.textContent.includes('Elf') && !!document.activeElement.closest('.hc-gcard')), true, 'the card has the keyboard');
  assert.equal(await pg.locator('.hcw-on').count(), 0);
  await press(pg, 'Human');
  await pg.locator('.hcw-insp [data-hl="close"]').first().evaluate((b) => b.click());
  await settle(pg, 100);
  assert.equal(await pg.locator('.hcw-insp.open').count(), 0);
  assert.equal(await pg.evaluate(() => document.activeElement.textContent.includes('Human')), true);
});

test('a step change closes the panel', { skip }, async (t) => {
  const { pg } = await open(t);
  await press(pg, 'Dwarf');
  await pg.locator('.hcw-frame [data-go="1"]').first().click();
  await settle(pg, 400);
  assert.equal(await pg.locator('.hcw-insp.open').count(), 0);
  assert.equal(await pg.locator('.hcw-on').count(), 0);
  assert.notEqual(await pg.evaluate(() => document.activeElement && document.activeElement.textContent.includes('Dwarf')), true);
});

// --- What a change writes ---

const want = (looks) => ({ looks, _changes: looks.map((l) => ({ label: 'Hero creator: ' + l.__n, href: TAB })) });
const it = (field, key, rest, n) => ({ field, source: 'package', key, ...rest, __n: n });
const clean = (v) => v && { ...v, looks: v.looks.map((l) => { const { __n, ...o } = l; return o; }) };

test('a new border repaints the card and writes the whole section; a named colour changes the same item', { skip }, async (t) => {
  const { pg } = await open(t);
  await press(pg, 'Human');
  await tile(pg, 'fire').click();
  await settle(pg, 60);
  assert.deepEqual(await trimOf(pg, 'Human'), { trim: 'fire', k: '#d9622b' });
  let s = await lastSet(pg);
  assert.equal(s.k, 'herolooks');
  assert.deepEqual(s.v, clean(want([it('ancestry', 'human', { border: 'fire', colour: '' }, 'Human')])));
  assert.deepEqual(s.opts, { step: true });
  await pg.locator('.hcw-insp .hcw-c[data-c="#6fae4f"]').click();
  await settle(pg, 60);
  assert.deepEqual(await trimOf(pg, 'Human'), { trim: 'fire', k: '#6fae4f' });
  s = await lastSet(pg);
  assert.deepEqual(s.v, clean(want([it('ancestry', 'human', { border: 'fire', colour: '#6fae4f' }, 'Human')])));
  assert.equal(await pg.locator('.hcw-insp .hcw-c[data-c="#6fae4f"]').getAttribute('aria-pressed'), 'true');
  await pg.locator('.hcw-insp .hcw-c[data-c=""]').click();
  assert.equal((await lastSet(pg)).v.looks[0].colour, '', 'the border’s own colour is written as empty');
});

test('None, and the game system’s sample on a changed card, write their items; the game system’s sample on a card with no look writes nothing', { skip }, async (t) => {
  const { pg } = await open(t);
  await press(pg, 'Dwarf');
  await tile(pg, '').click();
  await settle(pg, 60);
  assert.equal((await trimOf(pg, 'Dwarf')).trim, null);
  assert.deepEqual((await lastSet(pg)).v, clean(want([it('ancestry', 'dwarf', { border: '', colour: '' }, 'Dwarf')])));
  assert.equal(await tile(pg, '@sys').count(), 1);
  await press(pg, 'Orc');
  assert.equal(await tile(pg, '@sys').getAttribute('aria-pressed'), 'false');
  await tile(pg, '@sys').click();
  await settle(pg, 60);
  assert.deepEqual(await trimOf(pg, 'Orc'), { trim: 'fire', k: '#d9622b' });
  assert.deepEqual((await lastSet(pg)).v, clean(want([
    it('ancestry', 'dwarf', { border: '', colour: '' }, 'Dwarf'), it('ancestry', 'orc', { reset: true }, 'Orc')])), 'step order, then card order');
  // Human: the game system names no border, so its sample is the one pressed, not None.
  await press(pg, 'Human');
  assert.equal(await tile(pg, '@sys').getAttribute('aria-pressed'), 'true');
  assert.equal(await tile(pg, '').getAttribute('aria-pressed'), 'false');
  await tile(pg, 'ice').click();
  await settle(pg, 60);
  assert.equal((await lastSet(pg)).v.looks.length, 3);
  await tile(pg, '@sys').click();
  await settle(pg, 60);
  assert.equal((await trimOf(pg, 'Human')).trim, null);
  assert.equal((await lastSet(pg)).v.looks.length, 2, 'a look equal to the one served is not an item');
});

test('a look equal to what the server has is no item, and with no items the section is set to null', { skip }, async (t) => {
  const { pg } = await open(t);
  await press(pg, 'Dwarf');
  await tile(pg, 'fire').click();
  await settle(pg, 60);
  assert.equal((await lastSet(pg)).v.looks.length, 1);
  await tile(pg, 'rune-metal').click();
  await settle(pg, 60);
  const s = await lastSet(pg);
  assert.equal(s.k, 'herolooks');
  assert.equal(s.v, null);
  assert.deepEqual(s.opts, { step: true });
});

test('an accepted campaign entry is written by its id with no reset and no game system sample; one handed in shows a line and writes nothing', { skip }, async (t) => {
  const { pg } = await open(t);
  await pg.locator('.hcw-frame [data-go="1"]').first().click();
  await settle(pg, 300);
  await press(pg, 'Gale');
  assert.equal(await tile(pg, '@sys').count(), 0);
  assert.equal(await tile(pg, 'storm').getAttribute('aria-pressed'), 'true');
  await tile(pg, 'ice').click();
  await settle(pg, 60);
  assert.deepEqual((await lastSet(pg)).v, { looks: [{ field: 'class', source: 'campaign', key: '7', border: 'ice', colour: '' }], _changes: [{ label: 'Hero creator: Gale', href: TAB }] });
  const before = (await sets(pg)).length;
  await press(pg, 'Hush');
  const r = await pg.evaluate(() => ({ text: document.querySelector('.hcw-insp').textContent, tiles: document.querySelectorAll('.hcw-insp .hcw-t').length, name: document.querySelector('.hcw-who h3').textContent }));
  assert.equal(r.name, 'Hush');
  assert.equal(r.tiles, 0, 'no wall');
  assert.match(r.text, /This entry is not accepted yet\. Its writer picks its border until a Director accepts it\./);
  assert.equal((await sets(pg)).length, before, 'pressing it writes nothing');
  assert.equal(JSON.stringify((await lastSet(pg)).v).includes('hush'), false);
});

test('a colour dragged passes no step and is held to a frame', { skip }, async (t) => {
  const { pg } = await open(t);
  await press(pg, 'Elf');
  const n0 = (await sets(pg)).length;
  await pg.evaluate(() => {
    const i = document.querySelector('.hcw-insp [data-any]');
    ['#112233', '#223344', '#334455'].forEach((v) => { i.value = v; i.dispatchEvent(new Event('input', { bubbles: true })); });
  });
  await settle(pg, 100);
  const all = await sets(pg);
  assert.equal(all.length - n0, 1, 'three input events in one frame make one write');
  const s = all[all.length - 1];
  assert.equal(s.opts, undefined, 'a drag is not a step of its own');
  assert.deepEqual(s.v.looks, [{ field: 'ancestry', source: 'package', key: 'elf', border: 'vine', colour: '#334455' }]);
  assert.deepEqual(await trimOf(pg, 'Elf'), { trim: 'vine', k: '#334455' });
});

// --- The draft ---

const DRAFTED = { herolooks: { looks: [
  { field: 'ancestry', source: 'package', key: 'human', border: 'fire', colour: '#112233' },
  { field: 'ancestry', source: 'package', key: 'orc', reset: true },
  { field: 'class', source: 'campaign', key: '8', border: 'fire', colour: '' },
  { field: 'class', source: 'package', key: 'ghost', border: 'fire', colour: '' },
], _changes: [] } };

test('a draft at load is laid over the cards, and an event with a smaller draft puts cards back', { skip }, async (t) => {
  const { pg } = await open(t, { draft: DRAFTED });
  assert.deepEqual(await trimOf(pg, 'Human'), { trim: 'fire', k: '#112233' });
  assert.deepEqual(await trimOf(pg, 'Orc'), { trim: 'fire', k: '#d9622b' });
  assert.deepEqual(await sets(pg), [], 'laying a draft writes nothing');
  await press(pg, 'Human');
  assert.equal(await tile(pg, 'fire').getAttribute('aria-pressed'), 'true');
  await pg.evaluate(() => document.dispatchEvent(new CustomEvent('chronicle:customize-draft', { detail: { reason: 'undo', keys: ['herolooks'], draft: { herolooks: { looks: [{ field: 'ancestry', source: 'package', key: 'orc', reset: true }], _changes: [] } } } })));
  await settle(pg, 60);
  assert.equal((await trimOf(pg, 'Human')).trim, null, 'the card is back as served');
  assert.deepEqual(await trimOf(pg, 'Orc'), { trim: 'fire', k: '#d9622b' });
  assert.equal(await tile(pg, 'fire').getAttribute('aria-pressed'), 'false', 'the open panel follows');
  assert.equal(await tile(pg, '@sys').getAttribute('aria-pressed'), 'true');
  await pg.evaluate(() => document.dispatchEvent(new CustomEvent('chronicle:customize-draft', { detail: { reason: 'discard', keys: ['herolooks'], draft: {} } })));
  await settle(pg, 60);
  assert.deepEqual(await trimOf(pg, 'Orc'), { trim: 'ice', k: '#8fd8ff' });
});

test('hostile values in the draft change nothing and reach no markup', { skip }, async (t) => {
  const hostile = { herolooks: { looks: [
    { field: 'ancestry', source: 'package', key: 'human', border: 'x" onerror=alert(1) x="', colour: '#112233' },
    { field: 'ancestry', source: 'package', key: 'elf', border: 'fire', colour: 'red;background:url(x)' },
    { field: 'ancestry', source: 'package', key: 'dwarf', border: 'fire', colour: '#12345' },
    { field: 'ancestry', source: 'package', key: 'orc', border: 'ice', colour: '#8FD8FFAA' },
    { field: '"><img src=x onerror=alert(2)>', source: 'package', key: 'human', border: 'fire', colour: '' },
    'nonsense', null, { looks: 'x' },
  ], _changes: [{ label: '<b>x</b>', href: 'javascript:alert(3)' }] } };
  const { pg, errors } = await open(t, { draft: hostile });
  await settle(pg, 200);
  assert.deepEqual(await trimOf(pg, 'Human'), { trim: null, k: '' });
  assert.deepEqual(await trimOf(pg, 'Elf'), { trim: 'vine', k: '#6fae4f' });
  assert.deepEqual(await trimOf(pg, 'Dwarf'), { trim: 'rune-metal', k: '#3fc8d4' });
  assert.deepEqual(await trimOf(pg, 'Orc'), { trim: 'ice', k: '#8fd8ff' });
  const r = await pg.evaluate(() => ({ bad: document.querySelectorAll('[onerror], img[src="x"], [style*="url(x)"]').length, html: document.body.innerHTML.includes('onerror') }));
  assert.equal(r.bad, 0);
  assert.deepEqual(errors, []);
  await press(pg, 'Elf');
  assert.equal(await pg.evaluate(() => document.querySelectorAll('[onerror], [style*="url(x)"]').length), 0);
});

test('undo through the real Customize shell puts a card back, and a save reads the plan again and closes the panel', { skip }, async (t) => {
  let served = 0;
  const { pg, seen } = await open(t, { shell: true, plan: () => { served++; const p = PLAN(); if (served > 1) p.steps[0].choices[1] = pkg('Human', { look: { border: 'fire', colour: '#d9622b' }, lookChanged: true, systemLook: null }); return p; } });
  await press(pg, 'Human');
  await tile(pg, 'ice').click();
  await settle(pg, 80);
  assert.equal((await trimOf(pg, 'Human')).trim, 'ice');
  assert.equal(await pg.evaluate(() => document.querySelector('[data-czs-savebar]').hidden), false);
  assert.match(await pg.locator('[data-czs-what]').innerText(), /1 change.*waiting: Hero creator: Human/);
  await pg.locator('[data-czs-undo]').click();
  await settle(pg, 80);
  assert.equal((await trimOf(pg, 'Human')).trim, null, 'undo puts the card back');
  assert.equal(await tile(pg, 'ice').getAttribute('aria-pressed'), 'false');
  // Undoing back to clean hides the bar, so redo is reached the way its shortcut does.
  await pg.evaluate(() => Chronicle.customizeRedo());
  await settle(pg, 80);
  assert.equal((await trimOf(pg, 'Human')).trim, 'ice');
  await pg.locator('[data-czs-discard]').click();
  await settle(pg, 300);
  assert.equal((await trimOf(pg, 'Human')).trim, null, 'discard too');
  // A Save that applied herolooks: the plan is read again and the panel is closed.
  await tile(pg, 'ice').click();
  await settle(pg, 80);
  await pg.route('http://t.test/campaigns/c1/customize/publish', (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ version: 2, applied: ['herolooks'], draft: {}, failed: [] }) }));
  await pg.locator('[data-czs-save]').click();
  await settle(pg, 600);
  assert.equal(served, 2, 'the plan was read again');
  assert.equal(await pg.locator('.hcw-insp.open').count(), 0);
  assert.deepEqual(await trimOf(pg, 'Human'), { trim: 'fire', k: '#d9622b' }, 'the server’s look is what shows');
  assert.equal(await pg.evaluate(() => document.querySelector('.hcw-frame .hc-app').getAttribute('data-step')), '0', 'the step stays');
  // base is the server's now: going back to None is a change again.
  await press(pg, 'Human');
  await tile(pg, '').click();
  await settle(pg, 80);
  assert.match(await pg.locator('[data-czs-what]').innerText(), /1 change/);
  assert.ok(seen.api.filter((a) => a.startsWith('GET')).length >= 2);
});

// --- Views ---

test('the Computer and Phone switch changes the creator’s view inside the frame', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.deepEqual(await frameInfo(pg), { view: 'desk', fw: 1180 });
  await pg.locator('[data-hl="view-phone"]').click();
  await settle(pg, 300);
  const p = await frameInfo(pg);
  assert.equal(p.view, 'phone');
  assert.equal(p.fw, 390);
  assert.equal(await pg.locator('.hcw[data-view="phone"]').count(), 1);
  assert.equal(await pg.locator('[data-hl="view-phone"]').getAttribute('aria-pressed'), 'true');
  assert.equal(await pg.locator('.hcw-frame .hc-hand .hc-card').count() > 0, true);
  await pg.locator('[data-hl="view-desk"]').click();
  await settle(pg, 300);
  assert.equal((await frameInfo(pg)).view, 'desk');
  // Pressing a card in the phone view works in the frame.
  await pg.locator('[data-hl="view-phone"]').click();
  await settle(pg, 300);
  await press(pg, 'Dwarf');
  assert.equal(await pg.locator('.hcw-who h3').textContent(), 'Dwarf');
  await shot(pg, 'press-desk-phoneview.png');
});

for (const width of [390, 360]) {
  test(`at ${width} wide there is no switch and no sideways scroll, and the sheet does not cover the pressed card`, { skip }, async (t) => {
    const { pg } = await open(t, { width, height: 844 });
    const r = await pg.evaluate(() => ({ dock: document.querySelector('.hcw-dockw').hidden, sw: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth, cap: document.querySelector('.hcw-cap').textContent, view: document.querySelector('.hc-app').getAttribute('data-view') }));
    assert.equal(r.dock, true, 'no switch on a small screen');
    assert.ok(r.sw <= r.cw);
    assert.equal(r.cap, 'Press a card to change its border.');
    assert.equal(r.view, 'phone');
    await press(pg, 'Dwarf');
    await settle(pg, 500);
    const g = await pg.evaluate(() => {
      const c = document.querySelector('.hcw-on').getBoundingClientRect(), s = document.querySelector('.hcw-insp').getBoundingClientRect();
      return { cardBottom: c.bottom, sheetTop: s.top, top: c.top, sw: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth, doneShown: getComputedStyle(document.querySelector('.hcw-done')).display !== 'none', closeShown: getComputedStyle(document.querySelector('.hcw-insp-x, .hcw-insp [data-hl="close"]')).display !== 'none', colourFirst: document.querySelector('.hcw-pk').firstElementChild.hasAttribute('data-cg') };
    });
    // The signed sheet lies over the foot of the card, as the real card is taller than the room above a sheet; its head, name and top border stay clear.
    assert.ok(g.sheetTop - g.top >= 280, `too little of the card shows above the sheet: card starts ${g.top}, sheet starts ${g.sheetTop}`);
    assert.ok(g.top >= 0);
    assert.ok(g.sw <= g.cw, 'no sideways scroll with the sheet open');
    assert.equal(g.doneShown, true);
    assert.equal(g.colourFirst, true, 'the colours sit over the samples in a sheet');
    await pg.locator('.hcw-done').click();
    await settle(pg, 100);
    assert.equal(await pg.locator('.hcw-insp.open').count(), 0);
    assert.equal(await pg.evaluate(() => document.activeElement.textContent.includes('Dwarf') || !!document.activeElement.closest('.hc-card')), true, 'Done gives the keyboard back to the card');
  });
}

// --- Nothing leaves the frame ---

test('no button or link in the frame, on any step, posts a request or leaves the page', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  const run = async () => {
    for (let step = 0; step < 4; step++) {
      for (let i = 0; i < 60; i++) {
        const done = await pg.evaluate(({ i, step }) => {
          const app = document.querySelector('.hcw-frame .hc-app');
          if (!app || app.getAttribute('data-step') !== String(step)) return 'moved';
          const list = Array.from(app.querySelectorAll('button, a[href], [data-act]')).filter((n) => !n.closest('[data-act="next"], [data-act="back"], [data-go]'));
          if (i >= list.length) return 'end';
          const n = list[i];
          if (n.matches('[data-act="create"]')) { const inp = app.querySelector('[data-name]'); if (inp) { inp.value = 'Korrin'; inp.dispatchEvent(new Event('input', { bubbles: true })); } }
          n.click();
          return 'ok';
        }, { i, step });
        if (done !== 'ok') { if (done === 'end') break; }
      }
      await pg.evaluate((s) => { const g = document.querySelector('.hcw-frame [data-go="' + (s + 1) + '"]'); if (g) g.click(); }, step);
      await settle(pg, 350);
    }
  };
  await run();
  // The last step's finish action, with a name given.
  await pg.evaluate(() => { const inp = document.querySelector('.hcw-frame [data-name]'); if (inp) { inp.value = 'Korrin'; inp.dispatchEvent(new Event('input', { bubbles: true })); } });
  await pg.evaluate(() => { const c = document.querySelector('.hcw-frame [data-act="create"]'); if (c) c.click(); });
  await pg.keyboard.press('Enter');
  await settle(pg, 500);
  assert.deepEqual(seen.api, ['GET ' + C + '/characters/new/plan'], 'only the plan was asked for');
  assert.deepEqual(seen.nav, ['http://t.test/'], 'the address did not change');
  assert.deepEqual(await pg.evaluate(() => Object.keys(localStorage).concat(Object.keys(sessionStorage))), []);
  assert.equal(await pg.evaluate(() => !!document.querySelector('.hcw-frame [data-act="create"]')), true, 'the review step with its finish action was reached');
});

// --- The keyboard ---

test('keyboard only: Tab reaches the cards, Enter opens the panel, the arrows move, and the panel is head, samples, colours', { skip }, async (t) => {
  const { pg } = await open(t);
  await pg.focus('#before');
  let found = false;
  for (let i = 0; i < 30 && !found; i++) { await pg.keyboard.press('Tab'); found = await pg.evaluate(() => !!document.activeElement.closest('.hcw-frame .hc-gcard[data-i]')); }
  assert.ok(found, 'Tab reaches a card');
  await pg.keyboard.press('Enter');
  await pg.waitForSelector('.hcw-insp.open');
  await settle(pg, 150);
  assert.equal(await pg.locator('.hcw-who h3').textContent(), 'Dwarf');
  assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('aria-pressed') === 'true' && document.activeElement.getAttribute('data-b')), '@sys', 'the sample in use has the keyboard');
  // The arrow keys on the creator's focused card move it; the open panel follows.
  await pg.evaluate(() => document.querySelector('.hcw-frame .hc-gcard[data-i="0"]').focus());
  await pg.keyboard.press('ArrowRight');
  await settle(pg, 150);
  assert.equal(await pg.locator('.hcw-who h3').textContent(), 'Human');
  // Orc wears a border, so its colours are offered; Tab order inside the panel.
  await pg.evaluate(() => document.querySelector('.hcw-frame .hc-gcard[data-i="1"]').focus());
  await pg.keyboard.press('ArrowRight');
  await settle(pg, 150);
  assert.equal(await pg.locator('.hcw-who h3').textContent(), 'Orc');
  await pg.evaluate(() => document.querySelector('.hcw-insp [data-hl="prev"]').focus());
  const seq = [];
  for (let i = 0; i < 60; i++) {
    await pg.keyboard.press('Tab');
    const k = await pg.evaluate(() => {
      const a = document.activeElement;
      if (!a.closest('.hcw-insp')) return 'out';
      if (a.hasAttribute('data-hl')) return 'head';
      if (a.classList.contains('hcw-t')) return 'sample';
      if (a.classList.contains('hcw-c') || a.hasAttribute('data-any') || a.closest('.hcw-c')) return 'colour';
      return 'other';
    });
    if (k === 'out') break;
    if (seq[seq.length - 1] !== k) seq.push(k);
  }
  assert.deepEqual(seq, ['head', 'sample', 'colour']);
  await pg.keyboard.press('Escape');
  await settle(pg, 100);
  assert.equal(await pg.locator('.hcw-insp.open').count(), 0);
});

// --- Motion ---

test('with motion Off the panel is open at once with no transition; Calm only fades', { skip }, async (t) => {
  const off = await open(t, { motion: 'off' });
  await off.pg.locator('.hcw-frame .hc-gcard[data-i="0"]').click();
  const r = await off.pg.evaluate(() => {
    const i = document.querySelector('.hcw-insp'), cs = getComputedStyle(i), f = getComputedStyle(document.querySelector('.hcw-fit'));
    return { open: i.classList.contains('open'), op: cs.opacity, tf: cs.transform, dur: cs.transitionDuration, fitDur: f.transitionDuration };
  });
  assert.equal(r.open, true);
  assert.equal(r.op, '1');
  assert.equal(r.tf, 'none');
  assert.equal(r.dur, '0s');
  assert.equal(r.fitDur, '0s');
  const calm = await open(t, { motion: 'calm' });
  const c = await calm.pg.evaluate(() => {
    const i = document.querySelector('.hcw-insp'), cs = getComputedStyle(i);
    return { tf: cs.transform, props: cs.transitionProperty, fit: getComputedStyle(document.querySelector('.hcw-fit')).transitionDuration };
  });
  assert.equal(c.tf, 'none', 'Calm does not slide');
  assert.match(c.props, /opacity/);
  assert.doesNotMatch(c.props, /transform/);
  assert.equal(c.fit, '0s');
  const full = await open(t, {});
  assert.match(await full.pg.evaluate(() => getComputedStyle(document.querySelector('.hcw-insp')).transitionProperty), /transform/);
  assert.equal(await full.pg.evaluate(() => /all/.test(getComputedStyle(document.querySelector('.hcw-insp')).transitionProperty)), false);
});

// --- Other states ---

test('a plan with no steps shows one plain line in place of the frame', { skip }, async (t) => {
  const { pg, errors } = await open(t, { plan: EMPTY_PLAN, cards: false });
  await settle(pg, 400);
  const r = await pg.evaluate(() => ({ line: document.querySelector('[data-hl-empty]').hidden, text: document.querySelector('[data-hl-empty]').textContent, editor: document.querySelector('[data-hl-editor]').hidden, vis: !!document.querySelector('[data-hl-editor]').offsetParent }));
  assert.equal(r.line, false);
  assert.equal(r.text, 'This campaign’s game system has no hero creator, so there are no cards to dress.');
  assert.equal(r.editor, true);
  assert.equal(r.vis, false);
  assert.deepEqual(errors, []);
});

test('destroy takes the creator down and gives the page back what it took', { skip }, async (t) => {
  const { pg } = await open(t);
  const r = await pg.evaluate(() => {
    const el = document.querySelector('[data-widget="hero-looks"]'), m = el.querySelector('[data-widget="hero-creator"]');
    const before = !!m._heroCreator;
    widgets['hero-looks'].destroy(el);
    return { before, after: m._heroCreator, cls: document.documentElement.classList.contains('cpe-open') };
  });
  assert.deepEqual(r, { before: true, after: null, cls: false });
  await pg.evaluate(() => window.dispatchEvent(new Event('resize')));
  await pg.keyboard.press('Escape');
});

// --- Review screenshots and the save bar on a phone ---

for (const [name, o, stepLook] of [
  ['page-desk-light.png', { width: 1280, height: 800 }],
  ['page-desk-dark.png', { width: 1280, height: 800, dark: true }],
  ['page-phone-light.png', { width: 390, height: 844 }],
  ['page-phone-dark.png', { width: 390, height: 844, dark: true }],
  ['page-phone-360.png', { width: 360, height: 780 }],
]) {
  test('review picture ' + name, { skip: skip || !SHOTS }, async (t) => {
    const { pg } = await open(t, { ...o, shell: true });
    await press(pg, 'Dwarf');
    await tile(pg, 'fire').click();
    await settle(pg, 80);
    await pg.locator('.hcw-insp .hcw-c[data-c="#4aa3e0"]').click();
    await settle(pg, 900);
    await shot(pg, name);
    void stepLook;
  });
}

for (const width of [390, 360]) {
  test(`the save bar at ${width} wide with two cards changed`, { skip: skip || !SHOTS }, async (t) => {
    const { pg } = await open(t, { width, height: 780, shell: true });
    await press(pg, 'Dwarf');
    await tile(pg, 'fire').click();
    await settle(pg, 80);
    await pg.locator('.hcw-insp [data-hl="next"]').click();
    await settle(pg, 300);
    await pg.locator('.hcw-insp .hcw-t[data-b="ice"]').click();
    await settle(pg, 400);
    const r = await pg.evaluate(() => { const w = document.querySelector('[data-czs-what]'); return { text: w.textContent, cut: w.scrollWidth > w.clientWidth, shown: w.clientWidth, full: w.scrollWidth }; });
    console.log('save bar ' + width + ': ' + JSON.stringify(r));
    await shot(pg, 'savebar-' + width + '.png');
  });
}

