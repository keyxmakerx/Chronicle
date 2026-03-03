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
 */
(function () {
  'use strict';

  var catList = null;
  var catPanel = null;
  var isDrilled = false;

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
      link.addEventListener('click', function (e) {
        e.preventDefault();
        e.stopPropagation();
        drillIn();
        // Trigger the HTMX fetch manually since we prevented default.
        htmx.ajax('GET', link.getAttribute('data-drill-url'), {
          target: '#sidebar-cat-content',
          swap: 'innerHTML'
        });
      });
    });

    // Back button (event delegation for the static back button).
    var backBtn = document.getElementById('sidebar-back');
    if (backBtn) {
      backBtn.addEventListener('click', function () {
        drillOut();
      });
    }

    // Click the collapsed icon strip to go back.
    catList.addEventListener('click', function (e) {
      if (isDrilled) {
        e.preventDefault();
        e.stopPropagation();
        drillOut();
      }
    });

    // Auto-drill: if server pre-rendered the active state, mark as drilled.
    if (catPanel.classList.contains('sidebar-cat-active')) {
      isDrilled = true;
    }

    // Close drill-down on hx-boost navigation (user navigated away).
    window.addEventListener('chronicle:navigated', function () {
      if (isDrilled) drillOut();
    });
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
