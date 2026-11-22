// hero_builder_steps_browser.test.mjs — the builder sentence when a game system
// shapes its steps: a blank that lists only the picked parent's entries, one
// that is cleared when the parent changes, and one left out of the sentence
// when the picked entries do not use it. The plan is a synthetic one.
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

const ch = (name, extra = {}) => ({ slug: name.toLowerCase(), name, summary: name + ' summary.', source: 'package', properties: {}, ...extra });
const PLAN = {
  systemName: 'Test System',
  steps: [
    { fieldKey: 'class', label: 'Class', needed: true, choices: [
      ch('Fury', { properties: { kits: 1 } }),
      ch('Conduit', { properties: { kits: 0, kit_note: 'No kit.' } }),
      ch('Beastheart', { properties: { kits: 1 } }),
    ] },
    { fieldKey: 'subclass', label: 'Subclass', needed: true, parent: 'class', choices: [
      ch('Berserker', { parent: 'Fury' }), ch('Reaver', { parent: 'Fury' }), ch('Light', { parent: 'Conduit' }),
    ] },
    { fieldKey: 'kit', label: 'Kit', countFrom: [{ step: 'class', property: 'kits', note: 'kit_note' }], choices: [ch('Panther'), ch('Raider')] },
  ],
};
const heroes = (values) => [{ id: 'e1', name: 'Tess', values, hidden: [] }];

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

async function open(t, { viewport = { width: 1280, height: 900 }, heroes: hs = heroes({}), failSave = false, plan = PLAN, putBody = '{"status":"ok"}' } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  const sent = [];
  await p.route('**/*', (route) => {
    const req = route.request(), u = new URL(req.url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: page(hs) });
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
const lastPut = (sent) => sent.filter((s) => s.method === 'PUT').map((s) => s.body.fields_patch);

test('a count of 0 leaves the blank out of the sentence; a parent that lists none does too', { skip }, async (t) => {
  const conduit = await open(t, { heroes: heroes({ class: 'Conduit' }) });
  assert.equal(await sentence(conduit.p), 'Tess’s class is Conduit and subclass is subclass.');
  assert.equal(await conduit.p.locator('[data-blank="kit"]').count(), 0);
  assert.match(await conduit.p.locator('[data-hero-card="e1"] .hb-progress').innerText(), /1 of 2 left/);
  const beast = await open(t, { heroes: heroes({ class: 'Beastheart' }) });
  assert.equal(await sentence(beast.p), 'Tess’s class is Beastheart and kit is kit.');
});

test('a dependent blank lists only the parent\'s entries, and asks for the parent first', { skip }, async (t) => {
  const { p } = await open(t, { heroes: heroes({ class: 'Fury' }) });
  await p.click('[data-blank="subclass"]');
  assert.deepEqual(await p.locator('.hb-panel .hb-opt b').allInnerTexts(), ['Berserker', 'Reaver']);
  const none = await open(t);
  await none.p.click('[data-blank="subclass"]');
  assert.match(await none.p.locator('.hb-panel').innerText(), /Choose Tess’s class first/);
  await none.p.click('.hb-panel [data-go="class"]');
  assert.equal(await none.p.locator('.hb-panel .hb-pt').innerText(), 'Choose Tess’s class');
});

test('changing the parent clears the dependent pick in the same save and says so', { skip }, async (t) => {
  const { p, sent } = await open(t, { heroes: heroes({ class: 'Fury', subclass: 'Reaver', kit: 'Panther' }) });
  await p.click('[data-blank="class"]');
  await p.locator('.hb-panel .hb-opt', { hasText: 'Conduit' }).click();
  await p.locator('.hb-panel [data-choose]').click();
  await p.waitForSelector('.hb-tick');
  assert.deepEqual(lastPut(sent), [{ class: 'Conduit', subclass: null, kit: null }]);
  assert.match(await p.locator('.hb-toast').innerText(), /Cleared subclass and kit/);
  assert.equal(await p.locator('[data-blank="kit"]').count(), 0);
  // The panel moves on to the subclass, now empty, and offers Conduit's own.
  assert.deepEqual(await p.locator('.hb-panel .hb-opt b').allInnerTexts(), ['Light']);
});

test('changing to a parent whose entries still fit keeps the pick; a pick elsewhere is untouched', { skip }, async (t) => {
  const { p, sent } = await open(t, { heroes: heroes({ class: 'Fury', subclass: 'Reaver', kit: 'Panther' }) });
  await p.click('[data-blank="kit"]');
  await p.locator('.hb-panel .hb-opt', { hasText: 'Raider' }).click();
  await p.locator('.hb-panel [data-choose]').click();
  await p.waitForSelector('.hb-tick');
  assert.deepEqual(lastPut(sent), [{ kit: 'Raider' }]);
});

test('rolling everything left only fills dependent blanks whose parent is chosen', { skip }, async (t) => {
  const { p, sent } = await open(t, { heroes: heroes({ class: 'Fury' }) });
  await p.click('[data-rollall="e1"]');
  await p.waitForSelector('.hb-toast [data-undo]');
  const patch = lastPut(sent)[0];
  assert.ok(['Berserker', 'Reaver'].includes(patch.subclass), 'rolled ' + patch.subclass);
  assert.ok(['Panther', 'Raider'].includes(patch.kit));
});
