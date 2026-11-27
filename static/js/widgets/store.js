/**
 * store.js — the Store: a storefront sheet over any campaign page where the
 * owner browses what other campaigns on this server shared and adds it here.
 *
 * Mount: <div data-widget="store"> in the top bar (layouts/app.templ), with
 * data-campaign, data-csrf and data-css. The bar's button opens the sheet; so
 * does Chronicle.Store.open({ kind }) from a page that can use a kind.
 *
 * The sheet is a native modal <dialog>: it drapes down from the top bar over
 * the page with its shadow, nothing behind it moves, and it rolls back up on
 * its close button or Escape. A stray click never closes it. Kinds and the
 * game system are down the side; "For you" leads with what is new for the
 * campaign's game system; a thing opens in place with Add to this campaign.
 *
 * Everything shown comes from the design gallery's routes, so the server
 * decides what is visible, re-checks every add and keeps players out.
 * Typed text is never put through markup: it is escaped, or set with .value.
 *
 * The pure helpers are on Chronicle.Store.util so test/js can run them
 * without a page.
 */
(function () {
  'use strict';
  window.Chronicle = window.Chronicle || {};

  // The Store's kinds and the gallery filter each one reads.
  // TODO(#2910): read the kinds from the server once kinds register with the Store.
  var KINDS = [
    { id: 'rooms', label: 'Shop rooms', one: 'Shop room', where: 'Find it in your designs when you arrange a shop room.' },
    { id: 'crafting', label: 'Crafting looks', one: 'Crafting look', where: 'Find it in your designs when you dress a crafting bench.' }
  ];
  var KIND_OF = { 'room-theme': 'rooms', 'crafting-circle': 'crafting', 'crafting-item-look': 'crafting', 'crafting-bench': 'crafting' };
  var KIND_WORD = { 'room-theme': 'Shop room', 'crafting-circle': 'Magic circle', 'crafting-item-look': 'Item look', 'crafting-bench': 'Crafting bench' };
  var SYSTEMS = { dnd5e: 'D&D 5e', drawsteel: 'Draw Steel', pathfinder2e: 'Pathfinder 2e', daggerheart: 'Daggerheart' };
  var REASONS = [['adult', 'Adult or vulgar'], ['spam', 'Spam or advertising'], ['other', 'Something else']];
  var HOME_COUNT = 6, CLOSE_MS = 150;

  /* ---- Pure helpers ---- */
  function nice(v) { v = String(v || '').replace(/[-_]+/g, ' ').trim(); return v.charAt(0).toUpperCase() + v.slice(1); }
  function systemName(id) {
    if (!id) return 'no game system';
    if (SYSTEMS[id]) return SYSTEMS[id];
    return String(id).indexOf('custom:') === 0 ? 'a custom game system' : nice(id);
  }
  function kindById(id) { for (var i = 0; i < KINDS.length; i++) if (KINDS[i].id === id) return KINDS[i]; return null; }
  function kindWord(k) { return KIND_WORD[k] || nice(k) || 'Design'; }
  function kindOf(k) { return KIND_OF[k] || ''; }
  // The gallery query for one view: a kind (or every kind), a search, a page,
  // and the campaign's game system unless "any game system" is chosen.
  function query(f) {
    var q = ['page=' + Math.max(1, (f.page | 0) || 1)];
    if (f.all) q.push('system=all');
    if (f.kind && kindById(f.kind)) q.push('kind=' + encodeURIComponent(f.kind));
    if (f.q) q.push('q=' + encodeURIComponent(f.q));
    return '?' + q.join('&');
  }
  function pages(total, page, per) {
    per = per || 12;
    var n = Math.max(1, Math.ceil((total | 0) / per));
    page = Math.min(Math.max(1, (page | 0) || 1), n);
    return { pages: n, page: page, from: total ? (page - 1) * per + 1 : 0, to: Math.min(total | 0, page * per), prev: page > 1, next: page < n };
  }
  // The gentle notes on a thing: another game system first, then what this
  // campaign lacks, as the server worded it.
  function notes(card, campaignSystem) {
    var out = [];
    if (card && card.sameSystem === false) {
      out.push('Made for ' + systemName(card.systemId) + '. This campaign uses ' + systemName(campaignSystem) + '. Parts it uses may be named differently here.');
    }
    ((card && card.lacks) || []).forEach(function (l) { if (l) out.push(String(l)); });
    return out;
  }
  function reason(v) { for (var i = 0; i < REASONS.length; i++) if (REASONS[i][0] === v) return v; return 'other'; }
  function emptyLine(view, all, sys, q) {
    if (q) return 'Nothing matches “' + q + '”.';
    var what = view === 'home' ? 'Nothing has been shared' : 'No ' + (kindById(view) || { label: 'things' }).label.toLowerCase() + ' have been shared';
    return what + (all ? ' on this server yet.' : ' for ' + sys + ' yet.') + ' When owners on this server share something, it shows here.';
  }
  function motion() {
    var html = document.documentElement, m = html.getAttribute('data-motion') || '';
    if (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return 'off';
    if (m === 'calm' || m === 'off') return m;
    // A person's own Calm or Off in My view also sets these older flags
    // (theme.js), and the sheet must not travel for them either.
    if (html.getAttribute('data-view-motion') === 'calm' || html.classList.contains('nav-rm')) return 'calm';
    return 'full';
  }

  /* ---- State ---- */
  var host = null, toggle = null, dlg = null, campaign = '', csrf = '', searchT = 0, seq = 0;
  var S = freshState();
  function freshState() {
    return { view: 'home', all: false, q: '', page: 1, system: '', home: {}, list: null, item: null, from: 'home',
      err: '', added: {}, report: null, reportErr: '' };
  }

  function esc(t) { return window.Chronicle.escapeHtml(t); }
  function call(method, path, body) {
    return window.Chronicle.apiAction('/campaigns/' + encodeURIComponent(campaign) + path, { method: method, body: body, csrfToken: csrf });
  }
  function $(sel) { return dlg ? dlg.querySelector(sel) : null; }
  function sysLabel() { return systemName(S.system); }

  /* ---- Drawing ---- */
  function shell() {
    return '<div class="st-scrim" aria-hidden="true"></div>' +
      '<section class="st-sheet" aria-labelledby="st-title">' +
        '<header class="st-head"><h2 id="st-title" class="st-title">Store</h2>' +
          '<label class="sr-only" for="st-q">Search the Store</label>' +
          '<input id="st-q" class="st-search" type="search" autocomplete="off" placeholder="Search everything">' +
          '<button type="button" class="btn-ghost btn-icon st-close" data-st="close" aria-label="Close the Store" title="Close">' +
            '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"></path></svg>' +
          '</button></header>' +
        '<div class="st-body"><nav class="st-rail" aria-label="What to browse"></nav>' +
          '<div class="st-main" tabindex="-1"></div></div>' +
      '</section>';
  }
  function railHtml() {
    var h = '<button type="button" class="st-k" data-st="view" data-v="home" aria-current="' + (S.view === 'home' && !S.q) + '">For you</button>';
    KINDS.forEach(function (k) {
      var d = S.home[k.id], n = d && typeof d.total === 'number' ? d.total : '';
      h += '<button type="button" class="st-k" data-st="view" data-v="' + k.id + '" aria-current="' + (S.view === k.id && !S.q) + '">' +
        '<span>' + esc(k.label) + '</span>' + (n === '' ? '' : '<span class="st-n"><span class="sr-only">, </span>' + esc(n) + '</span>') + '</button>';
    });
    h += '<p class="st-lb" id="st-sys-lb">Game system</p><div class="st-sys" role="radiogroup" aria-labelledby="st-sys-lb">' +
      '<button type="button" class="st-k" role="radio" data-st="sys" data-v="cur" aria-checked="' + !S.all + '">' + esc(sysLabel()) + '</button>' +
      '<button type="button" class="st-k" role="radio" data-st="sys" data-v="all" aria-checked="' + S.all + '">Any game system</button></div>';
    return h;
  }
  function cardHtml(c) {
    var n = notes(c, S.system), sub = kindWord(c.kind) + ' · by ' + (c.makerName || 'someone') + (c.mine ? ' (you)' : '');
    return '<li><button type="button" class="st-card" data-st="item" data-id="' + esc(c.id) + '">' +
      '<span class="st-cn">' + esc(c.name) + '</span><span class="st-cs">' + esc(sub) + '</span>' +
      (c.note ? '<span class="st-cnote">' + esc(c.note) + '</span>' : '') +
      '<span class="st-cf">' + (S.added[c.id] ? '<span class="st-ok">In your designs</span>' : c.mine ? '<span class="st-dim">Yours</span>' : '') +
        (n.length ? '<span class="st-warn">' + esc(n.length === 1 ? 'One thing to know' : n.length + ' things to know') + '</span>' : '') + '</span>' +
      '</button></li>';
  }
  function gridHtml(list) { return '<ul class="st-grid" role="list">' + list.map(cardHtml).join('') + '</ul>'; }
  function placeholders(n) {
    var h = '<ul class="st-grid st-wait" aria-hidden="true">';
    for (var i = 0; i < n; i++) h += '<li><span class="st-card st-ph"><span></span><span></span><span></span></span></li>';
    return h + '</ul>';
  }
  function homeHtml() {
    var h = '', any = false, waiting = false;
    KINDS.forEach(function (k) {
      var d = S.home[k.id];
      if (!d) { waiting = true; return; }
      if (!d.entries || !d.entries.length) return;
      any = true;
      h += '<section class="st-sec" aria-labelledby="st-h-' + k.id + '"><div class="st-sh"><h3 class="st-h" id="st-h-' + k.id + '">New ' + esc(k.label.toLowerCase()) +
        (S.all ? '' : ' for ' + esc(sysLabel())) + '</h3>' +
        (d.total > HOME_COUNT ? '<button type="button" class="st-word" data-st="view" data-v="' + k.id + '">See all ' + esc(d.total) + '</button>' : '') + '</div>' +
        gridHtml(d.entries.slice(0, HOME_COUNT)) + '</section>';
    });
    if (waiting && !any) return '<p class="sr-only" role="status">Loading the Store</p>' + placeholders(HOME_COUNT);
    if (!any) return emptyHtml('home');
    return h;
  }
  function emptyHtml(view) {
    return '<div class="st-empty"><p>' + esc(emptyLine(view, S.all, sysLabel(), S.q)) + '</p>' +
      (S.all ? '' : '<button type="button" class="st-word" data-st="sys" data-v="all">Show every game system</button>') + '</div>';
  }
  function listHtml() {
    var d = S.list, k = kindById(S.view);
    var title = S.q ? 'Results for “' + S.q + '”' : (k ? k.label : 'Everything');
    var h = '<div class="st-sh"><h3 class="st-h">' + esc(title) + '</h3>' + (d ? '<span class="st-count" role="status">' + esc(d.total) + (d.total === 1 ? ' thing' : ' things') + (S.all ? '' : ' for ' + esc(sysLabel())) + '</span>' : '') + '</div>';
    if (!d) return h + '<p class="sr-only" role="status">Loading</p>' + placeholders(6);
    if (!d.entries.length) return h + emptyHtml(S.view);
    h += gridHtml(d.entries);
    var p = pages(d.total, d.page, d.perPage);
    if (p.pages > 1) {
      h += '<div class="st-pager"><button type="button" class="st-word" data-st="page" data-v="-1"' + (p.prev ? '' : ' disabled') + '>Newer</button>' +
        '<span class="st-dim">' + esc(String(p.from)) + '–' + esc(String(p.to)) + ' of ' + esc(String(d.total)) + '</span>' +
        '<button type="button" class="st-word" data-st="page" data-v="1"' + (p.next ? '' : ' disabled') + '>Older</button></div>';
    }
    return h;
  }
  function itemHtml() {
    var c = S.item, k = kindById(kindOf(c.kind)), back = S.from === 'home' ? 'For you' : S.q ? 'Results' : (kindById(S.from) || { label: 'the list' }).label;
    var h = '<button type="button" class="st-word st-back" data-st="back">← Back to ' + esc(back) + '</button>' +
      '<article class="st-item" aria-labelledby="st-item-h"><h3 class="st-ih" id="st-item-h" tabindex="-1">' + esc(c.name) + '</h3>' +
      '<p class="st-cs">' + esc(kindWord(c.kind)) + ' · made for ' + esc(systemName(c.systemId)) + ' · by ' + esc(c.makerName || 'someone') + (c.mine ? ' (you)' : '') + '</p>';
    if (c.note) h += '<p class="st-inote">' + esc(c.note) + '</p>';
    if (c.summary) h += '<p class="st-dim">' + esc(c.summary) + '</p>';
    notes(c, S.system).forEach(function (n) { h += '<p class="st-know" role="note">' + esc(n) + '</p>'; });
    if (c.mine) {
      h += '<p class="st-dim">You shared this. Change or unpublish it from your designs.</p>';
    } else if (S.added[c.id]) {
      h += '<p class="st-ok" role="status">Added to your designs. ' + esc(k ? k.where : '') + '</p>';
    } else {
      h += '<div class="st-acts"><button type="button" class="btn btn-primary st-add" data-st="add" data-working-label="Adding…">Add to this campaign</button>' +
        '<span class="st-dim">Your copy is your own. If the maker changes theirs later, yours stays as it is.</span></div>';
    }
    if (!c.mine) {
      if (c.reported) h += '<p class="st-dim">You reported this. An admin will look at it.</p>';
      else if (S.report) {
        h += '<fieldset class="st-rep"><legend>What is wrong with it</legend>' + REASONS.map(function (r) {
          return '<label class="st-rad"><input type="radio" name="st-reason" value="' + r[0] + '"' + (S.report.reason === r[0] ? ' checked' : '') + '> ' + esc(r[1]) + '</label>';
        }).join('') + '<label class="st-lab" for="st-rnote">A note, if you like</label><textarea id="st-rnote" rows="2" maxlength="140"></textarea>' +
          '<p class="st-err" role="alert" id="st-rerr">' + esc(S.reportErr) + '</p>' +
          '<div class="st-acts"><button type="button" class="btn btn-secondary" data-st="report-send" data-working-label="Sending…">Send report</button>' +
          '<button type="button" class="st-word" data-st="report-cancel">Cancel</button></div></fieldset>';
      } else h += '<button type="button" class="st-word st-quiet" data-st="report-open">Report this</button>';
    }
    return h + '</article>';
  }
  function paint(focusSel) {
    if (!dlg) return;
    $('.st-rail').innerHTML = railHtml();
    var main = $('.st-main');
    var body = S.view === 'item' && S.item ? itemHtml() : (S.view === 'home' && !S.q ? homeHtml() : listHtml());
    main.innerHTML = (S.err ? '<div class="st-errbox" role="alert"><p>' + esc(S.err) + '</p><button type="button" class="st-word" data-st="retry">Try again</button></div>' : '') + body;
    var rn = $('#st-rnote'); if (rn && S.report) rn.value = S.report.note || '';
    if (focusSel) { var f = $(focusSel); if (f) f.focus({ preventScroll: false }); }
  }

  /* ---- Loading ---- */
  function loadHome() {
    var mine = ++seq;
    S.home = {}; S.err = '';
    paint();
    KINDS.forEach(function (k) {
      call('GET', '/designs/gallery' + query({ kind: k.id, all: S.all, page: 1 })).then(function (d) {
        if (mine !== seq) return;
        S.home[k.id] = d; if (d && d.systemId && !S.all) S.system = d.systemId;
        paint();
      }).catch(function (e) { if (mine !== seq) return; S.home[k.id] = { entries: [], total: 0 }; S.err = (e && e.message) || 'Could not load the Store. Try again.'; paint(); });
    });
  }
  function loadList() {
    var mine = ++seq;
    S.list = null; S.err = '';
    paint();
    call('GET', '/designs/gallery' + query({ kind: S.q ? '' : S.view, all: S.all, q: S.q, page: S.page })).then(function (d) {
      if (mine !== seq) return;
      S.list = d; S.page = d.page || 1; if (d && d.systemId && !S.all) S.system = d.systemId;
      paint();
    }).catch(function (e) { if (mine !== seq) return; S.list = { entries: [], total: 0, page: 1, perPage: 12 }; S.err = (e && e.message) || 'Could not load the Store. Try again.'; paint(); });
  }
  function reload() { if (S.view === 'home' && !S.q) loadHome(); else if (S.view !== 'item') loadList(); }
  function findCard(id) {
    var lists = [S.list && S.list.entries].concat(KINDS.map(function (k) { return S.home[k.id] && S.home[k.id].entries; }));
    for (var i = 0; i < lists.length; i++) for (var j = 0; lists[i] && j < lists[i].length; j++) if (lists[i][j].id === id) return lists[i][j];
    return null;
  }

  /* ---- Actions ---- */
  function addItem(btn) {
    var c = S.item; if (!c || S.added[c.id]) return;
    if (window.Chronicle.working) window.Chronicle.working(btn, true);
    call('POST', '/designs/gallery/' + encodeURIComponent(c.id) + '/copy').then(function () {
      S.added[c.id] = true; S.err = ''; paint('.st-ok');
      if (window.Chronicle.notify) window.Chronicle.notify('“' + c.name + '” was added to your designs.', 'success');
    }).catch(function (e) {
      if (window.Chronicle.working) window.Chronicle.working(btn, false);
      S.err = (e && e.message) || 'Could not add it. Try again.'; paint();
    });
  }
  function sendReport(btn) {
    var c = S.item, r = S.report; if (!c || !r) return;
    var n = $('#st-rnote'); r.note = n ? n.value : '';
    if (window.Chronicle.working) window.Chronicle.working(btn, true);
    call('POST', '/designs/gallery/' + encodeURIComponent(c.id) + '/report', { reason: reason(r.reason), note: String(r.note || '').trim() }).then(function () {
      c.reported = true; S.report = null; S.reportErr = ''; paint('.st-back');
    }).catch(function (e) {
      S.reportErr = (e && e.message) || 'Could not send the report. Try again.'; paint('#st-rnote');
    });
  }
  function onClick(e) {
    var b = e.target.closest && e.target.closest('[data-st]'); if (!b || b.disabled) return;
    var act = b.getAttribute('data-st'), v = b.getAttribute('data-v');
    if (act === 'close') close();
    else if (act === 'view') { S.view = v; S.q = ''; S.page = 1; var q = $('#st-q'); if (q) q.value = ''; if (v === 'home') loadHome(); else loadList(); $('.st-main').focus(); }
    else if (act === 'sys') { S.all = v === 'all'; S.page = 1; if (S.view === 'item') S.view = S.from; reload(); }
    else if (act === 'page') { S.page += Number(v) || 0; loadList(); $('.st-main').focus(); }
    else if (act === 'item') { var c = findCard(b.getAttribute('data-id')); if (!c) return; S.from = S.view; S.item = c; S.view = 'item'; S.report = null; S.reportErr = ''; paint('#st-item-h'); }
    else if (act === 'back') { S.view = S.from; S.item = null; S.report = null; paint('.st-main'); }
    else if (act === 'add') addItem(b);
    else if (act === 'report-open') { S.report = { reason: 'other', note: '' }; paint('input[name="st-reason"]:checked'); }
    else if (act === 'report-cancel') { S.report = null; S.reportErr = ''; paint('[data-st="report-open"]'); }
    else if (act === 'report-send') sendReport(b);
    else if (act === 'retry') reload();
  }
  function onChange(e) { if (e.target && e.target.name === 'st-reason' && S.report) S.report.reason = reason(e.target.value); }
  function onInput(e) {
    if (!e.target || e.target.id !== 'st-q') return;
    clearTimeout(searchT);
    searchT = setTimeout(function () {
      var q = e.target.value.trim(); if (q === S.q) return;
      S.q = q; S.page = 1;
      if (!q && S.view === 'item') S.view = S.from;
      if (q) { if (S.view === 'item') S.view = S.from; loadList(); } else reload();
    }, 250);
  }

  /* ---- Opening and closing ---- */
  // The sheet's stylesheet loads on first use, and the sheet waits for it, so
  // it never shows unstyled. Hovering or focusing the icon starts the fetch
  // early; a stylesheet that never answers is given up on after 4s.
  var cssReady = null;
  function styleOnce() {
    if (cssReady) return cssReady;
    var href = host && host.getAttribute('data-css');
    if (!href || document.querySelector('link[data-store-css]')) { cssReady = Promise.resolve(); return cssReady; }
    var l = document.createElement('link'); l.rel = 'stylesheet'; l.href = href; l.setAttribute('data-store-css', '');
    cssReady = new Promise(function (done) {
      l.onload = l.onerror = function () { done(); };
      setTimeout(done, 4000);
    });
    document.head.appendChild(l);
    return cssReady;
  }
  function build() {
    dlg = document.createElement('dialog');
    dlg.className = 'st-dlg'; dlg.setAttribute('aria-labelledby', 'st-title');
    dlg.innerHTML = shell();
    dlg.addEventListener('click', onClick);
    dlg.addEventListener('change', onChange);
    dlg.addEventListener('input', onInput);
    // Escape rolls the sheet back up the same way the close button does.
    dlg.addEventListener('cancel', function (e) { e.preventDefault(); close(); });
    document.body.appendChild(dlg);
  }
  var opening = false;
  function open(opts) {
    if (!host || opening || (dlg && dlg.open)) return;
    opening = true;
    var mine = host, btn = toggle;
    if (btn && window.Chronicle.working) window.Chronicle.working(btn, true);
    styleOnce().then(function () {
      if (btn && window.Chronicle.working) window.Chronicle.working(btn, false);
      // A widget mounted again meanwhile is not the one that was clicked.
      if (host !== mine) return;
      opening = false;
      show(opts);
    });
  }
  function show(opts) {
    if (!dlg) build();
    var kind = opts && kindById(opts.kind) ? opts.kind : 'home';
    S.view = kind; S.from = kind; S.q = ''; S.page = 1; S.item = null; S.report = null; S.err = '';
    $('#st-q').value = '';
    dlg.setAttribute('data-motion', motion());
    dlg.classList.remove('st-in', 'st-out');
    dlg.showModal();
    document.documentElement.classList.add('st-open');
    if (toggle) toggle.setAttribute('aria-expanded', 'true');
    // The next frame starts the drape, so the sheet never shows before it moves.
    requestAnimationFrame(function () { requestAnimationFrame(function () { if (dlg.open) dlg.classList.add('st-in'); }); });
    if (kind === 'home') loadHome(); else loadList();
    $('#st-q').focus();
  }
  function close() {
    if (!dlg || !dlg.open || dlg.classList.contains('st-out')) return;
    clearTimeout(searchT);
    dlg.classList.remove('st-in'); dlg.classList.add('st-out');
    var wait = motion() === 'off' ? 0 : CLOSE_MS;
    setTimeout(function () {
      dlg.classList.remove('st-out'); dlg.close();
      document.documentElement.classList.remove('st-open');
      if (toggle) { toggle.setAttribute('aria-expanded', 'false'); toggle.focus(); }
    }, wait);
  }

  window.Chronicle.Store = {
    open: open,
    close: close,
    util: { query: query, pages: pages, notes: notes, systemName: systemName, kindWord: kindWord, kindOf: kindOf, reason: reason, emptyLine: emptyLine, KINDS: KINDS }
  };

  if (window.Chronicle.register) {
    window.Chronicle.register('store', {
      init: function (el) {
        host = el; campaign = el.getAttribute('data-campaign') || ''; csrf = el.getAttribute('data-csrf') || '';
        toggle = el.querySelector('[data-store-toggle]');
        if (toggle) {
          toggle.addEventListener('click', function () { if (dlg && dlg.open) close(); else open(); });
          toggle.addEventListener('pointerenter', styleOnce);
          toggle.addEventListener('focus', styleOnce);
        }
      },
      destroy: function () {
        if (dlg) { if (dlg.open) dlg.close(); dlg.remove(); dlg = null; }
        document.documentElement.classList.remove('st-open');
        host = null; toggle = null; opening = false; cssReady = null; S = freshState();
      }
    });
  }
})();
