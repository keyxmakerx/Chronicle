// dice_customize.spec.mjs — the Dice tab on the Customize page.
//
// The real dice_customize.js and dice_customize.css run against a stub of the
// Chronicle global: apiFetch keeps the campaign's settings and records every
// PUT, dice.roll records its arguments. It pins what each control saves, that a
// failed save puts the pick back, that every set is a tile with a picture, that
// the tray throws the campaign's picks inside itself, and that the tab fits a
// phone with no sideways scroll.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/dice_customize.spec.mjs

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
const css = read('internal/plugins/rolls/static/css/dice_customize.css');
const js = read('internal/plugins/rolls/static/js/dice_customize.js');

const key = (k) => ({ key: k, name: k[0].toUpperCase() + k.slice(1) });
const SETS = ['jewelled', 'oak', 'bone'].map((k) => ({ ...key(k), note: 'A note for ' + k, colours: ['a', 'b', 'c'].map((c) => ({ key: k === 'jewelled' && c === 'a' ? 'onyx' : k + '-' + c, name: 'Colour ' + c, swatch: ['#ccc', '#333'] })) }));
const CATALOGUE = {
  sets: SETS,
  crits: {
    succ: ['phoenix', 'storm'].map((k) => ({ ...key(k), what: 'What ' + k + ' does' })),
    fail: ['grasp', 'frost'].map((k) => ({ ...key(k), what: 'What ' + k + ' does' })),
  },
  defaults: { set: 'jewelled', colour: 'onyx', succ: 'phoenix', fail: 'grasp', quality: 'auto' },
};
const START = { look: 'quick', diceSet: '', diceColour: '', critSucc: '', critFail: '', quality: '', playersPick: true, diceOffered: [] };

const page = (failPuts, o = {}) => `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>:root{--color-bg-primary:#fff;--color-bg-secondary:#f9fafb;--color-bg-tertiary:#f3f4f6;--color-border:#d1d5db;--color-text-primary:#111;--color-text-body:#333;--color-text-secondary:#666;--color-accent:#6366f1;--color-accent-rgb:99 102 241;--color-ok-rgb:22 163 74;--color-bad-rgb:220 38 38;--dur-micro:120ms;--ease-out:ease-out;--elev-resting:0 1px 2px rgb(0 0 0/.1)}
body{margin:0;padding:16px;font:14px sans-serif}${css}</style></head><body>
<div class="dcz" data-widget="dice-customize" data-campaign-id="c1" data-gm-term="Director"></div>
<script>
window.__puts = []; window.__rolls = []; window.__state = ${JSON.stringify(o.start || START)}; window.__drawn = [];
window.Chronicle = {
  register: function (n, o) { window.__reg = o; },
  escapeHtml: function (s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); },
  apiFetch: function (u, o) {
    var json = function (b) { return Promise.resolve({ ok: true, json: function () { return Promise.resolve(JSON.parse(JSON.stringify(b))); } }); };
    if (o && o.method === 'PUT') {
      window.__puts.push(o.body);
      if (${failPuts}) return Promise.resolve({ ok: false, status: 400, json: function () { return Promise.resolve({ message: 'Pick one of the dice sets.' }); } });
      Object.assign(window.__state, o.body);
    }
    return json(window.__state);
  },
  dice: { catalogue: function () { return Promise.resolve(${JSON.stringify(o.catalogue || CATALOGUE)}); },${o.portrait ? " portrait: function (q) { window.__drawn.push(q.set + '|' + q.px); return new Promise(function (r) { setTimeout(function () { r('data:image/png;base64,iVBORw0KGgo='); }, 5); }); }," : ''} roll: function (g, o) { window.__rolls.push({ groups: g, opts: { table: o.table, inTray: !!o.host && o.host.matches('.dcz-tray [data-dcz-stage]'), keep: !!o.keep } }); return Promise.resolve([{ values: [7] }]); }, stop: function () {} }
};
</script><script>${js}</script><script>window.__reg.init(document.querySelector('.dcz'));</script></body></html>`;

let browser;
before(async () => { browser = await chromium.launch(); });
after(async () => { await browser?.close(); });

async function open(opts = {}) {
  const ctx = await browser.newContext({ viewport: opts.viewport || { width: 1000, height: 900 } });
  const p = await ctx.newPage();
  await p.setContent(page(!!opts.fail, opts));
  await p.waitForSelector('[data-dcz-set]');
  return { p, close: () => ctx.close() };
}
const puts = (p) => p.evaluate(() => window.__puts);

