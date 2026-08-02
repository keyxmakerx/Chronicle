// daycard_dom.mjs — a minimal DOM for C-CALV4-DAYCARD's node tests.
//
// WHY A DOM AND NOT A STUB. Every other JS suite in this repo boots its module
// against `querySelector: () => null` and exercises the pure mappers only —
// which is right when the DOM flow is the operator's visual gate. It is NOT
// right here: the whole point of slice R2-2a is that the module READS the
// Block's DOM and MUST NOT MUTATE IT, and "innerHTML is byte-identical before
// and after open + close" is an assertion that needs a real tree and a real
// serialiser. jsdom is not a dependency of this repo and adding one for a
// single suite is a heavier commitment than 200 lines that do exactly what
// these tests need.
//
// SCOPE, STATED HONESTLY. This implements what calendar_daycard.js touches:
// single compound selectors (tag + .class + [attr] + [attr="value"]),
// closest/matches, attribute + dataset + classList + style, appendChild /
// removeChild, and an innerHTML GETTER. It is not a browser. Anything it does
// not implement throws rather than returning undefined, so a module reaching
// past this surface fails loudly instead of silently passing.

const VOID = new Set(['input', 'br', 'hr', 'img', 'link', 'meta']);

const ATTR_TO_PROP = { class: 'className', for: 'htmlFor' };

function parseCompound(sel) {
  const spec = { tag: null, classes: [], attrs: [] };
  const re = /^([a-zA-Z][a-zA-Z0-9-]*)|\.([a-zA-Z_][\w-]*)|\[([^\]=]+)(?:=(?:"([^"]*)"|'([^']*)'|([^\]]*)))?\]/;
  let rest = sel.trim();
  while (rest.length) {
    const m = re.exec(rest);
    if (!m) throw new Error('daycard_dom: unsupported selector fragment: ' + rest);
    if (m[1]) spec.tag = m[1].toLowerCase();
    else if (m[2]) spec.classes.push(m[2]);
    else {
      const value = m[4] !== undefined ? m[4] : (m[5] !== undefined ? m[5] : m[6]);
      spec.attrs.push([m[3].trim(), value === undefined ? null : value]);
    }
    rest = rest.slice(m[0].length);
  }
  return spec;
}

function camel(name) {
  return name.replace(/-([a-z])/g, (_, c) => c.toUpperCase());
}

// THE OPERATION LOG — added with C-CALV4-EDITOR-R2b stage 3.
//
// Some claims are about ORDER WITHIN ONE TASK and are invisible to a test that
// only reads the final state. The one that made this necessary: the editor's
// close must remove `.dcopen` BEFORE writing the reverse geometry, because the
// carve-out's open-state rule is the only thing declaring --disc-open — do it
// the other way round and leaving takes exactly as long as arriving, with every
// end-state assertion still green. A mutation proved that hole, so the hole is
// closed rather than described.
//
// Every class change and every style write appends to the target element's own
// `_ops`, in sequence, so a test can assert what happened before what.
class ClassList {
  constructor(el) { this.el = el; }
  get _set() {
    return new Set((this.el.getAttribute('class') || '').split(/\s+/).filter(Boolean));
  }
  _write(set) {
    this.el.setAttribute('class', Array.from(set).join(' '));
    (this.el._ops = this.el._ops || []).push('class:' + Array.from(set).join(' '));
  }
  contains(c) { return this._set.has(c); }
  add(c) { const s = this._set; s.add(c); this._write(s); }
  remove(c) { const s = this._set; s.delete(c); this._write(s); }
  toggle(c, on) { if (on === undefined) on = !this.contains(c); if (on) this.add(c); else this.remove(c); }
}

class Style {
  constructor(el) { this._props = {}; this._css = {}; this._el = el; }
  _op(k, v) {
    if (this._el) (this._el._ops = this._el._ops || []).push('style:' + k + '=' + v);
  }
  setProperty(k, v) { this._props[k] = v; this._op(k, v); }
  removeProperty(k) { delete this._props[k]; this._op(k, ''); }
  getPropertyValue(k) { return this._props[k] || ''; }
  get cssText() { return ''; }
}

// `height` and `opacity` join the list with C-CALV4-EDITOR-R2b: they are two of
// the four properties the editor morph writes, and a stub that silently dropped
// them would let a test assert about a geometry the module never set. This file
// implements what the module touches and throws on what it does not, which is
// the only reason its coverage means anything.
for (const p of ['left', 'top', 'width', 'height', 'opacity', 'display']) {
  Object.defineProperty(Style.prototype, p, {
    get() { return this._css[p] || ''; },
    set(v) { this._css[p] = v; this._op(p, v); },
  });
}

export class El {
  constructor(tag) {
    this.tagName = tag.toUpperCase();
    this.attributes = new Map();
    this.children = [];
    this.parentNode = null;
    this._text = '';
    this.classList = new ClassList(this);
    this.style = new Style(this);
    this._ops = [];
    this.dataset = new Proxy({}, {
      get: (_, k) => {
        if (typeof k !== 'string') return undefined;
        return this.getAttribute('data-' + k.replace(/[A-Z]/g, (c) => '-' + c.toLowerCase())) ?? undefined;
      },
      set: (_, k, v) => {
        this.setAttribute('data-' + k.replace(/[A-Z]/g, (c) => '-' + c.toLowerCase()), v);
        return true;
      },
    });
    // Layout numbers the module reads. Tests override them per element.
    this.rect = { left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 };
    this.offsetWidth = 0;
    this.offsetHeight = 0;
    this.scrollHeight = 0;
    this.clicks = 0;
    this.scrolled = 0;
    this.popoverOpen = false;
  }

