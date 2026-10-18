// dice.test.mjs — pins Chronicle.dice: the values it settles before drawing
// (ranges, the tens die, caller-decided results, refusals), which dice crit
// (the d20 rule, a game system's own rule, the dice that count), how each die
// gets its look, and in the engine the numbering of every die, the promise
// that a real cannon-es throw turned by one of the die's own symmetries lands
// on the decided face, and that Calm's set-down shows the decided face too.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { webcrypto } from 'node:crypto';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const T = await import(join(root, 'static/vendor/three-dice.min.js'));
const C = await import(join(root, 'static/vendor/cannon-es.js'));
const E = await import(join(root, 'static/js/dice/engine.js'));

function load() {
  const window = { crypto: webcrypto, Chronicle: {} };
  vm.runInNewContext(readFileSync(join(root, 'static/js/dice.js'), 'utf8'), { window, Uint32Array, Promise, setTimeout });
  return window.Chronicle.dice;
}
const D = load();
const I = D._internal;
const plain = (v) => JSON.parse(JSON.stringify(v));

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
  assert.deepEqual(plain(out), [
    { label: 'Power roll', values: [7, 10], crits: [null, null] },
    { label: 'Percentile', values: [0, 3], crits: [null, null] },
  ]);
  // A partial results array decides only the dice it names, and a die may carry its own value.
  const part = await D.roll([{ label: 'a', dice: [{ sides: 6 }, { sides: 6 }, { sides: 8, value: 5 }] }], { results: [[2]] });
  assert.equal(part[0].values[0], 2);
  assert.ok(part[0].values[1] >= 1 && part[0].values[1] <= 6);
  assert.equal(part[0].values[2], 5);
});

test('onSettle and onEnd fire once each even when nothing is drawn', async () => {
  const calls = [];
  await D.roll([{ dice: [{ sides: 20 }] }], { onSettle: (pts) => calls.push(['settle', plain(pts)]), onEnd: () => calls.push(['end']) });
  assert.deepEqual(calls, [['settle', []], ['end']]);
});

test('impossible dice and values are refused', async () => {
  const bad = [
    [[{ dice: [{ sides: 7 }] }], {}],
    [[{ dice: [{ sides: 6, tens: true }] }], {}],
    [[{ label: 'no dice' }], {}],
    [[{ dice: [{ sides: 6 }] }], { results: [[7]] }],
    [[{ dice: [{ sides: 6, value: 0 }] }], {}],
    [[{ dice: [{ sides: 10, tens: true }] }], { results: [[15]] }],
    [[{ dice: [{ sides: 10, tens: true }] }], { results: [[100]] }],
    [[{ dice: [{ sides: 20 }] }], { results: [[0]] }],
  ];
  for (const [groups, opts] of bad) await assert.rejects(D.roll(groups, opts), JSON.stringify([groups, opts]));
  await assert.rejects(D.roll('2d6'));
});

test('which dice crit', async () => {
  const kinds = async (dice, values, opts) => (await D.roll([{ dice }], Object.assign({ results: [values] }, opts)))[0].crits;
  const d20 = { sides: 20 };
  const cases = [
    // A natural 20 or 1 on a d20, by default; no other die crits.
    { dice: [d20, d20, d20], values: [20, 1, 12], want: ['succ', 'fail', null] },
    { dice: [{ sides: 12 }, { sides: 10 }, { sides: 4 }], values: [12, 10, 1], want: [null, null, null] },
    // With a die marked as the one that counts (advantage), only it can crit by the d20 rule.
    { dice: [{ sides: 20, counts: true }, d20], values: [14, 20], want: [null, null] },
    { dice: [{ sides: 20, counts: true }, d20], values: [20, 3], want: ['succ', null] },
    // A die's own word wins over every rule.
    { dice: [{ sides: 20, crit: 'none' }, { sides: 6, crit: 'succ' }], values: [20, 2], want: [null, 'succ'] },
    // No crits at all.
    { dice: [d20], values: [20], opts: { crits: false }, want: [null] },
    // Draw Steel: a natural 19 or 20 on the 2d10 power roll is a crit, with no crit fail.
    { dice: [{ sides: 10 }, { sides: 10 }], values: [9, 10], opts: { crits: (g, v) => (v[0] + v[1] >= 19 ? ['succ', 'succ'] : [null, null]) }, want: ['succ', 'succ'] },
    { dice: [{ sides: 10 }, { sides: 10 }], values: [1, 1], opts: { crits: (g, v) => (v[0] + v[1] >= 19 ? ['succ', 'succ'] : [null, null]) }, want: [null, null] },
  ];
  for (const c of cases) assert.deepEqual(plain(await kinds(c.dice, c.values, c.opts)), c.want, JSON.stringify(c.values));
  // A crit preview shows on the first d20 that counts.
  const out = await D.roll([{ dice: [{ sides: 6 }, d20, { sides: 20, counts: true }] }], { crit: 'fail' });
  assert.equal(out[0].values[2], 1);
  assert.deepEqual(plain(out[0].crits), [null, null, 'fail']);
});

