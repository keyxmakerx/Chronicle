// deck_workshop.spec.mjs: the Director's deck workshop (/campaigns/:id/decks/workshop).
// A player or scribe meets a missing page; a Director makes a deck from a style
// (by mouse and by keyboard), adds and edits a card, changes the look and sees
// the big card change, draws, deals, puts back, keeps and sets "comes up next";
// every page draws in light, dark and at phone width with no sideways scroll
// and no console error; and, on the Look tab, every turn-over and the sheen
// behave as the card's own widget promises (ends face up, nothing still moving).
//
// Set LIVE_SHOTS to a folder to also save pictures there.
//
// RUN: tools/live-checks.sh test/live/deck_workshop.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, newPage, api, done } from './_live.mjs';

after(done);

const SHOTS = process.env.LIVE_SHOTS || '';
const TURNS = ['flip', 'slow', 'toss', 'slam', 'sweep', 'burn', 'ink', 'seal', 'bolt', 'forge'];
const LEVELS = ['full', 'calm', 'off'];

async function director(tag = 'dkw-dir') {
  const owner = await person({ tag, name: 'Director Dara', context: { reducedMotion: 'no-preference' } });
  const cid = await newCampaign(owner, 'Deck workshop check');
  await enableAddons(owner, cid, ['decks']);
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
    if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) errors.push(m.text());
  });
  owner.page.on('pageerror', (e) => errors.push(String(e)));
  return { owner, cid, page: owner.page, errors };
}

// makeDeck walks the making page with the mouse and lands on the new deck.
async function makeDeck(page, cid, name, style = 'Iron omens') {
  await page.goto(new URL(`/campaigns/${cid}/decks/workshop/new`, page.url().startsWith('http') ? page.url() : undefined).href);
  await page.getByLabel('Name', { exact: true }).fill(name);
  await page.getByRole('radio', { name: new RegExp(style, 'i') }).click();
  await page.getByRole('button', { name: 'Make the deck' }).click();
  await page.waitForURL(/\/decks\/workshop\/[0-9a-f-]{36}/);
  await page.locator('#dkw-panel').waitFor();
}

const setLevel = (page, level) => page.evaluate((l) => {
  if (l === 'full') document.documentElement.removeAttribute('data-motion');
  else document.documentElement.setAttribute('data-motion', l);
}, level);

const running = (page, scope) => page.evaluate((s) => document.getAnimations()
  .filter((a) => a.playState !== 'finished' && a.effect && a.effect.target && a.effect.target.closest && a.effect.target.closest(s))
  .map((a) => (a.effect.target.className || a.effect.target.tagName) + ':' + (a.animationName || a.id || 'waapi')), scope);

const noSideways = (page, what) => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  .then((w) => assert.ok(w <= 0, `${what}: the page scrolls sideways by ${w}px`));

async function addCard(page, name) {
  await page.getByRole('button', { name: '+ Card' }).click();
  await page.locator('#dkw-f-name').waitFor();
  await page.locator('#dkw-f-name').fill(name);
  await page.locator('#dkw-f-name').blur();
  await page.locator('#dkw-grid').getByRole('link', { name: new RegExp('^' + name) }).waitFor();
}

test('a player and a scribe meet a missing page, a Director sees the app', async () => {
  const { owner, cid, page } = await director();
  const player = await person({ tag: 'dkw-player', name: 'Player Pip' });
  const scribe = await person({ tag: 'dkw-scribe', name: 'Scribe Sol' });
  await addMember(owner, cid, player, 'player');
  await addMember(owner, cid, scribe, 'scribe');
  for (const who of [player, scribe]) {
    for (const path of ['workshop', 'workshop/new', 'workshop/00000000-0000-0000-0000-000000000000']) {
      const res = await who.go(`/campaigns/${cid}/decks/${path}`);
      assert.equal(res.status(), 404, `${path} for a member who is not the Director`);
      assert.equal(await who.page.locator('.dk-card, .dkw').count(), 0);
    }
  }
  const res = await owner.go(`/campaigns/${cid}/decks/workshop`);
  assert.equal(res.status(), 200);
  await page.getByRole('heading', { name: 'Card decks' }).first().waitFor();
  // The Director's app row is in the drawer; nobody else's is.
  await owner.go(`/campaigns/${cid}`);
  assert.ok(await page.locator('a[href$="/decks/workshop"]').count() >= 1, 'the Director has no Card decks row');
  await player.go(`/campaigns/${cid}`);
  assert.equal(await player.page.locator('a[href$="/decks/workshop"]').count(), 0, 'a player was shown the Card decks row');
});

