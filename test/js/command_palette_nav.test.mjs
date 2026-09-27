// command_palette_nav.test.mjs — contract for command_palette.js's campaign
// "Go to" commands. They come only from #sidebar[data-nav-commands], the
// server's list of the viewer's own sidebar rows, so the palette can never
// offer a row the sidebar withholds (a row hidden from players, an app the
// viewer cannot open, the owner's Manage pages) — and it has no hard-coded
// fallback that could bring one back.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'command_palette.js'), 'utf8');

/** A minimal but real element: children, classList, appendChild/querySelectorAll. */
function makeElement(tag) {
  const el = {
    tagName: tag,
    children: [],
    parentNode: null,
    style: {},
    _attrs: {},
    _classes: new Set(),
    _listeners: {},
    _text: '',
    _html: '',
  };
  Object.defineProperty(el, 'textContent', { get() { return el._text; }, set(v) { el._text = String(v); } });
  Object.defineProperty(el, 'innerHTML', { get() { return el._html; }, set(v) { el._html = String(v); } });
  Object.defineProperty(el, 'className', { get() { return el._attrs['class'] || ''; }, set(v) { el._attrs['class'] = v; } });
  el.classList = {
    add: (c) => el._classes.add(c),
    remove: (c) => el._classes.delete(c),
    contains: (c) => el._classes.has(c),
  };
  el.setAttribute = (k, v) => { el._attrs[k] = v; };
  el.getAttribute = (k) => (k in el._attrs ? el._attrs[k] : null);
  el.appendChild = (child) => { child.parentNode = el; el.children.push(child); return child; };
  el.addEventListener = (ev, fn) => { (el._listeners[ev] = el._listeners[ev] || []).push(fn); };
  el.removeEventListener = () => {};
  el.querySelector = () => null;
  el.querySelectorAll = () => [];
  el.focus = () => {};
  return el;
}

/** Boot command_palette.js in a vm and open the palette for a campaign path,
 *  with navData as the sidebar's data-nav-commands (null for none). */
function boot(pathname, navData) {
  const bodyEl = makeElement('body');
  const docHandlers = {};

  const document = {
    body: bodyEl,
    createElement: (tag) => makeElement(tag),
    querySelector: () => null,
    getElementById: (id) => (id === 'sidebar' && navData !== null ? {
      getAttribute: (name) => (name === 'data-nav-commands'
        ? (typeof navData === 'string' ? navData : JSON.stringify(navData)) : null),
    } : null),
    addEventListener: (ev, fn) => { (docHandlers[ev] = docHandlers[ev] || []).push(fn); },
    removeEventListener: () => {},
  };

  const window = {
    location: { pathname },
    addEventListener: () => {},
    dispatchEvent: () => {},
  };

  const Chronicle = {
    escapeHtml: (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'),
  };
  const navigator = { platform: 'Linux' };

  const sandbox = {
    Chronicle,
    document,
    window,
    navigator,
    console,
    CustomEvent: function CustomEvent(type, init) { this.type = type; this.detail = (init || {}).detail || null; },
    requestAnimationFrame: (fn) => fn(),
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);

  Chronicle.openCommandPalette();

  // The results list is the last child of the modal, which is the only
  // child appended to <body> (buildModal appends `overlay` once).
  const overlay = bodyEl.children[bodyEl.children.length - 1];
  const modal = overlay.children[overlay.children.length - 1];
  const resultsList = modal.children[modal.children.length - 1];
  return { resultsList };
}

const PLAYER_ROWS = [
  { label: 'Dashboard', href: '/campaigns/camp-1', icon: 'fa-house' },
  { label: 'Journal', href: '/campaigns/camp-1/journal', icon: 'fa-book-open' },
  { label: 'Characters', href: '/campaigns/camp-1/characters', icon: 'fa-masks-theater' },
  { label: 'Locations', href: '/campaigns/camp-1/locations', icon: 'fa-map-pin' },
  { label: 'All Pages', href: '/campaigns/camp-1/entities', icon: 'fa-layer-group' },
];

test('each row of the viewer\'s sidebar becomes a "Go to" command', () => {
  const { resultsList } = boot('/campaigns/camp-1/dashboard', PLAYER_ROWS);
  for (const row of PLAYER_ROWS) {
    assert.ok(resultsList.innerHTML.includes('Go to ' + row.label), 'missing "Go to ' + row.label + '"');
  }
  assert.ok(!resultsList.innerHTML.includes('Go to NPCs'),
    'Characters lists the party and NPCs; there is no separate NPCs command');
});

test('nothing the server left out comes back from a hard-coded list', () => {
  // The player's rows omit Maps (hidden from players), Calendar (off) and
  // the owner's Settings and Customize.
  const { resultsList } = boot('/campaigns/camp-1/dashboard', PLAYER_ROWS);
  for (const withheld of ['Go to Maps', 'Go to Calendar', 'Go to Settings', 'Go to Customize', 'Go to Media', 'Go to Timelines', 'Go to Sessions']) {
    assert.ok(!resultsList.innerHTML.includes(withheld), withheld + ' must only appear when the server lists it');
  }
});

test('an icon that is not a Font Awesome name never reaches the markup', () => {
  const { resultsList } = boot('/campaigns/camp-1/dashboard', [
    { label: 'Factions', href: '/campaigns/camp-1/factions', icon: 'fa-x" onmouseover="alert(1)' },
    { label: '<b>Bold</b>', href: '/campaigns/camp-1/bold', icon: 'fa-flag' },
  ]);
  assert.ok(!resultsList.innerHTML.includes('onmouseover'), 'a hostile icon must be replaced');
  assert.ok(resultsList.innerHTML.includes('Go to Factions'));
  assert.ok(!resultsList.innerHTML.includes('<b>Bold</b>'), 'labels are escaped');
});

test('missing or malformed data yields no campaign rows, and no error', () => {
  for (const data of [null, 'not json', '{"label":"x"}', [{ href: '/x' }, 7, null]]) {
    const { resultsList } = boot('/campaigns/camp-1/dashboard', data);
    assert.ok(!resultsList.innerHTML.includes('Go to Journal'));
    assert.ok(resultsList.innerHTML.includes('Go to Campaigns'), 'global commands still render');
  }
});