test('it starts on the defaults', async () => {
  const { p, close } = await open();
  const got = await p.evaluate(() => ({
    set: document.querySelector('[data-dcz-set]:checked').value,
    colour: document.querySelector('[data-dcz-colour][aria-pressed="true"]').dataset.dczColour,
    succ: document.querySelector('[data-dcz-succ]:checked').value,
    fail: document.querySelector('[data-dcz-fail]:checked').value,
    quality: document.querySelector('[data-dcz-quality][aria-pressed="true"]').dataset.dczQuality,
    look: document.querySelector('[data-dcz-look][aria-pressed="true"]').dataset.dczLook,
    picks: document.querySelector('[data-dcz-picks]').checked,
    note: document.querySelector('.dcz').textContent.includes('Each player can change the die, colours and animations for their own rolls. Everyone else still sees the campaign’s.'),
  }));
  await close();
  assert.deepEqual(got, { set: 'jewelled', colour: 'onyx', succ: 'phoenix', fail: 'grasp', quality: 'auto', look: 'quick', picks: true, note: true });
});

test('each pick saves at once, as a partial body, and shows Saved', async () => {
  const { p, close } = await open();
  await p.click('[data-dcz-set="oak"]');
  await p.click('[data-dcz-colour="oak-b"]');
  await p.click('[data-dcz-quality="hd"]');
  await p.click('[data-dcz-succ="storm"]');
  await p.click('[data-dcz-fail="frost"]');
  await p.click('[data-dcz-look="rune"]');
  await p.click('[data-dcz-picks]');
  const saved = await p.textContent('.dcz-status');
  const sent = await puts(p);
  await close();
  assert.deepEqual(sent, [
    { diceSet: 'oak', diceColour: '' }, { diceColour: 'oak-b' }, { quality: 'hd' },
    { critSucc: 'storm' }, { critFail: 'frost' }, { look: 'rune' }, { playersPick: false },
  ]);
  assert.match(saved, /Saved/);
});

test('the sets on offer: unchecking saves the list, all checked saves "every set", and the campaign\'s own is locked', async () => {
  const { p, close } = await open();
  const locked = await p.$eval('[data-dcz-offer="jewelled"]', (e) => e.disabled);
  await p.click('[data-dcz-offer="bone"]');
  await p.click('[data-dcz-offer="bone"]');
  // Choosing a set that isn't on offer adds it to the offer.
  await p.click('[data-dcz-offer="bone"]');
  await p.click('[data-dcz-set="bone"]');
  const sent = await puts(p);
  const lockedAfter = await p.$eval('[data-dcz-offer="bone"]', (e) => e.disabled);
  await close();
  assert.equal(locked, true);
  assert.deepEqual(sent, [
    { diceOffered: ['jewelled', 'oak'] }, { diceOffered: [] }, { diceOffered: ['jewelled', 'oak'] },
    { diceSet: 'bone', diceColour: '', diceOffered: ['jewelled', 'oak', 'bone'] },
  ]);
  assert.equal(lockedAfter, true);
});

test('the offer list is only there while players may pick', async () => {
  const { p, close } = await open();
  const on = await p.$$eval('[data-dcz-offer]', (e) => e.length);
  await p.click('[data-dcz-picks]');
  const off = await p.$$eval('[data-dcz-offer]', (e) => e.length);
  await close();
  assert.deepEqual([on, off], [3, 0]);
});

test('a save the server refuses puts the pick back and says why', async () => {
  const { p, close } = await open({ fail: true });
  await p.click('[data-dcz-set="oak"]');
  await p.waitForSelector('.dcz-bad');
  const got = await p.evaluate(() => ({ set: document.querySelector('[data-dcz-set]:checked').value, msg: document.querySelector('.dcz-bad').textContent }));
  await close();
  assert.equal(got.set, 'jewelled');
  assert.match(got.msg, /That wasn’t saved\. Pick one of the dice sets\./);
});

