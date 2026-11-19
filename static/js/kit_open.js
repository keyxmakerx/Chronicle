/**
 * kit_open.js — the one helper behind a style kit's opening (ui-standard.md,
 * "Style kits"): the move a card or panel makes when it comes out of
 * something clicked, and back into it on the way out.
 *
 * A component opts in by carrying data-kit-open and showing and hiding itself
 * with the hidden attribute, as it already does. This helper watches that
 * attribute, finds what was clicked, tells the card where it opened from
 * (--ko-x, --ko-y, --ko-s, --ko-ey and data-ko-edge) and marks the move
 * (data-ko="in" or "out"); the moves themselves are in style_kits.css. On
 * the way out the card stays shown while the move plays backwards, then goes
 * hidden. The same file runs the Save spark Moment. Without a kit (data-cz-kit on <html>) or an owner's entrance
 * (data-cz-entrance, Material only) or with motion Off, nothing here runs and
 * the component keeps its own move. data-kit-open="card" marks a card or
 * panel, which also takes the owner's entrance; a menu or picker carries the
 * bare attribute and keeps its quick move.
 */
(function () {
  'use strict';
  var C = (window.Chronicle = window.Chronicle || {});
  if (C.kitOpen) return;

  var root = document.documentElement;
  var last = null, lastAt = 0;
  var mine = new WeakMap(); // card -> how many of its own hidden changes to skip
  var CLOSE_MS = 400;       // never leave a card shown longer than this

  function clamp(v, a, b) { return Math.max(a, Math.min(b, v)); }

  // What was clicked or pressed most recently, for the card to open from.
  function note(e) { last = e.target; lastAt = Date.now(); }
  document.addEventListener('pointerdown', note, true);
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Enter' || e.key === ' ') note(e);
  }, true);

  // Where, inside the card's box, the card opened from, and which edge of the
  // card that is nearest. With nothing recent to open from it opens from the
  // middle of its top edge.
  function origin(trigger, card) {
    var c = card.getBoundingClientRect(), t = trigger && trigger.isConnected ? trigger.getBoundingClientRect() : null;
    var cx = t ? t.left + t.width / 2 : c.left + c.width / 2;
    var cy = t ? t.top + t.height / 2 : c.top;
    var below = cy > c.top + c.height / 2;
    return {
      x: clamp(cx - c.left, 0, c.width),
      y: clamp(cy - c.top, 0, c.height),
      edge: below ? 'bottom' : 'top',
    };
  }
  C.kitOpen = { origin: origin };

  function level() { return root.getAttribute('data-motion') || 'full'; }

  function opened(card) {
    if (level() === 'off') return;
    var trigger = last && Date.now() - lastAt < 1500
      ? (last.closest ? last.closest('button, a, summary, [role="button"], [aria-haspopup]') || last : last) : null;
    var o = origin(trigger, card), s = card.style;
    s.setProperty('--ko-x', o.x + 'px');
    s.setProperty('--ko-y', o.y + 'px');
    s.setProperty('--ko-s', o.edge === 'top' ? '1' : '-1');
    s.setProperty('--ko-ey', o.edge === 'top' ? '0%' : '100%');
    card.setAttribute('data-ko-edge', o.edge);
    card.setAttribute('data-ko', 'in');
  }

  function skip(card) { mine.set(card, (mine.get(card) || 0) + 1); }

  function closed(card) {
    if (card.getAttribute('data-ko') === 'out') return; // already on its way out
    if (level() === 'off' || card.getAttribute('data-ko') !== 'in') { card.removeAttribute('data-ko'); return; }
    skip(card); card.hidden = false;
    card.setAttribute('data-ko', 'out');
    var finish = function () {
      if (card.getAttribute('data-ko') !== 'out') return;
      card.removeAttribute('data-ko');
      skip(card); card.hidden = true;
    };
    card.addEventListener('animationend', finish, { once: true });
    setTimeout(finish, CLOSE_MS);
  }

  function watch(records) {
    if (!root.hasAttribute('data-cz-kit') && !root.hasAttribute('data-cz-entrance')) return;
    for (var i = 0; i < records.length; i++) {
      var card = records[i].target;
      if (!card.hasAttribute || !card.hasAttribute('data-kit-open')) continue;
      var n = mine.get(card) || 0;
      if (n > 0) { mine.set(card, n - 1); continue; }
      if (card.hidden) closed(card); else opened(card);
    }
  }

  // Save spark (a Moment, off until the owner turns it on): seven dots in the
  // accent radiate from whatever was pressed when a save lands. A save is a
  // successful htmx request that is not a read. Calm and Off never get it.
  function spark(e) {
    var moments = root.getAttribute('data-cz-moments') || '';
    if (moments.split(' ').indexOf('spark') < 0 || root.hasAttribute('data-motion')) return;
    var d = e.detail || {}, cfg = d.requestConfig || {};
    if (!d.successful || !cfg.verb || cfg.verb === 'get') return;
    var el = last && last.isConnected && Date.now() - lastAt < 3000 ? last : null;
    if (!el || !el.getBoundingClientRect) return;
    var r = el.getBoundingClientRect(), x = r.left + r.width / 2, y = r.top + r.height / 2;
    var host = document.createElement('div');
    host.className = 'mo-spark'; host.setAttribute('aria-hidden', 'true');
    host.style.left = x + 'px'; host.style.top = y + 'px';
    for (var i = 0; i < 7; i++) {
      var dot = document.createElement('i'), a = (i / 7) * 2 * Math.PI;
      dot.style.setProperty('--mx', Math.round(Math.cos(a) * 26) + 'px');
      dot.style.setProperty('--my', Math.round(Math.sin(a) * 26) + 'px');
      host.appendChild(dot);
    }
    document.body.appendChild(host);
    setTimeout(function () { if (host.parentNode) host.parentNode.removeChild(host); }, 800);
  }
  document.addEventListener('htmx:afterRequest', spark);

  function start() {
    new MutationObserver(watch).observe(document.body, { subtree: true, attributes: true, attributeFilter: ['hidden'] });
  }
  if (document.body) start(); else document.addEventListener('DOMContentLoaded', start);
})();
