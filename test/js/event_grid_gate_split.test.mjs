// event_grid_gate_split.test.mjs — C-CAL-UX-PAIR §Fix 1. event_grid.js's
// init() used to early-return for every non-Scribe viewer
// (`if (!isScribe || !calendarID) return;`), which killed the read-only
// quick-edit card the markup already server-gates to read-only for players
// (calendar_v2_quickedit.templ) — including the mobile agenda cards (#544),
// which ride the SAME [data-event-card] click wiring.
//
// The fix splits the gate: read-only interactions (tap → quick-edit card)
// wire for EVERY calendar member; write interactions (drawer open/save,
// cell-add, drag-create/move) stay behind the drawer's existence check
// (`document.getElementById('event-v2-drawer')` — the drawer itself is
// server-gated to Scribes, eventV2Drawer in calendar_v2.templ, so for every
// other role that lookup naturally returns null and the pre-existing
// `if (!drawer) return;` IS the gate).
//
// This harness runs event_grid.js in a node vm with a stub DOM, once per
// role, and asserts which listeners actually got attached — the dynamic
// counterpart to the source-level regex tests in event_quickedit.test.mjs /
// event_drag.test.mjs.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'event_grid.js');
const src = readFileSync(jsPath, 'utf8');

// stubEl is a minimal DOM-element double: records every addEventListener
// call by type (so tests can assert wiring happened, or didn't) and no-ops
// everything else event_grid.js might touch during setup.
function stubEl(extra) {
  const listeners = {};
  return Object.assign({
    dataset: {},
    style: {},
    classList: { add() {}, remove() {}, contains: () => false, toggle() {} },
    addEventListener(type, fn) { (listeners[type] = listeners[type] || []).push(fn); },
    removeEventListener() {},
    querySelector: () => null,
    querySelectorAll: () => [],
    setAttribute() {},
    getAttribute: () => null,
    closest: () => null,
    getBoundingClientRect: () => ({ left: 0, right: 0, top: 0, bottom: 0, width: 0, height: 0 }),
    __listeners: listeners,
  }, extra);
}

// boot runs event_grid.js's real init()/boot() against a fake page for the
// given role. isScribe=false means the drawer + qe-save/qe-expand buttons
// are ABSENT from the DOM (document.getElementById returns null for the
// drawer; the qe scaffold's querySelector stub returns null for its
// Scribe-only inner buttons too) — the same server-gated-markup shape a
// real player's page renders.
function boot(isScribe) {
  const card = stubEl({ dataset: { eventId: 'ev-1' } });
  const agendaCard = stubEl({ dataset: { eventId: 'ev-2' } }); // #544 mobile agenda card
  const cellAddBtn = stubEl();
  const qe = stubEl(); // scaffold always renders (read-only for players)
  const drawer = isScribe ? stubEl() : null;

  const root = stubEl({
    dataset: {
      calV2CalendarId: 'cal-1',
      calV2CampaignId: 'camp-1',
      calV2CsrfToken: 'tok',
      calV2IsScribe: isScribe ? 'true' : 'false',
      calV2IsOwner: 'false',
      calV2Events: '[]',
    },
  });

  const docListeners = {};
  const sandbox = {
    console,
    Chronicle: {
      apiFetch: () => Promise.resolve({ ok: true, json: () => Promise.resolve({}) }),
      notify() {},
    },
    setTimeout, clearTimeout,
    document: {
      readyState: 'complete',
      querySelector: (sel) => (sel === '[data-cal-v2-root]' ? root : null),
      querySelectorAll: (sel) => {
        if (sel === '[data-event-card]') return [card, agendaCard];
        if (sel === '[data-cell-add-event]') return [cellAddBtn];
        return [];
      },
      getElementById: (id) => {
        if (id === 'event-v2-drawer') return drawer;
        if (id === 'cal-v2-event-quickedit') return qe;
        return null;
      },
      addEventListener: (t, fn) => { (docListeners[t] = docListeners[t] || []).push(fn); },
      removeEventListener() {},
      activeElement: null,
    },
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'event_grid.js' });
  return { card, agendaCard, cellAddBtn, qe, drawer, docListeners };
}

function wired(el, type) { return !!(el.__listeners[type] && el.__listeners[type].length); }

test('player (non-Scribe): the event chip AND the #544 mobile agenda card both wire click → quick-edit', () => {
  const { card, agendaCard } = boot(false);
  assert.ok(wired(card, 'click'), 'month/week/day chip must wire click for players');
  assert.ok(wired(agendaCard, 'click'), 'mobile agenda card (data-event-card="agenda") must inherit the same wiring for players');
});

test('player (non-Scribe): write affordances do NOT wire — no drawer, no cell-add', () => {
  const { cellAddBtn } = boot(false);
  assert.ok(!wired(cellAddBtn, 'click'), 'cell-add-event must NOT wire for players (write, Scribe+)');
});

test('player (non-Scribe): boot does not throw (no crash reaching for the absent drawer)', () => {
  assert.doesNotThrow(() => boot(false));
});

test('Scribe: both the read-only chip wiring AND the write (cell-add) wiring attach', () => {
  const { card, agendaCard, cellAddBtn } = boot(true);
  assert.ok(wired(card, 'click'), 'chip click must still wire for Scribes');
  assert.ok(wired(agendaCard, 'click'), 'agenda card click must still wire for Scribes');
  assert.ok(wired(cellAddBtn, 'click'), 'cell-add-event must wire for Scribes (write, Scribe+)');
});

test('the drawer-existence check is the actual gate (source-level pin)', () => {
  // Structural pin so the dynamic behavior above can't silently regress if
  // someone reintroduces an isScribe check before the quick-edit block: the
  // read-only wiring must appear BEFORE the drawer lookup, and the ONLY
  // early-return between init()'s start and the drawer lookup must be the
  // role-blind calendarID check.
  const initStart = src.indexOf('function init() {');
  const drawerGate = src.indexOf("document.getElementById('event-v2-drawer')");
  const cardWiring = src.indexOf("document.querySelectorAll('[data-event-card]').forEach(function (card) {");
  assert.ok(initStart >= 0 && drawerGate > initStart, 'drawer lookup present inside init()');
  assert.ok(cardWiring > initStart && cardWiring < drawerGate,
    'the [data-event-card] click wiring must run BEFORE the drawer lookup (read-only, every role)');
  const preamble = src.slice(initStart, drawerGate);
  assert.doesNotMatch(preamble, /if \(!isScribe/, 'no isScribe gate may reappear before the drawer lookup');
});

test('initRibbonResize (drag-to-resize, a write action) is Scribe+ gated', () => {
  const start = src.indexOf('function initRibbonResize()');
  assert.ok(start >= 0, 'initRibbonResize present');
  const header = src.slice(start, start + 900); // the setup preamble, well before onPointerDown
  assert.match(header, /isScribe = root\.dataset\.calV2IsScribe === 'true'/, 'must read the scribe flag');
  assert.match(header, /if \(!isScribe \|\| !calendarID\) return;/, 'must gate on isScribe (write affordance)');
});
