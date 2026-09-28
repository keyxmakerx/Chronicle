/**
 * sidebar_editor.js -- the owner's edit mode for the campaign sidebar, and a
 * member's own pins
 *
 * The pencil in the sidebar's brand row turns the rows into an editor over
 * one staged draft: drag a row's handle (or focus it and use the arrow keys)
 * to move it, press its pin to move it into or out of Pinned, press its eye
 * to hide it from players, add and name sections, edit links. A "What
 * players see" card follows the draft. Save stores the whole draft with
 * PUT /campaigns/:id/sidebar-config; Cancel throws it away. Nothing is
 * stored before Save.
 *
 * The draft starts from the arrangement only the owner's page carries
 * (#sidebar-nav[data-nav-edit]), which includes rows hidden from players and
 * apps turned off in Extensions. Rows are built with DOM calls, never from
 * markup strings, because labels and link addresses are typed by the owner.
 *
 * Players and scribes have no editor; they pin rows for themselves with the
 * pin on each row, which the server stores for them alone.
 *
 * Motion moves only transform and opacity. A moved row is the real row,
 * lifted: let go, it glides into its new slot, or back into its own when the
 * drop is cancelled; it is never copied. Under reduced motion everything
 * crossfades.
 */
(function () {
  'use strict';

  var FLIP_MS = 320;
  var BACK_MS = 220;
  var EASE = 'cubic-bezier(.2,.8,.2,1)';
  var EASE_OUT = 'cubic-bezier(.16,1,.3,1)';
  var HIST_MAX = 40;
  var DRAG_SLOP = 5;
  var ICON_RE = /^fa-[a-z0-9-]{1,40}$/;

  // --- The draft: pure operations (exported for tests) ---------------------

  /** Where a row goes when it leaves Pinned or a removed section. */
  function homeFor(kind) {
    return kind === 'category' ? 'categories' : 'apps';
  }

  /** Which rows a section may hold; the server applies the same rule. */
  function accepts(sectionKind, rowKind) {
    if (sectionKind === 'apps') return rowKind === 'app' || rowKind === 'link';
    if (sectionKind === 'categories') return rowKind === 'category';
    return true;
  }

  function clone(o) {
    return JSON.parse(JSON.stringify(o));
  }

  /** The rows a section shows in the editor: apps turned off keep their
   *  place in the draft but are not drawn. */
  function shownRows(sec) {
    return sec.items.filter(function (r) { return !r.off; });
  }

  function sectionById(draft, id) {
    for (var i = 0; i < draft.sections.length; i++) {
      if (draft.sections[i].id === id) return draft.sections[i];
    }
    return null;
  }

  function findRow(draft, key) {
    for (var i = 0; i < draft.sections.length; i++) {
      var items = draft.sections[i].items;
      for (var j = 0; j < items.length; j++) {
        if (items[j].key === key) return { sec: draft.sections[i], index: j, row: items[j] };
      }
    }
    return null;
  }

  /**
   * Moves a row into section toId, before the shown row at shownIndex
   * (counted with the row already taken out), or to the end when shownIndex
   * is past the last. Returns false when the section cannot hold the row.
   */
  function moveRowTo(draft, key, toId, shownIndex) {
    var loc = findRow(draft, key), to = sectionById(draft, toId);
    if (!loc || !to || !accepts(to.kind, loc.row.kind)) return false;
    loc.sec.items.splice(loc.index, 1);
    var shown = shownRows(to);
    var at = shownIndex < shown.length ? to.items.indexOf(shown[shownIndex]) : to.items.length;
    to.items.splice(at, 0, loc.row);
    return true;
  }

  /**
   * Moves a row one shown place up (dir -1) or down (+1); at either end of
   * its section it crosses into the neighbouring section that can hold it.
   * Returns the section it lands in, or null when it cannot move.
   */
  function stepRow(draft, key, dir) {
    var loc = findRow(draft, key);
    if (!loc) return null;
    var cur = loc.sec, shown = shownRows(cur), i = shown.indexOf(loc.row);
    var conts = draft.sections.filter(function (s) { return accepts(s.kind, loc.row.kind); });
    var ci = conts.indexOf(cur);
    if (dir < 0) {
      if (i > 0) { moveRowTo(draft, key, cur.id, i - 1); return cur; }
      if (ci > 0) { moveRowTo(draft, key, conts[ci - 1].id, Infinity); return conts[ci - 1]; }
    } else {
      if (i < shown.length - 1) { moveRowTo(draft, key, cur.id, i + 1); return cur; }
      if (ci >= 0 && ci < conts.length - 1) { moveRowTo(draft, key, conts[ci + 1].id, 0); return conts[ci + 1]; }
    }
    return null;
  }

  /** Pins a row (to the end of Pinned) or unpins it (to the top of its
   *  home section). Returns the section it lands in. */
  function togglePin(draft, key) {
    var loc = findRow(draft, key);
    if (!loc) return null;
    var toId = loc.sec.id === 'pinned' ? homeFor(loc.row.kind) : 'pinned';
    if (!moveRowTo(draft, key, toId, toId === 'pinned' ? Infinity : 0)) return null;
    return sectionById(draft, toId);
  }

  /** Removes one of the owner's sections; its rows go back to their homes. */
  function removeSection(draft, id) {
    for (var i = 0; i < draft.sections.length; i++) {
      var sec = draft.sections[i];
      if (sec.id !== id || sec.kind !== 'custom') continue;
      draft.sections.splice(i, 1);
      sec.items.forEach(function (r) { sectionById(draft, homeFor(r.kind)).items.push(r); });
      return true;
    }
    return false;
  }

  function uniqueId(draft, prefix) {
    var taken = {};
    draft.sections.forEach(function (s) {
      taken[s.id] = true;
      s.items.forEach(function (r) { taken[r.key.slice(r.key.indexOf(':') + 1)] = true; });
    });
    for (;;) {
      var id = prefix + Math.random().toString(36).slice(2, 10);
      if (!taken[id]) return id;
    }
  }

  /** Adds a section at the end and returns it. */
  function addSection(draft) {
    var sec = { id: uniqueId(draft, 'sec_'), kind: 'custom', label: 'New section', items: [] };
    draft.sections.push(sec);
    return sec;
  }

  /** Adds an empty link at the end of Apps and returns it. */
  function addLink(draft) {
    var row = { key: 'link:' + uniqueId(draft, 'lnk_'), kind: 'link', label: '', url: '', icon: 'fa-link' };
    sectionById(draft, 'apps').items.push(row);
    return row;
  }

  /** The first link the server would refuse for a missing name or address. */
  function incompleteLink(draft) {
    for (var i = 0; i < draft.sections.length; i++) {
      var items = draft.sections[i].items;
      for (var j = 0; j < items.length; j++) {
        var r = items[j];
        if (r.kind === 'link' && (!String(r.label || '').trim() || !String(r.url || '').trim())) return r;
      }
    }
    return null;
  }

  /**
   * The draft as sidebar_config items, in order: each of the owner's
   * sections is written as a section item ahead of its rows, and every row
   * names its section. Apps turned off are written too, so they keep their
   * place for when they are turned back on. Every section is visible: a
   * heading the old editor hid never reaches the draft (NormalizeNav leaves
   * it out), so saving drops it instead of showing it.
   */
  function itemsFromDraft(draft) {
    var out = [];
    draft.sections.forEach(function (sec) {
      if (sec.kind === 'custom') {
        out.push({ type: 'section', id: sec.id, label: String(sec.label || '').trim() || 'Untitled', visible: true });
      }
      sec.items.forEach(function (r) {
        var ref = r.key.slice(r.key.indexOf(':') + 1), it;
        if (r.kind === 'app') it = { type: 'app', slug: ref };
        else if (r.kind === 'category') it = { type: 'category', type_id: parseInt(ref, 10) };
        else if (r.kind === 'link') {
          it = { type: 'link', id: ref, label: String(r.label || '').trim(), url: String(r.url || '').trim(), icon: r.icon || '' };
        } else return;
        it.visible = !r.hidden;
        it.section = sec.id;
        out.push(it);
      });
    });
    return out;
  }

  /** What players will see: every section's rows that are neither hidden
   *  from players nor turned off, leaving out sections with none. */
  function playerView(draft) {
    var out = [];
    draft.sections.forEach(function (sec) {
      var rows = sec.items.filter(function (r) { return !r.hidden && !r.off; });
      if (rows.length) out.push({ id: sec.id, label: sec.label, rows: rows });
    });
    return out;
  }

  /** Reads the owner's arrangement from the page, or null for anyone else. */
  function readModel(navEl) {
    var raw = navEl && navEl.getAttribute('data-nav-edit');
    if (!raw) return null;
    var m;
    try { m = JSON.parse(raw); } catch (e) { return null; }
    if (!m || !Array.isArray(m.sections) || !sectionById(m, 'pinned') || !sectionById(m, 'apps') || !sectionById(m, 'categories')) return null;
    m.sections.forEach(function (s) { if (!Array.isArray(s.items)) s.items = []; });
    if (!Array.isArray(m.off)) m.off = [];
    return m;
  }

  // --- Page helpers ----------------------------------------------------------

  var S = null; // the edit session: { saved, draft, hist, armed, saving }
  var D = null; // a drag in progress

  function $$(sel, root) {
    return Array.prototype.slice.call((root || document).querySelectorAll(sel));
  }

  function reduced() {
    var q = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
    return !!(q && q.matches) || document.documentElement.classList.contains('nav-rm');
  }

  // Element.animate, or nothing where it is missing: every caller copes with null.
  function play(node, frames, opts) {
    return node && node.animate ? node.animate(frames, opts) : null;
  }

  function whenDone(anim, fn) {
    if (!anim) { fn(); return; }
    anim.finished.then(fn, fn);
  }

  function nav() { return document.getElementById('sidebar-nav'); }
  function viewList() { return document.getElementById('sidebar-nav-list'); }
  function editList() { return document.getElementById('sidebar-nav-edit'); }
  // campaignNavTop (Dashboard, Pinned, Apps) is a separate list from the
  // categories area's. The editor draws one combined list in place of BOTH:
  // it hides this one outright — .nav-cat-zone is flex:1 next to it, so it
  // grows to cover the whole freed height once nav-top disappears — and
  // inserts its edit list where viewList() normally goes.
  function topList() { return document.getElementById('sidebar-nav-top-list'); }
  function sidebarEl() { return document.getElementById('sidebar'); }

  function el(tag, cls, attrs, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (attrs) Object.keys(attrs).forEach(function (k) { n.setAttribute(k, attrs[k]); });
    if (text != null) n.textContent = text;
    return n;
  }

  function faIcon(name, cls) {
    var i = el('i', 'fa-solid ' + (ICON_RE.test(name) ? name : 'fa-circle') + (cls ? ' ' + cls : ''));
    i.setAttribute('aria-hidden', 'true');
    return i;
  }

  function announce(msg) {
    var live = document.getElementById('nav-live');
    if (!live) {
      live = el('div', 'sr-only', { id: 'nav-live', 'aria-live': 'polite' });
      document.body.appendChild(live);
    }
    live.textContent = '';
    setTimeout(function () { live.textContent = msg; }, 30);
  }

  function notify(msg, type) {
    if (window.Chronicle && Chronicle.notify) Chronicle.notify(msg, type);
  }

  function changed() {
    return !!S && JSON.stringify(S.draft) !== JSON.stringify(S.saved);
  }

  function nameOf(row) {
    return row.kind === 'link' ? (String(row.label || '').trim() || 'New link') : row.label;
  }

  // --- Drawing the editor ----------------------------------------------------

  function rowIcon(row) {
    var ic = el('span', 'nav-ic');
    if (row.kind === 'category' && !ICON_RE.test(row.icon || '')) {
      var dot = el('span', 'nav-dot');
      dot.style.backgroundColor = row.color || '';
      ic.appendChild(dot);
    } else {
      var i = faIcon(ICON_RE.test(row.icon || '') ? row.icon : (row.kind === 'link' ? 'fa-link' : 'fa-circle'));
      if (row.color) i.style.color = row.color;
      ic.appendChild(i);
    }
    return ic;
  }

  function handle(key, name) {
    var h = el('button', 'nav-hd', {
      type: 'button', 'data-handle': '', 'data-k': key, 'data-fk': 'h:' + key,
      'aria-label': 'Move ' + name, 'aria-describedby': 'nav-hint-move'
    });
    h.appendChild(faIcon('fa-grip-vertical'));
    return h;
  }

  function pinButton(row, inPinned) {
    var n = nameOf(row), label = inPinned ? 'Unpin ' + n : 'Pin ' + n + ' to the top';
    var b = el('button', 'nav-eb nav-pin', {
      type: 'button', 'data-act': 'pin', 'data-k': row.key, 'data-fk': 'p:' + row.key,
      'aria-pressed': String(inPinned), 'aria-label': label, title: label
    });
    b.appendChild(faIcon('fa-thumbtack'));
    return b;
  }

  // Hiding a row changes only players' sidebar; the page still opens from a
  // link. The eye and every hidden row say so, and the editor points to a
  // page's visibility for real privacy.
  var HIDDEN_NOTE = "Hidden from players' sidebar. It still opens from a link.";
  var PRIVACY_HINT = "Hiding changes only players' sidebar. To keep a page private, set its visibility on the page.";

  function eyeButton(row) {
    var n = nameOf(row);
    var label = row.hidden ? 'Show ' + n + " in players' sidebar" : 'Hide ' + n + " from players' sidebar";
    var b = el('button', 'nav-eb nav-eye', {
      type: 'button', 'data-act': 'hide', 'data-k': row.key, 'data-fk': 'e:' + row.key,
      'aria-pressed': String(!!row.hidden), 'aria-label': label,
      title: row.hidden ? "Show in players' sidebar" : "Hide from players' sidebar. It still opens from a link."
    });
    b.appendChild(faIcon(row.hidden ? 'fa-eye-slash' : 'fa-eye'));
    return b;
  }

  function hiddenNote() {
    return el('span', 'nav-ed-note', null, HIDDEN_NOTE);
  }

  function editRow(row, sec) {
    var r = el('div', 'nav-row nav-ed' + (row.hidden ? ' is-hidden has-note' : '') + (row.kind === 'link' ? ' nav-ed-link' : ''), {
      'data-drag': row.key, 'data-flip': 'n:' + row.key
    });
    r.appendChild(handle(row.key, nameOf(row)));
    r.appendChild(rowIcon(row));
    if (row.kind !== 'link') {
      if (row.hidden) {
        var text = el('span', 'nav-ed-text');
        text.appendChild(el('span', 'nav-lb', null, row.label));
        text.appendChild(hiddenNote());
        r.appendChild(text);
      } else {
        r.appendChild(el('span', 'nav-lb', null, row.label));
      }
      r.appendChild(pinButton(row, sec.id === 'pinned'));
      r.appendChild(eyeButton(row));
      return r;
    }
    // A link is named on its first line and addressed on its second.
    var name = el('input', 'nav-li', {
      type: 'text', maxlength: '100', placeholder: 'Link name', 'aria-label': 'Link name',
      'data-link-label': row.key, 'data-fk': 'l:' + row.key
    });
    name.value = row.label || '';
    r.appendChild(name);
    r.appendChild(pinButton(row, sec.id === 'pinned'));
    r.appendChild(eyeButton(row));
    var line = el('span', 'nav-ed-url');
    var url = el('input', 'nav-li nav-li-url', {
      type: 'text', maxlength: '2048', placeholder: 'https://… or /campaigns/…', 'aria-label': 'Link address',
      'data-link-url': row.key, 'data-fk': 'u:' + row.key, spellcheck: 'false', autocomplete: 'off'
    });
    url.value = row.url || '';
    line.appendChild(url);
    var x = el('button', 'nav-eb', {
      type: 'button', 'data-act': 'dellink', 'data-k': row.key,
      'aria-label': 'Remove ' + nameOf(row), title: 'Remove link'
    });
    x.appendChild(faIcon('fa-xmark'));
    line.appendChild(x);
    r.appendChild(line);
    if (row.hidden) r.appendChild(hiddenNote());
    return r;
  }

  function editHeading(sec) {
    var h = el('div', 'nav-gh nav-gh-ed', { 'data-flip': 'g:' + sec.id });
    if (sec.kind === 'custom') {
      var input = el('input', 'nav-gi', {
        type: 'text', maxlength: '100', 'aria-label': 'Name of this section',
        'data-rename': sec.id, 'data-fk': 'r:' + sec.id
      });
      input.value = sec.label || '';
      h.appendChild(input);
    } else {
      var l = el('span', 'nav-gh-l');
      if (sec.kind === 'pinned') l.appendChild(faIcon('fa-thumbtack'));
      l.appendChild(document.createTextNode(sec.label));
      h.appendChild(l);
    }
    h.appendChild(el('span', 'nav-gh-n', null, String(shownRows(sec).length)));
    if (sec.kind === 'custom') {
      var x = el('button', 'nav-eb', {
        type: 'button', 'data-act': 'delsec', 'data-sec': sec.id,
        'aria-label': 'Remove the ' + (sec.label || 'Untitled') + ' section; its rows move back', title: 'Remove section'
      });
      x.appendChild(faIcon('fa-xmark'));
      h.appendChild(x);
    }
    return h;
  }

  function addButton(act, label, iconName) {
    var b = el('button', 'nav-addg', { type: 'button', 'data-act': act, 'data-fk': 'a:' + act, 'data-flip': 'a:' + act });
    b.appendChild(faIcon(iconName));
    b.appendChild(document.createTextNode(label));
    return b;
  }

  function offTray(model) {
    if (!model.off.length) return null;
    var tray = el('div', 'nav-off', { 'data-flip': 'off' });
    tray.appendChild(el('div', 'nav-off-h', null, 'Turned off in Extensions'));
    model.off.forEach(function (row) {
      var r = el('div', 'nav-row');
      r.appendChild(rowIcon(row));
      r.appendChild(el('span', 'nav-lb', null, row.label));
      tray.appendChild(r);
    });
    var base = nav() ? nav().getAttribute('data-nav-base') : '';
    var go = el('a', 'nav-off-go', { href: base + '/extensions', 'hx-boost': 'false' }, 'Turn apps on in Extensions');
    go.appendChild(faIcon('fa-arrow-right'));
    tray.appendChild(go);
    return tray;
  }

  /** Builds the editor's list from the draft. */
  function buildEditList() {
    var list = el('div', 'nav-list nav-edit-list', { id: 'sidebar-nav-edit' });
    S.draft.sections.forEach(function (sec) {
      var wrap = el('div', 'nav-sec', { 'data-nav-section': sec.id });
      wrap.appendChild(editHeading(sec));
      var body = el('div', 'nav-gb', { 'data-drop': sec.id });
      var rows = shownRows(sec);
      rows.forEach(function (row) { body.appendChild(editRow(row, sec)); });
      if (!rows.length) {
        body.appendChild(el('div', 'nav-empty', null, sec.kind === 'pinned' ? 'Drag rows here, or press a pin' : 'Drag rows here'));
      }
      wrap.appendChild(body);
      list.appendChild(wrap);
    });
    var anyHidden = S.draft.sections.some(function (sec) {
      return shownRows(sec).some(function (r) { return r.hidden; });
    });
    if (anyHidden) {
      var hint = el('div', 'nav-ed-hint', { 'data-flip': 'hint' });
      hint.appendChild(faIcon('fa-circle-info'));
      hint.appendChild(el('span', null, null, PRIVACY_HINT));
      list.appendChild(hint);
    }
    list.appendChild(addButton('newsec', 'New section', 'fa-plus'));
    list.appendChild(addButton('newlink', 'New link', 'fa-link'));
    var tray = offTray(S.saved);
    if (tray) list.appendChild(tray);
    return list;
  }

  // --- The save bar and the player preview -----------------------------------

  function chromeHost() {
    return document.getElementById('app-main') || document.body;
  }

  function button(act, cls, label, iconName) {
    var b = el('button', 'nav-btn ' + cls, { type: 'button', 'data-act': act });
    if (iconName) b.appendChild(faIcon(iconName));
    b.appendChild(document.createTextNode(label));
    return b;
  }

  function ensureChrome() {
    if (!document.getElementById('nav-hint-move')) {
      document.body.appendChild(el('span', null, { id: 'nav-hint-move', hidden: '' }, 'Arrow keys move it. Escape cancels a drag.'));
    }
    var host = chromeHost();
    var bar = el('div', 'nav-ebar', { id: 'nav-ebar', role: 'toolbar', 'aria-label': 'Sidebar changes' });
    var t = el('div', 'nav-ebar-t');
    t.appendChild(el('b', null, null, 'Editing navigation'));
    t.appendChild(el('span', null, { 'data-ebar-status': '' }));
    bar.appendChild(t);
    bar.appendChild(button('undo', 'nav-btn-gho', 'Undo', 'fa-rotate-left'));
    bar.appendChild(button('cancel', 'nav-btn-gho', 'Cancel'));
    bar.appendChild(button('save', 'nav-btn-pri', 'Save', 'fa-check'));
    host.appendChild(bar);
    host.appendChild(el('div', 'nav-pv', { id: 'nav-pv', 'aria-live': 'off' }));
  }

  function removeChrome() {
    ['nav-ebar', 'nav-pv'].forEach(function (id) {
      var n = document.getElementById(id);
      if (n) n.remove();
    });
  }

  /** Refreshes the save bar's state and the player preview from the draft. */
  function renderChrome() {
    var bar = document.getElementById('nav-ebar'), pv = document.getElementById('nav-pv');
    if (!S || !bar || !pv) return;
    var status = bar.querySelector('[data-ebar-status]');
    status.textContent = S.saving ? 'Saving…' : (changed() ? 'Players see this once you save.' : 'Drag a handle, press a pin or an eye.');
    bar.querySelector('[data-act="undo"]').disabled = !S.hist.length || !!S.saving;
    bar.querySelector('[data-act="cancel"]').disabled = !!S.saving;
    bar.querySelector('[data-act="save"]').disabled = !!S.saving;

    pv.textContent = '';
    var h = el('div', 'nav-pv-h');
    h.appendChild(faIcon('fa-eye'));
    h.appendChild(document.createTextNode('What players see'));
    pv.appendChild(h);
    pv.appendChild(el('div', 'nav-pv-s', null, 'Updates as you edit. Hidden rows and Manage are left out.'));
    playerView(S.draft).forEach(function (sec) {
      pv.appendChild(el('div', 'nav-pv-sec', null, sec.label || 'Untitled'));
      sec.rows.forEach(function (row) {
        var it = el('div', 'nav-pv-it');
        var i = faIcon(ICON_RE.test(row.icon || '') ? row.icon : (row.kind === 'link' ? 'fa-link' : 'fa-circle'));
        if (row.color) i.style.color = row.color;
        it.appendChild(i);
        it.appendChild(el('span', null, null, nameOf(row)));
        pv.appendChild(it);
      });
    });
    // Sit below the top bar, clear of the save bar.
    var host = chromeHost(), top = host.firstElementChild ? host.firstElementChild.offsetHeight + 12 : 12;
    pv.style.top = top + 'px';
    pv.style.maxHeight = Math.max(120, host.clientHeight - top - 84) + 'px';
  }

  // --- Moving between the sidebar and the editor -----------------------------

  /** The key an element keeps across a redraw, in the sidebar and the editor
   *  alike: a row by its key, a heading by its section. */
  function flipKey(node) {
    var k = node.getAttribute('data-flip');
    if (k) return k;
    if (node.hasAttribute('data-nav-key')) return 'n:' + node.getAttribute('data-nav-key');
    if (node.classList.contains('nav-gh')) {
      var sec = node.closest('[data-nav-section]');
      return sec ? 'g:' + sec.getAttribute('data-nav-section') : null;
    }
    return null;
  }

  var FLIP_SEL = '[data-flip], [data-nav-key], .nav-gh';

  /** Where every keyed element in root is on screen now. */
  function snapshot(root) {
    var out = {};
    if (!root) return out;
    $$(FLIP_SEL, root).forEach(function (node) {
      if (node.offsetParent === null) return;
      var k = flipKey(node);
      if (k && !out[k]) out[k] = node.getBoundingClientRect();
    });
    return out;
  }

  /**
   * Plays every keyed element in root from where it was (prev) to where it
   * is now, drawn whole at its final size from the first frame. Newcomers
   * fade in. The row the owner moved (o.lift) travels on its own raised
   * surface. Reduced motion crossfades the list instead.
   */
  function playFlip(root, prev, o) {
    o = o || {};
    if (!root) return;
    if (reduced()) {
      play(root, [{ opacity: 0.35 }, { opacity: 1 }], { duration: 180, easing: 'linear' });
      return;
    }
    $$(FLIP_SEL, root).forEach(function (node) {
      if (node.offsetParent === null) return;
      var k = flipKey(node);
      if (!k) return;
      var p = prev[k], n = node.getBoundingClientRect();
      if (!p) {
        play(node, [{ opacity: 0, transform: 'translateY(-6px)' }, { opacity: 1, transform: 'none' }],
          { duration: FLIP_MS, delay: 80, easing: EASE_OUT, fill: 'backwards' });
        return;
      }
      var dx = p.left - n.left, dy = p.top - n.top;
      if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5) return;
      var a = play(node, [{ transform: 'translate(' + dx + 'px,' + dy + 'px)' }, { transform: 'none' }],
        { duration: FLIP_MS, easing: EASE });
      if (a && o.lift === k) {
        node.classList.add('nav-lifted');
        whenDone(a, function () { node.classList.remove('nav-lifted'); });
      }
    });
  }

  function focusByKey(fk) {
    if (!fk) return;
    var root = editList();
    var node = root && root.querySelector('[data-fk="' + fk.replace(/["\\]/g, '\\$&') + '"]');
    if (!node) return;
    node.focus({ preventScroll: true });
    if (node.scrollIntoView) node.scrollIntoView({ block: 'nearest' });
  }

  /** Redraws the editor from the draft, playing everything from where it was. */
  function redraw(o) {
    o = o || {};
    var old = editList();
    if (!old) return;
    var fk = o.focus || (document.activeElement && old.contains(document.activeElement) ? document.activeElement.getAttribute('data-fk') : null);
    var prev = snapshot(old);
    if (o.prev) Object.keys(o.prev).forEach(function (k) { prev[k] = o.prev[k]; });
    var fresh = buildEditList();
    old.parentNode.replaceChild(fresh, old);
    playFlip(fresh, prev, o);
    focusByKey(fk);
    renderChrome();
  }

  /** Applies one change to the draft as an undoable step. */
  function change(mutate, o) {
    o = o || {};
    var before = clone(S.draft);
    if (mutate(S.draft) === false) return;
    S.hist.push(before);
    if (S.hist.length > HIST_MAX) S.hist.shift();
    redraw(o);
    if (o.msg) announce(o.msg);
  }

  function setPencil(on) {
    $$('[data-sidebar-edit-toggle]').forEach(function (b) {
      var label = on ? 'Save and stop editing' : 'Edit the sidebar';
      b.setAttribute('aria-pressed', String(on));
      b.setAttribute('aria-label', label);
      b.setAttribute('title', label);
      var i = b.querySelector('i');
      if (i) i.className = 'fa-solid ' + (on ? 'fa-check' : 'fa-pencil');
    });
  }

  function setEditing(on) {
    document.body.classList.toggle('nav-editing', on);
    // Keeps an unpinned sidebar from folding to icons while it is edited.
    window.dispatchEvent(new CustomEvent('chronicle:nav-editing', { detail: on }));
    setPencil(on);
  }

  function enter() {
    if (S) return;
    var list = viewList(), model = readModel(nav());
    if (!list || !model) return;
    S = { saved: model, draft: clone(model), hist: [], armed: null, saving: false };
    // Pinned and Apps rows live in campaignNavTop, not this list — snapshot
    // the whole sidebar so their FLIP into the combined edit list still
    // plays from where they really were.
    var prev = snapshot(sidebarEl());
    setEditing(true);
    var ed = buildEditList();
    list.parentNode.insertBefore(ed, list.nextSibling);
    list.hidden = true;
    var top = topList();
    if (top) top.hidden = true;
    ensureChrome();
    renderChrome();
    playFlip(ed, prev);
    if (!reduced()) {
      $$('.nav-hd, .nav-eb', ed).forEach(function (b, i) {
        play(b, [{ opacity: 0, transform: 'translateX(-6px) scale(.9)' }, { opacity: 1, transform: 'none' }],
          { duration: 240, delay: Math.min(i, 24) * 12, easing: EASE_OUT, fill: 'backwards' });
      });
      // The bar is centred with a transform everywhere but a phone.
      var phone = window.matchMedia && window.matchMedia('(max-width: 767px)').matches;
      var x = phone ? '' : 'translateX(-50%) ';
      play(document.getElementById('nav-ebar'), [{ opacity: 0, transform: x + 'translateY(18px)' }, { opacity: 1, transform: x + 'translateY(0)' }],
        { duration: 300, easing: EASE_OUT });
    } else {
      play(document.getElementById('nav-ebar'), [{ opacity: 0 }, { opacity: 1 }], { duration: 180 });
    }
    play(document.getElementById('nav-pv'), [{ opacity: 0 }, { opacity: 1 }], { duration: 220, delay: 60, fill: 'backwards' });
    var wide = window.matchMedia && window.matchMedia('(min-width: 901px)').matches;
    announce('Editing navigation. Drag a handle, or focus it and use the arrow keys.' +
      (wide ? ' Watch the preview on the right.' : '') + ' Save when you are done.');
  }

  /** Leaves edit mode for the sidebar list (the one saved, or the one that
   *  was there), playing the rows back into it. */
  function leave() {
    var ed = editList(), list = viewList();
    var prev = snapshot(ed);
    if (ed) ed.remove();
    if (list) list.hidden = false;
    var top = topList();
    if (top) top.hidden = false;
    S = null;
    removeChrome();
    setEditing(false);
    // Rows land back in either part, so the FLIP scans the whole sidebar.
    playFlip(sidebarEl(), prev);
    // The ring comes back once the rows have landed.
    var ring = sidebarEl() && sidebarEl().querySelector('.nav-ring');
    if (ring) play(ring, [{ opacity: 0 }, { opacity: 1 }], { duration: 220, delay: reduced() ? 0 : 220, fill: 'backwards' });
    var pencil = document.querySelector('[data-sidebar-edit-toggle]');
    if (pencil) pencil.focus({ preventScroll: true });
  }

  function cancel() {
    if (!S || S.saving) return;
    leave();
    notify('Changes discarded.', 'info');
  }

  /**
   * Swaps in the sidebar the server now draws for this page, with the
   * owner's new arrangement and the command palette's rows. The list stays
   * hidden until leave() plays the rows into it.
   */
  function refreshSidebar() {
    return fetch(window.location.href, { credentials: 'same-origin', headers: { Accept: 'text/html' } })
      .then(function (res) {
        if (!res.ok) throw new Error('refresh failed');
        return res.text();
      })
      .then(function (html) {
        var doc = new DOMParser().parseFromString(html, 'text/html');
        var fresh = doc.getElementById('sidebar-nav-list'), list = viewList();
        if (!fresh || !list) throw new Error('refresh failed');
        var node = document.importNode(fresh, true);
        node.hidden = true;
        list.parentNode.replaceChild(node, list);
        // campaignNavTop (Pinned, Apps) is a separate list the owner's new
        // arrangement can also have changed — swap it the same way, hidden
        // until leave()/togglePersonalPin play it back in.
        var freshTop = doc.getElementById('sidebar-nav-top-list'), top = topList();
        if (freshTop && top) {
          var topNode = document.importNode(freshTop, true);
          topNode.hidden = true;
          top.parentNode.replaceChild(topNode, top);
          if (window.htmx && htmx.process) htmx.process(topNode);
        }
        var freshNav = doc.getElementById('sidebar-nav'), n = nav();
        if (n) {
          var edit = freshNav && freshNav.getAttribute('data-nav-edit');
          if (edit) n.setAttribute('data-nav-edit', edit); else n.removeAttribute('data-nav-edit');
        }
        var freshSb = doc.getElementById('sidebar'), sb = document.getElementById('sidebar');
        if (freshSb && sb && freshSb.hasAttribute('data-nav-commands')) {
          sb.setAttribute('data-nav-commands', freshSb.getAttribute('data-nav-commands'));
        }
        if (window.htmx && htmx.process) htmx.process(node);
      });
  }

  function save() {
    if (!S || S.saving) return;
    if (!changed()) {
      leave();
      return;
    }
    var bad = incompleteLink(S.draft);
    if (bad) {
      notify('Each link needs a name and an address.', 'error');
      focusByKey((String(bad.label || '').trim() ? 'u:' : 'l:') + bad.key);
      return;
    }
    var base = nav() ? nav().getAttribute('data-nav-base') : '';
    S.saving = true;
    renderChrome();
    Chronicle.apiFetch(base + '/sidebar-config', { method: 'PUT', body: { items: itemsFromDraft(S.draft) } })
      .then(function (res) {
        if (res.ok) return true;
        return res.json().catch(function () { return {}; }).then(function (body) {
          throw new Error(body.message || body.error || 'The sidebar could not be saved.');
        });
      }, function () {
        throw new Error('The sidebar could not be saved. Check your connection and try again.');
      })
      .then(function () {
        return refreshSidebar().then(function () {
          leave();
          notify('Saved. Everyone sees this layout now.', 'success');
        }, function () {
          // Saved, but the new sidebar could not be fetched: load the page,
          // without the unsaved-changes prompt, since nothing is unsaved.
          S.saved = S.draft;
          window.location.reload();
        });
      })
      .catch(function (err) {
        if (!S) return;
        S.saving = false;
        renderChrome();
        notify(err.message, 'error');
      });
  }

  function undo() {
    if (!S || !S.hist.length) return;
    S.draft = S.hist.pop();
    redraw();
    announce('Undone.');
  }

  // --- Actions ---------------------------------------------------------------

  function doAct(act, node) {
    var key = node.getAttribute('data-k');
    var loc = key ? findRow(S.draft, key) : null;
    switch (act) {
      case 'save': save(); break;
      case 'cancel': cancel(); break;
      case 'undo': undo(); break;
      case 'pin': {
        if (!loc) return;
        var to = null;
        change(function (d) { to = togglePin(d, key); return !!to; },
          { lift: 'n:' + key, focus: 'p:' + key });
        if (to) announce(nameOf(loc.row) + ' moved to ' + to.label + '.');
        break;
      }
      case 'hide': {
        if (!loc) return;
        var hid = !loc.row.hidden;
        change(function (d) { findRow(d, key).row.hidden = hid; }, {
          focus: 'e:' + key,
          msg: hid ? nameOf(loc.row) + " is hidden from players' sidebar. It still opens from a link; set its visibility to keep it private."
            : nameOf(loc.row) + " is back in players' sidebar."
        });
        break;
      }
      case 'dellink':
        if (!loc) return;
        change(function (d) { var l = findRow(d, key); l.sec.items.splice(l.index, 1); }, { msg: 'Removed.' });
        break;
      case 'delsec': {
        var id = node.getAttribute('data-sec');
        change(function (d) { return removeSection(d, id); }, { msg: 'Removed. Its rows moved back.' });
        break;
      }
      case 'newsec': {
        var sec = null;
        change(function (d) { sec = addSection(d); });
        if (sec) {
          focusByKey('r:' + sec.id);
          var input = document.activeElement;
          if (input && input.select) input.select();
          announce('Added. Type a name, then drag rows into it.');
        }
        break;
      }
      case 'newlink': {
        var row = null;
        change(function (d) { row = addLink(d); });
        if (row) {
          focusByKey('l:' + row.key);
          announce('Link added at the end of Apps. Give it a name and an address.');
        }
        break;
      }
    }
  }

  // --- Dragging a row (pointer events: mouse, pen and touch) -----------------

  function scroller() { return nav(); }

  function onPointerDown(e) {
    if (!S || S.saving || D || e.button !== 0) return;
    var h = e.target.closest && e.target.closest('#sidebar-nav-edit [data-handle]');
    var row = h && h.closest('[data-drag]');
    if (!row) return;
    e.preventDefault();
    h.focus({ preventScroll: true });
    var sc = scroller();
    D = {
      key: row.getAttribute('data-drag'), row: row, h: h, pid: e.pointerId,
      sx: e.clientX, sy: e.clientY, x: e.clientX, y: e.clientY,
      st0: sc ? sc.scrollTop : 0, moved: false, target: null, line: null, raf: 0
    };
    try { h.setPointerCapture(e.pointerId); } catch (err) { /* capture is a nicety */ }
    h.addEventListener('pointermove', onDragMove);
    h.addEventListener('pointerup', onDragEnd);
    h.addEventListener('pointercancel', onDragCancel);
  }

  function beginDrag() {
    D.moved = true;
    // A row still gliding from the last change lands first, so it follows
    // the pointer from here rather than from where it was going.
    if (D.row.getAnimations) D.row.getAnimations().forEach(function (a) { a.finish(); });
    D.row.classList.add('nav-lifted', 'nav-dragged');
    document.body.classList.add('nav-dragging');
    D.line = el('div', 'nav-dropline', { 'aria-hidden': 'true' });
    D.line.hidden = true;
    editList().appendChild(D.line);
    var tick = function () {
      if (!D) return;
      autoScroll();
      D.raf = requestAnimationFrame(tick);
    };
    D.raf = requestAnimationFrame(tick);
  }

  /** Keeps the lifted row under the pointer, in its column. */
  function placeRow() {
    var sc = scroller();
    var dy = (D.y - D.sy) + (sc ? sc.scrollTop - D.st0 : 0);
    D.row.style.transform = 'translateY(' + dy + 'px)' + (reduced() ? '' : ' scale(1.02)');
  }

  function onDragMove(e) {
    if (!D) return;
    D.x = e.clientX;
    D.y = e.clientY;
    if (!D.moved) {
      if (Math.abs(D.x - D.sx) + Math.abs(D.y - D.sy) < DRAG_SLOP) return;
      beginDrag();
    }
    placeRow();
    findDrop();
  }

  /** Finds the section under the pointer that can take the row, and the
   *  place in it, and draws the drop line there. */
  function findDrop() {
    var list = editList(), sb = document.getElementById('sidebar');
    var row = findRow(S.draft, D.key), target = null, zoneRect = null, kids = [];
    var sr = sb ? sb.getBoundingClientRect() : null;
    if (row && (!sr || (D.x >= sr.left && D.x <= sr.right))) {
      $$('[data-drop]', list).some(function (zone) {
        var r = zone.getBoundingClientRect();
        if (D.y < r.top || D.y > r.bottom) return false;
        var sec = sectionById(S.draft, zone.getAttribute('data-drop'));
        if (!sec || !accepts(sec.kind, row.row.kind)) return true;
        kids = $$(':scope > [data-drag]', zone).filter(function (k) { return k !== D.row; });
        var idx = kids.length;
        for (var i = 0; i < kids.length; i++) {
          var kr = kids[i].getBoundingClientRect();
          if (D.y < kr.top + kr.height / 2) { idx = i; break; }
        }
        target = { sec: sec, index: idx };
        zoneRect = r;
        return true;
      });
    }
    D.target = target;
    if (!target) { D.line.hidden = true; return; }
    var lr = list.getBoundingClientRect();
    var y = kids[target.index] ? kids[target.index].getBoundingClientRect().top
      : (kids.length ? kids[kids.length - 1].getBoundingClientRect().bottom : zoneRect.top + 4);
    D.line.style.width = Math.max(24, zoneRect.width - 12) + 'px';
    D.line.style.transform = 'translate(' + (zoneRect.left - lr.left + 6) + 'px,' + (y - lr.top - 1.5) + 'px)';
    D.line.hidden = false;
  }

  // Near the top or bottom edge of the sidebar, a drag scrolls it.
  function autoScroll() {
    var sc = scroller();
    if (!D || !D.moved || !sc) return;
    var r = sc.getBoundingClientRect(), edge = 36;
    var dy = D.y < r.top + edge ? -8 : (D.y > r.bottom - edge ? 8 : 0);
    if (!dy) return;
    var before = sc.scrollTop;
    sc.scrollTop += dy;
    if (sc.scrollTop !== before) { placeRow(); findDrop(); }
  }

  function endDrag() {
    var d = D;
    D = null;
    cancelAnimationFrame(d.raf);
    d.h.removeEventListener('pointermove', onDragMove);
    d.h.removeEventListener('pointerup', onDragEnd);
    d.h.removeEventListener('pointercancel', onDragCancel);
    try { d.h.releasePointerCapture(d.pid); } catch (err) { /* already released */ }
    document.body.classList.remove('nav-dragging');
    if (d.line) d.line.remove();
    return d;
  }

  /** A row let go with nowhere to land glides back into its own slot. */
  function glideBack(d) {
    var row = d.row, from = row.style.transform;
    row.style.transform = '';
    var land = function () { row.classList.remove('nav-lifted', 'nav-dragged'); };
    if (reduced() || !from) {
      land();
      play(row, [{ opacity: 0.35 }, { opacity: 1 }], { duration: 180, easing: 'linear' });
      return;
    }
    whenDone(play(row, [{ transform: from }, { transform: 'none' }], { duration: BACK_MS, easing: EASE }), land);
  }

  function onDragEnd() {
    if (!D) return;
    var d = endDrag();
    if (!d.moved) return;
    if (!d.target) { glideBack(d); return; }
    var loc = findRow(S.draft, d.key), t = d.target;
    var prev = {};
    prev['n:' + d.key] = d.row.getBoundingClientRect();
    change(function (dr) { return moveRowTo(dr, d.key, t.sec.id, t.index); },
      { prev: prev, lift: 'n:' + d.key, focus: 'h:' + d.key });
    announce(nameOf(loc.row) + ' moved to ' + t.sec.label + ', position ' + (t.index + 1) + '.');
  }

  function onDragCancel() {
    if (!D) return;
    var d = endDrag();
    if (d.moved) glideBack(d);
  }

  // --- Keyboard and typing ---------------------------------------------------

  function onKeyDown(e) {
    if (!S) return;
    if (e.key === 'Escape' && D) {
      // Escape puts a dragged row back; edit mode itself stays open.
      e.preventDefault();
      var d = endDrag();
      if (d.moved) glideBack(d);
      return;
    }
    var t = e.target;
    var h = t && t.closest && t.closest('#sidebar-nav-edit [data-handle]');
    if (h && !D && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
      e.preventDefault();
      var key = h.getAttribute('data-k'), dir = e.key === 'ArrowUp' ? -1 : 1, loc = findRow(S.draft, key), to = null;
      change(function (dr) { to = stepRow(dr, key, dir); return !!to; }, { lift: 'n:' + key, focus: 'h:' + key });
      if (to) {
        var pos = shownRows(to).indexOf(findRow(S.draft, key).row) + 1;
        announce(nameOf(loc.row) + ': position ' + pos + ' of ' + shownRows(to).length + ' in ' + to.label + '.');
      }
      return;
    }
    if (e.key === 'Enter' && t && t.matches && t.matches('#sidebar-nav-edit input')) {
      e.preventDefault();
      t.blur();
    }
  }

  // Typing names a section or a link as it goes; the first keystroke after
  // focusing a field is one undo step.
  function onInput(e) {
    if (!S) return;
    var t = e.target;
    if (!t || !t.closest || !t.closest('#sidebar-nav-edit')) return;
    var id = t.getAttribute('data-rename'), lk = t.getAttribute('data-link-label'), lu = t.getAttribute('data-link-url');
    var field = id ? 'r:' + id : (lk ? 'l:' + lk : (lu ? 'u:' + lu : null));
    if (!field) return;
    if (S.armed !== field) {
      S.hist.push(clone(S.draft));
      if (S.hist.length > HIST_MAX) S.hist.shift();
      S.armed = field;
    }
    if (id) {
      var sec = sectionById(S.draft, id);
      if (sec) sec.label = t.value.trim() || 'Untitled';
    } else {
      var loc = findRow(S.draft, lk || lu);
      if (loc) loc.row[lk ? 'label' : 'url'] = t.value;
    }
    renderChrome();
  }

  function onFocusIn(e) {
    if (S && e.target && e.target.matches && !e.target.matches('#sidebar-nav-edit input')) S.armed = null;
  }

  function onClick(e) {
    if (!S) return;
    var t = e.target;
    var act = t && t.closest && t.closest('#sidebar-nav-edit [data-act], #nav-ebar [data-act]');
    if (!act || act.disabled) return;
    e.preventDefault();
    doAct(act.getAttribute('data-act'), act);
  }

  // --- A member's own pins ----------------------------------------------------

  // Players and scribes pin rows for themselves (PUT /campaigns/:id/nav-pins);
  // the server checks every pin against the sidebar they see and stores it
  // for them alone. The row then glides up into Pinned, or back down.
  var pinning = false;

  /** The viewer's own pins, in the order pinned (Pinned lists them so).
   *  Pinned itself lives in campaignNavTop, but a row being pinned FROM
   *  (Apps or a category) can be in either part, so this reads the whole
   *  sidebar, not just one list. */
  function ownPins() {
    return $$('#sidebar [data-nav-pin][aria-pressed="true"]').map(function (b) {
      return b.getAttribute('data-nav-pin');
    });
  }

  function togglePersonalPin(btn) {
    if (pinning || S) return;
    var key = btn.getAttribute('data-nav-pin'), on = btn.getAttribute('aria-pressed') !== 'true';
    var pins = ownPins().filter(function (k) { return k !== key; });
    if (on) pins.push(key);
    var label = btn.parentNode.querySelector('.nav-lb');
    var name = label ? label.textContent : '';
    var base = nav() ? nav().getAttribute('data-nav-base') : '';
    pinning = true;
    Chronicle.apiFetch(base + '/nav-pins', { method: 'PUT', body: { pins: pins } })
      .then(function (res) {
        if (res.ok) return true;
        return res.json().catch(function () { return {}; }).then(function (body) {
          throw new Error(body.message || body.error || 'That row could not be pinned.');
        });
      }, function () {
        throw new Error('That row could not be pinned. Check your connection and try again.');
      })
      .then(function () {
        // The row can land in campaignNavTop (Pinned) from either part, so
        // the FLIP snapshot and its replay both span the whole sidebar.
        var prev = snapshot(sidebarEl());
        return refreshSidebar().then(function () {
          var list = viewList(), top = topList();
          list.hidden = false;
          if (top) top.hidden = false;
          playFlip(sidebarEl(), prev, { lift: 'n:' + key });
          var again = sidebarEl().querySelector('[data-nav-pin="' + key.replace(/["\\]/g, '\\$&') + '"]');
          if (again) again.focus({ preventScroll: true });
          announce(on ? name + ' is pinned to the top of your sidebar.' : name + ' is unpinned.');
        }, function () {
          window.location.reload();
        });
      })
      .catch(function (err) { notify(err.message, 'error'); })
      .then(function () { pinning = false; });
  }

  document.addEventListener('click', function (e) {
    var btn = e.target && e.target.closest && e.target.closest('#sidebar [data-nav-pin]');
    if (!btn) return;
    e.preventDefault();
    togglePersonalPin(btn);
  });

  // Back and Forward swap in a whole new body, with the sidebar as the server
  // draws it (or, were htmx's history cache on, as it was left, perhaps
  // mid-edit). The editor follows what is live now: while editing, it
  // redraws from the draft; otherwise it clears any editor the body holds.
  document.addEventListener('htmx:historyRestore', function () {
    var ed = editList(), list = viewList(), top = topList();
    removeChrome();
    if (!S) {
      if (ed) ed.remove();
      if (list) list.hidden = false;
      if (top) top.hidden = false;
      setPencil(false);
      return;
    }
    if (!list) return;
    var fresh = buildEditList();
    if (ed) ed.parentNode.replaceChild(fresh, ed);
    else list.parentNode.insertBefore(fresh, list.nextSibling);
    list.hidden = true;
    if (top) top.hidden = true;
    ensureChrome();
    renderChrome();
    // After Alpine has set up the restored sidebar, so it stays open.
    setTimeout(function () { if (S) setEditing(true); }, 0);
  });

  // --- Wiring ----------------------------------------------------------------

  // The pencil: opens the editor, or saves and closes it.
  document.addEventListener('chronicle:toggle-sidebar-editor', function () {
    if (S) save(); else enter();
  });
  document.addEventListener('click', onClick);
  document.addEventListener('pointerdown', onPointerDown);
  document.addEventListener('keydown', onKeyDown);
  document.addEventListener('input', onInput);
  document.addEventListener('focusin', onFocusIn);
  window.addEventListener('beforeunload', function (e) {
    if (changed()) {
      e.preventDefault();
      e.returnValue = '';
    }
  });

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.navEditor = {
    accepts: accepts,
    homeFor: homeFor,
    moveRowTo: moveRowTo,
    stepRow: stepRow,
    togglePin: togglePin,
    removeSection: removeSection,
    addSection: addSection,
    addLink: addLink,
    incompleteLink: incompleteLink,
    itemsFromDraft: itemsFromDraft,
    playerView: playerView,
    readModel: readModel
  };
})();
