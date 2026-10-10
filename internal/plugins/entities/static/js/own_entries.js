/**
 * own_entries.js — the Directors' "Your own entries" page (signed mockup).
 *
 * One paper page with a tab per pick list the game system has (Ancestry,
 * Kit…, named by the system). Tapping an entry, or "Add", slides the hero
 * creator's leaf out from under the page with Edit and Preview ribbons;
 * Preview draws the entry as a player sees it in the creator.
 *
 * The pick lists, their labels and the facts the system's own entries carry
 * come from the creator's plan; the campaign's entries are read and written
 * through the systems plugin's entry API, which also enforces the Director
 * rule. Descriptions are HTML the server sanitizes; a passage wrapped in
 * <span data-secret> is kept from players, as on pages.
 *
 * Served from the plugin body-script registry so it survives boosted
 * navigation. Re-wires after HTMX swaps.
 */
(function () {
  'use strict';

  var DIRTY = 'own-entry';

  function esc(v) {
    var s = (v == null) ? '' : String(v);
    return s.replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function words(key) {
    var s = String(key).replace(/_display$/, '').replace(/[_-]+/g, ' ').trim();
    return s.charAt(0).toUpperCase() + s.slice(1);
  }
  function lower(s) { return String(s || '').toLowerCase(); }
  function an(w) { return /^[aeiou]/i.test(w) ? 'an' : 'a'; }
  // A fact's label becomes the property key the creator reads ("Size" → size).
  function keyOf(label) {
    var k = lower(label).replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
    if (!/^[a-z]/.test(k)) k = 'f_' + k;
    return k.slice(0, 40);
  }
  function isFact(v) { return typeof v === 'number' || (typeof v === 'string' && v.length > 0 && v.length <= 40); }
  function dirty(on) {
    if (!window.Chronicle) return;
    if (on && Chronicle.markDirty) Chronicle.markDirty(DIRTY);
    if (!on && Chronicle.markClean) Chronicle.markClean(DIRTY);
  }
  function notify(msg, kind) { if (window.Chronicle && Chronicle.notify) Chronicle.notify(msg, kind || 'success'); }
  function api(url, opts) {
    if (window.Chronicle && Chronicle.apiFetch) return Chronicle.apiFetch(url, opts);
    var o = { method: opts && opts.method || 'GET', credentials: 'same-origin', headers: { Accept: 'application/json' } };
    if (opts && opts.body) { o.headers['Content-Type'] = 'application/json'; o.body = JSON.stringify(opts.body); }
    if (opts && opts.csrfToken) o.headers['X-CSRF-Token'] = opts.csrfToken;
    return fetch(url, o);
  }
  function json(r) {
    return r.json().catch(function () { return {}; }).then(function (b) { return { ok: r.ok, status: r.status, body: b }; });
  }

  // The facts a system's own entries share: scalar properties present on at
  // least half of them, budget keys and display twins aside, most common
  // first, at most four. A new entry starts with these lines.
  function suggestedFacts(pkg) {
    var count = {};
    pkg.forEach(function (c) {
      Object.keys(c.properties || {}).forEach(function (k) {
        if (/(_display|_points)$/.test(k) || !isFact(c.properties[k])) return;
        count[k] = (count[k] || 0) + 1;
      });
    });
    return Object.keys(count).filter(function (k) { return count[k] * 2 >= pkg.length; })
      .sort(function (a, b) { return count[b] - count[a] || (a < b ? -1 : 1); }).slice(0, 4);
  }

  function Entries(root) {
    this.root = root;
    this.planUrl = root.getAttribute('data-plan-url');
    this.entriesUrl = root.getAttribute('data-entries-url');
    this.csrf = root.getAttribute('data-csrf') || '';
    this.system = '';
    this.fields = [];   // {key, label, pkg, facts}
    this.entries = [];  // the campaign's own, every field
    this.field = 0;
    this.open = null;   // entry id, or 'new'
    this.draft = null;
    this.tab = 'edit';
    this.confirm = false;
    this.busy = false;
  }

  Entries.prototype.load = function () {
    var self = this;
    Promise.all([api(this.planUrl).then(json), api(this.entriesUrl).then(json)]).then(function (res) {
      if (!res[0].ok || !res[1].ok) throw new Error('load');
      var plan = res[0].body || {};
      self.system = plan.systemName || 'The game system';
      self.fields = (plan.steps || []).map(function (st) {
        var pkg = (st.choices || []).filter(function (c) { return c.source !== 'campaign'; });
        return { key: st.fieldKey, label: st.label, pkg: pkg.length, facts: suggestedFacts(pkg) };
      });
      self.entries = (res[1].body && res[1].body.entries) || [];
      self.render();
    }).catch(function () {
      var t = self.root.querySelector('.hc-loading__text');
      if (t) t.textContent = 'Your entries could not be opened. Reload the page to try again.';
    });
  };

  Entries.prototype.cur = function () { return this.fields[this.field]; };
  Entries.prototype.mine = function (key) {
    return this.entries.filter(function (e) { return e.fieldKey === key; })
      .sort(function (a, b) { return lower(a.name) < lower(b.name) ? -1 : 1; });
  };

  function plural(w) {
    w = lower(w);
    if (/[^aeiou]y$/.test(w)) return w.slice(0, -1) + 'ies';
    if (/(s|x|z|ch|sh)$/.test(w)) return w + 'es';
    return w + 's';
  }
  // Numbers stay numbers so the creator can do sums with them ("3" points).
  function factValue(v) { v = v.trim(); return /^-?\d+(\.\d+)?$/.test(v) ? Number(v) : v; }
  function draftOf(e, f) {
    if (!e) {
      return { id: '', name: '', summary: '', text: '', who: 'everyone',
        facts: f.facts.map(function (k) { return { label: words(k), value: '', key: k }; }) };
    }
    var props = e.properties || {};
    return { id: e.id, name: e.name || '', summary: e.summary || '', text: e.description || '', who: e.visibility || 'everyone',
      facts: Object.keys(props).map(function (k) { return { label: words(k), value: String(props[k]), key: k }; }) };
  }
  // The writing box's HTML reduced to the few tags it can make, parsed in an
  // inert document and stripped of every attribute but data-secret, so the
  // preview never runs anything dropped into the box. The server sanitizes
  // what is saved on its own.
  var KEEP = { P: 1, BR: 1, B: 1, STRONG: 1, I: 1, EM: 1, U: 1, SPAN: 1, DIV: 1, UL: 1, OL: 1, LI: 1 };
  function tidy(html) {
    var doc = new DOMParser().parseFromString('<div>' + (html || '') + '</div>', 'text/html');
    var root = doc.body.firstChild;
    (function walk(el) {
      Array.prototype.slice.call(el.childNodes).forEach(function (n) {
        if (n.nodeType === 3) return;
        if (n.nodeType !== 1) { n.remove(); return; }
        walk(n);
        if (!KEEP[n.tagName]) {
          if (/^(SCRIPT|STYLE|IFRAME|OBJECT|EMBED|TEMPLATE|SVG|MATH)$/i.test(n.tagName)) n.remove();
          else n.replaceWith.apply(n, Array.prototype.slice.call(n.childNodes));
          return;
        }
        var secret = n.hasAttribute('data-secret');
        Array.prototype.slice.call(n.attributes).forEach(function (a) { n.removeAttribute(a.name); });
        if (secret) n.setAttribute('data-secret', 'true');
      });
    })(root);
    return root;
  }
  // The visible part of whatever scrolls the page: the app's main column, or
  // the window when nothing inside it scrolls.
  function scrollerRect(el) {
    for (var e = el.parentElement; e && e !== document.body; e = e.parentElement) {
      if (/(auto|scroll)/.test(getComputedStyle(e).overflowY) && e.scrollHeight > e.clientHeight) {
        var r = e.getBoundingClientRect();
        return { top: r.top, height: e.clientHeight };
      }
    }
    return { top: 0, height: window.innerHeight };
  }
  function sameDraft(a, b) { return JSON.stringify(a) === JSON.stringify(b); }

  Entries.prototype.render = function () {
    var open = this.open !== null;
    this.root.innerHTML = '<div class="hc-wrap paper-stack' + (open ? ' is-open' : '') + '">' + this.pageHTML() +
      '<aside class="hc-leaf paper' + (open ? ' is-open' : '') + '" aria-label="Entry" aria-hidden="' + !open + '"></aside></div>';
    if (open) this.fillLeaf();
  };

  Entries.prototype.pageHTML = function () {
    var self = this, f = this.cur();
    var head = '<div class="hc-head"><p class="paper-kind">' + esc(this.system) + ' · Directors only</p><h1 tabindex="-1">Your own entries</h1>' +
      '<p>Entries you add here sit beside ' + esc(this.system) + '’s own in the hero creator and the sheet’s pick lists. A system update never changes them.</p></div>';
    if (!f) {
      return '<section class="hc-page paper" aria-label="Your own entries">' + head +
        '<p class="hc-empty">' + esc(this.system) + ' doesn’t list any choices for making a hero, so there is nothing to add to here.</p></section>';
    }
    var tabs = this.fields.map(function (x, i) {
      return '<button type="button" role="tab" data-field="' + i + '" aria-selected="' + (i === self.field) + '">' + esc(x.label) +
        '<small>' + self.mine(x.key).length + '</small></button>';
    }).join('');
    var mine = this.mine(f.key), pl = plural(f.label);
    var rows = mine.length ? '<ul class="oe-list">' + mine.map(function (e) {
      return '<li><button type="button" class="hc-row" data-open="' + esc(e.id) + '" aria-expanded="' + (self.open === String(e.id)) + '"><b>' + esc(e.name) +
        (e.visibility === 'directors' ? ' <span class="hc-seal oe-seal--dm">Directors only</span>' : '') +
        '</b><span class="hc-row__go" aria-hidden="true">Edit ›</span><small>' + esc(e.summary) + '</small></button></li>';
    }).join('') + '</ul>'
      : '<p class="hc-empty">No ' + esc(pl) + ' of your own yet. Add one and players see it next to ' + esc(this.system) + '’s ' + esc(pl) + ' when they make a hero.</p>';
    return '<section class="hc-page paper" aria-label="Your own entries">' + head +
      '<div class="oe-fields" role="tablist" aria-label="Pick lists">' + tabs + '</div>' + rows +
      '<p class="oe-pkg">' + esc(this.system) + ' brings ' + f.pkg + ' ' + esc(f.pkg === 1 ? lower(f.label) : pl) + ' of its own; they are not changed here.</p>' +
      '<div class="hc-foot"><span>' + mine.length + ' of your own</span><span class="hc-foot__sp"></span>' +
      '<button type="button" class="hc-btn" data-act="add">+ Add ' + an(f.label) + ' ' + esc(lower(f.label)) + '</button></div></section>';
  };

  Entries.prototype.writeHTML = function (d) {
    var f = this.cur();
    var facts = d.facts.map(function (p, i) {
      return '<input aria-label="Fact name" data-fact="' + i + '" data-part="label" maxlength="40" value="' + esc(p.label) + '" placeholder="Name, e.g. Size">' +
        '<input aria-label="Fact value" data-fact="' + i + '" data-part="value" maxlength="200" value="' + esc(p.value) + '" placeholder="Value">' +
        '<button type="button" class="oe-rm" data-act="rmfact" data-i="' + i + '" aria-label="Remove ' + esc(p.label || 'this fact') + '">×</button>';
    }).join('');
    var hidden = /data-secret/.test(d.text);
    return '<div class="hc-pane" role="tabpanel">' +
      '<label class="oe-f"><span class="hc-k">Name</span><input class="oe-big" data-k="name" maxlength="100" autocomplete="off" value="' + esc(d.name) + '" placeholder="' + esc(f.label) + ' name"></label>' +
      '<label class="oe-f"><span class="hc-k">One line for the list</span><input data-k="summary" maxlength="300" autocomplete="off" value="' + esc(d.summary) + '" placeholder="What a player reads before opening it"></label>' +
      '<div class="oe-f"><span class="hc-k" id="oe-text-k">The whole entry</span><div class="oe-tool">' +
        '<button type="button" data-act="bold" title="Bold"><b>B</b></button><button type="button" data-act="italic" title="Italic"><i>I</i></button>' +
        '<button type="button" data-act="secret">Hide from players</button></div>' +
        '<div class="oe-text" contenteditable="true" role="textbox" aria-multiline="true" aria-labelledby="oe-text-k" data-k="text" data-placeholder="Write it as you would a page.">' + d.text + '</div>' +
        '<span class="oe-hint" data-hint>' + (hidden ? '<b>Part of this entry is hidden from players.</b> Directors still read it.'
          : 'Select a passage and press “Hide from players” to keep it for Directors.') + '</span></div>' +
      '<div class="oe-f"><span class="hc-k">Facts</span>' +
        (d.facts.length ? '<div class="oe-facts">' + facts + '</div>'
          : '<span class="oe-hint">' + esc(this.system) + '’s ' + esc(plural(f.label)) + ' carry no facts, so none are suggested.</span>') +
        '<div><button type="button" class="hc-link" data-act="addfact">+ Add a fact</button></div></div>' +
      '<fieldset class="oe-f oe-who"><legend class="hc-k">Who sees it</legend>' +
        '<label><input type="radio" name="oe-who" value="everyone"' + (d.who !== 'directors' ? ' checked' : '') + '><span>Everyone</span>' +
          '<small>Players can pick it in the hero creator and on their sheet.</small></label>' +
        '<label><input type="radio" name="oe-who" value="directors"' + (d.who === 'directors' ? ' checked' : '') + '><span>Directors only</span>' +
          '<small>For ' + an(f.label) + ' ' + esc(lower(f.label)) + ' the party hasn’t discovered yet. Switch it to Everyone when they do.</small></label></fieldset>' +
      '<p class="hc-error" role="alert" hidden data-error></p></div>';
  };

  Entries.prototype.previewHTML = function (d) {
    var f = this.cur();
    var facts = d.facts.filter(function (p) { return p.label.trim() && p.value.trim(); });
    // The server strips secret passages for players; the preview does the
    // same on a detached copy so nothing written runs here.
    var box = tidy(d.text);
    box.querySelectorAll('[data-secret]').forEach(function (s) { s.remove(); });
    var text = box.textContent.trim() ? box.innerHTML : '';
    return '<div class="hc-pane" role="tabpanel"><p class="oe-note">What a player sees when they open it in the hero creator' +
      (d.who === 'directors' ? ' (once you switch it to Everyone)' : '') + '.</p>' +
      '<div class="hc-kind"><span class="paper-kind">' + esc(f.label) + ' · this campaign</span><span class="hc-seal">Campaign</span></div>' +
      '<h2 class="hc-name">' + esc(d.name.trim() || 'Unnamed') + '</h2>' + (d.summary.trim() ? '<p class="hc-lede">' + esc(d.summary) + '</p>' : '') +
      (facts.length ? '<div class="hc-facts">' + facts.map(function (p) {
        return '<div><b>' + esc(p.value) + '</b><span class="hc-k">' + esc(p.label) + '</span></div>';
      }).join('') + '</div>' : '') +
      '<div class="hc-text">' + (text || '<p>Nothing written yet.</p>') + '</div></div>';
  };

  Entries.prototype.fillLeaf = function () {
    var leaf = this.root.querySelector('.hc-leaf'), d = this.draft, isNew = !d.id, f = this.cur();
    var body = this.confirm === 'discard'
      ? '<div class="hc-pane"><div class="oe-confirm" role="alertdialog" aria-label="Unsaved changes">Put ' + (d.name.trim() ? '<b>' + esc(d.name) + '</b>' : 'this entry') +
        ' away without saving? What you changed is lost.' +
        '<div class="oe-confirm__row"><button type="button" class="hc-btn oe-btn--danger" data-act="discard">Don’t save</button>' +
        '<button type="button" class="hc-btn hc-btn--ghost" data-act="nodel">Keep writing</button></div></div></div>'
      : this.confirm
      ? '<div class="hc-pane"><div class="oe-confirm" role="alertdialog" aria-label="Delete ' + esc(d.name) + '">Delete <b>' + esc(d.name) + '</b>? ' +
        'It leaves the pick lists for everyone. Heroes who picked it keep the name on their sheet.' +
        '<div class="oe-confirm__row"><button type="button" class="hc-btn oe-btn--danger" data-act="dodel">Delete</button>' +
        '<button type="button" class="hc-btn hc-btn--ghost" data-act="nodel">Keep it</button></div>' +
        '<p class="hc-error" role="alert" hidden data-error></p></div></div>'
      : (this.tab === 'edit' ? this.writeHTML(d) : this.previewHTML(d));
    leaf.innerHTML = '<button type="button" class="hc-leaf__x" data-act="close" aria-label="Close">×</button>' +
      '<div class="hc-ribbons" role="tablist">' +
        '<button type="button" role="tab" data-tab="edit" aria-selected="' + (this.tab === 'edit') + '">' + (isNew ? 'Write' : 'Edit') + '</button>' +
        '<button type="button" role="tab" data-tab="preview" aria-selected="' + (this.tab === 'preview') + '">Preview</button></div>' +
      '<div class="hc-leaf__body">' + body + '</div>' +
      '<div class="hc-leaf__foot">' + (isNew ? '' : '<button type="button" class="hc-link oe-del" data-act="del">Delete</button>') +
        '<span class="hc-foot__sp"></span><button type="button" class="hc-btn hc-btn--ghost" data-act="close">Cancel</button>' +
        '<button type="button" class="hc-btn" data-act="save"' + (d.name.trim() && !this.busy ? '' : ' disabled') + '>' +
        (isNew ? 'Add ' + esc(lower(f.label)) : 'Save') + '</button></div>';
  };

  Entries.prototype.openLeaf = function (e) {
    this.draft = draftOf(e, this.cur());
    this.saved = JSON.parse(JSON.stringify(this.draft));
    this.open = e ? String(e.id) : 'new';
    this.tab = 'edit';
    this.confirm = false;
    var self = this, wrap = this.root.querySelector('.hc-wrap'), leaf = this.root.querySelector('.hc-leaf');
    this.root.querySelectorAll('.hc-row').forEach(function (r) { r.setAttribute('aria-expanded', String(r.getAttribute('data-open') === self.open)); });
    this.fillLeaf();
    leaf.setAttribute('aria-hidden', 'false');
    // As in the creator: where the leaf covers the page it rises where the
    // reader has scrolled to.
    if (leaf.offsetWidth >= wrap.offsetWidth - 1) {
      var view = scrollerRect(wrap);
      leaf.style.setProperty('--hc-sheet-h', Math.max(320, view.height - 24) + 'px');
      var top = Math.min(wrap.offsetHeight - leaf.offsetHeight, view.top + 12 - wrap.getBoundingClientRect().top);
      leaf.style.setProperty('--hc-sheet-top', Math.max(0, top) + 'px');
    } else {
      leaf.style.removeProperty('--hc-sheet-top');
      leaf.style.removeProperty('--hc-sheet-h');
    }
    requestAnimationFrame(function () { wrap.classList.add('is-open'); leaf.classList.add('is-open'); });
    var n = leaf.querySelector('[data-k="name"]');
    if (n) n.focus({ preventScroll: true });
  };

  Entries.prototype.closeLeaf = function (force) {
    if (this.open === null) return;
    // Unsaved writing is never thrown away on a stray click: the leaf asks first.
    if (!force && this.isDirty()) {
      if (this.tab === 'edit' && !this.confirm) this.syncText();
      this.confirm = 'discard';
      this.fillLeaf();
      var keep = this.root.querySelector('[data-act="nodel"]');
      if (keep) keep.focus();
      return;
    }
    var was = this.open, wrap = this.root.querySelector('.hc-wrap'), leaf = this.root.querySelector('.hc-leaf');
    this.open = null;
    this.draft = null;
    this.confirm = false;
    dirty(false);
    wrap.classList.remove('is-open');
    leaf.classList.remove('is-open');
    leaf.setAttribute('aria-hidden', 'true');
    this.root.querySelectorAll('.hc-row').forEach(function (r) { r.setAttribute('aria-expanded', 'false'); });
    var back = this.root.querySelector('.hc-row[data-open="' + was + '"]') || this.root.querySelector('[data-act="add"]');
    if (back) back.focus({ preventScroll: true });
  };

  Entries.prototype.isDirty = function () { return !!this.draft && !sameDraft(this.draft, this.saved); };
  Entries.prototype.touched = function () {
    dirty(this.isDirty());
    var sv = this.root.querySelector('[data-act="save"]');
    if (sv) sv.disabled = !this.draft.name.trim() || this.busy;
  };

  // Redraws the leaf after a change that adds or removes fields, keeping the
  // scroll place and skipping the pane's entrance.
  Entries.prototype.redraw = function () {
    var body = this.root.querySelector('.hc-leaf__body'), top = body ? body.scrollTop : 0;
    this.fillLeaf();
    body = this.root.querySelector('.hc-leaf__body');
    if (body) { body.scrollTop = top; var pane = body.querySelector('.hc-pane'); if (pane) pane.style.animation = 'none'; }
  };

  Entries.prototype.syncText = function () {
    var box = this.root.querySelector('.oe-text');
    if (!box || !this.draft) return;
    this.draft.text = (box.textContent.trim() || box.querySelector('li')) ? box.innerHTML : '';
    var hint = this.root.querySelector('[data-hint]');
    if (hint) {
      hint.innerHTML = box.querySelector('[data-secret]') ? '<b>Part of this entry is hidden from players.</b> Directors still read it.'
        : 'Select a passage and press “Hide from players” to keep it for Directors.';
    }
  };

  // Wraps the selection in a secret span, or unwraps it when the selection
  // already sits inside one, so the button works both ways.
  Entries.prototype.secret = function () {
    var box = this.root.querySelector('.oe-text'), sel = window.getSelection();
    if (!box || !sel.rangeCount) return;
    var range = sel.getRangeAt(0);
    if (!box.contains(range.commonAncestorContainer)) { notify('Select a passage in the entry first.', 'info'); return; }
    var n = range.commonAncestorContainer, inside = null;
    for (; n && n !== box; n = n.parentNode) if (n.nodeType === 1 && n.hasAttribute('data-secret')) { inside = n; break; }
    if (inside) {
      inside.replaceWith.apply(inside, Array.prototype.slice.call(inside.childNodes));
      this.syncText(); this.touched(); this.announce('Passage shown to players again');
      return;
    }
    if (range.collapsed) { notify('Select a passage in the entry first.', 'info'); return; }
    var span = document.createElement('span');
    span.setAttribute('data-secret', 'true');
    span.appendChild(range.extractContents());
    range.insertNode(span);
    sel.removeAllRanges();
    this.syncText(); this.touched(); this.announce('Passage hidden from players');
  };

  Entries.prototype.announce = function (msg) {
    var live = this.root.querySelector('.hc-live');
    if (!live) { live = document.createElement('div'); live.className = 'hc-live sr-only'; live.setAttribute('aria-live', 'polite'); this.root.appendChild(live); }
    live.textContent = msg;
  };

  Entries.prototype.fail = function (msg) {
    this.busy = false;
    var err = this.root.querySelector('.hc-leaf [data-error]');
    if (err) { err.textContent = msg; err.hidden = false; err.scrollIntoView({ block: 'nearest' }); }
    this.touched();
  };

  Entries.prototype.save = function () {
    var self = this, d = this.draft, f = this.cur();
    if (this.busy || !d.name.trim()) return;
    if (this.tab === 'edit') this.syncText();
    var props = {}, seen = {};
    for (var i = 0; i < d.facts.length; i++) {
      var p = d.facts[i];
      if (!p.label.trim() || !p.value.trim()) continue;
      // An untouched label keeps the key it came with, so a fact the system
      // spells differently is not renamed by a save.
      var k = (p.key && words(p.key) === p.label) ? p.key : keyOf(p.label);
      if (seen[k]) { if (this.tab !== 'edit') { this.tab = 'edit'; this.redraw(); } this.fail('Two facts are both called “' + p.label.trim() + '”. Rename or remove one.'); return; }
      seen[k] = true;
      props[k] = factValue(p.value);
    }
    var text = tidy(d.text);
    var body = { name: d.name.trim(), summary: d.summary.trim(), description: text.textContent.trim() ? text.innerHTML : '',
      properties: props, visibility: d.who === 'directors' ? 'directors' : 'everyone' };
    var isNew = !d.id;
    if (isNew) body.fieldKey = f.key;
    this.busy = true;
    this.touched();
    var err = this.root.querySelector('.hc-leaf [data-error]');
    if (err) err.hidden = true;
    api(isNew ? this.entriesUrl : this.entriesUrl + '/' + encodeURIComponent(d.id), { method: isNew ? 'POST' : 'PUT', body: body, csrfToken: this.csrf })
      .then(json).then(function (res) {
        if (!res.ok || !res.body.entry) { self.fail(res.body.message || res.body.error || 'The entry could not be saved. Try again.'); return; }
        var e = res.body.entry;
        self.entries = self.entries.filter(function (x) { return x.id !== e.id; }).concat([e]);
        self.busy = false;
        self.closeLeaf(true);
        self.render();
        var row = self.root.querySelector('.hc-row[data-open="' + e.id + '"]');
        if (row) row.focus({ preventScroll: true });
        notify(e.name + (isNew ? ' added' : ' saved'));
      }).catch(function () { self.fail('The entry could not be saved. Check your connection and try again.'); });
  };

  Entries.prototype.remove = function () {
    var self = this, d = this.draft;
    if (this.busy || !d.id) return;
    this.busy = true;
    api(this.entriesUrl + '/' + encodeURIComponent(d.id), { method: 'DELETE', csrfToken: this.csrf }).then(function (r) {
      if (!r.ok) return json(r).then(function (res) { self.fail(res.body.message || res.body.error || 'The entry could not be deleted. Try again.'); });
      self.entries = self.entries.filter(function (x) { return x.id !== d.id; });
      self.busy = false;
      self.closeLeaf(true);
      self.render();
      var add = self.root.querySelector('[data-act="add"]');
      if (add) add.focus({ preventScroll: true });
      notify(d.name + ' deleted');
    }).catch(function () { self.fail('The entry could not be deleted. Check your connection and try again.'); });
  };

  Entries.prototype.wire = function () {
    var self = this, root = this.root;
    // The tools act on the writing box's selection, so pressing them must not
    // take focus away from it.
    root.addEventListener('mousedown', function (e) { if (e.target.closest('.oe-tool button')) e.preventDefault(); });
    root.addEventListener('click', function (e) {
      var t = e.target.closest('button');
      if (!t || !root.contains(t)) return;
      if (t.hasAttribute('data-field')) {
        if (self.open !== null) { self.closeLeaf(); if (self.open !== null) return; }
        self.field = +t.getAttribute('data-field');
        self.render();
        var tab = root.querySelector('.oe-fields [data-field="' + self.field + '"]');
        if (tab) tab.focus();
        return;
      }
      if (t.hasAttribute('data-open')) {
        var id = t.getAttribute('data-open');
        if (self.open === id) { self.closeLeaf(); return; }
        if (self.open !== null) { self.closeLeaf(); if (self.open !== null) return; }
        var hit = self.entries.filter(function (x) { return String(x.id) === id; })[0];
        if (hit) self.openLeaf(hit);
        return;
      }
      if (t.hasAttribute('data-tab')) {
        if (self.tab === 'edit') self.syncText();
        self.tab = t.getAttribute('data-tab');
        self.confirm = false;
        self.fillLeaf();
        var rib = root.querySelector('.hc-ribbons [data-tab="' + self.tab + '"]');
        if (rib) rib.focus();
        return;
      }
      switch (t.getAttribute('data-act')) {
        case 'add':
          if (self.open !== null) { self.closeLeaf(); if (self.open !== null) return; }
          self.openLeaf(null);
          break;
        case 'close': self.closeLeaf(); break;
        case 'save': self.save(); break;
        case 'bold': case 'italic':
          document.execCommand(t.getAttribute('data-act'));
          self.syncText(); self.touched();
          break;
        case 'secret': self.secret(); break;
        case 'addfact':
          self.syncText();
          self.draft.facts.push({ label: '', value: '', key: '' });
          self.redraw(); self.touched();
          var ins = root.querySelectorAll('.oe-facts input[data-part="label"]');
          if (ins.length) ins[ins.length - 1].focus();
          break;
        case 'rmfact':
          self.syncText();
          self.draft.facts.splice(+t.getAttribute('data-i'), 1);
          self.redraw(); self.touched();
          break;
        case 'del':
          if (self.tab === 'edit') self.syncText();
          self.confirm = 'delete'; self.fillLeaf();
          var keep = root.querySelector('[data-act="nodel"]');
          if (keep) keep.focus();
          break;
        case 'nodel': self.confirm = false; self.fillLeaf(); break;
        case 'dodel': self.remove(); break;
        case 'discard': self.closeLeaf(true); break;
      }
    });
    root.addEventListener('input', function (e) {
      var el = e.target;
      if (!self.draft) return;
      var k = el.getAttribute && el.getAttribute('data-k');
      if (k === 'text') self.syncText();
      else if (k) self.draft[k] = el.value;
      if (el.hasAttribute && el.hasAttribute('data-fact')) self.draft.facts[+el.getAttribute('data-fact')][el.getAttribute('data-part')] = el.value;
      self.touched();
    });
    root.addEventListener('change', function (e) {
      if (e.target.name === 'oe-who' && self.draft) { self.draft.who = e.target.value; self.touched(); }
    });
    // Pasted or dropped text goes in plain, so formatting from elsewhere (and
    // anything hidden in it) never reaches the entry.
    root.addEventListener('paste', function (e) {
      if (!e.target.closest || !e.target.closest('.oe-text')) return;
      e.preventDefault();
      document.execCommand('insertText', false, (e.clipboardData || window.clipboardData).getData('text/plain'));
    });
    root.addEventListener('drop', function (e) {
      if (e.target.closest && e.target.closest('.oe-text')) e.preventDefault();
    });
    root.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && self.open !== null) {
        e.preventDefault();
        if (self.confirm) { self.confirm = false; self.fillLeaf(); } else self.closeLeaf();
        return;
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && self.open !== null) { e.preventDefault(); self.save(); return; }
      if (e.key === 'Enter' && e.target.tagName === 'INPUT' && e.target.closest('.hc-leaf') && e.target.type !== 'radio') { e.preventDefault(); self.save(); }
    });
    // A click on the page outside the leaf puts it back, unless that would
    // throw away unsaved writing.
    document.addEventListener('click', function (e) {
      if (self.open === null || !document.body.contains(root) || self.isDirty()) return;
      if (e.target.closest('.hc-leaf') || e.target.closest('button')) return;
      if (root.contains(e.target)) self.closeLeaf();
    });
  };

  function boot(scope) {
    var nodes = (scope || document).querySelectorAll('[data-own-entries]:not([data-oe-ready])');
    for (var i = 0; i < nodes.length; i++) {
      nodes[i].setAttribute('data-oe-ready', 'true');
      var p = new Entries(nodes[i]);
      p.wire();
      p.load();
    }
  }

  function start() {
    boot(document);
    document.addEventListener('htmx:afterSettle', function (e) { boot(e.target); });
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
