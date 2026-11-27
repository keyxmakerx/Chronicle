/**
 * homebrew_asks.js: a person's own asks to the Director. A house rule with an
 * "Ask the Director" step waits for an answer; this widget shows each of the
 * viewer's asks while it waits ("Your Director is answering", with "Take back
 * my ask") and what came back: yes, yes with changes, no, or a written reply.
 * A "The player rolls first" step waiting on the viewer shows a roll button
 * ("Roll Might (2d10 + 2)"); the server rolls, judges and answers with the
 * result, which this shows ("Rolled 15. The rule carries on.").
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

  // spaced writes a formula the way people read it: "2d10 + 2".
  function spaced(f) {
    return String(f || '').replace(/\s*([+-])\s*/g, ' $1 ').trim();
  }

  // rollButton is what the roll button says: "Roll Might (2d10 + 2)".
  function rollButton(r) {
    if (!r) return 'Roll';
    return r.label ? 'Roll ' + r.label + ' (' + spaced(r.formula) + ')' : 'Roll ' + spaced(r.formula);
  }

  // condition is the roll as a condition: "roll Might 12 or more".
  function condition(r) {
    if (!r) return 'roll first';
    return r.label ? 'roll ' + r.label + ' ' + r.target + ' or more' : 'roll ' + r.target + ' or more on ' + spaced(r.formula);
  }

  // result is the line after the roll, from the server's total.
  function result(a) {
    var r = a.roll || {};
    if (a.state === 'rolled') return 'Rolled ' + r.total + '. The rule carries on.';
    if (a.state === 'missed') return 'Rolled ' + r.total + ', under ' + r.target + '. The rule stops here.';
    return '';
  }

  // pill is the state's short word and its colour class.
  function pill(a, gm) {
    switch (a.state) {
      case 'roll': return a.answered === 'yes' ? ['Yes, with a step first', 'ok'] : ['Your roll first', 'wait'];
      case 'rolled': return ['Made the roll', 'ok'];
      case 'missed': return ['Roll fell short', 'bad'];
      case 'set_aside': return ['Set aside', 'off'];
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
      case 'roll': case 'rolled': case 'missed':
        var c = condition(a.roll);
        return a.answered === 'yes' ? 'Yes, if you ' + c + ' first.' : upper(c) + ' first.';
      case 'set_aside': return 'Set aside before the roll. The rule stops here.';
      case 'yes': return (a.changes && a.changes.length) ? 'Yes, with these changes:' : 'Yes. The rule goes on.';
      case 'no': return 'No. The rule stops here.';
      case 'other': return a.reply || '';
    }
    return '';
  }

  function upper(t) { return t ? t.charAt(0).toUpperCase() + t.slice(1) : t; }

  var H = { title: title, pill: pill, said: said, rollButton: rollButton, condition: condition, result: result, spaced: spaced };
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
    // After a roll its button is gone; the result takes the focus it had.
    if (S.focus) {
      var res = el.querySelector('[data-hba-result="' + S.focus + '"]');
      if (res) { res.focus(); S.focus = ''; }
    }
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
    if (a.state === 'roll' || a.state === 'rolled' || a.state === 'missed') return rollCard(S, a, li);
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

  // rollCard is a roll step: what the ask's answer said, the roll button
  // while it waits on this viewer, then the server's result. The total
  // always comes from the server; this page only asks for the roll.
  function rollCard(S, a, li) {
    var box = h('div', 'hba-said');
    if (a.answered) box.appendChild(h('p', 'hba-said-h', 'Your ' + S.gm + ' said'));
    box.appendChild(h('p', 'hba-said-t', said(a)));
    if (a.changes && a.changes.length) {
      var ul = h('ul', 'hba-changes');
      a.changes.forEach(function (c) { ul.appendChild(h('li', '', c)); });
      box.appendChild(ul);
    }
    if (a.reply) box.appendChild(h('p', 'hba-said-t', a.reply));
    li.appendChild(box);
    if (a.state === 'roll') {
      var r = a.roll || {};
      if (a.canRoll) {
        li.appendChild(h('button', 'hba-rollbtn', S.busy === a.id ? 'Rolling…' : rollButton(r),
          { type: 'button', disabled: !!S.busy, 'data-hba-roll': a.id }, { click: function () { roll(S, a); } }));
        li.appendChild(h('p', 'hba-muted', 'Chronicle rolls the dice and checks the total. Under ' + r.target + ', the rule stops and says so.'));
      } else if (r.problem) {
        li.appendChild(h('p', 'hba-err', r.problem));
      } else {
        li.appendChild(h('p', 'hba-muted', 'Waiting for the player to roll.'));
      }
      if (S.error && S.busy === '' && S.errorFor === a.id) li.appendChild(h('p', 'hba-err', S.error, { role: 'alert' }));
      return li;
    }
    li.appendChild(h('p', 'hba-result hba-' + a.state, result(a), { tabindex: '-1', 'data-hba-result': a.id }));
    return li;
  }

  function roll(S, a) {
    S.busy = a.id; S.error = ''; draw(S);
    api(S.cfg.endpoint + '/asks/' + encodeURIComponent(a.id) + '/roll', 'POST', csrf(S)).then(function (got) {
      S.busy = '';
      if (got && got.id) {
        S.asks = S.asks.map(function (x) { return x.id === got.id ? got : x; });
        S.focus = got.id;
      }
    }).catch(function (e) {
      S.busy = ''; S.error = e.message; S.errorFor = a.id;
    }).then(function () { return load(S); });
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
