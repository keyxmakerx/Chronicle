// crafting_spoils.test.mjs — the spoils tray's cards and the Director's
// Spoils tab: internal/plugins/crafting/static/js/crafting_tray.js and
// crafting_spoils.js.
//
// Pins what the tray shows from what the server sends: every word escaped,
// an unseen item in its own words with no kinds, the copies of a creature
// numbered, a harvest's three tiers with the one the test landed in lit, a
// harvest that paid nothing said so, and the drop list editor's words and
// amounts. Runs the scripts in a vm.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const js = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'internal', 'plugins', 'crafting', 'static', 'js');
const window = {};
const ctx = vm.createContext({ window, Chronicle: { register() {} } });
for (const f of ['crafting_lib.js', 'crafting_tray.js', 'crafting_spoils.js']) vm.runInContext(readFileSync(join(js, f), 'utf8'), ctx);
const T = window.CraftingTray, S = window.CraftingSpoils;
const plain = (v) => JSON.parse(JSON.stringify(v));

const tiers = [{ label: '11 or less', qty: 1 }, { label: '12 to 16', qty: 2 }, { label: '17 or more', qty: 4 }];
const troll = {
  id: 's1', source: 'Troll', copy: 2, copies: 3, origin: 'Shrine cellar', at: '2026-10-28T10:00:00Z',
  lines: [
    { id: 7, itemId: 'tooth', name: 'Troll tooth', tags: ['bone'], kinds: 'bone', mode: 'take', qty: 2 },
    { id: 8, itemId: 'hide', name: 'Troll hide', tags: ['hide'], kinds: 'hide', mode: 'harvest', roll: '2d10 + Might', tiers },
    { id: 9, name: 'An item you haven’t seen', unseen: true, tags: [], mode: 'take', qty: 1 },
  ],
};
const acts = (l) => '<button data-line-act="' + l.id + '">Take</button>';
const at = Date.parse('2026-10-28T10:00:00Z');

test('how long ago reads as a person says it', () => {
  const cases = [[0, 'just now'], [5, '5 minutes ago'], [60, '1 hour ago'], [180, '3 hours ago'], [60 * 30, 'yesterday'], [60 * 24 * 4, '4 days ago']];
  for (const [mins, want] of cases) assert.equal(T.ago('2026-10-28T10:00:00Z', at + mins * 60000), want, mins + ' minutes');
  assert.equal(T.ago('', at), '');
});

test('a spoil says where it came from, or that it was put in by hand, and only when to a player who can’t see it', () => {
  assert.equal(T.from(troll, at + 10 * 60000), 'Shrine cellar, 10 minutes ago');
  assert.equal(T.from({ at: troll.at, byHand: true }, at), 'Put in by hand, just now');
  // A player who can't see where it came from is told only when.
  assert.equal(T.from({ at: troll.at }, at), 'just now');
});

test('the tray counts its lines', () => {
  assert.equal(T.count([troll, { lines: [{}] }]), 4);
  assert.equal(T.count(null), 0);
});

test('a card numbers the copies and lists each line with the caller’s buttons', () => {
  const h = T.card(troll, { acts, now: at });
  assert.match(h, /data-spoil="s1"/);
  assert.match(h, /<b>Troll<small>\(2 of 3\)<\/small><\/b>/);
  assert.match(h, /Troll tooth × 2<\/b><span class="cra-fine">bone<\/span>/);
  assert.match(h, /Take as it is/);
  assert.match(h, /Harvest: 2d10 \+ Might/);
  assert.equal((h.match(/data-line-act=/g) || []).length, 3);
  // A harvest has no amount until the test pays one.
  assert.doesNotMatch(h, /Troll hide ×/);
  assert.doesNotMatch(T.card({ ...troll, copy: 1, copies: 1 }, { acts }), /<small>/);
});

test('an unseen item is named in its own words and shows no kinds', () => {
  const h = T.card(troll, { acts });
  assert.match(h, /<b class="is-unseen">An item you haven’t seen × 1<\/b><span class="cra-fine">Whoever takes it gets to see it<\/span>/);
});

test('a harvest’s tiers light the one the test landed in', () => {
  const h = T.card(troll, { acts, hit: (l) => (l.id === 8 ? 1 : -1) });
  assert.match(h, /<span class="crs-tier">11 or less <b>1<\/b><\/span><span class="crs-tier is-hit">12 to 16 <b>2<\/b><\/span><span class="crs-tier">17 or more <b>4<\/b><\/span>/);
  assert.equal((T.card(troll, { acts }).match(/is-hit/g) || []).length, 0);
});

test('a card escapes every word it was sent', () => {
  const bad = { ...troll, source: '<img src=x onerror=alert(1)>', origin: '"><script>', lines: [{ ...troll.lines[0], name: '<b>x</b>', kinds: '<i>' }] };
  const h = T.card(bad, { acts: () => '' });
  assert.doesNotMatch(h, /<img|<script|<b>x|<i>/i);
  assert.match(h, /&lt;img src=x onerror=alert\(1\)&gt;/);
});

