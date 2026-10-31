// crafting_turns.test.mjs — the Crafting block's experimenting, breaking
// down and helping panels (internal/plugins/crafting/static/js/crafting_turns.js)
// and the bench moments that play them (static/js/widgets/craft_moments.js).
//
// Pins what a player is told: a near miss shows the hint and never a
// recipe's name, a find names who learned it, which copies can be broken
// down, and a hand's roll names the helper's characteristic. A moment plays
// its steps in order and always ends with the result's words.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const window = {};
const ctx = vm.createContext({ window, Chronicle: { register() {} } });
for (const f of ['crafting_lib.js', 'crafting_tray.js', 'crafting_turns.js', 'crafting_block.js']) {
  vm.runInContext(readFileSync(join(root, 'internal', 'plugins', 'crafting', 'static', 'js', f), 'utf8'), ctx);
}
vm.runInContext(readFileSync(join(root, 'static', 'js', 'widgets', 'craft_moments.js'), 'utf8'), ctx);
const U = window.CraftingTurns, B = window.CraftingBlock, M = window.CraftMoments;

const D = (over) => ({ character: { name: 'Orrin' }, try: { ready: true }, pack: { items: [{ itemId: 'moss', name: 'Bloodmoss', qty: 4 }, { itemId: 'ess', name: 'Remnant Essence', qty: 1 }] }, ...over });

test('the try fold counts what goes in and waits for a pick', () => {
  const empty = U.tryFold(D(), {}, false);
  assert.match(empty, /At most 0 items/);
  assert.match(empty, /data-act="ttry"[^>]* disabled/);
  const two = U.tryFold(D(), { moss: 2 }, false);
  assert.match(two, /Bloodmoss <small>2 of 4<\/small>/);
  assert.match(two, /aria-pressed="true"/);
  assert.match(two, /At most 2 items/);
  assert.doesNotMatch(two, /data-act="ttry"[^>]* disabled/);
});

test('a spent try says when the next one comes', () => {
  assert.match(U.tryFold(D({ try: { ready: false } }), {}, false), /Orrin has experimented since the last respite\. The next try comes with the next respite\./);
});

test('a near miss shows the hint and no name', () => {
  const res = { outcome: 'close', hint: 'Water drawn at dawn.', cost: 'half', used: ['1 × Bloodmoss'], back: ['1 × Bloodmoss'] };
  const h = U.triedFold(res, 'Orrin', false);
  assert.match(h, /close, but not yet/);
  assert.match(h, /“Water drawn at dawn\.”/);
  assert.match(h, /a failed try uses up half of what went in/);
  assert.match(h, /stays unnamed until it is found/);
  assert.doesNotMatch(h, /tstart/);
  assert.equal(U.triedWords(res, 'Orrin'), 'Something stirs in the flask, then fades. “Water drawn at dawn.”');
});

test('a find names the recipe and who learned it, and offers to start it', () => {
  const res = { outcome: 'found', recipe: 'Moonlit Tonic', recipeId: 'r9', party: true, learned: ['Orrin', 'Saatraaol'], cost: 'used', used: ['2 × Bloodmoss'], back: [] };
  const h = U.triedFold(res, 'Orrin', false);
  assert.match(h, /Orrin found a recipe: Moonlit Tonic\./);
  assert.match(h, /the recipe says the whole party learns it/);
  assert.match(h, /Orrin · Saatraaol/);
  assert.match(h, /data-act="tstart" data-id="r9"/);
  assert.equal(U.triedWords(res, 'Orrin'), 'Orrin found a recipe: Moonlit Tonic. The whole party learns it.');
});

test('while a moment plays, the fold holds only the bench', () => {
  const h = U.triedFold({ outcome: 'nothing', cost: 'used', used: [], back: [] }, 'Orrin', true);
  assert.match(h, /data-moment/);
  assert.doesNotMatch(h, /crp-lines/);
});

