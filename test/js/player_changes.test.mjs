// player_changes.js: which typed words become tags, which tags are offered,
// and the results request the page sends.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'playerchanges', 'static', 'js', 'player_changes.js'), 'utf8');

function load() {
  const sandbox = { module: { exports: {} } };
  vm.runInNewContext(src, sandbox);
  return sandbox.module.exports;
}
const P = load();

const list = [
  { kind: 'page', value: 'e1', label: 'Tyne' },
  { kind: 'field', value: 'hp', label: 'Stamina' },
  { kind: 'ai', value: '1', label: 'by an AI' },
  { kind: 'removed', value: '1', label: 'removed things' },
];

test('findSuggestion: an exact label, ignoring case and spaces', () => {
  assert.equal(P.findSuggestion(list, ' tyne ').value, 'e1');
  assert.equal(P.findSuggestion(list, 'BY AN AI').kind, 'ai');
  assert.equal(P.findSuggestion(list, 'Ty'), null);
  assert.equal(P.findSuggestion(list, ''), null);
});

test('offered: matches part of a label and leaves out chosen tags', () => {
  assert.deepEqual(P.offered(list, [], 'st').map((s) => s.label), ['Stamina']);
  assert.equal(P.offered(list, [list[0]], '').length, 3);
  assert.equal(P.offered(list, [], '').length, 4);
});

test('queryString: tags, words and range, each encoded', () => {
  assert.equal(P.queryString([list[0], list[1], list[2], list[3]], 'gold & silver', 'long'),
    'page=e1&field=hp&ai=1&removed=1&q=gold%20%26%20silver&range=long');
  assert.equal(P.queryString([], '  ', 'anything'), 'range=month');
});
