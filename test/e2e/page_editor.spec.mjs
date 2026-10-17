// page_editor.spec.mjs — Playwright coverage for the page editor
// (static/js/widgets/page_editor.js) in the Annals look.
//
// A harness page, as in sky_pane.spec.mjs: the real htmx, boot.js,
// aliases.js and page_editor.js, and the real page_look_annals.css, read off
// disk; the markup is the page editor's top row and header part transcribed
// from show.templ and page_editor.templ (page_editor_test.go pins the same
// attributes server-side). Only the network is stubbed, with the shapes the
// real handlers answer; any other request fails the test.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/page_editor.spec.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, '..', '..');
const BASE = '/campaigns/c1/entities';

// The few layout utilities the transcribed markup leans on, so the page lays
// out as it does with the built stylesheet.
const UTILITIES = `
  body { font: 16px/1.5 system-ui, sans-serif; margin: 0; padding: 24px; }
  [hidden] { display: none !important; }
  .flex { display: flex; } .items-start { align-items: flex-start; } .items-center { align-items: center; }
  .justify-between { justify-content: space-between; } .gap-2 { gap: .5rem; } .gap-3 { gap: .75rem; }
  .min-w-0 { min-width: 0; } .flex-1 { flex: 1 1 0%; } .mb-4 { margin-bottom: 1rem; }
  h1 { margin: 0; font-size: 1.875rem; }
`;

function markup() {
  const pen = `<button type="button" class="pe-pen" data-pe-pen="header" aria-expanded="false" aria-label="Edit the name and details" title="Edit the name and details"><svg class="pe-i pe-i-pen" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 20h9"></path></svg><svg class="pe-i pe-i-ok" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7"></path></svg></button>`;
  return `
<div class="max-w-7xl mx-auto" data-entity-type-slug="people" data-pe-page>
  <div class="flex items-start justify-between gap-3 mb-4">
    <nav class="min-w-0 flex-1"><a href="${BASE}">Pages</a> / <span data-page-crumb>Mara Vell</span></nav>
    <div class="pe-bar" data-widget="page-editor" data-entity-id="e1" data-campaign-id="c1"
      data-metadata-endpoint="${BASE}/e1/metadata" data-aliases-endpoint="${BASE}/e1/aliases"
      data-search-endpoint="${BASE}/search" data-name="Mara Vell">
      <span class="pe-hint" data-pe-hint>Click a part or its pen · <button type="button" class="pe-key" data-pe-mark aria-pressed="false">E</button> outlines them all · <kbd class="pe-key">Esc</kbd> steps back</span>
      <span class="pe-saved" data-pe-saved role="status" aria-live="polite"></span>
      <button type="button" data-visibility-badge="everyone">Everyone</button>
      <button type="button" class="btn-secondary btn-sm pe-edit" data-pe-edit aria-pressed="false">Edit page</button>
    </div>
  </div>
  <div class="pe-part" data-pe-part="header">
    <div class="flex items-start justify-between gap-3"><div class="min-w-0">
      <div class="flex items-center gap-2"><h1 class="cb-title" data-page-h1>Mara Vell</h1><button type="button" data-favorite-toggle="e1" data-entity-name="Mara Vell">star</button>${pen}</div>
      <div data-widget="aliases" data-entity-id="e1" data-campaign-id="c1" data-editable="false"></div>
    </div></div>
    <div class="pe-drape" data-pe-drape inert><div class="pe-drape-in"><div class="pe-sheet">
      <label class="pe-row"><span>Also known as</span><input type="text" class="pe-fld" data-pe-f="aka" autocomplete="off" disabled></label>
      <label class="pe-row"><span>Descriptor</span><input type="text" class="pe-fld" data-pe-f="label" maxlength="100" value="Ship's captain" autocomplete="off"></label>
      <div class="pe-row"><label for="pe-parent">Parent page</label><div class="pe-combo">
        <input id="pe-parent" type="text" class="pe-fld" role="combobox" aria-autocomplete="list" aria-expanded="false" aria-controls="pe-parent-list" autocomplete="off" data-pe-f="parent" data-id="p1" value="Highwater">
        <ul id="pe-parent-list" class="pe-list" role="listbox" aria-label="Pages" hidden></ul>
      </div></div>
      <div class="pe-qws"><button type="button" class="pe-qw">History</button></div>
    </div></div></div>
    <div id="page-history-slot"></div>
  </div>
  <div style="height:400px">The rest of the page.</div>
  <span data-page-label>Ship's captain</span>
</div>`;
}

