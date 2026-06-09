// calendar_permissions.js — C-CAL-DASHBOARD-W5b. Drives the per-calendar
// permissions modal on the Calendars dashboard. Reuses the Q-V2-7 chip-row
// VisibilityEditor widget DOM (the same one events use) with the extra
// "GM only" mode; this self-contained driver (no coupling to the event drawer)
// opens the modal for a card, seeds the chip-row from the card's level+rules,
// and saves via PUT /campaigns/:id/calendars/:calId/visibility.
//
// Editor modes ↔ stored model (mirrors the W5a resolver):
//   public  → visibility "everyone", no rules
//   gmonly  → visibility "dm_only"  (hard GM gate; allow-list does NOT admit)
//   specific→ visibility "everyone" + {allowed_users,denied_users} whitelist/deny
(function () {
  'use strict';

  // --- Pure mappers (exposed for tests) ----------------------------------

  // buildVisibilityPayload maps the editor mode + chip rules to the PUT body.
  // Only user-kind rules persist (the calendar visibility_rules model is
  // user-scoped, identical to events' allowed_users/denied_users).
  function buildVisibilityPayload(mode, chipRules) {
    if (mode === 'gmonly') return { visibility: 'dm_only', visibility_rules: null };
    if (mode === 'public') return { visibility: 'everyone', visibility_rules: null };
    // specific
    var allowed = [], denied = [];
    (chipRules || []).forEach(function (r) {
      if (!r || r.kind !== 'user' || !r.target) return;
      if (r.mode === 'allow') allowed.push(r.target);
      else if (r.mode === 'deny') denied.push(r.target);
    });
    var rules = {};
    if (allowed.length) rules.allowed_users = allowed;
    if (denied.length) rules.denied_users = denied;
    var json = (allowed.length || denied.length) ? JSON.stringify(rules) : null;
    return { visibility: 'everyone', visibility_rules: json };
  }

  // rulesToChips converts a stored {allowed_users,denied_users} JSON string into
  // the chip-row rule array the editor renders.
  function rulesToChips(rulesStr) {
    if (!rulesStr) return [];
    var r;
    try { r = JSON.parse(rulesStr); } catch (e) { return []; }
    if (!r || typeof r !== 'object') return [];
    var chips = [];
    (r.allowed_users || []).forEach(function (u) { chips.push({ mode: 'allow', kind: 'user', target: u, label: u }); });
    (r.denied_users || []).forEach(function (u) { chips.push({ mode: 'deny', kind: 'user', target: u, label: u }); });
    return chips;
  }

  if (typeof window !== 'undefined') {
    window.__calPerm = { buildVisibilityPayload: buildVisibilityPayload, rulesToChips: rulesToChips };
  }

  // --- DOM driver --------------------------------------------------------

  function init() {
    if (typeof document === 'undefined') return;
    var modal = document.getElementById('cal-permissions-modal');
    if (!modal || modal.dataset.calPermWired === '1') return;
    modal.dataset.calPermWired = '1';

    var editor = modal.querySelector('[data-visibility-editor]');
    var nameEl = modal.querySelector('[data-cal-permissions-calname]');
    var state = { calId: '', rules: [] };

    function campaignID() { return window.location.pathname.split('/')[2]; }

    function specificPanel() { return editor && editor.querySelector('[data-visibility-specific-panel]'); }

    function currentMode() {
      var checked = editor.querySelector('input[type="radio"][data-visibility-mode]:checked');
      return checked ? checked.dataset.visibilityMode : 'public';
    }

    function updatePanel() {
      var panel = specificPanel();
      if (panel) panel.style.display = currentMode() === 'specific' ? '' : 'none';
    }

    function renderChips() {
      var row = editor && editor.querySelector('[data-visibility-chip-row]');
      if (!row) return;
      row.innerHTML = '';
      state.rules.forEach(function (rule, i) { row.appendChild(buildChip(rule, i)); });
      var hidden = editor.querySelector('[data-visibility-rules-json]');
      if (hidden) hidden.value = JSON.stringify(state.rules);
    }

    function buildChip(rule, i) {
      var span = document.createElement('span');
      var color = rule.mode === 'allow' ? 'border-green-500/40 bg-green-500/10' : 'border-amber-500/40 bg-amber-500/10';
      span.className = 'chip-add inline-flex items-center gap-1 text-xs rounded px-2 py-1 border ' + color;
      var icon = document.createElement('span');
      icon.className = rule.mode === 'allow' ? 'text-green-500' : 'text-amber-500';
      icon.innerHTML = rule.mode === 'allow' ? '<i class="fa-solid fa-check"></i>' : '<i class="fa-solid fa-ban"></i>';
      span.appendChild(icon);
      var label = document.createElement('span');
      label.className = 'text-fg';
      label.textContent = rule.label || rule.target;
      span.appendChild(label);
      var rm = document.createElement('button');
      rm.type = 'button';
      rm.className = 'text-fg-secondary hover:text-fg ml-1';
      rm.setAttribute('aria-label', 'Remove rule');
      rm.innerHTML = '<i class="fa-solid fa-xmark text-[10px]" aria-hidden="true"></i>';
      rm.addEventListener('click', function () { state.rules.splice(i, 1); renderChips(); });
      span.appendChild(rm);
      return span;
    }

    function setMode(mode) {
      editor.querySelectorAll('input[type="radio"][data-visibility-mode]').forEach(function (r) {
        r.checked = (r.dataset.visibilityMode === mode);
      });
      updatePanel();
    }

    function open(btn) {
      state.calId = btn.getAttribute('data-calendar-id') || '';
      state.rules = rulesToChips(btn.getAttribute('data-cal-vis-rules') || '');
      setMode(btn.getAttribute('data-cal-vis-mode') || 'public');
      renderChips();
      if (nameEl) {
        var card = btn.closest('[data-cal-dashboard-row]');
        var nm = card ? card.querySelector('a') : null;
        nameEl.textContent = nm ? nm.textContent.trim() : '';
      }
      modal.classList.remove('hidden');
    }

    function close() { modal.classList.add('hidden'); }

    function save() {
      var payload = buildVisibilityPayload(currentMode(), state.rules);
      if (!state.calId || !window.Chronicle || !Chronicle.apiFetch) { close(); return; }
      Chronicle.apiFetch('/campaigns/' + campaignID() + '/calendars/' + state.calId + '/visibility', {
        method: 'PUT', body: payload,
      }).then(function (resp) {
        if (resp.ok) { window.location.reload(); return; }
        resp.json().catch(function () { return {}; }).then(function (d) {
          if (Chronicle.notify) Chronicle.notify(d.message || 'Failed to save permissions', 'error');
        });
      }).catch(function (err) {
        if (Chronicle.notify) Chronicle.notify('Network error: ' + err.message, 'error');
      });
    }

    // Open from any card's Permissions button (delegated — survives grid swaps).
    document.addEventListener('click', function (e) {
      var trigger = e.target.closest('[data-cal-permissions]');
      if (trigger) { e.preventDefault(); open(trigger); return; }
      if (e.target.closest('[data-cal-permissions-close]') || e.target.closest('[data-cal-permissions-overlay]')) {
        if (!modal.classList.contains('hidden')) close();
      }
      var saveBtn = e.target.closest('[data-cal-permissions-save]');
      if (saveBtn && modal.contains(saveBtn)) save();
    });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && !modal.classList.contains('hidden')) close();
    });

    // Mode radios toggle the specific panel.
    editor && editor.querySelectorAll('input[type="radio"][data-visibility-mode]').forEach(function (r) {
      r.addEventListener('change', updatePanel);
    });

    // Allow/deny picker (reuse the widget's picker DOM).
    var picker = editor && editor.querySelector('[data-visibility-picker]');
    var pendingMode = 'allow';
    editor && editor.querySelectorAll('[data-visibility-add]').forEach(function (b) {
      b.addEventListener('click', function () {
        pendingMode = b.getAttribute('data-visibility-add') || 'allow';
        if (picker) picker.classList.remove('hidden');
      });
    });
    if (picker) {
      var input = picker.querySelector('[data-visibility-picker-input]');
      var kindEl = picker.querySelector('[data-visibility-picker-kind]');
      var confirm = picker.querySelector('[data-visibility-picker-confirm]');
      var cancel = picker.querySelector('[data-visibility-picker-cancel]');
      if (cancel) cancel.addEventListener('click', function () { picker.classList.add('hidden'); });
      if (confirm) confirm.addEventListener('click', function () {
        var target = input ? input.value.trim() : '';
        if (!target) return;
        state.rules.push({ mode: pendingMode, kind: kindEl ? kindEl.value : 'user', target: target, label: target });
        if (input) input.value = '';
        picker.classList.add('hidden');
        renderChips();
      });
    }
  }

  if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();
    // Re-init after boosted navigation (the QA2 convention) — guarded per modal.
    document.addEventListener('htmx:afterSettle', init);
    document.addEventListener('htmx:load', init);
  }
})();
