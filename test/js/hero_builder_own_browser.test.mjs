// hero_builder_own_browser.test.mjs — "Make your own" in the Characters-page
// hero builder panel.
//
// Runs hero_builder.js with the shared own_entry_editor.js through boot.js's
// on-sight loader in a real Chromium, with the plan, grants and entry API
// answered here. It pins that the make row shows only while a permission is
// unspent, that the shared editor opens in the panel, that a saved or handed-in
// entry is picked for the hero with a field save, and that a draft is marked
// and can be edited.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';
import { bordersAttrs, serveBorders, contrast, INFO } from './_card_border_fixture.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const C = '/campaigns/c1';
const plan = (canMake, extra = []) => ({
  systemName: 'Draw Steel',
  steps: [
    { fieldKey: 'ancestry', label: 'Ancestry', canMake, choices: [
      { slug: 'human', name: 'Human', summary: 'Quick to adapt.', source: 'Draw Steel', properties: { size: '1M' } }, ...extra] },
    { fieldKey: 'class', label: 'Class', choices: [{ slug: 'fury', name: 'Fury', summary: 'Rage.', source: 'Draw Steel', properties: {} }] },
  ],
});
const attr = (v) => JSON.stringify(v).replace(/&/g, '&amp;').replace(/'/g, '&#39;');
const page = (heroes, o = {}) => `<!doctype html><html data-gm-term="Director"><head><meta charset="utf-8">
<style>:root{--color-accent:#4f46e5;--color-bg-primary:#f9fafb;--color-bg-secondary:#fff;--color-bg-tertiary:#f3f4f6;--color-border:#e5e7eb;--color-text-primary:#111827;--color-text-body:#374151;--color-text-secondary:#6b7280;
--dur-micro:120ms;--dur-standard:200ms;--dur-large:280ms;--dur-slide:360ms;--ease-out:cubic-bezier(.16,1,.3,1);--ease-slide:cubic-bezier(.22,.8,.24,1)}
html,body{margin:0;font:14px system-ui}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style></head><body>
<main id="main-content" style="padding:24px 16px"><div id="cast-content">
<a href="/campaigns/c1/characters/new" data-create-hero="true">Create hero</a>
<link rel="stylesheet" href="/static/plugins/entities/css/hero_builder.css">
<section class="hb" data-widget="hero-builder" data-plan-url="${C}/characters/new/plan"
  data-fields-base="${C}/entities/" data-campaign-id="c1" data-csrf="tok"
  data-heroes='${attr(heroes)}'${o.lib ? bordersAttrs({ pick: o.pick !== false }) : ''}><p class="hb-wait">Opening the hero builder…</p></section>
</div></main>
<script type="application/json" id="chronicle-widget-scripts">{"hero-builder":["/static/plugins/entities/js/hero_rules_lines.js"${o.scripts === false ? '' : ',"/static/plugins/entities/js/hero_borders.js","/static/plugins/entities/js/hero_border_picker.js"'},"/static/plugins/entities/js/own_entry_editor.js","/static/plugins/entities/js/hero_builder.js"]}</script>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} };</script>
<script src="/static/js/boot.js"></script></body></html>`;

const staticFile = (p) => p.startsWith('/static/plugins/entities/') ? join(root, 'internal/plugins/entities/static', p.slice('/static/plugins/entities/'.length)) : join(root, p);

async function open(t, { canMake = true, entries = [], extra = [], heroes = [{ id: 'e1', name: 'Tess', values: {}, hidden: [] }], width = 1280, lib = false, pick = true, scripts = true, dark = false } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: 'reduce', colorScheme: dark ? 'dark' : 'light' });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  const seen = { post: [], put: [] };
  const store = entries.slice();
  await p.route('**/*', (route) => {
    const req = route.request(), u = new URL(req.url()), m = req.method();
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (serveBorders(route, u.pathname)) return;
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: page(heroes, { lib, pick, scripts }) });
    if (u.pathname === C + '/characters/new/plan') return ok(plan(canMake, extra));
    if (u.pathname === C + '/system-entries/grants') return ok({ grants: [{ id: 1, fieldKey: 'ancestry', userId: 'u1', budget: 3 }] });
    if (u.pathname === C + '/system-entries' && m === 'GET') return ok({ entries: store });
    if (u.pathname === C + '/system-entries' && m === 'POST') {
      const b = JSON.parse(req.postData());
      seen.post.push(b);
      const e = { id: 41, fieldKey: b.fieldKey, slug: 'ashborn', name: b.name, summary: b.summary, description: b.description, properties: b.properties, status: b.status === 'sent' ? 'sent' : 'draft', createdBy: 'u1' };
      store.push(e);
      return ok({ entry: e }, 201);
    }
    if (u.pathname.startsWith(C + '/system-entries/') && m === 'PUT') { const b = JSON.parse(req.postData()); seen.put.push({ path: u.pathname, body: b }); const id = +u.pathname.split('/').pop(); const was = store.find((x) => x.id === id) || { id, fieldKey: 'ancestry', status: 'draft', properties: {} }; return ok({ entry: Object.assign({}, was, b) }); }
    if (u.pathname === C + '/characters/new' && m === 'POST') { seen.put.push({ path: u.pathname, body: JSON.parse(req.postData()) }); return ok({ id: 'e9', url: C + '/entities/e9' }, 201); }
    if (u.pathname.startsWith(C + '/entities/')) { seen.put.push({ path: u.pathname, body: JSON.parse(req.postData() || 'null') }); return ok({ status: 'ok' }); }
    if (u.pathname.startsWith('/static/')) {
      const f = staticFile(u.pathname);
      if (!existsSync(f)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(f) });
    }
    return route.fulfill({ status: 404, body: '' });
  });
  await p.goto('http://chronicle.test/');
  await p.waitForSelector('[data-hb-ready]', { state: 'attached' });
  return { p, seen };
}