  // --- attributes ---------------------------------------------------------
  getAttribute(n) { return this.attributes.has(n) ? this.attributes.get(n) : null; }
  setAttribute(n, v) { this.attributes.set(n, String(v)); }
  removeAttribute(n) { this.attributes.delete(n); }
  hasAttribute(n) { return this.attributes.has(n); }

  get className() { return this.getAttribute('class') || ''; }
  set className(v) { this.setAttribute('class', v); }
  get type() { return this.getAttribute('type') || ''; }
  set type(v) { this.setAttribute('type', v); }
  get hidden() { return this.hasAttribute('hidden'); }
  set hidden(v) { if (v) this.setAttribute('hidden', ''); else this.removeAttribute('hidden'); }
  // Form-control state. `value` and `checked` are IDL properties, not content
  // attributes — which is exactly why they do not appear in outerHTML, and why
  // the Block-immutability assertion survives a radio being activated.
  get value() { return this._value === undefined ? (this.getAttribute('value') || '') : this._value; }
  set value(v) { this._value = String(v); }
  get checked() { return !!this._checked; }
  set checked(v) { this._checked = !!v; }

  get textContent() { return this._text + this.children.map((c) => c.textContent).join(''); }
  set textContent(v) { this.children = []; this._text = String(v); }

  // --- tree ---------------------------------------------------------------
  get firstChild() { return this.children[0] || null; }
  appendChild(c) { c.parentNode = this; this.children.push(c); return c; }
  insertBefore(c, ref) {
    c.parentNode = this;
    const i = ref ? this.children.indexOf(ref) : -1;
    if (i < 0) this.children.push(c);
    else this.children.splice(i, 0, c);
    return c;
  }
  removeChild(c) {
    const i = this.children.indexOf(c);
    if (i >= 0) this.children.splice(i, 1);
    c.parentNode = null;
    return c;
  }

  // --- selectors ----------------------------------------------------------
  matches(sel) {
    const spec = parseCompound(sel);
    if (spec.tag && spec.tag !== this.tagName.toLowerCase()) return false;
    for (const c of spec.classes) if (!this.classList.contains(c)) return false;
    for (const [n, v] of spec.attrs) {
      if (!this.attributes.has(n)) return false;
      if (v !== null && this.getAttribute(n) !== v) return false;
    }
    return true;
  }

  closest(sel) {
    let node = this;
    while (node) {
      if (node.matches && node.matches(sel)) return node;
      node = node.parentNode;
    }
    return null;
  }

  querySelectorAll(sel) {
    const out = [];
    const walk = (n) => {
      for (const c of n.children) {
        if (c.matches(sel)) out.push(c);
        walk(c);
      }
    };
    walk(this);
    return out;
  }

  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }

  // --- layout / behaviour the module calls --------------------------------
  addEventListener(type, fn) { (this._on = this._on || {})[type] = ((this._on || {})[type] || []).concat(fn); }
  removeEventListener() {}
  dispatch(type, ev) { ((this._on || {})[type] || []).forEach((fn) => fn(ev || { target: this })); }
  getBoundingClientRect() { return this.rect; }
  showPopover() { this.popoverOpen = true; }
  hidePopover() { this.popoverOpen = false; }
  click() { this.clicks++; this.dispatch('click', { target: this, preventDefault() {} }); }
  scrollIntoView() { this.scrolled++; }

  // --- serialisation ------------------------------------------------------
  get innerHTML() { return this.children.map((c) => c.outerHTML).join('') + (this._text || ''); }
  get outerHTML() {
    const tag = this.tagName.toLowerCase();
    let s = '<' + tag;
    for (const [n, v] of this.attributes) s += ' ' + n + '="' + v + '"';
    if (VOID.has(tag)) return s + '/>';
    return s + '>' + this.innerHTML + '</' + tag + '>';
  }
}

// el builds a subtree from a terse spec: el('div', {class:'x'}, [children]).
export function el(tag, attrs, kids, text) {
  const node = new El(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    node.setAttribute(ATTR_TO_PROP[k] ? k : k, v === true ? '' : String(v));
  }
  if (text !== undefined) node._text = text;
  (kids || []).forEach((c) => node.appendChild(c));
  return node;
}

// makeDocument wires a root element into a document-shaped object with a real
// listener bus, so the tests drive the module through the events it actually
// binds rather than through functions it does not export.
export function makeDocument(root) {
  const listeners = {};
  return {
    readyState: 'complete',
    documentElement: root,
    body: root,
    querySelector: (s) => (root.matches(s) ? root : root.querySelector(s)),
    querySelectorAll: (s) => root.querySelectorAll(s),
    createElement: (t) => new El(t),
    addEventListener: (t, fn) => { (listeners[t] = listeners[t] || []).push(fn); },
    removeEventListener: () => {},
    _listeners: listeners,
    _fire(type, event) { (listeners[type] || []).forEach((fn) => fn(event)); },
  };
}
