// hero_builder_browser.test.mjs — the hero builder on the Characters page.
//
// Runs hero_builder.js through boot.js's on-sight loader in a real Chromium,
// with the plan, fields and create endpoints answered here. It pins what the
// player sees and what reaches the server: a blank opens every choice beside
// the sentence, a pick saves only that field, rolling everything left saves
// once and Undo clears what it set, a failed save puts the blank back, and on
// a phone the panel opens under the hero it fills in without widening the
// page. A new hero is made in the creator: Create hero is a plain link to it,
// and the builder offers no sentence for one.
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

const names = (prefix, n) => Array.from({ length: n }, (_, i) => ({ slug: prefix + i, name: prefix + ' ' + (i + 1), summary: 'About ' + prefix + ' ' + (i + 1) + '.', source: 'Draw Steel', properties: {} }));
const PLAN = {
  systemName: 'Draw Steel',
  steps: [
    { fieldKey: 'ancestry', label: 'Ancestry', choices: [
      { slug: 'human', name: 'Human', summary: 'Quick to adapt.', source: 'Draw Steel', description: 'Humans are **everywhere**.', properties: { size: '1M', speed: 5 } },
      { slug: 'dwarf', name: 'Dwarf', summary: 'Stone-born.', source: 'Draw Steel', properties: { size: '1M', speed: 5 } },
      { slug: 'gull', name: 'Gull-kin', summary: 'A campaign people.', source: 'campaign', description: '<p>From the <b>coast</b>.</p>', properties: {} },
    ] },
    { fieldKey: 'class', label: 'Class', choices: [
      { slug: 'fury', name: 'Fury', summary: 'Rage.', source: 'Draw Steel', properties: { heroic_resource: 'Ferocity' } },
      { slug: 'tactician', name: 'Tactician', summary: 'Orders.', source: 'Draw Steel', properties: { heroic_resource: 'Focus' } },
    ] },
    { fieldKey: 'career', label: 'Career', choices: names('Career', 18) },
    { fieldKey: 'secret', label: 'Secret', choices: names('Secret', 3) },
  ],
};
const HEROES = [{ id: 'e1', name: 'Tess', values: { ancestry: 'Human' }, hidden: ['secret'] }];

