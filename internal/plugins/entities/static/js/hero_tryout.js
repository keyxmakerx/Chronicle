/**
 * hero_tryout.js — a try-out kept in this browser, on the Characters page.
 *
 * A signed-in visitor on a look-around pass builds one practice hero that is
 * kept in localStorage and never sent to the server. This widget is the
 * page's three places for it, told apart by data-mode:
 *   visitor  the visitor's Practice section: the stored hero as one row, with
 *            "Change it" and "Throw it away", or a way in when there is none;
 *   offer    after the owner makes them a player: "You built <name> while you
 *            were looking around", with the one button that sends it;
 *   shelf    a player's Practice shelf: the stored hero as its last row, so a
 *            try-out that was not sent is never out of sight.
 *
 * What it reads came from storage, so it is only ever written with textContent
 * and never as markup. It sends nothing until the player presses the button,
 * and then only to the routes a player already has: the hero creator's save
 * (which checks every pick against the player's own plan) or, for a player who
 * already has a hero, stage one's practice save. When the server refuses, the
 * try-out stays and the creator opens with it filled in, by writing the
 * creator's own draft key, with the server's sentence as a notice.
 *
 * Loaded on sight (ADR-063) from the body-script registry, so it survives a
 * boosted swap of the page; the mounts are re-found after each swap.
 */
