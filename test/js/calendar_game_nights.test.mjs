// calendar_game_nights.test.mjs — pins how calendar_view.js shows game
// nights and takes answers: silence reads as "no answer yet" (never a no),
// each night of a series answers for its own date, stored text is escaped,
// only the organiser or owner gets the "count me" switch, an anchored world
// calendar places nights on the right world day, times read in the
// calendar's zone with the viewer's one press away, and a game-night link
// opens the right night.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');

// o.zone is the browser's own zone; o.search the page's query string;
// o.nights(url) answers a nights read.
function load(o = {}) {
  const defs = {};
  const calls = [];
  const RealDTF = Intl.DateTimeFormat;
  const sandbox = {
    module: { exports: {} },
    console,
    Promise,
    URLSearchParams,
    // A plain function, so `new Intl.DateTimeFormat(...)` still works.
    Intl: o.zone ? { DateTimeFormat: function (loc, opts) { return new RealDTF(loc, { timeZone: o.zone, ...(opts || {}) }); } } : Intl,
    window: { location: { search: o.search || '' } },
    Chronicle: {
      register(name, def) { defs[name] = def; },
      apiFetch(url, opts) {
        calls.push({ url, opts });
        const body = o.nights ? o.nights(url) : [];
        return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
      },
    },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { def: defs.calendar_view, calls, Chronicle: sandbox.Chronicle };
}

// A view with just enough state for the game-night methods; the DOM bits
// they touch after a save are stubbed out.
function view(def, nights) {
  const v = Object.create(def);
  const nullEl = { querySelector: () => null, classList: { contains: () => false }, dataset: {} };
  Object.assign(v, {
    campaignId: 'c1', cal: { mode: 'reallife', tracks_real_time: true }, view: { y: 2026, m: 10 },
    nightsByMonth: { '2026_10': nights }, eventsByMonth: {}, showNights: true, _gnNoteFor: null,
    wingEl: nullEl, evpEl: nullEl,
    _announce() {}, _toast() {}, refreshWing() {}, _paintMonth() {},
  });
  return v;
}

function night(over) {
  return Object.assign({
    sessionId: 's1', name: 'The Ashen Road', date: '2026-10-08', time: '19:00', tz: '',
    recurring: false, organizerId: 'dm', organizerName: 'Kael', past: false,
    tally: { going: 1, maybe: 0, cant: 0, noAnswer: 2 },
    roster: [
      { userId: 'dm', name: 'Kael', answer: 'yes', note: '', excluded: false },
      { userId: 'p1', name: 'Bryn', answer: '', note: '', excluded: false },
      { userId: 'p2', name: '<img src=x>', answer: '', note: 'late <b>maybe</b>', excluded: false },
    ],
    mine: { userId: 'p1', name: 'Bryn', answer: '', note: '' },
    canExclude: false,
  }, over || {});
}

test('a game night sits on its day, ahead of the events', () => {
  const { def } = load();
  const v = view(def, [night()]);
  v.eventsOnDay = () => [{ id: 'e1', name: 'Harvest', icon: '●' }];
  const html = v._marksHTML(2026, 10, 8);
  assert.ok(html.indexOf('gnm') < html.indexOf('data-ev="e1"'), 'game night mark comes first');
  assert.match(html, /data-ev="gn:s1:2026-10-08"/);
  assert.equal(v._marksHTML(2026, 10, 9).indexOf('gnm'), -1);
});

test('silence is "no answer yet", never a no', () => {
  const { def } = load();
  const v = view(def, [night()]);
  const html = v._gnRsvpHTML(night(), false);
  assert.match(html, /1 going · 0 maybe · 2 no answer yet/);
  assert.doesNotMatch(html, /aria-pressed="true"/);
  const full = v._gnRsvpHTML(night(), true);
  assert.match(full, /No answer yet/);
  assert.match(full, /A missing answer is never counted as a no/);
});

test('the day card is brief; the page adds the roster, escaped', () => {
  const { def } = load();
  const v = view(def, [night()]);
  assert.doesNotMatch(v._gnRsvpHTML(night(), false), /class="roster"/);
  const full = v._gnRsvpHTML(night(), true);
  assert.match(full, /class="roster"/);
  assert.doesNotMatch(full, /<img src=x>/);
  assert.doesNotMatch(full, /<b>maybe<\/b>/);
  assert.match(full, /Bryn \(you\)/);
  assert.match(full, /Kael · running it/);
});

test('only the organiser or owner gets the count-me switch', () => {
  const { def } = load();
  const v = view(def, [night()]);
  assert.doesNotMatch(v._gnRsvpHTML(night(), true), /data-gn-count/);
  const mine = { userId: 'dm', name: 'Kael', answer: 'yes', note: '', excluded: true };
  const html = v._gnRsvpHTML(night({ mine, canExclude: true }), true);
  assert.match(html, /data-gn-count/);
  assert.match(html, /aria-checked="false"/);
  assert.match(html, /Are you playing\?/);
});

test('a public viewer with no seat sees only the count', () => {
  const { def } = load();
  const v = view(def, [night()]);
  const html = v._gnRsvpHTML(night({ mine: null }), true);
  assert.doesNotMatch(html, /data-ans/);
  assert.doesNotMatch(html, /class="roster"/);
});

test('answering saves the stored words and recounts at once', async () => {
  const { def, calls } = load();
  const n = night();
  const v = view(def, [n]);
  v._gnSave('gn:s1:2026-10-08', { answer: 'maybe' });
  assert.equal(n.mine.answer, 'maybe');
  assert.deepEqual({ ...n.tally }, { going: 1, maybe: 1, cant: 0, noAnswer: 1 });
  assert.equal(calls[0].url, '/campaigns/c1/sessions/s1/rsvp');
  assert.equal(calls[0].opts.method, 'POST');
  assert.equal(calls[0].opts.body.status, 'tentative');
  assert.equal('occurrenceDate' in calls[0].opts.body, false, 'a one-off night answers the session itself');
});

test('a night of a series answers for its own date', () => {
  const { def, calls } = load();
  const n = night({ recurring: true, date: '2026-10-15' });
  const v = view(def, [n]);
  v._gnSave('gn:s1:2026-10-15', { answer: 'no' });
  assert.equal(calls[0].opts.body.status, 'declined');
  assert.equal(calls[0].opts.body.occurrenceDate, '2026-10-15');
});

test('clearing a note sends null, keeping the answer', () => {
  const { def, calls } = load();
  const n = night({ mine: { userId: 'p1', name: 'Bryn', answer: 'yes', note: 'late' } });
  const v = view(def, [n]);
  v._gnSave('gn:s1:2026-10-08', { note: '' });
  assert.equal(calls[0].opts.body.status, 'accepted');
  assert.equal(calls[0].opts.body.note, null);
});

test('no note is sent before there is an answer', () => {
  const { def, calls } = load();
  const v = view(def, [night()]);
  v._gnSave('gn:s1:2026-10-08', { note: 'hi' });
  assert.equal(calls.length, 0);
});

test('a time set in a zone carries its label', () => {
  const { def } = load();
  const v = view(def, []);
  assert.equal(v._gnTime(night({ time: '' })), '');
  assert.equal(v._gnTime(night()), '19:00');
  assert.match(v._gnTime(night({ tz: 'America/Chicago' })), /^19:00 (CDT|GMT-5)$/);
});

// Harptos-like: twelve 30-day months. World 1492-5-8 is real 2026-10-08.
function harptos(over) {
  return Object.assign({
    mode: 'fantasy', months: Array.from({ length: 12 }, (_, i) => ({ name: 'M' + (i + 1), days: 30 })),
    anchor_year: 1492, anchor_month: 5, anchor_day: 8, anchor_real_date: '2026-10-08T00:00:00Z',
  }, over || {});
}

function anchoredView(def, Chronicle, nights) {
  const v = view(def, []);
  v.cal = harptos();
  v._anchor = Chronicle.calendarRealAnchor(v.cal);
  v.view = { y: 1492, m: 5 };
  v.nightsByMonth = { '1492_5': nights };
  return v;
}

test('an anchored world calendar places a night on its world day', () => {
  const { def, Chronicle } = load();
  const v = anchoredView(def, Chronicle, [night()]);
  assert.equal(v._realIso(1492, 5, 8), '2026-10-08');
  assert.equal(v._realIso(1492, 6, 1), '2026-10-31');
  assert.equal(v.nightsOnDay(1492, 5, 8).length, 1);
  assert.equal(v.nightsOnDay(1492, 5, 9).length, 0);
  assert.match(v._gnBoxHTML(night()), /Real date: Thu Oct 8 · 19:00/);
});

test('without a whole anchor, a world calendar has no real dates', () => {
  const { Chronicle } = load();
  assert.equal(Chronicle.calendarRealAnchor(harptos({ anchor_day: null })), null);
  assert.equal(Chronicle.calendarRealAnchor({ mode: 'reallife', tracks_real_time: true }), null);
  const { def } = load();
  const v = view(def, [night()]);
  v.cal = harptos({ anchor_real_date: null });
  v._anchor = null;
  assert.equal(v._realIso(1492, 5, 8), '');
});

test('an anchored month asks for the real days it covers', async () => {
  const { def, calls, Chronicle } = load();
  const v = anchoredView(def, Chronicle, []);
  v.nightsByMonth = {};
  await v.fetchNights(1492, 5);
  assert.equal(calls.length, 1);
  assert.match(calls[0].url, /from=2026-10-01&to=2026-10-30$/);
});

test('times read in the calendar zone, with your own one press away', () => {
  const { def } = load({ zone: 'America/Los_Angeles' });
  const v = view(def, []);
  v.calZone = 'America/Chicago';
  const n = night({ tz: 'America/Chicago' });
  assert.equal(v._gnTime(n), '19:00 CDT');
  assert.match(v._gnZoneHTML(n), /The calendar’s time/);
  assert.match(v._gnZoneHTML(n), /Show in my time \(PDT\)/);
  v._gnSetZoneMode('mine');
  assert.equal(v._gnTime(n), '17:00 PDT');
  assert.match(v._gnZoneHTML(n), /Your time.*Show the calendar’s time/);
  // A night set with no zone of its own is read in the calendar's.
  assert.equal(v._gnTime(night({ tz: '' })), '17:00 PDT');
});

test('no switch when your zone reads the same as the calendar', () => {
  const { def } = load({ zone: 'America/Chicago' });
  const v = view(def, []);
  v.calZone = 'America/Chicago';
  assert.equal(v._gnZoneHTML(night({ tz: 'America/Chicago' })), '');
});

test('a night that falls on another day in your zone says which', () => {
  const { def } = load({ zone: 'Europe/Berlin' });
  const v = view(def, []);
  v.calZone = 'America/Chicago';
  v._gnZoneMode = 'mine';
  assert.match(v._gnTime(night({ tz: 'America/Chicago' })), /^Fri 02:00 /);
});

test('a game-night link names the night, or asks for the next one', () => {
  const cases = [
    ['?night=s1&date=2026-10-08', { sessionId: 's1', date: '2026-10-08' }],
    ['?night=next', { next: true }],
    ['?night=s1&date=soon', { next: true }],
    ['', null],
  ];
  for (const [search, want] of cases) {
    const { def } = load({ search });
    const v = view(def, []);
    assert.deepEqual(v._linkTarget() && { ...v._linkTarget() }, want, search);
  }
  const { def } = load({ search: '?night=next' });
  const v = view(def, []);
  v.showNights = false;
  assert.equal(v._linkTarget(), null, 'a viewer shown no nights is not jumped anywhere');
});

test('the next night is the first not yet played, across months', async () => {
  const { def } = load({
    nights: (url) => url.includes('from=2026-10-01') ? [night({ date: '2026-10-02', past: true })]
      : url.includes('from=2026-11-01') ? [night({ sessionId: 's2', date: '2026-11-20' }), night({ sessionId: 's3', date: '2026-11-06' })] : [],
  });
  const v = view(def, []);
  v.cal = { mode: 'reallife', tracks_real_time: true, current_year: 2026, current_month: 10, months: Array.from({ length: 12 }, () => ({ days: 31 })) };
  v.nightsByMonth = {};
  const n = await v._nextNight();
  assert.equal(n.sessionId, 's3');
});
