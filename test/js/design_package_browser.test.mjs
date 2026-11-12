// design_package_browser.test.mjs — the "From <system>" group and the Suggest
// fold in the saved-designs library, in a real Chromium against an in-page fake
// of the /designs routes: the read-only strip, Save as my design, and the
// export by keyboard and at phone width.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { skip } from './_browser_env.mjs';
import { launch, openArranged } from './_design_library_harness.mjs';

const theme = (name) => ({ version: 1, name, skeleton: { shape: 'alcove', opening: 'window-left', display: 'pegboard', counter: 'straight' },
  theme: { wall: 'stone', wood: '#6b4527', cloth: '#5a2024', accent: '#ff9a50', trim: '#c89a4a', light: 'forge', warmth: 0.8, outside: 'dusk' }, props: ['anvil'] });
const mine = (id, name) => ({ id, kind: 'room-theme', name, note: '', systemId: 'drawsteel', makerName: 'Alice', data: theme(name), origin: 'made', shared: false });
const pkgDesign = (id, name, extra) => Object.assign({ id, kind: 'room-theme', name, note: 'Warm oak and iron', makerName: 'Pack Maker', data: theme(name) }, extra);
const PKG = { systemId: 'drawsteel', systemName: 'Draw Steel', you: 'Alice', designs: [pkgDesign('pkg-1', 'Ashgate Forge'), pkgDesign('pkg-2', 'Iron Hall', { makerName: '' })] };

async function arranged(t, opts) {
  const browser = await launch();
  t.after(() => browser.close());
  const r = await openArranged(browser, opts);
  t.after(() => assert.deepEqual(r.errors, []));
  return r.page;
}
const calls = (p) => p.evaluate(() => window.__srv.calls);

test('the From group lists package designs and a pick shows the read-only strip', { skip }, async (t) => {
  const p = await arranged(t, { seed: { pkg: PKG, mine: [mine('m1', 'Cinderhold Forge')] } });
  await p.waitForSelector('[data-dl-pick="p:pkg-1"]');
  assert.match(await p.locator('.dl').innerText(), /from draw steel/i);
  await p.click('[data-dl-pick="p:pkg-1"]');
  const strip = await p.locator('.dl-detail').innerText();
  assert.match(strip, /Part of the game system package, so it can’t be renamed or deleted here\. Change it and use Save as my design to keep your own version\./);
  assert.equal(await p.locator('[data-dl-do="rename"], [data-dl-do="delete"]').count(), 0, 'no rename or delete on a package design');
  assert.equal(await p.getAttribute('[data-dl-pick="p:pkg-1"]', 'aria-pressed'), 'true');
});

test('Save as my design makes an independent library design', { skip }, async (t) => {
  const p = await arranged(t, { seed: { pkg: PKG } });
  await p.click('[data-dl-pick="p:pkg-2"]');
  await p.click('[data-dl-do="save-pkg"]');
  await p.waitForFunction(() => document.querySelector('[aria-label="My designs"] [data-dl-pick]'));
  assert.ok((await calls(p)).includes('POST /designs/package/pkg-2/copy'));
  assert.match(await p.locator('.dl-detail').innerText(), /Copied from the game system package/);
  assert.ok(await p.locator('.dl-detail [data-dl-do="rename"]').count(), 'the copy can be renamed');
  assert.equal(await p.locator('[data-dl-pick="p:pkg-2"]').count(), 1, 'the package group is unchanged');
});

test('no package designs means no From group, and a failed package read leaves the library working', { skip }, async (t) => {
  const p = await arranged(t, { seed: { pkgErr: 'boom', mine: [mine('m1', 'Cinderhold Forge')] } });
  await p.waitForSelector('[data-dl-pick="m1"]');
  assert.doesNotMatch(await p.locator('.dl').innerText(), /shipped with the game system/);
  await p.click('[data-dl-pick="m1"]');
  assert.ok(await p.locator('[data-dl-do="rename"]').count());
});

