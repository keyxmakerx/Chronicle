/**
 * permissions.js -- Chronicle Per-Entity Permissions Widget
 *
 * Manages entity visibility mode (everyone / dm_only / custom) and
 * fine-grained per-role and per-user permission grants.
 * Auto-mounted by boot.js on elements with data-widget="permissions".
 *
 * Config (from data-* attributes):
 *   data-endpoint  - Permissions API endpoint (GET/PUT), e.g. /campaigns/:id/entities/:eid/permissions
 *   data-editable  - "true" if user can modify permissions (Owner only)
 */
// Role constants matching Go permissions.RoleOwner.
var ROLE_OWNER = 3;

Chronicle.register('permissions', {
  init: function (el, config) {
    var state = {
      visibility: 'default',
      isPrivate: false,
      members: [],
      groups: [],
      permissions: [],
      loading: true,
      saving: false,
      saved: false,
      error: null,
      abortController: null
    };
    var editable = config.editable === true;

    // Inject scoped styles once.
    if (!document.getElementById('perm-widget-styles')) {
      var style = document.createElement('style');
      style.id = 'perm-widget-styles';
      style.textContent = [
        '.perm-section { border-top: 1px solid var(--color-edge, #e5e7eb); padding-top: 12px; margin-top: 12px; }',
        '.perm-radio-group { display: flex; flex-direction: column; gap: 8px; }',
        '.perm-radio-label { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-radius: 8px; cursor: pointer; border: 2px solid transparent; transition: all 0.15s; font-size: 14px; color: var(--color-fg-body, #374151); }',
        '.perm-radio-label:hover { background: var(--color-surface-alt, #f9fafb); }',
        '.perm-radio-label.active { border-color: var(--color-accent, #6366f1); background: var(--color-accent-subtle, #eef2ff); }',
        '.perm-radio-label input[type=radio] { accent-color: var(--color-accent, #6366f1); }',
        '.perm-radio-desc { font-size: 12px; color: var(--color-fg-secondary, #6b7280); margin-top: 1px; }',
        '.perm-grants { margin-top: 12px; }',
        '.perm-grant-row { display: flex; align-items: center; gap: 8px; padding: 6px 0; border-bottom: 1px solid var(--color-edge, #e5e7eb); }',
        '.perm-grant-row:last-child { border-bottom: none; }',
        '.perm-grant-name { flex: 1; font-size: 13px; color: var(--color-fg-body, #374151); display: flex; align-items: center; gap: 6px; }',
        '.perm-grant-name .perm-role-badge { font-size: 10px; padding: 1px 6px; border-radius: 9999px; background: var(--color-surface-alt, #f3f4f6); color: var(--color-fg-secondary, #6b7280); }',
        '.perm-toggle-group { display: flex; gap: 2px; }',
        '.perm-toggle { padding: 3px 10px; font-size: 12px; border: 1px solid var(--color-edge, #d1d5db); background: var(--color-surface, white); color: var(--color-fg-secondary, #6b7280); cursor: pointer; transition: all 0.15s; }',
        '.perm-toggle:first-child { border-radius: 6px 0 0 6px; }',
        '.perm-toggle:last-child { border-radius: 0 6px 6px 0; }',
        '.perm-toggle.active { background: var(--color-accent, #6366f1); color: white; border-color: var(--color-accent, #6366f1); }',
        '.perm-toggle:hover:not(.active) { background: var(--color-surface-alt, #f3f4f6); }',
        '.perm-toggle.disabled { opacity: 0.5; cursor: default; }',
        '.perm-warning { font-size: 12px; color: #d97706; padding: 8px 12px; background: #fffbeb; border: 1px solid #fde68a; border-radius: 6px; margin-top: 8px; }',
        '.dark .perm-warning { background: #451a03; border-color: #92400e; color: #fbbf24; }',
        '.perm-hint { font-size: 11px; color: var(--color-fg-muted, #9ca3af); margin-top: 6px; }',
        '.perm-status { font-size: 12px; height: 16px; margin-top: 6px; }',
        '.perm-status-saving { color: var(--color-fg-muted, #9ca3af); }',
        '.perm-status-saved { color: #16a34a; }',
        '.perm-status-error { color: #dc2626; }',
        '.perm-section-header { font-size: 12px; font-weight: 600; color: var(--color-fg-secondary, #6b7280); text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 4px; }',
        '.perm-readonly-badge { display: inline-flex; align-items: center; gap: 6px; padding: 6px 12px; border-radius: 6px; font-size: 13px; background: var(--color-surface-alt, #f3f4f6); color: var(--color-fg-body, #374151); }',
        '.perm-loading { padding: 12px; text-align: center; color: var(--color-fg-muted, #9ca3af); font-size: 13px; }'
      ].join('\n');
      document.head.appendChild(style);
    }

    // Role name mapping.
    var roleNames = { 1: 'Player', 2: 'Scribe', 3: 'Owner' };

    function getMode() {
      if (state.visibility === 'custom') return 'custom';
      if (state.isPrivate) return 'dm_only';
      return 'everyone';
    }

    function setMode(mode) {
      if (mode === 'everyone') {
        state.visibility = 'default';
        state.isPrivate = false;
        state.permissions = [];
      } else if (mode === 'dm_only') {
        state.visibility = 'default';
        state.isPrivate = true;
        state.permissions = [];
      } else if (mode === 'custom') {
        state.visibility = 'custom';
        state.isPrivate = false;
      }
    }

    function findGrant(subjectType, subjectId) {
      for (var i = 0; i < state.permissions.length; i++) {
        var p = state.permissions[i];
        if (p.subject_type === subjectType && p.subject_id === subjectId) {
          return p.permission;
        }
      }
      return 'none';
    }

    function setGrant(subjectType, subjectId, permission) {
      // Remove existing grant for this subject.
      state.permissions = state.permissions.filter(function (p) {
        return !(p.subject_type === subjectType && p.subject_id === subjectId);
      });
      // Add new grant if not "none".
      if (permission !== 'none') {
        state.permissions.push({
          subject_type: subjectType,
          subject_id: subjectId,
          permission: permission
        });
      }
    }

    function hasAnyGrants() {
      return state.permissions.length > 0;
    }

    function render() {
      el.innerHTML = '';

      if (state.loading) {
        var loadingDiv = document.createElement('div');
        loadingDiv.className = 'perm-loading';
        loadingDiv.textContent = 'Loading permissions...';
        el.appendChild(loadingDiv);
        return;
      }

      var mode = getMode();

      // Read-only mode for non-owners.
      if (!editable) {
        var badge = document.createElement('div');
        badge.className = 'perm-readonly-badge';
        var icon = mode === 'everyone' ? 'fa-globe' : mode === 'dm_only' ? 'fa-lock' : 'fa-shield-halved';
        var label = mode === 'everyone' ? 'Visible to everyone' : mode === 'dm_only' ? 'Private (GM only)' : 'Custom permissions';
        badge.innerHTML = '<i class="fa-solid ' + Chronicle.escapeHtml(icon) + ' text-xs"></i> ' + Chronicle.escapeHtml(label);
        el.appendChild(badge);
        return;
      }

      // Fieldset wrapper.
      var fieldset = document.createElement('fieldset');
      fieldset.className = 'perm-section';

      // Legend.
      var legend = document.createElement('legend');
      legend.className = 'text-sm font-semibold text-fg';
      legend.textContent = 'Visibility & Permissions';
      fieldset.appendChild(legend);

      // Radio group.
      var radioGroup = document.createElement('div');
      radioGroup.className = 'perm-radio-group';
      radioGroup.style.marginTop = '8px';

      var modes = [
        { value: 'everyone', icon: 'fa-globe', label: 'Everyone', desc: 'All campaign members can view and Scribes can edit.' },
        { value: 'dm_only', icon: 'fa-lock', label: 'DM Only', desc: 'Only Scribes and the campaign owner can see this page.' },
        { value: 'custom', icon: 'fa-shield-halved', label: 'Custom', desc: 'Choose exactly who can view or edit this page.' }
      ];

      modes.forEach(function (m) {
        var lbl = document.createElement('label');
        lbl.className = 'perm-radio-label' + (mode === m.value ? ' active' : '');

        var radio = document.createElement('input');
        radio.type = 'radio';
        radio.name = 'perm-mode';
        radio.value = m.value;
        radio.checked = mode === m.value;
        radio.addEventListener('change', function () {
          setMode(m.value);
          render();
          save();
        });

        var iconEl = document.createElement('i');
        iconEl.className = 'fa-solid ' + m.icon + ' text-sm';

        var textWrap = document.createElement('div');
        var labelText = document.createElement('div');
        labelText.textContent = m.label;
        var descText = document.createElement('div');
        descText.className = 'perm-radio-desc';
        descText.textContent = m.desc;
        textWrap.appendChild(labelText);
        textWrap.appendChild(descText);

        lbl.appendChild(radio);
        lbl.appendChild(iconEl);
        lbl.appendChild(textWrap);
        radioGroup.appendChild(lbl);
      });

      fieldset.appendChild(radioGroup);

      // Custom grants panel.
      if (mode === 'custom') {
        var grantsDiv = document.createElement('div');
        grantsDiv.className = 'perm-grants';

        // Role grants section.
        var roleHeader = document.createElement('div');
        roleHeader.className = 'perm-section-header';
        roleHeader.textContent = 'Role Permissions';
        grantsDiv.appendChild(roleHeader);

        var hintDiv = document.createElement('div');
        hintDiv.className = 'perm-hint';
        hintDiv.textContent = 'Granting access to Players also grants access to Scribes.';
        grantsDiv.appendChild(hintDiv);

        // Player and Scribe role rows.
        [1, 2].forEach(function (roleNum) {
          var row = createGrantRow(
            roleNames[roleNum], null, roleNum === 1 ? 'fa-user' : 'fa-pen',
            'role', String(roleNum),
            findGrant('role', String(roleNum))
          );
          grantsDiv.appendChild(row);
        });

        // User grants section.
        var nonOwnerMembers = state.members.filter(function (m) { return m.role < ROLE_OWNER; });
        if (nonOwnerMembers.length > 0) {
          var userHeader = document.createElement('div');
          userHeader.className = 'perm-section-header';
          userHeader.style.marginTop = '12px';
          userHeader.textContent = 'Individual Permissions';
          grantsDiv.appendChild(userHeader);

          nonOwnerMembers.forEach(function (member) {
            var row = createGrantRow(
              member.display_name || member.email, roleNames[member.role],
              'fa-user', 'user', member.user_id,
              findGrant('user', member.user_id)
            );
            grantsDiv.appendChild(row);
          });
        }

        // Group grants section.
        if (state.groups.length > 0) {
          var groupHeader = document.createElement('div');
          groupHeader.className = 'perm-section-header';
          groupHeader.style.marginTop = '12px';
          groupHeader.textContent = 'Group Permissions';
          grantsDiv.appendChild(groupHeader);

          state.groups.forEach(function (group) {
            var row = createGrantRow(
              group.name, null, 'fa-users',
              'group', String(group.id),
              findGrant('group', String(group.id))
            );
            grantsDiv.appendChild(row);
          });
        }

        // Owner members (greyed out).
        var owners = state.members.filter(function (m) { return m.role >= ROLE_OWNER; });
        if (owners.length > 0) {
          owners.forEach(function (member) {
            var row = document.createElement('div');
            row.className = 'perm-grant-row';
            row.style.opacity = '0.5';

            var nameDiv = document.createElement('div');
            nameDiv.className = 'perm-grant-name';
            nameDiv.innerHTML = '<i class="fa-solid fa-crown text-xs" style="color: #d97706;"></i> ' +
              Chronicle.escapeHtml(member.display_name || member.email);
            row.appendChild(nameDiv);

            var accessLabel = document.createElement('span');
            accessLabel.style.fontSize = '12px';
            accessLabel.style.color = 'var(--color-fg-muted, #9ca3af)';
            accessLabel.textContent = 'Full access';
            row.appendChild(accessLabel);

            grantsDiv.appendChild(row);
          });
        }

        // Warning if no grants in custom mode.
        if (!hasAnyGrants()) {
          var warning = document.createElement('div');
          warning.className = 'perm-warning';
          warning.innerHTML = '<i class="fa-solid fa-triangle-exclamation mr-1"></i> No grants set \u2014 only the campaign owner can see this page.';
          grantsDiv.appendChild(warning);
        }

        fieldset.appendChild(grantsDiv);
      }

      // Save status indicator.
      var statusDiv = document.createElement('div');
      statusDiv.className = 'perm-status';
      statusDiv.id = 'perm-status';
      fieldset.appendChild(statusDiv);

      el.appendChild(fieldset);
    }

    function createGrantRow(name, roleBadge, icon, subjectType, subjectId, currentPerm) {
      var row = document.createElement('div');
      row.className = 'perm-grant-row';

      var nameDiv = document.createElement('div');
      nameDiv.className = 'perm-grant-name';
      nameDiv.innerHTML = '<i class="fa-solid ' + Chronicle.escapeHtml(icon) + ' text-xs"></i> ' + Chronicle.escapeHtml(name);
      if (roleBadge) {
        nameDiv.innerHTML += ' <span class="perm-role-badge">' + Chronicle.escapeHtml(roleBadge) + '</span>';
      }
      row.appendChild(nameDiv);

      var toggleGroup = document.createElement('div');
      toggleGroup.className = 'perm-toggle-group';

      ['none', 'view', 'edit'].forEach(function (perm) {
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'perm-toggle' + (currentPerm === perm ? ' active' : '');
        btn.textContent = perm === 'none' ? 'None' : perm === 'view' ? 'View' : 'Edit';
        btn.addEventListener('click', function () {
          setGrant(subjectType, subjectId, perm);
          render();
          save();
        });
        toggleGroup.appendChild(btn);
      });

      row.appendChild(toggleGroup);
      return row;
    }

    function showStatus(type, message) {
      var statusEl = document.getElementById('perm-status');
      if (!statusEl) return;
      statusEl.className = 'perm-status perm-status-' + type;
      statusEl.textContent = message;
    }

    function save() {
      if (!editable || !config.endpoint) return;

      // Cancel in-flight request.
      if (state.abortController) {
        state.abortController.abort();
      }
      state.abortController = new AbortController();

      state.saving = true;
      showStatus('saving', 'Saving...');

      var body = {
        visibility: state.visibility,
        is_private: state.isPrivate,
        permissions: state.permissions.map(function (p) {
          return {
            subject_type: p.subject_type,
            subject_id: p.subject_id,
            permission: p.permission
          };
        })
      };

      Chronicle.apiFetch(config.endpoint, {
        method: 'PUT',
        body: body,
        signal: state.abortController.signal
      })
        .then(function (resp) {
          if (!resp.ok) {
            // Surface the server's specific error so the operator sees
            // the actual cause (validation, auth, etc.) instead of a
            // generic "Failed to save". Mirrors the layout-editor
            // widget's error handling.
            return resp.json().then(
              function (body) {
                var msg = (body && (body.message || body.error)) || ('Failed to save permissions (HTTP ' + resp.status + ')');
                throw new Error(msg);
              },
              function () {
                throw new Error('Failed to save permissions (HTTP ' + resp.status + ')');
              }
            );
          }
          state.saving = false;
          state.saved = true;
          showStatus('saved', 'Saved');
          setTimeout(function () {
            state.saved = false;
            showStatus('', '');
          }, 2000);
        })
        .catch(function (err) {
          if (err.name === 'AbortError') return;
          state.saving = false;
          showStatus('error', err.message || 'Error saving permissions');
          console.error('permissions: save error', err);
          Chronicle.notify(err.message || 'Failed to save permissions', 'error');
        });
    }

    function load() {
      if (!config.endpoint) {
        state.loading = false;
        render();
        return;
      }

      Chronicle.apiFetch(config.endpoint)
        .then(function (resp) {
          if (!resp.ok) throw new Error('Failed to load permissions');
          return resp.json();
        })
        .then(function (data) {
          state.visibility = data.visibility || 'default';
          state.isPrivate = data.is_private || false;
          state.members = data.members || [];
          state.groups = data.groups || [];
          state.permissions = (data.permissions || []).map(function (p) {
            return {
              subject_type: p.subject_type,
              subject_id: p.subject_id,
              permission: p.permission
            };
          });
          state.loading = false;
          render();
        })
        .catch(function (err) {
          state.loading = false;
          state.error = err.message;
          el.innerHTML = '<div class="perm-status perm-status-error">Failed to load permissions</div>';
          console.error('permissions: load error', err);
          Chronicle.notify('Failed to load permissions', 'error');
        });
    }

    el._permState = state;
    load();
  },

  destroy: function (el) {
    if (el._permState && el._permState.abortController) {
      el._permState.abortController.abort();
    }
    delete el._permState;
    el.innerHTML = '';
  }
});
