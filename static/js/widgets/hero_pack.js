/**
 * hero_pack.js -- the containers in a character's "Items and money" panel.
 *
 * The server draws each item as a tile in the container the hero carries it
 * in (internal/plugins/armory/character_panel.templ). Pressing a tile opens
 * its card under the boxes: the copies that remember something, spread out,
 * with Move, Share and where it is carried. When the viewer may rearrange
 * (the boxes carry data-place-url), a tile can be dragged to another box, or
 * sent with the card's "Carried" buttons, which work from the keyboard and on
 * touch. The tile moves at once and the server is told; a refusal puts it
 * back. The panel reloads itself after a move or a give (armory-moved), so
 * everything here is delegated from the document and keeps no state. The
 * script loads on the first page that mounts the boxes (ADR-063); the
 * listeners go on once, and init has nothing left to do per mount.
 */
(function () {
  'use strict';

  if (window.Chronicle && Chronicle.register) Chronicle.register('hero-pack', { init: function () {} });
  if (window.__heroPackBound) return;
  window.__heroPackBound = true;

  function notify(msg) { if (window.Chronicle && Chronicle.notify) Chronicle.notify(msg, 'error'); }

  function panelOf(el) { return el.closest('section'); }

  // toggle opens the tile's card, shutting any other card in the same panel.
  function toggle(tile, focusCard) {
    var panel = panelOf(tile);
    if (!panel) return;
    var id = tile.getAttribute('data-item-id');
    var open = tile.getAttribute('aria-expanded') === 'true';
    panel.querySelectorAll('[data-hp-tile][aria-expanded="true"]').forEach(function (t) { t.setAttribute('aria-expanded', 'false'); });
    panel.querySelectorAll('[data-hp-detail]').forEach(function (d) { d.hidden = true; });
    if (open) return;
    var card = panel.querySelector('[data-hp-detail="' + CSS.escape(id) + '"]');
    if (!card) return;
    tile.setAttribute('aria-expanded', 'true');
    card.hidden = false;
    if (focusCard) {
      var first = card.querySelector('button');
      if (first) first.focus({ preventScroll: true });
    }
  }

  // recount writes each box's item count and shows "Nothing here" when empty.
  function recount(panel) {
    panel.querySelectorAll('[data-hp-box]').forEach(function (box) {
      var n = 0;
      box.querySelectorAll('[data-hp-tile]').forEach(function (t) {
        var ct = t.querySelector('.hp-ct');
        n += ct ? parseInt(ct.textContent, 10) || 1 : 1;
      });
      var label = box.querySelector('[data-hp-n]');
      if (label) label.textContent = n ? (n === 1 ? '1 item' : n + ' items') : '';
      var empty = box.querySelector('[data-hp-empty]');
      if (empty) empty.hidden = n > 0;
    });
  }

  // place puts the tile in another box, then tells the server.
  function place(panel, id, where) {
    var boxes = panel.querySelector('[data-hero-pack]');
    var base = boxes && boxes.getAttribute('data-place-url');
    if (!base) return;
    var tile = panel.querySelector('[data-hp-tile][data-item-id="' + CSS.escape(id) + '"]');
    var dest = panel.querySelector('[data-hp-box="' + CSS.escape(where) + '"] [data-hp-tiles]');
    if (!tile || !dest || tile.parentNode === dest) return;
    var from = tile.parentNode, next = tile.nextSibling;
    dest.appendChild(tile);
    mark(panel, id, where);
    recount(panel);
    var fd = new FormData();
    fd.append('place', where);
    Chronicle.apiFetch(base + encodeURIComponent(id) + '/place', { method: 'POST', body: fd })
      .then(function (res) {
        if (res.ok) return;
        return res.json().then(function (b) { throw new Error(b && b.message); }, function () { throw new Error(''); });
      })
      .catch(function (err) {
        from.insertBefore(tile, next);
        var back = from.closest('[data-hp-box]');
        mark(panel, id, back ? back.getAttribute('data-hp-box') : '');
        recount(panel);
        notify((err && err.message) || 'That didn’t move. Try again.');
      });
  }

  // mark presses the card's "Carried" button for the item's box.
  function mark(panel, id, where) {
    var card = panel.querySelector('[data-hp-detail="' + CSS.escape(id) + '"]');
    if (!card) return;
    card.querySelectorAll('[data-hp-carry]').forEach(function (b) {
      b.setAttribute('aria-pressed', b.getAttribute('data-hp-carry') === where ? 'true' : 'false');
    });
  }

  document.addEventListener('click', function (e) {
    var t = e.target.closest && e.target.closest('[data-hp-tile]');
    if (t) { toggle(t, false); return; }
    var x = e.target.closest && e.target.closest('[data-hp-close]');
    if (x) {
      var card = x.closest('[data-hp-detail]'), panel = panelOf(x);
      if (!card || !panel) return;
      var tile = panel.querySelector('[data-hp-tile][data-item-id="' + CSS.escape(card.getAttribute('data-hp-detail')) + '"]');
      if (tile) { toggle(tile, false); tile.focus({ preventScroll: true }); }
      return;
    }
    var c = e.target.closest && e.target.closest('[data-hp-carry]');
    if (c) {
      var d = c.closest('[data-hp-detail]');
      if (d) place(panelOf(c), d.getAttribute('data-hp-detail'), c.getAttribute('data-hp-carry'));
    }
  });

  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    var t = e.target.closest && e.target.closest('[data-hp-tile]');
    if (!t || e.target !== t) return;
    e.preventDefault();
    toggle(t, true);
  });

  var dragging = null;
  document.addEventListener('dragstart', function (e) {
    var t = e.target.closest && e.target.closest('[data-hp-tile][draggable="true"]');
    if (!t) return;
    dragging = t;
    t.classList.add('is-dragging');
    try { e.dataTransfer.setData('text/plain', t.getAttribute('data-item-id')); e.dataTransfer.effectAllowed = 'move'; } catch (err) {}
  });
  document.addEventListener('dragend', function () {
    if (dragging) dragging.classList.remove('is-dragging');
    dragging = null;
    document.querySelectorAll('.hp-box.is-over').forEach(function (b) { b.classList.remove('is-over'); });
  });
  document.addEventListener('dragover', function (e) {
    if (!dragging) return;
    var box = e.target.closest && e.target.closest('[data-hp-box]');
    if (!box || panelOf(box) !== panelOf(dragging)) return;
    e.preventDefault();
    panelOf(box).querySelectorAll('.hp-box').forEach(function (b) { b.classList.toggle('is-over', b === box); });
  });
  document.addEventListener('drop', function (e) {
    if (!dragging) return;
    var box = e.target.closest && e.target.closest('[data-hp-box]');
    if (!box || panelOf(box) !== panelOf(dragging)) return;
    e.preventDefault();
    box.classList.remove('is-over');
    place(panelOf(box), dragging.getAttribute('data-item-id'), box.getAttribute('data-hp-box'));
  });
})();