test('Suggest: choose, note, credit, export, then the sent-it note', { skip }, async (t) => {
  const p = await arranged(t, { seed: { pkg: PKG, mine: [mine('m1', 'Cinderhold Forge'), mine('m2', 'Brass Hall')] } });
  await p.waitForSelector('[data-dl-do="suggest"]');
  assert.equal(await p.getAttribute('[data-dl-do="suggest"]', 'aria-expanded'), 'false');
  await p.click('[data-dl-do="suggest"]');
  await p.waitForSelector('#dsg-design');
  assert.equal(await p.evaluate(() => document.activeElement.id), 'dsg-design');
  assert.equal(await p.isChecked('#dsg-credit'), true, 'credit is on by default');
  assert.equal(await p.textContent('[data-dsg-you]'), 'Alice');
  assert.match(await p.locator('.dsg-list').innerText(), /No goods, prices or pictures/);
  await p.selectOption('#dsg-design', 'm2');
  await p.fill('#dsg-note', 'Suits the <b>Director</b>');
  const download = p.waitForEvent('download');
  await p.click('[data-dsg="export"]');
  const dl = await download;
  assert.equal(dl.suggestedFilename(), 'drawsteel-design-brass-hall.json');
  const body = JSON.parse(await (await import('node:fs/promises')).readFile(await dl.path(), 'utf8'));
  assert.equal(body.format, 'chronicle-design-suggestion:v1');
  assert.equal(body.messageForMaintainer, 'Suits the <b>Director</b>', 'typed text goes through as text');
  assert.ok((await calls(p)).includes('POST /designs/m2/suggestion {"note":"Suits the <b>Director</b>","credit":true}'));
  await p.waitForFunction(() => /Exported drawsteel-design-brass-hall\.json/.test(document.querySelector('[data-dsg-done]').textContent));
  assert.match(await p.textContent('[data-dsg-done]'), /Send it to the Draw Steel package maintainer/);
  assert.equal(await p.locator('#dsg-note b').count(), 0, 'the note was never parsed as markup');
});

test('Suggest without credit sends credit false, and an empty library says to save one first', { skip }, async (t) => {
  const p = await arranged(t, { seed: { pkg: PKG } });
  await p.click('[data-dl-do="suggest"]');
  assert.match(await p.locator('#dl-suggest').innerText(), /Save a design of your own first/);
  await p.click('[data-dsg="close"]');
  assert.equal(await p.locator('#dsg-design').count(), 0);

  const q = await arranged(t, { seed: { pkg: PKG, mine: [mine('m1', 'Cinderhold Forge')] } });
  await q.click('[data-dl-do="suggest"]');
  await q.uncheck('#dsg-credit');
  assert.match(await q.locator('.dsg-list').innerText(), /Your name is not included/);
  const download = q.waitForEvent('download');
  await q.click('[data-dsg="export"]');
  await download;
  assert.ok((await calls(q)).includes('POST /designs/m1/suggestion {"note":"","credit":false}'));
});

test('a refused export shows the server words and offers the file again', { skip }, async (t) => {
  const p = await arranged(t, { seed: { pkg: PKG, suggestErr: 'note must be at most 240 characters', mine: [mine('m1', 'Cinderhold Forge')] } });
  await p.click('[data-dl-do="suggest"]');
  await p.click('[data-dsg="export"]');
  await p.waitForFunction(() => document.querySelector('[data-dsg-err]').textContent.length > 0);
  assert.match(await p.textContent('[data-dsg-err]'), /at most 240/);
  assert.equal(await p.isEnabled('[data-dsg="export"]'), true);
});

test('at 400px nothing scrolls sideways with the group and the fold open', { skip }, async (t) => {
  const p = await arranged(t, { width: 400, height: 800, seed: { pkg: PKG, mine: [mine('m1', 'Cinderhold Forge')] } });
  await p.click('[data-dl-pick="p:pkg-1"]');
  await p.click('[data-dl-do="suggest"]');
  await p.waitForSelector('#dsg-design');
  assert.equal(await p.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth), true);
});
