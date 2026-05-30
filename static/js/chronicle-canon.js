// chronicle-canon.js — C-V2-DESIGN-REBUILD Phase 2A tabbed canon demo.
//
// Phase 2A.1 motion-diagnosis update:
//   - NEW css-reality-probe init block: reports OS reduced-motion,
//     canon-stylesheet-loaded, computed transitionDuration on an ACTUAL
//     demo button (not document.querySelector('button'), which grabs a
//     site-chrome button), canon token resolution, and scope-root
//     presence. Closes the CSS-reality verification gap on-page.
//   - NEW motion-banner block: ONLY un-hides the consensual
//     motion-preview banner when prefers-reduced-motion:reduce is
//     active; opt-in sets [data-canon-force-motion="true"] on the root.
//   - "Your picks" summary panel removed; picks still persist + export
//     via the single header "Copy my picks" button (build-on-demand).
//
// Architecture carryover (from PR #379): INIT_BLOCKS registry with
// per-block try/catch; __chronicleCanonInited set AFTER success;
// externalized JS via <script src=… defer>; document.title fallback.
//
// localStorage key: chronicle-canon-picks-v2 (no migration from the
// prior scrapped store).

(function () {
  'use strict';

  // ============================================================
  // Registry + diagnostics surface.
  // ============================================================
  var INIT_BLOCKS = [];
  function registerInitBlock(name, runner) {
    INIT_BLOCKS.push({ name: name, runner: runner });
  }

  var diagnostics = {
    blocks: [],     // [{name, status, error}]
    bindings: [],   // [{name, count}]
    features: [],   // [{name, supported}]
    css: [],        // [{name, value, ok}] — CSS/motion reality rows
    actions: [],    // last-action log (most recent first; max 12)
    ua: '',
  };

  function recordBinding(name, count) { diagnostics.bindings.push({ name: name, count: count }); }
  function recordFeature(name, supported) { diagnostics.features.push({ name: name, supported: !!supported }); }
  function recordCSS(name, value, ok) { diagnostics.css.push({ name: name, value: value, ok: !!ok }); }
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
        try { console.error('[chronicle-canon] Init block failed:', b.name, err); } catch (e) {}
      }
    }
  }

  function init() {
    if (window.__chronicleCanonInited) return;
    runAllInitBlocks();
    try { renderDashboard(); } catch (e) {
      try { document.title = '[canon dashboard render failed] ' + document.title; } catch (e2) {}
    }
    window.__chronicleCanonInited = true;
    window.__chronicleCanonInitResults = diagnostics.blocks.slice();
    window.__chronicleCanonDiagnostics = diagnostics;
  }

  // ============================================================
  // Block 1 — diagnostic dashboard skeleton + UA capture.
  // ============================================================
  registerInitBlock('diagnostic-dashboard', function () {
    try {
      diagnostics.ua = (navigator && navigator.userAgent) || 'unknown';
      var panel = document.querySelector('[data-canon-diagnostics]');
      if (!panel) {
        try { document.title = '[canon: diagnostics panel missing] ' + document.title; } catch (e) {}
        throw new Error('[data-canon-diagnostics] markup not found');
      }
      var uaEl = panel.querySelector('[data-ua-string]');
      if (uaEl) uaEl.textContent = diagnostics.ua;
    } catch (err) {
      try { document.title = '[canon dashboard skeleton fail] ' + document.title; } catch (e) {}
      throw err;
    }
  });

  // ============================================================
  // Block 2 — browser-compat detection.
  // ============================================================
  registerInitBlock('browser-compat-detect', function () {
    function supports(prop, val) {
      try {
        if (val === undefined) return typeof CSS !== 'undefined' && CSS.supports(prop);
        return typeof CSS !== 'undefined' && CSS.supports(prop, val);
      } catch (e) { return false; }
    }
    recordFeature('oklch',              supports('color', 'oklch(0.5 0.1 180)'));
    recordFeature('color-mix(oklch)',   supports('color', 'color-mix(in oklch, red, blue)'));
    recordFeature(':has() selector',    supports('selector(:has(*))'));
    recordFeature('clipboard.writeText', !!(navigator.clipboard && navigator.clipboard.writeText));
    recordFeature('localStorage', (function () {
      try { localStorage.setItem('__t', '1'); localStorage.removeItem('__t'); return true; }
      catch (e) { return false; }
    })());
  });

  // ============================================================
  // Block 3 — theme toggle. localStorage > prefers-color-scheme > dark.
  // ============================================================
  registerInitBlock('theme-toggle', function () {
    var root = document.querySelector('[data-chronicle-canon]');
    if (!root) throw new Error('canon root not found');
    function applyTheme(theme) { root.dataset.chronicleCanonTheme = theme; }
    var theme = 'dark';
    try {
      var stored = localStorage.getItem('chronicle-canon-theme');
      if (stored === 'light' || stored === 'dark') theme = stored;
      else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) theme = 'light';
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
        try { localStorage.setItem('chronicle-canon-theme', v); } catch (e) {}
        logAction('Theme → ' + v);
      });
    });
    recordBinding('theme buttons', btns.length);
  });

  // ============================================================
  // Block 4 — reduced-motion toggle (demo-scoped attribute).
  // ============================================================
  registerInitBlock('reduced-motion-toggle', function () {
    var root = document.querySelector('[data-chronicle-canon]');
    if (!root) throw new Error('canon root not found');
    var btns = document.querySelectorAll('[data-action="reduce-motion"]');
    btns.forEach(function (b) {
      b.addEventListener('click', function () {
        var v = b.dataset.value;
        root.dataset.chronicleCanonReduceMotion = v;
        btns.forEach(function (o) { o.setAttribute('aria-pressed', o.dataset.value === v ? 'true' : 'false'); });
        logAction('Reduced motion → ' + v);
      });
    });
    recordBinding('reduced-motion buttons', btns.length);
  });

  // ============================================================
  // Block 5 — tab strip.
  // ============================================================
  registerInitBlock('tab-strip', function () {
    var root = document.querySelector('[data-chronicle-canon]');
    if (!root) throw new Error('canon root not found');
    var tabs = document.querySelectorAll('[data-tab]');
    var enabled = 0, disabled = 0;
    tabs.forEach(function (t) {
      var id = t.getAttribute('data-tab');
      if (t.hasAttribute('data-tab-disabled')) {
        disabled++;
        t.addEventListener('click', function () { logAction('Tab (disabled) ' + id + ' — ships later'); });
        return;
      }
      enabled++;
      t.addEventListener('click', function () {
        root.dataset.activeTab = id;
        tabs.forEach(function (o) { o.setAttribute('aria-selected', o === t ? 'true' : 'false'); });
        try { history.replaceState(null, '', '#' + id); } catch (e) {}
        logAction('Tab → ' + id);
      });
    });
    if (location.hash) {
      var hashTab = location.hash.replace(/^#/, '');
      var match = document.querySelector('[data-tab="' + hashTab + '"]:not([data-tab-disabled])');
      if (match) match.click();
    }
    recordBinding('tabs (active)', enabled);
    recordBinding('tabs (disabled)', disabled);
  });

  // ============================================================
  // Picks store — single source of truth. localStorage key:
  // chronicle-canon-picks-v2. Shape: { <tab>: { <axis>: value|[values],
  // __notes: string } }.
  // ============================================================
  var PICKS_KEY = 'chronicle-canon-picks-v2';
  var MULTI_SELECT_AXES = { size: true };
  var picks = {};

  function loadPicks() {
    try {
      var raw = localStorage.getItem(PICKS_KEY);
      if (raw) picks = JSON.parse(raw) || {};
    } catch (e) { picks = {}; }
    if (!picks || typeof picks !== 'object') picks = {};
  }
  function savePicks() {
    try { localStorage.setItem(PICKS_KEY, JSON.stringify(picks)); } catch (e) {}
  }
  function currentTab() {
    var root = document.querySelector('[data-chronicle-canon]');
    return (root && root.dataset.activeTab) || 'buttons';
  }
  function tabPicks(tab) {
    if (!picks[tab]) picks[tab] = {};
    return picks[tab];
  }

  // ============================================================
  // Block 6 — pick handlers. Click/Enter/Space toggles a pick card.
  // Multi-select on size axis; single-select elsewhere.
  // ============================================================
  registerInitBlock('picks-hydrate-and-bind', function () {
    loadPicks();
    var cards = document.querySelectorAll('[data-pick]');
    cards.forEach(function (card) {
      var axis = card.getAttribute('data-pick-axis');
      var value = card.getAttribute('data-pick-value');
      var tab = currentTab();
      var stored = tabPicks(tab)[axis];
      var isPicked = MULTI_SELECT_AXES[axis]
        ? Array.isArray(stored) && stored.indexOf(value) !== -1
        : stored === value;
      if (isPicked) {
        card.setAttribute('data-picked', 'true');
        card.setAttribute('aria-pressed', 'true');
      }
      function toggle() {
        var bucket = tabPicks(currentTab());
        if (MULTI_SELECT_AXES[axis]) {
          var list = Array.isArray(bucket[axis]) ? bucket[axis].slice() : [];
          var idx = list.indexOf(value);
          if (idx === -1) list.push(value); else list.splice(idx, 1);
          bucket[axis] = list;
          card.setAttribute('data-picked', idx === -1 ? 'true' : 'false');
          card.setAttribute('aria-pressed', idx === -1 ? 'true' : 'false');
        } else {
          var siblings = document.querySelectorAll('[data-pick-axis="' + axis + '"]');
          siblings.forEach(function (s) {
            s.setAttribute('data-picked', 'false');
            s.setAttribute('aria-pressed', 'false');
          });
          if (bucket[axis] === value) {
            delete bucket[axis];
          } else {
            bucket[axis] = value;
            card.setAttribute('data-picked', 'true');
            card.setAttribute('aria-pressed', 'true');
          }
        }
        savePicks();
        logAction('Pick ' + axis + ' → ' + value);
      }
      card.addEventListener('click', function () { toggle(); });
      card.addEventListener('keydown', function (ev) {
        if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); toggle(); }
      });
    });
    recordBinding('pick cards', cards.length);
  });

  // ============================================================
  // Block 7 — per-tab notes textarea.
  // ============================================================
  registerInitBlock('notes-bind', function () {
    var nodes = document.querySelectorAll('[data-notes]');
    nodes.forEach(function (n) {
      var tab = n.getAttribute('data-notes-tab') || currentTab();
      var bucket = tabPicks(tab);
      if (bucket && bucket.__notes) n.value = bucket.__notes;
      n.addEventListener('input', function () {
        tabPicks(tab).__notes = n.value;
        savePicks();
      });
    });
    recordBinding('notes textareas', nodes.length);
  });

  // ============================================================
  // Block 8 — CSS / motion reality probe (Phase 2A.1 §A).
  //
  // The verification gap that hid the dead-motion bug across phases was
  // that nothing checked CSS reality in the actual browser. These rows
  // compute, at runtime: OS reduce-motion, canon stylesheet loaded,
  // computed transitionDuration on an ACTUAL demo button (the bug-prone
  // one — NOT document.querySelector('button')), canon token
  // resolution, and scope-root presence.
  // ============================================================
  registerInitBlock('css-reality-probe', function () {
    var rm = false;
    try { rm = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches; } catch (e) {}
    recordCSS('OS reduce-motion', rm ? 'ACTIVE ⚠' : 'inactive', !rm);

    var loaded = false;
    try {
      loaded = [].slice.call(document.styleSheets).some(function (s) {
        return s.href && s.href.indexOf('chronicle-canon.css') !== -1;
      });
    } catch (e) {}
    recordCSS('canon stylesheet', loaded ? 'loaded' : 'NOT FOUND', loaded);

    var btn = document.querySelector('.chronicle-canon-btn');
    if (btn) {
      var cs = getComputedStyle(btn);
      var dur = cs.transitionDuration || '(none)';
      var ok = dur && dur !== '0s' && dur !== '0ms';
      recordCSS('demo button transition', dur + ' / ' + (cs.transitionProperty || '?'), ok);
    } else {
      recordCSS('demo button transition', 'no .chronicle-canon-btn found', false);
    }

    var root = document.querySelector('[data-chronicle-canon]');
    if (root) {
      var accent = getComputedStyle(root).getPropertyValue('--chronicle-accent').trim();
      recordCSS('--chronicle-accent', accent || '(unresolved)', !!accent);
    } else {
      recordCSS('--chronicle-accent', '(no scope root)', false);
    }

    recordCSS('scope root [data-chronicle-canon]', root ? 'found' : 'MISSING', !!root);
  });

  // ============================================================
  // Block 9 — consensual motion-preview banner (Phase 2A.1 §B).
  // ONLY un-hides the banner when OS reduce-motion is active. Opt-in
  // sets [data-canon-force-motion="true"] on the root (CSS beats the
  // global guard within the demo only). Choice persists.
  // ============================================================
  registerInitBlock('motion-banner', function () {
    var root = document.querySelector('[data-chronicle-canon]');
    var banner = document.querySelector('[data-motion-banner]');
    if (!root || !banner) return; // banner absent is non-fatal
    var rm = false;
    try { rm = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches; } catch (e) {}

    var forced = false;
    try { forced = localStorage.getItem('chronicle-canon-force-motion') === 'true'; } catch (e) {}
    if (forced) root.setAttribute('data-canon-force-motion', 'true');

    var btn = banner.querySelector('[data-motion-toggle]');
    function paint() {
      var on = root.getAttribute('data-canon-force-motion') === 'true';
      if (btn) btn.textContent = on ? 'Disable motion preview' : 'Enable motion preview';
    }
    if (btn) btn.addEventListener('click', function () {
      var on = root.getAttribute('data-canon-force-motion') === 'true';
      if (on) root.removeAttribute('data-canon-force-motion');
      else root.setAttribute('data-canon-force-motion', 'true');
      try { localStorage.setItem('chronicle-canon-force-motion', on ? 'false' : 'true'); } catch (e) {}
      paint();
      logAction('Motion preview → ' + (on ? 'off' : 'on'));
    });
    paint();

    if (rm) banner.removeAttribute('hidden');
    recordBinding('motion-banner (shown)', rm ? 1 : 0);
  });

  // ============================================================
  // Picks export — build the markdown brief on demand (no panel DOM).
  // ============================================================
  function pickLabel(axis, value) {
    if (axis === 'variant' && value && value.indexOf(':') !== -1) {
      var parts = value.split(':');
      return parts[0] + ' (' + parts[1] + ')';
    }
    return value;
  }
  function summarize() {
    var tab = currentTab();
    var bucket = tabPicks(tab);
    var rows = [];
    Object.keys(bucket).forEach(function (axis) {
      if (axis === '__notes') return;
      var v = bucket[axis];
      if (Array.isArray(v)) {
        if (!v.length) return;
        rows.push({ axis: axis, value: v.map(function (x) { return pickLabel(axis, x); }).join(', ') });
      } else if (v) {
        rows.push({ axis: axis, value: pickLabel(axis, v) });
      }
    });
    return { tab: tab, rows: rows, notes: bucket.__notes || '' };
  }
  function buildBrief(s) {
    var lines = [];
    lines.push('# Chronicle canon — ' + s.tab + ' picks');
    lines.push('_' + (navigator.userAgent || '') + ' · ' + new Date().toISOString().slice(0, 10) + '_');
    lines.push('');
    if (!s.rows.length && !s.notes.trim()) {
      lines.push('_No picks yet._');
      return lines.join('\n');
    }
    if (s.rows.length) {
      lines.push('## Picks');
      s.rows.forEach(function (r) { lines.push('- **' + r.axis + ':** ' + r.value); });
      lines.push('');
    }
    if (s.notes.trim()) {
      lines.push('## Notes');
      lines.push(s.notes.trim());
      lines.push('');
    }
    return lines.join('\n');
  }
  function fallbackCopy(text) {
    try {
      var ta = document.createElement('textarea');
      ta.value = text;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
    } catch (e) {}
  }

  // ============================================================
  // Block 10 — picks copy (replaces the removed picks-panel).
  // Single "Copy my picks" button serializes the brief on demand.
  // ============================================================
  registerInitBlock('picks-copy', function () {
    var copyBtn = document.querySelector('[data-picks-copy]');
    if (!copyBtn) throw new Error('[data-picks-copy] button missing');
    copyBtn.addEventListener('click', function () {
      var brief = buildBrief(summarize());
      var done = function () {
        var t = copyBtn.textContent;
        copyBtn.textContent = 'Copied ✓';
        setTimeout(function () { copyBtn.textContent = t; }, 1500);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(brief).then(done, function () { fallbackCopy(brief); done(); });
      } else {
        fallbackCopy(brief); done();
      }
      logAction('Copy picks');
    });
    recordBinding('picks-copy button', 1);
  });

  // ============================================================
  // Block 11 — Copy diagnostic report button.
  // ============================================================
  registerInitBlock('diagnostics-copy-report', function () {
    var btn = document.querySelector('[data-copy-report]');
    if (!btn) throw new Error('[data-copy-report] missing');
    btn.addEventListener('click', function () {
      var lines = [];
      lines.push('# Chronicle canon demo — Diagnostic Report');
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
      lines.push('## CSS / motion reality');
      diagnostics.css.forEach(function (c) { lines.push('- ' + c.name + ': ' + c.value + (c.ok ? '' : '  ⚠')); });
      lines.push('');
      lines.push('## Browser features');
      diagnostics.features.forEach(function (f) { lines.push('- ' + f.name + ': ' + (f.supported ? 'Supported' : 'Missing')); });
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
        navigator.clipboard.writeText(report).then(done, function () { fallbackCopy(report); done(); });
      } else { fallbackCopy(report); done(); }
      logAction('Copy diagnostic report');
    });
  });

  // ============================================================
  // Dashboard fill. Auto-expands the <details> if any block FAILED or
  // any CSS-reality row warns.
  // ============================================================
  function listInto(selector, items, classify, text) {
    var list = document.querySelector(selector);
    if (!list) return;
    list.innerHTML = '';
    if (!items.length) {
      var li = document.createElement('li');
      li.className = 'chronicle-canon__diagnostics-item chronicle-canon__diagnostics-item--warn';
      li.textContent = '(none)';
      list.appendChild(li);
      return;
    }
    items.forEach(function (it) {
      var li = document.createElement('li');
      li.className = 'chronicle-canon__diagnostics-item chronicle-canon__diagnostics-item--' + classify(it);
      li.textContent = text(it);
      list.appendChild(li);
    });
  }

  function renderDashboard() {
    var panel = document.querySelector('[data-canon-diagnostics]');
    if (!panel) return;
    var failed = diagnostics.blocks.filter(function (b) { return b.status !== 'OK'; }).length;
    var cssFail = diagnostics.css.filter(function (c) { return !c.ok; }).length;
    panel.setAttribute('data-status', (failed > 0) ? 'fail' : 'ok');
    if (failed > 0 || cssFail > 0) panel.setAttribute('open', '');
    var summary = panel.querySelector('[data-diagnostics-status]');
    if (summary) {
      summary.textContent = failed > 0
        ? failed + ' of ' + diagnostics.blocks.length + ' init blocks FAILED'
        : (cssFail > 0
            ? 'init OK; ' + cssFail + ' CSS-reality warning(s)'
            : 'All ' + diagnostics.blocks.length + ' init blocks OK');
    }
    listInto('[data-init-block-list]', diagnostics.blocks,
      function (b) { return b.status === 'OK' ? 'ok' : 'fail'; },
      function (b) { return (b.status === 'OK' ? '✓ ' : '✗ ') + b.name + (b.error ? ' — ' + b.error : ''); });
    listInto('[data-binding-list]', diagnostics.bindings,
      function (b) { return b.count > 0 ? 'ok' : 'warn'; },
      function (b) { return b.name + ': ' + b.count; });
    listInto('[data-feature-list]', diagnostics.features,
      function (f) { return f.supported ? 'ok' : 'warn'; },
      function (f) { return (f.supported ? '✓ ' : '– ') + f.name; });
    var cssList = document.querySelector('[data-css-reality-list]');
    if (cssList) {
      cssList.innerHTML = '';
      diagnostics.css.forEach(function (c) {
        var li = document.createElement('li');
        li.className = 'chronicle-canon__diagnostics-item chronicle-canon__diagnostics-item--probe chronicle-canon__diagnostics-item--' + (c.ok ? 'ok' : 'warn');
        var name = document.createElement('span');
        name.textContent = (c.ok ? '✓ ' : '⚠ ') + c.name;
        var val = document.createElement('span');
        val.className = 'chronicle-canon__probe-value';
        val.textContent = c.value;
        li.appendChild(name);
        li.appendChild(val);
        cssList.appendChild(li);
      });
    }
    renderLastActionLog();
  }
  function renderLastActionLog() {
    var log = document.querySelector('[data-last-action-log]');
    if (!log) return;
    log.innerHTML = '';
    if (!diagnostics.actions.length) {
      var li = document.createElement('li');
      li.textContent = '(no actions yet)';
      log.appendChild(li);
      return;
    }
    diagnostics.actions.forEach(function (a) {
      var li = document.createElement('li');
      li.textContent = a;
      log.appendChild(li);
    });
  }

  // ============================================================
  // Trigger init.
  // ============================================================
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
