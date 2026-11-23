// cutscene_stage.test.mjs — which showing plays by itself when a page opens.

import test from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const { pickRecent, rowsOf, RECENT_MS } = createRequire(import.meta.url)('../../static/js/widgets/cutscene_stage.js');

const NOW = Date.parse('2026-11-23T20:00:00Z');
const ago = (ms) => new Date(NOW - ms).toISOString();
const row = (id, shownMsAgo, extra) => ({ showingId: id, name: id, shownAt: ago(shownMsAgo), rewatchUntil: ago(-3600e3), watched: false, ...extra });

test('pickRecent plays the newest unwatched showing from the last two minutes', () => {
  const cases = [
    { name: 'nothing to play', rows: [], want: null },
    { name: 'not a list', rows: null, want: null },
    { name: 'just shown', rows: [row('a', 5000)], want: 'a' },
    { name: 'at the edge of two minutes', rows: [row('a', RECENT_MS)], want: 'a' },
    { name: 'too old to take over the screen', rows: [row('a', RECENT_MS + 1)], want: null },
    { name: 'already watched', rows: [row('a', 5000, { watched: true })], want: null },
    { name: 'skips watched to the next', rows: [row('a', 1000, { watched: true }), row('b', 30000)], want: 'b' },
    { name: 'its time to watch again ran out', rows: [row('a', 5000, { rewatchUntil: ago(1) })], want: null },
    { name: 'an unreadable time never plays', rows: [row('a', 0, { shownAt: 'soon' })], want: null }
  ];
  for (const c of cases) {
    const got = pickRecent(c.rows, NOW);
    assert.equal(got ? got.showingId : null, c.want, c.name);
  }
});

test('rowsOf reads a bare array or a {data, total} page', () => {
  assert.deepEqual(rowsOf([1]), [1]);
  assert.deepEqual(rowsOf({ data: [2], total: 1 }), [2]);
  assert.deepEqual(rowsOf(null), []);
  assert.deepEqual(rowsOf({}), []);
});
