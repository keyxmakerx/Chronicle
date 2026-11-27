// A homemade entry's ability (a kit's signature ability): the form holds it
// part by part, a paste or an AI-written block fills it, and a save sends
// back exactly what the AI or the package wrote. The rules mirror the
// server's (internal/systems/entry_ability.go).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const js = (f) => readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'entities', 'static', 'js', f), 'utf8');
const { OE, OES } = (() => {
  const document = { readyState: 'complete', addEventListener() {}, querySelectorAll() { return []; } };
  const window = {};
  const sandbox = { window, document, Chronicle: { escapeHtml: (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) => '&#' + c.charCodeAt(0) + ';') }, JSON, Object, Array, String, Number };
  vm.createContext(sandbox);
  vm.runInContext(js('own_entry_editor.js'), sandbox);
  vm.runInContext(js('own_entries.js'), sandbox);
  return { OE: window.ChronicleOwnEntry, OES: window.ChronicleOwnEntries };
})();
const plain = (v) => JSON.parse(JSON.stringify(v));

// Two of the package's kits, as the plan lists them.
const pkg = [
  { name: 'Arcane Archer', properties: { speed_bonus: 0, ranged_damage_bonus: '+2/+2/+2', signature_ability: {
    name: 'Exploding Arrow', flavor: 'Your ammunition explodes with magical energy.', type: 'signature', action_type: 'Main action',
    keywords: ['Magic', 'Ranged', 'Strike', 'Weapon'], distance: 'Ranged 15', target: 'One creature or object',
    power_roll: 'Agility, Reason, Intuition, or Presence', tier1: '5 + A, R, I, or P fire damage', tier2: '7 + A, R, I, or P fire damage',
    tier3: '10 + A, R, I, or P fire damage', effect: 'One creature or object within 2 squares of the target takes fire damage.' } } },
  { name: 'Mountain', properties: { speed_bonus: 0, signature_ability: { name: 'Pain for Pain', type: 'signature', action_type: 'Main action',
    keywords: ['Melee', 'Strike', 'Weapon'], distance: 'Melee 1', target: 'One creature', power_roll: 'Might',
    tier1: '4 + M damage', tier2: '6 + M damage', tier3: '9 + M damage' } } },
];
const field = () => Object.assign(OE.shapeOf(pkg), { key: 'kit', label: 'Kit' });
const stats = ['Might', 'Agility', 'Reason', 'Intuition', 'Presence'].map((l) => ({ key: l.toLowerCase(), label: l }));
const rules = { characteristics: stats, tiers: [11, 16] };

// What the AI tools write: the kind's own example kit, as YAML decodes it.
const aiKit = () => ({ id: 9, name: 'Ember Archer', summary: 'Bow, fire and a steady hand.', visibility: 'everyone', status: 'accepted', properties: {
  speed_bonus: 1, ranged_damage_bonus: '+2/+2/+2', ranged_distance_bonus: 10, signature_ability: {
    name: 'Burning Shot', flavor: 'The arrow catches light as it flies.', type: 'signature', action_type: 'Main action',
    keywords: ['Magic', 'Ranged', 'Strike', 'Weapon'], distance: 'Ranged 10', target: 'One creature', power_roll: 'Agility or Reason',
    tier1: '3 + A or R fire damage', tier2: '5 + A or R fire damage', tier3: '8 + A or R fire damage',
    effect: 'The target takes 2 fire damage at the start of its next turn.' } } });

test('the kit field offers the signature ability its printed kits share', () => {
  const f = field();
  assert.equal(f.abilities.length, 1);
  assert.equal(f.abilities[0].key, 'signature_ability');
  assert.equal(f.abilities[0].label, 'Signature ability');
  assert.equal(f.abilities[0].type, 'signature');
  assert.deepEqual(plain(f.abilities[0].actions), ['Main action']);
  assert.ok(f.abilities[0].keywords.includes('Magic') && f.abilities[0].keywords.includes('Melee'));
  assert.ok(!f.facts.includes('signature_ability'), 'an ability is never offered as a fact');
});

