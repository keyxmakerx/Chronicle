/*
 * diag_picker.js — the AI helper's pickers (admin/diagnostics_picker.templ).
 *
 * The server renders every question, check and blank; this widget only folds
 * them, filters the hand-picking list as Find is typed, keeps the tray in step
 * with the ticked checks, and opens the review sheet when HTMX fills it. The
 * forms post to the compose route, which re-validates everything, so nothing
 * here decides what runs.
 */
(function () {
  'use strict';

  function isTyping(el) {
    if (!el) return false;
    return el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName);
  }

  function setFold(btn, open) {
    var drape = document.getElementById(btn.getAttribute('aria-controls'));
    btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (btn.parentNode) btn.parentNode.classList.toggle('is-open', open);
    if (drape) drape.classList.toggle('is-open', open);
  }

  Chronicle.register('diag-picker', {
    init: function (root) {
      var own = root.querySelector('[data-dw-own]');
      var find = root.querySelector('[data-dw-find]');
      var sheet = root.querySelector('[data-dw-sheet]');
      var review = root.querySelector('#dw-review');
      var ownToggle = root.querySelector('[data-dw-toggle="dw-own"]');
      var handlers = [];

      function on(target, type, fn) {
        target.addEventListener(type, fn);
        handlers.push([target, type, fn]);
      }

      // A question can submit once each blank is filled and a check is ticked.
      function refreshQuestion(form) {
        var blanks = form.querySelectorAll('.dw-blank[required]');
        var missing = false;
        blanks.forEach(function (b) {
          var filled = b.value.trim() !== '';
          b.classList.toggle('is-filled', filled);
          if (!filled) missing = true;
        });
        var n = form.querySelectorAll('input[name="call"]:checked').length;
        var go = form.querySelector('[data-dw-go]');
        go.disabled = missing || n === 0;
        go.textContent = 'Review ' + n + (n === 1 ? ' check' : ' checks');
        form.querySelector('[data-dw-need]').hidden = !missing;
      }

      // The tray lists ticked checks; a check's own text blank moves in with it.
      function refreshTray() {
        if (!own) return;
        var tray = own.querySelector('[data-dw-tray]');
        var ticked = [].slice.call(own.querySelectorAll('.dw-item input[name="call"]:checked'));
        var needs = { campaign: false, page: false };
        own.querySelectorAll('[data-dw-arg-home]').forEach(function (home) {
          var input = tray.querySelector('input[name="arg:' + home.getAttribute('data-dw-arg-home') + '"]');
          if (input) home.appendChild(input);
        });
        tray.textContent = '';
        ticked.forEach(function (box) {
          var name = box.value;
          var kind = box.getAttribute('data-dw-kind');
          var title = box.parentNode.querySelector('[data-dw-title]').getAttribute('data-dw-title');
          if (kind === 'campaign' || kind === 'campaign-text' || kind === 'campaign-page') needs.campaign = true;
          if (kind === 'campaign-page') needs.page = true;
          var li = document.createElement('li');
          var label = document.createElement('span');
          label.textContent = title;
          var rm = document.createElement('button');
          rm.type = 'button';
          rm.className = 'dw-icon';
          rm.setAttribute('aria-label', 'Remove ' + title);
          rm.setAttribute('data-dw-remove', name);
          rm.innerHTML = '<i class="fa-solid fa-xmark" aria-hidden="true"></i>';
          li.appendChild(label);
          li.appendChild(rm);
          var home = own.querySelector('[data-dw-arg-home="' + name + '"]');
          var arg = home && home.querySelector('input');
          if (arg) li.appendChild(arg);
          tray.appendChild(li);
        });
        own.querySelector('[data-dw-tray-n]').textContent = ticked.length;
        own.querySelector('[data-dw-empty]').hidden = ticked.length > 0;
        own.querySelectorAll('[data-dw-needs]').forEach(function (el) {
          el.hidden = !needs[el.getAttribute('data-dw-needs')];
        });
        own.querySelector('aside [data-dw-go]').disabled = ticked.length === 0;
        own.querySelectorAll('[data-dw-group]').forEach(function (g) {
          var n = g.querySelectorAll('input[name="call"]:checked').length;
          var pill = g.querySelector('[data-dw-picked]');
          pill.hidden = n === 0;
          pill.textContent = n + ' picked';
        });
        own.querySelectorAll('.dw-item').forEach(function (item) {
          item.classList.toggle('is-on', item.querySelector('input').checked);
        });
        own.querySelectorAll('.dw-blank').forEach(function (b) { b.classList.toggle('is-filled', b.value.trim() !== ''); });
      }

      // Find opens every group with a match and hides the rest.
      function filter() {
        var q = find.value.trim().toLowerCase();
        var any = false;
        own.querySelectorAll('[data-dw-group]').forEach(function (g) {
          var shown = 0;
          g.querySelectorAll('[data-dw-item]').forEach(function (item) {
            var hit = !q || item.getAttribute('data-dw-text').indexOf(q) >= 0;
            item.hidden = !hit;
            if (hit) shown++;
            var t = item.querySelector('[data-dw-title]');
            var text = t.getAttribute('data-dw-title');
            var i = q ? text.toLowerCase().indexOf(q) : -1;
            t.textContent = '';
            if (i < 0) { t.textContent = text; return; }
            var mark = document.createElement('mark');
            mark.textContent = text.slice(i, i + q.length);
            t.append(text.slice(0, i), mark, text.slice(i + q.length));
          });
          g.hidden = shown === 0;
          var count = g.querySelector('[data-dw-count]');
          var total = count.getAttribute('data-total');
          count.textContent = q ? shown + ' of ' + total : total;
          if (q) setFold(g.querySelector('[data-dw-fold]'), shown > 0);
          if (shown) any = true;
        });
        var none = own.querySelector('[data-dw-none]');
        none.hidden = any;
        none.textContent = any ? '' : 'No check matches "' + find.value.trim() + '". Try a plainer word, like errors or sync.';
      }

      function openOwn() {
        ownToggle.setAttribute('aria-expanded', 'true');
        own.classList.add('is-open');
      }

      function closeSheet() {
        sheet.hidden = true;
        review.textContent = '';
      }

      on(root, 'click', function (e) {
        var fold = e.target.closest('[data-dw-fold]');
        if (fold && root.contains(fold)) {
          var open = fold.getAttribute('aria-expanded') !== 'true';
          // One question open at a time, like the signed mockup.
          if (fold.closest('[data-dw-q]') && open) {
            root.querySelectorAll('[data-dw-q] [data-dw-fold][aria-expanded="true"]').forEach(function (b) { setFold(b, false); });
          }
          setFold(fold, open);
          return;
        }
        var toggle = e.target.closest('[data-dw-toggle]');
        if (toggle) {
          var target = document.getElementById(toggle.getAttribute('data-dw-toggle'));
          var now = toggle.getAttribute('aria-expanded') !== 'true';
          toggle.setAttribute('aria-expanded', now ? 'true' : 'false');
          target.classList.toggle('is-open', now);
          return;
        }
        var rm = e.target.closest('[data-dw-remove]');
        if (rm) {
          var box = own.querySelector('.dw-item input[value="' + CSS.escape(rm.getAttribute('data-dw-remove')) + '"]');
          if (box) box.checked = false;
          refreshTray();
          return;
        }
        if (e.target.closest('[data-dw-sheet-close]')) { closeSheet(); return; }
        var copy = e.target.closest('[data-dw-copy]');
        if (copy) {
          copy.textContent = 'Copied';
          setTimeout(function () { copy.textContent = 'Copy checks for your AI'; }, 2000);
        }
      });

      // Only a ticked or unticked check rebuilds the tray; typing in a blank
      // that lives in the tray must not move it out from under the caret.
      function onInput(e) {
        var t = e.target;
        var form = t.closest('[data-dw-q]');
        if (form) refreshQuestion(form);
        else if (t === find) filter();
        else if (own && own.contains(t) && t.name === 'call') refreshTray();
        else if (t.classList.contains('dw-blank')) t.classList.toggle('is-filled', t.value.trim() !== '');
      }
      on(root, 'input', onInput);
      on(root, 'change', onInput);

      // "/" opens the hand-picking list and goes to Find, never while typing.
      on(document, 'keydown', function (e) {
        if (e.key === 'Escape' && !sheet.hidden) { closeSheet(); return; }
        if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey || isTyping(document.activeElement)) return;
        if (!document.body.contains(root)) return;
        e.preventDefault();
        openOwn();
        find.focus();
      });

      // The review fragment arrives by HTMX; show its sheet and move focus in.
      on(root, 'htmx:afterSwap', function (e) {
        if (e.target !== review) return;
        sheet.hidden = review.textContent.trim() === '';
        var first = review.querySelector('select, input:not([type="hidden"]), button');
        if (first) first.focus({ preventScroll: true });
      });

      root.querySelectorAll('[data-dw-q]').forEach(refreshQuestion);
      refreshTray();
      root._dwHandlers = handlers;
    },
    destroy: function (root) {
      (root._dwHandlers || []).forEach(function (h) { h[0].removeEventListener(h[1], h[2]); });
      root._dwHandlers = null;
    },
  });
})();
