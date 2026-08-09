// impact_tree_registration.test.mjs — the "Impact Overview" diagram on the
// custom-system upload preview must actually render.
//
// C-WIDGET-REGISTERWIDGET, the item stage 9 booked rather than guessed at.
// static/js/widgets/impact_tree.js registered itself with
// `Chronicle.registerWidget('impact_tree', mount)` — an API boot.js has never
// had (`.ai/todo.md` lists it as an unchecked Sprint Q-1 future item; boot.js
// exposes `Chronicle.register`, `mountWidgets`, `mountWidget`, `destroyWidget`).
// The call sat behind `if (window.Chronicle && Chronicle.registerWidget)`, so
// the module loaded and self-disabled, falling through to a DOMContentLoaded
// scan that is the WRONG lifecycle for this mount: the preview fragment is
// delivered by an htmx swap (custom_system.templ posts to
// /campaigns/:id/systems/preview with hx-target="#custom-system-section"), long
// after DOMContentLoaded has been and gone. Net effect on a live upload: an
// "Impact Overview" heading with a sitemap icon over a permanently blank div.
//
// WHY THE TEST IS SHAPED LIKE THIS. Firing DOMContentLoaded with the mount
// already in the document would pass with OR without the fix, because the old
// fallback would catch it — the bug is invisible to that ordering. So the test
// reproduces the real delivery: boot with an EMPTY container (defer scripts run,
// DOMContentLoaded fires), and only THEN attach the fragment and dispatch
// htmx:afterSettle with detail.target, which is the single path boot.js offers
// for swapped-in content. Only a real `Chronicle.register` entry is reachable
// from there.
//
// The second half of the defect — the file was in no <script src> anywhere, so
// the browser never fetched it — is pinned by tools/check-widget-mounts.sh now
// that `impact_tree` is out of tools/widget-mount-allowlist.txt: drop the
// base.templ tag and the guard reports DEAD.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const read = (...p) => readFileSync(path.join(here, '..', '..', ...p), 'utf8');
const BOOT = read('static', 'js', 'boot.js');
const WIDGET = read('static', 'js', 'widgets', 'impact_tree.js');

// --- a DOM the size of this widget -----------------------------------------
// Implements exactly what impact_tree.js and boot.js's mount path touch. Same
// policy as test/js/daycard_dom.mjs: jsdom is not a dependency of this repo and
// is not worth adding for one suite, and anything outside the implemented
// surface should fail loudly rather than pass quietly.

class ClassList {
  constructor(el) { this.el = el; this.set = new Set(); }
  add(c) { this.set.add(c); }
  remove(c) { this.set.delete(c); }
  contains(c) { return this.set.has(c); }
  /** Returns the resulting state, which is what the collapse toggle reads. */
  toggle(c, on) {
    const next = on === undefined ? !this.set.has(c) : !!on;
    if (next) this.set.add(c); else this.set.delete(c);
    return next;
  }
}

class El {
  constructor(tag) {
    this.tagName = tag.toUpperCase();
    this.attributes = [];
    this.children = [];
    this.parentNode = null;
    this._text = '';
    this._html = '';
    this.classList = new ClassList(this);
    this.style = {};
    this._on = {};
  }

  getAttribute(n) {
    const a = this.attributes.find((x) => x.name === n);
    return a ? a.value : null;
  }

  setAttribute(n, v) {
    const a = this.attributes.find((x) => x.name === n);
    if (a) a.value = String(v);
    else this.attributes.push({ name: n, value: String(v) });
  }

  get className() { return this.getAttribute('class') || ''; }
  set className(v) { this.setAttribute('class', v); }

  get textContent() { return this._text + this.children.map((c) => c.textContent).join(''); }
  set textContent(v) { this.children = []; this._html = ''; this._text = String(v); }

  // A setter, unlike daycard_dom's getter-only innerHTML: impact_tree.js writes
  // it (the chevron markup, and `el.innerHTML = ''` to clear before painting).
  // Raw markup is stored, not parsed — nothing in this widget reads it back.
  get innerHTML() { return this._html + this.children.map((c) => c.outerHTML).join(''); }
  set innerHTML(v) { this.children = []; this._text = ''; this._html = String(v); }

