// chronicle-canon.js — C-V2-DESIGN-REBUILD Phase 2A tabbed canon demo.
//
// Fresh implementation after the PR #375-#381 single-page arc was
// scrapped. Architectural patterns carry forward conceptually:
//
//   - INIT_BLOCKS registry, per-block try/catch
//   - __chronicleCanonInited flag set AFTER successful run
//   - Visible diagnostic dashboard (collapsed by default this time;
//     auto-expands if any block FAILED)
//   - document.title fallback if even the dashboard render fails
//
// Surface this phase: theme + reduced-motion toggles; tab strip (only
// Buttons clickable); the Buttons tab's 6 pickable axes (variant,
// size, shadow, hover, motion, selected) + 1 reference section
// (states); per-tab notes textarea; "Your picks" summary + Copy
// markdown brief + Download + Reset.
//
// localStorage key: chronicle-canon-picks-v2 (fresh; no migration
// from the prior scrapped store).

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
    actions: [],    // last-action log (most recent first; max 12)
    ua: '',
  };

  function recordBinding(name, count) { diagnostics.bindings.push({ name: name, count: count }); }
  function recordFeature(name, supported) { diagnostics.features.push({ name: name, supported: !!supported }); }
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
  // Block 1 — diagnostic dashboard skeleton + UA capture + title
  // fallback if even this block fails.
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
    recordFeature('container-queries',  supports('container-type', 'inline-size'));
    recordFeature(':has() selector',    supports('selector(:has(*))'));
    recordFeature('clipboard.writeText', !!(navigator.clipboard && navigator.clipboard.writeText));
    recordFeature('localStorage', (function () {
      try { localStorage.setItem('__t', '1'); localStorage.removeItem('__t'); return true; }
      catch (e) { return false; }
    })());
    recordFeature('URLSearchParams',    typeof URLSearchParams === 'function');
  });

  // ============================================================
  // Block 3 — theme toggle. Hydrates from localStorage > prefers-
  // color-scheme > dark default. Persists explicit toggles.
  // ============================================================
  registerInitBlock('theme-toggle', function () {
    var root = document.querySelector('[data-chronicle-canon]');
    if (!root) throw new Error('canon root not found');

    function applyTheme(theme) {
      root.dataset.chronicleCanonTheme = theme;
    }

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
  // Block 4 — reduced-motion toggle.
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
  //
  // Buttons is the only active tab this phase. Disabled tabs surface
  // their "Coming next phase" tooltip; clicking them does nothing
  // navigational but logs the click so operator sees it registered.
  // URL hash reflects the active tab so refreshing keeps state.
  // ============================================================
  registerInitBlock('tab-strip', function () {
    var root = document.querySelector('[data-chronicle-canon]');
    if (!root) throw new Error('canon root not found');
    var tabs = document.querySelectorAll('[data-tab]');
    var enabled = 0;
    var disabled = 0;
    tabs.forEach(function (t) {
      var id = t.getAttribute('data-tab');
      var isDisabled = t.hasAttribute('data-tab-disabled');
      if (isDisabled) {
        disabled++;
        t.addEventListener('click', function () {
          logAction('Tab (disabled) ' + id + ' — ships later');
        });
        return;
      }
      enabled++;
      t.addEventListener('click', function () {
        root.dataset.activeTab = id;
        tabs.forEach(function (o) {
          o.setAttribute('aria-selected', o === t ? 'true' : 'false');
        });
        try { history.replaceState(null, '', '#' + id); } catch (e) {}
        logAction('Tab → ' + id);
      });
    });
    // Honour an existing hash on load (only matters if it's an enabled tab).
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
  // chronicle-canon-picks-v2 (fresh; no migration from prior demo).
  //
  // Shape: { <tab>: { <axis>: <value> | [<value>, ...] } }
  //
  // - <tab> = "buttons" / "menus" / ...
  // - <axis> = "variant" / "size" / "shadow" / "hover" / "motion" /
  //   "selected" (axes are per-tab; the variant axis carries a
  //   "<family>:<sub>" value such as "primary:A").
  // - Single-select axes store a string; multi-select axes store an
  //   array. Whether an axis is multi-select is dispatched on the
  //   axis name (sizes are multi-select; everything else is single).
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
  // Block 6 — pick handlers.
  //
  // Click (or Enter/Space on focus) on a [data-pick] card toggles
  // its picked state. Multi-select axes accumulate; single-select
  // axes clear sibling picks within the same axis on the same tab.
  // ============================================================
  registerInitBlock('picks-hydrate-and-bind', function () {
    loadPicks();
    var cards = document.querySelectorAll('[data-pick]');
    cards.forEach(function (card) {
      var axis = card.getAttribute('data-pick-axis');
      var value = card.getAttribute('data-pick-value');
      // Hydrate visual state from store.
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
        var tabNow = currentTab();
        var bucket = tabPicks(tabNow);
        if (MULTI_SELECT_AXES[axis]) {
          var list = Array.isArray(bucket[axis]) ? bucket[axis].slice() : [];
          var idx = list.indexOf(value);
          if (idx === -1) { list.push(value); }
          else { list.splice(idx, 1); }
          bucket[axis] = list;
          card.setAttribute('data-picked', idx === -1 ? 'true' : 'false');
          card.setAttribute('aria-pressed', idx === -1 ? 'true' : 'false');
        } else {
          // Single-select: clear other siblings within this axis.
          var siblings = document.querySelectorAll('[data-pick-axis="' + axis + '"]');
          siblings.forEach(function (s) {
            s.setAttribute('data-picked', 'false');
            s.setAttribute('aria-pressed', 'false');
          });
          if (bucket[axis] === value) {
            delete bucket[axis]; // clicking the picked one un-picks
          } else {
            bucket[axis] = value;
            card.setAttribute('data-picked', 'true');
            card.setAttribute('aria-pressed', 'true');
          }
        }
        savePicks();
        renderPicksPanel();
        logAction('Pick ' + axis + ' → ' + value);
      }
      card.addEventListener('click', function (ev) {
        // Don't toggle if the click landed on a button inside the
        // sample body (those buttons are previews; their state is
        // illustrative, but the toggle should still fire on the
        // surrounding card). So actually — toggle is on the whole
        // card; sample buttons forward the click up naturally.
        toggle();
      });
      card.addEventListener('keydown', function (ev) {
        if (ev.key === 'Enter' || ev.key === ' ') {
          ev.preventDefault();
          toggle();
        }
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
      // Hydrate from store.
      var bucket = tabPicks(tab);
      if (bucket && bucket.__notes) n.value = bucket.__notes;
      n.addEventListener('input', function () {
        var b = tabPicks(tab);
        b.__notes = n.value;
        savePicks();
        renderPicksPanel();
      });
    });
    recordBinding('notes textareas', nodes.length);
  });

  // ============================================================
  // "Your picks" summary + Copy/Download/Reset.
  // ============================================================
  function pickLabel(axis, value) {
    // For variant axis the value is "<family>:<sub>" — render both
    // parts so operator sees "primary (A)" rather than "primary:A".
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
      s.rows.forEach(function (r) {
        lines.push('- **' + r.axis + ':** ' + r.value);
      });
      lines.push('');
    }
    if (s.notes.trim()) {
      lines.push('## Notes');
      lines.push(s.notes.trim());
      lines.push('');
    }
    return lines.join('\n');
  }
  function renderPicksPanel() {
    var s = summarize();
    var countEl = document.querySelector('[data-picks-count]');
    var nameEl = document.querySelector('[data-picks-tabname]');
    var listEl = document.querySelector('[data-picks-list]');
    var emptyEl = document.querySelector('[data-picks-empty]');
    var briefEl = document.querySelector('[data-picks-brief]');
    if (countEl) countEl.textContent = String(s.rows.length);
    if (nameEl) nameEl.textContent = s.tab.charAt(0).toUpperCase() + s.tab.slice(1);
    if (listEl) {
      listEl.innerHTML = '';
      s.rows.forEach(function (r) {
        var li = document.createElement('li');
        var k = document.createElement('strong');
        k.textContent = r.axis;
        var v = document.createElement('span');
        v.textContent = r.value;
        li.appendChild(k);
        li.appendChild(v);
        listEl.appendChild(li);
      });
    }
    if (emptyEl) emptyEl.hidden = s.rows.length > 0 || (s.notes && s.notes.trim().length > 0);
    if (briefEl) briefEl.value = buildBrief(s);
  }

  registerInitBlock('picks-panel', function () {
    var copyBtn = document.querySelector('[data-picks-copy]');
    var dlBtn = document.querySelector('[data-picks-download]');
    var resetBtn = document.querySelector('[data-picks-reset]');
    if (copyBtn) copyBtn.addEventListener('click', function () {
      var brief = document.querySelector('[data-picks-brief]');
      if (!brief) return;
      var done = function () {
        var t = copyBtn.textContent;
        copyBtn.textContent = 'Copied ✓';
        setTimeout(function () { copyBtn.textContent = t; }, 1500);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(brief.value).then(done, function () {
          try { brief.select(); document.execCommand && document.execCommand('copy'); } catch (e) {}
          done();
        });
      } else {
        try { brief.removeAttribute('readonly'); brief.select(); document.execCommand('copy'); brief.setAttribute('readonly', ''); } catch (e) {}
        done();
      }
      logAction('Copy brief');
    });
    if (dlBtn) dlBtn.addEventListener('click', function () {
      var brief = document.querySelector('[data-picks-brief]');
      if (!brief) return;
      function dl(name, content, type) {
        var blob = new Blob([content], { type: type });
        var url = URL.createObjectURL(blob);
        var a = document.createElement('a');
        a.href = url; a.download = name;
        document.body.appendChild(a); a.click(); document.body.removeChild(a);
        setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
      }
      dl('chronicle-canon-picks.md', brief.value, 'text/markdown');
      dl('chronicle-canon-picks.json', JSON.stringify(picks, null, 2), 'application/json');
      logAction('Download brief');
    });
    if (resetBtn) resetBtn.addEventListener('click', function () {
      if (!confirm('Clear all picks (including notes)?')) return;
      picks = {};
      savePicks();
      location.reload();
    });
    // Initial render — defensive: the picks-hydrate block already
    // populated `picks`, so we can render even if other blocks fail.
    renderPicksPanel();
    var n = 0;
    if (copyBtn) n++;
    if (dlBtn) n++;
    if (resetBtn) n++;
    recordBinding('picks-panel buttons', n);
  });

  // ============================================================
  // Block 9 — Copy diagnostic report button.
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
        navigator.clipboard.writeText(report).then(done, function () { alert(report); done(); });
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

  // ============================================================
  // Dashboard fill — runs once after all blocks. Auto-expands the
  // <details> element if any block FAILED so collapsed-by-default
  // can't hide a failure.
  // ============================================================
  function renderDashboard() {
    var panel = document.querySelector('[data-canon-diagnostics]');
    if (!panel) return;
    var failed = diagnostics.blocks.filter(function (b) { return b.status !== 'OK'; }).length;
    panel.setAttribute('data-status', failed > 0 ? 'fail' : 'ok');
    if (failed > 0) panel.setAttribute('open', '');
    var summary = panel.querySelector('[data-diagnostics-status]');
    if (summary) {
      summary.textContent = failed > 0
        ? failed + ' of ' + diagnostics.blocks.length + ' init blocks FAILED'
        : 'All ' + diagnostics.blocks.length + ' init blocks OK';
    }
    var blockList = panel.querySelector('[data-init-block-list]');
    if (blockList) {
      blockList.innerHTML = '';
      diagnostics.blocks.forEach(function (b) {
        var li = document.createElement('li');
        li.className = 'chronicle-canon__diagnostics-item chronicle-canon__diagnostics-item--' + (b.status === 'OK' ? 'ok' : 'fail');
        li.textContent = (b.status === 'OK' ? '✓ ' : '✗ ') + b.name;
        if (b.error) {
          var err = document.createElement('code');
          err.textContent = ' — ' + b.error;
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
        li.className = 'chronicle-canon__diagnostics-item chronicle-canon__diagnostics-item--fail';
        li.textContent = '(no bindings recorded)';
        bindingList.appendChild(li);
      } else {
        diagnostics.bindings.forEach(function (b) {
          var li = document.createElement('li');
          li.className = 'chronicle-canon__diagnostics-item chronicle-canon__diagnostics-item--' + (b.count > 0 ? 'ok' : 'warn');
          li.textContent = b.name + ': ' + b.count;
          bindingList.appendChild(li);
        });
      }
    }
    var featureList = panel.querySelector('[data-feature-list]');
    if (featureList) {
      featureList.innerHTML = '';
      diagnostics.features.forEach(function (f) {
        var li = document.createElement('li');
        li.className = 'chronicle-canon__diagnostics-item chronicle-canon__diagnostics-item--' + (f.supported ? 'ok' : 'warn');
        li.textContent = (f.supported ? '✓ ' : '– ') + f.name;
        featureList.appendChild(li);
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
  // Trigger init. `defer` guarantees DOM is parsed; belt-and-
  // suspenders against document.readyState anyway.
  // ============================================================
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
