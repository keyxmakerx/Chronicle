/**
 * table.js — the header's Table tab: the roll log, the roller, your dice and
 * the roll notice (the header island). Built against the signed Table tab
 * mockup (ui-standard.md, "The table is one tab away").
 *
 * Every number comes from the server: a roll made here is decided by
 * POST /campaigns/:id/rolls, and every roll anyone makes (here, in Foundry,
 * or by crafting) arrives as a roll.made message on the campaign's socket,
 * already cut to what this person may see. This script only shows them.
 *
 * Per person, remembered in this browser: which section each leaf shows and
 * how dice are drawn (quick 2D, rune 3D or numbers only; a new player starts
 * on the table's choice). Kept with their account (GET/PUT my-dice), when the
 * Director allows it: their own rune set, colour and crit animations. Every
 * roll carries the dice the server settled for its roller (rollerDice), and
 * every screen throws it in them (roll_dice.js).
 */
(function () {
  'use strict';

  // A duration token in ms, read when the move starts so the owner's Motion
  // speed retimes it.
  function tokMs(name, fallback) {
    return window.Chronicle && window.Chronicle.motionMs ? window.Chronicle.motionMs(name, fallback) : fallback;
  }

  var W = window;
  var C = W.Chronicle;

  // The sections a leaf can show. Rules, Conditions, Party and Notes join
  // this list as they are built (TODO(#1475)).
  var BASE_SECTIONS = [['rolls', 'Rolls'], ['tonight', 'Tonight'], ['dice', 'My dice']];
  // With the Card decks add-on on, the Hand leaf joins, second in the default order.
  var HAND_SECTIONS = [['rolls', 'Rolls'], ['hand', 'Hand'], ['tonight', 'Tonight'], ['dice', 'My dice']];
  var SECTION_NAME = { rolls: 'Rolls', hand: 'Hand', tonight: 'Tonight', dice: 'My dice' };
  var LOOKS = [['quick', 'Quick 2D'], ['rune', 'Rune 3D'], ['none', 'Numbers only']];
  var QUICK = [4, 6, 8, 10, 12, 20];
  // A game with a power roll puts its own dice first; the rest wait behind "Other dice".
  var POWER_QUICK = [10, 6, 3];
  // Rune dice come in these sizes; any other die plays as quick 2D.
  var RUNE_SIDES = { 4: 1, 6: 1, 8: 1, 10: 1, 12: 1, 20: 1 };
  // How many rolls the log holds in the page before older ones are fetched.
  var PAGE = 50;
  // How long the island stays out, in ms.
  var ISLAND_STAY = 3400;
  // On a phone the drawer is a sheet resting on the bottom edge instead of
  // hanging from the header. Same drawer, same leaves, same data.
  var SHEET_QUERY = '(max-width: 639px), (pointer: coarse) and (max-width: 899px)';
  var HEIGHTS = ['rest', 'lift', 'full'];
  // The resting line is this tall, plus the phone's safe area.
  var REST_PX = 38;
  // The lifted sheet covers this much of the screen. It shows the dice only;
  // the character's key numbers wait on a system's headline fields (TODO(#2193)).
  var LIFT_SHARE = 0.6;

  // Dice figures go into markup unescaped, so anything that isn't a whole
  // number becomes 0 rather than trusting the payload's shape.
  function num(v) { var n = Number(v); return isFinite(n) ? Math.trunc(n) : 0; }
  function esc(s) { return Chronicle.escapeHtml(s); }
  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }
  function store(key, val) {
    try {
      if (val === undefined) return W.localStorage.getItem('chronicle.table.' + key);
      W.localStorage.setItem('chronicle.table.' + key, val);
    } catch (e) { /* private window: nothing is remembered */ }
    return null;
  }
  function motion() { return C.tableDice ? C.tableDice.motion() : 'full'; }
  function initials(name) {
    var parts = String(name || '?').trim().split(/\s+/);
    return ((parts[0] || '?').charAt(0) + (parts.length > 1 ? parts[parts.length - 1].charAt(0) : '')).toUpperCase();
  }
  function timeOf(iso) {
    var d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    return d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  }
  // How long a cut scene can be watched again: a time today, else the day too.
  function untilText(iso) {
    var d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    var t = d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
    return d.toDateString() === new Date().toDateString() ? t : d.toLocaleDateString([], { weekday: 'short' }) + ' ' + t;
  }
  // A single-key shortcut never fires while typing.
  function typing(t) {
    return !!(t && (t.isContentEditable || (t.closest && t.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"])'))));
  }

  var X = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg>';
  var CHEV = '<svg class="tt-more" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>';

  // ---------- One table per page ----------

  function Table(host) {
    var me = this;
    this.host = host;
    this.campaignId = host.getAttribute('data-campaign-id');
    this.userId = host.getAttribute('data-user-id');
    this.director = host.getAttribute('data-director') === 'true';
    this.gm = host.getAttribute('data-gm-term') || 'Director';
    this.decks = host.getAttribute('data-decks') === 'true';
    this.sections = this.decks ? HAND_SECTIONS : BASE_SECTIONS;
    this.tab = $('[data-tt-tab]', host);
    this.count = $('[data-tt-count]', host);
    this.header = host.closest('header') || host.parentNode;
    this.base = '/campaigns/' + encodeURIComponent(this.campaignId) + '/rolls';
    var key = this.campaignId + '.' + this.userId;
    this.key = key;
    this.leaves = this.readLeaves(store('leaves.' + key));
    this.phoneLeaf = 0;
    this.look = store('look.' + key) || '';
    // Crit animations are on unless this person turned them off in this browser.
    this.crits = store('crits.' + key) !== 'off';
    this.tableLook = 'quick';
    // The campaign's dice settings, once read; rolls use the defaults until then.
    this.camp = null;
    // This person's own dice, kept with their account; read with the settings.
    this.personal = readPicks('');
    this.pickErr = '';
    this.cat = null;
    // The game system's power roll and this person's hero (power_roll.js does the math).
    this.power = null;
    this.step = 0;
    this.otherDice = false;
    this.lastStat = '';
    this.catAsked = false;
    this.rolls = [];
    this.byId = {};
    // What the director has shown tonight, oldest first.
    this.shown = [];
    this.shownById = {};
    this.shownLoaded = false;
    // The Cut scenes shown to this person that can still be watched again.
    this.cuts = host.getAttribute('data-cutscenes') === 'true';
    this.cutRows = [];
    this.cutErr = {};
    this.cutTimer = 0;
    this.cutAsked = false;
    this.more = false;
    this.loaded = false;
    this.open = false;
    this.openRow = '';
    this.unseen = 0;
    // A card's moment (Card decks, Directors only): what is waiting, from the
    // server, and the notice that shows one of them. The Table's count is the
    // rolls not yet seen plus these.
    this.momentsOn = this.decks && this.director;
    this.moments = { count: 0, rows: [] };
    this.momKnown = null;
    this.momOpen = '';
    this.momHTML = '';
    this.momSeq = 0;
    this.momEl = null;
    this.isl = null;
    this.queue = [];
    this.ws = null;
    this.wsDelay = 0;
    this.alive = true;
    this.handlers = [];
    this.sheet = false;
    this.h = 'rest';
    this.mql = W.matchMedia ? W.matchMedia(SHEET_QUERY) : { matches: false };
    this.wire();
    this.connect();
    if (this.momentsOn) {
      this.on(W, 'chronicle:decks-moment', function () { me.askMoments(); });
      this.loadMoments();
    }
    this.loadSettings();
    this.loadPower();
    this.syncSheet();
    if (this.mql.addEventListener) this.on(this.mql, 'change', function () { me.syncSheet(); });
    this.openFromLink();
    this.watchDraw();
    if (this.cuts) this.wireCuts();
  }

  // A bell notice opens the Table on a leaf with ?table=<leaf>.
  Table.prototype.openFromLink = function () {
    var want = '';
    try { want = new URLSearchParams(W.location.search).get('table') || ''; } catch (e) { return; }
    if (this.leaves.indexOf(want) < 0 || want === 'rolls') return;
    var me = this;
    setTimeout(function () { if (me.alive) me.openTable(want); }, 0);
  };

  // A remembered layout keeps only sections that exist, each once.
  // On the phone's sheet every section is a tab, so none is out of reach; on
  // a desktop the drawer shows at most three side by side.
  Table.prototype.readLeaves = function (raw) {
    var out = [], all = this.sections.map(function (s) { return s[0]; });
    (raw || '').split(',').forEach(function (s) { if (all.indexOf(s) >= 0 && out.indexOf(s) < 0) out.push(s); });
    all.forEach(function (s) { if (out.indexOf(s) < 0) out.push(s); });
    return out.slice(0, this.sheet ? all.length : Math.min(3, all.length));
  };

  Table.prototype.on = function (target, type, fn, opt) {
    target.addEventListener(type, fn, opt);
    this.handlers.push([target, type, fn, opt]);
  };

  Table.prototype.destroy = function () {
    this.alive = false;
    this.handlers.forEach(function (h) { h[0].removeEventListener(h[1], h[2], h[3]); });
    this.handlers = [];
    clearTimeout(this.wsRetry);
    if (this.ws) { try { this.ws.close(); } catch (e) { /* already closed */ } }
    var d = $('#tt-drawer'); if (d) d.remove();
    document.documentElement.removeAttribute('data-tt-sheet');
    var c = $('.tt-isl-clip', this.header); if (c) c.remove();
    var t = $('#tt-tray'); if (t) t.remove();
    var det = $('#tt-isl-detail'); if (det) det.remove();
    var mo = $('#tt-moment'); if (mo) mo.remove();
    clearTimeout(this.momTimer);
    clearTimeout(this.cutTimer);
    if (this.drawWatch) this.drawWatch.disconnect();
  };

  // The dice this person sees: their own pick, else the table's.
  Table.prototype.myLook = function () { return this.look || this.tableLook || 'quick'; };

  Table.prototype.loadSettings = function () {
    var self = this;
    C.apiFetch(this.base + '/settings').then(function (r) { return r.ok ? r.json() : null; }).then(function (s) {
      if (s && s.look) self.tableLook = s.look;
      if (s) self.camp = s;
      self.applyDice();
      if (self.open) self.renderSection('dice');
      if (s) self.loadMyDice();
    }).catch(function () { /* the table's choice stays quick 2D */ });
  };

  // This person's own dice, kept with their account. A pick an older Table
  // kept in this browser moves to the account the first time, then the
  // browser's copy is dropped.
  Table.prototype.loadMyDice = function () {
    var self = this, local = 'dice.' + this.key;
    C.apiFetch(this.base + '/my-dice').then(function (r) { return r.ok ? r.json() : null; }).then(function (m) {
      if (!m || !self.alive) return;
      self.personal = fromServer(m.pick);
      var old = readPicks(store(local));
      var hasOld = !!(old.set || old.colour || old.succ || old.fail);
      if (hasOld && !self.hasPicks() && m.mayPick) {
        self.personal = old;
        self.savePicks(function () { store(local, ''); });
      } else if (hasOld) {
        store(local, '');
      }
      self.applyDice();
      if (self.open) self.renderSection('dice');
    }).catch(function () { /* the campaign's dice stand in */ });
  };

  // The campaign's power roll, when its game system declares one. Without it the
  // Table is unchanged.
  Table.prototype.loadPower = function () {
    var self = this;
    C.apiFetch(this.base + '/power').then(function (r) { return r.ok ? r.json() : null; }).then(function (p) {
      if (!p || !p.declared || !C.powerRoll || !(p.characteristics || []).length) return;
      self.power = { dice: p.dice || '2d10', edge: num(p.edge), bane: num(p.bane), tiers: [num((p.tiers || [])[0]), num((p.tiers || [])[1])], characteristics: p.characteristics, hero: p.hero || '', heroId: p.heroId || '' };
      if (self.loaded) self.refreshLog(true);
      var d = self.drawer();
      if (self.sheet && d) {
        var keep = self.keepForm();
        d.innerHTML = self.drawerHTML();
        self.restoreForm(keep);
      } else if (self.open) self.renderSection('rolls');
    }).catch(function () { /* no power roll line */ });
  };

  // How a roll reads as a power roll: { tier, crit, total } or null.
  Table.prototype.pread = function (r) {
    return this.power ? C.powerRoll.read(r, this.power) : null;
  };

  // The big figure for a roll: the tier first for a power roll, else the total.
  Table.prototype.figure = function (r) {
    if (r.hidden) return '?';
    var p = this.pread(r);
    return p ? C.powerRoll.headline(p) : String(r.total);
  };

  // The bonus typed for a hero with no numbers on the sheet, kept in this browser.
  Table.prototype.myBonus = function () {
    var n = parseInt(store('pbonus.' + this.key), 10);
    return isFinite(n) ? Math.max(-9, Math.min(20, n)) : 0;
  };

  // The server's pick ({ set, colour, critSucc, critFail }) in the picker's own names.
  function fromServer(p) {
    p = p || {};
    return readPicks(JSON.stringify({ set: p.set, colour: p.colour, succ: p.critSucc, fail: p.critFail }));
  }

  // Picks as JSON: only known keys, as strings shaped like a dice key.
  function readPicks(raw) {
    var out = { set: '', colour: '', succ: '', fail: '' };
    try {
      var o = JSON.parse(raw || '{}');
      Object.keys(out).forEach(function (k) { if (typeof o[k] === 'string' && /^[a-z0-9-]{0,24}$/.test(o[k])) out[k] = o[k]; });
    } catch (e) { /* nothing remembered */ }
    return out;
  }
  Table.prototype.hasPicks = function () {
    var p = this.personal;
    return !!(p.set || p.colour || p.succ || p.fail);
  };
  // savePicks stores this person's pick with their account. The server checks it against
  // what the campaign offers; a refused pick goes back to what was saved, with the reason.
  // after runs once the server has answered, either way.
  Table.prototype.savePicks = function (after, was) {
    var self = this, p = this.personal;
    this.pickErr = '';
    this.applyDice();
    C.apiFetch(this.base + '/my-dice', { method: 'PUT', body: { set: p.set, colour: p.colour, critSucc: p.succ, critFail: p.fail } }).then(function (res) {
      if (!res.ok) return res.json().catch(function () { return {}; }).then(function (b) { throw new Error((b && b.message) || ''); });
      return res.json();
    }).then(function (m) {
      if (!self.alive) return;
      self.personal = fromServer(m && m.pick);
    }, function (e) {
      if (!self.alive) return;
      self.personal = was || readPicks('');
      self.pickErr = 'That wasn’t saved. ' + (e && e.message ? e.message : 'Try again.');
    }).then(function () {
      if (!self.alive) return;
      if (after) after();
      self.applyDice();
      if (self.open) self.renderSection('dice');
    });
  };

  // The campaign's own 3D dice. An empty name is the default, which dice.js fills in.
  Table.prototype.campDice = function () {
    var c = this.camp || {};
    return { set: c.diceSet || '', colour: c.diceColour || '', succ: c.critSucc || '', fail: c.critFail || '', quality: c.quality || '' };
  };

  // Whether the Director has put a set on the shelf for players; none listed means all.
  Table.prototype.offered = function (set) {
    var o = this.camp && this.camp.diceOffered;
    return !o || !o.length || o.indexOf(set) >= 0;
  };

  // The dice for this person's own rolls: their picks where the Director lets players pick
  // (a set that is no longer offered falls back to the campaign's, colour with it), else the campaign's.
  Table.prototype.myDice = function () {
    var out = this.campDice(), p = this.personal;
    if (!this.camp || !this.camp.playersPick) return out;
    if (p.set && this.offered(p.set)) { out.set = p.set; out.colour = p.colour; }
    if (p.succ) out.succ = p.succ;
    if (p.fail) out.fail = p.fail;
    return out;
  };

  // The 3D dice one roll is thrown in. Without roll_dice.js loaded, the old rule stands.
  Table.prototype.diceFor = function (r) {
    if (C.rollDice) return C.rollDice.forRoll(r, this.campDice(), this.myDice());
    return r.mine ? this.myDice() : this.campDice();
  };

  // The page-wide look for this person's rolls from other places (shop, crafting, hero builder).
  // Only an explicit false turns crit effects off (see dice.js withTable), so the field is left out while they are on.
  Table.prototype.applyDice = function () {
    var t = this.myDice();
    if (!this.crits) t.crits = false;
    C.diceTable = t;
  };

  // ---------- Live rolls ----------

  Table.prototype.connect = function () {
    if (!this.alive || typeof W.WebSocket !== 'function') return;
    var self = this;
    var url = (W.location.protocol === 'https:' ? 'wss:' : 'ws:') + '//' + W.location.host + '/ws?campaign=' + encodeURIComponent(this.campaignId);
    var sock;
    try { sock = new W.WebSocket(url); } catch (e) { return; }
    this.ws = sock;
    sock.addEventListener('open', function () {
      self.wsDelay = 0;
      // Rolls made while the socket was down are in the log, not replayed.
      if (self.loaded) self.reload();
      // Nor are moments that came while it was down.
      if (self.momentsOn && self.momKnown) self.askMoments();
    });
    sock.addEventListener('message', function (ev) {
      var msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      if (msg && msg.type === 'roll.made' && msg.payload) self.receive(msg.payload, true);
      if (msg && msg.type === 'table.shown' && msg.payload) self.receiveShown(msg.payload, true);
      // Carries nothing: the hand and the hero strips fetch their own view.
      if (msg && msg.type === 'decks.changed') W.dispatchEvent(new CustomEvent('chronicle:decks-changed'));
      // Directors only; carries nothing, the moments panel fetches its own list.
      if (msg && msg.type === 'decks.moment') W.dispatchEvent(new CustomEvent('chronicle:decks-moment'));
    });
    sock.addEventListener('close', function () {
      if (!self.alive) return;
      self.wsDelay = Math.min(30000, (self.wsDelay || 1000) * 2);
      self.wsRetry = setTimeout(function () { self.connect(); }, self.wsDelay);
    });
  };

  // receive takes a roll from the socket or this person's own POST. The same
  // roll can arrive twice (the reply and the socket); it plays once, and a
  // full copy replaces a blind stub, never the other way.
  Table.prototype.receive = function (r, live) {
    if (!r || !r.id) return;
    r.mine = r.mine || (!!r.rollerId && r.rollerId === this.userId);
    var had = this.byId[r.id];
    if (had) {
      if (had.hidden && !r.hidden) { this.replace(had, r); this.refreshLog(); }
      return;
    }
    this.byId[r.id] = r;
    this.rolls.push(r);
    if (live) this.play(r);
  };

  Table.prototype.replace = function (old, r) {
    var i = this.rolls.indexOf(old);
    if (i >= 0) this.rolls[i] = r;
    this.byId[r.id] = r;
  };

  Table.prototype.load = function () {
    var self = this;
    return C.apiFetch(this.base + '?limit=' + PAGE).then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    }).then(function (page) {
      self.loaded = true;
      self.more = !!page.more;
      var known = self.byId;
      self.rolls = [];
      self.byId = {};
      (page.rolls || []).forEach(function (r) { self.receive(r, false); });
      // A roll that arrived live after the page was read stays.
      Object.keys(known).forEach(function (id) { if (!self.byId[id]) self.receive(known[id], false); });
      self.rolls.sort(function (a, b) { return a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : 0; });
      self.loadError = '';
      self.refreshLog();
      self.refreshGrab(false);
    }).catch(function () {
      self.loadError = 'The roll log could not be read. Close the Table and open it again to retry.';
      self.refreshLog();
    });
  };

  Table.prototype.reload = function () {
    if (this.open || this.sheet) { this.load(); this.loadShown(); this.loadCuts(); } else { this.loaded = false; this.shownLoaded = false; this.cutAsked = false; }
  };

  // ---------- Tonight: what the director shows ----------

  // receiveShown takes an item the director showed. Each one is listed once;
  // a live one also slides out the island.
  Table.prototype.receiveShown = function (it, live) {
    if (!it || !it.id || this.shownById[it.id]) return;
    it.shown = true;
    this.shownById[it.id] = it;
    this.shown.push(it);
    if (!live) return;
    if (this.open || this.sheet) this.renderSection('tonight');
    if (this.leafShowing('tonight')) {
      var li = $('#tt-tonight li[data-id="' + (W.CSS && CSS.escape ? CSS.escape(it.id) : it.id) + '"]');
      if (li) li.classList.add('tt-fresh');
    } else {
      this.notice(it);
    }
  };

  Table.prototype.loadShown = function () {
    var self = this;
    return C.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/table/shown').then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    }).then(function (out) {
      self.shownLoaded = true;
      var known = self.shownById;
      self.shown = [];
      self.shownById = {};
      ((out && out.items) || []).forEach(function (it) { self.receiveShown(it, false); });
      Object.keys(known).forEach(function (id) { if (!self.shownById[id]) self.receiveShown(known[id], false); });
      self.shown.sort(function (a, b) { return a.shownAt < b.shownAt ? -1 : a.shownAt > b.shownAt ? 1 : 0; });
      self.shownError = '';
      if (self.open || self.sheet) self.renderSection('tonight');
    }).catch(function () {
      self.shownError = 'What was shown tonight could not be read. Close the Table and open it again to retry.';
      if (self.open || self.sheet) self.renderSection('tonight');
    });
  };

  // ---------- Tonight: cut scenes to watch again ----------

  // The stage widget plays scenes and tells the page what changed; the list
  // follows it rather than opening another socket.
  Table.prototype.wireCuts = function () {
    var self = this;
    this.on(W, 'chronicle:cutscene-shown', function () { self.cutAsked = false; if (self.open || self.sheet) self.loadCuts(); });
    this.on(W, 'chronicle:cutscene-list', function (ev) { if (ev.detail && Array.isArray(ev.detail.rows)) self.takeCuts(ev.detail.rows); });
    this.on(W, 'chronicle:cutscene-watched', function (ev) {
      var id = ev.detail && ev.detail.showingId;
      self.cutRows.forEach(function (r) { if (String(r.showingId) === String(id)) r.watched = true; });
      self.renderCuts();
    });
  };

  Table.prototype.loadCuts = function () {
    var self = this;
    if (!this.cuts || this.cutAsked) return;
    this.cutAsked = true;
    C.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/cutscenes/mine').then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    }).then(function (body) {
      self.takeCuts(Array.isArray(body) ? body : (body && body.data) || []);
    }).catch(function () { self.cutAsked = false; });
  };

  Table.prototype.takeCuts = function (rows) {
    this.cutRows = rows.filter(function (r) { return r && r.showingId; });
    this.renderCuts();
  };

  // renderCuts redraws Tonight and wakes again when the next row's time to
  // watch again runs out, so a row is gone once it can no longer be played.
  Table.prototype.renderCuts = function () {
    var self = this, now = Date.now(), next = Infinity;
    clearTimeout(this.cutTimer);
    this.cutRows.forEach(function (r) {
      var until = Date.parse(r.rewatchUntil);
      if (until > now) next = Math.min(next, until);
    });
    if (next !== Infinity) this.cutTimer = setTimeout(function () { self.renderCuts(); }, Math.min(next - now + 500, 2147483000));
    if (this.open || this.sheet) this.renderSection('tonight');
  };

  Table.prototype.liveCuts = function () {
    var now = Date.now();
    return this.cutRows.filter(function (r) { return Date.parse(r.rewatchUntil) > now; });
  };

  Table.prototype.cutHTML = function (r) {
    var id = String(r.showingId), err = this.cutErr[id];
    return '<li class="tt-shown tt-cut" data-cut="' + esc(id) + '"><div class="tt-shead"><b>' + esc(r.name || 'A cut scene') + '</b>' +
      (r.watched ? '' : '<span class="tt-chip">Missed it</span>') +
      '<span class="tt-time">until ' + esc(untilText(r.rewatchUntil)) + '</span></div>' +
      '<button type="button" class="tt-quiet tt-open" data-tt-cut="' + esc(id) + '">' + (r.watched ? 'Watch again' : 'Watch') + '</button>' +
      (err ? '<p class="tt-note" role="alert">' + esc(err) + '</p>' : '') + '</li>';
  };

  Table.prototype.watchCut = function (id, btn) {
    var self = this;
    delete this.cutErr[id];
    if (!C.cutscenes || typeof C.cutscenes.watch !== 'function') {
      this.cutErr[id] = 'Cut scenes are still loading on this page. Try again in a moment.';
      this.renderSection('tonight');
      return;
    }
    btn.disabled = true;
    btn.setAttribute('aria-busy', 'true');
    C.cutscenes.watch(id).then(function () {
      if (btn.isConnected) { btn.disabled = false; btn.removeAttribute('aria-busy'); }
    }, function (e) {
      self.cutErr[id] = e && e.plain ? e.message : 'The scene could not be played. Try again in a moment.';
      self.renderSection('tonight');
    });
  };

  // leafShowing says whether a section is on screen now: open, in a leaf,
  // and on a phone the leaf that is picked.
  Table.prototype.leafShowing = function (sec) {
    var i = this.leaves.indexOf(sec);
    if (!this.open || i < 0) return false;
    if (this.sheet) return this.h === 'full' && this.phoneLeaf === i;
    var b = document.getElementById('tt-lb-' + i);
    return !!b && b.offsetParent !== null;
  };

  Table.prototype.pageURL = function (it) {
    return '/campaigns/' + encodeURIComponent(this.campaignId) + '/entities/' + encodeURIComponent(it.entityId);
  };

  Table.prototype.shownWhat = function (it) {
    return it.label || (it.kind === 'note' ? 'A note' : 'A page');
  };

  // shownBody is the same under a Tonight row and under the island: the
  // note's words, or a link to the page.
  Table.prototype.shownBody = function (it) {
    var out = '';
    if (it.text) out += '<p class="tt-note">' + esc(it.text) + '</p>';
    if (it.entityId) out += '<a class="tt-quiet tt-open" href="' + esc(this.pageURL(it)) + '">Open the page</a>';
    return out;
  };

  Table.prototype.tonightHTML = function () {
    var self = this, rows, cuts = this.liveCuts();
    // Pages, notes and cut scenes in the order they were shown.
    var all = this.shown.map(function (it) { return { at: it.shownAt, it: it }; })
      .concat(cuts.map(function (r) { return { at: r.shownAt, cut: r }; }))
      .sort(function (a, b) { return a.at < b.at ? -1 : a.at > b.at ? 1 : 0; });
    if (this.shownError && !cuts.length) rows = '<li class="tt-empty">' + esc(this.shownError) + '</li>';
    else if (!this.shownLoaded && !cuts.length) rows = '<li class="tt-empty">Reading…</li>';
    else if (!all.length) rows = '<li class="tt-empty">Nothing shown yet. When the ' + esc(this.gm) + ' shows you a page, a note or a cut scene, it lands here.</li>';
    else rows = all.map(function (x) {
      if (x.cut) return self.cutHTML(x.cut);
      var it = x.it;
      return '<li class="tt-shown" data-id="' + esc(it.id) + '"><div class="tt-shead"><b>' + esc(self.shownWhat(it)) + '</b>' +
        (it.to ? '<span class="tt-chip">Only you</span>' : '') + '<span class="tt-time">' + esc(timeOf(it.shownAt)) + '</span></div>' +
        self.shownBody(it) + '</li>';
    }).join('');
    return '<ul class="tt-log tt-tonight" id="tt-tonight" aria-label="Shown tonight">' + rows + '</ul>';
  };

  // ---------- Hand: the Card decks leaf ----------

  // The leaf is a place the server's hand fragment is put in; the deck-hand
  // widget (decks plugin) owns everything inside it.
  Table.prototype.handHTML = function () {
    var self = this;
    Promise.resolve().then(function () { self.fillHand(); });
    return '<div class="tt-hand" id="tt-hand" data-tt-hand aria-busy="true"><p class="tt-from">Reading your hand…</p></div>';
  };

  Table.prototype.fillHand = function () {
    var self = this;
    $$('[data-tt-hand]').forEach(function (box) {
      if (box.hasAttribute('data-tt-filling')) return;
      box.setAttribute('data-tt-filling', '');
      var url = '/campaigns/' + encodeURIComponent(self.campaignId) + '/decks/hand';
      C.apiFetch(url, { headers: { 'HX-Request': 'true', 'Accept': 'text/html' } }).then(function (res) {
        if (!res.ok) throw new Error('status ' + res.status);
        return res.text();
      }).then(function (html) {
        if (!box.isConnected) return;
        box.innerHTML = html;
        box.removeAttribute('aria-busy');
        if (C.mountWidgets) C.mountWidgets(box);
      }).catch(function () {
        if (!box.isConnected) return;
        box.removeAttribute('aria-busy');
        box.innerHTML = '<p class="tt-from">Your hand could not be read.</p><button type="button" class="tt-quiet" data-tt-handretry>Try again</button>';
        var b = box.querySelector('[data-tt-handretry]');
        b.addEventListener('click', function () {
          box.innerHTML = '<p class="tt-from">Reading your hand…</p>';
          box.setAttribute('aria-busy', 'true');
          self.fillHand();
        });
        b.focus();
      }).then(function () { box.removeAttribute('data-tt-filling'); });
    });
  };

  Table.prototype.older = function () {
    var self = this, first = this.rolls[0];
    if (!first) return;
    C.apiFetch(this.base + '?limit=' + PAGE + '&before=' + encodeURIComponent(first.id)).then(function (res) {
      return res.ok ? res.json() : null;
    }).then(function (page) {
      if (!page) return;
      self.more = !!page.more;
      var older = (page.rolls || []).filter(function (r) { return !self.byId[r.id]; });
      older.forEach(function (r) { r.mine = !!r.rollerId && r.rollerId === self.userId; self.byId[r.id] = r; });
      self.rolls = older.concat(self.rolls);
      self.refreshLog(true);
    });
  };

  // ---------- What a roll says ----------

  Table.prototype.seenBy = function (mode) {
    return { 'public': 'Everyone', gm: 'The ' + this.gm + ' and the roller', blind: 'Only the ' + this.gm, self: 'Only the roller' }[mode] || '';
  };

  // modeChip names a hidden roll's mode on its row; public rolls have none.
  Table.prototype.modeChip = function (r) {
    if (r.mode === 'gm') return this.gm + ' only';
    if (r.mode === 'blind') return 'Blind';
    if (r.mode === 'self') return 'Just me';
    return '';
  };

  Table.prototype.who = function (r) {
    if (r.mine) return 'You';
    return r.rollerName || (r.origin === 'foundry' ? 'Someone in Foundry' : 'Someone');
  };

  function fromWhere(r) {
    if (r.from) return r.from;
    if (r.origin === 'foundry') return 'Foundry';
    return 'The Table roller';
  }

  // detailHTML is the same under a log row and under the island.
  Table.prototype.detailHTML = function (r) {
    var rows = '';
    if (r.hidden) {
      rows += '<dt>Result</dt><dd>Only the ' + esc(this.gm) + ' sees it</dd><dt>Dice</dt><dd>' + esc(r.formula) + '</dd>';
    } else {
      var faces = (r.dice || []).map(function (d) {
        return '<span class="tt-face" title="d' + num(d.sides) + '">' + num(d.value) + '</span>';
      }).join('<span>+</span>');
      var mod = num(r.modifier);
      if (mod) faces += '<span>' + (mod > 0 ? '+' : '−') + '</span><span>' + Math.abs(mod) + '</span>';
      rows += '<dt>Math</dt><dd class="tt-math">' + (faces || esc(r.formula)) + '<span>=</span><span class="tt-eq">' + esc(r.total) + '</span></dd>';
      if (r.outcome) rows += '<dt>Result</dt><dd>' + esc(r.outcome) + '</dd>';
      var pr = this.pread(r);
      if (pr) rows += '<dt>Power roll</dt><dd>' + esc(C.powerRoll.headline(pr)) + '</dd>';
    }
    if (r.why) rows += '<dt>Why</dt><dd>' + esc(r.why) + '</dd>';
    rows += '<dt>From</dt><dd>' + esc(fromWhere(r)) + (r.origin === 'foundry' ? ' <span class="tt-chip">Foundry</span>' : '') + '</dd>';
    // Whose dice the roll was thrown in, named from the dice catalogue, which is read the
    // first time a roll that carries its dice is opened.
    if (!r.hidden && r.rollerDice && C.rollDice) {
      if (!this.cat) this.loadCatalogue();
      var dn = C.rollDice.name(r, this.cat, r.mine ? 'Your' : (r.rollerName || 'Their') + '’s');
      if (dn) rows += '<dt>Dice</dt><dd>' + esc(dn) + '</dd>';
    }
    rows += '<dt>Seen by</dt><dd>' + esc(this.seenBy(r.mode)) + '</dd>';
    return '<dl class="tt-dl">' + rows + '</dl>';
  };

  Table.prototype.rowHTML = function (r) {
    var open = this.openRow === r.id, chip = this.modeChip(r);
    var chips = '';
    if (r.origin === 'foundry') chips += '<span class="tt-chip">Foundry</span>';
    if (r.source === 'crafting') chips += '<span class="tt-chip">Crafting</span>';
    if (chip) chips += '<span class="tt-chip tt-dmc">' + esc(chip) + '</span>';
    if (!r.hidden && r.outcome) chips += '<span class="tt-chip tt-out">' + esc(r.outcome) + '</span>';
    var did = 'tt-det-' + esc(r.id);
    return '<li data-id="' + esc(r.id) + '"' + (chip ? ' class="tt-dm"' : '') + '>' +
      '<button type="button" class="tt-row" data-tt-row="' + esc(r.id) + '" aria-expanded="' + open + '" aria-controls="' + did + '">' +
      '<span class="tt-who" aria-hidden="true">' + esc(initials(r.rollerName)) + '</span>' +
      '<span class="tt-l1"><b>' + esc(this.who(r)) + '</b><span>' + esc(r.label || r.formula) + '</span>' + chips + '</span>' +
      '<span class="tt-tot' + (r.hidden ? ' tt-hid' : '') + (this.pread(r) ? ' tt-tier' : '') + '">' + esc(this.figure(r)) + '</span>' +
      '<span class="tt-l2">' + esc(r.formula) + ' · ' + esc(timeOf(r.createdAt)) + '</span></button>' +
      '<div class="tt-detail" id="' + did + '"' + (open ? '' : ' hidden') + '>' + this.detailHTML(r) + '</div></li>';
  };

  Table.prototype.logHTML = function () {
    if (!this.loaded && !this.loadError) return '<ul class="tt-log" id="tt-log"><li class="tt-empty">Reading the roll log…</li></ul>';
    var head = this.loadError ? '<li class="tt-empty tt-err">' + esc(this.loadError) + '</li>' : '';
    if (this.more) head += '<li class="tt-empty"><button type="button" class="tt-quiet" data-tt-older>Show older rolls</button></li>';
    if (!this.rolls.length) return '<ul class="tt-log" id="tt-log">' + head + (this.loadError ? '' : '<li class="tt-empty">No rolls yet. Pick a die below, or roll from a sheet.</li>') + '</ul>';
    var self = this;
    return '<ul class="tt-log" id="tt-log" aria-label="Roll log">' + head + this.rolls.map(function (r) { return self.rowHTML(r); }).join('') + '</ul>';
  };

  // The power roll line: the hero's characteristics as one-press rolls, and a
  // five-step edge or bane set before the press. A person with no hero numbers
  // types their bonus instead.
  Table.prototype.powerHTML = function () {
    var pw = this.power;
    if (!pw) return '';
    var self = this;
    var known = pw.characteristics.some(function (c) { return c.value != null; });
    var head = '<p class="tt-k">Power roll · ' + esc(pw.dice) + ' + a characteristic' + (pw.hero ? ' <span class="tt-from">' + esc(pw.hero) + '</span>' : '') + '</p>';
    var row;
    if (known) {
      row = '<div class="tt-pstats">' + pw.characteristics.map(function (c) {
        var v = c.value == null ? 0 : num(c.value);
        return '<button type="button" class="tt-dbtn tt-pchip" data-tt-pstat="' + esc(c.key) + '" aria-pressed="' + (self.lastStat === c.key) + '">' + esc(c.label) +
          ' <b>' + (v > 0 ? '+' + v : v) + '</b></button>';
      }).join('') + '</div>';
    } else {
      row = '<div class="tt-pstats"><label class="tt-pbonus"><span class="tt-from">Your bonus</span>' +
        '<input class="tt-field" id="tt-pbonus" type="number" inputmode="numeric" min="-9" max="20" step="1" value="' + this.myBonus() + '"></label>' +
        '<button type="button" class="tt-dbtn" data-tt-pplain>Power roll</button></div>';
    }
    var steps = [[-2, 'Double bane', '2× bane'], [-1, 'Bane', 'Bane'], [0, 'Plain', 'Plain'], [1, 'Edge', 'Edge'], [2, 'Double edge', '2× edge']];
    var seg = '<div class="tt-seg tt-pedge" role="group" aria-label="Edge or bane">' + steps.map(function (st) {
      return '<button type="button" data-tt-pstep="' + st[0] + '" aria-pressed="' + (self.step === st[0]) + '" aria-label="' + st[1] + '"><span class="tt-pw" aria-hidden="true">' + st[1] + '</span><span class="tt-pn" aria-hidden="true">' + st[2] + '</span></button>';
    }).join('') + '</div>';
    return '<div class="tt-power" role="group" aria-label="Power roll">' + head + row + seg +
      '<p class="tt-from">Press a characteristic to roll it. Edge or bane is set first and clears after the roll.</p></div>';
  };

  // powerRoll asks the server for one power roll; the edge or bane clears after.
  // A roll of the hero's own number names the hero, so Foundry posts it as
  // the hero's actor.
  Table.prototype.powerRoll = function (name, value, key, heroId) {
    var p = C.powerRoll.plan(this.power, name, value, this.step);
    this.lastStat = key || '';
    this.setStep(0);
    this.roll(p.formula, p.label, heroId);
    $$('[data-tt-pstat]').forEach(function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-tt-pstat') === key)); });
  };

  Table.prototype.setStep = function (n) {
    this.step = n;
    $$('[data-tt-pstep]').forEach(function (b) { b.setAttribute('aria-pressed', String(+b.getAttribute('data-tt-pstep') === n)); });
  };

  Table.prototype.rollerHTML = function () {
    var gm = esc(this.gm);
    var modes = this.director ?
      [['public', 'Everyone sees it'], ['gm', gm + 's only'], ['self', 'Just me']] :
      [['public', 'Everyone sees it'], ['gm', 'The ' + gm + ' and me'], ['blind', 'Blind: only the ' + gm], ['self', 'Just me']];
    var die = function (s) {
      return '<button type="button" class="tt-dbtn" data-tt-quick="d' + s + '" aria-label="Roll a d' + s + '"><span class="tt-mini" aria-hidden="true">' + C.tableDice.svg(s) + '</span>d' + s + '</button>';
    };
    var pw = this.power;
    var buttons = (pw ? POWER_QUICK : QUICK).map(die).join('');
    var rest = '';
    if (pw) {
      rest = '<button type="button" class="tt-quiet" data-tt-other aria-expanded="' + this.otherDice + '" aria-controls="tt-otherdice">Other dice</button>' +
        '<span class="tt-dice tt-other" id="tt-otherdice"' + (this.otherDice ? '' : ' hidden') + '>' +
        QUICK.filter(function (s) { return POWER_QUICK.indexOf(s) < 0; }).map(die).join('') + '</span>';
    }
    return '<form class="tt-roller" data-tt-form novalidate>' + this.powerHTML() + '<div class="tt-dice">' + buttons + rest + '</div>' +
      '<div class="tt-crow"><label class="tt-sr" for="tt-fx">Dice to roll</label><input class="tt-field" id="tt-fx" autocomplete="off" maxlength="120" placeholder="' + (pw ? '2d10+2, d6, d3' : '2d10+2, d20, 4d6') + '">' +
      '<label class="tt-sr" for="tt-mode">Who sees it</label><select class="tt-mode" id="tt-mode">' +
      modes.map(function (m) { return '<option value="' + m[0] + '">' + m[1] + '</option>'; }).join('') + '</select>' +
      '<button type="submit" class="tt-go">Roll</button></div><p class="tt-ferr" id="tt-ferr" role="alert" hidden></p></form>';
  };

  Table.prototype.diceHTML = function () {
    var self = this, mine = this.myLook();
    var seg = function (attr, cur) {
      return LOOKS.map(function (l) { return '<button type="button" ' + attr + '="' + l[0] + '" aria-pressed="' + (cur === l[0]) + '">' + l[1] + '</button>'; }).join('');
    };
    var note = { quick: 'Light and quick on any device, and fine on a busy table.', rune: 'The rune dice, thrown in 3D. Dice they don’t come in play as quick 2D.', none: 'No dice are thrown: the result goes straight to the notice and the log.' }[mine];
    var out = '<div class="tt-mydice"><div><p class="tt-k">Your dice</p><p class="tt-from">How dice are drawn for you. It changes nothing anyone else sees.</p></div>' +
      '<div class="tt-seg" role="group" aria-label="How your dice are drawn">' + seg('data-tt-look', mine) + '</div><p class="tt-from">' + note + '</p>';
    if (this.look) out += '<button type="button" class="tt-quiet" data-tt-look-reset>Use the table’s choice (' + esc(lookName(this.tableLook)) + ')</button>';
    var cseg = [['on', 'On'], ['off', 'Off']].map(function (c) {
      return '<button type="button" data-tt-crits="' + c[0] + '" aria-pressed="' + ((c[0] === 'on') === self.crits) + '">' + c[1] + '</button>';
    }).join('');
    out += '<div><p class="tt-k">Crit animations</p></div><div class="tt-seg" role="group" aria-label="Crit animations">' + cseg + '</div>' +
      '<p class="tt-from">Off keeps a crit’s coloured edge on the dice, without the effects. Only for you.</p>';
    if (this.director) {
      out += '<div><p class="tt-k">The table’s dice</p><p class="tt-from">What a player who hasn’t picked starts on.</p></div>' +
        '<div class="tt-seg" role="group" aria-label="The table’s dice">' + seg('data-tt-tablelook', self.tableLook) + '</div>';
    }
    if (mine === 'rune') out += this.pickHTML();
    out += '<div class="tt-ptray" id="tt-ptray"></div><button type="button" class="tt-quiet" data-tt-try>Throw a test set</button></div>';
    return out;
  };

  // The person's own rune dice: a die, its colour and the two crit animations, each starting on
  // "the campaign's". Needs the catalogue, which the first draw asks for.
  Table.prototype.pickHTML = function () {
    var self = this;
    if (!this.camp) return '';
    if (!this.camp.playersPick) return '<p class="tt-from">Your ' + esc(this.gm) + ' has set everyone’s dice.</p>';
    if (!this.cat) { this.loadCatalogue(); return '<p class="tt-from">Reading the dice…</p>'; }
    var p = this.personal, cat = this.cat;
    var sets = cat.sets.filter(function (x) { return self.offered(x.key); });
    var set = sets.filter(function (x) { return x.key === p.set; })[0];
    var opt = function (v, name, cur) { return '<option value="' + esc(v) + '"' + (v === cur ? ' selected' : '') + '>' + esc(name) + '</option>'; };
    var sel = function (id, label, body) {
      return '<label class="tt-pick"><span class="tt-from">' + label + '</span><select class="tt-mode" data-tt-dpick="' + id + '">' + body + '</select></label>';
    };
    var out = '<div class="tt-picks"><div><p class="tt-k">Your rune dice</p><p class="tt-from">Your rolls are thrown in these on everyone’s screen. Saved with your account.</p></div>';
    if (this.pickErr) out += '<p class="tt-ferr" role="alert">' + esc(this.pickErr) + '</p>';
    out += sel('set', 'Die', opt('', 'The campaign’s die', set ? p.set : '') + sets.map(function (x) { return opt(x.key, x.name, p.set); }).join(''));
    if (set) {
      var col = set.colours.some(function (c) { return c.key === p.colour; }) ? p.colour : set.colours[0].key;
      out += sel('colour', 'Colour', set.colours.map(function (c) { return opt(c.key, c.name, col); }).join(''));
    }
    var crit = function (id, label, list) {
      return sel(id, label, opt('', 'The campaign’s', p[id]) + list.map(function (c) { return opt(c.key, c.name, p[id]); }).join(''));
    };
    out += crit('succ', 'Crit success', cat.crits.succ) + crit('fail', 'Crit fail', cat.crits.fail);
    if (this.hasPicks()) out += '<button type="button" class="tt-quiet" data-tt-picks-reset>Use the campaign’s dice</button>';
    return out + '</div>';
  };

  Table.prototype.loadCatalogue = function () {
    var self = this;
    if (this.catAsked || !C.dice || typeof C.dice.catalogue !== 'function') return;
    this.catAsked = true;
    C.dice.catalogue().then(function (c) {
      self.cat = c;
      if (!self.alive) return;
      if (self.open) self.renderSection('dice');
      // Rows' details are drawn with the rows, so they are redrawn to name their dice.
      if (self.loaded) self.refreshLog(true);
    }).catch(function () { self.catAsked = false; });
  };

  function lookName(k) { var l = LOOKS.filter(function (x) { return x[0] === k; })[0]; return l ? l[1] : k; }

  Table.prototype.sectionHTML = function (id) {
    if (id === 'rolls') return this.logHTML() + this.rollerHTML();
    if (id === 'tonight') return this.tonightHTML();
    if (id === 'hand') return this.handHTML();
    return this.diceHTML();
  };

  Table.prototype.leafHTML = function (i) {
    var cur = this.leaves[i], self = this;
    var menu = this.sections.map(function (s) {
      var j = self.leaves.indexOf(s[0]);
      var note = s[0] === cur ? 'Showing here' : j >= 0 ? 'Swaps with leaf ' + (j + 1) : '';
      return '<button type="button" role="menuitemradio" aria-checked="' + (s[0] === cur) + '" data-tt-leaf="' + i + '" data-tt-sec="' + s[0] + '"><span>' + s[1] + '</span><span class="tt-mnote">' + note + '</span></button>';
    }).join('');
    return '<section class="tt-leaf' + (this.phoneLeaf === i ? ' tt-on-phone' : '') + '" aria-label="' + SECTION_NAME[cur] + '">' +
      '<div class="tt-ltabs"><button type="button" class="tt-lpick" data-tt-pick="' + i + '" aria-haspopup="menu" aria-expanded="false" aria-controls="tt-lm-' + i + '">' + SECTION_NAME[cur] +
      '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg></button>' +
      '<div class="tt-lmenu" id="tt-lm-' + i + '" role="menu" aria-label="Show in this leaf" hidden>' + menu + '</div></div>' +
      '<div class="tt-lbody" id="tt-lb-' + i + '">' + this.sectionHTML(cur) + '</div></section>';
  };

  Table.prototype.drawerHTML = function () {
    var self = this;
    var sw = '<div class="tt-phone-sw" role="group" aria-label="Leaves">' + this.leaves.map(function (s, i) {
      return '<button type="button" class="tt-ptab" data-tt-phone="' + i + '" aria-pressed="' + (self.phoneLeaf === i) + '">' + SECTION_NAME[s] + '</button>';
    }).join('') + '</div>';
    var body = '<div class="tt-dhead"><h2>Table</h2><button type="button" class="tt-icon" data-tt-close aria-label="Close the Table">' + X + '</button></div>' + this.waitHTML() + sw +
      '<div class="tt-leaves" style="--tt-n:' + this.leaves.length + '">' + this.leaves.map(function (s, i) { return self.leafHTML(i); }).join('') + '</div>';
    if (!this.sheet) return body;
    // Resting, the sheet's contents sit off screen: inert keeps them out of
    // the tab order until it is lifted.
    return this.grabHTML() + '<div class="tt-sheetbody" id="tt-sheetbody"' + (this.h === 'rest' ? ' inert' : '') + '>' + this.sheetMomentHTML() + body + '</div>';
  };

  // ---------- The phone's sheet ----------

  Table.prototype.syncSheet = function () {
    var on = !!this.mql.matches;
    if (on === this.sheet) return;
    this.sheet = on;
    this.leaves = this.readLeaves(store('leaves.' + this.key));
    var d = this.drawer(); if (d) d.remove();
    this.open = false;
    this.h = 'rest';
    this.tab.setAttribute('aria-expanded', 'false');
    if (on) {
      document.documentElement.setAttribute('data-tt-sheet', '');
      this.ensureSheet();
      if (!this.loaded) this.load();
      // The sheet holds Tonight from the start, so its list is read now too.
      if (!this.shownLoaded) this.loadShown();
    } else {
      document.documentElement.removeAttribute('data-tt-sheet');
    }
  };

  Table.prototype.ensureSheet = function () {
    var d = this.drawer();
    if (d && d.classList.contains('tt-sheet')) return d;
    if (d) d.remove();
    d = document.createElement('div');
    d.id = 'tt-drawer';
    d.className = 'tt-drawer tt-sheet';
    d.dataset.h = this.h;
    d.setAttribute('role', 'region');
    d.setAttribute('aria-label', 'Table');
    d.innerHTML = this.drawerHTML();
    document.body.appendChild(d);
    this.wireSheet(d);
    this.refreshGrab(false);
    return d;
  };

  Table.prototype.grabHTML = function () {
    return '<button type="button" class="tt-grab" data-tt-grab aria-expanded="' + (this.h !== 'rest') + '" aria-controls="tt-sheetbody" aria-label="The table: rolls and dice">' +
      '<span class="tt-grip" aria-hidden="true"></span><span class="tt-last" data-tt-last>' + esc(this.lastRollText()) + '</span>' +
      '<span class="tt-gn" data-tt-gcount hidden></span></button>';
  };

  // The resting line: who rolled last and what came up.
  Table.prototype.lastRollText = function () {
    var r = this.rolls[this.rolls.length - 1];
    if (!r) return 'The table';
    return this.who(r) + ' · ' + (r.label || r.formula) + ' · ' + (r.hidden ? '?' : r.total);
  };

  // refreshGrab redraws only the line's words and count, so a drag on it is
  // never interrupted; move is true for a new roll, which nudges the text.
  Table.prototype.refreshGrab = function (move) {
    var last = $('[data-tt-last]'), n = $('[data-tt-gcount]');
    if (!last) return;
    var text = this.lastRollText();
    if (last.textContent !== text) {
      last.textContent = text;
      if (move && motion() !== 'off') { last.classList.remove('tt-grow'); void last.offsetWidth; last.classList.add('tt-grow'); }
    }
    if (n) {
      var total = this.total();
      n.hidden = !total;
      n.textContent = total > 99 ? '99+' : String(total);
    }
  };

  Table.prototype.leafTo = function (sec) {
    var i = this.leaves.indexOf(sec);
    if (i < 0 || this.phoneLeaf === i) return;
    this.phoneLeaf = i;
    var keep = this.keepForm(), d = this.drawer();
    if (d) d.innerHTML = this.drawerHTML();
    this.restoreForm(keep);
  };

  // setHeight moves the sheet to a height. The move itself is CSS: a
  // transform on [data-h], so dragging and letting go share one path.
  Table.prototype.setHeight = function (h) {
    var d = this.ensureSheet();
    this.h = h;
    d.dataset.h = h;
    d.style.transform = '';
    d.classList.remove('tt-drag');
    this.open = h !== 'rest';
    if (motion() === 'calm') { d.classList.remove('tt-fadein'); void d.offsetWidth; d.classList.add('tt-fadein'); }
    if (h === 'lift') this.leafTo('rolls');
    var body = $('.tt-sheetbody', d), grab = $('[data-tt-grab]', d);
    if (body) { if (h === 'rest') body.setAttribute('inert', ''); else body.removeAttribute('inert'); }
    if (grab) grab.setAttribute('aria-expanded', String(h !== 'rest'));
    this.closeMenus();
    if (h === 'rest') {
      var a = document.activeElement;
      if (a && d.contains(a) && a !== grab && a.blur) a.blur();
    } else if (h === 'full') {
      this.scrollLog();
    }
    this.seen();
    this.refreshGrab(false);
  };

  Table.prototype.stepSheet = function (dir) {
    var i = HEIGHTS.indexOf(this.h);
    if (dir === undefined) dir = this.h === 'full' ? -1 : 1;
    this.setHeight(HEIGHTS[Math.max(0, Math.min(2, i + dir))]);
  };

  // The three resting places, as how far down the sheet's own height each sits.
  Table.prototype.offsets = function (d) {
    var H = d.offsetHeight, probe = document.createElement('div');
    probe.style.cssText = 'position:fixed;visibility:hidden;padding-bottom:env(safe-area-inset-bottom,0px)';
    document.body.appendChild(probe);
    var safe = parseFloat(getComputedStyle(probe).paddingBottom) || 0;
    probe.remove();
    return { rest: Math.max(0, H - REST_PX - safe), lift: Math.max(0, H - LIFT_SHARE * W.innerHeight), full: 0 };
  };

  // Dragging follows the finger 1:1 and lets go to the nearest height; a
  // quick flick goes one height in its direction; a tap goes one up.
  Table.prototype.wireSheet = function (d) {
    var self = this, st = null;
    d.addEventListener('pointerdown', function (ev) {
      var g = ev.target.closest && ev.target.closest('[data-tt-grab]');
      if (!g || (ev.pointerType === 'mouse' && ev.button !== 0)) return;
      var off = self.offsets(d);
      st = { id: ev.pointerId, y0: ev.clientY, from: off[self.h], off: off, moved: false, t0: Date.now(), samples: [[Date.now(), ev.clientY]] };
      try { g.setPointerCapture(ev.pointerId); } catch (e) { /* the drag still follows the sheet */ }
    });
    d.addEventListener('pointermove', function (ev) {
      if (!st || ev.pointerId !== st.id) return;
      var dy = ev.clientY - st.y0;
      if (!st.moved && Math.abs(dy) < 6) return;
      st.moved = true;
      d.classList.add('tt-drag');
      var y = Math.max(0, Math.min(st.off.rest, st.from + dy));
      d.style.transform = 'translateY(' + y + 'px)';
      st.y = y;
      st.samples.push([Date.now(), ev.clientY]);
      if (st.samples.length > 6) st.samples.shift();
    });
    var end = function (ev) {
      if (!st || ev.pointerId !== st.id) return;
      var s = st; st = null;
      if (!s.moved) { if (ev.type === 'pointerup') self.stepSheet(); return; }
      var a = s.samples[0], b = s.samples[s.samples.length - 1];
      var v = b[0] > a[0] ? (b[1] - a[1]) / (b[0] - a[0]) : 0;
      var idx = HEIGHTS.indexOf(self.h), to;
      if (Math.abs(v) > 0.5) to = Math.max(0, Math.min(2, idx + (v < 0 ? 1 : -1)));
      else {
        to = 0;
        HEIGHTS.forEach(function (h, i) { if (Math.abs(s.off[h] - s.y) < Math.abs(s.off[HEIGHTS[to]] - s.y)) to = i; });
      }
      self.setHeight(HEIGHTS[to]);
    };
    d.addEventListener('pointerup', end);
    d.addEventListener('pointercancel', end);
  };

  // ---------- Opening, closing, leaves ----------

  Table.prototype.drawer = function () { return document.getElementById('tt-drawer'); };

  Table.prototype.openTable = function (sec) {
    if (this.sheet) {
      if (sec) {
        var k = this.leaves.indexOf(sec);
        if (k < 0) { k = this.leaves.length - 1; this.leaves[k] = sec; this.saveLeaves(); }
        if (sec !== 'rolls') this.leafTo(sec);
      }
      this.setHeight(sec && sec !== 'rolls' ? 'full' : this.h === 'full' ? 'full' : 'lift');
      if (!this.loaded) this.load();
      if (!this.shownLoaded) this.loadShown();
      this.loadCuts();
      return;
    }
    if (sec) {
      var i = this.leaves.indexOf(sec);
      if (i < 0) { i = this.leaves.length - 1; this.leaves[i] = sec; this.saveLeaves(); }
      this.phoneLeaf = i;
    }
    var was = this.open;
    this.open = true;
    // The drawer lists what is waiting, so a notice hanging beside it goes.
    if (this.momOpen && !this.sheet) this.closeMoment(false);
    this.tab.setAttribute('aria-expanded', 'true');
    var d = this.drawer();
    if (!d) {
      d = document.createElement('div');
      d.id = 'tt-drawer';
      d.className = 'tt-drawer';
      d.setAttribute('role', 'region');
      d.setAttribute('aria-label', 'Table');
      this.header.appendChild(d);
    }
    d.innerHTML = this.drawerHTML();
    if (!was && this.draft) { this.restoreForm(this.draft); this.draft = null; }
    if (!was && motion() !== 'off') {
      d.classList.add('tt-opening');
      d.addEventListener('animationend', function () { d.classList.remove('tt-opening'); }, { once: true });
    }
    if (!this.loaded) this.load();
    if (!this.shownLoaded) this.loadShown();
    this.loadCuts();
    this.scrollLog();
    this.seen();
  };

  // closeTable keeps a half-typed roll for next time, since a click off the
  // drawer closes it too. refocus is false for that click, so focus stays
  // where the person pressed.
  Table.prototype.closeTable = function (refocus) {
    if (!this.open) return;
    if (this.sheet) {
      // The sheet stays on the page, so a half-typed roll stays in its field.
      this.setHeight('rest');
      return;
    }
    var self = this, d = this.drawer();
    var keep = this.keepForm();
    this.draft = keep.fx || keep.mode ? { fx: keep.fx, mode: keep.mode } : null;
    var finish = function () {
      if (self.open) return;
      if (d) d.remove();
    };
    this.open = false;
    this.closeMenus();
    this.tab.setAttribute('aria-expanded', 'false');
    if (d && motion() !== 'off') { d.classList.add('tt-closing'); setTimeout(finish, 120); } else finish();
    if (refocus !== false) this.tab.focus();
  };

  Table.prototype.saveLeaves = function () { store('leaves.' + this.key, this.leaves.join(',')); };

  // Picking a section another leaf shows swaps the two, so nothing shows twice.
  Table.prototype.setLeaf = function (i, sec) {
    var j = this.leaves.indexOf(sec);
    if (j === i) return;
    if (j >= 0) this.leaves[j] = this.leaves[i];
    this.leaves[i] = sec;
    this.saveLeaves();
    var keep = this.keepForm();
    var d = this.drawer();
    if (d) d.innerHTML = this.drawerHTML();
    this.restoreForm(keep);
    $$('.tt-lbody', d).forEach(function (b) { if (motion() === 'full') b.classList.add('tt-settle'); });
    this.scrollLog();
    this.seen();
  };

  Table.prototype.renderSection = function (sec) {
    var i = this.leaves.indexOf(sec), b = document.getElementById('tt-lb-' + i);
    if (i < 0 || !b) return;
    var keep = this.keepForm();
    b.innerHTML = this.sectionHTML(sec);
    this.restoreForm(keep);
  };

  // keepForm and restoreForm carry a half-typed roll across a redraw.
  Table.prototype.keepForm = function () {
    var fx = document.getElementById('tt-fx'), mode = document.getElementById('tt-mode');
    return { fx: fx ? fx.value : '', mode: mode ? mode.value : '', focus: document.activeElement && document.activeElement.id };
  };
  Table.prototype.restoreForm = function (k) {
    var fx = document.getElementById('tt-fx'), mode = document.getElementById('tt-mode');
    if (fx) fx.value = k.fx || '';
    if (mode && k.mode && mode.querySelector('option[value="' + k.mode + '"]')) mode.value = k.mode;
    if (k.focus === 'tt-fx' && fx) fx.focus();
  };

  // Only the log is redrawn when a roll lands, so the roller keeps its text.
  Table.prototype.refreshLog = function (keepScroll) {
    var log = document.getElementById('tt-log');
    if (!log) return;
    var top = log.scrollTop, height = log.scrollHeight;
    var tmp = document.createElement('div');
    tmp.innerHTML = this.logHTML();
    log.replaceWith(tmp.firstChild);
    log = document.getElementById('tt-log');
    if (keepScroll) log.scrollTop = top + (log.scrollHeight - height);
    else log.scrollTop = log.scrollHeight;
  };

  Table.prototype.scrollLog = function () { var log = document.getElementById('tt-log'); if (log) log.scrollTop = log.scrollHeight; };

  Table.prototype.logVisible = function () {
    if (!this.open) return false;
    var i = this.leaves.indexOf('rolls');
    if (i < 0) return false;
    if (this.sheet) return this.h === 'full' && this.phoneLeaf === i;
    return W.matchMedia('(min-width: 761px)').matches || this.phoneLeaf === i;
  };

  // The Table's count has two sources: rolls this browser has not seen, and
  // the moments the server says are waiting on a Director. Looking at the roll
  // log clears only the first.
  Table.prototype.total = function () { return this.unseen + this.moments.count; };

  Table.prototype.paintCount = function (grow) {
    var c = this.count, n = this.total();
    c.hidden = !n;
    c.textContent = n > 99 ? '99+' : String(n);
    if (grow && n) {
      c.classList.remove('hdr-n-grow');
      void c.offsetWidth;
      c.classList.add('hdr-n-grow');
    }
    this.refreshGrab(false);
  };

  Table.prototype.seen = function () {
    if (!this.logVisible()) return;
    this.unseen = 0;
    this.paintCount(false);
  };

  Table.prototype.bump = function () {
    if (this.logVisible()) return;
    this.unseen++;
    this.paintCount(true);
  };

  Table.prototype.closeMenus = function () {
    $$('.tt-lmenu').forEach(function (m) { m.hidden = true; });
    $$('.tt-lpick').forEach(function (p) { p.setAttribute('aria-expanded', 'false'); });
  };

  // ---------- Dice and the notice ----------

  Table.prototype.tray = function () {
    var t = document.getElementById('tt-tray');
    if (!t) {
      t = document.createElement('div');
      t.id = 'tt-tray';
      t.className = 'tt-tray';
      t.setAttribute('aria-hidden', 'true');
      document.body.appendChild(t);
    }
    return t;
  };

  // throwDice draws the roll's dice the way this person picked, and resolves
  // once they land. A hidden roll throws nothing.
  Table.prototype.throwDice = function (r, tray) {
    var look = this.myLook(), dice = r.dice || [];
    if (r.hidden || look === 'none' || !dice.length) return Promise.resolve();
    var rune = look === 'rune' && C.dice && typeof C.dice.roll === 'function' &&
      dice.every(function (d) { return RUNE_SIDES[d.sides]; });
    // Rune dice are thrown full screen whether or not a preview tray was offered: the tray is for
    // quick 2D. Every roll wears the dice the server settled for its roller (rollDice.forRoll).
    if (rune) {
      // The server decides which dice crit under the campaign's game system. With animations off
      // every die says 'none', which dice.js treats as a die's own word, so nothing plays.
      var crits = this.crits;
      return C.dice.roll([{ label: r.label || r.formula, dice: dice.map(function (d) {
        return { sides: d.sides, value: d.value, crit: crits && (d.crit === 'succ' || d.crit === 'fail') ? d.crit : 'none' };
      }) }], {
        table: this.diceFor(r),
        describe: function () { return ''; }
      }).then(function () {}, function () {});
    }
    return C.tableDice.throwInto(tray || this.tray(), dice, { fromLeft: !!r.mine, flash: this.crits });
  };

  Table.prototype.play = function (r) {
    var self = this;
    this.throwDice(r).then(function () {
      if (!self.alive) return;
      if (self.open || self.sheet) self.refreshLog();
      if (self.sheet) {
        // The sheet never lifts for a roll: the resting line changes.
        if (!self.logVisible()) self.bump();
        self.refreshGrab(true);
        if (self.logVisible()) {
          var row = $('#tt-log li[data-id="' + (W.CSS && CSS.escape ? CSS.escape(r.id) : r.id) + '"]');
          if (row) row.classList.add('tt-fresh');
        }
        return;
      }
      if (self.logVisible()) {
        var li = $('#tt-log li[data-id="' + (W.CSS && CSS.escape ? CSS.escape(r.id) : r.id) + '"]');
        if (li) li.classList.add('tt-fresh');
      } else {
        self.notice(r);
      }
    });
  };

  Table.prototype.clip = function () {
    var c = $('.tt-isl-clip', this.header);
    if (!c) {
      c = document.createElement('div');
      c.className = 'tt-isl-clip';
      this.header.appendChild(c);
    }
    var right = this.header.getBoundingClientRect().right - this.tab.getBoundingClientRect().left;
    c.style.right = Math.max(0, right) + 'px';
    return c;
  };

  Table.prototype.say = function (text) {
    var live = document.getElementById('tt-live');
    if (!live) {
      live = document.createElement('p');
      live.id = 'tt-live';
      live.className = 'tt-sr';
      live.setAttribute('aria-live', 'polite');
      document.body.appendChild(live);
    }
    live.textContent = text;
  };

  // notice slides the island out from behind the Table tab, waits, and slides
  // it back in. While a roll's details are open, new rolls wait their turn.
  Table.prototype.notice = function (r) {
    if (r.shown) { this.noticeShown(r); return; }
    var who = r.hidden ? 'You rolled blind' : this.who(r) + (r.origin === 'foundry' ? ' in Foundry' : '');
    var num = this.figure(r);
    var what = (r.label || r.formula) + (!r.hidden && r.outcome ? ' · ' + r.outcome : '');
    this.say(who + ', ' + what + ': ' + (r.hidden ? 'sent to the ' + this.gm : num));
    if (this.isl && this.isl.expanded) { this.queue.push(r); return; }
    this.retire(this.isl, true);
    var self = this, c = this.clip(), b = document.createElement('button');
    b.type = 'button';
    b.className = 'tt-island' + (this.modeChip(r) ? ' tt-dm' : '');
    b.setAttribute('aria-expanded', 'false');
    b.setAttribute('aria-label', who + ', ' + what + ', ' + num + '. Show the details');
    var first = (r.dice && r.dice[0]) ? r.dice[0].sides : 20;
    b.innerHTML = '<span class="tt-mini tt-mini-l" aria-hidden="true">' + C.tableDice.svg(first) + '</span><span class="tt-who2">' + esc(who) + '</span>' +
      '<span class="tt-what2">' + esc(what) + '</span><span class="tt-num">' + esc(num) + '</span>' + CHEV;
    c.appendChild(b);
    var me = { el: b, r: r, timer: 0, expanded: false };
    this.isl = me;
    requestAnimationFrame(function () { requestAnimationFrame(function () { b.classList.add('tt-in'); }); });
    me.timer = setTimeout(function () { if (!me.expanded) self.retire(me); }, ISLAND_STAY);
    b.addEventListener('click', function () { if (me.expanded) self.collapse(me); else self.expand(me); });
  };

  // noticeShown is the island for something the director showed. It stays
  // out twice as long as a roll, since there is something to open.
  Table.prototype.noticeShown = function (it) {
    var who = 'The ' + this.gm + ' shows' + (it.to ? ' you' : '');
    var what = this.shownWhat(it);
    this.say(who + ': ' + what);
    if (this.isl && this.isl.expanded) { this.queue.push(it); return; }
    this.retire(this.isl, true);
    var self = this, c = this.clip(), b = document.createElement('button');
    b.type = 'button';
    b.className = 'tt-island tt-shown-isl';
    b.setAttribute('aria-expanded', 'false');
    b.setAttribute('aria-label', who + ': ' + what + '. Show it');
    b.innerHTML = '<span class="tt-who2">' + esc(who) + '</span><span class="tt-what2">' + esc(what) + '</span>' + CHEV;
    c.appendChild(b);
    var me = { el: b, r: it, timer: 0, expanded: false };
    this.isl = me;
    requestAnimationFrame(function () { requestAnimationFrame(function () { b.classList.add('tt-in'); }); });
    me.timer = setTimeout(function () { if (!me.expanded) self.retire(me); }, ISLAND_STAY * 2);
    b.addEventListener('click', function () { if (me.expanded) self.collapse(me); else self.expand(me); });
  };

  Table.prototype.retire = function (me, instant) {
    if (!me) return;
    var self = this;
    clearTimeout(me.timer);
    if (me === this.isl) this.isl = null;
    var b = me.el;
    if (instant || motion() === 'off') { b.remove(); this.bump(); return; }
    b.classList.remove('tt-in');
    b.classList.add('tt-out');
    setTimeout(function () { b.remove(); self.bump(); self.next(); }, 200);
  };

  Table.prototype.next = function () {
    if (this.queue.length && !this.isl) this.notice(this.queue.shift());
  };

  Table.prototype.expand = function (me) {
    clearTimeout(me.timer);
    me.expanded = true;
    me.el.setAttribute('aria-expanded', 'true');
    var d = document.createElement('div'), a = me.el.getBoundingClientRect();
    d.className = 'tt-isl-detail' + (motion() === 'off' ? '' : ' tt-opening');
    d.id = 'tt-isl-detail';
    d.setAttribute('role', 'region');
    d.setAttribute('aria-label', 'Roll details');
    d.style.right = Math.max(10, W.innerWidth - a.right) + 'px';
    var r = me.r;
    if (r.shown) {
      d.setAttribute('aria-label', 'What was shown');
      d.innerHTML = '<div class="tt-dhd"><b>' + esc(this.shownWhat(r)) + '</b></div>' + this.shownBody(r) +
        '<div class="tt-foot"><button type="button" class="tt-quiet" data-tt-totonight>See tonight’s list</button><button type="button" class="tt-quiet" data-tt-iclose>Done</button></div>';
      document.body.appendChild(d);
      me.detail = d;
      return;
    }
    d.innerHTML = '<div class="tt-dhd"><b>' + esc(r.label || r.formula) + '</b><span class="tt-bignum">' + esc(this.figure(r)) + '</span></div>' + this.detailHTML(r) +
      '<div class="tt-foot"><button type="button" class="tt-quiet" data-tt-tolog>Open in the roll log</button><button type="button" class="tt-quiet" data-tt-iclose>Done</button></div>';
    document.body.appendChild(d);
    me.detail = d;
  };

  Table.prototype.collapse = function (me) {
    var self = this, d = me.detail;
    me.expanded = false;
    me.el.setAttribute('aria-expanded', 'false');
    if (d) {
      d.classList.remove('tt-opening');
      d.classList.add('tt-closing');
      setTimeout(function () { d.remove(); }, motion() === 'off' ? 0 : 120);
    }
    me.timer = setTimeout(function () { self.retire(me); }, 900);
  };


  // ---------- A card's moment: what is waiting on a Director ----------

  // askMoments asks again soon, so a burst of messages is one request.
  Table.prototype.askMoments = function () {
    var self = this;
    clearTimeout(this.momTimer);
    this.momTimer = setTimeout(function () { if (self.alive) self.loadMoments(); }, 80);
  };

  Table.prototype.momentURL = function (tail) {
    return '/campaigns/' + encodeURIComponent(this.campaignId) + '/decks/moments' + tail;
  };

  // fetchMoments gets the Director's notice fragment: the open moments and the
  // one shown. Only a Director's Table ever calls it; the server answers anyone
  // else with a 404 and nothing.
  Table.prototype.fetchMoments = function (at) {
    return C.apiFetch(this.momentURL('/panel' + (at ? '?at=' + encodeURIComponent(at) : '')), { headers: { 'HX-Request': 'true', 'Accept': 'text/html' } }).then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.text();
    });
  };

  Table.prototype.parseMoments = function (html) {
    var doc = new DOMParser().parseFromString('<div>' + html + '</div>', 'text/html');
    var rows = $$('[data-dkm-index] li', doc).map(function (li) {
      return { id: li.getAttribute('data-id'), slide: li.getAttribute('data-slide') === 'true', deck: li.getAttribute('data-deck') || '', reading: li.getAttribute('data-reading') || '' };
    });
    var root = $('[data-dkm]', doc);
    return { rows: rows, at: root ? root.getAttribute('data-at') : '' };
  };

  Table.prototype.loadMoments = function () {
    if (!this.momentsOn || !this.alive) return Promise.resolve();
    var self = this, seq = ++this.momSeq;
    return this.fetchMoments(this.momOpen).then(function (html) {
      if (seq !== self.momSeq || !self.alive) return;
      self.receiveMoments(html);
    }).catch(function () { /* the count stays as it was */ });
  };

  // receiveMoments takes the server's list: it sets the count, redraws "Waiting
  // on you", keeps an open notice current (or closes it when its moment went
  // away) and slides in one that is new and not quiet.
  Table.prototype.receiveMoments = function (html) {
    var got = this.parseMoments(html), first = this.momKnown === null;
    var known = this.momKnown || {};
    var fresh = got.rows.filter(function (r) { return !known[r.id]; });
    this.momKnown = {};
    var self = this;
    got.rows.forEach(function (r) { self.momKnown[r.id] = true; });
    this.moments = { count: got.rows.length, rows: got.rows };
    this.paintCount(!first && fresh.length > 0);
    this.paintWait();
    if (this.momOpen) {
      var still = got.rows.some(function (r) { return r.id === self.momOpen; });
      if (still && got.at === this.momOpen) this.paintMoment(html);
      else if (still) this.loadMomentAt(this.momOpen);
      else this.closeMoment(false);
      return;
    }
    // A moment that was already waiting when the page opened is in the count;
    // only one that comes while the page is open slides in.
    var slide = first ? [] : fresh.filter(function (r) { return r.slide; });
    if (slide.length) this.openMoment(slide[0].id, { slide: true, deck: slide[0].deck });
  };

  Table.prototype.waitHTML = function () {
    if (!this.momentsOn || !this.moments.rows.length) return '';
    return '<div class="tt-wait" data-tt-wait><h3>Waiting on you</h3><ul>' + this.moments.rows.map(function (r) {
      return '<li><button type="button" class="tt-waitrow" data-tt-moment="' + esc(r.id) + '"><b>' + esc(r.deck) + '</b><span>' + esc(r.reading) + '</span></button></li>';
    }).join('') + '</ul></div>';
  };

  Table.prototype.paintWait = function () {
    var d = this.drawer();
    if (!d) return;
    var old = $('[data-tt-wait]', d), html = this.waitHTML();
    if (!html) { if (old) old.remove(); return; }
    if (old) { old.outerHTML = html; return; }
    var head = $('.tt-dhead', d);
    if (head) head.insertAdjacentHTML('afterend', html);
  };

  Table.prototype.sheetMomentHTML = function () {
    if (!this.momentsOn || !this.momOpen) return '';
    return '<div class="tt-moment tt-moment-sheet" id="tt-moment" role="region" aria-label="A card’s moment">' + this.momHTML + '</div>';
  };

  // momentBox is the notice's surface: under the Table tab on a wide screen, the
  // top of the sheet on a phone.
  Table.prototype.momentBox = function () {
    var box = document.getElementById('tt-moment');
    if (box) return box;
    box = document.createElement('div');
    box.id = 'tt-moment';
    box.setAttribute('role', 'region');
    box.setAttribute('aria-label', 'A card’s moment');
    if (this.sheet) {
      box.className = 'tt-moment tt-moment-sheet';
      var body = $('.tt-sheetbody', this.ensureSheet());
      body.insertBefore(box, body.firstChild);
    } else {
      box.className = 'tt-moment';
      document.body.appendChild(box);
    }
    return box;
  };

  Table.prototype.placeMoment = function (box) {
    if (this.sheet) return;
    var r = this.tab.getBoundingClientRect();
    box.style.right = Math.max(10, W.innerWidth - r.right) + 'px';
  };

  Table.prototype.paintMoment = function (html) {
    this.momHTML = html;
    var box = this.momentBox();
    box.innerHTML = html;
    this.placeMoment(box);
    if (C.mountWidgets) C.mountWidgets(box);
  };

  // openMoment shows one waiting moment. A slide-in comes by itself, so it
  // takes no focus and is said once in the live region; one the Director opened
  // (from "Waiting on you") puts focus on the notice's heading.
  Table.prototype.openMoment = function (id, opt) {
    opt = opt || {};
    var self = this, was = this.momOpen;
    return this.fetchMoments(id).then(function (html) {
      if (!self.alive) return;
      var got = self.parseMoments(html);
      if (got.at !== id) return;
      if (opt.slide) self.say('A card’s moment has come in ' + (opt.deck || 'a deck') + '.');
      // The open drawer already lists it, and a notice hung beside the drawer covers that list.
      if (self.open && !self.sheet) { if (opt.slide) return; self.closeTable(false); }
      self.momOpen = id;
      if (self.sheet && self.h === 'rest') self.setHeight('lift');
      self.paintMoment(html);
      var box = document.getElementById('tt-moment');
      if (!was && box && motion() !== 'off' && !self.sheet) {
        box.classList.add('tt-opening');
        box.addEventListener('animationend', function () { box.classList.remove('tt-opening'); }, { once: true });
      }
      if (!opt.slide) self.focusMoment();
    }).catch(function () { /* nothing to show */ });
  };

  Table.prototype.focusMoment = function () {
    var h = document.getElementById('dkm-h');
    if (h) h.focus({ preventScroll: true });
  };

  // loadMomentAt redraws the notice at one moment, after an answer or a step.
  // It is the newest word on what is waiting, so a list asked for earlier and
  // answered later is dropped; an answer that leaves nothing waiting hands
  // focus back to the Table tab, since the button that held it is gone.
  Table.prototype.loadMomentAt = function (id, then) {
    var self = this, seq = ++this.momSeq;
    return this.fetchMoments(id).then(function (html) {
      if (seq !== self.momSeq || !self.alive || !self.momOpen) return;
      var got = self.parseMoments(html);
      self.momKnown = {};
      got.rows.forEach(function (r) { self.momKnown[r.id] = true; });
      self.moments = { count: got.rows.length, rows: got.rows };
      self.paintCount(false);
      self.paintWait();
      if (!got.rows.length) {
        self.closeMoment(false);
        var back = then && (self.sheet ? $('[data-tt-grab]') : self.tab);
        if (back) back.focus();
        return;
      }
      self.momOpen = got.at;
      self.paintMoment(html);
      if (then) then();
    }).catch(function () { /* the notice stays as it was */ });
  };

  // closeMoment puts the notice away. The count stays: the moment is still
  // waiting. Focus goes to the Table tab only if it was inside the notice.
  Table.prototype.closeMoment = function (refocus) {
    var box = document.getElementById('tt-moment');
    if (!this.momOpen && !box) return;
    var inside = box && box.contains(document.activeElement);
    this.momOpen = '';
    this.momHTML = '';
    var finish = function () { if (box && box.parentNode && !box.hasAttribute('data-keep')) box.remove(); };
    if (box && motion() !== 'off' && !this.sheet) { box.classList.remove('tt-opening'); box.classList.add('tt-closing'); setTimeout(finish, 120); } else finish();
    if (inside && refocus !== false) { var t = this.sheet ? $('[data-tt-grab]') : this.tab; if (t) t.focus(); }
  };

  Table.prototype.momentSay = function (msg) {
    var box = document.getElementById('tt-moment'), line = box && $('[data-dkm-msg]', box);
    if (!line) return;
    line.textContent = msg || '';
    line.hidden = !msg;
  };

  // answerMoment sends one answer and redraws the notice from the server's
  // reply: the next waiting moment, or nothing. A 409 (someone else answered, or
  // the card has gone) is said in a line and the notice redraws.
  Table.prototype.answerMoment = function (btn) {
    var self = this, box = document.getElementById('tt-moment');
    var panel = box && $('[data-dkm-panel]', box);
    if (!panel || btn.disabled) return;
    var id = panel.getAttribute('data-moment'), act = btn.getAttribute('data-dkm-act');
    var label = btn.textContent;
    $$('button', panel).forEach(function (b) { b.disabled = true; });
    btn.setAttribute('aria-busy', 'true');
    btn.setAttribute('data-working', '');
    if (btn.getAttribute('data-working-label')) btn.textContent = btn.getAttribute('data-working-label');
    this.momentSay('');
    C.apiFetch(this.momentURL('/' + encodeURIComponent(id) + '/' + act), { method: 'POST', body: {} }).then(function (res) {
      return res.json().catch(function () { return {}; }).then(function (data) { return { ok: res.ok, status: res.status, data: data }; });
    }).then(function (out) {
      if (!self.alive) return;
      if (!out.ok) {
        var said = (out.data && out.data.message) || 'That did not go through. Try again.';
        return self.loadMomentAt(id, function () { self.momentSay(said); self.focusMoment(); });
      }
      // Dealt or put away: the next waiting moment shows. Another card or an
      // invitation: the same moment, redrawn.
      var next = act === 'deal' || act === 'dismiss' ? '' : id;
      return self.loadMomentAt(next, function () {
        var again = $('[data-dkm-act="' + (act === 'invite' ? 'dismiss' : act) + '"]', document.getElementById('tt-moment') || document);
        if (again && act !== 'deal' && act !== 'dismiss') again.focus({ preventScroll: true }); else self.focusMoment();
      });
    }).catch(function () {
      btn.textContent = label;
      btn.removeAttribute('aria-busy');
      btn.removeAttribute('data-working');
      $$('button', panel).forEach(function (b) { b.disabled = false; });
      self.momentSay('That did not go through. Check your connection and try again.');
    });
  };

  Table.prototype.wireMoments = function () {
    var self = this;
    this.on(document, 'click', function (ev) {
      var b = ev.target.closest && ev.target.closest('[data-dkm-act], [data-dkm-nav], [data-dkm-close], [data-tt-moment]');
      if (!b || !self.momentsOn) return;
      if (b.hasAttribute('data-dkm-close')) { self.closeMoment(); return; }
      if (b.hasAttribute('data-dkm-act')) { self.answerMoment(b); return; }
      if (b.hasAttribute('data-dkm-nav')) { self.loadMomentAt(b.getAttribute('data-dkm-nav'), function () { self.focusMoment(); }); return; }
      if (b.hasAttribute('data-tt-moment')) { self.openMoment(b.getAttribute('data-tt-moment'), {}); }
    });
    // A press outside puts the notice away, and the count stays.
    this.on(document, 'pointerdown', function (ev) {
      var t = ev.target;
      if (!self.momOpen || self.sheet || ev.button !== 0 || !t || !t.closest) return;
      if (!t.closest('#tt-moment, [data-tt-tab], .tt-island, [data-dkh-over], [data-dkd-ov]')) self.closeMoment(false);
    }, true);
    this.on(W, 'resize', function () { var box = document.getElementById('tt-moment'); if (box) self.placeMoment(box); });
  };

  // ---------- Rolling ----------

  Table.prototype.roll = function (formula, label, characterId) {
    var self = this, err = document.getElementById('tt-ferr'), mode = document.getElementById('tt-mode');
    var body = { formula: formula, label: label || '', mode: mode ? mode.value : 'public' };
    if (characterId) body.characterId = characterId;
    C.apiFetch(this.base, { method: 'POST', body: body }).then(function (res) {
      return res.json().catch(function () { return {}; }).then(function (data) { return { ok: res.ok, data: data }; });
    }).then(function (out) {
      if (!out.ok) {
        if (err) { err.hidden = false; err.textContent = (out.data && (out.data.message || out.data.error)) || 'The roll didn’t go through. Try again.'; }
        return;
      }
      if (err) err.hidden = true;
      out.data.mine = true;
      self.receive(out.data, true);
    }).catch(function () {
      if (err) { err.hidden = false; err.textContent = 'The roll didn’t go through. Check your connection and try again.'; }
    });
  };

  // ---------- Events ----------

  Table.prototype.wire = function () {
    var self = this;
    this.on(this.tab, 'click', function () { if (self.open) self.closeTable(); else self.openTable(); });
    if (this.momentsOn) this.wireMoments();

    this.on(document, 'click', function (ev) {
      var t = ev.target;
      if (!t.closest('.tt-ltabs')) self.closeMenus();
      var b = t.closest('button');
      if (!b || !(b.closest('#tt-drawer') || b.closest('#tt-isl-detail'))) return;
      var d = b.dataset;
      if (b.hasAttribute('data-tt-close')) { self.closeTable(); return; }
      if (d.ttCut) { self.watchCut(d.ttCut, b); return; }
      // A press on the line is handled by its pointer events; a click with no
      // pointer behind it is the keyboard.
      if (b.hasAttribute('data-tt-grab')) { if (!ev.detail) self.stepSheet(); return; }
      if (d.ttPick !== undefined) {
        var m = document.getElementById('tt-lm-' + d.ttPick), opening = m.hidden;
        self.closeMenus();
        if (opening) {
          m.hidden = false;
          b.setAttribute('aria-expanded', 'true');
          var f = m.querySelector('[aria-checked="true"]'); if (f) f.focus();
        }
        return;
      }
      if (d.ttSec) { self.closeMenus(); self.setLeaf(+d.ttLeaf, d.ttSec); return; }
      if (d.ttPhone !== undefined) {
        self.phoneLeaf = +d.ttPhone;
        var keep = self.keepForm();
        self.drawer().innerHTML = self.drawerHTML();
        self.restoreForm(keep);
        // The names also stand in the lifted sheet's top row: a tap on one
        // raises the sheet to full height on that leaf.
        if (self.sheet && self.h === 'lift') self.setHeight('full');
        self.scrollLog();
        self.seen();
        if (!ev.detail) {
          var again = document.querySelector('[data-tt-phone="' + d.ttPhone + '"]');
          if (again) again.focus({ preventScroll: true });
        }
        return;
      }
      if (b.hasAttribute('data-tt-older')) { self.older(); return; }
      if (d.ttRow) { self.toggleRow(b, d.ttRow); return; }
      if (d.ttQuick) { self.roll('1' + d.ttQuick, ''); return; }
      if (d.ttCrits) {
        self.crits = d.ttCrits !== 'off';
        store('crits.' + self.key, self.crits ? 'on' : 'off');
        self.applyDice();
        self.renderSection('dice');
        return;
      }
      if (self.power && d.ttPstat !== undefined) {
        var st = self.power.characteristics.filter(function (c) { return c.key === d.ttPstat; })[0];
        if (st) self.powerRoll(st.label, st.value == null ? 0 : num(st.value), st.key, st.value == null ? '' : self.power.heroId);
        return;
      }
      if (self.power && b.hasAttribute('data-tt-pplain')) {
        var bi = document.getElementById('tt-pbonus'), bv = Math.max(-9, Math.min(20, parseInt(bi && bi.value, 10) || 0));
        store('pbonus.' + self.key, String(bv));
        self.powerRoll(C.powerRoll.PLAIN, bv, '');
        return;
      }
      if (d.ttPstep !== undefined) { self.setStep(+d.ttPstep); return; }
      if (b.hasAttribute('data-tt-other')) {
        self.otherDice = !self.otherDice;
        b.setAttribute('aria-expanded', String(self.otherDice));
        var od = document.getElementById('tt-otherdice'); if (od) od.hidden = !self.otherDice;
        return;
      }
      if (d.ttLook) { self.look = d.ttLook; store('look.' + self.key, self.look); self.renderSection('dice'); return; }
      if (b.hasAttribute('data-tt-picks-reset')) {
        var before = readPicks(JSON.stringify(self.personal));
        self.personal = readPicks('');
        self.savePicks(null, before);
        self.renderSection('dice');
        return;
      }
      if (b.hasAttribute('data-tt-look-reset')) { self.look = ''; store('look.' + self.key, ''); self.renderSection('dice'); return; }
      if (d.ttTablelook) { self.setTableLook(d.ttTablelook); return; }
      if (b.hasAttribute('data-tt-try')) {
        self.throwDice({ dice: QUICK.map(function (s) { return { sides: s, value: 1 + Math.floor(Math.random() * s) }; }), mine: true }, document.getElementById('tt-ptray'));
        return;
      }
      if (b.hasAttribute('data-tt-iclose')) { if (self.isl) self.collapse(self.isl); return; }
      if (b.hasAttribute('data-tt-totonight')) {
        if (self.isl) self.collapse(self.isl);
        self.openTable('tonight');
        return;
      }
      if (b.hasAttribute('data-tt-tolog')) {
        var me = self.isl;
        if (me) { self.openRow = me.r.id; self.collapse(me); }
        self.openTable('rolls');
      }
    });

    // The drawer and a roll's details are pull-downs: a press anywhere off
    // them closes them, losing nothing. Pointerdown, not click, because a
    // click inside redraws the drawer and its target is gone by then.
    this.on(document, 'pointerdown', function (ev) {
      var t = ev.target;
      if (ev.button !== 0 || !t || !t.closest) return;
      if (self.isl && self.isl.expanded && !t.closest('#tt-isl-detail, .tt-island')) self.collapse(self.isl);
      if (self.open && !t.closest('#tt-drawer, [data-tt-tab], .tt-island, #tt-isl-detail, [data-dkh-over], [data-dkd-ov]')) self.closeTable(false);
    }, true);

    // A pick of your own rune dice: the die, its colour, or a crit animation.
    this.on(document, 'change', function (ev) {
      var t = ev.target, id = t && t.getAttribute && t.getAttribute('data-tt-dpick');
      if (!id || !(id in self.personal)) return;
      var was = readPicks(JSON.stringify(self.personal));
      self.personal[id] = /^[a-z0-9-]{0,24}$/.test(t.value) ? t.value : '';
      // A colour belongs to its die.
      if (id === 'set') self.personal.colour = '';
      self.savePicks(null, was);
      self.renderSection('dice');
    });

    this.on(document, 'submit', function (ev) {
      if (!ev.target.matches || !ev.target.matches('[data-tt-form]')) return;
      ev.preventDefault();
      var fx = document.getElementById('tt-fx'), err = document.getElementById('tt-ferr'), f = fx.value.trim();
      if (!/^[+]?(\d*d\d+|\d+)([+-](\d*d\d+|\d+))*$/i.test(f.replace(/\s+/g, '')) || !/d/i.test(f)) {
        err.hidden = false;
        err.textContent = f ? 'Write the dice like 2d10+2, d20 or 4d6.' : 'Type the dice to roll, like 2d10+2.';
        fx.focus();
        return;
      }
      err.hidden = true;
      self.roll(f, '');
    });

    this.on(document, 'keydown', function (ev) {
      var open = $('.tt-lmenu:not([hidden])');
      if (open && (ev.key === 'ArrowDown' || ev.key === 'ArrowUp' || ev.key === 'Home' || ev.key === 'End')) {
        var items = $$('button', open), at = items.indexOf(document.activeElement);
        var to = ev.key === 'Home' ? 0 : ev.key === 'End' ? items.length - 1 : (at + (ev.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
        ev.preventDefault();
        items[to].focus();
        return;
      }
      if (ev.key === 'Escape') {
        if (ev.defaultPrevented) return;
        var menu = open;
        if (menu) { var op = $('.tt-lpick[aria-expanded="true"]'); self.closeMenus(); if (op) op.focus(); return; }
        if (self.isl && self.isl.expanded) { self.collapse(self.isl); return; }
        var mb = document.getElementById('tt-moment');
        if (self.momOpen && mb && mb.contains(document.activeElement)) { self.closeMoment(); return; }
        if (self.open) { self.closeTable(); }
        return;
      }
      // A surface that covers the window stands the page's shortcuts down, so
      // the drawer never opens where nobody can see it.
      if (typing(ev.target) || ev.ctrlKey || ev.metaKey || ev.altKey || ev.defaultPrevented || document.documentElement.hasAttribute('data-full-surface')) return;
      var k = (ev.key || '').toLowerCase();
      if (k === 't') { ev.preventDefault(); if (self.open) self.closeTable(); else self.openTable(); }
      else if (k === 'r') { ev.preventDefault(); self.openTable('rolls'); var fx = document.getElementById('tt-fx'); if (fx) fx.focus(); }
    });

    this.on(W, 'resize', function () { if ($('.tt-isl-clip', self.header)) self.clip(); });

    // With a text field on the page focused and the keyboard up, the sheet
    // steps aside and comes back after.
    this.on(document, 'focusin', function (ev) {
      var d = self.drawer();
      if (!self.sheet || !d || !typing(ev.target) || d.contains(ev.target) || !W.matchMedia('(pointer: coarse)').matches) return;
      d.classList.add('tt-away');
    });
    this.on(document, 'focusout', function () {
      setTimeout(function () {
        var d = self.drawer();
        if (d && !typing(document.activeElement)) d.classList.remove('tt-away');
      }, 0);
    });
  };

  Table.prototype.toggleRow = function (b, id) {
    var det = document.getElementById('tt-det-' + id), open = this.openRow === id;
    $$('.tt-row[aria-expanded="true"]').forEach(function (x) {
      x.setAttribute('aria-expanded', 'false');
      var dd = document.getElementById('tt-det-' + x.getAttribute('data-tt-row')); if (dd) dd.hidden = true;
    });
    this.openRow = open ? '' : id;
    if (!open && det) {
      det.hidden = false;
      b.setAttribute('aria-expanded', 'true');
      if (motion() === 'full' && det.animate) det.animate([{ opacity: 0, transform: 'translateY(-4px)' }, { opacity: 1, transform: 'none' }], { duration: tokMs('--dur-standard', 200), easing: 'cubic-bezier(0.16,1,0.3,1)' });
    }
  };

  Table.prototype.setTableLook = function (look) {
    var self = this, was = this.tableLook;
    this.tableLook = look;
    this.renderSection('dice');
    C.apiFetch(this.base + '/settings', { method: 'PUT', body: { look: look } }).then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
    }).catch(function () {
      self.tableLook = was;
      self.renderSection('dice');
      if (C.notify) C.notify('The table’s dice weren’t saved. Try again.', 'error');
    });
  };

  // ---------- The draw overlay (Card decks) ----------

  // deck_draw.js, its stylesheet and the card motion script load on the first
  // use: a press on a Draw button, the DM Screen's Card decks section, or a
  // card dealt to this player. A page that never needs them never loads them.
  var drawLoad = null;
  function loadScript(src) {
    return new Promise(function (resolve, reject) {
      var s = document.createElement('script');
      s.src = src;
      s.onload = resolve;
      s.onerror = function () { reject(new Error('script')); };
      document.head.appendChild(s);
    });
  }
  // Only the version token is read from the page; each path is fixed here, so
  // a changed attribute can never point the loader at another script.
  var DRAW_ASSETS = {
    'data-dkd-src': '/static/plugins/decks/js/deck_draw.js',
    'data-dkd-cards-src': '/static/plugins/decks/js/deck_cards.js',
    'data-dkd-css': '/static/css/decks_draw.css',
    'data-dkd-card-css': '/static/css/decks_card.css'
  };
  C.drawAsset = function (holder, attr) {
    var v = holder && holder.getAttribute(attr);
    if (!v) return '';
    var m = /\?v=([A-Za-z0-9._-]{1,80})$/.exec(v);
    return DRAW_ASSETS[attr] + (m ? '?v=' + encodeURIComponent(m[1]) : '');
  };
  C.loadDeckDraw = function () {
    if (W.DeckDraw) return Promise.resolve(W.DeckDraw);
    if (drawLoad) return drawLoad;
    var holder = document.querySelector('[data-dkd-src]');
    var src = C.drawAsset(holder, 'data-dkd-src');
    if (!src) return Promise.reject(new Error('no source'));
    // The overlay's own sheet, and the card sheet a page without cards lacks.
    ['data-dkd-css', 'data-dkd-card-css'].forEach(function (attr) {
      var href = C.drawAsset(holder, attr);
      if (!href || document.querySelector('link[' + attr + '], link[href^="' + DRAW_ASSETS[attr] + '"]')) return;
      var link = document.createElement('link');
      link.rel = 'stylesheet';
      link.href = href;
      link.setAttribute(attr, '');
      document.head.appendChild(link);
    });
    var cards = W.DeckCards ? Promise.resolve() : loadScript(C.drawAsset(holder, 'data-dkd-cards-src') || DRAW_ASSETS['data-dkd-cards-src']);
    drawLoad = cards.then(function () { return loadScript(src); }).then(function () { return W.DeckDraw; }, function (e) {
      drawLoad = null;
      throw e;
    });
    return drawLoad;
  };

  // The DM Screen's Card decks section mounts itself once the script is there;
  // the script is fetched when the section first appears on the page.
  Table.prototype.watchDraw = function () {
    var self = this;
    if (!this.decks) return;
    if (this.director && W.MutationObserver) {
      var done = false, queued = false, mo;
      var look = function () {
        queued = false;
        if (done || !self.alive || !document.querySelector('[data-dkd-screen]')) return;
        done = true;
        if (mo) mo.disconnect();
        C.loadDeckDraw().then(function (d) { d.scan(); }, function () { /* the section stays as the server drew it */ });
      };
      mo = new W.MutationObserver(function () { if (!queued) { queued = true; (W.requestAnimationFrame || W.setTimeout)(look); } });
      mo.observe(document.documentElement, { childList: true, subtree: true });
      this.drawWatch = mo;
      look();
    }
    if (!this.director) {
      this.freshShowing = false;
      this.freshSeen = {};
      // Any deck move in the campaign says the decks changed, so a burst of
      // moves asks once.
      var freshTimer = 0;
      this.on(W, 'chronicle:decks-changed', function () {
        clearTimeout(freshTimer);
        freshTimer = setTimeout(function () { self.askFresh(); }, 250);
      });
      // A card that waited behind another overlay comes once that one closes.
      this.on(W, 'chronicle:deck-draw-closed', function () { self.askFresh(); });
      this.askFresh();
    }
  };

  // A card a Director dealt to one of this player's heroes pops up once. The
  // server says which are new; the script is loaded only when there is one.
  Table.prototype.askFresh = function () {
    var self = this;
    if (!this.alive || this.freshShowing) return;
    var base = '/campaigns/' + encodeURIComponent(this.campaignId) + '/decks';
    C.apiFetch(base + '/hand/fresh').then(function (res) { return res.ok ? res.json() : null; }).then(function (data) {
      var cards = ((data && data.cards) || []).filter(function (c) { return !self.freshSeen[c.copyId]; });
      var card = cards[0];
      if (!card || !self.alive || self.freshShowing) return;
      self.freshShowing = true;
      return C.loadDeckDraw().then(function (d) {
        return d.arrive({ campaignId: self.campaignId, card: card, onClose: function () { self.freshShowing = false; self.askFresh(); } });
      }).then(function (shown) {
        // Several dealt at once pop up once, as the newest: the server marks
        // the older ones seen with it, and they wait in the Hand.
        if (!shown) { self.freshShowing = false; return; }
        cards.forEach(function (c) { self.freshSeen[c.copyId] = true; });
      });
    }).catch(function () { self.freshShowing = false; });
  };

  C.register('table', {
    init: function (el) {
      if (el._table || !el.getAttribute('data-campaign-id') || !C.tableDice) return;
      var css = el.getAttribute('data-css');
      if (css && !document.querySelector('link[data-tt-css]')) {
        var link = document.createElement('link');
        link.rel = 'stylesheet';
        link.href = css;
        link.setAttribute('data-tt-css', '');
        document.head.appendChild(link);
      }
      el._table = new Table(el);
    },
    destroy: function (el) {
      if (el._table) { el._table.destroy(); el._table = null; }
    }
  });
})();
