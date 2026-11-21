// calendar_year_track.test.mjs — pins the year track's pure logic: the draft's
// sentence and counts, the generated marker and the "edited by hand" check,
// what a re-roll may replace, weather a draw may not touch, the lit window and
// today line on unequal months, the chosen-days row, what is saved and as what
// visibility, and who is offered the generating tools.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';
import { escapeHtml } from './_escape.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const widgets = path.join(here, '..', '..', 'static', 'js', 'widgets');
const read = (f) => readFileSync(path.join(widgets, f), 'utf8');

function load() {
  const sandbox = { module: { exports: {} }, console, Chronicle: { escapeHtml }, document: {}, window: {} };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(read('chronicle_gen.js'), sandbox);
  const G = sandbox.ChronicleGen || sandbox.module.exports;
  sandbox.module = { exports: {} };
  vm.runInContext(read('calendar_year_track.js'), sandbox);
  return { G, YT: sandbox.module.exports };
}
const { G, YT } = load();

const DAYS = [30, 28, 31, 12, 30, 45];
const cal = {
  name: 'Uneven', current_year: 100, current_month: 2, current_day: 10, leap_year_every: 0,
  months: DAYS.map((days, i) => ({ name: 'M' + (i + 1), days, sort_order: i })),
  weekdays: ['One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven'].map((name, i) => ({ name, sort_order: i })),
  moons: [], seasons: [], eras: [],
};

const made = (lane, over) => {
  const e = Object.assign({ id: 'e1', name: 'Harvest fair', description: 'Stalls.', year: 100, month: 3, day: 4 }, over);
  e.payload = YT.markerFor(lane, e);
  return e;
};

// ---- Sentence and counts ----
test('a fill sentence counts what is switched on, in the signed wording', () => {
  const D = YT.blankDraft('fill', 's');
  D.ev = [{ _lane: 'events', gen: { key: 'a' }, year: 100 }, { _lane: 'events', gen: { key: 'b' }, year: 100 }, { _lane: 'sky', gen: { key: 'c' }, year: 100 }];
  D.wx = [{ year: 100, month: 2, day: 11 }];
  const ctx = { now: 'M2 10', year: '100 AS', yearOf: (n) => n + ' AS' };
  assert.equal(YT.sentence(D, ctx), 'From M2 10 to the end of 100 AS: 2 happenings, 1 sky event and 1 day of weather.');
  D.on.weather = false;
  assert.match(YT.sentence(D, ctx), /: 2 happenings and 1 sky event\.$/);
  D.on.events = false; D.on.sky = false;
  assert.equal(YT.sentence(D, ctx), 'Every part of this draft is switched off.');
});

test('counts leave out what the owner left out and what a fill switched off', () => {
  const D = YT.blankDraft('fill', 's');
  D.ev = [{ _lane: 'events', gen: { key: 'a' } }, { _lane: 'events', gen: { key: 'b' } }, { _lane: 'sky', gen: { key: 'c' } }];
  D.dropped.a = true;
  assert.deepEqual({ ...YT.counts(D) }, { events: 1, sky: 1, history: 0, copy: 0, weather: 0, all: 2 });
  D.on.sky = false;
  assert.equal(YT.counts(D).all, 1);
});

test('a history sentence says the eras stay, and its Keep reads Keep this history', () => {
  const D = YT.blankDraft('history', 's');
  D.ev = [{ _lane: 'history', gen: { key: 'a' }, year: 12 }, { _lane: 'history', gen: { key: 'b' }, year: 80 }];
  const s = YT.sentence(D, { now: '', year: '', yearOf: (n) => n + ' AS' });
  assert.equal(s, 'A history for the years before now: 2 events, from 12 AS to 80 AS. Your eras stay as they are.');
  assert.equal(YT.keepLabel(D), 'Keep this history');
  assert.equal(YT.keepLabel(YT.blankDraft('fill', 's')), 'Keep');
});

