// availability_harness.mjs — dependency-free mini-DOM harness for
// static/js/availability.js (the availability scheduler client).
//
// availability.js is a browser IIFE that builds a real element tree (createElement
// + appendChild), toggles classes, runs querySelector with attribute/class/
// descendant selectors, and drives a month<->week morph on setTimeout. To exercise
// it headless under `node --test` (no jsdom — the CI JS suite is deps-free) we
// implement just enough of the DOM: an Element with classList/attrs/style/children,
// a small CSS-selector engine (tag / #id / .class / [attr] / [attr="v"] / descendant
// combinator), and a document/window wired into a vm sandbox. Real timers are passed
// through so the 340ms morph cleanup fires against a normal clock (tests await it).

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'availability.js');
const SRC = readFileSync(jsPath, 'utf8');

// ---- selector engine ----------------------------------------------------
function parseCompound(str) {
  const c = { tag: '', id: '', classes: [], attrs: [] };
  const re = /([a-zA-Z][\w-]*)|#([\w-]+)|\.([\w-]+)|\[([\w-]+)(?:(=)"?([^"\]]*)"?)?\]/g;
  let m;
  while ((m = re.exec(str))) {
    if (m[1]) c.tag = m[1].toLowerCase();
    else if (m[2]) c.id = m[2];
    else if (m[3]) c.classes.push(m[3]);
    else if (m[4]) c.attrs.push({ name: m[4], op: m[5] || '', value: m[6] });
  }
  return c;
}
function parseSelector(sel) { return sel.trim().split(/\s+/).map(parseCompound); }

function matchCompound(el, c) {
  if (c.tag && el.tagName !== c.tag) return false;
  if (c.id && el.id !== c.id) return false;
  for (const cls of c.classes) if (!el._classes.has(cls)) return false;
  for (const a of c.attrs) {
    const v = el.getAttribute(a.name);
    if (a.op === '=') { if (v !== a.value) return false; }
    else if (v === null) return false;
  }
  return true;
}
// el matches the full (possibly descendant-combinated) selector.
function matchesChain(el, steps) {
  let i = steps.length - 1;
  if (!matchCompound(el, steps[i])) return false;
  i--;
  let node = el.parentNode;
  while (i >= 0) {
    let found = false;
    while (node) {
      if (matchCompound(node, steps[i])) { found = true; node = node.parentNode; break; }
      node = node.parentNode;
    }
    if (!found) return false;
    i--;
  }
  return true;
}

// ---- Element ------------------------------------------------------------
class MiniStyle {
  setProperty(k, v) { this[k] = v; }
}
class Element {
  constructor(tag) {
    this.tagName = String(tag).toLowerCase();
    this._attrs = {};
    this._classes = new Set();
    this.style = new MiniStyle();
    this._children = [];
    this.parentNode = null;
    this._text = '';
    this._html = '';
    this._ev = {};
    this.hidden = false;
    const self = this;
    this.classList = {
      add() { for (const c of arguments) self._classes.add(c); },
      remove() { for (const c of arguments) self._classes.delete(c); },
      toggle(c, force) {
        const on = force === undefined ? !self._classes.has(c) : force;
        if (on) self._classes.add(c); else self._classes.delete(c);
        return on;
      },
      contains(c) { return self._classes.has(c); },
    };
  }
  get className() { return Array.from(this._classes).join(' '); }
  set className(v) { this._classes = new Set(String(v).split(/\s+/).filter(Boolean)); }
  get id() { return this._attrs.id || ''; }
  set id(v) { this._attrs.id = String(v); }
  get children() { return this._children; }
  get firstChild() { return this._children[0] || null; }
  setAttribute(n, v) { if (n === 'class') { this.className = v; return; } this._attrs[n] = String(v); }
  getAttribute(n) { if (n === 'class') return this.className; return n in this._attrs ? this._attrs[n] : null; }
  removeAttribute(n) { delete this._attrs[n]; }
  hasAttribute(n) { return n in this._attrs; }
  appendChild(child) {
    if (child.parentNode) child.parentNode.removeChild(child);
    child.parentNode = this; this._children.push(child); return child;
  }
  removeChild(child) {
    const i = this._children.indexOf(child);
    if (i >= 0) { this._children.splice(i, 1); child.parentNode = null; }
    return child;
  }
  insertBefore(node, ref) {
    if (node.parentNode) node.parentNode.removeChild(node);
    const i = ref ? this._children.indexOf(ref) : -1;
    if (i < 0) this._children.push(node); else this._children.splice(i, 0, node);
    node.parentNode = this; return node;
  }
  set textContent(v) { this._text = v == null ? '' : String(v); this._html = ''; this._children = []; }
  get textContent() {
    const own = this._html ? this._html.replace(/<[^>]*>/g, '') : this._text;
    return own + this._children.map((c) => c.textContent).join('');
  }
  set innerHTML(v) { this._children = []; this._text = ''; this._html = v ? String(v) : ''; }
  get innerHTML() { return this._html; }
  get offsetWidth() { return 0; }
  getBoundingClientRect() { return { top: 0, left: 0, right: 0, bottom: 0, width: 100, height: 0 }; }
  addEventListener(type, fn) { (this._ev[type] = this._ev[type] || []).push(fn); }
  removeEventListener(type, fn) { const a = this._ev[type]; if (a) { const i = a.indexOf(fn); if (i >= 0) a.splice(i, 1); } }
  __fire(type, props) {
    const evt = Object.assign({ type, target: this, preventDefault() {}, stopPropagation() {} }, props || {});
    (this._ev[type] || []).slice().forEach((fn) => fn(evt));
    return evt;
  }
  click() { return this.__fire('click', {}); }
  _all() { const out = []; const walk = (n) => n._children.forEach((c) => { out.push(c); walk(c); }); walk(this); return out; }
  querySelectorAll(sel) { const steps = parseSelector(sel); return this._all().filter((d) => matchesChain(d, steps)); }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
}

