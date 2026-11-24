// cutscene_workshop_browser.test.mjs — the editor's Preview button, in
// Chromium: it posts the form, then plays the answer over the whole window
// exactly as a player's stage does, a modal dialog the size of the window,
// never inside a box on the page. At phone width too, and without a console
// error.
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
const ENGINE = readFileSync(join(root, 'static/js/cutscene_engine.js'));
const WORKSHOP = readFileSync(join(root, 'internal/plugins/cutscenes/static/js/cutscene_workshop.js'));

// The editor reduced to what Preview reads: the mount naming the engine, the
// form naming the preview route, the error line and the button. The empty
// box is the old in-page preview's; a page cached from before must still get
// the full-window preview.
const PAGE = `<!doctype html><html><head><meta charset="utf-8"></head><body>
<div data-engine-src="/engine.js"><form data-csw-form data-preview-url="/preview">
<input name="name" value="The Eye Opens"><p data-csw-err hidden></p>
<div data-csw-preview-box style="width:320px;height:180px" hidden></div>
<button type="button" id="pv" onclick="window.ChronicleCutsceneWorkshop.preview(this)">Preview</button>
</form></div>
<script>window.Chronicle = { apiFetch: function (u, o) { return fetch(u, o); } };</script>
<script src="/workshop.js"></script></body></html>`;

const SCENE = {
  id: '', name: 'The Eye Opens',
  background: { kind: 'fall-asleep', seconds: 1, params: {} },
  overlays: [{ kind: 'eye', at: 0.5, seconds: 0.5, params: { color: '#3dff7a' } }],
  text: [{ say: 'Something has noticed you.', at: 0.6, seconds: 0, style: 'whisper' }],
  ending: { kind: 'tap' }
};

async function open(t, viewport) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await (await browser.newContext({ viewport })).newPage();
  const errors = [];
  pg.on('pageerror', (e) => errors.push(String(e)));
  pg.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
  let posted = 0;
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/engine.js') return route.fulfill({ contentType: 'text/javascript', body: ENGINE });
    if (p === '/workshop.js') return route.fulfill({ contentType: 'text/javascript', body: WORKSHOP });
    if (p === '/preview') {
      posted++;
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify(SCENE) });
    }
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await pg.goto('http://t.test/');
  return { pg, errors, posted: () => posted };
}

for (const [name, viewport] of [['computer', { width: 1280, height: 800 }], ['phone', { width: 390, height: 844 }]]) {
  test(`${name}: Preview plays full window, as a player sees it`, { skip }, async (t) => {
    const { pg, errors, posted } = await open(t, viewport);
    await pg.click('#pv');
    const cut = pg.locator('.cs-cut');
    await cut.waitFor();
    assert.equal(posted(), 1, 'the form is posted to the preview route');
    const got = await cut.evaluate((el) => {
      const r = el.getBoundingClientRect();
      return { parent: el.parentElement === document.body, boxed: el.classList.contains('cs-cut-boxed'), modal: el.getAttribute('aria-modal'), w: r.width, h: r.height };
    });
    assert.ok(got.parent, 'played on the page body, like the player stage');
    assert.equal(got.boxed, false, 'not played in a box');
    assert.equal(got.modal, 'true', 'a modal dialog, as for players');
    assert.equal(got.w, viewport.width, 'as wide as the window');
    assert.equal(got.h, viewport.height, 'as tall as the window');
    // The scene gets as far as its words, so the preview is not a blank screen.
    await pg.getByText('Something has noticed you.').waitFor({ timeout: 5000 });
    assert.deepEqual(errors, []);
  });
}
