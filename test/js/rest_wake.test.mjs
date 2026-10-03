// rest_wake.test.mjs — Chronicle.restWake eases a looping effect to a stop
// when nobody is using the page and back up on input. These tests drive the
// real static/js/rest_wake.js against a fake clock, event registry and
// requestAnimationFrame, so the timings the owner signed (down over ~2 s, up
// over ~1 s, rest after 8 s without input, nothing drawn while resting, never
// moving under reduced motion) are pinned without a browser.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'rest_wake.js'), 'utf8');

// A world with a controllable clock. step() advances time one frame at a time,
// firing due timers and queued animation frames in order, like a browser.
function world({ hidden = false, frameSkew = 0 } = {}) {
  let now = 0;
  const timers = new Map();
  let nextTimer = 1;
  let frames = [];
  let nextFrame = 1;
  const listeners = { window: {}, document: {}, root: {} };
  const reg = (bucket) => (type, fn) => { (bucket[type] = bucket[type] || new Set()).add(fn); };
  const unreg = (bucket) => (type, fn) => { if (bucket[type]) bucket[type].delete(fn); };
  const root = { addEventListener: reg(listeners.root), removeEventListener: unreg(listeners.root) };
  const document = {
    hidden,
    documentElement: root,
    addEventListener: reg(listeners.document),
    removeEventListener: unreg(listeners.document),
  };
  const win = {
    Chronicle: undefined,
    performance: { now: () => now },
    addEventListener: reg(listeners.window),
    removeEventListener: unreg(listeners.window),
    setTimeout: (fn, ms) => { const id = nextTimer++; timers.set(id, { at: now + ms, fn }); return id; },
    clearTimeout: (id) => { timers.delete(id); },
    requestAnimationFrame: (fn) => { const id = nextFrame++; frames.push({ id, fn }); return id; },
    cancelAnimationFrame: (id) => { frames = frames.filter((f) => f.id !== id); },
  };
  win.window = win;
  const ctx = vm.createContext({ window: win, document, Date, Math });
  vm.runInContext(src, ctx);

  function step(ms = 16) {
    now += ms;
    for (const [id, t] of [...timers]) if (t.at <= now) { timers.delete(id); t.fn(); }
    const due = frames; frames = [];
    // A real frame's timestamp is its start, which may precede the clock.
    for (const f of due) f.fn(now + frameSkew);
  }
  function advance(ms) { for (let t = 0; t < ms; t += 16) step(16); }
  function fire(bucket, type) { for (const fn of [...(listeners[bucket][type] || [])]) fn({ type }); }
  const count = (bucket) => Object.values(listeners[bucket]).reduce((n, s) => n + s.size, 0);
  return { win, document, advance, fire, count, pendingFrames: () => frames.length, now: () => now, setHidden: (v) => { document.hidden = v; } };
}

function make(w, extra = {}) {
  const log = { levels: [], states: [] };
  const rw = w.win.Chronicle.restWake.create({
    onLevel: (l) => log.levels.push(l),
    onState: (s) => log.states.push(s),
    ...extra,
  });
  return { rw, log };
}

test('it starts awake and eases up to full speed in about a second', () => {
  const w = world();
  const { rw, log } = make(w);
  assert.deepEqual(log.states, ['active']);
  w.advance(500);
  assert.ok(rw.level() > 0.3 && rw.level() < 0.7, `half-way after 0.5 s, got ${rw.level()}`);
  w.advance(600);
  assert.equal(rw.level(), 1);
  assert.equal(w.pendingFrames(), 0, 'no frames once it is at full speed');
});

test('eight seconds without input eases it to a stop over about two seconds, then draws nothing', () => {
  const w = world();
  const { rw, log } = make(w);
  w.advance(1100);
  assert.equal(rw.level(), 1);
  w.advance(6800); // 7.9 s since start: still awake
  assert.equal(rw.level(), 1);
  w.advance(400); // past 8 s: easing down begins
  assert.ok(rw.level() < 1, 'easing down after the quiet period');
  w.advance(1000);
  assert.ok(rw.level() > 0, 'still easing one second in, not a sudden stop');
  w.advance(1200);
  assert.equal(rw.level(), 0);
  assert.equal(log.states.at(-1), 'resting');
  assert.ok(rw.isResting());
  assert.equal(w.pendingFrames(), 0, 'no frame is scheduled while resting');
  const seen = log.levels.length;
  w.advance(5000);
  assert.equal(log.levels.length, seen, 'nothing is drawn while resting');
});

