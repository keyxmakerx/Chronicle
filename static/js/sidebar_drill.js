/**
 * sidebar_drill.js -- HTMX-Powered Sidebar Drill-Down
 *
 * Handles CSS class toggling for the sidebar category drill-down animation.
 * The panel content is fetched via HTMX (hx-get on category links) and rendered
 * server-side as a Templ fragment. This script only manages visual state:
 *
 *   Drill-in: #sidebar-cat-list gets .sidebar-icon-only (hides text, shows icons).
 *             #sidebar-category gets .sidebar-cat-active (translateX(0)).
 *   Drill-out: classes removed, everything restores.
 *
 * Auto-drill on page load is handled server-side: the Templ template pre-renders
 * the panel content and applies CSS classes when the URL matches a category.
 *
 * Prefetch: hovers on category links trigger a background fetch after 100ms.
 * On click, prefetched content is swapped instantly if available.
 */
(function () {
  'use strict';

  var catList = null;
  var catPanel = null;
  var isDrilled = false;

  // Prefetch cache: Map<drillUrl, htmlString>
  var prefetchCache = {};
  var prefetchTimers = {};

  /**
   * Initialize the drill-down sidebar.
   */
  function init() {
    catList = document.getElementById('sidebar-cat-list');
    catPanel = document.getElementById('sidebar-category');

    if (!catList || !catPanel) return;

    // Category link clicks: HTMX handles the content fetch (hx-get),
    // but we need to preventDefault (to avoid navigation) and toggle CSS.
    var links = catList.querySelectorAll('.sidebar-category-link');
    links.forEach(function (link) {
      // Prefetch on hover with 100ms debounce.
      link.addEventListener('mouseenter', function () {
        var drillUrl = link.getAttribute('data-drill-url');
        if (!drillUrl || prefetchCache[drillUrl]) return;
        prefetchTimers[drillUrl] = setTimeout(function () {
          fetch(drillUrl, { headers: { 'HX-Request': 'true' } })
            .then(function (resp) { return resp.ok ? resp.text() : null; })
            .then(function (html) { if (html) prefetchCache[drillUrl] = html; })
            .catch(function () { /* ignore prefetch failures */ });
        }, 100);
      });

      link.addEventListener('mouseleave', function () {
        var drillUrl = link.getAttribute('data-drill-url');
        if (prefetchTimers[drillUrl]) {
          clearTimeout(prefetchTimers[drillUrl]);
          delete prefetchTimers[drillUrl];
        }
      });

      link.addEventListener('click', function (e) {
        e.preventDefault();
        e.stopPropagation();

        // Ensure sidebar is expanded when drilling into a category.
        ensureSidebarExpanded();

        drillIn();

        var drillUrl = link.getAttribute('data-drill-url');
        var target = document.getElementById('sidebar-cat-content');

        // Use prefetched content if available, otherwise fetch via HTMX.
        if (drillUrl && prefetchCache[drillUrl] && target) {
          target.innerHTML = prefetchCache[drillUrl];
          htmx.process(target);
          delete prefetchCache[drillUrl];
        } else {
          htmx.ajax('GET', drillUrl, {
            target: '#sidebar-cat-content',
            swap: 'innerHTML'
          });
        }
      });
    });

    // Back button (event delegation for the static back button).
    var backBtn = document.getElementById('sidebar-back');
    if (backBtn) {
      backBtn.addEventListener('click', function () {
        drillOut();
      });
    }

    // The drill panel now covers the full sidebar area. The Back button
    // in the panel header is the primary way to drill out.

    // Auto-drill: if server pre-rendered the active state, mark as drilled.
    if (catPanel.classList.contains('sidebar-cat-active')) {
      isDrilled = true;
    }

    // On hx-boost navigation, refresh the drill panel content if still
    // relevant (same category), or close it if user navigated away.
    // This ensures entity counts and recent items stay current after
    // entity creation, deletion, or page navigation.
    window.addEventListener('chronicle:navigated', function () {
      if (!isDrilled) return;
      var currentPath = window.location.pathname;
      var navLinks = catList.querySelectorAll('.sidebar-category-link');
      var matched = false;
      for (var i = 0; i < navLinks.length; i++) {
        var catUrl = navLinks[i].getAttribute('data-cat-url');
        if (catUrl && currentPath.indexOf(catUrl) === 0) {
          var drillUrl = navLinks[i].getAttribute('data-drill-url');
          if (drillUrl) {
            htmx.ajax('GET', drillUrl, {
              target: '#sidebar-cat-content',
              swap: 'innerHTML'
            });
          }
          matched = true;
          break;
        }
      }
      if (!matched) drillOut();
    });
  }

  /**
   * Ensure sidebar is expanded when interacting with drill content.
   * Sets Alpine.js hovered state to true so sidebar doesn't collapse mid-interaction.
   */
  function ensureSidebarExpanded() {
    var sidebar = document.getElementById('sidebar');
    if (sidebar && sidebar.__x) {
      sidebar.__x.$data.hovered = true;
    } else if (sidebar && sidebar._x_dataStack) {
      // Alpine v3 approach
      var data = sidebar._x_dataStack[0];
      if (data) data.hovered = true;
    }
  }

  /**
   * Drill into a category: collapse list, slide panel in.
   */
  function drillIn() {
    if (!catList || !catPanel) return;
    isDrilled = true;
    catList.classList.add('sidebar-icon-only');
    catPanel.classList.add('sidebar-cat-active');
  }

  /**
   * Drill out: restore list, slide panel out.
   */
  function drillOut() {
    if (!catList || !catPanel) return;
    isDrilled = false;
    catList.classList.remove('sidebar-icon-only');
    catPanel.classList.remove('sidebar-cat-active');
  }

  // Initialize when DOM is ready.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
