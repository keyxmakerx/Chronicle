// hero_creator_steps_browser.test.mjs — the creator when a game system shapes
// its steps: the order it lists, which are needed, a step whose list lives
// inside another step's entries (subclass under class), and a step whose count
// the picked entries set (kit: one, or none).
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
    ch('conduit', 'Conduit', { properties: { kits: 0, kit_note: 'Conduits choose a prayer and a ward instead.' } }),
    ch('beastheart', 'Beastheart', { properties: { kits: 1 } }),
  ] },
  { fieldKey: 'subclass', label: 'Subclass', needed: true, parent: 'class', choices: [
    sub('Berserker', 'Fury', { kits: 0, kit_note: 'Special kits.' }), sub('Reaver', 'Fury'), sub('Light', 'Conduit'), sub('Shade', 'Conduit'),
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

test('steps come in the order the system names, and only the optional one reads Optional', { skip }, async (t) => {
  const { pg } = await open(t);
  const labels = await pg.locator('.hc-rail .hc-row .hc-lb b').allInnerTexts();
  assert.deepEqual(labels.slice(0, 5), ['Ancestry', 'Class', 'Subclass', 'Kit', 'Complication']);
  assert.match(await rail(pg, 'ancestry').innerText(), /Not chosen yet/);
  assert.match(await rail(pg, 'class').innerText(), /Not chosen yet/);
  assert.match(await rail(pg, 'complication').innerText(), /Optional/);
  assert.match(await rail(pg, 'kit').innerText(), /Optional/, 'with no class picked the kit step reads as it always did');
});

test('a needed step on a phone has no Skip; an optional one does', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390 });
  assert.equal(await pg.locator('.hc-under [data-act="skip"]').count(), 0);
  await pg.evaluate(() => document.querySelector('[data-widget="hero-creator"]')._heroCreator.goStep(4, 'fwd'));
  assert.equal(await pg.locator('.hc-under [data-act="skip"]').count(), 1);
});

test('on a phone, the chip of a step the class does not use reads Not used', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390 });
  await pg.evaluate(() => document.querySelector('[data-widget="hero-creator"]')._heroCreator.goStep(1, 'fwd'));
  await pg.locator('.hc-card', { hasText: 'Conduit' }).click();
  await pg.click('.hc-sfoot [data-act="choose"]');
  await pg.evaluate(() => document.querySelector('[data-widget="hero-creator"]')._heroCreator.goStep(3, 'fwd'));
  const chips = await pg.locator('.hc-chip').allInnerTexts();
  assert.ok(chips.some((c) => /kit\s*not used/i.test(c)), chips.join(' | '));
  assert.ok(!chips.some((c) => /kit\s*choosing/i.test(c)), 'an unused step is not being chosen');
});

test('a dependent step offers only the picked parent\'s entries and asks for the parent first', { skip }, async (t) => {
  const { pg } = await open(t);
  await rail(pg, 'subclass').click();
  assert.match(await text(pg, '.hc-empty'), /Choose a class first/);
  assert.equal(await pg.locator('.hc-tools').count(), 0);
  await rail(pg, 'class').click();
  await pick(pg, 'Fury');
  await rail(pg, 'subclass').click();
  assert.deepEqual(await pg.locator('.hc-gcard .hc-disp').allInnerTexts(), ['Berserker', 'Reaver']);
  assert.match(await text(pg, '.hc-intro'), /2 to choose from/i);
});

test('changing the parent clears a pick that no longer fits and says so', { skip }, async (t) => {
  const { pg } = await open(t);
  await rail(pg, 'class').click();
  await pick(pg, 'Fury');
  await rail(pg, 'subclass').click();
  await pick(pg, 'Reaver');
  assert.match(await rail(pg, 'subclass').innerText(), /Reaver/);
  await rail(pg, 'class').click();
  await pick(pg, 'Conduit');
  assert.match(await rail(pg, 'subclass').innerText(), /Not chosen yet/);
  assert.match(await live(pg), /Cleared subclass/);
  await rail(pg, 'subclass').click();
  assert.deepEqual(await pg.locator('.hc-gcard .hc-disp').allInnerTexts(), ['Light', 'Shade']);
});

