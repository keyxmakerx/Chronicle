/**
 * Calendar Widget
 *
 * Interactive mini-calendar for dashboard and page embeds. Shows upcoming
 * events with click-to-view detail popup and quick-add capability for Scribes+.
 * Full editing stays on the dedicated calendar page.
 *
 * Mount: data-widget="calendar-widget"
 * Config:
 *   data-campaign-id  - Campaign UUID (required)
 *   data-limit        - Max events to show (default 5)
 */
(function () {
  'use strict';

  Chronicle.register('calendar-widget', {
    init: function (el, config) {
      this.el = el;
      this.campaignId = config.campaignId;
      this.limit = parseInt(config.limit, 10) || 5;
      this._quickAddOpen = false;

      this._bindEventClicks();
      this._addQuickAddButton();
    },

    /**
     * Bind click handlers on event items loaded via HTMX for detail popup.
     * Re-runs after HTMX swaps via MutationObserver.
     */
    _bindEventClicks: function () {
      var self = this;

      // Bind clicks on any event links within the widget.
      function bindItems() {
        var items = self.el.querySelectorAll('[data-event-id]');
        items.forEach(function (item) {
          if (item._calWidgetBound) return;
          item._calWidgetBound = true;
          item.style.cursor = 'pointer';
          item.addEventListener('click', function (e) {
            e.preventDefault();
            self._showEventDetail(item);
          });
        });
      }

      // Initial bind + observe for HTMX swaps.
      bindItems();
      this._observer = new MutationObserver(bindItems);
      this._observer.observe(this.el, { childList: true, subtree: true });
    },

    /**
     * Show a lightweight event detail popup.
     */
    _showEventDetail: function (item) {
      var name = item.dataset.eventName || item.textContent.trim();
      var eventId = item.dataset.eventId;

      // Remove any existing popup.
      var existing = this.el.querySelector('.cal-widget-popup');
      if (existing) existing.remove();

      var popup = document.createElement('div');
      popup.className = 'cal-widget-popup card p-4 shadow-lg absolute z-50';
      popup.style.cssText = 'min-width:220px;top:50%;left:50%;transform:translate(-50%,-50%);';
      popup.innerHTML =
        '<div class="flex items-center justify-between mb-2">' +
          '<span class="text-sm font-semibold text-fg">' + Chronicle.escapeHtml(name) + '</span>' +
          '<button class="cal-widget-popup-close text-fg-muted hover:text-fg text-xs p-1">' +
            '<i class="fa-solid fa-xmark"></i>' +
          '</button>' +
        '</div>' +
        '<a href="/campaigns/' + encodeURIComponent(this.campaignId) + '/calendar" ' +
           'class="text-xs text-accent hover:underline">' +
          '<i class="fa-solid fa-arrow-right mr-1"></i>Edit in Calendar' +
        '</a>';

      this.el.style.position = 'relative';
      this.el.appendChild(popup);

      popup.querySelector('.cal-widget-popup-close').addEventListener('click', function () {
        popup.remove();
      });
    },

    /**
     * Add a quick-add event button to the header area.
     */
    _addQuickAddButton: function () {
      var self = this;
      var header = this.el.querySelector('.flex.items-center.justify-between');
      if (!header) return;

      // Only show for Scribes+ (check if CSRF token exists, indicating auth).
      var csrfMeta = document.querySelector('meta[name="csrf-token"]');
      if (!csrfMeta) return;

      var btn = document.createElement('button');
      btn.className = 'text-xs text-fg-muted hover:text-accent transition-colors ml-2';
      btn.title = 'Quick add event';
      btn.innerHTML = '<i class="fa-solid fa-plus"></i>';
      btn.addEventListener('click', function (e) {
        e.preventDefault();
        self._openQuickAdd();
      });

      // Insert before the "View calendar" link.
      var viewLink = header.querySelector('a');
      if (viewLink) {
        header.insertBefore(btn, viewLink);
      } else {
        header.appendChild(btn);
      }
    },

    /**
     * Open a minimal quick-add form inline.
     */
    _openQuickAdd: function () {
      if (this._quickAddOpen) return;
      this._quickAddOpen = true;

      var self = this;
      var form = document.createElement('div');
      form.className = 'cal-widget-quickadd card p-3 mt-2 border border-accent/30';
      form.innerHTML =
        '<div class="space-y-2">' +
          '<input type="text" class="input text-sm w-full" placeholder="Event name" />' +
          '<div class="flex gap-2">' +
            '<button class="btn-primary text-xs px-3 py-1">Add</button>' +
            '<button class="btn-ghost text-xs px-3 py-1">Cancel</button>' +
          '</div>' +
        '</div>';

      // Insert after header.
      var header = this.el.querySelector('.flex.items-center.justify-between');
      if (header && header.nextSibling) {
        header.parentNode.insertBefore(form, header.nextSibling);
      } else {
        this.el.appendChild(form);
      }

      var input = form.querySelector('input');
      input.focus();

      form.querySelector('.btn-primary').addEventListener('click', function () {
        var name = input.value.trim();
        if (name) self._submitQuickAdd(name, form);
      });

      form.querySelector('.btn-ghost').addEventListener('click', function () {
        form.remove();
        self._quickAddOpen = false;
      });

      input.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') {
          var name = input.value.trim();
          if (name) self._submitQuickAdd(name, form);
        } else if (e.key === 'Escape') {
          form.remove();
          self._quickAddOpen = false;
        }
      });
    },

    /**
     * Submit the quick-add event via the calendar API.
     */
    _submitQuickAdd: function (name, formEl) {
      var self = this;
      var url = '/campaigns/' + encodeURIComponent(this.campaignId) + '/calendar/events';

      Chronicle.apiFetch(url, {
        method: 'POST',
        body: JSON.stringify({ name: name }),
      })
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          formEl.remove();
          self._quickAddOpen = false;
          // Refresh the HTMX content.
          var htmxEl = self.el.querySelector('[hx-get]');
          if (htmxEl && window.htmx) {
            htmx.trigger(htmxEl, 'intersect');
          }
        })
        .catch(function (err) {
          console.error('[calendar-widget] Quick add failed:', err);
          Chronicle.toast('Failed to create event', 'error');
        });
    },

    destroy: function (el) {
      if (this._observer) {
        this._observer.disconnect();
        this._observer = null;
      }
    }
  });
})();
