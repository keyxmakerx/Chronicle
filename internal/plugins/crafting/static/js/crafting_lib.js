/**
 * crafting_lib.js -- what a recipe means, for the Crafting app (window.CraftingLib)
 *
 * Pure: checks a recipe as it is built, says its steps as sentences, lays
 * down a template, turns a recipe into the page a player reads, and into
 * the bench engine's recipe a player works. The
 * fixed lists (tags, steps, bench kinds, templates) come from the server's
 * library, so these helpers and the checks on save read the same words.
 * book() mirrors book.go, so the editor's "What players see" is the page a
 * player gets. test/js/crafting_lib.test.mjs pins it.
 */
(function () {
  'use strict';

  var NUM = ['no', 'one', 'two', 'three', 'four', 'five', 'six', 'seven', 'eight', 'nine', 'ten'];
  function num(n) { return NUM[n] || String(n); }
  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return {'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]; }); }
  function clone(o) { return o == null ? o : JSON.parse(JSON.stringify(o)); }
  function find(list, k) { for (var i = 0; i < (list || []).length; i++) if (list[i].key === k) return list[i]; return null; }

  function tagLabel(lib, k) { var t = find(lib.tags, k); return t ? t.label.toLowerCase() : k; }
  function plural(lib, k) { var l = tagLabel(lib, k); return /s$/.test(l) ? l : l + 's'; }
  function a(word) { return (/^[aeiou]/i.test(word) ? 'an ' : 'a ') + word; }
  function orList(lib, ks) {
    var xs = ks.map(function (k) { return plural(lib, k); });
    return xs.length < 2 ? xs[0] || '' : xs.slice(0, -1).join(', ') + ' or ' + xs[xs.length - 1];
  }
  function stepKind(lib, k) { return find(lib.steps, k); }
  function benchKind(lib, k) { return find(lib.benchKinds, k); }
  function toolName(lib, k) { var t = find(lib.tools, k); return t ? t.label : k; }

  /* A material's tags: a tag slot's own, or the Armory item's crafting tags. */
  function matTags(m, items) {
    if (!m) return [];
    if (m.tag) return [m.tag];
    var it = items[m.itemId];
    return it ? it.tags || [] : [];
  }
  /* A material by name: the item's, "Any herb" for a tag, or a plain word
     for an item that is gone. */
  function matName(lib, m, items) {
    if (!m) return '';
    if (m.tag) return 'Any ' + tagLabel(lib, m.tag);
    var it = items[m.itemId];
    return it ? it.name : 'A deleted item';
  }
  function stepTool(lib, s) { var k = stepKind(lib, s.t); return s.t === 'add' ? s.to : k && k.tool; }
  function benchWith(lib, tool) {
    for (var i = 0; i < lib.benchKinds.length; i++) if (lib.benchKinds[i].tools.indexOf(tool) >= 0) return lib.benchKinds[i];
    return null;
  }

  /* What each step needs, in plain words. A warning names the fix. */
  function stepCheck(lib, r, i, items) {
    var s = r.steps[i], k = stepKind(lib, s.t), B = benchKind(lib, r.bench), tool = stepTool(lib, s);
    if (!k) return {warn: true, msg: 'This step isn’t one the bench knows.'};
    if (tool && B && B.tools.indexOf(tool) < 0) {
      var o = benchWith(lib, tool);
      return {warn: true, msg: 'The ' + B.name.toLowerCase() + ' has no ' + toolName(lib, tool) + '.', fix: o ? {label: 'Use the ' + o.name.toLowerCase(), bench: o.key} : null};
    }
    if (k.item) {
      var m = s.material != null ? r.materials[s.material] : null;
      if (!m) return {warn: true, msg: 'Pick which material this step uses.'};
      var tags = matTags(m, items), name = matName(lib, m, items);
      if (m.itemId && !items[m.itemId]) return {warn: true, msg: 'That item was deleted. Pick another.'};
      if (k.accepts && k.accepts.length) {
        if (!tags.length) return {warn: true, msg: name + ' has no crafting tag yet. Give it one under Materials.'};
        if (!tags.some(function (t) { return k.accepts.indexOf(t) >= 0; })) {
          return {warn: true, msg: k.name + ' takes ' + orList(lib, k.accepts) + '. ' + name + ' is ' + a(tagLabel(lib, tags[0])) + '.'};
        }
      }
      if (s.t === 'add') return {msg: name + ' goes into the ' + s.to + '.'};
      return {msg: name + (tags.length ? ' is ' + a(tagLabel(lib, tags[0])) + ', so it fits.' : ' fits.')};
    }
    if (s.t === 'grind' || s.t === 'tip') {
      var got = null;
      for (var j = i - 1; j >= 0; j--) {
        var p = r.steps[j];
        if (p.t === 'tip') break;
        if (p.t === 'add' && p.to === 'mortar' && p.material != null && r.materials[p.material]) { got = r.materials[p.material]; break; }
      }
      if (!got) return {warn: true, msg: s.t === 'grind' ? 'Nothing is in the mortar yet. Add something to grind first.' : 'The mortar is empty at this point.'};
      if (s.t === 'grind') {
        var gt = matTags(got, items);
        if (gt.length && !gt.some(function (t) { return k.accepts.indexOf(t) >= 0; })) {
          return {warn: true, msg: 'Grind takes ' + orList(lib, k.accepts) + '. ' + matName(lib, got, items) + ' is ' + a(tagLabel(lib, gt[0])) + '.'};
        }
        return {msg: 'Grinds the ' + (got.tag ? tagLabel(lib, got.tag) : matName(lib, got, items).toLowerCase()) + '.'};
      }
      return {msg: 'Moves the paste into the flask.'};
    }
    return {msg: k.blurb + '.'};
  }

  /* The checks under the preview: what is missing or won't play. */
  function checks(lib, r, items) {
    var out = [], bad = 0, used = {}, B = benchKind(lib, r.bench);
    r.steps.forEach(function (s, i) { if (stepCheck(lib, r, i, items).warn) bad++; if (s.material != null) used[s.material] = 1; });
    if (!r.steps.length) out.push({warn: true, msg: 'No steps yet. Pick a template, or add steps one by one.'});
    else out.push(bad ? {warn: true, msg: bad + (bad > 1 ? ' steps need' : ' step needs') + ' a look. Each says why.'} : {msg: 'Every step has what it needs.'});
    r.materials.forEach(function (m, i) { if (!used[i]) out.push({warn: true, msg: matName(lib, m, items) + ' is listed but no step uses it.'}); });
    var tools = {}, miss = [];
    r.steps.forEach(function (s) { var t = stepTool(lib, s); if (t) tools[t] = 1; });
    Object.keys(tools).forEach(function (t) { if (B && B.tools.indexOf(t) < 0) miss.push(toolName(lib, t)); });
    if (r.steps.length && B) out.push(miss.length ? {warn: true, msg: 'The ' + B.name.toLowerCase() + ' lacks the ' + miss.join(' and ') + '.'} : {msg: 'The ' + B.name.toLowerCase() + ' has every tool these steps use.'});
    if (r.steps.length && !r.steps.some(function (s) { var k = stepKind(lib, s.t); return k && k.graded; })) out.push({warn: true, msg: 'No graded steps, so clean work cannot earn an edge.'});
    if (!r.result.itemId) out.push({warn: true, msg: 'Pick the Armory item it makes.'});
    else if (!items[r.result.itemId]) out.push({warn: true, msg: 'The item it made was deleted. Pick another.'});
    else out.push({msg: 'Makes ' + r.result.qty + ' × ' + items[r.result.itemId].name.toLowerCase() + ', an Armory item.'});
    return out;
  }

  /* A step's material as a method line names it. */
  function stepMat(lib, r, s, items) {
    var m = s.material != null ? r.materials[s.material] : null;
    if (!m) return 'it';
    if (m.tag) return 'a ' + tagLabel(lib, m.tag) + ' of your choice';
    return items[m.itemId] ? 'the ' + items[m.itemId].name.toLowerCase() : 'the item';
  }
  function methodLine(lib, r, s, items) {
    var n = stepMat(lib, r, s, items);
    switch (s.t) {
      case 'add': return 'Put ' + n + ' in the ' + (s.to || 'mortar') + '.';
      case 'grind': return 'Grind it, ' + num(s.turns) + ' turns.';
      case 'tip': return 'Tip the paste into the flask.';
      case 'pour': return 'Pour ' + n + ' to the line.';
      case 'simmer': return s.secs >= 10 ? 'Keep the heat steady for a long while.' : 'Keep the heat steady.';
      case 'strike': return s.n > 1 ? 'Catch the moment, ' + num(s.n) + ' times.' : 'Catch the moment.';
    }
    return '';
  }
  function rollWords(lib, stat) {
    var R = lib.rules;
    if (!stat) return R.dice;
    var c = (R.characteristics || []).filter(function (x) { return x.key === stat; })[0];
    return R.dice + ' + ' + (c ? c.label : stat);
  }
  function templateName(lib, k) { var t = find(lib.templates, k); return t ? t.name : 'Recipe'; }

  /* A book rule with no line of its own, said plainly (book.go
     ruleSentence): "When The Lantern is full: the project roll has an edge." */
  function condWords(c) {
    switch (c.kind) {
      case 'moon': return (c.moon || '') + ' is ' + (c.phase || '');
      case 'event': return (c.event || '') + ' comes round';
      case 'rolled': return (c.event || '') + ' comes up ' + (c.result || '');
      case 'weather': return 'the weather is ' + (c.weather || '');
      case 'season': return 'it is ' + (c.season || '');
      case 'time': return 'it is ' + (c.time || '');
      case 'bench': return 'it is made at ' + (c.bench || '');
      case 'ingredient': return 'an ingredient is ' + (c.mark || '');
      case 'maker': return 'the maker has ' + (c.skill || '');
    }
    return '';
  }
  function effectWords(e) {
    switch (e.kind) {
      case 'only': return 'it can only be made then';
      case 'cannot': return 'it can’t be made';
      case 'edge': return 'the project roll has an edge';
      case 'bane': return 'the project roll has a bane';
      case 'points': return (e.amount < 0 ? '' : '+') + e.amount + ' project points';
      case 'extra': return 'it makes ' + num(e.amount) + ' more';
      case 'fine': return 'it comes out fine';
      case 'becomes': return 'it becomes ' + (e.item || '');
      case 'mark': return 'it comes out ' + (e.mark || '');
      case 'quirk': return 'it may come out with a quirk';
      case 'mishap': return 'something may go wrong';
      case 'discover': return 'you may discover ' + (e.recipe || '');
      case 'say': return e.text || '';
    }
    return '';
  }
  function ruleSentence(q) {
    var when = (q.when || []).map(condWords).filter(Boolean);
    var out = (when.length ? 'When ' + when.join(' and ') : 'Always') + ': ' + effectWords(q.then || {}) + (q.chance ? ' (' + q.chance + ')' : '');
    return /[.!?]$/.test(out) ? out : out + '.';
  }

  /* The page a player reads, shaped like the server's BookPage. */
  function book(lib, r, items) {
    var B = benchKind(lib, r.bench), graded = false;
    var p = {id: r.id, name: r.name, kind: templateName(lib, r.template), bench: B ? B.name : '', needs: [], method: [], lines: [],
      makes: r.result.itemId ? (items[r.result.itemId] ? items[r.result.itemId].name : 'an item you haven’t seen') : '', makesQty: r.result.qty,
      goal: r.goal, roll: rollWords(lib, r.stat), prerequisite: r.prerequisite, source: r.source, cost: r.cost && r.cost.amount > 0 ? r.cost : null};
    r.materials.forEach(function (m) { p.needs.push({qty: m.qty, name: m.tag ? 'any ' + tagLabel(lib, m.tag) : (items[m.itemId] ? items[m.itemId].name : 'an item you haven’t seen')}); });
    r.steps.forEach(function (s) { var l = methodLine(lib, r, s, items); if (l) p.method.push(l); var k = stepKind(lib, s.t); if (k && k.graded) graded = true; });
    p.graded = graded;
    (r.rules || []).forEach(function (q) { if (!q.draft && q.players === 'book') p.lines.push(q.bookLine || ruleSentence(q)); });
    return p;
  }
  function money(c) {
    var n = Math.round(c.amount * 100) / 100;
    return (n % 1 ? n.toFixed(2) : String(n)) + (c.currency ? ' ' + c.currency : '');
  }
  /* A book page as HTML; every word is escaped. */
  function bookHTML(lib, p) {
    var h = '<div class="cra-page paper"><p class="cra-bk-kind">' + esc(p.kind) + (p.bench ? ' · ' + esc(p.bench) : '') + '</p><h3>' + esc(p.name || 'Untitled') + '</h3>';
    h += '<p class="cra-bk-h">You need</p><ul>' + (p.needs.length ? p.needs.map(function (n) { return '<li>' + n.qty + ' × ' + esc(n.name.toLowerCase()) + '</li>'; }).join('') : '<li class="cra-bk-dim">nothing yet</li>') + '</ul>';
    if (p.cost) h += '<p class="cra-bk-cost">and ' + esc(money(p.cost)) + '</p>';
    h += '<p class="cra-bk-h">Method</p><ol>' + (p.method.length ? p.method.map(function (l) { return '<li>' + esc(l) + '</li>'; }).join('') : '<li class="cra-bk-dim">no steps yet</li>') + '</ol>';
    if (p.makes) h += '<p class="cra-bk-makes">Makes ' + p.makesQty + ' × ' + esc(p.makes.toLowerCase()) + '.</p>';
    if (p.lines.length) h += '<ul class="cra-bk-lines">' + p.lines.map(function (l) { return '<li>' + esc(l) + '</li>'; }).join('') + '</ul>';
    var foot = 'Project: ' + p.roll + ', goal ' + p.goal + (p.graded ? '. Clean work earns an edge.' : '.');
    if (p.prerequisite) foot += ' Needs ' + p.prerequisite.charAt(0).toLowerCase() + p.prerequisite.slice(1) + (/[.!?]$/.test(p.prerequisite) ? '' : '.');
    h += '<p class="cra-bk-foot">' + esc(foot) + '</p>';
    if (p.source) h += '<p class="cra-bk-src">' + esc(p.source) + '</p>';
    if (p.knownBy && p.knownBy.length) h += '<p class="cra-bk-src">Known by ' + esc(p.knownBy.join(', ')) + '</p>';
    return h + '</div>';
  }

  /* Lays a template's steps over a recipe: each step that wants a material
     takes the first one whose tags fit, or adds an "any" slot for it. */
  function applyTemplate(lib, r, key, items) {
    var T = find(lib.templates, key);
    if (!T) return r;
    var out = clone(r);
    out.steps = T.steps.map(function (ts) {
      var s = {t: ts.t};
      if (ts.to) s.to = ts.to;
      ['turns', 'secs', 'n'].forEach(function (p) { if (ts[p]) s[p] = ts[p]; });
      if (ts.want) {
        var idx = -1;
        out.materials.forEach(function (m, i) { if (idx < 0 && matTags(m, items).some(function (t) { return ts.want.indexOf(t) >= 0; })) idx = i; });
        if (idx < 0) { out.materials.push({tag: ts.want[0], qty: 1}); idx = out.materials.length - 1; }
        s.material = idx;
      }
      return s;
    });
    out.template = key;
    out.bench = T.bench;
    return out;
  }
  /* A new step of a kind, with its material and setting filled in. */
  function newStep(lib, r, key, items) {
    var k = stepKind(lib, key), s = {t: key};
    if (!k) return s;
    if (k.item) {
      var idx = -1;
      r.materials.forEach(function (m, i) { if (idx < 0 && (!k.accepts || matTags(m, items).some(function (t) { return k.accepts.indexOf(t) >= 0; }))) idx = i; });
      if (idx >= 0) s.material = idx;
    }
    if (k.into) s.to = k.into[0];
    if (k.option) s[k.option.key] = k.option['default'];
    return s;
  }
  /* Removing material i: steps that used it lose it, later ones shift down. */
  function removeMaterial(r, i) {
    r.materials.splice(i, 1);
    r.steps.forEach(function (s) { if (s.material === i) delete s.material; else if (s.material > i) s.material--; });
  }
  /* What a material wears, {picture, tint}: a kind's picture as the campaign
     set it (lib.tags), or an item's own, else its first kind's. Pass an
     item from the catalog ({tags, picture}), a kind key, or a recipe's
     material with the catalog's items. picture is '' when there is none. */
  function matPicture(lib, x, items) {
    if (x && typeof x === 'object' && (x.itemId || x.tag)) {
      if (x.tag) return matPicture(lib, x.tag);
      var it = (items || {})[x.itemId];
      return it ? matPicture(lib, it) : {picture: '', tint: '#8a8a8a'};
    }
    if (typeof x === 'string') {
      var t = find(lib.tags, x);
      return t ? {picture: t.picture, tint: t.tint} : {picture: '', tint: '#8a8a8a'};
    }
    if (x && x.picture && x.picture.picture) return {picture: x.picture.picture, tint: x.picture.tint};
    return x && x.tags && x.tags.length ? matPicture(lib, x.tags[0]) : {picture: '', tint: '#8a8a8a'};
  }
  /* The brew's colours when a recipe names none. */
  var BENCH_COLORS = {base: '#cfe3f2', paste: '#7e1a2a', mid: '#a63a4a', final: '#ff4d6d'};

  /* A recipe a hero knows (the server's CraftRecipe) as the bench engine
     plays it: each material an ingredient keyed by its item or tag, wearing
     the picture the server worked out for it, and the brew's colours.
     start is the project's points. */
  function benchRecipe(r, hero, start) {
    var items = {}, keyOf = [], col = r.colors || {};
    (r.needs || []).forEach(function (m, i) {
      var k = m.itemId ? 'i' + m.itemId.replace(/[^a-zA-Z0-9]/g, '') : 't' + m.tag, pic = m.picture || {};
      if (!items[k]) items[k] = {name: m.tag ? m.name.charAt(0).toUpperCase() + m.name.slice(1) : m.name, color: pic.tint || '#8a8a8a', picture: pic.picture || '', desc: m.tag ? 'Your pick from the pack.' : ''};
      keyOf[i] = k;
    });
    var stat = String(r.roll || '').split(' + ')[1] || '';
    return {items: items, recipe: {name: r.name, kind: r.kind, who: hero, stat: stat, mod: r.statValue || 0, goal: r.goal, start: start || 0,
      base: col.base || BENCH_COLORS.base, paste: col.paste || BENCH_COLORS.paste, mid: col.mid || BENCH_COLORS.mid, final: col.final || BENCH_COLORS.final,
      steps: (r.steps || []).map(function (s) {
        var o = {t: s.t};
        if (s.material != null && keyOf[s.material]) o.item = keyOf[s.material];
        ['to', 'turns', 'secs', 'n'].forEach(function (p) { if (s[p] != null) o[p] = s[p]; });
        return o;
      })}};
  }

  /* A project's points as a bar, the same in the block and the Projects tab. */
  function progress(points, goal, label, name) {
    var at = Math.min(points, goal), pct = goal > 0 ? Math.min(100, Math.round(at / goal * 100)) : 0;
    return '<div class="crp-prog"><div class="crp-lab"><span>' + esc(label || 'Project points') + '</span><b>' + at + ' of ' + goal + '</b></div>' +
      '<div class="crp-track" role="progressbar" aria-valuemin="0" aria-valuemax="' + goal + '" aria-valuenow="' + at + '" aria-label="' + esc(name) + ' points">' +
      '<i class="' + (at >= goal ? 'is-done' : '') + '" style="width:' + pct + '%"></i></div></div>';
  }

  /* What the editor sends to save a recipe. */
  function input(r) {
    return {name: r.name, template: r.template || '', bench: r.bench, materials: r.materials, steps: r.steps, result: r.result, goal: r.goal, stat: r.stat,
      prerequisite: r.prerequisite, source: r.source, cost: r.cost, who: r.who, known: r.known, colors: r.colors, version: r.version};
  }

  window.CraftingLib = {esc: esc, clone: clone, find: find, num: num, tagLabel: tagLabel, matTags: matTags, matName: matName, stepKind: stepKind, benchKind: benchKind,
    toolName: toolName, stepTool: stepTool, stepCheck: stepCheck, checks: checks, methodLine: methodLine, rollWords: rollWords, templateName: templateName,
    book: book, bookHTML: bookHTML, ruleSentence: ruleSentence, money: money, applyTemplate: applyTemplate, newStep: newStep, removeMaterial: removeMaterial, input: input,
    matPicture: matPicture, BENCH_COLORS: BENCH_COLORS, benchRecipe: benchRecipe, progress: progress};
})();