test('the make row shows only while a permission is unspent', { skip }, async (t) => {
  const a = await open(t, { canMake: false });
  await a.p.click('[data-blank="ancestry"]');
  assert.equal(await a.p.locator('[data-own-new]').count(), 0);
  const b = await open(t);
  await b.p.click('[data-blank="ancestry"]');
  assert.match(await b.p.locator('[data-own-new]').innerText(), /Make your own ancestry[\s\S]*Your Director allows one/);
});

test('writing one in the panel needs a name, then a hand-in is picked for the hero', { skip }, async (t) => {
  const { p, seen } = await open(t);
  await p.click('[data-blank="ancestry"]');
  await p.click('[data-own-new]');
  await p.waitForSelector('.hb-panel [data-k="name"]');
  assert.equal(await p.locator('.hb-panel .oed-budget, .hb-panel .oed').count() > 0, true);
  await p.click('.hb-panel [data-oed="send"]');
  assert.equal(await p.locator('.hb-panel [data-ferr="name"]').count(), 1, 'a missing name is said inline');
  assert.equal(seen.post.length, 0);
  await p.fill('.hb-panel [data-k="name"]', 'Ashborn');
  await p.fill('.hb-panel [data-k="summary"]', 'Born of ash.');
  await p.click('.hb-panel [data-oed="send"]');
  await p.waitForSelector('.hb-toast');
  assert.equal(seen.post.length, 1);
  assert.equal(seen.post[0].status, 'sent');
  assert.equal(seen.post[0].fieldKey, 'ancestry');
  assert.equal(seen.post[0].forEntityId, 'e1');
  const field = seen.put.find((x) => x.path === C + '/entities/e1/fields');
  assert.deepEqual(field.body, { fields_patch: { ancestry: 'Ashborn' } });
  assert.match(await p.locator('.hb-toast').innerText(), /handed in/);
  assert.equal(await p.locator('[data-own-new]').count(), 0, 'the permission is spent');
});

test('a draft is marked in the list, and can be edited from its detail', { skip }, async (t) => {
  const mine = { slug: 'ashborn', name: 'Ashborn', summary: 'Born of ash.', source: 'campaign', mine: true, status: 'draft', entryId: 41, properties: {} };
  const { p } = await open(t, { canMake: false, extra: [mine], entries: [{ id: 41, fieldKey: 'ancestry', name: 'Ashborn', summary: 'Born of ash.', properties: {}, status: 'draft', reviewNote: 'Lower the speed.' }] });
  await p.click('[data-blank="ancestry"]');
  assert.match(await p.locator('.hb-opt', { hasText: 'Ashborn' }).innerText(), /changes asked/i);
  await p.locator('.hb-opt', { hasText: 'Ashborn' }).click();
  assert.match(await p.locator('[data-hb-detail]').innerText(), /Lower the speed/);
  await p.click('[data-own-edit]');
  await p.waitForSelector('.hb-panel [data-k="name"]');
  assert.equal(await p.inputValue('.hb-panel [data-k="name"]'), 'Ashborn');
});

