/**
 * ai_import_review.js -- the AI import review's Alpine component
 * (aiImportReview): it counts the answers in the form's own radios, keeps a
 * removal's tick required while it is answered Yes, and adds J/K/Y/N.
 *
 * Mount: <div data-widget="ai-import-review" x-data x-ignore> around the
 * review form in ImportReview (ai_workspace/import_review.templ), which is
 * swapped in under the paste box or inside a connected AI's request. Alpine
 * reads x-data the moment the form arrives and htmx strips a script tag that
 * comes with it, so this loads on sight and init hands the form to Alpine.
 */
(function () {
  function typing(ev) {
    var t = ev.target;
    return t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));
  }
  function answer(row, yes) {
    var radio = row.querySelector('.air-yn input[value="' + (yes ? 'on' : 'off') + '"]');
    if (radio) { radio.checked = true; }
  }
  window.aiImportReview = function (startYes) {
    return {
      yes: startYes || 0,
      removals: 0,
      at: -1,
      init: function () { this.sync(); },
      rows: function () { return Array.prototype.slice.call(this.$root.querySelectorAll('[data-air-row]')); },
      // sync recounts the answers and makes each removal's tick
      // required exactly while that removal is answered Yes.
      sync: function () {
        var yes = 0, removals = 0;
        this.rows().forEach(function (row) {
          var on = !!row.querySelector('.air-y input:checked');
          var confirm = row.querySelector('[data-air-confirm]');
          if (confirm) { confirm.required = on; }
          var gm = row.hasAttribute('data-air-gm') ? row.querySelector('[data-air-gm-tick]') : null;
          if (on && !(gm && !gm.checked)) { yes++; }
          if (on && row.dataset.airVerb === 'Remove') { removals++; }
        });
        this.yes = yes;
        this.removals = removals;
      },
      onChange: function () { this.sync(); },
      answerAll: function (yes) {
        // Yes skips removals (set back to No) and rows held on No
        // because the AI's request can't go through as written.
        this.rows().forEach(function (row) {
          if (yes && row.hasAttribute('data-air-hold')) { return; }
          answer(row, yes && row.dataset.airVerb !== 'Remove');
        });
        this.sync();
      },
      onKey: function (ev) {
        if (typing(ev) || ev.ctrlKey || ev.metaKey || ev.altKey || !this.$root.isConnected) { return; }
        var k = (ev.key || '').toLowerCase();
        if ('jkyn'.indexOf(k) < 0 || k.length !== 1) { return; }
        var rows = this.rows();
        if (!rows.length) { return; }
        var cur = rows.indexOf(document.activeElement && document.activeElement.closest('[data-air-row]'));
        if (cur >= 0) { this.at = cur; }
        // Y and N answer only a row the operator has moved to, so
        // a stray key never answers the first row unseen.
        var picked = this.at >= 0;
        if (k === 'j') { this.at = Math.min(rows.length - 1, this.at + 1); }
        if (k === 'k') { this.at = Math.max(0, this.at - 1); }
        if (this.at < 0) { this.at = 0; }
        var row = rows[this.at];
        row.focus();
        row.scrollIntoView({ block: 'nearest' });
        if ((k === 'y' || k === 'n') && picked) { answer(row, k === 'y'); this.sync(); }
        ev.preventDefault();
      },
      applyDefaultsToAll: function () {
        var form = this.$root;
        var bulkCategory = form.querySelector('[name=bulk_default_category]')?.value || '';
        var bulkVisibility = form.querySelector('[name=bulk_default_visibility]')?.value || '';
        var bulkConflict = form.querySelector('[name=bulk_default_conflict]')?.value || '';
        form.querySelectorAll('select[name$="_category"]').forEach(function (sel) {
          if (sel.name === 'bulk_default_category') return;
          if (bulkCategory && Array.from(sel.options).some(function (o) { return o.value === bulkCategory; })) {
            sel.value = bulkCategory;
            sel.dispatchEvent(new Event('change'));
          }
        });
        form.querySelectorAll('select[name$="_visibility"]').forEach(function (sel) {
          if (sel.name === 'bulk_default_visibility') return;
          if (bulkVisibility) { sel.value = bulkVisibility; }
        });
        form.querySelectorAll('select[name$="_conflict"]').forEach(function (sel) {
          if (sel.name === 'bulk_default_conflict') return;
          if (bulkConflict) { sel.value = bulkConflict; }
        });
      },
      skipAllConflicts: function () {
        this.$root.querySelectorAll('select[name$="_conflict"]').forEach(function (sel) {
          if (sel.name === 'bulk_default_conflict') return;
          sel.value = 'skip';
        });
      },
    };
  };

  Chronicle.register('ai-import-review', {
    init: function (el) { Chronicle.startAlpine(el); },
  });
})();