test('an AI-written ability loads into the form and saves back unchanged', () => {
  const e = aiKit(), f = field();
  const d = OE.draftOf(e, f);
  assert.equal(d.abilities.length, 1);
  assert.equal(d.abilities[0].name, 'Burning Shot');
  assert.deepEqual(plain(d.abilities[0].keywords), ['Magic', 'Ranged', 'Strike', 'Weapon']);
  assert.equal(OE.keptLine(d, f), '', 'nothing is carried past the form');
  const out = OE.propsOf(d, f);
  assert.equal(out.error, undefined);
  assert.deepEqual(plain(out.props), e.properties);
  assert.deepEqual(plain(OE.problems(d, f)), []);
});

test('a printed kit rewritten keeps its ability, rules markers and all', () => {
  const e = { id: 3, name: 'Arcane Archer', status: 'accepted', properties: plain(pkg[0].properties) };
  e.properties.signature_ability.effect = 'Fire damage equal to the {@combat power-roll|power roll} characteristic.';
  const d = OE.draftOf(e, field());
  assert.deepEqual(plain(OE.propsOf(d, field()).props), e.properties);
});

test('a new kit starts with an empty ability that saves as nothing', () => {
  const f = field(), d = OE.draftOf(null, f);
  assert.equal(d.abilities.length, 1);
  assert.equal(d.abilities[0].type, 'signature');
  d.name = 'Plain Kit';
  assert.equal(OE.propsOf(d, f).props.signature_ability, undefined);
  assert.deepEqual(plain(OE.problems(d, f)), []);
});

test('typed parts save trimmed, in reading order, with the shared type', () => {
  const f = field(), d = OE.draftOf(null, f), a = d.abilities[0];
  d.name = 'Ember Archer';
  Object.assign(a, { name: ' Burning Shot ', action_type: 'Main action', distance: 'Ranged 10', power_roll: 'Agility or Reason', tier1: '3 fire damage', cost: '3', effect: '  ' });
  a.keywords = ['Magic', ' Ranged '];
  const o = OE.propsOf(d, f).props.signature_ability;
  assert.deepEqual(Object.keys(o), ['name', 'type', 'action_type', 'cost', 'keywords', 'distance', 'power_roll', 'tier1']);
  assert.equal(o.name, 'Burning Shot');
  assert.equal(o.type, 'signature');
  assert.equal(o.cost, 3, 'a cost of digits is a number');
  assert.deepEqual(plain(o.keywords), ['Magic', 'Ranged']);
  assert.deepEqual(plain(OE.checkAbility(o)), []);
});

test('an ability with parts but no name blocks the save', () => {
  const f = field(), d = OE.draftOf(null, f);
  d.name = 'Kit';
  d.abilities[0].tier1 = '3 damage';
  const p = OE.problems(d, f);
  assert.equal(p.length, 1);
  assert.equal(p[0][1], true);
  assert.match(p[0][0], /signature ability a name/);
});

test('checkAbility says what the server would refuse', () => {
  const ok = aiKit().properties.signature_ability;
  assert.deepEqual(plain(OE.checkAbility(ok)), []);
  const bad = (edit) => { const a = plain(ok); edit(a); return OE.checkAbility(a); };
  assert.match(bad((a) => { delete a.name; })[0], /name/);
  assert.match(bad((a) => { a.damage = '5'; })[0], /isn’t a part/);
  assert.match(bad((a) => { a.keywords = 'Magic'; })[0], /Keywords/);
  assert.match(bad((a) => { a.keywords = Array.from({ length: 13 }, (_, i) => 'k' + i); })[0], /Keywords/);
  assert.match(bad((a) => { a.distance = 'd'.repeat(201); })[0], /distance/);
  assert.equal(bad((a) => { a.effect = 'e'.repeat(2000); }).length, 0);
  assert.match(bad((a) => { a.effect = 'e'.repeat(2001); })[0], /effect/);
  assert.match(bad((a) => { a.cost = 100; })[0], /cost/);
  assert.match(bad((a) => { a.distance = 15; })[0], /text/);
  assert.match(OE.checkAbility([ok])[0], /object of parts/);
});

