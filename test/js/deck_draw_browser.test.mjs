// deck_draw_browser.test.mjs: the card draw overlay
// (internal/plugins/decks/static/js/deck_draw.js), the DM Screen's Card decks
// section, the Hand's Draw button and the dealt-card pop-up, in a real browser.
// The real scripts and stylesheets run (table.js loads deck_draw.js lazily the
// way a page does) against a small fake server written to the contract's routes.
// It pins: the fan is the stage before the first pull; a pull reacts within
// 100ms; the card stays face down and shows nothing of itself until the person
// turns it; Draw another sets the card aside into the drawn row and lifts the
// fan back to the stage; put back, the drawn row, refusals in the server's
// words, offline, Calm and Off, phone targets, the section's rows, Quick draw,
// the card menu and Choose decks, and the pop-up that comes once.

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(root, p), 'utf8');
const FILES = {
  '/static/plugins/decks/js/deck_draw.js': read('internal/plugins/decks/static/js/deck_draw.js'),
  '/static/plugins/decks/js/deck_cards.js': read('internal/plugins/decks/static/js/deck_cards.js'),
  '/js/deck_hand.js': read('internal/plugins/decks/static/js/deck_hand.js'),
  '/js/table.js': read('internal/plugins/rolls/static/js/table.js'),
  '/js/dm_screen.js': read('static/js/widgets/dm_screen.js'),
  '/static/css/decks_card.css': read('static/css/decks_card.css'),
  '/css/decks_hand.css': read('static/css/decks_hand.css'),
  '/static/css/decks_draw.css': read('static/css/decks_draw.css'),
  '/css/table.css': read('internal/plugins/rolls/static/css/table.css'),
};
let browser;
after(async () => { if (browser) await browser.close(); });

const TOKENS = `:root { --color-text-primary: #1b1b1f; --color-text-secondary: #444; --color-text-muted: #666; --color-text-body: #222; --color-card-bg: #fff; --color-bg-secondary: #f3f3f5; --color-bg-tertiary: #e6e6ea; --color-border: #c9c9d1; --color-accent: #2563eb; --color-accent-hover: #1d4ed8; --color-accent-link: #1d4ed8;
  --surface-overlay: rgb(6 7 11 / .55); --surface-text: #1b1b1f; --surface-text-muted: #555; --surface-raised: #f1f1f4; --surface-border: #d4d4da; --z-menu-lift: 70;
  --dur-instant: 80ms; --dur-micro: 120ms; --dur-everyday: 120ms; --dur-standard: 200ms; --dur-slide: 360ms; --ease-out: cubic-bezier(0.16, 1, 0.3, 1); --ease-in: cubic-bezier(0.4, 0, 1, 1); --ease-in-out: cubic-bezier(0.4, 0, 0.2, 1);
  --shape-r-ctl: 8px; --shape-r-panel: 12px; --shape-edge: 1px; --shape-rule: 1px; --elev-resting: 0 1px 2px rgb(0 0 0 / .12); --elev-hover: 0 4px 10px rgb(0 0 0 / .18); --elev-dragged: 0 12px 28px rgb(0 0 0 / .3); }
  *, *::before, *::after { box-sizing: border-box; } body { margin: 0; font: 16px/1.4 system-ui, sans-serif; color: var(--color-text-primary); background: #fafafa; } [hidden] { display: none !important; }
  .btn-primary { background: var(--color-accent); color: #fff; border: 0; cursor: pointer; } body > header { display: flex; justify-content: flex-end; }`;

// The fake server. It answers the contract's routes from a small state and
// records every call, so a test can say what was and was not asked.
const SERVER = `(function () {
  var NAMES = ['The Tide Withdraws','The Lantern Keeps Burning','The Drowned Bell Rings','Gulls Circle the Chapel','Salt in the Well','Red Sails at Dawn','A Kind Word from a Smuggler','A Stranger Pays in Old Coin','Fog Hides the Road'];
  var BACK = '<div class="dk-card dk-back dk-s-tall dk-sz-hand" aria-hidden="true"><span class="dk-bfr"><span class="dk-bw">OMENS</span></span></div>';
  function mk(name, who, n, shape) { var pile = []; for (var i = 1; i <= n; i++) pile.push(name[0].toLowerCase() + i); return { name: name, who: who, shape: shape || 'tall', turn: 'flip', pile: pile, shown: [], drawsLeft: 3 }; }
  var srv = window.__srv = { latency: 30, offline: false, viewer: 'director', calls: [], refuse: '', seen: {}, fresh: [], picks: null, handN: 0, held: [],
    decks: { d1: mk('Omens', 'Events', 9), d2: mk('Fates', 'The party', 7), d3: mk('Boons', 'Brannoc', 6, 'standard') } };
  function nameOf(id) { return NAMES[(parseInt(id.slice(1), 10) - 1) % NAMES.length]; }
  function plate(id, size) { return '<div class="dk-card dk-plate dk-s-tall dk-sz-' + size + '" aria-label="' + nameOf(id) + ', Omen"><b class="dk-name">' + nameOf(id) + '</b></div>'; }
  function leaf(id) { return '<div class="dk-leaf"><span class="dk-lfi"><b class="dk-dn">' + nameOf(id) + '</b><span class="dk-words">The sea pulls back from the stones.</span></span></div>'; }
  function face(id) { return plate(id, 'open') + leaf(id); }
  function shownHtml(id) { return '<div class="dk-wrap dk-sz-open dk-s-tall dk-open" data-dk-card data-widget="deck-cards" data-state="up">' + face(id) + '</div>'; }
  function compact(id) { return '<div class="dk-wrap dk-sz-row dk-s-tall" data-dk-card data-widget="deck-cards" data-state="up">' + plate(id, 'row') + '</div>'; }
  function dir() { return srv.viewer === 'director'; }
  function view(did) {
    var d = srv.decks[did], refusal = '', can = true;
    if (!d.pile.length) { refusal = 'nothing left in this deck'; can = false; }
    else if (dir() && d.shown.length >= 5) { refusal = 'five cards are already drawn. Use one or put one back first'; can = false; }
    else if (!dir() && d.drawsLeft <= 0) { refusal = 'no draws left'; can = false; }
    return { deckId: did, name: d.name, sub: d.who + ' · ' + d.pile.length + ' left', shape: d.shape, turn: d.turn, pile: d.pile.length, fan: Math.min(7, d.pile.length), backHtml: BACK, canDraw: can, refusal: refusal,
      drawsLeft: dir() ? null : d.drawsLeft, heroId: dir() ? '' : 'h1', shown: dir() ? d.shown.map(function (id) { return { copyId: id, html: shownHtml(id) }; }) : [] };
  }
  function row(did) {
    var d = srv.decks[did], n = d.shown.length;
    return '<div class="dkd-deck" data-dkd-deck="' + did + '" data-name="' + d.name + '"><button type="button" data-dkd-row aria-expanded="false" aria-controls="m-' + did + '"><span class="dkd-back" aria-hidden="true"><div class="dk-card dk-back dk-s-tall dk-sz-row"></div></span><span><span class="dkd-name">' + d.name + '</span><span data-dkd-sub>' + d.who + ' · ' + d.pile.length + ' left' + (n ? ' · ' + n + ' drawn' : '') + '</span></span><svg class="dkd-chev" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6"></path></svg></button>'
      + '<div id="m-' + did + '" data-dkd-acts hidden><button type="button" class="btn-primary dkd-btn" data-dkd-draw>Draw</button><button type="button" class="dkd-word" data-dkd-quick>Quick draw</button></div>'
      + '<div data-dkd-shown>' + d.shown.map(function (id) { return '<button type="button" data-dkd-copy="' + id + '" aria-haspopup="menu" aria-expanded="false" aria-label="' + nameOf(id) + '. Press for its menu.">' + compact(id) + '</button>'; }).join('') + '</div><p data-dkd-err role="alert" hidden></p></div>';
  }
  function screen() {
    var all = Object.keys(srv.decks), picked = srv.picks || all.slice(0, 2), rest = all.filter(function (k) { return picked.indexOf(k) < 0; });
    return '<div data-dkd-screen data-campaign="c1">' + picked.map(row).join('')
      + (rest.length ? '<button type="button" data-dkd-more aria-expanded="false">All decks · ' + all.length + '</button><div data-dkd-rest hidden>' + rest.map(row).join('') + '</div>' : '')
      + '<button type="button" data-dkd-choose aria-expanded="false">Choose decks</button><div data-dkd-picker hidden>' + all.map(function (k) { return '<label><input type="checkbox" value="' + k + '"' + (picked.indexOf(k) >= 0 ? ' checked' : '') + '> ' + srv.decks[k].name + '</label>'; }).join('') + '<button type="button" class="btn-primary dkd-btn" data-dkd-picks-save>Save</button></div>'
      + '<template data-dkd-heroes><button data-hero="h1">Brannoc</button><button data-hero="h2">Isolde Varr</button></template></div>';
  }
  function handHtml() {
    var d = srv.decks.d1;
    return '<div class="dkh" data-widget="deck-hand" data-dkh-root data-src="/campaigns/c1/decks/hand" data-base="/campaigns/c1/decks" data-campaign-id="c1"><div class="dkh-body" data-dkh-body><section class="dkh-sec" data-dkh-draw-sec><h3 class="dkh-lab">Draw</h3>'
      + '<p data-held>' + srv.held.length + ' cards held</p><div class="dkd-pacts"><button type="button" data-dkd-open data-deck="d1" data-hero="h1" class="btn-primary dkd-btn">Draw</button><button type="button" data-dkh-draw data-deck="d1" data-hero="h1" class="dkd-word">Quick draw</button></div>'
      + '<p class="dkh-quiet">' + d.drawsLeft + ' draws left</p></section></div><div class="dkh-notes" data-dkh-notes role="status"></div><p class="dkh-err" role="alert" data-dkh-err hidden></p></div>';
  }
  srv.screenHtml = screen; srv.handHtml = handHtml;
  function route(method, url, body) {
    var m, u = url.replace(/^https?:\\/\\/[^/]+/, '');
    var J = function (o, s) { return { status: s || 200, body: JSON.stringify(o) }; };
    var bad = function (msg, s) { return J({ message: msg }, s || 409); };
    if (srv.fail && srv.fail(method, u)) return srv.failAnswer || bad('that did not work');
    if (/\\/decks\\/moments\\/panel/.test(u)) return { status: 200, body: '<div class="dkm" data-dkm data-at=""><ul data-dkm-index hidden></ul></div>' };
    if (method === 'GET' && u === '/campaigns/c1/decks/screen') return { status: 200, body: screen() };
    if (method === 'GET' && u === '/campaigns/c1/dm-screen') return { status: 200, body: '<div data-dms-root><section data-dms-section="decks">' + screen() + '</section></div>' };
    if (method === 'PUT' && u === '/campaigns/c1/decks/screen/picks') { if (srv.picksRefuse) return bad(srv.picksRefuse, 400); srv.picks = JSON.parse(body).deckIds; return { status: 204, body: '' }; }
    if (method === 'GET' && u === '/campaigns/c1/decks/hand') return { status: 200, body: handHtml() };
    if (method === 'GET' && u === '/campaigns/c1/decks/hand/fresh') return J({ cards: dir() ? [] : srv.fresh.filter(function (c) { return !srv.seen[c.copyId]; }) });
    if (method === 'POST' && (m = u.match(/\\/decks\\/hand\\/fresh\\/([^/]+)\\/seen$/))) { srv.seen[m[1]] = true; return { status: 204, body: '' }; }
    if (method === 'GET' && (m = u.match(/\\/decks\\/(d\\d)\\/draw-view$/))) { return srv.decks[m[1]] ? J(view(m[1])) : bad('not found', 404); }
    if (method === 'GET' && (m = u.match(/\\/decks\\/(d\\d)\\/copies\\/([^/]+)\\/face$/))) return { status: 200, body: face(m[2]) };
    if (method === 'POST' && (m = u.match(/\\/decks\\/hand\\/(d\\d)\\/copies\\/([^/]+)\\/turn\\?size=open$/))) return { status: 200, body: face(m[2]) };
    if (method === 'POST' && (m = u.match(/\\/decks\\/(d\\d)\\/draw$/))) {
      var d = srv.decks[m[1]], v = view(m[1]);
      if (srv.refuse) return bad(srv.refuse);
      if (!v.canDraw) return bad(v.refusal);
      var id = d.pile.shift();
      if (dir()) d.shown.push(id); else { d.drawsLeft--; srv.held.push(id); }
      return J({ copyIds: [id] });
    }
    if (method === 'POST' && (m = u.match(/\\/decks\\/(d\\d)\\/copies\\/([^/]+)\\/(show|deal|put-back|play)$/))) {
      var dd = srv.decks[m[1]], cid = m[2];
      dd.shown = dd.shown.filter(function (x) { return x !== cid; });
      srv.held = srv.held.filter(function (x) { return x !== cid; });
      if (m[3] === 'put-back') dd.pile.push(cid);
      return J({ copyId: cid });
    }
    return bad('not found', 404);
  }
  window.fetch = function (url, opts) {
    opts = opts || {};
    var method = (opts.method || 'GET').toUpperCase(), body = opts.body;
    srv.calls.push(method + ' ' + String(url).replace(/^https?:\\/\\/[^/]+/, '') + (body ? ' ' + body : ''));
    return new Promise(function (res, rej) {
      setTimeout(function () {
        if (srv.offline) return rej(new TypeError('Failed to fetch'));
        var a = route(method, String(url), body);
        res(new Response(a.status === 204 ? null : a.body, { status: a.status }));
      }, srv.latency);
    });
  };
})();`;

