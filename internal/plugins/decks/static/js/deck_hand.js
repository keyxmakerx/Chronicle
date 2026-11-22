/**
 * Deck hand
 *
 * The player's hand (internal/plugins/decks/hand*.templ). The server draws
 * every fragment from the player types; this file only sends the player's
 * moves to the deck routes and shows what the server then says. One widget:
 *
 *   deck-hand  mounts on the Hand leaf of the Table and on the Cards part of a
 *              hero's page. It turns a face-down card over, opens a face-up
 *              card at full size with its controls, draws, and fetches its own
 *              view again when the campaign's socket says "your decks view
 *              changed" (the Table dispatches chronicle:decks-changed; the
 *              message carries nothing).
 *
 * A face is requested only by a tap on a face-down card, and only through the
 * one request that both turns the copy and returns its face; no card's name,
 * words, kind or picture is in the page, an attribute or a template before
 * that. A refused move changes nothing on screen, says why in plain words and
 * puts focus back where it was. Text from the server reaches the DOM only as
 * its own markup or through textContent.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});

  var LIVE_EVENT = 'chronicle:decks-changed';
  var REFRESH_WAIT = 150; // ms live messages are gathered before one refetch
  var TURNED_DOWN = 'Your Director turned that down. You can ask again.';
  var REFRESH_FAIL = 'Your hand could not be refreshed.';
  var HEADERS = { 'HX-Request': 'true', 'Accept': 'text/html, application/json' };

  // ---- Pure helpers (unit-tested in test/js/deck_hand.test.mjs) ----

  /** urls: every address the widget calls, built from the widget's base. */
  function urls(base, deckID, copyID) {
    var d = base + '/' + encodeURIComponent(deckID);
    return {
      turn: base + '/hand/' + encodeURIComponent(deckID) + '/copies/' + encodeURIComponent(copyID) + '/turn',
      open: base + '/hand/' + encodeURIComponent(deckID) + '/copies/' + encodeURIComponent(copyID),
      verb: function (v) { return d + '/copies/' + encodeURIComponent(copyID) + '/' + v; },
      draw: d + '/draw'
    };
  }

  /** words: a refusal as a sentence, never a status code. */
  function words(err) {
    var m = err && err.message ? String(err.message) : '';
    if (!m || /^request failed|^status \d|Failed to fetch|NetworkError|Load failed/i.test(m)) return 'That did not go through. Check your connection and try again.';
    return /[.!?]$/.test(m) ? m : m + '.';
  }

  /** moveFor: the fixed move a verb plays on the card, or '' for none. */
  function moveFor(verb, asked) {
    if (verb === 'play') return asked ? '' : 'play';
    if (verb === 'discard') return 'used';
    if (verb === 'show') return 'ready';
    return '';
  }

  /** waitingIn: the copy ids a root shows as waiting for a Director. */
  function waitingIn(root) {
    var out = [];
    Array.prototype.forEach.call(root.querySelectorAll('[data-dkh-slot]'), function (s) {
      if (s.querySelector('.dkh-chip-wait')) out.push(s.getAttribute('data-copy'));
    });
    return out;
  }

  /** copiesIn: every copy id a root shows. */
  function copiesIn(root) {
    var out = [];
    Array.prototype.forEach.call(root.querySelectorAll('[data-dkh-slot]'), function (s) { out.push(s.getAttribute('data-copy')); });
    return out;
  }

  /** heldIn: the copy ids a root shows in a hand (not in play, on the table or used). */
  function heldIn(root) {
    var out = [];
    Array.prototype.forEach.call(root.querySelectorAll('[data-dkh-held] [data-dkh-slot]'), function (s) { out.push(s.getAttribute('data-copy')); });
    return out;
  }

  /**
   * turnedDown: copies that were waiting, are still in a hand and are no
   * longer waiting. A play a Director let through leaves the hand (it lands in
   * play, on the table or in the used pile), so it is never read as a refusal.
   */
  function turnedDown(before, rootAfter) {
    var held = heldIn(rootAfter), still = waitingIn(rootAfter);
    return before.filter(function (id) { return held.indexOf(id) >= 0 && still.indexOf(id) < 0; });
  }

  // ---- Small DOM helpers ----

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }

  /** fail: an Error carrying the server's own sentence from a refused request. */
  function refusal(res) {
    return res.text().then(function (text) {
      var data = {};
      try { data = JSON.parse(text); } catch (e) { data = {}; }
      var msg = (data && (data.message || data.error)) || '';
      var e = new Error(res.status === 404 ? 'That card is not there any more.' : msg);
      e.status = res.status;
      throw e;
    });
  }

  function send(method, url, body) {
    var opts = { method: method, headers: HEADERS };
    if (body !== undefined) opts.body = body;
    return C.apiFetch(url, opts).then(function (res) { return res.ok ? res : refusal(res); });
  }

  function working(btn, on) { if (C.working) C.working(btn, on); }

  function adopt(node) {
    if (C.mountWidgets) C.mountWidgets(node);
  }

  C.register('deck-hand', {
    init: function (el) {
      var cid = el.getAttribute('data-campaign-id');
      var base = el.getAttribute('data-base') || '/campaigns/' + encodeURIComponent(cid) + '/decks';
      var src = el.getAttribute('data-src');
      var inst = {};
      el.__dkhInst = inst;
      var timer = 0;
      var inFlight = false;
      var again = false;
      var overlay = null;
      var opener = null;
      var said = {};

      function say(box, msg) {
        if (!box) return;
        box.textContent = msg || '';
        box.hidden = !msg;
        if (msg && !box.querySelector('button')) {
          var b = document.createElement('button');
          b.type = 'button';
          b.className = 'dkh-word';
          b.textContent = 'Dismiss';
          b.addEventListener('click', function () { box.hidden = true; });
          box.appendChild(document.createTextNode(' '));
          box.appendChild(b);
        }
      }
      function errBox() { return (overlay && $('[data-dkh-err]', overlay)) || $('[data-dkh-err]', el); }
      function slotOf(node) { return node && node.closest ? node.closest('[data-dkh-slot]') : null; }

      // ---- Refreshing the hand ----

      /** focusKey: where focus is, by copy and kind of control, so a redraw can put it back. */
      function focusKey() {
        var a = document.activeElement;
        if (!a || !el.contains(a) || (overlay && overlay.contains(a))) return null;
        var slot = slotOf(a);
        if (slot) return { copy: slot.getAttribute('data-copy'), kind: a.hasAttribute('data-dkh-open') ? '[data-dkh-open]' : '.dk-turn' };
        if (a.hasAttribute('data-dkh-draw')) return { draw: a.getAttribute('data-deck') };
        return null;
      }
      function restoreFocus(k) {
        if (!k) return;
        var t = null;
        if (k.copy) {
          var slot = $$('[data-dkh-slot]', el).filter(function (s) { return s.getAttribute('data-copy') === k.copy; })[0];
          t = slot && (slot.querySelector(k.kind) || slot.querySelector('[data-dkh-open]') || slot.querySelector('.dk-turn'));
        } else if (k.draw) {
          t = $$('[data-dkh-draw]', el).filter(function (b) { return b.getAttribute('data-deck') === k.draw; })[0];
        }
        if (t && t.focus) t.focus({ preventScroll: true });
      }

      /** arrive: a copy that was not in the hand before plays the deal move. */
      function arrive(before) {
        var DC = window.DeckCards;
        if (!DC || !DC.move) return;
        $$('[data-dkh-slot]', el).forEach(function (s) {
          if (before.indexOf(s.getAttribute('data-copy')) >= 0) return;
          var card = $('[data-dk-card]', s);
          if (card) DC.move(card, 'deal', { dx: 0, dy: -36 });
        });
      }

      function swap(html) {
        var doc = new DOMParser().parseFromString('<div>' + html + '</div>', 'text/html');
        var body = $('[data-dkh-body]', doc), notes = $('[data-dkh-notes]', doc);
        var oldBody = $('[data-dkh-body]', el), oldNotes = $('[data-dkh-notes]', el);
        if (!body || !oldBody) throw new Error('no body');
        var keep = focusKey();
        var before = copiesIn(el), waiting = waitingIn(el);
        // What this page told the player stays until they dismiss it; the
        // server's own lines under the hand are redrawn.
        var kept = oldNotes ? $$('[data-dkh-told]', oldNotes) : [];
        oldBody.replaceWith(body);
        if (notes && oldNotes) {
          oldNotes.replaceWith(notes);
          kept.forEach(function (n) { notes.appendChild(n); });
        }
        adopt(body);
        restoreFocus(keep);
        arrive(before);
        turnedDown(waiting, el).forEach(function (id) {
          if (said[id]) return;
          said[id] = true;
          var n = $('[data-dkh-notes]', el);
          if (n) {
            var p = document.createElement('p');
            p.className = 'dkh-note';
            p.setAttribute('data-dkh-told', '');
            p.textContent = TURNED_DOWN + ' ';
            var x = document.createElement('button');
            x.type = 'button';
            x.className = 'dkh-word';
            x.textContent = 'Dismiss';
            x.addEventListener('click', function () { p.parentNode.removeChild(p); });
            p.appendChild(x);
            n.appendChild(p);
          }
        });
        // A copy that is waiting again can be told of again next time.
        waitingIn(el).forEach(function (id) { delete said[id]; });
      }

      /** refresh: fetch the hand's own view again; one at a time, the last ask wins. */
      function refresh() {
        if (!src) return Promise.resolve();
        if (inFlight) { again = true; return Promise.resolve(); }
        inFlight = true;
        el.setAttribute('aria-busy', 'true');
        return fetch(src, { credentials: 'same-origin', headers: HEADERS }).then(function (r) {
          if (!r.ok) throw new Error('status ' + r.status);
          return r.text();
        }).then(function (html) {
          swap(html);
          say($('[data-dkh-err]', el), '');
          if (overlay) return reopen();
        }).catch(function () {
          say($('[data-dkh-err]', el), REFRESH_FAIL);
        }).then(function () {
          inFlight = false;
          el.removeAttribute('aria-busy');
          if (again) { again = false; return refresh(); }
        });
      }
      inst.refresh = refresh;

      function later() {
        clearTimeout(timer);
        timer = setTimeout(refresh, REFRESH_WAIT);
      }
      function onLive() {
        if (el.isConnected) { later(); return; }
        // Taken off the page without a teardown: stop listening for good.
        clearTimeout(timer);
        window.removeEventListener(LIVE_EVENT, onLive);
        close(false);
      }
      window.addEventListener(LIVE_EVENT, onLive);
      inst.onLive = onLive;
      inst.stop = function () { clearTimeout(timer); };

      // ---- Turning a face-down card over ----

      function turn(btn) {
        var slot = slotOf(btn), card = btn.closest('[data-dk-card]');
        var DC = window.DeckCards;
        if (!slot || !card || !DC || !DC.turnOver) return;
        var u = urls(base, slot.getAttribute('data-deck'), slot.getAttribute('data-copy'));
        var failure = null;
        function loadFace() {
          return send('POST', u.turn).then(function (r) { return r.text(); }, function (e) { failure = e; throw e; });
        }
        say(errBox(), '');
        DC.turnOver(card, loadFace).then(function () {
          if (failure) {
            say(errBox(), words(failure));
            // A stale card (already turned, moved, gone) is read again; the words stay.
            if (failure.status === 404 || failure.status === 409) later();
            return;
          }
          later();
        });
      }

      // ---- Opening a face-up card ----

      function close(toOpener) {
        if (!overlay) return;
        overlay.remove();
        overlay = null;
        var o = opener;
        opener = null;
        if (toOpener !== false && o && o.isConnected && o.focus) o.focus({ preventScroll: true });
      }

      inst.close = close;

      function place(html, focusIt) {
        var doc = new DOMParser().parseFromString('<div>' + html + '</div>', 'text/html');
        var fresh = $('[data-dkh-over]', doc);
        if (!fresh) throw new Error('no card');
        var had = overlay;
        // A card read again in place must not slide in a second time.
        // The sheet lives on the page itself: the Table drawer is a moving
        // box, and a fixed layer inside it would be clipped to it.
        fresh.addEventListener('click', inst.onClick);
        fresh.addEventListener('keydown', inst.onKey);
        if (had) { fresh.classList.add('dkh-again'); had.replaceWith(fresh); } else document.body.appendChild(fresh);
        overlay = fresh;
        adopt(fresh);
        if (focusIt) {
          var first = $('[data-dkh-act]', fresh) || $('[data-dkh-close]', fresh);
          if (first) first.focus({ preventScroll: true });
        }
      }

      function openCard(btn) {
        var slot = slotOf(btn);
        if (!slot) return;
        var u = urls(base, slot.getAttribute('data-deck'), slot.getAttribute('data-copy'));
        opener = btn;
        working(btn, true);
        say(errBox(), '');
        fetch(u.open, { credentials: 'same-origin', headers: HEADERS }).then(function (r) {
          return r.ok ? r.text() : refusal(r);
        }).then(function (html) {
          working(btn, false);
          place(html, true);
        }, function (e) {
          working(btn, false);
          say(errBox(), words(e));
          btn.focus({ preventScroll: true });
        });
      }

      /** reopen: the open card read again after the hand changed; gone means closed. */
      function reopen() {
        if (!overlay) return Promise.resolve();
        var u = urls(base, overlay.getAttribute('data-deck'), overlay.getAttribute('data-copy'));
        var hadFocus = overlay.contains(document.activeElement);
        var act = hadFocus && document.activeElement.getAttribute ? document.activeElement.getAttribute('data-dkh-act') : null;
        return fetch(u.open, { credentials: 'same-origin', headers: HEADERS }).then(function (r) {
          return r.ok ? r.text() : refusal(r);
        }).then(function (html) {
          place(html, false);
          if (hadFocus) {
            var t = (act && $('[data-dkh-act="' + act + '"]', overlay)) || $('[data-dkh-close]', overlay);
            if (t) t.focus({ preventScroll: true });
          }
        }, function (e) {
          if (e && e.status === 404) close(true);
        });
      }

      // ---- The opened card's moves ----

      function act(btn) {
        var verb = btn.getAttribute('data-dkh-act');
        var asked = btn.hasAttribute('data-dkh-ask');
        var u = urls(base, overlay.getAttribute('data-deck'), overlay.getAttribute('data-copy'));
        var big = $('[data-dkh-big] [data-dk-card]', overlay);
        say(errBox(), '');
        working(btn, true);
        send('POST', u.verb(verb), {}).then(function () {
          working(btn, false);
          var name = moveFor(verb, asked), DC = window.DeckCards;
          var played = name && DC && DC.move && big ? DC.move(big, name, { dx: 0, dy: -60 }) : Promise.resolve();
          return played.then(function () {
            if (verb === 'show' || (verb === 'play' && asked)) return reopen().then(refresh);
            close(false);
            return refresh().then(function () { restoreOpenerFocus(); });
          });
        }, function (e) {
          working(btn, false);
          say(errBox(), words(e));
          btn.focus({ preventScroll: true });
        });
      }
      function restoreOpenerFocus() {
        var t = $('[data-dkh-open], .dk-turn', el);
        if (t) t.focus({ preventScroll: true });
      }

      // ---- A player's draw ----

      function draw(btn) {
        var deck = btn.getAttribute('data-deck');
        var sec = btn.closest('[data-dkh-draw-sec]');
        var sel = sec && $('[data-dkh-hero]', sec);
        var hero = (sel && sel.value) || btn.getAttribute('data-hero');
        var box = (sec && $('[data-dkh-err]', sec)) || errBox();
        say(box, '');
        working(btn, true);
        send('POST', urls(base, deck, '').draw, { heroId: hero, count: 1 }).then(function () {
          working(btn, false);
          return refresh();
        }, function (e) {
          working(btn, false);
          say(errBox(), words(e));
          btn.focus({ preventScroll: true });
        });
      }

      // ---- Events ----

      // The motion widget binds its own click to a face-down card's button and
      // would ask the (empty) face address; this runs first and takes over.
      inst.onCapture = function (ev) {
        var btn = ev.target.closest && ev.target.closest('.dk-turn');
        if (!btn || !el.contains(btn) || !slotOf(btn)) return;
        ev.preventDefault();
        ev.stopPropagation();
        turn(btn);
      };
      el.addEventListener('click', inst.onCapture, true);

      inst.onClick = function (ev) {
        var t = ev.target;
        if (!t.closest) return;
        var b;
        if ((b = t.closest('[data-dkh-open]')) && el.contains(b)) return openCard(b);
        if ((b = t.closest('[data-dkh-act]')) && overlay && overlay.contains(b)) return act(b);
        if ((b = t.closest('[data-dkh-close]')) && overlay && overlay.contains(b)) return close(true);
        if ((b = t.closest('[data-dkh-draw]')) && el.contains(b)) return draw(b);
        if (overlay && t === overlay) return close(true);
      };
      el.addEventListener('click', inst.onClick);

      inst.onKey = function (ev) {
        if (ev.key === 'Escape' && overlay) { ev.preventDefault(); ev.stopPropagation(); close(true); }
      };
      el.addEventListener('keydown', inst.onKey);
    },

    destroy: function (el) {
      var w = el.__dkhInst;
      if (!w) return;
      el.__dkhInst = null;
      if (w.stop) w.stop();
      if (w.close) w.close(false);
      window.removeEventListener(LIVE_EVENT, w.onLive);
      if (w.onCapture) el.removeEventListener('click', w.onCapture, true);
      if (w.onClick) el.removeEventListener('click', w.onClick);
      if (w.onKey) el.removeEventListener('keydown', w.onKey);
    }
  });

  window.DeckHand = { urls: urls, words: words, moveFor: moveFor, waitingIn: waitingIn, copiesIn: copiesIn, heldIn: heldIn, turnedDown: turnedDown, TURNED_DOWN: TURNED_DOWN };
})();