test('what Keep says it did, whole and partial', () => {
  assert.equal(YT.keptWords(19, 120), 'Kept 19 events and 120 days of weather.');
  assert.equal(YT.keptWords(1, 0), 'Kept 1 event.');
  assert.equal(YT.keptWords(0, 1), 'Kept 1 day of weather.');
  assert.equal(YT.notKeptWords(500), 'Nothing was saved. The draft is still here.');
  assert.match(YT.notKeptWords(429), /^The server is busy\. Nothing was saved/);
  assert.match(YT.notKeptWords(0), /may or may not have been saved/, 'no answer is not a refusal');
  assert.match(YT.notUndoneWords(0), /may or may not have happened/);
  assert.match(YT.notUndoneWords(500), /Nothing was changed/);
  assert.match(YT.notUndoneWords(429), /^The server is busy\. Nothing was undone/);
});

// ---- The generated marker ----
test('a generated event is recognised by its lane while it matches its sig', () => {
  for (const lane of ['events', 'sky', 'history']) assert.equal(YT.generatedLane(made(lane)), lane);
});

test('an edit to name, date or description makes the event hand-made', () => {
  const e = made('events');
  assert.equal(YT.isHandMade(e), false);
  for (const over of [{ name: 'Harvest feast' }, { description: 'Stalls and music.' }, { day: 5 }, { month: 4 }, { year: 101 }]) {
    assert.equal(YT.isHandMade(Object.assign({}, e, over)), true, JSON.stringify(over));
  }
  // Colour, icon or visibility are not part of the sig.
  assert.equal(YT.isHandMade(Object.assign({}, e, { color: '#fff', visibility: 'everyone' })), false);
});

test('no payload, a moon payload, bad JSON or an unknown lane is hand-made', () => {
  const e = made('events');
  for (const payload of [undefined, '', '{"type":"new_moon"}', 'not json', '{"type":"generated","lane":"other","sig":"x"}', '[]']) {
    assert.equal(YT.isHandMade(Object.assign({}, e, { payload })), true, String(payload));
  }
});

test('the marker carries type, lane and sig, and nothing else', () => {
  const p = JSON.parse(YT.markerFor('sky', { name: 'N', year: 1, month: 2, day: 3, description: '' }));
  assert.deepEqual(Object.keys(p).sort(), ['lane', 'sig', 'type']);
  assert.equal(p.type, 'generated');
  assert.equal(p.lane, 'sky');
});

// ---- Never replacing hand-made events ----
test('a re-roll never offers a hand-made or recurring event for replacement', () => {
  const dates = [{ year: 100, month: 3, day: 4 }];
  const mine = { id: 'h1', name: 'My feast', year: 100, month: 3, day: 4, payload: '' };
  const edited = Object.assign(made('events', { id: 'h2' }), { name: 'Edited since' });
  const recurring = Object.assign(made('events', { id: 'h3' }), { is_recurring: true });
  const gen = made('events', { id: 'g1' });
  for (let i = 0; i < 12; i++) {
    const r = YT.buildReroll(G, cal, [mine, edited, recurring, gen], dates, 'seed' + i, 'n' + i);
    const removed = r.remove.map((e) => e.id);
    assert.ok(!removed.includes('h1') && !removed.includes('h2') && !removed.includes('h3'), 'removed a protected event');
    assert.ok(removed.every((id) => id === 'g1'));
  }
});

test('a re-roll leaves a generated event on a day that was not chosen', () => {
  const far = made('events', { id: 'g2', month: 5, day: 20 });
  const r = YT.buildReroll(G, cal, [far], [{ year: 100, month: 3, day: 4 }], 's', 'n');
  assert.equal(r.remove.length, 0);
});

// ---- Weather a draw may not touch ----
test('a new fill replaces an earlier fill from today on, and nothing a person made or changed', () => {
  const now = { year: 100, month: 2, day: 10 };
  const later = made('events', { id: 'g1', month: 3, day: 4 }), today = made('sky', { id: 'g2', month: 2, day: 10 });
  const past = made('events', { id: 'g3', month: 1, day: 9 }), otherYear = made('events', { id: 'g4', year: 101, month: 3, day: 4 });
  const edited = Object.assign(made('events', { id: 'g5', month: 4, day: 2 }), { name: 'Renamed by hand' });
  const hand = { id: 'h1', name: 'Coronation', year: 100, month: 5, day: 1 }, old = made('history', { id: 'g6', month: 5, day: 2 });
  const ids = YT.fillReplaces([later, today, past, otherYear, edited, hand, old], now).map((e) => e.id);
  assert.deepEqual(ids, ['g1', 'g2']);
});

