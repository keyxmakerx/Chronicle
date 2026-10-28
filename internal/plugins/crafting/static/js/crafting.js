/**
 * crafting.js -- the Crafting app (data-widget="crafting")
 *
 * A Director gets the workshop: Crafting recipes (crafting_recipes.js),
 * Materials (crafting_materials.js), Benches (crafting_benches.js), every
 * hero's Projects (crafting_projects.js), the party's Spoils and every drop
 * list (crafting_spoils.js) and the campaign's settings in a drawer. A player gets their recipe book
 * (crafting_book.js). This file is the glue: it reads the API, owns the tabs,
 * the save status, the bottom message and the confirm dialog, and hands each
 * part the app. Everything is drawn from what the server sends, so a player
 * never receives a draft, a secret rule or a recipe they haven't learned.
 * The plugin's .ai.md is the guide.
 *
 * Attributes: data-campaign-id, data-director ("true" for the workshop),
 * data-calendar-id (the default calendar the rules editor reads, may be "").
 */
(function () {
  'use strict';
  var L = window.CraftingLib;
  var esc = L.esc;

  function fa(n) { return '<i class="fa-solid fa-' + n + '" aria-hidden="true"></i>'; }

  function mount(el, cfg) {
    var cid = String(cfg.campaignId || ''), base = '/campaigns/' + encodeURIComponent(cid) + '/crafting';
    var app = {cid: cid, base: base, director: cfg.director === true || cfg.director === 'true', calendarId: cfg.calendarId ? String(cfg.calendarId) : '', el: el,
      lib: null, items: {}, itemList: [], characters: [], truncated: false, recipes: [], benches: [], parts: {}};
    // The end of a fight links to #spoils, so the tray opens first.
    var dead = false, toastT = 0, tab = window.location.hash === '#spoils' ? 'spoils' : 'recipes';

    /* JSON in and out; a refusal rejects with the server's own words. keep
       lets a save sent as the page closes outlive it. */
    app.api = function (method, path, body, keep) {
      return Chronicle.apiFetch(base + path, {method: method, body: body, keepalive: !!keep}).then(function (res) {
        if (res.status === 204) return null;
        return res.json().catch(function () { return null; }).then(function (data) {
          if (!res.ok) {
            var err = new Error((data && (data.message || data.error)) || 'Something went wrong. Try again.');
            err.status = res.status;
            throw err;
          }
          return data;
        });
      });
    };
    /* The save status in the bar: Saving…, All changes saved, or an error
       that stays until the next save, with a way to fix it. */
    app.status = function (state, msg, fix) {
      var s = el.querySelector('.cra-saved');
      if (!s) return;
      s.classList.toggle('is-bad', state === 'error');
      s.innerHTML = state === 'saving' ? 'Saving…' : state === 'error' ? fa('triangle-exclamation') + '<span>' + esc(msg) + '</span>' +
        (fix ? ' <button type="button" class="cra-link" data-fix>' + esc(fix.label) + '</button>' : '') : fa('check') + '<span>All changes saved</span>';
      var b = s.querySelector('[data-fix]');
      if (b && fix) b.addEventListener('click', fix.fn);
    };
    app.reloadFix = {label: 'Reload', fn: function () { window.location.reload(); }};
    /* A message at the bottom for three seconds, with Undo when it can be undone. */
    app.toast = function (msg, undo) {
      var t = el.querySelector('.cra-toast');
      if (!t) return;
      t.innerHTML = '<span>' + esc(msg) + '</span>' + (undo ? '<button type="button" class="cra-link">Undo</button>' : '');
      t.hidden = false;
      if (undo) t.querySelector('button').addEventListener('click', function () { t.hidden = true; undo(); });
      clearTimeout(toastT);
      toastT = setTimeout(function () { t.hidden = true; }, undo ? 5000 : 3000);
    };
    /* Deleting asks first, naming exactly what goes. */
    app.confirm = function (title, body, ok) {
      return new Promise(function (resolve) {
        var d = document.createElement('dialog');
        d.className = 'cra-dialog';
        d.innerHTML = '<form method="dialog"><h3>' + esc(title) + '</h3><p>' + esc(body) + '</p><div class="cra-row cra-end">' +
          '<button type="submit" value="no" class="btn btn-secondary btn-sm">Keep it</button><button type="submit" value="yes" class="btn btn-danger btn-sm">' + esc(ok) + '</button></div></form>';
        (el.querySelector('.cra') || el).appendChild(d);
        d.addEventListener('close', function () { resolve(d.returnValue === 'yes'); d.remove(); });
        if (d.showModal) d.showModal(); else resolve(window.confirm(title + '\n\n' + body));
      });
    };
    app.setItems = function (list) {
      app.itemList = list.slice().sort(function (a, b) { return a.name.localeCompare(b.name); });
      app.items = {};
      app.itemList.forEach(function (it) { app.items[it.id] = it; });
    };
    app.show = function (k, then) { if (then) then(app.parts[k]); showTab(k); };

    function showTab(k) {
      tab = k;
      Array.prototype.forEach.call(el.querySelectorAll('.cra-tabs [role="tab"]'), function (b) {
        var on = b.getAttribute('data-tab') === k; b.setAttribute('aria-selected', String(on)); b.tabIndex = on ? 0 : -1;
      });
      Array.prototype.forEach.call(el.querySelectorAll('.cra-pan'), function (p) { p.hidden = p.getAttribute('data-pane') !== k; });
      var part = app.parts[k];
      if (part && part.shown) part.shown();
    }

    function drawer() {
      var S = app.lib.settings, R = app.lib.rules;
      function seg(key, list) {
        return '<div class="cra-seg" role="group">' + list.map(function (c) {
          return '<button type="button" data-set="' + key + '" data-v="' + esc(c.key) + '" aria-pressed="' + (S[key] === c.key) + '">' + esc(c.label) + '</button>';
        }).join('') + '</div>';
      }
      function sw(key, label) { return '<label class="cra-sw"><input type="checkbox" data-set="' + key + '"' + (S[key] ? ' checked' : '') + '><span>' + esc(label) + '</span></label>'; }
      var stats = (R.characteristics || []).map(function (c) { return c.label; }).join(', ');
      return '<div class="cra-dhead"><h3>Crafting settings</h3><button type="button" class="cra-ib" data-close aria-label="Close">' + fa('xmark') + '</button></div>' +
        '<div class="cra-dbody"><div class="cra-dsec"><h4>The bench</h4>' +
        '<div class="cra-ctl"><span>Players start at</span>' + seg('skin', app.lib.skins) + '<em>Players can switch between the desk and the journal.</em></div>' +
        '<div class="cra-ctl"><span>Timing</span>' + seg('difficulty', app.lib.difficulties) + '<em>How tight the gold bands are on the graded steps.</em></div>' +
        sw('bane', 'Botched work gives a bane') + sw('skip', 'Players may skip the steps and just roll') + '</div>' +
        '<div class="cra-dsec"><h4>The project roll</h4><p class="cra-fine">' + esc(R.dice) + (stats ? ' plus one of ' + esc(stats) : '') +
        '. Clean work adds ' + R.edge + ', botched work ' + R.bane + '. ' + (app.lib.system ? 'From the ' + esc(app.lib.system) + ' package; it can’t be changed here.' : 'No game system is enabled, so these are Chronicle’s defaults.') + '</p></div></div>';
    }
    function saveSettings(next) {
      var was = app.lib.settings;
      app.lib.settings = next;
      app.status('saving');
      app.api('PUT', '/settings', next).then(function (s) { app.lib.settings = s; app.status('saved'); }, function (err) {
        app.lib.settings = was; app.status('error', err.message);
        el.querySelector('.cra-drawer').innerHTML = drawer();
      });
    }
    function wireDrawer() {
      var d = el.querySelector('.cra-drawer'), btn = el.querySelector('[data-settings]');
      function open(v) {
        d.classList.toggle('is-open', v); d.setAttribute('aria-hidden', String(!v)); btn.setAttribute('aria-expanded', String(v));
        if (v) { d.innerHTML = drawer(); var f = d.querySelector('button'); if (f) f.focus(); } else btn.focus();
      }
      btn.addEventListener('click', function () { open(!d.classList.contains('is-open')); });
      d.addEventListener('click', function (e) {
        if (e.target.closest('[data-close]')) { open(false); return; }
        var b = e.target.closest('button[data-set]');
        if (!b) return;
        var next = L.clone(app.lib.settings); next[b.getAttribute('data-set')] = b.getAttribute('data-v');
        Array.prototype.forEach.call(b.parentNode.children, function (x) { x.setAttribute('aria-pressed', String(x === b)); });
        saveSettings(next);
      });
      d.addEventListener('change', function (e) {
        var t = e.target; if (!t.matches('input[data-set]')) return;
        var next = L.clone(app.lib.settings); next[t.getAttribute('data-set')] = t.checked; saveSettings(next);
      });
      d.addEventListener('keydown', function (e) { if (e.key === 'Escape') open(false); });
    }

    function workshop() {
      el.innerHTML = '<div class="cra">' +
        '<div class="cra-bar"><div class="cra-tabs" role="tablist" aria-label="Crafting">' +
        '<button type="button" role="tab" data-tab="recipes" aria-selected="true">Crafting recipes<small>what is made</small></button>' +
        '<button type="button" role="tab" data-tab="materials" aria-selected="false" tabindex="-1">Materials<small>what they take</small></button>' +
        '<button type="button" role="tab" data-tab="benches" aria-selected="false" tabindex="-1">Benches<small>where it is made</small></button>' +
        '<button type="button" role="tab" data-tab="projects" aria-selected="false" tabindex="-1">Projects<small>what is being made</small></button>' +
        '<button type="button" role="tab" data-tab="spoils" aria-selected="false" tabindex="-1">Spoils<small>what was left</small></button></div>' +
        '<div class="cra-tools"><span class="cra-saved" role="status">' + fa('check') + '<span>All changes saved</span></span>' +
        '<button type="button" class="cra-tbtn cra-dm" data-settings aria-expanded="false">' + fa('sliders') + 'Settings</button></div></div>' +
        '<div class="cra-body"><section class="cra-pan" data-pane="recipes" role="tabpanel"></section><section class="cra-pan" data-pane="materials" role="tabpanel" hidden></section>' +
        '<section class="cra-pan" data-pane="benches" role="tabpanel" hidden></section><section class="cra-pan" data-pane="projects" role="tabpanel" hidden></section>' +
        '<section class="cra-pan" data-pane="spoils" role="tabpanel" hidden></section>' +
        '<aside class="cra-drawer" aria-hidden="true" aria-label="Crafting settings"></aside></div>' +
        (app.truncated ? '<p class="cra-note">This campaign has more pages than one list holds, so some items or characters are missing from the pickers.</p>' : '') +
        '<div class="cra-toast" role="status" hidden></div></div>';
      app.parts.recipes = CraftingRecipes.mount(el.querySelector('[data-pane="recipes"]'), app);
      app.parts.materials = CraftingMaterials.mount(el.querySelector('[data-pane="materials"]'), app);
      app.parts.benches = CraftingBenches.mount(el.querySelector('[data-pane="benches"]'), app);
      app.parts.projects = CraftingProjects.mount(el.querySelector('[data-pane="projects"]'), app);
      app.parts.spoils = CraftingSpoils.mount(el.querySelector('[data-pane="spoils"]'), app);
      el.querySelector('.cra-tabs').addEventListener('click', function (e) { var b = e.target.closest('[role="tab"]'); if (b) showTab(b.getAttribute('data-tab')); });
      el.querySelector('.cra-tabs').addEventListener('keydown', function (e) {
        var ts = Array.prototype.slice.call(el.querySelectorAll('.cra-tabs [role="tab"]')), i = ts.indexOf(e.target);
        var n = e.key === 'ArrowRight' ? i + 1 : e.key === 'ArrowLeft' ? i - 1 : null;
        if (i < 0 || n == null) return;
        e.preventDefault(); n = (n + ts.length) % ts.length; ts[n].focus(); showTab(ts[n].getAttribute('data-tab'));
      });
      wireDrawer();
    }
    // Already here when the link is followed: only the hash changes.
    function onHash() { if (window.location.hash === '#spoils' && app.parts.spoils) showTab('spoils'); }
    window.addEventListener('hashchange', onHash);

    function fail(err) {
      if (dead) return;
      el.innerHTML = '<div class="cra-empty cra-bad" role="alert">' + fa('triangle-exclamation') + ' Crafting couldn’t load: ' + esc(err.message || 'something went wrong') +
        '. <button type="button" class="cra-link" data-retry>Try again</button></div>';
      el.querySelector('[data-retry]').addEventListener('click', load);
    }
    function load() {
      el.innerHTML = '<div class="cra-loading" aria-hidden="true"><i></i><i></i><i></i></div><p class="sr-only" role="status">Loading crafting…</p>';
      var reads = [app.api('GET', '/library')];
      if (app.director) reads.push(app.api('GET', '/catalog'), app.api('GET', '/recipes'), app.api('GET', '/benches'));
      else reads.push(app.api('GET', '/book'));
      Promise.all(reads).then(function (got) {
        if (dead) return;
        app.lib = got[0];
        if (!app.director || !app.lib.director) { CraftingBook.mount(el, app, got[1] || []); return; }
        app.setItems(got[1].items || []);
        app.characters = got[1].characters || [];
        app.truncated = !!got[1].truncated;
        app.recipes = got[2] || [];
        app.benches = got[3] || [];
        workshop();
        showTab(tab);
      }, fail);
    }
    load();
    return {app: app, destroy: function () {
      dead = true; clearTimeout(toastT);
      window.removeEventListener('hashchange', onHash);
      Object.keys(app.parts).forEach(function (k) { if (app.parts[k] && app.parts[k].destroy) app.parts[k].destroy(); });
      el.innerHTML = '';
    }};
  }

  Chronicle.register('crafting', {
    init: function (el, cfg) {
      if (!window.CraftingLib || !window.CraftingRecipes || !window.CraftingMaterials || !window.CraftingBenches || !window.CraftingBook || !window.CraftingProjects || !window.CraftingSpoils) {
        el.innerHTML = '<p class="text-sm text-fg-muted" role="status">Crafting couldn’t load. Reload the page to try again.</p>';
        return;
      }
      el._cra = mount(el, cfg || {});
    },
    destroy: function (el) {
      if (el._cra) { el._cra.destroy(); el._cra = null; }
    }
  });
})();
