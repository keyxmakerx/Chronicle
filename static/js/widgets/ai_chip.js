/**
 * ai_chip.js -- the AI button in the top bar, the small bubble it drops, and
 * the side sheet a review opens in, over any campaign page
 * (layouts/ai_chip.templ; signed option D, D for players, and the AI bubble).
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
 * - Pressing the button drops the bubble (GET <base>/bubble): Copy and Paste
 *   answer in one thin row, then each waiting request with Review. Escape, a
 *   click outside and its close button shut it.
 * - Copy puts the text for an AI app on the clipboard: on an entity page the
 *   owner's copy is that page ("Copy this page"), read with the remembered
 *   privacy choice (localStorage ai-sees-<campaign>).
 * - Paste answer reads the clipboard; if the browser refuses, a small paste
 *   box opens in the bubble instead. An AI's answer (text starting a block:
 *   a --- line, then "key:") opens the side sheet and reviews it, as Ctrl+V
 *   outside a text field does on any page. Other text is refused with a line.
 * - Review, "Write a request…" and anything with data-ai-open open the side
 *   sheet (GET <base>/panel, tool=1 for the copy-and-paste tool), moved to
 *   <body> so the top bar never clips it. The old #copy-and-paste address
 *   opens the tool.
 * - A link to the Your AI page (data-ai-full) in the bubble asks first while
 *   the page has unsaved changes (Chronicle.isDirty).
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

  // The page id on an entity's own page, for "Copy this page".
  var ENTITY_RE = /^\/campaigns\/[^/]+\/entities\/([^/]+)/;
  function currentPage() {
    var m = ENTITY_RE.exec(location.pathname);
    return m && m[1] !== 'new' && m[1] !== 'search' ? m[1] : '';
  }

  function seesOf(cid) {
    try {
      var v = localStorage.getItem('ai-sees-' + cid);
      return v === 'permitted' || v === 'everything' ? v : 'safe';
    } catch (e) { return 'safe'; }
  }

  // Writing a pending item keeps the click's permission while the text loads
  // (Safari needs this); other browsers take the text once it is in.
  // copyFrom resolves with whether the server left the page out (it is
  // hidden by the owner's privacy choice), so the bubble can say so.
  function copyFrom(url) {
    var leftOut = false;
    var text = Chronicle.apiFetch(url).then(function (r) {
      if (!r.ok) throw new Error(String(r.status));
      leftOut = r.headers.get('X-AI-Page') === 'left-out';
      return r.text();
    });
    var done;
    if (window.ClipboardItem && navigator.clipboard && navigator.clipboard.write) {
      done = navigator.clipboard.write([new ClipboardItem({ 'text/plain': text.then(function (t) { return new Blob([t], { type: 'text/plain' }); }) })]);
    } else {
      done = text.then(function (t) { return navigator.clipboard.writeText(t); });
    }
    return done.then(function () { return leftOut; });
  }

  Chronicle.register('ai-chip', {
    init: function (host, config) {
      var cid = String(config.campaignId || '');
      var base = config.aiBase === 'ai/me' ? 'ai/me' : 'ai';
      var owner = base === 'ai';
      var chip = host.querySelector('[data-ai-chip]');
      var badge = host.querySelector('[data-ai-chip-n]');
      var label = host.querySelector('[data-ai-chip-label]');
      var note = host.querySelector('[data-ai-note]');
      var bubble = host.querySelector('[data-ai-bubble]');
      var panel = host.querySelector('.ai-panel');
      var scrim = host.querySelector('[data-ai-scrim]');
      if (!cid || !chip || !panel || !scrim || !bubble) return;
      var body = panel.querySelector('[data-ai-panel-body]');
      var root = '/campaigns/' + encodeURIComponent(cid) + '/' + base;

      // A boosted page swap leaves the last page's panel in <body>; only one
      // may exist, since its requests and paste box carry fixed ids.
      Array.prototype.forEach.call(document.querySelectorAll('body > .ai-panel, body > .ai-panel-scrim'), function (el) { el.remove(); });
      document.body.appendChild(scrim);
      document.body.appendChild(panel);

      var state = { open: false, loaded: false, mode: '', bubble: false, timer: null, noteTimer: null, saidTimer: null, waiting: -1 };

      function reviewOpen() { return !!panel.querySelector('#ai-import-review-host form, .aic-req-review form'); }

      function show(counts) {
        host.hidden = !!counts.off;
        if (counts.off) { closeBubble(); if (state.open) close(); return; }
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
        if (seen >= 0 && counts.waiting > seen && !state.open && !state.bubble) notice(counts.waiting);
        writeSeen(cid, counts.waiting || 0);
        state.waiting = counts.waiting || 0;
      }

      function poll() {
        if (document.hidden) return;
        Chronicle.apiFetch(root + '/chip')
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
        look.addEventListener('click', function () { hideNote(); openBubble(); });
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

      // --- The bubble ---

      function openBubble() {
        hideNote();
        if (state.bubble) return;
        state.bubble = true;
        bubble.hidden = false;
        bubble.innerHTML = '<p class="ai-panel-loading"><i class="fa-solid fa-spinner fa-spin mr-1.5" aria-hidden="true"></i>Opening…</p>';
        void bubble.offsetWidth;
        bubble.classList.add('is-open');
        chip.setAttribute('aria-expanded', 'true');
        htmx.ajax('GET', root + '/bubble', { target: bubble, swap: 'innerHTML' }).then(function () {
          if (!state.bubble) return;
          var copyLabel = bubble.querySelector('[data-ai-copy-label]');
          if (owner && copyLabel && currentPage()) copyLabel.textContent = 'Copy this page';
          var first = bubble.querySelector('[data-ai-copy]');
          if (first) first.focus(); else bubble.focus();
        }).catch(function () {
          bubble.innerHTML = '<p class="aib-said">Couldn’t open just now. Try again.</p>';
        });
      }

      function closeBubble(keepFocus) {
        if (!state.bubble) return;
        state.bubble = false;
        bubble.classList.remove('is-open');
        chip.setAttribute('aria-expanded', 'false');
        bubble.hidden = true;
        bubble.innerHTML = '';
        clearTimeout(state.saidTimer);
        if (!keepFocus) chip.focus();
      }

      function say(text, tone) {
        var said = bubble.querySelector('[data-ai-said]');
        if (!said) return;
        said.textContent = text;
        if (tone) said.setAttribute('data-tone', tone); else said.removeAttribute('data-tone');
        said.hidden = false;
        clearTimeout(state.saidTimer);
        if (tone !== 'bad') state.saidTimer = setTimeout(function () { said.hidden = true; }, 4000);
      }

      function doCopy(btn) {
        var url = btn.getAttribute('data-ai-copy');
        if (owner) {
          url += '?privacy=' + encodeURIComponent(seesOf(cid));
          if (currentPage()) url += '&page=' + encodeURIComponent(currentPage());
        }
        btn.disabled = true;
        copyFrom(url).then(function (leftOut) {
          if (leftOut) say('Copied, without this page: players can’t see it. Change what copying shares to include it.', 'warn');
          else say(owner && currentPage() ? 'Copied this page. Paste it into your AI app.' : 'Copied. Paste it into your AI app.', 'ok');
        }).catch(function () {
          say('Couldn’t copy just now. Try again.', 'bad');
        }).then(function () { btn.disabled = false; });
      }

      function askToPaste() {
        var box = bubble.querySelector('[data-ai-paste-box]');
        if (!box) return;
        box.hidden = false;
        var area = box.querySelector('textarea');
        if (area) area.focus();
      }

      function doPaste() {
        if (!navigator.clipboard || !navigator.clipboard.readText) { askToPaste(); return; }
        navigator.clipboard.readText().then(review, askToPaste);
      }

      // review sends an AI's answer to the side sheet's hidden paste box.
      function review(text) {
        text = String(text || '');
        if (!BLOCK_RE.test(text)) {
          say('That isn’t an AI’s answer. Copy its whole answer, then press Paste answer again.', 'bad');
          return;
        }
        closeBubble(true);
        openPanel('').then(function () {
          var area = document.getElementById('ai-import-paste-area');
          if (!area || !area.form) return;
          var lp = area.form.querySelector('[data-ai-lookup-privacy]');
          if (lp) lp.value = seesOf(cid);
          area.value = text;
          area.dispatchEvent(new Event('input', { bubbles: true }));
          area.form.requestSubmit();
        });
      }

      function openRequest(id) {
        closeBubble(true);
        openPanel('').then(function () {
          var target = null;
          if (owner) {
            var input = panel.querySelector('input[name="ai_request"][value="' + id + '"]');
            if (input && input.form) { input.form.requestSubmit(); target = input.form; }
          } else {
            var btn = panel.querySelector('[hx-get$="/requests/' + id + '"]');
            if (btn) { btn.click(); target = btn; }
          }
          if (target) target.closest('section').scrollIntoView({ block: 'start' });
        });
      }

      // A link away from the page waits while it has unsaved changes, and says
      // so in the bubble with Stay and Leave without saving.
      function leaveFor(href) {
        if (!(Chronicle.isDirty && Chronicle.isDirty())) { location.href = href; return; }
        var warn = bubble.querySelector('[data-ai-bubble-warn]');
        if (!warn) return;
        warn.innerHTML = '';
        var box = document.createElement('div');
        box.className = 'aib-warn';
        box.setAttribute('role', 'alert');
        var p = document.createElement('p');
        p.innerHTML = '<b>This page has changes you haven’t saved.</b> Going to Your AI would lose them. Save them first, then come back here.';
        var row = document.createElement('div');
        row.className = 'aib-warn-row';
        var stay = document.createElement('button');
        stay.type = 'button';
        stay.className = 'btn-primary btn-sm';
        stay.textContent = 'Stay';
        stay.addEventListener('click', function () { warn.innerHTML = ''; });
        var go = document.createElement('button');
        go.type = 'button';
        go.className = 'quiet-word text-bad';
        go.textContent = 'Leave without saving';
        go.addEventListener('click', function () {
          if (Chronicle.discardChanges) Chronicle.discardChanges();
          location.href = href;
        });
        row.appendChild(stay); row.appendChild(go);
        box.appendChild(p); box.appendChild(row);
        warn.appendChild(box);
        stay.focus();
      }

      function onBubbleClick(e) {
        var t = e.target;
        var el;
        if (t.closest('[data-ai-bubble-close]')) { closeBubble(); return; }
        if ((el = t.closest('[data-ai-copy]'))) { doCopy(el); return; }
        if (t.closest('[data-ai-paste]')) { doPaste(); return; }
        if ((el = t.closest('[data-ai-review]'))) { openRequest(el.getAttribute('data-ai-review')); return; }
        if (t.closest('[data-ai-tool]')) { closeBubble(true); openPanel('tool'); return; }
        if (t.closest('[data-ai-connect]')) { closeBubble(true); openPanel('menu'); return; }
        if ((el = t.closest('a[data-ai-full]'))) { e.preventDefault(); leaveFor(el.href); }
      }

      function onBubbleSubmit(e) {
        if (!e.target.closest('[data-ai-paste-box]')) return;
        e.preventDefault();
        var area = e.target.querySelector('textarea');
        review(area ? area.value : '');
      }

      // --- The side sheet ---

      function load(mode) {
        if (state.loaded && state.mode === mode && reviewOpen()) return Promise.resolve();
        state.loaded = true;
        state.mode = mode;
        var url = root + '/panel' + (mode === 'tool' ? '?tool=1' : mode === 'menu' ? '?menu=1' : '');
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
          htmx.ajax('GET', url, { target: body, swap: 'innerHTML' }).catch(done);
        });
      }

      function openPanel(mode) {
        hideNote();
        closeBubble(true);
        if (!state.open) {
          state.open = true;
          state.back = chip;
          scrim.hidden = false; panel.hidden = false;
          void panel.offsetWidth;
          scrim.classList.add('is-open'); panel.classList.add('is-open');
          document.documentElement.classList.add('overflow-hidden');
          panel.focus();
        }
        return load(mode || '').then(function () {
          var x = panel.querySelector('[data-ai-close]');
          if (state.open && x && panel.contains(document.activeElement) === false) x.focus();
        });
      }

      function close() {
        if (!state.open) return;
        state.open = false;
        scrim.classList.remove('is-open'); panel.classList.remove('is-open');
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
        review(text);
      }

      function onKey(e) {
        if (e.key !== 'Escape') return;
        if (state.bubble) { e.preventDefault(); closeBubble(); return; }
        if (!state.open) return;
        // A pop-up inside the panel closes first.
        var inner = panel.querySelector('[role=dialog]:not(.ai-panel)');
        if (inner && inner.offsetParent !== null) return;
        e.preventDefault();
        close();
      }

      function onClick(e) {
        // The click's path, not host.contains: Look and Stay remove themselves
        // from the page before this runs, which would read as a click outside.
        if (state.bubble && e.composedPath().indexOf(host) < 0) closeBubble(true);
        if (e.target.closest('[data-ai-open]') && !host.hidden) { e.preventDefault(); openPanel(''); return; }
        if (state.open && e.target.closest('[data-ai-close]')) close();
      }

      chip.addEventListener('click', function () { if (state.bubble) closeBubble(); else openBubble(); });
      bubble.addEventListener('click', onBubbleClick);
      bubble.addEventListener('submit', onBubbleSubmit);
      scrim.addEventListener('click', close);
      document.addEventListener('click', onClick);
      document.addEventListener('paste', onPaste);
      document.addEventListener('keydown', onKey);
      var onVisible = function () { if (!document.hidden) poll(); };
      document.addEventListener('visibilitychange', onVisible);
      var onHash = function () { if (location.hash === '#copy-and-paste' && !host.hidden && owner) openPanel('tool'); };
      window.addEventListener('hashchange', onHash);
      state.timer = setInterval(poll, POLL_MS);
      host._aiChip = { off: function () {
        clearInterval(state.timer); clearTimeout(state.noteTimer); clearTimeout(state.saidTimer);
        document.removeEventListener('click', onClick);
        document.removeEventListener('paste', onPaste);
        document.removeEventListener('keydown', onKey);
        document.removeEventListener('visibilitychange', onVisible);
        window.removeEventListener('hashchange', onHash);
        document.documentElement.classList.remove('overflow-hidden');
        panel.remove(); scrim.remove();
      } };

      poll();
      if (location.hash === '#copy-and-paste' && !host.hidden && owner) openPanel('tool');
    },
    destroy: function (host) {
      if (host._aiChip) { host._aiChip.off(); host._aiChip = null; }
    }
  });
})();
