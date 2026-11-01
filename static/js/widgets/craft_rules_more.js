/**
 * craft_rules_more.js -- the "If it goes wrong" and "Extras" tabs of the
 * crafting rules editor (window.CraftRulesMore)
 *
 * Each mechanic is a card: a choice, or a switch that opens it to a sentence
 * with blanks. Everything edits S.rec().x, the recipe's extras in the form
 * the AI writes and the server stores (CraftRules.cleanExtras), and calls
 * S.changed('extras') so the crafting app saves it with the rules.
 *
 * Quirks and mishaps roll on the campaign's own Rolling Tables, by name
 * (S.W.lists.tables, with rows in S.W.lists.tableRows). The tables are edited
 * on their own page, so here their rows are shown and can be tried, not
 * changed. A setting the project roll can't carry out yet (it needs an item
 * to remember something of its own, or a player action that doesn't exist)
 * is saved and labelled "Not yet carried out at the roll".
 */
(function () {
  'use strict';
  var R = window.CraftRules;

  function esc(s) { return R.esc(s); }
  function fa(n) { return '<i class="fa-solid fa-' + n + '" aria-hidden="true"></i>'; }
  function hl(o) { return window.CraftRulesEditor ? CraftRulesEditor.hl(o) : esc(JSON.stringify(o, null, 2)); }
  var NOTYET = '<span class="crr-tag2">Not yet carried out at the roll</span>';
  var DIES = [4, 6, 8, 10, 12, 20];

  function withCur(list, cur) { return cur && list.indexOf(cur) < 0 ? list.concat([cur]) : list; }
  function chips(path, opts, val) {
    return '<span class="crr-chips crr-inl">' + opts.map(function (o) {
      var v = Array.isArray(o) ? o[0] : o, l = Array.isArray(o) ? o[1] : o;
      return '<button type="button" class="crr-cx" data-x="' + esc(path) + '" data-v="' + esc(v) + '" aria-pressed="' + (String(val) === String(v)) + '">' + esc(l) + '</button>';
    }).join('') + '</span>';
  }
  function sw(path, on, label) {
    return '<label class="crr-sw"><input type="checkbox" data-sw="' + esc(path) + '"' + (on ? ' checked' : '') + ' aria-label="' + esc(label) + '"></label>';
  }
  function card(path, on, tone, icon, title, desc, body, tag) {
    return '<div class="crr-xc ' + tone + (on ? ' is-on' : '') + '"><div class="crr-xh"><span class="crr-xi">' + fa(icon) + '</span><span class="crr-xt"><b>' + esc(title) + (tag || '') +
      '</b><small>' + esc(desc) + '</small></span>' + (path ? sw(path, on, title) : '') + '</div><div class="crr-xb"><div><div class="crr-xbi">' + body + '</div></div></div></div>';
  }

  /* What switching a setting on starts it as: the editor's defaults. */
  function fresh(k, S) {
    var x = S.rec().x, tables = S.W.lists.tables || [], n = S.rec().ings.length;
    switch (k) {
      case 'mishaps': return {chance: '1 in 6', table: tables[0] || ''};
      case 'quality': return {fine: 'edge', poor: 'bane'};
      case 'mastery': return {afterCleanBrews: 3, gives: 'skip'};
      case 'discover': return {closeWhen: n > 1 ? '1 of ' + n : '', learnedBy: 'maker', failedTry: 'used'};
      case 'keeps': return {days: 30, thenBecomes: (S.W.lists.items || [])[0] || ''};
      case 'salvage': return [{item: (S.rec().ings[0] || [0, ''])[1], qty: 1}];
    }
    return x[k];
  }

  function mount(root, S, which) {
    var dead = false, tm = null;
    function rec() { return S.rec(); }
    function rows(name) { return ((S.W.lists.tableRows || {})[name]) || null; }
    function table(name, mishap) {
      var list = rows(name), l = rec().looks;
      if (!list) return '<p class="crr-hint">' + fa('triangle-exclamation') + ' There is no Rolling Table called “' + esc(name) + '”. Make it on the Rolling Tables page, or pick another.</p>';
      if (!list.length) return '<p class="crr-hint">“' + esc(name) + '” has no rows yet. Add them on the Rolling Tables page.</p>';
      return '<ol class="crr-dtable" data-table="' + esc(name) + '">' + list.map(function (t, i) {
        var lk = mishap && l['mishap:' + i], dots = Object.keys((lk && lk.spots) || {}).map(function (k) {
          var s = lk.spots[k], L = s.look && window.ItemLookFX && ItemLookFX.LOOKS[s.look];
          return L ? '<i style="background:' + esc(L.color) + '"></i>' : s.move ? '<i class="mv"></i>' : '';
        }).join('');
        return '<li data-row="' + i + '"><span class="crr-face">' + (i + 1) + '</span><span class="crr-rowt">' + esc(t) + '</span>' +
          (mishap && i < R.EXTRAS.limits.mishapMoments ? '<button type="button" class="btn btn-secondary btn-sm" data-watch="' + i + '" aria-label="Watch mishap ' + (i + 1) + ' on the bench"><span class="crr-dl">' + dots + '</span>' + fa('play') + 'Watch</button>' : '') + '</li>';
      }).join('') + '</ol><div class="crr-dice"><span class="crr-hint">These rows are the campaign’s Rolling Table. Change them on the Rolling Tables page.</span><span class="crr-grow"></span>' +
        (DIES.indexOf(list.length) < 0 ? '<span class="crr-tag2">' + list.length + ' rows</span>' : '') +
        '<span class="crr-die" aria-hidden="true">' + list.length + '</span><button type="button" class="btn btn-secondary btn-sm" data-roll>' + fa('dice') + 'Try a roll</button></div>';
    }
    function tablePick(path, cur) {
      var names = (S.W.lists.tables || []).slice();
      if (cur && names.indexOf(cur) < 0) names.push(cur);
      return names.length ? chips(path, names, cur) : '<span class="crr-hint">The campaign has no Rolling Tables yet.</span>';
    }
    function wrong() {
      var x = rec().x, cost = x.botch.costs, h = '<div class="crr-pane"><div class="crr-sech"><h4>When the work is botched</h4><small>Two or more misses at the bench, when banes are on for this campaign.</small></div><div class="crr-rcards">';
      [['bane', 'Only the bane', 'The roll takes a bane. Nothing else is lost.', 'arrow-down'], ['baneAndQuirk', 'A bane and a quirk', 'It still works, with something odd about it, rolled on a table.', 'shuffle'],
        ['baneAndIngredients', 'A bane, and the ingredients are gone', 'Harsh: they start over.', 'ban']].forEach(function (o) {
        h += '<button type="button" class="crr-rcard crr-risk" data-botch="' + o[0] + '" aria-pressed="' + (cost === o[0]) + '"><b>' + fa(o[3]) + esc(o[1]) + '</b><small>' + esc(o[2]) + '</small></button>';
      });
      h += '</div>';
      if (cost === 'baneAndQuirk') h += '<div class="crr-box" data-box="quirk"><div class="crr-sech"><h4>Quirk table</h4>' + tablePick('botch.quirkTable', x.botch.quirkTable) + '</div>' + (x.botch.quirkTable ? table(x.botch.quirkTable, false) : '') + '</div>';
      h += '<div class="crr-sech"><h4>Mishaps mid-brew</h4><small>Something goes wrong while they work. Each one has a moment on How it looks; press Watch to see it there.</small><span class="crr-r">' + sw('mishaps', !!x.mishaps, 'Mishaps can strike mid-brew') + '</span></div>';
      if (x.mishaps) h += '<div class="crr-box" data-box="mishap"><p class="crr-sent">Each roll has a ' + chips('mishaps.chance', R.CHANCES.slice(1), x.mishaps.chance) + ' chance of one from ' + tablePick('mishaps.table', x.mishaps.table) + '</p>' +
        (x.mishaps.table ? table(x.mishaps.table, true) : '') + '</div>';
      else h += '<p class="crr-hint">Off: brewing never goes wrong partway.</p>';
      return h + '<div class="crr-aicap crr-block">What the AI writes</div><pre class="crr-aij crr-block">' + hl(data('wrong')) + '</pre></div>';
    }
    function extras() {
      var r = rec(), x = r.x, n = r.ings.length, ings = r.ings.map(function (g, i) { return [String(i), g[1]]; }), tags = (S.W.lists.tags || R.EXTRAS.tags.map(function (k) { return {key: k, label: k.replace(/_/g, ' ')}; }));
      var h = '<div class="crr-pane"><div class="crr-sech"><h4>Extras</h4><small>Turn on what you want for this recipe. Everything can be turned off and nothing is required.</small></div><div class="crr-xgrid">';
      h += card('quality', !!x.quality, 'crr-help', 'gem', 'Ingredient quality', 'Harvested parts can be fine or poor.', x.quality ? '<p class="crr-sent">A fine part gives ' +
        chips('quality.fine', [['edge', 'an edge'], ['points', '+2 points'], ['extra', 'one extra'], ['none', 'nothing']], x.quality.fine) + '</p><p class="crr-sent">A poor one gives ' +
        chips('quality.poor', [['bane', 'a bane'], ['points', '−2 points'], ['none', 'nothing']], x.quality.poor) + '</p>' : '', NOTYET);
      h += card('', true, 'crr-change', 'right-left', 'Substitutes', 'For when they’re missing an ingredient.', (x.substitutes.length ? x.substitutes.map(function (s, i) {
        return '<p class="crr-sent">' + chips('substitutes.' + i + '.for', ings, String(s['for'])) + ' can be swapped for ' +
          chips('substitutes.' + i + '.use', tags.map(function (t) { return [t.key, 'any ' + String(t.label).toLowerCase()]; }), s.use) + ' at ' +
          chips('substitutes.' + i + '.costs', [['bane', 'a bane'], ['points', '−2 points'], ['none', 'no cost']], s.costs) +
          '<button type="button" class="crr-ib" data-rm-sub="' + i + '" aria-label="Remove this substitute">' + fa('xmark') + '</button></p>';
      }).join('') : '<p class="crr-hint">None yet: they need the real thing.</p>') +
        (x.substitutes.length < Math.min(n, R.EXTRAS.limits.subs) ? '<div><button type="button" class="btn btn-secondary btn-sm" data-add-sub>' + fa('plus') + 'Add a substitute</button></div>' : ''));
      h += card('breakthrough', x.breakthrough, 'crr-help', 'star', 'Breakthroughs', 'A natural 19 or 20: roll again and add it.', '<p class="crr-hint">Its moment is on How it looks.</p>');
      h += card('mastery', !!x.mastery, 'crr-help', 'user-graduate', 'Mastery', 'Practice pays off, per character.', x.mastery ? '<p class="crr-sent">After ' +
        chips('mastery.afterCleanBrews', R.EXTRAS.after.map(String), String(x.mastery.afterCleanBrews)) + ' clean brews, they ' +
        chips('mastery.gives', [['skip', 'can skip the steps'], ['edge', 'get an edge'], ['points', 'get +2 points']], x.mastery.gives) + '</p>' : '');
      var close = [];
      for (var i = 1; i < n; i++) close.push(i + ' of ' + n);
      h += card('discover', !!x.discover, 'crr-change', 'flask-vial', 'Finding it by experimenting', 'Players can stumble on it without the recipe.', x.discover ?
        (close.length ? '<p class="crr-sent">They come close with ' + chips('discover.closeWhen', close, x.discover.closeWhen) + ' ingredients right. Then the book says:</p>' : '') +
        '<input class="crr-in" data-xt="discover.hint" maxlength="' + R.EXTRAS.limits.hint + '" value="' + esc(x.discover.hint || '') + '" placeholder="A hint, in the book’s voice" aria-label="The hint">' +
        '<p class="crr-sent">When they find it, ' + chips('discover.learnedBy', [['maker', 'the maker learns it'], ['party', 'the whole party learns it']], x.discover.learnedBy) + '</p>' +
        '<p class="crr-sent">A try that finds nothing ' + chips('discover.failedTry', [['used', 'uses up what they put in'], ['half', 'uses up half'], ['kept', 'keeps everything']], x.discover.failedTry) + '</p>' +
        '<p class="crr-hint">Each hero gets one try at each respite. With this off, no one can find it by experimenting.</p>' : '');
      h += card('keeps', !!x.keeps, 'crr-risk', 'hourglass-half', 'It goes off', 'Counted on the campaign calendar, so long journeys matter.', x.keeps ? '<p class="crr-sent">It keeps for <span class="crr-stepper">' +
        '<button type="button" data-step="keeps.days" data-v="-1" aria-label="One day fewer">' + fa('minus') + '</button><output>' + x.keeps.days + '</output><button type="button" data-step="keeps.days" data-v="1" aria-label="One day more">' + fa('plus') +
        '</button></span> days, then becomes ' + chips('keeps.thenBecomes', withCur((S.W.lists.items || []).slice(0, 12), x.keeps.thenBecomes), x.keeps.thenBecomes) + '</p>' : '', NOTYET);
      h += card('makersMark', x.makersMark, 'crr-change', 'signature', 'Maker’s mark', 'The item says who made it, and when.', '<p class="crr-hint">“Made by the hero who brewed it, on the calendar’s date.”</p>', NOTYET);
      h += card('salvage', x.salvage.length > 0, 'crr-change', 'recycle', 'Breaking it down', 'They can take it apart for something back.', x.salvage.length ? '<p class="crr-sent">It gives back ' +
        chips('salvage.0.qty', ['1', '2'], String(x.salvage[0].qty)) + ' × ' + chips('salvage.0.item', r.ings.map(function (g) { return g[1]; }), x.salvage[0].item) + '</p>' : '');
      h += card('othersCanHelp', x.othersCanHelp, 'crr-when', 'people-group', 'Others can help', 'Party members can lend a hand on it.',
        x.othersCanHelp ? '<p class="crr-hint">Each party member can add one extra roll to a hero’s project at each respite, with their own characteristic.</p>' : '');
      return h + '</div><p class="crr-hint">Who knows the recipe is set on the recipe form, beside its ingredients.</p><div class="crr-aicap crr-block">What the AI writes</div><pre class="crr-aij crr-block">' + hl(data('extras')) + '</pre></div>';
    }
    function data(k) {
      var x = rec().x;
      if (k === 'wrong') return {botch: x.botch, mishaps: x.mishaps};
      var o = R.clone(x); delete o.botch; delete o.mishaps;
      return o;
    }
    /* set writes one value by its path; numbers stay numbers. */
    function set(path, v) {
      var ks = path.split('.'), last = ks.pop(), o = rec().x;
      ks.forEach(function (k) { o = o[k]; });
      o[last] = typeof o[last] === 'number' ? +v : v;
    }
    function draw(focus) {
      root.innerHTML = which === 'wrong' ? wrong() : extras();
      if (focus) { var f = root.querySelector(focus); if (f) f.focus(); }
    }
    function changed(focus) {
      var r = rec();
      r.x = R.cleanExtras(r.x, r.ings.length);
      draw(focus);
      S.changed('extras');
    }
    function tryRoll(btn) {
      var box = btn.closest('.crr-box'), list = box && box.querySelector('.crr-dtable'), die = box && box.querySelector('.crr-die');
      if (!list || !die) return;
      var lis = list.querySelectorAll('li'), hit = Math.floor(Math.random() * lis.length);
      Array.prototype.forEach.call(lis, function (li) { li.classList.remove('is-hit'); });
      die.classList.remove('is-roll'); void die.offsetWidth; die.classList.add('is-roll');
      clearTimeout(tm);
      tm = setTimeout(function () {
        if (dead) return;
        die.textContent = hit + 1; lis[hit].classList.add('is-hit');
        S.say('Rolled ' + (hit + 1) + ': ' + lis[hit].querySelector('.crr-rowt').textContent);
      }, S.calm() ? 0 : 700);
    }
    function onClick(e) {
      var t = e.target.closest('button');
      if (!t || !root.contains(t)) return;
      var x = rec().x;
      if (t.hasAttribute('data-botch')) {
        var c = t.getAttribute('data-botch');
        x.botch = c === 'baneAndQuirk' ? {costs: c, quirkTable: x.botch.quirkTable || (S.W.lists.tables || [])[0] || ''} : {costs: c};
        changed('[data-botch="' + c + '"]');
      } else if (t.hasAttribute('data-x')) {
        set(t.getAttribute('data-x'), t.getAttribute('data-v'));
        changed('[data-x="' + t.getAttribute('data-x') + '"][data-v="' + t.getAttribute('data-v') + '"]');
      } else if (t.hasAttribute('data-step')) {
        x.keeps.days = Math.max(1, Math.min(R.EXTRAS.limits.days, x.keeps.days + (+t.getAttribute('data-v'))));
        changed('[data-step][data-v="' + t.getAttribute('data-v') + '"]');
      } else if (t.hasAttribute('data-watch')) S.goLook('mishap:' + t.getAttribute('data-watch'));
      else if (t.hasAttribute('data-roll')) tryRoll(t);
      else if (t.hasAttribute('data-rm-sub')) { x.substitutes.splice(+t.getAttribute('data-rm-sub'), 1); changed('[data-add-sub]'); }
      else if (t.hasAttribute('data-add-sub')) {
        var used = x.substitutes.map(function (s) { return s['for']; }), f = 0;
        while (used.indexOf(f) >= 0) f++;
        x.substitutes.push({'for': f, use: R.EXTRAS.tags[0], costs: 'bane'});
        changed('[data-rm-sub="' + (x.substitutes.length - 1) + '"]');
      }
    }
    function onChange(e) {
      var t = e.target, k = t.getAttribute('data-sw');
      if (!k) return;
      var x = rec().x;
      if (typeof x[k] === 'boolean') x[k] = t.checked;
      else if (k === 'salvage') x.salvage = t.checked ? fresh(k, S) : [];
      else x[k] = t.checked ? fresh(k, S) : null;
      changed('[data-sw="' + k + '"]');
    }
    function onInput(e) {
      var t = e.target, p = t.getAttribute('data-xt');
      if (!p) return;
      set(p, t.value);
      var pre = root.querySelector('.crr-aij');
      if (pre) pre.innerHTML = hl(data(which));
      S.changed('extras');
    }
    root.addEventListener('click', onClick); root.addEventListener('change', onChange); root.addEventListener('input', onInput);
    draw();
    return {
      refresh: function () { draw(); },
      destroy: function () {
        dead = true; clearTimeout(tm);
        root.removeEventListener('click', onClick); root.removeEventListener('change', onChange); root.removeEventListener('input', onInput);
      }
    };
  }

  window.CraftRulesMore = {mount: mount};
})();
