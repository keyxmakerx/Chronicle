// nav_active_highlight.test.mjs — contract for boot.js's boosted-nav sidebar
// active-link highlighter (updateSidebarActiveLinks).
//
// Regression guard for C-NAV-ACTIVE-FIX: after an hx-boost swap (which leaves
// #sidebar un-re-rendered), the highlighter used to re-apply a hardcoded class
// vocabulary that no longer matched what the server actually renders for
// active/inactive nav links (layouts/app.templ's sidebarNavActive /
// sidebarNavInactive). That left the just-left link's "active" indicator
// classes stuck (never removed) while the newly-active link only got a
// partial set — the "wrong / doubled active nav item" bug. The fix reads the
// live vocabulary from #sidebar's data-nav-active-classes /
// data-nav-inactive-classes attributes (the single source of truth, populated
// server-side from the same Go constants used to render every link).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'boot.js'), 'utf8');

// The real current server vocabulary (layouts/app.templ:92-98), duplicated
// here ONLY as fixture data for these tests — not by boot.js itself anymore.
const SERVER_ACTIVE = ['sidebar-nav-active', 'text-sidebar-active', 'border-accent'];
const SERVER_INACTIVE = ['text-sidebar-text', 'hover:text-sidebar-active'];
const SHARED_BASE = ['sidebar-nav-glow', 'flex', 'items-center', 'border-l-2', 'border-transparent'];

/** A minimal but real Set-backed classList (contains/add/remove). */
function makeClassList(initial) {
  const set = new Set(initial);
  return {
    contains: (c) => set.has(c),
    add: (c) => set.add(c),
    remove: (c) => set.delete(c),
    toArray: () => Array.from(set),
  };
}

/** A fake sidebar nav <a>: href + classList, nothing else needed by the code under test. */
function makeLink(href, classes) {
  return {
    getAttribute: (name) => (name === 'href' ? href : null),
    classList: makeClassList(classes),
  };
}

/**
 * Boot boot.js in a vm with just enough of the DOM to drive
 * updateSidebarActiveLinks via the real htmx:pushedIntoHistory listener it
 * registers at load time.
 *
 * @param {Object} opts
 * @param {Object[]} opts.links - [{href, classes}] rendered inside #sidebar.
 * @param {boolean} [opts.withDataAttrs=true] - whether #sidebar carries the
 *   data-nav-active-classes / data-nav-inactive-classes attributes (the normal
 *   case); false exercises the FALLBACK_* vocabulary path.
 */
function boot({ links, withDataAttrs = true }) {
  const docHandlers = {};
  const winHandlers = {};

  function addHandler(map, event, fn) {
    if (!map[event]) map[event] = [];
    map[event].push(fn);
  }
  function fire(map, event, arg) {
    (map[event] || []).forEach((fn) => fn(arg));
  }

  const linkEls = links.map((l) => makeLink(l.href, l.classes));

  const sidebarAttrs = withDataAttrs
    ? {
        'data-nav-active-classes': SERVER_ACTIVE.join(' '),
        'data-nav-inactive-classes': SERVER_INACTIVE.join(' '),
      }
    : {};

  const sidebarEl = {
    getAttribute: (name) => (name in sidebarAttrs ? sidebarAttrs[name] : null),
    querySelectorAll: (sel) => (sel === 'a' ? linkEls : []),
  };

  const bodyEl = { classList: { add() {}, remove() {} } };

  const document = {
    getElementById: (id) => (id === 'sidebar' ? sidebarEl : null),
    querySelector: () => null,
    createElement: () => ({
      style: {}, className: '', innerHTML: '', textContent: '',
      setAttribute() {}, getAttribute() { return null; },
      classList: { add() {}, remove() {}, contains() { return false; } },
      appendChild() {}, removeChild() {}, insertBefore() {},
      addEventListener() {}, removeEventListener() {},
      querySelector() { return null; },
    }),
    addEventListener: (ev, fn) => addHandler(docHandlers, ev, fn),
    removeEventListener: () => {},
    dispatchEvent: () => {},
    readyState: 'complete',
    body: bodyEl,
    cookie: '',
  };

  const location = { pathname: '/campaigns/camp1/dashboard' };
  const window = {
    addEventListener: (ev, fn) => addHandler(winHandlers, ev, fn),
    dispatchEvent: () => {},
    location,
  };
  const htmx = { config: {} };
  function CustomEvent(type, init) { this.type = type; this.detail = (init || {}).detail || null; }

  const sandbox = { Chronicle: {}, document, window, htmx, CustomEvent, console, setTimeout, clearTimeout, Promise };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);

  return {
    linkEls,
    navigateTo(pathname) {
      location.pathname = pathname;
      fire(docHandlers, 'htmx:pushedIntoHistory');
    },
  };
}

