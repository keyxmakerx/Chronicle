// calendar_players_view.test.mjs — "See it as players do" on the deck reads
// the calendar as any player would, and back. Every read made meanwhile asks
// for the players' audience; what was cached for the other audience is
// dropped, and a read begun before the switch never lands in the cache.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');

const BASE = '/campaigns/c1/calendars/k1';
const dmCal = { id: 'k1', name: 'World', current_year: 100, current_month: 2, current_day: 8, moons: [{ id: 1, name: 'Selune' }, { id: 2, name: 'Secret' }], months: [{ name: 'A', days: 28 }, { name: 'B', days: 28 }] };
const playerCal = Object.assign({}, dmCal, { moons: [{ id: 1, name: 'Selune' }] });

// answer maps a requested URL to [ok, body]; reads are recorded in order.
function load(answer) {
  const defs = {}, reads = [];
  const sandbox = {
    module: { exports: {} }, console, Promise, URLSearchParams, Intl, Math,
    window: { location: { search: '' } },
    Chronicle: {
      register(name, def) { defs[name] = def; },
      apiFetch(url) {
        reads.push(url);
        return Promise.resolve(answer(url)).then(([ok, body]) => ({ ok, json: () => Promise.resolve(body) }));
      },
    },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  const v = Object.create(defs.calendar_view);
  const drawn = [];
  const classes = new Set();
  Object.assign(v, {
    apiBase: BASE, cal: dmCal, canAuthorDmOnly: true, freeDirector: false, showFree: false,
    _players: false, _audGen: 0, eventsByMonth: { '100_2': ['dm event'] }, rollsByMonth: {}, weatherByYear: { 100: {} }, _evPages: { e: {} }, _eventIndex: [],
    calEl: { classList: { toggle(c, on) { if (on) classes.add(c); else classes.delete(c); } } },
    closeAllPanels() { drawn.push('closed'); }, renderMonth() { drawn.push('month'); }, renderToday() {}, renderLegend() {},
    _skyDay() {}, say() {}, monthKey: (y, m) => y + '_' + m,
  });
  return { v, reads, drawn, classes };
}

const players = (url) => url.includes('audience=players');

test('reads ask for the players\' audience only while it is on', () => {
  const { v } = load(() => [true, []]);
  const tests = [
    { name: 'off, plain', on: false, url: BASE + '/events/index', want: BASE + '/events/index' },
    { name: 'on, plain', on: true, url: BASE + '/events/index', want: BASE + '/events/index?audience=players' },
    { name: 'on, with a query', on: true, url: BASE + '/events?year=1&month=2', want: BASE + '/events?year=1&month=2&audience=players' },
  ];
  for (const tt of tests) {
    v._players = tt.on;
    assert.equal(v._aud(tt.url), tt.want, tt.name);
  }
});

test('turning it on reloads the calendar as a player and drops the DM\'s caches', async () => {
  const { v, reads, drawn, classes } = load((url) => [true, url === BASE + '?audience=players' ? playerCal : []]);
  assert.equal(await v.setPlayersView(true), true);
  assert.equal(reads[0], BASE + '?audience=players');
  assert.equal(v._players, true);
  assert.equal(v.canAuthorDmOnly, false, 'nothing DM-only is drawn');
  assert.deepEqual(v.cal.moons.map((m) => m.name), ['Selune'], 'a moon hidden from players is gone');
  assert.equal(v.eventsByMonth['100_2'], undefined, 'the DM\'s month is not reused');
  assert.equal(v._eventIndex, null);
  assert.ok(classes.has('pv'));
  assert.deepEqual(drawn.slice(0, 2), ['closed', 'month']);
  assert.ok(reads.slice(1).every(players), 'every read after the switch is the players\' audience: ' + reads.join(', '));
});

test('turning it off brings back what this viewer sees', async () => {
  const { v, classes } = load((url) => [true, players(url) ? playerCal : (url === BASE ? dmCal : [])]);
  await v.setPlayersView(true);
  assert.equal(await v.setPlayersView(false), true);
  assert.equal(v._players, false);
  assert.equal(v.canAuthorDmOnly, true);
  assert.equal(v.cal.moons.length, 2);
  assert.ok(!classes.has('pv'));
});

test('a failed read leaves the view as it was', async () => {
  const { v, drawn } = load(() => [false, null]);
  assert.equal(await v.setPlayersView(true), false);
  assert.equal(v._players, false);
  assert.equal(v.canAuthorDmOnly, true);
  assert.deepEqual(v.eventsByMonth['100_2'], ['dm event']);
  assert.deepEqual(drawn, []);
});

test('a month read begun before the switch is not cached after it', async () => {
  let release;
  const held = new Promise((r) => { release = r; });
  const { v } = load((url) => (url.includes('/events?') && !players(url) ? held : [true, players(url) && !url.includes('/events') ? playerCal : []]));
  const dmRead = v.fetchMonth(100, 3);
  await v.setPlayersView(true);
  release([true, ['dm event']]);
  assert.deepEqual(await dmRead, ['dm event'], 'the caller still gets its answer');
  assert.equal(v.eventsByMonth['100_3'], undefined, 'but the players\' view never caches it');
});

test('it never goes on under the editor, and its read in flight holds the editor off', async () => {
  const { v, reads } = load((url) => [true, players(url) ? playerCal : []]);
  v._editor = { editing: true };
  assert.equal(await v.setPlayersView(true), false, 'editing: refused before any read');
  assert.equal(reads.length, 0);
  v._editor.editing = false;
  const p = v.setPlayersView(true);
  assert.ok(v._pvPending, 'the editor checks this before it turns on');
  assert.equal(v.setPlayersView(true), p, 'a second press waits on the same read');
  await p;
  assert.equal(v._pvPending, null);
});

test('what this viewer had is taken afresh each time it goes on', async () => {
  const { v } = load((url) => [true, players(url) ? playerCal : dmCal]);
  v.showFree = true;
  await v.setPlayersView(true);
  await v.setPlayersView(false);
  v.showFree = false; // turned the lines off meanwhile
  await v.setPlayersView(true);
  await v.setPlayersView(false);
  assert.equal(v.showFree, false);
});
