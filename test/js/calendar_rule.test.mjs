// calendar_rule.test.mjs — pins calendar_rule.js: the map between the
// drawer's rows and the API's recurrence_rule, the plain sentence, the
// ready-made rules and the checks before saving. The script is pure (no DOM),
// so it runs here as the browser loads it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_rule.js'), 'utf8');

// Run in this realm (not a vm context) so deepEqual compares plain objects.
function load() {
  globalThis.window = globalThis;
  globalThis.module = { exports: {} };
  vm.runInThisContext(src);
  const api = globalThis.module.exports;
  delete globalThis.module;
  return api;
}
const R = load();

const cal = {
  weekdays: ['Moonday', 'Tideday', 'Earthday', 'Kingsday', 'Fireday', 'Starday', 'Sunday'].map((name) => ({ name })),
  months: ['Deepwinter', 'Thawmoon', 'Seedtime', 'Rainfall', 'Blossom', 'Midsummer', 'Highsun', 'Goldfield', 'Harvestide']
    .map((name) => ({ name, days: 30 })),
  moons: [{ id: 3, name: 'Luna' }, { id: 4, name: 'Selûne', hidden_from_players: true }],
  seasons: [{ id: 7, name: 'Spring' }, { id: 8, name: 'Summer' }]
};
const events = [{ id: 'e-mask', name: 'the Festival of Masks' }];
const env = { cal, events, hiddenOk: true };

const full = { kind: 'moon_phase', moon_id: 3, phase: 'full' };

test('rule <-> form round trip, one case per condition kind', () => {
  const cases = [
    { match: [full] },
    { match: [{ kind: 'weekday', weekday: 5 }] },
    { match: [{ kind: 'weekday', weekdays: [0, 1, 2, 3, 4] }] },
    { match: [{ kind: 'nth_weekday', n: 3, weekday: 3 }] },
    { match: [{ kind: 'nth_weekday', n: -1, weekday: 5 }] },
    { match: [{ kind: 'day_of_month', day: 13 }] },
    { match: [{ kind: 'day_of_month', day: -1 }] },
    { match: [{ kind: 'month', month: 9 }] },
    { match: [{ kind: 'season_start', season_id: 7 }] },
    { match: [{ kind: 'season_start' }] },
    { match: [{ kind: 'relative_to_event', event_id: 'e-mask' }], offset_days: 2 },
    { match: [full, { kind: 'weekday', weekday: 5 }], every: 3, offset_days: -1 }
  ];
  for (const rule of cases) {
    assert.deepEqual(R.toRule(R.fromRule(rule)), rule, JSON.stringify(rule));
  }
});

test('defaults are left out of the rule, as the API stores them', () => {
  const form = { conds: [{ k: 'moon', ph: 'full', moon: 3 }], every: 1, shift: 0, dir: 'later' };
  assert.deepEqual(R.toRule(form), { match: [full] });
  assert.deepEqual(R.toRule({ ...form, every: 2, shift: 4, dir: 'earlier' }), { match: [full], every: 2, offset_days: -4 });
});

test('a single weekday sends weekday, several send weekdays in order', () => {
  assert.deepEqual(R.toRule({ conds: [{ k: 'wd', wds: [4] }], every: 1, shift: 0 }).match[0], { kind: 'weekday', weekday: 4 });
  assert.deepEqual(R.toRule({ conds: [{ k: 'wd', wds: [4, 1] }], every: 1, shift: 0 }).match[0], { kind: 'weekday', weekdays: [1, 4] });
});

test('a condition the rows cannot edit survives unchanged as a raw row', () => {
  const rule = { match: [{ kind: 'after_event', event_id: 'e-mask', days: 3 }, { kind: 'months', months: [1, 2] }, { kind: 'season', season_id: 8 }] };
  const form = R.fromRule(rule);
  assert.deepEqual(form.conds.map((c) => c.k), ['raw', 'raw', 'raw']);
  assert.deepEqual(R.toRule(form), rule);
  // A one-entry months list is the single-month row.
  assert.deepEqual(R.fromRule({ match: [{ kind: 'months', months: [4] }] }).conds, [{ k: 'month', m: 4 }]);
});

test('fromRule does not share state with its input', () => {
  const rule = { match: [{ kind: 'weekday', weekdays: [1, 2] }] };
  const form = R.fromRule(rule);
  form.conds[0].wds.push(5);
  assert.deepEqual(rule.match[0].weekdays, [1, 2]);
});

