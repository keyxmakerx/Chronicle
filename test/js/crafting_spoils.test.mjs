// crafting_spoils.test.mjs — the spoils tray's cards and the Director's
// Spoils tab: internal/plugins/crafting/static/js/crafting_tray.js and
// crafting_spoils.js.
//
// Pins what the tray shows from what the server sends: every word escaped,
// an unseen item in its own words with no kinds, the copies of a creature
// numbered, a harvest's three tiers with the one the test landed in lit, a
// harvest that paid nothing said so, a find waiting for a study shown as
// "Something unidentified" to a player and named to the Director, a study's
// result, and the drop list editor's words and amounts. Runs the scripts in
// a vm.

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

const study = { roll: '2d10 + Reason', tiers: ['11 or less', '12 to 16', '17 or more'], says: ['not yet', 'learn what it is', 'and which recipes use it'] };
const remnant = (line) => ({ id: 's2', source: 'Troll Remnant', origin: 'Bridge', at: '2026-10-28T10:00:00Z', lines: [line] });

test('a player reads a find waiting for a study as Something unidentified, with no item and its study’s outcomes', () => {
  const l = { id: 11, name: 'Something unidentified', tags: [], mode: 'take', qty: 2, locked: true, study: { ...study, tried: ['Mira Ashdown rolled 8'], wait: 'Mira Ashdown tries again after the next respite' } };
  const h = T.card(remnant(l), { acts, now: at });
  assert.match(h, /class="crs-line is-locked"/);
  assert.match(h, /<b class="is-unknown">Something unidentified × 2<\/b><span class="cra-fine">Study it to learn what it is<\/span>/);
  assert.match(h, /Study: 2d10 \+ Reason<\/span>/);
  assert.match(h, /11 or less <b>not yet<\/b>.*12 to 16 <b>learn what it is<\/b>.*17 or more <b>and which recipes use it<\/b>/);
  assert.match(h, /Mira Ashdown rolled 8, so not yet/);
  assert.doesNotMatch(h, /Not identified yet|Take as it is|Harvest:/);
  assert.equal(T.locked([remnant(l), troll]), 1);
});

test('a one chance find says a miss loses it, and a harvest’s amount stays hidden from a player', () => {
  const l = { id: 12, name: 'Something unidentified', tags: [], mode: 'harvest', locked: true, study: { ...study, once: true, says: ['lose it', 'learn what it is', 'and which recipes use it'] } };
  const h = T.card(remnant(l), { acts, now: at });
  assert.match(h, /<b class="is-unknown">Something unidentified<\/b><span class="cra-fine">One chance: a miss loses it<\/span>/);
  assert.match(h, /Study: 2d10 \+ Reason · one chance/);
  assert.match(h, /11 or less <b>lose it<\/b>/);
});

test('the Director sees the find by name, with its study and that the party doesn’t know it yet', () => {
  const l = { id: 13, itemId: 'ichor', name: 'Grave ichor', tags: ['ichor'], kinds: 'ichor', mode: 'harvest', roll: '2d10 + Intuition', tiers, locked: true, study };
  const h = T.card(remnant(l), { acts, now: at });
  assert.match(h, /<b>Grave ichor<\/b><span class="cra-fine">ichor<\/span>/);
  assert.match(h, /Study: 2d10 \+ Reason · <span class="crp-pill is-warn">Not identified yet<\/span>/);
  assert.doesNotMatch(h, /is-locked|Something unidentified/);
});

test('an identified find says who identified it and which recipes use it', () => {
  const l = { id: 14, itemId: 'ichor', name: 'Grave ichor', tags: ['ichor'], kinds: 'ichor', mode: 'take', qty: 1, identified: 'identified by Tobin Reed', usedIn: ['Troll-blood tonic'] };
  const h = T.card(remnant(l), { acts, now: at });
  assert.match(h, /<span class="cra-fine">ichor<\/span><span class="cra-fine crs-knew"><svg[^]*?<\/svg>identified by Tobin Reed<\/span><span class="cra-fine">Used in Troll-blood tonic<\/span>/);
  assert.match(h, /Take as it is/);
});

test('a study’s lowest outcome lights as a miss', () => {
  const h = T.card(remnant({ id: 15, name: 'Something unidentified', tags: [], mode: 'take', qty: 1, locked: true, study }), { acts, now: at, hit: () => 0 });
  assert.match(h, /class="crs-tier is-hit is-miss">11 or less/);
});

const studiedBase = { hero: 'Tobin Reed', dice: [9, 6], sides: 10, terms: [{ text: '2d10', n: 15 }, { text: 'Reason, from Tobin Reed’s sheet', n: 2 }], total: 17, tiers: study.tiers, mode: 'harvest', notes: [] };

