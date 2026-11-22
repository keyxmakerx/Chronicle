// deck_card_looks.spec.mjs: the Director's proof sheet of card looks
// (/campaigns/:id/decks/looks). A player meets a missing page; the Director's
// sheet draws in light, dark and at phone width with no sideways scroll, no
// clipped name and no console error; and every turn-over and fixed move, at
// Full, Calm and Off, ends on a plain face-up (or resting) card with nothing
// still animating a second later.
//
// Set LIVE_SHOTS to a folder to also save pictures of the sheet there.
//
// RUN: tools/live-checks.sh test/live/deck_card_looks.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, BASE } from './_live.mjs';

after(done);

const SHOTS = process.env.LIVE_SHOTS || '';
const TURNS = ['flip', 'slow', 'toss', 'slam', 'sweep', 'burn', 'ink', 'seal', 'bolt', 'forge'];
const MOVES = ['draw', 'deal', 'shuffle', 'play', 'used', 'gone', 'ready'];
const LEVELS = ['full', 'calm', 'off'];

async function sheet(opts = {}) {
  const owner = await person({ tag: 'decks-dir', name: 'Director Dara', context: { reducedMotion: 'no-preference', ...(opts.context || {}) } });
  const cid = await newCampaign(owner, 'Card looks check');
  await enableAddons(owner, cid, ['decks']);
  // A strong device, and a log of what each card animates, so a test can tell
  // a turn that played its own effect from one that fell back to a fade.
  await owner.page.addInitScript(() => {
    Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => 8 });
    window.__anims = [];
    const animate = Element.prototype.animate;
    Element.prototype.animate = function (frames, opts) {
      try {
        const keys = new Set();
        (Array.isArray(frames) ? frames : []).forEach((f) => Object.keys(f).forEach((k) => keys.add(k + (k === 'filter' ? '=' + f[k] : ''))));
        window.__anims.push({ on: String(this.className), keys: [...keys] });
      } catch (e) { /* the log is only a test aid */ }
      return animate.call(this, frames, opts);
    };
  });
  const errors = [];
  owner.page.on('console', (m) => {
    // Other hosts are cut off in these runs, so their failed loads are expected.
    if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) errors.push(m.text());
  });
  owner.page.on('pageerror', (e) => errors.push(String(e)));
  const res = await owner.go(`/campaigns/${cid}/decks/looks`);
  assert.equal(res.status(), 200);
  await owner.page.locator('.dk-pf').waitFor();
  return { owner, cid, page: owner.page, errors };
}

const setLevel = (page, level) => page.evaluate((l) => {
  if (l === 'full') document.documentElement.removeAttribute('data-motion');
  else document.documentElement.setAttribute('data-motion', l);
}, level);

const running = (page) => page.evaluate(() => document.getAnimations()
  .filter((a) => a.playState !== 'finished' && a.effect && a.effect.target && a.effect.target.closest && a.effect.target.closest('.dk-pf'))
  .map((a) => (a.effect.target.className || a.effect.target.tagName) + ':' + (a.animationName || a.id || 'waapi')));

test('a player meets a missing page, the Director meets the sheet', async () => {
  const { owner, cid } = await sheet();
  const player = await person({ tag: 'decks-player', name: 'Player Pip' });
  await addMember(owner, cid, player, 'player');
  const res = await player.go(`/campaigns/${cid}/decks/looks`);
  assert.equal(res.status(), 404);
  assert.equal(await player.page.locator('.dk-card').count(), 0);
});

test('the sheet draws in light, dark and at 390 wide with no sideways scroll, clipped name or error', async () => {
  const { page, errors } = await sheet();
  for (const [name, scheme, width] of [['light', 'light', 1280], ['dark', 'dark', 1280], ['phone', 'light', 390]]) {
    await page.setViewportSize({ width, height: 900 });
    await page.emulateMedia({ colorScheme: scheme });
    await page.waitForTimeout(200);
    const wide = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    assert.ok(wide <= 0, `${name}: the page scrolls sideways by ${wide}px`);
    const bad = await page.evaluate(() => [...document.querySelectorAll('.dk-pf .dk-name, .dk-pf .dk-dn')].filter((n) => {
      const card = n.closest('.dk-card, .dk-leaf');
      const a = n.getBoundingClientRect(), b = card.getBoundingClientRect();
      return n.scrollWidth > n.clientWidth + 1 || a.left < b.left - 1 || a.right > b.right + 1;
    }).length);
    assert.equal(bad, 0, `${name}: ${bad} names are clipped or overflow their card`);
    const over = await page.evaluate(() => [...document.querySelectorAll('.dk-pf .dk-card')].filter((c) => c.getBoundingClientRect().right > window.innerWidth + 1).length);
    assert.equal(over, 0, `${name}: ${over} cards run past the screen`);
    if (SHOTS) await page.screenshot({ path: `${SHOTS}/decks-looks-${name}.png`, fullPage: true });
  }
  assert.deepEqual(errors, []);
});

