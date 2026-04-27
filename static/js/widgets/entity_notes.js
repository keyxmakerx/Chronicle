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
        composeSharedWith: [],
        editingId: null,
        editingTitle: '',
        editingBody: '',
        editingAudience: 'private',
        editingSharedWith: [],
        // Campaign members lazy-loaded the first time the user picks
        // 'custom' audience. Cached afterwards so the picker stays
        // snappy for repeat sharing actions.
        members: null,
        membersLoading: false,
      };

      // --- API ---
      //
      // Chronicle.apiFetch returns a raw Response object, NOT parsed JSON
      // (it's a thin wrapper around fetch() — see boot.js:522). Every call
      // here funnels through asJSON which (a) parses the body, (b) bubbles
      // server-side error messages up to the .catch handlers so the
      // operator sees "you do not have permission to use this audience"
      // instead of "Something went wrong."
      function asJSON(resp) {
        if (!resp.ok) {
          return resp.json().then(
            function (body) {
              var msg = (body && (body.message || body.error)) || ('HTTP ' + resp.status);
              return Promise.reject(new Error(msg));
            },
            function () {
              return Promise.reject(new Error('HTTP ' + resp.status));
            }
          );
        }
        return resp.json();
      }

      function loadNotes() {
        Chronicle.apiFetch(endpoint)
          .then(asJSON)
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
        })
          .then(asJSON)
          .then(function (note) {
            // Optimistic insert in case the WS broadcast hasn't reached us yet.
            state.notes.unshift(note);
            return note;
          });
      }

      function updateNote(id, payload) {
        return Chronicle.apiFetch(endpoint + '/' + id, {
          method: 'PUT',
          body: JSON.stringify(payload),
        })
          .then(asJSON)
          .then(function (note) {
            for (var i = 0; i < state.notes.length; i++) {
              if (state.notes[i].id === id) { state.notes[i] = note; break; }
            }
            return note;
          });
      }

      function deleteNote(id) {
        return Chronicle.apiFetch(endpoint + '/' + id, { method: 'DELETE' })
          .then(function (resp) {
            if (!resp.ok) {
              return resp.json().then(
                function (body) { throw new Error((body && body.message) || 'HTTP ' + resp.status); },
                function () { throw new Error('HTTP ' + resp.status); }
              );
            }
            state.notes = state.notes.filter(function (n) { return n.id !== id; });
          });
      }

      // loadMembers fetches the campaign roster (lazy, cached). Used to
      // populate the custom-audience picker with display names instead
      // of raw UUIDs.
      function loadMembers() {
        if (state.members !== null || state.membersLoading) return;
        state.membersLoading = true;
        Chronicle.apiFetch('/campaigns/' + campaignId + '/members', {
          headers: { 'Accept': 'application/json' },
        })
          .then(asJSON)
          .then(function (members) {
            // Filter out the current viewer — sharing with yourself is
            // a no-op (author always sees own). Keep one row per user.
            state.members = (Array.isArray(members) ? members : []).filter(function (m) {
              return m && m.user_id;
            });
            state.membersLoading = false;
            render();
          })
          .catch(function (err) {
            console.error('[EntityNotes] Members load error:', err);
            state.members = [];
            state.membersLoading = false;
            render();
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
          // Server-side WS auth (internal/websocket/auth.go:79) reads
          // the campaign from `?campaign=` (not `?campaignId=`). Mismatch
          // here causes "campaign parameter required for session auth"
          // 401 even though the cookie is fine.
          var url = protocol + '//' + window.location.host + '/ws?campaign=' + encodeURIComponent(campaignId);
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
          // Connection failures (reverse proxy not forwarding Upgrade
          // headers, /ws endpoint not reachable, etc.) are non-fatal —
          // the 30s polling layer carries the feature. Suppressing the
          // error event prevents Firefox/Chrome from spamming the
          // operator's console; the polling continues regardless.
          ws.addEventListener('error', function (e) {
            e.preventDefault && e.preventDefault();
          });
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
        if (state.composeAudience === 'custom') {
          h += renderMemberPicker(state.composeSharedWith, 'compose');
        }
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

      // renderMemberPicker is a checkbox list of campaign members shown
      // when the user picks 'custom' audience. Each checkbox toggles
      // the user_id in/out of state.composeSharedWith (or editingSharedWith).
      // Members list is lazy-loaded via loadMembers() the first time
      // 'custom' is chosen.
      function renderMemberPicker(selected, namespace) {
        if (state.members === null) {
          // Trigger fetch (no-op if already in flight). loadMembers
          // calls render() when it lands.
          loadMembers();
          return '<div class="mt-2 text-xs text-fg-muted italic px-2">Loading campaign members…</div>';
        }
        if (state.membersLoading) {
          return '<div class="mt-2 text-xs text-fg-muted italic px-2">Loading campaign members…</div>';
        }
        var visibleMembers = state.members.filter(function (m) {
          // Don't show the viewer in their own picker — they always
          // see their own notes regardless of shared_with.
          return m.user_id !== currentUserID();
        });
        if (visibleMembers.length === 0) {
          return '<div class="mt-2 text-xs text-fg-muted italic px-2">No other members in this campaign yet.</div>';
        }
        var sel = {};
        for (var i = 0; i < selected.length; i++) sel[selected[i]] = true;
        var h = '<div class="mt-2 px-2 py-2 rounded bg-surface-alt/50 border border-edge">';
        h += '<div class="text-[11px] font-semibold uppercase tracking-wider text-fg-muted mb-1.5">Share with</div>';
        h += '<div class="flex flex-wrap gap-x-3 gap-y-1.5">';
        for (var j = 0; j < visibleMembers.length; j++) {
          var m = visibleMembers[j];
          var checked = sel[m.user_id] ? ' checked' : '';
          var label = m.display_name || m.email || m.user_id;
          h += '<label class="inline-flex items-center gap-1.5 text-xs cursor-pointer">';
          h += '<input type="checkbox" class="rounded" data-shared-user-id="' + esc(m.user_id) + '" data-shared-namespace="' + esc(namespace) + '"' + checked + '/>';
          h += '<span>' + esc(label) + '</span>';
          h += '</label>';
        }
        h += '</div>';
        h += '</div>';
        return h;
      }

      // currentUserID returns the viewer's user_id by reading the body's
      // notes data-widget mount (which carries it). Falls back to ''
      // if not found — in which case the picker just shows everyone.
      function currentUserID() {
        var notesEl = document.querySelector('[data-widget="notes"][data-user-id]');
        return notesEl ? notesEl.getAttribute('data-user-id') : '';
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
        if (state.editingAudience === 'custom') {
          h += renderMemberPicker(state.editingSharedWith, 'edit-' + note.id);
        }
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
            var payload = {
              audience: state.composeAudience,
              title: title,
              bodyHtml: bodyToHTML(body),
            };
            if (state.composeAudience === 'custom') {
              if (state.composeSharedWith.length === 0) {
                Chronicle.notify && Chronicle.notify('Pick at least one person to share with, or change the audience', 'warn');
                return;
              }
              payload.sharedWith = state.composeSharedWith.slice();
            }
            createNote(payload).then(function () {
              state.composeOpen = false;
              state.composeTitle = '';
              state.composeBody = '';
              state.composeAudience = 'private';
              state.composeSharedWith = [];
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
          // Hydrate shared list from the note so the edit picker shows
          // the existing recipients pre-checked. Server returns
          // sharedWith as an array (never null) per the model.
          state.editingSharedWith = Array.isArray(note.sharedWith) ? note.sharedWith.slice() : [];
          render();
        });
        // Cancel edit.
        bindAll('[data-action="cancel-edit"]', 'click', function () {
          state.editingId = null;
          state.editingSharedWith = [];
          render();
        });
        // Save edit.
        bindAll('[data-action="save-edit"]', 'click', function () {
          var id = this.getAttribute('data-note-id');
          var payload = {
            audience: state.editingAudience,
            title: state.editingTitle.trim(),
            bodyHtml: bodyToHTML(state.editingBody.trim()),
          };
          if (state.editingAudience === 'custom') {
            if (state.editingSharedWith.length === 0) {
              Chronicle.notify && Chronicle.notify('Pick at least one person to share with, or change the audience', 'warn');
              return;
            }
            payload.sharedWith = state.editingSharedWith.slice();
          } else {
            // Explicit empty list so the server clears any previous
            // shared_with when leaving custom audience.
            payload.sharedWith = [];
          }
          updateNote(id, payload).then(function () {
            state.editingId = null;
            state.editingSharedWith = [];
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
          audInput.addEventListener('change', function () {
            state.composeAudience = audInput.value;
            // Reset shared list when leaving custom; trigger render to
            // show/hide the picker. loadMembers fires inside the picker
            // render path on first display.
            if (state.composeAudience !== 'custom') {
              state.composeSharedWith = [];
            }
            render();
          });
        }
        bindSharedCheckboxes('compose');
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
          audInput.addEventListener('change', function () {
            state.editingAudience = audInput.value;
            if (state.editingAudience !== 'custom') {
              state.editingSharedWith = [];
            }
            render();
          });
        }
        bindSharedCheckboxes('edit-' + state.editingId);
      }

      // bindSharedCheckboxes wires the picker checkboxes. The same
      // markup is used for compose + every edit form, distinguished by
      // the namespace data attribute. Toggling a checkbox edits the
      // matching state slice in-place.
      function bindSharedCheckboxes(namespace) {
        var boxes = el.querySelectorAll('[data-shared-user-id][data-shared-namespace="' + namespace + '"]');
        for (var i = 0; i < boxes.length; i++) {
          boxes[i].addEventListener('change', function () {
            var uid = this.getAttribute('data-shared-user-id');
            var target = (namespace === 'compose') ? 'composeSharedWith' : 'editingSharedWith';
            if (this.checked) {
              if (state[target].indexOf(uid) === -1) state[target].push(uid);
            } else {
              state[target] = state[target].filter(function (id) { return id !== uid; });
            }
          });
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
