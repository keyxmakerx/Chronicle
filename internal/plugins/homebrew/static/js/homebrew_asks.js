/**
 * homebrew_asks.js: a person's own asks to the Director. A house rule with an
 * "Ask the Director" step waits for an answer; this widget shows each of the
 * viewer's asks while it waits ("Your Director is answering", with "Take back
 * my ask") and what came back: yes, yes with changes, no, or a written reply.
 *
 * It reads GET <endpoint>/asks/mine, which holds only the viewer's own words
 * and the answer, never the recipe or its rule, and reads it again when the
 * Table's socket says an ask changed (chronicle:homebrew-asks). It can be
 * mounted anywhere a player's own asks belong. User text goes in with
 * textContent, never as markup. Styles are in ../css/asks.css.
 */
(function () {
  'use strict';

  // ---------- Pure helpers (tested in test/js/homebrew_asks.test.mjs) ----------

  // title is an ask's heading: the author's words with a capital, or a
  // plain fallback when the rule gave none.
  function title(a) {
    var w = (a && a.ask) || '';
    return w ? w.charAt(0).toUpperCase() + w.slice(1) : 'Your ask';
  }

  // pill is the state's short word and its colour class.
  function pill(a, gm) {
    switch (a.state) {
      case 'waiting': return ['Your ' + gm + ' is answering', 'wait'];
      case 'yes': return [(a.changes && a.changes.length) ? 'Yes, with changes' : 'Yes', 'ok'];
      case 'no': return ['No', 'bad'];
      case 'other': return ['Replied', 'note'];
      case 'taken_back': return ['Taken back', 'off'];
    }
    return [a.state, 'off'];
  }

  // said is the first line of what came back, or "" while it waits.
  function said(a) {
    switch (a.state) {
      case 'yes': return (a.changes && a.changes.length) ? 'Yes, with these changes:' : 'Yes. The rule goes on.';
      case 'no': return 'No. The rule stops here.';
      case 'other': return a.reply || '';
    }
    return '';
  }

  var H = { title: title, pill: pill, said: said };
  if (typeof window !== 'undefined') window.HomebrewAsks = H;
  if (typeof Chronicle === 'undefined' || !Chronicle.register) return;

  function h(tag, cls, text, attrs, on) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null && text !== '') e.textContent = text;
    if (attrs) Object.keys(attrs).forEach(function (k) {
      if (attrs[k] === false || attrs[k] == null) return;
      e.setAttribute(k, attrs[k] === true ? '' : attrs[k]);
    });
    if (on) Object.keys(on).forEach(function (k) { e.addEventListener(k, on[k]); });
    return e;
  }

  function api(url, method, csrf) {
    return Chronicle.apiFetch(url, { method: method || 'GET', csrfToken: csrf }).then(function (res) {
      return res.text().then(function (txt) {
        var data = null;
        try { data = txt ? JSON.parse(txt) : null; } catch (e) { data = null; }
        if (!res.ok) throw new Error((data && (data.message || data.error)) || 'Something went wrong. Try again.');
        return data;
      });
    });
  }

  Chronicle.register('homebrew-asks', {
    init: function (el, cfg) {
      var S = { el: el, cfg: cfg, gm: cfg.gm || 'Director', asks: null, busy: '', error: '' };
      el._hba = S;
      S.onAsk = function () { load(S); };
      window.addEventListener('chronicle:homebrew-asks', S.onAsk);
      load(S);
    },
    destroy: function (el) {
      if (el._hba) window.removeEventListener('chronicle:homebrew-asks', el._hba.onAsk);
      el._hba = null;
    }
  });

  function csrf(S) { return S.cfg.csrfToken || (Chronicle.getCsrf ? Chronicle.getCsrf() : ''); }

  function load(S) {
    return api(S.cfg.endpoint + '/asks/mine').then(function (d) {
      S.asks = (d && d.data) || []; draw(S);
    }, function () {
      if (S.asks === null) S.asks = false;
      draw(S);
    });
  }

  function draw(S) {
    var el = S.el;
    el.textContent = '';
    if (S.asks === false) {
      el.appendChild(h('p', 'hba-muted', 'Your asks could not be read. Reload the page to try again.'));
      return;
    }
    if (!S.asks.length) {
      el.appendChild(h('p', 'hba-muted', 'You have no asks waiting or answered.'));
      return;
    }
    var list = h('ul', 'hba-list', '', { 'aria-live': 'polite' });
    S.asks.forEach(function (a) { list.appendChild(card(S, a)); });
    el.appendChild(list);
  }

  function card(S, a) {
    var li = h('li', 'hba-card hba-' + a.state, '', { 'aria-label': title(a) });
    li.appendChild(h('h3', 'hba-title', title(a)));
    li.appendChild(h('p', 'hba-muted', 'From your ' + S.gm + '’s house rules'));
    var p = pill(a, S.gm);
    li.appendChild(h('span', 'hba-pill hba-' + p[1], p[0]));
    if (a.state === 'waiting') {
      li.appendChild(h('p', 'hba-aside', 'You can keep playing. This tells you when the answer comes.'));
      if (a.canTakeBack) {
        var b = h('button', 'hba-quiet', S.busy === a.id ? 'Taking it back…' : 'Take back my ask', { type: 'button', disabled: !!S.busy },
          { click: function () { takeBack(S, a); } });
        li.appendChild(b);
      }
      if (S.error && S.busy === '' && S.errorFor === a.id) li.appendChild(h('p', 'hba-err', S.error, { role: 'alert' }));
      return li;
    }
    if (a.state === 'taken_back') return li;
    var box = h('div', 'hba-said');
    box.appendChild(h('p', 'hba-said-h', 'Your ' + S.gm + ' said'));
    box.appendChild(h('p', 'hba-said-t', said(a)));
    if (a.changes && a.changes.length) {
      var ul = h('ul', 'hba-changes');
      a.changes.forEach(function (c) { ul.appendChild(h('li', '', c)); });
      box.appendChild(ul);
    }
    if (a.state !== 'other' && a.reply) box.appendChild(h('p', 'hba-said-t', a.reply));
    li.appendChild(box);
    return li;
  }

  function takeBack(S, a) {
    S.busy = a.id; S.error = ''; draw(S);
    api(S.cfg.endpoint + '/asks/' + encodeURIComponent(a.id) + '/take-back', 'POST', csrf(S)).then(function () {
      S.busy = '';
      if (Chronicle.notify) Chronicle.notify('Ask taken back.', 'success');
    }).catch(function (e) {
      S.busy = ''; S.error = e.message; S.errorFor = a.id;
    }).then(function () { return load(S); });
  }
})();
