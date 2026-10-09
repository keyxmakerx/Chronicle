// calendar_game_nights.test.mjs — pins how calendar_view.js shows game
// nights and takes answers: silence reads as "no answer yet" (never a no),
// each night of a series answers for its own date, stored text is escaped,
// only the organiser or owner gets the "count me" switch, a world calendar
// places tonight's night on its own today, times read in the
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
  return { def: defs.calendar_view, calls, Chronicle: sandbox.Chronicle, exp: sandbox.module.exports };
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
  assert.equal(v._gnTime(night()), '7pm');
  assert.match(v._gnTime(night({ tz: 'America/Chicago' })), /^7pm (CDT|GMT-5)$/);
});

// Harptos-like: twelve 30-day months, its today 1492-5-8. The real today
// in these tests is 2026-10-08.
const REAL_TODAY = Date.UTC(2026, 9, 8, 12);
function harptos(over) {
  return Object.assign({
    mode: 'fantasy', months: Array.from({ length: 12 }, (_, i) => ({ name: 'M' + (i + 1), days: 30 })),
    current_year: 1492, current_month: 5, current_day: 8,
  }, over || {});
}

function anchoredView(def, Chronicle, nights, over) {
  const v = view(def, []);
  v.cal = harptos(over);
  v._anchor = Chronicle.calendarRealAnchor(v.cal, REAL_TODAY);
  v.view = { y: 1492, m: 5 };
  v.nightsByMonth = { '1492_5': nights };
  return v;
}

test('a world calendar places a night beside its own today', () => {
  const { def, Chronicle } = load();
  const v = anchoredView(def, Chronicle, [night()]);
  assert.equal(v._realIso(1492, 5, 8), '2026-10-08');
  assert.equal(v._realIso(1492, 6, 1), '2026-10-31');
  assert.equal(v._todayIso(), '2026-10-08');
  assert.equal(v.nightsOnDay(1492, 5, 8).length, 1);
  assert.equal(v.nightsOnDay(1492, 5, 9).length, 0);
  assert.match(v._gnBoxHTML(night()), /Real date: Thu Oct 8 · 7pm/);
});

test('moving the world’s date moves tonight’s night with it', () => {
  const { def, Chronicle } = load();
  // The story jumped ahead to 1492-7-20; tonight is still 2026-10-08.
  const v = anchoredView(def, Chronicle, [], { current_month: 7, current_day: 20 });
  assert.equal(v._realIso(1492, 7, 20), '2026-10-08');
  assert.equal(v._realIso(1492, 7, 27), '2026-10-15');
  assert.equal(v._realIso(1492, 5, 8), '2026-07-28');
});

test('a world calendar with no date, or a real-world one, has no anchor', () => {
  const { Chronicle } = load();
  assert.equal(Chronicle.calendarRealAnchor(harptos({ current_day: 0 }), REAL_TODAY), null);
  assert.equal(Chronicle.calendarRealAnchor({ mode: 'reallife', tracks_real_time: true }, REAL_TODAY), null);
  const { def } = load();
  const v = view(def, [night()]);
  v.cal = harptos();
  v._anchor = null;
  assert.equal(v._realIso(1492, 5, 8), '');
});

