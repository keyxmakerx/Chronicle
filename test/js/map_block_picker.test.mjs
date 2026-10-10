// map_block_picker.test.mjs — pins the Map block picker's pure parts: which
// cards a search keeps, the count line above the grid and the no-match line.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'maps', 'static', 'js', 'map_block_picker.js'), 'utf8');
const sandbox = { module: { exports: {} } };
vm.runInNewContext(src, sandbox);
const P = sandbox.module.exports;

const MAPS = [
  { name: 'Saltreach Isle' },
  { name: 'Port Mirel' },
  { name: 'The Shattered Coast' },
  { name: 'Mirelwood <b>' }
];

test('filterMaps keeps names containing the term, in order', () => {
  const cases = [
    { name: 'no term keeps all', term: '', want: ['Saltreach Isle', 'Port Mirel', 'The Shattered Coast', 'Mirelwood <b>'] },
    { name: 'spaces only keeps all', term: '   ', want: ['Saltreach Isle', 'Port Mirel', 'The Shattered Coast', 'Mirelwood <b>'] },
    { name: 'case-insensitive substring', term: 'MIREL', want: ['Port Mirel', 'Mirelwood <b>'] },
    { name: 'trimmed', term: '  coast ', want: ['The Shattered Coast'] },
    { name: 'markup is plain text', term: '<b>', want: ['Mirelwood <b>'] },
    { name: 'no match', term: 'dragon', want: [] },
    { name: 'null term', term: null, want: ['Saltreach Isle', 'Port Mirel', 'The Shattered Coast', 'Mirelwood <b>'] }
  ];
  for (const c of cases) {
    assert.deepEqual(Array.from(P.filterMaps(MAPS, c.term), (m) => m.name), c.want, c.name);
  }
  assert.equal(P.filterMaps([{}], 'x').length, 0, 'a map with no name never matches a term');
});

test('countLabel says what the grid shows', () => {
  const cases = [
    [4, 4, '', '4 maps'],
    [1, 1, '', '1 map'],
    [2, 4, 'mirel', '2 of 4 maps'],
    [0, 4, 'dragon', '0 of 4 maps'],
    [1, 1, 'salt', '1 of 1 map'],
    [4, 4, '  ', '4 maps']
  ];
  for (const [shown, total, term, want] of cases) assert.equal(P.countLabel(shown, total, term), want, `${shown}/${total} "${term}"`);
});

test('noMatchText quotes the trimmed term', () => {
  assert.equal(P.noMatchText(' dragon '), 'No maps match “dragon”.');
  assert.equal(P.noMatchText('<img>'), 'No maps match “<img>”.');
});
