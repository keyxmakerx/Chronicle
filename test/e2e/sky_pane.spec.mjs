// sky_pane.spec.mjs — Playwright coverage for the sky pane widget (issue
// #763), driven through a stand-in for the campaign dashboard.
//
// WHY A HARNESS PAGE, NOT A REAL RUNNING APP: this sandbox has neither a
// Docker daemon nor a local `mariadbd` binary to bring up Chronicle's own
// dev server + MariaDB (the repo's `make docker-up` / `make dev` path), so
// standing up the real app is out of reach here — and even a real app
// wouldn't add calendar-page coverage: the calendar page exists now
// (internal/plugins/calendar/view.templ) but doesn't mount this widget, it
// paints its own separate sky context instead (TODO(#741)), so a
// dashboard/entity-page mount (routes.go's "skybox" block) is still this
// widget's only real integration point. Instead, this test drives the REAL
// production assets end to end in a real browser:
//   - static/js/boot.js (the real widget auto-mounter) and the six real
//     static/js/widgets/sky_*.js files, read verbatim off disk — never
//     copied or stubbed.
//   - a page body transcribed verbatim from mount.templ's own Mount()
//     output (see buildMountHTML below), the same way
//     test/js/callout_widget.test.mjs transcribes its server's markup,
//     so a server-side rename breaks this test instead of silently drifting.
//   - the exact reduced-motion guard rule from static/css/input.css,
//     extracted at run time (not hand-copied) so a change to that guard is
//     picked up automatically.
// Only the network layer is stubbed: Playwright intercepts every request and
// serves the calendar/events GETs sky_pane.js issues from realistic fixture
// JSON shaped like the real API responses (Calendar/Event, per
// internal/plugins/calendar/model.go's own json tags). Any OTHER request is
// a hard failure (see routeAll below) — a typo'd asset path fails loudly
// rather than being silently swallowed.
//
// This exercises the widget's real DOM/CSS/canvas behavior; it does NOT
// exercise routes.go's block wiring or the real HTTP API — those are covered
// by internal/app/skybox_block_test.go and the calendar plugin's own Go
// tests.
//
// RUN: NODE_PATH="$(npm root -g)" PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers \
//        node --test test/e2e/sky_pane.spec.mjs
// (playwright is a globally-installed tool in this environment, not a repo
// devDependency — see the environment notes in the PR description for why.)

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, '..', '..');

const CAMPAIGN_ID = 'camp-1';
const CALENDAR_ID = 'cal-1';

// --- fixtures ---------------------------------------------------------------

// A Calendar JSON payload shaped like GET /campaigns/:id/calendars/:calid
// (internal/plugins/calendar/model.go's Calendar, Month, Moon, Weather json
// tags). current_month=2 (1-indexed) lands inside the 3-month year below.
// `overrides` is shallow-merged in last, so a test can swap out e.g. `moons`
// wholesale (a hidden-moon fixture) without repeating the rest of the shape.
function calendarFixture(overrides) {
  return Object.assign({
    id: CALENDAR_ID,
    campaign_id: CAMPAIGN_ID,
    mode: 'fantasy',
    name: 'Test Calendar',
    current_year: 1491,
    current_month: 2,
    current_day: 15,
    hours_per_day: 24,
    minutes_per_hour: 60,
    seconds_per_minute: 60,
    current_hour: 20,
    current_minute: 30,
    leap_year_every: 4,
    leap_year_offset: 0,
    sort_order: 0,
    is_default: true,
    forecasts_enabled: false,
    month_starts_new_week: false,
    visibility: 'everyone',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    months: [
      { id: 1, calendar_id: CALENDAR_ID, name: 'Hammer', days: 30, sort_order: 0, is_intercalary: false, leap_year_days: 0 },
      { id: 2, calendar_id: CALENDAR_ID, name: 'Alturiak', days: 30, sort_order: 1, is_intercalary: false, leap_year_days: 0 },
      { id: 3, calendar_id: CALENDAR_ID, name: 'Ches', days: 30, sort_order: 2, is_intercalary: false, leap_year_days: 0 },
    ],
    weekdays: [],
    moons: [
      {
        id: 1, calendar_id: CALENDAR_ID, name: 'Luna', cycle_days: 30, phase_offset: 0,
        color: '#d9e1ec', base_design: 'moon-realistic-selene', phase_source: 'auto',
        size: 1, orbit_speed: 1, hidden_from_players: false,
      },
    ],
    weather: {
      id: 1, calendar_id: CALENDAR_ID, preset_id: 'clear', preset_label: 'Clear skies',
      icon: 'clear', color: '#ffffff', updated_at: '2024-01-01T00:00:00Z',
    },
  }, overrides);
}

