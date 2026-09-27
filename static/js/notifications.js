/**
 * notifications.js -- Chronicle Toast Notification System
 *
 * Provides a global toast notification API for showing brief success, error,
 * info, and warning messages. Toasts appear in the top-right corner and
 * auto-dismiss after a configurable duration.
 *
 * Usage:
 *   Chronicle.notify('Entity saved successfully', 'success');
 *   Chronicle.notify('Failed to save', 'error');
 *   Chronicle.notify('New relation added', 'info');
 *   Chronicle.notify('Unsaved changes', 'warning', { duration: 10000 });
 *
 * Also listens for HTMX `afterRequest` events to show success/error toasts
 * based on HTTP response status when HTMX requests complete.
 */
(function () {
  'use strict';

  // Ensure global namespace exists.
  window.Chronicle = window.Chronicle || {};

  // Container element for toast notifications.
  var container = null;
  var toasts = [];
  var nextId = 0;

  /**
   * Whether the visitor asked the OS/browser for reduced motion. Checked live
   * (not cached) since it can change mid-session and toasts are short-lived.
   */
  function prefersReducedMotion() {
    return typeof window !== 'undefined' && typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  }

  /**
   * Find an on-screen toast with the same message and type, so a repeat can
   * reuse it (count badge + restarted timer) instead of stacking a copy.
   */
  function findActiveToast(message, type) {
    for (var i = 0; i < toasts.length; i++) {
      var t = toasts[i];
      // A toast mid fade-out is on its way off screen; a repeat gets a new one.
      if (t.message === message && t.type === type && t.el.parentNode && !t.el.dataset.dismissing) return t;
    }
    return null;
  }

  // Toast type configuration: icon, colors.
  var typeConfig = {
    success: {
      icon: 'fa-check-circle',
      bg: 'var(--color-card-bg, #fff)',
      border: '#22c55e',
      text: '#16a34a',
      darkText: '#4ade80',
    },
    error: {
      icon: 'fa-exclamation-circle',
      bg: 'var(--color-card-bg, #fff)',
      border: '#ef4444',
      text: '#dc2626',
      darkText: '#f87171',
    },
    info: {
      icon: 'fa-info-circle',
      bg: 'var(--color-card-bg, #fff)',
      border: '#3b82f6',
      text: '#2563eb',
      darkText: '#60a5fa',
    },
    warning: {
      icon: 'fa-exclamation-triangle',
      bg: 'var(--color-card-bg, #fff)',
      border: '#f59e0b',
      text: '#d97706',
      darkText: '#fbbf24',
    },
  };

  /**
   * Ensure the toast container exists in the DOM.
   */
  function ensureContainer() {
    if (container && document.body.contains(container)) return;

    // Inject the count badge's bump-on-repeat animation once. A transition,
    // not a hand-rolled transform in JS, so prefers-reduced-motion is
    // handled by simply never adding the class that triggers it.
    if (!document.getElementById('chronicle-toast-styles')) {
      var style = document.createElement('style');
      style.id = 'chronicle-toast-styles';
      style.textContent = [
        '.chronicle-toast-badge { transform: scale(1); transition: transform 0.15s ease; }',
        '.chronicle-toast-badge-bump { transform: scale(1.35); }',
      ].join(' ');
      document.head.appendChild(style);
    }

    container = document.createElement('div');
    container.id = 'chronicle-toasts';
    container.style.cssText = [
      'position: fixed',
      'top: 16px',
      'right: 16px',
      'z-index: 10000',
      'display: flex',
      'flex-direction: column',
      'gap: 8px',
      'pointer-events: none',
      'max-width: 380px',
      'width: 100%',
    ].join(';');
    document.body.appendChild(container);
  }

  /**
   * Show a toast notification.
   *
   * @param {string} message - The notification message.
   * @param {string} [type='info'] - Type: 'success', 'error', 'info', 'warning'.
   * @param {Object} [opts] - Options.
   * @param {number} [opts.duration=4000] - Auto-dismiss duration in ms. 0 = manual.
   */
  Chronicle.notify = function (message, type, opts) {
    type = type || 'info';
    opts = opts || {};
    var duration = opts.duration !== undefined ? opts.duration : 4000;

    ensureContainer();

    // A repeat of an on-screen toast (same message and type) reuses it --
    // a count badge and a restarted timer -- instead of stacking a copy
    // underneath it. Everything else about toasts stays the same.
    var existing = findActiveToast(message, type);
    if (existing) {
      bumpToast(existing, duration);
      return;
    }

    var config = typeConfig[type] || typeConfig.info;
    var id = ++nextId;

    var toast = document.createElement('div');
    toast.className = 'chronicle-toast';
    toast.dataset.toastId = id;
    toast.style.cssText = [
      'pointer-events: auto',
      'display: flex',
      'align-items: flex-start',
      'gap: 10px',
      'padding: 12px 14px',
      'background: ' + config.bg,
      'border: 1px solid var(--color-border, #e5e7eb)',
      'border-left: 4px solid ' + config.border,
      'border-radius: 8px',
      'box-shadow: 0 4px 12px rgba(0,0,0,0.1)',
      'font-size: 13px',
      'line-height: 1.4',
      'color: var(--color-text-body, #374151)',
      'transform: translateX(100%)',
      'opacity: 0',
      'transition: transform 0.3s ease, opacity 0.3s ease',
    ].join(';');

    // Icon.
    var icon = document.createElement('i');
    icon.className = 'fa-solid ' + config.icon;
    icon.style.cssText = 'color:' + config.text + ';font-size:16px;margin-top:1px;flex-shrink:0;';
    toast.appendChild(icon);

    // Message. Supports HTML when opts.html is true (use only with trusted content).
    var msg = document.createElement('span');
    msg.style.cssText = 'flex:1;';
    if (opts.html) {
      msg.innerHTML = message;
    } else {
      msg.textContent = message;
    }
    toast.appendChild(msg);

    var record = { id: id, el: toast, message: message, type: type, count: 1, msgEl: msg, badgeEl: null, timer: null, duration: duration };

    // Close button.
    var close = document.createElement('button');
    close.type = 'button';
    close.style.cssText = 'border:none;background:none;color:var(--color-text-muted,#9ca3af);cursor:pointer;padding:0;font-size:14px;flex-shrink:0;line-height:1;';
    close.innerHTML = '&times;';
    close.addEventListener('click', function () {
      if (record.timer) clearTimeout(record.timer);
      dismissToast(toast, id);
    });
    toast.appendChild(close);

    container.appendChild(toast);
    toasts.push(record);

    // Animate in.
    requestAnimationFrame(function () {
      toast.style.transform = 'translateX(0)';
      toast.style.opacity = '1';
    });

    restartTimer(record);
  };

  /**
   * (Re)start a toast's auto-dismiss timer, clearing any timer already
   * running. Shared by the initial show and every repeat, so a repeat
   * genuinely restarts the countdown instead of leaving the old one armed
   * alongside a new one.
   */
  function restartTimer(record) {
    if (record.timer) {
      clearTimeout(record.timer);
      record.timer = null;
    }
    if (record.duration > 0) {
      record.timer = setTimeout(function () {
        dismissToast(record.el, record.id);
      }, record.duration);
    }
  }

  /**
   * Reuse an on-screen toast for a repeat: bump its count badge and restart
   * its timer. The badge's "grow" animation is skipped under
   * prefers-reduced-motion; the count itself always updates.
   */
  function bumpToast(record, duration) {
    record.count += 1;
    record.duration = duration;

    if (!record.badgeEl) {
      var badge = document.createElement('span');
      badge.className = 'chronicle-toast-badge';
      badge.style.cssText = [
        'display: inline-flex',
        'align-items: center',
        'justify-content: center',
        'min-width: 16px',
        'height: 16px',
        'padding: 0 5px',
        'margin-left: 6px',
        'border-radius: 999px',
        'background: var(--color-accent-light, #a5b4fc)',
        'color: var(--color-accent-hover, #4f46e5)',
        'font-size: 11px',
        'font-weight: 700',
        'vertical-align: 1px',
      ].join(';');
      record.msgEl.appendChild(badge);
      record.badgeEl = badge;
    }
    record.badgeEl.textContent = '×' + record.count;

    if (!prefersReducedMotion()) {
      record.badgeEl.classList.add('chronicle-toast-badge-bump');
      setTimeout(function () {
        record.badgeEl.classList.remove('chronicle-toast-badge-bump');
      }, 200);
    }

    restartTimer(record);
  }

  /**
   * Dismiss a toast with animation.
   */
  function dismissToast(toast, id) {
    // Avoid double-dismiss.
    if (!toast.parentNode || toast.dataset.dismissing) return;
    toast.dataset.dismissing = 'true';

    toast.style.transform = 'translateX(100%)';
    toast.style.opacity = '0';

    setTimeout(function () {
      if (toast.parentNode) toast.parentNode.removeChild(toast);
      toasts = toasts.filter(function (t) { return t.id !== id; });
    }, 300);
  }

  // --- HTMX Integration ---
  // Listen for server-triggered notifications via HX-Trigger header.
  // Servers can send: HX-Trigger: {"chronicle:notify":{"message":"...","type":"error"}}
  document.addEventListener('chronicle:notify', function (evt) {
    var detail = evt.detail || {};
    if (detail.message) {
      Chronicle.notify(detail.message, detail.type || 'info', { duration: detail.duration });
    }
  });

  // Show toasts for HTMX request errors automatically.
  // Note: 4xx errors from HTMX are handled server-side via HX-Trigger
  // (chronicle:notify event), so this only fires for unhandled errors.
  document.addEventListener('htmx:responseError', function (evt) {
    var xhr = evt.detail.xhr;
    var status = xhr ? xhr.status : 0;
    // Skip if server already sent a chronicle:notify via HX-Trigger.
    if (xhr && xhr.getResponseHeader && xhr.getResponseHeader('HX-Trigger')) {
      try {
        var trigger = JSON.parse(xhr.getResponseHeader('HX-Trigger'));
        if (trigger['chronicle:notify']) return;
      } catch (e) { /* not JSON, continue */ }
    }
    var msg = 'Request failed';
    if (status === 400) msg = 'Invalid request';
    else if (status === 403) msg = 'Permission denied';
    else if (status === 404) msg = 'Not found';
    else if (status >= 500) msg = 'Server error. Please try again.';
    Chronicle.notify(msg, 'error');
  });

  // Show toast for HTMX connection errors.
  document.addEventListener('htmx:sendError', function () {
    Chronicle.notify('Connection error. Please check your network.', 'error', { duration: 6000 });
  });
})();
