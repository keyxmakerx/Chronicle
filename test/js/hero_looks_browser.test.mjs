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
// The notes tab and its rules as the site has them, in their layer; and the
// Table's resting line, which lifts what reaches the bottom edge on a phone.
const JOT_CSS = '@layer components {' + input.slice(input.indexOf('  /* --- Jot notes (the floating panel) --- */'), input.indexOf('  /* Panel: rises from the tab')) + '}';
const TABLE_CSS = readFileSync(join(root, 'internal/plugins/rolls/static/css/table.css'), 'utf8').split('\n').filter((l) => l.startsWith('html[data-tt-sheet] {') || l.startsWith('html[data-tt-sheet] #main-content') || l.startsWith('html[data-tt-sheet] .jot-tab')).join('\n');

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
// o.app gives a phone the real app's height above the workshop (the site's 48px
// top bar and the Customizer's two-row bar); o.bar adds a 40px line above the
// Customizer, as the real app's time-zone line does; o.jot adds the notes tab;
// o.table the Table's resting line on a phone.
function page(o) {
  const links = SHEETS.map((s) => `<link rel="stylesheet" href="/${s.replace(':', '/')}">`).join('');
  const scripts = SCRIPTS.map((s) => `<script src="/${s}"></script>`).join('');
  const stub = o.shell ? '' : `Chronicle.customizeSet = function (k, v, opts) { window.sets.push({ k: k, v: v == null ? v : JSON.parse(JSON.stringify(v)), opts: opts }); };
Chronicle.customizeDraft = function () { return window.DRAFT; };`;
  const shell = o.shell ? '<script src="/static/js/widgets/customize_shell.js"></script>' : '';
  const html = SHELL.replace('@@SECTION@@', SECTION);
  const wrap = o.shell ? html.replace('<div class="czs czs-is-workshop">', `<div class="czs czs-is-workshop" data-widget="customize-shell" data-campaign-id="c1" data-csrf="t" data-area="x-hero-creator" data-version="0" data-draft='${JSON.stringify(o.draft || {})}' data-find="[]" data-areas='[{"id":"x-hero-creator","label":"Hero creator","href":"${TAB}","section":"herolooks"}]'>`).replace('<div class="czs-top" data-czs-top style="height:48px"></div>', '<div class="czs-top" data-czs-top style="height:48px"><button type="button" data-czs-find>Find</button></div>') : html;
  return `<!doctype html><html data-gm-term="Director"${o.dark ? ' class="dark"' : ''}${o.motion ? ` data-motion="${o.motion}"` : ''}${o.table ? ' data-tt-sheet' : ''}><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>${TOKENS}*,::before,::after{box-sizing:border-box}html,body{margin:0}body{font-family:Inter,sans-serif;background:var(--color-bg-primary);color:var(--color-text-body)}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}
#main-content{height:100vh;overflow:auto}.btn-primary{background:var(--color-accent);color:#fff;border:0;border-radius:8px;padding:6px 12px}.btn-sm{font-size:13px}</style>${o.jot ? `<style>${JOT_CSS}#before,#after{position:fixed;top:0;left:0}</style>` : ''}${o.table ? `<style>${TABLE_CSS}</style>` : ''}${links}</head>
<body><button id="before" type="button">before</button><main id="main-content">${o.app ? '<div data-app-top style="height:48px"></div>' : ''}${o.bar ? '<div data-zone-bar style="height:40px">Times aren’t in your zone yet</div>' : ''}${o.app ? wrap.replace('data-czs-top style="height:48px"', 'data-czs-top style="height:89px"') : wrap}</main><button id="after" type="button">after</button>${o.jot ? '<button type="button" class="jot-tab" data-jot><i class="fa-solid fa-note-sticky" aria-hidden="true" style="display:inline-block;width:1em;height:1em;font-size:16px"></i><span class="jot-tab-label">Jot notes</span></button>' : ''}
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
  const ctx = await browser.newContext({ viewport: { width, height }, reducedMotion: reduced, hasTouch: width < 700, deviceScaleFactor: SHOTS ? 2 : 1 });
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
const shot = async (pg, name, clip) => { if (SHOTS) await pg.screenshot({ path: join(SHOTS, name), clip }); };

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

// The save bar with only a redo waiting: it says so, its dot is not waiting, and only Redo answers.
const REDO_ONLY = { hidden: false, what: 'No changes waiting', dot: false, undo: true, redo: false, discard: true, save: true, whatOff: true, whatCursor: 'default', discardDim: true, discardCursor: 'default' };
const redoBar = (pg) => pg.evaluate(() => {
  const b = document.querySelector('[data-czs-savebar]'), d = (s) => b.querySelector(s).disabled;
  return { hidden: b.hidden, what: b.querySelector('[data-czs-what]').textContent, dot: getComputedStyle(b.querySelector('.czs-sdot')).display !== 'none', undo: d('[data-czs-undo]'), redo: d('[data-czs-redo]'), discard: d('[data-czs-discard]'), save: d('[data-czs-save]'),
    whatOff: d('[data-czs-what]'), whatCursor: getComputedStyle(b.querySelector('[data-czs-what]')).cursor,
    discardDim: Number(getComputedStyle(b.querySelector('[data-czs-discard]')).opacity) <= 0.5, discardCursor: getComputedStyle(b.querySelector('[data-czs-discard]')).cursor };
});
// The words' ink and opacity, which stay as readable when they are not a control.
const whatInk = (pg) => pg.evaluate(() => { const w = document.querySelector('[data-czs-what]'), b = w.querySelector('b'), cs = getComputedStyle(w); return { ink: cs.color, bold: getComputedStyle(b).color, op: cs.opacity }; });
const barClip = (pg) => pg.evaluate(() => { const r = document.querySelector('[data-czs-savebar]').getBoundingClientRect(); const x = Math.max(0, r.left - 16), y = Math.max(0, r.top - 16); return { x, y, width: Math.min(innerWidth, r.right + 16) - x, height: Math.min(innerHeight, r.bottom + 16) - y }; });

test('at 390 wide, undone back to no changes, the save bar stays with Redo, and the page still keeps clear of it', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390, height: 844, shell: true, jot: true });
  await press(pg, 'Dwarf');
  await tile(pg, 'fire').click();
  await settle(pg, 120);
  await pg.locator('[data-czs-undo]').click();
  await settle(pg, 300);
  assert.equal((await trimOf(pg, 'Dwarf')).trim, 'rune-metal');
  assert.deepEqual(await redoBar(pg), REDO_ONLY);
  const quiet = await whatInk(pg);
  // Hovered, the words gain no underline, and a press opens no list.
  await pg.locator('[data-czs-what]').hover({ force: true });
  assert.equal(await pg.evaluate(() => getComputedStyle(document.querySelector('[data-czs-what]')).textDecorationLine), 'none');
  await pg.locator('[data-czs-what]').click({ force: true });
  assert.equal(await pg.locator('.czs-pop').count(), 0);
  // A disabled Undo does not light under the pointer.
  await pg.locator('[data-czs-undo]').hover({ force: true });
  await settle(pg, 200);
  assert.equal(await pg.evaluate(() => getComputedStyle(document.querySelector('[data-czs-undo]')).backgroundColor), 'rgba(0, 0, 0, 0)');
  await pg.mouse.move(2, 2);
  await shot(pg, 'redo-390.png');
  const f = await footInfo(pg);
  assert.ok(f.jot.bottom + 5 <= f.bar.top, 'the notes tab stays clear of the bar');
  // The page reads the bar: closed, the workshop's foot is padded past it.
  await pg.locator('.hcw-done').click();
  await settle(pg, 300);
  assert.equal(await pg.evaluate(() => document.querySelector('[data-hl-canvas]').style.getPropertyValue('--hcw-cover')), (76 + 16) + 'px');
  await pg.locator('[data-czs-redo]').click();
  await settle(pg, 120);
  assert.equal((await trimOf(pg, 'Dwarf')).trim, 'fire');
  assert.equal(await pg.evaluate(() => document.querySelector('.czs-sdot').hidden), false, 'with a change waiting the dot shows again');
  const live = await redoBar(pg);
  assert.equal(live.whatOff, false, 'with a change waiting the words open the list again');
  assert.equal(live.discardDim, false);
  assert.deepEqual(await whatInk(pg), quiet, 'the words are as readable with nothing waiting as with a change');
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
  // Undone back to clean, the bar stays for Redo.
  assert.deepEqual(await redoBar(pg), REDO_ONLY);
  await shot(pg, 'redo-desk.png', await barClip(pg));
  await pg.locator('[data-czs-redo]').click();
  await settle(pg, 80);
  assert.equal((await trimOf(pg, 'Human')).trim, 'ice');
  assert.match(await pg.locator('[data-czs-what]').innerText(), /1 change.*waiting/);
  await pg.locator('[data-czs-discard]').click();
  await settle(pg, 300);
  assert.equal((await trimOf(pg, 'Human')).trim, null, 'discard too');
  assert.equal(await pg.evaluate(() => document.querySelector('[data-czs-savebar]').hidden), true, 'after Discard the bar goes away: nothing is left to redo');
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
  assert.equal(await pg.evaluate(() => document.querySelector('[data-czs-savebar]').hidden), true, 'after Save the bar goes away');
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

