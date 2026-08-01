// calendar_permissions.js — C-CAL-DASHBOARD-W5b. Drives the per-calendar
// permissions modal on the Calendars dashboard. Reuses the Q-V2-7 chip-row
// VisibilityEditor widget DOM (the same one events use) with the extra
// "GM only" mode; this self-contained driver (no coupling to the event drawer)
// opens the modal for a card, seeds the chip-row from the card's level+rules,
// and saves via PUT /campaigns/:id/calendars/:calId/visibility.
//
// Editor modes ↔ stored model: THE MAPPING NOW LIVES IN ONE PLACE.
//
// C-CALV4-DAYCARD [DC-10] SIGNED extracted buildVisibilityPayload / rulesToChips
// into cal_visibility.js, because the day card's editor would have been the
// THIRD copy and three copies of a permission mapping drift into three different
// answers to "who can see this". This driver is now a consumer, not an owner.
// bench.templ mounts cal_visibility.js immediately before this file, and both
// are `defer`, so document order is load order.
(function () {
  'use strict';

  // --- The shared mappers (re-exported under this file's historical test
  // handle, so the pin on the mapping keeps working through the move) --------

  var V = (typeof window !== 'undefined' && window.ChronicleCalVisibility) || null;

  if (typeof window !== 'undefined' && V) {
    window.__calPerm = { buildVisibilityPayload: V.buildVisibilityPayload, rulesToChips: V.rulesToChips };
  }

  // NO LOCAL FALLBACK, DELIBERATELY. A "just in case" copy of the mapping here
  // would be the third copy this extraction exists to delete, and it would be
  // the one nobody notices going stale. If the shared module is missing the
  // modal simply does not wire — a visible failure beats a silently divergent
  // permission write.
  function buildVisibilityPayload(mode, chipRules) { return V.buildVisibilityPayload(mode, chipRules); }
  function rulesToChips(rulesStr) { return V.rulesToChips(rulesStr); }

  // --- DOM driver --------------------------------------------------------

  function init() {
    if (typeof document === 'undefined' || !V) return;
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
