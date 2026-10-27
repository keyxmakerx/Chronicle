// craft_rules_logic.test.mjs — what a recipe's "When… then…" rules mean:
// static/js/widgets/craft_rules_logic.js.
//
// Pins what the rules promise a Director and their AI: a calendar is read
// into days on the same day count the calendar page uses, rules the AI writes
// are cleaned to known kinds and real names (never pointed at a guess), a
// rule says honestly when it is next on, "can't be made" and "only then"
// combine into one verdict, and every rule reads as an escaped sentence. Also
// that the built-in weathers are the calendar generator's. Runs the scripts in
// a vm, with SkyWorld loaded for the day count.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const widgets = join(root, 'static', 'js', 'widgets');

function load() {
  const window = {};
  const ctx = vm.createContext({ window, document: { documentElement: { getAttribute: () => null } } });
  vm.runInContext(readFileSync(join(widgets, 'sky_world.js'), 'utf8'), ctx);
  vm.runInContext(readFileSync(join(widgets, 'craft_rules_logic.js'), 'utf8'), ctx);
  return { R: window.CraftRules, Sky: window.SkyWorld };
}
const { R, Sky } = load();
// vm objects come from another realm, so compare them as plain JSON.
const plain = (v) => JSON.parse(JSON.stringify(v));

// Twelve months of thirty days; today is the 8th of the ninth month, year 100.
const CAL = {
  name: 'Test', mode: 'fantasy', months: ['One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven', 'Eight', 'Nine', 'Ten', 'Eleven', 'Twelve'].map((n) => ({ name: n, days: 30 })),
  moons: [
    { name: 'Pale', cycle_days: 30, phase_offset: 0, color: '#e5e7eb' },
    { name: 'Red', cycle_days: 30, phase_offset: 15, color: '#d14b3a' },
    { name: 'Veil', phase_mode: 'manual', color: '#8b5cf6', state_names: { full: 'Open', dark: 'Drawn' }, marks: [{ year: 100, month: 9, day: 1, state: 'dark' }] },
  ],
  seasons: [{ name: 'Fading', start_month: 7, start_day: 1, end_month: 9, end_day: 30 }, { name: 'Frost', start_month: 10, start_day: 1, end_month: 12, end_day: 30 }],
  current_year: 100, current_month: 9, current_day: 8,
};
const LISTS = { benches: ['Bench'], skills: ['Alchemy'], items: ['Odd draught'], recipes: ['Salts'], marks: ['Moonlit'], tables: ['Quirks'] };
function world(extra = {}) {
  return R.world(Object.assign({
    calendar: CAL, sky: Sky, lists: LISTS,
    events: [{ name: 'Fair', year: 100, month: 9, day: 15 }],
    weather: { '100-9-10': 'Rain', '100-9-12': { label: 'Ley Surge', icon: 'storm' } },
    rolls: [{ event: 'Omen', result: 'Wildfire', year: 100, month: 9, day: 12 }],
    rolled: [{ name: 'Omen', results: ['Calm', 'Wildfire'] }],
  }, extra));
}
// The phase the calendar page shows for a cycling moon on day i of the run.
function phaseOn(W, name, i) {
  const m = CAL.moons.find((x) => x.name === name);
  return R.phaseName(Sky.phaseAt(m, Sky.dayIndex(CAL, W.day(i).year, W.day(i).month, W.day(i).day)));
}
const FX = { spots: ['window', 'heat', 'work', 'bench', 'result'], looks: { dawn: {}, frost: {}, rot: {} } };
const moon = (name, phase) => ({ kind: 'moon', moon: name, phase });
const r = (when, then, more = {}) => R.cleanRule(Object.assign({ when, then }, more), FX);

test('phaseName names a phase as the calendar does: each one centred on its turning point', () => {
  assert.equal(R.phaseName(0), 'new');
  assert.equal(R.phaseName(0.0624), 'new');
  assert.equal(R.phaseName(0.0625), 'waxing crescent');
  assert.equal(R.phaseName(0.25), 'first quarter');
  assert.equal(R.phaseName(0.4375), 'full');
  assert.equal(R.phaseName(0.5624), 'full');
  assert.equal(R.phaseName(0.75), 'last quarter');
  assert.equal(R.phaseName(0.9375), 'new');
});

