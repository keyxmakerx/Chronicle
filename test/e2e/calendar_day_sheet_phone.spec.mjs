// calendar_day_sheet_phone.spec.mjs — the day sheet on a phone scrolls to its
// last row and ends above the docked Table bar instead of behind it.
//
// A harness page with the real calendar-view.css and a stand-in for the bar
// (--bottom-bar is what table.css sets where the bar docks).
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/calendar_day_sheet_phone.spec.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');
const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const css = readFileSync(join(repoRoot, 'static/css/calendar-view.css'), 'utf8');

const events = Array.from({ length: 12 }, (_, i) =>
  `<div class="evd go"><div class="evh"><span class="qt0">Event ${i}</span></div></div>`).join('');

const html = `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1">
<style>html,body{margin:0}${css}
html{--bottom-bar:38px}.bar{position:fixed;left:0;right:0;bottom:0;height:38px;z-index:60}</style></head><body>
<div class="cal-v5"><div class="cal"><div class="wing live open" id="w">
<div class="leaf lf1"><div class="crease"><span>Day</span></div><div class="wb wbh"><h3>Date</h3></div></div>
<div class="leaf lf2"><div class="lscroll"><div class="wb wbr"><div class="evlist">${events}</div>
<div class="sect">Moons</div><div class="moonsec"><div id="last" style="height:60px">last</div></div></div></div></div>
</div></div></div><div class="bar"></div></body></html>`;

test('phone day sheet scrolls to its end above the Table bar', async () => {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ viewport: { width: 390, height: 844 }, hasTouch: true });
    await page.setContent(html);
    const r = await page.evaluate(() => {
      const w = document.getElementById('w');
      const scrollable = w.scrollHeight > w.clientHeight;
      w.scrollTop = 1e5;
      return {
        scrollable,
        sheetBottom: w.getBoundingClientRect().bottom,
        lastBottom: document.getElementById('last').getBoundingClientRect().bottom,
        barTop: innerHeight - 38,
      };
    });
    assert.ok(r.scrollable, 'sheet scrolls');
    assert.ok(r.sheetBottom <= r.barTop + 0.5, 'sheet ends above the bar');
    assert.ok(r.lastBottom <= r.barTop + 0.5, 'last row clear of the bar');
  } finally {
    await browser.close();
  }
});
