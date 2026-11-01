/**
 * ai_chip.js -- the AI button in the top bar and the panel it opens over any
 * campaign page (layouts/ai_chip.templ; signed option D, and D for players).
 *
 * Mount: <div data-widget="ai-chip" data-campaign-id data-ai-base> in the top
 * bar. data-ai-base is "ai" for the owner and "ai/me" for anyone else in the
 * campaign. A player's button starts hidden and shows only while the chip
 * answer says players may use AI here (off: false); while hidden, Ctrl+V is
 * left alone.
 *
 * - The button counts requests waiting and open warnings
 *   (GET /campaigns/:id/<base>/chip, JSON), every 30 seconds while the tab is
 *   in view. When more requests wait than this browser last saw, a notice
 *   drops under the button with Look.
 * - The panel loads its content from GET /campaigns/:id/<base>/panel through
 *   htmx, so the review and its widgets work as they do anywhere. It is
 *   moved to <body> so the top bar never clips it. Escape, the scrim and
 *   the close button shut it; focus goes back to the button.
 * - Ctrl+V outside a text field with an AI's answer copied (text starting a
 *   block: a --- line, then "key:") opens the panel and reviews the paste.
 *   Other pastes are left alone. An open review keeps its paste.
 * - Anything with data-ai-open opens the panel, and so does the old
 *   #copy-and-paste address.
 */
