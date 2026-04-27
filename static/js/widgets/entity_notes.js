/**
 * entity_notes.js -- Chronicle Player Notes Widget
 *
 * Per-user, per-entity notes with a 5-tier audience ACL:
 *   private    - author only
 *   dm_only    - Owner + IsDmGranted users (only Owner can author)
 *   dm_scribe  - Owner, Scribe, IsDmGranted users (Scribe-tier can author)
 *   everyone   - all campaign members
 *   custom     - explicit user list (sharedWith[])
 *
 * Server enforces ACL on every read/write. The widget shows audience
 * options based on data-can-author-* hints (UX), but the API will
 * reject unauthorized writes regardless.
 *
 * Live updates: subscribes to /ws for entity_note.* messages on the
 * current campaign and refetches the list. Falls back to 30s polling
 * if WebSocket is unavailable (or as a baseline; the two paths layer).
 *
 * Auto-mounted by boot.js on elements with data-widget="entity-notes".
 *
 * Config (from data-* attributes):
 *   data-entity-id              - Entity ID
 *   data-campaign-id            - Campaign ID
 *   data-endpoint               - API base (/campaigns/:id/entities/:eid/notes)
 *   data-csrf                   - CSRF token
 *   data-can-author-dm-only     - "true" if viewer can author dm_only notes
 *   data-can-author-dm-scribe   - "true" if viewer can author dm_scribe notes
 */
