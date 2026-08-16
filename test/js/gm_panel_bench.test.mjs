// gm_panel_bench.test.mjs — C-CALV4-GM-CONSOLE: the console's WRITE PATHS,
// exercised on a BENCH-SHAPED mount.
//
// WHY THIS FILE EXISTS. The three sibling gm_panel_*.test.mjs files cover the
// console's chrome — the state machine, the pull-out contract, re-init on
// boosted nav — and exactly ONE payload (clearEvents). Everything else was
// asserted only in Go, i.e. only that the BUTTON RENDERS. A rehousing whose
// risk is precisely "a control that renders but does not write" cannot be
// signed off on markup assertions, so every family's body is measured here,
// against the shipped driver, in a DOM that has NO `[data-cal-v2-root]` and NO
// sky engine — which is the Bench.
//
// THE MOUNT IS THE POINT OF THE FILE. gm_panel.js used to read its endpoint,
// calendar and CSRF token off `[data-cal-v2-root]` and RETURN when it was
// absent. That attribute exists on calendar_v2.templ and nowhere else, so the
// console rehoused unchanged would have rendered, looked perfect, and done
// nothing at all — in production only. The first test below is that trap.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(
  join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'gm_panel.js'),
  'utf8');

// Let the sandbox's promise chains drain. The vm context shares this process's
// event loop, so one real macrotask is enough for a .then().then() chain.
const tick = () => new Promise((r) => setTimeout(r, 0));

// The driver runs in a vm REALM of its own, so the objects it builds have that
// realm's Object.prototype and deepStrictEqual refuses them ("same structure
// but not reference-equal"). Round-tripping through JSON rebuilds the payload
// in this realm — and it is also exactly what apiFetch does to it on the way to
// the wire, so the comparison is against what the server would actually receive.
const wire = (call) => JSON.parse(JSON.stringify(call.opts.body));

function el(attrs, extra) {
  const node = Object.assign({
    _attrs: Object.assign({}, attrs || {}),
    _click: null,
    _change: null,
    _input: null,
    dataset: {},
    style: {},
    disabled: false,
    value: '',
    checked: false,
    children: [],
    textContent: '',
    setAttribute(k, v) { this._attrs[k] = String(v); },
    getAttribute(k) {
      return Object.prototype.hasOwnProperty.call(this._attrs, k) ? this._attrs[k] : null;
    },
    removeAttribute(k) { delete this._attrs[k]; },
    hasAttribute(k) { return Object.prototype.hasOwnProperty.call(this._attrs, k); },
    addEventListener(type, fn) {
      if (type === 'click') this._click = fn;
      if (type === 'change') this._change = fn;
      if (type === 'input') this._input = fn;
    },
    removeEventListener() {},
    appendChild(c) { this.children.push(c); return c; },
    querySelector() { return null; },
    querySelectorAll() { return []; },
    closest(sel) {
      // The delegated per-event-clear handler asks the CHIP whether it is one.
      const name = sel.replace(/^\[|\]$/g, '');
      return this.hasAttribute(name) ? this : null;
    },
    classList: { toggle() {}, add() {}, remove() {} },
  }, extra || {});
  return node;
}