(function () {
  'use strict';

  var POLL_MS = 30000;
  var NOTE_MS = 10000;
  // An AI's answer starts a block: a fence line, then a front-matter key.
  var BLOCK_RE = /^-{3,}[ \t]*\r?\n[ \t]*[A-Za-z_]+[ \t]*:/m;

  function typing(t) {
    return t && t.closest && (t.closest('input, textarea, select') || t.isContentEditable);
  }

  function seenKey(cid) { return 'ai-chip-seen-' + cid; }
  function readSeen(cid) {
    try { var v = sessionStorage.getItem(seenKey(cid)); return v === null ? -1 : Number(v); } catch (e) { return -1; }
  }
  function writeSeen(cid, n) {
    try { sessionStorage.setItem(seenKey(cid), String(n)); } catch (e) { /* private window */ }
  }

  Chronicle.register('ai-chip', {
    init: function (host, config) {
      var cid = String(config.campaignId || '');
      var base = config.aiBase === 'ai/me' ? 'ai/me' : 'ai';
      var chip = host.querySelector('[data-ai-chip]');
      var badge = host.querySelector('[data-ai-chip-n]');
      var label = host.querySelector('[data-ai-chip-label]');
      var note = host.querySelector('[data-ai-note]');
      var panel = host.querySelector('.ai-panel');
      var scrim = host.querySelector('[data-ai-scrim]');
      if (!cid || !chip || !panel || !scrim) return;
      var body = panel.querySelector('[data-ai-panel-body]');

      // A boosted page swap leaves the last page's panel in <body>; only one
      // may exist, since its requests and paste box carry fixed ids.
      Array.prototype.forEach.call(document.querySelectorAll('body > .ai-panel, body > .ai-panel-scrim'), function (el) { el.remove(); });
      document.body.appendChild(scrim);
      document.body.appendChild(panel);

      var state = { open: false, loaded: false, timer: null, noteTimer: null, waiting: -1 };

      function reviewOpen() { return !!panel.querySelector('#ai-import-review-host form, .aic-req-review form'); }

      function show(counts) {
        host.hidden = !!counts.off;
        if (counts.off) { if (state.open) close(); return; }
        var n = (counts.waiting || 0) + (counts.warnings || 0);
        badge.hidden = n === 0;
        badge.textContent = n > 9 ? '9+' : String(n);
        if (counts.warnings || counts.stopped) badge.setAttribute('data-tone', 'bad'); else badge.removeAttribute('data-tone');
        var words = 'Your AI';
        if (counts.waiting) words += ', ' + counts.waiting + (counts.waiting === 1 ? ' request waits' : ' requests wait');
        if (counts.warnings) words += ', ' + counts.warnings + (counts.warnings === 1 ? ' warning' : ' warnings');
        if (counts.stopped) words += ', stopped here';
        label.textContent = words;
        chip.title = words;
        var seen = readSeen(cid);
        if (seen >= 0 && counts.waiting > seen && !state.open) notice(counts.waiting);
        writeSeen(cid, counts.waiting || 0);
        state.waiting = counts.waiting || 0;
      }

      function poll() {
        if (document.hidden) return;
        Chronicle.apiFetch('/campaigns/' + encodeURIComponent(cid) + '/' + base + '/chip')
          .then(function (r) { return r.ok ? r.json() : null; })
          .then(function (c) { if (c) show(c); })
          .catch(function () { /* the next poll tries again */ });
      }

      function hideNote() { note.hidden = true; note.innerHTML = ''; clearTimeout(state.noteTimer); }
      function notice(n) {
        note.innerHTML = '';
        var p = document.createElement('span');
        p.textContent = n === 1 ? 'Your AI asks for changes.' : n + ' requests from your AI wait.';
        var look = document.createElement('button');
        look.type = 'button';
        look.className = 'btn-primary btn-sm';
        look.textContent = 'Look';
        look.addEventListener('click', function () { hideNote(); open(); });
        var x = document.createElement('button');
        x.type = 'button';
        x.className = 'aic-icon';
        x.setAttribute('aria-label', 'Dismiss');
        x.innerHTML = '<i class="fa-solid fa-xmark" aria-hidden="true"></i>';
        x.addEventListener('click', hideNote);
        note.appendChild(p); note.appendChild(look); note.appendChild(x);
        note.hidden = false;
        clearTimeout(state.noteTimer);
        state.noteTimer = setTimeout(hideNote, NOTE_MS);
      }

      function load() {
        if (state.loaded && reviewOpen()) return Promise.resolve();
        state.loaded = true;
        // Resolve once htmx has settled the new content: before that its forms
        // are not wired, and submitting one would reload the page instead.
        return new Promise(function (resolve) {
          function done() {
            body.removeEventListener('htmx:afterSettle', done);
            body.removeEventListener('htmx:responseError', done);
            resolve();
          }
          body.addEventListener('htmx:afterSettle', done);
          body.addEventListener('htmx:responseError', done);
          htmx.ajax('GET', '/campaigns/' + encodeURIComponent(cid) + '/' + base + '/panel', { target: body, swap: 'innerHTML' })
            .catch(done);
        });
      }

      function open() {
        hideNote();
        if (!state.open) {
          state.open = true;
          state.back = document.activeElement;
          scrim.hidden = false; panel.hidden = false;
          void panel.offsetWidth;
          scrim.classList.add('is-open'); panel.classList.add('is-open');
          chip.setAttribute('aria-expanded', 'true');
          document.documentElement.classList.add('overflow-hidden');
          panel.focus();
        }
        return load().then(function () {
          var close = panel.querySelector('[data-ai-close]');
          if (state.open && close && panel.contains(document.activeElement) === false) close.focus();
        });
      }

      function close() {
        if (!state.open) return;
        state.open = false;
        scrim.classList.remove('is-open'); panel.classList.remove('is-open');
        chip.setAttribute('aria-expanded', 'false');
        document.documentElement.classList.remove('overflow-hidden');
        setTimeout(function () {
          if (state.open) return;
          scrim.hidden = true; panel.hidden = true;
          // Keep a review in progress for the next open; otherwise drop the
          // content so its requests stop polling.
          if (!reviewOpen()) { body.innerHTML = ''; state.loaded = false; }
        }, 220);
        if (location.hash === '#copy-and-paste') history.replaceState(null, '', location.pathname + location.search);
        if (state.back && state.back.focus) state.back.focus(); else chip.focus();
        poll();
      }

      function onPaste(e) {
        if (host.hidden || typing(e.target) || reviewOpen()) return;
        var text = e.clipboardData && e.clipboardData.getData('text/plain');
        if (!text || !BLOCK_RE.test(text)) return;
        e.preventDefault();
        open().then(function () {
          var area = document.getElementById('ai-import-paste-area');
          if (!area || !area.form) return;
          area.value = text;
          area.dispatchEvent(new Event('input', { bubbles: true }));
          area.form.requestSubmit();
        });
      }

      function onKey(e) {
        if (e.key !== 'Escape' || !state.open) return;
        // A pop-up inside the panel closes first.
        var inner = panel.querySelector('[role=dialog]:not(.ai-panel)');
        if (inner && inner.offsetParent !== null) return;
        e.preventDefault();
        close();
      }

      function onClick(e) {
        if (e.target.closest('[data-ai-open]') && !host.hidden) { e.preventDefault(); open(); return; }
        if (state.open && e.target.closest('[data-ai-close]')) close();
      }

      chip.addEventListener('click', function () { if (state.open) close(); else open(); });
      scrim.addEventListener('click', close);
      document.addEventListener('click', onClick);
      document.addEventListener('paste', onPaste);
      document.addEventListener('keydown', onKey);
      var onVisible = function () { if (!document.hidden) poll(); };
      document.addEventListener('visibilitychange', onVisible);
      var onHash = function () { if (location.hash === '#copy-and-paste' && !host.hidden) open(); };
      window.addEventListener('hashchange', onHash);
      state.timer = setInterval(poll, POLL_MS);
      host._aiChip = { off: function () {
        clearInterval(state.timer); clearTimeout(state.noteTimer);
        document.removeEventListener('click', onClick);
        document.removeEventListener('paste', onPaste);
        document.removeEventListener('keydown', onKey);
        document.removeEventListener('visibilitychange', onVisible);
        window.removeEventListener('hashchange', onHash);
        document.documentElement.classList.remove('overflow-hidden');
        panel.remove(); scrim.remove();
      } };

      poll();
      if (location.hash === '#copy-and-paste' && !host.hidden) open();
    },
    destroy: function (host) {
      if (host._aiChip) { host._aiChip.off(); host._aiChip = null; }
    }
  });
})();
