/**
 * hovercard.js -- Chronicle's one hover card.
 *
 * Every hover-over that previews content (a linked page, a rule word, a
 * creature, a game-system entry) opens this card, so they all behave and
 * look alike. Callers describe the content; the card owns behaviour and look.
 *
 *   Chronicle.hovercard.bind(root, selector, contentFn, opts) -> unbind()
 *     contentFn(trigger) returns a content object, a Promise of one, or null.
 *     opts.pinOnClick: clicking the trigger pins the card instead of
 *     following it (rule words). Links leave it off so a click still navigates.
 *     opts.corner: draw the card as the fading corner (time lines), its corner
 *     on a short stem over the trigger, in place of the campaign's look.
 *   Chronicle.hovercard.open(trigger, content, {pin}) / .close() / .update(content)
 *   Chronicle.hovercard.html(content) -> markup, for previews (Customize).
 *   Chronicle.hovercard.look() -> the campaign's look.
 *
 * Content: { kind, kindIcon, title, locked, pic, badge, facts:[[label,value]],
 * text, extraHTML, foot, link:{href,label} }. badge is a second, round picture
 * on the corner of pic (a creature's token beside its art). Everything but extraHTML is
 * escaped here; extraHTML must already be escaped by its caller.
 *
 * Behaviour (the UI standard's hover rules): opens after the pointer rests
 * 250ms; stays open while the pointer is on the card; click or tap pins it
 * with a close button; keyboard focus opens it and Escape closes it; one
 * card at a time; kept inside the window, flipping above near the bottom.
 * The fading corner sits above its trigger and flips below near the top of
 * the window, and to the trigger's left near the right edge.
 */
