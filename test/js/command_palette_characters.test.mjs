// command_palette_characters.test.mjs — contract for command_palette.js's
// campaign navigation commands after the NPCs/Characters consolidation
// (issue: the sidebar's "NPCs" link is redundant with "Characters", which
// already lists the party and NPCs together). The palette's only route to
// that page must say "Go to Characters" and link straight at /characters,
// not the old /npcs redirect hop.

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

/** Boot command_palette.js in a vm and open the palette for a campaign path. */
function boot(pathname) {
  const bodyEl = makeElement('body');
  const docHandlers = {};

  const document = {
    body: bodyEl,
    createElement: (tag) => makeElement(tag),
    querySelector: () => null,
    addEventListener: (ev, fn) => { (docHandlers[ev] = docHandlers[ev] || []).push(fn); },
    removeEventListener: () => {},
  };

  const window = {
    location: { pathname },
    addEventListener: () => {},
    dispatchEvent: () => {},
  };

  const Chronicle = { escapeHtml: (s) => String(s) };
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

test('the palette offers "Go to Characters" straight to /characters, not a redundant "Go to NPCs"', () => {
  const { resultsList } = boot('/campaigns/camp-1/dashboard');

  assert.ok(!resultsList.innerHTML.includes('Go to NPCs'),
    'the palette must not keep a "Go to NPCs" command now that the sidebar link is gone');
  assert.ok(resultsList.innerHTML.includes('Go to Characters'),
    'the palette must still offer a way to the Characters page');
});
