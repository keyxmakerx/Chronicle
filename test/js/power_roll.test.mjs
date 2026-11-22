// power_roll.test.mjs — the power roll's arithmetic (rolls/static/js/power_roll.js): what is
// asked of the server for an edge or bane, and how a stored roll reads as a tier.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const file = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'internal', 'plugins', 'rolls', 'static', 'js', 'power_roll.js');
const sandbox = {};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(readFileSync(file, 'utf8'), sandbox);
const P = sandbox.Chronicle.powerRoll;

const rules = {
  dice: '2d10', edge: 2, bane: -2, tiers: [11, 16],
  characteristics: [{ key: 'might', label: 'Might' }, { key: 'agility', label: 'Agility' }],
};
const dice = (a, b, crit) => [{ sides: 10, value: a, crit: crit || '' }, { sides: 10, value: b }];
// A roll as the log stores it: the label and total the server kept.
const roll = (label, a, b, bonus, extra) => Object.assign({ label, dice: dice(a, b), total: a + b + bonus }, extra);

test('edges and banes cancel one for one', () => {
  assert.equal(P.net(1, 1), 0);
  assert.equal(P.net(2, 1), 1);
  assert.equal(P.net(0, 3), -3);
});

test('one edge or bane is the system\'s +2 / -2 in the formula', () => {
  assert.deepEqual({ ...P.plan(rules, 'Agility', 2, 0) }, { formula: '2d10+2', label: 'Agility' });
  assert.deepEqual({ ...P.plan(rules, 'Agility', 2, 1) }, { formula: '2d10+4', label: 'Agility, edge' });
  assert.deepEqual({ ...P.plan(rules, 'Might', 1, -1) }, { formula: '2d10-1', label: 'Might, bane' });
  assert.equal(P.plan(rules, 'Reason', 0, 0).formula, '2d10');
  assert.equal(P.plan(rules, 'Reason', 0, -1).formula, '2d10-2');
});

test('a double edge or double bane adds nothing to the formula', () => {
  assert.deepEqual({ ...P.plan(rules, 'Might', 1, 2) }, { formula: '2d10+1', label: 'Might, double edge' });
  assert.deepEqual({ ...P.plan(rules, 'Might', 1, -2) }, { formula: '2d10+1', label: 'Might, double bane' });
  assert.equal(P.plan(rules, '', 3, 0).label, 'Power roll');
});

test('the tiers break at 11 and 16', () => {
  assert.equal(P.read(roll('Might', 4, 6, 1), rules).tier, 1); // 11
  assert.equal(P.read(roll('Might', 5, 6, 1), rules).tier, 2); // 12
  assert.equal(P.read(roll('Might', 7, 8, 1), rules).tier, 2); // 16
  assert.equal(P.read(roll('Might', 8, 8, 1), rules).tier, 3); // 17
});

test('a double edge moves the tier up one and a double bane down one', () => {
  assert.equal(P.read(roll('Might, double edge', 4, 5, 1), rules).tier, 2); // 10 is tier 1, up to 2
  assert.equal(P.read(roll('Might, double edge', 7, 8, 1), rules).tier, 3); // 16 is tier 2, up to 3
  assert.equal(P.read(roll('Might, double bane', 7, 8, 1), rules).tier, 1);
  assert.equal(P.read(roll('Might, double bane', 2, 3, 1), rules).tier, 1, 'never below tier 1');
  assert.equal(P.read(roll('Might, double edge', 9, 8, 1), rules).tier, 3, 'never above tier 3');
});

test('a single edge is in the total, so it does not also move the tier', () => {
  const r = P.read(roll('Might, edge', 4, 5, 3), rules); // 4 + 5 + (1 + 2) = 12
  assert.equal(r.tier, 2);
  assert.equal(r.total, 12);
});

test('a natural 19 or 20 is tier 3 whatever the total', () => {
  assert.equal(P.read(roll('Might', 9, 10, -2), rules).tier, 3); // total 17 anyway
  assert.equal(P.read({ label: 'Might', dice: dice(9, 10), total: 15 }, rules).tier, 3, 'nat 19 with a total of 15');
  assert.equal(P.read({ label: 'Might, double bane', dice: dice(10, 10), total: 12 }, rules).tier, 3, 'nat 20 beats a double bane');
  assert.equal(P.read({ label: 'Might', dice: dice(8, 10), total: 12 }, rules).tier, 2, '18 is not natural 19');
});

test('Critical comes only from the server\'s crit mark', () => {
  const marked = { label: 'Might', dice: dice(10, 10, 'succ'), total: 21 };
  const p = P.read(marked, rules);
  assert.equal(p.crit, true);
  assert.equal(P.headline(p), 'Critical · Tier 3 · 21');
  const bare = P.read({ label: 'Might', dice: dice(10, 10), total: 21 }, rules);
  assert.equal(bare.crit, false, 'a nat 20 with no mark is not called Critical here');
  assert.equal(P.headline(bare), 'Tier 3 · 21');
  assert.equal(P.headline(P.read(roll('Agility', 6, 6, 2), rules)), 'Tier 2 · 14');
});

// The dice exactly as the server sends a Draw Steel crit (TestPowerRollCritWire in
// internal/plugins/rolls/crits_test.go pins the same text). The headline reads the result,
// so it says Critical whether or not this person turned crit animations off.
test('a natural 19 marked by the server reads as Critical · Tier 3', () => {
  const wire = JSON.parse('[{"sides":10,"value":9,"crit":"succ"},{"sides":10,"value":10,"crit":"succ"}]');
  assert.equal(P.headline(P.read({ label: 'Might', dice: wire, total: 21 }, rules)), 'Critical · Tier 3 · 21');
  assert.equal(P.headline(P.read({ label: 'Might', dice: wire.map((x) => ({ sides: x.sides, value: x.value, crit: '' })), total: 21 }, rules)), 'Tier 3 · 21');
});

test('only a power roll reads as one', () => {
  assert.equal(P.read(roll('Fade', 5, 5, 0), rules), null, 'not a characteristic');
  assert.equal(P.read({ label: 'Might', dice: [{ sides: 20, value: 5 }], total: 5 }, rules), null);
  assert.equal(P.read({ label: 'Might', hidden: true }, rules), null);
  assert.equal(P.read(roll('Power roll, edge', 5, 6, 2), rules).tier, 2);
  assert.equal(P.read(roll('Might', 5, 5, 0), null), null, 'no declaration, no power roll');
});
