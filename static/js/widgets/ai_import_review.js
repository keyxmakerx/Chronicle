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
      // swapSimilar turns an add that is much like an existing page into a
      // change to that page (and back) by setting the row's own name and
      // action fields, so the commit reads what the owner sees. It lives here,
      // not in an inline handler, so the markup carries no script.
      swapSimilar: function (b) {
        var s = b.closest('.air-sim'), r = s.closest('[data-air-row]'), f = this.$root, d = s.dataset;
        var n = f.elements[d.nameField], a = f.elements[d.actionField];
        if (!n || !a) { return; }
        var to = d.state !== 'on';
        d.state = to ? 'on' : 'off';
        n.value = to ? d.matchName : d.sentName;
        a.value = to ? 'update' : 'create';
        var v = to ? 'Change' : 'Add';
        r.dataset.airVerb = v;
        var p = r.querySelector('.air-pill');
        if (p) { p.textContent = v; p.className = 'air-pill ' + (to ? 'is-change' : 'is-add'); }
        var w = r.querySelector('.air-what');
        if (w) {
          w.firstChild.textContent = (to ? 'Change the page' : 'Add the page') + ' ';
          var nb = w.querySelector('b');
          if (nb) { nb.textContent = to ? d.matchName : d.sentName; }
        }
        s.querySelectorAll('span[data-sim]').forEach(function (e) { e.hidden = (e.dataset.sim === 'on') !== to; });
        var g = s.querySelector('[data-sim-gm]');
        if (g) {
          g.hidden = !to;
          if (to) { r.setAttribute('data-air-gm', ''); } else { r.removeAttribute('data-air-gm'); }
        }
        b.textContent = to ? 'Add a new page instead' : 'Update that page instead';
        b.setAttribute('aria-label', to ? 'Add a new page again instead of changing ' + d.matchName : 'Update the page ' + d.matchName + ' instead of adding a new one');
        f.dispatchEvent(new Event('change', { bubbles: true }));
      },
      // toggleBoth opens and closes the side-by-side texts under a similar row.
      toggleBoth: function (b) {
        var p = b.closest('.air-sim').querySelector('.air-sim-both'), o = p.hidden;
        p.hidden = !o;
        b.setAttribute('aria-expanded', String(o));
        b.textContent = o ? 'Hide both' : 'See both';
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
