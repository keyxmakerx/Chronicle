// rumours.js: the pure rules behind the Rumours block, which are what is
// shown folded or unfolded, what each filter means to each viewer, the words
// on the joined truth/belief mark, and what may be saved.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'rumours', 'static', 'js', 'rumours.js'), 'utf8');

function load() {
  const sandbox = { module: { exports: {} } };
  vm.runInNewContext(src, sandbox);
  return sandbox.module.exports;
}
const R = load();
// Values built inside the vm context have another realm's prototypes; deepEqual needs plain ones.
const plain = (x) => JSON.parse(JSON.stringify(x));

const make = (n, over) => Array.from({ length: n }, (_, i) => Object.assign({ id: 'r' + i, belief: '', truth: 'open' }, over && over(i)));

test('motionOf: a device asking for less motion is always off', () => {
  const cases = [
    [undefined, false, 'full'], ['calm', false, 'calm'], ['off', false, 'off'],
    ['calm', true, 'off'], ['anything', false, 'full'],
  ];
  for (const [attr, reduced, want] of cases) assert.equal(R.motionOf(attr, reduced), want, `${attr}/${reduced}`);
});

test('viewKind: the Director, a Scribe, a player', () => {
  assert.equal(R.viewKind(true, true), 'dm');
  assert.equal(R.viewKind(false, true), 'scribe');
  assert.equal(R.viewKind(false, false), 'player');
});

test('filterSet: only the Director is offered filters on the truth', () => {
  const dm = R.filterSet('dm').map((f) => f[0]);
  assert.deepEqual(plain(dm), ['all', 'open', 'true', 'false']);
  for (const kind of ['scribe', 'player']) {
    assert.deepEqual(plain(R.filterSet(kind).map((f) => f[0])), ['all', 'true', 'false', 'unsure']);
  }
});

test('filterValue: a Scribe or player never filters on a truth they were not sent', () => {
  const it = { truth: 'true', belief: 'false' };
  assert.equal(R.filterValue('dm', it), 'true');
  assert.equal(R.filterValue('scribe', it), 'false');
  assert.equal(R.filterValue('player', { belief: '' }), 'none');
  assert.equal(R.filterValue('player', { truth: 'true', belief: '' }), 'none');
  assert.equal(R.filterValue('dm', {}), 'open');
});

test('wanted: folded shows the newest two; unfolded shows the rest', () => {
  const cases = [
    { name: 'folded, short', items: make(5), st: { unfolded: false, filter: 'all', shown: 0 }, list: 2, left: 3 },
    { name: 'folded, one', items: make(1), st: { unfolded: false, filter: 'all', shown: 0 }, list: 1, left: 0 },
    { name: 'unfolded short list shows all', items: make(5), st: { unfolded: true, filter: 'all', shown: 0 }, list: 5, left: 0 },
    { name: 'unfolded long list pages', items: make(120), st: { unfolded: true, filter: 'all', shown: 10 }, list: 12, left: 108 },
    { name: 'unfolded long list, second page', items: make(120), st: { unfolded: true, filter: 'all', shown: 20 }, list: 22, left: 98 },
    { name: 'a filter applies only unfolded', items: make(6, (i) => ({ truth: i % 2 ? 'true' : 'open' })), st: { unfolded: false, filter: 'true', shown: 0 }, list: 2, left: 4 },
    { name: 'unfolded filter', items: make(6, (i) => ({ truth: i % 2 ? 'true' : 'open' })), st: { unfolded: true, filter: 'true', shown: 0 }, list: 3, left: 0 },
  ];
  for (const tc of cases) {
    const got = R.wanted(tc.items, 'dm', tc.st);
    assert.equal(got.list.length, tc.list, `${tc.name}: list`);
    assert.equal(got.left, tc.left, `${tc.name}: left`);
  }
});

test('paged: the earlier rumours come in pages only from a long list', () => {
  assert.equal(R.paged(23), false);
  assert.equal(R.paged(24), true);
});

test('pairState: the joined mark follows how truth and belief compare', () => {
  const cases = [
    ['true', '', 'none'], ['open', '', 'none'],
    ['true', 'true', 'agree'], ['false', 'false', 'agree'],
    ['true', 'false', 'differ'], ['false', 'true', 'differ'],
    ['open', 'true', 'marked'], [undefined, 'false', 'marked'],
    ['true', 'unsure', 'differ'],
  ];
  for (const [t, b, want] of cases) assert.equal(R.pairState(t, b), want, `${t}/${b}`);
});

