/**
 * crafting_block.js -- the Crafting block on a character page (data-widget="crafting-block")
 *
 * The party's spoils tray, the hero's projects in progress and the recipes
 * they know, laid out as the signed Crafting projects and Crafting Spoils
 * mockups. Taking or harvesting a spoil goes to the server, which rolls a
 * harvest's test and puts what it pays in the hero's pack. Starting a project, working it and every
 * roll go to the server, which takes the materials and cost through the
 * Armory, rolls the dice and reads the recipe's rules; the block shows what
 * it answered, and throws Chronicle's dice on the server's numbers. The
 * bench is the campaign's own, played by the craft_bench widget, which hands
 * back how the steps went. The server renders the block only for the
 * Director and the player who claimed the character.
 *
 * Attributes: data-campaign-id, data-entity-id.
 */
(function () {
  'use strict';
  var L = window.CraftingLib, T = window.CraftingTray, esc = L.esc, progress = L.progress;
  // What the server says in place of a secret rule (verdict.go ruleWords).
  var HIDDEN = 'A rule the Director keeps hidden';
  var ICON = {
    more: '<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><circle cx="3" cy="8" r="1.5" fill="currentColor"/><circle cx="8" cy="8" r="1.5" fill="currentColor"/><circle cx="13" cy="8" r="1.5" fill="currentColor"/></svg>',
    check: '<svg viewBox="0 0 20 20" width="18" height="18" aria-hidden="true"><path d="M4 10.5l4 4 8-9" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>'
  };

  function plural(n, one, many) { return n + ' ' + (n === 1 ? one : many); }
  function trimDot(s) { return String(s || '').replace(/[.]+$/, ''); }
  function num(cents) { var n = Math.round(cents) / 100; return n % 1 ? n.toFixed(2) : String(n); }
  function signed(n) { return n < 0 ? '−' + (-n) : '+' + n; }

  /* ---------- What the pack allows (pure, for the tests) ---------- */

  function held(pack, id) {
    var n = 0;
    (pack.items || []).forEach(function (it) { if (it.itemId === id) n += it.qty; });
    return n;
  }
  /* How many of an item the recipe's named materials take. */
  function reserved(r, id) {
    var n = 0;
    r.needs.forEach(function (m) { if (m.itemId === id) n += m.qty; });
    return n;
  }
  /* The carried items that can fill an "any <tag>" slot, after the recipe's
     named materials have taken theirs. */
  function tagged(pack, r, tag) {
    return (pack.items || []).filter(function (it) { return (it.tags || []).indexOf(tag) >= 0; })
      .map(function (it) { return {id: it.itemId, name: it.name, qty: it.qty - reserved(r, it.itemId)}; })
      .filter(function (x) { return x.qty > 0; });
  }
  /* What may stand in for a named material: carried items with its
     substitute's tag, other than the material itself. */
  function standIns(pack, r, m) {
    return m.sub ? tagged(pack, r, m.sub.tag).filter(function (x) { return x.id !== m.itemId && x.qty >= m.qty; }) : [];
  }
  var SUB_COST = {bane: 'at a bane on every roll', points: 'at −2 on every roll', none: 'at no cost'};
  /* The cost in words, against the hero's way of paying. */
  function costText(r, pack) {
    if (!r.cost) return '';
    if (pack.payKind === 'wealth') return 'Wealth ' + num(r.charge);
    return L.money(r.cost);
  }
  /* The unit a coins sheet's money reads in: the recipe's coin when no
     exchange was needed, else the sheet's own word for it. */
  function unit(r, pack) {
    if (pack.payKind === 'purse') return 'gp';
    return r.cost && Math.round(r.cost.amount * 100) === r.charge ? r.cost.currency : pack.moneyLabel;
  }
  /* Why a recipe can't be started now, first reason first. */
  function blockers(r, pack, hero) {
    var out = [], named = {};
    if (r.goal < 1) out.push('No goal yet. The Director sets one.');
    if (!r.makes) out.push('It makes nothing yet. The Director picks the item.');
    r.needs.forEach(function (m) {
      if (m.itemId) {
        if (named[m.itemId]) return;
        named[m.itemId] = 1;
        var want = reserved(r, m.itemId), have = held(pack, m.itemId);
        if (have < want && !standIns(pack, r, m).length) out.push('Needs ' + want + ' × ' + m.name + '. ' + hero + ' has ' + (have || 'none') + '.');
      } else {
        var n = tagged(pack, r, m.tag).reduce(function (a, x) { return a + x.qty; }, 0);
        if (n < m.qty) out.push('Needs ' + m.qty + ' ' + m.name + '. ' + hero + ' has ' + (n || 'none') + '.');
      }
    });
    if (r.cost) {
      if (!pack.payKind) out.push('Costs ' + L.money(r.cost) + ', and ' + hero + '’s sheet has no money field.');
      else if (pack.have < r.charge) {
        if (pack.payKind === 'wealth') out.push('Needs Wealth ' + num(r.charge) + '. ' + hero + '’s Wealth is ' + num(pack.have) + '.');
        else out.push('Costs ' + L.money(r.cost) + '. ' + hero + ' has ' + num(pack.have) + ' ' + unit(r, pack) + '.');
      }
    }
    return out;
  }
  /* What befell the work on a roll, beside its points: a breakthrough, a
     mishap, ingredients lost to a botch, and what the item came out as. */
  function befell(R, name, hero) {
    var h = '', came = (R.project && R.project.came) || [];
    if (R.breakthrough) h += '<div class="crp-made crp-befell">' + ICON.check + '<span><b>A breakthrough.</b> The dice came up high, so they were rolled again and added.</span></div>';
    if (R.mishap) h += '<div class="crp-made is-warn crp-befell"><span><b>A mishap mid-brew:</b> ' + esc(R.mishap.text) + '</span></div>';
    if (R.lost) h += '<div class="crp-made is-warn crp-befell"><span><b>The botch ruined the ingredients.</b> ' + esc(hero) + ' has to start ' + esc(name) + ' again.</span></div>';
    if (R.finished && came.length) h += '<p class="cra-fine crp-came">It came out: ' + came.map(esc).join(' · ') + '</p>';
    return h;
  }
  /* A ready roll held by the recipe's rule, said in full. */
  function waitText(p, today) {
    var w = trimDot(p.waits);
    if (w === HIDDEN) return w + ' holds it today, so the roll waits.';
    return w + ' (the recipe’s rule). ' + (today ? 'Today is ' + today + ', so the roll waits.' : 'So the roll waits.');
  }
  /* What a roll's terms are, as the result lists them. */

  /* ---------- The block ---------- */

  function menuBtn(id, isOpen) {
    return '<button type="button" class="crp-ib" data-act="menu" data-id="' + esc(id) + '" aria-haspopup="menu" aria-expanded="' + isOpen + '" aria-label="More for this project">' + ICON.more + '</button>';
  }

  function mount(el, cfg) {
    var cid = String(cfg.campaignId || ''), eid = String(cfg.entityId || '');
    var base = '/campaigns/' + encodeURIComponent(cid) + '/crafting';
    var D = null, open = null, pick = {}, last = null, got = null, busy = false, dead = false, benchEl = null, toastT = 0;

    /* JSON in and out; a refusal rejects with the server's own words. */
    function api(method, path, body) {
      return Chronicle.apiFetch(base + path, {method: method, body: body}).then(function (res) {
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
    }
    function toast(msg) {
      var t = el.querySelector('.cra-toast');
      if (!t) return;
      t.textContent = msg; t.hidden = false;
      clearTimeout(toastT);
      toastT = setTimeout(function () { t.hidden = true; }, 4000);
    }
    function isOpen(kind, id) { return !!(open && open.kind === kind && open.id === id); }
    function recipe(id) { for (var i = 0; i < D.recipes.length; i++) if (D.recipes[i].id === id) return D.recipes[i]; return null; }
    function project(id) { for (var i = 0; i < D.projects.length; i++) if (D.projects[i].id === id) return D.projects[i]; return null; }
    function hero() { return D.character.name; }
    function bench(kind) { for (var i = 0; i < D.benches.length; i++) if (D.benches[i].kind === kind) return D.benches[i]; return null; }
    /* Skipping the steps is the campaign's call for a player; a recipe with
       no steps is only ever rolled. */
    function canSkip(r) { return !r || !r.steps.length || D.settings.skip || D.director; }

    function projectRow(p) {
      var r = recipe(p.recipeId), acts, id = p.id;
      if (p.status === 'made') acts = '<span class="crp-pill is-ok">Made</span>';
      else if (p.rollsReady > 0 && p.waits) acts = '<span class="crp-why is-block">' + esc(waitText(p, D.today)) + '</span>' + menuBtn(id, isOpen('menu', id));
      else if (p.rollsReady > 0) {
        var steps = r && r.steps.length;
        acts = '<span class="crp-pill is-ok">' + (p.rollsReady > 1 ? plural(p.rollsReady, 'roll', 'rolls') + ' ready' : 'Roll ready') + '</span>' +
          (steps ? '<button type="button" class="btn btn-primary btn-sm" data-act="bench" data-id="' + esc(id) + '" aria-expanded="' + isOpen('bench', id) + '">Work at the bench</button>' : '') +
          (canSkip(r) ? '<button type="button" class="btn ' + (steps ? 'btn-secondary' : 'btn-primary') + ' btn-sm" data-act="plain" data-id="' + esc(id) + '">' + (steps ? 'Just roll' : 'Roll') + '</button>' : '') +
          menuBtn(id, isOpen('menu', id));
      } else acts = '<span class="crp-why">Next roll at the next respite</span>' + menuBtn(id, isOpen('menu', id));
      if (isOpen('menu', id)) acts += '<div class="crp-menu" role="menu"><button type="button" role="menuitem" class="is-bad" data-act="askcancel" data-id="' + esc(id) + '">Cancel the project</button></div>';
      var h = '<li data-p="' + esc(id) + '"><div class="crp-row is-pr"><div class="crp-nm"><b>' + esc(p.name) + '</b><span class="cra-fine">' + esc([p.kind, p.roll].filter(Boolean).join(' · ')) + '</span></div>' +
        progress(p.points, p.goal, '', p.name) + '<div class="crp-acts">' + acts + '</div></div>';
      if (isOpen('bench', id) && r) h += benchFold(p, r);
      if (isOpen('result', id) && last) h += resultFold(p);
      if (isOpen('confirm', id)) h += '<div class="crp-confirm"><p>Cancel ' + esc(p.name) + '? What it took stays spent. The Director can give things back with <b>Give an item</b>.</p>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="close" data-id="' + esc(id) + '">Keep it</button><button type="button" class="btn btn-danger btn-sm" data-act="cancel" data-id="' + esc(id) + '">Cancel the project</button></div>';
      return h + '</li>';
    }

    function recipeRow(r) {
      var why = blockers(r, D.pack, hero()), on = isOpen('start', r.id);
      var mats = r.needs.map(function (m) { return m.qty + ' × ' + m.name; }).concat(r.cost ? [costText(r, D.pack)] : []).join(' · ');
      var h = '<li><div class="crp-row"><div class="crp-nm"><b>' + esc(r.name) + '</b><span class="cra-fine">' + esc(r.kind + ' · goal ' + r.goal + ' · ' + r.roll) + '</span></div>' +
        '<div class="cra-fine">' + esc(mats || 'No materials') + '</div><div class="crp-acts">' + (why.length ? '<span class="crp-why is-block">' + esc(why[0]) + '</span>' : '') +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="start" data-id="' + esc(r.id) + '" aria-expanded="' + on + '"' + (why.length ? ' disabled' : '') + '>Start</button></div></div>';
      if (on && !why.length) h += startFold(r);
      return h + '</li>';
    }

    function startFold(r) {
      var who = esc(hero()), lines = r.needs.map(function (m, i) {
        if (m.itemId && m.sub) {
          var real = held(D.pack, m.itemId) >= reserved(r, m.itemId), subs = standIns(D.pack, r, m);
          if (pick[i] == null || (pick[i] && !subs.some(function (o) { return o.id === pick[i]; })) || (!pick[i] && !real)) pick[i] = real ? '' : subs[0] && subs[0].id;
          return '<li><span class="crp-k">' + m.qty + ' × ' + esc(m.name) + '<small><label for="crk-pick-' + i + '">Or something in its place, ' + esc(SUB_COST[m.sub.costs] || '') + '</label></small></span><span class="crp-v">' +
            '<select class="cra-in cra-sm" id="crk-pick-' + i + '" data-pick="' + i + '">' + (real ? '<option value=""' + (!pick[i] ? ' selected' : '') + '>' + esc(m.name) + ' (' + held(D.pack, m.itemId) + ')</option>' : '') +
            subs.map(function (o) { return '<option value="' + esc(o.id) + '"' + (o.id === pick[i] ? ' selected' : '') + '>' + esc(o.name) + ' (' + o.qty + '), in its place</option>'; }).join('') + '</select></span></li>';
        }
        if (m.itemId) return '<li><span class="crp-k">' + m.qty + ' × ' + esc(m.name) + '<small>from ' + who + '’s pack, which has ' + held(D.pack, m.itemId) + '</small></span><span class="crp-v is-ok">' + ICON.check + '</span></li>';
        var opts = tagged(D.pack, r, m.tag);
        if (!pick[i] || !opts.some(function (o) { return o.id === pick[i]; })) pick[i] = opts[0] && opts[0].id;
        return '<li><span class="crp-k">' + m.qty + ' ' + esc(m.name) + '<small><label for="crk-pick-' + i + '">Pick which one from ' + who + '’s pack</label></small></span><span class="crp-v">' +
          '<select class="cra-in cra-sm" id="crk-pick-' + i + '" data-pick="' + i + '">' + opts.map(function (o) {
            return '<option value="' + esc(o.id) + '"' + (o.id === pick[i] ? ' selected' : '') + '>' + esc(o.name) + ' (' + o.qty + ')</option>';
          }).join('') + '</select></span></li>';
      }).join('');
      var pk = D.pack.payKind;
      if (r.cost && pk === 'wealth') lines += '<li><span class="crp-k">Cost<small>Wealth is a threshold, as at a shop, so nothing is spent</small></span><span class="crp-v">Wealth ' + num(r.charge) + ' <span class="is-ok">· ' + who + ' has ' + num(D.pack.have) + '</span></span></li>';
      else if (r.cost) lines += '<li><span class="crp-k">Cost<small>' + who + ' has ' + num(D.pack.have) + ' ' + esc(unit(r, D.pack)) + ', so ' + num(D.pack.have - r.charge) + ' ' + esc(unit(r, D.pack)) + ' will be left</small></span><span class="crp-v">' + esc(L.money(r.cost)) + '</span></li>';
      lines += '<li><span class="crp-k">The project<small>One roll at each respite. The first comes with the next respite.</small></span><span class="crp-v">Goal ' + r.goal + ' · ' + esc(r.roll) + (r.statValue != null ? ' (' + r.statValue + ')' : '') + '</span></li>';
      var coins = r.cost && pk && pk !== 'wealth';
      return '<div class="crp-fold"><h4>Start ' + esc(r.name) + '</h4><ul class="crp-lines">' + lines + '</ul>' +
        '<div class="crp-foot"><p class="cra-fine">The materials' + (coins ? ' and the coins' : '') + ' leave ' + who + '’s pack now and show in the Armory’s history.</p>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="close" data-id="' + esc(r.id) + '">Not now</button>' +
        '<button type="button" class="btn btn-primary btn-sm" data-act="begin" data-id="' + esc(r.id) + '"' + (busy ? ' disabled' : '') + '>Start the project</button></div></div>';
    }

    function benchFold(p, r) {
      var b = bench(r.bench), E = D.edge, B = D.bane;
      return '<div class="crp-fold"><h4>' + esc(p.name) + (b ? ' at ' + esc(b.name) : ' at the bench') + '</h4>' +
        '<div class="crp-bench" data-bench></div>' +
        '<div class="crp-foot"><p class="cra-fine">Clean work earns an edge (' + signed(E) + '). ' +
        (D.settings.bane ? 'Botched work gives a bane (' + signed(B) + ').' : 'Botched work makes a plain roll, because banes are off.') +
        ' Putting the bench away keeps the roll for later.</p>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="close" data-id="' + esc(p.id) + '">Put it away</button></div></div>';
    }

    function resultFold(p) {
      var R = last, rows = T.terms(R), h = '<div class="crp-fold" aria-live="polite"><h4>Project roll: ' + esc(p.name) + '</h4><div class="crp-result">' +
        (R.dice.length ? '<div class="crp-dice" aria-label="The dice show ' + esc(R.dice.join(' and ')) + '">' + R.dice.map(function (d) { return '<span class="crp-die">' + d + '</span>'; }).join('') + '</div>' : '<div></div>') +
        '<ul class="crp-terms">' + rows.map(function (t) { return '<li><span>' + esc(t.text) + '</span><span>' + esc(t.n) + '</span></li>'; }).join('') +
        '<li class="is-tot"><span>Project points</span><span>+' + R.total + '</span></li></ul>' +
        '<div class="crp-outcome">' + progress(R.to, p.goal, R.from + ' → ' + R.to, p.name);
      h += befell(R, p.name, hero());
      if (R.finished && R.made) h += '<div class="crp-made">' + ICON.check + '<span>' + esc(p.name) + ' is made. ' + esc(R.made) + ' is in ' + esc(hero()) + '’s pack.</span></div>';
      else if (R.finished) h += '<div class="crp-made is-warn"><span>' + esc(p.name) + ' is made.</span></div>';
      else if (!R.lost) h += '<p class="cra-fine">' + (p.goal - R.to) + ' more to go. ' + (p.rollsReady > 0 ? 'You have another roll ready.' : 'The next roll comes with the next respite.') + '</p>';
      return h + '</div></div><div class="crp-foot"><p class="cra-fine">' + esc(R.notes.join(' ')) + '</p>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="close" data-id="' + esc(p.id) + '">Done</button></div></div>';
    }

    /* ---------- The spoils tray ---------- */

    function spoilActs(l) {
      if (got && isOpen('harvest', String(l.id))) return '<span class="crp-pill is-ok">Harvested</span>';
      var harvest = l.mode === 'harvest';
      return '<button type="button" class="btn ' + (harvest ? 'btn-primary' : 'btn-secondary') + ' btn-sm" data-act="take" data-id="' + l.id + '"' + (busy ? ' disabled' : '') +
        ' aria-label="' + esc((harvest ? 'Harvest ' : 'Take ') + l.name + ' for ' + hero()) + '">' + (harvest ? 'Harvest' : 'Take') + '</button>';
    }

    function harvestFold(l) {
      return got && isOpen('harvest', String(l.id)) ? T.result(got, l.id, 'Harvest: ' + got.item) : '';
    }

    function spoils() {
      if (!D.spoils || !D.spoils.length) return '';
      return '<p class="cra-fine">Spoils from the party’s fights wait here until someone takes them. Take a thing as it is, or harvest it with a test that adds a characteristic from ' +
        esc(hero()) + '’s sheet: the better the roll, the more ' + esc(hero()) + ' gets.</p><h4 class="crp-sub">Spoils to take</h4><div class="crs-tray">' +
        D.spoils.map(function (sp) {
          return T.card(sp, {acts: spoilActs, fold: harvestFold, hit: function (l) { return got && isOpen('harvest', String(l.id)) ? got.tier || 0 : -1; }});
        }).join('') + '</div>';
    }

    function render() {
      unmountBench();
      var ready = 0, waiting = T.count(D.spoils);
      D.projects.forEach(function (p) { if (p.status === 'active' && !p.waits) ready += p.rollsReady; });
      var h = '<div class="cra crp"><div class="crp-head"><h3 class="cb-head"><i class="fa-solid fa-flask" aria-hidden="true"></i> Crafting</h3>' +
        (waiting ? '<span class="crp-pill">' + waiting + ' to take</span>' : '') +
        (ready ? '<span class="crp-pill is-ok">' + plural(ready, 'roll', 'rolls') + ' ready</span>' : '') + '</div>' + spoils() +
        '<p class="cra-fine">Each project gets one roll for every respite the Director logs.' + (D.today ? ' Today is ' + esc(D.today) + '.' : '') + '</p>' +
        '<h4 class="crp-sub">In progress</h4><ul class="crp-rows">' +
        (D.projects.length ? D.projects.map(projectRow).join('') : '<li class="crp-empty">No projects yet. Start one from a recipe below.</li>') + '</ul>' +
        '<h4 class="crp-sub">Recipes ' + esc(hero()) + ' knows</h4><ul class="crp-rows">' +
        (D.recipes.length ? D.recipes.map(recipeRow).join('') : '<li class="crp-empty">' + esc(hero()) + ' knows no recipes yet. The Director teaches them in the Crafting app.</li>') + '</ul>' +
        '<div class="cra-toast" role="status" hidden></div></div>';
      el.innerHTML = h;
      if (open && open.kind === 'bench') mountBench(open.id);
    }

    function load(keep) {
      return api('GET', '/characters/' + encodeURIComponent(eid)).then(function (got) {
        if (dead) return;
        var shown = keep && open && open.kind === 'result' && last && !got.projects.some(function (p) { return p.id === last.project.id; });
        if (shown) {
          var at = 0;
          D.projects.forEach(function (p, i) { if (p.id === last.project.id) at = i; });
          got.projects.splice(Math.min(at, got.projects.length), 0, last.project);
        }
        D = got;
        if (!keep) open = null;
        render();
      });
    }
    function fail(err) {
      if (dead) return;
      el.innerHTML = '<div class="cra-empty cra-bad" role="alert">Crafting couldn’t load: ' + esc(err.message || 'something went wrong') +
        '. <button type="button" class="cra-link" data-retry>Try again</button></div>';
      el.querySelector('[data-retry]').addEventListener('click', function () { load().catch(fail); });
    }

    function unmountBench() {
      if (!benchEl) return;
      if (Chronicle.destroyWidget) Chronicle.destroyWidget(benchEl);
      benchEl.remove(); benchEl = null;
    }
    /* The campaign's bench of the recipe's kind, as the Director arranged it
       for the campaign's look, playing this one recipe. When the steps are
       done it hands back how they went and the server makes the roll. */
    function mountBench(pid) {
      var p = project(pid), r = p && recipe(p.recipeId), box = el.querySelector('[data-bench]');
      if (!p || !r || !box || !window.Chronicle.mountWidget) return;
      var b = bench(r.bench), skin = D.settings.skin === 'journal' ? 'journal' : 'desk', data = L.benchRecipe(r, hero(), p.points);
      benchEl = document.createElement('div');
      benchEl.setAttribute('data-widget', 'craft_bench');
      benchEl.setAttribute('data-skin', skin);
      if (D.calendarId) { benchEl.setAttribute('data-campaign-id', cid); benchEl.setAttribute('data-calendar-id', D.calendarId); }
      benchEl.innerHTML = '<p class="cra-fine" role="status">Loading the bench…</p>';
      benchEl._craftApp = {state: b && b[skin] && typeof b[skin] === 'object' ? b[skin] : null, recipes: {work: data.recipe}, items: data.items, recipe: 'work',
        settings: {difficulty: D.settings.difficulty, bane: D.settings.bane, skip: false},
        onWorked: function (work) { roll(pid, work, b ? b.id : ''); }};
      box.appendChild(benchEl);
      Chronicle.mountWidget(benchEl);
    }

    /* The server's dice, thrown on screen (CraftingTray.throwDice). */
    function throwDice(res, name, said) { T.throwDice(el.querySelector('.crp-result'), res, name, said); }

    function focus(sel) { setTimeout(function () { var f = el.querySelector(sel); if (f) f.focus(); }, 0); }

    function roll(pid, work, benchId) {
      var p = project(pid);
      if (!p || busy) return;
      busy = true;
      api('POST', '/projects/' + encodeURIComponent(pid) + '/roll', {work: work, bench: benchId || '', version: p.version}).then(function (res) {
        busy = false;
        if (dead) return;
        last = res;
        D.projects = D.projects.map(function (x) { return x.id === pid ? res.project : x; });
        open = {kind: 'result', id: pid};
        render();
        throwDice(res, p.name);
        focus('[data-p="' + pid + '"] [data-act="close"]');
        // A finished project changed the pack, and a roll can change what waits.
        load(true).catch(function () {});
      }, function (err) {
        busy = false;
        if (dead) return;
        toast(err.message);
        // A held roll or a stale project: show where things stand now.
        if (err.status === 409) load().catch(fail);
        else if (benchEl && benchEl._crb && benchEl._crb.retry) benchEl._crb.retry();
      });
    }

    /* Takes a spoil for this hero; a harvest shows its test where the line
       was until Done, then the tray reloads. */
    function take(lid) {
      if (busy) return;
      busy = true;
      render();
      api('POST', '/spoil-lines/' + encodeURIComponent(lid) + '/take', {characterId: eid}).then(function (res) {
        busy = false;
        if (dead) return;
        if (!res.dice || !res.dice.length) {
          return load().then(function () { toast(res.got === 'nothing' ? 'That paid nothing.' : res.got + ' went to ' + res.hero + '’s pack.'); });
        }
        got = res;
        open = {kind: 'harvest', id: String(lid)};
        render();
        throwDice(res, 'Harvest: ' + res.item, 'a test of ' + res.total);
        focus('[data-act="sdone"]');
      }, function (err) {
        busy = false;
        if (dead) return;
        toast(err.message);
        load().catch(fail);
      });
    }

    function begin(rid) {
      var r = recipe(rid);
      if (!r || busy) return;
      var picks = {};
      r.needs.forEach(function (m, i) { if ((!m.itemId || m.sub) && pick[i]) picks[i] = pick[i]; });
      busy = true;
      render();
      api('POST', '/characters/' + encodeURIComponent(eid) + '/projects', {recipeId: rid, picks: picks}).then(function (pv) {
        busy = false;
        return load().then(function () {
          toast(r.name + ' started. ' + (pv.took ? pv.took + ' left ' + hero() + '’s pack.' : '') + (r.cost && D.pack.payKind === 'wealth' ? ' Wealth is never spent.' : ''));
        });
      }, function (err) { busy = false; render(); toast(err.message); });
    }

    function cancel(pid) {
      var p = project(pid);
      if (!p) return;
      api('DELETE', '/projects/' + encodeURIComponent(pid)).then(function () {
        return load().then(function () { toast(p.name + ' was cancelled. What it took stays spent.'); });
      }, function (err) { toast(err.message); load().catch(fail); });
    }

    function close(id) {
      var p = project(id);
      if (p && p.status !== 'active') D.projects = D.projects.filter(function (x) { return x !== p; });
      open = null; render();
      focus('[data-id="' + id + '"][data-act]');
    }

    function onClick(e) {
      var b = e.target.closest('[data-act]');
      if (!b || !el.contains(b) || b.closest('[data-bench]') || !D) return;
      var act = b.getAttribute('data-act'), id = b.getAttribute('data-id');
      switch (act) {
        case 'menu': open = isOpen('menu', id) ? null : {kind: 'menu', id: id}; render(); focus('.crp-menu button'); break;
        case 'start': open = isOpen('start', id) ? null : {kind: 'start', id: id}; pick = {}; render(); focus('[data-act="begin"]'); break;
        case 'begin': begin(id); break;
        case 'bench': open = isOpen('bench', id) ? null : {kind: 'bench', id: id}; render(); break;
        case 'plain': roll(id, 'skipped', ''); break;
        case 'close': close(id); break;
        case 'askcancel': open = {kind: 'confirm', id: id}; render(); focus('[data-act="cancel"]'); break;
        case 'cancel': open = null; cancel(id); break;
        case 'take': take(id); break;
        case 'sdone': open = null; got = null; load().catch(fail); break;
      }
    }
    function onChange(e) {
      var s = e.target.closest('[data-pick]');
      if (s) pick[s.getAttribute('data-pick')] = s.value;
    }
    function outside(e) {
      if (open && open.kind === 'menu' && !e.target.closest('.crp-menu, [data-act="menu"]')) { open = null; render(); }
    }
    function onKey(e) {
      if (e.key === 'Escape' && open && open.kind === 'menu') { var id = open.id; open = null; render(); focus('[data-act="menu"][data-id="' + id + '"]'); }
    }
    el.addEventListener('click', onClick);
    el.addEventListener('change', onChange);
    document.addEventListener('click', outside);
    document.addEventListener('keydown', onKey);

    el.innerHTML = '<div class="cra-loading" aria-hidden="true"><i></i><i></i><i></i></div><p class="sr-only" role="status">Loading crafting…</p>';
    load().catch(fail);
    return {destroy: function () { dead = true; clearTimeout(toastT); unmountBench(); el.removeEventListener('click', onClick); el.removeEventListener('change', onChange); document.removeEventListener('click', outside); document.removeEventListener('keydown', onKey); el.innerHTML = ''; }};
  }

  Chronicle.register('crafting-block', {
    init: function (el, cfg) {
      if (!window.CraftingLib || !window.CraftingTray) {
        el.innerHTML = '<p class="text-sm text-fg-muted" role="status">Crafting couldn’t load. Reload the page to try again.</p>';
        return;
      }
      el._crk = mount(el, cfg || {});
    },
    destroy: function (el) {
      if (el._crk) { el._crk.destroy(); el._crk = null; }
    }
  });

  window.CraftingBlock = {held: held, tagged: tagged, blockers: blockers, waitText: waitText, termRows: T.terms, costText: costText, befell: befell, standIns: standIns};
})();
