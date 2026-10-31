/**
 * Session edit dialog
 *
 * Save in the session page's Edit Session dialog (editSessionModal in
 * sessions.templ): reads the form, sends the partial update to the session's
 * own URL and reloads on success. Mounted by data-widget="session-edit" on the
 * dialog and loaded on sight, so it works whether the page was loaded directly
 * or reached by a boosted swap, where a script tag in the page is stripped.
 */
(function () {
  'use strict';

  function val(root, id) {
    var el = root.querySelector('#' + id);
    return el ? el.value : '';
  }

  function save(root, btn) {
    var name = val(root, 'edit-session-name').trim();
    if (!name) {
      Chronicle.notify('Session name is required', 'error');
      return;
    }
    var recurring = root.querySelector('#edit-session-recurring');
    var recType = root.querySelector('#edit-session-recurrence-type');
    var recInterval = root.querySelector('#edit-session-recurrence-interval');
    var recEnd = root.querySelector('#edit-session-recurrence-end');

    var body = {
      name: name,
      summary: val(root, 'edit-session-summary') || null,
      scheduled_date: val(root, 'edit-session-date') || null,
      scheduled_time: val(root, 'edit-session-time') || null,
      status: val(root, 'edit-session-status'),
      is_recurring: !!(recurring && recurring.checked)
    };
    if (body.is_recurring && recType) {
      body.recurrence_type = recType.value || null;
      body.recurrence_interval = recInterval ? parseInt(recInterval.value, 10) || 0 : 0;
      body.recurrence_end_date = recEnd ? (recEnd.value || null) : null;
    }

    btn.disabled = true;
    btn.innerHTML = '<i class="fa-solid fa-spinner fa-spin text-xs mr-1"></i> Saving...';
    Chronicle.apiFetch(location.pathname, { method: 'PUT', body: body })
      .then(function (res) {
        if (res.ok) {
          location.reload();
          return;
        }
        res.json().then(function (d) {
          Chronicle.notify(d.error || 'Failed to update session', 'error');
        }).catch(function () {
          Chronicle.notify('Failed to update session', 'error');
        });
      })
      .catch(function () {
        Chronicle.notify('Failed to update session', 'error');
      })
      .finally(function () {
        btn.disabled = false;
        btn.innerHTML = '<i class="fa-solid fa-check mr-1"></i> Save Changes';
      });
  }

  Chronicle.register('session-edit', {
    init: function (root) {
      var btn = root.querySelector('#edit-session-save-btn');
      if (!btn) return;
      root._sessionEditSave = function () { save(root, btn); };
      btn.addEventListener('click', root._sessionEditSave);
    },
    destroy: function (root) {
      var btn = root.querySelector('#edit-session-save-btn');
      if (btn && root._sessionEditSave) btn.removeEventListener('click', root._sessionEditSave);
      root._sessionEditSave = null;
    }
  });
})();
