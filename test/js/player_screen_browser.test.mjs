// player_screen_browser.test.mjs — a player's die, T and R work the docked
// players' screen (dm_screen.js docking the dmscreen plugin's fragment)
// instead of the Table's drawer, against the real table.js and dm_screen.js
// in a real Chromium. The Rolls card is drawn by the Table, so its roller is
// the one R focuses. On a phone-sized window the die keeps the sheet and the
// dock is never loaded.
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
const tableJS = readFileSync(join(root, 'internal/plugins/rolls/static/js/table.js'));
const leafJS = readFileSync(join(root, 'internal/plugins/rolls/static/js/table_leaf.js'));
const screenJS = readFileSync(join(root, 'static/js/widgets/dm_screen.js'));

// The fragment's shape as the dmscreen plugin's PlayerPanel draws it, cut to
// two cards: the hero, and Rolls as a box the Table draws into.
const card = (id, title, body) => `<section class="dms-sec" data-dms-sec="${id}" aria-labelledby="dms-h-${id}" hidden>
<header class="dms-sec-h"><button type="button" class="dms-sec-t" data-dms-fold aria-expanded="true"><span id="dms-h-${id}">${title}</span></button>
<button type="button" class="dms-ic dms-sec-x" data-dms-shut aria-label="Close ${title}">x</button></header>
<div class="dms-sec-b">${body}</div></section>`;
// With Card decks on, a Hand card follows Rolls, as PlayerView.SectionsIn adds it.
const panel = (decks) => `<div class="dms" data-dms-root data-pls role="complementary">
<nav class="dms-rail"><button type="button" class="dms-ic" data-dms-tuck aria-label="Tuck the players' screen away">&gt;</button>
<button type="button" class="dms-ic" data-dms-open="hero" aria-pressed="false" aria-label="My hero">h</button>
<button type="button" class="dms-ic" data-dms-open="rolls" aria-pressed="false" aria-label="Rolls">r</button>${decks ? '<button type="button" class="dms-ic" data-dms-open="hand" aria-pressed="false" aria-label="Hand">c</button>' : ''}</nav>
<div class="dms-col" data-dms-col><div class="dms-colhead"><span data-dms-count></span><button type="button" data-dms-closeall>Close all</button></div>
<p data-dms-empty hidden>Pick a section</p><div class="dms-cards" data-dms-cards>
${card('hero', 'My hero', '<p class="pls-name">Orrin</p>')}
${card('rolls', 'Rolls', '<div class="pls-table" data-widget="table-leaf" data-tt-mount="rolls"><p>Reading the Table…</p></div>')}
${decks ? card('hand', 'Hand', '<div class="pls-table" data-widget="table-leaf" data-tt-mount="hand"><p>Reading the Table…</p></div>') : ''}
</div></div></div>`;

const page = (decks) => `<!doctype html><html><head><meta charset="utf-8"></head><body>
<header id="app-topbar"><div class="tt-host" data-widget="table" data-decks="${decks ? 'true' : 'false'}" data-campaign-id="c1" data-user-id="u1"><button type="button" class="tt-tab" data-tt-tab aria-expanded="false">Table<span class="hdr-n" data-tt-count hidden></span></button></div>
<div hidden data-pls-host data-pls-widget="dm-screen" data-dms-driven data-url="/campaigns/c1/player-screen" data-dms-store="chronicle.pls." data-dms-name="players' screen" data-dms-tab-label="Show your screen" data-dms-tab-icon="fa-dice-d20" data-dms-default-open="hero,rolls" data-dms-default-lines="hero" data-dms-lazy="true"></div></header>
<input id="field"><main id="main">page</main>
<script>
window.__panelFetches = 0;
window.Chronicle = (function () {
  var defs = {};
  function mountOne(el) {
    var d = defs[el.getAttribute('data-widget')];
    if (d && !el.__mounted) { el.__mounted = true; d.init(el); }
  }
  return {
    tableDice: new Proxy({}, { get: (_, k) => (k === 'motion' ? () => 'off' : () => '') }),
    escapeHtml: (v) => String(v), motionMs: (n, f) => f, reducedMotion: () => true,
    apiFetch: (url) => {
      if (String(url).indexOf('/player-screen') >= 0) {
        window.__panelFetches++;
        return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(${JSON.stringify(panel(decks))}) });
      }
      return Promise.resolve({ ok: false, status: 404, json: () => Promise.resolve(null), text: () => Promise.resolve('') });
    },
    register: (n, d) => { defs[n] = d; document.querySelectorAll('[data-widget="' + n + '"]').forEach(mountOne); },
    mountWidget: mountOne,
    mountWidgets: (r) => r.querySelectorAll('[data-widget]').forEach(mountOne),
  };
})();
</script><script src="/dm_screen.js"></script><script src="/table.js"></script><script src="/table_leaf.js"></script></body></html>`;

