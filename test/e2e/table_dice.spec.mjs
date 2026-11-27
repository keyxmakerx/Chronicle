// table_dice.spec.mjs — the Table's "My dice" leaf and how it throws rune dice.
//
// The real table.js, roll_dice.js and table.css run against a stub of the
// Chronicle global whose dice.roll records its arguments and whose apiFetch
// stands in for the server, including its check of a pick against the sets
// on offer. It pins: Rune 3D's "Throw a test set" throws real rune dice (not
// the flat 2D tray), your picks are saved with your account (PUT my-dice),
// every roll wears the dice the server settled for its roller on every
// screen, a roll that carries none wears the campaign's, only the offered
// sets are listed, a pick an older Table kept in the browser moves to the
// account once, and a Director who has fixed everyone's dice leaves a line
// instead of pickers.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/table_dice.spec.mjs

import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(repoRoot, p), 'utf8');
const tableCSS = read('internal/plugins/rolls/static/css/table.css');
const tableJS = read('internal/plugins/rolls/static/js/table.js');
const rollDiceJS = read('internal/plugins/rolls/static/js/roll_dice.js');

const CATALOGUE = {
  sets: ['jewelled', 'oak', 'bone'].map((k) => ({ key: k, name: k.toUpperCase(), colours: [{ key: k + '-a', name: 'A' }, { key: k + '-b', name: 'B' }] })),
  crits: { succ: [{ key: 'storm', name: 'Storm' }, { key: 'dawn', name: 'Dawn' }], fail: [{ key: 'grasp', name: 'Grasp' }, { key: 'frost', name: 'Frost' }] },
};
const CAMPAIGN = { look: 'rune', diceSet: 'oak', diceColour: 'oak-b', critSucc: 'storm', critFail: 'grasp', quality: 'hd', playersPick: true, diceOffered: [] };
const NO_PICK = { set: '', colour: '', critSucc: '', critFail: '' };

const page = (settings, pick) => `<!doctype html><html><head><meta charset="utf-8"><style>:root{--dur-slide:360ms}[hidden]{display:none!important}${tableCSS}</style></head><body>
<header><div class="tt-host" data-widget="table" data-campaign-id="c1" data-user-id="u1" data-director="false" data-gm-term="Director">
<button type="button" class="tt-tab" data-tt-tab aria-expanded="false" aria-controls="tt-drawer"><span data-tt-count hidden></span></button></div></header>
<main id="main-content"></main>
<script>
window.__sockets = []; window.__rolls = []; window.__quick = 0; window.__puts = [];
window.__settings = ${JSON.stringify(settings)}; window.__pick = ${JSON.stringify(pick)};
window.WebSocket = function () { var s = { l: {}, addEventListener: function (t, f) { this.l[t] = f; }, close: function () {} }; window.__sockets.push(s); return s; };
function reply(ok, body) { return Promise.resolve({ ok: ok, status: ok ? 200 : 400, json: function () { return Promise.resolve(body); } }); }
window.Chronicle = {
  register: function (n, o) { window.__reg = o; },
  apiFetch: function (u, o) {
    var S = window.__settings;
    if (/my-dice/.test(u)) {
      if (o && o.method === 'PUT') {
        window.__puts.push(o.body);
        if (o.body.set && S.diceOffered.length && S.diceOffered.indexOf(o.body.set) < 0) return reply(false, { message: 'Pick one of the dice sets on offer.' });
        window.__pick = o.body;
      }
      return reply(true, { pick: window.__pick, effective: {}, mayPick: S.playersPick });
    }
    return reply(true, /settings/.test(u) ? S : { rolls: [], more: false });
  },
  escapeHtml: function (s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); },
  tableDice: { motion: function () { return 'full'; }, svg: function () { return '<svg viewBox="0 0 8 8"></svg>'; }, throwInto: function () { window.__quick++; return Promise.resolve(); } },
  dice: { roll: function (g, o) { window.__rolls.push({ groups: g, opts: o }); return Promise.resolve([]); }, catalogue: function () { return Promise.resolve(${JSON.stringify(CATALOGUE)}); } }
};
</script>
<script>${rollDiceJS}</script>
<script>${tableJS}</script>
<script>window.__reg.init(document.querySelector('.tt-host'));</script>
</body></html>`;

let browser;
before(async () => { browser = await chromium.launch(); });
after(async () => { await browser?.close(); });

