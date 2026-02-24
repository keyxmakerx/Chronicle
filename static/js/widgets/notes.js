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
    var csrfToken = '';

    // Read CSRF token from cookie.
    var match = document.cookie.match('(?:^|; )chronicle_csrf=([^;]*)');
    if (match) csrfToken = decodeURIComponent(match[1]);

    var HEARTBEAT_INTERVAL = 2 * 60 * 1000; // 2 minutes

    var state = {
      open: false,
      tab: entityId ? 'page' : 'all',  // 'page' or 'all'
      notes: [],
      pageNotes: [],
      editingId: null,
      loading: true,
      // Locking state.
      lockHeartbeatTimer: null,
      lockedNoteId: null,       // note we currently hold a lock on
      // Version history sub-panel.
      versionsNoteId: null,     // note whose history is shown (null = hidden)
      versions: [],
      versionsLoading: false
    };

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

    // --- Event Handlers ---

    fab.addEventListener('click', function () {
      state.open = true;
      panel.classList.remove('notes-panel-hidden');
      fab.classList.add('notes-fab-hidden');
      loadNotes();
      setTimeout(function () { if (quickInput) quickInput.focus(); }, 100);
    });

    closeBtn.addEventListener('click', function () {
      state.open = false;
      panel.classList.add('notes-panel-hidden');
      fab.classList.remove('notes-fab-hidden');
      // Release any held lock when closing the panel.
      releaseLockIfHeld();
      state.editingId = null;
      state.versionsNoteId = null;
    });

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

      // Close popover when clicking outside.
      document.addEventListener('click', function (e) {
        if (!settingsPopover.classList.contains('notes-settings-hidden') &&
            !settingsPopover.contains(e.target) &&
            e.target !== settingsBtn && !settingsBtn.contains(e.target)) {
          settingsPopover.classList.add('notes-settings-hidden');
        }
      });
    }

    // --- API Functions ---

    function apiUrl(path) {
      return '/campaigns/' + campaignId + '/notes' + (path || '');
    }

    function apiHeaders() {
      return {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        'X-CSRF-Token': csrfToken
      };
    }

    function loadNotes() {
      state.loading = true;
      renderNotes();

      var promises = [
        fetch(apiUrl('?scope=all'), { headers: apiHeaders() }).then(function (r) { return r.ok ? r.json() : []; })
      ];

      if (entityId) {
        promises.push(
          fetch(apiUrl('?scope=entity&entity_id=' + entityId), { headers: apiHeaders() }).then(function (r) { return r.ok ? r.json() : []; })
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

      fetch(apiUrl(), {
        method: 'POST',
        headers: apiHeaders(),
        body: JSON.stringify(body)
      }).then(function (r) { return r.json(); })
        .then(function (note) {
          if (isPageNote) {
            state.pageNotes.unshift(note);
          }
          state.notes.unshift(note);
          renderNotes();
        });
    }

    /** Full create with editing mode (from + button). */
    function createNote() {
      var isPageNote = state.tab === 'page' && entityId;
      var body = {
        title: '',
        content: [{ type: 'text', value: '' }]
      };
      if (isPageNote) {
        body.entityId = entityId;
      }

      fetch(apiUrl(), {
        method: 'POST',
        headers: apiHeaders(),
        body: JSON.stringify(body)
      }).then(function (r) { return r.json(); })
        .then(function (note) {
          if (isPageNote) {
            state.pageNotes.unshift(note);
          }
          state.notes.unshift(note);
          state.editingId = note.id;
          renderNotes();
          var titleInput = notesList.querySelector('.note-card[data-id="' + note.id + '"] .note-title-input');
          if (titleInput) titleInput.focus();
        });
    }

    function updateNote(id, data) {
      return fetch(apiUrl('/' + id), {
        method: 'PUT',
        headers: apiHeaders(),
        body: JSON.stringify(data)
      }).then(function (r) { return r.json(); })
        .then(function (updated) {
          replaceNoteInState(updated);
          return updated;
        });
    }

    function deleteNote(id) {
      // Release lock before deleting if we hold one.
      if (state.lockedNoteId === id) {
        releaseLockIfHeld();
      }
      fetch(apiUrl('/' + id), {
        method: 'DELETE',
        headers: apiHeaders()
      }).then(function () {
        state.notes = state.notes.filter(function (n) { return n.id !== id; });
        state.pageNotes = state.pageNotes.filter(function (n) { return n.id !== id; });
        if (state.editingId === id) state.editingId = null;
        renderNotes();
      });
    }

    function toggleCheck(noteId, blockIdx, itemIdx) {
      fetch(apiUrl('/' + noteId + '/toggle'), {
        method: 'POST',
        headers: apiHeaders(),
        body: JSON.stringify({ blockIndex: blockIdx, itemIndex: itemIdx })
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
      return fetch(apiUrl('/' + noteId + '/lock'), {
        method: 'POST',
        headers: apiHeaders()
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
      return fetch(apiUrl('/' + noteId + '/unlock'), {
        method: 'POST',
        headers: apiHeaders()
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
      fetch(apiUrl('/' + noteId + '/heartbeat'), {
        method: 'POST',
        headers: apiHeaders()
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

      fetch(apiUrl('/' + noteId + '/versions'), {
        headers: apiHeaders()
      }).then(function (r) { return r.ok ? r.json() : []; })
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
      fetch(apiUrl('/' + noteId + '/versions/' + versionId + '/restore'), {
        method: 'POST',
        headers: apiHeaders()
      }).then(function (r) { return r.json(); })
        .then(function (note) {
          replaceNoteInState(note);
          state.versionsNoteId = null;
          state.versions = [];
          renderNotes();
        });
    }

    // --- Rendering ---

    function updateQuickPlaceholder() {
      if (!quickInput) return;
      quickInput.placeholder = state.tab === 'page'
        ? 'Quick note for this page...'
        : 'Quick note...';
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
        notesList.innerHTML = '<div class="notes-empty">' + escapeHtml(emptyMsg) + '</div>';
        return;
      }

      var html = '';
      list.forEach(function (note) {
        html += renderNoteCard(note);
      });
      notesList.innerHTML = html;

      bindCardEvents();
    }

    function renderNoteCard(note) {
      var isEditing = state.editingId === note.id;
      var isOwner = note.userId === currentUserId;
      var isShared = note.isShared;
      var isLockedByOther = isShared && note.lockedBy && note.lockedBy !== currentUserId && note.lockedAt;
      var pinClass = note.pinned ? ' note-pinned' : '';
      var sharedClass = isShared ? ' note-shared' : '';
      var html = '<div class="note-card' + pinClass + sharedClass + '" data-id="' + escapeAttr(note.id) + '">';

      // Header row.
      html += '<div class="note-card-header">';
      if (isEditing) {
        html += '<input type="text" class="note-title-input" value="' + escapeAttr(note.title === 'Untitled' ? '' : note.title) + '" placeholder="Note title...">';
      } else {
        html += '<span class="note-title">' + escapeHtml(note.title) + '</span>';
      }
      html += '<div class="note-actions">';

      // Shared badge.
      if (isShared && !isOwner) {
        html += '<span class="note-shared-badge" title="Shared note"><i class="fa-solid fa-users text-[9px]"></i></span>';
      }

      // Share toggle (owner only).
      if (isOwner) {
        html += '<button class="note-btn note-share-btn" title="' + (isShared ? 'Make private' : 'Share with campaign') + '">' +
          '<i class="fa-solid ' + (isShared ? 'fa-lock-open' : 'fa-share-nodes') + ' text-[10px]"></i></button>';
      }

      // Lock indicator (shared notes locked by another user).
      if (isLockedByOther) {
        html += '<span class="note-lock-badge" title="Being edited by another user"><i class="fa-solid fa-lock text-[9px]"></i></span>';
      }

      // Pin button (owner only).
      if (isOwner) {
        html += '<button class="note-btn note-pin-btn" title="' + (note.pinned ? 'Unpin' : 'Pin') + '"><i class="fa-solid fa-thumbtack' + (note.pinned ? '' : ' fa-rotate-45') + '"></i></button>';
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

      // Rich text content takes priority over legacy blocks.
      if (note.entryHtml && !isEditing) {
        html += '<div class="note-entry-html">' + note.entryHtml + '</div>';
      } else if (note.content && note.content.length > 0) {
        note.content.forEach(function (block, bIdx) {
          if (block.type === 'text') {
            if (isEditing) {
              html += '<textarea class="note-text-input" data-block="' + bIdx + '" placeholder="Write something...">' + escapeHtml(block.value || '') + '</textarea>';
            } else if (block.value) {
              html += '<p class="note-text">' + escapeHtml(block.value) + '</p>';
            }
          } else if (block.type === 'checklist') {
            html += '<div class="note-checklist" data-block="' + bIdx + '">';
            if (block.items) {
              block.items.forEach(function (item, iIdx) {
                var checked = item.checked ? ' checked' : '';
                var strikeClass = item.checked ? ' note-checked' : '';
                html += '<label class="note-check-item' + strikeClass + '">';
                html += '<input type="checkbox"' + checked + ' data-block="' + bIdx + '" data-item="' + iIdx + '" class="note-checkbox">';
                if (isEditing) {
                  html += '<input type="text" class="note-check-text-input" value="' + escapeAttr(item.text) + '" data-block="' + bIdx + '" data-item="' + iIdx + '" placeholder="List item...">';
                } else {
                  html += '<span>' + escapeHtml(item.text) + '</span>';
                }
                html += '</label>';
              });
            }
            if (isEditing) {
              html += '<button class="note-add-check-item" data-block="' + bIdx + '"><i class="fa-solid fa-plus text-[9px]"></i> Add item</button>';
            }
            html += '</div>';
          }
        });
      }

      // In editing mode, buttons to add blocks.
      if (isEditing) {
        html += '<div class="note-add-block">';
        html += '<button class="note-add-text-block" title="Add text"><i class="fa-solid fa-paragraph text-[10px]"></i></button>';
        html += '<button class="note-add-checklist-block" title="Add checklist"><i class="fa-solid fa-list-check text-[10px]"></i></button>';
        html += '</div>';
      }

      html += '</div></div>';
      return html;
    }

    /** Render the version history sub-panel. */
    function renderVersionsPanel() {
      var note = findNote(state.versionsNoteId);
      var title = note ? escapeHtml(note.title) : 'Note';

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
          html += '<div class="notes-version-item" data-vid="' + escapeAttr(v.id) + '">';
          html += '<div class="notes-version-info">';
          html += '<span class="notes-version-title">' + escapeHtml(v.title || 'Untitled') + '</span>';
          html += '<span class="notes-version-date">' + escapeHtml(dateStr) + '</span>';
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
          if (note && note.isShared) {
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

      // Share toggle button.
      notesList.querySelectorAll('.note-share-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          var note = findNote(noteId);
          if (note) {
            updateNote(noteId, { isShared: !note.isShared }).then(function () {
              renderNotes();
            });
          }
        });
      });

      // Delete button.
      notesList.querySelectorAll('.note-delete-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          deleteNote(card.getAttribute('data-id'));
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

      card.querySelectorAll('.note-text-input').forEach(function (ta) {
        var bIdx = parseInt(ta.getAttribute('data-block'), 10);
        if (note.content[bIdx]) {
          note.content[bIdx].value = ta.value;
        }
      });

      card.querySelectorAll('.note-check-text-input').forEach(function (inp) {
        var bIdx = parseInt(inp.getAttribute('data-block'), 10);
        var iIdx = parseInt(inp.getAttribute('data-item'), 10);
        if (note.content[bIdx] && note.content[bIdx].items && note.content[bIdx].items[iIdx]) {
          note.content[bIdx].items[iIdx].text = inp.value;
        }
      });

      updateNote(noteId, { title: note.title, content: note.content });
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
        '<input type="text" class="notes-quick-input" placeholder="' + escapeAttr(quickPlaceholder) + '" autocomplete="off">' +
        '</div>' +
        '<div class="notes-list"></div>';
    }

    // --- Helpers ---

    function escapeHtml(text) {
      var div = document.createElement('div');
      div.textContent = text || '';
      return div.innerHTML;
    }

    function escapeAttr(text) {
      return String(text || '').replace(/[&"'<>]/g, function (c) {
        return { '&': '&amp;', '"': '&quot;', "'": '&#39;', '<': '&lt;', '>': '&gt;' }[c];
      });
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
    el._notesNavHandler = onNavigated;
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
    if (el._notesFab) el._notesFab.remove();
    if (el._notesPanel) el._notesPanel.remove();
    delete el._notesState;
    delete el._notesFab;
    delete el._notesPanel;
  }
});
