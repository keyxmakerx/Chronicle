// own_entries.js: a save carries back everything the editor did not show.
// A Director rewriting a printed entry edits a few lines; the rest of the
// entry (rules attached to a trait, a yes/no, an object) must come back
// exactly as loaded, or the save quietly breaks the hero creator.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'entities', 'static', 'js', 'own_entries.js'), 'utf8');
const editorSrc = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'entities', 'static', 'js', 'own_entry_editor.js'), 'utf8');
const OE = (() => {
  const document = { readyState: 'complete', addEventListener() {}, querySelectorAll() { return []; } };
  const window = {};
  const sandbox = { window, document, Chronicle: { escapeHtml: (s) => String(s) }, JSON, Object, Array, String, Number };
  vm.createContext(sandbox);
  vm.runInContext(editorSrc, sandbox);
  vm.runInContext(src, sandbox);
  return window.ChronicleOwnEntries;
})();

// The shape the editor reads from the package's ancestries.
const field = {
  facts: ['size'],
  lists: [{ key: 'signature_traits', label: 'Signature traits', buy: false }, { key: 'purchased_traits', label: 'Purchased traits', buy: true }],
  budgetKey: 'ancestry_points', budget: 3,
};

const dwarf = () => ({
  id: 7, name: 'Dwarf', summary: 'Stone folk.', description: '<p>Dense.</p>', visibility: 'everyone', status: 'accepted',
  standsFor: 'dwarf', standsForName: 'Dwarf',
  properties: {
    size: '1M', speed: 5, ancestry_points: 3, provenance: 'custom fields — description',
    stability_bonus: true,
    modifiers: { stamina: 6 },
    quick_build: ['Grounded', 'Spark Off Your Skin'],
    benefit: 'x'.repeat(130),
    signature_traits: [{ name: 'Runic Carving', description: 'Carve a rune.', grants: { ability: 'rune' } }],
    purchased_traits: [
      { name: 'Grounded', cost: 1, description: 'Hard to move.', modifiers: { stability: 1 } },
      { name: 'Great Fortitude', cost: 2, description: 'Cannot be weakened.' },
    ],
  },
});

test('an untouched entry saves back exactly as loaded', () => {
  const e = dwarf();
  const d = OE.draftOf(e, field);
  const out = OE.propsOf(d, field);
  assert.equal(out.error, undefined);
  assert.deepEqual(JSON.parse(JSON.stringify(out.props)), e.properties);
});

test('the draft knows what it rewrote and who read it', () => {
  const d = OE.draftOf(dwarf(), field);
  assert.equal(d.standsFor, 'dwarf');
  assert.equal(d.standsForName, 'Dwarf');
  assert.equal(d.savedWho, 'everyone');
  // The credit is carried, never offered as a fact or named as kept.
  assert.deepEqual(Object.keys(d.carried).sort(), ['modifiers', 'provenance', 'stability_bonus']);
  assert.equal(d.facts.some((f) => f.key === 'provenance'), false);
  assert.equal(d.wordLists[0].key, 'quick_build');
  assert.equal(d.facts.find((f) => f.key === 'benefit').long, true);
});

test('a renamed item drops the rules attached to its old name', () => {
  const d = OE.draftOf(dwarf(), field);
  d.lists.purchased_traits[0].name = 'Rooted';
  const p = OE.propsOf(d, field).props;
  assert.deepEqual(JSON.parse(JSON.stringify(p.purchased_traits[0])), { name: 'Rooted', cost: 1, description: 'Hard to move.' });
  // The other item and the signature trait keep theirs.
  assert.deepEqual(JSON.parse(JSON.stringify(p.signature_traits[0].grants)), { ability: 'rune' });
});

test('an edited description keeps the item\'s other keys', () => {
  const d = OE.draftOf(dwarf(), field);
  d.lists.purchased_traits[0].description = 'Very hard to move.';
  const p = OE.propsOf(d, field).props;
  assert.equal(p.purchased_traits[0].description, 'Very hard to move.');
  assert.deepEqual(JSON.parse(JSON.stringify(p.purchased_traits[0].modifiers)), { stability: 1 });
});

