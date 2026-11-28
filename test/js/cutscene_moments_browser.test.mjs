// cutscene_moments_browser.test.mjs — a cut scene's moments, played for real
// in Chromium: the scene clock stops at the beat and the question stands over
// the dimmed scene, as text and never markup. With onAnswer a viewer writes or
// leaves it to the Director and sees it sent; without it the ways are only
// drawn. Only go() (or the preview's own Continue) lets it run on, showing the
// lines that joined the story first; Esc still gets the player out. hold()
// and released land a late screen on a moment. Calm fades the question in,
// Off just shows it, and a phone stacks the ways.
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

const PAGE = (motion) => `<!doctype html><html${motion ? ` data-motion="${motion}"` : ''}><head><meta charset="utf-8"></head><body>
<main id="main"><button id="opener">Watch</button></main>
<script src="/engine.js"></script></body></html>`;

const ASK = '<b>What</b> do the fishermen whisper about the bell?';
const SCENE = (moments) => ({
  id: 's1', name: 'The Drowned Bell',
  background: { kind: 'black' },
  overlays: [{ kind: 'embers', at: 0, seconds: 0.5 }],
  text: [{ say: 'Saltreach', at: 0.1, seconds: 0, style: 'title' }],
  moments,
  ending: { kind: 'auto', after: 0 }
});

async function open(t, { viewport = { width: 1100, height: 760 }, motion = '', reducedMotion = false } = {}) {
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
    return route.fulfill({ contentType: 'text/html', body: PAGE(motion) });
  });
  await pg.goto('http://t.test/');
  return { pg, errors };
}

function start(pg, scene, opts = {}) {
  return pg.evaluate(([scene, opts]) => {
    document.getElementById('opener').focus();
    window.shut = [];
    window.asked = [];
    window.run = ChronicleCutscene.play(document.body, scene, {
      ...opts,
      onClose: (why) => window.shut.push(why),
      onMoment: (i, m) => window.asked.push([i, m.ask, m.ways.map((w) => w.kind), m.to])
    });
  }, [scene, opts]);
}

test('a moment stops the clock, asks over the dimmed scene, and the preview\'s Continue lets it run on', { skip }, async (t) => {
  const { pg, errors } = await open(t);
  await start(pg, SCENE([{ at: 0.8, ask: ASK, ways: [{ kind: 'write' }, { kind: 'director' }], to: 'one', note: 'They lie about the bell.' }]), { canContinue: true });
  await pg.waitForFunction(() => window.run.held() === 0);
  const at = await pg.evaluate(() => window.run.time());
  assert.ok(Math.abs(at - 0.8) < 1e-9, `held at ${at}`);
  await pg.waitForTimeout(600);
  assert.equal(await pg.evaluate(() => window.run.time()), at, 'the scene clock stays put while it holds');
  assert.deepEqual(await pg.evaluate(() => window.shut), [], 'an auto ending never closes a held scene');

  const state = await pg.evaluate(() => {
    const a = document.querySelector('.cs-cut-ask');
    const r = a.getBoundingClientRect();
    return {
      turn: a.querySelector('.cs-cut-turn').textContent,
      q: a.querySelector('.cs-cut-q').textContent,
      markup: a.querySelectorAll('b').length,
      label: document.getElementById(a.getAttribute('aria-labelledby'))?.textContent,
      ways: [...a.querySelectorAll('.cs-cut-way')].map((w) => [w.firstChild.textContent, w.getAttribute('aria-disabled'), getComputedStyle(w).opacity]),
      focus: document.activeElement.className,
      scrim: getComputedStyle(document.querySelector('.cs-cut-scrim')).opacity,
      inside: r.top >= 0 && r.bottom <= innerHeight && r.left >= 0 && r.right <= innerWidth,
      note: document.body.textContent.includes('They lie'),
      close: getComputedStyle(document.querySelector('.cs-cut-end')).visibility
    };
  });
  assert.deepEqual(state, {
    turn: 'Your turn', q: ASK, markup: 0, label: ASK,
    ways: [['Write it', 'true', '0.5'], ['Leave it to your Director', 'true', '0.5']],
    focus: 'cs-cut-go', scrim: '1', inside: true, note: false, close: 'hidden'
  });
  assert.deepEqual(await pg.evaluate(() => window.asked), [[0, ASK, ['write', 'director'], 'one']]);

  // A tap anywhere on a held scene does nothing; Tab stays on Continue.
  await pg.mouse.click(40, 700);
  await pg.keyboard.press('Tab');
  assert.equal(await pg.evaluate(() => document.activeElement.className), 'cs-cut-go');
  assert.equal(await pg.evaluate(() => window.run.held()), 0);

  await pg.keyboard.press('Enter');
  assert.equal(await pg.evaluate(() => window.run.held()), -1);
  await pg.waitForFunction(() => !document.querySelector('.cs-cut-ask'));
  await pg.waitForFunction((at) => window.run.time() > at + 0.3, at);
  await pg.waitForFunction(() => getComputedStyle(document.querySelector('.cs-cut-scrim')).opacity === '0');
  // The auto ending now runs: the moment was the last thing in the scene.
  await pg.waitForFunction(() => window.shut.length === 1);
  assert.deepEqual(await pg.evaluate(() => window.shut), ['auto']);
  assert.deepEqual(errors, []);
});

