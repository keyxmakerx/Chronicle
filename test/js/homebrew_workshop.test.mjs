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

// The catalog the trackers adapter registers, as the live preview reads it.
const CAT = {
  parts: [{ name: 'tracker', label: 'Tracker', preview: 'shape' }, { name: 'clock', label: 'Clock', preview: 'shape' },
    { name: 'creature-mark', label: 'Creature mark', forceSee: 'director' }],
  events: [{ name: 'tracker.reached', label: 'A tracker reaches', partKinds: ['tracker', 'clock'], preview: { op: 'reach', arg: 'value' } },
    { name: 'moon', label: 'The moon turns' }],
  actions: [{ name: 'step', label: 'Step a tracker', partKinds: ['tracker', 'clock'], preview: { op: 'add', arg: 'by' } },
    { name: 'set', label: 'Set a tracker', partKinds: ['tracker', 'clock'], preview: { op: 'set', arg: 'to' } },
    { name: 'tick', label: 'Tick a clock', partKinds: ['clock'], preview: { op: 'add' } },
    { name: 'rise-as-variant', label: 'Rise as its variant', partKinds: ['creature-mark'], clientRun: true }]
};

test('the preview keeps a shape\'s bounds and bands as the trackers ledger does', () => {
  const { H } = load();
  const cases = [
    [{ shape: 'count', min: 0, max: 5 }, 9, 5], [{ shape: 'count', min: 0, max: 5 }, -2, 0],
    [{ shape: 'count' }, -7, -7], [{ shape: 'count', max: 3 }, -7, -7], [{ shape: 'count', min: 2 }, 900, 900],
    [{ shape: 'clock', max: 6 }, 8, 6], [{ shape: 'clock', max: 6 }, -1, 0],
    [{ shape: 'steps', steps: [{ name: 'a' }, { name: 'b' }, { name: 'c' }] }, 5, 2]
  ];
  for (const [d, v, want] of cases) assert.equal(H.clampV(d, v), want, JSON.stringify(d) + ' ' + v);
  assert.deepEqual(plain(H.bounds({ shape: 'count' })), { lo: 0, hi: null, floor: false });
  const banded = { shape: 'count', max: 5, bands: [{ at: 4, text: 'Changes on the 24th' }, { at: 2, text: 'Bane to resist' }] };
  assert.equal(H.nowText(banded, 1), '');
  assert.equal(H.nowText(banded, 3), 'Bane to resist');
  assert.equal(H.nowText(banded, 5), 'Changes on the 24th');
  assert.equal(H.nowText({ shape: 'steps', steps: [{ name: 'Calm', text: '' }, { name: 'Angry', text: 'Attacks' }] }, 1), 'Angry: Attacks');
});

test('a new preview starts each numbered part at its lowest and leaves other kinds alone', () => {
  const { H } = load();
  const doc = sample();
  doc.parts.push({ key: 'mark', kind: 'creature-mark', label: 'Mark', see: 'director', data: {} });
  doc.parts[0].data.min = 1;
  const st = H.simStart(doc, CAT);
  assert.deepEqual(plain(st.v), { need: 1, doom: 0 });
  doc.parts[0].data.max = 0; doc.parts[0].data.min = -3;
  st.v.need = 4;
  H.simSync(doc, CAT, st);
  assert.equal(st.v.need, 0, 'a changed shape re-clamps');
  doc.parts.shift();
  H.simSync(doc, CAT, st);
  assert.deepEqual(plain(st.v), { doom: 0 }, 'a removed part is dropped');
});

test('stepping a part runs the rules that watch it, in order, and lights their cards', () => {
  const { H } = load();
  const doc = sample();
  const st = H.simStart(doc, CAT);
  let lit = [];
  for (let i = 0; i < 3; i++) lit = H.simNudge(doc, CAT, st, 'need', 1);
  assert.equal(st.v.need, 3);
  assert.deepEqual(plain(lit), ['p:need'], 'nothing watches 3');
  lit = H.simNudge(doc, CAT, st, 'need', 1);
  // Reaching 4 ticks Doom, then sets Need back to 0.
  assert.deepEqual(plain(lit), ['p:need', 'r0', 'r0.0', 'p:doom', 'r0.1', 'p:need']);
  assert.deepEqual(plain(st.v), { need: 0, doom: 1 });
  assert.match(st.log.at(-1), /Set a tracker: Need 4 to 0/);
  assert.deepEqual(plain(H.simNudge(doc, CAT, st, 'need', -1)), ['p:need'], 'a step past the floor is lost');
  assert.equal(st.v.need, 0);
});