test('the card’s own buttons, what folds under its head and under a line go where they belong', () => {
  const h = T.card(troll, { acts, head: () => '<i data-head></i>', under: () => '<i data-under></i>', fold: (l) => (l.id === 7 ? '<i data-fold></i>' : '') });
  assert.ok(h.indexOf('data-head') < h.indexOf('</div><i data-under>'), 'the head’s buttons sit in the head');
  assert.ok(h.indexOf('data-under') < h.indexOf('<ul class="crs-lines">'), 'the fold under the head comes before the lines');
  assert.ok(h.indexOf('data-fold') > h.indexOf('data-line="7"') && h.indexOf('data-fold') < h.indexOf('data-line="8"'), 'a line’s fold sits in its own line');
});

const harvested = { hero: 'Mira', item: 'Troll hide', got: '2 × Troll hide', qty: 2, dice: [6, 5], sides: 10, total: 13, tier: 1, tiers,
  terms: [{ text: '2d10', n: 11 }, { text: 'Might, from Mira’s sheet', n: 2 }], notes: [] };

test('a harvest’s result shows the test, the tier and what reached the pack', () => {
  const h = T.result(harvested, 8, 'Harvest: Troll hide');
  assert.match(h, /<h4>Harvest: Troll hide<\/h4>/);
  assert.match(h, /<span class="crp-die">6<\/span><span class="crp-die">5<\/span>/);
  assert.match(h, /<li><span>2d10<\/span><span>11<\/span><\/li><li><span>Might, from Mira’s sheet<\/span><span>\+2<\/span><\/li>/);
  assert.match(h, /<li class="is-tot"><span>The test<\/span><span>13<\/span><\/li>/);
  assert.match(h, /12 to 16\./);
  assert.match(h, /2 × Troll hide went to Mira’s pack\./);
  assert.match(h, /The line leaves the tray for everyone\./);
  assert.match(h, /data-act="sdone" data-id="8"/);
});

test('a harvest in the lowest tier names it, though the server leaves a 0 tier out', () => {
  const low = { ...harvested, qty: 1, got: '1 × Troll hide', total: 9 };
  delete low.tier;
  assert.match(T.result(low, 8, 'Harvest'), /<p class="cra-fine">11 or less: the poorest outcome, which still pays\.<\/p>/);
});

test('a harvest that paid nothing says so once, and keeps the other notes', () => {
  const h = T.result({ ...harvested, qty: 0, got: 'nothing', tier: 0, notes: ['Mira’s sheet has no number for Might, so it adds +0.', 'That tier pays nothing for this one.'] }, 8, 'Harvest');
  assert.equal((h.match(/pays nothing/g) || []).length, 1);
  assert.match(h, /crp-made is-warn/);
  assert.match(h, /Mira’s sheet has no number for Might, so it adds \+0\. The line leaves the tray for everyone\./);
});

test('terms show the dice as rolled and every other term signed', () => {
  assert.deepEqual(plain(T.terms({ terms: [{ text: '2d10', n: 9 }, { text: 'Might', n: -1 }, { text: 'Agility', n: 0 }] })).map((r) => r.n), ['9', '−1', '0']);
});

test('the tray’s line counts things and sources', () => {
  assert.equal(S.trayLine([]), 'Nothing is waiting.');
  assert.equal(S.trayLine([troll]), '3 things from 1 source, waiting until someone takes them or you clear them.');
  assert.equal(S.trayLine([troll, { lines: [{}] }]), '4 things from 2 sources, waiting until someone takes them or you clear them.');
});

test('a drop list’s row says its lines and the page it also drops from', () => {
  assert.equal(S.dropWords({ lines: [{}] }), '1 line');
  assert.equal(S.dropWords({ lines: [{}, {}], also: 'troll', alsoName: 'Troll' }), '2 lines, plus what Troll drops');
  assert.equal(S.dropWords({ lines: [], also: 'gone' }), '0 lines, plus what another page drops');
});

test('switching a line’s mode keeps the item and starts a harvest at 1, 2 and 3', () => {
  const stats = [{ key: 'might', label: 'Might' }, { key: 'agility', label: 'Agility' }];
  assert.deepEqual(plain(S.lineAs({ itemId: 'hide', mode: 'take', qty: 3 }, 'harvest', stats)), { itemId: 'hide', mode: 'harvest', stat: 'might', yields: [1, 2, 3] });
  assert.deepEqual(plain(S.lineAs({ itemId: 'hide', mode: 'harvest', stat: 'agility', yields: [2, 3, 5] }, 'harvest', stats)), { itemId: 'hide', mode: 'harvest', stat: 'agility', yields: [2, 3, 5] });
  assert.deepEqual(plain(S.lineAs({ itemId: 'hide', mode: 'harvest', stat: 'might', yields: [1, 2, 3] }, 'take', stats)), { itemId: 'hide', mode: 'take', qty: 1 });
  // No game system: the test adds nothing.
  assert.equal(S.lineAs({ itemId: 'hide' }, 'harvest', []).stat, '');
});

test('typed amounts stay inside what the server takes', () => {
  const cases = [['3', 1, 3], ['0', 1, 1], ['0', 0, 0], ['', 1, 1], ['junk', 0, 0], ['2.6', 0, 3], ['500', 1, 99], ['-4', 0, 0]];
  for (const [v, least, want] of cases) assert.equal(S.amount(v, least), want, JSON.stringify([v, least]));
});