test('only go() resumes a player\'s scene; a moment at the start holds at once; hold() lands a late screen on a moment', { skip }, async (t) => {
  const { pg, errors } = await open(t);
  await start(pg, SCENE([{ at: 0, ask: 'First?' }, { at: 3, ask: 'Second?' }, { at: 6, ask: 'Third?' }]));
  assert.equal(await pg.evaluate(() => window.run.held()), 0, 'held before the first frame');
  assert.equal(await pg.evaluate(() => document.querySelectorAll('.cs-cut-go').length), 0, 'a player gets no Continue');
  assert.equal(await pg.evaluate(() => document.activeElement.classList.contains('cs-cut')), true, 'focus stays on the scene');
  await pg.waitForTimeout(400);
  assert.equal(await pg.evaluate(() => window.run.time()), 0);

  // A late screen lands on the third moment: the second counts as asked.
  await pg.evaluate(() => window.run.hold(2));
  assert.deepEqual(await pg.evaluate(() => [window.run.held(), window.run.time(), document.querySelector('.cs-cut-ask:not(.cs-cut-ask-gone) .cs-cut-q').textContent]), [2, 6, 'Third?'], 'the first question leaves as the third arrives');
  await pg.waitForFunction(() => document.querySelectorAll('.cs-cut-ask').length === 1);
  await pg.evaluate(() => window.run.go(2));
  await pg.waitForFunction(() => window.shut.length === 1);
  assert.deepEqual(await pg.evaluate(() => window.asked.map((a) => a[0])), [0, 2], 'the skipped moment never asked');
  assert.deepEqual(await pg.evaluate(() => window.asked[0][3]), 'each');
  assert.deepEqual(errors, []);
});

test('a moment let go before it is reached never holds, and Esc gets out of a held scene after two seconds', { skip }, async (t) => {
  const { pg, errors } = await open(t);
  await start(pg, SCENE([{ at: 0.5, ask: 'Skipped?' }, { at: 0.9, ask: 'Held?' }]));
  await pg.evaluate(() => window.run.go(0));
  await pg.waitForFunction(() => window.run.held() === 1);
  assert.deepEqual(await pg.evaluate(() => window.asked.map((a) => a[1])), ['Held?']);
  await pg.keyboard.press('Escape');
  assert.equal(await pg.evaluate(() => window.shut.length), 0, 'no way out in the first two seconds');
  await pg.waitForTimeout(2100);
  assert.equal(await pg.evaluate(() => getComputedStyle(document.querySelector('.cs-cut-x')).visibility), 'visible', 'the close mark shows on the wall clock');
  await pg.keyboard.press('Escape');
  assert.deepEqual(await pg.evaluate(() => window.shut), ['escape']);
  assert.deepEqual(errors, []);
});

for (const [name, setup, want] of [
  ['full motion rises', {}, 'cs-cut-ask'],
  ['Calm fades', { motion: 'calm' }, 'cs-cut-fade'],
  ['Off just shows it', { motion: 'off' }, 'none'],
  ['a device asking for reduced motion is Off', { reducedMotion: true }, 'none']
]) {
  test(`the question: ${name}`, { skip }, async (t) => {
    const { pg, errors } = await open(t, setup);
    await start(pg, SCENE([{ at: 0, ask: 'Now?' }]), { canContinue: true });
    assert.equal(await pg.evaluate(() => getComputedStyle(document.querySelector('.cs-cut-ask')).animationName), want);
    await pg.click('.cs-cut-go');
    if (want === 'none') assert.equal(await pg.evaluate(() => document.querySelectorAll('.cs-cut-ask').length), 0, 'Off takes it away at once');
    else await pg.waitForFunction(() => !document.querySelector('.cs-cut-ask'));
    assert.deepEqual(errors, []);
  });
}

