// own_entries_browser.test.mjs — the Director's marks on the own-entries page.
//
// Runs own_entries.js in a real Chromium against a stubbed server. It pins
// what a Director sees and what reaches the server: every entry of a list
// with its mark, a quiet mark saved at once, a limiting mark checked first
// and saved with its reason, waiting Ask-first requests allowed or answered
// with a note, and a player's page with none of it.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';
import { bordersAttrs, pickerScripts, serveBorders, contrast, INFO } from './_card_border_fixture.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const C = '/campaigns/c1';

const PLAN = { systemName: 'Draw Steel', steps: [
  { fieldKey: 'class', label: 'Class', choices: [
    { slug: 'fury', name: 'Fury', summary: 'Rage.', source: 'Draw Steel', properties: {} },
    { slug: 'tactician', name: 'Tactician', summary: 'Orders.', source: 'Draw Steel', properties: {} },
  ] },
] };
const entry = (key, name, extra = {}) => ({ key, source: 'package', slug: key, name, summary: name + ' summary.', everyone: null, players: [], state: 'open', ruleCount: 0, ...extra });
const RULES = () => ({
  fieldKey: 'class', systemName: 'Draw Steel',
  groups: [
    { source: 'campaign', key: '*', label: 'This campaign’s own', everyone: null, players: [] },
  ],
  entries: [
    entry('fury', 'Fury'),
    entry('tactician', 'Tactician', { everyone: { state: 'ask_first', reason: '' }, state: 'ask_first' }),
  ],
  members: [{ userId: 'u2', name: 'Pat' }], orphans: [], warnings: [],
});
const REQUEST = { id: 7, fieldKey: 'class', fieldLabel: 'Class', source: 'package', key: 'tactician', entryName: 'Tactician', userId: 'u2', playerName: 'Pat Lee', entityId: 'e1', entityName: 'Tess', status: 'waiting' };

const html = (director, o = {}) => `<!doctype html><html${o.dark ? ' class="dark"' : ''}><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}${o.css ? '*,::before,::after{box-sizing:border-box}:root{--color-accent:#4f46e5;--color-accent-link:#4338ca;--color-bg-primary:#f9fafb;--color-bg-secondary:#f3f4f6;--color-card-bg:#fff;--color-bg-tertiary:#f3f4f6;--color-border:#e5e7eb;--color-text-primary:#111827;--color-text-secondary:#4b5563;--color-text-body:#374151;--color-accent-rgb:79 70 229;--dur-micro:120ms;--dur-standard:200ms;--ease-out:cubic-bezier(.16,1,.3,1);--ease-in:cubic-bezier(.4,0,1,1)}html.dark{--color-accent-link:#a5b4fc;--color-bg-primary:#111827;--color-bg-secondary:#1f2937;--color-card-bg:#1f2937;--color-bg-tertiary:#374151;--color-border:#374151;--color-text-primary:#f9fafb;--color-text-secondary:#c4c9d2;--color-text-body:#e5e7eb}' : ''}</style>${o.css ? '<link rel="stylesheet" href="/paper.css"><link rel="stylesheet" href="/own_entries.css">' : ''}</head><body>
<div class="hc hc--tool" data-widget="own-entries" data-own-entries="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-entries-url="${C}/system-entries"
  data-members-url="${C}/members" data-director="${director}" data-user="u1" data-field="class" data-csrf="tok"${o.lib ? bordersAttrs({ pick: o.pick !== false }) : ''}><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { gmTerm: () => 'Director', register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)), escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])) };</script>
${o.lib && o.scripts !== false ? pickerScripts : ''}<script src="/own_entry_editor.js"></script><script src="/own_entries.js"></script></body></html>`;

