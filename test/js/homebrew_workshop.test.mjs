// homebrew_workshop.test.mjs: the pure helpers of the Homebrew Workshop. The
// widget file is loaded in a bare vm context with a stubbed Chronicle, the
// way the server ships it, and its exported helpers are driven directly.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const plugin = join(here, '..', '..', 'internal', 'plugins', 'homebrew');
const src = readFileSync(join(plugin, 'static', 'js', 'homebrew_workshop.js'), 'utf8');

function load() {
  const registered = {};
  const sandbox = { window: {}, Chronicle: { register: (n, i) => { registered[n] = i; } }, document: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { H: sandbox.window.HomebrewWorkshop, registered };
}

const plain = (o) => JSON.parse(JSON.stringify(o));

function sample() {
  return {
    parts: [
      { key: 'need', kind: 'tracker', label: 'Need', see: 'party', data: { shape: 'count', min: 0, max: 5 } },
      { key: 'doom', kind: 'clock', label: 'Doom', see: 'director', data: { shape: 'clock', max: 6 } }
    ],
    rules: [
      { when: { event: 'tracker.reached', part: 'need', args: { value: 4 } }, timing: 'now',
        then: [{ action: 'tick', part: 'doom', args: {} }, { action: 'set', part: 'need', args: { to: 0 } }] },
      { when: { event: 'moon', part: '', args: {} }, timing: 'held', then: [{ action: 'step', part: 'need', args: { by: 1 } }] }
    ],
    layout: {}
  };
}

test('the widget registers once under its mount name', () => {
  const { registered } = load();
  assert.deepEqual(Object.keys(registered), ['homebrew-workshop']);
  assert.equal(typeof registered['homebrew-workshop'].init, 'function');
  assert.equal(typeof registered['homebrew-workshop'].destroy, 'function');
});

test('a part key is made from its label and never repeats', () => {
  const { H } = load();
  const go = readFileSync(join(plugin, 'validate.go'), 'utf8');
  const keyRe = new RegExp(/keyRe = regexp\.MustCompile\(`([^`]+)`\)/.exec(go)[1]);
  const cases = [
    ['New tracker', [], 'new-tracker'],
    ['New tracker', ['new-tracker'], 'new-tracker-2'],
    ['New tracker', ['new-tracker', 'new-tracker-2'], 'new-tracker-3'],
    ['  !!  ', [], 'part'],
    ['Ünïcode Ink', [], 'n-code-ink'],
    ['x'.repeat(80), [], 'x'.repeat(32)]
  ];
  for (const [label, taken, want] of cases) {
    const got = H.partKey(label, taken);
    assert.equal(got, want, label);
    assert.ok(keyRe.test(got) && got.length <= 40, `${got} is a key the server accepts`);
  }
});

test('cards come from the doc: one per part, When and Then', () => {
  const { H } = load();
  const doc = sample();
  const ids = plain(H.cardsOf(doc).map((c) => c.id));
  assert.deepEqual(ids, ['r0', 'r0.0', 'r0.1', 'r1', 'r1.0', 'p:need', 'p:doom']);
  // The ids are the layout keys the server keeps.
  const go = readFileSync(join(plugin, 'validate.go'), 'utf8');
  const ruleRe = new RegExp(/ruleCardRe = regexp\.MustCompile\(`([^`]+)`\)/.exec(go)[1]);
  ids.filter((id) => id[0] === 'r').forEach((id) => assert.ok(ruleRe.test(id), id));
});

test('a card sits where the layout puts it, else in its column without overlap', () => {
  const { H } = load();
  const doc = sample();
  doc.layout = { 'p:doom': { x: 900, y: 12 } };
  const cards = H.cardsOf(doc);
  const at = Object.fromEntries(cards.map((c) => [c.id, plain(c.pos)]));
  assert.deepEqual(at['p:doom'], { x: 900, y: 12 });
  assert.ok(at['r0'].x < at['r0.0'].x && at['r0.0'].x < at['p:need'].x, 'When, Then and Part columns run left to right');
  assert.ok(at['r0.1'].y > at['r0.0'].y, 'a rule\'s Thens stack downwards');
  assert.ok(at['r1'].y > at['r0.1'].y, 'the next rule starts below the last Then');
});

test('edges run from a part to its When, the When to its Thens, and each Then to its part', () => {
  const { H } = load();
  assert.deepEqual(plain(H.edgesOf(sample())), [
    ['p:need', 'r0'], ['r0', 'r0.0'], ['r0.0', 'p:doom'], ['r0', 'r0.1'], ['r0.1', 'p:need'],
    ['r1', 'r1.0'], ['r1.0', 'p:need']
  ]);
});

test('shapes and args read as words', () => {
  const { H } = load();
  assert.equal(H.shapeText({ shape: 'clock', max: 6 }), 'Clock of 6 segments');
  assert.equal(H.shapeText({ shape: 'steps', steps: [{ name: 'Calm' }, { name: 'Angry' }] }), 'Named steps: Calm, Angry');
  assert.equal(H.shapeText({ shape: 'count', min: 0, max: 5 }), 'A number, 0 to 5');
  assert.equal(H.shapeText({ shape: 'count' }), 'A number, no limits');
  assert.equal(H.shapeText({ shape: 'count', min: 1 }), 'A number, 1 to no highest');
  const fields = [{ name: 'by', label: 'By', type: 'whole' }, { name: 'note', label: 'Note', type: 'text' }];
  assert.equal(H.argsText(fields, { by: 2 }), 'By 2');
  assert.equal(H.argsText(fields, { by: 0, note: 'x' }), 'By 0, Note x');
  assert.equal(H.argsText(fields, {}), '');
  assert.equal(H.argsText(undefined, { by: 1 }), '');
});

test('only parts of a kind the event or action takes are offered', () => {
  const { H } = load();
  const doc = sample();
  assert.deepEqual(H.compatible(doc, { partKinds: ['clock'] }).map((p) => p.key), ['doom']);
  assert.deepEqual(H.compatible(doc, { partKinds: ['tracker', 'clock'] }).map((p) => p.key), ['need', 'doom']);
  assert.deepEqual(plain(H.compatible(doc, { partKinds: [] })), []);
  assert.deepEqual(plain(H.compatible(doc, null)), []);
});

test('removing a part takes the rules and steps that used it, and says so', () => {
  const { H } = load();
  const doc = sample();
  const gone = H.removePart(doc, 'need');
  assert.deepEqual(plain(gone), { rules: 1, steps: 1 });
  assert.deepEqual(doc.parts.map((p) => p.key), ['doom']);
  assert.equal(doc.rules.length, 1);
  assert.equal(doc.rules[0].when.event, 'moon');
  assert.deepEqual(plain(doc.rules[0].then), []);
});

test('new trackers and clocks start with shapes the server accepts', () => {
  const { H } = load();
  assert.deepEqual(plain(H.defaultData('clock')), { shape: 'clock', max: 6 });
  assert.deepEqual(plain(H.defaultData('tracker')), { shape: 'count', min: 0, max: 5 });
  assert.deepEqual(plain(H.defaultData('creature-mark')), {});
  const go = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'trackers', 'validate.go'), 'utf8');
  const num = (name) => Number(new RegExp(name + '\\s*=\\s*(\\d+)').exec(go)[1]);
  const seg = H.defaultData('clock').max;
  assert.ok(seg >= num('MinSegments') && seg <= num('MaxSegments'));
});

test('a new When or Then starts with every whole-number field at 1', () => {
  const { H } = load();
  assert.deepEqual(plain(H.startArgs({ fields: [{ name: 'by', label: 'By', type: 'whole' }, { name: 'note', label: 'Note', type: 'text' }] })), { by: 1 });
  assert.deepEqual(plain(H.startArgs({})), {});
  assert.deepEqual(plain(H.startArgs(null)), {});
});

test('a new card is moved down until it covers no other card', () => {
  const { H } = load();
  const doc = sample();
  doc.layout = { 'p:need': { x: 600, y: 40 }, 'p:doom': { x: 600, y: 150 } };
  const spot = plain(H.freeSpot(doc, { x: 610, y: 50 }));
  assert.equal(spot.x, 610);
  assert.ok(spot.y >= 250, `moved below both parts, got ${spot.y}`);
  assert.deepEqual(plain(H.freeSpot(doc, { x: 2000, y: 50 })), { x: 2000, y: 50 }, 'an empty place is kept');
});