const PAGE = ({ viewer, heldHand = false }) => `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<style>${TOKENS}</style><link rel="stylesheet" href="/static/css/decks_card.css"><link rel="stylesheet" href="/css/decks_hand.css"><link rel="stylesheet" href="/css/table.css"></head>
<body data-dkd-src="/static/plugins/decks/js/deck_draw.js?v=t1" data-dkd-cards-src="/static/plugins/decks/js/deck_cards.js?v=t2" data-dkd-css="/static/css/decks_draw.css?v=t3">
<header><div class="tt-host" data-widget="table" data-campaign-id="c1" data-user-id="u1" data-director="${viewer === 'director'}" data-decks="true" data-gm-term="Director">
<button type="button" class="tt-tab" data-tt-tab aria-expanded="false" aria-controls="tt-drawer"><span data-tt-count hidden></span></button></div></header>
<main id="main-content"><button id="elsewhere" type="button">Elsewhere</button><div id="host"></div></main>
<script>
window.__viewer = '${viewer}';
${SERVER}
window.__srv.viewer = '${viewer}';
var widgets = {};
function respond(body) { return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(body); }, text: function () { return Promise.resolve(''); } }); }
window.WebSocket = function () { return { addEventListener: function () {}, close: function () {} }; };
window.Chronicle = {
  register: function (n, d) { widgets[n] = d; },
  mountWidgets: function (r) { Object.keys(widgets).forEach(function (n) { if (n === 'table') return; (r.querySelectorAll ? r : document).querySelectorAll('[data-widget="' + n + '"]').forEach(function (el) { if (el.__m) return; el.__m = true; widgets[n].init(el); }); }); },
  apiFetch: function (u, o) {
    o = o || {};
    if (/\\/rolls|\\/table\\/shown/.test(u)) return respond(/settings|power/.test(u) ? {} : { rolls: [], more: false });
    return fetch(u, { method: o.method, headers: o.headers, body: o.body && typeof o.body === 'object' ? JSON.stringify(o.body) : o.body });
  },
  working: function (b, on) { if (on) b.setAttribute('data-working', ''); else b.removeAttribute('data-working'); },
  escapeHtml: function (s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); },
  tableDice: { motion: function () { return 'off'; }, svg: function () { return ''; }, throwInto: function () { return Promise.resolve(); } },
  dice: { roll: function () { return Promise.resolve([]); }, catalogue: function () { return Promise.resolve({ sets: [], crits: { succ: [], fail: [] } }); } }
};
Object.defineProperty(navigator, 'hardwareConcurrency', { get: function () { return 8; } });
document.getElementById('host').innerHTML = ${viewer === 'director' ? 'window.__srv.screenHtml()' : 'window.__srv.handHtml()'};
</script>
<script src="/js/table.js"></script>
${viewer === 'player' ? '<script src="/js/deck_hand.js"></script>' : ''}
<script>
${'window.__start = function () { widgets.table.init(document.querySelector(".tt-host")); Chronicle.mountWidgets(document); };'}
</script></body></html>`;

/**
 * open: a page for a viewer. `motion` is full (the default), calm or off.
 * `srv` runs in the page before anything is asked, to set the server's state.
 */
