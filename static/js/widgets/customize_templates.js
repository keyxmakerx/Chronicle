/**
 * customize_templates.js: a category's Starting text tab in Customize.
 *
 * Lists the campaign's content templates (the text a new page can start
 * with) with the draft's "templates" section laid over them. Nothing is
 * written until the owner presses Save on the one save bar. Section shape,
 * applied by internal/app/customize_templates.go:
 *
 *   { items: { "<id>": { name, description, icon, content_json, content_html, delete },
 *              "new-<slug>": { new, name, description, icon, content_json, content_html } },
 *     _changes: [{ label, href }] }
 *
 * An existing template's item holds only what differs from what is saved.
 *
 * Config attributes:
 *   data-campaign-id  -- the campaign
 *   data-href         -- this tab's address, for the save bar's lines
 */
(function () {
  'use strict';

  var SECTION = 'templates';
  var ICONS = [
    ['fa-file-lines', 'File'], ['fa-scroll', 'Scroll'], ['fa-user', 'Person'], ['fa-location-dot', 'Location'],
    ['fa-list-check', 'Checklist'], ['fa-dragon', 'Dragon'], ['fa-shield-halved', 'Shield'], ['fa-book', 'Book'],
    ['fa-map', 'Map'], ['fa-crown', 'Crown'], ['fa-skull', 'Skull'], ['fa-wand-magic-sparkles', 'Magic']
  ];
  var BLANK_JSON = '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Start writing..."}]}]}';
  var BLANK_HTML = '<p>Start writing...</p>';
  var KEYS = ['name', 'description', 'icon', 'content_json', 'content_html'];

  function esc(s) { return Chronicle.escapeHtml(s); }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }
  function icon(i) { return /^fa-[a-z0-9-]+$/.test(i || '') ? esc(i) : 'fa-file-lines'; }

  Chronicle.register('customize-templates', {
    init: function (el, config) {
      var campaignId = config.campaignId || el.getAttribute('data-campaign-id') || '';
      var HREF = el.getAttribute('data-href') || '';
      var saved = [], loading = true, failed = false;
      // The open form: { key, form } or null. Held here so a redraw from a
      // draft change doesn't lose what is being typed.
      var open = null;
      var alive = true;

      function draft() { return Chronicle.customizeDraft ? Chronicle.customizeDraft() : {}; }
      function items() { var s = draft()[SECTION]; return clone((s && s.items) || {}); }
      function savedById(id) { return saved.filter(function (t) { return String(t.id) === id; })[0]; }

      // The templates as the draft has them.
      function model() {
        var its = items();
        var list = saved.map(function (t) {
          var it = its[String(t.id)] || {};
          return {
            key: String(t.id), isNew: false, isGlobal: !!t.is_global, deleted: !!it.delete, changed: Object.keys(it).length > 0,
            name: it.name !== undefined ? it.name : t.name, description: it.description !== undefined ? it.description : (t.description || ''),
            icon: it.icon || t.icon, content_json: it.content_json !== undefined ? it.content_json : t.content_json,
            content_html: it.content_html !== undefined ? it.content_html : t.content_html, typeName: t.entity_type_name || ''
          };
        });
        Object.keys(its).forEach(function (k) {
          if (!/^new-/.test(k)) return;
          var it = its[k];
          list.push({ key: k, isNew: true, isGlobal: false, deleted: false, changed: true, name: it.name || '', description: it.description || '',
            icon: it.icon || 'fa-file-lines', content_json: it.content_json || BLANK_JSON, content_html: it.content_html || BLANK_HTML, typeName: '' });
        });
        return list;
      }

      function changes(its) {
        return Object.keys(its).map(function (k) {
          var it = its[k], s = savedById(k), nm = it.name || (s && s.name) || 'Starting text';
          return { label: (it.new ? 'New starting text ' : it.delete ? 'Delete starting text ' : 'Starting text ') + nm, href: HREF };
        });
      }
      function commit(its, step) {
        if (!Chronicle.customizeSet) return;
        Chronicle.customizeSet(SECTION, Object.keys(its).length ? { items: its, _changes: changes(its) } : null, step ? { step: true } : undefined);
      }
      // put keeps only what differs from what is saved, then hands the
      // section to the frame.
      function put(key, patch) {
        var its = items(), it = its[key] || {};
        Object.keys(patch).forEach(function (k) { if (patch[k] === undefined) delete it[k]; else it[k] = patch[k]; });
        var s = savedById(key);
        if (s) {
          KEYS.forEach(function (k) { if (it[k] !== undefined && it[k] === (s[k] === undefined || s[k] === null ? '' : s[k])) delete it[k]; });
          if (it.delete === false) delete it.delete;
          // The preview HTML only goes with a changed body.
          if (it.content_json === undefined) delete it.content_html;
        }
        if (Object.keys(it).length) its[key] = it; else delete its[key];
        commit(its, true);
      }
      function newKey() {
        var its = items(), n = 1;
        while (its['new-t' + n]) n++;
        return 'new-t' + n;
      }

      function draw() {
        if (!alive) return;
        if (loading) { el.innerHTML = '<div class="space-y-3" aria-busy="true"><div class="skeleton h-4 w-1/3"></div><div class="skeleton h-40"></div></div>'; return; }
        if (failed) { el.innerHTML = '<p class="text-sm text-fg-secondary">The starting text could not be loaded. Reload the page to try again.</p>'; return; }
        if (open) { drawForm(); return; }
        var list = model();
        el.innerHTML = '<div class="space-y-4"><div><h1 class="czs-h1">Starting text</h1>' +
          '<p class="czs-lede">Pre-fill the page editor when creating new pages. Templates appear in the create form and the page editor’s “/” menu.</p></div>' +
          '<div class="space-y-2">' + (list.length ? list.map(row).join('') : '<div class="card p-6 text-center"><p class="text-sm text-fg-secondary">No starting text yet.</p></div>') + '</div>' +
          '<button type="button" class="btn-primary text-sm" data-new><i class="fa-solid fa-plus word-ic" aria-hidden="true"></i> New starting text</button></div>';
      }
      function row(t) {
        var tag = t.isGlobal ? '<span class="text-[10px] text-fg-muted bg-surface-alt px-2 py-0.5 rounded-full">Built-in</span>' : '';
        if (t.isNew) tag += '<span class="czs-pill is-new">New</span>';
        else if (t.deleted) tag += '<span class="czs-pill is-warn">Deleted when you save</span>';
        else if (t.changed) tag += '<span class="czs-pill is-new">Changed</span>';
        var acts = '';
        if (!t.isGlobal) {
          acts = t.deleted
            ? '<button type="button" class="btn-secondary btn-sm" data-keep="' + esc(t.key) + '">Keep it</button>'
            : '<button type="button" class="w-7 h-7 flex items-center justify-center rounded text-fg-muted hover:text-accent hover:bg-accent/10" data-edit="' + esc(t.key) + '" aria-label="Edit ' + esc(t.name) + '" title="Edit"><i class="fa-solid fa-pen text-xs" aria-hidden="true"></i></button>' +
              '<button type="button" class="w-7 h-7 flex items-center justify-center rounded text-fg-muted hover:text-red-500" data-del="' + esc(t.key) + '" aria-label="Delete ' + esc(t.name) + '" title="Delete"><i class="fa-solid fa-trash text-xs" aria-hidden="true"></i></button>';
        }
        return '<div class="card p-4 flex items-center gap-3' + (t.deleted ? ' opacity-60' : '') + '"><span class="w-8 h-8 rounded-lg bg-accent/10 flex items-center justify-center shrink-0"><i class="fa-solid text-accent text-sm ' + icon(t.icon) + '" aria-hidden="true"></i></span>' +
          '<div class="flex-1 min-w-0"><div class="font-medium text-sm text-fg truncate">' + esc(t.name) + '</div>' +
          (t.description ? '<div class="text-xs text-fg-secondary truncate">' + esc(t.description) + '</div>' : '') +
          (t.typeName ? '<div class="text-[10px] text-fg-muted mt-0.5">' + esc(t.typeName) + '</div>' : '') + '</div>' +
          '<div class="flex items-center gap-2 shrink-0">' + tag + acts + '</div></div>';
      }

      function drawForm() {
        var f = open.form;
        el.innerHTML = '<div class="card p-6 space-y-4"><h3 class="text-sm font-semibold text-fg">' + (open.key ? 'Edit starting text' : 'New starting text') + '</h3>' +
          '<div><label class="block text-sm font-medium text-fg-body mb-1" for="cst-name">Name</label><input class="input w-full" id="cst-name" maxlength="200" placeholder="e.g., Session Recap, NPC Profile" value="' + esc(f.name) + '"><p class="czs-err" id="cst-name-e" hidden>Give it a name.</p></div>' +
          '<div><label class="block text-sm font-medium text-fg-body mb-1" for="cst-desc">Description (optional)</label><input class="input w-full" id="cst-desc" maxlength="500" placeholder="Brief description of what this is for" value="' + esc(f.description) + '"></div>' +
          '<div><label class="block text-sm font-medium text-fg-body mb-1" for="cst-icon">Icon</label><select class="input w-full" id="cst-icon">' +
          ICONS.map(function (i) { return '<option value="' + i[0] + '"' + (i[0] === f.icon ? ' selected' : '') + '>' + i[1] + '</option>'; }).join('') + '</select></div>' +
          '<div><label class="block text-sm font-medium text-fg-body mb-1" for="cst-json">Content (TipTap JSON)</label><textarea class="input w-full h-32 font-mono text-xs" id="cst-json" placeholder=\'{"type":"doc","content":[...]}\'>' + esc(f.content_json) + '</textarea>' +
          '<p class="czs-err" id="cst-json-e" hidden>That is not valid JSON.</p>' +
          '<p class="mt-1 text-xs text-fg-secondary">Paste TipTap/ProseMirror document JSON. To get this, create the text in any page’s editor, then copy the entry JSON from the browser developer tools.</p></div>' +
          '<div class="flex items-center gap-3"><button type="button" class="btn-primary text-sm" data-done>Done</button><button type="button" class="btn-secondary text-sm" data-cancel>Cancel</button></div></div>';
        var n = el.querySelector('#cst-name');
        if (n) n.focus();
      }
      function finishForm() {
        var f = open.form;
        f.name = el.querySelector('#cst-name').value.trim();
        f.description = el.querySelector('#cst-desc').value.trim();
        f.icon = el.querySelector('#cst-icon').value;
        f.content_json = el.querySelector('#cst-json').value.trim() || BLANK_JSON;
        var bad = false;
        el.querySelector('#cst-name-e').hidden = !!f.name;
        if (!f.name) bad = true;
        try { JSON.parse(f.content_json); el.querySelector('#cst-json-e').hidden = true; } catch (e) { el.querySelector('#cst-json-e').hidden = false; bad = true; }
        if (bad) return;
        // The preview HTML only describes the saved JSON; a changed body
        // drops it rather than keep a stale picture of the old text.
        var body = f.content_json === open.startJSON ? undefined : (f.content_json === BLANK_JSON ? BLANK_HTML : '');
        var key = open.key || newKey();
        if (open.key) {
          put(key, { name: f.name, description: f.description, icon: f.icon, content_json: f.content_json, content_html: body });
        } else {
          var its = items();
          its[key] = { new: true, name: f.name, description: f.description, icon: f.icon, content_json: f.content_json, content_html: f.content_json === BLANK_JSON ? BLANK_HTML : '' };
          commit(its, true);
        }
        open = null;
        draw();
      }

      function onClick(e) {
        var t = e.target.closest('[data-new],[data-edit],[data-del],[data-keep],[data-done],[data-cancel]');
        if (!t || !el.contains(t)) return;
        if (t.hasAttribute('data-new')) { open = { key: '', startJSON: BLANK_JSON, form: { name: '', description: '', icon: 'fa-file-lines', content_json: BLANK_JSON } }; draw(); }
        else if (t.hasAttribute('data-edit')) {
          var m = model().filter(function (x) { return x.key === t.getAttribute('data-edit'); })[0];
          if (!m) return;
          open = { key: m.key, startJSON: m.content_json, form: { name: m.name, description: m.description, icon: m.icon, content_json: m.content_json } };
          draw();
        } else if (t.hasAttribute('data-del')) {
          var k = t.getAttribute('data-del');
          if (/^new-/.test(k)) { var its = items(); delete its[k]; commit(its, true); } else put(k, { delete: true });
        } else if (t.hasAttribute('data-keep')) put(t.getAttribute('data-keep'), { delete: false });
        else if (t.hasAttribute('data-done')) finishForm();
        else if (t.hasAttribute('data-cancel')) { open = null; draw(); }
      }
      function onDraft(e) {
        var d = e.detail || {};
        if (d.reason === 'publish' && (d.applied || []).indexOf(SECTION) >= 0) { load(); return; }
        if (!open) draw();
      }

      function load() {
        loading = true; failed = false; draw();
        Chronicle.apiFetch('/campaigns/' + campaignId + '/content-templates').then(function (r) {
          if (!r.ok) throw new Error('load');
          return r.json();
        }).then(function (list) {
          saved = Array.isArray(list) ? list : [];
          loading = false; draw();
        }).catch(function () { loading = false; failed = true; draw(); });
      }

      el.addEventListener('click', onClick);
      document.addEventListener('chronicle:customize-draft', onDraft);
      el._cstDestroy = function () {
        alive = false;
        el.removeEventListener('click', onClick);
        document.removeEventListener('chronicle:customize-draft', onDraft);
      };
      load();
    },
    destroy: function (el) {
      if (el._cstDestroy) el._cstDestroy();
      el.innerHTML = '';
    }
  });
})();