test('a world month asks for the real days it covers', async () => {
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
  assert.equal(v._gnTime(n), '7pm CDT');
  assert.match(v._gnZoneHTML(n), /The calendar’s time/);
  assert.match(v._gnZoneHTML(n), /Show in my time \(PDT\)/);
  v._gnSetZoneMode('mine');
  assert.equal(v._gnTime(n), '5pm PDT');
  assert.match(v._gnZoneHTML(n), /Your time.*Show the calendar’s time/);
  // A night set with no zone of its own is read in the calendar's.
  assert.equal(v._gnTime(night({ tz: '' })), '5pm PDT');
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
  assert.match(v._gnTime(night({ tz: 'America/Chicago' })), /^Fri 2am /);
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

// Who's free: a week as the availability overlay sends it, for the
// Director (with names and lanes).
function overlay(over) {
  const days = Array.from({ length: 7 }, (_, i) => ({
    date: '2026-10-' + String(5 + i).padStart(2, '0'),
    hours: Array.from({ length: 24 }, (_, h) => ({ free: 0 })),
  }));
  // Thu Oct 8 (day 3): everyone 19-22, Jack all day, Julie 18-22.
  const lanes = {
    jack: [{ day: 3, start: 0, end: 1440 }],
    julie: [{ day: 3, start: 1080, end: 1320 }],
    bryn: [{ day: 3, start: 1140, end: 1380 }],
  };
  for (let h = 0; h < 24; h++) {
    let n = 1; // Jack
    if (h >= 18 && h < 22) n++;
    if (h >= 19 && h < 23) n++;
    days[3].hours[h].free = n;
  }
  return Object.assign({
    weekStart: '2026-10-05', totalMembers: 4, includeDetail: true, days,
    members: [
      { userId: 'jack', name: 'Jack', hasAnswered: true, lanes: lanes.jack },
      { userId: 'julie', name: 'Julie', hasAnswered: true, lanes: lanes.julie },
      { userId: 'bryn', name: 'Bryn', hasAnswered: true, lanes: lanes.bryn },
      { userId: 'dee', name: 'Dee', hasAnswered: false, lanes: [] },
    ],
  }, over || {});
}

function freeView(def, ov) {
  const v = view(def, []);
  v.cal = { mode: 'reallife', tracks_real_time: true, current_year: 2026, current_month: 10, current_day: 3, months: Array.from({ length: 12 }, (_, i) => ({ name: 'M' + (i + 1), days: 31 })) };
  v.freeCan = true; v.freeDirector = true; v.showFree = true; v.role = 3;
  v.freeByDate = {}; v._freeWeeks = {};
  if (ov) v._fileFreeWeek(ov);
  return v;
}

test('each day draws one line per player, green where they are free', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  const html = v._freeCellHTML(2026, 10, 8);
  assert.equal((html.match(/class="ln"/g) || []).length, 4);
  assert.match(html, /left:0\.00%;width:100\.00%/, 'Jack is free all day');
  assert.match(html, /left:75\.00%;width:16\.67%/, 'Julie is free 6pm to 10pm');
  v.showFree = false;
  assert.equal(v._freeCellHTML(2026, 10, 8), '', 'nothing drawn with the switch off');
});

test('the band marks only the hours when everyone is free', () => {
  const { def } = load();
  const ov = overlay({ totalMembers: 3, members: overlay().members.slice(0, 3) });
  const v = freeView(def, ov);
  const html = v._freeCellHTML(2026, 10, 8);
  assert.match(html, /class="fband" style="left:79\.17%;width:12\.50%"/, '7pm to 10pm');
  assert.doesNotMatch(v._freeCellHTML(2026, 10, 9), /class="fband"/);
});

test('a player who never painted hours does not hide the band', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  assert.match(v._freeCellHTML(2026, 10, 8), /class="fband" style="left:79\.17%;width:12\.50%"/, 'Dee has not answered');
});

test('hovering names each player and their hours', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  const html = v._freeGlanceHTML('2026-10-08');
  assert.match(html, /Jack<\/span><span>free all day/);
  assert.match(html, /Julie<\/span><span>6pm to 10pm/);
  assert.match(html, /Dee<\/span><span>hasn’t painted hours yet/);
});

test('a player sees counts, never names', () => {
  const { def } = load();
  const ov = overlay({ includeDetail: false, members: [] });
  const v = freeView(def, ov);
  v.freeDirector = false; v.showFree = false; v.role = 1;
  assert.equal(v._freeCellHTML(2026, 10, 8), '');
  const wing = v._freeWingHTML({ y: 2026, m: 10, d: 8 });
  assert.match(wing, /3 of 4 free 7pm to 10pm/);
  assert.doesNotMatch(wing, /Julie|class="flines"/);
  assert.match(wing, /<button type="button" class="lnk" data-pl-mine>Change my usual hours/, 'opens My hours in the calendar');
  assert.doesNotMatch(wing, /href="[^"]*\/availability/, 'never leaves the calendar');
  assert.doesNotMatch(wing, /Plan a game night/);
});

test('Best times picks the strongest three-hour slot, with who is missing', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  const best = v.bestTimes(2026, 10);
  assert.equal(best.length, 1);
  assert.equal(best[0].iso, '2026-10-08');
  assert.deepEqual({ ...best[0].w }, { start: 19, end: 22, free: 3 });
  const html = v._fvDirectorHTML(2026, 10, 'October');
  assert.match(html, /Best times in October/);
  assert.match(html, /Thu Oct 8<\/b>, 7pm to 10pm/);
  assert.match(html, /3 of 4 free · Dee hasn’t painted hours/);
  assert.match(html, /data-best-plan="2026-10-08" data-plan-at="19"/, 'Plan it plans inside the calendar');
});

