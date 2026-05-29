// chronicle-canon-demo.js — C-V2-DESIGN-REBUILD Phase 1.8 demo init.
//
// Externalized from the prior inline demoScript() per dispatch §A.
// Core architecture changes:
//
//   1. Per-block error isolation. Each interactive feature registers
//      with INIT_BLOCKS; runAllInitBlocks() wraps each runner in its own
//      try/catch. One block failing can no longer kill every other
//      handler on the page (the silent-death failure mode that blocked
//      operator validation through PRs #375–#378).
//
//   2. __chronicleDemoInited is set AFTER the run, never before. The
//      prior code set the flag, then ran the work, so a throw left the
//      flag true and defeated the DOMContentLoaded retry.
//
//   3. Visible diagnostic dashboard. Init-block status, element binding
//      counts, browser-compat, UA, and a last-action log all render
//      into a panel at the top of the page so operator's next report
//      becomes "block X says FAILED with error Y" instead of "doesn't
//      work." The dashboard skeleton is the FIRST registered block and
//      has a document.title fallback if even the dashboard fails.
//
// No exotic browser features required: querySelector, dataset,
// addEventListener, localStorage. Vanilla JS.

(function () {
  'use strict';

  // ==========================================================
  // Init-block registry
  // ==========================================================
  var INIT_BLOCKS = [];
  function registerInitBlock(name, runner) {
    INIT_BLOCKS.push({ name: name, runner: runner });
  }

  // diagnostics — single mutable surface that the dashboard reads.
  var diagnostics = {
    blocks: [],     // [{name, status, error}]
    bindings: [],   // [{name, count}]
    features: [],   // [{name, supported}]
    actions: [],    // last-action log (most recent first; max 12)
    ua: '',
  };

  function recordBinding(name, count) {
    diagnostics.bindings.push({ name: name, count: count });
  }
  function recordFeature(name, supported) {
    diagnostics.features.push({ name: name, supported: !!supported });
  }
  function logAction(label) {
    var ts;
    try { ts = new Date().toLocaleTimeString(); } catch (e) { ts = '?'; }
    diagnostics.actions.unshift(ts + ' · ' + label);
    if (diagnostics.actions.length > 12) diagnostics.actions.length = 12;
    try { renderLastActionLog(); } catch (e) {}
  }

  function runAllInitBlocks() {
    for (var i = 0; i < INIT_BLOCKS.length; i++) {
      var b = INIT_BLOCKS[i];
      try {
        b.runner();
        diagnostics.blocks.push({ name: b.name, status: 'OK', error: null });
      } catch (err) {
        var msg = (err && err.message) ? err.message : String(err);
        diagnostics.blocks.push({ name: b.name, status: 'FAILED', error: msg });
        try { console.error('[chronicle-canon-demo] Init block failed:', b.name, err); } catch (e) {}
      }
    }
  }

  function init() {
    if (window.__chronicleDemoInited) return;
    runAllInitBlocks();
    try { renderDashboard(); } catch (e) {
      try { document.title = '[demo dashboard render failed] ' + document.title; } catch (e2) {}
    }
    window.__chronicleDemoInited = true;
    window.__chronicleDemoInitResults = diagnostics.blocks.slice();
    window.__chronicleDemoDiagnostics = diagnostics;
  }

  // ==========================================================
  // Block 1 — diagnostic dashboard skeleton + UA capture.
  // FIRST registered block; has its own try/catch + document.title
  // fallback so even a catastrophic failure here leaves a signal.
  // ==========================================================
  registerInitBlock('diagnostic-dashboard', function () {
    try {
      diagnostics.ua = (navigator && navigator.userAgent) || 'unknown';
      var panel = document.querySelector('[data-demo-diagnostics]');
      if (!panel) {
        try { document.title = '[demo: diagnostics panel markup missing] ' + document.title; } catch (e) {}
        throw new Error('[data-demo-diagnostics] panel markup not found');
      }
      panel.removeAttribute('hidden');
      var uaEl = panel.querySelector('[data-ua-string]');
      if (uaEl) uaEl.textContent = diagnostics.ua;
    } catch (err) {
      try { document.title = '[demo dashboard skeleton fail] ' + document.title; } catch (e) {}
      throw err;
    }
  });

  // ==========================================================
  // Block 2 — browser-compat feature detection.
  // ==========================================================
  registerInitBlock('browser-compat-detect', function () {
    function supports(prop, val) {
      try {
        if (val === undefined) return typeof CSS !== 'undefined' && CSS.supports(prop);
        return typeof CSS !== 'undefined' && CSS.supports(prop, val);
      } catch (e) { return false; }
    }
    recordFeature('oklch', supports('color', 'oklch(0.5 0.1 180)'));
    recordFeature('color-mix(oklch)', supports('color', 'color-mix(in oklch, red, blue)'));
    recordFeature('light-dark', supports('color', 'light-dark(red, blue)'));
    recordFeature('container-queries', supports('container-type', 'inline-size'));
    recordFeature('anchor-positioning', supports('anchor-name', '--x'));
    recordFeature('popover API', typeof HTMLElement !== 'undefined' && 'showPopover' in HTMLElement.prototype);
    recordFeature('view-transitions', typeof document.startViewTransition === 'function');
    recordFeature(':has() selector', supports('selector(:has(*))'));
    recordFeature('clipboard.writeText', !!(navigator.clipboard && navigator.clipboard.writeText));
    recordFeature('localStorage', (function () {
      try { localStorage.setItem('__t', '1'); localStorage.removeItem('__t'); return true; } catch (e) { return false; }
    })());
  });

  // ==========================================================
  // Block 3 — theme toggle.
  // Hydrates from localStorage > prefers-color-scheme > dark default.
  // ==========================================================
  registerInitBlock('theme-toggle', function () {
    var root = document.querySelector('[data-chronicle-demo]');
    if (!root) throw new Error('demo root not found');

    function applyTheme(theme) {
      root.dataset.chronicleDemoTheme = theme;
      if (theme === 'dark') root.classList.add('dark');
      else root.classList.remove('dark');
    }

    var theme = 'dark';
    try {
      var stored = localStorage.getItem('chronicle-demo-theme');
      if (stored === 'light' || stored === 'dark') {
        theme = stored;
      } else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) {
        theme = 'light';
      }
    } catch (e) {
      if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) theme = 'light';
    }
    applyTheme(theme);

    var btns = document.querySelectorAll('[data-action="theme"]');
    btns.forEach(function (b) {
      b.setAttribute('aria-pressed', b.dataset.value === theme ? 'true' : 'false');
      b.addEventListener('click', function () {
        var v = b.dataset.value;
        applyTheme(v);
        btns.forEach(function (o) { o.setAttribute('aria-pressed', o.dataset.value === v ? 'true' : 'false'); });
        try { localStorage.setItem('chronicle-demo-theme', v); } catch (e) {}
        logAction('Theme toggle → ' + v);
      });
    });
    recordBinding('theme buttons', btns.length);
  });

  // ==========================================================
  // Block 4 — reduced-motion toggle.
  // ==========================================================
  registerInitBlock('reduced-motion-toggle', function () {
    var root = document.querySelector('[data-chronicle-demo]');
    if (!root) throw new Error('demo root not found');
    var btns = document.querySelectorAll('[data-action="reduce-motion"]');
    btns.forEach(function (b) {
      b.addEventListener('click', function () {
        var v = b.dataset.value;
        root.dataset.chronicleDemoReduceMotion = v;
        btns.forEach(function (o) { o.setAttribute('aria-pressed', o.dataset.value === v ? 'true' : 'false'); });
        logAction('Reduced motion → ' + v);
      });
    });
    recordBinding('reduced-motion buttons', btns.length);
  });

  // ==========================================================
  // Choice-picker — state store.
  // Decision shape:
  //   rate:<id>   → { type:'rate',   label, rating, note }
  //   choose:<g>  → { type:'choose', label, votes:{value:'up'|'down'}, chosen, note }
  // ==========================================================
  var DECISIONS_KEY = 'chronicle-canon-decisions';
  var decisions = {};

  function persistDecisions() {
    try { localStorage.setItem(DECISIONS_KEY, JSON.stringify(decisions)); } catch (e) {}
  }
  function setDecision(id, patch) {
    decisions[id] = Object.assign({}, decisions[id], patch);
    persistDecisions();
    try { renderDecisionsPanel(); } catch (e) {}
  }
  function variantGroupLabel(g) {
    return ({
      bg: 'Dark background tint', accent: 'Accent palette', radius: 'Corner radius',
      shadow: 'Shadow / elevation', hover: 'Card hover style', motion: 'Motion speed',
      density: 'Density', selected: 'Selected-state', button: 'Button family',
    })[g] || g;
  }

  // ==========================================================
  // Block 5 — decision-store hydrate.
  // Loads the prior session's decisions; tolerant of legacy shapes
  // (PR #378's storage was compatible — same key + JSON shape).
  // ==========================================================
  registerInitBlock('decision-store-hydrate', function () {
    try {
      var raw = localStorage.getItem(DECISIONS_KEY);
      if (raw) decisions = JSON.parse(raw) || {};
    } catch (e) {
      decisions = {};
    }
    // Phase 1.9 — render immediately after hydrate so the panel
    // reflects the persisted state even if a later init block throws
    // before block 9 (decisions-panel) can do its end-of-init render.
    // No-op when the panel markup isn't present yet (querySelector
    // returns null and renderDecisionsPanel handles missing nodes).
    try { renderDecisionsPanel(); } catch (e) {}
  });

  // ==========================================================
  // Block 6 — rate-mode pills + notes.
  // ==========================================================
  registerInitBlock('choice-picker-rate', function () {
    var groups = document.querySelectorAll('[data-rate-id]');
    var pillCount = 0;
    groups.forEach(function (grp) {
      var id = grp.getAttribute('data-rate-id');
      var label = grp.getAttribute('data-rate-label') || id;
      var key = 'rate:' + id;
      var prior = decisions[key];
      // Hydrate UI from prior state.
      grp.querySelectorAll('[data-rate]').forEach(function (p) {
        pillCount++;
        if (prior && String(prior.rating) === p.getAttribute('data-rate')) {
          p.setAttribute('aria-pressed', 'true');
        }
        p.addEventListener('click', function () {
          grp.querySelectorAll('[data-rate]').forEach(function (o) {
            o.setAttribute('aria-pressed', o === p ? 'true' : 'false');
          });
          var rating = parseInt(p.getAttribute('data-rate'), 10);
          setDecision(key, { type: 'rate', label: label, rating: rating, note: (decisions[key] || {}).note || '' });
          (grp.closest('[data-picker-item]') || grp).setAttribute('data-answered', 'true');
          logAction('Rate ' + label + ' → ' + rating + '/5');
        });
      });
      var note = grp.querySelector('[data-note]');
      if (note) {
        if (prior && prior.note) note.value = prior.note;
        note.addEventListener('input', function () {
          var existing = decisions[key] || { type: 'rate', label: label };
          setDecision(key, Object.assign({}, existing, { note: note.value }));
        });
      }
    });
    recordBinding('rate-mode pills', pillCount);
  });

  // ==========================================================
  // Block 7 — choose-mode 👍/👎 votes.
  // ==========================================================
  registerInitBlock('choice-picker-vote', function () {
    var groups = document.querySelectorAll('[data-vote-id]');
    var btnCount = 0;
    groups.forEach(function (vg) {
      var id = vg.getAttribute('data-vote-id');
      var sep = id.indexOf('::');
      var group = id.slice(0, sep);
      var value = id.slice(sep + 2);
      var key = 'choose:' + group;
      var prior = decisions[key];
      // Hydrate
      if (prior && prior.votes && prior.votes[value]) {
        vg.querySelectorAll('[data-vote]').forEach(function (b) {
          b.setAttribute('aria-pressed', b.getAttribute('data-vote') === prior.votes[value] ? 'true' : 'false');
        });
      }
      vg.querySelectorAll('[data-vote]').forEach(function (b) {
        btnCount++;
        b.addEventListener('click', function () {
          var cur = decisions[key] || { type: 'choose', label: variantGroupLabel(group), votes: {} };
          cur.votes = cur.votes || {};
          var dir = b.getAttribute('data-vote');
          if (cur.votes[value] === dir) delete cur.votes[value];
          else cur.votes[value] = dir;
          vg.querySelectorAll('[data-vote]').forEach(function (o) {
            o.setAttribute('aria-pressed', (cur.votes[value] && o.getAttribute('data-vote') === cur.votes[value]) ? 'true' : 'false');
          });
          setDecision(key, cur);
          logAction('Vote ' + group + '/' + value + ' → ' + (cur.votes[value] || 'cleared'));
        });
      });
    });
    recordBinding('vote buttons', btnCount);
  });

  // ==========================================================
  // Block 8 — variant Apply (live token mutation on demo root).
  // Scope-isolated: only mutates --chronicle-* on the demo root.
  // Never touches global --color-accent-rgb (audit §6).
  // ==========================================================
  function applyVariant(group, value) {
    var root = document.querySelector('[data-chronicle-demo]');
    if (!root) return;
    if (group === 'bg') {
      root.setAttribute('data-chronicle-bg', value);
    } else if (group === 'accent') {
      var am = {
        indigo: 'oklch(0.58 0.20 270)',
        emerald: 'oklch(0.65 0.15 155)',
        amber: 'oklch(0.78 0.16 75)',
        rose: 'oklch(0.65 0.20 22)',
      };
      root.style.setProperty('--chronicle-accent', am[value] || am.indigo);
    } else if (group === 'radius') {
      var rs = { sharp: ['4px', '4px', '6px'], medium: ['6px', '8px', '12px'], soft: ['12px', '14px', '18px'] }[value];
      if (rs) {
        root.style.setProperty('--chronicle-radius', rs[0]);
        root.style.setProperty('--chronicle-radius-md', rs[1]);
        root.style.setProperty('--chronicle-radius-lg', rs[2]);
      }
    } else if (group === 'shadow') {
      var sh = {
        none: ['none', 'none', 'none'],
        subtle: ['0 1px 2px 0 oklch(0 0 0 / 0.04)', '0 4px 12px -2px oklch(0 0 0 / 0.10)', '0 12px 32px -4px oklch(0 0 0 / 0.18)'],
        pronounced: ['0 2px 6px -1px oklch(0 0 0 / 0.14)', '0 8px 20px -4px oklch(0 0 0 / 0.22)', '0 18px 40px -6px oklch(0 0 0 / 0.30)'],
        heavy: ['0 4px 10px -1px oklch(0 0 0 / 0.24)', '0 14px 30px -4px oklch(0 0 0 / 0.34)', '0 26px 56px -8px oklch(0 0 0 / 0.42)'],
      }[value];
      if (sh) {
        root.style.setProperty('--chronicle-elev-2-shadow', sh[0]);
        root.style.setProperty('--chronicle-elev-3-shadow', sh[1]);
        root.style.setProperty('--chronicle-elev-4-shadow', sh[2]);
      }
    } else if (group === 'hover') {
      root.setAttribute('data-hover-style', value);
    } else if (group === 'motion') {
      var mo = { snappy: ['100ms', '150ms'], canon: ['150ms', '250ms'], relaxed: ['250ms', '400ms'] }[value];
      if (mo) {
        root.style.setProperty('--chronicle-motion-base', mo[0]);
        root.style.setProperty('--chronicle-motion-medium', mo[1]);
      }
    } else if (group === 'density') {
      root.setAttribute('data-chronicle-density', value);
    } else if (group === 'selected') {
      root.setAttribute('data-selected-style', value);
    }
  }

  registerInitBlock('choice-picker-apply', function () {
    var btns = document.querySelectorAll('[data-variant-apply]');
    btns.forEach(function (b) {
      b.addEventListener('click', function () {
        var g = b.getAttribute('data-variant-group');
        var v = b.getAttribute('data-variant');
        applyVariant(g, v);
        document.querySelectorAll('.chronicle-variant-option[data-variant-group="' + g + '"]').forEach(function (opt) {
          opt.setAttribute('data-chosen', opt.getAttribute('data-variant') === v ? 'true' : 'false');
        });
        var key = 'choose:' + g;
        var cur = decisions[key] || { type: 'choose', label: variantGroupLabel(g), votes: {} };
        cur.chosen = v;
        setDecision(key, cur);
        logAction('Apply ' + g + ' → ' + v);
      });
    });
    recordBinding('variant Apply buttons', btns.length);

    // Re-apply any previously-chosen variants so the page reflects the
    // last session's picks across reloads.
    Object.keys(decisions).forEach(function (k) {
      if (k.indexOf('choose:') === 0 && decisions[k] && decisions[k].chosen) {
        applyVariant(k.slice(7), decisions[k].chosen);
      }
    });
  });

  // ==========================================================
  // Block 9 — "Your Decisions" panel.
  //
  // Phase 1.9 rewrite: render is bulletproof against the Bug #2 mode
  // from PR #379 (counter showed 5, body said "No decisions yet").
  // The new render ALWAYS:
  //   - Computes ratings/votes/applied tallies from a unified pass
  //     over `decisions`.
  //   - Populates the visible [data-decisions-summary] pills with the
  //     tallies (so a render bug in the textarea can't silently hide
  //     non-zero state — the summary is the authoritative visible UI).
  //   - Sets out.value unconditionally (never leaves the textarea at a
  //     server-rendered default).
  //   - Emits the markdown with three subsections (Ratings / Votes /
  //     Applied) keyed off explicit type-and-content discriminators.
  // ==========================================================
  function summarizeDecisions() {
    // Single pass over the store; classifies each entry into one
    // or more buckets. A 'choose' entry can carry BOTH votes AND a
    // chosen variant — it contributes to both Votes and Applied.
    var ratings = []; // { key, label, rating, note }
    var votes   = []; // { key, label, ups:[], downs:[], note }
    var applied = []; // { key, label, chosen, note }
    Object.keys(decisions).forEach(function (k) {
      var d = decisions[k];
      if (!d) return;
      if (d.type === 'rate' && (d.rating || (d.note && d.note.trim()))) {
        ratings.push({ key: k, label: d.label || k, rating: d.rating || 0, note: d.note || '' });
        return;
      }
      if (d.type === 'choose') {
        var ups = [], downs = [];
        Object.keys(d.votes || {}).forEach(function (opt) {
          if (d.votes[opt] === 'up') ups.push(opt);
          else if (d.votes[opt] === 'down') downs.push(opt);
        });
        if (ups.length || downs.length) {
          votes.push({ key: k, label: d.label || k, ups: ups, downs: downs, note: d.note || '' });
        }
        if (d.chosen) {
          applied.push({ key: k, label: d.label || k, chosen: d.chosen, note: d.note || '' });
        }
      }
    });
    return { ratings: ratings, votes: votes, applied: applied };
  }
  function buildDecisionsMarkdown(summary) {
    var lines = [];
    lines.push('# Chronicle Canon — Design Decisions');
    lines.push('_' + (navigator.userAgent || '') + ' · ' + new Date().toISOString().slice(0, 10) + '_');
    lines.push('');
    if (summary.ratings.length) {
      lines.push('## Ratings (locked decisions — 1–5)');
      summary.ratings.forEach(function (r) {
        var line = '- ' + r.label + ': ' + (r.rating ? r.rating + '/5' : '—');
        if (r.note) line += ' · note: ' + r.note;
        lines.push(line);
      });
      lines.push('');
    }
    if (summary.votes.length) {
      lines.push('## Votes (which options you prefer)');
      summary.votes.forEach(function (v) {
        var line = '- ' + v.label + ':';
        if (v.ups.length) line += ' 👍 ' + v.ups.join(', ');
        if (v.downs.length) line += (v.ups.length ? ' ·' : '') + ' 👎 ' + v.downs.join(', ');
        if (v.note) line += ' · note: ' + v.note;
        lines.push(line);
      });
      lines.push('');
    }
    if (summary.applied.length) {
      lines.push('## Applied (variants you live-previewed)');
      summary.applied.forEach(function (a) {
        var line = '- ' + a.label + ': ' + a.chosen;
        if (a.note) line += ' · note: ' + a.note;
        lines.push(line);
      });
      lines.push('');
    }
    if (!summary.ratings.length && !summary.votes.length && !summary.applied.length) {
      lines.push('_No decisions yet — start rating, voting, or applying above._');
    }
    return lines.join('\n');
  }
  function renderDecisionsPanel() {
    var summary = summarizeDecisions();
    // Header counter — sum of buckets the operator has actually
    // touched. (A single 'choose' entry with BOTH votes and chosen
    // counts as 2 of these — that matches the visible summary pills.)
    var total = summary.ratings.length + summary.votes.length + summary.applied.length;
    var countEl = document.querySelector('[data-decisions-count]');
    if (countEl) countEl.textContent = String(total);
    // Visible summary pills.
    var rEl = document.querySelector('[data-summary-ratings]');
    var vEl = document.querySelector('[data-summary-votes]');
    var aEl = document.querySelector('[data-summary-applied]');
    var emptyEl = document.querySelector('[data-summary-empty]');
    if (rEl) {
      rEl.textContent = 'Ratings ' + summary.ratings.length;
      rEl.setAttribute('data-zero', summary.ratings.length === 0 ? 'true' : 'false');
    }
    if (vEl) {
      vEl.textContent = 'Votes ' + summary.votes.length;
      vEl.setAttribute('data-zero', summary.votes.length === 0 ? 'true' : 'false');
    }
    if (aEl) {
      aEl.textContent = 'Applied ' + summary.applied.length;
      aEl.setAttribute('data-zero', summary.applied.length === 0 ? 'true' : 'false');
    }
    if (emptyEl) emptyEl.hidden = total > 0;
    // Textarea — unconditional set so a server-rendered default
    // can't ever stay visible.
    var out = document.querySelector('[data-decisions-output]');
    if (out) out.value = buildDecisionsMarkdown(summary);
  }

  registerInitBlock('decisions-panel', function () {
    var panel = document.querySelector('[data-decisions]');
    if (!panel) throw new Error('[data-decisions] panel markup not found');
    var head = panel.querySelector('[data-decisions-toggle]');
    if (head) head.addEventListener('click', function () {
      panel.setAttribute('data-collapsed', panel.getAttribute('data-collapsed') === 'true' ? 'false' : 'true');
    });
    var copyBtn = panel.querySelector('[data-decisions-copy]');
    if (copyBtn) copyBtn.addEventListener('click', function () {
      var out = panel.querySelector('[data-decisions-output]');
      if (!out) return;
      var done = function () {
        var t = copyBtn.textContent;
        copyBtn.textContent = 'Copied ✓';
        setTimeout(function () { copyBtn.textContent = t; }, 1500);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(out.value).then(done, function () {
          try { out.select(); document.execCommand && document.execCommand('copy'); } catch (e) {}
          done();
        });
      } else {
        try { out.removeAttribute('readonly'); out.select(); document.execCommand('copy'); out.setAttribute('readonly', ''); } catch (e) {}
        done();
      }
      logAction('Copy decisions markdown');
    });
    var dlBtn = panel.querySelector('[data-decisions-download]');
    if (dlBtn) dlBtn.addEventListener('click', function () {
      var out = panel.querySelector('[data-decisions-output]');
      if (!out) return;
      function dl(name, content, type) {
        var blob = new Blob([content], { type: type });
        var url = URL.createObjectURL(blob);
        var a = document.createElement('a');
        a.href = url; a.download = name;
        document.body.appendChild(a); a.click(); document.body.removeChild(a);
        setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
      }
      dl('chronicle-canon-decisions.md', out.value, 'text/markdown');
      dl('chronicle-canon-decisions.json', JSON.stringify(decisions, null, 2), 'application/json');
      logAction('Download decisions');
    });
    var resetBtn = panel.querySelector('[data-decisions-reset]');
    if (resetBtn) resetBtn.addEventListener('click', function () {
      if (!confirm('Clear all your decisions?')) return;
      decisions = {};
      persistDecisions();
      location.reload();
    });
    renderDecisionsPanel();
  });

  // ==========================================================
  // Block 10 — preview drawer toggle (small live-preview surface).
  // ==========================================================
  registerInitBlock('preview-drawer', function () {
    var open = document.querySelector('[data-preview-drawer-toggle]');
    var close = document.querySelector('[data-preview-drawer-close]');
    var drawer = document.querySelector('[data-preview-drawer]');
    if (!drawer) return; // preview section can be absent on render error; non-fatal
    function set(state) {
      drawer.setAttribute('data-open', state ? 'true' : 'false');
      if (state) drawer.scrollTop = 0;
    }
    if (open) open.addEventListener('click', function () {
      set(drawer.getAttribute('data-open') !== 'true');
      logAction('Preview drawer toggled');
    });
    if (close) close.addEventListener('click', function () { set(false); });
    document.addEventListener('keydown', function (ev) {
      if (ev.key === 'Escape' && drawer.getAttribute('data-open') === 'true') set(false);
    });
    var n = 0;
    if (open) n++;
    if (close) n++;
    recordBinding('preview-drawer controls', n);
  });

  // ==========================================================
  // Block 11 — Copy diagnostic report button.
  // ==========================================================
  registerInitBlock('diagnostics-copy-report', function () {
    var btn = document.querySelector('[data-copy-report]');
    if (!btn) throw new Error('[data-copy-report] button missing');
    btn.addEventListener('click', function () {
      var lines = [];
      lines.push('# Chronicle Canon Demo — Diagnostic Report');
      lines.push('Generated: ' + new Date().toISOString());
      lines.push('User Agent: ' + diagnostics.ua);
      lines.push('');
      lines.push('## Init blocks');
      diagnostics.blocks.forEach(function (b) {
        lines.push('- ' + b.name + ' — ' + b.status + (b.error ? ' (' + b.error + ')' : ''));
      });
      lines.push('');
      lines.push('## Element bindings');
      diagnostics.bindings.forEach(function (b) { lines.push('- ' + b.name + ': ' + b.count); });
      lines.push('');
      lines.push('## Browser features');
      diagnostics.features.forEach(function (f) {
        lines.push('- ' + f.name + ': ' + (f.supported ? 'Supported' : 'Missing'));
      });
      lines.push('');
      lines.push('## Last actions');
      if (!diagnostics.actions.length) lines.push('- (none yet)');
      else diagnostics.actions.forEach(function (a) { lines.push('- ' + a); });
      var report = lines.join('\n');
      var done = function () {
        var t = btn.textContent;
        btn.textContent = 'Copied ✓';
        setTimeout(function () { btn.textContent = t; }, 1500);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(report).then(done, function () {
          alert(report); done();
        });
      } else {
        try {
          var ta = document.createElement('textarea');
          ta.value = report;
          document.body.appendChild(ta);
          ta.select();
          document.execCommand('copy');
          document.body.removeChild(ta);
        } catch (e) {}
        done();
      }
      logAction('Copy diagnostic report');
    });
  });

  // ==========================================================
  // Dashboard fill (runs once after all blocks, in init()).
  // ==========================================================
  function renderDashboard() {
    var panel = document.querySelector('[data-demo-diagnostics]');
    if (!panel) return;
    var blockList = panel.querySelector('[data-init-block-list]');
    if (blockList) {
      blockList.innerHTML = '';
      diagnostics.blocks.forEach(function (b) {
        var li = document.createElement('li');
        li.className = 'chronicle-diagnostics__item chronicle-diagnostics__item--' + (b.status === 'OK' ? 'ok' : 'fail');
        var status = document.createElement('span');
        status.className = 'chronicle-diagnostics__status';
        status.textContent = b.status === 'OK' ? '✓' : '✗';
        var name = document.createElement('span');
        name.className = 'chronicle-diagnostics__name';
        name.textContent = b.name;
        li.appendChild(status); li.appendChild(name);
        if (b.status !== 'OK' && b.error) {
          var err = document.createElement('code');
          err.className = 'chronicle-diagnostics__error';
          err.textContent = b.error;
          li.appendChild(err);
        }
        blockList.appendChild(li);
      });
    }
    var bindingList = panel.querySelector('[data-binding-list]');
    if (bindingList) {
      bindingList.innerHTML = '';
      if (!diagnostics.bindings.length) {
        var li = document.createElement('li');
        li.className = 'chronicle-diagnostics__item chronicle-diagnostics__item--fail';
        li.textContent = '(no bindings recorded — init likely failed before any block ran)';
        bindingList.appendChild(li);
      } else {
        diagnostics.bindings.forEach(function (b) {
          var li = document.createElement('li');
          li.className = 'chronicle-diagnostics__item chronicle-diagnostics__item--' + (b.count > 0 ? 'ok' : 'warn');
          li.innerHTML = '<span class="chronicle-diagnostics__name">' + escapeHTML(b.name) + '</span><span class="chronicle-diagnostics__count">' + b.count + '</span>';
          bindingList.appendChild(li);
        });
      }
    }
    var featureList = panel.querySelector('[data-feature-list]');
    if (featureList) {
      featureList.innerHTML = '';
      diagnostics.features.forEach(function (f) {
        var li = document.createElement('li');
        li.className = 'chronicle-diagnostics__item chronicle-diagnostics__item--' + (f.supported ? 'ok' : 'warn');
        li.innerHTML = '<span class="chronicle-diagnostics__status">' + (f.supported ? '✓' : '–') + '</span><span class="chronicle-diagnostics__name">' + escapeHTML(f.name) + '</span>';
        featureList.appendChild(li);
      });
    }
    renderLastActionLog();
    // Top-level pass/fail banner.
    var failed = diagnostics.blocks.filter(function (b) { return b.status !== 'OK'; }).length;
    panel.setAttribute('data-status', failed > 0 ? 'fail' : 'ok');
    var summary = panel.querySelector('[data-diagnostics-summary]');
    if (summary) {
      summary.textContent = failed > 0
        ? failed + ' of ' + diagnostics.blocks.length + ' init blocks FAILED'
        : 'All ' + diagnostics.blocks.length + ' init blocks OK';
    }
  }
  function renderLastActionLog() {
    var log = document.querySelector('[data-last-action-log]');
    if (!log) return;
    log.innerHTML = '';
    if (!diagnostics.actions.length) {
      var li = document.createElement('li');
      li.textContent = '(no actions yet — click something above)';
      log.appendChild(li);
      return;
    }
    diagnostics.actions.forEach(function (a) {
      var li = document.createElement('li');
      li.textContent = a;
      log.appendChild(li);
    });
  }
  function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  // ==========================================================
  // Trigger init. `defer` guarantees DOM is parsed; we still
  // belt-and-suspenders against document.readyState.
  // ==========================================================
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
