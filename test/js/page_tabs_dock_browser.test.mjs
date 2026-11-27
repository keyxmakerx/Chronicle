// page_tabs_dock_browser.test.mjs — the page tabs docking into a game-system
// sheet's tab row (docs/system-package-rendering.md, "The sheet's dock").
//
// Runs widgets/page_tabs.js in a real Chromium beside a stand-in sheet that
// keeps the sheet's side of the contract: the three slots, data-sheet-dock
// and the dock / undock / tab events. The Draw Steel package pins the real
// sheet's side in its own tests.
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

function tab(key, label, i) {
  const sel = i === 0 ? 'aria-selected="true"' : 'aria-selected="false" tabindex="-1"';
  return `<button type="button" class="pt-tab" role="tab" id="pt-tab-${key}" aria-controls="pt-pane-${key}" ${sel} data-pt-tab="${key}" data-pt-count=".row" data-pt-act=".add">${label} <span class="pt-n" data-pt-n></span></button>`;
}
function pane(key, i) {
  return `<div class="pt-pane" role="tabpanel" tabindex="0" id="pt-pane-${key}" data-pt-pane="${key}"${i ? ' hidden' : ''}><div class="row">${key} row</div><button class="add">Add ${key}</button></div>`;
}
const KEYS = [['notes', 'Notes'], ['items', 'Items'], ['files', 'Files'], ['links', 'Links']];
const STRIP = `<section class="pt" data-widget="page-tabs"><div class="pt-bar"><div class="pt-tabs" role="tablist">${KEYS.map(([k, l], i) => tab(k, l, i)).join('')}</div></div>
${KEYS.map(([k], i) => pane(k, i)).join('')}<button type="button" class="pt-act" data-pt-press hidden><span data-pt-press-label></span></button></section>`;

// The sheet's side: slots in its tab row, pane area and Equipment pane.
const SHEET = `<div id="mount"><div data-sheet><div class="sh-dtabs" role="tablist">
<button role="tab" data-cs-dtab="abilities" aria-selected="true">Abilities</button><button role="tab" data-cs-dtab="equipment" aria-selected="false">Equipment</button>
<span data-sheet-dock-tabs></span></div>
<div data-cs-dmain data-dtab="abilities"><div data-dpane="equipment"><div data-sheet-dock-piece="items"><h3>Pack and money</h3></div></div><div data-sheet-dock-panes></div></div></div></div>`;

const SHEET_JS = `window.sheet = {
  opened: 0,
  root: function () { return document.querySelector('[data-sheet]'); },
  dock: function () { var r = this.root(); r.setAttribute('data-sheet-dock', 'open'); r.dispatchEvent(new CustomEvent('chronicle:sheet-dock', { bubbles: true })); },
  undock: function () { var r = this.root(); r.removeAttribute('data-sheet-dock'); r.dispatchEvent(new CustomEvent('chronicle:sheet-undock', { bubbles: true })); },
  tab: function (id) { document.querySelector('[data-cs-dmain]').setAttribute('data-dtab', id); this.root().dispatchEvent(new CustomEvent('chronicle:sheet-tab', { bubbles: true, detail: { tab: id } })); },
  redraw: function () { this.undock(); var m = document.getElementById('mount'); m.innerHTML = m.innerHTML; this.dock(); }
};
document.addEventListener('chronicle:sheet-dock-open', function (e) {
  if (e.target.closest('[data-sheet-dock-tabs]')) { window.sheet.opened++; document.querySelector('[data-cs-dmain]').setAttribute('data-dtab', 'dock'); }
});`;

async function page(t, { dockFirst = true } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent('<!doctype html><html><head><style>[hidden]{display:none}</style></head><body><div id="main-content">' + SHEET + STRIP + '</div></body></html>');
  await p.addScriptTag({ content: SHEET_JS });
  if (dockFirst) await p.evaluate(() => window.sheet.dock());
  await p.addScriptTag({ content: 'window.Chronicle = { register: function (n, impl) { window.impl = impl; impl.init(document.querySelector("[data-widget=" + n + "]")); } };' });
  await p.addScriptTag({ content: readFileSync(join(root, 'static/js/widgets/page_tabs.js'), 'utf8') });
  await frames(p);
  return p;
}
const frames = (p) => p.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
const state = (p) => p.evaluate(() => ({
  row: [...document.querySelectorAll('[data-sheet-dock-tabs] > [role="tab"]')].map((x) => x.dataset.ptTab),
  strip: [...document.querySelectorAll('.pt-tabs > [role="tab"]')].map((x) => x.dataset.ptTab),
  stripHidden: document.querySelector('section.pt').hidden,
  selected: [...document.querySelectorAll('[data-pt-tab]')].filter((x) => x.getAttribute('aria-selected') === 'true').map((x) => x.dataset.ptTab),
  open: [...document.querySelectorAll('[data-pt-pane]')].filter((x) => !x.hidden && !x.closest('[data-sheet-dock-piece]')).map((x) => x.dataset.ptPane),
  piece: !!document.querySelector('[data-sheet-dock-piece][data-docked] > [data-pt-pane="items"]:not([hidden])'),
  dtab: document.querySelector('[data-cs-dmain]').getAttribute('data-dtab'),
  press: document.querySelector('[data-pt-press]').hidden ? null : document.querySelector('[data-pt-press-label]').textContent,
}));