// A month of free hours filed straight into the view: who is free on each
// date, as [start hour, end hour] per player.
function fileMonth(v, byDate) {
  const people = ['ana', 'bo', 'cy'];
  Object.keys(byDate).forEach((iso) => {
    const hours = Array(24).fill(0);
    const members = people.map((id) => {
      const r = byDate[iso][id];
      if (r) for (let h = r[0]; h < r[1]; h++) hours[h]++;
      return { userId: id, name: id[0].toUpperCase() + id.slice(1), answered: true, segs: r ? [[r[0] * 60, r[1] * 60, false]] : [] };
    });
    v.freeByDate[iso] = { total: 3, detail: true, hours, members };
  });
}

test('Best times counts a window only up to a game night’s length', () => {
  const { def } = load();
  const v = freeView(def);
  const eve = [18, 22], day = [9, 17];
  // Mon Oct 5: everyone free all working day. Sat Oct 10 and 17: everyone
  // free for the evening, every week. The long Monday used to win on length.
  fileMonth(v, {
    '2026-10-05': { ana: day, bo: day, cy: day },
    '2026-10-12': { ana: day, bo: day },
    '2026-10-10': { ana: eve, bo: eve, cy: eve },
    '2026-10-17': { ana: eve, bo: eve, cy: eve },
  });
  const best = JSON.parse(JSON.stringify(v.bestTimes(2026, 10).map((b) => b.iso)));
  assert.deepEqual(best, ['2026-10-10', '2026-10-17', '2026-10-05'], 'the every-week Saturdays outrank one long Monday');
});

test('a weekday that is best overall says who it loses on which dates', () => {
  const { def } = load();
  const v = freeView(def);
  const eve = [18, 22];
  // Saturdays: Cy plays every other week. Thursdays: everyone, once.
  fileMonth(v, {
    '2026-10-10': { ana: eve, bo: eve, cy: eve },
    '2026-10-17': { ana: eve, bo: eve },
    '2026-10-24': { ana: eve, bo: eve, cy: eve },
    '2026-10-31': { ana: eve, bo: eve },
    '2026-10-08': { ana: eve, bo: eve, cy: eve },
  });
  const wk = v.bestWeekday(2026, 10);
  assert.equal(wk.weekday, 'Saturday');
  assert.deepEqual(JSON.parse(JSON.stringify(wk.away)), [{ name: 'Cy', dates: ['Oct 17', 'Oct 31'] }]);
  v._freeRoster = () => [{ userId: 'ana', name: 'Ana', answered: true }];
  assert.match(v._fvDirectorHTML(2026, 10, 'October'), /<b>Saturdays<\/b> are best overall, 6pm to 10pm\. Cy is away Oct 17 and Oct 31\./);
});

test('a run marked as a best time keeps its own colour in the lines', () => {
  const { def } = load();
  const ov = overlay();
  ov.members[1].lanes = [{ day: 3, start: 1080, end: 1200, state: 'available' }, { day: 3, start: 1200, end: 1320, state: 'preferred' }];
  const v = freeView(def, ov);
  const html = v._freeCellHTML(2026, 10, 8);
  assert.match(html, /<b class="p" style="left:83\.33%;width:8\.33%">/);
  assert.match(v._freeGlanceHTML('2026-10-08'), /Julie<\/span><span>6pm to 8pm, 8pm to 10pm \(best\)/);
});

test('the Director plans a game night inside the day’s card', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  v.calZone = 'America/Chicago';
  let wing = v._freeWingHTML({ y: 2026, m: 10, d: 8 });
  assert.match(wing, /<button type="button" class="btn sm" data-plan="2026-10-08" data-plan-at="19">.*Plan a game night at 7pm/);
  v._planFor = { iso: '2026-10-08', start: 19 };
  wing = v._freeWingHTML({ y: 2026, m: 10, d: 8 });
  assert.match(wing, /<form class="gnplan" data-plan-form="2026-10-08">/);
  assert.match(wing, /Starts \(CDT\)<\/span><input type="time" name="time" value="19:00"/);
  assert.match(wing, /<button type="button" class="lnk" data-plan-more>More options<\/button>/, 'More options opens the game night editor');
  assert.doesNotMatch(wing, /\/sessions\?plan_date/, 'never the Sessions page');
  assert.deepEqual({ ...v._planFields('2026-10-08', { name: ' Night one ', time: '19:30', repeat: 'weekly' }) }, {
    name: 'Night one', scheduled_date: '2026-10-08', scheduled_time: '19:30', scheduled_tz: 'America/Chicago',
    is_recurring: '1', recurrence_type: 'weekly',
  });
  assert.equal(v._planFields('2026-10-08', { name: 'x', time: '19:00', repeat: '' }).is_recurring, undefined);
});

