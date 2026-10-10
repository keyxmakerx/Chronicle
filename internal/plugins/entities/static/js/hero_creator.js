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
 * Served per-plugin at /static/plugins/entities/js/hero_creator.js from the
 * body-script registry, so it survives boosted navigation. Re-wires after
 * HTMX swaps.
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
      if (namedList(list) && list.every(function (it) { return typeof it.cost === 'number'; })) {
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
    this.draftKey = 'chronicle:hero-draft:' + this.campaignId;
    this.step = 0;
    this.open = null;
    this.tab = 'overview';
    this.picks = {};
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
      self.restore();
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
      if (!d) return;
      var self = this;
      Object.keys(d.picks || {}).forEach(function (k) {
        if (self.stepByKey(k) && self.choice(k, d.picks[k].name)) self.picks[k] = { name: d.picks[k].name, options: d.picks[k].options || [] };
      });
      this.name = typeof d.name === 'string' ? d.name : '';
    } catch (e) { /* no storage: start fresh */ }
  };
  Creator.prototype.save = function () {
    try { window.localStorage.setItem(this.draftKey, JSON.stringify({ picks: this.picks, name: this.name })); } catch (e) { /* ignore */ }
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
    var html = '<div class="hc-wrap"><section class="hc-page paper paper-stack" aria-label="Hero creator">' +
      this.trailHTML() + (this.isDetails() ? this.detailsHTML() : this.stepHTML()) + this.footHTML() +
      '</section>' + (this.isDetails() ? '' : '<aside class="hc-leaf paper" aria-hidden="true"></aside>') + '</div>';
    this.root.innerHTML = html;
    this.open = null;
  };

  Creator.prototype.trailHTML = function () {
    var self = this;
    var labels = this.plan.steps.map(function (s) { return s.label; }).concat(['Name']);
    return '<nav class="hc-trail" aria-label="Steps">' + labels.map(function (l, i) {
      var key = i < self.plan.steps.length ? self.plan.steps[i].fieldKey : null;
      var done = key ? !!(self.picks[key] && self.picks[key].name) : !!self.name.trim();
      return (i ? '<span class="hc-trail__ln" aria-hidden="true"></span>' : '') +
        '<button type="button" class="hc-stop' + (i === self.step ? ' is-on' : '') + (done ? ' is-done' : '') + '" data-step="' + i + '"' +
        (i === self.step ? ' aria-current="step"' : '') + '><i aria-hidden="true">' + (done && i !== self.step ? '✓' : (i + 1)) + '</i>' +
        '<span class="hc-stop__lbl">' + esc(l) + '</span></button>';
    }).join('') + '</nav>';
  };

  Creator.prototype.stepHTML = function () {
    var st = this.cur(), self = this;
    var head = '<div class="hc-head"><p class="paper-kind">Step ' + (this.step + 1) + ' of ' + this.stepCount() + '</p>' +
      '<h1 tabindex="-1">Choose your ' + esc(lower(st.label)) + '</h1><p>Tap one to pull out its leaf and read the whole entry.</p></div>';
    var rows = st.choices.map(function (c, i) {
      var picked = self.picks[st.fieldKey] && lower(self.picks[st.fieldKey].name) === lower(c.name);
      return '<li><button type="button" class="hc-row" data-open="' + i + '" aria-expanded="false"><b>' + esc(c.name) +
        (c.source === 'campaign' ? ' <span class="hc-seal">Campaign</span>' : '') +
        (picked ? ' <span class="hc-row__pick" aria-label="chosen">✓</span>' : '') +
        '</b><span class="hc-row__go" aria-hidden="true">Read ›</span><small>' + esc(c.summary) + '</small></button></li>';
    }).join('');
    return head + '<ul class="hc-list">' + rows + '</ul>';
  };

  Creator.prototype.footHTML = function () {
    var back = this.step > 0 ? '<button type="button" class="hc-btn hc-btn--ghost" data-act="back">← Back</button>' : '';
    if (this.isDetails()) {
      return '<div class="hc-foot"><span class="hc-foot__sp"></span>' + back +
        '<button type="button" class="hc-btn" data-act="create">Create hero</button></div>';
    }
    var st = this.cur(), p = this.picks[st.fieldKey];
    var chosen = p && p.name ? '<b>' + esc(p.name) + '</b>' + (p.options.length ? ' · ' + esc(p.options.join(', ')) : '')
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
        ? '<dd>' + esc(p.name) + (p.options.length ? '<small>' + esc(p.options.join(', ')) + '</small>' : '') + '</dd>'
        : '<dd class="is-none">Not chosen</dd>') +
        '<dd><button type="button" class="hc-link" data-step="' + i + '">Change</button></dd>';
    }).join('');
    var whose = this.forSelf ? 'The hero will be yours, on a new ' + esc(this.typeName) + ' page.'
      : 'The hero goes on a new ' + esc(this.typeName) + ' page, not yet claimed by a player.';
    var none = this.plan.steps.length ? '' : '<p class="hc-empty">' + (this.plan.systemName ? esc(this.plan.systemName) + ' doesn’t list' : 'This campaign’s game system doesn’t list') +
      ' choices for heroes yet, so the creator only asks for a name. Directors can add their own entries.</p>';
    return '<div class="hc-head"><p class="paper-kind">Step ' + this.stepCount() + ' of ' + this.stepCount() + '</p>' +
      '<h1 tabindex="-1">Name your hero</h1></div>' + none +
      '<label class="hc-field"><span class="hc-k">Name</span><input type="text" maxlength="200" autocomplete="off" data-name value="' + esc(this.name) + '" required></label>' +
      (rows ? '<dl class="hc-sum">' + rows + '</dl>' : '') + '<p class="hc-note">' + whose + ' You can change any of it on the sheet later.</p>' +
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
      '<button type="button" class="hc-btn" data-act="choose"' + (isPicked ? ' aria-pressed="true"' : '') + '>' +
      (isPicked ? '✓ ' + esc(c.name) + ' chosen' : 'Choose ' + esc(c.name)) + '</button></div>';
  };

  // The visible part of whatever scrolls the page: the app's main column, or
  // the window when nothing inside it scrolls.
  function scrollerRect(el) {
    for (var e = el.parentElement; e && e !== document.body; e = e.parentElement) {
      if (/(auto|scroll)/.test(getComputedStyle(e).overflowY) && e.scrollHeight > e.clientHeight) {
        var r = e.getBoundingClientRect();
        return { top: r.top, height: e.clientHeight };
      }
    }
    return { top: 0, height: window.innerHeight };
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
      var view = scrollerRect(wrap);
      leaf.style.setProperty('--hc-sheet-h', Math.max(320, view.height - 24) + 'px');
      var top = Math.min(wrap.offsetHeight - leaf.offsetHeight, view.top + 12 - wrap.getBoundingClientRect().top);
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
    if (trail) trail.outerHTML = this.trailHTML();
    var foot = this.root.querySelector('.hc-foot');
    if (foot) foot.outerHTML = this.footHTML();
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
    var send = window.Chronicle && Chronicle.apiFetch
      ? Chronicle.apiFetch(this.createUrl, { method: 'POST', body: { name: name, picks: picks }, csrfToken: this.csrf })
      : fetch(this.createUrl, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrf }, body: JSON.stringify({ name: name, picks: picks }) });
    send.then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (body) { return { ok: r.ok, body: body }; });
    }).then(function (res) {
      if (!res.ok || !res.body.url) { fail(res.body.message || res.body.error || 'The hero could not be made. Try again.'); return; }
      self.clearDraft();
      if (window.Chronicle && Chronicle.go) Chronicle.go(res.body.url); else window.location.href = res.body.url;
    }).catch(function () { fail('The hero could not be made. Check your connection and try again.'); });
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
        if (trail) trail.outerHTML = self.trailHTML();
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

  function boot(scope) {
    var nodes = (scope || document).querySelectorAll('[data-hero-creator]:not([data-hc-ready])');
    for (var i = 0; i < nodes.length; i++) {
      nodes[i].setAttribute('data-hc-ready', 'true');
      var c = new Creator(nodes[i]);
      c.wire();
      c.load();
    }
  }

  function start() {
    boot(document);
    document.addEventListener('htmx:afterSettle', function (e) { boot(e.target); });
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