test('summary sentences', () => {
  const f = (conds, extra) => ({ conds, every: 1, shift: 0, dir: 'later', ...extra });
  const moon = { k: 'moon', ph: 'full', moon: 3 };
  const cases = [
    [f([moon]), 'Every full moon of Luna.'],
    [f([moon], { every: 2 }), 'Every other full moon of Luna.'],
    [f([moon], { every: 3 }), 'Every 3rd full moon of Luna.'],
    [f([{ k: 'nth', n: 3, wd: 3 }]), 'The 3rd Kingsday of every month.'],
    [f([{ k: 'nth', n: -1, wd: 5 }]), 'The last Starday of every month.'],
    [f([{ k: 'nth', n: 3, wd: 3 }], { every: 2 }), 'Every other 3rd Kingsday of the month.'],
    [f([{ k: 'dom', d: -1 }]), 'The last day of every month.'],
    [f([moon, { k: 'wd', wds: [5] }]), 'Every full moon of Luna, only when it falls on a Starday.'],
    [f([{ k: 'dom', d: 13 }, { k: 'wd', wds: [4] }]), 'The 13th of every month, only when it falls on a Fireday.'],
    [f([{ k: 'wd', wds: [4] }, { k: 'month', m: 9 }]), 'Every Fireday, only when it falls in Harvestide.'],
    [f([moon], { shift: 1, dir: 'earlier' }), '1 day before every full moon of Luna.'],
    [f([{ k: 'event', event: 'e-mask' }], { shift: 2 }), '2 days after the Festival of Masks.'],
    [f([{ k: 'event', event: 'e-mask' }]), 'The same days as the Festival of Masks.'],
    [f([{ k: 'wd', wds: [0, 1, 2, 3, 4] }]), 'Every Moonday, Tideday, Earthday, Kingsday or Fireday.'],
    [f([{ k: 'season', season: 7 }]), 'The first day of Spring.'],
    [f([{ k: 'season', season: 0 }]), 'The first day of every season.'],
    [f([{ k: 'season', season: 0 }], { every: 2 }), 'Every other first day of a season.'],
    [f([{ k: 'wd', wds: [4] }, { k: 'season', season: 0 }]), 'Every Fireday, only when it falls on the first day of a season.'],
    [f([]), 'Add a condition to start.']
  ];
  for (const [form, want] of cases) assert.equal(R.summary(form, env), want);
});

test('an unknown event or moon is named plainly, never "undefined"', () => {
  const s = R.summary({ conds: [{ k: 'event', event: 'gone' }], every: 1, shift: 2, dir: 'later' }, env);
  assert.equal(s, '2 days after another event.');
  assert.doesNotMatch(R.summary({ conds: [{ k: 'moon', ph: 'new', moon: 99 }], every: 1, shift: 0 }, env), /undefined/);
});

test('summaryInline reads after "Repeats:"', () => {
  assert.equal(R.summaryInline({ conds: [{ k: 'moon', ph: 'full', moon: 3 }], every: 1, shift: 0 }, env), 'every full moon of Luna');
});

test('ready-made rules use the calendar\'s own names and include the extras', () => {
  const ids = R.presets(env).map((p) => p.id);
  for (const id of ['full-moon', 'new-moon', 'before-full', 'third-full', 'first-wd', 'last-wd', 'last-day', 'season-start', 'weekdays-only', 'after-event']) {
    assert.ok(ids.includes(id), id + ' offered');
  }
  const by = Object.fromEntries(R.presets(env).map((p) => [p.id, p]));
  assert.equal(by['full-moon'].label, 'Every full moon of Luna');
  assert.equal(by['first-wd'].label, 'The first Moonday of every month');
  assert.equal(by['weekdays-only'].label, 'Weekdays only (first five weekdays)');
  assert.deepEqual(R.toRule(by['weekdays-only'].form), { match: [{ kind: 'weekday', weekdays: [0, 1, 2, 3, 4] }] });
  assert.deepEqual(R.toRule(by['before-full'].form), { match: [full], offset_days: -1 });
  assert.deepEqual(R.toRule(by['first-wd'].form), { match: [{ kind: 'nth_weekday', n: 1, weekday: 0 }] });
  assert.equal(R.summary(by['before-full'].form, env), '1 day before every full moon of Luna.');
});

