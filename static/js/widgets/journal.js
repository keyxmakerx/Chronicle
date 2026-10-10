/**
 * journal.js -- the Journal workspace.
 *
 * Three panes: a resizable list of the viewer's notes (search with filter
 * tokens, saved views, grouping, a hover peek, bulk actions), the note being
 * edited (rich text with [[links]], @mentions and checklists; sharing, pin,
 * archive, the shared-note edit lock, autosave) and a details drawer (links,
 * backlinks, history, audio). The markup is journal.templ, the list rules are
 * journal_list.js and the links are editor_notelink.js.
 *
 * Everything shown comes from the notes JSON routes, which decide who sees
 * what; the browser never does. Live updates arrive over the campaign
 * WebSocket as ids only and are fetched again.
 *
 * Mount: <div class="jnl" data-widget="journal" data-campaign-id data-user-id
 *             data-gm data-note-id>
 */
(function () {
  'use strict';

  var AUTOSAVE_DELAY = 1500; // ms; notes.js mirrors it (test/js/notes_autosave.test.mjs)
  var HEARTBEAT_MS = 120000; // the server drops an edit lock after 5 minutes without one
  var LOCK_FRESH_MS = 5 * 60000;
  var LOCK_IDLE_MS = 3 * 60000;  // no edits for this long hands the lock back
  var PEEK_REST_MS = 300;
  var CHUNK = 60;
  var UNDO_MS = 6000;
  var BACKLINK_CAP = 25;
  var SIZES = ['collapsed', 'compact', 'normal', 'wide'];
  var COMPACT_PX = 210;
  var WIDE_PX = 600;
  var MIN_EDITOR_PX = 260;
  var ID_RE = /^[0-9a-fA-F-]{8,64}$/;
  var SIZE_KEY = 'chronicle.journal.listSize';

  var JL = null; // Chronicle.JournalList, resolved when the first Journal mounts

  // --- Small helpers ---------------------------------------------------------

  function esc(s) { return Chronicle.escapeHtml(s == null ? '' : String(s)); }
  function attr(s) { return Chronicle.escapeAttr(s == null ? '' : String(s)); }
  function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }
  function plural(n, one, many) { return n.toLocaleString() + ' ' + (n === 1 ? one : (many || one + 's')); }
  function media(q) { try { return window.matchMedia(q).matches; } catch (e) { return false; } }
  function reduced() { return media('(prefers-reduced-motion: reduce)'); }
  function isPhone() { return media('(max-width: 760px)'); }
  function canHover() { return media('(hover: hover) and (pointer: fine)'); }
  function load(key, fallback) {
    try {
      var v = localStorage.getItem(key);
      return v == null ? fallback : JSON.parse(v);
    } catch (e) { return fallback; }
  }
  function store(key, value) {
    try { localStorage.setItem(key, JSON.stringify(value)); } catch (e) { /* private window: not remembered */ }
  }
  function json(r) {
    if (!r.ok) {
      var err = new Error('HTTP ' + r.status);
      err.status = r.status;
      throw err;
    }
    return r.json();
  }
  // Wraps the first case-insensitive match of q in a soft mark.
  function mark(text, q) {
    if (!q) return esc(text);
    var i = text.toLowerCase().indexOf(q.toLowerCase());
    if (i === -1) return esc(text);
    return esc(text.slice(0, i)) + '<mark class="nl-mark">' + esc(text.slice(i, i + q.length)) + '</mark>' + esc(text.slice(i + q.length));
  }
  function lockFresh(at) {
    var t = Date.parse(at || '');
    return !isNaN(t) && Date.now() - t < LOCK_FRESH_MS;
  }

  // --- The widget ------------------------------------------------------------

  function Journal(el, config) {
    this.el = el;
    this.cid = String(config.campaignId || '');
    this.uid = String(config.userId || '');
    this.isGM = config.gm === true;
    this.deepId = ID_RE.test(String(config.noteId || '')) ? String(config.noteId) : '';
    this.prefix = 'chronicle.journal.' + this.cid + '.';

    this.notes = [];      // index rows of notes (no folders)
    this.byId = {};       // id -> index row, notes and folders
    this.folders = {};    // id -> {title, parentId, userId, visibility}
    this.members = {};    // userId -> display name
    this.memberList = []; // [{id, name, role}]
    this.indexAt = 0;

    var prefs = load(this.prefix + 'prefs', {}) || {};
    this.view = {
      query: '', scope: 'title', tabFilters: [], fieldFilters: [], filters: [],
      groupBy: 'folder', sortBy: 'updated', sortDir: 'desc', archiveView: false,
      density: prefs.density === 'comfortable' ? 'comfortable' : 'compact',
      collapsed: prefs.collapsed && typeof prefs.collapsed === 'object' ? prefs.collapsed : {},
      tab: 'all',
      views: Array.isArray(prefs.views) ? prefs.views : []
    };
    this.contentHits = null;
    this.selected = {};
    this.anchorId = null;
    this.items = [];
    this.result = null;
    this.rendered = 0;
    this.pendingDelete = null;

    this.active = null;     // the full note being edited
    this.editor = null;
    this.wiki = null;
    this.mention = null;
    this.dirtyTitle = false;
    this.dirtyBody = false;
    this.saving = false;
    this.lastSaveAt = 0;
    this.readOnly = false;
    this.lockHeld = null;
    this.openSeq = 0;
    this.drawerFor = null;
    this.backlinks = [];     // notes linking to the open note
    this.pageBacklinks = []; // pages linking to it

    this.size = 'normal';
    this.lastOpenSize = 'normal';
    this.timers = {};
    this.ws = null;
    this.dead = false;
  }

  /** The notes API path for this campaign. */
  Journal.prototype.api = function (suffix) {
    return '/campaigns/' + encodeURIComponent(this.cid) + '/notes' + (suffix || '');
  };

  Journal.prototype.q = function (sel) { return this.el.querySelector(sel); };

  Journal.prototype.start = function () {
    var self = this;
    JL = Chronicle.JournalList;
    var el = this.el;
    this.dom = {
      list: this.q('.jnl-list'),
      field: this.q('.nl-search-field'),
      scope: this.q('.nl-scope'),
      viewBtn: this.q('.nl-viewbtn'),
      newCaret: this.q('.nl-newcaret'),
      newMenu: this.q('.nl-newmenu'),
      tabs: this.q('.nl-tabs'),
      scroll: this.q('.nl-scroll'),
      inner: this.q('.nl-inner'),
      pill: this.q('.nl-bulkpill'),
      foot: this.q('.nl-foot'),
      handle: this.q('.tp-handle'),
      cycle: this.q('.tp-cycle'),
      blank: this.q('.jnl-blank'),
      note: this.q('.jnl-note'),
      title: this.q('.ed-title'),
      archiveBtn: this.q('[data-act="archive"]'),
      pinBtn: this.q('[data-act="pin"]'),
      drawerBtn: this.q('[data-act="drawer"]'),
      badge: this.q('.dt-badge'),
      chips: this.q('.vis-control'),
      owner: this.q('.ed-owner'),
      lock: this.q('.ed-lock'),
      toolbar: this.q('.ed-toolbar'),
      body: this.q('.ed-body'),
      legacy: this.q('.ed-legacy'),
      status: this.q('.ed-status'),
      updated: this.q('.ed-updated'),
      audioInput: this.q('[data-audio-input]'),
      drawer: this.q('.jnl-drawer'),
      out: this.q('[data-list="out"]'),
      back: this.q('[data-list="back"]'),
      history: this.q('[data-list="history"]'),
      audio: this.q('[data-list="audio"]'),
      fold: this.q('.nl-fold'),
      suggest: this.q('.nl-suggest'),
      peek: this.q('.nl-peek'),
      toasts: this.q('.jnl-toasts')
    };
    if (!JL || !this.dom.list) {
      console.error('[Journal] journal_list.js or the Journal markup is missing.');
      return;
    }

    this.bindList();
    this.bindSearch();
    this.bindEditor();
    this.bindDrawer();
    this.bindResize();
    this.bindGlobal();
    this.renderTabs();
    this.updatePill();

    Chronicle.openJournalNote = function (id) { return self.openFromOutside(id); };

    Promise.all([this.loadMembers(), this.loadIndex()]).then(function () {
      if (self.dead) return;
      self.renderList();
      self.applyLinksParam();
      var want = self.deepId || load(self.prefix + 'last', '');
      if (want) self.open(want, { quiet: !self.deepId, keepPane: !self.deepId });
    });
    this.connect();
  };

  /**
   * ?links=<id> opens the list narrowed to notes linking to a page or note
   * (the Jot panel's "see them in the Journal"). The token is labelled from
   * what the viewer's own list already shows, never looked up.
   */
  Journal.prototype.applyLinksParam = function () {
    var id = '';
    try { id = new URLSearchParams(window.location.search).get('links') || ''; } catch (e) { return; }
    if (!ID_RE.test(id)) return;
    var label = '';
    var row = this.byId[id];
    if (row && !row.isFolder) label = row.title;
    for (var i = 0; !label && i < this.notes.length; i++) {
      var links = this.notes[i].links || [];
      for (var k = 0; k < links.length; k++) {
        if (links[k].kind === 'page' && links[k].id === id && links[k].label) { label = links[k].label; break; }
      }
    }
    this.setFieldTokens([{ field: 'linksTo', value: id, kw: 'links', label: label || 'this page' }]);
    try { window.history.replaceState(window.history.state, '', window.location.pathname); } catch (e) { /* stays */ }
  };

  // --- Data ------------------------------------------------------------------

  Journal.prototype.loadMembers = function () {
    var self = this;
    return Chronicle.apiFetch(this.api('/members')).then(json).then(function (list) {
      self.memberList = (list || []).map(function (m) {
        return { id: m.user_id, name: m.username || 'Someone', role: m.role || '' };
      });
      self.members = {};
      self.memberList.forEach(function (m) { self.members[m.id] = m.name; });
    }).catch(function () { /* names fall back to "Someone" */ });
  };

  /** A member's display name; the viewer reads as "You". */
  Journal.prototype.memberName = function (id) {
    if (id === this.uid) return 'You';
    return this.members[id] || 'Someone';
  };

  Journal.prototype.loadIndex = function () {
    var self = this;
    return Chronicle.apiFetch(this.api('/index')).then(json).then(function (data) {
      self.setIndex((data && data.notes) || []);
      self.loadFailed = false;
    }).catch(function () {
      self.loadFailed = true;
    });
  };

  Journal.prototype.setIndex = function (rows) {
    var notes = [];
    var folders = {};
    var byId = {};
    rows.forEach(function (r) {
      byId[r.id] = r;
      if (r.isFolder) {
        folders[r.id] = { title: r.title, parentId: r.parentId || null, userId: r.userId, visibility: r.visibility };
      } else {
        notes.push(r);
      }
    });
    this.notes = notes;
    this.folders = folders;
    this.byId = byId;
    this.indexAt = Date.now();
    // Every label is asked for again: sharing may have changed since the
    // last load, and a title must not outlive the viewer's access to it.
    if (Chronicle.NoteLabels) {
      Chronicle.NoteLabels.reset(this.cid);
      Chronicle.NoteLabels.prime(this.cid, notes);
    }
    // Selection only ever holds rows that still exist.
    var sel = {};
    for (var id in this.selected) if (byId[id]) sel[id] = true;
    this.selected = sel;
  };

  /** Reloads the list soon, coalescing bursts (live events, saves). */
  Journal.prototype.refreshSoon = function (delay) {
    var self = this;
    clearTimeout(this.timers.refresh);
    this.timers.refresh = setTimeout(function () {
      self.loadIndex().then(function () {
        if (self.dead) return;
        self.renderList();
        self.checkActiveStillVisible();
      });
    }, delay == null ? 400 : delay);
  };

  /** Folds a note the server just returned into the index row for it. */
  Journal.prototype.upsertRow = function (n, extra) {
    if (!n || n.entityId) return;
    var row = this.byId[n.id];
    if (!row) {
      row = { id: n.id, snippet: '', links: [], inCount: 0, hasAudio: false };
      this.byId[n.id] = row;
      if (!n.isFolder) this.notes.push(row);
    }
    row.title = n.title;
    row.parentId = n.parentId || null;
    row.isFolder = !!n.isFolder;
    row.userId = n.userId;
    row.visibility = n.visibility;
    row.sharedWith = n.userId === this.uid ? n.sharedWith : undefined;
    row.pinned = !!n.pinned;
    row.archived = !!n.archived;
    row.createdAt = n.createdAt;
    row.updatedAt = n.updatedAt;
    if (extra) for (var k in extra) row[k] = extra[k];
    if (n.isFolder) {
      this.folders[n.id] = { title: n.title, parentId: n.parentId || null, userId: n.userId, visibility: n.visibility };
    } else if (Chronicle.NoteLabels) {
      Chronicle.NoteLabels.prime(this.cid, [row]);
    }
    return row;
  };

  /** Removes rows from the local index (a delete the server confirmed). */
  Journal.prototype.dropRows = function (ids) {
    var self = this;
    var gone = {};
    ids.forEach(function (id) {
      gone[id] = true;
      delete self.byId[id];
      delete self.folders[id];
      delete self.selected[id];
      if (Chronicle.NoteLabels) Chronicle.NoteLabels.forget(self.cid, id);
    });
    this.notes = this.notes.filter(function (n) { return !gone[n.id]; });
  };

  // --- The list ----------------------------------------------------------------

  /** Context for the list engine: names, labels and folders, per viewer. */
  Journal.prototype.listCtx = function () {
    var self = this;
    var withNotes = {};
    this.notes.forEach(function (n) { if (n.parentId && !n.archived) withNotes[n.parentId] = true; });
    var empty = Object.keys(this.folders).filter(function (id) { return !withNotes[id]; });
    return {
      folders: this.folders,
      memberName: function (id) { return self.memberName(id); },
      noteTitle: function (id) {
        var n = self.byId[id];
        return n && !n.isFolder ? n.title : '';
      },
      contentHits: this.view.scope === 'contents' ? this.contentHits : null,
      emptyFolders: empty
    };
  };

  Journal.prototype.visibleNotes = function () {
    var pd = this.pendingDelete;
    if (!pd) return this.notes;
    return this.notes.filter(function (n) { return pd.ids.indexOf(n.id) === -1; });
  };

  /**
   * Recomputes the display list and draws it. Keeps however many rows were
   * already drawn, so a save or a live update never yanks the scroll back.
   */
  Journal.prototype.renderList = function () {
    var v = this.view;
    v.filters = v.tabFilters.concat(v.fieldFilters);
    this.ctx = this.listCtx();
    this.result = JL.compute(this.visibleNotes(), {
      query: v.query, scope: v.scope, filters: v.filters, groupBy: v.groupBy, sortBy: v.sortBy,
      sortDir: v.sortDir, archiveView: v.archiveView, collapsed: v.collapsed
    }, this.ctx);
    this.items = this.result.items;
    this.rendered = Math.min(this.items.length, Math.max(CHUNK, this.rendered));
    this.drawRows();
    this.renderFoot();
    this.updatePill();
  };

  Journal.prototype.drawRows = function () {
    var inner = this.dom.inner;
    this.hidePeek();
    if (this.loadFailed && !this.notes.length) {
      inner.innerHTML = '<div class="nl-empty">The notes didn’t load.<br><button type="button" data-retry>Try again</button></div>';
      return;
    }
    if (!this.items.length) {
      inner.innerHTML = '<div class="nl-empty">' + esc(this.emptyText()) + '</div>';
      return;
    }
    var html = '';
    for (var i = 0; i < this.rendered; i++) html += this.itemHtml(this.items[i]);
    inner.innerHTML = html;
  };

  Journal.prototype.emptyText = function () {
    var v = this.view;
    if (v.query || v.filters.length) return 'No notes match here yet.';
    if (v.archiveView) return 'Nothing is archived.';
    return 'No notes yet. Start one with New.';
  };

  /** Appends the next chunk once the list is scrolled near its end. */
  Journal.prototype.growList = function () {
    if (this.rendered >= this.items.length) return;
    var s = this.dom.scroll;
    if (s.scrollTop + s.clientHeight < s.scrollHeight - 240) return;
    var end = Math.min(this.items.length, this.rendered + CHUNK);
    var html = '';
    for (var i = this.rendered; i < end; i++) html += this.itemHtml(this.items[i]);
    this.dom.inner.insertAdjacentHTML('beforeend', html);
    this.rendered = end;
  };

  Journal.prototype.itemHtml = function (it) {
    return it.kind === 'header' ? this.headHtml(it) : this.rowHtml(it);
  };

  Journal.prototype.gmLabel = function () {
    return this.isGM ? 'GM only' : 'Shared with GM';
  };

  Journal.prototype.visLabel = function (v) {
    return v === 'gm' ? this.gmLabel() : JL.visLabel(v);
  };

  Journal.prototype.rowHtml = function (it) {
    var n = it.note;
    var v = this.view;
    var comfy = v.density === 'comfortable';
    var active = !!(this.active && this.active.id === n.id);
    var sel = !!this.selected[n.id];
    var q = (v.query || '').trim();
    var link = JL.firstLink(n, this.ctx);
    var hint = link ? link.label : JL.folderPath(n.parentId, this.folders);
    if (it.snippet && !comfy) hint = it.snippet;
    var snip = comfy ? (it.snippet || n.snippet || '') : '';
    // Visibility shows only when it is not the everyday "shared with the
    // party": a muted glyph, never a coloured dot.
    var vis = '';
    if (n.visibility === 'private') vis = '<i class="fa-solid fa-eye-slash nl-vis" title="Private" aria-label="Private"></i>';
    else if (n.visibility === 'gm') vis = '<i class="fa-solid fa-shield-halved nl-vis" title="' + attr(this.gmLabel()) + '" aria-label="' + attr(this.gmLabel()) + '"></i>';
    else if (n.visibility === 'custom') vis = '<i class="fa-solid fa-user-group nl-vis" title="Specific people" aria-label="Specific people"></i>';
    var title = n.title || 'Untitled';
    var line1 =
      '<span class="nl-gutter"><span class="nl-circle" data-select></span></span>' + vis +
      (n.pinned ? '<i class="fa-solid fa-thumbtack nl-pin" title="Pinned" aria-label="Pinned"></i>' : '') +
      '<span class="nl-title" title="' + attr(title) + '">' + mark(title, q) + '</span>' +
      (hint ? '<span class="nl-hint">' + esc(hint) + '</span>' : '') +
      '<span class="nl-date" title="' + attr(JL.longDate(n.updatedAt)) + '">' + esc(JL.relDate(n.updatedAt, Date.now())) + '</span>';
    return '<div class="nl-row' + (active ? ' active' : '') + (sel ? ' selected' : '') + (comfy ? ' comfy' : '') +
      '" data-note="' + attr(n.id) + '" role="option" aria-selected="' + sel + '"' + (active ? ' aria-current="true"' : '') + ' tabindex="-1">' +
      (comfy ? '<div class="nl-line1">' + line1 + '</div>' : line1) +
      (snip ? '<div class="nl-snip">' + esc(snip) + '</div>' : '') +
      '</div>';
  };

  Journal.prototype.headHtml = function (it) {
    var collapsed = !!this.view.collapsed[it.key];
    var f = it.folderId ? this.folders[it.folderId] : null;
    var manage = !!(f && f.userId === this.uid);
    return '<div class="nl-ghead' + (collapsed ? ' collapsed' : '') + (it.pinned ? ' pinned' : '') +
      '" data-group="' + attr(it.key) + '" role="presentation" tabindex="-1" aria-expanded="' + !collapsed + '">' +
      '<i class="fa-solid fa-chevron-right chev" aria-hidden="true"></i>' +
      (it.pinned ? '<i class="fa-solid fa-thumbtack" aria-hidden="true" style="font-size:9px"></i>' : '') +
      '<span class="lbl">' + esc(it.label) + '</span>' +
      (manage ? '<button type="button" class="gmenu" data-folder-menu="' + attr(it.folderId) + '" title="Folder options" aria-label="Options for ' + attr(it.label) + '"><i class="fa-solid fa-ellipsis" aria-hidden="true"></i></button>' : '') +
      '<span class="cnt">' + it.count + '</span></div>';
  };

  // One quiet line: "473 notes · 12 archived".
  Journal.prototype.renderFoot = function () {
    var r = this.result;
    if (!r) return;
    var text;
    if (this.view.archiveView) text = plural(r.matched, 'archived note');
    else text = plural(r.matched, 'note') + (r.archivedTotal ? ' · ' + r.archivedTotal.toLocaleString() + ' archived' : '');
    this.dom.foot.textContent = text;
  };

  /** Re-marks the active row without redrawing the list. */
  Journal.prototype.markActiveRow = function () {
    var id = this.active ? this.active.id : '';
    var rows = this.dom.inner.querySelectorAll('.nl-row');
    for (var i = 0; i < rows.length; i++) {
      var on = rows[i].getAttribute('data-note') === id;
      rows[i].classList.toggle('active', on);
      if (on) rows[i].setAttribute('aria-current', 'true'); else rows[i].removeAttribute('aria-current');
    }
  };

  // --- Tabs: All, saved views, Archive ---------------------------------------

  Journal.prototype.renderTabs = function () {
    var v = this.view;
    function tab(id, name, extra) {
      var on = v.tab === id;
      return '<button type="button" role="tab" class="nl-tab' + (on ? ' on' : '') + '" data-tab="' + attr(id) + '" aria-selected="' + on + '">' + esc(name) + (extra || '') + '</button>';
    }
    var html = tab('all', 'All');
    v.views.forEach(function (sv) {
      html += tab(sv.id, sv.name, '<span class="x" data-rm-tab="' + attr(sv.id) + '" role="button" title="Remove this view" aria-label="Remove the view ' + attr(sv.name) + '"><i class="fa-solid fa-xmark" aria-hidden="true"></i></span>');
    });
    html += tab('archive', 'Archive');
    html += '<button type="button" class="nl-tab add" data-tab-add title="Save the current filters as a view" aria-label="Save the current filters as a view">+</button>';
    this.dom.tabs.innerHTML = html;
  };

  Journal.prototype.tabPreset = function (id) {
    if (id === 'all') return { groupBy: 'folder', sortBy: 'updated', sortDir: 'desc', archiveView: false, filters: [] };
    if (id === 'archive') return { groupBy: 'none', sortBy: 'updated', sortDir: 'desc', archiveView: true, filters: [] };
    for (var i = 0; i < this.view.views.length; i++) if (this.view.views[i].id === id) return this.view.views[i];
    return null;
  };

  /**
   * Selecting a tab replaces the grouping, sorting and preset filters. "All"
   * also clears whatever is typed; another tab keeps the typed tokens, so
   * they narrow the view further.
   */
  Journal.prototype.setTab = function (id) {
    var p = this.tabPreset(id);
    if (!p) return;
    var v = this.view;
    v.tab = id;
    v.groupBy = p.groupBy;
    v.sortBy = p.sortBy;
    v.sortDir = p.sortDir;
    v.archiveView = !!p.archiveView;
    v.tabFilters = (p.filters || []).map(function (r) { return { field: r.field, value: r.value, label: r.label }; });
    if (id === 'all') {
      v.fieldFilters = [];
      v.query = '';
      this.dom.field.innerHTML = '';
      this.contentHits = null;
    }
    this.clearSelection(true);
    this.renderTabs();
    this.renderList();
    if (this.foldOpen) this.renderFold();
  };

  Journal.prototype.savePrefs = function () {
    var v = this.view;
    store(this.prefix + 'prefs', { density: v.density, collapsed: v.collapsed, views: v.views });
  };

  Journal.prototype.promptSaveView = function (anchor) {
    var self = this;
    this.openPop(anchor,
      '<label class="pop-label" for="jnl-view-name">View name</label>' +
      '<input type="text" id="jnl-view-name" maxlength="40" placeholder="e.g. GM prep" autocomplete="off">' +
      '<div class="pop-foot"><button type="button" class="jnl-btn ghost" data-pop-cancel>Cancel</button><button type="button" class="jnl-btn" data-pop-ok>Save</button></div>',
      function (pop) {
        var input = pop.querySelector('input');
        function save() {
          var name = input.value.trim();
          if (!name) { input.focus(); return; }
          var v = self.view;
          var id = 'v' + Date.now().toString(36);
          v.views.push({
            id: id, name: name, groupBy: v.groupBy, sortBy: v.sortBy, sortDir: v.sortDir, archiveView: v.archiveView,
            filters: v.tabFilters.concat(v.fieldFilters).map(function (r) { return { field: r.field, value: r.value, label: r.label }; })
          });
          self.savePrefs();
          self.closePop();
          self.setTab(id);
          self.toast('Saved view “' + name + '”');
        }
        pop.querySelector('[data-pop-ok]').addEventListener('click', save);
        pop.querySelector('[data-pop-cancel]').addEventListener('click', function () { self.closePop(true); });
        input.addEventListener('keydown', function (e) { if (e.key === 'Enter') { e.preventDefault(); save(); } });
      });
  };

  Journal.prototype.removeView = function (id) {
    var v = this.view;
    v.views = v.views.filter(function (sv) { return sv.id !== id; });
    this.savePrefs();
    if (v.tab === id) this.setTab('all'); else this.renderTabs();
  };

  // --- Selection and bulk actions ----------------------------------------------

  Journal.prototype.toggleSelect = function (id, range) {
    if (range && this.anchorId) {
      var ids = JL.rangeBetween(this.items, this.anchorId, id);
      if (ids.length) {
        for (var i = 0; i < ids.length; i++) this.selected[ids[i]] = true;
      } else {
        this.selected[id] = true;
        this.anchorId = id;
      }
    } else {
      if (this.selected[id]) delete this.selected[id]; else this.selected[id] = true;
      this.anchorId = id;
    }
    this.syncSelectionMarks();
  };

  Journal.prototype.clearSelection = function (silent) {
    this.selected = {};
    this.anchorId = null;
    if (!silent) this.syncSelectionMarks(); else this.updatePill();
  };

  Journal.prototype.syncSelectionMarks = function () {
    var rows = this.dom.inner.querySelectorAll('.nl-row');
    for (var i = 0; i < rows.length; i++) {
      var on = !!this.selected[rows[i].getAttribute('data-note')];
      rows[i].classList.toggle('selected', on);
      rows[i].setAttribute('aria-selected', String(on));
    }
    this.updatePill();
  };

  Journal.prototype.selectedIds = function () { return Object.keys(this.selected); };

  Journal.prototype.updatePill = function () {
    var n = this.selectedIds().length;
    this.dom.pill.classList.toggle('on', n > 0);
    this.dom.scroll.classList.toggle('selecting', n > 0);
    this.dom.pill.querySelector('.cnt').textContent = n + ' selected';
    var arch = this.dom.pill.querySelector('[data-bulk="archive"]');
    var label = this.view.archiveView ? 'Unarchive' : 'Archive';
    arch.title = label;
    arch.setAttribute('aria-label', label);
  };

  /** Runs one bulk action on the selection; only the viewer's own notes change. */
  Journal.prototype.bulk = function (action, extra) {
    var self = this;
    var ids = this.selectedIds();
    if (!ids.length) return;
    var body = { action: action, ids: ids };
    if (extra) for (var k in extra) body[k] = extra[k];
    Chronicle.apiFetch(this.api('/bulk'), { method: 'POST', body: body }).then(json).then(function (res) {
      var done = (res && res.done) || [];
      var skipped = (res && res.skipped) || [];
      var verb = { archive: 'Archived', unarchive: 'Unarchived', move: 'Moved', visibility: 'Visibility set on' }[action] || 'Changed';
      var msg = verb + ' ' + plural(done.length, 'note');
      if (action === 'move') msg += extra.folderId ? ' to ' + (JL.folderPath(extra.folderId, self.folders) || 'the folder') : ' to the top level';
      if (action === 'visibility') msg += ': ' + self.visLabel(extra.visibility);
      if (skipped.length) msg += ' · ' + skipped.length + ' not yours, left alone';
      self.toast(msg);
      self.clearSelection(true);
      if (self.active && done.indexOf(self.active.id) !== -1) self.reloadActive();
      self.refreshSoon(0);
    }).catch(function () {
      Chronicle.notify('That didn’t work. Nothing was changed.', 'error');
    });
  };

  /**
   * Deletes the selected notes of the viewer's own after a grace period with
   * Undo, instead of asking first. The rows leave the list at once; the
   * server delete lands when the toast goes, or right away if the page is
   * left.
   */
  Journal.prototype.deleteSelected = function () {
    var self = this;
    var ids = this.selectedIds();
    if (!ids.length) return;
    this.commitDelete();
    var own = ids.filter(function (id) { return self.byId[id] && self.byId[id].userId === self.uid; });
    var foreign = ids.length - own.length;
    if (!own.length) {
      this.toast('Only a note’s owner can delete it.');
      return;
    }
    var wasActive = this.active && own.indexOf(this.active.id) !== -1 ? this.active.id : null;
    if (wasActive) this.closeNote();
    var pd = { ids: own };
    pd.timer = setTimeout(function () { self.commitDelete(); }, UNDO_MS);
    this.pendingDelete = pd;
    this.clearSelection(true);
    this.renderList();
    this.toast(plural(own.length, 'note') + ' deleted' + (foreign ? ' · ' + foreign + ' not yours, left alone' : ''), {
      duration: UNDO_MS,
      undo: function () {
        if (self.pendingDelete !== pd) return;
        clearTimeout(pd.timer);
        self.pendingDelete = null;
        self.renderList();
        if (wasActive) self.open(wasActive, { keepPane: true });
      }
    });
  };

  /** Sends a pending delete now. keepalive lets it outlive the page. */
  Journal.prototype.commitDelete = function (keepalive) {
    var pd = this.pendingDelete;
    if (!pd) return;
    clearTimeout(pd.timer);
    this.pendingDelete = null;
    var self = this;
    var body = { action: 'delete', ids: pd.ids };
    if (keepalive) {
      this.beacon(this.api('/bulk'), 'POST', body);
      return;
    }
    Chronicle.apiFetch(this.api('/bulk'), { method: 'POST', body: body }).then(json).then(function (res) {
      if (self.dead) return;
      self.dropRows((res && res.done) || []);
      self.renderList();
      self.refreshSoon();
    }).catch(function () {
      Chronicle.notify('The notes could not be deleted.', 'error');
      if (!self.dead) self.refreshSoon(0);
    });
  };

  /** A same-origin write that survives the page unloading. */
  Journal.prototype.beacon = function (url, method, body) {
    try {
      Chronicle.apiFetch(url, { method: method, keepalive: true, body: body || undefined })
        .catch(function () { /* the page is going; nothing to tell */ });
    } catch (e) { /* ditto */ }
  };

  Journal.prototype.openMovePop = function (anchor) {
    var self = this;
    var paths = Object.keys(this.folders).map(function (id) { return { id: id, path: JL.folderPath(id, self.folders) }; })
      .sort(function (a, b) { return a.path.toLowerCase() < b.path.toLowerCase() ? -1 : 1; });
    var html = '<span class="pop-label">Move to folder</span>' +
      '<button type="button" class="pop-opt" data-move-to=""><i class="fa-solid fa-layer-group" aria-hidden="true"></i>Top level</button>' +
      paths.map(function (p) { return '<button type="button" class="pop-opt" data-move-to="' + attr(p.id) + '"><i class="fa-solid fa-folder" aria-hidden="true"></i>' + esc(p.path) + '</button>'; }).join('') +
      (paths.length ? '' : '<p class="pop-text">No folders yet. Make one with the arrow next to New.</p>');
    this.openPop(anchor, html, function (pop) {
      pop.addEventListener('click', function (e) {
        var b = e.target.closest('[data-move-to]');
        if (!b) return;
        self.closePop();
        self.bulk('move', { folderId: b.getAttribute('data-move-to') });
      });
    });
  };

  Journal.prototype.openBulkVisPop = function (anchor) {
    var self = this;
    var html = '<span class="pop-label">Set visibility</span>' +
      '<button type="button" class="pop-opt" data-vis-to="private"><i class="fa-solid fa-eye-slash" aria-hidden="true"></i>Private</button>' +
      '<button type="button" class="pop-opt" data-vis-to="party"><i class="fa-solid fa-users" aria-hidden="true"></i>Shared with party</button>' +
      '<button type="button" class="pop-opt" data-vis-to="gm"><i class="fa-solid fa-shield-halved" aria-hidden="true"></i>' + esc(this.gmLabel()) + '</button>';
    this.openPop(anchor, html, function (pop) {
      pop.addEventListener('click', function (e) {
        var b = e.target.closest('[data-vis-to]');
        if (!b) return;
        self.closePop();
        self.bulk('visibility', { visibility: b.getAttribute('data-vis-to') });
      });
    });
  };

  // --- New notes and folders -----------------------------------------------------

  /** The folder a new note goes into: the one an in: token narrows to. */
  Journal.prototype.targetFolder = function () {
    var rules = this.view.tabFilters.concat(this.view.fieldFilters);
    for (var i = 0; i < rules.length; i++) {
      if (rules[i].field === 'folder' && rules[i].value && this.folders[rules[i].value]) return rules[i].value;
    }
    return '';
  };

  Journal.prototype.createNote = function (folderId) {
    var self = this;
    this.closeNewMenu();
    var body = { title: 'Untitled note', content: [], visibility: 'private' };
    var parent = folderId != null ? folderId : this.targetFolder();
    if (parent) body.parentId = parent;
    Chronicle.apiFetch(this.api(''), { method: 'POST', body: body }).then(json).then(function (note) {
      self.upsertRow(note);
      if (self.view.archiveView) self.setTab('all');
      else self.renderList();
      return self.open(note.id, { fresh: true });
    }).catch(function () {
      Chronicle.notify('Could not create the note.', 'error');
    });
  };

  Journal.prototype.createFolder = function () {
    var self = this;
    this.closeNewMenu();
    Chronicle.apiFetch(this.api(''), { method: 'POST', body: { title: 'New folder', isFolder: true, content: [], visibility: 'private' } })
      .then(json).then(function (folder) {
        self.upsertRow(folder);
        if (self.view.tab !== 'all' || self.view.groupBy !== 'folder') self.setTab('all');
        else self.renderList();
        var head = self.dom.inner.querySelector('[data-group="' + CSS.escape(folder.id) + '"]');
        if (head) {
          head.scrollIntoView({ block: 'nearest' });
          self.renameFolder(folder.id, head.querySelector('[data-folder-menu]') || head);
        }
        self.toast('Folder added. File notes into it with Move.');
      }).catch(function () {
        Chronicle.notify('Could not create the folder.', 'error');
      });
  };

  Journal.prototype.openFolderMenu = function (id, anchor) {
    var self = this;
    var f = this.folders[id];
    if (!f) return;
    var vis = f.visibility || 'private';
    function opt(act, icon, label, on) {
      return '<button type="button" class="pop-opt" data-folder-act="' + act + '"' + (on ? ' aria-pressed="true"' : '') + '><i class="fa-solid ' + icon + '" aria-hidden="true"></i>' + esc(label) + (on ? ' ✓' : '') + '</button>';
    }
    var html = '<span class="pop-label">' + esc(JL.folderPath(id, this.folders)) + '</span>' +
      opt('new', 'fa-plus', 'New note here') +
      opt('rename', 'fa-pen', 'Rename') +
      '<span class="pop-label" style="margin-top:8px">Who can see the folder</span>' +
      opt('vis-private', 'fa-eye-slash', 'Private', vis === 'private') +
      opt('vis-party', 'fa-users', 'Shared with party', vis === 'party') +
      opt('vis-gm', 'fa-shield-halved', this.gmLabel(), vis === 'gm') +
      '<button type="button" class="pop-opt danger" data-folder-act="delete"><i class="fa-solid fa-trash-can" aria-hidden="true"></i>Delete folder</button>';
    this.openPop(anchor, html, function (pop) {
      pop.addEventListener('click', function (e) {
        var b = e.target.closest('[data-folder-act]');
        if (!b) return;
        var act = b.getAttribute('data-folder-act');
        if (act === 'new') { self.closePop(); self.createNote(id); }
        else if (act === 'rename') self.renameFolder(id, anchor);
        else if (act === 'delete') self.confirmDeleteFolder(id, anchor);
        else if (act.indexOf('vis-') === 0) { self.closePop(); self.setFolderVisibility(id, act.slice(4)); }
      });
    });
  };

  Journal.prototype.renameFolder = function (id, anchor) {
    var self = this;
    var f = this.folders[id];
    if (!f) return;
    this.openPop(anchor,
      '<label class="pop-label" for="jnl-folder-name">Folder name</label>' +
      '<input type="text" id="jnl-folder-name" maxlength="200" autocomplete="off" value="' + attr(f.title) + '">' +
      '<div class="pop-foot"><button type="button" class="jnl-btn ghost" data-pop-cancel>Cancel</button><button type="button" class="jnl-btn" data-pop-ok>Save</button></div>',
      function (pop) {
        var input = pop.querySelector('input');
        setTimeout(function () { input.select(); }, 0);
        function save() {
          var name = input.value.trim();
          if (!name) { input.focus(); return; }
          self.closePop();
          Chronicle.apiFetch(self.api('/' + encodeURIComponent(id)), { method: 'PUT', body: { title: name } }).then(json).then(function (folder) {
            self.upsertRow(folder);
            self.renderList();
          }).catch(function () { Chronicle.notify('Could not rename the folder.', 'error'); });
        }
        pop.querySelector('[data-pop-ok]').addEventListener('click', save);
        pop.querySelector('[data-pop-cancel]').addEventListener('click', function () { self.closePop(true); });
        input.addEventListener('keydown', function (e) { if (e.key === 'Enter') { e.preventDefault(); save(); } });
      });
  };

  Journal.prototype.setFolderVisibility = function (id, vis) {
    var self = this;
    Chronicle.apiFetch(this.api('/' + encodeURIComponent(id)), { method: 'PUT', body: { visibility: vis } }).then(json).then(function (folder) {
      self.upsertRow(folder);
      self.renderList();
      self.toast('Folder: ' + self.visLabel(vis));
    }).catch(function () { Chronicle.notify('Could not change who sees the folder.', 'error'); });
  };

  /**
   * Deleting a folder first moves what is filed in it to the top level, so
   * a folder delete never takes notes with it.
   */
  Journal.prototype.confirmDeleteFolder = function (id, anchor) {
    var self = this;
    var f = this.folders[id];
    if (!f) return;
    this.openPop(anchor,
      '<p class="pop-text">Delete “' + esc(f.title) + '”? What is filed in it moves to the top level.</p>' +
      '<div class="pop-foot"><button type="button" class="jnl-btn ghost" data-pop-cancel>Cancel</button><button type="button" class="jnl-btn danger" data-pop-ok>Delete</button></div>',
      function (pop) {
        pop.querySelector('[data-pop-cancel]').addEventListener('click', function () { self.closePop(true); });
        pop.querySelector('[data-pop-ok]').addEventListener('click', function () {
          self.closePop();
          var children = Object.keys(self.byId).filter(function (cid) {
            var r = self.byId[cid];
            return r.parentId === id && r.userId === self.uid;
          });
          var move = children.length
            ? Chronicle.apiFetch(self.api('/bulk'), { method: 'POST', body: { action: 'move', ids: children, folderId: '' } }).then(json)
            : Promise.resolve();
          move.then(function () {
            return Chronicle.apiFetch(self.api('/' + encodeURIComponent(id)), { method: 'DELETE' }).then(json);
          }).then(function () {
            self.dropRows([id]);
            self.toast('Folder deleted');
            self.refreshSoon(0);
          }).catch(function () {
            Chronicle.notify('Could not delete the folder.', 'error');
            self.refreshSoon(0);
          });
        });
      });
  };

  Journal.prototype.toggleNewMenu = function () {
    if (this.newOpen) this.closeNewMenu(); else this.openNewMenu();
  };
  Journal.prototype.openNewMenu = function () {
    this.closeFold();
    this.closePop();
    this.newOpen = true;
    this.dom.newMenu.classList.add('open');
    this.dom.newCaret.setAttribute('aria-expanded', 'true');
    var first = this.dom.newMenu.querySelector('button');
    if (first) first.focus({ preventScroll: true });
  };
  Journal.prototype.closeNewMenu = function (restore) {
    if (!this.newOpen) return;
    this.newOpen = false;
    this.dom.newMenu.classList.remove('open');
    this.dom.newCaret.setAttribute('aria-expanded', 'false');
    if (restore) this.dom.newCaret.focus();
  };

  // --- List events ---------------------------------------------------------------

  Journal.prototype.bindList = function () {
    var self = this;
    var d = this.dom;

    d.inner.addEventListener('click', function (e) {
      if (e.target.closest('[data-retry]')) { self.loadIndex().then(function () { self.renderList(); }); return; }
      var fm = e.target.closest('[data-folder-menu]');
      if (fm) { e.stopPropagation(); self.openFolderMenu(fm.getAttribute('data-folder-menu'), fm); return; }
      var head = e.target.closest('[data-group]');
      if (head) { self.toggleGroup(head.getAttribute('data-group')); return; }
      var row = e.target.closest('[data-note]');
      if (!row) return;
      var id = row.getAttribute('data-note');
      if (self.suppressClick) { self.suppressClick = false; return; }
      // On touch, once something is selected a tap adds to the selection.
      if (self.lastPointer === 'touch' && self.selectedIds().length) { self.toggleSelect(id, false); return; }
      if (e.target.closest('[data-select]')) { self.toggleSelect(id, e.shiftKey); return; }
      if (e.shiftKey) { self.toggleSelect(id, true); return; }
      if (e.metaKey || e.ctrlKey) { self.toggleSelect(id, false); return; }
      self.open(id);
    });

    // Touch has no hover circle or modifier keys: a long press selects.
    d.inner.addEventListener('pointerdown', function (e) {
      self.lastPointer = e.pointerType;
      clearTimeout(self.timers.press);
      if (e.pointerType !== 'touch') return;
      var row = e.target.closest('.nl-row');
      if (!row) return;
      self.pressAt = { x: e.clientX, y: e.clientY };
      self.timers.press = setTimeout(function () {
        self.suppressClick = true;
        self.toggleSelect(row.getAttribute('data-note'), false);
      }, 500);
    });
    d.inner.addEventListener('pointermove', function (e) {
      if (self.pressAt && Math.abs(e.clientX - self.pressAt.x) + Math.abs(e.clientY - self.pressAt.y) > 10) clearTimeout(self.timers.press);
    });
    d.inner.addEventListener('pointerup', function () { clearTimeout(self.timers.press); self.pressAt = null; });
    d.inner.addEventListener('pointercancel', function () { clearTimeout(self.timers.press); self.pressAt = null; });
    d.inner.addEventListener('contextmenu', function (e) {
      if (self.lastPointer === 'touch' && e.target.closest('.nl-row')) e.preventDefault();
    });

    d.scroll.addEventListener('scroll', function () {
      self.hidePeek();
      self.growList();
    }, { passive: true });

    d.scroll.addEventListener('keydown', function (e) { self.onListKey(e); });

    d.tabs.addEventListener('click', function (e) {
      var rm = e.target.closest('[data-rm-tab]');
      if (rm) { e.stopPropagation(); self.removeView(rm.getAttribute('data-rm-tab')); return; }
      if (e.target.closest('[data-tab-add]')) { self.promptSaveView(e.target.closest('[data-tab-add]')); return; }
      var t = e.target.closest('[data-tab]');
      if (t) self.setTab(t.getAttribute('data-tab'));
    });

    d.pill.addEventListener('click', function (e) {
      var b = e.target.closest('[data-bulk]');
      if (!b) return;
      var act = b.getAttribute('data-bulk');
      if (act === 'clear') self.clearSelection();
      else if (act === 'archive') self.bulk(self.view.archiveView ? 'unarchive' : 'archive');
      else if (act === 'move') self.openMovePop(b);
      else if (act === 'visibility') self.openBulkVisPop(b);
      else if (act === 'delete') self.deleteSelected();
    });

    d.viewBtn.addEventListener('click', function () { if (self.foldOpen) self.closeFold(true); else self.openFold(); });
    d.fold.addEventListener('click', function (e) { self.onFoldClick(e); });
    d.newCaret.addEventListener('click', function () { self.toggleNewMenu(); });
    this.el.addEventListener('click', function (e) {
      var nb = e.target.closest('[data-new]');
      if (!nb) return;
      if (nb.getAttribute('data-new') === 'folder') self.createFolder(); else self.createNote();
    });

    this.bindPeek();
  };

  Journal.prototype.toggleGroup = function (key) {
    var c = this.view.collapsed;
    if (c[key]) delete c[key]; else c[key] = true;
    this.savePrefs();
    this.renderList();
    var head = this.dom.inner.querySelector('[data-group="' + CSS.escape(key) + '"]');
    if (head && document.activeElement === this.dom.scroll) head.classList.add('is-focused');
  };

  /** Arrow keys walk the rows and group headers; Enter opens or folds. */
  Journal.prototype.onListKey = function (e) {
    var items = Array.prototype.slice.call(this.dom.inner.querySelectorAll('.nl-row, .nl-ghead'));
    if (!items.length) return;
    var cur = -1;
    for (var i = 0; i < items.length; i++) if (items[i].classList.contains('is-focused')) { cur = i; break; }
    var next = cur;
    if (e.key === 'ArrowDown') next = Math.min(items.length - 1, cur + 1);
    else if (e.key === 'ArrowUp') next = Math.max(0, cur - 1);
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = items.length - 1;
    else if ((e.key === 'Enter' || e.key === ' ') && cur !== -1) {
      e.preventDefault();
      var it = items[cur];
      if (it.hasAttribute('data-group')) this.toggleGroup(it.getAttribute('data-group'));
      else if (e.key === ' ') this.toggleSelect(it.getAttribute('data-note'), e.shiftKey);
      else this.open(it.getAttribute('data-note'));
      return;
    } else return;
    e.preventDefault();
    if (cur !== -1) items[cur].classList.remove('is-focused');
    items[next].classList.add('is-focused');
    items[next].scrollIntoView({ block: 'nearest' });
    this.growList();
    if (items[next].hasAttribute('data-note') && canHover()) this.showPeek(items[next]);
    else this.hidePeek();
  };

  // --- Hover peek ------------------------------------------------------------------

  /**
   * A glance, never a destination: it shows after a short rest on a row (or
   * at once for keyboard focus), never takes the pointer, and hides the
   * moment the pointer leaves the row. Off on touch.
   */
  Journal.prototype.bindPeek = function () {
    var self = this;
    var inner = this.dom.inner;
    inner.addEventListener('pointerover', function (e) {
      if (e.pointerType && e.pointerType !== 'mouse') return;
      if (!canHover()) return;
      var row = e.target.closest('.nl-row');
      if (!row || row === self.peekRow) return;
      self.hidePeek();
      self.peekRow = row;
      self.timers.peek = setTimeout(function () {
        if (self.peekRow === row && row.isConnected) self.showPeek(row);
      }, reduced() ? 0 : PEEK_REST_MS);
    });
    inner.addEventListener('pointerout', function (e) {
      var row = e.target.closest('.nl-row');
      var to = e.relatedTarget && e.relatedTarget.closest ? e.relatedTarget.closest('.nl-row') : null;
      if (row && row !== to) self.hidePeek();
    });
    inner.addEventListener('pointerdown', function () { self.hidePeek(); });
  };

  Journal.prototype.peekHtml = function (n) {
    var where = JL.folderPath(n.parentId, this.folders) || 'No folder';
    var visIcon = { private: 'fa-eye-slash', gm: 'fa-shield-halved', custom: 'fa-user-group' }[n.visibility] || 'fa-users';
    var out = (n.links || []).length;
    var html = '<div class="p-ttl">' + esc(n.title || 'Untitled') + '</div>' +
      (n.snippet ? '<div class="p-body">' + esc(n.snippet) + '</div>' : '') +
      '<div class="p-rows">' +
      '<div class="p-row"><i class="fa-solid fa-folder" aria-hidden="true"></i><span class="tt">' + esc(where) + '</span></div>' +
      '<div class="p-row"><i class="fa-solid ' + visIcon + '" aria-hidden="true"></i><span class="tt">' + esc(this.visLabel(n.visibility)) + '</span></div>' +
      '<div class="p-row"><i class="fa-solid fa-link" aria-hidden="true"></i><span class="tt">' + out + ' out · ' + (n.inCount || 0) + ' in</span></div>' +
      (n.userId !== this.uid ? '<div class="p-row"><i class="fa-solid fa-user" aria-hidden="true"></i><span class="tt">By ' + esc(this.memberName(n.userId)) + '</span></div>' : '') +
      '<div class="p-row"><i class="fa-solid fa-clock-rotate-left" aria-hidden="true"></i><span class="tt">Updated ' + esc(JL.longDate(n.updatedAt)) + '</span></div>' +
      '</div>';
    return html;
  };

  Journal.prototype.showPeek = function (row) {
    var n = this.byId[row.getAttribute('data-note')];
    if (!n) return;
    var el = this.dom.peek;
    el.innerHTML = this.peekHtml(n);
    var r = row.getBoundingClientRect();
    var w = 264;
    el.style.left = clamp(r.left + 18, 8, window.innerWidth - w - 8) + 'px';
    var top = r.bottom + 6;
    var h = el.offsetHeight || 170;
    if (top + h > window.innerHeight - 8) {
      top = Math.max(8, r.top - h - 6);
      el.style.transformOrigin = 'bottom left';
    } else {
      el.style.transformOrigin = 'top left';
    }
    el.style.top = top + 'px';
    el.classList.add('on');
  };

  Journal.prototype.hidePeek = function () {
    clearTimeout(this.timers.peek);
    this.peekRow = null;
    if (this.dom && this.dom.peek) this.dom.peek.classList.remove('on');
  };

  // --- Search: free text plus in:/by:/links:/is:/has:/before:/after: tokens ---------

  Journal.prototype.bindSearch = function () {
    var self = this;
    var f = this.dom.field;
    f.addEventListener('input', function () { self.onFieldInput(); });
    f.addEventListener('keydown', function (e) { self.onFieldKey(e); });
    f.addEventListener('blur', function () {
      clearTimeout(self.timers.suggestBlur);
      self.timers.suggestBlur = setTimeout(function () { self.hideSuggest(); }, 150);
    });
    // Plain text only: a pasted fragment must not bring markup into the field.
    f.addEventListener('paste', function (e) {
      e.preventDefault();
      var text = ((e.clipboardData && e.clipboardData.getData('text/plain')) || '').replace(/\s+/g, ' ');
      self.insertAtCaret(document.createTextNode(text));
      self.onFieldInput();
    });
    f.addEventListener('drop', function (e) { e.preventDefault(); });
    f.addEventListener('click', function (e) {
      var rm = e.target.closest('[data-tok-rm]');
      if (!rm) return;
      rm.closest('.nl-token').remove();
      self.syncField();
      f.focus();
    });
    this.dom.suggest.addEventListener('mousedown', function (e) {
      var item = e.target.closest('[data-i]');
      if (!item) return;
      e.preventDefault();
      self.acceptSuggest(parseInt(item.getAttribute('data-i'), 10));
    });
    this.dom.scope.addEventListener('click', function () {
      var v = self.view;
      v.scope = v.scope === 'title' ? 'contents' : 'title';
      var b = self.dom.scope;
      b.setAttribute('data-scope', v.scope);
      b.textContent = v.scope === 'title' ? 'Titles' : 'Contents';
      b.title = v.scope === 'title' ? 'Searching titles only — click to also search contents' : 'Searching titles and contents — click for titles only';
      self.searchContents();
      self.renderList();
    });
  };

  Journal.prototype.insertAtCaret = function (node) {
    var f = this.dom.field;
    var sel = window.getSelection();
    var r;
    if (sel && sel.rangeCount && f.contains(sel.getRangeAt(0).startContainer)) {
      r = sel.getRangeAt(0);
      r.deleteContents();
    } else {
      r = document.createRange();
      r.selectNodeContents(f);
      r.collapse(false);
    }
    r.insertNode(node);
    r.setStartAfter(node);
    r.collapse(true);
    sel.removeAllRanges();
    sel.addRange(r);
  };

  Journal.prototype.onFieldInput = function () {
    var trig = this.fieldTrigger();
    if (trig) this.showSuggest(trig); else this.hideSuggest();
    this.syncField();
  };

  Journal.prototype.onFieldKey = function (e) {
    var s = this.suggest;
    if (s && s.open) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        if (s.items.length) {
          s.active = (s.active + (e.key === 'ArrowDown' ? 1 : -1) + s.items.length) % s.items.length;
          this.paintSuggest();
        }
        return;
      }
      if ((e.key === 'Enter' || e.key === 'Tab') && s.items.length) {
        e.preventDefault();
        if (!this.commitDateToken()) this.acceptSuggest(s.active);
        return;
      }
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); this.hideSuggest(); return; }
    }
    if ((e.key === ' ' || e.key === 'Enter') && this.commitDateToken()) { e.preventDefault(); return; }
    if (e.key === 'Enter') e.preventDefault();
    if (e.key === 'ArrowDown' && !(s && s.open)) {
      e.preventDefault();
      this.dom.scroll.focus();
      this.onListKey({ key: 'Home', preventDefault: function () {} });
    }
  };

  /** The token being typed at the caret ("in:ruin"), or null. */
  Journal.prototype.fieldTrigger = function () {
    var sel = window.getSelection();
    if (!sel || !sel.rangeCount) return null;
    var range = sel.getRangeAt(0);
    if (!range.collapsed) return null;
    var node = range.startContainer;
    if (!node || node.nodeType !== 3 || !this.dom.field.contains(node)) return null;
    var text = node.textContent.slice(0, range.startOffset);
    var kws = JL.TOKENS.map(function (t) { return t.kw; }).join('|');
    var m = new RegExp('(?:^|\\s)(' + kws + '):(\\S{0,40})$').exec(text);
    if (!m) return null;
    var r = document.createRange();
    r.setStart(node, range.startOffset - m[1].length - 1 - m[2].length);
    r.setEnd(node, range.startOffset);
    return { range: r, kw: m[1], query: m[2] };
  };

  /** Suggestions for a keyword, each {value, label, sub}. */
  Journal.prototype.tokenOptions = function (kw, query) {
    var self = this;
    var q = (query || '').toLowerCase();
    var out = [];
    function push(value, label, sub) {
      if (!q || String(label).toLowerCase().indexOf(q) !== -1) out.push({ value: value, label: label, sub: sub });
    }
    if (kw === 'in') {
      push('', 'no folder', 'Not in a folder');
      Object.keys(this.folders).map(function (id) { return { id: id, path: JL.folderPath(id, self.folders) }; })
        .sort(function (a, b) { return a.path.toLowerCase() < b.path.toLowerCase() ? -1 : 1; })
        .forEach(function (f) { push(f.id, f.path, 'Folder'); });
    } else if (kw === 'by') {
      var owners = {};
      this.notes.forEach(function (n) { owners[n.userId] = true; });
      owners[this.uid] = true;
      Object.keys(owners).map(function (id) { return { id: id, name: self.memberName(id) }; })
        .sort(function (a, b) { return a.id === self.uid ? -1 : b.id === self.uid ? 1 : a.name.localeCompare(b.name); })
        .forEach(function (o) { push(o.id, o.id === self.uid ? 'me' : o.name, 'Owner'); });
    } else if (kw === 'links') {
      var pages = {};
      this.notes.forEach(function (n) {
        (n.links || []).forEach(function (l) { if (l.kind === 'page' && l.label && !pages[l.id]) pages[l.id] = l.label; });
      });
      Object.keys(pages).sort(function (a, b) { return pages[a].localeCompare(pages[b]); })
        .forEach(function (id) { push(id, pages[id], 'Page'); });
      this.notes.filter(function (n) { return !n.archived; })
        .sort(function (a, b) { return (a.title || '').localeCompare(b.title || ''); })
        .forEach(function (n) { push(n.id, n.title || 'Untitled', 'Note'); });
    } else if (kw === 'is') {
      JL.IS_VALUES.forEach(function (it) { push(it.value, it.value === 'gm' ? (self.isGM ? 'gm only' : 'shared with gm') : it.label, 'Is'); });
    } else if (kw === 'has') {
      push('audio', 'audio', 'Has a recording');
    } else if (kw === 'before' || kw === 'after') {
      var day = new Date();
      day.setHours(0, 0, 0, 0);
      push(day.getTime(), 'today', 'Or type a date, like 2026-09-01');
      push(day.getTime() - 6 * 86400000, 'this week', 'The last 7 days');
      push(day.getTime() - 29 * 86400000, 'this month', 'The last 30 days');
    }
    return out.slice(0, 8);
  };

  Journal.prototype.showSuggest = function (trig) {
    clearTimeout(this.timers.suggestBlur);
    this.suggest = { open: true, range: trig.range, kw: trig.kw, active: 0, items: this.tokenOptions(trig.kw, trig.query) };
    this.paintSuggest();
    var el = this.dom.suggest;
    var rects = trig.range.getClientRects();
    var rect = rects.length ? rects[rects.length - 1] : trig.range.getBoundingClientRect();
    var w = Math.min(260, window.innerWidth * 0.9);
    el.style.left = clamp(rect.left, 8, window.innerWidth - w - 8) + 'px';
    var top = rect.bottom + 6;
    if (top + 240 > window.innerHeight) top = Math.max(8, rect.top - 244);
    el.style.top = top + 'px';
    el.classList.add('open');
  };

  Journal.prototype.paintSuggest = function () {
    var s = this.suggest;
    var kw = s.kw;
    this.dom.suggest.innerHTML = s.items.length
      ? s.items.map(function (it, i) {
        return '<div class="nl-sitem' + (i === s.active ? ' active' : '') + '" role="option" aria-selected="' + (i === s.active) + '" data-i="' + i + '">' +
          '<i class="fa-solid ' + (kw === 'links' ? 'fa-link' : 'fa-filter') + ' nl-ico" aria-hidden="true"></i>' +
          '<span><b>' + esc(kw) + ':</b>' + esc(it.label) + '<small>' + esc(it.sub) + '</small></span></div>';
      }).join('')
      : '<div class="nl-sempty">No matches. Keep typing, or Esc.</div>';
  };

  Journal.prototype.hideSuggest = function () {
    if (this.suggest) this.suggest.open = false;
    this.dom.suggest.classList.remove('open');
  };

  Journal.prototype.tokenNode = function (field, value, kw, label) {
    var span = document.createElement('span');
    span.className = 'nl-token';
    span.contentEditable = 'false';
    span.setAttribute('data-field', field);
    span.setAttribute('data-value', String(value));
    span.setAttribute('data-kw', kw);
    span.innerHTML = '<b>' + esc(kw) + ':</b>' + esc(label) +
      '<button type="button" data-tok-rm tabindex="-1" aria-label="Remove ' + attr(kw + ':' + label) + '"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button>';
    return span;
  };

  /** Swaps the typed trigger for an atomic token and puts the caret after it. */
  Journal.prototype.placeToken = function (range, node) {
    range.deleteContents();
    range.insertNode(node);
    var space = document.createTextNode(' ');
    node.parentNode.insertBefore(space, node.nextSibling);
    var sel = window.getSelection();
    var r = document.createRange();
    r.setStartAfter(space);
    r.collapse(true);
    sel.removeAllRanges();
    sel.addRange(r);
    this.hideSuggest();
    this.syncField();
  };

  Journal.prototype.acceptSuggest = function (i) {
    var s = this.suggest;
    var it = s && s.items[i];
    if (!it || !s.range) { this.hideSuggest(); return; }
    var tok = JL.tokenByKw(s.kw);
    this.placeToken(s.range, this.tokenNode(tok.field, it.value, s.kw, it.label));
  };

  /** before:/after: also take a typed date ("2026-09-01") on space or Enter. */
  Journal.prototype.commitDateToken = function () {
    var trig = this.fieldTrigger();
    if (!trig || (trig.kw !== 'before' && trig.kw !== 'after') || !trig.query) return false;
    var ms = JL.parseDate(trig.query);
    if (ms == null) return false;
    var tok = JL.tokenByKw(trig.kw);
    this.placeToken(trig.range, this.tokenNode(tok.field, ms, trig.kw, JL.shortDate(ms)));
    return true;
  };

  /**
   * Reads the field back: each token becomes a filter rule and the loose
   * text is the query. The one source of truth for typed and inserted tokens.
   */
  Journal.prototype.syncField = function () {
    var rules = [];
    var text = '';
    Array.prototype.forEach.call(this.dom.field.childNodes, function (node) {
      if (node.nodeType === 1 && node.classList.contains('nl-token')) {
        var field = node.getAttribute('data-field');
        var raw = node.getAttribute('data-value');
        var value = field === 'updatedBefore' || field === 'updatedAfter' ? Number(raw) : field === 'audio' ? true : raw;
        rules.push({ field: field, value: value, label: node.textContent });
      } else {
        text += node.textContent || '';
      }
    });
    var v = this.view;
    v.fieldFilters = rules;
    var q = text.replace(/\u00a0/g, ' ').replace(/\s+/g, ' ').trim();
    // An emptied field can keep a stray <br>; clear it so the placeholder returns.
    if (!rules.length && !q && this.dom.field.innerHTML !== '') this.dom.field.innerHTML = '';
    var changed = q !== v.query;
    v.query = q;
    if (changed) this.searchContents();
    this.renderList();
  };

  /** Puts filter tokens in the field, as if typed (the backlinks "more" link). */
  Journal.prototype.setFieldTokens = function (tokens) {
    var f = this.dom.field;
    f.innerHTML = '';
    var self = this;
    tokens.forEach(function (t) {
      f.appendChild(self.tokenNode(t.field, t.value, t.kw, t.label));
      f.appendChild(document.createTextNode(' '));
    });
    this.syncField();
  };

  /** Asks the server which notes' text matches, when searching contents. */
  Journal.prototype.searchContents = function () {
    var self = this;
    var v = this.view;
    clearTimeout(this.timers.search);
    if (v.scope !== 'contents' || Array.from(v.query).length < 2) {
      this.contentHits = null;
      return;
    }
    var q = v.query;
    this.timers.search = setTimeout(function () {
      Chronicle.apiFetch(self.api('/search?q=' + encodeURIComponent(q))).then(json).then(function (hits) {
        if (self.dead || self.view.query !== q || self.view.scope !== 'contents') return;
        var map = {};
        (hits || []).forEach(function (h) { map[h.id] = h.snippet || ''; });
        self.contentHits = map;
        self.renderList();
      }).catch(function () { /* titles still match; contents just add nothing */ });
    }, 250);
  };

  // --- View options: group, sort, direction, density, show archived ------------------

  Journal.prototype.renderFold = function () {
    var v = this.view;
    function opt(attrName, value, label, on) {
      return '<button type="button" class="nl-optrow" role="menuitemradio" aria-checked="' + on + '" data-' + attrName + '="' + value + '">' +
        '<i class="fa-solid fa-check ck" aria-hidden="true"></i><span>' + esc(label) + '</span></button>';
    }
    var dirs = v.sortBy === 'title' ? ['A → Z', 'Z → A'] : ['Newest first', 'Oldest first'];
    // For title the natural first choice is ascending; for dates, descending.
    var first = v.sortBy === 'title' ? 'asc' : 'desc';
    var second = first === 'asc' ? 'desc' : 'asc';
    this.dom.fold.innerHTML =
      '<div class="nl-fold-sec"><div class="nl-fold-label">Group by</div>' +
      JL.GROUPS.map(function (g) { return opt('group', g[0], g[1], v.groupBy === g[0]); }).join('') + '</div>' +
      '<div class="nl-fold-sec"><div class="nl-fold-label">Sort</div>' +
      JL.SORTS.map(function (s) { return opt('sort', s[0], s[1], v.sortBy === s[0]); }).join('') +
      '<div class="nl-fold-row2"><span>Direction</span><div class="nl-seg2">' +
      '<button type="button" data-dir="' + first + '" aria-pressed="' + (v.sortDir === first) + '">' + dirs[0] + '</button>' +
      '<button type="button" data-dir="' + second + '" aria-pressed="' + (v.sortDir === second) + '">' + dirs[1] + '</button></div></div></div>' +
      '<div class="nl-fold-sec"><div class="nl-fold-label">Density</div><div class="nl-seg2" style="display:flex">' +
      '<button type="button" data-density="compact" aria-pressed="' + (v.density === 'compact') + '">Compact</button>' +
      '<button type="button" data-density="comfortable" aria-pressed="' + (v.density === 'comfortable') + '">Comfortable</button></div></div>' +
      '<div class="nl-fold-sec"><div class="nl-check-row"><span id="jnl-archived-label">Show archived</span>' +
      '<button type="button" class="nl-tog" role="switch" aria-labelledby="jnl-archived-label" aria-checked="' + v.archiveView + '" data-archived></button></div></div>';
  };

  Journal.prototype.onFoldClick = function (e) {
    var v = this.view;
    var b;
    if ((b = e.target.closest('[data-group]'))) v.groupBy = b.getAttribute('data-group');
    else if ((b = e.target.closest('[data-sort]'))) {
      var was = v.sortBy;
      v.sortBy = b.getAttribute('data-sort');
      if ((was === 'title') !== (v.sortBy === 'title')) v.sortDir = v.sortBy === 'title' ? 'asc' : 'desc';
    } else if ((b = e.target.closest('[data-dir]'))) v.sortDir = b.getAttribute('data-dir');
    else if ((b = e.target.closest('[data-density]'))) { v.density = b.getAttribute('data-density'); this.savePrefs(); }
    else if ((b = e.target.closest('[data-archived]'))) { v.archiveView = !v.archiveView; this.clearSelection(true); }
    else return;
    this.renderFold();
    this.renderList();
    var again = b.getAttribute('data-group') ? '[data-group="' + v.groupBy + '"]' : null;
    if (again) { var el = this.dom.fold.querySelector(again); if (el) el.focus({ preventScroll: true }); }
  };

  Journal.prototype.openFold = function () {
    this.closeNewMenu();
    this.closePop();
    this.renderFold();
    var r = this.dom.viewBtn.getBoundingClientRect();
    var w = 236;
    var fold = this.dom.fold;
    fold.style.left = clamp(r.right - w, 8, window.innerWidth - w - 8) + 'px';
    fold.style.top = (r.bottom + 6) + 'px';
    fold.classList.add('open');
    this.dom.viewBtn.classList.add('open');
    this.dom.viewBtn.setAttribute('aria-expanded', 'true');
    this.foldOpen = true;
    var first = fold.querySelector('[aria-checked="true"]') || fold.querySelector('button');
    if (first) first.focus({ preventScroll: true });
  };

  Journal.prototype.closeFold = function (restore) {
    if (!this.foldOpen) return;
    this.foldOpen = false;
    this.dom.fold.classList.remove('open');
    this.dom.viewBtn.classList.remove('open');
    this.dom.viewBtn.setAttribute('aria-expanded', 'false');
    if (restore) this.dom.viewBtn.focus();
  };

  // --- Opening a note --------------------------------------------------------------

  /**
   * Opens a note in the editor. The note is fetched fresh through the gated
   * route, so a deep link to something the viewer cannot see reads exactly
   * like one to a note that does not exist.
   */
  Journal.prototype.open = function (id, opts) {
    opts = opts || {};
    var self = this;
    if (!ID_RE.test(id || '')) return Promise.resolve(false);
    if (this.active && this.active.id === id && !opts.force) {
      if (isPhone() && !opts.keepPane) this.setPane('note');
      return Promise.resolve(true);
    }
    this.flushSave();
    this.releaseLock();
    var seq = ++this.openSeq;
    return Chronicle.apiFetch(this.api('/' + encodeURIComponent(id))).then(json).then(function (note) {
      if (seq !== self.openSeq || self.dead) return false;
      if (note.entityId) {
        // A jot lives on its page, not in the Journal.
        Chronicle.go('/campaigns/' + encodeURIComponent(self.cid) + '/entities/' + encodeURIComponent(note.entityId));
        return false;
      }
      if (note.isFolder) return false;
      self.showNote(note, opts);
      return true;
    }).catch(function () {
      if (seq !== self.openSeq || self.dead) return false;
      if (!opts.quiet) self.toast('That note isn’t available.');
      if (load(self.prefix + 'last', '') === id) store(self.prefix + 'last', '');
      if (!self.active) self.setUrl(null);
      return false;
    });
  };

  /** Entry point for the app's search and other surfaces while mounted. */
  Journal.prototype.openFromOutside = function (id) {
    if (!ID_RE.test(id || '')) return false;
    this.open(id);
    return true;
  };

  Journal.prototype.reloadActive = function () {
    if (!this.active) return Promise.resolve(false);
    var keep = this.pane;
    return this.open(this.active.id, { force: true, keepPane: true, quiet: true }).then(function (ok) { return ok && keep; });
  };

  Journal.prototype.setUrl = function (id) {
    // In an outside app's frame the address is the frame's own; a reload
    // must land back on the frame, not the cookie-only Journal page.
    if (Chronicle.embed) return;
    var path = '/campaigns/' + encodeURIComponent(this.cid) + '/journal' + (id ? '/' + encodeURIComponent(id) : '');
    try {
      if (window.location.pathname !== path) window.history.replaceState(window.history.state, '', path);
    } catch (e) { /* the address bar just stays put */ }
  };

  Journal.prototype.showNote = function (note, opts) {
    opts = opts || {};
    this.active = note;
    this.dirtyTitle = false;
    this.dirtyBody = false;
    store(this.prefix + 'last', note.id);
    this.setUrl(note.id);
    this.dom.blank.hidden = true;
    this.dom.note.hidden = false;
    this.dom.title.value = note.title || '';
    this.mountEditor(note);
    this.renderMeta();
    this.renderLegacy();
    var lockedBy = this.lockedByOther();
    this.applyReadOnly(!!lockedBy);
    this.setStatus(lockedBy ? 'Read only while ' + this.memberName(lockedBy) + ' edits' : 'Saved');
    this.renderUpdated();
    this.upsertRow(note);
    this.renderList();
    this.drawerFor = null;
    this.renderLinksOut();
    this.loadBacklinks();
    if (this.drawerIsOpen()) this.loadDrawerDetails();
    if (isPhone() && !opts.keepPane) this.setPane('note');
    if (opts.fresh) {
      this.dom.title.focus();
      this.dom.title.select();
    }
  };

  /** Clears the editor back to the "no note open" state. */
  Journal.prototype.closeNote = function () {
    this.flushSave();
    this.releaseLock();
    this.destroyEditor();
    this.active = null;
    this.openSeq++;
    this.dom.note.hidden = true;
    this.dom.blank.hidden = false;
    this.dom.body.innerHTML = '';
    this.setUrl(null);
    this.setDrawer(false);
    this.markActiveRow();
  };

  /** Called after the list reloads: the open note may have been deleted or unshared. */
  Journal.prototype.checkActiveStillVisible = function () {
    var a = this.active;
    if (!a || this.byId[a.id]) return;
    if (this.pendingDelete && this.pendingDelete.ids.indexOf(a.id) !== -1) return;
    this.dirtyTitle = this.dirtyBody = false;
    this.closeNote();
    this.toast('That note is no longer available to you.');
  };

  // --- The editor ----------------------------------------------------------------------

  function initialContent(note) {
    if (note.entry) {
      try { return typeof note.entry === 'string' ? JSON.parse(note.entry) : note.entry; } catch (e) { /* fall through */ }
    }
    if (note.entryHtml) return note.entryHtml;
    // Notes written in the old floating panel keep their text in blocks.
    var html = '';
    (note.content || []).forEach(function (b) {
      if (b.type === 'text' && b.value) html += '<p>' + esc(b.value).replace(/\n/g, '<br>') + '</p>';
    });
    return html;
  }

  Journal.prototype.mountEditor = function (note) {
    var self = this;
    this.destroyEditor();
    var host = this.dom.body;
    host.innerHTML = '';
    var T = window.TipTap;
    if (!T || !T.Editor || !Chronicle.NoteLink) {
      host.innerHTML = '<div class="ProseMirror prose prose-sm max-w-none">' + (note.entryHtml || '') + '</div>';
      this.readOnly = true;
      return;
    }
    var LinkMark = Chronicle.MentionLink || T.Link;
    var ext = [
      T.StarterKit.configure({ link: false, underline: false, heading: { levels: [1, 2, 3] } }),
      T.Underline,
      T.Placeholder.configure({ placeholder: 'Start writing… type [[ to link a note or a page.' }),
      LinkMark.configure({ openOnClick: false, autolink: true }),
      Chronicle.NoteLink.configure({ campaignId: this.cid, onOpen: function (id) { self.open(id); } })
    ];
    if (T.TaskList && T.TaskItem) ext.push(T.TaskList, T.TaskItem.configure({ nested: true }));
    // Pictures are in the schema for everyone, so a note holding one loads.
    if (Chronicle.EditorImage) ext.push(Chronicle.EditorImage.extension);
    if (Chronicle.EditorDiagram && Chronicle.EditorDiagram.extension) ext.push(Chronicle.EditorDiagram.extension);
    var pictureProps = Chronicle.EditorImage ? Chronicle.EditorImage.pasteDropProps(function () { return self.editor; }, this.cid) : {};

    this.wiki = Chronicle.WikiLinkExtension({
      campaignId: this.cid,
      notes: function (q) { return self.linkCandidates(q); },
      onLinked: function (it) { self.toast('Linked to ' + it.title); }
    });
    this.mention = Chronicle.MentionExtension ? Chronicle.MentionExtension({ campaignId: this.cid }) : null;

    var ed = new T.Editor({
      element: host,
      extensions: ext,
      content: initialContent(note),
      editorProps: {
        attributes: { class: 'prose prose-sm max-w-none focus:outline-none', 'aria-label': 'Note text', role: 'textbox', 'aria-multiline': 'true' },
        handleKeyDown: function (view, event) {
          if (self.wiki && self.wiki.onKeyDown(null, event)) return true;
          if (self.mention && self.mention.onKeyDown(null, event)) return true;
          return false;
        },
        handlePaste: pictureProps.handlePaste,
        handleDrop: pictureProps.handleDrop
      },
      onUpdate: function (p) {
        // Only a real edit counts: toggling editability emits an update too.
        if (p.transaction && !p.transaction.docChanged) return;
        if (self.wiki) self.wiki.onUpdate(p.editor);
        if (self.mention) self.mention.onUpdate(p.editor);
        self.onBodyChange();
      },
      onSelectionUpdate: function (p) {
        self.paintToolbar();
        if (self.wiki && self.wiki.isOpen()) self.wiki.onUpdate(p.editor);
      },
      onFocus: function () { self.ensureLock(); },
      onBlur: function () { self.flushSave(); }
    });
    this.editor = ed;
    if (Chronicle.EditorImage) Chronicle.EditorImage.useNotePictures(ed, this.cid);
    this.wiki.onCreate(ed);
    if (this.mention) this.mention.onCreate(ed);
    this.paintToolbar();
  };

  Journal.prototype.destroyEditor = function () {
    clearTimeout(this.timers.links);
    if (this.wiki) { this.wiki.onDestroy(); this.wiki = null; }
    if (this.mention) { this.mention.onDestroy(); this.mention = null; }
    if (this.editor) { this.editor.destroy(); this.editor = null; }
  };

  /** Notes the [[ picker offers: visible, not archived, not this one. */
  Journal.prototype.linkCandidates = function (query) {
    var self = this;
    var q = (query || '').trim().toLowerCase();
    var here = this.active ? this.active.id : '';
    var pd = this.pendingDelete ? this.pendingDelete.ids : [];
    var list = this.notes.filter(function (n) {
      return n.id !== here && !n.archived && pd.indexOf(n.id) === -1 && (!q || (n.title || '').toLowerCase().indexOf(q) !== -1);
    });
    list.sort(function (a, b) {
      var ap = (a.title || '').toLowerCase().indexOf(q) === 0 ? 0 : 1;
      var bp = (b.title || '').toLowerCase().indexOf(q) === 0 ? 0 : 1;
      if (ap !== bp) return ap - bp;
      return JL.toMs(b.updatedAt) - JL.toMs(a.updatedAt);
    });
    return list.slice(0, 6).map(function (n) {
      return { id: n.id, title: n.title || 'Untitled', sub: JL.folderPath(n.parentId, self.folders) || 'Journal note' };
    });
  };

  Journal.prototype.paintToolbar = function () {
    var ed = this.editor;
    var tb = this.dom.toolbar;
    function on(cmd, active) {
      var b = tb.querySelector('[data-cmd="' + cmd + '"]');
      if (b) { b.classList.toggle('on', !!active); b.setAttribute('aria-pressed', String(!!active)); }
    }
    on('bold', ed && ed.isActive('bold'));
    on('italic', ed && ed.isActive('italic'));
    on('heading', ed && ed.isActive('heading'));
    on('tasks', ed && ed.isActive('taskList'));
  };

  Journal.prototype.runCommand = function (cmd) {
    var ed = this.editor;
    if (!ed || this.readOnly) return;
    var c = ed.chain().focus();
    if (cmd === 'bold') c.toggleBold().run();
    else if (cmd === 'italic') c.toggleItalic().run();
    else if (cmd === 'heading') c.toggleHeading({ level: 2 }).run();
    else if (cmd === 'tasks' && c.toggleTaskList) c.toggleTaskList().run();
    else if (cmd === 'link' && this.wiki) this.wiki.begin();
    else if (cmd === 'picture' && Chronicle.EditorImage) Chronicle.EditorImage.pickAndInsert(ed, this.cid);
    else if (cmd === 'diagram' && Chronicle.EditorDiagram && Chronicle.EditorDiagram.insert) Chronicle.EditorDiagram.insert(ed);
    this.paintToolbar();
  };

  // Ownership and the edit lock decide what can be touched.
  Journal.prototype.isOwner = function () {
    return !!(this.active && this.active.userId === this.uid);
  };

  /** Who else holds a live edit lock on the open note, or "". */
  Journal.prototype.lockedByOther = function () {
    var n = this.active;
    if (!n || n.visibility === 'private' || !n.lockedBy || n.lockedBy === this.uid || !lockFresh(n.lockedAt)) return '';
    return n.lockedBy;
  };

  Journal.prototype.applyReadOnly = function (ro) {
    this.readOnly = !!ro;
    if (this.editor) this.editor.setEditable(!ro, false);
    this.dom.title.readOnly = !!ro;
    this.dom.body.classList.toggle('readonly', !!ro);
    Array.prototype.forEach.call(this.dom.toolbar.querySelectorAll('button, input'), function (b) { b.disabled = !!ro; });
    Array.prototype.forEach.call(this.dom.legacy.querySelectorAll('input'), function (b) { b.disabled = !!ro; });
  };

  Journal.prototype.renderMeta = function () {
    var n = this.active;
    if (!n) return;
    var own = this.isOwner();
    var chips = this.dom.chips;
    chips.classList.toggle('readonly', !own);
    Array.prototype.forEach.call(chips.querySelectorAll('.vis-chip'), function (b) {
      var on = b.getAttribute('data-vis') === n.visibility;
      b.setAttribute('aria-pressed', String(on));
      b.disabled = !own;
      b.title = own ? '' : 'Only the note’s owner can change who sees it';
    });
    var custom = chips.querySelector('[data-vis="custom"] span');
    if (custom) {
      var count = own && n.visibility === 'custom' && n.sharedWith ? n.sharedWith.length : 0;
      custom.textContent = count ? 'Specific people (' + count + ')' : 'Specific people';
    }
    var pin = this.dom.pinBtn;
    pin.hidden = !own;
    pin.setAttribute('aria-pressed', String(!!n.pinned));
    pin.title = n.pinned ? 'Unpin note' : 'Pin note';
    pin.setAttribute('aria-label', pin.title);
    var arch = this.dom.archiveBtn;
    arch.hidden = !own;
    arch.classList.toggle('on', !!n.archived);
    arch.title = n.archived ? 'Unarchive this note' : 'Archive this note';
    arch.setAttribute('aria-label', arch.title);
    this.dom.owner.hidden = own;
    this.dom.owner.textContent = own ? '' : 'By ' + this.memberName(n.userId);
    var lockedBy = this.lockedByOther();
    this.dom.lock.hidden = !lockedBy;
    if (lockedBy) this.dom.lock.querySelector('.who').textContent = 'Editing: ' + this.memberName(lockedBy);
  };

  Journal.prototype.renderUpdated = function () {
    var n = this.active;
    this.dom.updated.textContent = n ? '· updated ' + JL.longDate(n.updatedAt) : '';
  };

  Journal.prototype.setStatus = function (text, error) {
    this.dom.status.textContent = text;
    this.dom.status.classList.toggle('error', !!error);
  };

  /** Legacy checklist blocks from the old floating panel stay tickable here. */
  Journal.prototype.renderLegacy = function () {
    var n = this.active;
    var html = '';
    (n && n.content || []).forEach(function (b, bi) {
      if (b.type !== 'checklist' || !b.items || !b.items.length) return;
      html += '<div class="lg-title">Checklist</div>';
      b.items.forEach(function (it, ii) {
        html += '<label class="' + (it.checked ? 'done' : '') + '"><input type="checkbox" data-b="' + bi + '" data-i="' + ii + '"' + (it.checked ? ' checked' : '') + '><span>' + esc(it.text) + '</span></label>';
      });
    });
    this.dom.legacy.innerHTML = html;
    this.dom.legacy.hidden = !html;
  };

  Journal.prototype.toggleLegacy = function (bi, ii) {
    var self = this;
    var n = this.active;
    if (!n || this.readOnly) return;
    Chronicle.apiFetch(this.api('/' + encodeURIComponent(n.id) + '/toggle'), { method: 'POST', body: { blockIndex: bi, itemIndex: ii } })
      .then(json).then(function (updated) {
        if (!self.active || self.active.id !== updated.id) return;
        self.active.content = updated.content;
        self.active.updatedAt = updated.updatedAt;
        self.renderLegacy();
        self.renderUpdated();
      }).catch(function () {
        Chronicle.notify('Could not tick that item.', 'error');
        self.renderLegacy();
      });
  };

  // --- Saving -----------------------------------------------------------------------------

  Journal.prototype.onBodyChange = function () {
    if (this.readOnly || !this.active) return;
    this.dirtyBody = true;
    this.ensureLock();
    this.scheduleSave();
    this.linksSoon();
  };

  Journal.prototype.onTitleInput = function () {
    if (this.readOnly || !this.active) return;
    this.dirtyTitle = true;
    this.ensureLock();
    this.scheduleSave();
    var t = this.dom.title.value.trim() || 'Untitled';
    var row = this.byId[this.active.id];
    if (row) row.title = t;
    var el = this.dom.inner.querySelector('.nl-row.active .nl-title');
    if (el) { el.textContent = t; el.title = t; }
    if (Chronicle.NoteLabels) Chronicle.NoteLabels.prime(this.cid, [{ id: this.active.id, title: t, archived: this.active.archived }]);
  };

  Journal.prototype.scheduleSave = function () {
    var self = this;
    this.setStatus('Edited');
    clearTimeout(this.timers.save);
    this.timers.save = setTimeout(function () { self.save(); }, AUTOSAVE_DELAY);
  };

  /** Saves pending edits now (switching notes, blur, leaving the page). */
  Journal.prototype.flushSave = function (keepalive) {
    clearTimeout(this.timers.save);
    if (this.dirtyTitle || this.dirtyBody) return this.save(keepalive);
    return Promise.resolve();
  };

  /**
   * Sends the title and/or body. Writes are chained so an older one can
   * never land after a newer one; a failed write puts its edits back and
   * tries again while the note is still open.
   */
  Journal.prototype.save = function (keepalive) {
    var self = this;
    var n = this.active;
    clearTimeout(this.timers.save);
    if (!n || this.readOnly || !(this.dirtyTitle || this.dirtyBody)) return Promise.resolve();
    var body = {};
    if (this.dirtyTitle) body.title = this.dom.title.value;
    if (this.dirtyBody && this.editor) {
      body.entry = JSON.stringify(this.editor.getJSON());
      body.entryHtml = this.editor.getHTML();
    }
    this.dirtyTitle = false;
    this.dirtyBody = false;
    var url = this.api('/' + encodeURIComponent(n.id));
    if (keepalive) {
      this.beacon(url, 'PUT', body);
      return Promise.resolve();
    }
    var extra = body.entryHtml != null ? this.editorSummary() : null;
    this.setStatus('Saving…');
    var p = (this.saveChain || Promise.resolve()).then(function () {
      return Chronicle.apiFetch(url, { method: 'PUT', body: body }).then(json);
    }).then(function (updated) {
      self.lastSaveAt = Date.now();
      if (self.dead) return;
      if (self.active && self.active.id === updated.id) {
        self.active.title = updated.title;
        self.active.updatedAt = updated.updatedAt;
        self.active.entry = updated.entry;
        self.active.entryHtml = updated.entryHtml;
        if (!self.dirtyTitle && !self.dirtyBody) self.setStatus('Saved');
        self.renderUpdated();
      }
      self.upsertRow(updated, extra);
      self.renderList();
    }).catch(function (err) {
      var still = self.active && self.active.id === n.id;
      if (err && (err.status === 404 || err.status === 400)) {
        Chronicle.notify(err.status === 404 ? 'That note is gone, so the change was not saved.' : 'That change could not be saved.', 'error');
        if (still) self.setStatus('Not saved', true);
        return;
      }
      if (!still) {
        Chronicle.notify('A change to “' + (n.title || 'Untitled') + '” could not be saved.', 'error');
        return;
      }
      if ('title' in body) self.dirtyTitle = true;
      if ('entry' in body) self.dirtyBody = true;
      self.setStatus('Couldn’t save. Retrying…', true);
      clearTimeout(self.timers.save);
      self.timers.save = setTimeout(function () { self.save(); }, 5000);
    });
    this.saveChain = p;
    return p;
  };

  /** The open note's text and links as the list shows them, read from the editor. */
  Journal.prototype.editorSummary = function () {
    var out = { snippet: '', links: [] };
    var ed = this.editor;
    if (!ed) return out;
    out.snippet = ed.getText({ blockSeparator: ' ' }).replace(/\s+/g, ' ').trim().slice(0, 180);
    out.links = this.editorLinks();
    return out;
  };

  /** Distinct outgoing links in the open note: [{kind, id, label, href}]. */
  Journal.prototype.editorLinks = function () {
    var ed = this.editor;
    var seen = {};
    var links = [];
    if (!ed) return links;
    ed.state.doc.descendants(function (node) {
      if (node.type.name === 'noteLink' && node.attrs.noteId && !seen['note:' + node.attrs.noteId]) {
        seen['note:' + node.attrs.noteId] = true;
        links.push({ kind: 'note', id: node.attrs.noteId });
      }
      if (node.isText) {
        node.marks.forEach(function (m) {
          var id = m.attrs && m.attrs['data-mention-id'];
          if (m.type.name === 'link' && id && ID_RE.test(id) && !seen['page:' + id]) {
            seen['page:' + id] = true;
            links.push({ kind: 'page', id: id, label: node.text.replace(/^@/, '').trim(), href: m.attrs.href });
          }
        });
      }
    });
    return links;
  };

  // --- Sharing, pin, archive -------------------------------------------------------------------

  Journal.prototype.putActive = function (body) {
    var self = this;
    var n = this.active;
    if (!n) return Promise.reject(new Error('no note'));
    return Chronicle.apiFetch(this.api('/' + encodeURIComponent(n.id)), { method: 'PUT', body: body }).then(json).then(function (updated) {
      if (self.active && self.active.id === updated.id) {
        ['visibility', 'sharedWith', 'isShared', 'pinned', 'archived', 'archivedAt', 'updatedAt', 'parentId'].forEach(function (k) { self.active[k] = updated[k]; });
        self.renderMeta();
      }
      self.upsertRow(updated);
      self.renderList();
      return updated;
    });
  };

  Journal.prototype.setVisibility = function (vis, sharedWith) {
    var self = this;
    if (!this.isOwner()) return;
    var body = { visibility: vis };
    if (vis === 'custom') body.sharedWith = sharedWith || [];
    this.putActive(body).then(function (n) {
      if (n.visibility === 'private') self.releaseLock();
      self.toast('Visibility: ' + self.visLabel(n.visibility));
    }).catch(function () {
      Chronicle.notify('Could not change who sees this note.', 'error');
    });
  };

  Journal.prototype.openPeoplePop = function (anchor) {
    var self = this;
    var n = this.active;
    var chosen = {};
    (n.sharedWith || []).forEach(function (id) { chosen[id] = true; });
    var people = this.memberList.filter(function (m) { return m.id !== self.uid; });
    var html = '<span class="pop-label">Share with</span>' +
      (people.length
        ? people.map(function (m) {
          return '<label class="pop-person"><input type="checkbox" value="' + attr(m.id) + '"' + (chosen[m.id] ? ' checked' : '') + '>' +
            esc(m.name) + (m.role ? ' <span style="color:var(--mut);font-size:11px">· ' + esc(m.role) + '</span>' : '') + '</label>';
        }).join('')
        : '<p class="pop-text">Nobody else is in this campaign yet.</p>') +
      '<div class="pop-foot"><button type="button" class="jnl-btn ghost" data-pop-cancel>Cancel</button><button type="button" class="jnl-btn" data-pop-ok>Share</button></div>';
    this.openPop(anchor, html, function (pop) {
      var ok = pop.querySelector('[data-pop-ok]');
      function picked() {
        return Array.prototype.map.call(pop.querySelectorAll('input:checked'), function (i) { return i.value; });
      }
      function sync() { ok.disabled = picked().length === 0; }
      sync();
      pop.addEventListener('change', sync);
      pop.querySelector('[data-pop-cancel]').addEventListener('click', function () { self.closePop(true); });
      ok.addEventListener('click', function () {
        var ids = picked();
        if (!ids.length) return;
        self.closePop();
        self.setVisibility('custom', ids);
      });
    });
  };

  Journal.prototype.togglePin = function () {
    var self = this;
    if (!this.isOwner()) return;
    this.putActive({ pinned: !this.active.pinned }).then(function (n) {
      self.toast(n.pinned ? 'Pinned' : 'Unpinned');
    }).catch(function () { Chronicle.notify('Could not pin the note.', 'error'); });
  };

  Journal.prototype.toggleArchive = function () {
    var self = this;
    if (!this.isOwner()) return;
    this.flushSave();
    this.putActive({ archived: !this.active.archived }).then(function (n) {
      self.toast(n.archived ? 'Archived' : 'Unarchived');
    }).catch(function () { Chronicle.notify('Could not archive the note.', 'error'); });
  };

  // --- The shared-note edit lock ---------------------------------------------------------------

  /**
   * A note other people can see is edited by one person at a time: the lock
   * is taken when editing starts, kept alive by a heartbeat and handed back
   * on leaving. A private note needs none.
   */
  Journal.prototype.ensureLock = function () {
    var self = this;
    var n = this.active;
    this.lastActivity = Date.now();
    if (!n || this.readOnly || n.visibility === 'private' || this.lockHeld === n.id || this.locking === n.id) return;
    var id = n.id;
    this.locking = id;
    Chronicle.apiFetch(this.api('/' + encodeURIComponent(id) + '/lock'), { method: 'POST' }).then(json).then(function () {
      self.locking = null;
      if (!self.active || self.active.id !== id) { self.unlock(id); return; }
      self.lockHeld = id;
      clearInterval(self.timers.heartbeat);
      self.timers.heartbeat = setInterval(function () {
        // Reading a note is not editing it: after a quiet spell the lock goes
        // back, and the next keystroke takes it again.
        if (Date.now() - (self.lastActivity || 0) > LOCK_IDLE_MS) {
          self.flushSave();
          self.releaseLock();
          return;
        }
        Chronicle.apiFetch(self.api('/' + encodeURIComponent(id) + '/heartbeat'), { method: 'POST' }).catch(function () { /* the next edit re-takes it */ });
      }, HEARTBEAT_MS);
    }).catch(function (err) {
      self.locking = null;
      if (!err || err.status !== 409 || !self.active || self.active.id !== id) return;
      // Someone else got there first: drop what was typed in the meantime
      // and show the note as they are leaving it.
      self.dirtyTitle = self.dirtyBody = false;
      clearTimeout(self.timers.save);
      self.reloadActive().then(function () {
        var who = self.lockedByOther();
        self.toast((who ? self.memberName(who) : 'Someone') + ' is editing this note.');
      });
    });
  };

  Journal.prototype.unlock = function (id, keepalive) {
    var url = this.api('/' + encodeURIComponent(id) + '/unlock');
    if (keepalive) { this.beacon(url, 'POST'); return; }
    Chronicle.apiFetch(url, { method: 'POST' }).catch(function () { /* it expires on its own */ });
  };

  Journal.prototype.releaseLock = function (keepalive) {
    clearInterval(this.timers.heartbeat);
    if (!this.lockHeld) return;
    var id = this.lockHeld;
    this.lockHeld = null;
    this.unlock(id, keepalive);
  };

  Journal.prototype.forceUnlock = function () {
    var self = this;
    var n = this.active;
    if (!n || !this.isGM) return;
    Chronicle.apiFetch(this.api('/' + encodeURIComponent(n.id) + '/force-unlock'), { method: 'POST' }).then(json).then(function () {
      self.toast('Unlocked');
      self.reloadActive();
    }).catch(function () { Chronicle.notify('Could not unlock the note.', 'error'); });
  };

  // --- Audio ----------------------------------------------------------------------------------------

  Journal.prototype.uploadAudio = function (file) {
    var self = this;
    var n = this.active;
    if (!n || !file) return;
    var fd = new FormData();
    fd.append('file', file);
    this.setStatus('Uploading audio…');
    Chronicle.apiFetch(this.api('/' + encodeURIComponent(n.id) + '/attachments'), { method: 'POST', body: fd }).then(json).then(function () {
      self.setStatus('Saved');
      self.toast('Audio attached');
      var row = self.byId[n.id];
      if (row) row.hasAudio = true;
      if (self.active && self.active.id === n.id) self.loadAudio();
    }).catch(function () {
      self.setStatus('Upload failed', true);
      Chronicle.notify('Could not upload the audio.', 'error');
    });
  };

  // --- Details drawer: links, backlinks, history, audio -------------------------------------------

  Journal.prototype.drawerIsOpen = function () {
    return isPhone() ? this.pane === 'links' : this.dom.drawer.classList.contains('open');
  };

  Journal.prototype.setDrawer = function (open, fromKeyboard) {
    var d = this.dom.drawer;
    var was = d.classList.contains('open');
    d.classList.toggle('open', !!open);
    this.dom.drawerBtn.setAttribute('aria-expanded', String(!!open));
    if (open && !was) {
      this.loadDrawerDetails();
      if (fromKeyboard) { var c = d.querySelector('[data-act="drawer-close"]'); if (c) c.focus(); }
    }
    if (!open && was && d.contains(document.activeElement)) this.dom.drawerBtn.focus();
  };

  /** History and audio load when the drawer is first shown for a note. */
  Journal.prototype.loadDrawerDetails = function () {
    if (!this.active || this.drawerFor === this.active.id) return;
    this.drawerFor = this.active.id;
    this.loadHistory();
    this.loadAudio();
  };

  Journal.prototype.jumpTo = function (section) {
    if (isPhone()) this.setPane('links'); else this.setDrawer(true);
    var el = this.dom.drawer.querySelector('[data-sec="' + section + '"]');
    if (el) setTimeout(function () { el.scrollIntoView({ behavior: reduced() ? 'auto' : 'smooth', block: 'nearest' }); }, reduced() ? 0 : 60);
  };

  Journal.prototype.linksSoon = function () {
    var self = this;
    clearTimeout(this.timers.links);
    this.timers.links = setTimeout(function () { self.renderLinksOut(); }, 400);
  };

  /** Links out, read from the editor: notes are labelled per viewer, never from the body. */
  Journal.prototype.renderLinksOut = function () {
    var self = this;
    var links = this.editor ? this.editorLinks() : ((this.active && this.byId[this.active.id] && this.byId[this.active.id].links) || []);
    var box = this.dom.out;
    if (!links.length) {
      box.innerHTML = '<p class="dr-hint">This note doesn’t link anywhere yet. Type <b>[[</b> in the note to add one.</p>';
      return;
    }
    links.forEach(function (l) { if (l.kind === 'note' && Chronicle.NoteLabels) Chronicle.NoteLabels.want(self.cid, l.id); });
    box.innerHTML = links.map(function (l) {
      if (l.kind === 'page') {
        var href = l.href || ('/campaigns/' + encodeURIComponent(self.cid) + '/entities/' + encodeURIComponent(l.id));
        return '<a class="dr-item page" href="' + attr(href) + '"><div class="ttl"><i class="fa-solid fa-book-open" aria-hidden="true"></i><span>' + esc(l.label || 'A page') + '</span></div><div class="snip">Campaign page</div></a>';
      }
      var lab = Chronicle.NoteLabels ? Chronicle.NoteLabels.get(self.cid, l.id) : undefined;
      if (lab === null) {
        return '<div class="dr-item hidden-note"><div class="ttl"><i class="fa-solid fa-lock" aria-hidden="true"></i><span>Private note</span></div><div class="snip">A note you can’t see</div></div>';
      }
      return '<button type="button" class="dr-item" data-goto-note="' + attr(l.id) + '"><div class="ttl"><i class="fa-solid fa-file-lines" aria-hidden="true"></i><span>' +
        esc(lab ? lab.title : 'Journal note') + '</span></div><div class="snip">' + (lab && lab.archived ? 'Archived note' : 'Journal note') + '</div></button>';
    }).join('');
  };

  Journal.prototype.loadBacklinks = function () {
    var self = this;
    var n = this.active;
    if (!n) return;
    var id = n.id;
    this.backlinks = [];
    this.pageBacklinks = [];
    this.paintBadge();
    this.dom.back.innerHTML = '<p class="dr-hint">Loading…</p>';
    Chronicle.apiFetch(this.api('/' + encodeURIComponent(id) + '/backlinks')).then(json).then(function (res) {
      if (!self.active || self.active.id !== id) return;
      self.backlinks = (res && res.notes) || [];
      self.pageBacklinks = (res && res.pages) || [];
      self.renderBacklinks();
    }).catch(function () {
      if (self.active && self.active.id === id) self.dom.back.innerHTML = '<p class="dr-hint">Backlinks didn’t load.</p>';
    });
  };

  Journal.prototype.paintBadge = function () {
    var b = this.dom.badge;
    var n = this.backlinks.length + this.pageBacklinks.length;
    b.hidden = !n;
    b.textContent = n > 99 ? '99+' : String(n);
    this.dom.drawerBtn.setAttribute('aria-label', n ? 'Note details, ' + plural(n, 'backlink') : 'Note details');
  };

  Journal.prototype.renderBacklinks = function () {
    var self = this;
    var refs = this.backlinks;
    var pages = this.pageBacklinks;
    this.paintBadge();
    var box = this.dom.back;
    if (!refs.length && !pages.length) {
      box.innerHTML = '<p class="dr-hint">Nothing links here yet. Backlinks appear the moment a note or a page links to this one.</p>';
      return;
    }
    var html = refs.slice(0, BACKLINK_CAP).map(function (r) {
      var jot = !!r.entityId;
      var sub = r.snippet || (jot ? 'A jot on a page' : '');
      return '<button type="button" class="dr-item" ' + (jot ? 'data-goto-page="' + attr(r.entityId) + '"' : 'data-goto-note="' + attr(r.id) + '"') + '>' +
        '<div class="ttl"><i class="fa-solid ' + (jot ? 'fa-note-sticky' : 'fa-file-lines') + '" aria-hidden="true"></i><span>' + esc(r.title || 'Untitled') + '</span></div>' +
        (sub ? '<div class="snip">' + esc(sub) + '</div>' : '') + '</button>';
    }).join('');
    if (refs.length > BACKLINK_CAP) {
      html += '<button type="button" class="jnl-btn ghost dr-more" data-backlinks-all>+' + (refs.length - BACKLINK_CAP) + ' more — filter the list by this</button>';
    }
    // Pages that link here: named only as far as this viewer may see them.
    html += pages.map(function (p) {
      var href = '/campaigns/' + encodeURIComponent(self.cid) + '/entities/' + encodeURIComponent(p.id);
      return '<a class="dr-item page" href="' + attr(href) + '"><div class="ttl"><i class="fa-solid fa-book-open" aria-hidden="true"></i><span>' +
        esc(p.name || 'A page') + '</span></div><div class="snip">' + esc(p.typeName ? p.typeName + ' page' : 'Campaign page') + '</div></a>';
    }).join('');
    box.innerHTML = html;
  };

  /** Too many backlinks to list: narrow the note list to them instead. */
  Journal.prototype.filterByLinksTo = function (id, label) {
    this.setTab('all');
    this.view.groupBy = 'none';
    this.setFieldTokens([{ field: 'linksTo', value: id, kw: 'links', label: label }]);
    this.setDrawer(false);
    if (isPhone()) this.setPane('list');
    this.toast('Filtered to notes linking to this.');
  };

  Journal.prototype.loadHistory = function () {
    var self = this;
    var n = this.active;
    if (!n) return;
    var id = n.id;
    this.dom.history.innerHTML = '<p class="dr-hint">Loading…</p>';
    Chronicle.apiFetch(this.api('/' + encodeURIComponent(id) + '/versions')).then(json).then(function (versions) {
      if (!self.active || self.active.id !== id) return;
      var cur = self.active;
      var html = '<div class="dr-item" style="cursor:default"><div class="dr-row"><span class="grow"><span class="ttl"><span>' + esc(JL.longDate(cur.updatedAt)) + '</span></span>' +
        '<span class="snip">' + esc(self.memberName(cur.lastEditedBy || cur.userId)) + '</span></span><span class="dr-badge">current</span></div></div>';
      html += (versions || []).map(function (v) {
        return '<div class="dr-item" style="cursor:default"><div class="dr-row"><span class="grow"><span class="ttl"><span>' + esc(JL.longDate(v.createdAt)) + '</span></span>' +
          '<span class="snip">' + esc(self.memberName(v.userId)) + '</span></span>' +
          (self.readOnly ? '' : '<button type="button" class="jnl-btn ghost" style="height:26px;padding:0 9px;font-size:11.5px" data-restore="' + attr(v.id) + '" data-when="' + attr(JL.longDate(v.createdAt)) + '">Restore</button>') +
          '</div></div>';
      }).join('');
      if (!versions || !versions.length) html += '<p class="dr-hint">Earlier versions appear here as the note changes; up to 50 are kept.</p>';
      self.dom.history.innerHTML = html;
    }).catch(function () {
      if (self.active && self.active.id === id) self.dom.history.innerHTML = '<p class="dr-hint">History didn’t load.</p>';
    });
  };

  Journal.prototype.restoreVersion = function (vid, when) {
    var self = this;
    var n = this.active;
    if (!n || this.readOnly) return;
    this.flushSave().then(function () {
      return Chronicle.apiFetch(self.api('/' + encodeURIComponent(n.id) + '/versions/' + encodeURIComponent(vid) + '/restore'), { method: 'POST' }).then(json);
    }).then(function (updated) {
      if (!self.active || self.active.id !== updated.id) return;
      self.showNote(updated, { keepPane: true });
      self.drawerFor = null;
      if (self.drawerIsOpen()) self.loadDrawerDetails();
      self.toast('Restored the ' + when + ' version');
    }).catch(function () { Chronicle.notify('Could not restore that version.', 'error'); });
  };

  /** /media/:id takes the file's id: the stored path's name without its folders or extension. */
  function mediaId(p) {
    if (!p) return '';
    var base = p.slice(p.lastIndexOf('/') + 1);
    var dot = base.lastIndexOf('.');
    return dot > 0 ? base.slice(0, dot) : base;
  }

  Journal.prototype.loadAudio = function () {
    var self = this;
    var n = this.active;
    if (!n) return;
    var id = n.id;
    Chronicle.apiFetch(this.api('/' + encodeURIComponent(id) + '/attachments')).then(json).then(function (atts) {
      if (!self.active || self.active.id !== id) return;
      atts = atts || [];
      if (!atts.length) {
        self.dom.audio.innerHTML = '<p class="dr-hint">No audio attached. Attach a session recording from the toolbar microphone.</p>';
        return;
      }
      self.dom.audio.innerHTML = atts.map(function (a) {
        return '<div class="dr-item dr-audio" style="cursor:default" data-att="' + attr(a.id) + '">' +
          '<div class="dr-row"><span class="grow"><span class="ttl"><i class="fa-solid fa-file-audio" aria-hidden="true"></i><span>' + esc(a.originalName || 'Recording') + '</span></span>' +
          '<span class="snip">' + esc(JL.longDate(a.createdAt)) + '</span></span>' +
          '<button type="button" class="del" data-att-del="' + attr(a.id) + '" title="Remove this recording">Remove</button></div>' +
          '<audio controls preload="none" src="/media/' + attr(encodeURIComponent(mediaId(a.filePath))) + '"></audio>' +
          '<details><summary>Transcript</summary><textarea rows="4" data-transcript placeholder="Paste or type a transcript…">' + esc(a.transcript || '') + '</textarea>' +
          '<div class="dr-row" style="justify-content:flex-end"><button type="button" data-att-save="' + attr(a.id) + '">Save transcript</button></div></details>' +
          '</div>';
      }).join('');
    }).catch(function () {
      if (self.active && self.active.id === id) self.dom.audio.innerHTML = '<p class="dr-hint">Audio didn’t load.</p>';
    });
  };

  Journal.prototype.saveTranscript = function (aid, text) {
    var self = this;
    var n = this.active;
    if (!n) return;
    Chronicle.apiFetch(this.api('/' + encodeURIComponent(n.id) + '/attachments/' + encodeURIComponent(aid) + '/transcript'), { method: 'PUT', body: { transcript: text } })
      .then(json).then(function () { self.toast('Transcript saved'); })
      .catch(function () { Chronicle.notify('Could not save the transcript.', 'error'); });
  };

  Journal.prototype.removeAudio = function (aid, anchor) {
    var self = this;
    var n = this.active;
    if (!n) return;
    this.openPop(anchor,
      '<p class="pop-text">Remove this recording? It cannot be brought back.</p>' +
      '<div class="pop-foot"><button type="button" class="jnl-btn ghost" data-pop-cancel>Cancel</button><button type="button" class="jnl-btn danger" data-pop-ok>Remove</button></div>',
      function (pop) {
        pop.querySelector('[data-pop-cancel]').addEventListener('click', function () { self.closePop(true); });
        pop.querySelector('[data-pop-ok]').addEventListener('click', function () {
          self.closePop();
          Chronicle.apiFetch(self.api('/' + encodeURIComponent(n.id) + '/attachments/' + encodeURIComponent(aid)), { method: 'DELETE' }).then(json).then(function () {
            self.toast('Recording removed');
            self.loadAudio();
            self.refreshSoon();
          }).catch(function () { Chronicle.notify('Could not remove the recording.', 'error'); });
        });
      });
  };

  Journal.prototype.bindDrawer = function () {
    var self = this;
    this.dom.drawer.addEventListener('click', function (e) {
      var b;
      if (e.target.closest('[data-act="drawer-close"]')) { self.setDrawer(false); return; }
      if ((b = e.target.closest('[data-goto-note]'))) { self.open(b.getAttribute('data-goto-note')); return; }
      if ((b = e.target.closest('[data-goto-page]'))) {
        self.flushSave();
        Chronicle.go('/campaigns/' + encodeURIComponent(self.cid) + '/entities/' + encodeURIComponent(b.getAttribute('data-goto-page')));
        return;
      }
      if (e.target.closest('[data-backlinks-all]')) { self.filterByLinksTo(self.active.id, self.active.title || 'this note'); return; }
      if ((b = e.target.closest('[data-restore]'))) { self.restoreVersion(b.getAttribute('data-restore'), b.getAttribute('data-when')); return; }
      if ((b = e.target.closest('[data-att-del]'))) { self.removeAudio(b.getAttribute('data-att-del'), b); return; }
      if ((b = e.target.closest('[data-att-save]'))) {
        var ta = b.closest('.dr-audio').querySelector('[data-transcript]');
        self.saveTranscript(b.getAttribute('data-att-save'), ta ? ta.value : '');
      }
    });
    this.dom.drawer.addEventListener('click', function (e) {
      if (e.target.closest('a.dr-item')) self.flushSave();
    }, true);
    if (Chronicle.NoteLabels) {
      this.offLabels = Chronicle.NoteLabels.subscribe(function () { if (self.active) self.renderLinksOut(); });
    }
  };

  // --- Editor events --------------------------------------------------------------------------------

  Journal.prototype.bindEditor = function () {
    var self = this;
    var d = this.dom;
    d.title.addEventListener('input', function () { self.onTitleInput(); });
    d.title.addEventListener('focus', function () { self.ensureLock(); });
    d.title.addEventListener('blur', function () { self.flushSave(); });
    d.title.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') {
        e.preventDefault();
        if (self.editor) self.editor.commands.focus('start');
      }
    });
    d.note.querySelector('.ed-head').addEventListener('click', function (e) {
      var b = e.target.closest('[data-act], [data-vis]');
      if (!b || b.disabled) return;
      var act = b.getAttribute('data-act');
      if (act === 'archive') self.toggleArchive();
      else if (act === 'pin') self.togglePin();
      else if (act === 'drawer') self.setDrawer(!d.drawer.classList.contains('open'), e.detail === 0);
      else if (act === 'unlock') self.forceUnlock();
      else if (b.hasAttribute('data-vis')) {
        var vis = b.getAttribute('data-vis');
        if (vis === 'custom') self.openPeoplePop(b);
        else if (!self.active || vis !== self.active.visibility) self.setVisibility(vis);
      }
    });
    d.toolbar.addEventListener('mousedown', function (e) {
      // Keep the editor's selection when a formatting button is pressed.
      if (e.target.closest('[data-cmd]')) e.preventDefault();
    });
    d.toolbar.addEventListener('click', function (e) {
      var b = e.target.closest('[data-cmd]');
      if (b && !b.disabled) self.runCommand(b.getAttribute('data-cmd'));
    });
    d.audioInput.addEventListener('change', function () {
      var f = d.audioInput.files && d.audioInput.files[0];
      d.audioInput.value = '';
      if (f) self.uploadAudio(f);
    });
    d.note.querySelector('.ed-foot').addEventListener('click', function (e) {
      var b = e.target.closest('[data-jump]');
      if (b) self.jumpTo(b.getAttribute('data-jump'));
    });
    d.legacy.addEventListener('change', function (e) {
      var i = e.target.closest('input[data-b]');
      if (i) self.toggleLegacy(parseInt(i.getAttribute('data-b'), 10), parseInt(i.getAttribute('data-i'), 10));
    });
    // A page link in the note goes to the page on a plain click.
    d.body.addEventListener('click', function (e) {
      var a = e.target.closest('a[data-mention-id]');
      if (!a || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
      var href = a.getAttribute('href');
      if (!href || href.charAt(0) !== '/') return;
      e.preventDefault();
      self.flushSave();
      Chronicle.go(href);
    });
  };

  // --- Phone panes ----------------------------------------------------------------------------------

  Journal.prototype.setPane = function (name) {
    this.pane = name;
    var el = this.el;
    el.classList.remove('pane-list', 'pane-note', 'pane-links');
    el.classList.add('pane-' + name);
    Array.prototype.forEach.call(el.querySelectorAll('.jnl-panetabs [data-pane]'), function (b) {
      b.setAttribute('aria-selected', String(b.getAttribute('data-pane') === name));
    });
    if (name === 'links') this.loadDrawerDetails();
  };

  // --- The resizable list edge ---------------------------------------------------------------------

  Journal.prototype.rowWidth = function () {
    var r = this.q('.jnl-row');
    return r ? r.getBoundingClientRect().width : 900;
  };

  Journal.prototype.wideMax = function () {
    return Math.max(COMPACT_PX + 40, this.rowWidth() - MIN_EDITOR_PX - 13);
  };

  /**
   * Target width for a size. "normal" has no fixed value: it is measured off
   * the stylesheet's own clamp() by briefly clearing the inline width, so a
   * viewer who never resizes keeps the fluid default.
   */
  Journal.prototype.targetWidth = function (size) {
    if (size === 'collapsed') return 0;
    if (size === 'compact') return COMPACT_PX;
    if (size === 'wide') return Math.min(WIDE_PX, this.wideMax());
    var el = this.dom.list;
    var prevW = el.style.width;
    var prevSize = el.getAttribute('data-size');
    var prevT = el.style.transition;
    el.style.transition = 'none';
    el.style.width = '';
    el.setAttribute('data-size', 'normal');
    var w = el.getBoundingClientRect().width;
    el.style.width = prevW;
    if (prevSize == null) el.removeAttribute('data-size'); else el.setAttribute('data-size', prevSize);
    void el.offsetWidth;
    el.style.transition = prevT;
    return w;
  };

  Journal.prototype.nearestSize = function (px) {
    var normal = this.targetWidth('normal');
    var wide = this.targetWidth('wide');
    if (px < COMPACT_PX / 2) return 'collapsed';
    if (px < (COMPACT_PX + normal) / 2) return 'compact';
    if (px < (normal + wide) / 2) return 'normal';
    return 'wide';
  };

  /**
   * The one place the size changes (drag release, the button, the keys). The
   * width lands in one frame; only the content's opacity eases up after it.
   */
  Journal.prototype.applySize = function (size, opts) {
    opts = opts || {};
    if (SIZES.indexOf(size) === -1) size = 'normal';
    this.size = size;
    if (size !== 'collapsed') this.lastOpenSize = size;
    var el = this.dom.list;
    el.classList.remove('tp-live');
    el.setAttribute('data-size', size);
    el.style.width = size === 'normal' ? '' : this.targetWidth(size) + 'px';
    // A collapsed list keeps its place in the tab order out of reach.
    el.inert = size === 'collapsed' && !isPhone();
    if (!opts.first && size !== 'collapsed' && el.animate) {
      el.animate([{ opacity: 0.35 }, { opacity: 1 }], { duration: 160, easing: 'ease-out' });
    }
    var h = this.dom.handle;
    h.setAttribute('aria-valuenow', String(SIZES.indexOf(size)));
    h.setAttribute('aria-valuetext', size);
    var label = size === 'collapsed' ? 'Show note list' : 'Resize note list (' + size + ')';
    this.dom.cycle.title = label;
    this.dom.cycle.setAttribute('aria-label', label);
    this.dom.cycle.innerHTML = '<i class="fa-solid ' + (size === 'collapsed' ? 'fa-chevron-right' : 'fa-table-columns') + '" aria-hidden="true"></i>';
    if (!opts.first) store(SIZE_KEY, size);
  };

  Journal.prototype.bindResize = function () {
    var self = this;
    var h = this.dom.handle;
    var saved = load(SIZE_KEY, 'normal');
    if (SIZES.indexOf(saved) !== -1 && saved !== 'collapsed') this.lastOpenSize = saved;
    this.applySize(saved, { first: true });

    this.dom.cycle.addEventListener('click', function () {
      self.applySize(SIZES[(SIZES.indexOf(self.size) + 1) % SIZES.length]);
    });
    h.addEventListener('keydown', function (e) {
      var i = SIZES.indexOf(self.size);
      if (e.target !== h) return;
      if (e.key === 'ArrowRight') { e.preventDefault(); self.applySize(SIZES[clamp(i + 1, 0, 3)]); }
      else if (e.key === 'ArrowLeft') { e.preventDefault(); self.applySize(SIZES[clamp(i - 1, 0, 3)]); }
      else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); self.applySize(self.size === 'collapsed' ? self.lastOpenSize : 'collapsed'); }
    });
    h.addEventListener('pointerdown', function (e) {
      if (e.button != null && e.button !== 0) return;
      if (e.target.closest('.tp-cycle')) return;
      var el = self.dom.list;
      var startX = e.clientX;
      var startW = el.getBoundingClientRect().width;
      var moved = false;
      h.classList.add('dragging');
      el.classList.add('tp-live');
      try { h.setPointerCapture(e.pointerId); } catch (err) { /* old browsers: moves still arrive */ }
      function move(ev) {
        var dx = ev.clientX - startX;
        if (Math.abs(dx) > 2) moved = true;
        var px = clamp(startW + dx, 0, self.wideMax() + 24);
        el.style.width = px + 'px';
        el.setAttribute('data-size', self.nearestSize(px));
      }
      function up() {
        h.removeEventListener('pointermove', move);
        h.removeEventListener('pointerup', up);
        h.removeEventListener('pointercancel', up);
        h.classList.remove('dragging');
        if (!moved) { el.classList.remove('tp-live'); el.setAttribute('data-size', self.size); return; }
        self.applySize(self.nearestSize(el.getBoundingClientRect().width));
      }
      h.addEventListener('pointermove', move);
      h.addEventListener('pointerup', up);
      h.addEventListener('pointercancel', up);
      e.preventDefault();
    });
  };

  // --- Popovers and toasts ------------------------------------------------------------------------

  /** A small panel that comes out of anchor; bind(pop) wires its contents. */
  Journal.prototype.openPop = function (anchor, html, bind) {
    this.closePop();
    this.closeFold();
    this.closeNewMenu();
    var pop = document.createElement('div');
    pop.className = 'jnl-pop';
    pop.setAttribute('role', 'dialog');
    pop.innerHTML = html;
    this.el.appendChild(pop);
    var r = anchor.getBoundingClientRect();
    var w = pop.offsetWidth || 230;
    var hgt = pop.offsetHeight || 200;
    var left = clamp(r.left, 8, window.innerWidth - w - 8);
    var top = r.bottom + 6;
    var origin = 'top left';
    if (top + hgt > window.innerHeight - 8) { top = Math.max(8, r.top - hgt - 6); origin = 'bottom left'; }
    pop.style.left = left + 'px';
    pop.style.top = top + 'px';
    pop.style.transformOrigin = origin;
    this.popEl = pop;
    this.popAnchor = anchor;
    if (bind) bind(pop);
    requestAnimationFrame(function () { pop.classList.add('open'); });
    var focus = pop.querySelector('input[type="text"], input, button');
    if (focus) focus.focus({ preventScroll: true });
  };

  Journal.prototype.closePop = function (restore) {
    if (!this.popEl) return;
    var pop = this.popEl;
    var anchor = this.popAnchor;
    this.popEl = null;
    this.popAnchor = null;
    pop.remove();
    if (restore && anchor && anchor.isConnected) anchor.focus();
  };

  /** A bottom-centre pill; with opts.undo it carries an Undo button. */
  Journal.prototype.toast = function (msg, opts) {
    opts = opts || {};
    var wrap = this.dom.toasts;
    var el = document.createElement('div');
    el.className = 'jnl-toast';
    el.setAttribute('role', 'status');
    el.innerHTML = '<i class="fa-solid fa-check" aria-hidden="true"></i><span></span>' + (opts.undo ? '<button type="button" class="undo">Undo</button>' : '');
    el.querySelector('span').textContent = msg;
    // Only the newest few stay: a burst of actions must not stack a wall.
    while (wrap.children.length >= 3) wrap.removeChild(wrap.firstChild);
    wrap.appendChild(el);
    requestAnimationFrame(function () { el.classList.add('open'); });
    var done = false;
    function dismiss() {
      if (done) return;
      done = true;
      el.classList.remove('open');
      setTimeout(function () { el.remove(); }, reduced() ? 130 : 260);
    }
    if (opts.undo) {
      el.querySelector('.undo').addEventListener('click', function () {
        if (done) return;
        done = true;
        opts.undo();
        el.remove();
      });
    }
    setTimeout(dismiss, opts.duration || 2400);
  };

  // --- Page-level keys, clicks and lifecycle ---------------------------------------------------------

  Journal.prototype.bindGlobal = function () {
    var self = this;

    Array.prototype.forEach.call(this.el.querySelectorAll('.jnl-panetabs [data-pane]'), function (b) {
      b.addEventListener('click', function () { self.setPane(b.getAttribute('data-pane')); });
    });

    this.onDocKey = function (e) { self.handleKey(e); };
    this.onDocPointer = function (e) { self.handleOutside(e); };
    this.onPageHide = function () { self.leave(true); };
    this.onVisibility = function () {
      if (document.visibilityState === 'hidden') { self.flushSave(); return; }
      if (Date.now() - self.indexAt > 60000) self.refreshSoon(0);
    };
    this.onNoteCreated = function () { self.refreshSoon(0); };
    this.onOpenNote = function (e) {
      var id = e && e.detail && e.detail.noteId;
      if (id && self.byId[id]) self.open(id);
    };
    this.onResize = function () {
      self.hidePeek();
      if (self.size === 'wide') self.applySize('wide', { first: true });
    };
    window.addEventListener('pagehide', this.onPageHide);
    window.addEventListener('chronicle:note-created', this.onNoteCreated);
    window.addEventListener('chronicle:open-note', this.onOpenNote);
    window.addEventListener('resize', this.onResize);
  };

  function typingIn(el) {
    if (!el) return false;
    var tag = el.tagName;
    return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable;
  }

  /**
   * "/" jumps to search. Escape closes the topmost thing that is open, one
   * per press, and hands focus back to whatever opened it.
   */
  Journal.prototype.handleKey = function (e) {
    if (e.key === '/' && !e.ctrlKey && !e.metaKey && !e.altKey && !typingIn(document.activeElement)) {
      if (isPhone()) this.setPane('list');
      if (this.size === 'collapsed') this.applySize(this.lastOpenSize);
      e.preventDefault();
      this.dom.field.focus();
      return;
    }
    if (e.key !== 'Escape' || e.defaultPrevented) return;
    if (this.suggest && this.suggest.open) { this.hideSuggest(); return; }
    if (this.popEl) { this.closePop(true); return; }
    if (this.foldOpen) { this.closeFold(true); return; }
    if (this.newOpen) { this.closeNewMenu(true); return; }
    if (this.dom.peek.classList.contains('on')) { this.hidePeek(); return; }
    if (this.selectedIds().length) { this.clearSelection(); return; }
    if (!isPhone() && this.dom.drawer.classList.contains('open')) { this.setDrawer(false); this.dom.drawerBtn.focus(); }
  };

  /** A press outside an open fold, menu or popover closes it. */
  Journal.prototype.handleOutside = function (e) {
    var t = e.target;
    if (this.foldOpen && !this.dom.fold.contains(t) && !this.dom.viewBtn.contains(t)) this.closeFold();
    if (this.newOpen && !this.dom.newMenu.contains(t) && !this.dom.newCaret.contains(t)) this.closeNewMenu();
    if (this.popEl && !this.popEl.contains(t) && !(this.popAnchor && this.popAnchor.contains(t))) this.closePop();
  };

  /**
   * Leaving the page (or the Journal): pending edits, a pending delete and
   * a held lock all go out now. keepalive lets them outlive an unloading page.
   */
  Journal.prototype.leave = function (keepalive) {
    this.flushSave(keepalive);
    this.commitDelete(keepalive);
    this.releaseLock(keepalive);
  };

  // --- Live updates ---------------------------------------------------------------------------------------

  /**
   * Note events carry ids only and reach only the note's audience; each one
   * is answered by fetching again through the gated routes.
   */
  Journal.prototype.connect = function () {
    var self = this;
    if (typeof window.WebSocket !== 'function' || this.dead) return;
    var proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    var url = proto + '//' + window.location.host + '/ws?campaign=' + encodeURIComponent(this.cid);
    // Inside an outside app's frame there is no sign-in cookie; the notes
    // grant goes as a subprotocol, which keeps it out of the URL and logs.
    var embed = Chronicle.embed;
    var ws;
    try {
      ws = embed && embed.token
        ? new WebSocket(url, ['chronicle.notes', 'chronicle.grant.' + embed.token])
        : new WebSocket(url);
    } catch (e) { return; }
    this.ws = ws;
    ws.addEventListener('message', function (ev) {
      var msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      if (!msg || msg.campaignId !== self.cid || typeof msg.type !== 'string' || msg.type.indexOf('note.') !== 0) return;
      self.onRemote(msg.type, (msg.payload && msg.payload.noteId) || msg.resourceId || '');
    });
    ws.addEventListener('error', function (e) { if (e.preventDefault) e.preventDefault(); });
    ws.addEventListener('close', function () {
      if (self.ws === ws) self.ws = null;
      if (self.dead) return;
      // Reconnect gently; the list is refetched on return so nothing is missed.
      self.wsDelay = Math.min(60000, (self.wsDelay || 2500) * 2);
      self.timers.ws = setTimeout(function () {
        self.connect();
        self.refreshSoon(0);
      }, self.wsDelay);
    });
    ws.addEventListener('open', function () { self.wsDelay = 0; });
  };

  Journal.prototype.onRemote = function (type, noteId) {
    var self = this;
    var a = this.active;
    // An echo of our own save changes nothing the list does not already show.
    var echo = a && a.id === noteId && type === 'note.updated' && Date.now() - this.lastSaveAt < 2500;
    if (!echo) this.refreshSoon();
    if (!a || a.id !== noteId) return;
    if (type === 'note.deleted') {
      this.dirtyTitle = this.dirtyBody = false;
      this.closeNote();
      this.toast('That note was deleted.');
      return;
    }
    // Our own saves echo back; only someone else's change reloads the note,
    // and never over edits that have not been sent yet.
    if (this.dirtyTitle || this.dirtyBody || Date.now() - this.lastSaveAt < 2500 || this.lockHeld === a.id) return;
    clearTimeout(this.timers.remote);
    this.timers.remote = setTimeout(function () {
      if (!self.active || self.active.id !== noteId || self.dirtyTitle || self.dirtyBody) return;
      var before = self.active.updatedAt;
      Chronicle.apiFetch(self.api('/' + encodeURIComponent(noteId))).then(json).then(function (fresh) {
        if (!self.active || self.active.id !== fresh.id || self.dirtyTitle || self.dirtyBody) return;
        if (fresh.updatedAt === before && fresh.lockedBy === self.active.lockedBy) return;
        self.showNote(fresh, { keepPane: true });
      }).catch(function () { self.checkActiveStillVisible(); });
    }, 300);
  };

  // --- Teardown ------------------------------------------------------------------------------------------------

  Journal.prototype.destroy = function () {
    this.leave(false);
    this.dead = true;
    for (var k in this.timers) { clearTimeout(this.timers[k]); clearInterval(this.timers[k]); }
    if (this.ws) { try { this.ws.close(); } catch (e) { /* already closing */ } this.ws = null; }
    this.destroyEditor();
    this.closePop();
    this.hidePeek();
    if (this.offLabels) this.offLabels();
    window.removeEventListener('pagehide', this.onPageHide);
    window.removeEventListener('chronicle:note-created', this.onNoteCreated);
    window.removeEventListener('chronicle:open-note', this.onOpenNote);
    window.removeEventListener('resize', this.onResize);
    if (Chronicle.openJournalNote && this.el._journalOpen === Chronicle.openJournalNote) Chronicle.openJournalNote = null;
  };

  // Document listeners are added and removed here, in one place, so each
  // mount's handlers are exactly the ones its teardown takes away.
  Chronicle.register('journal', {
    init: function (el, config) {
      var j = new Journal(el, config);
      el._journal = j;
      j.start();
      el._journalOpen = Chronicle.openJournalNote;
      if (!j.onDocKey) return;
      document.addEventListener('keydown', j.onDocKey);
      document.addEventListener('pointerdown', j.onDocPointer);
      document.addEventListener('visibilitychange', j.onVisibility);
    },
    destroy: function (el) {
      var j = el._journal;
      if (!j) return;
      document.removeEventListener('keydown', j.onDocKey);
      document.removeEventListener('pointerdown', j.onDocPointer);
      document.removeEventListener('visibilitychange', j.onVisibility);
      j.destroy();
      el._journal = null;
    }
  });
})();