async function open(t, { director = true, checkCount = 0, requests = [REQUEST], putStatus = 200, lib = false, pick = true, scripts = true, css = false, dark = false, width = 1100, entries = [], grants = [] } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width, height: 900 }, colorScheme: dark ? 'dark' : 'light' });
  const errors = [];
  pg.on('pageerror', (e) => errors.push(e.message));
  const seen = { put: [], check: [], post: [], entry: [], replace: [], errors };
  const store = entries.slice();
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname, m = req.method();
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (p === '/own_entry_editor.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'internal/plugins/entities/static/js/own_entry_editor.js')) });
    if (serveBorders(route, p)) return;
    if (p === '/own_entry_editor.css') return route.fulfill({ contentType: 'text/css', body: css ? readFileSync(join(root, 'internal/plugins/entities/static/css/own_entry_editor.css')) : '' });
    if (p === '/own_entries.css') return route.fulfill({ contentType: 'text/css', body: readFileSync(join(root, 'internal/plugins/entities/static/css/own_entries.css')) });
    if (p === '/paper.css') return route.fulfill({ contentType: 'text/css', body: readFileSync(join(root, 'static/css/paper.css')) });
    if (p === '/own_entries.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'internal/plugins/entities/static/js/own_entries.js')) });
    if (p === C + '/characters/new/plan') return ok(PLAN);
    if (p === C + '/system-entries' && m === 'GET') return ok({ entries: store });
    if (p === C + '/system-entries' && m === 'POST') {
      const b = JSON.parse(req.postData());
      seen.entry.push({ method: m, body: b });
      const e = { id: 41, slug: 'ashborn', status: b.status === 'sent' ? 'sent' : 'draft', visibility: 'everyone', createdBy: 'u1', ...b };
      store.push(e);
      return ok({ entry: e }, 201);
    }
    if (p.startsWith(C + '/system-entries/') && !p.endsWith('/grants') && m === 'PUT') {
      const b = JSON.parse(req.postData());
      seen.entry.push({ method: m, path: p, body: b });
      const e = Object.assign(store.find((x) => '/system-entries/' + x.id === p.slice(C.length)) || {}, b);
      return ok({ entry: e });
    }
    if (p === C + '/system-entries/rewrite' && m === 'POST') {
      seen.replace.push({ path: p, body: JSON.parse(req.postData()) });
      return ok({ entry: { id: 52, slug: 'fury-2', name: 'Fury', fieldKey: 'class', visibility: 'directors', standsFor: 'fury' } }, 201);
    }
    if (/\/system-entries\/\d+\/tie$/.test(p) && m === 'POST') {
      seen.replace.push({ path: p, body: JSON.parse(req.postData()) });
      return ok({ entry: {} });
    }
    if (p === C + '/system-entries/grants') return ok({ grants });
    if (p === C + '/members') return ok([{ user_id: 'u2', display_name: 'Pat Lee' }]);
    if (p === C + '/choice-requests') return ok({ requests, count: requests.length });
    if (p === C + '/choice-rules/entries') return ok(RULES());
    if (p === C + '/choice-rules/check') {
      seen.check.push(JSON.parse(req.postData()));
      return ok({ count: checkCount, heroes: checkCount ? [{ heroId: 'e1', heroName: 'Tess', playerId: 'u2', playerName: 'Pat', fieldKey: 'class', value: 'Fury', kind: 'not_offered' }] : [] });
    }
    if (p === C + '/choice-rules' && m === 'PUT') {
      seen.put.push(JSON.parse(req.postData()));
      return ok(putStatus === 200 ? { rules: [] } : { message: 'Players can’t see Fury yet.' }, putStatus);
    }
    if (p.startsWith(C + '/choice-requests/') && m === 'POST') {
      seen.post.push({ p, body: JSON.parse(req.postData() || '{}') });
      return p.endsWith('/allow') ? ok({ heroUpdated: true }) : route.fulfill({ status: 204 });
    }
    return route.fulfill({ contentType: 'text/html', body: html(director, { lib, pick, scripts, css, dark }) });
  });
  await pg.goto('http://t.test/');
  return { pg, seen };
}

const row = (pg, name) => pg.locator('.oe-ent', { has: pg.locator('b', { hasText: new RegExp('^' + name) }) });

test('a Director sees every entry with its mark', { skip }, async (t) => {
  const { pg } = await open(t);
  await pg.waitForSelector('.oe-ent');
  assert.match(await pg.textContent('.oe-ent--group'), /This campaign/);
  assert.equal(await row(pg, 'Fury').locator('.oe-mark').count(), 0, 'open has no mark');
  assert.match(await row(pg, 'Tactician').locator('.oe-mark').textContent(), /Ask first/);
  assert.ok(await pg.locator('select[data-show]').count(), 'the Showing switch is there');
});

