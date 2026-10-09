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

})();
