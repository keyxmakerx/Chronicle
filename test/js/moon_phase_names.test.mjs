// moon_phase_names.test.mjs — the browser's three copies of the moon phase
// names (the calendar page, the sky pane and the generator engine) must agree
// with Moon.MoonPhaseName in internal/plugins/calendar/model.go, whose own
// test is moon_phase_test.go. Each name is centered on its turning point, so
// the night before a full moon reads "Full Moon" and New Moon wraps the cycle.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const widgets = join(here, '..', '..', 'static', 'js', 'widgets');

// Each copy is a small pure function inside a larger file that needs a DOM,
// so it is lifted out by its own signature and evaluated on its own.
const copies = [
  ['calendar_view.js', /name: function \(phase\) \{[\s\S]*?\n {4}\}/, 'name: '],
  ['sky_pane.js', /function phaseName\(p\) \{[\s\S]*?\n {2}\}/, ''],
  ['chronicle_gen.js', /function moonPhaseName\(p\) \{[\s\S]*?\n\}/, ''],
].map(([file, re, prefix]) => {
  const m = readFileSync(join(widgets, file), 'utf8').match(re);
  assert.ok(m, `${file}: phase-name function not found`);
  return [file, vm.runInNewContext('(' + m[0].slice(prefix.length) + ')')];
});

const NAMES = ['New Moon', 'Waxing Crescent', 'First Quarter', 'Waxing Gibbous',
  'Full Moon', 'Waning Gibbous', 'Last Quarter', 'Waning Crescent'];

test('each name holds from half a sector before its turning point', () => {
  for (const [file, name] of copies) {
    NAMES.forEach((want, i) => {
      // The same 32-day cycle as the Go test: day i*4 is the turning point.
      for (const day of [i * 4 - 2, i * 4, i * 4 + 1]) {
        const phase = ((day / 32) % 1 + 1) % 1;
        assert.equal(name(phase), want, `${file}: phase ${phase}`);
      }
    });
  }
});

test('New Moon wraps across the end of the cycle', () => {
  for (const [file, name] of copies) {
    for (const phase of [0, 0.03, 0.0624, 0.9375, 0.99]) {
      assert.equal(name(phase), 'New Moon', `${file}: phase ${phase}`);
    }
    assert.equal(name(0.4375), 'Full Moon', `${file}: the night before full`);
    assert.equal(name(0.4374), 'Waxing Gibbous', `${file}: just before the Full Moon sector`);
  }
});
