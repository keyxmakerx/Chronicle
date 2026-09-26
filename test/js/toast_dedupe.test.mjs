// toast_dedupe.test.mjs — contract for notifications.js's duplicate-toast
// handling (Chronicle.notify).
//
// A repeat of a message+type still on screen must reuse that toast (a count
// badge, timer restarted) instead of stacking a second copy underneath it —
// everything else about toasts (position, colors, icons, close button) is
// unchanged. The bump animation on the badge must be skipped under
// prefers-reduced-motion, without changing the count itself.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'notifications.js'), 'utf8');

/** A minimal but real element: children, classList, attrs, appendChild/removeChild. */
function makeElement(tag) {
  const el = {
    tagName: tag,
    children: [],
    parentNode: null,
    style: {},
    dataset: {},
    _attrs: {},
    _classes: new Set(),
    _listeners: {},
    _text: '',
    _html: '',
  };
  Object.defineProperty(el, 'textContent', {
    get() { return el._text; },
    set(v) { el._text = String(v); el._html = ''; },
  });
  Object.defineProperty(el, 'innerHTML', {
    get() { return el._html; },
    set(v) { el._html = String(v); },
  });
  Object.defineProperty(el, 'id', {
    get() { return el._attrs.id || ''; },
    set(v) { el._attrs.id = v; },
  });
  Object.defineProperty(el, 'className', {
    get() { return el._attrs['class'] || ''; },
    set(v) {
      el._attrs['class'] = v;
      el._classes = new Set(String(v).split(/\s+/).filter(Boolean));
    },
  });
  el.classList = {
    add: (c) => el._classes.add(c),
    remove: (c) => el._classes.delete(c),
    contains: (c) => el._classes.has(c),
  };
  el.setAttribute = (k, v) => { el._attrs[k] = v; };
  el.getAttribute = (k) => (k in el._attrs ? el._attrs[k] : null);
  el.appendChild = (child) => { child.parentNode = el; el.children.push(child); return child; };
  el.removeChild = (child) => {
    const idx = el.children.indexOf(child);
    if (idx >= 0) el.children.splice(idx, 1);
    child.parentNode = null;
    return child;
  };
  el.addEventListener = (ev, fn) => { (el._listeners[ev] = el._listeners[ev] || []).push(fn); };
  el.removeEventListener = () => {};
  el.querySelector = () => null;
  el.contains = (other) => {
    let n = other;
    while (n) { if (n === el) return true; n = n.parentNode; }
    return false;
  };
  return el;
}

/** Find the first descendant carrying `cls` in its classList (depth-first). */
function findByClass(root, cls) {
  for (const child of root.children) {
    if (child.classList.contains(cls)) return child;
    const found = findByClass(child, cls);
    if (found) return found;
  }
  return null;
}

/**
 * Boot notifications.js in a vm with a fake DOM and a fake, advanceable
 * clock (so "restarts its timer" can be verified precisely: a duration that
 * elapses on the ORIGINAL schedule must not dismiss a toast that was
 * refreshed in the meantime).
 *
 * @param {Object} [opts]
 * @param {boolean} [opts.reducedMotion=false]
 */
function boot({ reducedMotion = false } = {}) {
  const docHandlers = {};
  const bodyEl = makeElement('body');
  const headEl = makeElement('head');

  const document = {
    body: bodyEl,
    head: headEl,
    createElement: (tag) => makeElement(tag),
    getElementById: () => null,
    querySelector: () => null,
    addEventListener: (ev, fn) => { (docHandlers[ev] = docHandlers[ev] || []).push(fn); },
    removeEventListener: () => {},
    dispatchEvent: () => {},
  };

  const window = {
    matchMedia: (query) => ({ matches: reducedMotion, media: query, addListener() {}, removeListener() {} }),
  };

  // Fake advanceable clock: setTimeout/clearTimeout schedule against a
  // virtual `now`, and advance(ms) fires everything due, in order, removing
  // each timer before it runs (so a handler can reschedule itself).
  let now = 0;
  let nextId = 1;
  const pending = new Map();
  function fakeSetTimeout(fn, delay) {
    const id = nextId++;
    pending.set(id, { at: now + (delay || 0), fn });
    return id;
  }
  function fakeClearTimeout(id) { pending.delete(id); }
  function advance(ms) {
    now += ms;
    while (true) {
      let dueId = null;
      let dueAt = Infinity;
      for (const [id, t] of pending) {
        if (t.at <= now && t.at < dueAt) { dueId = id; dueAt = t.at; }
      }
      if (dueId === null) break;
      const t = pending.get(dueId);
      pending.delete(dueId);
      t.fn();
    }
  }

  const sandbox = {
    Chronicle: {},
    document,
    window,
    console,
    setTimeout: fakeSetTimeout,
    clearTimeout: fakeClearTimeout,
    requestAnimationFrame: (fn) => fn(),
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);

  return {
    Chronicle: sandbox.Chronicle,
    container: () => bodyEl.children.find((c) => c._attrs.id === 'chronicle-toasts'),
    advance,
  };
}