function harnessHTML(motion) {
  const scripts = ['vendor/htmx.min.js', 'js/boot.js', 'js/widgets/aliases.js', 'js/widgets/page_editor.js']
    .map((p) => `<script src="/static/${p}" defer></script>`).join('\n');
  return `<!doctype html>
<html lang="en" data-cz-page="annals"${motion ? ` data-motion="${motion}"` : ''}>
<head><meta charset="utf-8"><style>${UTILITIES}</style><link rel="stylesheet" href="/static/css/page_look_annals.css"></head>
<body><main id="main-content">${markup()}</main>
${scripts}
</body></html>`;
}

async function withPage(opts, fn) {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: opts.viewport || { width: 1280, height: 800 } });
    const unexpected = [];
    const pageErrors = [];
    const puts = [];
    page.on('pageerror', (err) => pageErrors.push(err.message));
    await page.route('**/*', async (route) => {
      const req = route.request();
      const p = new URL(req.url()).pathname;
      const json = (o) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(o) });
      if (p === '/page') return route.fulfill({ status: 200, contentType: 'text/html; charset=utf-8', body: harnessHTML(opts.motion) });
      if (p.startsWith('/static/')) {
        const type = p.endsWith('.css') ? 'text/css' : 'text/javascript';
        return route.fulfill({ status: 200, contentType: type, body: readFileSync(join(repoRoot, p.slice(1)), 'utf8') });
      }
      if (p === `${BASE}/e1/aliases` && req.method() === 'GET') return json({ aliases: [{ alias: 'the Grey Gull' }] });
      if (req.method() === 'PUT' && (p === `${BASE}/e1/aliases` || p === `${BASE}/e1/metadata`)) {
        const body = JSON.parse(req.postData() || '{}');
        puts.push({ path: p.slice(BASE.length + 4), body });
        return json({ status: 'ok', name: body.name });
      }
      if (p === `${BASE}/search`) return json({ results: [{ id: 'p1', name: 'Highwater' }, { id: 'p2', name: 'The Salt Coast' }, { id: 'e1', name: 'Mara Vell' }] });
      unexpected.push(`${req.method()} ${p}`);
      return route.fulfill({ status: 404, body: '' });
    });
    await page.goto('https://chronicle.test/page');
    await page.waitForFunction(() => window.Chronicle && window.Chronicle._pageEditor && document.querySelector('[data-widget="aliases"]').textContent.includes('Gull'));
    await fn(page, puts);
    assert.deepEqual(unexpected, [], 'no unexpected requests');
    assert.deepEqual(pageErrors, [], 'no uncaught errors');
  } finally {
    await browser.close();
  }
}

// Where everything a reader sees sits, so a test can show nothing moved.
function boxes(page) {
  return page.evaluate(() => ['[data-page-crumb]', '[data-page-h1]', '[data-favorite-toggle]', '[data-pe-pen]',
    '[data-widget="aliases"]', '[data-visibility-badge]', '[data-pe-edit]', '[data-page-label]']
    .map((s) => { const r = document.querySelector(s).getBoundingClientRect(); return `${s} ${Math.round(r.x)},${Math.round(r.y)}`; }));
}

test('Edit page and opening the header move nothing on the page', { timeout: 30000 }, async () => {
  await withPage({}, async (page) => {
    const rest = await boxes(page);
    assert.equal(await page.$eval('[data-pe-pen]', (e) => getComputedStyle(e).visibility), 'hidden');
    await page.click('[data-pe-edit]');
    assert.equal(await page.textContent('[data-pe-edit]'), 'Done');
    assert.equal(await page.$eval('[data-pe-page]', (e) => e.hasAttribute('data-editing')), true);
    await page.waitForTimeout(350);
    assert.deepEqual(await boxes(page), rest, 'Edit page moved something');
    await page.click('[data-pe-pen]');
    await page.waitForTimeout(350);
    assert.deepEqual(await boxes(page), rest, 'opening the header moved something');
    assert.equal(await page.$eval('[data-pe-drape]', (e) => getComputedStyle(e).visibility + ' ' + e.inert), 'visible false');
    assert.equal(await page.$eval('[data-pe-pen]', (e) => e.getAttribute('aria-label')), 'Done');
  });
});

