/**
 * notice_boards.js: the Notice boards block on a place's page (a tavern, a
 * temple): several boards the viewer turns between like turning their head,
 * with quest notices, notes, pinned pages, maps and string. For the DM, the
 * Board ledger folds out underneath.
 *
 * Who may change a board is the board's own setting (only the DM, scribes
 * too, or everyone); players move and remove only what they pinned. The
 * server enforces all of it and sends a page the viewer may not see as a
 * face-down card with no name, so this file only mirrors the rules to keep
 * buttons honest.
 *
 * Mount: <div data-widget="notice_boards" data-endpoint data-campaign-id
 * data-entity-id (page mount only) data-csrf-token data-can-manage data-member-role>. Shared
 * parts: quest_board_kit.js; styles: static/css/quest_board.css.
 */
(function () {
  'use strict';
  var K = window.QuestBoardKit;
  if (!K || !window.Chronicle) return;
  var I = K.I, esc = K.esc, lrow = K.lrow;

  var WHO_TEXT = { dm: 'Only the DM changes this board', scribe: 'Scribes and the DM change this board', all: '' };
  var WHO_SET = { dm: 'Only me', scribe: 'Scribes and me', all: 'Everyone' };
  var PEOPLE = ['#0ea5e9', '#a855f7', '#ef4444', '#f59e0b', '#10b981', '#ec4899', '#14b8a6', '#6366f1'];

  Chronicle.register('notice_boards', {
    init: function (el, cfg) {
      var S = { el: el, cfg: cfg, data: null, cur: 0, tab: 'boards', stops: [], tying: null,
        campaignUrl: '/campaigns/' + encodeURIComponent(cfg.campaignId) };
      el.classList.add('qbk');
      el.innerHTML = '<div class="loading">Loading the notice boards…</div>';
      el._nb = S;
      K.api(cfg.endpoint).then(function (data) {
        S.seen = JSON.stringify(data); S.data = normalise(data); first(S);
      }).catch(function (err) {
        if (err.status === 404) { el.hidden = true; return; }
        el.innerHTML = '<div class="loading">' + esc(err.message) + '</div>';
      });
      S.stops.push(K.live(cfg.campaignId, function (m) { onLive(S, m); }));
    },
    destroy: function (el) {
      var S = el._nb; if (!S) return;
      clearTimeout(S.liveTimer);
      S.stops.forEach(function (f) { f(); });
      el._nb = null;
    }
  });

  function normalise(d) {
    d = d || {};
    d.boards = d.boards || [];
    d.looks = d.looks || { board: 'lit', ledger: 'lit' };
    d.me = d.me || {};
    d.boards.forEach(function (b) { b.items = b.items || []; });
    return d;
  }
  function isDm(S) { return !!(S.data.canManage || S.data.me.isDm); }
  function board(S) { return S.data.boards[S.cur]; }
  // Mirrors the server: the DM manages everything; scribes manage boards set
  // to "Scribes and me"; everyone else only what they pinned.
  function manages(S, b) { return isDm(S) || (b && b.who === 'scribe' && (S.data.me.role || 0) >= 2); }
  function canMoveItem(S, it) { var b = board(S); return manages(S, b) || (it && it.mine && b && b.canChange); }
  function base(S) { return S.cfg.endpoint; }
  function itemUrl(S, b, id) { return base(S) + '/boards/' + encodeURIComponent(b.id) + '/items' + (id ? '/' + encodeURIComponent(id) : ''); }
  function itemById(S, id) { var b = board(S); return b && b.items.filter(function (i) { return i.id === id; })[0]; }
  function colour(name) { var h = 0; String(name || '').split('').forEach(function (c) { h = (h * 31 + c.charCodeAt(0)) >>> 0; }); return PEOPLE[h % PEOPLE.length]; }

  // ---------- Rendering ----------
  function first(S) {
    var el = S.el;
    el.innerHTML = '<div class="bw wide">' + frameHTML(S) + (isDm(S) ? K.ribbon('Board ledger') + ledgerHTML(S) : '') + '</div>';
    K.setLooks(el, S.data.looks);
    var b = el.querySelector('.board');
    setTimeout(function () { b.classList.remove('enter'); }, 1600);
    S.stops.push(K.dust(el.querySelector('.dust')), K.restLoops(el));
    drawFace(S);
    wire(S);
    var onResize = function () { requestAnimationFrame(function () { strings(S); }); };
    window.addEventListener('resize', onResize);
    S.stops.push(function () { window.removeEventListener('resize', onResize); });
  }
  function frameHTML(S) {
    var plate = '<div class="bplate turn"><button type="button" class="arr" data-turn="-1" aria-label="Previous board">‹</button><span class="bnm"></span>' +
      '<button type="button" class="arr" data-turn="1" aria-label="Next board">›</button></div>';
    var tray = '<div class="tray"><span class="dots" aria-hidden="true"></span><span class="sp"></span><span class="lockline"></span>' +
      '<div class="addw"><button type="button" class="addb" aria-haspopup="true" aria-expanded="false">+ Add to board</button>' +
      '<div class="menu" hidden><button type="button" data-add="notice">Post a quest notice</button><button type="button" data-add="note">Note</button>' +
      '<button type="button" data-add="page">Pin a page…</button>' +
      // The maps addon off hides map pins; the server refuses them too.
      (S.data.mapsOn === false ? '' : '<button type="button" data-add="map">Pin a map…</button>') + '<button type="button" data-add="string">Tie string</button></div></div></div>';
    return K.boardFrame('wide multi', plate, '<div class="face"></div>', tray);
  }
  function drawFace(S) {
    var el = S.el, face = el.querySelector('.face'), b = board(S), boards = S.data.boards;
    if (!b) {
      face.innerHTML = '<div class="empty">' + (isDm(S) ? 'No boards yet. Add one in the Board ledger below.' : 'Nothing is posted here yet.') + '</div>';
      el.querySelector('.bnm').textContent = 'Notices';
      el.querySelector('.addw').hidden = true;
      el.querySelectorAll('[data-turn]').forEach(function (a) { a.hidden = true; });
      return;
    }
    var shown = b.items.filter(function (it) { return it.kind !== 'string'; });
    face.innerHTML = '<svg class="strings" aria-hidden="true"></svg>' + shown.map(function (it, i) { return itemHTML(S, it, i); }).join('') +
      (shown.length ? '' : '<div class="empty">' + (b.canChange ? 'Nothing pinned yet. Use “Add to board”.' : 'Nothing is posted here yet.') + '</div>');
    el.querySelector('.bnm').textContent = b.name;
    el.querySelector('.dots').innerHTML = boards.map(function (_, i) { return '<i' + (i === S.cur ? ' class="on"' : '') + '></i>'; }).join('');
    el.querySelectorAll('[data-turn]').forEach(function (a) { a.hidden = boards.length < 2; });
    el.querySelector('.lockline').innerHTML = isDm(S) || !WHO_TEXT[b.who] ? '' : I.lock + esc(WHO_TEXT[b.who]);
    el.querySelector('.addw').hidden = !b.canChange;
    el.querySelector('[data-add="notice"]').hidden = !manages(S, b);
    decorate(S);
    requestAnimationFrame(function () { strings(S); });
  }
  function strings(S) {
    var b = board(S), face = S.el.querySelector('.face'); if (!b || !face) return;
    K.drawStrings(face, b.items.filter(function (it) { return it.kind === 'string'; }).map(function (s) { return [s.from, s.to]; }));
  }
  function itemHTML(S, it, i) {
    var mine = canMoveItem(S, it), style = K.placeStyle(it, i);
    var head = '<div class="bi k-' + (it.concealed ? 'hidden' : it.kind) + (mine ? ' mine' : '') + (it.hidden ? ' veiled' : '') + '" data-id="' + esc(it.id) + '" tabindex="0" style="' + style + '"';
    var who = it.ownerName && !it.byDm ? '<span class="by" title="Pinned by ' + esc(it.ownerName) + '" style="background:' + colour(it.ownerName) + '">' + esc(it.ownerInitial || it.ownerName[0]) + '</span>' : '';
    if (it.kind === 'notice') {
      var stamp = it.status === 'done' ? 'Done' : it.status === 'active' ? 'Taken' : it.status === 'failed' ? 'Failed' : '';
      return K.noticeHTML({ kicker: it.kicker, title: it.title, blurb: it.blurb, reward: it.reward }, {
        cls: 'bi notice' + (mine ? ' mine' : '') + (it.hidden ? ' veiled' : ''), attrs: ' data-id="' + esc(it.id) + '" data-notice="' + esc(it.questId) + '"',
        style: style, stamp: stamp, brass: i % 3 === 1, torn: i % 3 === 2,
        left: it.status === 'done' || it.status === 'failed' ? '' : K.daysText(it.daysLeft), late: it.daysLeft < 0 });
    }
    if (it.kind === 'note') {
      return head + ' aria-label="Note' + (it.ownerName ? ' by ' + esc(it.ownerName) : '') + '"><span class="pin"></span><div class="sticky"><div class="nt"' +
        (it.mine && mine ? ' contenteditable="plaintext-only" spellcheck="false" aria-label="Note text"' : '') + '>' + esc(it.text) + '</div></div>' + who + '</div>';
    }
    if (it.kind === 'map') {
      return head + ' role="button" data-open="' + esc(it.url || S.campaignUrl + '/maps/' + encodeURIComponent(it.mapId)) + '" aria-label="Map: ' + esc(it.name) + '"><span class="pin brass"></span>' +
        '<div class="pp torn" style="padding:16px 6px 6px">' + K.mapSVG(it.name) + '</div>' + who + '</div>';
    }
    if (it.concealed) return head + ' aria-label="A page you can’t see"><span class="pin"></span><div class="cardback"><span>?</span></div>' + who + '</div>';
    return head + ' role="button" data-open="' + esc(it.url || S.campaignUrl + '/entities/' + encodeURIComponent(it.entityId)) + '" aria-label="Page: ' + esc(it.name) + '"><span class="pin"></span>' +
      '<div class="photo">' + K.photoHTML(it.name, it.imageUrl) + '<div class="cap">' + esc(it.name) + (it.typeName ? '<small>' + esc(it.typeName) + '</small>' : '') + '</div></div>' + who + '</div>';
  }

  // Like turning your head: the old board swings away to one side as the
  // next swings in from the other.
  function turn(S, dir) {
    var boards = S.data.boards; if (boards.length < 2) return;
    dir = dir > 0 ? 1 : -1;
    var face = S.el.querySelector('.face'), next = (S.cur + dir + boards.length) % boards.length;
    if (K.reduce()) { S.cur = next; drawFace(S); redrawLedger(S); return; }
    var ghost = face.cloneNode(true); ghost.classList.add('ghost'); face.parentNode.appendChild(ghost);
    S.cur = next; drawFace(S); redrawLedger(S);
    var o = { duration: 620, easing: 'cubic-bezier(.45,0,.15,1)' };
    ghost.animate([{ transform: 'none', opacity: 1 }, { transform: 'translateX(' + (-dir * 55) + '%) rotateY(' + (dir * 38) + 'deg)', opacity: 0 }], o).onfinish = function () { ghost.remove(); };
    face.animate([{ transform: 'translateX(' + (dir * 55) + '%) rotateY(' + (-dir * 38) + 'deg)', opacity: 0 }, { transform: 'none', opacity: 1 }], o).onfinish = function () { strings(S); };
    face.parentNode.animate([{ filter: 'none' }, { filter: 'brightness(.82)' }, { filter: 'none' }], o);
  }

  // ---------- The DM's (and an owner's) fold-out options ----------
  function decorate(S) {
    var b = board(S);
    S.el.querySelectorAll('.face [data-id]').forEach(function (el) {
      var it = itemById(S, el.dataset.id); if (!it) return;
      var acts = [], dm = isDm(S), man = manages(S, b);
      if (dm) acts.push(['vis', it.hidden ? I.eyeOff : I.eye, it.hidden ? 'Show to players' : 'Hide from players']);
      if (dm && it.kind === 'notice') acts.push(['done', I.check, it.status === 'done' ? 'Lift the Done stamp' : 'Stamp it done']);
      if ((it.kind === 'notice' || it.kind === 'page' || it.kind === 'map') && !it.concealed) acts.push(['open', I.out, it.kind === 'map' ? 'Open the map' : 'Open the page']);
      // A board locked to "Only me" leaves owners nothing to change.
      var own = it.mine && b && b.canChange;
      if (man || own) acts.push(['down', I.down, 'Take it down']);
      if (acts.length && (man || own || dm)) K.dmTab(el, acts);
    });
  }
  function act(S, btn) {
    var el = btn.closest('[data-id]'), it = itemById(S, el.dataset.id), b = board(S), a = btn.dataset.act;
    if (!it) return;
    if (a === 'vis') {
      it.hidden = !it.hidden; el.classList.toggle('veiled', it.hidden); decorate(S);
      K.toast(it.hidden ? 'Hidden from players. It stays on your board only.' : 'Players can see it again.');
      send(S, 'PATCH', itemUrl(S, b, it.id), { hidden: it.hidden });
    } else if (a === 'done') {
      var qurl = S.campaignUrl + '/quests/' + encodeURIComponent(it.questId), want = it.status === 'done' ? 'active' : 'done';
      K.api(qurl).then(function (q) {
        return K.api(qurl, 'PUT', { version: q.version, status: want }, S.cfg.csrfToken);
      }).then(function () {
        it.status = want; drawFace(S);
        K.toast(want === 'done' ? 'Stamped done. Players see it right away.' : 'Stamp lifted.');
      }).catch(function (err) { K.toast(err.message); });
    } else if (a === 'open') {
      if (it.kind === 'notice') Chronicle.go(S.campaignUrl + '/entities/' + encodeURIComponent(it.questId));
      else Chronicle.go(el.dataset.open);
    } else if (a === 'down') {
      K.askDown(btn.closest('.dmtab'), 'Take it down?');
    } else if (a === 'no') {
      decorate(S);
    } else if (a === 'yes') {
      K.takeDownAnim(el, function () {
        b.items = b.items.filter(function (x) { return x.id !== it.id && x.from !== it.id && x.to !== it.id; });
        el.remove(); strings(S); redrawLedger(S);
        K.toast(it.kind === 'note' ? 'Note taken down.' : 'Taken down. The page itself is kept.');
        send(S, 'DELETE', itemUrl(S, b, it.id));
      });
    }
  }

  // ---------- Live updates ----------
  // Fetch again when these boards change, or a quest pinned on them is saved
  // (its title or status shows on the notice). Never under a viewer who is
  // mid-change: wait until they finish so a drag or an edit is not lost.
  function onLive(S, m) {
    var cat = S.cfg.endpoint.indexOf('/category-boards/') >= 0, key = S.cfg.endpoint.split('/').pop();
    var hit = m.type === 'reconnected' ||
      (m.type === 'notice_boards.updated' && m.resourceId === key && (m.payload && m.payload.home) === (cat ? 'category' : 'page')) ||
      (m.type === 'quest.updated' && S.data && S.data.boards.some(function (b) { return b.items.some(function (it) { return it.questId === m.resourceId; }); }));
    if (hit) { S.stale = true; refreshWhenIdle(S); }
  }
  function idle(S) {
    var a = document.activeElement, pk = S.el.querySelector('.picker');
    return !S.tying && !K.sheetIsOpen() && !S.el.querySelector('.dragging') && !(pk && !pk.hidden) &&
      !(a && S.el.contains(a) && (a.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName)));
  }
  function refreshWhenIdle(S) {
    clearTimeout(S.liveTimer);
    if (!S.stale || !S.data || !S.el.isConnected) return;
    if (!idle(S)) { S.liveTimer = setTimeout(function () { refreshWhenIdle(S); }, 1500); return; }
    S.stale = false;
    K.api(S.cfg.endpoint).then(function (data) {
      if (!S.data) return;
      if (!idle(S)) { S.stale = true; refreshWhenIdle(S); return; }
      var raw = JSON.stringify(data);
      if (raw === S.seen) return;
      S.seen = raw; apply(S, data);
    }).catch(function (err) { if (err.status === 404 || err.status === 403) S.el.hidden = true; });
  }

  // ---------- Talking to the server ----------
  // A refused change puts the board back the way the server has it.
  function send(S, method, url, body) {
    return K.api(url, method, body, S.cfg.csrfToken).catch(function (err) {
      K.toast(err.message);
      return reload(S).then(function () { throw err; });
    });
  }
  function reload(S) {
    return K.api(S.cfg.endpoint).then(function (data) {
      S.seen = JSON.stringify(data); apply(S, data);
    }).catch(function () {});
  }
  // Shows a fresh copy, staying on the board the viewer was looking at.
  function apply(S, data) {
    var curId = board(S) && board(S).id;
    S.data = normalise(data);
    var i = S.data.boards.map(function (b) { return b.id; }).indexOf(curId);
    S.cur = i >= 0 ? i : Math.min(S.cur, Math.max(0, S.data.boards.length - 1));
    drawFace(S); redrawLedger(S);
  }
  function addItem(S, item) {
    var b = board(S);
    return send(S, 'POST', itemUrl(S, b), item).then(function (saved) {
      if (!saved || !saved.id) return reload(S);
      b.items.push(saved);
      var face = S.el.querySelector('.face');
      if (saved.kind === 'string') { strings(S); return saved; }
      var empty = face.querySelector('.empty'); if (empty) empty.remove();
      var tmp = document.createElement('div'); tmp.innerHTML = itemHTML(S, saved, 0);
      var nel = tmp.firstChild; face.appendChild(nel); decorate(S);
      K.dropIn(nel, saved.r || 0);
      redrawLedger(S);
      return saved;
    });
  }
  function spot() { return { x: 30 + Math.random() * 20, y: 18 + Math.random() * 16, r: Math.round((Math.random() * 6 - 3) * 10) / 10 }; }

  // ---------- The board ledger ----------
  function ledgerHTML(S) {
    var boards = S.data.boards, b = board(S);
    var list = K.listHTML({ list: 'boards', items: boards, add: 'Add a board…', max: 80, row: function (bd) {
      return '<span class="grow" data-rename="name" data-max="80" title="Double-click to rename">' + esc(bd.name) + '</span>' +
        '<select class="wsel" data-bwho="' + esc(bd.id) + '" aria-label="Who can change ' + esc(bd.name) + '">' +
        ['dm', 'scribe', 'all'].map(function (w) { return '<option value="' + w + '"' + (bd.who === w ? ' selected' : '') + '>' + WHO_SET[w] + '</option>'; }).join('') + '</select>';
    } });
    var notices = b ? b.items.filter(function (it) { return it.kind === 'notice'; }).map(function (it) {
      return lrow('<span class="ico">' + I.note + '</span><a class="lk grow" href="' + S.campaignUrl + '/entities/' + encodeURIComponent(it.questId) + '">' + esc(it.title) + '</a>' +
        '<span class="sub">' + esc(it.status === 'done' ? 'done' : it.status === 'active' ? 'taken' : it.status === 'failed' ? 'failed' : 'open') + (it.hidden ? ', hidden' : '') + '</span>');
    }).join('') : '';
    var theirs = b ? b.items.filter(function (it) { return !it.byDm && it.kind !== 'notice'; }).length : 0;
    var fixed = lrow('<span class="ico">' + I.note + '</span><span class="grow">' + (b ? 'Showing “' + esc(b.name) + '”' : 'No boards yet') + '</span><span class="sub">' +
      (b ? (S.cur + 1) + ' of ' + boards.length : '') + '</span>');
    return K.ledgerShell({ title: 'Board Ledger', fixed: fixed, tab: S.tab,
      tabs: [['boards', 'Boards'], ['quests', 'Quests'], ['players', 'Players'], ['look', I.gear, 'Looks']],
      panels: {
        boards: list,
        quests: notices || lrow('<span class="sub grow">No quest notices on this board.</span>'),
        players: lrow('<span class="grow">' + theirs + (theirs === 1 ? ' note or pin' : ' notes and pins') + ' from players</span>' +
          (theirs ? '<button type="button" class="lbtn" data-clear>Clear…</button>' : '')),
        look: K.lookPanel(S.data.looks)
      } }).replace('<section class="ledger fold"', '<section class="ledger fold" data-ledger="boards"');
  }
  function redrawLedger(S) {
    var old = S.el.querySelector('.ledger'); if (!old) return;
    var tmp = document.createElement('div'); tmp.innerHTML = ledgerHTML(S);
    var nl = tmp.firstChild; old.replaceWith(nl);
    K.syncPin(nl.closest('.bw'));
  }

  // ---------- Events ----------
  function wire(S) {
    var el = S.el;
    K.wireDrag(el, {
      canMove: function (item) { return canMoveItem(S, itemById(S, item.dataset.id)); },
      denied: function (item, narrow) {
        var it = itemById(S, item.dataset.id), b = board(S);
        K.toast(narrow ? 'Move things on a wider screen.' : !b.canChange ? WHO_TEXT[b.who] + '.' :
          it && it.ownerName ? it.ownerName + ' pinned this, so only they or the DM can move it.' : 'Only the DM can move notices.');
      },
      moved: function (item, x, y) {
        var it = itemById(S, item.dataset.id); if (!it) return;
        it.x = x; it.y = y; send(S, 'PATCH', itemUrl(S, board(S), it.id), { x: x, y: y });
      },
      redraw: function () { strings(S); }
    });
    K.wireLedgers(el, {
      busy: function () { return false; },
      onAdd: function (list, text) {
        if (list !== 'boards') return;
        send(S, 'POST', base(S) + '/boards', { name: text, who: 'dm' }).then(function (saved) {
          if (!saved || !saved.id) return reload(S);
          saved.items = saved.items || []; saved.canChange = true;
          S.data.boards.push(saved);
          var dir = S.data.boards.length - 1 - S.cur;
          if (dir) turn(S, dir); else { drawFace(S); redrawLedger(S); }
          K.toast('Board added. Players see it next time they turn.');
        }).catch(function () {});
      },
      onRemove: function (list, i) {
        var bd = S.data.boards[i]; if (!bd) return;
        S.data.boards.splice(i, 1);
        if (S.cur >= S.data.boards.length) S.cur = Math.max(0, S.data.boards.length - 1);
        drawFace(S); redrawLedger(S);
        K.toast('Board “' + bd.name + '” removed, with everything pinned to it.');
        send(S, 'DELETE', base(S) + '/boards/' + encodeURIComponent(bd.id)).catch(function () {});
      },
      onRename: function (list, i, field, text) {
        var bd = S.data.boards[i]; if (!bd) return;
        bd.name = text; if (i === S.cur) el.querySelector('.bnm').textContent = text;
        redrawLedger(S);
        send(S, 'PATCH', base(S) + '/boards/' + encodeURIComponent(bd.id), { name: text }).catch(function () {});
      },
      onReorder: function (list, order) {
        var old = S.data.boards, curId = board(S) && board(S).id;
        S.data.boards = order.map(function (i) { return old[i]; });
        S.cur = Math.max(0, S.data.boards.map(function (b) { return b.id; }).indexOf(curId));
        drawFace(S); redrawLedger(S);
        send(S, 'PUT', base(S) + '/order', { ids: S.data.boards.map(function (b) { return b.id; }) }).catch(function () {});
      }
    });

    el.addEventListener('click', function (e) {
      var t = e.target, b = board(S);
      var db = t.closest('.dmtab button'); if (db) { e.preventDefault(); e.stopPropagation(); act(S, db); return; }
      var tab = t.closest('[data-ltab]'); if (tab) { S.tab = tab.dataset.ltab; return; }
      var tr = t.closest('[data-turn]'); if (tr) { turn(S, +tr.dataset.turn); return; }
      var menu = el.querySelector('.menu'), addB = t.closest('.addb');
      if (addB) { menu.hidden = !menu.hidden; addB.setAttribute('aria-expanded', String(!menu.hidden)); return; }
      if (menu && !menu.hidden && !t.closest('.menu')) { menu.hidden = true; el.querySelector('.addb').setAttribute('aria-expanded', 'false'); }
      var add = t.closest('[data-add]');
      if (add) { menu.hidden = true; startAdd(S, add.dataset.add); return; }
      var clr = t.closest('[data-clear]');
      if (clr) {
        if (!clr.classList.contains('sure')) { clr.classList.add('sure'); clr.textContent = 'Clear them all?'; return; }
        b.items = b.items.filter(function (it) { return it.byDm || it.kind === 'notice'; });
        drawFace(S); redrawLedger(S); K.toast('Players’ notes and pins cleared from this board.');
        send(S, 'DELETE', base(S) + '/boards/' + encodeURIComponent(b.id) + '/player-items').catch(function () {});
        return;
      }
      if (S.tying) { tie(S, t); e.preventDefault(); return; }
      if (t.closest('.ledger, .ribbon, .picker, [contenteditable]')) return;
      var item = t.closest('.face [data-id]'); if (!item) return;
      var it = itemById(S, item.dataset.id); if (!it) return;
      if (it.concealed) { K.toast((it.ownerName || 'Someone') + ' pinned a page you can’t see, so it stays face down for you.'); return; }
      if (it.kind === 'notice') { readNotice(S, item, it); return; }
      if (item.dataset.open) Chronicle.go(item.dataset.open);
    });
    el.addEventListener('keydown', function (e) {
      var item = e.target.closest && e.target.closest('.face [data-id]');
      if (item && e.target === item && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); item.click(); }
      if (e.key === 'Escape' && S.tying) endTie(S);
    });
    // A note saves when you leave it; one left empty comes down again.
    el.addEventListener('focusout', function (e) {
      var nt = e.target.closest && e.target.closest('.face .nt[contenteditable]'); if (!nt) return;
      var item = nt.closest('[data-id]'), it = itemById(S, item.dataset.id), text = nt.textContent.trim().slice(0, 400);
      if (item.dataset.draft) {
        if (!text) { item.remove(); return; }
        var d = JSON.parse(item.dataset.draft); item.remove();
        addItem(S, { kind: 'note', text: text, x: d.x, y: d.y, w: 14, r: d.r }).catch(function () {});
        return;
      }
      if (!it || text === it.text) return;
      if (!text) { nt.textContent = it.text; K.toast('A note needs some words. Take it down instead to remove it.'); return; }
      it.text = text; send(S, 'PATCH', itemUrl(S, board(S), it.id), { text: text }).catch(function () {});
    });
    el.addEventListener('change', function (e) {
      var t = e.target;
      if (t.matches('[data-bwho]')) {
        var bd = S.data.boards.filter(function (x) { return x.id === t.dataset.bwho; })[0]; if (!bd) return;
        bd.who = t.value; drawFace(S);
        K.toast({ dm: '“' + bd.name + '”: only you can change it.', scribe: '“' + bd.name + '”: scribes can change it too.', all: '“' + bd.name + '”: everyone can add. Players move only their own things.' }[t.value]);
        send(S, 'PATCH', base(S) + '/boards/' + encodeURIComponent(bd.id), { who: t.value }).catch(function () {});
        return;
      }
      if (t.matches('select[data-lookfor]')) {
        S.data.looks[t.dataset.lookfor] = t.value; K.setLooks(el, S.data.looks);
        requestAnimationFrame(function () { strings(S); });
        send(S, 'PUT', base(S) + '/looks', S.data.looks).catch(function () {});
      }
    });
  }

  function startAdd(S, kind) {
    var host = S.el.querySelector('.board'), p = spot();
    if (kind === 'note') {
      // A draft note goes up at once; it is saved when you leave it with words on it.
      var face = S.el.querySelector('.face'), empty = face.querySelector('.empty'); if (empty) empty.remove();
      var tmp = document.createElement('div');
      tmp.innerHTML = itemHTML(S, { id: 'draft', kind: 'note', text: '', mine: true, x: p.x, y: p.y, w: 14, r: p.r }, 0);
      var nel = tmp.firstChild; nel.dataset.draft = JSON.stringify(p); face.appendChild(nel); K.dropIn(nel, p.r);
      setTimeout(function () { var nt = nel.querySelector('.nt'); if (nt) nt.focus(); }, 350);
      return;
    }
    if (kind === 'string') {
      S.tying = { from: null }; S.el.querySelector('.face').classList.add('tying');
      K.toast('Click two pins to tie string between them. Tie the same two again to untie.');
      return;
    }
    var picks = { page: ['page', 'Pin a page', null], map: ['map', 'Pin a map', 'Search maps'], notice: ['quest', 'Post a quest notice', 'Search quest pages'] }[kind];
    K.picker(host, { url: S.campaignUrl + '/quests/picker', kind: picks[0], title: picks[1], placeholder: picks[2], onPick: function (row) {
      if (!row) return;
      var item = { kind: kind, refId: row.id, x: p.x, y: p.y, w: kind === 'notice' ? 22 : kind === 'map' ? 20 : 13, r: p.r };
      addItem(S, item).then(function () { K.toast(kind === 'notice' ? 'Notice posted.' : 'Pinned “' + row.name + '”.'); }).catch(function () {});
    } });
  }
  function tie(S, target) {
    var bi = target.closest('.face [data-id]');
    if (!bi) { endTie(S); return; }
    if (!S.tying.from) { S.tying.from = bi.dataset.id; bi.classList.add('from'); return; }
    var a = S.tying.from, z = bi.dataset.id, b = board(S);
    endTie(S);
    if (a === z) return;
    var existing = b.items.filter(function (s) { return s.kind === 'string' && ((s.from === a && s.to === z) || (s.from === z && s.to === a)); })[0];
    if (existing) {
      if (!(existing.mine || manages(S, b))) { K.toast('Someone else tied that string.'); return; }
      b.items = b.items.filter(function (s) { return s !== existing; }); strings(S);
      K.toast('String untied.');
      send(S, 'DELETE', itemUrl(S, b, existing.id)).catch(function () {});
      return;
    }
    addItem(S, { kind: 'string', from: a, to: z, x: 0, y: 0, w: 5, r: 0 }).catch(function () {});
  }
  function endTie(S) {
    S.tying = null;
    var face = S.el.querySelector('.face'); face.classList.remove('tying');
    var f = face.querySelector('.from'); if (f) f.classList.remove('from');
  }

  // Reading a posted notice: the quest's own sheet, as this viewer may see it.
  function readNotice(S, el, it) {
    K.api(S.campaignUrl + '/quests/' + encodeURIComponent(it.questId)).then(function (q) {
      K.openSheet(el, sheetHTML(S, q, it), S.data.looks.board, null);
    }).catch(function (err) { K.toast(err.message); });
  }
  function sheetHTML(S, q, it) {
    var n = q.notice || { title: it.title, kicker: it.kicker, body: [] }, dm = !!q.canEdit;
    var steps = (q.steps || []).filter(function (s) { return dm || s.shown !== false; }).map(function (s) {
      return '<li class="' + (s.done ? 'done' : '') + '"><span class="bx"></span><span>' + esc(s.text) + (dm && !s.shown ? ' <em style="font-size:.85em">(hidden from players)</em>' : '') + '</span></li>';
    });
    if (!dm && q.hiddenSteps) steps.splice(Math.max(0, steps.length - 1), 0, '<li class="unknown"><span class="bx"></span><span>Something more, not yet known…</span></li>');
    return (n.kicker ? '<div class="k">' + esc(n.kicker) + '</div>' : '') + '<h2 id="qbk-sheet-title">' + esc(n.title) + '</h2>' +
      (n.postedBy ? '<div class="by">' + esc(n.postedBy) + '</div>' : '') + (n.body || []).map(function (p) { return '<p>' + esc(p) + '</p>'; }).join('') +
      (n.reward ? '<h4>Reward</h4><p style="margin:0">' + esc(n.reward) + '</p>' : '') +
      (steps.length ? '<h4>What must be done</h4><ul class="steps">' + steps.join('') + '</ul>' : '') +
      '<div class="sfoot"><span class="due">' + (q.due ? 'Due ' + esc(q.due.label) + ' <span class="dl">· ' + esc(K.daysText(q.due.daysLeft)) + '</span>' : esc(n.due || '')) + '</span><span style="display:flex;gap:10px;align-items:center">' +
      '<a class="pbtn" href="' + S.campaignUrl + '/entities/' + encodeURIComponent(it.questId) + '">Open the quest</a>' +
      '<button type="button" class="pbtn" data-close>Pin it back</button><span class="seal" aria-hidden="true">' + esc(K.sealLetter(n.title)) + '</span></span></div>';
  }
})();
