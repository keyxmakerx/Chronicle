// editor_mention_rows.test.mjs — pins the rows of the list that opens when a
// page is linked with @ while writing. A row is the page's name, then its
// kind as a quiet word at the end; a category's icon stands in front only when
// the campaign asks for it, and by default nothing is drawn there. A page not
// everyone can read says so on a line under its name, and Foundry scenes line
// up with the pages above them. The list stays inside the window: it opens
// above the cursor when there is no room under it, and clear of the sides.
//
// Runs the real editor and the real picker in Chromium against a stubbed
// search (needs playwright-core, see _browser_env.mjs), wired the way
// editor.js wires them.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';
import { KIND_ICON_CSS, KIND_ICON_TOKENS } from './_kind_icon_css.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const MENTION_JS = readFileSync(join(root, 'static/js/widgets/editor_mention.js'), 'utf8');
// The list is placed by Chronicle.placeAtCaret, which lives in theme.js.
const THEME_JS = readFileSync(join(root, 'static/js/theme.js'), 'utf8');
const TIPTAP_JS = readFileSync(join(root, 'static/vendor/tiptap-bundle.min.js'), 'utf8');

const page = (id, name, kind, more = {}) => ({ id, name, url: '/campaigns/c1/entities/' + id, type_name: kind, type_icon: 'fa-user', type_color: '#336699', ...more });

async function open(t, { results = [], scenes = [], hostOpen = false, viewport = { width: 1280, height: 720 }, style = '', icons = '', colour = '' } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport });
  const p = await ctx.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await ctx.route('**/*', (r) => {
    const u = new URL(r.request().url());
    if (u.pathname.endsWith('/entities/search')) return r.fulfill({ contentType: 'application/json', body: JSON.stringify({ results }) });
    // The site's own icon rules, and the attributes a campaign's choices put on <html>.
    const attrs = (icons ? ' data-cz-icons="' + icons + '"' : '') + (colour ? ' data-cz-icon-colour="' + colour + '"' : '');
    return r.fulfill({ contentType: 'text/html', body: '<!doctype html><html' + attrs + '><head><style>' + KIND_ICON_TOKENS + KIND_ICON_CSS + '</style></head><body style="margin:0"><div id="ed" style="' + style + '"></div></body></html>' });
  });
  await p.goto('https://chronicle.test/campaigns/c1/entities/e1');
  await p.addScriptTag({ content: TIPTAP_JS });
  await p.evaluate((scenes) => {
    const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
    window.Chronicle = {
      apiFetch: (url, opts) => fetch(url, opts), escapeHtml: esc, escapeAttr: esc,
      FoundryScenes: { want: () => {}, pickerItems: () => scenes },
    };
  }, scenes);
  await p.addScriptTag({ content: THEME_JS });
  await p.addScriptTag({ content: MENTION_JS });
  await p.evaluate((hostOpen) => {
    const ext = window.Chronicle.MentionExtension({ campaignId: 'c1', hostOpen });
    window.ed = new window.TipTap.Editor({
      element: document.getElementById('ed'),
      extensions: [window.TipTap.StarterKit.configure({ link: false }), window.Chronicle.MentionLink.configure({ openOnClick: false })],
      editable: true,
      content: '<p></p>',
      editorProps: { handleKeyDown: (view, event) => ext.onKeyDown(null, event) },
    });
    ext.onCreate(window.ed);
    window.ed.on('update', () => ext.onUpdate(window.ed));
  }, hostOpen);
  await p.click('#ed .ProseMirror');
  await p.keyboard.type('See @Tho');
  await p.waitForSelector('.mention-popup__item');
  return p;
}

// What each row of the open list shows, read from the page as drawn.
const rows = (p) => p.$$eval('.mention-popup__item', (els) => els.map((el) => {
  const line = el.firstElementChild;
  const name = el.querySelector('.mention-popup__name');
  const kind = el.querySelector('.mention-popup__kind');
  const box = el.getBoundingClientRect();
  return {
    name: name.textContent,
    kind: kind ? kind.textContent : '',
    // The first thing drawn on the line: a hidden icon slot does not count.
    nameLeads: [...line.children].find((c) => getComputedStyle(c).display !== 'none') === name,
    kindEnds: !kind || line.lastElementChild === kind,
    inset: Math.round(name.getBoundingClientRect().left - box.left),
    kindRight: kind ? Math.round(box.right - kind.getBoundingClientRect().right) : null,
    icons: [...el.querySelectorAll('i')].filter((i) => getComputedStyle(i).display !== 'none').map((i) => i.className),
    kindIcon: (() => { const i = el.querySelector('i.kind-ic'); return i ? { shown: getComputedStyle(i).display !== 'none', cls: i.className, style: i.getAttribute('style') || '', colour: getComputedStyle(i).color } : null; })(),
    under: el.children[1] ? el.children[1].textContent.trim() : '',
    nameCut: name.scrollWidth > name.clientWidth,
    kindCut: kind ? kind.scrollWidth > kind.clientWidth : false,
    width: Math.round(box.width),
    html: el.outerHTML,
  };
}));

