// homebrew_asks_roll_browser.test.mjs: the roll button on a player's "Your
// asks" list for a house rule waiting on their roll, in a real Chromium with
// the list's own styles and a stubbed API. The page only asks for the roll;
// the total it shows is the server's.
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
const JS = read('internal/plugins/homebrew/static/js/homebrew_asks.js');
const CSS = read('internal/plugins/homebrew/static/css/asks.css');

const WAITING = {
  id: 'run-1', ask: 'draw a card', state: 'roll', answered: 'yes', at: new Date().toISOString(),
  roll: { label: 'Might', formula: '2d10+2', target: 12 }, canRoll: true
};
const made = Object.assign({}, WAITING, { state: 'rolled', canRoll: false, roll: { label: 'Might', formula: '2d10+2', target: 12, total: 15, made: true } });
const missed = Object.assign({}, WAITING, { state: 'missed', canRoll: false, roll: { label: 'Might', formula: '2d10+2', target: 12, total: 9 } });

// page mounts the list over a stub whose roll answers with the given view.
async function page(t, rolled, width) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: width || 420, height: 700 }, colorScheme: 'dark', deviceScaleFactor: 2 });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent('<!doctype html><html><head><style>:root{--color-accent:#6366f1;--color-text-primary:#e5e7eb;--color-text-muted:#9ca3af;--color-border:#374151;--color-bg-tertiary:#1f2937;--color-bg-primary:#111827}body{background:#0b1020;margin:0;padding:16px;font-family:system-ui,sans-serif}' + CSS +
    '</style></head><body><div class="hba" id="a"></div></body></html>');
  await p.evaluate(({ waiting, rolled }) => {
    window.__posts = [];
    let asks = [waiting];
    const json = (status, body) => Promise.resolve({ ok: status < 400, status, text: () => Promise.resolve(JSON.stringify(body)) });
    window.Chronicle = {
      register: (name, impl) => { window.__impl = impl; },
      apiFetch: (url, opts) => {
        if (url.endsWith('/asks/mine')) return json(200, { data: asks, total: asks.length });
        window.__posts.push({ url, method: opts.method, body: opts.body || null });
        if (rolled.status >= 400) return json(rolled.status, { message: rolled.message });
        asks = [rolled];
        return json(200, rolled);
      }
    };
  }, { waiting: WAITING, rolled });
  await p.addScriptTag({ content: JS });
  await p.evaluate(() => window.__impl.init(document.getElementById('a'), { endpoint: '/campaigns/c1/homebrew', csrfToken: 't', gm: 'Director' }));
  await p.waitForSelector('.hba-card');
  return p;
}

test('the roll button asks the server to roll and shows its total', { skip }, async (t) => {
  const cases = [[made, 'Rolled 15. The rule carries on.', 'Made the roll'], [missed, 'Rolled 9, under 12. The rule stops here.', 'Roll fell short']];
  for (const [view, result, pill] of cases) {
    const p = await page(t, view);
    assert.equal(await p.textContent('.hba-pill'), 'Yes, with a step first');
    assert.equal(await p.textContent('.hba-said-t'), 'Yes, if you roll Might 12 or more first.');
    assert.equal(await p.textContent('.hba-rollbtn'), 'Roll Might (2d10 + 2)');
    await p.click('.hba-rollbtn');
    await p.waitForSelector('.hba-result');
    const posts = await p.evaluate(() => window.__posts);
    assert.equal(posts.length, 1);
    assert.equal(posts[0].url, '/campaigns/c1/homebrew/asks/run-1/roll');
    assert.equal(posts[0].method, 'POST');
    assert.equal(posts[0].body, null, 'the page sends no result of its own');
    assert.equal(await p.textContent('.hba-result'), result);
    assert.equal(await p.textContent('.hba-pill'), pill);
    assert.equal(await p.$('.hba-rollbtn'), null, 'the button is gone once rolled');
    assert.equal(await p.evaluate(() => document.activeElement.className), 'hba-result hba-' + view.state, 'focus moves to the result');
  }
});

test('a refused roll keeps the button with the reason', { skip }, async (t) => {
  const p = await page(t, { status: 429, message: "That's a lot of rolls at once. Wait a moment and roll again." });
  await p.click('.hba-rollbtn');
  await p.waitForSelector('.hba-err');
  assert.match(await p.textContent('.hba-err'), /^That's a lot of rolls/);
  assert.equal(await p.textContent('.hba-rollbtn'), 'Roll Might (2d10 + 2)');
});

test('the roll button fits a phone without sideways scroll', { skip }, async (t) => {
  const p = await page(t, made, 320);
  const over = await p.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(over <= 0, 'page scrolls sideways by ' + over + 'px');
  const h = await p.evaluate(() => document.querySelector('.hba-rollbtn').getBoundingClientRect().height);
  assert.ok(h >= 44, 'touch target is ' + h + 'px');
});
