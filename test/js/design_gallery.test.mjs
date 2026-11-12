// design_gallery.test.mjs — the pure parts of static/js/widgets/design_library_gallery.js:
// the status words an owner reads, the browse query the sheet sends, paging, the
// count line and the gentle notes under a card. The server decides everything
// else; these only word and shape what it said.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const sandbox = {};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(readFileSync(join(root, 'static/js/widgets/design_library_gallery.js'), 'utf8'), sandbox);
const U = sandbox.DesignGallery.util;
const plain = (o) => JSON.parse(JSON.stringify(o));
const sysName = (id) => ({ drawsteel: 'Draw Steel', dnd5e: 'D&D 5e' }[id] || 'no game system');

test('status chips use the three words the brief names', () => {
  assert.equal(U.statusLabel('visible'), 'Visible');
  assert.equal(U.statusLabel('pending'), 'Waiting for review');
  assert.equal(U.statusLabel('rejected'), 'Not approved');
  assert.equal(U.statusLabel('removed'), 'Not approved');
  assert.equal(U.statusLabel('anything else'), 'Waiting for review');
  assert.equal(U.statusTone('visible'), 'ok');
  assert.equal(U.statusTone('pending'), 'wait');
  assert.equal(U.statusTone('removed'), 'no');
  assert.match(U.statusNote('pending'), /Only you can see it/);
});

test('the browse query carries only the filters that are set', () => {
  const cases = [
    [{ page: 1, all: false, kind: 'all', q: '' }, '?page=1'],
    [{ page: 2, all: true, kind: 'rooms', q: '' }, '?page=2&system=all&kind=rooms'],
    [{ page: 1, all: false, kind: 'crafting', q: 'moon & star' }, '?page=1&kind=crafting&q=moon%20%26%20star'],
    [{ page: 0, all: false, kind: 'all', q: '' }, '?page=1'],
  ];
  for (const [f, want] of cases) assert.equal(U.browseQuery(f), want);
});

test('paging clamps and counts', () => {
  assert.deepEqual(plain(U.pageInfo(0, 1, 12)), { pages: 1, page: 1, from: 0, to: 0, hasPrev: false, hasNext: false });
  assert.deepEqual(plain(U.pageInfo(15, 2, 12)), { pages: 2, page: 2, from: 13, to: 15, hasPrev: true, hasNext: false });
  assert.deepEqual(plain(U.pageInfo(15, 9, 12)).page, 2);
  assert.equal(U.pageInfo(12, 1, 12).pages, 1);
});

test('the count line follows the system toggle', () => {
  assert.equal(U.countLine(2, false, 'Draw Steel'), '2 designs made for Draw Steel, newest first');
  assert.equal(U.countLine(1, true, 'Draw Steel'), '1 design for any game system, newest first');
});

test('a card says a different system gently, then what is lacking', () => {
  const same = U.cardNotes({ sameSystem: true, lacks: [] }, 'drawsteel', sysName);
  assert.deepEqual(plain(same), []);
  const n = plain(U.cardNotes({ sameSystem: false, systemId: 'dnd5e', lacks: ['This campaign has no silver.'] }, 'drawsteel', sysName));
  assert.equal(n.length, 2);
  assert.match(n[0], /Made for D&D 5e\. This campaign uses Draw Steel\./);
  assert.equal(n[1], 'This campaign has no silver.');
});

test('a design can be shared once, and only a listed reason or filter is accepted', () => {
  const mine = [{ id: 'a' }, { id: 'b' }];
  const own = [{ id: 'g1', sourceDesignId: 'a' }];
  assert.deepEqual(plain(U.freeDesigns(mine, own)), [{ id: 'b' }]);
  assert.equal(U.entryFor(own, 'a').id, 'g1');
  assert.equal(U.entryFor(own, 'b'), null);
  assert.equal(U.reportReason('spam'), 'spam');
  assert.equal(U.reportReason('<img>'), 'other');
  assert.equal(U.kindFilter('rooms'), 'rooms');
  assert.equal(U.kindFilter('x" onclick="'), 'all');
});

test('the filters speak the words the server reads', () => {
  const go = readFileSync(join(root, 'internal/plugins/designs/gallery_model.go'), 'utf8');
  const js = readFileSync(join(root, 'static/js/widgets/design_library_gallery.js'), 'utf8');
  for (const [k, v] of [['KindFilterRooms', 'rooms'], ['KindFilterCrafting', 'crafting'], ['ReportAdult', 'adult'], ['ReportSpam', 'spam'], ['ReportOther', 'other']]) {
    assert.ok(new RegExp(k + '\\s*=\\s*"' + v + '"').test(go), k);
    assert.ok(js.includes("'" + v + "'"), v);
  }
  assert.match(go, /GalleryPageSize\s*=\s*12/);
  assert.match(js, /PAGE_SIZE = 12/);
});

test('no typed or attribute text is concatenated into markup', () => {
  const js = readFileSync(join(root, 'static/js/widgets/design_library_gallery.js'), 'utf8');
  // Typed fields are set with .value; none is written into a value="" attribute.
  assert.ok(!/value="'\s*\+ (?!esc\()/.test(js), 'a value attribute is built from unescaped text');
  assert.ok(!/getAttribute\([^)]*\)\s*\+\s*'</.test(js));
});