(function () {
  'use strict';

  var C = window.Chronicle = window.Chronicle || {};
  if (C.hovercard) return;

  var OPEN_DELAY = 250;
  var CLOSE_DELAY = 180;
  var LONG_PRESS = 500;
  var LOOKS = { paper: 1, plain: 1, night: 1, compact: 1 };
  // The fading corner's stem, and the room it leaves for a time line bead
  // grown on hover, from the trigger's centre to the corner.
  var STEM = 7;
  var CORNER_OFF = 8 + STEM;

  var card = null, inner = null;
  var current = null;   // trigger the card belongs to
  var pinned = false;
  var corner = false;   // the card is drawn as the fading corner
  var restoringFocus = false; // set while Escape hands focus back to the trigger
  var openTimer = 0, closeTimer = 0;
  var token = 0;        // guards against a slow contentFn answering for an old trigger

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function look() {
    var v = document.documentElement.getAttribute('data-cz-hover');
    return LOOKS[v] ? v : 'paper';
  }

  // Only same-site or http(s) links may be followed from a card.
  function safeHref(h) {
    h = String(h || '');
    return /^(\/(?![\/\\])|https?:\/\/|#)/i.test(h) ? h : '';
  }

  function html(c, idPrefix) {
    if (!c) return '';
    var title = '<h3 class="chc__title"' + (idPrefix ? ' id="' + idPrefix + '-t"' : '') + '>' + esc(c.title) +
      (c.locked ? ' <i class="fa-solid fa-lock chc__lock" aria-label="Private"></i>' : '') + '</h3>';
    var facts = '';
    if (c.facts && c.facts.length) {
      facts = '<dl class="chc__facts">' + c.facts.map(function (f) {
        return '<dt>' + esc(f[0]) + '</dt><dd title="' + esc(f[1]) + '">' + esc(f[1]) + '</dd>';
      }).join('') + '</dl>';
    }
    var h = '<button type="button" class="chc__x" aria-label="Close">&times;</button>';
    if (c.kind) h += '<span class="chc__kind">' + (c.kindIcon ? '<i class="fa-solid ' + esc(c.kindIcon) + '" aria-hidden="true"></i>' : '') + esc(c.kind) + '</span>';
    var pic = '<img class="chc__pic" src="' + esc(c.pic) + '" alt="">';
    // A badge gets a frame for the art so the art can settle inside it while
    // the badge sits over the frame's corner.
    if (c.pic && c.badge) pic = '<span class="chc__picw"><span class="chc__frame">' + pic + '</span><img class="chc__badge" src="' + esc(c.badge) + '" alt=""></span>';
    if (c.pic) h += '<div class="chc__top">' + pic + '<div>' + title + facts + '</div></div>';
    else h += title + facts;
    if (c.extraHTML) h += '<div class="chc__extra">' + c.extraHTML + '</div>';
    if (c.text) h += '<p class="chc__text">' + esc(c.text) + '</p>';
    if (c.loading) h += '<p class="chc__loading">Loading…</p>';
    var href = c.link && safeHref(c.link.href);
    // c.peek: a same-site peek address. Phones have no hover or Shift, so the
    // card (opened by a long press there) is where they reach the side panel.
    // The address rides in a data attribute so the inline handler needs no escaping.
    var peek = c.peek && C.peekAddress ? C.peekAddress(c.peek) : '';
    var peekBtn = peek ? '<button type="button" class="chc__peek" data-peek="' + esc(peek) + '" onclick="(function(b){if(window.Chronicle&amp;&amp;Chronicle.peek){Chronicle.peek.open(b.getAttribute(\'data-peek\'));Chronicle.hovercard.close();}})(this)">Peek</button>' : '';
    if (c.foot || href || peekBtn) {
      h += '<div class="chc__foot">' + (c.foot ? '<span>' + esc(c.foot) + '</span>' : '') + peekBtn +
        (href ? '<a class="chc__link" href="' + esc(href) + '">' + esc(c.link.label || 'Open') + ' &rarr;</a>' : '') + '</div>';
    }
    return h;
  }

  function ensure() {
    if (card) return card;
    card = document.createElement('div');
    card.className = 'chc';
    card.id = 'chc';
    card.setAttribute('role', 'dialog');
    card.hidden = true;
    var stem = document.createElement('i');
    stem.className = 'chc__stem';
    stem.setAttribute('aria-hidden', 'true');
    card.appendChild(stem);
    inner = document.createElement('div');
    inner.className = 'chc__in';
    card.appendChild(inner);
    document.body.appendChild(card);
    card.addEventListener('pointerenter', function () { clearTimeout(closeTimer); });
    card.addEventListener('pointerleave', function (e) { if (e.pointerType !== 'touch') soonClose(); });
    card.addEventListener('focusout', function (e) {
      if (!pinned && !card.contains(e.relatedTarget) && e.relatedTarget !== current) soonClose();
    });
    card.addEventListener('click', function (e) {
      e.stopPropagation();
      if (e.target.closest('.chc__x')) {
        var t = current;
        close();
        restoringFocus = true; if (t && t.focus) t.focus(); restoringFocus = false;
      }
    });
    return card;
  }

  function place() {
    if (!card || !current || !current.isConnected) return;
    var r = current.getBoundingClientRect();
    var w = card.offsetWidth, h = card.offsetHeight, vw = window.innerWidth, vh = window.innerHeight, gap = 8;
    if (corner) { placeCorner(r, w, h, vw); return; }
    var left = Math.min(Math.max(8, r.left), Math.max(8, vw - w - 8));
    var top = r.bottom + gap, up = false;
    if (top + h > vh - 8 && r.top - gap - h > 8) { top = r.top - gap - h; up = true; }
    card.style.left = Math.round(left) + 'px';
    card.style.top = Math.round(top) + 'px';
    card.classList.toggle('chc--up', up);
  }

  // The corner's edge is centred on the trigger, so the stem meets the bead
  // in its middle; its 1.5px edge is why the offset is .75. When neither side
  // has room (a phone), the card narrows to the roomier side and wraps.
  function placeCorner(r, w, h, vw) {
    var cx = r.left + r.width / 2, cy = r.top + r.height / 2, roomR = vw - 8 - cx, roomL = cx - 8;
    if (w > roomR && w > roomL) {
      card.style.maxWidth = Math.max(roomR, roomL) + 'px';
      w = card.offsetWidth; h = card.offsetHeight;
    }
    var right = w > roomR, down = cy - CORNER_OFF - h < 8;
    card.style.left = (right ? cx + 0.75 - w : cx - 0.75).toFixed(2) + 'px';
    card.style.top = Math.round(down ? cy + CORNER_OFF : cy - CORNER_OFF - h) + 'px';
    card.classList.toggle('chc--cr', right);
    card.classList.toggle('chc--cd', down);
    card.classList.toggle('chc--up', !down);
  }

  function render(c) {
    inner.innerHTML = html(c, 'chc');
    if (c && c.title) card.setAttribute('aria-labelledby', 'chc-t');
    else card.removeAttribute('aria-labelledby');
  }

  function open(trigger, content, opts) {
    ensure();
    clearTimeout(openTimer);
    clearTimeout(closeTimer);
    if (current && current !== trigger) detachTrigger(current);
    current = trigger;
    pinned = !!(opts && opts.pin);
    corner = !!(opts && opts.corner);
    card.dataset.look = corner ? 'corner' : look();
    card.style.maxWidth = '';
    if (!corner) card.classList.remove('chc--cr', 'chc--cd');
    card.classList.toggle('chc--pinned', pinned);
    trigger.setAttribute('aria-expanded', 'true');
    trigger.setAttribute('aria-controls', 'chc');
    render(content);
    card.hidden = false;
    place();
    requestAnimationFrame(function () { if (current === trigger) card.classList.add('chc--show'); });
  }

  function update(content) {
    if (!card || !current) return;
    render(content);
    place();
  }

  function detachTrigger(t) {
    t.setAttribute('aria-expanded', 'false');
  }

  function close() {
    clearTimeout(openTimer);
    clearTimeout(closeTimer);
    token++;
    if (!card) return;
    card.classList.remove('chc--show', 'chc--pinned');
    if (current) detachTrigger(current);
    current = null;
    pinned = false;
    setTimeout(function () { if (card && !current) card.hidden = true; }, 160);
  }

  function soonClose() {
    if (pinned) return;
    clearTimeout(closeTimer);
    closeTimer = setTimeout(close, CLOSE_DELAY);
  }

  // Resolve the caller's content (value or Promise) and show it, with a
  // loading line while a fetch is in flight. A null answer closes the card.
  function show(trigger, contentFn, pin, cornered) {
    var my = ++token;
    var res, o = { pin: pin, corner: cornered };
    try { res = contentFn(trigger); } catch (e) { res = null; }
    if (res && typeof res.then === 'function') {
      open(trigger, { title: trigger.textContent.trim(), loading: true }, o);
      res.then(function (c) {
        if (my !== token) return;
        if (c) update(c); else close();
      }, function () { if (my === token) close(); });
    } else if (res) {
      open(trigger, res, o);
    }
  }

  function bind(root, selector, contentFn, opts) {
    root = root || document;
    opts = opts || {};
    var pressTimer = 0, pressed = false;
    function find(e) { return e.target && e.target.closest ? e.target.closest(selector) : null; }
    function within(t) { return t && (root === document || root.contains(t)); }

    function onOver(e) {
      if (e.pointerType === 'touch') return;
      var t = find(e);
      if (!within(t) || (pinned && current)) return;
      if (t === current) { clearTimeout(closeTimer); return; }
      clearTimeout(openTimer);
      openTimer = setTimeout(function () { show(t, contentFn, false, !!opts.corner); }, OPEN_DELAY);
    }
    function onOut(e) {
      if (e.pointerType === 'touch') return;
      var t = find(e);
      if (!within(t)) return;
      if (t.contains(e.relatedTarget)) return;
      clearTimeout(openTimer);
      if (t === current) soonClose();
    }
    function onFocusIn(e) {
      if (restoringFocus) return;
      var t = find(e);
      if (within(t) && !(pinned && current) && t !== current) show(t, contentFn, false, !!opts.corner);
    }
    function onFocusOut(e) {
      var t = find(e);
      if (!within(t) || t !== current) return;
      if (card && card.contains(e.relatedTarget)) return;
      soonClose();
    }
    function onClick(e) {
      var t = find(e);
      if (!within(t)) return;
      if (pressed) { pressed = false; e.preventDefault(); return; }
      if (!opts.pinOnClick) return;
      e.preventDefault();
      e.stopPropagation();
      if (t === current && pinned) { close(); return; }
      show(t, contentFn, true, !!opts.corner);
    }
    function onKey(e) {
      if (!opts.pinOnClick || (e.key !== 'Enter' && e.key !== ' ')) return;
      var t = find(e);
      if (!within(t) || t.tagName === 'A' || t.tagName === 'BUTTON') return;
      e.preventDefault();
      if (t === current && pinned) close(); else show(t, contentFn, true, !!opts.corner);
    }
    // On touch, links open pinned on a long press so a tap still follows them.
    function onTouchStart(e) {
      var t = find(e);
      if (!within(t) || opts.pinOnClick) return;
      clearTimeout(pressTimer);
      pressed = false;
      pressTimer = setTimeout(function () { pressed = true; show(t, contentFn, true, !!opts.corner); }, LONG_PRESS);
    }
    function cancelPress() { clearTimeout(pressTimer); }

    var on = [
      ['pointerover', onOver], ['pointerout', onOut], ['focusin', onFocusIn], ['focusout', onFocusOut],
      ['click', onClick, true], ['keydown', onKey], ['touchstart', onTouchStart, { passive: true }],
      ['touchend', cancelPress], ['touchmove', cancelPress, { passive: true }]
    ];
    on.forEach(function (h) { root.addEventListener(h[0], h[1], h[2]); });
    return function unbind() {
      on.forEach(function (h) { root.removeEventListener(h[0], h[1], h[2]); });
      if (current && within(current)) close();
    };
  }

  // Shared dismissals: click elsewhere, Escape, scrolling (a pinned card follows).
  document.addEventListener('click', function (e) {
    if (current && card && !card.contains(e.target) && !current.contains(e.target)) close();
  });
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && current) {
      var t = current;
      // This Escape belongs to the card; a peek panel open beside it waits
      // for the next one.
      e.preventDefault();
      close();
      // Returning focus must not open the card again.
      restoringFocus = true;
      if (t.focus) t.focus();
      restoringFocus = false;
    }
  });
  window.addEventListener('scroll', function () {
    if (!current) return;
    if (pinned) place(); else close();
  }, true);
  window.addEventListener('resize', function () { if (current) place(); });
  // An HTMX swap can remove the trigger; never leave its card floating.
  document.addEventListener('htmx:afterSwap', function () { if (current && !current.isConnected) close(); });

  C.hovercard = {
    bind: bind,
    open: function (trigger, content, opts) { open(trigger, content, opts); },
    update: update,
    close: close,
    html: function (c) { return html(c, ''); },
    look: look,
    current: function () { return current; }
  };
})();