// A weekly market is stored once, on its first day. When the date has moved
// on, a new fill ends it the day before today and starts its own.
test('a new fill ends an earlier fill\'s repeat that began before today and still runs', () => {
  const now = { year: 100, month: 2, day: 10 };
  const weekly = (id, over) => made('events', Object.assign({ id, month: 1, day: 9, is_recurring: true, recurrence_type: 'weekly' }, over));
  const tests = [
    ['weekly, no end', weekly('a'), true],
    ['biweekly', weekly('a', { recurrence_type: 'biweekly' }), true],
    ['monthly', weekly('a', { recurrence_type: 'monthly' }), true],
    ['ends today or later', weekly('a', { recurrence_end_year: 100, recurrence_end_month: 2, recurrence_end_day: 10 }), true],
    ['already ended before today', weekly('a', { recurrence_end_year: 100, recurrence_end_month: 2, recurrence_end_day: 9 }), false],
    ['an every-N-weeks repeat is not one a fill makes', weekly('a', { recurrence_type: 'custom' }), false],
    ['yearly: its next day is next year', weekly('a', { recurrence_type: 'yearly' }), false],
    ['capped by a count', weekly('a', { recurrence_max_occurrences: 8 }), false],
    ['begins today or later: that one is deleted instead', weekly('a', { month: 2, day: 10 }), false],
    ['does not repeat', weekly('a', { is_recurring: false, recurrence_type: null }), false],
    ['begun in another year', weekly('a', { year: 99 }), false],
    ['a sky event counts too', Object.assign(made('sky', { id: 'a', month: 1, day: 9, is_recurring: true, recurrence_type: 'weekly' })), true],
    ['edited by hand', Object.assign(weekly('a'), { name: 'Renamed by hand' }), false],
    ['made by hand', { id: 'a', name: 'Market', year: 100, month: 1, day: 9, is_recurring: true, recurrence_type: 'weekly' }, false],
    ['a history event', made('history', { id: 'a', month: 1, day: 9, is_recurring: true, recurrence_type: 'weekly' }), false],
  ];
  for (const [name, e, want] of tests) {
    assert.equal(YT.fillRetires([e], now).length === 1, want, name);
  }
  // Nothing is both deleted and ended.
  const mixed = [weekly('w'), made('events', { id: 'later', month: 3, day: 4 }), weekly('w2', { month: 4, day: 1 })];
  const gone = YT.fillReplaces(mixed, now).map((e) => e.id), ended = YT.fillRetires(mixed, now).map((e) => e.id);
  assert.deepEqual(gone, ['later', 'w2']);
  assert.deepEqual(ended, ['w']);
});

test('the day before today follows the calendar\'s own months', () => {
  const md = (i) => DAYS[i];
  const tests = [
    [{ year: 100, month: 2, day: 10 }, { year: 100, month: 2, day: 9 }],
    [{ year: 100, month: 3, day: 1 }, { year: 100, month: 2, day: 28 }],
    [{ year: 100, month: 5, day: 1 }, { year: 100, month: 4, day: 12 }],
    [{ year: 100, month: 1, day: 1 }, null],
  ];
  for (const [now, want] of tests) assert.deepEqual(YT.dayBefore(now, md) && { ...YT.dayBefore(now, md) }, want);
  assert.deepEqual({ ...YT.dayBefore({ year: 100, month: 3, day: 1 }, (i) => (i === 1 ? 0 : 5)) }, { year: 100, month: 1, day: 5 }, 'a month with no days is stepped over');
});