test('world reads days forward from today, across months, with seasons, events, weather and rolls', () => {
  const W = world();
  assert.equal(W.today.label, 'Nine 8');
  assert.equal(W.today.season, 'Fading');
  assert.equal(W.day(22).label, 'Nine 30');
  assert.equal(W.day(23).label, 'Ten 1');
  assert.equal(W.day(23).season, 'Frost');
  assert.equal(W.day(23).abs, Sky.dayIndex(CAL, 100, 10, 1));
  assert.deepEqual(plain(W.day(7).events), ['Fair']);
  assert.equal(W.day(2).weather, 'Rain');
  assert.equal(W.day(4).weather, 'Ley Surge');
  assert.equal(W.day(4).rolled.Omen, 'Wildfire');
  assert.equal(W.day(29).known, true);
  assert.equal(W.day(30).known, false);
  assert.deepEqual(plain(W.moons.map((m) => m.name)), ['Pale', 'Red', 'Veil']);
  for (let i = 0; i < 40; i++) assert.equal(R.moonOn(W, W.moons[0], W.day(i)).name, phaseOn(W, 'Pale', i));
});

test('a moon set by hand starts at the state of its latest mark, in the calendar’s own words', () => {
  const W = world();
  assert.equal(W.hand.Veil, 'dark');
  const veil = R.moonBy(W, 'Veil');
  assert.equal(veil.words.full, 'Open');
  assert.equal(veil.words.half, 'Half');
  assert.equal(R.phaseWord(W, moon('Veil', 'full')), 'Open');
});

test('weather keeps each label’s picture, the owner’s own kinds included', () => {
  const W = world({ weatherKinds: [{ name: 'Ash Bloom', icon: 'fog' }, { name: 'Bad', icon: 'lava' }, 'Plain'] });
  assert.equal(W.glyphs.Rain, 'rain');
  assert.equal(W.glyphs['Ley Surge'], 'storm');
  assert.equal(W.glyphs['Ash Bloom'], 'fog');
  assert.equal(W.glyphs.Bad, undefined);
  assert.ok(W.weatherKinds.includes('Ash Bloom') && W.weatherKinds.includes('Plain'));
  assert.deepEqual(plain(W.forecastKinds), ['Rain', 'Ley Surge']);
});

test('the built-in weathers and their pictures are the calendar generator’s, in its order', () => {
  const sandbox = { module: { exports: {} }, console, window: {} };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(join(widgets, 'chronicle_gen.js'), 'utf8'), sandbox);
  const presets = sandbox.ChronicleGen.weather.presets();
  assert.equal(R.WEATHER_TABLE.map((w) => w.join('|')).join(','), presets.map((p) => p.label + '|' + p.glyph).join(','));
  const go = readFileSync(join(root, 'internal', 'plugins', 'calendar', 'weather_kinds.go'), 'utf8');
  const labels = [...go.slice(go.indexOf('var weatherPresets'), go.indexOf('var weatherEffects')).matchAll(/Label: "([^"]+)"/g)].map((m) => m[1]);
  assert.deepEqual(plain(R.WEATHER), labels);
});

test('cleanRule reads the AI’s form and gives the same rule back, keys and all', () => {
  const ai = { when: [moon('Pale', 'full')], then: { kind: 'edge' }, players: 'book', bookLine: 'Better under a full moon.', look: { spots: { window: { look: 'dawn' } }, words: 'Full' } };
  const c = R.cleanRule(ai, FX);
  assert.equal(c.players, 'book');
  assert.equal(c.line, 'Better under a full moon.');
  assert.deepEqual(plain(R.ruleData(c)), ai);
  const shown = R.cleanRule({ when: [{ kind: 'weather', weather: 'Rain' }], then: { kind: 'extra', amount: 1 }, chance: '1 in 6', players: 'whenItHappens' }, FX);
  assert.equal(shown.players, 'shown');
  assert.equal(R.ruleData(shown).players, 'whenItHappens');
  assert.equal(R.ruleData(shown).chance, '1 in 6');
});