test('word lists and facts are edited as text', () => {
  const d = OE.draftOf(dwarf(), field);
  d.wordLists[0].text = 'Grounded\n\n  Great Fortitude ';
  d.facts.find((f) => f.key === 'speed').value = '6';
  const p = OE.propsOf(d, field).props;
  assert.deepEqual(Array.from(p.quick_build), ['Grounded', 'Great Fortitude']);
  assert.equal(p.speed, 6);
  // Emptying a word list drops it.
  d.wordLists[0].text = '  ';
  assert.equal(OE.propsOf(d, field).props.quick_build, undefined);
});

test('two facts with one name are refused', () => {
  const d = OE.draftOf(dwarf(), field);
  d.facts.push({ label: 'Size', value: '2', key: '' });
  assert.match(OE.propsOf(d, field).error, /Two facts are both called/);
});

test('the kept line names the carried parts in words, quietly', () => {
  const d = OE.draftOf(dwarf(), field);
  assert.equal(OE.keptLine(d, field), 'Kept as printed, not edited here: Stability bonus; Modifiers; the rules attached to Runic Carving, Grounded.');
  d.lists.purchased_traits[0].name = 'Rooted';
  assert.equal(OE.keptLine(d, field), 'Kept as printed, not edited here: Stability bonus; Modifiers; the rules attached to Runic Carving.');
});

test('a plain entry has no kept line', () => {
  const d = OE.draftOf({ id: 1, name: 'Ashborn', properties: { size: '1M' } }, field);
  assert.equal(OE.keptLine(d, field), '');
  assert.equal(OE.keptLine(OE.draftOf(null, field), field), '');
});

// The page's counts are the ones Campaign settings shows: accepted entries,
// with handed-in drafts beside them, never inside them.
const page = () => {
  const p = Object.create(OE.Entries.prototype);
  p.director = true;
  p.fields = [{ key: 'ancestry', label: 'Ancestry' }, { key: 'class', label: 'Class' }];
  p.entries = [
    { id: 1, fieldKey: 'ancestry', name: 'Hollowkin', status: 'accepted' },
    { id: 2, fieldKey: 'ancestry', name: 'Ashborn', status: 'sent', createdBy: 'u1' },
    { id: 3, fieldKey: 'ancestry', name: 'Wip', status: 'draft', createdBy: 'u1' },
  ];
  p.members = [{ user_id: 'u1', display_name: 'Mira' }];
  return p;
};

test('accepted counts exclude drafts and handed-in lists only the sent ones', () => {
  const p = page();
  assert.equal(p.accepted('ancestry').length, 1);
  assert.deepEqual(p.handedIn('ancestry').map((e) => e.id), [2]);
  assert.equal(p.accepted('class').length, 0);
});

test('the Waiting for you block names the writer and is empty when nothing waits', () => {
  const p = page();
  const html = p.draftsWaitingHTML();
  assert.match(html, /Waiting for you/);
  assert.match(html, /Ashborn/);
  assert.match(html, /A draft from Mira/);
  assert.match(html, /data-readdraft="2"/);
  assert.doesNotMatch(html, /Wip/);
  p.entries = p.entries.filter((e) => e.status !== 'sent');
  assert.equal(p.draftsWaitingHTML(), '');
});

// A fact the Director hides is listed in director_only on save, the list is
// never shown as a word list, and a hidden part the form shows otherwise
// stays hidden as stored.
test('hidden facts round-trip through director_only', () => {
  const plain = (v) => JSON.parse(JSON.stringify(v));
  const e = { id: 9, name: 'The Red Tide', visibility: 'everyone', status: 'accepted', properties: {
    benefit: 'Gain a surge.', drawback: 'Your shadow rises.', signature_traits: [{ name: 'Hunger' }], director_only: ['drawback', 'signature_traits'] } };
  const d = OE.draftOf(e, field);
  assert.deepEqual(plain(d.facts.map((p) => [p.key, p.hidden])), [['benefit', false], ['drawback', true]]);
  assert.equal(d.wordLists.some((w) => w.key === 'director_only'), false);
  assert.deepEqual(plain(OE.propsOf(d, field).props.director_only), ['drawback', 'signature_traits']);
  d.facts[0].hidden = true;
  d.facts[1].hidden = false;
  assert.deepEqual(plain(OE.propsOf(d, field).props.director_only), ['benefit', 'signature_traits']);
  d.facts[0].hidden = false;
  d.lists.signature_traits = [];
  assert.equal('director_only' in OE.propsOf(d, field).props, false);
});
