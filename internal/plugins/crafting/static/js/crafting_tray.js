/**
 * crafting_tray.js -- the spoils tray's cards (window.CraftingTray)
 *
 * The Crafting block and the Director's Spoils tab draw the same tray, a card
 * per thing that dropped and a line per item, as the signed Crafting Spoils
 * mockup; only the buttons on each line differ, so each caller hands those
 * in. Everything shown comes from the server, which already put "An item you
 * haven't seen" or "Something the party found" where a player may not look.
 */
window.CraftingTray = (function () {
  'use strict';
  var esc = window.CraftingLib.esc;
  var CHECK = '<svg viewBox="0 0 20 20" width="18" height="18" aria-hidden="true"><path d="M4 10.5l4 4 8-9" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>';

  function plural(n, one, many) { return n + ' ' + (n === 1 ? one : many); }
  function signed(n) { return n < 0 ? '−' + (-n) : '+' + n; }

  /* How long ago, in the words a person uses. */
  function ago(iso, now) {
    var t = Date.parse(iso);
    if (!t) return '';
    var m = Math.max(0, Math.round(((now || Date.now()) - t) / 60000));
    if (m < 2) return 'just now';
    if (m < 60) return m + ' minutes ago';
    var h = Math.round(m / 60);
    if (h < 24) return plural(h, 'hour', 'hours') + ' ago';
    var d = Math.round(h / 24);
    return d === 1 ? 'yesterday' : d + ' days ago';
  }

  /* Where and when a spoil dropped: the fight's name, or by hand. A player
     who can't see the source gets only when. */
  function from(s, now) {
    return [s.origin || (s.byHand ? 'Put in by hand' : ''), ago(s.at, now)].filter(Boolean).join(', ');
  }

  /* How many lines wait in a tray. */
  function count(tray) {
    return (tray || []).reduce(function (n, s) { return n + s.lines.length; }, 0);
  }

  /* A harvest's three tiers as chips, the one the test landed in lit. */
  function tiers(l, hit) {
    return '<div class="crs-tiers">' + (l.tiers || []).map(function (t, i) {
      return '<span class="crs-tier' + (i === hit ? ' is-hit' : '') + '">' + esc(t.label) + ' <b>' + t.qty + '</b></span>';
    }).join('') + '</div>';
  }

  function how(l, hit) {
    if (l.mode !== 'harvest') return '<div class="crs-how">Take as it is</div>';
    return '<div class="crs-how">Harvest: ' + esc(l.roll) + tiers(l, hit) + '</div>';
  }

  function item(l) {
    var qty = l.mode === 'harvest' ? '' : ' × ' + l.qty;
    if (l.unseen) return '<div class="crs-it"><b class="is-unseen">' + esc(l.name) + qty + '</b><span class="cra-fine">Whoever takes it gets to see it</span></div>';
    return '<div class="crs-it"><b>' + esc(l.name) + qty + '</b>' + (l.kinds ? '<span class="cra-fine">' + esc(l.kinds) + '</span>' : '') + '</div>';
  }

  /* One card. o.acts(line) gives a line's buttons, o.fold(line) anything
     unfolded under it, o.head(spoil) the card's own buttons, o.under(spoil)
     anything unfolded under its head, o.hit(line) the tier a harvest landed
     in. */
  function card(s, o) {
    var name = '<b>' + esc(s.source) + (s.copies > 1 ? '<small>(' + s.copy + ' of ' + s.copies + ')</small>' : '') + '</b>';
    return '<div class="crs-spoil" data-spoil="' + esc(s.id) + '"><div class="crs-head">' + name + '<span class="cra-fine">' + esc(from(s, o.now)) + '</span>' +
      (o.head ? o.head(s) : '') + '</div>' + (o.under ? o.under(s) : '') + '<ul class="crs-lines">' + s.lines.map(function (l) {
        var hit = o.hit ? o.hit(l) : -1;
        return '<li data-line="' + l.id + '"><div class="crs-line">' + item(l) + how(l, hit) + '<div class="crp-acts">' + o.acts(l, s) + '</div></div>' + (o.fold ? o.fold(l, s) : '') + '</li>';
      }).join('') + '</ul></div>';
  }

  /* A roll's terms as rows: the dice's sum, then each signed modifier. */
  function terms(res) {
    return (res.terms || []).map(function (t, i) { return {text: t.text, n: i === 0 || t.n === 0 ? String(t.n) : signed(t.n)}; });
  }

  /* A harvest's test as the server rolled it, unfolded under its line until
     Done: the dice, the terms, the tier it landed in and what reached the
     pack. title heads it ("Harvest: Troll hide"). */
  function result(R, lineId, title) {
    var tier = (R.tiers || [])[R.tier || 0], h = '<div class="crp-fold" aria-live="polite"><h4>' + esc(title) + '</h4><div class="crp-result">' +
      '<div class="crp-dice" aria-label="The dice show ' + esc(R.dice.join(' and ')) + '">' + R.dice.map(function (d) { return '<span class="crp-die">' + d + '</span>'; }).join('') + '</div>' +
      '<ul class="crp-terms">' + terms(R).map(function (t) { return '<li><span>' + esc(t.text) + '</span><span>' + esc(t.n) + '</span></li>'; }).join('') +
      '<li class="is-tot"><span>The test</span><span>' + R.total + '</span></li></ul><div class="crp-outcome"><p class="cra-fine">' + esc(tier ? tier.label : '') +
      (!R.tier && R.qty > 0 ? ': the poorest outcome, which still pays.' : '.') + '</p>';
    if (R.qty > 0) h += '<div class="crp-made">' + CHECK + '<span>' + esc(R.got) + ' went to ' + esc(R.hero) + '’s pack.</span></div>';
    else h += '<div class="crp-made is-warn"><span>That tier pays nothing for this one.</span></div>';
    return h + '</div></div><div class="crp-foot"><p class="cra-fine">' + esc((R.notes || []).filter(function (n) { return n.indexOf('pays nothing') < 0; }).concat(['The line leaves the tray for everyone.']).join(' ')) + '</p>' +
      '<button type="button" class="btn btn-secondary btn-sm" data-act="sdone" data-id="' + esc(String(lineId)) + '">Done</button></div></div>';
  }

  /* The server's dice, thrown on screen over fold (a .crp-result): the terms
     and what they did wait until the dice land, and show anyway if the throw
     can't happen. */
  function throwDice(fold, res, name, said) {
    var dice = window.Chronicle && Chronicle.dice;
    if (!fold || !res.dice || !res.dice.length || !dice || typeof dice.roll !== 'function') return;
    var parts = fold.querySelectorAll('.crp-dice, .crp-terms, .crp-outcome'), shown = false;
    function show() { if (shown) return; shown = true; Array.prototype.forEach.call(parts, function (x) { x.style.visibility = ''; }); }
    Array.prototype.forEach.call(parts, function (x) { x.style.visibility = 'hidden'; });
    setTimeout(show, 8000);
    Promise.resolve().then(function () {
      return dice.roll([{label: name, dice: res.dice.map(function (v) { return {sides: res.sides, value: v, crit: 'none'}; })}], {
        from: fold,
        describe: function (g, values) { return name + ': ' + (values || []).join(' and ') + ', ' + (said || res.total + ' project points') + '.'; },
        onSettle: show
      });
    }).then(show, show);
  }

  return {ago: ago, from: from, count: count, tiers: tiers, card: card, terms: terms, result: result, throwDice: throwDice};
})();
