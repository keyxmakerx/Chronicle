/**
 * character_surface.js — the generic box renderers for the `character_surface`
 * layout block (the "big widget" character sheet). It registers default box-body
 * renderers on the dynamic-surface frame so ANY entity renders as a sheet with
 * no game system installed; a System (e.g. Draw Steel) can later override these
 * by re-registering the same box names with richer renderers.
 *
 * The frame builds the boxes and mounts the seeded schema (emitted by the Go
 * `character_surface` block); these renderers just paint each box body from the
 * seed. Loaded globally after dynamic_surface.js, so the renderers are
 * registered before boot.js auto-mounts the `dynamic-surface` widget.
 *
 * The description box does NOT inline entry HTML — it mounts the shared `editor`
 * widget (same as the standard entry block), so role-aware secrets and edit
 * gating are identical. Vanilla browser JS, esbuild-validated (es2015).
 */
(function () {
  'use strict';
  if (typeof window === 'undefined' || !window.Chronicle || !Chronicle.surface) return;
  var surface = Chronicle.surface;

  function esc(v) {
    var s = (v == null) ? '' : String(v);
    return Chronicle.escapeHtml ? Chronicle.escapeHtml(s)
      : s.replace(/[&<>"']/g, function (c) {
          return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
        });
  }

  // Identity: portrait + type. The entity name rides the box title bar, so the
  // body shows the portrait and type only (no duplicate name).
  surface.registerBox('char-header', function (def, d) {
    d = d || {};
    var portrait = d.image
      ? '<img class="cs-ch__portrait" src="' + esc(d.image) + '" alt="">'
      : '<div class="cs-ch__portrait cs-ch__portrait--empty"><i class="fa-solid fa-user"></i></div>';
    var sub = esc(d.typeName || '');
    if (d.typeLabel) sub += ' · ' + esc(d.typeLabel);
    return '<div class="cs-ch__head">' + portrait +
      '<div class="cs-ch__id">' + (sub ? '<div class="cs-ch__sub">' + sub + '</div>' : '') + '</div></div>';
  });

  // Details: the entity's custom fields, grouped by section.
  surface.registerBox('char-attributes', function (def, d) {
    d = d || {};
    if (!d.sections || !d.sections.length) {
      return '<p class="cs-ch__empty">No details yet.</p>';
    }
    return d.sections.map(function (s) {
      var head = s.title ? '<h4 class="cs-ch__sec">' + esc(s.title) + '</h4>' : '';
      var rows = (s.fields || []).map(function (f) {
        return '<div class="cs-ch__attr"><dt>' + esc(f.label) + '</dt><dd>' + esc(f.value) + '</dd></div>';
      }).join('');
      return head + '<dl class="cs-ch__attrs">' + rows + '</dl>';
    }).join('');
  });

  // Description: mount the shared editor widget (role-aware, autosaving). Return
  // a DOM element so the frame appends it and Chronicle.mountWidgets mounts it.
  surface.registerBox('char-entry', function (def, d) {
    d = d || {};
    var e = d.editor;
    if (!e || !e.endpoint) return '<p class="cs-ch__empty">No description yet.</p>';
    var div = document.createElement('div');
    div.setAttribute('data-widget', 'editor');
    div.setAttribute('data-endpoint', e.endpoint);
    if (e.campaignId) div.setAttribute('data-campaign-id', e.campaignId);
    if (e.canEdit) div.setAttribute('data-editable', 'true');
    div.setAttribute('data-autosave', '30');
    div.setAttribute('data-csrf-token', e.csrf || '');
    return div;
  });

  injectStyles();

  function injectStyles() {
    if (document.getElementById('cs-character-surface-styles')) return;
    var c = [
      '.cs-character-surface{display:block;}',
      '.cs-ch__head{display:flex;gap:16px;align-items:center;}',
      '.cs-ch__portrait{flex:none;width:84px;height:84px;border-radius:14px;object-fit:cover;' +
        'background:var(--surface-alt,#f3f4f6);}',
      '.cs-ch__portrait--empty{display:flex;align-items:center;justify-content:center;' +
        'color:var(--surface-text-muted,#9ca3af);font-size:30px;}',
      '.cs-ch__name{font-size:20px;font-weight:700;color:var(--surface-text,#111827);line-height:1.2;}',
      '.cs-ch__sub{margin-top:3px;font-size:13px;color:var(--surface-text-muted,#6b7280);}',
      '.cs-ch__sec{margin:10px 0 6px;font-size:11px;font-weight:600;text-transform:uppercase;' +
        'letter-spacing:.05em;color:var(--surface-text-muted,#6b7280);}',
      '.cs-ch__sec:first-child{margin-top:0;}',
      '.cs-ch__attrs{margin:0;display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:8px 18px;}',
      '.cs-ch__attr{display:flex;flex-direction:column;gap:1px;border-bottom:1px solid var(--surface-border,#e5e7eb);padding-bottom:6px;}',
      '.cs-ch__attr dt{font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--surface-text-muted,#6b7280);}',
      '.cs-ch__attr dd{margin:0;font-size:14px;color:var(--surface-text,#111827);}',
      '.cs-ch__empty{margin:0;font-size:13px;color:var(--surface-text-muted,#6b7280);font-style:italic;}'
    ].join('');
    var style = document.createElement('style');
    style.id = 'cs-character-surface-styles';
    style.textContent = c;
    (document.head || document.documentElement).appendChild(style);
  }
})();