test('by default a page is listed by its name, then its kind, with nothing drawn in front', { skip }, async (t) => {
  const p = await open(t, { results: [page('e2', 'Thorne', 'Character'), page('e3', 'Thornwall Keep', 'Location', { type_icon: 'fa-map-pin', type_color: '#ef4444' })] });
  const r = await rows(p);
  assert.deepEqual(r.map((x) => [x.name, x.kind]), [['Thorne', 'Character'], ['Thornwall Keep', 'Location']]);
  for (const x of r) {
    assert.ok(x.nameLeads && x.kindEnds, x.html);
    assert.deepEqual(x.icons, [], 'no icon stands in front of a name');
    assert.equal(x.under, '', 'an open page has one line');
    assert.equal(x.inset, 12, 'the name starts at the row\'s own edge');
    assert.equal(x.kindRight, 12, 'the kind ends at the row\'s other edge');
    assert.equal(x.kindIcon && x.kindIcon.shown, false, 'the category\'s icon is there but hidden');
  }
});

test('with icons beside names the category\'s icon stands in front of the name, and the kind still ends the row', { skip }, async (t) => {
  for (const amount of ['names', 'all']) {
    const p = await open(t, { icons: amount, results: [page('e2', 'Thorne', 'Character'), page('e3', 'Thornwall Keep', 'Location', { type_icon: 'fa-map-pin', type_color: '#ef4444' })] });
    const r = await rows(p);
    assert.equal(r.length, 2);
    for (const x of r) {
      assert.ok(x.kindIcon.shown && x.icons.length === 1, amount + ': ' + x.html);
      assert.ok(x.kindEnds, x.html);
      assert.ok(x.inset > 12, 'the icon takes room before the name');
      assert.equal(x.kindRight, 12);
    }
    assert.match(r[1].kindIcon.cls, /fa-map-pin/);
    assert.equal(r[0].inset, r[1].inset, 'names line up');
    assert.equal(r[1].kindIcon.colour, 'rgb(100, 107, 120)', 'Ink: the muted text colour');
  }
});

test('the icon colour theme reaches the link list: Accent and Category', { skip }, async (t) => {
  const results = [page('e3', 'Thornwall Keep', 'Location', { type_icon: 'fa-map-pin', type_color: '#ef4444' }), page('e4', 'Thornfield', 'Place', { type_icon: 'fa-tree', type_color: '#16a34a' })];
  const accent = await rows(await open(t, { icons: 'names', colour: 'accent', results }));
  assert.equal(accent[0].kindIcon.colour, 'rgb(99, 102, 241)');
  const cat = await rows(await open(t, { icons: 'names', colour: 'category', results }));
  assert.equal(cat[0].kindIcon.style.includes('--kind-colour:#ef4444'), true);
  assert.notEqual(cat[0].kindIcon.colour, cat[1].kindIcon.colour);
});