test('the copies to break down: each remembered copy, then a plain one', () => {
  const pack = { items: [{ itemId: 'pot', name: 'Healing Potion', qty: 3, kept: [{ id: 5, qty: 1, how: 'made', by: 'Orrin' }, { id: 6, qty: 1, quality: 'fine' }, { id: 7, qty: 1, off: true }] }] };
  const b = U.breakable(pack, 'pot', B.keptOf, B.keptName);
  assert.equal(b.held, 3);
  assert.deepEqual(JSON.parse(JSON.stringify(b.options)), [{ id: 5, name: 'The one made by Orrin' }, { id: 6, name: 'Fine' }, { id: 0, name: 'A plain one' }]);
  const all = U.breakable({ items: [{ itemId: 'pot', name: 'Healing Potion', qty: 1, kept: [{ id: 5, qty: 1, by: 'Orrin', how: 'made' }] }] }, 'pot', B.keptOf, B.keptName);
  assert.equal(all.options.length, 1, 'no plain one when every copy is remembered');
});

test('the break-down fold says what comes back and that it takes no roll', () => {
  const r = { id: 'h', name: 'Healing Potion', makes: 'Elixir', salvage: ['1 × Spring water'] };
  const h = U.breakFold(r, 'Orrin', { held: 1, options: [{ id: 0, name: 'A plain one' }] }, '0', false);
  assert.match(h, /Break down an Elixir/);
  assert.match(h, /1 × Spring water/);
  assert.match(h, /This takes no roll\./);
  assert.equal(U.brokeWords({ back: ['1 × Spring water'] }, 'Orrin'), 'Broken down. 1 × Spring water is back in Orrin’s pack.');
  assert.equal(U.brokeWords({ back: ['2 × Spring water'] }, 'Orrin'), 'Broken down. 2 × Spring water are back in Orrin’s pack.');
});

test('a hand names the helper’s characteristic, and once helped there is no button', () => {
  const p = { id: 'p1', hero: 'Saatraaol', name: 'Healing Potion', kind: 'Recipe', roll: '2d10 + Reason', points: 6, goal: 10 };
  const prog = () => '';
  const open = U.helpRow({ project: p, helped: false }, 'Orrin', 'help', false, prog);
  assert.match(open, /2d10 \+ Orrin’s Reason, at Saatraaol’s bench/);
  assert.match(open, /data-act="hgo" data-id="p1"/);
  const done = U.helpRow({ project: p, helped: true }, 'Orrin', 'help', false, prog);
  assert.match(done, /Orrin helped this respite/);
  assert.doesNotMatch(done, /data-act="help"/);
});

// A cue whose later() runs on a fake clock, in time order, recording what
// happened.
function fakeCue(calm) {
  const log = [], at = [];
  let now = 0;
  const fx = {};
  for (const k of ['drop', 'fizz', 'steam', 'smoke', 'success', 'dust', 'spark']) fx[k] = () => log.push(k);
  return {
    log, calm,
    cue: { P: {}, fx, L: { flask: { x: 1, y: 2, neck: 3 }, mortar: { x: 4, y: 5 } }, calm,
      say: (t, btns) => log.push('say:' + t + (btns && btns.length ? ' [' + btns.map((b) => b.label).join(',') + ']' : '')),
      sfx: () => {}, later: (fn, ms) => at.push([now + ms, fn]) },
    run() { while (at.length) { at.sort((a, b) => a[0] - b[0]); const [t, fn] = at.shift(); now = t; fn(); } },
  };
}

test('each moment plays its own ending and finishes on the result’s words', () => {
  const ends = { found: 'success', close: 'smoke', nothing: 'smoke', breakdown: 'dust' };
  for (const kind of Object.keys(ends)) {
    const f = fakeCue(false);
    let done = 0;
    M.play(f.cue, { kind, first: 'start', last: 'the end', buttons: kind === 'found' ? [{ label: 'Start it', fn() {} }] : [], done: () => done++ });
    f.run();
    assert.equal(f.log[0], 'say:start', kind);
    assert.ok(f.log.includes(ends[kind]), kind + ' plays ' + ends[kind]);
    assert.equal(f.log[f.log.length - 1], 'say:the end' + (kind === 'found' ? ' [Start it]' : ''), kind);
    assert.equal(done, 1, kind);
  }
  const help = fakeCue(false);
  M.play(help.cue, { kind: 'help', finished: false, last: 'x' });
  help.run();
  assert.ok(!help.log.includes('success'), 'an unfinished project gets no success burst');
});

test('under Calm a moment shows its words at once', () => {
  const f = fakeCue(true);
  let done = 0;
  M.play(f.cue, { kind: 'found', first: 'start', last: 'the end', done: () => done++ });
  assert.deepEqual(f.log, ['say:the end']);
  assert.equal(done, 1);
});