// A real origin, so localStorage works as it does on the site.
async function load(ctx, settings, pick = NO_PICK) {
  const p = await ctx.newPage();
  await p.route('http://chronicle.test/', (r) => r.fulfill({ contentType: 'text/html', body: page(settings, pick) }));
  return p;
}

// Opens the Table with Rune 3D picked and waits for the settings to land.
async function open(settings = CAMPAIGN, pick = NO_PICK) {
  const ctx = await browser.newContext({ viewport: { width: 1100, height: 800 } });
  const p = await load(ctx, settings, pick);
  await p.goto('http://chronicle.test/');
  await p.click('[data-tt-tab]');
  await p.waitForSelector('[data-tt-look="rune"]');
  await p.click('[data-tt-look="rune"]');
  await p.waitForFunction(() => window.Chronicle.diceTable);
  return { p, close: () => ctx.close() };
}
const plain = (v) => JSON.parse(JSON.stringify(v));
const socketRoll = (p, payload) => p.evaluate((pl) => window.__sockets[0].l.message({ data: JSON.stringify({ type: 'roll.made', payload: pl }) }), payload);
const fromMara = (extra) => Object.assign({ id: 'r' + Math.random(), rollerId: 'u2', rollerName: 'Mara', formula: '1d20', total: 12, dice: [{ sides: 20, value: 12 }], createdAt: '2026-11-11T20:00:00Z' }, extra);

test('Throw a test set with Rune 3D throws real rune dice in the campaign dice, not the flat tray', async () => {
  const { p, close } = await open();
  await p.click('[data-tt-try]');
  await p.waitForFunction(() => window.__rolls.length === 1);
  const got = await p.evaluate(() => ({ quick: window.__quick, call: window.__rolls[0], table: window.Chronicle.diceTable }));
  await close();
  assert.equal(got.quick, 0, 'the 2D tray was not used');
  assert.deepEqual(got.call.groups[0].dice.map((d) => d.sides), [4, 6, 8, 10, 12, 20]);
  assert.deepEqual(plain(got.call.opts.table), { set: 'oak', colour: 'oak-b', succ: 'storm', fail: 'grasp', quality: 'hd' });
  assert.deepEqual(plain(got.table), plain(got.call.opts.table));
});

test('Quick 2D still throws into the tray', async () => {
  const { p, close } = await open();
  await p.click('[data-tt-look="quick"]');
  await p.click('[data-tt-try]');
  const got = await p.evaluate(() => ({ quick: window.__quick, rolls: window.__rolls.length }));
  await close();
  assert.deepEqual(got, { quick: 1, rolls: 0 });
});

test('your picks are saved with your account, not in this browser', async () => {
  const { p, close } = await open();
  await p.selectOption('[data-tt-dpick="set"]', 'bone');
  await p.selectOption('[data-tt-dpick="colour"]', 'bone-b');
  await p.selectOption('[data-tt-dpick="succ"]', 'dawn');
  await p.selectOption('[data-tt-dpick="fail"]', 'frost');
  await p.waitForFunction(() => window.__puts.length === 4);
  await p.click('[data-tt-try]');
  const got = await p.evaluate(() => ({ mine: window.__rolls[0].opts.table, last: window.__puts[3], kept: localStorage.getItem('chronicle.table.dice.c1.u1') }));
  await close();
  assert.deepEqual(plain(got.mine), { set: 'bone', colour: 'bone-b', succ: 'dawn', fail: 'frost', quality: 'hd' });
  assert.deepEqual(plain(got.last), { set: 'bone', colour: 'bone-b', critSucc: 'dawn', critFail: 'frost' });
  assert.equal(got.kept, null, 'nothing is kept in the browser');
});

test('someone else\'s roll wears the dice it carries; one that carries none wears the campaign\'s', async () => {
  const { p, close } = await open();
  await socketRoll(p, fromMara({ rollerDice: { set: 'jewelled', colour: 'jewelled-b', critSucc: 'dawn', critFail: 'frost', own: true } }));
  await p.waitForFunction(() => window.__rolls.length === 1);
  await socketRoll(p, fromMara({}));
  await p.waitForFunction(() => window.__rolls.length === 2);
  const got = await p.evaluate(() => window.__rolls.map((r) => r.opts.table));
  await close();
  assert.deepEqual(plain(got[0]), { set: 'jewelled', colour: 'jewelled-b', succ: 'dawn', fail: 'frost', quality: 'hd' }, 'Mara\'s own dice');
  assert.deepEqual(plain(got[1]), { set: 'oak', colour: 'oak-b', succ: 'storm', fail: 'grasp', quality: 'hd' }, 'an older roll in the campaign\'s');
});