test('Keep ends a repeat with an update item, and Undo gives it its old end back', () => {
  const end = { year: 100, month: 2, day: 9 };
  const items = [{ body: { name: 'new market' } }];
  const open = made('events', { id: 'm1', is_recurring: true, recurrence_type: 'weekly' });
  const capped = made('events', { id: 'm2', is_recurring: true, recurrence_type: 'weekly', recurrence_end_year: 100, recurrence_end_month: 11, recurrence_end_day: 30 });
  const body = JSON.parse(JSON.stringify(YT.keepBatchBody(items, [{ id: 'old' }], [], [open, capped], end)));
  assert.deepEqual(body.update, [
    { id: 'm1', changes: { recurrence_end_year: 100, recurrence_end_month: 2, recurrence_end_day: 9 } },
    { id: 'm2', changes: { recurrence_end_year: 100, recurrence_end_month: 2, recurrence_end_day: 9 } },
  ]);
  assert.equal('update' in YT.keepBatchBody(items, [], [], [], end), false, 'nothing to end sends no update');
  assert.equal('update' in YT.keepBatchBody(items, [], [], [open], null), false, 'no day before today ends nothing');

  const rec = { created: [{ id: 'n1' }], removed: [], wx: [], prev: {}, retired: [{ id: 'm1', was: null }, { id: 'm2', was: { year: 100, month: 11, day: 30 } }] };
  assert.deepEqual(JSON.parse(JSON.stringify(YT.undoBatchBody(rec).update)), [
    { id: 'm1', changes: { recurrence_end_year: null, recurrence_end_month: null, recurrence_end_day: null } },
    { id: 'm2', changes: { recurrence_end_year: 100, recurrence_end_month: 11, recurrence_end_day: 30 } },
  ]);
});

test('a fill that ends repeats counts them among what it replaces', () => {
  const D = YT.blankDraft('fill', 's');
  D.ev = [{ _lane: 'events', gen: { key: 'a' }, year: 100 }];
  D.remove = [made('events', { id: 'g1' })];
  D.retire = [made('events', { id: 'm1', is_recurring: true, recurrence_type: 'weekly' }), made('sky', { id: 'm2', is_recurring: true, recurrence_type: 'monthly' })];
  const ctx = { now: 'M2 10', year: '100', yearOf: String };
  assert.match(YT.sentence(D, ctx), /It replaces 3 events from an earlier fill\.$/);
  D.on.sky = false;
  assert.deepEqual(YT.retiresOf(D).map((e) => e.id), ['m1']);
  D.on.events = false;
  assert.equal(YT.retiresOf(D).length, 0);
  assert.equal(YT.retiresOf(YT.blankDraft('history', 's')).length, 0);
});

test('a fill removes an earlier fill only for the parts switched on, and says so', () => {
  const D = YT.blankDraft('fill', 's');
  D.remove = [made('events', { id: 'g1' }), made('sky', { id: 'g2', day: 5 })];
  const ctx = { now: 'M2 10', year: '100', yearOf: String };
  assert.equal(YT.removalsOf(D).length, 2);
  assert.match(YT.sentence(D, ctx), /It replaces 2 events from an earlier fill\.$/);
  D.on.sky = false;
  assert.deepEqual(YT.removalsOf(D).map((e) => e.id), ['g1']);
  assert.match(YT.sentence(D, ctx), /It replaces 1 event from an earlier fill\.$/);
  D.on.events = false;
  assert.equal(YT.removalsOf(D).length, 0);
  assert.doesNotMatch(YT.sentence(D, ctx), /replaces/);
});

test('painted and locked days are protected, an unlocked generated one is not', () => {
  assert.equal(YT.protectedDay(null), false);
  assert.equal(YT.protectedDay({ source: 'manual' }), true);
  assert.equal(YT.protectedDay({ source: 'generated', locked: true }), true);
  assert.equal(YT.protectedDay({ source: 'generated', locked: false }), false);
});

test('fill puts weather only on days that hold none; a re-roll only on unlocked generated ones', () => {
  const days = [1, 2, 3, 4].map((day) => ({ year: 100, month: 2, day }));
  const stored = { 1: { source: 'manual' }, 2: { source: 'generated', locked: true }, 3: { source: 'generated' } };
  const readingOf = (d) => stored[d.day] || null;
  assert.deepEqual(YT.emptyDays(days, readingOf).map((d) => d.day), [4]);
  assert.deepEqual(YT.redrawable(days, readingOf).map((d) => d.day), [3]);
});

