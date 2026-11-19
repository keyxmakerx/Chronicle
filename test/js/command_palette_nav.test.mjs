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
function boot(pathname, navData, controls = {}) {
  const bodyEl = makeElement('body');
  const clicked = [];
  const docHandlers = {};

  const document = {
    body: bodyEl,
    createElement: (tag) => makeElement(tag),
    // controls maps a selector to the title of a control the page shows.
    querySelector: (sel) => (sel in controls
      ? { click: () => { clicked.push(sel); }, getAttribute: (n) => (n === 'title' ? controls[sel] : null) }
      : null),
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
  const inputWrap = modal.children[0];
  const input = inputWrap.children[1];
  /** Types into the palette's field and returns the list's markup. */
  const type = (text) => {
    input.value = text;
    (input._listeners.input || []).forEach((fn) => fn.call(input, {}));
    return resultsList.innerHTML;
  };
  return { resultsList, type, clicked };
}

const PLAYER_ROWS = [
  { label: 'Dashboard', href: '/campaigns/camp-1', icon: 'fa-house' },
  { label: 'Journal', href: '/campaigns/camp-1/journal', icon: 'fa-book-open' },
  { label: 'Characters', href: '/campaigns/camp-1/characters', icon: 'fa-masks-theater' },
  { label: 'Locations', href: '/campaigns/camp-1/locations', icon: 'fa-map-pin' },
  { label: 'All pages', href: '/campaigns/camp-1/entities', icon: 'fa-layer-group' },
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

const OWNER_ROWS = [
  { label: 'Campaign settings', href: '/campaigns/camp-1/dashboard', icon: 'fa-sliders', words: 'settings manage owner options' },
  { label: "Guests' dashboard", href: '/campaigns/camp-1/dashboard#line-guests-home', icon: 'fa-eye', words: 'guest visitor public dashboard home view as preview' },
  { label: 'Export a copy', href: '/campaigns/camp-1/settings?tab=data', icon: 'fa-file-export', words: 'export backup download copy' },
  { label: 'Archive or delete this campaign', href: '/campaigns/camp-1/settings#danger', icon: 'fa-triangle-exclamation', words: 'delete archive remove end' },
  { label: 'Invite someone', href: '/campaigns/camp-1/members', icon: 'fa-user-plus', words: 'invite add player join' },
  { label: 'Quest board', href: '/campaigns/camp-1/extensions#quest-board', icon: 'fa-scroll', words: 'quest board quests cork put on a page' },
  { label: 'Trackers', href: '/campaigns/camp-1/extensions#trackers', icon: 'fa-gauge', words: 'tracker clock clocks counter steps put on a page' },
  { label: 'Trash', href: '/campaigns/camp-1/trash', icon: 'fa-trash-can', words: 'delete deleted restore undo bin' },
];

test('a command is found by its words as well as its label', () => {
  const cases = [
    ['visitor', 'Go to Guests'],
    ['backup', 'Go to Export a copy'],
    ['remove', 'Go to Archive or delete'],
    ['join', 'Go to Invite someone'],
    ['manage', 'Go to Campaign settings'],
    ['quest board', 'Go to Quest board'],
    ['tracker', 'Go to Trackers'],
    ['clock', 'Go to Trackers'],
    ['restore', 'Go to Trash'],
  ];
  for (const [typed, expect] of cases) {
    const { type } = boot('/campaigns/camp-1/dashboard', OWNER_ROWS);
    assert.ok(type(typed).includes(expect), typed + ' should find ' + expect);
  }
});

test('a label match ranks before a words match', () => {
  // "dashboard" is in the guests' label and, as a word, in nothing else here;
  // "settings" is a label of one row and a word of another.
  const rows = [
    { label: 'Export a copy', href: '/a', words: 'settings backup' },
    { label: 'Campaign settings', href: '/b', words: 'manage' },
  ];
  const { type } = boot('/campaigns/camp-1/dashboard', rows);
  const html = type('settings');
  assert.ok(html.indexOf('Go to Campaign settings') < html.indexOf('Go to Export a copy'));
});

test('words that match nothing never list a command', () => {
  const { type } = boot('/campaigns/camp-1/dashboard', OWNER_ROWS);
  assert.ok(!type('zzzz').includes('Go to Export'));
});

const CONTROLS = {
  '#user-menu-panel a[href="/account"]': '',
  '[data-tt-tab]': 'The table',
  '[data-dms-toggle]': 'GM Screen',
};

test('everyday words find the account, the theme, the table and the screen', () => {
  const cases = [
    ['dark', 'Toggle Theme'],
    ['password', 'Account settings'],
    ['2fa', 'Account settings'],
    ['dice', 'The table'],
    ['screen', 'GM Screen'],
    ['homebrew', 'GM Screen'],
    ['encounter', 'GM Screen'],
    ['fight', 'GM Screen'],
  ];
  for (const [typed, expect] of cases) {
    const { type } = boot('/campaigns/camp-1/dashboard', PLAYER_ROWS, CONTROLS);
    assert.ok(type(typed).includes(expect), typed + ' should find ' + expect);
  }
});

test('a viewer whose page lacks the control never gets its command', () => {
  // A player has the table and the account menu but no GM Screen button.
  const { type } = boot('/campaigns/camp-1/dashboard', PLAYER_ROWS, {
    '#user-menu-panel a[href="/account"]': '', '[data-tt-tab]': 'The table',
  });
  assert.ok(!type('screen').includes('GM Screen'));
  assert.ok(!type('homebrew').includes('GM Screen'));
  const none = boot('/campaigns/camp-1/dashboard', PLAYER_ROWS, {});
  assert.ok(!none.type('password').includes('Account settings'));
  assert.ok(!none.type('dice').includes('The table'));
});