async function open({ viewer = 'director', phone = false, motion = 'full', srv, start = true } = {}) {
  browser = browser || await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1280, height: 800 },
    reducedMotion: 'no-preference',
  });
  const page = await ctx.newPage();
  page.setDefaultTimeout(8000);
  const loaded = [];
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  await page.route('http://chronicle.test/**', (r) => {
    const path = new URL(r.request().url()).pathname;
    loaded.push(path);
    if (path === '/') return r.fulfill({ contentType: 'text/html', body: PAGE({ viewer }) });
    if (FILES[path]) return r.fulfill({ contentType: path.endsWith('.css') ? 'text/css' : 'text/javascript', body: FILES[path] });
    return r.fulfill({ status: 404, body: '' });
  });
  await page.goto('http://chronicle.test/');
  await page.evaluate((m) => { if (m !== 'full') document.documentElement.setAttribute('data-motion', m); }, motion);
  if (srv) await page.evaluate(srv);
  // The page's own markup is drawn from the state the test set.
  await page.evaluate((v) => { document.getElementById('host').innerHTML = v === 'director' ? window.__srv.screenHtml() : window.__srv.handHtml(); }, viewer);
  if (start) await page.evaluate(() => window.__start());
  page.loaded = loaded;
  page.close2 = async () => { await ctx.close(); assert.deepEqual(errors, [], 'the page threw'); };
  return page;
}
const calls = (page) => page.evaluate(() => window.__srv.calls.slice());
const drawsMade = async (page) => (await calls(page)).filter((c) => /^POST \/campaigns\/c1\/decks\/d\d\/draw /.test(c)).length;
const text = (page, sel) => page.evaluate((s) => (document.querySelector(s) || {}).textContent, sel);
// The fan has dealt out and is still: n backs (any number when n is null), focus on one, or all disabled.
const waitFan = (page, n) => page.waitForFunction((k) => {
  const backs = [...document.querySelectorAll('[data-dko-back]')];
  const calm = !document.querySelector('[data-dko-fan]').getAnimations({ subtree: true }).length;
  return (k == null || backs.length === k) && calm && (backs.every((b) => b.disabled) || document.activeElement.hasAttribute('data-dko-back'));
}, n);
const SEVEN = 'Shuffled: every back is the same draw. Pull one. 7 of 9 shown.';

/** openDirector: the Director's Draw on Omens, the overlay open and the fan dealt. */
async function openDirector(opts = {}) {
  const page = await open(opts);
  await page.waitForSelector('[data-dkd-screen] [data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-draw]');
  await page.waitForSelector('[data-dkd-ov] [data-dko-back]');
  await waitFan(page, opts.backs === undefined ? (opts.phone ? 5 : 7) : opts.backs);
  return page;
}
async function openPlayer(opts = {}) {
  const page = await open({ viewer: 'player', ...opts });
  await page.waitForSelector('[data-dkd-open]');
  await page.click('[data-dkd-open]');
  await page.waitForSelector('[data-dkd-ov] [data-dko-back]');
  await waitFan(page, opts.backs === undefined ? (opts.phone ? 5 : 7) : opts.backs);
  return page;
}
const pullMiddle = async (page) => {
  const n = await page.locator('[data-dko-back]').count();
  await page.locator('[data-dko-back]').nth(Math.floor(n / 2)).click();
};
/** pulled: a pull has finished: the face-down card is on the stage and the fan has settled. */
const pulled = (page, n = 1) => page.waitForFunction((k) => document.activeElement.hasAttribute('data-dko-cardturn') && !document.querySelector('[data-dko-fan]').getAnimations({ subtree: true }).length && document.querySelectorAll('[data-dko-row] .dko-tcard').length >= k - 1, n);
const turned = (page) => page.waitForFunction(() => document.querySelector('[data-dko-primary]') && document.querySelector('.dko-slot [data-state="up"]'));

// ---------------------------------------------------------------- Director

test('the section folds a row to Draw and Quick draw, one row at a time', { skip }, async () => {
  const page = await open();
  await page.waitForSelector('[data-dkd-row]');
  const r1 = '[data-dkd-deck="d1"] [data-dkd-row]', r2 = '[data-dkd-deck="d2"] [data-dkd-row]';
  assert.equal(await page.getAttribute(r1, 'aria-expanded'), 'false');
  assert.equal(await page.isVisible('[data-dkd-deck="d1"] [data-dkd-draw]'), false);
  assert.equal((await text(page, '[data-dkd-deck="d1"] [data-dkd-sub]')), 'Events · 9 left');
  await page.click(r1);
  await page.waitForSelector('[data-dkd-deck="d1"] [data-dkd-draw]', { state: 'visible' });
  assert.deepEqual(await page.$$eval('[data-dkd-deck="d1"] [data-dkd-acts] button', (b) => b.map((x) => x.textContent.trim())), ['Draw', 'Quick draw']);
  await page.click(r2);
  await page.waitForSelector('[data-dkd-deck="d2"] [data-dkd-quick]', { state: 'visible' });
  await page.waitForFunction(() => document.querySelector('[data-dkd-deck="d1"] [data-dkd-acts]').hidden);
  assert.equal(await page.getAttribute(r1, 'aria-expanded'), 'false');
  await page.close2();
});

test('the script loads only where the section is, and the overlay opens, traps focus and closes on Escape, not on the scrim', { skip }, async () => {
  const page = await open();
  await page.waitForFunction(() => window.DeckDraw);
  assert.ok(page.loaded.includes('/static/plugins/decks/js/deck_draw.js') && page.loaded.includes('/static/plugins/decks/js/deck_cards.js') && page.loaded.includes('/static/css/decks_draw.css'));
  await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
  const draw = '[data-dkd-deck="d1"] [data-dkd-draw]';
  await page.focus(draw);
  await page.click(draw);
  await page.waitForSelector('[data-dkd-ov] [data-dko-back]');
  const s = await page.evaluate(() => {
    const ov = document.querySelector('[data-dkd-ov]'), p = ov.querySelector('[role="dialog"]');
    return { onBody: ov.parentNode === document.body, modal: p.getAttribute('aria-modal'), title: p.querySelector('h2').textContent, sub: p.querySelector('.dkd-subhead').textContent,
      locked: document.documentElement.classList.contains('dkd-locked'), scrim: getComputedStyle(ov.querySelector('.dkd-scrim')).backgroundColor, inert: document.querySelector('main').hasAttribute('inert'), closeSize: ov.querySelector('[data-dkd-close]').getBoundingClientRect().width };
  });
  assert.deepEqual({ ...s, closeSize: Math.round(s.closeSize) }, { onBody: true, modal: 'true', title: 'Omens', sub: 'Events · 9 left', locked: true, scrim: 'rgba(6, 7, 11, 0.55)', inert: true, closeSize: 44 });
  await page.waitForFunction(() => document.querySelector('[data-dkd-ov]').contains(document.activeElement));
  // Tab never leaves the panel.
  for (let i = 0; i < 12; i++) {
    await page.keyboard.press('Tab');
    assert.equal(await page.evaluate(() => document.querySelector('[data-dkd-ov] .dkd-panel').contains(document.activeElement)), true, 'focus left the overlay');
  }
  await page.mouse.click(8, 400);
  await page.waitForTimeout(300);
  assert.ok(await page.$('[data-dkd-ov]'), 'a press on the scrim closed it');
  await page.keyboard.press('Escape');
  await page.waitForSelector('[data-dkd-ov]', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.documentElement.classList.contains('dkd-locked')), false);
  await page.waitForFunction(() => document.activeElement.hasAttribute('data-dkd-row'));
  await page.close2();
});

test('the close mark closes it and the overlay grows out of the pressed thing in about 200ms', { skip }, async () => {
  const page = await open();
  await page.waitForFunction(() => window.DeckDraw);
  await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-draw]');
  await page.waitForSelector('[data-dkd-ov]');
  const open1 = await page.evaluate(() => { const a = document.querySelector('.dkd-panel').getAnimations().map((x) => x.effect.getComputedTiming().duration); return a; });
  assert.ok(open1.length && open1.every((d) => d >= 150 && d <= 260), `the panel's open move is ${open1}`);
  await page.click('[data-dkd-close]');
  await page.waitForSelector('[data-dkd-ov]', { state: 'detached' });
  await page.close2();
});

test('before the first pull the fan is the stage: seven backs, named, and nothing drawn', { skip }, async () => {
  const page = await openDirector();
  const r = await page.evaluate(() => {
    const backs = [...document.querySelectorAll('[data-dko-back]')];
    return { n: backs.length, label3: backs[2].getAttribute('aria-label'), stage: document.querySelector('[data-dko]').hasAttribute('data-stage'), hint: document.querySelector('[data-dko-hint]').textContent, cards: document.querySelectorAll('.dko-slot .dk-wrap').length,
      centered: (() => { const f = document.querySelector('[data-dko-fan]').getBoundingClientRect(), p = document.querySelector('.dkd-panel').getBoundingClientRect(); return Math.abs((f.left + f.width / 2) - (p.left + p.width / 2)) < 4; })(),
      focused: document.activeElement.hasAttribute('data-dko-back'), tab: backs.filter((b) => b.tabIndex === 0).length };
  });
  assert.equal(r.n, 7);
  assert.equal(r.label3, 'Face-down card 3 of 7, Omens');
  assert.ok(r.stage && r.centered, 'the fan is not centred on the stage');
  assert.equal(r.hint, SEVEN);
  assert.equal(r.cards, 0);
  assert.ok(r.focused);
  assert.equal(r.tab, 1, 'the fan is one tab stop');
  assert.equal((await calls(page)).filter((c) => /POST .*\/draw/.test(c)).length, 0, 'opening drew a card');
  await page.close2();
});