test('a painted or locked day reaches the generator as a lock', () => {
  assert.deepEqual({ ...YT.asEngineDay({ source: 'manual', zone_id: 'z' }, null).gen }, { locked: true });
  assert.equal(YT.asEngineDay({ source: 'generated', zone_id: 'z' }, null).gen, undefined);
  assert.equal(YT.asEngineDay({ source: 'generated', zone_id: 'z' }, null).zone_id, null);
});

test('a draft names a day of weather only once', () => {
  const D = YT.blankDraft('fill', 's');
  D.wx = [{ year: 100, month: 2, day: 11 }, { year: 100, month: 2, day: 11 }, { year: 100, month: 2, day: 12 }];
  assert.equal(YT.activeWeather(D).length, 2);
});

test('a fill draft stays inside today to the end of the year and never hits a painted day', () => {
  for (let i = 0; i < 8; i++) {
    const D = YT.blankDraft('fill', 'seed' + i);
    const setup = YT.weatherSetup(G, null);
    const stored = { '2_20': { year: 100, month: 2, day: 20, source: 'manual', preset_id: null } };
    YT.rollFillWeather(G, cal, D, 100, stored, setup);
    D.wx.forEach((w) => {
      assert.equal(w.year, 100);
      assert.ok(w.month > 2 || (w.month === 2 && w.day >= 10), 'before today');
      assert.ok(!(w.month === 2 && w.day === 20), 'over a painted day');
    });
    YT.rollFillLane(G, cal, D, 'events');
    D.ev.forEach((e) => {
      assert.equal(e.year, 100);
      assert.ok(e.month > 2 || (e.month === 2 && e.day >= 10), 'event before today');
    });
  }
});

// ---- Lit window and today line ----
test('the lit window and today line follow day counts, not equal-width months', () => {
  const g = YT.geometry(DAYS);
  assert.equal(g.total, 176);
  const w = YT.windowFor(g, 4);
  assert.ok(Math.abs(w.at - 89 / 176) < 1e-9);
  assert.ok(Math.abs(w.span - 12 / 176) < 1e-9);
  const w6 = YT.windowFor(g, 6);
  assert.ok(Math.abs(w6.at + w6.span - 1) < 1e-9);
  assert.ok(Math.abs(YT.nowAt(g, 2, 10) - (30 + 9 + 0.5) / 176) < 1e-9);
  assert.equal(YT.nowAt(g, 1, 1), 0.5 / 176);
});

test('the window is null for a month with no days, and the line is 0 with no year', () => {
  assert.equal(YT.windowFor(YT.geometry([30, 0]), 2), null);
  assert.equal(YT.nowAt(YT.geometry([]), 1, 1), 0);
});

test('day of the year goes both ways across unequal months', () => {
  const g = YT.geometry(DAYS);
  for (const [m, d] of [[1, 1], [2, 28], [4, 12], [6, 45]]) {
    const back = g.fromDoy(g.doy(m, d));
    assert.deepEqual({ ...back }, { month: m, day: d });
  }
});

// ---- The chosen-days row ----
test('Repeat every month needs one day with one non-recurring event, and a viewer who may generate', () => {
  const ev = { id: 'a', name: 'X', year: 100, month: 2, day: 3 };
  const day = [{ year: 100, month: 2, day: 3 }];
  assert.equal(YT.chosenActions([ev], day, true).every, ev);
  assert.equal(YT.chosenActions([ev], day, false).every, null);
  assert.equal(YT.chosenActions([ev, ev], day, true).every, null);
  assert.equal(YT.chosenActions([ev], day.concat([{ year: 100, month: 2, day: 4 }]), true).every, null);
  assert.equal(YT.chosenActions([Object.assign({}, ev, { is_recurring: true })], day, true).every, null);
  assert.equal(YT.chosenActions([], day, true).every, null);
});

