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
 * on the table's choice).
 */
(function () {
  'use strict';

  var W = window;
  var C = W.Chronicle;

  // The sections a leaf can show. Rules, Conditions, Party and Notes join
  // this list as they are built (TODO(#1475)).
  var SECTIONS = [['rolls', 'Rolls'], ['dice', 'My dice']];
  var SECTION_NAME = { rolls: 'Rolls', dice: 'My dice' };
  var LOOKS = [['quick', 'Quick 2D'], ['rune', 'Rune 3D'], ['none', 'Numbers only']];
  var QUICK = [4, 6, 8, 10, 12, 20];
  // Rune dice come in these sizes; any other die plays as quick 2D.
  var RUNE_SIDES = { 4: 1, 6: 1, 8: 1, 10: 1, 12: 1, 20: 1 };
  // How many rolls the log holds in the page before older ones are fetched.
  var PAGE = 50;
  // How long the island stays out, in ms.
  var ISLAND_STAY = 3400;

  // Dice figures go into markup unescaped, so anything that isn't a whole
  // number becomes 0 rather than trusting the payload's shape.
  function num(v) { var n = Number(v); return isFinite(n) ? Math.trunc(n) : 0; }
  function esc(s) { return C.escapeHtml ? C.escapeHtml(s == null ? '' : String(s)) : String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
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
  // A single-key shortcut never fires while typing.
  function typing(t) {
    return !!(t && t.closest && t.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"]'));
  }

  var X = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg>';
  var CHEV = '<svg class="tt-more" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>';

  // ---------- One table per page ----------

  function Table(host) {
    this.host = host;
    this.campaignId = host.getAttribute('data-campaign-id');
    this.userId = host.getAttribute('data-user-id');
    this.director = host.getAttribute('data-director') === 'true';
    this.gm = host.getAttribute('data-gm-term') || 'Director';
    this.tab = $('[data-tt-tab]', host);
    this.count = $('[data-tt-count]', host);
    this.header = host.closest('header') || host.parentNode;
    this.base = '/campaigns/' + encodeURIComponent(this.campaignId) + '/rolls';
    var key = this.campaignId + '.' + this.userId;
    this.key = key;
    this.leaves = this.readLeaves(store('leaves.' + key));
    this.phoneLeaf = 0;
    this.look = store('look.' + key) || '';
    this.tableLook = 'quick';
    this.rolls = [];
    this.byId = {};
    this.more = false;
    this.loaded = false;
    this.open = false;
    this.openRow = '';
    this.unseen = 0;
    this.isl = null;
    this.queue = [];
    this.ws = null;
    this.wsDelay = 0;
    this.alive = true;
    this.handlers = [];
    this.wire();
    this.connect();
    this.loadSettings();
  }

  // A remembered layout keeps only sections that exist, each once.
  Table.prototype.readLeaves = function (raw) {
    var out = [];
    (raw || '').split(',').forEach(function (s) { if (SECTION_NAME[s] && out.indexOf(s) < 0) out.push(s); });
    SECTIONS.forEach(function (s) { if (out.indexOf(s[0]) < 0) out.push(s[0]); });
    return out.slice(0, Math.min(3, SECTIONS.length));
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
    var c = $('.tt-isl-clip', this.header); if (c) c.remove();
    var t = $('#tt-tray'); if (t) t.remove();
    var det = $('#tt-isl-detail'); if (det) det.remove();
  };

  // The dice this person sees: their own pick, else the table's.
  Table.prototype.myLook = function () { return this.look || this.tableLook || 'quick'; };

  Table.prototype.loadSettings = function () {
    var self = this;
    C.apiFetch(this.base + '/settings').then(function (r) { return r.ok ? r.json() : null; }).then(function (s) {
      if (s && s.look) self.tableLook = s.look;
      if (self.open) self.renderSection('dice');
    }).catch(function () { /* the table's choice stays quick 2D */ });
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
    });
    sock.addEventListener('message', function (ev) {
      var msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      if (msg && msg.type === 'roll.made' && msg.payload) self.receive(msg.payload, true);
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
    }).catch(function () {
      self.loadError = 'The roll log could not be read. Close the Table and open it again to retry.';
      self.refreshLog();
    });
  };

  Table.prototype.reload = function () { if (this.open) this.load(); else this.loaded = false; };

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
    }
    if (r.why) rows += '<dt>Why</dt><dd>' + esc(r.why) + '</dd>';
    rows += '<dt>From</dt><dd>' + esc(fromWhere(r)) + (r.origin === 'foundry' ? ' <span class="tt-chip">Foundry</span>' : '') + '</dd>';
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
      '<span class="tt-tot' + (r.hidden ? ' tt-hid' : '') + '">' + (r.hidden ? '?' : esc(r.total)) + '</span>' +
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

  Table.prototype.rollerHTML = function () {
    var gm = esc(this.gm);
    var modes = this.director ?
      [['public', 'Everyone sees it'], ['gm', gm + 's only'], ['self', 'Just me']] :
      [['public', 'Everyone sees it'], ['gm', 'The ' + gm + ' and me'], ['blind', 'Blind: only the ' + gm], ['self', 'Just me']];
    var buttons = QUICK.map(function (s) {
      return '<button type="button" class="tt-dbtn" data-tt-quick="d' + s + '" aria-label="Roll a d' + s + '"><span class="tt-mini" aria-hidden="true">' + C.tableDice.svg(s) + '</span>d' + s + '</button>';
    }).join('');
    return '<form class="tt-roller" data-tt-form novalidate><div class="tt-dice">' + buttons + '</div>' +
      '<div class="tt-crow"><label class="tt-sr" for="tt-fx">Dice to roll</label><input class="tt-field" id="tt-fx" autocomplete="off" maxlength="120" placeholder="2d10+2, d20, 4d6">' +
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
    if (this.director) {
      out += '<div><p class="tt-k">The table’s dice</p><p class="tt-from">What a player who hasn’t picked starts on.</p></div>' +
        '<div class="tt-seg" role="group" aria-label="The table’s dice">' + seg('data-tt-tablelook', self.tableLook) + '</div>';
    }
    out += '<div class="tt-ptray" id="tt-ptray"></div><button type="button" class="tt-quiet" data-tt-try>Throw a test set</button></div>';
    return out;
  };

  function lookName(k) { var l = LOOKS.filter(function (x) { return x[0] === k; })[0]; return l ? l[1] : k; }

  Table.prototype.sectionHTML = function (id) {
    if (id === 'rolls') return this.logHTML() + this.rollerHTML();
    return this.diceHTML();
  };

  Table.prototype.leafHTML = function (i) {
    var cur = this.leaves[i], self = this;
    var menu = SECTIONS.map(function (s) {
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
    return '<div class="tt-dhead"><h2>Table</h2><button type="button" class="tt-icon" data-tt-close aria-label="Close the Table">' + X + '</button></div>' + sw +
      '<div class="tt-leaves" style="--tt-n:' + this.leaves.length + '">' + this.leaves.map(function (s, i) { return self.leafHTML(i); }).join('') + '</div>';
  };

  // ---------- Opening, closing, leaves ----------

  Table.prototype.drawer = function () { return document.getElementById('tt-drawer'); };

  Table.prototype.openTable = function (sec) {
    if (sec) {
      var i = this.leaves.indexOf(sec);
      if (i < 0) { i = this.leaves.length - 1; this.leaves[i] = sec; this.saveLeaves(); }
      this.phoneLeaf = i;
    }
    var was = this.open;
    this.open = true;
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
    if (!was && motion() !== 'off') {
      d.classList.add('tt-opening');
      d.addEventListener('animationend', function () { d.classList.remove('tt-opening'); }, { once: true });
    }
    if (!this.loaded) this.load();
    this.scrollLog();
    this.seen();
  };

  Table.prototype.closeTable = function () {
    if (!this.open) return;
    var self = this, d = this.drawer();
    var finish = function () {
      if (self.open) return;
      if (d) d.remove();
    };
    this.open = false;
    this.closeMenus();
    this.tab.setAttribute('aria-expanded', 'false');
    if (d && motion() !== 'off') { d.classList.add('tt-closing'); setTimeout(finish, 120); } else finish();
    this.tab.focus();
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
    return W.matchMedia('(min-width: 761px)').matches || this.phoneLeaf === i;
  };

  Table.prototype.seen = function () {
    if (!this.logVisible()) return;
    this.unseen = 0;
    this.count.hidden = true;
  };

  Table.prototype.bump = function () {
    if (this.logVisible()) return;
    this.unseen++;
    var c = this.count;
    c.hidden = false;
    c.textContent = this.unseen > 99 ? '99+' : String(this.unseen);
    c.classList.remove('tt-grow');
    void c.offsetWidth;
    c.classList.add('tt-grow');
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
    if (rune && !tray) {
      return C.dice.roll([{ label: r.label || r.formula, dice: dice.map(function (d) { return { sides: d.sides, value: d.value }; }) }], {
        describe: function () { return ''; }
      }).then(function () {}, function () {});
    }
    return C.tableDice.throwInto(tray || this.tray(), dice, { fromLeft: !!r.mine });
  };

  Table.prototype.play = function (r) {
    var self = this;
    this.throwDice(r).then(function () {
      if (!self.alive) return;
      if (self.open) self.refreshLog();
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
    var who = r.hidden ? 'You rolled blind' : this.who(r) + (r.origin === 'foundry' ? ' in Foundry' : '');
    var num = r.hidden ? '?' : String(r.total);
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
    d.innerHTML = '<div class="tt-dhd"><b>' + esc(r.label || r.formula) + '</b><span class="tt-bignum">' + (r.hidden ? '?' : esc(r.total)) + '</span></div>' + this.detailHTML(r) +
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

  // ---------- Rolling ----------

  Table.prototype.roll = function (formula, label) {
    var self = this, err = document.getElementById('tt-ferr'), mode = document.getElementById('tt-mode');
    var body = { formula: formula, label: label || '', mode: mode ? mode.value : 'public' };
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

    this.on(document, 'click', function (ev) {
      var t = ev.target;
      if (!t.closest('.tt-ltabs')) self.closeMenus();
      var b = t.closest('button');
      if (!b || !(b.closest('#tt-drawer') || b.closest('#tt-isl-detail'))) return;
      var d = b.dataset;
      if (b.hasAttribute('data-tt-close')) { self.closeTable(); return; }
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
        self.scrollLog();
        self.seen();
        return;
      }
      if (b.hasAttribute('data-tt-older')) { self.older(); return; }
      if (d.ttRow) { self.toggleRow(b, d.ttRow); return; }
      if (d.ttQuick) { self.roll('1' + d.ttQuick, ''); return; }
      if (d.ttLook) { self.look = d.ttLook; store('look.' + self.key, self.look); self.renderSection('dice'); return; }
      if (b.hasAttribute('data-tt-look-reset')) { self.look = ''; store('look.' + self.key, ''); self.renderSection('dice'); return; }
      if (d.ttTablelook) { self.setTableLook(d.ttTablelook); return; }
      if (b.hasAttribute('data-tt-try')) {
        self.throwDice({ dice: QUICK.map(function (s) { return { sides: s, value: 1 + Math.floor(Math.random() * s) }; }), mine: true }, document.getElementById('tt-ptray'));
        return;
      }
      if (b.hasAttribute('data-tt-iclose')) { if (self.isl) self.collapse(self.isl); return; }
      if (b.hasAttribute('data-tt-tolog')) {
        var me = self.isl;
        if (me) { self.openRow = me.r.id; self.collapse(me); }
        self.openTable('rolls');
      }
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
      if (ev.key === 'Escape') {
        var menu = $('.tt-lmenu:not([hidden])');
        if (menu) { var op = $('.tt-lpick[aria-expanded="true"]'); self.closeMenus(); if (op) op.focus(); return; }
        if (self.isl && self.isl.expanded) { self.collapse(self.isl); return; }
        if (self.open) { self.closeTable(); }
        return;
      }
      if (typing(ev.target) || ev.ctrlKey || ev.metaKey || ev.altKey || ev.defaultPrevented) return;
      var k = (ev.key || '').toLowerCase();
      if (k === 't') { ev.preventDefault(); if (self.open) self.closeTable(); else self.openTable(); }
      else if (k === 'r') { ev.preventDefault(); self.openTable('rolls'); var fx = document.getElementById('tt-fx'); if (fx) fx.focus(); }
    });

    this.on(W, 'resize', function () { if ($('.tt-isl-clip', self.header)) self.clip(); });
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
      if (motion() === 'full' && det.animate) det.animate([{ opacity: 0, transform: 'translateY(-4px)' }, { opacity: 1, transform: 'none' }], { duration: 200, easing: 'cubic-bezier(0.16,1,0.3,1)' });
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