test('cleanRule drops what it doesn’t know and clamps what it does', () => {
  assert.equal(R.cleanRule({ when: [{ kind: 'vibes' }], then: { kind: 'edge' } }, FX), null);
  assert.equal(R.cleanRule({ when: [moon('Pale', 'full')], then: { kind: 'explode' } }, FX), null);
  assert.equal(R.cleanRule('nope', FX), null);
  const c = R.cleanRule({ when: [moon('Pale', 'gibbous-ish'), { kind: 'vibes' }, { kind: 'time', time: 'teatime' }, { kind: 'season', season: 'Frost' }, { kind: 'event', event: 'Fair' }],
    then: { kind: 'points', amount: 99, extra: 'x' }, chance: '9 in 10', players: 'everyone', script: 'alert(1)' }, FX);
  assert.deepEqual(plain(c.when), [moon('Pale', 'full'), { kind: 'time', time: 'night' }, { kind: 'season', season: 'Frost' }]);
  assert.deepEqual(plain(c.then), { kind: 'points', amount: 10 });
  assert.equal(c.chance, 'always');
  assert.equal(c.players, 'book');
  assert.equal('script' in c, false);
  assert.equal(R.cleanRule({ when: [moon('Pale', 'full')], then: { kind: 'points', amount: 0 } }, FX).then.amount, 2);
  assert.equal(R.cleanRule({ when: [moon('Pale', 'full')], then: { kind: 'extra', amount: 7 } }, FX).then.amount, 3);
});

test('"only then" is never a chance, and "every time" stands alone', () => {
  assert.equal(r([moon('Pale', 'full')], { kind: 'only' }, { chance: '1 in 4' }).chance, 'always');
  assert.deepEqual(plain(r([moon('Pale', 'full'), { kind: 'always' }], { kind: 'edge' }).when), [{ kind: 'always' }]);
});

test('a look keeps only the shared engine’s spots and looks, the two moves and short words; a secret rule has none', () => {
  const c = r([moon('Pale', 'full')], { kind: 'edge' }, { look: { spots: { window: { look: 'dawn', move: 'dim' }, heat: { look: 'lava' }, tool: { look: 'dawn' }, bench: { move: 'spin' }, result: { move: 'shake' } }, words: 'x'.repeat(80) } });
  assert.deepEqual(plain(c.look.spots), { window: { look: 'dawn', move: 'dim' }, result: { move: 'shake' } });
  assert.equal(c.look.words.length, R.LIMITS.words);
  assert.equal(r([moon('Pale', 'full')], { kind: 'edge' }, { players: 'secret', look: { spots: { window: { look: 'dawn' } } } }).look, null);
});

test('cleanRules keeps at most forty rules', () => {
  const many = Array.from({ length: 60 }, () => ({ when: [moon('Pale', 'full')], then: { kind: 'edge' } }));
  assert.equal(R.cleanRules(many, FX).length, R.LIMITS.rules);
  assert.equal(R.cleanRules('x', FX).length, 0);
});

test('a name the calendar doesn’t have keeps the rule off and says which', () => {
  const W = world();
  const bad = r([moon('Blue', 'full')], { kind: 'edge' });
  assert.deepEqual(plain(R.unknownNames(W, bad)), ['Blue']);
  assert.equal(R.ruleOn(W, bad, W.today), false);
  assert.deepEqual(plain(R.status(W, bad, 0)), { cls: 'warn', key: 'unknown', text: '“Blue” isn’t on this calendar' });
  assert.deepEqual(plain(R.unknownNames(W, r([moon('Veil', 'waxing gibbous')], { kind: 'edge' }))), ['waxing gibbous']);
  assert.deepEqual(plain(R.unknownNames(W, r([{ kind: 'rolled', event: 'Omen', result: 'Flood' }], { kind: 'edge' }))), ['Flood']);
  assert.deepEqual(plain(R.unknownNames(W, r([{ kind: 'weather', weather: 'Sprinkles' }], { kind: 'edge' }))), ['Sprinkles']);
});

test('status says when a rule is next on, counted on the calendar page’s moons', () => {
  const W = world();
  let first = -1;
  for (let i = 1; i < 60 && first < 0; i++) if (phaseOn(W, 'Pale', i) === 'full') first = i;
  const s = R.status(W, r([moon('Pale', 'full')], { kind: 'edge' }), 0);
  assert.equal(s.key, 'next');
  assert.equal(s.at, first);
  assert.match(s.text, new RegExp('in ' + first + ' days$'));
  assert.equal(R.status(W, r([moon('Pale', 'full')], { kind: 'edge' }), first).key, 'on');
  assert.equal(R.status(W, r([{ kind: 'always' }], { kind: 'edge' }), 0).text, 'On today');
});

