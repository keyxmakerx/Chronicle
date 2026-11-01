/**
 * What players changed
 *
 * The owner's "ask the history" box (internal/plugins/playerchanges/
 * player_changes.templ). Words typed into the box turn into tags when they
 * name something the page offered (a page, a field, "by an AI", "removed
 * things"); the rest stay as words. Every change to the tags, the words or
 * the time switch asks the server for the answer, which arrives as HTML and
 * replaces the results. The server decides what matches and what may be
 * undone; this file decides nothing about either. Text reaches the DOM only
 * through textContent, except the server's own results.
 */
(function () {
  'use strict';

  var WAIT = 180; // ms of quiet typing before asking

  // ---- Pure helpers (unit-tested in test/js/player_changes.test.mjs) ----

  /** findSuggestion: the offered tag whose label is exactly s, ignoring case. */
  function findSuggestion(list, s) {
    var want = String(s || '').trim().toLowerCase();
    if (!want) return null;
    for (var i = 0; i < list.length; i++) {
      if (String(list[i].label).toLowerCase() === want) return list[i];
    }
    return null;
  }

  /** offered: the tags to offer for typed text, leaving out chosen ones. */
  function offered(list, chosen, text) {
    var q = String(text || '').trim().toLowerCase();
    return list.filter(function (s) {
      for (var i = 0; i < chosen.length; i++) {
        if (chosen[i].kind === s.kind && chosen[i].value === s.value) return false;
      }
      return !q || String(s.label).toLowerCase().indexOf(q) >= 0;
    });
  }

  /** queryString: the results request for the tags, words and range. */
  function queryString(toks, text, range) {
    var p = [];
    toks.forEach(function (t) {
      if (t.kind === 'page') p.push('page=' + encodeURIComponent(t.value));
      else if (t.kind === 'field') p.push('field=' + encodeURIComponent(t.value));
      else if (t.kind === 'ai') p.push('ai=1');
      else if (t.kind === 'removed') p.push('removed=1');
    });
    var words = String(text || '').trim();
    if (words) p.push('q=' + encodeURIComponent(words));
    p.push('range=' + (range === 'long' ? 'long' : 'month'));
    return p.join('&');
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { findSuggestion: findSuggestion, offered: offered, queryString: queryString };
    return;
  }

  Chronicle.register('player-changes', {
    init: function (host) {
      var url = host.getAttribute('data-results-url') || '';
      var list = [];
      try { list = JSON.parse(host.getAttribute('data-suggestions') || '[]') || []; } catch (e) { list = []; }
      var input = host.querySelector('[data-pcx-input]');
      var toksEl = host.querySelector('[data-pcx-toks]');
      var sugEl = host.querySelector('[data-pcx-suggest]');
      var results = host.querySelector('[data-pcx-results]');
      var st = { toks: [], text: '', range: 'month', seq: 0, timer: 0 };

      function drawToks() {
        toksEl.textContent = '';
        st.toks.forEach(function (t, i) {
          var span = document.createElement('span');
          span.className = 'pcx-tok';
          span.appendChild(document.createTextNode(t.label));
          var x = document.createElement('button');
          x.type = 'button';
          x.textContent = '×';
          x.setAttribute('aria-label', 'Remove ' + t.label);
          x.addEventListener('click', function () { st.toks.splice(i, 1); drawAll(); ask(); input.focus(); });
          span.appendChild(x);
          toksEl.appendChild(span);
        });
      }

      function drawSuggest() {
        sugEl.textContent = '';
        offered(list, st.toks, st.text).slice(0, 12).forEach(function (s) {
          var b = document.createElement('button');
          b.type = 'button';
          b.textContent = '+ ' + s.label;
          b.addEventListener('click', function () { add(s); input.focus(); });
          sugEl.appendChild(b);
        });
      }

      function drawAll() { drawToks(); drawSuggest(); }

      function add(s) {
        st.toks.push(s);
        input.value = '';
        st.text = '';
        drawAll();
        ask();
      }

      function skeleton() {
        var wrap = document.createElement('div');
        wrap.className = 'pcx-cards';
        wrap.setAttribute('aria-hidden', 'true');
        for (var i = 0; i < 3; i++) {
          var d = document.createElement('div');
          d.className = 'pcx-skel';
          wrap.appendChild(d);
        }
        return wrap;
      }

      function ask() {
        clearTimeout(st.timer);
        var mine = ++st.seq;
        results.setAttribute('aria-busy', 'true');
        if (st.range === 'long') {
          var note = document.createElement('p');
          note.className = 'pcx-count';
          note.textContent = 'Unpacking older changes…';
          results.textContent = '';
          results.appendChild(note);
          results.appendChild(skeleton());
        }
        fetch(url + '?' + queryString(st.toks, st.text, st.range), {
          credentials: 'same-origin',
          headers: { 'HX-Request': 'true', Accept: 'text/html' }
        }).then(function (r) {
          if (!r.ok) throw new Error('status ' + r.status);
          return r.text();
        }).then(function (html) {
          if (mine !== st.seq) return; // a newer question is on its way
          results.innerHTML = html;
          results.setAttribute('aria-busy', 'false');
          if (window.htmx) window.htmx.process(results);
        }).catch(function () {
          if (mine !== st.seq) return;
          results.setAttribute('aria-busy', 'false');
          results.textContent = '';
          var p = document.createElement('p');
          p.className = 'pcx-empty';
          p.textContent = 'Chronicle couldn’t search just now. Try again in a moment.';
          results.appendChild(p);
        });
      }

      function onInput() {
        st.text = input.value;
        var exact = /\s$/.test(input.value) && findSuggestion(offered(list, st.toks, ''), input.value);
        if (exact) { add(exact); return; }
        drawSuggest();
        clearTimeout(st.timer);
        st.timer = setTimeout(ask, WAIT);
      }

      function onKey(e) {
        if (e.key === 'Backspace' && !input.value && st.toks.length) {
          st.toks.pop();
          drawAll();
          ask();
        } else if (e.key === 'Enter') {
          e.preventDefault();
          var only = offered(list, st.toks, input.value);
          var exact = findSuggestion(only, input.value) || (only.length === 1 && input.value.trim() ? only[0] : null);
          if (exact) add(exact); else ask();
        }
      }

      function onRange(e) {
        var b = e.target.closest('[data-pcx-range]');
        if (!b) return;
        var r = b.getAttribute('data-pcx-range');
        host.querySelectorAll('[data-pcx-range]').forEach(function (x) { x.setAttribute('aria-pressed', String(x === b)); });
        if (r !== st.range) { st.range = r; ask(); }
      }

      input.addEventListener('input', onInput);
      input.addEventListener('keydown', onKey);
      host.addEventListener('click', onRange);
      drawAll();
      ask();

      host._pcxOff = function () {
        clearTimeout(st.timer);
        st.seq++;
        input.removeEventListener('input', onInput);
        input.removeEventListener('keydown', onKey);
        host.removeEventListener('click', onRange);
      };
    },
    destroy: function (host) {
      if (host._pcxOff) host._pcxOff();
    }
  });
})();
