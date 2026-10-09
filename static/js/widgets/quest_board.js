/**
 * quest_board.js: the Quest board block on a quest's page, and for the DM the
 * Quest ledger that folds out of the bookmark under it.
 *
 * The board shows the quest's notice, the map it points at and its reward
 * tag; clicking the notice lifts it into a reading sheet. Players get only
 * what the server lets them see (steps the DM showed, nothing hidden), so this
 * file never filters anything for safety, only for looks. The DM moves the
 * pieces, edits the notice on the sheet itself, and keeps steps, rewards,
 * foes and links in the ledger; every change saves straight away.
 *
 * Mount: <div data-widget="quest_board" data-endpoint data-campaign-id
 * data-entity-id data-csrf-token data-can-edit>. Shared parts:
 * quest_board_kit.js; styles: static/css/quest_board.css.
 */
(function () {
  'use strict';
  var K = window.QuestBoardKit;
  if (!K || !window.Chronicle) return;
  var I = K.I, esc = K.esc, lrow = K.lrow;

  var STATUS = [['not_started', 'Not started'], ['active', 'Active'], ['done', 'Done'], ['failed', 'Failed']];
  var REWARD_KINDS = { money: ['coin', 'money'], item: ['item', 'item'], other: ['star', 'other'] };
  var uid = 0;

  Chronicle.register('quest_board', {
    init: function (el, cfg) {
      var S = {
        el: el, cfg: cfg, id: 'qb' + (++uid), data: null, saving: Promise.resolve(),
        campaignUrl: '/campaigns/' + encodeURIComponent(cfg.campaignId),
        tab: 'steps', stops: []
      };
      el.classList.add('qbk');
      el.innerHTML = '<div class="loading">Loading the quest board…</div>';
      el._qb = S;
      load(S);
      S.stops.push(K.live(cfg.campaignId, function (m) { onLive(S, m); }));
    },
    destroy: function (el) {
      var S = el._qb; if (!S) return;
      clearTimeout(S.liveTimer);
      S.stops.forEach(function (f) { f(); });
      el._qb = null;
    }
  });

  function load(S) {
    K.api(S.cfg.endpoint).then(function (data) {
      S.data = normalise(data);
      first(S);
    }).catch(function (err) {
      if (err.status === 404) { S.el.hidden = true; return; }
      S.el.innerHTML = '<div class="loading">' + esc(err.message) + '</div>';
    });
  }

  // Fill the gaps an older or partial answer leaves, so rendering never trips.
  function normalise(d) {
    d = d || {};
    d.layout = d.layout || {};
    ['notice', 'map', 'tag'].forEach(function (k) { d.layout[k] = d.layout[k] || {}; });
    d.looks = d.looks || { board: 'lit', ledger: 'lit' };
    ['steps', 'rewards', 'foes', 'links'].forEach(function (k) { d[k] = d[k] || []; });
    if (d.notice) d.notice.body = d.notice.body || [];
    if (!d.map && d.mapId) d.map = { id: d.mapId, name: d.mapName || 'Map' };
    return d;
  }
  function dm(S) { return !!(S.data && S.data.canEdit); }

  // ---------- Rendering ----------
  function first(S) {
    var el = S.el;
    el.innerHTML = '<div class="bw q">' + boardHTML(S, true) + (dm(S) ? K.ribbon('Quest ledger') + ledgerHTML(S) : '') + '</div>';
    K.setLooks(el, S.data.looks);
    var board = el.querySelector('.board');
    setTimeout(function () { board.classList.remove('enter'); }, 1600);
    S.stops.push(K.dust(el.querySelector('.dust')), K.restLoops(el));
    wire(S);
    decorate(S);
  }
  function redrawBoard(S) {
    var old = S.el.querySelector('.board'); if (!old) return;
    var tmp = document.createElement('div'); tmp.innerHTML = boardHTML(S, false);
    var nb = tmp.firstChild; old.replaceWith(nb);
    S.stops.push(K.dust(nb.querySelector('.dust')));
    decorate(S);
  }
  function redrawLedger(S) {
    var old = S.el.querySelector('.ledger'); if (!old) return;
    var keep = {}; old.querySelectorAll('.lpanel').forEach(function (p) { keep[p.dataset.lpanel] = p.scrollTop; });
    var tmp = document.createElement('div'); tmp.innerHTML = ledgerHTML(S);
    var nl = tmp.firstChild; old.replaceWith(nl);
    // An open hand-out panel and its warning survive a redraw underneath them.
    old.querySelectorAll(':scope > .give, :scope > .warn').forEach(function (k) { nl.appendChild(k); });
    nl.querySelectorAll('.lpanel').forEach(function (p) { p.scrollTop = keep[p.dataset.lpanel] || 0; });
    K.syncPin(nl.closest('.bw'));
    K.setLooks(S.el, S.data.looks);
  }

  function boardHTML(S, enter) {
    var d = S.data, n = d.notice, L = d.layout, isDm = dm(S), parts = [], i = 0;
    var plate = (n && n.plate) || (isDm ? 'Name this board' : '');
    var plateHTML = plate ? '<div class="bplate"' + (isDm ? ' data-edit="plate" title="Double-click to rename"' : '') + '>' + esc(plate) + '</div>' : '';
    if (n && (isDm || !L.notice.hidden)) {
      var stamp = d.status === 'done' ? 'Done' : d.status === 'failed' ? 'Failed' : '';
      parts.push(K.noticeHTML(n, { style: K.placeStyle(pos(L.notice, 7, 8, 58, -2.5), i++), attrs: ' data-id="notice" data-piece="notice"',
        seal: K.sealLetter(n.title), stamp: stamp, cls: L.notice.hidden ? 'veiled' : '' }));
    }
    if (d.map && (isDm || !L.map.hidden)) {
      parts.push('<div class="note scrap' + (L.map.hidden ? ' veiled' : '') + '" tabindex="0" role="button" aria-label="Open the map: ' + esc(d.map.name) + '" data-id="map" data-piece="map" style="' +
        K.placeStyle(pos(L.map, 61, 44, 34, 7), i++) + '"><span class="pin brass"></span><div class="pp torn" style="padding:16px 6px 6px">' + K.mapSVG(d.map.name) + '</div></div>');
    }
    var reward = n && n.reward;
    if (reward && (isDm ? true : d.showTag !== false && !L.tag.hidden)) {
      var cut = reward.search(/[,;(]/), big = cut > 0 ? reward.slice(0, cut) : reward, small = cut > 0 ? reward.slice(cut + 1).replace(/^[\s)]+|[\s)]+$/g, '') : '';
      parts.push('<div class="tag' + (L.tag.hidden ? ' veiled' : '') + '" tabindex="0" role="button" aria-label="Read the reward" data-id="tag" data-piece="tag" style="' +
        K.placeStyle(pos(L.tag, 9, 66, 30, -8), i++) + '"><span class="str"></span><div class="tg">' + esc(big) + (small ? '<small>' + esc(small) + '</small>' : '') + '</div><span class="hole"></span></div>');
    }
    if (!parts.length) parts.push('<div class="empty">Nothing is posted here yet.</div>');
    var html = K.boardFrame('q', plateHTML, parts.join(''));
    return enter ? html : html.replace('class="board q enter"', 'class="board q"');
  }
  function pos(p, x, y, w, r) {
    return { x: num(p.x, x), y: num(p.y, y), w: num(p.w, w), r: num(p.r, r) };
  }
  function num(v, d) { return typeof v === 'number' && isFinite(v) ? v : d; }

  // ---------- The reading sheet ----------
  function sheetHTML(S) {
    var d = S.data, n = d.notice || {}, isDm = dm(S);
    function line(field, tag, cls, text, ph) {
      if (!isDm) return text ? '<' + tag + (cls ? ' class="' + cls + '"' : '') + '>' + esc(text) + '</' + tag + '>' : '';
      return '<' + tag + ' class="' + (cls ? cls + ' ' : '') + 'edit" data-edit="' + field + '" tabindex="0" title="Double-click to change">' +
        (text ? esc(text) : '<em class="ph">' + esc(ph) + '</em>') + '</' + tag + '>';
    }
    var steps = d.steps.map(function (s) {
      if (!isDm && s.shown === false) return null;
      return '<li class="' + (s.done ? 'done' : '') + '"><span class="bx"></span><span>' + esc(s.text) +
        (isDm && !s.shown ? ' <em style="font-size:.85em">(hidden from players)</em>' : '') + '</span></li>';
    }).filter(Boolean);
    if (!isDm && d.hiddenSteps) steps.splice(Math.max(0, steps.length - 1), 0, '<li class="unknown"><span class="bx"></span><span>Something more, not yet known…</span></li>');
    var body = (n.body || []).map(function (p, i) {
      return isDm ? '<p class="edit" data-edit="body" data-p="' + i + '" tabindex="0" title="Double-click to change">' + esc(p) + '</p>' : '<p>' + esc(p) + '</p>';
    }).join('');
    if (isDm) body += '<p class="edit" data-edit="body" data-p="' + (n.body || []).length + '" tabindex="0"><em class="ph">Add a paragraph…</em></p>';
    return line('kicker', 'div', 'k', n.kicker, 'Help wanted') +
      (isDm ? line('title', 'h2', '', n.title, 'Title').replace('<h2 ', '<h2 id="qbk-sheet-title" ') : '<h2 id="qbk-sheet-title">' + esc(n.title) + '</h2>') +
      line('postedBy', 'div', 'by', n.postedBy, 'Posted by…') + body +
      ((n.reward || isDm) ? '<h4>Reward</h4>' + line('reward', 'p', '', n.reward, 'What the reward is') : '') +
      (steps.length ? '<h4>What must be done</h4><ul class="steps">' + steps.join('') + '</ul>' : '') +
      '<div class="sfoot">' + line('due', 'span', 'due', n.due, 'When it is due') +
      '<span style="display:flex;gap:10px;align-items:center"><button type="button" class="pbtn" data-close>Pin it back</button><span class="seal" aria-hidden="true">' + esc(K.sealLetter(n.title)) + '</span></span></div>' +
      (isDm ? '<div class="dmhint">Double-click any line to change it. Players read this sheet without the hidden steps.</div>' : '');
  }
  function openNotice(S, note) {
    var sheet = K.openSheet(note, sheetHTML(S), S.data.looks.board, null);
    if (!sheet || !dm(S)) return;
    sheet.ondblclick = function (e) { editSheetLine(S, sheet, e.target.closest('[data-edit]')); };
    sheet.onkeydown = function (e) {
      var t = e.target.closest('[data-edit]');
      if (t && e.key === 'Enter' && !t.isContentEditable) { e.preventDefault(); editSheetLine(S, sheet, t); }
    };
  }
  var LIMITS = { kicker: 40, title: 120, postedBy: 160, reward: 160, due: 160, body: 2000, plate: 60 };
  function editSheetLine(S, sheet, t) {
    if (!t) return;
    var field = t.dataset.edit;
    if (t.querySelector('.ph')) t.textContent = '';
    K.editInline(t, {
      max: LIMITS[field], allowEmpty: field !== 'title', multiline: field === 'body',
      onSave: function (text) { saveNotice(S, field, text, +t.dataset.p); sheet.innerHTML = sheetHTML(S); },
      onCancel: function () { sheet.innerHTML = sheetHTML(S); }
    });
  }
  function saveNotice(S, field, text, p) {
    var n = S.data.notice = S.data.notice || { body: [] };
    if (field === 'body') {
      var body = (n.body || []).slice();
      if (text) body[p] = text; else body.splice(p, 1);
      n.body = body.filter(function (x) { return x; });
    } else n[field] = text;
    save(S, { notice: n }, field === 'plate' || field === 'title' || field === 'kicker' || field === 'reward' ? 'board' : '');
  }

  // ---------- Live updates ----------
  // Someone else saved this quest: fetch it again, but never under a viewer
  // who is mid-change; wait until they finish so nothing they typed is lost.
  function onLive(S, m) {
    var mine = m.type === 'quest.updated' && m.resourceId === S.cfg.entityId;
    if (mine && m.payload && S.data && m.payload.version != null && m.payload.version <= S.data.version) return;
    if (mine || m.type === 'reconnected') { S.stale = true; refreshWhenIdle(S); }
  }
  function idle(S) {
    var a = document.activeElement;
    return !S.give && !K.sheetIsOpen() && !S.el.querySelector('.dragging') &&
      !(a && S.el.contains(a) && (a.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName)));
  }
  function refreshWhenIdle(S) {
    clearTimeout(S.liveTimer);
    if (!S.stale || !S.data || !S.el.isConnected) return;
    if (!idle(S)) { S.liveTimer = setTimeout(function () { refreshWhenIdle(S); }, 1500); return; }
    S.stale = false;
    var gen = S.gen || 0;
    // Queued behind this viewer's own saves, so the answer is never older.
    S.saving = S.saving.then(function () { return K.api(S.cfg.endpoint); }).then(function (data) {
      if (gen !== (S.gen || 0) || !S.data) return;
      if (!idle(S)) { S.stale = true; refreshWhenIdle(S); return; }
      if (data && data.version != null && data.version === S.data.version) return;
      S.data = normalise(data); redrawBoard(S); redrawLedger(S);
    }).catch(function (err) { if (err.status === 404 || err.status === 403) S.el.hidden = true; });
  }

  // ---------- Saving ----------
  // Saves queue one behind another so the version always matches; a clash
  // with someone else's save reloads theirs and says so.
  function save(S, patch, redraw) {
    if (redraw === 'board' || redraw === 'both') redrawBoard(S);
    if (redraw === 'ledger' || redraw === 'both') redrawLedger(S);
    var gen = S.gen || 0;
    S.saving = S.saving.then(function () {
      // A clash reloaded the quest, so saves queued before it carry lists
      // built from the old copy; sending them would undo the other change.
      if (gen !== (S.gen || 0)) return;
      patch.version = S.data.version;
      return K.api(S.cfg.endpoint, 'PUT', patch, S.cfg.csrfToken).then(function (data) {
        S.data.version = data && data.version != null ? data.version : S.data.version;
      });
    }).catch(function (err) {
      S.gen = (S.gen || 0) + 1;
      K.toast(err.status === 409 ? 'Someone else changed this quest just now. Showing their version.' : err.message);
      return K.api(S.cfg.endpoint).then(function (data) {
        S.data = normalise(data); redrawBoard(S); redrawLedger(S);
      }).catch(function () {});
    });
    return S.saving;
  }

  // ---------- The ledger ----------
  function ledgerHTML(S) {
    var d = S.data, n = d.notice || {}, done = d.steps.filter(function (s) { return s.done; }).length;
    var head = '<select class="lsel" data-status aria-label="Status">' + STATUS.map(function (s) {
      return '<option value="' + s[0] + '"' + (d.status === s[0] ? ' selected' : '') + '>' + s[1] + '</option>';
    }).join('') + '</select>';
    var due = lrow('<span class="ico">' + I.cal + '</span><span class="grow" data-rename="due" data-max="160" title="Double-click to change">' +
      (n.due ? esc(n.due) : '<em class="ph">When is it due?</em>') + '</span>', false, ' data-list="notice" data-i="0"');
    var html = K.ledgerShell({
      title: 'Quest Ledger', head: head, fixed: due, tab: S.tab,
      tabs: [['steps', 'Steps ' + done + '/' + d.steps.length], ['rewards', 'Rewards'], ['foes', 'Foes'], ['links', 'Links'], ['look', I.gear, 'Looks']],
      panels: { steps: stepsHTML(S), rewards: rewardsHTML(S), foes: foesHTML(S), links: linksHTML(S), look: K.lookPanel(d.looks) }
    });
    return html.replace('<section class="ledger fold"', '<section class="ledger fold" data-ledger="quest"');
  }
  function stepsHTML(S) {
    return K.listHTML({ list: 'steps', items: S.data.steps, add: 'Add a step…', max: 200, row: function (s, i) {
      var id = S.id + 'st' + i;
      return '<input type="checkbox" id="' + id + '" data-step="' + i + '"' + (s.done ? ' checked' : '') + '>' +
        '<label class="grow" for="' + id + '" data-rename="text" title="Double-click to rename">' + esc(s.text) + '</label>' +
        '<button type="button" class="eye' + (s.shown ? ' on' : '') + '" data-eye="' + i + '" aria-pressed="' + !!s.shown + '" title="' +
        (s.shown ? 'Players can see this step' : 'Hidden from players') + '">' + (s.shown ? I.eye : I.eyeOff) + '</button>';
    } });
  }
  function rewardsHTML(S) {
    var d = S.data;
    var kinds = '<select class="wsel" data-add-kind aria-label="Kind of reward"><option value="money">Money</option><option value="item">Item</option><option value="other">Other</option></select>';
    return K.listHTML({ list: 'rewards', items: d.rewards, add: 'Add a reward…', max: 200, extra: kinds, row: function (r) {
      var k = REWARD_KINDS[r.kind] || REWARD_KINDS.other;
      var name = r.entityId ? '<a class="lk grow" href="' + entityUrl(S, r.entityId) + '" data-rename="text" title="Double-click to rename">' + esc(r.text) + '</a>'
        : '<span class="grow" data-rename="text" title="Double-click to rename">' + esc(r.text) + '</span>';
      return '<button type="button" class="ico kind" data-kind title="Kind: ' + k[1] + ' (click to change)">' + I[k[0]] + '</button>' + name +
        (r.kind === 'item' ? '<button type="button" class="lnk" data-link-item title="' + (r.entityId ? 'Change the Armory item' : 'Pick the Armory item') + '">' + I.link + '</button>' : '') +
        '<span class="sub">' + k[1] + '</span>';
    } }) + lrow(d.handedOut ? '<span class="handed grow">Handed out to the party</span>' : '<button type="button" class="wax-btn" data-give>Hand out rewards…</button>');
  }
  function foesHTML(S) {
    return K.listHTML({ list: 'foes', items: S.data.foes, add: 'Add a foe or challenge…', max: 200, row: function (f) {
      var name = f.entityId ? '<a class="lk grow" href="' + entityUrl(S, f.entityId) + '" data-rename="text" title="Double-click to rename">' + esc(f.text) + '</a>'
        : '<span class="grow" data-rename="text" title="Double-click to rename">' + esc(f.text) + '</span>';
      return '<span class="ico">' + I.sword + '</span>' + name +
        '<button type="button" class="lnk" data-link-foe title="' + (f.entityId ? 'Change the linked page' : 'Link a page') + '">' + I.link + '</button>' +
        '<span class="sub edit" data-rename="note" data-max="200" title="Double-click to add a note">' + (f.note ? esc(f.note) : '<em class="ph">note…</em>') + '</span>';
    } });
  }
  function linksHTML(S) {
    return K.listHTML({ list: 'links', items: S.data.links, row: function (l) {
      var isMap = l.kind === 'map', url = isMap ? S.campaignUrl + '/maps/' + encodeURIComponent(l.refId) : entityUrl(S, l.refId);
      return '<span class="ico">' + (isMap ? I.map : I.note) + '</span><a class="lk grow" href="' + url + '">' + esc(l.label || l.name || 'Untitled') + '</a>' +
        '<span class="sub">' + esc(isMap ? 'map' : (l.typeName || 'page').toLowerCase()) + '</span>';
    } }) + lrow('<button type="button" class="lbtn" data-add-link="page">+ Link a page</button>' +
      (S.data.mapsOn === false ? '' : '<button type="button" class="lbtn" data-add-link="map">+ Link a map</button>') +
      (S.data.mapId || S.data.mapsOn === false ? '' : '<span class="sub grow" style="text-align:right">The first map you link is pinned to the board.</span>'));
  }
  function entityUrl(S, id) { return S.campaignUrl + '/entities/' + encodeURIComponent(id); }
  function pickerUrl(S) { return S.campaignUrl + '/quests/picker'; }
  function newId() { return Math.random().toString(36).slice(2, 10); }

  // ---------- The DM's options on the board ----------
  function decorate(S) {
    if (!dm(S)) return;
    var L = S.data.layout;
    S.el.querySelectorAll('.cork [data-piece]').forEach(function (el) {
      var p = el.dataset.piece, hidden = L[p] && L[p].hidden;
      var acts = [['vis', hidden ? I.eyeOff : I.eye, hidden ? 'Show to players' : 'Hide from players']];
      if (p === 'notice') acts.push(['done', I.check, S.data.status === 'done' ? 'Lift the Done stamp' : 'Stamp it done']);
      if (p === 'tag') acts.push(['give', I.coin, 'Hand out rewards']);
      if (p === 'map') acts.push(['open', I.out, 'Open the map'], ['down', I.down, 'Take it down']);
      K.dmTab(el, acts);
    });
  }
  function dmAct(S, btn) {
    var el = btn.closest('[data-piece]'), piece = el.dataset.piece, act = btn.dataset.act, d = S.data;
    if (act === 'vis') {
      d.layout[piece].hidden = !d.layout[piece].hidden;
      K.toast(d.layout[piece].hidden ? 'Hidden from players. It stays on your board only.' : 'Players can see it again.');
      save(S, { layout: d.layout }, 'board');
    } else if (act === 'done') {
      d.status = d.status === 'done' ? 'active' : 'done';
      K.toast(d.status === 'done' ? 'Stamped done. Players see it right away.' : 'Stamp lifted.');
      save(S, { status: d.status }, 'both');
      var st = S.el.querySelector('.cork [data-piece="notice"] .stamp');
      if (st && !K.reduce()) st.animate([{ transform: 'rotate(-14deg) scale(2.2)', opacity: 0 }, { transform: 'rotate(-14deg) scale(1)', opacity: .8 }], { duration: 260, easing: 'cubic-bezier(.3,1.4,.5,1)' });
    } else if (act === 'give') {
      S.tab = 'rewards'; redrawLedger(S); K.openLedger(S.el.querySelector('.bw'));
      startGive(S);
    } else if (act === 'open') {
      if (d.map) Chronicle.go(S.campaignUrl + '/maps/' + encodeURIComponent(d.map.id));
    } else if (act === 'down') {
      K.askDown(btn.closest('.dmtab'), 'Take it down?');
    } else if (act === 'no') {
      decorate(S);
    } else if (act === 'yes') {
      K.takeDownAnim(el, function () {
        d.mapId = ''; d.mapName = ''; d.map = null;
        save(S, { mapId: '' }, 'both');
        K.toast('Map taken down. The map itself is kept.');
      });
    }
  }

  // ---------- Hand out rewards ----------
  // Each linked item reward goes through the Armory's own give, so the item
  // lands on the sheet, its history and the player's notification exactly as
  // a give from the Armory would. A money reward is split between the ticked
  // characters and each share paid onto their sheet through the Armory's pay,
  // which records it in their money history.
  var AV = ['#8a5a2b', '#3f6e5a', '#5a4a8a', '#8a3f4f', '#3f5f8a', '#6e6a3f'];
  function startGive(S) {
    var bw = S.el.querySelector('.bw');
    if (S.give) { K.openLedger(bw); return; }
    S.tab = 'rewards'; redrawLedger(S); K.openLedger(bw);
    S.give = { dirty: false, party: null, done: {} };
    K.api(pickerUrl(S) + '?kind=character', 'GET').then(function (party) {
      if (!S.give) return;
      S.give.party = party || [];
      var led = S.el.querySelector('.ledger'); if (!led) return;
      var tmp = document.createElement('div'); tmp.innerHTML = giveHTML(S, S.give.party);
      var g = tmp.firstChild; led.appendChild(g);
      requestAnimationFrame(function () { g.classList.add('on'); g.setAttribute('aria-hidden', 'false'); });
      updateShares(S);
    }).catch(function (err) { S.give = null; K.toast(err.message); });
  }
  function giveHTML(S, party) {
    var d = S.data, rows = '';
    var money = d.rewards.filter(function (r) { return r.kind === 'money'; });
    var items = d.rewards.filter(function (r) { return r.kind === 'item'; });
    if (!party.length) {
      rows += lrow('<span class="grow">This campaign has no characters to give to yet.</span>');
    }
    var chips = party.map(function (p, i) {
      var who = p.name + (p.player ? ' (' + p.player + ')' : '');
      return '<label class="cchip" title="' + esc(who) + '"><input type="checkbox" name="coin" value="' + i + '" checked aria-label="' + esc(who) + '">' +
        '<span class="av" style="background:' + AV[i % AV.length] + '">' + esc((p.name || '?').charAt(0).toUpperCase()) + '</span></label>';
    }).join('');
    money.forEach(function (r) {
      if (r.amount == null) {
        rows += lrow('<span class="grow">' + esc(r.text) + '</span><span class="sub">give it an amount to pay it</span>');
        return;
      }
      rows += lrow('<span class="grow">Split ' + esc(r.text) + ' between</span>');
      if (party.length) rows += lrow('<span class="chips" data-split="' + esc(r.id) + '">' + chips + '</span><span class="sp"></span><span class="sub" data-share="' + esc(r.id) + '"></span>');
    });
    var opts = '<option value="">Nobody</option>' + party.map(function (p, i) {
      return '<option value="' + i + '"' + (i === 0 ? ' selected' : '') + '>' + esc(p.name) + '</option>';
    }).join('');
    items.forEach(function (r) {
      if (!r.entityId) {
        rows += lrow('<span class="grow">' + esc(r.text) + '</span><span class="sub">pick its Armory item first</span>');
        return;
      }
      rows += lrow('<span class="grow">' + esc(r.text) + ' goes to</span><select class="wsel" data-give-to="' + esc(r.id) + '" aria-label="' + esc(r.text) + ' goes to">' + opts + '</select>');
    });
    rows += '<label class="row"><input type="checkbox" data-give-done' + (d.status === 'done' ? '' : ' checked') + '><span class="grow">Mark the quest done</span></label>';
    var title = (d.notice && d.notice.title) || '';
    return '<div class="give" aria-hidden="true" role="dialog" aria-label="Hand out rewards"><div class="wh"><b>Hand out rewards</b><span class="sp"></span><span class="only">' + esc(title) + '</span></div>' +
      '<div class="gb">' + rows + '</div><div class="gf"><button type="button" class="mini" data-give-cancel>Cancel</button><button type="button" class="mini pri" data-give-ok>Hand out</button></div></div>';
  }
  function rewardById(S, id) { return S.data.rewards.filter(function (x) { return x.id === id; })[0]; }
  // share is one character's part of a split, in hundredths, rounded down so
  // the party is never paid more than the reward.
  function share(amount, n) { return n ? Math.floor(Math.round(amount * 100) / n) : 0; }
  function fmtShare(h) { return h % 100 ? (h / 100).toFixed(2) : String(h / 100); }
  function updateShares(S) {
    var g = S.el.querySelector('.give'); if (!g) return;
    g.querySelectorAll('[data-share]').forEach(function (sp) {
      var r = rewardById(S, sp.dataset.share), n = 0;
      sp.closest('.row').querySelectorAll('input[name=coin]').forEach(function (b) { if (b.checked) n++; });
      if (!r || !n) { sp.textContent = 'nobody picked'; return; }
      var h = share(r.amount, n);
      sp.textContent = h ? fmtShare(h) + ' each' : 'too little to split';
    });
  }
  function closeGive(S) {
    var g = S.el.querySelector('.give'), w = S.el.querySelector('.ledger .warn');
    if (w) w.remove();
    S.give = null;
    if (!g) return;
    g.classList.remove('on'); g.setAttribute('aria-hidden', 'true');
    setTimeout(function () { g.remove(); }, K.reduce() ? 0 : 400);
  }
  function giveWarn(S) {
    var led = S.el.querySelector('.ledger'); if (!led) return;
    var w = led.querySelector('.warn');
    if (w) { w.querySelector('.k2').focus(); return; }
    w = document.createElement('div'); w.className = 'warn'; w.setAttribute('role', 'alertdialog');
    w.innerHTML = '<div>You picked who gets what but haven’t handed it out. Leave anyway?</div><div class="wr"><button type="button" data-w="discard">Discard</button><button type="button" class="k2" data-w="keep">Keep editing</button></div>';
    led.appendChild(w); w.querySelector('.k2').focus();
  }
  // A fold-away while the panel is open: ask first if something was picked.
  function giveBlocked(S, bw) {
    if (S.give && S.give.dirty) { giveWarn(S); return; }
    closeGive(S); K.closeLedger(bw);
  }
  function giveOne(S, itemId, characterId) {
    var fd = new FormData();
    fd.append('character_id', characterId); fd.append('item_id', itemId); fd.append('quantity', '1');
    return Chronicle.apiFetch(S.campaignUrl + '/armory/give', { method: 'POST', body: fd, csrfToken: S.cfg.csrfToken, headers: { 'HX-Request': 'true' } }).then(function (res) {
      if (res.ok) return;
      return res.json().catch(function () { return {}; }).then(function (b) { throw new Error(b.message || 'The Armory refused that give.'); });
    });
  }
  function payOne(S, characterId, hundredths, reason) {
    var fd = new FormData();
    fd.append('character_id', characterId); fd.append('amount', (hundredths / 100).toFixed(2)); fd.append('reason', reason);
    return K.api(S.campaignUrl + '/armory/pay', 'POST', fd, S.cfg.csrfToken);
  }
  // A whole hand-out is a list of steps, run one at a time. A step that went
  // through is remembered, so a retry after a refusal never pays or gives
  // anything twice.
  function handOut(S) {
    var g = S.el.querySelector('.give'); if (!g || !S.give || S.give.sending) return;
    var party = S.give.party || [], steps = [], done = S.give.done;
    var title = (S.data.notice && S.data.notice.title) || '';
    var reason = title ? 'as a reward for ' + title : 'as a quest reward';
    g.querySelectorAll('[data-split]').forEach(function (row) {
      var r = rewardById(S, row.dataset.split); if (!r || r.amount == null) return;
      var picked = Array.prototype.filter.call(row.querySelectorAll('input[name=coin]'), function (b) { return b.checked; });
      var h = share(r.amount, picked.length); if (!h) return;
      picked.forEach(function (b) {
        var p = party[+b.value]; if (!p) return;
        steps.push({ key: 'pay:' + r.id + ':' + p.id, label: fmtShare(h) + ' to ' + p.name, run: function () { return payOne(S, p.id, h, reason); } });
      });
    });
    g.querySelectorAll('[data-give-to]').forEach(function (sel) {
      if (sel.value === '') return;
      var r = rewardById(S, sel.dataset.giveTo), p = party[+sel.value];
      if (r && r.entityId && p) steps.push({ key: 'give:' + r.id + ':' + p.id, label: r.text + ' to ' + p.name, run: function () { return giveOne(S, r.entityId, p.id); } });
    });
    var markDone = !!(g.querySelector('[data-give-done]') || {}).checked;
    S.give.sending = true;
    var ok = g.querySelector('[data-give-ok]'); ok.disabled = true;
    var given = [];
    var chain = steps.reduce(function (p, st) {
      return p.then(function () {
        if (done[st.key]) return;
        return st.run().then(function () { done[st.key] = true; given.push(st); });
      });
    }, Promise.resolve());
    chain.then(function () {
      var patch = { handedOut: true }; S.data.handedOut = true;
      if (markDone) { patch.status = 'done'; S.data.status = 'done'; }
      closeGive(S);
      save(S, patch, markDone ? 'both' : 'ledger');
      K.toast(given.length ? 'Handed out: ' + given.map(function (x) { return x.label; }).join(', ') + '.' : 'Rewards marked as handed out.');
    }).catch(function (err) {
      if (S.give) S.give.sending = false;
      ok.disabled = false;
      K.toast((given.length ? 'Handed out ' + given.length + ', then stopped: ' : '') + err.message + ' Press Hand out again to finish; nothing is given twice.');
    });
  }

  // ---------- Events ----------
  function wire(S) {
    var el = S.el;
    K.wireDrag(el, {
      canMove: function () { return dm(S); },
      denied: function (item, narrow) { K.toast(narrow ? 'Move things on a wider screen.' : 'Only the DM can move things on this board.'); },
      moved: function (item, x, y) {
        var p = item.dataset.piece; S.data.layout[p].x = x; S.data.layout[p].y = y;
        save(S, { layout: S.data.layout }, '');
      },
      redraw: function () {}
    });
    var ledgers = K.wireLedgers(el, {
      busy: function () { return !!S.give; },
      blocked: function (bw) { giveBlocked(S, bw); },
      onAdd: function (list, text, row) { addTo(S, list, text, row); },
      onRemove: function (list, i) {
        var gone = S.data[list].splice(i, 1)[0];
        var patch = {}; patch[list] = S.data[list];
        if (list === 'links' && gone && gone.kind === 'map' && S.data.mapId === gone.refId) { patch.mapId = ''; S.data.mapId = ''; S.data.map = null; }
        K.toast('Removed “' + (gone.text || gone.label || gone.name || '') + '”.');
        save(S, patch, list === 'links' ? 'both' : 'ledger');
      },
      onRename: function (list, i, field, text) {
        if (list === 'notice') { saveNotice(S, field, text); redrawLedger(S); return; }
        var it = S.data[list][i]; if (!it) return;
        it[field] = text;
        var patch = {}; patch[list] = S.data[list];
        save(S, patch, field === 'note' ? 'ledger' : '');
      },
      onReorder: function (list, order) {
        var old = S.data[list];
        S.data[list] = order.map(function (i) { return old[i]; });
        var patch = {}; patch[list] = S.data[list];
        save(S, patch, 'ledger');
        if (list === 'steps') K.toast('Steps reordered. Players see the new order.');
      }
    });
    el.addEventListener('click', function (e) {
      var t = e.target;
      if (t.closest('.dmtab button')) { e.preventDefault(); e.stopPropagation(); dmAct(S, t.closest('.dmtab button')); return; }
      var tab = t.closest('[data-ltab]'); if (tab) S.tab = tab.dataset.ltab;
      var eye = t.closest('[data-eye]');
      if (eye) {
        var s = S.data.steps[+eye.dataset.eye]; s.shown = !s.shown;
        K.toast(s.shown ? 'Players can now read “' + s.text + '” on the notice.' : 'Hidden from players.');
        save(S, { steps: S.data.steps }, 'ledger');
        return;
      }
      var kind = t.closest('[data-kind]');
      if (kind) {
        var r = S.data.rewards[+kind.closest('[data-i]').dataset.i];
        r.kind = r.kind === 'money' ? 'item' : r.kind === 'item' ? 'other' : 'money';
        if (r.kind !== 'item') r.entityId = '';
        save(S, { rewards: S.data.rewards }, 'ledger');
        return;
      }
      if (t.closest('[data-link-item], [data-link-foe]')) {
        var isItem = !!t.closest('[data-link-item]'), idx = +t.closest('[data-i]').dataset.i, listName = isItem ? 'rewards' : 'foes';
        K.picker(el.querySelector('.board'), { url: pickerUrl(S), kind: 'page', title: isItem ? 'Pick the Armory item' : 'Link a page',
          onPick: function (row) {
            var it = S.data[listName][idx]; if (!it || !row) return;
            it.entityId = row.id; if (!it.text) it.text = row.name;
            var patch = {}; patch[listName] = S.data[listName]; save(S, patch, 'ledger');
          } });
        return;
      }
      var al = t.closest('[data-add-link]');
      if (al) {
        var mk = al.dataset.addLink;
        K.picker(el.querySelector('.board'), { url: pickerUrl(S), kind: mk, title: mk === 'map' ? 'Link a map' : 'Link a page', placeholder: mk === 'map' ? 'Search maps' : null,
          onPick: function (row) {
            if (!row) return;
            S.data.links.push({ id: newId(), kind: mk === 'map' ? 'map' : 'entity', refId: row.id, label: row.name, typeName: row.typeName });
            var patch = { links: S.data.links };
            if (mk === 'map' && !S.data.mapId) { patch.mapId = S.data.mapId = row.id; S.data.mapName = row.name; S.data.map = { id: row.id, name: row.name }; }
            save(S, patch, 'both');
            K.toast('Linked “' + row.name + '”.');
          } });
        return;
      }
      if (t.closest('[data-give]')) { startGive(S); return; }
      if (t.closest('[data-give-cancel]')) { closeGive(S); return; }
      if (t.closest('[data-give-ok]')) { handOut(S); return; }
      var w = t.closest('.warn [data-w]');
      if (w) { if (w.dataset.w === 'discard') closeGive(S); else w.closest('.warn').remove(); return; }
      if (t.closest('.ledger') || t.closest('.ribbon') || t.closest('.picker')) return;
      var piece = t.closest('.cork [data-piece]');
      if (!piece) return;
      if (piece.dataset.piece === 'map') { if (S.data.map) Chronicle.go(S.campaignUrl + '/maps/' + encodeURIComponent(S.data.map.id)); return; }
      if (S.data.notice) openNotice(S, piece);
    });
    el.addEventListener('dblclick', function (e) {
      var pl = e.target.closest('.bplate[data-edit]'); if (!pl || !dm(S)) return;
      if (!(S.data.notice && S.data.notice.plate)) pl.textContent = '';
      K.editInline(pl, { max: 60, allowEmpty: true, onSave: function (t) { saveNotice(S, 'plate', t); }, onCancel: function () { redrawBoard(S); } });
    });
    el.addEventListener('keydown', function (e) {
      var piece = e.target.closest && e.target.closest('.cork [data-piece]');
      if (piece && (e.key === 'Enter' || e.key === ' ') && e.target === piece) { e.preventDefault(); piece.click(); }
    });
    el.addEventListener('change', function (e) {
      var t = e.target;
      if (t.closest('.give')) {
        if (S.give) S.give.dirty = true;
        if (t.name === 'coin') updateShares(S);
        return;
      }
      if (t.matches('[data-status]')) {
        S.data.status = t.value; save(S, { status: t.value }, 'board');
        K.toast('Quest marked ' + t.options[t.selectedIndex].text.toLowerCase() + '.');
        return;
      }
      if (t.matches('[data-step]')) {
        S.data.steps[+t.dataset.step].done = t.checked;
        var st = el.querySelector('[data-ltab="steps"]');
        if (st) st.textContent = 'Steps ' + S.data.steps.filter(function (x) { return x.done; }).length + '/' + S.data.steps.length;
        save(S, { steps: S.data.steps }, '');
        return;
      }
      if (t.matches('select[data-lookfor]')) {
        S.data.looks[t.dataset.lookfor] = t.value;
        K.setLooks(el, S.data.looks);
        save(S, { looks: S.data.looks }, '');
      }
    });
    S.ledgers = ledgers;
  }

  function addTo(S, list, text, row) {
    var item = { id: newId(), text: text };
    if (list === 'steps') { item.done = false; item.shown = false; }
    if (list === 'rewards') {
      var k = row.querySelector('[data-add-kind]'); item.kind = k ? k.value : 'other';
      var m = item.kind === 'money' && text.replace(/,/g, '').match(/\d+(\.\d+)?/); item.amount = m ? +m[0] : null;
      item.entityId = '';
    }
    if (list === 'foes') { item.note = ''; item.entityId = ''; }
    S.data[list].push(item);
    var patch = {}; patch[list] = S.data[list];
    save(S, patch, 'ledger');
    var pn = S.el.querySelector('.ledger [data-lpanel="' + list + '"]');
    if (pn) {
      var rows = pn.querySelectorAll('.drow'), nr = rows[rows.length - 1];
      if (nr && !K.reduce()) nr.animate([{ opacity: 0, transform: 'translateY(-6px)' }, { opacity: 1, transform: 'none' }], { duration: 240, easing: 'cubic-bezier(.2,.8,.2,1)' });
      pn.scrollTop = pn.scrollHeight;
      var inp = pn.querySelector('[data-add-input]'); if (inp) inp.focus();
    }
    if (list === 'steps') K.toast('Added “' + text + '”. Hidden from players until you tap the eye.');
  }
})();
