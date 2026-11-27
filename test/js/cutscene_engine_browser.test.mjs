// cutscene_engine_browser.test.mjs — a Cut scene played for real in Chromium:
// it takes the whole window as a dialog, keeps the page out of reach, shows
// its words as text and never as markup, lets the player out after two
// seconds and gives the page back as it was. At phone width and with reduced
// motion too, and without a console error.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const ENGINE = readFileSync(join(root, 'static/js/cutscene_engine.js'));

const PAGE = `<!doctype html><html><head><meta charset="utf-8"></head><body>
<main id="main"><button id="opener">Watch</button><p>The page underneath.</p></main>
<script src="/engine.js"></script></body></html>`;

const SCENE = (look, eye) => ({
  id: 's1', name: 'The Eye Opens',
  background: { kind: 'fall-asleep', seconds: 1, params: { look } },
  overlays: [{ kind: 'eye', at: 0.5, seconds: 0.5, params: { look: eye } }, { kind: 'embers', at: 0.5, seconds: 0.5 }, { kind: 'dragon', at: 0 }],
  text: [{ say: '<img src=x onerror="window.pwned=1">', at: 0.6, seconds: 0, style: 'whisper' }, { say: 'Below.', at: 0.6, seconds: 0, style: 'title' }],
  ending: { kind: 'tap' }
});

async function open(t, viewport, reducedMotion) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport, reducedMotion: reducedMotion ? 'reduce' : 'no-preference' });
  const pg = await ctx.newPage();
  const errors = [];
  pg.on('pageerror', (e) => errors.push(String(e)));
  pg.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/engine.js') return route.fulfill({ contentType: 'text/javascript', body: ENGINE });
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await pg.goto('http://t.test/');
  return { pg, errors };
}

function start(pg, scene) {
  return pg.evaluate((scene) => {
    document.getElementById('opener').focus();
    window.shut = [];
    window.run = ChronicleCutscene.play(document.body, scene, { onClose: (why) => window.shut.push(why), label: 'Cut scene: test' });
  }, scene);
}

for (const [look, eye] of [['drift', 'gather'], ['eyelids', 'almond'], ['ink', 'rift']]) {
  test(`${look} with the ${eye} eye: a dialog over everything, closed by Esc after two seconds`, { skip }, async (t) => {
    const { pg, errors } = await open(t, { width: 1280, height: 800 });
    await start(pg, SCENE(look, eye));
    const state = await pg.evaluate(() => {
      const d = document.querySelector('.cs-cut');
      const r = d.getBoundingClientRect();
      return { role: d.getAttribute('role'), modal: d.getAttribute('aria-modal'), label: d.getAttribute('aria-label'), focus: document.activeElement === d,
        cover: r.width === innerWidth && r.height === innerHeight, inert: document.getElementById('main').hasAttribute('inert'),
        surface: document.documentElement.hasAttribute('data-full-surface') };
    });
    assert.deepEqual(state, { role: 'dialog', modal: 'true', label: 'Cut scene: test', focus: true, cover: true, inert: true, surface: true });

    // No way out for the first two seconds: Esc does nothing, the close mark is hidden.
    await pg.keyboard.press('Escape');
    assert.equal(await pg.evaluate(() => !!document.querySelector('.cs-cut') && window.shut.length), 0);
    assert.equal(await pg.evaluate(() => getComputedStyle(document.querySelector('.cs-cut-x')).visibility), 'hidden');

    await pg.waitForFunction(() => window.run.time() >= 2.05);
    // Words are text, never markup.
    const words = await pg.evaluate(() => [...document.querySelectorAll('.cs-cut-line')].map((l) => l.textContent));
    assert.deepEqual(words, ['<img src=x onerror="window.pwned=1">', 'Below.']);
    assert.equal(await pg.evaluate(() => document.querySelectorAll('.cs-cut-words img').length + (window.pwned || 0)), 0);
    assert.equal(await pg.evaluate(() => getComputedStyle(document.querySelector('.cs-cut-x')).visibility), 'visible');
    // Everything has arrived, so a tap scene offers its quiet Close.
    assert.equal(await pg.evaluate(() => document.querySelector('.cs-cut-end').classList.contains('cs-cut-on')), true);

    await pg.keyboard.press('Escape');
    assert.deepEqual(await pg.evaluate(() => window.shut), ['escape']);
    await pg.waitForFunction(() => !document.querySelector('.cs-cut'));
    const after = await pg.evaluate(() => ({ focus: document.activeElement.id, inert: document.getElementById('main').hasAttribute('inert'), surface: document.documentElement.hasAttribute('data-full-surface') }));
    assert.deepEqual(after, { focus: 'opener', inert: false, surface: false });
    assert.deepEqual(errors, []);
  });
}

