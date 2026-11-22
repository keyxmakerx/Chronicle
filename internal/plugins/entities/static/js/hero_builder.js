/**
 * hero_builder.js — the hero builder on the Characters page.
 *
 * Each of the viewer's heroes reads as one sentence ("Tess's ancestry is
 * [Human], class is [ ] …"). A blank opens a panel docked beside the
 * sentence with every choice on that list, its rules and a roll range, so a
 * player reads before they pick and never leaves the page. A pick on an
 * existing hero saves at once through the fields door, with a green Saved
 * beside it. A new hero is made in the hero creator: the page's Create hero
 * is a plain link to it, and nothing here offers a sentence for one.
 *
 * The steps come from the creator plan (the same lists as the creator
 * page), so a campaign's own entries appear beside the game system's.
 * Campaign descriptions arrive as HTML the server sanitized; package text is
 * plain and escaped here.
 *
 * A Director's rules shape the lists: entries marked Not here are gone (a
 * quiet foot line names them), Ask first entries carry a mark, are skipped
 * by the dice, and when picked wait for the Director beside the blank while
 * the rest of the hero is filled in.
 *
 * A player a Director let write one entry of their own sees "Make your own"
 * at the end of that list, and the panel turns into the shared own-entry
 * editor (own_entry_editor.js, listed first): write it, save a draft or hand
 * it in, and it is picked for the hero. Their own entries are marked Draft or
 * Waiting for the Director and can be changed from the panel.
 *
 * A Director may set times when a player can change their picks (after
 * making the hero, after a new level). Outside them a hero carries a shut
 * lock: a line says when the picks open again, a step already chosen can be
 * read but not changed, and blanks can still be filled, since finishing a
 * hero is not changing it. The fields door enforces the same rule.
 *
 * Loaded on sight for data-widget="hero-builder" (ADR-063).
 */
