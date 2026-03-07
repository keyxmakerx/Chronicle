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

  // --- HTMX Loading Indicator ---
  // Toggle body.htmx-request class to show/hide the global progress bar.
  var activeRequests = 0;
  document.addEventListener('htmx:beforeRequest', function () {
    activeRequests++;
    document.body.classList.add('htmx-request');
  });
  document.addEventListener('htmx:afterRequest', function () {
    activeRequests = Math.max(0, activeRequests - 1);
    if (activeRequests === 0) {
      document.body.classList.remove('htmx-request');
    }
  });

  // --- Form Validation Feedback ---
  // On invalid submit, show inline .field-error hints below invalid fields
  // and add .input-error class for red border styling.
  document.addEventListener('invalid', function (e) {
    var el = e.target;
    if (!el.classList.contains('input')) return;
    el.classList.add('input-error');
    // Remove existing hint if any.
    var next = el.nextElementSibling;
    if (next && next.classList.contains('field-error')) next.remove();
    // Insert validation message.
    if (el.validationMessage) {
      var hint = document.createElement('div');
      hint.className = 'field-error';
      hint.textContent = el.validationMessage;
      el.parentNode.insertBefore(hint, el.nextSibling);
    }
  }, true);
  // Clear error state on input.
  document.addEventListener('input', function (e) {
    var el = e.target;
    if (!el.classList.contains('input-error')) return;
    el.classList.remove('input-error');
    var next = el.nextElementSibling;
    if (next && next.classList.contains('field-error')) next.remove();
  }, true);

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

  // --- Unsaved Changes Warning ---
  // Global dirty state tracker. Widgets and forms register themselves as
  // dirty sources. A single beforeunload listener warns the user when
  // navigating away with unsaved changes.

  var dirtySources = {};

  /**
   * Mark a source as having unsaved changes.
   * @param {string} id - Unique source identifier (e.g. 'editor', 'form:entity-edit').
   */
  Chronicle.markDirty = function (id) {
    dirtySources[id] = true;
  };

  /**
   * Mark a source as clean (changes saved or discarded).
   * @param {string} id - Source identifier to clear.
   */
  Chronicle.markClean = function (id) {
    delete dirtySources[id];
  };

  /**
   * Check if any source has unsaved changes.
   * @returns {boolean}
   */
  Chronicle.isDirty = function () {
    for (var k in dirtySources) {
      if (dirtySources.hasOwnProperty(k)) return true;
    }
    return false;
  };

  // Warn user before leaving the page with unsaved changes.
  window.addEventListener('beforeunload', function (e) {
    if (Chronicle.isDirty()) {
      e.preventDefault();
      e.returnValue = '';
    }
  });

  // Clear all dirty sources when HTMX triggers a redirect (HX-Redirect).
  // This prevents false "unsaved changes" warnings on successful form
  // submissions that redirect to a new page.
  document.addEventListener('htmx:beforeRedirect', function () {
    dirtySources = {};
  });

  // Also clear dirty state when any HTMX request completes with a
  // redirect header. This catches cases where htmx:beforeRedirect
  // doesn't fire due to timing or response handling differences.
  document.addEventListener('htmx:afterRequest', function (event) {
    if (event.detail && event.detail.xhr) {
      var redirect = event.detail.xhr.getResponseHeader('HX-Redirect');
      if (redirect) {
        dirtySources = {};
      }
    }
  });

  // Clear form dirty sources when HTMX swaps out tracked forms.
  document.addEventListener('htmx:beforeSwap', function (event) {
    if (event.detail && event.detail.target) {
      var forms = event.detail.target.querySelectorAll('form[data-track-changes]');
      for (var i = 0; i < forms.length; i++) {
        var formId = forms[i].getAttribute('data-track-changes');
        if (formId) Chronicle.markClean('form:' + formId);
      }
    }
  });

  // --- Form Change Tracking ---
  // Forms with data-track-changes="<id>" are auto-tracked. Any input/change
  // event marks the form dirty. Successful HTMX submission clears it.

  function trackFormChanges(form) {
    var formId = form.getAttribute('data-track-changes');
    if (!formId || form._trackingChanges) return;
    form._trackingChanges = true;

    var dirtyKey = 'form:' + formId;

    function onInput() { Chronicle.markDirty(dirtyKey); }

    form.addEventListener('input', onInput);
    form.addEventListener('change', onInput);

    // Clear dirty state after successful HTMX request from this form.
    form.addEventListener('htmx:afterRequest', function (evt) {
      if (evt.detail && evt.detail.successful) {
        Chronicle.markClean(dirtyKey);
      }
    });
  }

  function initFormTracking(root) {
    var forms = root.querySelectorAll('form[data-track-changes]');
    for (var i = 0; i < forms.length; i++) {
      trackFormChanges(forms[i]);
    }
  }

  document.addEventListener('DOMContentLoaded', function () {
    initFormTracking(document);
  });

  document.addEventListener('htmx:afterSettle', function (event) {
    if (event.detail && event.detail.target) {
      initFormTracking(event.detail.target);
    }
  });

  // --- Shared Utilities ---
  // Centralized utility functions used by multiple widgets. Widgets should
  // call Chronicle.escapeHtml() etc. instead of defining their own copies.

  /**
   * Escape a string for safe insertion into HTML content.
   * Uses DOM textContent/innerHTML for correctness.
   *
   * @param {string} str - Raw string to escape.
   * @returns {string} HTML-safe string.
   */
  Chronicle.escapeHtml = function (str) {
    var div = document.createElement('div');
    div.textContent = String(str || '');
    return div.innerHTML;
  };

  /**
   * Escape a string for safe insertion into an HTML attribute value.
   *
   * @param {string} str - Raw string to escape.
   * @returns {string} Attribute-safe string.
   */
  Chronicle.escapeAttr = function (str) {
    return String(str || '')
      .replace(/&/g, '&amp;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  };

  /**
   * Read the CSRF token from the chronicle_csrf cookie.
   *
   * @returns {string} CSRF token value or empty string.
   */
  Chronicle.getCsrf = function () {
    var m = document.cookie.match('(?:^|; )chronicle_csrf=([^;]*)');
    return m ? decodeURIComponent(m[1]) : '';
  };

  /**
   * Convenience wrapper around fetch() for API calls.
   *
   * Automatically:
   *  - Sets Accept: application/json
   *  - Adds X-CSRF-Token header on mutating requests (POST/PUT/DELETE)
   *  - Serializes plain-object bodies as JSON (sets Content-Type)
   *  - Sets credentials: same-origin
   *
   * @param {string} url - Request URL.
   * @param {Object} [opts] - Options forwarded to fetch().
   * @param {string} [opts.method] - HTTP method (default GET).
   * @param {Object|FormData|string} [opts.body] - Request body. Plain objects are JSON-serialized.
   * @param {Object} [opts.headers] - Extra headers (merged with defaults).
   * @param {AbortSignal} [opts.signal] - AbortController signal.
   * @param {string} [opts.csrfToken] - Explicit CSRF token; falls back to cookie.
   * @returns {Promise<Response>} The fetch Response (caller handles .json() / .ok).
   */
  Chronicle.apiFetch = function (url, opts) {
    opts = opts || {};
    var method = (opts.method || 'GET').toUpperCase();
    var headers = {};

    // Merge caller-supplied headers first.
    if (opts.headers) {
      for (var k in opts.headers) {
        headers[k] = opts.headers[k];
      }
    }

    // Default Accept header.
    if (!headers['Accept']) {
      headers['Accept'] = 'application/json';
    }

    // Auto-attach CSRF token on mutating requests.
    if (method !== 'GET' && method !== 'HEAD' && !headers['X-CSRF-Token']) {
      var csrf = opts.csrfToken || Chronicle.getCsrf();
      if (csrf) {
        headers['X-CSRF-Token'] = csrf;
      }
    }

    // Serialize plain objects as JSON.
    var body = opts.body;
    if (body && typeof body === 'object' && !(body instanceof FormData)) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify(body);
    }

    var fetchOpts = {
      method: method,
      headers: headers,
      credentials: 'same-origin'
    };
    if (body !== undefined) fetchOpts.body = body;
    if (opts.signal) fetchOpts.signal = opts.signal;

    return fetch(url, fetchOpts);
  };
})();
