/**
 * shop_room_keeper.js -- the shop keeper's manner: what the keeper says at
 * each moment of a visit.
 *
 * A shop's saved layout may name a manner and hold the GM's own lines per
 * moment (layout.manner, layout.moments). The GM's lines for a moment always
 * win; a moment with none falls back to the manner's starting lines; with no
 * manner either, the keeper says nothing then. The keeper's general lines
 * (layout.lines, said when pressed) are separate and unchanged. The refuse
 * moment is what the keeper says when the shop won't serve the party; the
 * player Shops page shows it at a refusing shop's door.
 *
 * The moment keys and manner names must match the allowlists the server
 * validates saved rooms against (internal/plugins/armory/shop_room_validate.go).
 */
(function () {
  'use strict';

  // In visit order, with the label the Arrange panel shows.
  var MOMENTS = [
    ['hello', 'You walk up'],
    ['look', 'You pick something'],
    ['buy', 'You buy'],
    ['poor', 'You can’t afford it'],
    ['bye', 'You leave'],
    ['refuse', 'Won’t serve you']
  ];

  var MANNERS = {
    warm: { name: 'Warm', lines: {
      hello: ['Come in, come in, mind the step. What can I find for you?', 'There you are! Have a look round, take your time.'],
      look: ['Ah, that one has a story. Take your time with it.', 'Good eye. I made that one on a rainy week.'],
      buy: ['Lovely choice. I’ll wrap it so it travels well.', 'That’ll serve you well, I promise.'],
      poor: ['Short a coin or two? Come back when the purse is heavier, I’ll keep it aside.', 'Not today, love. It’ll still be here.'],
      bye: ['Safe roads, and come back and tell me how it went.', 'Mind how you go!'],
      refuse: ['I’m sorry, love, not after last time.', 'Not today. Please go.']
    } },
    gruff: { name: 'Gruff', lines: {
      hello: ['Touch nothing you won’t pay for.', 'Well? Buying or browsing?'],
      look: ['It’s sharp. That’s the point.', 'Careful with that.'],
      buy: ['Coin on the counter.', 'Fine. It’s yours.'],
      poor: ['Then put it down.', 'Come back with coin.'],
      bye: ['Door sticks. Shove it.', 'Hm.'],
      refuse: ['Your coin’s no good here.', 'Out.']
    } },
    shrewd: { name: 'Shrewd', lines: {
      hello: ['Customers! You have the look of people with gold to spend.', 'Welcome, welcome. Everything’s for sale, if the price is right.'],
      look: ['For you, a special price. Well. A price.', 'Ah, that one’s very popular. Very.'],
      buy: ['Pleasure doing business. Truly.', 'A fine bargain, for both of us. Mostly for me.'],
      poor: ['No credit. Not even for heroes. Especially not for heroes.', 'Come back richer, friend.'],
      bye: ['Tell your friends. Tell them the prices are fair.', 'Come again, and bring your purse.'],
      refuse: ['Bad for business, you are. Move along.', 'I don’t sell to people who don’t pay.']
    } },
    nervous: { name: 'Nervous', lines: {
      hello: ['Oh! Customers. Yes. Hello. Please don’t knock the jars.', 'H-hello! Is it busy out there? It’s not busy in here.'],
      look: ['Careful, careful, that one’s, um, mostly safe.', 'That one’s fine. Probably fine.'],
      buy: ['Sold! Oh good. That’s good.', 'Thank you! Don’t open it indoors.'],
      poor: ['I’m so sorry, I really can’t go lower, she checks the books.', 'Oh dear. Maybe next time?'],
      bye: ['Bye! Mind the, oh, you’ve found the step.', 'Come back! Or don’t, it’s fine.'],
      refuse: ['I c-can’t, she said not to let you in.', 'Please, I don’t want trouble.']
    } },
    grand: { name: 'Grand', lines: {
      hello: ['Welcome, travellers, to the finest shop this side of the mountains!', 'Enter, and be dazzled!'],
      look: ['Behold! Crafted by my own two hands, in a single sleepless night.', 'Ah, a piece of legend.'],
      buy: ['A purchase worthy of song.', 'Wield it with glory!'],
      poor: ['Alas, even legends must be paid for.', 'Return when fortune smiles on you!'],
      bye: ['Go forth, and speak my name in every tavern!', 'Farewell, heroes!'],
      refuse: ['This establishment does not serve the likes of you!', 'Begone, and take your reputation with you!']
    } }
  };
  var ORDER = ['warm', 'gruff', 'shrewd', 'nervous', 'grand'];

  // The lines the keeper picks from at a moment: the GM's own, else the manner's.
  function linesFor(manner, moments, moment) {
    var own = moments && moments[moment];
    if (own && own.length) return own;
    var m = MANNERS[manner];
    return m ? m.lines[moment] || [] : [];
  }

  // One line for a moment, never the same one twice in a row when there is a choice.
  var last = {};
  function pick(manner, moments, moment, rnd) {
    var ls = linesFor(manner, moments, moment);
    if (!ls.length) return '';
    var r = rnd || Math.random, i = Math.floor(r() * ls.length);
    if (ls.length > 1 && ls[i] === last[moment]) i = (i + 1) % ls.length;
    last[moment] = ls[i];
    return ls[i];
  }

  window.ShopRoomKeeper = { MOMENTS: MOMENTS, MANNERS: MANNERS, ORDER: ORDER, linesFor: linesFor, pick: pick };
})();
