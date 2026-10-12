/**
 * customize_categories.js: Customize's Categories area.
 *
 * Draws the grid of categories, and a category's header, tabs and Settings,
 * from data-state (the saved categories) with the draft's "categories"
 * section laid over it. Every change goes into that section through
 * Chronicle.customizeSet (customize_shell.js); nothing is written until the
 * owner presses Save. Section shape, applied by the entities plugin:
 *
 *   { items: { "<id>" | "new-<slug>": { new, name, name_plural, icon, color,
 *       claimable, parent, clear_parent, copy_from, menu, delete } },
 *     _changes: [{ label, href }] }
 *
 * An existing category's item holds only what differs from what is saved,
 * so a change put back by hand leaves the draft.
 */
(function () {
  'use strict';

  var SECTION = 'categories';
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
  var TABS = [['layout', 'Page layout', 'fa-table-columns'], ['home', 'Category home', 'fa-house'], ['fields', 'Fields', 'fa-list-ul'], ['text', 'Starting text', 'fa-align-left'], ['settings', 'Settings', 'fa-sliders']];

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
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
    var ADVENTURES_URL = state.adventuresUrl || '';
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
    function shellSet(v) {
      if (Chronicle.customizeSet) Chronicle.customizeSet(SECTION, v);
      else (Chronicle._customizeQueue = Chronicle._customizeQueue || []).push([SECTION, v]);
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
          deleted: !!it.delete, changed: Object.keys(it).length > 0
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
    function write(key, patch) {
      var its = items(), it = its[key] || {};
      Object.keys(patch).forEach(function (k) { if (patch[k] === undefined) delete it[k]; else it[k] = patch[k]; });
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
      commit(its);
    }
    function commit(its) {
      if (!Object.keys(its).length) { shellSet(null); return; }
      shellSet({ items: its, _changes: changes(its) });
    }
    // The save bar's lines for this section.
    function changes(its) {
      var out = [], byKey = {};
      model().forEach(function (c) { byKey[c.key] = c; });
      Object.keys(its).forEach(function (k) {
        var it = its[k], s = savedById(k), nm = it.name_plural || (s && s.plural) || 'Category';
        if (it.new) out.push({ label: 'New category ' + nm, href: BASE + '/categories/' + k + '/settings' });
        else if (it.delete) out.push({ label: 'Delete ' + (s ? s.plural : nm), href: BASE + '/categories' });
        else out.push({ label: nm + ' settings', href: BASE + '/categories/' + k + '/settings' });
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
        '<button type="button" class="btn-primary" data-new><i class="fa-solid fa-plus mr-1.5" aria-hidden="true"></i>New category</button></div>' +
        '<div class="czs-cat-grid" hx-boost="true" hx-target="#main-content" hx-select="#main-content" hx-swap="innerHTML show:window:top">' +
        top.map(card).join('') +
        '<button type="button" class="czs-cat-card czs-add" data-new><span class="czs-plus"><i class="fa-solid fa-plus" aria-hidden="true"></i></span>New category</button></div>' +
        (hasAdventures() || !ADVENTURES_URL ? '' : adventuresPanel());
      if (window.htmx) window.htmx.process(el);
    }
    function hasAdventures() { return saved.some(function (c) { return c.dmOnly; }); }
    // The one-click offer for a campaign made before Adventures existed. It
    // acts at once, not through the draft: adding the category is idempotent
    // and a draft that holds other edits is kept across the reload.
    function adventuresPanel() {
      return '<div class="czs-panel czs-row-set czs-adv-offer"><div><span class="czs-lab"><i class="fa-solid fa-dungeon" aria-hidden="true"></i> Adventures</span>' +
        '<p class="czs-hint">A category for your prep that only the DM team sees: the owner, co-DMs and scribes. Players get no menu entry, search result or link to it. It starts with a page layout for running an adventure at the table.</p>' +
        '<p class="czs-err" data-adv-err role="alert" hidden></p></div>' +
        '<button type="button" class="btn-secondary" data-add-adv><i class="fa-solid fa-plus mr-1.5" aria-hidden="true"></i>Add Adventures</button></div>';
    }
    function addAdventures(btn) {
      var err = $('[data-adv-err]');
      btn.disabled = true;
      if (err) err.hidden = true;
      Chronicle.apiFetch(ADVENTURES_URL, { method: 'POST' }).then(function (r) {
        if (!r.ok) throw new Error('refused');
        return shellFlush();
      }).then(function () {
        if (alive) location.reload();
      }).catch(function () {
        if (!alive) return;
        btn.disabled = false;
        if (err) { err.textContent = 'Adventures could not be added. Try again in a moment.'; err.hidden = false; }
      });
    }
    function card(c) {
      var kids = kidsOf(c.key), href = BASE + '/categories/' + c.key + '/' + (c.isNew ? 'settings' : 'layout');
      var pills = [];
      if (c.isNew) pills.push('<span class="czs-pill is-new">New</span>');
      if (c.deleted) pills.push('<span class="czs-pill is-warn">Deleted when you save</span>');
      else if (c.changed && !c.isNew) pills.push('<span class="czs-pill is-new">Changed</span>');
      if (c.dmOnly) pills.push('<span class="czs-pill is-dm"><i class="fa-solid fa-lock" aria-hidden="true"></i> Only the DM team sees it</span>');
      else pills.push('<span class="czs-pill">' + (c.menu ? 'In the menu' : 'Hidden from players’ menu') + '</span>');
      if (claiming && c.claimable && !c.dmOnly) pills.push('<span class="czs-pill">Players can claim</span>');
      return '<a class="czs-cat-card' + (c.deleted ? ' is-gone' : '') + '" href="' + esc(href) + '">' +
        '<span class="czs-meta">' + chip(c) + '<span style="min-width:0"><span class="czs-nm">' + esc(c.plural || c.name) + '</span>' +
        '<span class="czs-sub">' + (c.isNew ? 'Made in your draft' : pagesText(c.pages)) + '</span></span></span>' +
        (kids.length ? '<span class="czs-kids">Kinds: ' + esc(kids.map(function (k) { return k.plural || k.name; }).join(', ')) + '</span>' : '') +
        '<span class="czs-foot">' + pills.join('') + '</span></a>';
    }

    /* ---------- A category's workspace ---------- */
    function drawWorkspace() {
      var c = find(KEY), head = $('[data-czs-ws-head]'), tabs = $('[data-czs-ws-tabs]'), body = $('[data-czs-ws-settings]');
      if (!c) {
        head.innerHTML = '<a class="czs-icon-btn" href="' + esc(BASE + '/categories') + '" aria-label="All categories"><i class="fa-solid fa-arrow-left" aria-hidden="true"></i></a>' +
          '<div><h1 class="czs-h1">Not in your draft</h1><p class="czs-lede">This category was made in a draft that has since been saved or thrown away.</p></div>';
        tabs.innerHTML = '';
        if (body) body.innerHTML = '<a class="btn-secondary" href="' + esc(BASE + '/categories') + '">All categories</a>';
        return;
      }
      var parent = c.parent ? find(c.parent) : null;
      head.innerHTML = '<a class="czs-icon-btn" href="' + esc(BASE + '/categories') + '" aria-label="All categories" title="All categories"><i class="fa-solid fa-arrow-left" aria-hidden="true"></i></a>' +
        '<a class="czs-ident" href="' + esc(BASE + '/categories/' + c.key + '/settings') + '" title="Change name, icon and colour">' + chip(c, 'lg') +
        '<span><h1>' + esc(c.plural || c.name || 'New category') + '</h1><span class="czs-sub">' +
        (c.isNew ? 'New, saved with your other changes' : pagesText(c.pages)) +
        (parent ? ' · a kind of ' + esc(parent.plural || parent.name) : '') +
        ' · ' + (c.dmOnly ? 'only the DM team sees it' : c.menu ? 'in the menu' : 'hidden from players’ menu') +
        (c.deleted ? ' · deleted when you save' : '') + '</span></span></a>';
      tabs.setAttribute('role', 'tablist');
      tabs.innerHTML = TABS.map(function (t) {
        var lbl = '<i class="fa-solid ' + t[2] + '" aria-hidden="true"></i> ' + t[1];
        if (c.isNew && t[0] !== 'settings') return '<span title="Available once you save this category">' + lbl + '</span>';
        var dot = t[0] === 'settings' && c.changed ? '<span class="czs-dot" title="Unsaved changes"></span>' : '';
        return '<a role="tab" href="' + esc(BASE + '/categories/' + c.key + '/' + t[0]) + '"' + (t[0] === TAB ? ' aria-current="page" aria-selected="true"' : ' aria-selected="false"') + '>' + lbl + dot + '</a>';
      }).join('');
      if (body && TAB === 'settings') drawSettings(c, body);
    }

    function drawSettings(c, body) {
      var focusId = document.activeElement && body.contains(document.activeElement) ? document.activeElement.id : '';
      var kids = kidsOf(c.key), parent = c.parent ? find(c.parent) : null;
      var blocked = c.isNew ? '' : (c.pages > 0 ? 'Move or delete its ' + pagesText(c.pages) + ' first.' :
        c.trashed > 0 ? pagesText(c.trashed) + ' of it ' + (c.trashed === 1 ? 'is' : 'are') + ' in the Trash. Restore or empty ' + (c.trashed === 1 ? 'it' : 'them') + ' first.' :
        kids.length ? 'Delete its kinds first.' : '');
      body.innerHTML = '<div class="czs-form">' +
        '<div class="czs-split"><div class="czs-field"><label for="czc-plural">Name</label><input class="input" id="czc-plural" maxlength="100" value="' + esc(c.plural) + '"><p class="czs-err" id="czc-plural-e" hidden>A category needs a name.</p></div>' +
        '<div class="czs-field"><label for="czc-one">One of them is called</label><input class="input" id="czc-one" maxlength="100" value="' + esc(c.name) + '"><p class="czs-hint">Used in “New ' + esc((c.name || 'page').toLowerCase()) + '”.</p><p class="czs-err" id="czc-one-e" hidden>Give one of them a name too.</p></div></div>' +
        '<div class="czs-field"><span class="czs-lab" id="czc-ic-l">Icon and colour</span><div><button type="button" class="btn-secondary btn-sm" id="czc-icon" aria-haspopup="dialog">' + chip(c, 'sm') + '<span class="ml-2">Change icon</span></button></div>' +
        '<div class="czs-colors" role="group" aria-labelledby="czc-ic-l">' + COLORS.map(function (col) {
          return '<button type="button" data-col="' + col + '" aria-label="Colour ' + col + '" aria-pressed="' + (col.toLowerCase() === String(c.color).toLowerCase()) + '" style="background:' + col + '"></button>';
        }).join('') + '<input type="color" id="czc-color" value="' + esc(/^#[0-9a-fA-F]{6}$/.test(c.color) ? c.color : '#6366f1') + '" aria-label="Your own colour"></div></div>' +
        (c.dmOnly ? '<div class="czs-panel"><span class="czs-lab"><i class="fa-solid fa-lock" aria-hidden="true"></i> Only the DM team sees it</span><p class="czs-hint">Players get no menu entry, search result, link preview or relation for these pages, and cannot claim them. The owner, co-DMs and scribes see them as usual. Because of that it has no kinds of its own.</p></div>' : '') +
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
              (!k.isNew && (k.pages > 0 || k.trashed > 0 || kidsOf(k.key).length) ? ' disabled title="It has pages; move or delete them first"' : ' title="Remove"') + '><i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>';
          }).join('') : '<p class="czs-hint">No kinds yet.</p>') + '</div>' +
          '<div class="czs-act-row"><input class="input" id="czc-kind" maxlength="100" placeholder="Add a kind, like “Dungeons”" style="flex:1;min-width:0;width:auto"><button type="button" class="btn-secondary" id="czc-kind-add"><i class="fa-solid fa-plus mr-1.5" aria-hidden="true"></i>Add</button></div></div>') +
        (c.deleted ? '<div class="czs-danger"><div><span class="czs-lab">Deleted when you save</span><p class="czs-hint">Nothing is gone yet.</p></div><button type="button" class="btn-secondary" id="czc-keep">Keep it</button></div>' :
          '<div class="czs-danger"><div><span class="czs-lab">Delete ' + esc(c.plural || 'this category') + '</span><p class="czs-hint">' + (c.isNew ? 'It is only in your draft, so nothing else changes.' : (blocked || 'It has no pages, so nothing else is lost.')) + '</p></div>' +
          '<button type="button" class="btn-danger" id="czc-delete"' + (blocked ? ' disabled' : '') + '><i class="fa-solid fa-trash-can mr-1.5" aria-hidden="true"></i>Delete category</button></div>') +
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
      if (keep) keep.addEventListener('click', function () { write(c.key, { delete: undefined }); drawWorkspace(); });
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
    // when it is the category itself rather than one of its kinds.
    function removeCategory(c, leave) {
      if (!c) return;
      if (c.isNew) {
        var its = items();
        Object.keys(its).forEach(function (k) { if (k === c.key || its[k].parent === c.key) delete its[k]; });
        commit(its);
        if (leave) shellFlush().then(function () { if (alive) location.href = BASE + '/categories'; });
        else drawWorkspace();
        return;
      }
      confirmDelete(c, function () {
        write(c.key, { delete: true });
        if (leave) shellFlush().then(function () { if (alive) location.href = BASE + '/categories'; });
        else drawWorkspace();
      });
    }
    function confirmDelete(c, ok) {
      var sc = document.createElement('div');
      sc.className = 'czs-scrim'; sc._czsCat = true;
      var nm = esc(c.plural || c.name);
      sc.innerHTML = '<div class="czs-dlg" role="alertdialog" aria-modal="true" aria-labelledby="czc-dt" aria-describedby="czc-dd" style="width:min(460px,100%)">' +
        '<h2 id="czc-dt">Delete ' + nm + '?</h2><p id="czc-dd">It has no pages, so nothing else is lost. It is deleted when you save, and until then Discard or Undo brings it back.</p>' +
        '<div class="czs-acts"><button type="button" class="btn-secondary" data-x>Keep it</button><button type="button" class="btn-danger" data-ok>Delete category</button></div></div>';
      document.body.appendChild(sc);
      var opener = document.activeElement;
      function close() { sc.remove(); if (opener && opener.focus && document.body.contains(opener)) opener.focus(); }
      $('[data-x]', sc).addEventListener('click', close);
      $('[data-ok]', sc).addEventListener('click', function () { sc.remove(); ok(); });
      sc.addEventListener('pointerdown', function (e) { if (e.target === sc) close(); });
      sc.addEventListener('keydown', function (e) { if (e.key === 'Escape') close(); });
      $('[data-x]', sc).focus();
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
      drawDlg();
      focus('#czc-nn');
    }
    on(el, 'click', function (e) {
      var b = e.target.closest('[data-new]');
      if (b) { e.preventDefault(); newCategoryDialog(); return; }
      var adv = e.target.closest('[data-add-adv]');
      if (adv) { e.preventDefault(); addAdventures(adv); }
    });
    if (!KEY && location.hash === '#new') setTimeout(newCategoryDialog, 0);

    function draw() { if (KEY) drawWorkspace(); else drawGrid(); }

    function onDraft(e) {
      var d = e.detail || {};
      if (d.reason === 'publish' && (d.applied || []).indexOf(SECTION) >= 0) {
        // What is saved changed under this page; read it again.
        var to = /^new-/.test(KEY) ? BASE + '/categories' : location.pathname;
        if (window.htmx && to === location.pathname) {
          window.htmx.ajax('GET', to, { target: '#main-content', select: '#main-content', swap: 'innerHTML' });
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
