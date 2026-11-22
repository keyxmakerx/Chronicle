/**
 * hero_creator.js — the hero creator: "Steps, with cards inside".
 *
 * Each plan step (Ancestry, Kit…) is one step of a short walk. A step deals
 * its entries as cards; a card opens its full rules (a half-height sheet on a
 * phone, the right-hand column on a computer) and choosing it plays the card
 * into its place in the picks, then deals the next step. The last step asks
 * for a name and makes the hero.
 *
 * Everything drawn comes from the entry itself, so an uploaded system whose
 * entries are only a name and some text still gets a card and a rules view:
 * - scalar properties become the facts;
 * - lists of named items, text, and nested objects become sections;
 * - a list whose items carry a numeric "cost", with a "<…>_points" budget,
 *   can be bought from (the server checks the same rule).
 *
 * Campaign entries' descriptions arrive as HTML the server sanitized (and
 * stripped of Director-only text for players); package text is plain and is
 * escaped here. Picks are kept on this device until the hero is made.
 *
 * A Director's rules shape each step (the plan's per-step `rules`, worded by
 * hero_rules_lines.js as in the hero builder): Not here entries are gone with
 * a quiet card, an Ask first entry carries a mark and is sent to the Director
 * once the hero is made, and a required step with one entry left is filled in
 * and locked.
 *
 * Rules words: {@category term|label} in package text becomes a dotted,
 * focusable term when the mount's data-terms-url (fetched once) knows it; a
 * small hover card explains it. Without the address, or when the fetch
 * fails, the words are plain text.
 *
 * A step whose list a Director let this player write in links to "Your own
 * entries"; the player's draft then shows in the list, and once the hero is
 * made the draft is kept to that hero.
 *
 * Skins: the mount's data-skin (plain, paper, ornate) picks a token set in
 * hero_creator.css; light or dark follows the site theme. Motion follows
 * html[data-motion]: Lively by default, Gentle (fades and nudges) under Calm,
 * instant under Off or prefers-reduced-motion.
 *
 * Practice mode (data-practice="true"): the same pages make a practice hero
 * instead of a real one. The plan puts back the entries the rules keep from
 * the player, marked "Not offered here"; nothing is locked, forced or asked
 * for; the hero is saved with its own draft key, from a preload for a copy or
 * a change, and the maker lands on the practice hero's page. Saving goes
 * through persist() alone, so another place to keep the result can replace it.
 *
 * Try-out mode (data-tryout="true"), for a signed-in visitor on a look-around
 * pass: the practice pages again, but the finished hero is never sent. It is
 * kept in this browser under chronicle:hero-tryout:<campaign>:<user> and the
 * server learns of it only if the owner later makes the visitor a player. The
 * plan comes from a read-only route, nothing is fetched with another method,
 * and a stored hero opens the page filled in so it can be changed.
 *
 * Whatever mode, a one-time notice another page left (chronicle:hero-handoff:
 * <campaign> in sessionStorage) is shown above the steps: it is how the
 * Characters page says why it sent a stored try-out here.
 *
 * Stage look (data-look="stage" on the mount): hero_stage.js, listed ahead of
 * this file, defines a subclass that draws the hero at the top of the page. The
 * methods it overrides are the named seams here; this file never reads the look.
 *
 * Served per-plugin at /static/plugins/entities/js/hero_creator.js from the
 * body-script registry, so it survives boosted navigation. Re-wires after
 * HTMX swaps.
 */
