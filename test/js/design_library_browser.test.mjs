// design_library_browser.test.mjs — the saved-designs library in the shop room
// builder (static/js/widgets/design_library.js inside shop_room_builder.js), in
// a real Chromium against an in-page fake of the /designs routes.
//
// Starting from has three groups; a saved design applies to the room; the name
// box saves the current theme; a pasted code previews with the system note and
// what the campaign lacks before anything is added; the Share sheet has its
// three tabs and works from the keyboard, at phone width too.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { skip } from './_browser_env.mjs';
import { launch, openArranged } from './_design_library_harness.mjs';

const theme = (name) => ({ version: 1, name, skeleton: { shape: 'alcove', opening: 'window-left', display: 'pegboard', counter: 'straight' },
  theme: { wall: 'stone', wood: '#6b4527', cloth: '#5a2024', accent: '#ff9a50', trim: '#c89a4a', light: 'forge', warmth: 0.8, outside: 'dusk' }, props: ['anvil'] });
const mine = (id, name, extra) => Object.assign({ id, kind: 'room-theme', name, note: '', systemId: 'drawsteel', makerName: 'Alice', data: theme(name), origin: 'made', shared: false }, extra);
const code = (o) => 'chronicle-design:v1:' + Buffer.from(JSON.stringify(Object.assign({ kind: 'room-theme', system_id: 'dnd5e', name: 'Moonlit Ward', maker_name: 'Tobias Fenwick-Okoye', data: theme('Moonlit Ward') }, o))).toString('base64url');

async function arranged(t, opts) {
  const browser = await launch();
  t.after(() => browser.close());
  const r = await openArranged(browser, opts);
  t.after(() => assert.deepEqual(r.errors, []));
  return r.page;
}
const calls = (p) => p.evaluate(() => window.__srv.calls);
const groups = (p) => p.evaluate(() => Array.from(document.querySelectorAll('.shr-cb [role="group"]'), (g) => g.getAttribute('aria-label') + ': ' + Array.from(g.querySelectorAll('button'), (b) => b.textContent.trim()).join('|')));

test('Starting from has Built in, My designs and Shared with me', { skip }, async (t) => {
  const p = await arranged(t, { seed: { mine: [mine('m1', 'Cinderhold Forge')], shared: [{ id: 's1', kind: 'room-theme', name: 'Ember Sigil', makerName: 'Marrow Quill' }] } });
  await p.waitForSelector('[data-dl-pick="m1"]');
  const g = await groups(p);
  assert.ok(g.some((x) => /^Built in: Plain counter\|The Smithy/.test(x)), g.join('\n'));
  assert.ok(g.includes('My designs: Cinderhold Forge'));
  assert.ok(g.includes('Shared with me: Ember Sigil from Marrow Quill'));
});

test('picking a saved design applies it to the room and offers Rename, Share, Delete', { skip }, async (t) => {
  const p = await arranged(t, { seed: { mine: [mine('m1', 'Cinderhold Forge')] } });
  await p.click('[data-dl-pick="m1"]');
  await p.waitForFunction(() => document.querySelector('.shr-cb [data-cb-say]').textContent.indexOf('Cinderhold Forge') >= 0);
  assert.equal(await p.evaluate(() => document.querySelector('[data-cb-path="theme.wall"]').value), 'stone');
  assert.equal(await p.evaluate(() => document.querySelector('[data-cb-path="skeleton.display"]').value), 'pegboard');
  const acts = await p.evaluate(() => Array.from(document.querySelectorAll('.dl-detail .dl-acts button'), (b) => b.textContent));
  assert.deepEqual(acts, ['Rename', 'Share…', 'Delete'], 'Stop sharing shows only for a shared design');
  assert.equal(await p.evaluate(() => document.querySelector('[data-dl-pick="m1"]').getAttribute('aria-pressed')), 'true');
});

test('Save as my design sends the current theme under the name, and an empty name is refused inline', { skip }, async (t) => {
  const p = await arranged(t, {});
  await p.click('[data-dl-do="save"]');
  assert.match(await p.textContent('.dl-err'), /Give the design a name/);
  assert.ok(!(await calls(p)).some((c) => c.startsWith('POST')), 'nothing was sent');
  await p.fill('#dl-save', 'Brass Hall');
  await p.press('#dl-save', 'Enter');
  await p.waitForSelector('[data-dl-pick]');
  const post = (await calls(p)).find((c) => c.startsWith('POST /designs '));
  const body = JSON.parse(post.slice('POST /designs '.length));
  assert.equal(body.kind, 'room-theme');
  assert.equal(body.name, 'Brass Hall');
  assert.equal(body.data.name, 'Brass Hall');
  assert.ok(body.data.skeleton && body.data.theme && Array.isArray(body.data.props));
  assert.equal(await p.textContent('.dl-detail b'), 'Brass Hall');
});