test('a roll\'s details name whose dice it was thrown in', async () => {
  const { p, close } = await open();
  await p.click('[data-tt-look="quick"]');
  await socketRoll(p, fromMara({ id: 'rd', rollerDice: { set: 'bone', colour: 'bone-b', critSucc: '', critFail: '', own: true } }));
  await p.waitForSelector('#tt-log [data-tt-row="rd"]');
  await p.click('#tt-log [data-tt-row="rd"]');
  await p.waitForFunction(() => /DiceMara’s BONE · B/.test(document.querySelector('#tt-det-rd').textContent));
  await close();
});

test('Use the campaign\'s dice clears the picks on the account', async () => {
  const { p, close } = await open(CAMPAIGN, { set: 'bone', colour: 'bone-a', critSucc: '', critFail: '' });
  await p.waitForSelector('[data-tt-picks-reset]');
  await p.click('[data-tt-picks-reset]');
  await p.waitForFunction(() => window.__puts.length === 1 && !document.querySelector('[data-tt-picks-reset]'));
  const got = await p.evaluate(() => ({ table: window.Chronicle.diceTable, put: window.__puts[0] }));
  await close();
  assert.equal(got.table.set, 'oak');
  assert.deepEqual(plain(got.put), NO_PICK);
});

test('only the sets on offer are listed', async () => {
  const { p, close } = await open({ ...CAMPAIGN, diceOffered: ['oak', 'jewelled'] });
  const options = await p.$$eval('[data-tt-dpick="set"] option', (os) => os.map((o) => o.value));
  await close();
  assert.deepEqual(options, ['', 'jewelled', 'oak']);
});

test('a pick this browser kept moves to the account once', async () => {
  const ctx = await browser.newContext({ viewport: { width: 1100, height: 800 } });
  const p = await load(ctx, CAMPAIGN);
  await p.addInitScript(() => { if (!sessionStorage.getItem('seeded')) { sessionStorage.setItem('seeded', '1'); localStorage.setItem('chronicle.table.dice.c1.u1', JSON.stringify({ set: 'bone', colour: 'bone-a', succ: 'dawn', fail: '' })); } });
  await p.goto('http://chronicle.test/');
  await p.waitForFunction(() => window.__puts.length === 1 && window.Chronicle.diceTable && window.Chronicle.diceTable.set === 'bone');
  const got = await p.evaluate(() => ({ put: window.__puts[0], kept: localStorage.getItem('chronicle.table.dice.c1.u1') }));
  await ctx.close();
  assert.deepEqual(plain(got.put), { set: 'bone', colour: 'bone-a', critSucc: 'dawn', critFail: '' });
  assert.equal(got.kept, '');
});

test('a kept pick the server refuses is dropped and the campaign\'s dice stand', async () => {
  const ctx = await browser.newContext({ viewport: { width: 1100, height: 800 } });
  const p = await load(ctx, { ...CAMPAIGN, diceOffered: ['oak', 'jewelled'] });
  await p.addInitScript(() => { if (!sessionStorage.getItem('seeded')) { sessionStorage.setItem('seeded', '1'); localStorage.setItem('chronicle.table.dice.c1.u1', JSON.stringify({ set: 'bone', colour: 'bone-a', succ: 'dawn', fail: '' })); } });
  await p.goto('http://chronicle.test/');
  await p.waitForFunction(() => window.__puts.length === 1 && localStorage.getItem('chronicle.table.dice.c1.u1') === '');
  const table = await p.evaluate(() => window.Chronicle.diceTable);
  await ctx.close();
  assert.deepEqual(plain(table), { set: 'oak', colour: 'oak-b', succ: 'storm', fail: 'grasp', quality: 'hd' });
});

test('when the Director has set everyone\'s dice there are no pickers, and your own rolls wear the campaign\'s', async () => {
  const { p, close } = await open({ ...CAMPAIGN, playersPick: false });
  const got = await p.evaluate(() => ({ pickers: document.querySelectorAll('[data-tt-dpick]').length, text: document.querySelector('#tt-drawer').textContent, table: window.Chronicle.diceTable }));
  await close();
  assert.equal(got.pickers, 0);
  assert.match(got.text, /Your Director has set everyone’s dice\./);
  assert.equal(got.table.set, 'oak');
});
