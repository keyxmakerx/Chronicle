/**
 * entity_notes.js -- Chronicle Player Notes Widget
 *
 * Per-user, per-entity notes with a 5-tier audience ACL: private (author
 * only), dm_only (Owner + IsDmGranted, Owner-authored), dm_scribe (adds
 * Scribe-tier authors), everyone, custom (sharedWith[]). The server
 * enforces the ACL on every read/write; data-can-author-* hints only
 * drive which options the UI offers — an unauthorized write is still
 * rejected server-side.
 *
 * Live updates via /ws entity_note.* messages, with 30s-poll fallback.
 * Auto-mounted by boot.js on data-widget="entity-notes".
 */
(function () {
  'use strict';

  // Audience options visible in the picker. Order = display order.
  // The dm_* options are hidden when the viewer can't author them
  // (see filterAudienceOptions). Server checks again.
  //
  // Each row carries audience info via its small badge (icon + label);
  // badgeClass styles that badge. The row itself stays neutral so the
  // campaign accent (hover, expanded-row tint, chevron, buttons) owns
  // the visual hierarchy instead of competing with per-audience colors.
  var ALL_AUDIENCES = [
    { value: 'private',   label: 'Private',    icon: 'fa-lock',        desc: 'Only you can see this',
      badgeClass: 'bg-slate-500/10 text-slate-600 dark:text-slate-400' },
    { value: 'dm_only',   label: 'DM Only',    icon: 'fa-user-secret', desc: 'GM (and DM-granted users) only',
      badgeClass: 'bg-red-500/10 text-red-700 dark:text-red-300' },
    { value: 'dm_scribe', label: 'DM + Co-DM', icon: 'fa-user-shield', desc: 'GM, Co-DMs, DM-granted users',
      badgeClass: 'bg-amber-500/10 text-amber-700 dark:text-amber-300' },
    { value: 'everyone',  label: 'Everyone',   icon: 'fa-users',       desc: 'All campaign members',
      badgeClass: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' },
    { value: 'custom',    label: 'Custom',     icon: 'fa-user-plus',   desc: 'Specific users (advanced)',
      badgeClass: 'bg-purple-500/10 text-purple-700 dark:text-purple-300' }
  ];

  Chronicle.register('entity-notes', {
    init: function (el, config) {
      var endpoint = config.endpoint || '';
      var campaignId = config.campaignId || '';
      var entityId = config.entityId || '';
      var canAuthorDMOnly = config.canAuthorDmOnly === true;
      var canAuthorDMScribe = config.canAuthorDmScribe === true;
      // The viewer's user ID, emitted by show.templ on the mount element.
      // Required to gate edit/pin/delete affordances to a note's author —
      // the server enforces author-only writes, but without this gate the
      // kebab menu shows up on every note and clicks return a confusing
      // 404 ("note not found") for any note the viewer didn't author.
      var currentUserId = config.currentUserId || '';

      var state = {
        notes: [],
        loading: true,
        composeOpen: false,
        composeAudience: 'private',
        composeTitle: '',
        composeBody: '',
        composeSharedWith: [],
        // expandedIds tracks which collapsed note rows are currently
        // open. A Set so multiple notes can be expanded simultaneously
        // (matches a file-browser feel — open multiple folders at once).
        expandedIds: {},
        // editingId is the single note currently in edit mode. Editing
        // implies expanded; collapsing the row cancels the edit.
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
      // (a thin wrapper around fetch()). Every call here funnels through
      // asJSON which (a) parses the body, (b) bubbles
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

      // Live updates and the poll call this too. The widget redraws as a
      // whole, so it holds the redraw while a note is being written or
      // edited (the typing would lose its cursor) and skips it when nothing
      // changed; closing the form draws whatever arrived meanwhile.
      var lastLoaded = null;
      function loadNotes() {
        Chronicle.apiFetch(endpoint)
          .then(asJSON)
          .then(function (notes) {
            state.notes = Array.isArray(notes) ? notes : [];
            var sig = JSON.stringify(state.notes);
            var unchanged = !state.loading && sig === lastLoaded;
            lastLoaded = sig;
            state.loading = false;
            if (unchanged || state.composeOpen || state.editingId) return;
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
          // Server-side WS auth reads the campaign from `?campaign=` (not
          // `?campaignId=`); mismatch causes a 401 even with a valid cookie.
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
          return m.user_id !== currentUserId;
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

      // isAuthor returns true when the viewer authored the given note.
      // Used to gate edit/pin/delete UI — the server enforces author-only
      // writes, but the UI must match or the user gets confusing 404s
      // when clicking Delete on a note that's visible-but-not-theirs.
      function isAuthor(note) {
        return !!note && !!currentUserId && note.authorUserId === currentUserId;
      }

      function renderList() {
        if (state.notes.length === 0) {
          return '<div class="card p-6 text-center"><i class="fa-solid fa-sticky-note text-2xl text-fg-muted mb-2"></i><p class="text-sm text-fg-muted">No notes yet. Click <strong>New Note</strong> to add one.</p></div>';
        }
        // Single bordered container with thin row dividers — like a
        // file-list. The wrapper override (`!p-0`) cancels .card's
        // default p-4 so rows go edge-to-edge. `overflow-visible` is
        // important: the kebab dropdown lives inside a row and would
        // otherwise be clipped by the wrapper.
        var h = '<div class="card !p-0 divide-y divide-edge overflow-visible">';
        for (var i = 0; i < state.notes.length; i++) {
          h += renderNote(state.notes[i]);
        }
        h += '</div>';
        return h;
      }

      // renderNote builds one row in the file-list.
      //
      // Three modes:
      //   collapsed   — header only (file-list row)
      //   expanded    — header + body content + action footer
      //   editing     — header + edit form (title/body/audience/picker)
      //
      // editing implies expanded; the kebab "Edit" entry expands the
      // row first. Cancel returns to expanded read-only.
      //
      // Audience info lives in the small badge in the header, not a
      // colored left border (which fights the campaign accent). Expanded
      // rows tint with the campaign accent and gain a 2px accent-colored
      // left border; collapsed rows have no left border.
      function renderNote(note) {
        var isExpanded = !!state.expandedIds[note.id] || state.editingId === note.id;
        var isEditing = state.editingId === note.id;
        var aud = audienceMeta(note.audience);
        var h = '';
        h += '<div class="' +
             (isExpanded ? 'border-l-2 border-l-accent bg-accent/5' : '') +
             '" data-note-id="' + esc(note.id) + '">';
        h += renderNoteHeader(note, aud, isExpanded, isEditing);
        if (isExpanded) {
          h += '<div class="border-t border-edge">';
          if (isEditing) {
            h += renderNoteEditor(note);
          } else {
            h += renderNoteContent(note);
          }
          h += '</div>';
        }
        h += '</div>';
        return h;
      }

      // renderNoteHeader is the always-visible row: chevron, title,
      // metadata, audience badge, pin indicator, kebab menu.
      // The whole row (except action buttons) toggles expand/collapse.
      //
      // Hover and the expanded chevron use the campaign accent for
      // consistency with the row tint. The date column has a fixed width
      // (w-16, right-aligned) so audience badges line up vertically across
      // rows regardless of date-text length.
      //
      // The kebab dropdown uses position: fixed (set in JS at click time)
      // so it escapes any clipping from ancestor overflow contexts.
      function renderNoteHeader(note, aud, isExpanded, isEditing) {
        var h = '';
        var titleText = note.title && note.title.trim()
          ? note.title
          : (firstLineOf(htmlToEditText(note.bodyHtml || '')) || 'Untitled');
        h += '<div class="px-2.5 py-1.5 flex items-center gap-2 select-none text-sm ' +
             (isEditing ? '' : 'cursor-pointer hover:bg-accent/5 transition-colors') + '" ' +
             (isEditing ? '' : 'data-action="toggle-note" data-note-id="' + esc(note.id) + '"') + '>';

        // Chevron — accent color when row is expanded so it ties into
        // the campaign theme (not just gray).
        h += '<i class="fa-solid fa-chevron-' + (isExpanded ? 'down' : 'right') +
             ' text-[9px] w-2.5 ' + (isExpanded ? 'text-accent' : 'text-fg-muted') + '"></i>';

        // Title (or first-line excerpt fallback), file-list weight
        // rather than a card title.
        h += '<span class="text-fg flex-1 truncate">' + esc(titleText) + '</span>';

        // Pinned indicator
        if (note.pinned) {
          h += '<i class="fa-solid fa-thumbtack text-[10px] text-amber-500 shrink-0" title="Pinned"></i>';
        }

        // Updated timestamp — fixed-width column. Renders BEFORE the
        // badge so the badge stays at a consistent horizontal offset
        // from the kebab regardless of how wide the date string is.
        // Hidden on small screens to save horizontal real estate.
        h += '<span class="text-[10px] text-fg-muted hidden lg:inline-block shrink-0 w-16 text-right">' + esc(formatShortDate(note.updatedAt)) + '</span>';

        // Audience badge — icon-only on small screens, +label on md+.
        h += '<span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium shrink-0 ' + aud.badgeClass + '" title="' + esc(aud.desc) + '">';
        h += '<i class="fa-solid ' + aud.icon + ' text-[9px] md:mr-1"></i>';
        h += '<span class="hidden md:inline">' + esc(aud.label) + '</span>';
        h += '</span>';

        // Kebab — only when not editing AND viewer authored the note.
        // The kebab opens the edit/pin/delete menu; all three are
        // author-only on the server, so hiding the entry point on
        // non-authored notes is the right UX. The audience badge plus
        // expanded body is what non-authors get.
        if (!isEditing && isAuthor(note)) {
          h += '<button class="text-fg-muted hover:text-accent text-xs p-1 shrink-0 transition-colors" data-action="note-menu" data-note-id="' + esc(note.id) + '" title="More actions" onclick="event.stopPropagation()"><i class="fa-solid fa-ellipsis-vertical"></i></button>';
        }

        h += '</div>';
        return h;
      }

      // renderNoteContent is the read-only expanded body view: rendered
      // HTML plus a small footer bar with primary actions.
      function renderNoteContent(note) {
        var h = '';
        h += '<div class="px-4 py-3">';
        if (note.bodyHtml) {
          h += '<div class="prose prose-sm dark:prose-invert max-w-none">' + note.bodyHtml + '</div>';
        } else if (note.body) {
          h += '<div class="text-sm text-fg whitespace-pre-wrap">' + esc(extractText(note.body)) + '</div>';
        } else {
          h += '<div class="text-sm text-fg-muted italic">(empty)</div>';
        }
        // Custom-audience: show recipients so the author remembers who has access.
        if (note.audience === 'custom' && Array.isArray(note.sharedWith) && note.sharedWith.length > 0) {
          h += '<div class="mt-3 pt-2 border-t border-edge text-xs text-fg-muted">';
          h += '<i class="fa-solid fa-user-plus mr-1"></i>Shared with ' + note.sharedWith.length + ' user' + (note.sharedWith.length === 1 ? '' : 's');
          h += '</div>';
        }
        // Action footer.
        // Edit/Pin are author-only (mirrors the server gate). For
        // non-authors we still render the footer so the "Created X" date
        // stays in place — it's read-only and useful context.
        h += '<div class="mt-3 pt-2 border-t border-edge flex items-center gap-2 text-xs">';
        if (isAuthor(note)) {
          h += '<button class="btn-secondary btn-sm" data-action="edit-note" data-note-id="' + esc(note.id) + '"><i class="fa-solid fa-pen mr-1"></i>Edit</button>';
          h += '<button class="btn-ghost btn-sm" data-action="toggle-pin" data-note-id="' + esc(note.id) + '"><i class="fa-solid fa-thumbtack mr-1"></i>' + (note.pinned ? 'Unpin' : 'Pin') + '</button>';
        }
        h += '<div class="flex-1"></div>';
        if (note.createdAt) {
          h += '<span class="text-fg-muted">Created ' + esc(formatShortDate(note.createdAt)) + '</span>';
        }
        h += '</div>';
        h += '</div>';
        return h;
      }

      function renderNoteEditor(note) {
        var h = '';
        h += '<div class="px-4 py-3 space-y-2 bg-surface-alt/30">';
        h += '<input type="text" class="input w-full text-sm" data-field="title" data-note-id="' + esc(note.id) + '" value="' + esc(state.editingTitle) + '" placeholder="Title (optional)"/>';
        h += '<textarea class="input w-full text-sm font-sans" rows="6" data-field="body" data-note-id="' + esc(note.id) + '" placeholder="Your note…">' + esc(state.editingBody) + '</textarea>';
        h += '<div class="flex items-center gap-2 flex-wrap">';
        h += renderAudiencePicker(state.editingAudience, 'edit-' + note.id);
        h += '<div class="flex-1"></div>';
        h += '<button class="btn-secondary text-xs" data-action="cancel-edit">Cancel</button>';
        h += '<button class="btn-primary text-xs" data-action="save-edit" data-note-id="' + esc(note.id) + '">Save changes</button>';
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
        // Toggle expand/collapse on row click.
        bindAll('[data-action="toggle-note"]', 'click', function (e) {
          // Bail out if the click landed on a nested action button —
          // those have their own handlers and we don't want clicking
          // "Edit" inside the kebab to also toggle the row state.
          if (e.target.closest('[data-action]') !== this) return;
          var id = this.getAttribute('data-note-id');
          if (state.expandedIds[id]) {
            delete state.expandedIds[id];
          } else {
            state.expandedIds[id] = true;
          }
          render();
        });

        // Kebab menu open/close.
        //
        // The menu is body-mounted with position: fixed so it never gets
        // clipped by any ancestor's overflow context. There's exactly
        // one menu element shared by every kebab button — we rebuild
        // its contents and position on each click. Click outside closes.
        bindAll('[data-action="note-menu"]', 'click', function (e) {
          e.stopPropagation();
          var id = this.getAttribute('data-note-id');
          var note = findNote(id);
          if (!note) return;
          openKebabMenu(this, note);
        });

        // Pin/unpin shortcut. Optimistic update so the star indicator
        // flips immediately; on error, the next poll cycle will undo.
        bindAll('[data-action="toggle-pin"]', 'click', function (e) {
          e.stopPropagation();
          var id = this.getAttribute('data-note-id');
          var note = findNote(id);
          if (!note) return;
          var newPinned = !note.pinned;
          updateNote(id, { pinned: newPinned })
            .then(render)
            .catch(function (err) {
              console.error('[EntityNotes] Pin error:', err);
              Chronicle.notify && Chronicle.notify(humanError(err), 'error');
            });
        });

        // Edit note — entry point from kebab menu OR from expanded
        // content footer "Edit" button. Auto-expands the row if the
        // user clicked Edit from a collapsed state.
        bindAll('[data-action="edit-note"]', 'click', function (e) {
          e.stopPropagation();
          var id = this.getAttribute('data-note-id');
          var note = findNote(id);
          if (!note) return;
          state.editingId = id;
          state.expandedIds[id] = true; // editing implies expanded
          state.editingTitle = note.title || '';
          state.editingBody = htmlToEditText(note.bodyHtml || '');
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
        bindAll('[data-action="delete-note"]', 'click', function (e) {
          e.stopPropagation();
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

      // --- Kebab menu (body-mounted, position: fixed) ---
      //
      // A single dropdown element is appended to document.body once per
      // widget instance. Every kebab click repositions it next to the
      // clicked button and rebuilds its contents for the right note.
      // Outside click closes it. position: fixed lets it escape any
      // ancestor overflow contexts cleanly.

      var menuEl = null;

      function ensureMenuEl() {
        if (menuEl) return menuEl;
        menuEl = document.createElement('div');
        menuEl.className = 'fixed z-[9999] hidden bg-surface border border-edge rounded-lg shadow-lg py-1 min-w-[140px]';
        menuEl.setAttribute('data-entity-notes-menu', '');
        document.body.appendChild(menuEl);
        // Outside-click closes. Bound once per widget instance via
        // the el.__cleanup for safe teardown on widget destroy.
        var outsideHandler = function (ev) {
          if (!menuEl || menuEl.classList.contains('hidden')) return;
          if (ev.target.closest('[data-entity-notes-menu]')) return;
          if (ev.target.closest('[data-action="note-menu"]')) return;
          menuEl.classList.add('hidden');
        };
        document.addEventListener('click', outsideHandler);
        // Reposition / close on scroll or resize so the menu doesn't
        // float in mid-air after the user scrolls the entity page.
        var closeOnInteraction = function () {
          if (menuEl) menuEl.classList.add('hidden');
        };
        window.addEventListener('scroll', closeOnInteraction, true);
        window.addEventListener('resize', closeOnInteraction);
        // Stash teardown refs on the widget root so destroy() can
        // remove the listeners + node.
        el.__entityNotesMenuTeardown = function () {
          document.removeEventListener('click', outsideHandler);
          window.removeEventListener('scroll', closeOnInteraction, true);
          window.removeEventListener('resize', closeOnInteraction);
          if (menuEl && menuEl.parentNode) menuEl.parentNode.removeChild(menuEl);
          menuEl = null;
        };
        return menuEl;
      }

      function openKebabMenu(button, note) {
        var menu = ensureMenuEl();
        menu.innerHTML = '' +
          '<button class="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-alt" data-menu-action="edit"><i class="fa-solid fa-pen mr-2 w-3"></i>Edit</button>' +
          '<button class="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-alt" data-menu-action="pin"><i class="fa-solid fa-thumbtack mr-2 w-3"></i>' + (note.pinned ? 'Unpin' : 'Pin') + '</button>' +
          '<button class="w-full text-left px-3 py-1.5 text-xs text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20" data-menu-action="delete"><i class="fa-solid fa-trash mr-2 w-3"></i>Delete</button>';

        // Wire menu items now that the contents are fresh.
        var items = menu.querySelectorAll('[data-menu-action]');
        for (var i = 0; i < items.length; i++) {
          items[i].addEventListener('click', function (ev) {
            ev.stopPropagation();
            var action = this.getAttribute('data-menu-action');
            menu.classList.add('hidden');
            handleMenuAction(action, note);
          });
        }

        // Position relative to the button. Right-aligned, just below.
        // Falls back to dropping ABOVE if there's no room below.
        var rect = button.getBoundingClientRect();
        // Show first (so we can read offsetWidth/Height) then move.
        menu.classList.remove('hidden');
        var menuW = menu.offsetWidth;
        var menuH = menu.offsetHeight;
        var top = rect.bottom + 4;
        if (top + menuH > window.innerHeight - 8) {
          top = rect.top - menuH - 4;
        }
        var left = rect.right - menuW;
        if (left < 8) left = 8;
        menu.style.top = top + 'px';
        menu.style.left = left + 'px';
      }

      function handleMenuAction(action, note) {
        if (action === 'edit') {
          // Reuse the existing edit-init logic by faking a click
          // through the same code path: set state and render.
          state.editingId = note.id;
          state.expandedIds[note.id] = true;
          state.editingTitle = note.title || '';
          state.editingBody = htmlToEditText(note.bodyHtml || '');
          state.editingAudience = note.audience || 'private';
          state.editingSharedWith = Array.isArray(note.sharedWith) ? note.sharedWith.slice() : [];
          render();
        } else if (action === 'pin') {
          updateNote(note.id, { pinned: !note.pinned })
            .then(render)
            .catch(function (err) {
              console.error('[EntityNotes] Pin error:', err);
              Chronicle.notify && Chronicle.notify(humanError(err), 'error');
            });
        } else if (action === 'delete') {
          if (!window.confirm('Delete this note? This cannot be undone.')) return;
          deleteNote(note.id)
            .then(render)
            .catch(function (err) {
              console.error('[EntityNotes] Delete error:', err);
              Chronicle.notify && Chronicle.notify(humanError(err), 'error');
            });
        }
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

      // htmlToEditText turns a note back into the text the edit box shows,
      // keeping paragraphs, line breaks and list items on their own lines
      // so saving it again (bodyToHTML) doesn't run them together.
      function htmlToEditText(html) {
        if (!html) return '';
        var div = document.createElement('div');
        div.innerHTML = html;
        var blocks = [];
        var line = '';
        function flush() {
          if (line.replace(/\s+/g, '') !== '') blocks.push(line.replace(/^\s+|\s+$/g, ''));
          line = '';
        }
        (function walk(node) {
          for (var c = node.firstChild; c; c = c.nextSibling) {
            if (c.nodeType === 3) { line += c.nodeValue; continue; }
            if (c.nodeType !== 1) continue;
            var tag = c.nodeName;
            if (tag === 'BR') { line += '\n'; continue; }
            var block = /^(P|DIV|LI|H[1-6]|BLOCKQUOTE|PRE|UL|OL)$/.test(tag);
            if (block) flush();
            if (tag === 'LI') line += '- ';
            walk(c);
            if (block) flush();
          }
        })(div);
        flush();
        return blocks.join('\n\n');
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

      // firstLineOf returns the first non-empty line of a string,
      // truncated. Used as a fallback display when a note has no
      // explicit title — gives the file-list row something readable.
      function firstLineOf(s) {
        if (!s) return '';
        var lines = String(s).split(/\r?\n/);
        for (var i = 0; i < lines.length; i++) {
          var line = lines[i].trim();
          if (line) {
            return line.length > 80 ? line.slice(0, 80) + '…' : line;
          }
        }
        return '';
      }

      // formatShortDate returns a compact relative-ish timestamp for
      // header rows. "Today 14:23", "Yesterday", "Apr 22", "2025-12-09".
      function formatShortDate(iso) {
        if (!iso) return '';
        var d = new Date(iso);
        if (isNaN(d.getTime())) return '';
        var now = new Date();
        var sameDay = d.getFullYear() === now.getFullYear() &&
                      d.getMonth() === now.getMonth() &&
                      d.getDate() === now.getDate();
        if (sameDay) {
          var hh = String(d.getHours()).padStart(2, '0');
          var mm = String(d.getMinutes()).padStart(2, '0');
          return hh + ':' + mm;
        }
        var yesterday = new Date(now);
        yesterday.setDate(now.getDate() - 1);
        var sameYesterday = d.getFullYear() === yesterday.getFullYear() &&
                            d.getMonth() === yesterday.getMonth() &&
                            d.getDate() === yesterday.getDate();
        if (sameYesterday) return 'Yesterday';
        var sameYear = d.getFullYear() === now.getFullYear();
        var months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
        if (sameYear) return months[d.getMonth()] + ' ' + d.getDate();
        return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
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
        // Tear down the body-mounted kebab menu + its global listeners.
        // Set up by ensureMenuEl on first kebab click.
        if (typeof el.__entityNotesMenuTeardown === 'function') {
          el.__entityNotesMenuTeardown();
          delete el.__entityNotesMenuTeardown;
        }
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
