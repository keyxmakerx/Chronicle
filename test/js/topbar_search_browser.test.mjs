// topbar_search_browser.test.mjs — quick search (Ctrl+K in a campaign) lists
// recent pages before typing, offers to create a page when nothing matches
// (only for those who may create pages), and lists matching palette commands
// under the pages it finds.
//
// Runs static/js/topbar_search.js in a real Chromium against a stubbed search
// endpoint. Needs playwright-core and a Chromium (see _browser_env.mjs).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const SEARCH_JS = readFileSync(join(root, 'static/js/topbar_search.js'), 'utf8');

function body(canCreate) {
  return '<div id="topbar-content">Top</div>' +
    '<div id="topbar-search" class="hidden" data-can-create="' + canCreate + '">' +
    '<input id="topbar-search-input"><div id="topbar-search-results" class="hidden"></div></div>';
}

async function open(t, { canCreate = true, hits = [] } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.route('**/*', (r) => r.fulfill({ contentType: 'text/html', body: '<!doctype html><html><body>' + body(canCreate) + '</body></html>' }));
  await p.goto('http://chronicle.test/campaigns/c1/entities');
  await p.evaluate((hits) => {
    localStorage.setItem('chronicle-recent-c1', JSON.stringify([
      { id: 'e2', name: 'Mira Vant', type: 'npc', ts: 2 }, { id: 'e1', name: 'Port Rell', type: 'place', ts: 1 }]));
    window.ran = [];
    window.Chronicle = {
      escapeHtml: (s) => String(s).replace(/[&<>"']/g, (c) => '&#' + c.charCodeAt(0) + ';'),
      escapeAttr: (s) => String(s).replace(/[&<>"']/g, (c) => '&#' + c.charCodeAt(0) + ';'),
      apiFetch: () => Promise.resolve({ json: () => Promise.resolve({ results: hits }) }),
      paletteCommands: () => [
        { id: 'search', label: 'Search Pages', icon: 'fa-search' },
        { id: 'new-page', label: 'New Page', icon: 'fa-plus', href: '/campaigns/c1/entities/new' },
        { label: 'Toggle Theme', icon: 'fa-circle-half-stroke', action: () => window.ran.push('theme') },
      ],
    };
  }, hits);
  await p.addScriptTag({ content: SEARCH_JS });
  await p.evaluate(() => document.dispatchEvent(new Event('DOMContentLoaded')));
  await p.evaluate(() => Chronicle.openSearch());
  return p;
}
const rows = (p) => p.$$eval('#topbar-search-results a', (as) => as.map((a) => ({ text: a.textContent.trim(), href: a.getAttribute('href') })));
async function type(p, q) {
  await p.fill('#topbar-search-input', q);
  await p.waitForTimeout(350);
}

test('before typing, the recent pages are listed newest first', { skip }, async (t) => {
  const p = await open(t);
  const r = await rows(p);
  assert.deepEqual(r.map((x) => x.href), ['/campaigns/c1/entities/e2', '/campaigns/c1/entities/e1']);
  assert.match(await p.textContent('#topbar-search-results'), /^Recent/);
});

test('no match offers to create the page, with its name filled in', { skip }, async (t) => {
  const p = await open(t);
  await type(p, 'Ferryman');
  const r = await rows(p);
  assert.ok(r.some((x) => x.href === '/campaigns/c1/entities/new?name=Ferryman' && /Create a page named/.test(x.text)), JSON.stringify(r));
});

test('a viewer who may not create pages gets neither the create row nor New Page', { skip }, async (t) => {
  const p = await open(t, { canCreate: false });
  await type(p, 'new');
  const r = await rows(p);
  assert.ok(!r.some((x) => /Create a page named|New Page/.test(x.text)), JSON.stringify(r));
});

test('matching commands follow the pages, and Enter runs one', { skip }, async (t) => {
  const p = await open(t, { hits: [{ id: 'e9', name: 'Theme park', type_name: 'Place', url: '/campaigns/c1/entities/e9' }] });
  await type(p, 'theme');
  const r = await rows(p);
  assert.equal(r[0].text.startsWith('Theme park'), true);
  assert.ok(r.some((x) => x.text.startsWith('Toggle Theme')));
  assert.ok(!r.some((x) => /Search Pages|Create a page named/.test(x.text)), 'search never lists itself, and a match means no create row');
  await p.keyboard.press('ArrowDown');
  await p.keyboard.press('Enter');
  assert.deepEqual(await p.evaluate(() => window.ran), ['theme']);
});
