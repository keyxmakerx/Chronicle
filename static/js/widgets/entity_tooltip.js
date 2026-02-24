/**
 * entity_tooltip.js -- Chronicle Entity Tooltip/Popover Widget
 *
 * Provides hover tooltips for entity references throughout the app.
 * When the user hovers over any element with a `data-entity-preview`
 * attribute (whose value is the preview API URL), a floating card shows:
 *
 *   - Gradient-bordered image (entity type color -> purple) with
 *     attributes side-by-side when both are present
 *   - Type badge, optional descriptor, privacy indicator
 *   - Entity name
 *   - Up to 5 key-value attribute pairs from the entity's custom fields
 *   - Entry excerpt (first ~150 chars, 3-line clamp)
 *
 * Content is controlled per-entity via popup_config (showImage,
 * showAttributes, showEntry). Layout adapts dynamically.
 *
 * Two usage modes:
 *   1. Auto-mounted by boot.js on elements with data-widget="entity-tooltip"
 *      (scans children for data-entity-preview elements).
 *   2. Global helper: Chronicle.tooltip.attach(element, previewURL) for
 *      other widgets to programmatically attach tooltips.
 *
 * Features:
 *   - Debounced hover (300ms) to avoid API spam
 *   - Client-side LRU cache (max 100 entries)
 *   - Smart positioning (above or below, avoids viewport overflow)
 *   - Touch support (long press to show, tap elsewhere to dismiss)
 *   - Dark mode support via .dark class on <html>
 *   - Accessible: role="tooltip", aria-describedby
 *   - Inline CSS injected once (no Tailwind dependency)
 */
