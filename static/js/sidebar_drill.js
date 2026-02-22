/**
 * sidebar_drill.js -- Drill-Down Sidebar Navigation
 *
 * Implements the iOS-style "push" navigation for the sidebar. Clicking a
 * category link in the main panel slides both panels left, revealing the
 * category sub-panel. The main panel peeks from the left edge (~10px) with
 * a hover animation to indicate it's clickable. Clicking the peek or the
 * back button slides everything back.
 *
 * Architecture:
 *   #sidebar-slider is a flex row holding two w-64 panels.
 *   Drill-in: translateX(calc(-16rem + 10px)) — main panel peeks 10px.
 *   Drill-out: translateX(0) — main panel fully visible.
 *
 * The category sub-panel is populated dynamically from data-cat-* attributes
 * on the clicked category link, so no extra API call is needed.
 */
(function () {
  'use strict';

  var PEEK_PX = 10;

  /** @type {HTMLElement|null} */
  var slider = null;
  /** @type {HTMLElement|null} */
  var backBtn = null;
  /** @type {HTMLElement|null} */
  var catIcon = null;
  /** @type {HTMLElement|null} */
  var catName = null;
  /** @type {HTMLElement|null} */
  var catCount = null;
  /** @type {HTMLElement|null} */
  var catNav = null;
  /** @type {HTMLElement|null} */
  var mainPanel = null;

  /** Whether the sidebar is currently drilled into a category. */
  var isDrilled = false;

  /** Current active category slug (to restore on page load). */
  var activeSlug = null;

  /**
   * Initialize the drill-down sidebar on DOMContentLoaded.
   */
  function init() {
    slider = document.getElementById('sidebar-slider');
    backBtn = document.getElementById('sidebar-back');
    catIcon = document.getElementById('sidebar-cat-icon');
    catName = document.getElementById('sidebar-cat-name');
    catCount = document.getElementById('sidebar-cat-count');
    catNav = document.getElementById('sidebar-cat-nav');
    mainPanel = document.getElementById('sidebar-main');

    if (!slider || !backBtn) {
      console.warn('[sidebar_drill] init aborted: slider=' + !!slider + ', backBtn=' + !!backBtn);
      return;
    }

    // Bind category links.
    var links = document.querySelectorAll('.sidebar-category-link');
    links.forEach(function (link) {
      link.addEventListener('click', function (e) {
        e.preventDefault();
        e.stopPropagation(); // Prevent bubbling to mainPanel (which would drillOut).
        drillIn(link);
      });
    });

    // Bind back button.
    backBtn.addEventListener('click', function () {
      drillOut();
    });

    // Make the main panel peek area clickable to go back.
    if (mainPanel) {
      mainPanel.addEventListener('click', function (e) {
        if (isDrilled) {
          e.preventDefault();
          e.stopPropagation();
          drillOut();
        }
      });
    }

    // Check if the current URL matches a category and auto-drill.
    var currentPath = window.location.pathname;
    links.forEach(function (link) {
      var catUrl = link.getAttribute('data-cat-url');
      if (catUrl && currentPath.indexOf(catUrl) === 0) {
        drillIn(link, true); // true = no animation on initial load.
      }
    });
  }

  /**
   * Drill into a category sub-panel.
   * @param {HTMLElement} link - The category link that was clicked.
   * @param {boolean} [instant] - If true, skip the animation (initial load).
   */
  function drillIn(link, instant) {
    if (!slider) {
      console.warn('[sidebar_drill] drillIn: slider is null');
      return;
    }

    var slug = link.getAttribute('data-cat-slug') || '';
    var label = link.getAttribute('data-cat-label') || '';
    var color = link.getAttribute('data-cat-color') || '#6b7280';
    var icon = link.getAttribute('data-cat-icon') || '';
    var count = link.getAttribute('data-cat-count') || '0';
    var catUrl = link.getAttribute('data-cat-url') || '#';
    var newUrl = link.getAttribute('data-cat-new-url') || '#';
    var campaignId = link.getAttribute('data-campaign-id') || '';

    activeSlug = slug;
    isDrilled = true;

    // Populate category header.
    if (catIcon) {
      if (icon) {
        catIcon.innerHTML = '<i class="fa-solid ' + escapeAttr(icon) + ' text-sm" style="color: ' + escapeAttr(color) + '"></i>';
      } else {
        catIcon.innerHTML = '<span class="w-3 h-3 rounded-full" style="background-color: ' + escapeAttr(color) + '"></span>';
      }
    }
    if (catName) catName.textContent = label;
    if (catCount) catCount.textContent = count + (parseInt(count, 10) === 1 ? ' page' : ' pages');

    // Build category sub-nav content.
    if (catNav) {
      catNav.innerHTML = buildCategoryNav(catUrl, newUrl, label, color, icon, campaignId, slug);
    }

    // Slide the panels using pixel value (avoid calc inside translate3d).
    // 16rem = 256px, minus PEEK_PX = 246px offset.
    var offset = -(256 - PEEK_PX);
    if (instant) {
      slider.style.transition = 'none';
    }
    slider.style.transform = 'translate3d(' + offset + 'px, 0px, 0px)';

    if (instant) {
      // Force reflow then restore transition.
      slider.offsetHeight; // eslint-disable-line no-unused-expressions
      slider.style.transition = '';
    }

    // Add peek interaction class to main panel.
    if (mainPanel) {
      mainPanel.classList.add('sidebar-peek');
    }
  }

  /**
   * Drill out back to the main navigation.
   */
  function drillOut() {
    if (!slider) return;

    isDrilled = false;
    activeSlug = null;
    slider.style.transform = 'translate3d(0, 0, 0)';

    if (mainPanel) {
      mainPanel.classList.remove('sidebar-peek');
    }
  }

  /**
   * Build HTML for the category sub-nav panel.
   */
  function buildCategoryNav(catUrl, newUrl, label, color, icon, campaignId, slug) {
    var linkClass = 'flex items-center px-4 py-2 text-sm transition-colors text-sidebar-text hover:bg-sidebar-hover hover:text-sidebar-active';
    var searchUrl = '/campaigns/' + campaignId + '/entities/search?type_slug=' + slug;

    var html = '';

    // View all link (to category dashboard).
    html += '<a href="' + escapeAttr(catUrl) + '" class="' + linkClass + '">' +
      '<span class="w-4 h-4 mr-3 shrink-0 flex items-center justify-center">' +
      '<i class="fa-solid fa-th-large text-xs" style="color: ' + escapeAttr(color) + '"></i>' +
      '</span>' +
      '<span class="flex-1">View All ' + escapeHtml(label) + '</span>' +
      '</a>';

    // New page link.
    html += '<a href="' + escapeAttr(newUrl) + '" class="' + linkClass + '">' +
      '<span class="w-4 h-4 mr-3 shrink-0 flex items-center justify-center">' +
      '<i class="fa-solid fa-plus text-xs text-gray-400"></i>' +
      '</span>' +
      '<span class="flex-1">New Page</span>' +
      '</a>';

    // Divider.
    html += '<div class="my-2 mx-4 border-t border-gray-700/50"></div>';

    // Quick search (HTMX-powered).
    html += '<div class="px-4 mb-2">' +
      '<div class="relative">' +
      '<input type="search" placeholder="Search ' + escapeAttr(label) + '..." ' +
      'class="w-full px-3 py-1.5 pl-8 text-xs rounded-md bg-gray-800 border border-gray-700 text-gray-300 placeholder-gray-500 focus:outline-none focus:border-gray-500 transition-colors" ' +
      'hx-get="/campaigns/' + escapeAttr(campaignId) + '/entities/search?type_slug=' + escapeAttr(slug) + '" ' +
      'hx-trigger="keyup changed delay:300ms" ' +
      'hx-target="#sidebar-cat-results" ' +
      'hx-swap="innerHTML" ' +
      'name="q" autocomplete="off"/>' +
      '<i class="fa-solid fa-magnifying-glass absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-500 text-[10px] pointer-events-none"></i>' +
      '</div>' +
      '</div>';

    // Search results area.
    html += '<div id="sidebar-cat-results" class="px-2"></div>';

    // Section header for recent pages.
    html += '<div class="px-4 mt-2 mb-1 text-[10px] font-semibold uppercase tracking-wider text-gray-600">Recent</div>';

    // Placeholder for recent pages (loaded via HTMX or populated later).
    html += '<div id="sidebar-cat-recent" class="text-xs text-gray-500 px-4 py-2 italic">' +
      'Visit "View All" to browse pages' +
      '</div>';

    return html;
  }

  /**
   * Escape HTML to prevent XSS.
   */
  function escapeHtml(text) {
    var div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  /**
   * Escape a string for use in an HTML attribute.
   */
  function escapeAttr(text) {
    return String(text).replace(/[&"'<>]/g, function (c) {
      return { '&': '&amp;', '"': '&quot;', "'": '&#39;', '<': '&lt;', '>': '&gt;' }[c];
    });
  }

  // Initialize when DOM is ready.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
