/**
 * notes.js -- Floating Notes Panel Widget
 *
 * Quick-capture note-taking panel with two modes:
 *   - "Page" mode: auto-selected when on an entity page, shows notes for that page.
 *   - "All" mode: campaign-wide notes, always available.
 *
 * Quick capture: panel opens with a text input already focused. Type and hit
 * Enter to create a note instantly — no extra clicks needed.
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
    var csrfToken = '';

    // Read CSRF token from cookie.
    var match = document.cookie.match('(?:^|; )chronicle_csrf=([^;]*)');
    if (match) csrfToken = decodeURIComponent(match[1]);

    var state = {
      open: false,
      tab: entityId ? 'page' : 'all',  // 'page' or 'all'
      notes: [],
      pageNotes: [],
      editingId: null,
      loading: true
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
      // Auto-focus the quick input so user can start typing immediately.
      setTimeout(function () { if (quickInput) quickInput.focus(); }, 100);
    });

    closeBtn.addEventListener('click', function () {
      state.open = false;
      panel.classList.add('notes-panel-hidden');
      fab.classList.remove('notes-fab-hidden');
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
        fetch(apiUrl('?scope=all'), { headers: apiHeaders() }).then(function (r) { return r.json(); })
      ];

      if (entityId) {
        promises.push(
          fetch(apiUrl('?scope=entity&entity_id=' + entityId), { headers: apiHeaders() }).then(function (r) { return r.json(); })
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

    // --- Rendering ---

    function updateQuickPlaceholder() {
      if (!quickInput) return;
      quickInput.placeholder = state.tab === 'page'
        ? 'Quick note for this page...'
        : 'Quick note...';
    }

    function renderNotes() {
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
      var pinClass = note.pinned ? ' note-pinned' : '';
      var html = '<div class="note-card' + pinClass + '" data-id="' + escapeAttr(note.id) + '">';

      // Header row.
      html += '<div class="note-card-header">';
      if (isEditing) {
        html += '<input type="text" class="note-title-input" value="' + escapeAttr(note.title === 'Untitled' ? '' : note.title) + '" placeholder="Note title...">';
      } else {
        html += '<span class="note-title">' + escapeHtml(note.title) + '</span>';
      }
      html += '<div class="note-actions">';
      html += '<button class="note-btn note-pin-btn" title="' + (note.pinned ? 'Unpin' : 'Pin') + '"><i class="fa-solid fa-thumbtack' + (note.pinned ? '' : ' fa-rotate-45') + '"></i></button>';
      if (isEditing) {
        html += '<button class="note-btn note-done-btn" title="Done"><i class="fa-solid fa-check"></i></button>';
      } else {
        html += '<button class="note-btn note-edit-btn" title="Edit"><i class="fa-solid fa-pen text-[10px]"></i></button>';
      }
      html += '<button class="note-btn note-delete-btn" title="Delete"><i class="fa-solid fa-trash-can text-[10px]"></i></button>';
      html += '</div></div>';

      // Content blocks.
      html += '<div class="note-card-body">';
      if (note.content && note.content.length > 0) {
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

    function bindCardEvents() {
      // Edit button.
      notesList.querySelectorAll('.note-edit-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          state.editingId = card.getAttribute('data-id');
          renderNotes();
        });
      });

      // Done button -- save and exit editing.
      notesList.querySelectorAll('.note-done-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          var noteId = card.getAttribute('data-id');
          saveEditingNote(card, noteId);
          state.editingId = null;
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

      // Delete button.
      notesList.querySelectorAll('.note-delete-btn').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
          e.stopPropagation();
          var card = btn.closest('.note-card');
          deleteNote(card.getAttribute('data-id'));
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

      return '<div class="notes-header">' +
        '<span class="notes-header-title">' + (eid ? 'Page Notes' : 'All Notes') + '</span>' +
        '<div class="notes-header-actions">' +
        '<button class="note-btn notes-close" title="Close"><i class="fa-solid fa-xmark"></i></button>' +
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

    // Store references for cleanup.
    el._notesState = state;
    el._notesFab = fab;
    el._notesPanel = panel;
  },

  /**
   * Clean up the notes widget.
   * @param {HTMLElement} el - Mount point element.
   */
  destroy: function (el) {
    if (el._notesFab) el._notesFab.remove();
    if (el._notesPanel) el._notesPanel.remove();
    delete el._notesState;
    delete el._notesFab;
    delete el._notesPanel;
  }
});