(function () {
  'use strict';

  var MAX_NAME = 200;

  function el(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  }

  function go(url) {
    if (window.Chronicle && Chronicle.go) Chronicle.go(url); else window.location.href = url;
  }

  // The stored hero, cleaned: only a name, picks of {name, options}, and a
  // summary line survive, whatever else is in storage.
  function clean(raw) {
    if (!raw || typeof raw !== 'object' || raw.v !== 1) return null;
    var name = typeof raw.name === 'string' ? raw.name.trim().slice(0, MAX_NAME) : '';
    if (!name || !raw.picks || typeof raw.picks !== 'object' || Array.isArray(raw.picks)) return null;
    var picks = {};
    Object.keys(raw.picks).forEach(function (k) {
      var p = raw.picks[k];
      if (!p || typeof p.name !== 'string' || !p.name.trim()) return;
      picks[k] = { name: p.name };
      if (Array.isArray(p.options)) picks[k].options = p.options.filter(function (o) { return typeof o === 'string'; });
    });
    return { name: name, picks: picks, summary: typeof raw.summary === 'string' ? raw.summary.slice(0, 400) : '' };
  }

  function Tryout(root) {
    this.root = root;
    this.mode = root.getAttribute('data-mode');
    this.campaign = root.getAttribute('data-campaign-id') || '';
    this.user = root.getAttribute('data-user-id') || '';
    this.base = root.getAttribute('data-base') || '';
    this.csrf = root.getAttribute('data-csrf') || '';
    this.hasHero = root.getAttribute('data-has-hero') === 'true';
    this.key = 'chronicle:hero-tryout:' + this.campaign + ':' + this.user;
    this.hiddenKey = 'chronicle:hero-tryout-hidden:' + this.campaign + ':' + this.user;
    this.confirming = false;
    this.busy = false;
    this.note = '';
  }

  Tryout.prototype.read = function () {
    try { return clean(JSON.parse(window.localStorage.getItem(this.key) || 'null')); } catch (e) { return null; }
  };
  Tryout.prototype.forget = function () {
    try { window.localStorage.removeItem(this.key); } catch (e) { /* nothing to remove */ }
  };
  Tryout.prototype.offerHidden = function () {
    try { return window.sessionStorage.getItem(this.hiddenKey) === '1'; } catch (e) { return false; }
  };

  // Every mount reads the same key, so a change in one is drawn in all.
  function refreshAll() {
    var all = document.querySelectorAll('[data-widget="hero-tryout"]');
    for (var i = 0; i < all.length; i++) if (all[i]._heroTryout) all[i]._heroTryout.render();
  }

  Tryout.prototype.render = function () {
    var rec = this.read();
    if (this.mode === 'visitor') this.renderVisitor(rec);
    else if (this.mode === 'shelf') this.renderShelf(rec);
    else this.renderOffer(rec);
  };

  // --- Rows ---

  // The hero as a hairline row: name, summary, and the small-caps mark at the
  // end. markText is "Practice" for a visitor and "On this device" on a shelf.
  Tryout.prototype.rowHTML = function (rec, markText) {
    var line = el('div', 'pr-line');
    var main = el('div', 'pr-main');
    main.appendChild(el('b', null, rec.name));
    if (rec.summary) main.appendChild(el('small', null, rec.summary));
    line.appendChild(main);
    line.appendChild(el('span', 'pr-mark', markText));
    var li = el('li', 'pr-row pr-row--stack');
    li.appendChild(line);
    var ul = el('ul', 'pr-list pr-list--device');
    ul.appendChild(li);
    return { ul: ul, li: li };
  };

  // "Throw it away", asking first in place: a try-out cannot be brought back.
  Tryout.prototype.awayWords = function (rec, host, withChange) {
    var self = this;
    var words = el('div', 'pr-words');
    if (withChange) {
      var change = el('a', 'pr-word', 'Change it');
      change.setAttribute('href', this.base + '/characters/try');
      words.appendChild(change);
    }
    var away = el('button', 'pr-word', 'Throw it away');
    away.type = 'button';
    away.setAttribute('aria-expanded', 'false');
    words.appendChild(away);

    var box = el('span', 'pr-confirm');
    box.hidden = true;
    box.appendChild(el('span', null, 'Throw away ' + rec.name + '? It can’t be brought back.'));
    var yes = el('button', 'btn-danger btn-sm', 'Throw it away');
    yes.type = 'button';
    var keep = el('button', 'pr-word', 'Keep it');
    keep.type = 'button';
    box.appendChild(yes);
    box.appendChild(keep);

    function toggle(open) {
      box.hidden = !open;
      away.hidden = open;
      away.setAttribute('aria-expanded', open ? 'true' : 'false');
      (open ? yes : away).focus();
    }
    away.addEventListener('click', function () { toggle(true); });
    keep.addEventListener('click', function () { toggle(false); });
    yes.addEventListener('click', function () {
      self.forget();
      refreshAll();
      // The row is gone, so keyboard focus goes to what replaced it.
      var next = self.root.hidden ? null : self.root.querySelector('a, button');
      if (next) next.focus();
    });
    words.appendChild(box);
    host.appendChild(words);
  };

  Tryout.prototype.renderVisitor = function (rec) {
    var slot = this.root.querySelector('[data-tryout-slot]');
    if (!slot) return;
    // With nothing stored the server's own words (the way in) stay as drawn.
    if (!rec) {
      if (slot._tryoutRow) { slot._tryoutRow = false; slot.textContent = ''; slot.appendChild(slot._tryoutWay); }
      return;
    }
    if (!slot._tryoutRow) slot._tryoutWay = slot.firstElementChild;
    slot._tryoutRow = true;
    slot.textContent = '';
    var row = this.rowHTML(rec, 'Practice');
    slot.appendChild(row.ul);
    this.awayWords(rec, row.li, true);
  };

  Tryout.prototype.renderShelf = function (rec) {
    this.root.textContent = '';
    this.root.hidden = !rec;
    if (!rec) return;
    var row = this.rowHTML(rec, 'On this device');
    this.root.appendChild(row.ul);
    this.awayWords(rec, row.li, false);
  };

  // --- The offer ---

  Tryout.prototype.renderOffer = function (rec) {
    var self = this, root = this.root;
    root.textContent = '';
    var offered = rec && !this.offerHidden();
    // A notice outlives the try-out it is about: once the hero is made the
    // try-out is gone, and the sentence about a pick that could not be sent
    // is still what the player needs to read.
    if (!offered && !this.note) { root.hidden = true; return; }
    root.hidden = false;

    if (offered) {
      var p = el('p', 'pr-offer__line');
      p.appendChild(document.createTextNode('You built '));
      p.appendChild(el('b', null, rec.name));
      p.appendChild(document.createTextNode(' while you were looking around.'));
      root.appendChild(p);

      var acts = el('div', 'pr-acts');
      var label = this.hasHero ? 'Keep ' + rec.name + ' as a practice hero' : 'Make ' + rec.name + ' my hero';
      var make = el('button', 'btn-primary', label);
      make.type = 'button';
      make.disabled = this.busy;
      make.addEventListener('click', function () { self.make(rec); });
      var later = el('button', 'pr-word', 'Not now');
      later.type = 'button';
      later.addEventListener('click', function () {
        try { window.sessionStorage.setItem(self.hiddenKey, '1'); } catch (e) { /* shown again next time */ }
        self.note = '';
        refreshAll();
      });
      acts.appendChild(make);
      acts.appendChild(later);
      root.appendChild(acts);
    }

    if (this.note) {
      var n = el('p', 'pr-note', this.note);
      n.setAttribute('role', 'alert');
      root.appendChild(n);
      if (this.noteLink) {
        var a = el('a', 'pr-word', 'Open the hero');
        a.setAttribute('href', this.noteLink);
        root.appendChild(a);
      }
    }
  };

  function fetchJSON(url, body, csrf) {
    var send = window.Chronicle && Chronicle.apiFetch
      ? Chronicle.apiFetch(url, { method: 'POST', body: body, csrfToken: csrf })
      : fetch(url, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf }, body: JSON.stringify(body) });
    return send.then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (b) { return { ok: r.ok, status: r.status, body: b || {} }; });
    });
  }

  // Sends the stored hero the way the creator does. It is the one request this
  // widget makes, and only on the player's press.
  Tryout.prototype.make = function (rec) {
    var self = this;
    if (this.busy) return;
    this.busy = true;
    this.note = '';
    this.noteLink = '';
    this.render();
    var url = this.base + (this.hasHero ? '/characters/practice' : '/characters/new');
    fetchJSON(url, { name: rec.name, picks: rec.picks }, this.csrf).then(function (res) {
      self.busy = false;
      if (res.ok && res.body.url) { self.sent(res.body); return; }
      self.refused(rec, res.body.message || res.body.error || '');
    }).catch(function () {
      self.busy = false;
      self.note = 'That could not be sent. Check your connection and try again. Your hero is still kept on this device.';
      self.render();
    });
  };

  Tryout.prototype.sent = function (body) {
    // The hero exists now, so the try-out has done its job.
    this.forget();
    var failed = body.requestFailed || [];
    if (!this.hasHero && failed.length) {
      // Same words as the creator: the hero is made, a pick for the Director
      // could not be sent, and the way on is the hero's own page.
      this.note = 'Your hero is made, but a pick for ' + failed.map(function (k) { return String(k).replace(/[_-]+/g, ' '); }).join(', ') +
        ' could not be sent to your Director. Open the hero and pick it again.';
      this.noteLink = body.url;
      this.render();
      return;
    }
    go(body.url);
  };

  // The server turned it down: a pick that is not on this player's list, a
  // full shelf, a hero already made. Nothing is lost. The creator opens with
  // the try-out in it and the server's sentence above the steps.
  Tryout.prototype.refused = function (rec, message) {
    var draft = this.hasHero ? 'chronicle:hero-practice:' : 'chronicle:hero-draft:';
    try {
      window.localStorage.setItem(draft + this.campaign, JSON.stringify({ picks: rec.picks, name: rec.name }));
      window.sessionStorage.setItem('chronicle:hero-handoff:' + this.campaign, JSON.stringify({
        message: message || 'That didn’t go through. Your hero is filled in here so you can look at it.'
      }));
    } catch (e) { /* the creator opens blank, and the try-out is still kept */ }
    go(this.base + (this.hasHero ? '/characters/practice/new' : '/characters/new'));
  };

  // --- Mounting ---

  function register() {
    Chronicle.register('hero-tryout', {
      init: function (root) {
        if (root._heroTryout) return;
        var t = new Tryout(root);
        root._heroTryout = t;
        t.render();
      }
    });
  }
  if (window.Chronicle && Chronicle.register) register();
})();