test('make a deck from a style by mouse, add a card, change the look and see the big card change', async () => {
  const { owner, cid, page, errors } = await director();
  await owner.go(`/campaigns/${cid}/decks/workshop/new`);
  assert.equal(await page.locator('.dkw-style').count(), 7);
  await page.getByLabel('Name', { exact: true }).fill('Omens of the road');
  await page.getByRole('radio', { name: /Iron omens/i }).click();
  assert.equal(await page.getByRole('radio', { name: /Iron omens/i }).getAttribute('aria-checked'), 'true');
  await page.getByRole('button', { name: 'Make the deck' }).click();
  await page.waitForURL(/\/decks\/workshop\/[0-9a-f-]{36}/);
  assert.equal(await page.locator('[data-dkw-name]').inputValue(), 'Omens of the road');

  await addCard(page, 'The Crow Speaks');
  assert.match(await page.locator('#dkw-grid').textContent(), /The Crow Speaks/);

  await page.getByRole('tab', { name: 'Look' }).click();
  const big = page.locator('#dkw-bigcard [data-dk-card]').first();
  await big.waitFor();
  assert.ok(await page.locator('[data-dk-stage]').count() >= 1, 'the big card is not on a stage');
  const before = await page.locator('#dkw-bigcard').innerHTML();
  const border = page.locator('[data-row=border] [role=radio]:not([aria-checked=true]):not([aria-disabled=true])').first();
  const pickedValue = await border.getAttribute('data-value');
  await border.click();
  await page.waitForFunction((v) => document.querySelector(`[data-row=border] [data-value="${v}"]`)?.getAttribute('aria-checked') === 'true', pickedValue);
  await page.waitForFunction((b) => document.querySelector('#dkw-bigcard')?.innerHTML !== b, before, { timeout: 5000 }).catch(() => assert.fail('the big card did not change'));
  const turn = page.locator('[data-row=turn] [role=radio]:not([aria-checked=true]):not([aria-disabled=true])').first();
  const turnValue = await turn.getAttribute('data-value');
  await turn.click();
  await page.waitForFunction((v) => document.querySelector(`[data-row=turn] [data-value="${v}"]`)?.getAttribute('aria-checked') === 'true', turnValue);
  // A reload keeps what was chosen: it was saved, not just drawn.
  await page.reload();
  await page.locator('#dkw-panel').waitFor();
  assert.equal(await page.locator(`[data-row=border] [data-value="${pickedValue}"]`).getAttribute('aria-checked'), 'true');
  assert.deepEqual(errors, []);
});

test('make a deck and add a card with the keyboard alone', async () => {
  const { owner, cid, page, errors } = await director();
  await owner.go(`/campaigns/${cid}/decks/workshop/new`);
  await page.locator('#dkw-name').focus();
  await page.keyboard.type('Keyboard deck');
  // Tab to the kind chips, the hero chooser is optional; arrow to a style and press Space.
  await page.getByRole('radio', { name: /Tavern game/i }).focus();
  await page.keyboard.press('Space');
  assert.equal(await page.getByRole('radio', { name: /Tavern game/i }).getAttribute('aria-checked'), 'true');
  await page.getByRole('button', { name: 'Make the deck' }).focus();
  await page.keyboard.press('Enter');
  await page.waitForURL(/\/decks\/workshop\/[0-9a-f-]{36}/);
  await page.getByRole('button', { name: '+ Card' }).focus();
  await page.keyboard.press('Enter');
  await page.locator('#dkw-f-name').waitFor();
  assert.equal(await page.locator('#dkw-f-name').evaluate((n) => document.activeElement === n), true, 'focus did not land on the new card name');
  await page.keyboard.press('Control+A');
  await page.keyboard.type('Keyed card');
  await page.keyboard.press('Tab');
  await page.locator('#dkw-grid').getByRole('link', { name: /^Keyed card/ }).waitFor();
  // The tabs move with the arrow keys.
  await page.getByRole('tab', { name: 'Cards' }).focus();
  await page.keyboard.press('ArrowRight');
  await page.locator('#dkw-tab-look[aria-selected=true]').waitFor();
  assert.deepEqual(errors, []);
});

