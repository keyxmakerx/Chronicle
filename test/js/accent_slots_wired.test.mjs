// accent_slots_wired.test.mjs — the campaign's Headers and events colour
// (slot 4, --color-accent-surface-1) and Tags and marks colour (slot 5,
// --color-accent-surface-2) paint real pages, not only the Look example.
// Each consumer must read the slot with a fallback to what it drew before,
// so an unset slot changes nothing; and the server must keep emitting the
// derived forms those consumers read.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const read = (p) => readFileSync(join(root, p), 'utf8');

// Every consumer, the exact reference it must make, and how often at least.
// A reference without a fallback would blank the element when the slot is
// unset, so each pattern requires the comma that opens one.
const consumers = [
  ['static/css/page_look_annals.css', /var\(--color-accent-surface-1, var\(--annals-acc\)\)/g, 2, 'Annals title rule'],
  ['static/css/style_kits.css', /var\(--color-accent-surface-1, var\(--kit-acc\)\)/g, 4, 'kit title rules'],
  ['static/css/calendar-view.css', /var\(--color-accent-surface-1-ink, oklch\(/g, 5, 'calendar app events'],
  ['internal/plugins/calendar/dashboard_embed.templ', /var\(--color-accent-surface-1-fill, /g, 1, 'upcoming-events dots'],
  ['internal/plugins/calendar/dashboard_embed.templ', /var\(--color-accent-surface-1-on-fill, /g, 1, 'upcoming-events dot icons'],
  ['internal/app/coming_up.templ', /var\(--color-accent-surface-1-ink, var\(--color-text-secondary\)\)/g, 1, 'Coming up event tiles'],
  ['internal/app/coming_up.templ', /var\(--color-accent-surface-1, var\(--color-border\)\)/g, 1, 'Coming up event tile borders'],
  ['static/css/page_timeline.css', /--ptl-acc: var\(--color-accent-surface-2, /g, 1, 'page timeline marks'],
  ['internal/plugins/timeline/static/js/timeline_widget.js', /var\(--color-accent-surface-2, var\(--color-accent\)\)/g, 1, 'timeline widget dots'],
  ['internal/plugins/timeline/static/js/timeline_viz.js', /var\(--color-accent-surface-2, var\(--color-accent, #6366f1\)\)/g, 3, 'timeline chart connections, arrows and clusters'],
];

for (const [file, re, min, what] of consumers) {
  test(`${what} read the campaign colour with a fallback (${file})`, () => {
    const n = (read(file).match(re) || []).length;
    assert.ok(n >= min, `${file}: expected ${min}+ of ${re}, found ${n}`);
  });
}

test('the server emits slot 4\'s readable forms, only when it is set', () => {
  const go = read('internal/templates/layouts/appearance.go');
  const a = go.indexOf('func surfaceSlotCSS(');
  assert.ok(a > 0, 'surfaceSlotCSS is gone');
  const body = go.slice(a, go.indexOf('\n}\n', a));
  for (const t of ['--color-accent-surface-1-fill', '--color-accent-surface-1-on-fill', '--color-accent-surface-1-ink']) {
    assert.ok(body.includes(t), t);
  }
  assert.match(body, /if !colour\.ValidHex\(c\) \{\s*return/, 'an unset slot must write nothing');
  assert.match(go, /surfaceSlotCSS\(ctx, a, &root, &dark\)/, 'AppearanceCSS no longer calls surfaceSlotCSS');
});

test('focus and selection stay on the first colour', () => {
  assert.match(read('static/css/page_timeline.css'), /\.ptl-ev:focus-visible[^}]*var\(--kit-acc, var\(--color-accent\)\)/);
  // The chart's minimap viewport is a selection.
  const viz = read('internal/plugins/timeline/static/js/timeline_viz.js');
  assert.match(viz, /\.attr\('fill', 'var\(--color-accent, #6366f1\)'\)/);
});
