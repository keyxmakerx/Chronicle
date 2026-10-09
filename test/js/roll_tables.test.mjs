// roll_tables.test.mjs — pins the rolling tables against the real generator
// engine: every table the picker offers rolls, the library's tables carry a
// credit that copies keep, a campaign's own table rolls and can use a
// starter's {braces}, and the page node saves only what the sanitizer keeps
// (classes), reading back what it saved.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const require = createRequire(import.meta.url);
const G = require('../../static/js/widgets/chronicle_gen.js');
const LIBRARY = JSON.parse(readFileSync(join(root, 'static/roll-tables/library.json'), 'utf8'));

function loadRT() {
  const window = { Chronicle: {} };
  const document = { currentScript: { getAttribute: () => '/x/chronicle_gen.js' }, querySelector: () => null };
  const matchMedia = () => ({ matches: true });
  vm.runInNewContext(readFileSync(join(root, 'static/js/widgets/roll_tables.js'), 'utf8'),
    { window, document, matchMedia, Chronicle: window.Chronicle });
  return window.Chronicle.RollTables;
}

function loadNode() {
  const added = [];
  const div = (cls) => ({ getAttribute: (a) => (a === 'class' ? cls : null) });
  const window = {
    TipTap: { Node: { create: (spec) => ({ spec }) } },
    Chronicle: { SlashCommands: { addCommand: (c) => added.push(c) } },
  };
  vm.runInNewContext(readFileSync(join(root, 'static/js/widgets/editor_rolltable.js'), 'utf8'),
    { window, Chronicle: window.Chronicle, TipTap: window.TipTap });
  return { E: window.Chronicle.EditorRollTable, added, div };
}

const OWN = [{ id: 'harbour', name: 'Harbour news', entries: [
  { name: 'A ship from far off', brief: 'It carries {trade}.', weight: 2 },
  { name: 'Fog for a week', brief: '', weight: 1 },
] }];

test('every table in the picker rolls something', () => {
  const RT = loadRT();
  RT._internal.useLibrary(LIBRARY);
  const groups = RT._internal.catalog(G, OWN);
  assert.deepEqual([...groups.map((g) => g[0])], ['Your tables', 'Happenings', 'Names', 'Words', 'Customs and festivals', 'Ironsworn oracles', 'Ironsworn: Delve', 'D&D 5e SRD']);
  for (const [, items] of groups) {
    for (const t of items) {
      const r = RT.roll(G, OWN, t.id);
      assert.ok(r && typeof r.name === 'string' && r.name.length, t.id);
      assert.notEqual(r.problem, true, t.id + ': ' + r.brief);
    }
  }
});

test('a campaign table rolls its own lines and fills a starter\'s {braces}', () => {
  const RT = loadRT();
  for (let i = 0; i < 20; i++) {
    const r = RT.roll(G, OWN, 'harbour');
    assert.ok(['A ship from far off', 'Fog for a week'].includes(r.name), r.name);
    if (r.brief) assert.doesNotMatch(r.brief, /[{}]/);
  }
});

test('without the library the starters still roll, and the library groups are absent', () => {
  const RT = loadRT();
  const groups = RT._internal.catalog(G, OWN);
  assert.deepEqual([...groups.map((g) => g[0])], ['Your tables', 'Happenings', 'Names', 'Words', 'Customs and festivals']);
  assert.notEqual(RT.roll(G, OWN, 'rumours').problem, true);
});