test('the form knows the same parts the server does', () => {
  const go = readFileSync(path.join(here, '..', '..', 'internal', 'systems', 'entry_ability.go'), 'utf8');
  const m = go.match(/var abilityParts = \[\]string\{([^}]*)\}/);
  const goParts = m[1].match(/"([a-z_0-9]+)"/g).map((s) => s.slice(1, -1));
  const example = JSON.parse(OE.abilityExample({ type: 'signature' }));
  assert.deepEqual(plain(OE.checkAbility(example)), []);
  for (const p of goParts) assert.deepEqual(plain(OE.checkAbility({ name: 'X', [p]: p === 'keywords' ? ['w'] : 'x' })), [], p);
  assert.match(OE.checkAbility({ name: 'X', not_a_part: 'x' })[0], new RegExp(goParts.join(', ')));
});

test('a JSON paste reads an ability, or an entry holding one', () => {
  const ab = aiKit().properties.signature_ability;
  assert.deepEqual(plain(OE.readAbility(JSON.stringify(ab), 'signature_ability').ability), ab);
  assert.deepEqual(plain(OE.readAbility('```json\n' + JSON.stringify({ signature_ability: ab }) + '\n```', 'signature_ability').ability), ab);
  assert.deepEqual(plain(OE.readAbility(JSON.stringify(aiKit()), 'signature_ability').ability), ab);
  assert.match(OE.readAbility('{name: x}', 'signature_ability').error, /isn’t JSON/);
  assert.match(OE.readAbility('', 'signature_ability').error, /Paste/);
  assert.match(OE.readAbility('{"name":"X","roll":"1"}', 'signature_ability').error, /isn’t a part/);
});

test('pressing Fill the boxes loads the paste into the form', () => {
  const f = field(), d = OE.draftOf(null, f), ui = {};
  const ab = aiKit().properties.signature_ability;
  const btn = (act) => ({ getAttribute: (k) => ({ 'data-abact': act, 'data-ab': '0' })[k] });
  const ctx = { ui, rules, shapes: f.abilities };
  assert.equal(OE.abilityClick(d, btn('paste'), ctx).next, 'redraw');
  assert.equal(ui.signature_ability.open, true);
  ui.signature_ability.text = '{"name":"X","roll":"1"}';
  OE.abilityClick(d, btn('pastego'), ctx);
  assert.equal(ui.signature_ability.bad, true);
  assert.equal(d.abilities[0].name, '', 'a refused paste leaves the boxes alone');
  ui.signature_ability.text = JSON.stringify(ab);
  const r = OE.abilityClick(d, btn('pastego'), ctx);
  assert.equal(r.next, 'redraw');
  assert.equal(ui.signature_ability.open, false);
  d.name = 'Ember Archer';
  assert.deepEqual(plain(OE.propsOf(d, f).props.signature_ability), ab);
});

test('the characteristic toggles write the roll as the book prints it', () => {
  assert.deepEqual(plain(OE.rollStats('Agility, Reason, Intuition, or Presence', stats)), ['Agility', 'Reason', 'Intuition', 'Presence']);
  assert.deepEqual(plain(OE.rollStats('Might or Agility', stats)), ['Might', 'Agility']);
  assert.deepEqual(plain(OE.rollStats('', stats)), []);
  assert.equal(OE.rollStats('Might, or the highest of the others', stats), null);
  assert.equal(OE.rollText(['Presence', 'Agility'], stats), 'Agility or Presence');
  assert.equal(OE.rollText(['Presence', 'Agility', 'Reason', 'Intuition'], stats), 'Agility, Reason, Intuition, or Presence');
  const f = field(), d = OE.draftOf(null, f);
  const tog = (s) => ({ getAttribute: (k) => ({ 'data-abact': 'stat', 'data-ab': '0', 'data-stat': s })[k], setAttribute() {} });
  OE.abilityClick(d, tog('Reason'), { ui: {}, rules });
  OE.abilityClick(d, tog('Might'), { ui: {}, rules });
  assert.equal(d.abilities[0].power_roll, 'Might or Reason');
  OE.abilityClick(d, tog('Might'), { ui: {}, rules });
  assert.equal(d.abilities[0].power_roll, 'Reason');
});