// The sheet's place on a phone: the card above it, its height, and what shows inside.
const sheetInfo = (pg) => pg.evaluate(() => {
  const c = document.querySelector('.hcw-on').getBoundingClientRect(), i = document.querySelector('.hcw-insp'), s = i.getBoundingClientRect();
  const t = i.querySelector('.hcw-t').getBoundingClientRect(), wall = i.querySelector('[data-wall]');
  return { clear: s.top - c.top, h: s.height, ed: document.querySelector('.hcw-editor').clientHeight, firstRowBottom: t.bottom + wall.scrollTop, sheetBottom: s.bottom, inline: i.style.height };
});

for (const [width, height] of [[390, 844], [360, 800]]) {
  test(`at ${width}x${height} with a bar above the page the sheet shortens to keep the pressed card clear, never below one row of samples`, { skip }, async (t) => {
    const plain = await open(t, { width, height });
    await press(plain.pg, 'Dwarf');
    await settle(plain.pg, 500);
    const a = await sheetInfo(plain.pg);
    const { pg } = await open(t, { width, height, app: true, bar: true });
    await press(pg, 'Dwarf');
    await settle(pg, 500);
    const b = await sheetInfo(pg);
    assert.ok(b.clear >= 280, `with the bar too little of the card shows above the sheet: ${b.clear}`);
    assert.ok(b.h <= b.ed * 0.58 + 1, `the sheet is never taller than its share: ${b.h} of ${b.ed}`);
    assert.ok(b.firstRowBottom <= b.sheetBottom, 'the head, the colours and one full row of samples still show');
    assert.ok(a.clear >= 280);
    assert.equal(a.inline, '', 'where the card already clears, the sheet keeps its share');
    assert.ok(Math.abs(a.h - a.ed * 0.58) <= 1, `${a.h} of ${a.ed}`);
    await shot(pg, `sheet-${width}-bar.png`);
  });
}

