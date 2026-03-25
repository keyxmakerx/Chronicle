/**
 * Timeline Widget
 *
 * Interactive timeline embed for dashboard and page embeds. Shows timeline
 * summaries with click-to-view event detail popup. Full editing stays on
 * the dedicated timeline page.
 *
 * Mount: data-widget="timeline-widget"
 * Config:
 *   data-campaign-id  - Campaign UUID (required)
 *   data-limit        - Max timelines to show (default 5)
 */
(function () {
  'use strict';

  Chronicle.register('timeline-widget', {
    init: function (el, config) {
      this.el = el;
      this.campaignId = config.campaignId;
      this.limit = parseInt(config.limit, 10) || 5;

      this._bindTimelineClicks();
    },

    /**
     * Bind click handlers on timeline items loaded via HTMX.
     * Each timeline link gets an enhanced click that can show a quick preview.
     */
    _bindTimelineClicks: function () {
      var self = this;

      function bindItems() {
        var items = self.el.querySelectorAll('a[href*="/timelines/"]');
        items.forEach(function (item) {
          if (item._tlWidgetBound) return;
          item._tlWidgetBound = true;

          // Add a preview icon button next to each timeline link.
          var previewBtn = document.createElement('button');
          previewBtn.className = 'text-fg-muted hover:text-accent text-xs ml-1.5 shrink-0';
          previewBtn.title = 'Preview timeline';
          previewBtn.innerHTML = '<i class="fa-solid fa-eye text-[10px]"></i>';
          previewBtn.addEventListener('click', function (e) {
            e.preventDefault();
            e.stopPropagation();
            self._showTimelinePreview(item);
          });

          // Insert after the link text, inside its parent.
          if (item.parentNode) {
            item.parentNode.insertBefore(previewBtn, item.nextSibling);
          }
        });
      }

      // Initial bind + observe for HTMX swaps.
      bindItems();
      this._observer = new MutationObserver(bindItems);
      this._observer.observe(this.el, { childList: true, subtree: true });
    },

    /**
     * Show a lightweight preview popup for a timeline.
     */
    _showTimelinePreview: function (linkEl) {
      var self = this;
      var href = linkEl.getAttribute('href');
      var name = linkEl.textContent.trim();

      // Remove any existing popup.
      var existing = this.el.querySelector('.tl-widget-popup');
      if (existing) existing.remove();

      var popup = document.createElement('div');
      popup.className = 'tl-widget-popup card p-4 shadow-lg absolute z-50';
      popup.style.cssText = 'min-width:250px;top:50%;left:50%;transform:translate(-50%,-50%);';
      popup.innerHTML =
        '<div class="flex items-center justify-between mb-3">' +
          '<span class="text-sm font-semibold text-fg">' + Chronicle.escapeHtml(name) + '</span>' +
          '<button class="tl-widget-popup-close text-fg-muted hover:text-fg text-xs p-1">' +
            '<i class="fa-solid fa-xmark"></i>' +
          '</button>' +
        '</div>' +
        '<div class="tl-widget-events text-xs text-fg-muted mb-3">Loading events...</div>' +
        '<a href="' + Chronicle.escapeAttr(href) + '" ' +
           'class="text-xs text-accent hover:underline">' +
          '<i class="fa-solid fa-arrow-right mr-1"></i>Open Timeline' +
        '</a>';

      this.el.style.position = 'relative';
      this.el.appendChild(popup);

      popup.querySelector('.tl-widget-popup-close').addEventListener('click', function () {
        popup.remove();
      });

      // Fetch timeline events for preview.
      this._fetchTimelineEvents(href, popup.querySelector('.tl-widget-events'));
    },

    /**
     * Fetch timeline events and render a compact list in the popup.
     */
    _fetchTimelineEvents: function (href, container) {
      // Timeline data endpoint is at <timeline_url>/data.
      var dataUrl = href + '/data';

      Chronicle.apiFetch(dataUrl)
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          return res.json();
        })
        .then(function (data) {
          var events = data.events || [];
          if (events.length === 0) {
            container.textContent = 'No events in this timeline.';
            return;
          }

          // Show first 5 events as a compact list.
          var html = '<ul class="space-y-1">';
          var limit = Math.min(events.length, 5);
          for (var i = 0; i < limit; i++) {
            var evt = events[i];
            var name = Chronicle.escapeHtml(evt.name || 'Untitled');
            var date = evt.display_date || '';
            html += '<li class="flex items-center gap-2">' +
              '<span class="w-1.5 h-1.5 rounded-full bg-accent shrink-0"></span>' +
              '<span class="truncate">' + name + '</span>' +
              (date ? '<span class="text-fg-muted ml-auto shrink-0">' + Chronicle.escapeHtml(date) + '</span>' : '') +
            '</li>';
          }
          if (events.length > 5) {
            html += '<li class="text-fg-muted">+' + (events.length - 5) + ' more</li>';
          }
          html += '</ul>';
          container.innerHTML = html;
        })
        .catch(function () {
          container.textContent = 'Failed to load events.';
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
