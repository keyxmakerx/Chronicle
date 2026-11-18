// topbar_search_browser.test.mjs — quick search (Ctrl+K in a campaign) lists
// recent pages before typing, offers to create a page when no page carries the
// words in its name (only for those who may create pages), and lists the
// viewer's matching notes and matching palette commands under the pages it
// finds. Ctrl+K opens this search, the same one the header button opens.
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
const MODAL_JS = readFileSync(join(root, 'static/js/search_modal.js'), 'utf8');

function body(canCreate) {
  return '<div id="topbar-content">Top</div>' +
    '<div id="topbar-search" class="hidden" data-can-create="' + canCreate + '">' +
    '<input id="topbar-search-input"><div id="topbar-search-results" class="hidden"></div></div>';
}

async function open(t, { canCreate = true, hits = [], notes = [], viaKey = false } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.route('**/*', (r) => r.fulfill({ contentType: 'text/html', body: '<!doctype html><html><body>' + body(canCreate) + '</body></html>' }));
  await p.goto('http://chronicle.test/campaigns/c1/entities');
  await p.evaluate(({ hits, notes }) => {
    localStorage.setItem('chronicle-recent-c1', JSON.stringify([
      { id: 'e2', name: 'Mira Vant', type: 'npc', ts: 2 }, { id: 'e1', name: 'Port Rell', type: 'place', ts: 1 }]));
    window.ran = [];
    window.Chronicle = {
      escapeHtml: (s) => String(s).replace(/[&<>"']/g, (c) => '&#' + c.charCodeAt(0) + ';'),
      escapeAttr: (s) => String(s).replace(/[&<>"']/g, (c) => '&#' + c.charCodeAt(0) + ';'),
      apiFetch: (url) => Promise.resolve({ json: () => Promise.resolve(/\/notes$/.test(url) ? notes : { results: hits }) }),
      paletteCommands: () => [
        { id: 'search', label: 'Search Pages', icon: 'fa-search' },
        { id: 'new-page', label: 'New page', icon: 'fa-plus', href: '/campaigns/c1/entities/new' },
        { label: 'Toggle Theme', icon: 'fa-circle-half-stroke', action: () => window.ran.push('theme') },
      ],
    };
  }, { hits, notes });
  // The page loads the modal first and the top bar's search after it, as
  // base.templ does; the modal owns the Ctrl+K key.
  await p.addScriptTag({ content: MODAL_JS });
  await p.addScriptTag({ content: SEARCH_JS });
  await p.evaluate(() => document.dispatchEvent(new Event('DOMContentLoaded')));
  if (viaKey) await p.keyboard.press('Control+k');
  else await p.evaluate(() => Chronicle.openSearch());
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

test('a viewer who may not create pages gets neither the create row nor New page', { skip }, async (t) => {
  const p = await open(t, { canCreate: false });
  await type(p, 'new');
  const r = await rows(p);
  assert.ok(!r.some((x) => /Create a page named|New page/.test(x.text)), JSON.stringify(r));
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

test('Ctrl+K opens the top bar search, not the old modal, and closes it again', { skip }, async (t) => {
  const p = await open(t, { viaKey: true });
  const state = () => p.evaluate(() => ({
    bar: !document.getElementById('topbar-search').classList.contains('hidden'),
    focused: document.activeElement && document.activeElement.id,
    modal: !!document.querySelector('.fixed.inset-0'),
  }));
  await p.waitForFunction(() => document.activeElement && document.activeElement.id === 'topbar-search-input');
  assert.deepEqual(await state(), { bar: true, focused: 'topbar-search-input', modal: false });
  assert.match(await p.textContent('#topbar-search-results'), /^Recent/, 'the shortcut lands on the search with recent pages');
  await p.keyboard.press('Control+k');
  assert.equal((await state()).bar, false, 'the same key closes it');
  assert.equal((await state()).modal, false);
});

test('a page that only mentions the words does not hide the offer to create one', { skip }, async (t) => {
  const p = await open(t, { hits: [{ id: 'e3', name: 'Saltmere', type_name: 'Place', url: '/campaigns/c1/entities/e3' }] });
  await type(p, 'ferryman');
  const r = await rows(p);
  assert.equal(r[0].text.startsWith('Saltmere'), true, 'the mention is still listed, and first');
  assert.ok(r.some((x) => x.href === '/campaigns/c1/entities/new?name=ferryman'), JSON.stringify(r));
});

test('something that is not a page, named like the query, does not hide the offer either', { skip }, async (t) => {
  const p = await open(t, { hits: [{ name: 'Ferryman Row', type_name: 'Map', url: '/campaigns/c1/maps/m1' }] });
  await type(p, 'ferryman');
  assert.ok((await rows(p)).some((x) => /Create a page named/.test(x.text)));
});

test('the viewer\'s notes with a matching title follow the pages and open in the Journal', { skip }, async (t) => {
  const p = await open(t, {
    hits: [{ id: 'e9', name: 'Tide tables', type_name: 'Item', url: '/campaigns/c1/entities/e9' }],
    notes: [
      { id: 'n1', title: 'Tide notes for session 3' },
      { id: 'n2', title: 'Tide folder', isFolder: true },
      { id: 'n3', title: 'Old tide plan', archived: true },
      { id: 'n4', title: 'Shopping list' },
      { id: 'n5', title: 'Tide jot', entityId: 'e9' },
    ],
  });
  await type(p, 'tide');
  const r = await rows(p);
  assert.deepEqual(r.map((x) => x.text.replace(/\s+/g, ' ')), ['Tide tablesItem', 'Tide notes for session 3Note', 'Tide jotNote', 'View all results']);
  assert.equal(r[1].href, '/campaigns/c1/journal/n1');
  await p.evaluate(() => { window.opened = []; window.addEventListener('chronicle:open-note', (e) => window.opened.push(e.detail.noteId)); });
  await p.click('#topbar-search-results a:nth-of-type(3)');
  assert.deepEqual(await p.evaluate(() => window.opened), ['n5'], 'a jot opens on its page\'s notes panel');
});