test('a quiet mark saves at once, without a check', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pg.waitForSelector('.oe-ent');
  await row(pg, 'Fury').locator('.oe-dots').click();
  await pg.getByRole('menuitemradio', { name: 'Ask first' }).click();
  await pg.waitForFunction(() => document.querySelector('.oe-ent .oe-mark'));
  assert.equal(seen.check.length, 0);
  assert.deepEqual(seen.put[0].marks, [{ source: 'package', key: 'fury', target: '', state: 'ask_first' }]);
});

test('Not here is checked first, names the heroes, and saves with its reason', { skip }, async (t) => {
  const { pg, seen } = await open(t, { checkCount: 1 });
  await pg.waitForSelector('.oe-ent');
  await row(pg, 'Fury').locator('.oe-dots').click();
  await pg.getByRole('menuitemradio', { name: 'Not here' }).click();
  await pg.waitForSelector('.oe-strip');
  assert.equal(seen.put.length, 0, 'nothing saved before the Director agrees');
  assert.match(await pg.textContent('.oe-asked'), /1 hero already made breaks this: Tess \(Pat\)\. They keep what they have\./);
  await pg.fill('[data-strip-reason]', 'No Furies this time');
  await pg.getByRole('button', { name: 'Save' }).click();
  await pg.waitForFunction(() => !document.querySelector('.oe-strip'));
  assert.deepEqual(seen.put[0].marks, [{ source: 'package', key: 'fury', target: '', state: 'not_offered', reason: 'No Furies this time' }]);
});

test('a refused mark shows the server sentence and keeps the strip', { skip }, async (t) => {
  const { pg } = await open(t, { putStatus: 422 });
  await pg.waitForSelector('.oe-ent');
  await row(pg, 'Fury').locator('.oe-dots').click();
  await pg.getByRole('menuitemradio', { name: 'Ask first' }).click();
  await pg.waitForSelector('.oe-error');
  assert.match(await pg.textContent('.oe-error'), /Players can.t see Fury yet\./);
});

test('a waiting request is allowed for the player', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pg.waitForSelector('.oe-wait');
  assert.match(await pg.textContent('.oe-wait'), /Pat Lee.*asks for.*Tactician.*Tess/s);
  await pg.getByRole('button', { name: 'Allow for Pat' }).click();
  await pg.waitForTimeout(200);
  assert.equal(seen.post[0].p, C + '/choice-requests/7/allow');
});

test('Suggest another sends the short note', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pg.waitForSelector('.oe-wait');
  await pg.getByRole('button', { name: 'Suggest another' }).click();
  await pg.fill('[data-suggest-msg]', 'Try Fury, it suits the table');
  await pg.getByRole('button', { name: 'Send' }).click();
  await pg.waitForTimeout(200);
  assert.equal(seen.post[0].p, C + '/choice-requests/7/suggest');
  assert.deepEqual(seen.post[0].body, { message: 'Try Fury, it suits the table' });
});

const own = (id, name, extra = {}) => ({ id, slug: name.toLowerCase(), name, fieldKey: 'class', status: 'accepted', visibility: 'everyone', createdBy: 'u1', ...extra });

test('Replace with your own: a copy of the printed entry, opened in the editor', { skip }, async (t) => {
  const { pg, seen } = await open(t, { entries: [own(41, 'Ashborn'), own(43, 'Stormcaller', { standsFor: 'tactician', standsForName: 'Tactician' })] });
  await pg.waitForSelector('.oe-ent');
  await row(pg, 'Fury').locator('.oe-dots').click();
  await pg.getByRole('menuitem', { name: 'Replace with your own…' }).click();
  assert.match(await pg.textContent('.oe-menu'), /Players get yours in place of Draw Steel’s Fury/);
  assert.equal(await pg.getByRole('menuitem', { name: 'Use Ashborn' }).count(), 1, 'an entry of the campaign’s own can take the place');
  assert.equal(await pg.getByRole('menuitem', { name: 'Use Stormcaller' }).count(), 0, 'one that already stands in for another is not offered');
  await pg.getByRole('menuitem', { name: 'Start from a copy of Fury' }).click();
  await pg.waitForURL(/entry=52/);
  assert.deepEqual(seen.replace, [{ path: C + '/system-entries/rewrite', body: { fieldKey: 'class', slug: 'fury' } }]);
  assert.match(pg.url(), /\?field=class&entry=52&made=1$/);
});

