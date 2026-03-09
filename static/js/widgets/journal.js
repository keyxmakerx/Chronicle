/**
 * journal.js -- Full-Page Journal Widget
 *
 * Renders an Obsidian-like two-panel journal view: note tree on the left,
 * rich text editor on the right. Communicates with the existing notes API
 * endpoints (GET/POST/PUT/DELETE /campaigns/:id/notes).
 *
 * The floating notes panel (notes.js) is hidden on the journal page to
 * avoid sync conflicts — this widget takes full ownership of note state.
 *
 * Mount: <div data-widget="journal" data-campaign-id="..." data-user-id="...">
 */
Chronicle.register('journal', {
  /**
   * Initialize the full-page journal widget.
   * @param {HTMLElement} el - Mount point element with data-campaign-id.
   * @param {Object} config - Parsed data-* attributes.
   */
  init: function (el, config) {
    'use strict';

    var campaignId = config.campaignId || '';
    var currentUserId = config.userId || '';
    var AUTOSAVE_DELAY = 1500; // ms

    // --- State ---

    var state = {
      notes: [],
      activeNoteId: null,
      searchFilter: '',
      tab: 'all', // 'all', 'campaign', 'session'
      loading: true,
      saving: false,
      collapsedFolders: loadCollapsedFolders()
    };

    var autosaveTimer = null;
    var activeEditor = null; // TipTap instance for the active note

    // --- API helpers ---

    function apiUrl(suffix) {
      return '/campaigns/' + campaignId + '/notes' + (suffix || '');
    }

    // --- Persistence for collapsed folders ---

    function loadCollapsedFolders() {
      try {
        var raw = localStorage.getItem('chronicle_journal_collapsed_' + campaignId);
        return raw ? JSON.parse(raw) : {};
      } catch (e) { return {}; }
    }

    function saveCollapsedFolders() {
      try {
        localStorage.setItem('chronicle_journal_collapsed_' + campaignId, JSON.stringify(state.collapsedFolders));
      } catch (e) { /* ignore */ }
    }

    // --- Load notes from API ---

    function loadNotes() {
      state.loading = true;
      renderNoteList();

      Chronicle.apiFetch(apiUrl('?scope=all'))
        .then(function (r) { return r.ok ? r.json() : []; })
        .then(function (notes) {
          state.notes = notes || [];
          state.loading = false;
          renderNoteList();
          // If we had an active note, re-select it.
          if (state.activeNoteId) {
            var found = state.notes.find(function (n) { return n.id === state.activeNoteId; });
            if (!found) {
              state.activeNoteId = null;
              showEmptyState();
            }
          }
        })
        .catch(function () {
          state.notes = [];
          state.loading = false;
          renderNoteList();
        });
    }

    // --- Create note ---

    function createNote(opts) {
      opts = opts || {};
      var body = {
        title: opts.title || '',
        content: [{ type: 'text', value: '' }],
        isFolder: !!opts.isFolder
      };
      if (opts.parentId) body.parentId = opts.parentId;

      Chronicle.apiFetch(apiUrl(), { method: 'POST', body: body })
        .then(function (r) { return r.json(); })
        .then(function (note) {
          state.notes.unshift(note);
          renderNoteList();
          if (!note.isFolder) {
            selectNote(note.id);
          }
        })
        .catch(function () {
          Chronicle.notify('Failed to create note', 'error');
        });
    }

    // --- Delete note ---

    function deleteNote(id) {
      if (!confirm('Delete this note? This cannot be undone.')) return;

      Chronicle.apiFetch(apiUrl('/' + id), { method: 'DELETE' })
        .then(function (r) {
          if (!r.ok) throw new Error('delete failed');
          state.notes = state.notes.filter(function (n) { return n.id !== id; });
          if (state.activeNoteId === id) {
            state.activeNoteId = null;
            destroyEditor();
            showEmptyState();
          }
          renderNoteList();
        })
        .catch(function () {
          Chronicle.notify('Failed to delete note', 'error');
        });
    }

    // --- Save active note ---

    function saveActiveNote() {
      if (!state.activeNoteId || !activeEditor) return;

      var note = findNote(state.activeNoteId);
      if (!note) return;

      var titleInput = document.getElementById('journal-note-title');
      var title = titleInput ? titleInput.value : note.title;

      var json = JSON.stringify(activeEditor.getJSON());
      var html = activeEditor.getHTML();

      state.saving = true;
      updateStatus('Saving...');

      Chronicle.apiFetch(apiUrl('/' + state.activeNoteId), {
        method: 'PUT',
        body: { title: title, entry: json, entryHtml: html }
      })
        .then(function (r) { return r.json(); })
        .then(function (updated) {
          // Update the note in state.
          for (var i = 0; i < state.notes.length; i++) {
            if (state.notes[i].id === updated.id) {
              state.notes[i] = updated;
              break;
            }
          }
          state.saving = false;
          updateStatus('Saved');
          updateTimestamp(updated.updatedAt);
          renderNoteList();
        })
        .catch(function () {
          state.saving = false;
          updateStatus('Save failed');
          Chronicle.notify('Failed to save note', 'error');
        });
    }

    function scheduleAutosave() {
      if (autosaveTimer) clearTimeout(autosaveTimer);
      autosaveTimer = setTimeout(saveActiveNote, AUTOSAVE_DELAY);
      updateStatus('Editing...');
    }

    // --- Toggle pin / share ---

    function togglePin(id) {
      var note = findNote(id);
      if (!note) return;
      Chronicle.apiFetch(apiUrl('/' + id), {
        method: 'PUT',
        body: { pinned: !note.pinned }
      }).then(function (r) { return r.json(); })
        .then(function (updated) {
          for (var i = 0; i < state.notes.length; i++) {
            if (state.notes[i].id === updated.id) { state.notes[i] = updated; break; }
          }
          renderNoteList();
          updatePinButton(updated);
        });
    }

    function toggleShare(id) {
      var note = findNote(id);
      if (!note) return;
      Chronicle.apiFetch(apiUrl('/' + id), {
        method: 'PUT',
        body: { isShared: !note.isShared }
      }).then(function (r) { return r.json(); })
        .then(function (updated) {
          for (var i = 0; i < state.notes.length; i++) {
            if (state.notes[i].id === updated.id) { state.notes[i] = updated; break; }
          }
          renderNoteList();
          updateShareButton(updated);
        });
    }

    // --- Select a note for editing ---

    function selectNote(id) {
      // Save current note before switching.
      if (state.activeNoteId && activeEditor) {
        saveActiveNote();
      }

      state.activeNoteId = id;
      var note = findNote(id);
      if (!note) { showEmptyState(); return; }

      // Show editor area, hide empty state.
      var emptyState = document.getElementById('journal-empty-state');
      var content = document.getElementById('journal-note-content');
      if (emptyState) emptyState.classList.add('hidden');
      if (content) content.classList.remove('hidden');

      // Set title.
      var titleInput = document.getElementById('journal-note-title');
      if (titleInput) {
        titleInput.value = note.title;
        titleInput.oninput = scheduleAutosave;
      }

      // Update action buttons.
      updatePinButton(note);
      updateShareButton(note);

      // Update timestamp.
      updateTimestamp(note.updatedAt);
      updateStatus('Ready');

      // Initialize TipTap editor with note content.
      initEditor(note);

      // Highlight active note in list.
      renderNoteList();
    }

    // --- TipTap editor lifecycle ---

    function initEditor(note) {
      destroyEditor();

      var editorArea = document.getElementById('journal-editor-area');
      if (!editorArea) return;
      editorArea.innerHTML = '';

      // Check if TipTap is available (loaded by the main editor widget).
      if (!window.Chronicle || !window.Chronicle._tiptapBundle) {
        // Fallback: render as HTML with a textarea.
        renderFallbackEditor(editorArea, note);
        return;
      }

      var bundle = window.Chronicle._tiptapBundle;
      var extensions = [
        bundle.StarterKit,
        bundle.Underline,
        bundle.Placeholder.configure({ placeholder: 'Start writing...' })
      ];

      // Parse entry JSON if available, otherwise use empty doc.
      var content = '';
      if (note.entry) {
        try { content = JSON.parse(note.entry); }
        catch (e) { content = note.entryHtml || ''; }
      } else if (note.entryHtml) {
        content = note.entryHtml;
      }

      activeEditor = new bundle.Editor({
        element: editorArea,
        extensions: extensions,
        content: content,
        editorProps: {
          attributes: {
            class: 'prose prose-sm dark:prose-invert max-w-none focus:outline-none min-h-[300px]'
          }
        },
        onUpdate: function () {
          scheduleAutosave();
        }
      });
    }

    /** Fallback for when TipTap bundle isn't loaded: show entryHtml as read-only
     *  with a simple textarea for editing. */
    function renderFallbackEditor(container, note) {
      if (note.entryHtml) {
        container.innerHTML = '<div class="prose prose-sm dark:prose-invert max-w-none">' +
          note.entryHtml + '</div>';
      } else {
        var blocks = note.content || [];
        var text = blocks.map(function (b) { return b.value || ''; }).join('\n');
        var ta = document.createElement('textarea');
        ta.className = 'w-full h-full min-h-[300px] bg-transparent text-fg border-none outline-none resize-none text-sm';
        ta.value = text;
        ta.oninput = scheduleAutosave;
        container.appendChild(ta);
      }
    }

    function destroyEditor() {
      if (activeEditor) {
        activeEditor.destroy();
        activeEditor = null;
      }
      if (autosaveTimer) {
        clearTimeout(autosaveTimer);
        autosaveTimer = null;
      }
    }

    // --- UI helpers ---

    function findNote(id) {
      return state.notes.find(function (n) { return n.id === id; }) || null;
    }

    function showEmptyState() {
      var emptyState = document.getElementById('journal-empty-state');
      var content = document.getElementById('journal-note-content');
      if (emptyState) emptyState.classList.remove('hidden');
      if (content) content.classList.add('hidden');
      destroyEditor();
    }

    function updateStatus(text) {
      var el = document.getElementById('journal-note-status');
      if (el) el.textContent = text;
    }

    function updateTimestamp(ts) {
      var el = document.getElementById('journal-note-updated');
      if (!el || !ts) return;
      var d = new Date(ts);
      el.textContent = 'Updated ' + d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    }

    function updatePinButton(note) {
      var btn = document.getElementById('journal-note-pin');
      if (!btn) return;
      var icon = btn.querySelector('i');
      if (icon) {
        icon.className = note.pinned
          ? 'fa-solid fa-thumbtack text-sm text-accent'
          : 'fa-solid fa-thumbtack text-sm';
      }
    }

    function updateShareButton(note) {
      var btn = document.getElementById('journal-note-share');
      if (!btn) return;
      var icon = btn.querySelector('i');
      if (icon) {
        icon.className = note.isShared
          ? 'fa-solid fa-share-nodes text-sm text-accent'
          : 'fa-solid fa-share-nodes text-sm';
      }
    }

    // --- Render note list ---

    function renderNoteList() {
      var container = document.getElementById('journal-note-list');
      if (!container) return;

      if (state.loading) {
        container.innerHTML = '<div class="flex items-center justify-center py-12 text-fg-muted text-sm">' +
          '<i class="fa-solid fa-spinner fa-spin mr-2"></i> Loading...</div>';
        return;
      }

      // Filter notes by tab.
      var notes = filterNotes(state.notes);

      // Filter by search.
      if (state.searchFilter) {
        var q = state.searchFilter.toLowerCase();
        notes = notes.filter(function (n) {
          return (n.title || '').toLowerCase().indexOf(q) !== -1;
        });
      }

      if (notes.length === 0) {
        container.innerHTML = '<div class="flex flex-col items-center justify-center py-12 text-fg-muted text-sm">' +
          '<i class="fa-solid fa-file-circle-plus text-lg mb-2 opacity-30"></i>' +
          '<span>No notes yet</span></div>';
        return;
      }

      // Build tree from flat list.
      var tree = buildTree(notes);
      container.innerHTML = '';
      renderTreeNodes(container, tree, 0);
    }

    function filterNotes(notes) {
      switch (state.tab) {
        case 'campaign':
          return notes.filter(function (n) { return !n.entityId; });
        case 'session':
          return notes.filter(function (n) {
            return (n.title || '').toLowerCase().indexOf('session') !== -1 ||
                   (n.color === '#7c3aed'); // Session journal color
          });
        default:
          return notes;
      }
    }

    // Build parent-child tree from flat list.
    function buildTree(notes) {
      var map = {};
      var roots = [];

      notes.forEach(function (n) { map[n.id] = { note: n, children: [] }; });

      notes.forEach(function (n) {
        if (n.parentId && map[n.parentId]) {
          map[n.parentId].children.push(map[n.id]);
        } else {
          roots.push(map[n.id]);
        }
      });

      // Sort: folders first, then pinned, then by title.
      function sortNodes(arr) {
        arr.sort(function (a, b) {
          if (a.note.isFolder !== b.note.isFolder) return a.note.isFolder ? -1 : 1;
          if (a.note.pinned !== b.note.pinned) return a.note.pinned ? -1 : 1;
          return (a.note.title || '').localeCompare(b.note.title || '');
        });
        arr.forEach(function (node) { sortNodes(node.children); });
      }
      sortNodes(roots);

      return roots;
    }

    function renderTreeNodes(container, nodes, depth) {
      nodes.forEach(function (node) {
        var note = node.note;
        var hasChildren = node.children.length > 0;
        var isCollapsed = !!state.collapsedFolders[note.id];
        var isActive = note.id === state.activeNoteId;

        var item = document.createElement('div');
        item.className = 'journal-list-item flex items-center px-2 py-1.5 rounded-md text-sm cursor-pointer transition-colors ' +
          (isActive ? 'bg-accent/10 text-fg' : 'text-fg-secondary hover:bg-surface hover:text-fg');
        item.style.paddingLeft = (8 + depth * 16) + 'px';

        // Toggle for folders.
        if (note.isFolder || hasChildren) {
          var toggle = document.createElement('span');
          toggle.className = 'w-4 h-4 flex items-center justify-center mr-1 shrink-0 text-fg-muted hover:text-fg cursor-pointer';
          toggle.innerHTML = isCollapsed
            ? '<i class="fa-solid fa-chevron-right text-[8px]"></i>'
            : '<i class="fa-solid fa-chevron-down text-[8px]"></i>';
          toggle.addEventListener('click', function (e) {
            e.stopPropagation();
            if (state.collapsedFolders[note.id]) {
              delete state.collapsedFolders[note.id];
            } else {
              state.collapsedFolders[note.id] = true;
            }
            saveCollapsedFolders();
            renderNoteList();
          });
          item.appendChild(toggle);
        } else if (depth > 0) {
          var spacer = document.createElement('span');
          spacer.className = 'w-4 mr-1 shrink-0';
          item.appendChild(spacer);
        }

        // Icon.
        var icon = document.createElement('span');
        icon.className = 'w-4 h-4 flex items-center justify-center mr-2 shrink-0';
        if (note.isFolder) {
          icon.innerHTML = isCollapsed
            ? '<i class="fa-solid fa-folder text-amber-400/70 text-xs"></i>'
            : '<i class="fa-solid fa-folder-open text-amber-400/70 text-xs"></i>';
        } else if (note.color === '#7c3aed') {
          // Session journal note.
          icon.innerHTML = '<i class="fa-solid fa-book-open text-purple-400/70 text-xs"></i>';
        } else {
          icon.innerHTML = '<i class="fa-solid fa-file-lines text-fg-muted text-[10px]"></i>';
        }
        item.appendChild(icon);

        // Title.
        var title = document.createElement('span');
        title.className = 'flex-1 truncate text-xs';
        title.textContent = note.title || 'Untitled';
        item.appendChild(title);

        // Badges.
        if (note.pinned) {
          var pin = document.createElement('i');
          pin.className = 'fa-solid fa-thumbtack text-[8px] text-accent ml-1 shrink-0';
          item.appendChild(pin);
        }
        if (note.isShared) {
          var share = document.createElement('i');
          share.className = 'fa-solid fa-users text-[8px] text-fg-muted ml-1 shrink-0';
          item.appendChild(share);
        }

        // Click to select (unless it's a folder, then just toggle).
        item.addEventListener('click', function () {
          if (note.isFolder) {
            // Toggle folder collapse.
            if (state.collapsedFolders[note.id]) {
              delete state.collapsedFolders[note.id];
            } else {
              state.collapsedFolders[note.id] = true;
            }
            saveCollapsedFolders();
            renderNoteList();
          } else {
            selectNote(note.id);
          }
        });

        container.appendChild(item);

        // Render children (if not collapsed).
        if (hasChildren && !isCollapsed) {
          renderTreeNodes(container, node.children, depth + 1);
        }
      });
    }

    // --- Event bindings ---

    // New note button.
    var newNoteBtn = document.getElementById('journal-new-note');
    if (newNoteBtn) {
      newNoteBtn.addEventListener('click', function () {
        createNote({ title: '' });
      });
    }

    // New folder button.
    var newFolderBtn = document.getElementById('journal-new-folder');
    if (newFolderBtn) {
      newFolderBtn.addEventListener('click', function () {
        createNote({ title: 'New Folder', isFolder: true });
      });
    }

    // Search input.
    var searchInput = document.getElementById('journal-search');
    if (searchInput) {
      searchInput.addEventListener('input', function () {
        state.searchFilter = searchInput.value;
        renderNoteList();
      });
    }

    // Tab buttons.
    var tabs = document.querySelectorAll('.journal-tab');
    tabs.forEach(function (tab) {
      tab.addEventListener('click', function () {
        state.tab = tab.getAttribute('data-tab') || 'all';
        // Update active tab styling.
        tabs.forEach(function (t) {
          t.classList.remove('text-fg', 'border-accent');
          t.classList.add('text-fg-muted', 'border-transparent');
        });
        tab.classList.remove('text-fg-muted', 'border-transparent');
        tab.classList.add('text-fg', 'border-accent');
        renderNoteList();
      });
      // Set initial active tab.
      if (tab.getAttribute('data-tab') === state.tab) {
        tab.classList.remove('text-fg-muted', 'border-transparent');
        tab.classList.add('text-fg', 'border-accent');
      }
    });

    // Delete button.
    var deleteBtn = document.getElementById('journal-note-delete');
    if (deleteBtn) {
      deleteBtn.addEventListener('click', function () {
        if (state.activeNoteId) deleteNote(state.activeNoteId);
      });
    }

    // Pin button.
    var pinBtn = document.getElementById('journal-note-pin');
    if (pinBtn) {
      pinBtn.addEventListener('click', function () {
        if (state.activeNoteId) togglePin(state.activeNoteId);
      });
    }

    // Share button.
    var shareBtn = document.getElementById('journal-note-share');
    if (shareBtn) {
      shareBtn.addEventListener('click', function () {
        if (state.activeNoteId) toggleShare(state.activeNoteId);
      });
    }

    // --- Initial load ---
    loadNotes();

    // --- Cleanup ---
    return function () {
      destroyEditor();
      if (autosaveTimer) clearTimeout(autosaveTimer);
    };
  }
});