test('editing a draft the hero has not picked never changes the hero', { skip }, async (t) => {
  const mine = { slug: 'ashborn', name: 'Ashborn', summary: 'Born of ash.', source: 'campaign', mine: true, status: 'draft', entryId: 41, properties: {} };
  const { p, seen } = await open(t, { canMake: false, extra: [mine], heroes: [{ id: 'e1', name: 'Tess', values: { ancestry: 'Human' }, hidden: [] }], entries: [{ id: 41, fieldKey: 'ancestry', name: 'Ashborn', summary: 'Born of ash.', properties: {}, status: 'draft' }] });
  await p.click('[data-blank="ancestry"]');
  await p.locator('.hb-opt', { hasText: 'Ashborn' }).click();
  await p.click('[data-own-edit]');
  await p.waitForSelector('.hb-panel [data-k="name"]');
  await p.fill('.hb-panel [data-k="summary"]', 'Born of cinders.');
  const saved = p.waitForRequest((r) => r.method() === 'PUT' && r.url().endsWith('/system-entries/41'));
  await p.click('.hb-panel [data-oed="draft"]');
  await saved;
  await p.waitForFunction(() => { const t = document.querySelector('.hb-toast'); return t && !t.hidden; });
  assert.match(await p.locator('.hb-toast').innerText(), /saved as a draft\.$/);
  assert.equal(await p.locator('.hb-panel .hb-opt', { hasText: 'Ashborn' }).count(), 1, 'the panel stays open on the list');
  assert.ok(seen.put.some((x) => x.path === C + '/system-entries/41'), 'the entry itself is saved');
  assert.equal(seen.put.filter((x) => x.path === C + '/entities/e1/fields').length, 0, 'the hero keeps Human');
});

// --- The Card border row (the builder hands the form its library on its own mount) ---

const FIELD = '[data-hcw-f]';
const form = async (t, o = {}) => {
  const r = await open(t, { lib: true, ...o });
  await r.p.click('[data-blank="ancestry"]');
  await r.p.click('[data-own-new]');
  await r.p.waitForSelector('.hb-panel [data-k="name"]');
  return r;
};
const look = (p) => p.evaluate(() => {
  const f = document.querySelector('[data-hcw-f]');
  return { name: f.querySelector('[data-fname]').textContent, col: f.querySelector('[data-fcol]').textContent, word: f.querySelector('[data-fword]').textContent,
    rowTrim: f.querySelector('[data-frow] .hc-gcard').getAttribute('data-trim'), liveTrim: f.querySelector('[data-flive]').getAttribute('data-trim'),
    liveK: f.querySelector('[data-flive]').style.getPropertyValue('--k').trim() };
});

test('Card border: the builder’s form has the closed row, opens it, and a choice repaints the row and the live card', { skip }, async (t) => {
  const { p } = await form(t);
  assert.equal(await p.evaluate(() => document.querySelector('#oed-s').closest('.oed-field').nextElementSibling.matches('[data-hcw-f]')), true);
  assert.deepEqual(await look(p), { name: 'None', col: 'A plain card', word: 'Change', rowTrim: null, liveTrim: null, liveK: '' });
  assert.equal(await p.locator(FIELD + ' .hcw-pk').isVisible(), false);
  await p.click(FIELD + ' [data-hcw="fold"]');
  assert.equal(await p.locator(FIELD + ' [data-hcw="fold"]').getAttribute('aria-expanded'), 'true');
  await p.click(FIELD + ' .hcw-t[data-b="storm"]');
  await p.click(FIELD + ' .hcw-c[data-c="#4aa3e0"]');
  assert.deepEqual(await look(p), { name: 'Storm', col: 'Sky', word: 'Done', rowTrim: 'storm', liveTrim: 'storm', liveK: '#4aa3e0' });
  // hero_creator.css is not on this page: the form's own base rules draw the live card.
  const card = await p.evaluate(() => { const c = document.querySelector('[data-flive]'); const s = getComputedStyle(c); return { w: c.getBoundingClientRect().width, h: c.getBoundingClientRect().height, display: s.display }; });
  assert.ok(card.w >= 150 && card.h >= 120 && card.display === 'flex', JSON.stringify(card));
  assert.equal(await p.locator('link[href*="hero_creator.css"]').count(), 0, 'the creator’s sheet is not pulled onto this page');
  assert.equal(await p.locator('link[href*="hero_border_picker.css"]').count(), 1);
});

