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

    // Message.
    var msg = document.createElement('span');
    msg.style.cssText = 'flex:1;';
    msg.textContent = message;
    toast.appendChild(msg);

    // Close button.
    var close = document.createElement('button');
    close.type = 'button';
    close.style.cssText = 'border:none;background:none;color:var(--color-text-muted,#9ca3af);cursor:pointer;padding:0;font-size:14px;flex-shrink:0;line-height:1;';
    close.innerHTML = '&times;';
    close.addEventListener('click', function () {
      dismissToast(toast, id);
    });
    toast.appendChild(close);

    container.appendChild(toast);
    toasts.push({ id: id, el: toast });

    // Animate in.
    requestAnimationFrame(function () {
      toast.style.transform = 'translateX(0)';
      toast.style.opacity = '1';
    });

    // Auto-dismiss.
    if (duration > 0) {
      setTimeout(function () {
        dismissToast(toast, id);
      }, duration);
    }
  };

  /**
   * Dismiss a toast with animation.
   */
  function dismissToast(toast, id) {
    // Avoid double-dismiss.
    if (!toast.parentNode) return;

    toast.style.transform = 'translateX(100%)';
    toast.style.opacity = '0';

    setTimeout(function () {
      if (toast.parentNode) toast.parentNode.removeChild(toast);
      toasts = toasts.filter(function (t) { return t.id !== id; });
    }, 300);
  }

  // --- HTMX Integration ---
  // Show toasts for HTMX request errors automatically.
  document.addEventListener('htmx:responseError', function (evt) {
    var status = evt.detail.xhr ? evt.detail.xhr.status : 0;
    var msg = 'Request failed';
    if (status === 403) msg = 'Permission denied';
    else if (status === 404) msg = 'Not found';
    else if (status >= 500) msg = 'Server error. Please try again.';
    Chronicle.notify(msg, 'error');
  });

  // Show toast for HTMX connection errors.
  document.addEventListener('htmx:sendError', function () {
    Chronicle.notify('Connection error. Please check your network.', 'error', { duration: 6000 });
  });
})();