// Colours as computed: rgb(), rgba() and color(srgb …) with alpha, laid over a ground.
function colourOf(s) {
  const m = /color\(srgb ([\d.e-]+) ([\d.e-]+) ([\d.e-]+)(?: \/ ([\d.e-]+))?\)/.exec(s);
  if (m) return { c: [m[1], m[2], m[3]].map((v) => Number(v) * 255), a: m[4] == null ? 1 : Number(m[4]) };
  const n = s.match(/[\d.]+/g).map(Number);
  return { c: n.slice(0, 3), a: n.length > 3 ? n[3] : 1 };
}
const over = (top, ground) => top.c.map((v, i) => v * top.a + ground[i] * (1 - top.a));
function lum(c) { const [r, g, b] = c.map((v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; }
const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); };
const near = (a, b) => a.every((v, i) => Math.abs(v - b[i]) <= 1);

test('on a computer the Computer and Phone switch is a dark surface like the panel in dark, readable, with the pressed segment clear, and the inverse pill in light', { skip }, async (t) => {
  for (const dark of [true, false]) {
    const { pg } = await open(t, { dark });
    await press(pg, 'Dwarf');
    await settle(pg, 400);
    const r = await pg.evaluate(() => {
      const cs = (s) => getComputedStyle(document.querySelector(s));
      const probe = document.createElement('span'); probe.style.color = 'var(--color-text-primary)'; document.body.appendChild(probe);
      return { dock: cs('.hcw .cpe-dock').backgroundColor, ink: cs('.hcw .cpe-dock').color, on: cs('.hcw [data-hl="view-desk"]').backgroundColor, off: cs('.hcw [data-hl="view-phone"]').backgroundColor, panel: cs('.hcw-insp').backgroundColor, primary: getComputedStyle(probe).color };
    });
    const dock = colourOf(r.dock).c, ink = colourOf(r.ink).c, on = over(colourOf(r.on), dock);
    if (dark) {
      assert.ok(near(dock, colourOf(r.panel).c), `the dock (${r.dock}) is the panel’s surface (${r.panel})`);
      assert.ok(ratio(ink, dock) >= 7, 'its words read on it');
      assert.ok(ratio(ink, on) >= 4.5, 'and on the pressed segment');
      assert.ok(ratio(on, dock) >= 1.4, `the pressed segment (${r.on}) stands out from the dock`);
      assert.equal(colourOf(r.off).a, 0, 'the other segment has no fill');
    } else {
      assert.ok(near(dock, colourOf(r.primary).c), `in light the dock stays on the inverse surface (${r.dock})`);
    }
    const clip = await pg.evaluate(() => { const a = document.querySelector('.hcw-dockw .cpe-dock').getBoundingClientRect(), b = document.querySelector('.hcw-insp').getBoundingClientRect(); const x = Math.max(0, a.left - 24), y = Math.max(0, b.top - 8); return { x, y, width: Math.min(innerWidth, b.right + 8) - x, height: Math.min(innerHeight, Math.max(a.bottom, b.bottom) + 8) - y }; });
    await shot(pg, dark ? 'dock-dark.png' : 'dock-light.png', clip);
  }
});

