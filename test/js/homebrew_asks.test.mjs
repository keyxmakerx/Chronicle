// homebrew_asks.test.mjs: the pure helpers of a person's own asks list,
// loaded in a bare vm context with a stubbed Chronicle the way the server
// ships it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'homebrew', 'static', 'js', 'homebrew_asks.js'), 'utf8');

function load() {
  const registered = {};
  const sandbox = { window: {}, Chronicle: { register: (n, i) => { registered[n] = i; } }, document: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { H: sandbox.window.HomebrewAsks, registered };
}

test('registers the asks widget', () => {
  const { registered } = load();
  assert.equal(typeof registered['homebrew-asks'].init, 'function');
});

test('title capitalises the ask, or says "Your ask"', () => {
  const { H } = load();
  assert.equal(H.title({ ask: 'draw a card' }), 'Draw a card');
  assert.equal(H.title({}), 'Your ask');
});

test('each state has its word and colour', () => {
  const { H } = load();
  const cases = [
    [{ state: 'waiting' }, ['Your Director is answering', 'wait']],
    [{ state: 'yes' }, ['Yes', 'ok']],
    [{ state: 'yes', changes: ['Copies: 2 (was 1)'] }, ['Yes, with changes', 'ok']],
    [{ state: 'no' }, ['No', 'bad']],
    [{ state: 'other', reply: 'Later.' }, ['Replied', 'note']],
    [{ state: 'taken_back' }, ['Taken back', 'off']]
  ];
  for (const [a, want] of cases) assert.deepEqual(Array.from(H.pill(a, 'Director')), want, a.state);
  assert.equal(H.pill({ state: 'waiting' }, 'GM')[0], 'Your GM is answering');
});

test('said is what came back', () => {
  const { H } = load();
  assert.equal(H.said({ state: 'waiting' }), '');
  assert.equal(H.said({ state: 'yes' }), 'Yes. The rule goes on.');
  assert.equal(H.said({ state: 'yes', changes: ['x'] }), 'Yes, with these changes:');
  assert.equal(H.said({ state: 'no' }), 'No. The rule stops here.');
  assert.equal(H.said({ state: 'other', reply: 'After the fight.' }), 'After the fight.');
});

test('a roll step reads as the condition of a yes, then as its result', () => {
  const { H } = load();
  const roll = { label: 'Might', formula: '2d10+2', target: 12 };
  assert.equal(H.rollButton(roll), 'Roll Might (2d10 + 2)');
  assert.equal(H.rollButton({ formula: '1d20-1', target: 5 }), 'Roll 1d20 - 1');
  const cases = [
    [{ state: 'roll', answered: 'yes', roll }, ['Yes, with a step first', 'ok'], 'Yes, if you roll Might 12 or more first.', ''],
    [{ state: 'roll', roll: { formula: '2d6', target: 7 } }, ['Your roll first', 'wait'], 'Roll 7 or more on 2d6 first.', ''],
    [{ state: 'rolled', answered: 'yes', roll: Object.assign({ total: 15, made: true }, roll) }, ['Made the roll', 'ok'],
      'Yes, if you roll Might 12 or more first.', 'Rolled 15. The rule carries on.'],
    [{ state: 'missed', answered: 'yes', roll: Object.assign({ total: 9 }, roll) }, ['Roll fell short', 'bad'],
      'Yes, if you roll Might 12 or more first.', 'Rolled 9, under 12. The rule stops here.'],
    [{ state: 'set_aside' }, ['Set aside', 'off'], 'Set aside before the roll. The rule stops here.', '']
  ];
  for (const [a, pill, said, result] of cases) {
    assert.deepEqual(Array.from(H.pill(a, 'Director')), pill, a.state);
    assert.equal(H.said(a), said, a.state);
    assert.equal(H.result(a), result, a.state);
  }
});
