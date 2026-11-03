// craft_paint.test.mjs — the painted benches' rules: static/js/widgets/craft_paint.js
// and craft_paint_stage.js.
//
// Pins what the stage promises a slow device: frames that run slow for about
// two seconds step the effect level down, a short stutter does not, the first
// frames after a level starts are not judged, and a device that looks weak
// starts a level lower. Also the motion choices (Off beats Calm, any reduce
// switch means Calm), the level names, and the toolkit's seeded noise and
// particles. Runs the scripts in a vm with a minimal fake window.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const dir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'static', 'js', 'widgets');
const read = (f) => readFileSync(join(dir, f), 'utf8');

function load() {
  const window = {};
  const document = { addEventListener: () => {}, documentElement: { getAttribute: () => null, hasAttribute: () => false } };
  const ctx = vm.createContext({ window, document, Float32Array, Math });
  for (const f of ['craft_paint.js', 'craft_paint_forge.js', 'craft_paint_stage.js']) vm.runInContext(read(f), ctx);
  return { P: window.CraftPaint, S: window.CraftStage };
}
const { P, S } = load();

function frames(gov, ms, n) {
  for (let i = 0; i < n; i++) if (gov.tick(ms)) return i + 1;
  return 0;
}

test('steady frames never step the level down', () => {
  assert.equal(frames(S.governor(), 16.7, 2000), 0);
  assert.equal(frames(S.governor(), 22, 2000), 0, 'a little under the line is fine');
});

test('about two seconds of slow frames steps the level down', () => {
  const at = frames(S.governor(), 40, 500);
  // 30 frames are not judged, then two seconds of 40 ms frames is about 50 more.
  assert.ok(at > 30 + 45 && at < 30 + 70, `stepped down after ${at} frames`);
});

test('the first frames after a level starts are not judged', () => {
  const g = S.governor();
  assert.equal(frames(g, 100, 30), 0);
});

test('a short stutter does not step the level down, and slow time drains away', () => {
  const g = S.governor();
  frames(g, 16.7, 40);
  assert.equal(frames(g, 60, 12), 0, 'a stutter of a dozen slow frames');
  frames(g, 16.7, 600);
  assert.equal(g.slowFor, 0, 'normal frames drain what the stutter added');
});

test('reset starts the watch again', () => {
  const g = S.governor();
  frames(g, 40, 70);
  g.reset();
  assert.equal(g.slowFor, 0);
  assert.equal(g.warm, 30);
  assert.equal(g.ema, 16.7);
});

test('a device that looks weak starts at canvas, others at the graphics card', () => {
  assert.equal(S.startLevel({ hardwareConcurrency: 8, deviceMemory: 8 }), 0);
  assert.equal(S.startLevel({}), 0, 'a browser that says nothing starts at the top');
  assert.equal(S.startLevel(undefined), 0);
  assert.equal(S.startLevel({ hardwareConcurrency: 4 }), 1);
  assert.equal(S.startLevel({ hardwareConcurrency: 8, deviceMemory: 2 }), 1);
  assert.equal(S.startLevel({ hardwareConcurrency: 8, connection: { saveData: true } }), 1);
});

function root(attrs) {
  return { getAttribute: (k) => (k in attrs ? attrs[k] : null), hasAttribute: (k) => k in attrs };
}
test('motion: Off beats Calm, and any reduce switch means Calm', () => {
  assert.equal(S.motionMode(root({}), false), 'full');
  assert.equal(S.motionMode(root({}), true), 'calm', 'the device asks for less motion');
  assert.equal(S.motionMode(root({ 'data-motion': 'calm' }), false), 'calm');
  assert.equal(S.motionMode(root({ 'data-view-motion': 'calm' }), false), 'calm');
  assert.equal(S.motionMode(root({ 'data-cz-reduce': '' }), false), 'calm');
  assert.equal(S.motionMode(root({ 'data-motion': 'off' }), true), 'off');
});

test('levels run from the richest to the still frame, and unknown names are not levels', () => {
  assert.deepEqual([...S.LEVELS], ['gpu', 'canvas', 'basic', 'still']);
  assert.equal(S.levelOf('canvas'), 1);
  assert.equal(S.levelOf('auto'), -1);
  assert.equal(S.levelOf('<b>'), -1);
  assert.equal(S.levelOf(undefined), -1);
});

test('the forge registers as a scene', () => {
  assert.equal(typeof P.scenes.forge, 'function');
});

test('seeded noise and random numbers repeat, so baked textures look the same everywhere', () => {
  const a = P.rng(42), b = P.rng(42);
  for (let i = 0; i < 20; i++) assert.equal(a(), b());
  const n1 = P.noise(9), n2 = P.noise(9);
  assert.equal(n1.fbm(3.3, 7.1, 4), n2.fbm(3.3, 7.1, 4));
  const v = n1.v(12.5, 3.25);
  assert.ok(v >= 0 && v <= 1);
});

test('particles fall, bounce off the floor and are dropped when spent', () => {
  const ps = new P.Particles();
  ps.add({ x: 0, y: 0, vx: 0, vy: 100, g: 0, bounce: .5, max: .5 });
  ps.step(.1, 5);
  assert.equal(ps.list[0].y, 5, 'it stops at the floor');
  assert.equal(ps.list[0].vy, -50, 'and rises with half its speed');
  ps.step(.5, 5);
  assert.equal(ps.list.length, 0);
});