test('at phone width the scene fits, a tap closes it at the end, and stop() ends it quietly', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 390, height: 844 });
  await start(pg, SCENE('ink', 'rift'));
  await pg.waitForFunction(() => window.run.time() >= 2.05);
  const fit = await pg.evaluate(() => {
    const x = document.querySelector('.cs-cut-x').getBoundingClientRect(), w = document.querySelector('.cs-cut-words').getBoundingClientRect();
    return { x: x.right <= innerWidth && x.width >= 44 && x.height >= 44, words: w.left >= 0 && w.right <= innerWidth, scroll: document.documentElement.scrollWidth <= innerWidth };
  });
  assert.deepEqual(fit, { x: true, words: true, scroll: true });
  await pg.mouse.click(195, 300);
  assert.deepEqual(await pg.evaluate(() => window.shut), ['tap']);

  await start(pg, { background: { kind: 'black' }, text: [{ say: 'Wait.', at: 0, seconds: 0 }], ending: { kind: 'tap' } });
  await pg.evaluate(() => window.run.stop());
  assert.deepEqual(await pg.evaluate(() => window.shut), ['stopped']);
  await pg.waitForFunction(() => !document.querySelector('.cs-cut'));
  assert.deepEqual(errors, []);
});

test('reduced motion crossfades: no film grain, no backdrop blur, and it still closes by itself', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 1280, height: 800 }, true);
  await start(pg, { ...SCENE('ink', 'rift'), ending: { kind: 'auto', after: 0.2 } });
  const calm = await pg.evaluate(() => ({ calm: document.querySelector('.cs-cut').classList.contains('cs-cut-calm'), grain: !!document.querySelector('.cs-cut-grain'), veil: !!document.querySelector('.cs-cut-veil') }));
  assert.deepEqual(calm, { calm: true, grain: false, veil: false });
  await pg.waitForFunction(() => window.shut.length === 1, null, { timeout: 6000 });
  assert.deepEqual(await pg.evaluate(() => window.shut), ['auto']);
  assert.deepEqual(errors, []);
});

// The fire blooms round the eye once it opens, rising above it, in every
// look; with reduced motion it is there but holds still.
for (const eye of ['rift', 'almond', 'gather']) {
  test(`the ${eye} eye opens into fire that burns above it, and holds still for reduced motion`, { skip }, async (t) => {
    for (const rm of [false, true]) {
      const { pg, errors } = await open(t, { width: 1280, height: 800 }, rm);
      await start(pg, { id: 'f', name: 'Fire', background: { kind: 'black' }, overlays: [{ kind: 'eye', at: 0, seconds: 0.8, params: { look: eye } }], ending: { kind: 'tap' } });
      await pg.waitForFunction(() => window.run.time() >= 1.6);
      // The eye sits at 47% of the height with no words; sample a band of
      // the canvas above it for bright green, twice, a moment apart.
      const band = () => pg.evaluate(() => {
        const c = document.querySelector('.cs-cut-canvas'), g = c.getContext('2d');
        const y = Math.round(c.height * 0.47 - c.width * 0.07 - c.height * 0.12), d = g.getImageData(Math.round(c.width * 0.35), y, Math.round(c.width * 0.3), 40).data;
        let lit = 0, sum = 0;
        for (let i = 0; i < d.length; i += 4) { sum += d[i + 1]; if (d[i + 1] > 120 && d[i + 1] > d[i] + 30) lit++; }
        return { lit, sum };
      });
      const a = await band();
      await pg.waitForTimeout(400);
      const b = await band();
      assert.ok(a.lit > 50, `${eye}${rm ? ' (reduced)' : ''}: fire above the eye, ${a.lit} lit pixels`);
      if (rm) assert.equal(a.sum, b.sum, `${eye}: still under reduced motion`);
      else assert.notEqual(a.sum, b.sum, `${eye}: the fire moves`);
      assert.deepEqual(errors, []);
    }
  });
}