test('input wakes it at once and it is back at full speed within about a second', () => {
  const w = world();
  const { rw, log } = make(w);
  w.advance(12000);
  assert.ok(rw.isResting());
  w.fire('window', 'pointermove');
  assert.equal(log.states.at(-1), 'active', 'the effect may start drawing immediately');
  w.advance(500);
  assert.ok(rw.level() > 0.3 && rw.level() < 1);
  w.advance(600);
  assert.equal(rw.level(), 1);
});

test('input keeps it awake: the quiet period counts from the last input', () => {
  const w = world();
  const { rw } = make(w);
  w.advance(7000);
  w.fire('window', 'keydown');
  w.advance(7000); // 14 s on the clock, 7 s since input
  assert.equal(rw.level(), 1);
  w.advance(1500); // past 8 s since input
  assert.ok(rw.level() < 1);
});

for (const [name, trigger] of [
  ['window blur', (w) => w.fire('window', 'blur')],
  ['the tab being hidden', (w) => { w.setHidden(true); w.fire('document', 'visibilitychange'); }],
  ['the pointer leaving the window', (w) => w.fire('root', 'mouseleave')],
]) {
  test(`${name} eases it to a stop without waiting for the quiet period`, () => {
    const w = world();
    const { rw, log } = make(w);
    w.advance(1200);
    assert.equal(rw.level(), 1);
    trigger(w);
    w.advance(2200);
    assert.equal(rw.level(), 0);
    assert.equal(log.states.at(-1), 'resting');
  });
}

test('a tab that becomes visible again stays at rest until there is input', () => {
  const w = world();
  const { rw } = make(w);
  w.advance(1200);
  w.setHidden(true); w.fire('document', 'visibilitychange');
  w.advance(2200);
  w.setHidden(false); w.fire('document', 'visibilitychange');
  w.advance(3000);
  assert.equal(rw.level(), 0);
  w.fire('window', 'pointerdown');
  w.advance(1100);
  assert.equal(rw.level(), 1);
});

test('a page opened in a hidden tab starts at rest', () => {
  const w = world({ hidden: true });
  const { rw, log } = make(w);
  w.advance(2000);
  assert.equal(rw.level(), 0);
  assert.deepEqual(log.states, []);
});

test('under reduced motion it stays inert: no listeners, no frames, level 0', () => {
  for (const reduced of [true, () => true]) {
    const w = world();
    const { rw, log } = make(w, { reduced });
    assert.equal(w.count('window') + w.count('document') + w.count('root'), 0, 'no listeners');
    w.fire('window', 'pointermove');
    rw.wake();
    w.advance(3000);
    assert.equal(rw.level(), 0);
    assert.equal(w.pendingFrames(), 0);
    assert.deepEqual(log.states, []);
    assert.deepEqual(log.levels, []);
    rw.destroy(); // safe to call
  }
});

test('suppress ignores wake-ups for a moment, then input works again', () => {
  const w = world();
  const { rw } = make(w);
  w.advance(1200);
  rw.suppress(1500);
  rw.rest();
  w.fire('window', 'pointermove');
  w.advance(2200);
  assert.equal(rw.level(), 0, 'the click that asked to rest must not wake it');
  w.advance(1000);
  w.fire('window', 'pointermove');
  w.advance(1100);
  assert.equal(rw.level(), 1);
});

test('destroy removes every listener and cancels pending work', () => {
  const w = world();
  const { rw, log } = make(w);
  assert.ok(w.count('window') > 0);
  rw.destroy();
  assert.equal(w.count('window') + w.count('document') + w.count('root'), 0);
  const seen = log.levels.length;
  w.advance(20000);
  assert.equal(log.levels.length, seen);
  w.fire('window', 'pointermove');
  w.advance(2000);
  assert.equal(log.levels.length, seen);
});

test('a frame stamped before the clock read that scheduled it does not undo a wake', () => {
  const w = world({ frameSkew: -20 });
  const { rw, log } = make(w);
  w.advance(12000);
  assert.ok(rw.isResting());
  w.fire('window', 'pointermove');
  w.advance(1200);
  assert.equal(rw.level(), 1);
  assert.equal(log.states.at(-1), 'active', 'the effect must not be paused while the level rises');
});
