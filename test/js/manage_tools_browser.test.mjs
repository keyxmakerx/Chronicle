// manage_tools_browser.test.mjs -- the owner's Manage tools in a real Chromium:
// "/" opening Find and never taking a typed slash, the Find field's ranking and
// keys, the list coming from the server, the pin's hand-over to the sidebar,
// and the Manage row's count (drawn, kept, dropped). Skips locally, and fails
// under CI, without a browser.

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (f) => readFileSync(join(root, f), 'utf8');
const SCRIPTS = {
  '/static/js/keyboard_shortcuts.js': read('static/js/keyboard_shortcuts.js'),
  '/static/js/find_palette.js': read('static/js/find_palette.js'),
  '/static/js/manage_find.js': read('static/js/manage_find.js'),
  '/static/js/manage_pin.js': read('static/js/manage_pin.js'),
  '/static/js/sidebar_nav.js': read('static/js/sidebar_nav.js'),
};

let shared = null;
after(async () => { if (shared) await (await shared).close(); });

const ENTRIES = [
  { label: 'People', path: 'Manage', href: '/campaigns/c/members', icon: 'fa-users', words: 'invite role' },
  { label: 'Trash', path: 'Manage', href: '/campaigns/c/trash', icon: 'fa-trash-can', words: 'deleted restore' },
  { label: 'Campaign name', path: 'Manage › The campaign', href: '/campaigns/c/dashboard#line-name', icon: 'fa-pen' },
  { label: 'Colours', path: 'Customize › Look', href: '/campaigns/c/customize/look#colours', icon: 'fa-palette', words: 'colour color' },
];

const PAGE = `<!doctype html><html><body>
<button type="button" id="find" data-manage-find data-find-url="/campaigns/c/dashboard/find" data-find-js="/static/js/find_palette.js" data-find-css="/static/css/find_palette.css" onclick="(function(b){var C=window.Chronicle=window.Chronicle||{};function go(){C.manageFind.open(b);}if(C.manageFind){go();return;}var s=document.createElement('script');s.src='/static/js/manage_find.js';s.onload=go;document.head.appendChild(s);})(this)">Find</button>
<input id="field"><textarea id="area"></textarea><select id="pick"><option>a</option></select><div id="edit" contenteditable="true">x</div><div id="plain" tabindex="0">p</div>
<aside id="sidebar"><div id="sidebar-nav-list"></div>
 <a class="nav-row" data-nav-key="manage" href="/campaigns/c/dashboard"><span class="nav-tr"></span></a></aside>
</body></html>`;

async function open({ find = true, findStatus = 200, count = 0, countStatus = 200, sidebar = true } = {}) {
  shared = shared || pw.chromium.launch({ executablePath: chromePath });
  const ctx = await (await shared).newContext();
  const page = await ctx.newPage();
  const seen = { find: 0, count: 0 };
  await page.route('http://t.test/**', async (route) => {
    const u = new URL(route.request().url());
    if (SCRIPTS[u.pathname]) return route.fulfill({ contentType: 'text/javascript', body: SCRIPTS[u.pathname] });
    if (u.pathname === '/static/css/find_palette.css') return route.fulfill({ contentType: 'text/css', body: '' });
    if (u.pathname.endsWith('/dashboard/find')) {
      seen.find++;
      return route.fulfill({ status: findStatus, contentType: 'application/json', body: JSON.stringify({ entries: ENTRIES }) });
    }
    if (u.pathname.endsWith('/dashboard/needs')) {
      seen.count++;
      return route.fulfill({ status: countStatus, contentType: 'application/json', body: JSON.stringify({ count: typeof count === 'function' ? count() : count }) });
    }
    return route.fulfill({ contentType: 'text/html', body: PAGE.replace(find ? '' : /<button[\s\S]*?<\/button>/, '').replace(sidebar ? '' : /<aside[\s\S]*<\/aside>/, '') });
  });
  await page.goto('http://t.test/');
  await page.evaluate(() => { window.Chronicle = window.Chronicle || {}; window.Chronicle.notify = (m) => { window.notified = m; }; window.Chronicle.escapeHtml = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])); });
  return { ctx, page, seen };
}

const load = (page, name) => page.addScriptTag({ url: `http://t.test/static/js/${name}` });

// --- "/" ------------------------------------------------------------------

test('"/" outside a field opens Find; the button and the key share one handler', { skip }, async () => {
  const { ctx, page, seen } = await open();
  try {
    await load(page, 'keyboard_shortcuts.js');
    await page.focus('#plain');
    await page.keyboard.press('/');
    await page.waitForSelector('.fnd-box');
    assert.equal(seen.find, 1);
    assert.equal(await page.getAttribute('#find', 'aria-expanded'), 'true');
    assert.equal(await page.evaluate(() => document.activeElement.tagName), 'INPUT', 'focus moves into the field');
    await page.keyboard.press('Escape');
    await page.waitForSelector('.fnd-box', { state: 'detached' });
    assert.equal(await page.getAttribute('#find', 'aria-expanded'), 'false');
    assert.equal(await page.evaluate(() => document.activeElement.id), 'plain', 'focus returns to where it was');
  } finally { await ctx.close(); }
});