test('an icon name or colour that is not plain is never put in markup', { skip }, async (t) => {
  const p = await open(t, { icons: 'names', colour: 'category', results: [
    page('e3', 'Thornwall Keep', 'Location', { type_icon: 'fa-map-pin', type_color: 'red;background:url(//x.test/a)' }),
    page('e4', 'Thornfield', 'Place', { type_icon: 'fa-x" onmouseover="alert(1)', type_color: '#16a34a' }),
  ] });
  const r = await rows(p);
  assert.ok(!/url\(|onmouseover|red;/.test(r.map((x) => x.html).join('')));
  assert.equal(r[0].kindIcon.style.includes('--kind-colour'), false);
  assert.equal(r[1].kindIcon, null, 'an icon that is not a Font Awesome name is dropped');
});

test('a long name gives way, and a long kind is cut before it crowds the name out', { skip }, async (t) => {
  const p = await open(t, {
    results: [
      page('e2', 'Thorne of the Seven Drowned Bells, Keeper of the Lantern at Saltmere', 'Character'),
      page('e3', 'Thornwall', 'Noble houses of the eastern reach and their sworn bannermen'),
    ],
  });
  const r = await rows(p);
  assert.ok(r[0].nameCut && !r[0].kindCut, 'the name is cut and its kind is whole');
  assert.equal(r[0].kind, 'Character');
  assert.ok(r[1].kindCut && !r[1].nameCut, 'a very long kind is cut, and the short name beside it is whole');
  for (const x of r) assert.ok(x.width <= 320, 'the list keeps its width: ' + x.width);
});

test('a page not everyone can read says so on a line under its name', { skip }, async (t) => {
  const p = await open(t, { results: [page('e2', 'Thorne', 'Character', { restricted: 'dm_only' }), page('e3', 'Thornwall', 'Location', { restricted: 'custom' })] });
  const r = await rows(p);
  assert.deepEqual(r.map((x) => [x.name, x.kind, x.under]), [
    ['Thorne', 'Character', 'Hidden from players'],
    ['Thornwall', 'Location', 'Only some players can see it'],
  ]);
  for (const x of r) {
    assert.ok(x.nameLeads && x.kindEnds, x.html);
    assert.equal(x.icons.length, 1, 'the lock is the one mark, and it sits with the note');
    assert.match(x.icons[0], /fa-lock/);
  }
});

test('linking such a page into text players read warns under the name, and links on the second pick', { skip }, async (t) => {
  const p = await open(t, { hostOpen: true, results: [page('e2', 'Thorne', 'Character', { restricted: 'dm_only' })] });
  await p.keyboard.press('Enter');
  const r = await rows(p);
  assert.equal(r.length, 1);
  assert.deepEqual([r[0].name, r[0].kind], ['Thorne', 'Character']);
  assert.match(r[0].under, /^Players read this text and will see this name\./);
  assert.equal(await p.locator('#ed a[data-mention-id]').count(), 0, 'nothing is linked by the first pick');
  await p.keyboard.press('Enter');
  await p.waitForFunction(() => document.querySelector('#ed a[data-mention-id="e2"]'));
});

test('Foundry scenes line up with the pages above them', { skip }, async (t) => {
  const p = await open(t, {
    results: [page('e2', 'Thorne', 'Character')],
    scenes: [
      { kind: 'scene', sceneId: 's1', name: 'Thornwood Camp', folder: 'Act 2' },
      { kind: 'scene', sceneId: 's2', name: 'Thorn Gate', folder: '' },
      { kind: 'scene', sceneId: null, name: 'Tho', needed: true },
    ],
  });
  assert.equal(await p.textContent('.mention-popup__group'), 'Foundry scenes');
  const r = await rows(p);
  assert.deepEqual(r.map((x) => [x.name, x.kind, x.under]), [
    ['Thorne', 'Character', ''],
    ['Thornwood Camp', 'Act 2', ''],
    ['Thorn Gate', 'Top level', ''],
    ['“Tho”', '', 'Not in Foundry yet: mark the map as needed'],
  ]);
  for (const x of r) {
    assert.deepEqual(x.icons, [], x.html);
    assert.equal(x.inset, 12, 'every name starts at the same edge');
  }
});

// Where the open list sits against the cursor's line and the window.
const placed = (p) => p.evaluate(() => {
  const b = document.querySelector('.mention-popup').getBoundingClientRect();
  const c = window.getSelection().getRangeAt(0).getBoundingClientRect();
  return { top: Math.round(b.top), bottom: Math.round(b.bottom), left: Math.round(b.left), right: Math.round(b.right),
    cursorTop: Math.round(c.top), cursorBottom: Math.round(c.bottom), cursorLeft: Math.round(c.left), vw: window.innerWidth, vh: window.innerHeight };
});
const MANY = Array.from({ length: 9 }, (_, i) => page('e' + (i + 2), 'Thorne ' + (i + 1), 'Character'));

test('with room under the cursor the list opens there, starting at the cursor', { skip }, async (t) => {
  const p = await open(t, { results: MANY, style: 'position:absolute;top:40px;left:60px;width:600px' });
  const at = await placed(p);
  assert.ok(at.top >= at.cursorBottom && at.top - at.cursorBottom <= 6, JSON.stringify(at));
  assert.ok(Math.abs(at.left - at.cursorLeft) <= 1, JSON.stringify(at));
  assert.ok(at.bottom <= at.vh, 'the whole list is inside the window');
});

test('with no room under the cursor the list opens above it, whole', { skip }, async (t) => {
  const p = await open(t, { results: MANY, style: 'position:absolute;bottom:12px;left:60px;width:600px' });
  const at = await placed(p);
  assert.ok(at.bottom <= at.cursorTop && at.cursorTop - at.bottom <= 6, 'the list ends just above the cursor\'s line: ' + JSON.stringify(at));
  assert.ok(at.top >= 0 && at.bottom <= at.vh, JSON.stringify(at));
  assert.equal(at.bottom - at.top, 240, 'a full list keeps its full height');
});

test('a short list above the cursor still touches the cursor\'s line', { skip }, async (t) => {
  const p = await open(t, { results: [page('e2', 'Thorne', 'Character')], style: 'position:absolute;bottom:12px;left:60px;width:600px' });
  const at = await placed(p);
  assert.ok(at.bottom <= at.cursorTop && at.cursorTop - at.bottom <= 6, JSON.stringify(at));
});

test('on a narrow window the list stays clear of the right edge', { skip }, async (t) => {
  const p = await open(t, { results: MANY, viewport: { width: 390, height: 700 }, style: 'position:absolute;top:40px;left:250px;width:130px' });
  const at = await placed(p);
  assert.ok(at.right <= at.vw - 8 && at.left >= 8, 'inside the window with its margin: ' + JSON.stringify(at));
});

test('in a window too short for a full list either way, the list takes the larger side and scrolls', { skip }, async (t) => {
  const p = await open(t, { results: MANY, viewport: { width: 800, height: 300 }, style: 'position:absolute;top:120px;left:60px;width:600px' });
  const at = await placed(p);
  assert.ok(at.top >= 0 && at.bottom <= at.vh, 'no part of the list is outside the window: ' + JSON.stringify(at));
  assert.ok(at.bottom - at.top >= 96, 'and it keeps a usable height');
});
