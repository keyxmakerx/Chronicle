// rules_page.js -- a rules category's list and an entry's page
// (internal/systems/rules_list.templ, rules_page.templ).
//
// The list's name filter narrows the rows as you type; the server answers the
// same ?q= without this script, so the form still works when it cannot run.
// On an entry page the contents rail marks the section being read. Text only
// ever reaches the page through attributes the server wrote; nothing here
// builds markup. Chronicle.register('rules-page', ...).
(function () {
  'use strict';

  // filterRows hides the rows whose data-find does not hold q, and reports
  // how many stay. Exposed for the unit test.
  function filterRows(rows, q) {
    var want = String(q || '').trim().toLowerCase();
    var shown = 0;
    for (var i = 0; i < rows.length; i++) {
      var hit = !want || (rows[i].getAttribute('data-find') || '').indexOf(want) !== -1;
      rows[i].hidden = !hit;
      if (hit) shown++;
    }
    return shown;
  }

  function initFilter(el, state) {
    var field = el.querySelector('[data-rp-filter]');
    if (!field) return;
    var form = el.querySelector('[data-rp-filter-form]');
    var go = el.querySelector('[data-rp-filter-go]');
    var none = el.querySelector('[data-rp-none]');
    // Live filtering makes the Find button redundant.
    if (go) go.hidden = true;
    var apply = function () {
      var rows = el.querySelectorAll('li[data-find]');
      var shown = filterRows(rows, field.value);
      // A run whose rows are all hidden hides its heading too.
      el.querySelectorAll('.rp-leftout').forEach(function (sec) {
        sec.hidden = !sec.querySelector('li[data-find]:not([hidden])');
      });
      if (none) {
        none.hidden = shown > 0 || rows.length === 0;
        if (!none.hidden) none.textContent = 'Nothing matches “' + field.value.trim() + '”.';
      }
      try {
        var u = new URL(window.location.href);
        if (field.value.trim()) u.searchParams.set('q', field.value.trim()); else u.searchParams.delete('q');
        window.history.replaceState(window.history.state, '', u.pathname + u.search + u.hash);
      } catch (e) { /* the address keeps the server's query */ }
    };
    var onSubmit = function (e) { e.preventDefault(); apply(); };
    field.addEventListener('input', apply);
    if (form) form.addEventListener('submit', onSubmit);
    state.off.push(function () {
      field.removeEventListener('input', apply);
      if (form) form.removeEventListener('submit', onSubmit);
    });
  }

  function initRail(el, state) {
    var toc = el.querySelector('[data-rp-toc]');
    if (!toc || !('IntersectionObserver' in window)) return;
    var links = toc.querySelectorAll('[data-rp-toc-link]');
    var seen = {};
    var mark = function () {
      var current = null;
      for (var i = 0; i < links.length; i++) {
        var id = links[i].getAttribute('data-rp-toc-link');
        if (seen[id]) { current = id; break; }
      }
      if (!current && links.length) current = links[0].getAttribute('data-rp-toc-link');
      for (var j = 0; j < links.length; j++) {
        if (links[j].getAttribute('data-rp-toc-link') === current) links[j].setAttribute('aria-current', 'true');
        else links[j].removeAttribute('aria-current');
      }
    };
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (en) { seen[en.target.id] = en.isIntersecting; });
      mark();
    }, { rootMargin: '-15% 0px -55% 0px' });
    for (var k = 0; k < links.length; k++) {
      var sec = document.getElementById(links[k].getAttribute('data-rp-toc-link'));
      if (sec) io.observe(sec);
    }
    mark();
    state.off.push(function () { io.disconnect(); });
  }

  if (window.Chronicle && Chronicle.register) {
    Chronicle.register('rules-page', {
      init: function (el) {
        var state = { off: [] };
        el._rulesPage = state;
        initFilter(el, state);
        initRail(el, state);
        // The Director's amber words (rules_director.js), when the page has any.
        var d = window.ChronicleRulesDirector && window.ChronicleRulesDirector.init(el);
        if (d) state.off.push(function () { d.unwire(); });
      },
      destroy: function (el) {
        var state = el._rulesPage;
        if (!state) return;
        state.off.forEach(function (f) { f(); });
        el._rulesPage = null;
      },
    });
  }

  window.ChronicleRulesPage = { filterRows: filterRows };
})();
