/**
 * customize_categories.js: Customize's Categories area.
 *
 * Draws the grid of categories and a category's Settings, and keeps the
 * category's name and tab marks in the frame's bar in step. It works from
 * data-state (the saved categories) with the draft's "categories"
 * section laid over it. Every change goes into that section through
 * Chronicle.customizeSet (customize_shell.js); nothing is written until the
 * owner presses Save. Section shape, applied by the entities plugin:
 *
 *   { items: { "<id>" | "new-<slug>": { new, name, name_plural, icon, color,
 *       claimable, parent, clear_parent, copy_from, menu, delete, move_to,
 *       layout, home, description, fields } },
 *     _changes: [{ label, href }] }
 *
 * An existing category's item holds only what differs from what is saved,
 * so a change put back by hand leaves the draft. A category deleted with
 * pages carries move_to, the saved category its pages move into on Save. The Customizer's page layout
 * (customize_page_editor.js) owns `layout`, `home` and `description` and
 * writes them through Chronicle.customizeCategoryWrite, so the save bar's
 * lines stay this widget's to make.
 */
(function () {
  'use strict';
  function gmTerm() { return (window.Chronicle && Chronicle.gmTerm) ? Chronicle.gmTerm() : 'GM'; }


  var SECTION = 'categories';
  // The item keys the Category home and Fields tabs write.
  var HOME_KEYS = { home: true, description: true, fields: true };
  var COLORS = ['#6366f1', '#0ea5e9', '#10b981', '#f59e0b', '#ef4444', '#ec4899', '#8b5cf6', '#64748b'];
  // Icons the picker offers, with the words a search matches beyond the name.
  var ICONS = [
    ['fa-map-location-dot', 'place map'], ['fa-map-pin', 'place pin'], ['fa-location-dot', 'place'], ['fa-globe', 'world'],
    ['fa-mountain', 'place'], ['fa-tree', 'forest nature'], ['fa-water', 'sea river'], ['fa-city', 'town'],
    ['fa-building', 'house'], ['fa-landmark', 'temple'], ['fa-church', 'temple faith'], ['fa-chess-rook', 'castle tower'],
    ['fa-dungeon', 'ruin'], ['fa-campground', 'camp'], ['fa-ship', 'boat'], ['fa-anchor', 'port sea'],
    ['fa-route', 'road journey'], ['fa-compass', 'travel'], ['fa-user', 'person character'], ['fa-users', 'people group faction'],
    ['fa-user-shield', 'player character'], ['fa-person-hiking', 'adventurer'], ['fa-crown', 'ruler noble'], ['fa-hat-wizard', 'mage'],
    ['fa-chess-knight', 'knight'], ['fa-hand-fist', 'fighter'], ['fa-masks-theater', 'performer'], ['fa-ghost', 'spirit undead'],
    ['fa-skull', 'death monster'], ['fa-dragon', 'monster creature'], ['fa-paw', 'beast animal creature'], ['fa-horse', 'animal mount'],
    ['fa-spider', 'creature'], ['fa-fish', 'creature'], ['fa-feather', 'bird'], ['fa-shield', 'faction guard'],
    ['fa-shield-halved', 'faction order'], ['fa-flag', 'faction nation'], ['fa-scroll', 'quest lore'], ['fa-book', 'lore book'],
    ['fa-book-open', 'journal'], ['fa-sticky-note', 'note'], ['fa-feather-pointed', 'writing'], ['fa-calendar', 'event'],
    ['fa-hourglass-half', 'time history'], ['fa-clock', 'time'], ['fa-star', 'favourite'], ['fa-gem', 'item treasure'],
    ['fa-coins', 'money treasure'], ['fa-box', 'item'], ['fa-key', 'item'], ['fa-ring', 'item'],
    ['fa-flask', 'potion alchemy'], ['fa-wand-sparkles', 'magic spell'], ['fa-hand-sparkles', 'magic'], ['fa-fire', 'spell'],
    ['fa-bolt', 'spell'], ['fa-moon', 'night'], ['fa-sun', 'day'], ['fa-eye', 'secret'],
    ['fa-khanda', 'weapon'], ['fa-gavel', 'law'], ['fa-heart', 'love'], ['fa-circle', 'other'],
    ['fa-puzzle-piece', 'other'], ['fa-dice-d20', 'game'], ['fa-music', 'song'], ['fa-language', 'language'],
    ['fa-leaf', 'plant'], ['fa-seedling', 'plant'], ['fa-virus', 'disease'], ['fa-cross', 'faith']
  ];

  function esc(s) { return Chronicle.escapeHtml(s); }
  function parse(s, d) { try { var v = JSON.parse(s || ''); return v == null ? d : v; } catch (e) { return d; } }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }
  // A colour the page shows as a background; anything else falls back so a
  // stored value can never break out of the style attribute.
  // Both go into markup, so they are escaped as well as checked.
  function colour(c) { return /^#[0-9a-fA-F]{3,8}$/.test(c || '') ? esc(c) : '#64748b'; }
  function icon(i) { return /^fa-[a-z0-9-]+$/.test(i || '') ? esc(i) : 'fa-file'; }
  function slugify(s) { return String(s).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 30) || 'category'; }
  function singular(s) { return String(s).replace(/ies$/i, 'y').replace(/([^s])s$/i, '$1'); }
  // Field names on the saved categories, in either spelling the server uses.
  function f(c, camel, snake) { return c[camel] !== undefined ? c[camel] : c[snake]; }

  function mount(el) {
    var state = parse(el.getAttribute('data-state'), {});
    var BASE = state.base || '';
    var saved = (state.categories || []).map(function (c) {
      return {
        id: f(c, 'id', 'ID'), name: f(c, 'name', 'Name') || '', plural: f(c, 'namePlural', 'name_plural') || f(c, 'NamePlural', 'NamePlural') || '',
        icon: f(c, 'icon', 'Icon') || 'fa-file', color: f(c, 'color', 'Color') || '#64748b',
        parent: f(c, 'parentId', 'parent_id') != null ? String(f(c, 'parentId', 'parent_id')) : (c.ParentID != null ? String(c.ParentID) : ''),
        claimable: !!f(c, 'claimable', 'Claimable'), pages: Number(f(c, 'pages', 'Pages')) || 0,
        trashed: Number(f(c, 'trashed', 'Trashed')) || 0,
        dmOnly: !!f(c, 'dmOnly', 'DMOnly')
      };
    });
    var inMenu = state.inMenu || {};
    var claiming = !!state.claiming;
    var KEY = state.key || '', TAB = state.tab || '';
    var alive = true, listeners = [];
    function on(t, type, fn, o) { t.addEventListener(type, fn, o); listeners.push([t, type, fn, o]); }
    function $(sel, root) { return (root || el).querySelector(sel); }
    function $$(sel, root) { return Array.prototype.slice.call((root || el).querySelectorAll(sel)); }

    // The frame's draft helpers, or a stand-in while its script is still
    // loading (customize_shell.js drains the queue when it mounts).
    var SHELL = el.closest('[data-widget="customize-shell"]');
    function shellDraft() {
      if (Chronicle.customizeDraft) return Chronicle.customizeDraft();
      return SHELL ? parse(SHELL.getAttribute('data-draft'), {}) : {};
    }
    function shellSet(v, opts) {
      if (Chronicle.customizeSet) Chronicle.customizeSet(SECTION, v, opts);
      else (Chronicle._customizeQueue = Chronicle._customizeQueue || []).push([SECTION, v, opts]);
    }
    function shellFlush() { return Chronicle.customizeFlush ? Chronicle.customizeFlush() : Promise.resolve(true); }
    function section() { var d = shellDraft(); return (d && d[SECTION]) || { items: {} }; }
    function items() { return clone(section().items || {}); }
    function savedById(key) { return saved.filter(function (c) { return String(c.id) === key; })[0]; }

    // The categories as the draft has them.
    function model() {
      var its = items(), list = saved.map(function (c) {
        var it = its[String(c.id)] || {}, key = String(c.id);
        var parent = it.clear_parent ? '' : (it.parent !== undefined ? String(it.parent) : c.parent);
        return {
          key: key, id: c.id, isNew: false,
          name: it.name !== undefined ? it.name : c.name,
          plural: it.name_plural !== undefined ? it.name_plural : c.plural,
          icon: it.icon || c.icon, color: it.color || c.color,
          claimable: it.claimable !== undefined ? !!it.claimable : c.claimable,
          menu: it.menu !== undefined ? !!it.menu : inMenu[c.id] !== false,
          parent: parent, pages: c.pages, trashed: c.trashed, dmOnly: c.dmOnly,
          deleted: !!it.delete, moveTo: it.move_to || '', changed: settingsKeys(it).length > 0, layoutChanged: !!it.layout, fieldsChanged: !!it.fields
        };
      });
      Object.keys(its).forEach(function (k) {
        if (!/^new-/.test(k)) return;
        var it = its[k];
        list.push({
          key: k, isNew: true, name: it.name || '', plural: it.name_plural || '', icon: it.icon || 'fa-file', color: it.color || COLORS[0],
          claimable: !!it.claimable, menu: it.menu !== false, parent: it.parent ? String(it.parent) : '', pages: 0, trashed: 0,
          deleted: false, changed: true, copyFrom: it.copy_from || 0
        });
      });
      return list;
    }
    function find(key) { return model().filter(function (c) { return c.key === key; })[0]; }
    function kidsOf(key) { return model().filter(function (c) { return c.parent === key && !c.deleted; }); }

    // write puts one category's item back, keeping only what differs from
    // what is saved, then hands the whole section to the frame.
    function write(key, patch, opts) {
      var its = items(), it = its[key] || {};
      Object.keys(patch).forEach(function (k) { if (patch[k] === undefined) delete it[k]; else it[k] = patch[k]; });
      // A category being deleted keeps no layout or home change; Save would
      // refuse them.
      if (it.delete) { delete it.layout; delete it.page_layouts; delete it.home; delete it.description; delete it.fields; }
      var s = savedById(key);
      if (s) {
        if (it.name === s.name) delete it.name;
        if (it.name_plural === s.plural) delete it.name_plural;
        if (it.icon === s.icon) delete it.icon;
        if (it.color === s.color) delete it.color;
        if (it.claimable === s.claimable) delete it.claimable;
        if (it.menu !== undefined && it.menu === (inMenu[s.id] !== false)) delete it.menu;
        if (it.parent !== undefined && String(it.parent) === s.parent) delete it.parent;
        if (it.clear_parent && !s.parent) delete it.clear_parent;
        if (it.delete === false) delete it.delete;
      }
      if (Object.keys(it).length) its[key] = it; else delete its[key];
      commit(its, opts);
    }
    function commit(its, opts) {
      if (!Object.keys(its).length) { shellSet(null, opts); return; }
      shellSet({ items: its, _changes: changes(its) }, opts);
    }
    // The keys of an item that the Settings tab shows; the page layout and
    // the category home have tabs of their own.
    function settingsKeys(it) { return Object.keys(it).filter(function (k) { return !HOME_KEYS[k] && k !== 'layout' && k !== 'page_layouts'; }); }
    // The save bar's lines for this section.
    function changes(its) {
      var out = [], byKey = {};
      model().forEach(function (c) { byKey[c.key] = c; });
      Object.keys(its).forEach(function (k) {
        var it = its[k], s = savedById(k), nm = it.name_plural || (s && s.plural) || 'Category';
        if (it.new) out.push({ label: 'New category ' + nm, href: BASE + '/categories/' + k + '/settings' });
        else if (it.delete) {
          var to = it.move_to ? byKey[String(it.move_to)] : null;
          out.push({ label: 'Delete ' + (s ? s.plural : nm) + (to ? ', its pages move to ' + (to.plural || to.name) : ''), href: BASE + '/categories' });
        }
        else {
          if (it.layout || it.page_layouts) out.push({ label: nm + ' page layout', href: BASE + '/categories/' + k + '/layout' });
          if (it.home || it.description !== undefined) out.push({ label: nm + ' home', href: BASE + '/categories/' + k + '/home' });
          if (it.fields) out.push({ label: nm + ' fields', href: BASE + '/categories/' + k + '/fields' });
          if (settingsKeys(it).length) out.push({ label: nm + ' settings', href: BASE + '/categories/' + k + '/settings' });
        }
      });
      return out;
    }

    function chip(c, cls) {
      return '<span class="czs-cat-ic' + (cls ? ' ' + cls : '') + '" style="background:' + colour(c.color) + '"><i class="fa-solid ' + icon(c.icon) + '" aria-hidden="true"></i></span>';
    }
    function pagesText(n) { n = Number(n) || 0; return n + (n === 1 ? ' page' : ' pages'); }

    /* ---------- The grid ---------- */
    function drawGrid() {
      var all = model(), top = all.filter(function (c) { return !c.parent || !find(c.parent); });
      el.innerHTML = '<div class="czs-head-row"><div><h1 class="czs-h1">Categories</h1>' +
        '<p class="czs-lede">Each category is a kind of page. Open one to set up its page, its home, its fields and the text new pages start with.</p></div>' +
        '<button type="button" class="btn-primary" data-new><i class="fa-solid fa-plus word-ic" aria-hidden="true"></i>New category</button></div>' +
        '<div class="czs-cat-grid" hx-boost="true" hx-target="#main-content" hx-select="#main-content > *" hx-swap="innerHTML show:window:top">' +
        top.map(card).join('') + '</div>';
      if (window.htmx) window.htmx.process(el);
    }
    function card(c) {
      var kids = kidsOf(c.key), href = BASE + '/categories/' + c.key + '/' + (c.isNew ? 'settings' : 'layout');
      var pills = [];
      if (c.isNew) pills.push('<span class="czs-pill is-new">New</span>');
      if (c.deleted) {
        var to = c.moveTo ? find(c.moveTo) : null;
        pills.push('<span class="czs-pill is-warn">Deleted when you save</span>');
        if (to) pills.push('<span class="czs-pill">Pages move to ' + esc(to.plural || to.name) + '</span>');
      }
      else if (c.changed && !c.isNew) pills.push('<span class="czs-pill is-new">Changed</span>');
      if (c.dmOnly) pills.push('<span class="czs-pill is-dm"><i class="fa-solid fa-lock" aria-hidden="true"></i> Only the ' + gmTerm() + ' team sees it</span>');
      else pills.push('<span class="czs-pill">' + (c.menu ? 'In the menu' : 'Hidden from players’ menu') + '</span>');
      if (claiming && c.claimable && !c.dmOnly) pills.push('<span class="czs-pill">Players can claim</span>');
      // The ⋯ sits beside the link, not in it: a button inside a link is
      // not valid, and a long press on the card stays the browser's own.
      return '<div class="czs-cat-cell"><a class="czs-cat-card' + (c.deleted ? ' is-gone' : '') + '" href="' + esc(href) + '">' +
        '<span class="czs-meta">' + chip(c) + '<span style="min-width:0"><span class="czs-nm">' + esc(c.plural || c.name) + '</span>' +
        '<span class="czs-sub">' + (c.isNew ? 'Made in your draft' : pagesText(c.pages)) + '</span></span></span>' +
        (kids.length ? '<span class="czs-kids">Kinds: ' + esc(kids.map(function (k) { return k.plural || k.name; }).join(', ')) + '</span>' : '') +
        '<span class="czs-foot">' + pills.join('') + '</span></a>' +
        '<button type="button" class="czs-icon-btn czs-cat-more" data-more="' + esc(c.key) + '" aria-haspopup="menu" aria-expanded="false" aria-label="More for ' + esc(c.plural || c.name) + '">' +
        '<i class="fa-solid fa-ellipsis" aria-hidden="true"></i></button></div>';
    }

    /* ---------- A category's workspace ----------
     * The category's name and tabs are the frame's one bar
     * (customizeCatBarNav); this keeps the name and each tab's mark in step
     * with the draft. The facts about it (pages, menu) are on its card in
     * the Categories list and on Settings. */
    function drawWorkspace() {
      var c = find(KEY), head = $('[data-czs-ws-head]'), body = $('[data-czs-ws-settings]');
      var bar = SHELL ? SHELL.querySelector('[data-czs-catbar]') : null;
      if (!c) {
        head.hidden = false;
        head.innerHTML = '<div><h1 class="czs-h1">Not in your draft</h1><p class="czs-lede">This category was made in a draft that has since been saved or thrown away.</p></div>';
        if (body) body.innerHTML = '<a class="btn-secondary" href="' + esc(BASE + '/categories') + '">All categories</a>';
        return;
      }
      head.hidden = true;
      if (bar) {
        var nm = c.plural || c.name || 'New category', nmEl = bar.querySelector('[data-czs-catname]');
        if (nmEl && nmEl.textContent !== nm) nmEl.textContent = nm;
        bar.setAttribute('aria-label', nm);
        var marks = { settings: c.changed, layout: c.layoutChanged, fields: c.fieldsChanged };
        $$('[data-czs-cattab]', bar).forEach(function (t) {
          var dot = t.querySelector('.czs-dot');
          if (dot) dot.hidden = !marks[t.getAttribute('data-czs-cattab')];
        });
      }
      if (body && TAB === 'settings') drawSettings(c, body);
    }

    function drawSettings(c, body) {
      var focusId = document.activeElement && body.contains(document.activeElement) ? document.activeElement.id : '';
      var kids = kidsOf(c.key), parent = c.parent ? find(c.parent) : null;
      var blocked = c.isNew ? '' : (kids.length ? 'Delete its kinds first.' : '');
      var pagesNote = (c.pages + c.trashed) > 0 ? 'Its ' + pagesText(c.pages + c.trashed) + ((c.pages + c.trashed) === 1 ? ' moves' : ' move') + ' to a category you choose, so no page is deleted.' : 'It has no pages, so nothing else is lost.';
      body.innerHTML = '<h1 class="czs-h1">Settings</h1><p class="czs-lede">The name, icon and colour of ' + esc(c.plural || 'this category') + ', and who sees it in the menu.</p><div class="czs-form mt-6">' +
        '<div class="czs-split"><div class="czs-field"><label for="czc-plural">Name</label><input class="input" id="czc-plural" maxlength="100" value="' + esc(c.plural) + '"><p class="czs-err" id="czc-plural-e" hidden>A category needs a name.</p></div>' +
        '<div class="czs-field"><label for="czc-one">One of them is called</label><input class="input" id="czc-one" maxlength="100" value="' + esc(c.name) + '"><p class="czs-hint">Used in “New ' + esc((c.name || 'page').toLowerCase()) + '”.</p><p class="czs-err" id="czc-one-e" hidden>Give one of them a name too.</p></div></div>' +
        '<div class="czs-field"><span class="czs-lab" id="czc-ic-l">Icon and colour</span><div><button type="button" class="btn-secondary btn-sm" id="czc-icon" aria-haspopup="dialog">' + chip(c, 'sm') + '<span class="ml-2">Change icon</span></button></div>' +
        '<div class="czs-colors" role="group" aria-labelledby="czc-ic-l">' + COLORS.map(function (col) {
          return '<button type="button" data-col="' + col + '" aria-label="Colour ' + col + '" aria-pressed="' + (col.toLowerCase() === String(c.color).toLowerCase()) + '" style="background:' + col + '"></button>';
        }).join('') + '<input type="color" id="czc-color" value="' + esc(/^#[0-9a-fA-F]{6}$/.test(c.color) ? c.color : '#6366f1') + '" aria-label="Your own colour"></div></div>' +
        (c.dmOnly ? '<div class="czs-panel"><span class="czs-lab"><i class="fa-solid fa-lock" aria-hidden="true"></i> Only the ' + gmTerm() + ' team sees it</span><p class="czs-hint">Players get no menu entry, search result, link preview or relation for these pages, and cannot claim them. The owner, co-' + gmTerm() + 's and scribes see them as usual. Because of that it has no kinds of its own.</p></div>' : '') +
        (claiming && !c.dmOnly ? '<div class="czs-panel czs-row-set"><div><span class="czs-lab">Players can claim a page</span><p class="czs-hint">A player can make a page their own, like their character.</p></div>' +
          '<button type="button" class="toggle" role="switch" id="czc-claim" aria-checked="' + !!c.claimable + '" aria-label="Players can claim a page"><span class="toggle-knob"></span></button></div>' : '') +
        (c.dmOnly ? '' : '<div class="czs-panel czs-row-set"><div><span class="czs-lab">Shown in players’ menu</span><p class="czs-hint">A hidden category still works: players reach it through search and links, and you still see it in your menu.</p></div>' +
        '<button type="button" class="toggle" role="switch" id="czc-menu" aria-checked="' + !!c.menu + '" aria-label="Shown in players’ menu"><span class="toggle-knob"></span></button></div>') +
        (c.dmOnly ? '' : parent ?'<div class="czs-panel czs-row-set"><div><span class="czs-lab">A kind of ' + esc(parent.plural || parent.name) + '</span><p class="czs-hint">It sits under ' + esc(parent.plural || parent.name) + ' in the menu.</p></div>' +
          '<button type="button" class="btn-secondary btn-sm" id="czc-unparent">Make it a category of its own</button></div>' :
          '<div class="czs-field"><span class="czs-lab">Kinds within ' + esc(c.plural || 'this category') + '</span><p class="czs-hint">Like “Towns” within Places. Each kind is a category of its own that sits under this one in the menu.</p>' +
          '<div class="czs-kinds">' + (kids.length ? kids.map(function (k) {
            return '<div class="czs-kind">' + chip(k, 'sm') + '<span class="czs-nm">' + esc(k.plural || k.name) + (k.isNew ? ' <span class="czs-pill is-new">New</span>' : '') + '</span>' +
              '<a class="btn-ghost btn-sm" href="' + esc(BASE + '/categories/' + k.key + '/' + (k.isNew ? 'settings' : 'layout')) + '">Open</a>' +
              '<button type="button" class="czs-icon-btn" data-rm-kind="' + esc(k.key) + '" aria-label="Remove ' + esc(k.plural || k.name) + '"' +
              ' title="Remove"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>';
          }).join('') : '<p class="czs-hint">No kinds yet.</p>') + '</div>' +
          '<div class="czs-act-row"><input class="input" id="czc-kind" maxlength="100" placeholder="Add a kind, like “Dungeons”" style="flex:1;min-width:0;width:auto"><button type="button" class="btn-secondary" id="czc-kind-add"><i class="fa-solid fa-plus" aria-hidden="true"></i>Add</button></div></div>') +
        (c.deleted ? '<div class="czs-danger"><div><span class="czs-lab">Deleted when you save</span><p class="czs-hint">Nothing is gone yet.</p></div><button type="button" class="btn-secondary" id="czc-keep">Keep it</button></div>' :
          '<div class="czs-danger"><div><span class="czs-lab">Delete ' + esc(c.plural || 'this category') + '</span><p class="czs-hint">' + (c.isNew ? 'It is only in your draft, so nothing else changes.' : (blocked || pagesNote)) + '</p></div>' +
          '<button type="button" class="btn-danger" id="czc-delete"' + (blocked ? ' disabled' : '') + '><i class="fa-solid fa-trash-can word-ic" aria-hidden="true"></i>Delete category</button></div>') +
        '</div>';
      bindSettings(c, body);
      if (focusId && document.getElementById(focusId)) document.getElementById(focusId).focus();
    }

    function bindSettings(c, body) {
      function text(id, field, errId) {
        var inp = $('#' + id, body);
        inp.addEventListener('input', function () {
          var v = inp.value.trim();
          $('#' + errId, body).hidden = !!v;
          if (!v) return;
          var p = {}; p[field] = v;
          write(c.key, p);
          drawWorkspaceHead();
        });
      }
      text('czc-plural', 'name_plural', 'czc-plural-e');
      text('czc-one', 'name', 'czc-one-e');
      $('#czc-icon', body).addEventListener('click', function (e) {
        iconPicker(e.currentTarget, c.icon, function (v) { write(c.key, { icon: v }); drawWorkspace(); var b = $('#czc-icon', body); if (b) b.focus(); });
      });
      $$('[data-col]', body).forEach(function (b) {
        b.addEventListener('click', function () { write(c.key, { color: b.getAttribute('data-col') }); drawWorkspace(); });
      });
      $('#czc-color', body).addEventListener('change', function () { write(c.key, { color: this.value }); drawWorkspace(); });
      var claim = $('#czc-claim', body);
      if (claim) claim.addEventListener('click', function () { write(c.key, { claimable: !c.claimable }); drawWorkspace(); });
      var menuBtn = $('#czc-menu', body);
      if (menuBtn) menuBtn.addEventListener('click', function () { write(c.key, { menu: !c.menu }); drawWorkspace(); });
      var un = $('#czc-unparent', body);
      if (un) un.addEventListener('click', function () {
        if (c.isNew) write(c.key, { parent: undefined });
        else write(c.key, { parent: undefined, clear_parent: true });
        drawWorkspace();
      });
      var add = $('#czc-kind-add', body), kin = $('#czc-kind', body);
      function addKind() {
        var v = kin ? kin.value.trim() : '';
        if (!v) { if (kin) kin.focus(); return; }
        var key = newKey(v), its = items();
        its[key] = { new: true, name: singular(v), name_plural: v, icon: c.icon, color: c.color, parent: c.key };
        commit(its);
        drawWorkspace();
        var k2 = $('#czc-kind', body); if (k2) k2.focus();
      }
      if (add) add.addEventListener('click', addKind);
      if (kin) kin.addEventListener('keydown', function (e) { if (e.key === 'Enter') { e.preventDefault(); addKind(); } });
      $$('[data-rm-kind]', body).forEach(function (b) {
        b.addEventListener('click', function () { removeCategory(find(b.getAttribute('data-rm-kind'))); });
      });
      var del = $('#czc-delete', body);
      if (del) del.addEventListener('click', function () { removeCategory(c, true); });
      var keep = $('#czc-keep', body);
      if (keep) keep.addEventListener('click', function () { write(c.key, { delete: undefined, move_to: undefined }); drawWorkspace(); });
    }
    function drawWorkspaceHead() {
      // Redraw the header and tabs only, so a field being typed in keeps its caret.
      var body = $('[data-czs-ws-settings]');
      if (body) body.removeAttribute('data-czs-ws-settings');
      drawWorkspace();
      if (body) body.setAttribute('data-czs-ws-settings', '');
    }
    function newKey(name) {
      var base = 'new-' + slugify(name), key = base, n = 2, its = items();
      while (its[key]) key = base + '-' + (n++);
      return key;
    }
    // removeCategory takes a category out: one made in the draft simply
    // leaves it; a saved one is marked to be deleted on Save, after a confirm
    // that also asks where its pages go when it has any.
    function removeCategory(c, leave) {
      if (!c) return;
      if (c.isNew) {
        var its = items();
        Object.keys(its).forEach(function (k) { if (k === c.key || its[k].parent === c.key) delete its[k]; });
        commit(its);
        after(leave);
        return;
      }
      deleteDialog(c, function (moveTo) {
        write(c.key, { delete: true, move_to: moveTo || undefined });
        after(leave);
      });
    }
    function after(leave) {
      if (leave) shellFlush().then(function () { if (alive) location.href = BASE + '/categories'; });
      else draw();
    }
    // keepCategory takes a category back out of the deletions.
    function keepCategory(c) { write(c.key, { delete: undefined, move_to: undefined }); draw(); }

    // moveTargets is where a category's pages can go: a saved category that
    // stays, and, for pages only the GM team sees, another category only
    // they see, since the category is what hides those pages.
    function moveTargets(c) {
      return model().filter(function (t) {
        return !t.isNew && !t.deleted && t.key !== c.key && t.parent !== c.key && (!c.dmOnly || t.dmOnly);
      });
    }
    // targetLabel names a place the pages can go so two of the same name
    // can still be told apart: who sees it, then how many pages it has.
    function targetLabel(t, all) {
      var p = t.parent ? find(t.parent) : null;
      var base = (p ? (p.plural || p.name) + ' › ' : '') + (t.plural || t.name);
      var twin = all.filter(function (o) { return o !== t && (o.plural || o.name) === (t.plural || t.name); });
      if (t.dmOnly) base += ' (only the ' + gmTerm() + ' team sees it)';
      if (twin.length && twin.every(function (o) { return o.dmOnly === t.dmOnly; })) base += ' (' + pagesText(t.pages) + ')';
      return base;
    }

    // deleteDialog confirms a deletion. A category with pages names where
    // they go first; one with kinds, or whose pages have nowhere they can
    // go, says why and offers no delete.
    function deleteDialog(c, ok) {
      var nm = esc(c.plural || c.name), n = c.pages + c.trashed;
      var kids = kidsOf(c.key), targets = n ? moveTargets(c) : [];
      // Another category deleted in this draft may be sending its pages here.
      var inbound = model().filter(function (o) { return o.deleted && o.moveTo === c.key; })[0];
      var why = kids.length ? 'Delete its kinds first: ' + esc(kids.map(function (k) { return k.plural || k.name; }).join(', ')) + '.' :
        inbound ? 'The pages of ' + esc(inbound.plural || inbound.name) + ' are set to move here. Choose somewhere else for them first.' :
        n && !targets.length ? 'Its pages are for the ' + esc(gmTerm()) + ' team only, and no other category is. Move them one by one first, or make another category only the ' + esc(gmTerm()) + ' team sees.' : '';
      var trash = c.trashed ? ' (' + c.trashed + ' of them in the Trash)' : '';
      var body = why ? '<p id="czc-dd">' + why + '</p>' :
        n ? '<p id="czc-dd">Its ' + pagesText(n) + trash + (n === 1 ? ' moves' : ' move') + ' to another category first, so no page is deleted.</p>' +
          '<div class="czs-field"><label for="czc-moveto">Move its pages to</label><select class="input" id="czc-moveto">' +
          targets.map(function (t) { return '<option value="' + esc(t.key) + '">' + esc(targetLabel(t, targets)) + '</option>'; }).join('') + '</select>' +
          '<p class="czs-hint">They keep their words, tags and links. Its own page layout, home and fields are deleted with it; a value in a field the other category doesn’t have stays saved on the page but isn’t shown.</p></div>' +
          '<p class="czs-hint">Nothing happens until you save, and until then Discard or Undo brings it back.</p>' :
          '<p id="czc-dd">It has no pages, so nothing else is lost. It is deleted when you save, and until then Discard or Undo brings it back.</p>';
      var sc = document.createElement('div');
      sc.className = 'czs-scrim'; sc._czsCat = true;
      sc.innerHTML = '<div class="czs-dlg" role="alertdialog" aria-modal="true" aria-labelledby="czc-dt" aria-describedby="czc-dd" style="width:min(460px,100%)">' +
        '<h2 id="czc-dt">' + (why ? nm + ' can’t be deleted yet' : 'Delete ' + nm + '?') + '</h2>' + body +
        '<div class="czs-acts">' + (why ? '<button type="button" class="btn-secondary" data-x>Close</button>' :
          '<button type="button" class="btn-secondary" data-x>Keep it</button><button type="button" class="btn-danger" data-ok>' + (n ? 'Move pages and delete' : 'Delete category') + '</button>') + '</div></div>';
      document.body.appendChild(sc);
      var opener = document.activeElement;
      if (window.Chronicle && Chronicle.modal) Chronicle.modal(sc, { opener: null, escape: false, focus: false, aria: false });
      function close() { sc.remove(); if (opener && opener.focus && document.body.contains(opener)) opener.focus(); }
      $('[data-x]', sc).addEventListener('click', close);
      var okBtn = $('[data-ok]', sc);
      if (okBtn) okBtn.addEventListener('click', function () {
        var sel = $('#czc-moveto', sc);
        var v = sel ? sel.value : '';
        sc.remove();
        ok(v);
      });
      sc.addEventListener('pointerdown', function (e) { if (e.target === sc) close(); });
      sc.addEventListener('keydown', function (e) { if (e.key === 'Escape') close(); });
      ($('#czc-moveto', sc) || $('[data-x]', sc)).focus();
    }

    /* ---------- A card's ⋯ menu ---------- */
    function moreMenu(btn, c) {
      var p = document.createElement('div');
      p.className = 'czs-pop czs-menu'; p._czsCat = true;
      p.setAttribute('role', 'menu'); p.setAttribute('aria-label', btn.getAttribute('aria-label'));
      var href = BASE + '/categories/' + c.key + '/settings';
      p.innerHTML = '<a role="menuitem" href="' + esc(href) + '">Settings</a>' +
        (c.deleted ? '<button type="button" role="menuitem" data-act="keep">Keep it</button>' :
          '<button type="button" role="menuitem" class="is-bad" data-act="delete">' + (c.isNew ? 'Remove from draft' : 'Delete…') + '</button>');
      document.body.appendChild(p);
      var r = btn.getBoundingClientRect();
      p.style.left = Math.max(12, Math.min(r.right - p.offsetWidth, innerWidth - p.offsetWidth - 12)) + 'px';
      var below = r.bottom + 4 + p.offsetHeight <= innerHeight;
      p.style.top = (below ? r.bottom + 4 : Math.max(12, r.top - p.offsetHeight - 4)) + 'px';
      p.style.transformOrigin = (below ? 'top' : 'bottom') + ' right';
      btn.setAttribute('aria-expanded', 'true');
      var rows = $$('[role="menuitem"]', p);
      function close(back) {
        p.remove(); btn.setAttribute('aria-expanded', 'false');
        document.removeEventListener('pointerdown', off, true);
        if (back && document.body.contains(btn)) btn.focus();
      }
      function off(e) { if (!p.contains(e.target) && !btn.contains(e.target)) close(false); }
      setTimeout(function () { document.addEventListener('pointerdown', off, true); }, 0);
      p.addEventListener('keydown', function (e) {
        var i = rows.indexOf(document.activeElement);
        if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(true); }
        else if (e.key === 'ArrowDown') { e.preventDefault(); rows[(i + 1) % rows.length].focus(); }
        else if (e.key === 'ArrowUp') { e.preventDefault(); rows[(i - 1 + rows.length) % rows.length].focus(); }
        else if (e.key === 'Tab') close(false);
      });
      p.addEventListener('click', function (e) {
        var b = e.target.closest('[data-act]');
        if (!b) { if (e.target.closest('a')) close(false); return; }
        close(true);
        if (b.getAttribute('data-act') === 'keep') keepCategory(c);
        else removeCategory(c);
      });
      rows[0].focus();
      p._close = close;
      return p;
    }

    /* ---------- The one icon picker ---------- */
    function iconPicker(anchor, current, pick) {
      var p = document.createElement('div');
      p.className = 'czs-pop'; p._czsCat = true;
      p.setAttribute('role', 'dialog'); p.setAttribute('aria-label', 'Pick an icon');
      p.innerHTML = '<input class="input" placeholder="Search icons" aria-label="Search icons" autocomplete="off"><div class="czs-icons" role="listbox" aria-label="Icons"></div>';
      document.body.appendChild(p);
      var r = anchor.getBoundingClientRect();
      p.style.left = Math.max(12, Math.min(r.left, innerWidth - p.offsetWidth - 12)) + 'px';
      p.style.top = (r.bottom + 6 + p.offsetHeight > innerHeight ? Math.max(12, r.top - p.offsetHeight - 6) : r.bottom + 6) + 'px';
      p.style.transformOrigin = 'top left';
      var grid = $('.czs-icons', p), q = $('input', p);
      function drawIcons() {
        var w = q.value.trim().toLowerCase();
        var list = ICONS.filter(function (i) { return !w || (i[0].replace('fa-', '').replace(/-/g, ' ') + ' ' + i[1]).indexOf(w) >= 0; });
        grid.innerHTML = list.length ? list.map(function (i) {
          var nm = i[0].replace('fa-', '').replace(/-/g, ' ');
          return '<button type="button" role="option" data-ic="' + i[0] + '" aria-pressed="' + (i[0] === current) + '" aria-label="' + nm + '" title="' + nm + '"><i class="fa-solid ' + i[0] + '" aria-hidden="true"></i></button>';
        }).join('') : '<p class="czs-icons-empty">No icon matches. Try “person”, “place” or “magic”.</p>';
      }
      function close(back) { p.remove(); document.removeEventListener('pointerdown', off, true); if (back && anchor.focus) anchor.focus(); }
      function off(e) { if (!p.contains(e.target) && e.target !== anchor) close(false); }
      setTimeout(function () { document.addEventListener('pointerdown', off, true); }, 0);
      q.addEventListener('input', drawIcons);
      grid.addEventListener('click', function (e) { var b = e.target.closest('[data-ic]'); if (b) { close(false); pick(b.getAttribute('data-ic')); } });
      p.addEventListener('keydown', function (e) { if (e.key === 'Escape') { e.stopPropagation(); close(true); } });
      drawIcons();
      q.focus();
    }

    /* ---------- New category ---------- */
    function newCategoryDialog() {
      var tops = model().filter(function (c) { return !c.isNew && !c.deleted && !c.parent; });
      var st = { from: 0, plural: '', one: '', oneTouched: false, icon: 'fa-dragon', color: COLORS[4] };
      var sc = document.createElement('div');
      sc.className = 'czs-scrim'; sc._czsCat = true;
      var opener = document.activeElement;
      function drawDlg() {
        sc.innerHTML = '<div class="czs-dlg" role="dialog" aria-modal="true" aria-labelledby="czc-nt"><h2 id="czc-nt">New category</h2>' +
          '<div class="czs-field"><span class="czs-lab" id="czc-sf">Start from</span><div class="czs-starts" role="group" aria-labelledby="czc-sf">' +
          '<button type="button" data-from="0" aria-pressed="' + (st.from === 0) + '"><span class="czs-cat-ic sm" style="background:#64748b"><i class="fa-solid fa-file" aria-hidden="true"></i></span><span><span class="czs-nm">Blank</span><span class="czs-ds">Name and text only</span></span></button>' +
          tops.map(function (c) {
            return '<button type="button" data-from="' + Number(c.id) + '" aria-pressed="' + (st.from === c.id) + '">' + chip(c, 'sm') + '<span><span class="czs-nm">Like ' + esc(c.plural) + '</span><span class="czs-ds">Its page layout and fields</span></span></button>';
          }).join('') + '</div><p class="czs-hint">You can change everything afterwards.</p></div>' +
          '<div class="czs-split"><div class="czs-field"><label for="czc-nn">Name</label><input class="input" id="czc-nn" maxlength="100" placeholder="Monsters" value="' + esc(st.plural) + '"></div>' +
          '<div class="czs-field"><label for="czc-no">One of them is called</label><input class="input" id="czc-no" maxlength="100" placeholder="Monster" value="' + esc(st.one) + '"></div></div>' +
          '<div class="czs-field"><span class="czs-lab" id="czc-nc">Icon and colour</span><div><button type="button" class="btn-secondary btn-sm" id="czc-ni" aria-haspopup="dialog">' + chip(st, 'sm') + '<span class="ml-2">Change icon</span></button></div>' +
          '<div class="czs-colors" role="group" aria-labelledby="czc-nc">' + COLORS.map(function (col) {
            return '<button type="button" data-col="' + col + '" aria-label="Colour ' + col + '" aria-pressed="' + (col === st.color) + '" style="background:' + col + '"></button>';
          }).join('') + '</div></div>' +
          '<div class="czs-acts"><button type="button" class="btn-secondary" data-x>Cancel</button><button type="button" class="btn-primary" data-ok' + (st.plural.trim() ? '' : ' disabled') + '>Create category</button></div></div>';
        $$('[data-from]', sc).forEach(function (b) { b.addEventListener('click', function () { st.from = Number(b.getAttribute('data-from')); var src = savedById(String(st.from)); if (src) { st.icon = src.icon; st.color = src.color; } drawDlg(); focus('[data-from="' + st.from + '"]'); }); });
        $$('[data-col]', sc).forEach(function (b) { b.addEventListener('click', function () { st.color = b.getAttribute('data-col'); drawDlg(); focus('[data-col="' + st.color + '"]'); }); });
        var nn = $('#czc-nn', sc), no = $('#czc-no', sc);
        nn.addEventListener('input', function () {
          st.plural = nn.value;
          if (!st.oneTouched) { st.one = singular(nn.value.trim()); no.value = st.one; }
          $('[data-ok]', sc).disabled = !st.plural.trim();
        });
        no.addEventListener('input', function () { st.one = no.value; st.oneTouched = true; });
        $('#czc-ni', sc).addEventListener('click', function (e) { iconPicker(e.currentTarget, st.icon, function (v) { st.icon = v; drawDlg(); focus('#czc-ni'); }); });
        $('[data-x]', sc).addEventListener('click', close);
        $('[data-ok]', sc).addEventListener('click', create);
        nn.addEventListener('keydown', function (e) { if (e.key === 'Enter' && st.plural.trim()) create(); });
      }
      function focus(sel) { var x = $(sel, sc); if (x) x.focus(); }
      function close() { sc.remove(); if (opener && opener.focus && document.body.contains(opener)) opener.focus(); }
      function create() {
        var plural = st.plural.trim();
        if (!plural) return;
        var key = newKey(plural), its = items();
        its[key] = { new: true, name: (st.one.trim() || singular(plural)), name_plural: plural, icon: st.icon, color: st.color };
        if (st.from) its[key].copy_from = st.from;
        commit(its);
        sc.remove();
        // The new category's page reads the draft, so it opens once the
        // draft is kept.
        shellFlush().then(function () { if (alive) location.href = BASE + '/categories/' + key + '/settings'; });
      }
      sc.addEventListener('pointerdown', function (e) { if (e.target === sc) close(); });
      sc.addEventListener('keydown', function (e) { if (e.key === 'Escape' && !document.querySelector('.czs-pop')) close(); });
      document.body.appendChild(sc);
      if (window.Chronicle && Chronicle.modal) Chronicle.modal(sc, { opener: null, escape: false, focus: false, aria: false, allow: '.czs-pop' });
      drawDlg();
      focus('#czc-nn');
    }
    on(el, 'click', function (e) {
      var b = e.target.closest('[data-new]');
      if (b) { e.preventDefault(); newCategoryDialog(); return; }
      var m = e.target.closest('[data-more]');
      if (m) {
        e.preventDefault();
        var open = document.querySelector('.czs-menu');
        var wasOpen = m.getAttribute('aria-expanded') === 'true';
        if (open && open._close) open._close(false);
        if (wasOpen) return;
        var c = find(m.getAttribute('data-more'));
        if (c) moreMenu(m, c);
      }
    });
    if (!KEY && location.hash === '#new') setTimeout(newCategoryDialog, 0);

    function draw() { if (KEY) drawWorkspace(); else drawGrid(); }

    // The page layout's way into this section; see the header.
    var categoryWrite = function (key, patch, opts) {
      if (!alive) return false;
      write(key, patch, opts);
      draw();
      return true;
    };
    Chronicle.customizeCategoryWrite = categoryWrite;

    function onDraft(e) {
      var d = e.detail || {};
      if (d.reason === 'publish' && (d.applied || []).indexOf(SECTION) >= 0) {
        // What is saved changed under this page; read it again.
        var to = /^new-/.test(KEY) ? BASE + '/categories' : location.pathname;
        if (window.htmx && to === location.pathname) {
          window.htmx.ajax('GET', to, { target: '#main-content', select: '#main-content > *', swap: 'innerHTML' });
        } else {
          location.href = to;
        }
        return;
      }
      draw();
    }
    on(document, 'chronicle:customize-draft', onDraft);

    draw();
    return function destroy() {
      alive = false;
      if (Chronicle.customizeCategoryWrite === categoryWrite) Chronicle.customizeCategoryWrite = null;
      listeners.forEach(function (l) { l[0].removeEventListener(l[1], l[2], l[3]); });
      $$('.czs-pop, .czs-scrim', document).forEach(function (p) { if (p._czsCat) p.remove(); });
    };
  }

  Chronicle.register('customize-categories', {
    init: function (el) { el._czcDestroy = mount(el); },
    destroy: function (el) {
      if (el._czcDestroy) el._czcDestroy();
      el._czcDestroy = null;
    }
  });
})();
