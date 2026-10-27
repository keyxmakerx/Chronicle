#!/usr/bin/env node
// tools/craft-thumbs.mjs — draws a bench skin's build-mode tray pictures.
//
// Each picture is cut from the real scene with one choice made, so the tray
// shows exactly what a choice does. Run it after changing how a skin draws
// (static/js/widgets/craft_desk.js or craft_journal.js); it rewrites
// static/img/crafting/<skin>/<key>-<value>.jpg. Needs playwright-core and a
// Chromium (see test/js/_browser_env.mjs).
//
//   node tools/craft-thumbs.mjs                  every desk picture
//   node tools/craft-thumbs.mjs cat look         only the renders that cut those
//   node tools/craft-thumbs.mjs cat-grey         only the render that cuts that one
//   node tools/craft-thumbs.mjs --skin=journal   the journal's pictures (same filters)

import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { open, baked, root } from '../test/js/_craft_harness.mjs';

const SKIN = (process.argv.find((a) => a.startsWith('--skin=')) || '--skin=desk').slice(7);
const OUT = join(root, 'static', 'img', 'crafting', SKIN);
const H = 152; // twice the tray's 76px, for sharp pictures on dense screens

// Each render sets a few choices at once and cuts one region per choice.
const RENDERS = [
  [{}, { wall: 'plaster', bench: 'oak', view: 'harbour', cat: 'black', cover: 'oxblood', light: 'candle', mood: 'moody' }],
  [{ wall: 'stone', bench: 'walnut', view: 'forest' }, null],
  [{ wall: 'timber', bench: 'pine', view: 'mountains' }, null],
  [{ wall: 'brick', bench: 'slate', view: 'city' }, null],
  [{ view: 'shutters' }, null],
  [{ light: 'lantern' }, null], [{ light: 'witch' }, null],
  [{ mood: 'cosy' }, null], [{ mood: 'grim' }, null],
  [{ cat: 'ginger', cover: 'green' }, null], [{ cat: 'grey', cover: 'navy' }, null], [{ cat: 'cream', cover: 'black' }, null],
  [{ kit: ['skull', 'ball', 'scales', 'mandrake'] }, { 'kit-skull': '', 'kit-ball': '', 'kit-scales': '', 'kit-mandrake': '' }],
];
const DESK_BOX = {
  wall: [990, 40, 160, 106], bench: [585, 388, 135, 90], light: [1022, 262, 140, 210], view: [686, 26, 288, 236],
  cat: [836, 192, 100, 104], cover: [40, 368, 430, 294], mood: [20, 20, 540, 330], look: [0, 0, 1200, 700],
  'kit-skull': [980, 402, 64, 64], 'kit-ball': [1138, 418, 58, 58], 'kit-scales': [586, 334, 118, 118], 'kit-mandrake': [112, 292, 76, 92],
};

/* The journal: renders that change one choice in several places at once
   only where those places don't touch, and the seal's pictures are cut from
   a recipe that has really been finished and sealed. */
const J_ALL = {
  stock: ['parchment', 'cotton', 'field', 'vellum', 'grimoire'], ink: ['gall', 'sepia', 'indigo', 'viridian', 'crimson', 'gold'],
  hand: ['kalam', 'caveat', 'apple', 'fine', 'fell'], seal: ['scarlet', 'oxblood', 'forest', 'midnight', 'black', 'gold'],
  emblem: ['moth', 'key', 'crescent', 'leaf', 'flask'], binding: ['oxblood', 'forest', 'black', 'tan', 'midnight'],
  light: ['candle', 'lantern', 'witch', 'none'], window: ['calendar', 'rain', 'snow', 'morning'], age: ['clean', 'worn', 'battered'],
};
const J_GROUPS = [['ink', 'seal'], ['hand', 'emblem', 'age'], ['binding'], ['stock'], ['light'], ['window']];
const J_RENDERS = J_GROUPS.flatMap((g) => {
  const n = Math.max(...g.map((k) => J_ALL[k].length)), out = [];
  for (let i = 0; i < n; i++) { const set = {}; g.forEach((k) => { if (J_ALL[k][i]) set[k] = J_ALL[k][i]; }); out.push([set, null]); }
  return out;
});
const J_BOX = {
  stock: [130, 52, 300, 225], ink: [140, 176, 260, 195], hand: [140, 90, 280, 210], seal: [470, 562, 100, 75], emblem: [478, 570, 84, 63],
  binding: [470, 560, 160, 120], light: [900, 20, 300, 225], window: [100, 40, 460, 345], age: [880, 30, 220, 165], look: [0, 0, 1200, 700],
};
const BOX = SKIN === 'journal' ? J_BOX : DESK_BOX;
const MOUNT = SKIN === 'journal' ? 'data-mode="play" data-skin="journal"' : 'data-mode="play"';

