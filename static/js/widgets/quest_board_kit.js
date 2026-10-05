/**
 * quest_board_kit.js: the pieces the quest board (quest_board.js) and the
 * notice boards (notice_boards.js) share: icons, the overlay layer with its
 * toast and reading sheet, the fold-out ledger book (tabs, editable lists,
 * looks), dragging things on the cork, string, the DM's fold-out options and
 * the looping details that rest with MotionRest.
 *
 * Styles live in static/css/quest_board.css, scoped under .qbk. Positions on
 * a board are percentages of the cork, so a board looks the same at any width.
 * Nothing here talks to the server; each widget passes callbacks.
 */
(function () {
  'use strict';
  if (window.QuestBoardKit) return;

  var reduce = function () {
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches ||
      document.documentElement.getAttribute('data-view-motion') === 'calm';
  };

  var I = {
    eye: '<svg viewBox="0 0 24 24"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/></svg>',
    eyeOff: '<svg viewBox="0 0 24 24"><path d="M3 3l18 18M10.6 5.1A10 10 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3.2 4M6.6 6.6C3.8 8.4 2 12 2 12s3.5 7 10 7a9.6 9.6 0 0 0 5.4-1.6"/></svg>',
    lock: '<svg viewBox="0 0 24 24"><rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/></svg>',
    map: '<svg viewBox="0 0 24 24"><path d="M9 4l-6 2v14l6-2 6 2 6-2V4l-6 2z"/></svg>',
    pin: '<svg viewBox="0 0 24 24"><path d="M12 21s-7-6.5-7-12a7 7 0 0 1 14 0c0 5.5-7 12-7 12z"/></svg>',
    npc: '<svg viewBox="0 0 24 24"><circle cx="12" cy="8" r="4"/><path d="M4 21c1-4 4-6 8-6s7 2 8 6"/></svg>',
    note: '<svg viewBox="0 0 24 24"><path d="M5 4h11l3 3v13H5z"/><path d="M9 10h6M9 14h6"/></svg>',
    sword: '<svg viewBox="0 0 24 24"><path d="M14 4h6v6L9 21l-6-6zM5 13l6 6"/></svg>',
    coin: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/><path d="M12 8v8"/></svg>',
    item: '<svg viewBox="0 0 24 24"><path d="M9 3h6v4l3 3v11H6V10l3-3z"/></svg>',
    star: '<svg viewBox="0 0 24 24"><path d="M12 3l2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1L3.2 9.5l6.1-.9z"/></svg>',
    cal: '<svg viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18"/></svg>',
    check: '<svg viewBox="0 0 24 24"><path d="M5 12l5 5 9-10"/></svg>',
    out: '<svg viewBox="0 0 24 24"><path d="M14 4h6v6M20 4l-9 9M18 14v6H4V6h6"/></svg>',
    down: '<svg viewBox="0 0 24 24"><path d="M12 4v11M7 10l5 5 5-5M5 20h14"/></svg>',
    link: '<svg viewBox="0 0 24 24"><path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/></svg>',
    gear: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1L7 17M17 7l2.1-2.1"/></svg>'
  };

  var LOOKS = [['lit', 'Lamp-lit'], ['plain', 'Plain'], ['parchment', 'Parchment'], ['midnight', 'Midnight']];

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  // Server error bodies carry a human message; anything else gets a plain one.
  function apiCall(url, method, body, csrf) {
    return Chronicle.apiFetch(url, { method: method || 'GET', body: body, csrfToken: csrf }).then(function (res) {
      return res.text().then(function (txt) {
        var data = null;
        try { data = txt ? JSON.parse(txt) : null; } catch (e) { data = null; }
        if (!res.ok) {
          var err = new Error((data && (data.message || data.error)) || 'Something went wrong. Try again.');
          err.status = res.status;
          throw err;
        }
        return data;
      });
    });
  }

  // ---------- The overlay layer: toast, reading sheet, flying paper ----------
  // One layer for the page, so a sheet opened from any board lies over everything.
  var layer = null;
  function getLayer() {
    if (layer && layer.isConnected) return layer;
    layer = document.createElement('div');
    layer.className = 'qbk qbk-layer';
    layer.innerHTML = '<div class="reader" hidden><article class="sheet" tabindex="-1" role="dialog" aria-modal="true" aria-labelledby="qbk-sheet-title"></article></div>' +
      '<div class="toast" role="status" aria-live="polite"></div>';
    document.body.appendChild(layer);
    return layer;
  }
  function toast(msg) {
    var t = getLayer().querySelector('.toast');
    t.textContent = msg; t.classList.add('on');
    clearTimeout(t._h); t._h = setTimeout(function () { t.classList.remove('on'); }, 2600);
  }

  function rectOf(el) { var r = el.getBoundingClientRect(); return { left: r.left, top: r.top, width: r.width, height: r.height }; }
  function px(r) { return { left: r.left + 'px', top: r.top + 'px', width: r.width + 'px', height: r.height + 'px' }; }

  // A notice peels off the cork and grows into the reading sheet: the pin pops
  // out and drops away, and the paper flies to the middle of the screen.
  var open = null;
  function openSheet(note, html, look, onClose) {
    if (open) return null;
    var L = getLayer(), reader = L.querySelector('.reader'), sheet = L.querySelector('.sheet');
    L.setAttribute('data-bl', look || 'lit');
    sheet.innerHTML = html;
    sheet.classList.remove('show');
    reader.hidden = false;
    var pp = note.querySelector('.pp, .tg') || note, from = rectOf(pp), to = rectOf(sheet);
    var r = parseFloat(getComputedStyle(note).getPropertyValue('--r')) || 0;
    open = { note: note, r: r, onClose: onClose };
    requestAnimationFrame(function () { reader.classList.add('on'); });
    if (reduce()) { note.classList.add('lift'); sheet.classList.add('show'); sheet.focus(); return sheet; }
    var pin = note.querySelector('.pin');
    if (pin) {
      var pr = rectOf(pin), lp = document.createElement('span');
      lp.className = 'loose-pin'; if (pin.classList.contains('brass')) lp.style.background = getComputedStyle(pin).backgroundImage;
      lp.style.left = pr.left + 'px'; lp.style.top = pr.top + 'px'; L.appendChild(lp);
      lp.animate([{ transform: 'none' }, { transform: 'translate(8px,-22px) rotate(40deg)', offset: .3 }, { transform: 'translate(26px,160px) rotate(200deg)', opacity: 0 }],
        { duration: 750, easing: 'cubic-bezier(.3,0,.7,1)' }).onfinish = function () { lp.remove(); };
    }
    var fl = document.createElement('div'); fl.className = 'flyer';
    fl.innerHTML = '<div class="pp"></div>'; Object.assign(fl.style, px(from)); L.appendChild(fl);
    var paper = fl.firstChild; paper.innerHTML = pp.innerHTML; paper.style.overflow = 'hidden';
    paper.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 260, delay: 140, fill: 'forwards' });
    note.classList.add('lift');
    var lifted = { left: from.left - 6 + 'px', top: from.top - 18 + 'px', width: from.width + 12 + 'px', height: from.height + 12 + 'px', transform: 'rotate(' + (r * 2.4) + 'deg) perspective(600px) rotateX(16deg)', offset: .28 };
    fl.animate([Object.assign(px(from), { transform: 'rotate(' + r + 'deg)' }), lifted, Object.assign(px(to), { transform: 'none' })],
      { duration: 820, easing: 'cubic-bezier(.5,0,.15,1)', fill: 'forwards' }).onfinish = function () {
        sheet.classList.add('show'); requestAnimationFrame(function () { fl.remove(); }); sheet.focus();
      };
    paper.animate([{ background: getComputedStyle(pp).backgroundColor }, { background: '#ead9b2' }], { duration: 820, fill: 'forwards' });
    return sheet;
  }
  function closeSheet() {
    if (!open || open.closing) return;
    open.closing = true;
    var L = getLayer(), reader = L.querySelector('.reader'), sheet = L.querySelector('.sheet'), note = open.note, r = open.r, cb = open.onClose;
    var done = function () {
      reader.hidden = true; note.classList.remove('lift'); open = null;
      if (note.isConnected) note.focus({ preventScroll: true });
      if (cb) cb();
    };
    reader.classList.remove('on');
    if (reduce() || !note.isConnected) { done(); return; }
    var from = rectOf(sheet), to = rectOf(note.querySelector('.pp, .tg') || note);
    sheet.classList.remove('show');
    var fl = document.createElement('div'); fl.className = 'flyer'; fl.innerHTML = '<div class="pp" style="height:100%"></div>';
    Object.assign(fl.style, px(from)); L.appendChild(fl);
    fl.animate([Object.assign(px(from), { transform: 'none' }), { left: to.left - 6 + 'px', top: to.top - 20 + 'px', width: to.width + 10 + 'px', height: to.height + 10 + 'px', transform: 'rotate(' + (r * 2) + 'deg)', offset: .78 }, Object.assign(px(to), { transform: 'rotate(' + r + 'deg)' })],
      { duration: 700, easing: 'cubic-bezier(.5,0,.2,1)', fill: 'forwards' }).onfinish = function () {
        done(); fl.remove();
        var pin = note.querySelector('.pin');
        if (pin) pin.animate([{ transform: 'translateY(-10px) scale(1.8)' }, { transform: 'none' }], { duration: 380, easing: 'cubic-bezier(.2,1.4,.4,1)' });
        note.animate([{ transform: 'rotate(' + r + 'deg) scale(1.03)' }, { transform: 'rotate(' + r + 'deg)' }], { duration: 300 });
      };
  }
  function sheetIsOpen() { return !!open; }
  document.addEventListener('click', function (e) {
    if (!open || !layer) return;
    var reader = layer.querySelector('.reader');
    if (!reader.contains(e.target)) return;
    if (e.target.closest('[data-close]') || !e.target.closest('.sheet')) closeSheet();
  });
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && open && !e.target.closest('[contenteditable]')) closeSheet();
  });

  // Double-click (or Enter on a focused line) turns a line into an editor;
  // Enter or leaving saves, Escape puts the old text back.
  function editInline(el, opts) {
    if (el.isContentEditable) return;
    var was = el.textContent;
    el.contentEditable = 'plaintext-only';
    if (el.contentEditable !== 'plaintext-only') el.contentEditable = 'true';
    el.focus();
    var range = document.createRange(); range.selectNodeContents(el);
    var sel = window.getSelection(); sel.removeAllRanges(); sel.addRange(range);
    function finish(save) {
      el.removeEventListener('keydown', key); el.removeEventListener('blur', blur);
      el.removeAttribute('contenteditable');
      var t = el.textContent.replace(/\s+/g, ' ').trim().slice(0, opts.max || 200);
      if (save && t !== was.trim() && (t || opts.allowEmpty)) { el.textContent = t; opts.onSave(t); }
      else { el.textContent = was; if (opts.onCancel) opts.onCancel(); }
    }
    function key(k) {
      if (k.key === 'Enter' && !(opts.multiline && k.shiftKey)) { k.preventDefault(); el.blur(); }
      else if (k.key === 'Escape') { k.preventDefault(); k.stopPropagation(); finish(false); }
    }
    function blur() { finish(true); }
    el.addEventListener('keydown', key); el.addEventListener('blur', blur);
  }

  // A remove button asks once ("Remove?") before it acts, so a stray click
  // never loses anything.
  function armRemove(btn, label, onConfirm) {
    if (!btn.classList.contains('arm')) {
      btn.classList.add('arm'); btn._was = btn.innerHTML; btn.textContent = label || 'Remove?';
      clearTimeout(btn._h); btn._h = setTimeout(function () { btn.classList.remove('arm'); btn.innerHTML = btn._was; }, 3000);
      return;
    }
    clearTimeout(btn._h);
    var row = btn.closest('.row');
    if (!row || reduce()) { onConfirm(); return; }
    row.style.overflow = 'hidden';
    row.animate([{ height: row.offsetHeight + 'px', opacity: 1 }, { height: '0px', opacity: 0, transform: 'translateX(18px)' }],
      { duration: 220, easing: 'cubic-bezier(.4,0,.2,1)' }).onfinish = onConfirm;
  }

  // ---------- The ledger book ----------
  // A header, one fixed line, a row of index tabs and one short page at a time.
  function ribbon(label) {
    return '<button type="button" class="ribbon" aria-expanded="false" title="Click to open or close"><span>' + esc(label) +
      '</span><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 9l6 6 6-6"/></svg></button>';
  }
  function pinBtn() {
    return '<button type="button" class="lpin" title="Keep it open" aria-pressed="false"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 3h6l-1 6 4 4H6l4-4zM12 13v8"/></svg></button>';
  }
  function lrow(h, drag, attrs) {
    return '<div class="row' + (drag ? ' drow' : '') + '"' + (attrs || '') + '>' +
      (drag ? '<span class="grip" title="Drag to reorder" aria-hidden="true"></span>' : '') + h + '</div>';
  }
  function ledgerShell(o) {
    return '<section class="ledger fold" aria-label="' + esc(o.title) + '">' +
      '<header class="lhead"><b>' + esc(o.title) + '</b><span class="only" title="Only you see this">' + I.lock + '</span><span class="sp"></span>' + (o.head || '') + pinBtn() + '</header>' +
      '<div class="ltabs" role="tablist">' + o.tabs.map(function (t) {
        var on = t[0] === o.tab;
        return '<button type="button" class="ltab' + (on ? ' on' : '') + '" role="tab" aria-selected="' + on + '" data-ltab="' + t[0] + '"' + (t[2] ? ' title="' + esc(t[2]) + '" aria-label="' + esc(t[2]) + '"' : '') + '>' + t[1] + '</button>';
      }).join('') + '</div>' +
      '<div class="lpage">' + (o.fixed || '') + o.tabs.map(function (t) {
        return '<div class="lpanel" role="tabpanel" data-lpanel="' + t[0] + '"' + (t[0] === o.tab ? '' : ' hidden') + '>' + (o.panels[t[0]] || '') + '</div>';
      }).join('') + '</div>' + (o.after || '') + '</section>';
  }
  function lookOpts(cur) {
    return LOOKS.map(function (o) { return '<option value="' + o[0] + '"' + (o[0] === cur ? ' selected' : '') + '>' + o[1] + '</option>'; }).join('');
  }
  function lookPanel(looks) {
    return lrow('<span class="grow">Board look</span><select class="wsel" data-lookfor="board" aria-label="Board look">' + lookOpts(looks.board) + '</select>') +
      lrow('<span class="grow">Ledger look</span><select class="wsel" data-lookfor="ledger" aria-label="Ledger look">' + lookOpts(looks.ledger) + '</select>');
  }
  function setLooks(root, looks) {
    root.setAttribute('data-bl', looks.board || 'lit');
    root.setAttribute('data-ll', looks.ledger || 'lit');
    root.querySelectorAll('select[data-lookfor]').forEach(function (s) { s.value = s.dataset.lookfor === 'board' ? looks.board : looks.ledger; });
  }

  // An editable list on a ledger page: every row can be renamed (double-click
  // a [data-rename] part), removed (× then "Remove?") and dragged by its grip;
  // the last line adds a new one. o: {list, items, row(item, i), add, extra}.
  function listHTML(o) {
    var rows = o.items.map(function (it, i) {
      return lrow(o.row(it, i) + '<button type="button" class="xdel" data-del title="Remove" aria-label="Remove">×</button>', true, ' data-list="' + o.list + '" data-i="' + i + '"');
    }).join('');
    if (!o.add) return rows;
    return rows + '<div class="row addrow" data-list="' + o.list + '"><span class="plus" aria-hidden="true">+</span>' +
      '<input class="addstep grow" data-add-input placeholder="' + esc(o.add) + '" aria-label="' + esc(o.add) + '" maxlength="' + (o.max || 200) + '" autocomplete="off">' +
      (o.extra || '') + '<span class="sub">Enter</span></div>';
  }

  // Wires one ledger-carrying widget: tabs, the list controls, row dragging and
  // the fold-out. h: {onAdd(list, text, row), onRemove(list, i), onRename(list,
  // i, field, text), onReorder(list, order), busy(), blocked(bw)}:
  // while busy() is true a fold-away calls blocked instead. Called once per root;
  // panels can be re-rendered freely afterwards.
  function wireLedgers(root, h) {
    var tabs = {};
    root.addEventListener('click', function (e) {
      var tb = e.target.closest('[data-ltab]');
      if (tb && root.contains(tb)) {
        var led = tb.closest('.ledger'), name = tb.dataset.ltab;
        tabs[led.dataset.ledger || ''] = name;
        led.querySelectorAll('[data-ltab]').forEach(function (b) { var on = b === tb; b.classList.toggle('on', on); b.setAttribute('aria-selected', String(on)); });
        led.querySelectorAll('[data-lpanel]').forEach(function (pn) {
          var show = pn.dataset.lpanel === name; pn.hidden = !show;
          if (show && !reduce()) pn.animate([{ opacity: 0, transform: 'translateX(12px)' }, { opacity: 1, transform: 'none' }], { duration: 260, easing: 'cubic-bezier(.2,.8,.2,1)' });
        });
        return;
      }
      var x = e.target.closest('.ledger [data-del]');
      if (x) {
        e.preventDefault();
        var row = x.closest('[data-list]');
        armRemove(x, null, function () { h.onRemove(row.dataset.list, +row.dataset.i); });
        return;
      }
      if (e.target.closest('.ledger [contenteditable]')) e.preventDefault();
    });
    root.addEventListener('dblclick', function (e) {
      var el = e.target.closest('.ledger [data-rename]'); if (!el) return;
      e.preventDefault();
      var row = el.closest('[data-list]');
      var ph = el.querySelector('.ph'), phHTML = ph ? el.innerHTML : null;
      if (ph) el.textContent = '';
      editInline(el, { onCancel: function () { if (phHTML) el.innerHTML = phHTML; }, max: +(el.dataset.max || 200), allowEmpty: el.dataset.rename !== 'text', onSave: function (t) { h.onRename(row.dataset.list, +row.dataset.i, el.dataset.rename, t); } });
    });
    root.addEventListener('keydown', function (e) {
      var inp = e.target.closest && e.target.closest('.ledger [data-add-input]');
      if (inp && e.key === 'Enter') {
        var t = inp.value.trim(); if (!t) return;
        e.preventDefault();
        h.onAdd(inp.closest('[data-list]').dataset.list, t, inp.closest('.addrow'));
        return;
      }
      var rn = e.target.closest && e.target.closest('.ledger [data-rename]');
      if (rn && e.key === 'F2') { e.preventDefault(); rn.dispatchEvent(new MouseEvent('dblclick', { bubbles: true })); }
    });

    // Rows drag by their grip; neighbours slide out of the way as it passes.
    var lift = null;
    root.addEventListener('pointerdown', function (e) {
      var g = e.target.closest('.ledger .grip'); if (!g || e.button !== 0) return;
      e.preventDefault(); var row = g.closest('.drow');
      lift = { row: row, y: e.clientY, start: Array.prototype.indexOf.call(row.parentNode.children, row) };
      row.classList.add('lifting');
      if (g.setPointerCapture) g.setPointerCapture(e.pointerId);
    });
    root.addEventListener('pointermove', function (e) {
      if (!lift) return;
      var row = lift.row, dy = e.clientY - lift.y;
      function swap(other, down) {
        var before = other.getBoundingClientRect().top;
        if (down) row.parentNode.insertBefore(other, row); else row.parentNode.insertBefore(row, other);
        lift.y += down ? other.offsetHeight : -other.offsetHeight;
        var delta = before - other.getBoundingClientRect().top;
        if (!reduce()) other.animate([{ transform: 'translateY(' + delta + 'px)' }, { transform: 'none' }], { duration: 200, easing: 'cubic-bezier(.2,.8,.2,1)' });
      }
      var nx = row.nextElementSibling, pv = row.previousElementSibling;
      if (nx && nx.classList.contains('drow') && dy > nx.offsetHeight / 2) swap(nx, true);
      else if (pv && pv.classList.contains('drow') && dy < -pv.offsetHeight / 2) swap(pv, false);
      row.style.transform = 'translateY(' + (e.clientY - lift.y) + 'px)';
    });
    function drop() {
      if (!lift) return;
      var row = lift.row; lift = null;
      if (!reduce()) row.animate([{ transform: row.style.transform || 'none' }, { transform: 'none' }], { duration: 180, easing: 'ease-out' });
      row.style.transform = ''; row.classList.remove('lifting');
      var rows = row.parentNode.querySelectorAll('.drow[data-list="' + row.dataset.list + '"]');
      var order = Array.prototype.map.call(rows, function (r) { return +r.dataset.i; });
      if (order.some(function (v, i) { return v !== i; })) h.onReorder(row.dataset.list, order);
    }
    root.addEventListener('pointerup', drop);
    root.addEventListener('pointercancel', drop);

    // Click the bookmark to open or close; a click anywhere else folds it away,
    // unless it is pinned or something in it is half done.
    root.addEventListener('click', function (e) {
      var r = e.target.closest('.ribbon');
      if (r) {
        e.stopPropagation(); var bw = r.closest('.bw');
        if (!bw.classList.contains('open')) openLedger(bw);
        else if (h.busy && h.busy()) { if (h.blocked) h.blocked(bw); }
        else closeLedger(bw);
        return;
      }
      var p = e.target.closest('.lpin');
      if (p) { e.stopPropagation(); var b2 = p.closest('.bw'); b2._pinned = !b2._pinned; syncPin(b2); }
    }, true);
    function onDown(e) {
      if (!root.isConnected) { document.removeEventListener('pointerdown', onDown); document.removeEventListener('keydown', onKey); return; }
      if (layer && layer.contains(e.target)) return;
      root.querySelectorAll('.bw.open').forEach(function (bw) {
        if (bw.contains(e.target) || bw._pinned) return;
        if (h.busy && h.busy()) { if (h.blocked) h.blocked(bw); return; }
        closeLedger(bw);
      });
    }
    function onKey(e) {
      if (e.key !== 'Escape' || open) return;
      root.querySelectorAll('.bw.open').forEach(function (bw) {
        if (h.busy && h.busy()) { if (h.blocked) h.blocked(bw); return; }
        bw._pinned = false; closeLedger(bw);
      });
    }
    document.addEventListener('pointerdown', onDown);
    document.addEventListener('keydown', onKey);
    return { tab: function (name) { return tabs[name]; } };
  }
  function syncPin(bw) {
    var r = bw.querySelector('.ribbon'), p = bw.querySelector('.lpin');
    if (r) r.setAttribute('aria-expanded', String(bw.classList.contains('open')));
    if (p) { p.setAttribute('aria-pressed', String(!!bw._pinned)); p.title = bw._pinned ? 'Let it fold away' : 'Keep it open'; }
  }
  function openLedger(bw) {
    if (!bw.classList.contains('open')) {
      bw.classList.add('open', 'inking'); setTimeout(function () { bw.classList.remove('inking'); }, 900);
    }
    syncPin(bw);
  }
  function closeLedger(bw) { bw.classList.remove('open'); syncPin(bw); }

  // ---------- The board ----------
  function boardFrame(cls, plate, inner, tray) {
    return '<div class="board ' + cls + ' enter">' + plate +
      '<span class="nail a"></span><span class="nail b"></span><span class="nail c"></span><span class="nail d"></span>' +
      '<span class="lamp" aria-hidden="true"><span class="halo"></span><span class="arm"></span><span class="cup"></span><span class="flame"></span></span>' +
      '<div class="cork"><canvas class="dust" aria-hidden="true"></canvas>' + inner + '</div>' + (tray || '') + '</div>';
  }
  function placeStyle(p, i) {
    return 'left:' + p.x + '%;top:' + p.y + '%;width:' + p.w + '%;--r:' + p.r + 'deg;--in:' + (i * .08).toFixed(2) + 's;--d:' + (i % 5) * 1.5 + 's';
  }
  // A notice: kicker, title, a short blurb and the reward line, on torn paper.
  function noticeHTML(n, o) {
    return '<div class="note' + (o.cls ? ' ' + o.cls : '') + '" tabindex="0" role="button" aria-label="Read: ' + esc(n.title) + '"' + (o.attrs || '') + ' style="' + o.style + '">' +
      '<span class="pin' + (o.brass ? ' brass' : '') + '"></span>' +
      '<div class="pp' + (o.torn ? ' torn' : '') + '"><div class="k">' + esc(n.kicker) + '</div><div class="t">' + esc(n.title) + '</div>' +
      (n.blurb ? '<div class="x">' + esc(n.blurb) + '</div>' : '') + (n.reward ? '<div class="rw">' + esc(n.reward) + '</div>' : '') +
      (o.stamp ? '<span class="stamp' + (o.stamp === 'Done' ? ' done' : '') + '">' + esc(o.stamp) + '</span>' : '') +
      (o.seal ? '<span class="wax">' + esc(o.seal) + '</span>' : '') + '</div></div>';
  }
  function mapSVG(name) {
    return '<svg viewBox="0 0 100 80" aria-hidden="true">' +
      '<path d="M2 60 C20 50 30 64 48 54 S80 40 98 48" fill="none" stroke="#6b5434" stroke-width="1.4"/>' +
      '<path d="M2 70 C22 62 34 74 52 66 S82 54 98 60" fill="none" stroke="#6b5434" stroke-width=".8" stroke-dasharray="3 3"/>' +
      '<path d="M18 30 l6 -10 l6 10 z M30 34 l5 -8 l5 8 z" fill="#8a6d44"/>' +
      '<path d="M66 50 l4 4 m0 -4 l-4 4" stroke="#9b2c2c" stroke-width="2.2"/>' +
      '<text x="50" y="14" text-anchor="middle" font-family="IM Fell English SC, Georgia, serif" font-size="10" fill="#3a2a18">' + esc(String(name || '').slice(0, 22)) + '</text></svg>';
  }
  // A page's own picture when it has one, else its initial on card.
  function photoHTML(name, imageUrl) {
    if (imageUrl) return '<img src="' + esc(imageUrl) + '" alt="" loading="lazy" draggable="false">';
    return '<div class="noimg">' + esc((String(name || '?').trim()[0] || '?').toUpperCase()) + '</div>';
  }
  function sealLetter(title) { return (String(title || '').replace(/^(The|A|An) /i, '').trim()[0] || '·').toUpperCase(); }

  // Dust drifting through the lamp light, on the MotionRest clock.
  function dust(c) {
    if (!c || reduce()) return function () {};
    var ctx = c.getContext('2d'), dpr = Math.min(2, window.devicePixelRatio || 1), W = 0, H = 0, motes = [], raf = 0, stopped = false;
    function size() { var r = c.getBoundingClientRect(); W = r.width; H = r.height; c.width = Math.max(1, W * dpr); c.height = Math.max(1, H * dpr); ctx.setTransform(dpr, 0, 0, dpr, 0, 0); }
    size();
    for (var i = 0; i < 46; i++) motes.push({ x: Math.random() * W, y: Math.random() * H, r: .4 + Math.random() * 1.3, vx: (Math.random() - .3) * .12, vy: -.03 - Math.random() * .1, p: Math.random() * 6.28 });
    var MR = window.MotionRest, last = MR ? MR.now() : performance.now() / 1000;
    function tick() {
      raf = 0;
      if (stopped || !c.isConnected) return;
      var now = MR ? MR.now() : performance.now() / 1000, dt = Math.min(3, (now - last) * 60); last = now;
      if (c.width !== Math.round(c.getBoundingClientRect().width * dpr)) size();
      ctx.clearRect(0, 0, W, H);
      motes.forEach(function (m) {
        m.p += .02 * dt; m.x += (m.vx + Math.sin(m.p) * .08) * dt; m.y += m.vy * dt;
        if (m.y < -4) { m.y = H + 4; m.x = Math.random() * W; } if (m.x > W + 4) m.x = -4; if (m.x < -4) m.x = W + 4;
        // Motes glow only inside the lamp's cone from the top left.
        var d = Math.hypot(m.x / (W || 1), m.y / (H || 1) * .8), a = Math.max(0, .75 - d) * (.6 + .4 * Math.sin(m.p * 2));
        if (a <= .02) return;
        ctx.fillStyle = 'rgba(255,226,170,' + a.toFixed(3) + ')';
        ctx.beginPath(); ctx.arc(m.x, m.y, m.r, 0, 6.283); ctx.fill();
      });
      if (MR && MR.still()) return;
      raf = requestAnimationFrame(tick);
    }
    if (MR) MR.onWake(function () { if (!raf && !stopped) { last = MR.now(); raf = requestAnimationFrame(tick); } });
    raf = requestAnimationFrame(tick);
    return function () { stopped = true; if (raf) cancelAnimationFrame(raf); };
  }
  // The CSS loops (lamp flicker, the draught lifting paper corners) slow and
  // pause with MotionRest, like every other loop on the page.
  function restLoops(root) {
    var t = setInterval(function () {
      if (!root.isConnected) { clearInterval(t); return; }
      var MR = window.MotionRest, sp = MR ? MR.speed() : 1;
      root.classList.toggle('rest', sp === 0);
      if (!root.getAnimations) return;
      root.getAnimations({ subtree: true }).forEach(function (a) {
        if (!(a instanceof CSSAnimation) || !/flicker|draught|flame/.test(a.animationName)) return;
        if (sp === 0) { if (a.playState === 'running') a.pause(); return; }
        a.playbackRate = Math.max(.02, sp); if (a.playState === 'paused') a.play();
      });
    }, 250);
    return function () { clearInterval(t); };
  }

  // String hangs between two pins with a little sag, a soft shadow on the cork
  // and a twisted-fibre highlight.
  function drawStrings(face, pairs) {
    var svg = face.querySelector('svg.strings'); if (!svg) return;
    var fr = face.getBoundingClientRect(), out = '', ends = [];
    svg.setAttribute('viewBox', '0 0 ' + Math.max(1, fr.width) + ' ' + Math.max(1, fr.height));
    function pt(id) {
      var el = face.querySelector('[data-id="' + id + '"] .pin'); if (!el) return null;
      var r = el.getBoundingClientRect(); return [r.left + r.width / 2 - fr.left, r.top + r.height / 2 - fr.top];
    }
    pairs.forEach(function (s) {
      var a = pt(s[0]), b = pt(s[1]); if (!a || !b) return;
      var len = Math.hypot(b[0] - a[0], b[1] - a[1]), mx = (a[0] + b[0]) / 2, my = (a[1] + b[1]) / 2 + Math.min(40, len * .12);
      var d = 'M' + a[0].toFixed(1) + ' ' + a[1].toFixed(1) + ' Q' + mx.toFixed(1) + ' ' + my.toFixed(1) + ' ' + b[0].toFixed(1) + ' ' + b[1].toFixed(1);
      out += '<path d="' + d + '" class="s-sh"/><path d="' + d + '" class="s-r"/><path d="' + d + '" class="s-hi"/>';
      ends.push(a, b);
    });
    ends.forEach(function (e) { out += '<circle cx="' + e[0].toFixed(1) + '" cy="' + e[1].toFixed(1) + '" r="7" fill="url(#qbk-pinhead)"/>'; });
    svg.innerHTML = '<defs><radialGradient id="qbk-pinhead" cx=".35" cy=".3" r=".7"><stop offset="0" stop-color="#ff9b8a"/><stop offset=".55" stop-color="#c0392b"/><stop offset="1" stop-color="#6e1a12"/></radialGradient></defs>' + out;
  }

  // Things on the cork drag with the pointer; a short press still counts as a
  // click. o: {canMove(el), denied(el), moved(el, x, y), redraw()}.
  function wireDrag(root, o) {
    var drag = null, suppress = false;
    root.addEventListener('pointerdown', function (e) {
      var el = e.target.closest('.cork [data-id]');
      if (!el || e.button !== 0 || e.target.closest('[contenteditable]:focus, .dmtab')) return;
      if (root.querySelector('.tying')) return;
      var fr = el.parentElement.getBoundingClientRect(), r = el.getBoundingClientRect();
      drag = { el: el, sx: e.clientX, sy: e.clientY, ox: r.left - fr.left, oy: r.top - fr.top, fw: fr.width, fh: fr.height, moved: false,
        ok: o.canMove(el) && window.innerWidth > 760 };
    });
    root.addEventListener('pointermove', function (e) {
      if (!drag) return;
      var dx = e.clientX - drag.sx, dy = e.clientY - drag.sy;
      if (!drag.moved && Math.hypot(dx, dy) < 5) return;
      if (!drag.moved) {
        drag.moved = true;
        if (!drag.ok) {
          o.denied(drag.el, window.innerWidth <= 760);
          // The press turned into a drag that was refused; the click it ends in is not a click.
          suppress = true; setTimeout(function () { suppress = false; }, 400);
          if (!reduce()) drag.el.animate([{ translate: '0' }, { translate: '-3px 0' }, { translate: '3px 0' }, { translate: '0' }], { duration: 260 });
          drag = null; return;
        }
        drag.el.classList.add('dragging');
        if (drag.el.setPointerCapture) try { drag.el.setPointerCapture(e.pointerId); } catch (err) { /* already released */ }
      }
      var x = Math.max(0, Math.min(drag.fw - 30, drag.ox + dx)), y = Math.max(0, Math.min(drag.fh - 30, drag.oy + dy));
      drag.el.style.left = (x / drag.fw * 100) + '%'; drag.el.style.top = (y / drag.fh * 100) + '%';
      drag.el.style.right = 'auto'; drag.el.style.bottom = 'auto';
      o.redraw();
    });
    function up() {
      if (!drag) return;
      if (drag.moved) {
        suppress = true; setTimeout(function () { suppress = false; }, 0);
        drag.el.classList.remove('dragging');
        var pin = drag.el.querySelector('.pin');
        if (pin && !reduce()) pin.animate([{ transform: 'translateY(-8px) scale(1.6)' }, { transform: 'none' }], { duration: 360, easing: 'cubic-bezier(.2,1.4,.4,1)' });
        o.moved(drag.el, Math.round(parseFloat(drag.el.style.left) * 10) / 10, Math.round(parseFloat(drag.el.style.top) * 10) / 10);
      }
      drag = null;
    }
    root.addEventListener('pointerup', up);
    root.addEventListener('pointercancel', function () { if (drag && drag.moved) up(); drag = null; });
    root.addEventListener('click', function (e) { if (suppress) { e.stopPropagation(); e.preventDefault(); } }, true);
  }

  // The DM's options fold out from behind a thing on the board. Absolutely
  // placed, so opening one never moves anything else. acts: [[act, icon, title]].
  function dmTab(el, acts) {
    var old = el.querySelector(':scope > .dmtab'); if (old) old.remove();
    if (!acts.length) return;
    var left = (el.offsetLeft + el.offsetWidth) > el.parentElement.clientWidth * .74;
    var t = document.createElement('div'); t.className = 'dmtab' + (left ? ' tl' : '');
    t.setAttribute('role', 'toolbar'); t.setAttribute('aria-label', 'Options');
    t.innerHTML = acts.map(function (a) { return '<button type="button" data-act="' + a[0] + '" title="' + esc(a[2]) + '" aria-label="' + esc(a[2]) + '">' + a[1] + '</button>'; }).join('');
    el.appendChild(t);
  }
  // "Take it down?" asks inside the same fold-out before it acts.
  function askDown(tab, question) {
    tab.classList.add('ask');
    tab.innerHTML = '<span>' + esc(question) + '</span><button type="button" data-act="yes">Yes</button><button type="button" data-act="no">No</button>';
  }
  function takeDownAnim(el, done) {
    if (reduce()) { done(); return; }
    el.animate([{ transform: getComputedStyle(el).transform, opacity: 1 }, { transform: 'translateY(40px) rotate(18deg)', opacity: 0 }],
      { duration: 480, easing: 'cubic-bezier(.5,0,.8,.4)', fill: 'forwards' }).onfinish = done;
  }
  function dropIn(el, r) {
    if (reduce()) return;
    el.animate([{ opacity: 0, transform: 'translateY(-24px) rotate(' + r * 3 + 'deg) scale(1.08)' }, { opacity: 1, transform: 'rotate(' + r + 'deg)' }], { duration: 600, easing: 'cubic-bezier(.16,1,.3,1)' });
    var pin = el.querySelector('.pin');
    if (pin) pin.animate([{ transform: 'translateY(-8px) scale(1.7)' }, { transform: 'none' }], { duration: 420, delay: 380, easing: 'cubic-bezier(.2,1.4,.4,1)', fill: 'backwards' });
  }

  // A search box over pages (or maps) the viewer can see. o: {url, kind,
  // title, onPick(row)}; rows come from the server's picker, already filtered.
  function picker(host, o) {
    var p = host.querySelector('.picker');
    if (!p) { p = document.createElement('div'); p.className = 'picker'; host.appendChild(p); }
    p.hidden = false;
    p.innerHTML = '<div class="wh"><b>' + esc(o.title) + '</b><span class="sp"></span><button type="button" class="mini" data-pick-close>Close</button></div>' +
      '<input class="pk" placeholder="' + esc(o.placeholder || 'Search pages you can see') + '" aria-label="Search"><div class="pl" role="listbox"></div>';
    var q = p.querySelector('.pk'), list = p.querySelector('.pl'), seq = 0, tm = 0, rows = [];
    function load() {
      var my = ++seq;
      apiCall(o.url + (o.url.indexOf('?') < 0 ? '?' : '&') + 'kind=' + encodeURIComponent(o.kind) + '&q=' + encodeURIComponent(q.value.trim())).then(function (data) {
        if (my !== seq) return;
        rows = (data && (data.data || data.results || data)) || [];
        list.innerHTML = rows.length ? rows.map(function (r, i) {
          return '<button type="button" class="prow" role="option" data-pick="' + i + '"><span class="pthumb">' + (o.kind === 'map' ? '<span class="noimg">' + I.map + '</span>' : photoHTML(r.name, r.imageUrl)) +
            '</span><span class="grow">' + esc(r.name) + '</span><span class="sub">' + esc(r.typeName || (o.kind === 'map' ? 'Map' : '')) + '</span></button>';
        }).join('') : '<div class="sub" style="padding:8px">' + (o.kind !== 'map' && q.value.trim().length < 2 ? 'Type a name to search.' : 'Nothing you can see matches.') + '</div>';
      }).catch(function (err) { if (my === seq) list.innerHTML = '<div class="sub" style="padding:8px">' + esc(err.message) + '</div>'; });
    }
    q.addEventListener('input', function () { clearTimeout(tm); tm = setTimeout(load, 180); });
    p.onclick = function (e) {
      if (e.target.closest('[data-pick-close]')) { e.stopPropagation(); p.hidden = true; return; }
      var b = e.target.closest('[data-pick]'); if (!b) return;
      e.stopPropagation(); p.hidden = true; o.onPick(rows[+b.dataset.pick]);
    };
    load(); q.focus();
    if (!reduce()) p.animate([{ transform: 'translateY(16px)', opacity: 0 }, { transform: 'none', opacity: 1 }], { duration: 320, easing: 'cubic-bezier(.16,1,.3,1)' });
    return p;
  }

  window.QuestBoardKit = {
    boardFrame: boardFrame, placeStyle: placeStyle, noticeHTML: noticeHTML, mapSVG: mapSVG, photoHTML: photoHTML, sealLetter: sealLetter,
    dust: dust, restLoops: restLoops, drawStrings: drawStrings, wireDrag: wireDrag, dmTab: dmTab, askDown: askDown,
    takeDownAnim: takeDownAnim, dropIn: dropIn, picker: picker,
    I: I, LOOKS: LOOKS, esc: esc, api: apiCall, reduce: reduce,
    getLayer: getLayer, toast: toast,
    openSheet: openSheet, closeSheet: closeSheet, sheetIsOpen: sheetIsOpen,
    editInline: editInline, armRemove: armRemove,
    ribbon: ribbon, lrow: lrow, ledgerShell: ledgerShell, lookPanel: lookPanel, setLooks: setLooks,
    listHTML: listHTML, wireLedgers: wireLedgers, openLedger: openLedger, closeLedger: closeLedger, syncPin: syncPin
  };
})();
