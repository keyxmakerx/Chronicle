/**
 * picture_box.js -- the "Change picture" box in the top-bar user menu
 * (Chronicle.PictureBox).
 *
 * The server renders the menu header and the box (layouts/picture_box.templ);
 * this file opens it as a fold inside the menu, sends the upload to the
 * existing account endpoints (POST/DELETE /account/avatar, which validate the
 * file and only ever act on the signed-in user), and repaints every
 * [data-pk-av] element (top bar, menu header, box, the user's own People row)
 * so the new picture shows without a reload.
 *
 * Controls call in through plain inline onclick expressions rendered
 * Go-side, so nothing depends on a script inside a swapped fragment.
 */
(function () {
  'use strict';

  var MAX_BYTES = 2 * 1024 * 1024;
  var TYPES = /^image\/(jpeg|png|webp|gif)$/;
  var busy = false;

  function $(id) { return document.getElementById(id); }
  function reduced() {
    var h = document.documentElement;
    return matchMedia('(prefers-reduced-motion: reduce)').matches ||
      !!document.querySelector('.nav-rm') ||
      h.hasAttribute('data-cz-reduce') || h.getAttribute('data-view-motion') === 'calm';
  }

  // Opens or shuts the fold by animating its height to the content's real
  // height, then releases it to auto so later content changes never clip.
  function fold(el, open) {
    if (reduced()) { el.style.height = open ? 'auto' : '0px'; el.classList.toggle('is-open', open); return; }
    var from = el.getBoundingClientRect().height;
    el.style.height = 'auto';
    var to = open ? el.scrollHeight : 0;
    el.style.height = from + 'px';
    el.getBoundingClientRect();
    el.classList.toggle('is-open', open);
    el.style.height = to + 'px';
    var done = false;
    function fin() {
      if (done) return;
      done = true;
      el.removeEventListener('transitionend', onEnd);
      if (open) el.style.height = 'auto';
    }
    function onEnd(e) { if (e.target === el && e.propertyName === 'height') fin(); }
    el.addEventListener('transitionend', onEnd);
    setTimeout(fin, 420);
  }
  function isOpen() { var f = $('pk-fold'); return !!f && f.classList.contains('is-open'); }
  function regrow() { var f = $('pk-fold'); if (f && isOpen()) f.style.height = 'auto'; }

  function toast(msg) {
    var old = document.querySelector('.ag-toast');
    if (old) old.remove();
    var t = document.createElement('div');
    t.className = 'ag-toast';
    t.setAttribute('role', 'status');
    t.innerHTML = '<i class="fa-solid fa-circle-check"></i><span></span>';
    t.querySelector('span').textContent = msg;
    document.body.appendChild(t);
    setTimeout(function () { t.remove(); }, 3200);
  }

  // The amber bar is the one place an error shows; it clears on the next try.
  function warn(msg) {
    var w = $('pk-warn');
    if (!w) return;
    if (!msg) { w.classList.remove('is-on'); regrow(); return; }
    w.querySelector('span').textContent = msg;
    w.classList.remove('is-on'); void w.offsetWidth; w.classList.add('is-on');
    regrow();
  }

  function show(id, on) { var el = $(id); if (el) el.hidden = !on; }

  // paint sets every picture of the signed-in user to url, or to their
  // initials when url is empty.
  function paint(url) {
    var els = document.querySelectorAll('[data-pk-av]');
    for (var i = 0; i < els.length; i++) {
      var el = els[i];
      el.textContent = '';
      if (url) {
        var img = document.createElement('img');
        img.src = url;
        img.alt = '';
        img.className = 'w-full h-full object-cover';
        el.appendChild(img);
      } else {
        el.textContent = el.getAttribute('data-initials') || '?';
      }
    }
    show('pk-remove', !!url);
    show('pk-none', !url);
    var top = $('pk-top-av');
    if (top && !reduced()) { top.classList.remove('ag-ring'); void top.offsetWidth; top.classList.add('ag-ring'); }
  }

  function setBusy(on, label) {
    busy = on;
    var up = $('pk-up');
    if (up) up.disabled = on;
    var rm = $('pk-remove');
    if (rm) rm.disabled = on;
    show('pk-prog', on);
    var t = $('pk-pt');
    if (t && label) t.textContent = label;
    regrow();
  }

  function request(method, body, onOK, failMsg) {
    var x = new XMLHttpRequest();
    x.open(method, '/account/avatar');
    x.setRequestHeader('Accept', 'application/json');
    var csrf = window.Chronicle && Chronicle.getCsrf ? Chronicle.getCsrf() : '';
    if (csrf) x.setRequestHeader('X-CSRF-Token', csrf);
    var bar = document.querySelector('#pk-prog .ag-prog > span');
    var pn = $('pk-pn');
    if (bar) bar.style.width = '0';
    if (pn) pn.textContent = '0%';
    if (x.upload) {
      x.upload.onprogress = function (e) {
        if (!e.lengthComputable) return;
        var n = Math.round(e.loaded / e.total * 100);
        if (bar) bar.style.width = n + '%';
        if (pn) pn.textContent = n + '%';
        if (n >= 100 && $('pk-pt')) $('pk-pt').textContent = 'Saving…';
      };
    }
    x.onload = function () {
      var d = {};
      try { d = JSON.parse(x.responseText); } catch (e) { d = {}; }
      if (x.status >= 200 && x.status < 300) { onOK(d); return; }
      setBusy(false);
      warn(d.message || d.error || failMsg);
    };
    x.onerror = function () { setBusy(false); warn(failMsg); };
    x.send(body);
  }

  var PB = {
    // toggle is the "Change picture" link in the menu header.
    toggle: function () {
      var f = $('pk-fold'), b = $('pk-toggle');
      if (!f) return;
      var on = !isOpen();
      fold(f, on);
      if (b) b.setAttribute('aria-expanded', on ? 'true' : 'false');
      f.setAttribute('aria-hidden', on ? 'false' : 'true');
      // inert keeps a closed fold's buttons out of the tab order.
      f.inert = !on;
      if (!on) PB.reset();
    },

    // open is called from elsewhere on the page (the People row link): it
    // opens the menu if needed, then the box.
    open: function () {
      var btn = $('user-menu-btn'), menu = $('user-menu-panel');
      if (!btn || !menu) return;
      // Deferred so the click that got us here is not read as a click
      // outside the menu the moment it opens.
      setTimeout(function () {
        if (menu.offsetParent === null) btn.click();
        if (!isOpen()) PB.toggle();
      }, 0);
    },

    // shut is called when the menu closes, so it opens fresh next time.
    shut: function () {
      var f = $('pk-fold');
      if (!f || !isOpen()) return;
      f.classList.remove('is-open');
      f.style.height = '0px';
      var b = $('pk-toggle');
      if (b) b.setAttribute('aria-expanded', 'false');
      f.setAttribute('aria-hidden', 'true');
      f.inert = true;
      PB.reset();
    },

    reset: function () {
      if (busy) return;
      warn('');
      show('pk-confirm', false);
      show('pk-main', true);
    },

    pick: function () { if (!busy) { var f = $('pk-file'); if (f) f.click(); } },

    chosen: function (input) {
      var file = input && input.files && input.files[0];
      if (!file || busy) return;
      input.value = '';
      warn('');
      if (!TYPES.test(file.type)) { warn('That file isn’t a picture Chronicle can use. Try a JPG, PNG, WebP or GIF.'); return; }
      if (file.size > MAX_BYTES) { warn('That picture is over 2 MB. Pick a smaller one.'); return; }
      var fd = new FormData();
      fd.append('avatar', file);
      setBusy(true, 'Uploading…');
      request('POST', fd, function (d) {
        setBusy(false);
        paint(d.avatar_path || '');
        var av = $('pk-box-av');
        if (av && !reduced()) { av.classList.remove('ag-landed'); void av.offsetWidth; av.classList.add('ag-landed'); }
        toast('Picture updated');
      }, 'The picture didn’t upload. Check your connection and try again.');
    },

    askRemove: function () {
      if (busy) return;
      warn('');
      show('pk-main', false);
      show('pk-confirm', true);
      regrow();
    },
    keep: function () { show('pk-confirm', false); show('pk-main', true); regrow(); },

    remove: function () {
      if (busy) return;
      show('pk-confirm', false);
      show('pk-main', true);
      setBusy(true, 'Removing…');
      request('DELETE', null, function () {
        setBusy(false);
        paint('');
        toast('Picture removed');
      }, 'The picture didn’t come off. Try again.');
    }
  };

  window.Chronicle = window.Chronicle || {};
  Chronicle.PictureBox = PB;
})();
