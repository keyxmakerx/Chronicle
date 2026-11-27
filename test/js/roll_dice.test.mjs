// roll_dice.test.mjs — which 3D dice a roll is thrown in on this screen
// (rolls/static/js/roll_dice.js): the dice the server settled for the roller
// when the roll carries them, else the campaign's, or the viewer's own for
// their own roll; and how the details name them.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const file = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'internal', 'plugins', 'rolls', 'static', 'js', 'roll_dice.js');
const sandbox = {};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(readFileSync(file, 'utf8'), sandbox);
const R = sandbox.Chronicle.rollDice;
const plain = (v) => JSON.parse(JSON.stringify(v));

const camp = { set: 'neon', colour: 'blue', succ: 'storm', fail: 'grasp', quality: 'hd' };
const mine = { set: 'oak', colour: 'walnut', succ: 'dawn', fail: 'frost', quality: 'hd' };
const clockwork = { set: 'clockwork', colour: 'brass', critSucc: 'phoenix', critFail: 'abyss', own: true };

test('an incoming roll wears the dice it carries', () => {
  const cases = [
    { name: 'someone else\'s pick', roll: { rollerId: 'u2', rollerDice: clockwork }, want: { set: 'clockwork', colour: 'brass', succ: 'phoenix', fail: 'abyss', quality: 'hd' } },
    { name: 'someone on the campaign\'s dice', roll: { rollerId: 'u2', rollerDice: { set: 'neon', colour: 'blue', critSucc: 'storm', critFail: 'grasp' } }, want: camp },
    { name: 'my own roll, as the server settled it', roll: { mine: true, rollerDice: clockwork }, want: { set: 'clockwork', colour: 'brass', succ: 'phoenix', fail: 'abyss', quality: 'hd' } },
    { name: 'a campaign on the engine defaults', roll: { rollerDice: { set: '', colour: '', critSucc: '', critFail: '' } }, want: { set: '', colour: '', succ: '', fail: '', quality: 'hd' } },
    { name: 'an older roll that carries none', roll: { rollerId: 'u2' }, want: camp },
    { name: 'my older roll, or a test throw', roll: { mine: true }, want: mine },
    { name: 'names that are not dice keys are the defaults', roll: { rollerDice: { set: '../x', colour: 'Dark Red', critSucc: 7, critFail: null } }, want: { set: '', colour: '', succ: '', fail: '', quality: 'hd' } },
  ];
  for (const c of cases) assert.deepEqual(plain(R.forRoll(c.roll, camp, mine)), c.want, c.name);
});

test('detail is the campaign\'s, never the roll\'s', () => {
  assert.equal(R.forRoll({ rollerDice: Object.assign({ quality: 'basic' }, clockwork) }, camp, mine).quality, 'hd');
  assert.equal(R.forRoll({ rollerDice: clockwork }, null, null).quality, '');
});

test('the details say whose dice a roll was thrown in', () => {
  const cat = { sets: [{ key: 'jewelled', name: 'Jewelled', colours: [{ key: 'onyx', name: 'Onyx' }] }, { key: 'clockwork', name: 'Clockwork', colours: [{ key: 'steel', name: 'Steel' }, { key: 'brass', name: 'Brass' }] }] };
  assert.equal(R.name({ rollerDice: clockwork }, cat, 'Mira’s'), 'Mira’s Clockwork · Brass');
  assert.equal(R.name({ rollerDice: clockwork }, cat, 'Your'), 'Your Clockwork · Brass');
  assert.equal(R.name({ rollerDice: { set: 'clockwork', colour: '' } }, cat, 'Mira’s'), 'The campaign’s Clockwork · Steel');
  assert.equal(R.name({ rollerDice: { set: '', colour: '' } }, cat, 'Mira’s'), 'The campaign’s Jewelled · Onyx');
  assert.equal(R.name({}, cat, 'Mira’s'), '', 'a roll that carries no dice names none');
  assert.equal(R.name({ rollerDice: clockwork }, null, 'Mira’s'), '', 'nor before the catalogue is read');
});