test('a count of 0 reads Not used with its note, is skipped, and is never sent', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await rail(pg, 'class').click();
  await pick(pg, 'Fury');
  await rail(pg, 'kit').click();
  await pick(pg, 'Panther');
  assert.match(await rail(pg, 'kit').innerText(), /Panther/);
  await rail(pg, 'class').click();
  await pick(pg, 'Conduit');
  assert.match(await rail(pg, 'kit').innerText(), /Not used/);
  assert.match(await live(pg), /Cleared kit/, 'the pick the class no longer allows is dropped');
  await rail(pg, 'kit').click();
  assert.match(await text(pg, '.hc-intro'), /not used/i);
  const empty = await text(pg, '.hc-empty');
  assert.equal(empty.trim(), 'Conduits choose a prayer and a ward instead.', "the system's note stands alone, not after a lead-in saying the same");
  // Picking the subclass moves on past the unused kit.
  await rail(pg, 'subclass').click();
  await pick(pg, 'Light');
  await pg.waitForFunction(() => document.querySelector('.hc-app').getAttribute('data-step') === '4');
  await rail(pg, 'ancestry').click();
  await pick(pg, 'Orc');
  await pg.locator('.hc-rail .hc-row').last().click();
  await pg.fill('[data-name]', 'Tess');
  assert.match(await text(pg, '.hc-sum'), /kit\s*not used/i);
  await pg.click('[data-act="create"]');
  await pg.waitForTimeout(100);
  assert.deepEqual(Object.keys(seen.post[0].picks).sort(), ['ancestry', 'class', 'subclass']);
});

test('a parent that lists nothing leaves the dependent step unused', { skip }, async (t) => {
  const { pg } = await open(t);
  await rail(pg, 'class').click();
  await pick(pg, 'Beastheart');
  assert.match(await rail(pg, 'subclass').innerText(), /Not used/);
  await rail(pg, 'subclass').click();
  assert.match(await text(pg, '.hc-empty'), /A Beastheart doesn.t use a subclass/);
  assert.match(await rail(pg, 'kit').innerText(), /Not chosen yet/, 'a count of one makes the kit needed');
});

test('a subclass can override the class\'s kit count', { skip }, async (t) => {
  const { pg } = await open(t);
  await rail(pg, 'class').click();
  await pick(pg, 'Fury');
  assert.match(await rail(pg, 'kit').innerText(), /Not chosen yet/);
  await rail(pg, 'subclass').click();
  await pick(pg, 'Berserker');
  assert.match(await rail(pg, 'kit').innerText(), /Not used/);
});

test('the die deals only from the entries the step offers', { skip }, async (t) => {
  const { pg } = await open(t);
  await rail(pg, 'class').click();
  await pick(pg, 'Fury');
  await rail(pg, 'subclass').click();
  for (let i = 0; i < 6; i++) {
    await pg.click('[data-act="die"]');
    await pg.waitForTimeout(80);
    const at = await pg.evaluate(() => { const c = document.querySelector('[data-widget="hero-creator"]')._heroCreator; return c.curEntry().name; });
    assert.ok(['Berserker', 'Reaver'].includes(at), 'dealt ' + at);
  }
});

test('a restored draft drops a pick that no longer fits its parent or count', { skip }, async (t) => {
  const keep = await open(t, { draft: { picks: { class: { name: 'Fury', options: [] }, subclass: { name: 'Reaver', options: [] }, kit: { name: 'Panther', options: [] } }, name: '' } });
  assert.match(await rail(keep.pg, 'subclass').innerText(), /Reaver/);
  assert.match(await rail(keep.pg, 'kit').innerText(), /Panther/);
  const bad = await open(t, { draft: { picks: { class: { name: 'Conduit', options: [] }, subclass: { name: 'Reaver', options: [] }, kit: { name: 'Panther', options: [] } }, name: '' } });
  assert.match(await rail(bad.pg, 'subclass').innerText(), /Not chosen yet/);
  assert.match(await rail(bad.pg, 'kit').innerText(), /Not used/);
});

test('practice shows the same shape and sends nothing for an unused step', { skip }, async (t) => {
  const { pg } = await open(t, { practice: true });
  await rail(pg, 'class').click();
  await pick(pg, 'Conduit');
  assert.match(await rail(pg, 'kit').innerText(), /Not used/);
  assert.match(await rail(pg, 'ancestry').innerText(), /Not chosen yet/);
});
