// calendar_month_geometry.test.mjs — C-CAL-DESIGN-PASS-1 geometry + touch
// contract pin. The month grid is server-rendered templ, so (like
// css_render_split) this harness pins the rule by asserting the SOURCE carries
// the load-bearing geometry/touch markers rather than by measuring layout —
// screenshot/computed-layout checks aren't available here (the live geometry
// was proven separately via a headless Chromium probe: all week rows equal
// height, cells fill, the 22px number box unchanged across plain/today/event
// cells).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (rel) => readFileSync(join(root, rel), 'utf8');
const templ = read('internal/plugins/calendar/calendar_v2.templ');
const helpers = read('internal/plugins/calendar/calendar_v2_helpers.go');
const inputCss = read('static/css/input.css');

test('uniform grid: week rows share one grid with 1fr auto-rows', () => {
  assert.ok(
    templ.includes('grid-auto-rows: 1fr'),
    'monthWeekRows must wrap every week row in ONE grid with 1fr auto-rows (equal-height rows)',
  );
  assert.ok(
    templ.includes('content-stretch') && templ.includes('flex-1'),
    'the cell layer must stretch (flex-1 + content-stretch) so cells fill the equalized row height',
  );
});

test('fixed number box: every day number sits in a 22px line box, today only repaints', () => {
  const fn = helpers.slice(helpers.indexOf('func monthDayNumberClasses'));
  const body = fn.slice(0, fn.indexOf('\n}'));
  assert.ok(body.includes('h-[22px]'), 'the day-number box must be a fixed 22px line box');
  assert.ok(
    body.includes('rounded-full') && body.includes('bg-accent'),
    'today must REPAINT the number into an accent pill (not resize/move it)',
  );
});

test('touch: hover-revealed month affordances are visible at rest on coarse pointers', () => {
  // The rule must be a real @media (pointer: coarse) block targeting the reveal
  // class, and it must be UNLAYERED (outside @layer) so it beats the layered
  // opacity-50 utility without !important.
  const m = inputCss.match(/@media\s*\(pointer:\s*coarse\)\s*\{[^}]*\.cal-touch-reveal\s*\{[^}]*opacity:\s*1/);
  assert.ok(m, 'input.css must reveal .cal-touch-reveal at opacity:1 under @media (pointer: coarse)');
  // Guard against it being nested inside an @layer block (which would lose to
  // the utilities layer). Find the coarse block and ensure the nearest
  // preceding unbalanced brace is not an @layer.
  const idx = inputCss.indexOf('@media (pointer: coarse)');
  const before = inputCss.slice(0, idx);
  const layerOpens = (before.match(/@layer[^;{]*\{/g) || []).length;
  const anyOpens = (before.match(/\{/g) || []).length;
  const anyCloses = (before.match(/\}/g) || []).length;
  assert.equal(anyOpens - anyCloses, 0, 'the touch rule must be unlayered (not inside an open @layer/{} block)');
  assert.ok(layerOpens >= 0); // sanity
});

test('pips + hooks: month event lines keep the compact-card interaction hooks', () => {
  const fn = templ.slice(templ.indexOf('templ monthEventLine'));
  const body = fn.slice(0, fn.indexOf('\ntempl ') > 0 ? fn.indexOf('\ntempl ') : fn.length);
  for (const hook of ['data-event-card="compact"', 'data-event-id=', 'draggable="true"']) {
    assert.ok(body.includes(hook), `month event line must carry ${hook} (click→edit + drag survive the pip refactor)`);
  }
});

test('overflow: the +N more toggle + its endpoint hooks are preserved', () => {
  assert.ok(templ.includes('data-cell-overflow-toggle="true"'), 'the +N more overflow toggle must remain');
  assert.ok(templ.includes('data-cell-overflow-day='), 'the overflow toggle must keep its day hook');
});