test('each die wears its own look, else the table\'s, else the default', () => {
  const groups = [{ dice: [{ sides: 20, set: 'cage', colour: 'iron', succ: 'storm', fail: 'nonsense' }, { sides: 10, tens: true }, { sides: 6, set: '../x', colour: 'red' }] }];
  const out = [{ values: [20, 0, 4], crits: ['succ', null, null] }];
  const table = I.engineDice(E, groups, out, { set: 'oak', colour: 'walnut' });
  assert.deepEqual(plain(table.map((d) => [d.sides, d.kind, d.colour, d.succFx, d.failFx, d.value, d.crit])), [
    [20, 'cage', 'iron', 'storm', null, 20, 'succ'],
    [100, 'oak', 'walnut', null, null, 0, null],
    [6, 'oak', 'red', null, null, 4, null],
  ]);
  const none = I.engineDice(E, groups, out, { set: 'nope' });
  assert.deepEqual(plain(none.map((d) => d.kind)), ['cage', 'jewelled', 'jewelled']);
});

// ---------- The engine ----------

function testDie(sides, size) {
  const sd = E.solid(sides);
  const shape = new C.ConvexPolyhedron({ vertices: sd.v.map((p) => new C.Vec3(p.x * size, p.y * size, p.z * size)), faces: sd.faces.map((f) => f.idx.slice()) });
  return { sd, sides, size, shape, faces: sd.faces.map((f) => ({ f, value: f.value })), group: new T.Group(), rest: (sd.faces[0].c.x * sd.faces[0].n.x + sd.faces[0].c.y * sd.faces[0].n.y + sd.faces[0].c.z * sd.faces[0].n.z) * size };
}
const SIDES = [4, 6, 8, 10, 12, 20];
const faceValues = (sides) => (sides === 100 ? [0, 10, 20, 30, 40, 50, 60, 70, 80, 90] : Array.from({ length: sides }, (_, i) => i + 1));

test('each die is numbered like a real one', () => {
  for (const sides of SIDES) {
    const sd = E.solid(sides);
    assert.equal(sd.faces.length, sides, `d${sides} faces`);
    assert.deepEqual([...sd.faces.map((f) => f.value)].sort((a, b) => a - b), faceValues(sides));
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
  const sd = E.solid(100);
  assert.deepEqual([...sd.faces.map((f) => f.label)].sort(), ['00', '10', '20', '30', '40', '50', '60', '70', '80', '90']);
  for (const f of sd.faces) assert.equal(f.label, f.value ? String(f.value) : '00');
});

test('every face can be turned onto every other by the die\'s own symmetry', () => {
  for (const sides of SIDES) {
    const sd = E.solid(sides);
    for (let a = 0; a < sides; a++) for (let b = 0; b < sides; b++) {
      const q = E.symmetry(T, sd, a, b, null);
      const n = new T.Vector3(sd.faces[a].n.x, sd.faces[a].n.y, sd.faces[a].n.z).applyQuaternion(q);
      const m = sd.faces[b].n;
      assert.ok(Math.abs(n.x - m.x) + Math.abs(n.y - m.y) + Math.abs(n.z - m.z) < 1e-4, `d${sides} face ${a} onto ${b}`);
    }
  }
});

test('a real throw, turned by the symmetry, lands on the decided face', () => {
  for (const sides of [4, 6, 8, 10, 100, 12, 20]) {
    const dice = [0, 1, 2].map(() => testDie(sides, 1));
    const values = faceValues(sides), want = dice.map(() => values[Math.floor(Math.random() * values.length)]);
    const plan = E.simulate(C, { hw: 14, hh: 9, padTop: 56 }, dice, null);
    dice.forEach((d, i) => {
      const k = (plan.steps - 1) * 7, f = plan.frames[i];
      const qSim = new T.Quaternion(f[k + 3], f[k + 4], f[k + 5], f[k + 6]);
      const fix = E.symmetry(T, d.sd, E.faceOf(d, want[i]), plan.landed[i].face, qSim);
      const shown = qSim.clone().multiply(fix);
      assert.equal(d.faces[E.landedFace(d, shown).face].value, want[i], `d${sides} die ${i}`);
    });
  }
});

test('Calm sets each die down showing its decided face, on the table', () => {
  const st = { hw: 13, hh: 8.3, padTop: 56 };
  for (const sides of [4, 6, 8, 10, 100, 12, 20]) {
    for (const v of faceValues(sides)) {
      const dice = [testDie(sides, 0.9), testDie(sides, 0.9), testDie(sides, 0.9)];
      E.setDown(T, st, dice, dice.map(() => v), { x: -9, z: 6 });
      dice.forEach((d, i) => {
        assert.equal(d.faces[E.landedFace(d, d.group.quaternion).face].value, v, `d${sides} showing ${v}, die ${i}`);
        assert.ok(Math.abs(d.group.position.x) < st.hw && Math.abs(d.group.position.z) < st.hh, `d${sides} die ${i} is on the table`);
      });
    }
  }
});