test('a landed study names the item and, on the top outcome, the recipes that use it', () => {
  const h = T.studied({ ...studiedBase, tier: 2, known: true, item: 'Grave ichor', kinds: 'ichor', usedIn: ['Troll-blood tonic'] }, 13);
  assert.match(h, /<h4>Study<\/h4>/);
  assert.match(h, /<li class="is-tot"><span>The test<\/span><span>17<\/span><\/li>/);
  assert.match(h, /17 or more\./);
  assert.match(h, /Tobin Reed knows it: Grave ichor \(ichor\)\.<small>Every player sees it now, and it can be harvested\. It’s used in Troll-blood tonic, a recipe the party knows\.<\/small>/);
  assert.match(h, /Rolled on Chronicle’s server, and listed in the Director’s Taken lately\./);
  assert.match(h, /data-act="sdone" data-id="13"/);
  // The top outcome with no recipe that uses it says so.
  assert.match(T.studied({ ...studiedBase, tier: 2, known: true, item: 'Grave ichor', mode: 'take' }, 13), /it can be taken\. No recipe the party knows uses it\./);
  assert.doesNotMatch(T.studied({ ...studiedBase, total: 13, tier: 1, known: true, item: 'Grave ichor' }, 13), /recipe/);
});

test('a missed study says it stays unidentified, or that a one chance find is lost', () => {
  const miss = T.studied({ ...studiedBase, total: 8, tier: 0, known: false, notes: ['Reason isn’t a number on Tobin Reed’s sheet, so it added nothing.'] }, 13);
  assert.match(miss, /crp-made is-warn/);
  assert.match(miss, /Tobin Reed can’t make it out\. It stays unidentified\.<small>Another hero can still try, and Tobin Reed can try again after the next respite\.<\/small>/);
  assert.match(miss, /Reason isn’t a number on Tobin Reed’s sheet, so it added nothing\. Rolled on/);
  const lost = T.studied({ ...studiedBase, total: 8, tier: 0, known: false, lost: true }, 13);
  assert.match(lost, /and that was its one chance\. The party leaves it behind\.<small>Nobody will learn what it was\. It leaves the tray for everyone\.<\/small>/);
});

test('a study’s result escapes what it was sent', () => {
  const h = T.studied({ ...studiedBase, tier: 1, known: true, hero: '<b>Mira</b>', item: '<i>x</i>', kinds: '"a"' }, 13);
  assert.doesNotMatch(h, /<b>Mira|<i>x/);
  assert.match(h, /&lt;b&gt;Mira&lt;\/b&gt; knows it: &lt;i&gt;x&lt;\/i&gt;/);
});

test('the tray’s line and a drop list’s row count what needs study', () => {
  const locked = { lines: [{ locked: true }, { locked: true }] };
  assert.match(S.trayLine([troll, locked]), /^5 things from 2 sources, waiting until someone takes them or you clear them\. 2 are not identified yet: players see “Something unidentified” until a hero studies it or you reveal it\.$/);
  assert.match(S.trayLine([{ lines: [{ locked: true }] }]), /One is not identified yet/);
  assert.equal(S.dropWords({ lines: [{ study: true }, {}], also: 'troll', alsoName: 'Troll' }), '2 lines, 1 needing study, plus what Troll drops');
});

test('a study mark survives a change of mode, and a new study adds Reason or Intelligence', () => {
  const stats = [{ key: 'might', label: 'Might' }, { key: 'reason', label: 'Reason' }];
  assert.deepEqual(plain(S.lineAs({ itemId: 'hide', mode: 'take', qty: 2, study: true, studyStat: 'might', once: true }, 'harvest', stats)),
    { itemId: 'hide', mode: 'harvest', stat: 'might', yields: [1, 2, 3], study: true, studyStat: 'might', once: true });
  assert.deepEqual(plain(S.lineAs({ itemId: 'hide', mode: 'harvest', stat: 'might', yields: [1, 2, 3], study: true }, 'take', stats)), { itemId: 'hide', mode: 'take', qty: 1, study: true, studyStat: '' });
  assert.equal(S.studyStat(stats), 'reason');
  assert.equal(S.studyStat([{ key: 'str', label: 'Strength' }, { key: 'int', label: 'Intelligence' }]), 'int');
  assert.equal(S.studyStat([{ key: 'might', label: 'Might' }]), 'might');
  assert.equal(S.studyStat([]), '');
});

test('the editor says what a study has to roll, from the system’s tiers', () => {
  assert.equal(S.lands(['11 or less', '12 to 16', '17 or more']), 'rolls 12 or more');
  assert.equal(S.lands(['Fail', 'Pass']), 'studies it successfully');
  assert.equal(S.lands(undefined), 'studies it successfully');
});
