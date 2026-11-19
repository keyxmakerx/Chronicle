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

  // --- Pop-up layers and the shared modal helper -------------------------
  (function () {
    /**
     * layerHost: where a floating layer (toast, sheet, password check) must be
     * mounted to be usable. Everything outside an open modal <dialog> is inert,
     * so a layer raised while one is open goes inside the topmost.
     */
    C.layerHost = function () {
      var open = document.querySelectorAll('dialog[open]');
      for (var i = open.length - 1; i >= 0; i--) {
        if (open[i].matches(':modal')) return open[i];
      }
      return document.body;
    };

    var FOCUSABLE = 'a[href],button:not([disabled]),input:not([disabled]):not([type="hidden"]),select:not([disabled]),textarea:not([disabled]),summary,[tabindex]:not([tabindex="-1"])';
    var MKEY = '_chronicleModal';
    var stack = [];

    function shown(el) {
      if (!el.isConnected || el.hidden || el.classList.contains('hidden')) return false;
      return el.getClientRects().length > 0;
    }

    function focusables(el) {
      var out = [];
      var all = el.querySelectorAll(FOCUSABLE);
      for (var i = 0; i < all.length; i++) {
        var n = all[i];
        if (n.closest('[hidden],[inert]') || n.getClientRects().length === 0) continue;
        if (getComputedStyle(n).visibility === 'hidden') continue;
        out.push(n);
      }
      return out;
    }

    // Drops pop-ups that were removed or hidden by something that never told us.
    function purge() {
      while (stack.length && !shown(stack[stack.length - 1].el)) stack[stack.length - 1].release(false);
    }

    function onKey(e) {
      purge();
      var m = stack[stack.length - 1];
      if (!m) return;
      if (e.key === 'Escape') {
        if (m.opts.escape === false || e.defaultPrevented) return;
        e.preventDefault();
        m.close();
      } else if (e.key === 'Tab') {
        var f = focusables(m.el);
        if (!f.length) { e.preventDefault(); return; }
        var a = document.activeElement;
        if (!m.el.contains(a)) {
          // A layer the pop-up opened, living elsewhere in the page, keeps its own Tab.
          if (m.opts.allow && a && a.closest && a.closest(m.opts.allow)) return;
          e.preventDefault(); f[e.shiftKey ? f.length - 1 : 0].focus();
        }
        else if (e.shiftKey && a === f[0]) { e.preventDefault(); f[f.length - 1].focus(); }
        else if (!e.shiftKey && a === f[f.length - 1]) { e.preventDefault(); f[0].focus(); }
      }
    }

    /**
     * Chronicle.modal(el, {opener, onClose, escape, focus}): treat a shown pop-up
     * as a modal. Gives it role="dialog" and aria-modal when missing, moves focus
     * in ([autofocus], else the first control), keeps Tab and Shift+Tab inside,
     * closes on Escape (not when something already handled the key) and puts
     * focus back on the opener when it closes, whoever closes it. onClose runs
     * only when the helper closes it (Escape or close()); without one the helper
     * hides the pop-up with the "hidden" class. escape:false leaves Escape to the
     * pop-up's own guarded close; focus:false leaves the opening focus to it;
     * opener:null skips returning focus; watch names an ancestor whose hiding
     * also ends the modal; aria:false leaves the roles to markup that has them;
     * allow is a selector for a layer opened from the pop-up that lives outside
     * it and keeps its own Tab.
     * Returns {close}. Calling it again on a managed pop-up returns the same one.
     */
    C.modal = function (el, opts) {
      if (!el) return { close: function () {} };
      purge();
      if (el[MKEY]) return el[MKEY].handle;
      opts = opts || {};
      var opener = ('opener' in opts) ? opts.opener : document.activeElement;
      if (opts.aria !== false) {
        if (!el.hasAttribute('role')) el.setAttribute('role', 'dialog');
        if (!el.hasAttribute('aria-modal')) el.setAttribute('aria-modal', 'true');
      }
      var obs = null;
      var m = { el: el, opts: opts };
      m.release = function (restore) {
        var at = stack.indexOf(m);
        if (at < 0) return;
        stack.splice(at, 1);
        if (!stack.length) document.removeEventListener('keydown', onKey);
        if (obs) obs.disconnect();
        delete el[MKEY];
        if (restore !== false && opener && opener.isConnected && opener.focus) opener.focus();
      };
      m.close = function () {
        if (stack.indexOf(m) < 0) return;
        if (typeof opts.onClose === 'function') opts.onClose();
        else el.classList.add('hidden');
        m.release(true);
      };
      m.handle = { close: m.close };
      el[MKEY] = m;
      if (!stack.length) document.addEventListener('keydown', onKey);
      stack.push(m);
      // The owner hiding it (a Cancel button, a save) ends the modal too.
      if (window.MutationObserver) {
        obs = new MutationObserver(function () { if (!shown(el)) m.release(true); });
        var seen = { attributes: true, attributeFilter: ['class', 'hidden', 'style'] };
        obs.observe(el, seen);
        // A pop-up shown by an ancestor (a reader around its sheet) names it.
        if (opts.watch && opts.watch !== el) obs.observe(opts.watch, seen);
      }
      if (opts.focus !== false) {
        var f = el.querySelector('[autofocus]') || focusables(el)[0];
        if (f) f.focus();
        else { if (!el.hasAttribute('tabindex')) el.setAttribute('tabindex', '-1'); el.focus(); }
      }
      return m.handle;
    };

    // A button with data-opens="<id>" shows that pop-up and opens it as a modal.
    // Inline on* handlers can't be written from Go (templ refuses them), so the
    // app openings' create buttons name their pop-up here instead.
    // data-opener-global="<name>" records the button on window for pop-ups
    // that return focus themselves (the calendar wizard, loaded by htmx).
    document.addEventListener('click', function (e) {
      var b = e.target && e.target.closest && e.target.closest('[data-opens],[data-opener-global]');
      if (!b) return;
      var g = b.getAttribute('data-opener-global');
      if (g) window[g] = b;
      var id = b.getAttribute('data-opens');
      var target = id && document.getElementById(id);
      if (!target) return;
      target.classList.remove('hidden');
      target.hidden = false;
      C.modal(target, { opener: b });
    });
  })();

  /**
   * Working state for a button (ui-standard.md, "Waiting and feedback"): a
   * spinner after 0.3 s, an optional "…ing" label from data-working-label,
   * and no second press while it runs. An htmx request started by a button,
   * or by the form it submits, sets and clears it on its own; other code
   * calls Chronicle.working(button, true) and (button, false).
   */
  var working = function (btn, on) {
    if (!btn || !btn.setAttribute) return;
    if (on) {
      if (btn.hasAttribute('data-working')) return;
      btn.setAttribute('data-working', '');
      btn.setAttribute('aria-busy', 'true');
      var label = btn.getAttribute('data-working-label');
      if (label) { btn._label = btn.textContent; btn.textContent = label; }
    } else {
      if (!btn.hasAttribute('data-working')) return;
      btn.removeAttribute('data-working');
      btn.removeAttribute('aria-busy');
      if (btn._label != null) { btn.textContent = btn._label; btn._label = null; }
    }
  };
  C.working = working;

  /**
   * Put a fixed menu beside the caret and inside the window: under the caret
   * when a full-height menu fits there, above it when it does not, and 8px
   * clear of the window's sides. On a phone the window is what the keyboard
   * leaves showing (visualViewport).
   *
   * The side is chosen for the menu's full height (its CSS max-height, read
   * once), so it doesn't jump as a filter shortens the list.
   *
   * @param {HTMLElement} menuEl - the menu; position:fixed.
   * @param {Object} caretRect - {left, top, bottom} of the caret, in viewport px.
   */
  C.placeAtCaret = function (menuEl, caretRect) {
    if (!menuEl || !caretRect) return;
    var GAP = 4, EDGE = 8, MIN = 96;
    if (menuEl._placeCap == null) {
      var cs = window.getComputedStyle ? window.getComputedStyle(menuEl) : null;
      menuEl._placeCap = (cs && parseFloat(cs.maxHeight)) || 300;
    }
    var cap = menuEl._placeCap;
    var vv = window.visualViewport;
    var winTop = vv ? vv.offsetTop : 0;
    var winLeft = vv ? vv.offsetLeft : 0;
    var winH = vv ? vv.height : window.innerHeight;
    var winW = vv ? vv.width : window.innerWidth;
    var roomBelow = winTop + winH - EDGE - (caretRect.bottom + GAP);
    var roomAbove = caretRect.top - GAP - (winTop + EDGE);
    var below = roomBelow >= cap || roomBelow >= roomAbove;
    menuEl.style.maxHeight = Math.max(MIN, Math.min(cap, below ? roomBelow : roomAbove)) + 'px';
    menuEl.style.top = (below ? caretRect.bottom + GAP : caretRect.top - GAP - menuEl.offsetHeight) + 'px';
    var maxLeft = winLeft + winW - menuEl.offsetWidth - EDGE;
    menuEl.style.left = Math.max(winLeft + EDGE, Math.min(caretRect.left, maxLeft)) + 'px';
  };

  function requestButton(e) {
    var d = e.detail || {}, el = d.elt;
    if (!el || !el.matches) return null;
    if (el.matches('.btn')) return el;
    var sub = d.requestConfig && d.requestConfig.triggeringEvent && d.requestConfig.triggeringEvent.submitter;
    if (sub && sub.matches && sub.matches('.btn')) return sub;
    return el.tagName === 'FORM' ? el.querySelector('[type="submit"].btn') : null;
  }
  document.addEventListener('htmx:beforeRequest', function (e) { working(requestButton(e), true); });
  document.addEventListener('htmx:afterRequest', function (e) { working(requestButton(e), false); });
  // A button that is working can't be pressed again, even by keyboard.
  document.addEventListener('click', function (e) {
    var b = e.target && e.target.closest && e.target.closest('.btn[data-working]');
    if (b) { e.preventDefault(); e.stopPropagation(); }
  }, true);
})();