test('every set is a tile with a picture of its die, in one radio group, however many sets there are', async () => {
  const { p, close } = await open();
  const got = await p.evaluate(() => ({
    tiles: [...document.querySelectorAll('.dcz-tile')].map((t) => t.textContent.trim()),
    radios: [...document.querySelectorAll('.dcz-tile input[type="radio"]')].map((i) => i.name),
    pictured: [...document.querySelectorAll('.dcz-tile img')].filter((i) => /^data:image\//.test(i.getAttribute('src') || '')).length,
    on: document.querySelector('.dcz-tile.is-on').textContent.trim(),
    note: document.querySelector('.dcz-die').textContent.includes('A note for jewelled'),
    oldTry: document.querySelectorAll('[data-dcz-roll="2d10"], [data-dcz-roll="d20"]').length,
  }));
  await close();
  assert.deepEqual(got.tiles, ['Jewelled', 'Oak', 'Bone']);
  assert.deepEqual(got.radios, ['dcz-set', 'dcz-set', 'dcz-set']);
  assert.equal(got.pictured, 3);
  assert.equal(got.on, 'Jewelled');
  assert.equal(got.note, true);
  assert.equal(got.oldTry, 0);
});

test('the pictures are drawn one at a time: the campaign\'s own die first, then the tiles on screen, then the rest', async () => {
  // Twenty-four sets on a short screen, the campaign's die being the last of them.
  const many = Array.from({ length: 24 }, (_, i) => ({ key: 's' + i, name: 'Set ' + i, note: '', colours: [{ key: 'c' + i, name: 'C', swatch: ['#ccc', '#333'] }] }));
  const { p, close } = await open({ viewport: { width: 1000, height: 340 }, portrait: true, catalogue: { ...CATALOGUE, sets: many }, start: { ...START, diceSet: 's23' } });
  await p.waitForFunction(() => window.__drawn.length === 25);
  const got = await p.evaluate(() => ({
    drawn: window.__drawn,
    seen: [...document.querySelectorAll('.dcz-tile')].filter((t) => { const r = t.getBoundingClientRect(); return r.bottom > 0 && r.top < innerHeight; }).map((t) => t.querySelector('input').value),
  }));
  await close();
  assert.deepEqual(got.drawn.slice(0, 2), ['s23|72', 's23|140'], 'the chosen tile and the tray come first');
  const onScreen = got.seen.filter((k) => k !== 's23').map((k) => k + '|72');
  assert.ok(onScreen.length > 0 && onScreen.length < 23, `some tiles are on screen and some not (${onScreen.length})`);
  assert.deepEqual(got.drawn.slice(2, 2 + onScreen.length), onScreen, 'then the tiles on screen');
  assert.equal(new Set(got.drawn).size, 25, 'each picture is drawn once');
});

test('the tray throws the campaign\'s die inside itself on each pick and each word, and stays put across redraws', async () => {
  const { p, close } = await open();
  const stage = await p.$('[data-dcz-stage]');
  await p.click('[data-dcz-set="oak"]');
  await p.click('[data-dcz-colour="oak-c"]');
  await p.click('[data-dcz-roll="again"]');
  await p.click('[data-dcz-roll="succ"]');
  await p.click('[data-dcz-roll="fail"]');
  const rolls = await p.evaluate(() => window.__rolls);
  const same = await p.evaluate((s) => s === document.querySelector('[data-dcz-stage]'), stage);
  const label = await p.textContent('[data-dcz-tray-lab]');
  await close();
  assert.deepEqual(rolls.map((r) => r.groups[0].dice), [
    [{ sides: 20 }], [{ sides: 20 }], [{ sides: 20 }], [{ sides: 20, value: 20, crit: 'succ' }], [{ sides: 20, value: 1, crit: 'fail' }],
  ]);
  assert.ok(rolls.every((r) => r.opts.inTray && r.opts.keep), 'every throw lands in the tray and stays there');
  assert.deepEqual(rolls[4].opts.table, { set: 'oak', colour: 'oak-c', succ: 'phoenix', fail: 'grasp', quality: 'auto' });
  assert.equal(same, true);
  assert.equal(label, 'Oak · Colour c');
});

test('on a phone it folds to one column with no sideways scroll, and every control is 44 tall', async () => {
  const { p, close } = await open({ viewport: { width: 360, height: 800 } });
  const got = await p.evaluate(() => ({
    over: document.documentElement.scrollWidth - innerWidth,
    small: [...document.querySelectorAll('.dcz-row, .dcz-seg button, .dcz-col, .dcz-chk, .dcz-tile, .dcz-switch')].filter((e) => e.getBoundingClientRect().height < 43.5).map((e) => e.className + ':' + Math.round(e.getBoundingClientRect().height)),
  }));
  await close();
  assert.ok(got.over <= 0, `scrolls sideways by ${got.over}px`);
  assert.deepEqual(got.small, []);
});
