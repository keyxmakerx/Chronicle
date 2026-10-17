/**
 * ask_slip.js -- the "Ask in place" slip (Chronicle.askSlip).
 *
 * A box holding unsaved work never closes behind the person's back and never
 * shakes. When they try to leave it (Escape, or the thing that opened it),
 * a slip slides out from under the box's foot and asks: what would be thrown
 * away, with "Keep editing" and a red button that names the loss. The box
 * grows to hold it; nothing else on the page moves.
 *
 *   Chronicle.askSlip(box, {
 *     ask:       'Discard the new name?',            // the question
 *     detail:    '"Brother Ansel" hasn’t been saved.', // what goes, plain text
 *     discard:   'Discard',                           // the red button
 *     onDiscard: function () { ... },                 // throw it away and close
 *     onKeep:    function () { ... }                  // optional
 *   });
 *   Chronicle.askSlip.isOpen(box)   // true while the slip is out
 *   Chronicle.askSlip.close(box)    // put it back (the work is kept)
 *
 * Asking again while the slip is out means "never mind" and puts it back, so
 * a second Escape returns to the work. The slip is the box's last child; a
 * box near the foot of the window scrolls just enough to show it. Under Calm
 * it fades in place; under Off it simply appears.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var KEY = '_askSlip';

  function reduced() {
    if (C.reducedMotion) return C.reducedMotion();
    return !!(window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  function build(box) {
    var slip = document.createElement('div');
    slip.className = 'ask-slip';
    slip.setAttribute('aria-hidden', 'true');
    slip.innerHTML =
      '<div class="ask-slip-clip"><div class="ask-slip-in">' +
      '<p class="ask-slip-q" role="alert"><i class="fa-solid fa-circle-exclamation" aria-hidden="true"></i>' +
      '<span><b data-ask></b><span data-detail></span></span></p>' +
      '<button type="button" class="btn-secondary btn-sm" data-keep>Keep editing</button>' +
      '<button type="button" class="btn-danger btn-sm" data-drop>Discard</button>' +
      '</div></div>';
    slip.addEventListener('click', function (e) {
      var st = box[KEY];
      if (!st) return;
      if (e.target.closest('[data-keep]')) { e.stopPropagation(); keep(box); }
      else if (e.target.closest('[data-drop]')) {
        e.stopPropagation();
        var fn = st.onDiscard;
        hide(box, false);
        if (fn) fn();
      }
    });
    box.appendChild(slip);
    return slip;
  }

  // hide puts the slip back. refocus returns focus to where the person was.
  function hide(box, refocus) {
    var st = box && box[KEY];
    if (!st || !st.open) return;
    st.open = false;
    st.slip.classList.remove('is-on');
    st.slip.setAttribute('aria-hidden', 'true');
    var back = st.back;
    st.back = null;
    if (refocus && back && back.isConnected) back.focus({ preventScroll: true });
  }

  function keep(box) {
    var st = box[KEY];
    var fn = st && st.onKeep;
    hide(box, true);
    if (fn) fn();
  }

  // reveal scrolls only as far as needed to show the slip, once it is out.
  function reveal(slip) {
    var r = slip.getBoundingClientRect();
    var h = window.innerHeight || document.documentElement.clientHeight;
    if (r.bottom <= h && r.top >= 0) return;
    slip.scrollIntoView({ block: 'nearest', behavior: reduced() ? 'auto' : 'smooth' });
  }

  function ask(box, o) {
    if (!box) return;
    o = o || {};
    var st = box[KEY];
    // A redraw of the box can take the slip with it; start again then.
    var gone = !st || st.slip.parentNode !== box;
    if (st && st.open && !gone) { keep(box); return; }
    if (gone) {
      st = box[KEY] = { slip: null, open: false };
      st.slip = build(box);
    } else if (st.slip !== box.lastElementChild) {
      box.appendChild(st.slip);
    }
    var slip = st.slip;
    slip.querySelector('[data-ask]').textContent = o.ask || 'Discard your changes?';
    slip.querySelector('[data-detail]').textContent = o.detail || '';
    slip.querySelector('[data-drop]').textContent = o.discard || 'Discard';
    st.onDiscard = o.onDiscard || null;
    st.onKeep = o.onKeep || null;
    var active = document.activeElement;
    st.back = active && active !== document.body && !slip.contains(active) ? active : null;
    st.open = true;
    slip.setAttribute('aria-hidden', 'false');
    void slip.offsetHeight;
    slip.classList.add('is-on');
    var wait = reduced() ? 0 : 380;
    setTimeout(function () {
      if (!st.open) return;
      var k = slip.querySelector('[data-keep]');
      if (k) k.focus({ preventScroll: true });
      reveal(slip);
    }, wait);
  }

  ask.isOpen = function (box) { var st = box && box[KEY]; return !!(st && st.open && st.slip.parentNode === box); };
  ask.close = function (box) { hide(box, false); };

  C.askSlip = ask;
})();
