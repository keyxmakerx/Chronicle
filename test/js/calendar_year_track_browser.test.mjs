// calendar_year_track_browser.test.mjs — the year track's month names stay
// whole on a phone. A name wider than its month column, at either end of the
// track, would spill under the sticky row names (or past the track's end) and
// lose its first or last letters.
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
const css = readFileSync(join(root, 'static/css/calendar-view.css'), 'utf8');

// The track as calendar_year_track.js draws it: twelve months, one named.
const track = (cur) => {
  const months = Array.from({ length: 12 }, (_, i) => {
    const on = i === cur ? ' cur' : '';
    return `<button type="button" class="yt-mo${on}" style="left:${(i / 12 * 100).toFixed(4)}%;width:${(100 / 12).toFixed(4)}%"><span>Hammerfall</span></button>`;
  }).join('');
  return `<div class="cal-v5"><div class="yt-year"><div class="yt-track"><div class="yt-pan"><div class="yt-rows">
    <div class="yt-row yt-months"><span class="yt-ln">1523 DR</span><div class="yt-lb">${months}</div></div>
  </div></div></div></div></div>`;
};

for (const [label, cur] of [['first', 0], ['last', 11]]) {
  test(`the ${label} month's name is whole on a phone`, { skip }, async (t) => {
    const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
    t.after(() => browser.close());
    const p = await browser.newPage({ viewport: { width: 390, height: 600 } });
    await p.setContent(`<!doctype html><style>${css}</style><body style="margin:0">${track(cur)}</body>`);
    if (cur === 11) await p.$eval('.yt-track', (el) => { el.scrollLeft = el.scrollWidth; });
    const m = await p.evaluate(() => {
      const s = document.querySelector('.yt-mo.cur span').getBoundingClientRect();
      const lb = document.querySelector('.yt-lb').getBoundingClientRect();
      const ln = document.querySelector('.yt-ln').getBoundingClientRect();
      const tr = document.querySelector('.yt-track').getBoundingClientRect();
      return { left: s.left, right: s.right, lbLeft: lb.left, lbRight: lb.right, lnRight: ln.right, trRight: tr.right };
    });
    assert.ok(m.left >= m.lbLeft, `the name starts at ${m.left}, before the track's start ${m.lbLeft}`);
    assert.ok(m.right <= m.lbRight, `the name ends at ${m.right}, past the track's end ${m.lbRight}`);
  });
}
