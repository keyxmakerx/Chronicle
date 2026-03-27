/**
 * notes.js -- Floating Notes Panel Widget
 *
 * Quick-capture note-taking panel with two modes:
 *   - "Page" mode: auto-selected when on an entity page, shows notes for that page.
 *   - "All" mode: campaign-wide notes, always available.
 *
 * Features:
 *   - Quick capture: type and Enter to create instantly.
 *   - Shared notes: toggle sharing; other campaign members see shared notes.
 *   - Pessimistic edit locking: shared notes acquire a lock before editing,
 *     with 2-minute heartbeat to keep it alive (5-minute server expiry).
 *   - Version history: view and restore previous snapshots of a note.
 *   - Rich text display: renders entryHtml when present (server-sanitized).
 *
 * Mount: <div data-widget="notes" data-campaign-id="..." data-entity-id="...">
 *
 * The widget is fully self-contained: it creates its own DOM, fetches data
 * from the API, and manages state internally.
 */
Chronicle.register('notes', {
  /**
   * Initialize the notes widget.
   * @param {HTMLElement} el - Mount point element.
   * @param {Object} config - Parsed data-* attributes.
   */
  init: function (el, config) {
    var campaignId = config.campaignId || '';
    var entityId = config.entityId || '';
    var currentUserId = config.userId || '';

    var HEARTBEAT_INTERVAL = 2 * 60 * 1000; // 2 minutes

    var STORAGE_COLLAPSED = 'chronicle_notes_collapsed';

    var state = {
      open: false,
      tab: entityId ? 'page' : 'all',  // 'page' or 'all'
      notes: [],
      pageNotes: [],
      editingId: null,
      loading: true,
      searchFilter: '',
      // Locking state.
      lockHeartbeatTimer: null,
      lockedNoteId: null,       // note we currently hold a lock on
      // Version history sub-panel.
      versionsNoteId: null,     // note whose history is shown (null = hidden)
      versions: [],
      versionsLoading: false,
      // Folder collapse state: Set of folder IDs that are collapsed.
      collapsedFolders: loadCollapsedFolders(),
      // Cached campaign members for share picker.
      members: null,
      membersLoading: false
    };

    // Track mini TipTap editor instances per note ID for cleanup.
    var miniEditors = {};

    // --- DOM Construction ---

    // Floating button (minimized state).
    var fab = document.createElement('button');
    fab.className = 'notes-fab';
    fab.innerHTML = '<i class="fa-solid fa-note-sticky"></i>';
    fab.title = 'Notes';
    fab.setAttribute('aria-label', 'Toggle notes panel');

    // Panel container.
    var panel = document.createElement('div');
    panel.className = 'notes-panel notes-panel-hidden';
    panel.innerHTML = buildPanelHTML(entityId);

    el.appendChild(fab);
    el.appendChild(panel);

    // Hide the floating FAB if the topbar has a notes trigger button.
    // The topbar button calls Chronicle.toggleNotes() which clicks the FAB.
    if (document.getElementById('topbar-notes-trigger')) {
      fab.classList.add('notes-fab-hidden');
    }

    // --- Saved preferences (localStorage) ---
    var STORAGE_KEY = 'chronicle_notes_size';
    var STORAGE_TEXT_SIZE = 'chronicle_notes_text_size';
    var TEXT_SIZE_DEFAULT = 'md';

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
      // Update the active state on the size option buttons.
      panel.querySelectorAll('.notes-size-opt').forEach(function (btn) {
        btn.classList.toggle('notes-size-opt-active', btn.getAttribute('data-size') === size);
      });
      try { localStorage.setItem(STORAGE_TEXT_SIZE, size); } catch (e) { /* ignore */ }
    }

    restoreTextSize();

    /** Load collapsed folder IDs from localStorage. */
    function loadCollapsedFolders() {
      try {
        var saved = JSON.parse(localStorage.getItem(STORAGE_COLLAPSED));
        if (Array.isArray(saved)) return new Set(saved);
      } catch (e) { /* ignore */ }
      return new Set();
    }

    /** Persist collapsed folder IDs to localStorage. */
    function saveCollapsedFolders() {
      try {
        localStorage.setItem(STORAGE_COLLAPSED, JSON.stringify(Array.from(state.collapsedFolders)));
      } catch (e) { /* ignore */ }
    }

    /** Toggle a folder's collapsed state. */
    function toggleFolderCollapse(folderId) {
      if (state.collapsedFolders.has(folderId)) {
        state.collapsedFolders.delete(folderId);
      } else {
        state.collapsedFolders.add(folderId);
      }
      saveCollapsedFolders();
      renderNotes();
    }

    // --- Resize handle ---
    var resizeHandle = panel.querySelector('.notes-resize-handle');
    if (resizeHandle) {
      var resizing = false;
      var startX, startY, startW, startH;

      resizeHandle.addEventListener('mousedown', function (e) {
        e.preventDefault();
        resizing = true;
        startX = e.clientX;
        startY = e.clientY;
        startW = panel.offsetWidth;
        startH = panel.offsetHeight;
        document.body.style.userSelect = 'none';
      });

      document.addEventListener('mousemove', function (e) {
        if (!resizing) return;
        // Dragging top-left corner: moving left increases width, moving up increases height.
        var newW = Math.max(280, startW - (e.clientX - startX));
        var newH = Math.max(300, startH - (e.clientY - startY));
        panel.style.width = newW + 'px';
        panel.style.height = newH + 'px';
      });

      document.addEventListener('mouseup', function () {
        if (!resizing) return;
        resizing = false;
        document.body.style.userSelect = '';
        saveSize();
      });

      // Touch support for mobile resize (if ever used on tablet).
      resizeHandle.addEventListener('touchstart', function (e) {
        var touch = e.touches[0];
        resizing = true;
        startX = touch.clientX;
        startY = touch.clientY;
        startW = panel.offsetWidth;
        startH = panel.offsetHeight;
      }, { passive: true });

      document.addEventListener('touchmove', function (e) {
        if (!resizing) return;
        var touch = e.touches[0];
        var newW = Math.max(280, startW - (touch.clientX - startX));
        var newH = Math.max(300, startH - (touch.clientY - startY));
        panel.style.width = newW + 'px';
        panel.style.height = newH + 'px';
      }, { passive: true });

      document.addEventListener('touchend', function () {
        if (!resizing) return;
        resizing = false;
        saveSize();
      });
    }

    // Cache panel elements.
    var headerTitle = panel.querySelector('.notes-header-title');
    var closeBtn = panel.querySelector('.notes-close');
    var tabBtns = panel.querySelectorAll('.notes-tab');
    var quickInput = panel.querySelector('.notes-quick-input');
    var notesList = panel.querySelector('.notes-list');
    var searchInput = panel.querySelector('.notes-search-input');

    // --- Event Handlers ---

    fab.addEventListener('click', function () {
      state.open = true;
      panel.classList.remove('notes-panel-hidden');
      fab.classList.add('notes-fab-hidden');
      loadNotes();
      setTimeout(function () { if (quickInput) quickInput.focus(); }, 100);
    });

    // Expose global toggle for the topbar notes button.
    Chronicle.toggleNotes = function () {
      if (state.open) {
        closeBtn.click();
      } else {
        fab.click();
      }
    };

    closeBtn.addEventListener('click', function () {
      state.open = false;
      panel.classList.add('notes-panel-hidden');
      fab.classList.remove('notes-fab-hidden');
      // Release any held lock when closing the panel.
      releaseLockIfHeld();
      state.editingId = null;
      state.versionsNoteId = null;
    });

    // New folder button.
    var newFolderBtn = panel.querySelector('.notes-new-folder-btn');
    if (newFolderBtn) {
      newFolderBtn.addEventListener('click', function () {
        createFolder();
      });
    }

    // On mobile, dodge the virtual keyboard by adjusting panel bottom offset
    // when the visual viewport shrinks (keyboard opening).
    if (window.visualViewport && window.innerWidth < 640) {
      window.visualViewport.addEventListener('resize', function () {
        if (!state.open) return;
        var kbHeight = window.innerHeight - window.visualViewport.height;
        panel.style.bottom = (kbHeight > 0 ? kbHeight : 0) + 'px';
      });
    }

    // Quick-add: Enter creates note instantly.
    if (quickInput) {
      quickInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault();
          var text = quickInput.value.trim();
          if (!text) return;
          quickInput.value = '';
          quickCreateNote(text);
        }
      });
    }

    // Search filter: re-render on input with debounce.
    if (searchInput) {
      var searchTimer = null;
      searchInput.addEventListener('input', function () {
        clearTimeout(searchTimer);
        searchTimer = setTimeout(function () {
          state.searchFilter = searchInput.value.trim().toLowerCase();
          renderNotes();
        }, 150);
      });
    }

    // Tab switching.
    tabBtns.forEach(function (btn) {
      btn.addEventListener('click', function () {
        state.tab = btn.getAttribute('data-tab');
        tabBtns.forEach(function (b) { b.classList.remove('notes-tab-active'); });
        btn.classList.add('notes-tab-active');
        updateQuickPlaceholder();
        renderNotes();
      });
    });

    // Settings gear -- toggle popover.
    var settingsBtn = panel.querySelector('.notes-settings-btn');
    var settingsPopover = panel.querySelector('.notes-settings-popover');
    if (settingsBtn && settingsPopover) {
      settingsBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        settingsPopover.classList.toggle('notes-settings-hidden');
      });

      // Size option buttons.
      settingsPopover.querySelectorAll('.notes-size-opt').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          applyTextSize(btn.getAttribute('data-size'));
        });
      });

      // Close popover and move menus when clicking outside.
      document.addEventListener('click', function (e) {
        if (!settingsPopover.classList.contains('notes-settings-hidden') &&
            !settingsPopover.contains(e.target) &&
            e.target !== settingsBtn && !settingsBtn.contains(e.target)) {
          settingsPopover.classList.add('notes-settings-hidden');
        }
        // Close any open move-to-folder menus.
        panel.querySelectorAll('.note-move-menu:not(.note-move-hidden)').forEach(function (m) {
          if (!m.contains(e.target) && !m.previousElementSibling.contains(e.target)) {
            m.classList.add('note-move-hidden');
          }
        });
        // Close any open share popovers.
        panel.querySelectorAll('.note-share-popover:not(.note-share-hidden)').forEach(function (p) {
          if (!p.contains(e.target)) {
            var shareBtn = p.previousElementSibling;
            if (!shareBtn || !shareBtn.contains(e.target)) {
              p.classList.add('note-share-hidden');
            }
          }
        });
      });
    }

    // --- External Events ---
    // Listen for note-created events (from quick capture modal) to refresh.
    var _onNoteCreated = function () { if (state.open) loadNotes(); };
    window.addEventListener('chronicle:note-created', _onNoteCreated);

    // Listen for open-note events (from search modal / session journal) to
    // open the panel and scroll to a specific note.
    var _onOpenNote = function (e) {
      var noteId = e.detail && e.detail.noteId;
      if (!noteId) return;
      // Open panel if closed.
      if (!state.open) {
        state.open = true;
        panel.classList.remove('notes-panel-hidden');
        fab.classList.add('notes-fab-hidden');
      }
      // Switch to "All" tab and reload notes, then highlight the target note.
      state.tab = 'all';
      tabBtns.forEach(function (b) {
        b.classList.toggle('notes-tab-active', b.getAttribute('data-tab') === 'all');
      });
      loadNotes();
      // After reload, try to scroll to and highlight the note.
      setTimeout(function () {
        var noteEl = panel.querySelector('[data-note-id="' + noteId + '"]');
        if (noteEl) {
          noteEl.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
          noteEl.classList.add('note-highlight');
          setTimeout(function () { noteEl.classList.remove('note-highlight'); }, 2000);
        }
      }, 500);
    };
    window.addEventListener('chronicle:open-note', _onOpenNote);

    // --- API Functions ---

    function apiUrl(path) {
      return '/campaigns/' + campaignId + '/notes' + (path || '');
    }

    function loadNotes() {
      state.loading = true;
      renderNotes();

      var promises = [
        Chronicle.apiFetch(apiUrl('?scope=all')).then(function (r) { return r.ok ? r.json() : []; })
      ];

      if (entityId) {
        promises.push(
          Chronicle.apiFetch(apiUrl('?scope=entity&entity_id=' + entityId)).then(function (r) { return r.ok ? r.json() : []; })
        );
      }

      Promise.all(promises).then(function (results) {
        state.notes = results[0] || [];
        state.pageNotes = results[1] || [];
        state.loading = false;
        renderNotes();
      }).catch(function () {
        state.loading = false;
        state.notes = [];
        state.pageNotes = [];
        renderNotes();
      });
    }

    /** Quick-create: one-step note from the quick-add input. */
    function quickCreateNote(text) {
      var isPageNote = state.tab === 'page' && entityId;
      var body = {
        title: text,
        content: [{ type: 'text', value: '' }]
      };
      if (isPageNote) {
        body.entityId = entityId;
      }

      Chronicle.apiFetch(apiUrl(), {
        method: 'POST',
        body: body
      }).then(function (r) { return r.json(); })
        .then(function (note) {
          if (isPageNote) {
            state.pageNotes.unshift(note);
          }
          state.notes.unshift(note);
          renderNotes();
        })
        .catch(function (err) {
          console.error('[notes] Failed to create note:', err);
          Chronicle.notify('Failed to save note', 'error');
          renderNotes();
        });
    }

    /** Full create with editing mode (from + button). */
    function createNote(parentId) {
      var isPageNote = state.tab === 'page' && entityId;
      var body = {
        title: '',
        content: [{ type: 'text', value: '' }]
      };
      if (isPageNote) {
        body.entityId = entityId;
      }
      if (parentId) {
        body.parentId = parentId;
      }

      Chronicle.apiFetch(apiUrl(), {
        method: 'POST',
        body: body
      }).then(function (r) { return r.json(); })
        .then(function (note) {
          if (isPageNote) {
            state.pageNotes.unshift(note);
          }
          state.notes.unshift(note);
          state.editingId = note.id;
          // Expand parent folder if creating inside one.
          if (parentId) {
            state.collapsedFolders.delete(parentId);
            saveCollapsedFolders();
          }
          renderNotes();
          var titleInput = notesList.querySelector('.note-card[data-id="' + note.id + '"] .note-title-input');
          if (titleInput) titleInput.focus();
        });
    }

    /** Create a new folder. */
    function createFolder(parentId) {
      var name = prompt('Folder name:');
      if (!name || !name.trim()) return;

      var isPageNote = state.tab === 'page' && entityId;
      var body = {
        title: name.trim(),
        isFolder: true,
        content: []
      };
      if (isPageNote) {
        body.entityId = entityId;
      }
      if (parentId) {
        body.parentId = parentId;
      }

      Chronicle.apiFetch(apiUrl(), {
        method: 'POST',
        body: body
      }).then(function (r) { return r.json(); })
        .then(function (folder) {
          if (isPageNote) {
            state.pageNotes.unshift(folder);
          }
          state.notes.unshift(folder);
          if (parentId) {
            state.collapsedFolders.delete(parentId);
            saveCollapsedFolders();
          }
          renderNotes();
        });
    }

    /** Move a note into or out of a folder. */
    function moveNote(noteId, newParentId) {
      var data = { parentId: newParentId || '' };
      updateNote(noteId, data).then(function () {
        renderNotes();
      });
    }

    function updateNote(id, data) {
      return Chronicle.apiFetch(apiUrl('/' + id), {
        method: 'PUT',
        body: data
      }).then(function (r) { return r.json(); })
        .then(function (updated) {
          replaceNoteInState(updated);
          return updated;
        });
    }

    function deleteNote(id) {
      var note = findNote(id);
      var msg = note && note.isFolder
        ? 'Delete this folder and all notes inside it? This cannot be undone.'
        : 'Delete this note? This cannot be undone.';
      if (!confirm(msg)) return;
      // Release lock before deleting if we hold one.
      if (state.lockedNoteId === id) {
        releaseLockIfHeld();
      }
      Chronicle.apiFetch(apiUrl('/' + id), {
        method: 'DELETE'
      }).then(function () {
        state.notes = state.notes.filter(function (n) { return n.id !== id; });
        state.pageNotes = state.pageNotes.filter(function (n) { return n.id !== id; });
        if (state.editingId === id) state.editingId = null;
        renderNotes();
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
      state.notes = state.notes.map(function (n) { return n.id === updated.id ? updated : n; });
      state.pageNotes = state.pageNotes.map(function (n) { return n.id === updated.id ? updated : n; });
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
          // Exclude the current user from the picker.
          state.members = (members || []).filter(function (m) {
            return m.id !== currentUserId;
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

    // --- Rendering ---

    function updateQuickPlaceholder() {
      if (!quickInput) return;
      quickInput.placeholder = state.tab === 'page'
        ? 'Quick note for this page...'
        : 'Quick note...';
    }

    /**
     * Build a tree structure from a flat list of notes.
     * Returns array of root-level items, each with a `children` array.
     */
    function buildTree(notes) {
      var byId = {};
      var roots = [];
      // Index all notes by ID.
      notes.forEach(function (n) { byId[n.id] = Object.assign({}, n, { children: [] }); });
      // Assign children to parents.
      notes.forEach(function (n) {
        var node = byId[n.id];
        if (n.parentId && byId[n.parentId]) {
          byId[n.parentId].children.push(node);
        } else {
          roots.push(node);
        }
      });
      // Sort: folders first, then pinned, then by updatedAt.
      var sortFn = function (a, b) {
        if (a.isFolder !== b.isFolder) return a.isFolder ? -1 : 1;
        if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
        return 0;
      };
      roots.sort(sortFn);
      Object.keys(byId).forEach(function (id) { byId[id].children.sort(sortFn); });
      return roots;
    }

    /** Get all folders from the current note list (for move-to dropdown). */
    function getFolders(list) {
      return (list || []).filter(function (n) { return n.isFolder; });
    }

    function renderNotes() {
      // If version history sub-panel is open, render that instead.
      if (state.versionsNoteId) {
        renderVersionsPanel();
        return;
      }

      var list = state.tab === 'page' ? state.pageNotes : state.notes;
      if (headerTitle) {
        headerTitle.textContent = state.tab === 'page' ? 'Page Notes' : 'All Notes';
      }

      if (state.loading) {
        notesList.innerHTML = '<div class="notes-empty"><i class="fa-solid fa-spinner fa-spin"></i> Loading...</div>';
        return;
      }

      if (!list || list.length === 0) {
        var emptyMsg = state.tab === 'page'
          ? 'No notes for this page yet'
          : 'No notes yet';
        notesList.innerHTML = '<div class="notes-empty">' + Chronicle.escapeHtml(emptyMsg) + '</div>';
        return;
      }

      // Apply search filter: keep notes whose title matches, plus any
      // folders that contain matching children.
      var filtered = list;
      if (state.searchFilter) {
        var q = state.searchFilter;
        // Collect IDs of notes that match the query.
        var matchIds = new Set();
        list.forEach(function (n) {
          if (!n.isFolder && n.title && n.title.toLowerCase().indexOf(q) !== -1) {
            matchIds.add(n.id);
            // Also include the parent folder so the tree stays intact.
            if (n.parentId) matchIds.add(n.parentId);
          }
        });
        filtered = list.filter(function (n) {
          return matchIds.has(n.id);
        });
        if (filtered.length === 0) {
          notesList.innerHTML = '<div class="notes-empty">No matching notes</div>';
          return;
        }
      }

      var tree = buildTree(filtered);
      var html = '';
      tree.forEach(function (node) {
        html += renderTreeNode(node, 0, list);
      });
      notesList.innerHTML = html;

      bindCardEvents();
      initMiniEditors();
    }

    /** Render a tree node (folder or note) at a given depth. */
    function renderTreeNode(node, depth, allNotes) {
      if (node.isFolder) {
        return renderFolderCard(node, depth, allNotes);
      }
      return renderNoteCard(node, depth, allNotes);
    }

    /** Render a folder as a collapsible container with children. */
    function renderFolderCard(folder, depth, allNotes) {
      var isOwner = folder.userId === currentUserId;
      var isCollapsed = state.collapsedFolders.has(folder.id);
      var indent = depth > 0 ? ' style="margin-left:' + (depth * 12) + 'px"' : '';
      var chevron = isCollapsed ? 'fa-chevron-right' : 'fa-chevron-down';
      var childCount = folder.children ? folder.children.length : 0;

      var html = '<div class="note-folder"' + indent + ' data-folder-id="' + Chronicle.escapeAttr(folder.id) + '">';
      html += '<div class="note-folder-header" data-id="' + Chronicle.escapeAttr(folder.id) + '">';
      html += '<button class="note-btn note-folder-toggle" data-folder="' + Chronicle.escapeAttr(folder.id) + '" title="' + (isCollapsed ? 'Expand' : 'Collapse') + '">';
      html += '<i class="fa-solid ' + chevron + ' text-[10px]"></i></button>';
      html += '<i class="fa-solid fa-folder' + (isCollapsed ? '' : '-open') + ' text-[11px] text-fg-muted mr-1"></i>';
      html += '<span class="note-folder-name">' + Chronicle.escapeHtml(folder.title || 'Untitled Folder') + '</span>';
      html += '<span class="note-folder-count text-fg-muted text-[10px] ml-1">(' + childCount + ')</span>';
      html += '<div class="note-actions">';
      // Add note inside folder.
      html += '<button class="note-btn note-add-in-folder" data-folder="' + Chronicle.escapeAttr(folder.id) + '" title="Add note in folder"><i class="fa-solid fa-plus text-[10px]"></i></button>';
      // Rename folder.
      if (isOwner) {
        html += '<button class="note-btn note-rename-folder" data-folder="' + Chronicle.escapeAttr(folder.id) + '" title="Rename folder"><i class="fa-solid fa-pen text-[10px]"></i></button>';
      }
      // Delete folder (owner only).
      if (isOwner) {
        html += '<button class="note-btn note-delete-btn" title="Delete folder"><i class="fa-solid fa-trash-can text-[10px]"></i></button>';
      }
      html += '</div></div>';

      // Children (hidden when collapsed).
      if (!isCollapsed && folder.children && folder.children.length > 0) {
        html += '<div class="note-folder-children">';
        folder.children.forEach(function (child) {
          html += renderTreeNode(child, depth + 1, allNotes);
        });
        html += '</div>';
      }

      html += '</div>';
      return html;
    }

    function renderNoteCard(note, depth, allNotes) {
      var isEditing = state.editingId === note.id;
      var isOwner = note.userId === currentUserId;
      var isShared = note.isShared;
      var hasSharedWith = note.sharedWith && note.sharedWith.length > 0;
      var isSharedAny = isShared || hasSharedWith;
      var isLockedByOther = isSharedAny && note.lockedBy && note.lockedBy !== currentUserId && note.lockedAt;
      var pinClass = note.pinned ? ' note-pinned' : '';
      var sharedClass = isSharedAny ? ' note-shared' : '';
      var indent = depth > 0 ? ' style="margin-left:' + (depth * 12) + 'px"' : '';
      var html = '<div class="note-card' + pinClass + sharedClass + '"' + indent + ' data-id="' + Chronicle.escapeAttr(note.id) + '">';

      // Header row.
      html += '<div class="note-card-header">';
      if (isEditing) {
        html += '<input type="text" class="note-title-input" value="' + Chronicle.escapeAttr(note.title === 'Untitled' ? '' : note.title) + '" placeholder="Note title...">';
      } else {
        html += '<span class="note-title">' + Chronicle.escapeHtml(note.title) + '</span>';
      }
      html += '<div class="note-actions">';

      // Shared badge (non-owners see a simple indicator).
      if (isSharedAny && !isOwner) {
        var shareTitle = hasSharedWith ? 'Shared with you' : 'Shared note';
        html += '<span class="note-shared-badge" title="' + shareTitle + '"><i class="fa-solid fa-users text-[9px]"></i></span>';
      }

      // Share button (owner only) — opens sharing popover.
      if (isOwner) {
        var shareIcon = isSharedAny ? 'fa-lock-open' : 'fa-share-nodes';
        var shareLabel = isShared ? 'Everyone' : hasSharedWith ? note.sharedWith.length + ' player(s)' : 'Private';
        html += '<div class="note-share-wrap">';
        html += '<button class="note-btn note-share-btn" title="Sharing: ' + shareLabel + '">' +
          '<i class="fa-solid ' + shareIcon + ' text-[10px]"></i></button>';
        html += '<div class="note-share-popover note-share-hidden" data-note-id="' + Chronicle.escapeAttr(note.id) + '">';
        html += '<div class="note-share-opts">';
        html += '<label class="note-share-opt"><input type="radio" name="share-' + Chronicle.escapeAttr(note.id) + '" value="private"' + (!isSharedAny ? ' checked' : '') + '> Private</label>';
        html += '<label class="note-share-opt"><input type="radio" name="share-' + Chronicle.escapeAttr(note.id) + '" value="everyone"' + (isShared ? ' checked' : '') + '> Everyone</label>';
        html += '<label class="note-share-opt"><input type="radio" name="share-' + Chronicle.escapeAttr(note.id) + '" value="specific"' + (hasSharedWith ? ' checked' : '') + '> Specific Players</label>';
        html += '</div>';
        html += '<div class="note-share-members' + (hasSharedWith ? '' : ' note-share-hidden') + '" data-note-id="' + Chronicle.escapeAttr(note.id) + '">';
        html += '<div class="note-share-members-loading"><i class="fa-solid fa-spinner fa-spin text-[10px]"></i> Loading...</div>';
        html += '</div>';
        html += '</div>';
        html += '</div>';
      }

      // Lock indicator (shared notes locked by another user).
      if (isLockedByOther) {
        html += '<span class="note-lock-badge" title="Being edited by another user"><i class="fa-solid fa-lock text-[9px]"></i></span>';
      }

      // Pin button (owner only).
      if (isOwner) {
        html += '<button class="note-btn note-pin-btn" title="' + (note.pinned ? 'Unpin' : 'Pin') + '"><i class="fa-solid fa-thumbtack' + (note.pinned ? '' : ' fa-rotate-45') + '"></i></button>';
      }

      // Move to folder button (owner only, non-folder notes).
      if (isOwner && !note.isFolder) {
        var folders = getFolders(allNotes || []);
        if (folders.length > 0 || note.parentId) {
          html += '<div class="note-move-wrap">';
          html += '<button class="note-btn note-move-btn" title="Move to folder"><i class="fa-solid fa-folder-tree text-[10px]"></i></button>';
          html += '<div class="note-move-menu note-move-hidden">';
          if (note.parentId) {
            html += '<button class="note-move-opt" data-move-to="">— Top level —</button>';
          }
          folders.forEach(function (f) {
            if (f.id !== note.id && f.id !== note.parentId) {
              html += '<button class="note-move-opt" data-move-to="' + Chronicle.escapeAttr(f.id) + '">' + Chronicle.escapeHtml(f.title || 'Untitled') + '</button>';
            }
          });
          html += '</div></div>';
        }
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

      html += '</div></div>';

      // Content blocks.
      html += '<div class="note-card-body">';

      if (isEditing) {
        // TipTap rich text editor mount point (initialized after DOM insertion).
        html += '<div class="note-tiptap-mount" data-note-editor="' + Chronicle.escapeAttr(note.id) + '"></div>';

        // Checklist blocks remain as interactive checkboxes (not TipTap).
        if (note.content && note.content.length > 0) {
          note.content.forEach(function (block, bIdx) {
            if (block.type === 'checklist') {
              html += '<div class="note-checklist" data-block="' + bIdx + '">';
              if (block.items) {
                block.items.forEach(function (item, iIdx) {
                  var checked = item.checked ? ' checked' : '';
                  var strikeClass = item.checked ? ' note-checked' : '';
                  html += '<label class="note-check-item' + strikeClass + '">';
                  html += '<input type="checkbox"' + checked + ' data-block="' + bIdx + '" data-item="' + iIdx + '" class="note-checkbox">';
                  html += '<input type="text" class="note-check-text-input" value="' + Chronicle.escapeAttr(item.text) + '" data-block="' + bIdx + '" data-item="' + iIdx + '" placeholder="List item...">';
                  html += '</label>';
                });
              }
              html += '<button class="note-add-check-item" data-block="' + bIdx + '"><i class="fa-solid fa-plus text-[9px]"></i> Add item</button>';
              html += '</div>';
            }
          });
        }
      } else {
        // Display mode: show rendered HTML or legacy blocks.
        if (note.entryHtml) {
          html += '<div class="note-entry-html">' + note.entryHtml + '</div>';
        } else if (note.content && note.content.length > 0) {
          note.content.forEach(function (block, bIdx) {
            if (block.type === 'text') {
              if (block.value) {
                html += '<p class="note-text">' + Chronicle.escapeHtml(block.value) + '</p>';
              }
            } else if (block.type === 'checklist') {
              html += '<div class="note-checklist" data-block="' + bIdx + '">';
              if (block.items) {
                block.items.forEach(function (item, iIdx) {
                  var checked = item.checked ? ' checked' : '';
                  var strikeClass = item.checked ? ' note-checked' : '';
                  html += '<label class="note-check-item' + strikeClass + '">';
                  html += '<input type="checkbox"' + checked + ' data-block="' + bIdx + '" data-item="' + iIdx + '" class="note-checkbox">';
                  html += '<span>' + Chronicle.escapeHtml(item.text) + '</span>';
                  html += '</label>';
                });
              }
              html += '</div>';
            }
          });
        }
      }

      html += '</div></div>';
      return html;
    }

    /** Render the version history sub-panel. */
    function renderVersionsPanel() {
      var note = findNote(state.versionsNoteId);
      var title = note ? Chronicle.escapeHtml(note.title) : 'Note';

      if (headerTitle) {
        headerTitle.textContent = 'History: ' + (note ? note.title : '');
      }

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

          // If shared note, acquire lock before entering edit mode.
          var noteIsSharedAny = note && (note.isShared || (note.sharedWith && note.sharedWith.length > 0));
          if (noteIsSharedAny) {
            acquireLock(noteId).then(function (locked) {
              if (locked) {
                state.editingId = noteId;
                renderNotes();
              } else {
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
          saveEditingNote(card, noteId);
          state.editingId = null;
          // Release lock if we hold one for this note.
          if (state.lockedNoteId === noteId) {
            releaseLock(noteId);
          }
          renderNotes();
        });
      });

      // Pin button.
      notesList.querySelectorAll('.note-pin-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          var note = findNote(noteId);
          if (note) {
            updateNote(noteId, { pinned: !note.pinned }).then(function () {
              renderNotes();
            });
          }
        });
      });

      // Share button — toggle share popover.
      notesList.querySelectorAll('.note-share-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var popover = btn.nextElementSibling;
          if (!popover) return;
          // Close all other share popovers.
          notesList.querySelectorAll('.note-share-popover').forEach(function (p) {
            if (p !== popover) p.classList.add('note-share-hidden');
          });
          popover.classList.toggle('note-share-hidden');
          if (!popover.classList.contains('note-share-hidden')) {
            initSharePopover(popover);
          }
        });
      });

      // Delete button (works for both notes and folders).
      notesList.querySelectorAll('.note-delete-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var folder = btn.closest('.note-folder-header');
          if (card) {
            deleteNote(card.getAttribute('data-id'));
          } else if (folder) {
            deleteNote(folder.getAttribute('data-id'));
          }
        });
      });

      // History button.
      notesList.querySelectorAll('.note-history-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          loadVersions(noteId);
        });
      });

      // Checkbox toggle (works in both view and edit modes).
      notesList.querySelectorAll('.note-checkbox').forEach(function (cb) {
        cb.addEventListener('change', function () {
          var card = cb.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          var bIdx = parseInt(cb.getAttribute('data-block'), 10);
          var iIdx = parseInt(cb.getAttribute('data-item'), 10);
          toggleCheck(noteId, bIdx, iIdx);
        });
      });

      // Add checklist item button.
      notesList.querySelectorAll('.note-add-check-item').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          var bIdx = parseInt(btn.getAttribute('data-block'), 10);
          var note = findNote(noteId);
          if (note && note.content[bIdx] && note.content[bIdx].type === 'checklist') {
            saveEditingNote(card, noteId);
            note = findNote(noteId);
            note.content[bIdx].items.push({ text: '', checked: false });
            updateNote(noteId, { content: note.content }).then(function () {
              renderNotes();
              var inputs = notesList.querySelectorAll('.note-card[data-id="' + noteId + '"] .note-check-text-input[data-block="' + bIdx + '"]');
              if (inputs.length) inputs[inputs.length - 1].focus();
            });
          }
        });
      });

      // Add text block.
      notesList.querySelectorAll('.note-add-text-block').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          saveEditingNote(card, noteId);
          var note = findNote(noteId);
          if (note) {
            note.content.push({ type: 'text', value: '' });
            updateNote(noteId, { content: note.content }).then(function () {
              renderNotes();
            });
          }
        });
      });

      // Add checklist block.
      notesList.querySelectorAll('.note-add-checklist-block').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          saveEditingNote(card, noteId);
          var note = findNote(noteId);
          if (note) {
            note.content.push({ type: 'checklist', items: [{ text: '', checked: false }] });
            updateNote(noteId, { content: note.content }).then(function () {
              renderNotes();
            });
          }
        });
      });

      // Folder toggle (expand/collapse).
      notesList.querySelectorAll('.note-folder-toggle').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var folderId = btn.getAttribute('data-folder');
          toggleFolderCollapse(folderId);
        });
      });

      // Folder header click also toggles.
      notesList.querySelectorAll('.note-folder-header').forEach(function (hdr) {
        hdr.addEventListener('click', function (e) {
          // Don't toggle if clicking on an action button.
          if (e.target.closest('.note-actions') || e.target.closest('.note-btn')) return;
          var folderId = hdr.getAttribute('data-id');
          toggleFolderCollapse(folderId);
        });
      });

      // Add note inside folder.
      notesList.querySelectorAll('.note-add-in-folder').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var folderId = btn.getAttribute('data-folder');
          createNote(folderId);
        });
      });

      // Rename folder.
      notesList.querySelectorAll('.note-rename-folder').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var folderId = btn.getAttribute('data-folder');
          var folder = findNote(folderId);
          if (!folder) return;
          var newName = prompt('Rename folder:', folder.title);
          if (newName !== null && newName.trim()) {
            updateNote(folderId, { title: newName.trim() }).then(function () {
              renderNotes();
            });
          }
        });
      });

      // Move to folder dropdown toggle.
      notesList.querySelectorAll('.note-move-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var menu = btn.nextElementSibling;
          // Close all other open menus first.
          notesList.querySelectorAll('.note-move-menu').forEach(function (m) {
            if (m !== menu) m.classList.add('note-move-hidden');
          });
          menu.classList.toggle('note-move-hidden');
        });
      });

      // Move option selected.
      notesList.querySelectorAll('.note-move-opt').forEach(function (opt) {
        opt.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = opt.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          var targetId = opt.getAttribute('data-move-to');
          moveNote(noteId, targetId);
        });
      });
    }

    /** Initialize a share popover: wire radio buttons and load members. */
    function initSharePopover(popover) {
      var noteId = popover.getAttribute('data-note-id');
      var note = findNote(noteId);
      if (!note) return;

      // Wire radio buttons.
      popover.querySelectorAll('input[type="radio"]').forEach(function (radio) {
        radio.addEventListener('change', function () {
          var val = radio.value;
          var membersDiv = popover.querySelector('.note-share-members');

          if (val === 'private') {
            if (membersDiv) membersDiv.classList.add('note-share-hidden');
            updateNote(noteId, { isShared: false, sharedWith: [] }).then(function () {
              renderNotes();
            });
          } else if (val === 'everyone') {
            if (membersDiv) membersDiv.classList.add('note-share-hidden');
            updateNote(noteId, { isShared: true, sharedWith: [] }).then(function () {
              renderNotes();
            });
          } else if (val === 'specific') {
            if (membersDiv) membersDiv.classList.remove('note-share-hidden');
            loadShareMembers(popover, noteId);
          }
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
        var currentShared = (note && note.sharedWith) || [];
        var html = '';
        members.forEach(function (m) {
          var checked = currentShared.indexOf(m.id) !== -1 ? ' checked' : '';
          html += '<label class="note-share-member">';
          html += '<input type="checkbox" class="note-share-member-cb" value="' + Chronicle.escapeAttr(m.id) + '"' + checked + '>';
          html += ' ' + Chronicle.escapeHtml(m.name);
          html += '</label>';
        });
        membersDiv.innerHTML = html;

        // Wire checkbox changes to update sharing.
        membersDiv.querySelectorAll('.note-share-member-cb').forEach(function (cb) {
          cb.addEventListener('change', function () {
            var selected = [];
            membersDiv.querySelectorAll('.note-share-member-cb:checked').forEach(function (c) {
              selected.push(c.value);
            });
            updateNote(noteId, { isShared: false, sharedWith: selected }).then(function () {
              // Update local state without full re-render (keeps popover open).
              var n = findNote(noteId);
              if (n) {
                n.sharedWith = selected;
                n.isShared = false;
              }
            });
          });
        });
      });
    }

    /** Show a brief lock error toast in the notes panel. */
    function showLockError() {
      var toast = document.createElement('div');
      toast.className = 'notes-lock-toast';
      toast.textContent = 'This note is being edited by another user';
      panel.appendChild(toast);
      setTimeout(function () { toast.remove(); }, 3000);
    }

    /** Read all editing inputs from a card and save to the API. */
    function saveEditingNote(card, noteId) {
      var note = findNote(noteId);
      if (!note) return;

      var titleInput = card.querySelector('.note-title-input');
      if (titleInput) {
        note.title = titleInput.value.trim() || 'Untitled';
      }

      // Read TipTap editor content if present.
      var editor = miniEditors[noteId];
      var updateData = { title: note.title };

      if (editor) {
        var entryJSON = JSON.stringify(editor.getJSON());
        var entryHTML = editor.getHTML();
        updateData.entry = entryJSON;
        updateData.entryHtml = entryHTML;
        // Update local note state for display after save.
        note.entry = entryJSON;
        note.entryHtml = entryHTML;
      }

      // Read checklist block edits (checklists remain outside TipTap).
      card.querySelectorAll('.note-check-text-input').forEach(function (inp) {
        var bIdx = parseInt(inp.getAttribute('data-block'), 10);
        var iIdx = parseInt(inp.getAttribute('data-item'), 10);
        if (note.content[bIdx] && note.content[bIdx].items && note.content[bIdx].items[iIdx]) {
          note.content[bIdx].items[iIdx].text = inp.value;
        }
      });

      // Include checklist content updates if any checklists exist.
      if (note.content && note.content.some(function (b) { return b.type === 'checklist'; })) {
        updateData.content = note.content;
      }

      updateNote(noteId, updateData);
    }

    /**
     * Initialize mini TipTap editors for notes currently in edit mode.
     * Called after DOM rendering. Creates a TipTap instance in each
     * .note-tiptap-mount element, populated with the note's entry content
     * or converted from legacy text blocks.
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
      if (!mount || miniEditors[state.editingId]) return;

      var note = findNote(state.editingId);
      if (!note) return;

      // Determine initial content: prefer entry JSON, then convert legacy blocks.
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

      var editor = new TipTap.Editor({
        element: mount,
        extensions: [
          TipTap.StarterKit,
          TipTap.Underline,
          TipTap.Placeholder.configure({ placeholder: 'Write something...' })
        ],
        editable: true,
        content: initialContent || '<p></p>',
        editorProps: {
          attributes: {
            class: 'prose prose-sm max-w-none focus:outline-none min-h-[60px] p-2 text-fg-body'
          }
        }
      });

      miniEditors[state.editingId] = editor;
    }

    /** Destroy a mini TipTap editor instance for a note. */
    function destroyMiniEditor(noteId) {
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
          // Convert newlines to paragraphs.
          var lines = block.value.split('\n');
          lines.forEach(function (line) {
            html += '<p>' + Chronicle.escapeHtml(line || '') + '</p>';
          });
        }
        // Checklist blocks are rendered separately, not in TipTap.
      });
      return html || '<p></p>';
    }

    function findNote(id) {
      for (var i = 0; i < state.notes.length; i++) {
        if (state.notes[i].id === id) return state.notes[i];
      }
      for (var j = 0; j < state.pageNotes.length; j++) {
        if (state.pageNotes[j].id === id) return state.pageNotes[j];
      }
      return null;
    }

    // --- Panel HTML ---

    function buildPanelHTML(eid) {
      var tabsHtml = '';
      if (eid) {
        tabsHtml = '<div class="notes-tabs">' +
          '<button class="notes-tab notes-tab-active" data-tab="page">This Page</button>' +
          '<button class="notes-tab" data-tab="all">All Notes</button>' +
          '</div>';
      }

      var quickPlaceholder = eid ? 'Quick note for this page...' : 'Quick note...';

      return '<div class="notes-resize-handle" title="Drag to resize"></div>' +
        '<div class="notes-header">' +
        '<span class="notes-header-title">' + (eid ? 'Page Notes' : 'All Notes') + '</span>' +
        '<div class="notes-header-actions">' +
        '<button class="note-btn notes-settings-btn" title="Settings"><i class="fa-solid fa-gear"></i></button>' +
        '<button class="note-btn notes-close" title="Close"><i class="fa-solid fa-xmark"></i></button>' +
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
        tabsHtml +
        '<div class="notes-quick-add">' +
        '<i class="fa-solid fa-plus text-[10px] text-fg-muted"></i>' +
        '<input type="text" class="notes-quick-input" placeholder="' + Chronicle.escapeAttr(quickPlaceholder) + '" autocomplete="off">' +
        '<button class="note-btn notes-new-folder-btn" title="New folder"><i class="fa-solid fa-folder-plus text-[11px]"></i></button>' +
        '</div>' +
        '<div class="notes-search" style="padding:4px 8px">' +
        '<input type="text" class="notes-search-input" placeholder="Filter notes..." autocomplete="off" style="width:100%;padding:4px 8px;font-size:12px;border:1px solid var(--border-color,#e5e7eb);border-radius:4px;outline:none;background:transparent;color:inherit;">' +
        '</div>' +
        '<div class="notes-list"></div>';
    }

    // --- hx-boost navigation sync ---
    // The notes widget is outside #main-content, so it persists across
    // boosted navigations. Detect entity context changes and re-mount
    // with the correct entity ID when the URL changes.
    function onNavigated() {
      var newEntityId = extractEntityIdFromUrl();
      if (newEntityId !== entityId) {
        // Update the data attribute so re-mount picks up the new entity.
        if (newEntityId) {
          el.setAttribute('data-entity-id', newEntityId);
        } else {
          el.removeAttribute('data-entity-id');
        }
        // Destroy and re-create the widget with the new entity context.
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

    // Store references for cleanup.
    el._notesState = state;
    el._notesFab = fab;
    el._notesPanel = panel;
    el._notesMiniEditors = miniEditors;
    el._notesNavHandler = onNavigated;
    el._notesOnCreated = _onNoteCreated;
    el._notesOnOpenNote = _onOpenNote;
  },

  /**
   * Clean up the notes widget.
   * @param {HTMLElement} el - Mount point element.
   */
  destroy: function (el) {
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
    // Release any held lock and stop heartbeat.
    if (el._notesState) {
      if (el._notesState.lockHeartbeatTimer) {
        clearInterval(el._notesState.lockHeartbeatTimer);
      }
      // Best-effort unlock on destroy (page navigation).
      if (el._notesState.lockedNoteId && el._notesPanel) {
        var campaignId = '';
        var panel = el._notesPanel;
        if (panel && panel.dataset) {
          campaignId = el.dataset.campaignId || '';
        }
        // We can't reliably call the API during page unload, but try anyway.
      }
    }
    // Destroy mini TipTap editors.
    if (el._notesMiniEditors) {
      Object.keys(el._notesMiniEditors).forEach(function (id) {
        if (el._notesMiniEditors[id]) el._notesMiniEditors[id].destroy();
      });
      delete el._notesMiniEditors;
    }
    if (el._notesFab) el._notesFab.remove();
    if (el._notesPanel) el._notesPanel.remove();
    delete el._notesState;
    delete el._notesFab;
    delete el._notesPanel;
  }
});
