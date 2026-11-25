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
  assert.equal(DW.words(new Error('that hero page is not in this campaign')), 'That hero page is not in this campaign.');
});

test('a fragment carries the deck version the widget reads, under the name the server writes', () => {
  assert.equal(DW.parseVersion('<div><span hidden data-dkw-version="7"></span></div>'), 7);
  assert.equal(DW.parseVersion('<div>nothing here</div>'), 0);
  assert.equal(DW.parseVersion(''), 0);
  assert.equal(DW.parseVersion(null), 0);
  const templ = readFileSync(join(decks, 'workshop_deck.templ'), 'utf8');
  assert.match(templ, /data-dkw-version=\{ strconv\.Itoa\(v\) \}/);
  // Every fragment (the four tabs through deckTabPanel, and the table row) writes the mark.
  assert.match(templ, /templ deckTabPanel\(p deckPage\) \{\s*@versionMark\(p\.Deck\.Version, p\.Deck\.Name\)/);
  assert.match(readFileSync(join(decks, 'workshop_table.templ'), 'utf8'), /@versionMark\(p\.Deck\.Version, p\.Deck\.Name\)/);
});

test('the widget takes a newer version from a reload and never goes back', () => {
  const src = readFileSync(join(decks, 'static', 'js', 'deck_workshop.js'), 'utf8');
  assert.match(src, /versionSink = function \(v\) \{ if \(v > version\(\)\)/);
  assert.match(src, /var v = parseVersion\(text\);\s*if \(v && versionSink\) versionSink\(v\);/);
});

// ---- Comes up when ----

function goKeys() {
  const src = readFileSync(join(decks, 'when.go'), 'utf8');
  const block = src.match(/var keysOf = map\[string\]\[\]string\{([\s\S]*?)\n\}/);
  assert.ok(block, 'no keysOf in when.go');
  const out = {};
  for (const m of block[1].matchAll(/Test(\w+):\s*\{([^}]*)\}/g)) {
    out[m[1].toLowerCase()] = [...m[2].matchAll(/"([^"]*)"/g)].map((x) => x[1]);
  }
  return out;
}

const opts = {
  available: true,
  fields: [{ key: 'hp', label: 'Hit points', maxKey: 'hpmax' }, { key: 'stamina', label: 'Stamina' }],
  trackers: [{ id: 'k-doom', name: 'Doom' }],
  tags: [{ id: 7, name: 'Cursed' }],
  layers: ['Haunted'],
};
const plain = (v) => JSON.parse(JSON.stringify(v));
const heroDeck = { heroDeck: true, holder: 'h-tam' };

test('the editor offers the keys and the limit the server accepts', () => {
  assert.equal(DW.MAX_TESTS, Number(readFileSync(join(decks, 'when.go'), 'utf8').match(/MaxTests\s*=\s*(\d+)/)[1]));
  const go = goKeys();
  for (const kind of Object.keys(DW.TEST_KEYS)) {
    assert.deepEqual([...DW.TEST_KEYS[kind]], go[kind] || [], kind);
  }
});

test('each kind of test starts with values the server accepts', () => {
  const n = DW.newTest('number', heroDeck, opts);
  assert.deepEqual(plain(DW.whenBody([n])), [{ test: 'number', open: false, who: 'holder', field: 'hp', op: 'lte', of: 'hpmax', share: 'half' }]);
  const tr = DW.newTest('tracker', { heroDeck: false }, opts);
  assert.deepEqual(plain(DW.whenBody([tr])), [{ test: 'tracker', open: false, who: 'any', kind: 'k-doom', op: 'gte', value: 1 }]);
  const tg = DW.newTest('tag', heroDeck, opts);
  assert.deepEqual(plain(DW.whenBody([tg])), [{ test: 'tag', open: false, who: 'holder', tag: 7, has: true }]);
  const ly = DW.newTest('layer', heroDeck, opts);
  assert.deepEqual(plain(DW.whenBody([ly])), [{ test: 'layer', open: false, who: 'holder', name: 'Haunted', on: true }]);
  assert.deepEqual(plain(DW.whenBody([DW.newTest('say', heroDeck, opts)])), [{ test: 'say', open: false }]);
});

test('a layer with no name to offer is a draft and is not sent', () => {
  const ly = DW.newTest('layer', heroDeck, { available: true, layers: [] });
  assert.equal(ly._draft, true);
  assert.deepEqual(plain(DW.whenBody([ly])), []);
  assert.deepEqual(plain(DW.whenBody([DW.changeTest(ly, 'name', 'Hexed', opts)])), [{ test: 'layer', open: false, who: 'holder', name: 'Hexed', on: true }]);
});

test('a kind the campaign has nothing for is not offered', () => {
  assert.equal(DW.kindAvailable('say', {}), true);
  assert.equal(DW.kindAvailable('layer', { available: false }), false);
  assert.equal(DW.kindAvailable('tag', { available: true, tags: [] }), false);
  assert.equal(DW.kindAvailable('tracker', { available: true, trackers: [{}] }), true);
  assert.equal(DW.kindAvailable('number', { available: false, fields: [{}] }), false);
  assert.equal(DW.kindAvailable('moon', { available: true, hasCalendar: true, moons: ['Nyx'] }), true);
  assert.equal(DW.kindAvailable('moon', { available: true, hasCalendar: true, moons: [] }), false);
  assert.equal(DW.kindAvailable('event', { available: true, hasCalendar: false }), false);
  assert.equal(DW.kindAvailable('time', { available: true, hasCalendar: true }), true);
  assert.equal(DW.kindAvailable('time', { available: false, hasCalendar: true }), false);
});

test('the calendar tests start with values the server accepts and name no page', () => {
  const cal = { available: true, hasCalendar: true, moons: ['Nyx'], eventKinds: [{ slug: 'blood', name: 'Bloodnight' }] };
  assert.deepEqual(plain(DW.whenBody([DW.newTest('moon', heroDeck, cal)])), [{ test: 'moon', open: false, name: 'Nyx', phase: 'full', is: true }]);
  assert.deepEqual(plain(DW.whenBody([DW.newTest('event', heroDeck, cal)])), [{ test: 'event', open: false, name: 'Bloodnight', has: true }]);
  assert.deepEqual(plain(DW.whenBody([DW.newTest('time', heroDeck, cal)])), [{ test: 'time', open: false, time: 'night', is: true }]);
  assert.equal(DW.newTest('moon', heroDeck, { available: true, hasCalendar: true, moons: [] }), null);
  const ev = DW.newTest('event', heroDeck, { available: true, hasCalendar: true, eventKinds: [] });
  assert.equal(ev._draft, true, 'an event with no kind to offer is a draft');
  assert.deepEqual(plain(DW.whenBody([ev])), []);
});

test('a moon test turns between a phase and days of its cycle', () => {
  const m = DW.newTest('moon', heroDeck, { moons: ['Nyx'] });
  const days = DW.changeTest(m, 'phase', 'days', opts);
  assert.deepEqual(plain(DW.whenBody([days])), [{ test: 'moon', open: false, name: 'Nyx', from: 1, to: 1, is: true }]);
  const range = DW.changeTest(DW.changeTest(days, 'from', 23, opts), 'to', 1, opts);
  assert.deepEqual([range.from, range.to], [23, 1]);
  const back = DW.changeTest(range, 'phase', 'waning', opts);
  assert.deepEqual(plain(DW.whenBody([back])), [{ test: 'moon', open: false, name: 'Nyx', phase: 'waning', is: true }]);
  assert.equal(DW.changeTest(back, 'is', 'false', opts).is, false);
  assert.equal(DW.changeTest(DW.newTest('time', heroDeck, {}), 'time', 'dusk', opts).time, 'dusk');
});

test('changing one blank changes only that blank', () => {
  const n = DW.newTest('number', heroDeck, opts);
  const op = DW.changeTest(n, 'op', 'gte', opts);
  assert.equal(op.op, 'gte');
  assert.equal(n.op, 'lte', 'the old test is not edited in place');
  const num = DW.changeTest(n, 'compare', 'number', opts);
  assert.equal(num.value, 0);
  assert.equal(num.of, undefined);
  assert.equal(num.share, undefined);
  const back = DW.changeTest(num, 'compare', 'quarter', opts);
  assert.equal(back.share, 'quarter');
  assert.equal(back.of, 'hpmax');
  assert.equal(back.value, undefined);
  assert.equal(DW.changeTest(DW.newTest('tag', heroDeck, opts), 'has', 'false', opts).has, false);
  assert.equal(DW.changeTest(DW.newTest('tag', heroDeck, opts), 'tag', '7', opts).tag, 7);
});

test('a typed number is read the way a person writes it', () => {
  assert.equal(DW.parseNumber('12'), 12);
  assert.equal(DW.parseNumber(' -3,5 '), -3.5);
  assert.equal(DW.parseNumber('.5'), 0.5);
  for (const bad of ['', 'abc', '1e3', '1 2', null]) assert.equal(DW.parseNumber(bad), null, String(bad));
});

test('the hint and the Now line say what a Director needs', () => {
  assert.equal(DW.whenHint([{ open: true }]), '');
  assert.equal(DW.whenHint([{ open: false }]), '');
  assert.equal(DW.whenHint([{ open: true }, { open: false }]), 'The second condition is yours alone, so the card shows only the first.');
  assert.equal(DW.whenHint([{ open: true }, { open: false }, { open: false }]), 'The second and third conditions are yours alone, so the card shows only the first.');
  assert.equal(DW.nowLine([], { fitsNow: true }), '');
  assert.equal(DW.nowLine([{}], { fitsNow: true }), 'Fits now.');
  assert.equal(DW.nowLine([{}], { fitsNow: false, whyNot: 'Tam is above half' }), 'Waiting: Tam is above half.');
});

test('the open tab of the New deck page decides what goes out', () => {
  const plain = (v) => JSON.parse(JSON.stringify(v)); // across the sandbox's realm
  assert.deepEqual(plain(DW.startBody('style', { look: { border: 'metal' }, deck: 'd1', table: 't1' })), { url: '', body: { look: { border: 'metal' } } });
  assert.deepEqual(plain(DW.startBody('copy', { look: {}, deck: 'd1', table: 't1' })), { url: 'from', body: { deckId: 'd1' } });
  assert.deepEqual(plain(DW.startBody('table', { look: {}, deck: 'd1', table: 't1' })), { url: 'from', body: { tableId: 't1' } });
  assert.match(DW.startBody('copy', { look: {}, deck: '', table: 't1' }).error, /Choose a deck/);
  assert.match(DW.startBody('table', { look: {}, deck: 'd1', table: '' }).error, /Choose a rolling table/);
});