// Finish the first recipe and press its seal, so the page carries one.
async function sealOne(page) {
  for (let n = 0; n < 12; n++) {
    const done = await page.evaluate(async () => {
      const c = document.querySelector('[data-widget="craft_bench"]')._crb, wait = (ms) => new Promise((r) => setTimeout(r, ms));
      c.reset(); await wait(200); c.skip(); await wait(100);
      document.querySelector('.cr-acts .cr-btn-primary').click(); await wait(1500);
      const S = c.state(); if (S.points < S.r.goal) return false;
      const b = [...document.querySelectorAll('.cr-acts .cr-btn')].find((x) => /seal/i.test(x.textContent)); if (!b) return false;
      b.click(); await wait(3500); return true;
    });
    if (done) return;
  }
  throw new Error('the recipe never finished, so there is no seal to draw');
}

async function shoot(set, take) {
  const { browser, page, errors } = await open({ mounts: [MOUNT], width: 1200, height: 760, scale: 2, paintedSky: true });
  try {
    if (SKIN === 'journal') {
      await page.waitForSelector('.pj-ready', { timeout: 120000 });
      await page.addStyleTag({ content: '.cr-hint,.cr-card{display:none!important}' });
      if (['seal', 'emblem'].some((k) => k in set)) await sealOne(page);
    }
    await baked(page);
    await page.evaluate((s) => {
      const el = document.querySelector('[data-widget="craft_bench"]'), skin = el._crb.skin, box = el.querySelector('.crb-scene');
      if (s.look) skin.look(s.look); else Object.keys(s).forEach((k) => skin.setOpt(k, s[k]));
      ['time', 'weather', 'light'].forEach((k) => box.setAttribute('data-' + k, skin.opts[k]));
    }, set);
    await page.waitForTimeout(800);
    await baked(page, 0, 240000);
    await page.waitForTimeout(2500); // the window's sky paints, the flames settle
    for (const [k, v] of Object.entries(take || set)) {
      const key = set.look ? 'look' : k, b = BOX[key];
      const clip = await page.evaluate((b) => {
        const m = document.querySelector('.cr-svg').getScreenCTM();
        return { x: m.a * b[0] + m.e, y: m.d * b[1] + m.f, width: m.a * b[2], height: m.d * b[3] };
      }, b);
      const png = await page.screenshot({ clip, timeout: 120000 });
      // Shrink in the page: Playwright cannot resize, a canvas can.
      const jpg = await page.evaluate(async ({ data, h }) => {
        const img = new Image(); img.src = 'data:image/png;base64,' + data; await img.decode();
        const c = document.createElement('canvas'); c.height = h; c.width = Math.round(img.width * h / img.height);
        const x = c.getContext('2d'); x.imageSmoothingQuality = 'high'; x.drawImage(img, 0, 0, c.width, c.height);
        return c.toDataURL('image/jpeg', 0.82).split(',')[1];
      }, { data: png.toString('base64'), h: H });
      const name = set.look ? 'look-' + set.look : k.startsWith('kit-') ? k : k + '-' + v;
      writeFileSync(join(OUT, name + '.jpg'), Buffer.from(jpg, 'base64'));
      console.log('wrote', name + '.jpg');
    }
    if (errors.length) throw new Error('page errors: ' + errors.join('; '));
  } finally { await browser.close(); }
}

mkdirSync(OUT, { recursive: true });
const looks = (SKIN === 'journal' ? ['herbalist', 'grimoire', 'field', 'court'] : ['apothecary', 'hold', 'hut', 'city']).map((l) => [{ look: l }, { look: l }]);
const only = process.argv.slice(2).filter((a) => !a.startsWith('--'));
const cuts = ([set, take]) => Object.entries(take || set).flatMap(([k, v]) =>
  k.startsWith('kit-') ? ['kit', k] : set.look ? ['look', 'look-' + v] : [k, k + '-' + v]);
const all = (SKIN === 'journal' ? J_RENDERS : RENDERS).concat(looks).filter((r) => !only.length || cuts(r).some((k) => only.includes(k)));
// One at a time: each render bakes the light and paints the sky in software
// at twice the size, and two at once starve each other until they time out.
for (const [set, take] of all) await shoot(set, take);
