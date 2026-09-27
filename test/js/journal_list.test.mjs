// journal_list.test.mjs — the Journal list's pure rules (journal_list.js):
// archive split, pinned group, filter tokens, grouping, sorting, the
// never-leak rule for links to notes the viewer cannot see, and dates.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/widgets/journal_list.js'), 'utf8');
// Evaluated in this realm (not a vm context) so the arrays it returns
// compare with deepEqual against arrays written here.
const win = {};
new Function('window', src)(win);
const JL = win.Chronicle.JournalList;

const NOW = Date.parse('2026-09-25T18:00:00Z');
const day = 86400000;
const iso = (ms) => new Date(ms).toISOString();

const folders = {
  f1: { title: 'Locations', parentId: null },
  f2: { title: 'NPCs & Monsters', parentId: null },
  f3: { title: 'Ruins', parentId: 'f1' },
};
const notes = [
  { id: 'a', title: 'Ashkeep Ruins', parentId: 'f3', userId: 'gm', visibility: 'party', pinned: true, updatedAt: iso(NOW - 1 * day), createdAt: iso(NOW - 40 * day), links: [{ kind: 'page', id: 'p1', label: 'Thalrik Mourngrave' }] },
  { id: 'b', title: 'Thalrik Mourngrave', parentId: 'f2', userId: 'gm', visibility: 'gm', updatedAt: iso(NOW - 2 * day), createdAt: iso(NOW - 30 * day), links: [{ kind: 'note', id: 'hidden' }, { kind: 'note', id: 'a' }] },
  { id: 'c', title: 'Session 12', parentId: null, userId: 'ana', visibility: 'private', updatedAt: iso(NOW - 3 * day), createdAt: iso(NOW - 3 * day), hasAudio: true, links: [{ kind: 'note', id: 'hidden' }] },
  { id: 'd', title: 'Old lore', parentId: 'f2', userId: 'gm', visibility: 'party', archived: true, updatedAt: iso(NOW - 300 * day), createdAt: iso(NOW - 400 * day) },
];
const titles = { a: 'Ashkeep Ruins', b: 'Thalrik Mourngrave', c: 'Session 12' };
const ctx = {
  folders,
  memberName: (id) => ({ gm: 'Gina (GM)', ana: 'Ana' }[id] || id),
  noteTitle: (id) => titles[id] || '',
  contentHits: null,
};
const base = { query: '', scope: 'title', filters: [], groupBy: 'folder', sortBy: 'updated', sortDir: 'desc', archiveView: false, collapsed: {} };

function shape(res) {
  return res.items.map((it) => (it.kind === 'header' ? `# ${it.label} (${it.count})` : it.note.id));
}

test('pinned notes float into their own group, archived notes stay out', () => {
  const res = JL.compute(notes, base, ctx);
  assert.deepEqual(shape(res), ['# Pinned (1)', 'a', '# NPCs & Monsters (1)', 'b', '# No folder (1)', 'c']);
  assert.equal(res.archivedTotal, 1);
  assert.equal(res.matched, 3);
});

test('the archive view lists only archived notes, with no pinned group', () => {
  const res = JL.compute(notes, { ...base, archiveView: true, groupBy: 'none' }, ctx);
  assert.deepEqual(shape(res), ['d']);
});

test('folder groups show the folder path', () => {
  const res = JL.compute(notes, { ...base, collapsed: { __pinned__: true }, filters: [{ field: 'folder', value: 'f3' }] }, ctx);
  assert.deepEqual(shape(res), ['# Pinned (1)']);
  const unpinned = JL.compute(notes.map((n) => ({ ...n, pinned: false })), base, ctx);
  assert.ok(shape(unpinned).includes('# Locations / Ruins (1)'));
});

test('an empty folder still shows its header until something narrows the list', () => {
  const withEmpty = { ...ctx, emptyFolders: ['f1'] };
  assert.ok(shape(JL.compute(notes, base, withEmpty)).includes('# Locations (0)'));
  assert.ok(!shape(JL.compute(notes, { ...base, query: 'x' }, withEmpty)).includes('# Locations (0)'));
});

test('filter tokens: owner, links, is, has, dates', () => {
  const only = (rule) => JL.compute(notes, { ...base, groupBy: 'none', filters: [rule] }, ctx).items.filter((i) => i.kind === 'row').map((i) => i.note.id).sort();
  assert.deepEqual(only({ field: 'owner', value: 'ana' }), ['c']);
  assert.deepEqual(only({ field: 'linksTo', value: 'p1' }), ['a']);
  assert.deepEqual(only({ field: 'is', value: 'gm' }), ['b']);
  assert.deepEqual(only({ field: 'is', value: 'pinned' }), ['a']);
  assert.deepEqual(only({ field: 'audio', value: true }), ['c']);
  assert.deepEqual(only({ field: 'updatedAfter', value: NOW - 2.5 * day }), ['a', 'b']);
  assert.deepEqual(only({ field: 'updatedBefore', value: NOW - 2.5 * day }), ['c']);
  assert.deepEqual(only({ field: 'folder', value: '' }), ['c']);
});

