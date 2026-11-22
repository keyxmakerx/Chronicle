// deck_workshop.test.mjs: the Director's deck workshop script
// (internal/plugins/decks/static/js/deck_workshop.js) must offer exactly the
// choices the server accepts, keep the draw stepper inside what can be drawn,
// build the one-value patch the server expects, and recognise the server's own
// "this deck changed" refusal. The pieces are pure, so they run here without a
// browser; the walk through the pages is test/live/deck_workshop.spec.mjs.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const decks = join(here, '..', '..', 'internal', 'plugins', 'decks');

function load() {
  const registered = {};
  const sandbox = { console, Promise, setTimeout, clearTimeout };
  sandbox.window = sandbox;
  sandbox.Chronicle = { register: (name, def) => { registered[name] = def; } };
  sandbox.document = { addEventListener() {}, querySelector() { return null; }, querySelectorAll() { return []; } };
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(join(decks, 'static', 'js', 'deck_workshop.js'), 'utf8'), sandbox);
  return { DW: sandbox.DeckWorkshop, registered };
}
const { DW, registered } = load();

function goList(file, name) {
  const src = readFileSync(join(decks, file), 'utf8');
  const m = src.match(new RegExp(name + '\\s*=\\s*(?:\\[\\]string)?\\{([^}]*)\\}'));
  assert.ok(m, `no ${name} in ${file}`);
  return [...m[1].matchAll(/"([^"]*)"/g)].map((x) => x[1]);
}
function goConst(name) {
  const m = readFileSync(join(decks, 'model.go'), 'utf8').match(new RegExp(name + '\\s*=\\s*(\\d+)'));
  assert.ok(m, `no ${name} in model.go`);
  return Number(m[1]);
}

test('the widget registers under the name the page mounts', () => {
  assert.ok(registered['deck-workshop']);
});

test('every choice list matches the list the server accepts', () => {
  const pairs = {
    shape: 'shapeValues', border: 'borderValues', turn: 'turnValues', window: 'windowValues',
    name: 'nameValues', lettering: 'letteringValues', corner: 'cornerValues', marker: 'markerValues',
    finish: 'finishValues', 'back.pattern': 'patternValues', 'back.sign': 'backSignValues',
  };
  for (const [path, goName] of Object.entries(pairs)) {
    assert.deepEqual([...DW.LISTS[path]], goList('settings.go', goName), path);
  }
  assert.deepEqual([...DW.PALETTE], goList('settings.go', 'Palette'));
  assert.deepEqual([...DW.LISTS['back.colour']], ['', ...goList('settings.go', 'Palette')]);
});

test('the stepper limits match the server limits', () => {
  assert.equal(DW.DRAW_MAX, goConst('MaxDrawCount'));
  assert.equal(DW.COPIES_MAX, goConst('MaxCopiesPerCard'));
  assert.equal(DW.MAX_KINDS, goConst('MaxKinds'));
});

test('the draw stepper stays between one and what can be drawn', () => {
  assert.equal(DW.stepCount(1, -1, 4), 1);
  assert.equal(DW.stepCount(1, 1, 4), 2);
  assert.equal(DW.stepCount(4, 1, 4), 4);
  assert.equal(DW.stepCount(5, 1, 9), 5);
  assert.equal(DW.stepCount(3, 1, 2), 2);
  assert.equal(DW.stepCount('x', 1, 4), 2);
  assert.equal(DW.clampCount(0, 4), 1);
  assert.equal(DW.clampCount(99, 3), 3);
  assert.equal(DW.clampCount(2, 0), 1);
  assert.equal(DW.clampCount('2', null), 2);
});

test('a look choice becomes the one-value patch, and a value the lists lack is refused', () => {
  assert.deepEqual(JSON.parse(JSON.stringify(DW.lookPatch('border', 'brass'))), { border: 'brass' });
  assert.deepEqual(JSON.parse(JSON.stringify(DW.lookPatch('back.colour', 'teal'))), { back: { colour: 'teal' } });
  assert.deepEqual(JSON.parse(JSON.stringify(DW.lookPatch('back.colour', ''))), { back: { colour: '' } });
  assert.equal(DW.lookPatch('border', 'gold'), null);
  assert.equal(DW.lookPatch('nonsense', 'x'), null);
});

test('a new kind gets a key and a name no other kind has', () => {
  assert.equal(DW.kindKey('Bane', []), 'bane');
  assert.equal(DW.kindKey('Bane', ['bane']), 'bane-2');
  assert.equal(DW.kindKey('Bane', ['bane', 'bane-2']), 'bane-3');
  assert.equal(DW.kindKey('7 Seas!', []), 'kind-7-seas');
  assert.equal(DW.kindKey('', []), 'kind');
  assert.equal(DW.nextKindName([]), 'Kind 1');
  assert.equal(DW.nextKindName(['Kind 1', 'Kind 3']), 'Kind 2');
});

test('a stale save is recognised from the words the server uses', () => {
  const src = readFileSync(join(decks, 'errors.go'), 'utf8');
  const m = src.match(/NewConflict\("([^"]*)"\)/);
  assert.ok(m, 'no stale message in errors.go');
  assert.equal(DW.isStale(m[1]), true);
  assert.equal(DW.isStale('the name is too long'), false);
  assert.equal(DW.isStale(''), false);
});

test('a failed request is said in plain words', () => {
  assert.match(DW.words(new Error('Failed to fetch')), /did not go through/);
  assert.equal(DW.words(new Error('That card is gone')), 'That card is gone.');
  assert.equal(DW.words(new Error('Already ends.')), 'Already ends.');
});

test('a fragment carries the deck version the widget reads, under the name the server writes', () => {
  assert.equal(DW.parseVersion('<div><span hidden data-dkw-version="7"></span></div>'), 7);
  assert.equal(DW.parseVersion('<div>nothing here</div>'), 0);
  assert.equal(DW.parseVersion(''), 0);
  assert.equal(DW.parseVersion(null), 0);
  const templ = readFileSync(join(decks, 'workshop_deck.templ'), 'utf8');
  assert.match(templ, /data-dkw-version=\{ strconv\.Itoa\(v\) \}/);
  // Every fragment (the four tabs through deckTabPanel, and the table row) writes the mark.
  assert.match(templ, /templ deckTabPanel\(p deckPage\) \{\s*@versionMark\(p\.Deck\.Version\)/);
  assert.match(readFileSync(join(decks, 'workshop_table.templ'), 'utf8'), /@versionMark\(p\.Deck\.Version\)/);
});

test('the widget takes a newer version from a reload and never goes back', () => {
  const src = readFileSync(join(decks, 'static', 'js', 'deck_workshop.js'), 'utf8');
  assert.match(src, /versionSink = function \(v\) \{ if \(v > version\(\)\)/);
  assert.match(src, /var v = parseVersion\(text\);\s*if \(v && versionSink\) versionSink\(v\);/);
});