test('pairTip: says who marked it, and never invents a name', () => {
  assert.equal(R.pairTip({ truth: 'true', belief: 'false', beliefBy: 'Tomas' }), 'You: true. Party (marked by Tomas): false.');
  assert.equal(R.pairTip({ truth: 'false', belief: 'true' }), 'You: false. Party (marked by a player): true.');
  assert.equal(R.pairTip({}), 'You: open. Party: not marked yet.');
});

test('tailLabel: the fold button counts what is hidden and what is left', () => {
  assert.equal(R.tailLabel('open', { earlier: 1 }), '1 earlier rumour');
  assert.equal(R.tailLabel('open', { earlier: 118 }), '118 earlier rumours');
  assert.equal(R.tailLabel('close', {}), 'Show fewer');
  assert.equal(R.tailLabel('more', { filter: 'all', shown: 12, total: 120, left: 108 }), '10 of 118 earlier rumours · Show 10 more (108 left)');
  assert.equal(R.tailLabel('more', { filter: 'all', shown: 118, total: 120, left: 4 }), '116 of 118 earlier rumours · Show the last 4');
  assert.equal(R.tailLabel('more', { filter: 'true', shown: 10, total: 40, left: 30 }), '10 of 40 true rumours · Show 10 more (30 left)');
});

test('cleanText and validate: the rules the server applies, said before the trip', () => {
  assert.deepEqual(plain(R.cleanText('  a   b \n c ', 280)), { text: 'a b c', ok: true });
  assert.equal(R.cleanText('bad\u0007char', 280).ok, false);
  assert.equal(R.cleanText('x'.repeat(281), 280).ok, false);
  assert.equal(R.cleanText('x'.repeat(280), 280).ok, true);
  // Counted in characters, not UTF-16 units.
  assert.equal(R.cleanText('\u{1F600}'.repeat(280), 280).ok, true);

  const cases = [
    ['', 'Bram', 'Write what was said first.'],
    ['   ', '', 'Write what was said first.'],
    ['bad\u0007', '', 'Use plain text for the rumour.'],
    ['x'.repeat(281), '', 'Keep the rumour to 280 characters or fewer.'],
    ['Fine.', 'w'.repeat(81), 'Keep who said it to 80 characters or fewer, in plain text.'],
    ['Fine.', '', ''],
    ['Fine.', 'Old Bram', ''],
  ];
  for (const [q, w, want] of cases) assert.equal(R.validate(q, w), want, `${q.slice(0, 8)}/${w.slice(0, 8)}`);
});

test('sureWord: out of range reads as the middle step', () => {
  assert.deepEqual(plain([1, 2, 3, 0, 4, 1.5, undefined].map(R.sureWord)),
    ['Heard it', 'Fairly sure', 'Swears by it', 'Fairly sure', 'Fairly sure', 'Fairly sure', 'Fairly sure']);
});

test('escapeShouldClose: Escape alone', () => {
  assert.equal(R.escapeShouldClose({ key: 'Escape' }), true);
  assert.equal(R.escapeShouldClose({ key: 'Escape', ctrlKey: true }), false);
  assert.equal(R.escapeShouldClose({ key: 'a' }), false);
  assert.equal(R.escapeShouldClose(null), false);
});

test('replaceItem and dropItem: a newer answer replaces in place; a new one goes first', () => {
  const items = [{ id: 'a', quote: '1' }, { id: 'b', quote: '2' }];
  assert.deepEqual(plain(R.replaceItem(items, { id: 'b', quote: 'B' }).map((i) => i.quote)), ['1', 'B']);
  assert.deepEqual(plain(R.replaceItem(items, { id: 'c', quote: 'C' }).map((i) => i.id)), ['c', 'a', 'b']);
  assert.deepEqual(items.map((i) => i.id), ['a', 'b']);
  assert.deepEqual(plain(R.dropItem(items, 'a').map((i) => i.id)), ['b']);
  assert.deepEqual(plain(R.dropItem(null, 'a')), []);
});

test('cutNotice: only when a bound cut the list off', () => {
  assert.equal(R.cutNotice(200, 200), '');
  assert.equal(R.cutNotice(350, 200), 'Showing the newest 200 of 350 rumours on this page.');
});
