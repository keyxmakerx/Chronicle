// calendar_mobile_agenda_geometry.test.mjs — C-CAL-MOBILE-AGENDA geometry +
// wiring contract pin. The mobile mini-month + agenda are server-rendered
// templ (like the desktop grid), so — mirroring calendar_month_geometry
// .test.mjs's approach — this harness pins the rule by asserting the SOURCE
// carries the load-bearing geometry/reuse markers rather than by measuring
// live layout (no screenshot/computed-layout harness is available here).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (rel) => readFileSync(join(root, rel), 'utf8');
const templ = read('internal/plugins/calendar/calendar_v2.templ');
const helpers = read('internal/plugins/calendar/calendar_v2_mobile_agenda.go');
const eventGrid = read('internal/plugins/calendar/static/js/event_grid.js');

function block(src, startMarker, endMarker) {
  const start = src.indexOf(startMarker);
  assert.ok(start >= 0, `expected to find ${startMarker}`);
  const rest = src.slice(start);
  const end = endMarker ? rest.indexOf(endMarker) : -1;
  return end > 0 ? rest.slice(0, end) : rest;
}

test('breakpoint swap: the Month view renders BOTH branches present-but-hidden (no fragment swap)', () => {
  const fn = block(templ, 'templ monthViewPlaceholder', '\ntempl ');
  assert.ok(fn.includes('hidden md:block'), 'the desktop grid wrapper must hide via CSS at <768px, not be omitted server-side');
  assert.ok(fn.includes('data-month-desktop-grid="true"'), 'the desktop grid wrapper needs a stable hook');
  assert.ok(fn.includes('@mobileMonthAssembly(data)'), 'the mobile assembly must render in the SAME pass as the desktop grid');
});

test('mobileMonthAssembly is md:hidden — the other half of the breakpoint swap', () => {
  const fn = block(templ, 'templ mobileMonthAssembly', '\ntempl ');
  assert.ok(fn.includes('md:hidden'), 'mobileMonthAssembly must hide at >=768px');
});

test('mini-month: every day number sits in a fixed h-10 cell; dots render in a reserved absolute layer', () => {
  const fn = block(templ, 'templ mobileMiniMonth', '\ntempl ');
  assert.ok(fn.includes('h-10'), 'the day-number cell must be a fixed h-10 (40px) box');
  assert.ok(
    fn.includes('absolute bottom-1 left-1/2 -translate-x-1/2'),
    'the operator\'s alignment rule applied verbatim: dots live in a reserved, absolutely-positioned layer so they can never move the number',
  );
  // The number and the dots must be SIBLINGS inside the same fixed cell (not
  // one nested inside logic that could resize the other).
  assert.ok(/<span>\{ fmt\.Sprint\(d\.Day\) \}<\/span>/.test(fn), 'the day number renders as its own element inside the fixed cell');
});

test('mini-month dots cap mirrors the Month grid\'s own visible cap (mobileMiniMonthDotsCap → monthCellVisibleCap)', () => {
  const fn = block(helpers, 'func mobileMiniMonthDotsCap()', '\n}');
  assert.ok(
    fn.includes('monthCellVisibleCap()'),
    'the mini-month dot cap must derive from the SAME cap the Month grid pips use, not a second hardcoded number that could drift',
  );
});

test('agenda grouping: reuses monthCellEvents (the Month grid\'s own recurrence-aware per-day projection)', () => {
  const fn = block(helpers, 'func mobileAgendaGroups', '\n}\n\n// mobileAgendaDayHeader');
  assert.ok(
    fn.includes('monthCellEvents(data, day)'),
    'the agenda must group the SAME per-day event set the Month grid cells show, so the two breakpoints never disagree on a day\'s events',
  );
  assert.ok(fn.includes('data.Day'), 'grouping must start from the selected/current day cursor');
});

test('agenda card: carries the [data-event-card] hook event_grid.js already wires document-wide (no new JS needed to open it)', () => {
  const fn = block(templ, 'templ mobileAgendaCard', '\ntempl ');
  for (const hook of ['data-event-card="agenda"', 'data-event-id=', '<button']) {
    assert.ok(fn.includes(hook), `agenda card missing ${hook}`);
  }
  // event_grid.js's click wiring selects ALL [data-event-card] elements
  // (not scoped to a specific value), so the "agenda" value rides the
  // existing wiring for free — pin that the selector really is unscoped.
  assert.ok(
    eventGrid.includes("document.querySelectorAll('[data-event-card]')"),
    'event_grid.js must keep an unscoped [data-event-card] selector for the new "agenda" card value to be picked up without any JS change',
  );
});

test('agenda card actions are visible at rest — no hover-only/opacity-gated affordance (pointer:coarse)', () => {
  const fn = block(templ, 'templ mobileAgendaCard', '\ntempl ');
  assert.ok(!fn.includes('opacity-0'), 'agenda card must not hide its affordance behind opacity-0');
  assert.ok(!fn.includes('group-hover'), 'agenda card must not gate its affordance behind hover (touch has no hover)');
  assert.ok(fn.includes('fa-chevron-right'), 'agenda card needs a visible-at-rest tap affordance');
});

test('command bar: phone reduction hides Week/Day/Timeline via one wrapper, keeps Month + adds Agenda', () => {
  const fn = block(templ, 'templ calendarV2ViewSwitcher', '\ntempl mobileAgendaPill');
  assert.ok(fn.includes('@mobileAgendaPill(data)'), 'view switcher must render the mobile Agenda pill');
  assert.ok(
    fn.includes('<span class="hidden md:contents">'),
    'Week/Day/Timeline must be wrapped in one hidden-md:contents group (display:contents at md+ keeps desktop\'s pill row unchanged)',
  );
});

test('Agenda pill links to the SAME month-view route the Month pill uses — no new endpoint', () => {
  const fn = block(templ, 'templ mobileAgendaPill', '\ntempl ');
  assert.ok(fn.includes('v2ViewHref(data, "month")'), 'Agenda pill must route to view=month, not a new server view');
  assert.ok(fn.includes('md:hidden'), 'Agenda pill must be phone-only');
});

test('day-select is a plain link (miniMonthDayHref) — no client JS required for the tap-to-select interaction', () => {
  const fn = block(templ, 'templ mobileMiniMonth', '\ntempl ');
  assert.ok(
    fn.includes('miniMonthDayHref(data, miniMonthDay{Day: d.Day})'),
    'mini-month day cells must reuse the desktop sidebar\'s existing jump-link builder, not a new JS click handler',
  );
});

test('sidebar dedup: the persistent desktop mini-month sidebar hides ONLY for the Month view at <768px', () => {
  const fn = block(helpers, 'func mobileSidebarClasses', '\n}');
  assert.ok(fn.includes('data.View == "month"'), 'the sidebar hide must be scoped to the Month view only — Week/Day/Timeline keep their current mobile presentation');
  assert.ok(fn.includes('hidden md:block'), 'the hidden branch must use the same CSS-swap technique as the grid/assembly pair');
});