// A dragon slit is its own drawing, not the round pupil renamed: the same
// eye, held still, paints its middle differently with each pupil.
test('a slit pupil draws a different eye from a round one', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 1280, height: 800 }, true);
  const middle = async (pupil) => {
    await start(pg, { id: 'p', name: 'Pupil', background: { kind: 'black' }, overlays: [{ kind: 'eye', at: 0, seconds: 0.5, params: { look: 'rift', pupil } }], ending: { kind: 'tap' } });
    await pg.waitForFunction(() => window.run.time() >= 1);
    const px = await pg.evaluate(() => {
      const c = document.querySelector('.cs-cut-canvas'), g = c.getContext('2d');
      const w = Math.round(c.width * 0.06), x = Math.round(c.width / 2 - w / 2), y = Math.round(c.height * 0.47 - w / 2);
      return Array.from(g.getImageData(x, y, w, w).data);
    });
    await pg.evaluate(() => window.run.stop());
    await pg.waitForFunction(() => !document.querySelector('.cs-cut'));
    return px;
  };
  const round = await middle('round'), slit = await middle('slit');
  let differ = 0;
  for (let i = 0; i < round.length; i += 4) if (Math.abs(round[i + 1] - slit[i + 1]) > 40) differ++;
  assert.ok(differ > round.length / 4 * 0.1, `${differ} pixels differ between the pupils`);
  // Each pupil holds still under reduced motion, so the comparison is fair.
  assert.deepEqual(await middle('round'), round);
  assert.deepEqual(errors, []);
});

// A title card and a place card built from separate layers: film bars on
// top, a caption and a title that settle in the middle, then a corner card
// that slides in at the bottom left, above the bars, and slides away.
const CARDS = {
  id: 'c1', name: 'Chapter two',
  background: { kind: 'none', seconds: 1 },
  overlays: [{ kind: 'letterbox', at: 0, seconds: 0.6 }],
  text: [
    { say: 'Chapter two', at: 0.2, seconds: 1.4, style: 'caption', in: 'rise' },
    { say: 'The Drowned Bell', at: 0.3, seconds: 1.3, style: 'title', in: 'settle', out: 'fade' },
    { say: 'Saltreach', at: 1.8, seconds: 0, style: 'title', layout: 'corner', in: 'slide' },
    { say: 'Three days later', at: 1.9, seconds: 0.8, style: 'whisper', layout: 'corner', in: 'slide', out: 'slide' }
  ],
  ending: { kind: 'tap' }
};

for (const [label, viewport] of [['desktop', { width: 1280, height: 720 }], ['phone', { width: 390, height: 844 }]]) {
  test(`a title card and a place card from separate layers, on ${label}`, { skip }, async (t) => {
    const { pg, errors } = await open(t, viewport);
    await start(pg, CARDS);
    await pg.waitForFunction(() => document.querySelectorAll('.cs-cut-words:not(.cs-cut-corner) .cs-cut-line').length === 2);
    const middle = await pg.evaluate(() => [...document.querySelectorAll('.cs-cut-words:not(.cs-cut-corner) .cs-cut-line')].map((p) => ({ text: p.textContent, cls: p.className })));
    assert.deepEqual(middle, [
      { text: 'Chapter two', cls: 'cs-cut-line cs-cut-caption cs-cut-in-rise' },
      { text: 'The Drowned Bell', cls: 'cs-cut-line cs-cut-title cs-cut-in-settle' }
    ]);
    // Film bars cover the top and bottom of the canvas once built.
    await pg.waitForTimeout(800);
    const bars = await pg.evaluate(() => {
      const c = document.querySelector('.cs-cut-canvas'), g = c.getContext('2d');
      const at = (y) => Array.from(g.getImageData(Math.floor(c.width / 2), y, 1, 1).data);
      return { top: at(2), bottom: at(c.height - 3), mid: at(Math.floor(c.height / 2)) };
    });
    assert.deepEqual(bars.top, [0, 0, 0, 255]);
    assert.deepEqual(bars.bottom, [0, 0, 0, 255]);
    assert.equal(bars.mid[3], 0, 'the middle shows what is under the scene');

    await pg.waitForFunction(() => document.querySelectorAll('.cs-cut-corner .cs-cut-line').length === 2);
    const corner = await pg.evaluate(() => {
      const box = document.querySelector('.cs-cut-corner').getBoundingClientRect();
      const lines = [...document.querySelectorAll('.cs-cut-corner .cs-cut-line')];
      return {
        lines: lines.map((p) => ({ text: p.textContent, cls: p.className, words: p.querySelectorAll('.cs-cut-w').length })),
        left: box.left < innerWidth * 0.12, aboveBars: box.bottom <= innerHeight * 0.88 + 1, clear: box.right <= innerWidth,
        aligned: getComputedStyle(lines[0]).textAlign
      };
    });
    assert.deepEqual(corner.lines, [
      { text: 'Saltreach', cls: 'cs-cut-line cs-cut-title cs-cut-in-slide', words: 0 },
      { text: 'Three days later', cls: 'cs-cut-line cs-cut-whisper cs-cut-in-slide', words: 0 }
    ], 'a whisper with its own arrival chosen is one line, not word by word');
    assert.ok(corner.left && corner.aboveBars && corner.clear, JSON.stringify(corner));
    assert.equal(corner.aligned, 'left');
    // Each line leaves its chosen way, and a line with none leaves the old way.
    await pg.waitForFunction(() => document.querySelector('.cs-cut-out-slide'));
    assert.ok(await pg.evaluate(() => !!document.querySelector('.cs-cut-corner .cs-cut-gone.cs-cut-out-slide')));
    assert.deepEqual(errors, []);
  });
}

