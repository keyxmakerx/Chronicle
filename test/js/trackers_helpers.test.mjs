// trackers_helpers.test.mjs: the pure helpers of the Trackers block. The
// widget file is loaded in a bare vm context with a stubbed Chronicle, the
// way the server ships it, and its exported helpers are driven directly.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'trackers', 'static', 'js', 'trackers.js'), 'utf8');

function load() {
  const registered = {};
  const sandbox = { window: {}, Chronicle: { register: (n, i) => { registered[n] = i; } }, document: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { H: sandbox.window.TrackerHelpers, registered };
}

const plain = (o) => JSON.parse(JSON.stringify(o));

test('the widget registers once under its mount name', () => {
  const { registered } = load();
  assert.deepEqual(Object.keys(registered), ['trackers']);
  assert.equal(typeof registered.trackers.init, 'function');
  assert.equal(typeof registered.trackers.destroy, 'function');
});

test('the form limits match the server', () => {
  const { H } = load();
  const go = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'trackers', 'validate.go'), 'utf8');
  const num = (name) => Number(new RegExp(name + '\\s*=\\s*(\\d+)').exec(go)[1]);
  assert.equal(H.LIM.name, num('MaxNameChars'));
  assert.equal(H.LIM.text, num('MaxTextChars'));
  assert.equal(H.LIM.note, num('MaxNoteChars'));
  assert.equal(H.LIM.steps, num('MaxSteps'));
  assert.equal(H.LIM.bands, num('MaxBands'));
});

test('parseSteps reads names with and without meanings', () => {
  const { H } = load();
  assert.deepEqual(plain(H.parseSteps('Calm: nothing yet\n\n  Stirring \nInked: the ink: takes hold')), [
    { name: 'Calm', text: 'nothing yet' },
    { name: 'Stirring', text: '' },
    { name: 'Inked', text: 'the ink: takes hold' },
  ]);
  assert.deepEqual(plain(H.parseSteps('')), []);
});

test('parseBands reports lines it cannot read instead of guessing', () => {
  const { H } = load();
  const r = H.parseBands('2: A bane\n4) Takes hold\nno number\n-1 Below zero\n');
  assert.deepEqual(plain(r.bands), [
    { at: 2, text: 'A bane' }, { at: 4, text: 'Takes hold' }, { at: -1, text: 'Below zero' },
  ]);
  assert.deepEqual(plain(r.bad), [3]);
});

test('bandIndex picks the highest band at or below the value', () => {
  const { H } = load();
  const bands = [{ at: 4, text: 'b' }, { at: 2, text: 'a' }];
  const cases = [[1, -1], [2, 1], [3, 1], [4, 0], [9, 0]];
  for (const [v, want] of cases) assert.equal(H.bandIndex(bands, v), want, 'value ' + v);
  assert.equal(H.bandIndex([], 3), -1);
});

test('change and valueLabel describe a tracker in words', () => {
  const { H } = load();
  const steps = { shape: 'steps', steps: [{ name: 'Calm' }, { name: 'Inked' }], value: 1 };
  const clock = { shape: 'clock', max: 6, value: 2 };
  const count = { shape: 'count', value: 7 };
  const cases = [
    [steps, { op: 'set', amount: 1 }, 'set to Inked'],
    [steps, { op: 'add', amount: 1 }, 'stepped forward'],
    [steps, { op: 'add', amount: -1 }, 'stepped back'],
    [count, { op: 'add', amount: 3 }, '+3'],
    [count, { op: 'add', amount: -2 }, '−2'],
    [count, { op: 'set', amount: 0 }, 'set to 0'],
  ];
  for (const [t, e, want] of cases) assert.equal(H.change(t, e), want);
  assert.equal(H.valueLabel(steps), 'Inked');
  assert.equal(H.valueLabel(clock), '2 of 6');
  assert.equal(H.valueLabel(count), '7');
});

test('canStep stops at each shape\'s ends', () => {
  const { H } = load();
  const cases = [
    [{ shape: 'count', value: 0, min: 0, max: null }, -1, false],
    [{ shape: 'count', value: 0, min: null, max: null }, -1, true],
    [{ shape: 'count', value: 99, min: 0, max: null }, 1, true],
    [{ shape: 'count', value: 3, min: 0, max: 3 }, 1, false],
    [{ shape: 'clock', value: 0, max: 4 }, -1, false],
    [{ shape: 'clock', value: 4, max: 4 }, 1, false],
    [{ shape: 'steps', value: 1, steps: [{}, {}] }, 1, false],
    [{ shape: 'steps', value: 1, steps: [{}, {}] }, -1, true],
  ];
  for (const [t, dir, want] of cases) assert.equal(H.canStep(t, dir), want, JSON.stringify(t) + ' ' + dir);
});

test('segments draws one closed wedge per segment from twelve o\'clock', () => {
  const { H } = load();
  const s = H.segments(4);
  assert.equal(s.length, 4);
  assert.match(s[0], /^M0 0 L0\.0000 -1\.0000 A1 1 0 0 1 1\.0000 0\.0000 Z$/);
  s.forEach((d) => assert.match(d, /Z$/));
});

test('kindBody builds each shape and names what to fix', () => {
  const { H } = load();
  assert.deepEqual(plain(H.kindBody({ name: ' Ink ', shape: 'count', min: '0', max: '3', bands: '3: Inked', visibility: 'everyone' })),
    { name: 'Ink', shape: 'count', visibility: 'everyone', steps: [], bands: [{ at: 3, text: 'Inked' }], min: 0, max: 3 });
  assert.deepEqual(plain(H.kindBody({ name: 'Need', shape: 'count', min: '', max: '' })),
    { name: 'Need', shape: 'count', visibility: 'directors', steps: [], bands: [] });
  assert.deepEqual(plain(H.kindBody({ name: 'Bells', shape: 'clock', segments: '6' })),
    { name: 'Bells', shape: 'clock', visibility: 'directors', steps: [], bands: [], max: 6 });
  const steps = H.kindBody({ name: 'Mood', shape: 'steps', steps: 'Calm\nAngry: watch out' });
  assert.equal(steps.steps.length, 2);

  const bad = [
    [{ name: '', shape: 'count' }, /name/],
    [{ name: 'x', shape: 'steps', steps: 'only one' }, /two steps/],
    [{ name: 'x', shape: 'clock', segments: '1' }, /2 to 24/],
    [{ name: 'x', shape: 'clock', segments: '25' }, /2 to 24/],
    [{ name: 'x', shape: 'count', min: '1.5', max: '' }, /whole numbers/],
    [{ name: 'x', shape: 'count', min: '3', max: '3' }, /above lowest/],
    [{ name: 'x', shape: 'count', min: '', max: '', bands: 'nope' }, /Start each band/],
  ];
  for (const [f, re] of bad) assert.match(H.kindBody(f).error || '', re, JSON.stringify(f));
});
