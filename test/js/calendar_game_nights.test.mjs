// calendar_game_nights.test.mjs — pins how calendar_view.js shows game
// nights and takes answers: silence reads as "no answer yet" (never a no),
// each night of a series answers for its own date, stored text is escaped,
// and only the organiser or owner gets the "count me" switch.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');

function load() {
  const defs = {};
  const calls = [];
  const sandbox = {
    module: { exports: {} },
    console,
    Promise,
    Intl,
    Chronicle: {
      register(name, def) { defs[name] = def; },
      apiFetch(url, opts) { calls.push({ url, opts }); return Promise.resolve({ ok: true, json: () => Promise.resolve([]) }); },
    },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { def: defs.calendar_view, calls };
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