  get outerHTML() {
    const tag = this.tagName.toLowerCase();
    const attrs = this.attributes.map((a) => ` ${a.name}="${a.value}"`).join('');
    return `<${tag}${attrs}>${this._html}${this._text}${this.children.map((c) => c.outerHTML).join('')}</${tag}>`;
  }

  appendChild(c) { c.parentNode = this; this.children.push(c); return c; }

  addEventListener(type, fn) { (this._on[type] = this._on[type] || []).push(fn); }
  removeEventListener() {}

  /**
   * Supports `[:scope > ]tag[attr][attr="v"]…` — the widget's `:scope > ul`,
   * boot.js's `[data-widget]` / `[data-widget="name"]`, and the compounds its
   * own load path uses (`form[data-track-changes]`). Anything richer throws
   * rather than silently matching nothing, so a module that grows past this
   * surface fails instead of quietly passing.
   */
  querySelectorAll(sel) {
    const scoped = sel.startsWith(':scope > ');
    const rest = scoped ? sel.slice(':scope > '.length) : sel;
    const m = /^([a-z]+)?((?:\[[^\]]+\])*)$/.exec(rest.trim());
    if (!m) throw new Error('impact_tree test DOM: unsupported selector ' + sel);
    const tag = m[1] || null;
    const attrs = [...(m[2] || '').matchAll(/\[([^\]=]+)(?:="([^"]*)")?\]/g)]
      .map((a) => [a[1], a[2]]);
    const match = (n) => {
      if (tag && n.tagName.toLowerCase() !== tag) return false;
      return attrs.every(([name, value]) => {
        const v = n.getAttribute(name);
        return v !== null && (value === undefined || v === value);
      });
    };
    if (scoped) return this.children.filter(match);
    const out = [];
    const walk = (n) => { for (const c of n.children) { if (match(c)) out.push(c); walk(c); } };
    walk(this);
    return out;
  }

  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
  closest() { return null; }
}

/**
 * Evaluate boot.js and impact_tree.js in one context over a shared document,
 * exactly as two `defer` tags on base.templ do (boot.js is line 70, every
 * widget follows it, and defer preserves document order).
 *
 * @returns {{container: El, afterSettle: Function, Chronicle: Object,
 *            errors: string[], warnings: string[]}}
 */
