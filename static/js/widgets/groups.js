/**
 * groups.js -- Chronicle Campaign Groups Management Widget
 *
 * Full CRUD for campaign groups: create, rename, delete groups and
 * add/remove members. Auto-mounted by boot.js on elements with
 * data-widget="groups".
 *
 * Config (from data-* attributes):
 *   data-campaign-id     - Campaign ID
 *   data-csrf            - CSRF token for mutating requests
 *   data-groups-endpoint - Groups API base URL, e.g. /campaigns/:id/groups
 *   data-members-json    - JSON array of campaign members
 */
Chronicle.register('groups', {
  init: function (el, config) {
    var state = {
      groups: [],
      members: [],
      loading: true,
      error: null,
      editingGroup: null, // group ID being edited
      expandedGroup: null // group ID whose members are shown
    };

    try {
      state.members = JSON.parse(config.membersJson || '[]');
    } catch (e) {
      state.members = [];
    }

    var csrf = config.csrf || '';
    var endpoint = config.groupsEndpoint || '';

    // --- API helpers ---

    function apiFetch(url, opts) {
      opts = opts || {};
      opts.headers = opts.headers || {};
      opts.headers['Content-Type'] = 'application/json';
      if (csrf) opts.headers['X-CSRF-Token'] = csrf;
      opts.credentials = 'same-origin';
      return fetch(url, opts).then(function (r) {
        if (!r.ok) {
          return r.text().then(function (t) {
            var msg;
            try { msg = JSON.parse(t).message; } catch (e) { msg = t; }
            throw new Error(msg || 'Request failed');
          });
        }
        if (r.status === 204) return null;
        return r.json();
      });
    }

    function loadGroups() {
      state.loading = true;
      state.error = null;
      render();
      apiFetch(endpoint).then(function (data) {
        state.groups = data.groups || [];
        state.loading = false;
        render();
      }).catch(function (err) {
        state.error = err.message;
        state.loading = false;
        render();
      });
    }

    // --- Render ---

    function render() {
      var html = '';

      if (state.loading) {
        html = '<div class="text-fg-muted text-sm py-8 text-center">Loading groups…</div>';
        el.innerHTML = html;
        return;
      }

      if (state.error) {
        html += '<div class="alert-error mb-4" role="alert">' + escHtml(state.error) + '</div>';
      }

      // Create group form.
      html += '<div class="card p-4 mb-4">';
      html += '<h2 class="text-sm font-semibold text-fg mb-3">Create Group</h2>';
      html += '<div class="flex items-end gap-3">';
      html += '<div class="flex-1">';
      html += '<label class="block text-xs font-medium text-fg-body mb-1">Name</label>';
      html += '<input type="text" id="grp-new-name" class="input w-full" placeholder="e.g. Party A" maxlength="100"/>';
      html += '</div>';
      html += '<div class="flex-1">';
      html += '<label class="block text-xs font-medium text-fg-body mb-1">Description <span class="text-fg-muted">(optional)</span></label>';
      html += '<input type="text" id="grp-new-desc" class="input w-full" placeholder="Description" maxlength="500"/>';
      html += '</div>';
      html += '<button type="button" class="btn-primary text-sm" data-action="create">Create</button>';
      html += '</div>';
      html += '</div>';

      // Groups list.
      if (state.groups.length === 0) {
        html += '<div class="card p-8 text-center">';
        html += '<div class="text-fg-muted text-sm">No groups yet. Create one above to get started.</div>';
        html += '<p class="text-xs text-fg-secondary mt-2">Groups let you grant entity access to multiple users at once.</p>';
        html += '</div>';
      } else {
        for (var i = 0; i < state.groups.length; i++) {
          html += renderGroup(state.groups[i]);
        }
      }

      el.innerHTML = html;
      bindEvents();
    }

    function renderGroup(group) {
      var isExpanded = state.expandedGroup === group.id;
      var isEditing = state.editingGroup === group.id;
      var memberCount = group.members ? group.members.length : 0;

      var html = '<div class="card mb-3 overflow-hidden" data-group-id="' + group.id + '">';

      // Header row.
      html += '<div class="flex items-center justify-between px-4 py-3 bg-surface-alt border-b border-edge">';
      if (isEditing) {
        html += '<div class="flex items-center gap-2 flex-1">';
        html += '<input type="text" class="input text-sm flex-1" data-edit-name value="' + escAttr(group.name) + '" maxlength="100"/>';
        html += '<input type="text" class="input text-sm flex-1" data-edit-desc value="' + escAttr(group.description || '') + '" placeholder="Description"/>';
        html += '<button class="text-xs text-accent hover:text-accent-hover" data-action="save-edit" data-gid="' + group.id + '">Save</button>';
        html += '<button class="text-xs text-fg-muted hover:text-fg" data-action="cancel-edit">Cancel</button>';
        html += '</div>';
      } else {
        html += '<div class="flex items-center gap-2 cursor-pointer flex-1" data-action="toggle" data-gid="' + group.id + '">';
        html += '<i class="fa-solid fa-chevron-' + (isExpanded ? 'down' : 'right') + ' text-xs text-fg-muted w-3"></i>';
        html += '<i class="fa-solid fa-users text-fg-secondary text-sm"></i>';
        html += '<span class="font-medium text-fg text-sm">' + escHtml(group.name) + '</span>';
        if (group.description) {
          html += '<span class="text-fg-secondary text-xs ml-2">— ' + escHtml(group.description) + '</span>';
        }
        html += '<span class="text-xs text-fg-muted ml-2">(' + memberCount + ' member' + (memberCount !== 1 ? 's' : '') + ')</span>';
        html += '</div>';
        html += '<div class="flex items-center gap-2">';
        html += '<button class="text-xs text-fg-secondary hover:text-fg" data-action="edit" data-gid="' + group.id + '" title="Rename"><i class="fa-solid fa-pen text-xs"></i></button>';
        html += '<button class="text-xs text-red-600 dark:text-red-400 hover:text-red-500" data-action="delete" data-gid="' + group.id + '" data-gname="' + escAttr(group.name) + '" title="Delete"><i class="fa-solid fa-trash text-xs"></i></button>';
        html += '</div>';
      }
      html += '</div>';

      // Expanded member panel.
      if (isExpanded) {
        html += '<div class="px-4 py-3">';

        // Add member dropdown.
        var availableMembers = getAvailableMembers(group);
        if (availableMembers.length > 0) {
          html += '<div class="flex items-center gap-2 mb-3">';
          html += '<select class="input text-sm flex-1" data-member-select>';
          html += '<option value="">Add a member…</option>';
          for (var j = 0; j < availableMembers.length; j++) {
            var am = availableMembers[j];
            html += '<option value="' + escAttr(am.user_id) + '">' + escHtml(am.display_name) + ' (' + escHtml(am.email) + ') — ' + escHtml(am.role) + '</option>';
          }
          html += '</select>';
          html += '<button class="btn-primary text-xs" data-action="add-member" data-gid="' + group.id + '">Add</button>';
          html += '</div>';
        }

        // Current members.
        if (memberCount > 0) {
          html += '<div class="divide-y divide-edge">';
          for (var k = 0; k < group.members.length; k++) {
            var gm = group.members[k];
            html += '<div class="flex items-center justify-between py-2">';
            html += '<div>';
            html += '<span class="text-sm text-fg">' + escHtml(gm.display_name) + '</span>';
            html += '<span class="text-xs text-fg-secondary ml-2">' + escHtml(gm.email) + '</span>';
            html += '<span class="ml-2 text-xs px-1.5 py-0.5 rounded-full bg-surface-alt text-fg-muted">' + escHtml(gm.role) + '</span>';
            html += '</div>';
            html += '<button class="text-xs text-red-600 dark:text-red-400 hover:text-red-500" data-action="remove-member" data-gid="' + group.id + '" data-uid="' + escAttr(gm.user_id) + '" title="Remove from group"><i class="fa-solid fa-xmark"></i></button>';
            html += '</div>';
          }
          html += '</div>';
        } else {
          html += '<div class="text-sm text-fg-muted py-2">No members in this group yet.</div>';
        }

        html += '</div>';
      }

      html += '</div>';
      return html;
    }

    function getAvailableMembers(group) {
      var memberIds = {};
      if (group.members) {
        for (var i = 0; i < group.members.length; i++) {
          memberIds[group.members[i].user_id] = true;
        }
      }
      var available = [];
      for (var j = 0; j < state.members.length; j++) {
        if (!memberIds[state.members[j].user_id]) {
          available.push(state.members[j]);
        }
      }
      return available;
    }

    // --- Event binding ---

    function bindEvents() {
      el.addEventListener('click', handleClick);
    }

    function handleClick(e) {
      var btn = e.target.closest('[data-action]');
      if (!btn) return;

      var action = btn.getAttribute('data-action');
      var gid = btn.getAttribute('data-gid');

      switch (action) {
        case 'create':
          createGroup();
          break;
        case 'toggle':
          toggleGroup(parseInt(gid, 10));
          break;
        case 'edit':
          state.editingGroup = parseInt(gid, 10);
          render();
          break;
        case 'cancel-edit':
          state.editingGroup = null;
          render();
          break;
        case 'save-edit':
          saveGroupEdit(parseInt(gid, 10));
          break;
        case 'delete':
          deleteGroup(parseInt(gid, 10), btn.getAttribute('data-gname'));
          break;
        case 'add-member':
          addMember(parseInt(gid, 10));
          break;
        case 'remove-member':
          removeMember(parseInt(gid, 10), btn.getAttribute('data-uid'));
          break;
      }
    }

    // --- Actions ---

    function createGroup() {
      var nameInput = el.querySelector('#grp-new-name');
      var descInput = el.querySelector('#grp-new-desc');
      var name = nameInput ? nameInput.value.trim() : '';
      if (!name) {
        state.error = 'Group name is required.';
        render();
        return;
      }
      var desc = descInput ? descInput.value.trim() : '';
      var body = { name: name };
      if (desc) body.description = desc;

      state.error = null;
      apiFetch(endpoint, { method: 'POST', body: JSON.stringify(body) })
        .then(function () {
          loadGroups();
        })
        .catch(function (err) {
          state.error = err.message;
          render();
        });
    }

    function toggleGroup(gid) {
      if (state.expandedGroup === gid) {
        state.expandedGroup = null;
        render();
        return;
      }
      // Load members for this group.
      state.expandedGroup = gid;
      apiFetch(endpoint + '/' + gid).then(function (data) {
        // Update group in state with members.
        for (var i = 0; i < state.groups.length; i++) {
          if (state.groups[i].id === gid) {
            state.groups[i].members = data.members || [];
            break;
          }
        }
        render();
      }).catch(function (err) {
        state.error = err.message;
        render();
      });
    }

    function saveGroupEdit(gid) {
      var card = el.querySelector('[data-group-id="' + gid + '"]');
      if (!card) return;
      var nameInput = card.querySelector('[data-edit-name]');
      var descInput = card.querySelector('[data-edit-desc]');
      var name = nameInput ? nameInput.value.trim() : '';
      if (!name) {
        state.error = 'Group name is required.';
        render();
        return;
      }
      var desc = descInput ? descInput.value.trim() : '';
      var body = { name: name };
      if (desc) body.description = desc;
      else body.description = null;

      state.editingGroup = null;
      apiFetch(endpoint + '/' + gid, { method: 'PUT', body: JSON.stringify(body) })
        .then(function () {
          loadGroups();
        })
        .catch(function (err) {
          state.error = err.message;
          render();
        });
    }

    function deleteGroup(gid, gname) {
      if (!confirm('Delete group "' + (gname || 'this group') + '"? Any entity permission grants for this group will also be removed.')) {
        return;
      }
      apiFetch(endpoint + '/' + gid, { method: 'DELETE' })
        .then(function () {
          if (state.expandedGroup === gid) state.expandedGroup = null;
          loadGroups();
        })
        .catch(function (err) {
          state.error = err.message;
          render();
        });
    }

    function addMember(gid) {
      var card = el.querySelector('[data-group-id="' + gid + '"]');
      if (!card) return;
      var select = card.querySelector('[data-member-select]');
      if (!select || !select.value) return;
      var uid = select.value;

      apiFetch(endpoint + '/' + gid + '/members', {
        method: 'POST',
        body: JSON.stringify({ user_id: uid })
      }).then(function (data) {
        // Update members in state.
        for (var i = 0; i < state.groups.length; i++) {
          if (state.groups[i].id === gid) {
            state.groups[i].members = data.members || [];
            break;
          }
        }
        render();
      }).catch(function (err) {
        state.error = err.message;
        render();
      });
    }

    function removeMember(gid, uid) {
      apiFetch(endpoint + '/' + gid + '/members/' + uid, { method: 'DELETE' })
        .then(function () {
          // Remove from local state.
          for (var i = 0; i < state.groups.length; i++) {
            if (state.groups[i].id === gid && state.groups[i].members) {
              state.groups[i].members = state.groups[i].members.filter(function (m) {
                return m.user_id !== uid;
              });
              break;
            }
          }
          render();
        })
        .catch(function (err) {
          state.error = err.message;
          render();
        });
    }

    // --- Helpers ---

    function escHtml(s) {
      var d = document.createElement('div');
      d.textContent = s || '';
      return d.innerHTML;
    }

    function escAttr(s) {
      return (s || '').replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    // Initial load.
    loadGroups();

    // Store cleanup ref.
    el._groupsCleanup = function () {
      el.removeEventListener('click', handleClick);
    };
  },

  destroy: function (el) {
    if (el._groupsCleanup) {
      el._groupsCleanup();
      delete el._groupsCleanup;
    }
  }
});
