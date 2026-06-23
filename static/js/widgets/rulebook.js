/**
 * rulebook.js — interactive rules-glossary browser (Chronicle core widget).
 *
 * The "Rulebook" flagship: the dynamic-surface paradigm pointed at reference
 * content. Fetches the active game system's rules-glossary, groups the terms by
 * category, and renders them as a Chronicle.surface — a category nav beside a
 * column of expanding rule boxes — with client-side search and stackable
 * {@term} cross-reference overlays (read a referenced rule without losing your
 * place; Back/Escape pops the stack).
 *
 * Mount: <div data-widget="rulebook" data-campaign-id="..." data-mod="<system-slug>">
 * Auto-mounted by boot.js. Reuses the shared surface frame
 * (static/js/widgets/dynamic_surface.js) for box chrome, motion and the overlay
 * stack, so this widget only supplies data + the rule body renderer.
 *
 * Read-only: serves reference content, never mutates campaign data.
 */
(function () {
  'use strict';
  if (!window.Chronicle || !Chronicle.register) return;

  // Known rule categories get friendly labels + a stable display order; any
  // other category falls back to a title-cased slug, appended alphabetically.
  var CAT_LABELS = {
    condition: 'Conditions', movement: 'Movement', action: 'Actions',
    combat: 'Combat', resource: 'Resources', duration: 'Durations', help: 'Help'
  };
  var CAT_ORDER = ['condition', 'movement', 'action', 'combat', 'resource', 'duration'];

  function esc(s) {
    s = s == null ? '' : String(s);
    if (Chronicle.escapeHtml) return Chronicle.escapeHtml(s);
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;')
      .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }
  function lc(s) { return (s == null ? '' : String(s)).toLowerCase(); }
  function catLabel(slug) {
    if (CAT_LABELS[slug]) return CAT_LABELS[slug];
    slug = String(slug || 'general');
    return slug.charAt(0).toUpperCase() + slug.slice(1);
  }
  function catRank(slug) {
    var i = CAT_ORDER.indexOf(slug);
    return i === -1 ? 999 : i;
  }

  // ── styles (component-scoped; the surface frame injects its own cs-* CSS) ──
  function injectStyles() {
    if (document.getElementById('rb-styles')) return;
    var css = [
      '.rb{display:block;margin:0 0 4px;}',
      '.rb-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-bottom:14px;}',
      '.rb-title{display:flex;align-items:center;gap:8px;font-size:17px;font-weight:700;color:var(--surface-text,#111827);}',
      '.rb-title i{color:var(--surface-accent,#6366f1);}',
      '.rb-search{flex:0 1 240px;padding:7px 11px;border:1px solid var(--surface-border,#e5e7eb);border-radius:9px;background:var(--surface-bg,#fff);color:var(--surface-text,#111827);font:inherit;font-size:13px;}',
      '.rb-search:focus{outline:none;border-color:var(--surface-accent,#6366f1);}',
      '.rb-body{display:flex;gap:16px;align-items:flex-start;}',
      '.rb-nav{flex:none;width:170px;display:flex;flex-direction:column;gap:2px;position:sticky;top:12px;}',
      '.rb-nav__btn{display:flex;align-items:center;justify-content:space-between;gap:8px;padding:7px 10px;border:0;border-radius:8px;background:none;color:var(--surface-text-body,#374151);font:inherit;font-size:13px;text-align:left;cursor:pointer;}',
      '.rb-nav__btn:hover{background:var(--surface-alt,#f3f4f6);}',
      '.rb-nav__btn[aria-current="true"]{background:var(--surface-alt,#f3f4f6);color:var(--surface-accent,#6366f1);font-weight:600;}',
      '.rb-nav__count{font-size:11px;opacity:.55;}',
      '.rb-surface{flex:1;min-width:0;}',
      '.rb-empty{padding:18px 4px;color:var(--surface-text-body,#374151);opacity:.7;font-size:13px;}',
      '.rb-ref{color:var(--surface-accent,#6366f1);cursor:pointer;border-bottom:1px dotted currentColor;text-decoration:none;}',
      '.rb-ref:hover{opacity:.8;}',
      '.rb-desc{line-height:1.55;white-space:pre-line;}',
      '.rb-detail__head{display:flex;align-items:baseline;gap:10px;margin-bottom:10px;}',
      '.rb-detail__name{font-size:18px;font-weight:700;color:var(--surface-text,#111827);}',
      '.rb-detail__cat{font-size:11px;text-transform:uppercase;letter-spacing:.04em;opacity:.55;}',
      '.rb-detail__desc{color:var(--surface-text-body,#374151);line-height:1.55;white-space:pre-line;}',
      '@media (max-width:640px){.rb-body{flex-direction:column;}.rb-nav{flex-direction:row;flex-wrap:wrap;width:auto;position:static;}}'
    ].join('');
    var s = document.createElement('style');
    s.id = 'rb-styles';
    s.textContent = css;
    (document.head || document.documentElement).appendChild(s);
  }

  // ── {@category term|display} cross-references ──
  // Escape the plain text around each ref and emit a clickable span whose
  // data-rb-slug is the looked-up term (display defaults to the term).
  function renderRefs(text) {
    text = text == null ? '' : String(text);
    var re = /\{@([a-z0-9_-]+)\s+([^|}]+?)(?:\|([^}]+))?\}/gi;
    var out = '', last = 0, m;
    while ((m = re.exec(text))) {
      out += esc(text.slice(last, m.index));
      var term = m[2].trim();
      var disp = (m[3] != null ? m[3] : m[2]).trim();
      out += '<a class="rb-ref" role="button" tabindex="0" data-rb-slug="' + esc(term) +
        '" data-rb-cat="' + esc(m[1]) + '">' + esc(disp) + '</a>';
      last = m.index + m[0].length;
    }
    out += esc(text.slice(last));
    return out;
  }

  function ruleBodyHtml(entry) {
    if (!entry) return '';
    return '<div class="rb-desc">' + renderRefs(entry.description || '') + '</div>';
  }
  function ruleDetailHtml(entry) {
    if (!entry) return '';
    var cat = entry.properties && entry.properties.category ? entry.properties.category : '';
    return '<div class="rb-detail">' +
      '<div class="rb-detail__head">' +
        '<span class="rb-detail__name">' + esc(entry.name || entry.slug || 'Rule') + '</span>' +
        (cat ? '<span class="rb-detail__cat">' + esc(catLabel(cat)) + '</span>' : '') +
      '</div>' +
      '<div class="rb-detail__desc">' + renderRefs(entry.description || '') + '</div>' +
    '</div>';
  }

  // Box renderer: the per-rule data rides on the box def (def.rule), so no
  // provider/fetch is needed — the glossary is already in memory.
  function rRule(def) { return ruleBodyHtml(def && def.rule); }
  var boxesRegistered = false;
  function registerBoxes() {
    if (boxesRegistered) return;
    var s = Chronicle.surface;
    if (!s || !s.registerBox) return;
    s.registerBox('rulebook-rule', rRule);
    boxesRegistered = true;
  }

  // Build a single-column surface schema, one expanding box per rule.
  function buildSchema(scope, entries) {
    var boxes = entries.map(function (e) {
      return {
        id: 'rule:' + scope + ':' + (e.slug || e.name),
        title: e.name || e.slug || 'Rule',
        block: 'rulebook-rule',
        expand: 'collapsed',
        transition: 'expand',
        rule: e
      };
    });
    return { rows: [{ columns: [{ width: 12, boxes: boxes }] }] };
  }

  function fetchGlossary(cid, mod) {
    var url = '/campaigns/' + encodeURIComponent(cid) +
      '/systems/' + encodeURIComponent(mod) + '/rules-glossary';
    if (Chronicle.apiFetch) {
      return Chronicle.apiFetch(url).then(function (r) { return r && r.json ? r.json() : []; });
    }
    return fetch(url, { credentials: 'same-origin' }).then(function (r) { return r.json(); });
  }

  // ── stateful wiring (state lives on root._rb so multiple mounts are safe) ──

  function renderNav(st) {
    st.navEl.innerHTML = st.cats.map(function (c) {
      var on = c === st.active && !st.query;
      return '<button type="button" class="rb-nav__btn" data-rb-cat="' + esc(c) + '"' +
        (on ? ' aria-current="true"' : '') + '>' +
        '<span>' + esc(catLabel(c)) + '</span>' +
        '<span class="rb-nav__count">' + st.groups[c].length + '</span>' +
      '</button>';
    }).join('');
  }

  // (Re)build the rule surface: the active category, or — while searching —
  // every matching rule across all categories.
  function refresh(st) {
    var list;
    if (st.query) {
      list = [];
      st.cats.forEach(function (c) {
        st.groups[c].forEach(function (e) {
          var hay = lc((e.name || '') + ' ' + (e.description || ''));
          if (hay.indexOf(st.query) >= 0) list.push(e);
        });
      });
    } else {
      list = st.groups[st.active] || [];
    }
    if (st.surfaceEl._csSurfaceCleanup) {
      try { st.surfaceEl._csSurfaceCleanup(); } catch (e) { /* already gone */ }
      st.surfaceEl._csSurfaceCleanup = null;
    }
    st.surfaceEl.innerHTML = '';
    if (!list.length) {
      var d = document.createElement('div');
      d.className = 'rb-empty';
      d.textContent = st.query
        ? ('No rules match “' + st.searchEl.value + '”.')
        : 'No rules in this category.';
      st.surfaceEl.appendChild(d);
      return;
    }
    if (Chronicle.surface && Chronicle.surface.mount) {
      Chronicle.surface.mount(st.surfaceEl, buildSchema(st.query ? 'search' : st.active, list));
    }
  }

  // Open a cross-referenced rule as a stacked overlay; nested refs in the
  // overlay re-use the same handler, so the stack can grow ("Back" pops it).
  function openRef(st, slug) {
    var entry = st.bySlug[slug];
    if (!entry || !Chronicle.surface || !Chronicle.surface.overlay) return;
    var ov = Chronicle.surface.overlay.push(ruleDetailHtml(entry), {
      transition: 'deal', label: entry.name || 'Rule'
    });
    if (ov && ov.panel) {
      ov.panel.addEventListener('click', st.onRefClick);
      ov.panel.addEventListener('keydown', st.onRefClick);
    }
  }

  function setup(root, entries) {
    var groups = {}, bySlug = {};
    entries.forEach(function (e) {
      if (!e) return;
      var cat = (e.properties && e.properties.category) || 'general';
      (groups[cat] || (groups[cat] = [])).push(e);
      if (e.slug) bySlug[e.slug] = e;
    });
    var cats = Object.keys(groups).sort(function (a, b) {
      var ra = catRank(a), rb = catRank(b);
      return ra !== rb ? ra - rb : a.localeCompare(b);
    });
    if (!cats.length) return;
    cats.forEach(function (c) {
      groups[c].sort(function (a, b) { return (a.name || '').localeCompare(b.name || ''); });
    });

    root.classList.add('rb');
    root.innerHTML =
      '<div class="rb-head">' +
        '<div class="rb-title"><i class="fa-solid fa-book-open"></i><span>Rulebook</span></div>' +
        '<input type="search" class="rb-search" placeholder="Search rules…" aria-label="Search rules">' +
      '</div>' +
      '<div class="rb-body">' +
        '<nav class="rb-nav" aria-label="Rule categories"></nav>' +
        '<div class="rb-surface"></div>' +
      '</div>';

    var st = {
      groups: groups, bySlug: bySlug, cats: cats, active: cats[0], query: '', _t: null,
      navEl: root.querySelector('.rb-nav'),
      surfaceEl: root.querySelector('.rb-surface'),
      searchEl: root.querySelector('.rb-search')
    };
    root._rb = st;

    st.onRefClick = function (e) {
      var a = e.target && e.target.closest ? e.target.closest('.rb-ref') : null;
      if (!a) return;
      if (e.type === 'keydown' && e.key !== 'Enter' && e.key !== ' ' && e.key !== 'Spacebar') return;
      e.preventDefault();
      openRef(st, a.getAttribute('data-rb-slug'));
    };
    st.onNavClick = function (e) {
      var b = e.target && e.target.closest ? e.target.closest('.rb-nav__btn') : null;
      if (!b) return;
      var c = b.getAttribute('data-rb-cat');
      if (!c) return;
      st.active = c;
      if (st.query) { st.query = ''; st.searchEl.value = ''; }
      renderNav(st);
      refresh(st);
    };
    st.onSearch = function () {
      if (st._t) clearTimeout(st._t);
      st._t = setTimeout(function () {
        st.query = lc(st.searchEl.value);
        renderNav(st);
        refresh(st);
      }, 120);
    };

    st.surfaceEl.addEventListener('click', st.onRefClick);
    st.surfaceEl.addEventListener('keydown', st.onRefClick);
    st.navEl.addEventListener('click', st.onNavClick);
    st.searchEl.addEventListener('input', st.onSearch);

    renderNav(st);
    refresh(st);
  }

  Chronicle.register('rulebook', {
    init: function (root) {
      if (!root || root._rb) return;
      var ds = root.dataset || {};
      var cid = ds.campaignId || '';
      var mod = ds.mod || '';
      if (!cid || !mod) return;
      injectStyles();
      registerBoxes();
      fetchGlossary(cid, mod).then(function (entries) {
        // No glossary → the widget stays invisible (an empty mount div).
        if (!Array.isArray(entries) || !entries.length || root._rb) return;
        setup(root, entries);
      }).catch(function () { /* reference is best-effort */ });
    },
    destroy: function (root) {
      var st = root && root._rb;
      if (!st) return;
      if (st._t) clearTimeout(st._t);
      if (st.surfaceEl && st.surfaceEl._csSurfaceCleanup) {
        try { st.surfaceEl._csSurfaceCleanup(); } catch (e) { /* already gone */ }
      }
      if (st.searchEl) st.searchEl.removeEventListener('input', st.onSearch);
      if (st.navEl) st.navEl.removeEventListener('click', st.onNavClick);
      if (st.surfaceEl) {
        st.surfaceEl.removeEventListener('click', st.onRefClick);
        st.surfaceEl.removeEventListener('keydown', st.onRefClick);
      }
      root._rb = null;
      root.classList.remove('rb');
      root.innerHTML = '';
    }
  });
}());
