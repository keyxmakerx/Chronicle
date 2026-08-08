// theater_harness.mjs — the shared boot for C-CALV4-THEATER's node tests.
//
// It builds an ENTITY-PAGE-SHAPED fixture — the embed's Block, the Expand
// button, and the theater scaffold with a SECOND Block inside it, all under one
// widgetbindings.BlockHost wrapper — and runs the shipped
// internal/plugins/calendar/static/js/calendar_theater.js in a node `vm` over
// the minimal DOM in daycard_dom.mjs.
//
// WHY THE FIXTURE CARRIES THE BlockHost WRAPPER. [TH-14] was RE-SIGNED because
// its "outside any HTMX-swappable region" constraint rested on a refuted
// citation: calendar_widget_type.go wraps the WHOLE component in BlockHost and
// picker.templ swaps THAT with hx-swap="outerHTML". The scaffold therefore sits
// inside the swappable region deliberately, and the required counterpart is an
// htmx:beforeSwap close. That counterpart cannot be tested without a fixture
// that has the wrapper in it, so the wrapper is here.
//
// TIMERS ARE FAKE AND THAT IS THE POINT. The register's close is a real 160ms
// wait in a browser; here it is a queue the test inspects, so "under reduced
// motion the module does not wait for a transition that will never fire" is an
// assertion about a queue LENGTH rather than about a stopwatch.
//
// THE DOM SHIM IS daycard_dom.mjs, REUSED RATHER THAN FORKED — it already
// implements exactly what a module of this shape touches and THROWS on what it
// does not, which is the only reason its coverage means anything. What it does
// not have is `<dialog>`: showModal/close/open and a document.activeElement are
// added HERE, per instance, so the shared shim is not edited.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';
import { el, makeDocument } from './daycard_dom.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'calendar_theater.js');
export const SRC = readFileSync(jsPath, 'utf8');

const SCAFFOLD_ID = 'cal-theater-harptos-ent-1-theater';
const TWIN_ID = 'cal-theater-elven-ent-1-theater';

// blockSubtree is a Block-shaped fixture: the query container, the clipping
// `.block`, a couple of day cells with their radios, and a docked Ledger. It is
// deliberately the same SHAPE on both sides — the whole claim of the slice is
// that the theater renders the same Block with a different layer set — and the
// two differ only in the namespace token their ids and radio names carry.
function blockSubtree(ns, opts) {
  const cells = [3, 4].map((d) => el('div', { class: 'cell', 'data-cell': '', 'data-day': 'harptos-' + d }, [
    el('input', { type: 'radio', class: 'daypick', name: 'day-harptos-' + ns, id: 'day-harptos-' + ns + '-' + d }),
    el('label', { class: 'dsel', for: 'day-harptos-' + ns + '-' + d }, [el('span', { class: 'vh' }, [], 'Day ' + d)]),
    el('span', { class: 'dn' }, [], String(d)),
  ]));

  const kids = [
    el('div', { class: 'np' }, [
      el('span', { class: 'seg tie', role: 'group' }, [
        el('input', { type: 'radio', class: 'tiepick', 'data-tie-pick': 'tied', name: 'tie-harptos-' + ns, id: 'tie-harptos-' + ns + '-tied', checked: '' }),
        el('input', { type: 'radio', class: 'tiepick', 'data-tie-pick': 'whole', name: 'tie-harptos-' + ns, id: 'tie-harptos-' + ns + '-whole' }),
      ]),
    ]),
    el('div', { class: 'grid' }, cells),
  ];
  if (opts && opts.ledger) {
    kids.push(el('div', { class: 'ledger', 'data-zone': 'ledger' }, [el('div', { class: 'lrows' })]));
  }
  if (opts && opts.switchboard) {
    kids.push(el('div', { popover: '', id: 'lsheet-harptos-' + ns, class: 'lsheet' }));
  }
  return el('div', { class: 'cal-block-host', 'data-cal-block': '' }, [el('div', { class: 'block' }, kids)]);
}