test('Card border: the builder sends the border and colour with the entry, and the own colour sends none', { skip }, async (t) => {
  const { p, seen } = await form(t);
  await p.fill('.hb-panel [data-k="name"]', 'Ashborn');
  await p.click(FIELD + ' [data-hcw="fold"]');
  await p.click(FIELD + ' .hcw-t[data-b="fire"]');
  await p.click(FIELD + ' .hcw-c[data-c="#b3243a"]');
  await p.click('.hb-panel [data-oed="draft"]');
  await p.waitForSelector('.hb-toast');
  assert.equal(seen.post[0].properties.card_border, 'fire');
  assert.equal(seen.post[0].properties.card_colour, '#b3243a');
  const own = await form(t);
  await own.p.fill('.hb-panel [data-k="name"]', 'Ashborn');
  await own.p.click(FIELD + ' [data-hcw="fold"]');
  await own.p.click(FIELD + ' .hcw-t[data-b="fire"]');
  await own.p.click('.hb-panel [data-oed="draft"]');
  await own.p.waitForSelector('.hb-toast');
  assert.deepEqual([own.seen.post[0].properties.card_border, 'card_colour' in own.seen.post[0].properties], ['fire', false]);
});

test('Card border: a stored border that left the library goes back untouched; no library or no script means no field', { skip }, async (t) => {
  const mine = { slug: 'ashborn', name: 'Ashborn', summary: 'Born of ash.', source: 'campaign', mine: true, status: 'draft', entryId: 41, properties: {} };
  const stored = [{ id: 41, fieldKey: 'ancestry', name: 'Ashborn', summary: 'Born of ash.', properties: { card_border: 'gone', card_colour: '#abcdef' }, status: 'draft' }];
  for (const o of [{ lib: true }, { lib: true, pick: false }, { lib: false }, { lib: true, scripts: false }]) {
    const { p, seen } = await open(t, { canMake: false, extra: [mine], entries: stored, ...o });
    await p.click('[data-blank="ancestry"]');
    await p.locator('.hb-opt', { hasText: 'Ashborn' }).click();
    await p.click('[data-own-edit]');
    await p.waitForSelector('.hb-panel [data-k="name"]');
    const field = await p.locator(FIELD).count();
    assert.equal(field, o.lib && o.pick !== false && o.scripts !== false ? 1 : 0, JSON.stringify(o));
    if (field) assert.equal((await look(p)).name, 'None');
    assert.ok(!(await p.locator('.oed-fact input[data-part="label"]').evaluateAll((els) => els.some((e) => /card/i.test(e.value)))));
    await p.click('.hb-panel [data-oed="draft"]');
    await p.waitForSelector('.hb-toast:not([hidden])');
    const put = seen.put.find((x) => x.path === C + '/system-entries/41').body.properties;
    assert.deepEqual([put.card_border, put.card_colour], ['gone', '#abcdef'], JSON.stringify(o));
  }
});

test('Card border: at 360 wide the builder’s row, samples and swatches are 44 high with no sideways scroll; light and dark words reach 4.5:1', { skip }, async (t) => {
  for (const dark of [false, true]) {
    const { p } = await form(t, { width: 360, dark });
    await p.click(FIELD + ' [data-hcw="fold"]');
    await p.click(FIELD + ' .hcw-t[data-b="fire"]');
    const m = await p.evaluate(() => {
      const hs = (sel) => Array.from(document.querySelectorAll(sel)).map((e) => Math.round(e.getBoundingClientRect().height));
      const rgb = (c) => (c.match(/[\d.]+/g) || []).slice(0, 4).map(Number);
      const bg = (el) => { for (let e = el; e; e = e.parentElement) { const c = rgb(getComputedStyle(e).backgroundColor); if (c.length === 3 || (c.length === 4 && c[3] > 0.95)) return c.slice(0, 3); } return [255, 255, 255]; };
      const words = {};
      ['[data-fname]', '[data-fcol]', '[data-fword]', '[data-hcw-f] .oed-hint'].forEach((s) => { const el = document.querySelector(s); words[s] = [rgb(getComputedStyle(el).color).slice(0, 3), bg(el)]; });
      return { row: hs('[data-hcw="fold"]'), tiles: hs('[data-hcw-f] .hcw-t'), sw: hs('[data-hcw-f] .hcw-c'), x: document.documentElement.scrollWidth - document.documentElement.clientWidth, words };
    });
    assert.ok(m.row[0] >= 44 && m.tiles.every((h) => h >= 44) && m.sw.every((h) => h >= 44) && m.sw.length >= 12, JSON.stringify(m));
    assert.ok(m.x <= 0, 'no sideways scroll');
    // Colour pairs are checked where the page has the site's tokens; here the page has only its own light ones.
    if (!dark) for (const [k, [fg, bgc]] of Object.entries(m.words)) assert.ok(contrast(fg, bgc) >= 4.5, k + ' ' + contrast(fg, bgc).toFixed(2));
  }
});