test('a rule on a moon set by hand waits for the Director, and a day-less condition makes it "on, if…"', () => {
  const W = world();
  const veil = r([moon('Veil', 'full')], { kind: 'edge' });
  assert.equal(R.status(W, veil, 0).key, 'hand');
  assert.match(R.status(W, veil, 0).text, /Waits for you to set Veil to Open/);
  const night = r([{ kind: 'season', season: 'Fading' }, { kind: 'time', time: 'night' }], { kind: 'edge' });
  assert.equal(R.ruleOn(W, night, W.today), null);
  assert.equal(R.status(W, night, 0).text, 'On today, if made at night');
});

test('two moons that are never full together never happen; weather past the forecast is unknown, not never', () => {
  const W = world();
  const both = r([moon('Pale', 'full'), moon('Red', 'full')], { kind: 'mark', mark: 'Moonlit' });
  assert.equal(R.neverTogether(W, both), true);
  assert.equal(R.status(W, both, 0).key, 'never');
  assert.equal(R.neverTogether(W, r([moon('Pale', 'full'), moon('Red', 'new')], { kind: 'edge' })), false);
  const snow = r([{ kind: 'weather', weather: 'Snow' }], { kind: 'edge' });
  assert.equal(R.status(W, snow, 0).key, 'unknown-ahead');
  assert.match(R.status(W, snow, 0).text, /^No Snow in the weather before /);
});

test('verdict: a certain "can’t be made" wins, "only then" needs one on, and an edge cancels a bane', () => {
  const W = world();
  const always = (then, more) => r([{ kind: 'always' }], then, more);
  assert.equal(R.verdict(W, [always({ kind: 'cannot' })], 0).can, false);
  assert.equal(R.verdict(W, [always({ kind: 'cannot' }, { chance: '1 in 2' })], 0).can, true);
  assert.equal(R.verdict(W, [r([{ kind: 'event', event: 'Fair' }], { kind: 'only' })], 0).can, false);
  assert.equal(R.verdict(W, [r([{ kind: 'event', event: 'Fair' }], { kind: 'only' })], 7).can, true);
  assert.equal(R.verdict(W, [always({ kind: 'edge' }), always({ kind: 'bane' })], 0).roll, 'plain');
  assert.equal(R.verdict(W, [always({ kind: 'edge' }), always({ kind: 'edge' }), always({ kind: 'bane' })], 0).roll, 'edge');
  assert.equal(R.verdict(W, [always({ kind: 'bane' }, { chance: '1 in 4' })], 0).roll, 'plain');
  const draft = always({ kind: 'cannot' }, { draft: true });
  assert.equal(R.verdict(W, [draft], 0).can, true);
  const maybe = R.verdict(W, [r([{ kind: 'time', time: 'night' }], { kind: 'edge' })], 0);
  assert.equal(maybe.act[0].maybe, true);
});

test('a rule every one of whose days is a certain "can’t be made" day is flagged as never taking effect', () => {
  const W = world();
  const rules = [r([{ kind: 'season', season: 'Fading' }], { kind: 'cannot' }), r([{ kind: 'event', event: 'Fair' }], { kind: 'edge' }), r([{ kind: 'always' }], { kind: 'bane' })];
  assert.equal(R.cancelledBy(W, rules, 1), 0);
  assert.equal(R.cancelledBy(W, rules, 2), -1);
  assert.equal(R.cancelledBy(W, rules, 0), -1);
});

test('every rule reads as one sentence, with names escaped', () => {
  const W = world({ events: [{ name: '<b>Fair</b>', year: 100, month: 9, day: 15 }] });
  assert.equal(R.sentence(W, r([moon('Pale', 'full')], { kind: 'edge' })), 'When <b>Pale</b> is <b>full</b>, the project roll has an <b>edge</b>.');
  assert.equal(R.sentence(W, r([{ kind: 'always' }], { kind: 'extra', amount: 2 }, { chance: '1 in 6' })), 'Every time, there’s a <b>1 in 6</b> chance it makes <b>2 extra</b>.');
  assert.equal(R.sentence(W, r([{ kind: 'event', event: '<b>Fair</b>' }], { kind: 'cannot' })), 'When the calendar shows <b>&lt;b&gt;Fair&lt;/b&gt;</b>, it <b>can’t be made</b>.');
  assert.equal(R.sentence(W, r([moon('Veil', 'full')], { kind: 'say', text: 'It hums.' })), 'When <b>Veil</b> is <b>Open</b>, the bench says <b>“It hums.”</b>');
});