const attr = (v) => JSON.stringify(v).replace(/&/g, '&amp;').replace(/'/g, '&#39;');
const page = (heroes) => `<!doctype html><html data-gm-term="Director"><head><meta charset="utf-8">
<style>:root{--color-accent:#4f46e5;--color-accent-rgb:79 70 229;--color-ok-rgb:22 163 74;--color-warn-rgb:217 119 6;--color-bad-rgb:220 38 38;
--color-bg-primary:#f9fafb;--color-bg-secondary:#fff;--color-bg-tertiary:#f3f4f6;--color-border:#e5e7eb;--color-text-primary:#111827;--color-text-body:#374151;--color-text-secondary:#6b7280;
--elev-resting:0 1px 2px rgb(0 0 0/.05);--elev-hover:0 6px 16px -4px rgb(0 0 0/.12);--elev-dragged:0 16px 36px -8px rgb(0 0 0/.22);
--dur-micro:120ms;--dur-standard:200ms;--dur-large:280ms;--dur-slide:360ms;--ease-out:cubic-bezier(.16,1,.3,1);--ease-slide:cubic-bezier(.22,.8,.24,1)}
html,body{margin:0;font:14px system-ui}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)}</style></head><body>
<main id="main-content" style="padding:24px 16px"><div id="cast-content">
<a href="/campaigns/c1/characters/new" data-create-hero="true">Create hero</a>
<link rel="stylesheet" href="/static/plugins/entities/css/hero_builder.css">
<section class="hb" data-widget="hero-builder" data-plan-url="/campaigns/c1/characters/new/plan"
  data-fields-base="/campaigns/c1/entities/" data-campaign-id="c1" data-csrf="tok" data-heroes='${attr(heroes)}'><p class="hb-wait">Opening the hero builder…</p></section>
<div style="height:1200px"></div></div></main>
<script type="application/json" id="chronicle-widget-scripts">{"hero-builder":["/static/plugins/entities/js/hero_rules_lines.js","/static/plugins/entities/js/hero_builder.js"]}</script>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} };</script>
<script src="/static/js/boot.js"></script>
</body></html>`;

function staticFile(path) {
  if (path.startsWith('/static/plugins/entities/')) return join(root, 'internal/plugins/entities/static', path.slice('/static/plugins/entities/'.length));
  return join(root, path);
}

async function open(t, { viewport = { width: 1280, height: 900 }, heroes = HEROES, failSave = false, plan = PLAN, putBody = '{"status":"ok"}' } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  const sent = [];
  await p.route('**/*', (route) => {
    const req = route.request(), u = new URL(req.url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: page(heroes) });
    if (u.pathname === '/campaigns/c1/characters/new/plan') return route.fulfill({ contentType: 'application/json', body: JSON.stringify(plan) });
    if (u.pathname.startsWith('/campaigns/c1/')) {
      sent.push({ method: req.method(), path: u.pathname, csrf: req.headers()['x-csrf-token'], body: JSON.parse(req.postData() || 'null') });
      if (req.method() === 'POST') return route.fulfill({ status: 201, contentType: 'application/json', body: '{}' });
      if (failSave) return route.fulfill({ status: 400, contentType: 'application/json', body: '{"message":"\\"Fury\\" is not on the class list"}' });
      return route.fulfill({ contentType: 'application/json', body: putBody });
    }
    if (u.pathname.startsWith('/static/')) {
      const file = staticFile(u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    }
    return route.fulfill({ status: 404, body: '' });
  });
  await p.goto('http://chronicle.test/');
  await p.waitForSelector('[data-hb-ready]', { state: 'attached' });
  return { p, sent };
}

const sentence = (p) => p.locator('[data-hero-card="e1"] .hb-sentence').innerText();

test('a hero reads as one sentence, with GM-only steps left out', { skip }, async (t) => {
  const { p } = await open(t);
  assert.equal(await sentence(p), 'Tess’s ancestry is Human, class is class and career is career.');
  assert.equal(await p.locator('[data-blank="secret"]').count(), 0);
  assert.match(await p.locator('[data-hero-card="e1"] .hb-progress').innerText(), /2 of 3 left/);
});

test('a blank opens every choice beside the sentence, and a pick saves only that field', { skip }, async (t) => {
  const { p, sent } = await open(t);
  await p.click('[data-blank="class"]');
  const panel = p.locator('.hb-panel');
  assert.equal(await panel.isVisible(), true);
  assert.equal(await panel.locator('.hb-pt').innerText(), 'Choose Tess’s class');
  assert.deepEqual(await panel.locator('.hb-rg').allInnerTexts(), ['1–5', '6–10']);
  const box = await panel.boundingBox(), line = await p.locator('[data-hero-card="e1"] .hb-sentence').boundingBox();
  assert.ok(box.x >= line.x + line.width - 1, 'the panel sits beside the sentence, not over it');

  await panel.locator('.hb-opt', { hasText: 'Tactician' }).click();
  assert.match(await panel.locator('.hb-f').innerText(), /Heroic resource\s+Focus/);
  await panel.locator('[data-choose]').click();
  await p.waitForSelector('.hb-tick');
  assert.deepEqual(sent, [{ method: 'PUT', path: '/campaigns/c1/entities/e1/fields', csrf: 'tok', body: { fields_patch: { class: 'Tactician' } } }]);
  // The panel moves on to the next blank, a long list with a search box and percentile ranges.
  assert.equal(await panel.locator('.hb-pt').innerText(), 'Choose Tess’s career');
  assert.equal(await panel.locator('.hb-rg').first().innerText(), '01–05');
  await panel.locator('[data-hb-q]').fill('career 12');
  assert.equal(await panel.locator('.hb-opt:visible').count(), 1);

  await p.keyboard.press('Escape');
  assert.equal(await panel.isVisible(), false);
  assert.equal(await p.evaluate(() => document.activeElement.getAttribute('data-blank')), 'career');
});

test('rolling everything left saves once, and Undo clears what it set', { skip }, async (t) => {
  const { p, sent } = await open(t);
  await p.click('[data-rollall="e1"]');
  await p.waitForSelector('.hb-toast [data-undo]');
  assert.equal(sent.length, 1);
  assert.deepEqual(Object.keys(sent[0].body.fields_patch).sort(), ['career', 'class']);
  assert.match(await sentence(p), /^Tess’s ancestry is Human, class is (Fury|Tactician) and career is Career \d+\.$/);
  await p.click('.hb-toast [data-undo]');
  await p.waitForFunction(() => document.querySelectorAll('.hb-blank.is-empty').length === 2);
  // The blanks clear before the save goes out, so wait for the request too.
  for (let i = 0; sent.length < 2 && i < 100; i++) await p.waitForTimeout(20);
  assert.deepEqual(sent[1].body, { fields_patch: { class: null, career: null } });
});

test('a refused save puts the blank back and says why', { skip }, async (t) => {
  const { p } = await open(t, { failSave: true });
  await p.click('[data-blank="class"]');
  await p.click('.hb-panel [data-choose]');
  await p.waitForSelector('.hb-toast:not([hidden])');
  assert.match(await p.locator('.hb-toast').innerText(), /not on the class list/);
  assert.equal(await p.locator('[data-blank="class"]').innerText(), 'class');
});

const DEVIL = { slug: 'devil', name: 'Devil', summary: 'Silver-tongued.', source: 'Draw Steel', properties: {
  ancestry_points: 3,
  purchased_traits: [{ name: 'Wings', cost: 2 }, { name: 'Silver Tongue', cost: 1 }, { name: 'Hellfire', cost: 2 }],
  signature_traits: [{ name: 'Impressive Horns' }],
} };
const BUY_PLAN = { systemName: 'Draw Steel', steps: [
  { fieldKey: 'ancestry', label: 'Ancestry', choices: [DEVIL, PLAN.steps[0].choices[1]] },
  { fieldKey: 'culture', label: 'Culture', choices: [{ slug: 'wode', name: 'Wode', summary: 'Forest folk.', source: 'Draw Steel', properties: { languages: 'One language', skill_options: 'One skill', skill_options_quick_build: 'Lore' } }] },
] };

test('an ancestry with points keeps the panel on it to buy traits, within its points', { skip }, async (t) => {
  const { p, sent } = await open(t, { heroes: [{ id: 'e1', name: 'Vex', values: {}, hidden: [] }], plan: BUY_PLAN });
  await p.click('[data-blank="ancestry"]');
  await p.click('.hb-panel [data-choose]');
  assert.equal(await p.locator('.hb-pt').innerText(), 'Choose Vex’s ancestry', 'the panel stays on the ancestry');
  assert.match(await p.locator('[data-hb-buy] legend').innerText(), /Spend Devil’s 3 points/);
  await p.check('[data-buy="Wings"]');
  await p.waitForFunction(() => /1 of 3 left/.test(document.querySelector('.hb-buy-left').textContent));
  assert.equal(await p.locator('[data-buy="Hellfire"]').isDisabled(), true, 'a trait that would overspend is off');
  await p.check('[data-buy="Silver Tongue"]');
  await p.waitForFunction(() => /0 of 3/.test(document.querySelector('.hb-buy-left').textContent));
  // The culture's grants are chosen on the sheet, which the panel says.
  await p.click('.hb-panel [data-go="culture"]');
  assert.match(await p.locator('.hb-detail').innerText(), /You choose the skills and languages this gives on Vex’s sheet\./);
  assert.deepEqual(sent.map((x) => x.body), [
    { fields_patch: { ancestry: 'Devil' } },
    { fields_patch: { ancestry_choices_json: '["Wings"]' } },
    { fields_patch: { ancestry_choices_json: '["Wings","Silver Tongue"]' } },
  ]);
});

test('on a saved hero, bought traits save as one list, and a new ancestry drops them', { skip }, async (t) => {
  const heroes = [{ id: 'e1', name: 'Tess', values: { ancestry: 'Devil', ancestry_choices_json: '["Wings"]' }, hidden: [] }];
  const { p, sent } = await open(t, { heroes, plan: BUY_PLAN });
  await p.click('[data-blank="ancestry"]');
  assert.equal(await p.locator('[data-buy="Wings"]').isChecked(), true);
  await p.check('[data-buy="Silver Tongue"]');
  await p.waitForFunction(() => document.querySelector('.hb-buy-left') && /0 of 3/.test(document.querySelector('.hb-buy-left').textContent));
  assert.deepEqual(sent[0].body, { fields_patch: { ancestry_choices_json: '["Wings","Silver Tongue"]' } });
  await p.click('.hb-opt:has-text("Dwarf")');
  await p.click('.hb-panel [data-choose]');
  await p.waitForFunction(() => document.querySelectorAll('.hb-tick').length > 0);
  assert.deepEqual(sent[1].body, { fields_patch: { ancestry: 'Dwarf' } }, 'the server clears what Devil bought');
  await p.click('[data-blank="ancestry"]');
  await p.click('.hb-opt:has-text("Devil")');
  await p.click('.hb-panel [data-choose]');
  assert.equal(await p.locator('[data-buy="Wings"]').isChecked(), false, 'traits from before are not brought back');
});

test('Create hero leads to the creator, and a sentence kept on this device never opens one here', { skip }, async (t) => {
  for (const heroes of [[], HEROES]) {
    const { p } = await open(t, { heroes });
    await p.evaluate(() => localStorage.setItem('chronicle:hero-builder:c1', JSON.stringify({ name: 'Kept', values: { ancestry: 'Human' }, open: true })));
    await p.reload();
    await p.waitForSelector('[data-hb-ready]', { state: 'attached' });
    assert.equal(await p.locator('[data-hero-card="new"], [data-make], [data-hb-name]').count(), 0, 'no new-hero sentence');
    if (!heroes.length) assert.equal(await p.evaluate(() => document.querySelector('[data-widget="hero-builder"]').hidden), true, 'with no heroes the builder takes no room');
    const nav = p.waitForRequest((r) => r.url() === 'http://chronicle.test/campaigns/c1/characters/new');
    await p.click('[data-create-hero]');
    await nav;
  }
});

test('on a phone the panel opens under the hero and the page does not widen', { skip }, async (t) => {
  const { p } = await open(t, { viewport: { width: 390, height: 800 } });
  await p.click('[data-blank="career"]');
  const card = await p.locator('[data-hero-card="e1"]').boundingBox(), box = await p.locator('.hb-panel').boundingBox();
  assert.ok(box.y >= card.y + card.height - 1, 'the panel opens under the hero');
  assert.equal(await p.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
  const close = await p.locator('.hb-x').boundingBox();
  assert.ok(close.width >= 32 && close.height >= 32, 'the close button is a full hit area');
});

test('with the dice engine on the page, its faces pick the entry', { skip }, async (t) => {
  const { p, sent } = await open(t);
  // Career has 18 entries, so percentile dice: tens 40 and ones 7 is 47,
  // which lands in Career 9 (45–50). Class has 2, so a d10: 7 is Tactician.
  await p.evaluate(() => {
    window.thrown = [];
    Chronicle.dice = { roll: (groups, opts) => { window.thrown.push({ groups, from: !!opts.from }); return Promise.resolve(groups.map((g) => ({ label: g.label, values: g.dice.length === 2 ? [40, 7] : [7] }))); } };
  });
  await p.click('[data-rollall="e1"]');
  await p.waitForSelector('.hb-toast [data-undo]');
  assert.deepEqual(sent[0].body, { fields_patch: { class: 'Tactician', career: 'Career 9' } });
  const thrown = await p.evaluate(() => window.thrown);
  assert.deepEqual(thrown, [{ from: true, groups: [{ label: 'Class', dice: [{ sides: 10 }] }, { label: 'Career', dice: [{ sides: 10, tens: true }, { sides: 10 }] }] }]);

  // 00 and a 10 on the ones die read as 100, the last entry.
  await p.evaluate(() => { Chronicle.dice.roll = (groups) => Promise.resolve(groups.map((g) => ({ label: g.label, values: [0, 10] }))); });
  await p.click('.hb-toast [data-undo]');
  await p.click('[data-blank="career"]');
  await p.click('.hb-panel [data-rollone]');
  await p.waitForFunction(() => document.querySelector('[data-blank="career"]').textContent === 'Career 18');

  // An engine that refuses falls back to an instant roll.
  // The panel has moved on to the class, the one blank left.
  await p.evaluate(() => { Chronicle.dice.roll = () => Promise.reject(new RangeError('no')); });
  assert.equal(await p.locator('.hb-pt').innerText(), 'Choose Tess’s class');
  await p.click('.hb-panel [data-rollone]');
  await p.waitForFunction(() => document.querySelector('[data-blank="class"]').textContent !== 'class');
});

// A Director's rules on the class and ancestry lists: Fury is Ask first, Null
// is Not here, and ancestry is a required list left with one entry.
const RULED = { ...PLAN, steps: [
  { fieldKey: 'ancestry', label: 'Ancestry', rules: { limited: true, required: true, prefilled: 'Human' }, choices: [PLAN.steps[0].choices[0]] },
  { fieldKey: 'class', label: 'Class', rules: { limited: true, notHereCount: 1, notHere: [{ name: 'Null', reason: 'No psionics here' }] }, choices: [
    { slug: 'fury', name: 'Fury', summary: 'Rage.', source: 'Draw Steel', state: 'ask_first', properties: {} },
    { slug: 'tactician', name: 'Tactician', summary: 'Orders.', source: 'Draw Steel', properties: {} },
    { slug: 'censor', name: 'Censor', summary: 'Fear.', source: 'Draw Steel', properties: {} },
  ] },
] };

test('an Ask first entry has a mark, is skipped by the dice, and is asked for, not saved', { skip }, async (t) => {
  const { p, sent } = await open(t, { plan: RULED, heroes: [{ id: 'e1', name: 'Tess', values: { ancestry: 'Human' }, hidden: [] }],
    putBody: '{"status":"ok","requests":[{"id":3,"fieldKey":"class","entryName":"Fury","status":"waiting"}]}' });
  await p.click('[data-blank="class"]');
  const panel = p.locator('.hb-panel');
  assert.deepEqual(await panel.locator('.hb-rg').allInnerTexts(), ['', '1–5', '6–10'], 'Ask first takes no number');
  assert.equal(await panel.locator('.hb-ask').innerText(), 'Ask first');
  await panel.locator('.hb-opt', { hasText: 'Fury' }).click();
  assert.equal(await panel.locator('[data-choose]').innerText(), 'Ask for Fury');
  await panel.locator('[data-choose]').click();
  await p.waitForSelector('.hb-blank.is-wait');
  assert.match(await sentence(p), /class is Fury waiting for your Director/);
  assert.match(await p.locator('[data-hero-card="e1"] .hb-progress').innerText(), /1 waiting for your Director/);
  assert.deepEqual(sent.map((x) => x.body), [{ fields_patch: { class: 'Fury' } }]);
  // Rolling everything left never lands on it: nothing is left to roll.
  assert.equal(await p.locator('[data-rollall]').count(), 0);
});

test('what is not here folds open at the list foot with the Director\'s reason', { skip }, async (t) => {
  const { p } = await open(t, { plan: RULED });
  await p.click('[data-blank="class"]');
  const foot = p.locator('.hb-nh [data-act="nothere"]');
  assert.equal(await foot.innerText(), '1 not here');
  assert.equal(await p.locator('.hb-nh-list').count(), 0);
  await foot.click();
  assert.match(await p.locator('.hb-nh-list').innerText(), /Null isn’t here\. Your Director says: “No psionics here”/);
});

test('the Director\'s note asking for another pick is shown, and a request waiting is shown beside a held pick', { skip }, async (t) => {
  const plan = { ...RULED, steps: [RULED.steps[0], { ...RULED.steps[1], rules: { limited: true, suggested: { id: 4, entryName: 'Fury', message: 'Try a Censor' } } }] };
  const { p } = await open(t, { plan, heroes: [{ id: 'e1', name: 'Tess', values: { ancestry: 'Human' }, hidden: [] }] });
  assert.match(await p.locator('[data-hero-card="e1"] .hb-suggest').innerText(), /asked for another class instead of Fury: “Try a Censor”/);
  const held = { ...RULED, steps: [RULED.steps[0], { ...RULED.steps[1], rules: { limited: true, waiting: { id: 5, entryName: 'Fury', entityId: 'e1' } } }] };
  const b = await open(t, { plan: held, heroes: [{ id: 'e1', name: 'Tess', values: { ancestry: 'Human', class: 'Censor' }, hidden: [] }] });
  assert.match(await b.p.locator('[data-hero-card="e1"] .hb-sentence').innerText(), /class is Censor asking for Fury, waiting for your Director/);
});

// The Director's times for changing picks. A shut hero says when the picks
// open again; a chosen step can be read but not changed, a blank can still be
// filled; an open window names its last day. A save the server locks shows
// the lock without a reload.
test('a shut hero keeps its picks, still fills blanks, and says when they open', { skip }, async (t) => {
  const heroes = [{ id: 'e1', name: 'Tess', values: { ancestry: 'Human' }, hidden: ['secret'], lock: { ruled: true, open: false, nextLevel: 3 } }];
  const { p, sent } = await open(t, { heroes });
  const line = p.locator('[data-hero-card="e1"] [data-hb-lock="shut"]');
  assert.equal(await line.innerText(), 'Your picks are set for now. They open again when Tess reaches level 3. Your Director can open them sooner. Blanks can still be filled.');
  await p.click('[data-blank="ancestry"]');
  const panel = p.locator('.hb-panel');
  assert.equal(await panel.locator('[data-choose]').count(), 0, 'a chosen step offers no Choose');
  assert.equal(await panel.locator('[data-rollone]').count(), 0, 'nor a roll');
  assert.match(await panel.locator('.hb-locked').innerText(), /Human stays Tess’s ancestry for now/);
  await p.click('[data-blank="class"]');
  await panel.locator('.hb-opt', { hasText: 'Fury' }).click();
  await panel.locator('[data-choose]').click();
  await p.waitForSelector('.hb-tick');
  assert.deepEqual(sent.map((s) => s.body), [{ fields_patch: { class: 'Fury' } }]);
});

test('an open window names its last day, and no times means no line', { skip }, async (t) => {
  const until = new Date(Date.now() + 3 * 864e5).toISOString();
  const { p } = await open(t, { heroes: [{ id: 'e1', name: 'Tess', values: {}, hidden: [], lock: { ruled: true, open: true, until, reopened: true } }] });
  assert.match(await p.locator('[data-hb-lock="open"]').innerText(), /^You can change your picks until .+\. Your Director opened them\.$/);
  const plain = await open(t);
  assert.equal(await plain.p.locator('[data-hb-lock]').count(), 0);
});

test('a save the server locks puts the pick back and shows the lock', { skip }, async (t) => {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const heroes = [{ id: 'e1', name: 'Tess', values: { ancestry: 'Human', class: 'Fury' }, hidden: ['secret'], lock: { ruled: true, open: true } }];
  await p.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: page(heroes) });
    if (u.pathname === '/campaigns/c1/characters/new/plan') return route.fulfill({ contentType: 'application/json', body: JSON.stringify(PLAN) });
    if (u.pathname.startsWith('/campaigns/c1/')) return route.fulfill({ status: 403, contentType: 'application/json', body: '{"message":"Your hero\'s picks are locked right now. Your picks are set. Your Director can open them again."}' });
    if (u.pathname.startsWith('/static/')) {
      const file = staticFile(u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    }
    return route.fulfill({ status: 404, body: '' });
  });
  await p.goto('http://chronicle.test/');
  await p.waitForSelector('[data-hb-ready]', { state: 'attached' });
  await p.click('[data-blank="class"]');
  await p.locator('.hb-panel .hb-opt', { hasText: 'Tactician' }).click();
  await p.locator('.hb-panel [data-choose]').click();
  await p.waitForSelector('[data-hb-lock="shut"]');
  assert.equal(await p.locator('[data-blank="class"]').innerText(), 'Fury');
  assert.match(await p.locator('.hb-toast').innerText(), /locked right now/);
});