test('arrow keys, Home and End move along the fan with a ring, and Enter pulls', { skip }, async () => {
  const page = await openDirector();
  const idx = () => page.evaluate(() => [...document.querySelectorAll('[data-dko-back]')].indexOf(document.activeElement));
  await page.keyboard.press('Home'); assert.equal(await idx(), 0);
  await page.keyboard.press('ArrowRight'); assert.equal(await idx(), 1);
  await page.keyboard.press('End'); assert.equal(await idx(), 6);
  await page.keyboard.press('ArrowLeft'); assert.equal(await idx(), 5);
  const ring = await page.evaluate(() => { const o = getComputedStyle(document.activeElement); return o.outlineStyle + ' ' + o.outlineWidth; });
  assert.match(ring, /solid 3px/);
  await page.keyboard.press('Enter');
  await pulled(page);
  assert.equal(await drawsMade(page), 1);
  await page.close2();
});

test('a pull reacts within 100ms, then the card rises face down and the fan settles; nothing of the card is asked for or shown', { skip }, async () => {
  const page = await openDirector({ srv: () => { window.__srv.latency = 700; } });
  const seen = await page.evaluate(() => new Promise((res) => {
    const b = document.querySelectorAll('[data-dko-back]')[3];
    b.click();
    setTimeout(() => { const cs = getComputedStyle(b); res({ working: b.hasAttribute('data-working'), ring: cs.boxShadow, lift: cs.transform }); }, 95);
  }));
  assert.equal(seen.working, true);
  assert.notEqual(seen.ring, 'none', 'no ring within 100ms');
  assert.notEqual(seen.lift, 'none');
  await pulled(page);
  const r = await page.evaluate(() => ({
    stage: document.querySelector('[data-dko]').hasAttribute('data-stage'), backs: document.querySelectorAll('[data-dko-back]').length, down: document.querySelector('.dko-slot .dk-wrap').getAttribute('data-state'),
    body: document.querySelector('[data-dkd-ov]').textContent, turnLabel: document.querySelector('[data-dko-cardturn]').getAttribute('aria-label'), line: document.querySelector('[data-dko-line]').textContent,
    inBand: !!document.querySelector('[data-dko-bandfan] [data-dko-fan]'),
  }));
  assert.equal(r.stage, false);
  assert.equal(r.inBand, true, 'the fan did not settle into the lower band');
  assert.equal(r.backs, 7, 'the fan did not refill from the pile');
  assert.equal(r.down, 'down');
  assert.doesNotMatch(r.body, /Tide|Lantern|Bell|Gulls|Salt|Sails|Smuggler|Stranger|Fog|sea pulls/);
  assert.equal(r.turnLabel, 'Turn over the card you pulled from Omens');
  assert.equal(r.line, 'Face down. Turn it over to read it.');
  assert.deepEqual((await calls(page)).filter((c) => /face|turn/.test(c)), [], 'a face was asked for before the person turned it');
  await page.close2();
});

test('the card turns only when pressed, with the deck\'s turn, then the leaf and the Director\'s actions come', { skip }, async () => {
  const page = await openDirector();
  await pullMiddle(page);
  await pulled(page);
  await page.waitForTimeout(400);
  assert.equal((await calls(page)).filter((c) => /face/.test(c)).length, 0);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  const r = await page.evaluate(() => ({ leaf: document.querySelector('[data-dko-leaf] .dk-dn').textContent, words: document.querySelector('[data-dko-leaf] .dk-words').textContent, acts: [...document.querySelectorAll('[data-dko-acts] button')].map((b) => b.textContent.trim()),
    focus: document.activeElement.textContent.trim(), plate: document.querySelector('.dko-slot .dk-plate').getAttribute('aria-label') }));
  assert.deepEqual(r.acts, ['Show the table', 'Deal to…', 'Put it back', 'Draw another']);
  assert.match(r.leaf, /The/);
  assert.match(r.words, /sea pulls back/);
  assert.equal(r.focus, 'Show the table');
  assert.match(r.plate, /, Omen$/);
  assert.equal((await calls(page)).filter((c) => /GET .*\/face$/.test(c)).length, 1);
  await page.close2();
});

test('Draw another sets the card aside into the Drawn row and lifts the fan back to the stage without drawing', { skip }, async () => {
  const page = await openDirector();
  await pullMiddle(page);
  await pulled(page);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  const before = await drawsMade(page);
  await page.click('[data-dko-another]');
  await page.waitForFunction(() => document.querySelector('[data-dko]').hasAttribute('data-stage') && document.querySelectorAll('[data-dko-row] .dko-tcard').length === 1 && document.activeElement.hasAttribute('data-dko-back') && !document.querySelector('[data-dko-fan]').getAnimations({ subtree: true }).length);
  const r = await page.evaluate(() => ({
    drawn: document.querySelector('[data-dko-traylab]').textContent.replace(/\s+/g, ' ').trim(), slot: document.querySelectorAll('.dko-slot .dk-wrap').length, leaf: document.querySelector('[data-dko-leaf]').children.length,
    fanInStage: !!document.querySelector('[data-dko-stagefan] [data-dko-fan]'), backs: document.querySelectorAll('[data-dko-back]:not([disabled])').length, hint: document.querySelector('[data-dko-hint]').textContent,
    tcardLabel: document.querySelector('.dko-tcard').getAttribute('aria-label'), centered: (() => { const f = document.querySelector('[data-dko-fan]').getBoundingClientRect(), p = document.querySelector('.dkd-panel').getBoundingClientRect(); return Math.abs((f.left + f.width / 2) - (p.left + p.width / 2)) < 4; })(),
    trayVisible: document.querySelector('[data-dko-traybody]').getBoundingClientRect().height > 0,
  }));
  assert.equal(await drawsMade(page), before, 'Draw another drew a card by itself');
  assert.equal(r.drawn, 'Drawn 1 of 5');
  assert.equal(r.slot, 0, 'the card is still on the stage');
  assert.equal(r.leaf, 0, 'the card\'s leaf stayed behind');
  assert.ok(r.fanInStage && r.centered, 'the fan did not lift to the middle');
  assert.equal(r.backs, 7);
  assert.equal(r.hint, 'Pull one.');
  assert.match(r.tcardLabel, /^The Tide Withdraws, Omen\. Press to bring it back to the stage\.$/);
  assert.ok(r.trayVisible);
  // Ready to pull: the next pull draws, takes the stage, and the drawn card stays set aside.
  await pullMiddle(page);
  await pulled(page, 2);
  assert.equal(await drawsMade(page), before + 1);
  assert.equal(await page.locator('[data-dko-row] .dko-tcard').count(), 1);
  assert.equal(await page.evaluate(() => document.querySelector('[data-dko]').hasAttribute('data-stage')), false);
  await page.close2();
});

test('a back pulled from the lowered fan draws too, and the card that was on the stage goes to the Drawn row', { skip }, async () => {
  const page = await openDirector();
  await pullMiddle(page);
  await pulled(page);
  await pullMiddle(page);
  await page.waitForFunction(() => document.querySelectorAll('[data-dko-row] .dko-tcard').length === 1 && !document.querySelector('[data-dko-fan]').getAnimations({ subtree: true }).length);
  assert.equal(await drawsMade(page), 2);
  assert.equal(await page.evaluate(() => document.querySelector('[data-dko-traylab]').textContent.replace(/\s+/g, ' ').trim()), 'Drawn 2 of 5');
  await page.close2();
});

test('Draw another also works on the phone, where the drawn row sits below in the scroll region', { skip }, async () => {
  const page = await openDirector({ phone: true });
  await pullMiddle(page);
  await pulled(page);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  await page.click('[data-dko-another]');
  await page.waitForFunction(() => document.querySelector('[data-dko]').hasAttribute('data-stage') && document.querySelectorAll('[data-dko-row] .dko-tcard').length === 1 && document.activeElement.hasAttribute('data-dko-back') && !document.querySelector('[data-dko-fan]').getAnimations({ subtree: true }).length);
  const r = await page.evaluate(() => ({ tray: !!document.querySelector('.dko-scroll [data-dko-row] .dko-tcard') && document.querySelector('.dko-tcard').getBoundingClientRect().height > 40, fan: !!document.querySelector('[data-dko-stagefan] [data-dko-fan]'), backs: document.querySelectorAll('[data-dko-back]:not([disabled])').length }));
  assert.deepEqual(r, { tray: true, fan: true, backs: 5 });
  await page.close2();
});