test('draw three, deal one to the table, put one back, keep one in a second deck, set and clear comes up next', async () => {
  const { owner, cid, page, errors } = await director();
  await makeDeck(page, cid, 'Second deck', 'Plain');
  await makeDeck(page, cid, 'Main deck', 'Plain');
  for (const n of ['One', 'Two', 'Three', 'Four']) await addCard(page, n);
  await page.getByRole('tab', { name: 'Cards' }).click();
  assert.match(await page.locator('[data-dkw-left]').textContent(), /4/);

  // A stepper that stops at what can be drawn.
  const count = page.locator('[data-dkw-count]');
  const up = page.getByRole('button', { name: 'Draw one more' });
  await up.click(); await up.click();
  assert.equal((await count.textContent()).trim(), '3');
  await page.getByRole('button', { name: 'Draw', exact: true }).click();
  await page.locator('.dkw-shownone').nth(2).waitFor();
  assert.equal(await page.locator('.dkw-shownone').count(), 3);
  assert.match(await page.locator('[data-dkw-left]').textContent(), /1/);

  await page.locator('.dkw-shownone').first().getByRole('button', { name: 'Deal to the table' }).click();
  await page.waitForFunction(() => document.querySelectorAll('.dkw-shownone').length === 2);
  await page.locator('.dkw-shownone').first().getByRole('button', { name: 'Put back' }).click();
  await page.waitForFunction(() => document.querySelectorAll('.dkw-shownone').length === 1);
  assert.match(await page.locator('.dkw-playlist').textContent(), /In play/);

  await page.locator('.dkw-shownone').first().locator('select[data-act=keep]').selectOption({ label: 'Second deck' });
  await page.waitForFunction(() => document.querySelectorAll('.dkw-shownone').length === 0);

  const next = page.locator('#dkw-nudge-next');
  await next.selectOption({ label: 'Four' });
  await page.waitForFunction(() => document.querySelector('[data-act=nudge-clear][data-which=next]:not([disabled])'));
  await page.reload();
  assert.equal(await page.locator('#dkw-nudge-next option:checked').textContent(), 'Four');
  await page.locator('[data-act=nudge-clear][data-which=next]').click();
  await page.waitForFunction(() => document.querySelector('[data-act=nudge-clear][data-which=next]')?.disabled);
  assert.equal(await page.locator('#dkw-nudge-next option:checked').textContent(), 'No card');
  assert.deepEqual(errors, []);
  void owner;
});

