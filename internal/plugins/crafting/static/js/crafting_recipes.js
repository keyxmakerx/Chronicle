/**
 * crafting_recipes.js -- the recipe editor (window.CraftingRecipes)
 *
 * The Crafting Editors mockup's first tab: the recipe list, the form (name,
 * template, materials, steps, the item made, the project, the cost, where
 * and who), and what players see with the checks under it. A recipe is
 * picked from parts: Armory items or "any herb" by tag, steps from the
 * library, a bench kind, so nothing typed here can break the bench. Every
 * change saves as it is made; the form and the rules editor share one queue
 * per recipe, so their saves never overtake each other.
 */
window.CraftingRecipes = (function () {
  'use strict';
  var L = window.CraftingLib, esc = L.esc;

  /* Material art is one small shape per crafting tag, tinted, so a new item
     never needs a new picture. */
  var TAG_SHAPE = {herb: 'herb', liquid: 'liquid', metal: 'metal', hide: 'hide', bone: 'bone', magic_essence: 'essence', remnant_part: 'essence', body_part: 'hide', magic_item: 'mineral'};
  var TAG_COLOR = {herb: '#6a9c3a', liquid: '#8fbbe0', metal: '#8d939c', hide: '#9a6a3e', bone: '#e3d6b8', magic_essence: '#a77cf0', remnant_part: '#6d45c4', body_part: '#b3263a', magic_item: '#7fd6d2'};
  var BENCH_COLORS = {base: '#cfe3f2', paste: '#7e1a2a', mid: '#a63a4a', final: '#ff4d6d'};
  function shape(kind, c, sz) {
    var s = sz || 32, k = 'stroke="rgb(0 0 0 / .38)" stroke-width="1.4" stroke-linejoin="round"', d;
    if (kind === 'herb') d = '<path d="M20 4C31 11 33 25 20 36C7 25 9 11 20 4Z" fill="' + c + '" ' + k + '/><path d="M20 9V33M20 17l-5-4M20 23l6-5M20 28l-5-3" stroke="rgb(0 0 0 / .32)" stroke-width="1.2" fill="none"/>';
    else if (kind === 'liquid') d = '<path d="M16 5h8v8l7 12a6 6 0 0 1-5 9H14a6 6 0 0 1-5-9l7-12z" fill="rgb(255 255 255 / .55)" ' + k + '/><path d="M11.5 23h17l2 3a5 5 0 0 1-4.6 7H14a5 5 0 0 1-4.5-7z" fill="' + c + '"/><rect x="15" y="3" width="10" height="3.5" rx="1" fill="#8a6a44"/>';
    else if (kind === 'mineral') d = '<path d="M20 3l9 9-3 22H14L11 12z" fill="' + c + '" ' + k + '/><path d="M11 12h18M20 3v31M14 34l6-22 6 22" stroke="rgb(255 255 255 / .45)" stroke-width="1" fill="none"/>';
    else if (kind === 'metal') d = '<path d="M5 28l6-12h18l6 12z" fill="' + c + '" ' + k + '/><path d="M11 16l3 4h12l3-4" stroke="rgb(255 255 255 / .5)" fill="none" stroke-width="1.2"/>';
    else if (kind === 'hide') d = '<path d="M8 8c4 2 8 1 12-2 4 3 8 4 12 2-2 5 0 10 2 13-3 4-3 9-1 12-5-1-9 0-13 2-4-2-8-3-13-2 2-3 2-8-1-12 2-3 4-8 2-13z" fill="' + c + '" ' + k + '/>';
    else if (kind === 'bone') d = '<path d="M12 6c-4 0-6 4-4 7l14 17c2 4 7 4 9 1 3 0 5-3 3-6-1-1-2-2-3-2L17 9c0-2-2-3-5-3z" fill="' + c + '" ' + k + '/>';
    else if (kind === 'essence') d = '<circle cx="20" cy="22" r="13" fill="' + c + '" opacity=".25"/><path d="M20 6c6 8 9 13 9 17a9 9 0 0 1-18 0c0-4 3-9 9-17z" fill="' + c + '" ' + k + '/><circle cx="17" cy="22" r="2.4" fill="rgb(255 255 255 / .7)"/>';
    else if (kind === 'potion') d = '<rect x="16" y="3" width="8" height="4" rx="1" fill="#8a6a44"/><path d="M16.5 7h7v6a11 11 0 1 1-7 0z" fill="rgb(255 255 255 / .5)" ' + k + '/><path d="M9.6 24a10.5 10.5 0 0 0 20.8 0z" fill="' + c + '"/><circle cx="16" cy="20" r="2" fill="rgb(255 255 255 / .7)"/>';
    else d = '<circle cx="20" cy="20" r="14" fill="none" stroke="currentColor" stroke-width="1.6" stroke-dasharray="4 3"/><text x="20" y="25" text-anchor="middle" font-size="14" font-weight="700" fill="currentColor">?</text>';
    return '<svg class="cra-ico" viewBox="0 0 40 40" width="' + s + '" height="' + s + '" aria-hidden="true">' + d + '</svg>';
  }
  function matIcon(m, items, sz) {
    var tags = L.matTags(m, items), t = tags[0];
    if (!t) return shape('', '', sz);
    return m.tag ? '<span class="cra-slot">' + shape(TAG_SHAPE[t], 'rgb(0 0 0 / .12)', sz) + '</span>' : shape(TAG_SHAPE[t], TAG_COLOR[t], sz);
  }
  var I = {
    x: '<svg viewBox="0 0 16 16" width="14" height="14" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8"/></svg>',
    up: '<svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 10l4-4 4 4"/></svg>',
    dn: '<svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 6l4 4 4-4"/></svg>',
    grip: '<svg viewBox="0 0 10 16" width="10" height="16" fill="currentColor" aria-hidden="true"><circle cx="3" cy="3" r="1.4"/><circle cx="7" cy="3" r="1.4"/><circle cx="3" cy="8" r="1.4"/><circle cx="7" cy="8" r="1.4"/><circle cx="3" cy="13" r="1.4"/><circle cx="7" cy="13" r="1.4"/></svg>',
    ok: '<svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3.5 8.5l3 3 6-7"/></svg>',
    warn: '<svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="M8 2.5l6 11H2z" stroke-linejoin="round"/><path d="M8 7v3M8 12v.01"/></svg>'
  };
  function opt(v, label, on) { return '<option value="' + esc(v) + '"' + (on ? ' selected' : '') + '>' + esc(label) + '</option>'; }
  function chk(c, i) {
    return '<div class="cra-chk ' + (c.warn ? 'is-warn' : 'is-ok') + '">' + (c.warn ? I.warn : I.ok) + '<span>' + esc(c.msg) + '</span>' +
      (c.fix ? '<button type="button" class="cra-link" data-act="fix" data-bench="' + esc(c.fix.bench) + '"' + (i != null ? ' data-i="' + i + '"' : '') + '>' + esc(c.fix.label) + '</button>' : '') + '</div>';
  }
  function cap(s) { s = String(s || ''); return s.charAt(0).toUpperCase() + s.slice(1); }

  function mount(el, app) {
    var lib = app.lib, R = app.recipes, sel = 0, dragFrom = null, queue = {}, rulesEl = null, rulesFor = null, dead = false;
    function cur() { return R[sel]; }
    function $(s) { return el.querySelector(s); }

    /* ---------- Saving: one queue per recipe ---------- */
    function want(r, kind, data) {
      var q = queue[r.id] = queue[r.id] || {r: r, timer: 0, busy: false, form: false, rules: null};
      if (kind === 'form') q.form = true; else q.rules = data;
      clearTimeout(q.timer);
      q.timer = setTimeout(function () { flush(r); }, 500);
      app.status('saving');
    }
    function failed(err) {
      if (err.status === 409 && /changed by someone else/.test(err.message)) app.status('error', 'Someone else changed this recipe. Reload to see it.', app.reloadFix);
      else app.status('error', cap(err.message));
    }
    function flush(r, keep) {
      var q = queue[r.id];
      if (!q || q.busy) return;
      var send;
      clearTimeout(q.timer);
      if (q.form) { q.form = false; send = app.api('PUT', '/recipes/' + encodeURIComponent(r.id), L.input(r), keep); }
      else if (q.rules) { var d = q.rules; q.rules = null; send = app.api('PUT', '/recipes/' + encodeURIComponent(r.id) + '/rules', {rules: d.rules, looks: d.looks, version: r.version}, keep); }
      else { app.status('saved'); return; }
      q.busy = true;
      send.then(function (got) {
        q.busy = false;
        r.version = got.version;
        r.rules = got.rules; r.looks = got.looks;
        if (!dead && r === cur()) { renderPreview(); renderChecks(); }
        if (q.form || q.rules) flush(r, keep); else app.status('saved');
      }, function (err) { q.busy = false; failed(err); });
    }
    function touch() { want(cur(), 'form'); renderList(); renderPreview(); renderChecks(); }
    function changed() { want(cur(), 'form'); renderAll(); }

    /* ---------- Drawing ---------- */
    function renderList() {
      var ul = $('.cra-rl');
      ul.innerHTML = R.length ? '' : '<li class="cra-fine">No recipes yet.</li>';
      R.forEach(function (r, i) {
        var li = document.createElement('li'), b = document.createElement('button'), col = (r.colors && r.colors.final) || BENCH_COLORS.final;
        b.type = 'button'; b.className = 'cra-ritem'; b.setAttribute('aria-current', String(i === sel));
        b.innerHTML = shape('potion', col, 28) + '<span class="cra-rtxt"><b>' + esc(r.name || 'Untitled') + '</b><small>' + esc(L.templateName(lib, r.template) === 'Recipe' ? 'No template' : L.templateName(lib, r.template)) +
          (r.who === 'learned' ? ' · must be learned' : '') + '</small>' + (r.draft ? '<span class="cra-pill cra-pill-dm">AI draft, not approved</span>' : '') + '</span>';
        b.addEventListener('click', function () { sel = i; renderAll(); });
        li.appendChild(b); ul.appendChild(li);
      });
    }
    function matSelect(m) {
      var h = '<optgroup label="From the Armory">';
      app.itemList.forEach(function (it) { h += opt(it.id, it.name, m.itemId === it.id); });
      if (m.itemId && !app.items[m.itemId]) h += opt(m.itemId, 'A deleted item', true);
      h += '</optgroup><optgroup label="Any item with a tag">';
      lib.tags.forEach(function (t) { h += opt('tag:' + t.key, 'Any ' + t.label.toLowerCase(), m.tag === t.key); });
      return h + '</optgroup>';
    }
    function qty(n, act, label) {
      return '<span class="cra-qty"><button type="button" data-act="' + act + '-" aria-label="' + label + ' fewer">−</button><output>' + n + '</output><button type="button" data-act="' + act + '+" aria-label="' + label + ' more">+</button></span>';
    }
    function stepRow(r, s, i, fixes) {
      var k = L.stepKind(lib, s.t) || {name: s.t}, c = L.stepCheck(lib, r, i, app.items), tool = L.stepTool(lib, s), ctl = '';
      fixes[i] = c.fix;
      if (k.item) {
        ctl += '<select class="cra-in cra-sm" data-f="material" aria-label="Material">' + (s.material == null || !r.materials[s.material] ? opt('', 'Pick one', true) : '');
        r.materials.forEach(function (m, j) { ctl += opt(j, L.matName(lib, m, app.items), s.material === j); });
        ctl += '</select>';
      }
      if (k.into) ctl += '<span class="cra-into">into the</span><select class="cra-in cra-sm" data-f="to" aria-label="Into">' + k.into.map(function (t) { return opt(t, t, s.to === t); }).join('') + '</select>';
      if (k.option) {
        var o = k.option, v = s[o.key], choices = o.choices.slice();
        if (choices.indexOf(v) < 0) choices.push(v);
        ctl += '<select class="cra-in cra-sm" data-f="opt" aria-label="' + esc(k.name) + ' setting">' + choices.sort(function (a, b) { return a - b; }).map(function (n) {
          var w = o.words && o.choices.indexOf(n) >= 0 ? o.words[o.choices.indexOf(n)] : (n + (o.label ? ' ' + (n === 1 ? o.label.replace(/s$/, '').replace(/presse$/, 'press') : o.label) : (o.words ? ' seconds' : '')));
          return opt(n, w, v === n);
        }).join('') + '</select>';
      }
      return '<li class="cra-step' + (c.warn ? ' is-warn' : '') + '" draggable="false" data-i="' + i + '"><span class="cra-grip" title="Drag to move">' + I.grip + '</span><span class="cra-num">' + (i + 1) + '</span>' +
        '<div class="cra-sbody"><div class="cra-sline"><b>' + esc(k.name) + '</b>' + ctl + (k.graded ? '<span class="cra-pill">Graded</span>' : '') +
        (tool ? '<span class="cra-tooln">' + esc(L.toolName(lib, tool)) + '</span>' : '') + '</div>' + chk(c, i) + '</div>' +
        '<div class="cra-sact"><button type="button" class="cra-ib" data-act="up" aria-label="Move up"' + (i ? '' : ' disabled') + '>' + I.up + '</button>' +
        '<button type="button" class="cra-ib" data-act="down" aria-label="Move down"' + (i < r.steps.length - 1 ? '' : ' disabled') + '>' + I.dn + '</button>' +
        '<button type="button" class="cra-ib" data-act="rm-step" aria-label="Remove step ' + (i + 1) + '">' + I.x + '</button></div></li>';
    }
    function renderForm() {
      var f = $('.cra-rform'), r = cur(), h = '', fixes = [], Rl = lib.rules, labels = Rl.labels || {};
      if (!r) {
        f.innerHTML = '<div class="cra-empty">No recipes yet. <button type="button" class="cra-link" data-act="new">Write the first one</button>.</div>';
        return;
      }
      if (r.draft) {
        h += '<div class="cra-banner"><div><b>Drafted by your AI</b>' + (r.draftPrompt ? ' from “' + esc(r.draftPrompt) + '”' : '') + '. Check it, then approve it. Players can’t see it until you do.</div>' +
          '<div class="cra-row"><button type="button" class="btn btn-primary btn-sm" data-act="approve">Approve</button><button type="button" class="btn btn-secondary btn-sm" data-act="discard">Discard</button></div></div>';
      }
      h += '<div class="cra-fsec cra-head"><label class="cra-fl"><span>Name</span><input class="cra-in cra-big" data-f="name" value="' + esc(r.name) + '" maxlength="120" placeholder="What is it called?"></label>' +
        '<div class="cra-ctl"><span>Template</span><div class="cra-chips" role="group" aria-label="Template">';
      lib.templates.forEach(function (T) {
        var B = L.benchKind(lib, T.bench);
        h += '<button type="button" class="cra-chip" data-tpl="' + esc(T.key) + '" aria-pressed="' + (r.template === T.key) + '">' + esc(T.name) + '<small>' + esc(B ? B.name : '') + '</small></button>';
      });
      h += '</div><em>A template fills in the steps and the bench. Change anything afterwards.</em></div></div>';

      h += '<div class="cra-fsec"><h4>Materials</h4>' + (r.materials.length ? '' : '<p class="cra-empty">No materials yet.</p>') + '<ul class="cra-mats">';
      r.materials.forEach(function (m, i) {
        var tags = L.matTags(m, app.items);
        h += '<li class="cra-mat" data-i="' + i + '">' + matIcon(m, app.items, 30) + '<select class="cra-in" data-f="mat" aria-label="Material ' + (i + 1) + '">' + matSelect(m) + '</select>' +
          '<span class="cra-tag">' + esc(tags.length ? tags.map(function (t) { return L.tagLabel(lib, t); }).join(', ') : 'no tag') + '</span>' + qty(m.qty, 'mq', 'One') +
          '<button type="button" class="cra-ib" data-act="rm-mat" aria-label="Remove ' + esc(L.matName(lib, m, app.items)) + '">' + I.x + '</button></li>';
      });
      h += '</ul><div class="cra-row"><button type="button" class="btn btn-secondary btn-sm" data-act="add-mat">Add an item</button><button type="button" class="btn btn-secondary btn-sm" data-act="add-slot">Add “any herb”</button></div>' +
        (app.itemList.length ? '' : '<p class="cra-fine">The Armory has no items yet. Add items there, then tag them under Materials.</p>') + '</div>';

      h += '<div class="cra-fsec"><h4>Steps <small>in order: drag a row, or use the arrows</small></h4>' + (r.steps.length ? '' : '<p class="cra-empty">No steps yet. Pick a template above, or add steps one by one.</p>') + '<ol class="cra-steps">';
      r.steps.forEach(function (s, i) { h += stepRow(r, s, i, fixes); });
      h += '</ol><div class="cra-addwrap"><button type="button" class="btn btn-secondary btn-sm" data-act="add-step" aria-expanded="false" aria-haspopup="menu">Add a step</button><div class="cra-menu" role="menu" hidden>';
      var B = L.benchKind(lib, r.bench);
      lib.steps.forEach(function (k) {
        var off = k.tool && B && B.tools.indexOf(k.tool) < 0;
        h += '<button type="button" role="menuitem" data-k="' + esc(k.key) + '"><span><b>' + esc(k.name) + '</b><small>' + esc(k.tool ? L.toolName(lib, k.tool) : 'any bench') + (k.graded ? ' · graded' : '') +
          (k.accepts ? ' · takes ' + k.accepts.map(function (t) { return L.tagLabel(lib, t); }).join(', ') : '') + '</small></span>' + (off ? '<span class="cra-pill cra-pill-warn">not on this bench</span>' : '') + '</button>';
      });
      h += '</div></div></div>';

      var col = r.colors || {};
      h += '<div class="cra-fsec"><h4>Result</h4><div class="cra-mat">' + shape('potion', col.final || BENCH_COLORS.final, 30) + '<select class="cra-in" data-f="result" aria-label="The item it makes">' +
        (r.result.itemId ? '' : opt('', 'Pick the Armory item it makes', true)) + app.itemList.map(function (it) { return opt(it.id, it.name, r.result.itemId === it.id); }).join('') +
        (r.result.itemId && !app.items[r.result.itemId] ? opt(r.result.itemId, 'A deleted item', true) : '') + '</select>' + qty(r.result.qty, 'rq', 'Make one') + '</div>' +
        '<div class="cra-colors"><span>On the bench</span>' + [['base', 'Water'], ['paste', 'Paste'], ['mid', 'Halfway'], ['final', 'Done']].map(function (c) {
          return '<label class="cra-swatch"><input type="color" data-color="' + c[0] + '" value="' + esc(col[c[0]] || BENCH_COLORS[c[0]]) + '"><span>' + c[1] + '</span></label>';
        }).join('') + '</div></div>';

      h += '<div class="cra-fsec"><h4>Project <span class="cra-src">' + esc(lib.system ? 'from the ' + lib.system + ' package' : 'Chronicle’s defaults') + '</span></h4><div class="cra-grid2">' +
        '<label class="cra-fl"><span>' + esc(labels.goal || 'Goal') + '</span><input type="number" class="cra-in" data-f="goal" min="1" max="999" step="5" value="' + r.goal + '"><em>project points</em></label>' +
        '<label class="cra-fl"><span>Project roll</span><span class="cra-roll"><span class="cra-mono">' + esc(Rl.dice) + ' +</span>';
      if ((Rl.characteristics || []).length) {
        h += '<select class="cra-in" data-f="stat">' + opt('', 'nothing', !r.stat) + Rl.characteristics.map(function (c) { return opt(c.key, c.label, r.stat === c.key); }).join('') + '</select>';
      } else {
        h += '<input class="cra-in" data-f="stat" value="' + esc(r.stat) + '" maxlength="40" placeholder="a characteristic">';
      }
      h += '</span><em>the characteristic it uses</em></label>' +
        '<label class="cra-fl"><span>' + esc(labels.prerequisite || 'Item prerequisite') + '</span><input class="cra-in" data-f="prerequisite" maxlength="200" value="' + esc(r.prerequisite) + '"></label>' +
        '<label class="cra-fl"><span>' + esc(labels.source || 'Project source') + '</span><input class="cra-in" data-f="source" maxlength="200" value="' + esc(r.source) + '"></label></div>' +
        '<p class="cra-fine">These come from the game system. The rest of the recipe works the same under any system.</p></div>';

      h += '<div class="cra-fsec"><h4>Cost</h4><div class="cra-grid2"><label class="cra-fl"><span>Coins</span><span class="cra-roll"><input type="number" class="cra-in" data-f="cost" min="0" step="any" value="' + (r.cost.amount || 0) + '">' +
        '<input class="cra-in cra-cur" data-f="currency" maxlength="12" value="' + esc(r.cost.currency) + '" placeholder="gp" aria-label="Coin"></span><em>on top of the materials; 0 for none</em></label></div>' +
        '<p class="cra-fine">Paid with the materials when the project starts. A hero whose sheet keeps Wealth needs it instead, as at a shop, and spends nothing.</p></div>';

      h += '<div class="cra-fsec"><h4>Where and who</h4><div class="cra-grid2"><label class="cra-fl"><span>Bench</span><select class="cra-in" data-f="bench">' +
        lib.benchKinds.map(function (b) { return opt(b.key, b.name, r.bench === b.key); }).join('') + '</select><em><button type="button" class="cra-link" data-act="open-bench">Open the bench editor</button></em></label>' +
        '<div class="cra-ctl"><span>Who knows it</span><div class="cra-seg" role="group" aria-label="Who knows it"><button type="button" data-who="all" aria-pressed="' + (r.who === 'all') + '">Everyone</button>' +
        '<button type="button" data-who="learned" aria-pressed="' + (r.who === 'learned') + '">Must be learned</button></div>';
      if (r.who === 'learned') {
        var names = {};
        app.characters.forEach(function (c) { names[c.id] = c.name; });
        h += '<div class="cra-known">' + (r.known.length ? r.known.map(function (id, i) {
          return '<span class="cra-pill">' + esc(names[id] || 'A character') + '<button type="button" class="cra-x" data-act="rm-known" data-i="' + i + '" aria-label="Forget for ' + esc(names[id] || 'this character') + '">' + I.x + '</button></span>';
        }).join('') : '<span class="cra-fine">Nobody yet.</span>') + '</div>' +
          '<select class="cra-in cra-sm" data-f="add-known" aria-label="Add a character who knows it">' + opt('', 'Add a character…', true) +
          app.characters.filter(function (c) { return r.known.indexOf(c.id) < 0; }).map(function (c) { return opt(c.id, c.name, false); }).join('') + '</select><em>Characters also learn it in play.</em>';
      } else {
        h += '<em>Any character can try it.</em>';
      }
      h += '</div></div></div>';

      var shown = (r.rules || []).filter(function (q) { return q.players !== 'secret'; }).length, n = (r.rules || []).length;
      h += '<div class="cra-fsec"><h4>When… then…</h4><p class="cra-fine">' + (n ? n + (n > 1 ? ' rules' : ' rule') + ' change this recipe' + (n - shown ? ', ' + (n - shown) + ' of them yours alone' : '') + '.' :
        'No rules yet: a full moon that helps, weather that spoils it, an event that makes an extra one.') + '</p>' +
        '<div class="cra-row"><button type="button" class="btn btn-secondary btn-sm" data-act="rules" aria-expanded="' + (rulesFor === r.id) + '">' + (rulesFor === r.id ? 'Close the rules' : 'Edit the rules') + '</button></div></div>';

      h += '<div class="cra-fsec cra-danger"><button type="button" class="btn btn-danger btn-sm" data-act="delete">Delete this recipe</button></div>';
      f.innerHTML = h;
      f.__fixes = fixes;
    }
    function renderPreview() {
      var r = cur(), b = $('.cra-book');
      if (!r) { b.innerHTML = ''; return; }
      b.innerHTML = L.bookHTML(lib, L.book(lib, r, app.items)) + (r.draft ? '<p class="cra-fine">Players don’t see drafts.</p>' : r.who === 'learned' ? '<p class="cra-fine">Only characters who know it see this page.</p>' : '');
    }
    function renderChecks() {
      var r = cur();
      $('.cra-checks').innerHTML = r ? L.checks(lib, r, app.items).map(function (c) { return '<li>' + chk(c) + '</li>'; }).join('') : '';
    }
    function renderAll() { renderList(); renderForm(); renderPreview(); renderChecks(); if (rulesFor && (!cur() || rulesFor !== cur().id)) closeRules(); }

    /* ---------- The rules editor, under the form ---------- */
    function closeRules() {
      if (rulesEl) { if (Chronicle.destroyWidget) Chronicle.destroyWidget(rulesEl); rulesEl.remove(); rulesEl = null; }
      rulesFor = null;
      $('.cra-rules').hidden = true;
    }
    function openRules() {
      var r = cur(), box = $('.cra-rules');
      closeRules();
      rulesFor = r.id;
      box.hidden = false;
      box.innerHTML = '<div class="cra-rules-head"><h3>When… then… · ' + esc(r.name) + '</h3><button type="button" class="cra-ib" data-act="rules-close" aria-label="Close the rules">' + I.x + '</button></div>';
      rulesEl = document.createElement('div');
      rulesEl.setAttribute('data-widget', 'craft_rules');
      rulesEl.setAttribute('data-campaign-id', app.cid);
      rulesEl.setAttribute('data-calendar-id', app.calendarId);
      rulesEl._craftApp = {
        recipe: {id: r.id, name: r.name, ings: r.materials.map(function (m) { return [m.qty, L.matName(lib, m, app.items)]; }), goal: r.goal, roll: L.rollWords(lib, r.stat),
          liquid: (r.colors && r.colors.final) || BENCH_COLORS.final, rules: r.rules, looks: r.looks},
        lists: {benches: app.benches.map(function (b) { return b.name; }), skills: [], items: app.itemList.map(function (it) { return it.name; }), marks: [],
          recipes: R.filter(function (x) { return x !== r; }).map(function (x) { return x.name; }), tables: []},
        save: function (data) { want(r, 'rules', data); }
      };
      box.appendChild(rulesEl);
      Chronicle.mountWidget(rulesEl);
      box.scrollIntoView({behavior: 'smooth', block: 'start'});
    }

    /* ---------- Changes ---------- */
    function newName() {
      var n = 'New recipe', i = 2, taken = {};
      R.forEach(function (r) { taken[r.name] = 1; });
      while (taken[n]) n = 'New recipe ' + (i++);
      return n;
    }
    function create() {
      app.status('saving');
      app.api('POST', '/recipes', {name: newName(), bench: lib.benchKinds[0].key, goal: 30, who: 'all', result: {qty: 1}}).then(function (r) {
        R.push(r); sel = R.length - 1; app.status('saved'); renderAll();
        var n = $('[data-f="name"]'); if (n) { n.focus(); n.select(); }
      }, failed);
    }
    function remove(r, why) {
      app.confirm(why || 'Delete “' + r.name + '”?', 'The recipe goes, and characters who learned it forget it. The items it made stay in the Armory.', 'Delete recipe').then(function (yes) {
        if (!yes) return;
        app.api('DELETE', '/recipes/' + encodeURIComponent(r.id)).then(function () {
          var i = R.indexOf(r); if (i >= 0) R.splice(i, 1);
          sel = Math.max(0, Math.min(sel, R.length - 1)); renderAll(); app.toast('Deleted “' + r.name + '”.');
        }, failed);
      });
    }
    function menuOpen(v) {
      var m = $('.cra-menu'), b = $('[data-act="add-step"]');
      if (!m) return;
      m.hidden = !v; b.setAttribute('aria-expanded', String(v));
      if (v) { var first = m.querySelector('button'); if (first) first.focus(); }
    }
    function bump(v, d, lo, hi) { return Math.max(lo, Math.min(hi, v + d)); }

    el.innerHTML = '<div class="cra-red"><aside class="cra-rlist"><h3 class="cra-ph">Recipes</h3><ul class="cra-rl"></ul>' +
      '<button type="button" class="btn btn-secondary btn-sm" data-act="new">New crafting recipe</button></aside>' +
      '<div class="cra-rform"></div><aside class="cra-rprev"><h3 class="cra-ph">What players see</h3><div class="cra-book"></div><h3 class="cra-ph">Checks</h3><ul class="cra-checks"></ul></aside></div>' +
      '<section class="cra-rules" hidden></section>';

    el.addEventListener('input', function (e) {
      var t = e.target, k = t.getAttribute('data-f'), r = cur();
      if (!r) return;
      if (k === 'name') { r.name = t.value; touch(); }
      else if (k === 'goal') { var g = parseInt(t.value, 10); if (g >= 1) { r.goal = Math.min(999, g); touch(); } }
      else if (k === 'prerequisite' || k === 'source') { r[k] = t.value; touch(); }
      else if (k === 'stat' && t.tagName === 'INPUT') { r.stat = t.value; touch(); }
      else if (k === 'cost') { var c = parseFloat(t.value); if (c >= 0) { r.cost.amount = c; touch(); } }
      else if (k === 'currency') { r.cost.currency = t.value; touch(); }
    });
    el.addEventListener('change', function (e) {
      var t = e.target, k = t.getAttribute('data-f'), r = cur(), li = t.closest('[data-i]'), i = li ? +li.getAttribute('data-i') : -1, v = t.value;
      if (!r) return;
      if (t.hasAttribute('data-color')) { r.colors = r.colors || {}; r.colors[t.getAttribute('data-color')] = v.toLowerCase(); changed(); return; }
      switch (k) {
        case 'mat': r.materials[i] = v.indexOf('tag:') === 0 ? {tag: v.slice(4), qty: r.materials[i].qty} : {itemId: v, qty: r.materials[i].qty}; break;
        case 'material': if (v === '') delete r.steps[i].material; else r.steps[i].material = +v; break;
        case 'to': r.steps[i].to = v; break;
        case 'opt': var sk = L.stepKind(lib, r.steps[i].t); r.steps[i][sk.option.key] = +v; break;
        case 'result': r.result.itemId = v; break;
        case 'stat': if (t.tagName === 'SELECT') r.stat = v; else return; break;
        case 'bench': r.bench = v; break;
        case 'add-known': if (v && r.known.indexOf(v) < 0) r.known.push(v); break;
        default: return;
      }
      changed();
    });
    el.addEventListener('click', function (e) {
      var b = e.target.closest('button');
      if (!b || !el.contains(b) || b.closest('.cra-rules [data-widget]')) return;
      var r = cur(), li = b.closest('[data-i]'), i = li ? +li.getAttribute('data-i') : -1, act = b.getAttribute('data-act');
      if (act === 'new') { create(); return; }
      if (act === 'rules-close') { closeRules(); renderForm(); return; }
      if (!r) return;
      if (b.hasAttribute('data-tpl')) {
        var before = L.clone(r), T = L.find(lib.templates, b.getAttribute('data-tpl'));
        R[sel] = L.applyTemplate(lib, r, T.key, app.items);
        changed();
        app.toast('Steps and bench set from “' + T.name + '”.', function () { before.version = R[sel].version; R[sel] = before; changed(); });
        return;
      }
      if (b.hasAttribute('data-who')) { r.who = b.getAttribute('data-who'); changed(); return; }
      if (b.hasAttribute('data-k')) { r.steps.push(L.newStep(lib, r, b.getAttribute('data-k'), app.items)); changed(); return; }
      switch (act) {
        case 'add-step': menuOpen($('.cra-menu').hidden); return;
        case 'add-mat':
          if (!app.itemList.length) { app.toast('The Armory has no items yet.'); return; }
          var used = {}; r.materials.forEach(function (m) { if (m.itemId) used[m.itemId] = 1; });
          var next = app.itemList.filter(function (it) { return !used[it.id]; })[0] || app.itemList[0];
          r.materials.push({itemId: next.id, qty: 1}); break;
        case 'add-slot': r.materials.push({tag: 'herb', qty: 1}); break;
        case 'rm-mat': L.removeMaterial(r, i); break;
        case 'mq-': case 'mq+': r.materials[i].qty = bump(r.materials[i].qty, act === 'mq+' ? 1 : -1, 1, 99); break;
        case 'rq-': case 'rq+': r.result.qty = bump(r.result.qty, act === 'rq+' ? 1 : -1, 1, 99); break;
        case 'up': case 'down':
          var j = i + (act === 'up' ? -1 : 1);
          if (j < 0 || j >= r.steps.length) return;
          r.steps.splice(j, 0, r.steps.splice(i, 1)[0]); changed();
          var nb = el.querySelectorAll('.cra-step')[j];
          if (nb) (nb.querySelector('[data-act="' + act + '"]:not([disabled])') || nb.querySelector('[data-act="rm-step"]')).focus();
          return;
        case 'rm-step': r.steps.splice(i, 1); break;
        case 'rm-known': r.known.splice(+b.getAttribute('data-i'), 1); break;
        case 'fix': r.bench = b.getAttribute('data-bench'); break;
        case 'approve':
          app.status('saving');
          app.api('POST', '/recipes/' + encodeURIComponent(r.id) + '/approve').then(function (got) {
            R[R.indexOf(r)] = got; app.status('saved'); renderAll(); app.toast('Approved. Players can now see it.');
          }, failed);
          return;
        case 'discard': remove(r, 'Discard the draft “' + r.name + '”?'); return;
        case 'delete': remove(r); return;
        case 'open-bench': app.show('benches', function (p) { if (p && p.tryRecipe) p.tryRecipe(r.id); }); return;
        case 'rules': if (rulesFor === r.id) { closeRules(); renderForm(); } else { openRules(); renderForm(); } return;
        default: return;
      }
      changed();
    });
    /* Steps move by their grip only, so text in a row can still be selected. */
    el.addEventListener('pointerdown', function (e) { var li = e.target.closest('.cra-step'); if (li) li.setAttribute('draggable', String(!!e.target.closest('.cra-grip'))); });
    el.addEventListener('dragstart', function (e) {
      var li = e.target.closest('.cra-step'); if (!li) return;
      dragFrom = +li.getAttribute('data-i'); e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', 'step'); li.classList.add('is-dragging');
    });
    el.addEventListener('dragover', function (e) {
      var li = e.target.closest('.cra-step'); if (!li || dragFrom == null) return;
      e.preventDefault();
      Array.prototype.forEach.call(el.querySelectorAll('.cra-step.is-over'), function (x) { if (x !== li) x.classList.remove('is-over'); });
      li.classList.add('is-over');
    });
    el.addEventListener('drop', function (e) {
      var li = e.target.closest('.cra-step'); if (!li || dragFrom == null) return;
      e.preventDefault();
      var to = +li.getAttribute('data-i'), r = cur();
      r.steps.splice(to, 0, r.steps.splice(dragFrom, 1)[0]); dragFrom = null; changed();
    });
    el.addEventListener('dragend', function () { dragFrom = null; Array.prototype.forEach.call(el.querySelectorAll('.cra-step'), function (x) { x.classList.remove('is-over', 'is-dragging'); }); });
    function outside(e) { var m = $('.cra-menu'); if (m && !m.hidden && !e.target.closest('.cra-addwrap')) menuOpen(false); }
    function esc_(e) { var m = $('.cra-menu'); if (e.key === 'Escape' && m && !m.hidden) { menuOpen(false); $('[data-act="add-step"]').focus(); } }
    document.addEventListener('click', outside);
    document.addEventListener('keydown', esc_);
    // What is still waiting goes now, so closing the page loses nothing typed
    function leave() { Object.keys(queue).forEach(function (k) { flush(queue[k].r, true); }); }
    window.addEventListener('pagehide', leave);
    renderAll();

    return {
      // Tags and benches change on the other tabs; the checks read both
      shown: function () { if (R.length) renderAll(); },
      destroy: function () {
        closeRules();
        document.removeEventListener('click', outside);
        document.removeEventListener('keydown', esc_);
        leave();
        dead = true;
        window.removeEventListener('pagehide', leave);
      }
    };
  }
  return {mount: mount, shape: shape, TAG_SHAPE: TAG_SHAPE, TAG_COLOR: TAG_COLOR, BENCH_COLORS: BENCH_COLORS};
})();