test('flattenEvents gives one entry per day an event falls on, skipping skipped dates', () => {
  const got = R.flattenEvents([
    { name: 'Market', occurrences: [{ year: 100, month: 9, day: 3 }, { year: 100, month: 9, day: 10, skipped: true }, { year: 100, month: 9, day: 17 }] },
    { name: 'Fair', year: 100, month: 9, day: 15 },
    { name: 'Old', year: 99, month: 9, day: 15 },
    { year: 100, month: 9, day: 1 },
  ], 100, 9);
  assert.deepEqual(plain(got.map((e) => e.name + ' ' + e.day)), ['Market 3', 'Market 17', 'Fair 15']);
  assert.deepEqual(plain(R.flattenEvents({ data: [{ name: 'A', year: 1, month: 1, day: 2 }] }, 1, 1)), [{ name: 'A', year: 1, month: 1, day: 2 }]);
  assert.deepEqual(plain(R.flattenRolls({ e1: [{ year: 1, month: 1, day: 2, result: 'Calm' }, { year: 1, month: 1, day: 3 }], e2: [{ result: 'x' }] }, { e1: 'Omen' })),
    [{ event: 'Omen', result: 'Calm', year: 1, month: 1, day: 2 }]);
});

test('starting points use the calendar’s own names and leave out what it can’t fill', () => {
  const W = world();
  const s = R.starters(W);
  assert.deepEqual(plain(s.map((x) => x.key)), ['better', 'not', 'night', 'weather', 'event', 'rolled', 'quirk']);
  assert.equal(s[0].rule.when[0].moon, 'Pale');
  assert.equal(s[1].rule.when[0].moon, 'Red');
  assert.equal(s[3].rule.when[0].weather, 'Rain');
  assert.equal(s[6].rule.when[0].moon, 'Veil');
  s.forEach((x) => assert.deepEqual(plain(R.unknownNames(W, x.rule)), [], x.key));
  const bare = R.world({ calendar: { months: [{ name: 'M', days: 30 }], current_year: 1, current_month: 1, current_day: 1 }, sky: Sky, lists: {} });
  assert.deepEqual(plain(R.starters(bare).map((x) => x.key)), ['night', 'weather']);
  assert.equal(R.defCond(bare, 'moon').moon, '');
  assert.deepEqual(plain(R.defLook(W, s[1].rule)), { spots: { window: { look: 'frost' }, heat: { move: 'dim' } }, words: 'Not tonight' });
});

test('parseAI reads one rule, a list or {rules}, fenced or not, and every rule comes back a draft', () => {
  const one = { when: [moon('Pale', 'full')], then: { kind: 'edge' } };
  assert.equal(R.parseAI(JSON.stringify(one), FX).rules[0].draft, true);
  assert.equal(R.parseAI('```json\n' + JSON.stringify([one, one]) + '\n```', FX).rules.length, 2);
  const mixed = R.parseAI(JSON.stringify({ rules: [one, { when: [{ kind: 'vibes' }], then: { kind: 'edge' } }] }), FX);
  assert.equal(mixed.rules.length, 1);
  assert.equal(mixed.dropped, 1);
  assert.equal(R.parseAI('not json', FX).error, 'json');
  assert.equal(R.parseAI('  ', FX).error, 'empty');
  assert.equal(R.parseAI('[]', FX).error, 'none');
});

test('the brief for an AI lists the calendar’s names, and its rules clean back to themselves', () => {
  const W = world();
  const rules = [r([moon('Pale', 'full')], { kind: 'edge' }, { line: 'Better.' }), r([moon('Veil', 'full')], { kind: 'quirk', table: 'Quirks' }, { players: 'secret' })];
  const b = plain(R.aiBrief(W, 'Draught', rules, FX));
  assert.deepEqual(b.calendar.moons.map((m) => m.name), ['Pale', 'Red', 'Veil']);
  assert.equal(b.calendar.moons[2].setByHand, true);
  assert.deepEqual(b.calendar.rolledEvents, [{ name: 'Omen', results: ['Calm', 'Wildfire'] }]);
  assert.deepEqual(b.look.spots, FX.spots);
  assert.deepEqual(plain(R.cleanRules(b.rules, FX)), plain(rules));
});