test('the Director’s Who’s free view lists the players, a reminder and the full planner', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  const html = v._fvDirectorHTML(2026, 10, 'October');
  assert.match(html, /Jack<\/span><span class="ok">Hours given/);
  assert.match(html, /Dee<\/span><span class="wait">No hours yet/);
  assert.match(html, /data-best-nudge><i class="fa-solid fa-bell"><\/i> Remind the 1 without hours/);
  assert.match(html, /<input type="checkbox" data-fv-lines checked>/, 'the lines switch, on');
  assert.match(html, /data-fv-planner="team">.*Open the full planner/, 'the full planner, inside the calendar');
});

test('a player’s Who’s free view asks for their hours until they give them', () => {
  const { def } = load();
  const v = freeView(def, overlay({ includeDetail: false, members: [] }));
  v.freeDirector = false; v.role = 1;
  v.mine = { answered: false, blocks: [] };
  let html = v._fvPlayerHTML(2026, 10);
  assert.match(html, /You haven’t given your hours yet/);
  assert.match(html, /<button type="button" class="btn fvgo" data-fv-planner="mine">.*Give my hours/);
  v.mine = { answered: true, tz: 'America/Chicago', blocks: [{ dayOfWeek: 6, startMinute: 1080, endMinute: 1320 }] };
  html = v._fvPlayerHTML(2026, 10);
  assert.match(html, /Sat<\/span><span class="ln"><b style="left:75\.00%;width:16\.67%">/, 'Saturday 6pm to 10pm');
  assert.match(html, /Change my hours/);
  assert.match(html, /in America\/Chicago time/);
  assert.doesNotMatch(html, /Jack|Julie/, 'no names for players');
});

test('a player sees how many are free at each game night this month', () => {
  const { def } = load();
  const v = freeView(def, overlay({ includeDetail: false, members: [] }));
  v.freeDirector = false; v.role = 1; v.mine = { answered: true, blocks: [] };
  v.nightsByMonth = { '2026_10': [night({ date: '2026-10-08', time: '19:00' })] };
  const html = v._fvPlayerHTML(2026, 10);
  assert.match(html, /data-best-open="2026_10_8"/);
  assert.match(html, /3 of 4 free at 7pm/);
});

test('the month reads the weeks its days fall in, in the calendar zone', async () => {
  const { def, calls } = load();
  const v = freeView(def);
  v.calZone = 'America/Chicago';
  await v.fetchFreeMonth(2026, 10);
  const weeks = calls.map((c) => c.url.match(/week=([\d-]+)/)[1]);
  assert.deepEqual(weeks, ['2026-09-28', '2026-10-05', '2026-10-12', '2026-10-19', '2026-10-26']);
  assert.match(calls[0].url, /tz=America%2FChicago/);
  await v.fetchFreeMonth(2026, 10);
  assert.equal(calls.length, 5, 'a week is read once');
});

test('a week read that lands while the day card unfolds redraws it once open', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  let redraws = 0;
  v.refreshWing = () => { redraws++; };
  v.wingFor = '2026_10_8';
  v._pw = { state: 'opening' };
  v._refreshWingOnceOpen();
  assert.equal(redraws, 0, 'not while unfolding');
  assert.equal(v._wingStale, '2026_10_8', 'remembered for when it opens');
  v._pw.state = 'open';
  v._refreshWingOnceOpen();
  assert.equal(redraws, 1);
});

