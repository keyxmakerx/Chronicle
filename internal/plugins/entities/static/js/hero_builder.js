/**
 * hero_builder.js — the hero builder on the Characters page.
 *
 * Each of the viewer's heroes reads as one sentence ("Tess's ancestry is
 * [Human], class is [ ] …"). A blank opens a panel docked beside the
 * sentence with every choice on that list, its rules and a roll range, so a
 * player reads before they pick and never leaves the page. A pick on an
 * existing hero saves at once through the fields door, with a green Saved
 * beside it; a new hero keeps its picks on this device until it is made.
 *
 * The steps come from the creator plan (the same lists as the creator
 * page), so a campaign's own entries appear beside the game system's.
 * Campaign descriptions arrive as HTML the server sanitized; package text is
 * plain and escaped here.
 *
 * Loaded on sight for data-widget="hero-builder" (ADR-063).
 */
(function () {
  'use strict';

  function esc(v) {
    var s = (v == null) ? '' : String(v);
    return s.replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

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
  // One roll against a list of n: the face and the entry it lands on.

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
    this.createUrl = el.getAttribute('data-create-url');
    this.fieldsBase = el.getAttribute('data-fields-base');
    this.csrf = el.getAttribute('data-csrf');
    this.canCreate = el.getAttribute('data-can-create') === 'true';
    this.forSelf = el.getAttribute('data-for-self') === 'true';
    this.typeName = el.getAttribute('data-type-name') || 'character';
    this.draftKey = 'chronicle:hero-builder:' + el.getAttribute('data-campaign-id');
    try { this.heroes = JSON.parse(el.getAttribute('data-heroes') || '[]') || []; } catch (e) { this.heroes = []; }
    this.heroes.forEach(function (h) { h.values = h.values || {}; h.hidden = h.hidden || []; });
    this.fresh = { id: 'new', name: '', values: {}, hidden: [], isNew: true };
    // The new-hero sentence stays folded until the page's Create hero is
    // clicked, or a draft left open on this device brings it back.
    this.freshOpen = false;
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
      self.plan.steps = (self.plan.steps || []).filter(function (s) { return s.choices && s.choices.length; });
      self.restore();
      self.render();
    }).catch(function () {
      self.el.innerHTML = '<p class="hb-wait">The hero builder could not open. Reload the page to try again.</p>';
    });
  };
  Builder.prototype.restore = function () {
    if (!this.canCreate) return;
    try {
      var d = JSON.parse(window.localStorage.getItem(this.draftKey) || 'null');
      if (!d) return;
      var self = this;
      this.fresh.name = typeof d.name === 'string' ? d.name : '';
      Object.keys(d.values || {}).forEach(function (k) {
        // Keep only picks the lists still offer, in case the system changed.
        var st = self.stepByKey(k), c = st && self.choiceByName(st, d.values[k]);
        if (c) self.fresh.values[k] = c.name;
      });
      this.freshOpen = d.open !== false && !!(this.fresh.name.trim() || Object.keys(this.fresh.values).length);
    } catch (e) { /* no storage: start fresh */ }
  };
  Builder.prototype.saveDraft = function () {
    try { window.localStorage.setItem(this.draftKey, JSON.stringify({ name: this.fresh.name, values: this.fresh.values, open: this.freshOpen })); } catch (e) { /* ignore */ }
  };
  Builder.prototype.clearDraft = function () {
    try { window.localStorage.removeItem(this.draftKey); } catch (e) { /* ignore */ }
  };

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
    if (id === 'new') return this.fresh;
    for (var i = 0; i < this.heroes.length; i++) if (this.heroes[i].id === id) return this.heroes[i];
    return null;
  };
  // The steps a hero shows: GM-only fields stay out of a player's sentence.
  Builder.prototype.stepsFor = function (h) {
    return this.plan.steps.filter(function (s) { return h.hidden.indexOf(s.fieldKey) < 0; });
  };
  Builder.prototype.left = function (h) {
    return this.stepsFor(h).filter(function (s) { return !h.values[s.fieldKey]; });
  };

  // --- The sentences ---
  Builder.prototype.render = function () {
    var self = this;
    var showFresh = this.canCreate && this.freshOpen;
    // Nothing to fill in yet: the mount stays, empty, so Create hero can open it.
    this.el.hidden = !this.heroes.length && !showFresh;
    this.el.setAttribute('data-hb-ready', '');
    if (!this.plan.steps.length && !this.canCreate) { this.el.innerHTML = ''; return; }
    var html = '<div class="hb-wrap' + (this.panel ? ' is-open' : '') + '"><div class="hb-main">';
    html += this.heroes.map(function (h) { return self.heroHtml(h); }).join('');
    if (showFresh) html += this.freshHtml();
    html += '</div><aside class="hb-panel" aria-label="Choices"' + (this.panel ? '' : ' hidden') + '>' + this.panelHtml() + '</aside></div>' +
      '<div class="hb-toast" role="status" aria-live="polite" hidden></div>';
    this.el.innerHTML = html;
    this.placePanel();
  };

  Builder.prototype.blankHtml = function (h, st) {
    var v = h.values[st.fieldKey], on = this.panel && this.panel.hero === h.id && this.panel.key === st.fieldKey;
    var c = v && this.choiceByName(st, v);
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
    var all = this.stepsFor(h).length, left = this.left(h).length;
    if (!all) return '';
    return '<div class="hb-below"><span class="hb-progress">' + (left ? left + ' of ' + all + ' left' : 'Every choice made') +
      '<span class="hb-bar" aria-hidden="true"><i style="width:' + Math.round((all - left) / all * 100) + '%"></i></span></span>' +
      (left ? '<span class="hb-hint">Pick a blank to read every choice first.</span>' : '') + '</div>';
  };

  Builder.prototype.heroHtml = function (h) {
    var left = this.left(h).length;
    var sheet = this.fieldsBase.replace(/\/$/, '') + '/' + encodeURIComponent(h.id);
    return '<article class="hb-hero" data-hero-card="' + esc(h.id) + '">' +
      '<header class="hb-head"><div class="hb-names"><span class="hb-kicker">Your hero</span><h2 class="hb-title">' + esc(h.name) + '</h2></div>' +
      '<div class="hb-tools"><a class="hb-link" href="' + esc(sheet) + '">Open the sheet</a>' +
      (left ? '<button type="button" class="hb-btn" data-rollall="' + esc(h.id) + '">Roll everything left</button>' : '') +
      '</div></header>' + this.sentenceHtml(h, esc(possessive(h.name))) + this.progressHtml(h) + '</article>';
  };

  Builder.prototype.freshHtml = function () {
    var h = this.fresh, left = this.left(h).length;
    var who = '<label class="hb-name"><span class="sr-only">Your new hero’s name</span><input type="text" maxlength="200" autocomplete="off" data-hb-name placeholder="Their name" value="' + esc(h.name) + '"></label>’s';
    var whose = this.forSelf ? 'The hero will be yours, on a new ' + esc(this.typeName) + ' page.'
      : 'The hero goes on a new ' + esc(this.typeName) + ' page, not yet claimed by a player.';
    var none = this.plan.steps.length ? '' : '<p class="hb-note">' + (this.plan.systemName ? esc(this.plan.systemName) + ' doesn’t list' : 'This campaign’s game system doesn’t list') +
      ' choices for heroes yet, so a new hero only needs a name.</p>';
    return '<article class="hb-hero hb-fresh" data-hero-card="new">' +
      '<header class="hb-head"><div class="hb-names"><span class="hb-kicker">' + (this.heroes.length ? 'Another hero' : 'A new hero') + '</span></div>' +
      '<div class="hb-tools">' + (left ? '<button type="button" class="hb-btn" data-rollall="new">Roll everything left</button>' : '') +
      '<button type="button" class="hb-link" data-act="fold">Not now</button></div>' +
      '</header>' + none + (this.plan.steps.length ? this.sentenceHtml(h, who) : '<p class="hb-sentence">' + who.replace('’s', '') + '</p>') +
      this.progressHtml(h) +
      '<div class="hb-make"><button type="button" class="hb-btn hb-go" data-make>Make this hero</button><span class="hb-note">' + whose + ' Anything left blank can be chosen later.</span></div>' +
      '<p class="hb-error" role="alert" data-hb-error hidden></p></article>';
  };
  // --- The choose panel: every entry on one list, with its rules ---
  Builder.prototype.panelHtml = function () {
    var p = this.panel;
    if (!p) return '';
    var h = this.heroById(p.hero), st = this.stepByKey(p.key);
    if (!h || !st) return '';
    var steps = this.stepsFor(h), idx = steps.indexOf(st), list = st.choices, n = list.length;
    var cur = h.values[st.fieldKey], fo = (p.focus != null && list[p.focus]) || (cur && this.choiceByName(st, cur)) || list[0];
    var whose = h.isNew ? (h.name.trim() ? esc(possessive(h.name.trim())) : 'the new hero’s') : esc(possessive(h.name));
    var html = '<div class="hb-ph"><div class="hb-ph1"><span class="hb-kicker">' + (idx + 1) + ' of ' + steps.length + '</span>' +
      '<button type="button" class="hb-x" data-act="close" aria-label="Close the choices"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>' +
      '<h3 class="hb-pt">Choose ' + whose + ' ' + esc(lower(st.label)) + '</h3>' +
      (n > 6 ? '<input type="search" class="hb-q" data-hb-q placeholder="Search ' + n + ' entries" aria-label="Search the ' + esc(lower(st.label)) + ' list" autocomplete="off">' : '') +
      '<span class="hb-die">' + (sidesFor(n) === 10 ? 'Rolling uses a d10' : 'Rolling uses percentile dice, tens and ones') + ' against the numbers on the left.</span></div>';
    html += '<div class="hb-list" role="listbox" aria-label="' + esc(st.label) + '">' + list.map(function (c, i) {
      return '<button type="button" class="hb-opt" role="option" data-focus="' + i + '" aria-selected="' + (c === fo) + '">' +
        '<span class="hb-rg">' + fmtRange(n, rangeOf(n, i)) + '</span><b>' + esc(c.name) +
        (c.source === 'campaign' ? ' <span class="hb-tag">this campaign</span>' : '') + '</b>' +
        (cur && lower(cur) === lower(c.name) ? '<span class="hb-cur">chosen</span>' : '<span></span>') + '</button>';
    }).join('') + '<p class="hb-none" data-hb-none hidden>Nothing on this list matches.</p></div>';
    html += '<div class="hb-detail" data-hb-detail>' + this.detailHtml(st, fo) + '</div>';
    var chosen = cur && lower(cur) === lower(fo.name);
    html += '<div class="hb-pf"><button type="button" class="hb-btn hb-go" data-choose' + (chosen ? ' aria-pressed="true"' : '') + '>' +
      (chosen ? 'Keep ' : 'Choose ') + esc(fo.name) + '</button>' +
      '<button type="button" class="hb-btn" data-rollone>Roll for me</button></div>';
    var prev = steps[idx - 1], next = steps[idx + 1];
    html += '<div class="hb-pf hb-steps">' +
      (prev ? '<button type="button" class="hb-link" data-go="' + esc(prev.fieldKey) + '">Back to ' + esc(lower(prev.label)) + '</button>' : '<span></span>') +
      (next ? '<button type="button" class="hb-link" data-go="' + esc(next.fieldKey) + '">On to ' + esc(lower(next.label)) + '</button>' : '<span></span>') + '</div>';
    return html;
  };

  Builder.prototype.detailHtml = function (st, c) {
    var props = c.properties || {};
    var facts = Object.keys(props).filter(function (k) { return !/_display$/.test(k) && isScalar(props[k]); }).slice(0, 6);
    var body = '';
    if (c.description) {
      body = c.source === 'campaign' ? '<div class="hb-text">' + c.description + '</div>'
        : (c.description.trim() !== (c.summary || '').trim() ? '<div class="hb-text">' + prose(c.description) + '</div>' : '');
    }
    return '<h4 class="hb-dn">' + esc(c.name) + '</h4>' + (c.summary ? '<p class="hb-lede">' + esc(c.summary) + '</p>' : '') +
      (facts.length ? '<dl class="hb-facts">' + facts.map(function (k) {
        return '<div class="hb-f"><dt>' + esc(words(k)) + '</dt><dd>' + esc(props[k]) + '</dd></div>';
      }).join('') + '</dl>' : '') + body;
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

  Builder.prototype.openPanel = function (heroId, key) {
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
  };

  Builder.prototype.focusEntry = function (i) {
    this.panel.focus = i;
    var aside = this.el.querySelector('.hb-panel'), st = this.stepByKey(this.panel.key), c = st.choices[i];
    var h = this.heroById(this.panel.hero), cur = h.values[st.fieldKey], chosen = cur && lower(cur) === lower(c.name);
    aside.querySelectorAll('.hb-opt').forEach(function (o) { o.setAttribute('aria-selected', String(+o.getAttribute('data-focus') === i)); });
    var d = aside.querySelector('[data-hb-detail]');
    d.innerHTML = this.detailHtml(st, c);
    d.scrollTop = 0;
    var btn = aside.querySelector('[data-choose]');
    btn.textContent = (chosen ? 'Keep ' : 'Choose ') + c.name;
    if (chosen) btn.setAttribute('aria-pressed', 'true'); else btn.removeAttribute('aria-pressed');
  };

  Builder.prototype.filter = function (q) {
    var v = lower(q).trim(), shown = 0, aside = this.el.querySelector('.hb-panel');
    aside.querySelectorAll('.hb-opt').forEach(function (o) {
      var hit = !v || lower(o.querySelector('b').textContent).indexOf(v) >= 0;
      o.hidden = !hit;
      if (hit) shown++;
    });
    aside.querySelector('[data-hb-none]').hidden = shown > 0;
  };
  // --- Picking and saving ---
  Builder.prototype.redrawHero = function (h) {
    var card = this.el.querySelector('[data-hero-card="' + h.id + '"]');
    if (card) card.outerHTML = h.isNew ? this.freshHtml() : this.heroHtml(h);
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
      return r.json().catch(function () { return {}; }).then(function (b) { throw new Error(b.message || b.error || 'That choice could not be saved. Try again.'); });
    });
  };

  // Puts values on a hero: at once on the page, then saved for an existing
  // hero (put back if the save fails) or kept in the draft for a new one.
  Builder.prototype.apply = function (h, values) {
    var self = this, before = {};
    Object.keys(values).forEach(function (k) {
      before[k] = h.values[k] || null;
      if (values[k] == null) delete h.values[k]; else h.values[k] = values[k];
    });
    if (h.isNew) { this.saveDraft(); return Promise.resolve(before); }
    return this.send(h, values).then(function () { return before; }, function (err) {
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
    if (!b || h.isNew) return;
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

  Builder.prototype.pick = function (heroId, key, name, note) {
    var h = this.heroById(heroId), self = this, v = {};
    v[key] = name;
    // Choosing moves the panel on to the next blank, so filling in a hero is
    // one choice after another; with nothing left it closes.
    var rest = this.left(h).filter(function (s) { return s.fieldKey !== key; });
    var steps = this.stepsFor(h), at = steps.indexOf(this.stepByKey(key));
    var next = rest.filter(function (s) { return steps.indexOf(s) > at; })[0] || rest[0];
    this.panel = next ? { hero: h.id, key: next.fieldKey, focus: null } : null;
    var saving = this.apply(h, v);
    this.redrawHero(h);
    // Saved shows once the server has it; a new hero has nothing to save yet.
    saving.then(function () { self.tick(h, key); }, function () {});
    if (note) this.toast(note);
    else if (!next) this.toast(h.isNew ? 'Every choice made. Name your hero and make it.' : h.name + ' has every choice made.');
    if (!next) this.focusAfter(h);
    else { var q = this.el.querySelector('[data-hb-q]'); if (q && !this.narrow()) q.focus({ preventScroll: true }); }
    return saving;
  };

  Builder.prototype.focusAfter = function (h) {
    var el = h.isNew ? (this.el.querySelector('[data-hb-name]').value.trim() ? this.el.querySelector('[data-make]') : this.el.querySelector('[data-hb-name]'))
      : this.el.querySelector('[data-hero-card="' + h.id + '"] .hb-title');
    if (el) { if (el.classList.contains('hb-title')) el.setAttribute('tabindex', '-1'); el.focus({ preventScroll: true }); }
  };

  Builder.prototype.rollOne = function (btn) {
    var self = this, heroId = this.panel.hero, st = this.stepByKey(this.panel.key), n = st.choices.length;
    if (this.rolling) return;
    this.rolling = true;
    throwFor([{ label: st.label, n: n }], btn).then(function (faces) {
      self.rolling = false;
      var c = st.choices[indexFor(n, faces[0])];
      self.pick(heroId, st.fieldKey, c.name, 'Rolled ' + fmtFace(n, faces[0]) + ' for ' + lower(st.label) + ': ' + c.name + '.');
    });
  };

  Builder.prototype.rollAll = function (id, btn) {
    var h = this.heroById(id), self = this;
    var left = this.left(h);
    if (!left.length || this.rolling) return;
    this.rolling = true;
    throwFor(left.map(function (st) { return { label: st.label, n: st.choices.length }; }), btn).then(function (faces) {
      self.rolling = false;
      var v = {};
      // Only blanks still empty when the dice land are filled.
      left.forEach(function (st, i) {
        if (!h.values[st.fieldKey]) v[st.fieldKey] = st.choices[indexFor(st.choices.length, faces[i])].name;
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
        self.toast('The dice chose ' + names + (h.isNew ? '.' : '. Saved.'), function () {
          self.apply(h, before).then(function () { self.redrawHero(h); self.toast('Put back.'); }, function () {});
          self.redrawHero(h);
        });
      }, function () {});
    });
  };

  Builder.prototype.make = function () {
    var self = this, h = this.fresh, err = this.el.querySelector('[data-hb-error]'), btn = this.el.querySelector('[data-make]');
    var name = h.name.trim();
    function fail(msg) { err.textContent = msg; err.hidden = false; btn.disabled = false; }
    if (!name) { fail('Give your hero a name first.'); this.el.querySelector('[data-hb-name]').focus(); return; }
    if (this.busy) return;
    this.busy = true;
    btn.disabled = true;
    err.hidden = true;
    var picks = {};
    Object.keys(h.values).forEach(function (k) { picks[k] = { name: h.values[k] }; });
    var body = { name: name, picks: picks };
    var send = (window.Chronicle && Chronicle.apiFetch)
      ? Chronicle.apiFetch(this.createUrl, { method: 'POST', body: body, csrfToken: this.csrf })
      : fetch(this.createUrl, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrf }, body: JSON.stringify(body) });
    send.then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (b) { return { ok: r.ok, body: b }; });
    }).then(function (res) {
      self.busy = false;
      if (!res.ok || !res.body.url) { fail(res.body.message || res.body.error || 'The hero could not be made. Try again.'); return; }
      self.clearDraft();
      // Reload this page so the new hero joins the builder and the cast.
      var here = window.location.pathname + window.location.search;
      if (window.Chronicle && Chronicle.go) Chronicle.go(here); else window.location.reload();
    }).catch(function () { self.busy = false; fail('The hero could not be made. Check your connection and try again.'); });
  };
  // --- Wiring ---
  Builder.prototype.wire = function () {
    var self = this, el = this.el;
    this.onClick = function (e) {
      var t = e.target.closest('button');
      if (!t || !el.contains(t) || !self.plan) return;
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
      } else if (t.hasAttribute('data-make')) {
        self.make();
      } else if (t.hasAttribute('data-undo')) {
        var u = self.undo;
        self.el.querySelector('.hb-toast').hidden = true;
        self.undo = null;
        if (u) u();
      } else if (t.getAttribute('data-act') === 'close') {
        self.closePanel();
      } else if (t.getAttribute('data-act') === 'fold') {
        self.foldFresh();
      }
    };
    this.onInput = function (e) {
      if (e.target.hasAttribute('data-hb-name')) {
        self.fresh.name = e.target.value;
        self.saveDraft();
        var err = el.querySelector('[data-hb-error]');
        if (err) err.hidden = true;
      } else if (e.target.hasAttribute('data-hb-q')) {
        self.filter(e.target.value);
      }
    };
    this.onKey = function (e) {
      if (e.key === 'Escape' && self.panel) { e.preventDefault(); self.closePanel(); return; }
      if (e.key === 'Enter' && e.target.hasAttribute && e.target.hasAttribute('data-hb-name')) { e.preventDefault(); self.make(); return; }
      // Up and down move through the list; the entry read is the one picked.
      if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && e.target.closest && (e.target.closest('.hb-list') || e.target.hasAttribute('data-hb-q'))) {
        var opts = Array.prototype.filter.call(el.querySelectorAll('.hb-opt'), function (o) { return !o.hidden; });
        if (!opts.length) return;
        e.preventDefault();
        var at = opts.indexOf(el.querySelector('.hb-opt[aria-selected="true"]'));
        var to = opts[Math.max(0, Math.min(opts.length - 1, at + (e.key === 'ArrowDown' ? 1 : -1)))];
        self.focusEntry(+to.getAttribute('data-focus'));
        to.focus({ preventScroll: false });
      }
    };
    // The page's Create hero button leads to the sentence for a new hero
    // instead of the separate creator page.
    this.onCreate = function (e) {
      var a = e.target.closest && e.target.closest('[data-create-hero]');
      if (!a || !self.canCreate || !self.plan || !document.body.contains(el)) return;
      e.preventDefault();
      self.openFresh();
    };
    this.onResize = function () { self.placePanel(); };
    el.addEventListener('click', this.onClick);
    el.addEventListener('input', this.onInput);
    el.addEventListener('keydown', this.onKey);
    document.addEventListener('click', this.onCreate, true);
    window.addEventListener('resize', this.onResize);
  };

  // Create hero unfolds the sentence for a new hero; Not now folds it away
  // again, keeping the draft on this device for the next time.
  Builder.prototype.openFresh = function () {
    var grow = !this.freshOpen;
    if (grow) {
      this.freshOpen = true;
      this.saveDraft();
      this.render();
    }
    var card = this.el.querySelector('[data-hero-card="new"]'), name = this.el.querySelector('[data-hb-name]');
    if (!card) return;
    if (grow) card.classList.add('is-growing');
    card.scrollIntoView({ block: 'nearest' });
    if (name) name.focus({ preventScroll: true });
  };
  Builder.prototype.foldFresh = function () {
    this.freshOpen = false;
    this.saveDraft();
    if (this.panel && this.panel.hero === 'new') this.panel = null;
    this.render();
    var go = document.querySelector('[data-create-hero]');
    if (go) go.focus({ preventScroll: true });
  };

  Builder.prototype.unwire = function () {
    this.el.removeEventListener('click', this.onClick);
    this.el.removeEventListener('input', this.onInput);
    this.el.removeEventListener('keydown', this.onKey);
    document.removeEventListener('click', this.onCreate, true);
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