test('the name saves on its own key, and an E typed into it is a letter', { timeout: 30000 }, async () => {
  await withPage({}, async (page, puts) => {
    await page.click('[data-pe-edit]');
    await page.click('[data-pe-pen]');
    await page.waitForFunction(() => document.activeElement && document.activeElement.hasAttribute('data-page-h1'));
    await page.keyboard.type('e');
    assert.equal(await page.$eval('[data-pe-page]', (e) => e.hasAttribute('data-pe-marked')), false);
    await page.waitForFunction(() => document.querySelector('[data-pe-saved]').textContent.includes('Saved'));
    assert.deepEqual(puts, [{ path: 'metadata', body: { name: 'Mara Velle' } }]);
    assert.equal(await page.textContent('[data-page-crumb]'), 'Mara Velle');
  });
});

test('a half-typed other name waits; a whole one saves the list', { timeout: 30000 }, async () => {
  await withPage({}, async (page, puts) => {
    await page.click('[data-pe-edit]');
    await page.click('[data-pe-pen]');
    await page.waitForFunction(() => !document.querySelector('[data-pe-f="aka"]').disabled);
    await page.click('[data-pe-f="aka"]');
    await page.keyboard.press('End');
    await page.keyboard.type(', T');
    await page.waitForTimeout(1200);
    assert.deepEqual(puts, []);
    await page.keyboard.type('ide');
    await page.waitForFunction(() => document.querySelector('[data-pe-saved]').textContent.includes('Saved'));
    assert.deepEqual(puts, [{ path: 'aliases', body: { aliases: ['the Grey Gull', 'Tide'] } }]);
  });
});

test('a parent picked from the list saves at once; Esc steps back one thing at a time', { timeout: 30000 }, async () => {
  await withPage({}, async (page, puts) => {
    await page.click('[data-pe-edit]');
    await page.click('[data-pe-pen]');
    await page.click('[data-pe-f="parent"]');
    await page.waitForSelector('#pe-parent-list:not([hidden]) [role="option"]');
    const options = await page.$$eval('#pe-parent-list [role="option"]', (ls) => ls.map((l) => l.textContent));
    assert.deepEqual(options, ['No parent page', 'Highwater', 'The Salt Coast'], 'the page itself is never offered');
    await page.click('#pe-parent-list [role="option"]:nth-child(3)');
    await page.waitForFunction(() => document.querySelector('[data-pe-saved]').textContent.includes('Saved'));
    assert.deepEqual(puts, [{ path: 'metadata', body: { parent_id: 'p2' } }]);

    await page.keyboard.press('ArrowDown');
    await page.waitForSelector('#pe-parent-list:not([hidden])');
    await page.keyboard.press('Escape');
    assert.equal(await page.$eval('#pe-parent-list', (e) => e.hidden), true, 'Esc closes the list first');
    assert.equal(await page.$eval('[data-pe-part]', (e) => e.classList.contains('is-open')), true);
    await page.keyboard.press('Escape');
    assert.equal(await page.$eval('[data-pe-part]', (e) => e.classList.contains('is-open')), false, 'then the part');
    assert.equal(await page.evaluate(() => document.activeElement.getAttribute('data-pe-pen')), 'header');
    await page.keyboard.press('e');
    assert.equal(await page.$eval('[data-pe-page]', (e) => e.hasAttribute('data-pe-marked')), true, 'E outlines the parts');
    await page.keyboard.press('Escape');
    assert.equal(await page.textContent('[data-pe-edit]'), 'Edit page', 'then editing');
  });
});

test('Calm fades the sheet instead of unrolling it', { timeout: 30000 }, async () => {
  await withPage({ motion: 'calm' }, async (page) => {
    await page.click('[data-pe-edit]');
    await page.click('[data-pe-pen]');
    assert.equal(await page.$eval('[data-pe-drape]', (e) => getComputedStyle(e).clipPath), 'none');
    await page.waitForFunction(() => getComputedStyle(document.querySelector('[data-pe-drape]')).opacity === '1');
  });
});
