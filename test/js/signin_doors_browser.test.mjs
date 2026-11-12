// signin_doors_browser.test.mjs — the sign-in scene follows the open tab,
// drifts under full motion, holds still under Calm and Off, and pauses when
// the tab is hidden.
//
// Runs auth_doors.css and signin_scene.js in a real Chromium. Needs
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

const SCENES = [['castle', 'signin'], ['dragon', 'new'], ['tavern', 'code']];
const LAYERS = ['back', 'mid', 'front', 'action'];

// The reduce rules mirror input.css's Calm and Off blocks: a keyframe
// animation ends at once, and with no fill it falls back to its resting place.
function page(attr) {
  const sets = SCENES.map(([s]) => `<div class="doors-set" data-scene="${s}">${LAYERS.map((l) => `<div class="doors-layer" data-layer="${l}"><img alt="" src="data:image/gif;base64,R0lGODlhAQABAAAAACw="></div>`).join('')}</div>`).join('');
  const tabs = SCENES.map(([, d], i) => `<a id="doors-tab-${d}" class="doors-tab" role="tab" href="#${d}" data-door="${d}" aria-selected="${i === 0}">${d}</a>`).join('');
  return `<!doctype html><html${attr}><head><style>${file('static/css/auth_doors.css')}
:root{--dur-ambient:2s;--dur-slide:50ms}
html[data-view-motion="calm"] *{animation-duration:.01ms !important;animation-iteration-count:1 !important}
html[data-motion="off"] *{animation-duration:.01ms !important;animation-iteration-count:1 !important;transition-duration:.01ms !important}
</style></head><body>
<div id="doors-stage" class="doors" data-door="signin" data-mode="scenes" data-widget="signin-scene">
<div class="doors-scene">${sets}</div>
<div class="doors-panel"><nav id="doors-tabs" role="tablist">${tabs}</nav></div></div>
<script>window.Chronicle={register:function(n,i){document.querySelectorAll('[data-widget="'+n+'"]').forEach(function(e){i.init(e)})}};</script>
<script>${file('static/js/widgets/signin_scene.js')}</script></body></html>`;
}

async function open(attr) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const p = await browser.newPage({ viewport: { width: 900, height: 600 } });
  await p.setContent(page(attr));
  return { browser, p };
}
const front = (p, scene) => p.evaluate((s) => getComputedStyle(document.querySelector(`.doors-set[data-scene=${s}] [data-layer=front]`)).transform, scene);

test('a tab click crossfades to its scene', { skip }, async () => {
  const { browser, p } = await open('');
  try {
    await p.evaluate(() => document.addEventListener('click', (e) => e.preventDefault(), true));
    for (const [scene, door] of SCENES) {
      await p.click(`#doors-tab-${door}`);
      await p.waitForTimeout(150);
      const op = await p.evaluate(() => Object.fromEntries([...document.querySelectorAll('.doors-set')].map((s) => [s.dataset.scene, getComputedStyle(s).opacity])));
      for (const [s] of SCENES) assert.equal(op[s], s === scene ? '1' : '0', `${door}: ${s}`);
    }
  } finally { await browser.close(); }
});

test('layers drift at different speeds under full motion', { skip }, async () => {
  const { browser, p } = await open('');
  try {
    const a = await front(p, 'castle');
    await p.waitForTimeout(400);
    assert.notEqual(await front(p, 'castle'), a, 'the front layer moves');
    const dur = await p.evaluate(() => ['back', 'front'].map((l) => parseFloat(getComputedStyle(document.querySelector(`.doors-set[data-scene=castle] [data-layer=${l}]`)).animationDuration)));
    assert.ok(dur[0] > dur[1], 'the back layer is slower than the front');
  } finally { await browser.close(); }
});

for (const [name, attr] of [['Calm', ' data-view-motion="calm"'], ['Off', ' data-motion="off"']]) {
  test(`${name} holds the scene still`, { skip }, async () => {
    const { browser, p } = await open(attr);
    try {
      await p.waitForTimeout(100);
      const a = await front(p, 'castle');
      await p.waitForTimeout(400);
      assert.equal(await front(p, 'castle'), a);
      assert.equal(a, 'none', 'rests in its home position');
    } finally { await browser.close(); }
  });
}

test('a hidden tab pauses the drift', { skip }, async () => {
  const { browser, p } = await open('');
  try {
    await p.evaluate(() => {
      Object.defineProperty(document, 'hidden', { value: true, configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    const a = await front(p, 'castle');
    await p.waitForTimeout(300);
    assert.equal(await front(p, 'castle'), a);
    assert.ok(await p.evaluate(() => document.getElementById('doors-stage').classList.contains('is-still')));
  } finally { await browser.close(); }
});
