/**
 * crafting_upgrades.js -- a bench's upgrade ladders (window.CraftingUpgrades)
 *
 * Under the bench in the Benches tab: each piece of the bench (a mortar, a
 * burner) climbs a short ladder of levels, as in the signed rules editor
 * mockup's bench upgrades. A level changes the brew in plain words the
 * Director keeps for the table, or as a rule in the same words as a
 * recipe's, which every project rolled at this bench carries out once the
 * party has that level. The first level is where every bench starts.
 *
 * The Director edits in place; each change goes to opts.onChange, which
 * saves the whole ladder with the bench's version (crafting_benches.js).
 * The server cleans it the same way (upgrades.go), so the limits here match.
 */
window.CraftingUpgrades = (function () {
  'use strict';
  var L = window.CraftingLib, esc = L.esc;

  var LIMITS = {pieces: 6, levels: 5, name: 60, text: 240};
  var HOW = [['recipe', 'Made from a recipe'], ['bought', 'Bought'], ['given', 'Found or given by you']];
  var TIMES = ['dawn', 'day', 'dusk', 'night'];
  var CHANCES = ['always', '1 in 2', '1 in 3', '1 in 4', '1 in 6', '1 in 10', '1 in 20'];
  /* The effects an upgrade offers in its pickers: the ones that make a
     piece better. A rule the AI wrote with another effect is kept as it is. */
  var THENS = [['edge', 'Edge on the roll'], ['points', 'Project points'], ['extra', 'An extra one'], ['fine', 'Comes out fine']];

  function opt(v, label, on) { return '<option value="' + esc(v) + '"' + (on ? ' selected' : '') + '>' + esc(label) + '</option>'; }

  /* ---------- Words: a level reads as a short line and a sentence ---------- */
  function condTitle(c) {
    switch (c && c.kind) {
      case 'time': return 'At ' + c.time;
      case 'maker': return 'Maker knows ' + c.skill;
      case 'moon': return (c.moon || 'A moon') + ' is ' + (c.phase || 'full');
      case 'event': return c.event;
      case 'rolled': return (c.result || '') + ' comes up';
      case 'weather': return c.weather;
      case 'season': return c.season;
      case 'bench': return 'At the ' + c.bench;
      case 'ingredient': return 'An ingredient is ' + c.mark;
    }
    return 'Every time';
  }
  function thenTitle(t) {
    var n = +(t && t.amount) || 0;
    switch (t && t.kind) {
      case 'edge': return 'Edge on the roll';
      case 'bane': return 'Bane on the roll';
      case 'points': return (n < 0 ? '' : '+') + n + ' project points';
      case 'extra': return n > 1 ? n + ' extra' : 'One extra';
      case 'fine': return 'Comes out fine';
      case 'becomes': return 'Becomes ' + t.item;
      case 'mark': return 'Marked ' + t.mark;
      case 'quirk': return 'A quirk';
      case 'mishap': return 'A mishap';
      case 'discover': return 'Discovers ' + t.recipe;
      case 'say': return 'The bench speaks';
      case 'only': return 'Only then';
      case 'cannot': return 'Can’t be made';
    }
    return '';
  }
  function chanceOf(r) { return r.chance && r.chance !== 'always' ? r.chance : ''; }
  /* The line under a step on the ladder. */
  function short(l, i) {
    if (!i) return 'Where every bench starts.';
    if (!l.rule) return l.changes || 'Nothing written yet.';
    var r = l.rule, c = (r.when || [])[0], ch = chanceOf(r);
    return (c && c.kind !== 'always' ? condTitle(c) + ': ' : '') + (ch ? ch + ', ' : '') + thenTitle(r.then).toLowerCase();
  }
  /* The rule as one sentence, as the recipe editor words it; names keep
     their capitals. */
  function low(s) { s = String(s || ''); return s.charAt(0).toLowerCase() + s.slice(1); }
  function condText(c) {
    if (c.kind === 'time') return 'it is made at ' + c.time;
    if (c.kind === 'maker') return 'the maker knows ' + c.skill;
    return condTitle(c);
  }
  function sentence(r) {
    var ch = chanceOf(r), when = (r.when || []).filter(function (c) { return c.kind !== 'always'; });
    return (when.length ? 'When ' + when.map(condText).join(' and ') + ', ' : 'Every time, ') +
      (ch ? 'there’s a ' + ch + ' chance: ' : '') + low(thenTitle(r.then)) + '.';
  }

  /* ---------- Shapes ---------- */
  function newLevel(i) { return i ? {name: 'Level ' + (i + 1), changes: '', how: 'given'} : {name: 'Plain', how: 'start'}; }
  function newPiece(n) { return {name: 'Piece ' + n, have: 0, levels: [newLevel(0), newLevel(1)]}; }
  function newRule() { return {when: [{kind: 'always'}], then: {kind: 'edge'}, chance: 'always', players: 'shown'}; }
  function upgradesOf(b) {
    if (!b.upgrades || !Array.isArray(b.upgrades.pieces)) b.upgrades = {pieces: []};
    return b.upgrades;
  }
  /* The When picker's value for a rule's first condition: a time, a skill,
     always, or "keep" for a condition the AI wrote that the picker lacks. */
  function whenKey(r) {
    var c = (r.when || [])[0] || {kind: 'always'};
    if (r.when && r.when.length > 1) return 'keep';
    if (c.kind === 'always') return 'always';
    if (c.kind === 'time') return 'time:' + c.time;
    if (c.kind === 'maker') return 'maker:' + c.skill;
    return 'keep';
  }

  function mount(el, b, opts) {
    var picks = [], skills = (opts && opts.skills) || [];
    function U() { return upgradesOf(b); }
    function changed() { if (opts && opts.onChange) opts.onChange(); }
    function pickOf(pi) {
      var p = U().pieces[pi], k = picks[pi];
      if (k == null) k = Math.min(p.have + 1, p.levels.length - 1);
      return Math.max(0, Math.min(k, p.levels.length - 1));
    }

    function whenPicker(r, pi) {
      var k = whenKey(r), h = '<select class="cra-in cra-sm" data-u="when" data-p="' + pi + '" aria-label="When">';
      if (k === 'keep') h += opt('keep', (r.when || []).map(condTitle).join(' and ') + ' (as written)', true);
      h += opt('always', 'Every time', k === 'always');
      h += '<optgroup label="Time of day">' + TIMES.map(function (t) { return opt('time:' + t, 'At ' + t, k === 'time:' + t); }).join('') + '</optgroup>';
      var list = skills.slice(), cur = k.indexOf('maker:') === 0 ? k.slice(6) : '';
      if (cur && list.indexOf(cur) < 0) list.unshift(cur);
      if (list.length) h += '<optgroup label="The maker knows">' + list.map(function (s) { return opt('maker:' + s, s, k === 'maker:' + s); }).join('') + '</optgroup>';
      return h + '</select>';
    }
    function thenPicker(r, pi) {
      var t = r.then || {}, known = THENS.some(function (x) { return x[0] === t.kind; });
      var h = '<select class="cra-in cra-sm" data-u="then" data-p="' + pi + '" aria-label="Then">' + (known ? '' : opt('keep', thenTitle(t) + ' (as written)', true)) +
        THENS.map(function (x) { return opt(x[0], x[1], t.kind === x[0]); }).join('') + '</select>';
      if (t.kind === 'points' || t.kind === 'extra') {
        var n = +t.amount || (t.kind === 'points' ? 2 : 1);
        h += '<span class="cra-ustep" role="group" aria-label="How many"><button type="button" class="cra-ib" data-u="less" data-p="' + pi + '" aria-label="Fewer">−</button>' +
          '<output>' + (t.kind === 'points' && n > 0 ? '+' : '') + n + '</output><button type="button" class="cra-ib" data-u="more" data-p="' + pi + '" aria-label="More">+</button></span>';
      }
      return h;
    }
    function levelEd(p, pi, li) {
      var l = p.levels[li], h = '<div class="cra-ulv"><label class="cra-fl"><span>Level ' + (li + 1) + ' name</span><input class="cra-in" data-u="lname" data-p="' + pi + '" maxlength="' + LIMITS.name + '" value="' + esc(l.name) + '"></label>';
      if (!li) return h + '<p class="cra-fine">Where every bench starts: nothing to come by, and nothing it changes.</p></div>';
      var rule = l.rule;
      h += '<div class="cra-ctl"><span>What changes</span><div class="cra-seg" role="group" aria-label="What changes"><button type="button" data-u="words" data-p="' + pi + '" aria-pressed="' + !rule + '">In words</button>' +
        '<button type="button" data-u="rule" data-p="' + pi + '" aria-pressed="' + !!rule + '">As a rule</button></div></div>';
      if (rule) {
        h += '<div class="cra-urule"><span class="cra-uw">When</span>' + whenPicker(rule, pi) + '<span class="cra-uw">Then</span>' + thenPicker(rule, pi) +
          '<span class="cra-uw">Chance</span><select class="cra-in cra-sm" data-u="chance" data-p="' + pi + '" aria-label="Chance">' + CHANCES.map(function (c) { return opt(c, c === 'always' ? 'Always' : c, (rule.chance || 'always') === c); }).join('') + '</select></div>' +
          '<p class="cra-fine cra-usent">' + esc(sentence(rule)) + ' Every project rolled at this bench carries it out once they have this level.</p>';
      } else {
        h += '<label class="cra-fl"><span class="cra-sr">What changes, in words</span><textarea class="cra-in" rows="2" data-u="changes" data-p="' + pi + '" maxlength="' + LIMITS.text + '" placeholder="Grinding takes one fewer turn.">' + esc(l.changes || '') + '</textarea>' +
          '<em>Words are for you and the table; the roll doesn’t read them.</em></label>';
      }
      h += '<div class="cra-ctl"><span>How they get it</span><div class="cra-row cra-uhow">' + HOW.map(function (o) {
        return '<button type="button" class="cra-chip cra-chip-sm" data-u="how" data-how="' + o[0] + '" data-p="' + pi + '" aria-pressed="' + (l.how === o[0]) + '">' + esc(o[1]) + '</button>';
      }).join('') + '</div></div>' +
        '<label class="cra-fl"><span>Note</span><input class="cra-in" data-u="note" data-p="' + pi + '" maxlength="' + LIMITS.text + '" value="' + esc(l.note || '') + '" placeholder="Where they find it, or the recipe that makes it."></label>';
      h += '<div class="cra-row">';
      if (li > p.have) h += '<button type="button" class="btn btn-primary btn-sm" data-u="give" data-p="' + pi + '">Give it to them now</button>';
      else h += '<button type="button" class="btn btn-secondary btn-sm" data-u="take" data-p="' + pi + '">Take it back</button>';
      h += '<button type="button" class="btn btn-secondary btn-sm" data-u="dellevel" data-p="' + pi + '">Remove this level</button></div>';
      return h + '</div>';
    }
    function piece(p, pi) {
      var li = picks[pi] = pickOf(pi);
      var h = '<div class="cra-upc" data-p="' + pi + '"><div class="cra-uhd"><input class="cra-in cra-uname" data-u="pname" data-p="' + pi + '" maxlength="' + LIMITS.name + '" value="' + esc(p.name) + '" aria-label="Piece name">' +
        '<small>They have level ' + (p.have + 1) + ' of ' + p.levels.length + '</small>' +
        '<button type="button" class="cra-ib" data-u="delpiece" data-p="' + pi + '" aria-label="Remove ' + esc(p.name) + '" title="Remove this piece">✕</button></div>' +
        '<div class="cra-ladder" role="group" aria-label="' + esc(p.name) + ' levels">';
      p.levels.forEach(function (l, i) {
        h += '<button type="button" class="cra-lstep' + (i <= p.have ? ' got' : '') + '" data-u="pick" data-p="' + pi + '" data-l="' + i + '" aria-pressed="' + (i === li) + '">' +
          '<span class="cra-dot" aria-hidden="true">' + (i <= p.have ? '✓' : i + 1) + '</span><b>' + esc(l.name) + '</b><small>' + esc(short(l, i)) + '</small>' +
          (i <= p.have ? '<span class="cra-sr">(they have it)</span>' : '') + '</button>';
      });
      if (p.levels.length < LIMITS.levels) h += '<button type="button" class="cra-lstep cra-ladd" data-u="addlevel" data-p="' + pi + '"><span class="cra-dot" aria-hidden="true">+</span><b>Add a level</b></button>';
      return h + '</div>' + levelEd(p, pi, li) + '</div>';
    }
    function render() {
      var u = U(), h = '<div class="cra-uphd"><h3 class="cra-ph">Upgrades</h3><p class="cra-fine">Each piece can get better. A level changes the brew in the same words as a recipe’s rules, ' +
        'so a moonstone mortar can work only at night. Every recipe made here gets the levels they have.</p></div>';
      if (!u.pieces.length) h += '<p class="cra-empty">No upgrades yet. Add a piece, such as a mortar or a burner, and the levels it can climb.</p>';
      h += u.pieces.map(piece).join('');
      if (u.pieces.length < LIMITS.pieces) h += '<div class="cra-row"><button type="button" class="btn btn-secondary btn-sm" data-u="addpiece">Add a piece</button></div>';
      el.innerHTML = h;
    }
    function refocus(sel) { var f = el.querySelector(sel); if (f) f.focus(); }

    function onClick(e) {
      var t = e.target.closest('[data-u]');
      if (!t || !el.contains(t) || t.tagName !== 'BUTTON') return;
      var act = t.getAttribute('data-u'), pi = +t.getAttribute('data-p'), u = U(), p = u.pieces[pi], li = p ? pickOf(pi) : 0, l = p && p.levels[li];
      var focus = '[data-u="' + act + '"][data-p="' + pi + '"]';
      switch (act) {
        case 'pick': picks[pi] = +t.getAttribute('data-l'); focus = '[data-u="pick"][data-p="' + pi + '"][data-l="' + picks[pi] + '"]'; render(); refocus(focus); return;
        case 'addpiece': u.pieces.push(newPiece(u.pieces.length + 1)); picks[u.pieces.length - 1] = 1; focus = '[data-u="pname"][data-p="' + (u.pieces.length - 1) + '"]'; break;
        case 'delpiece': u.pieces.splice(pi, 1); picks.splice(pi, 1); focus = '[data-u="addpiece"]'; break;
        case 'addlevel': p.levels.push(newLevel(p.levels.length)); picks[pi] = p.levels.length - 1; focus = '[data-u="lname"][data-p="' + pi + '"]'; break;
        case 'dellevel': p.levels.splice(li, 1); if (p.have >= li) p.have--; picks[pi] = li - 1; focus = '[data-u="pick"][data-p="' + pi + '"][data-l="' + (li - 1) + '"]'; break;
        case 'words': l.rule = null; break;
        case 'rule': if (!l.rule) l.rule = newRule(); break;
        case 'how': l.how = t.getAttribute('data-how'); focus = '[data-how="' + l.how + '"][data-p="' + pi + '"]'; break;
        case 'give': p.have = li; focus = '[data-u="take"][data-p="' + pi + '"]'; if (opts.toast) opts.toast('They have the ' + l.name + ' now.'); break;
        case 'take': p.have = li - 1; focus = '[data-u="give"][data-p="' + pi + '"]'; break;
        case 'less': case 'more': {
          var th = l.rule.then, step = act === 'more' ? 1 : -1, n = (+th.amount || (th.kind === 'points' ? 2 : 1)) + step;
          if (th.kind === 'points') { if (!n) n += step; th.amount = Math.max(-10, Math.min(10, n)); } else th.amount = Math.max(1, Math.min(3, n));
          break;
        }
        default: return;
      }
      render(); refocus(focus); changed();
    }
    function onChange(e) {
      var t = e.target, act = t.getAttribute('data-u'), pi = +t.getAttribute('data-p');
      if (t.tagName !== 'SELECT' || !U().pieces[pi]) return;
      var r = U().pieces[pi].levels[pickOf(pi)].rule, v = t.value;
      if (!r) return;
      if (act === 'when' && v !== 'keep') {
        var i = v.indexOf(':'), kind = i < 0 ? v : v.slice(0, i), val = v.slice(i + 1);
        r.when = [kind === 'time' ? {kind: 'time', time: val} : kind === 'maker' ? {kind: 'maker', skill: val} : {kind: 'always'}];
      } else if (act === 'then' && v !== 'keep') {
        r.then = v === 'points' ? {kind: 'points', amount: 2} : v === 'extra' ? {kind: 'extra', amount: 1} : {kind: v};
      } else if (act === 'chance') r.chance = v;
      else return;
      render(); refocus('[data-u="' + act + '"][data-p="' + pi + '"]'); changed();
    }
    /* Typing saves without redrawing, so the caret stays; the ladder's own
       words follow along. A blank name isn't saved, as the bench's isn't. */
    function onInput(e) {
      var t = e.target, act = t.getAttribute('data-u'), pi = +t.getAttribute('data-p'), p = U().pieces[pi];
      if (!p || ['pname', 'lname', 'changes', 'note'].indexOf(act) < 0) return;
      var li = pickOf(pi), l = p.levels[li], v = t.value, step = el.querySelector('[data-u="pick"][data-p="' + pi + '"][data-l="' + li + '"]');
      if ((act === 'pname' || act === 'lname') && !v.trim()) return;
      if (act === 'pname') p.name = v;
      else if (act === 'lname') { l.name = v; if (step) step.querySelector('b').textContent = v; }
      else if (act === 'changes') { l.changes = v; if (step) step.querySelector('small').textContent = short(l, li); }
      else l.note = v;
      changed();
    }
    el.addEventListener('click', onClick);
    el.addEventListener('change', onChange);
    el.addEventListener('input', onInput);
    render();
    return {render: render};
  }

  return {mount: mount, sentence: sentence, short: short, LIMITS: LIMITS};
})();