// ---- boot ---------------------------------------------------------------
// opts.reduced -> matchMedia reports prefers-reduced-motion.
// opts.canDetail (default true) -> DM/owner detail view.
// opts.overlay(weekISO) -> custom overlay payload factory.
// opts.exceptions -> seed rows [{id, onDate, startMinute, endMinute, state}] for
//   the stateful exceptions mock (C-SCHED-OUT-THIS-WEEK): GET reads the current
//   store, PUT replaces one date's rows (replace-day semantics, mirrors the real
//   ReplaceDayExceptions endpoint), DELETE removes one row by id (404 if absent —
//   the real endpoint's "already gone" case). Defaults to an empty store, same
//   externally-observable GET behavior as before this mock existed.
export function boot(opts = {}) {
  const doc = new Element('#document');
  const head = new Element('head');
  const body = new Element('body');
  doc.appendChild(head); doc.appendChild(body);

  const root = new Element('div');
  root.setAttribute('data-availability-root', '');
  root.setAttribute('data-campaign-id', 'camp1');
  root.setAttribute('data-user-id', 'u1');
  root.setAttribute('data-can-detail', String(opts.canDetail !== false));
  root.setAttribute('data-tz', 'America/New_York');
  const mk = (tag, attrs) => { const e = new Element(tag); for (const k in (attrs || {})) e.setAttribute(k, attrs[k]); return e; };
  const tablist = mk('div', { role: 'tablist' });
  const tMine = mk('button', { 'data-avail-tab': 'mine', 'aria-selected': 'true' });
  const tOv = mk('button', { 'data-avail-tab': 'overlay', 'aria-selected': 'false' });
  tablist.appendChild(tMine); tablist.appendChild(tOv);
  const pMine = mk('section', { 'data-avail-panel': 'mine' });
  const pOv = mk('section', { 'data-avail-panel': 'overlay' }); pOv.hidden = true;
  const live = mk('div', { 'data-avail-live': '', 'aria-live': 'polite' });
  const tzLabel = mk('span', { 'data-tz-label': '' });
  const tzText = mk('span', { 'data-tz-label-text': '' });
  tzLabel.appendChild(tzText);
  root.appendChild(tzLabel); root.appendChild(tablist);
  root.appendChild(pMine); root.appendChild(pOv); root.appendChild(live);
  body.appendChild(root);

  const overlayFactory = opts.overlay || defaultOverlay;
  let exceptionsStore = (opts.exceptions || []).map((e) => Object.assign({}, e));
  let nextExcId = 1;
  let putCount = 0;
  const failPutAfter = opts.failPutAfter == null ? Infinity : opts.failPutAfter;
  const resp = (ok, status, payload) => Promise.resolve({
    ok, status, json: () => Promise.resolve(payload), text: () => Promise.resolve(''),
  });
  const Chronicle = {
    apiFetch(url, fopts = {}) {
      const method = (fopts.method || 'GET').toUpperCase();
      const excMatch = url.match(/availability\/exceptions(?:\/([^/?]+))?/);
      if (excMatch) {
        const eid = excMatch[1];
        if (method === 'DELETE' && eid) {
          const before = exceptionsStore.length;
          exceptionsStore = exceptionsStore.filter((e) => e.id !== eid);
          const found = exceptionsStore.length !== before;
          return resp(found, found ? 200 : 404, found ? { status: 'ok' } : { error: 'not found' });
        }
        if (method === 'PUT') {
          // opts.failPutAfter simulates the real per-user cap (C-SCHED-P2 0d)
          // rejecting a write partway through a batch of sequential PUTs.
          if (putCount >= failPutAfter) {
            return resp(false, 400, { error: 'too many availability exceptions; delete some before adding more' });
          }
          putCount++;
          const onDate = fopts.body.onDate;
          exceptionsStore = exceptionsStore.filter((e) => e.onDate !== onDate);
          (fopts.body.blocks || []).forEach((b) => {
            exceptionsStore.push({ id: 'e' + (nextExcId++), onDate, startMinute: b.startMinute, endMinute: b.endMinute, state: b.state });
          });
          return resp(true, 200, { status: 'ok' });
        }
        // GET (bare or with an id suffix that isn't a real route in this app).
        return resp(true, 200, exceptionsStore.slice());
      }
      let payload = null;
      if (/availability\/mine/.test(url)) payload = { tz: 'America/New_York', blocks: [] };
      else if (/availability\/overlay/.test(url)) { const m = url.match(/week=([0-9-]+)/); payload = overlayFactory(m ? m[1] : '2026-07-13'); }
      else if (/proposals/.test(url)) payload = { link: '/campaigns/camp1/proposals/p1' };
      return resp(true, 200, payload);
    },
    escapeHtml: (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c])),
    notify() {},
  };

  const location = { search: opts.search || '', href: 'https://x.test/campaigns/camp1/availability' };
  const sandbox = {
    console, Chronicle, location,
    setTimeout, clearTimeout, Intl, Date, Math, Array, String, Number, JSON,
    encodeURIComponent, parseInt, parseFloat, isNaN,
    matchMedia: () => ({ matches: !!opts.reduced }),
    document: {
      readyState: 'complete',
      head, body,
      createElement: (t) => new Element(t),
      createTextNode: (s) => { const t = new Element('#text'); t._text = s == null ? '' : String(s); return t; },
      querySelector: (s) => doc.querySelector(s),
      querySelectorAll: (s) => doc.querySelectorAll(s),
      addEventListener: () => {},
      removeEventListener: () => {},
      getElementById: (id) => doc.querySelector('#' + id),
    },
  };
  sandbox.window = sandbox;
  sandbox.global = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(SRC, sandbox, { filename: 'availability.js' });
  return { doc, root, window: sandbox, live, getExceptions: () => exceptionsStore.slice() };
}

