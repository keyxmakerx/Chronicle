/**
 * Theme Toggle
 *
 * Manages dark/light mode. The first-paint inline script in base.templ picks
 * the theme: a signed-in person's account choice (html[data-view-theme]) wins,
 * "device" follows the OS, and a signed-out visitor gets this browser's saved
 * theme (localStorage) or the OS. This module then toggles it.
 *
 * Signed in, a toggle saves to the account (PUT /account/view-prefs) so the
 * choice follows the person to every device, and localStorage is left alone;
 * signed out, it is saved in localStorage as before.
 *
 * Usage: window.Chronicle.toggleTheme() or click [data-theme-toggle]
 */
(function () {
  'use strict';

  /** True when the server rendered the signed-in person's own view choices. */
  function signedIn() {
    return document.documentElement.hasAttribute('data-view-theme');
  }

  /** Put the theme on <html> and the toggle icons without saving anywhere. */
  function paintTheme(theme) {
    var html = document.documentElement;
    // Site admin is always dark: a saved choice still applies everywhere else.
    if (html.hasAttribute('data-site-admin')) theme = 'dark';
    if (theme === 'dark') {
      html.classList.add('dark');
    } else {
      html.classList.remove('dark');
    }
    updateIcons(theme);
  }

  /** Apply the given theme ('dark' or 'light') to <html> and persist. */
  function setTheme(theme) {
    var html = document.documentElement;
    paintTheme(theme);
    if (signedIn()) {
      var prev = html.getAttribute('data-view-theme');
      html.setAttribute('data-view-theme', theme);
      if (window.Chronicle && Chronicle.apiFetch) {
        Chronicle.apiFetch('/account/view-prefs', { method: 'PUT', body: { theme: theme } })
          .then(function (r) { if (!r.ok) throw new Error('save failed'); })
          .catch(function () {
            // Not saved: put the old choice back so the page does not claim one.
            window.Chronicle.applyViewPref('theme', prev);
            if (window.Chronicle && Chronicle.notify) Chronicle.notify('Could not save your theme choice', 'error');
          });
      }
      return;
    }
    try { localStorage.setItem('chronicle-theme', theme); } catch (e) { /* noop */ }
  }

  /** Get the current active theme. */
  function getTheme() {
    return document.documentElement.classList.contains('dark') ? 'dark' : 'light';
  }

  /** Toggle between dark and light. */
  function toggleTheme() {
    setTheme(getTheme() === 'dark' ? 'light' : 'dark');
  }

  /** Update sun/moon icons on all toggle buttons. */
  function updateIcons(theme) {
    document.querySelectorAll('[data-theme-toggle]').forEach(function (btn) {
      var sun = btn.querySelector('.theme-icon-light');
      var moon = btn.querySelector('.theme-icon-dark');
      if (sun && moon) {
        if (theme === 'dark') {
          sun.classList.remove('hidden');
          moon.classList.add('hidden');
        } else {
          sun.classList.add('hidden');
          moon.classList.remove('hidden');
        }
      }
    });
  }

  // Bind click handlers once DOM is ready.
  document.addEventListener('DOMContentLoaded', function () {
    updateIcons(getTheme());
    document.querySelectorAll('[data-theme-toggle]').forEach(function (btn) {
      btn.addEventListener('click', toggleTheme);
    });
  });

  // Expose for programmatic use.
  window.Chronicle = window.Chronicle || {};
  window.Chronicle.toggleTheme = toggleTheme;
  window.Chronicle.getTheme = getTheme;
  window.Chronicle.setTheme = setTheme;

  /**
   * Apply one My view choice to this page at once (the account page calls it
   * on a tap, before the save lands), using the same attributes the layout
   * writes on load. Only per-viewer attributes are touched.
   */
  window.Chronicle.applyViewPref = function (key, value) {
    var html = document.documentElement;
    if (key === 'theme') {
      html.setAttribute('data-view-theme', value);
      if (value === 'device') {
        paintTheme(window.matchMedia && window.matchMedia('(prefers-color-scheme:dark)').matches ? 'dark' : 'light');
      } else {
        paintTheme(value);
      }
      return;
    }
    if (key === 'motion') {
      // Calm and Off both write data-view-motion="calm" (the "wants less
      // motion" flag scripts check); data-motion says how much less.
      if (value === 'calm' || value === 'off') html.setAttribute('data-view-motion', 'calm');
      else html.removeAttribute('data-view-motion');
      window.Chronicle.syncMotion(value);
      return;
    }
    var attr = { textSize: 'data-view-text', contrast: 'data-view-contrast' }[key];
    if (!attr) return;
    if (value === 'standard') html.removeAttribute(attr); else html.setAttribute(attr, value);
  };

  /**
   * Re-work html[data-motion] and nav-rm, matching MotionLevel on the
   * server: the device's reduced-motion setting or an Off pick give "off",
   * Calm or the owner's switch give "calm". pick is the person's new My view
   * choice; without it (after a Customize save) an existing "off" stands,
   * since only the device or the person can set it.
   */
  window.Chronicle.syncMotion = function (pick) {
    var html = document.documentElement;
    var device = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    var off = device || (pick ? pick === 'off' : html.getAttribute('data-motion') === 'off');
    var calm = html.hasAttribute('data-cz-reduce') || html.getAttribute('data-view-motion') === 'calm';
    if (off || calm) html.setAttribute('data-motion', off ? 'off' : 'calm');
    else html.removeAttribute('data-motion');
    // nav-rm is how the menu's script hears a reduce switch.
    html.classList.toggle('nav-rm', off || calm);
  };

  /**
   * True when moves should not travel: the device asks for reduced motion,
   * or the page is Calm or Off (html[data-motion], or the older nav-rm and
   * data-view-motion flags the first-paint script and syncMotion also set).
   * Read at the moment of the move, so a My view change applies at once.
   */
  window.Chronicle.reducedMotion = function () {
    var html = document.documentElement;
    return !!(html.getAttribute('data-motion') || html.classList.contains('nav-rm') ||
      html.getAttribute('data-view-motion') === 'calm' ||
      (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches));
  };

})();

/**
 * The "Ask in place" slip (Chronicle.askSlip).
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