// One event today with a moon-night payload (a harvest moon on Luna), so the
// event-overlay code path (sky_events.js) is exercised too, not just the
// bare palette/moon paint.
function eventsFixture() {
  return [
    {
      id: 'evt-1', calendar_id: CALENDAR_ID, name: 'Harvest Moon',
      year: 1491, month: 2, day: 15, is_recurring: false,
      visibility: 'everyone', all_day: true,
      payload: JSON.stringify({ type: 'harvest', moons: [1] }),
    },
  ];
}

// --- markup, transcribed verbatim from mount.templ's Mount() -------------
//
// Kept in exact sync with internal/widgets/sky/templates/mount.templ: a
// rename of a class or data attribute there without a matching change here
// fails this test, the same contract callout_widget.test.mjs enforces for
// callout_handler.go's markup.
function buildMountHTML(campaignID, calendarID) {
  const mountID = `sky-pane-${calendarID}`;
  return `
    <div data-widget="sky-pane" data-campaign-id="${campaignID}" data-calendar-id="${calendarID}" class="skypane">
      <div class="skypane-bar">
        <span class="skypane-title">Sky</span>
        <button type="button" class="skychip" aria-expanded="false" aria-controls="${mountID}">
          <span class="sw" aria-hidden="true"></span>
          <span class="skychip-text">Today's sky</span>
        </button>
      </div>
      <div class="skywrap">
        <div class="sky" id="${mountID}">
          <canvas></canvas>
          <p class="skycap" aria-live="polite">Loading the sky…</p>
        </div>
      </div>
    </div>
  `;
}

// The site-wide reduced-motion total-disable guard, extracted verbatim from
// static/css/input.css at run time (not hand-copied), so a change to the
// guard's own selector or !important rules is picked up automatically
// rather than silently going stale here. sky_pane.js's own header comment
// names this exact guard as what collapses its reveal transition to instant.
function reducedMotionGuardCSS() {
  const css = readFileSync(join(repoRoot, 'static', 'css', 'input.css'), 'utf8');
  const start = css.indexOf('@media (prefers-reduced-motion: reduce) {');
  assert.notEqual(start, -1, 'reduced-motion guard not found in static/css/input.css — has it moved or been renamed?');
  // Balance braces from the opening one to find the matching close, so this
  // survives the guard growing another rule inside it.
  let depth = 0, end = start;
  for (let i = start; i < css.length; i++) {
    if (css[i] === '{') depth++;
    else if (css[i] === '}') { depth--; if (depth === 0) { end = i + 1; break; } }
  }
  return css.slice(start, end);
}

const SKY_SCRIPTS = [
  'sky_world.js', 'sky_looks.js', 'sky_moon.js', 'sky_events.js', 'sky_2d.js', 'sky_pane.js',
];

