// put_on_page.test.mjs: the "Put on a page" chooser's pure helpers, loaded in
// a bare vm context with a stubbed Chronicle the way the server ships it, and
// a check that the Game & features page and the DM Screen share the one chooser.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'put_on_page.js'), 'utf8');

function load() {
  const registered = {};
  const sandbox = { window: {}, Chronicle: { register: (n, i) => { registered[n] = i; } }, document: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { P: sandbox.window.PutOnPage, registered };
}

const cat = (id, plural, extra = {}) => ({ id, name: plural.slice(0, -1), namePlural: plural, suggested: false, placed: false, ...extra });

test('registers the put-on-page widget', () => {
  const { registered } = load();
  assert.equal(typeof registered['put-on-page'].init, 'function');
});

test('the heading names the block the way the card draws it', () => {
  const { P } = load();
  assert.equal(P.heading('quest_board'), 'Put the quest board on');
  assert.equal(P.heading('notice_boards'), 'Put the notice boards on');
  assert.equal(P.heading('trackers'), 'Put the Trackers on');
  assert.equal(P.heading('page_tests'), 'Put the Tests on');
});

test('a row reads "<Category> pages"', () => {
  const { P } = load();
  assert.equal(P.rowLabel(cat(1, 'Quests')), 'Quests pages');
});

test('the suggested category stays first and the long tail waits behind "Another category"', () => {
  const { P } = load();
  const cats = [cat(1, 'Quests', { suggested: true }), cat(2, 'Locations'), cat(3, 'Adventures'), cat(4, 'Items'), cat(5, 'Notes')];
  const v = P.view(cats);
  assert.deepEqual(v.shown.map((c) => c.id), [1, 2, 3]);
  assert.deepEqual(v.rest.map((c) => c.id), [4, 5]);
  assert.equal(v.shown[0].suggested, true);
});

test('a short list has nothing behind "Another category"', () => {
  const { P } = load();
  assert.equal(P.view([cat(1, 'Quests')]).rest.length, 0);
  assert.equal(JSON.stringify(P.view(null)), JSON.stringify({ shown: [], rest: [] }));
});

test('the toast says where it went, or that it was already there', () => {
  const { P } = load();
  const q = cat(1, 'Quests');
  assert.equal(P.doneMessage('quest_board', q, 'added'), 'Put the quest board at the end of Quests pages. Move it later in the Customizer.');
  assert.equal(P.doneMessage('quest_board', q, 'already_there'), 'The quest board is already on Quests pages.');
});

test('the DM Screen shares the chooser instead of copying it', () => {
  const drawer = readFileSync(join(root, 'internal', 'plugins', 'homebrew', 'static', 'js', 'homebrew_drawer.js'), 'utf8');
  assert.match(drawer, /data-widget': 'put-on-page'/);
  assert.match(drawer, /data-put-block': 'trackers'/);
  assert.doesNotMatch(drawer, /entity-types/, 'the drawer must not call the layout routes itself');
});