// ---- date helpers mirroring availability.js's own UTC-anchored civil-date
// math, exported so tests can compute "this real week" independently (the
// feature under test is deliberately based on the actual current date, so
// tests assert against the same live computation rather than a fixed date).
export function isoOfUTC(d) {
  return d.getUTCFullYear() + '-' + String(d.getUTCMonth() + 1).padStart(2, '0') + '-' + String(d.getUTCDate()).padStart(2, '0');
}
export function todayUTC() {
  const n = new Date();
  return new Date(Date.UTC(n.getFullYear(), n.getMonth(), n.getDate()));
}
export function mondayOfUTC(d) {
  const off = (d.getUTCDay() + 6) % 7; // days since Monday
  return new Date(d.getTime() - off * 86400000);
}
export function thisWeekDates() {
  const monday = mondayOfUTC(todayUTC());
  const out = [];
  for (let i = 0; i < 7; i++) out.push(isoOfUTC(new Date(monday.getTime() + i * 86400000)));
  return out;
}

// A default overlay payload: hours 18:00–21:59 have both members free (19:00
// preferred), matching the encoding the tests assert against.
export function defaultOverlay(weekISO) {
  const p = weekISO.split('-');
  const base = Date.UTC(+p[0], +p[1] - 1, +p[2]);
  const iso = (n) => { const d = new Date(base + n * 86400000); return d.getUTCFullYear() + '-' + String(d.getUTCMonth() + 1).padStart(2, '0') + '-' + String(d.getUTCDate()).padStart(2, '0'); };
  const wd = (n) => new Date(base + n * 86400000).getUTCDay();
  const days = [];
  for (let i = 0; i < 7; i++) {
    const hours = [];
    for (let h = 0; h < 24; h++) {
      const free = (h >= 18 && h < 22) ? 2 : 0;
      hours.push({ free, prefer: h === 19 ? 2 : 0, freeIds: ['u1', 'u2'].slice(0, free), preferIds: h === 19 ? ['u1', 'u2'] : [] });
    }
    days.push({ date: iso(i), weekday: wd(i), hours });
  }
  return {
    days, totalMembers: 2, viewerTz: 'America/New_York',
    members: [
      { userId: 'u1', name: 'Alice', role: 'DM', color: '#4488ff', lanes: [{ day: 2, start: 1080, end: 1320, state: 'available' }] },
      { userId: 'u2', name: 'Bob', role: 'player', color: '#ff8844', lanes: [] },
    ],
  };
}

export const wait = (ms) => new Promise((r) => setTimeout(r, ms));