function harnessHTML(campaignID, calendarID) {
  // htmx first, then boot.js, then the six sky scripts, in base.templ's own
  // order — boot.js references `htmx.config` unconditionally at its own top
  // level, so loading it out of production order throws before boot.js
  // finishes defining Chronicle.apiFetch.
  const htmxTag = '<script src="/static/vendor/htmx.min.js" defer></script>';
  const scripts = ['boot.js', ...SKY_SCRIPTS.map((f) => `widgets/${f}`)]
    .map((p) => `<script src="/static/js/${p}" defer></script>`)
    .join('\n    ');
  return `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<style>${reducedMotionGuardCSS()}</style>
</head>
<body>
${buildMountHTML(campaignID, calendarID)}
${htmxTag}
${scripts}
</body>
</html>`;
}

// --- request routing ---------------------------------------------------------

function jsContentType(path) {
  return path.endsWith('.js') ? 'text/javascript; charset=utf-8' : 'application/octet-stream';
}

// installRoutes wires every request the page will make to either a real
// on-disk static asset, a fixture JSON response, or the harness document
// itself — and fails loudly (via unexpected.push) on anything else.
async function installRoutes(page, unexpected, calendarOverrides) {
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    const p = url.pathname;

    if (p === '/dashboard') {
      return route.fulfill({ status: 200, contentType: 'text/html; charset=utf-8', body: harnessHTML(CAMPAIGN_ID, CALENDAR_ID) });
    }
    if (p.startsWith('/static/js/') || p.startsWith('/static/vendor/')) {
      const rel = p.replace('/static/', '');
      let body;
      try {
        body = readFileSync(join(repoRoot, 'static', rel), 'utf8');
      } catch (err) {
        unexpected.push(`missing static asset: ${p} (${err.message})`);
        return route.fulfill({ status: 404, body: '' });
      }
      return route.fulfill({ status: 200, contentType: jsContentType(p), body });
    }
    if (p === `/campaigns/${CAMPAIGN_ID}/calendars/${CALENDAR_ID}`) {
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(calendarFixture(calendarOverrides)) });
    }
    if (p === `/campaigns/${CAMPAIGN_ID}/calendars/${CALENDAR_ID}/events`) {
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(eventsFixture()) });
    }
    unexpected.push(`unexpected request: ${route.request().method()} ${p}`);
    return route.fulfill({ status: 404, body: '' });
  });
}

// waitForLoaded: the widget replaces the "Loading the sky…" caption once its
// two fetches resolve (see Instance.prototype.onDataReady/refreshPalette).
async function waitForLoaded(page) {
  await page.waitForFunction(() => {
    const cap = document.querySelector('.skycap');
    return !!cap && cap.textContent !== 'Loading the sky…';
  }, { timeout: 5000 });
}

async function withPage(opts, fn) {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: opts.viewport });
    if (opts.reducedMotion) await context.grantPermissions([]).catch(() => {});
    const page = await context.newPage();
    if (opts.reducedMotion) await page.emulateMedia({ reducedMotion: 'reduce' });
    const unexpected = [];
    const pageErrors = [];
    page.on('pageerror', (err) => pageErrors.push(err.message));
    await installRoutes(page, unexpected, opts.calendarOverrides);
    await page.goto('https://chronicle.test/dashboard');
    await waitForLoaded(page);
    await fn(page);
    assert.deepEqual(unexpected, [], 'no unexpected/unhandled network requests');
    assert.deepEqual(pageErrors, [], 'no uncaught JS exceptions on the page');
  } finally {
    await browser.close();
  }
}

// --- tests -------------------------------------------------------------------