test('boosted nav moves the FULL active vocabulary from the old link to the new one (no doubling)', () => {
  const dashboard = { href: '/campaigns/camp1/dashboard', classes: [...SHARED_BASE, ...SERVER_ACTIVE] };
  const members = { href: '/campaigns/camp1/members', classes: [...SHARED_BASE, ...SERVER_INACTIVE] };
  const { linkEls, navigateTo } = boot({ links: [dashboard, members] });
  const [dashboardEl, membersEl] = linkEls;

  navigateTo('/campaigns/camp1/members');

  // The new active link gets every active-only class...
  for (const c of SERVER_ACTIVE) {
    assert.ok(membersEl.classList.contains(c), `members link should gain "${c}"`);
  }
  // ...and none of the inactive-only classes.
  for (const c of SERVER_INACTIVE) {
    assert.ok(!membersEl.classList.contains(c), `members link should not keep "${c}"`);
  }

  // The old (now-inactive) link must have EVERY active-only class removed —
  // this is exactly what the pre-fix code got wrong (it never removed
  // sidebar-nav-active / border-accent, leaving the previous item still
  // looking active: the "doubled" symptom).
  for (const c of SERVER_ACTIVE) {
    assert.ok(!dashboardEl.classList.contains(c), `dashboard link must lose "${c}"`);
  }
  for (const c of SERVER_INACTIVE) {
    assert.ok(dashboardEl.classList.contains(c), `dashboard link should gain "${c}"`);
  }

  // Shared/base classes (present regardless of state) must be left untouched.
  for (const c of SHARED_BASE) {
    assert.ok(dashboardEl.classList.contains(c) && membersEl.classList.contains(c),
      `shared base class "${c}" must not be toggled`);
  }
});

test('a second boost correctly hands the active state back (no residual state across hops)', () => {
  const dashboard = { href: '/campaigns/camp1/dashboard', classes: [...SHARED_BASE, ...SERVER_ACTIVE] };
  const members = { href: '/campaigns/camp1/members', classes: [...SHARED_BASE, ...SERVER_INACTIVE] };
  const { linkEls, navigateTo } = boot({ links: [dashboard, members] });
  const [dashboardEl, membersEl] = linkEls;

  navigateTo('/campaigns/camp1/members');
  navigateTo('/campaigns/camp1/dashboard');

  for (const c of SERVER_ACTIVE) {
    assert.ok(dashboardEl.classList.contains(c), `dashboard link should regain "${c}"`);
    assert.ok(!membersEl.classList.contains(c), `members link must lose "${c}" again`);
  }
});

test('longest-prefix match wins between two candidates sharing a prefix', () => {
  const entities = { href: '/campaigns/camp1/entities', classes: [...SHARED_BASE, ...SERVER_ACTIVE] };
  const dashboard = { href: '/campaigns/camp1/dashboard', classes: [...SHARED_BASE, ...SERVER_INACTIVE] };
  const { linkEls, navigateTo } = boot({ links: [entities, dashboard] });
  const [entitiesEl, dashboardEl] = linkEls;

  // A sub-path of /entities must activate the /entities link, not leave the
  // (unrelated, shorter, non-matching) dashboard link touched incorrectly.
  navigateTo('/campaigns/camp1/entities/42');

  for (const c of SERVER_ACTIVE) assert.ok(entitiesEl.classList.contains(c));
  for (const c of SERVER_ACTIVE) assert.ok(!dashboardEl.classList.contains(c));
  for (const c of SERVER_INACTIVE) assert.ok(dashboardEl.classList.contains(c));
});

test('falls back to the hardcoded vocabulary when #sidebar has no data attributes', () => {
  const dashboard = { href: '/campaigns/camp1/dashboard', classes: [...SHARED_BASE, ...SERVER_ACTIVE] };
  const members = { href: '/campaigns/camp1/members', classes: [...SHARED_BASE, ...SERVER_INACTIVE] };
  const { linkEls, navigateTo } = boot({ links: [dashboard, members], withDataAttrs: false });
  const [dashboardEl, membersEl] = linkEls;

  navigateTo('/campaigns/camp1/members');

  // The fallback vocabulary must be the CURRENT server vocabulary, not the
  // stale pre-fix one (bg-sidebar-hover / hover:bg-sidebar-hover).
  for (const c of SERVER_ACTIVE) assert.ok(membersEl.classList.contains(c));
  for (const c of SERVER_ACTIVE) assert.ok(!dashboardEl.classList.contains(c));
  assert.ok(!membersEl.classList.contains('bg-sidebar-hover'),
    'must not reintroduce the dead bg-sidebar-hover class');
});
