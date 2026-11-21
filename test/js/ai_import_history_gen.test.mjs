// ai_import_history_gen.test.mjs — the AI Import review screen's runner for a
// history generator row: it runs the real engine on the server's plan and
// writes only what the server saves, keeping the calendar's own eras.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);
const G = require('../../static/js/widgets/chronicle_gen.js');
const SRC = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'markdown_importer.js'), 'utf8');

// confirmHandler loads the importer and returns its htmx:confirm listener.
function confirmHandler() {
  let handler = null;
  const sandbox = {
    window: { ChronicleGen: G },
    Chronicle: { register() {}, escapeHtml: (s) => s },
    document: { addEventListener(type, fn) { if (type === 'htmx:confirm') handler = fn; }, head: { appendChild() {} } },
  };
  vm.runInNewContext(SRC, sandbox);
  assert.ok(handler, 'the importer listens for htmx:confirm');
  return handler;
}

// runRow presses Import on a review form holding one history row.
function runRow(plan) {
  const handler = confirmHandler();
  const out = { value: '' };
  const row = {
    getAttribute: () => JSON.stringify(plan),
    querySelector: (sel) => (sel.includes('_include') ? { checked: true } : sel.includes('_generated') ? out : null),
  };
  const form = { id: 'ai-import-review', querySelectorAll: () => [row], getAttribute: () => '/static/js/widgets/chronicle_gen.js' };
  return new Promise((resolve) => {
    handler({ preventDefault() {}, detail: { elt: form, issueRequest: () => resolve(out.value) } });
  });
}

const months = Array.from({ length: 12 }, (_, i) => ({ name: 'Month ' + (i + 1), days: 30 }));
// plan is a history row's plan as the server's generator kind sends it.
function plan(over = {}) {
  const keepEras = over.keepEras || [];
  return {
    generator: 'history', seed: 'seed-1', basedOn: 'history.kingdom', makesEras: true,
    calendar: { id: 'cal', name: 'Reckoning', months, weekdays: [{ name: 'One' }, { name: 'Two' }], current_year: 1492, current_month: 1, current_day: 1, eras: keepEras },
    recipe: { generator: 'history', makes: {}, details: {} },
    ...over,
    keepEras,
  };
}
const END = (e) => (e.end_year == null ? Infinity : e.end_year);
const overlaps = (a, b) => a.start_year <= END(b) && b.start_year <= END(a);

test('a history run sends eras and events with only the fields the server saves', async () => {
  const got = JSON.parse(await runRow(plan()));
  assert.ok(got.eras.length >= 2, 'eras were made');
  assert.ok(got.events.length > 10, 'events were made');
  for (const e of got.eras) {
    assert.deepEqual(Object.keys(e).sort(), ['color', 'description', 'end_year', 'name', 'start_year']);
    assert.match(e.color, /^#[0-9a-f]{6}$/, 'era colours are hex, as the calendar stores them');
    assert.ok(e.start_year >= 1 && END(e) >= e.start_year);
  }
  got.eras.forEach((a, i) => got.eras.slice(i + 1).forEach((b) => assert.ok(!overlaps(a, b), a.name + ' overlaps ' + b.name)));
  for (const e of got.events) {
    assert.deepEqual(Object.keys(e).sort(), ['day', 'description', 'month', 'name', 'visibility', 'year']);
    assert.ok(e.year <= 1492, 'history is in the past');
    assert.ok(e.visibility === 'everyone' || e.visibility === 'dm_only');
  }
  assert.ok(got.events.some((e) => e.visibility === 'dm_only'), 'hidden history stays marked Director-only');
  assert.equal(await runRow(plan()), JSON.stringify(got), 'the plan\'s seed makes the same history every time');
});

test('the row\'s settings lie over the built-in', async () => {
  const got = JSON.parse(await runRow(plan({ basedOn: 'history.dark', recipe: { generator: 'history', makes: {}, details: { years: 100, secrets: false } } })));
  assert.ok(got.events.length > 0);
  assert.ok(got.events.every((e) => e.year >= 1392), 'years: 100 kept the history to the last century');
  assert.ok(got.events.every((e) => e.visibility === 'everyone'), 'secrets: false made no hidden history');
});

test('a plan that makes no eras sends none', async () => {
  const got = JSON.parse(await runRow(plan({ makesEras: false })));
  assert.deepEqual(got.eras, []);
  assert.ok(got.events.length > 0, 'events still go');
});

test('the calendar\'s own era is kept: generated eras that overlap it give way', async () => {
  const always = { name: 'The Long Now', start_year: 1, end_year: null };
  let got = JSON.parse(await runRow(plan({ keepEras: [always] })));
  assert.deepEqual(got.eras, [], 'an era covering all time leaves no room');
  assert.ok(got.events.length > 0);

  // A later open era: the generator's own check reads an open end as this
  // year, the server as forever, so the runner applies the server's rule.
  const later = { name: 'Prophecy', start_year: 1600, end_year: null };
  got = JSON.parse(await runRow(plan({ keepEras: [later] })));
  assert.ok(got.eras.every((e) => !overlaps(e, later)), 'no generated era runs on into a later era');
  assert.ok(got.eras.every((e) => e.name !== 'Prophecy'), 'the kept era is not sent back');
});
