/**
 * boot.js -- Chronicle Widget Auto-Mounter
 *
 * Scans the DOM for elements with `data-widget` attributes and mounts
 * registered widget implementations. Widgets register themselves via
 * `Chronicle.register(name, { init, destroy })`.
 *
 * Usage in HTML:
 *   <div data-widget="editor"
 *        data-endpoint="/api/v1/campaigns/abc/entities/42/entry"
 *        data-editable="true">
 *   </div>
 *
 * Widget registration:
 *   Chronicle.register('editor', {
 *       init(el, config) { ... },
 *       destroy(el) { ... }
 *   });
 *
 * After HTMX swaps (hx-swap), new widgets are auto-mounted via the
 * htmx:afterSettle event listener.
 */
(function () {
  'use strict';

  // Global namespace for Chronicle's widget system.
  window.Chronicle = window.Chronicle || {};

  // Registry of widget implementations keyed by name.
  var widgets = {};

  // WeakMap tracking mounted widget instances to prevent double-init
  // and enable cleanup on destroy.
  var mounted = new WeakMap();

  /**
   * Register a widget implementation.
   *
   * @param {string} name - Widget name (matches data-widget attribute).
   * @param {Object} impl - Implementation with init(el, config) and optional destroy(el).
   */
  Chronicle.register = function (name, impl) {
    if (!name || !impl || typeof impl.init !== 'function') {
      console.error('[Chronicle] Invalid widget registration:', name);
      return;
    }
    widgets[name] = impl;

    // If DOM is already loaded, mount any existing elements for this widget.
    if (document.readyState !== 'loading') {
      mountWidgets(document, name);
    }
  };

  /**
   * Mount all widget elements within a root element.
   * If widgetName is provided, only mount widgets of that type.
   *
   * @param {Element} root - Root element to scan.
   * @param {string} [widgetName] - Optional: only mount this widget type.
   */
  function mountWidgets(root, widgetName) {
    var selector = widgetName
      ? '[data-widget="' + widgetName + '"]'
      : '[data-widget]';

    var elements = root.querySelectorAll(selector);
    for (var i = 0; i < elements.length; i++) {
      mountElement(elements[i]);
    }
  }

  /**
   * Mount a single widget element.
   *
   * @param {Element} el - DOM element with data-widget attribute.
   */
  function mountElement(el) {
    // Skip if already mounted.
    if (mounted.has(el)) {
      return;
    }

    var name = el.getAttribute('data-widget');
    var impl = widgets[name];
    if (!impl) {
      // Widget not yet registered -- will be mounted when register() is called.
      return;
    }

    // Collect all data-* attributes as a config object.
    // data-endpoint="/foo" -> config.endpoint = "/foo"
    // data-auto-save="30" -> config.autoSave = "30"
    var config = {};
    for (var j = 0; j < el.attributes.length; j++) {
      var attr = el.attributes[j];
      if (attr.name.startsWith('data-') && attr.name !== 'data-widget') {
        var key = attr.name
          .slice(5) // Remove 'data-'
          .replace(/-([a-z])/g, function (_, c) {
            return c.toUpperCase();
          }); // kebab-case to camelCase
        config[key] = attr.value;
      }
    }

    // Parse boolean and numeric values.
    for (var k in config) {
      if (config[k] === 'true') config[k] = true;
      else if (config[k] === 'false') config[k] = false;
      else if (config[k] !== '' && !isNaN(Number(config[k])))
        config[k] = Number(config[k]);
    }

    try {
      impl.init(el, config);
      mounted.set(el, name);
    } catch (err) {
      console.error('[Chronicle] Failed to mount widget "' + name + '":', err);
    }
  }

  /**
   * Destroy a mounted widget and clean up.
   *
   * @param {Element} el - DOM element with a mounted widget.
   */
  function destroyElement(el) {
    var name = mounted.get(el);
    if (!name) return;

    var impl = widgets[name];
    if (impl && typeof impl.destroy === 'function') {
      try {
        impl.destroy(el);
      } catch (err) {
        console.error(
          '[Chronicle] Failed to destroy widget "' + name + '":',
          err
        );
      }
    }
    mounted.delete(el);
  }

  // --- CSRF Integration ---
  // Automatically attach the CSRF token to all HTMX mutating requests.
  // Reads the token from the chronicle_csrf cookie (set by CSRF middleware).
  document.addEventListener('htmx:configRequest', function (evt) {
    var match = document.cookie.match(
      '(?:^|; )chronicle_csrf=([^;]*)'
    );
    if (match) {
      evt.detail.headers['X-CSRF-Token'] = decodeURIComponent(match[1]);
    }
  });

  // --- Lifecycle ---

  // Mount all widgets on initial page load.
  document.addEventListener('DOMContentLoaded', function () {
    mountWidgets(document);
  });

  // Re-mount widgets after HTMX content swaps.
  // htmx:afterSettle fires after new content is settled in the DOM.
  document.addEventListener('htmx:afterSettle', function (event) {
    if (event.detail && event.detail.target) {
      mountWidgets(event.detail.target);
    }
  });

  // Destroy widgets before HTMX removes content.
  // htmx:beforeSwap fires before old content is removed.
  document.addEventListener('htmx:beforeSwap', function (event) {
    if (event.detail && event.detail.target) {
      var elements = event.detail.target.querySelectorAll('[data-widget]');
      for (var i = 0; i < elements.length; i++) {
        destroyElement(elements[i]);
      }
    }
  });

  // --- Sidebar Active Link Highlighting (for hx-boost navigation) ---
  // When hx-boost swaps only #main-content, the sidebar is NOT re-rendered.
  // Active/inactive CSS classes on sidebar links must be updated client-side
  // after the URL changes. Uses longest-prefix-match to highlight the most
  // specific matching link.

  var ACTIVE_CLASSES = ['bg-sidebar-hover', 'text-sidebar-active'];
  var INACTIVE_CLASSES = ['text-sidebar-text', 'hover:bg-sidebar-hover', 'hover:text-sidebar-active'];

  /**
   * Update sidebar navigation link active/inactive CSS classes
   * based on the current URL path.
   */
  function updateSidebarActiveLinks() {
    var path = window.location.pathname;
    var sidebar = document.getElementById('sidebar');
    if (!sidebar) return;

    // Collect all nav links (skip category drill-down links).
    var links = sidebar.querySelectorAll('a');
    var candidates = [];

    for (var i = 0; i < links.length; i++) {
      var link = links[i];
      var href = link.getAttribute('href');
      if (!href) continue;
      // Only process styled nav links (those with active or inactive classes).
      if (!link.classList.contains('text-sidebar-text') &&
          !link.classList.contains('text-sidebar-active')) continue;
      // Skip category drill-down links (handled by sidebar_drill.js).
      if (link.classList.contains('sidebar-category-link')) continue;

      candidates.push({ el: link, href: href });
    }

    // Find the longest matching href (most specific match wins).
    var bestHref = null;
    var bestLen = -1;
    for (var j = 0; j < candidates.length; j++) {
      var h = candidates[j].href;
      if (path === h || path.indexOf(h + '/') === 0) {
        if (h.length > bestLen) {
          bestLen = h.length;
          bestHref = h;
        }
      }
    }

    // Apply active/inactive classes.
    for (var k = 0; k < candidates.length; k++) {
      var c = candidates[k];
      var isActive = (c.href === bestHref);
      var m;

      if (isActive) {
        for (m = 0; m < INACTIVE_CLASSES.length; m++) c.el.classList.remove(INACTIVE_CLASSES[m]);
        for (m = 0; m < ACTIVE_CLASSES.length; m++) c.el.classList.add(ACTIVE_CLASSES[m]);
      } else {
        for (m = 0; m < ACTIVE_CLASSES.length; m++) c.el.classList.remove(ACTIVE_CLASSES[m]);
        for (m = 0; m < INACTIVE_CLASSES.length; m++) c.el.classList.add(INACTIVE_CLASSES[m]);
      }
    }
  }

  // Update sidebar active links after hx-boost pushes a new URL.
  // Also close mobile sidebar and notify Alpine.js components.
  document.addEventListener('htmx:pushedIntoHistory', function () {
    updateSidebarActiveLinks();
    window.dispatchEvent(new CustomEvent('chronicle:navigated'));
  });

  // Also handle browser back/forward navigation.
  window.addEventListener('popstate', function () {
    requestAnimationFrame(updateSidebarActiveLinks);
  });

  // Expose mount/destroy for manual use if needed.
  Chronicle.mountWidgets = mountWidgets;
  Chronicle.destroyWidget = destroyElement;
})();