function boot() {
  const docHandlers = {};
  const errors = [];
  const warnings = [];
  const root = new El('div');
  // #custom-system-section — campaign_system_handler.go's hx-target.
  const container = new El('div');
  root.appendChild(container);

  const document = {
    // 'loading' while the deferred scripts evaluate: register() must not mount
    // eagerly, so the lifecycle listeners are the only mount path, as on a real
    // page load.
    readyState: 'loading',
    documentElement: root,
    body: Object.assign(new El('body'), { classList: new ClassList(null) }),
    createElement: (t) => new El(t),
    querySelector: (s) => root.querySelector(s),
    querySelectorAll: (s) => root.querySelectorAll(s),
    getElementById: () => null,
    addEventListener: (ev, fn) => { (docHandlers[ev] = docHandlers[ev] || []).push(fn); },
    removeEventListener: () => {},
    dispatchEvent: () => {},
    cookie: '',
  };
  const fire = (ev, detail) => (docHandlers[ev] || []).forEach((fn) => fn({ type: ev, detail }));

  const sandbox = {
    document,
    console: {
      log() {}, info() {}, debug() {},
      warn: (...a) => warnings.push(a.join(' ')),
      error: (...a) => errors.push(a.join(' ')),
    },
    // boot.js dereferences htmx.config at load time (selfRequestsOnly etc.);
    // without the stub it throws ReferenceError before ever registering its
    // lifecycle listeners, which would mask the result under test.
    htmx: { config: {} },
    setTimeout: (fn) => { fn(); return 0; },
    clearTimeout: () => {},
    requestAnimationFrame: (fn) => { fn(); return 0; },
    CustomEvent: class { constructor(t, o) { this.type = t; this.detail = o && o.detail; } },
    localStorage: { getItem: () => null, setItem() {}, removeItem() {} },
    location: { pathname: '/campaigns/1/systems' },
    navigator: { userAgent: 'node' },
    fetch: () => Promise.reject(new Error('no network in this test')),
  };
  sandbox.window = sandbox;
  sandbox.globalThis = sandbox;
  sandbox.self = sandbox;
  sandbox.window.addEventListener = () => {};
  sandbox.window.dispatchEvent = () => {};

  const ctx = vm.createContext(sandbox);
  vm.runInContext(BOOT, ctx, { filename: 'boot.js' });
  vm.runInContext(WIDGET, ctx, { filename: 'impact_tree.js' });

  // Load settles with the container still EMPTY — the preview fragment has not
  // been requested yet, let alone swapped in.
  document.readyState = 'complete';
  fire('DOMContentLoaded');

  return {
    container,
    Chronicle: sandbox.Chronicle,
    errors,
    warnings,
    /**
     * Attach the preview fragment's mount and announce it the way htmx does.
     * Built node-wise rather than via an innerHTML string because this DOM
     * stores markup without parsing it; the resulting tree is what a browser
     * hands boot.js either way.
     */
    afterSettle(treeJSON) {
      const mount = new El('div');
      mount.setAttribute('id', 'impact-tree');
      mount.setAttribute('data-widget', 'impact_tree');
      mount.setAttribute('data-tree', treeJSON);
      container.appendChild(mount);
      fire('htmx:afterSettle', { target: container });
      return mount;
    },
  };
}

// Shaped like internal/systems/preview.go's BuildImpactTree output.
const TREE = {
  label: 'Probe System v1.0.0',
  type: 'root',
  children: [
    {
      label: 'Reference Data',
      type: 'section',
      children: [{ label: 'Spells', type: 'category', badge: '12' }],
    },
    { label: 'No entity presets declared', type: 'warning' },
  ],
};

test('impact_tree registers through the API boot.js actually exposes', () => {
  const app = boot();
  assert.equal(typeof app.Chronicle.register, 'function');
  // The regression itself: the widget aimed at a name that has never existed.
  assert.equal(app.Chronicle.registerWidget, undefined,
    'boot.js exposes no registerWidget — .ai/todo.md still lists it as an ' +
    'unchecked Sprint Q-1 item. If that changes, this widget is not the place ' +
    'to find out.');
  assert.ok(app.Chronicle.mountWidget, 'boot.js mount API is present');
});

test('the impact tree paints when the preview fragment is swapped in', () => {
  const app = boot();
  const mount = app.afterSettle(JSON.stringify(TREE));

  assert.notEqual(mount.innerHTML.length, 0,
    'the "Impact Overview" card rendered nothing — this is the shipped ' +
    'behaviour when the widget registers with an API boot.js does not have');
  const text = mount.textContent;
  assert.match(text, /Probe System v1\.0\.0/);
  assert.match(text, /Reference Data/);
  assert.match(text, /Spells/);
  assert.match(text, /12/, 'category badge renders');
  assert.match(text, /No entity presets declared/, 'warning nodes render');
  assert.deepEqual(app.errors, []);
});

test('a mount swapped in after load is never reported as an unmatched widget', () => {
  // Stage 9 added the once-per-name console.warn for widgets whose JS ships on
  // no page. impact_tree must no longer trip it, in either lifecycle.
  const app = boot();
  app.afterSettle(JSON.stringify(TREE));
  assert.deepEqual(app.warnings.filter((w) => w.includes('impact_tree')), []);
});

test('a malformed data-tree degrades to a message, not a thrown mount', () => {
  const app = boot();
  const mount = app.afterSettle('{not json');
  assert.match(mount.textContent, /Failed to parse tree data/);
  assert.deepEqual(app.errors, [],
    'boot.js catches init() throws and console.errors them; the widget should ' +
    'not be getting there at all');
});
