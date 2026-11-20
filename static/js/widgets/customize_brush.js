/**
 * customize_brush.js — the Customize brush in the header, for the owner.
 *
 * Mount: <div data-widget="customize-brush"> drawn by
 * layouts/customize_brush.templ. The brush opens a small menu: first a row
 * for the place the owner is on ("How Character pages are laid out"), then
 * the campaign's look, everything in Customize, and "Hide this brush".
 *
 * The header is never redrawn by a boosted page change, so the first row is
 * asked of the server by the page's address (GET .../customize/here?path=),
 * once per address, ahead of time: on load, after each page change, and when
 * the pointer or focus reaches the brush. A row that is not known yet when
 * the menu opens is added the moment it arrives.
 *
 * The menu Grows out of the brush (.mn-enter/.mn-from, input.css): 120ms in,
 * 80ms out; from the keyboard it appears at once, and Calm and Off are the
 * page-wide guards. Enter or Space opens it, the arrows move, Home and End
 * jump, Escape closes it and gives focus back to the brush, Tab closes it.
 *
 * "Hide this brush" saves the person's own display choice (PUT
 * /account/view-prefs {brush:"hidden"}) and puts the brush away as an inert
 * template, from which Campaign settings brings it back.
 */
(function () {
  'use strict';
  window.Chronicle = window.Chronicle || {};
  var C = window.Chronicle;

  // The first row for each address, kept for this page's life.
  var cache = {};

  function rows(menu) {
    return Array.prototype.slice.call(menu.querySelectorAll('[role="menuitem"]')).filter(function (r) { return !r.hidden; });
  }

  // How long the menu's leaving move lasts, so it is hidden only after it.
  function leaveMs(menu) {
    var d = parseFloat(getComputedStyle(menu).transitionDuration) || 0;
    return d * 1000;
  }

  window.Chronicle.register('customize-brush', {
    init: function (host) {
      var pristine = host.outerHTML;
      var btn = host.querySelector('[data-brush-toggle]');
      var menu = host.querySelector('[data-brush-menu]');
      var here = host.querySelector('[data-brush-here]');
      var hide = host.querySelector('[data-brush-hide]');
      var url = host.getAttribute('data-here-url');
      if (!btn || !menu || !here || !url) return;
      var isOpen = false, timer = 0, alive = true;

      function show(row) {
        if (!row) { here.hidden = true; return; }
        here.querySelector('[data-brush-here-t]').textContent = row.label;
        here.querySelector('[data-brush-here-s]').textContent = row.path;
        here.setAttribute('href', row.href);
        here.hidden = false;
        if (window.htmx && htmx.process) htmx.process(here);
      }

      function fetchHere() {
        var path = location.pathname;
        if (Object.prototype.hasOwnProperty.call(cache, path)) {
          var hit = cache[path];
          return hit && hit.then ? hit : Promise.resolve(hit);
        }
        var p = C.apiFetch(url + '?path=' + encodeURIComponent(path)).then(function (r) {
          if (!r.ok) throw new Error('status ' + r.status);
          return r.json();
        }).then(function (j) {
          cache[path] = (j && j.row) || null;
          return cache[path];
        }, function () {
          // Not known: the menu goes without its first row, and asks again
          // next time.
          delete cache[path];
          return null;
        });
        cache[path] = p;
        return p;
      }

      function refresh() {
        var path = location.pathname;
        fetchHere().then(function (row) {
          if (alive && path === location.pathname) show(row);
        });
      }

      function open(fromKeys) {
        if (isOpen) return;
        isOpen = true;
        clearTimeout(timer);
        refresh();
        btn.setAttribute('aria-expanded', 'true');
        menu.hidden = false;
        menu.classList.remove('mn-leave', 'mn-to');
        menu.classList.add('mn-enter', 'mn-from');
        void menu.offsetWidth;
        menu.classList.remove('mn-from');
        menu.classList.add('mn-to');
        if (fromKeys) {
          var r = rows(menu);
          if (r[0]) r[0].focus();
        }
      }

      function close(toButton) {
        if (!isOpen) return;
        isOpen = false;
        btn.setAttribute('aria-expanded', 'false');
        menu.classList.remove('mn-enter', 'mn-to');
        menu.classList.add('mn-leave', 'mn-from');
        var ms = leaveMs(menu);
        clearTimeout(timer);
        // From the keyboard it goes at once, as it came.
        if (ms <= 0 || toButton) menu.hidden = true;
        else timer = setTimeout(function () { if (!isOpen) menu.hidden = true; }, ms + 20);
        if (toButton) btn.focus();
      }

      function move(by) {
        var r = rows(menu);
        if (!r.length) return;
        var at = r.indexOf(document.activeElement);
        var next = by === 'first' ? 0 : by === 'last' ? r.length - 1 : (at + by + r.length) % r.length;
        r[next].focus();
      }

      // A keyboard press opens it at once with the first row focused; a
      // pointer press opens it with the Grow.
      var keyed = false;
      btn.addEventListener('keydown', function (e) {
        if (e.key === 'Enter' || e.key === ' ') { keyed = true; return; }
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
          e.preventDefault();
          open(true);
          if (e.key === 'ArrowUp') move('last');
        }
      });
      btn.addEventListener('click', function () {
        var k = keyed;
        keyed = false;
        if (isOpen) close(false); else open(k);
      });
      btn.addEventListener('pointerenter', refresh);
      btn.addEventListener('focus', refresh);

      menu.addEventListener('keydown', function (e) {
        switch (e.key) {
          case 'ArrowDown': move(1); break;
          case 'ArrowUp': move(-1); break;
          case 'Home': move('first'); break;
          case 'End': move('last'); break;
          case 'Escape': close(true); break;
          case 'Tab': close(false); return;
          case ' ':
            if (document.activeElement && document.activeElement.click) document.activeElement.click();
            break;
          default: return;
        }
        e.preventDefault();
        e.stopPropagation();
      });
      menu.addEventListener('click', function (e) {
        var row = e.target.closest('[role="menuitem"]');
        if (row && row !== hide) close(false);
      });

      hide.addEventListener('click', function () {
        hide.setAttribute('aria-busy', 'true');
        C.apiFetch('/account/view-prefs', { method: 'PUT', body: { brush: 'hidden' } }).then(function (r) {
          if (!r.ok) throw new Error('status ' + r.status);
          putAway();
        }).catch(function () {
          hide.removeAttribute('aria-busy');
          if (C.notify) C.notify('The brush could not be hidden. Nothing changed.', 'error');
        });
      });

      // The brush goes back into an inert template; the settings page's
      // Brush row reads Hidden if it is on screen.
      function putAway() {
        close(false);
        var t = document.createElement('template');
        t.setAttribute('data-brush-kept', '');
        t.innerHTML = pristine;
        destroy();
        host.replaceWith(t);
        var row = document.querySelector('[data-desk-blank][data-id="brush"]');
        if (row) {
          row.setAttribute('data-value', 'hidden');
          var w = row.querySelector('.desk-t');
          if (w) w.textContent = 'Hidden';
        }
        document.dispatchEvent(new CustomEvent('chronicle:brush', { detail: { shown: false } }));
        if (C.notify) C.notify('The brush is hidden. Campaign settings brings it back.', 'success');
      }

      function onDown(e) { if (isOpen && !host.contains(e.target)) close(false); }
      function onNav() { close(false); refresh(); }
      document.addEventListener('pointerdown', onDown, true);
      ['htmx:pushedIntoHistory', 'htmx:historyRestore'].forEach(function (t) { document.addEventListener(t, onNav); });
      window.addEventListener('popstate', onNav);

      function destroy() {
        alive = false;
        clearTimeout(timer);
        document.removeEventListener('pointerdown', onDown, true);
        ['htmx:pushedIntoHistory', 'htmx:historyRestore'].forEach(function (t) { document.removeEventListener(t, onNav); });
        window.removeEventListener('popstate', onNav);
      }
      host._brushDestroy = destroy;

      // Ask ahead, once the page has settled.
      (window.requestIdleCallback || function (f) { return setTimeout(f, 200); })(refresh);
    },
    destroy: function (host) { if (host && host._brushDestroy) host._brushDestroy(); }
  });
})();