test('the tiers are named from the system’s power roll, else by number', () => {
  assert.deepEqual(plain(OE.tierNames(rules)), ['11 or lower', '12 to 16', '17 or higher']);
  assert.deepEqual(plain(OE.tierNames(null)), ['Tier 1', 'Tier 2', 'Tier 3']);
});

test('the form and the card escape what was typed', () => {
  const f = field(), d = OE.draftOf(null, f);
  Object.assign(d.abilities[0], { name: '<img src=x onerror=alert(1)>', tier1: '"><u>x', power_roll: 'Might' });
  d.abilities[0].keywords = ['<i>'];
  const html = OE.abilityFormHTML(d, { c: { f: 'f', k: 'k', hint: 'h', link: 'l' }, rules, shapes: f.abilities, ui: {}, kind: 'Kit' });
  const card = OE.abilityCardHTML(d.abilities[0], rules);
  for (const h of [html, card]) {
    assert.ok(!h.includes('<img') && !h.includes('<u>') && !h.includes('<i>'), 'typed markup is escaped');
  }
  assert.match(html, /aria-pressed="true"[^>]*>M</);
  assert.match(html, /11 or lower/);
  assert.match(card, /Add the power roll and all three tiers/);
});

// The Directors' "Paste from an AI" reads the same ability from YAML or JSON.
const entries = (f) => ({ cur: () => f, fields: [f], draft: OE.draftOf(null, f), tryBuy: [] });

test('Paste from an AI reads an ability from YAML front matter', () => {
  const f = field(), me = entries(f);
  const text = '---\nkind: system-entry\nentry: kit\nname: Ember Archer\nspeed_bonus: 1\nsignature_ability:\n  name: Burning Shot\n  type: signature\n' +
    '  keywords: [Magic, Ranged]\n  distance: Ranged 10\n  power_roll: Agility or Reason\n  tier1: "3 + A or R fire damage"\n---\nThe kit.';
  assert.equal(OES.Entries.prototype.readIn.call(me, text), '');
  const o = OE.propsOf(me.draft, f).props;
  assert.deepEqual(plain(o.signature_ability), { name: 'Burning Shot', type: 'signature', keywords: ['Magic', 'Ranged'], distance: 'Ranged 10', power_roll: 'Agility or Reason', tier1: '3 + A or R fire damage' });
  assert.equal(o.speed_bonus, 1);
});

test('Paste from an AI reads a block-style keyword list too', () => {
  const f = field(), me = entries(f);
  const text = '---\nname: Ember Archer\nsignature_ability:\n  name: Burning Shot\n  keywords:\n    - Magic\n    - Ranged\n---';
  assert.equal(OES.Entries.prototype.readIn.call(me, text), '');
  assert.deepEqual(plain(me.draft.abilities[0].keywords), ['Magic', 'Ranged']);
});

test('Paste from an AI reads a JSON entry and refuses a bad ability', () => {
  const f = field(), me = entries(f), e = aiKit();
  assert.equal(OES.Entries.prototype.readIn.call(me, JSON.stringify({ entry: 'kit', name: e.name, summary: e.summary, properties: e.properties })), '');
  assert.deepEqual(plain(OE.propsOf(me.draft, f).props), e.properties);
  const again = entries(f);
  assert.match(OES.Entries.prototype.readIn.call(again, '{"name":"K","signature_ability":{"name":"X","roll":"1"}}'), /isn’t an ability/);
});
