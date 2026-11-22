// deck_cards_browser.test.mjs: the motion script in a real browser, on a card
// written the way card.templ writes it. A face that fails to load, even after
// the halfway wait while a second half is running, must leave the card face
// down with nothing inline, nothing animating and the button back with focus;
// and a move on a card that is mid-turn must finish the turn, not strip it.

import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { writeFileSync, mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const TURNS = ['flip', 'slow', 'toss', 'slam', 'sweep', 'burn', 'ink', 'seal', 'bolt', 'forge'];
let browser, file;

const card = (turn) => `<div class="dk-wrap dk-sz-row dk-s-tall dk-t-${turn}" id="c" data-dk-card data-turn="${turn}" data-shape="tall" data-state="down">
<span class="dk-turner"><div class="dk-card dk-back dk-s-tall dk-p-lattice dk-pal-indigo" aria-hidden="true"><span class="dk-bfr"><span class="dk-bw">Deck</span></span></div><span class="dk-faceslot"></span></span>
<button type="button" class="dk-turn" aria-label="Turn over card 1 of 1, Deck"></button>
<span class="dk-sr dk-live" role="status" aria-live="polite"></span></div>`;
const FACE = '<div class="dk-card dk-plate dk-s-tall" aria-label="Sample card, Kind"><b class="dk-name">Sample card</b></div>';

before(() => {
  if (skip) return;
  const dir = mkdtempSync(join(tmpdir(), 'dkcards-'));
  file = join(dir, 'page.html');
  writeFileSync(file, `<!doctype html><meta charset="utf-8"><link rel="stylesheet" href="file://${root}/static/css/decks_card.css">
<body style="padding:30px">__CARD__<script src="file://${root}/internal/plugins/decks/static/js/deck_cards.js"></script>`);
});
after(async () => { if (browser) await browser.close(); });

async function open(turn) {
  browser = browser || await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({ reducedMotion: 'no-preference' });
  const page = await ctx.newPage();
  await page.addInitScript((face) => { window.FACE_HTML = face; Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => 8 }); }, FACE);
  await page.route('**/page.html', (r) => r.fulfill({ contentType: 'text/html', body: require_html(turn) }));
  await page.goto('file:///x/page.html').catch(() => {});
  return page;
}
// The page is served from memory so the card can change per test.
import { readFileSync } from 'node:fs';
const require_html = (turn) => readFileSync(file, 'utf8').replace('__CARD__', card(turn));

test('a face that fails after the halfway wait leaves the card face down and clean, for every turn-over', { skip }, async () => {
  for (const turn of TURNS) {
    const page = await open(turn);
    await page.evaluate(() => DeckCards.turnOver(document.getElementById('c'), () => new Promise((_, rej) => setTimeout(rej, 700))));
    const r = await page.evaluate(() => {
      const c = document.getElementById('c');
      const back = c.querySelector('.dk-back');
      const btn = c.querySelector('.dk-turn');
      return {
        state: c.getAttribute('data-state'),
        opacity: getComputedStyle(back).opacity,
        filter: getComputedStyle(back).filter,
        mask: getComputedStyle(c.querySelector('.dk-turner')).maskImage,
        inline: [c.querySelector('.dk-turner'), back, c.querySelector('.dk-faceslot')].map((n) => n.getAttribute('style') || ''),
        running: document.getAnimations().length,
        fx: c.querySelectorAll('.dk-fx').length,
        slot: c.querySelector('.dk-faceslot').childNodes.length,
        live: c.querySelector('.dk-live').textContent,
        enabled: !btn.disabled,
        focused: document.activeElement === btn,
        classes: c.querySelector('.dk-turner').className,
      };
    });
    await page.waitForTimeout(600); // an effect started late would show here
    const later = await page.evaluate(() => ({ running: document.getAnimations().length, fx: document.querySelectorAll('.dk-fx').length }));
    const w = `${turn}: ${JSON.stringify(r)}`;
    assert.equal(r.state, 'down', w);
    assert.equal(r.opacity, '1', w);
    assert.equal(r.filter, 'none', w);
    assert.deepEqual(r.inline, ['', '', ''], w);
    assert.equal(r.running, 0, w);
    assert.equal(r.fx, 0, w);
    assert.equal(r.slot, 0, w);
    assert.equal(r.live, 'That card could not be turned over. Try again.', w);
    assert.ok(r.enabled && r.focused, w);
    assert.equal(r.classes, 'dk-turner', w);
    assert.deepEqual(later, { running: 0, fx: 0 }, turn + ' moved after failing');
    await page.context().close();
  }
});

test('a move on a card that is mid-turn finishes the turn first', { skip }, async () => {
  const page = await open('seal');
  const r = await page.evaluate(async () => {
    const c = document.getElementById('c');
    DeckCards.turnOver(c, () => new Promise((res) => setTimeout(() => res(FACE_HTML), 150)));
    await new Promise((res) => setTimeout(res, 60)); // the turn is under way
    await DeckCards.move(c, 'used', {});
    return {
      state: c.getAttribute('data-state'),
      plate: c.querySelectorAll('.dk-plate').length,
      parts: c.querySelectorAll('.dk-back, .dk-turner, .dk-fx, .dk-plain').length,
      spent: c.classList.contains('dk-spent'),
      running: document.getAnimations().length,
    };
  }).catch((e) => ({ error: String(e) }));
  assert.equal(r.error, undefined);
  assert.deepEqual(r, { state: 'up', plate: 1, parts: 0, spent: true, running: 0 });
  await page.context().close();
});