test('Replace with your own: the campaign’s entry of the same name takes the place', { skip }, async (t) => {
  const { pg, seen } = await open(t, { entries: [own(41, 'Fury')] });
  await pg.waitForSelector('.oe-ent');
  await row(pg, 'Tactician').locator('.oe-dots').click();
  await pg.getByRole('menuitem', { name: 'Replace with your own…' }).click();
  assert.equal(await pg.getByRole('menuitem', { name: 'Start from a copy of Tactician' }).count(), 1);
  await pg.getByRole('menuitem', { name: 'Back' }).click();
  await pg.getByRole('menuitem', { name: 'Replace with your own…' }).waitFor();
  await pg.keyboard.press('Escape');
  await row(pg, 'Fury').first().locator('.oe-dots').click();
  await pg.getByRole('menuitem', { name: 'Replace with your own…' }).click();
  assert.equal(await pg.getByRole('menuitem', { name: /Start from a copy/ }).count(), 0, 'no copy while a Fury of its own exists');
  await pg.getByRole('menuitem', { name: 'Use the Fury you wrote' }).click();
  await pg.waitForURL(/entry=41/);
  assert.deepEqual(seen.replace, [{ path: C + '/system-entries/41/tie', body: { slug: 'fury' } }]);
});

test('a player’s page has no marks', { skip }, async (t) => {
  const { pg } = await open(t, { director: false });
  await pg.waitForSelector('.hc-page, .hc-loading__text');
  await pg.waitForTimeout(300);
  assert.equal(await pg.locator('.oe-dots, select[data-show], .oe-wait').count(), 0);
});

// --- The Card border row, where an entry is written on this page ---

const FIELD = '[data-hcw-f]';
const POP = '.oe-pop';
const PLAYER = { grants: [{ id: 1, fieldKey: 'class', userId: 'u1', budget: 0 }] };
const sheets = (pg) => pg.waitForFunction(() => { const l = document.querySelector('link[data-oed-css]'); return !!(l && l.sheet) && Array.from(document.querySelectorAll('link[href*="hero_border"]')).every((x) => x.sheet); });
const addNew = async (pg) => { await pg.waitForSelector('[data-act="add"]'); await sheets(pg); await pg.click('[data-act="add"]'); await pg.waitForSelector(POP + ' ' + FIELD); };
const stored = (props) => ({ entries: [{ id: 41, fieldKey: 'class', slug: 'ashborn', name: 'Ashborn', summary: 'Born of ash.', description: '', status: 'draft', createdBy: 'u1', properties: props }],
  grants: [{ id: 1, fieldKey: 'class', userId: 'u1', entryId: 41, budget: 0 }] });
const openStored = async (pg) => { await pg.waitForSelector('.hc-row[data-open="41"]'); await pg.evaluate(() => 0); await pg.click('.hc-row[data-open="41"]'); await pg.waitForSelector(POP + ' [data-k="name"]'); };
const read = (pg) => pg.evaluate(() => {
  const f = document.querySelector('[data-hcw-f]');
  return { name: f.querySelector('[data-fname]').textContent, col: f.querySelector('[data-fcol]').textContent, word: f.querySelector('[data-fword]').textContent,
    rowTrim: f.querySelector('[data-frow] .hc-gcard').getAttribute('data-trim'), liveTrim: f.querySelector('[data-flive]').getAttribute('data-trim'), liveK: f.querySelector('[data-flive]').style.getPropertyValue('--k').trim(),
    liveName: f.querySelector('[data-flname]').textContent, liveSum: f.querySelector('[data-flsum]').textContent };
});