test('on a phone the ways stack and nothing leaves the screen', { skip }, async (t) => {
  const { pg, errors } = await open(t, { viewport: { width: 390, height: 844 } });
  await start(pg, SCENE([{ at: 0, ask: 'What do the fishermen whisper about the Drowned Bell when the tide will not turn?', ways: ['write', 'director'] }]), { canContinue: true });
  const got = await pg.evaluate(() => {
    const ws = [...document.querySelectorAll('.cs-cut-way')].map((w) => w.getBoundingClientRect());
    const a = document.querySelector('.cs-cut-ask').getBoundingClientRect();
    const go = document.querySelector('.cs-cut-go').getBoundingClientRect();
    return { stacked: ws[1].top >= ws[0].bottom, inside: a.top >= 0 && a.bottom <= innerHeight && a.left >= 0 && a.right <= innerWidth, goTall: go.height >= 44, wayTall: ws.every((r) => r.height >= 44) };
  });
  assert.deepEqual(got, { stacked: true, inside: true, goTall: true, wayTall: true });
  assert.deepEqual(errors, []);
});

test('a scene without moments draws nothing new', { skip }, async (t) => {
  const { pg, errors } = await open(t);
  await start(pg, SCENE(undefined));
  assert.deepEqual(await pg.evaluate(() => [document.querySelectorAll('.cs-cut-scrim, .cs-cut-ask').length, window.run.held()]), [0, -1]);
  assert.deepEqual(errors, []);
});

// startAnswering plays a scene whose onAnswer records each answer and
// settles the way window.reply says: 'ok', or a message to reject with.
function startAnswering(pg, scene, opts = {}) {
  return pg.evaluate(([scene, opts]) => {
    window.sent = [];
    window.reply = 'ok';
    window.shut = [];
    window.run = ChronicleCutscene.play(document.body, scene, {
      ...opts,
      onClose: (why) => window.shut.push(why),
      onAnswer: (i, a) => {
        window.sent.push([i, a.way, a.text]);
        if (window.reply === 'ok') return Promise.resolve({});
        const e = new Error(window.reply); e.plain = true; return Promise.reject(e);
      }
    });
  }, [scene, opts]);
}

test('a viewer writes an answer in the scene, sees it sent, can change it, and a refusal says why', { skip }, async (t) => {
  const { pg, errors } = await open(t);
  await startAnswering(pg, SCENE([{ at: 0.3, ask: ASK, ways: [{ kind: 'write' }, { kind: 'director' }] }]));
  await pg.waitForFunction(() => window.run.held() === 0);
  const ways = await pg.$$eval('.cs-cut-ways button.cs-cut-way', (bs) => bs.map((b) => [b.firstChild.textContent, b.disabled]));
  assert.deepEqual(ways, [['Write it', false], ['Leave it to your Director', false]]);
  assert.equal(await pg.evaluate(() => document.activeElement.textContent.startsWith('Write it')), true, 'the first way takes focus');

  await pg.click('.cs-cut-way >> nth=0');
  assert.equal(await pg.evaluate(() => document.activeElement.tagName), 'TEXTAREA');
  assert.equal(await pg.getAttribute('.cs-cut-box textarea', 'maxlength'), '280');
  await pg.keyboard.press('Enter');
  assert.match(await pg.textContent('.cs-cut-err'), /Write a few words/);
  assert.deepEqual(await pg.evaluate(() => window.sent), [], 'an empty answer is not sent');

  await pg.keyboard.type('<b>It</b> rang   twice.');
  await pg.evaluate(() => { window.reply = 'The scene has moved on from this moment.'; });
  await pg.keyboard.press('Enter');
  await pg.waitForFunction(() => document.querySelector('.cs-cut-err').textContent.includes('moved on'));
  assert.equal(await pg.inputValue('.cs-cut-box textarea'), '<b>It</b> rang   twice.', 'a refusal keeps the words');

  await pg.evaluate(() => { window.reply = 'ok'; });
  await pg.click('.cs-cut-box .cs-cut-go');
  await pg.waitForSelector('.cs-cut-said');
  assert.deepEqual(await pg.evaluate(() => window.sent.at(-1)), [0, 'write', '<b>It</b> rang twice.']);
  assert.equal(await pg.textContent('.cs-cut-said'), '“<b>It</b> rang twice.”', 'the answer is text, never markup');
  assert.equal(await pg.$('.cs-cut-said b'), null);
  assert.match(await pg.textContent('.cs-cut-note'), /Your Director reads it before it joins the story/);

  // Escape in the box steps back rather than closing the scene.
  await pg.click('text=Change my answer');
  await pg.click('.cs-cut-way >> nth=0');
  assert.equal(await pg.inputValue('.cs-cut-box textarea'), '<b>It</b> rang twice.', 'changing starts from what was sent');
  await pg.waitForTimeout(2100);
  await pg.keyboard.press('Escape');
  assert.equal(await pg.$('.cs-cut-box'), null);
  assert.ok(await pg.$('.cs-cut-said'), 'back to the answer as sent');
  assert.deepEqual(await pg.evaluate(() => window.shut), [], 'Escape in the box did not close the scene');

  // Leave it to your Director sends at once.
  await pg.click('text=Change my answer');
  await pg.click('.cs-cut-way >> nth=1');
  await pg.waitForFunction(() => window.sent.length === 3);
  assert.deepEqual(await pg.evaluate(() => window.sent.at(-1)), [0, 'director', '']);
  assert.match(await pg.textContent('.cs-cut-note'), /You left it to your Director/);
  assert.equal(await pg.evaluate(() => window.run.held()), 0, 'answering never lets the scene go on');
  assert.deepEqual(errors, []);
});

