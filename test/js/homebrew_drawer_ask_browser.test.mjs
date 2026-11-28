// homebrew_drawer_ask_browser.test.mjs: the Homebrew drawer's answer card for
// a rule waiting at an "Ask the Director" step, in a real Chromium with the
// drawer's own styles and a stubbed API.
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
const read = (f) => readFileSync(join(root, f), 'utf8');
const JS = read('internal/plugins/homebrew/static/js/homebrew_drawer.js');
const CSS = read('internal/plugins/homebrew/static/css/drawer.css');

// One rule waits at its ask: Mira wants to draw a card, and a Yes shuffles
// copies into a deck and steps Doom.
const ASK = {
  id: 'run-1', recipe: 'Lucky draw', what: 'A card is played', status: 'held', lines: [], at: new Date().toISOString(),
  waits: 'Asks you before it goes on.', recipeId: 'rec-1', scope: 'hero-1',
  steps: [{ action: 'ask-director', label: 'Ask the Director' }, { action: 'deck.add-copies', label: 'Shuffle copies of a card into a deck' }, { action: 'step', label: 'Step', part: 'doom' }],
  ask: {
    asker: 'Mira', askerId: 'mira-1', ask: 'draw a card', step: 0,
    later: [
      { step: 1, action: 'deck.add-copies', label: 'Shuffle copies of a card into a deck', args: { deck: 'Fortunes', card: 'Luck', copies: 1 },
        fields: [{ name: 'deck', label: 'Deck', type: 'text' }, { name: 'card', label: 'Card', type: 'text' }, { name: 'copies', label: 'Copies', type: 'whole' }] },
      { step: 2, action: 'step', label: 'Step', part: 'doom', args: { by: 1 }, fields: [{ name: 'by', label: 'By', type: 'whole' }],
        parts: [{ key: 'doom', label: 'Doom' }, { key: 'dread', label: 'Dread' }] }
    ]
  }
};

async function page(t, answerReply) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 420, height: 900 }, colorScheme: 'dark' });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent('<!doctype html><html><head><style>:root{--color-accent:#6366f1;--color-text-primary:#e5e7eb;--color-text-muted:#9ca3af;--color-border:#374151;--color-bg-tertiary:#1f2937;--color-bg-primary:#111827}body{background:#0b1020;margin:0}' + CSS +
    '</style></head><body><div class="hbd" id="d"></div></body></html>');
  await p.evaluate(({ ask, reply }) => {
    window.__posts = [];
    let waiting = [ask];
    const json = (status, body) => Promise.resolve({ ok: status < 400, status, text: () => Promise.resolve(JSON.stringify(body)) });
    window.Chronicle = {
      register: (name, impl) => { window.__impl = impl; },
      apiFetch: (url, opts) => {
        if (url.endsWith('/drawer')) return json(200, { presses: [], waiting, recent: [] });
        window.__posts.push({ url, body: opts.body ? JSON.parse(opts.body) : null });
        if (reply.status >= 400) {
          // A 409 means the ask was answered elsewhere: it is gone on reload.
          if (reply.status === 409) waiting = [];
          return json(reply.status, { message: reply.message });
        }
        waiting = [];
        return json(200, { id: ask.id, status: 'done', lines: ['Answered yes.'] });
      },
      working: () => {}
    };
  }, { ask: ASK, reply: answerReply || { status: 200 } });
  await p.addScriptTag({ content: JS });
  await p.evaluate(() => window.__impl.init(document.getElementById('d'), { endpoint: '/campaigns/c1/homebrew', campaignId: 'c1', csrfToken: 't' }));
  await p.waitForSelector('.hbd-ask');
  return p;
}