test('Card border: a Director’s new entry has the closed row after the one-line field, the short hint, and saves the choice', { skip }, async (t) => {
  const { pg, seen } = await open(t, { director: true, lib: true, css: true });
  await addNew(pg);
  assert.equal(await pg.evaluate(() => document.querySelector('[data-k="summary"]').closest('label').nextElementSibling.matches('[data-hcw-f]')), true);
  assert.deepEqual(await read(pg), { name: 'None', col: 'A plain card', word: 'Change', rowTrim: null, liveTrim: null, liveK: '', liveName: 'Your class', liveSum: 'What a player reads before opening it.' });
  assert.equal((await pg.textContent(FIELD + ' .oe-hint')).trim(), 'How this card looks in the creator.');
  assert.equal(await pg.locator(FIELD + ' .hcw-pk').isVisible(), false);
  await pg.click(FIELD + ' [data-hcw="fold"]');
  assert.equal(await pg.getAttribute(FIELD + ' [data-hcw="fold"]', 'aria-expanded'), 'true');
  assert.equal(await pg.textContent(FIELD + ' [data-fword]'), 'Done');
  await pg.fill(POP + ' [data-k="name"]', 'Ashborn');
  await pg.fill(POP + ' [data-k="summary"]', 'Born of ash.');
  await pg.click(FIELD + ' .hcw-t[data-b="fire"]');
  await pg.click(FIELD + ' .hcw-c[data-c="#d65b9a"]');
  const r = await read(pg);
  assert.deepEqual([r.name, r.col, r.rowTrim, r.liveTrim, r.liveK, r.liveName, r.liveSum], ['Fire', 'Rose', 'fire', 'fire', '#d65b9a', 'Ashborn', 'Born of ash.']);
  await pg.click(POP + ' [data-act="save"]');
  await pg.waitForFunction(() => document.querySelector('.oe-pop').hidden);
  assert.equal(seen.entry.length, 1);
  assert.equal(seen.entry[0].method, 'POST');
  assert.equal(seen.entry[0].body.properties.card_border, 'fire');
  assert.equal(seen.entry[0].body.properties.card_colour, '#d65b9a');
  assert.deepEqual(seen.errors, []);
});

test('Card border: the border’s own colour and None send no colour; None sends no border', { skip }, async (t) => {
  for (const border of ['ice', '']) {
    const { pg, seen } = await open(t, { director: true, lib: true, css: true });
    await addNew(pg);
    await pg.fill(POP + ' [data-k="name"]', 'Ashborn');
    await pg.click(FIELD + ' [data-hcw="fold"]');
    if (border) await pg.click(FIELD + ' .hcw-t[data-b="' + border + '"]');
    await pg.click(POP + ' [data-act="save"]');
    await pg.waitForFunction(() => document.querySelector('.oe-pop').hidden);
    const p = seen.entry[0].body.properties;
    assert.equal('card_colour' in p, false);
    assert.equal(p.card_border, border || undefined);
  }
});

test('Card border: a player sees that the Director can change it; an edit shows the saved choice and no fact for it, and PUTs', { skip }, async (t) => {
  const { pg, seen } = await open(t, { director: false, lib: true, css: true, ...stored({ card_border: 'storm', card_colour: '#2FB5A4', size: '1M' }) });
  await openStored(pg);
  assert.equal((await pg.textContent(FIELD + ' .oe-hint')).trim(), 'How your card looks in the creator. Your Director can change it.');
  const r = await read(pg);
  assert.deepEqual([r.name, r.col, r.rowTrim], ['Storm', 'Teal', 'storm']);
  const labels = await pg.locator(POP + ' [data-fact][data-part="label"]').evaluateAll((els) => els.map((e) => e.value));
  assert.deepEqual(labels, ['Size'], 'no Card border or Card colour fact');
  await pg.click(FIELD + ' [data-hcw="fold"]');
  await pg.click(FIELD + ' .hcw-t[data-b="vine"]');
  await pg.click(POP + ' [data-act="save"]');
  await pg.waitForFunction(() => document.querySelector('.oe-pop').hidden);
  assert.equal(seen.entry[0].method, 'PUT');
  assert.deepEqual([seen.entry[0].body.properties.card_border, seen.entry[0].body.properties.card_colour], ['vine', '#2fb5a4']);
});