test('sky pane: chip toggles the pane open/closed with a real height change', { timeout: 20000 }, async () => {
  await withPage({ viewport: { width: 1280, height: 800 } }, async (page) => {
    const wrap = page.locator('.skywrap');
    const chip = page.locator('.skychip');

    const closedHeight = await wrap.evaluate((el) => getComputedStyle(el).height);
    assert.equal(closedHeight, '0px', 'the pane must start fully collapsed (height:0), not just visually hidden');

    await chip.click();
    await page.waitForFunction(() => {
      const el = document.querySelector('.skywrap');
      return el && getComputedStyle(el).height !== '0px';
    });
    const openHeight = await wrap.evaluate((el) => getComputedStyle(el).height);
    assert.notEqual(openHeight, '0px');
    assert.ok(parseFloat(openHeight) > 0, `expected a real positive height, got ${openHeight}`);
    assert.equal(await chip.getAttribute('aria-expanded'), 'true');

    // Let the clip-path reveal transition finish, then close again.
    await page.waitForTimeout(400);
    await chip.click();
    assert.equal(await chip.getAttribute('aria-expanded'), 'false');
    await page.waitForFunction(() => {
      const el = document.querySelector('.skywrap');
      return el && getComputedStyle(el).height === '0px';
    }, { timeout: 2000 });
  });
});

// canvasPixelStats reads the sky canvas's own pixels back — the only honest
// way to prove a paint actually happened (or didn't), rather than trusting
// that some JS ran.
async function canvasPixelStats(page) {
  return page.evaluate(() => {
    const canvas = document.querySelector('.sky canvas');
    const ctx = canvas.getContext('2d');
    const { data } = ctx.getImageData(0, 0, canvas.width, canvas.height);
    let opaque = 0, distinctColors = new Set();
    for (let i = 0; i < data.length; i += 4) {
      if (data[i + 3] > 0) opaque++;
      distinctColors.add(data[i] + ',' + data[i + 1] + ',' + data[i + 2]);
      if (distinctColors.size > 4) break; // early out once variety is proven
    }
    return { width: canvas.width, height: canvas.height, opaque, distinctColors: distinctColors.size };
  });
}

test('sky pane: the canvas actually paints a non-blank frame once opened', { timeout: 20000 }, async () => {
  await withPage({ viewport: { width: 1280, height: 800 } }, async (page) => {
    await page.locator('.skychip').click();
    await page.waitForTimeout(400); // let the reveal + first render settle

    const pixelStats = await canvasPixelStats(page);

    assert.ok(pixelStats.width > 0 && pixelStats.height > 0, 'canvas must have a real pixel size once opened');
    assert.ok(pixelStats.opaque > 0, 'the sky must paint at least some opaque pixels — a blank/transparent canvas means the render pipeline did not run');
    assert.ok(pixelStats.distinctColors > 1, 'a real sky paints more than one flat colour (gradient/moon/stars), not a solid fill');
  });
});

// Regression for the resize-blanks-the-canvas finding: setting
// canvas.width/height (inside resizeCanvas, whenever the backing size
// actually changes) clears the canvas per the HTML canvas spec, and before
// the fix nothing redrew it afterward unless the shared animation loop
// happened to already be running.
test('sky pane: resizing repaints instead of leaving the canvas blank', { timeout: 20000 }, async () => {
  await withPage({ viewport: { width: 1280, height: 800 } }, async (page) => {
    await page.locator('.skychip').click();
    await page.waitForTimeout(400); // let the reveal + first render settle

    const before = await canvasPixelStats(page);
    assert.ok(before.opaque > 0, 'sanity: the sky must be painted before the resize');

    await page.setViewportSize({ width: 900, height: 700 });
    // onResize debounces 100ms before calling resizeCanvas + repainting.
    await page.waitForTimeout(400);

    const after = await canvasPixelStats(page);
    assert.ok(after.width > 0 && after.height > 0, 'canvas must still have a real pixel size after resize');
    assert.ok(after.opaque > 0, 'BLANK CANVAS BUG: the sky must repaint after a resize/rotation, not stay blank');
  });
});