for (const [id, what] of [['field', 'an input'], ['area', 'a textarea'], ['pick', 'a select'], ['edit', 'an editable element']]) {
  test(`"/" never fires while typing in ${what}, and the slash is not swallowed`, { skip }, async () => {
    const { ctx, page, seen } = await open();
    try {
      await load(page, 'keyboard_shortcuts.js');
      await page.evaluate(() => { window.slashDefaultPrevented = null; document.addEventListener('keydown', (e) => { if (e.key === '/') setTimeout(() => { window.slashDefaultPrevented = e.defaultPrevented; }, 0); }); });
      await page.focus('#' + id);
      await page.keyboard.press('/');
      await page.waitForTimeout(150);
      assert.equal(await page.$('.fnd-box'), null, 'Find opened while typing');
      assert.equal(seen.find, 0);
      assert.equal(await page.evaluate(() => window.slashDefaultPrevented), false, 'the key was prevented, so the slash could not be typed');
      if (id === 'field') assert.equal(await page.inputValue('#field'), '/');
    } finally { await ctx.close(); }
  });
}

test('"/" with Ctrl, Alt or Meta is not Find, and a page without the button leaves "/" alone', { skip }, async () => {
  const { ctx, page, seen } = await open();
  try {
    await load(page, 'keyboard_shortcuts.js');
    await page.focus('#plain');
    for (const combo of ['Control+/', 'Alt+/', 'Meta+/']) await page.keyboard.press(combo);
    await page.waitForTimeout(150);
    assert.equal(seen.find, 0);
  } finally { await ctx.close(); }
  const bare = await open({ find: false });
  try {
    await load(bare.page, 'keyboard_shortcuts.js');
    const prevented = await bare.page.evaluate(() => {
      const e = new KeyboardEvent('keydown', { key: '/', bubbles: true, cancelable: true });
      document.body.dispatchEvent(e);
      return e.defaultPrevented;
    });
    assert.equal(prevented, false, 'a page with no Find must not take the key');
  } finally { await bare.ctx.close(); }
});

// --- the field ------------------------------------------------------------

test('Find ranks a label hit first, matches hidden words, moves with arrows and goes on Enter', { skip }, async () => {
  const { ctx, page } = await open();
  try {
    await load(page, 'find_palette.js');
    const ranked = await page.evaluate((entries) => ({
      colour: window.Chronicle.findPalette.match(entries, 'colour').map((f) => f.label),
      deleted: window.Chronicle.findPalette.match(entries, 'deleted').map((f) => f.label),
      manage: window.Chronicle.findPalette.match(entries, 'manage').map((f) => f.label),
      none: window.Chronicle.findPalette.match(entries, 'zzz').length,
    }), ENTRIES);
    assert.deepEqual(ranked.colour, ['Colours'], 'a hidden word and a label word both find it');
    assert.deepEqual(ranked.deleted, ['Trash'], 'words that are not shown still match');
    assert.deepEqual(ranked.manage, ['People', 'Trash', 'Campaign name'], 'path matches keep the server order');
    assert.equal(ranked.none, 0);

    await page.evaluate((entries) => { window.went = null; window.Chronicle.findPalette.open({ finds: entries, onGo: (f) => { window.went = f.href; } }); }, ENTRIES);
    await page.keyboard.type('a');
    await page.keyboard.press('ArrowDown');
    const sel = await page.$eval('.fnd-res [aria-selected="true"] .fnd-t', (e) => e.textContent);
    await page.keyboard.press('Enter');
    const went = await page.evaluate(() => window.went);
    assert.ok(went && went.startsWith('/campaigns/c/'), 'Enter goes to the chosen setting');
    assert.ok(ENTRIES.find((e) => e.label === sel && e.href === went), 'it went to the highlighted row');
    assert.equal(await page.$('.fnd-box'), null, 'the field closes on going');
  } finally { await ctx.close(); }
});

test('Find says so when nothing matches, and the list is fetched when it opens', { skip }, async () => {
  const { ctx, page, seen } = await open();
  try {
    await page.click('#find');
    await page.waitForSelector('.fnd-box');
    assert.equal(seen.find, 1);
    await page.keyboard.type('qqqq');
    assert.match(await page.textContent('.fnd-none'), /Nothing matches/);
    await page.keyboard.press('Escape');
    await page.click('#find');
    await page.waitForSelector('.fnd-box');
    assert.equal(seen.find, 2, 'a fresh list each time, so a switched-off tool never lingers');
  } finally { await ctx.close(); }
});