test('Card border: a border that left the library reads None and goes back untouched; a hostile colour reaches no markup', { skip }, async (t) => {
  const a = await open(t, { director: false, lib: true, css: true, ...stored({ card_border: 'gone', card_colour: '#abcdef' }) });
  await openStored(a.pg);
  assert.equal((await read(a.pg)).name, 'None');
  await a.pg.fill(POP + ' [data-k="summary"]', 'Born of cinders.');
  await a.pg.click(POP + ' [data-act="save"]');
  await a.pg.waitForFunction(() => document.querySelector('.oe-pop').hidden);
  assert.deepEqual([a.seen.entry[0].body.properties.card_border, a.seen.entry[0].body.properties.card_colour], ['gone', '#abcdef']);
  const b = await open(t, { director: false, lib: true, css: true, ...stored({ card_border: 'fire', card_colour: 'red;background:url(x)' }) });
  await openStored(b.pg);
  assert.ok(!/url\(x\)|red;background/.test(await b.pg.evaluate(() => document.querySelector('.oe-pop').outerHTML)));
  assert.equal((await read(b.pg)).liveK, INFO.fire[1]);
});

test('Card border: a fact named Card border is refused; with no library or no script there is no field and the stored values stay', { skip }, async (t) => {
  const { pg, seen } = await open(t, { director: true, lib: true, css: true });
  await addNew(pg);
  await pg.fill(POP + ' [data-k="name"]', 'Ashborn');
  await pg.click(POP + ' [data-act="addfact"]');
  await pg.locator(POP + ' [data-fact][data-part="label"]').last().fill('Card border');
  await pg.locator(POP + ' [data-fact][data-part="value"]').last().fill('fire');
  await pg.click(POP + ' [data-act="save"]');
  await pg.waitForSelector(POP + ' [data-error]:not([hidden])');
  assert.match(await pg.textContent(POP + ' [data-error]'), /is set in the Card border field, not as a fact/);
  assert.equal(seen.entry.length, 0);
  for (const o of [{ lib: true, pick: false }, { lib: false }, { lib: true, scripts: false }]) {
    const r = await open(t, { director: false, css: true, ...o, ...stored({ card_border: 'fire', card_colour: '#abcdef' }) });
    await openStored(r.pg);
    assert.equal(await r.pg.locator(FIELD).count(), 0, JSON.stringify(o));
    assert.deepEqual(await r.pg.locator(POP + ' [data-fact][data-part="label"]').evaluateAll((els) => els.map((e) => e.value)), [], 'not facts');
    await r.pg.click(POP + ' [data-act="save"]');
    await r.pg.waitForFunction(() => document.querySelector('.oe-pop').hidden);
    assert.deepEqual([r.seen.entry[0].body.properties.card_border, r.seen.entry[0].body.properties.card_colour], ['fire', '#abcdef'], JSON.stringify(o));
    assert.deepEqual(r.seen.errors, [], JSON.stringify(o));
  }
});

test('Card border: at 360 wide on this page every sample, swatch and the row are 44 high with no sideways scroll; words reach 4.5:1 in light and dark', { skip }, async (t) => {
  for (const dark of [false, true]) {
    const { pg } = await open(t, { director: true, lib: true, css: true, width: 360, dark });
    await addNew(pg);
    await pg.click(FIELD + ' [data-hcw="fold"]');
    await pg.click(FIELD + ' .hcw-t[data-b="fire"]');
    const m = await pg.evaluate(() => {
      const hs = (sel) => Array.from(document.querySelectorAll(sel)).map((e) => Math.round(e.getBoundingClientRect().height));
      const rgb = (c) => (c.match(/[\d.]+/g) || []).slice(0, 4).map(Number);
      const bg = (el) => { for (let e = el; e; e = e.parentElement) { const c = rgb(getComputedStyle(e).backgroundColor); if (c.length === 3 || (c.length === 4 && c[3] > 0.95)) return c.slice(0, 3); } return [255, 255, 255]; };
      const words = {};
      ['[data-fname]', '[data-fcol]', '[data-fword]', '[data-hcw-f] .oe-hint'].forEach((s) => { const el = document.querySelector(s); words[s] = [rgb(getComputedStyle(el).color).slice(0, 3), bg(el)]; });
      const wide = Array.from(document.querySelectorAll('.oe-pop [data-hcw-f] *')).filter((e) => e.getBoundingClientRect().right > innerWidth + 1 && !e.closest('.hcw-cs, .hcw-sm')).map((e) => e.className).slice(0, 4);
      return { row: hs('[data-hcw="fold"]'), tiles: hs('[data-hcw-f] .hcw-t'), sw: hs('[data-hcw-f] .hcw-c'), x: document.documentElement.scrollWidth - document.documentElement.clientWidth, words, wide };
    });
    assert.ok(m.row[0] >= 44 && m.tiles.every((h) => h >= 44) && m.sw.every((h) => h >= 44) && m.sw.length >= 12, JSON.stringify(m));
    assert.ok(m.x <= 0 && m.wide.length === 0, 'no sideways scroll ' + JSON.stringify(m.wide) + m.x);
    for (const [k, [fg, bgc]] of Object.entries(m.words)) assert.ok(contrast(fg, bgc) >= 4.5, `${dark ? 'dark' : 'light'} ${k} ${contrast(fg, bgc).toFixed(2)} ${fg} on ${bgc}`);
  }
});

