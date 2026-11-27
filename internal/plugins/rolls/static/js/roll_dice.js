/**
 * roll_dice.js — Chronicle.rollDice, which 3D dice a roll is thrown in.
 *
 * The server settles each roll's dice when it is made and sends them as
 * rollerDice: the roller's own pick where the campaign lets players pick and
 * the set is on offer, else the campaign's. Every screen throws the roll in
 * those dice, so a player's pick goes with them. A roll from before rolls
 * carried them is thrown in the campaign's dice (or, for the viewer's own,
 * their own). Pure, so it is tested without a page.
 */
(function () {
  'use strict';

  var W = window;
  W.Chronicle = W.Chronicle || {};

  // Names go to the dice engine as file keys, so only the shape a key can
  // have is passed on; anything else is the engine's default.
  var KEY = /^[a-z0-9-]{0,24}$/;
  function key(v) { return typeof v === 'string' && KEY.test(v) ? v : ''; }

  // forRoll returns the dice engine's look ({ set, colour, succ, fail,
  // quality }) for one roll. camp is the campaign's dice in that shape and
  // mine the viewer's own; detail (quality) is always the campaign's, since
  // it is about the device, not the dice.
  function forRoll(roll, camp, mine) {
    camp = camp || {};
    var d = roll && roll.rollerDice;
    if (d && typeof d === 'object') {
      return { set: key(d.set), colour: key(d.colour), succ: key(d.critSucc), fail: key(d.critFail), quality: camp.quality || '' };
    }
    var base = roll && roll.mine && mine ? mine : camp;
    return { set: base.set || '', colour: base.colour || '', succ: base.succ || '', fail: base.fail || '', quality: camp.quality || '' };
  }

  // name says whose dice a roll was thrown in, as the details show it:
  // "Mira’s Clockwork · Brass", or "The campaign’s Neon · Blue". whose is the
  // roller's possessive ("Mira’s", "Your"). cat is the dice catalogue; without
  // it, or for a roll that carries no dice, it is ''. An empty set or colour
  // is the catalogue's first, as the engine draws it.
  function name(roll, cat, whose) {
    var d = roll && roll.rollerDice;
    if (!d || !cat || !cat.sets || !cat.sets.length) return '';
    var set = cat.sets.filter(function (s) { return s.key === d.set; })[0] || cat.sets[0];
    var cols = set.colours || [];
    var col = cols.filter(function (c) { return c.key === d.colour; })[0] || cols[0];
    var owner = d.own ? (whose || 'Their') + ' ' : 'The campaign’s ';
    return owner + set.name + (col ? ' · ' + col.name : '');
  }

  W.Chronicle.rollDice = { forRoll: forRoll, name: name };
})();
