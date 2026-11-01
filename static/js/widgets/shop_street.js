/**
 * shop_street.js -- the Shops street.
 *
 * Draws the shop pages the viewer can see as a street of storefronts, one
 * street per town (a shop's town is the page it sits under). Pressing a
 * storefront leans it toward the viewer and opens the shop's page, where the
 * shop room's door is. While the campaign's shops are closed (downtime is
 * off) every storefront takes its closed look; buying then needs the GM's OK,
 * which the shop page already handles.
 *
 * The GM (data-can-dress) gets a small icon on each storefront, and a
 * right-click or long-press does the same: it opens a card to dress that
 * storefront, with a live preview, saved through PUT .../front.
 *
 * Mount via:
 *   <section data-widget="shop_street" data-campaign-id="..." data-can-dress="true"></section>
 */
(function () {
  'use strict';

  var TUNE = '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M3 17v2h6v-2H3zM3 5v2h10V5H3zm10 16v-2h8v-2h-8v-2h-2v6h2zM7 9v2H3v2h4v2h2V9H7zm14 4v-2H11v2h10zm-6-4h2V7h4V5h-4V3h-2v6z"/></svg>';
  var CLOSE = '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg>';
  var SKYLINE = '<svg class="ss-skyline" viewBox="0 0 600 80" preserveAspectRatio="none" aria-hidden="true"><path d="M0 80V52h30V34l14-10 14 10v18h22V40h40V20l10-12 10 12v32h36V44h26V30h18v22h40V36l20-14 20 14v16h30V42h44V26h16v26h36V38l12-8 12 8v14h40V30l18-16 18 16v22h24V46h20v34z" fill="currentColor"/></svg>';
  // Shops with no town page share one street, named by this.
  var NO_TOWN = '';
  var LONG_PRESS_MS = 500;

  function esc(s) { return Chronicle.escapeHtml(String(s == null ? '' : s)); }
  function motionOff() { return document.documentElement.getAttribute('data-motion') === 'off'; }

  /* groupByTown keeps the server's order (by name) and lists towns in the
     order their first shop appears, with shops of no town last. */
  function groupByTown(shops) {
    var order = [], by = {};
    shops.forEach(function (s) {
      var k = s.townId || NO_TOWN;
      if (!by[k]) { by[k] = { id: k, name: s.townName || 'Other shops', shops: [] }; order.push(k); }
      by[k].shops.push(s);
    });
    order.sort(function (a, b) { return (a === NO_TOWN) - (b === NO_TOWN); });
    return order.map(function (k) { return by[k]; });
  }

  Chronicle.register('shop_street', {
    init: function (el) {
      var S = window.ChronicleStorefront;
      var campaignId = el.dataset.campaignId;
      var canDress = el.dataset.canDress === 'true';
      var base = '/campaigns/' + encodeURIComponent(campaignId) + '/armory/shops';
      var st = { view: null, towns: [], town: null, dressing: null };
      var alive = true;

      el.classList.add('ss');
      el.innerHTML =
        '<div class="ss-towns" role="tablist" aria-label="Towns" hidden></div>' +
        '<div class="ss-stage">' + SKYLINE +
        '<div class="ss-row" role="list"></div><div class="ss-empty" hidden></div><div class="ss-road"></div>' +
        '<div class="ss-scrim"></div><div class="ss-card" role="dialog" aria-modal="true" tabindex="-1" hidden></div></div>' +
        '<p class="ss-foot" aria-live="polite"></p>';
      var townsEl = el.querySelector('.ss-towns'), row = el.querySelector('.ss-row'),
        empty = el.querySelector('.ss-empty'), foot = el.querySelector('.ss-foot'),
        scrim = el.querySelector('.ss-scrim'), card = el.querySelector('.ss-card');

      function load() {
        Chronicle.apiFetch(base + '/street').then(function (r) {
          if (!r.ok) throw new Error('street ' + r.status);
          return r.json();
        }).then(function (v) {
          if (!alive) return;
          st.view = v;
          st.towns = groupByTown(v.shops || []);
          if (!st.towns.some(function (t) { return t.id === st.town; })) st.town = st.towns.length ? st.towns[0].id : null;
          render();
        }).catch(function () {
          if (!alive) return;
          empty.hidden = false;
          empty.innerHTML = '<b>The street could not be loaded</b>Reload the page to try again.';
        });
      }

      function render() {
        var v = st.view;
        townsEl.hidden = st.towns.length < 2;
        townsEl.innerHTML = st.towns.map(function (t) {
          return '<button type="button" role="tab" aria-selected="' + (t.id === st.town) + '" data-town="' + esc(t.id) + '">' + esc(t.name) + '</button>';
        }).join('');
        var town = st.towns.filter(function (t) { return t.id === st.town; })[0];
        var shops = town ? town.shops : [];
        empty.hidden = shops.length > 0;
        if (!shops.length) {
          empty.innerHTML = '<b>No shops yet</b>' + (canDress ? 'Make a page of the Shop kind and it appears here.' : 'Your GM hasn’t opened any shops to you.');
        }
        row.innerHTML = shops.map(function (s, i) {
          var front = s.front || {};
          front.name = s.name;
          return '<div class="ss-lot" role="listitem" style="--i:' + i + '" data-id="' + esc(s.id) + '">' +
            '<a class="ss-front front' + (v.open ? '' : ' shut') + '" style="--i:' + i + '" href="' + esc(s.url) + '" data-front="' + esc(s.id) + '">' + S.draw(front) + '</a>' +
            (canDress ? '<button type="button" class="ss-gm" data-dress="' + esc(s.id) + '" aria-label="Dress the storefront of ' + esc(s.name) + '" aria-haspopup="dialog">' + TUNE + '</button>' : '') +
            '<div class="ss-plate"><span class="ss-nm">' + esc(s.name) + '</span>' +
            '<span class="ss-pill ' + (v.open ? 'open' : 'shut') + '">' + (v.open ? 'Open' : 'Closed') + '</span></div></div>';
        }).join('');
        var msg = v.open ? '' : 'Shops are closed for now. You can still ask the GM to buy something.';
        if (v.truncated) msg += (msg ? ' ' : '') + 'Only the first shops are shown here; the rest are in the list below.';
        foot.textContent = msg;
        foot.hidden = !msg;
      }

      function shopById(id) {
        var all = (st.view && st.view.shops) || [];
        for (var i = 0; i < all.length; i++) if (all[i].id === id) return all[i];
        return null;
      }

      /* Pressing a storefront leans it toward the viewer, then follows the
         link. A modified click (new tab) is left to the browser. */
      function enter(a, e) {
        if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
        if (motionOff()) return;
        e.preventDefault();
        a.classList.add('going');
        setTimeout(function () { window.location.href = a.getAttribute('href'); }, 200);
      }

      var WALLS = ['#d7b98f', '#8d9098', '#b7cdb0', '#3b3561', '#a9b9c6', '#d9c7a3', '#c98b6b', '#5d6272'];
      var ACCENTS = ['#2f5d50', '#7a1f2b', '#1f4e79', '#3f6b4f', '#9a7b2f', '#5a2d2d', '#7a5a2a', '#4f6b55'];
      var NAMES = {
        style: { timber: 'Timber', stone: 'Stone', plaster: 'Plaster', painted: 'Painted' },
        awning: { none: 'None', plain: 'Plain', stripe: 'Striped' },
        sign: { hang: 'Hanging', board: 'Board', banner: 'Banner' },
        closed: { shutters: 'Shutters', curtains: 'Curtains', boards: 'Boarded', board: 'Note', dark: 'Dark' },
        ware: { bottle: 'Potions', sword: 'Blades', scroll: 'Scrolls', gem: 'Gems', bread: 'Bread', shield: 'Shields', hammer: 'Tools', herb: 'Herbs' }
      };
      function seg(key, val) {
        return '<div class="ss-seg" role="group">' + S.LISTS[key].filter(Boolean).map(function (v) {
          return '<button type="button" data-k="' + key + '" data-v="' + v + '" aria-pressed="' + (v === val) + '">' + NAMES[key][v] + '</button>';
        }).join('') + '</div>';
      }
      function swatches(key, val, list) {
        return '<div class="ss-sw" role="group">' + list.map(function (c) {
          return '<button type="button" data-k="' + key + '" data-v="' + c + '" aria-pressed="' + (c === val) + '" aria-label="Colour ' + c + '" style="background:' + c + '"></button>';
        }).join('') + '</div>';
      }
      function field(label, html) { return '<div class="ss-f"><span>' + label + '</span>' + html + '</div>'; }

      /* The card edits a copy (st.dressing.f); nothing is saved until Save. */
      function drawCard() {
        var d = st.dressing, f = d.f;
        var prev = JSON.parse(JSON.stringify(f));
        prev.name = d.shop.name;
        card.innerHTML =
          '<header><div><div class="ss-sub">Only you see this</div><h3>' + esc(d.shop.name) + '</h3></div>' +
          '<button type="button" class="ss-x" data-act="close" aria-label="Close">' + CLOSE + '</button></header>' +
          '<div class="ss-prev"><div class="front' + (d.peekShut ? ' shut' : '') + '">' + S.draw(prev) + '</div></div>' +
          '<label class="ss-row2"><span>Show it closed</span><input type="checkbox" data-act="peek"' + (d.peekShut ? ' checked' : '') + '></label>' +
          field('Building', seg('style', f.style)) +
          field('Walls', swatches('colour', f.colour, WALLS)) +
          field('Trim', swatches('trim', f.trim, ACCENTS)) +
          field('Awning', seg('awning', f.awning) + (f.awning !== 'none' ? swatches('awningColour', f.awningColour, ACCENTS) : '')) +
          field('Sign', seg('sign', f.sign)) +
          field('Words on the sign', '<input type="text" data-t="words" maxlength="24" placeholder="' + esc(d.shop.name) + '" value="' + esc(f.words) + '">') +
          (f.sign === 'hang' ? field('Picture on the sign', '<select data-t="icon">' + S.LISTS.icon.map(function (v) {
            return '<option value="' + v + '"' + (v === f.icon ? ' selected' : '') + '>' + (v ? NAMES.ware[v] : 'None') + '</option>';
          }).join('') + '</select>') : '') +
          field('In the windows (up to three)', '<div class="ss-seg" role="group">' + S.LISTS.ware.map(function (w) {
            return '<button type="button" data-ware="' + w + '" aria-pressed="' + (f.wares.indexOf(w) >= 0) + '">' + NAMES.ware[w] + '</button>';
          }).join('') + '</div>') +
          '<label class="ss-row2"><span>Lamp by the door</span><input type="checkbox" data-act="lamp"' + (f.lamp ? ' checked' : '') + '></label>' +
          field('When it’s closed', seg('closed', f.closed) +
            (f.closed === 'shutters' ? swatches('shutterColour', f.shutterColour, ACCENTS) : '') +
            (f.closed === 'curtains' ? swatches('curtainColour', f.curtainColour, ACCENTS) : '') +
            (f.closed === 'board' ? '<input type="text" data-t="closedNote" maxlength="26" placeholder="Back soon" value="' + esc(f.closedNote) + '">' : '')) +
          '<div class="ss-msg" role="status">' + esc(d.msg || '') + '</div>' +
          '<div class="ss-acts"><button type="button" class="btn-secondary text-sm" data-act="close">Cancel</button>' +
          '<button type="button" class="btn-primary text-sm" data-act="save"' + (d.saving ? ' disabled' : '') + '>' + (d.saving ? 'Saving…' : 'Save') + '</button></div>';
      }
      /* redrawPreview swaps only the picture, so typing keeps focus. */
      function redrawPreview() {
        var d = st.dressing, prev = JSON.parse(JSON.stringify(d.f));
        prev.name = d.shop.name;
        var box = card.querySelector('.ss-prev .front');
        box.innerHTML = S.draw(prev);
        box.classList.toggle('shut', !!d.peekShut);
      }

      function openDress(id, from) {
        var shop = shopById(id);
        if (!shop) return;
        st.dressing = { shop: shop, f: S.clean(shop.front || {}), from: from, peekShut: false, msg: '' };
        drawCard();
        card.hidden = false;
        requestAnimationFrame(function () { card.classList.add('on'); scrim.classList.add('on'); card.focus({ preventScroll: true }); });
      }
      function closeDress() {
        var d = st.dressing;
        st.dressing = null;
        card.classList.remove('on');
        scrim.classList.remove('on');
        setTimeout(function () { if (!st.dressing) card.hidden = true; }, motionOff() ? 0 : 220);
        if (d && d.from && d.from.isConnected) d.from.focus({ preventScroll: true });
      }
      function save() {
        var d = st.dressing;
        if (!d || d.saving) return;
        var body = JSON.parse(JSON.stringify(d.f));
        delete body.name;
        delete body.refuseNote;
        d.saving = true; d.msg = '';
        drawCard();
        Chronicle.apiAction(base + '/' + encodeURIComponent(d.shop.id) + '/front', { method: 'PUT', body: body }).then(function (res) {
          d.shop.front = res.front;
          d.shop.saved = true;
          closeDress();
          render();
        }).catch(function (err) {
          d.saving = false;
          d.msg = (err && err.message) || 'The storefront could not be saved.';
          if (st.dressing === d) drawCard();
        });
      }
      card.addEventListener('click', function (e) {
        var d = st.dressing;
        if (!d) return;
        var b = e.target.closest('button,input[type=checkbox]');
        if (!b) return;
        var act = b.getAttribute('data-act');
        if (act === 'close') { closeDress(); return; }
        if (act === 'save') { save(); return; }
        if (act === 'peek') { d.peekShut = b.checked; redrawPreview(); return; }
        if (act === 'lamp') { d.f.lamp = b.checked; redrawPreview(); return; }
        var w = b.getAttribute('data-ware');
        if (w) {
          var at = d.f.wares.indexOf(w);
          if (at >= 0) d.f.wares.splice(at, 1);
          else if (d.f.wares.length < 3) d.f.wares.push(w);
          drawCard();
          return;
        }
        var k = b.getAttribute('data-k');
        if (k) {
          d.f[k] = b.getAttribute('data-v');
          if (k === 'closed') d.peekShut = true;
          drawCard();
        }
      });
      card.addEventListener('input', function (e) {
        var d = st.dressing, t = e.target.getAttribute('data-t');
        if (!d || !t) return;
        d.f[t] = e.target.value;
        redrawPreview();
      });

      function onClick(e) {
        var t = e.target.closest('[data-town]');
        if (t && townsEl.contains(t)) {
          st.town = t.getAttribute('data-town');
          row.classList.add('out');
          setTimeout(function () { render(); row.classList.remove('out'); }, motionOff() ? 0 : 200);
          return;
        }
        var g = e.target.closest('[data-dress]');
        if (g) { openDress(g.getAttribute('data-dress'), g); return; }
        var a = e.target.closest('[data-front]');
        if (a) { if (a._held) { e.preventDefault(); a._held = false; return; } enter(a, e); }
      }
      function onContext(e) {
        var a = canDress && e.target.closest('[data-front]');
        if (!a) return;
        e.preventDefault();
        openDress(a.getAttribute('data-front'), a);
      }
      var pressTimer = null;
      function onDown(e) {
        var a = canDress && e.pointerType !== 'mouse' && e.target.closest('[data-front]');
        if (!a) return;
        pressTimer = setTimeout(function () {
          a._held = true;
          a.classList.add('held');
          setTimeout(function () { a.classList.remove('held'); }, 250);
          openDress(a.getAttribute('data-front'), a);
        }, LONG_PRESS_MS);
      }
      function onUp() { clearTimeout(pressTimer); }
      function onKey(e) { if (e.key === 'Escape' && st.dressing) closeDress(); }

      el.addEventListener('click', onClick);
      el.addEventListener('contextmenu', onContext);
      el.addEventListener('pointerdown', onDown);
      el.addEventListener('pointerup', onUp);
      el.addEventListener('pointercancel', onUp);
      el.addEventListener('pointerleave', onUp);
      el.addEventListener('keydown', onKey);
      scrim.addEventListener('click', function () { closeDress(); });
      el._ssDestroy = function () {
        alive = false;
        clearTimeout(pressTimer);
        el.removeEventListener('click', onClick);
        el.removeEventListener('contextmenu', onContext);
        el.removeEventListener('pointerdown', onDown);
        el.removeEventListener('pointerup', onUp);
        el.removeEventListener('pointercancel', onUp);
        el.removeEventListener('pointerleave', onUp);
        el.removeEventListener('keydown', onKey);
      };
      load();
    },
    destroy: function (el) {
      if (el._ssDestroy) el._ssDestroy();
    }
  });

  // Exposed for the unit tests.
  window.ChronicleShopStreet = { groupByTown: groupByTown };
})();