test('a reach fires when the number lands on its value or moves past it, either way', () => {
  const { H } = load();
  const doc = sample();
  doc.rules[0].then = [{ action: 'tick', part: 'doom', args: {} }];
  const st = H.simStart(doc, CAT);
  H.simSetTo(doc, CAT, st, 'need', 5);
  assert.equal(st.v.doom, 1, 'jumping past 4 counts');
  H.simSetTo(doc, CAT, st, 'need', 2);
  assert.equal(st.v.doom, 2, 'coming down past 4 counts');
  assert.equal(H.simSetTo(doc, CAT, st, 'need', 2), null, 'already there');
  assert.equal(H.simSetTo(doc, CAT, st, 'need', 9), null, 'past the top');
});

test('a rule timed for later waits until it is run', () => {
  const { H } = load();
  const doc = sample();
  doc.rules[0].timing = 'held';
  const st = H.simStart(doc, CAT);
  H.simSetTo(doc, CAT, st, 'need', 4);
  assert.equal(st.v.doom, 0);
  assert.deepEqual(plain(st.waiting.map((w) => [w.rule, w.what, w.timing])), [[0, 'A tracker reaches: Need', 'held']]);
  // Removing an earlier rule keeps the waiting one pointing at itself.
  doc.rules.unshift({ when: { event: 'moon', part: '', args: {} }, timing: 'now', then: [] });
  H.simSync(doc, CAT, st);
  assert.equal(st.waiting[0].rule, 1);
  const lit = H.simRunWaiting(doc, CAT, st, 0);
  assert.deepEqual(plain(lit), ['r1', 'r1.0', 'p:doom', 'r1.1', 'p:need']);
  assert.deepEqual(plain(st.v), { need: 0, doom: 1 });
  assert.equal(st.waiting.length, 0);
});

test('a When from outside the recipe is a button, and actions it can\'t act out say what they would do', () => {
  const { H } = load();
  const doc = sample();
  doc.parts.push({ key: 'mark', kind: 'creature-mark', label: 'Mark', see: 'director', data: {} });
  doc.rules[1].then.push({ action: 'rise-as-variant', part: 'mark', args: {} });
  assert.deepEqual(plain(H.firable(doc, CAT)), [1]);
  const st = H.simStart(doc, CAT);
  assert.deepEqual(plain(H.simFire(doc, CAT, st, 1)), ['r1'], 'a held rule only waits');
  assert.equal(st.waiting.length, 1);
  doc.rules[1].timing = 'now';
  const lit = H.simFire(doc, CAT, st, 1);
  assert.deepEqual(plain(lit), ['r1', 'r1.0', 'p:need', 'r1.1']);
  assert.equal(st.v.need, 1);
  assert.equal(st.log.at(-1), 'You would be asked to run: Rise as its variant, Mark');
});

test('rules that set each other off stop instead of hanging the page', () => {
  const { H } = load();
  const doc = sample();
  doc.parts[0].data = { shape: 'count' };
  doc.rules = [
    { when: { event: 'tracker.reached', part: 'need', args: { value: 1 } }, timing: 'now', then: [{ action: 'set', part: 'need', args: { to: 0 } }] },
    { when: { event: 'tracker.reached', part: 'need', args: { value: 0 } }, timing: 'now', then: [{ action: 'set', part: 'need', args: { to: 1 } }] }
  ];
  const st = H.simStart(doc, CAT);
  H.simNudge(doc, CAT, st, 'need', 1);
  assert.match(st.log.at(-1), /^Stopped after 50 steps/);
  assert.ok(st.log.length <= 40, 'the log stays short');
});
