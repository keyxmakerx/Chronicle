/**
 * sidebar_drill.js -- Two-Stage Slide-Over Category Panel
 *
 * When a category is clicked, the panel slides over in two stages:
 *   Stage 1: Panel appears at left:48px, category icons briefly visible (~500ms)
 *   Stage 2: Panel slides to left:0, fully covering the icon strip
 *
 * Back button returns to the category list.
 *
 * Subtle peek: when mouse reaches the left edge of the panel (~12px),
 * the panel nudges right ~20px so icons are partially visible (about 1/3).
 * Moving the mouse away from the sidebar ends the peek.
 *
 * Prefetch: hovers on category links trigger a background fetch after 100ms.
 * On click, prefetched content is swapped instantly if available.
 */
(function () {
  'use strict';

  var catList = null;
  var catPanel = null;
  var isDrilled = false;
  var stage2Timer = null;
  var isPeeking = false;
  var PEEK_EDGE_PX = 12; // Mouse must be within this many px of the panel's left edge

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

    // Category link clicks.
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
            .catch(function () { /* ignore */ });
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

        ensureSidebarExpanded();
        loadAndDrill(link);
      });
    });

    // Auto-drill: if server pre-rendered the active state, mark as drilled.
    if (catPanel.classList.contains('sidebar-drill-active')) {
      isDrilled = true;
      // Go straight to stage 2 on page load (no pause needed).
      catPanel.classList.add('sidebar-drill-full');
    }

    // Subtle peek: detect mouse near the left edge of the panel.
    catPanel.addEventListener('mousemove', function (e) {
      if (!isDrilled || !catPanel.classList.contains('sidebar-drill-full')) return;
      var panelRect = catPanel.getBoundingClientRect();
      var distFromLeft = e.clientX - panelRect.left;
      if (distFromLeft <= PEEK_EDGE_PX && !isPeeking) {
        isPeeking = true;
        catPanel.classList.add('sidebar-drill-peeking');
      }
    });

    // End peek when mouse leaves the sidebar entirely.
    var sidebar = document.getElementById('sidebar');
    if (sidebar) {
      sidebar.addEventListener('mouseleave', function () {
        if (isPeeking) {
          isPeeking = false;
          catPanel.classList.remove('sidebar-drill-peeking');
        }
      });
    }

    // Also end peek when mouse moves away from the left edge into the panel body.
    catPanel.addEventListener('mousemove', function (e) {
      if (!isPeeking) return;
      var panelRect = catPanel.getBoundingClientRect();
      var distFromLeft = e.clientX - panelRect.left;
      // Once mouse is well into the panel (past 40px), end peek.
      if (distFromLeft > 40) {
        isPeeking = false;
        catPanel.classList.remove('sidebar-drill-peeking');
      }
    });

    // On hx-boost navigation, refresh or close the drill panel.
    window.addEventListener('chronicle:navigated', function () {
      if (!isDrilled) return;
      var currentPath = window.location.pathname;
      var navLinks = catList.querySelectorAll('.sidebar-category-link');
      var matched = false;

      for (var i = 0; i < navLinks.length; i++) {
        var catUrl = navLinks[i].getAttribute('data-cat-url');
        if (catUrl && currentPath.indexOf(catUrl) === 0) {
          // Refresh the panel content for the current category.
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
   * Load panel content and drill in with two-stage animation.
   */
  function loadAndDrill(link) {
    var drillUrl = link.getAttribute('data-drill-url');
    var target = document.getElementById('sidebar-cat-content');

    // Load content: use prefetch cache or fetch via HTMX.
    if (drillUrl && prefetchCache[drillUrl] && target) {
      target.innerHTML = prefetchCache[drillUrl];
      htmx.process(target);
      delete prefetchCache[drillUrl];
    } else if (drillUrl) {
      htmx.ajax('GET', drillUrl, {
        target: '#sidebar-cat-content',
        swap: 'innerHTML'
      });
    }

    // Stage 1: slide in, icons briefly visible.
    drillIn();
  }

  /**
   * Stage 1: slide panel in (icons visible at left:48px for 500ms).
   * Stage 2: slide to left:0 after delay.
   */
  function drillIn() {
    if (!catList || !catPanel) return;

    // Clear any pending stage 2 timer.
    if (stage2Timer) {
      clearTimeout(stage2Timer);
      stage2Timer = null;
    }

    isDrilled = true;
    isPeeking = false;
    catList.classList.add('sidebar-icon-only');
    catPanel.classList.add('sidebar-drill-active');
    catPanel.classList.remove('sidebar-drill-full');
    catPanel.classList.remove('sidebar-drill-peeking');

    // Stage 2: after 500ms, slide to fully cover icons.
    stage2Timer = setTimeout(function () {
      catPanel.classList.add('sidebar-drill-full');
      stage2Timer = null;
    }, 500);
  }

  /**
   * Drill out: close the panel, restore the category list.
   */
  function drillOut() {
    if (!catList || !catPanel) return;

    if (stage2Timer) {
      clearTimeout(stage2Timer);
      stage2Timer = null;
    }

    isDrilled = false;
    isPeeking = false;
    catList.classList.remove('sidebar-icon-only');
    catPanel.classList.remove('sidebar-drill-active');
    catPanel.classList.remove('sidebar-drill-full');
    catPanel.classList.remove('sidebar-drill-peeking');
  }

  /**
   * Ensure sidebar is expanded when drilling.
   */
  function ensureSidebarExpanded() {
    var sidebar = document.getElementById('sidebar');
    if (sidebar && sidebar.__x) {
      sidebar.__x.$data.hovered = true;
    } else if (sidebar && sidebar._x_dataStack) {
      var data = sidebar._x_dataStack[0];
      if (data) data.hovered = true;
    }
  }

  // Initialize on DOM ready.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  // Expose drillOut for the back button (used via onclick).
  window.Chronicle = window.Chronicle || {};
  window.Chronicle.drillOut = function () {
    drillOut();
  };
})();
