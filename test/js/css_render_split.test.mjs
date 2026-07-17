// css_render_split.test.mjs — the prod↔demo RENDER-PARITY GUARD
// (C-CAL-WORLDSTATE-GM-OVERHAUL §4 — the structural fix for the recurring
// "works in /demo, broken in production" class).
//
// The contract:
//   • cal-almanac-render.css = the widget's RENDER layer. Loaded by EVERY
//     surface (/demo, /calendar/v2 band, both entity embeds, the dashboard
//     shelf). It must never scope a rule to the demo shell — a shell-scoped
//     rule silently skips production, which is exactly the historical bug
//     class (unstyled shelf, giant unstyled stars, missing sun…).
//   • cal-almanac.css = demo CHROME only. It must not (re)style the widget's
//     render classes — a render rule landing there would exist demo-only and
//     re-open the drift. (Forced-state proof classes for the headless
//     screenshot gate are the one sanctioned exception, plus the shell's
//     legitimately demo-only INTERACTION affordances on render elements —
//     cursor/pointer-events for demo-only controls.)
//   • Every surface that renders the widget must link the render layer.
//
// If this test fails, a render rule is about to ship demo-only (or the file
// roles got mixed). Fix the rule's home, don't loosen the guard.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const renderCSS = readFileSync(join(root, 'static', 'css', 'cal-almanac-render.css'), 'utf8');
const chromeCSS = readFileSync(join(root, 'static', 'css', 'cal-almanac.css'), 'utf8');

// Strip comments so prose mentioning class names can't trip the guard.
function stripComments(css) {
  return css.replace(/\/\*[\s\S]*?\*\//g, '');
}
// All selector lines (the text before each '{' at nesting depth 0/1).
function selectors(css) {
  const out = [];
  const noComments = stripComments(css);
  const re = /([^{}]+)\{/g;
  let m;
  while ((m = re.exec(noComments)) !== null) {
    const sel = m[1].trim();
    if (sel.startsWith('@')) continue; // at-rules (their inner selectors recurse via the same regex)
    out.push(...sel.split(',').map((s) => s.trim()).filter(Boolean));
  }
  return out;
}

// The widget's render-class vocabulary: anything matching these is a RENDER
// rule and must live (unscoped) in the render layer.
const RENDER_CLASS = /\.cal-almanac-(sky|sun|moon|shelf|hourglass|rain|snow|fog|cloud|cloudbank|lightning|meteor|eclipse|celestial-stub)\b|\[data-cal-sky[\]-]|\[data-cal-atmosphere-paused|\[data-cal-moon-fx|\[data-cal-hourglass/;

test('render layer never scopes to the demo shell', () => {
  const offenders = selectors(renderCSS).filter((s) => s.includes('.cal-almanac-shell'));
  assert.deepEqual(offenders, [],
    'cal-almanac-render.css must stay shell-free (these rules would silently skip production): ' + offenders.join(' | '));
});

test('demo chrome never (re)styles the widget render classes', () => {
  const offenders = selectors(chromeCSS).filter((s) => {
    if (!RENDER_CLASS.test(s)) return false;
    // Sanctioned: headless-gate forced-state classes...
    if (s.includes('.cal-almanac--proof')) return false;
    // ...and the @supports unsupported-browser guard's shell-wide hide.
    if (s.includes('.cal-almanac-unsupported')) return false;
    // ...and shell-scoped INTERACTION affordances (cursor/pointer wiring for
    // demo-only controls living on render elements). These are whitelisted
    // individually so anything new gets reviewed here.
    const interactionAllow = [
      '.cal-almanac-shell .cal-almanac-sky__time',
      '.cal-almanac-shell .cal-almanac-sky__time-input',
      '.cal-almanac-shell .cal-almanac-sky__time:hover',
      '.cal-almanac-shell .cal-almanac-sky__time:active',
      '.cal-almanac-shell .cal-almanac-sky__time:focus-visible',
      '.cal-almanac-shell .cal-almanac-sky__date',
      '.cal-almanac-shell .cal-almanac-sky__date:hover strong',
      '.cal-almanac-shell .cal-almanac-sky__date:focus-visible',
    ];
    if (interactionAllow.includes(s)) return false;
    return true;
  });
  assert.deepEqual(offenders, [],
    'render-class rules found in the demo-chrome file (they would exist demo-only): ' + offenders.join(' | '));
});

test('every widget surface links the render layer', () => {
  const surfaces = [
    'internal/plugins/calendar/calendar_v2_worldstate.templ', // /calendar/v2 band + dashboard shelf
    'internal/plugins/calendar/entity_worldstate_block.templ',
    'internal/plugins/calendar/entity_calendar_block.templ',
    'internal/templates/demo/calendar.templ',
  ];
  for (const f of surfaces) {
    const src = readFileSync(join(root, f), 'utf8');
    assert.ok(src.includes('/static/css/cal-almanac-render.css'),
      f + ' must link cal-almanac-render.css (the shared render layer)');
  }
  // The demo keeps its chrome on top of (after) the render layer.
  const demo = readFileSync(join(root, 'internal/templates/demo/calendar.templ'), 'utf8');
  const renderAt = demo.indexOf('cal-almanac-render.css');
  const chromeAt = demo.indexOf('"/static/css/cal-almanac.css"');
  assert.ok(chromeAt > renderAt, 'demo must load chrome AFTER the render layer (override order)');
  // Production surfaces must NOT load the demo chrome.
  for (const f of surfaces.slice(0, 3)) {
    const src = readFileSync(join(root, f), 'utf8');
    assert.ok(!src.includes('"/static/css/cal-almanac.css"'),
      f + ' must not load the demo-chrome stylesheet');
  }
});

test('the band markup carries both engine canvases (back + front)', () => {
  // C-SKYBOX-WIDGET: the canvas markup now lives in the skybox widget
  // package (internal/widgets/skybox/skybox.templ) — calendar_v2_worldstate
  // .templ's worldStateSkyBandV2 delegates to it (skybox.Skybox) rather than
  // inlining the scaffold, so the render-parity guard now points at the
  // widget's own source, and separately confirms the calendar plugin still
  // composes it.
  const skyboxWidget = readFileSync(join(root, 'internal/widgets/skybox/skybox.templ'), 'utf8');
  assert.ok(skyboxWidget.includes('data-cal-sky-canvas'), 'back canvas missing');
  assert.ok(skyboxWidget.includes('data-cal-sky-canvas-front'), 'front canvas missing');
  const band = readFileSync(join(root, 'internal/plugins/calendar/calendar_v2_worldstate.templ'), 'utf8');
  assert.ok(band.includes('skybox.Skybox('), 'the calendar plugin must still compose the skybox widget for its band');
  const demo = readFileSync(join(root, 'internal/templates/demo/calendar.templ'), 'utf8');
  assert.ok(demo.includes('data-cal-sky-canvas-front'), 'demo must carry the front canvas too (parity)');
});
