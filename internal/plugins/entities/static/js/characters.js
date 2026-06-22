/**
 * characters.js — progressive enhancement for the Characters ("Cast") page.
 *
 * Each cast card is already a real link to the entity's page, so the page works
 * with NO JavaScript. When the dynamic-surface frame (Chronicle.surface) is
 * present, this upgrades the card's "quick look" button into a mini→full launch:
 * an overlay that grows from the card (container-transform) and shows the
 * entity's real preview (fetched from the existing /preview endpoint) with an
 * "Open full page" link. This is the frame's first production adopter.
 *
 * Served per-plugin at /static/plugins/entities/js/characters.js. Pure browser
 * JS (esbuild-validated, es2015). Re-wires after HTMX fragment swaps.
 */
(function () {
  'use strict';

  function esc(v) {
    var s = (v == null) ? '' : String(v);
    return (window.Chronicle && Chronicle.escapeHtml) ? Chronicle.escapeHtml(s)
      : s.replace(/[&<>"']/g, function (c) {
          return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
        });
  }

  // previewHTML — render the /preview JSON into the overlay panel body.
  function previewHTML(d, fullURL) {
    d = d || {};
    var img = d.image_path
      ? '<div class="cast-peek__img"><img src="' + esc(d.image_path) + '" alt="" loading="lazy"></div>'
      : '';
    var icon = d.type_icon ? '<i class="fa-solid ' + esc(d.type_icon) + '"></i> ' : '';
    var badge = '<span class="cast-peek__type" style="background:' + esc(d.type_color || '#6366f1') + '">' +
      icon + esc(d.type_name || '') + '</span>';
    var label = d.type_label ? '<span class="cast-peek__label">' + esc(d.type_label) + '</span>' : '';
    var attrs = '';
    if (d.attributes && d.attributes.length) {
      attrs = '<dl class="cast-peek__attrs">' + d.attributes.map(function (a) {
        return '<div><dt>' + esc(a.label) + '</dt><dd>' + esc(a.value) + '</dd></div>';
      }).join('') + '</dl>';
    }
    var excerpt = d.entry_excerpt ? '<p class="cast-peek__excerpt">' + esc(d.entry_excerpt) + '</p>' : '';
    var open = fullURL ? '<a class="cast-peek__open" href="' + esc(fullURL) + '">Open full page ↗</a>' : '';
    return '<div class="cast-peek">' +
      '<div class="cast-peek__head">' + img +
      '<div class="cast-peek__meta"><div class="cast-peek__badges">' + badge + label + '</div>' +
      '<h2 class="cast-peek__name">' + esc(d.name || '') + '</h2></div></div>' +
      attrs + excerpt + open +
      '</div>';
  }

  function errHTML(fullURL) {
    var open = fullURL ? ' <a class="cast-peek__open" href="' + esc(fullURL) + '">Open full page ↗</a>' : '';
    return '<div class="cast-peek"><p class="cast-peek__err">Couldn’t load the preview.' + open + '</p></div>';
  }

  // openPeek — launch the mini→full overlay from a card.
  function openPeek(card) {
    var surface = window.Chronicle && Chronicle.surface;
    var link = card.querySelector('[data-cast-link]');
    var fullURL = link ? link.getAttribute('href') : card.getAttribute('data-full-url');
    var previewURL = card.getAttribute('data-preview-url');

    // No frame / no preview endpoint: fall back to plain navigation.
    if (!surface || !surface.launch || !previewURL) {
      if (fullURL) window.location.href = fullURL;
      return;
    }

    var panel = document.createElement('div');
    panel.innerHTML = '<div class="cast-peek cast-peek--loading">Loading…</div>';
    surface.launch(card, panel, { label: card.getAttribute('data-entity-name') || 'Character' });

    var doFetch = (window.Chronicle && Chronicle.apiFetch) || (window.fetch && window.fetch.bind(window));
    if (!doFetch) { panel.innerHTML = errHTML(fullURL); return; }
    doFetch(previewURL, { headers: { Accept: 'application/json' }, credentials: 'same-origin' })
      .then(function (r) { if (!r.ok) throw new Error('preview ' + r.status); return r.json(); })
      .then(function (data) { panel.innerHTML = previewHTML(data, fullURL); })
      .catch(function () { panel.innerHTML = errHTML(fullURL); });
  }

  function wire(root) {
    var scope = root && root.querySelectorAll ? root : document;
    var btns = scope.querySelectorAll('[data-cast-peek]');
    Array.prototype.forEach.call(btns, function (btn) {
      if (btn._castWired) return;
      btn._castWired = true;
      btn.addEventListener('click', function (e) {
        e.preventDefault();
        e.stopPropagation();
        var card = btn.closest('[data-cast-card]');
        if (card) openPeek(card);
      });
    });
    injectStyles();
  }

  function boot() {
    wire(document);
    // Re-wire after HTMX swaps the page fragment. This script lives inside
    // #main-content, so a boosted nav re-executes it — register the global
    // listener at most once so handlers don't stack across boosted loads.
    if (!window.__castAfterSettle) {
      window.__castAfterSettle = true;
      document.addEventListener('htmx:afterSettle', function (e) { wire(e.target); });
    }
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();

  // injectStyles — overlay-body styles (theme-aware via the --surface-* tokens).
  function injectStyles() {
    if (document.getElementById('cast-peek-styles')) return;
    var c = [
      '.cast-peek{padding:0 0 18px;}',
      '.cast-peek--loading{padding:40px;text-align:center;color:var(--surface-text-muted,#6b7280);}',
      '.cast-peek__head{display:flex;gap:16px;align-items:flex-start;padding:20px 22px 14px;}',
      '.cast-peek__img{flex:none;width:96px;height:96px;border-radius:12px;overflow:hidden;background:var(--surface-alt,#f3f4f6);}',
      '.cast-peek__img img{width:100%;height:100%;object-fit:cover;}',
      '.cast-peek__meta{flex:1;min-width:0;}',
      '.cast-peek__badges{display:flex;flex-wrap:wrap;align-items:center;gap:8px;margin-bottom:6px;}',
      '.cast-peek__type{display:inline-flex;align-items:center;gap:5px;padding:2px 10px;border-radius:999px;' +
        'font-size:11px;font-weight:600;color:#fff;}',
      '.cast-peek__label{font-size:12px;color:var(--surface-text-muted,#6b7280);}',
      '.cast-peek__name{margin:0;font-size:20px;font-weight:700;color:var(--surface-text,#111827);line-height:1.2;}',
      '.cast-peek__attrs{margin:0;padding:0 22px;display:grid;grid-template-columns:1fr 1fr;gap:8px 18px;}',
      '.cast-peek__attrs div{display:flex;flex-direction:column;gap:1px;border-bottom:1px solid var(--surface-border,#e5e7eb);padding-bottom:6px;}',
      '.cast-peek__attrs dt{font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--surface-text-muted,#6b7280);}',
      '.cast-peek__attrs dd{margin:0;font-size:13px;color:var(--surface-text,#111827);}',
      '.cast-peek__excerpt{margin:14px 22px 0;font-size:13px;line-height:1.6;color:var(--surface-text-body,#374151);}',
      '.cast-peek__err{margin:0;padding:30px 22px;text-align:center;color:var(--surface-text-muted,#6b7280);}',
      '.cast-peek__open{display:inline-block;margin:16px 22px 0;font-size:13px;font-weight:600;color:var(--surface-accent,#6366f1);}'
    ].join('');
    var style = document.createElement('style');
    style.id = 'cast-peek-styles';
    style.textContent = c;
    (document.head || document.documentElement).appendChild(style);
  }
})();