test('Roll again needs a chosen generated event of the happenings or sky lanes', () => {
  const day = [{ year: 100, month: 3, day: 4 }];
  assert.equal(YT.chosenActions([made('events')], day, true).again, true);
  assert.equal(YT.chosenActions([made('sky')], day, true).again, true);
  assert.equal(YT.chosenActions([made('history')], day, true).again, false);
  assert.equal(YT.chosenActions([Object.assign(made('events'), { is_recurring: true })], day, true).again, false);
  assert.equal(YT.chosenActions([Object.assign(made('events'), { name: 'Edited' })], day, true).again, false);
  assert.equal(YT.chosenActions([made('events')], day, false).again, false);
});

// ---- What is saved, and as what visibility ----
test('everything generated is saved dm_only and marked with its own sig', () => {
  const e = { name: 'Fair', description: 'd', year: 100, month: 3, day: 4, visibility: 'everyone', gen: { key: 'k' }, category: 'x', announce: 'on-the-day' };
  for (const lane of ['events', 'sky', 'history']) {
    const b = YT.createBody(e, { lane });
    assert.equal(b.visibility, 'dm_only');
    assert.equal(YT.generatedLane(b), lane);
    assert.equal(b.announced, 'on_day');
    assert.equal(b.gen, undefined);
    assert.equal(b.category, undefined);
  }
});

test('a copy keeps the visibility of the event it copies and drops the generated marker', () => {
  const src = made('events', { visibility: 'everyone' });
  assert.equal(YT.createBody(src, {}).visibility, 'everyone');
  assert.equal(YT.createBody(src, {}).payload, undefined);
  assert.equal(YT.createBody(Object.assign({}, src, { visibility: 'dm_only' }), {}).visibility, 'dm_only');
  const moon = { name: 'N', year: 1, month: 1, day: 1, payload: '{"type":"full_moon"}' };
  assert.equal(YT.createBody(moon, {}).payload, '{"type":"full_moon"}');
});

test('an Undo puts a removed generated event back with its marker', () => {
  const src = made('sky', { visibility: 'dm_only' });
  assert.equal(YT.createBody(src, { keepPayload: true }).payload, src.payload);
});

test('a name over the limit is cut to 200', () => {
  assert.equal(YT.createBody({ name: 'x'.repeat(300), year: 1, month: 1, day: 1 }, { lane: 'events' }).name.length, 200);
});

// ---- Who is offered the generating tools ----
test('only an owner who may write director-only content may generate', () => {
  assert.equal(YT.mayGenerate({ canAuthorDmOnly: true, role: 3 }), true);
  assert.equal(YT.mayGenerate({ canAuthorDmOnly: true, role: 2 }), false);
  assert.equal(YT.mayGenerate({ canAuthorDmOnly: false, role: 3 }), false);
  assert.equal(YT.mayGenerate(null), false);
});

// ---- One batch, all or nothing ----
test('Keep sends the new events, the ids they replace and the weather in one body', () => {
  const items = [{ body: { name: 'a' } }, { body: { name: 'b' } }];
  const removed = [{ id: 'old1' }, { id: 'old2' }];
  const put = [{ year: 100, month: 2, day: 11, source: 'generated' }];
  assert.deepEqual(JSON.parse(JSON.stringify(YT.keepBatchBody(items, removed, put))), {
    create: [{ name: 'a' }, { name: 'b' }], delete: ['old1', 'old2'], weather: { set: put },
  });
  assert.equal('weather' in YT.keepBatchBody(items, [], []), false, 'a draft with no weather sends no weather key');
});

test('Undo deletes what Keep made, puts the replaced events back with their marker, and restores weather', () => {
  const old = made('events', { id: 'old1' });
  const prev = { year: 100, month: 2, day: 11, preset_id: 'rain' };
  const rec = {
    created: [{ id: 'new1' }, { id: 'new2' }], removed: [old],
    wx: [{ year: 100, month: 2, day: 11 }, { year: 100, month: 2, day: 12 }], prev: { '100_2_11': prev },
  };
  const b = JSON.parse(JSON.stringify(YT.undoBatchBody(rec)));
  assert.deepEqual(b.delete, ['new1', 'new2']);
  assert.equal(b.create.length, 1);
  assert.equal(b.create[0].payload, old.payload, 'an Undo puts the generated marker back');
  assert.deepEqual(b.weather, { clear: rec.wx, set: [prev] });
  rec.wx = [];
  assert.equal('weather' in YT.undoBatchBody(rec), false);
});