test('Card border: keyboard only on this page — Enter on the row unfolds it, Tab walks colours then samples, Enter picks', { skip }, async (t) => {
  const { pg } = await open(t, { director: true, lib: true, css: true, width: 360 });
  await addNew(pg);
  await pg.focus(POP + ' [data-k="summary"]');
  await pg.keyboard.press('Tab');
  assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('data-hcw')), 'fold', 'Tab reaches the row');
  const ring = await pg.evaluate(() => { const s = getComputedStyle(document.activeElement); return [s.outlineStyle, parseFloat(s.outlineWidth)]; });
  await pg.keyboard.press('Enter');
  assert.equal(await pg.getAttribute(FIELD + ' [data-hcw="fold"]', 'aria-expanded'), 'true');
  await pg.focus(FIELD + ' .hcw-t[data-b="earth"]');
  await pg.keyboard.press('Enter');
  assert.equal((await read(pg)).name, 'Earth');
  await pg.focus(FIELD + ' [data-hcw="fold"]');
  await pg.keyboard.press('Tab');
  assert.equal(await pg.evaluate(() => document.activeElement.className), 'hcw-c hcw-c--own', 'colours come first');
  assert.ok(ring[0] !== 'none' && ring[1] >= 2, 'a visible focus ring ' + ring);
});

test('Card border: html[data-motion="off"] opens the fold at once on this page', { skip }, async (t) => {
  const { pg } = await open(t, { director: true, lib: true, css: true });
  await pg.evaluate(() => document.documentElement.setAttribute('data-motion', 'off'));
  await addNew(pg);
  await pg.click(FIELD + ' [data-hcw="fold"]');
  const d = await pg.evaluate(() => parseFloat(getComputedStyle(document.querySelector('[data-ffold]')).transitionDuration));
  assert.ok(d <= 0.001);
  assert.equal(await pg.locator(FIELD + ' .hcw-t[data-b="fire"]').isVisible(), true);
});

test('Card border: review picture of the Own entries page', { skip: skip || !process.env.ROW_SHOTS }, async (t) => {
  const { pg } = await open(t, { director: true, lib: true, css: true, width: 360 });
  await addNew(pg);
  await pg.fill(POP + ' [data-k="name"]', 'Ashborn');
  await pg.fill(POP + ' [data-k="summary"]', 'Born of ash and ember.');
  await pg.click(FIELD + ' [data-hcw="fold"]');
  await pg.click(FIELD + ' .hcw-t[data-b="fire"]');
  await pg.click(FIELD + ' .hcw-c[data-c="#2fb5a4"]');
  await pg.waitForTimeout(600);
  await pg.locator(FIELD + ' .oed-k, ' + FIELD + ' .oe-k').first().evaluate((e) => e.scrollIntoView({ block: 'start' }));
    await pg.waitForTimeout(150);
  await pg.screenshot({ path: join(process.env.ROW_SHOTS, 'row-ownentries-360.png') });
});
