// heading_buttons.spec.mjs — a button beside a heading keeps its sentence case
// in a campaign whose headings are capitals.
//
// The page look sets capitals on the heading's whole row, and a link drawn as
// a button inherits them where a real button does not, so "Create hero" (a
// link) read "CREATE HERO" beside "Add a map" (a button). A harness page with
// the real page_look_annals.css and style_kits.css around the page opening
// transcribed from layouts/app_opening.templ.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/heading_buttons.spec.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(repoRoot, p), 'utf8');
const annalsCSS = read('static/css/page_look_annals.css');
const kitsCSS = read('static/css/style_kits.css');
const openingTempl = read('internal/templates/layouts/app_opening.templ');

const BUTTON_CLASSES = ['btn', 'btn-primary', 'btn-secondary', 'btn-ghost', 'btn-danger', 'btn-warning', 'btn-success'];

// The capitals are written on the root by AppearanceCSS when the campaign's
// headings are Oswald. A style kit clears them and sets its own: Sharp puts
// every block heading, and so the opening's row, in capitals.
function harnessHTML(kit) {
  const links = BUTTON_CLASSES.map((c) => `<a href="#" class="${c}" data-t="link-${c}">Create hero</a>`).join('');
  return `<!doctype html>
<html lang="en" data-cz-page="annals" ${kit ? `data-cz-kit="${kit}"` : ''} style="--annals-hcase:uppercase">
<head><meta charset="utf-8"><style>${annalsCSS}</style><style>${kitsCSS}</style></head>
<body>
  <header class="mb-6 flex flex-wrap items-center gap-x-4 gap-y-2" data-app-opening="full">
    <h1 class="cb-title text-2xl font-bold text-fg">Characters</h1>
    <div class="flex min-w-0 flex-[1_1_16rem] flex-wrap items-center gap-x-4 gap-y-2 text-sm text-fg-secondary"><span>The cast.</span></div>
    <div class="ml-auto">
      <a href="#" class="btn-primary text-sm gap-2" data-t="action-link">Create hero</a>
      <button type="button" class="btn-primary text-sm gap-2" data-t="action-button">Add a map</button>
    </div>
  </header>
  <h2 class="cb-head">Posts ${links}</h2>
  <p class="cb-section-h">Facts <a href="#" class="btn-secondary" data-t="in-section">Edit</a></p>
  <a href="#" data-t="plain-link-in-heading-row">not a button</a>
</body></html>`;
}

const caseOf = (page, t) => page.$eval(`[data-t="${t}"]`, (e) => getComputedStyle(e).textTransform);

test('the opening draws its main action with a button class, as a link or a button', () => {
  const action = openingTempl.slice(openingTempl.indexOf('templ appOpeningAction'));
  assert.match(action, /<a href=\{[^}]+\} class="btn-primary /, 'the link form');
  assert.match(action, /<button type="button" class="btn-primary /, 'the button form');
  assert.match(openingTempl, /<header class="mb-6 flex[^"]*" data-app-opening="full">/, 'the opening row is plain; only its title is a heading');
  assert.doesNotMatch(openingTempl, /<header class="[^"]*cb-head/, 'no opening header is a block heading');
});

for (const kit of ['', 'sharp']) {
  test(`capital headings${kit ? ' under the ' + kit + ' kit' : ''}: the heading is capitals, a button beside it is not`, async () => {
    const browser = await chromium.launch();
    try {
      const page = await browser.newPage();
      await page.setContent(harnessHTML(kit));
      if (!kit) assert.equal(await page.$eval('h1.cb-title', (e) => getComputedStyle(e).textTransform), 'uppercase', 'the title keeps its capitals');
      assert.notEqual(await page.$eval('[data-app-opening] div.ml-auto', (e) => getComputedStyle(e).textTransform), 'uppercase', 'the opening row is not capitals');
      assert.equal(await page.$eval('h2.cb-head', (e) => getComputedStyle(e).textTransform), 'uppercase', 'a block heading keeps its capitals');
      assert.equal(await caseOf(page, 'action-button'), 'none', 'a real button');
      assert.equal(await caseOf(page, 'action-link'), 'none', 'a link drawn as a button');
      for (const c of BUTTON_CLASSES) assert.equal(await caseOf(page, 'link-' + c), 'none', c);
      assert.equal(await caseOf(page, 'in-section'), 'none', 'a button in a section heading');
    } finally {
      await browser.close();
    }
  });
}
