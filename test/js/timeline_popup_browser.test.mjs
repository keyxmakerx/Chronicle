// timeline_popup_browser.test.mjs — the owner's choice of popup on time lines
// (Customize › Hover cards › Popups on time lines). The lean line under a
// page's title opens the fading corner when <html data-cz-tlpop="corner">,
// and the classic hover card otherwise; the corner sits over its bead on a
// stem, flipping to the bead's left near the right edge and below it near
// the top of the window.
//
// Runs hovercard.js and page_timeline.js in a real Chromium. Needs
// playwright-core and a Chromium (see _browser_env.mjs); skips locally, and
// fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const file = (p) => readFileSync(join(root, p), 'utf8');

function bead(left, name) {
  return `<li style="left:${left}%" data-ptl-at="${left}" data-ptl-year="1000"><a class="ptl-ev" href="/e/${name}" aria-label="${name}" data-ptl-name="${name}" data-ptl-date="Thaw 9, 1000"></a></li>`;
}

function page(attr, top) {
  return `<!doctype html><html${attr}><head><style>${file('static/css/paper.css')}${file('static/css/page_timeline.css')}
body{margin:0;padding:${top}px 16px 400px}</style></head><body>
<section class="ptl" data-widget="page_timeline"><div class="ptl-rail"><div class="ptl-in"><ol class="ptl-dots">${bead(10, 'Early')}${bead(50, 'Middle')}${bead(100, 'Late')}</ol></div></div></section>
<script>window.Chronicle={register:function(n,i){document.querySelectorAll('[data-widget="'+n+'"]').forEach(function(e){i.init(e)})}};</script>
<script>${file('static/js/hovercard.js')}</script><script>${file('static/js/widgets/page_timeline.js')}</script>
</body></html>`;
}

// Focuses the named bead and reports the card next to the bead's centre.
async function openOn(attr, top, name, width = 800) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const p = await browser.newPage({ viewport: { width, height: 600 } });
  await p.setContent(page(attr, top));
  await p.focus(`.ptl-ev[aria-label="${name}"]`);
  await p.waitForSelector('#chc.chc--show');
  await p.waitForTimeout(250);
  const got = await p.evaluate((n) => {
    const b = document.querySelector(`.ptl-ev[aria-label="${n}"]`).getBoundingClientRect();
    const c = document.getElementById('chc'), r = c.getBoundingClientRect();
    return { vw: window.innerWidth, look: c.dataset.look, cls: c.className, cx: b.left + b.width / 2, cy: b.top + b.height / 2, l: r.left, r: r.right, t: r.top, b: r.bottom, text: c.innerText };
  }, name);
  await browser.close();
  return got;
}

test('the classic card is used unless the owner picked the corner', { skip }, async () => {
  const c = await openOn('', 200, 'Middle');
  assert.notEqual(c.look, 'corner');
  assert.doesNotMatch(c.cls, /chc--c[rd]/);
  assert.match(c.text, /Middle/);
});

test('the corner sits above its bead, its edge on the bead\'s centre', { skip }, async () => {
  const c = await openOn(' data-cz-tlpop="corner"', 200, 'Middle');
  assert.equal(c.look, 'corner');
  assert.doesNotMatch(c.cls, /chc--c[rd]/);
  assert.ok(Math.abs(c.l - (c.cx - 0.75)) < 1, `left ${c.l} vs bead centre ${c.cx}`);
  assert.ok(c.b <= c.cy - 14 && c.b >= c.cy - 16, `bottom ${c.b} vs bead centre ${c.cy}`);
  assert.match(c.text, /Middle/);
});

test('near the right edge the corner flips to the bead\'s left', { skip }, async () => {
  const c = await openOn(' data-cz-tlpop="corner"', 200, 'Late');
  assert.match(c.cls, /chc--cr/);
  assert.ok(Math.abs(c.r - (c.cx + 0.75)) < 1, `right ${c.r} vs bead centre ${c.cx}`);
});

test('near the top of the window the corner flips below its bead', { skip }, async () => {
  const c = await openOn(' data-cz-tlpop="corner"', 20, 'Early');
  assert.match(c.cls, /chc--cd/);
  assert.ok(c.t >= c.cy + 14 && c.t <= c.cy + 16, `top ${c.t} vs bead centre ${c.cy}`);
});

test('on a phone the corner narrows to stay in the window, its edge still on the bead', { skip }, async () => {
  const c = await openOn(' data-cz-tlpop="corner"', 200, 'Middle', 340);
  assert.ok(c.l >= 7 && c.r <= c.vw - 7, `card ${c.l}..${c.r} in a ${c.vw}px window`);
  const edge = /chc--cr/.test(c.cls) ? c.r - 0.75 : c.l + 0.75;
  assert.ok(Math.abs(edge - c.cx) < 1, `edge ${edge} vs bead centre ${c.cx}`);
});
