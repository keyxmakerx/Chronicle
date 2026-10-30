// crafting_upgrades_browser.test.mjs — a bench's upgrade ladders in a real
// Chromium: internal/plugins/crafting/static/js/crafting_upgrades.js. A
// piece's ladder marks the levels the party has; a level's change is words
// or a rule picked from the time of day, the maker's skills and the effects
// the roll carries out, and reads back as a sentence; "Give it to them now"
// moves the level they have; levels and pieces stop at the server's limits;
// every change reaches onChange in the shape upgrades.go reads.
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
const plugin = join(root, 'internal', 'plugins', 'crafting', 'static');

const BENCH = {
  id: 'b1', name: 'Apothecary bench', version: 3,
  upgrades: { pieces: [
    { name: 'Mortar', have: 0, levels: [{ name: 'Granite mortar', how: 'start' }, { name: 'Saltglass mortar', changes: 'Grinding takes one fewer turn.', how: 'recipe' },
      { name: 'Moonstone mortar', how: 'given', rule: { when: [{ kind: 'moon', moon: 'Lacrimosa', phase: 'full' }], then: { kind: 'fine' }, chance: '1 in 6', players: 'whenItHappens' } }] },
  ] },
};

function html() {
  return `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/app.css"><link rel="stylesheet" href="/plugin/css/crafting.css">
<style>body{margin:0;padding:16px}</style></head><body><div class="cra" data-widget="crafting"><section class="cra-upmount"></section></div>
<script src="/plugin/js/crafting_lib.js"></script><script src="/plugin/js/crafting_upgrades.js"></script>
<script>window.saves = 0; window.toasts = []; window.bench = ${JSON.stringify(BENCH)};
CraftingUpgrades.mount(document.querySelector('.cra-upmount'), window.bench, {skills: ['Alchemy', 'Herbalism'], toast: function (m) { toasts.push(m); }, onChange: function () { saves++; }});</script>
</body></html>`;
}

async function open(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const page = await browser.newPage({ viewport: { width: 1100, height: 900 } });
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: html() });
    let file = '';
    if (u.pathname.startsWith('/plugin/')) file = join(plugin, u.pathname.slice(8));
    else if (u.pathname.startsWith('/static/')) file = join(root, u.pathname);
    try {
      return route.fulfill({ contentType: file.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    } catch { return route.fulfill({ status: 404, body: '' }); }
  });
  await page.goto('http://lab.test/', { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('.cra-ladder');
  return { page, errors };
}
const click = (page, sel) => page.$eval(sel, (b) => b.click());
const bench = (page) => page.evaluate(() => JSON.parse(JSON.stringify(window.bench)));

test('the ladder shows what they have and opens on the next level', { skip }, async (t) => {
  const { page, errors } = await open(t);
  assert.deepEqual(await page.$$eval('.cra-lstep:not(.cra-ladd)', (els) => els.map((e) => [e.classList.contains('got'), e.getAttribute('aria-pressed')])),
    [[true, 'false'], [false, 'true'], [false, 'false']]);
  assert.match(await page.textContent('.cra-uhd small'), /level 1 of 3/);
  // An AI-written condition the pickers lack is kept as written.
  await click(page, '[data-u="pick"][data-l="2"]');
  assert.match(await page.$eval('[data-u="when"]', (s) => s.selectedOptions[0].textContent), /Lacrimosa is full \(as written\)/);
  assert.match(await page.textContent('.cra-usent'), /When Lacrimosa is full, there’s a 1 in 6 chance: comes out fine\./);
  assert.deepEqual(errors, []);
});

test('a level becomes a rule the roll reads, and giving it moves the ladder', { skip }, async (t) => {
  const { page, errors } = await open(t);
  await click(page, '[data-u="rule"]');
  await page.selectOption('[data-u="when"]', 'maker:Alchemy');
  await page.selectOption('[data-u="then"]', 'points');
  await click(page, '[data-u="more"]');
  await page.selectOption('[data-u="chance"]', '1 in 4');
  assert.match(await page.textContent('.cra-usent'), /When the maker knows Alchemy, there’s a 1 in 4 chance: \+3 project points\./);
  await click(page, '[data-u="give"]');
  const b = await bench(page);
  assert.deepEqual(b.upgrades.pieces[0].levels[1].rule, { when: [{ kind: 'maker', skill: 'Alchemy' }], then: { kind: 'points', amount: 3 }, chance: '1 in 4', players: 'shown' });
  assert.equal(b.upgrades.pieces[0].have, 1);
  assert.deepEqual(await page.evaluate(() => window.toasts), ['They have the Saltglass mortar now.']);
  assert.ok(await page.evaluate(() => window.saves) >= 6);
  // Back to words: the rule goes, the words stay for the table.
  await click(page, '[data-u="words"]');
  assert.equal((await bench(page)).upgrades.pieces[0].levels[1].rule, null);
  assert.equal(await page.inputValue('[data-u="changes"]'), 'Grinding takes one fewer turn.');
  assert.deepEqual(errors, []);
});

test('typing renames in place, and pieces and levels stop at the limits', { skip }, async (t) => {
  const { page, errors } = await open(t);
  await page.fill('[data-u="lname"]', 'Glass mortar');
  assert.equal(await page.textContent('[data-u="pick"][data-l="1"] b'), 'Glass mortar');
  await page.fill('[data-u="lname"]', '');
  assert.equal((await bench(page)).upgrades.pieces[0].levels[1].name, 'Glass mortar', 'a blank name is not saved');
  await click(page, '[data-u="addlevel"]');
  await click(page, '[data-u="addlevel"]');
  assert.equal(await page.$$eval('[data-u="addlevel"]', (els) => els.length), 0, 'five levels at most');
  for (let i = 0; i < 5; i++) await click(page, '[data-u="addpiece"]');
  assert.equal((await bench(page)).upgrades.pieces.length, 6);
  assert.equal(await page.$$eval('[data-u="addpiece"]', (els) => els.length), 0, 'six pieces at most');
  // Removing a level they have steps the ladder back.
  await click(page, '[data-u="pick"][data-p="0"][data-l="1"]');
  await click(page, '[data-u="give"][data-p="0"]');
  await click(page, '[data-u="dellevel"][data-p="0"]');
  const p = (await bench(page)).upgrades.pieces[0];
  assert.equal(p.levels.length, 4);
  assert.equal(p.have, 0);
  await click(page, '[data-u="delpiece"][data-p="0"]');
  assert.equal((await bench(page)).upgrades.pieces.length, 5);
  assert.deepEqual(errors, []);
});