test('the batch caps match the server', () => {
  const go = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'calendar', 'event_batch.go'), 'utf8');
  assert.equal(+/maxBatchEventWrites = (\d+)/.exec(go)[1], YT.BATCH_MAX_EVENTS);
  assert.equal(+/maxBatchWeatherDays = (\d+)/.exec(go)[1], YT.BATCH_MAX_DAYS);
});

test('the year track no longer carries the pause-and-retry write loop', () => {
  const src = read('calendar_year_track.js');
  assert.doesNotMatch(src, /writeEach|MAX_PAUSES|retryAfter|WRITE_AT_ONCE/);
});

test('a refill on a year whose weather is all set says so instead of counting zero days', () => {
  const D = YT.blankDraft('fill', 's');
  D.ev = [{ _lane: 'events', gen: { key: 'a' }, year: 100 }];
  const ctx = { now: 'M2 10', year: '100', yearOf: String };
  assert.equal(YT.sentence(D, ctx), 'From M2 10 to the end of 100: 1 happening and 0 sky events. Every day already has its weather.');
  D.on.events = false; D.on.sky = false;
  assert.equal(YT.sentence(D, ctx), 'From M2 10 to the end of 100: 0 days of weather.');
});

// ---- Fill: when players see what it keeps ----
test('a fill kept "On their day" is shown to everyone but held until its day; the default stays dm_only', () => {
  const e = { name: 'Fair', description: 'd', year: 100, month: 3, day: 4, gen: { key: 'k' } };
  for (const lane of ['events', 'sky']) {
    const held = YT.createBody(e, { lane });
    const day = YT.createBody(e, { lane, onDay: true });
    assert.equal(held.visibility, 'dm_only');
    assert.equal(day.visibility, 'everyone');
    assert.equal(day.announced, 'on_day');
    assert.equal(YT.generatedLane(day), lane);
    // The marker does not depend on who sees it, so Undo and a later fill still recognise it.
    assert.equal(day.payload, held.payload);
    assert.equal(YT.isHandMade(day), false);
  }
});

test('a history is never saved to show on its day: it is already past', () => {
  const b = YT.createBody({ name: 'War', year: 10, month: 1, day: 1 }, { lane: 'history', onDay: true });
  assert.equal(b.visibility, 'dm_only');
});

test('the Fill body offers "Players see them" with the default on "When I reveal them"', () => {
  const body = (D) => YT.YearTrack.prototype.makesHTML(D);
  const D = YT.blankDraft('fill', 's');
  assert.equal(D.see, 'later');
  let html = body(D);
  assert.match(html, /Players see them/);
  assert.match(html, /data-yt="see:later" aria-pressed="true">When I reveal them/);
  assert.match(html, /data-yt="see:day" aria-pressed="false">On their day/);
  D.see = 'day';
  html = body(D);
  assert.match(html, /data-yt="see:day" aria-pressed="true">On their day/);
  // Nothing to apply it to once happenings and sky events are both off.
  D.on.events = false; D.on.sky = false;
  assert.doesNotMatch(body(D), /Players see them/);
  assert.doesNotMatch(body(YT.blankDraft('history', 's')), /Players see them/);
});

test('the sentence says so only when events are set to show on their day', () => {
  const D = YT.blankDraft('fill', 's');
  D.ev = [{ _lane: 'events', gen: { key: 'a' }, year: 100 }];
  const ctx = { now: 'M2 10', year: '100 AS', yearOf: (n) => n + ' AS' };
  assert.doesNotMatch(YT.sentence(D, ctx), /on its day/);
  D.see = 'day';
  assert.match(YT.sentence(D, ctx), /Players see each event on its day\./);
});
