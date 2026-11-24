// hero_creator_multi_browser.test.mjs — a step that takes several picks
// (multi): a class that picks two subclasses keeps the step open until both
// are chosen, a pick toggles, and the hero is sent with both joined.
//
// The plan is a synthetic one, not any real package's. Runs hero_creator.js in
// a real Chromium. Needs playwright-core and a Chromium (see _browser_env.mjs);
// skips locally, and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const C = '/campaigns/c1';
const js = (n) => readFileSync(join(root, 'internal/plugins/entities/static/js', n));
const css = () => readFileSync(join(root, 'internal/plugins/entities/static/css/hero_creator.css'));

const ch = (key, name, extra = {}) => ({ key, slug: key, name, summary: name + ' summary.', source: 'package', properties: {}, ...extra });
const sub = (name, parent, props = {}) => ch(parent.toLowerCase() + '--' + name.toLowerCase(), name, { parent, properties: props });
const PLAN = () => ({ systemName: 'Test System', steps: [
  { fieldKey: 'ancestry', label: 'Ancestry', needed: true, choices: [ch('orc', 'Orc'), ch('elf', 'Elf')] },
  { fieldKey: 'class', label: 'Class', needed: true, choices: [
    ch('fury', 'Fury', { properties: { kits: 1 } }),
    ch('conduit', 'Conduit', { properties: { kits: 0, subclass_count: 2, kit_note: 'Conduits choose a prayer and a ward instead.' } }),
    ch('beastheart', 'Beastheart', { properties: { kits: 1 } }),
  ] },
  { fieldKey: 'subclass', label: 'Subclass', needed: true, parent: 'class', multi: true, countFrom: [{ step: 'class', property: 'subclass_count' }], choices: [
    sub('Berserker', 'Fury', { kits: 0, kit_note: 'Special kits.' }), sub('Reaver', 'Fury'), sub('Light', 'Conduit'), sub('Shade', 'Conduit'), sub('Storm', 'Conduit'),
  ] },
  { fieldKey: 'kit', label: 'Kit', countFrom: [{ step: 'class', property: 'kits', note: 'kit_note' }, { step: 'subclass', property: 'kits', note: 'kit_note' }],
    choices: [ch('panther', 'Panther'), ch('raider', 'Raider')] },
  { fieldKey: 'complication', label: 'Complication', choices: [ch('lucky', 'Lucky')] },
] });

const html = `<!doctype html><html data-gm-term="Director"><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style><link rel="stylesheet" href="/hero_creator.css"></head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true" data-practice="PRACTICE"><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { register: (n, d) => { window.hcImpl = d; document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)); }, escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director' };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_creator.js"></script></body></html>`;

async function open(t, { width = 1100, practice = false, draft = null } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: 'reduce' });
  const seen = { post: [] };
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname;
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_creator.js') });
    if (p === '/hero_creator.css') return route.fulfill({ contentType: 'text/css', body: css() });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_rules_lines.js') });
    if (p === C + '/characters/new/plan') return ok(PLAN());
    if (p === C + '/characters/new' && req.method() === 'POST') { seen.post.push(JSON.parse(req.postData())); return ok({ id: 'e9', url: C + '/entities/e9' }, 201); }
    return route.fulfill({ contentType: 'text/html', body: html.replace('PRACTICE', String(practice)) });
  });
  if (draft) await pg.addInitScript((d) => { try { if (!localStorage.getItem('seeded')) { localStorage.setItem('seeded', '1'); localStorage.setItem(d.key, JSON.stringify(d.value)); } } catch (e) { /* none */ } },
    { key: (practice ? 'chronicle:hero-practice:c1' : 'chronicle:hero-draft:c1'), value: draft });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  return { pg, seen };
}
const text = (pg, sel) => pg.locator(sel).first().innerText();
const gcard = (pg, name) => pg.locator('.hc-gcard', { hasText: name });
const rail = (pg, key) => pg.locator('.hc-rail .hc-row[data-k="' + key + '"]');
const pick = async (pg, name) => { await gcard(pg, name).click(); await pg.click('.hc-right [data-act="choose"]'); };
const live = (pg) => pg.waitForFunction(() => document.querySelector('.hc-live') && document.querySelector('.hc-live').textContent !== '').then(() => text(pg, '.hc-live'));

test('a class that takes two subclasses keeps the step open until both are picked, and sends both', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await rail(pg, 'class').click();
  await pick(pg, 'Conduit');
  await rail(pg, 'subclass').click();
  assert.match(await text(pg, '.hc-intro'), /Choose 2\./);
  await pick(pg, 'Light');
  assert.match(await live(pg), /Light chosen\. Choose one more\./);
  assert.equal(await pg.locator('.hc-app').getAttribute('data-step'), '2', 'still on the subclass step');
  // Picking it again takes it off; a third pick makes room by dropping the oldest.
  await pick(pg, 'Light');
  await pg.waitForFunction(() => /Light taken off/.test(document.querySelector('.hc-live').textContent));
  await pick(pg, 'Storm');
  await pick(pg, 'Shade');
  await pg.waitForFunction(() => document.querySelector('.hc-app').getAttribute('data-step') !== '2');
  assert.match(await rail(pg, 'subclass').innerText(), /Storm, Shade/);
  await rail(pg, 'ancestry').click();
  await pick(pg, 'Orc');
  await pg.locator('.hc-rail .hc-row').last().click();
  await pg.fill('[data-name]', 'Tess');
  await pg.click('[data-act="create"]');
  await pg.waitForTimeout(100);
  assert.equal(seen.post[0].picks.subclass.name, 'Storm, Shade');
});

test('a class that takes one subclass still moves on after one pick', { skip }, async (t) => {
  const { pg } = await open(t);
  await rail(pg, 'class').click();
  await pick(pg, 'Fury');
  await rail(pg, 'subclass').click();
  assert.doesNotMatch(await text(pg, '.hc-intro'), /Choose 2/);
  await pick(pg, 'Reaver');
  await pg.waitForFunction(() => document.querySelector('.hc-app').getAttribute('data-step') !== '2');
  assert.match(await rail(pg, 'subclass').innerText(), /Reaver/);
});

test('changing the class drops the subclass picks that no longer fit', { skip }, async (t) => {
  const { pg } = await open(t);
  await rail(pg, 'class').click();
  await pick(pg, 'Conduit');
  await rail(pg, 'subclass').click();
  await pick(pg, 'Light');
  await pick(pg, 'Shade');
  await rail(pg, 'class').click();
  await pick(pg, 'Fury');
  assert.match(await rail(pg, 'subclass').innerText(), /Not chosen yet/);
});