test('the full planner shows the week day by day, the month’s nights with who is coming, and Best times', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  v._plWeek = '2026-10-05';
  v.nightsByMonth = { '2026_10': [night({ date: '2026-10-08' })] };
  const html = v._plTeamHTML();
  assert.match(html, /Week of Oct 5 – Oct 11/);
  assert.match(html, /data-pl-day="2026_10_8"><b>Thu Oct 8<\/b>/);
  assert.match(html, /Julie<\/span><span class="ln"><b style="left:75\.00%;width:16\.67%">/);
  assert.match(html, /Game nights in M10/);
  assert.match(html, /1 going · 0 maybe · 0 can’t · 2 no answer/);
  assert.match(html, /<span class="plw y" title="Kael: going">Kael<\/span>/);
  assert.match(html, /Best times in M10/);
  assert.doesNotMatch(html, /Open the full planner/, 'the drawer does not link to itself');
});

test('painting your hours saves the every-week pattern', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  v.mine = { answered: true, tz: 'America/Chicago', blocks: [
    { dayOfWeek: 6, startMinute: 1080, endMinute: 1320, state: 'available', weekCadence: 0 },
  ] };
  v._plLoadGrid();
  assert.equal(v._plAlt, false);
  assert.equal(v._plGrids[0][5][18], 'available', 'Saturday 6pm, Monday first');
  v._plGrids[0][0][19] = 'preferred'; v._plGrids[0][0][20] = 'preferred';
  assert.deepEqual(JSON.parse(JSON.stringify(v._plBlocks())), [
    { dayOfWeek: 1, startMinute: 1140, endMinute: 1260, state: 'preferred', weekCadence: 0 },
    { dayOfWeek: 6, startMinute: 1080, endMinute: 1320, state: 'available', weekCadence: 0 },
  ]);
  v._plPaint = true; v._plTool = 'available';
  const html = v._plMineHTML();
  assert.match(html, /data-g="0" data-c="5" data-h="18" data-st="available"/);
  assert.match(html, /data-pl-alt="0" aria-pressed="true">Same every week/);
  assert.doesNotMatch(html, /href="[^"]*\/availability/, 'alternating weeks are painted here, not on another page');
});

test('alternating weeks paint two weeks, named by date, and save each as its own week', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  v.mine = { answered: true, tz: 'America/Chicago', blocks: [
    { dayOfWeek: 6, startMinute: 1080, endMinute: 1320, state: 'available', weekCadence: 0 },
    { dayOfWeek: 2, startMinute: 600, endMinute: 720, state: 'preferred', weekCadence: 1 },
  ] };
  v._plLoadGrid();
  assert.equal(v._plAlt, true, 'a member with alternating hours opens on them');
  assert.equal(v._plGrids[1][5][18], 'available', 'every-week Saturday holds on week one');
  assert.equal(v._plGrids[2][5][18], 'available', 'and on week two');
  assert.equal(v._plGrids[1][1][10], 'preferred', 'the alternating Tuesday is on week one only');
  assert.equal(v._plGrids[2][1][10], '');
  const blocks = JSON.parse(JSON.stringify(v._plBlocks()));
  assert.ok(blocks.every((b) => b.weekCadence === 1 || b.weekCadence === 2), 'only the two weeks are saved');
  assert.equal(blocks.filter((b) => b.dayOfWeek === 6).length, 2);
  // 2026-10-04 is a Sunday 2961 weeks after 1970-01-04, an odd week: the
  // server's WeekCadenceFor calls it week two (2), so Oct 11 is week one.
  v.cal.current_year = 2026; v.cal.current_month = 10; v.cal.current_day = 7;
  v._plPaint = true;
  const html = v._plMineHTML();
  assert.match(html, /Week of Sun Oct 4 and every other week after<\/h5><div class="plgrid" role="grid" aria-label="[^"]*"><div class="plgh" aria-hidden="true">(?:<span>[^<]*<\/span>)*<\/div><div class="plrow" role="row"><span class="nm">Sun<\/span><span class="plcells"><button type="button" class="plc" role="gridcell" data-g="2"/, 'the sooner week first, holding week two');
  assert.match(html, /Week of Sun Oct 11 and every other week after[\s\S]*data-g="1"/);
  assert.match(html, /<div class="plrow" role="row"><span class="nm">Sun<\/span>/, 'an alternating week starts on Sunday');
  assert.match(html, /data-g="2"/);
});

