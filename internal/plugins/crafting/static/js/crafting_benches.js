/**
 * crafting_benches.js -- the campaign's benches (window.CraftingBenches)
 *
 * The workshop's Benches tab: a strip of the campaign's benches over the
 * bench in build mode (craft_bench with data-mode="build", the signed
 * Customize mockup). A bench is a named place of one ready-made kind; each
 * look (the desk, the journal) keeps its own arrangement of the room, its
 * pieces, their words and right-click actions. The build editor hands every
 * change here through el._craftApp.onChange, and it saves a moment later
 * with the bench's version, so two Directors never silently overwrite each
 * other. "Try it as a player" plays the campaign's own recipes. Under the
 * bench, its upgrade ladders (crafting_upgrades.js) save through the same queue.
 */
window.CraftingBenches = (function () {
  'use strict';
  var L = window.CraftingLib, esc = L.esc;

  function opt(v, label, on) { return '<option value="' + esc(v) + '"' + (on ? ' selected' : '') + '>' + esc(label) + '</option>'; }
  function cap(s) { s = String(s || ''); return s.charAt(0).toUpperCase() + s.slice(1); }
  function a(w) { return (/^[aeiou]/i.test(w) ? 'an ' : 'a ') + w; }

  /* The campaign's recipes as the bench engine plays them: each material
     becomes an ingredient keyed by its item or tag, tinted by its tag. */
  function engineData(app, kind) {
    var lib = app.lib, CR = window.CraftingRecipes, items = {}, recipes = {};
    var stats = {};
    (lib.rules.characteristics || []).forEach(function (c) { stats[c.key] = c.label; });
    app.recipes.forEach(function (r) {
      if (!r.steps.length || r.bench !== kind) return;
      var keyOf = [];
      r.materials.forEach(function (m, i) {
        var k = m.itemId ? 'i' + m.itemId.replace(/[^a-zA-Z0-9]/g, '') : 't' + m.tag, t = L.matTags(m, app.items)[0];
        if (!items[k]) items[k] = {name: L.matName(lib, m, app.items), color: (t && CR.TAG_COLOR[t]) || '#8a8a8a', desc: m.tag ? 'Any ' + L.tagLabel(lib, m.tag) + ' will do.' : ''};
        keyOf[i] = k;
      });
      var col = r.colors || {};
      recipes['r' + r.id.replace(/[^a-zA-Z0-9]/g, '')] = {name: r.name, kind: L.templateName(lib, r.template), who: 'the hero', stat: stats[r.stat] || r.stat || '', mod: 0,
        goal: r.goal, start: 0, base: col.base || CR.BENCH_COLORS.base, paste: col.paste || CR.BENCH_COLORS.paste, mid: col.mid || CR.BENCH_COLORS.mid, final: col.final || CR.BENCH_COLORS.final,
        steps: r.steps.map(function (s) {
          var o = {t: s.t};
          if (s.material != null && keyOf[s.material]) o.item = keyOf[s.material];
          ['to', 'turns', 'secs', 'n'].forEach(function (p) { if (s[p] != null) o[p] = s[p]; });
          return o;
        })};
    });
    return {items: items, recipes: recipes};
  }
  function engineKey(id) { return 'r' + String(id).replace(/[^a-zA-Z0-9]/g, ''); }

  function mount(el, app) {
    var lib = app.lib, B = app.benches, sel = 0, skin = lib.settings.skin || 'desk', tryId = '', benchEl = null, q = null, seen = false;
    function cur() { return B[sel]; }
    function $(s) { return el.querySelector(s); }

    /* ---------- Saving: one queue per bench, rename and arrangement alike ---------- */
    function failed(err) {
      if (err.status === 409 && /changed by someone else/.test(err.message)) app.status('error', 'Someone else changed this bench. Reload to see it.', app.reloadFix);
      else app.status('error', cap(err.message));
    }
    function queue(b) {
      if (!q || q.id !== b.id) q = {id: b.id, b: b, timer: 0, busy: false, name: false, up: false, looks: {}};
      return q;
    }
    function want(b, what, data) {
      var w = queue(b);
      if (what === 'name') w.name = true; else if (what === 'upgrades') w.up = true; else w.looks[what] = data;
      clearTimeout(w.timer);
      w.timer = setTimeout(function () { flush(w); }, 600);
      app.status('saving');
      edStatus('Saving…');
    }
    function flush(w, keep) {
      if (!w) return Promise.resolve();
      if (w.busy) return w.p;
      var b = w.b, path = '/benches/' + encodeURIComponent(b.id), send, skinKey = Object.keys(w.looks)[0];
      clearTimeout(w.timer);
      if (w.name) { w.name = false; send = app.api('PUT', path, {name: b.name, kind: b.kind, version: b.version}, keep); }
      else if (w.up) { w.up = false; send = app.api('PUT', path + '/upgrades', {upgrades: b.upgrades, version: b.version}, keep); }
      else if (skinKey) { var st = w.looks[skinKey]; delete w.looks[skinKey]; send = app.api('PUT', path + '/look/' + skinKey, {state: st, version: b.version}, keep); }
      else { app.status('saved'); edStatus('All changes saved'); return Promise.resolve(); }
      w.busy = true;
      // w.p settles once everything queued behind this save has gone too
      w.p = send.then(function (got) {
        w.busy = false;
        b.version = got.version; b.name = got.name;
        if (!w.looks.desk) b.desk = got.desk;
        if (!w.looks.journal) b.journal = got.journal;
        return flush(w, keep);
      }, function (err) { w.busy = false; failed(err); edStatus('Not saved'); });
      return w.p;
    }
    function edStatus(t) { if (benchEl && benchEl._crb && benchEl._crb.status) benchEl._crb.status(t); }

    /* ---------- The bench in build mode ---------- */
    function unmountBench() {
      if (benchEl) { if (Chronicle.destroyWidget) Chronicle.destroyWidget(benchEl); benchEl.remove(); benchEl = null; }
    }
    function mountBench() {
      unmountBench();
      var b = cur(), box = $('.cra-bmount');
      if (!b || !box) return;
      var data = engineData(app, b.kind), keys = Object.keys(data.recipes), pick = tryId && data.recipes[engineKey(tryId)] ? engineKey(tryId) : keys[0];
      benchEl = document.createElement('div');
      benchEl.setAttribute('data-widget', 'craft_bench');
      benchEl.setAttribute('data-mode', 'build');
      benchEl.setAttribute('data-skin', skin);
      benchEl.setAttribute('data-thumbs', '/static/img/crafting/' + skin + '/');
      if (app.calendarId) { benchEl.setAttribute('data-campaign-id', app.cid); benchEl.setAttribute('data-calendar-id', app.calendarId); }
      benchEl.innerHTML = '<p class="cra-fine" role="status">Loading the bench…</p>';
      var saved = b[skin] && typeof b[skin] === 'object' ? b[skin] : null, which = skin;
      benchEl._craftApp = {state: saved, recipes: keys.length ? data.recipes : null, items: data.items, recipe: pick, status: 'Changes save as you go.',
        onChange: function (state) { b[which] = state; want(b, which, state); }};
      box.appendChild(benchEl);
      Chronicle.mountWidget(benchEl);
    }

    /* ---------- Drawing ---------- */
    function render() {
      var b = cur();
      if (!b) {
        unmountBench();
        el.innerHTML = '<div class="cra-empty cra-bempty"><h3 class="cra-ph">No benches yet</h3><p>A bench is where recipes are made. Set up its room, name its pieces and give them right-click actions; players see it when they craft.</p>' +
          '<div class="cra-row">' + lib.benchKinds.map(function (k) { return '<button type="button" class="btn btn-primary btn-sm" data-new="' + esc(k.key) + '">Set up ' + esc(a(k.name.toLowerCase())) + '</button>'; }).join('') + '</div></div>';
        return;
      }
      var kind = L.benchKind(lib, b.kind), data = engineData(app, b.kind), keys = Object.keys(data.recipes);
      var h = '<div class="cra-bstrip"><ul class="cra-blist" aria-label="Benches">' + B.map(function (x, i) {
        var k = L.benchKind(lib, x.kind);
        return '<li><button type="button" class="cra-chip" data-b="' + i + '" aria-pressed="' + (i === sel) + '">' + esc(x.name) + '<small>' + esc(k ? k.name : x.kind) + '</small></button></li>';
      }).join('') + '</ul><div class="cra-addwrap"><button type="button" class="btn btn-secondary btn-sm" data-act="new-menu" aria-expanded="false" aria-haspopup="menu">New bench</button>' +
        '<div class="cra-menu" role="menu" hidden>' + lib.benchKinds.map(function (k) {
          return '<button type="button" role="menuitem" data-new="' + esc(k.key) + '"><span><b>' + esc(k.name) + '</b><small>' + esc(k.tools.map(function (t) { return L.toolName(lib, t); }).join(', ')) + '</small></span></button>';
        }).join('') + '</div></div></div>';
      h += '<div class="cra-bbar"><label class="cra-fl"><span>Name</span><input class="cra-in" data-f="bname" maxlength="80" value="' + esc(b.name) + '"></label>' +
        '<div class="cra-ctl"><span>Look</span><div class="cra-seg" role="group" aria-label="Look">' + lib.skins.map(function (s) {
          return '<button type="button" data-skin="' + esc(s.key) + '" aria-pressed="' + (skin === s.key) + '">' + esc(s.label) + '</button>';
        }).join('') + '</div><em>Each look keeps its own room.</em></div>' +
        '<label class="cra-fl"><span>Try it with</span><select class="cra-in" data-f="try"' + (keys.length ? '' : ' disabled') + '>' + (keys.length ? app.recipes.filter(function (r) { return data.recipes[engineKey(r.id)]; }).map(function (r) {
          return opt(r.id, r.name, engineKey(r.id) === (tryId && data.recipes[engineKey(tryId)] ? engineKey(tryId) : keys[0]));
        }).join('') : opt('', 'A sample recipe', true)) + '</select><em>' + (keys.length ? 'Played by “Try it as a player”.' : 'Your ' + esc(kind ? kind.name.toLowerCase() : 'bench') + ' recipes appear here once they have steps.') + '</em></label>' +
        '<div class="cra-bend"><button type="button" class="btn btn-danger btn-sm" data-act="delete">Delete bench</button></div></div>' +
        '<p class="cra-fine">Players craft a recipe at any bench of its kind' + (kind ? ': this is ' + esc(a(kind.name.toLowerCase())) : '') + '.</p>' +
        '<div class="cra-bmount"></div><section class="cra-upmount" aria-label="Upgrades"></section>';
      el.innerHTML = h;
      mountBench();
      window.CraftingUpgrades.mount($('.cra-upmount'), b, {skills: app.skills, toast: app.toast, onChange: function () { want(b, 'upgrades'); }});
    }
    function menuOpen(v) {
      var m = $('.cra-menu'), b = $('[data-act="new-menu"]');
      if (!m) return;
      m.hidden = !v; b.setAttribute('aria-expanded', String(v));
      if (v) { var first = m.querySelector('button'); if (first) first.focus(); }
    }
    function newName(kind) {
      var k = L.benchKind(lib, kind), n = k ? k.name : 'Bench', i = 2, taken = {};
      B.forEach(function (b) { taken[b.name] = 1; });
      var base = n;
      while (taken[n]) n = base + ' ' + (i++);
      return n;
    }
    /* Switching bench or look waits for the last save, so nothing is lost
       when the editor is rebuilt. */
    function then(fn) { (q ? flush(q) : Promise.resolve()).then(function () { fn(); }); }
    function create(kind) {
      then(function () {
        app.status('saving');
        app.api('POST', '/benches', {name: newName(kind), kind: kind}).then(function (b) {
          B.push(b); sel = B.length - 1; app.status('saved'); render();
          var n = $('[data-f="bname"]'); if (n) { n.focus(); n.select(); }
        }, failed);
      });
    }
    function remove(b) {
      app.confirm('Delete “' + b.name + '”?', 'The bench and both of its arranged looks go. Recipes stay, and players craft them at another bench of the same kind.', 'Delete bench').then(function (yes) {
        if (!yes) return;
        if (q && q.id === b.id) { clearTimeout(q.timer); q = null; }
        app.api('DELETE', '/benches/' + encodeURIComponent(b.id)).then(function () {
          var i = B.indexOf(b); if (i >= 0) B.splice(i, 1);
          sel = Math.max(0, Math.min(sel, B.length - 1)); render(); app.toast('Deleted “' + b.name + '”.');
        }, failed);
      });
    }

    el.addEventListener('input', function (e) {
      if (e.target.getAttribute('data-f') !== 'bname' || !cur()) return;
      var v = e.target.value;
      if (!v.trim()) return;
      cur().name = v;
      var chip = el.querySelector('.cra-blist [data-b="' + sel + '"]');
      if (chip) chip.firstChild.nodeValue = v;
      want(cur(), 'name');
    });
    el.addEventListener('change', function (e) {
      if (e.target.getAttribute('data-f') === 'try') { tryId = e.target.value; then(mountBench); }
    });
    el.addEventListener('click', function (e) {
      var b = e.target.closest('button');
      if (!b || !el.contains(b) || b.closest('.cra-bmount, .cra-upmount')) return;
      if (b.hasAttribute('data-b')) { var i = +b.getAttribute('data-b'); if (i !== sel) then(function () { sel = i; render(); }); return; }
      if (b.hasAttribute('data-new')) { menuOpen(false); create(b.getAttribute('data-new')); return; }
      if (b.hasAttribute('data-skin')) {
        var k = b.getAttribute('data-skin');
        if (k !== skin) then(function () { skin = k; render(); });
        return;
      }
      var act = b.getAttribute('data-act');
      if (act === 'new-menu') { menuOpen($('.cra-menu').hidden); return; }
      if (act === 'delete' && cur()) remove(cur());
    });
    function outside(e) { var m = $('.cra-menu'); if (m && !m.hidden && !e.target.closest('.cra-addwrap')) menuOpen(false); }
    function escKey(e) { var m = $('.cra-menu'); if (e.key === 'Escape' && m && !m.hidden) { menuOpen(false); $('[data-act="new-menu"]').focus(); } }
    document.addEventListener('click', outside);
    document.addEventListener('keydown', escKey);
    function leave() { if (q) flush(q, true); }
    window.addEventListener('pagehide', leave);

    return {
      // The bench mounts the first time its tab opens, and again after the
      // recipes it plays may have changed.
      shown: function () { if (!seen) { seen = true; render(); } else then(render); },
      // Called before the tab shows, so the bench mounts once, already on it
      tryRecipe: function (rid) {
        var r = null;
        app.recipes.forEach(function (x) { if (x.id === rid) r = x; });
        tryId = rid;
        if (r) for (var i = 0; i < B.length; i++) if (B[i].kind === r.bench) { sel = i; break; }
      },
      destroy: function () {
        leave();
        unmountBench();
        document.removeEventListener('click', outside);
        document.removeEventListener('keydown', escKey);
        window.removeEventListener('pagehide', leave);
      }
    };
  }
  return {mount: mount, engineData: engineData};
})();