// A Bench-shaped console: the full control set MINUS Pause (which the Bench
// producer does not render, because the sky engine it drives is not on this
// page), mounted with its own url/csrf/standalone attributes and no page root.
function boot(opts) {
  opts = opts || {};
  const nodes = {
    clock: el(),
    activeEvents: el(),
    advance10m: el({}, { dataset: { gmMinutes: '10' } }),
    advance1d: el({}, { dataset: { gmDays: '1' } }),
    setTime: el(),
    hour: el(), minute: el(),
    setDate: el(),
    year: el(), month: el(), day: el(),
    weatherRain: el({ 'data-gm-id': 'rain', 'data-gm-label': 'Rain' }),
    weatherClear: el({ 'data-gm-id': 'clear', 'data-gm-label': 'Clear' }),
    eventEclipse: el({ 'data-gm-id': 'eclipse-solar', 'data-gm-label': 'Solar Eclipse' }),
    dmOnly: el(),
    clearAll: el(),
    moodPreset: el({}, { dataset: { gmMoodColor: 'oklch(0.55 0.22 25)', gmMoodIntensity: '0.45' } }),
    moodCustom: el(),
    moodSlider: el(),
    moodClear: el(),
    reset: el(),
    pill: el(),
  };
  nodes.hour.value = '21';
  nodes.minute.value = '5';
  nodes.year.value = '1523';
  nodes.month.value = '2';
  nodes.day.value = '14';
  nodes.moodSlider.value = '60';
  nodes.moodCustom.value = '#112233';

  const single = {
    '[data-gm-active-events]': nodes.activeEvents,
    '[data-gm-clock]': nodes.clock,
    '[data-gm-panel-open]': nodes.pill,
    '[data-gm-set-time]': nodes.setTime,
    '[data-gm-time-hour]': nodes.hour,
    '[data-gm-time-minute]': nodes.minute,
    '[data-gm-set-date]': nodes.setDate,
    '[data-gm-date-year]': nodes.year,
    '[data-gm-date-month]': nodes.month,
    '[data-gm-date-day]': nodes.day,
    '[data-gm-event-dmonly]': nodes.dmOnly,
    '[data-gm-events-clear]': nodes.clearAll,
    '[data-gm-mood-custom]': nodes.moodCustom,
    '[data-gm-mood-intensity-slider]': nodes.moodSlider,
    '[data-gm-mood-clear]': nodes.moodClear,
    '[data-gm-reset]': nodes.reset,
    // PAUSE IS DELIBERATELY ABSENT — see the Go test of the same name. The
    // driver must wire cleanly without it.
    '[data-gm-pause]': null,
  };
  const many = {
    '[data-gm-advance]': [nodes.advance10m, nodes.advance1d],
    '[data-gm-weather-tile]': [nodes.weatherRain, nodes.weatherClear],
    '[data-gm-event-tile]': [nodes.eventEclipse],
    '[data-gm-mood]': [nodes.moodPreset],
  };

  let panelClick = null;
  const panel = el({
    'data-gm-collapsed': 'true',
    'data-gm-sheet': '',
    'data-gm-url': '/campaigns/camp-1/calendar/world-state?calendarId=cal-harptos',
    'data-gm-csrf': 'tok-9',
    // The Bench declares it: no sky engine on this page.
    ...(opts.standalone === false ? {} : { 'data-gm-standalone': '' }),
  }, {
    querySelector: (sel) => (Object.prototype.hasOwnProperty.call(single, sel) ? single[sel] : null),
    querySelectorAll: (sel) => (many[sel] || []),
    addEventListener(type, fn) { if (type === 'click') panelClick = fn; },
    contains: () => true,
  });

  const calls = [];
  let reloads = 0;
  const sandbox = {
    console,
    matchMedia: () => ({ matches: false }),
    setTimeout: () => 0,
    clearTimeout() {},
    location: { reload() { reloads++; } },
    document: {
      readyState: 'complete',
      // NO [data-cal-v2-root]. That is the whole shape of the Bench.
      querySelector: (sel) => (sel === '[data-gm-panel]' ? panel : null),
      addEventListener() {},
      removeEventListener() {},
      createElement: () => el(),
      createTextNode: (t) => ({ text: t }),
    },
  };
  sandbox.window = sandbox;
  sandbox.Chronicle = {
    apiFetch: (url, o) => {
      calls.push({ url, opts: o || {} });
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(opts.seed || { weather: { type: 'rain' }, events: [], timeOfDay: 0.5 }),
      });
    },
    notify() {},
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'gm_panel.js' });
  return {
    panel, nodes, calls,
    reloads: () => reloads,
    panelClick: (target) => panelClick({ target }),
    writes: () => calls.filter((c) => (c.opts.method || 'GET') === 'PUT'),
    reads: () => calls.filter((c) => (c.opts.method || 'GET') === 'GET'),
  };
}

// --- the trap ---------------------------------------------------------------

test('wires from its own mount attributes with NO [data-cal-v2-root] present', async () => {
  const h = boot();
  h.nodes.clearAll._click();
  await tick();
  const w = h.writes();
  assert.equal(w.length, 1, 'the console did not write at all — this is the silent-death trap');
  assert.equal(w[0].url, '/campaigns/camp-1/calendar/world-state?calendarId=cal-harptos',
    'the write did not go to the mount\'s own endpoint, calendarId included');
  assert.equal(w[0].opts.headers['X-CSRF-Token'], 'tok-9',
    'the write carried no CSRF token from the mount');
});

// --- every family's payload -------------------------------------------------

