// calendar_waiting.test.mjs — pins the Game nights calendar's "waiting on
// you" button: it shows only while something waits, counts it, lists a
// night as a button that opens it here and anything else as a link, and
// escapes what it shows.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');

function load(body) {
  const defs = {};
  const calls = [];
  const sandbox = {
    module: { exports: {} }, console, Promise, URLSearchParams, Intl,
    window: { location: { search: '' } },
    Chronicle: {
      register(name, def) { defs[name] = def; },
      apiFetch(url) {
        calls.push(url);
        return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
      },
    },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { def: defs.calendar_view, calls };
}

// The button, its count and the list, as plain objects.
function view(def) {
  const attrs = {};
  const count = { textContent: '' };
  const btn = {
    hidden: true, focused: false,
    setAttribute(k, v) { attrs[k] = v; }, focus() { this.focused = true; },
    querySelector: (s) => (s === '.wcount' ? count : null),
  };
  const pop = { hidden: true, innerHTML: '', querySelector: () => null };
  const el = { querySelector: (s) => (s === '#cal5-waitbtn' ? btn : s === '#cal5-waitpop' ? pop : null) };
  const v = Object.create(def);
  Object.assign(v, { el, campaignId: 'c 1' });
  return { v, btn, pop, count, attrs };
}

const rows = [
  { title: 'Game night · 8 pm', sub: '0 of 4 coming · you haven\'t answered', action: 'Answer', href: '/campaigns/c1/game-nights?session=s1&date=2026-10-14', tileTop: 'Wed', tileBig: '14', sessionId: 's1', date: '2026-10-14' },
  { title: 'Pick <b>a night</b>', sub: 'Date poll · 3 answers so far', action: 'Vote', href: '/campaigns/c1/proposals/p1', tileTop: 'Poll', tileIcon: 'fa-solid fa-chart-simple' },
];

test('reads the list for this campaign and shows the count', async () => {
  const { def, calls } = load(rows);
  const { v, btn, count, attrs } = view(def);
  v.loadWaiting();
  await new Promise((r) => setTimeout(r, 0));
  assert.deepEqual(calls, ['/campaigns/c%201/coming-up/waiting']);
  assert.equal(btn.hidden, false);
  assert.equal(count.textContent, '2');
  assert.equal(attrs['aria-label'], '2 waiting on you');
});

test('a night opens on the calendar; a poll is a link; text is escaped', () => {
  const { def } = load([]);
  const { v, pop } = view(def);
  v.waiting = rows;
  v.renderWaiting();
  assert.match(pop.innerHTML, /<button type="button" class="wrow" data-wait-night="s1" data-wait-date="2026-10-14">/);
  assert.match(pop.innerHTML, /<a class="wrow" href="\/campaigns\/c1\/proposals\/p1">/);
  assert.match(pop.innerHTML, />Vote</);
  assert.doesNotMatch(pop.innerHTML, /<b>a night<\/b>/);
});

test('nothing waiting hides the button and closes the list', () => {
  const { def } = load([]);
  const { v, btn, pop } = view(def);
  v.waiting = rows;
  v.renderWaiting();
  v.openWaiting();
  assert.equal(pop.hidden, false);
  v.waiting = [];
  v.renderWaiting();
  assert.equal(btn.hidden, true);
  assert.equal(pop.hidden, true);
});

test('a night with no usable date falls back to its link', () => {
  const { def } = load([]);
  const { v, pop } = view(def);
  v.waiting = [{ ...rows[0], date: 'soon' }];
  v.renderWaiting();
  assert.doesNotMatch(pop.innerHTML, /data-wait-night/);
  assert.match(pop.innerHTML, /<a class="wrow" href="\/campaigns\/c1\/game-nights\?session=s1&amp;date=2026-10-14">/);
});