test('Put it back turns the card face down into the fan and the fan grows', { skip }, async () => {
  const page = await openDirector({ backs: 6, srv: () => { window.__srv.decks.d1.pile.splice(0, 3); } });
  assert.equal(await page.locator('[data-dko-back]').count(), 6);
  await pullMiddle(page);
  await pulled(page);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  assert.equal(await page.locator('[data-dko-back]').count(), 5);
  await page.click('[data-dko-putback]');
  await page.waitForFunction(() => document.querySelectorAll('[data-dko-back]').length === 6 && !document.querySelector('.dko-slot .dk-wrap') && !document.querySelector('[data-dko-fan]').getAnimations({ subtree: true }).length);
  assert.ok((await calls(page)).includes('POST /campaigns/c1/decks/d1/copies/o4/put-back {}'));
  assert.equal(await text(page, '[data-dko-line]'), 'Back in the deck.');
  assert.match(await text(page, '.dkd-subhead'), /^Events · 6 left$/);
  await page.close2();
});

test('a card in the Drawn row comes back to the stage when pressed', { skip }, async () => {
  const page = await openDirector({ backs: 7, srv: () => { window.__srv.decks.d1.shown = ['o1', 'o2']; window.__srv.decks.d1.pile.splice(0, 2); } });
  assert.equal(await text(page, '[data-dko-traylab]'), 'Drawn 2 of 5');
  assert.equal(await page.locator('.dko-tcard').count(), 2);
  await page.locator('.dko-tcard').first().click();
  await page.waitForFunction(() => document.querySelector('.dko-slot [data-state="up"]') && document.querySelector('[data-dko-leaf] .dk-dn') && !document.querySelector('[data-dko-fan]').getAnimations({ subtree: true }).length);
  assert.equal(await page.evaluate(() => document.querySelector('[data-dko]').hasAttribute('data-stage')), false);
  assert.equal(await page.locator('.dko-tcard').count(), 2 - 1);
  assert.equal(await text(page, '[data-dko-leaf] .dk-dn'), 'The Tide Withdraws');
  assert.equal(await text(page, '[data-dko-traylab]'), 'Drawn 2 of 5');
  await page.close2();
});

