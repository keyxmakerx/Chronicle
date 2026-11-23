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

  function escapeHtml(s) { return Chronicle.escapeHtml(s); }
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

  // recipeOf lays the row's own settings over a built-in recipe, so the
  // server never has to hold a copy of the engine's built-ins.
  function recipeOf(G, plan) {
    if (!plan.base) return plan.recipe;
    var r = G.recipes.get(plan.base);
    if (!r) throw new Error('unknown recipe');
    var own = plan.recipe || {};
    Object.keys(own.makes || {}).forEach(function (k) { r.makes[k] = own.makes[k]; });
    Object.keys(own.details || {}).forEach(function (k) { r.details[k] = own.details[k]; });
    delete r.id; delete r.builtIn; // a copy, so the engine checks it like any recipe
    return r;
  }

  // oklchHex turns the engine's oklch() colours into the hex the calendar
  // stores; anything else comes back '' (no colour).
  function oklchHex(s) {
    var m = /^oklch\(\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)\s*\)$/.exec(s || '');
    if (!m) return '';
    var L = +m[1], C = +m[2], h = +m[3] * Math.PI / 180, a = C * Math.cos(h), b = C * Math.sin(h);
    var l = Math.pow(L + 0.3963377774 * a + 0.2158037573 * b, 3);
    var mm = Math.pow(L - 0.1055613458 * a - 0.0638541728 * b, 3);
    var sv = Math.pow(L - 0.0894841775 * a - 1.2914855480 * b, 3);
    return '#' + [
      4.0767416621 * l - 3.3077115913 * mm + 0.2309699292 * sv,
      -1.2684380046 * l + 2.6097574011 * mm - 0.3413193965 * sv,
      -0.0041960863 * l - 0.7034186147 * mm + 1.7076147010 * sv
    ].map(function (x) {
      x = Math.min(1, Math.max(0, x));
      x = x <= 0.0031308 ? 12.92 * x : 1.055 * Math.pow(x, 1 / 2.4) - 0.055;
      return ('0' + Math.round(x * 255).toString(16)).slice(-2);
    }).join('');
  }

  // runHistory lays the row's settings over its built-in recipe and keeps
  // the calendar's own eras: a generated era that overlaps one (an open end
  // runs on forever, as the server reads it) is left out, and when the plan
  // makes no eras none go back. Only the fields the server saves are sent.
  function runHistory(G, plan) {
    var recipe = JSON.parse(JSON.stringify(G.recipes.get(plan.basedOn)));
    ['makes', 'details'].forEach(function (part) {
      var over = (plan.recipe && plan.recipe[part]) || {};
      Object.keys(over).forEach(function (k) { recipe[part][k] = over[k]; });
    });
    var keep = plan.keepEras || [];
    var end = function (e) { return e.end_year == null ? Infinity : e.end_year; };
    var res = G.run('history', { recipe: recipe, seed: plan.seed, calendar: plan.calendar, locked: { eras: keep } });
    var eras = !plan.makesEras ? [] : (res.eras || []).filter(function (e) {
      return !(e.gen && e.gen.locked) && !keep.some(function (k) { return e.start_year <= end(k) && k.start_year <= end(e); });
    });
    return {
      eras: eras.map(function (e) {
        return { name: e.name, start_year: e.start_year, end_year: e.end_year == null ? null : e.end_year, description: e.description, color: oklchHex(e.color) };
      }),
      events: (res.events || []).map(function (e) {
        return { name: e.name, year: e.year, month: e.month, day: e.day, description: e.description, visibility: e.visibility };
      })
    };
  }

  // run turns one plan into the JSON the server's generator kind reads.
  function run(G, plan) {
    if (plan.generator === 'history') return runHistory(G, plan);
    var opts = { recipe: recipeOf(G, plan), seed: plan.seed };
    if (plan.calendar) opts.calendar = plan.calendar;
    if (plan.scope) opts.scope = plan.scope;
    if (Array.isArray(plan.locked)) opts.locked = plan.locked;
    // A place's weather runs as that place's one zone, from its own climate.
    if (Array.isArray(plan.zones) && plan.zones.length) opts.zones = plan.zones;
    // The owner's own kinds of weather; one the engine can't read is left
    // out, as the calendar's weather sheet does, rather than failing the run.
    if (Array.isArray(plan.kinds)) {
      opts.kinds = plan.kinds.filter(function (k) { return G.weather.validateKind(k, plan.kinds).ok; });
    }
    var res = G.run(plan.generator, opts);
    if (plan.generator === 'names') return (res.names && res.names[plan.namesKind]) || [];
    if (plan.generator === 'calendar') return res.preset;
    // Only the new moons go back; the kept ones are already on the calendar.
    if (plan.generator === 'moons') {
      return (res.moons || []).filter(function (m) { return !(m.gen && m.gen.locked); }).map(function (m) {
        return { name: m.name, cycle_days: m.cycle_days, phase_offset: m.phase_offset, color: m.color, hidden: m.visibility === 'dm_only' };
      });
    }
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
