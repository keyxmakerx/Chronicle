// deck_cards.test.mjs: the card's motion script (internal/plugins/decks/static/
// js/deck_cards.js) must name exactly the turn-overs and moves the server
// accepts, play the cheaper twin when it should, hold a turn at its halfway
// point for no more than 400ms, turn one card at a time, and end every move on
// the page's own motion level (Calm fades, Off shows the end at once). The
// pieces are pure or take their hooks, so they run here without a browser; the
// browser check lives in test/live/deck_card_looks.spec.mjs.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const decks = join(root, 'internal', 'plugins', 'decks');

function load(env = {}) {
  const sandbox = { setTimeout, clearTimeout, Promise, console, ...env };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(join(decks, 'static', 'js', 'deck_cards.js'), 'utf8'), sandbox);
  return sandbox.DeckCards;
}
const DC = load();

function goList(file, name) {
  const src = readFileSync(join(decks, file), 'utf8');
  const m = src.match(new RegExp(name + '\\s*=\\s*(?:\\[\\]string)?\\{([^}]*)\\}'));
  assert.ok(m, `no ${name} in ${file}`);
  return [...m[1].matchAll(/"([^"]*)"/g)].map((x) => x[1]);
}

test('the turn-overs match the list the server accepts', () => {
  assert.deepEqual([...DC.TURNS], goList('settings.go', 'turnValues'));
});