test('switching to alternating weeks starts both weeks from the usual hours', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  v.mine = { answered: true, tz: 'UTC', blocks: [{ dayOfWeek: 1, startMinute: 1140, endMinute: 1200, state: 'available', weekCadence: 0 }] };
  v._plLoadGrid();
  v.refreshPlanner = () => {};
  v._plSetAlt(true);
  assert.equal(v._plGrids[1][0][19], 'available');
  assert.equal(v._plGrids[2][0][19], 'available');
  assert.equal(v._plDirty, true);
  v._plGrids[2][0][19] = '';
  assert.deepEqual(JSON.parse(JSON.stringify(v._plBlocks())), [{ dayOfWeek: 1, startMinute: 1140, endMinute: 1200, state: 'available', weekCadence: 1 }]);
  v._plSetAlt(false);
  assert.deepEqual(JSON.parse(JSON.stringify(v._plBlocks())), [{ dayOfWeek: 1, startMinute: 1140, endMinute: 1200, state: 'available', weekCadence: 0 }]);
});

test('a day’s card shows the viewer’s own hours that day and changes just that day', async () => {
  const { def, calls } = load();
  const v = freeView(def, overlay({ includeDetail: false, members: [] }));
  v.freeDirector = false; v.role = 1;
  v.mine = { answered: true, tz: 'America/Chicago', blocks: [] };
  v.myDays = [{ id: 'x', onDate: '2026-10-09', startMinute: 0, endMinute: 1440, state: 'unavailable' }, { id: 'y', onDate: '2026-10-10', startMinute: 1200, endMinute: 1380, state: 'available' }];
  assert.match(v._myDayHTML('2026-10-08'), /This day: <b>Your usual hours/);
  assert.match(v._myDayHTML('2026-10-09'), /You can’t play/);
  assert.match(v._myDayHTML('2026-10-10'), /8pm to 11pm/);
  v._myDayFor = '2026-10-10';
  v._myDayForm = { mode: 'diff', start: 1200, end: 1380 };
  const form = v._myDayHTML('2026-10-10');
  assert.match(form, /<form class="myday edit" data-myday="2026-10-10">/, 'a form, so the card holds open');
  assert.match(form, /data-myday-from value="20:00"[\s\S]*data-myday-to value="23:00"/);
  assert.match(form, /In America\/Chicago time/);
});


test('a recap with formatting is never edited as plain text in the calendar', () => {
  const { def } = load();
  const v = view(def, [night()]);
  v.role = 2;
  v._gnPages = {
    plain: { recap: 'The vault opened.', recapHtml: '<p>The vault opened.</p>', links: [] },
    rich: { recap: 'The vault opened.', recapHtml: '<p>The <strong>vault</strong> opened.</p><ul><li>Loot</li></ul>', links: [] },
  };
  assert.match(v._gnExtraHTML('plain'), /data-gn-recap="edit"><i class="fa-solid fa-pen"><\/i> Edit the recap/);
  const rich = v._gnExtraHTML('rich');
  assert.doesNotMatch(rich, /data-gn-recap="edit"/, 'no plain-text editor for a formatted recap');
  assert.match(rich, /<a class="lnk" href="\/campaigns\/c1\/sessions\/rich">.*Edit on the page/);
  v._gnRecapFor = 'rich';
  assert.doesNotMatch(v._gnExtraHTML('rich'), /data-gn-recap-input/, 'even when asked, the textarea never opens on it');
  v.role = 1;
  assert.doesNotMatch(v._gnExtraHTML('plain'), /data-gn-recap="edit"/, 'players read the recap only');
});

test('the Director can remind any one player, never themselves', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  v.userId = 'jack';
  const html = v._fvDirectorHTML(2026, 10, 'October');
  assert.doesNotMatch(html, /data-remind="jack"/, 'no bell for yourself');
  assert.match(html, /Jack<\/span><span class="ok">Hours given<\/span><span class="fvme">You/);
  assert.match(html, /data-remind="julie"/);
  assert.match(html, /data-remind="dee"/, 'a player with no hours can be reminded too');
  v._reminded = { julie: true };
  assert.match(v._fvDirectorHTML(2026, 10, 'October'), /data-remind="julie" disabled aria-label="Reminded Julie"/);
});

