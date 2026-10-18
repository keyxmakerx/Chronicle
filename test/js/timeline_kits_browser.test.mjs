// timeline_kits_browser.test.mjs — a style kit draws the lean line and both
// of its popups from its --kit-tl-* tokens (style_kits.css). The classic card
// is drawn in the kit only while a kit is on, and the Customizer's sample
// popups keep Material.
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

function bead(left, name, cls = '') {
  return `<li style="left:${left}%" data-ptl-at="${left}" data-ptl-year="1000"><a class="ptl-ev ${cls}" href="/e/${name}" aria-label="${name}" data-ptl-name="${name}" data-ptl-date="Thaw 9, 1000"><span class="ptl-yr">1000</span></a></li>`;
}

function page(attr) {
  return `<!doctype html><html${attr}><head><style>${file('static/css/paper.css')}${file('static/css/page_timeline.css')}${file('static/css/style_kits.css')}
body{margin:0;padding:200px 16px 400px} :root{--color-accent:#6366f1;--color-card-bg:#fff}</style></head><body>
<main id="main-content"><h1 class="cb-title">Ilvara</h1>
<section class="ptl" data-widget="page_timeline"><div class="ptl-rail"><div class="ptl-tr"></div><div class="ptl-wa"></div><div class="ptl-in"><ol class="ptl-dots">${bead(10, 'Early')}${bead(50, 'Middle')}${bead(100, 'Late', 'ptl-ev--later')}</ol></div></div></section></main>
<span class="tlp-sample"><span class="chc chc--show" data-look="corner"><i class="chc__stem"></i><span class="chc__in">Sample</span></span></span>
<script>window.Chronicle={register:function(n,i){document.querySelectorAll('[data-widget="'+n+'"]').forEach(function(e){i.init(e)})}};</script>
<script>${file('static/js/hovercard.js')}</script><script>${file('static/js/widgets/page_timeline.js')}</script>
</body></html>`;
}

// Opens the middle bead's popup and reads back how the line, a bead, the
// popup and the Customizer's sample corner are drawn.
async function draw(attr) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const p = await browser.newPage({ viewport: { width: 800, height: 600 } });
  await p.setContent(page(attr));
  await p.focus('.ptl-ev[aria-label="Middle"]');
  await p.waitForSelector('#chc.chc--show');
  const got = await p.evaluate(() => {
    const cs = (el, pseudo) => getComputedStyle(el, pseudo);
    const tr = cs(document.querySelector('.ptl-tr'));
    const ev = cs(document.querySelector('.ptl-ev[aria-label="Early"]'));
    const c = document.getElementById('chc'), inn = c.querySelector('.chc__in');
    const sample = document.querySelector('.tlp-sample .chc__in');
    return {
      look: c.dataset.look, lineH: tr.height, lineMask: tr.maskImage, wash: cs(document.querySelector('.ptl-wa')).display,
      beadW: ev.width, beadR: ev.borderTopLeftRadius, beadSh: ev.boxShadow,
      edge: cs(inn, '::after'), cardBorder: cs(inn).borderLeftStyle + ' ' + cs(inn).borderLeftWidth, cardTop: cs(inn).borderTopWidth,
      cardRadius: cs(inn).borderTopLeftRadius, cardBg: cs(inn).backgroundColor,
      sampleEdge: cs(sample, '::after').borderLeftWidth, sampleR: cs(sample).borderBottomLeftRadius,
    };
  });
  await browser.close();
  return got;
}

test('with no kit the line and its classic card are as they were', { skip }, async () => {
  const d = await draw(' data-cz-page="annals"');
  assert.equal(d.look, 'paper');
  assert.equal(d.lineH, '1px');
  assert.equal(d.lineMask, 'none');
  assert.equal(d.wash, 'block');
  assert.equal(d.beadW, '11px');
  assert.equal(d.beadR, '50%');
});

test('Sharp: a two-pixel rule with no wash, square beads, a square ink frame under a heavy rule', { skip }, async () => {
  const d = await draw(' data-cz-page="annals" data-cz-kit="sharp"');
  assert.equal(d.look, 'kit');
  assert.equal(d.lineH, '2px');
  assert.equal(d.wash, 'none');
  assert.equal(d.beadW, '10px');
  assert.equal(d.beadR, '0px');
  assert.match(d.cardBorder, /^solid 1(\.5)?px$/);
  assert.equal(d.cardTop, '4px');
  assert.equal(d.cardRadius, '0px');
  assert.equal(d.cardBg, 'rgb(251, 250, 247)');
});

test('Ornate: a double gilt line and corner, beads ringed in gilt', { skip }, async () => {
  const d = await draw(' data-cz-page="annals" data-cz-kit="ornate" data-cz-tlpop="corner"');
  assert.equal(d.look, 'corner');
  assert.equal(d.lineH, '4px');
  assert.match(d.lineMask, /linear-gradient/);
  assert.match(d.beadSh, /rgb\(138, 131, 80\) 0px 0px 0px 3px/);
  assert.equal(d.edge.borderLeftStyle, 'double');
  assert.equal(d.edge.borderLeftWidth, '3px');
  assert.equal(d.edge.borderLeftColor, 'rgb(138, 131, 80)');
  assert.equal(d.cardRadius, '0px');
});

test('Flowing and Sketchbook draw the line through a masked stroke', { skip }, async () => {
  for (const k of ['flowing', 'sketchbook']) {
    const d = await draw(` data-cz-page="annals" data-cz-kit="${k}" data-cz-tlpop="corner"`);
    assert.equal(d.lineH, '8px', k);
    assert.match(d.lineMask, /^url\("data:image\/svg\+xml/, k);
    assert.notEqual(d.beadR, '50%', k);
  }
});

test('the Customizer\'s sample corner keeps Material whatever the kit', { skip }, async () => {
  const d = await draw(' data-cz-page="annals" data-cz-kit="ornate" data-cz-tlpop="corner"');
  assert.match(d.sampleEdge, /^1(\.5)?px$/);
  assert.equal(d.sampleR, '12px');
});