test('a calendar with no moons, seasons or events hides those kinds and rules', () => {
  const bare = { cal: { weekdays: cal.weekdays, months: cal.months }, events: [], hiddenOk: true };
  const ids = R.presets(bare).map((p) => p.id);
  for (const id of ['full-moon', 'new-moon', 'before-full', 'third-full', 'full-on-wd', 'after-event', 'season-start']) assert.ok(!ids.includes(id), id + ' hidden');
  assert.deepEqual(R.availableKinds(bare, { conds: [] }), ['wd', 'nth', 'dom', 'month']);
  // A kind an existing rule already uses stays selectable.
  assert.ok(R.availableKinds(bare, { conds: [{ k: 'moon' }] }).includes('moon'));
});

test('weekdays-only needs a week longer than five days', () => {
  const short = { cal: { weekdays: cal.weekdays.slice(0, 5), months: cal.months }, events: [] };
  assert.ok(!R.presets(short).some((p) => p.id === 'weekdays-only'));
});

test('a moon hidden from players is not offered to a viewer who cannot name it', () => {
  assert.deepEqual(R.moonsOf({ cal, hiddenOk: false }).map((m) => m.id), [3]);
  assert.deepEqual(R.moonsOf({ cal, hiddenOk: true }).map((m) => m.id), [3, 4]);
});

test('presetFor recognises a ready-made rule and calls an edited one your own', () => {
  const p = R.presetForm('full-moon', env);
  assert.equal(R.presetFor(p, env), 'full-moon');
  p.shift = 1;
  // One day earlier is the "day before" ready-made rule only with its own direction.
  assert.equal(R.presetFor(p, env), null);
  p.conds.push({ k: 'wd', wds: [2] });
  assert.equal(R.presetFor(p, env), null);
});

test('a stored rule that equals a ready-made one opens as it', () => {
  assert.equal(R.presetFor(R.fromRule({ match: [full], every: 3 }), env), 'third-full');
});

test('validate: the first thing wrong, in words', () => {
  const ok = { conds: [{ k: 'wd', wds: [1] }], every: 1, shift: 0, dir: 'later' };
  assert.equal(R.validate(ok, env), null);
  assert.match(R.validate({ ...ok, conds: [] }, env), /at least one condition/);
  assert.match(R.validate({ ...ok, conds: Array(7).fill({ k: 'wd', wds: [1] }) }, env), /at most 6/);
  assert.match(R.validate({ ...ok, conds: [{ k: 'moon', ph: 'full', moon: 99 }] }, env), /Pick a moon/);
  assert.match(R.validate({ ...ok, conds: [{ k: 'event', event: '' }] }, env), /Pick an event/);
  assert.match(R.validate({ ...ok, conds: [{ k: 'wd', wds: [9] }] }, env), /Pick a weekday/);
  assert.match(R.validate({ ...ok, conds: [{ k: 'month', m: 40 }] }, env), /Pick a month/);
  assert.match(R.validate({ ...ok, every: 100 }, env), /between 1 and 99/);
  assert.match(R.validate({ ...ok, shift: 366 }, env), /at most 365/);
  // A moon the viewer may not name is refused here as the server refuses it.
  assert.match(R.validate({ ...ok, conds: [{ k: 'moon', ph: 'full', moon: 4 }] }, { ...env, hiddenOk: false }), /Pick a moon/);
});

test('defaultCond opens on something real for this calendar', () => {
  assert.deepEqual(R.defaultCond('moon', env, {}), { k: 'moon', ph: 'full', moon: 3 });
  assert.deepEqual(R.defaultCond('wd', env, { wd: 5 }), { k: 'wd', wds: [5] });
  assert.deepEqual(R.defaultCond('wd', env, { wd: 40 }), { k: 'wd', wds: [6] });
  assert.deepEqual(R.defaultCond('month', env, { m: 99 }), { k: 'month', m: 9 });
  assert.deepEqual(R.defaultCond('season', env, {}), { k: 'season', season: 0 });
  assert.deepEqual(R.defaultCond('event', env, {}), { k: 'event', event: 'e-mask' });
});

test('"any season" validates with seasons present and is a ready-made rule', () => {
  const f = { conds: [{ k: 'season', season: 0 }], every: 1, shift: 0, dir: 'later' };
  assert.equal(R.validate(f, env), null);
  assert.equal(R.presetFor(f, env), 'season-start');
  assert.deepEqual(R.toRule(R.presetForm('season-start', env)), { match: [{ kind: 'season_start' }] });
  assert.match(R.validate({ ...f, conds: [{ k: 'season', season: 99 }] }, env), /Pick a season/);
});