test('Deal to… lists the party, Escape closes the list first, then dealing sends the hero', { skip }, async () => {
  const page = await openDirector();
  await pullMiddle(page);
  await pulled(page);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  await page.click('[data-dko-deal]');
  await page.waitForSelector('[role="menu"]');
  assert.deepEqual(await page.$$eval('[role="menu"] .dkd-mi', (b) => b.map((x) => x.textContent)), ['Brannoc', 'Isolde Varr']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('[role="menu"]', { state: 'detached' });
  assert.ok(await page.$('[data-dkd-ov]'), 'Escape closed the overlay before the list');
  assert.equal(await page.evaluate(() => document.activeElement.hasAttribute('data-dko-deal')), true);
  await page.click('[data-dko-deal]');
  await page.click('[role="menu"] [data-hero="h2"]');
  await page.waitForFunction(() => document.querySelector('[data-dko-line]').textContent === 'Dealt to Isolde Varr.');
  assert.ok((await calls(page)).some((c) => /\/decks\/d1\/copies\/o\d\/deal \{"to":"hand","heroId":"h2"\}$/.test(c)));
  await page.keyboard.press('Escape');
  await page.waitForSelector('[data-dkd-ov]', { state: 'detached' });
  await page.close2();
});

test('Show the table sends the show verb and says so', { skip }, async () => {
  const page = await openDirector();
  await pullMiddle(page);
  await pulled(page);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  await page.click('[data-dko-primary]');
  await page.waitForFunction(() => document.querySelector('[data-dko-line]').textContent === 'On the table.');
  assert.ok((await calls(page)).some((c) => /\/decks\/d1\/copies\/o\d\/show \{\}$/.test(c)));
  assert.equal(await page.locator('.dko-slot .dk-wrap').count(), 0);
  await page.close2();
});

// ---------------------------------------------------------------- refusals

test('a deck with five shown says so in the server\'s words, capitalised, and the backs are disabled', { skip }, async () => {
  const page = await open({ srv: () => { const d = window.__srv.decks.d1; d.shown = d.pile.splice(0, 5); } });
  await page.waitForSelector('[data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-draw]');
  await page.waitForSelector('[data-dko-back]');
  await waitFan(page, 4);
  const r = await page.evaluate(() => ({ hint: document.querySelector('[data-dko-hint]').textContent, off: [...document.querySelectorAll('[data-dko-back]')].every((b) => b.disabled), lab: document.querySelector('[data-dko-traylab]').textContent }));
  assert.equal(r.hint, 'Five cards are already drawn. Use one or put one back first.');
  assert.ok(r.off);
  assert.equal(r.lab, 'Drawn 5 of 5');
  await page.close2();
});

test('an empty pile says "Nothing left in this deck." and shows no backs', { skip }, async () => {
  const page = await open({ srv: () => { const d = window.__srv.decks.d1; d.pile = []; } });
  await page.waitForSelector('[data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-draw]');
  await page.waitForSelector('[data-dko-hint]');
  await page.waitForFunction(() => document.querySelector('[data-dko-hint]').textContent !== '');
  assert.equal(await text(page, '[data-dko-hint]'), 'Nothing left in this deck.');
  assert.equal(await page.locator('[data-dko-back]').count(), 0);
  await page.close2();
});

test('a refused pull says the server\'s words under the fan and leaves the backs to try again', { skip }, async () => {
  const page = await openDirector();
  await page.evaluate(() => { window.__srv.refuse = 'the deck was shuffled by someone else'; });
  await pullMiddle(page);
  await page.waitForFunction(() => /^The deck was shuffled by someone else\.$/.test(document.querySelector('[data-dko-hint]').textContent));
  assert.equal(await page.locator('[data-dko-back][data-working]').count(), 0);
  assert.equal(await page.locator('[data-dko-back]').count(), 7);
  assert.equal(await page.locator('.dko-slot .dk-wrap').count(), 0);
  await page.close2();
});

test('offline: the draw failed says so, the back is live again and focus stays', { skip }, async () => {
  const page = await openDirector();
  await page.evaluate(() => { window.__srv.offline = true; });
  await page.locator('[data-dko-back]').nth(2).click();
  await page.waitForFunction(() => document.querySelector('[data-dko-hint]').textContent === 'The draw failed. Pull again to try again.');
  assert.equal(await page.locator('[data-dko-back][data-working]').count(), 0);
  assert.equal(await page.locator('[data-dko-back]:not([disabled])').count(), 7);
  assert.equal(await page.evaluate(() => document.activeElement.hasAttribute('data-dko-back')), true);
  await page.evaluate(() => { window.__srv.offline = false; });
  await pullMiddle(page);
  await pulled(page);
  await page.close2();
});

test('a turn that fails offline leaves the card face down, says why and can be tried again', { skip }, async () => {
  const page = await openDirector();
  await pullMiddle(page);
  await pulled(page);
  await page.evaluate(() => { window.__srv.offline = true; });
  await page.click('[data-dko-cardturn]');
  await page.waitForFunction(() => /That did not go through\. Check your connection and try again\./.test(document.querySelector('[data-dko-line]').textContent));
  assert.equal(await page.getAttribute('.dko-slot .dk-wrap', 'data-state'), 'down');
  await page.evaluate(() => { window.__srv.offline = false; });
  await page.click('[data-dko-cardturn]');
  await turned(page);
  await page.close2();
});

// ---------------------------------------------------------------- Calm, Off, phone

for (const motion of ['calm', 'off']) {
  test(`${motion}: a pull, a turn and Draw another travel nothing${motion === 'off' ? ' and play no animation' : ', they only fade'}`, { skip }, async () => {
    const page = await open({ motion });
    await page.waitForSelector('[data-dkd-row]');
    await page.evaluate(() => {
      window.__seen = { transform: false, any: false };
      setInterval(() => {
        document.getAnimations().forEach((a) => {
          if (!a.effect || !a.effect.target || !a.effect.target.closest || !a.effect.target.closest('[data-dkd-ov]')) return;
          window.__seen.any = true;
          if (a.effect.getKeyframes().some((k) => 'transform' in k && k.transform !== 'none' && k.transform !== '')) window.__seen.transform = true;
          if (a.transitionProperty && /transform|translate|rotate|scale/.test(a.transitionProperty)) window.__seen.transform = true;
        });
      }, 8);
    });
    await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
    await page.click('[data-dkd-deck="d1"] [data-dkd-draw]');
    await page.waitForSelector('[data-dko-back]');
    await waitFan(page, 7);
    await pullMiddle(page);
    await pulled(page);
    await page.click('[data-dko-cardturn]');
    await turned(page);
    await page.click('[data-dko-another]');
    await page.waitForFunction(() => document.querySelector('[data-dko]').hasAttribute('data-stage'));
    await page.waitForTimeout(450);
    const seen = await page.evaluate(() => window.__seen);
    assert.equal(seen.transform, false, `${motion}: something travelled`);
    if (motion === 'off') assert.equal(seen.any, false, 'off: an animation played');
    assert.equal(await page.locator('[data-dko-row] .dko-tcard').count(), 1);
    await page.close2();
  });
}

test('on a phone the panel is a sheet, the fan has five backs and every control is 44px', { skip }, async () => {
  const page = await openDirector({ phone: true });
  const r = await page.evaluate(() => {
    const p = document.querySelector('.dkd-panel').getBoundingClientRect(), backs = [...document.querySelectorAll('[data-dko-back]')];
    const cx = backs.map((b) => { const r = b.getBoundingClientRect(); return r.left + r.width / 2; });
    return { w: p.width, vw: innerWidth, n: backs.length, gaps: cx.slice(1).map((x, i) => x - cx[i]), hint: document.querySelector('[data-dko-hint]').textContent, close: document.querySelector('[data-dkd-close]').getBoundingClientRect().height, scrollX: document.documentElement.scrollWidth - innerWidth };
  });
  assert.equal(r.w, r.vw);
  assert.equal(r.n, 5);
  assert.match(r.hint, /^Shuffled: every back is the same draw\. Pull one\. 5 of 9 shown\.$/);
  r.gaps.forEach((g) => assert.ok(g >= 44, `backs are ${g}px apart`));
  assert.equal(Math.round(r.close), 44);
  assert.ok(r.scrollX <= 0, 'the page scrolls sideways');
  await pullMiddle(page);
  await pulled(page);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  const t = await page.evaluate(() => ({
    acts: [...document.querySelectorAll('[data-dko-acts] button')].map((b) => [b.textContent.trim(), Math.round(b.getBoundingClientRect().height)]),
    scroll: (() => { const s = document.querySelector('.dko-scroll'); return s.scrollWidth - s.clientWidth; })(), page: document.documentElement.scrollWidth - innerWidth,
    leafUnder: document.querySelector('[data-dko-leaf]').getBoundingClientRect().top >= document.querySelector('.dko-slot').getBoundingClientRect().bottom - 2,
  }));
  t.acts.forEach(([n, h]) => assert.ok(h >= 44, `${n} is ${h}px tall`));
  assert.ok(t.scroll <= 0 && t.page <= 0, JSON.stringify(t));
  assert.ok(t.leafUnder, 'the leaf is not under the card on a phone');
  await page.close2();
});

test('the section on a phone: rows and buttons are 44px and nothing scrolls sideways', { skip }, async () => {
  const page = await open({ phone: true });
  await page.waitForSelector('[data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
  await page.waitForSelector('[data-dkd-deck="d1"] [data-dkd-draw]', { state: 'visible' });
  const r = await page.evaluate(() => ({
    row: document.querySelector('[data-dkd-row]').getBoundingClientRect().height, draw: document.querySelector('[data-dkd-draw]').getBoundingClientRect().height, quick: document.querySelector('[data-dkd-quick]').getBoundingClientRect().height,
    choose: document.querySelector('[data-dkd-choose]').getBoundingClientRect().height, more: document.querySelector('[data-dkd-more]').getBoundingClientRect().height, x: document.documentElement.scrollWidth - innerWidth }));
  ['row', 'draw', 'quick', 'choose', 'more'].forEach((k) => assert.ok(r[k] >= 44, `${k} is ${r[k]}px`));
  assert.ok(r.x <= 0);
  await page.close2();
});

// ---------------------------------------------------------------- section: Quick draw, menu, choose

test('Quick draw draws one, the card sits in the row, and its menu shows the table, deals, puts back and opens', { skip }, async () => {
  const page = await open();
  await page.waitForSelector('[data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-quick]');
  await page.waitForSelector('[data-dkd-deck="d1"] [data-dkd-copy]');
  assert.ok((await calls(page)).includes('POST /campaigns/c1/decks/d1/draw {"count":1}'));
  assert.equal(await text(page, '[data-dkd-deck="d1"] [data-dkd-sub]'), 'Events · 8 left · 1 drawn');
  assert.equal(await page.getAttribute('[data-dkd-deck="d1"] [data-dkd-row]', 'aria-expanded'), 'true', 'the row folded itself on a draw');
  await page.click('[data-dkd-copy]');
  await page.waitForSelector('[role="menu"]');
  assert.deepEqual(await page.$$eval('[role="menu"] .dkd-mi', (b) => b.map((x) => x.textContent)), ['Show the table', 'Deal to…', 'Put it back', 'Open']);
  await page.click('[role="menu"] [data-act="put-back"]');
  await page.waitForFunction(() => !document.querySelector('[data-dkd-copy]'));
  assert.equal(await text(page, '[data-dkd-deck="d1"] [data-dkd-sub]'), 'Events · 9 left');
  await page.close2();
});

test('a drawn card\'s menu: Deal to… sends the hero, Open puts that card on the overlay\'s stage, Escape closes the menu', { skip }, async () => {
  const page = await open({ srv: () => { const d = window.__srv.decks.d1; d.shown = ['o1', 'o2']; d.pile.splice(0, 2); } });
  await page.waitForSelector('[data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
  await page.click('[data-dkd-copy="o1"]');
  await page.waitForSelector('[role="menu"]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('[role="menu"]', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.getAttribute('data-dkd-copy')), 'o1');
  await page.click('[data-dkd-copy="o2"]');
  await page.click('[role="menu"] [data-act="deal"]');
  await page.click('[role="menu"] [data-hero="h1"]');
  await page.waitForFunction(() => !document.querySelector('[data-dkd-copy="o2"]'));
  assert.ok((await calls(page)).includes('POST /campaigns/c1/decks/d1/copies/o2/deal {"to":"hand","heroId":"h1"}'));
  await page.click('[data-dkd-copy="o1"]');
  await page.click('[role="menu"] [data-act="open"]');
  await page.waitForSelector('[data-dkd-ov] .dko-slot [data-state="up"]');
  assert.equal(await text(page, '[data-dko-leaf] .dk-dn'), 'The Tide Withdraws');
  assert.equal(await page.evaluate(() => document.querySelector('[data-dko]').hasAttribute('data-stage')), false);
  assert.equal(await page.locator('[data-dko-acts] button').first().textContent(), 'Show the table');
  await page.close2();
});

test('a refused Quick draw is said under the row in the server\'s words and the row stays open', { skip }, async () => {
  const page = await open({ srv: () => { window.__srv.refuse = 'five cards are already drawn. Use or put one back first'; } });
  await page.waitForSelector('[data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-quick]');
  await page.waitForSelector('[data-dkd-deck="d1"] [data-dkd-err]:not([hidden])');
  assert.equal(await text(page, '[data-dkd-deck="d1"] [data-dkd-err]'), 'Five cards are already drawn. Use or put one back first.');
  assert.equal(await page.getAttribute('[data-dkd-deck="d1"] [data-dkd-err]', 'role'), 'alert');
  assert.equal(await page.getAttribute('[data-dkd-deck="d1"] [data-dkd-row]', 'aria-expanded'), 'true');
  await page.close2();
});

test('Choose decks lists every deck with ticks, saves the picks in order, and All decks unfolds the rest', { skip }, async () => {
  const page = await open();
  await page.waitForSelector('[data-dkd-row]');
  assert.equal(await page.locator('[data-dkd-deck]:visible').count(), 2);
  assert.equal(await text(page, '[data-dkd-more]'), 'All decks · 3');
  await page.click('[data-dkd-more]');
  await page.waitForSelector('[data-dkd-rest]', { state: 'visible' });
  assert.equal(await page.locator('[data-dkd-deck]:visible').count(), 3);
  await page.click('[data-dkd-choose]');
  await page.waitForSelector('[data-dkd-picker]', { state: 'visible' });
  assert.deepEqual(await page.$$eval('[data-dkd-picker] input', (i) => i.map((x) => [x.value, x.checked])), [['d1', true], ['d2', true], ['d3', false]]);
  await page.uncheck('[data-dkd-picker] input[value="d1"]');
  await page.check('[data-dkd-picker] input[value="d3"]');
  await page.click('[data-dkd-picks-save]');
  await page.waitForFunction(() => window.__srv.picks);
  assert.ok((await calls(page)).includes('PUT /campaigns/c1/decks/screen/picks {"deckIds":["d2","d3"]}'));
  await page.waitForFunction(() => [...document.querySelectorAll('[data-dkd-screen] > .dkd-deck')].map((n) => n.getAttribute('data-dkd-deck')).join() === 'd2,d3');
  assert.deepEqual(await page.$$eval('[data-dkd-screen] > .dkd-deck', (n) => n.map((x) => x.getAttribute('data-dkd-deck'))), ['d2', 'd3']);
  assert.equal(await page.getAttribute('[data-dkd-choose]', 'aria-expanded'), 'false');
  await page.close2();
});

test('a refused save of the picks says why under the list', { skip }, async () => {
  const page = await open({ srv: () => { window.__srv.picksRefuse = 'at most 12 decks can be picked'; } });
  await page.waitForSelector('[data-dkd-row]');
  await page.click('[data-dkd-choose]');
  await page.click('[data-dkd-picks-save]');
  await page.waitForSelector('[data-dkd-picks-err]:not([hidden])');
  assert.equal(await text(page, '[data-dkd-picks-err]'), 'At most 12 decks can be picked.');
  await page.close2();
});

// ---------------------------------------------------------------- the player

test('a player\'s Draw opens the overlay, pulls with the hero, turns through the hand\'s turn and never shows the card before it', { skip }, async () => {
  const page = await openPlayer();
  assert.ok(page.loaded.includes('/static/plugins/decks/js/deck_draw.js'));
  assert.equal(await text(page, '.dkd-subhead'), 'Events · 9 left');
  assert.equal(await text(page, '[data-dko-hint]'), SEVEN + ' 3 draws left.');
  await pullMiddle(page);
  await pulled(page);
  assert.ok((await calls(page)).includes('POST /campaigns/c1/decks/d1/draw {"heroId":"h1","count":1}'));
  const html = await page.evaluate(() => document.querySelector('[data-dkd-ov]').outerHTML);
  assert.doesNotMatch(html, /Tide|Lantern|Bell|Gulls|Salt|Sails|Smuggler|Stranger|Fog|sea pulls/);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  assert.ok((await calls(page)).includes('POST /campaigns/c1/decks/hand/d1/copies/o1/turn?size=open'));
  assert.deepEqual(await page.$$eval('[data-dko-acts] button', (b) => b.map((x) => x.textContent.trim())), ['Play it', 'Show the table', 'Keep it', 'Draw another · 2 left']);
  assert.equal(await text(page, '[data-dko-line]'), 'In your hand.');
  await page.close2();
});

test('a player\'s Draw another sets the card aside, "Drawn now" counts, and the last draw dims the backs and says "No draws left."', { skip }, async () => {
  const page = await openPlayer({ srv: () => { window.__srv.decks.d1.drawsLeft = 2; } });
  await pullMiddle(page);
  await pulled(page);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  await page.click('[data-dko-another]');
  await page.waitForFunction(() => document.querySelector('[data-dko]').hasAttribute('data-stage') && document.querySelectorAll('[data-dko-row] .dko-tcard').length === 1 && document.activeElement.hasAttribute('data-dko-back') && !document.querySelector('[data-dko-fan]').getAnimations({ subtree: true }).length);
  assert.equal(await text(page, '[data-dko-traylab]'), 'Drawn now 1');
  assert.equal(await drawsMade(page), 1);
  await pullMiddle(page);
  await pulled(page, 2);
  await page.waitForFunction(() => document.querySelector('[data-dko-hint]').textContent === 'No draws left.');
  const r = await page.evaluate(() => ({ off: [...document.querySelectorAll('[data-dko-back]')].every((b) => b.disabled), dim: document.querySelector('[data-dko-fan]').hasAttribute('data-off'), lab: document.querySelector('[data-dko-traylab]').textContent }));
  assert.ok(r.off && r.dim);
  assert.equal(r.lab, 'Drawn now 2');
  await page.close2();
});

test('Play it sends the play verb; closing reads the hand again and puts focus back on Draw', { skip }, async () => {
  const page = await openPlayer();
  await pullMiddle(page);
  await pulled(page);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  await page.click('[data-dko-primary]');
  await page.waitForFunction(() => document.querySelector('[data-dko-line]').textContent === 'Played.');
  assert.ok((await calls(page)).some((c) => /\/decks\/d1\/copies\/o\d\/play \{\}$/.test(c)));
  const before = (await calls(page)).filter((c) => c === 'GET /campaigns/c1/decks/hand').length;
  await page.keyboard.press('Escape');
  await page.waitForSelector('[data-dkd-ov]', { state: 'detached' });
  await page.waitForFunction((n) => window.__srv.calls.filter((c) => c === 'GET /campaigns/c1/decks/hand').length > n, before);
  assert.equal(await page.evaluate(() => document.activeElement.hasAttribute('data-dkd-open')), true);
  await page.close2();
});

test('Quick draw keeps its behaviour: one request, no overlay', { skip }, async () => {
  const page = await open({ viewer: 'player' });
  await page.waitForSelector('[data-dkh-draw]');
  assert.equal(await text(page, '[data-dkh-draw]'), 'Quick draw');
  await page.click('[data-dkh-draw]');
  await page.waitForFunction(() => window.__srv.calls.some((c) => /decks\/d1\/draw /.test(c)));
  await page.waitForTimeout(300);
  assert.equal(await page.$('[data-dkd-ov]'), null);
  assert.ok((await calls(page)).includes('POST /campaigns/c1/decks/d1/draw {"heroId":"h1","count":1}'));
  assert.equal(page.loaded.includes('/static/plugins/decks/js/deck_draw.js'), false, 'Quick draw loaded the overlay');
  await page.close2();
});

test('a player whose deck is closed to them is told in the server\'s words and nothing opens', { skip }, async () => {
  const page = await open({ viewer: 'player' });
  await page.waitForSelector('[data-dkd-open]');
  await page.evaluate(() => { window.__srv.fail = (m, u) => /draw-view/.test(u); window.__srv.failAnswer = { status: 404, body: '{"message":"not found"}' }; });
  await page.click('[data-dkd-open]');
  await page.waitForSelector('[data-dkh-err]:not([hidden])');
  assert.match(await text(page, '[data-dkh-err]'), /^That card is not here any more\./);
  assert.equal(await page.$('[data-dkd-ov]'), null);
  await page.close2();
});

// ---------------------------------------------------------------- live

test('a live message reads the counts again and a shown card taken away elsewhere closes its stage with a line', { skip }, async () => {
  const page = await openDirector();
  await pullMiddle(page);
  await pulled(page);
  await page.click('[data-dko-cardturn]');
  await turned(page);
  await page.evaluate(() => { const d = window.__srv.decks.d1; d.shown = []; d.pile.splice(0, 2); window.dispatchEvent(new CustomEvent('chronicle:decks-changed')); });
  await page.waitForFunction(() => document.querySelector('[data-dko-line]').textContent === 'That card was taken away.');
  await page.waitForFunction(() => !document.querySelector('.dko-slot .dk-wrap'));
  assert.match(await text(page, '.dkd-subhead'), /^Events · 6 left$/);
  await page.close2();
});

test('a live message re-reads the section too', { skip }, async () => {
  const page = await open();
  await page.waitForSelector('[data-dkd-row]');
  await page.evaluate(() => { window.__srv.decks.d1.pile.splice(0, 4); window.dispatchEvent(new CustomEvent('chronicle:decks-changed')); });
  await page.waitForFunction(() => document.querySelector('[data-dkd-deck="d1"] [data-dkd-sub]').textContent === 'Events · 5 left');
  await page.close2();
});

// ---------------------------------------------------------------- the dealt card

const FRESH = { deckId: 'd1', copyId: 'o1', deckName: 'Omens', sub: 'Events · 9 left', backHtml: '<div class="dk-card dk-back dk-s-tall dk-sz-hand" aria-hidden="true"><span class="dk-bfr"><span class="dk-bw">OMENS</span></span></div>', turn: 'flip', shape: 'tall' };
const fresh = () => { window.__srv.fresh = [window.__FRESH]; };

test('a dealt card pops up once: face down, centred, nothing of it shown, Turn it over and Later', { skip }, async () => {
  const page = await open({ viewer: 'player', start: false });
  await page.evaluate((f) => { window.__FRESH = f; }, FRESH);
  await page.evaluate(fresh);
  await page.evaluate(() => window.__start());
  await page.waitForSelector('[data-dkd-ov][data-arrive] [data-dko-cardturn]');
  const r = await page.evaluate(() => {
    const ov = document.querySelector('[data-dkd-ov]'), p = ov.querySelector('.dkd-panel').getBoundingClientRect(), c = ov.querySelector('.dk-wrap').getBoundingClientRect();
    return { html: ov.textContent, acts: [...ov.querySelectorAll('[data-dko-acts] button')].map((b) => b.textContent.trim()), centred: Math.abs((c.left + c.width / 2) - (p.left + p.width / 2)) < 4, title: ov.querySelector('h2').textContent, state: ov.querySelector('.dk-wrap').getAttribute('data-state'), modal: ov.querySelector('[role=dialog]').getAttribute('aria-modal') };
  });
  assert.deepEqual(r.acts, ['Turn it over', 'Later']);
  assert.ok(r.centred);
  assert.equal(r.state, 'down');
  assert.equal(r.title, 'Omens');
  assert.equal(r.modal, 'true');
  assert.doesNotMatch(r.html, /Tide|sea pulls/);
  assert.equal((await calls(page)).filter((c) => /seen$/.test(c)).length, 1, 'the card was not marked seen as it opened');
  await page.click('[data-dko-later]');
  await page.waitForSelector('[data-dkd-ov]', { state: 'detached' });
  // Another live message does not show it again.
  await page.evaluate(() => window.dispatchEvent(new CustomEvent('chronicle:decks-changed')));
  await page.waitForTimeout(400);
  assert.equal(await page.$('[data-dkd-ov]'), null, 'it popped a second time');
  assert.equal((await calls(page)).filter((c) => /seen$/.test(c)).length, 1);
  assert.ok((await calls(page)).includes('POST /campaigns/c1/decks/hand/fresh/o1/seen'));
  await page.close2();
});

test('several cards dealt at once pop up once, as the newest, and the rest are marked seen', { skip }, async () => {
  const page = await open({ viewer: 'player', start: false });
  await page.evaluate((f) => { window.__srv.fresh = [f, Object.assign({}, f, { copyId: 'o2' }), Object.assign({}, f, { copyId: 'o3' })]; }, FRESH);
  await page.evaluate(() => window.__start());
  await page.waitForSelector('[data-dkd-ov][data-arrive] [data-dko-cardturn]');
  await page.waitForFunction(() => window.__srv.calls.filter((c) => /seen$/.test(c)).length === 3);
  await page.click('[data-dko-later]');
  await page.waitForSelector('[data-dkd-ov]', { state: 'detached' });
  await page.waitForTimeout(400);
  assert.equal(await page.$('[data-dkd-ov]'), null, 'a second card popped up after the first');
  const seen = (await calls(page)).filter((c) => /seen$/.test(c)).sort();
  assert.deepEqual(seen, ['o1', 'o2', 'o3'].map((id) => 'POST /campaigns/c1/decks/hand/fresh/' + id + '/seen'));
  await page.close2();
});

test('pressing the dealt card turns it through the hand\'s turn; Escape closes it', { skip }, async () => {
  const page = await open({ viewer: 'player', start: false });
  await page.evaluate((f) => { window.__FRESH = f; }, FRESH);
  await page.evaluate(fresh);
  await page.evaluate(() => window.__start());
  await page.waitForSelector('[data-dko-cardturn]');
  await page.waitForFunction(() => document.activeElement.hasAttribute('data-dko-cardturn'));
  assert.deepEqual((await calls(page)).filter((c) => /\/turn(\?|$)/.test(c)), []);
  await page.click('[data-dko-cardturn]');
  await page.waitForFunction(() => document.querySelector('[data-dko-arrive] [data-state="up"]') && document.querySelector('[data-dko-leaf] .dk-dn'));
  assert.ok((await calls(page)).includes('POST /campaigns/c1/decks/hand/d1/copies/o1/turn?size=open'));
  assert.equal(await text(page, '[data-dko-line]'), 'In your hand.');
  await page.keyboard.press('Escape');
  await page.waitForSelector('[data-dkd-ov]', { state: 'detached' });
  await page.close2();
});

test('a page with no fresh card never loads the overlay, and a Director is never asked', { skip }, async () => {
  const p = await open({ viewer: 'player' });
  await p.waitForFunction(() => window.__srv.calls.some((c) => /hand\/fresh$/.test(c)));
  await p.waitForTimeout(300);
  assert.equal(p.loaded.includes('/static/plugins/decks/js/deck_draw.js'), false);
  assert.equal(p.loaded.includes('/static/css/decks_draw.css'), false);
  await p.close2();
  const d = await open({ viewer: 'director' });
  await d.waitForFunction(() => window.__srv.calls.some((c) => /decks\/moments\/panel/.test(c)));
  assert.equal((await calls(d)).some((c) => /hand\/fresh/.test(c)), false, 'a Director asked for fresh cards');
  await d.close2();
});

test('a dealt card waits behind an overlay that is already open and pops up when it closes', { skip }, async () => {
  const page = await open({ viewer: 'player' });
  await page.click('[data-dkd-open]');
  await page.waitForSelector('[data-dkd-ov] [data-dko-back]');
  await page.evaluate((f) => { window.__srv.fresh = [f]; window.dispatchEvent(new CustomEvent('chronicle:decks-changed')); }, FRESH);
  await page.waitForTimeout(500);
  assert.equal(await page.locator('[data-dkd-ov]').count(), 1);
  assert.equal(await page.locator('[data-dkd-ov][data-arrive]').count(), 0, 'it popped over the open overlay');
  await page.keyboard.press('Escape');
  await page.waitForSelector('[data-dkd-ov][data-arrive] [data-dko-cardturn]');
  await page.close2();
});

// ---------------------------------------------------------------- the Table

test('a press inside the overlay does not close the Table drawer behind it', { skip }, async () => {
  const page = await open();
  await page.waitForFunction(() => window.DeckDraw);
  await page.click('[data-tt-tab]');
  await page.waitForFunction(() => document.querySelector('.tt-host')._table.open);
  await page.evaluate(() => window.DeckDraw.open({ campaignId: 'c1', deckId: 'd1', from: document.querySelector('[data-dkd-deck="d1"] [data-dkd-row]'), viewer: 'director' }));
  await page.waitForSelector('[data-dkd-ov] [data-dko-back]');
  await page.locator('.dkd-panel .dkd-head').click();
  assert.equal(await page.evaluate(() => document.querySelector('.tt-host')._table.open), true);
  await page.close2();
});

// ------------------------------------------------- the DM Screen on its own

// Where the Table is not on the page (Site admin), the DM Screen fetches the
// overlay itself, from the addresses on its own button.
test('the DM Screen loads the overlay itself where the Table is not on the page', { skip }, async () => {
  browser = browser || await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await ctx.newPage();
  page.setDefaultTimeout(8000);
  const errors = [];
  const loaded = [];
  const urls = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  page.on('request', (r) => urls.push(r.url()));
  const body = `<!doctype html><html><head><meta charset="utf-8"><style>${TOKENS}</style></head><body>
<header id="app-topbar"><div data-widget="dm-screen" data-url="/campaigns/c1/dm-screen" data-dkd-src="/static/plugins/decks/js/deck_draw.js?v=t1" data-dkd-cards-src="/static/plugins/decks/js/deck_cards.js?v=t2" data-dkd-css="/static/css/decks_draw.css?v=t3" data-dkd-card-css="/static/css/decks_card.css?v=t4">
<button type="button" data-dms-toggle aria-expanded="false">Screen</button></div></header><main id="main-content"></main>
<script>${SERVER}
window.__srv.viewer = 'director';
var widgets = {};
window.Chronicle = {
  register: function (n, d) { widgets[n] = d; },
  mountWidgets: function () {}, destroyWidget: function () {},
  apiFetch: function (u, o) { o = o || {}; return fetch(u, { method: o.method, headers: o.headers, body: o.body && typeof o.body === 'object' ? JSON.stringify(o.body) : o.body }); },
  working: function () {}, escapeHtml: function (s) { return String(s == null ? '' : s).replace(/[&<>]/g, function (c) { return { '&': '&#38;', '<': '&#60;', '>': '&#62;' }[c]; }); }, notify: function () {}, motionMs: function (n) { return n; }, reducedMotion: function () { return true; }, gmTerm: function () { return 'Director'; }
};
</script><script src="/js/dm_screen.js"></script>
<script>widgets['dm-screen'].init(document.querySelector('[data-widget="dm-screen"]'));</script></body></html>`;
  await page.route('http://chronicle.test/**', (r) => {
    const path = new URL(r.request().url()).pathname;
    loaded.push(path);
    if (path === '/') return r.fulfill({ contentType: 'text/html', body });
    if (FILES[path]) return r.fulfill({ contentType: path.endsWith('.css') ? 'text/css' : 'text/javascript', body: FILES[path] });
    return r.fulfill({ status: 404, body: '' });
  });
  await page.goto('http://chronicle.test/');
  await page.click('[data-dms-toggle]');
  await page.waitForSelector('[data-dms-stage] [data-dkd-screen] [data-dkd-row]');
  await page.waitForFunction(() => window.DeckDraw && document.querySelector('[data-dkd-screen]').__dkd);
  for (const f of ['/static/plugins/decks/js/deck_cards.js', '/static/plugins/decks/js/deck_draw.js', '/static/css/decks_draw.css', '/static/css/decks_card.css']) {
    assert.equal(loaded.filter((p) => p === f).length, 1, f + ' loaded once');
  }
  // The version token rides along; the path is the loader's own.
  assert.ok(urls.includes('http://chronicle.test/static/plugins/decks/js/deck_draw.js?v=t1'));
  assert.ok(urls.includes('http://chronicle.test/static/plugins/decks/js/deck_cards.js?v=t2'));
  await page.click('[data-dkd-deck="d1"] [data-dkd-row]');
  await page.click('[data-dkd-deck="d1"] [data-dkd-draw]');
  await page.waitForSelector('[data-dkd-ov] [data-dko-back]');
  await ctx.close();
  assert.deepEqual(errors, [], 'the page threw');
});
