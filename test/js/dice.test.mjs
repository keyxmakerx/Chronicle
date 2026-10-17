// dice.test.mjs — pins Chronicle.dice: the values it settles before drawing
// (ranges, the tens die, caller-decided results, refusals), the numbering of
// every die, and the promise that a real cannon-es throw, turned by one of
// the die's own symmetries, lands on the decided face.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { webcrypto } from 'node:crypto';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const T = await import(join(root, 'static/vendor/three.module.min.js'));
const C = await import(join(root, 'static/vendor/cannon-es.js'));

function load() {
  const window = { crypto: webcrypto, Chronicle: {} };
  vm.runInNewContext(readFileSync(join(root, 'static/js/dice.js'), 'utf8'), { window, Uint32Array, Promise });
  return window.Chronicle.dice;
}
const D = load();
const I = D._internal;

test('every die rolls only its own faces', () => {
  const cases = [
    { die: { sides: 4 }, ok: (v) => v >= 1 && v <= 4 },
    { die: { sides: 6 }, ok: (v) => v >= 1 && v <= 6 },
    { die: { sides: 8 }, ok: (v) => v >= 1 && v <= 8 },
    { die: { sides: 10 }, ok: (v) => v >= 1 && v <= 10 },
    { die: { sides: 10, tens: true }, ok: (v) => v >= 0 && v <= 90 && v % 10 === 0 },
    { die: { sides: 12 }, ok: (v) => v >= 1 && v <= 12 },
    { die: { sides: 20 }, ok: (v) => v >= 1 && v <= 20 },
  ];
  for (const { die, ok } of cases) {
    const seen = new Set();
    for (let i = 0; i < 2000; i++) {
      const v = I.decide([{ label: 'x', dice: [die] }])[0].values[0];
      assert.ok(ok(v), `d${die.sides}${die.tens ? ' tens' : ''} gave ${v}`);
      seen.add(v);
    }
    assert.equal(seen.size, die.tens ? 10 : die.sides, `every face of d${die.sides} turns up`);
  }
});

test('a d6 is fair within a loose bound', () => {
  const n = [0, 0, 0, 0, 0, 0, 0];
  for (let i = 0; i < 12000; i++) n[I.decide([{ dice: [{ sides: 6 }] }])[0].values[0]]++;
  for (let f = 1; f <= 6; f++) assert.ok(n[f] > 1800 && n[f] < 2200, `face ${f} came up ${n[f]} times`);
});

test('results come back grouped and labelled, keeping caller values', async () => {
  const groups = [
    { label: 'Power roll', dice: [{ sides: 10 }, { sides: 10 }] },
    { label: 'Percentile', dice: [{ sides: 10, tens: true }, { sides: 10 }] },
  ];
  const out = await D.roll(groups, { results: [[7, 10], [0, 3]] });
  assert.deepEqual(JSON.parse(JSON.stringify(out)), [
    { label: 'Power roll', values: [7, 10] },
    { label: 'Percentile', values: [0, 3] },
  ]);
  // A partial results array decides only the dice it names.
  const part = await D.roll([{ label: 'a', dice: [{ sides: 6 }, { sides: 6 }] }], { results: [[2]] });
  assert.equal(part[0].values[0], 2);
  assert.ok(part[0].values[1] >= 1 && part[0].values[1] <= 6);
});

test('impossible dice and values are refused', async () => {
  const bad = [
    [[{ dice: [{ sides: 7 }] }], {}],
    [[{ dice: [{ sides: 6, tens: true }] }], {}],
    [[{ label: 'no dice' }], {}],
    [[{ dice: [{ sides: 6 }] }], { results: [[7]] }],
    [[{ dice: [{ sides: 10, tens: true }] }], { results: [[15]] }],
    [[{ dice: [{ sides: 10, tens: true }] }], { results: [[100]] }],
    [[{ dice: [{ sides: 20 }] }], { results: [[0]] }],
  ];
  for (const [groups, opts] of bad) await assert.rejects(D.roll(groups, opts), JSON.stringify([groups, opts]));
  await assert.rejects(D.roll('2d6'));
});

test('each die is numbered like a real one', () => {
  for (const sides of [4, 6, 8, 10, 12, 20]) {
    const sd = I.solid(sides);
    assert.equal(sd.faces.length, sides, `d${sides} faces`);
    assert.deepEqual([...sd.faces.map((f) => f.value)].sort((a, b) => a - b), Array.from({ length: sides }, (_, i) => i + 1));
    if (sides === 4) {
      // A d4 face means the corner it does not touch: the top one when it rests there.
      sd.faces.forEach((f) => assert.ok(!f.idx.includes(f.value - 1)));
      continue;
    }
    for (const f of sd.faces) {
      const opp = sd.faces.reduce((a, g) => (f.n.x * g.n.x + f.n.y * g.n.y + f.n.z * g.n.z < f.n.x * a.n.x + f.n.y * a.n.y + f.n.z * a.n.z ? g : a));
      assert.equal(f.value + opp.value, sides + 1, `d${sides} opposite faces of ${f.value}`);
    }
  }
});

test('the tens die reads 00 to 90 and means 0 to 90', () => {
  const sd = I.solid(10), tens = { sides: 10, tens: true };
  const labels = [...sd.faces.map((f) => I.faceLabel(tens, f))].sort();
  assert.deepEqual(labels, ['00', '10', '20', '30', '40', '50', '60', '70', '80', '90']);
  for (const f of sd.faces) assert.equal(I.faceValue(tens, f), f.value === 10 ? 0 : f.value * 10);
  for (let v = 0; v <= 90; v += 10) assert.equal(I.faceValue(tens, sd.faces[I.faceFor(sd, tens, v)]), v);
});

test('every face can be turned onto every other by the die\'s own symmetry', () => {
  for (const sides of [4, 6, 8, 10, 12, 20]) {
    const sd = I.solid(sides);
    for (let a = 0; a < sides; a++) for (let b = 0; b < sides; b++) {
      const q = I.symmetry(T, sd, a, b, null);
      const n = new T.Vector3(sd.faces[a].n.x, sd.faces[a].n.y, sd.faces[a].n.z).applyQuaternion(q);
      const m = sd.faces[b].n;
      assert.ok(Math.abs(n.x - m.x) + Math.abs(n.y - m.y) + Math.abs(n.z - m.z) < 1e-4, `d${sides} face ${a} onto ${b}`);
    }
  }
});

test('a real throw, turned by the symmetry, lands on the decided face', () => {
  const size = 1;
  for (const spec of [{ sides: 4 }, { sides: 6 }, { sides: 8 }, { sides: 10 }, { sides: 10, tens: true }, { sides: 12 }, { sides: 20 }]) {
    const sd = I.solid(spec.sides);
    const dice = [0, 1, 2].map(() => ({ sd, shape: I.bodyShape(C, sd, size), size }));
    const want = I.decide([{ dice: [spec, spec, spec] }])[0].values;
    const plan = I.simulate(C, { hw: 14, hh: 9, padTop: 56 }, dice, null);
    dice.forEach((d, i) => {
      const k = (plan.steps - 1) * 7, f = plan.frames[i];
      const qSim = new T.Quaternion(f[k + 3], f[k + 4], f[k + 5], f[k + 6]);
      const fix = I.symmetry(T, sd, I.faceFor(sd, spec, want[i]), plan.landed[i].face, qSim);
      const shown = qSim.clone().multiply(fix);
      const got = I.faceValue(spec, sd.faces[I.landedFace(sd, shown).face]);
      assert.equal(got, want[i], `d${spec.sides}${spec.tens ? ' tens' : ''} die ${i}`);
    });
  }
});