test('advance verbs send a signed {advance:{days,hours,minutes}} and re-read the page', async () => {
  const h = boot();
  h.nodes.advance10m._click();
  await tick();
  assert.deepEqual(wire(h.writes()[0]), { advance: { days: 0, hours: 0, minutes: 10 } });
  h.nodes.advance1d._click();
  await tick();
  assert.deepEqual(wire(h.writes()[1]), { advance: { days: 1, hours: 0, minutes: 0 } });
  // The Bench prints the in-world date in SERVER markup the seed cannot repaint
  // (the Block's nameplate, its sky clock and gradient, the today ring), so a
  // date move is re-read rather than patched.
  assert.equal(h.reloads(), 2, 'a date move on a standalone mount must re-read the page');
});

test('Set time sends {time:{hour,minute}} and Set date sends {time:{year,month,day}}', async () => {
  const h = boot();
  h.nodes.setTime._click();
  await tick();
  assert.deepEqual(wire(h.writes()[0]), { time: { hour: 21, minute: 5 } });
  h.nodes.setDate._click();
  await tick();
  assert.deepEqual(wire(h.writes()[1]), { time: { year: 1523, month: 2, day: 14 } });
});

test('an EMPTIED coordinate sends nothing at all (year 0 is not invented)', async () => {
  const h = boot();
  h.nodes.year.value = '   ';
  h.nodes.setDate._click();
  await tick();
  assert.equal(h.writes().length, 0,
    'an empty Year box produced a write — this is the defect that moved a campaign to year 0');
  // A TYPED zero is still a value and is still sent: year 0 and negative years
  // are legitimate on a fantasy calendar, so absence is refused, not a value.
  h.nodes.year.value = '0';
  h.nodes.setDate._click();
  await tick();
  assert.deepEqual(wire(h.writes()[0]), { time: { year: 0, month: 2, day: 14 } });
});

test('a weather tile sends {weather:"<id>"} and does NOT reload the page', async () => {
  const h = boot();
  h.nodes.weatherRain._click();
  await tick();
  assert.deepEqual(wire(h.writes()[0]), { weather: 'rain' });
  assert.equal(h.reloads(), 0,
    'a weather tap reloaded the page — mid-session, per tap, which is what the ' +
    'standalone repaint branch exists to avoid');
  // …and the console repainted itself from the response seed: the active ring
  // follows the server's answer.
  assert.equal(h.nodes.weatherRain.getAttribute('data-gm-active'), 'true');
  assert.equal(h.nodes.weatherClear.getAttribute('data-gm-active'), 'false');
});

test('a world-event tile sends triggerEvent, carrying dm_only from the checkbox', async () => {
  const h = boot();
  h.nodes.dmOnly.checked = true;
  h.nodes.eventEclipse._click();
  await tick();
  assert.deepEqual(wire(h.writes()[0]), {
    triggerEvent: {
      type: 'eclipse-solar', name: 'Solar Eclipse',
      start_hour: 22, duration_hours: 2, dm_only: true,
    },
  });
});

test('the dm_only switch defaults OFF and is honoured as false when unchecked', async () => {
  const h = boot();
  h.nodes.eventEclipse._click();
  await tick();
  assert.equal(wire(h.writes()[0]).triggerEvent.dm_only, false);
});

test('clear-all sends {clearEvents:true}', async () => {
  const h = boot();
  h.nodes.clearAll._click();
  await tick();
  assert.deepEqual(wire(h.writes()[0]), { clearEvents: true });
});

// THE PER-EVENT × CHIP. It had ZERO tests in either language: no Go test
// referenced ClearEventType and no JS test referenced clearEventType, and the
// chips are BUILT BY THE DRIVER, so there is no markup anywhere to assert on.
// This drives the real path — a seed carrying an event, the chip the driver
// builds from it, and the delegated click that clears exactly that type.
test('the per-event × chip is built from the seed and clears that type only', async () => {
  const h = boot({ seed: { weather: { type: 'clear' }, events: [{ type: 'blood-moon', name: 'Blood Moon' }] } });
  h.nodes.clearAll._click();
  await tick();
  const chip = h.nodes.activeEvents.children[0];
  assert.ok(chip, 'no chip was built for the active world-event in the seed');
  // The chip holds a TEXT NODE (the event's name) before the ×, so the search
  // has to tolerate a child with no attributes at all.
  const x = chip.children.find((c) => c.getAttribute && c.getAttribute('data-gm-event-clear'));
  assert.ok(x, 'the chip carries no per-event clear ×');
  assert.equal(x.getAttribute('data-gm-event-clear'), 'blood-moon');
  h.panelClick(x);
  await tick();
  assert.deepEqual(wire(h.writes()[1]), { clearEventType: 'blood-moon' });
});