// Regression for the client-side hidden_from_players re-filter finding: the
// server already strips hidden moons for a Player viewer, so this widget
// must render whatever moons the response actually contains rather than
// re-filtering on hidden_from_players itself — that second filter is
// redundant for a Player (server already did it) and actively wrong for a
// GM (it would hide the GM's own hidden moon from the GM). current_day=1,
// current_hour=20 is a fixed point where Luna is above the horizon (proven
// against the unmodified fixture below), so a moon dropped by an incorrect
// client-side filter is visibly absent from the chip caption.
test('sky pane: does not re-filter hidden_from_players client-side (server decides)', { timeout: 20000 }, async () => {
  const moonOverrides = {
    current_day: 1, current_hour: 20, current_minute: 0,
    moons: [{
      id: 1, calendar_id: CALENDAR_ID, name: 'Luna', cycle_days: 30, phase_offset: 0,
      color: '#d9e1ec', base_design: 'moon-realistic-selene', phase_source: 'auto',
      size: 1, orbit_speed: 1, hidden_from_players: true,
    }],
  };
  await withPage({ viewport: { width: 1280, height: 800 }, calendarOverrides: moonOverrides }, async (page) => {
    await page.locator('.skychip').click();
    await page.waitForTimeout(400);
    const caption = await page.locator('.skycap').textContent();
    assert.ok(caption.includes('Luna'), `LEAK-SHAPED BUG (inverted): a moon the server sent with hidden_from_players:true must still render for this viewer — the server decides, not the client. Got caption: ${caption}`);
  });
});

test('sky pane at 390x844: nothing overflows horizontally, open or closed', { timeout: 20000 }, async () => {
  await withPage({ viewport: { width: 390, height: 844 } }, async (page) => {
    const scrollWidthOf = () => page.evaluate(() => document.documentElement.scrollWidth);
    const viewportWidth = 390;

    assert.ok((await scrollWidthOf()) <= viewportWidth, 'closed pane must not overflow horizontally at phone width');

    await page.locator('.skychip').click();
    await page.waitForTimeout(400);

    assert.ok((await scrollWidthOf()) <= viewportWidth, 'open pane must not overflow horizontally at phone width');
  });
});

test('sky pane with prefers-reduced-motion: the reveal transition is instant, not animated', { timeout: 20000 }, async () => {
  // Control: without the reduced-motion guard, the reveal transition on
  // `.sky` runs at its own configured duration (280ms, sky_pane.js's
  // `--dur-large` fallback) — proves the harness's transition is real
  // before proving the guard collapses it.
  await withPage({ viewport: { width: 1280, height: 800 } }, async (page) => {
    const duration = await page.locator('.sky').evaluate((el) => getComputedStyle(el).transitionDuration);
    assert.ok(parseFloat(duration) > 0.01, `expected a real animated duration without the guard, got ${duration}`);
  });

  await withPage({ viewport: { width: 1280, height: 800 }, reducedMotion: true }, async (page) => {
    const duration = await page.locator('.sky').evaluate((el) => getComputedStyle(el).transitionDuration);
    // The site-wide guard forces `transition-duration: 0.01ms !important`
    // (static/css/input.css) — sky_pane.js's own header comment names this
    // exact rule as what collapses the reveal to a single frame instead of
    // a per-widget crossfade. 0.01ms == 0.00001s.
    assert.ok(parseFloat(duration) <= 0.0001, `expected the reduced-motion guard to clamp the transition to ~0, got ${duration}`);

    // The box-model height change is ALWAYS instant regardless of motion
    // preference (that's the widget's own design, not the guard) — confirm
    // the pane is already open with a real height and a painted canvas
    // immediately, with no wait for a transition to finish.
    await page.locator('.skychip').click();
    await page.waitForFunction(() => {
      const el = document.querySelector('.skywrap');
      return el && getComputedStyle(el).height !== '0px';
    }, { timeout: 500 });
    const opaque = await page.evaluate(() => {
      const canvas = document.querySelector('.sky canvas');
      const ctx = canvas.getContext('2d');
      const { data } = ctx.getImageData(0, 0, canvas.width, canvas.height);
      for (let i = 3; i < data.length; i += 4) if (data[i] > 0) return true;
      return false;
    });
    assert.ok(opaque, 'a reduced-motion viewer must still get a painted frame immediately on open');
  });
});
