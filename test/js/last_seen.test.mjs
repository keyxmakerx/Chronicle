// last_seen.test.mjs — pins the "Last seen" block's pure parts: the "N weeks
// ago" rule (the same cases as sessions.AgoText in Go), row titles and source
// words, newest-first order, the line's text, the "Every night" merge, and the
// strip's squeeze into blocks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'sessions', 'static', 'js', 'last_seen.js'), 'utf8');
const sandbox = { module: { exports: {} } };
vm.runInNewContext(src, sandbox);
const L = sandbox.module.exports;

// Local noon, so the day count does not depend on the machine's zone.
// Values built inside the vm context have another realm's prototypes, so they are
// compared through JSON.
const J = (x) => JSON.parse(JSON.stringify(x));

const NOW = new Date(2026, 9, 17, 12).getTime();

test('motionOf: reduced motion is always off; otherwise the page attribute decides', () => {
  const cases = [
    { attr: null, reduced: false, want: 'full' },
    { attr: 'calm', reduced: false, want: 'calm' },
    { attr: 'off', reduced: false, want: 'off' },
    { attr: 'calm', reduced: true, want: 'off' },
    { attr: 'bogus', reduced: false, want: 'full' }
  ];
  for (const c of cases) assert.equal(L.motionOf(c.attr, c.reduced), c.want, JSON.stringify(c));
});

test('agoText: days, weeks, months, years; the same cases as the Go test', () => {
  const cases = [
    ['2026-10-18', 'coming up'],
    ['2026-10-17', 'today'],
    ['2026-10-16', 'yesterday'],
    ['2026-10-10', '7 days ago'],
    ['2026-10-03', '2 weeks ago'],
    ['2026-09-12', '5 weeks ago'],
    ['2026-08-22', '8 weeks ago'],
    ['2026-08-15', '2 months ago'],
    ['2025-10-17', '12 months ago'],
    ['2023-10-17', '3 years ago'],
    ['', ''],
    ['nonsense', '']
  ];
  for (const [date, want] of cases) assert.equal(L.agoText(date, NOW), want, date);
});

test('fmtDate and nightsLabel', () => {
  assert.equal(L.fmtDate('2026-10-03'), '3 Oct 2026');
  assert.equal(L.fmtDate(''), '');
  assert.equal(L.nightsLabel(0), '0 nights');
  assert.equal(L.nightsLabel(1), '1 night');
  assert.equal(L.nightsLabel(6), '6 nights');
});

test('newestFirst: latest night first, a series with no night last, then by name', () => {
  const list = [
    { sessionId: 'a', sessionName: 'Series', date: '' },
    { sessionId: 'b', sessionName: 'Old', date: '2026-01-01' },
    { sessionId: 'c', sessionName: 'Zed', date: '2026-09-01' },
    { sessionId: 'd', sessionName: 'Amy', date: '2026-09-01' },
    { sessionId: 'e', sessionName: 'Newest', date: '2026-10-10' }
  ];
  const got = L.newestFirst(list).map((x) => x.sessionId);
  assert.deepEqual(J(got), ['e', 'd', 'c', 'b', 'a']);
  assert.equal(list[0].sessionId, 'a', 'the input is not reordered');
});

test('rowTitle: a series with no night says so', () => {
  assert.equal(L.rowTitle({ sessionName: 'The Drowned Bell', date: '2026-09-12' }), 'The Drowned Bell');
  assert.equal(L.rowTitle({ sessionName: 'Weekly game', date: '' }), 'Seen during Weekly game, night not set');
});

test('sourceText: where a sighting came from', () => {
  const cases = [
    { it: { source: 'link' }, dm: true, user: 'u1', want: 'From the game night' },
    { it: { source: 'checkin', addedBy: 'u1' }, dm: true, user: 'u1', want: 'From the game night check-in' },
    { it: { source: 'manual', addedBy: 'u1' }, dm: true, user: 'u1', want: 'Added by you' },
    { it: { source: 'manual', addedBy: 'u2' }, dm: true, user: 'u1', want: 'Added by the DM' },
    { it: { source: 'manual' }, dm: false, user: 'u1', want: 'From the game night' },
    { it: { source: 'checkin' }, dm: false, user: 'u1', want: 'From the game night' },
    { it: { source: 'checkin' }, dm: true, user: 'u1', want: 'From the game night check-in' }
  ];
  for (const c of cases) assert.equal(L.sourceText(c.it, c.dm, c.user), c.want, JSON.stringify(c));
});