(function () {
  'use strict';

  function esc(v) { return Chronicle.escapeHtml(v); }

  // The campaign's own word for its game master, already HTML-safe.
  function gm() { return Chronicle.gmTerm ? Chronicle.gmTerm() : 'GM'; }

  // Package prose: {@category term|shown} markers keep their shown text and
  // **bold** becomes bold. Escaped first, so nothing in it can become markup.
  function prose(s) {
    var t = esc(s).replace(/\{@[a-zA-Z]+\s+([^}|]*)(?:\|([^}]*))?\}/g, function (m, term, shown) { return shown || term; });
    t = t.replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>');
    return t.split(/\n{2,}/).map(function (p) { return '<p>' + p.replace(/\n/g, '<br>') + '</p>'; }).join('');
  }

  function words(key) {
    var s = String(key).replace(/_display$/, '').replace(/[_-]+/g, ' ').trim();
    return s.charAt(0).toUpperCase() + s.slice(1);
  }
  function lower(s) { return String(s || '').toLowerCase(); }
  function isScalar(v) { return typeof v === 'number' || (typeof v === 'string' && v.length > 0 && v.length <= 40); }
  function possessive(name) { return name + '’s'; }

  // What an entry lets a hero buy, read the way the server's heroBuyList does:
  // the points are the first positive "<x>_points" property, the list the
  // first property (in key order) whose items all have a name and a cost.
  var OPTS = '_choices_json';
  function buyListOf(c) {
    var props = (c && c.properties) || {}, keys = Object.keys(props).sort(), budget = 0, i;
    for (i = 0; i < keys.length; i++) {
      if (typeof props[keys[i]] === 'number' && props[keys[i]] > 0 && /_points$/.test(keys[i])) { budget = props[keys[i]]; break; }
    }
    if (!budget) return null;
    for (i = 0; i < keys.length; i++) {
      var list = props[keys[i]];
      if (!Array.isArray(list) || !list.length) continue;
      var ok = list.every(function (it) { return it && typeof it.name === 'string' && it.name.trim() && typeof it.cost === 'number' && it.cost >= 0; });
      if (ok) return { budget: budget, items: list };
    }
    return null;
  }
  function boughtOf(h, key) {
    try {
      var v = JSON.parse(h.values[key + OPTS] || '[]');
      return Array.isArray(v) ? v.filter(function (x) { return typeof x === 'string'; }) : [];
    } catch (e) { return []; }
  }
  // Grants a culture or career lists (skills, languages, perks) are chosen on
  // the sheet, which keeps those lists; the builder only says so.
  function grantsOf(c) {
    var keys = Object.keys((c && c.properties) || {}).join(' ');
    return ['skills', 'languages', 'perks'].filter(function (w) { return new RegExp(w.slice(0, -1), 'i').test(keys); });
  }

  // Roll ranges: a list of up to ten is read off one d10, a longer one off
  // percentile dice, each entry owning an even share of the faces.
  function sidesFor(n) { return n <= 10 ? 10 : 100; }
  function rangeOf(n, i) { var R = sidesFor(n); return [Math.floor(i * R / n) + 1, Math.floor((i + 1) * R / n)]; }
  function fmtFace(n, x) { return sidesFor(n) === 100 ? (x === 100 ? '00' : (x < 10 ? '0' + x : String(x))) : String(x); }
  function fmtRange(n, r) { return r[0] === r[1] ? fmtFace(n, r[0]) : fmtFace(n, r[0]) + '–' + fmtFace(n, r[1]); }
  function indexFor(n, total) { for (var i = 0; i < n; i++) { var r = rangeOf(n, i); if (total >= r[0] && total <= r[1]) return i; } return n - 1; }
  function rollFace(sides) {
    try {
      var a = new Uint32Array(1);
      window.crypto.getRandomValues(a);
      return 1 + (a[0] % sides);
    } catch (e) { return 1 + Math.floor(Math.random() * sides); }
  }
  // The entries the dice can land on: Ask first ones are skipped, so the
  // numbers run over what a player may simply take.
  function rollable(st) { return st.choices.filter(function (c) { return c.state !== 'ask_first'; }); }

  // Throws one roll per list ([{label, n}]) and resolves to each list's
  // face. Chronicle's dice engine throws them on screen when the site has
  // it (it decides the faces first and honours Calm and Off itself);
  // otherwise, or if it refuses, the faces are drawn here at once.
  function throwFor(lists, from) {
    var instant = function () { return lists.map(function (l) { return rollFace(sidesFor(l.n)); }); };
    var dice = window.Chronicle && Chronicle.dice;
    if (!dice || typeof dice.roll !== 'function') return Promise.resolve(instant());
    var groups = lists.map(function (l) {
      return { label: l.label, dice: sidesFor(l.n) === 10 ? [{ sides: 10 }] : [{ sides: 10, tens: true }, { sides: 10 }] };
    });
    return Promise.resolve().then(function () {
      return dice.roll(groups, {
        from: from,
        describe: function (g, values) { return g.label + ': ' + (values || []).join(' and '); }
      });
    }).then(function (res) {
      return lists.map(function (l, i) {
        var v = (res[i] && res[i].values) || [];
        if (sidesFor(l.n) === 10) return v[0] >= 1 && v[0] <= 10 ? v[0] : rollFace(10);
        // Percentile: the tens die reads 00-90, the ones die's 10 is a 0,
        // and 00 with 0 is 100.
        var t = (v[0] || 0) + ((v[1] || 0) % 10);
        return t === 0 ? 100 : t;
      });
    }).catch(instant);
  }

  function Builder(el) {
    this.el = el;
    this.planUrl = el.getAttribute('data-plan-url');
    this.fieldsBase = el.getAttribute('data-fields-base');
    this.csrf = el.getAttribute('data-csrf');
    try { this.heroes = JSON.parse(el.getAttribute('data-heroes') || '[]') || []; } catch (e) { this.heroes = []; }
    this.heroes.forEach(function (h) { h.values = h.values || {}; h.hidden = h.hidden || []; });
    this.panel = null;
    this.busy = false;
  }

  // --- Loading and the draft kept on this device ---
  Builder.prototype.load = function () {
    var self = this;
    var get = (window.Chronicle && Chronicle.apiFetch) ? Chronicle.apiFetch(this.planUrl) : fetch(this.planUrl, { credentials: 'same-origin' });
    get.then(function (r) {
      if (!r.ok) throw new Error('plan ' + r.status);
      return r.json();
    }).then(function (plan) {
      self.plan = plan || {};
      self.plan.steps = (self.plan.steps || []).filter(function (s) { return (s.choices && s.choices.length) || s.canMake; });
      return self.readNotes().then(function () { self.render(); });
    }).catch(function () {
      self.el.innerHTML = '<p class="hb-wait">The hero builder could not open. Reload the page to try again.</p>';
    });
  };
  // The note a Director left on a player's own draft comes from the entry API
  // the editor uses; the plan does not carry it. A failure leaves it out.
  Builder.prototype.readNotes = function () {
    var self = this;
    var has = this.plan.steps.some(function (st) { return (st.choices || []).some(function (c) { return c.mine && c.status === 'draft'; }); });
    if (!has || !(window.Chronicle && Chronicle.apiFetch)) return Promise.resolve();
    return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.el.getAttribute('data-campaign-id')) + '/system-entries').then(function (r) {
      return r.ok ? r.json() : null;
    }).then(function (b) {
      var notes = {};
      ((b && b.entries) || []).forEach(function (e) { if (e.reviewNote && e.status === 'draft') notes[e.id] = e.reviewNote; });
      self.plan.steps.forEach(function (st) { (st.choices || []).forEach(function (c) { if (c.mine && notes[c.entryId]) c.reviewNote = notes[c.entryId]; }); });
    }).catch(function () { /* the marks still show without it */ });
  };
  // Whether the editor script came with the page; without it there is no
  // "make your own".
  Builder.prototype.ownEnabled = function () { return !!(window.ChronicleOwnEntry && window.ChronicleOwnEntry.Editor); };
  Builder.prototype.canOwn = function (st) { return !!(st && st.canMake && this.ownEnabled()); };


  // --- Lookups ---
  Builder.prototype.stepByKey = function (k) {
    for (var i = 0; i < this.plan.steps.length; i++) if (this.plan.steps[i].fieldKey === k) return this.plan.steps[i];
    return null;
  };
  Builder.prototype.choiceByName = function (st, name) {
    for (var i = 0; i < st.choices.length; i++) if (lower(st.choices[i].name) === lower(name)) return st.choices[i];
    return null;
  };
  Builder.prototype.heroById = function (id) {
    for (var i = 0; i < this.heroes.length; i++) if (this.heroes[i].id === id) return this.heroes[i];
    return null;
  };
  // The steps a hero shows: GM-only fields stay out of a player's sentence.
  Builder.prototype.stepsFor = function (h) {
    return this.plan.steps.filter(function (s) { return h.hidden.indexOf(s.fieldKey) < 0; });
  };
  // The pick of this step that waits for a Director: a saved hero's request
  // from the plan, or, on a hero not made yet, an Ask first entry just picked.
  Builder.prototype.waitingName = function (h, st) {
    var w = st.rules && st.rules.waiting;
    return w && String(w.entityId) === String(h.id) ? w.entryName : '';
  };
  Builder.prototype.left = function (h) {
    var self = this;
    return this.stepsFor(h).filter(function (s) { return !h.values[s.fieldKey] && !self.waitingName(h, s); });
  };
  Builder.prototype.waiting = function (h) {
    var self = this;
    return this.stepsFor(h).filter(function (s) { return self.waitingName(h, s); });
  };

  // --- The sentences ---
  Builder.prototype.render = function () {
    var self = this;
    this.el.hidden = !this.heroes.length;
    this.el.setAttribute('data-hb-ready', '');
    if (!this.plan.steps.length) { this.el.innerHTML = ''; return; }
    var html = '<div class="hb-wrap' + (this.panel ? ' is-open' : '') + '"><div class="hb-main">';
    html += this.heroes.map(function (h) { return self.heroHtml(h); }).join('');
    html += '</div><aside class="hb-panel" aria-label="Choices"' + (this.panel ? '' : ' hidden') + '>' + this.panelHtml() + '</aside></div>' +
      '<div class="hb-toast" role="status" aria-live="polite" hidden></div>';
    this.el.innerHTML = html;
    this.placePanel();
    this.mountOwn();
  };

  Builder.prototype.blankHtml = function (h, st) {
    var v = h.values[st.fieldKey], on = this.panel && this.panel.hero === h.id && this.panel.key === st.fieldKey;
    var c = v && this.choiceByName(st, v), wait = this.waitingName(h, st);
    // A hero that already holds a pick keeps showing it; the request that
    // would replace it waits beside it.
    if (wait && v) {
      return '<button type="button" class="hb-blank' + (on ? ' is-cur' : '') + (c && c.source === 'campaign' ? ' is-own' : '') + '" data-blank="' + esc(st.fieldKey) + '" data-hero="' + esc(h.id) + '"' +
        ' aria-label="' + esc(st.label + ': ' + v + '. Asking your ' + gm() + ' for ' + wait + '. Show every choice') + '" aria-expanded="' + !!on + '">' + esc(v) + '</button>' +
        ' <span class="hb-waiting">asking for ' + esc(wait) + ', waiting for your ' + gm() + '</span>';
    }
    if (wait) {
      return '<button type="button" class="hb-blank is-wait' + (on ? ' is-cur' : '') + '" data-blank="' + esc(st.fieldKey) + '" data-hero="' + esc(h.id) + '"' +
        ' aria-label="' + esc(st.label + ': ' + wait + ', waiting for your ' + gm() + '. Show every choice') + '" aria-expanded="' + !!on + '">' +
        esc(wait) + '</button> <span class="hb-waiting">waiting for your ' + gm() + '</span>';
    }
    return '<button type="button" class="hb-blank' + (v ? '' : ' is-empty') + (on ? ' is-cur' : '') +
      (c && c.source === 'campaign' ? ' is-own' : '') + '" data-blank="' + esc(st.fieldKey) + '" data-hero="' + esc(h.id) + '"' +
      ' aria-label="' + esc(st.label + ': ' + (v || 'not chosen') + '. Show every choice') + '" aria-expanded="' + !!on + '">' +
      esc(v || lower(st.label)) + '</button>';
  };

  // "Tess’s ancestry is [..], class is [..] and kit is [..]." The game
  // system may later supply its own sentence; this one fits any list.
  Builder.prototype.sentenceHtml = function (h, who) {
    var self = this, steps = this.stepsFor(h);
    if (!steps.length) return '';
    var parts = steps.map(function (st) { return esc(lower(st.label)) + ' is ' + self.blankHtml(h, st); });
    var body = parts.length > 1 ? parts.slice(0, -1).join(', ') + ' and ' + parts[parts.length - 1] : parts[0];
    return '<p class="hb-sentence">' + who + ' ' + body + '.</p>';
  };

  Builder.prototype.progressHtml = function (h) {
    var all = this.stepsFor(h).length, left = this.left(h).length, w = this.waiting(h).length;
    if (!all) return '';
    var said = left ? left + ' of ' + all + ' left' + (w ? ' · ' + w + ' waiting' : '') : (w ? w + ' waiting for your ' + gm() : 'Every choice made');
    return '<div class="hb-below"><span class="hb-progress">' + said +
      '<span class="hb-bar" aria-hidden="true"><i style="width:' + Math.round((all - left) / all * 100) + '%"></i></span></span>' +
      (left ? '<span class="hb-hint">Pick a blank to read every choice first.</span>' : '') + this.suggestedHtml(h) + '</div>';
  };

  // A Director's note asking for another pick, shown under the sentence.
  Builder.prototype.suggestedHtml = function (h) {
    var self = this;
    return this.stepsFor(h).map(function (st) {
      var g = st.rules && st.rules.suggested;
      if (!g || self.waitingName(h, st) || h.values[st.fieldKey]) return '';
      return '<span class="hb-note hb-suggest">' + window.ChronicleHeroRules.suggestedText(st.label, g) + '</span>';
    }).join('');
  };

  Builder.prototype.heroHtml = function (h) {
    var left = this.left(h).length;
    var sheet = this.fieldsBase.replace(/\/$/, '') + '/' + encodeURIComponent(h.id);
    // Only a page the game system draws is a sheet; any other opens as a plain page.
    var open = h.sheet ? 'Open the sheet' : 'Open the page';
    return '<article class="hb-hero" data-hero-card="' + esc(h.id) + '">' +
      '<header class="hb-head"><div class="hb-names"><span class="hb-kicker">Your hero</span><h2 class="hb-title">' + esc(h.name) + '</h2></div>' +
      '<div class="hb-tools"><a class="hb-link" href="' + esc(sheet) + '">' + open + '</a>' +
      (left ? '<button type="button" class="hb-btn" data-rollall="' + esc(h.id) + '">Roll everything left</button>' : '') +
      '</div></header>' + this.lockHtml(h) + this.sentenceHtml(h, esc(possessive(h.name))) + this.progressHtml(h) + '</article>';
  };

  // --- The choose panel: every entry on one list, with its rules ---
  Builder.prototype.panelHtml = function () {
    var p = this.panel;
    if (!p) return '';
    var h = this.heroById(p.hero), st = this.stepByKey(p.key);
    if (!h || !st) return '';
    if (p.own) return this.ownShell(h, st);
    var steps = this.stepsFor(h), idx = steps.indexOf(st), list = st.choices, n = list.length, rl = rollable(st), nr = rl.length;
    var rules = st.rules || {}, waitName = this.waitingName(h, st);
    var cur = h.values[st.fieldKey], fo = (p.focus != null && list[p.focus]) || (cur && this.choiceByName(st, cur)) || list[0];
    if (!fo) return this.ownOnly(h, st, idx, steps.length);
    var whose = esc(possessive(h.name));
    var html = '<div class="hb-ph"><div class="hb-ph1"><span class="hb-kicker">' + (idx + 1) + ' of ' + steps.length + '</span>' +
      '<button type="button" class="hb-x" data-act="close" aria-label="Close the choices"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>' +
      '<h3 class="hb-pt">Choose ' + whose + ' ' + esc(lower(st.label)) + '</h3>' +
      (n > 6 ? '<input type="search" class="hb-q" data-hb-q placeholder="Search ' + n + ' entries" aria-label="Search the ' + esc(lower(st.label)) + ' list" autocomplete="off">' : '') +
      '<span class="hb-die">' + (sidesFor(nr) === 10 ? 'Rolling uses a d10' : 'Rolling uses percentile dice, tens and ones') + ' against the numbers on the left' +
      (nr < n ? ' and skips Ask first' : '') + '.</span>' + this.panelNotes(st, h, waitName) + '</div>';
    html += '<div class="hb-list" role="listbox" aria-label="' + esc(st.label) + '">' + list.map(function (c, i) {
      var ask = c.state === 'ask_first', at = rl.indexOf(c);
      return '<button type="button" class="hb-opt" role="option" data-focus="' + i + '" aria-selected="' + (c === fo) + '">' +
        '<span class="hb-rg">' + (ask ? '' : fmtRange(nr, rangeOf(nr, at))) + '</span><b>' + esc(c.name) +
        (c.mine && c.status ? ' <span class="hb-tag">' + (c.status === 'sent' ? 'waiting for your ' + gm() : c.reviewNote ? 'changes asked' : 'draft') + '</span>'
          : c.source === 'campaign' ? ' <span class="hb-tag">this campaign</span>' : '') + '</b>' +
        (ask ? '<span class="hb-ask">Ask first</span>' : cur && lower(cur) === lower(c.name) ? '<span class="hb-cur">chosen</span>' : '<span></span>') + '</button>';
    }).join('') + (this.canOwn(st) ? '<button type="button" class="hb-opt hb-opt--own" data-own-new><span class="hb-rg"></span><b>Make your own ' + esc(lower(st.label)) + '</b><span class="hb-ask">Your ' + gm() + ' allows one</span></button>' : '') +
      '<p class="hb-none" data-hb-none hidden>Nothing on this list matches.</p></div>' + this.notHereHtml(rules);
    html += '<div class="hb-detail" data-hb-detail>' + this.detailHtml(st, fo, h) + '</div>';
    var chosen = cur && lower(cur) === lower(fo.name);
    if (this.locked(h, st)) {
      html += '<div class="hb-pf"><p class="hb-note hb-locked">' + esc(cur) + ' stays ' + esc(possessive(h.name)) + ' ' + esc(lower(st.label)) +
        ' for now. Your ' + gm() + ' can open the picks sooner.</p></div>';
    } else {
      html += '<div class="hb-pf"><button type="button" class="hb-btn hb-go" data-choose' + (chosen ? ' aria-pressed="true"' : '') + '>' +
        this.chooseText(fo, chosen, waitName) + '</button>' +
        (nr ? '<button type="button" class="hb-btn" data-rollone>Roll for me</button>' : '') + '</div>';
    }
    var prev = steps[idx - 1], next = steps[idx + 1];
    html += '<div class="hb-pf hb-steps">' +
      (prev ? '<button type="button" class="hb-link" data-go="' + esc(prev.fieldKey) + '">Back to ' + esc(lower(prev.label)) + '</button>' : '<span></span>') +
      (next ? '<button type="button" class="hb-link" data-go="' + esc(next.fieldKey) + '">On to ' + esc(lower(next.label)) + '</button>' : '<span></span>') + '</div>';
    return html;
  };

  // A chosen step the Director's times keep from changing now.
  Builder.prototype.locked = function (h, st) {
    return !!(h && h.lock && !h.lock.open && h.values[st.fieldKey]);
  };

  // The line under a hero's name that says when its picks may change. Only
  // a campaign with times gives a hero a lock.
  Builder.prototype.lockHtml = function (h) {
    var l = h.lock;
    if (!l || !l.ruled) return '';
    if (l.open) {
      if (!l.until) return '';
      var d = new Date(l.until), when = isNaN(d) ? '' : d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' });
      return '<p class="hb-lock is-open" data-hb-lock="open">You can change your picks until ' + esc(when) + '.' +
        (l.reopened ? ' Your ' + gm() + ' opened them.' : '') + '</p>';
    }
    return '<p class="hb-lock" data-hb-lock="shut"><i class="fa-solid fa-lock" aria-hidden="true"></i> Your picks are set for now.' +
      (l.nextLevel ? ' They open again when ' + esc(h.name) + ' reaches level ' + l.nextLevel + '.' : '') +
      ' Your ' + gm() + ' can open them sooner. Blanks can still be filled.</p>';
  };

  Builder.prototype.chooseText = function (c, chosen, waitName) {
    if (c.state === 'ask_first') return (waitName && lower(waitName) === lower(c.name) ? 'Asking for ' : 'Ask for ') + c.name;
    return (chosen ? 'Keep ' : 'Choose ') + c.name;
  };

  // What the Director's rules say about this list, above the entries.
  Builder.prototype.panelNotes = function (st, h, waitName) {
    return window.ChronicleHeroRules.notes(st, waitName, 'hb');
  };

  // "1 not here" at the list's foot, folding open the names and the reasons.
  Builder.prototype.notHereHtml = function (r) {
    return window.ChronicleHeroRules.notHere(r, !!(this.panel && this.panel.nh), 'hb');
  };

  Builder.prototype.detailHtml = function (st, c, h) {
    var props = c.properties || {};
    var chosen = h && h.values[st.fieldKey] && lower(h.values[st.fieldKey]) === lower(c.name);
    var facts = Object.keys(props).filter(function (k) { return !/_display$/.test(k) && isScalar(props[k]); }).slice(0, 6);
    var body = '';
    if (c.description) {
      body = c.source === 'campaign' ? '<div class="hb-text">' + c.description + '</div>'
        : (c.description.trim() !== (c.summary || '').trim() ? '<div class="hb-text">' + prose(c.description) + '</div>' : '');
    }
    var own = c.mine && c.status && this.ownEnabled() ? (c.reviewNote && c.status === 'draft' ? '<p class="hb-note"><b>Your ' + gm() + ' asked:</b> ' + esc(c.reviewNote) + '</p>' : '') +
      '<p class="hb-ownbar"><button type="button" class="hb-link" data-own-edit="' + st.choices.indexOf(c) + '">Edit your entry</button><span>' +
      (c.status === 'sent' ? 'Waiting for your ' + gm() + ', and you can still pick it.' : 'A draft only you and your ' + gm() + ' see. You can pick it now.') + '</span></p>' : '';
    return '<h4 class="hb-dn">' + esc(c.name) + '</h4>' + own + (c.state === 'ask_first' ? '<p class="hb-note">Ask first: your ' + gm() + ' allows it, or suggests another. You can carry on with the rest of your hero meanwhile.</p>' : '') +
      (c.summary ? '<p class="hb-lede">' + esc(c.summary) + '</p>' : '') +
      (facts.length ? '<dl class="hb-facts">' + facts.map(function (k) {
        return '<div class="hb-f"><dt>' + esc(words(k)) + '</dt><dd>' + esc(props[k]) + '</dd></div>';
      }).join('') + '</dl>' : '') + (chosen ? this.buyHtml(st, c, h) : '') + this.grantsNote(c, h) + body;
  };

  // The points an entry gives and what they buy, once it is this hero's pick.
  // A box that would overspend is off until something else is let go.
  Builder.prototype.buyHtml = function (st, c, h) {
    var buy = buyListOf(c);
    if (!buy) return '';
    var have = boughtOf(h, st.fieldKey).map(lower), spent = 0, shut = this.locked(h, st) && have.length > 0;
    buy.items.forEach(function (it) { if (have.indexOf(lower(it.name)) >= 0) spent += it.cost; });
    var left = buy.budget - spent;
    return '<fieldset class="hb-buy" data-hb-buy><legend>Spend ' + esc(possessive(c.name)) + ' ' + buy.budget + ' points</legend>' +
      '<p class="hb-buy-left" aria-live="polite">' + left + ' of ' + buy.budget + ' left</p>' +
      buy.items.map(function (it) {
        var on = have.indexOf(lower(it.name)) >= 0, off = shut || (!on && it.cost > left);
        return '<label class="hb-trait' + (off ? ' is-off' : '') + '"><input type="checkbox" data-buy="' + esc(it.name) + '"' +
          (on ? ' checked' : '') + (off ? ' disabled' : '') + '><span class="hb-trait-n">' + esc(it.name) + '</span>' +
          '<span class="hb-trait-c">' + it.cost + (it.cost === 1 ? ' point' : ' points') + '</span></label>';
      }).join('') + '</fieldset>';
  };

  Builder.prototype.grantsNote = function (c, h) {
    var g = grantsOf(c);
    if (!g.length) return '';
    var what = g.length > 1 ? g.slice(0, -1).join(', ') + ' and ' + g[g.length - 1] : g[0];
    return '<p class="hb-note">You choose the ' + esc(what) + ' this gives on ' + (h ? esc(possessive(h.name)) : 'your hero’s') + ' sheet.</p>';
  };

  // Ticks or unticks a bought trait on the current pick and saves the list.
  Builder.prototype.buy = function (box) {
    var p = this.panel, h = p && this.heroById(p.hero), st = p && this.stepByKey(p.key);
    if (!h || !st) return;
    var c = this.choiceByName(st, h.values[st.fieldKey] || ''), buy = buyListOf(c);
    if (!buy) return;
    var names = [];
    this.el.querySelectorAll('[data-hb-buy] [data-buy]').forEach(function (b) { if (b.checked) names.push(b.getAttribute('data-buy')); });
    var v = {}, self = this;
    v[st.fieldKey + OPTS] = names.length ? JSON.stringify(names) : null;
    this.apply(h, v).then(function () {
      var d = self.el.querySelector('[data-hb-detail]'), top = d ? d.scrollTop : 0;
      self.redrawDetail(h, st, c);
      d = self.el.querySelector('[data-hb-detail]');
      if (d) d.scrollTop = top;
      // Found by comparing names, not a built selector, so no trait name
      // can break or bend the query.
      var name = box.getAttribute('data-buy');
      Array.prototype.some.call(self.el.querySelectorAll('[data-hb-buy] [data-buy]'), function (b) {
        if (b.getAttribute('data-buy') !== name) return false;
        b.focus({ preventScroll: true });
        return true;
      });
    }, function () { self.redrawDetail(h, st, c); });
  };

  Builder.prototype.redrawDetail = function (h, st, c) {
    var d = this.el.querySelector('[data-hb-detail]');
    if (d) d.innerHTML = this.detailHtml(st, c, h);
  };

  // Beside the sentences on a wide screen; on a narrow one, right under the
  // hero being filled in, so it never covers the sentence it edits.
  Builder.prototype.narrow = function () {
    try { return window.matchMedia('(max-width: 899px)').matches; } catch (e) { return false; }
  };
  Builder.prototype.placePanel = function () {
    var aside = this.el.querySelector('.hb-panel'), wrap = this.el.querySelector('.hb-wrap');
    if (!aside || !wrap) return;
    if (this.panel && this.narrow()) {
      var card = this.el.querySelector('[data-hero-card="' + this.panel.hero + '"]');
      if (card && card.nextSibling !== aside) card.parentNode.insertBefore(aside, card.nextSibling);
    } else if (aside.parentNode !== wrap) {
      wrap.appendChild(aside);
    }
  };

  // --- The player's own entry, in the panel ---

  // The panel while writing: a way back, and the shared editor's form and foot.
  Builder.prototype.ownShell = function () {
    return '<div class="hb-ph"><div class="hb-ph1"><button type="button" class="hb-link" data-ownback>‹ Back to the list</button>' +
      '<button type="button" class="hb-x" data-act="close" aria-label="Close the choices"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div></div>' +
      '<div class="hb-own" data-ownbody></div><div class="hb-pf hb-ownfoot" data-ownfoot></div>';
  };
  // A list with nothing in it yet that a player may still write one for.
  Builder.prototype.ownOnly = function (h, st, idx, total) {
    var whose = esc(possessive(h.name));
    return '<div class="hb-ph"><div class="hb-ph1"><span class="hb-kicker">' + (idx + 1) + ' of ' + total + '</span>' +
      '<button type="button" class="hb-x" data-act="close" aria-label="Close the choices"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>' +
      '<h3 class="hb-pt">Choose ' + whose + ' ' + esc(lower(st.label)) + '</h3></div>' +
      '<div class="hb-list" role="listbox" aria-label="' + esc(st.label) + '">' + (this.canOwn(st) ? '<button type="button" class="hb-opt hb-opt--own" data-own-new><span class="hb-rg"></span><b>Make your own ' + esc(lower(st.label)) + '</b><span class="hb-ask">Your ' + gm() + ' allows one</span></button>' : '<p class="hb-none">Nothing is on this list for you right now.</p>') + '</div>';
  };
  Builder.prototype.mountOwn = function () {
    var o = this.panel && this.panel.own, aside = this.el.querySelector('.hb-panel');
    if (o && aside) o.editor.mount(aside.querySelector('[data-ownbody]'), aside.querySelector('[data-ownfoot]'));
  };
  Builder.prototype.endOwn = function () {
    if (this.panel && this.panel.own) { this.panel.own.editor.destroy(); this.panel.own = null; }
  };
  // Opens the editor for a new entry, or for the player's own entry at index.
  Builder.prototype.openOwn = function (index) {
    var self = this, p = this.panel, Own = window.ChronicleOwnEntry;
    if (!p || !Own || !Own.Editor) return;
    var h = this.heroById(p.hero), st = this.stepByKey(p.key), c = index != null ? st.choices[index] : null;
    if (!h || !st) return;
    var ed = new Own.Editor({
      campaignId: this.el.getAttribute('data-campaign-id'), csrf: this.csrf, step: st, entry: c, forId: h.id,
      onSaved: function (e, mode, isNew) { self.ownSaved(h, st, e, mode, isNew); }
    });
    this.endOwn();
    this.panel = { hero: h.id, key: st.fieldKey, focus: null, own: { editor: ed } };
    this.drawPanel();
    ed.start();
    var aside = this.el.querySelector('.hb-panel');
    if (aside && this.narrow()) aside.scrollIntoView({ block: 'nearest' });
  };
  Builder.prototype.backFromOwn = function () {
    var self = this, p = this.panel;
    if (!p || !p.own) return;
    p.own.editor.requestClose(function () { var h = p.hero, k = p.key; self.endOwn(); self.panel = { hero: h, key: k, focus: null }; self.drawPanel(); });
  };
  // A new entry joins the list and is picked for the hero at once, as the
  // Own entries page always did for a hero it was opened for. Editing an
  // entry re-picks it only when it is already the hero's pick (a rename), so
  // fixing a draft never replaces what the hero holds.
  Builder.prototype.ownSaved = function (h, st, e, mode, isNew) {
    var ch = window.ChronicleOwnEntry.Editor.choiceOf(e), at = -1, wasPick = false;
    st.choices.forEach(function (c, n) { if (c.entryId === e.id) at = n; });
    if (at >= 0) {
      wasPick = !!h.values[st.fieldKey] && lower(h.values[st.fieldKey]) === lower(st.choices[at].name);
      if (st.choices[at].state) ch.state = st.choices[at].state;
      st.choices[at] = ch;
    } else st.choices.push(ch);
    if (isNew) st.canMake = false;
    this.endOwn();
    this.panel = { hero: h.id, key: st.fieldKey, focus: null };
    if (isNew || wasPick) {
      this.pick(h.id, st.fieldKey, ch.name);
      if (!buyListOf(ch)) this.toast(ch.name + (mode === 'send' ? ' handed in. It waits for your ' + gm() + ', and you can still change it.' : ' saved as a draft and picked.'));
    } else {
      this.drawPanel();
      this.toast(ch.name + (mode === 'send' ? ' handed in. It waits for your ' + gm() + '.' : ' saved as a draft.'));
    }
  };

  Builder.prototype.openPanel = function (heroId, key) {
    var self = this;
    // Writing is never thrown away by opening another blank: the editor asks.
    if (this.panel && this.panel.own) {
      this.panel.own.editor.requestClose(function () { self.endOwn(); self.openPanel(heroId, key); });
      return;
    }
    var reopen = !!this.panel;
    this.panel = { hero: heroId, key: key, focus: null };
    this.drawPanel();
    var aside = this.el.querySelector('.hb-panel'), q = this.el.querySelector('[data-hb-q]');
    if (!reopen) {
      aside.classList.remove('is-in');
      void aside.offsetWidth;
      aside.classList.add('is-in');
    }
    (q || this.el.querySelector('.hb-opt[aria-selected="true"]') || aside).focus({ preventScroll: true });
    if (this.narrow()) aside.scrollIntoView({ block: 'nearest' });
  };

  Builder.prototype.closePanel = function () {
    var self = this;
    if (this.panel && this.panel.own) {
      this.panel.own.editor.requestClose(function () { self.endOwn(); self.closePanel(); });
      return;
    }
    var was = this.panel;
    this.panel = null;
    this.drawPanel();
    if (was) {
      var b = this.el.querySelector('[data-blank="' + was.key + '"][data-hero="' + was.hero + '"]');
      if (b) b.focus({ preventScroll: true });
    }
  };

  // Re-draws the panel and the blanks' marks, leaving the sentences (and a
  // half-typed name) as they are.
  Builder.prototype.drawPanel = function () {
    var self = this, aside = this.el.querySelector('.hb-panel'), wrap = this.el.querySelector('.hb-wrap');
    if (!aside) return;
    var list = aside.querySelector('.hb-list'), top = list ? list.scrollTop : 0;
    aside.innerHTML = this.panelHtml();
    aside.hidden = !this.panel;
    wrap.classList.toggle('is-open', !!this.panel);
    var list2 = aside.querySelector('.hb-list');
    if (list2) list2.scrollTop = top;
    this.el.querySelectorAll('.hb-blank').forEach(function (b) {
      var on = !!self.panel && b.getAttribute('data-hero') === self.panel.hero && b.getAttribute('data-blank') === self.panel.key;
      b.classList.toggle('is-cur', on);
      b.setAttribute('aria-expanded', String(on));
    });
    this.placePanel();
    this.mountOwn();
  };

  Builder.prototype.focusEntry = function (i) {
    this.panel.focus = i;
    var aside = this.el.querySelector('.hb-panel'), st = this.stepByKey(this.panel.key), c = st.choices[i];
    var h = this.heroById(this.panel.hero), cur = h.values[st.fieldKey], chosen = cur && lower(cur) === lower(c.name);
    aside.querySelectorAll('.hb-opt').forEach(function (o) { o.setAttribute('aria-selected', String(+o.getAttribute('data-focus') === i)); });
    var d = aside.querySelector('[data-hb-detail]');
    d.innerHTML = this.detailHtml(st, c, h);
    d.scrollTop = 0;
    var btn = aside.querySelector('[data-choose]');
    btn.textContent = this.chooseText(c, chosen, this.waitingName(h, st));
    if (chosen) btn.setAttribute('aria-pressed', 'true'); else btn.removeAttribute('aria-pressed');
  };

  Builder.prototype.filter = function (q) {
    var v = lower(q).trim(), shown = 0, aside = this.el.querySelector('.hb-panel');
    aside.querySelectorAll('.hb-opt').forEach(function (o) {
      if (o.hasAttribute('data-own-new')) { o.hidden = !!v; return; }
      var hit = !v || lower(o.querySelector('b').textContent).indexOf(v) >= 0;
      o.hidden = !hit;
      if (hit) shown++;
    });
    aside.querySelector('[data-hb-none]').hidden = shown > 0;
  };
  // --- Picking and saving ---
  Builder.prototype.redrawHero = function (h) {
    var card = this.el.querySelector('[data-hero-card="' + h.id + '"]');
    if (card) card.outerHTML = this.heroHtml(h);
    this.drawPanel();
  };

  // Sends a partial fields write for an existing hero. A null value clears
  // the field, which is how Undo puts back a blank.
  Builder.prototype.send = function (h, patch) {
    var url = this.fieldsBase.replace(/\/$/, '') + '/' + encodeURIComponent(h.id) + '/fields';
    var opts = { method: 'PUT', body: { fields_patch: patch }, csrfToken: this.csrf };
    var req = (window.Chronicle && Chronicle.apiFetch) ? Chronicle.apiFetch(url, opts)
      : fetch(url, { method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrf }, body: JSON.stringify({ fields_patch: patch }) });
    return req.then(function (r) {
      if (r.ok) return null;
      return r.json().catch(function () { return {}; }).then(function (b) {
        var e = new Error(b.message || b.error || 'That choice could not be saved. Try again.');
        e.status = r.status;
        throw e;
      });
    });
  };

  // Puts values on a hero: at once on the page, then saved for an existing
  // hero (put back if the save fails) or kept in the draft for a new one.
  Builder.prototype.apply = function (h, values) {
    var self = this, before = {};
    Object.keys(values).forEach(function (k) {
      before[k] = h.values[k] || null;
      // What was bought belongs to the old pick; the server clears it on a
      // saved hero, and it goes here too.
      if (self.stepByKey(k) && lower(h.values[k]) !== lower(values[k])) delete h.values[k + OPTS];
      if (values[k] == null) delete h.values[k]; else h.values[k] = values[k];
    });
    return this.send(h, values).then(function () { return before; }, function (err) {
      // The Director's times closed since the page was drawn: show the lock.
      if (err.status === 403 && /locked/i.test(err.message)) h.lock = { ruled: true, open: false };
      // Put back only what a later pick has not already changed again.
      Object.keys(before).forEach(function (k) {
        if ((h.values[k] || null) !== (values[k] == null ? null : values[k])) return;
        if (before[k] == null) delete h.values[k]; else h.values[k] = before[k];
      });
      self.redrawHero(h);
      self.toast(err.message);
      throw err;
    });
  };

  // A small green Saved beside the blank that changed, gone after a moment.
  Builder.prototype.tick = function (h, key) {
    var b = this.el.querySelector('[data-blank="' + key + '"][data-hero="' + h.id + '"]');
    if (!b) return;
    var t = document.createElement('span');
    t.className = 'hb-tick';
    t.textContent = 'Saved';
    b.insertAdjacentElement('afterend', t);
    setTimeout(function () { t.remove(); }, 1900);
  };

  Builder.prototype.toast = function (msg, undo) {
    var t = this.el.querySelector('.hb-toast'), self = this;
    if (!t) return;
    t.innerHTML = '<span>' + esc(msg) + '</span>' + (undo ? '<button type="button" data-undo>Undo</button>' : '');
    t.hidden = false;
    this.undo = undo || null;
    clearTimeout(this.toastTimer);
    this.toastTimer = setTimeout(function () { t.hidden = true; self.undo = null; }, undo ? 6000 : 3500);
  };

  // Picking an Ask first entry on a saved hero sends a request instead of
  // saving the pick: the hero's field stays as it was, and the blank reads
  // that the pick waits for the Director. A hero that already holds the
  // entry just keeps it, which the server tells apart.
  Builder.prototype.askPick = function (h, st, c) {
    var self = this, patch = {}, steps = this.stepsFor(h), at = steps.indexOf(st);
    patch[st.fieldKey] = c.name;
    var rest = this.left(h).filter(function (s) { return s.fieldKey !== st.fieldKey; });
    var next = rest.filter(function (s) { return steps.indexOf(s) > at; })[0] || rest[0];
    this.panel = next ? { hero: h.id, key: next.fieldKey, focus: null } : null;
    var url = this.fieldsBase.replace(/\/$/, '') + '/' + encodeURIComponent(h.id) + '/fields';
    var opts = { method: 'PUT', body: { fields_patch: patch }, csrfToken: this.csrf };
    var req = (window.Chronicle && Chronicle.apiFetch) ? Chronicle.apiFetch(url, opts)
      : fetch(url, { method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrf }, body: JSON.stringify({ fields_patch: patch }) });
    return req.then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (b) {
        if (!r.ok) throw new Error(b.message || b.error || 'That choice could not be sent. Try again.');
        if (b.requestFailed && b.requestFailed.indexOf(st.fieldKey) >= 0) throw new Error('Your pick of ' + c.name + ' could not be sent to your ' + gm() + '. Try again.');
        var q = (b.requests || []).filter(function (x) { return x.fieldKey === st.fieldKey; })[0];
        if (q) {
          st.rules = st.rules || {};
          st.rules.waiting = { id: q.id, entryName: q.entryName || c.name, entityId: h.id };
          delete st.rules.suggested;
          self.toast(c.name + ' is waiting for your ' + gm() + '. Carry on with the rest of your hero.');
        } else {
          h.values[st.fieldKey] = c.name;
          self.tick(h, st.fieldKey);
        }
        self.redrawHero(h);
      });
    }).catch(function (err) { self.redrawHero(h); self.toast(err.message); });
  };

  Builder.prototype.pick = function (heroId, key, name, note) {
    var h = this.heroById(heroId), self = this, v = {};
    v[key] = name;
    var pc = this.choiceByName(this.stepByKey(key), name);
    if (pc && pc.state === 'ask_first' && !note) return this.askPick(h, this.stepByKey(key), pc);
    // Choosing moves the panel on to the next blank, so filling in a hero is
    // one choice after another; with nothing left it closes.
    var rest = this.left(h).filter(function (s) { return s.fieldKey !== key; });
    var steps = this.stepsFor(h), at = steps.indexOf(this.stepByKey(key));
    var next = rest.filter(function (s) { return steps.indexOf(s) > at; })[0] || rest[0];
    // A pick with points to spend keeps the panel on it, so its traits are
    // bought now rather than forgotten.
    var buy = buyListOf(this.choiceByName(this.stepByKey(key), name));
    if (buy && !note) {
      this.panel = { hero: h.id, key: key, focus: null };
      var keeping = this.apply(h, v);
      this.redrawHero(h);
      keeping.then(function () { self.tick(h, key); }, function () {});
      this.toast(name + ' chosen. Spend its ' + buy.budget + ' points below, then move on.');
      var first = this.el.querySelector('[data-hb-buy] [data-buy]:not([disabled])');
      if (first) first.focus({ preventScroll: false });
      return keeping;
    }
    this.panel = next ? { hero: h.id, key: next.fieldKey, focus: null } : null;
    var saving = this.apply(h, v);
    this.redrawHero(h);
    // Saved shows once the server has it; a new hero has nothing to save yet.
    saving.then(function () { self.tick(h, key); }, function () {});
    if (note) this.toast(note);
    else if (!next) this.toast(h.name + ' has every choice made.');
    if (!next) this.focusAfter(h);
    else { var q = this.el.querySelector('[data-hb-q]'); if (q && !this.narrow()) q.focus({ preventScroll: true }); }
    return saving;
  };

  Builder.prototype.focusAfter = function (h) {
    var el = this.el.querySelector('[data-hero-card="' + h.id + '"] .hb-title');
    if (el) { if (el.classList.contains('hb-title')) el.setAttribute('tabindex', '-1'); el.focus({ preventScroll: true }); }
  };

  Builder.prototype.rollOne = function (btn) {
    var self = this, heroId = this.panel.hero, st = this.stepByKey(this.panel.key), rl = rollable(st), n = rl.length;
    if (this.rolling || !n) return;
    this.rolling = true;
    throwFor([{ label: st.label, n: n }], btn).then(function (faces) {
      self.rolling = false;
      var c = rl[indexFor(n, faces[0])];
      self.pick(heroId, st.fieldKey, c.name, 'Rolled ' + fmtFace(n, faces[0]) + ' for ' + lower(st.label) + ': ' + c.name + '.');
    });
  };

  Builder.prototype.rollAll = function (id, btn) {
    var h = this.heroById(id), self = this;
    var left = this.left(h).filter(function (st) { return rollable(st).length; });
    if (!left.length || this.rolling) return;
    this.rolling = true;
    throwFor(left.map(function (st) { return { label: st.label, n: rollable(st).length }; }), btn).then(function (faces) {
      self.rolling = false;
      var v = {};
      // Only blanks still empty when the dice land are filled.
      left.forEach(function (st, i) {
        var rl = rollable(st);
        if (!h.values[st.fieldKey]) v[st.fieldKey] = rl[indexFor(rl.length, faces[i])].name;
      });
      var keys = Object.keys(v);
      if (!keys.length) return;
      self.panel = null;
      var saving = self.apply(h, v);
      self.redrawHero(h);
      keys.forEach(function (k) {
        var b = self.el.querySelector('[data-blank="' + k + '"][data-hero="' + h.id + '"]');
        if (b) b.classList.add('is-glow');
      });
      var names = keys.map(function (k) { return v[k]; }).join(', ');
      saving.then(function (before) {
        self.toast('The dice chose ' + names + '. Saved.', function () {
          self.apply(h, before).then(function () { self.redrawHero(h); self.toast('Put back.'); }, function () {});
          self.redrawHero(h);
        });
      }, function () {});
    });
  };

  // --- Wiring ---
  Builder.prototype.wire = function () {
    var self = this, el = this.el;
    this.onClick = function (e) {
      var t = e.target.closest('button');
      if (!t || !el.contains(t) || !self.plan) return;
      if (t.hasAttribute('data-own-new')) { self.openOwn(null); return; }
      if (t.hasAttribute('data-own-edit')) { self.openOwn(+t.getAttribute('data-own-edit')); return; }
      if (t.hasAttribute('data-ownback')) { self.backFromOwn(); return; }
      if (t.hasAttribute('data-blank')) {
        var hid = t.getAttribute('data-hero'), key = t.getAttribute('data-blank');
        if (self.panel && self.panel.hero === hid && self.panel.key === key) self.closePanel(); else self.openPanel(hid, key);
      } else if (t.hasAttribute('data-focus')) {
        self.focusEntry(+t.getAttribute('data-focus'));
      } else if (t.hasAttribute('data-choose')) {
        var st = self.stepByKey(self.panel.key), c = st.choices[self.panel.focus != null ? self.panel.focus : self.currentIndex(st)];
        self.pick(self.panel.hero, st.fieldKey, c.name);
      } else if (t.hasAttribute('data-rollone')) {
        self.rollOne(t);
      } else if (t.hasAttribute('data-rollall')) {
        self.rollAll(t.getAttribute('data-rollall'), t);
      } else if (t.hasAttribute('data-go')) {
        self.openPanel(self.panel.hero, t.getAttribute('data-go'));
      } else if (t.hasAttribute('data-undo')) {
        var u = self.undo;
        self.el.querySelector('.hb-toast').hidden = true;
        self.undo = null;
        if (u) u();
      } else if (t.getAttribute('data-act') === 'nothere') {
        if (self.panel) {
          self.panel.nh = !self.panel.nh;
          self.drawPanel();
          var nb = el.querySelector('[data-act="nothere"]');
          if (nb) nb.focus({ preventScroll: true });
        }
      } else if (t.getAttribute('data-act') === 'close') {
        self.closePanel();
      }
    };
    this.onInput = function (e) {
      if (e.target.hasAttribute('data-hb-q')) {
        self.filter(e.target.value);
      }
    };
    this.onKey = function (e) {
      if (e.key === 'Escape' && self.panel) { e.preventDefault(); self.closePanel(); return; }
      // Up and down move through the list; the entry read is the one picked.
      if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && e.target.closest && (e.target.closest('.hb-list') || e.target.hasAttribute('data-hb-q'))) {
        var opts = Array.prototype.filter.call(el.querySelectorAll('.hb-opt[data-focus]'), function (o) { return !o.hidden; });
        if (!opts.length) return;
        e.preventDefault();
        var at = opts.indexOf(el.querySelector('.hb-opt[aria-selected="true"]'));
        var to = opts[Math.max(0, Math.min(opts.length - 1, at + (e.key === 'ArrowDown' ? 1 : -1)))];
        self.focusEntry(+to.getAttribute('data-focus'));
        to.focus({ preventScroll: false });
      }
    };
    this.onResize = function () { self.placePanel(); };
    el.addEventListener('click', this.onClick);
    el.addEventListener('input', this.onInput);
    this.onChange = function (e) {
      if (e.target.hasAttribute && e.target.hasAttribute('data-buy')) self.buy(e.target);
    };
    el.addEventListener('change', this.onChange);
    el.addEventListener('keydown', this.onKey);
    window.addEventListener('resize', this.onResize);
  };


  Builder.prototype.unwire = function () {
    this.endOwn();
    this.el.removeEventListener('click', this.onClick);
    this.el.removeEventListener('input', this.onInput);
    this.el.removeEventListener('change', this.onChange);
    this.el.removeEventListener('keydown', this.onKey);
    window.removeEventListener('resize', this.onResize);
    clearTimeout(this.toastTimer);
  };

  Builder.prototype.currentIndex = function (st) {
    var h = this.heroById(this.panel.hero), cur = h.values[st.fieldKey];
    for (var i = 0; i < st.choices.length; i++) if (cur && lower(st.choices[i].name) === lower(cur)) return i;
    return 0;
  };

  function register() {
    Chronicle.register('hero-builder', {
      init: function (el) {
        var b = new Builder(el);
        el._heroBuilder = b;
        b.wire();
        b.load();
      },
      destroy: function (el) {
        if (el._heroBuilder) el._heroBuilder.unwire();
      }
    });
  }
  if (window.Chronicle && Chronicle.register) register();
})();