test('who else is answering shows under the question, and one person\'s moment shuts for the rest', { skip }, async (t) => {
  const { pg, errors } = await open(t);
  await startAnswering(pg, SCENE([{ at: 0.3, ask: ASK, ways: [{ kind: 'write' }], to: 'one' }]));
  await pg.waitForFunction(() => window.run.held() === 0);
  await pg.evaluate(() => window.run.status(0, '<i>Ana</i> is answering this one.', true));
  assert.equal(await pg.textContent('.cs-cut-status'), '<i>Ana</i> is answering this one.');
  assert.equal(await pg.$eval('.cs-cut-way', (b) => b.disabled), true);
  await pg.evaluate(() => window.run.status(0, '', false));
  assert.equal(await pg.$eval('.cs-cut-way', (b) => b.disabled), false);
  assert.deepEqual(errors, []);
});

test('go() shows the lines that joined the story, then the scene runs on; a late screen gets its answer back', { skip }, async (t) => {
  const { pg, errors } = await open(t);
  await startAnswering(pg, SCENE([{ at: 0.3, ask: ASK, ways: [{ kind: 'write' }] }, { at: 0.6, ask: 'And then?', ways: [{ kind: 'write' }] }]));
  await pg.waitForFunction(() => window.run.held() === 0);
  await pg.evaluate(() => window.run.answered(0, { way: 'write', text: 'Sent before the reload.' }));
  assert.equal(await pg.textContent('.cs-cut-said'), '“Sent before the reload.”');

  await pg.evaluate(() => window.run.go(0, [{ line: '<em>It</em> rang.', from: 'Ana' }, { line: '', from: 'Bo' }]));
  const lines = await pg.$$eval('.cs-cut-lines li', (ls) => ls.map((l) => l.textContent));
  assert.deepEqual(lines, ['“<em>It</em> rang.”from Ana'], 'only real lines, as text');
  assert.equal(await pg.textContent('.cs-cut-turn'), 'Joins the story');
  assert.equal(await pg.evaluate(() => window.run.held()), 0, 'the lines stay a while');
  await pg.waitForFunction(() => window.run.held() === 1, null, { timeout: 8000 });
  assert.equal(await pg.$('.cs-cut-lines'), null);

  // Let go with nothing added: straight on.
  await pg.evaluate(() => window.run.go(1, []));
  assert.equal(await pg.evaluate(() => window.run.held()), -1);
  assert.deepEqual(errors, []);
});

test('moments already let go are played past', { skip }, async (t) => {
  const { pg, errors } = await open(t);
  await startAnswering(pg, SCENE([{ at: 0.2, ask: ASK, ways: [{ kind: 'write' }] }, { at: 0.4, ask: 'And then?', ways: [{ kind: 'write' }] }]), { released: [0] });
  await pg.waitForFunction(() => window.run.held() === 1);
  assert.equal(await pg.textContent('.cs-cut-q'), 'And then?');
  assert.deepEqual(errors, []);
});

test('without onAnswer the ways stay drawn but not answerable', { skip }, async (t) => {
  const { pg, errors } = await open(t);
  await start(pg, SCENE([{ at: 0.2, ask: ASK, ways: [{ kind: 'write' }] }]));
  await pg.waitForFunction(() => window.run.held() === 0);
  assert.equal(await pg.$('button.cs-cut-way'), null);
  assert.equal(await pg.getAttribute('li.cs-cut-way', 'aria-disabled'), 'true');
  assert.deepEqual(errors, []);
});
