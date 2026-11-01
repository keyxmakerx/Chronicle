/**
 * crafting_part.js -- the Workshop's Crafting recipes part
 * (data-widget="crafting-recipes-part")
 *
 * A homebrew can hold a set of crafting recipes, like an herbalist's cures,
 * written in one sitting. The Workshop draws this part with crafting's own
 * widget: on the canvas a quick table (type a name and press Enter for a
 * row, Tab across, materials by a few letters plus Enter), and in the
 * inspector who can make them and how new rows start. Every row is an
 * ordinary crafting recipe that the Crafting app and the item's page show
 * too; the part keeps only which recipes it holds. New rows start as
 * drafts unless the Director says otherwise. The generator, the door to
 * the Director's own AI and recipe packs live in crafting_part_more.js.
 *
 * The Workshop puts its part on el._hbPart: {campaignId, part, setData(d),
 * setPrimary(label, fn)}. Attribute data-mode is "canvas" or "inspector".
 * Both mounts of one part share one store, so a change in either redraws
 * both.
 */
(function () {
  'use strict';
  var L = window.CraftingLib, P = window.CraftingPick, esc = L.esc;

  // The Workshop page doesn't know crafting, so the part brings its own
  // styles, from beside this script and with the same version.
  var SELF = (document.currentScript && document.currentScript.src) || '';
  function styles() {
    if (!SELF || document.querySelector('link[data-crm-css]')) return;
    ['crafting.css', 'making.css'].forEach(function (f) {
      var l = document.createElement('link');
      l.rel = 'stylesheet'; l.href = SELF.replace(/js\/crafting_part\.js/, 'css/' + f); l.setAttribute('data-crm-css', '');
      document.head.appendChild(l);
    });
  }

  function fa(n) { return '<i class="fa-solid fa-' + n + '" aria-hidden="true"></i>'; }
  function plural(n, one, many) { return n + ' ' + (n === 1 ? one : (many || one + 's')); }

  var WHO = [['all', 'Anyone'], ['learned', 'Learned'], ['director', 'Director only']];
  var START = [['draft', 'Drafts'], ['ready', 'Ready to use']];

  /* ---------- The store both mounts share ---------- */

  function store(ctx) {
    if (ctx._crp) return ctx._crp;
    var cid = String(ctx.campaignId || ''), base = '/campaigns/' + encodeURIComponent(cid) + '/crafting';
    var st = {ctx: ctx, cid: cid, base: base, app: base, lib: null, items: [], byId: {}, tables: [], recipes: {}, all: [],
      view: 'table', status: '', bad: false, mounts: [], ready: false, failed: '', gen: null, ai: null, packs: null};
    st.api = function (method, path, body, raw) {
      var opts = {method: method, body: body};
      if (raw) opts.headers = {'Content-Type': 'application/json'};
      return Chronicle.apiFetch(base + path, opts).then(function (res) {
        if (res.status === 204) return null;
        return res.json().catch(function () { return null; }).then(function (d) {
          if (!res.ok) { var e = new Error((d && (d.message || d.error)) || 'Something went wrong. Try again.'); e.status = res.status; throw e; }
          return d;
        });
      });
    };
    st.data = function () {
      var d = ctx.part.data || {};
      return {who: d.who || 'all', start: d.start || 'draft', recipes: (d.recipes || []).slice()};
    };
    st.setData = function (d) { ctx.part.data = d; if (ctx.setData) ctx.setData(d); };
    st.rows = function () { return st.data().recipes.map(function (id) { return st.recipes[id]; }).filter(Boolean); };
    st.drafts = function () { return st.rows().filter(function (r) { return r.draft; }); };
    st.say = function (text, bad) { st.status = text || ''; st.bad = !!bad; st.mounts.forEach(function (m) { m.status(); }); };
    st.emit = function () { st.mounts.forEach(function (m) { m.draw(); }); };
    st.keep = function (r) {
      if (!st.recipes[r.id]) st.all.push(r); else st.all = st.all.map(function (x) { return x.id === r.id ? r : x; });
      st.recipes[r.id] = r;
    };
    st.addItem = function (it) {
      if (st.byId[it.id]) return;
      st.byId[it.id] = it; st.items.push(it);
      st.items.sort(function (a, b) { return a.name.localeCompare(b.name); });
    };
    // add puts recipes into the part, after the ones it holds.
    st.add = function (ids) {
      var d = st.data(), n = 0;
      ids.forEach(function (id) { if (d.recipes.indexOf(id) < 0 && d.recipes.length < 200) { d.recipes.push(id); n++; } });
      if (n) st.setData(d);
      return n;
    };
    st.who = function () { var w = st.data().who; return w === 'director' ? 'learned' : w; };
    // newRecipe starts a recipe the way this part says new rows start.
    st.newRecipe = function (r) {
      var body = {name: r.name, template: r.template || '', bench: '', materials: r.materials || [], steps: r.steps || [], result: r.result || {itemId: '', qty: 1},
        goal: r.goal || st.usualGoal(), stat: '', prerequisite: '', source: '', cost: {amount: 0, currency: ''}, who: st.who(), known: [], colors: {}, version: 0,
        draft: r.draft != null ? r.draft : st.data().start !== 'ready'};
      return st.api('POST', '/recipes', body).then(function (got) { st.keep(got); return got; });
    };
    // usualGoal is the middle of the Director's approved goals, so a quick
    // row starts near the rest; the Director sets the real one in Crafting.
    st.usualGoal = function () {
      var g = st.all.filter(function (r) { return !r.draft && r.goal > 0; }).map(function (r) { return r.goal; }).sort(function (a, b) { return a - b; });
      return g.length ? g[Math.floor(g.length / 2)] : 1;
    };
    st.save = function (r) {
      return st.api('PUT', '/recipes/' + encodeURIComponent(r.id), L.input(r)).then(function (got) { st.keep(got); return got; });
    };
    st.approve = function (rows) {
      var done = 0;
      return rows.reduce(function (p, r) {
        return p.then(function () {
          return st.api('POST', '/recipes/' + encodeURIComponent(r.id) + '/approve').then(function (got) { st.keep(got); done++; });
        });
      }, Promise.resolve()).then(function () { return done; });
    };
    st.load = function () {
      st.failed = '';
      return Promise.all([st.api('GET', '/library'), st.api('GET', '/catalog'), st.api('GET', '/recipes')]).then(function (got) {
        st.lib = got[0];
        st.items = ((got[1] && got[1].items) || []).slice().sort(function (a, b) { return a.name.localeCompare(b.name); });
        st.byId = {}; st.items.forEach(function (it) { st.byId[it.id] = it; });
        st.tables = (got[1] && got[1].tables) || [];
        var list = Array.isArray(got[2]) ? got[2] : ((got[2] && got[2].data) || []);
        st.all = list; st.recipes = {}; list.forEach(function (r) { st.recipes[r.id] = r; });
        st.ready = true;
        st.emit();
      }, function (err) { st.failed = err.message; st.emit(); });
    };
    st.setView = function (v) {
      st.view = v; st.say('');
      if (ctx.setPrimary) ctx.setPrimary(null);
      st.emit();
    };
    ctx._crp = st;
    st.load();
    return st;
  }

  /* ---------- The canvas: the table ---------- */

  function stateCell(r) {
    if (r.draft) return '<span class="crm-pill is-draft" title="Players don’t see it until you approve it">Draft</span>';
    return '<span class="crm-pill is-ok">' + (r.who === 'learned' ? 'Learned' : 'Approved') + '</span>';
  }

  function methodCell(st, r) {
    var opts = [['', 'By hand']].concat(st.lib.templates.map(function (t) { return [t.key, t.name]; }));
    return '<select class="crm-sel" data-method="' + esc(r.id) + '" aria-label="Method for ' + esc(r.name) + '">' + opts.map(function (o) {
      return '<option value="' + esc(o[0]) + '"' + (o[0] === (r.template || '') ? ' selected' : '') + '>' + esc(o[1]) + '</option>';
    }).join('') + '</select>';
  }

  function matsCell(st, r) {
    return '<div class="crm-chips">' + r.materials.map(function (m, i) {
      var n = L.matName(st.lib, m, st.byId);
      return '<span class="crm-chip">' + P.pic(L.matPicture(st.lib, m, st.byId), 18) + '<span>' + m.qty + ' ' + esc(n) + '</span>' +
        '<a class="crm-play" href="' + st.app + '#try=' + esc(r.id) + '" title="Try it as a player" aria-label="Try ' + esc(n) + ' as a player">' + fa('play') + '</a>' +
        '<button type="button" class="crm-x" data-rm-mat="' + esc(r.id) + ':' + i + '" aria-label="Remove ' + esc(n) + '">' + fa('xmark') + '</button></span>';
    }).join('') + '<span class="crm-type"><input class="crm-in" data-mat="' + esc(r.id) + '" placeholder="' + (r.materials.length ? 'Add' : 'Like “2 moon”') +
      '" aria-label="Add a material to ' + esc(r.name) + '" autocomplete="off"></span></div>';
  }

  function resultPic(st, r) {
    var it = r.result && r.result.itemId && st.byId[r.result.itemId];
    return P.pic(it ? L.matPicture(st.lib, it) : null, 20);
  }

  function tableHTML(st) {
    var rows = st.rows(), drafts = st.drafts().length;
    var h = '<div class="crm-part"><div class="crm-head"><h3 class="crm-h">Crafting recipes</h3><span class="crm-count">' + plural(rows.length, 'recipe') + '</span>' +
      '<span class="crm-words">' + (drafts ? '<button type="button" class="crm-q" data-act="approve-all">Approve the ' + plural(drafts, 'draft') + '</button>' : '') +
      '<button type="button" class="crm-q" data-view="generate">' + fa('dice') + 'Generate</button>' +
      (st.lib.owner ? '<button type="button" class="crm-q" data-view="ai">' + fa('wand-magic-sparkles') + 'Write with your AI</button>' : '') +
      '<button type="button" class="crm-q" data-view="packs">' + fa('box-archive') + 'Packs</button></span></div>';
    h += '<div class="crm-tablewrap"><table class="crm-table"><thead><tr><th scope="col">Recipe</th><th scope="col">Materials</th><th scope="col">Method</th><th scope="col">State</th>' +
      '<th scope="col"><span class="sr-only">Take out</span></th></tr></thead><tbody>';
    rows.forEach(function (r) {
      h += '<tr data-row="' + esc(r.id) + '"><td class="crm-name">' + resultPic(st, r) +
        '<input class="crm-in" data-name="' + esc(r.id) + '" value="' + esc(r.name) + '" aria-label="Recipe name" maxlength="120"></td>' +
        '<td>' + matsCell(st, r) + '</td><td>' + methodCell(st, r) + '</td><td>' + stateCell(r) + '</td>' +
        '<td><span class="crm-ends"><a class="crm-ib" href="' + st.app + '#recipe=' + esc(r.id) + '" title="Open it in Crafting" aria-label="Open ' + esc(r.name) + ' in Crafting">' + fa('arrow-up-right-from-square') + '</a>' +
        '<button type="button" class="crm-ib" data-out="' + esc(r.id) + '" title="Take it out of this part" aria-label="Take ' + esc(r.name) + ' out of this part">' + fa('xmark') + '</button></span></td></tr>';
    });
    h += '<tr class="crm-newrow"><td colspan="5"><span class="crm-type"><input class="crm-in" data-new placeholder="' + (rows.length ? 'Type a name for the next recipe' : 'Type a recipe’s name and press Enter') +
      '" aria-label="New recipe" autocomplete="off" maxlength="120" role="combobox" aria-expanded="false" aria-controls="crm-newpull">' +
      '<span class="crm-pull" id="crm-newpull" role="listbox" hidden></span></span></td></tr></tbody></table></div>';
    h += '<p class="crm-help">Enter makes a row. Tab moves across; materials take a few letters and Enter, as on the item page. Each row is an ordinary crafting recipe, kept in Crafting and shown on the item it makes.</p></div>';
    return h;
  }

  // newHits is what the next-row box offers: a new recipe, then recipes
  // already written that this part doesn't hold yet.
  function newHits(st, q) {
    q = q.trim();
    if (!q) return [];
    var lo = q.toLowerCase(), held = st.data().recipes, out = [{make: q}];
    st.all.forEach(function (r) {
      if (out.length < 6 && held.indexOf(r.id) < 0 && r.name.toLowerCase().indexOf(lo) >= 0) out.push({id: r.id, name: r.name});
    });
    var same = st.all.filter(function (r) { return r.name.toLowerCase() === lo; })[0];
    if (same) out.shift();
    return out;
  }

  function wireTable(st, el, m) {
    el.querySelectorAll('[data-mat]').forEach(function (inp) {
      var rid = inp.getAttribute('data-mat');
      m.pickers.push(P.material(inp, {lib: function () { return st.lib; }, items: function () { return st.items; },
        onPick: function (pick) {
          var r = L.clone(st.recipes[rid]);
          r.materials.push(pick);
          m.refocus = '[data-mat="' + rid + '"]';
          save(st, r);
        },
        onMissing: function (name) { addToArmory(st, name, [], function (it) {
          var r = L.clone(st.recipes[rid]);
          r.materials.push({itemId: it.id, qty: 1});
          m.refocus = '[data-mat="' + rid + '"]';
          save(st, r);
        }); }}));
    });
    el.querySelectorAll('[data-name]').forEach(function (inp) {
      inp.addEventListener('change', function () {
        var r = L.clone(st.recipes[inp.getAttribute('data-name')]), v = inp.value.trim();
        if (!v || v === r.name) { inp.value = r.name; return; }
        r.name = v; save(st, r);
      });
    });
    el.querySelectorAll('[data-method]').forEach(function (s) {
      s.addEventListener('change', function () {
        var r = L.clone(st.recipes[s.getAttribute('data-method')]);
        if (s.value) r = L.applyTemplate(st.lib, r, s.value, st.byId); else r.steps = [];
        r.template = s.value;
        save(st, r);
      });
    });
    var nb = el.querySelector('[data-new]'), pull = el.querySelector('#crm-newpull'), hits = [], at = 0;
    if (!nb) return;
    function drawPull() {
      hits = newHits(st, nb.value);
      at = Math.min(at, Math.max(0, hits.length - 1));
      if (!hits.length) { pull.hidden = true; nb.setAttribute('aria-expanded', 'false'); return; }
      pull.innerHTML = hits.map(function (x, i) {
        return '<div class="crm-opt" role="option" data-i="' + i + '" aria-selected="' + (i === at) + '"><span class="crm-opt-n">' +
          (x.make ? 'New recipe “' + esc(x.make) + '”' : esc(x.name)) + '</span><span class="crm-opt-h">' + (x.make ? 'Enter' : 'add it here') + '</span></div>';
      }).join('');
      pull.hidden = false; nb.setAttribute('aria-expanded', 'true');
    }
    function take(i) {
      var x = hits[i];
      if (!x) return;
      pull.hidden = true;
      m.refocus = '[data-new]';
      if (x.id) { st.add([x.id]); st.say('Added ' + x.name + ' to this part.'); st.emit(); return; }
      nb.disabled = true;
      st.newRecipe({name: x.make}).then(function (got) {
        st.add([got.id]);
        st.say(got.draft ? 'Started ' + got.name + ' as a draft.' : 'Started ' + got.name + '.');
        m.refocus = '[data-mat="' + got.id + '"]';
        st.emit();
      }, function (err) { nb.disabled = false; st.say(err.message, true); });
    }
    nb.addEventListener('input', function () { at = 0; drawPull(); });
    nb.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown' && hits.length) { e.preventDefault(); at = (at + 1) % hits.length; drawPull(); }
      else if (e.key === 'ArrowUp' && hits.length) { e.preventDefault(); at = (at - 1 + hits.length) % hits.length; drawPull(); }
      else if (e.key === 'Enter') { e.preventDefault(); if (!pull.hidden) take(at); }
      else if (e.key === 'Escape' && !pull.hidden) { e.preventDefault(); e.stopPropagation(); pull.hidden = true; }
    });
    nb.addEventListener('blur', function () { setTimeout(function () { pull.hidden = true; nb.setAttribute('aria-expanded', 'false'); }, 150); });
    pull.addEventListener('mousedown', function (e) { e.preventDefault(); });
    pull.addEventListener('click', function (e) { var o = e.target.closest('[data-i]'); if (o) take(+o.getAttribute('data-i')); });
  }

  function save(st, r) {
    st.say('Saving…');
    st.save(r).then(function () { st.say('Saved'); st.emit(); }, function (err) { st.say(err.message, true); st.emit(); });
  }

  // addToArmory makes a Director-only item for a material the campaign
  // doesn't have yet.
  function addToArmory(st, name, kinds, then) {
    st.say('Adding ' + name + ' to the Armory…');
    st.api('POST', '/materials', {name: name, kinds: kinds}).then(function (it) {
      st.addItem(it);
      st.say(it.name + ' is in the Armory now, seen only by you.');
      if (then) then(it);
    }, function (err) { st.say(err.message, true); });
  }

  function tableClick(st, b) {
    var act = b.getAttribute('data-act');
    if (b.hasAttribute('data-rm-mat')) {
      var p = b.getAttribute('data-rm-mat').split(':'), r = L.clone(st.recipes[p[0]]);
      L.removeMaterial(r, +p[1]); save(st, r); return true;
    }
    if (b.hasAttribute('data-out')) {
      var id = b.getAttribute('data-out'), d = st.data();
      d.recipes = d.recipes.filter(function (x) { return x !== id; });
      st.setData(d);
      st.say('Taken out of this part. It is still in Crafting.');
      st.emit(); return true;
    }
    if (act === 'approve-all') { approveAll(st); return true; }
    return false;
  }

  function approveAll(st) {
    var rows = st.drafts();
    if (!rows.length) return;
    st.say('Approving…');
    st.approve(rows).then(function (n) { st.say('Approved ' + plural(n, 'recipe') + '. Players who know them can make them now.'); st.emit(); },
      function (err) { st.say(err.message, true); st.emit(); });
  }

  /* ---------- The inspector ---------- */

  function seg(name, opts, cur) {
    return '<div class="crm-seg" role="group">' + opts.map(function (o) {
      return '<button type="button" data-seg="' + name + '" data-v="' + esc(o[0]) + '" aria-pressed="' + (o[0] === cur) + '">' + esc(o[1]) + '</button>';
    }).join('') + '</div>';
  }

  function inspectorHTML(st) {
    var d = st.data(), drafts = st.drafts().length;
    return '<div class="crm-insp"><p class="crm-insp-h">Crafting recipes part</p>' +
      '<p>A list of crafting recipes kept together, so a set like an herbalist’s cures is written in one sitting. Each row is an ordinary recipe in Crafting and on the item it makes.</p>' +
      '<p class="crm-insp-h">Who can make them</p>' + seg('who', WHO, d.who) +
      '<p class="crm-insp-h">New rows start as</p>' + seg('start', START, d.start) +
      '<p>Players never see a draft. Approve a row here or on its item.' +
      (drafts ? ' <button type="button" class="crm-q" data-act="approve-all">Approve the ' + plural(drafts, 'draft') + '</button>' : '') + '</p>' +
      '<p class="crm-insp-h">Rules can use it</p><p>Add a When such as “a respite is logged” to hand out a recipe, or a Then to teach one.</p></div>';
  }

  // setWho changes the part's setting and every recipe it holds. Director
  // only is "learned" with nobody taught, so it forgets who learned them.
  function setWho(st, v) {
    var d = st.data(), rows = st.rows(), want = v === 'director' ? 'learned' : v;
    var taught = rows.filter(function (r) { return (r.known || []).length; });
    if (v === 'director' && taught.length && !window.confirm('Director only forgets who learned ' + plural(taught.length, 'recipe') + ' in this part. Go ahead?')) return;
    d.who = v; st.setData(d);
    var change = rows.filter(function (r) { return r.who !== want || (v === 'director' && (r.known || []).length); });
    if (!change.length) { st.emit(); return; }
    st.say('Saving…');
    change.reduce(function (p, r) {
      return p.then(function () { var x = L.clone(r); x.who = want; if (v === 'director') x.known = []; return st.save(x); });
    }, Promise.resolve()).then(function () { st.say('Saved'); st.emit(); }, function (err) { st.say(err.message, true); st.emit(); });
  }

  /* ---------- One mount ---------- */

  function mount(el, cfg) {
    var ctx = el._hbPart;
    if (!ctx || !ctx.part) { el.hidden = true; return null; }
    var st = store(ctx), mode = cfg.mode === 'inspector' ? 'inspector' : 'canvas', More = window.CraftingPartMore;
    var m = {pickers: [], refocus: '', dead: false};
    m.status = function () {
      var s = el.querySelector('.crm-status');
      if (s) { s.textContent = st.status; s.classList.toggle('is-bad', st.bad); }
    };
    m.draw = function () {
      if (m.dead) return;
      m.pickers.forEach(function (p) { p.destroy(); });
      m.pickers = [];
      if (st.failed) {
        el.innerHTML = '<div class="crm"><p class="crm-bad" role="alert">' + fa('triangle-exclamation') + ' Crafting couldn’t load: ' + esc(st.failed) +
          ' <button type="button" class="crm-q" data-act="retry">Try again</button></p></div>';
        return;
      }
      if (!st.ready) { el.innerHTML = '<div class="crm"><p class="crm-mut" role="status">Opening crafting…</p></div>'; return; }
      var view = More && st.view !== 'table' ? More[st.view] : null, h;
      if (mode === 'inspector') h = view ? view.inspector(st) : inspectorHTML(st);
      else h = view ? view.canvas(st) : tableHTML(st);
      el.innerHTML = '<div class="crm">' + h + (mode === 'canvas' ? '<p class="crm-status" role="status" aria-live="polite"></p>' : '') + '</div>';
      m.status();
      if (view) view.wire(st, el, m, mode); else if (mode === 'canvas') wireTable(st, el, m);
      if (m.refocus) { var f = el.querySelector(m.refocus); if (f) f.focus(); m.refocus = ''; }
    };
    el.addEventListener('click', function (e) {
      var b = e.target.closest('button');
      if (!b || !el.contains(b)) return;
      if (b.getAttribute('data-act') === 'retry') { st.load(); return; }
      if (b.hasAttribute('data-view')) { st.setView(b.getAttribute('data-view')); return; }
      if (b.hasAttribute('data-seg') && st.view === 'table') {
        var k = b.getAttribute('data-seg'), v = b.getAttribute('data-v');
        if (k === 'who') { setWho(st, v); return; }
        if (k === 'start') { var d = st.data(); d.start = v; st.setData(d); st.emit(); return; }
      }
      if (st.view === 'table') { tableClick(st, b); return; }
      if (More && More[st.view] && More[st.view].click) More[st.view].click(st, b, mode);
    });
    st.mounts.push(m);
    m.draw();
    return {destroy: function () {
      m.dead = true;
      m.pickers.forEach(function (p) { p.destroy(); });
      st.mounts = st.mounts.filter(function (x) { return x !== m; });
      el.innerHTML = '';
    }};
  }

  // Shared with crafting_part_more.js, which draws the other views.
  window.CraftingPart = {fa: fa, plural: plural, seg: seg, save: save, addToArmory: addToArmory};

  Chronicle.register('crafting-recipes-part', {
    init: function (el, cfg) {
      if (!window.CraftingLib || !window.CraftingPick) { el.hidden = true; return; }
      styles();
      el._crpMount = mount(el, cfg || {});
    },
    destroy: function (el) { if (el._crpMount) { el._crpMount.destroy(); el._crpMount = null; } }
  });
})();
