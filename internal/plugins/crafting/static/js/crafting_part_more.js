/**
 * crafting_part_more.js -- the Crafting recipes part's other views
 * (window.CraftingPartMore)
 *
 * Three ways to fill the part besides typing rows, each drawn on the canvas
 * with its settings in the inspector:
 *   generate  Chronicle's own seeded generator. No AI: the same seed gives
 *             the same list, a reroll changes one row. Goals come only from
 *             the game system's published goals or the Director's own
 *             recipes; with neither, the Director types one before keeping.
 *   ai        A door to the Director's own AI: a ready prompt naming this
 *             campaign's materials, and a paste box that goes through the
 *             same plan-and-tick review as every AI paste. Owner only, as
 *             the AI page is.
 *   packs     Recipe packs: save the part's recipes as a pack, share it with
 *             Directors on this server, or bring one in from another
 *             campaign, a shared pack or a file, matching its materials.
 * Everything kept or brought in arrives as a draft. crafting_part.js owns
 * the store and loads after this file, so helpers are read when called.
 */
(function () {
  'use strict';
  var L = window.CraftingLib, P = window.CraftingPick, esc = L.esc;
  function H() { return window.CraftingPart; }
  function fa(n) { return '<i class="fa-solid fa-' + n + '" aria-hidden="true"></i>'; }
  function plural(n, one, many) { return n + ' ' + (n === 1 ? one : (many || one + 's')); }
  function back() { return '<button type="button" class="crm-q" data-view="table">' + fa('arrow-left') + 'Back to the table</button>'; }

  /* ---------- Generate ---------- */

  var SEED_WORDS = ['ash', 'moss', 'ember', 'brine', 'thorn', 'frost', 'loam', 'reed', 'cinder', 'dusk'];
  function newSeed() { return SEED_WORDS[Math.floor(Math.random() * SEED_WORDS.length)] + '-' + (1000 + Math.floor(Math.random() * 9000)); }

  function gen(st) {
    if (!st.gen) {
      var mk = (st.lib.makes || [])[0] || {key: 'potions', kinds: []};
      st.gen = {make: mk.key, kinds: (mk.kinds || []).slice(), count: 6, echelon: 1, table: '', seed: newSeed(), rerolls: [], rows: null, keep: {}, goals: {},
        busy: false, adding: ''};
    }
    return st.gen;
  }

  function roll(st, fresh) {
    var g = gen(st);
    if (fresh) { g.rerolls = []; g.keep = {}; g.goals = {}; }
    g.busy = true; st.say('Rolling…');
    st.api('POST', '/generate', {make: g.make, kinds: g.kinds, count: g.count, echelon: g.echelon, table: g.table, seed: g.seed, rerolls: g.rerolls}).then(function (got) {
      g.busy = false; g.rows = got.rows || [];
      g.rows.forEach(function (r, i) { if (g.keep[i] == null) g.keep[i] = !r.taken; if (r.taken) g.keep[i] = false; });
      st.say(''); st.emit();
    }, function (err) { g.busy = false; st.say(err.message, true); st.emit(); });
  }

  function goalOf(g, r, i) { return r.goal || +g.goals[i] || 0; }
  function kept(g) { return (g.rows || []).filter(function (r, i) { return g.keep[i]; }); }

  function keepRows(st) {
    var g = gen(st), want = [];
    (g.rows || []).forEach(function (r, i) { if (g.keep[i]) want.push({r: r, i: i}); });
    var noGoal = want.filter(function (x) { return !goalOf(g, x.r, x.i); });
    if (!want.length) { st.say('Tick the rows to keep.', true); return; }
    if (noGoal.length) { st.say('Give ' + noGoal.map(function (x) { return x.r.name; }).join(', ') + ' a goal first: nothing published covers it.', true); return; }
    var ids = [], skipped = [];
    st.say('Keeping ' + plural(want.length, 'recipe') + '…');
    want.reduce(function (p, x) {
      return p.then(function () {
        var r = x.r, item = r.resultId ? Promise.resolve({id: r.resultId}) : st.api('POST', '/materials', {name: r.name, kinds: []}).then(function (it) { st.addItem(it); return it; });
        return item.then(function (it) {
          return st.newRecipe({name: r.name, template: r.template, materials: r.materials.map(function (m) { return m.itemId ? {itemId: m.itemId, qty: m.qty} : {tag: m.tag, qty: m.qty}; }),
            steps: r.steps, result: {itemId: it.id, qty: 1}, goal: goalOf(g, r, x.i), draft: true});
        }).then(function (got) { ids.push(got.id); }, function (err) { skipped.push(r.name + ' (' + err.message + ')'); });
      });
    }, Promise.resolve()).then(function () {
      st.add(ids);
      st.gen.rows = null;
      st.setView('table');
      st.say((ids.length ? 'Kept ' + plural(ids.length, 'recipe') + ' as drafts.' : 'Nothing was kept.') + (skipped.length ? ' Left out: ' + skipped.join('; ') + '.' : ''), !!skipped.length);
    });
  }

  function genRow(st, g, r, i) {
    var first = r.materials[0], mats = r.materials.map(function (m) { return m.qty + ' ' + esc(m.name); }).join(', ');
    var method = r.template ? L.templateName(st.lib, r.template) : 'By hand';
    var goal = r.goal ? method + ' · goal ' + r.goal + (r.goalFrom === 'recipes' ? ' <span class="crm-mut">like your recipes</span>' : '')
      : method + ' · <label class="crm-mut">goal <input class="crm-num" type="number" min="1" max="999" data-goal="' + i + '" value="' + esc(g.goals[i] || '') + '" aria-label="Goal for ' + esc(r.name) + '"></label>';
    var notes = [];
    if (r.taken) notes.push('A recipe has this name already.');
    (r.missing || []).forEach(function (k) {
      var lab = L.tagLabel(st.lib, k);
      notes.push(g.adding === i + ':' + k
        ? '<span class="crm-type"><input class="crm-in" data-add-name="' + i + ':' + esc(k) + '" placeholder="Name the ' + esc(lab) + ', then Enter" aria-label="Name the new ' + esc(lab) + '" maxlength="120"></span>'
        : 'Needs any ' + esc(lab) + ': you have none. <button type="button" class="crm-q" data-add-kind="' + i + ':' + esc(k) + '">Add one to the Armory</button>');
    });
    return '<tr><td><input type="checkbox" class="crm-tick" data-keep="' + i + '"' + (g.keep[i] ? ' checked' : '') + (r.taken ? ' disabled' : '') + ' aria-label="Keep ' + esc(r.name) + '"></td>' +
      '<td class="crm-name">' + P.pic(first ? (first.itemId ? L.matPicture(st.lib, st.byId[first.itemId] || {tags: []}) : L.matPicture(st.lib, first.tag)) : null, 20) + '<span>' + esc(r.name) + '</span></td>' +
      '<td>' + mats + (notes.length ? '<br><span class="crm-mut">' + notes.join('<br>') + '</span>' : '') + '</td><td>' + goal + '</td>' +
      '<td><button type="button" class="crm-ib" data-reroll="' + i + '" title="Roll this row again" aria-label="Roll ' + esc(r.name) + ' again">' + fa('rotate') + '</button></td></tr>';
  }

  var generate = {
    canvas: function (st) {
      var g = gen(st);
      var h = '<div class="crm-part"><div class="crm-head"><h3 class="crm-h">Generated</h3><span class="crm-seed">seed ' + esc(g.seed) + '</span><span class="crm-words">' +
        '<button type="button" class="crm-q" data-act="roll-all"' + (g.busy ? ' disabled' : '') + '>Roll all again</button>' + back() + '</span></div>';
      if (!g.rows) return h + '<p class="crm-mut" role="status">Rolling…</p></div>';
      if (!g.rows.length) return h + '<p class="crm-none">Nothing to roll from. Give some Armory items a crafting kind, or pick other kinds.</p></div>';
      h += '<div class="crm-tablewrap"><table class="crm-table"><thead><tr><th scope="col"><span class="sr-only">Keep</span></th><th scope="col">Recipe</th><th scope="col">Materials</th>' +
        '<th scope="col">Method and goal</th><th scope="col"><span class="sr-only">Roll again</span></th></tr></thead><tbody>';
      g.rows.forEach(function (r, i) { h += genRow(st, g, r, i); });
      h += '</tbody></table></div><p class="crm-help">Names, materials and goals come from your campaign’s own materials' + (st.lib.system ? ' and ' + esc(st.lib.system) + '’s project rules' : '') +
        '. Nothing is saved until you keep it; kept rows arrive as drafts. A row with no goal has no published one: type it.</p></div>';
      return h;
    },
    inspector: function (st) {
      var g = gen(st), makes = st.lib.makes || [], seg = H().seg;
      var tables = [['', 'From the material']].concat(st.tables.map(function (t) { return [t.name, t.name]; }));
      var goals = (st.lib.rules && st.lib.rules.goals) || [];
      return '<div class="crm-insp"><p class="crm-insp-h">Generate crafting recipes</p><p>Chronicle’s own generator, like names and weather. Same seed, same list.</p>' +
        '<div class="crm-field"><span>Make</span>' + seg('make', makes.map(function (m) { return [m.key, m.label]; }), g.make) + '</div>' +
        '<div class="crm-field"><span>From</span><div class="crm-chips">' + st.lib.tags.map(function (t) {
          return '<button type="button" class="crm-chip crm-chip-t" data-kind="' + esc(t.key) + '" aria-pressed="' + (g.kinds.indexOf(t.key) >= 0) + '">' + esc(t.label) + '</button>';
        }).join('') + '</div></div>' +
        '<div class="crm-field"><span>How many</span><input class="crm-num" type="number" min="1" max="12" data-g="count" value="' + g.count + '"></div>' +
        '<div class="crm-field"><span>Echelon</span>' + seg('echelon', [[1, '1'], [2, '2'], [3, '3'], [4, '4']].map(function (e) { return [String(e[0]), e[1]]; }), String(g.echelon)) + '</div>' +
        '<div class="crm-field"><span>Names</span><select class="crm-sel" data-g="table" aria-label="Where names come from">' + tables.map(function (t) {
          return '<option value="' + esc(t[0]) + '"' + (t[0] === g.table ? ' selected' : '') + '>' + esc(t[1]) + '</option>';
        }).join('') + '</select></div>' +
        '<div class="crm-field"><span>Seed</span><input class="crm-in" data-g="seed" value="' + esc(g.seed) + '" maxlength="40" aria-label="Seed"></div>' +
        '<p>' + (goals.length ? 'Goals are the published ones for the echelon you pick.' : 'Your game system publishes no project goals here, so a goal comes from your own approved recipes, or you type it.') + '</p>' +
        '<p>A row that needs a material you don’t have yet offers to add it to the Armory.</p></div>';
    },
    wire: function (st, el, m, mode) {
      var g = gen(st);
      if (mode === 'canvas') {
        if (!g.rows && !g.busy) roll(st, true);
        var n = kept(g).length;
        if (st.ctx.setPrimary) st.ctx.setPrimary(n ? 'Keep ' + n + ' as drafts' : null, function () { keepRows(st); });
        el.querySelectorAll('[data-keep]').forEach(function (c) {
          c.addEventListener('change', function () { g.keep[+c.getAttribute('data-keep')] = c.checked; m.refocus = '[data-keep="' + c.getAttribute('data-keep') + '"]'; st.emit(); });
        });
        el.querySelectorAll('[data-goal]').forEach(function (c) {
          c.addEventListener('change', function () { g.goals[+c.getAttribute('data-goal')] = Math.max(1, Math.min(999, parseInt(c.value, 10) || 0)) || ''; });
        });
        var add = el.querySelector('[data-add-name]');
        if (add) {
          add.focus();
          add.addEventListener('keydown', function (e) {
            if (e.key === 'Escape') { g.adding = ''; st.emit(); return; }
            if (e.key !== 'Enter' || !add.value.trim()) return;
            e.preventDefault();
            var kind = add.getAttribute('data-add-name').split(':')[1];
            H().addToArmory(st, add.value.trim(), [kind], function () { g.adding = ''; roll(st, false); });
          });
        }
        return;
      }
      el.querySelectorAll('[data-g]').forEach(function (c) {
        c.addEventListener('change', function () {
          var k = c.getAttribute('data-g');
          if (k === 'count') g.count = Math.max(1, Math.min(12, parseInt(c.value, 10) || 6));
          if (k === 'table') g.table = c.value;
          if (k === 'seed') g.seed = c.value.trim().slice(0, 40) || newSeed();
          roll(st, true);
        });
      });
    },
    click: function (st, b) {
      var g = gen(st);
      if (b.getAttribute('data-act') === 'roll-all') { g.seed = newSeed(); roll(st, true); return; }
      if (b.hasAttribute('data-reroll')) { var i = +b.getAttribute('data-reroll'); g.rerolls[i] = (g.rerolls[i] || 0) + 1; delete g.goals[i]; roll(st, false); return; }
      if (b.hasAttribute('data-add-kind')) { g.adding = b.getAttribute('data-add-kind'); st.emit(); return; }
      if (b.hasAttribute('data-kind')) {
        var k = b.getAttribute('data-kind'), at = g.kinds.indexOf(k);
        if (at >= 0) g.kinds.splice(at, 1); else g.kinds.push(k);
        roll(st, true); return;
      }
      if (b.hasAttribute('data-seg')) {
        var s = b.getAttribute('data-seg'), v = b.getAttribute('data-v');
        if (s === 'make') {
          g.make = v;
          (st.lib.makes || []).forEach(function (mk) { if (mk.key === v) g.kinds = (mk.kinds || []).slice(); });
        }
        if (s === 'echelon') g.echelon = +v;
        roll(st, true);
      }
    }
  };

  /* ---------- Write with your AI ---------- */

  // goalRange is what the prompt asks goals to fall between: the system's
  // published goals, else the Director's own approved recipes.
  function goalRange(st) {
    var g = ((st.lib.rules && st.lib.rules.goals) || []).map(function (x) { return x.goal; });
    if (!g.length) g = st.all.filter(function (r) { return !r.draft; }).map(function (r) { return r.goal; });
    if (!g.length) return '';
    var lo = Math.min.apply(null, g), hi = Math.max.apply(null, g);
    return lo === hi ? 'Goals around ' + lo + '.' : 'Goals ' + lo + ' to ' + hi + '.';
  }

  function prompt(st) {
    var mats = st.items.filter(function (it) { return (it.tags || []).length; }).slice(0, 80);
    var kinds = st.lib.tags.map(function (t) { return 'any ' + t.label.toLowerCase(); });
    var who = (st.ctx.part && st.ctx.part.label) || 'this set';
    var lines = ['Write 8 crafting recipes for ' + who + '.', 'Use only these materials or kinds:'];
    lines.push('  ' + (mats.length ? mats.map(function (it) { return it.name + ' (' + it.tags.map(function (k) { return L.tagLabel(st.lib, k); }).join(', ') + ')'; }).join(', ') : '(no materials yet)'));
    lines.push('  ' + kinds.join(', '));
    lines.push('Methods: ' + st.lib.templates.map(function (t) { return t.key; }).join(', ') + '. ' + goalRange(st));
    lines.push('Answer in Chronicle’s recipe format, one block per recipe (kind: recipe):', '', '---', 'kind: recipe', 'name: Troll-blood tonic', 'makes: 1 Troll-blood tonic',
      'materials:', '  - 2 Troll tooth', '  - 1 any liquid', 'method: steady', 'goal: 30', '---');
    return lines.join('\n');
  }

  var ai = {
    canvas: function (st) {
      if (!st.lib.owner) return '<div class="crm-part"><div class="crm-head"><h3 class="crm-h">Write recipes with your AI</h3><span class="crm-words">' + back() + '</span></div>' +
        '<p class="crm-none">Only the campaign’s owner can use the AI door, as on the AI page.</p></div>';
      st.ai = st.ai || {};
      return '<div class="crm-part"><div class="crm-head"><h3 class="crm-h">Write recipes with your AI</h3><span class="crm-words">' + back() + '</span></div>' +
        '<p class="crm-step-h">1 · Copy this to your AI</p><pre class="crm-pre" data-prompt>' + esc(prompt(st)) + '</pre>' +
        '<p><button type="button" class="crm-q" data-act="copy">' + fa('copy') + 'Copy the prompt</button> <span class="crm-mut">It lists this campaign’s materials so the AI uses yours.</span></p>' +
        '<p class="crm-step-h">2 · Paste its answer, then check each one</p>' +
        '<form data-ai hx-post="/campaigns/' + encodeURIComponent(st.cid) + '/ai-workspace/import/parse" hx-target="#hbw-ai-review-host" hx-swap="innerHTML">' +
        '<input type="hidden" name="review_host" value="hbw-ai-review-host"><input type="hidden" name="lookup_privacy" value="safe">' +
        '<label class="sr-only" for="crm-ai-paste">What your AI wrote</label><textarea id="crm-ai-paste" class="crm-paste" name="markdown_paste" maxlength="5242880" placeholder="Paste what your AI wrote"></textarea>' +
        '<p><button type="submit" class="crm-btn crm-btn-2">Check what it wrote</button></p></form>' +
        '<div id="hbw-ai-review-host" aria-live="polite"></div>' +
        '<p class="crm-help">The same check as every AI paste: nothing saves until you tick it, a recipe needing a material the Armory lacks is held back with the reason, and new recipes stay drafts. What you save here joins this part.</p></div>';
    },
    inspector: function () {
      return '<div class="crm-insp"><p class="crm-insp-h">Already built</p><p>The AI connection can read crafting and write recipes, material kinds, drop lists and item looks today, each one checked before it saves. This is a door to it from the Workshop, with a prompt that already names your materials.</p>' +
        '<p class="crm-insp-h">Chronicle has no AI of its own</p><p>It works with the AI you already use, through the AI page or the connector.</p></div>';
    },
    wire: function (st, el, m, mode) {
      if (mode !== 'canvas' || !st.lib.owner) return;
      var host = el.querySelector('#hbw-ai-review-host'), form = el.querySelector('[data-ai]');
      // A redraw keeps a review that is open.
      if (st.ai.host && st.ai.host.childNodes.length) { while (st.ai.host.firstChild) host.appendChild(st.ai.host.firstChild); }
      st.ai.host = host;
      if (window.htmx) window.htmx.process(form);
      form.addEventListener('htmx:beforeRequest', function () { st.ai.before = st.all.map(function (r) { return r.id; }); });
      host.addEventListener('htmx:afterSwap', function (e) {
        var path = (e.detail && e.detail.pathInfo && e.detail.pathInfo.requestPath) || '';
        if (path.indexOf('/import/commit') < 0 || !st.ai.before) return;
        var before = st.ai.before;
        st.api('GET', '/recipes').then(function (list) {
          list = Array.isArray(list) ? list : ((list && list.data) || []);
          var fresh = list.filter(function (r) { return before.indexOf(r.id) < 0; });
          fresh.forEach(function (r) { st.keep(r); });
          var n = st.add(fresh.map(function (r) { return r.id; }));
          st.say(n ? 'Added ' + plural(n, 'new recipe') + ' to this part, as drafts.' : 'Nothing new to add to this part.');
        }, function (err) { st.say(err.message, true); });
      });
    },
    click: function (st, b) {
      if (b.getAttribute('data-act') !== 'copy') return;
      var text = prompt(st);
      (navigator.clipboard ? navigator.clipboard.writeText(text) : Promise.reject()).then(function () { st.say('Copied. Paste it into your AI.'); }, function () {
        var pre = document.querySelector('[data-prompt]'), sel = window.getSelection(), rg = document.createRange();
        if (pre) { rg.selectNodeContents(pre); sel.removeAllRanges(); sel.addRange(rg); }
        st.say('Your browser wouldn’t copy it. It is selected: copy it yourself.');
      });
    }
  };

  /* ---------- Recipe packs ---------- */

  function pk(st) {
    if (!st.packs) st.packs = {tab: 'packs', list: null, open: null, matches: {}, pickAll: true, pick: {}, busy: false};
    return st.packs;
  }
  function loadPacks(st) {
    var k = pk(st);
    k.busy = true;
    st.api('GET', '/packs').then(function (got) {
      k.busy = false; k.list = Array.isArray(got) ? got : ((got && got.data) || []); st.emit();
    }, function (err) { k.busy = false; k.list = []; st.say(err.message, true); st.emit(); });
  }
  function fromWords(p) {
    if (p.file) return 'a file you uploaded';
    if (p.from === 'campaign') return 'from another of your campaigns' + (p.campaign ? ', ' + esc(p.campaign) : '');
    if (p.from === 'server') return 'shared by a Director on this server';
    return p.shared ? 'saved here, shared on this server' : 'saved here';
  }
  function openPack(st, id) {
    var k = pk(st);
    st.say('Opening the pack…');
    st.api('GET', '/packs/' + encodeURIComponent(id)).then(function (d) {
      k.open = d; k.matches = {}; k.pickAll = true; k.pick = {};
      (d.materials || []).forEach(function (mt) { k.matches[mt.name] = (d.matches && d.matches[mt.name]) || ''; });
      (d.recipes || []).forEach(function (r) { k.pick[r.name] = true; });
      st.say(''); st.emit();
    }, function (err) { st.say(err.message, true); });
  }
  function picked(k) {
    if (!k.open) return [];
    return (k.open.recipes || []).filter(function (r) { return k.pickAll || k.pick[r.name]; }).map(function (r) { return r.name; });
  }
  function bringIn(st) {
    var k = pk(st), names = picked(k), d = k.open;
    if (!names.length) { st.say('Pick at least one recipe.', true); return; }
    st.say('Bringing in ' + plural(names.length, 'recipe') + '…');
    st.api('POST', '/packs/' + encodeURIComponent(d.id) + '/import', {matches: k.matches, recipes: k.pickAll ? [] : names}).then(function (res) {
      (res.items || []).forEach(function (it) { st.addItem({id: it.id, name: it.name, tags: []}); });
      return st.api('GET', '/recipes').then(function (list) {
        list = Array.isArray(list) ? list : ((list && list.data) || []);
        list.forEach(function (r) { st.keep(r); });
        st.add((res.made || []).map(function (r) { return r.id; }));
        k.open = null;
        st.setView('table');
        var skip = res.skipped || [];
        st.say('Brought in ' + plural((res.made || []).length, 'recipe') + ' as drafts.' + (skip.length ? ' Left out: ' + skip.map(function (s) { return s.name + ' (' + s.reason + ')'; }).join('; ') + '.' : ''), !!skip.length);
      });
    }, function (err) { st.say(err.message, true); });
  }

  function packRow(st, p, k) {
    var isOpen = k.open && k.open.id === p.id, mine = p.from === 'here' && !p.file;
    var h = '<div class="crm-pack' + (isOpen ? ' is-open' : '') + '"><div class="crm-pack-pics">' + fa('box-archive') + '</div><div><b>' + esc(p.name) + '</b><br><span class="crm-mut">' +
      plural(p.count, 'recipe') + ' · ' + fromWords(p) + '</span></div><span class="crm-words">';
    if (mine) {
      h += '<button type="button" class="crm-q" data-share="' + esc(p.id) + '" aria-pressed="' + p.shared + '">' + (p.shared ? 'Stop sharing' : 'Share on this server') + '</button>' +
        '<a class="crm-q" href="' + st.base + '/packs/' + encodeURIComponent(p.id) + '/file" download>Download</a>' +
        '<button type="button" class="crm-q" data-del="' + esc(p.id) + '">Delete</button>';
    } else {
      h += '<button type="button" class="crm-q" data-open="' + esc(p.id) + '">' + (isOpen ? 'Open' : 'Look inside') + '</button>';
      if (p.from === 'here') h += '<button type="button" class="crm-q" data-del="' + esc(p.id) + '">Delete</button>';
    }
    h += '</span></div>';
    if (isOpen && !k.pickAll) {
      h += '<div class="crm-picklist">' + (k.open.recipes || []).map(function (r, i) {
        return '<label class="crm-pickrow"><input type="checkbox" class="crm-tick" data-pick="' + i + '"' + (k.pick[r.name] ? ' checked' : '') + '> ' + esc(r.name) + '</label>';
      }).join('') + '</div>';
    }
    return h;
  }

  var packs = {
    canvas: function (st) {
      var k = pk(st), seg = H().seg;
      var h = '<div class="crm-part"><div class="crm-head"><h3 class="crm-h">Recipe packs</h3><span class="crm-words">' + back() + '</span></div>' +
        seg('ptab', [['packs', 'Packs'], ['here', 'Shared from here']], k.tab);
      if (!k.list) return h + '<p class="crm-mut" role="status">Opening packs…</p></div>';
      var mine = k.list.filter(function (p) { return p.from === 'here' && !p.file; }), theirs = k.list.filter(function (p) { return !(p.from === 'here' && !p.file); });
      h += '<div class="crm-packs">';
      if (k.tab === 'packs') {
        h += theirs.length ? theirs.map(function (p) { return packRow(st, p, k); }).join('') : '<p class="crm-none">No packs to bring in yet: none from your other campaigns, and none shared on this server.</p>';
        h += '</div><p><label class="crm-q crm-file">' + fa('file-arrow-up') + 'Bring one in from a file<input type="file" accept=".json,application/json" data-upload class="sr-only"></label></p>';
      } else {
        var n = st.data().recipes.length, name = (st.ctx.part && st.ctx.part.label) || 'Crafting recipes';
        h += mine.length ? mine.map(function (p) { return packRow(st, p, k); }).join('') : '<p class="crm-none">Nothing saved from this campaign yet.</p>';
        h += '</div><div class="crm-field crm-savepack"><span>Save as</span><span class="crm-words"><input class="crm-in" data-pack-name value="' + esc(name) + '" maxlength="80" aria-label="Pack name">' +
          '<label class="crm-mut"><input type="checkbox" class="crm-tick" data-pack-share> Share on this server</label>' +
          '<button type="button" class="crm-btn crm-btn-2" data-act="save-pack"' + (n ? '' : ' disabled') + '>Save the ' + plural(n, 'recipe') + ' as a pack</button></span></div>';
      }
      return h + '<p class="crm-help">A pack is a set of crafting recipes with their materials’ names and kinds, never page text. Share one from a campaign, or bring one in from a file; what comes in arrives as drafts.</p></div>';
    },
    inspector: function (st) {
      var k = pk(st), d = k.open;
      if (!d) return '<div class="crm-insp"><p class="crm-insp-h">Recipe packs</p><p>Look inside a pack to match its materials to yours and pick what to bring in. What you bring in joins this part as drafts.</p>' +
        '<p>A pack you save holds only recipe and material names, kinds and amounts. Sharing it lets other Directors on this server bring it in.</p></div>';
      var opts = '<option value="">Make it new</option>' + st.items.map(function (it) { return '<option value="' + esc(it.id) + '">' + esc(it.name) + '</option>'; }).join('');
      var h = '<div class="crm-insp"><p class="crm-insp-h">' + esc(d.name) + ' · ' + (d.recipes || []).length + '</p><p>Match its materials to yours. Unmatched ones are made as new Armory items only you see.</p><div>';
      (d.materials || []).forEach(function (mt, i) {
        h += '<div class="crm-match"><span>' + esc(mt.name) + '</span><span class="crm-mut">to</span><select data-match="' + i + '" aria-label="Match ' + esc(mt.name) + '">' +
          opts.replace('value="' + esc(k.matches[mt.name] || '') + '"', 'value="' + esc(k.matches[mt.name] || '') + '" selected') + '</select></div>';
      });
      var n = picked(k).length, all = (d.recipes || []).length;
      return h + '</div><p class="crm-insp-h">Bring in</p>' + H().seg('pick', [['all', 'All ' + all], ['pick', 'Pick']], k.pickAll ? 'all' : 'pick') +
        '<p>' + plural(n, 'recipe') + ' arrive as drafts. A recipe whose name you already use is left out, with the reason.</p></div>';
    },
    wire: function (st, el, m, mode) {
      var k = pk(st);
      if (!k.list && !k.busy) loadPacks(st);
      if (mode === 'canvas') {
        var n = picked(k).length;
        if (st.ctx.setPrimary) st.ctx.setPrimary(k.open && n ? 'Bring in ' + n + ' as drafts' : null, function () { bringIn(st); });
        el.querySelectorAll('[data-pick]').forEach(function (c) {
          c.addEventListener('change', function () { k.pick[k.open.recipes[+c.getAttribute('data-pick')].name] = c.checked; m.refocus = '[data-pick="' + c.getAttribute('data-pick') + '"]'; st.emit(); });
        });
        var up = el.querySelector('[data-upload]');
        if (up) up.addEventListener('change', function () {
          var f = up.files && up.files[0];
          if (!f) return;
          if (f.size > 1024 * 1024) { st.say('A pack file is at most 1 MB.', true); return; }
          f.text().then(function (text) {
            st.say('Reading the pack…');
            return st.api('POST', '/packs/file', text, true).then(function (p) { k.list = null; loadPacks(st); openPack(st, p.id); });
          }).catch(function (err) { st.say(err.message, true); });
        });
        return;
      }
      el.querySelectorAll('[data-match]').forEach(function (s) {
        s.addEventListener('change', function () { k.matches[k.open.materials[+s.getAttribute('data-match')].name] = s.value; });
      });
    },
    click: function (st, b) {
      var k = pk(st), act = b.getAttribute('data-act');
      if (b.getAttribute('data-seg') === 'ptab') { k.tab = b.getAttribute('data-v'); st.emit(); return; }
      if (b.getAttribute('data-seg') === 'pick') { k.pickAll = b.getAttribute('data-v') === 'all'; st.emit(); return; }
      if (b.hasAttribute('data-open')) { openPack(st, b.getAttribute('data-open')); return; }
      if (b.hasAttribute('data-share')) {
        var p = k.list.filter(function (x) { return x.id === b.getAttribute('data-share'); })[0];
        st.api('PUT', '/packs/' + encodeURIComponent(p.id), {name: p.name, shared: !p.shared}).then(function (got) {
          p.shared = got.shared; st.say(got.shared ? 'Shared. Directors on this server can bring it in.' : 'Not shared any more.'); st.emit();
        }, function (err) { st.say(err.message, true); });
        return;
      }
      if (b.hasAttribute('data-del')) {
        var q = k.list.filter(function (x) { return x.id === b.getAttribute('data-del'); })[0];
        if (!window.confirm('Delete the pack “' + q.name + '”? Recipes already brought in from it stay.')) return;
        st.api('DELETE', '/packs/' + encodeURIComponent(q.id)).then(function () { k.list = k.list.filter(function (x) { return x !== q; }); st.say('Deleted.'); st.emit(); },
          function (err) { st.say(err.message, true); });
        return;
      }
      if (act === 'save-pack') {
        var root = b.closest('.crm'), name = root.querySelector('[data-pack-name]').value.trim(), share = root.querySelector('[data-pack-share]').checked;
        if (!name) { st.say('Name the pack.', true); return; }
        b.disabled = true;
        st.api('POST', '/packs', {name: name, recipes: st.data().recipes, shared: share}).then(function () {
          st.say(share ? 'Saved and shared on this server.' : 'Saved. Share it when you want other Directors to bring it in.'); loadPacks(st);
        }, function (err) { b.disabled = false; st.say(err.message, true); });
      }
    }
  };

  window.CraftingPartMore = {generate: generate, ai: ai, packs: packs};
})();
