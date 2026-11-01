/**
 * crafting_turns.js -- the Crafting block's experimenting, breaking down and
 * helping folds (window.CraftingTurns)
 *
 * Pure HTML for the signed "Crafting at the bench" panels: what a hero puts
 * into a try and what it came to, breaking a made item down, and a hand on a
 * party member's project. crafting_block.js owns the state and the clicks;
 * with motion on it plays the matching moment at the bench before it shows
 * a result panel, and under Calm, Off or reduced motion the panel is all
 * there is. A near miss shows only the hint the server sent, never a name.
 */
(function () {
  'use strict';
  var esc = window.CraftingLib.esc;

  function line(k, small, v) {
    return '<li><span class="crp-k">' + esc(k) + (small ? '<small>' + esc(small) + '</small>' : '') + '</span><span class="crp-v">' + esc(v) + '</span></li>';
  }
  function list(xs) { return xs && xs.length ? xs.join(' · ') : 'Nothing'; }
  function total(picks) { var n = 0; Object.keys(picks).forEach(function (id) { n += picks[id]; }); return n; }

  /* What can go into a try: everything in the pack, with how many are in. */
  function chips(pack, picks) {
    return '<div class="crp-chips" role="group" aria-label="What goes in">' + (pack.items || []).map(function (it) {
      var n = picks[it.itemId] || 0;
      return '<button type="button" class="crp-chip" data-act="tpick" data-id="' + esc(it.itemId) + '" aria-pressed="' + (n > 0) + '" aria-label="' +
        esc(it.name + ': ' + n + ' of ' + it.qty + ' going in. ' + (n >= it.qty ? 'Press to take them all out.' : 'Press to put in one more.')) + '">' +
        esc(it.name) + ' <small>' + n + ' of ' + it.qty + '</small></button>';
    }).join('') + '</div>';
  }

  /* "Try something new": the pick, or a line saying the try is spent. */
  function tryFold(D, picks, busy) {
    var who = D.character.name;
    if (!D.try.ready) return '<p class="cra-fine crp-tried">' + esc(who) + ' has experimented since the last respite. The next try comes with the next respite.</p>';
    var n = total(picks);
    return '<div class="crp-fold crp-try"><h4>Experiment at the bench</h4>' +
      '<p class="cra-fine">Pick what ' + esc(who) + ' puts in. If it matches a recipe ' + esc(who) + ' doesn’t know yet, ' + esc(who) + ' may find it. The book never names a recipe before it is found.</p>' +
      ((D.pack.items || []).length ? chips(D.pack, picks) : '<p class="cra-fine">' + esc(who) + '’s pack is empty.</p>') +
      '<ul class="crp-lines">' + line('If nothing comes of it', 'Some or all of what goes in can be used up. Each recipe says how much.', 'At most ' + n + (n === 1 ? ' item' : ' items')) +
      line('The try', 'Each hero gets one try at each respite the Director logs.', 'Ready now') + '</ul>' +
      '<div class="crp-foot"><p class="cra-fine">What is used up leaves ' + esc(who) + '’s pack now.</p>' +
      '<button type="button" class="btn btn-secondary btn-sm" data-act="tclear" data-id="try"' + (n ? '' : ' disabled') + '>Not now</button>' +
      '<button type="button" class="btn btn-primary btn-sm" data-act="ttry" data-id="try"' + (n && !busy ? '' : ' disabled') + '>Try it</button></div></div>';
  }

  var COST = {used: 'a failed try uses up what went in', half: 'a failed try uses up half of what went in', kept: 'a failed try keeps everything'};
  var HEAD = {found: 'Experiment: a discovery', close: 'Experiment: close, but not yet', nothing: 'Experiment: nothing comes of it'};

  /* What the bench says as a try ends, the same words the panel leads with. */
  function triedWords(res, who) {
    if (res.outcome === 'found') return who + ' found a recipe: ' + res.recipe + '.' + (res.party ? ' The whole party learns it.' : '');
    if (res.outcome === 'close') return 'Something stirs in the flask, then fades.' + (res.hint ? ' “' + res.hint + '”' : '');
    return 'The mixture settles, and nothing comes of it.';
  }

  /* A fold that holds the bench while a moment plays; the block adds the
     panel under it when the moment ends. */
  function playing(head) { return '<div class="crp-fold" aria-live="polite"><h4>' + esc(head) + '</h4><div class="crp-bench" data-moment></div></div>'; }

  /* What a try came to, or the bench playing it. */
  function triedFold(res, who, play) {
    if (play) return playing(HEAD[res.outcome]);
    var h = '<div class="crp-fold" aria-live="polite"><h4>' + esc(HEAD[res.outcome]) + '</h4>';
    var big = res.outcome === 'found' ? who + ' found a recipe: ' + res.recipe + '.' : triedWords(res, who).split(' “')[0];
    h += '<p class="crp-big">' + esc(big) + '</p>' + (res.hint ? '<p class="crp-book">“' + esc(res.hint) + '”</p>' : '') + '<ul class="crp-lines">';
    if (res.outcome === 'found') {
      h += line('Who learns it', res.party ? 'the recipe says the whole party learns it' : 'the recipe says the maker learns it', list(res.learned)) +
        line('What went in', 'a discovery uses up what went in', list(res.used));
    } else {
      h += line('Used up', COST[res.cost] || '', list(res.used)) + (res.back.length ? line('Back in the pack', '', list(res.back)) : '');
    }
    h += '</ul><div class="crp-foot"><p class="cra-fine">' + esc(res.outcome === 'found' ? 'It is in the recipe list below now.' :
      res.outcome === 'close' ? 'The hint is all the book gives. The recipe stays unnamed until it is found.' : 'Nothing the book knows came of this mix.') + '</p>' +
      '<button type="button" class="btn btn-secondary btn-sm" data-act="tdone" data-id="try">Done</button>' +
      (res.outcome === 'found' ? '<button type="button" class="btn btn-primary btn-sm" data-act="tstart" data-id="' + esc(res.recipeId) + '">Start it</button>' : '') + '</div>';
    return h + '</div>';
  }

  /* The copies a hero could break: each remembered copy, then a plain one
     when they hold more than the copies account for. */
  function breakable(pack, id, keptOf, keptName) {
    var ks = keptOf(pack, id), held = 0, rem = 0, out = [];
    (pack.items || []).forEach(function (it) { if (it.itemId === id) held += it.qty; });
    ks.forEach(function (k) {
      rem += k.qty;
      out.push({id: k.id, name: k.by ? 'The one ' + (k.how === 'harvested' ? 'harvested' : 'made') + ' by ' + k.by + (k.marks && k.marks.length || k.quality ? ' (' + keptName(k) + ')' : '') : keptName(k)});
    });
    if (held > rem) out.push({id: 0, name: 'A plain one'});
    return {held: held, options: out};
  }

  /* Breaking one made item down, from its recipe's row. */
  function breakFold(r, who, b, copy, busy) {
    var thing = r.makes || r.name;
    return '<div class="crp-fold"><h4>Break down ' + esc(thing.match(/^[aeiou]/i) ? 'an ' : 'a ') + esc(thing) + '</h4><ul class="crp-lines">' +
      '<li><span class="crp-k"><label for="crk-brk-' + esc(r.id) + '">Which one</label><small>' + esc(who) + ' holds ' + b.held + '</small></span><span class="crp-v">' +
      '<select class="cra-in cra-sm" id="crk-brk-' + esc(r.id) + '" data-brk="' + esc(r.id) + '">' + b.options.map(function (o) {
        return '<option value="' + o.id + '"' + (String(o.id) === String(copy) ? ' selected' : '') + '>' + esc(o.name) + '</option>';
      }).join('') + '</select></span></li>' +
      line('Gives back', 'the recipe says what breaking it down returns', list(r.salvage)) + '</ul>' +
      '<div class="crp-foot"><p class="cra-fine">The ' + esc(thing) + ' is gone for good, and what it remembered goes with it. This takes no roll.</p>' +
      '<button type="button" class="btn btn-secondary btn-sm" data-act="close" data-id="' + esc(r.id) + '">Not now</button>' +
      '<button type="button" class="btn btn-danger btn-sm" data-act="bgo" data-id="' + esc(r.id) + '"' + (busy ? ' disabled' : '') + '>Break it down</button></div></div>';
  }

  function brokeWords(res, who) {
    return res.back.length ? 'Broken down. ' + list(res.back) + (res.back.length === 1 && /^1 ×/.test(res.back[0]) ? ' is' : ' are') + ' back in ' + who + '’s pack.' : 'Broken down.';
  }

  function brokeFold(res, who, id, play) {
    if (play) return playing('Breaking down ' + res.item);
    return '<div class="crp-fold" aria-live="polite"><h4>Broken down: ' + esc(res.item) + '</h4>' + '<ul class="crp-lines">' + line('Back in ' + who + '’s pack', '', list(res.back)) + '</ul>' +
      '<div class="crp-foot"><p class="cra-fine">' + esc(res.notes.join(' ')) + '</p>' +
      '<button type="button" class="btn btn-secondary btn-sm" data-act="bdone" data-id="' + esc(id) + '">Done</button></div></div>';
  }

  /* A party member's project this hero may lend a hand on. */
  function helpRow(hv, who, open, busy, progress) {
    var p = hv.project, id = p.id, h = '<li data-hp="' + esc(id) + '"><div class="crp-row is-pr"><div class="crp-nm"><b>' + esc(p.hero + '’s ' + p.name) + '</b><span class="cra-fine">' +
      esc([p.kind, 'others can help'].filter(Boolean).join(' · ')) + '</span></div>' + progress(p.points, p.goal, '', p.hero + '’s ' + p.name) + '<div class="crp-acts">';
    if (hv.helped) h += '<span class="crp-why">' + esc(who) + ' helped this respite</span>';
    else {
      h += '<span class="crp-pill is-ok">' + esc(who) + ' can help</span><button type="button" class="btn btn-primary btn-sm" data-act="help" data-id="' + esc(id) +
        '" aria-expanded="' + (open === 'help') + '">Help</button>';
    }
    h += '</div></div>';
    if (open === 'help' && !hv.helped) {
      h += '<div class="crp-fold"><h4>Help ' + esc(p.hero) + '</h4><ul class="crp-lines">' +
        line(who + '’s help', 'one extra roll on this project, ' + (p.roll || 'a plain roll').replace(/ \+ /, ' + ' + who + '’s ') + ', at ' + p.hero + '’s bench and its upgrades', '+1 roll') +
        line('How often', 'each hero can help a project once at each respite; ' + who + '’s own projects keep their rolls', 'Once this respite') + '</ul>' +
        '<div class="crp-foot"><p class="cra-fine">The points go to ' + esc(p.hero) + '’s project, and the made item to ' + esc(p.hero) + '’s pack. Only recipes that let others help show here.</p>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="close" data-id="' + esc(id) + '">Not now</button>' +
        '<button type="button" class="btn btn-primary btn-sm" data-act="hgo" data-id="' + esc(id) + '"' + (busy ? ' disabled' : '') + '>Help and roll</button></div></div>';
    }
    return h;
  }

  window.CraftingTurns = {tryFold: tryFold, triedFold: triedFold, triedWords: triedWords, breakable: breakable, breakFold: breakFold,
    brokeFold: brokeFold, brokeWords: brokeWords, helpRow: helpRow, playing: playing, total: total};
})();