// A settling title draws its letters together without changing its layout:
// the line keeps its wrapping, and the line above it never moves.
for (const [label, viewport] of [['desktop', { width: 1280, height: 720 }], ['phone', { width: 390, height: 844 }]]) {
test(`a long title that settles keeps its lines and its neighbours still while it draws together, on ${label}`, { skip }, async (t) => {
  const { pg, errors } = await open(t, viewport);
  await start(pg, { ...CARDS, text: CARDS.text.slice(0, 2).map((l) => ({ ...l, at: 0.1, seconds: 0 })) });
  await pg.waitForFunction(() => document.querySelector('.cs-cut-in-settle'));
  const samples = [];
  for (let i = 0; i < 12; i++) {
    samples.push(await pg.evaluate(() => {
      const [cap, title] = document.querySelectorAll('.cs-cut-words:not(.cs-cut-corner) .cs-cut-line');
      // Layout positions, which a rise or settle transform does not change.
      return { cap: cap.offsetTop + cap.offsetParent.offsetTop, h: title.offsetHeight };
    }));
    await pg.waitForTimeout(150);
  }
  assert.equal(new Set(samples.map((s) => s.cap)).size, 1, 'the caption moved: ' + JSON.stringify(samples));
  assert.equal(new Set(samples.map((s) => s.h)).size, 1, 'the title rewrapped: ' + JSON.stringify(samples));
  assert.deepEqual(errors, []);
});
}

// A box centres its lines as a group, so a line joining or leaving moves the
// others: they glide there, never jump in one frame. The glide's first
// frame moves about 8px; the old jump was 36px, so 14px leaves room for a
// slow runner without letting a jump through.
test('lines in a box glide, never jump, when another line joins or leaves', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 1000, height: 640 });
  await pg.evaluate(() => {
    window.track = [];
    const tick = () => {
      const ls = [...document.querySelectorAll('.cs-cut-words:not(.cs-cut-corner) .cs-cut-line')];
      window.track.push(ls.map((l) => ({ t: l.textContent, y: l.getBoundingClientRect().top })));
      if (window.track.length < 400) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
  await start(pg, { ...CARDS, text: [
    { say: 'Chapter two', at: 0.1, seconds: 0.9, style: 'caption', in: 'fade', out: 'fade' },
    { say: 'The Drowned Bell', at: 0.6, seconds: 0, style: 'title', in: 'fade' }
  ] });
  await pg.waitForFunction(() => window.track.length >= 400 || (window.run.time() > 3.4));
  const steps = { 'Chapter two': 0, 'The Drowned Bell': 0 };
  const track = await pg.evaluate(() => window.track);
  for (let i = 1; i < track.length; i++) {
    for (const l of track[i]) {
      const prev = track[i - 1].find((p) => p.t === l.t);
      if (prev) steps[l.t] = Math.max(steps[l.t], Math.abs(l.y - prev.y));
    }
  }
  assert.ok(track.some((f) => f.length === 2) && track.at(-1).length === 1, 'both lines shown, then the caption gone');
  assert.ok(steps['Chapter two'] < 14, 'the caption jumped ' + steps['Chapter two'] + 'px in one frame');
  assert.ok(steps['The Drowned Bell'] < 14, 'the title jumped ' + steps['The Drowned Bell'] + 'px in one frame');
  assert.deepEqual(errors, []);
});

test('under reduced motion the chosen arrivals are plain fades and the bars are simply there', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 1280, height: 720 }, true);
  await start(pg, CARDS);
  await pg.waitForFunction(() => document.querySelectorAll('.cs-cut-line').length >= 2);
  const names = await pg.evaluate(() => [...document.querySelectorAll('.cs-cut-line')].map((p) => getComputedStyle(p).animationName));
  for (const n of names) assert.equal(n, 'cs-cut-fade');
  assert.deepEqual(errors, []);
});
