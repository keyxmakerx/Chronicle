/**
 * crafting_item.js -- "How it's made" on an Armory item's page
 * (data-widget="crafting-item")
 *
 * The signed placement for starting a recipe: on the item it makes. A
 * Director sees the recipe that makes the item and edits it in place
 * (materials by a few letters plus Enter, steps as short lines, a play mark
 * on each to try it as a player) or, with none, two ways to start one. For
 * a material, "In crafting" gives its kinds (edited here), the recipes that
 * use it and the drop lists that give it. A player sees only the recipe
 * pages their book already holds, on paper, and nothing at all when there
 * are none. Every save goes through the Crafting app's own routes, so the
 * same checks apply; new recipes start as drafts players never see.
 *
 * Attributes: data-campaign-id, data-entity-id, data-director ("true").
 */
(function () {
  'use strict';
  var L = window.CraftingLib, P = window.CraftingPick, esc = L.esc;

  function fa(n) { return '<i class="fa-solid fa-' + n + '" aria-hidden="true"></i>'; }

  function mount(el, cfg) {
    var cid = String(cfg.campaignId || ''), iid = String(cfg.entityId || ''), director = cfg.director === true || cfg.director === 'true';
    var base = '/campaigns/' + encodeURIComponent(cid) + '/crafting', app = '/campaigns/' + encodeURIComponent(cid) + '/crafting';
    var S = {lib: null, data: null, items: [], byId: {}, sel: 0, peek: false, kindsOpen: false, timer: 0, dead: false, status: '', pickers: []};

    function api(method, path, body) {
      return Chronicle.apiFetch(base + path, {method: method, body: body}).then(function (res) {
        if (res.status === 204) return null;
        return res.json().catch(function () { return null; }).then(function (d) {
          if (!res.ok) { var e = new Error((d && (d.message || d.error)) || 'Something went wrong. Try again.'); e.status = res.status; throw e; }
          return d;
        });
      });
    }
    function setStatus(text, bad) {
      S.status = text;
      var s = el.querySelector('.crm-status');
      if (s) { s.textContent = text; s.classList.toggle('is-bad', !!bad); }
    }
    function cur() { return S.data && S.data.madeBy[S.sel]; }

    function load() {
      var reads = [api('GET', '/items/' + encodeURIComponent(iid)), api('GET', '/library')];
      if (director) reads.push(api('GET', '/catalog'));
      Promise.all(reads).then(function (got) {
        if (S.dead) return;
        S.data = got[0]; S.lib = got[1];
        if (got[2]) {
          S.items = (got[2].items || []).slice().sort(function (a, b) { return a.name.localeCompare(b.name); });
          S.items.forEach(function (it) { S.byId[it.id] = it; });
        }
        draw();
      }, function (err) {
        if (S.dead) return;
        // A player who can't read anything here gets no empty block.
        if (!director) { el.hidden = true; return; }
        el.innerHTML = '<div class="crm"><p class="crm-bad" role="alert">' + fa('triangle-exclamation') + ' Crafting couldn’t load: ' + esc(err.message) +
          ' <button type="button" class="crm-q" data-retry>Try again</button></p></div>';
        el.querySelector('[data-retry]').addEventListener('click', load);
      });
    }

    /* ---------- Drawing ---------- */

    function draw() {
      S.pickers.forEach(function (p) { p.destroy(); });
      S.pickers = [];
      var d = S.data;
      if (!director) { drawPlayer(d); return; }
      var isMaterial = d.kinds.length || d.uses.length || d.drops.length;
      var h = '<div class="crm">';
      if (d.madeBy.length || !isMaterial) h += made(d);
      if (isMaterial || S.kindsOpen) h += inCrafting(d);
      h += '<p class="crm-status" role="status" aria-live="polite">' + esc(S.status) + '</p></div>';
      el.innerHTML = h;
      wire();
    }

    function drawPlayer(d) {
      if (!d.book.length && !d.uses.length) { el.hidden = true; el.innerHTML = ''; return; }
      var h = '<div class="crm">';
      if (d.book.length) {
        h += '<div class="crm-head"><h3 class="crm-h">How it’s made</h3></div>';
        d.book.forEach(function (p) { h += '<div class="crm-book">' + L.bookHTML(S.lib, p) + '</div>'; });
      }
      if (d.uses.length) {
        h += '<div class="crm-head"><h3 class="crm-h">In crafting</h3></div><dl class="crm-dl"><dt>Used in</dt><dd>' +
          d.uses.map(function (u) { return '<a href="' + app + '">' + esc(u.name) + '</a> <span class="crm-mut">' + useWords(u) + '</span>'; }).join('<br>') + '</dd></dl>';
      }
      el.innerHTML = h + '</div>';
    }

    function useWords(u) { return u.byKind ? 'as any ' + esc(L.tagLabel(S.lib, u.byKind)) + ', ' + u.qty + ' each' : u.qty + ' each'; }

    function state(r) {
      if (r.draft) return '<span class="crm-pill is-draft">Draft · players don’t see it</span><button type="button" class="crm-q" data-act="approve">Approve</button>';
      return r.who === 'learned' ? '<span class="crm-pill is-ok">Heroes who learn it can make it</span>' : '<span class="crm-pill is-ok">Players can make it</span>';
    }

    function made(d) {
      var h = '<section class="crm-sec" aria-label="How it’s made"><div class="crm-head"><h3 class="crm-h">How it’s made</h3>';
      if (!d.madeBy.length) {
        h += (!d.kinds.length && !S.kindsOpen ? '<span class="crm-words"><button type="button" class="crm-q" data-act="kinds">Use it as a material</button></span>' : '') +
          '</div><p class="crm-none">Nothing makes this yet. <button type="button" class="crm-q" data-act="write">Write how it’s made</button> or ' +
          '<button type="button" class="crm-q" data-act="bench">Work it out at the bench</button></p></section>';
        return h;
      }
      var r = cur();
      h += '<span class="crm-words">' + state(r) +
        '<a class="crm-q" href="' + app + '#try=' + esc(r.id) + '">Open at the bench</a>' +
        '<a class="crm-q" href="' + app + '#recipe=' + esc(r.id) + '&rules">Rules</a>' +
        '<button type="button" class="crm-q" data-act="peek" aria-expanded="' + S.peek + '">What players see</button></span></div>';
      if (d.madeBy.length > 1) {
        h += '<div class="crm-tabs" role="tablist" aria-label="Recipes that make it">' + d.madeBy.map(function (x, i) {
          return '<button type="button" role="tab" data-sel="' + i + '" aria-selected="' + (i === S.sel) + '">' + esc(x.name) + '</button>';
        }).join('') + '</div>';
      }
      h += '<div class="crm-grid' + (S.peek ? ' has-peek' : '') + '"><div class="crm-edit">' + takes(r) + steps(r) + foot(r) + checks(r) + '</div>';
      if (S.peek) h += '<div class="crm-book">' + L.bookHTML(S.lib, L.book(S.lib, r, S.byId)) + '</div>';
      return h + '</div></section>';
    }

    function takes(r) {
      var h = '<div class="crm-row"><span class="crm-lab">Takes</span><div class="crm-chips">';
      r.materials.forEach(function (m, i) {
        h += '<span class="crm-chip">' + P.pic(L.matPicture(S.lib, m, S.byId), 20) + '<span>' + m.qty + ' × ' + esc(L.matName(S.lib, m, S.byId)) + '</span>' +
          '<a class="crm-play" href="' + app + '#try=' + esc(r.id) + '" title="Try it as a player" aria-label="Try ' + esc(L.matName(S.lib, m, S.byId)) + ' as a player">' + fa('play') + '</a>' +
          '<button type="button" class="crm-x" data-rm-mat="' + i + '" aria-label="Remove ' + esc(L.matName(S.lib, m, S.byId)) + '">' + fa('xmark') + '</button></span>';
      });
      return h + '<span class="crm-type"><input class="crm-in" data-mat placeholder="' + (r.materials.length ? 'Add a material' : 'Type a material, like “2 moon”') + '" aria-label="Add a material" autocomplete="off"></span></div></div>';
    }

    function steps(r) {
      var h = '<div class="crm-row crm-row-top"><span class="crm-lab">Steps</span><ol class="crm-steps">';
      r.steps.forEach(function (s, i) {
        var k = L.stepKind(S.lib, s.t), chk = L.stepCheck(S.lib, r, i, S.byId);
        h += '<li class="crm-step' + (chk.warn ? ' is-warn' : '') + '"><span class="crm-n">' + (i + 1) + '</span><span class="crm-k">' + esc(k ? k.name.split(' ')[0] : s.t) + '</span>' +
          '<span class="crm-t">' + esc(P.stepWords(S.lib, r, s, S.byId)) + (chk.warn ? '<em>' + esc(chk.msg) + '</em>' : '') + '</span>' +
          '<span class="crm-ends"><button type="button" class="crm-ib" data-up="' + i + '" aria-label="Move step ' + (i + 1) + ' up"' + (i ? '' : ' disabled') + '>' + fa('arrow-up') + '</button>' +
          '<button type="button" class="crm-ib" data-rm-step="' + i + '" aria-label="Remove step ' + (i + 1) + '">' + fa('xmark') + '</button>' +
          '<a class="crm-play" href="' + app + '#try=' + esc(r.id) + '" title="Try it as a player" aria-label="Try step ' + (i + 1) + ' as a player">' + fa('play') + '</a></span></li>';
      });
      return h + '<li class="crm-step crm-step-new"><span class="crm-n"></span><input class="crm-in" data-step placeholder="Type a step, like “grind 3”" aria-label="Add a step" autocomplete="off"></li></ol></div>';
    }

    function foot(r) {
      var R = S.lib.rules, stats = R.characteristics || [], B = L.benchKind(S.lib, r.bench), label = (R.labels && R.labels.goal) || 'Goal';
      var makes = r.result.itemId && S.byId[r.result.itemId] ? S.byId[r.result.itemId].name : S.data.name;
      return '<div class="crm-foot"><label><b>Makes</b> <input class="crm-num" type="number" min="1" max="99" data-f="qty" value="' + r.result.qty + '" aria-label="How many it makes"> × ' + esc(makes) + '</label>' +
        '<label><b>' + esc(label) + '</b> <input class="crm-num" type="number" min="1" max="999" data-f="goal" value="' + r.goal + '"> · ' + esc(R.dice) +
        (stats.length ? ' + <select class="crm-sel" data-f="stat" aria-label="Characteristic"><option value="">nothing</option>' + stats.map(function (c) {
          return '<option value="' + esc(c.key) + '"' + (c.key === r.stat ? ' selected' : '') + '>' + esc(c.label) + '</option>';
        }).join('') + '</select>' : '') + '</label>' +
        (B ? '<span><b>Bench</b> ' + esc(B.name.replace(/ bench$/i, '')) + '</span>' : '') +
        '<span><b>Who knows it</b> <button type="button" class="crm-q" data-act="who">' + (r.who === 'learned' ? 'Heroes who learn it' : 'Everyone') + '</button></span></div>';
    }

    function checks(r) {
      var warn = L.checks(S.lib, r, S.byId).filter(function (c) { return c.warn; });
      return warn.length ? '<ul class="crm-checks">' + warn.map(function (c) { return '<li>' + fa('circle-exclamation') + esc(c.msg) + '</li>'; }).join('') + '</ul>' : '';
    }

    function inCrafting(d) {
      var h = '<section class="crm-sec" aria-label="In crafting"><div class="crm-head"><h3 class="crm-h">In crafting</h3><span class="crm-words">' +
        '<button type="button" class="crm-q" data-act="uses">Make a recipe that uses it</button></span></div>';
      h += '<div class="crm-row"><span class="crm-lab">Kind</span><div class="crm-chips">' + d.kinds.map(function (k) {
        return '<span class="crm-chip">' + P.pic(L.matPicture(S.lib, k), 20) + '<span>' + esc(L.tagLabel(S.lib, k).replace(/^./, function (c) { return c.toUpperCase(); })) + '</span>' +
          '<button type="button" class="crm-x" data-rm-kind="' + esc(k) + '" aria-label="Remove the kind ' + esc(L.tagLabel(S.lib, k)) + '">' + fa('xmark') + '</button></span>';
      }).join('') + '<span class="crm-type"><input class="crm-in" data-kind placeholder="Add a kind" aria-label="Add a kind" autocomplete="off" list="crm-kinds-' + esc(iid) + '">' +
        '<datalist id="crm-kinds-' + esc(iid) + '">' + S.lib.tags.filter(function (t) { return d.kinds.indexOf(t.key) < 0; }).map(function (t) { return '<option value="' + esc(t.label) + '">'; }).join('') + '</datalist></span></div></div>';
      h += '<div class="crm-row"><span class="crm-lab">Used in</span><div>' + (d.uses.length ? d.uses.map(function (u) {
        return '<a class="crm-link" href="' + app + '#recipe=' + esc(u.id) + '">' + esc(u.name) + '</a> <span class="crm-mut">' + useWords(u) + (u.draft ? ' · draft' : '') + '</span>';
      }).join('<br>') : '<span class="crm-mut">No recipe asks for it yet.</span>') + '</div></div>';
      h += '<div class="crm-row"><span class="crm-lab">Comes from</span><div>' + (d.drops.length ? d.drops.map(function (x) {
        return '<a class="crm-link" href="/campaigns/' + encodeURIComponent(cid) + '/entities/' + encodeURIComponent(x.pageId) + '">' + esc(x.page) + '</a> <span class="crm-mut">' +
          (x.mode === 'harvest' ? 'harvested' : (x.qty || 1) + ' dropped') + '</span>';
      }).join('<br>') : '<span class="crm-mut">No drop list names it yet.</span>') +
        (d.dropsCut ? '<br><span class="crm-mut">Only the newest drop lists were read.</span>' : '') +
        ' <a class="crm-q" href="' + app + '#spoils">Add it to a creature’s drops</a></div></div>';
      return h + '</section>';
    }

    /* ---------- Saving ---------- */

    function save(r) {
      clearTimeout(S.timer);
      setStatus('Saving…');
      S.timer = setTimeout(function () {
        api('PUT', '/recipes/' + encodeURIComponent(r.id), L.input(r)).then(function (got) {
          var i = S.data.madeBy.indexOf(r);
          if (i >= 0) S.data.madeBy[i] = got;
          setStatus('Saved');
          if (i === S.sel) draw();
        }, function (err) { setStatus(err.message, true); });
      }, 400);
    }
    function change(fn) { var r = cur(); fn(r); draw(); save(r); }

    function newRecipe(extra) {
      var r = {name: S.data.name, template: '', bench: '', materials: [], steps: [], result: {itemId: iid, qty: 1}, goal: 1, stat: '', prerequisite: '', source: '',
        cost: {amount: 0, currency: ''}, who: 'all', known: [], colors: {}, version: 0, draft: true};
      if (extra) extra(r);
      return api('POST', '/recipes', r);
    }

    function wire() {
      el.querySelectorAll('[data-sel]').forEach(function (b) { b.addEventListener('click', function () { S.sel = +b.getAttribute('data-sel'); draw(); }); });
      var mi = el.querySelector('[data-mat]');
      if (mi) S.pickers.push(P.material(mi, {lib: function () { return S.lib; }, items: function () { return S.items; },
        onPick: function (m) { change(function (r) { r.materials.push(m); }); refocus('[data-mat]'); }}));
      var si = el.querySelector('[data-step]');
      if (si) si.addEventListener('keydown', function (e) {
        if (e.key !== 'Enter') return;
        e.preventDefault();
        var r = cur(), got = P.step(si.value, S.lib, r, S.items);
        if (got.error) { setStatus(got.error, true); return; }
        change(function (x) {
          if (got.addMaterial) { x.materials.push(got.addMaterial); got.step.material = x.materials.length - 1; }
          x.steps.push(got.step);
        });
        refocus('[data-step]');
      });
      var ki = el.querySelector('[data-kind]');
      if (ki) ki.addEventListener('keydown', function (e) {
        if (e.key !== 'Enter') return;
        e.preventDefault();
        var q = ki.value.trim().toLowerCase(), t = null;
        S.lib.tags.forEach(function (x) { if (!t && (x.label.toLowerCase().indexOf(q) === 0 || x.key === q)) t = x; });
        if (!q || !t) { setStatus(q ? 'There is no kind called “' + ki.value.trim() + '”.' : '', !!q); return; }
        setKinds(S.data.kinds.concat([t.key]), '[data-kind]');
      });
      el.querySelectorAll('[data-f]').forEach(function (f) {
        f.addEventListener('change', function () {
          var k = f.getAttribute('data-f'), v = f.value;
          change(function (r) {
            if (k === 'qty') r.result.qty = Math.max(1, Math.min(99, parseInt(v, 10) || 1));
            if (k === 'goal') r.goal = Math.max(1, Math.min(999, parseInt(v, 10) || 1));
            if (k === 'stat') r.stat = v;
          });
        });
      });
    }
    function refocus(sel) { var x = el.querySelector(sel); if (x) x.focus(); }

    function setKinds(kinds, focus) {
      setStatus('Saving…');
      api('PUT', '/materials/' + encodeURIComponent(iid), {tags: kinds}).then(function (got) {
        S.data.kinds = (got && got.tags) || kinds;
        setStatus('Saved'); draw(); if (focus) refocus(focus);
      }, function (err) { setStatus(err.message, true); });
    }

    el.addEventListener('click', function (e) {
      var b = e.target.closest('button');
      if (!b || !el.contains(b)) return;
      var act = b.getAttribute('data-act');
      if (b.hasAttribute('data-rm-mat')) { var i = +b.getAttribute('data-rm-mat'); change(function (r) { L.removeMaterial(r, i); }); return; }
      if (b.hasAttribute('data-rm-step')) { var j = +b.getAttribute('data-rm-step'); change(function (r) { r.steps.splice(j, 1); }); return; }
      if (b.hasAttribute('data-up')) { var u = +b.getAttribute('data-up'); change(function (r) { var s = r.steps.splice(u, 1)[0]; r.steps.splice(u - 1, 0, s); }); return; }
      if (b.hasAttribute('data-rm-kind')) { var k = b.getAttribute('data-rm-kind'); setKinds(S.data.kinds.filter(function (x) { return x !== k; })); return; }
      if (act === 'peek') { S.peek = !S.peek; draw(); return; }
      if (act === 'kinds') { S.kindsOpen = true; draw(); refocus('[data-kind]'); return; }
      if (act === 'who') { change(function (r) { r.who = r.who === 'learned' ? 'all' : 'learned'; }); return; }
      if (act === 'approve') {
        var r = cur();
        api('POST', '/recipes/' + encodeURIComponent(r.id) + '/approve').then(function (got) { S.data.madeBy[S.sel] = got; setStatus('Approved. Players who know it can make it now.'); draw(); },
          function (err) { setStatus(err.message, true); });
        return;
      }
      if (act === 'write' || act === 'bench') {
        b.disabled = true;
        setStatus('Starting a recipe…');
        newRecipe().then(function (got) {
          if (act === 'bench') { window.location.href = app + '#try=' + encodeURIComponent(got.id); return; }
          S.data.madeBy.push(got); S.sel = S.data.madeBy.length - 1; setStatus('Started as a draft. Players see it once you approve it.'); draw(); refocus('[data-mat]');
        }, function (err) { b.disabled = false; setStatus(err.message, true); });
        return;
      }
      if (act === 'uses') {
        b.disabled = true;
        newRecipe(function (r) { r.name = 'Made with ' + S.data.name; r.result = {itemId: '', qty: 1}; r.materials = [{itemId: iid, qty: 1}]; }).then(function (got) {
          window.location.href = app + '#recipe=' + encodeURIComponent(got.id);
        }, function (err) { b.disabled = false; setStatus(err.message, true); });
      }
    });

    load();
    return {destroy: function () { S.dead = true; clearTimeout(S.timer); S.pickers.forEach(function (p) { p.destroy(); }); el.innerHTML = ''; }};
  }

  Chronicle.register('crafting-item', {
    init: function (el, cfg) {
      if (!window.CraftingLib || !window.CraftingPick) { el.hidden = true; return; }
      el._crm = mount(el, cfg || {});
    },
    destroy: function (el) { if (el._crm) { el._crm.destroy(); el._crm = null; } }
  });
})();
