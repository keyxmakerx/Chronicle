/**
 * own_entries.js — the "Own entries" page.
 *
 * A page in the site's plain look with a tab per pick list the game system
 * has (Ancestry, Kit…, named by the system). Opening an entry, or "Add",
 * grows an editor over the page from the row clicked: the form on the left,
 * and on the right what a player sees in the hero creator, where the points
 * can be test-spent. The form has the parts the system's own entries share
 * (facts, lists of traits with costs, the points budget), so a homebrew
 * ancestry is bought from in the creator exactly like a packaged one.
 * "Paste from an AI" reads the AI Import format into the form for checking.
 *
 * A Director can also let one player write one entry of their own for a
 * list, with the points budget the Director sets. That player opens the same
 * page and editor, limited to their own entries: they save a draft and hand
 * it in, and the Director accepts it or hands it back with a note. Until it
 * is accepted only they and the Directors see it.
 *
 * The pick lists, their labels and the system's own entries come from the
 * creator's plan; the campaign's entries and the permissions are read and
 * written through the systems plugin's entry API, which also enforces who
 * may write what and validates every list. Descriptions are HTML the server sanitizes; a
 * passage wrapped in <span data-secret> is kept from players, as on pages.
 *
 * A Director's tab also lists every entry the game system brings, beside the
 * campaign's own, each with its mark (Open, Ask first, Not here, Everyone
 * takes it) set from the row's menu, for everyone or for one player. The marks
 * are the campaign's hero-choice rules, read and written through the
 * choice-rules API; a request a player makes for an Ask-first entry waits
 * beside the handed-in drafts, to allow or answer.
 *
 * An entry can hold more than this form edits (a rewritten copy of a
 * system's own entry carries ability objects, rolls, extra keys on traits).
 * What the form does not show goes back on save exactly as it was loaded, and
 * a trait keeps its extra keys while its name is unchanged, so a save never
 * drops a part the Director could not see.
 *
 * Served from the plugin body-script registry so it survives boosted
 * navigation. Re-wires after HTMX swaps.
 */