test('an open dock takes the tabs into the sheet\'s row and Items into its slot', { skip }, async (t) => {
  const p = await page(t);
  const s = await state(p);
  assert.deepEqual(s.row, ['notes', 'files', 'links']);
  assert.deepEqual(s.strip, ['items'], 'Items has no tab of its own while docked');
  assert.equal(s.stripHidden, true);
  assert.equal(s.piece, true, 'the Items pane shows in the Equipment slot');
  assert.deepEqual(s.selected, [], 'the sheet\'s own tab stays open');
  assert.deepEqual(s.open, []);
  assert.equal(s.dtab, 'abilities');
  assert.equal(await p.getAttribute('[data-pt-pane="items"]', 'role'), null, 'no tab panel inside the sheet\'s pane');
});

test('a docked tab opens its pane in the sheet and the sheet\'s tab lets it go', { skip }, async (t) => {
  const p = await page(t);
  await p.click('#pt-tab-files');
  let s = await state(p);
  assert.deepEqual(s.selected, ['files']);
  assert.deepEqual(s.open, ['files']);
  assert.equal(s.dtab, 'dock');
  assert.equal(s.press, 'Add files');
  assert.equal(await p.evaluate(() => window.sheet.opened), 1);
  assert.equal(await p.getAttribute('#pt-tab-files', 'tabindex'), '0');
  await p.evaluate(() => window.sheet.tab('equipment'));
  s = await state(p);
  assert.deepEqual(s.selected, []);
  assert.deepEqual(s.open, []);
  assert.equal(s.piece, true, 'Items stays in the Equipment slot');
  assert.equal(await p.getAttribute('#pt-tab-files', 'tabindex'), '-1');
});

test('docked, arrow keys are the sheet\'s: the panel moves nothing itself', { skip }, async (t) => {
  const p = await page(t);
  await p.click('#pt-tab-notes');
  await p.keyboard.press('ArrowRight');
  assert.deepEqual((await state(p)).selected, ['notes']);
});

test('undocking puts every node back as it was, and docking again reopens the tab', { skip }, async (t) => {
  const p = await page(t);
  await p.click('#pt-tab-links');
  await p.evaluate(() => window.sheet.undock());
  let s = await state(p);
  assert.deepEqual(s.strip, ['notes', 'items', 'files', 'links']);
  assert.deepEqual(s.row, []);
  assert.equal(s.stripHidden, false);
  assert.deepEqual(s.selected, ['links'], 'the strip opens on the tab the viewer had');
  assert.deepEqual(s.open, ['links']);
  assert.equal(await p.getAttribute('[data-pt-pane="items"]', 'role'), 'tabpanel');
  assert.equal(await p.evaluate(() => document.querySelector('section.pt').lastElementChild.hasAttribute('data-pt-press')), true);
  await p.evaluate(() => window.sheet.dock());
  s = await state(p);
  assert.deepEqual(s.selected, ['links']);
  assert.equal(s.dtab, 'dock');
});

test('a sheet that redraws keeps the open tab, and its slots come back filled', { skip }, async (t) => {
  const p = await page(t);
  await p.click('#pt-tab-notes');
  await p.evaluate(() => window.sheet.redraw());
  const s = await state(p);
  assert.deepEqual(s.row, ['notes', 'files', 'links']);
  assert.equal(s.piece, true);
  assert.deepEqual(s.selected, ['notes']);
  assert.equal(await p.evaluate(() => document.querySelectorAll('[data-pt-pane="notes"]').length), 1);
});

test('a sheet that offers its dock after the panel mounted still gets it', { skip }, async (t) => {
  const p = await page(t, { dockFirst: false });
  assert.equal((await state(p)).stripHidden, false);
  await p.evaluate(() => window.sheet.dock());
  assert.deepEqual((await state(p)).row, ['notes', 'files', 'links']);
});

test('a live refresh finds the panel whole, then it docks again', { skip }, async (t) => {
  const p = await page(t);
  const during = await p.evaluate(() => {
    var m = document.getElementById('main-content'), seen = null;
    m.dispatchEvent(new CustomEvent('chronicle:page-refreshing', { bubbles: true }));
    seen = document.querySelectorAll('section.pt [data-pt-tab]').length + document.querySelectorAll('section.pt [data-pt-pane]').length;
    window.sheet.redraw();
    var stillHome = document.querySelectorAll('[data-sheet-dock-tabs] > *').length;
    m.dispatchEvent(new CustomEvent('chronicle:page-refreshed', { bubbles: true }));
    return [seen, stillHome];
  });
  assert.deepEqual(during, [8, 0], 'every tab and pane inside the panel while the refresh compares parts');
  assert.deepEqual((await state(p)).row, ['notes', 'files', 'links']);
});

test('destroy takes everything home, so the page\'s swap removes it with the panel', { skip }, async (t) => {
  const p = await page(t);
  await p.evaluate(() => window.impl.destroy(document.querySelector('section.pt')));
  const s = await state(p);
  assert.deepEqual(s.strip, ['notes', 'items', 'files', 'links']);
  assert.equal(await p.evaluate(() => document.querySelectorAll('section.pt [data-pt-pane]').length), 4);
  await p.evaluate(() => window.sheet.dock());
  assert.deepEqual((await state(p)).row, [], 'a destroyed panel no longer docks');
});

test('page_live says it is refreshing around its update in place', () => {
  const js = readFileSync(join(root, 'static/js/widgets/page_live.js'), 'utf8');
  const body = js.slice(js.indexOf('function updateInPlace'), js.indexOf('Chronicle.served = html;'));
  assert.match(body, /chronicle:page-refreshing[\s\S]*updatePart[\s\S]*finally[\s\S]*chronicle:page-refreshed/);
});