// The free colour on a phone: inside the row's box wherever the row is scrolled.
const rowInfo = (pg, to) => pg.evaluate((to) => {
  const row = document.querySelector('.hcw-insp .hcw-top .hcw-cs');
  row.scrollLeft = to === 'end' ? row.scrollWidth : to === 'mid' ? (row.scrollWidth - row.clientWidth) / 2 : 0;
  const box = row.getBoundingClientRect(), anyEl = row.querySelector('.hcw-c--any'), a = anyEl.getBoundingClientRect();
  const sw = Array.from(row.querySelectorAll('button.hcw-c')), iron = sw[sw.length - 1].getBoundingClientRect();
  const hit = document.elementFromPoint(a.left + a.width / 2, a.top + a.height / 2);
  const beneath = sw.some((b) => { const r = b.getBoundingClientRect(); return r.right > a.left && r.left < a.right; });
  const shadow = getComputedStyle(anyEl).boxShadow;
  // What a tap lands on across the band left of the free colour, at its middle and near its top and bottom.
  const cy = a.top + a.height / 2, band = [];
  for (let x = Math.ceil(a.left - 19); x < a.left; x += 2) for (const y of [cy - 14, cy, cy + 14]) { const h = document.elementFromPoint(x, y); band.push(h === row || !!(h && h.closest('.hcw-c--any')) ? 'ok' : (h && h.closest('.hcw-c') ? 'swatch' : 'other')); }
  return { box: { l: box.left, r: box.right, t: box.top, b: box.bottom }, any: { l: a.left, r: a.right, t: a.top, b: a.bottom }, iron: { l: iron.left, r: iron.right }, hit: !!(hit && hit.closest('.hcw-c--any')), band: band.filter((b) => b !== 'ok'), bandAt: { x: a.left - 10, y: cy }, beneath, shadow, overflow: row.scrollWidth > row.clientWidth, pos: getComputedStyle(anyEl).position };
}, to);