(function () {
  'use strict';

  // --- Scoped Styles (injected once) ---

  if (!document.getElementById('entity-tooltip-styles')) {
    var style = document.createElement('style');
    style.id = 'entity-tooltip-styles';
    style.textContent = [
      '.et-tooltip { position: fixed; z-index: 9999; width: 320px; max-width: 90vw; background: #ffffff; border: 1px solid #e5e7eb; border-radius: 12px; box-shadow: 0 8px 24px rgba(0,0,0,0.12); overflow: hidden; pointer-events: none; opacity: 0; transform: translateY(4px); transition: opacity 150ms ease, transform 150ms ease; font-family: Inter, system-ui, -apple-system, sans-serif; }',
      '.et-tooltip--visible { opacity: 1; transform: translateY(0); pointer-events: auto; }',
      '.et-tooltip--above { transform: translateY(-4px); }',
      '.et-tooltip--above.et-tooltip--visible { transform: translateY(0); }',
      '.dark .et-tooltip { background: #1f2937; border-color: #374151; box-shadow: 0 8px 24px rgba(0,0,0,0.3); }',
      /* Content area with image + attributes side by side */
      '.et-tooltip__content { display: flex; gap: 12px; padding: 12px; }',
      '.et-tooltip__content--no-image { display: block; padding: 12px; }',
      /* Gradient-bordered image */
      '.et-tooltip__image-wrap { flex-shrink: 0; width: 76px; height: 76px; padding: 2px; border-radius: 10px; background: linear-gradient(135deg, var(--et-color, #6366f1), #a855f7); }',
      '.et-tooltip__image { width: 100%; height: 100%; object-fit: cover; display: block; border-radius: 8px; }',
      '.et-tooltip__info { flex: 1; min-width: 0; }',
      '.et-tooltip__body { padding: 0 12px; }',
      '.et-tooltip__name { font-size: 15px; font-weight: 600; color: #111827; margin: 0 0 4px 0; line-height: 1.3; display: flex; align-items: center; gap: 6px; }',
      '.dark .et-tooltip__name { color: #f3f4f6; }',
      '.et-tooltip__private { color: #9ca3af; font-size: 11px; }',
      '.et-tooltip__badge { display: inline-flex; align-items: center; gap: 4px; padding: 2px 8px; border-radius: 9999px; font-size: 10px; font-weight: 500; line-height: 16px; white-space: nowrap; }',
      '.et-tooltip__label { font-size: 11px; color: #6b7280; margin-left: 4px; }',
      '.dark .et-tooltip__label { color: #9ca3af; }',
      '.et-tooltip__type-row { display: flex; align-items: center; flex-wrap: wrap; gap: 4px; margin-bottom: 6px; }',
      /* Attributes key-value list */
      '.et-tooltip__attrs { margin-top: 4px; }',
      '.et-tooltip__attr { display: flex; gap: 6px; font-size: 11px; line-height: 1.6; }',
      '.et-tooltip__attr-label { color: #9ca3af; flex-shrink: 0; }',
      '.dark .et-tooltip__attr-label { color: #6b7280; }',
      '.et-tooltip__attr-value { color: #374151; font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }',
      '.dark .et-tooltip__attr-value { color: #d1d5db; }',
      /* Excerpt */
      '.et-tooltip__excerpt { font-size: 12px; line-height: 1.5; color: #4b5563; margin: 0; padding: 0 12px 10px; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; }',
      '.dark .et-tooltip__excerpt { color: #d1d5db; }',
      '.et-tooltip__footer { padding: 6px 12px; border-top: 1px solid #f3f4f6; font-size: 10px; color: #9ca3af; }',
      '.dark .et-tooltip__footer { border-top-color: #374151; color: #6b7280; }',
      '.et-tooltip__loading { padding: 20px; text-align: center; font-size: 13px; color: #9ca3af; }'
    ].join('\n');
    document.head.appendChild(style);
  }

  // --- LRU Cache ---

  var MAX_CACHE = 100;
  var cache = {};
  var cacheOrder = []; // Most recently used at the end.

  /**
   * Retrieve a cached preview, promoting it to most-recently-used.
   * @param {string} url - Preview API URL.
   * @returns {Object|null} Cached preview data or null.
   */
  function cacheGet(url) {
    if (!(url in cache)) return null;
    // Promote to end (most recent).
    var idx = cacheOrder.indexOf(url);
    if (idx !== -1) cacheOrder.splice(idx, 1);
    cacheOrder.push(url);
    return cache[url];
  }

  /**
   * Store a preview in the cache, evicting the oldest entry if at capacity.
   * @param {string} url - Preview API URL.
   * @param {Object} data - Preview response data.
   */
  function cacheSet(url, data) {
    if (url in cache) {
      var idx = cacheOrder.indexOf(url);
      if (idx !== -1) cacheOrder.splice(idx, 1);
    } else if (cacheOrder.length >= MAX_CACHE) {
      // Evict least recently used.
      var oldest = cacheOrder.shift();
      delete cache[oldest];
    }
    cache[url] = data;
    cacheOrder.push(url);
  }

  // --- Tooltip Singleton ---

  var tooltipEl = null;    // The tooltip DOM element (created lazily).
  var activeTarget = null; // Currently hovered trigger element.
  var hoverTimer = null;   // Debounce timer for showing.
  var hideTimer = null;    // Delay timer for hiding.
  var touchTimer = null;   // Long-press timer for touch.
  var tooltipIdCounter = 0;

  /**
   * Create the singleton tooltip element if it doesn't exist.
   * @returns {HTMLElement}
   */
  function ensureTooltip() {
    if (tooltipEl) return tooltipEl;
    tooltipEl = document.createElement('div');
    tooltipEl.className = 'et-tooltip';
    tooltipEl.setAttribute('role', 'tooltip');
    tooltipEl.id = 'entity-tooltip-' + (++tooltipIdCounter);
    tooltipEl.style.display = 'none';
    document.body.appendChild(tooltipEl);

    // Allow hovering over the tooltip itself without dismissing it.
    tooltipEl.addEventListener('mouseenter', function () {
      clearTimeout(hideTimer);
    });
    tooltipEl.addEventListener('mouseleave', function () {
      hideTooltip();
    });

    return tooltipEl;
  }

  /**
   * Render the tooltip content from preview data.
   * @param {Object} data - Preview API response.
   * @param {string} entityURL - URL to the entity page.
   */
  function renderTooltip(data, entityURL) {
    var tip = ensureTooltip();
    var html = '';
    var hasImage = data.image_path && data.image_path !== '';
    var hasAttrs = data.attributes && data.attributes.length > 0;
    var hasExcerpt = data.entry_excerpt && data.entry_excerpt !== '';

    // Set the entity type color as a CSS custom property for the gradient border.
    tip.style.setProperty('--et-color', data.type_color || '#6366f1');

    // Content area: image (with gradient border) + info side by side.
    html += '<div class="' + (hasImage ? 'et-tooltip__content' : 'et-tooltip__content--no-image') + '">';

    if (hasImage) {
      html += '<div class="et-tooltip__image-wrap">';
      html += '<img class="et-tooltip__image" src="' + escapeAttr(data.image_path) + '" alt="' + escapeAttr(data.name) + '" />';
      html += '</div>';
    }

    html += '<div class="et-tooltip__info">';

    // Type badge row.
    html += '<div class="et-tooltip__type-row">';
    html += '<span class="et-tooltip__badge" style="background-color: ' + escapeAttr(data.type_color) + '; color: ' + contrastTextColor(data.type_color) + '">';
    if (data.type_icon) {
      html += '<i class="fa-solid ' + escapeAttr(data.type_icon) + '" style="font-size:9px"></i> ';
    }
    html += escapeHtml(data.type_name);
    html += '</span>';
    if (data.type_label) {
      html += '<span class="et-tooltip__label">' + escapeHtml(data.type_label) + '</span>';
    }
    if (data.is_private) {
      html += ' <span class="et-tooltip__private" title="Private"><i class="fa-solid fa-lock" style="font-size:9px"></i></span>';
    }
    html += '</div>';

    // Entity name.
    html += '<div class="et-tooltip__name">' + escapeHtml(data.name) + '</div>';

    // Attributes (key-value pairs, up to 5).
    if (hasAttrs) {
      html += '<div class="et-tooltip__attrs">';
      for (var i = 0; i < data.attributes.length; i++) {
        var attr = data.attributes[i];
        html += '<div class="et-tooltip__attr">';
        html += '<span class="et-tooltip__attr-label">' + escapeHtml(attr.label) + '</span>';
        html += '<span class="et-tooltip__attr-value">' + escapeHtml(attr.value) + '</span>';
        html += '</div>';
      }
      html += '</div>';
    }

    html += '</div>'; // end .et-tooltip__info
    html += '</div>'; // end .et-tooltip__content

    // Entry excerpt below the content area.
    if (hasExcerpt) {
      html += '<p class="et-tooltip__excerpt">' + escapeHtml(data.entry_excerpt) + '</p>';
    }

    // Footer with "Click to view" hint.
    html += '<div class="et-tooltip__footer">Click to view &rarr;</div>';

    tip.innerHTML = html;
  }

  /**
   * Position the tooltip near the target element using smart placement.
   * Prefers below the element; flips above if insufficient space below.
   * @param {HTMLElement} target - The trigger element.
   */
  function positionTooltip(target) {
    var tip = ensureTooltip();
    var rect = target.getBoundingClientRect();
    var tipRect = tip.getBoundingClientRect();
    var gap = 8;
    var viewW = window.innerWidth;
    var viewH = window.innerHeight;

    // Determine vertical placement: prefer below, flip above if no room.
    var placeAbove = false;
    var top;
    if (rect.bottom + gap + tipRect.height > viewH && rect.top - gap - tipRect.height > 0) {
      // Not enough room below, but enough above.
      placeAbove = true;
      top = rect.top - tipRect.height - gap;
    } else {
      top = rect.bottom + gap;
    }

    // Horizontal: center on the target, but clamp to viewport.
    var left = rect.left + (rect.width / 2) - (tipRect.width / 2);
    if (left < 8) left = 8;
    if (left + tipRect.width > viewW - 8) left = viewW - 8 - tipRect.width;

    tip.style.top = top + 'px';
    tip.style.left = left + 'px';

    tip.classList.toggle('et-tooltip--above', placeAbove);
  }

  /**
   * Show the tooltip for a target element with the given preview URL.
   * Fetches data if not cached, then renders and positions.
   * @param {HTMLElement} target - The trigger element.
   * @param {string} previewURL - The preview API endpoint.
   */
  function showTooltip(target, previewURL) {
    if (!previewURL) return;
    activeTarget = target;

    var tip = ensureTooltip();
    tip.style.display = 'block';
    tip.classList.remove('et-tooltip--visible');

    // Set ARIA attributes on the trigger.
    target.setAttribute('aria-describedby', tip.id);

    var cached = cacheGet(previewURL);
    if (cached) {
      renderTooltip(cached, target.getAttribute('href') || '#');
      // Position after render (need dimensions).
      requestAnimationFrame(function () {
        positionTooltip(target);
        tip.classList.add('et-tooltip--visible');
      });
      return;
    }

    // Show loading state.
    tip.innerHTML = '<div class="et-tooltip__loading">Loading...</div>';
    requestAnimationFrame(function () {
      positionTooltip(target);
      tip.classList.add('et-tooltip--visible');
    });

    // Fetch preview data.
    fetch(previewURL, {
      method: 'GET',
      headers: { 'Accept': 'application/json' },
      credentials: 'same-origin'
    })
      .then(function (res) {
        if (!res.ok) throw new Error('Preview fetch failed: ' + res.status);
        return res.json();
      })
      .then(function (data) {
        cacheSet(previewURL, data);
        // Only render if this target is still the active one.
        if (activeTarget === target) {
          renderTooltip(data, target.getAttribute('href') || '#');
          requestAnimationFrame(function () {
            positionTooltip(target);
          });
        }
      })
      .catch(function () {
        // Silently fail: hide tooltip if fetch fails.
        if (activeTarget === target) {
          hideTooltip();
        }
      });
  }

  /**
   * Hide the tooltip and clean up ARIA attributes.
   */
  function hideTooltip() {
    clearTimeout(hoverTimer);
    clearTimeout(hideTimer);
    clearTimeout(touchTimer);

    if (tooltipEl) {
      tooltipEl.classList.remove('et-tooltip--visible');
      // Wait for fade-out transition before hiding.
      setTimeout(function () {
        if (tooltipEl && !tooltipEl.classList.contains('et-tooltip--visible')) {
          tooltipEl.style.display = 'none';
        }
      }, 160);
    }

    if (activeTarget) {
      activeTarget.removeAttribute('aria-describedby');
      activeTarget = null;
    }
  }

  // --- Event Delegation (mouse) ---

  /**
   * Find the closest ancestor (or self) with data-entity-preview.
   * @param {HTMLElement} el
   * @returns {HTMLElement|null}
   */
  function findPreviewTrigger(el) {
    return el.closest ? el.closest('[data-entity-preview]') : null;
  }

  // Use event delegation on the document for efficiency.
  document.addEventListener('mouseover', function (e) {
    var trigger = findPreviewTrigger(e.target);
    if (!trigger) return;

    // If we're already showing for this trigger, skip.
    if (activeTarget === trigger) {
      clearTimeout(hideTimer);
      return;
    }

    // Clear any pending show/hide.
    clearTimeout(hoverTimer);
    clearTimeout(hideTimer);

    // Debounce: wait 300ms before fetching/showing.
    hoverTimer = setTimeout(function () {
      var url = trigger.getAttribute('data-entity-preview');
      showTooltip(trigger, url);
    }, 300);
  });

  document.addEventListener('mouseout', function (e) {
    var trigger = findPreviewTrigger(e.target);
    if (!trigger) return;

    clearTimeout(hoverTimer);

    // Small delay before hiding so user can move mouse to tooltip.
    hideTimer = setTimeout(function () {
      hideTooltip();
    }, 100);
  });

  // --- Event Delegation (touch) ---

  document.addEventListener('touchstart', function (e) {
    var trigger = findPreviewTrigger(e.target);
    if (!trigger) {
      // Touch outside: dismiss any visible tooltip.
      if (activeTarget) hideTooltip();
      return;
    }

    // Long press: show tooltip after 500ms.
    clearTimeout(touchTimer);
    touchTimer = setTimeout(function () {
      e.preventDefault();
      var url = trigger.getAttribute('data-entity-preview');
      showTooltip(trigger, url);
    }, 500);
  }, { passive: false });

  document.addEventListener('touchend', function () {
    clearTimeout(touchTimer);
  });

  document.addEventListener('touchmove', function () {
    clearTimeout(touchTimer);
  });

  // --- Dismiss on Escape and Scroll ---

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && activeTarget) {
      hideTooltip();
    }
  });

  // Dismiss on any scroll (window or scrollable ancestor).
  window.addEventListener('scroll', function () {
    if (activeTarget) hideTooltip();
  }, true); // Capture phase to catch all scrollable elements.

  // --- Utility ---

  function escapeHtml(str) {
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  function escapeAttr(str) {
    return str
      .replace(/&/g, '&amp;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }

  /**
   * Return a contrasting text color (hex) for the given background hex color.
   * Uses ITU-R BT.709 perceived brightness formula.
   * @param {string} hex - Background color (e.g., "#ff0000").
   * @returns {string} "#ffffff" or "#1f2937".
   */
  function contrastTextColor(hex) {
    hex = (hex || '').replace('#', '');
    if (hex.length === 3) {
      hex = hex[0] + hex[0] + hex[1] + hex[1] + hex[2] + hex[2];
    }
    if (hex.length !== 6) return '#ffffff';
    var r = parseInt(hex.substring(0, 2), 16);
    var g = parseInt(hex.substring(2, 4), 16);
    var b = parseInt(hex.substring(4, 6), 16);
    var luminance = 0.299 * r + 0.587 * g + 0.114 * b;
    return luminance > 186 ? '#1f2937' : '#ffffff';
  }

  // --- Widget Registration ---

  // Register as a widget so boot.js can mount it on container elements.
  // When mounted, it does nothing extra beyond what event delegation already
  // handles -- but this allows explicit scoping and lifecycle management.
  Chronicle.register('entity-tooltip', {
    init: function (el, config) {
      // Nothing to do: event delegation on document handles everything.
      // This registration exists so other code can explicitly place a
      // data-widget="entity-tooltip" container to signal intent.
      el._entityTooltipInit = true;
    },
    destroy: function (el) {
      delete el._entityTooltipInit;
    }
  });

  // --- Global Helper API ---

  // Expose a global helper so other widgets (e.g., editor, relation lists)
  // can programmatically attach tooltip behavior to dynamic elements.
  Chronicle.tooltip = {
    /**
     * Attach tooltip behavior to an element.
     * Sets the data-entity-preview attribute so event delegation picks it up.
     *
     * @param {HTMLElement} element - The element to make hoverable.
     * @param {string} previewURL - The entity preview API URL.
     */
    attach: function (element, previewURL) {
      if (element && previewURL) {
        element.setAttribute('data-entity-preview', previewURL);
      }
    },

    /**
     * Detach tooltip behavior from an element.
     *
     * @param {HTMLElement} element - The element to clean up.
     */
    detach: function (element) {
      if (element) {
        element.removeAttribute('data-entity-preview');
        if (activeTarget === element) {
          hideTooltip();
        }
      }
    },

    /**
     * Manually show the tooltip for an element.
     *
     * @param {HTMLElement} element - The trigger element.
     * @param {string} previewURL - The entity preview API URL.
     */
    show: function (element, previewURL) {
      showTooltip(element, previewURL);
    },

    /**
     * Manually hide the tooltip.
     */
    hide: function () {
      hideTooltip();
    },

    /**
     * Clear the preview cache (useful after entity edits).
     */
    clearCache: function () {
      cache = {};
      cacheOrder = [];
    }
  };
})();
