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
 * Each set is a tile with a picture of its own d20, drawn once by the dice
 * engine (Chronicle.dice.portrait) and kept for the page; without WebGL the
 * tile shows a flat d20 in the set's colours. The tray beside the tiles shows
 * the chosen die and throws it inside itself whenever a set or colour is
 * picked, or Roll again, Crit or Fumble is pressed. Its dice are given the
 * campaign's look explicitly, so the owner's own personal picks never stand
 * in for it. Under Calm the die is set down and fades in; with motion Off it
 * is shown at rest on its number.
 *
 * The tray is drawn once and kept: the rest of the tab is redrawn after each
 * pick, and redrawing the tray would throw away the die resting in it.
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
  // Picture sizes in CSS pixels: a tile's, and the tray's die at rest.
  var TILE_PX = 72, TRAY_PX = 140;

  function esc(s) { return C.escapeHtml(s); }

  // Matches dice.js: html[data-motion] is "calm" or "off"; a device asking for reduced motion is Off.
  function motion() {
    var m = document.documentElement.getAttribute('data-motion');
    if (m === 'off' || m === 'calm') return m;
    if (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return 'off';
    return 'full';
  }
  var GL = null;
  function hasGL() {
    if (GL === null) {
      try { var c = document.createElement('canvas'); GL = !!(c.getContext('webgl2') || c.getContext('webgl')); } catch (e) { GL = false; }
    }
    return GL;
  }

  // A flat d20 in a colourway's own colours: the stand-in when the engine can't draw. The colours are
  // the catalogue's hex values; anything else falls back to grey.
  function hex(c, d) { return /^#[0-9a-f]{3,8}$/i.test(c || '') ? c : d; }
  function light(c) {
    var h = c.slice(1); if (h.length < 6) h = h.split('').map(function (x) { return x + x; }).join('');
    var n = parseInt(h.slice(0, 6), 16);
    return (0.3 * (n >> 16 & 255) + 0.59 * (n >> 8 & 255) + 0.11 * (n & 255)) / 255 > 0.6;
  }
  function flatD20(colour, value) {
    var g = (colour && colour.swatch) || [], a = hex(g[0], '#9ca3af'), b = hex(g[g.length - 1], '#4b5563');
    var ink = light(a) ? '#2a2b30' : '#f4f4f5', n = String(value || 20);
    var svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">' +
      '<stop offset="0" stop-color="' + a + '"/><stop offset="1" stop-color="' + b + '"/></linearGradient></defs>' +
      '<g stroke="' + ink + '" stroke-opacity="0.55" stroke-width="1.6" stroke-linejoin="round" fill="none">' +
      '<polygon points="50,5 89,27.5 89,72.5 50,95 11,72.5 11,27.5" fill="url(#g)" stroke-width="2.4"/>' +
      '<polygon points="50,25 72,63 28,63"/>' +
      '<path d="M50 25L50 5M50 25L11 27.5M50 25L89 27.5M72 63L89 27.5M72 63L89 72.5M72 63L50 95M28 63L11 27.5M28 63L11 72.5M28 63L50 95"/></g>' +
      '<text x="50" y="55" text-anchor="middle" font-family="Georgia,serif" font-weight="700" font-size="' + (n.length > 1 ? 15 : 17) + '" fill="' + ink + '">' + n + '</text></svg>';
    return 'data:image/svg+xml,' + encodeURIComponent(svg);
  }

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
    // Pictures by set|colour|value|px: a data URL once drawn.
    this.pics = {};
    this.asked = {};
    // Pictures asked for but not drawn yet, by key, and whether one is being drawn now.
    this.wait = {};
    this.drawing = false;
    this.trayKey = '';
    this.throwN = 0;
    this.wire();
    this.load();
  }

  Dice.prototype.on = function (type, fn, target) {
    (target || this.el).addEventListener(type, fn);
    this.handlers.push([type, fn, target || this.el]);
  };

  Dice.prototype.destroy = function () {
    this.alive = false;
    clearTimeout(this.timer);
    this.handlers.forEach(function (h) { h[2].removeEventListener(h[0], h[1]); });
    this.handlers = [];
    // The tray's dice hold a WebGL context until they are cleared.
    if (this.stage && C.dice && typeof C.dice.stop === 'function') C.dice.stop(this.stage);
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
      self.frame();
      self.render();
      self.trayRest();
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
  // The colour a set's tile shows: the chosen one on the chosen set, else the one picking that set
  // would give (a pick of a set clears the colour).
  Dice.prototype.tileColour = function (set) {
    if (set.key === this.curSet().key) return this.curColour(set);
    var want = this.defaults().colour;
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

  // ---------- Pictures ----------

  function picKey(set, colour, value, px) { return [set, colour, value, px].join('|'); }

  // The picture for a die if it is drawn, else ''. Asks for it once; when it arrives every <img>
  // waiting on it is filled. Without WebGL the flat d20 stands in at once, and it stands in too
  // when a draw fails.
  Dice.prototype.pic = function (set, colour, value, px) {
    var key = picKey(set.key, colour.key, value, px), self = this, d = C.dice;
    if (this.pics[key]) return this.pics[key];
    if (!hasGL() || !d || typeof d.portrait !== 'function') return (this.pics[key] = flatD20(colour, value));
    if (!this.asked[key]) {
      this.asked[key] = true;
      this.wait[key] = { set: set, colour: colour, value: value, px: px, n: Object.keys(this.asked).length };
      // After the redraw that asked has put its <img> on the page, so the order can see it.
      Promise.resolve().then(function () { self.drawNext(); });
    }
    return '';
  };

  // Which waiting picture to draw next: the campaign's own die (the tray and its tile), then
  // tiles on screen, then the rest in catalogue order. Asked again before every draw, so a
  // scroll or a new pick changes what comes next.
  Dice.prototype.rank = function (key) {
    var set = this.curSet(), mine = picKey(set.key, this.curColour(set).key, 20, TILE_PX);
    if (key === mine || (this.still && key === this.still.getAttribute('data-dcz-pic'))) return 0;
    var imgs = this.el.querySelectorAll('img[data-dcz-pic]'), h = window.innerHeight || 0, w = window.innerWidth || 0;
    for (var i = 0; i < imgs.length; i++) {
      if (imgs[i].getAttribute('data-dcz-pic') !== key) continue;
      var r = imgs[i].getBoundingClientRect();
      if (r.bottom > 0 && r.top < h && r.right > 0 && r.left < w) return 1;
    }
    return 2;
  };

  // Draws the waiting pictures one at a time: the engine has one shared table for them, and
  // choosing each next one only when the last is done is what lets the order follow the screen.
  Dice.prototype.drawNext = function () {
    var self = this, keys = Object.keys(this.wait);
    if (this.drawing || !keys.length || !this.alive) return;
    var key = keys.map(function (k) { return [self.rank(k), self.wait[k].n, k]; })
      .sort(function (a, b) { return a[0] - b[0] || a[1] - b[1]; })[0][2];
    var job = this.wait[key];
    delete this.wait[key];
    this.drawing = true;
    var done = function (url) {
      self.pics[key] = url;
      self.drawing = false;
      if (!self.alive) return;
      self.el.querySelectorAll('img[data-dcz-pic]').forEach(function (img) {
        if (img.getAttribute('data-dcz-pic') === key) self.fill(img, url);
      });
      self.drawNext();
    };
    C.dice.portrait({ set: job.set.key, colour: job.colour.key, value: job.value, px: job.px })
      .then(done, function () { done(flatD20(job.colour, job.value)); });
  };

  // A picture fades in once decoded, so a tile never flashes half drawn.
  Dice.prototype.fill = function (img, url) {
    if (img.getAttribute('src') === url) return;
    img.classList.remove('is-in');
    img.onload = function () { img.classList.add('is-in'); };
    img.src = url;
  };

  function img(key, url, cls) {
    return '<img class="' + cls + (url ? ' is-in' : '') + '" alt="" data-dcz-pic="' + esc(key) + '"' + (url ? ' src="' + esc(url) + '"' : '') + ' draggable="false">';
  }

  // ---------- Drawing ----------

  function swatch(c) {
    var g = c.swatch || ['#888', '#444'];
    return '<span class="dcz-sw" style="background:linear-gradient(135deg,' + esc(g[0]) + ',' + esc(g[g.length - 1]) + ')" aria-hidden="true"></span>';
  }

  Dice.prototype.tiles = function () {
    var cur = this.curSet().key, self = this;
    return this.cat.sets.map(function (x) {
      var on = x.key === cur, col = self.tileColour(x), key = picKey(x.key, col.key, 20, TILE_PX);
      return '<label class="dcz-tile' + (on ? ' is-on' : '') + '"><input class="dcz-tile-in" type="radio" name="dcz-set" value="' + esc(x.key) + '" data-dcz-set="' + esc(x.key) + '"' + (on ? ' checked' : '') + '>' +
        '<span class="dcz-pic">' + img(key, self.pic(x, col, 20, TILE_PX), 'dcz-pic-img') + '</span>' +
        '<span class="dcz-tile-nm">' + esc(x.name) + '</span></label>';
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

  // The parts drawn once: the lede and status, the two columns, and the tray.
  Dice.prototype.frame = function () {
    this.el.innerHTML =
      '<div class="dcz-head"><p class="dcz-lede">The dice every roll at this table is thrown with. Each pick saves as you make it.</p>' +
      '<p class="dcz-status" role="status" aria-live="polite" data-dcz-status></p></div>' +
      '<div class="dcz-layout">' +
      '<section class="dcz-sec dcz-die" aria-labelledby="dcz-h-die" data-dcz-die></section>' +
      '<aside class="dcz-tray" aria-label="The campaign’s die, thrown">' +
      '<div class="dcz-stage" data-dcz-stage><img class="dcz-still" alt="" data-dcz-still draggable="false"></div>' +
      '<p class="dcz-tray-lab" data-dcz-tray-lab></p>' +
      '<div class="dcz-acts">' +
      '<button type="button" class="dcz-word" data-dcz-roll="again">Roll again</button>' +
      '<button type="button" class="dcz-word" data-dcz-roll="succ">Crit</button>' +
      '<button type="button" class="dcz-word" data-dcz-roll="fail">Fumble</button></div></aside>' +
      '<div class="dcz-rest" data-dcz-rest></div></div>';
    this.stage = this.el.querySelector('[data-dcz-stage]');
    this.still = this.el.querySelector('[data-dcz-still]');
    // The tray stays in view below the Customize page's own sticky tab row, whatever its height.
    var top = document.querySelector('.czs-top');
    if (top) this.el.style.setProperty('--dcz-stick', Math.round(top.getBoundingClientRect().height) + 'px');
  };

  Dice.prototype.renderStatus = function () {
    var st = this.el.querySelector('[data-dcz-status]');
    if (!st) return;
    st.innerHTML = this.error ? '<span class="dcz-bad" role="alert">' + esc(this.error) + '</span>' : this.saved ? '<span class="dcz-ok"><i class="fa-solid fa-check" aria-hidden="true"></i> Saved</span>' : '';
  };

  Dice.prototype.render = function () {
    if (!this.s || !this.cat || !this.stage) return;
    var keep = document.activeElement && this.el.contains(document.activeElement) ? this.focusKey(document.activeElement) : null;
    var s = this.s, set = this.curSet(), colour = this.curColour(set);
    this.renderStatus();

    this.el.querySelector('[data-dcz-die]').innerHTML =
      '<h2 id="dcz-h-die" class="dcz-h">The campaign’s die</h2>' +
      '<div class="dcz-tiles" role="radiogroup" aria-labelledby="dcz-h-die">' + this.tiles() + '</div>' +
      (set.note ? '<p class="dcz-hint"><span class="dcz-strong">' + esc(set.name) + ':</span> ' + esc(set.note) + '</p>' : '') +
      '<div class="dcz-colours"><p class="dcz-cap" id="dcz-colour-lab">Colour</p>' +
      '<div class="dcz-cols" role="group" aria-labelledby="dcz-colour-lab">' + this.colourButtons() + '</div>' +
      '<p class="dcz-hint"><span class="dcz-strong">' + esc(colour.name) + (colour.note ? ':</span> ' + esc(colour.note) : '</span>') + '</p></div>';

    this.el.querySelector('[data-dcz-rest]').innerHTML =
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
      '<p class="dcz-hint">What a player who hasn’t picked starts on.</p>' + seg('data-dcz-look', LOOKS, s.look || 'quick', 'The table’s dice') + '</section>';

    this.el.querySelector('[data-dcz-tray-lab]').textContent = set.name + ' · ' + colour.name;
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

  // save puts a change at once and keeps it only if the server took it. A failed save that changed
  // the die sets the tray back to the die that is saved.
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
      self.timer = setTimeout(function () { self.saved = false; if (self.alive) self.renderStatus(); }, SAVED_STAY);
    }).catch(function (e) {
      if (!self.alive) return;
      var die = change.diceSet !== undefined || change.diceColour !== undefined;
      self.s = was;
      self.error = 'That wasn’t saved. ' + (e && e.message && !/^status/.test(e.message) ? e.message : 'Try again.');
      self.render();
      if (die) self.trayRest();
    });
  };

  // The campaign's own set is always on offer, so choosing one that isn't adds it.
  Dice.prototype.pickSet = function (key) {
    var change = { diceSet: key, diceColour: '' };
    var o = this.s.diceOffered;
    if (o && o.length && o.indexOf(key) < 0) change.diceOffered = o.concat([key]);
    this.save(change);
    this.throwTray('again');
  };

  Dice.prototype.pickColour = function (key) {
    this.save({ diceColour: key });
    this.throwTray('again');
  };

  Dice.prototype.toggleOffer = function (key, on) {
    var cur = this.offeredKeys(), own = this.curSet().key;
    var next = this.cat.sets.map(function (x) { return x.key; }).filter(function (k) { return k === own || (k === key ? on : cur.indexOf(k) >= 0); });
    // Every set on offer is saved as "all", so a later new set is on offer too.
    this.save({ diceOffered: next.length === this.cat.sets.length ? [] : next });
  };

  // ---------- The tray ----------

  // The chosen die at rest, value up, as a still picture; any thrown die is cleared.
  Dice.prototype.trayRest = function (value) {
    if (!this.stage) return;
    if (C.dice && typeof C.dice.stop === 'function' && this.stage.querySelector('canvas')) C.dice.stop(this.stage);
    var set = this.curSet(), colour = this.curColour(set), v = value || 20, key = picKey(set.key, colour.key, v, TRAY_PX);
    this.still.setAttribute('data-dcz-pic', key);
    this.still.hidden = false;
    var url = this.pic(set, colour, v, TRAY_PX);
    if (url) this.fill(this.still, url); else { this.still.removeAttribute('src'); this.still.classList.remove('is-in'); }
  };

  // Throws the chosen die inside the tray: a random number, or a crit or a fumble shown with the
  // campaign's crit animations. Off, or without WebGL, the die is shown at rest on its number. The
  // number is read out by Chronicle.dice either way.
  Dice.prototype.throwTray = function (what) {
    var self = this, d = C.dice;
    if (!this.stage || !d || typeof d.roll !== 'function') return;
    var die = what === 'succ' ? { sides: 20, value: 20, crit: 'succ' } : what === 'fail' ? { sides: 20, value: 1, crit: 'fail' } : { sides: 20 };
    var drawn = motion() !== 'off' && hasGL(), n = ++this.throwN;
    if (drawn) this.still.hidden = true;
    d.roll([{ label: 'Test throw', dice: [die] }], {
      host: drawn ? this.stage : null, keep: true, skippable: false, table: this.look(),
      describe: function (g, v) { return 'Test throw: ' + (v || []).join(', ') + '.'; },
      // A throw that didn't draw (its files were late or failed) leaves the die at rest instead.
      onEnd: function () { if (self.alive && n === self.throwN && drawn && !self.stage.querySelector('canvas')) self.trayRest(self.lastValue); }
    }).then(function (out) {
      self.lastValue = out && out[0] && out[0].values[0];
      if (self.alive && n === self.throwN && !drawn) self.trayRest(self.lastValue);
    }, function () {});
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
      if ((a = b.getAttribute('data-dcz-colour')) !== null) self.pickColour(a);
      else if ((a = b.getAttribute('data-dcz-quality')) !== null) self.save({ quality: a });
      else if ((a = b.getAttribute('data-dcz-look')) !== null) self.save({ look: a });
      else if ((a = b.getAttribute('data-dcz-roll')) !== null) self.throwTray(a);
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