(function () {
  'use strict';

  // Audience options visible in the picker. Order = display order.
  // The dm_* options are hidden when the viewer can't author them
  // (see filterAudienceOptions). Server checks again.
  var ALL_AUDIENCES = [
    { value: 'private',    label: 'Private',    icon: 'fa-lock',           desc: 'Only you can see this' },
    { value: 'dm_only',    label: 'DM Only',    icon: 'fa-user-secret',    desc: 'GM (and DM-granted users) only' },
    { value: 'dm_scribe',  label: 'DM + Co-DM', icon: 'fa-user-shield',    desc: 'GM, Co-DMs, DM-granted users' },
    { value: 'everyone',   label: 'Everyone',   icon: 'fa-users',          desc: 'All campaign members' },
    { value: 'custom',     label: 'Custom',     icon: 'fa-user-plus',      desc: 'Specific users (advanced)' }
  ];

  Chronicle.register('entity-notes', {
    init: function (el, config) {
      var endpoint = config.endpoint || '';
      var campaignId = config.campaignId || '';
      var entityId = config.entityId || '';
      var canAuthorDMOnly = config.canAuthorDmOnly === 'true';
      var canAuthorDMScribe = config.canAuthorDmScribe === 'true';

      var state = {
        notes: [],
        loading: true,
        composeOpen: false,
        composeAudience: 'private',
        composeTitle: '',
        composeBody: '',
        editingId: null,
        editingTitle: '',
        editingBody: '',
        editingAudience: 'private',
      };

      // --- API ---

      function loadNotes() {
        Chronicle.apiFetch(endpoint)
          .then(function (notes) {
            state.notes = Array.isArray(notes) ? notes : [];
            state.loading = false;
            render();
          })
          .catch(function (err) {
            console.error('[EntityNotes] Load error:', err);
            state.loading = false;
            render();
          });
      }

      function createNote(payload) {
        return Chronicle.apiFetch(endpoint, {
          method: 'POST',
          body: JSON.stringify(payload),
        }).then(function (note) {
          // Optimistic insert in case the WS broadcast hasn't reached us yet.
          state.notes.unshift(note);
          return note;
        });
      }

      function updateNote(id, payload) {
        return Chronicle.apiFetch(endpoint + '/' + id, {
          method: 'PUT',
          body: JSON.stringify(payload),
        }).then(function (note) {
          for (var i = 0; i < state.notes.length; i++) {
            if (state.notes[i].id === id) { state.notes[i] = note; break; }
          }
          return note;
        });
      }

      function deleteNote(id) {
        return Chronicle.apiFetch(endpoint + '/' + id, { method: 'DELETE' })
          .then(function () {
            state.notes = state.notes.filter(function (n) { return n.id !== id; });
          });
      }

      // --- Live updates ---
      // Two layers: a short polling interval as a robustness baseline,
      // and (when supported) a WebSocket subscription for instant pushes.
      // Either layer triggers a re-fetch; the server reapplies the ACL.

      var pollTimer = null;
      var ws = null;

      function startPolling() {
        // 30s is a deliberate compromise: short enough that a player who
        // missed the WS push (e.g. background tab woke up) sees fresh
        // content within a session-relevant window, long enough that
        // the API isn't hammered by a hundred entity pages each polling
        // every few seconds.
        pollTimer = window.setInterval(loadNotes, 30000);
      }

      function startWebSocket() {
        if (typeof window.WebSocket !== 'function') return;
        try {
          var protocol = (window.location.protocol === 'https:') ? 'wss:' : 'ws:';
          var url = protocol + '//' + window.location.host + '/ws?campaignId=' + encodeURIComponent(campaignId);
          ws = new WebSocket(url);
          ws.addEventListener('message', function (ev) {
            var msg;
            try { msg = JSON.parse(ev.data); } catch (e) { return; }
            if (!msg || msg.campaignId !== campaignId) return;
            if (msg.type === 'entity_note.created' ||
                msg.type === 'entity_note.updated' ||
                msg.type === 'entity_note.deleted') {
              // Refetch rather than apply diffs locally — the audience
              // ACL is server-side, and a payload with a note body would
              // leak private content. The list refetch is cheap.
              loadNotes();
            }
          });
          ws.addEventListener('close', function () { ws = null; });
          ws.addEventListener('error', function () { /* fall back to polling */ });
        } catch (e) {
          // Ignore; polling carries us.
        }
      }

      // --- Render ---

      function render() {
        if (state.loading) {
          el.innerHTML = '<div class="text-sm text-fg-muted py-4">Loading notes…</div>';
          return;
        }
        var html = '';
        html += renderHeader();
        if (state.composeOpen) html += renderCompose();
        html += renderList();
        el.innerHTML = html;
        bindEvents();
      }

      function renderHeader() {
        var h = '';
        h += '<div class="flex items-center justify-between mb-3">';
        h += '<h3 class="text-sm font-semibold text-fg-secondary uppercase tracking-wider">';
        h += '<i class="fa-solid fa-sticky-note mr-1.5"></i>Player Notes';
        if (state.notes.length > 0) {
          h += ' <span class="text-xs font-normal text-fg-muted">(' + state.notes.length + ')</span>';
        }
        h += '</h3>';
        h += '<button class="btn-secondary text-xs" data-action="toggle-compose">';
        h += '<i class="fa-solid fa-' + (state.composeOpen ? 'xmark' : 'plus') + ' mr-1"></i>';
        h += state.composeOpen ? 'Cancel' : 'New Note';
        h += '</button>';
        h += '</div>';
        return h;
      }

      function renderCompose() {
        var h = '';
        h += '<div class="card p-3 mb-3 space-y-2 border-l-4 border-accent">';
        h += '<input type="text" class="input w-full text-sm" data-field="title" placeholder="Title (optional)" value="' + esc(state.composeTitle) + '"/>';
        h += '<textarea class="input w-full text-sm font-sans" rows="4" data-field="body" placeholder="Your note…">' + esc(state.composeBody) + '</textarea>';
        h += '<div class="flex items-center gap-2 flex-wrap">';
        h += renderAudiencePicker(state.composeAudience, 'compose');
        h += '<div class="flex-1"></div>';
        h += '<button class="btn-primary text-xs" data-action="save-compose">Save Note</button>';
        h += '</div>';
        h += '</div>';
        return h;
      }

      function renderAudiencePicker(current, namespace) {
        var options = filterAudienceOptions();
        var h = '';
        h += '<select class="input text-xs" data-field="audience" data-namespace="' + namespace + '" title="Who can see this note">';
        for (var i = 0; i < options.length; i++) {
          var o = options[i];
          h += '<option value="' + o.value + '"' + (o.value === current ? ' selected' : '') + '>';
          h += o.label;
          h += '</option>';
        }
        h += '</select>';
        return h;
      }

      // filterAudienceOptions hides options the viewer cannot author.
      // The server is the actual gate; this just keeps the UI honest.
      function filterAudienceOptions() {
        return ALL_AUDIENCES.filter(function (o) {
          if (o.value === 'dm_only')   return canAuthorDMOnly;
          if (o.value === 'dm_scribe') return canAuthorDMScribe;
          return true;
        });
      }

      function renderList() {
        if (state.notes.length === 0) {
          return '<div class="card p-6 text-center"><i class="fa-solid fa-sticky-note text-2xl text-fg-muted mb-2"></i><p class="text-sm text-fg-muted">No notes yet. Click <strong>New Note</strong> to add one.</p></div>';
        }
        var h = '<div class="space-y-2">';
        for (var i = 0; i < state.notes.length; i++) {
          h += renderNote(state.notes[i]);
        }
        h += '</div>';
        return h;
      }

      function renderNote(note) {
        var isEditing = state.editingId === note.id;
        var h = '';
        h += '<div class="card overflow-hidden" data-note-id="' + esc(note.id) + '">';

        if (isEditing) {
          h += renderNoteEditor(note);
        } else {
          h += renderNoteView(note);
        }
        h += '</div>';
        return h;
      }

      function renderNoteView(note) {
        var aud = audienceMeta(note.audience);
        var h = '';
        h += '<div class="px-4 py-3">';
        h += '<div class="flex items-start justify-between gap-2">';
        h += '<div class="flex-1 min-w-0">';
        if (note.title) {
          h += '<div class="font-medium text-sm text-fg">' + esc(note.title) + '</div>';
        }
        h += '<div class="text-xs text-fg-muted mt-0.5">';
        h += '<i class="fa-solid ' + aud.icon + ' mr-1"></i>' + esc(aud.label);
        if (note.pinned) h += ' <span class="ml-2"><i class="fa-solid fa-thumbtack text-amber-500"></i></span>';
        h += '</div>';
        h += '</div>';
        // Only the author can edit/delete; the API enforces this regardless.
        // We approximate "is this mine" by audience: if it's `private` or
        // shows up in the list at all, it might be ours; the server returns
        // 404 on non-author edit attempts so wrong guesses are safe.
        h += '<div class="flex gap-1">';
        h += '<button class="text-fg-muted hover:text-fg text-xs p-1" data-action="edit-note" data-note-id="' + esc(note.id) + '" title="Edit"><i class="fa-solid fa-pen"></i></button>';
        h += '<button class="text-fg-muted hover:text-red-500 text-xs p-1" data-action="delete-note" data-note-id="' + esc(note.id) + '" title="Delete"><i class="fa-solid fa-trash"></i></button>';
        h += '</div>';
        h += '</div>';
        if (note.bodyHtml) {
          h += '<div class="prose prose-sm dark:prose-invert max-w-none mt-2">' + note.bodyHtml + '</div>';
        } else if (note.body) {
          // Fallback: render plain text from the JSON. Rare path.
          h += '<div class="text-sm text-fg whitespace-pre-wrap mt-2">' + esc(extractText(note.body)) + '</div>';
        }
        h += '</div>';
        return h;
      }

      function renderNoteEditor(note) {
        var h = '';
        h += '<div class="px-4 py-3 space-y-2 border-l-4 border-accent">';
        h += '<input type="text" class="input w-full text-sm" data-field="title" data-note-id="' + esc(note.id) + '" value="' + esc(state.editingTitle) + '" placeholder="Title (optional)"/>';
        h += '<textarea class="input w-full text-sm font-sans" rows="4" data-field="body" data-note-id="' + esc(note.id) + '" placeholder="Your note…">' + esc(state.editingBody) + '</textarea>';
        h += '<div class="flex items-center gap-2 flex-wrap">';
        h += renderAudiencePicker(state.editingAudience, 'edit-' + note.id);
        h += '<div class="flex-1"></div>';
        h += '<button class="btn-secondary text-xs" data-action="cancel-edit">Cancel</button>';
        h += '<button class="btn-primary text-xs" data-action="save-edit" data-note-id="' + esc(note.id) + '">Save</button>';
        h += '</div>';
        h += '</div>';
        return h;
      }

      // --- Events ---

      function bindEvents() {
        // Compose toggle.
        var toggleBtn = el.querySelector('[data-action="toggle-compose"]');
        if (toggleBtn) {
          toggleBtn.addEventListener('click', function () {
            state.composeOpen = !state.composeOpen;
            if (!state.composeOpen) {
              state.composeTitle = '';
              state.composeBody = '';
              state.composeAudience = 'private';
            }
            render();
          });
        }
        // Compose field bindings.
        bindComposeFields();
        // Save compose.
        var saveBtn = el.querySelector('[data-action="save-compose"]');
        if (saveBtn) {
          saveBtn.addEventListener('click', function () {
            var body = state.composeBody.trim();
            var title = state.composeTitle.trim();
            if (!body && !title) {
              Chronicle.notify && Chronicle.notify('Please write something first', 'warn');
              return;
            }
            createNote({
              audience: state.composeAudience,
              title: title,
              bodyHtml: bodyToHTML(body),
            }).then(function () {
              state.composeOpen = false;
              state.composeTitle = '';
              state.composeBody = '';
              state.composeAudience = 'private';
              render();
            }).catch(function (err) {
              console.error('[EntityNotes] Create error:', err);
              Chronicle.notify && Chronicle.notify(humanError(err), 'error');
            });
          });
        }
        // Edit note.
        bindAll('[data-action="edit-note"]', 'click', function () {
          var id = this.getAttribute('data-note-id');
          var note = findNote(id);
          if (!note) return;
          state.editingId = id;
          state.editingTitle = note.title || '';
          state.editingBody = stripHtml(note.bodyHtml || '');
          state.editingAudience = note.audience || 'private';
          render();
        });
        // Cancel edit.
        bindAll('[data-action="cancel-edit"]', 'click', function () {
          state.editingId = null;
          render();
        });
        // Save edit.
        bindAll('[data-action="save-edit"]', 'click', function () {
          var id = this.getAttribute('data-note-id');
          updateNote(id, {
            audience: state.editingAudience,
            title: state.editingTitle.trim(),
            bodyHtml: bodyToHTML(state.editingBody.trim()),
          }).then(function () {
            state.editingId = null;
            render();
          }).catch(function (err) {
            console.error('[EntityNotes] Update error:', err);
            Chronicle.notify && Chronicle.notify(humanError(err), 'error');
          });
        });
        // Delete note.
        bindAll('[data-action="delete-note"]', 'click', function () {
          var id = this.getAttribute('data-note-id');
          if (!window.confirm('Delete this note? This cannot be undone.')) return;
          deleteNote(id)
            .then(render)
            .catch(function (err) {
              console.error('[EntityNotes] Delete error:', err);
              Chronicle.notify && Chronicle.notify(humanError(err), 'error');
            });
        });
        // Edit field bindings.
        bindEditFields();
      }

      function bindComposeFields() {
        var titleInput = el.querySelector('[data-field="title"]:not([data-note-id])');
        if (titleInput) {
          titleInput.addEventListener('input', function () { state.composeTitle = titleInput.value; });
        }
        var bodyInput = el.querySelector('[data-field="body"]:not([data-note-id])');
        if (bodyInput) {
          bodyInput.addEventListener('input', function () { state.composeBody = bodyInput.value; });
        }
        var audInput = el.querySelector('[data-field="audience"][data-namespace="compose"]');
        if (audInput) {
          audInput.addEventListener('change', function () { state.composeAudience = audInput.value; });
        }
      }

      function bindEditFields() {
        if (!state.editingId) return;
        var titleInput = el.querySelector('[data-field="title"][data-note-id="' + state.editingId + '"]');
        if (titleInput) {
          titleInput.addEventListener('input', function () { state.editingTitle = titleInput.value; });
        }
        var bodyInput = el.querySelector('[data-field="body"][data-note-id="' + state.editingId + '"]');
        if (bodyInput) {
          bodyInput.addEventListener('input', function () { state.editingBody = bodyInput.value; });
        }
        var audInput = el.querySelector('[data-field="audience"][data-namespace="edit-' + state.editingId + '"]');
        if (audInput) {
          audInput.addEventListener('change', function () { state.editingAudience = audInput.value; });
        }
      }

      function bindAll(selector, evt, handler) {
        var nodes = el.querySelectorAll(selector);
        for (var i = 0; i < nodes.length; i++) nodes[i].addEventListener(evt, handler);
      }

      // --- Helpers ---

      function findNote(id) {
        for (var i = 0; i < state.notes.length; i++) {
          if (state.notes[i].id === id) return state.notes[i];
        }
        return null;
      }

      function audienceMeta(value) {
        for (var i = 0; i < ALL_AUDIENCES.length; i++) {
          if (ALL_AUDIENCES[i].value === value) return ALL_AUDIENCES[i];
        }
        return ALL_AUDIENCES[0];
      }

      // bodyToHTML wraps the textarea text in <p> tags so the server's
      // sanitizer accepts it as HTML. Preserves blank-line paragraph breaks.
      function bodyToHTML(text) {
        if (!text) return '';
        var paragraphs = text.split(/\n{2,}/);
        return paragraphs.map(function (p) {
          return '<p>' + escHtml(p).replace(/\n/g, '<br>') + '</p>';
        }).join('');
      }

      function stripHtml(html) {
        if (!html) return '';
        var div = document.createElement('div');
        div.innerHTML = html;
        return div.textContent || div.innerText || '';
      }

      function extractText(jsonBody) {
        // Best-effort fallback for ProseMirror JSON when bodyHtml is missing.
        try {
          var parsed = typeof jsonBody === 'string' ? JSON.parse(jsonBody) : jsonBody;
          var out = [];
          (function walk(node) {
            if (!node) return;
            if (typeof node.text === 'string') out.push(node.text);
            if (Array.isArray(node.content)) node.content.forEach(walk);
          })(parsed);
          return out.join(' ');
        } catch (e) {
          return '';
        }
      }

      function humanError(err) {
        if (err && err.message) return err.message;
        if (typeof err === 'string') return err;
        return 'Something went wrong';
      }

      function esc(s) {
        return Chronicle.escapeHtml ? Chronicle.escapeHtml(s || '') : escHtml(s || '');
      }
      function escHtml(s) {
        return String(s)
          .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
          .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
      }

      // --- Init ---

      loadNotes();
      startPolling();
      startWebSocket();

      // Cleanup hooks. boot.js's destroy callback fires on widget unmount
      // (HTMX swap, page nav). Without these, polling intervals would
      // leak and stale WS connections would pile up across navigations.
      el.__entityNotesCleanup = function () {
        if (pollTimer) { window.clearInterval(pollTimer); pollTimer = null; }
        if (ws) { try { ws.close(); } catch (e) {} ws = null; }
      };
    },

    destroy: function (el) {
      if (typeof el.__entityNotesCleanup === 'function') {
        el.__entityNotesCleanup();
        delete el.__entityNotesCleanup;
      }
      el.innerHTML = '';
    },
  });
})();
