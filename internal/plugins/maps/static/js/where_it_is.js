/**
 * Where it is
 *
 * The entity page's "Where it is" block (internal/plugins/maps/
 * where_is_block.templ) is a row of small map pictures, closest first. This
 * file is the behaviour behind its inline handlers:
 *
 *   - Resting on a card (or keyboard focus) for 250 ms Peeks a slightly bigger
 *     preview (fade and a 4px rise) and outlines the pin spot on the next map.
 *   - A click Grows the enlarged map out of its card, over the page. Nothing
 *     below moves. It has a visible Close, Escape closes it, and a click
 *     outside does not (a panel closes only on purpose). The arrows walk the
 *     ladder; "Open map" is a plain link to the map's page.
 *
 * The server renders every card with its facts as data attributes and decides
 * what a viewer may see (a step players cannot reach is not sent to them), so
 * this file makes no permission decision. Text reaches the DOM only through
 * textContent. Motion follows html[data-motion] (calm fades, off shows the end
 * state) and a device asking for reduced motion is always off.
 *
 * The handlers are inline IIFEs (the block can arrive by an HTMX swap), so this
 * exposes window.ChronicleWhereIt and adds no delegated listener. The one
 * document keydown listener (Escape) lives only while a map is enlarged.
 */
(function () {
  'use strict';

  var REST_MS = 250;
  var GROW_MS = 220;
  var CLOSE_MS = 150;
  var EASE_OUT = 'cubic-bezier(.16,1,.3,1)';
  var EASE_IN = 'cubic-bezier(.4,0,1,1)';
  var DEFAULT_HINT = 'Rest on a map for a peek, or click one to enlarge it.';

  // ---- Pure helpers (unit-tested in test/js/where_it_is.test.mjs) ----

  /** motionOf: how much motion the page allows ('full' | 'calm' | 'off'). */
  function motionOf(attr, reduced) {
    if (reduced) return 'off';
    return attr === 'calm' || attr === 'off' ? attr : 'full';
  }

  /**
   * growKeyframes: the enlarged map growing out of its card (`from`, the
   * card's rect) into place (`to`, its own rect), or the reverse when `back`.
   * Full scales from the card, calm fades, off plays nothing (null).
   */
  function growKeyframes(level, from, to, back) {
    if (level === 'off') return null;
    var fade = [{ opacity: 0 }, { opacity: 1 }];
    var kf;
    if (level === 'calm' || !from || !to || !to.width || !to.height) {
      kf = fade;
    } else {
      var tx = from.left - to.left;
      var ty = from.top - to.top;
      var sx = Math.max(from.width / to.width, 0.05);
      var sy = Math.max(from.height / to.height, 0.05);
      kf = [
        { transform: 'translate(' + tx + 'px,' + ty + 'px) scale(' + sx + ',' + sy + ')', opacity: 0.2 },
        { transform: 'none', opacity: 1 }
      ];
    }
    return back ? kf.slice().reverse() : kf;
  }

  /**
   * peekLeft: where the peek's left edge goes: centred under the card, kept
   * inside the block's width.
   */
  function peekLeft(cardLeft, cardWidth, wrapLeft, wrapWidth, peekWidth) {
    var left = cardLeft - wrapLeft + cardWidth / 2 - peekWidth / 2;
    return Math.max(0, Math.min(left, wrapWidth - peekWidth));
  }

  /** stepIndex: the card an arrow goes to, or -1 at either end of the ladder. */
  function stepIndex(pos, count, dir) {
    var n = pos + dir;
    return n < 0 || n >= count ? -1 : n;
  }

  /** escapeShouldClose: Escape closes, unless a modifier is held. */
  function escapeShouldClose(e) {
    if (!e || e.key !== 'Escape') return false;
    return !(e.ctrlKey || e.metaKey || e.altKey);
  }

  /**
   * panelRows: the facts under the enlarged map's name, from a card's data.
   * `dm` is whether the viewer is the Director (the widest-map line says
   * "you can see" to a player, "in this ladder" to the Director).
   */
  function panelRows(d, entityName, dm) {
    var rows = [];
    rows.push(d.here ? ['This place', entityName + ' is here'] : ['Pin on this map', d.child || '']);
    rows.push(d.next
      ? ['Zooms out to', d.next]
      : ['Reach', dm ? (d.dmNext ? 'Widest map players can see' : 'Widest map in this ladder') : 'Widest map you can see']);
    return rows.filter(function (r) { return r[1]; });
  }

  /** dmLine: the Director's note on how this map is placed in Maps. */
  function dmLine(d) {
    var up = d.dmNext || d.next;
    return up
      ? 'Set in Maps: this map sits under the ' + up + ' pin.'
      : 'Set in Maps: no pin leads up from this map.';
  }

  var api = {
    motionOf: motionOf,
    growKeyframes: growKeyframes,
    peekLeft: peekLeft,
    stepIndex: stepIndex,
    escapeShouldClose: escapeShouldClose,
    panelRows: panelRows,
    dmLine: dmLine,
    REST_MS: REST_MS
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof window === 'undefined' || typeof document === 'undefined') return;

  // ---- Browser part ----

  function level() {
    var reduced = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    return motionOf(document.documentElement.getAttribute('data-motion'), reduced);
  }

  function rootOf(el) { return el && el.closest ? el.closest('.wi') : null; }
  function $(root, sel) { return root.querySelector(sel); }
  function cells(root) { return Array.prototype.slice.call(root.querySelectorAll('.wi-cell')); }
  // Steps in player view (the owner's preview) are display:none; skip them.
  function shown(root) {
    return cells(root).filter(function (c) { return c.offsetParent !== null || c.getClientRects().length > 0; });
  }
  function cardOf(cell) { return cell.querySelector('.wi-mc'); }
  function dataOf(btn) {
    return {
      name: btn.getAttribute('data-name') || '',
      here: btn.getAttribute('data-here') === '1',
      dmStep: btn.getAttribute('data-dm-step') === '1',
      desc: btn.getAttribute('data-desc') || '',
      page: btn.getAttribute('data-page') || '',
      big: btn.getAttribute('data-big') || '',
      child: btn.getAttribute('data-child') || '',
      next: btn.getAttribute('data-next') || '',
      nextPage: btn.getAttribute('data-next-page') || '',
      // Only on a step whose next map is DM-only, and only for the Director;
      // used by the Director's note alone, never by what a player preview shows.
      dmNext: btn.getAttribute('data-dm-next') || '',
      dmNextPage: btn.getAttribute('data-dm-next-page') || ''
    };
  }

  // Per-block state lives on the block element, not in a shared closure, so
  // two blocks on a page do not share an open panel.
  function stateOf(root) {
    if (!root.__wi) root.__wi = { timer: 0, open: null, token: 0, onKey: null, hot: null };
    return root.__wi;
  }

  // The chevrons are as tall as the pictures; the picture's height follows the
  // card's width, so it is measured rather than guessed.
  function syncThumbHeight(root) {
    var t = root.querySelector('.wi-thumb');
    var h = t ? t.getBoundingClientRect().height : 0;
    if (h > 20) root.style.setProperty('--wi-thumb-h', Math.round(h) + 'px');
  }

  function setHint(root, btn) {
    var hint = $(root, '.wi-hint');
    if (!hint) return;
    hint.textContent = '';
    if (!btn) { hint.textContent = DEFAULT_HINT; return; }
    var d = dataOf(btn);
    function bold(t) { var b = document.createElement('b'); b.textContent = t; return b; }
    function text(t) { return document.createTextNode(t); }
    if (d.next) {
      hint.appendChild(bold(d.name));
      hint.appendChild(text(' is at the outlined spot on '));
      hint.appendChild(bold(d.next));
      hint.appendChild(text('.'));
    } else {
      hint.appendChild(bold(d.name));
      var dm = root.getAttribute('data-dm') === '1';
      hint.appendChild(text(' is the widest map ' + (dm ? 'in this ladder' : 'you can see') + '.'));
    }
  }

  // The pin spot of a hovered card and of the next one out are drawn bolder.
  function setHot(root, btn) {
    var st = stateOf(root);
    (st.hot || []).forEach(function (c) { c.classList.remove('wi-hot'); });
    st.hot = [];
    if (btn) {
      var list = shown(root);
      var cell = btn.closest('.wi-cell');
      var i = list.indexOf(cell);
      [list[i], list[i + 1]].forEach(function (c) {
        if (c) { c.classList.add('wi-hot'); st.hot.push(c); }
      });
    }
    setHint(root, btn);
  }

  function hidePeek(root) {
    var st = stateOf(root);
    clearTimeout(st.timer);
    st.timer = 0;
    var p = $(root, '.wi-peek');
    if (p) p.classList.remove('wi-open');
  }

  function fillThumb(into, btn, big) {
    into.textContent = '';
    var thumb = btn.querySelector('.wi-thumb');
    if (!thumb) return;
    var copy = thumb.cloneNode(true);
    var img = copy.querySelector('img');
    if (img && big) img.setAttribute('src', big);
    into.appendChild(copy);
  }

  function showPeek(root, btn) {
    var st = stateOf(root);
    if (st.open) return;
    var d = dataOf(btn);
    var peek = $(root, '.wi-peek');
    var wrap = $(root, '.wi-wrap');
    if (!peek || !wrap) return;
    peek.textContent = '';
    fillThumb(peek, btn, '');
    var tt = document.createElement('div');
    tt.className = 'wi-ptt';
    var b = document.createElement('b');
    b.textContent = d.name;
    tt.appendChild(b);
    if (d.here) {
      var w = document.createElement('span');
      w.className = 'wi-hereword';
      w.textContent = 'You are here';
      tt.appendChild(w);
    }
    peek.appendChild(tt);
    if (d.desc) {
      var p = document.createElement('p');
      p.textContent = d.desc;
      peek.appendChild(p);
    }
    var wr = wrap.getBoundingClientRect();
    var cr = btn.getBoundingClientRect();
    var pw = Math.min(peek.offsetWidth || 252, wr.width);
    peek.style.left = peekLeft(cr.left, cr.width, wr.left, wr.width, pw) + 'px';
    peek.style.top = (cr.bottom - wr.top + 4) + 'px';
    peek.classList.add('wi-open');
  }

  function enter(el) {
    var root = rootOf(el);
    if (!root) return;
    syncThumbHeight(root);
    setHot(root, el);
    hidePeek(root);
    var st = stateOf(root);
    st.timer = setTimeout(function () { showPeek(root, el); }, REST_MS);
  }

  function leave(el) {
    var root = rootOf(el);
    if (!root) return;
    setHot(root, null);
    hidePeek(root);
  }

  // Keyboard focus peeks like a rest does; a pointer press focuses too, but
  // then the click opens the map, so only a focus the keyboard made peeks.
  function focus(el) {
    var root = rootOf(el);
    if (!root) return;
    setHot(root, el);
    hidePeek(root);
    var keyboard = false;
    try { keyboard = el.matches(':focus-visible'); } catch (e) { keyboard = false; }
    if (keyboard) {
      var st = stateOf(root);
      st.timer = setTimeout(function () { showPeek(root, el); }, REST_MS);
    }
  }

  // ---- The enlarged map ----

  function settle(el) {
    var l = level();
    if (l === 'off' || !el || !el.animate) return;
    el.animate(l === 'full' ? [{ opacity: 0, transform: 'translateY(6px)' }, { opacity: 1, transform: 'none' }] : [{ opacity: 0 }, { opacity: 1 }],
      { duration: 200, easing: EASE_OUT });
  }

  function fillPanel(root, btn) {
    var panel = $(root, '.wi-panel');
    var d = dataOf(btn);
    var dm = root.getAttribute('data-dm') === '1';
    fillThumb($(panel, '.wi-pmap'), btn, d.big);
    var pk = $(panel, '.wi-pk');
    pk.textContent = d.here ? 'You are here' : '';
    pk.hidden = !d.here;
    $(panel, '.wi-pn').textContent = d.name;
    var about = $(panel, '.wi-pabout');
    about.textContent = d.desc;
    about.hidden = !d.desc;
    var dl = $(panel, '.wi-pdl');
    dl.textContent = '';
    panelRows(d, root.getAttribute('data-entity-name') || '', dm).forEach(function (r) {
      var row = document.createElement('div');
      var dt = document.createElement('dt');
      var dd = document.createElement('dd');
      dt.textContent = r[0];
      dd.textContent = r[1];
      row.appendChild(dt);
      row.appendChild(dd);
      dl.appendChild(row);
    });
    var dmp = $(panel, '.wi-pdm');
    if (dmp) {
      dmp.hidden = false;
      $(dmp, '.wi-pdm-t').textContent = dmLine(d);
      var a = $(dmp, '.wi-pdm-a');
      var up = d.dmNextPage || d.nextPage;
      a.hidden = !up;
      if (up) a.setAttribute('href', up);
    }
    $(panel, '.wi-open').setAttribute('href', d.page);
    var list = shown(root);
    var i = list.indexOf(btn.closest('.wi-cell'));
    $(panel, '.wi-prev').hidden = stepIndex(i, list.length, -1) < 0;
    $(panel, '.wi-next').hidden = stepIndex(i, list.length, 1) < 0;
  }

  function setExpanded(btn, on) {
    if (btn) btn.setAttribute('aria-expanded', on ? 'true' : 'false');
  }

  function openFor(root, btn) {
    var st = stateOf(root);
    var panel = $(root, '.wi-panel');
    var swap = !!st.open;
    if (swap) setExpanded(st.open, false);
    hidePeek(root);
    st.open = btn;
    st.token++;
    fillPanel(root, btn);
    setExpanded(btn, true);
    var wasHidden = panel.hidden;
    panel.hidden = false;
    var close = $(panel, '.wi-close');
    if (swap && !wasHidden) {
      settle($(panel, '.wi-pmap'));
      settle($(panel, '.wi-pside'));
      close.focus();
      return;
    }
    var kf = growKeyframes(level(), btn.getBoundingClientRect(), panel.getBoundingClientRect(), false);
    if (kf && panel.animate) panel.animate(kf, { duration: GROW_MS, easing: EASE_OUT });
    if (!st.onKey) {
      st.onKey = function (e) {
        if (!root.isConnected) { document.removeEventListener('keydown', st.onKey); st.onKey = null; return; }
        if (st.open && escapeShouldClose(e)) { e.preventDefault(); closePanel(root); }
      };
      document.addEventListener('keydown', st.onKey);
    }
    close.focus();
    if (panel.scrollIntoView) panel.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }

  function closePanel(root, instant) {
    var st = stateOf(root);
    if (!st.open) return;
    var btn = st.open;
    var panel = $(root, '.wi-panel');
    var my = ++st.token;
    st.open = null;
    setExpanded(btn, false);
    if (st.onKey) { document.removeEventListener('keydown', st.onKey); st.onKey = null; }
    function done() {
      if (my !== st.token) return;
      panel.hidden = true;
      if (!instant && btn.isConnected && btn.focus) btn.focus();
    }
    var kf = instant ? null : growKeyframes(level(), btn.getBoundingClientRect(), panel.getBoundingClientRect(), true);
    if (!kf || !panel.animate) { done(); return; }
    var a = panel.animate(kf, { duration: CLOSE_MS, easing: EASE_IN, fill: 'forwards' });
    a.onfinish = function () { done(); try { a.cancel(); } catch (e) { /* already gone */ } };
  }

  function toggle(el) {
    var root = rootOf(el);
    if (!root) return;
    var st = stateOf(root);
    if (st.open === el) { closePanel(root); return; }
    openFor(root, el);
  }

  function close(el) {
    var root = rootOf(el);
    if (root) closePanel(root);
  }

  function walk(el, dir) {
    var root = rootOf(el);
    if (!root) return;
    var st = stateOf(root);
    if (!st.open) return;
    var list = shown(root);
    var n = stepIndex(list.indexOf(st.open.closest('.wi-cell')), list.length, dir);
    if (n < 0) return;
    var btn = cardOf(list[n]);
    if (btn.scrollIntoView) btn.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    openFor(root, btn);
  }

  window.ChronicleWhereIt = { toggle: toggle, close: close, walk: walk, enter: enter, leave: leave, focus: focus };

  // The block mounts as a widget only so its script loads on sight (ADR-063);
  // every handler is inline.
  if (window.Chronicle && window.Chronicle.register) {
    window.Chronicle.register('where-it-is', {
      init: function (el) { syncThumbHeight(el); },
      destroy: function (el) { var r = rootOf(el) || el; if (r.__wi && r.__wi.onKey) document.removeEventListener('keydown', r.__wi.onKey); }
    });
  }
})();