test('isStored: only a checked-in or hand-added row can be cleared', () => {
  assert.equal(L.isStored({ source: 'manual' }), true);
  assert.equal(L.isStored({ source: 'checkin' }), true);
  assert.equal(L.isStored({ source: 'link' }), false);
});

test('lineParts: the newest sighting leads', () => {
  assert.deepEqual(J(L.lineParts([], NOW)), { what: 'Not seen yet', when: '' });
  const seen = [
    { sessionName: 'Old', date: '2026-01-01' },
    { sessionName: 'The Drowned Bell', date: '2026-09-12' }
  ];
  assert.deepEqual(J(L.lineParts(seen, NOW)), { what: 'The Drowned Bell', when: '5 weeks ago' });
  assert.deepEqual(J(L.lineParts([{ sessionName: 'Weekly game', date: '' }], NOW)), { what: 'during Weekly game, night not set', when: '' });
});

test('suggestLabel: names the page and agrees with the count', () => {
  assert.equal(L.suggestLabel(1, 'Brother Ansel'), '1 night mentions Brother Ansel. Add?');
  assert.equal(L.suggestLabel(2, 'Brother Ansel'), '2 nights mention Brother Ansel. Add?');
});

test('allRows: every night, seen ones kept, newest first', () => {
  const nights = [
    { sessionId: 's', sessionName: 'Weekly', date: '2026-10-10' },
    { sessionId: 's', sessionName: 'Weekly', date: '2026-10-03' },
    { sessionId: 's', sessionName: 'Weekly', date: '2026-09-26' }
  ];
  const seen = [
    { sessionId: 's', sessionName: 'Weekly', date: '2026-10-03', source: 'manual' },
    { sessionId: 'x', sessionName: 'Series', date: '', source: 'link' }
  ];
  const rows = L.allRows(nights, seen).map((r) => r.state + ':' + r.it.sessionId + '|' + r.it.date);
  assert.deepEqual(J(rows), ['none:s|2026-10-10', 'seen:s|2026-10-03', 'none:s|2026-09-26', 'seen:x|']);
});

test('stripBlocks: one cell per night until they outnumber the room, then blocks', () => {
  const flags = [true, false, true, false, false, true];
  assert.deepEqual(J(L.stripBlocks(flags, 10).map((b) => [b.from, b.to, b.count])), [[0, 1, 1], [1, 2, 0], [2, 3, 1], [3, 4, 0], [4, 5, 0], [5, 6, 1]]);
  const squeezed = L.stripBlocks(flags, 3);
  assert.deepEqual(J(squeezed.map((b) => [b.from, b.to, b.count])), [[0, 2, 1], [2, 4, 1], [4, 6, 1]]);
  assert.equal(squeezed.reduce((n, b) => n + b.size, 0), flags.length, 'every night lands in a block');
  assert.deepEqual(J(L.stripBlocks([], 5)), []);
});

test('blockLevel: darker with the share of nights seen', () => {
  assert.equal(L.blockLevel(0, 4), 0);
  assert.equal(L.blockLevel(1, 1), 3);
  assert.equal(L.blockLevel(1, 4), 1);
  assert.equal(L.blockLevel(2, 4), 2);
  assert.equal(L.blockLevel(4, 4), 3);
});

test('escapeShouldClose: Escape closes, unless a modifier is held', () => {
  assert.equal(L.escapeShouldClose({ key: 'Escape' }), true);
  assert.equal(L.escapeShouldClose({ key: 'Escape', ctrlKey: true }), false);
  assert.equal(L.escapeShouldClose({ key: 'a' }), false);
  assert.equal(L.escapeShouldClose(null), false);
});

test('nightPageURL: a dated night opens its day; a series with none opens its own page', () => {
  assert.equal(L.nightPageURL('c 1', { sessionId: 's1', date: '2026-10-10' }), '/campaigns/c%201/game-nights?session=s1&date=2026-10-10');
  assert.equal(L.nightPageURL('c1', { sessionId: 's1', date: '' }), '/campaigns/c1/game-nights?session=s1');
});