test('after an ask, the roster shows who confirmed since', () => {
  const { def } = load();
  const v = freeView(def, overlay());
  v.userId = 'jack';
  v.answers = { askedAt: '2026-10-01T12:00:00Z', members: [
    { userId: 'julie', answeredAt: '2026-10-02T09:00:00Z' },
    { userId: 'bryn', answeredAt: '2026-09-20T09:00:00Z' },
  ] };
  const html = v._fvDirectorHTML(2026, 10, 'October');
  assert.match(html, /Julie<\/span><span class="ok">Confirmed/);
  assert.match(html, /Bryn<\/span><span class="wait">Not confirmed yet/);
  assert.match(html, /Dee<\/span><span class="wait">No hours yet/);
  assert.match(html, /<b>1 of 3<\/b> confirmed since you asked/, 'the asker is not counted');
  assert.match(html, /data-ask-confirm>.*Ask again/);
});

test('a player is asked to confirm only while an ask is newer than their answer', () => {
  const { def } = load();
  const v = freeView(def, overlay({ includeDetail: false, members: [] }));
  v.freeDirector = false; v.role = 1;
  v.answers = { askedAt: '2026-10-01T12:00:00Z' };
  v.mine = { answered: true, answeredAt: '2026-09-01T00:00:00Z', blocks: [] };
  assert.equal(v._confirmDue(), true);
  assert.match(v._fvPlayerHTML(2026, 10), /data-confirm-mine>.*My times are still right/);
  v.mine.answeredAt = '2026-10-02T00:00:00Z';
  assert.equal(v._confirmDue(), false);
  v.mine = { answered: false, blocks: [] };
  assert.equal(v._confirmDue(), false, 'no hours yet: asked to give them, not confirm');
});

test('whole days off read back as away stretches; shaped days do not', () => {
  const { exp } = load();
  const off = (d) => ({ onDate: d, startMinute: 0, endMinute: 1440, state: 'unavailable' });
  const got = exp.awayStretches([
    off('2026-10-03'), off('2026-10-04'), off('2026-10-05'),
    off('2026-10-09'),
    { onDate: '2026-10-12', startMinute: 1080, endMinute: 1320, state: 'available' },
    off('2026-09-20'),
  ], '2026-10-01');
  assert.deepEqual(JSON.parse(JSON.stringify(got)), [
    { from: '2026-10-03', to: '2026-10-05' },
    { from: '2026-10-09', to: '2026-10-09' },
  ]);
});

test('only the owner of a world calendar gets Set today, and it checks the form', () => {
  const { def } = load();
  const v = freeView(def);
  v.apiBase = '/campaigns/c1/calendar';
  assert.equal(v._canSetToday(), false, 'a real-world calendar follows the clock');
  v.cal = { mode: 'fantasy', current_year: 1200, current_month: 2, current_day: 5, hours_per_day: 24, minutes_per_hour: 60,
    months: [{ name: 'Frost', days: 30 }, { name: 'Thaw', days: 28 }] };
  assert.equal(v._canSetToday(), true);
  v.role = 2;
  assert.equal(v._canSetToday(), false, 'below owner: read only');
  const form = (o) => ({ elements: Object.fromEntries(Object.entries(Object.assign({ year: '1200', month: '2', day: '5', hour: '20', minute: '0' }, o)).map(([k, x]) => [k, { value: x }])) });
  assert.deepEqual(JSON.parse(JSON.stringify(v._readTodayForm(form()))), { y: 1200, m: 2, d: 5, h: 20, mi: 0 });
  assert.match(v._readTodayForm(form({ day: '29' })).error, /28 days/);
  assert.match(v._readTodayForm(form({ hour: '24' })).error, /0 to 23/);
  assert.match(v._readTodayForm(form({ year: 'soon' })).error, /every box/);
});

test('the DM sees a day a player marked off although usually free', () => {
  const { def } = load();
  const base = overlay();
  // Dee marked Thursday (column 3) off; their usual hours would cover it.
  const ov = overlay({ members: base.members.map((m) => (m.userId === 'dee' ? { ...m, hasAnswered: true, offDays: [3] } : m)) });
  const v = freeView(def, ov);
  v._todayIso = () => '2026-10-01';
  const wing = v._freeWingHTML({ y: 2026, m: 10, d: 8 });
  assert.match(wing, /Dee<\/span><span class="ln off"><\/span><span class="sr">Dee: off this day, usually free/);
  assert.match(wing, /<i class="o"><\/i>Off, usually free/, 'the key explains the broken line');
  assert.match(v._freeCellHTML(2026, 10, 8), /<span class="ln off"><\/span>/, 'on the month too');
  assert.doesNotMatch(v._freeWingHTML({ y: 2026, m: 10, d: 9 }), /ln off|Off, usually free/, 'only on that day');
});
