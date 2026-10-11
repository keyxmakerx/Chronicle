/**
 * notes.js -- Jot notes: the floating panel of notes on a page.
 *
 * One entry point, a small "Jot notes" tab bottom-right with this page's
 * count, opens the panel: This page / All my jots, how many Journal notes
 * reference the page, flat jot cards (no folders: those live in the Journal),
 * and a button to add a jot. A jot can be sent to the Journal, which makes a
 * private Journal note linked to the page and remembers it on the jot.
 *
 * Shared jots use pessimistic locking (2-min heartbeat, 5-min server expiry)
 * and keep version history. entryHtml is server-sanitized and rendered as-is;
 * [[note]] links in it are labelled per viewer (editor_notelink.js).
 *
 * Mount: <div data-widget="notes" data-campaign-id="..." data-entity-id="..." data-user-id="...">
 */
Chronicle.register('notes', {
  /**
   * Initialize the jot panel.
   * @param {HTMLElement} el - Mount point element.
   * @param {Object} config - Parsed data-* attributes.
   */
  init: function (el, config) {
    var campaignId = config.campaignId || '';
    var entityId = config.entityId || '';
    var currentUserId = config.userId || '';
    // In an outside app's frame (the Foundry jot window) the frame is the
    // panel: always open, filling it, with no tab, close, pin or resize.
    var embedded = config.embed === 'true' || config.embed === true;

    var HEARTBEAT_INTERVAL = 2 * 60 * 1000; // 2 minutes
    var STORAGE_KEY = 'chronicle_notes_size';
    var STORAGE_TEXT_SIZE = 'chronicle_notes_text_size';
    var STORAGE_PINNED = 'chronicle_jots_pinned';
    var TEXT_SIZE_DEFAULT = 'md';
    var REF_CAP = 12;

    var state = {
      open: false,
      collapsed: false,
      tab: entityId ? 'page' : 'mine', // 'page' (this page's jots) or 'mine' (all of mine)
      jots: [],       // the viewer's own jots, on every page
      pageJots: [],   // the jots on this page the viewer can see
      pageNames: {},  // page id -> name, for jots listed away from their page
      refs: [],       // Journal notes that link to this page
      editingId: null,
      loading: true,
      loadFailed: false, // the last load failed: say so, not "no jots"
      searchFilter: '',
      // Locking state.
      lockHeartbeatTimer: null,
      lockedNoteId: null,       // note we currently hold a lock on
      // Version history sub-panel.
      versionsNoteId: null,     // note whose history is shown (null = hidden)
      versions: [],
      versionsLoading: false,
      // Cached campaign members for share picker.
      members: null,
      membersLoading: false,
      // The viewer's Journal notes, for the [[ picker; loaded on first edit.
      journal: null
    };

    // Track mini TipTap editor instances per note ID for cleanup.
    var miniEditors = {};
    var wikiExt = null;    // the [[ picker of the jot being edited
    var mentionExt = null; // the @ picker of the jot being edited

    // Debounced autosave for the note currently in edit mode. Mirrors
    // journal.js: an edit schedules a save ~1.5s after typing stops, and a
    // blur / SPA-navigation / page-unload flushes immediately. notesDirty
    // gates every save so blur, the timer, and the explicit Done button all
    // "save only if there are unsaved changes" — which is what keeps Done
    // from writing the note a second time after a blur already flushed it.
    var AUTOSAVE_DELAY = 1500; // ms
    var autosaveTimer = null;
    var noteLinksOff = null; // unsubscribes the note-link labels of the drawn list
    var notesDirty = false;

    // --- DOM Construction ---

    // The one way in: a small tab bottom-right with this page's jot count.
    var fab = document.createElement('button');
    fab.type = 'button';
    fab.className = 'jot-tab';
    fab.innerHTML = '<i class="fa-solid fa-note-sticky" aria-hidden="true"></i><span>Jot notes</span><span class="jot-tab-count" hidden></span>';
    fab.setAttribute('aria-label', 'Open jot notes');
    fab.setAttribute('aria-expanded', 'false');

    // Panel container.
    var panel = document.createElement('div');
    panel.className = 'notes-panel notes-panel-hidden';
    panel.setAttribute('role', 'dialog');
    panel.setAttribute('aria-label', 'Jot notes');
    panel.innerHTML = buildPanelHTML(entityId);

    el.appendChild(fab);
    el.appendChild(panel);

    // --- Saved preferences (localStorage) ---

    function restoreSize() {
      try {
        var saved = JSON.parse(localStorage.getItem(STORAGE_KEY));
        if (saved && saved.w && saved.h) {
          panel.style.width = saved.w + 'px';
          panel.style.height = saved.h + 'px';
        }
      } catch (e) { /* ignore */ }
    }

    function saveSize() {
      try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify({
          w: panel.offsetWidth,
          h: panel.offsetHeight
        }));
      } catch (e) { /* ignore */ }
    }

    // Apply saved size on desktop only (mobile uses full-width).
    if (window.innerWidth >= 640) restoreSize();

    /** Restore and apply the saved text size preference. */
    function restoreTextSize() {
      try {
        var saved = localStorage.getItem(STORAGE_TEXT_SIZE);
        if (saved && (saved === 'sm' || saved === 'md' || saved === 'lg')) {
          applyTextSize(saved);
          return;
        }
      } catch (e) { /* ignore */ }
      applyTextSize(TEXT_SIZE_DEFAULT);
    }

    /** Apply a text size class to the panel and update the settings UI. */
    function applyTextSize(size) {
      panel.classList.remove('notes-size-sm', 'notes-size-md', 'notes-size-lg');
      panel.classList.add('notes-size-' + size);
      panel.querySelectorAll('.notes-size-opt').forEach(function (btn) {
        btn.classList.toggle('notes-size-opt-active', btn.getAttribute('data-size') === size);
      });
      try { localStorage.setItem(STORAGE_TEXT_SIZE, size); } catch (e) { /* ignore */ }
    }

    restoreTextSize();

    function isPinnedOpen() {
      try { return localStorage.getItem(STORAGE_PINNED) === '1'; } catch (e) { return false; }
    }

    // --- Resize handle ---
    // Document listeners live on el so destroy() can take them away: the
    // widget is re-mounted on every page change.
    var resizeHandle = panel.querySelector('.notes-resize-handle');
    var resizing = false;
    var startX, startY, startW, startH;
    function resizeTo(x, y) {
      // Dragging the top-left corner: moving left or up grows the panel.
      panel.style.width = Math.max(280, startW - (x - startX)) + 'px';
      panel.style.height = Math.max(300, startH - (y - startY)) + 'px';
    }
    resizeHandle.addEventListener('pointerdown', function (e) {
      e.preventDefault();
      resizing = true;
      startX = e.clientX;
      startY = e.clientY;
      startW = panel.offsetWidth;
      startH = panel.offsetHeight;
      document.body.style.userSelect = 'none';
    });
    el._notesPointerMove = function (e) {
      if (resizing) resizeTo(e.clientX, e.clientY);
    };
    el._notesPointerUp = function () {
      if (!resizing) return;
      resizing = false;
      document.body.style.userSelect = '';
      saveSize();
    };
    document.addEventListener('pointermove', el._notesPointerMove);
    document.addEventListener('pointerup', el._notesPointerUp);

    // Cache panel elements.
    var headerTitle = panel.querySelector('.notes-header-title');
    var closeBtn = panel.querySelector('.notes-close');
    var pinPanelBtn = panel.querySelector('.notes-pin-panel');
    var collapseBtn = panel.querySelector('.notes-collapse-btn');
    var tabBtns = panel.querySelectorAll('.notes-tab');
    var addBtn = panel.querySelector('.jot-add-btn');
    var notesList = panel.querySelector('.notes-list');
    var searchInput = panel.querySelector('.notes-search-input');
    var refsBox = panel.querySelector('.jot-refs');
    var tabCount = fab.querySelector('.jot-tab-count');

    // --- Live updates ---

    /**
     * While the panel is open, note events (ids only, each reaching only
     * the note's audience) reload the list quietly. A reload never redraws
     * under someone at work: while a jot is being edited, its history is
     * open, or focus is in the list, it waits and tries again. That also
     * covers our own saves, which come back as note.updated.
     */
    var live = (function () {
      var ws = null;
      var retry = null;
      var reload = null;
      var delay = 0;
      var on = false;

      function busy() {
        var a = document.activeElement;
        return !!(state.editingId || state.versionsNoteId || state.loading ||
          (a && a !== document.body && notesList.contains(a)));
      }

      function refresh(wait) {
        clearTimeout(reload);
        reload = setTimeout(function tick() {
          if (!on) return;
          if (busy()) { reload = setTimeout(tick, 2000); return; }
          loadNotes(true);
        }, wait);
      }

      function connect() {
        if (!on || typeof window.WebSocket !== 'function' || !campaignId) return;
        var url = (window.location.protocol === 'https:' ? 'wss:' : 'ws:') + '//' + window.location.host +
          '/ws?campaign=' + encodeURIComponent(campaignId);
        // In an outside app's frame the notes grant stands in for the
        // sign-in cookie, offered as a subprotocol (as the Journal does).
        var embed = Chronicle.embed;
        var sock;
        try {
          sock = embed && embed.token
            ? new WebSocket(url, ['chronicle.notes', 'chronicle.grant.' + embed.token])
            : new WebSocket(url);
        } catch (e) { return; }
        ws = sock;
        sock.addEventListener('message', function (ev) {
          var msg;
          try { msg = JSON.parse(ev.data); } catch (e) { return; }
          if (!msg || msg.campaignId !== campaignId || typeof msg.type !== 'string' || msg.type.indexOf('note.') !== 0) return;
          refresh(400);
        });
        sock.addEventListener('error', function (e) { if (e.preventDefault) e.preventDefault(); });
        sock.addEventListener('open', function () { delay = 0; });
        sock.addEventListener('close', function () {
          if (ws === sock) ws = null;
          if (!on) return;
          // Reconnect gently; reload on return so nothing is missed.
          delay = Math.min(60000, (delay || 2500) * 2);
          retry = setTimeout(function () { connect(); refresh(0); }, delay);
        });
      }

      return {
        refresh: refresh,
        start: function () {
          if (on) return;
          on = true;
          delay = 0;
          connect();
        },
        stop: function () {
          on = false;
          clearTimeout(retry);
          clearTimeout(reload);
          if (ws) { try { ws.close(); } catch (e) { /* already closing */ } ws = null; }
        }
      };
    })();

    // --- Opening, pinning, collapsing ---

    function openPanel() {
      state.open = true;
      panel.classList.remove('notes-panel-hidden');
      fab.classList.add('jot-tab-hidden');
      fab.setAttribute('aria-expanded', 'true');
      loadNotes();
      live.start();
    }

    function closePanel() {
      if (embedded) return;
      state.open = false;
      live.stop();
      flushAutosave();
      panel.classList.add('notes-panel-hidden');
      fab.classList.remove('jot-tab-hidden');
      fab.setAttribute('aria-expanded', 'false');
      // Release any held lock when closing the panel.
      releaseLockIfHeld();
      state.editingId = null;
      state.versionsNoteId = null;
      fab.focus({ preventScroll: true });
    }

    fab.addEventListener('click', function () {
      openPanel();
      setTimeout(function () {
        var first = panel.querySelector('.notes-tab-active') || addBtn;
        if (first) first.focus({ preventScroll: true });
      }, 100);
    });

    closeBtn.addEventListener('click', closePanel);

    if (embedded) {
      fab.hidden = true;
      panel.classList.add('notes-panel-embed');
      closeBtn.hidden = true;
      pinPanelBtn.hidden = true;
      collapseBtn.hidden = true;
      resizeHandle.hidden = true;
    }

    // A pinned panel opens again by itself on the next page.
    function paintPinPanel() {
      var on = isPinnedOpen();
      pinPanelBtn.setAttribute('aria-pressed', String(on));
      pinPanelBtn.classList.toggle('notes-pin-on', on);
      pinPanelBtn.title = on ? 'Stop keeping the panel open' : 'Keep the panel open on every page';
    }
    pinPanelBtn.addEventListener('click', function () {
      try { localStorage.setItem(STORAGE_PINNED, isPinnedOpen() ? '0' : '1'); } catch (e) { /* not remembered */ }
      paintPinPanel();
    });
    paintPinPanel();

    collapseBtn.addEventListener('click', function () {
      state.collapsed = !state.collapsed;
      panel.classList.toggle('notes-panel-collapsed', state.collapsed);
      collapseBtn.setAttribute('aria-expanded', String(!state.collapsed));
      collapseBtn.title = state.collapsed ? 'Expand' : 'Collapse';
    });

    // Escape inside the panel closes it (or its open popover first).
    panel.addEventListener('keydown', function (e) {
      if (e.key !== 'Escape' || e.defaultPrevented) return;
      var pop = panel.querySelector('.note-share-popover:not(.note-share-hidden)');
      if (pop) { pop.classList.add('note-share-hidden'); e.preventDefault(); return; }
      if (!settingsPopover.classList.contains('notes-settings-hidden')) {
        settingsPopover.classList.add('notes-settings-hidden');
        e.preventDefault();
        return;
      }
      e.preventDefault();
      closePanel();
    });

    // On mobile, dodge the virtual keyboard by adjusting panel bottom offset
    // when the visual viewport shrinks (keyboard opening).
    function onViewport() {
      if (!state.open) return;
      var kbHeight = window.innerHeight - window.visualViewport.height;
      panel.style.bottom = (kbHeight > 0 ? kbHeight : 0) + 'px';
    }
    if (window.visualViewport && window.innerWidth < 640) {
      window.visualViewport.addEventListener('resize', onViewport);
    }

    // New jot on this page: made at once and opened for writing.
    if (addBtn) addBtn.addEventListener('click', createNote);

    // Search filter: re-render on input with debounce.
    var searchTimer = null;
    searchInput.addEventListener('input', function () {
      clearTimeout(searchTimer);
      searchTimer = setTimeout(function () {
        state.searchFilter = searchInput.value.trim().toLowerCase();
        renderNotes();
      }, 150);
    });

    // Tab switching.
    tabBtns.forEach(function (btn) {
      btn.addEventListener('click', function () {
        // The tab already open reloads its list, so the click always shows
        // something happening (the loading line, then the list or the
        // empty line), even when it is the only tab. The short wait keeps
        // the loading line on screen long enough to be seen.
        if (btn.getAttribute('data-tab') === state.tab && !state.editingId && !state.versionsNoteId) {
          if (state.loading) return;
          state.loading = true;
          renderNotes();
          setTimeout(loadNotes, 300);
          return;
        }
        state.tab = btn.getAttribute('data-tab');
        tabBtns.forEach(function (b) {
          var on = b === btn;
          b.classList.toggle('notes-tab-active', on);
          b.setAttribute('aria-selected', String(on));
        });
        renderNotes();
      });
    });

    // Settings gear -- toggle popover.
    var settingsBtn = panel.querySelector('.notes-settings-btn');
    var settingsPopover = panel.querySelector('.notes-settings-popover');
    settingsBtn.addEventListener('click', function (e) {
      e.stopPropagation();
      settingsPopover.classList.toggle('notes-settings-hidden');
    });
    settingsPopover.querySelectorAll('.notes-size-opt').forEach(function (btn) {
      btn.addEventListener('click', function (e) {
        e.stopPropagation();
        applyTextSize(btn.getAttribute('data-size'));
      });
    });

    // Close the settings and share popovers when clicking outside them.
    el._notesDocClick = function (e) {
      if (!settingsPopover.classList.contains('notes-settings-hidden') &&
          !settingsPopover.contains(e.target) && !settingsBtn.contains(e.target)) {
        settingsPopover.classList.add('notes-settings-hidden');
      }
      panel.querySelectorAll('.note-share-popover:not(.note-share-hidden)').forEach(function (p) {
        var shareBtn = p.previousElementSibling;
        if (!p.contains(e.target) && (!shareBtn || !shareBtn.contains(e.target))) {
          p.classList.add('note-share-hidden');
        }
      });
    };
    document.addEventListener('click', el._notesDocClick);

    // --- External Events ---
    // Listen for note-created events (from quick capture modal) to refresh.
    var _onNoteCreated = function () { if (state.open) loadNotes(); };
    window.addEventListener('chronicle:note-created', _onNoteCreated);

    // An "open this note" request (search, session journal): a jot opens here;
    // anything else is a Journal note and opens in the Journal.
    var _onOpenNote = function (e) {
      var noteId = e.detail && e.detail.noteId;
      if (!noteId) return;
      fetchNote(noteId).then(function (note) {
        if (!note) return;
        if (!note.entityId) {
          if (Chronicle.openJournalNote && Chronicle.openJournalNote(note.id)) return;
          Chronicle.go(journalUrl(note.id));
          return;
        }
        if (!state.open) openPanel();
        state.tab = note.entityId === entityId ? 'page' : 'mine';
        tabBtns.forEach(function (b) {
          var on = b.getAttribute('data-tab') === state.tab;
          b.classList.toggle('notes-tab-active', on);
          b.setAttribute('aria-selected', String(on));
        });
        loadNotes().then(function () {
          var noteEl = panel.querySelector('.note-card[data-id="' + noteId + '"]');
          if (!noteEl) return;
          noteEl.scrollIntoView({ block: 'nearest' });
          noteEl.classList.add('note-highlight');
          setTimeout(function () { noteEl.classList.remove('note-highlight'); }, 2000);
        });
      });
    };
    window.addEventListener('chronicle:open-note', _onOpenNote);

    // --- API Functions ---

    function apiUrl(path) {
      return '/campaigns/' + campaignId + '/notes' + (path || '');
    }

    function journalUrl(noteId) {
      return '/campaigns/' + encodeURIComponent(campaignId) + '/journal' + (noteId ? '/' + encodeURIComponent(noteId) : '');
    }

    function fetchNote(id) {
      return Chronicle.apiFetch(apiUrl('/' + encodeURIComponent(id)))
        .then(function (r) { return r.ok ? r.json() : null; })
        .catch(function () { return null; });
    }

    function jotsOnly(list) {
      return (list || []).filter(function (n) { return n.entityId && !n.isFolder; });
    }

    /**
     * @param {boolean} [quiet] keep the list on screen while it reloads
     *   (a live update), instead of showing the loading line.
     */
    function loadNotes(quiet) {
      // The jot being edited keeps what was typed over the fresh copy.
      keepEditingInputs();
      var editing = state.editingId ? findNote(state.editingId) : null;
      if (!quiet) {
        state.loading = true;
        renderNotes();
      }

      // A live reload that fails keeps the list rather than emptying it.
      var read = function (r) {
        if (!r.ok && quiet) throw new Error('HTTP ' + r.status);
        return r.ok ? r.json() : [];
      };
      var promises = [
        Chronicle.apiFetch(apiUrl('?scope=jots')).then(read)
      ];
      if (entityId) {
        promises.push(
          Chronicle.apiFetch(apiUrl('?scope=entity&entity_id=' + encodeURIComponent(entityId))).then(read)
        );
      }

      return Promise.all(promises).then(function (results) {
        // Someone started editing while a live reload was out: drawing now
        // would replace the editor under them, so try again later.
        if (quiet && state.editingId !== (editing ? editing.id : null)) {
          live.refresh(2000);
          return;
        }
        state.jots = jotsOnly(results[0]).filter(function (n) { return n.userId === currentUserId; });
        state.pageJots = jotsOnly(results[1]);
        if (editing) replaceNoteInState(editing);
        state.loading = false;
        state.loadFailed = false;
        updateTabCount();
        renderNotes();
        loadPageNames();
        loadRefs();
      }).catch(function () {
        if (quiet) return;
        state.loading = false;
        state.loadFailed = true;
        state.jots = [];
        state.pageJots = [];
        renderNotes();
      });
    }

    /** Names the pages of the viewer's jots, for the All my jots tab. */
    function loadPageNames() {
      var want = [];
      state.jots.forEach(function (n) {
        if (n.entityId !== entityId && !(n.entityId in state.pageNames) && want.indexOf(n.entityId) === -1) want.push(n.entityId);
      });
      // The server names up to 200 pages per request.
      for (var i = 0; i < want.length; i += 200) askPageNames(want.slice(i, i + 200));
    }

    function askPageNames(ids) {
      Chronicle.apiFetch(apiUrl('/page-names?ids=' + encodeURIComponent(ids.join(','))))
        .then(function (r) { return r.ok ? r.json() : {}; })
        .then(function (names) {
          // A page the viewer cannot see stays unnamed ("" marks it asked).
          ids.forEach(function (id) { state.pageNames[id] = (names && names[id]) || ''; });
          if (state.tab === 'mine') renderNotes();
        })
        .catch(function () { /* the cards say "on a page" */ });
    }

    /** How many Journal notes the viewer can see link to this page. */
    function loadRefs() {
      if (!entityId || !refsBox) return;
      Chronicle.apiFetch(apiUrl('/page-refs?entity_id=' + encodeURIComponent(entityId)))
        .then(function (r) { return r.ok ? r.json() : []; })
        .then(function (refs) {
          state.refs = refs || [];
          renderRefs();
        })
        .catch(function () { /* the box keeps its last count */ });
    }

    /** Loads this page's jot count for the tab while the panel is closed. */
    function loadCount() {
      if (!entityId) return;
      Chronicle.apiFetch(apiUrl('?scope=entity&entity_id=' + encodeURIComponent(entityId)))
        .then(function (r) { return r.ok ? r.json() : []; })
        .then(function (list) {
          if (state.open) return;
          state.pageJots = jotsOnly(list);
          updateTabCount();
        })
        .catch(function () { /* the tab just shows no count */ });
    }

    function updateTabCount() {
      var n = entityId ? state.pageJots.length : 0;
      tabCount.hidden = n === 0;
      tabCount.textContent = String(n);
      fab.setAttribute('aria-label', n ? 'Open jot notes, ' + n + ' on this page' : 'Open jot notes');
    }

    /**
     * Leaves the jot being edited before another one opens: its unsaved
     * edit is saved and its edit lock handed back.
     */
    function leaveEditing(nextId) {
      flushAutosave();
      if (state.lockedNoteId && state.lockedNoteId !== nextId) releaseLock(state.lockedNoteId);
    }

    /** A new, empty jot on this page, opened for writing with its title focused. */
    function createNote() {
      if (!entityId) return;
      leaveEditing(null);
      Chronicle.apiFetch(apiUrl(), {
        method: 'POST',
        body: { title: '', entityId: entityId, content: [{ type: 'text', value: '' }] }
      }).then(function (r) {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        return r.json();
      }).then(function (note) {
        addNoteToState(note);
        state.editingId = note.id;
        renderNotes();
        var titleInput = notesList.querySelector('.note-card[data-id="' + note.id + '"] .note-title-input');
        if (titleInput) titleInput.focus();
      }).catch(function () {
        Chronicle.notify('Failed to save note', 'error');
      });
    }

    function addNoteToState(note) {
      if (state.tab !== 'page' && entityId) {
        state.tab = 'page';
        tabBtns.forEach(function (b) {
          var on = b.getAttribute('data-tab') === 'page';
          b.classList.toggle('notes-tab-active', on);
          b.setAttribute('aria-selected', String(on));
        });
      }
      state.pageJots.unshift(note);
      if (note.userId === currentUserId) state.jots.unshift(note);
      updateTabCount();
    }

    function updateNote(id, data) {
      return Chronicle.apiFetch(apiUrl('/' + id), {
        method: 'PUT',
        body: data
      }).then(function (r) {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        return r.json();
      }).then(function (updated) {
        replaceNoteInState(updated);
        return updated;
      });
    }

    function deleteNote(id) {
      if (!confirm('Delete this jot? This cannot be undone.')) return;
      // Release lock before deleting if we hold one.
      if (state.lockedNoteId === id) {
        releaseLockIfHeld();
      }
      Chronicle.apiFetch(apiUrl('/' + id), {
        method: 'DELETE'
      }).then(function (r) {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        state.jots = state.jots.filter(function (n) { return n.id !== id; });
        state.pageJots = state.pageJots.filter(function (n) { return n.id !== id; });
        if (state.editingId === id) state.editingId = null;
        updateTabCount();
        renderNotes();
      }).catch(function () {
        Chronicle.notify('Could not delete the jot.', 'error');
      });
    }

    function toggleCheck(noteId, blockIdx, itemIdx) {
      Chronicle.apiFetch(apiUrl('/' + noteId + '/toggle'), {
        method: 'POST',
        body: { blockIndex: blockIdx, itemIndex: itemIdx }
      }).then(function (r) { return r.json(); })
        .then(function (updated) {
          replaceNoteInState(updated);
          renderNotes();
        });
    }

    function replaceNoteInState(updated) {
      state.jots = state.jots.map(function (n) { return n.id === updated.id ? updated : n; });
      state.pageJots = state.pageJots.map(function (n) { return n.id === updated.id ? updated : n; });
    }

    /**
     * Sends a jot to the Journal: the server makes (or finds) the private
     * Journal note, and the Journal opens at it.
     */
    function sendToJournal(noteId) {
      Chronicle.apiFetch(apiUrl('/' + encodeURIComponent(noteId) + '/send-to-journal'), { method: 'POST' })
        .then(function (r) {
          if (!r.ok) throw new Error('HTTP ' + r.status);
          return r.json();
        })
        .then(function (res) {
          if (res.jot) replaceNoteInState(res.jot);
          renderNotes();
          Chronicle.notify(res.existing ? 'Already in the Journal.' : 'Sent to the Journal, linked both ways.', 'success');
          openInJournal(res.note.id);
        })
        .catch(function () {
          Chronicle.notify('Could not send the jot to the Journal.', 'error');
        });
    }

    function openInJournal(noteId) {
      flushAutosave();
      if (Chronicle.openJournalNote && Chronicle.openJournalNote(noteId)) return;
      Chronicle.go(journalUrl(noteId));
    }

    // --- Locking API ---

    /** Acquire edit lock on a shared note. Returns the refreshed note or null. */
    function acquireLock(noteId) {
      return Chronicle.apiFetch(apiUrl('/' + noteId + '/lock'), {
        method: 'POST'
      }).then(function (r) {
        if (!r.ok) return null;
        return r.json();
      }).then(function (note) {
        if (note) {
          replaceNoteInState(note);
          state.lockedNoteId = noteId;
          startHeartbeat(noteId);
        }
        return note;
      });
    }

    /** Release a held lock. */
    function releaseLock(noteId) {
      stopHeartbeat();
      state.lockedNoteId = null;
      return Chronicle.apiFetch(apiUrl('/' + noteId + '/unlock'), {
        method: 'POST'
      }).catch(function () { /* best effort */ });
    }

    /** Release the currently held lock, if any. */
    function releaseLockIfHeld() {
      if (state.lockedNoteId) {
        releaseLock(state.lockedNoteId);
      }
    }

    /** Send heartbeat to keep the lock alive. */
    function sendHeartbeat(noteId) {
      Chronicle.apiFetch(apiUrl('/' + noteId + '/heartbeat'), {
        method: 'POST'
      }).catch(function () { /* best effort */ });
    }

    function startHeartbeat(noteId) {
      stopHeartbeat();
      state.lockHeartbeatTimer = setInterval(function () {
        sendHeartbeat(noteId);
      }, HEARTBEAT_INTERVAL);
    }

    function stopHeartbeat() {
      if (state.lockHeartbeatTimer) {
        clearInterval(state.lockHeartbeatTimer);
        state.lockHeartbeatTimer = null;
      }
    }

    // --- Version History API ---

    function loadVersions(noteId) {
      state.versionsLoading = true;
      state.versionsNoteId = noteId;
      state.versions = [];
      renderNotes();

      Chronicle.apiFetch(apiUrl('/' + noteId + '/versions')).then(function (r) { return r.ok ? r.json() : []; })
        .then(function (versions) {
          state.versions = versions || [];
          state.versionsLoading = false;
          renderNotes();
        }).catch(function () {
          state.versions = [];
          state.versionsLoading = false;
          renderNotes();
        });
    }

    function restoreVersion(noteId, versionId) {
      Chronicle.apiFetch(apiUrl('/' + noteId + '/versions/' + versionId + '/restore'), {
        method: 'POST'
      }).then(function (r) { return r.json(); })
        .then(function (note) {
          replaceNoteInState(note);
          state.versionsNoteId = null;
          state.versions = [];
          renderNotes();
        });
    }

    // --- Members API (for share picker) ---

    /** Fetch campaign members for the share picker. Caches the result. */
    function fetchMembers() {
      if (state.members) return Promise.resolve(state.members);
      if (state.membersLoading) return Promise.resolve([]);
      state.membersLoading = true;
      return Chronicle.apiFetch(apiUrl('/members'))
        .then(function (r) { return r.ok ? r.json() : []; })
        .then(function (members) {
          // Exclude the current user from the picker. The wire shape is the
          // memberRef in internal/widgets/notes/handler.go — user_id/username/
          // role — so match on user_id, not a non-existent `id`.
          state.members = (members || []).filter(function (m) {
            return m && m.user_id && m.user_id !== currentUserId;
          });
          state.membersLoading = false;
          return state.members;
        })
        .catch(function () {
          state.membersLoading = false;
          state.members = [];
          return [];
        });
    }

    /** The viewer's Journal notes, for the [[ picker. Loaded once, on first edit. */
    function loadJournalForLinks() {
      if (state.journal) return;
      state.journal = [];
      Chronicle.apiFetch(apiUrl('/index'))
        .then(function (r) { return r.ok ? r.json() : { notes: [] }; })
        .then(function (idx) {
          state.journal = ((idx && idx.notes) || []).filter(function (n) { return !n.isFolder && !n.archived; });
          if (Chronicle.NoteLabels) Chronicle.NoteLabels.prime(campaignId, state.journal);
        })
        .catch(function () { /* pages still link; notes just are not offered */ });
    }

    function linkCandidates(query) {
      var q = (query || '').trim().toLowerCase();
      return (state.journal || []).filter(function (n) {
        return !q || (n.title || '').toLowerCase().indexOf(q) !== -1;
      }).slice(0, 6).map(function (n) {
        return { id: n.id, title: n.title || 'Untitled', sub: 'Journal note' };
      });
    }

    // --- Rendering ---

    /** Jots flat, pinned first, then the most recently changed. */
    function sortJots(list) {
      return list.slice().sort(function (a, b) {
        if (!!a.pinned !== !!b.pinned) return a.pinned ? -1 : 1;
        return (Date.parse(b.updatedAt) || 0) - (Date.parse(a.updatedAt) || 0);
      });
    }

    function renderRefs() {
      if (!refsBox) return;
      var refs = state.refs || [];
      refsBox.querySelector('.jot-refs-count').textContent = refs.length;
      refsBox.querySelector('.jot-refs-noun').textContent = refs.length === 1 ? 'Journal note references this page' : 'Journal notes reference this page';
      var list = refsBox.querySelector('.jot-refs-list');
      if (!refs.length) {
        list.innerHTML = '<div class="refline">No Journal notes link to this page yet.</div>';
        return;
      }
      var html = refs.slice(0, REF_CAP).map(function (r) {
        return '<a class="refline" href="' + Chronicle.escapeAttr(journalUrl(r.id)) + '" data-open-note="' + Chronicle.escapeAttr(r.id) + '">' +
          Chronicle.escapeHtml(r.title || 'Untitled') + ' <i class="fa-solid fa-arrow-right" aria-hidden="true"></i></a>';
      }).join('');
      if (refs.length > REF_CAP) {
        html += '<a class="refline more" href="' + Chronicle.escapeAttr(journalUrl() + '?links=' + encodeURIComponent(entityId)) + '">+' +
          (refs.length - REF_CAP) + ' more — see them in the Journal</a>';
      }
      list.innerHTML = html;
    }

    function renderNotes() {
      keepEditingInputs();
      // If version history sub-panel is open, render that instead.
      if (state.versionsNoteId) {
        renderVersionsPanel();
        return;
      }
      headerTitle.textContent = 'Jot notes';

      if (state.loading) {
        notesList.innerHTML = '<div class="notes-empty"><i class="fa-solid fa-spinner fa-spin"></i> Loading...</div>';
        return;
      }

      var list = state.tab === 'page' ? state.pageJots : state.jots;
      if (state.searchFilter) {
        var q = state.searchFilter;
        list = list.filter(function (n) { return n.title && n.title.toLowerCase().indexOf(q) !== -1; });
      }
      if (!list.length) {
        var emptyMsg = state.loadFailed ? 'Couldn\'t load your jots. Click the tab to try again.'
          : state.searchFilter ? 'No matching jots'
          : state.tab === 'page' ? 'No jots here yet — add one below.'
            : 'No jots yet. Jots live on pages: open one to add a jot.';
        notesList.innerHTML = '<div class="notes-empty">' + Chronicle.escapeHtml(emptyMsg) + '</div>';
        return;
      }

      notesList.innerHTML = sortJots(list).map(renderNoteCard).join('');

      // [[links]] to notes are stored without titles; label them for this viewer.
      if (noteLinksOff) noteLinksOff();
      noteLinksOff = Chronicle.hydrateNoteLinks ? Chronicle.hydrateNoteLinks(notesList, campaignId) : null;
      bindCardEvents();
      initMiniEditors();
      // Saved diagrams in the cards that show HTML are drawn here.
      if (Chronicle.EditorDiagram && Chronicle.EditorDiagram.hydrate) Chronicle.EditorDiagram.hydrate(notesList);
    }

    /**
     * Carries what is typed in the jot being edited into its state before the
     * list is drawn again, so a redraw (a share change, page names arriving)
     * never puts back an older title or checklist line. Nothing is saved here.
     */
    function keepEditingInputs() {
      if (!state.editingId) return;
      var card = notesList.querySelector('.note-card[data-id="' + state.editingId + '"]');
      var note = findNote(state.editingId);
      if (!card || !note) return;
      var titleInput = card.querySelector('.note-title-input');
      if (titleInput) note.title = titleInput.value.trim() || 'Untitled';
      card.querySelectorAll('.note-check-text-input').forEach(function (inp) {
        var b = note.content && note.content[parseInt(inp.getAttribute('data-block'), 10)];
        var it = b && b.items && b.items[parseInt(inp.getAttribute('data-item'), 10)];
        if (it) it.text = inp.value;
      });
    }

    /** Who can see a jot, as the share button's label. */
    function shareLabel(note) {
      switch (note.visibility) {
        case 'party': return 'Shared with party';
        case 'gm': return 'Shared with the GM';
        case 'custom': return 'Shared with ' + (note.sharedWith || []).length + ' people';
        default: return 'Private';
      }
    }

    function renderNoteCard(note) {
      var isEditing = state.editingId === note.id;
      var isOwner = note.userId === currentUserId;
      var vis = note.visibility || (note.isShared ? 'party' : (note.sharedWith && note.sharedWith.length ? 'custom' : 'private'));
      var isSharedAny = vis !== 'private';
      var isLockedByOther = isSharedAny && note.lockedBy && note.lockedBy !== currentUserId && note.lockedAt;
      var accent = /^#[0-9a-fA-F]{3,8}$/.test(note.color || '') && note.color !== '#374151' ? ' style="border-left-color:' + note.color + '"' : '';
      var html = '<div class="note-card"' + accent + ' data-id="' + Chronicle.escapeAttr(note.id) + '">';

      // Header row.
      html += '<div class="note-card-header">';
      if (isEditing) {
        html += '<input type="text" class="note-title-input" value="' + Chronicle.escapeAttr(note.title === 'Untitled' ? '' : note.title) + '" placeholder="Jot title..." maxlength="200">';
      } else {
        html += '<span class="note-title">' + Chronicle.escapeHtml(note.title) + '</span>';
      }
      html += '<div class="note-actions">';

      // Share button (owner only) — opens the who-can-see-it popover.
      if (isOwner) {
        var shareIcon = isSharedAny ? 'fa-lock-open' : 'fa-share-nodes';
        var nid = Chronicle.escapeAttr(note.id);
        html += '<div class="note-share-wrap">';
        html += '<button class="note-btn note-share-btn" title="' + Chronicle.escapeAttr(shareLabel(note)) + '" aria-haspopup="true">' +
          '<i class="fa-solid ' + shareIcon + ' text-[10px]"></i></button>';
        html += '<div class="note-share-popover note-share-hidden" data-note-id="' + nid + '">';
        html += '<div class="note-share-opts">';
        html += '<label class="note-share-opt"><input type="radio" name="share-' + nid + '" value="private"' + (vis === 'private' ? ' checked' : '') + '> Private</label>';
        html += '<label class="note-share-opt"><input type="radio" name="share-' + nid + '" value="party"' + (vis === 'party' ? ' checked' : '') + '> Shared with party</label>';
        html += '<label class="note-share-opt"><input type="radio" name="share-' + nid + '" value="gm"' + (vis === 'gm' ? ' checked' : '') + '> Shared with the GM</label>';
        html += '<label class="note-share-opt"><input type="radio" name="share-' + nid + '" value="specific"' + (vis === 'custom' ? ' checked' : '') + '> Specific people</label>';
        html += '</div>';
        html += '<div class="note-share-members' + (vis === 'custom' ? '' : ' note-share-hidden') + '" data-note-id="' + nid + '">';
        html += '<div class="note-share-members-loading"><i class="fa-solid fa-spinner fa-spin text-[10px]"></i> Loading...</div>';
        html += '</div>';
        html += '</div>';
        html += '</div>';
      }

      // Version history button.
      html += '<button class="note-btn note-history-btn" title="Version history"><i class="fa-solid fa-clock-rotate-left text-[10px]"></i></button>';

      // Edit / Done button.
      if (isEditing) {
        html += '<button class="note-btn note-done-btn" title="Done"><i class="fa-solid fa-check"></i></button>';
      } else if (!isLockedByOther) {
        html += '<button class="note-btn note-edit-btn" title="Edit"><i class="fa-solid fa-pen text-[10px]"></i></button>';
      }

      // Delete button (owner only).
      if (isOwner) {
        html += '<button class="note-btn note-delete-btn" title="Delete"><i class="fa-solid fa-trash-can text-[10px]"></i></button>';
      }

      // Marks last, so at rest (tools hidden) they sit at the card's edge.
      if (isLockedByOther) {
        html += '<span class="note-lock-badge" title="Being edited by another user"><i class="fa-solid fa-lock text-[9px]"></i></span>';
      }
      if (isSharedAny && !isOwner) {
        html += '<span class="note-shared-badge" title="' + Chronicle.escapeAttr(shareLabel(note)) + '"><i class="fa-solid fa-users text-[9px]"></i></span>';
      }
      if (note.pinned) {
        html += '<span class="note-pin-mark" title="Pinned"><i class="fa-solid fa-thumbtack text-[9px]"></i></span>';
      }

      html += '</div></div>';

      // In All my jots, each card says which page it sits on.
      if (state.tab === 'mine' && note.entityId) {
        var pageName = note.entityId === entityId ? 'this page' : state.pageNames[note.entityId];
        html += '<a class="jot-page" href="/campaigns/' + Chronicle.escapeAttr(encodeURIComponent(campaignId)) + '/entities/' + Chronicle.escapeAttr(encodeURIComponent(note.entityId)) + '">on ' +
          Chronicle.escapeHtml(pageName || 'a page') + '</a>';
      }

      // Content blocks.
      html += '<div class="note-card-body">';

      if (isEditing) {
        // TipTap rich text editor mount point (initialized after DOM insertion).
        html += '<div class="note-tiptap-mount" data-note-editor="' + Chronicle.escapeAttr(note.id) + '"></div>';

        // Checklist blocks remain as interactive checkboxes (not TipTap).
        (note.content || []).forEach(function (block, bIdx) {
          if (block.type !== 'checklist') return;
          html += '<div class="note-checklist" data-block="' + bIdx + '">';
          (block.items || []).forEach(function (item, iIdx) {
            var checked = item.checked ? ' checked' : '';
            var strikeClass = item.checked ? ' note-checked' : '';
            html += '<label class="note-check-item' + strikeClass + '">';
            html += '<input type="checkbox"' + checked + ' data-block="' + bIdx + '" data-item="' + iIdx + '" class="note-checkbox">';
            html += '<input type="text" class="note-check-text-input" value="' + Chronicle.escapeAttr(item.text) + '" data-block="' + bIdx + '" data-item="' + iIdx + '" placeholder="List item...">';
            html += '</label>';
          });
          html += '<button class="note-add-check-item" data-block="' + bIdx + '"><i class="fa-solid fa-plus text-[9px]"></i> Add item</button>';
          html += '</div>';
        });
        html += '<div class="note-add-block"><button class="note-add-tasks" type="button" title="Checklist" aria-label="Checklist"><i class="fa-solid fa-list-check"></i></button>' +
          (Chronicle.EditorImage ? '<button class="note-add-picture" type="button" title="Picture" aria-label="Picture"><i class="fa-solid fa-image"></i></button>' : '') +
          (Chronicle.EditorDiagram && Chronicle.EditorDiagram.insert ? '<button class="note-add-diagram" type="button" title="Diagram" aria-label="Diagram"><i class="fa-solid fa-diagram-project"></i></button>' : '') + '</div>';
      } else {
        // Display mode: the rich text, then any checklist blocks.
        if (note.entryHtml) {
          html += '<div class="note-entry-html">' + note.entryHtml + '</div>';
        } else {
          (note.content || []).forEach(function (block) {
            if (block.type === 'text' && block.value) {
              html += '<p class="note-text">' + Chronicle.escapeHtml(block.value) + '</p>';
            }
          });
        }
        (note.content || []).forEach(function (block, bIdx) {
          if (block.type !== 'checklist') return;
          html += '<div class="note-checklist" data-block="' + bIdx + '">';
          (block.items || []).forEach(function (item, iIdx) {
            var checked = item.checked ? ' checked' : '';
            var strikeClass = item.checked ? ' note-checked' : '';
            html += '<label class="note-check-item' + strikeClass + '">';
            html += '<input type="checkbox"' + checked + ' data-block="' + bIdx + '" data-item="' + iIdx + '" class="note-checkbox">';
            html += '<span>' + Chronicle.escapeHtml(item.text) + '</span>';
            html += '</label>';
          });
          html += '</div>';
        });
      }
      html += '</div>';

      // Card foot: pin, and the link to the Journal (owner only).
      if (isOwner && !isEditing) {
        html += '<div class="jot-card-foot">';
        html += '<button type="button" class="jot-mini-btn note-pin-btn"><i class="fa-solid fa-thumbtack" aria-hidden="true"></i>' + (note.pinned ? 'Unpin' : 'Pin') + '</button>';
        if (note.linkedNoteId) {
          html += '<button type="button" class="jot-mini-btn linked" data-open-journal="' + Chronicle.escapeAttr(note.linkedNoteId) + '"><i class="fa-solid fa-link" aria-hidden="true"></i>Open in Journal</button>';
        } else {
          html += '<button type="button" class="jot-mini-btn linked" data-send-journal><i class="fa-solid fa-paper-plane" aria-hidden="true"></i>Send to Journal</button>';
        }
        html += '</div>';
      }

      html += '</div>';
      return html;
    }

    /** Render the version history sub-panel. */
    function renderVersionsPanel() {
      var note = findNote(state.versionsNoteId);
      var title = note ? Chronicle.escapeHtml(note.title) : 'Note';

      headerTitle.textContent = 'History: ' + (note ? note.title : '');

      if (state.versionsLoading) {
        notesList.innerHTML = '<div class="notes-versions-header">' +
          '<button class="note-btn notes-versions-back" title="Back"><i class="fa-solid fa-arrow-left"></i></button>' +
          '<span class="notes-versions-title">' + title + '</span>' +
          '</div>' +
          '<div class="notes-empty"><i class="fa-solid fa-spinner fa-spin"></i> Loading...</div>';
        bindVersionsBackBtn();
        return;
      }

      var html = '<div class="notes-versions-header">' +
        '<button class="note-btn notes-versions-back" title="Back"><i class="fa-solid fa-arrow-left"></i></button>' +
        '<span class="notes-versions-title">' + title + '</span>' +
        '</div>';

      if (!state.versions || state.versions.length === 0) {
        html += '<div class="notes-empty">No version history yet</div>';
      } else {
        html += '<div class="notes-versions-list">';
        state.versions.forEach(function (v) {
          var date = new Date(v.createdAt);
          var dateStr = date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
          html += '<div class="notes-version-item" data-vid="' + Chronicle.escapeAttr(v.id) + '">';
          html += '<div class="notes-version-info">';
          html += '<span class="notes-version-title">' + Chronicle.escapeHtml(v.title || 'Untitled') + '</span>';
          html += '<span class="notes-version-date">' + Chronicle.escapeHtml(dateStr) + '</span>';
          html += '</div>';
          html += '<button class="note-btn notes-version-restore" title="Restore this version"><i class="fa-solid fa-rotate-left text-[10px]"></i></button>';
          html += '</div>';
        });
        html += '</div>';
      }

      notesList.innerHTML = html;
      bindVersionsBackBtn();
      bindVersionEvents();
    }

    function bindVersionsBackBtn() {
      var backBtn = notesList.querySelector('.notes-versions-back');
      if (backBtn) {
        backBtn.addEventListener('click', function (e) {
          e.stopPropagation();
          state.versionsNoteId = null;
          state.versions = [];
          renderNotes();
        });
      }
    }

    function bindVersionEvents() {
      notesList.querySelectorAll('.notes-version-restore').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var item = btn.closest('.notes-version-item');
          var vid = item.getAttribute('data-vid');
          restoreVersion(state.versionsNoteId, vid);
        });
      });
    }

    function bindCardEvents() {
      // Edit button -- for shared notes, acquire lock first.
      notesList.querySelectorAll('.note-edit-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          var note = findNote(noteId);
          leaveEditing(noteId);
          if (note && note.visibility && note.visibility !== 'private') {
            acquireLock(noteId).then(function (locked) {
              if (locked) {
                state.editingId = noteId;
                renderNotes();
              } else {
                // The jot open before has already let go of its lock.
                if (state.editingId && state.editingId !== noteId) {
                  state.editingId = null;
                  renderNotes();
                }
                showLockError();
              }
            });
          } else {
            state.editingId = noteId;
            renderNotes();
          }
        });
      });

      // Done button -- save, exit editing, release lock.
      notesList.querySelectorAll('.note-done-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          // Flush instead of an unconditional save: clicking Done blurs the
          // editor first (which already flushed if dirty), so a bare
          // save here would write the note a second time.
          flushAutosave();
          state.editingId = null;
          // Release lock if we hold one for this note.
          if (state.lockedNoteId === noteId) {
            releaseLock(noteId);
          }
          renderNotes();
        });
      });

      // Autosave: mark dirty + debounce on edits to the title and checklist
      // text inputs. The TipTap editor is wired separately via onUpdate in
      // initMiniEditors.
      notesList.querySelectorAll('.note-title-input, .note-check-text-input').forEach(function (inp) {
        inp.addEventListener('input', markNoteDirty);
      });

      // Pin (card foot, owner only).
      notesList.querySelectorAll('.note-pin-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var noteId = btn.closest('.note-card').getAttribute('data-id');
          var note = findNote(noteId);
          if (note) {
            updateNote(noteId, { pinned: !note.pinned }).then(function () {
              renderNotes();
            }).catch(function () { Chronicle.notify('Could not pin the jot.', 'error'); });
          }
        });
      });

      // Send to Journal / Open in Journal (card foot, owner only).
      notesList.querySelectorAll('[data-send-journal]').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          sendToJournal(btn.closest('.note-card').getAttribute('data-id'));
        });
      });
      notesList.querySelectorAll('[data-open-journal]').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          openInJournal(btn.getAttribute('data-open-journal'));
        });
      });

      // Share button — toggle share popover.
      notesList.querySelectorAll('.note-share-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var popover = btn.nextElementSibling;
          if (!popover) return;
          notesList.querySelectorAll('.note-share-popover').forEach(function (p) {
            if (p !== popover) p.classList.add('note-share-hidden');
          });
          popover.classList.toggle('note-share-hidden');
          if (!popover.classList.contains('note-share-hidden')) {
            initSharePopover(popover);
          }
        });
      });

      // Delete button.
      notesList.querySelectorAll('.note-delete-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          deleteNote(btn.closest('.note-card').getAttribute('data-id'));
        });
      });

      // History button.
      notesList.querySelectorAll('.note-history-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          loadVersions(btn.closest('.note-card').getAttribute('data-id'));
        });
      });

      // Checkbox toggle (works in both view and edit modes).
      notesList.querySelectorAll('.note-checkbox').forEach(function (cb) {
        cb.addEventListener('change', function () {
          var noteId = cb.closest('.note-card').getAttribute('data-id');
          var bIdx = parseInt(cb.getAttribute('data-block'), 10);
          var iIdx = parseInt(cb.getAttribute('data-item'), 10);
          toggleCheck(noteId, bIdx, iIdx);
        });
      });

      // Add an item to a legacy checklist block.
      notesList.querySelectorAll('.note-add-check-item').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          var bIdx = parseInt(btn.getAttribute('data-block'), 10);
          var note = findNote(noteId);
          if (note && note.content[bIdx] && note.content[bIdx].type === 'checklist') {
            // One save carries what was typed and the new, empty line.
            keepEditingInputs();
            note.content[bIdx].items.push({ text: '', checked: false });
            if (autosaveTimer) { clearTimeout(autosaveTimer); autosaveTimer = null; }
            notesDirty = false;
            saveEditingNote(noteId).then(function () {
              renderNotes();
              var inputs = notesList.querySelectorAll('.note-card[data-id="' + noteId + '"] .note-check-text-input[data-block="' + bIdx + '"]');
              if (inputs.length) inputs[inputs.length - 1].focus();
            });
          }
        });
      });

      // A checklist in the rich text, the same kind the Journal writes.
      notesList.querySelectorAll('.note-add-tasks').forEach(function (btn) {
        btn.addEventListener('mousedown', function (e) { e.preventDefault(); });
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var ed = miniEditors[state.editingId];
          if (ed && ed.chain().toggleTaskList) ed.chain().focus().toggleTaskList().run();
        });
      });

      // A picture in the rich text. Pasting or dropping one works too.
      notesList.querySelectorAll('.note-add-picture').forEach(function (btn) {
        btn.addEventListener('mousedown', function (e) { e.preventDefault(); });
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var ed = miniEditors[state.editingId];
          if (ed && Chronicle.EditorImage) Chronicle.EditorImage.pickAndInsert(ed, campaignId);
        });
      });

      // A diagram in the rich text (Mermaid text, drawn beside it).
      notesList.querySelectorAll('.note-add-diagram').forEach(function (btn) {
        btn.addEventListener('mousedown', function (e) { e.preventDefault(); });
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var ed = miniEditors[state.editingId];
          if (ed && Chronicle.EditorDiagram) Chronicle.EditorDiagram.insert(ed);
        });
      });

      // Journal references open in the Journal.
      notesList.querySelectorAll('a.note-link').forEach(function (a) {
        a.addEventListener('click', function () { flushAutosave(); });
      });
    }

    /** Initialize a share popover: wire the four audiences and load members. */
    function initSharePopover(popover) {
      var noteId = popover.getAttribute('data-note-id');
      var note = findNote(noteId);
      if (!note || popover._wired) {
        if (note && popover.querySelector('input[value="specific"]:checked')) loadShareMembers(popover, noteId);
        return;
      }
      popover._wired = true;

      popover.querySelectorAll('input[type="radio"]').forEach(function (radio) {
        radio.addEventListener('change', function () {
          var val = radio.value;
          var membersDiv = popover.querySelector('.note-share-members');
          if (val === 'specific') {
            if (membersDiv) membersDiv.classList.remove('note-share-hidden');
            loadShareMembers(popover, noteId);
            return;
          }
          if (membersDiv) membersDiv.classList.add('note-share-hidden');
          updateNote(noteId, { visibility: val }).then(function () {
            renderNotes();
          }).catch(function () { Chronicle.notify('Could not change who sees the jot.', 'error'); });
        });
      });

      // If "specific" is already selected, load members now.
      var specificRadio = popover.querySelector('input[value="specific"]');
      if (specificRadio && specificRadio.checked) {
        loadShareMembers(popover, noteId);
      }
    }

    /** Load and render member checkboxes in a share popover. */
    function loadShareMembers(popover, noteId) {
      var membersDiv = popover.querySelector('.note-share-members');
      if (!membersDiv) return;
      var note = findNote(noteId);

      fetchMembers().then(function (members) {
        if (!members || members.length === 0) {
          membersDiv.innerHTML = '<div class="note-share-no-members">No other members</div>';
          return;
        }
        var currentShared = (note && note.visibility === 'custom' && note.sharedWith) || [];
        var html = '';
        members.forEach(function (m) {
          // memberRef fields (handler.go): user_id / username / role.
          var checked = currentShared.indexOf(m.user_id) !== -1 ? ' checked' : '';
          html += '<label class="note-share-member">';
          html += '<input type="checkbox" class="note-share-member-cb" value="' + Chronicle.escapeAttr(m.user_id) + '"' + checked + '>';
          html += ' ' + Chronicle.escapeHtml(m.username || m.user_id);
          html += '</label>';
        });
        membersDiv.innerHTML = html;

        // Each tick shares with the ticked people; none ticked is private.
        membersDiv.querySelectorAll('.note-share-member-cb').forEach(function (cb) {
          cb.addEventListener('change', function () {
            var selected = [];
            membersDiv.querySelectorAll('.note-share-member-cb:checked').forEach(function (c) {
              selected.push(c.value);
            });
            var data = selected.length ? { visibility: 'custom', sharedWith: selected } : { visibility: 'private' };
            updateNote(noteId, data).catch(function () {
              Chronicle.notify('Could not change who sees the jot.', 'error');
            });
          });
        });
      });
    }

    /** Show a brief lock error toast in the notes panel. */
    function showLockError() {
      var toast = document.createElement('div');
      toast.className = 'notes-lock-toast';
      toast.setAttribute('role', 'status');
      toast.textContent = 'This jot is being edited by someone else';
      panel.appendChild(toast);
      setTimeout(function () { toast.remove(); }, 3000);
    }

    /**
     * Saves the jot being edited: its title and checklist lines as typed
     * (kept in state by keepEditingInputs, so this works while the card is
     * off screen) and the editor's text. Returns the save.
     */
    function saveEditingNote(noteId) {
      var note = findNote(noteId);
      if (!note) return Promise.resolve(null);
      if (noteId === state.editingId) keepEditingInputs();

      var updateData = { title: note.title };
      var editor = miniEditors[noteId];
      if (editor) {
        var entryJSON = JSON.stringify(editor.getJSON());
        var entryHTML = editor.getHTML();
        updateData.entry = entryJSON;
        updateData.entryHtml = entryHTML;
        // Update local note state for display after save.
        note.entry = entryJSON;
        note.entryHtml = entryHTML;
      }
      // Legacy checklists live outside the editor.
      if (note.content && note.content.some(function (b) { return b.type === 'checklist'; })) {
        updateData.content = note.content;
      }

      return updateNote(noteId, updateData).catch(function () {
        Chronicle.notify('Could not save the jot.', 'error');
        return null;
      });
    }

    /**
     * Mark the in-progress edit dirty and (re)arm the debounced autosave.
     * Called from every edit surface: the TipTap editor, the title input,
     * and the checklist text inputs.
     */
    function markNoteDirty() {
      notesDirty = true;
      if (autosaveTimer) clearTimeout(autosaveTimer);
      autosaveTimer = setTimeout(flushAutosave, AUTOSAVE_DELAY);
    }

    /**
     * Persist the note currently being edited if it has unsaved changes,
     * then clear the dirty flag and any pending timer. Safe to call from
     * blur, navigation, page-unload, and the Done button — a no-op when
     * nothing is being edited or nothing changed, so it never double-saves.
     */
    function flushAutosave() {
      if (autosaveTimer) {
        clearTimeout(autosaveTimer);
        autosaveTimer = null;
      }
      if (!notesDirty || !state.editingId) return;
      saveEditingNote(state.editingId);
      notesDirty = false;
    }

    /** Drop any pending autosave without saving (fresh edit session). */
    function resetAutosave() {
      if (autosaveTimer) {
        clearTimeout(autosaveTimer);
        autosaveTimer = null;
      }
      notesDirty = false;
    }

    /**
     * Initialize the TipTap editor for the jot in edit mode: the Journal's
     * editor, with [[links]], @mentions and checklists. Populated from the
     * jot's entry, else its HTML, else its legacy text blocks.
     */
    function initMiniEditors() {
      // Destroy stale editors for notes no longer editing.
      Object.keys(miniEditors).forEach(function (noteId) {
        if (noteId !== state.editingId) {
          destroyMiniEditor(noteId);
        }
      });

      if (!state.editingId) return;
      if (!window.TipTap) return; // TipTap bundle not loaded.

      var mount = panel.querySelector('[data-note-editor="' + state.editingId + '"]');
      if (!mount) return;
      var open = miniEditors[state.editingId];
      if (open) {
        // The list was drawn again: the same editor, unsaved text and all,
        // moves into the new card.
        if (open.view && open.view.dom && open.view.dom.parentNode !== mount) mount.appendChild(open.view.dom);
        return;
      }

      var note = findNote(state.editingId);
      if (!note) return;

      var initialContent = null;
      if (note.entry) {
        try {
          initialContent = typeof note.entry === 'string' ? JSON.parse(note.entry) : note.entry;
        } catch (e) {
          initialContent = null;
        }
      }
      if (!initialContent && note.entryHtml) {
        initialContent = note.entryHtml;
      }
      if (!initialContent) {
        initialContent = legacyBlocksToHTML(note);
      }

      loadJournalForLinks();
      var extensions = [
        TipTap.StarterKit.configure({ link: false, underline: false }),
        TipTap.Underline,
        TipTap.Placeholder.configure({ placeholder: 'Write something… type [[ to link a note or a page.' }),
        (Chronicle.MentionLink || TipTap.Link).configure({ openOnClick: false, autolink: true })
      ];
      if (Chronicle.NoteLink) extensions.push(Chronicle.NoteLink.configure({ campaignId: campaignId, onOpen: openInJournal }));
      if (TipTap.TaskList && TipTap.TaskItem) extensions.push(TipTap.TaskList, TipTap.TaskItem.configure({ nested: true }));
      // Pictures are in the schema for everyone, so a note holding one loads.
      if (Chronicle.EditorImage) extensions.push(Chronicle.EditorImage.extension);
      if (Chronicle.EditorDiagram && Chronicle.EditorDiagram.extension) extensions.push(Chronicle.EditorDiagram.extension);
      var pictureProps = Chronicle.EditorImage ? Chronicle.EditorImage.pasteDropProps(function () { return editor; }, campaignId) : {};

      wikiExt = Chronicle.WikiLinkExtension ? Chronicle.WikiLinkExtension({ campaignId: campaignId, notes: linkCandidates }) : null;
      mentionExt = Chronicle.MentionExtension ? Chronicle.MentionExtension({ campaignId: campaignId }) : null;

      var editor = new TipTap.Editor({
        element: mount,
        extensions: extensions,
        editable: true,
        content: initialContent || '<p></p>',
        editorProps: {
          attributes: {
            class: 'prose prose-sm max-w-none focus:outline-none min-h-[60px] p-2 text-fg-body'
          },
          handleKeyDown: function (view, event) {
            if (wikiExt && wikiExt.onKeyDown(null, event)) return true;
            if (mentionExt && mentionExt.onKeyDown(null, event)) return true;
            return false;
          },
          handlePaste: pictureProps.handlePaste,
          handleDrop: pictureProps.handleDrop
        },
        // Autosave: debounce on content changes, flush when the editor
        // loses focus (e.g. the user clicks elsewhere before the timer).
        // Toggling editability also emits an update; only edits count.
        onUpdate: function (p) { if (p.transaction && !p.transaction.docChanged) return; if (wikiExt) wikiExt.onUpdate(p.editor); if (mentionExt) mentionExt.onUpdate(p.editor); markNoteDirty(); },
        onBlur: function () { flushAutosave(); }
      });

      miniEditors[state.editingId] = editor;
      if (Chronicle.EditorImage) Chronicle.EditorImage.useNotePictures(editor, campaignId);
      if (wikiExt) wikiExt.onCreate(editor);
      if (mentionExt) mentionExt.onCreate(editor);
      // New edit session: start clean so a stale flag from a prior note
      // can't trigger a spurious save. Runs once per session — initMiniEditors
      // returns early when the editor already exists.
      resetAutosave();
    }

    /** Destroy a mini TipTap editor instance for a note. */
    function destroyMiniEditor(noteId) {
      if (wikiExt) { wikiExt.onDestroy(); wikiExt = null; }
      if (mentionExt) { mentionExt.onDestroy(); mentionExt = null; }
      if (miniEditors[noteId]) {
        miniEditors[noteId].destroy();
        delete miniEditors[noteId];
      }
    }

    /** Convert legacy text blocks to HTML for TipTap initialization. */
    function legacyBlocksToHTML(note) {
      if (!note.content || note.content.length === 0) return '';
      var html = '';
      note.content.forEach(function (block) {
        if (block.type === 'text' && block.value) {
          block.value.split('\n').forEach(function (line) {
            html += '<p>' + Chronicle.escapeHtml(line || '') + '</p>';
          });
        }
        // Checklist blocks are rendered separately, not in TipTap.
      });
      return html || '<p></p>';
    }

    function findNote(id) {
      for (var i = 0; i < state.pageJots.length; i++) {
        if (state.pageJots[i].id === id) return state.pageJots[i];
      }
      for (var j = 0; j < state.jots.length; j++) {
        if (state.jots[j].id === id) return state.jots[j];
      }
      return null;
    }

    // --- Panel HTML ---

    function buildPanelHTML(eid) {
      var tabsHtml = '<div class="notes-tabs" role="tablist">' +
        (eid ? '<button class="notes-tab notes-tab-active" data-tab="page" role="tab" aria-selected="true">This page</button>' : '') +
        '<button class="notes-tab' + (eid ? '' : ' notes-tab-active') + '" data-tab="mine" role="tab" aria-selected="' + (eid ? 'false' : 'true') + '">All my jots</button>' +
        '</div>';

      var refsHtml = eid
        ? '<details class="jot-refs"><summary><i class="fa-solid fa-link" aria-hidden="true"></i> <span class="jot-refs-count">0</span> <span class="jot-refs-noun">Journal notes reference this page</span></summary>' +
          '<div class="jot-refs-list"></div></details>'
        : '';

      var footHtml = eid
        ? '<div class="jot-foot"><button class="jot-add-btn" type="button"><i class="fa-solid fa-plus" aria-hidden="true"></i> New jot on this page</button></div>'
        : '<div class="notes-foot-hint">Jots live on pages: open one to add a jot there.</div>';

      return '<div class="notes-resize-handle" title="Drag to resize"></div>' +
        '<div class="notes-header">' +
        '<i class="fa-solid fa-note-sticky notes-header-icon" aria-hidden="true"></i>' +
        '<span class="notes-header-title">Jot notes</span>' +
        '<div class="notes-header-actions">' +
        '<button class="note-btn notes-pin-panel" type="button" aria-pressed="false"><i class="fa-solid fa-thumbtack"></i></button>' +
        '<button class="note-btn notes-collapse-btn" type="button" title="Collapse" aria-expanded="true"><i class="fa-solid fa-chevron-down"></i></button>' +
        '<button class="note-btn notes-settings-btn" type="button" title="Settings"><i class="fa-solid fa-gear"></i></button>' +
        '<button class="note-btn notes-close" type="button" title="Close" aria-label="Close jot notes"><i class="fa-solid fa-xmark"></i></button>' +
        '</div>' +
        '<div class="notes-settings-popover notes-settings-hidden">' +
        '<div class="notes-settings-label">Text Size</div>' +
        '<div class="notes-settings-sizes">' +
        '<button class="notes-size-opt" data-size="sm">S</button>' +
        '<button class="notes-size-opt" data-size="md">M</button>' +
        '<button class="notes-size-opt" data-size="lg">L</button>' +
        '</div>' +
        '</div>' +
        '</div>' +
        '<div class="notes-body">' +
        tabsHtml +
        refsHtml +
        '<div class="notes-search"><input type="text" class="notes-search-input" placeholder="Filter jots…" aria-label="Filter jots" autocomplete="off"></div>' +
        '<div class="notes-list"></div>' +
        footHtml +
        '</div>';
    }

    // Journal references in the refs box open in place when the Journal is up.
    if (refsBox) {
      refsBox.addEventListener('click', function (e) {
        var a = e.target.closest('[data-open-note]');
        if (!a) return;
        if (Chronicle.openJournalNote && Chronicle.openJournalNote(a.getAttribute('data-open-note'))) e.preventDefault();
      });
    }

    // --- hx-boost navigation sync ---
    // The panel is outside #main-content, so it persists across boosted
    // navigations. Re-mount it with the new page when the URL changes.
    function onNavigated() {
      // SPA navigation tears the widget down; flush any unsaved edit first.
      flushAutosave();
      var newEntityId = extractEntityIdFromUrl();
      if (newEntityId !== entityId) {
        if (newEntityId) {
          el.setAttribute('data-entity-id', newEntityId);
        } else {
          el.removeAttribute('data-entity-id');
        }
        Chronicle.destroyWidget(el);
        Chronicle.mountWidgets(el.parentElement || document);
      }
    }

    /**
     * Extract entity ID from the current URL.
     * Matches /campaigns/{id}/entities/{eid}[/...].
     */
    function extractEntityIdFromUrl() {
      var parts = window.location.pathname.split('/');
      if (parts.length >= 5 && parts[3] === 'entities' &&
          parts[4] !== 'new' && parts[4] !== 'search' && parts[4] !== '') {
        return parts[4];
      }
      return '';
    }

    window.addEventListener('chronicle:navigated', onNavigated);

    // Best-effort save if the tab is closed / reloaded mid-edit. The PUT
    // may not complete during unload (same caveat as the lock release
    // below), but it costs nothing and rescues the common case.
    window.addEventListener('beforeunload', flushAutosave);

    // A pinned panel reopens on the next page; otherwise just count this page.
    if (embedded || isPinnedOpen()) openPanel(); else loadCount();

    // Store references for cleanup.
    el._notesState = state;
    el._notesFab = fab;
    el._notesPanel = panel;
    el._notesMiniEditors = miniEditors;
    el._notesNavHandler = onNavigated;
    el._notesBeforeUnload = flushAutosave;
    el._notesOnCreated = _onNoteCreated;
    el._notesOnOpenNote = _onOpenNote;
    el._notesViewport = window.visualViewport ? onViewport : null;
    el._notesLinksOff = function () {
      if (noteLinksOff) noteLinksOff();
      noteLinksOff = null;
      Object.keys(miniEditors).forEach(destroyMiniEditor);
    };
    el._notesRelease = releaseLockIfHeld;
    el._notesLive = live;
  },

  /**
   * Clean up the jot panel.
   * @param {HTMLElement} el - Mount point element.
   */
  destroy: function (el) {
    // Flush any unsaved edit, then drop the page-unload autosave listener.
    if (el._notesBeforeUnload) {
      el._notesBeforeUnload();
      window.removeEventListener('beforeunload', el._notesBeforeUnload);
      delete el._notesBeforeUnload;
    }
    document.removeEventListener('pointermove', el._notesPointerMove);
    document.removeEventListener('pointerup', el._notesPointerUp);
    document.removeEventListener('click', el._notesDocClick);
    if (el._notesViewport && window.visualViewport) {
      window.visualViewport.removeEventListener('resize', el._notesViewport);
    }
    // Remove hx-boost navigation handler.
    if (el._notesNavHandler) {
      window.removeEventListener('chronicle:navigated', el._notesNavHandler);
      delete el._notesNavHandler;
    }
    // Remove external event listeners.
    if (el._notesOnCreated) {
      window.removeEventListener('chronicle:note-created', el._notesOnCreated);
      delete el._notesOnCreated;
    }
    if (el._notesOnOpenNote) {
      window.removeEventListener('chronicle:open-note', el._notesOnOpenNote);
      delete el._notesOnOpenNote;
    }
    if (el._notesLive) {
      el._notesLive.stop();
      delete el._notesLive;
    }
    // Hand back a held edit lock and stop its heartbeat.
    if (el._notesRelease) {
      el._notesRelease();
      delete el._notesRelease;
    }
    // Label subscriptions and the editors (and their pickers) go too.
    if (el._notesLinksOff) {
      el._notesLinksOff();
      delete el._notesLinksOff;
    }
    delete el._notesMiniEditors;
    if (el._notesFab) el._notesFab.remove();
    if (el._notesPanel) el._notesPanel.remove();
    delete el._notesState;
    delete el._notesFab;
    delete el._notesPanel;
  }
});
