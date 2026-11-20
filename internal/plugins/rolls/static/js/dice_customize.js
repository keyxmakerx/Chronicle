/**
 * dice_customize.js — the Dice tab on the Customize page.
 *
 * The owner picks the campaign's die (set and colour), the detail, the crit
 * success and crit fail animations, whether players may pick their own and
 * which sets they may pick from, and the table's look (what a player who
 * hasn't picked starts on). Each pick saves the moment it is made with a
 * partial PUT to /campaigns/:id/rolls/settings; a failed save puts the pick
 * back. The pickers are drawn from Chronicle.dice.catalogue(), so a new set or
 * crit appears here with no change to this file.
 *
 * The preview throws real dice with the picks as they stand. Its dice are given
 * the campaign's look explicitly, so the owner's own personal picks never
 * stand in for it.
 */
(function () {
  'use strict';

  var C = window.Chronicle;
  var LOOKS = [['quick', 'Quick 2D'], ['rune', 'Rune 3D'], ['none', 'Numbers only']];
  var QUALITIES = [['auto', 'Auto'], ['basic', 'Basic'], ['hd', 'HD']];
  var QUALITY_NOTE = {
    auto: 'Starts from what the device can draw and eases off if frames run long.',
    basic: 'Plainer dice that run well on any device.',
    hd: 'The full detail, on every device that can show it.'
  };
  // How long the "Saved" mark stays, in ms.
  var SAVED_STAY = 2200;

  function esc(s) { return C.escapeHtml(s); }

  function Dice(el) {
    this.el = el;
    this.base = '/campaigns/' + encodeURIComponent(el.getAttribute('data-campaign-id')) + '/rolls/settings';
    this.gm = el.getAttribute('data-gm-term') || 'Director';
    this.s = null;
    this.cat = null;
    this.saved = false;
    this.error = '';
    this.timer = 0;
    this.alive = true;
    this.pending = 0;
    this.handlers = [];
    this.wire();
    this.load();
  }

  Dice.prototype.on = function (type, fn) {
    this.el.addEventListener(type, fn);
    this.handlers.push([type, fn]);
  };

  Dice.prototype.destroy = function () {
    this.alive = false;
    clearTimeout(this.timer);
    var el = this.el;
    this.handlers.forEach(function (h) { el.removeEventListener(h[0], h[1]); });
    this.handlers = [];
  };

  Dice.prototype.load = function () {
    var self = this;
    var settings = C.apiFetch(this.base).then(function (r) { if (!r.ok) throw new Error('status ' + r.status); return r.json(); });
    var catalogue = C.dice && typeof C.dice.catalogue === 'function' ? C.dice.catalogue() : Promise.reject(new Error('no dice'));
    Promise.all([settings, catalogue]).then(function (v) {
      if (!self.alive) return;
      self.s = v[0];
      self.s.diceOffered = self.s.diceOffered || [];
      self.cat = v[1];
      self.render();
    }).catch(function () {
      if (!self.alive) return;
      self.el.innerHTML = '<p class="dcz-hint dcz-bad" role="alert">The dice settings couldn’t be read. Reload the page to try again.</p>';
    });
  };

  // ---------- What the catalogue says about the picks ----------

  Dice.prototype.defaults = function () { return this.cat.defaults || {}; };
  Dice.prototype.setOf = function (key) {
    return this.cat.sets.filter(function (x) { return x.key === key; })[0] || this.cat.sets[0];
  };
  // The set in use: the pick, else the default.
  Dice.prototype.curSet = function () { return this.setOf(this.s.diceSet || this.defaults().set); };
  // The colour in use, found the way the dice engine finds it: the pick, else the default, else
  // the set's first.
  Dice.prototype.curColour = function (set) {
    var want = this.s.diceColour || this.defaults().colour;
    return set.colours.filter(function (c) { return c.key === want; })[0] || set.colours[0];
  };
  Dice.prototype.curCrit = function (kind) {
    var list = this.cat.crits[kind], want = kind === 'succ' ? this.s.critSucc || this.defaults().succ : this.s.critFail || this.defaults().fail;
    return list.filter(function (c) { return c.key === want; })[0] || list[0];
  };
  Dice.prototype.offeredKeys = function () {
    var o = this.s.diceOffered;
    return o && o.length ? o : this.cat.sets.map(function (x) { return x.key; });
  };
  // The campaign's look as the dice take it.
  Dice.prototype.look = function () {
    var set = this.curSet();
    return { set: set.key, colour: this.curColour(set).key, succ: this.curCrit('succ').key, fail: this.curCrit('fail').key, quality: this.s.quality || 'auto' };
  };

  // ---------- Drawing ----------

  function swatch(c, size) {
    var g = c.swatch || ['#888', '#444'];
    return '<span class="dcz-sw' + (size ? ' dcz-sw-' + size : '') + '" style="background:linear-gradient(135deg,' + esc(g[0]) + ',' + esc(g[g.length - 1]) + ')" aria-hidden="true"></span>';
  }

  Dice.prototype.setRows = function () {
    var cur = this.curSet().key;
    return this.cat.sets.map(function (x) {
      var on = x.key === cur;
      return '<label class="dcz-row' + (on ? ' is-on' : '') + '"><input type="radio" name="dcz-set" value="' + esc(x.key) + '" data-dcz-set="' + esc(x.key) + '"' + (on ? ' checked' : '') + '>' +
        '<span class="dcz-main"><span class="dcz-nm">' + esc(x.name) + '</span>' + (x.note ? '<span class="dcz-what">' + esc(x.note) + '</span>' : '') + '</span>' +
        '<span class="dcz-sws" aria-hidden="true">' + x.colours.slice(0, 4).map(function (c) { return swatch(c, 's'); }).join('') + '</span></label>';
    }).join('');
  };

  Dice.prototype.colourButtons = function () {
    var set = this.curSet(), cur = this.curColour(set).key;
    return set.colours.map(function (c) {
      return '<button type="button" class="dcz-col" data-dcz-colour="' + esc(c.key) + '" aria-pressed="' + (c.key === cur) + '" aria-label="' + esc(c.name) + '" title="' + esc(c.name) + '">' + swatch(c) + '</button>';
    }).join('');
  };

  Dice.prototype.critRows = function (kind) {
    var cur = this.curCrit(kind).key, name = 'dcz-' + kind;
    return this.cat.crits[kind].map(function (c) {
      var on = c.key === cur;
      return '<label class="dcz-row' + (on ? ' is-on' : '') + '"><input type="radio" name="' + name + '" value="' + esc(c.key) + '" data-dcz-' + kind + '="' + esc(c.key) + '"' + (on ? ' checked' : '') + '>' +
        '<span class="dcz-main"><span class="dcz-nm">' + esc(c.name) + '</span><span class="dcz-what">' + esc(c.what) + '</span></span></label>';
    }).join('');
  };

  function seg(attr, list, cur, label) {
    return '<div class="dcz-seg" role="group" aria-label="' + esc(label) + '">' + list.map(function (l) {
      return '<button type="button" ' + attr + '="' + l[0] + '" aria-pressed="' + (cur === l[0]) + '">' + l[1] + '</button>';
    }).join('') + '</div>';
  }

  Dice.prototype.offerList = function () {
    var offered = this.offeredKeys(), own = this.curSet().key;
    return this.cat.sets.map(function (x) {
      var isOwn = x.key === own;
      return '<label class="dcz-chk"><input type="checkbox" data-dcz-offer="' + esc(x.key) + '"' + (offered.indexOf(x.key) >= 0 ? ' checked' : '') + (isOwn ? ' disabled' : '') + '>' +
        '<span>' + esc(x.name) + (isOwn ? ' <span class="dcz-what">the campaign’s die</span>' : '') + '</span></label>';
    }).join('');
  };

  Dice.prototype.render = function () {
    if (!this.s || !this.cat) return;
    var keep = document.activeElement && this.el.contains(document.activeElement) ? this.focusKey(document.activeElement) : null;
    var s = this.s, set = this.curSet(), colour = this.curColour(set);
    var status = this.error ? '<span class="dcz-bad" role="alert">' + esc(this.error) + '</span>' : this.saved ? '<span class="dcz-ok"><i class="fa-solid fa-check" aria-hidden="true"></i> Saved</span>' : '';
    this.el.innerHTML =
      '<div class="dcz-head"><div><p class="dcz-lede">The dice every roll at this table is thrown with. Each pick saves as you make it.</p></div>' +
      '<p class="dcz-status" role="status" aria-live="polite">' + status + '</p></div>' +

      '<section class="dcz-sec" aria-labelledby="dcz-h-die"><h2 id="dcz-h-die" class="dcz-h">The campaign’s die</h2>' +
      '<div class="dcz-rows" role="radiogroup" aria-label="The campaign’s die">' + this.setRows() + '</div>' +
      '<div class="dcz-colours"><p class="dcz-lab" id="dcz-colour-lab">Colour: ' + esc(colour.name) + '</p>' +
      '<div class="dcz-cols" role="group" aria-labelledby="dcz-colour-lab">' + this.colourButtons() + '</div>' +
      (colour.note ? '<p class="dcz-hint">' + esc(colour.note) + '</p>' : '') + '</div></section>' +

      '<section class="dcz-sec" aria-labelledby="dcz-h-detail"><h2 id="dcz-h-detail" class="dcz-h">Detail</h2>' +
      seg('data-dcz-quality', QUALITIES, s.quality || 'auto', 'Detail') + '<p class="dcz-hint">' + QUALITY_NOTE[s.quality || 'auto'] + '</p></section>' +

      '<section class="dcz-sec" aria-labelledby="dcz-h-succ"><h2 id="dcz-h-succ" class="dcz-h">Crit success</h2>' +
      '<div class="dcz-rows" role="radiogroup" aria-label="Crit success">' + this.critRows('succ') + '</div></section>' +

      '<section class="dcz-sec" aria-labelledby="dcz-h-fail"><h2 id="dcz-h-fail" class="dcz-h">Crit fail</h2>' +
      '<div class="dcz-rows" role="radiogroup" aria-label="Crit fail">' + this.critRows('fail') + '</div></section>' +

      '<section class="dcz-sec" aria-labelledby="dcz-h-players"><h2 id="dcz-h-players" class="dcz-h">Players</h2>' +
      '<label class="dcz-switch"><input type="checkbox" role="switch" data-dcz-picks' + (s.playersPick ? ' checked' : '') + '><span class="dcz-track" aria-hidden="true"></span>' +
      '<span class="dcz-main"><span class="dcz-nm">Players may pick their own dice</span>' +
      '<span class="dcz-what">Each player can change the die, colours and animations for their own rolls. Everyone else still sees the campaign’s.</span></span></label>' +
      (s.playersPick ? '<div class="dcz-offer"><p class="dcz-lab">Sets on offer to players</p><div class="dcz-chks">' + this.offerList() + '</div></div>' : '') + '</section>' +

      '<section class="dcz-sec" aria-labelledby="dcz-h-look"><h2 id="dcz-h-look" class="dcz-h">The table’s dice</h2>' +
      '<p class="dcz-hint">What a player who hasn’t picked starts on.</p>' + seg('data-dcz-look', LOOKS, s.look || 'quick', 'The table’s dice') + '</section>' +

      '<section class="dcz-sec" aria-labelledby="dcz-h-try"><h2 id="dcz-h-try" class="dcz-h">Try it</h2>' +
      '<div class="dcz-try">' +
      '<button type="button" class="btn-secondary" data-dcz-roll="2d10">Roll 2d10</button>' +
      '<button type="button" class="btn-secondary" data-dcz-roll="d20">Roll a d20</button>' +
      '<button type="button" class="btn-secondary" data-dcz-roll="succ">Show crit success</button>' +
      '<button type="button" class="btn-secondary" data-dcz-roll="fail">Show crit fail</button></div>' +
      '<p class="dcz-hint">Thrown with the picks above. Under Calm the dice are set down without a throw, and with motion Off nothing is drawn.</p></section>';
    if (keep) {
      var back = this.el.querySelector('[' + keep[0] + '="' + keep[1] + '"]');
      if (back && !back.disabled) back.focus({ preventScroll: true });
    }
  };

  // The attribute and value that name a control, so focus can come back after a redraw. The values
  // are catalogue keys and fixed words, never free text.
  Dice.prototype.focusKey = function (t) {
    var names = ['data-dcz-set', 'data-dcz-colour', 'data-dcz-quality', 'data-dcz-succ', 'data-dcz-fail', 'data-dcz-picks', 'data-dcz-offer', 'data-dcz-look', 'data-dcz-roll'];
    for (var i = 0; i < names.length; i++) {
      if (t.hasAttribute && t.hasAttribute(names[i])) return [names[i], t.getAttribute(names[i])];
    }
    return null;
  };

  // ---------- Saving ----------

  // save puts a change at once and keeps it only if the server took it.
  Dice.prototype.save = function (change) {
    var self = this, was = JSON.parse(JSON.stringify(this.s));
    Object.keys(change).forEach(function (k) { self.s[k] = change[k]; });
    this.saved = false;
    this.error = '';
    this.render();
    var n = ++this.pending;
    C.apiFetch(this.base, { method: 'PUT', body: change }).then(function (res) {
      if (!res.ok) return res.json().catch(function () { return {}; }).then(function (b) { throw new Error((b && b.message) || 'status ' + res.status); });
      return res.json();
    }).then(function (fresh) {
      if (!self.alive) return;
      // A later pick is still in flight: its answer will have the last word.
      if (n === self.pending) { self.s = fresh; self.s.diceOffered = self.s.diceOffered || []; }
      self.saved = true;
      self.error = '';
      self.render();
      clearTimeout(self.timer);
      self.timer = setTimeout(function () { self.saved = false; if (self.alive) self.render(); }, SAVED_STAY);
    }).catch(function (e) {
      if (!self.alive) return;
      self.s = was;
      self.error = 'That wasn’t saved. ' + (e && e.message && !/^status/.test(e.message) ? e.message : 'Try again.');
      self.render();
    });
  };

  // The campaign's own set is always on offer, so choosing one that isn't adds it.
  Dice.prototype.pickSet = function (key) {
    var change = { diceSet: key, diceColour: '' };
    var o = this.s.diceOffered;
    if (o && o.length && o.indexOf(key) < 0) change.diceOffered = o.concat([key]);
    this.save(change);
  };

  Dice.prototype.toggleOffer = function (key, on) {
    var cur = this.offeredKeys(), own = this.curSet().key;
    var next = this.cat.sets.map(function (x) { return x.key; }).filter(function (k) { return k === own || (k === key ? on : cur.indexOf(k) >= 0); });
    // Every set on offer is saved as "all", so a later new set is on offer too.
    this.save({ diceOffered: next.length === this.cat.sets.length ? [] : next });
  };

  // ---------- Preview ----------

  Dice.prototype.preview = function (what, from) {
    if (!C.dice || typeof C.dice.roll !== 'function') return;
    var table = this.look(), opts = { from: from, table: table, describe: function (g, v) { return 'Test throw: ' + (v || []).join(', ') + '.'; } };
    var dice;
    if (what === '2d10') dice = [{ sides: 10 }, { sides: 10 }];
    else if (what === 'd20') dice = [{ sides: 20 }];
    else if (what === 'succ') dice = [{ sides: 20, value: 20, crit: 'succ' }];
    else dice = [{ sides: 20, value: 1, crit: 'fail' }];
    C.dice.roll([{ label: 'Test throw', dice: dice }], opts).then(function () {}, function () {});
  };

  Dice.prototype.wire = function () {
    var self = this;
    this.on('change', function (ev) {
      var t = ev.target, a;
      if (!self.s || !t || !t.getAttribute) return;
      if ((a = t.getAttribute('data-dcz-set')) !== null) self.pickSet(a);
      else if ((a = t.getAttribute('data-dcz-succ')) !== null) self.save({ critSucc: a });
      else if ((a = t.getAttribute('data-dcz-fail')) !== null) self.save({ critFail: a });
      else if (t.hasAttribute('data-dcz-picks')) self.save({ playersPick: !!t.checked });
      else if ((a = t.getAttribute('data-dcz-offer')) !== null) self.toggleOffer(a, !!t.checked);
    });
    this.on('click', function (ev) {
      var b = ev.target && ev.target.closest ? ev.target.closest('button') : null, a;
      if (!b || !self.s) return;
      if ((a = b.getAttribute('data-dcz-colour')) !== null) self.save({ diceColour: a });
      else if ((a = b.getAttribute('data-dcz-quality')) !== null) self.save({ quality: a });
      else if ((a = b.getAttribute('data-dcz-look')) !== null) self.save({ look: a });
      else if ((a = b.getAttribute('data-dcz-roll')) !== null) self.preview(a, b);
    });
  };

  C.register('dice-customize', {
    init: function (el) {
      if (el._dice || !el.getAttribute('data-campaign-id')) return;
      el._dice = new Dice(el);
    },
    destroy: function (el) {
      if (el._dice) { el._dice.destroy(); el._dice = null; }
    }
  });
})();