test('a mood preset sends its colour and intensity; Clear NULLs both', async () => {
  const h = boot();
  h.nodes.moodPreset._click();
  await tick();
  assert.deepEqual(wire(h.writes()[0]),
    { moodTint: { color: 'oklch(0.55 0.22 25)', intensity: 0.45 } });
  h.nodes.moodClear._click();
  await tick();
  assert.deepEqual(wire(h.writes()[1]), { moodTint: { color: null, intensity: 0 } });
});

test('the custom colour picker commits at the slider’s strength', async () => {
  const h = boot();
  h.nodes.moodCustom._change();
  await tick();
  assert.deepEqual(wire(h.writes()[0]), { moodTint: { color: '#112233', intensity: 0.6 } });
});

// THE SLIDER'S READ PATH, which used to be engine-dependent. It re-commits the
// ACTIVE wash at a new strength by reading the current colour — and it read it
// off `window.__calWorldState`, a global defined by cal-almanac.js. On a page
// with no engine that is never there, so the control silently did nothing. The
// driver now remembers the last seed the SERVER gave it.
test('the intensity slider re-commits the active wash without the engine global', async () => {
  const h = boot({ seed: { weather: { type: 'clear' }, events: [], moodTint: { color: '#abcdef' } } });
  // No wash known yet → nothing to re-strengthen, and nothing is sent.
  h.nodes.moodSlider._change();
  await tick();
  assert.equal(h.writes().length, 0, 'the slider invented a wash before one was known');
  // A commit teaches the driver the current seed…
  h.nodes.clearAll._click();
  await tick();
  // …and now the slider can re-commit that colour at the new strength.
  h.nodes.moodSlider.value = '20';
  h.nodes.moodSlider._change();
  await tick();
  assert.deepEqual(wire(h.writes()[1]), { moodTint: { color: '#abcdef', intensity: 0.2 } });
});

// THE CLOCK IS THE SERVER'S ON A STANDALONE MOUNT. syncFromSeed's readout
// assumes a 24-HOUR DAY and prints unpadded `H:MM`; Chronicle's calendars may
// have any hours-per-day, and the Bench renders the clock server-side through
// Calendar.FormatCurrentTime, which knows the real geometry and pads.
// Repainting it from the seed would make a ten-hour-day calendar print a wrong
// time after a WEATHER tap — one readout drifting alone from the three other
// places the page prints the same date.
test('a standalone mount does NOT repaint its server-rendered clock', async () => {
  const h = boot();
  h.nodes.clock.textContent = '07:30';
  h.nodes.weatherRain._click();
  await tick();
  assert.equal(h.nodes.clock.textContent, '07:30',
    'the driver overwrote a server-rendered clock with its own 24-hour arithmetic');
});

test('reset sky sends all three clears in ONE partial PUT', async () => {
  const h = boot();
  h.nodes.reset._click();
  await tick();
  assert.deepEqual(wire(h.writes()[0]), {
    weather: 'clear', clearEvents: true, moodTint: { color: null, intensity: 0 },
  });
});

// --- the opening seed -------------------------------------------------------

test('the seed is GET on first open, once, and repaints the console', async () => {
  const h = boot();
  assert.equal(h.reads().length, 0, 'a collapsed console must not pay for a seed nobody opened');
  h.nodes.pill._click();
  await tick();
  const reads = h.reads();
  assert.equal(reads.length, 1, 'opening the console did not pull the world-state seed');
  assert.equal(reads[0].url, '/campaigns/camp-1/calendar/world-state?calendarId=cal-harptos');
  assert.equal(h.nodes.weatherRain.getAttribute('data-gm-active'), 'true',
    'the pulled seed did not reach the active-weather ring');
  h.nodes.pill._click();
  await tick();
  assert.equal(h.reads().length, 1, 'the seed was pulled twice — it is a first-open cost');
});

// --- the V2 console is not disturbed ----------------------------------------

// A mount WITHOUT data-gm-standalone is the V2 console, and every branch this
// slice touched must behave there exactly as it shipped: no seed pull, and a
// commit that falls back to a page reload when the engine has not defined
// window.__calSetWorldState yet.
test('a non-standalone mount pulls no seed and keeps the shipped reload fallback', async () => {
  const h = boot({ standalone: false });
  h.nodes.pill._click();
  await tick();
  assert.equal(h.reads().length, 0, 'the V2 console started fetching a seed it is handed');
  h.nodes.weatherRain._click();
  await tick();
  assert.equal(h.reloads(), 1,
    'the V2 console lost its "engine not ready → page-load read" fallback');
});
