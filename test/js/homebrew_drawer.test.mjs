// homebrew_drawer.test.mjs: the pure helpers of the Homebrew drawer, loaded
// in a bare vm context with a stubbed Chronicle the way the server ships it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'homebrew', 'static', 'js', 'homebrew_drawer.js'), 'utf8');

function load() {
  const registered = {};
  const sandbox = { window: {}, Chronicle: { register: (n, i) => { registered[n] = i; } }, document: {}, console, Date };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { H: sandbox.window.HomebrewDrawer, registered };
}

test('registers the drawer widget', () => {
  const { registered } = load();
  assert.equal(typeof registered['homebrew-drawer'].init, 'function');
});

test('ago says when a run happened', () => {
  const { H } = load();
  const now = Date.parse('2026-10-19T12:00:00Z');
  assert.equal(H.ago('2026-10-19T11:59:40Z', now), 'just now');
  assert.equal(H.ago('2026-10-19T11:45:00Z', now), '15 min ago');
  assert.equal(H.ago('2026-10-19T11:00:00Z', now), '1 hour ago');
  assert.equal(H.ago('2026-10-19T07:00:00Z', now), '5 hours ago');
  assert.equal(H.ago('not a date', now), '');
});

test('stepNote says what Run does with each step', () => {
  const { H } = load();
  assert.equal(H.stepNote({ byHand: true }, false), 'For you to do');
  assert.equal(H.stepNote({ browser: true }, true), 'Runs here');
  assert.match(H.stepNote({ browser: true }, false), /^For you to do/);
  assert.equal(H.stepNote({}, false), 'Runs on the server');
  assert.equal(H.stepNote({ rolls: true }, false), 'Waiting on the player to roll');
  assert.equal(H.stepNote({ asks: true }, false), 'Asks you before what follows');
});

test('statusText and pressLabel read plainly', () => {
  const { H } = load();
  assert.equal(H.statusText('dismissed'), 'Set aside');
  assert.equal(H.statusText('failed'), 'Stopped');
  assert.equal(H.pressLabel({ label: 'Log a respite', rules: 1 }), 'Log a respite (1 rule)');
  assert.equal(H.pressLabel({ label: 'Log a respite', rules: 3 }), 'Log a respite (3 rules)');
});

test('a named button posts its name and keeps its spelling', () => {
  const { H } = load();
  const respite = { event: 'respite.logged', label: 'Log a respite', rules: 1 };
  const dose = { event: 'button.pressed', label: 'Dose & Calm', button: 'Dose & Calm', arg: 'button', rules: 2 };
  assert.equal(H.pressURL('/campaigns/c1/homebrew', respite), '/campaigns/c1/homebrew/press/respite.logged');
  assert.equal(H.pressURL('/campaigns/c1/homebrew', dose), '/campaigns/c1/homebrew/press/button.pressed?button=Dose%20%26%20Calm');
  assert.equal(H.pressLabel(dose), 'Dose & Calm (2 rules)');
  assert.equal(H.pressRunning(respite), 'Running log a respite…');
  assert.equal(H.pressRunning(dose), 'Running Dose & Calm…');
});

test('an ask reads as who wants what', () => {
  const { H } = load();
  assert.equal(H.askTitle({ asker: 'Mira', ask: 'draw a card' }), 'Mira wants to draw a card');
  assert.equal(H.askTitle({ asker: 'Mira' }), 'Mira is asking you');
  assert.equal(H.askTitle({}), 'A player is asking you');
});

test('editsFor sends only the steps the Director changed, with whole args', () => {
  const { H } = load();
  const later = [
    { step: 2, action: 'deck.add-copies', args: { deck: 'Fortunes', card: 'Luck', copies: 1 },
      fields: [{ name: 'deck', label: 'Deck', type: 'text' }, { name: 'card', label: 'Card', type: 'text' }, { name: 'copies', label: 'Copies', type: 'whole' }] },
    { step: 3, action: 'step', part: 'doom', args: { by: 1 }, fields: [{ name: 'by', label: 'By', type: 'whole' }] }
  ];
  const plain = (v) => JSON.parse(JSON.stringify(v));
  assert.deepEqual(plain(H.editsFor(later, {})), { steps: [], error: '' });
  assert.deepEqual(plain(H.editsFor(later, { 2: { part: null, args: { deck: 'Fortunes', copies: '1' } } })), { steps: [], error: '' },
    'values equal to the rule are no change');
  assert.deepEqual(plain(H.editsFor(later, { 2: { part: null, args: { deck: ' Rewards ', copies: '3' } } })),
    { steps: [{ step: 2, args: { deck: 'Rewards', card: 'Luck', copies: 3 } }], error: '' });
  assert.deepEqual(plain(H.editsFor(later, { 3: { part: 'dread', args: {} } })),
    { steps: [{ step: 3, part: 'dread', args: { by: 1 } }], error: '' });
  assert.deepEqual(plain(H.editsFor(later, { 2: { part: null, args: { card: '' } } })).steps[0].args, { deck: 'Fortunes', copies: 1 },
    'an emptied text field leaves the arg out');
  assert.equal(H.editsFor(later, { 3: { part: null, args: { by: 'two' } } }).error, 'By needs a whole number.');
});

test('sendLabel says what Send does', () => {
  const { H } = load();
  const later = [{ label: 'Shuffle copies of a card into a deck' }, { label: 'Step' }];
  assert.equal(H.sendLabel('no', later, false), 'Send: no');
  assert.equal(H.sendLabel('other', later, false), 'Send reply');
  assert.equal(H.sendLabel('yes', [], false), 'Send: yes');
  assert.equal(H.sendLabel('yes', later, false), 'Send: yes, then shuffle copies of a card into a deck, then step');
  assert.equal(H.sendLabel('yes', later.slice(1), true), 'Send: yes, with changes, then step');
});
