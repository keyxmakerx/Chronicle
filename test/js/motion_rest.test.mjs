// motion_rest.test.mjs — the rest clock every looping animation reads its time
// from (static/js/motion_rest.js).
//
// Pins the behaviour signed on #935: stepping away eases motion to a standstill
// over about two seconds, the clock then stops, and coming back eases it up
// over about a second from exactly where it stopped. A hidden tab rests at
// once, and eight seconds without input counts as stepping away.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'motion_rest.js'), 'utf8');

// A page with a hand-driven clock: timers and frames run only when the test moves time on.
function load() {
  let now = 0;
  const timers = [], frames = [], on = {}, rootOn = {}, docOn = {};
  const listen = (map) => (ev, fn) => { (map[ev] = map[ev] || []).push(fn); };
  const fire = (map, ev) => (map[ev] || []).slice().forEach((fn) => fn({ type: ev }));
  const document = { hidden: false, addEventListener: listen(docOn), documentElement: { addEventListener: listen(rootOn) } };
  const sandbox = {
    console, Math, performance: { now: () => now }, document,
    addEventListener: listen(on),
    requestAnimationFrame: (fn) => { frames.push(fn); return frames.length; },
    setTimeout: (fn, ms) => { timers.push({ at: now + ms, fn }); return timers.length; },
    clearTimeout: (id) => { if (timers[id - 1]) timers[id - 1].fn = null; },
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'motion_rest.js' });
  // Moves time on in frame-sized steps, running any timer and frame that falls due.
  const advance = (s) => {
    const end = now + s * 1000;
    while (now < end) {
      now = Math.min(end, now + 16);
      timers.forEach((t) => { if (t.fn && t.at <= now) { const fn = t.fn; t.fn = null; fn(); } });
      frames.splice(0).forEach((fn) => fn(now));
    }
  };
  return {
    R: sandbox.MotionRest, advance,
    input: (ev = 'pointermove') => fire(on, ev),
    hide: (h) => { document.hidden = h; fire(docOn, 'visibilitychange'); },
    blur: () => fire(on, 'blur'), focus: () => fire(on, 'focus'),
    leave: () => fire(rootOn, 'mouseleave'), enter: () => fire(rootOn, 'mouseenter'),
  };
}

test('the clock runs at full speed while the viewer is here', () => {
  const { R, advance, input } = load();
  const t0 = R.now();
  advance(3); input(); advance(3);
  assert.equal(R.speed(), 1);
  assert.ok(Math.abs(R.now() - t0 - 6) < 1e-6, 'six seconds pass in six seconds');
  assert.equal(R.still(), false);
});

test('stepping away eases to a standstill over two seconds, then the clock stops', () => {
  const { R, advance } = load();
  advance(1);
  const t0 = R.now();
  R.simulate(true);
  advance(1);
  const mid = R.speed();
  assert.ok(mid > .3 && mid < .7, 'half way down, got ' + mid);
  assert.equal(R.still(), false, 'still easing');
  advance(1.1);
  assert.equal(R.speed(), 0);
  assert.equal(R.still(), true);
  const gained = R.now() - t0;
  assert.ok(Math.abs(gained - 1) < .02, 'the ease covers half its length in clock time, got ' + gained);
  const stopped = R.now();
  advance(5);
  assert.equal(R.now(), stopped, 'nothing moves at rest');
});

test('coming back eases up over a second, from exactly where it stopped, and wakes the loops', () => {
  const { R, advance } = load();
  let woken = 0;
  R.onWake(() => woken++);
  R.simulate(true); advance(3);
  const stopped = R.now();
  R.simulate(false);
  assert.equal(woken, 1);
  assert.ok(R.now() - stopped < 1e-6, 'no jump when it wakes');
  advance(.5);
  const mid = R.speed();
  assert.ok(mid > .3 && mid < .7, 'half way up, got ' + mid);
  advance(.6);
  assert.equal(R.speed(), 1);
  assert.equal(R.still(), false);
});

test('eight seconds without input counts as stepping away; any input wakes it', () => {
  const { R, advance, input } = load();
  advance(7.9);
  assert.equal(R.speed(), 1, 'not yet');
  advance(2.2);
  assert.equal(R.still(), true, 'rests after eight quiet seconds and the ease down');
  input('keydown');
  advance(1.1);
  assert.equal(R.speed(), 1);
});

test('a hidden tab rests at once; a blurred window or a mouse that leaves eases down', () => {
  const a = load();
  a.hide(true);
  assert.equal(a.R.still(), true, 'nothing is drawn in a hidden tab, so there is nothing to ease');
  a.hide(false); a.advance(1.1);
  assert.equal(a.R.speed(), 1);

  const b = load();
  b.blur(); b.advance(.5);
  assert.ok(b.R.speed() < 1 && b.R.speed() > 0, 'a blurred window eases down');
  b.focus(); b.advance(1.1);
  assert.equal(b.R.speed(), 1);

  const c = load();
  c.leave(); c.advance(2.1);
  assert.equal(c.R.still(), true, 'a mouse that left the window rests it');
  c.enter(); c.advance(1.1);
  assert.equal(c.R.speed(), 1);
});

test('a waker that throws does not stop the others', () => {
  const { R } = load();
  let ran = 0;
  const err = console.error; console.error = () => {};
  try {
    R.onWake(() => { throw new Error('boom'); });
    R.onWake(() => ran++);
    R.simulate(true); R.simulate(false);
  } finally { console.error = err; }
  assert.equal(ran, 1);
});
