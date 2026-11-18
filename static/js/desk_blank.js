/**
 * desk_blank.js -- the dashed words on the owner's desk (Manage's home page).
 *
 * A "blank" is a word in a sentence that the owner changes where it stands and
 * that saves the moment it is changed:
 *
 *   choice  opens a small menu that grows out of the word; a pick saves at once
 *   name    turns the word into a field; Enter or leaving it saves, Escape keeps
 *
 * Each blank's button carries its own inline onclick (built in
 * campaigns/desk_view.go) that calls Chronicle.deskBlank.activate, because the
 * desk arrives by a boosted swap where a script tag or a delegated listener may
 * never run. Nothing here listens on document except while a menu is open, and
 * that one goes away when the menu does.
 *
 * Keyboard: Enter or Space opens the menu with focus on the chosen row, the
 * arrow keys, Home and End move, Enter or Space picks, Escape closes and puts
 * focus back on the word, Tab closes and moves on. A change shows at once
 * (the word, a working state), and is undone with an error if the save fails.
 *
 * Motion reads html[data-motion]: full grows the menu out of its word, Calm
 * fades it, Off shows it. Only transform and opacity ever move.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var current = null; // the one open menu: { el, btn, off }

  var CHECK = '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" focusable="false"><path d="M3 8.5l3.2 3L13 4.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';

  function level() {
    var m = document.documentElement.getAttribute('data-motion');
    return m === 'off' ? 'off' : m === 'calm' ? 'calm' : 'full';
  }

  // A duration or curve from the page's tokens, so the owner's Motion speed
  // reaches it; the fallback only matters where the stylesheet is absent.
  function token(name, fallback) {
    try {
      var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
      return v || fallback;
    } catch (e) { return fallback; }
  }
  function ms(name, fallback) {
    var v = parseFloat(token(name, ''));
    return isFinite(v) && v >= 0 ? v : fallback;
  }

  // Runs a Web Animations keyframe pair under the current motion level and
  // resolves when it ends. Off resolves at once; Calm keeps only the opacity.
  function play(el, from, to, dur, ease) {
    var lv = level();
    if (lv === 'off' || !el.animate) return Promise.resolve();
    var a = { opacity: from.opacity }, b = { opacity: to.opacity };
    if (lv === 'full' && from.transform) { a.transform = from.transform; b.transform = to.transform; }
    try {
      var anim = el.animate([a, b], { duration: dur, easing: ease, fill: 'both' });
      return anim.finished.then(function () { try { anim.cancel(); } catch (e) { /* gone */ } }, function () { /* cancelled */ });
    } catch (e) { return Promise.resolve(); }
  }

  function options(btn) {
    try { return JSON.parse(btn.getAttribute('data-options') || '[]'); } catch (e) { return []; }
  }

  function setShown(btn, text) {
    btn.textContent = text;
    btn.setAttribute('aria-label', (btn.getAttribute('data-label') || '') + ': ' + text);
  }

  // ---- the menu ----------------------------------------------------------

  function closeMenu(opts) {
    if (!current) return;
    var m = current;
    current = null;
    document.removeEventListener('pointerdown', m.off, true);
    if (m.btn.isConnected) m.btn.setAttribute('aria-expanded', 'false');
    if (opts && opts.now) { m.el.remove(); return; }
    play(m.el, { opacity: 1 }, { opacity: 0 }, ms('--dur-everyday-out', 80), token('--ease-in', 'ease-in')).then(function () { m.el.remove(); });
  }

  function place(menu, btn, host) {
    var h = host.getBoundingClientRect(), b = btn.getBoundingClientRect();
    var below = window.innerHeight - b.bottom, up = below < menu.offsetHeight + 12 && b.top > menu.offsetHeight + 12;
    var left = Math.max(0, Math.min(b.left - h.left, h.width - menu.offsetWidth));
    menu.style.left = left + 'px';
    menu.style.top = (up ? b.top - h.top - menu.offsetHeight - 4 : b.bottom - h.top + 4) + 'px';
    menu.style.transformOrigin = (up ? 'bottom' : 'top') + ' left';
  }

  function openMenu(btn, byKey) {
    closeMenu({ now: true });
    var host = btn.closest('[data-desk]') || document.body;
    var menu = document.createElement('div');
    menu.className = 'desk-menu';
    menu.setAttribute('role', 'menu');
    menu.setAttribute('aria-label', btn.getAttribute('data-label') || '');
    var value = btn.getAttribute('data-value');
    options(btn).forEach(function (o) {
      var it = document.createElement('button');
      it.type = 'button';
      it.setAttribute('role', 'menuitemradio');
      it.setAttribute('aria-checked', String(o.v === value));
      it.setAttribute('data-v', o.v);
      it.innerHTML = '<span></span>' + CHECK;
      it.firstChild.textContent = o.l;
      menu.appendChild(it);
    });
    host.appendChild(menu);
    place(menu, btn, host);
    btn.setAttribute('aria-expanded', 'true');
    current = {
      el: menu, btn: btn,
      // A click off the menu closes it and loses nothing.
      off: function (e) { if (!menu.contains(e.target) && e.target !== btn) closeMenu(); }
    };
    document.addEventListener('pointerdown', current.off, true);
    menu.addEventListener('click', function (e) {
      var it = e.target.closest('[role=menuitemradio]');
      if (it) choose(btn, it.getAttribute('data-v'));
    });
    menu.addEventListener('keydown', function (e) { menuKey(e, menu, btn); });

    var items = [].slice.call(menu.children);
    if (byKey) {
      // From the keyboard it appears at once, with focus where the choice is.
      (menu.querySelector('[aria-checked=true]') || items[0]).focus();
    } else {
      play(menu, { opacity: 0, transform: 'scale(0.94)' }, { opacity: 1, transform: 'none' }, ms('--dur-everyday', 120), token('--ease-out', 'ease-out'));
    }
  }

  function menuKey(e, menu, btn) {
    var items = [].slice.call(menu.querySelectorAll('[role=menuitemradio]'));
    var i = items.indexOf(document.activeElement);
    var k = e.key;
    if (k === 'ArrowDown' || k === 'ArrowUp') {
      items[(i + (k === 'ArrowDown' ? 1 : -1) + items.length) % items.length].focus();
    } else if (k === 'Home') {
      items[0].focus();
    } else if (k === 'End') {
      items[items.length - 1].focus();
    } else if (k === 'Escape') {
      closeMenu();
      btn.focus();
    } else if (k === 'Tab') {
      // Focus goes back to the word first, so Tab carries on from there
      // rather than from the end of the page.
      closeMenu({ now: true });
      btn.focus();
      return;
    } else if (k === ' ' || k === 'Enter') {
      // The row is a real button, so its own click fires; stop the page's
      // scroll on Space.
      if (k === ' ') e.preventDefault();
      if (document.activeElement && document.activeElement.click) document.activeElement.click();
    } else {
      return;
    }
    e.preventDefault();
    e.stopPropagation();
  }

  // ---- saving ------------------------------------------------------------

  function bodyFor(btn, value) {
    var b = {};
    b[btn.getAttribute('data-field')] = btn.hasAttribute('data-bool') ? value === 'true' : value;
    return b;
  }

  // The People role route answers a redirect when it worked and a page that
  // says why when it did not, always with a 200 for the latter. So the save
  // is read from the redirect, and the reason is taken from that page.
  function sendForRedirect(url, body, method) {
    var h = { 'Content-Type': 'application/json', Accept: 'text/html' };
    var csrf = C.getCsrf && C.getCsrf();
    if (csrf) h['X-CSRF-Token'] = csrf;
    return fetch(url, { method: method, headers: h, credentials: 'same-origin', redirect: 'manual', body: JSON.stringify(body) }).then(function (r) {
      if (r.type === 'opaqueredirect') return {};
      if (r.status === 200) {
        return r.text().then(function (t) {
          var why = '';
          try { var a = new DOMParser().parseFromString(t, 'text/html').querySelector('[role=alert]'); why = a ? a.textContent.trim() : ''; } catch (e) { why = ''; }
          throw new Error(why || 'The change was refused.');
        });
      }
      throw new Error('request failed (' + r.status + ')');
    });
  }

  function send(btn, value) {
    var opts = { method: btn.getAttribute('data-method') || 'PUT', body: bodyFor(btn, value) };
    var url = btn.getAttribute('data-url');
    if (btn.getAttribute('data-expect') === 'redirect') return sendForRedirect(url, opts.body, opts.method);
    if (C.apiAction) return C.apiAction(url, opts);
    return C.apiFetch(url, opts).then(function (r) {
      return r.text().then(function (t) {
        var j = {};
        try { j = t ? JSON.parse(t) : {}; } catch (e) { j = {}; }
        if (!r.ok) throw new Error((j && (j.message || j.error)) || 'request failed (' + r.status + ')');
        return j;
      });
    });
  }

  function fail(err) {
    var msg = (err && err.message) || '';
    if (C.notify) C.notify((msg ? msg + ' ' : '') + 'Nothing changed.', 'error');
  }

  // "Saved", in place, in the line's own acts: it settles in, then fades.
  function tick(line) {
    if (!line) return;
    var host = line.querySelector('.desk-acts') || line;
    var old = host.querySelector('.desk-saved');
    if (old) old.remove();
    var s = document.createElement('span');
    s.className = 'desk-saved';
    s.setAttribute('role', 'status');
    s.innerHTML = CHECK + '<span>Saved</span>';
    host.insertBefore(s, host.firstChild);
    setTimeout(function () {
      if (!s.isConnected) return;
      play(s, { opacity: 1 }, { opacity: 0 }, 300, token('--ease-in', 'ease-in')).then(function () { s.remove(); });
    }, 1600);
  }

  // The Glow: the line the owner just changed, lit for about a second.
  function glow(line) {
    if (!line || level() === 'off') return;
    line.classList.remove('is-glow');
    void line.offsetWidth;
    line.classList.add('is-glow');
    setTimeout(function () { line.classList.remove('is-glow'); }, 1300);
  }

  function saved(btn, value) {
    var line = btn.closest('[data-line]');
    btn.setAttribute('data-value', value);
    btn.removeAttribute('data-state');
    btn.removeAttribute('aria-busy');
    tick(line);
    glow(line);
    // A word inside a line's fold has changed what the line's sentence says.
    if (C.deskFold && C.deskFold.changed) C.deskFold.changed(btn);
  }

  function choose(btn, value) {
    closeMenu();
    if (value === btn.getAttribute('data-value')) { btn.focus(); return; }
    var opt = options(btn).filter(function (o) { return o.v === value; })[0];
    if (!opt) { btn.focus(); return; }
    // A local choice (the invite's role) is read by the button that sends it.
    if (btn.hasAttribute('data-local')) {
      setShown(btn, opt.l);
      btn.setAttribute('data-value', value);
      btn.focus({ preventScroll: true });
      return;
    }
    var was = { value: btn.getAttribute('data-value'), text: btn.textContent };
    var after = btn.parentNode.nextElementSibling;
    var para = btn.closest('p');
    after = after && after.hasAttribute('data-desk-after') ? after : (para ? para.querySelector('[data-desk-after]') : null);
    var wasAfter = after ? after.textContent : '';
    // Feedback first: the new word and a working state, inside a tenth of a
    // second, then the save settles it or puts the old word back.
    setShown(btn, opt.l);
    if (after && opt.a) after.textContent = opt.a;
    btn.setAttribute('data-state', 'working');
    btn.setAttribute('aria-busy', 'true');
    btn.focus({ preventScroll: true });
    send(btn, value).then(function () {
      saved(btn, value);
    }, function (err) {
      setShown(btn, was.text);
      if (after) after.textContent = wasAfter;
      btn.removeAttribute('data-state');
      btn.removeAttribute('aria-busy');
      fail(err);
    });
  }

  // ---- the name ----------------------------------------------------------

  function rename(btn) {
    var wasName = btn.getAttribute('data-value');
    var input = document.createElement('input');
    input.type = 'text';
    input.className = 'desk-ty';
    input.value = wasName;
    input.maxLength = 200;
    input.size = Math.max(10, wasName.length);
    input.setAttribute('aria-label', btn.getAttribute('data-label') || 'Name');
    btn.replaceWith(input);
    input.focus();
    input.select();
    var done = false;
    function end(save) {
      if (done) return;
      done = true;
      var v = input.value.trim();
      input.replaceWith(btn);
      btn.focus({ preventScroll: true });
      if (!save || v === wasName) return;
      if (!v) { if (C.notify) C.notify('A campaign needs a name.', 'error'); return; }
      setShown(btn, v);
      btn.setAttribute('data-state', 'working');
      btn.setAttribute('aria-busy', 'true');
      send(btn, v).then(function (res) {
        var name = (res && res.name) || v;
        setShown(btn, name);
        saved(btn, name);
      }, function (err) {
        setShown(btn, wasName);
        btn.removeAttribute('data-state');
        btn.removeAttribute('aria-busy');
        fail(err);
      });
    }
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') { e.preventDefault(); end(true); }
      else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); end(false); }
    });
    input.addEventListener('blur', function () { end(true); });
  }

  // ---- entry -------------------------------------------------------------

  /**
   * Called by a blank's inline onclick with the button and the click event. A
   * click made from the keyboard has detail 0, and opens the menu at once with
   * focus inside it.
   */
  function activate(btn, ev) {
    if (!btn || btn.getAttribute('aria-disabled') === 'true') return;
    if (btn.getAttribute('data-desk-blank') === 'name') { rename(btn); return; }
    if (current && current.btn === btn) { closeMenu(); return; }
    openMenu(btn, !!ev && ev.detail === 0);
  }

  C.deskBlank = { activate: activate, close: function () { closeMenu({ now: true }); } };
})();
