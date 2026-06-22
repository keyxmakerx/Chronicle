/**
 * surface_demo.js — a worked example of a Chronicle *System* consuming the
 * dynamic-surface frame (Chronicle.surface). It plays exactly the role a real
 * game-system pack (e.g. Draw Steel) plays: it registers box-body renderers and
 * mounts a character SHEET from a declarative schema. The FRAME owns the motion,
 * the boxes, the overlay stack and the data provider; this file only supplies
 * (a) sample character DATA and (b) the per-box render functions.
 *
 * Mounted by the Design Lab surface-demo page (/admin/design-lab). Pure browser
 * JS, no node runtime. The provider is SEEDED (zero network) so the demo works
 * offline. Intentionally Draw Steel-flavoured so it reads like a real sheet, but
 * nothing here is system-specific machinery — swap the data + renderers and the
 * same frame renders any system's sheet.
 *
 * Load ordering: the page body renders BEFORE the global <script> tags in
 * base.templ, so dynamic_surface.js may not have run yet when this file is
 * parsed. We therefore poll for Chronicle.surface (the deferred frame runs after
 * parse, so once it exists the DOM — and our mount targets — are ready too).
 */
(function () {
  'use strict';

  // ── sample character (stands in for a server entities/:id/surface payload) ──
  var HERO = {
    name: 'Tyne Brightwood',
    initials: 'TB',
    ancestry: 'Human',
    className: 'Tactician',
    level: 4,
    stamina: { current: 18, max: 34 },
    recoveries: { current: 6, max: 8, value: 11 },
    resource: { name: 'Focus', value: 5 },
    characteristics: [
      { key: 'Might', value: 2 },
      { key: 'Agility', value: 1 },
      { key: 'Reason', value: 2 },
      { key: 'Intuition', value: 1 },
      { key: 'Presence', value: -1 }
    ],
    backstory: 'A veteran field-commander turned wandering strategist. Tyne reads a ' +
      'battlefield the way others read a book — and is never short of a plan, however ' +
      'reckless. Carries a worn standard from a company that no longer exists.',
    inventory: [
      { name: 'Tactician’s Blade', note: 'Light weapon · +2 damage' },
      { name: 'Field Standard', note: 'Rally allies within 10' },
      { name: 'Healing Potion', qty: 2 }
    ]
  };

  // esc — coerce + HTML-escape (Chronicle.escapeHtml expects a string).
  function esc(v) {
    var s = (v == null) ? '' : String(v);
    return (window.Chronicle && Chronicle.escapeHtml) ? Chronicle.escapeHtml(s)
      : s.replace(/[&<>"']/g, function (c) {
          return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
        });
  }

  function signed(n) { return (n >= 0 ? '+' : '') + n; }

  function bar(label, cur, max, mod) {
    var pct = max ? Math.max(0, Math.min(100, Math.round((cur / max) * 100))) : 0;
    return '<div class="sd-stat">' +
      '<div class="sd-stat__row"><span>' + esc(label) + '</span>' +
      '<span class="sd-stat__num">' + esc(cur) + ' / ' + esc(max) + '</span></div>' +
      '<div class="sd-bar"><span class="sd-bar__fill ' + (mod || '') + '" style="width:' + pct + '%"></span></div>' +
      '</div>';
  }

  // ── box-body renderers — a System registers these; the frame calls each with
  //    (boxDef, providerData) and paints the returned HTML into that box body ──
  function registerRenderers(surface) {
    surface.registerBox('demo-vitals', function (def, hero) {
      hero = hero || HERO;
      var s = hero.stamina, r = hero.recoveries;
      return '<div class="sd-vitals">' +
        bar('Stamina', s.current, s.max, 'sd-bar--hp') +
        bar('Recoveries', r.current, r.max, 'sd-bar--rec') +
        '<div class="sd-chips">' +
        '<span class="sd-chip"><b>' + esc(hero.resource.value) + '</b> ' + esc(hero.resource.name) + '</span>' +
        '<span class="sd-chip">Recovery value <b>' + esc(r.value) + '</b></span>' +
        '</div>' +
        '</div>';
    });

    surface.registerBox('demo-abilities', function (def, hero) {
      hero = hero || HERO;
      var cells = hero.characteristics.map(function (c) {
        return '<div class="sd-char"><div class="sd-char__v">' + esc(signed(c.value)) + '</div>' +
          '<div class="sd-char__k">' + esc(c.key) + '</div></div>';
      }).join('');
      return '<div class="sd-chars">' + cells + '</div>';
    });

    surface.registerBox('demo-backstory', function (def, hero) {
      hero = hero || HERO;
      return '<p class="sd-prose">' + esc(hero.backstory) + '</p>';
    });

    surface.registerBox('demo-inventory', function (def, hero) {
      hero = hero || HERO;
      var rows = hero.inventory.map(function (it) {
        var qty = it.qty ? ' <span class="sd-inv__qty">×' + esc(it.qty) + '</span>' : '';
        var note = it.note ? '<span class="sd-inv__note">' + esc(it.note) + '</span>' : '';
        return '<li class="sd-inv__item"><span class="sd-inv__name">' + esc(it.name) + qty + '</span>' + note + '</li>';
      }).join('');
      return '<ul class="sd-inv">' + rows + '</ul>';
    });
  }

  // rollOverlayHtml — the static body an action pushes as an overlay. A real
  // System would compute the roll; here it is canned so the demo is offline.
  function rollOverlayHtml() {
    return '<div class="sd-roll">' +
      '<h3 class="sd-roll__title">Power Roll · 2d10 + Might</h3>' +
      '<div class="sd-roll__dice"><span>7</span><span>+</span><span>4</span><span>+</span><span>2</span></div>' +
      '<p class="sd-roll__total">Total <b>13</b> — tier 2 hit</p>' +
      '<p class="sd-roll__hint">The frame pushed this as a <code>scale-fade</code> overlay. ' +
      'Press <kbd>Esc</kbd> or click the backdrop to dismiss.</p>' +
      '</div>';
  }

  // schema — what a server …/surface endpoint would emit. The provider is SEEDED
  // so no network call is made; the seed key 'demo:tyne' is shared by the inline
  // sheet and the launched full sheet so they stay in lock-step.
  function schema(seed) {
    return {
      provider: { key: 'demo:tyne', seed: seed },
      rows: [{
        columns: [
          { width: 7, boxes: [
            { id: 'demo-vitals', title: 'Vitals', block: 'demo-vitals', expand: 'expanded',
              transition: 'container-transform',
              actions: [{ label: 'Roll Power', on: 'overlay', transition: 'scale-fade', html: rollOverlayHtml() }] },
            { id: 'demo-abilities', title: 'Characteristics', block: 'demo-abilities', expand: 'expanded' }
          ] },
          { width: 5, boxes: [
            { id: 'demo-backstory', title: 'Backstory', block: 'demo-backstory', expand: 'collapsed', transition: 'expand' },
            { id: 'demo-inventory', title: 'Inventory', block: 'demo-inventory', expand: 'collapsed', transition: 'expand' }
          ] }
        ]
      }]
    };
  }

  // buildMiniCard — the "per-player mini profile" pattern: a compact card that
  // launches the full sheet via the frame's mini→full container-transform.
  function buildMiniCard(surface) {
    var card = document.createElement('button');
    card.type = 'button';
    card.className = 'sd-mini';
    card.setAttribute('aria-label', 'Open ' + HERO.name + "'s full character sheet");
    card.title = 'Open full character sheet';
    var pct = Math.round(HERO.stamina.current / HERO.stamina.max * 100);
    card.innerHTML =
      '<span class="sd-mini__avatar">' + esc(HERO.initials) + '</span>' +
      '<span class="sd-mini__meta">' +
      '<span class="sd-mini__name">' + esc(HERO.name) + '</span>' +
      '<span class="sd-mini__sub">' + esc(HERO.ancestry) + ' ' + esc(HERO.className) + ' · Lvl ' + esc(HERO.level) + '</span>' +
      '<span class="sd-mini__bar"><span style="width:' + pct + '%"></span></span>' +
      '</span>' +
      '<span class="sd-mini__go">Open full sheet ↗</span>';
    card.addEventListener('click', function () {
      var full = document.createElement('div');
      full.className = 'sd-full';
      full.innerHTML = '<h2 class="sd-full__title">' + esc(HERO.name) + '</h2>' +
        '<p class="sd-full__sub">' + esc(HERO.ancestry) + ' ' + esc(HERO.className) + ' · Level ' + esc(HERO.level) + '</p>';
      var mountEl = document.createElement('div');
      full.appendChild(mountEl);
      surface.mount(mountEl, schema(HERO));            // shares provider key 'demo:tyne'
      surface.launch(card, full, { label: HERO.name + ' — character sheet' });
    });
    return card;
  }

  // start — runs once the frame is present; registers renderers and mounts.
  function start() {
    if (window.__sdSurfaceDemoBooted) return;
    var surface = window.Chronicle && Chronicle.surface;
    if (!surface || !surface.mount || !surface.registerBox) return false;  // frame not ready
    window.__sdSurfaceDemoBooted = true;

    registerRenderers(surface);
    injectDemoStyles();

    var root = document.getElementById('surface-demo-root');
    if (root) surface.mount(root, schema(HERO));

    var miniSlot = document.getElementById('surface-demo-mini');
    if (miniSlot) miniSlot.appendChild(buildMiniCard(surface));
    return true;
  }

  // Poll for the deferred frame (see header note on load ordering).
  var tries = 0;
  (function waitForFrame() {
    if (start() === false && tries++ < 120) { window.setTimeout(waitForFrame, 30); }
  })();

  // injectDemoStyles — component styles for the sd-* demo bits (theme-aware via
  // the --surface-* token contract). JS-injected with an id guard, like the
  // frame and entity_tooltip.
  function injectDemoStyles() {
    if (document.getElementById('sd-surface-demo-styles')) return;
    var c = [
      '.sd-vitals{display:flex;flex-direction:column;gap:12px;}',
      '.sd-stat__row{display:flex;justify-content:space-between;font-size:13px;margin-bottom:4px;}',
      '.sd-stat__num{color:var(--surface-text-muted,#6b7280);font-variant-numeric:tabular-nums;}',
      '.sd-bar{height:8px;border-radius:999px;background:var(--surface-alt,#f3f4f6);overflow:hidden;}',
      '.sd-bar__fill{display:block;height:100%;border-radius:999px;background:var(--surface-accent,#6366f1);}',
      '.sd-bar__fill.sd-bar--hp{background:linear-gradient(90deg,#ef4444,#f59e0b);}',
      '.sd-bar__fill.sd-bar--rec{background:linear-gradient(90deg,#10b981,#22d3ee);}',
      '.sd-chips{display:flex;flex-wrap:wrap;gap:8px;margin-top:2px;}',
      '.sd-chip{font-size:12px;padding:3px 10px;border-radius:999px;background:var(--surface-alt,#f3f4f6);' +
        'color:var(--surface-text-body,#374151);}',
      '.sd-chip b{color:var(--surface-text,#111827);}',
      '.sd-chars{display:grid;grid-template-columns:repeat(5,1fr);gap:8px;}',
      '.sd-char{text-align:center;padding:8px 4px;border-radius:10px;background:var(--surface-alt,#f3f4f6);}',
      '.sd-char__v{font-size:18px;font-weight:700;color:var(--surface-text,#111827);font-variant-numeric:tabular-nums;}',
      '.sd-char__k{font-size:10px;text-transform:uppercase;letter-spacing:.05em;color:var(--surface-text-muted,#6b7280);margin-top:2px;}',
      '.sd-prose{font-size:13px;line-height:1.6;color:var(--surface-text-body,#374151);margin:0;}',
      '.sd-inv{list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:6px;}',
      '.sd-inv__item{display:flex;justify-content:space-between;gap:8px;font-size:13px;padding:6px 0;' +
        'border-bottom:1px solid var(--surface-border,#e5e7eb);}',
      '.sd-inv__item:last-child{border-bottom:0;}',
      '.sd-inv__name{color:var(--surface-text,#111827);}',
      '.sd-inv__qty{color:var(--surface-text-muted,#6b7280);}',
      '.sd-inv__note{color:var(--surface-text-muted,#6b7280);font-size:12px;text-align:right;}',
      // mini profile card
      '.sd-mini{display:flex;align-items:center;gap:14px;width:100%;max-width:420px;text-align:left;cursor:pointer;' +
        'padding:12px 14px;border-radius:14px;border:1px solid var(--surface-border,#e5e7eb);' +
        'background:var(--surface-bg,#fff);color:var(--surface-text,#111827);font:inherit;' +
        'box-shadow:var(--surface-elev-resting,0 1px 2px 0 rgba(0,0,0,.05));' +
        'transition:box-shadow var(--surface-dur,200ms) var(--surface-ease,ease),transform var(--surface-dur,200ms) var(--surface-ease,ease);}',
      '.sd-mini:hover{box-shadow:var(--surface-elev,0 8px 20px -6px rgba(0,0,0,.18));transform:translateY(-1px);}',
      '.sd-mini__avatar{flex:none;width:44px;height:44px;border-radius:50%;display:flex;align-items:center;justify-content:center;' +
        'font-weight:700;color:#fff;background:linear-gradient(135deg,var(--surface-accent,#6366f1),#a855f7);}',
      '.sd-mini__meta{display:flex;flex-direction:column;gap:3px;flex:1;min-width:0;}',
      '.sd-mini__name{font-weight:600;}',
      '.sd-mini__sub{font-size:12px;color:var(--surface-text-muted,#6b7280);}',
      '.sd-mini__bar{height:5px;border-radius:999px;background:var(--surface-alt,#f3f4f6);overflow:hidden;margin-top:2px;}',
      '.sd-mini__bar>span{display:block;height:100%;background:linear-gradient(90deg,#ef4444,#f59e0b);}',
      '.sd-mini__go{flex:none;font-size:12px;color:var(--surface-accent,#6366f1);font-weight:500;}',
      // roll overlay + launched full sheet
      '.sd-roll{padding:22px 24px;text-align:center;}',
      '.sd-roll__title{margin:0 0 14px;font-size:14px;font-weight:600;color:var(--surface-text,#111827);}',
      '.sd-roll__dice{display:flex;gap:8px;align-items:center;justify-content:center;font-size:22px;font-weight:700;' +
        'color:var(--surface-accent,#6366f1);}',
      '.sd-roll__dice>span:nth-child(even){font-size:15px;color:var(--surface-text-muted,#6b7280);}',
      '.sd-roll__total{margin:14px 0 4px;font-size:15px;color:var(--surface-text,#111827);}',
      '.sd-roll__hint{margin:0;font-size:12px;color:var(--surface-text-muted,#6b7280);}',
      '.sd-full{padding:22px 26px;}',
      '.sd-full__title{margin:0;font-size:20px;font-weight:700;color:var(--surface-text,#111827);}',
      '.sd-full__sub{margin:2px 0 18px;font-size:13px;color:var(--surface-text-muted,#6b7280);}'
    ].join('');
    var style = document.createElement('style');
    style.id = 'sd-surface-demo-styles';
    style.textContent = c;
    (document.head || document.documentElement).appendChild(style);
  }
})();