test('title search, and contents search through the server hits with their line', () => {
  const t = JL.compute(notes, { ...base, groupBy: 'none', query: 'SESSION' }, ctx);
  assert.deepEqual(shape(t), ['c']);
  const c = JL.compute(notes, { ...base, groupBy: 'none', query: 'lich', scope: 'contents' }, { ...ctx, contentHits: { b: '…the lich waits…' } });
  assert.deepEqual(shape(c), ['b']);
  assert.equal(c.items[0].snippet, '…the lich waits…');
  const ignored = JL.compute(notes, { ...base, groupBy: 'none', query: 'lich', scope: 'title' }, { ...ctx, contentHits: { b: 'x' } });
  assert.deepEqual(shape(ignored), [], 'content hits count only when searching contents');
});

test('grouping by linked page never labels a link to a note the viewer cannot see', () => {
  const res = JL.compute(notes, { ...base, groupBy: 'link', collapsed: { __pinned__: true } }, ctx);
  const labels = res.items.filter((i) => i.kind === 'header').map((i) => i.label);
  assert.deepEqual(labels, ['Pinned', 'Ashkeep Ruins', 'Not linked']);
  assert.equal(JL.firstLink(notes[2], ctx), null, 'a link only to a hidden note has no label');
});

test('grouping by owner, month and visibility', () => {
  const owners = JL.compute(notes, { ...base, groupBy: 'owner', collapsed: { __pinned__: true } }, ctx).items.filter((i) => i.kind === 'header').map((i) => i.label);
  assert.deepEqual(owners, ['Pinned', 'Ana', 'Gina (GM)']);
  const vis = JL.compute(notes, { ...base, groupBy: 'visibility', collapsed: { __pinned__: true } }, ctx).items.filter((i) => i.kind === 'header').map((i) => i.label);
  assert.deepEqual(vis, ['Pinned', 'Private', 'GM only']);
  const months = JL.compute(notes, { ...base, groupBy: 'month', archiveView: true }, ctx).items.filter((i) => i.kind === 'header').map((i) => i.label);
  assert.equal(months.length, 1);
  assert.match(months[0], /^(November|December) 2025$/);
});

test('sorting by title and created, both directions', () => {
  const rows = (s) => JL.compute(notes, { ...base, groupBy: 'none', collapsed: { __pinned__: true }, ...s }, ctx).items.filter((i) => i.kind === 'row').map((i) => i.note.id);
  assert.deepEqual(rows({ sortBy: 'title', sortDir: 'asc' }), ['c', 'b']);
  assert.deepEqual(rows({ sortBy: 'created', sortDir: 'asc' }), ['b', 'c']);
});

test('shift-click ranges follow display order', () => {
  const res = JL.compute(notes, { ...base, groupBy: 'none' }, ctx);
  assert.deepEqual(JL.rangeBetween(res.items, 'c', 'a'), ['a', 'b', 'c']);
  assert.deepEqual(JL.rangeBetween(res.items, 'a', 'zzz'), []);
});

test('relative and long dates', () => {
  assert.equal(JL.relDate(NOW - 30 * 1000, NOW), 'now');
  assert.equal(JL.relDate(NOW - 5 * 60000, NOW), '5m');
  assert.equal(JL.relDate(NOW - 3 * 3600000, NOW), '3h');
  assert.match(JL.relDate(NOW - 2 * day, NOW), /^(Sun|Mon|Tue|Wed|Thu|Fri|Sat)$/);
  assert.match(JL.relDate(NOW - 30 * day, NOW), /^Aug \d+$/);
  assert.match(JL.relDate(NOW - 400 * day, NOW), /, 2025$/);
  assert.match(JL.longDate(NOW), /^Sep 25, \d{1,2}:\d{2}(am|pm)$/);
});

test('the qualifier vocabulary has no tag:, since notes carry no tags', () => {
  assert.equal(JL.tokenByKw('tag'), null);
  assert.equal(JL.tokenByKw('in').field, 'folder');
  assert.equal(JL.parseDate('2026-09-01') !== null, true);
  assert.equal(JL.parseDate('someday'), null);
});