(function () {
  'use strict';

  var DIRTY = 'own-entry';

  // The parts of an entry the form holds and sends are shared with the editor
  // players write theirs in (own_entry_editor.js, listed before this script).
  var OE = window.ChronicleOwnEntry;
  var namedList = OE.namedList, shapeOf = OE.shapeOf, isDraft = OE.isDraft, draftOf = OE.draftOf, listsOf = OE.listsOf,
    propsOf = OE.propsOf, keptLine = OE.keptLine, tidy = OE.tidy;

  function esc(v) { return Chronicle.escapeHtml(v); }
  // The campaign's own word for its game master, already HTML-safe.
  function gm() { return window.Chronicle && Chronicle.gmTerm ? Chronicle.gmTerm() : 'GM'; }
  function words(key) {
    var s = String(key).replace(/_display$/, '').replace(/[_-]+/g, ' ').trim();
    return s.charAt(0).toUpperCase() + s.slice(1);
  }
  function lower(s) { return String(s || '').toLowerCase(); }
  function an(w) { return /^[aeiou]/i.test(w) ? 'an' : 'a'; }
  function dirty(on) {
    if (!window.Chronicle) return;
    if (on && Chronicle.markDirty) Chronicle.markDirty(DIRTY);
    if (!on && Chronicle.markClean) Chronicle.markClean(DIRTY);
  }
  function notify(msg, kind) { if (window.Chronicle && Chronicle.notify) Chronicle.notify(msg, kind || 'success'); }
  function api(url, opts) {
    if (window.Chronicle && Chronicle.apiFetch) return Chronicle.apiFetch(url, opts);
    var o = { method: opts && opts.method || 'GET', credentials: 'same-origin', headers: { Accept: 'application/json' } };
    if (opts && opts.body) { o.headers['Content-Type'] = 'application/json'; o.body = JSON.stringify(opts.body); }
    if (opts && opts.csrfToken) o.headers['X-CSRF-Token'] = opts.csrfToken;
    return fetch(url, o);
  }
  function json(r) {
    return r.json().catch(function () { return {}; }).then(function (b) { return { ok: r.ok, status: r.status, body: b }; });
  }


  function Entries(root) {
    this.root = root;
    this.planUrl = root.getAttribute('data-plan-url');
    this.entriesUrl = root.getAttribute('data-entries-url');
    this.csrf = root.getAttribute('data-csrf') || '';
    this.membersUrl = root.getAttribute('data-members-url');
    this.director = root.getAttribute('data-director') === 'true';
    this.user = root.getAttribute('data-user') || '';
    this.startField = root.getAttribute('data-field') || '';
    this.forId = root.getAttribute('data-for-id') || '';
    this.forName = root.getAttribute('data-for-name') || '';
    this.backUrl = root.getAttribute('data-back-url') || '';
    // Opened by address: straight on one entry (the page's pen, or landing
    // after a rewrite), or on a new one; returnUrl is the page to go back to.
    this.startEntry = root.getAttribute('data-entry') || '';
    this.startNew = root.getAttribute('data-new') === 'true';
    this.made = root.getAttribute('data-made') === 'true';
    this.returnUrl = root.getAttribute('data-return-url') || '';
    this.grants = [];   // permissions: all for a Director, the player's own
    this.members = [];  // a Director's list of who can be given one
    this.giving = false;
    this.system = '';
    this.fields = [];   // {key, label, pkg, facts, lists, budgetKey, budget, sample}
    this.entries = [];  // the campaign's own, every field
    this.field = 0;
    this.open = null;   // entry id, or 'new'
    this.draft = null;
    this.confirm = false;
    this.paste = false;
    this.tryBuy = [];   // traits ticked in the player's view, never saved
    this.busy = false;
    // A Director's marks: each list's entries with their marks per field and
    // viewer, the requests waiting for an answer, and what is open on the page.
    this.apiBase = this.entriesUrl ? this.entriesUrl.replace(/\/system-entries$/, '') : '';
    this.rules = {};
    this.rulesErr = {};
    this.rulesBusy = {};
    this.requests = [];
    this.as = '';       // the player the list is shown as; '' is everyone
    this.ui = { menu: null, strip: null, suggest: null, q: '', marked: false, more: 40 };
  }

  Entries.prototype.load = function () {
    var self = this;
    var reads = [api(this.planUrl).then(json), api(this.entriesUrl).then(json), api(this.entriesUrl + '/grants').then(json)];
    if (this.director) {
      reads.push(api(this.membersUrl, { headers: { Accept: 'application/json' } }).then(json));
      // The requests are a part of the page; failing to read them never stops it.
      reads.push(api(this.apiBase + '/choice-requests').then(json).catch(function () { return { ok: false }; }));
    }
    Promise.all(reads).then(function (res) {
      if (!res[0].ok || !res[1].ok || !res[2].ok) throw new Error('load');
      var plan = res[0].body || {};
      self.system = plan.systemName || 'The game system';
      self.fields = (plan.steps || []).map(function (st) {
        var pkg = (st.choices || []).filter(function (c) { return c.source !== 'campaign'; });
        var sh = shapeOf(pkg);
        return { key: st.fieldKey, label: st.label, pkg: pkg.length, facts: sh.facts, lists: sh.lists,
          budgetKey: sh.budgetKey, budget: sh.budget, sample: pkg[0] || null };
      });
      self.entries = (res[1].body && res[1].body.entries) || [];
      self.grants = (res[2].body && res[2].body.grants) || [];
      if (res[3] && res[3].ok && Array.isArray(res[3].body)) self.members = res[3].body;
      if (res[4] && res[4].ok && res[4].body && Array.isArray(res[4].body.requests)) self.requests = res[4].body.requests;
      if (!self.director) {
        // A player's page holds only what they were let write, and what they wrote.
        self.entries = self.entries.filter(function (e) { return e.createdBy === self.user; });
        self.fields = self.fields.filter(function (f) { return self.grantFor(f.key) || self.mine(f.key).length; });
      }
      var start = self.startEntry ? self.entries.filter(function (e) { return String(e.id) === self.startEntry; })[0] : null;
      var key = start ? start.fieldKey : self.startField;
      for (var i = 0; i < self.fields.length; i++) if (self.fields[i].key === key) self.field = i;
      // With no list asked for, open where something waits, else where the
      // campaign has entries of its own, else the first list.
      if (!key && self.director) {
        var pick = function (test) {
          for (var j = 0; j < self.fields.length; j++) if (self.mine(self.fields[j].key).some(test)) return j;
          return -1;
        };
        var at = pick(function (e) { return e.status === 'sent'; });
        for (var q = 0; at < 0 && q < self.fields.length; q++) if (self.waitingFor(self.fields[q].key).length) at = q;
        if (at < 0) at = pick(function (e) { return !isDraft(e); });
        if (at >= 0) self.field = at;
      }
      self.render();
      if (start && self.cur() && self.cur().key === start.fieldKey) { self.openPop(start, null); self.leaveTo = self.returnUrl; }
      else if (self.startNew && self.cur()) { self.openPop(null, self.root.querySelector('[data-act="add"]')); self.leaveTo = self.returnUrl; }
    }).catch(function () {
      var t = self.root.querySelector('.hc-loading__text');
      if (t) t.textContent = 'Your entries could not be opened. Reload the page to try again.';
    });
  };

  Entries.prototype.cur = function () { return this.fields[this.field]; };
  // The player's own permission for a list (a Director has many, by player).
  Entries.prototype.grantFor = function (key) {
    var self = this;
    return this.grants.filter(function (g) { return g.fieldKey === key && (self.director || g.userId === self.user); })[0] || null;
  };
  Entries.prototype.memberName = function (id) {
    var m = this.members.filter(function (x) { return x.user_id === id; })[0];
    return m ? (m.display_name || 'A player') : 'A player';
  };
  // Entries a Director has accepted, and drafts handed in: the two numbers
  // Campaign settings shows, so a tab and that row always agree.
  Entries.prototype.accepted = function (key) {
    return this.mine(key).filter(function (e) { return !isDraft(e); });
  };
  Entries.prototype.handedIn = function (key) {
    return this.mine(key).filter(function (e) { return e.status === 'sent'; });
  };
  Entries.prototype.mine = function (key) {
    return this.entries.filter(function (e) { return e.fieldKey === key; })
      .sort(function (a, b) { return lower(a.name) < lower(b.name) ? -1 : 1; });
  };

  function plural(w) {
    w = lower(w);
    if (/[^aeiou]y$/.test(w)) return w.slice(0, -1) + 'ies';
    if (/(s|x|z|ch|sh)$/.test(w)) return w + 'es';
    return w + 's';
  }
  function sameDraft(a, b) { return JSON.stringify(a) === JSON.stringify(b); }

  // The page, and the editor that grows over it from the row clicked.
  Entries.prototype.render = function () {
    var open = this.open !== null;
    this.root.innerHTML = '<div class="oe-desk' + (open ? ' is-open' : '') + '"><div class="hc-wrap paper-stack">' + this.pageHTML() + '</div>' +
      '<div class="oe-scrim" data-act="close" aria-hidden="true"></div>' +
      '<section class="oe-pop" role="dialog" aria-modal="true" aria-label="Entry"' + (open ? '' : ' hidden') + '></section></div>';
    if (open) this.fillPop();
  };

  Entries.prototype.pageHTML = function () {
    if (!this.director) return this.playerPageHTML();
    var self = this, f = this.cur();
    var head = '<div class="hc-head"><p class="paper-kind">' + esc(this.system) + ' · ' + gm() + 's only</p><h1 tabindex="-1">Own entries</h1>' +
      '<p>Entries you add here sit beside ' + esc(this.system) + '’s own in the hero creator and the sheet’s pick lists. A system update never changes them. Players’ drafts wait here for you, and you let players make their own. Mark any entry to limit what players can pick, or to ask them to take it.</p></div>';
    if (!f) {
      return '<section class="hc-page paper" aria-label="Own entries">' + head +
        '<p class="hc-empty">' + esc(this.system) + ' doesn’t list any choices for making a hero, so there is nothing to add to here.</p></section>';
    }
    var tabs = this.fields.map(function (x, i) {
      var waiting = self.handedIn(x.key).length + self.waitingFor(x.key).length;
      return '<button type="button" role="tab" data-field="' + i + '" aria-selected="' + (i === self.field) + '">' + esc(x.label) +
        '<small>' + self.accepted(x.key).length + (waiting ? ' · ' + waiting + ' waiting' : '') + '</small></button>';
    }).join('');
    var mine = this.mine(f.key), pl = plural(f.label), data = this.rulesFor(f.key);
    var list;
    if (data) list = this.listHTML(data, f);
    else if (this.rulesErr[this.ck(f.key)]) {
      list = '<p class="hc-empty" role="alert">' + esc(this.rulesErr[this.ck(f.key)]) + ' <button type="button" class="oe-link" data-act="retryrules">Try again</button></p>';
    } else list = '<p class="oe-hint" role="status">Opening the ' + esc(lower(f.label)) + ' list…</p>';
    return '<section class="hc-page paper" aria-label="Own entries">' + head +
      this.draftsWaitingHTML() + '<div class="oe-fields" role="tablist" aria-label="Pick lists">' + tabs + '</div>' + this.waitingHTML(f) + list +
      this.grantsHTML() +
      '<div class="hc-foot"><span>' + this.accepted(f.key).length + ' of your own</span><span class="hc-foot__sp"></span>' +
      '<button type="button" class="hc-btn" data-act="add">+ Add ' + an(f.label) + ' ' + esc(lower(f.label)) + '</button></div></section>';
  };

  // --- A Director's marks ---------------------------------------------------
  // Each list's rows come from the choice-rules API: the system's entries and
  // the campaign's own, each with the mark that applies, and the marks set
  // for single players. Nothing here decides who may pick what; the server
  // does, so this only draws what it says and sends what the Director sets.

  var STATES = [['open', 'Open'], ['ask_first', 'Ask first'], ['not_offered', 'Not here'], ['required', 'Everyone takes it']];
  function stateWord(st) {
    for (var i = 0; i < STATES.length; i++) if (STATES[i][0] === st) return STATES[i][1];
    return st === 'left_out' ? 'Not on the short list' : '';
  }
  // A mark's words, aimed at one player when there is one.
  function markLabel(st, who) {
    if (!who) return stateWord(st);
    if (st === 'required') return who + ' takes it';
    return stateWord(st) + ' for ' + who;
  }
  function byAttr(root, sel, attr, val) {
    var nodes = root.querySelectorAll(sel);
    for (var i = 0; i < nodes.length; i++) if (nodes[i].getAttribute(attr) === val) return nodes[i];
    return null;
  }

  Entries.prototype.ck = function (key) { return key + '|' + this.as; };
  Entries.prototype.rulesFor = function (key) {
    var ck = this.ck(key);
    if (this.rules[ck]) return this.rules[ck];
    if (!this.rulesBusy[ck] && !this.rulesErr[ck]) this.fetchRules(key);
    return null;
  };
  // Reads one list's marks. What is on screen stays until the answer lands,
  // so a mark that was just saved never flashes an empty list.
  Entries.prototype.fetchRules = function (key, then) {
    var self = this, ck = this.ck(key);
    this.rulesBusy[ck] = true;
    delete this.rulesErr[ck];
    api(this.apiBase + '/choice-rules/entries?fieldKey=' + encodeURIComponent(key) + (this.as ? '&as=' + encodeURIComponent(this.as) : '')).then(json).then(function (res) {
      delete self.rulesBusy[ck];
      if (res.ok && res.body && res.body.entries) self.rules[ck] = res.body;
      else self.rulesErr[ck] = res.body.message || res.body.error || 'The marks could not be opened.';
    }).catch(function () {
      delete self.rulesBusy[ck];
      self.rulesErr[ck] = 'The marks could not be opened. Check your connection and try again.';
    }).then(function () {
      if (then) then(); else if (self.open === null) self.render();
    });
  };
  // After anything that changes the entries or the marks: the other lists'
  // copies are stale, the shown one is read again in place.
  Entries.prototype.reloadRules = function (then) {
    var f = this.cur(), keep = f && this.rules[this.ck(f.key)], mine = {};
    if (keep) mine[this.ck(f.key)] = keep;
    this.rules = mine;
    this.rulesErr = {};
    if (f) this.fetchRules(f.key, then);
    else if (then) then();
  };
  Entries.prototype.reloadRequests = function (then) {
    var self = this;
    api(this.apiBase + '/choice-requests').then(json).then(function (res) {
      if (res.ok && res.body && Array.isArray(res.body.requests)) self.requests = res.body.requests;
    }).catch(function () {}).then(function () { if (then) then(); });
  };
  Entries.prototype.waitingFor = function (key) {
    return this.requests.filter(function (r) { return r.fieldKey === key; });
  };
  Entries.prototype.memberLabel = function (data, id) {
    var m = (data.members || []).filter(function (x) { return x.userId === id; })[0];
    return m ? m.name : this.memberName(id);
  };
  function firstName(n) { return String(n || 'them').trim().split(/\s+/)[0] || 'them'; }

  // Players' Ask-first requests wait here, beside the handed-in drafts below.
  Entries.prototype.waitingHTML = function (f) {
    var self = this, rs = this.waitingFor(f.key), lab = lower(f.label);
    if (!rs.length) return '';
    return '<div class="oe-wait" role="region" aria-label="Waiting for you"><p class="oe-k">Waiting for you</p><ul>' + rs.map(function (r) {
      var sg = String(self.ui.suggest) === String(r.id), who = r.playerName || 'A player';
      return '<li><p><b>' + esc(who) + '</b> asks for <b>' + esc(r.entryName) + '</b> as ' +
        (r.entityName ? esc(r.entityName) + '’s ' : 'their ') + esc(lab) + '.</p>' +
        '<div class="oe-req__acts"><button type="button" class="oe-btn" data-act="allow" data-req="' + esc(r.id) + '">Allow for ' + esc(firstName(who)) + '</button>' +
        '<button type="button" class="oe-link" data-act="suggest" data-req="' + esc(r.id) + '" aria-expanded="' + sg + '">Suggest another</button></div>' +
        (sg ? '<div class="oe-suggest"><label class="oe-f"><span class="oe-k">What ' + esc(firstName(who)) + ' reads</span>' +
          '<input type="text" data-suggest-msg maxlength="300" autocomplete="off" placeholder="A short note, such as which entry fits the table better"></label>' +
          '<div class="oe-req__acts"><button type="button" class="oe-btn" data-act="sendsuggest" data-req="' + esc(r.id) + '">Send</button>' +
          '<button type="button" class="oe-link" data-act="nosuggest">Cancel</button></div></div>' : '') + '</li>';
    }).join('') + '</ul></div>';
  };

  // Handed-in drafts of every list, at the top of the page so a Director need
  // not open each tab to find them.
  Entries.prototype.draftsWaitingHTML = function () {
    var self = this, rows = [];
    this.fields.forEach(function (f) {
      self.handedIn(f.key).forEach(function (e) { rows.push({ e: e, f: f }); });
    });
    if (!rows.length) return '';
    return '<div class="oe-wait oe-wait--drafts" role="region" aria-label="Waiting for you"><p class="oe-k">Waiting for you</p><ul>' + rows.map(function (r) {
      return '<li><p><b>' + esc(r.e.name) + '</b> <span class="oe-s">' + esc(r.f.label) + '</span><br><span class="oe-s">A draft from ' + esc(self.memberName(r.e.createdBy)) + '</span></p>' +
        '<div class="oe-req__acts"><button type="button" class="oe-link" data-readdraft="' + esc(r.e.id) + '">Read</button></div></li>';
    }).join('') + '</ul></div>';
  };

  // The list: a search and who it is shown as, then the rows.
  Entries.prototype.listHTML = function (data, f) {
    var self = this, ui = this.ui, pl = plural(f.label);
    var shown = '<label class="oe-show"><span class="oe-k">Showing</span><select data-show aria-label="Show the list as">' +
      '<option value="">Everyone</option>' + (data.members || []).map(function (m) {
        return '<option value="' + esc(m.userId) + '"' + (m.userId === self.as ? ' selected' : '') + '>' + esc(m.name) + '</option>';
      }).join('') + '</select></label>';
    var marked = data.entries.filter(function (e) { return self.markedEntry(e); }).length;
    var ctl = '<div class="oe-ctl"><label class="sr-only" for="oe-q">Search the ' + esc(lower(f.label)) + ' list</label>' +
      '<input id="oe-q" type="search" data-oe-q autocomplete="off" placeholder="Search ' + data.entries.length + ' ' + esc(data.entries.length === 1 ? lower(f.label) : pl) + '" value="' + esc(ui.q) + '">' + shown +
      (marked ? '<button type="button" class="oe-link" data-act="onlymarked" aria-pressed="' + ui.marked + '">Only marked (' + marked + ')</button>' : '') + '</div>';
    var foot = '';
    (data.warnings || []).forEach(function (w) { foot += '<p class="oe-pkg">' + esc(w) + '</p>'; });
    (data.orphans || []).forEach(function (o, i) {
      foot += '<p class="oe-pkg">' + esc(o.name || o.key) + ' was marked ' + esc(lower(stateWord(o.state))) + (o.target ? ' for ' + esc(self.memberLabel(data, o.target)) : '') +
        ' and is no longer in ' + esc(self.system) + '. <button type="button" class="oe-link" data-act="clearorphan" data-orphan="' + i + '">Clear it</button></p>';
    });
    return ctl + '<div class="oe-rows" data-rows>' + this.rowsHTML(data, f) + '</div>' + foot;
  };

  // Whether an entry carries a mark of its own, for anyone.
  Entries.prototype.markedEntry = function (e) {
    return !!(e.everyone && e.everyone.state !== 'open') || (e.players && e.players.length > 0) || e.state !== 'open';
  };

  Entries.prototype.rowsHTML = function (data, f) {
    var self = this, ui = this.ui, q = lower(ui.q).trim();
    var groups = (data.groups || []).map(function (g) {
      var note = g.source === 'campaign' ? 'Your homebrew, and anything you write later.' : esc(self.system) + '’s own, and anything a package update adds.';
      return self.rowHTML({ group: true, ref: g.source + ':' + g.key, source: g.source, key: g.key, name: g.label, note: note,
        everyone: g.everyone, players: g.players || [], state: g.everyone ? g.everyone.state : 'open', reason: g.everyone && g.everyone.reason }, data);
    }).join('');
    var list = data.entries.filter(function (e) {
      if (ui.marked && !self.markedEntry(e)) return false;
      return !q || lower(e.name + ' ' + (e.summary || '')).indexOf(q) >= 0;
    });
    var rows = list.slice(0, ui.more).map(function (e) {
      return self.rowHTML({ ref: e.source + ':' + e.key, source: e.source, key: e.key, name: e.name, e: e,
        everyone: e.everyone, players: e.players || [], state: e.state, reason: e.reason }, data);
    }).join('');
    var more = list.length > ui.more ? '<p class="oe-more"><button type="button" class="oe-link" data-act="more">Show ' + Math.min(list.length - ui.more, 60) + ' more</button>' +
      '<span class="oe-hint"> ' + (list.length - ui.more) + ' not shown</span></p>' : '';
    var none = !list.length ? '<p class="hc-empty">' + (q ? 'Nothing here matches “' + esc(ui.q.trim()) + '”.' : ui.marked ? 'Nothing is marked yet.' : 'No ' + esc(plural(f.label)) + ' to show.') + '</p>' : '';
    var mine = this.mine(f.key);
    var own = !mine.length && !q && !ui.marked ? '<p class="oe-pkg">No ' + esc(plural(f.label)) + ' of your own yet. Add one and players see it next to ' + esc(this.system) + '’s ' + esc(plural(f.label)) + ' when they make a hero.</p>' : '';
    return '<ul class="oe-list oe-groups">' + groups + '</ul><ul class="oe-list oe-ents">' + rows + '</ul>' + none + more + this.restHTML(data) + own;
  };

  Entries.prototype.sealsHTML = function (e) {
    if (!e) return '';
    return (e.status === 'sent' ? ' <span class="hc-seal oe-seal--wait">Waiting for you · ' + esc(this.memberName(e.createdBy)) + '</span>' : '') +
      (e.status === 'draft' ? ' <span class="hc-seal oe-seal--dm">Draft · ' + esc(this.memberName(e.createdBy)) + '</span>' : '') +
      (!isDraft(e) && e.visibility === 'directors' ? ' <span class="hc-seal oe-seal--dm">' + gm() + 's only</span>' : '') +
      (!isDraft(e) && e.visibility === 'character' ? ' <span class="hc-seal oe-seal--dm">For ' + esc(e.forName || 'one character') + '</span>' : '');
  };

  // Marks the entry carries for single players, and why, in a quiet line.
  // Each part is plain text; the caller escapes it.
  Entries.prototype.noteLine = function (ent, data) {
    var self = this, parts = [];
    ent.players.forEach(function (p) {
      if (self.as && p.userId !== self.as) return;
      parts.push(markLabel(p.state, p.playerName || self.memberLabel(data, p.userId)));
    });
    if (ent.reason && ent.state !== 'open') parts.push('Why: ' + ent.reason);
    if (ent.e && ent.e.inherited && ent.state !== 'open') parts.push('Same as the rest of ' + (ent.source === 'campaign' ? 'this campaign’s own' : self.system + '’s own'));
    if (ent.state === 'left_out') parts.push('Not on the short list');
    return parts;
  };

  Entries.prototype.rowHTML = function (ent, data) {
    var ui = this.ui, e = ent.e, st = ent.state || 'open', ref = ent.ref;
    var campaign = ent.source === 'campaign' && !ent.group && e && e.entryId;
    var mineEntry = campaign ? this.entries.filter(function (x) { return String(x.id) === String(e.entryId); })[0] : null;
    var summary = ent.group ? ent.note : (e.summary || '');
    var notes = this.noteLine(ent, data);
    var menuOpen = ui.menu && ui.menu.ref === ref, strip = ui.strip && ui.strip.ref === ref && !ui.strip.bulk;
    var main;
    if (mineEntry) {
      main = '<button type="button" class="hc-row" data-open="' + esc(mineEntry.id) + '" aria-expanded="' + (this.open === String(mineEntry.id)) + '"><b>' + esc(ent.name) + this.sealsHTML(mineEntry) +
        '</b><span class="hc-row__go" aria-hidden="true">' + (mineEntry.status === 'sent' ? 'Review ›' : 'Edit ›') + '</span>' +
        (summary ? '<small>' + esc(summary) + '</small>' : '') + '</button>';
    } else {
      main = '<div class="hc-row oe-ent__static"><b>' + esc(ent.name) + (campaign ? ' <span class="hc-seal">Campaign</span>' : '') + '</b>' +
        (summary ? '<small>' + (ent.group ? summary : esc(summary)) + '</small>' : '') + '</div>';
    }
    var chip = st === 'open' ? '' : '<span class="oe-mark oe-mark--' + esc(st.replace('_', '-')) + '">' + esc(st === 'required' && this.as ? 'Takes it' : stateWord(st)) + '</span>';
    return '<li class="oe-ent' + (ent.group ? ' oe-ent--group' : '') + (st === 'not_offered' ? ' is-no' : '') + (ui.working === ref ? ' is-working' : '') + '" data-ref="' + esc(ref) + '">' +
      '<div class="oe-ent__line">' + main + chip +
      '<button type="button" class="oe-dots" data-menu="' + esc(ref) + '" aria-haspopup="menu" aria-expanded="' + !!menuOpen + '" aria-label="Marks for ' + esc(ent.name) + '"><i class="fa-solid fa-ellipsis" aria-hidden="true"></i></button>' +
      (menuOpen ? this.menuHTML(ent, data) : '') + '</div>' +
      (notes.length ? '<p class="oe-ent__note">' + notes.map(function (n) { return esc(n); }).join(' · ') + '</p>' : '') +
      (strip ? this.stripHTML() : '') + '</li>';
  };

  // The row's menu: the four marks for everyone (or for the player the list
  // is shown as), then "For one player…" and the reason. A second step picks
  // the player and a third their marks; Back returns.
  Entries.prototype.menuItems = function (ent, data) {
    var m = this.ui.menu, items = [];
    if (m.step === 'rest') {
      items.push({ head: ent.restNote });
      items.push({ act: 'restmark', state: 'not_offered', label: 'Not here' });
      items.push({ act: 'restmark', state: 'ask_first', label: 'Ask first' });
      return items;
    }
    if (m.step === 'players') {
      items.push({ act: 'back', label: 'Back' }, { sep: true });
      (data.members || []).forEach(function (p) {
        var mk = ent.players.filter(function (x) { return x.userId === p.userId; })[0];
        items.push({ act: 'player', player: p.userId, label: p.name, note: mk ? stateWord(mk.state) : '' });
      });
      return items;
    }
    var target = m.step === 'player' ? m.player : this.as, who = target ? this.memberLabel(data, target) : '';
    var mk = target ? ent.players.filter(function (x) { return x.userId === target; })[0] : ent.everyone;
    var cur = mk ? mk.state : (target ? null : 'open');
    if (m.step === 'player') items.push({ act: 'back', label: 'Back' }, { sep: true });
    STATES.forEach(function (s) {
      items.push({ act: 'mark', state: s[0], target: target, checked: cur === s[0], label: markLabel(s[0], who) });
    });
    if (target && mk) items.push({ act: 'mark', state: 'clear', target: target, label: 'Same as everyone' });
    var extra = [];
    if (!target && (data.members || []).length) extra.push({ act: 'players', label: 'For one player…' });
    if (mk && mk.state !== 'open') extra.push({ act: 'reason', state: mk.state, target: target, label: mk.reason ? 'Change the reason…' : 'Add a reason…' });
    if (extra.length) items.push({ sep: true });
    return items.concat(extra);
  };

  Entries.prototype.menuHTML = function (ent, data) {
    var items = this.menuItems(ent, data);
    return '<div class="oe-menu" role="menu" aria-label="Marks for ' + esc(ent.name) + '">' + items.map(function (it) {
      if (it.sep) return '<div class="oe-menu__sep" role="separator"></div>';
      if (it.head) return '<p class="oe-menu__h">' + esc(it.head) + '</p>';
      var radio = it.act === 'mark' && it.state !== 'clear';
      return '<button type="button" class="oe-menu__item" role="' + (radio ? 'menuitemradio' : 'menuitem') + '"' + (radio ? ' aria-checked="' + !!it.checked + '"' : '') +
        ' data-mact="' + it.act + '"' + (it.state ? ' data-state="' + it.state + '"' : '') + (it.target != null ? ' data-target="' + esc(it.target) + '"' : '') +
        (it.player ? ' data-player="' + esc(it.player) + '"' : '') + '><span>' + esc(it.label) + '</span>' +
        (it.note ? '<small>' + esc(it.note) + '</small>' : '') + '</button>';
    }).join('') + '</div>';
  };

  // "Mark the rest…": every entry with no mark of its own, marked at once.
  Entries.prototype.restOf = function (data) {
    return data.entries.filter(function (e) {
      return !e.everyone && e.status !== 'draft' && e.status !== 'sent';
    });
  };
  Entries.prototype.restHTML = function (data) {
    if (this.as) return '';
    var rest = this.restOf(data), ui = this.ui;
    if (!rest.length) return '';
    var open = ui.menu && ui.menu.ref === 'rest', strip = ui.strip && ui.strip.bulk;
    var ent = { ref: 'rest', name: 'the rest', players: [], restNote: 'The other ' + rest.length + ' with no mark' };
    return '<div class="oe-rest"><button type="button" class="oe-link" data-menu="rest" aria-haspopup="menu" aria-expanded="' + !!open + '">Mark the rest…</button>' +
      '<span class="oe-hint">' + rest.length + ' with no mark</span>' + (open ? this.menuHTML(ent, data) : '') + (strip ? this.stripHTML() : '') + '</div>';
  };

  // The strip that opens under a row when a mark needs a word from the
  // Director: heroes it would break, an error, and the optional reason.
  Entries.prototype.stripHTML = function () {
    var s = this.ui.strip, c = s.check, h = '<div class="oe-strip" role="group" aria-label="Before this is saved">';
    h += '<p class="oe-strip__what">' + s.what + '</p>';
    if (s.error) h += '<p class="oe-error" role="alert">' + esc(s.error) + '</p>';
    if (c && c.count) {
      var shownN = Math.min(6, (c.heroes || []).length);
      var heroes = (c.heroes || []).slice(0, 6).map(function (x) { return esc(x.heroName) + (x.playerName ? ' (' + esc(x.playerName) + ')' : ''); }).join(', ');
      var more = c.count > shownN ? ', and ' + (c.count - shownN) + ' more' : '';
      h += '<p class="oe-asked" role="note">' + c.count + (c.count === 1 ? ' hero already made breaks' : ' heroes already made break') + ' this: ' + heroes + more + '. They keep what they have.</p>';
    }
    h += '<label class="oe-f"><span class="oe-k">Reason players see (optional)</span><input type="text" data-strip-reason maxlength="300" autocomplete="off" value="' + esc(s.reason || '') + '"></label>' +
      '<div class="oe-req__acts"><button type="button" class="oe-btn" data-act="stripsave">Save</button>' +
      '<button type="button" class="oe-link" data-act="stripno">Cancel</button></div></div>';
    return h;
  };

  // Redraws only the rows, so a half-typed search and the scroll place stay.
  Entries.prototype.redrawRows = function () {
    var f = this.cur(), data = f && this.rules[this.ck(f.key)], box = this.root.querySelector('[data-rows]');
    if (box && data) box.innerHTML = this.rowsHTML(data, f);
  };
  Entries.prototype.focusDots = function (ref) {
    var b = byAttr(this.root, '[data-menu]', 'data-menu', ref);
    if (b) b.focus({ preventScroll: true });
  };
  Entries.prototype.entOf = function (data, ref) {
    var g = (data.groups || []).filter(function (x) { return x.source + ':' + x.key === ref; })[0];
    if (g) return { group: true, ref: ref, source: g.source, key: g.key, name: g.label, everyone: g.everyone, players: g.players || [] };
    var e = data.entries.filter(function (x) { return x.source + ':' + x.key === ref; })[0];
    return e ? { ref: ref, source: e.source, key: e.key, name: e.name, e: e, everyone: e.everyone, players: e.players || [] } : null;
  };
  Entries.prototype.closeMenu = function (focus) {
    var ref = this.ui.menu && this.ui.menu.ref;
    this.ui.menu = null;
    this.redrawRows();
    if (focus && ref) this.focusDots(ref);
  };
  Entries.prototype.openMenu = function (ref) {
    var f = this.cur(), data = this.rules[this.ck(f.key)];
    if (!data) return;
    if (this.ui.menu && this.ui.menu.ref === ref) { this.closeMenu(true); return; }
    this.ui.menu = { ref: ref, step: ref === 'rest' ? 'rest' : 'marks' };
    this.ui.strip = null;
    this.redrawRows();
    this.focusMenu();
  };
  Entries.prototype.focusMenu = function () {
    var cur = this.root.querySelector('.oe-menu [aria-checked="true"]') || this.root.querySelector('.oe-menu .oe-menu__item');
    if (cur) cur.focus({ preventScroll: true });
  };

  // A mark in a sentence: lower-cased, but never a player's name.
  function markIn(st, who) {
    if (st === 'required' && who) return who + ' takes it';
    return lower(stateWord(st)) + (who ? ' for ' + who : '');
  }
  // What the strip says it will do.
  Entries.prototype.phrase = function (ent, target, state, data) {
    var who = target ? this.memberLabel(data, target) : '';
    if (state === 'clear') return 'Put <b>' + esc(ent.name) + '</b> back to the same as everyone for <b>' + esc(who) + '</b>.';
    return 'Mark <b>' + esc(ent.name) + '</b> as <b>' + esc(markIn(state, who)) + '</b>' + (target ? '' : ' for everyone') + '.';
  };
  Entries.prototype.markSaid = function (ent, target, state, data) {
    var who = target ? this.memberLabel(data, target) : '';
    if (state === 'clear') return ent.name + ' is the same as everyone again for ' + who;
    return ent.name + (state === 'required' && who ? ': ' : ' is ') + markIn(state, who);
  };

  Entries.prototype.chooseMark = function (ref, target, state, reasonOnly) {
    var f = this.cur(), data = this.rules[this.ck(f.key)], ent = this.entOf(data, ref);
    if (!ent) return;
    var cur = target ? ent.players.filter(function (x) { return x.userId === target; })[0] : ent.everyone;
    var strip = { ref: ref, bulk: false, marks: [{ source: ent.source, key: ent.key, target: target || '', state: state }],
      what: reasonOnly ? 'Reason for <b>' + esc(ent.name) + '</b>, marked <b>' + esc(lower(stateWord(state))) + '</b>.' : this.phrase(ent, target, state, data),
      reason: (cur && cur.reason) || '', check: null, error: '', label: this.markSaid(ent, target, state, data) };
    this.ui.menu = null;
    if (reasonOnly) { strip.useReason = true; this.ui.strip = strip; this.redrawRows(); this.focusReason(); return; }
    this.review(strip, state === 'not_offered' || state === 'required');
  };
  Entries.prototype.focusReason = function () {
    var i = this.root.querySelector('[data-strip-reason]');
    if (i) i.focus({ preventScroll: true });
  };

  // A mark that could break a hero already made is checked first; a quiet
  // one is saved at once, since a single pick saves the moment it is made.
  Entries.prototype.review = function (strip, ask) {
    var self = this, f = this.cur();
    this.ui.working = strip.ref;
    this.ui.strip = null;
    this.redrawRows();
    if (!ask) { this.saveMarks(strip); return; }
    api(this.apiBase + '/choice-rules/check', { method: 'POST', body: { fieldKey: f.key, marks: strip.marks }, csrfToken: this.csrf }).then(json).then(function (res) {
      self.ui.working = null;
      if (!res.ok) { strip.error = res.body.message || res.body.error || 'That mark could not be checked. Try again.'; self.ui.strip = strip; self.redrawRows(); self.focusReason(); return; }
      strip.check = res.body;
      if (res.body.count > 0) { self.ui.strip = strip; self.redrawRows(); self.focusReason(); return; }
      self.saveMarks(strip);
    }).catch(function () {
      self.ui.working = null;
      strip.error = 'That mark could not be checked. Check your connection and try again.';
      self.ui.strip = strip; self.redrawRows();
    });
  };

  Entries.prototype.saveMarks = function (strip) {
    var self = this, f = this.cur();
    var marks = strip.marks.map(function (m) {
      var o = { source: m.source, key: m.key, target: m.target, state: m.state };
      if (strip.useReason || (strip.reason && strip.reason.trim())) o.reason = (strip.reason || '').trim();
      return o;
    });
    this.ui.working = strip.ref;
    this.redrawRows();
    // Over 200 at once go in several saves; each is whole on its own.
    var chunks = [];
    for (var i = 0; i < marks.length; i += 200) chunks.push(marks.slice(i, i + 200));
    var step = function (n) {
      if (n >= chunks.length) return Promise.resolve({ ok: true });
      return api(self.apiBase + '/choice-rules', { method: 'PUT', body: { fieldKey: f.key, marks: chunks[n] }, csrfToken: self.csrf }).then(json).then(function (res) {
        return res.ok ? step(n + 1) : res;
      });
    };
    step(0).then(function (res) {
      self.ui.working = null;
      if (!res.ok) {
        strip.error = res.body.message || res.body.error || 'That mark could not be saved. Try again.';
        self.ui.strip = strip; self.redrawRows(); self.focusReason();
        return;
      }
      self.ui.strip = null;
      self.reloadRules(function () {
        self.render();
        self.focusDots(strip.ref);
        notify(strip.label);
        self.announce(strip.label);
      });
    }).catch(function () {
      self.ui.working = null;
      strip.error = 'That mark could not be saved. Check your connection and try again.';
      self.ui.strip = strip; self.redrawRows();
    });
  };

  // Marks every entry with no mark of its own.
  Entries.prototype.markRest = function (state) {
    var f = this.cur(), data = this.rules[this.ck(f.key)], rest = this.restOf(data);
    if (!rest.length) return;
    var strip = { ref: 'rest', bulk: true, useReason: false, reason: '', check: null, error: '',
      marks: rest.map(function (e) { return { source: e.source, key: e.key, target: '', state: state }; }),
      what: 'Mark the other <b>' + rest.length + '</b> as <b>' + esc(lower(stateWord(state))) + '</b> for everyone.',
      label: rest.length + ' marked ' + lower(stateWord(state)) };
    this.ui.menu = null;
    this.review(strip, state === 'not_offered');
  };

  // A request a player made for an Ask-first entry: allow it, or send a note.
  // A draft or ask just answered changes Campaign settings' "needs you" count;
  // the sidebar keeps its last answer for a while, so ask it to read again.
  function refreshNeeds() {
    try { if (window.Chronicle && Chronicle.nav && Chronicle.nav.refreshCount) Chronicle.nav.refreshCount(true); } catch (e) { /* the badge catches up on the next load */ }
  }

  Entries.prototype.answer = function (id, verb, body) {
    var self = this, r = this.requests.filter(function (x) { return String(x.id) === String(id); })[0];
    if (!r || this.busy) return;
    this.busy = true;
    api(this.apiBase + '/choice-requests/' + encodeURIComponent(id) + '/' + verb, { method: 'POST', body: body || {}, csrfToken: this.csrf }).then(function (res) {
      return json(res).then(function (b) {
        self.busy = false;
        var gone = res.status === 409;
        if (!res.ok && !gone) { notify(b.body.message || b.body.error || 'That didn’t save. Try again.', 'error'); return; }
        self.ui.suggest = null;
        var said = gone ? 'That request was already answered.' : verb === 'allow'
          ? r.entryName + ' allowed for ' + (r.playerName || 'them') + (b.body && b.body.heroUpdated === false ? '' : ' and put on ' + (r.entityName || 'their hero'))
          : 'Your note went to ' + (r.playerName || 'them');
        refreshNeeds();
        self.reloadRequests(function () {
          self.reloadRules(function () { self.render(); notify(said, gone ? 'info' : 'success'); self.announce(said); });
        });
      });
    }).catch(function () { self.busy = false; notify('That didn’t save. Check your connection and try again.', 'error'); });
  };

  // Clears a mark whose entry has left the game system.
  Entries.prototype.clearOrphan = function (i) {
    var f = this.cur(), data = this.rules[this.ck(f.key)], o = data && data.orphans[i];
    if (!o) return;
    this.saveMarks({ ref: 'orphan', marks: [{ source: o.source, key: o.key, target: o.target, state: 'clear' }], reason: '', useReason: false, label: (o.name || o.key) + ' mark cleared' });
  };

  // A Director's list of the players who may write their own entry for this
  // list, with the quiet "let a player" that opens the small form.
  Entries.prototype.grantsHTML = function () {
    var self = this, f = this.cur(), l = lower(f.label);
    var gs = this.grants.filter(function (g) { return g.fieldKey === f.key; });
    var made = function (g) {
      var e = self.entries.filter(function (x) { return x.id === g.entryId; })[0];
      if (g.entryId && e) return 'made ' + esc(e.name);
      return g.entryId ? 'used it' : 'hasn’t made one yet';
    };
    var rows = gs.map(function (g) {
      return '<li><span><b>' + esc(self.memberName(g.userId)) + '</b> ' + made(g) +
        (f.budgetKey && g.budget ? ' · ' + g.budget + ' point' + (g.budget === 1 ? '' : 's') : '') + '</span>' +
        (g.entryId ? '<button type="button" class="oe-link" data-act="regrant" data-user="' + esc(g.userId) + '" data-budget="' + g.budget + '">Let them make another</button>' : '') +
        '<button type="button" class="oe-link" data-act="revoke" data-grant="' + esc(g.id) + '">Take back</button></li>';
    }).join('');
    var given = {};
    gs.forEach(function (g) { given[g.userId] = true; });
    var players = this.members.filter(function (m) { return m.role > 0 && m.role < 3 && !given[m.user_id]; });
    var form = '';
    if (this.giving) {
      form = players.length ? '<div class="oe-give" role="group" aria-label="Let a player make their own">' +
        '<label><span class="oe-k">Player</span><select data-give-user>' + players.map(function (m) {
          return '<option value="' + esc(m.user_id) + '">' + esc(m.display_name || 'A player') + (m.character_name ? ' (' + esc(m.character_name) + ')' : '') + '</option>';
        }).join('') + '</select></label>' +
        (f.budgetKey ? '<label><span class="oe-k">' + esc(words(f.budgetKey)) + '</span><input type="number" min="1" max="20" step="1" data-give-budget value="' + (f.budget || 3) + '"></label>' : '') +
        '<button type="button" class="oe-btn" data-act="give">Let them make one</button>' +
        '<button type="button" class="oe-link" data-act="givecancel">Cancel</button></div>'
        : '<p class="oe-hint">Everyone who plays here already may. <button type="button" class="oe-link" data-act="givecancel">Close</button></p>';
    }
    return '<div class="oe-grants"><p class="oe-k">Players who make their own</p>' +
      (rows ? '<ul>' + rows + '</ul>' : '<p class="oe-hint">Let a player write one ' + esc(l) + ' of their own, on this page. You read it and accept it before anyone else sees it.</p>') +
      (form || '<button type="button" class="oe-link" data-act="givestart">+ Let a player make their own ' + esc(l) + '</button>') + '</div>';
  };

  // A player's page: the lists a Director let them write in, their entries
  // and how far along each is.
  Entries.prototype.playerPageHTML = function () {
    var self = this, f = this.cur();
    var back = this.backUrl ? '<p><a class="oe-link" href="' + esc(this.backUrl) + '">← Back to ' + esc(this.forName || 'the hero creator') + '</a></p>' : '';
    var head = '<div class="hc-head"><p class="paper-kind">' + esc(this.system) + ' · yours</p><h1 tabindex="-1">Own entries</h1>' + back +
      '<p>Your ' + gm() + ' lets you write your own. Only you and the ' + gm() + 's see it until a ' + gm() + ' accepts it.</p></div>';
    if (!f) {
      return '<section class="hc-page paper" aria-label="Own entries">' + head +
        '<p class="hc-empty">Your ' + gm() + ' hasn’t let you make an entry of your own. Ask them, and it shows up here.</p></section>';
    }
    var tabs = this.fields.length > 1 ? '<div class="oe-fields" role="tablist" aria-label="Pick lists">' + this.fields.map(function (x, i) {
      return '<button type="button" role="tab" data-field="' + i + '" aria-selected="' + (i === self.field) + '">' + esc(x.label) + '</button>';
    }).join('') + '</div>' : '';
    var state = { draft: 'Draft', sent: 'Handed in', accepted: 'Accepted' };
    var mine = this.mine(f.key), g = this.grantFor(f.key);
    var rows = mine.length ? '<ul class="oe-list">' + mine.map(function (e) {
      var open = isDraft(e);
      return '<li><' + (open ? 'button type="button" data-open="' + esc(e.id) + '" aria-expanded="' + (self.open === String(e.id)) + '"' : 'div') + ' class="hc-row"><b>' + esc(e.name) +
        ' <span class="hc-seal' + (e.status === 'draft' && e.reviewNote ? ' oe-seal--wait' : ' oe-seal--dm') + '">' + (e.status === 'draft' && e.reviewNote ? 'Changes asked' : state[e.status] || 'Accepted') + '</span></b>' +
        (open ? '<span class="hc-row__go" aria-hidden="true">Edit ›</span>' : '') + '<small>' + esc(e.summary) + '</small></' + (open ? 'button' : 'div') + '></li>';
    }).join('') + '</ul>' : '';
    var can = g && !g.entryId;
    return '<section class="hc-page paper" aria-label="Own entries">' + head + tabs + rows +
      (can ? '' : (mine.length ? '' : '<p class="hc-empty">You’ve made your ' + esc(lower(f.label)) + '.</p>')) +
      '<div class="hc-foot"><span class="hc-foot__sp"></span>' +
      (can ? '<button type="button" class="hc-btn" data-act="add">+ Make your ' + esc(lower(f.label)) + '</button>' : '') + '</div></section>';
  };

  function sing(label) { return lower(label).replace(/ies$/, 'y').replace(/s$/, ''); }
  function costSeg(li, ii, c) {
    return '<span class="oe-cost" role="group" aria-label="Cost">' + [1, 2, 3].map(function (n) {
      return '<button type="button" data-cost="' + n + '" data-li="' + esc(li) + '" data-ii="' + ii + '" aria-pressed="' + (c === n) + '">' + n + ' pt' + (n > 1 ? 's' : '') + '</button>';
    }).join('') + '</span>';
  }

  Entries.prototype.writeHTML = function (d) {
    var f = this.cur();
    var facts = d.facts.map(function (p, i) {
      var val = p.long
        ? '<textarea class="oe-fact-long" rows="4" aria-label="' + esc(p.label || 'Fact') + '" data-fact="' + i + '" data-part="value" maxlength="2000">' + esc(p.value) + '</textarea>'
        : '<input aria-label="Fact value" data-fact="' + i + '" data-part="value" maxlength="200" value="' + esc(p.value) + '" placeholder="Value">';
      return '<input aria-label="Fact name" data-fact="' + i + '" data-part="label" maxlength="40" value="' + esc(p.label) + '" placeholder="Name, e.g. Size">' + val +
        '<button type="button" class="oe-rm" data-act="rmfact" data-i="' + i + '" aria-label="Remove ' + esc(p.label || 'this fact') + '">×</button>';
    }).join('');
    var hidden = /data-secret/.test(d.text);
    var lists = listsOf(d, f).map(function (l) {
      var items = d.lists[l.key] || [];
      return '<div class="oe-f"><span class="oe-k">' + esc(l.label) + '</span>' + items.map(function (it, i) {
        return '<div class="oe-item"><input class="oe-item__name" aria-label="' + esc(sing(l.label)) + ' name" data-li="' + esc(l.key) + '" data-ii="' + i + '" data-part="name" maxlength="100" value="' + esc(it.name) + '" placeholder="Name">' +
          (l.buy ? costSeg(l.key, i, it.cost) : '') +
          '<button type="button" class="oe-rm" data-act="rmitem" data-li="' + esc(l.key) + '" data-ii="' + i + '" aria-label="Remove ' + esc(it.name || sing(l.label)) + '">×</button>' +
          '<textarea rows="2" aria-label="What it does" data-li="' + esc(l.key) + '" data-ii="' + i + '" data-part="description" maxlength="2000" placeholder="What it does">' + esc(it.description) + '</textarea></div>';
      }).join('') + '<div><button type="button" class="oe-link" data-act="additem" data-li="' + esc(l.key) + '">+ Add ' + an(sing(l.label)) + ' ' + esc(sing(l.label)) + '</button></div></div>';
    }).join('');
    var g = this.director ? null : this.grantFor(f.key);
    var budget = f.budgetKey && g && g.budget ? '<div class="oe-bud"><span class="oe-k">' + esc(words(f.budgetKey)) + '</span><output>' + d.budget + '</output>' +
      '<span class="oe-hint">Set by your ' + gm() + '. You choose what the points buy.</span></div>'
      : f.budgetKey ? '<div class="oe-bud"><span class="oe-k" id="oe-bud-k">' + esc(words(f.budgetKey)) + '</span>' +
      '<span class="oe-step" role="group" aria-labelledby="oe-bud-k"><button type="button" data-act="minus" aria-label="One point fewer">−</button>' +
      '<output>' + d.budget + '</output><button type="button" data-act="plus" aria-label="One point more">+</button></span>' +
      '<span class="oe-hint">Points a hero spends on ' + esc(lower(f.lists.filter(function (l) { return l.buy; })[0].label)) + '.</span></div>' : '';
    return '<div class="oe-ed">' +
      '<label class="oe-f"><span class="oe-k">Name</span><input class="oe-big" data-k="name" maxlength="100" autocomplete="off" value="' + esc(d.name) + '" placeholder="' + esc(f.label) + ' name"></label>' +
      '<label class="oe-f"><span class="oe-k">One line for the list</span><input data-k="summary" maxlength="300" autocomplete="off" value="' + esc(d.summary) + '" placeholder="What a player reads before opening it"></label>' +
      '<div class="oe-f"><span class="oe-k" id="oe-text-k">The whole entry</span><div class="oe-tool">' +
        '<button type="button" data-act="bold" title="Bold"><b>B</b></button><button type="button" data-act="italic" title="Italic"><i>I</i></button>' +
        // A player's own entry has no hidden passages: players never read them, its writer included.
        (this.director ? '<button type="button" data-act="secret">Hide from players</button>' : '') + '</div>' +
        '<div class="oe-text" contenteditable="true" role="textbox" aria-multiline="true" aria-labelledby="oe-text-k" data-k="text" data-placeholder="Write it as you would a page.">' + d.text + '</div>' +
        (this.director ? '<span class="oe-hint" data-hint>' + (hidden ? '<b>Part of this entry is hidden from players.</b> ' + gm() + 's still read it.'
          : 'Select a passage and press “Hide from players” to keep it for ' + gm() + 's.') + '</span>' : '') + '</div>' +
      '<div class="oe-f"><span class="oe-k">Facts</span>' + (d.facts.length ? '<div class="oe-facts">' + facts + '</div>' : '') +
        '<div><button type="button" class="oe-link" data-act="addfact">+ Add a fact</button></div></div>' +
      (d.wordLists || []).map(function (w, i) {
        return '<label class="oe-f"><span class="oe-k">' + esc(w.label) + ' <small class="oe-hint">one per line</small></span>' +
          '<textarea class="oe-words" rows="' + Math.min(6, Math.max(2, w.text.split('\n').length)) + '" data-word="' + i + '">' + esc(w.text) + '</textarea></label>';
      }).join('') +
      budget + lists + (keptLine(d, f) ? '<p class="oe-hint oe-kept">' + esc(keptLine(d, f)) + '</p>' : '') + this.whoHTML(d) +
      '</div>';
  };

  // Who sees the entry. A player doesn't choose: it is theirs and the
  // Directors' until accepted. A Director reviewing a draft chooses who sees
  // it once they accept it, and can write what to change.
  Entries.prototype.whoHTML = function (d) {
    var f = this.cur();
    if (!this.director) {
      return '<div class="oe-f"><span class="oe-k">Who sees it</span><p class="oe-hint">Only you and the ' + gm() + 's, until a ' + gm() + ' accepts it' +
        (d.forName || this.forName ? '. It’s for ' + esc(d.forName || this.forName) : '') + '.</p></div>';
    }
    var review = isDraft(d) ? '<label class="oe-f"><span class="oe-k">What to change</span>' +
      '<textarea rows="2" maxlength="500" data-k="note" placeholder="Sent with “Ask for changes”">' + esc(d.note) + '</textarea></label>' : '';
    return review +
      '<fieldset class="oe-f oe-who"><legend class="oe-k">' + (isDraft(d) ? 'Who sees it once you accept it' : 'Who sees it') + '</legend>' +
        '<label><input type="radio" name="oe-who" value="everyone"' + (d.who === 'everyone' ? ' checked' : '') + '><span>Everyone</span>' +
          '<small>Players can pick it in the hero creator and on their sheet.</small></label>' +
        '<label><input type="radio" name="oe-who" value="directors"' + (d.who === 'directors' ? ' checked' : '') + '><span>' + gm() + 's only</span>' +
          '<small>For ' + an(f.label) + ' ' + esc(lower(f.label)) + ' the party hasn’t discovered yet. Switch it to Everyone when they do.</small></label>' +
        // Kept to one character (set from the AI page): offered only while it is, so a save never opens it by accident.
        (d.forName !== undefined ? '<label><input type="radio" name="oe-who" value="character"' + (d.who === 'character' ? ' checked' : '') + '><span>Only ' + esc(d.forName || 'one character') + '’s player</span>' +
          '<small>' + gm() + 's and whoever plays ' + esc(d.forName || 'that character') + ' see it.</small></label>' : '') +
        '</fieldset>';
  };

  // The system's rules for an entry, read back. Each is [message, blocks]:
  // what the server would refuse blocks Save; a budget a hero can't spend
  // is only a warning, since a Director may mean it.
  Entries.prototype.problems = function (d) { return OE.problems(d, this.cur()).map(function (m) { return [m[0], m[1]]; }); };
  Entries.prototype.checksHTML = function (d) {
    var f = this.cur(), p = this.problems(d);
    if (!p.length) {
      return '<p class="oe-ready">✓ Ready' + (f.budgetKey ? '. A hero can spend all ' + d.budget + ' points.' : '.') + '</p>';
    }
    return '<ul class="oe-problems" aria-label="Before you save">' + p.map(function (m) { return '<li class="' + (m[1] ? 'is-block' : '') + '">' + esc(m[0]) + '</li>'; }).join('') + '</ul>';
  };

  // What a player sees in the hero creator, drawn from the draft as typed.
  Entries.prototype.viewHTML = function (d) {
    var self = this, f = this.cur();
    var facts = d.facts.filter(function (p) { return p.label.trim() && p.value.trim(); });
    var box = tidy(d.text);
    box.querySelectorAll('[data-secret]').forEach(function (s) { s.remove(); });
    var text = box.textContent.trim() ? box.innerHTML : '';
    var spent = this.tryBuy.reduce(function (n, t) {
      var it = (d.lists[t.split(':')[0]] || [])[+t.split(':')[1]];
      return n + (it ? it.cost : 0);
    }, 0);
    var lists = listsOf(d, f).map(function (l) {
      var items = (d.lists[l.key] || []).filter(function (it) { return it.name.trim(); });
      if (!items.length) return '';
      if (!l.buy || !f.budgetKey) {
        return '<div class="hc-sec"><span class="hc-k">' + esc(l.label) + '</span>' + items.map(function (it) {
          return '<div class="hc-item"><b>' + esc(it.name) + '</b>' + (it.description.trim() ? '<p>' + esc(it.description) + '</p>' : '') + '</div>';
        }).join('') + '</div>';
      }
      var left = d.budget - spent, dots = '';
      for (var i = 0; i < d.budget; i++) dots += '<i class="' + (i < spent ? 'is-used' : '') + '"></i>';
      return '<div class="hc-sec"><span class="hc-k">' + esc(l.label) + ' <span class="hc-pts" aria-live="polite">' + dots + ' ' + Math.max(0, left) + ' of ' + d.budget + ' left</span></span>' +
        (d.lists[l.key] || []).map(function (it, i) {
          if (!it.name.trim()) return '';
          var tk = l.key + ':' + i, on = self.tryBuy.indexOf(tk) >= 0;
          return '<label class="hc-opt"><input type="checkbox" data-try="' + esc(tk) + '"' + (on ? ' checked' : '') + (!on && it.cost > left ? ' disabled' : '') + '>' +
            '<b>' + esc(it.name) + '</b><span class="hc-opt__c">' + it.cost + ' pt' + (it.cost === 1 ? '' : 's') + '</span>' +
            (it.description.trim() ? '<p>' + esc(it.description) + '</p>' : '') + '</label>';
        }).join('') + '</div>';
    }).join('');
    return '<p class="oe-note">What a player sees in the hero creator' + (d.who === 'directors' ? ', once you switch it to Everyone' : '') +
      (f.budgetKey ? '. Tick to try spending the points' : '') + '.</p>' +
      '<div class="hc-kind"><span class="paper-kind">' + esc(f.label) + ' · this campaign</span><span class="hc-seal">Campaign</span></div>' +
      '<h3 class="hc-name">' + esc(d.name.trim() || 'Unnamed') + '</h3>' + (d.summary.trim() ? '<p class="hc-lede">' + esc(d.summary) + '</p>' : '') +
      (facts.length ? '<div class="hc-facts">' + facts.map(function (p) {
        return '<div><b>' + esc(p.value) + '</b><span class="hc-k">' + esc(p.label) + '</span></div>';
      }).join('') + '</div>' : '') +
      (text ? '<div class="hc-text">' + text + '</div>' : '') + lists +
      '<div class="oe-checks" data-checks>' + this.checksHTML(d) + '</div>';
  };

  Entries.prototype.pasteHTML = function () {
    var f = this.cur();
    return '<div class="oe-paste" role="group" aria-label="Paste from an AI">' +
      '<p>Paste ' + an(lower(f.label)) + ' ' + esc(lower(f.label)) + ' an AI wrote in Chronicle’s format. It fills in the form so you can check it; nothing is saved until you press Save. “Copy the guide” copies the format with ' + esc(this.system) + '’s shape and an example, for any AI chat.</p>' +
      '<label class="sr-only" for="oe-paste-box">Text to read in</label><textarea id="oe-paste-box" spellcheck="false" placeholder="---&#10;name: …&#10;---"></textarea>' +
      '<div class="oe-paste__acts"><button type="button" class="oe-btn" data-act="readin">Read it in</button>' +
      '<button type="button" class="oe-btn oe-btn--ghost" data-act="guide">Copy the guide</button>' +
      '<button type="button" class="oe-link" data-act="pasteclose">Close</button><span class="oe-paste__msg" role="status" data-paste-msg></span></div></div>';
  };

  Entries.prototype.fillPop = function () {
    var pop = this.root.querySelector('.oe-pop'), d = this.draft, isNew = !d.id, f = this.cur(), body;
    if (this.confirm === 'discard' || this.confirm === 'delete') {
      var del = this.confirm === 'delete';
      body = '<div class="oe-confirm" role="alertdialog" aria-label="' + (del ? 'Delete ' + esc(d.name) : 'Unsaved changes') + '">' +
        (del ? 'Delete <b>' + esc(d.name) + '</b>? ' + (this.delRules > 0 ? '<b>' + esc(d.name) + '</b> is in ' + this.delRules + ' hero rule' + (this.delRules === 1 ? '' : 's') + '; they go with it. ' : '') +
          'It leaves the pick lists for everyone. Heroes who picked it keep the name on their sheet.'
          : 'Close ' + (d.name.trim() ? '<b>' + esc(d.name) + '</b>' : 'this entry') + ' without saving? What you changed is lost.') +
        '<div class="oe-confirm__row"><button type="button" class="oe-btn oe-btn--danger" data-act="' + (del ? 'dodel' : 'discard') + '">' + (del ? 'Delete' : 'Don’t save') + '</button>' +
        '<button type="button" class="oe-btn oe-btn--ghost" data-act="nodel">' + (del ? 'Keep it' : 'Keep writing') + '</button></div></div>';
    } else {
      body = (this.paste ? this.pasteHTML() : '') + '<div class="oe-cols">' + this.writeHTML(d) + '<aside class="oe-view" aria-label="What a player sees">' + this.viewHTML(d) + '</aside></div>';
    }
    var asked = d.reviewNote && d.status === 'draft' && !this.director ? '<p class="oe-asked" role="note"><b>Your ' + gm() + ' asked:</b> ' + esc(d.reviewNote) + '</p>' : '';
    var handed = isDraft(d) && this.director && d.status === 'sent' ? '<p class="oe-asked" role="note"><b>' + esc(this.memberName(d.by)) + ' handed this in.</b> Accept it, or say what to change.</p>' : '';
    // A rewrite starts as the Directors' draft; it replaces nothing until
    // they change who sees it.
    var draftOnly = this.director && d.standsFor && d.savedWho === 'directors' && !isDraft(d)
      ? '<p class="oe-asked oe-asked--dm" role="note" data-dm-only>Only ' + gm() + 's see this yet. Players still read ' + esc(this.system) + '’s ' + esc(d.standsForName || d.name) +
        '. Choose Everyone under “Who sees it” to put yours in play.</p>' : '';
    if (!this.confirm) body = asked + handed + draftOnly + body;
    pop.innerHTML = '<header class="oe-pop__head"><div><p class="oe-kind">' + esc(f.label) + ' · ' + (this.director ? 'this campaign · ' + gm() + 's only' : 'yours') + '</p>' +
      '<h2 data-pop-title tabindex="-1">' + esc(d.name.trim() || (isNew ? 'New ' + lower(f.label) : 'Unnamed')) + '</h2></div>' +
      '<div class="oe-pop__acts">' + (this.confirm ? '' : '<button type="button" class="oe-btn oe-btn--ghost" data-act="paste" aria-expanded="' + this.paste + '">Paste from an AI</button>') +
      '<button type="button" class="oe-x" data-act="close" aria-label="Close">×</button></div></header>' +
      '<div class="oe-pop__body">' + body + '</div>' +
      (this.tell ? this.tellHTML() : this.footHTML(d, isNew));
  };

  Entries.prototype.footHTML = function (d, isNew) {
    return '<footer class="oe-pop__foot">' + (isNew || !this.director ? '' : '<button type="button" class="oe-link oe-del" data-act="del">Delete</button>') +
      '<p class="oe-error" role="alert" hidden data-error></p><span class="hc-foot__sp"></span>' +
      '<button type="button" class="oe-btn oe-btn--ghost" data-act="close">Cancel</button>' + this.footActsHTML(d, isNew) + '</footer>';
  };

  // After a save that changed a trait a hero holds, the editor asks once, in
  // place of its commands: tell the players, with a line they will read, or
  // not now. The change is already saved and live either way.
  Entries.prototype.tellHTML = function () {
    var q = this.tell.q;
    return '<footer class="oe-pop__foot oe-tell" role="group" aria-labelledby="oe-tell-q">' +
      '<p class="oe-tell__q" id="oe-tell-q">Saved. Players read this now. <b>' + esc(q.holders) + '</b></p><span class="hc-foot__sp"></span>' +
      '<button type="button" class="oe-btn oe-btn--ghost" data-act="tellno">Not now</button>' +
      '<button type="button" class="oe-btn" data-act="tell" data-working-label="Telling…">Tell the players</button>' +
      '<label class="oe-tell__line"><span class="oe-k">What they will read</span>' +
      '<input data-tell-line maxlength="300" autocomplete="off" value="' + esc(this.tell.line) + '"></label>' +
      '<p class="oe-error" role="alert" hidden data-error></p></footer>';
  };

  // tellPlayers posts the line; it shows on the entry's page and peek.
  Entries.prototype.tellPlayers = function () {
    var self = this, input = this.root.querySelector('[data-tell-line]'), line = input ? input.value.trim() : '';
    var btn = this.root.querySelector('[data-act="tell"]'), err = this.root.querySelector('.oe-tell [data-error]');
    if (!line) { if (err) { err.textContent = 'Write the line the players will read, or choose Not now.'; err.hidden = false; } return; }
    if (this.busy) return;
    this.busy = true;
    if (window.Chronicle && Chronicle.working && btn) Chronicle.working(btn, true);
    api(this.entriesUrl + '/' + encodeURIComponent(this.tell.id) + '/notes', { method: 'POST', body: { line: line }, csrfToken: this.csrf })
      .then(json).then(function (res) {
        self.busy = false;
        if (window.Chronicle && Chronicle.working && btn) Chronicle.working(btn, false);
        if (!res.ok) { if (err) { err.textContent = res.body.message || res.body.error || 'The players were not told. Try again.'; err.hidden = false; } return; }
        self.tell = null;
        self.closePop(true);
        self.render();
        notify('Told the players. They read it on the entry’s page.');
      }).catch(function () {
        self.busy = false;
        if (window.Chronicle && Chronicle.working && btn) Chronicle.working(btn, false);
        if (err) { err.textContent = 'The players were not told. Check your connection and try again.'; err.hidden = false; }
      });
  };

  // The commands for the open entry. A Director saves, and on a player's
  // draft also asks for changes or accepts; the player saves their draft or
  // hands it in.
  Entries.prototype.footActsHTML = function (d, isNew) {
    var off = this.canSave() ? '' : ' disabled', f = this.cur();
    if (!this.director) {
      return '<button type="button" class="oe-btn oe-btn--ghost" data-act="save"' + off + '>' + (d.status === 'sent' ? 'Save' : 'Save draft') + '</button>' +
        (d.status === 'sent' ? '' : '<button type="button" class="oe-btn" data-act="send"' + off + '>Hand it in</button>');
    }
    if (isDraft(d)) {
      return '<button type="button" class="oe-btn oe-btn--ghost" data-act="save"' + off + '>Save</button>' +
        '<button type="button" class="oe-btn oe-btn--ghost" data-act="askchanges"' + off + '>Ask for changes</button>' +
        '<button type="button" class="oe-btn" data-act="accept"' + off + '>Accept</button>';
    }
    return '<button type="button" class="oe-btn" data-act="save"' + off + '>' + (isNew ? 'Add ' + esc(lower(f.label)) : 'Save') + '</button>';
  };

  Entries.prototype.canSave = function () {
    return !!this.draft && !this.busy && !this.problems(this.draft).some(function (p) { return p[1]; });
  };

  Entries.prototype.openPop = function (e, from) {
    var f = this.cur(), g = this.director ? null : this.grantFor(f.key);
    this.draft = draftOf(e, f, g && g.budget);
    this.saved = JSON.parse(JSON.stringify(this.draft));
    this.open = e ? String(e.id) : 'new';
    this.confirm = false;
    this.paste = false;
    this.tryBuy = [];
    var desk = this.root.querySelector('.oe-desk'), pop = this.root.querySelector('.oe-pop');
    this.root.querySelectorAll('.hc-row').forEach(function (r) { r.setAttribute('aria-expanded', String(r === from)); });
    this.fillPop();
    pop.hidden = false;
    // It grows out of the row (or button) that opened it.
    if (from) {
      var r = from.getBoundingClientRect(), d = desk.getBoundingClientRect();
      pop.style.transformOrigin = Math.round(r.left - d.left + r.width / 2) + 'px ' + Math.round(r.top - d.top + r.height / 2) + 'px';
    }
    this.fit();
    requestAnimationFrame(function () { desk.classList.add('is-open'); });
    if (pop.getBoundingClientRect().top < 0) pop.scrollIntoView({ block: 'start' });
    var n = pop.querySelector('[data-k="name"]');
    if (n) n.focus({ preventScroll: true });
    this.from = from;
  };

  // The page behind grows with the editor, so the editor never clips.
  Entries.prototype.fit = function () {
    var desk = this.root.querySelector('.oe-desk'), pop = this.root.querySelector('.oe-pop');
    if (desk && pop) desk.style.minHeight = this.open === null ? '' : (pop.offsetHeight + 8) + 'px';
  };

  Entries.prototype.closePop = function (force) {
    if (this.open === null) return;
    // Unsaved writing is never thrown away on a stray click: it asks first.
    if (!force && this.isDirty()) {
      this.syncText();
      this.confirm = 'discard';
      this.paste = false;
      this.fillPop();
      var keep = this.root.querySelector('[data-act="nodel"]');
      if (keep) keep.focus();
      return;
    }
    var desk = this.root.querySelector('.oe-desk'), pop = this.root.querySelector('.oe-pop'), self = this;
    this.open = null;
    this.draft = null;
    this.confirm = false;
    this.tell = null;
    // Opened from a page by address: closing goes back to that page.
    if (this.leaveTo) { var to = this.leaveTo; this.leaveTo = ''; dirty(false); window.location.assign(to); return; }
    dirty(false);
    desk.classList.remove('is-open');
    this.root.querySelectorAll('.hc-row').forEach(function (r) { r.setAttribute('aria-expanded', 'false'); });
    var done = function () { if (self.open === null) { pop.hidden = true; self.fit(); } };
    var ms = parseFloat(getComputedStyle(pop).transitionDuration) * 1000 || 0;
    setTimeout(done, ms);
    var back = this.from && document.body.contains(this.from) ? this.from : this.root.querySelector('[data-act="add"]');
    if (back) back.focus({ preventScroll: true });
  };

  Entries.prototype.isDirty = function () { return !!this.draft && !sameDraft(this.draft, this.saved); };

  // After a keystroke: the player's view, the title and Save follow the
  // draft; the form itself is left alone so focus and typing are undisturbed.
  Entries.prototype.touched = function () {
    if (!this.draft) return;
    dirty(this.isDirty());
    // Writing again after the question puts the commands back: the new
    // change has to be saved before anyone is told about it.
    if (this.tell && this.isDirty()) {
      this.tell = null;
      var asked = this.root.querySelector('.oe-tell');
      if (asked) asked.outerHTML = this.footHTML(this.draft, !this.draft.id);
    }
    var d = this.draft, view = this.root.querySelector('.oe-view');
    if (view) view.innerHTML = this.viewHTML(d);
    var t = this.root.querySelector('[data-pop-title]');
    if (t) t.textContent = d.name.trim() || (d.id ? 'Unnamed' : 'New ' + lower(this.cur().label));
    var can = this.canSave();
    this.root.querySelectorAll('.oe-pop__foot .oe-btn:not([data-act="close"])').forEach(function (b) { b.disabled = !can; });
  };

  // Redraws the form after a change that adds or removes fields, keeping the
  // scroll place.
  Entries.prototype.redraw = function () {
    var body = this.root.querySelector('.oe-pop__body'), top = body ? body.scrollTop : 0;
    this.fillPop();
    body = this.root.querySelector('.oe-pop__body');
    if (body) body.scrollTop = top;
    this.fit();
  };

  Entries.prototype.syncText = function () {
    var box = this.root.querySelector('.oe-text');
    if (!box || !this.draft) return;
    this.draft.text = (box.textContent.trim() || box.querySelector('li')) ? box.innerHTML : '';
    var hint = this.root.querySelector('[data-hint]');
    if (hint) {
      hint.innerHTML = box.querySelector('[data-secret]') ? '<b>Part of this entry is hidden from players.</b> ' + gm() + 's still read it.'
        : 'Select a passage and press “Hide from players” to keep it for ' + gm() + 's.';
    }
  };

  // Wraps the selection in a secret span, or unwraps it when the selection
  // already sits inside one, so the button works both ways.
  Entries.prototype.secret = function () {
    var box = this.root.querySelector('.oe-text'), sel = window.getSelection();
    if (!box || !sel.rangeCount) return;
    var range = sel.getRangeAt(0);
    if (!box.contains(range.commonAncestorContainer)) { notify('Select a passage in the entry first.', 'info'); return; }
    var n = range.commonAncestorContainer, inside = null;
    for (; n && n !== box; n = n.parentNode) if (n.nodeType === 1 && n.hasAttribute('data-secret')) { inside = n; break; }
    if (inside) {
      inside.replaceWith.apply(inside, Array.prototype.slice.call(inside.childNodes));
      this.syncText(); this.touched(); this.announce('Passage shown to players again');
      return;
    }
    if (range.collapsed) { notify('Select a passage in the entry first.', 'info'); return; }
    var span = document.createElement('span');
    span.setAttribute('data-secret', 'true');
    span.appendChild(range.extractContents());
    range.insertNode(span);
    sel.removeAllRanges();
    this.syncText(); this.touched(); this.announce('Passage hidden from players');
  };

  Entries.prototype.announce = function (msg) {
    var live = this.root.querySelector('.hc-live');
    if (!live) { live = document.createElement('div'); live.className = 'hc-live sr-only'; live.setAttribute('aria-live', 'polite'); this.root.appendChild(live); }
    live.textContent = msg;
  };

  Entries.prototype.fail = function (msg) {
    this.busy = false;
    var err = this.root.querySelector('.oe-pop [data-error]');
    if (err) { err.textContent = msg; err.hidden = false; }
    this.touched();
  };

  Entries.prototype.properties = function () { return propsOf(this.draft, this.cur()); };

  // mode is '' (save), 'send' (a player hands it in), 'accept' or 'ask' (a
  // Director's answer to a player's draft).
  Entries.prototype.save = function (mode) {
    var self = this, d = this.draft, f = this.cur();
    this.syncText();
    if (!this.canSave()) return;
    if (mode === 'ask' && !d.note.trim()) { this.fail('Write what to change first, in “What to change”.'); return; }
    var built = this.properties();
    if (built.error) { this.fail(built.error); return; }
    var text = tidy(d.text);
    var body = { name: d.name.trim(), summary: d.summary.trim(), description: text.textContent.trim() ? text.innerHTML : '',
      properties: built.props };
    var isNew = !d.id;
    if (this.director) {
      body.visibility = d.who === 'directors' || d.who === 'character' ? d.who : 'everyone';
      // A draft keeps who it is for until it is accepted.
      if (isDraft(d) && mode !== 'accept') delete body.visibility;
      if (mode === 'accept') body.status = 'accepted';
      if (mode === 'ask') { body.status = 'draft'; body.reviewNote = d.note.trim(); }
    } else {
      if (mode === 'send') body.status = 'sent';
      if (isNew && this.forId) body.forEntityId = this.forId;
    }
    if (isNew) body.fieldKey = f.key;
    this.busy = true;
    this.touched();
    var err = this.root.querySelector('.oe-pop [data-error]');
    if (err) err.hidden = true;
    api(isNew ? this.entriesUrl : this.entriesUrl + '/' + encodeURIComponent(d.id), { method: isNew ? 'POST' : 'PUT', body: body, csrfToken: this.csrf })
      .then(json).then(function (res) {
        if (!res.ok || !res.body.entry) { self.fail(res.body.message || res.body.error || 'The entry could not be saved. Try again.'); return; }
        var e = res.body.entry;
        var had = self.entries.filter(function (x) { return x.id === e.id; })[0];
        if (had && had.standsForName && !e.standsForName) e.standsForName = had.standsForName;
        self.entries = self.entries.filter(function (x) { return x.id !== e.id; }).concat([e]);
        if (res.body.tellPlayers && self.director && !isNew) {
          var q = res.body.tellPlayers;
          self.busy = false;
          self.tell = { q: q, id: e.id, line: q.line || '' };
          self.draft = draftOf(e, f);
          self.saved = JSON.parse(JSON.stringify(self.draft));
          dirty(false);
          self.redraw();
          var tb = self.root.querySelector('[data-act="tell"]');
          if (tb) tb.focus();
          return;
        }
        if (isNew && !self.director) {
          self.grants.forEach(function (g) { if (g.fieldKey === f.key && g.userId === self.user) g.entryId = e.id; });
          // Made for their hero: the hero's sheet takes the new name at once.
          if (self.forId) self.setHeroField(f.key, e.name);
        }
        self.busy = false;
        self.closePop(true);
        if (self.director) refreshNeeds();
        var finish = function () {
          self.render();
          var row = byAttr(self.root, '.hc-row[data-open]', 'data-open', String(e.id));
          if (row) row.focus({ preventScroll: true });
          var said = { send: ' handed in to your ' + gm(), accept: ' accepted', ask: ' handed back with your note' }[mode];
          notify(e.name + (said || (isNew ? (self.director ? ' added' : ' saved as a draft') : ' saved')));
        };
        // The marks list shows the new entry, or the changed name, at once.
        if (self.director) self.reloadRules(finish); else finish();
      }).catch(function () { self.fail('The entry could not be saved. Check your connection and try again.'); });
  };

  // Writes the entry's name into the player's hero, as picking it would.
  Entries.prototype.setHeroField = function (key, name) {
    var patch = {};
    patch[key] = name;
    var url = this.entriesUrl.replace(/\/system-entries$/, '') + '/entities/' + encodeURIComponent(this.forId) + '/fields';
    api(url, { method: 'PUT', body: { fields_patch: patch }, csrfToken: this.csrf }).then(function (r) {
      if (!r.ok) notify('Saved, but it isn’t on your sheet yet. Pick it there from the list.', 'info');
    }).catch(function () {});
  };

  // A Director's permissions: give one (or renew a used one), or take it back.
  Entries.prototype.give = function (userId, budget) {
    var self = this, f = this.cur();
    if (this.busy) return;
    this.busy = true;
    api(this.entriesUrl + '/grants', { method: 'POST', body: { fieldKey: f.key, userId: userId, budget: budget || 0 }, csrfToken: this.csrf })
      .then(json).then(function (res) {
        self.busy = false;
        if (!res.ok || !res.body.grant) { notify(res.body.message || res.body.error || 'That didn’t save. Try again.', 'error'); return; }
        var g = res.body.grant;
        self.grants = self.grants.filter(function (x) { return x.id !== g.id; }).concat([g]);
        self.giving = false;
        self.render();
        notify(self.memberName(userId) + ' can make their own ' + lower(f.label));
      }).catch(function () { self.busy = false; notify('That didn’t save. Check your connection and try again.', 'error'); });
  };
  Entries.prototype.revoke = function (id) {
    var self = this;
    if (this.busy) return;
    this.busy = true;
    api(this.entriesUrl + '/grants/' + encodeURIComponent(id), { method: 'DELETE', csrfToken: this.csrf }).then(function (r) {
      self.busy = false;
      if (!r.ok) { notify('That didn’t save. Try again.', 'error'); return; }
      self.grants = self.grants.filter(function (x) { return String(x.id) !== String(id); });
      self.render();
      notify('Permission taken back');
    }).catch(function () { self.busy = false; notify('That didn’t save. Check your connection and try again.', 'error'); });
  };

  Entries.prototype.remove = function () {
    var self = this, d = this.draft;
    if (this.busy || !d.id) return;
    this.busy = true;
    api(this.entriesUrl + '/' + encodeURIComponent(d.id), { method: 'DELETE', csrfToken: this.csrf }).then(function (r) {
      if (!r.ok) return json(r).then(function (res) { self.fail(res.body.message || res.body.error || 'The entry could not be deleted. Try again.'); });
      self.entries = self.entries.filter(function (x) { return x.id !== d.id; });
      self.busy = false;
      self.closePop(true);
      var finish = function () {
        self.render();
        var add = self.root.querySelector('[data-act="add"]');
        if (add) add.focus({ preventScroll: true });
        notify(d.name + ' deleted');
      };
      // Its rules went with it, and so did any request waiting on it.
      self.reloadRequests(function () { self.reloadRules(finish); });
    }).catch(function () { self.fail('The entry could not be deleted. Check your connection and try again.'); });
  };

  // Reads the homebrew format AI Import uses: YAML front matter between
  // "---" lines, then the entry's text. Only the small part of YAML an entry
  // needs is understood (key: value, lists of items, "|" and ">" blocks);
  // anything else is reported rather than guessed at.
  function unquote(v) {
    v = v.trim();
    if (/^".*"$/.test(v)) { try { return JSON.parse(v); } catch (e) { return v.slice(1, -1); } }
    if (/^'.*'$/.test(v)) return v.slice(1, -1).replace(/''/g, "'");
    return v;
  }
  function scalar(v) {
    var s = unquote(v);
    if (s !== v.trim()) return s;
    if (/^-?\d+(\.\d+)?$/.test(s)) return Number(s);
    if (/^(true|yes)$/i.test(s)) return true;
    if (/^(false|no)$/i.test(s)) return false;
    return s;
  }
  function readYAML(src) {
    var lines = src.split('\n'), out = {}, errs = [], i = 0;
    function block(style, ind) {
      var buf = [];
      while (i < lines.length && (!lines[i].trim() || lines[i].match(/^\s*/)[0].length > ind)) { buf.push(lines[i].trim()); i++; }
      while (buf.length && !buf[buf.length - 1]) buf.pop();
      return style === '|' ? buf.join('\n') : buf.join(' ').replace(/\s+/g, ' ').trim();
    }
    function value(raw, ind) { return /^[|>][+-]?$/.test(raw.trim()) ? block(raw.trim().charAt(0), ind) : scalar(raw); }
    while (i < lines.length) {
      var line = lines[i];
      if (!line.trim() || /^\s*#/.test(line)) { i++; continue; }
      var m = line.match(/^([A-Za-z_][\w-]*):\s*(.*)$/);
      if (!m) { errs.push('Line ' + (i + 1) + ' isn’t “name: value”.'); i++; continue; }
      i++;
      if (m[2].trim()) { out[m[1]] = value(m[2], 0); continue; }
      var list = [], cur = null;
      while (i < lines.length && (!lines[i].trim() || /^\s/.test(lines[i]))) {
        var l = lines[i], ind = l.match(/^\s*/)[0].length;
        if (!l.trim()) { i++; continue; }
        var item = l.trim().match(/^-\s+([A-Za-z_][\w-]*):\s*(.*)$/), kv = l.trim().match(/^([A-Za-z_][\w-]*):\s*(.*)$/);
        i++;
        if (item) { cur = {}; list.push(cur); cur[item[1]] = value(item[2], ind + 1); }
        else if (kv && cur) cur[kv[1]] = value(kv[2], ind);
        else errs.push('Line ' + i + ' under “' + m[1] + '” isn’t an item.');
      }
      out[m[1]] = list;
    }
    return { data: out, errs: errs };
  }

  Entries.prototype.readIn = function (text) {
    var f = this.cur(), src = text.replace(/\r/g, '').replace(/^\s*```[a-z]*\n?|\n?```\s*$/g, '').trim(), body = '';
    var fm = src.match(/^---\s*\n([\s\S]*?)\n---\s*(?:\n([\s\S]*))?$/);
    if (fm) { body = (fm[2] || '').trim(); src = fm[1]; }
    var y = readYAML(src), data = y.data;
    if (y.errs.length) return y.errs[0];
    if (data.kind && data.kind !== 'system-entry') return 'This is “' + data.kind + '”, not an entry.';
    if (data.entry && data.entry !== f.key) {
      var other = this.fields.filter(function (x) { return x.key === data.entry; })[0];
      return other ? 'This is ' + an(lower(other.label)) + ' ' + lower(other.label) + '. Open the ' + other.label + ' tab to read it in.' : 'This is for “' + data.entry + '”, which isn’t a pick list here.';
    }
    if (!data.name) return 'No name found. Check the text has a “name:” line.';
    var d = this.draft, lists = {};
    f.lists.forEach(function (l) { lists[l.key] = []; });
    d.name = String(data.name);
    d.summary = data.summary ? String(data.summary) : '';
    // A paste that doesn't say who sees it keeps the entry's current choice.
    if (data.director === true || data.visibility === 'directors') d.who = 'directors';
    else if (data.director === false || data.visibility === 'everyone') d.who = 'everyone';
    d.facts = [];
    var txt = body || (typeof data.description === 'string' ? data.description : '');
    d.text = txt ? txt.split(/\n\s*\n/).map(function (para) { return '<p>' + esc(para.trim()).replace(/\n/g, '<br>') + '</p>'; }).join('') : '';
    Object.keys(data).forEach(function (k) {
      var v = data[k];
      if (/^(kind|entry|name|summary|director|visibility|description|action|tags)$/.test(k)) return;
      if (Array.isArray(v)) {
        lists[k] = v.filter(function (it) { return it && it.name; }).map(function (it) {
          return { name: String(it.name), cost: typeof it.cost === 'number' ? it.cost : 1, description: String(it.description || it.text || '') };
        });
      } else if (k === f.budgetKey && +v > 0) d.budget = Math.min(20, Math.round(+v));
      else d.facts.push({ label: words(k), value: String(v), key: k });
    });
    d.lists = lists;
    this.tryBuy = [];
    return '';
  };

  // A guide any AI chat can follow: the format, filled with this system's
  // shape for the pick list, and one of the system's own entries as a model.
  function yamlStr(v) {
    if (typeof v === 'number' || typeof v === 'boolean') return String(v);
    var s = String(v);
    return /^[\w][\w .,'()/+-]*$/.test(s) && !/^(true|false|yes|no|null)$/i.test(s) && !/^\d/.test(s) ? s : JSON.stringify(s);
  }
  function plainRules(s) { return String(s || '').replace(/\{@\w+ ([^}|]*)(?:\|([^}]*))?\}/g, function (_, a, b) { return b || a; }); }
  Entries.prototype.guide = function () {
    var f = this.cur(), out = [], label = lower(f.label);
    out.push('Write ' + an(label) + ' ' + label + ' for ' + this.system + ' in Chronicle’s homebrew format. Reply with only one block like the template below, filled in.');
    if (f.budgetKey) {
      var buy = f.lists.filter(function (l) { return l.buy; }).map(function (l) { return l.key; });
      out.push('A hero spends ' + f.budgetKey + ' (usually ' + f.budget + ') on items from ' + buy.join(', ') + '; each item there has a whole-number cost, usually 1 or 2.');
    }
    out.push('', 'Template:', '---', 'kind: system-entry', 'entry: ' + f.key, 'name: …', 'summary: one line a player reads in the list');
    f.facts.forEach(function (k) { out.push(k + ': …'); });
    if (f.budgetKey) out.push(f.budgetKey + ': ' + f.budget);
    f.lists.forEach(function (l) { out.push(l.key + ':', '  - name: …'); if (l.buy) out.push('    cost: 1'); out.push('    description: what it does'); });
    out.push('---', 'The whole entry, as plain paragraphs.');
    var c = f.sample;
    if (c) {
      out.push('', 'One of ' + this.system + '’s own ' + plural(f.label) + ', in the same format, as a model:', '---', 'kind: system-entry', 'entry: ' + f.key, 'name: ' + yamlStr(c.name));
      if (c.summary) out.push('summary: ' + yamlStr(plainRules(c.summary)));
      var p = c.properties || {};
      f.facts.concat(f.budgetKey ? [f.budgetKey] : []).forEach(function (k) { if (p[k] != null && !Array.isArray(p[k])) out.push(k + ': ' + yamlStr(p[k])); });
      f.lists.forEach(function (l) {
        if (!namedList(p[l.key])) return;
        out.push(l.key + ':');
        p[l.key].slice(0, 2).forEach(function (it) {
          out.push('  - name: ' + yamlStr(it.name));
          if (typeof it.cost === 'number') out.push('    cost: ' + it.cost);
          if (it.description) out.push('    description: ' + yamlStr(plainRules(it.description)));
        });
      });
      out.push('---');
    }
    return out.join('\n');
  };

  // A pick in a row's menu: a mark, a step to the players, or the reason.
  Entries.prototype.menuAct = function (t) {
    var m = this.ui.menu, act = t.getAttribute('data-mact');
    if (!m) return;
    if (act === 'mark') { this.chooseMark(m.ref, t.getAttribute('data-target') || '', t.getAttribute('data-state')); return; }
    if (act === 'reason') { this.chooseMark(m.ref, t.getAttribute('data-target') || '', t.getAttribute('data-state'), true); return; }
    if (act === 'restmark') { this.markRest(t.getAttribute('data-state')); return; }
    if (act === 'players') m.step = 'players';
    else if (act === 'player') { m.step = 'player'; m.player = t.getAttribute('data-player'); }
    else if (act === 'back') m.step = m.step === 'player' ? 'players' : 'marks';
    this.redrawRows();
    this.focusMenu();
  };

  Entries.prototype.wire = function () {
    var self = this, root = this.root;
    // The tools act on the writing box's selection, so pressing them must not
    // take focus away from it.
    root.addEventListener('mousedown', function (e) { if (e.target.closest('.oe-tool button')) e.preventDefault(); });
    root.addEventListener('click', function (e) {
      var t = e.target.closest('button, .oe-scrim');
      if (!t || !root.contains(t)) return;
      if (t.hasAttribute('data-field')) {
        if (self.open !== null) return;
        self.field = +t.getAttribute('data-field');
        self.ui.menu = null; self.ui.strip = null; self.ui.suggest = null; self.ui.more = 40; self.ui.marked = false;
        self.render();
        var tab = root.querySelector('.oe-fields [data-field="' + self.field + '"]');
        if (tab) tab.focus();
        return;
      }
      if (t.hasAttribute('data-menu')) { self.openMenu(t.getAttribute('data-menu')); return; }
      if (t.hasAttribute('data-mact')) { self.menuAct(t); return; }
      if (t.hasAttribute('data-readdraft')) {
        if (self.open !== null) return;
        var did = t.getAttribute('data-readdraft');
        var dr = self.entries.filter(function (x) { return String(x.id) === did; })[0];
        if (!dr) return;
        for (var k = 0; k < self.fields.length; k++) if (self.fields[k].key === dr.fieldKey) self.field = k;
        self.ui.menu = null; self.ui.strip = null; self.ui.suggest = null; self.ui.more = 40; self.ui.marked = false;
        self.render();
        self.openPop(dr, null);
        return;
      }
      if (t.hasAttribute('data-open')) {
        var id = t.getAttribute('data-open');
        var hit = self.entries.filter(function (x) { return String(x.id) === id; })[0];
        if (hit && self.open === null) self.openPop(hit, t);
        return;
      }
      var d = self.draft, li = t.getAttribute('data-li'), ii = +t.getAttribute('data-ii');
      if (t.hasAttribute('data-cost')) {
        d.lists[li][ii].cost = +t.getAttribute('data-cost');
        t.parentNode.querySelectorAll('button').forEach(function (b) { b.setAttribute('aria-pressed', String(b === t)); });
        self.tryBuy = [];
        self.touched();
        return;
      }
      switch (t.getAttribute('data-act')) {
        case 'add': if (self.open === null) self.openPop(null, t); break;
        case 'retryrules': self.rulesErr = {}; self.render(); break;
        case 'more': self.ui.more += 60; self.redrawRows(); break;
        case 'onlymarked':
          self.ui.marked = !self.ui.marked; self.ui.more = 40;
          t.setAttribute('aria-pressed', String(self.ui.marked));
          self.redrawRows();
          break;
        case 'stripsave': if (self.ui.strip) self.saveMarks(self.ui.strip); break;
        case 'stripno':
          var from = self.ui.strip && self.ui.strip.ref;
          self.ui.strip = null; self.redrawRows(); if (from) self.focusDots(from);
          break;
        case 'clearorphan': self.clearOrphan(+t.getAttribute('data-orphan')); break;
        case 'allow': self.answer(t.getAttribute('data-req'), 'allow'); break;
        case 'suggest':
          self.ui.suggest = String(self.ui.suggest) === t.getAttribute('data-req') ? null : t.getAttribute('data-req');
          self.render();
          var msgBox = root.querySelector('[data-suggest-msg]');
          if (msgBox) msgBox.focus({ preventScroll: true });
          break;
        case 'nosuggest': self.ui.suggest = null; self.render(); break;
        case 'sendsuggest':
          var note = root.querySelector('[data-suggest-msg]');
          self.answer(t.getAttribute('data-req'), 'suggest', { message: note ? note.value.trim() : '' });
          break;
        case 'close': self.closePop(); break;
        case 'save': self.save(''); break;
        case 'send': self.save('send'); break;
        case 'accept': self.save('accept'); break;
        case 'askchanges': self.save('ask'); break;
        case 'givestart': self.giving = true; self.render(); var sel = root.querySelector('[data-give-user]'); if (sel) sel.focus(); break;
        case 'givecancel': self.giving = false; self.render(); break;
        case 'give':
          var u = root.querySelector('[data-give-user]'), bud = root.querySelector('[data-give-budget]');
          if (u && u.value) self.give(u.value, bud ? Math.max(1, Math.min(20, Math.round(+bud.value) || 1)) : 0);
          break;
        case 'regrant': self.give(t.getAttribute('data-user'), +t.getAttribute('data-budget') || 0); break;
        case 'revoke': self.revoke(t.getAttribute('data-grant')); break;
        case 'bold': case 'italic':
          document.execCommand(t.getAttribute('data-act'));
          self.syncText(); self.touched();
          break;
        case 'secret': self.secret(); break;
        case 'addfact':
          self.syncText();
          d.facts.push({ label: '', value: '', key: '' });
          self.redraw(); self.touched();
          var fs = root.querySelectorAll('.oe-facts input[data-part="label"]');
          if (fs.length) fs[fs.length - 1].focus();
          break;
        case 'rmfact':
          self.syncText();
          d.facts.splice(+t.getAttribute('data-i'), 1);
          self.redraw(); self.touched();
          break;
        case 'additem':
          self.syncText();
          (d.lists[li] = d.lists[li] || []).push({ name: '', cost: 1, description: '' });
          self.redraw(); self.touched();
          var ns = root.querySelectorAll('.oe-item__name[data-li="' + li + '"]');
          if (ns.length) ns[ns.length - 1].focus();
          break;
        case 'rmitem':
          self.syncText();
          d.lists[li].splice(ii, 1);
          self.tryBuy = [];
          self.redraw(); self.touched();
          break;
        case 'plus': case 'minus':
          d.budget = Math.max(1, Math.min(20, d.budget + (t.getAttribute('data-act') === 'plus' ? 1 : -1)));
          t.parentNode.querySelector('output').textContent = d.budget;
          self.tryBuy = [];
          self.touched();
          break;
        case 'paste':
          self.syncText();
          self.paste = !self.paste;
          self.redraw();
          if (self.paste) root.querySelector('#oe-paste-box').focus();
          break;
        case 'pasteclose': self.paste = false; self.redraw(); break;
        case 'readin':
          var msg = self.readIn(root.querySelector('#oe-paste-box').value);
          if (msg) { var m = root.querySelector('[data-paste-msg]'); m.textContent = msg; m.classList.add('is-bad'); break; }
          self.paste = false;
          self.redraw(); self.touched();
          self.announce(self.draft.name + ' read in. Check it, then save.');
          notify(self.draft.name + ' read in. Check it, then save.', 'info');
          break;
        case 'guide':
          var text = self.guide(), box = root.querySelector('#oe-paste-box'), out = root.querySelector('[data-paste-msg]');
          var ok = function () { out.textContent = 'Guide copied. Paste it into any AI chat with your idea.'; out.classList.remove('is-bad'); };
          var fallback = function () { box.value = text; box.select(); out.textContent = 'The guide is in the box; copy it from there.'; };
          if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(text).then(ok, fallback); else fallback();
          break;
        case 'del':
          self.syncText();
          self.confirm = 'delete'; self.paste = false; self.delRules = 0; self.redraw();
          var keep = root.querySelector('[data-act="nodel"]');
          if (keep) keep.focus();
          // The hero rules that name the entry go with it, so say so first.
          api(self.apiBase + '/choice-rules/count?entryId=' + encodeURIComponent(d.id)).then(json).then(function (res) {
            if (!res.ok || self.confirm !== 'delete' || !(res.body.count > 0)) return;
            self.delRules = res.body.count;
            self.redraw();
            var again = root.querySelector('[data-act="nodel"]');
            if (again) again.focus();
          }).catch(function () {});
          break;
        case 'nodel': self.confirm = false; self.redraw(); break;
        case 'dodel': self.remove(); break;
        case 'discard': self.closePop(true); break;
        case 'tell': self.tellPlayers(); break;
        case 'tellno': self.tell = null; self.closePop(true); self.render(); break;
      }
    });
    // A Director's marks: the search, the reason in a strip, who the list is
    // shown as, and a small menu closing on a click off it.
    root.addEventListener('input', function (e) {
      var el = e.target;
      if (!el.hasAttribute) return;
      if (el.hasAttribute('data-oe-q')) { self.ui.q = el.value; self.ui.more = 40; self.redrawRows(); }
      else if (el.hasAttribute('data-strip-reason') && self.ui.strip) self.ui.strip.reason = el.value;
    });
    root.addEventListener('change', function (e) {
      var el = e.target;
      if (!el.hasAttribute || !el.hasAttribute('data-show')) return;
      self.as = el.value;
      self.ui.menu = null; self.ui.strip = null; self.ui.more = 40;
      self.render();
      var sel = root.querySelector('[data-show]');
      if (sel) sel.focus({ preventScroll: true });
    });
    this.offMenu = function (e) {
      if (!document.body.contains(root)) { document.removeEventListener('click', self.offMenu); return; }
      if (!self.ui.menu) return;
      var path = e.composedPath ? e.composedPath() : [];
      var inside = path.some(function (n) { return n.classList && (n.classList.contains('oe-menu') || (n.hasAttribute && n.hasAttribute('data-menu'))); });
      if (!inside) self.closeMenu(false);
    };
    document.addEventListener('click', this.offMenu);
    root.addEventListener('input', function (e) {
      var el = e.target, d = self.draft;
      if (!d || !el.getAttribute) return;
      var k = el.getAttribute('data-k');
      if (k === 'text') self.syncText();
      else if (k) d[k] = el.value;
      if (el.hasAttribute('data-fact')) d.facts[+el.getAttribute('data-fact')][el.getAttribute('data-part')] = el.value;
      if (el.hasAttribute('data-li') && el.hasAttribute('data-part')) d.lists[el.getAttribute('data-li')][+el.getAttribute('data-ii')][el.getAttribute('data-part')] = el.value;
      if (el.hasAttribute('data-word')) d.wordLists[+el.getAttribute('data-word')].text = el.value;
      if (k || el.hasAttribute('data-fact') || el.hasAttribute('data-li') || el.hasAttribute('data-word')) self.touched();
    });
    root.addEventListener('change', function (e) {
      var el = e.target;
      if (el.name === 'oe-who' && self.draft) { self.draft.who = el.value; self.touched(); return; }
      if (el.hasAttribute && el.hasAttribute('data-try')) {
        var tk = el.getAttribute('data-try');
        if (el.checked) self.tryBuy.push(tk); else self.tryBuy = self.tryBuy.filter(function (x) { return x !== tk; });
        self.touched();
        var again = root.querySelector('[data-try="' + tk + '"]');
        if (again) again.focus();
      }
    });
    // Pasted or dropped text goes in plain, so formatting from elsewhere (and
    // anything hidden in it) never reaches the entry.
    root.addEventListener('paste', function (e) {
      if (!e.target.closest || !e.target.closest('.oe-text')) return;
      e.preventDefault();
      document.execCommand('insertText', false, (e.clipboardData || window.clipboardData).getData('text/plain'));
    });
    root.addEventListener('drop', function (e) {
      if (e.target.closest && e.target.closest('.oe-text')) e.preventDefault();
    });
    root.addEventListener('keydown', function (e) {
      if (self.tell && self.open !== null && (e.key === 'Escape' || (e.key === 'Enter' && e.target.hasAttribute && e.target.hasAttribute('data-tell-line')))) {
        e.preventDefault();
        if (e.key === 'Enter') self.tellPlayers(); else { self.tell = null; self.closePop(true); self.render(); }
        return;
      }
      if (self.open === null && self.director) {
        var inMenu = e.target.closest && e.target.closest('.oe-menu');
        if (e.key === 'Escape') {
          if (self.ui.menu) { e.preventDefault(); self.closeMenu(true); return; }
          if (self.ui.strip) { e.preventDefault(); var sf = self.ui.strip.ref; self.ui.strip = null; self.redrawRows(); self.focusDots(sf); return; }
          if (self.ui.suggest) { e.preventDefault(); self.ui.suggest = null; self.render(); return; }
        }
        if (inMenu && (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Home' || e.key === 'End')) {
          e.preventDefault();
          var its = Array.prototype.slice.call(inMenu.querySelectorAll('.oe-menu__item')), at = its.indexOf(document.activeElement);
          var to = e.key === 'Home' ? 0 : e.key === 'End' ? its.length - 1 : Math.max(0, Math.min(its.length - 1, at + (e.key === 'ArrowDown' ? 1 : -1)));
          if (its[to]) its[to].focus();
          return;
        }
        if (inMenu && e.key === 'Tab') { self.closeMenu(false); return; }
        if (e.key === 'Enter' && e.target.hasAttribute) {
          if (e.target.hasAttribute('data-strip-reason') && self.ui.strip) { e.preventDefault(); self.saveMarks(self.ui.strip); return; }
          if (e.target.hasAttribute('data-suggest-msg')) {
            e.preventDefault();
            self.answer(String(self.ui.suggest), 'suggest', { message: e.target.value.trim() });
            return;
          }
        }
      }
      if (e.key === 'Escape' && self.open !== null) {
        e.preventDefault();
        if (self.paste) { self.paste = false; self.redraw(); }
        else if (self.confirm) { self.confirm = false; self.redraw(); }
        else self.closePop();
        return;
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && self.open !== null) { e.preventDefault(); self.save(''); return; }
      if (e.key === 'Enter' && e.target.tagName === 'INPUT' && e.target.closest('.oe-pop') && e.target.type !== 'radio' && e.target.type !== 'checkbox') { e.preventDefault(); self.save(''); }
    });
    if (window.ResizeObserver) {
      new ResizeObserver(function () { if (self.open !== null) self.fit(); }).observe(root);
    }
  };

  window.ChronicleOwnEntries = { draftOf: draftOf, propsOf: propsOf, keptLine: keptLine, listsOf: listsOf, Entries: Entries };

  // Loaded on sight (ADR-063): boot.js fetches this file when it meets the
  // page's data-widget="own-entries" mount, and register mounts it.
  function register() {
    Chronicle.register('own-entries', {
      init: function (el) {
        if (el._ownEntries) return;
        var p = new Entries(el);
        el._ownEntries = p;
        p.wire();
        p.load();
      }
    });
  }
  if (window.Chronicle && Chronicle.register) register();
})();
