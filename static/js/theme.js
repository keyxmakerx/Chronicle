/**
 * Theme Toggle
 *
 * Manages dark/light mode. Reads preference from localStorage on page load
 * (before first paint via inline script in base.templ), then toggles via
 * this module. Falls back to system preference if no stored value.
 *
 * Usage: window.Chronicle.toggleTheme() or click [data-theme-toggle]
 */
(function () {
  'use strict';

  /** Apply the given theme ('dark' or 'light') to <html> and persist. */
  function setTheme(theme) {
    var html = document.documentElement;
    if (theme === 'dark') {
      html.classList.add('dark');
    } else {
      html.classList.remove('dark');
    }
    try { localStorage.setItem('chronicle-theme', theme); } catch (e) { /* noop */ }
    updateIcons(theme);
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
})();