test('a failing list says so and does not leave a dead field open', { skip }, async () => {
  const { ctx, page } = await open({ findStatus: 500 });
  try {
    await page.click('#find');
    await page.waitForFunction(() => window.notified);
    assert.match(await page.evaluate(() => window.notified), /Find could not load/);
    assert.equal(await page.$('.fnd-box'), null);
    assert.equal(await page.getAttribute('#find', 'aria-expanded'), 'false');
  } finally { await ctx.close(); }
});

// --- the pin --------------------------------------------------------------

const PIN = `<button type="button" id="pin" class="manage-pin" data-manage-pin="manage:trash" data-pin-name="Trash" aria-pressed="false">p</button>`;

test('the pin hands the press to the sidebar, keeps focus, and ignores a second press while saving', { skip }, async () => {
  const { ctx, page } = await open();
  try {
    await page.evaluate((html) => { document.body.insertAdjacentHTML('beforeend', html); }, PIN);
    await page.evaluate(() => {
      window.calls = [];
      let release;
      window.release = () => release();
      window.Chronicle.navEditor = {
        setPersonalPin: (key, on, name, opts) => { window.calls.push({ key, on, name, opts }); return new Promise((r) => { release = () => r(true); }); },
        syncManagePins: () => { window.synced = (window.synced || 0) + 1; },
      };
    });
    await load(page, 'manage_pin.js');
    await page.evaluate(() => { window.p1 = window.Chronicle.managePin.toggle(document.getElementById('pin')); window.p2 = window.Chronicle.managePin.toggle(document.getElementById('pin')); });
    assert.equal(await page.evaluate(() => window.calls.length), 1, 'a second press while saving is ignored');
    assert.deepEqual(await page.evaluate(() => window.calls[0]), { key: 'manage:trash', on: true, name: 'Trash', opts: { keepFocus: true } });
    assert.equal(await page.getAttribute('#pin', 'aria-busy'), 'true');
    await page.evaluate(() => window.release());
    await page.evaluate(() => window.p1);
    assert.equal(await page.getAttribute('#pin', 'aria-busy'), null);
    assert.equal(await page.evaluate(() => window.synced), 1);
    // A pinned button unpins.
    await page.evaluate(() => { document.getElementById('pin').setAttribute('aria-pressed', 'true'); window.Chronicle.managePin.toggle(document.getElementById('pin')); });
    assert.equal(await page.evaluate(() => window.calls[1].on), false);
  } finally { await ctx.close(); }
});

test('the pin says so when the sidebar is not there to save it', { skip }, async () => {
  const { ctx, page } = await open();
  try {
    await page.evaluate((html) => { document.body.insertAdjacentHTML('beforeend', html); }, PIN);
    await load(page, 'manage_pin.js');
    await page.evaluate(() => window.Chronicle.managePin.toggle(document.getElementById('pin')));
    assert.match(await page.evaluate(() => window.notified), /sidebar is not ready/);
    assert.equal(await page.getAttribute('#pin', 'aria-busy'), null);
  } finally { await ctx.close(); }
});

// --- the Manage row's count ---------------------------------------------------

const rowState = (page) => page.$eval('[data-nav-key="manage"]', (r) => ({
  text: r.querySelector('.nav-tr').textContent, needs: r.getAttribute('data-needs'), label: r.getAttribute('aria-label'),
}));

test('the Manage row shows what needs the owner, keeps it a minute, and drops it as things are fixed', { skip }, async () => {
  let n = 2;
  const { ctx, page, seen } = await open({ count: () => n });
  try {
    await load(page, 'sidebar_nav.js');
    await page.waitForFunction(() => document.querySelector('[data-nav-key="manage"] .nav-tr').textContent !== '');
    assert.deepEqual(await rowState(page), { text: '2', needs: '2', label: 'Manage, 2 things need you' });
    assert.equal(seen.count, 1);
    // Kept: a second ask is answered from the keep.
    await page.evaluate(() => window.Chronicle.nav.refreshCount(false));
    assert.equal(seen.count, 1, 'a fresh keep is not asked again');
    // Something was fixed: asking with force reads the server and the count drops.
    n = 1;
    await page.evaluate(() => window.Chronicle.nav.refreshCount(true));
    assert.deepEqual(await rowState(page), { text: '1', needs: '1', label: 'Manage, 1 thing needs you' });
    n = 0;
    await page.evaluate(() => window.Chronicle.nav.refreshCount(true));
    assert.deepEqual(await rowState(page), { text: '', needs: null, label: null }, 'nothing waiting draws nothing');
    // A page that knows the number sets it without a request.
    const before = seen.count;
    await page.evaluate(() => window.Chronicle.nav.setCount(3));
    assert.equal((await rowState(page)).text, '3');
    assert.equal(seen.count, before);
  } finally { await ctx.close(); }
});

test('a failed count leaves the row as it was', { skip }, async () => {
  const { ctx, page } = await open({ countStatus: 500 });
  try {
    await load(page, 'sidebar_nav.js');
    await page.waitForTimeout(300);
    assert.deepEqual(await rowState(page), { text: '', needs: null, label: null });
  } finally { await ctx.close(); }
});
