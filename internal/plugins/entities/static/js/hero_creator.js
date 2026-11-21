/**
 * hero_creator.js — the hero creator (the signed "pull-out leaf" design).
 *
 * One paper page per step. A step lists the entries for one character field
 * (Ancestry, Kit…); tapping an entry slides its leaf out from under the page,
 * with ribbons for the leaf's pages. The last step asks for a name and makes
 * the hero.
 *
 * Everything drawn comes from the entry itself, so an uploaded system whose
 * entries are only a name and some text gets just those pages:
 * - scalar properties become the facts row;
 * - a property that is a list of named items becomes a ribbon page;
 * - a list whose items carry a numeric "cost", with a "<…>_points" budget,
 *   can be bought from (the server checks the same rule).
 *
 * Campaign entries' descriptions arrive as HTML the server sanitized (and
 * stripped of Director-only text for players); package text is plain and is
 * escaped here. Picks are kept on this device until the hero is made.
 *
 * A Director's rules shape each step (the plan's per-step `rules`, worded by
 * hero_rules_lines.js as in the hero builder): Not here entries are gone with
 * a quiet foot line, an Ask first entry carries a mark and is sent to the
 * Director once the hero is made, and a required step with one entry left is
 * filled in and locked.
 *
 * A step whose list a Director let this player write in links to "Your own
 * entries"; the player's draft then shows in the list, and once the hero is
 * made the draft is kept to that hero.
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
 * Served per-plugin at /static/plugins/entities/js/hero_creator.js from the
 * body-script registry, so it survives boosted navigation. Re-wires after
 * HTMX swaps.
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

  var NO_STORAGE = 'This browser won\'t keep things, so the try-out can\'t be saved here.';

  function lower(s) { return String(s || '').toLowerCase(); }

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

  function Creator(root) {
    this.root = root;
    this.campaignId = root.getAttribute('data-campaign-id');
    this.planUrl = root.getAttribute('data-plan-url');
    this.createUrl = root.getAttribute('data-create-url');
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
    this.open = null;
    this.tab = 'overview';
    this.picks = {};
    this.nh = {};
    this.name = '';
  }

  Creator.prototype.load = function () {
    var self = this;
    var get = (window.Chronicle && Chronicle.apiFetch) ? Chronicle.apiFetch(this.planUrl) : fetch(this.planUrl, { credentials: 'same-origin' });
    get.then(function (r) {
      if (!r.ok) throw new Error('plan ' + r.status);
      return r.json();
    }).then(function (plan) {
      self.plan = plan || { steps: [] };
      self.plan.steps = self.plan.steps || [];
      if (self.preload) self.adopt(self.preload); else if (self.tryout) self.adopt(self.stored() || {}); else self.restore();
      self.prefill();
      self.render();
    }).catch(function () {
      var page = self.root.querySelector('.hc-loading__text');
      if (page) page.textContent = 'The creator could not open. Reload the page to try again.';
    });
  };

  // --- Draft kept on this device ---
  Creator.prototype.restore = function () {
    try {
      var d = JSON.parse(window.localStorage.getItem(this.draftKey) || 'null');
      if (d) this.adopt(d);
    } catch (e) { /* no storage: start fresh */ }
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
      self.picks[k] = { name: c.name, options: bl ? opts.filter(function (o) { return bl.items.some(function (it) { return it.name === o; }); }) : [] };
    });
    this.name = typeof d.name === 'string' ? d.name : '';
  };
  Creator.prototype.save = function () {
    // A copy or a change starts from the server's hero, so it keeps no draft
    // that a later blank start would pick up.
    // A try-out has one place to live, written only when it is kept.
    if (this.preload || this.tryout) return;
    try { window.localStorage.setItem(this.draftKey, JSON.stringify({ picks: this.picks, name: this.name })); } catch (e) { /* ignore */ }
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
  Creator.prototype.stepCount = function () { return this.plan.steps.length + 1; };
  Creator.prototype.isDetails = function () { return this.step >= this.plan.steps.length; };
  Creator.prototype.cur = function () { return this.plan.steps[this.step]; };
  Creator.prototype.stepByKey = function (k) {
    for (var i = 0; i < this.plan.steps.length; i++) if (this.plan.steps[i].fieldKey === k) return this.plan.steps[i];
    return null;
  };
  Creator.prototype.choice = function (key, name) {
    var st = this.stepByKey(key);
    if (!st) return null;
    for (var i = 0; i < st.choices.length; i++) if (lower(st.choices[i].name) === lower(name)) return st.choices[i];
    return null;
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
  Creator.prototype.mine = function (key, c) {
    var p = this.picks[key];
    return p && lower(p.name) === lower(c.name) ? p.options : [];
  };
  Creator.prototype.spent = function (key, c, bl) {
    var mine = this.mine(key, c);
    return bl.items.reduce(function (n, it) { return n + (mine.indexOf(it.name) >= 0 ? it.cost : 0); }, 0);
  };

  // --- The page ---
  Creator.prototype.render = function () {
    // The tucked sheet hangs on the wrapper, not the page: the page's own
    // stacking context would otherwise paint it over the page.
    var html = '<div class="hc-wrap paper-stack"><section class="hc-page paper" aria-label="Hero creator">' +
      this.trailHTML() + (this.isDetails() ? this.detailsHTML() : this.stepHTML()) + this.footHTML() +
      '</section>' + (this.isDetails() ? '' : '<aside class="hc-leaf paper" aria-hidden="true"></aside>') + '</div>';
    this.root.innerHTML = this.bandHTML() + html;
    this.open = null;
    this.watchTrail();
  };

  // A step that starts a wrapped line drops the rule that leads into it; the
  // rule belongs to its step, so none is left hanging at a line's end.
  Creator.prototype.watchTrail = function () {
    var trail = this.root.querySelector('.hc-trail');
    if (this.trailWatch) { this.trailWatch.disconnect(); this.trailWatch = null; }
    if (!trail) return;
    var mark = function () {
      var prev = null;
      trail.querySelectorAll('.hc-stop').forEach(function (b) {
        b.classList.toggle('is-wrapped', !!prev && b.offsetTop > prev.offsetTop + 2);
        prev = b;
      });
    };
    mark();
    if (window.ResizeObserver) { this.trailWatch = new ResizeObserver(mark); this.trailWatch.observe(trail); }
  };

  // Practice mode says what it is above the steps, in the page's own words.
  Creator.prototype.bandHTML = function () {
    var note = this.handoff ? '<p class="hc-band hc-band--note" role="status">' + esc(this.handoff) + '</p>' : '';
    if (this.tryout) {
      return note + '<p class="hc-band"><b>Trying it out.</b> Kept in this browser only.' +
        (this.noStorage ? ' ' + NO_STORAGE : '') + '</p>';
    }
    return note + (this.practice ? '<p class="hc-band"><b>Practice hero.</b> Not in the party, not sent to Foundry.</p>' : '');
  };

  Creator.prototype.trailHTML = function () {
    var self = this;
    var labels = this.plan.steps.map(function (s) { return s.label; }).concat(['Name']);
    return '<nav class="hc-trail" aria-label="Steps">' + labels.map(function (l, i) {
      var key = i < self.plan.steps.length ? self.plan.steps[i].fieldKey : null;
      var done = key ? !!(self.picks[key] && self.picks[key].name) : !!self.name.trim();
      return '<button type="button" class="hc-stop' + (i === self.step ? ' is-on' : '') + (done ? ' is-done' : '') + '" data-step="' + i + '"' +
        (i === self.step ? ' aria-current="step"' : '') + '><i aria-hidden="true">' + (done && i !== self.step ? '✓' : (i + 1)) + '</i>' +
        '<span class="hc-stop__lbl">' + esc(l) + '</span></button>';
    }).join('') + '</nav>';
  };

  Creator.prototype.stepHTML = function () {
    var st = this.cur(), self = this;
    var head = '<div class="hc-head"><p class="paper-kind">Step ' + (this.step + 1) + ' of ' + this.stepCount() + '</p>' +
      '<h1 tabindex="-1">Choose your ' + esc(lower(st.label)) + '</h1><p>' +
      (this.locked(st) ? 'Everyone starts with this. Tap it to pull out its leaf and read the whole entry.' : 'Tap one to pull out its leaf and read the whole entry.') + '</p></div>';
    var rows = st.choices.map(function (c, i) {
      var picked = self.picks[st.fieldKey] && lower(self.picks[st.fieldKey].name) === lower(c.name);
      return '<li><button type="button" class="hc-row" data-open="' + i + '" aria-expanded="false"><b>' + esc(c.name) +
        (c.mine && c.status ? ' <span class="hc-seal">Yours · ' + (c.status === 'sent' ? 'handed in' : 'draft') + '</span>'
          : c.source === 'campaign' ? ' <span class="hc-seal">Campaign</span>' : '') +
        (c.state === 'ask_first' ? ' <span class="hc-seal">Ask first</span>' : '') +
        (c.state === 'not_offered' ? ' <span class="hc-seal">Not offered here</span>' : '') +
        (picked && self.locked(st) ? ' <span class="hc-seal">Everyone starts with this</span>' : '') +
        (picked ? ' <span class="hc-row__pick" aria-label="chosen">✓</span>' : '') +
        '</b><span class="hc-row__go" aria-hidden="true">Read ›</span><small>' + esc(c.summary) + '</small></button></li>';
    }).join('');
    var make = st.canMake && !this.practice ? '<p class="hc-make">Your ' + gm() + ' lets you make your own ' + esc(lower(st.label)) + '. ' +
      '<a class="hc-link" href="/campaigns/' + encodeURIComponent(this.campaignId) + '/characters/entries?field=' + encodeURIComponent(st.fieldKey) + '&back=new">Make yours ›</a> ' +
      'Your picks here are kept while you do.</p>' : '';
    var none = !rows && st.rules ? '<p class="hc-empty">Nothing is on this list for you right now.</p>' : '';
    return head + this.notesHTML(st) + (rows ? '<ul class="hc-list">' + rows + '</ul>' : none) + this.notHereHTML(st) + make;
  };

  Creator.prototype.notHereHTML = function (st) {
    return window.ChronicleHeroRules.notHere(st.rules, !!this.nh[st.fieldKey], 'hc');
  };

  Creator.prototype.footHTML = function () {
    var back = this.step > 0 ? '<button type="button" class="hc-btn hc-btn--ghost" data-act="back">← Back</button>' : '';
    if (this.isDetails()) {
      return '<div class="hc-foot"><span class="hc-foot__sp"></span>' + back +
        '<button type="button" class="hc-btn" data-act="create">' + (this.tryout ? 'Keep it on this device' : this.practice ? (this.updateUrl ? 'Save changes' : 'Keep this practice hero') : 'Create hero') + '</button></div>';
    }
    var st = this.cur(), p = this.picks[st.fieldKey];
    var note = this.pickNote(st);
    var chosen = p && p.name ? '<b>' + esc(p.name) + '</b>' + (p.options.length ? ' · ' + esc(p.options.join(', ')) : '') +
      (note ? ' · <span class="hc-wait">' + esc(note) + '</span>' : '')
      : 'No ' + esc(lower(st.label)) + ' chosen yet';
    var next = this.step + 1 < this.plan.steps.length ? this.plan.steps[this.step + 1].label : 'Name';
    return '<div class="hc-foot"><span class="hc-chosen" data-chosen>' + chosen + '</span><span class="hc-foot__sp"></span>' + back +
      '<button type="button" class="hc-btn" data-act="next">' + (p && p.name ? 'Next: ' : 'Skip to ') + esc(next) + ' →</button></div>';
  };

  Creator.prototype.detailsHTML = function () {
    var self = this;
    var rows = this.plan.steps.map(function (s, i) {
      var p = self.picks[s.fieldKey];
      return '<dt>' + esc(s.label) + '</dt>' + (p && p.name
        ? '<dd>' + esc(p.name) + (p.options.length ? '<small>' + esc(p.options.join(', ')) + '</small>' : '') +
          (self.pickNote(s) ? '<small>' + esc(self.pickNote(s)) + '</small>' : '') + '</dd>'
        : '<dd class="is-none">Not chosen</dd>') +
        '<dd>' + (self.locked(s) ? '' : '<button type="button" class="hc-link" data-step="' + i + '">Change</button>') + '</dd>';
    }).join('');
    var whose = this.tryout ? 'This stays in this browser: only you can see it, and nothing is sent to the campaign.' : this.practice ? 'This stays a practice hero: only you and the Directors see it, and it never joins the party.' : this.forSelf ? 'The hero will be yours, on a new ' + esc(this.typeName) + ' page.'
      : 'The hero goes on a new ' + esc(this.typeName) + ' page, not yet claimed by a player.';
    var none = this.plan.steps.length ? '' : '<p class="hc-empty">' + (this.plan.systemName ? esc(this.plan.systemName) + ' doesn’t list' : 'This campaign’s game system doesn’t list') +
      ' choices for heroes yet, so the creator only asks for a name. ' + gm() + 's can add their own entries.</p>';
    return '<div class="hc-head"><p class="paper-kind">Step ' + this.stepCount() + ' of ' + this.stepCount() + '</p>' +
      '<h1 tabindex="-1">Name your ' + (this.practice ? 'practice ' : '') + 'hero</h1></div>' + none +
      '<label class="hc-field"><span class="hc-k">Name</span><input type="text" maxlength="200" autocomplete="off" data-name value="' + esc(this.name) + '" required></label>' +
      (rows ? '<dl class="hc-sum">' + rows + '</dl>' : '') + '<p class="hc-note">' + whose + (this.practice ? '' : ' You can change any of it on the sheet later.') + '</p>' +
      '<p class="hc-error" role="alert" hidden data-error></p>';
  };

  // --- The leaf ---
  Creator.prototype.tabs = function (c) {
    var t = [['overview', 'Overview']], props = c.properties || {}, bl = buyList(props);
    Object.keys(props).sort().forEach(function (k) {
      if (namedList(props[k])) t.push([k, bl && bl.key === k ? 'Choose ' + lower(words(k)) : words(k)]);
    });
    return t;
  };

  Creator.prototype.overviewHTML = function (st, c) {
    var props = c.properties || {}, bl = buyList(props);
    var facts = Object.keys(props).filter(function (k) {
      return !/_display$/.test(k) && isScalar(props[k]);
    }).slice(0, 6).map(function (k) {
      return '<div><b>' + esc(props[k]) + '</b><span class="hc-k">' + esc(bl && k === bl.budgetKey ? 'Points to spend' : words(k)) + '</span></div>';
    }).join('');
    var body = '';
    if (c.description) {
      body = c.source === 'campaign' ? '<div class="hc-text">' + c.description + '</div>'
        : (c.description.trim() !== (c.summary || '').trim() ? '<div class="hc-text">' + prose(c.description) + '</div>' : '');
    }
    return '<div class="hc-kind"><span class="paper-kind">' + esc(st.label) + ' · ' + (c.source === 'campaign' ? 'this campaign' : esc(this.plan.systemName || 'game system')) + '</span>' +
      (c.source === 'campaign' ? '<span class="hc-seal">Campaign</span>' : '') + '</div>' +
      '<h2 class="hc-name" tabindex="-1">' + esc(c.name) + '</h2>' + (c.summary ? '<p class="hc-lede">' + esc(c.summary) + '</p>' : '') +
      (c.state === 'ask_first' ? (this.practice ? '<p class="hc-note">Ask first at this table. Picking it here is only practice.</p>'
        : '<p class="hc-note">Ask first: your ' + gm() + ' allows it, or suggests another. You can carry on with the rest of your hero meanwhile.</p>') : '') +
      (c.state === 'not_offered' ? '<p class="hc-note">Not offered at this table. It stays a practice pick, and needs your ' + gm() + '’s yes to play.</p>' : '') +
      (facts ? '<div class="hc-facts">' + facts + '</div>' : '') + body;
  };

  Creator.prototype.listHTML = function (st, c, key) {
    var props = c.properties || {}, bl = buyList(props), items = props[key] || [];
    if (!(bl && bl.key === key)) {
      return '<div class="hc-sec"><span class="hc-k">' + esc(words(key)) + '</span>' + items.map(function (it) {
        return '<div class="hc-item"><b>' + esc(it.name) + '</b>' + (itemText(it) ? '<p>' + prose(itemText(it)).replace(/^<p>|<\/p>$/g, '') + '</p>' : '') + '</div>';
      }).join('') + '</div>';
    }
    var mine = this.mine(st.fieldKey, c), spent = this.spent(st.fieldKey, c, bl), left = bl.budget - spent;
    var dots = '';
    for (var i = 0; i < bl.budget && i < 12; i++) dots += '<i class="' + (i < spent ? 'is-used' : '') + '"></i>';
    var qb = Array.isArray(props.quick_build) && props.quick_build.length
      ? '<div class="hc-qb">Not sure? <button type="button" class="hc-link" data-act="quick">Use the suggested set</button> (' + esc(props.quick_build.join(', ')) + ')</div>' : '';
    return '<div class="hc-sec"><span class="hc-k">' + esc(words(key)) + ' <span class="hc-pts" aria-live="polite">' + dots + ' ' + left + ' of ' + bl.budget + ' left</span></span>' + qb +
      items.map(function (it) {
        var on = mine.indexOf(it.name) >= 0;
        return '<label class="hc-opt"><input type="checkbox" data-option="' + esc(it.name) + '"' + (on ? ' checked' : '') + (!on && it.cost > left ? ' disabled' : '') + '>' +
          '<b>' + esc(it.name) + '</b><span class="hc-opt__c">' + it.cost + ' pt' + (it.cost === 1 ? '' : 's') + '</span>' +
          (itemText(it) ? '<p>' + prose(itemText(it)).replace(/^<p>|<\/p>$/g, '') + '</p>' : '') + '</label>';
      }).join('') + '</div>';
  };

  Creator.prototype.fillLeaf = function () {
    var st = this.cur(), c = st.choices[this.open], leaf = this.root.querySelector('.hc-leaf');
    var tabs = this.tabs(c), self = this;
    if (!tabs.some(function (t) { return t[0] === self.tab; })) this.tab = 'overview';
    var pane = this.tab === 'overview' ? this.overviewHTML(st, c) : this.listHTML(st, c, this.tab);
    var p = this.picks[st.fieldKey], isPicked = p && lower(p.name) === lower(c.name);
    var bl = buyList(c.properties);
    var toBuy = this.tab === 'overview' && bl ? '<button type="button" class="hc-link" data-tab="' + esc(bl.key) + '">Choose ' + esc(lower(words(bl.key))) + ' ›</button>' : '';
    leaf.innerHTML = '<button type="button" class="hc-leaf__x" data-act="close" aria-label="Put the leaf back">×</button>' +
      (tabs.length > 1 ? '<div class="hc-ribbons" role="tablist">' + tabs.map(function (t) {
        return '<button type="button" role="tab" data-tab="' + esc(t[0]) + '" aria-selected="' + (self.tab === t[0]) + '">' + esc(t[1]) + '</button>';
      }).join('') + '</div>' : '<div class="hc-ribbons hc-ribbons--none"></div>') +
      '<div class="hc-leaf__body"><div class="hc-pane" role="tabpanel">' + pane + '</div></div>' +
      '<div class="hc-leaf__foot">' + toBuy + '<span class="hc-foot__sp"></span>' +
      '<button type="button" class="hc-btn" data-act="choose"' + (isPicked ? ' aria-pressed="true"' : '') + (isPicked && this.locked(st) ? ' disabled' : '') + '>' +
      (c.state === 'ask_first' && !this.practice ? (isPicked ? '✓ Asking for ' : 'Ask for ') + esc(c.name)
        : isPicked ? '✓ ' + esc(c.name) + (this.locked(st) ? ', everyone starts with this' : ' chosen') : 'Choose ' + esc(c.name)) + '</button></div>';
  };

  // The visible part of whatever scrolls the page: the app's main column, or
  // the window when nothing inside it scrolls.
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
  // leaf stops above it instead of running under.
  function dockedBar(viewBottom) {
    var bar = document.getElementById('tt-drawer');
    if (!bar || getComputedStyle(bar).position !== 'fixed') return 0;
    return Math.max(0, viewBottom - bar.getBoundingClientRect().top);
  }

  Creator.prototype.openLeaf = function (i) {
    if (this.open !== i) this.tab = 'overview';
    this.open = i;
    this.fillLeaf();
    var wrap = this.root.querySelector('.hc-wrap'), leaf = this.root.querySelector('.hc-leaf');
    this.root.querySelectorAll('.hc-row').forEach(function (r) { r.setAttribute('aria-expanded', String(r.getAttribute('data-open') === String(i))); });
    leaf.setAttribute('aria-hidden', 'false');
    // On a narrow screen the leaf covers the page, so it rises where the
    // reader has scrolled to rather than at the page's top.
    if (leaf.offsetWidth >= wrap.offsetWidth - 1) {
      var view = scrollerRect(wrap), wtop = wrap.getBoundingClientRect().top;
      // The leaf starts at the page's top or the reader's, whichever is lower,
      // and ends 12px above the bottom of what is visible.
      var from = Math.max(wtop, view.top + 12);
      leaf.style.setProperty('--hc-sheet-h', Math.max(320, view.top + view.height - 12 - from) + 'px');
      var top = Math.min(wrap.offsetHeight - leaf.offsetHeight, from - wtop);
      leaf.style.setProperty('--hc-sheet-top', Math.max(0, top) + 'px');
    } else {
      leaf.style.removeProperty('--hc-sheet-top');
      leaf.style.removeProperty('--hc-sheet-h');
    }
    requestAnimationFrame(function () { wrap.classList.add('is-open'); leaf.classList.add('is-open'); });
    var h = leaf.querySelector('.hc-name');
    if (h) h.focus({ preventScroll: true });
  };

  Creator.prototype.closeLeaf = function () {
    var was = this.open, wrap = this.root.querySelector('.hc-wrap'), leaf = this.root.querySelector('.hc-leaf');
    this.open = null;
    if (!leaf) return;
    wrap.classList.remove('is-open');
    leaf.classList.remove('is-open');
    leaf.setAttribute('aria-hidden', 'true');
    this.root.querySelectorAll('.hc-row').forEach(function (r) { r.setAttribute('aria-expanded', 'false'); });
    var row = this.root.querySelector('.hc-row[data-open="' + was + '"]');
    if (row) row.focus({ preventScroll: true });
  };

  // Re-draws what a pick changes, leaving the leaf where it is.
  Creator.prototype.refresh = function () {
    var st = this.cur(), self = this;
    var trail = this.root.querySelector('.hc-trail');
    if (trail) { trail.outerHTML = this.trailHTML(); this.watchTrail(); }
    var foot = this.root.querySelector('.hc-foot');
    if (foot) foot.outerHTML = this.footHTML();
    var notes = this.root.querySelector('.hc-notes');
    if (notes) notes.outerHTML = this.notesHTML(st);
    this.root.querySelectorAll('.hc-row').forEach(function (r) {
      var c = st.choices[+r.getAttribute('data-open')], p = self.picks[st.fieldKey];
      var b = r.querySelector('b'), mark = b.querySelector('.hc-row__pick');
      var on = p && lower(p.name) === lower(c.name);
      if (on && !mark) b.insertAdjacentHTML('beforeend', ' <span class="hc-row__pick" aria-label="chosen">✓</span>');
      if (!on && mark) mark.remove();
    });
    if (this.open !== null) {
      var body = this.root.querySelector('.hc-leaf__body'), top = body ? body.scrollTop : 0;
      this.fillLeaf();
      body = this.root.querySelector('.hc-leaf__body');
      if (body) { body.scrollTop = top; var pane = body.querySelector('.hc-pane'); if (pane) pane.style.animation = 'none'; }
    }
    this.save();
  };

  Creator.prototype.choose = function () {
    var st = this.cur(), c = st.choices[this.open], p = this.picks[st.fieldKey];
    if (!p || lower(p.name) !== lower(c.name)) this.picks[st.fieldKey] = { name: c.name, options: [] };
    this.refresh();
    this.announce(c.name + ' chosen');
    // Where the leaf covers the page, choosing puts it away so the step's
    // Next button is in reach; beside the page it stays open to re-read.
    var wrap = this.root.querySelector('.hc-wrap'), leaf = this.root.querySelector('.hc-leaf');
    if (leaf && leaf.offsetWidth >= wrap.offsetWidth - 1) this.closeLeaf();
  };

  Creator.prototype.toggleOption = function (name, on) {
    var st = this.cur(), c = st.choices[this.open], p = this.picks[st.fieldKey];
    if (!p || lower(p.name) !== lower(c.name)) { p = { name: c.name, options: [] }; this.picks[st.fieldKey] = p; }
    if (on && p.options.indexOf(name) < 0) p.options.push(name);
    if (!on) p.options = p.options.filter(function (o) { return o !== name; });
    this.refresh();
  };

  Creator.prototype.quick = function () {
    var st = this.cur(), c = st.choices[this.open], bl = buyList(c.properties);
    var names = (c.properties.quick_build || []).filter(function (n) { return bl && bl.items.some(function (it) { return it.name === n; }); });
    this.picks[st.fieldKey] = { name: c.name, options: names };
    this.refresh();
    this.announce('Suggested set chosen: ' + names.join(', '));
  };

  Creator.prototype.go = function (i) {
    this.step = Math.max(0, Math.min(i, this.stepCount() - 1));
    this.render();
    var h = this.root.querySelector('.hc-head h1');
    if (h) h.focus({ preventScroll: true });
    var top = this.root.getBoundingClientRect().top;
    if (top < 0) this.root.scrollIntoView({ block: 'start' });
  };

  Creator.prototype.announce = function (msg) {
    var live = this.root.querySelector('.hc-live');
    if (!live) { live = document.createElement('div'); live.className = 'hc-live sr-only'; live.setAttribute('aria-live', 'polite'); this.root.appendChild(live); }
    live.textContent = msg;
  };

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
    var body = { name: name, picks: picks };
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

  Creator.prototype.wire = function () {
    var self = this, root = this.root;
    root.addEventListener('click', function (e) {
      var t = e.target.closest('button');
      if (!t || !root.contains(t)) return;
      if (t.hasAttribute('data-open')) { var i = +t.getAttribute('data-open'); if (self.open === i) self.closeLeaf(); else self.openLeaf(i); return; }
      if (t.hasAttribute('data-step')) { self.go(+t.getAttribute('data-step')); return; }
      if (t.hasAttribute('data-tab')) {
        self.tab = t.getAttribute('data-tab');
        self.fillLeaf();
        var tab = root.querySelector('.hc-ribbons [data-tab="' + self.tab + '"]');
        if (tab) tab.focus();
        return;
      }
      switch (t.getAttribute('data-act')) {
        case 'close': self.closeLeaf(); break;
        case 'choose': self.choose(); break;
        case 'quick': self.quick(); break;
        case 'next': self.go(self.step + 1); break;
        case 'back': self.go(self.step - 1); break;
        case 'create': self.create(); break;
        case 'nothere':
          var st = self.cur(), nh = root.querySelector('.hc-nh');
          self.nh[st.fieldKey] = !self.nh[st.fieldKey];
          if (nh) nh.outerHTML = self.notHereHTML(st);
          var again = root.querySelector('.hc-nh .hc-link');
          if (again) again.focus();
          break;
      }
    });
    root.addEventListener('change', function (e) {
      var t = e.target;
      if (t.hasAttribute && t.hasAttribute('data-option')) self.toggleOption(t.getAttribute('data-option'), t.checked);
    });
    root.addEventListener('input', function (e) {
      if (e.target.hasAttribute && e.target.hasAttribute('data-name')) {
        self.name = e.target.value;
        var trail = root.querySelector('.hc-trail');
        if (trail) { trail.outerHTML = self.trailHTML(); self.watchTrail(); }
        self.save();
      }
    });
    root.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && self.open !== null) { e.preventDefault(); self.closeLeaf(); }
      if (e.key === 'Enter' && e.target.hasAttribute && e.target.hasAttribute('data-name')) { e.preventDefault(); self.create(); }
    });
    // A click on the page outside the leaf puts it back.
    document.addEventListener('click', function (e) {
      if (self.open === null || !document.body.contains(root)) return;
      if (e.target.closest('.hc-leaf') || e.target.closest('.hc-row')) return;
      if (root.contains(e.target)) self.closeLeaf();
    });
  };

  // Loaded on sight (ADR-063): boot.js fetches this script when it meets the
  // page's data-widget mount, then mounts it through register().
  function register() {
    Chronicle.register('hero-creator', {
      init: function (el) {
        if (el._heroCreator) return;
        var p = new Creator(el);
        el._heroCreator = p;
        p.wire();
        p.load();
      }
    });
  }
  if (window.Chronicle && Chronicle.register) register();
})();