test('the fixed moves match the server list', () => {
  const src = readFileSync(join(decks, 'looks_page.go'), 'utf8');
  const m = src.match(/func moveNames\(\) \[\]string \{\s*return \[\]string\{([^}]*)\}/);
  assert.ok(m, 'no moveNames in looks_page.go');
  assert.deepEqual([...DC.MOVES], [...m[1].matchAll(/"([^"]*)"/g)].map((x) => x[1]));
});

test('the page level is Calm or Off from the page, and Off for a device that asks for less', () => {
  assert.equal(DC.levelOf(null, false), 'full');
  assert.equal(DC.levelOf('calm', false), 'calm');
  assert.equal(DC.levelOf('off', false), 'off');
  assert.equal(DC.levelOf('calm', true), 'off');
  assert.equal(DC.levelOf('junk', false), 'full');
});

test('burn plays as sweep and ink as a fade on a weak device, a crowded screen and the small shape', () => {
  const full = { level: 'full', shape: 'tall', weak: false, crowded: false };
  assert.equal(DC.planTurn('burn', full).name, 'burn');
  assert.equal(DC.planTurn('ink', full).name, 'ink');
  for (const c of [{ weak: true }, { crowded: true }, { shape: 'small' }]) {
    assert.equal(DC.planTurn('burn', { ...full, ...c }).name, 'sweep', JSON.stringify(c));
    assert.equal(DC.planTurn('ink', { ...full, ...c }).name, 'fade', JSON.stringify(c));
    assert.equal(DC.planTurn('flip', { ...full, ...c }).name, 'flip');
    assert.equal(DC.planTurn('forge', { ...full, ...c }).name, 'forge');
  }
});

test('the weak-device test is the calendar one', () => {
  assert.equal(DC.deviceIsLimited({ hardwareConcurrency: 4 }), true);
  assert.equal(DC.deviceIsLimited({ deviceMemory: 2 }), true);
  assert.equal(DC.deviceIsLimited({ hardwareConcurrency: 8, deviceMemory: 8 }), false);
  assert.equal(DC.deviceIsLimited({}), false);
});

test('foil shows as sheen on the small shape only', () => {
  assert.equal(DC.finishFor('foil', 'small'), 'sheen');
  assert.equal(DC.finishFor('foil', 'tall'), 'foil');
  assert.equal(DC.finishFor('sheen', 'small'), 'sheen');
});

test('Calm fades and Off shows the end, whatever the turn-over', () => {
  for (const t of DC.TURNS) {
    assert.equal(DC.planTurn(t, { level: 'calm', shape: 'tall' }).kind, 'calm');
    assert.equal(DC.planTurn(t, { level: 'off', shape: 'tall' }).kind, 'off');
  }
});

test('a deal moves seven cards, sixty milliseconds apart', () => {
  const p = DC.dealPlan(10);
  assert.equal(p.filter((x) => x.moves).length, 7);
  assert.deepEqual(Array.from(p.slice(0, 7), (x) => x.delay), [0, 60, 120, 180, 240, 300, 360]);
  assert.ok(p.slice(7).every((x) => !x.moves));
});

test('turns queue: one at a time, in the order asked', async () => {
  const q = DC.createQueue();
  const log = [];
  let running = 0, most = 0;
  const job = (name, ms) => () => new Promise((res) => {
    running++; most = Math.max(most, running); log.push('start ' + name);
    setTimeout(() => { running--; log.push('end ' + name); res(); }, ms);
  });
  await Promise.all([q.push(job('a', 15)), q.push(job('b', 1)), q.push(job('c', 1))]);
  assert.equal(most, 1);
  assert.deepEqual(log, ['start a', 'end a', 'start b', 'end b', 'start c', 'end c']);
});

test('a failed turn does not stall the queue', async () => {
  const q = DC.createQueue();
  await q.push(() => Promise.reject(new Error('x'))).catch(() => {});
  assert.equal(await q.push(() => Promise.resolve(7)), 7);
});

// A harness for the halfway wait: sleeps and loads are settled by hand.
function harness({ loadAfter, rejectAt, wait = 400 }) {
  const log = [];
  const timers = [];
  let loadCalled = false;
  const h = {
    wait,
    load: () => { loadCalled = true; log.push('load'); return new Promise((res, rej) => timers.push({ at: rejectAt ?? loadAfter, fn: () => (rejectAt ? rej(new Error('no')) : res('FACE')) })); },
    firstHalf: () => { log.push('first'); return Promise.resolve(); },
    sleep: (ms) => new Promise((res) => timers.push({ at: ms, fn: res })),
    secondHalf: (face, late) => { log.push('second:' + face + ':' + late); return Promise.resolve(); },
    fill: (face) => { log.push('fill:' + face); },
    fail: () => { log.push('fail'); },
  };
  return { h, log, timers, loadCalled: () => loadCalled };
}
async function flush(timers, upTo) {
  // settle timers in time order up to a time
  for (let i = 0; i < 8; i++) {
    await Promise.resolve();
    timers.filter((t) => t.at <= upTo && !t.done).sort((a, b) => a.at - b.at).forEach((t) => { t.done = true; t.fn(); });
  }
}

test('the face is asked for when the turn starts, not before', async () => {
  const x = harness({ loadAfter: 10 });
  assert.equal(x.loadCalled(), false);
  const p = DC.sequence(x.h);
  await Promise.resolve();
  assert.equal(x.log[0], 'load');
  await flush(x.timers, 1000); await p;
});

test('a face that arrives in time is shown at the halfway point', async () => {
  const x = harness({ loadAfter: 100 });
  const p = DC.sequence(x.h);
  await flush(x.timers, 150); await p;
  assert.deepEqual(x.log, ['load', 'first', 'second:FACE:false']);
});

test('a face that is late is not waited for past 400ms; it fills in after', async () => {
  const x = harness({ loadAfter: 900 });
  const p = DC.sequence(x.h);
  await flush(x.timers, 450);
  await Promise.resolve();
  assert.ok(x.log.includes('second:null:true'), x.log.join(','));
  assert.ok(!x.log.includes('fill:FACE'));
  await flush(x.timers, 1000); await p;
  assert.deepEqual(x.log, ['load', 'first', 'second:null:true', 'fill:FACE']);
});

test('the wait is the 400ms the page names', () => {
  assert.equal(DC.HALFWAY_WAIT, 400);
});

test('a face that cannot be loaded ends the turn without a half-turned card', async () => {
  const x = harness({ loadAfter: Infinity, rejectAt: 5 });
  const p = DC.sequence(x.h);
  await flush(x.timers, 10); await p;
  assert.ok(x.log.includes('fail'));
  assert.ok(!x.log.some((l) => l.startsWith('second')));
});

// End states for the fixed moves, with a stand-in card.
function card() {
  const classes = new Set();
  return {
    hidden: false, classes,
    classList: { add: (...c) => c.forEach((x) => classes.add(x)), remove: (...c) => c.forEach((x) => classes.delete(x)), contains: (c) => classes.has(c) },
    querySelector: () => null,
    appendChild() {}, style: { setProperty() {} },
    animate() { return { finished: Promise.resolve(), finish() {}, cancel() {} }; },
  };
}
function page(level) {
  return load({ document: { documentElement: { getAttribute: (a) => (a === 'data-motion' ? level : null) }, querySelectorAll: () => [], createElement: () => ({ style: {}, appendChild() {}, setAttribute() {} }) } });
}

test('Off: used and gone reach their end at once, nothing travels', async () => {
  const d = page('off');
  const a = card(); await d.move(a, 'used', {});
  assert.ok(a.classes.has('dk-spent'));
  const b = card(); await d.move(b, 'gone', {});
  assert.equal(b.hidden, true);
  const c = card(); let called = 0; c.animate = () => { called++; return { finished: Promise.resolve(), finish() {}, cancel() {} }; };
  await d.move(c, 'draw', {}); await d.move(c, 'play', {});
  assert.equal(called, 0);
});

test('Calm: used and gone end the same way after a fade, with no travel', async () => {
  const d = page('calm');
  const frames = [];
  const mk = () => { const a = card(); a.animate = (f) => { frames.push(f); return { finished: Promise.resolve(), finish() {}, cancel() {} }; }; return a; };
  const a = mk(); await d.move(a, 'used', {});
  assert.ok(a.classes.has('dk-spent'));
  const b = mk(); await d.move(b, 'gone', {});
  assert.equal(b.hidden, true);
  const c = mk(); await d.move(c, 'draw', { dx: 90, dy: 40 });
  assert.ok(frames.every((f) => f.every((k) => !('transform' in k))), 'Calm animated a transform');
});

test('an unknown move does nothing', async () => {
  const d = page(null);
  const a = card(); await d.move(a, 'nope', {});
  assert.equal(a.hidden, false);
});

// ---- Crowding, the stage, the timing table and stored values ----

const box = (l, t, w = 100, h = 100) => ({ getBoundingClientRect: () => ({ left: l, top: t, right: l + w, bottom: t + h, width: w, height: h }) });
const staged = { closest: (q) => (q === '[data-dk-stage]' ? {} : null), ...box(0, 0) };
const plain = { closest: () => null, ...box(0, 0) };

test('only cards in view count toward the crowd', () => {
  const inView = Array.from({ length: 10 }, (_, i) => box(i * 10, 0));
  const offscreen = Array.from({ length: 40 }, (_, i) => box(i * 10, 5000));
  assert.equal(DC.crowdedFor(plain, [...inView, ...offscreen], 1280, 900), false);
  assert.equal(DC.crowdedFor(plain, [...inView, ...Array.from({ length: 5 }, (_, i) => box(i * 10, 200))], 1280, 900), true);
  assert.equal(DC.crowdedFor(plain, offscreen, 1280, 900), false);
});

test('a staged card ignores the crowd rule and plays burn with 40 cards on screen', () => {
  const forty = Array.from({ length: 40 }, (_, i) => box(i * 10, 0));
  assert.equal(DC.crowdedFor(plain, forty, 1280, 900), true);
  const crowded = DC.crowdedFor(staged, forty, 1280, 900);
  assert.equal(crowded, false);
  const ctx = { level: 'full', shape: 'tall', weak: false, crowded, staged: true };
  assert.equal(DC.planTurn('burn', ctx).name, 'burn');
  assert.equal(DC.planTurn('ink', ctx).name, 'ink');
  // the weak-device and small-shape twins still apply on a stage
  assert.equal(DC.planTurn('burn', { ...ctx, weak: true }).name, 'sweep');
  assert.equal(DC.planTurn('ink', { ...ctx, shape: 'small' }).name, 'fade');
  // and the flag alone is enough even if a caller passes the raw crowd
  assert.equal(DC.planTurn('burn', { ...ctx, crowded: true }).name, 'burn');
});

test('every turn-over fits 900ms at normal Motion speed, from the script table', () => {
  const sum = (a) => a.reduce((x, y) => x + y, 0);
  for (const t of [...DC.TURNS, 'fade']) {
    const ph = DC.PH[t];
    assert.ok(ph, `no timing for ${t}`);
    const total = sum(ph.a) + sum(ph.b);
    assert.ok(total <= 900, `${t} takes ${total}ms`);
  }
});

test('the fixed moves keep their limits: under 500 to say where, up to 1000 for gone', () => {
  const sum = (a) => a.reduce((x, y) => x + y, 0);
  for (const m of ['draw', 'deal', 'shuffle', 'play', 'used']) assert.ok(sum(DC.MOVE_PHASES[m]) < 500, `${m} takes ${sum(DC.MOVE_PHASES[m])}ms`);
  assert.ok(sum(DC.MOVE_PHASES.gone) <= 1000);
  assert.deepEqual(Array.from(DC.MOVE_PHASES.ready), [1000, 1000]);
});

test('a stored shape or turn outside the lists reads as the default, as on the server', () => {
  assert.deepEqual(Array.from(DC.SHAPES), goList('settings.go', 'shapeValues'));
  assert.equal(DC.shapeOf('wide'), 'tall');
  assert.equal(DC.shapeOf('small'), 'small');
  assert.equal(DC.turnOf('spin'), 'flip');
  assert.equal(DC.turnOf('forge'), 'forge');
  assert.equal(DC.turnOf(null), 'flip');
});
