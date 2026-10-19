// tools/token_frames_previews.mjs: redraws static/img/token_frames.webp, the
// still previews the token maker's picker shows (one sprite, so the picker
// never draws thirty frames live).
//
//   node tools/token_frames_previews.mjs
//
// Run it after adding, removing, reordering or restyling a design in
// static/js/token_frames.js. Each frame is drawn still, in its own colours,
// around the engine's painted sample figure, in the engine's design order:
// TILE pixels square, COLS to a row. token_maker.js reads the sprite with the
// same COLS, and test/js/token_maker_browser.test.mjs fails when the sprite no
// longer matches the engine. Needs playwright-core and a Chromium, found the
// way the browser tests find them (test/js/_browser_env.mjs).

import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from '../test/js/_browser_env.mjs';

const TILE = 128, COLS = 8;

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
if (skip) { console.error('token_frames_previews: ' + skip); process.exit(1); }

const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
try {
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setContent('<!doctype html><html><body></body></html>');
  await page.addScriptTag({ content: readFileSync(join(root, 'static/js/token_frames.js'), 'utf8') });
  const out = await page.evaluate(({ TILE, COLS }) => {
    const F = window.ChronicleTokenFrames, art = F.sample(), place = F.place(art);
    const rows = Math.ceil(F.designs.length / COLS), sheet = document.createElement('canvas');
    sheet.width = COLS * TILE; sheet.height = rows * TILE;
    const g = sheet.getContext('2d'), tile = document.createElement('canvas');
    tile.width = tile.height = TILE;
    F.designs.forEach((d, i) => {
      F.render(tile.getContext('2d'), TILE, TILE, { img: art, place, frame: d.id, colour: '', shape: 'round', t: 0, still: true });
      g.drawImage(tile, (i % COLS) * TILE, Math.floor(i / COLS) * TILE);
    });
    return { url: sheet.toDataURL('image/webp', 0.86), ids: F.designs.map((d) => d.id) };
  }, { TILE, COLS });
  if (errors.length) throw new Error(errors.join('; '));
  const file = join(root, 'static/img/token_frames.webp');
  writeFileSync(file, Buffer.from(out.url.split(',')[1], 'base64'));
  console.log('wrote static/img/token_frames.webp: ' + out.ids.length + ' frames, ' + out.ids.join(' '));
} finally {
  await browser.close();
}