test('rename and delete work in place, and delete asks first', { skip }, async (t) => {
  const p = await arranged(t, { seed: { mine: [mine('m1', 'Cinderhold Forge')] } });
  await p.click('[data-dl-pick="m1"]');
  await p.click('[data-dl-do="rename"]');
  await p.fill('#dl-rename', 'Cinderhold Smithy'); await p.press('#dl-rename', 'Enter');
  await p.waitForFunction(() => document.querySelector('.dl-detail b') && document.querySelector('.dl-detail b').textContent === 'Cinderhold Smithy');
  assert.ok((await calls(p)).includes('PATCH /designs/m1 {"name":"Cinderhold Smithy"}'), 'a rename sends only the name');
  await p.click('[data-dl-do="delete"]');
  assert.match(await p.textContent('.dl-detail'), /Copies others already took stay/);
  assert.ok(!(await calls(p)).some((c) => c.startsWith('DELETE')), 'asking is not deleting');
  await p.click('[data-dl-do="delete-ok"]');
  await p.waitForFunction(() => !document.querySelector('[data-dl-pick="m1"]'));
});

test('a pasted code is checked and previewed before it is added', { skip }, async (t) => {
  const p = await arranged(t, { seed: { takeData: theme('Moonlit Ward'),
    preview: { name: 'Moonlit Ward', kind: 'room-theme', makerName: 'Tobias Fenwick-Okoye', systemId: 'dnd5e', sameSystem: false, lacks: ['This campaign has no ‘moonsilver’ material; it will show as plain silver until you add it.'] } } });
  await p.click('[data-dl-do="preview"]');
  assert.match(await p.textContent('.dl-err'), /Paste a design code first/);
  await p.fill('#dl-paste', 'hello'); await p.click('[data-dl-do="preview"]');
  assert.match(await p.textContent('.dl-err'), /not a Chronicle design code/);
  await p.fill('#dl-paste', 'chronicle-design:v1:@@@'); await p.click('[data-dl-do="preview"]');
  assert.match(await p.textContent('.dl-err'), /damaged/);
  assert.ok(!(await calls(p)).some((c) => c.indexOf('code/preview') >= 0), 'a bad code never reaches the server');

  await p.fill('#dl-paste', code().replace(/^(.{30})/, '$1\n'));
  await p.click('[data-dl-do="preview"]');
  await p.waitForSelector('.dl-card');
  const card = await p.textContent('.dl-card');
  assert.match(card, /Moonlit Ward/); assert.match(card, /by Tobias Fenwick-Okoye/); assert.match(card, /Room theme/); assert.match(card, /Made for D&D 5e/);
  const notes = await p.evaluate(() => Array.from(document.querySelectorAll('.dl-card .dl-note'), (n) => n.textContent));
  assert.equal(notes.length, 2);
  assert.match(notes[0], /Made for D&D 5e\. This campaign uses Draw Steel\./);
  assert.match(notes[1], /moonsilver/);
  assert.equal((await calls(p)).filter((c) => c.indexOf('code/take') >= 0).length, 0, 'previewing adds nothing');

  await p.click('.dl-card [data-dl-do="take"]');
  await p.waitForSelector('[data-dl-pick]');
  assert.equal(await p.$('.dl-card'), null);
  assert.match(await p.textContent('.dl-detail'), /Added from a code by Tobias/);
  // Server refusals are shown inline, in its own words.
  await p.evaluate(() => { window.__srv.previewErr = 'that design code is too large'; });
  await p.fill('#dl-paste', code()); await p.click('[data-dl-do="preview"]');
  await p.waitForFunction(() => /too large/.test(document.querySelector('.dl-err').textContent));
});