test('a failed save says so in plain words, puts the choice back and returns focus', async () => {
  const { owner, cid, page } = await director();
  await makeDeck(page, cid, 'Failing deck', 'Plain');
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('[data-row=border]').waitFor();
  const start = await page.locator('[data-row=border] [role=radio][aria-checked=true]').getAttribute('data-value');
  await page.route(/\/decks\/[0-9a-f-]{36}$/, (r) => (r.request().method() === 'PUT' ? r.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"The server could not save that."}' }) : r.continue()));
  const other = page.locator('[data-row=border] [role=radio]:not([aria-checked=true]):not([aria-disabled=true])').first();
  await other.click();
  await page.locator('[data-dkw-err]:not([hidden])').first().waitFor();
  assert.match(await page.locator('[data-dkw-err]:not([hidden])').first().textContent(), /could not save/i);
  assert.equal(await page.locator('[data-row=border] [role=radio][aria-checked=true]').getAttribute('data-value'), start);
  assert.equal(await page.evaluate(() => !!document.activeElement.closest('[data-row=border]')), true, 'focus left the choice');
  void owner;
});

test('small cards say why a choice is unavailable, and Show all opens the folded rows', async () => {
  const { owner, cid, page } = await director();
  await makeDeck(page, cid, 'Small deck', 'Plain');
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('[data-row=shape]').waitFor();
  const small = page.locator('[data-row=shape] [data-value=small]');
  await small.click();
  // The press checks the choice at once; which choices the small shape rules
  // out arrive with the saved tab, so wait for that and not for the check.
  await page.waitForFunction(() => document.querySelector('[data-row=shape] [data-value=small]')?.getAttribute('aria-checked') === 'true'
    && document.querySelector('[data-row] [data-value=burn]')?.getAttribute('aria-disabled') === 'true');
  const burn = page.locator('[data-row] [data-value=burn]').first();
  assert.equal(await burn.getAttribute('aria-disabled'), 'true');
  assert.ok((await burn.getAttribute('title')).length > 3, 'no reason is given');
  const fold = page.locator('[data-act=show-all]').first();
  const hiddenBefore = await page.locator('[data-row] [role=radio][hidden]').count();
  assert.ok(hiddenBefore > 0);
  await fold.click();
  assert.equal(await fold.getAttribute('aria-expanded'), 'true');
  assert.ok(await page.locator('[data-row] [role=radio][hidden]').count() < hiddenBefore);
  void owner;
});

test('every page and tab draws in light, dark and at 390 wide with no sideways scroll or error', async () => {
  const { owner, cid, page, errors } = await director();
  await makeDeck(page, cid, 'Wide check', 'boons');
  await addCard(page, 'A card with a rather long name to test the clipping of names');
  const url = page.url();
  for (const [name, scheme, width] of [['light', 'light', 1280], ['dark', 'dark', 1280], ['phone', 'light', 390]]) {
    await page.setViewportSize({ width, height: 900 });
    await page.emulateMedia({ colorScheme: scheme });
    for (const tab of ['Cards', 'Look', 'Rules', 'Players']) {
      await page.getByRole('tab', { name: tab }).click();
      await page.locator(`#dkw-tab-${tab.toLowerCase()}[aria-selected=true]`).waitFor();
      await page.waitForTimeout(250);
      await noSideways(page, `${name} ${tab}`);
      const over = await page.evaluate(() => { const scrolls = (n) => { for (let a = n.parentElement; a && a !== document.body; a = a.parentElement) { if (/auto|scroll/.test(getComputedStyle(a).overflowX)) return true; } return false; }; return [...document.querySelectorAll('.dkw .dk-card')].filter((c) => !c.closest('[hidden]') && !scrolls(c) && c.getBoundingClientRect().width > 0 && c.getBoundingClientRect().right > window.innerWidth + 1).length; });
      assert.equal(over, 0, `${name} ${tab}: ${over} cards run past the screen`);
      if (SHOTS) await page.screenshot({ path: `${SHOTS}/workshop-${tab.toLowerCase()}-${name}.png`, fullPage: true });
    }
    for (const path of ['workshop', 'workshop/new']) {
      await owner.go(`/campaigns/${cid}/decks/${path}`);
      await noSideways(page, `${name} ${path}`);
      if (SHOTS) await page.screenshot({ path: `${SHOTS}/workshop-${path.replace('/', '-')}-${name}.png`, fullPage: true });
    }
    await page.goto(url);
  }
  assert.deepEqual(errors, []);
});

test('on the Look tab every turn-over, at Full, Calm and Off, ends face up with nothing still moving', async () => {
  const { owner, cid, page, errors } = await director();
  await makeDeck(page, cid, 'Turns deck', 'Plain');
  await addCard(page, 'Sample');
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('[data-row=turn]').waitFor();
  const seen = [];
  const unfold = async () => {
    const b = page.locator('[data-row=turn] [data-act=show-all][aria-expanded=false]');
    if (await b.count()) await b.click();
  };
  for (const level of LEVELS) {
    await setLevel(page, level);
    for (const t of TURNS) {
      const opt = page.locator(`[data-row=turn] [data-value="${t}"]`);
      await unfold();
      if (await opt.count() === 0) continue;
      if (await opt.getAttribute('aria-disabled') === 'true') continue;
      await opt.click();
      await page.waitForFunction((v) => document.querySelector(`[data-row=turn] [data-value="${v}"]`)?.getAttribute('aria-checked') === 'true', t);
      await page.getByRole('button', { name: 'Play the turn-over' }).click();
      const card = page.locator('#dkw-bigcard [data-dk-card]').first();
      await card.and(page.locator('[data-state=up]')).waitFor({ timeout: 5000 });
      await page.waitForTimeout(1000);
      if (level === 'full' && t === 'burn') seen.push(await page.evaluate(() => window.__anims.map((a) => a.on + ' ' + a.keys.join(' ')).join('\n')));
      assert.deepEqual(await running(page, '#dkw-bigcard'), [], `${t} at ${level} left an animation running`);
      assert.equal(await page.locator('#dkw-bigcard .dk-back, #dkw-bigcard .dk-turner, #dkw-bigcard .dk-fx, #dkw-bigcard .dk-plain').count(), 0, `${t} at ${level} left parts behind`);
      assert.equal(await page.locator('#dkw-bigcard .dk-plate').count(), 1, `${t} at ${level} ended without the face`);
    }
  }
  assert.match(seen.join('\n'), /--dk-r/, 'burn did not play its own effect');
  assert.deepEqual(errors, []);
  void owner;
});

test('sheen and foil move only while the pointer rests on the Look tab, and not on Calm or Off', async () => {
  const { owner, cid, page } = await director();
  await makeDeck(page, cid, 'Finish deck', 'Plain');
  await addCard(page, 'Sample');
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('[data-row=finish]').waitFor();
  const opt = page.locator('[data-row=finish] [data-value=sheen]');
  await opt.click();
  await page.waitForFunction(() => document.querySelector('[data-row=finish] [data-value=sheen]')?.getAttribute('aria-checked') === 'true');
  const big = page.locator('#dkw-bigcard .dk-card').first();
  const sweep = () => page.evaluate(() => document.getAnimations().filter((a) => a.animationName === 'dk-sheen').length);
  await page.mouse.move(0, 0);
  await setLevel(page, 'full');
  assert.equal(await sweep(), 0, 'it moves with no pointer on it');
  await big.hover();
  await page.waitForTimeout(200);
  assert.ok((await sweep()) >= 1, 'it did not move under the pointer at Full');
  await page.mouse.move(0, 0);
  for (const level of ['calm', 'off']) {
    await setLevel(page, level);
    await big.hover();
    await page.waitForTimeout(200);
    assert.equal(await sweep(), 0, `it moved at ${level}`);
    await page.mouse.move(0, 0);
  }
  void owner;
  void api;
});

const pickBorder = async (page, nth = 0) => {
  const opt = page.locator('[data-row=border] [role=radio]:not([aria-checked=true]):not([aria-disabled=true]):not([hidden])').nth(nth);
  const v = await opt.getAttribute('data-value');
  await opt.click();
  return v;
};

test('a save refused as stale is said so, shows the other page, and the next change saves', async () => {
  const { owner, cid, page } = await director();
  await makeDeck(page, cid, 'Two pages', 'Plain');
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('[data-row=border]').waitFor();
  const other = await page.context().newPage();
  await other.goto(page.url());
  await other.getByRole('tab', { name: 'Look' }).click();
  await other.locator('[data-row=border]').waitFor();
  const fromOther = await pickBorder(other, 0);
  await other.waitForFunction((v) => document.querySelector(`[data-row=border] [data-value="${v}"]`)?.getAttribute('aria-checked') === 'true', fromOther);
  await other.locator('[data-dkw-saved].is-on').waitFor();

  // The first page still holds the old version: its change is refused, in words, and it shows the other page's value.
  const mine = await pickBorder(page, 1);
  assert.notEqual(mine, fromOther);
  await page.locator('#dkw-panel [data-dkw-err]:not([hidden])').first().waitFor();
  assert.match(await page.locator('#dkw-panel [data-dkw-err]:not([hidden])').first().textContent(), /changed somewhere else/i);
  await page.waitForFunction((v) => document.querySelector(`[data-row=border] [data-value="${v}"]`)?.getAttribute('aria-checked') === 'true', fromOther);

  // The reload gave it the new version, so the next change goes through.
  const next = page.locator('[data-row=turn] [role=radio]:not([aria-checked=true]):not([aria-disabled=true]):not([hidden])').first();
  const turn = await next.getAttribute('data-value');
  await next.click();
  await page.waitForFunction((v) => document.querySelector(`[data-row=turn] [data-value="${v}"]`)?.getAttribute('aria-checked') === 'true', turn);
  await page.locator('[data-dkw-saved].is-on').waitFor();
  await page.reload();
  await page.locator('[data-row=turn]').waitFor();
  assert.equal(await page.locator(`[data-row=turn] [data-value="${turn}"]`).getAttribute('aria-checked'), 'true', 'the change after the refusal was not saved');
  assert.equal(await page.locator(`[data-row=border] [data-value="${fromOther}"]`).getAttribute('aria-checked'), 'true');
  void owner;
});

test('kinds are picked with dots and glyph buttons, and a deck with none says so', async () => {
  const { owner, cid, page, errors } = await director();
  await makeDeck(page, cid, 'Kinds deck', 'Plain');
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('[data-row=border]').waitFor();
  assert.match(await page.locator('.dkw-nokinds').textContent(), /No kinds yet/);
  assert.equal(await page.locator('.dkw-kindbtns').count(), 0);
  await page.locator('.dkw-nokinds [data-act=toggle-kinds]').click();
  await page.locator('#dkw-kinds').waitFor({ state: 'visible' });
  await page.getByRole('button', { name: 'Add a kind' }).click();
  await page.locator('.dkw-kindrow').waitFor();
  const row = page.locator('.dkw-kindrow').first();
  assert.equal(await row.locator('[data-kind-colour] [role=radio]').count(), 10);
  assert.equal(await row.locator('[data-kind-sign] [role=radio]').count(), 8);
  assert.equal(await row.locator('select').count(), 0, 'colour and sign are not dropdowns');
  await row.locator('[data-kind-colour] [aria-label=rose]').click();
  await page.locator('.dkw-kindrow [data-kind-colour] [aria-label=rose][aria-checked=true]').waitFor();
  assert.equal(await page.evaluate(() => document.activeElement.getAttribute('aria-label')), 'rose', 'focus left the colour');
  await row.locator('[data-kind-sign] [aria-label=diamond]').click();
  await page.locator('.dkw-kindrow [data-kind-sign] [aria-label=diamond][aria-checked=true]').waitFor();
  await page.reload();
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('.dkw-kindbtns').waitFor();
  assert.equal(await page.locator('.dkw-kindbtns .dkw-chip').count(), 1);
  assert.deepEqual(errors, []);
  void owner;
});

test('a style asks first only when the look has been changed since it, and says what stays', async () => {
  const { owner, cid, page } = await director();
  await makeDeck(page, cid, 'Style deck', 'Plain');
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('[data-row=border]').waitFor();
  // Still a style's own look: a press applies at once.
  await page.locator('[data-act=apply-style][data-style=iron-omens]').click();
  await page.waitForFunction(() => document.querySelector('[data-dkw-stylenote]')?.textContent.includes('Iron omens'));
  assert.equal(await page.locator('[data-dkw-stylenote]').getAttribute('data-changed'), 'false');
  // After an edit it asks, naming what is replaced and what stays.
  await pickBorder(page, 0);
  await page.waitForFunction(() => document.querySelector('[data-dkw-stylenote]')?.getAttribute('data-changed') === 'true');
  const before = await page.locator('[data-row=border] [aria-checked=true]').getAttribute('data-value');
  await page.locator('[data-act=apply-style][data-style=tavern-game]').click();
  const slip = page.getByText(/Your kinds and the words on the back stay/);
  await slip.waitFor();
  assert.equal(await page.locator('[data-row=border] [aria-checked=true]').getAttribute('data-value'), before, 'it applied before asking');
  await page.getByRole('button', { name: 'Replace them' }).click();
  await page.waitForFunction((b) => document.querySelector('[data-row=border] [aria-checked=true]')?.getAttribute('data-value') !== b, before);
  void owner;
});

test('on a desktop the Look stage sticks, fits the screen, and its leaf spans it whole', async () => {
  const { owner, cid, page, errors } = await director();
  await page.setViewportSize({ width: 1280, height: 800 });
  await makeDeck(page, cid, 'Desk deck', 'Plain');
  await addCard(page, 'One');
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('[data-row=border]').waitFor();
  const stage = page.locator('.dkw-stagecol');
  const box = await stage.boundingBox();
  assert.ok(box.height < 800, `the stage is ${box.height}px, taller than the screen`);
  assert.ok(box.width >= 380, `the stage column is ${box.width}px, not about 24rem`);
  const leaf = page.locator('.dkw-stagecol .dk-leaf');
  const lb = await leaf.boundingBox();
  assert.ok(lb.width >= box.width - 40, `the leaf is ${lb.width}px in a ${box.width}px stage`);
  assert.equal(await leaf.evaluate((n) => getComputedStyle(n).borderBottomWidth), '1px', 'the leaf lost its bottom border');
  await page.evaluate(() => {
    for (let a = document.querySelector('.dkw-stagecol').parentElement; a; a = a.parentElement) {
      if (a.scrollHeight > a.clientHeight && /auto|scroll/.test(getComputedStyle(a).overflowY)) { a.style.scrollBehavior = 'auto'; a.scrollTop = 1200; }
    }
  });
  await page.waitForTimeout(400);
  const top = (await stage.boundingBox()).y;
  assert.ok(top > 0 && top + box.height <= 800, `the stage did not stick (top ${top})`);
  assert.deepEqual(errors, []);
  void owner;
});

test('on a phone the table row is compact, the card stays at the top of Look, inputs are 44 tall and Delete is last', async () => {
  const { owner, cid, page, errors } = await director();
  await page.setViewportSize({ width: 390, height: 800 });
  await makeDeck(page, cid, 'Phone deck', 'Plain');
  for (const n of ['One', 'Two']) await addCard(page, n);
  await page.getByRole('button', { name: 'Draw', exact: true }).click();
  await page.locator('.dkw-shownone').first().waitFor();
  const table = await page.locator('#dkw-table').boundingBox();
  assert.ok(table.height < 800, `the table row is ${table.height}px tall on a phone, more than a screen`);
  const hs = await page.locator('.dkw-line:visible, .dkw-title').evaluateAll((ns) => ns.map((n) => n.getBoundingClientRect().height));
  assert.ok(hs.every((h) => h >= 43.5), `an input is shorter than 44: ${hs}`);
  // Kinds so the buttons show.
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('[data-row=border]').waitFor();
  await page.locator('[data-act=toggle-kinds]').last().click();
  await page.getByRole('button', { name: 'Add a kind' }).click();
  await page.locator('.dkw-kindrow').waitFor();
  await page.locator('[data-dkw-kindlist] [data-kind-name]').first().blur();
  await page.reload();
  await page.getByRole('tab', { name: 'Look' }).click();
  await page.locator('.dkw-kindbtns').waitFor();
  assert.ok(await page.locator('.dkw-kindbtns .dkw-chip').first().isVisible(), 'the kind buttons are hidden on a phone');
  // The stage sticks to the top while the rows scroll, and is under 45% of the screen.
  const stage = page.locator('.dkw-stagecol');
  const h = (await stage.boundingBox()).height;
  assert.ok(h <= 800 * 0.45, `the sticky stage is ${h}px of 800`);
  await page.evaluate(() => {
    // Whatever scrolls the page (the window or an app container above the stage).
    for (let a = document.querySelector('.dkw-stagecol').parentElement; a; a = a.parentElement) {
      if (a.scrollHeight > a.clientHeight && /auto|scroll/.test(getComputedStyle(a).overflowY)) a.scrollTop = 99999;
    }
    window.scrollTo(0, 99999);
  });
  await page.waitForTimeout(300);
  const top = (await stage.boundingBox()).y;
  assert.ok(top >= -1 && top < 120, `the stage scrolled away (top ${top})`);
  // Delete this deck is the last thing on the page.
  const del = page.getByRole('button', { name: 'Delete this deck' });
  const dy = (await del.boundingBox()).y;
  const lastRow = await page.locator('[data-row]').last().boundingBox();
  assert.ok(dy > lastRow.y, 'Delete this deck is not at the end of the page');
  assert.deepEqual(errors, []);
  void owner;
});

// ---- New deck: copy a deck, from a rolling table ----

const JSON_HEADERS = { 'Content-Type': 'application/json' };
const post = (owner, url, body) => owner.api(url, { method: 'POST', body: JSON.stringify(body), headers: JSON_HEADERS });

// A hero deck for a hero page with two cards: one carries a printed test the
// copy can keep and a test on the deck's hero a party deck cannot hold.
async function seedSource(owner, cid, name = 'Source deck') {
  const hero = await newPage(owner, cid, 'Character', 'Orrin');
  const made = await post(owner, `/campaigns/${cid}/decks`, { name, kind: 'hero', holderEntityId: hero });
  assert.equal(made.status, 201, JSON.stringify(made.body));
  const id = made.body.id;
  const a = await post(owner, `/campaigns/${cid}/decks/${id}/cards`, { name: 'Crow speaks', words: 'It knows your name.', copies: 2, corner: 'IV',
    when: [{ test: 'say', open: true }, { test: 'layer', who: 'holder', name: 'Haunted', on: true }] });
  assert.equal(a.status, 201, JSON.stringify(a.body));
  const b = await post(owner, `/campaigns/${cid}/decks/${id}/cards`, { name: 'Quiet road', copies: 1 });
  assert.equal(b.status, 201, JSON.stringify(b.body));
  return { id, hero };
}

// The page picks light or dark when it loads, so a change of scheme in the middle of a walk sets the class the page's own script would have set.
const setScheme = async (page, scheme) => {
  await page.emulateMedia({ colorScheme: scheme });
  await page.evaluate((dark) => document.documentElement.classList.toggle('dark', dark), scheme === 'dark');
};
const startTab = (page, name) => page.getByRole('tab', { name }).click();
const deckOf = async (owner, cid, id) => (await owner.api(`/campaigns/${cid}/decks/${id}`)).body;
const idFromUrl = (page) => page.url().match(/workshop\/([0-9a-f-]{36})/)[1];

test('copy a deck: its cards and a condition arrive in the pile, and what the new deck cannot hold is said once', async () => {
  const { owner, cid, page, errors } = await director('dkw-copy');
  const { id: src } = await seedSource(owner, cid);
  await owner.go(`/campaigns/${cid}/decks/workshop/new`);
  await page.getByLabel('Name', { exact: true }).fill('Party omens');
  await page.getByRole('radio', { name: 'The party' }).click();
  await startTab(page, 'Copy a deck');
  assert.equal(await page.locator('[data-start=copy]').isVisible(), true);
  assert.equal(await page.locator('[data-start=style]').isVisible(), false);
  await page.getByRole('button', { name: 'Make the deck' }).click();
  await page.locator('[data-dkw-err]').filter({ hasText: 'Choose a deck to copy.' }).waitFor();
  await page.locator('[data-deck]').filter({ hasText: 'Source deck' }).click();
  await page.getByRole('button', { name: 'Make the deck' }).click();
  await page.waitForURL(/\/decks\/workshop\/[0-9a-f-]{36}/);
  const id = idFromUrl(page);
  assert.notEqual(id, src);

  const notice = page.locator('[data-dkw-leftoff]');
  await notice.waitFor();
  assert.match(await notice.textContent(), /1 condition was left off because the new deck can't hold it: Crow speaks\./);
  assert.equal(await page.locator('#dkw-grid').getByRole('link', { name: /^Crow speaks/ }).count(), 1);
  assert.equal(await page.locator('#dkw-grid').getByRole('link', { name: /^Quiet road/ }).count(), 1);

  const deck = await deckOf(owner, cid, id);
  assert.equal(deck.kind, 'party');
  assert.equal(deck.cards.length, 2);
  const crow = deck.cards.find((c) => c.name === 'Crow speaks');
  assert.deepEqual(crow.when.map((w) => w.test), ['say'], 'only the test the deck can hold is stored');
  assert.equal(crow.copies, 2);
  assert.equal(crow.words, 'It knows your name.');
  assert.equal(deck.counts.pile, 3, 'every copy starts in the pile');

  await page.reload();
  await page.locator('#dkw-panel').waitFor();
  assert.equal(await page.locator('[data-dkw-leftoff]').count(), 0, 'the notice goes away on the next load');
  assert.deepEqual(errors, []);
});

test('make a deck from a rolling table by keyboard alone, and read what the cards cannot do yet', async () => {
  const { owner, cid, page, errors } = await director('dkw-table');
  const put = await owner.api(`/campaigns/${cid}/roll-tables`, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify({ tables: [
    { id: 'omens', name: 'Dockside omens', entries: [
      { name: 'Gulls scatter', brief: 'Something large is below.', weight: 3 },
      { name: 'Red sky', brief: 'At dawn.', weight: 0.4 },
      { name: 'Full moon tide', brief: 'Only on a full moon.', weight: 2, when: { moon: { name: 'any', phase: 'full', within: 1 } } },
      { name: 'Never', weight: 0 },
    ] },
    { id: 'secret', name: 'Only for the Director', visibility: 'dm_only', entries: [{ name: 'Twist', weight: 1 }] },
  ] }) });
  assert.equal(put.status, 200, JSON.stringify(put.body));
  await owner.go(`/campaigns/${cid}/decks/workshop/new`);
  await page.locator('#dkw-name').focus();
  await page.keyboard.type('Dock omens');
  // To the tab strip, to "From a rolling table", into the list, down to the first table, choose it.
  await page.getByRole('tab', { name: 'A style' }).focus();
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.locator('#dkw-start-table[aria-selected=true]').waitFor();
  assert.equal(await page.locator('[data-table]').count(), 2, 'a Director sees the Director-only table too');
  assert.match(await page.locator('[data-table=omens]').textContent(), /Dockside omens\s*4 lines/);
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.getAttribute('data-table')), 'omens');
  await page.keyboard.press('Space');
  assert.equal(await page.locator('[data-table=omens]').getAttribute('aria-checked'), 'true');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');
  assert.equal(await page.locator('[data-table=omens]').getAttribute('aria-checked'), 'false', 'a group takes one choice');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('Space');
  await page.getByRole('button', { name: 'Make the deck' }).focus();
  await page.keyboard.press('Enter');
  await page.waitForURL(/\/decks\/workshop\/[0-9a-f-]{36}/);

  const notice = page.locator('[data-dkw-leftoff]');
  await notice.waitFor();
  assert.match(await notice.textContent(), /1 line only comes up on certain days\. Cards can't wait for a day yet, so that card comes up any time\./);
  const deck = await deckOf(owner, cid, idFromUrl(page));
  assert.deepEqual(deck.cards.map((c) => [c.name, c.copies]).sort(), [['Full moon tide', 2], ['Gulls scatter', 3], ['Red sky', 1]]);
  assert.ok(deck.cards.every((c) => !c.when || c.when.length === 0), 'no day condition was stored');
  assert.equal(deck.counts.pile, 6);
  assert.deepEqual(errors, []);
});

