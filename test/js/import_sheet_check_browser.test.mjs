// import_sheet_check_browser.test.mjs — the AI Import review row's check of
// the numbers the AI wrote, against a game system's published formulas.
//
// Runs static/js/widgets/import_sheet_check.js in a real Chromium on the
// markup reviewRowSheetCheck renders, with a stand-in for the game system's
// Chronicle.sheetChecks entry.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const WIDGET = readFileSync(join(root, 'static/js/widgets/import_sheet_check.js'), 'utf8');

// The row's markup as reviewRowSheetCheck renders it.
const ROW = '<form><div data-widget="import-sheet-check" data-campaign-id="camp-1" data-sheet-kind="creature"' +
  ' data-fields="{&#34;level&#34;:1,&#34;stamina&#34;:20}" data-written="[&#34;stamina&#34;]"' +
  ' data-input="page_0_sheet_formula" class="air-sheet"></div>' +
  '<input type="hidden" name="page_0_sheet_formula" value=""></form>';

// check stands in for a game system: EV matches, Stamina and a damage line
// differ. It records what it was given.
const CHECK = `window.seen = null;
Chronicle.sheetChecks = { creature: { check: function (fields, env) {
  window.seen = { fields: fields, env: env };
  return Promise.resolve({
    note: 'Level 1 Platoon Harrier',
    items: [
      { id: 'ev', label: 'Encounter value', ai: '6', formula: '6', matches: true },
      { id: 'stamina', label: 'Stamina', ai: '20', formula: '30', matches: false },
      { id: 'spear', label: 'Hooked Spear', ai: '<b>4</b> / 6 / 8', formula: '3 / 6 / 7', matches: false }
    ],
    patch: function (ids) {
      var p = {};
      if (ids.indexOf('stamina') >= 0) p.stamina = 30;
      if (ids.indexOf('spear') >= 0) p.abilities_json = '[{"name":"Hooked Spear","tier1":"3 damage"}]';
      return p;
    }
  });
} } };`;

async function page(t, setup, kind) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent('<!doctype html><html><body>' + (kind ? ROW.replace('data-sheet-kind="creature"', 'data-sheet-kind="' + kind + '"') : ROW) + '</body></html>');
  await p.addScriptTag({ content: 'window.Chronicle = { register: function (n, impl) { window.impl = impl; } };' + (setup || '') });
  await p.addScriptTag({ content: WIDGET });
  await p.evaluate(() => window.impl.init(document.querySelector('[data-widget]')));
  await p.waitForTimeout(50);
  return p;
}

const state = (p) => p.evaluate(() => ({
  note: (document.querySelector('.air-sheet > .air-line') || {}).textContent || '',
  tiles: [...document.querySelectorAll('.air-stat')].map((s) => ({
    tag: s.tagName, cls: s.className, label: s.querySelector('span').textContent,
    value: s.querySelector('b').textContent, small: s.querySelector('small').textContent,
    pressed: s.getAttribute('aria-pressed'), html: s.querySelector('b').innerHTML,
  })),
  all: (document.querySelector('.air-stats-all') || {}).textContent || '',
  input: document.querySelector('input[name="page_0_sheet_formula"]').value,
}));

test('marks what differs, and the director takes the formula for all, or for one', { skip }, async (t) => {
  const p = await page(t, CHECK);
  const seen = await p.evaluate(() => window.seen);
  assert.deepEqual(seen, { fields: { level: 1, stamina: 20 }, env: { campaignId: 'camp-1', written: ['stamina'], stored: {} } });

  let s = await state(p);
  assert.equal(s.note, 'Level 1 Platoon Harrier');
  assert.deepEqual(s.tiles.map((x) => [x.tag, x.label, x.value, x.small]), [
    ['DIV', 'Encounter value', '6', 'Matches the formula'],
    ['BUTTON', 'Stamina', '20', 'Formula gives 30'],
    ['BUTTON', 'Hooked Spear', '<b>4</b> / 6 / 8', 'Formula gives 3 / 6 / 7'],
  ]);
  assert.equal(s.tiles[2].html, '&lt;b&gt;4&lt;/b&gt; / 6 / 8', 'the AI’s text is shown, never run as markup');
  assert.ok(s.tiles[1].cls.includes('is-off') && s.tiles[0].cls.includes('is-ok'));
  assert.equal(s.all, 'Use the formula for all 2');
  assert.equal(s.input, '', 'nothing is replaced without a press');

  await p.click('.air-stats-all');
  s = await state(p);
  assert.deepEqual(s.tiles.slice(1).map((x) => [x.value, x.small, x.pressed]), [
    ['30', 'Formula used (AI wrote 20)', 'true'],
    ['3 / 6 / 7', 'Formula used (AI wrote <b>4</b> / 6 / 8)', 'true'],
  ]);
  assert.equal(s.all, 'Keep the AI’s numbers');
  assert.deepEqual(JSON.parse(s.input), { stamina: 30, abilities_json: '[{"name":"Hooked Spear","tier1":"3 damage"}]' });

  await p.click('.air-stat[data-id="spear"]');
  s = await state(p);
  assert.equal(s.tiles[2].small, 'Formula gives 3 / 6 / 7');
  assert.equal(s.all, 'Use the formula for all 2');
  assert.deepEqual(JSON.parse(s.input), { stamina: 30 });

  await p.click('.air-stats-all');
  await p.click('.air-stats-all');
  s = await state(p);
  assert.equal(s.input, '', 'keeping the AI’s numbers sends none');
});