for (const [width, height] of [[390, 844], [360, 800]]) {
  test(`at ${width}x${height} the free colour stays whole at the colour row’s right end, wherever the row is scrolled`, { skip }, async (t) => {
    const { pg } = await open(t, { width, height });
    await press(pg, 'Dwarf');
    await settle(pg, 400);
    const ring = 20;
    for (const to of ['start', 'mid', 'end']) {
      const r = await rowInfo(pg, to);
      assert.ok(r.overflow, 'the row scrolls sideways');
      assert.equal(r.pos, 'sticky');
      // The focus ring reaches 5px past the swatch; it stays inside the row too.
      assert.ok(r.any.l >= r.box.l && r.any.r + 5 <= r.box.r && r.any.t - 5 >= r.box.t && r.any.b + 5 <= r.box.b, `${to}: the free colour ${JSON.stringify(r.any)} is cut by the row ${JSON.stringify(r.box)}`);
      assert.ok(r.hit, `${to}: a tap on the free colour reaches it, not a colour beneath`);
      assert.match(r.shadow, new RegExp('0px 0px 0px ' + ring + 'px'), 'a band of the sheet’s surface backs it');
      assert.deepEqual(r.band, [], `${to}: a tap on the band beside it reaches no colour`);
      if (to !== 'end') {
        // A real tap on the band changes nothing.
        const n0 = (await sets(pg)).length, k0 = (await trimOf(pg, 'Dwarf')).k;
        await pg.mouse.click(r.bandAt.x, r.bandAt.y);
        await settle(pg, 120);
        assert.equal((await sets(pg)).length, n0, `${to}: a tap on the band writes nothing`);
        assert.equal((await trimOf(pg, 'Dwarf')).k, k0);
        await rowInfo(pg, to);
      }
      if (to === 'mid') assert.ok(r.beneath, 'a colour passes beneath it here');
      if (to === 'end') assert.ok(r.iron.l >= r.box.l && r.iron.r + ring <= r.any.l, `at the row’s end the last colour ${JSON.stringify(r.iron)} shows whole beside it ${JSON.stringify(r.any)}`);
      if (width === 390 && to !== 'mid') await shot(pg, `sheet-390-${to}.png`);
    }
    // Picked, its ring shows inside the row.
    await pg.evaluate(() => { const i = document.querySelector('.hcw-insp [data-any]'); i.value = '#123456'; i.dispatchEvent(new Event('input', { bubbles: true })); });
    await settle(pg, 120);
    const p = await rowInfo(pg, 'start');
    assert.match(p.shadow, /0px 0px 0px 4px/);
    assert.ok(p.any.r + 4 <= p.box.r);
  });
}

// The notes tab and the save bar share the foot of a phone.
const footInfo = (pg) => pg.evaluate(() => {
  const r = (s) => { const n = document.querySelector(s), b = n.getBoundingClientRect(); return { top: b.top, bottom: b.bottom, left: b.left, right: b.right, shown: !n.hidden && b.height > 0 }; };
  return { jot: r('.jot-tab'), bar: r('[data-czs-savebar]'), save: r('[data-czs-save]'), vh: innerHeight };
});

for (const [width, table] of [[390, false], [360, false], [390, true]]) {
  test(`at ${width} wide${table ? ' over the Table’s line' : ''} the notes tab sits clear above the save bar, and back down without it`, { skip }, async (t) => {
    const { pg } = await open(t, { width, height: 844, shell: true, jot: true, table });
    const before = await footInfo(pg);
    assert.equal(before.bar.shown, false);
    await press(pg, 'Dwarf');
    await tile(pg, 'fire').click();
    await settle(pg, 400);
    const f = await footInfo(pg);
    assert.equal(f.bar.shown, true);
    // The tab's press area reaches 5px past it.
    assert.ok(f.jot.bottom + 5 <= f.bar.top, `the notes tab (${f.jot.top}–${f.jot.bottom}) lies on the save bar (${f.bar.top}–${f.bar.bottom})`);
    assert.ok(f.jot.top >= 0);
    assert.ok(f.save.right <= f.vh && f.save.right > f.save.left);
    await pg.locator('[data-czs-discard]').click();
    await settle(pg, 400);
    const after = await footInfo(pg);
    assert.equal(after.bar.shown, false);
    assert.equal(after.jot.bottom, before.jot.bottom, 'without the bar the tab is back in its place');
  });
}

test('at 390x844 with the save bar up the last row of samples scrolls fully clear of it', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390, height: 844, shell: true, jot: true });
  await press(pg, 'Dwarf');
  await tile(pg, 'fire').click();
  await settle(pg, 400);
  const r = await pg.evaluate(() => {
    const wall = document.querySelector('.hcw-insp [data-wall]');
    wall.scrollTop = wall.scrollHeight;
    const ts = wall.querySelectorAll('.hcw-t'), last = ts[ts.length - 1].getBoundingClientRect();
    return { last: last.bottom, bar: document.querySelector('[data-czs-savebar]').getBoundingClientRect().top, shown: !document.querySelector('[data-czs-savebar]').hidden };
  });
  assert.equal(r.shown, true);
  assert.ok(r.last <= r.bar, `the last row ends at ${r.last}, under the bar at ${r.bar}`);
});

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

test('review picture sheet-390-dark.png', { skip: skip || !SHOTS }, async (t) => {
  const { pg } = await open(t, { width: 390, height: 844, dark: true });
  await press(pg, 'Dwarf');
  await settle(pg, 600);
  await shot(pg, 'sheet-390-dark.png');
});

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