async function open(t, viewport, decks) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport: viewport || { width: 1280, height: 800 } });
  const pg = await ctx.newPage();
  const errors = [];
  pg.on('pageerror', (e) => errors.push(String(e) + ' ' + e.stack));
  t.after(() => assert.deepEqual(errors, [], 'no script errors'));
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/table.js') return route.fulfill({ contentType: 'text/javascript', body: tableJS });
    if (p === '/table_leaf.js') return route.fulfill({ contentType: 'text/javascript', body: leafJS });
    if (p === '/dm_screen.js') return route.fulfill({ contentType: 'text/javascript', body: screenJS });
    return route.fulfill({ contentType: 'text/html', body: page(decks) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForFunction(() => document.querySelector('.tt-host')._table);
  return pg;
}

const state = (pg) => pg.evaluate(() => {
  const dock = document.querySelector('.dms-dock');
  return {
    docked: document.documentElement.classList.contains('dms-docked'),
    dockShown: !!dock && !dock.hidden,
    drawer: !!document.getElementById('tt-drawer'),
    expanded: document.querySelector('[data-tt-tab]').getAttribute('aria-expanded'),
    fetches: window.__panelFetches,
  };
});

test('the die docks the players\' screen, T tucks it, and the drawer never opens', { skip }, async (t) => {
  const pg = await open(t);
  let s = await state(pg);
  assert.equal(s.fetches, 0, 'the screen is not fetched until it is asked for');
  assert.equal(s.docked, false);

  await pg.click('[data-tt-tab]');
  await pg.waitForFunction(() => document.querySelector('[data-dms-sec="hero"]') && !document.querySelector('[data-dms-sec="hero"]').hidden);
  s = await state(pg);
  assert.equal(s.docked, true, 'the page makes room for the dock');
  assert.equal(s.dockShown, true, 'the dock is out');
  assert.equal(s.drawer, false, 'the Table drawer stays shut');
  assert.equal(s.expanded, 'true', 'the die says the screen is out');
  assert.equal(s.fetches, 1);

  // The Rolls card is drawn by the Table itself.
  assert.ok(await pg.evaluate(() => document.querySelector('[data-tt-mount="rolls"]').classList.contains('tt-mount')), 'the Table drew the Rolls card');

  await pg.click('#main');
  await pg.keyboard.press('t');
  await pg.waitForFunction(() => !document.documentElement.classList.contains('dms-docked'));
  s = await state(pg);
  assert.equal(s.dockShown, false, 'T tucks the screen');
  assert.equal(s.expanded, 'false');
  assert.equal(s.drawer, false);

  await pg.keyboard.press('t');
  await pg.waitForFunction(() => document.documentElement.classList.contains('dms-docked'));
  assert.equal((await state(pg)).fetches, 1, 'bringing it back does not fetch again');
});

test('R opens Rolls on the screen with the roller in focus', { skip }, async (t) => {
  const pg = await open(t);
  await pg.click('#main');
  await pg.keyboard.press('r');
  await pg.waitForFunction(() => document.activeElement && document.activeElement.id === 'tt-fx');
  const s = await state(pg);
  assert.equal(s.docked, true);
  assert.equal(s.drawer, false);
  assert.ok(await pg.evaluate(() => !!document.activeElement.closest('[data-dms-sec="rolls"]')), 'the focused roller is in the Rolls card');
});

test('a typed letter never works the screen', { skip }, async (t) => {
  const pg = await open(t);
  await pg.focus('#field');
  await pg.keyboard.type('tr');
  assert.equal(await pg.inputValue('#field'), 'tr');
  assert.equal((await state(pg)).docked, false);
});

test('on a phone the die keeps the sheet and the dock is never loaded', { skip }, async (t) => {
  const pg = await open(t, { width: 390, height: 800 });
  await pg.click('[data-tt-tab]');
  await pg.waitForTimeout(150);
  const s = await state(pg);
  assert.equal(s.docked, false);
  assert.equal(s.fetches, 0);
  assert.equal(await pg.evaluate(() => document.querySelector('[data-pls-host]').hasAttribute('data-widget')), false, 'the dock script is never asked for');
});

test('with Card decks on, the bell\'s Hand link opens the Hand card on the screen', { skip }, async (t) => {
  const pg = await open(t, null, true);
  assert.equal(await pg.getAttribute('[data-tt-tab]', 'aria-controls'), null, 'the die does not claim the drawer');
  await pg.evaluate(() => document.querySelector('.tt-host')._table.openTable('hand'));
  await pg.waitForFunction(() => {
    const sec = document.querySelector('[data-dms-sec="hand"]');
    return sec && !sec.hidden && sec.querySelector('[data-tt-mount="hand"].tt-mount [data-tt-hand]');
  });
  const s = await state(pg);
  assert.equal(s.docked, true, 'the screen is out');
  assert.equal(s.drawer, false, 'the drawer stays shut');
});
