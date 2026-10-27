#!/usr/bin/env node
// tools/craft-thumbs.mjs — draws the desk bench's build-mode tray pictures.
//
// Each picture is cut from the real scene with one choice made, so the tray
// shows exactly what a choice does. Run it after changing how the desk skin
// draws (static/js/widgets/craft_desk.js); it rewrites
// static/img/crafting/desk/<key>-<value>.jpg. Needs playwright-core and a
// Chromium (see test/js/_browser_env.mjs).
//
//   node tools/craft-thumbs.mjs            every picture
//   node tools/craft-thumbs.mjs cat look   only the renders that cut those
//   node tools/craft-thumbs.mjs cat-grey   only the render that cuts that one

import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { open, baked, root } from '../test/js/_craft_harness.mjs';

const OUT = join(root, 'static', 'img', 'crafting', 'desk');
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
const BOX = {
  wall: [990, 40, 160, 106], bench: [585, 388, 135, 90], light: [1022, 262, 140, 210], view: [686, 26, 288, 236],
  cat: [836, 192, 100, 104], cover: [40, 368, 430, 294], mood: [20, 20, 540, 330], look: [0, 0, 1200, 700],
  'kit-skull': [980, 402, 64, 64], 'kit-ball': [1138, 418, 58, 58], 'kit-scales': [586, 334, 118, 118], 'kit-mandrake': [112, 292, 76, 92],
};

async function shoot(set, take) {
  const { browser, page, errors } = await open({ width: 1200, height: 760, scale: 2, paintedSky: true });
  try {
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
const looks = ['apothecary', 'hold', 'hut', 'city'].map((l) => [{ look: l }, { look: l }]);
const only = process.argv.slice(2);
const cuts = ([set, take]) => Object.entries(take || set).flatMap(([k, v]) =>
  k.startsWith('kit-') ? ['kit', k] : set.look ? ['look', 'look-' + v] : [k, k + '-' + v]);
const all = RENDERS.concat(looks).filter((r) => !only.length || cuts(r).some((k) => only.includes(k)));
// One at a time: each render bakes the light and paints the sky in software
// at twice the size, and two at once starve each other until they time out.
for (const [set, take] of all) await shoot(set, take);