test('the answer card asks Yes, No or Other and carries "Yes, and…" changes', { skip }, async (t) => {
  const p = await page(t);
  assert.equal(await p.textContent('.hbd-ask-title'), 'Mira wants to draw a card');
  assert.equal(await p.textContent('.hbd-count'), '1 needs you');
  assert.equal(await p.$('.hbd-send'), null, 'nothing to send before a pick');
  await p.click('.hbd-choice:text("Yes")');
  assert.equal(await p.getAttribute('.hbd-choice:text("Yes")', 'aria-pressed'), 'true');
  assert.equal(await p.evaluate(() => document.activeElement.textContent), 'Yes', 'focus stays on the pick');
  // Text fields and the part show at once; how many waits behind its chip.
  const labels = await p.$$eval('.hbd-cond-l', (els) => els.map((e) => e.textContent));
  assert.deepEqual(labels, ['Deck', 'Card', 'On']);
  assert.equal(await p.textContent('.hbd-send'), 'Send: yes, then shuffle copies of a card into a deck, then step');
  await p.click('.hbd-chip');
  await p.fill('[aria-label="Shuffle copies of a card into a deck: Copies"]', '3');
  await p.fill('[aria-label="Shuffle copies of a card into a deck: Deck"]', 'Rewards');
  await p.selectOption('[aria-label="Step: on"]', 'dread');
  assert.match(await p.textContent('.hbd-send'), /^Send: yes, with changes, then/);
  await p.click('.hbd-send');
  await p.waitForFunction(() => !document.querySelector('.hbd-ask'));
  const posts = await p.evaluate(() => window.__posts);
  assert.equal(posts.length, 1);
  assert.equal(posts[0].url, '/campaigns/c1/homebrew/runs/run-1/answer');
  assert.deepEqual(posts[0].body, { answer: 'yes', steps: [
    { step: 1, args: { deck: 'Rewards', card: 'Luck', copies: 3 } },
    { step: 2, part: 'dread', args: { by: 1 } }
  ] });
  assert.match(await p.textContent('.hbd-status'), /^Sent: yes\./);
});

test('Other needs words, and a refused change keeps the card with the reason', { skip }, async (t) => {
  const p = await page(t, { status: 422, message: 'step 2: give a whole number of copies from 1 to 20' });
  await p.click('.hbd-other');
  assert.equal(await p.evaluate(() => document.activeElement.className), 'hbd-reply', 'Other opens the reply and puts focus in it');
  assert.equal(await p.textContent('.hbd-send'), 'Send reply');
  await p.click('.hbd-send');
  assert.match(await p.textContent('.hbd-err'), /^Write the reply Mira gets/);
  assert.equal((await p.evaluate(() => window.__posts)).length, 0, 'an empty reply is not sent');
  await p.click('.hbd-choice:text("Yes")');
  await p.click('.hbd-chip');
  await p.fill('[aria-label="Shuffle copies of a card into a deck: Copies"]', '99');
  await p.click('.hbd-send');
  await p.waitForSelector('.hbd-err');
  assert.equal(await p.textContent('.hbd-err'), 'step 2: give a whole number of copies from 1 to 20');
  assert.ok(await p.$('.hbd-ask'), 'the card stays to fix the change');
  assert.equal(await p.inputValue('[aria-label="Shuffle copies of a card into a deck: Copies"]'), '99', 'what was typed is kept');
});

test('an answer that lost to another one says why on the status line', { skip }, async (t) => {
  const p = await page(t, { status: 409, message: 'this ask is already being answered' });
  await p.click('.hbd-choice:text("Yes")');
  await p.click('.hbd-send');
  await p.waitForFunction(() => !document.querySelector('.hbd-ask'));
  assert.equal(await p.textContent('.hbd-status'), 'this ask is already being answered');
  assert.match(await p.getAttribute('.hbd-status', 'class'), /\berr\b/);
});

test('the answer card fits a phone without sideways scroll', { skip }, async (t) => {
  const p = await page(t);
  await p.click('.hbd-choice:text("Yes")');
  await p.click('.hbd-chip');
  await p.setViewportSize({ width: 320, height: 900 });
  const over = await p.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(over <= 0, 'page scrolls sideways by ' + over + 'px');
});
