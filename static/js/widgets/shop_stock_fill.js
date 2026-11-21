/**
 * shop_stock_fill.js -- the draft sheet behind a shop's "Fill the shelves".
 *
 * A Director's draft over the stock list: the kind of shop, how many goods,
 * how dear, which sources, then pin, leave out, roll again and Keep. Nothing
 * is saved until Keep. The draft itself comes from ShopStockGen
 * (shop_stock_gen.js); this file draws it and talks to the armory:
 *
 *   GET  <campaign>/armory/shops/<id>/fill-sources   items, system goods, food price
 *   GET  <campaign>/armory/shops/<id>/room           the shop's kind (roomType)
 *   GET  <campaign>/roll-tables                      a table to draw goods from
 *   GET  /static/shop-goods/everyday.json            the everyday list
 *   POST <campaign>/armory/shops/<id>/stock          Keep, one batch
 *   PUT  <campaign>/armory/food-price                the campaign's day of food
 *
 * Chronicle.shopFill.open(host, cfg) draws the sheet into host and returns
 * { close() }. cfg: { campaignUrl, shopId, words, onKept(result), onClose() }.
 */
(function () {
  'use strict';

  var KINDS = [
    ['general', 'General store'], ['forge', 'Forge'], ['apothecary', 'Apothecary'],
    ['jeweler', 'Jeweller'], ['market', 'Market'], ['library', 'Library']
  ];
  var LEVELS = [['cheap', 'Cheap'], ['fair', 'Fair'], ['dear', 'Dear']];
  var ORDINALS = ['', '1st', '2nd', '3rd', '4th', '5th', '6th'];
  // Rows shown before "Show all": enough to judge the draft at a glance.
  var FIRST_ROWS = 6;

  function el(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  }

  function fmt(n) {
    return String(Math.round(n * 100) / 100);
  }

  function ordinal(n) { return ORDINALS[n] || (n + 'th'); }

  function plural(n, one, many) { return n + ' ' + (n === 1 ? one : many); }

  function open(host, cfg) {
    var api = Chronicle.apiFetch;
    var base = cfg.campaignUrl + '/armory';
    var gen = window.ShopStockGen;

    var S = {
      loading: true, failed: false, keeping: false, showAll: false,
      sources: null, everyday: [], tables: [],
      kind: 'general', count: 14, prices: 'fair', cap: 1,
      on: { everyday: true, items: true, system: true, table: false },
      tableId: '', seed: String(Date.now()),
      pinned: {}, leftOut: [], edits: {},
      result: null, spots: cfg.spots || 2, dirty: false
    };
    var root = el('div', 'shop-draft');
    root.setAttribute('role', 'region');
    root.setAttribute('aria-label', 'Fill the shelves');
    host.appendChild(root);
    var dirtyId = 'shop-fill:' + cfg.shopId;

    function coinWord() {
      var w = Chronicle.localName ? Chronicle.localName(cfg.words || {}, 'gp') : { text: 'gp' };
      return w.text || 'gp';
    }

    // --- Draft ---

    function pool() {
      var s = S.sources || {};
      var tbl = null;
      for (var i = 0; i < S.tables.length; i++) if (S.tables[i].id === S.tableId) tbl = S.tables[i];
      return {
        everyday: S.everyday, items: s.items || [], system: s.system || [],
        table: tbl, listed: s.listed || []
      };
    }

    function lockedGoods() {
      var out = [];
      if (!S.result) return out;
      S.result.goods.forEach(function (g) { if (S.pinned[g.key]) out.push(g); });
      return out;
    }

    function draw(newSeed) {
      if (newSeed) S.seed = String(Date.now()) + Math.random().toString(36).slice(2, 6);
      var locked = lockedGoods();
      S.result = gen.run({
        kind: S.kind, count: S.count, prices: S.prices, foodPrice: (S.sources && S.sources.foodPrice) || 1,
        currency: 'gp', rarityCap: S.cap, spots: S.spots, sources: S.on, pool: pool(),
        seed: S.seed, locked: locked, leftOut: S.leftOut
      });
    }

    function noteDirty() {
      if (!S.dirty && Chronicle.markDirty) { Chronicle.markDirty(dirtyId); }
      S.dirty = true;
    }

    // --- Pieces ---

    function seg(name, options, value, onPick) {
      var wrap = el('span', 'sf-seg');
      wrap.setAttribute('role', 'group');
      wrap.setAttribute('aria-label', name);
      options.forEach(function (o) {
        var b = el('button', 'sf-seg-b' + (o[0] === value ? ' is-on' : ''), o[1]);
        b.type = 'button';
        b.setAttribute('aria-pressed', o[0] === value ? 'true' : 'false');
        b.onclick = function () { onPick(o[0]); };
        wrap.appendChild(b);
      });
      return wrap;
    }

    function select(label, options, value, onPick, extra) {
      var f = el('label', 'sf-field');
      f.appendChild(el('span', 'sf-label', label));
      var s = el('select', 'sf-select');
      options.forEach(function (o) {
        var op = el('option', '', o[1]);
        op.value = o[0];
        if (o[0] === value) op.selected = true;
        s.appendChild(op);
      });
      s.onchange = function () { onPick(s.value); };
      f.appendChild(s);
      if (extra) f.appendChild(extra);
      return f;
    }

    function sw(label, on, onToggle, disabledNote) {
      var b = el('button', 'sf-sw' + (on ? ' is-on' : ''));
      b.type = 'button';
      b.setAttribute('role', 'switch');
      b.setAttribute('aria-checked', on ? 'true' : 'false');
      b.appendChild(el('i', 'sf-sw-track'));
      b.appendChild(el('span', '', label));
      if (disabledNote) b.title = disabledNote;
      b.onclick = onToggle;
      return b;
    }

    function iconBtn(cls, icon, label, onClick, pressed) {
      var b = el('button', 'sf-ico ' + cls);
      b.type = 'button';
      b.setAttribute('aria-label', label);
      b.title = label;
      if (pressed != null) b.setAttribute('aria-pressed', pressed ? 'true' : 'false');
      b.appendChild(el('i', 'fa-solid ' + icon));
      b.onclick = onClick;
      return b;
    }

    function sourceLabel(g) {
      if (g.source === 'item') return 'Your item';
      if (g.source === 'system') {
        var sys = (S.sources && S.sources.systemName) || 'System';
        var bits = [sys];
        var more = [];
        if (g.kind) more.push(g.kind.toLowerCase());
        if (g.rarity) more.push(ordinal(g.rarity) + ' ' + ((S.sources && S.sources.rarityLabel) || 'rarity'));
        return sys + (more.length ? ' · ' + more.join(', ') : '');
      }
      if (g.source === 'table') return 'From ' + ((S.tables.filter(function (t) { return t.id === S.tableId; })[0] || {}).name || 'a table');
      return 'Everyday';
    }

    // A price or count the Director types pins the good, so a roll again
    // never throws away what they set by hand.
    function numberLine(value, label, min, step, onSet, placeholder) {
      var i = el('input', 'sf-line');
      i.type = 'number'; i.min = String(min); i.step = String(step);
      i.value = value == null ? '' : String(value);
      if (placeholder) i.placeholder = placeholder;
      i.setAttribute('aria-label', label);
      i.onchange = function () { onSet(i.value); };
      return i;
    }

    function patch(g, fields) {
      Object.keys(fields).forEach(function (k) { g[k] = fields[k]; });
      S.pinned[g.key] = true;
      g.edited = true;
      noteDirty();
      render();
    }

    function row(g) {
      var r = el('div', 'sf-g' + (S.pinned[g.key] ? ' is-kept' : ''));
      r.setAttribute('data-good', g.key);
      var nm = el('div', 'sf-nm');
      var title;
      if (g.source === 'everyday' || g.source === 'table') {
        // A good with no page is named here, so its name is the Director's to change.
        title = el('input', 'sf-line sf-name');
        title.type = 'text'; title.maxLength = 200; title.value = g.name;
        title.setAttribute('aria-label', 'Name of ' + g.name);
        title.onchange = function () {
          var v = title.value.trim();
          if (!v) { title.value = g.name; return; }
          patch(g, { name: v });
        };
      } else {
        title = el('b', 'sf-name-text', g.name);
      }
      nm.appendChild(title);
      if (S.pinned[g.key]) nm.appendChild(el('span', 'sf-kept', ' · kept'));
      var sub = el('span', 'sf-sub', sourceLabel(g) + ' ');
      var shelf = el('button', 'sf-q', g.today ? 'Always on the shelf instead' : 'Put on today’s shelf instead');
      shelf.type = 'button';
      shelf.onclick = function () { patch(g, { today: !g.today }); };
      sub.appendChild(shelf);
      nm.appendChild(sub);
      r.appendChild(nm);

      var pc = el('span', 'sf-pc');
      var price = numberLine(g.price == null ? '' : g.price, 'Price of ' + g.name, 0, 0.01, function (v) {
        var n = parseFloat(v);
        patch(g, { price: n > 0 ? n : null, note: n > 0 ? '' : 'Set a price' });
      }, g.price == null ? 'Set a price' : '');
      price.className += ' sf-price' + (g.price == null ? ' is-unset' : '');
      pc.appendChild(price);
      pc.appendChild(el('span', 'sf-coin', g.price == null ? '' : coinWord()));
      pc.appendChild(el('span', 'sf-times', '×'));
      pc.appendChild(numberLine(g.quantity, 'How many of ' + g.name, 0, 1, function (v) {
        var n = parseInt(v, 10);
        patch(g, { quantity: n >= 0 ? n : 1 });
      }));
      r.appendChild(pc);

      r.appendChild(iconBtn('sf-pin' + (S.pinned[g.key] ? ' is-on' : ''), 'fa-thumbtack',
        S.pinned[g.key] ? 'Unpin ' + g.name : 'Pin ' + g.name, function () {
          if (S.pinned[g.key]) delete S.pinned[g.key]; else S.pinned[g.key] = true;
          noteDirty(); render();
        }, !!S.pinned[g.key]));
      r.appendChild(iconBtn('sf-out', 'fa-xmark', 'Leave out ' + g.name, function () {
        S.leftOut.push(g.key);
        delete S.pinned[g.key];
        S.result.goods = S.result.goods.filter(function (x) { return x.key !== g.key; });
        noteDirty();
        // Leaving a good out shrinks the draft; the summary is redrawn from what is left.
        draw(false);
        render();
      }));
      return r;
    }

    function goodsList() {
      var goods = S.result.goods;
      var box = el('div', 'sf-goods');
      if (goods.length === 0) {
        box.appendChild(el('p', 'sf-empty', 'Nothing drafted. Switch on a source or change the kind of shop.'));
        return box;
      }
      var shown = S.showAll ? goods : goods.slice(0, FIRST_ROWS);
      var wasToday = false, started = false;
      shown.forEach(function (g) {
        if (g.today && !wasToday) {
          var lab = el('div', 'sf-glab');
          lab.appendChild(el('span', 'sf-lab', 'Today’s shelf · ' + (S.spots === 1 ? 'one' : ['', '', 'two', 'three', 'four', 'five'][S.spots] || S.spots) + ' a day'));
          box.appendChild(lab);
        }
        wasToday = g.today;
        started = true;
        box.appendChild(row(g));
      });
      if (!started) return box;
      return box;
    }

    // --- Sheet ---

    // Clearing the sheet drops the focused field, and the browser answers with
    // a blur and change event that calls render again mid-clear. The nested
    // call is queued and run once the first finishes.
    var drawing = false, redraw = false;
    function render() {
      if (drawing) { redraw = true; return; }
      drawing = true;
      try {
        do { redraw = false; paint(); } while (redraw);
      } finally {
        drawing = false;
      }
    }

    function paint() {
      var keepFocus = document.activeElement && document.activeElement.getAttribute
        ? document.activeElement.getAttribute('aria-label') : null;
      while (root.firstChild) root.removeChild(root.firstChild);

      if (S.loading) { root.appendChild(el('p', 'sf-say', 'Getting the goods ready…')); return; }
      if (S.failed) {
        var bad = el('p', 'sf-say');
        bad.setAttribute('role', 'alert');
        bad.appendChild(document.createTextNode('The draft couldn’t be made. '));
        var again = el('button', 'sf-q', 'Try again');
        again.type = 'button';
        again.onclick = function () { S.loading = true; S.failed = false; render(); load(); };
        bad.appendChild(again);
        var cl = el('button', 'sf-q', 'Close');
        cl.type = 'button'; cl.onclick = close;
        bad.appendChild(document.createTextNode(' '));
        bad.appendChild(cl);
        root.appendChild(bad);
        return;
      }

      var sources = S.sources || {};
      var res = S.result;
      var say = el('p', 'sf-say');
      say.setAttribute('aria-live', 'polite');
      say.textContent = res.summary;
      root.appendChild(say);

      var ctl = el('div', 'sf-ctl');
      ctl.appendChild(select('Kind of shop', KINDS, S.kind, function (v) { S.kind = v; draw(false); render(); }));
      var many = el('label', 'sf-field');
      many.appendChild(el('span', 'sf-label', 'How many'));
      var num = el('input', 'sf-num');
      num.type = 'number'; num.min = '1'; num.max = '80'; num.value = String(S.count);
      num.setAttribute('aria-label', 'How many goods');
      num.onchange = function () {
        var n = parseInt(num.value, 10);
        S.count = n >= 1 ? Math.min(80, n) : 14;
        draw(false); render();
      };
      many.appendChild(num);
      ctl.appendChild(many);
      var pr = el('div', 'sf-field');
      pr.appendChild(el('span', 'sf-label', 'Prices'));
      pr.appendChild(seg('Prices', LEVELS, S.prices, function (v) { S.prices = v; draw(false); render(); }));
      ctl.appendChild(pr);
      if (sources.rarityLabel && (sources.system || []).length) {
        var caps = [['0', 'Any']];
        var top = 1;
        (sources.system || []).forEach(function (g) { if (g.rarity > top) top = g.rarity; });
        for (var c = 1; c <= Math.min(top, 6); c++) caps.push([String(c), ordinal(c) + ' ' + sources.rarityLabel]);
        ctl.appendChild(select('Finds up to', caps, String(S.cap), function (v) { S.cap = parseInt(v, 10) || 0; draw(false); render(); }));
      }
      root.appendChild(ctl);

      var src = el('div', 'sf-src');
      src.appendChild(sw('Everyday goods', S.on.everyday, function () { S.on.everyday = !S.on.everyday; draw(false); render(); }));
      src.appendChild(sw('Your items (' + (sources.items || []).length + ')', S.on.items, function () { S.on.items = !S.on.items; draw(false); render(); }));
      if ((sources.system || []).length) {
        src.appendChild(sw((sources.systemName || 'System') + ' items (' + sources.system.length + ')', S.on.system, function () { S.on.system = !S.on.system; draw(false); render(); }));
      }
      src.appendChild(sw('A rolling table', S.on.table, function () {
        S.on.table = !S.on.table;
        if (S.on.table && !S.tableId && S.tables.length) S.tableId = S.tables[0].id;
        draw(false); render();
      }));
      root.appendChild(src);
      if (S.on.table) {
        if (S.tables.length) {
          root.appendChild(select('Table', S.tables.map(function (t) { return [t.id, t.name + ' (' + t.entries.length + ')']; }), S.tableId, function (v) { S.tableId = v; draw(false); render(); }));
        } else {
          root.appendChild(el('p', 'sf-hint', 'No rolling tables yet. Make one under Tables, then come back.'));
        }
      }

      // The scale every everyday price follows, kept for the whole campaign.
      var food = el('label', 'sf-food');
      food.appendChild(document.createTextNode('A day’s food costs '));
      var fi = el('input', 'sf-line sf-foodin');
      fi.type = 'number'; fi.min = '0.01'; fi.step = '0.01'; fi.value = fmt(sources.foodPrice || 1);
      fi.setAttribute('aria-label', 'What a day’s food costs');
      fi.onchange = function () { saveFood(parseFloat(fi.value)); };
      food.appendChild(fi);
      food.appendChild(document.createTextNode(' ' + coinWord() + '. Everyday prices follow it; prices you set by hand stay.'));
      root.appendChild(food);

      root.appendChild(goodsList());

      if (res.goods.length > FIRST_ROWS) {
        var more = el('button', 'sf-q sf-more', S.showAll ? 'Show fewer' : 'Show all ' + res.goods.length);
        more.type = 'button';
        more.setAttribute('aria-expanded', S.showAll ? 'true' : 'false');
        more.onclick = function () { S.showAll = !S.showAll; render(); };
        root.appendChild(more);
      }

      var foot = el('div', 'sf-foot');
      var keep = el('button', 'btn-primary sf-keep', S.keeping ? 'Keeping…' : 'Keep ' + plural(res.goods.length, 'good', 'goods'));
      keep.type = 'button';
      keep.disabled = S.keeping || res.goods.length === 0;
      keep.onclick = keepAll;
      foot.appendChild(keep);
      var roll = el('button', 'btn-secondary sf-roll', 'Roll again');
      roll.type = 'button';
      roll.disabled = S.keeping;
      roll.onclick = function () { draw(true); render(); };
      foot.appendChild(roll);
      var disc = el('button', 'sf-q', 'Discard');
      disc.type = 'button';
      disc.onclick = close;
      foot.appendChild(disc);
      root.appendChild(foot);

      var unpriced = res.goods.filter(function (g) { return g.price == null; }).length;
      if (unpriced > 0) {
        root.appendChild(el('p', 'sf-hint', (unpriced === 1 ? 'One good has' : (['', '', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven', 'Eight', 'Nine'][unpriced] || unpriced) + ' goods have') +
          ' no price yet. Players can’t buy ' + (unpriced === 1 ? 'it' : 'them') + ' until you set one.'));
      }

      if (keepFocus) {
        // Matched by comparing the label, never by building a selector from
        // it: a good's name can hold quotes and backslashes.
        var labelled = root.querySelectorAll('[aria-label]');
        for (var li = 0; li < labelled.length; li++) {
          if (labelled[li].getAttribute('aria-label') === keepFocus) { labelled[li].focus(); break; }
        }
      }
    }

    // --- Talking to the armory ---

    function saveFood(n) {
      if (!(n > 0)) { Chronicle.notify('A day’s food has to cost more than nothing.', 'error'); render(); return; }
      api(base + '/food-price', { method: 'PUT', body: { foodPrice: n } })
        .then(function (res) { if (!res.ok) throw new Error('save'); return res.json(); })
        .then(function (j) { S.sources.foodPrice = j.foodPrice; draw(false); render(); })
        .catch(function () { Chronicle.notify('Could not save what a day’s food costs.', 'error'); render(); });
    }

    function keepAll() {
      if (S.keeping || !S.result.goods.length) return;
      S.keeping = true; render();
      var goods = S.result.goods.map(function (g) {
        var out = { name: g.name, price: g.price == null ? 0 : g.price, currency: g.currency || 'gp', today: !!g.today };
        if (g.itemId) out.itemId = g.itemId;
        if (g.quantity != null) out.quantity = g.quantity;
        return out;
      });
      api(base + '/shops/' + cfg.shopId + '/stock', { method: 'POST', body: { goods: goods } })
        .then(function (res) {
          if (!res.ok) {
            return res.json().catch(function () { return {}; }).then(function (b) {
              throw new Error(b.message || 'The goods couldn’t be kept.');
            });
          }
          return res.json();
        })
        .then(function (j) {
          var unpriced = S.result.goods.filter(function (g) { return g.price == null; }).length;
          if (cfg.onKept) cfg.onKept({ batch: j.batch, relations: j.relations || [], skipped: j.skipped || 0, unpriced: unpriced });
          close(true);
        })
        .catch(function (err) {
          S.keeping = false; render();
          Chronicle.notify(err.message || 'The goods couldn’t be kept.', 'error');
        });
    }

    function load() {
      var everyday = fetch('/static/shop-goods/everyday.json').then(function (r) { return r.ok ? r.json() : { goods: [] }; }).catch(function () { return { goods: [] }; });
      var sources = api(base + '/shops/' + cfg.shopId + '/fill-sources', { method: 'GET' }).then(function (r) {
        if (!r.ok) throw new Error('sources');
        return r.json();
      });
      var room = api(base + '/shops/' + cfg.shopId + '/room', { method: 'GET' }).then(function (r) { return r.ok ? r.json() : null; }).catch(function () { return null; });
      var tables = api(cfg.campaignUrl + '/roll-tables', { method: 'GET' }).then(function (r) { return r.ok ? r.json() : null; }).catch(function () { return null; });
      Promise.all([everyday, sources, room, tables]).then(function (a) {
        S.everyday = a[0].goods || [];
        S.sources = a[1];
        var kind = a[2] && (a[2].roomType || (a[2].layout && a[2].layout.roomType));
        if (kind && gen.KIND_WORDS[kind]) S.kind = kind;
        S.tables = ((a[3] && a[3].tables) || []).filter(function (t) { return t.entries && t.entries.length; });
        S.on.system = (S.sources.system || []).length > 0;
        draw(false);
        S.loading = false;
        render();
        var first = root.querySelector('.sf-keep');
        if (first && first.focus) first.focus();
      }).catch(function () {
        S.loading = false; S.failed = true; render();
      });
    }

    var closed = false;
    function close(kept) {
      if (closed) return;
      closed = true;
      if (S.dirty && Chronicle.markClean) Chronicle.markClean(dirtyId);
      document.removeEventListener('keydown', onKey, true);
      if (root.parentNode) root.parentNode.removeChild(root);
      if (cfg.onClose) cfg.onClose(kept === true);
    }

    // Escape leaves the sheet, but never while typing in one of its fields.
    function onKey(e) {
      if (e.key !== 'Escape') return;
      var t = e.target;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA') && t.type !== 'checkbox') { t.blur(); return; }
      if (root.contains && !root.contains(t) && t !== document.body) return;
      close();
    }
    document.addEventListener('keydown', onKey, true);

    render();
    load();
    return { close: close, root: root };
  }

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.shopFill = { open: open };
})();
