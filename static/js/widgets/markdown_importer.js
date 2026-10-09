/**
 * markdown_importer.js — drag/drop + multi-file upload widget for
 * AI Workspace > Import.
 *
 * Mount: data-widget="markdown-importer", using child elements
 * data-importer-dropzone, data-importer-file-input (keyboard fallback),
 * data-importer-file-list, data-importer-file-announce (sr-only aria-live).
 *
 * Owns only the drag/preview UI (drag-active state, file list, a11y
 * announcements). Submission is the surrounding HTMX <form>'s job — the
 * widget never POSTs.
 */
(function () {
  'use strict';

  Chronicle.register('markdown-importer', {
    init: function (el) {
      var zone = el.querySelector('[data-importer-dropzone]');
      var input = el.querySelector('[data-importer-file-input]');
      var list = el.querySelector('[data-importer-file-list]');
      var announce = el.querySelector('[data-importer-file-announce]');
      if (!zone || !input || !list) {
        return;
      }

      ['dragenter', 'dragover'].forEach(function (evt) {
        zone.addEventListener(evt, function (e) {
          e.preventDefault();
          e.stopPropagation();
          zone.classList.add('ring-2', 'ring-accent', 'bg-accent/5');
          announceText(announce, 'Drop to attach markdown files.');
        });
      });
      ['dragleave', 'drop'].forEach(function (evt) {
        zone.addEventListener(evt, function (e) {
          e.preventDefault();
          e.stopPropagation();
          zone.classList.remove('ring-2', 'ring-accent', 'bg-accent/5');
        });
      });

      zone.addEventListener('drop', function (e) {
        var dropped = Array.from(e.dataTransfer.files || []).filter(function (f) {
          var n = f.name.toLowerCase();
          return n.endsWith('.md') || n.endsWith('.markdown');
        });
        if (dropped.length === 0) {
          announceText(announce, 'No markdown files in that drop.');
          return;
        }
        // Replace the input's FileList with the dropped files.
        // FileList is read-only; build a DataTransfer to assign.
        var dt = new DataTransfer();
        dropped.forEach(function (f) { dt.items.add(f); });
        input.files = dt.files;
        renderList(dropped, list);
        announceText(announce, dropped.length + ' file(s) attached.');
      });

      input.addEventListener('change', function () {
        var files = Array.from(input.files || []);
        renderList(files, list);
        if (files.length > 0) {
          announceText(announce, files.length + ' file(s) selected.');
        }
      });
    },
  });

  function announceText(target, msg) {
    if (!target) { return; }
    target.textContent = msg;
  }

  function renderList(files, list) {
    list.innerHTML = '';
    if (files.length === 0) {
      return;
    }
    files.forEach(function (f) {
      var li = document.createElement('div');
      li.innerHTML = '<i class="fa-solid fa-file-code mr-1" aria-hidden="true"></i> ' +
        escapeHtml(f.name) +
        ' <span class="text-fg-muted">(' + (f.size / 1024).toFixed(1) + ' KB)</span>';
      list.appendChild(li);
    });
  }

  function escapeHtml(s) {
    return s.replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
})();

/**
 * AI Import generator rows: Chronicle's generators run only in the browser,
 * so a review row with data-ai-gen holds the plan the server made (which
 * generator, over which days, with which seed). Pressing Import runs every
 * included plan, writes each result into the row's hidden
 * rec_N_generated field, then lets the request go. The server re-checks
 * every result against its own plan.
 */
(function () {
  'use strict';

  var ENGINE_RE = /^\/static\/js\/widgets\/chronicle_gen\.js(\?v=[A-Za-z0-9._-]{1,80})?$/;
  var enginePromise = null;
  function engine(src) {
    if (window.ChronicleGen) return Promise.resolve(window.ChronicleGen);
    if (enginePromise) return enginePromise;
    if (!ENGINE_RE.test(src || '')) return Promise.reject(new Error('generator engine not available'));
    enginePromise = new Promise(function (resolve, reject) {
      var s = document.createElement('script');
      s.src = src;
      s.onload = function () { window.ChronicleGen ? resolve(window.ChronicleGen) : reject(new Error('engine missing')); };
      s.onerror = function () { enginePromise = null; reject(new Error('engine failed to load')); };
      document.head.appendChild(s);
    });
    return enginePromise;
  }

  // run turns one plan into the JSON the server's generator kind reads.
  function run(G, plan) {
    var opts = { recipe: plan.recipe, seed: plan.seed };
    if (plan.calendar) opts.calendar = plan.calendar;
    if (plan.scope) opts.scope = plan.scope;
    var res = G.run(plan.generator, opts);
    if (plan.generator === 'names') return (res.names && res.names[plan.namesKind]) || [];
    if (plan.generator === 'weather') {
      return (res.days || []).map(function (d) {
        var w = G.weather.toWeatherInput(d);
        w.year = d.year; w.month = d.month; w.day = d.day;
        return w;
      });
    }
    return (res.events || []).map(function (e) {
      return {
        name: e.name, year: e.year, month: e.month, day: e.day,
        end_year: e.end_year, end_month: e.end_month, end_day: e.end_day,
        description: e.description, visibility: e.visibility, color: e.color, icon: e.icon,
        all_day: !!e.all_day, start_hour: e.start_hour, start_minute: e.start_minute,
        is_recurring: !!e.is_recurring, recurrence_type: e.recurrence_type
      };
    });
  }

  document.addEventListener('htmx:confirm', function (evt) {
    var form = evt.detail && evt.detail.elt;
    if (!form || form.id !== 'ai-import-review') return;
    var rows = Array.prototype.filter.call(form.querySelectorAll('[data-ai-gen]'), function (row) {
      var inc = row.querySelector('input[name$="_include"]');
      var out = row.querySelector('input[name$="_generated"]');
      return inc && inc.checked && out && !out.value;
    });
    if (!rows.length) return;
    evt.preventDefault();
    engine(form.getAttribute('data-engine-src')).then(function (G) {
      rows.forEach(function (row) {
        var out = row.querySelector('input[name$="_generated"]');
        try {
          out.value = JSON.stringify(run(G, JSON.parse(row.getAttribute('data-ai-gen'))));
        } catch (e) {
          out.value = ''; // the server reports the row as not run
        }
      });
    }, function () { /* rows go without output; the server reports them */ })
      .then(function () { evt.detail.issueRequest(true); });
  });
})();