test('the Share sheet: code, link and named owners', { skip }, async (t) => {
  const hits = [{ userId: 'u1', displayName: 'Marrow Quill', systems: ['drawsteel', 'dnd5e'] }, { userId: 'u2', displayName: 'Marrow Vale', systems: ['dnd5e'] }];
  const p = await arranged(t, { seed: { mine: [mine('m1', 'Cinderhold Forge')], hits } });
  await p.click('[data-dl-pick="m1"]');
  await p.click('[data-dl-do="share"]');
  await p.waitForSelector('.dl-sheet textarea');
  assert.equal(await p.getAttribute('.dl-sheet', 'role'), 'dialog');
  assert.equal(await p.getAttribute('.dl-sheet', 'aria-modal'), 'true');
  assert.match(await p.textContent('.dl-sh h3'), /Share “Cinderhold Forge”/);
  assert.equal(await p.inputValue('#dl-code'), 'chronicle-design:v1:eyJraW5kIjoicm9vbS10aGVtZSJ9');
  assert.equal(await p.evaluate(() => getComputedStyle(document.querySelector('.dl-sheet')).backgroundColor !== 'rgba(0, 0, 0, 0)'), true, 'the sheet is opaque');

  // Link: off by default, on gives a full URL, off again takes it away.
  await p.click('[data-tab="link"]');
  assert.equal(await p.getAttribute('[role="switch"]', 'aria-checked'), 'false');
  await p.click('[role="switch"]');
  await p.waitForSelector('#dl-url');
  assert.match(await p.inputValue('#dl-url'), /^about:\/\/|^https?:\/\/.*\/designs\/link\/tok\d+$|\/designs\/link\/tok\d+$/);
  await p.click('[role="switch"]');
  await p.waitForFunction(() => !document.querySelector('#dl-url'));

  // Named owners: search, add (the one on another system cannot be added), remove.
  await p.click('[data-tab="owners"]');
  await p.fill('#dl-q', 'Mar');
  await p.waitForSelector('.dl-hits li');
  const rows = await p.evaluate(() => Array.from(document.querySelectorAll('.dl-hits li'), (li) => li.querySelector('b').textContent + ' ' + li.querySelector('button').disabled));
  assert.deepEqual(rows, ['Marrow Quill false', 'Marrow Vale true']);
  assert.match(await p.textContent('.dl-hits li:nth-child(2)'), /No campaign on Draw Steel/);
  assert.match(await p.textContent('.dl-hits li:nth-child(1)'), /Draw Steel · D&D 5e/);
  await p.click('[aria-label="Add Marrow Quill"]');
  await p.waitForSelector('.dl-chipx');
  await p.click('.dl-chipx button');
  await p.waitForFunction(() => !document.querySelector('.dl-chipx'));

  // Stop sharing, then Escape closes and focus returns to Share….
  await p.click('[data-dl-sh="stop"]');
  await p.waitForFunction(() => /Sharing stopped/.test(document.querySelector('.dl-sb [role="status"]').textContent));
  await p.keyboard.press('Escape');
  await p.waitForFunction(() => !document.querySelector('.dl-sheet'));
  assert.equal(await p.evaluate(() => document.activeElement.getAttribute('data-dl-do')), 'share');
});

test('the sheet keeps focus inside and its tabs answer the arrow keys', { skip }, async (t) => {
  const p = await arranged(t, { seed: { mine: [mine('m1', 'Cinderhold Forge')] } });
  await p.click('[data-dl-pick="m1"]'); await p.click('[data-dl-do="share"]'); await p.waitForSelector('.dl-sheet textarea');
  await p.focus('[data-tab="code"]'); await p.keyboard.press('ArrowRight');
  assert.equal(await p.getAttribute('[data-tab="link"]', 'aria-selected'), 'true');
  assert.equal(await p.evaluate(() => document.activeElement.getAttribute('data-tab')), 'link');
  for (let i = 0; i < 12; i++) await p.keyboard.press('Tab');
  assert.equal(await p.evaluate(() => !!document.activeElement.closest('.dl-sheet')), true, 'Tab never leaves the sheet');
});

test('at phone width nothing overflows and the sheet fits the panel', { skip }, async (t) => {
  const p = await arranged(t, { width: 400, height: 760, seed: { mine: [mine('m1', 'Cinderhold Forge', { shared: true })], shared: [{ id: 's1', kind: 'room-theme', name: 'Ember Sigil', makerName: 'Marrow Quill' }] } });
  await p.click('[data-dl-pick="m1"]');
  const fits = () => p.evaluate(() => { const b = document.querySelector('.shr-panel'); return b.scrollWidth <= b.clientWidth + 1; });
  assert.equal(await fits(), true);
  await p.click('[data-dl-do="share"]'); await p.waitForSelector('.dl-sheet textarea');
  const r = await p.evaluate(() => { const s = document.querySelector('.dl-sheet').getBoundingClientRect(), pn = document.querySelector('.shr-panel').getBoundingClientRect(); return [s.left >= pn.left - 1, s.right <= pn.right + 1, s.bottom <= pn.bottom + 1, document.documentElement.scrollWidth <= innerWidth]; });
  assert.deepEqual(r, [true, true, true, true]);
});

test('picking something shared with me previews it, and Add copies it into My designs', { skip }, async (t) => {
  const p = await arranged(t, { seed: { takeData: theme('Ember Sigil'), shared: [{ id: 's1', kind: 'room-theme', name: 'Ember Sigil', makerName: 'Marrow Quill' }],
    preview: { name: 'Ember Sigil', kind: 'room-theme', makerName: 'Marrow Quill', systemId: 'drawsteel', sameSystem: true, lacks: [] } } });
  await p.click('[data-dl-pick="s:s1"]');
  await p.waitForSelector('.dl-card');
  assert.equal(await p.$('.dl-card .dl-note'), null, 'same system and nothing lacking: no warnings');
  assert.ok((await calls(p)).includes('GET /designs/shared/s1/preview'));
  await p.click('.dl-card [data-dl-do="take"]');
  await p.waitForSelector('.dl-detail');
  assert.match(await p.textContent('.dl-detail'), /Ember Sigil/);
  assert.match(await p.textContent('.dl-detail'), /Shared by Marrow Quill, from Elsewhere/);
  assert.ok((await calls(p)).includes('POST /designs/shared/s1/take'));
});