// buildOne is one (calendar, entity) pair: its opener, its embed Block and its
// scaffold, all under one BlockHost wrapper. It is a function rather than a
// literal because [TH-15] is a claim about what happens when there are TWO, and
// a fixture that can only build one cannot express it.
function buildOne(id, ns, hostID) {
  const opener = el('button', {
    type: 'button',
    'data-theater-pick': id,
    'aria-haspopup': 'dialog',
    'aria-expanded': 'false',
    'aria-controls': id,
  }, [], 'Expand');

  // The EMBED's Block keeps its switchboard ([TH-13]); the THEATER's has none.
  const embed = blockSubtree(ns, { switchboard: true });
  const theaterBlock = blockSubtree(ns + '-theater', { ledger: true });

  const closeBtn = el('button', { type: 'button', class: 'tclose', 'data-theater-close': '', 'aria-label': 'Close' });
  const dialog = el('dialog', {
    id: id,
    class: 'cal-bench cal-theater',
    'data-cal-theater': '',
    'aria-modal': 'true',
    'aria-labelledby': id + '-head',
  }, [
    el('div', { class: 'tbox', 'data-theater-box': '' }, [
      el('div', { class: 'thead' }, [
        el('h2', { class: 'thd', id: id + '-head' }, [], 'Harptos of Imix'),
        closeBtn,
      ]),
      el('div', { class: 'tbody', 'data-theater-scroll': '' }, [theaterBlock]),
    ]),
  ]);

  // <dialog> behaviour the shared shim does not carry. Per instance, so
  // daycard_dom.mjs is untouched.
  dialog.open = false;
  dialog.showModal = function () { this.open = true; this.setAttribute('open', ''); };
  dialog.close = function () { this.open = false; this.removeAttribute('open'); };

  // THE HTMX SWAP TARGET. widgetbindings.BlockHost wraps the whole component,
  // and picker.templ's three paths replace THIS element with outerHTML.
  const host = el('div', { id: hostID, 'data-widget-block-host': '' }, [
    el('div', { class: 'card p-0', 'data-entity-calendar': '' }, [
      el('div', { class: 'flex' }, [
        el('a', { 'data-open-calendar': '' }, [], 'Open full calendar'),
        opener,
      ]),
      el('div', { class: 'p-3' }, [embed]),
      dialog,
    ]),
  ]);

  // THE LAYOUT FLUSH, MADE OBSERVABLE. The module forces one read of the box's
  // offsetHeight between showing the dialog and adding the reveal class; without
  // it the class lands in the same style change as the dialog becoming visible,
  // there is no before-change style to transition from, and the reveal silently
  // does not run — with every end-state assertion still green. So the read is
  // recorded in the element's own op log, in sequence, and the suite asserts it
  // happened BEFORE the class. A reader who deletes the `void` as a stray now
  // meets a red test instead of a still animation.
  const boxEl = dialog.querySelector('[data-theater-box]');
  Object.defineProperty(boxEl, 'offsetHeight', {
    configurable: true,
    get() { (this._ops = this._ops || []).push('read:offsetHeight'); return 0; },
  });

  return { host, dialog, opener, closeBtn, embed, theaterBlock, boxEl, scaffoldID: id };
}

export function buildEntityFixture(opts) {
  opts = opts || {};
  const one = buildOne(SCAFFOLD_ID, 'ent-1', 'wb-block-calendar-ent-1');
  const kids = [one.host];
  let twin = null;
  if (opts.twin) {
    twin = buildOne(TWIN_ID, 'ent-1-elven', 'wb-block-calendar-elven-ent-1');
    kids.push(twin.host);
  }
  const root = el('div', { id: 'page' }, kids);
  return { ...one, root, twin };
}

export function boot(opts) {
  opts = opts || {};
  const fx = buildEntityFixture(opts);
  const document = makeDocument(fx.root);

  // Focus, which the shared shim has no notion of. The return contract is the
  // single most asserted claim in this suite ([TH-4] clause 3), so `focus()` is
  // a real write to a real activeElement rather than a spy.
  document.activeElement = null;
  const focused = [];
  const giveFocus = (node) => {
    node.focus = function () { document.activeElement = this; focused.push(this); };
  };
  [fx.opener, fx.closeBtn, fx.dialog].forEach(giveFocus);
  if (fx.twin) [fx.twin.opener, fx.twin.closeBtn, fx.twin.dialog].forEach(giveFocus);

  const timers = [];
  let nextTimer = 1;
  const sandbox = {
    console: opts.console || console,
    document,
    setTimeout: (fn, ms) => { const id = nextTimer++; timers.push({ id, fn, ms }); return id; },
    clearTimeout: (id) => {
      const i = timers.findIndex((t) => t.id === id);
      if (i >= 0) timers.splice(i, 1);
    },
  };
  sandbox.window = sandbox;
  sandbox.matchMedia = (q) => ({
    matches: !!opts.reduced && /prefers-reduced-motion:\s*reduce/.test(q),
  });
  sandbox.addEventListener = () => {};
  sandbox.removeEventListener = () => {};

  vm.createContext(sandbox);
  vm.runInContext(SRC, sandbox, { filename: 'calendar_theater.js' });

  return {
    ...fx,
    document,
    timers,
    api: sandbox.window.__calTheater,
    activeElement: () => document.activeElement,
    focused,
    // fireOn dispatches to an ELEMENT's own listeners, which is how this module
    // binds everything: the opener's click, the dialog's cancel / click /
    // transitionend. There is no delegated document handler to drive.
    fireOn: (type, target, extra) => {
      const handlers = (target && target._on && target._on[type]) || [];
      let defaultPrevented = false;
      const ev = { target, preventDefault() { defaultPrevented = true; }, ...(extra || {}) };
      handlers.forEach((fn) => fn(ev));
      return { defaultPrevented };
    },
    // fire drives a DOCUMENT-level listener (htmx:afterSettle, htmx:beforeSwap).
    fire: (type, event) => document._fire(type, event || {}),
    flush: () => { const q = timers.splice(0, timers.length); q.forEach((t) => t.fn()); },
    lockedNow: () => fx.root.classList.contains('cal-theater-lock'),
    boxOpen: () => fx.dialog.querySelector('[data-theater-box]').classList.contains('tbopen'),
  };
}
