/**
 * sidebar_drill.js -- Sidebar Category Accordion
 *
 * Manages inline expandable category sections in the sidebar. Each category
 * has a toggle button and a collapsible body containing search, entity list,
 * favorites, and recent sections.
 *
 * Features:
 *   - Click toggle to expand/collapse a category inline
 *   - Only one category open at a time (accordion behavior)
 *   - Entity list lazy-loaded on first expand via HTMX
 *   - Prefetch on hover for instant opening
 *   - Auto-opens the active category on page load (server-rendered)
 *   - Navigating away closes the accordion if URL no longer matches
 */
(function () {
  'use strict';

  // Prefetch cache: slug -> HTML string
  var prefetchCache = {};
  var prefetchTimers = {};

  /**
   * Initialize accordion behavior for all category sections.
   */
  function init() {
    var zone = document.getElementById('sidebar-categories-zone');
    if (!zone) return;

    var toggles = zone.querySelectorAll('.sidebar-accordion-toggle');

    toggles.forEach(function (toggle) {
      var slug = toggle.getAttribute('data-cat-slug');
      var accordion = toggle.closest('.sidebar-accordion');
      if (!accordion) return;

      // Prefetch entity list on hover (100ms debounce).
      toggle.addEventListener('mouseenter', function () {
        if (prefetchCache[slug]) return;
        var body = accordion.querySelector('.sidebar-accordion-body');
        var placeholder = body && body.querySelector('.sidebar-accordion-placeholder');
        if (!placeholder) return; // Already loaded

        var loadUrl = placeholder.getAttribute('data-load-url');
        if (!loadUrl) return;

        prefetchTimers[slug] = setTimeout(function () {
          fetch(loadUrl, { headers: { 'HX-Request': 'true' } })
            .then(function (resp) { return resp.ok ? resp.text() : null; })
            .then(function (html) { if (html) prefetchCache[slug] = html; })
            .catch(function () { /* ignore */ });
        }, 100);
      });

      toggle.addEventListener('mouseleave', function () {
        if (prefetchTimers[slug]) {
          clearTimeout(prefetchTimers[slug]);
          delete prefetchTimers[slug];
        }
      });

      // Toggle on click.
      toggle.addEventListener('click', function (e) {
        e.preventDefault();
        e.stopPropagation();
        ensureSidebarExpanded();

        var isOpen = accordion.querySelector('.sidebar-accordion-body.sidebar-accordion-open');

        if (isOpen) {
          // Close this accordion.
          closeAccordion(accordion);
        } else {
          // Close any other open accordion first.
          closeAllAccordions(zone);
          // Open this one.
          openAccordion(accordion, slug);
        }
      });
    });

    // On navigation, refresh or close the active accordion.
    window.addEventListener('chronicle:navigated', function () {
      var currentPath = window.location.pathname;
      var accordions = zone.querySelectorAll('.sidebar-accordion');
      var anyMatched = false;

      accordions.forEach(function (acc) {
        var catUrl = acc.getAttribute('data-cat-url');
        var body = acc.querySelector('.sidebar-accordion-body');

        if (catUrl && currentPath.indexOf(catUrl) === 0) {
          // This category matches the URL — ensure it's open and refresh.
          anyMatched = true;
          if (body && !body.classList.contains('sidebar-accordion-open')) {
            closeAllAccordions(zone);
            var slug2 = acc.getAttribute('data-cat-slug');
            openAccordion(acc, slug2);
          } else {
            // Already open — refresh entity list.
            refreshAccordionResults(acc);
          }
        }
      });

      // If no category matches, close all.
      if (!anyMatched) {
        closeAllAccordions(zone);
      }
    });
  }

  /**
   * Open a single accordion section. Lazy-loads entities if needed.
   */
  function openAccordion(accordion, slug) {
    var body = accordion.querySelector('.sidebar-accordion-body');
    var toggle = accordion.querySelector('.sidebar-accordion-toggle');
    var chevron = toggle && toggle.querySelector('.sidebar-accordion-chevron');

    if (!body) return;

    body.classList.add('sidebar-accordion-open');
    if (toggle) toggle.classList.add('text-sidebar-active', 'bg-sidebar-hover/50');
    if (chevron) chevron.classList.add('rotate-90');

    // Lazy-load: replace placeholder with actual entity list.
    var placeholder = body.querySelector('.sidebar-accordion-placeholder');
    if (placeholder) {
      var loadUrl = placeholder.getAttribute('data-load-url');
      var resultsContainer = body.querySelector('.sidebar-accordion-results');

      if (prefetchCache[slug] && resultsContainer) {
        // Use prefetched content.
        resultsContainer.innerHTML = prefetchCache[slug];
        htmx.process(resultsContainer);
        delete prefetchCache[slug];
      } else if (loadUrl && resultsContainer) {
        // Fetch via HTMX.
        htmx.ajax('GET', loadUrl, {
          target: resultsContainer,
          swap: 'innerHTML'
        });
      }
    }
  }

  /**
   * Close a single accordion section.
   */
  function closeAccordion(accordion) {
    var body = accordion.querySelector('.sidebar-accordion-body');
    var toggle = accordion.querySelector('.sidebar-accordion-toggle');
    var chevron = toggle && toggle.querySelector('.sidebar-accordion-chevron');

    if (body) body.classList.remove('sidebar-accordion-open');
    if (toggle) toggle.classList.remove('text-sidebar-active', 'bg-sidebar-hover/50');
    if (chevron) chevron.classList.remove('rotate-90');
  }

  /**
   * Close all open accordions within the zone.
   */
  function closeAllAccordions(zone) {
    var openBodies = zone.querySelectorAll('.sidebar-accordion-body.sidebar-accordion-open');
    openBodies.forEach(function (body) {
      var acc = body.closest('.sidebar-accordion');
      if (acc) closeAccordion(acc);
    });
  }

  /**
   * Refresh the entity results in an already-open accordion.
   */
  function refreshAccordionResults(accordion) {
    var resultsContainer = accordion.querySelector('.sidebar-accordion-results');
    if (!resultsContainer) return;

    // Find the search input to get the HTMX URL.
    var searchInput = accordion.querySelector('input[name="q"]');
    if (searchInput) {
      var loadUrl = searchInput.getAttribute('hx-get');
      if (loadUrl) {
        htmx.ajax('GET', loadUrl, {
          target: resultsContainer,
          swap: 'innerHTML'
        });
      }
    }
  }

  /**
   * Ensure sidebar is expanded when interacting with accordion content.
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
})();
