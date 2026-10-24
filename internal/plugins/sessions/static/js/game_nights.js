/**
 * Game nights
 *
 * The behaviour of the Game nights list (internal/plugins/sessions/
 * sessions.templ, styles in static/css/game_nights.css), for members:
 *
 *   - "Add to your calendar" drapes down a panel under the heading with the
 *     member's private calendar link, Copy, and Replace link behind an inline
 *     confirm. The link is made the first time the panel opens, so a member
 *     who never opens it never has one. Shown only while the owner's
 *     calendar feed switch is on; the server decides that.
 *   - A row's Answer button (or, once answered, their answer, which is the
 *     button to change it) drapes Going, Maybe and Can't down inside the row.
 *     A pick is saved at once; the row then shows the new answer and its
 *     attending count.
 *
 * Data: POST sessions/feed/token and sessions/feed/token/replace (the
 * member's own link, as a path), POST sessions/:sid/rsvp. This file makes no
 * permission decision: the server renders these controls only for a member,
 * and the routes check again. Text reaches the DOM only through textContent.
 * Every panel has a visible Close or closes from the button that opened it;
 * a click outside never closes one.
 */
(function () {
  'use strict';

  // ---- Pure helpers (unit-tested in test/js/game_nights.test.mjs) ----

  var ANSWERS = {
    accepted: { word: 'Going', icon: 'fa-check' },
    tentative: { word: 'Maybe', icon: 'fa-question' },
    declined: { word: "Can't", icon: 'fa-xmark' }
  };

  /** answerWord: the word for a stored answer, or '' for none or an unknown one. */
  function answerWord(status) {
    return Object.prototype.hasOwnProperty.call(ANSWERS, status) ? ANSWERS[status].word : '';
  }

  /**
   * countAfter: the row's "N/M attending" once `prev` becomes `next`. A
   * member with no answer row yet ('' or unknown) gains one, so M grows.
   */
  function countAfter(accepted, total, prev, next) {
    var a = accepted, t = total;
    if (!answerWord(prev) && prev !== 'invited') t += 1;
    if (prev === 'accepted') a -= 1;
    if (next === 'accepted') a += 1;
    return { accepted: Math.max(0, a), total: Math.max(t, a) };
  }

  /**
   * feedURL: the full address a calendar app subscribes to. The server
   * answers a path; anything else (an absolute URL, a protocol-relative one)
   * is refused, so the field can only ever hold this site's own link.
   */
  function feedURL(origin, path) {
    if (typeof path !== 'string' || !/^\/sessions\/feed\/[A-Za-z0-9_-]+\.ics$/.test(path)) return '';
    return String(origin || '').replace(/\/+$/, '') + path;
  }

  var api = { answerWord: answerWord, countAfter: countAfter, feedURL: feedURL };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof window === 'undefined' || typeof document === 'undefined') return;

  // ---- Browser part ----

  function notify(msg, kind) {
    if (window.Chronicle && Chronicle.notify) Chronicle.notify(msg, kind || 'success');
  }

  function errorOf(res, fallback) {
    return res.json().catch(function () { return {}; }).then(function (body) {
      throw new Error(body.error || body.message || fallback);
    });
  }

  function post(url, body) {
    return Chronicle.apiFetch(url, { method: 'POST', body: body || {} });
  }

  function setOpen(drape, opener, on) {
    drape.classList.toggle('gn-on', on);
    if (opener) opener.setAttribute('aria-expanded', String(on));
  }

  // -- The calendar link --

  function feedOpen(root, on) {
    var drape = root.querySelector('[data-gn-feed]');
    var tog = root.querySelector('[data-gn-feed-toggle]');
    if (!drape) return;
    setOpen(drape, tog, on);
    if (!on) {
      var conf = drape.querySelector('[data-gn-confirm]');
      if (conf) conf.hidden = true;
      return;
    }
    var field = drape.querySelector('[data-gn-feed-url]');
    if (field && !field.value && !root.__gnFeedLoading) loadFeed(root, drape, false);
  }

  function loadFeed(root, drape, replace) {
    var field = drape.querySelector('[data-gn-feed-url]');
    var base = root.getAttribute('data-gn-base');
    var url = base + '/sessions/feed/token' + (replace ? '/replace' : '');
    root.__gnFeedLoading = true;
    if (!replace) field.placeholder = 'Making your link…';
    return post(url)
      .then(function (res) {
        if (!res.ok) return errorOf(res, 'Your calendar link could not be made. Try again.');
        return res.json();
      }, function () {
        throw new Error('Your calendar link could not be made. Check your connection and try again.');
      })
      .then(function (body) {
        var full = feedURL(window.location.origin, body && body.url);
        if (!full) throw new Error('Your calendar link could not be made. Try again.');
        field.value = full;
        if (replace) notify('New link made. The old one no longer works.');
      })
      .catch(function (err) { field.placeholder = ''; notify(err.message, 'error'); })
      .then(function () { root.__gnFeedLoading = false; });
  }

  function copyFeed(btn, field) {
    if (!field.value) return;
    function done() {
      btn.textContent = 'Copied';
      setTimeout(function () { btn.textContent = 'Copy'; }, 1500);
    }
    function fallback() {
      field.focus();
      field.select();
      var ok = false;
      try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
      if (ok) done(); else notify('Press and hold the link to copy it.', 'info');
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(field.value).then(done, fallback);
    } else {
      fallback();
    }
  }

  // -- Answering a night --

  function rowParts(row) {
    return {
      drape: row.querySelector('[data-gn-drape]'),
      chip: row.querySelector('[data-gn-chip]'),
      count: row.querySelector('[data-gn-count]'),
      buttons: row.querySelectorAll('[data-gn-rsvp]')
    };
  }

  // The answered chip: their answer as a badge that is itself the button.
  function answeredButton(status, drapeID, name) {
    var b = document.createElement('button');
    b.type = 'button';
    b.className = 'gn-mine gn-mine-' + status;
    b.setAttribute('data-gn-answer', '');
    b.setAttribute('aria-expanded', 'false');
    b.setAttribute('aria-controls', drapeID);
    b.setAttribute('aria-label', 'Your answer for ' + name + ': ' + answerWord(status) + '. Change it');
    b.title = 'Change your answer';
    var i = document.createElement('i');
    i.className = 'fa-solid ' + ANSWERS[status].icon;
    i.setAttribute('aria-hidden', 'true');
    b.appendChild(i);
    b.appendChild(document.createTextNode(' ' + answerWord(status)));
    return b;
  }

  function answer(row, btn) {
    var status = btn.getAttribute('data-gn-rsvp');
    if (!answerWord(status) || row.__gnSaving) return;
    var p = rowParts(row);
    var prev = row.getAttribute('data-gn-mine') || '';
    row.__gnSaving = true;
    Array.prototype.forEach.call(p.buttons, function (b) { b.disabled = true; });
    post(row.getAttribute('data-gn-url'), { status: status })
      .then(function (res) {
        if (!res.ok) return errorOf(res, 'Your answer could not be saved. Try again.');
      }, function () {
        throw new Error('Your answer could not be saved. Check your connection and try again.');
      })
      .then(function () {
        row.setAttribute('data-gn-mine', status);
        Array.prototype.forEach.call(p.buttons, function (b) {
          b.setAttribute('aria-pressed', String(b === btn));
        });
        if (p.count) {
          var n = countAfter(+p.count.getAttribute('data-accepted') || 0, +p.count.getAttribute('data-total') || 0, prev, status);
          p.count.setAttribute('data-accepted', String(n.accepted));
          p.count.setAttribute('data-total', String(n.total));
          p.count.textContent = ' ' + n.accepted + '/' + n.total + ' attending';
        }
        var fresh = answeredButton(status, p.drape.id, row.getAttribute('data-gn-name') || 'this night');
        p.chip.textContent = '';
        p.chip.appendChild(fresh);
        setOpen(p.drape, fresh, false);
        fresh.focus({ preventScroll: true });
        notify('Saved: ' + answerWord(status));
      })
      .catch(function (err) { notify(err.message, 'error'); })
      .then(function () {
        row.__gnSaving = false;
        Array.prototype.forEach.call(p.buttons, function (b) { b.disabled = false; });
      });
  }

  // -- Wiring --

  function onClick(e) {
    var root = this, t = e.target;
    if (!t || !t.closest) return;
    var hit;
    if ((hit = t.closest('[data-gn-feed-toggle]'))) {
      feedOpen(root, hit.getAttribute('aria-expanded') !== 'true');
    } else if (t.closest('[data-gn-feed-close]')) {
      feedOpen(root, false);
      var tog = root.querySelector('[data-gn-feed-toggle]');
      if (tog) tog.focus({ preventScroll: true });
    } else if ((hit = t.closest('[data-gn-copy]'))) {
      copyFeed(hit, root.querySelector('[data-gn-feed-url]'));
    } else if (t.closest('[data-gn-replace]')) {
      var conf = root.querySelector('[data-gn-confirm]');
      conf.hidden = false;
      var no = conf.querySelector('[data-gn-replace-no]');
      if (no) no.focus({ preventScroll: true });
    } else if (t.closest('[data-gn-replace-no]')) {
      root.querySelector('[data-gn-confirm]').hidden = true;
      root.querySelector('[data-gn-replace]').focus({ preventScroll: true });
    } else if (t.closest('[data-gn-replace-yes]')) {
      root.querySelector('[data-gn-confirm]').hidden = true;
      var drape = root.querySelector('[data-gn-feed]');
      if (!root.__gnFeedLoading) loadFeed(root, drape, true);
    } else if ((hit = t.closest('[data-gn-answer]'))) {
      var row = hit.closest('[data-gn-row]');
      var d = row && row.querySelector('[data-gn-drape]');
      if (d) setOpen(d, hit, hit.getAttribute('aria-expanded') !== 'true');
    } else if ((hit = t.closest('[data-gn-rsvp]'))) {
      var r = hit.closest('[data-gn-row]');
      if (r) answer(r, hit);
    } else {
      return;
    }
    e.preventDefault();
  }

  if (window.Chronicle && window.Chronicle.register) {
    window.Chronicle.register('game-nights', {
      init: function (el) {
        el.__gnClick = onClick.bind(el);
        el.addEventListener('click', el.__gnClick);
      },
      destroy: function (el) {
        if (el.__gnClick) el.removeEventListener('click', el.__gnClick);
        el.__gnClick = null;
      }
    });
  }
})();