test('every turn-over, at Full, Calm and Off, ends face up with nothing still moving', async () => {
  const { page, errors } = await sheet();
  const ones = page.locator('#turns .dk-pf-one');
  assert.equal(await ones.count(), TURNS.length);
  for (const level of LEVELS) {
    await setLevel(page, level);
    for (let i = 0; i < TURNS.length; i++) {
      const one = ones.nth(i);
      const card = one.locator('[data-dk-card]');
      assert.equal(await card.getAttribute('data-turn'), TURNS[i]);
      await one.scrollIntoViewIfNeeded();
      const button = card.locator('.dk-turn');
      assert.match(await button.getAttribute('aria-label'), /^Turn over card \d+ of 10, Sample deck$/);
      await button.focus();
      await page.keyboard.press('Enter'); // a turn started from the keyboard still plays
      await card.and(page.locator('[data-state=up]')).waitFor({ timeout: 4000 });
      await page.waitForTimeout(1000);
      if (level === 'full' && (TURNS[i] === 'burn' || TURNS[i] === 'ink')) {
        // Burn moves its own edge and ink blurs the back; a fade does neither.
        const seen = await page.evaluate(() => window.__anims.map((a) => a.on + ' ' + a.keys.join(' ')).join('\n'));
        if (TURNS[i] === 'burn') assert.match(seen, /--dk-r/, 'burn did not play its own effect');
        else assert.match(seen, /dk-back[^\n]*filter=blur\(3px\)/, 'ink did not play its own effect');
      }
      assert.deepEqual(await running(page), [], `${TURNS[i]} at ${level} left an animation running`);
      assert.equal(await card.locator('.dk-back, .dk-turner, .dk-fx, .dk-plain, .dk-turn').count(), 0, `${TURNS[i]} at ${level} left parts behind`);
      assert.equal(await card.locator('.dk-plate').count(), 1, `${TURNS[i]} at ${level} ended without the face`);
      assert.equal(await card.evaluate((c) => document.activeElement === c), true, `${TURNS[i]} at ${level}: focus left the card`);
      assert.equal((await card.locator('.dk-live').textContent()).trim(), 'Sample card, Kind violet');
      await one.getByRole('button', { name: 'Reset' }).click();
      await card.and(page.locator('[data-state=down]')).waitFor();
    }
  }
  assert.deepEqual(errors, []);
});

test('a face that is slow to arrive is not waited for past the halfway point, and still lands', async () => {
  const { page } = await sheet();
  await setLevel(page, 'full');
  const one = page.locator('#turns .dk-pf-one').first();
  await one.locator('template.dk-face-tpl').evaluate((t) => t.setAttribute('data-delay', '1200'));
  const card = one.locator('[data-dk-card]');
  const t0 = Date.now();
  await card.locator('.dk-turn').click();
  // The second half has begun by the halfway wait, before the face is there.
  await page.waitForFunction(() => document.querySelector('#turns .dk-pf-one [data-dk-card] .dk-plain'), null, { timeout: 1500 });
  assert.ok(Date.now() - t0 < 1100, 'the turn waited for the face');
  await card.and(page.locator('[data-state=up]')).waitFor({ timeout: 4000 });
  await page.waitForTimeout(1000);
  assert.equal(await card.locator('.dk-plate').count(), 1);
  assert.deepEqual(await running(page), []);
});

test('each fixed move, at Full, Calm and Off, ends on the resting card', async () => {
  const { page, errors } = await sheet();
  const ones = page.locator('#moves .dk-pf-one');
  assert.equal(await ones.count(), MOVES.length);
  for (const level of LEVELS) {
    await setLevel(page, level);
    for (let i = 0; i < MOVES.length; i++) {
      const one = ones.nth(i);
      await one.scrollIntoViewIfNeeded();
      await one.locator('[data-dk-move]').click();
      await page.waitForTimeout(level === 'off' ? 100 : MOVES[i] === 'ready' ? 2400 : 1700);
      assert.deepEqual(await running(page), [], `${MOVES[i]} at ${level} is still moving`);
      assert.equal(await one.locator('.dk-fx').count(), 0, `${MOVES[i]} at ${level} left an effect layer`);
      const cards = one.locator('[data-dk-card]');
      if (MOVES[i] === 'gone') assert.equal(await cards.first().evaluate((c) => c.hidden), true);
      if (MOVES[i] === 'used') assert.equal(await cards.first().evaluate((c) => c.classList.contains('dk-spent')), true);
      assert.equal(await cards.first().evaluate((c) => c.style.transform || ''), '', `${MOVES[i]} left a transform`);
      await one.locator('[data-dk-move]').evaluate((b) => b.closest('[data-dk-one]').querySelectorAll('[data-dk-card]').forEach((c) => { c.hidden = false; c.classList.remove('dk-spent'); }));
    }
  }
  assert.deepEqual(errors, []);
});

test('sheen and foil move only while the pointer rests, and not at all on Calm or Off', async () => {
  const { page } = await sheet();
  const fin = page.locator('#finishes .dk-pf-one', { hasText: /sheen/i }).first().locator('.dk-card');
  await fin.scrollIntoViewIfNeeded();
  const sweep = () => page.evaluate(() => document.getAnimations().filter((a) => a.animationName === 'dk-sheen').length);
  assert.equal(await sweep(), 0, 'it moves with no pointer on it');
  await setLevel(page, 'full');
  await fin.hover();
  await page.waitForTimeout(200);
  assert.ok((await sweep()) >= 1, 'it did not move under the pointer at Full');
  await page.mouse.move(0, 0);
  for (const level of ['calm', 'off']) {
    await setLevel(page, level);
    await fin.hover();
    await page.waitForTimeout(200);
    assert.equal(await sweep(), 0, `it moved at ${level}`);
    await page.mouse.move(0, 0);
  }
});
