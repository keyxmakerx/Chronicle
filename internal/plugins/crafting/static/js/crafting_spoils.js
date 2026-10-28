/**
 * crafting_spoils.js -- the Director's Spoils tab (window.CraftingSpoils)
 *
 * The party's spoils tray with a hand-out on every line, putting a page's
 * drops in by hand, what was taken lately and every page's drop list, as the
 * signed Crafting Spoils mockup. Taking, harvesting and clearing go to the
 * server, which rolls a harvest's test and fills the hero's pack; the tab
 * shows what it answered and reloads. A drop list saves as it is edited,
 * with its version, so two Directors never silently overwrite each other.
 * Players never see this tab or a drop list, only what lands in the tray.
 */
window.CraftingSpoils = (function () {
  'use strict';
  var L = window.CraftingLib, T = window.CraftingTray, esc = L.esc;
  var MORE = '<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><circle cx="3" cy="8" r="1.5" fill="currentColor"/><circle cx="8" cy="8" r="1.5" fill="currentColor"/><circle cx="13" cy="8" r="1.5" fill="currentColor"/></svg>';
  var X = '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>';
  // The server's cap on a drop line's amounts (validate.go maxQty).
  var MAX_QTY = 99;

  function plural(n, one, many) { return n + ' ' + (n === 1 ? one : many); }

  /* The line beside the tray's buttons. */
  function trayLine(tray) {
    var n = T.count(tray);
    if (!n) return 'Nothing is waiting.';
    return plural(n, 'thing', 'things') + ' from ' + plural(tray.length, 'source', 'sources') + ', waiting until someone takes them or you clear them.';
  }
  /* A drop list in a few words, for its row. */
  function dropWords(d) {
    return plural(d.lines.length, 'line', 'lines') + (d.also ? ', plus what ' + (d.alsoName || 'another page') + ' drops' : '');
  }
  /* A line switched to a mode, keeping what carries over: a harvest starts
     at the first characteristic and pays 1, 2 and 3. */
  function lineAs(l, mode, stats) {
    if (mode === 'harvest') return {itemId: l.itemId, mode: mode, stat: l.stat || (stats[0] ? stats[0].key : ''), yields: l.yields && l.yields.length === 3 ? l.yields : [1, 2, 3]};
    return {itemId: l.itemId, mode: 'take', qty: l.qty || 1};
  }
  /* A typed amount, held to what the server takes; the lowest tier of a
     harvest pays at least 1. */
  function amount(v, least) {
    var n = Math.round(Number(v));
    return Math.max(least, Math.min(MAX_QTY, isFinite(n) ? n : least));
  }

  function mount(el, app) {
    var page = null, open = null, got = null, editing = null, busy = false, dead = false;
    var picks = {}, lastHero = '', putSrc = '', putN = 1, q = '', seq = 0, searchT = 0, saveT = {}, saving = {}, dirty = {};

    function isOpen(kind, id) { return !!(open && open.kind === kind && open.id === String(id)); }
    function drop(pid) { for (var i = 0; i < page.drops.length; i++) if (page.drops[i].pageId === pid) return page.drops[i]; return null; }
    function line(id) {
      for (var i = 0; i < page.tray.length; i++) for (var j = 0; j < page.tray[i].lines.length; j++) if (String(page.tray[i].lines[j].id) === String(id)) return page.tray[i].lines[j];
      return null;
    }
    function spoil(id) { for (var i = 0; i < page.tray.length; i++) if (page.tray[i].id === id) return page.tray[i]; return null; }
    function itemName(id) { for (var i = 0; i < page.items.length; i++) if (page.items[i].id === id) return page.items[i].name; return ''; }
    function focus(sel) { setTimeout(function () { var f = el.querySelector(sel); if (f) f.focus(); }, 0); }
    function hero(lid) {
      var h = picks[lid] || lastHero;
      return page.heroes.some(function (x) { return x.id === h; }) ? h : (page.heroes[0] ? page.heroes[0].id : '');
    }

    /* ---------- The tray ---------- */

    function lineActs(l) {
      if (got && isOpen('harvest', l.id)) return '<span class="crp-pill is-ok">Harvested</span>';
      var id = esc(String(l.id)), h = '';
      if (page.heroes.length) {
        var who = hero(l.id);
        h = '<label class="sr-only" for="crs-who-' + id + '">Who gets ' + esc(l.name) + '</label><select class="cra-in cra-sm" id="crs-who-' + id + '" data-f="who" data-id="' + id + '">' +
          page.heroes.map(function (x) { return '<option value="' + esc(x.id) + '"' + (x.id === who ? ' selected' : '') + '>' + esc(x.name) + '</option>'; }).join('') + '</select>' +
          '<button type="button" class="btn btn-secondary btn-sm" data-act="give" data-id="' + id + '"' + (busy ? ' disabled' : '') + '>' + (l.mode === 'harvest' ? 'Harvest for them' : 'Give') + '</button>';
      } else h = '<span class="cra-fine">No characters to give it to</span>';
      return h + '<button type="button" class="crp-ib" data-act="clearline" data-id="' + id + '"' + (busy ? ' disabled' : '') + ' aria-label="Clear ' + esc(l.name) + '">' + X + '</button>';
    }
    function cardHead(s) {
      var id = esc(s.id), on = isOpen('menu', s.id);
      return '<span class="crp-acts"><button type="button" class="crp-ib" data-act="menu" data-id="' + id + '" aria-haspopup="menu" aria-expanded="' + on + '" aria-label="More for ' + esc(s.source) + '">' + MORE + '</button>' +
        (on ? '<div class="crp-menu" role="menu"><button type="button" role="menuitem" class="is-bad" data-act="askspoil" data-id="' + id + '">Clear what ' + esc(s.source) + ' left</button></div>' : '') + '</span>';
    }
    function cardUnder(s) {
      if (!isOpen('spoil', s.id)) return '';
      return '<div class="crp-confirm"><p>Clear everything ' + esc(s.source) + ' left? Nobody gets it, and it can’t come back.</p>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="close">Keep it</button><button type="button" class="btn btn-danger btn-sm" data-act="clearspoil" data-id="' + esc(s.id) + '">Clear it</button></div>';
    }
    function tray() {
      if (!page.tray.length) return '<div class="cra-empty">The tray is empty. It fills when you end a fight, or when you put spoils in by hand.</div>';
      return '<div class="crs-tray">' + page.tray.map(function (s) {
        return T.card(s, {acts: lineActs, head: cardHead, under: cardUnder,
          fold: function (l) { return got && isOpen('harvest', l.id) ? T.result(got, l.id, 'Harvest: ' + got.item + ' for ' + got.hero) : ''; },
          hit: function (l) { return got && isOpen('harvest', l.id) ? got.tier || 0 : -1; }});
      }).join('') + '</div>';
    }
    function putFold() {
      if (isOpen('clear', 'all')) {
        return '<div class="crp-confirm crs-confirm"><p>Clear the whole tray? Nobody gets any of it, and it can’t come back.</p>' +
          '<button type="button" class="btn btn-secondary btn-sm" data-act="close">Keep it</button><button type="button" class="btn btn-danger btn-sm" data-act="cleartray">Clear the tray</button></div>';
      }
      if (!isOpen('put', 'tray')) return '';
      if (!page.drops.length) return '<div class="crs-put"><p class="cra-fine">Nothing has a drop list yet. Give a page one under Drop lists, then put its spoils in.</p>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="close">Not now</button></div>';
      if (!drop(putSrc)) putSrc = page.drops[0].pageId;
      return '<div class="crs-put"><label for="crs-put-src">From</label><select class="cra-in cra-sm" id="crs-put-src" data-f="putsrc">' +
        page.drops.map(function (d) { return '<option value="' + esc(d.pageId) + '"' + (d.pageId === putSrc ? ' selected' : '') + '>' + esc(d.page) + '</option>'; }).join('') + '</select>' +
        '<label for="crs-put-n">How many</label><input type="number" class="cra-in cra-sm num" id="crs-put-n" data-f="putn" min="1" max="20" value="' + putN + '">' +
        '<button type="button" class="btn btn-primary btn-sm" data-act="putgo"' + (busy ? ' disabled' : '') + '>Put in the tray</button><button type="button" class="btn btn-secondary btn-sm" data-act="close">Not now</button>' +
        '<p class="cra-fine">For a fight run in Foundry, a place the party searches, or a ruined item. Each one drops its whole list once.</p></div>';
    }

    /* ---------- Drop lists ---------- */

    function editor(d) {
      var pid = esc(d.pageId), stats = page.stats || [];
      var others = page.drops.filter(function (x) { return x.pageId !== d.pageId && (!x.also || x.pageId === d.also); });
      var also = '<div class="crs-also"><label for="crs-also-' + pid + '">Also drops what</label><select class="cra-in cra-sm" id="crs-also-' + pid + '" data-f="also" data-id="' + pid + '">' +
        '<option value="">no other page</option>' + others.map(function (x) { return '<option value="' + esc(x.pageId) + '"' + (x.pageId === d.also ? ' selected' : '') + '>' + esc(x.page) + '</option>'; }).join('') +
        (d.also && !others.some(function (x) { return x.pageId === d.also; }) ? '<option value="' + esc(d.also) + '" selected>' + esc(d.alsoName || 'Another page') + '</option>' : '') +
        '</select><span class="cra-fine">drops. For a Remnant: its original’s list comes first, then these.</span></div>';
      var only = d.lines.length === 1 && !d.also;
      var rows = d.lines.map(function (l, i) {
        var at = ' data-id="' + pid + '" data-i="' + i + '"', key = pid + '-' + i, name = itemName(l.itemId);
        var items = page.items.map(function (it) { return '<option value="' + esc(it.id) + '"' + (it.id === l.itemId ? ' selected' : '') + '>' + esc(it.name) + '</option>'; }).join('');
        if (!name) items = '<option value="' + esc(l.itemId) + '" selected>A deleted item</option>' + items;
        var right;
        if (l.mode === 'harvest') {
          right = '<div class="crs-yields"><label>' + esc(page.dice || '2d10') + ' + <select class="cra-in cra-sm" data-f="stat"' + at + ' aria-label="Characteristic">' +
            '<option value="">nothing</option>' + stats.map(function (s) { return '<option value="' + esc(s.key) + '"' + (s.key === l.stat ? ' selected' : '') + '>' + esc(s.label) + '</option>'; }).join('') +
            '</select></label><span class="crs-ytiers">' + (page.tiers || []).map(function (t, j) {
              return '<label>' + esc(t) + ' <input type="number" class="cra-in cra-sm num" data-f="y" data-j="' + j + '"' + at + ' min="' + (j ? 0 : 1) + '" max="' + MAX_QTY + '" value="' + (l.yields[j] || 0) + '"></label>';
            }).join('') + '</span></div>';
        } else right = '<div class="crs-yields"><label>How many <input type="number" class="cra-in cra-sm num" data-f="qty"' + at + ' min="1" max="' + MAX_QTY + '" value="' + l.qty + '"></label></div>';
        return '<div class="crs-dl-row"><select class="cra-in cra-sm" id="crs-item-' + key + '" data-f="item"' + at + ' aria-label="Item">' + items + '</select>' +
          '<select class="cra-in cra-sm" id="crs-mode-' + key + '" data-f="mode"' + at + ' aria-label="How it is taken"><option value="take"' + (l.mode === 'take' ? ' selected' : '') + '>Take as it is</option>' +
          '<option value="harvest"' + (l.mode === 'harvest' ? ' selected' : '') + '>Harvest with a test</option></select>' + right +
          (only ? '<span></span>' : '<button type="button" class="crp-ib" data-act="rmline"' + at + ' aria-label="Remove ' + esc(name || 'this line') + '">' + X + '</button>') + '</div>';
      }).join('');
      var h = '<div class="crs-dl">' + also + rows + '<div class="crs-dl-foot">' +
        (d.lines.length < 20 ? '<button type="button" class="btn btn-secondary btn-sm" data-act="addline" data-id="' + pid + '">Add a line</button>' : '') +
        '<span class="cra-fine">Items come from the Armory.' + (page.tiers && page.tiers[0] ? ' Even ' + esc(page.tiers[0]) + ' pays at least 1.' : '') + '</span>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="askdrop" data-id="' + pid + '">Remove this drop list</button></div>';
      if (isOpen('rmdrop', d.pageId)) h += '<div class="crp-confirm crs-confirm"><p>Remove ' + esc(d.page) + '’s drop list? What is already in the tray stays.</p>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="close">Keep it</button><button type="button" class="btn btn-danger btn-sm" data-act="rmdrop" data-id="' + pid + '">Remove it</button></div>';
      return h + '</div>';
    }
    function dropRows() {
      if (!page.drops.length) return '<tr><td colspan="4" class="cra-fine">No drop lists yet. Find a page above to give it one.</td></tr>';
      return page.drops.map(function (d) {
        var on = editing === d.pageId, pid = esc(d.pageId);
        return '<tr><td class="is-who">' + esc(d.page) + '</td><td>' + esc(d.kind) + '</td><td data-words="' + pid + '">' + esc(dropWords(d)) + '</td>' +
          '<td class="is-r"><button type="button" class="btn btn-secondary btn-sm" data-act="edit" data-id="' + pid + '" aria-expanded="' + on + '">' + (on ? 'Close' : 'Edit') + '</button></td></tr>' +
          (on ? '<tr><td colspan="4" class="is-strip">' + editor(d) + '</td></tr>' : '');
      }).join('');
    }

    function render() {
      if (!page) return;
      el.innerHTML = '<div class="crp-tab"><div class="crp-resp"><div><b>In the tray</b><span class="cra-fine">' + esc(trayLine(page.tray)) + '</span></div>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="put" aria-expanded="' + isOpen('put', 'tray') + '">Put spoils in the tray</button>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="askclear"' + (page.tray.length ? '' : ' disabled') + '>Clear the tray</button></div>' +
        putFold() + tray() +
        '<section class="crs-card"><div class="crp-head"><h3>Taken lately</h3></div><ul class="crp-done">' +
        (page.taken.length ? page.taken.map(function (t) { return '<li><span>' + esc(t.text) + '</span><span class="cra-fine">' + esc(T.ago(t.at)) + '</span></li>'; }).join('') : '<li class="cra-fine">Nothing taken yet.</li>') +
        '</ul></section><section class="crs-card" aria-labelledby="crs-dl-h"><div class="crp-head"><h3 id="crs-dl-h">Drop lists</h3></div>' +
        '<p class="cra-fine">What each page leaves when it is defeated, harvested or found. A creature, a Remnant, a place or a ruined item can each have one. Players never see a drop list, only what lands in the tray.</p>' +
        '<div class="crs-add"><label class="sr-only" for="crs-q">Give a page a drop list</label><input type="search" class="cra-in" id="crs-q" autocomplete="off" placeholder="Give a page a drop list: find it by name" value="' + esc(q) + '">' +
        '<div class="crs-found" hidden></div></div>' +
        '<div class="crp-twrap"><table class="crp-table" aria-label="Drop lists"><thead><tr><th scope="col">Page</th><th scope="col">Kind</th><th scope="col">Drops</th><th scope="col"><span class="sr-only">Actions</span></th></tr></thead><tbody>' +
        dropRows() + '</tbody></table></div>' +
        (page.truncated ? '<p class="cra-note">This campaign has more pages than one list holds, so some items or characters are missing from the pickers.</p>' : '') + '</section></div>';
    }

    function load() {
      return app.api('GET', '/spoils').then(function (res) {
        if (dead) return;
        page = res;
        render();
      }, function (err) {
        if (dead) return;
        el.innerHTML = '<div class="cra-empty cra-bad" role="alert">Spoils couldn’t load: ' + esc(err.message) + '. <button type="button" class="cra-link" data-act="retry">Try again</button></div>';
      });
    }
    /* After a change, reload and say what happened; a refusal says why. */
    function then(p, msg) {
      busy = true;
      render();
      return p.then(function (res) {
        busy = false; open = null;
        var said = typeof msg === 'function' ? msg(res) : msg;
        return load().then(function () { if (said) app.toast(said); });
      }, function (err) {
        busy = false; open = null;
        app.toast(err.message.charAt(0).toUpperCase() + err.message.slice(1));
        return load();
      });
    }

    /* Gives a line to the hero picked beside it; a harvest's test unfolds
       where the line was until Done. */
    function give(lid) {
      var l = line(lid), who = hero(lid);
      if (!l || !who) return;
      lastHero = who;
      if (l.mode !== 'harvest') {
        then(app.api('POST', '/spoil-lines/' + encodeURIComponent(lid) + '/take', {characterId: who}), function (res) {
          return res.qty > 0 ? 'Gave ' + res.got + ' to ' + res.hero + '.' : 'That paid nothing.';
        });
        return;
      }
      busy = true;
      render();
      app.api('POST', '/spoil-lines/' + encodeURIComponent(lid) + '/take', {characterId: who}).then(function (res) {
        busy = false;
        if (dead) return;
        got = res;
        open = {kind: 'harvest', id: String(lid)};
        render();
        T.throwDice(el.querySelector('.crp-result'), res, 'Harvest: ' + res.item, 'a test of ' + res.total);
        focus('[data-act="sdone"]');
      }, function (err) {
        busy = false;
        if (dead) return;
        app.toast(err.message.charAt(0).toUpperCase() + err.message.slice(1));
        load();
      });
    }

    /* A drop list saves a moment after the last change, one save at a time
       per page; the answer brings the next version. */
    function save(pid) {
      clearTimeout(saveT[pid]);
      dirty[pid] = true;
      app.status('saving');
      saveT[pid] = setTimeout(function () { flush(pid); }, 400);
    }
    function flush(pid, keep) {
      var d = drop(pid);
      clearTimeout(saveT[pid]);
      if (!d || !dirty[pid] || saving[pid]) return;
      dirty[pid] = false;
      saving[pid] = true;
      app.api('PUT', '/drops/' + encodeURIComponent(pid), {also: d.also || '', lines: d.lines, version: d.version}, keep).then(function (res) {
        saving[pid] = false;
        if (dead) return;
        d.version = res.version;
        d.alsoName = res.alsoName;
        words(d);
        if (dirty[pid]) flush(pid);
        else app.status('saved');
      }, function (err) {
        saving[pid] = false;
        if (dead) return;
        if (err.status === 409) app.status('error', 'Someone else changed ' + d.page + '’s drop list. Reload to see it.', app.reloadFix);
        else app.status('error', d.page + ': ' + err.message);
      });
    }
    function words(d) { var c = el.querySelector('[data-words="' + d.pageId + '"]'); if (c) c.textContent = dropWords(d); }

    /* Pages to give a drop list, found as the Director types; a page that
       already has one isn't offered. */
    function search() {
      var box = el.querySelector('.crs-found'), mine = ++seq;
      if (!box) return;
      if (q.trim().length < 2) { box.hidden = true; box.innerHTML = ''; return; }
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(app.cid) + '/entities/search?q=' + encodeURIComponent(q.trim()), {headers: {Accept: 'application/json'}})
        .then(function (res) { return res.ok ? res.json() : {results: []}; })
        .then(function (body) {
          if (dead || mine !== seq || !(box = el.querySelector('.crs-found'))) return;
          var pages = (body.results || []).filter(function (r) { return r.id && /\/entities\//.test(r.url || '') && !drop(r.id); }).slice(0, 8);
          box.innerHTML = pages.length ? pages.map(function (r) {
            return '<button type="button" data-act="newdrop" data-id="' + esc(r.id) + '">' + esc(r.name) + (r.type_name ? '<small>' + esc(r.type_name) + '</small>' : '') + '</button>';
          }).join('') : '<p class="cra-fine">No page matches, or every match has a drop list.</p>';
          box.hidden = false;
        }, function () {});
    }
    function create(pid) {
      if (!page.items.length) { app.toast('Add an item to the Armory first: drop lists give Armory items.'); return; }
      app.status('saving');
      app.api('PUT', '/drops/' + encodeURIComponent(pid), {lines: [{itemId: page.items[0].id, mode: 'take', qty: 1}], version: 0}).then(function (d) {
        if (dead) return;
        page.drops.push(d);
        page.drops.sort(function (a, b) { return a.page.toLowerCase().localeCompare(b.page.toLowerCase()); });
        editing = d.pageId; q = ''; open = null;
        app.status('saved');
        render();
        app.toast(d.page + ' has a drop list. Set what it drops.');
        focus('#crs-item-' + d.pageId + '-0');
      }, function (err) {
        if (dead) return;
        app.status('error', err.message.charAt(0).toUpperCase() + err.message.slice(1), err.status === 409 ? app.reloadFix : null);
      });
    }

    el.addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]');
      if (!b || !el.contains(b) || b.disabled) return;
      var act = b.getAttribute('data-act'), id = b.getAttribute('data-id') || '', i = Number(b.getAttribute('data-i')), d = id ? drop(id) : null;
      if (act === 'retry') { load(); return; }
      if (busy) return;
      switch (act) {
        case 'put': open = isOpen('put', 'tray') ? null : {kind: 'put', id: 'tray'}; render(); if (open) focus('#crs-put-src'); break;
        case 'askclear': open = {kind: 'clear', id: 'all'}; render(); focus('[data-act="cleartray"]'); break;
        case 'close': open = null; render(); break;
        case 'putgo':
          var n = amount(putN, 1);
          if (n > 20) n = 20;
          var src = drop(putSrc);
          if (!src) break;
          then(app.api('POST', '/spoils', {pageId: putSrc, copies: n}), src.page + '’s spoils are in the tray' + (n > 1 ? ', ' + n + ' times' : '') + '.');
          putN = 1;
          break;
        case 'cleartray': then(app.api('DELETE', '/spoils'), 'The tray is empty.'); break;
        case 'menu': open = isOpen('menu', id) ? null : {kind: 'menu', id: id}; render(); focus('.crp-menu button'); break;
        case 'askspoil': open = {kind: 'spoil', id: id}; render(); focus('[data-act="clearspoil"]'); break;
        case 'clearspoil': var s = spoil(id); then(app.api('DELETE', '/spoils/' + encodeURIComponent(id)), 'Cleared what ' + (s ? s.source : 'it') + ' left.'); break;
        case 'clearline': var l = line(id); then(app.api('DELETE', '/spoil-lines/' + encodeURIComponent(id)), 'Cleared ' + (l ? l.name : 'it') + '. Nobody gets it.'); break;
        case 'give': give(id); break;
        case 'sdone': open = null; got = null; load(); break;
        case 'edit': editing = editing === id ? null : id; open = null; render(); break;
        case 'addline':
          if (!d || !page.items.length) break;
          d.lines.push({itemId: page.items[0].id, mode: 'take', qty: 1});
          render(); save(id); focus('#crs-item-' + id + '-' + (d.lines.length - 1));
          break;
        case 'rmline': if (d) { d.lines.splice(i, 1); render(); save(id); } break;
        case 'askdrop': open = {kind: 'rmdrop', id: id}; render(); focus('[data-act="rmdrop"]'); break;
        case 'rmdrop':
          if (!d) break;
          clearTimeout(saveT[id]); dirty[id] = false;
          editing = null;
          then(app.api('DELETE', '/drops/' + encodeURIComponent(id)), d.page + '’s drop list is gone. What was in the tray stays.');
          break;
        case 'newdrop': create(id); break;
      }
    });

    el.addEventListener('change', function (e) {
      var t = e.target, f = t.getAttribute('data-f');
      if (!f) return;
      if (f === 'who') { picks[t.getAttribute('data-id')] = t.value; lastHero = t.value; return; }
      if (f === 'putsrc') { putSrc = t.value; return; }
      if (f === 'putn') { putN = Math.min(20, amount(t.value, 1)); t.value = putN; return; }
      var pid = t.getAttribute('data-id'), d = pid && drop(pid);
      if (!d) return;
      var l = d.lines[Number(t.getAttribute('data-i'))];
      if (f === 'also') { d.also = t.value; d.alsoName = t.value ? (drop(t.value) || {}).page || '' : ''; render(); focus('#crs-also-' + pid); }
      else if (!l) return;
      else if (f === 'item') l.itemId = t.value;
      else if (f === 'mode') { d.lines[Number(t.getAttribute('data-i'))] = lineAs(l, t.value, page.stats || []); render(); focus('#' + t.id); }
      else if (f === 'qty') { l.qty = amount(t.value, 1); t.value = l.qty; }
      else if (f === 'stat') l.stat = t.value;
      else if (f === 'y') {
        var j = Number(t.getAttribute('data-j'));
        if (j === 0 && !(Number(t.value) >= 1)) app.toast('Even ' + (page.tiers[0] || 'the lowest tier') + ' pays something: set it to 1 or more.');
        l.yields[j] = amount(t.value, j ? 0 : 1);
        t.value = l.yields[j];
      }
      words(d);
      save(pid);
    });

    el.addEventListener('input', function (e) {
      if (e.target.id !== 'crs-q') return;
      q = e.target.value;
      clearTimeout(searchT);
      searchT = setTimeout(search, 250);
    });

    el.addEventListener('keydown', function (e) {
      if (e.key !== 'Escape') return;
      var box = el.querySelector('.crs-found');
      if (box && !box.hidden) { box.hidden = true; return; }
      if (open && open.kind !== 'harvest') { var was = open; open = null; render(); if (was.kind === 'menu') focus('[data-act="menu"][data-id="' + was.id + '"]'); }
    });

    el.innerHTML = '<div class="cra-loading" aria-hidden="true"><i></i><i></i><i></i></div>';
    return {
      // Fresh each time the tab opens: fights end and players take in between.
      shown: function () { if (!Object.keys(dirty).some(function (k) { return dirty[k]; })) load(); },
      destroy: function () {
        Object.keys(dirty).forEach(function (k) { if (dirty[k]) flush(k, true); });
        dead = true; clearTimeout(searchT);
        el.innerHTML = '';
      }
    };
  }
  return {mount: mount, trayLine: trayLine, dropWords: dropWords, lineAs: lineAs, amount: amount};
})();