(function () {
  'use strict';

  function esc(v) { return Chronicle.escapeHtml(v); }

  // The campaign's own word for its game master, already HTML-safe.
  function gm() { return Chronicle.gmTerm ? Chronicle.gmTerm() : 'GM'; }

  function words(key) {
    var s = String(key).replace(/_display$/, '').replace(/[_-]+/g, ' ').trim();
    return s.charAt(0).toUpperCase() + s.slice(1);
  }

  var NO_STORAGE = 'This browser won\'t keep things, so the try-out can\'t be saved here.';

  function lower(s) { return String(s || '').toLowerCase(); }
  function cap(s) { s = String(s || ''); return s.charAt(0).toUpperCase() + s.slice(1); }
  function an(w) { return /^[aeiou]/i.test(w) ? 'an' : 'a'; }
  function hasOwn(o, k) { return Object.prototype.hasOwnProperty.call(o, k); }

  function isScalar(v) { return typeof v === 'number' || (typeof v === 'string' && v.length > 0 && v.length <= 40); }

  function namedList(v) {
    return Array.isArray(v) && v.length > 0 && v.every(function (it) {
      return it && typeof it === 'object' && typeof it.name === 'string' && it.name.trim();
    });
  }

  // The same rule as heroBuyList on the server.
  function buyList(props) {
    props = props || {};
    var keys = Object.keys(props).sort();
    var budget = 0;
    for (var i = 0; i < keys.length; i++) {
      var v = props[keys[i]];
      if (/_points$/.test(keys[i]) && typeof v === 'number' && v > 0) { budget = v; break; }
    }
    if (!budget) return null;
    for (var j = 0; j < keys.length; j++) {
      var list = props[keys[j]];
      if (namedList(list) && list.every(function (it) { return typeof it.cost === 'number' && it.cost >= 0; })) {
        return { key: keys[j], items: list, budget: budget, budgetKey: keys[i] };
      }
    }
    return null;
  }

  function itemText(it) {
    if (typeof it.description === 'string' && it.description.trim()) return it.description;
    if (it.ability && typeof it.ability.effect === 'string') return it.ability.effect;
    return '';
  }

  // --- Rules words ---------------------------------------------------------

  // The key a term is looked up by: the term as written in the markup,
  // lowercased, trimmed, runs of whitespace as one "-" ("Power roll" and
  // "power-roll" are the same word). The terms endpoint uses the same rule.
  function termKey(t) { return String(t == null ? '' : t).trim().toLowerCase().replace(/\s+/g, '-'); }

  var MARK = /\{@([a-zA-Z]+)\s+([^}|]*)(?:\|([^}]*))?\}/g;
  function shown(term, label) { return label || String(term).trim().replace(/-/g, ' '); }
  // Text with the markers resolved to their shown words, unescaped: for card
  // fronts and places a button cannot sit.
  function plain(s) {
    return String(s == null ? '' : s).replace(MARK, function (m, c, term, label) { return shown(term, label); });
  }
  // Escaped text in which each marker the glossary knows is a term button and
  // every other marker is just its shown words. Nothing in it is trusted.
  function marked(s, terms) {
    s = String(s == null ? '' : s);
    var out = '', last = 0, m;
    MARK.lastIndex = 0;
    while ((m = MARK.exec(s))) {
      var label = shown(m[2], m[3]), key = termKey(m[2]);
      out += esc(s.slice(last, m.index));
      out += terms && hasOwn(terms, key)
        ? '<button type="button" class="hc-term" data-term="' + esc(key) + '" aria-expanded="false">' + esc(label) + '</button>'
        : esc(label);
      last = m.index + m[0].length;
    }
    return out + esc(s.slice(last));
  }

  // Package prose: markers, **bold**, and one paragraph per line, with the
  // lines that read as tiers or bullets set apart.
  function prose(s, terms) {
    var lines = String(s == null ? '' : s).split(/\n+/).filter(function (l) { return l.trim(); });
    return lines.map(function (l) {
      var tier = /^(- |• )/.test(l) || /^(≤|\d+\s*[-–]\s*\d+:|\d+\+|11 or lower)/.test(l);
      var t = marked(l.replace(/^(- |• )/, ''), terms).replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>');
      return '<p' + (tier ? ' class="hc-tier"' : '') + '>' + t + '</p>';
    }).join('');
  }

  // Text for one trait or item: a single paragraph's worth, markers resolved.
  function inline(s, terms) { return marked(s, terms).replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>'); }

  // Properties that are bookkeeping, not rules text.
  var HIDE = { type: 1, omissions: 1, custom_fields: 1, provenance_display: 1, details_display: 1 };
  function hidden(k) { return HIDE[k] === 1 || /_display$/.test(k) || /quick_build$/.test(k); }

  // The scalar facts of an entry, as [label, value]; the buy list's budget is
  // "Points to spend".
  function factsOf(c) {
    var props = c.properties || {}, bl = buyList(props), out = [];
    Object.keys(props).forEach(function (k) {
      if (hidden(k)) return;
      var v = props[k];
      // A fact is a short line, so a rules marker in it shows as its words.
      if (isScalar(v)) out.push([bl && k === bl.budgetKey ? 'Points to spend' : words(k), plain(v)]);
      else if (Array.isArray(v) && v.length && v.every(function (x) { return typeof x === 'string' || typeof x === 'number'; })) {
        var j = v.map(plain).join(', ');
        if (j.length <= 40) out.push([words(k), j]);
      }
    });
    return out;
  }

  // A nested object (an ability, a signature move) as lines: its name, then
  // each field, with tier lines set apart.
  function objectHTML(o, terms) {
    var h = '', name = typeof o.name === 'string' ? o.name : '';
    if (name) h += '<p class="hc-obj"><b>' + esc(name) + '</b></p>';
    Object.keys(o).forEach(function (k) {
      var v = o[k];
      if (k === 'name' || k === 'type' || v == null || v === '') return;
      if (Array.isArray(v)) v = v.filter(function (x) { return typeof x === 'string' || typeof x === 'number'; }).join(', ');
      if (typeof v !== 'string' && typeof v !== 'number') return;
      var t = /^tier(\d)$/.exec(k);
      if (t) h += '<p class="hc-tier"><b>Tier ' + t[1] + '</b> ' + inline(v, terms) + '</p>';
      else h += '<p><span class="hc-lab">' + esc(words(k)) + '</span> ' + inline(v, terms) + '</p>';
    });
    return h;
  }

  // The rules sections an entry carries beyond its facts and buy list.
  function sectionsOf(c, terms) {
    var props = c.properties || {}, bl = buyList(props), out = [];
    Object.keys(props).sort().forEach(function (k) {
      var v = props[k];
      if (hidden(k) || (bl && bl.key === k) || isScalar(v)) return;
      var html = '';
      if (namedList(v)) {
        html = v.map(function (it) {
          return '<div class="hc-item"><b>' + esc(it.name) + '</b>' + (itemText(it) ? '<p>' + inline(itemText(it), terms) + '</p>' : '') + '</div>';
        }).join('');
      } else if (typeof v === 'string') html = prose(v, terms);
      else if (Array.isArray(v)) {
        var flat = v.filter(function (x) { return typeof x === 'string' || typeof x === 'number'; });
        if (flat.length) html = '<p>' + esc(flat.join(', ')) + '</p>';
      } else if (v && typeof v === 'object') html = objectHTML(v, terms);
      if (html) out.push({ title: words(k), html: html });
    });
    return out;
  }

  // The opening line and the rest of an entry's text. A package's summary is
  // often its description cut short with an ellipsis; then the description's
  // first paragraph is the opening line, so nothing reads twice or is cut.
  // A campaign entry's description is sanitized HTML and stays whole below.
  function splitText(c) {
    var sum = String(c.summary || '').trim(), desc = String(c.description || '').trim();
    if (!desc) return { lede: sum, body: '' };
    if (c.source === 'campaign') return { lede: sum, body: desc };
    var cut = sum.replace(/(…|\.\.\.)$/, '').trim();
    if (cut && desc.indexOf(cut) === 0) {
      if (cut !== sum) { var parts = desc.split(/\n\n+/); return { lede: parts[0], body: parts.slice(1).join('\n\n') }; }
      return { lede: sum, body: desc.slice(sum.length).trim() };
    }
    return { lede: sum, body: desc === sum ? '' : desc };
  }

  // What a hero adds up to from the picks, for the figures the picks name.
  // A figure no pick names stays out, so another system shows only its own.
  function num(v) { return typeof v === 'number' && isFinite(v) ? v : null; }
  function sumOf(parts) {
    var t = null;
    parts.forEach(function (p) { if (p != null) t = (t || 0) + p; });
    return t;
  }

  // --- Dice ----------------------------------------------------------------

  // Roll ranges: a list of up to ten is read off one d10, a longer one off
  // percentile dice, each entry owning an even share of the faces.
  function sidesFor(n) { return n <= 10 ? 10 : 100; }
  function rangeOf(n, i) { var R = sidesFor(n); return [Math.floor(i * R / n) + 1, Math.floor((i + 1) * R / n)]; }
  function indexFor(n, total) { for (var i = 0; i < n; i++) { var r = rangeOf(n, i); if (total >= r[0] && total <= r[1]) return i; } return n - 1; }
  function rollFace(sides) {
    try {
      var a = new Uint32Array(1);
      window.crypto.getRandomValues(a);
      return 1 + (a[0] % sides);
    } catch (e) { return 1 + Math.floor(Math.random() * sides); }
  }
  // The entries the dice can land on: Ask first and Not offered ones are
  // skipped, so the numbers run over what a player may simply take.
  function rollable(list) { return list.filter(function (c) { return c.state !== 'ask_first' && c.state !== 'not_offered'; }); }

  // One roll for a list of n: Chronicle's dice engine throws it on screen when
  // the site has it (it decides the face first and honours Calm and Off
  // itself); otherwise, or if it refuses, the face is drawn here at once.
  function throwFor(label, n, from) {
    var instant = function () { return rollFace(sidesFor(n)); };
    var dice = window.Chronicle && Chronicle.dice;
    if (!dice || typeof dice.roll !== 'function') return Promise.resolve(instant());
    var group = { label: label, dice: sidesFor(n) === 10 ? [{ sides: 10 }] : [{ sides: 10, tens: true }, { sides: 10 }] };
    return Promise.resolve().then(function () {
      return dice.roll([group], { from: from, describe: function (g, values) { return g.label + ': ' + (values || []).join(' and '); } });
    }).then(function (res) {
      var v = (res[0] && res[0].values) || [];
      if (sidesFor(n) === 10) return v[0] >= 1 && v[0] <= 10 ? v[0] : instant();
      // Percentile: the tens die reads 00-90, the ones die's 10 is a 0, and
      // 00 with 0 is 100.
      var t = (v[0] || 0) + ((v[1] || 0) % 10);
      return t === 0 ? 100 : t;
    }).catch(instant);
  }

  // --- Motion --------------------------------------------------------------

  // "lively" (the default), "gentle" under Calm, "still" under Off or on a
  // device that asks for less motion, as app_drawer.js reads the same signal.
  function motion() {
    var h = document.documentElement, m = h.getAttribute('data-motion');
    var rm = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (m === 'off' || rm) return 'still';
    if (m === 'calm' || h.classList.contains('nav-rm') || h.hasAttribute('data-cz-reduce')) return 'gentle';
    return 'lively';
  }

  // A signed duration, retimed by the owner's Motion speed: the site's standard
  // step (200ms by default) is the unit.
  function ms(n) {
    var u = window.Chronicle && Chronicle.motionMs ? Chronicle.motionMs('--dur-standard', 200) : 200;
    return Math.round(n * (u / 200));
  }

  // Built-in explanations for the marks a Director's rules put on entries.
  function tips() {
    return {
      ask: { name: 'Ask first', category: 'Your ' + gm() + ' decides', html: 'Your ' + gm() + ' looks at this choice before your hero plays. You can keep building meanwhile; they get a note when you finish.' },
      nh: { name: 'Not offered here', category: 'Set by your ' + gm(), html: 'Your ' + gm() + ' has kept these out of this campaign. The reason is written under each one.' },
      lock: { name: 'Everyone starts with this', category: 'Set by your ' + gm(), html: 'This step is filled in for every hero in this campaign, so there is nothing to choose.' },
      mine: { name: 'Your own entry', category: 'Written by you', html: 'You wrote this entry. Your ' + gm() + ' reads it before it counts at the table.' },
      allow: { name: 'Your ' + gm() + ' allows one', category: 'Set by your ' + gm(), html: 'Your ' + gm() + ' lets you write one of your own here. Save it as a draft, or hand it in: they accept it or ask for changes, and you can pick it for your hero meanwhile.' },
      campaign: { name: 'This campaign', category: 'Made for this table', html: 'Your ' + gm() + ' made this entry for this campaign. It is not from the game system.' }
    };
  }

  function Creator(root) {
    this.root = root;
    this.campaignId = root.getAttribute('data-campaign-id');
    this.planUrl = root.getAttribute('data-plan-url');
    this.createUrl = root.getAttribute('data-create-url');
    this.termsUrl = root.getAttribute('data-terms-url') || '';
    this.csrf = root.getAttribute('data-csrf');
    this.typeName = root.getAttribute('data-type-name') || 'character';
    this.forSelf = root.getAttribute('data-for-self') === 'true';
    this.tryout = root.getAttribute('data-tryout') === 'true';
    this.practice = this.tryout || root.getAttribute('data-practice') === 'true';
    this.backUrl = root.getAttribute('data-back-url') || '';
    this.tryoutKey = 'chronicle:hero-tryout:' + this.campaignId + ':' + (root.getAttribute('data-user-id') || '');
    this.handoff = this.takeHandoff();
    this.updateUrl = root.getAttribute('data-update-url') || '';
    this.preload = null;
    try { var pre = root.getAttribute('data-preload'); if (pre) this.preload = JSON.parse(pre); } catch (e) { /* start blank */ }
    this.draftKey = (this.practice ? 'chronicle:hero-practice:' : 'chronicle:hero-draft:') + this.campaignId;
    this.step = 0;
    this.view = 'phone';
    this.mode = 'cards';
    this.picks = {};
    this.buys = {};
    this.skipped = {};
    this.find = {};
    this.at = {};
    this.nh = {};
    this.name = '';
    this.about = {};
    this.terms = null;
    this.busy = false;
    this.sheet = null;
    this.own = null;
    this.grants = {};
  }

  Creator.prototype.load = function () {
    var self = this;
    function get(url) { return (window.Chronicle && Chronicle.apiFetch) ? Chronicle.apiFetch(url) : fetch(url, { credentials: 'same-origin' }); }
    // The glossary is read once, alongside the plan. A missing address or any
    // failure just leaves the words plain.
    var glossary = !this.termsUrl ? Promise.resolve(null) : Promise.resolve().then(function () { return get(self.termsUrl); }).then(function (r) {
      return r.ok ? r.json() : null;
    }).then(function (j) {
      var t = j && j.terms && typeof j.terms === 'object' ? j.terms : null, out = {};
      if (!t) return null;
      Object.keys(t).forEach(function (k) {
        var v = t[k];
        if (v && typeof v === 'object' && typeof v.name === 'string') out[termKey(k)] = v;
      });
      return out;
    }).catch(function () { return null; });
    get(this.planUrl).then(function (r) {
      if (!r.ok) throw new Error('plan ' + r.status);
      return r.json();
    }).then(function (plan) {
      return Promise.all([glossary, self.readOwn(plan, get)]).then(function (r) { return { plan: plan, terms: r[0] }; });
    }).then(function (res) {
      self.plan = res.plan || { steps: [] };
      self.plan.steps = self.plan.steps || [];
      self.terms = res.terms;
      self.ownFirst();
      if (self.preload) self.adopt(self.preload); else if (self.tryout) self.adopt(self.stored() || {}); else self.restore();
      self.prefill();
      self.step = self.firstStep();
      self.measure();
      self.loadFonts();
      self.render();
      self.fit();
      self.watch();
    }).catch(function () {
      var page = self.root.querySelector('.hc-loading__text');
      if (page) page.textContent = 'The creator could not open. Reload the page to try again.';
    });
  };

  // What the own-entry card and the cards of the player's own entries need
  // beyond the plan: the points their Director gave them (the plan only says a
  // permission exists) and the note a Director left on a draft. Both are read
  // from the entry API the editor uses; a failure just leaves them out.
  Creator.prototype.readOwn = function (plan, get) {
    var self = this, base = '/campaigns/' + encodeURIComponent(this.campaignId) + '/system-entries', steps = (plan && plan.steps) || [];
    if (this.tryout || !this.ownEnabled()) return Promise.resolve();
    var make = !this.practice && steps.some(function (st) { return st.canMake; });
    var mine = steps.some(function (st) { return (st.choices || []).some(function (c) { return c.mine && c.status; }); });
    var jobs = [];
    if (make) jobs.push(get(base + '/grants').then(function (r) { return r.ok ? r.json() : null; }).then(function (b) {
      ((b && b.grants) || []).forEach(function (g) { if (g.budget > 0) self.grants[g.fieldKey] = g.budget; });
    }));
    if (mine) jobs.push(get(base).then(function (r) { return r.ok ? r.json() : null; }).then(function (b) {
      var notes = {};
      ((b && b.entries) || []).forEach(function (e) { if (e.reviewNote && e.status === 'draft') notes[e.id] = e.reviewNote; });
      steps.forEach(function (st) { (st.choices || []).forEach(function (c) { if (c.mine && notes[c.entryId]) c.reviewNote = notes[c.entryId]; }); });
    }));
    return Promise.all(jobs.map(function (j) { return j.catch(function () { /* the card still works without it */ }); }));
  };
  // The own-entry editor is a separate script the registry lists first; a
  // page that lacks it simply has no "make your own".
  Creator.prototype.ownEnabled = function () { return !!(window.ChronicleOwnEntry && window.ChronicleOwnEntry.Editor); };
  // A player's own entries lead the hand: what they wrote is what they came
  // back for.
  Creator.prototype.ownFirst = function () {
    this.plan.steps.forEach(function (st) {
      var mine = [], rest = [];
      (st.choices || []).forEach(function (c) { (c.mine && c.status ? mine : rest).push(c); });
      st.choices = mine.concat(rest);
    });
  };

  // Paper and Ornate set their headings in faces the site does not load for
  // everything else; they are fetched only when a skin asks for them.
  Creator.prototype.loadFonts = function () {
    var skin = this.root.getAttribute('data-skin');
    if (skin !== 'paper' && skin !== 'ornate') return;
    if (document.getElementById('hc-fonts')) return;
    var l = document.createElement('link');
    l.id = 'hc-fonts';
    l.rel = 'stylesheet';
    l.href = 'https://fonts.googleapis.com/css2?family=Cormorant+Garamond:wght@500;600;700&family=Cinzel:wght@500;600;700&family=Source+Serif+4:opsz,wght@8..60,400;8..60,600;8..60,700&display=swap';
    document.head.appendChild(l);
  };

  // --- Draft kept on this device ---
  Creator.prototype.restore = function () {
    try {
      var own = JSON.parse(window.localStorage.getItem(this.draftKey) || 'null'), d = own || (!this.practice && this.sentenceDraft());
      if (d) { this.adopt(d); this.resumed = true; }
      // A carried-over sentence is removed as it is read, so it is kept as
      // this creator's own draft at once, or a reload would lose it.
      if (d && !own) this.save();
    } catch (e) { /* no storage: start fresh */ }
  };
  // The Characters page once offered a new hero as a sentence, kept on this
  // device as { name, values } with bought options in "<key>_choices_json".
  // A sentence left there carries over here once, as the creator's draft.
  var SENTENCE = 'chronicle:hero-builder:';
  Creator.prototype.sentenceDraft = function () {
    var raw = window.localStorage.getItem(SENTENCE + this.campaignId);
    if (!raw) return null;
    window.localStorage.removeItem(SENTENCE + this.campaignId);
    var s = JSON.parse(raw) || {}, v = s.values || {}, picks = {}, O = '_choices_json';
    Object.keys(v).forEach(function (k) {
      if (k.slice(-O.length) === O || typeof v[k] !== 'string') return;
      var opts = [];
      try { opts = JSON.parse(v[k + O] || '[]'); } catch (e) { /* none */ }
      picks[k] = { name: v[k], options: Array.isArray(opts) ? opts.filter(function (o) { return typeof o === 'string'; }) : [] };
    });
    return Object.keys(picks).length || (s.name || '').trim() ? { picks: picks, name: typeof s.name === 'string' ? s.name : '' } : null;
  };
  // Takes a draft or a preload ({picks, name}) as the starting point, keeping
  // only what the plan still offers.
  Creator.prototype.adopt = function (d) {
    var self = this;
    Object.keys(d.picks || {}).forEach(function (k) {
      var c = self.stepByKey(k) && self.choice(k, d.picks[k].name);
      if (!c) return;
      // Keep only options the entry still offers, in case the system changed.
      var bl = buyList(c.properties), opts = Array.isArray(d.picks[k].options) ? d.picks[k].options : [];
      var kept = bl ? opts.filter(function (o) { return bl.items.some(function (it) { return it.name === o; }); }) : [];
      self.picks[k] = { name: c.name, options: kept };
    });
    this.scrub();
    this.name = typeof d.name === 'string' ? d.name : '';
    var ab = d.about && typeof d.about === 'object' ? d.about : {};
    this.about = {};
    this.aboutFields().forEach(function (f) { if (typeof ab[f.key] === 'string') self.about[f.key] = ab[f.key]; });
  };
  Creator.prototype.save = function () {
    // A copy or a change starts from the server's hero, so it keeps no draft
    // that a later blank start would pick up.
    // A try-out has one place to live, written only when it is kept.
    if (this.preload || this.tryout) return;
    // Whether the last write held, for a look that tells the player.
    try { window.localStorage.setItem(this.draftKey, JSON.stringify({ picks: this.picks, name: this.name, about: this.about })); this.kept = true; } catch (e) { this.kept = false; }
  };
  // The try-out kept in this browser, or null when there is none, it is not
  // one of ours, or the browser will not say.
  Creator.prototype.stored = function () {
    try {
      var d = JSON.parse(window.localStorage.getItem(this.tryoutKey) || 'null');
      return d && d.v === 1 && typeof d === 'object' ? d : null;
    } catch (e) { this.noStorage = true; return null; }
  };
  // A notice left by the page that sent the visitor here, read once.
  Creator.prototype.takeHandoff = function () {
    var key = 'chronicle:hero-handoff:' + this.campaignId;
    try {
      var d = JSON.parse(window.sessionStorage.getItem(key) || 'null');
      window.sessionStorage.removeItem(key);
      return d && typeof d.message === 'string' ? d.message : '';
    } catch (e) { return ''; }
  };
  Creator.prototype.clearDraft = function () {
    try { window.localStorage.removeItem(this.draftKey); } catch (e) { /* ignore */ }
  };

  // --- Lookups ---
  Creator.prototype.steps = function () { return this.plan.steps; };
  Creator.prototype.stepCount = function () { return this.last() + 1; };
  // The boxes of "About your hero": who the hero is in the player's words,
  // all optional. A practice hero or a try-out never joins the party, so it
  // isn't asked.
  Creator.prototype.aboutFields = function () {
    var a = this.plan && this.plan.about;
    return !this.practice && Array.isArray(a) ? a.filter(function (f) { return f && typeof f.key === 'string'; }) : [];
  };
  Creator.prototype.hasAbout = function () { return this.aboutFields().length > 0; };
  // After the plan's steps come About your hero, when the hero's page has
  // those fields, and last Name and review.
  Creator.prototype.last = function () { return this.plan.steps.length + (this.hasAbout() ? 1 : 0); };
  Creator.prototype.isDetails = function () { return this.step >= this.last(); };
  Creator.prototype.isAbout = function () { return this.hasAbout() && this.step === this.plan.steps.length; };
  // Past the steps that pick from a list.
  Creator.prototype.isExtra = function () { return this.step >= this.plan.steps.length; };
  // What the About boxes hold, in a line: the short ones, then a count.
  Creator.prototype.aboutLine = function () {
    var self = this, short = [], long = 0;
    this.aboutFields().forEach(function (f) {
      var v = String(self.about[f.key] || '').trim();
      if (!v) return;
      if (f.long) long++; else short.push(v);
    });
    if (long) short.push(long === 1 ? (short.length ? 'and more' : 'Written') : long + ' written');
    return short.join(' · ');
  };
  Creator.prototype.cur = function () { return this.plan.steps[this.step]; };
  Creator.prototype.stepByKey = function (k) {
    for (var i = 0; i < this.plan.steps.length; i++) if (this.plan.steps[i].fieldKey === k) return this.plan.steps[i];
    return null;
  };
  Creator.prototype.choice = function (key, name) {
    var st = this.stepByKey(key);
    if (!st) return null;
    // Two parents may list an entry of the same name; the one under the
    // parent picked now wins, else the first of that name.
    var pp = st.parent && this.picks[st.parent] && this.picks[st.parent].name, first = null;
    for (var i = 0; i < st.choices.length; i++) {
      if (lower(st.choices[i].name) !== lower(name)) continue;
      if (!pp || lower(st.choices[i].parent) === lower(pp)) return st.choices[i];
      first = first || st.choices[i];
    }
    return first;
  };
  // --- Steps a system shapes: parents and counts ---

  // The entry picked at a step, whatever its position, or null.
  Creator.prototype.pickedAt = function (key) {
    var p = this.picks[key];
    return p && p.name ? this.choice(key, p.name) : null;
  };
  // How many of this step the picked parent entries allow. A later countFrom
  // entry whose picked entry defines the number overrides an earlier one;
  // known is false when none does, and the step then reads as it always did.
  Creator.prototype.allowance = function (st) {
    var out = { known: false, count: 0, by: '', note: '' }, self = this;
    (st.countFrom || []).forEach(function (cf) {
      var e = self.pickedAt(cf.step), n = e && e.properties && e.properties[cf.property];
      if (typeof n !== 'number' || n < 0) return;
      var note = cf.note && e.properties[cf.note];
      out = { known: true, count: Math.floor(n), by: e.name, note: typeof note === 'string' ? note : '' };
    });
    return out;
  };
  // The entries a step offers now: all of them, or for a dependent step only
  // those under the parent entry picked.
  Creator.prototype.pool = function (st) {
    if (!st.parent) return st.choices;
    var p = this.picks[st.parent];
    if (!p || !p.name) return [];
    return st.choices.filter(function (c) { return lower(c.parent) === lower(p.name); });
  };
  // 'wait': the parent is not picked yet. 'unused': the picked entries leave
  // this step nothing to ask (a count of 0, or a parent that lists none).
  // Otherwise 'open'.
  Creator.prototype.stepState = function (st) {
    if (st.parent) {
      var p = this.picks[st.parent];
      if (!p || !p.name) return 'wait';
      if (!this.pool(st).length && !(st.canMake && !this.practice)) return 'unused';
    }
    var a = this.allowance(st);
    return a.known && a.count === 0 ? 'unused' : 'open';
  };
  Creator.prototype.unused = function (st) { return this.stepState(st) === 'unused'; };
  // The plain sentence for a step the picked entries do not use. A system's
  // own note already says so in its words, so it stands alone when given.
  Creator.prototype.unusedLine = function (st) {
    var a = this.allowance(st);
    if (a.known && a.count === 0 && a.note) return a.note;
    var by = a.known && a.count === 0 ? a.by : (this.picks[st.parent] && this.picks[st.parent].name) || '';
    return by ? cap(an(by)) + ' ' + by + ' doesn\u2019t use ' + an(st.label) + ' ' + lower(st.label) + '.' : cap(st.label) + ' isn\u2019t used here.';
  };
  // Takes off every pick that no longer fits what is picked at its parent or
  // what its parent allows, until nothing more changes; returns the labels.
  Creator.prototype.scrub = function () {
    var self = this, gone = [], again = true, guard = 0;
    while (again && guard++ < 8) {
      again = false;
      this.plan.steps.forEach(function (st) {
        var p = self.picks[st.fieldKey];
        if (!p || !p.name) return;
        var c = self.choice(st.fieldKey, p.name);
        var fits = c && !self.unused(st) && (!st.parent || (self.picks[st.parent] && self.picks[st.parent].name && lower(c.parent) === lower(self.picks[st.parent].name)));
        if (fits) return;
        delete self.picks[st.fieldKey];
        gone.push(st.label);
        again = true;
      });
    }
    return gone;
  };
  // A required step the rules leave one entry on starts filled in.
  Creator.prototype.prefill = function () {
    var self = this;
    this.plan.steps.forEach(function (st) {
      var want = st.rules && st.rules.prefilled, c = want && self.choice(st.fieldKey, want), p = self.picks[st.fieldKey];
      if (c) self.picks[st.fieldKey] = { name: c.name, options: p && lower(p.name) === lower(c.name) ? p.options : [] };
    });
  };
  Creator.prototype.locked = function (st) {
    var want = st.rules && st.rules.prefilled, p = this.picks[st.fieldKey];
    return !!(want && p && lower(p.name) === lower(want));
  };
  // The first step a hero still has something to choose in; a step the rules
  // fill in for everyone is not a place to start. A player coming back to a
  // saved draft carries on at the first step without a pick, or at Name and
  // review when every step has one. A copy, a change or a kept try-out starts
  // at the top, where the player looks it over.
  Creator.prototype.firstStep = function () {
    var self = this, steps = this.plan.steps;
    var open = function (i) { return !self.locked(steps[i]) && !self.unused(steps[i]) && (!self.resumed || !self.picked(i)); };
    for (var i = 0; i < steps.length; i++) if (open(i)) return i;
    return this.last();
  };
  // A step is settled once it holds a pick or was skipped on purpose.
  Creator.prototype.settled = function (i) {
    var st = this.plan.steps[i];
    return !!(st && ((this.picks[st.fieldKey] && this.picks[st.fieldKey].name) || this.skipped[st.fieldKey] || this.unused(st)));
  };
  Creator.prototype.picked = function (i) {
    var st = this.plan.steps[i], p = st && this.picks[st.fieldKey];
    return p && p.name ? p.name : '';
  };
  Creator.prototype.pickedEntry = function (i) {
    var st = this.plan.steps[i], n = this.picked(i);
    return n ? this.choice(st.fieldKey, n) : null;
  };
  // The next step still open after i, wrapping once; the name step when every
  // step is settled.
  Creator.prototype.nextOpen = function (from) {
    var n = this.plan.steps.length, i;
    for (i = from + 1; i < n; i++) if (!this.settled(i)) return i;
    for (i = 0; i < n; i++) if (!this.settled(i) && i !== from) return i;
    return n;
  };
  Creator.prototype.stepWord = function (i) { return i >= this.last() ? 'Name and review' : i >= this.plan.steps.length ? 'About your hero' : this.plan.steps[i].label; };
  // A step the rules require can't be left empty; practice forces nothing.
  // The system's own expectations still show in practice: a step it needs
  // reads as not optional, though nothing refuses a hero without it.
  Creator.prototype.needed = function (st) {
    if (this.unused(st)) return false;
    var a = this.allowance(st);
    return !!st.needed || (a.known && a.count >= 1);
  };
  Creator.prototype.optional = function (st) {
    if (this.unused(st)) return true;
    return !this.needed(st) && (this.practice || !(st.rules && st.rules.required));
  };
  // What a step reads as before it has a pick.
  Creator.prototype.stateWord = function (st) {
    return this.skipped[st.fieldKey] ? 'None' : this.unused(st) ? 'Not used' : this.optional(st) ? 'Optional' : 'Not chosen yet';
  };
  Creator.prototype.missing = function () {
    var self = this;
    return this.plan.steps.map(function (st, i) { return !self.picked(i) && !self.optional(st) ? i : -1; }).filter(function (i) { return i >= 0; });
  };
  // The pick of a step that waits for the Director's yes: an Ask first entry.
  Creator.prototype.waitName = function (st) {
    if (this.practice) return '';
    var p = this.picks[st.fieldKey], c = p && p.name && this.choice(st.fieldKey, p.name);
    return c && c.state === 'ask_first' ? c.name : '';
  };
  // What a pick still needs from the Director or the rules, as a short phrase.
  Creator.prototype.pickNote = function (st) {
    if (this.practice) {
      var pk = this.picks[st.fieldKey], pc = pk && pk.name && this.choice(st.fieldKey, pk.name);
      return pc && pc.state === 'not_offered' ? 'not offered here' : pc && pc.state === 'ask_first' ? 'ask first' : '';
    }
    if (this.waitName(st)) return 'waiting for your ' + gm();
    return this.locked(st) ? window.ChronicleHeroRules.LOCK : '';
  };
  Creator.prototype.notesHTML = function (st) {
    return '<div class="hc-notes" aria-live="polite">' + window.ChronicleHeroRules.notes(st, this.waitName(st), 'hc') + '</div>';
  };
  Creator.prototype.isPicked = function (st, c) {
    var p = this.picks[st.fieldKey];
    return !!(p && p.name && lower(p.name) === lower(c.name));
  };
  // The bought options for an entry: the pick's own list once it is chosen,
  // else a list kept here so what was bought is still there if it is chosen.
  Creator.prototype.mine = function (key, c) {
    var p = this.picks[key];
    if (p && p.name && lower(p.name) === lower(c.name)) return p.options;
    var bk = key + '|' + lower(c.name);
    return this.buys[bk] || (this.buys[bk] = []);
  };
  Creator.prototype.spent = function (key, c, bl) {
    var mine = this.mine(key, c);
    return bl.items.reduce(function (n, it) { return n + (mine.indexOf(it.name) >= 0 ? it.cost : 0); }, 0);
  };

  // The entries of a step the find box lets through.
  Creator.prototype.entries = function (si) {
    var st = this.plan.steps[si], q = lower((this.find[st.fieldKey] || '').trim());
    if (this.unused(st)) return [];
    var pool = this.pool(st);
    if (!q) return pool;
    return pool.filter(function (c) {
      var f = factsOf(c).map(function (x) { return x[1]; }).join(' ');
      return lower(c.name + ' ' + plain(c.summary) + ' ' + f).indexOf(q) >= 0;
    });
  };
  Creator.prototype.curIdx = function () {
    var st = this.cur(), l = this.entries(this.step), k = st.fieldKey;
    if (this.at[k] == null) {
      var j = -1, p = this.picks[k];
      l.forEach(function (c, n) { if (p && lower(c.name) === lower(p.name)) j = n; });
      this.at[k] = j >= 0 ? j : 0;
    }
    return Math.max(0, Math.min(this.at[k], l.length - 1));
  };
  Creator.prototype.curEntry = function () { var l = this.entries(this.step); return l.length ? l[this.curIdx()] : null; };

  // The figures the picks add up to, where the picks name them.
  Creator.prototype.stats = function () {
    var self = this, o = { stamina: null, speed: null, size: null, stab: null, rec: null };
    var by = function (re) { for (var i = 0; i < self.plan.steps.length; i++) if (re.test(self.plan.steps[i].fieldKey)) return self.pickedEntry(i); return null; };
    var cls = by(/^(class|calling)$/), anc = by(/^(ancestry|race|species|heritage)$/), kit = by(/^kit$/);
    var cp = cls && cls.properties || {}, ap = anc && anc.properties || {}, kp = kit && kit.properties || {};
    o.stamina = sumOf([num(cp.starting_stamina), num(kp.stamina_bonus)]);
    o.rec = num(cp.recoveries);
    var speed = num(ap.speed);
    if (speed != null && anc) {
      var bl = buyList(ap), mine = this.mine(this.plan.steps[this.stepIndexOf(anc)].fieldKey, anc);
      if (bl) bl.items.forEach(function (it) {
        var m = mine.indexOf(it.name) >= 0 && /You have speed (\d+)/.exec(itemText(it));
        if (m) speed = Math.max(speed, +m[1]);
      });
    }
    o.speed = sumOf([speed, num(kp.speed_bonus)]);
    o.size = anc && ap.size != null ? String(ap.size) : null;
    o.stab = sumOf([num(ap.stability), num(kp.stability_bonus)]);
    return o;
  };
  Creator.prototype.stepIndexOf = function (c) {
    for (var i = 0; i < this.plan.steps.length; i++) if (this.plan.steps[i].choices.indexOf(c) >= 0) return i;
    return 0;
  };
  // A line naming the first picks, for the hero card.
  Creator.prototype.line = function (n) {
    var self = this, out = [];
    this.plan.steps.forEach(function (st, i) { if (out.length < n && self.picked(i)) out.push(self.picked(i)); });
    return out.join(' · ');
  };

  // --- The page ---

  var I = {
    chev: '<svg class="hc-ico hc-chev" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 8l5 5 5-5"/></svg>',
    x: '<svg class="hc-ico" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="M5 5l10 10M15 5L5 15"/></svg>',
    back: '<svg class="hc-ico" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12.5 4.5L7 10l5.5 5.5"/></svg>',
    next: '<svg class="hc-ico" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M7.5 4.5L13 10l-5.5 5.5"/></svg>',
    find: '<svg class="hc-ico" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><circle cx="9" cy="9" r="5.5"/><path d="M13.2 13.2L17 17"/></svg>',
    die: '<svg class="hc-ico hc-ico--die" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round" aria-hidden="true"><path d="M12 2.8l8 4.6v9.2l-8 4.6-8-4.6V7.4z"/><path d="M4 7.4l8 4.6 8-4.6M12 12v9.2"/><circle cx="12" cy="7.2" r="1.1" fill="currentColor" stroke="none"/><circle cx="7.6" cy="13.6" r="1.1" fill="currentColor" stroke="none"/><circle cx="16.4" cy="13.6" r="1.1" fill="currentColor" stroke="none"/></svg>',
    up: '<svg class="hc-ico" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12l5-5 5 5"/></svg>'
  };

  Creator.prototype.title = function () {
    return this.tryout ? 'Try the hero builder' : this.practice ? (this.updateUrl ? 'Change practice hero' : 'New practice hero') : 'New hero';
  };

  Creator.prototype.render = function () {
    var keepOwn = this.own && this.own.editor && !this.own.leaving;
    this.sheet = null;
    this.sh = null;
    this.closeTip(true);
    var desk = this.view === 'desk', cols;
    if (desk) {
      cols = '<div class="hc-cols">' + this.railHTML() + (this.isExtra()
        ? '<section class="hc-mid hc-mid--wide">' + this.extraHTML() + '</section>'
        : '<section class="hc-mid" data-mid>' + this.midInner() + '</section><aside class="hc-right" data-right aria-label="The entry">' + this.rightInner() + '</aside>') + '</div>';
    } else {
      cols = this.sheadHTML() + this.chipsBlockHTML() + '<div class="hc-body" data-body>' + this.bodyInner() + '</div>';
    }
    // A redraw keeps the height and shortness the last fit measured, so the
    // cards are laid out once at their real size rather than twice.
    // The live region outlives a redraw, or what a pick just announced (and
    // anything it cleared) would be wiped before a screen reader read it.
    var liveEl = this.root.querySelector('.hc-live');
    this.root.innerHTML = '<div class="hc-app' + (this.short ? ' is-short' : '') + '" data-view="' + this.view + '" data-step="' + this.step + '" data-mo="' + motion() + '"' + (this.fitH ? ' style="--hc-h:' + this.fitH + 'px"' : '') + '>' +
      this.topHTML(this.backHref()) + this.bandHTML() + cols + '</div>';
    if (liveEl) this.root.appendChild(liveEl);
    this.fit();
    this.after();
    // Redrawing shrinks the page for a moment, which can scroll it back; the
    // builder goes back to where it was aligned.
    if (this.aligned) this.alignTop();
    // Turning the screen between phone and computer keeps what was being written.
    if (keepOwn) this.showOwn(true);
  };

  // Where the back link in the bar goes: the page that sent the player here,
  // else the Characters page.
  Creator.prototype.backHref = function () {
    return this.backUrl || ('/campaigns/' + encodeURIComponent(this.campaignId) + '/characters');
  };
  Creator.prototype.barHTML = function (back) {
    return '<header class="hc-abar"><a class="hc-ibtn" href="' + esc(back) + '" aria-label="Back to characters">' + I.back + '</a>' +
      '<div class="hc-abar__t"><h1>' + esc(this.title()) + '</h1><small>' + esc(this.plan.systemName || 'Game system') + '</small></div></header>';
  };
  // Everything above the practice band: the bar alone here; a look that
  // seats more in the top of the page (the stage's banner) overrides it.
  Creator.prototype.topHTML = function (back) { return this.barHTML(back); };

  // Practice mode says what it is above the steps, in the page's own words.
  Creator.prototype.bandHTML = function () {
    var note = this.handoff ? '<p class="hc-band hc-band--note" role="status">' + esc(this.handoff) + '</p>' : '';
    if (this.tryout) {
      return note + '<p class="hc-band"><b>Trying it out.</b> Kept in this browser only.' +
        (this.noStorage ? ' ' + NO_STORAGE : '') + '</p>';
    }
    return note + (this.practice ? '<p class="hc-band"><b>Practice hero.</b> Not in the party, not sent to Foundry.</p>' : '');
  };

  // Phone: the one-line "Step N of M · Label" that opens the step list, and a
  // thin bar of how far along the steps are.
  Creator.prototype.sheadHTML = function () {
    var self = this, n = this.stepCount();
    var pr = this.plan.steps.map(function (s, i) {
      return '<i class="' + (self.picked(i) ? 'is-ok' : '') + (i === self.step ? ' is-cur' : '') + '"></i>';
    }).join('') + (this.hasAbout() ? '<i class="' + (this.aboutLine() ? 'is-ok' : '') + (this.isAbout() ? ' is-cur' : '') + '"></i>' : '') +
      '<i class="' + (this.isDetails() ? 'is-cur' : '') + '"></i>';
    return '<div class="hc-shead"><button class="hc-sbtn" type="button" aria-expanded="false" aria-label="All steps" data-act="steps">' +
      '<span class="hc-of">Step ' + (this.step + 1) + ' of ' + n + ' ·</span><span class="hc-nm">' + esc(this.stepWord(this.step)) + '</span>' + I.chev + '</button>' +
      '<div class="hc-prog" aria-hidden="true">' + pr + '</div></div>';
  };

  Creator.prototype.chipsBlockHTML = function () {
    return '<ul class="hc-chips" aria-label="Your picks">' + this.chipsHTML() + '</ul>';
  };
  // The row of what is picked; tapping one goes back to that step.
  Creator.prototype.chipsHTML = function () {
    var self = this, h = '';
    this.plan.steps.forEach(function (st, i) {
      var p = self.picked(i), cur = i === self.step, e = self.pickedEntry(i);
      if (!p && !cur && !self.isExtra() && !self.skipped[st.fieldKey]) return;
      var ask = e && (e.state === 'ask_first' ? !self.practice : e.state === 'not_offered');
      var on = (cur ? ' is-cur' : '') + (ask ? ' is-ask' : '');
      if (p) h += '<li><button type="button" class="hc-chip' + on + '" data-k="' + esc(st.fieldKey) + '" data-go="' + i + '"><small>' + esc(st.label) + '</small><b>' + esc(p) + '</b></button></li>';
      else if (self.skipped[st.fieldKey]) h += '<li><button type="button" class="hc-chip' + on + '" data-k="' + esc(st.fieldKey) + '" data-go="' + i + '"><small>' + esc(st.label) + '</small><b>None</b></button></li>';
      else h += '<li><button type="button" class="hc-chip is-slot' + on + '" data-k="' + esc(st.fieldKey) + '" data-go="' + i + '"><small>' + esc(st.label) + '</small><b>' + (self.unused(st) ? 'Not used' : cur ? 'Choosing…' : 'Not yet') + '</b></button></li>';
    });
    return h;
  };

  // The marks an entry wears, as words. In the open entry they are buttons that
  // explain themselves; on a card (itself a button) they are plain words that
  // still answer a hover.
  Creator.prototype.marksHTML = function (st, c, interactive) {
    var out = [];
    function mk(cls, label, tip) {
      if (interactive) out.push('<button type="button" class="hc-mark ' + cls + '" data-tip="' + tip + '" aria-expanded="false">' + label + '</button>');
      else out.push('<span class="hc-mark ' + cls + '" data-tip="' + tip + '">' + label + '</span>');
    }
    if (c.mine && c.status) mk('is-own', c.status === 'sent' ? 'Waiting for your ' + gm() : c.reviewNote ? 'Changes asked' : 'Draft', 'mine');
    else if (c.source === 'campaign') mk('is-own', 'Campaign', 'campaign');
    if (c.state === 'ask_first') mk('is-warn', 'Ask first', 'ask');
    if (c.state === 'not_offered') mk('is-quiet', 'Not offered here', 'nh');
    if (this.isPicked(st, c) && this.locked(st)) mk('is-quiet', 'Everyone starts with this', 'lock');
    return out.join('');
  };

  Creator.prototype.factsHTML = function (c, max) {
    var f = factsOf(c).slice(0, max || 12);
    if (!f.length) return '';
    return '<dl class="hc-facts">' + f.map(function (x) { return '<div><dt>' + esc(x[0]) + '</dt><dd>' + esc(x[1]) + '</dd></div>'; }).join('') + '</dl>';
  };

  Creator.prototype.introHTML = function () {
    var st = this.cur(), n = this.pool(st).length, desk = this.view === 'desk', state = this.stepState(st);
    if (state !== 'open') {
      return '<div class="hc-intro">' + (desk ? '<div class="hc-of">Step ' + (this.step + 1) + ' of ' + this.stepCount() + '</div>' : '') +
        '<h2 class="hc-disp" tabindex="-1">' + (state === 'unused' ? esc(cap(st.label)) + ': not used' : 'Choose your ' + esc(lower(st.label))) + '</h2></div>';
    }
    var al = this.allowance(st), alNote = al.known && al.count >= 1 && al.note ? ' ' + esc(al.note) : '';
    var hint = this.locked(st) ? 'Everyone starts with this. Open it to read the whole entry.' : desk ? 'Pick a card to read its rules on the right.' : 'Tap a card to read its rules.';
    return '<div class="hc-intro">' + (desk ? '<div class="hc-of">Step ' + (this.step + 1) + ' of ' + this.stepCount() + ' · ' + n + ' to choose from</div>' : '') +
      '<h2 class="hc-disp" tabindex="-1">Choose your ' + esc(lower(st.label)) + '</h2><p>' + hint + alNote + '</p></div>' + this.notesHTML(st);
  };

  Creator.prototype.toolsHTML = function () {
    var st = this.cur(), q = this.find[st.fieldKey] || '', desk = this.view === 'desk';
    var h = '<div class="hc-tools"><label class="hc-find">' + I.find + '<span class="hc-sr">Find ' + an(st.label) + ' ' + esc(lower(st.label)) + '</span>' +
      // A phone's row is narrow: the placeholder is one word there, the label
      // above still names the list in full.
      '<input type="search" data-find autocomplete="off" placeholder="' + (desk ? 'Find ' + an(st.label) + ' ' + esc(lower(st.label)) : 'Find') + '" value="' + esc(q) + '">' +
      (q ? '<button type="button" class="hc-ibtn hc-clr" data-act="clear" aria-label="Clear the search">' + I.x + '</button>' : '') + '</label>' +
      '<button type="button" class="hc-ibtn hc-die" data-act="die" aria-label="Deal a random card" title="Deal a random card">' + I.die + '</button>' +
      '<div class="hc-mode" role="group" aria-label="Show as"><button type="button" data-mode="cards" aria-pressed="' + (this.mode === 'cards') + '">Cards</button>' +
      '<button type="button" data-mode="list" aria-pressed="' + (this.mode === 'list') + '">List</button></div>';
    if (desk) h += '<span class="hc-keys"><kbd>←</kbd> <kbd>→</kbd> move · <kbd>Enter</kbd> choose</span>';
    return h + '</div>';
  };

  Creator.prototype.nhHTML = function (st) {
    return window.ChronicleHeroRules.notHere(st.rules, !!this.nh[st.fieldKey], 'hc');
  };
  Creator.prototype.showNh = function (st) {
    return !(this.find[st.fieldKey] || '').trim() && st.rules && st.rules.notHereCount;
  };
  Creator.prototype.emptyHTML = function () {
    var st = this.cur(), q = this.find[st.fieldKey] || '', state = this.stepState(st);
    if (state === 'unused') return '<div class="hc-empty">' + esc(this.unusedLine(st)) + '</div>';
    if (state === 'wait') {
      var ps = this.stepByKey(st.parent);
      return '<div class="hc-empty">Choose ' + (ps ? an(ps.label) + ' ' + esc(lower(ps.label)) : 'the earlier step') + ' first.</div>';
    }
    if (q) return '<div class="hc-empty">No ' + esc(lower(st.label)) + ' matches “' + esc(q) + '”. <button type="button" class="hc-qw" data-act="clear">Clear the search</button></div>';
    return '<div class="hc-empty">Nothing is on this list for you right now.</div>';
  };

  // "Make your own": offered while a Director's permission for this list is
  // unspent. Not in practice (writing an entry spends the one permission, which
  // belongs to the real hero) and never to a try-out guest, whose plan carries
  // no permission. Hidden while the find box is in use, as it is not a match.
  Creator.prototype.canOwn = function (st) {
    return !!(st && st.canMake && !this.practice && !this.tryout && this.ownEnabled() && !(this.find[st.fieldKey] || '').trim());
  };
  Creator.prototype.ownBudget = function (st) {
    if (this.grants[st.fieldKey]) return this.grants[st.fieldKey];
    var pkg = st.choices.filter(function (c) { return c.source !== 'campaign'; });
    return window.ChronicleOwnEntry.shapeOf(pkg).budget || 0;
  };
  Creator.prototype.ownCardHTML = function (kind) {
    var st = this.cur();
    if (!this.canOwn(st)) return '';
    var lbl = lower(st.label), pts = this.ownBudget(st), title = 'Make your own ' + esc(lbl);
    var txt = 'None of these? Write your own: a name, a few words' + (pts ? ' and ' + pts + ' points of traits' : '') + '.';
    var mk = '<span class="hc-mark is-own" data-tip="allow">Your ' + gm() + ' allows one</span>';
    if (kind === 'row') {
      return '<button type="button" class="hc-lrow hc-lrow--own" data-act="ownopen"><span class="hc-h"><b>' + title + '</b>' + mk + '</span><span class="hc-s">' + txt + '</span></button>';
    }
    if (kind === 'grid') {
      return '<div class="hc-gcard hc-gcard--own" role="button" tabindex="0" data-act="ownopen" aria-label="' + title + '"><span class="hc-top">' + mk + '</span><h3 class="hc-disp">' + title + '</h3><p class="hc-sum">' + txt + '</p>' +
        '<span class="hc-ft"><b>Start writing</b></span></div>';
    }
    return '<div class="hc-card hc-card--own" role="button" tabindex="0" data-act="ownopen" aria-label="' + title + ', start writing"><span class="hc-top">' + mk + '</span>' +
      '<h3 class="hc-disp">' + title + '</h3><p class="hc-sum">' + txt + '</p>' +
      (pts ? '<dl class="hc-facts"><div><dt>Points to spend</dt><dd>' + pts + ', set by your ' + gm() + '</dd></div></dl>' : '') + '<span class="hc-more">Start writing ' + I.up + '</span></div>';
  };

  // The "3 / 11" count in a card's top row.
  Creator.prototype.cardIx = function (i, n) { return '<span class="hc-ix">' + (i + 1) + ' / ' + n + '</span>'; };

  // Extra attributes for an entry card's opening tag; the stage look puts the
  // entry's border there. The make-your-own card and list rows never take any.
  Creator.prototype.cardAttrs = function (c) { return ''; };

  // How far a card's content runs past what it may use. A stage card with a
  // frame also keeps its foot line out of the frame, so it overrides this.
  Creator.prototype.overflow = function (card) { return card.scrollHeight - card.clientHeight; };

  Creator.prototype.cardHTML = function (c, i, n, st) {
    var pk = this.isPicked(st, c);
    return '<div class="hc-card' + (pk ? ' is-picked' : '') + '" role="button" tabindex="0" data-i="' + i + '" aria-label="' + esc(c.name) + ', read the rules"' + this.cardAttrs(c) + '>' +
      '<span class="hc-top"><span' + (pk ? ' class="hc-pk"' : '') + '>' + (pk ? 'Your ' + esc(lower(st.label)) : esc(st.label)) + '</span>' + this.marksHTML(st, c, false) + this.cardIx(i, n) + '</span>' +
      '<h3 class="hc-disp">' + esc(c.name) + '</h3><p class="hc-sum">' + esc(plain(c.summary)) + '</p>' + this.factsHTML(c, 3) +
      '<span class="hc-more">Read the rules ' + I.up + '</span></div>';
  };

  Creator.prototype.handHTML = function () {
    var st = this.cur(), l = this.entries(this.step), self = this;
    if (!l.length && !this.showNh(st) && !this.canOwn(st)) return this.emptyHTML();
    return '<div class="hc-hand" data-hand aria-label="' + esc(st.label) + ' cards">' + l.map(function (c, i) { return self.cardHTML(c, i, l.length, st); }).join('') +
      this.ownCardHTML('card') + (this.showNh(st) ? '<div class="hc-card hc-card--nh"><span class="hc-top"><span class="hc-mark is-quiet" data-tip="nh">Not offered here</span></span>' + this.nhHTML(st) + '</div>' : '') + '</div>';
  };

  Creator.prototype.listHTML = function () {
    var st = this.cur(), l = this.entries(this.step), self = this, desk = this.view === 'desk';
    if (!l.length && !this.showNh(st) && !this.canOwn(st)) return '<div class="hc-list">' + this.emptyHTML() + '</div>';
    var h = '<div class="hc-list" data-list>', ci = this.curIdx();
    l.forEach(function (c, i) {
      var pk = self.isPicked(st, c);
      h += '<button type="button" class="hc-lrow' + (pk ? ' is-picked' : '') + (desk && i === ci ? ' is-focus' : '') + '" data-i="' + i + '"><span class="hc-h"><b>' + esc(c.name) + '</b>' +
        self.marksHTML(st, c, false) + (pk ? '<span class="hc-mark is-own">Chosen</span>' : '') + '</span><span class="hc-s">' + esc(plain(c.summary)) + '</span></button>';
    });
    h += this.ownCardHTML('row');
    if (this.showNh(st)) h += '<div class="hc-nhbox">' + this.nhHTML(st) + '</div>';
    return h + '</div>';
  };

  Creator.prototype.gridHTML = function () {
    var st = this.cur(), l = this.entries(this.step), ci = this.curIdx(), self = this;
    if (!l.length && !this.showNh(st) && !this.canOwn(st)) return '<div class="hc-grid">' + this.emptyHTML() + '</div>';
    var h = '<div class="hc-grid" data-grid role="listbox" aria-label="' + esc(st.label) + '">';
    l.forEach(function (c, i) {
      var pk = self.isPicked(st, c), ft = factsOf(c).slice(0, 2).map(function (x) { return '<b>' + esc(x[0]) + '</b> ' + esc(x[1]); }).join(' · ');
      h += '<div class="hc-gcard' + (i === ci ? ' is-focus' : '') + (pk ? ' is-picked' : '') + '" role="option" tabindex="' + (i === ci ? 0 : -1) + '" aria-selected="' + (i === ci) + '" data-i="' + i + '"' + self.cardAttrs(c) + '>' +
        '<span class="hc-top"><span' + (pk ? ' class="hc-pk"' : '') + '>' + (pk ? 'Your choice' : esc(st.label)) + '</span>' + self.marksHTML(st, c, false) + '</span>' +
        '<h3 class="hc-disp">' + esc(c.name) + '</h3><p class="hc-sum">' + esc(plain(c.summary)) + '</p>' + (ft ? '<span class="hc-ft">' + ft + '</span>' : '') + '</div>';
    });
    h += this.ownCardHTML('grid');
    if (this.showNh(st)) h += '<div class="hc-gcard hc-gcard--nh"><span class="hc-top"><span class="hc-mark is-quiet" data-tip="nh">Not offered here</span></span>' + this.nhHTML(st) + '</div>';
    return h + '</div>';
  };

  // Phone, under the cards: where you are in the hand, and the way on.
  Creator.prototype.underHTML = function () {
    var st = this.cur(), l = this.entries(this.step), n = l.length, i = this.curIdx(), mid = '';
    if (this.mode === 'cards' && n) {
      if (n <= 12) { mid = '<span class="hc-dots" aria-hidden="true">'; for (var j = 0; j < n; j++) mid += '<i class="' + (j === i ? 'is-on' : '') + '"></i>'; mid += '</span>'; }
      else mid = '<span class="hc-pos"><span data-posn>' + (i + 1) + '</span> of ' + n + '</span>';
    } else mid = n || this.stepState(st) === 'open' ? '<span class="hc-pos">' + n + ' ' + (n === 1 ? 'entry' : 'entries') + '</span>' : '';
    var right = '';
    if (this.picked(this.step) || this.unused(st)) right = '<button type="button" class="hc-qw" data-act="next">Next ' + I.next + '</button>';
    else if (this.optional(st)) right = '<button type="button" class="hc-qw" data-act="skip">Skip</button>';
    return '<div class="hc-under"><button type="button" class="hc-qw" data-act="back"' + (this.step === 0 ? ' disabled' : '') + '>' + I.back +
      (this.step > 0 ? esc(this.plan.steps[this.step - 1].label) : '') + '</button>' + mid + (right || '<span class="hc-qw-sp"></span>') + '</div>';
  };

  Creator.prototype.bodyInner = function () {
    if (this.isExtra()) return this.extraHTML();
    return this.introHTML() + (this.stepState(this.cur()) === 'open' ? this.toolsHTML() : '') + (this.mode === 'cards' ? this.handHTML() : this.listHTML()) + this.underHTML();
  };
  Creator.prototype.midInner = function () {
    return this.introHTML() + (this.stepState(this.cur()) === 'open' ? this.toolsHTML() : '') + (this.mode === 'cards' ? this.gridHTML() : this.listHTML());
  };

  // --- The open entry ---

  Creator.prototype.chooseLabel = function (c, st) {
    var pk = this.isPicked(st, c), name = esc(c.name), sub = '', bl = buyList(c.properties);
    if (pk && this.locked(st)) return '✓ ' + name + ', everyone starts with this';
    if (bl) { var left = bl.budget - this.spent(st.fieldKey, c, bl); sub = left ? left + (left === 1 ? ' point' : ' points') + ' unspent' : 'all points spent'; }
    if (c.state === 'ask_first' && !this.practice) {
      return (pk ? '✓ Asking for ' : 'Ask for ') + name + ' <small>· your ' + gm() + ' is asked' + (sub ? ', ' + sub : '') + '</small>';
    }
    return (pk ? 'Keep ' : 'Choose ') + name + (sub ? ' <small>· ' + sub + '</small>' : '');
  };

  Creator.prototype.chooseDisabled = function (c, st) { return this.isPicked(st, c) && this.locked(st); };

  Creator.prototype.detHTML = function (c, st) {
    var T = this.terms, pk = this.isPicked(st, c);
    var h = '<div class="hc-det"><div class="hc-kind"><span>' + esc(st.label) + ' · ' + (c.source === 'campaign' ? 'this campaign' : esc(this.plan.systemName || 'game system')) + '</span>' +
      this.marksHTML(st, c, true) + (pk ? '<span class="hc-yours">Your choice</span>' : '') + '</div>';
    h += '<h3 class="hc-disp" id="hc-dt" tabindex="-1">' + esc(c.name) + '</h3>';
    if (c.mine && c.status && !this.tryout && this.ownEnabled()) {
      h += (c.reviewNote && c.status === 'draft' ? '<p class="hc-note"><b>Your ' + gm() + ' asked:</b> ' + esc(c.reviewNote) + '</p>' : '') +
        '<p class="hc-ownbar"><button type="button" class="hc-qw" data-act="ownedit">Edit your entry</button>' +
        '<span>' + (c.status === 'sent' ? 'Waiting for your ' + gm() + ', and you can still pick it.' : 'A draft only you and your ' + gm() + ' see. You can pick it now.') + '</span></p>';
    }
    var tx = splitText(c);
    if (tx.lede) h += '<p class="hc-lede">' + marked(tx.lede, T) + '</p>';
    if (c.state === 'ask_first') h += this.practice ? '<p class="hc-note">Ask first at this table. Picking it here is only practice.</p>'
      : '<p class="hc-note">Ask first: your ' + gm() + ' allows it, or suggests another. You can carry on with the rest of your hero meanwhile.</p>';
    if (c.state === 'not_offered') h += '<p class="hc-note">Not offered at this table. It stays a practice pick, and needs your ' + gm() + '’s yes to play.</p>';
    h += this.factsHTML(c, 12);
    var bl = buyList(c.properties);
    if (bl) h += this.buyHTML(st, c, bl);
    if (tx.body) h += '<section><h4>About</h4>' + (c.source === 'campaign' ? '<div class="hc-text">' + tx.body + '</div>' : prose(tx.body, T)) + '</section>';
    sectionsOf(c, T).forEach(function (s) { h += '<section><h4>' + esc(s.title) + '</h4>' + s.html + '</section>'; });
    return h + '</div>';
  };

  Creator.prototype.buyHTML = function (st, c, bl) {
    var self = this, mine = this.mine(st.fieldKey, c), left = bl.budget - this.spent(st.fieldKey, c, bl), pips = '';
    for (var i = 0; i < bl.budget && i < 12; i++) pips += '<i class="' + (i < bl.budget - left ? 'is-on' : '') + '"></i>';
    var qb = Array.isArray(c.properties.quick_build) && c.properties.quick_build.length;
    return '<div class="hc-buy" data-buy><div class="hc-meter"><span class="hc-big" data-left>' + left + '</span><span class="hc-lab2"><b>' + esc(words(bl.budgetKey)) + ' left</b>' +
      '<span>of ' + bl.budget + ' to spend</span></span><span class="hc-pips" aria-hidden="true">' + pips + '</span></div>' +
      '<div class="hc-buyrow" role="list">' + bl.items.map(function (it, n) { return self.tcardHTML(it, n, mine, left); }).join('') + '</div>' +
      '<div class="hc-buyx">' + (qb ? '<button type="button" class="hc-qw" data-act="quick" title="' + esc(c.properties.quick_build.join(', ')) + '">Use the suggested set</button>' : '') +
      '<button type="button" class="hc-qw" data-act="unbuy">Clear</button></div></div>';
  };
  Creator.prototype.tstate = function (it, mine, left) { var on = mine.indexOf(it.name) >= 0; return { on: on, no: !on && it.cost > left }; };
  Creator.prototype.tlabel = function (it, s, left) {
    return s.on ? 'Bought ✓' : s.no ? 'Needs ' + it.cost + (it.cost === 1 ? ' point' : ' points') + ', ' + left + ' left' : 'Buy for ' + it.cost + (it.cost === 1 ? ' point' : ' points');
  };
  Creator.prototype.tcardHTML = function (it, n, mine, left) {
    var s = this.tstate(it, mine, left);
    return '<div class="hc-tcard' + (s.on ? ' is-on' : '') + (s.no ? ' is-no' : '') + '" role="listitem" data-t="' + n + '"><div class="hc-th"><b>' + esc(it.name) + '</b><span class="hc-cost">' + it.cost + (it.cost === 1 ? ' point' : ' points') + '</span></div>' +
      '<p>' + (itemText(it) ? inline(itemText(it), this.terms) : '') + '</p>' +
      '<button type="button" class="hc-tbuy" data-buy="' + n + '" aria-pressed="' + s.on + '"' + (s.no ? ' aria-disabled="true"' : '') + '>' + this.tlabel(it, s, left) + '</button></div>';
  };

  // --- Computer: steps rail, your hero so far, the open entry ---

  // What stands above the step rows in the rail; nothing in this look.
  Creator.prototype.railHead = function () { return ''; };

  Creator.prototype.railHTML = function () {
    var self = this, h = '<aside class="hc-rail">' + this.railHead() + '<nav class="hc-steps" aria-label="Steps">';
    this.plan.steps.forEach(function (st, i) {
      var p = self.picked(i), cur = i === self.step, note = self.pickNote(st);
      h += '<button type="button" class="hc-row" data-go="' + i + '" data-k="' + esc(st.fieldKey) + '"' + (cur ? ' aria-current="step"' : '') + '><span class="hc-num' + (p ? ' is-done' : cur ? ' is-now' : '') + '">' + (p ? '✓' : i + 1) + '</span>' +
        '<span class="hc-lb"><b>' + esc(st.label) + '</b><span>' + (p ? esc(p) + (note && !self.locked(st) ? ' · ' + esc(note) : '') : self.stateWord(st)) + '</span></span></button>';
    });
    if (this.hasAbout()) {
      var a = this.plan.steps.length, al = this.aboutLine();
      h += '<button type="button" class="hc-row" data-go="' + a + '"' + (this.isAbout() ? ' aria-current="step"' : '') + '><span class="hc-num' + (al ? ' is-done' : this.isAbout() ? ' is-now' : '') + '">' + (al ? '✓' : a + 1) + '</span>' +
        '<span class="hc-lb"><b>About your hero</b><span data-about-line>' + (al ? esc(al) : 'Optional') + '</span></span></button>';
    }
    var n = this.last();
    h += '<button type="button" class="hc-row" data-go="' + n + '"' + (this.isDetails() ? ' aria-current="step"' : '') + '><span class="hc-num' + (this.isDetails() ? ' is-now' : '') + '">' + (n + 1) + '</span>' +
      '<span class="hc-lb"><b>Name and review</b><span>' + (this.name.trim() ? esc(this.name.trim()) : 'Last') + '</span></span></button></nav>';
    return h + this.sofarHTML() + '</aside>';
  };

  Creator.prototype.statsHTML = function (stacked) {
    var s = this.stats(), items = [['stamina', 'Stamina'], ['speed', 'Speed'], ['size', 'Size'], ['stab', 'Stability']];
    if (!stacked) items.push(['rec', 'Recoveries']);
    var any = items.some(function (it) { return s[it[0]] != null; });
    if (!any) return '';
    return '<div class="hc-stats">' + items.map(function (it) {
      var v = s[it[0]];
      return v == null ? '' : '<div class="hc-stat"><span>' + it[1] + '</span><b data-stat="' + it[0] + '">' + esc(v) + '</b></div>';
    }).join('') + '</div>';
  };
  Creator.prototype.sofarHTML = function () {
    var nm = this.name.trim();
    return '<div class="hc-sofar" data-sofar><h4>Your hero so far</h4><p class="hc-nm2 hc-disp">' + esc(nm || 'Unnamed hero') + '</p><p class="hc-ln">' +
      esc(this.line(2) || 'Choose something to begin') + '</p>' + this.statsHTML(true) + '</div>';
  };
  Creator.prototype.rightInner = function () {
    var c = this.curEntry(), st = this.cur();
    if (!c) return '<div class="hc-empty hc-empty--r">Nothing to read yet.</div>';
    return '<div class="hc-sc" data-rsc>' + this.detHTML(c, st) + '</div><div class="hc-foot"><button type="button" class="hc-btn hc-wide" data-act="choose"' + (this.chooseDisabled(c, st) ? ' disabled' : '') + '>' +
      this.chooseLabel(c, st) + '</button><span class="hc-hint">or press Enter on a card</span></div>';
  };

  // --- About your hero ---

  Creator.prototype.extraHTML = function () { return this.isAbout() ? this.aboutHTML() : this.reviewHTML(); };
  // Every box is optional, and each stays editable on the sheet. The
  // backstory is the one Foundry's Biography carries.
  var ABOUT_HINT = { pronouns: 'she/her, he/him, they/them…', appearance: 'Height, hair, scars, what they wear…', backstory: 'Where they come from, who they left behind…' };
  Creator.prototype.aboutHeadHTML = function () { return '<h2 class="hc-disp hc-rev__t" tabindex="-1">About your hero</h2>'; };
  Creator.prototype.aboutHTML = function () {
    var self = this, fields = this.aboutFields(), bio = fields.some(function (f) { return f.key === 'backstory'; });
    var h = '<div class="hc-rev hc-about" data-about-step>' + this.aboutHeadHTML();
    fields.forEach(function (f) {
      var id = 'hc-ab-' + esc(f.key), v = self.about[f.key] || '', ph = ABOUT_HINT[f.key] || 'Optional';
      h += '<div class="hc-field"><label for="' + id + '">' + esc(f.label) + '</label>' + (f.long
        ? '<textarea id="' + id + '" rows="' + (f.key === 'backstory' ? 6 : 3) + '" maxlength="' + (f.key === 'backstory' ? 20000 : 2000) + '" placeholder="' + esc(ph) + '" data-about="' + esc(f.key) + '">' + esc(v) + '</textarea>'
        : '<input id="' + id + '" type="text" maxlength="60" autocomplete="off" placeholder="' + esc(ph) + '" data-about="' + esc(f.key) + '" value="' + esc(v) + '">') + '</div>';
    });
    h += '<p class="hc-note hc-plain">All optional, and you can change any of it on the sheet later.' + (bio ? ' The backstory goes to Foundry’s Biography.' : '') + '</p>';
    var back = this.plan.steps.length ? this.stepWord(this.step - 1) : '';
    return h + '<div class="hc-under hc-under--about">' + (back ? '<button type="button" class="hc-qw" data-act="back">' + I.back + esc(back) + '</button>' : '<span class="hc-qw-sp"></span>') +
      '<button type="button" class="hc-btn" data-act="next">Next: Name and review</button></div></div>';
  };

  // --- Name and review ---

  Creator.prototype.heroCard = function () {
    return '<div class="hc-hero"><div class="hc-who">' + (this.practice ? 'Practice hero' : 'New hero') + '</div><h2 class="hc-disp" data-hname>' + esc(this.name.trim() || 'Unnamed hero') + '</h2>' +
      '<p class="hc-line">' + esc(this.line(3) || 'Nothing chosen yet') + '</p>' + this.statsHTML(false) + '</div>';
  };
  Creator.prototype.reviewHTML = function () {
    var self = this, desk = this.view === 'desk';
    var whose = this.tryout ? 'This stays in this browser: only you can see it, and nothing is sent to the campaign.' : this.practice ? 'This stays a practice hero: only you and the Directors see it, and it never joins the party.' : this.forSelf ? 'The hero will be yours, on a new ' + esc(this.typeName) + ' page.'
      : 'The hero goes on a new ' + esc(this.typeName) + ' page, not yet claimed by a player.';
    var none = this.plan.steps.length ? '' : '<p class="hc-empty">' + (this.plan.systemName ? esc(this.plan.systemName) + ' doesn’t list' : 'This campaign’s game system doesn’t list') +
      ' choices for heroes yet, so the creator only asks for a name' + (this.hasAbout() ? ' and a little about them' : '') + '. ' + gm() + 's can add their own entries.</p>';
    var h = '<div class="hc-rev" data-rev><h2 class="hc-disp hc-rev__t" tabindex="-1">Name your ' + (this.practice ? 'practice ' : '') + 'hero</h2>' + none + (desk ? '<div class="hc-revgrid"><div>' : '');
    h += '<div class="hc-field"><label for="hc-nm">Your hero’s name</label><input id="hc-nm" type="text" maxlength="200" autocomplete="off" placeholder="Type a name" data-name value="' + esc(this.name) + '" required></div>';
    h += this.heroCard();
    var asks = [];
    this.plan.steps.forEach(function (st) { var w = self.waitName(st); if (w) asks.push(w); });
    if (asks.length) h += '<div class="hc-callout"><b>' + esc(asks.join(' and ')) + (asks.length > 1 ? ' need' : ' needs') + ' your ' + gm() + '’s okay.</b> You can finish now; they get a note, and your hero plays once they agree.</div>';
    h += (desk ? '</div><div>' : '') + '<ul class="hc-picks hc-sum">';
    this.plan.steps.forEach(function (st, i) {
      var p = self.picked(i), c = self.pickedEntry(i), extra = [], note = self.pickNote(st);
      var bl = c && buyList(c.properties);
      if (bl) { var mine = self.mine(st.fieldKey, c); extra.push(mine.length ? mine.join(', ') : 'No ' + lower(words(bl.key)) + ' bought yet'); }
      if (note) extra.push(note);
      var miss = !p && !self.optional(st), off = !p && self.unused(st);
      h += '<li class="' + (miss ? 'is-miss' : '') + '"><span class="hc-lb"><small>' + esc(st.label) + '</small><b>' + (p ? esc(p) : off ? 'Not used' : self.optional(st) ? 'None' : 'Not chosen yet') + '</b>' +
        (extra.length ? '<em>' + esc(extra.join(' · ')) + '</em>' : '') + '</span>' +
        (self.locked(st) || off ? '' : '<button type="button" class="hc-qw" data-go="' + i + '">' + (p ? 'Change' : 'Choose') + '</button>') + '</li>';
    });
    if (this.hasAbout()) {
      var al = this.aboutLine();
      h += '<li><span class="hc-lb"><small>About your hero</small><b>' + (al ? esc(al) : 'None') + '</b></span>' +
        '<button type="button" class="hc-qw" data-go="' + this.plan.steps.length + '">' + (al ? 'Change' : 'Add') + '</button></li>';
    }
    h += '</ul><div class="hc-fin">';
    var mi = this.missing();
    if (mi.length) h += '<p class="hc-hint hc-hint--l">Still to choose: ' + esc(mi.map(function (i) { return lower(self.plan.steps[i].label); }).join(', ')) + '.</p>';
    h += '<p class="hc-note hc-plain">' + whose + (this.practice ? '' : ' You can change any of it on the sheet later.') + '</p>';
    h += '<p class="hc-error" role="alert" hidden data-error></p>';
    h += '<button type="button" class="hc-btn hc-wide" data-act="create">' + (this.tryout ? 'Keep it on this device' : this.practice ? (this.updateUrl ? 'Save changes' : 'Keep this practice hero') : 'Create hero') + '</button></div>';
    return h + (desk ? '</div></div>' : '') + '</div>';
  };

  // --- Size, view and motion ---

  function q1(sel, el) { return el.querySelector(sel); }
  function qa(sel, el) { return Array.prototype.slice.call(el.querySelectorAll(sel)); }

  Creator.prototype.app = function () { return this.root.querySelector('.hc-app'); };
  Creator.prototype.$ = function (sel, el) { return (el || this.app() || this.root).querySelector(sel); };
  Creator.prototype.$$ = function (sel, el) { return qa(sel, el || this.app() || this.root); };
  Creator.prototype.lively = function () { return motion() === 'lively'; };
  Creator.prototype.still = function () { return motion() === 'still'; };
  Creator.prototype.desk = function () { return this.view === 'desk'; };

  // The three columns need room: a desk 1280 wide with the sidebar open still gets them.
  Creator.prototype.measure = function () {
    var v = this.root.clientWidth >= 900 ? 'desk' : 'phone', changed = v !== this.view;
    this.view = v;
    return changed;
  };

  // The visible part of whatever scrolls the page: the app's main column, or
  // the window when nothing inside it scrolls.
  // The ones a person can see: a move is played only where it can be seen.
  function onScreen(list) {
    var H = window.innerHeight, W = window.innerWidth;
    return list.filter(function (x) { var b = x.getBoundingClientRect(); return b.bottom > 0 && b.top < H && b.right > 0 && b.left < W; });
  }

  function scrollerRect(el) {
    for (var e = el.parentElement; e && e !== document.body; e = e.parentElement) {
      if (/(auto|scroll)/.test(getComputedStyle(e).overflowY) && e.scrollHeight > e.clientHeight) {
        var r = e.getBoundingClientRect();
        return { top: r.top, height: e.clientHeight - dockedBar(r.top + e.clientHeight) };
      }
    }
    return { top: 0, height: window.innerHeight - dockedBar(window.innerHeight) };
  }

  // How much of the visible bottom a docked bar (The table) covers, so the
  // builder stops above it instead of running under.
  function dockedBar(viewBottom) {
    var bar = document.getElementById('tt-drawer');
    if (!bar || getComputedStyle(bar).position !== 'fixed') return 0;
    return Math.max(0, viewBottom - bar.getBoundingClientRect().top);
  }

  // The room the builder may fill, and the least height it keeps. The floor is
  // low enough on a phone that a short screen still keeps the foot above The
  // table bar.
  Creator.prototype.room = function () {
    var v = scrollerRect(this.root);
    return { height: v.height, min: this.desk() ? 560 : 400 };
  };

  // The builder fills what is visible above The table bar, so the step header
  // and the thumb-height Choose always sit on the screen.
  Creator.prototype.fit = function () {
    var app = this.app();
    if (!app) return;
    var v = this.room();
    this.fitH = Math.max(v.min, Math.floor(v.height));
    app.style.setProperty('--hc-h', this.fitH + 'px');
    // A short phone gives the cards the room the step's intro line would take.
    this.short = !this.desk() && v.height < 580;
    app.classList.toggle('is-short', this.short);
    if (this.$('[data-hand]')) { this.fitCards(); this.scrollHandTo(this.curIdx(), false); this.tilt(); }
  };

  Creator.prototype.alignTop = function () {
    var app = this.app();
    if (!app) return;
    var v = scrollerRect(this.root), top = app.getBoundingClientRect().top;
    if (Math.abs(top - v.top) > 2) app.scrollIntoView({ block: 'start' });
    this.aligned = true;
  };

  Creator.prototype.watch = function () {
    var self = this, pend = 0;
    var run = function () {
      pend = 0;
      if (!document.body.contains(self.root)) return;
      if (self.measure()) { self.render(); self.alignTop(); } else self.fit();
    };
    var kick = function () { if (!pend) pend = requestAnimationFrame(run); };
    if (window.ResizeObserver) { this.ro = new ResizeObserver(kick); this.ro.observe(this.root); }
    // The table bar is built after the page, and moves: fit again when it
    // arrives or changes size, so the foot never ends up under it.
    if (window.MutationObserver) {
      var barSeen = null;
      this.mo = new MutationObserver(function () {
        var bar = document.getElementById('tt-drawer');
        if (bar === barSeen) return;
        barSeen = bar;
        if (bar && self.ro) self.ro.observe(bar);
        kick();
      });
      this.mo.observe(document.body, { childList: true, subtree: true });
      var bar0 = document.getElementById('tt-drawer');
      if (bar0 && this.ro) { barSeen = bar0; this.ro.observe(bar0); }
    }
    this.onResize = kick;
    window.addEventListener('resize', kick);
    this.alignTop();
    requestAnimationFrame(function () { self.fit(); self.alignTop(); });
  };

  // A Web Animations call that honours the motion level: Lively plays it as
  // given, Gentle keeps only the fade (and a slide a seventh the size), Still
  // plays nothing.
  Creator.prototype.anim = function (el, kf, opt) {
    var mo = motion();
    if (!el || mo === 'still' || !el.animate) return null;
    opt = opt || {};
    if (mo === 'gentle') {
      kf = kf.map(function (k) {
        var o = {};
        for (var p in k) if (p === 'opacity' || p === 'offset') o[p] = k[p];
        if (k.transform && opt.slide && /translate/.test(k.transform)) {
          o.transform = k.transform.replace(/-?\d+(\.\d+)?px/g, function (v) { return (parseFloat(v) / 7).toFixed(1) + 'px'; }).replace(/scale\([^)]*\)|rotate\([^)]*\)/g, '');
        }
        return o;
      });
    }
    var o2 = { duration: opt.duration != null ? opt.duration : ms(280), easing: opt.easing || 'cubic-bezier(.16,1,.3,1)', fill: opt.fill || 'both', delay: opt.delay || 0 };
    if (mo === 'gentle') o2.duration = Math.min(o2.duration, 180);
    try { return el.animate(kf, o2); } catch (e) { return null; }
  };

  Creator.prototype.say = function (t) {
    var live = this.root.querySelector('.hc-live');
    if (!live) { live = document.createElement('div'); live.className = 'hc-live sr-only'; live.setAttribute('aria-live', 'polite'); this.root.appendChild(live); }
    live.textContent = '';
    setTimeout(function () { live.textContent = t; }, 30);
  };
  Creator.prototype.announce = Creator.prototype.say;

  function rel(app, r) { var o = app.getBoundingClientRect(); return { left: r.left - o.left, top: r.top - o.top, width: r.width, height: r.height }; }

  // --- The hand: snap, track, tilt ---

  Creator.prototype.after = function () {
    var hand = this.$('[data-hand]');
    if (hand) { this.setupHand(hand); this.fitCards(); this.scrollHandTo(this.curIdx(), false); this.tilt(); }
    var chips = this.$('.hc-chips');
    if (chips) { var c = chips.querySelector('.is-cur') || chips.lastElementChild; if (c) chips.scrollLeft = Math.max(0, c.offsetLeft - 16 - (chips.clientWidth - c.offsetWidth) / 2); }
    if (this.desk()) { var g = this.$('.hc-gcard.is-focus'); if (g && g.scrollIntoView) g.scrollIntoView({ block: 'nearest' }); }
  };

  // The smallest a name on a card is ever set, in px.
  var NAME_MIN = 14;

  // A card front must read whole: shorten the summary, then hide the last
  // fact, before anything is cut. Redone whenever the room changes.
  // Only the cards near the one in hand are fitted, each once for its size:
  // fitting reads layout after every change, which across a forty-card step
  // held a slow phone for seconds. The rest are fitted as the hand reaches them.
  Creator.prototype.fitCards = function (at) {
    var hand = this.$('[data-hand]');
    if (!hand) return;
    var self = this, all = qa('.hc-card', hand);
    if (at == null) { var cur = hand.querySelector('.hc-card[data-i="' + this.curIdx() + '"]'); at = Math.max(0, all.indexOf(cur)); }
    // Reads and writes are batched across the cards: reset every card, read
    // them all, shorten each by what its overflow says, then check them all.
    var jobs = all.filter(function (c, n) { return Math.abs(n - at) <= 2 && c.querySelector('.hc-sum'); })
      .map(function (c) { return { c: c, size: c.clientWidth + 'x' + c.clientHeight }; })
      .filter(function (j) { return j.c._fitted !== j.size; });
    jobs.forEach(function (j) {
      var c = j.c, sum = c.querySelector('.hc-sum'), dl = c.querySelector('.hc-facts'), h3 = c.querySelector('h3');
      j.sum = sum; j.dl = dl; j.h3 = h3; j.ix = c.querySelector('.hc-top .hc-ix');
      sum.style.webkitLineClamp = ''; sum.hidden = false; if (h3) { h3.style.fontSize = ''; h3.style.webkitLineClamp = ''; h3.style.overflowWrap = ''; h3.classList.remove('is-clamp'); } if (j.ix) j.ix.hidden = false;
      if (dl) { dl.hidden = false; qa('div', dl).forEach(function (d) { d.hidden = false; }); }
    });
    jobs.forEach(function (j) {
      j.over = self.overflow(j.c);
      j.lh = parseFloat(getComputedStyle(j.sum).lineHeight) || 24;
      j.lines = Math.round(j.sum.scrollHeight / j.lh);
      j.facts = j.dl ? qa('div', j.dl).map(function (d) { return d.offsetHeight + 1; }) : [];
      j.ixWrap = !!j.ix && j.ix.offsetTop > j.ix.parentNode.firstElementChild.offsetTop; j.ixH = j.ix ? j.ix.offsetHeight : 0;
    });
    // "Read the rules" at the foot always stays on the card: the summary and
    // facts give way first, down to the name alone on the smallest phones,
    // and last of all a long name is cut to two lines, then one.
    var steps = function (j) {
      var shown = function () { return j.dl ? qa('div', j.dl).filter(function (d) { return !d.hidden; }) : []; };
      var clamp = function (n) { return { save: Math.max(0, j.lines - n) * j.lh - Math.max(0, j.lines - (j.clamp || j.lines)) * j.lh, run: function () { j.sum.style.webkitLineClamp = n; j.clamp = n; } }; };
      var drop = function (keep) { return { save: shown().length > keep ? j.facts[shown().length - 1] + (shown().length === 1 ? 10 : 0) : 0, run: function () { var f = shown(); if (f.length > keep) f[f.length - 1].hidden = true; } }; };
      // The "n / 100" count goes first, and only when it has wrapped to a line of its own.
      return [function () { return { skip: !j.ixWrap, save: j.ixWrap ? j.ixH + 6 : 0, run: function () { if (j.ixWrap) j.ix.hidden = true; } }; }, function () { return clamp(4); }, function () { return clamp(3); }, function () { return drop(2); }, function () { return clamp(2); },
        function () { return drop(1); }, function () { return drop(0); }, function () { return clamp(1); },
        function () { return { save: 0, size: true, run: function () { if (j.h3) j.h3.style.fontSize = '24px'; } }; },
        function () { return { save: 0, run: function () { j.sum.hidden = true; } }; },
        function () { return { save: 0, size: true, run: function () { if (j.h3) j.h3.style.fontSize = '20px'; } }; },
        function () { return { save: 0, run: function () { if (j.h3) { j.h3.classList.add('is-clamp'); j.h3.style.webkitLineClamp = 2; } } }; },
        function () { return { save: 0, run: function () { if (j.h3) j.h3.style.webkitLineClamp = 1; } }; }];
    };
    jobs.forEach(function (j) {
      j.list = steps(j); j.i = 0;
      var left = j.over - 1;
      while (left > 0 && j.i < j.list.length) { var s = j.list[j.i++](); if (s.skip) continue; left -= s.save; s.run(); if (!s.save && left > 0) break; }
    });
    // A card the estimate left long is finished one step at a time.
    jobs.forEach(function (j) {
      while (j.i < j.list.length && self.overflow(j.c) > 1) j.list[j.i++]().run();
      if (j.dl) j.dl.hidden = !qa('div', j.dl).some(function (d) { return !d.hidden; });
      // A one-word name wider than the card cannot wrap, so it is set smaller
      // until it fits across; a smaller name only ever frees height.
      if (j.h3 && j.h3.scrollWidth > j.h3.clientWidth + 1) {
        var px = parseFloat(getComputedStyle(j.h3).fontSize), across = px * j.h3.clientWidth / j.h3.scrollWidth;
        j.h3.style.fontSize = Math.max(NAME_MIN, Math.floor(across)) + 'px';
        // A word too long even at the smallest size breaks inside the card,
        // set as large as lays it on two lines, and the card gives way again
        // for the lines it now takes. The steps that set a name's size are
        // passed over: they would set it larger.
        if (across < NAME_MIN) {
          j.h3.style.fontSize = Math.max(NAME_MIN, Math.min(px, Math.floor(across * 1.9))) + 'px';
          j.h3.style.overflowWrap = 'break-word';
          while (j.i < j.list.length && self.overflow(j.c) > 1) { var st = j.list[j.i++](); if (!st.size) st.run(); }
          if (j.dl) j.dl.hidden = !qa('div', j.dl).some(function (d) { return !d.hidden; });
        }
      }
      j.c._fitted = j.size;
    });
  };

  Creator.prototype.setupHand = function (hand) {
    var self = this, raf = 0;
    // The strip snaps to a card, which pulls back the browser's own small
    // scroll to a focused neighbour; the keyboard's card is brought to the
    // middle. A pointer's focus is left alone so a drag or tap is not fought.
    hand.addEventListener('focusin', function (ev) {
      var t = ev.target, c = t && t.closest ? t.closest('.hc-card') : null, keyboard = false;
      if (!c) return;
      try { keyboard = t.matches(':focus-visible'); } catch (e) { /* older engines */ }
      if (keyboard) self.scrollHandTo(qa('.hc-card', hand).indexOf(c), true);
    });
    hand.addEventListener('scroll', function () {
      if (raf) return;
      raf = requestAnimationFrame(function () {
        raf = 0;
        self.tilt();
        var cards = qa('.hc-card[data-i]', hand);
        if (!cards.length) return;
        var mid = hand.scrollLeft + hand.clientWidth / 2, best = 0, bd = 1e9;
        cards.forEach(function (c, i) { var d = Math.abs(c.offsetLeft + c.offsetWidth / 2 - mid); if (d < bd) { bd = d; best = i; } });
        var k = self.cur() && self.cur().fieldKey;
        if (k && best !== self.at[k]) { self.at[k] = best; self.refreshUnder(); }
        self.fitCards(qa('.hc-card', hand).indexOf(cards[best]));
      });
    }, { passive: true });
  };

  // Neighbours sit slightly back so the centre card reads as the one in hand.
  Creator.prototype.tilt = function () {
    var hand = this.$('[data-hand]'), self = this;
    if (!hand) return;
    var mid = hand.scrollLeft + hand.clientWidth / 2, lively = this.lively(), still = this.still();
    // Every position is read before any card is restyled, so the hand lays
    // out once rather than once a card.
    var cards = qa('.hc-card', hand).filter(function (c) { return !c.classList.contains('is-dealing'); });
    var ds = cards.map(function (c) { return Math.min(1, Math.abs(c.offsetLeft + c.offsetWidth / 2 - mid) / (c.offsetWidth + 12)); });
    cards.forEach(function (c, n) {
      var d = ds[n];
      c.style.transform = lively ? 'scale(' + (1 - 0.05 * d).toFixed(3) + ')' : '';
      c.style.opacity = still ? '' : (1 - 0.35 * d).toFixed(3);
    });
  };

  Creator.prototype.scrollHandTo = function (i, smooth) {
    var hand = this.$('[data-hand]');
    if (!hand) return;
    var c = qa('.hc-card', hand)[i];
    if (!c) return;
    var x = c.offsetLeft - (hand.clientWidth - c.offsetWidth) / 2;
    if (smooth && !this.still() && hand.scrollTo) hand.scrollTo({ left: x, behavior: this.lively() ? 'smooth' : 'auto' }); else hand.scrollLeft = x;
  };

  Creator.prototype.refreshUnder = function () {
    var u = this.$('.hc-under');
    if (u) u.outerHTML = this.underHTML();
  };

  // Redraws the cards (or rows) after the find box or the Cards/List switch.
  Creator.prototype.refreshList = function () {
    var b = this.$('[data-body]') || this.$('[data-mid]');
    if (!b) return;
    var old = this.$('.hc-hand, .hc-list, .hc-grid, .hc-body > .hc-empty, [data-mid] > .hc-empty', b);
    var tmp = document.createElement('div');
    tmp.innerHTML = this.desk() ? (this.mode === 'cards' ? this.gridHTML() : this.listHTML()) : (this.mode === 'cards' ? this.handHTML() : this.listHTML());
    var nu = tmp.firstElementChild;
    if (old && old.parentNode) old.parentNode.replaceChild(nu, old);
    if (!this.desk()) this.refreshUnder();
    if (this.$('[data-hand]')) { this.setupHand(this.$('[data-hand]')); this.fitCards(); this.scrollHandTo(this.curIdx(), false); this.tilt(); }
    if (this.desk()) this.refreshRight();
    this.anim(nu, [{ opacity: 0 }, { opacity: 1 }], { duration: ms(90), easing: 'linear', fill: 'none' });
  };

  Creator.prototype.refreshRight = function () {
    var r = this.$('[data-right]');
    if (!r || (this.own && this.desk())) return;
    r.innerHTML = this.rightInner();
    this.anim(this.$('.hc-det', r), [{ opacity: 0, transform: 'translateY(6px)' }, { opacity: 1, transform: 'none' }], { duration: ms(200), fill: 'none', slide: true });
  };

  // Desk: focus moves between cards with the arrows; the right column shows
  // the entry the focus is on.
  Creator.prototype.setFocus = function (i, scroll) {
    var self = this;
    if (this.own) { this.leaveOwn(function () { self.setFocus(i, scroll); }); return; }
    var cards = this.$$('.hc-gcard[data-i]'), st = this.cur();
    if (!cards.length) {
      var rows = this.$$('.hc-lrow[data-i]');
      if (!rows.length) return;
      i = Math.max(0, Math.min(i, rows.length - 1));
      this.at[st.fieldKey] = i;
      rows.forEach(function (r, n) { r.classList.toggle('is-focus', n === i); });
      this.refreshRight();
      return;
    }
    i = Math.max(0, Math.min(i, cards.length - 1));
    var changed = i !== this.curIdx();
    this.at[st.fieldKey] = i;
    cards.forEach(function (c, n) { var on = n === i; c.classList.toggle('is-focus', on); c.tabIndex = on ? 0 : -1; c.setAttribute('aria-selected', String(on)); });
    if (scroll !== false) {
      cards[i].focus({ preventScroll: true });
      if (cards[i].scrollIntoView) cards[i].scrollIntoView({ block: 'nearest', behavior: this.still() ? 'auto' : 'smooth' });
    }
    if (changed) this.refreshRight();
  };
  Creator.prototype.gridCols = function () {
    var g = this.$('[data-grid]');
    return g ? getComputedStyle(g).gridTemplateColumns.split(' ').length : 1;
  };

  // --- The sheet (phone) ---

  // A half-height sheet over the cards: drag up or scroll for the rest, swipe
  // down or Esc to put it away. Everything behind it is inert while it is open.
  // own: the player's own entry editor fills the sheet in place of an entry's
  // rules, opens full height (it is a form, not a read) and has no Choose; its
  // Save draft and Hand it in take the foot. own.instant is a remount after a
  // redraw, with nothing to animate.
  Creator.prototype.openSheet = function (i, own) {
    var app = this.app(), c = own ? null : this.entries(this.step)[i], st = this.cur(), self = this;
    if (!app || (!c && !own)) return;
    if (!own) this.at[st.fieldKey] = i;
    this.closeSheet(true);
    var scrim = document.createElement('div'); scrim.className = 'hc-scrim'; scrim.setAttribute('data-scrim', '');
    // Not aria-modal: Choose (or the editor's foot) is a sibling of the sheet,
    // and a reader that honours aria-modal could not reach it. setInert takes
    // everything else out of reach instead.
    var sh = document.createElement('div'); sh.className = 'hc-sheet'; sh.setAttribute('role', 'dialog');
    if (own) sh.setAttribute('aria-label', 'Your own ' + lower(st.label)); else sh.setAttribute('aria-labelledby', 'hc-dt');
    sh.innerHTML = '<div class="hc-grab" aria-hidden="true"><i></i></div><button type="button" class="hc-ibtn hc-x" data-act="closesheet" aria-label="Close">' + I.x + '</button><div class="hc-sc" data-ssc>' + (own ? '<div data-ownbody></div>' : this.detHTML(c, st)) + '</div>';
    var ft = document.createElement('div'); ft.className = 'hc-sfoot';
    if (own) ft.setAttribute('data-ownfoot', '');
    else ft.innerHTML = '<button type="button" class="hc-btn hc-wide" data-act="choose"' + (this.chooseDisabled(c, st) ? ' disabled' : '') + '>' + this.chooseLabel(c, st) + '</button><span class="hc-hint" data-shint>Drag up for the full rules</span>';
    app.appendChild(scrim); app.appendChild(sh); app.appendChild(ft);
    var avail = app.clientHeight;
    this.sh = { H: avail - 58, y: 0, half: 0, el: sh, ft: ft, scrim: scrim, own: !!own,
      from: own ? this.$('[data-act="ownopen"]') : (this.$$('.hc-hand .hc-card[data-i]')[i] || this.$$('.hc-lrow[data-i]')[i]) };
    this.sh.half = Math.round(this.sh.H - avail * 0.6);
    sh.style.height = this.sh.H + 'px';
    var rest = own ? 0 : this.sh.half;
    this.sheet = own ? 'full' : 'half';
    this.setY(rest, false);
    requestAnimationFrame(function () { scrim.classList.add('is-on'); });
    var s = this.sh, mo = motion();
    if (own && own.instant) mo = 'still';
    if (mo === 'lively') {
      sh.animate([{ transform: 'translateY(' + s.H + 'px)' }, { transform: 'translateY(' + rest + 'px)' }], { duration: ms(360), easing: 'cubic-bezier(.22,.8,.24,1)' });
      ft.animate([{ transform: 'translateY(' + (s.H - rest) + 'px)' }, { transform: 'none' }], { duration: ms(360), easing: 'cubic-bezier(.22,.8,.24,1)' });
    } else if (mo === 'gentle') {
      sh.animate([{ opacity: 0, transform: 'translateY(' + (rest + 12) + 'px)' }, { opacity: 1, transform: 'translateY(' + rest + 'px)' }], { duration: ms(180), easing: 'ease-out' });
      ft.animate([{ opacity: 0 }, { opacity: 1 }], { duration: ms(180), easing: 'ease-out' });
    }
    var card = own ? this.$('.hc-hand [data-act="ownopen"]') : this.$$('.hc-hand .hc-card[data-i]')[i]; if (card) card.classList.add('is-reading');
    this.setInert(true);
    this.wireSheet(sh);
    if (own) {
      this.say('Make your own ' + lower(st.label) + ' opened. Save draft and Hand it in are at the bottom.');
      return;
    }
    // Focus lands on Choose with a keyboard or mouse, and on the title with a
    // finger so no button looks pressed.
    var coarse = window.matchMedia && window.matchMedia('(pointer: coarse)').matches;
    var target = coarse ? sh.querySelector('#hc-dt') : ft.querySelector('[data-act="choose"]:not([disabled])') || sh.querySelector('#hc-dt');
    if (target) target.focus({ preventScroll: true });
    this.say(c.name + ' opened. Choose is at the bottom.');
  };

  Creator.prototype.setInert = function (on) {
    this.$$('.hc-abar, .hc-stgw, .hc-band, .hc-shead, .hc-chips, .hc-body').forEach(function (e) { if (on) e.setAttribute('inert', ''); else e.removeAttribute('inert'); });
  };

  Creator.prototype.setY = function (y, animate) {
    var s = this.sh;
    if (!s) return;
    s.y = y;
    s.el.style.transition = animate && !this.still() ? 'transform ' + (this.lively() ? '360ms cubic-bezier(.22,.8,.24,1)' : '160ms ease-out') : 'none';
    s.el.style.transform = 'translateY(' + y + 'px)';
    var hint = this.$('[data-shint]');
    if (hint) hint.textContent = y > 4 ? 'Drag up for the full rules' : 'Drag down to make it smaller';
  };
  Creator.prototype.setSheet = function (state) { this.sheet = state; if (this.sh) this.setY(state === 'full' ? 0 : this.sh.half, true); };

  Creator.prototype.closeSheet = function (now, cb, refocus) {
    var s = this.sh, self = this;
    // Writing is never thrown away on a stray swipe: the editor asks first.
    if (s && s.own && !now && this.own && !this.own.leaving) {
      this.own.editor.requestClose(function () { self.own.leaving = true; self.closeSheet(false, cb, refocus); });
      return;
    }
    this.closeTip(true);
    this.$$('.hc-card.is-reading').forEach(function (c) { c.classList.remove('is-reading'); });
    if (!s) { if (cb) cb(); return; }
    var i = this.cur() ? this.curIdx() : 0;
    this.sh = null; this.sheet = null;
    this.setInert(false);
    var gone = function () {
      s.el.remove(); s.scrim.remove(); s.ft.remove();
      if (s.own) self.endOwn();
      if (refocus) { var c = s.own ? self.$('[data-act="ownopen"]') : self.$$('.hc-hand .hc-card[data-i], .hc-lrow[data-i]')[i]; if (c) c.focus({ preventScroll: true }); }
      if (cb) cb();
    };
    if (now || this.still()) { gone(); return; }
    s.scrim.classList.remove('is-on');
    var lv = this.lively(), opt = { duration: lv ? 220 : 140, easing: 'cubic-bezier(.4,0,1,1)', fill: 'forwards' };
    s.ft.animate(lv ? [{ transform: 'none' }, { transform: 'translateY(' + (s.H - s.y + 10) + 'px)' }] : [{ opacity: 1 }, { opacity: 0 }], opt);
    var a = s.el.animate(lv ? [{ transform: 'translateY(' + s.y + 'px)' }, { transform: 'translateY(' + (s.H + 10) + 'px)' }] : [{ opacity: 1 }, { opacity: 0 }], opt);
    a.onfinish = gone;
  };

  Creator.prototype.wireSheet = function (sh) {
    var self = this, s = this.sh, grab = sh.querySelector('.hc-grab'), head = sh.querySelector('.hc-kind'), drag = null;
    function down(ev) {
      if (ev.target.closest('button')) return;
      drag = { y0: ev.clientY, s0: s.y, t: performance.now(), y: ev.clientY, v: 0 };
      sh.style.transition = 'none';
      try { ev.target.setPointerCapture(ev.pointerId); } catch (x) { /* a pointer that cannot be captured still drags */ }
    }
    function move(ev) {
      if (!drag) return;
      var dy = ev.clientY - drag.y0, now = performance.now();
      drag.v = (ev.clientY - drag.y) / Math.max(1, now - drag.t); drag.y = ev.clientY; drag.t = now;
      var y = drag.s0 + dy; if (y < 0) y = y / 3;
      s.y = y; sh.style.transform = 'translateY(' + y + 'px)';
    }
    function up() {
      if (!drag) return;
      var v = drag.v; drag = null;
      if (v < -0.45) return self.setSheet('full');
      if (v > 0.6) return self.sheet === 'full' && s.y < s.half ? self.setSheet('half') : self.closeSheet(false, null, true);
      if (s.y > s.half + (s.H - s.half) * 0.35) return self.closeSheet(false, null, true);
      self.setSheet(s.y < s.half / 2 ? 'full' : 'half');
    }
    [grab, head].forEach(function (el) {
      if (!el) return;
      el.style.touchAction = 'none';
      el.addEventListener('pointerdown', down); el.addEventListener('pointermove', move); el.addEventListener('pointerup', up); el.addEventListener('pointercancel', up);
    });
    var sc = sh.querySelector('.hc-sc');
    sc.addEventListener('scroll', function () { if (self.sheet === 'half' && sc.scrollTop > 6) self.setSheet('full'); self.followTip(); }, { passive: true });
    sc.addEventListener('wheel', function (ev) { if (self.sheet === 'half' && ev.deltaY > 0) { ev.preventDefault(); self.setSheet('full'); } }, { passive: false });
  };

  // --- The player's own entry ---

  // Opens the editor for a new entry, or for the player's own entry c. It
  // takes the sheet on a phone and the right pane on a computer; the form,
  // its checks and Save draft / Hand it in are the shared editor's.
  Creator.prototype.openOwn = function (c) {
    var self = this, st = this.cur(), Own = window.ChronicleOwnEntry;
    if (!Own || !Own.Editor || !st || this.busy) return;
    this.endOwn();
    var desk = this.desk();
    var ed = new Own.Editor({
      campaignId: this.campaignId, csrf: this.csrf, step: st, entry: c || null, closeButton: desk,
      say: function (t) { self.say(t); },
      onSaved: function (e, mode, isNew) { self.ownSaved(st, c, e, mode, isNew); },
      onClose: function () { self.closeOwn(); }
    });
    this.own = { editor: ed, key: st.fieldKey };
    ed.start();
    this.showOwn(false);
  };

  // Puts the editor on screen (again, after a redraw when instant).
  Creator.prototype.showOwn = function (instant) {
    var self = this, o = this.own;
    if (!o) return;
    if (this.desk()) {
      var r = this.$('[data-right]');
      if (!r) return;
      r.innerHTML = '<div class="hc-sc" data-rsc><div data-ownbody></div></div><div class="hc-foot" data-ownfoot></div>';
      this.$$('.hc-gcard.is-focus').forEach(function (g) { g.classList.remove('is-focus'); g.setAttribute('aria-selected', 'false'); });
      var oc = this.$('.hc-gcard--own'); if (oc) oc.classList.add('is-focus');
      this.anim(r.firstElementChild, [{ opacity: 0, transform: 'translateY(6px)' }, { opacity: 1, transform: 'none' }], { duration: ms(200), fill: 'none', slide: true });
    } else {
      this.openSheet(null, { instant: !!instant });
    }
    o.editor.mount(this.$('[data-ownbody]'), this.$('[data-ownfoot]'));
    if (!instant) o.editor.focusFirst();
  };

  Creator.prototype.endOwn = function () {
    if (!this.own) return;
    this.own.editor.destroy();
    this.own = null;
  };

  // Gets out of the editor on the computer (a phone's sheet closes through
  // closeSheet), asking first when something unsaved would be lost.
  Creator.prototype.leaveOwn = function (next) {
    var self = this;
    if (!this.own) { next(); return; }
    this.own.editor.requestClose(function () { self.endOwn(); self.refreshRight(); next(); });
  };

  Creator.prototype.closeOwn = function () {
    if (this.desk()) {
      this.endOwn();
      this.refreshRight();
      var oc = this.$('.hc-gcard--own, .hc-lrow--own'); if (oc) oc.focus({ preventScroll: true });
    } else this.closeSheet(false, null, true);
  };

  // A saved entry joins the step's list at the front, marked Draft or Waiting
  // for the Director, and can be chosen for the hero like any other. A new one
  // spends the permission, so the make card goes. If it was the pick and its
  // name changed, the pick follows.
  Creator.prototype.ownSaved = function (st, was, e, mode, isNew) {
    var self = this, Own = window.ChronicleOwnEntry, ch = Own.Editor.choiceOf(e);
    var at = -1;
    st.choices.forEach(function (c, n) { if (c.entryId === e.id) at = n; });
    if (at >= 0) {
      var old = st.choices[at], p = this.picks[st.fieldKey];
      if (p && lower(p.name) === lower(old.name)) { p.name = ch.name; this.save(); }
      if (old.state) ch.state = old.state;
      st.choices.splice(at, 1);
    }
    st.choices.unshift(ch);
    if (isNew) st.canMake = false;
    this.at[st.fieldKey] = 0;
    this.find[st.fieldKey] = '';
    var said = ch.name + (mode === 'send' ? ' handed in. It waits for your ' + gm() + ', and you can still choose it.' : ' saved as a draft. You can choose it now.');
    var finish = function () {
      if (self.cur() !== st) return;
      self.own = null;
      var tl = self.$('.hc-tools'); if (tl) tl.outerHTML = self.toolsHTML();
      self.refreshList();
      var c0 = self.$$('.hc-hand .hc-card[data-i], .hc-gcard[data-i]')[0] || self.$$('.hc-lrow[data-i]')[0];
      if (c0) {
        if (!self.desk() && self.mode === 'cards') self.scrollHandTo(0, true);
        c0.classList.remove('is-flash'); void c0.offsetWidth;
        setTimeout(function () { c0.classList.add('is-flash'); }, self.lively() ? 300 : 0);
      }
      self.say(said);
    };
    if (this.desk()) { this.own.leaving = true; this.endOwn(); finish(); }
    else { this.own.leaving = true; this.closeSheet(false, finish); }
  };

  // --- The step list (phone) ---

  Creator.prototype.toggleDrape = function (force) {
    var d = this.$('[data-drape]'), btn = this.$('.hc-sbtn'), self = this, app = this.app();
    var open = force != null ? force : !d;
    if (!open) {
      if (!d) return;
      if (btn) btn.setAttribute('aria-expanded', 'false');
      var sc = this.$('[data-dscrim]');
      if (sc) sc.classList.remove('is-on');
      var a = this.anim(d, [{ clipPath: 'inset(0 0 0 0)' }, { clipPath: 'inset(0 0 100% 0)' }], { duration: ms(150), easing: 'cubic-bezier(.4,0,1,1)' });
      var done = function () { d.remove(); if (sc) sc.remove(); };
      if (a) a.onfinish = done; else done();
      return;
    }
    if (d || !app) return;
    this.closeSheet(true);
    var head = rel(app, q1('.hc-shead', app).getBoundingClientRect()), y = head.top + head.height;
    var scrim = document.createElement('div'); scrim.className = 'hc-scrim'; scrim.setAttribute('data-dscrim', ''); scrim.setAttribute('data-act', 'drape'); scrim.style.top = y + 'px';
    d = document.createElement('div'); d.className = 'hc-drape'; d.setAttribute('data-drape', ''); d.setAttribute('role', 'group'); d.setAttribute('aria-label', 'All steps'); d.style.top = y + 'px';
    // With many steps the list stops at the app's foot and scrolls inside itself.
    d.style.maxHeight = 'calc(100% - ' + y + 'px)';
    var h = '';
    for (var i = 0; i <= this.last(); i++) {
      var st = this.plan.steps[i], ab = !st && i < this.last(), p = ab ? this.aboutLine() : this.picked(i), note = st && this.pickNote(st);
      h += '<button type="button" class="hc-row" data-go="' + i + '"' + (i === this.step ? ' aria-current="step"' : '') + '><span class="hc-num' + (p ? ' is-done' : i === this.step ? ' is-now' : '') + '">' + (p ? '✓' : i + 1) + '</span><span class="hc-lb"><b>' + esc(this.stepWord(i)) + '</b><span>' +
        (i === this.last() ? (this.name.trim() ? esc(this.name.trim()) : 'Give your hero a name, then check everything') : ab ? (p ? esc(p) : 'Optional') : p ? esc(p) + (note && !this.locked(st) ? ' · ' + esc(note) : '') : this.stateWord(st)) + '</span></span></button>';
    }
    d.innerHTML = h;
    app.appendChild(scrim); app.appendChild(d);
    if (btn) btn.setAttribute('aria-expanded', 'true');
    requestAnimationFrame(function () { scrim.classList.add('is-on'); });
    this.anim(d, [{ clipPath: 'inset(0 0 100% 0)' }, { clipPath: 'inset(0 0 0 0)' }], { duration: ms(200) });
    var cur = d.querySelector('[aria-current]');
    if (cur) {
      cur.focus({ preventScroll: true });
      // The drape's own scroll, so no other scroller moves with it.
      d.scrollTop = Math.max(0, cur.offsetTop - (d.clientHeight - cur.offsetHeight) / 2);
    }
  };

  // --- Choosing ---

  // Makes an entry the step's pick, keeping what was bought from it.
  Creator.prototype.commitPick = function (st, c) {
    var arr = this.mine(st.fieldKey, c);
    this.picks[st.fieldKey] = { name: c.name, options: arr };
    delete this.skipped[st.fieldKey];
    // A pick elsewhere may no longer fit what this one changed (a subclass
    // under another class, a kit a class does not use).
    this.cleared = this.scrub();
    this.save();
  };

  Creator.prototype.choose = function (i) {
    if (this.busy || this.isExtra()) return;
    var l = this.entries(this.step), st = this.cur(), si = this.step;
    if (i == null) i = this.curIdx();
    var c = l[i];
    if (!c || this.chooseDisabled(c, st)) return;
    this.commitPick(st, c);
    var nx = this.nextOpen(si);
    this.say(c.name + ' chosen' + (this.cleared && this.cleared.length ? '. Cleared ' + lower(this.cleared.join(' and ')) + ', which no longer fit.' : ''));
    this.root.querySelector('.hc-app').setAttribute('data-mo', motion());
    if (this.desk()) return this.chooseDesk(c, st, si, i, nx);
    var src = this.mode === 'cards' ? this.$$('.hc-hand .hc-card[data-i]')[i] : this.$$('.hc-lrow[data-i]')[i];
    var chip = this.flightTarget(st), self = this;
    if (!this.lively() || !src || !chip) {
      this.closeSheet(false, function () { self.goStep(nx, 'fwd', st.fieldKey); });
      return;
    }
    this.busy = true;
    this.prepFlight(chip);
    var app = this.app(), from = rel(app, src.getBoundingClientRect()), to = rel(app, chip.getBoundingClientRect());
    this.closeSheet(false);
    var g = this.ghost(c, st, from);
    src.style.visibility = 'hidden';
    onScreen(this.$$('.hc-hand .hc-card, .hc-lrow')).forEach(function (x) {
      if (x !== src) x.animate([{ opacity: getComputedStyle(x).opacity, transform: x.style.transform || 'none' }, { opacity: 0, transform: 'translateX(-28px) ' + (x.style.transform || '') }], { duration: ms(180), easing: 'cubic-bezier(.4,0,1,1)', fill: 'forwards' });
    });
    this.$$('[data-body] .hc-intro, [data-body] .hc-notes, [data-body] .hc-tools, [data-body] .hc-under').forEach(function (x) { x.animate([{ opacity: 1 }, { opacity: 0 }], { duration: ms(160), easing: 'ease-in', fill: 'forwards' }); });
    var fl = this.flight(g, from, to, ms(460)), seq = this.navSeq;
    // A step the player presses mid-flight wins over the move to the next one.
    setTimeout(function () { if (self.navSeq === seq) self.goStep(nx, 'fwd', null, true); }, ms(300));
    fl.onfinish = function () {
      g.remove(); self.busy = false;
      self.landed(st);
      var sh = self.$('.hc-shead'); if (sh) sh.outerHTML = self.sheadHTML();
    };
  };

  // Where a chosen card flies on a phone: its chip. The chip row is brought
  // to it first, so it is on screen when the card arrives.
  Creator.prototype.flightTarget = function (st) { return this.$('.hc-chips .hc-chip[data-k="' + st.fieldKey + '"]'); };
  Creator.prototype.prepFlight = function (chip) {
    var chips = this.$('.hc-chips');
    chips.style.scrollBehavior = 'auto';
    chips.scrollLeft = Math.max(0, chip.offsetLeft - 16);
  };
  // The card has landed: the chip row catches up and the chip glows.
  Creator.prototype.landed = function (st) {
    var cs = this.$('.hc-chips');
    if (!cs) return;
    cs.innerHTML = this.chipsHTML();
    var nc = cs.querySelector('[data-k="' + st.fieldKey + '"]'); if (nc) nc.classList.add('is-glow');
    var slot = cs.querySelector('.is-slot.is-cur'); if (slot) this.anim(slot, [{ opacity: 0, transform: 'translateX(8px)' }, { opacity: 1, transform: 'none' }], { duration: ms(220), slide: true });
    this.chipsInto();
  };

  Creator.prototype.chipsInto = function () {
    var chips = this.$('.hc-chips');
    if (!chips) return;
    var c = chips.querySelector('.is-cur') || chips.lastElementChild;
    if (c && chips.scrollTo) chips.scrollTo({ left: Math.max(0, c.offsetLeft - 16 - (chips.clientWidth - c.offsetWidth) / 2), behavior: this.still() ? 'auto' : 'smooth' });
  };

  // The card that travels: a copy of the chosen card that shrinks into its
  // chip, its words crossing over mid-flight.
  Creator.prototype.ghost = function (c, st, from) {
    var g = document.createElement('div'); g.className = 'hc-ghost';
    g.innerHTML = '<div class="hc-gf"><span class="hc-gk">' + esc(st.label) + '</span><span class="hc-disp hc-gn" style="font-size:' + (from.height > 120 ? 30 : 22) + 'px">' + esc(c.name) + '</span><span class="hc-gs">' + esc(plain(c.summary)) + '</span></div>' +
      '<div class="hc-gc"><small>' + esc(st.label) + '</small><b>' + esc(c.name) + '</b></div>';
    g.style.left = from.left + 'px'; g.style.top = from.top + 'px'; g.style.width = from.width + 'px'; g.style.height = from.height + 'px';
    this.app().appendChild(g);
    return g;
  };
  Creator.prototype.flight = function (g, from, to, dur) {
    var k = [{ left: from.left + 'px', top: from.top + 'px', width: from.width + 'px', height: from.height + 'px', borderRadius: '14px' },
      { left: (from.left + (to.left - from.left) * 0.35) + 'px', top: (from.top + (to.top - from.top) * 0.6 - 10) + 'px', width: (from.width * 0.7 + to.width * 0.3) + 'px', height: (from.height * 0.45 + to.height * 0.55) + 'px', offset: 0.45 },
      { left: to.left + 'px', top: to.top + 'px', width: to.width + 'px', height: to.height + 'px', borderRadius: '10px' }];
    g.querySelector('.hc-gf').animate([{ opacity: 1 }, { opacity: 0 }], { duration: dur * 0.42, easing: 'ease-in', fill: 'forwards' });
    g.querySelector('.hc-gc').animate([{ opacity: 0 }, { opacity: 1 }], { duration: dur * 0.3, delay: dur * 0.5, fill: 'forwards' });
    return g.animate(k, { duration: dur, easing: 'cubic-bezier(.3,.7,.25,1)', fill: 'forwards' });
  };

  Creator.prototype.chooseDesk = function (c, st, si, i, nx) {
    var src = this.$$('.hc-gcard[data-i]')[i] || this.$$('.hc-lrow[data-i]')[i], self = this;
    var row = this.$('.hc-rail .hc-row[data-k="' + st.fieldKey + '"]');
    if (!this.lively() || !src || !row) { this.goStep(nx, 'fwd', st.fieldKey); return; }
    this.busy = true;
    var app = this.app(), from = rel(app, src.getBoundingClientRect()), to = rel(app, row.getBoundingClientRect());
    var g = this.ghost(c, st, from);
    src.style.visibility = 'hidden';
    onScreen(this.$$('.hc-gcard, .hc-lrow')).forEach(function (x) { if (x !== src) x.animate([{ opacity: 1 }, { opacity: 0, transform: 'translateY(8px)' }], { duration: ms(160), easing: 'ease-in', fill: 'forwards' }); });
    var fl = this.flight(g, from, to, ms(480)), seq = this.navSeq;
    setTimeout(function () { if (self.navSeq === seq) self.goStep(nx, 'fwd', null, true); }, ms(300));
    fl.onfinish = function () {
      g.remove(); self.busy = false;
      var rail = self.$('.hc-rail');
      if (rail) { rail.outerHTML = self.railHTML(); var nr = self.$('.hc-rail .hc-row[data-k="' + st.fieldKey + '"]'); if (nr) nr.classList.add('is-glow'); self.countStats(); }
    };
  };

  // The figures in "Your hero so far" count to their new value.
  Creator.prototype.countStats = function () {
    var s = this.stats(), self = this, was = this.lastStats || {};
    this.$$('[data-stat]').forEach(function (b) {
      var k = b.getAttribute('data-stat'), v = s[k], w = was[k];
      if (v == null || v === w) return;
      if (typeof v === 'number' && typeof w === 'number' && self.lively()) {
        var t0 = performance.now();
        (function tick(t) { var p = Math.min(1, (t - t0) / 420), e = 1 - Math.pow(1 - p, 3); b.textContent = Math.round(w + (v - w) * e); if (p < 1) requestAnimationFrame(tick); })(t0);
      } else { b.classList.remove('is-pop'); void b.offsetWidth; if (!self.still()) b.classList.add('is-pop'); }
    });
    this.lastStats = s;
  };

  // Go to a step; dir 'fwd' deals from the right, 'back' from the left.
  // keepChips leaves the chip row alone while a card is still flying to it.
  Creator.prototype.goStep = function (i, dir, glowKey, keepChips) {
    var me = this;
    if (this.own && this.desk()) { this.leaveOwn(function () { me.goStep(i, dir, glowKey, keepChips); }); return; }
    // Each move counts, so a choose's delayed step-on can tell the player has
    // already gone somewhere; a redraw mid-flight ends the flight, and with it
    // the busy flag.
    this.navSeq = (this.navSeq || 0) + 1;
    if (!keepChips) this.busy = false;
    this.closeTip(true);
    this.toggleDrape(false);
    var prev = this.step;
    this.step = Math.max(0, Math.min(i, this.last()));
    var app = this.app();
    if (app) app.setAttribute('data-mo', motion());
    if (this.desk()) {
      if (!keepChips) this.render();
      else {
        var cols = this.$('.hc-cols'), tmp = document.createElement('div');
        tmp.innerHTML = '<div class="hc-cols">' + this.railHTML() + (this.isExtra() ? '<section class="hc-mid hc-mid--wide">' + this.extraHTML() + '</section>'
          : '<section class="hc-mid" data-mid>' + this.midInner() + '</section><aside class="hc-right" data-right aria-label="The entry">' + this.rightInner() + '</aside>') + '</div>';
        cols.parentNode.replaceChild(tmp.firstElementChild, cols);
        app.setAttribute('data-step', this.step);
      }
      if (glowKey) { var r = this.$('.hc-rail .hc-row[data-k="' + glowKey + '"]'); if (r) r.classList.add('is-glow'); }
      this.after();
      this.countStats();
      this.deal(dir || (i < prev ? 'back' : 'fwd'));
      this.focusHead();
      return;
    }
    if (keepChips) {
      var b = this.$('[data-body]');
      b.innerHTML = this.bodyInner();
      var sh = this.$('.hc-shead'); if (sh) sh.outerHTML = this.sheadHTML();
      app.setAttribute('data-step', this.step);
      this.after();
    } else this.render();
    if (glowKey) this.glowPick(glowKey);
    this.deal(dir || (i < prev ? 'back' : 'fwd'));
    this.focusHead();
    var top = this.root.getBoundingClientRect().top;
    if (top < 0) this.root.scrollIntoView({ block: 'start' });
  };

  // On a phone the pick just made glows where it is shown: its chip.
  Creator.prototype.glowPick = function (key) {
    var c = this.$('.hc-chips [data-k="' + key + '"]');
    if (c) c.classList.add('is-glow');
  };

  Creator.prototype.focusHead = function () {
    var h = this.$('.hc-intro h2, .hc-rev__t');
    if (h) h.focus({ preventScroll: true });
  };

  // The new step's cards are dealt in; under Gentle only a fade.
  Creator.prototype.deal = function (dir) {
    if (this.still()) return;
    var sgn = dir === 'back' ? -1 : 1, self = this;
    this.$$('[data-body] .hc-intro, [data-mid] .hc-intro, .hc-sbtn').forEach(function (x) { self.anim(x, [{ opacity: 0, transform: 'translateY(6px)' }, { opacity: 1, transform: 'none' }], { duration: ms(280), fill: 'none', slide: true }); });
    if (this.lively()) {
      var hand = this.$('[data-hand]'), mid = hand ? hand.scrollLeft + hand.clientWidth / 2 : 0;
      var cards = hand ? qa('.hc-card', hand).filter(function (c) { return Math.abs(c.offsetLeft + c.offsetWidth / 2 - mid) < hand.clientWidth; }) : [];
      cards.forEach(function (c, n) {
        var end = c.style.transform || 'none', op = c.style.opacity || 1;
        c.classList.add('is-dealing');
        var a = c.animate([{ opacity: 0, transform: 'translateX(' + (sgn * 90) + 'px) rotate(' + (sgn * 3) + 'deg) scale(.96)' }, { opacity: op, transform: end }], { duration: ms(340), delay: 40 + n * 60, easing: 'cubic-bezier(.16,1,.3,1)', fill: 'backwards' });
        a.onfinish = function () { c.classList.remove('is-dealing'); };
      });
      this.$$('.hc-gcard').slice(0, 9).forEach(function (c, n) { c.animate([{ opacity: 0, transform: 'translateY(14px) scale(.98)' }, { opacity: 1, transform: 'none' }], { duration: ms(300), delay: 30 + n * 35, easing: 'cubic-bezier(.16,1,.3,1)', fill: 'backwards' }); });
      this.$$('.hc-lrow[data-i]').slice(0, 8).forEach(function (c, n) { c.animate([{ opacity: 0, transform: 'translateX(' + (sgn * 24) + 'px)' }, { opacity: 1, transform: 'none' }], { duration: ms(260), delay: n * 30, easing: 'cubic-bezier(.16,1,.3,1)', fill: 'backwards' }); });
      var r = this.$('[data-right] .hc-det'); if (r) r.animate([{ opacity: 0, transform: 'translateY(8px)' }, { opacity: 1, transform: 'none' }], { duration: ms(280), delay: 120, easing: 'cubic-bezier(.16,1,.3,1)', fill: 'backwards' });
      this.$$('.hc-rev .hc-hero, .hc-rev .hc-field, .hc-rev .hc-picks, .hc-rev .hc-fin').forEach(function (x, n) { x.animate([{ opacity: 0, transform: 'translateY(10px)' }, { opacity: 1, transform: 'none' }], { duration: ms(300), delay: n * 60, easing: 'cubic-bezier(.16,1,.3,1)', fill: 'backwards' }); });
    } else {
      this.$$('[data-hand], .hc-list, .hc-grid, [data-right] .hc-det, .hc-rev').forEach(function (x) { self.anim(x, [{ opacity: 0 }, { opacity: 1 }], { duration: ms(180), fill: 'none' }); });
    }
  };

  // --- Buying from a list of costed traits ---

  Creator.prototype.buyTarget = function () {
    var c = this.curEntry(), st = this.cur();
    var bl = c && buyList(c.properties);
    return bl ? { st: st, c: c, bl: bl } : null;
  };

  Creator.prototype.toggleBuy = function (n) {
    var t = this.buyTarget();
    if (!t) return;
    var it = t.bl.items[n], mine = this.mine(t.st.fieldKey, t.c), at = mine.indexOf(it.name), left = t.bl.budget - this.spent(t.st.fieldKey, t.c, t.bl);
    if (at >= 0) mine.splice(at, 1);
    else if (it.cost <= left) mine.push(it.name);
    else {
      var tc = this.$$('.hc-tcard')[n];
      if (tc) { tc.classList.remove('is-flash'); void tc.offsetWidth; tc.classList.add('is-flash'); }
      this.say('Needs ' + it.cost + (it.cost === 1 ? ' point' : ' points') + ', ' + left + ' left.');
      return;
    }
    this.refreshBuy(t);
    this.save();
  };

  Creator.prototype.refreshBuy = function (t) {
    var self = this, mine = this.mine(t.st.fieldKey, t.c), left = t.bl.budget - this.spent(t.st.fieldKey, t.c, t.bl), big = this.$('[data-left]');
    if (big && big.textContent !== String(left)) { big.textContent = left; big.classList.remove('is-pop'); void big.offsetWidth; if (!this.still()) big.classList.add('is-pop'); }
    this.$$('.hc-buy .hc-pips i').forEach(function (p, n) { p.classList.toggle('is-on', n < t.bl.budget - left); });
    this.$$('.hc-buy .hc-tcard').forEach(function (c) {
      var it = t.bl.items[+c.getAttribute('data-t')], s = self.tstate(it, mine, left);
      c.classList.toggle('is-on', s.on); c.classList.toggle('is-no', s.no);
      var bt = c.querySelector('.hc-tbuy'); bt.textContent = self.tlabel(it, s, left); bt.setAttribute('aria-pressed', String(s.on));
      if (s.no) bt.setAttribute('aria-disabled', 'true'); else bt.removeAttribute('aria-disabled');
    });
    this.$$('[data-act="choose"]').forEach(function (b) { b.innerHTML = self.chooseLabel(t.c, t.st); });
    this.say(left + ' ' + lower(words(t.bl.budgetKey)) + ' left.');
    if (this.desk()) { var sf = this.$('[data-sofar]'); if (sf) { sf.outerHTML = this.sofarHTML(); this.countStats(); } }
  };

  Creator.prototype.quick = function () {
    var t = this.buyTarget();
    if (!t || !Array.isArray(t.c.properties.quick_build)) return;
    var mine = this.mine(t.st.fieldKey, t.c), left = t.bl.budget, names = [];
    mine.length = 0;
    t.c.properties.quick_build.forEach(function (n) {
      var it = t.bl.items.filter(function (x) { return x.name === n; })[0];
      if (it && it.cost <= left) { mine.push(n); names.push(n); left -= it.cost; }
    });
    this.refreshBuy(t);
    this.save();
    this.say('Suggested set chosen: ' + names.join(', '));
  };

  Creator.prototype.unbuy = function () {
    var t = this.buyTarget();
    if (!t) return;
    this.mine(t.st.fieldKey, t.c).length = 0;
    this.refreshBuy(t);
    this.save();
  };

  // --- The die ---

  // Deals one card at random, read off the same dice as the hero builder: one
  // d10 for up to ten cards, percentile dice for more. Ask first and Not
  // offered cards are not dealt.
  Creator.prototype.dealRandom = function (btn) {
    var l = this.entries(this.step), rl = rollable(l), n = rl.length, st = this.cur(), self = this, si = this.step;
    if (!n) { this.say('There is nothing here to deal.'); return; }
    if (this.rolling) return;
    this.rolling = true;
    btn.classList.remove('is-roll'); void btn.offsetWidth; if (!this.still()) btn.classList.add('is-roll');
    throwFor(st.label, n, btn).then(function (face) {
      self.rolling = false;
      if (self.step !== si || !document.body.contains(self.root)) return;
      var c = rl[indexFor(n, face)], i = l.indexOf(c);
      self.dealTo(i);
      self.say('Dealt ' + c.name + '.');
    });
  };

  Creator.prototype.dealTo = function (i) {
    var st = this.cur(), self = this;
    this.at[st.fieldKey] = i;
    var flash = function (c) { if (!c) return; c.classList.remove('is-flash'); void c.offsetWidth; setTimeout(function () { c.classList.add('is-flash'); }, self.lively() ? 300 : 0); };
    if (this.desk()) { this.setFocus(i); flash(this.$$('.hc-gcard[data-i]')[i] || this.$$('.hc-lrow[data-i]')[i]); return; }
    if (this.mode === 'cards') { this.scrollHandTo(i, true); flash(this.$$('.hc-hand .hc-card[data-i]')[i]); this.refreshUnder(); return; }
    var row = this.$$('.hc-lrow[data-i]')[i];
    if (row) { if (row.scrollIntoView) row.scrollIntoView({ block: 'center', behavior: this.still() ? 'auto' : 'smooth' }); this.openSheet(i); }
  };

  // --- Rules words: one hover card ---

  Creator.prototype.tipInfo = function (el) {
    var key = el.getAttribute('data-term'), id = el.getAttribute('data-tip');
    if (key) {
      var t = this.terms && hasOwn(this.terms, key) ? this.terms[key] : null;
      return t ? { cat: esc(cap(t.category || 'Rules')), name: esc(t.name), html: esc(t.text || '') } : null;
    }
    var T = tips();
    return id && hasOwn(T, id) ? { cat: T[id].category, name: esc(T[id].name), html: T[id].html } : null;
  };

  Creator.prototype.showTip = function (el, pin) {
    var self = this, app = this.app();
    clearTimeout(this.tipTimer); clearTimeout(this.tipHide);
    var info = this.tipInfo(el);
    if (!info || !app) return;
    var X = '<button type="button" class="hc-ibtn hc-x" data-act="tipx" aria-label="Close">' + I.x + '</button>';
    if (this.tipFor === el && this.tip) {
      if (pin && !this.tipPinned) { this.tipPinned = true; this.tip.classList.add('is-pinned'); this.tip.insertAdjacentHTML('beforeend', X); }
      return;
    }
    this.closeTip(true);
    var tip = document.createElement('div');
    tip.className = 'hc-tip' + (pin ? ' is-pinned' : '');
    tip.id = 'hc-tip';
    tip.setAttribute('role', 'tooltip');
    tip.innerHTML = '<div class="hc-tipb"><div class="hc-cat">' + info.cat + '</div><div class="hc-tipnm hc-disp">' + info.name + '</div><p>' + info.html + '</p></div>' + (pin ? X : '');
    var tb = tip.querySelector('.hc-tipb');
    tb.addEventListener('scroll', function () { tb.classList.toggle('is-more', tb.scrollTop + tb.clientHeight < tb.scrollHeight - 1); }, { passive: true });
    tip.addEventListener('pointerenter', function () { clearTimeout(self.tipHide); });
    tip.addEventListener('pointerleave', function (ev) { if (ev.pointerType === 'mouse' && !self.tipPinned) self.tipHide = setTimeout(function () { self.closeTip(); }, 160); });
    app.appendChild(tip);
    this.tip = tip; this.tipFor = el; this.tipPinned = !!pin;
    el.setAttribute('aria-expanded', 'true'); el.setAttribute('aria-describedby', 'hc-tip');
    this.placeTip();
    this.anim(tip, [{ opacity: 0, transform: tip.getAttribute('data-side') ? 'translateX(6px)' : 'translateY(' + (tip.getAttribute('data-below') === '1' ? -4 : 4) + 'px)' }, { opacity: 1, transform: 'none' }], { duration: ms(140), fill: 'none', slide: true });
  };

  // Below the term when there is room, else above; always inside the screen.
  // In the computer's reading column a word explains itself in the margin
  // beside it, so the lines that follow stay readable.
  Creator.prototype.placeTip = function () {
    var tip = this.tip, el = this.tipFor, app = this.app();
    if (!tip || !el || !app) return;
    var r = rel(app, el.getBoundingClientRect()), W = app.clientWidth, H = Math.min(app.clientHeight, window.innerHeight - app.getBoundingClientRect().top - dockedBar(window.innerHeight));
    var w = tip.offsetWidth, h = tip.offsetHeight;
    var col = this.desk() && el.closest('[data-right]');
    if (col) {
      var c = rel(app, col.getBoundingClientRect());
      if (c.left - w - 14 > 8) {
        tip.style.left = (c.left - w - 14) + 'px'; tip.style.top = Math.max(6, Math.min(r.top - 14, H - h - 6)) + 'px';
        tip.setAttribute('data-below', '0'); tip.setAttribute('data-side', '1');
        return;
      }
    }
    // On a phone the card never lies on the paragraph being read: it goes
    // under the paragraph or over it, and when neither has room it sits at
    // the foot and the reading scrolls up clear of it.
    if (!this.desk()) {
      // A long explanation scrolls inside a card at most under half the
      // visible height, so the card always has somewhere to go.
      var body = tip.querySelector('.hc-tipb');
      if (body) { body.style.maxHeight = Math.max(120, Math.floor(H * 0.45) - 25) + 'px'; h = tip.offsetHeight; body.classList.toggle('is-more', body.scrollTop + body.clientHeight < body.scrollHeight - 1); }
      var para = el.closest('p, li, dd, .hc-lede') || el, pr = rel(app, para.getBoundingClientRect());
      tip.style.left = '8px';
      tip.removeAttribute('data-side');
      if (pr.top + pr.height + 8 + h <= H - 6) { tip.style.top = (pr.top + pr.height + 8) + 'px'; tip.setAttribute('data-below', '1'); return; }
      if (pr.top - 8 - h >= 6) { tip.style.top = (pr.top - 8 - h) + 'px'; tip.setAttribute('data-below', '0'); return; }
      var foot = H - h - 6, sc = el.closest('.hc-sc, .hc-rev, .hc-list');
      tip.style.top = Math.max(6, foot) + 'px';
      tip.setAttribute('data-below', '1');
      // The paragraph scrolls clear as far as the word stays in view, and the
      // word itself always does: near the end of the text the scroller gets
      // room at its foot for that, given back when the card closes.
      var over = pr.top + pr.height + 8 - foot, room = r.top - (sc ? rel(app, sc.getBoundingClientRect()).top : 0) - 6;
      var want = Math.max(Math.min(over, room), r.top + r.height + 8 - foot);
      if (sc && want > 0) {
        var left = sc.scrollHeight - sc.clientHeight - sc.scrollTop;
        if (left < want && !this.tipPad) {
          this.tipPad = { sc: sc, was: sc.style.paddingBottom };
          sc.style.paddingBottom = ((parseFloat(getComputedStyle(sc).paddingBottom) || 0) + Math.ceil(want - left)) + 'px';
        }
        sc.scrollTop += want;
      }
      return;
    }
    var below = r.top + r.height + 8 + h <= H - 6 || r.top - 8 - h < 6;
    var top = below ? r.top + r.height + 8 : r.top - 8 - h;
    top = Math.max(6, Math.min(top, H - h - 6));
    tip.style.left = Math.max(8, Math.min(r.left + r.width / 2 - w / 2, W - w - 8)) + 'px';
    tip.style.top = top + 'px';
    tip.setAttribute('data-below', below ? '1' : '0');
    tip.removeAttribute('data-side');
  };

  Creator.prototype.followTip = function () {
    if (!this.tip || !this.tipFor) return;
    if (!this.tipPinned) return this.closeTip(true);
    var sc = this.tipFor.closest('.hc-sc');
    if (sc) { var a = this.tipFor.getBoundingClientRect(), b = sc.getBoundingClientRect(); if (a.bottom < b.top || a.top > b.bottom) return this.closeTip(true); }
    this.placeTip();
  };

  Creator.prototype.closeTip = function (now, refocus) {
    clearTimeout(this.tipTimer); clearTimeout(this.tipHide);
    var el = this.tip, f = this.tipFor;
    if (!el) return;
    this.tip = null; this.tipFor = null; this.tipPinned = false;
    if (this.tipPad) { this.tipPad.sc.style.paddingBottom = this.tipPad.was; this.tipPad = null; }
    if (f) { f.setAttribute('aria-expanded', 'false'); f.removeAttribute('aria-describedby'); if (refocus) f.focus(); }
    if (now || this.still() || !el.animate) { el.remove(); return; }
    var a = el.animate([{ opacity: 1 }, { opacity: 0 }], { duration: ms(90), easing: 'ease-in', fill: 'forwards' });
    a.onfinish = function () { el.remove(); };
  };

  // --- Making the hero ---

  Creator.prototype.create = function () {
    var self = this, err = this.root.querySelector('[data-error]'), btn = this.root.querySelector('[data-act="create"]');
    var name = this.name.trim();
    function fail(msg) { err.textContent = msg; err.hidden = false; btn.disabled = false; }
    if (!name) { fail('Give your hero a name first.'); this.root.querySelector('[data-name]').focus(); return; }
    btn.disabled = true;
    err.hidden = true;
    var picks = {};
    Object.keys(this.picks).forEach(function (k) { if (self.picks[k].name) picks[k] = self.picks[k]; });
    if (this.tryout) { this.keepHere(name, picks, fail); return; }
    var send = this.persist(name, picks);
    send.then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (body) { return { ok: r.ok, body: body }; });
    }).then(function (res) {
      if (!res.ok || !res.body.url) { fail(res.body.message || res.body.error || 'The hero could not be made. Try again.'); return; }
      self.clearDraft();
      var go = function () { if (window.Chronicle && Chronicle.go) Chronicle.go(res.body.url); else window.location.href = res.body.url; };
      if (self.practice) { go(); return; }
      // A pick the Director has to allow could not be sent: say so, with a
      // way on, instead of moving past it.
      var failed = res.body.requestFailed || [];
      if (failed.length) {
        err.innerHTML = esc('Your hero is made, but a pick for ' + failed.map(function (k) { var s2 = self.stepByKey(k); return s2 ? lower(s2.label) : k; }).join(', ') +
          ' could not be sent to your ') + gm() + '. Open the hero and pick it again.' +
          ' <a class="hc-link" href="' + esc(res.body.url) + '">Open the hero</a>';
        err.hidden = false;
        return;
      }
      self.keepDraftsTo(res.body.id).then(go, go);
    }).catch(function () { fail('The hero could not be made. Check your connection and try again.'); });
  };

  // The ancestry and class of a hero, worded as the party block does: the
  // same keys, in the same order, so a stored try-out reads like a hero.
  Creator.prototype.summary = function (picks) {
    function read(keys) {
      for (var i = 0; i < keys.length; i++) {
        var p = picks[keys[i]];
        if (p && typeof p.name === 'string' && p.name.trim()) return p.name.split(/\s+/).filter(Boolean).join(' ');
      }
      return '';
    }
    return [read(['ancestry', 'race', 'species', 'heritage']), read(['class', 'calling', 'career'])].filter(Boolean).join(' ');
  };

  // Keeps a try-out in this browser and goes back to the Characters page. This
  // is the whole of "saving" in try-out mode: no request is made.
  Creator.prototype.keepHere = function (name, picks, fail) {
    try {
      window.localStorage.setItem(this.tryoutKey, JSON.stringify({
        v: 1, name: name, picks: picks, summary: this.summary(picks), savedAt: new Date().toISOString()
      }));
    } catch (e) { fail(NO_STORAGE); return; }
    var to = this.backUrl || ('/campaigns/' + encodeURIComponent(this.campaignId) + '/characters');
    if (window.Chronicle && Chronicle.go) Chronicle.go(to); else window.location.href = to;
  };

  // Sends the finished hero to the server: a new one, or in practice mode a
  // change to the practice hero being edited. The one place a hero is kept.
  Creator.prototype.persist = function (name, picks) {
    var edit = this.practice && this.updateUrl, url = edit ? this.updateUrl : this.createUrl, method = edit ? 'PUT' : 'POST';
    var body = { name: name, picks: picks }, self = this, about = {};
    this.aboutFields().forEach(function (f) { var v = String(self.about[f.key] || '').trim(); if (v) about[f.key] = v; });
    if (Object.keys(about).length) body.about = about;
    // A copy of the player's own hero: the server reads that hero again.
    if (this.practice && !edit && this.preload && this.preload.copy) body.copy = true;
    return window.Chronicle && Chronicle.apiFetch
      ? Chronicle.apiFetch(url, { method: method, body: body, csrfToken: this.csrf })
      : fetch(url, { method: method, credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrf }, body: JSON.stringify(body) });
  };

  // The player's own draft entries they picked are kept to the hero just
  // made, so a Director accepting one can give it to that hero only.
  Creator.prototype.keepDraftsTo = function (heroId) {
    var self = this, calls = [];
    if (!heroId || !(window.Chronicle && Chronicle.apiFetch)) return Promise.resolve();
    Object.keys(this.picks).forEach(function (k) {
      var c = self.picks[k].name && self.choice(k, self.picks[k].name);
      if (!c || !c.mine || !c.status || !c.entryId) return;
      calls.push(Chronicle.apiFetch('/campaigns/' + encodeURIComponent(self.campaignId) + '/system-entries/' + encodeURIComponent(c.entryId),
        { method: 'PUT', body: { forEntityId: heroId }, csrfToken: self.csrf }).catch(function () {}));
    });
    return Promise.all(calls);
  };

  // --- Events ---

  Creator.prototype.act = function (n, el) {
    var self = this, st = this.plan.steps[this.step];
    switch (n) {
      case 'choose': this.choose(); break;
      case 'closesheet': this.closeSheet(false, null, true); break;
      case 'ownopen':
        // A card peeking in comes to the middle first, as an entry's does.
        if (!this.desk() && el.classList.contains('hc-card')) {
          var hand = this.$('[data-hand]'), mid = hand.scrollLeft + hand.clientWidth / 2;
          if (Math.abs(el.offsetLeft + el.offsetWidth / 2 - mid) > el.offsetWidth / 2) { this.scrollHandTo(this.$$('.hc-hand .hc-card').indexOf(el), true); break; }
        }
        this.openOwn(null);
        break;
      case 'ownedit': this.openOwn(this.curEntry()); break;
      case 'tipx': this.closeTip(false, true); break;
      case 'drape': this.toggleDrape(false); break;
      case 'steps': this.toggleDrape(); break;
      case 'back': if (this.step > 0) this.goStep(this.step - 1, 'back'); break;
      case 'next': this.goStep(this.step + 1, 'fwd'); break;
      case 'skip': this.skipped[st.fieldKey] = true; this.goStep(this.nextOpen(this.step), 'fwd', st.fieldKey); break;
      case 'clear':
        this.find[st.fieldKey] = ''; this.at[st.fieldKey] = 0;
        var tl = this.$('.hc-tools');
        tl.outerHTML = this.toolsHTML();
        this.refreshList();
        var inp = this.$('[data-find]'); if (inp) inp.focus();
        break;
      case 'die': this.dealRandom(el); break;
      case 'quick': this.quick(); break;
      case 'unbuy': this.unbuy(); break;
      case 'create': this.create(); break;
      case 'nothere':
        var nh = this.$('.hc-nh');
        this.nh[st.fieldKey] = !this.nh[st.fieldKey];
        if (nh) nh.outerHTML = this.nhHTML(st);
        var again = this.$('.hc-nh .hc-link');
        if (again) again.focus();
        break;
    }
  };

  Creator.prototype.wire = function () {
    var self = this, root = this.root;

    root.addEventListener('click', function (ev) {
      var t = ev.target;
      var term = t.closest('[data-term], button[data-tip]');
      if (term && root.contains(term) && !term.closest('.hc-card, .hc-gcard, .hc-lrow')) {
        ev.preventDefault();
        if (self.tipPinned && self.tipFor === term) self.closeTip(); else self.showTip(term, true);
        return;
      }
      var a = t.closest('[data-act]');
      if (a && root.contains(a)) { if (a.tagName === 'A') return; self.act(a.getAttribute('data-act'), a, ev); return; }
      if (!self.plan) return;
      var go = t.closest('[data-go]');
      if (go && root.contains(go)) {
        var gi = +go.getAttribute('data-go');
        self.toggleDrape(false);
        self.closeSheet(true);
        if (gi !== self.step) self.goStep(gi, gi < self.step ? 'back' : 'fwd');
        return;
      }
      var md = t.closest('[data-mode]');
      if (md) {
        if (self.mode !== md.getAttribute('data-mode')) {
          self.mode = md.getAttribute('data-mode');
          self.$$('.hc-mode button').forEach(function (b) { b.setAttribute('aria-pressed', String(b === md)); });
          self.refreshList();
        }
        return;
      }
      var buy = t.closest('[data-buy].hc-tbuy, button[data-buy]');
      if (buy) { self.toggleBuy(+buy.getAttribute('data-buy')); return; }
      if (!self.desk()) {
        var card = t.closest('.hc-hand .hc-card[data-i], .hc-lrow[data-i]');
        if (card) {
          var i = +card.getAttribute('data-i');
          if (card.classList.contains('hc-card')) {
            var hand = self.$('[data-hand]'), mid = hand.scrollLeft + hand.clientWidth / 2;
            // A neighbour that is peeking in comes to the middle first.
            if (Math.abs(card.offsetLeft + card.offsetWidth / 2 - mid) > card.offsetWidth / 2) { self.at[self.cur().fieldKey] = i; self.scrollHandTo(i, true); return; }
          }
          self.openSheet(i);
        }
        return;
      }
      var gc = t.closest('.hc-gcard[data-i]:not(.hc-gcard--nh), .hc-mid .hc-lrow[data-i]');
      if (gc) self.setFocus(+gc.getAttribute('data-i'));
    });

    root.addEventListener('dblclick', function (ev) {
      var gc = ev.target.closest('.hc-gcard[data-i], .hc-mid .hc-lrow[data-i]');
      if (gc && self.desk()) self.choose(+gc.getAttribute('data-i'));
    });

    root.addEventListener('input', function (ev) {
      var t = ev.target;
      if (t.hasAttribute && t.hasAttribute('data-find')) {
        var k = self.cur().fieldKey;
        self.find[k] = t.value; self.at[k] = 0;
        var clr = self.$('.hc-find .hc-clr');
        if (t.value && !clr) t.insertAdjacentHTML('afterend', '<button type="button" class="hc-ibtn hc-clr" data-act="clear" aria-label="Clear the search">' + I.x + '</button>');
        if (!t.value && clr) clr.remove();
        self.refreshList();
        return;
      }
      if (t.hasAttribute && t.hasAttribute('data-about')) {
        self.about[t.getAttribute('data-about')] = t.value;
        var line = self.$('[data-about-line]'); if (line) line.textContent = self.aboutLine() || 'Optional';
        self.save();
        return;
      }
      if (t.hasAttribute && t.hasAttribute('data-name')) {
        self.name = t.value;
        var nm = self.name.trim(), h = self.$('[data-hname]');
        if (h) h.textContent = nm || 'Unnamed hero';
        var rl = self.$('.hc-rail .hc-row[data-go="' + self.last() + '"] .hc-lb span'); if (rl) rl.textContent = nm || 'Last';
        var sf = self.$('[data-sofar] .hc-nm2'); if (sf) sf.textContent = nm || 'Unnamed hero';
        self.save();
      }
    });

    root.addEventListener('keydown', function (ev) {
      var t = ev.target, inField = /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName) || t.isContentEditable;
      if (ev.key === 'Escape') {
        if (self.tip) { self.closeTip(false, true); ev.preventDefault(); return; }
        if (self.$('[data-drape]')) { self.toggleDrape(false); var sb = self.$('.hc-sbtn'); if (sb) sb.focus(); ev.preventDefault(); return; }
        if (self.sh) { self.closeSheet(false, null, true); ev.preventDefault(); return; }
        if (self.own && self.desk()) { self.own.editor.requestClose(function () { self.closeOwn(); }); ev.preventDefault(); return; }
        if (inField && t.hasAttribute('data-find') && t.value) { self.act('clear'); ev.preventDefault(); return; }
      }
      if (ev.key === 'Enter' && t.hasAttribute && t.hasAttribute('data-name')) { ev.preventDefault(); self.create(); return; }
      if (ev.key === 'Tab' && self.sh) {
        var f = qa('button:not([disabled]), [href], input, [tabindex]:not([tabindex="-1"])', self.sh.el).concat(qa('button:not([disabled])', self.sh.ft));
        if (f.length) {
          var first = f[0], last = f[f.length - 1];
          if (ev.shiftKey && (t === first || !self.sh.el.contains(t) && !self.sh.ft.contains(t))) { ev.preventDefault(); last.focus(); }
          else if (!ev.shiftKey && t === last) { ev.preventDefault(); first.focus(); }
        }
        return;
      }
      if (inField || !self.plan || self.isExtra()) return;
      var card = t.closest && t.closest('.hc-hand .hc-card[data-i]');
      if (card && (ev.key === 'Enter' || ev.key === ' ')) { ev.preventDefault(); self.openSheet(+card.getAttribute('data-i')); return; }
      if (card && (ev.key === 'ArrowRight' || ev.key === 'ArrowLeft')) {
        ev.preventDefault();
        var j = +card.getAttribute('data-i') + (ev.key === 'ArrowRight' ? 1 : -1), cs = self.$$('.hc-hand .hc-card[data-i]');
        if (cs[j]) { self.at[self.cur().fieldKey] = j; self.scrollHandTo(j, true); cs[j].focus({ preventScroll: true }); self.refreshUnder(); }
        return;
      }
      var oc = t.closest && t.closest('[data-act="ownopen"]');
      if (oc && (ev.key === 'Enter' || ev.key === ' ')) { ev.preventDefault(); self.act('ownopen', oc); return; }
      if (self.desk() && t.closest && t.closest('.hc-gcard:not(.hc-gcard--own), .hc-mid .hc-lrow:not(.hc-lrow--own)')) {
        var cur = self.curIdx(), cols = self.mode === 'cards' ? self.gridCols() : 1, n = self.entries(self.step).length;
        var map = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: cols, ArrowUp: -cols };
        if (map[ev.key] != null) { ev.preventDefault(); self.setFocus(Math.max(0, Math.min(n - 1, cur + map[ev.key]))); return; }
        if (ev.key === 'Home') { ev.preventDefault(); self.setFocus(0); return; }
        if (ev.key === 'End') { ev.preventDefault(); self.setFocus(n - 1); return; }
        if (ev.key === 'Enter') { ev.preventDefault(); self.choose(); return; }
      }
    });

    // Rules words and marks: a hover card on hover (after a beat), focus or tap.
    root.addEventListener('pointerover', function (ev) {
      var t = ev.target.closest && ev.target.closest('[data-term], [data-tip]');
      if (!t || ev.pointerType !== 'mouse' || self.tipPinned) return;
      clearTimeout(self.tipHide); clearTimeout(self.tipTimer);
      self.tipTimer = setTimeout(function () { self.showTip(t, false); }, 150);
    });
    root.addEventListener('pointerout', function (ev) {
      var t = ev.target.closest && ev.target.closest('[data-term], [data-tip]');
      if (!t || ev.pointerType !== 'mouse') return;
      if (ev.relatedTarget && t.contains(ev.relatedTarget)) return;
      clearTimeout(self.tipTimer);
      if (!self.tipPinned) self.tipHide = setTimeout(function () { self.closeTip(); }, 160);
    });
    root.addEventListener('focusin', function (ev) {
      var t = ev.target.closest && ev.target.closest('button[data-term], button[data-tip]');
      if (t && t.matches(':focus-visible')) self.showTip(t, false);
    });
    root.addEventListener('focusout', function (ev) {
      var t = ev.target.closest && ev.target.closest('button[data-term], button[data-tip]');
      if (t && !self.tipPinned && self.tipFor === t) self.closeTip();
    });
    // A tap anywhere else puts a pinned card away.
    this.onDocDown = function (ev) {
      if (!document.body.contains(root) || !self.tip || !self.tipPinned) return;
      if (ev.target.closest && (ev.target.closest('.hc-tip') || ev.target.closest('[data-term], [data-tip]'))) return;
      self.closeTip();
    };
    document.addEventListener('pointerdown', this.onDocDown);
  };

  // Unmount on a boosted navigation: the document and window listeners and the
  // ResizeObserver would otherwise keep this Creator and its plan alive.
  Creator.prototype.destroy = function () {
    if (this.onDocDown) document.removeEventListener('pointerdown', this.onDocDown);
    if (this.onResize) window.removeEventListener('resize', this.onResize);
    if (this.ro) this.ro.disconnect();
    if (this.mo) this.mo.disconnect();
    this.onDocDown = this.onResize = this.ro = this.mo = null;
    this.closeTip(true);
    this.endOwn();
  };

  // Loaded on sight (ADR-063): boot.js fetches this script when it meets the
  // page's data-widget mount, then mounts it through register().
  // The stage look is a subclass in its own file (hero_stage.js, listed ahead
  // of this one): it is built once, from what it may borrow of the creator.
  var StageCreator = null;
  function stage() {
    if (!StageCreator && typeof window.ChronicleHeroStage === 'function') {
      StageCreator = window.ChronicleHeroStage(Creator, {
        esc: esc, lower: lower, cap: cap, an: an, plain: plain, words: words, hasOwn: hasOwn, factsOf: factsOf, buyList: buyList, itemText: itemText,
        motion: motion, ms: ms, rel: rel, qa: qa, q1: q1, onScreen: onScreen, dockedBar: dockedBar, I: I, num: num, sumOf: sumOf
      });
    }
    return StageCreator;
  }

  function register() {
    Chronicle.register('hero-creator', {
      init: function (el) {
        if (el._heroCreator) return;
        var Look = el.getAttribute('data-look') === 'stage' && stage() || Creator;
        var p = new Look(el);
        el._heroCreator = p;
        p.wire();
        p.load();
      },
      destroy: function (el) {
        if (el._heroCreator) el._heroCreator.destroy();
        el._heroCreator = null;
      }
    });
  }
  window.ChronicleHeroCreator = { termKey: termKey, motion: motion, marked: marked, plain: plain, factsOf: factsOf, indexFor: indexFor, rollable: rollable };
  if (window.Chronicle && Chronicle.register) register();
})();
