// sky_events.test.mjs — the moving things a look or a calendar event adds to
// the sky (static/js/widgets/sky_events.js).
//
// Every number handed to the painter must be finite: one NaN in a meteor
// reaches a uniform every pixel reads, and the whole sky draws black for
// that frame, so a meteor shower blinked on and off.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const dir = join(here, '..', '..', 'static', 'js', 'widgets');

function load() {
  const sandbox = { console, Math, JSON, Object, Array, Number, String, Float32Array, Uint8Array, Uint8ClampedArray, Error };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  for (const f of ['sky_world.js', 'sky_looks.js', 'sky_events.js']) {
    vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox, { filename: f });
  }
  return sandbox;
}

test('a meteor shower never hands the painter a non-finite meteor', () => {
  const { SkyEvents, SkyLooks } = load();
  const look = SkyLooks.resolve(SkyLooks.spec({ preset_id: 'meteor-shower', preset_label: 'Meteor shower' }));
  assert.ok(look.fire.length, 'the meteor look brings a shower');
  let frames = 0;
  for (let t = 0; t < 60; t += .05) {
    const st = { W: 600, H: 150, L: { hor: 120 }, look, dark: 1, wx: { cloud: 0 }, t, rm: false, moons: [], sl: { night: 1, col: [1, 1, 1], amb: .3 } };
    SkyEvents.apply(st, []);
    st.met.forEach((m) => m.forEach((v, i) => assert.ok(Number.isFinite(v), `meteor value ${i} is ${v} at t=${t.toFixed(2)}`)));
    if (st.met.length) frames++;
  }
  assert.ok(frames > 100, 'the shower is actually falling, got ' + frames + ' frames with meteors');
});