test('the New deck tabs draw in light, dark and at 390 wide, empty and full, with no sideways scroll or error', async () => {
  const { owner, cid, page, errors } = await director('dkw-newtabs');
  const modes = [['light', 'light', 1280], ['dark', 'dark', 1280], ['phone', 'light', 390]];
  const tabs = [['style', 'A style'], ['copy', 'Copy a deck'], ['table', 'From a rolling table']];
  const walk = async (label) => {
    for (const [mode, scheme, width] of modes) {
      await page.setViewportSize({ width, height: 900 });
      await setScheme(page, scheme);
      for (const [key, name] of tabs) {
        await startTab(page, name);
        await page.locator(`[data-start=${key}]`).waitFor({ state: 'visible' });
        await page.waitForTimeout(150);
        await noSideways(page, `${label} ${key} ${mode}`);
        if (label === 'empty' && key !== 'style') {
          const line = key === 'copy' ? 'No decks to copy yet.' : 'No rolling tables yet.';
          assert.equal(await page.locator(`[data-start=${key}]`).getByText(line).count(), 1, `${key} has no quiet line`);
        }
        if (width === 390) {
          const small = await page.evaluate(() => [...document.querySelectorAll('.dkw-tab, .dkw-choice')].filter((n) => n.getBoundingClientRect().height > 0 && n.getBoundingClientRect().height < 43).length);
          assert.equal(small, 0, `${label} ${key}: a control is under 44 tall on a phone`);
        }
        if (SHOTS) await page.screenshot({ path: `${SHOTS}/new-${key}-${label}-${mode}.png`, fullPage: true });
      }
    }
  };
  await owner.go(`/campaigns/${cid}/decks/workshop/new`);
  await walk('empty');
  const { id: src } = await seedSource(owner, cid);
  await owner.api(`/campaigns/${cid}/roll-tables`, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify({ tables: [
    { id: 'omens', name: 'Dockside omens', entries: [{ name: 'Gulls', weight: 1 }, { name: 'Tide', weight: 2, when: { moon: { name: 'any', phase: 'full', within: 1 } } }] },
    { id: 'rumours', name: 'Rumours of the road with a rather long name to see how it wraps', entries: [{ name: 'A', weight: 1 }] },
  ] }) });
  await owner.go(`/campaigns/${cid}/decks/workshop/new`);
  await walk('full');

  // The new deck's page with the left-off sentence, in each mode.
  await startTab(page, 'Copy a deck');
  await page.getByLabel('Name', { exact: true }).fill('Party copy');
  await page.getByRole('radio', { name: 'The party' }).click();
  await page.locator(`[data-deck="${src}"]`).click();
  await page.getByRole('button', { name: 'Make the deck' }).click();
  await page.waitForURL(/\/decks\/workshop\/[0-9a-f-]{36}/);
  await page.locator('[data-dkw-leftoff]').waitFor();
  for (const [mode, scheme, width] of modes) {
    await page.setViewportSize({ width, height: 900 });
    await setScheme(page, scheme);
    await page.waitForTimeout(150);
    await noSideways(page, `left-off ${mode}`);
    if (SHOTS) await page.screenshot({ path: `${SHOTS}/new-leftoff-${mode}.png`, fullPage: true });
  }
  assert.deepEqual(errors, []);
});
