// hero_builder_browser.test.mjs — the hero builder on the Characters page.
//
// Runs hero_builder.js through boot.js's on-sight loader in a real Chromium,
// with the plan, fields and create endpoints answered here. It pins what the
// player sees and what reaches the server: a blank opens every choice beside
// the sentence, a pick saves only that field, rolling everything left saves
// once and Undo clears what it set, a failed save puts the blank back, a new
// hero is made from its sentence, and on a phone the panel opens under the
// hero it fills in without widening the page.
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
const page = (heroes, canCreate) => `<!doctype html><html><head><meta charset="utf-8">
<style>:root{--color-accent:#4f46e5;--color-accent-rgb:79 70 229;--color-ok-rgb:22 163 74;--color-warn-rgb:217 119 6;--color-bad-rgb:220 38 38;
--color-bg-primary:#f9fafb;--color-bg-secondary:#fff;--color-bg-tertiary:#f3f4f6;--color-border:#e5e7eb;--color-text-primary:#111827;--color-text-body:#374151;--color-text-secondary:#6b7280;
--elev-resting:0 1px 2px rgb(0 0 0/.05);--elev-hover:0 6px 16px -4px rgb(0 0 0/.12);--elev-dragged:0 16px 36px -8px rgb(0 0 0/.22);
--dur-micro:120ms;--dur-standard:200ms;--dur-large:280ms;--dur-slide:360ms;--ease-out:cubic-bezier(.16,1,.3,1);--ease-slide:cubic-bezier(.22,.8,.24,1)}
html,body{margin:0;font:14px system-ui}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)}</style></head><body>
<main id="main-content" style="padding:24px 16px"><div id="cast-content">
<a href="/campaigns/c1/characters/new" data-create-hero="true">Create hero</a>
<link rel="stylesheet" href="/static/plugins/entities/css/hero_builder.css">
<section class="hb" data-widget="hero-builder" data-plan-url="/campaigns/c1/characters/new/plan" data-create-url="/campaigns/c1/characters/new"
  data-fields-base="/campaigns/c1/entities/" data-campaign-id="c1" data-csrf="tok" data-can-create="${canCreate}" data-for-self="true"
  data-type-name="Hero" data-heroes='${attr(heroes)}'><p class="hb-wait">Opening the hero builder…</p></section>
<div style="height:1200px"></div></div></main>
<script type="application/json" id="chronicle-widget-scripts">{"hero-builder":["/static/plugins/entities/js/hero_builder.js"]}</script>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} };</script>
<script src="/static/js/boot.js"></script>
</body></html>`;

function staticFile(path) {
  if (path.startsWith('/static/plugins/entities/')) return join(root, 'internal/plugins/entities/static', path.slice('/static/plugins/entities/'.length));
  return join(root, path);
}

async function open(t, { viewport = { width: 1280, height: 900 }, heroes = HEROES, canCreate = false, failSave = false } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  const sent = [];
  await p.route('**/*', (route) => {
    const req = route.request(), u = new URL(req.url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: page(heroes, canCreate) });
    if (u.pathname === '/campaigns/c1/characters/new/plan') return route.fulfill({ contentType: 'application/json', body: JSON.stringify(PLAN) });
    if (u.pathname.startsWith('/campaigns/c1/')) {
      sent.push({ method: req.method(), path: u.pathname, csrf: req.headers()['x-csrf-token'], body: JSON.parse(req.postData() || 'null') });
      if (req.method() === 'POST') return route.fulfill({ status: 201, contentType: 'application/json', body: '{"id":"e9","url":"/campaigns/c1/entities/e9"}' });
      if (failSave) return route.fulfill({ status: 400, contentType: 'application/json', body: '{"message":"\\"Fury\\" is not on the class list"}' });
      return route.fulfill({ contentType: 'application/json', body: '{"status":"ok"}' });
    }
    if (u.pathname.startsWith('/static/')) {
      const file = staticFile(u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    }
    return route.fulfill({ status: 404, body: '' });
  });
  await p.goto('http://chronicle.test/');
  await p.waitForSelector('.hb-sentence');
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

test('a new hero is made from its own sentence', { skip }, async (t) => {
  const { p, sent } = await open(t, { heroes: [], canCreate: true });
  await p.click('[data-create-hero]');
  assert.equal(await p.evaluate(() => document.activeElement.hasAttribute('data-hb-name')), true);
  await p.click('[data-make]');
  assert.equal(await p.locator('[data-hb-error]').innerText(), 'Give your hero a name first.');
  await p.fill('[data-hb-name]', 'Bram');
  await p.click('[data-hero-card="new"] [data-blank="ancestry"]');
  assert.equal(await p.locator('.hb-pt').innerText(), 'Choose Bram’s ancestry');
  await p.click('.hb-opt:has-text("Gull-kin")');
  assert.match(await p.locator('.hb-detail').innerHTML(), /<b>coast<\/b>/);
  await p.click('.hb-panel [data-choose]');
  assert.equal(sent.length, 0, 'a new hero saves nothing until it is made');
  const nav = p.waitForRequest((r) => r.url() === 'http://chronicle.test/');
  await p.click('[data-make]');
  await nav;
  assert.deepEqual(sent, [{ method: 'POST', path: '/campaigns/c1/characters/new', csrf: 'tok', body: { name: 'Bram', picks: { ancestry: { name: 'Gull-kin' } } } }]);
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
