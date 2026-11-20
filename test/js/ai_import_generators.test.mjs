// markdown_importer.js's AI Import generator runner, for the whole-calendar
// and moons rows: what it sends back for the server to re-check, and that the
// server's lists of ids and built-ins match the engine's own.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(here, '..', '..');
const require = createRequire(import.meta.url);
const G = require('../../static/js/widgets/chronicle_gen.js');
const runnerSrc = readFileSync(path.join(root, 'static', 'js', 'widgets', 'markdown_importer.js'), 'utf8');
const goSrc = readFileSync(path.join(root, 'internal', 'plugins', 'calendar', 'calendarai', 'generator_calendar.go'), 'utf8');

// loadRunner evaluates the widget file against a stand-in page and returns
// the review form's htmx:confirm handler.
function loadRunner() {
  const listeners = {};
  const sandbox = {
    Chronicle: { register() {}, escapeHtml: (s) => s },
    window: { ChronicleGen: G },
    document: { addEventListener: (t, fn) => { listeners[t] = fn; }, head: { appendChild() {} } },
    JSON, Promise, Array, Object, Error,
  };
  vm.runInNewContext(runnerSrc, sandbox);
  return listeners['htmx:confirm'];
}

function row(plan) {
  const include = { checked: true };
  const out = { value: '' };
  return {
    out,
    getAttribute: (n) => (n === 'data-ai-gen' ? JSON.stringify(plan) : null),
    querySelector: (sel) => (sel.includes('_include') ? include : sel.includes('_generated') ? out : null),
  };
}

// runPlans presses Import on a review form holding these plans and returns
// each row's generated output, parsed.
async function runPlans(plans) {
  const confirm = loadRunner();
  const rows = plans.map(row);
  const form = { id: 'ai-import-review', querySelectorAll: () => rows, getAttribute: () => '' };
  await new Promise((resolve) => {
    confirm({ preventDefault() {}, detail: { elt: form, issueRequest: resolve } });
  });
  return rows.map((r) => (r.out.value ? JSON.parse(r.out.value) : null));
}

// goList reads `name = []string{...}` from the Go file.
function goList(name) {
  const m = goSrc.match(new RegExp(name + '\\s*=\\s*\\[\\]string\\{([^}]*)\\}'));
  assert.ok(m, name + ' not found in generator_calendar.go');
  return [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
}

test('calendar: a built-in with the row\'s settings laid over it comes back as a Chronicle preset', async () => {
  const plan = {
    generator: 'calendar', base: 'calendar.lunar', seed: 'abc',
    recipe: { generator: 'calendar', makes: { moons: 2, seasons: 3 }, details: { theme: 'elven', name: 'Starfall Count', currentYear: 812 } },
  };
  const [out, again] = await runPlans([plan, plan]);
  assert.equal(out.format, 'chronicle-calendar-v1');
  assert.equal(out.calendar.name, 'Starfall Count');
  assert.equal(out.calendar.current_year, 812);
  assert.equal(out.calendar.moons.length, 2, 'the row\'s moons override the built-in');
  assert.equal(out.calendar.seasons.length, 3);
  assert.ok(!out.calendar.events, 'the generator makes no events');
  assert.ok(G.cal.validatePreset(out).ok);
  assert.deepEqual(out, again, 'same seed, same calendar');
});

test('calendar: without a built-in the recipe runs as given', async () => {
  const [out] = await runPlans([{ generator: 'calendar', seed: 's1', recipe: { generator: 'calendar', makes: {}, details: { template: 'even' } } }]);
  assert.equal(out.format, 'chronicle-calendar-v1');
  assert.ok(out.calendar.months.length > 0);
});

test('moons: only the new moons come back, clear of the kept ones, with the hidden ones marked', async () => {
  const locked = [{ name: 'Selune', cycle_days: 30 }];
  const [out] = await runPlans([{
    generator: 'moons', seed: 'm1', locked,
    recipe: { generator: 'moons', makes: { count: 3 }, details: { hidden: 1, theme: 'grim' } },
  }]);
  assert.equal(out.length, 2, 'count includes the kept moon');
  for (const m of out) {
    assert.notEqual(m.name.toLowerCase(), 'selune');
    assert.ok(m.cycle_days > 0);
    assert.ok(Math.abs(m.cycle_days - 30) >= 2.5, 'cycles stay clear of the kept moon');
    assert.deepEqual(Object.keys(m).sort(), ['color', 'cycle_days', 'hidden', 'name', 'phase_offset']);
  }
  assert.equal(out.filter((m) => m.hidden).length, 1);
});

test('a plan the engine refuses leaves the row without output, for the server to report', async () => {
  const [out] = await runPlans([{ generator: 'calendar', base: 'calendar.nope', seed: 'x', recipe: {} }]);
  assert.equal(out, null);
});

test('the server\'s ids and built-ins are the engine\'s own', () => {
  const cal = G.generators.find((g) => g.id === 'calendar');
  const moons = G.generators.find((g) => g.id === 'moons');
  assert.ok(cal && moons);
  assert.deepEqual(goList('genCalTemplates').sort(), Object.keys(G.calendar.templates()).sort());
  assert.deepEqual(goList('genThemes').sort(), G.names.themes().map((t) => t.id).sort());
  assert.deepEqual(goList('genFreqs'), G.recipes.levels);
  const cycles = moons.details.cycles.options.map((o) => o.value);
  assert.deepEqual(goList('genMoonCycles').sort(), cycles.sort());
  const styles = [...goSrc.matchAll(/"[a-z-]+":\s*"(calendar\.[a-z-]+)"/g)].map((m) => m[1]).sort();
  assert.deepEqual(styles, G.recipes.builtIn('calendar').map((r) => r.id).sort());
  // Bounds the server enforces before the engine sees the recipe.
  assert.equal(cal.makes.moons.max, 5);
  assert.equal(cal.makes.seasons.max, 6);
  assert.equal(cal.details.months.max, 24);
  assert.equal(cal.details.weekLength.max, 14);
  assert.equal(cal.details.name.max, 80);
  assert.equal(moons.makes.count.max, 6);
});