test('a repeat of an on-screen toast (same message and type) updates it instead of stacking a copy', () => {
  const { Chronicle, container } = boot();

  Chronicle.notify('Kaelen Duskwood saved', 'success');
  Chronicle.notify('Kaelen Duskwood saved', 'success');
  Chronicle.notify('Kaelen Duskwood saved', 'success');

  assert.equal(container().children.length, 1, 'three identical notifies must leave exactly one toast on screen');
  const badge = findByClass(container(), 'chronicle-toast-badge');
  assert.ok(badge, 'a repeated toast must grow a count badge');
  assert.equal(badge.textContent, '×3', 'the badge must reflect the total repeat count');
});

test('a different message, or a different type, still stacks its own toast', () => {
  const { Chronicle, container } = boot();

  Chronicle.notify('Kaelen Duskwood saved', 'success');
  Chronicle.notify('Ashenmoor Reclamation saved', 'success');
  Chronicle.notify('Kaelen Duskwood saved', 'error');

  assert.equal(container().children.length, 3, 'distinct message/type pairs must not be merged');
});

test('reusing a toast restarts its dismiss timer instead of leaving the old one armed', () => {
  const { Chronicle, container, advance } = boot();

  Chronicle.notify('Kaelen Duskwood saved', 'success'); // t=0, dismiss scheduled for t=4000
  advance(1000); // t=1000
  Chronicle.notify('Kaelen Duskwood saved', 'success'); // restarts: dismiss now scheduled for t=5000

  advance(3000); // t=4000 — the ORIGINAL schedule elapses
  assert.equal(container().children.length, 1,
    'the toast must survive its original dismiss time once the timer was restarted');

  advance(1000); // t=5000 — the RESTARTED schedule elapses, starting the fade-out
  advance(300); // the fade-out's own timer actually removes the element
  assert.equal(container().children.length, 0, 'the restarted timer must still dismiss the toast');
});

test('the badge bump animation is skipped under prefers-reduced-motion, but the count still updates', () => {
  const { Chronicle, container } = boot({ reducedMotion: true });

  Chronicle.notify('Kaelen Duskwood saved', 'success');
  Chronicle.notify('Kaelen Duskwood saved', 'success');

  const badge = findByClass(container(), 'chronicle-toast-badge');
  assert.equal(badge.textContent, '×2', 'the count must still update under reduced motion');
  assert.ok(!badge.classList.contains('chronicle-toast-badge-bump'),
    'reduced motion must skip the bump animation class entirely');
});

test('without reduced motion, the bump animation class is applied on a repeat', () => {
  const { Chronicle, container } = boot({ reducedMotion: false });

  Chronicle.notify('Kaelen Duskwood saved', 'success');
  Chronicle.notify('Kaelen Duskwood saved', 'success');

  const badge = findByClass(container(), 'chronicle-toast-badge');
  assert.ok(badge.classList.contains('chronicle-toast-badge-bump'),
    'a repeat without reduced motion must apply the bump animation class');
});

test('a repeat that lands while a closed toast fades out gets its own toast', () => {
  const { Chronicle, container, advance } = boot();

  Chronicle.notify('Kaelen Duskwood saved', 'success');
  const first = container().children[0];
  const closeBtn = first.children.find((c) => c.tagName === 'button');
  closeBtn._listeners.click.forEach((fn) => fn());
  Chronicle.notify('Kaelen Duskwood saved', 'success'); // during the fade-out

  advance(300); // the fade-out removes the closed toast
  assert.equal(container().children.length, 1, 'the repeat must still be on screen');
  assert.notEqual(container().children[0], first, 'the repeat must not reuse the toast being closed');
});