test('says so when the game system has no check or its check fails, and stays empty when it has nothing to compare', { skip }, async (t) => {
  const cases = [
    ['', /no check for these numbers/],
    ['Chronicle.sheetChecks = { creature: { check: function () { throw new Error("no refs"); } } };', /could not run/],
    ['Chronicle.sheetChecks = { creature: { check: function () { return Promise.resolve({ items: [] }); } } };', null],
  ];
  for (const [setup, line] of cases) {
    const p = await page(t, setup);
    const s = await state(p);
    assert.deepEqual([s.tiles.length, s.all, s.input], [0, '', '']);
    const text = await p.evaluate(() => document.querySelector('.air-sheet').textContent);
    if (line) assert.match(text, line); else assert.equal(text, '');
  }
  // A row that isn't a stat block has no figures to check, so it says nothing.
  const item = await page(t, '', 'item');
  assert.equal(await item.evaluate(() => document.querySelector('.air-sheet').textContent), '');
});

test('shows every number up to the cap and says how many it did not check', { skip }, async (t) => {
  const many = (n) => 'Chronicle.sheetChecks = { creature: { check: function () { var items = []; for (var i = 0; i < ' + n +
    '; i++) items.push({ id: "n" + i, label: "Number " + i, ai: "1", formula: "2", matches: false });' +
    ' return Promise.resolve({ items: items, patch: function (ids) { return { count: ids.length }; } }); } } };';
  let p = await page(t, many(60));
  let s = await state(p);
  assert.equal(s.tiles.length, 60);
  assert.equal(s.all, 'Use the formula for all 60');
  assert.equal(await p.evaluate(() => document.querySelectorAll('.air-bad').length), 0);

  p = await page(t, many(230));
  s = await state(p);
  assert.equal(s.tiles.length, 200);
  assert.equal(s.all, 'Use the formula for all 200');
  const note = await p.evaluate(() => document.querySelector('.air-bad').textContent);
  assert.match(note, /first 200 numbers\. 30 more were not checked/);
});

// A check for a creature the AI left figures off: blank figures open taken,
// a figure that followed the old level opens taken, one somebody set opens
// off, and a figure no formula covers is a plain line.
const OPEN = `Chronicle.sheetChecks = { creature: { check: function () {
  return Promise.resolve({
    items: [
      { id: 'ev', label: 'Encounter value', ai: '', formula: '4', start: 'used', blank: true },
      { id: 'stamina', label: 'Stamina', ai: '30', formula: '50', start: 'used', from: 'stored',
        taken: 'Follows the formula, as at level 1 (was 30)' },
      { id: 'free_strike', label: 'Free strike', ai: '6', formula: '5', from: 'stored',
        kept: 'Yours, kept. Formula gives 5' },
      { id: 'swarm', said: 'No published formula covers a swarm’s Stamina.' }
    ],
    patch: function (ids) {
      var p = {};
      if (ids.indexOf('ev') >= 0) p.ev = 4;
      if (ids.indexOf('stamina') >= 0) p.stamina = 50;
      if (ids.indexOf('free_strike') >= 0) p.free_strike = 5;
      return p;
    }
  });
} } };`;

test('figures the AI left out open taken, and a press puts them back', { skip }, async (t) => {
  const p = await page(t, OPEN);
  let s = await state(p);
  assert.deepEqual(s.tiles.map((x) => x.cls.replace('air-stat ', '')), ['is-used', 'is-used', 'is-off']);
  assert.equal(s.tiles[0].small, 'Formula used (was blank)');
  assert.equal(s.tiles[1].small, 'Follows the formula, as at level 1 (was 30)');
  assert.equal(s.tiles[2].small, 'Yours, kept. Formula gives 5');
  assert.equal(s.tiles[2].value, '6');
  // The patch is in the input before any press.
  assert.deepEqual(JSON.parse(s.input), { ev: 4, stamina: 50 });
  assert.equal(s.all, 'Keep the old figures');
  const said = await p.evaluate(() => [...document.querySelectorAll('.air-sheet > .air-line')].map((n) => ({ tag: n.tagName, text: n.textContent })));
  assert.deepEqual(said, [{ tag: 'SPAN', text: 'No published formula covers a swarm’s Stamina.' }]);
  assert.equal(await p.locator('.air-sheet button').count(), 4);

  await p.click('button[data-id="ev"]');
  s = await state(p);
  assert.equal(s.tiles[0].value, 'blank');
  assert.equal(s.tiles[0].small, 'Left blank. Formula gives 4');
  assert.deepEqual(JSON.parse(s.input), { stamina: 50 });

  await p.click('.air-stats-all');
  s = await state(p);
  assert.equal(s.input, '');
  assert.equal(s.all, 'Use the formula for all 3');
});