test('the library is one valid set with the starters, and every table is credited under an allowed licence', () => {
  const merged = { format: LIBRARY.format, version: 1, id: 'x', tables: G._internal.STARTER_TABLES.tables.concat(LIBRARY.tables) };
  const v = G.tables.validate(merged);
  assert.ok(v.ok, JSON.stringify(v.errors.slice(0, 3)));
  const allowed = { 'CC BY 4.0': 'https://creativecommons.org/licenses/by/4.0/' };
  const sources = readFileSync(join(root, 'static/roll-tables/SOURCES.md'), 'utf8');
  for (const t of LIBRARY.tables) {
    const c = t.credit || {};
    assert.ok(c.source && c.author && c.url && c.notice, t.id + ' credit');
    assert.equal(allowed[c.licence], c.licenceUrl, t.id + ' licence');
    assert.match(c.url, /^https:\/\//, t.id);
    assert.ok(sources.includes('`' + t.id + '`'), t.id + ' is missing from SOURCES.md');
  }
});

test('library tables with templates roll real words, never braces', () => {
  const RT = loadRT();
  RT._internal.useLibrary(LIBRARY);
  for (const id of ['delve-site-name', 'is-settlement-name', 'is-settlement-quick-name', 'delve-threat', 'delve-site-place']) {
    for (let i = 0; i < 25; i++) {
      const r = RT.roll(G, OWN, id);
      assert.notEqual(r.problem, true, id + ': ' + r.brief);
      assert.doesNotMatch(r.name + ' ' + (r.brief || ''), /[{}]/, id);
    }
  }
});

test('a copy of a library table keeps its lines, odds and credit, and its braces still roll', () => {
  const RT = loadRT();
  RT._internal.useLibrary(LIBRARY);
  const src = RT._internal.starter(G, 'delve-site-name');
  const copy = { id: RT._internal.freeId(G, OWN, 'delve-site-name'), name: 'Site name (copy)', entries: RT._internal.entriesFor(G, OWN, 'delve-site-name'), credit: src.credit };
  assert.equal(copy.id, 'delve-site-name-2');
  assert.deepEqual(copy.entries.map((e) => e.weight), src.entries.map((e) => e.weight));
  const mine = OWN.concat([copy]);
  for (let i = 0; i < 10; i++) assert.doesNotMatch(RT.roll(G, mine, copy.id).name, /[{}]/);
  assert.equal(RT._internal.creditLine(copy.credit).startsWith('Adapted from <a href="https://www.ironswornrpg.com"'), true);
});

test('a credit only links https addresses, and escapes its text', () => {
  const RT = loadRT();
  const line = RT._internal.creditLine({ source: '<b>S</b>', url: 'javascript:alert(1)', licence: 'CC BY 4.0', licenceUrl: 'https://x.test/"y' });
  assert.doesNotMatch(line, /<a /);
  assert.match(line, /&lt;b&gt;S&lt;\/b&gt;/);
  assert.equal(RT._internal.creditLine(null), '');
  assert.equal(RT._internal.creditHTML({ author: 'no source' }), '');
});

test('printed ranges follow the die, and odd weights stay choosable', () => {
  const RT = loadRT();
  const es = [{ weight: 1 }, { weight: 4 }, { weight: 95 }];
  assert.deepEqual([...RT._internal.ranges({ dice: 'd100' }, es)], ['01', '02–05', '06–100']);
  assert.deepEqual([...RT._internal.ranges({ dice: 'd6' }, [{ weight: 1 }, { weight: 5 }])], ['1', '2–6']);
  assert.equal(RT._internal.ranges({ dice: 'd100' }, [{ weight: 1 }]), null);
  assert.equal(RT._internal.ranges({}, es), null);
  assert.deepEqual([...RT._internal.weightChoices(2)], [1, 2, 3]);
  assert.deepEqual([...RT._internal.weightChoices(15)], [1, 2, 3, 15]);
});

test('a copy gets a free id that never shadows a starter or a name table', () => {
  const RT = loadRT();
  const id = RT._internal.freeId(G, OWN, 'Rumours');
  assert.equal(id, 'rumours-2');
  assert.match(RT._internal.freeId(G, OWN, 'Harbour news'), RT._internal.ID_RE);
  assert.equal(RT._internal.freeId(G, OWN, 'harbour'), 'harbour-2');
  assert.equal(RT._internal.freeId(G, OWN, 'names-people'), 'names-people-2');
});

test('the page node saves table and count as classes and reads them back', () => {
  const { E, added, div } = loadNode();
  const spec = E._renderSpec({ table: 'sky-sights', count: 5 });
  assert.deepEqual(JSON.parse(JSON.stringify(spec)), ['div', { class: 'ce-roll ce-roll--t-sky-sights ce-roll--n-5' }]);
  assert.deepEqual({ ...E._attrsFromDiv(div(spec[1].class)) }, { table: 'sky-sights', count: 5 });
  // Anything odd falls back to the defaults rather than reaching a class.
  assert.deepEqual({ ...E._attrsFromDiv(div('ce-roll ce-roll--t-Bad"x ce-roll--n-7')) }, { table: 'rumours', count: 3 });
  assert.equal(added.length, 1);
  assert.equal(added[0].label, 'Rolling table');
});

test('put in page writes a bold name and its line, one paragraph each', () => {
  const { E } = loadNode();
  const c = JSON.parse(JSON.stringify(E._resultContent([{ name: 'Fog', brief: 'For a week.' }, { name: 'Bells' }])));
  assert.deepEqual(c, [
    { type: 'paragraph', content: [{ type: 'text', text: 'Fog', marks: [{ type: 'bold' }] }, { type: 'text', text: ' For a week.' }] },
    { type: 'paragraph', content: [{ type: 'text', text: 'Bells', marks: [{ type: 'bold' }] }] },
  ]);
});
