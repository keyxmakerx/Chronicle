/**
 * campaign_leave.js -- the three dots on My campaigns' cards and the box that
 * asks before a member leaves (Chronicle.CampaignLeave).
 *
 * The server draws the cards and one hidden box per campaign
 * (internal/plugins/campaigns/campaign_card.templ, leave_box.templ); every
 * control calls in here through an inline handler built Go-side, so nothing
 * depends on a script inside a swapped fragment. The outside-click and Escape
 * listeners live only while a menu is open.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var L = (C.CampaignLeave = C.CampaignLeave || {});

  var open = null; // {menu, btn}

  function calm() {
    return document.documentElement.getAttribute('data-motion') === 'off' ||
      (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  function outside(e) {
    if (!open) return;
    if (!open.menu.isConnected) { closeMenu(); return; }
    if (open.menu.contains(e.target) || open.btn.contains(e.target)) return;
    closeMenu();
  }
  function onKey(e) {
    if (e.key !== 'Escape' || !open) return;
    var b = open.btn;
    closeMenu();
    if (b && b.isConnected) b.focus();
  }
  function listen(on) {
    var m = on ? 'addEventListener' : 'removeEventListener';
    document[m]('mousedown', outside, true);
    document[m]('keydown', onKey, true);
  }
  function closeMenu() {
    if (!open) return;
    var o = open;
    open = null;
    listen(false);
    o.menu.hidden = true;
    o.btn.setAttribute('aria-expanded', 'false');
  }

  // The dots button opens or closes its card's menu.
  L.toggle = function (btn) {
    var menu = btn.parentNode.querySelector('[data-leave-menu]');
    if (!menu) return;
    if (open && open.menu === menu) { closeMenu(); return; }
    closeMenu();
    menu.hidden = false;
    btn.setAttribute('aria-expanded', 'true');
    open = { menu: menu, btn: btn };
    listen(true);
    var first = menu.querySelector('[role="menuitem"]');
    if (first && document.activeElement === btn && !btn.matches(':hover')) first.focus();
  };

  // "Leave campaign..." closes the menu and opens the question as a modal;
  // closing it returns focus to the dots.
  L.ask = function (item) {
    var box = document.getElementById(item.getAttribute('data-box'));
    var dots = open ? open.btn : null;
    closeMenu();
    if (!box) return;
    var err = box.querySelector('[data-leave-error]');
    if (err) err.classList.add('hidden');
    box.classList.remove('hidden');
    if (C.modal) C.modal(box, { opener: dots });
  };

  function boxOf(el) { return el.closest('[data-leave-box]'); }

  // Stay shuts the box and changes nothing.
  L.stay = function (btn) {
    var box = boxOf(btn);
    if (!box) return;
    if (C.modal) C.modal(box).close(); else box.classList.add('hidden');
  };

  function fail(box, btn, msg) {
    if (C.working) C.working(btn, false);
    var err = box.querySelector('[data-leave-error]');
    if (err) { err.textContent = msg; err.classList.remove('hidden'); }
  }

  // Remove the card as the person's own list changes: it shrinks away unless
  // motion is off, then the empty message shows when it was the last one.
  // Focus was on the card's dots, so it moves to the next card (or the one
  // before, or the heading) rather than dropping to the page.
  function dropCard(id) {
    var card = document.querySelector('[data-campaign-card="' + id + '"]');
    if (!card) return;
    var grid = card.parentNode;
    var list = card.closest('#campaign-list');
    function sibling(el, dir) {
      for (el = el[dir]; el; el = el[dir]) if (el.hasAttribute && el.hasAttribute('data-campaign-card')) return el;
      return null;
    }
    var next = sibling(card, 'nextElementSibling') || sibling(card, 'previousElementSibling');
    function gone() {
      var hadFocus = card.contains(document.activeElement) || document.activeElement === document.body;
      card.remove();
      var to = next && next.querySelector('a[href]');
      if (grid && !grid.querySelector('[data-campaign-card]')) {
        // Other pages may still hold campaigns; then this page is the empty one.
        var more = list && list.hasAttribute('data-more-pages');
        var p = document.createElement('p');
        p.className = 'text-fg-secondary text-lg py-16 text-center';
        p.textContent = more ? 'No campaigns left on this page. ' : "You don't have any campaigns yet.";
        if (more) {
          var a = document.createElement('a');
          a.href = '/campaigns';
          a.className = 'text-accent-link underline';
          a.textContent = 'See your campaigns';
          p.appendChild(a);
          to = a;
        }
        grid.replaceWith(p);
      }
      if (!to && list) {
        to = list.querySelector('h1');
        if (to) to.setAttribute('tabindex', '-1');
      }
      if (hadFocus && to) to.focus({ preventScroll: true });
    }
    if (calm()) { gone(); return; }
    card.style.transition = 'opacity var(--dur-everyday, 120ms) var(--ease-in, ease-in), transform var(--dur-everyday, 120ms) var(--ease-in, ease-in)';
    card.style.opacity = '0';
    card.style.transform = 'scale(0.96)';
    setTimeout(gone, 160);
  }

  // The red button posts the leave for the signed-in person. Only success
  // removes the card; a refusal shows its words in the box.
  L.leave = function (btn) {
    var box = boxOf(btn);
    if (!box || btn.hasAttribute('data-working')) return;
    var name = box.getAttribute('data-campaign-name') || 'the campaign';
    if (C.working) C.working(btn, true);
    C.apiFetch(box.getAttribute('data-leave-url'), { method: 'POST' })
      .then(function (r) {
        if (r.ok) return null;
        return r.json().catch(function () { return {}; }).then(function (j) {
          // message is the server's sentence; error is its machine code.
          throw new Error((j && (j.message || j.error)) || 'Could not leave. Try again.');
        });
      })
      .then(function () {
        if (C.working) C.working(btn, false);
        if (C.modal) C.modal(box, { opener: null }).close(); else box.classList.add('hidden');
        dropCard(box.getAttribute('data-campaign-id'));
        if (C.notify) C.notify('You left ' + name + '.', 'success');
        // A page about the campaign itself has nothing left to show.
        var then = box.getAttribute('data-leave-then');
        if (then && then.charAt(0) === '/' && then.charAt(1) !== '/') window.location.assign(then);
      })
      .catch(function (e) { fail(box, btn, e && e.message ? e.message : 'Could not leave. Try again.'); });
  };

  Chronicle.register('campaign-leave', { init: function () {}, destroy: closeMenu });
})();
