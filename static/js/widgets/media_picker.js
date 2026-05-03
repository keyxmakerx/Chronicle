/**
 * Media Picker (slideout)
 *
 * Opt-in companion to the file-upload input. Operator clicks
 * "Choose from campaign" → a slideout card panel slides in from
 * the right showing every media file in the campaign as either a
 * thumbnail grid (default) or a metadata list (toggle in the panel
 * header). Click a card → fires a `media-picker:select` custom event
 * on the trigger button with `{id, url, thumbnailUrl, originalName,
 * mimeType, fileSize}`. The consuming form listens for that event and
 * sets the relevant hidden field — that way one widget serves every
 * surface that wants to pick existing media (map settings, entity
 * images, etc.) without the picker knowing what form it's plugged into.
 *
 * NOT the default — file uploads stay the primary path. The picker is
 * a discoverable secondary action.
 *
 * Mount: data-widget="media-picker" on a <button> element.
 * Config (data-* on the button):
 *   data-campaign-id   — Campaign UUID (required).
 *   data-mime-prefix   — (optional) Filter to MIME types starting with
 *                        this prefix, e.g. "image/" to hide audio.
 *   data-event-target  — (optional) Custom event name; defaults to
 *                        "media-picker:select".
 *
 * The button itself becomes the click target. The slideout is appended
 * to <body> on first open and reused thereafter.
 */
(function () {
  'use strict';

  // Singleton state shared across all picker buttons. The slideout
  // element is created once and reused (cheaper than rebuilding +
  // also avoids two pickers being open at once).
  var slideoutEl = null;
  var slideoutEscHandler = null;
  // Module-scoped state for the slideout's data + view, scoped to whichever
  // button last opened it.
  var state = {
    campaignId: '',
    mimePrefix: '',
    eventName: 'media-picker:select',
    triggerBtn: null,
    items: [],
    page: 1,
    perPage: 24,
    total: 0,
    view: 'grid',  // "grid" or "list"
    sort: 'recent', // "recent" | "name" | "size"
    loading: false,
  };

  Chronicle.register('media-picker', {
    init: function (el, config) {
      this.el = el;
      this.campaignId = config.campaignId || '';
      this.mimePrefix = config.mimePrefix || '';
      this.eventName = config.eventTarget || 'media-picker:select';

      this._onClick = this._onClick.bind(this);
      el.addEventListener('click', this._onClick);
    },

    _onClick: function (e) {
      e.preventDefault();
      e.stopPropagation();
      state.campaignId = this.campaignId;
      state.mimePrefix = this.mimePrefix;
      state.eventName = this.eventName;
      state.triggerBtn = this.el;
      state.page = 1;
      openSlideout();
      loadItems();
    },

    destroy: function () {
      if (this._onClick) this.el.removeEventListener('click', this._onClick);
    },
  });

  // ── Slideout DOM construction ─────────────────────────────────────

  /**
   * Build the slideout once and append to body. Subsequent opens just
   * re-show + reload data. Animation handled via CSS classes added
   * after a tick so the browser has a chance to render the initial
   * off-screen state.
   */
  function ensureSlideout() {
    if (slideoutEl) return slideoutEl;
    var el = document.createElement('div');
    el.className = 'fixed inset-0 z-[9999] hidden';
    el.innerHTML =
      '<div class="absolute inset-0 bg-black/40 transition-opacity duration-200" data-action="picker-close-bg" style="opacity:0"></div>' +
      '<aside class="absolute top-0 right-0 h-full w-full max-w-xl bg-surface shadow-2xl border-l border-edge flex flex-col transform transition-transform duration-200 translate-x-full">' +
        '<header class="px-4 py-3 border-b border-edge flex items-center gap-2 shrink-0">' +
          '<h2 class="text-sm font-semibold text-fg flex-1">Choose from campaign</h2>' +
          '<div class="flex items-center gap-1 border border-edge rounded-md p-0.5" role="tablist" aria-label="View mode">' +
            '<button class="text-xs px-2 py-1 rounded data-[active=true]:bg-accent data-[active=true]:text-white text-fg-secondary hover:text-fg" data-action="picker-view" data-view="grid" data-active="true" title="Grid view">' +
              '<i class="fa-solid fa-grid"></i>' +
            '</button>' +
            '<button class="text-xs px-2 py-1 rounded data-[active=true]:bg-accent data-[active=true]:text-white text-fg-secondary hover:text-fg" data-action="picker-view" data-view="list" title="List view">' +
              '<i class="fa-solid fa-list"></i>' +
            '</button>' +
          '</div>' +
          '<select class="text-xs px-2 py-1 border border-edge rounded bg-surface text-fg" data-action="picker-sort" title="Sort">' +
            '<option value="recent">Newest first</option>' +
            '<option value="name">Name (A→Z)</option>' +
            '<option value="size">Size (largest first)</option>' +
          '</select>' +
          '<button class="ml-1 text-fg-muted hover:text-fg p-1" data-action="picker-close" title="Close">' +
            '<i class="fa-solid fa-xmark"></i>' +
          '</button>' +
        '</header>' +
        '<div class="flex-1 overflow-y-auto" data-picker-body></div>' +
        '<footer class="px-4 py-2 border-t border-edge text-[11px] text-fg-muted shrink-0" data-picker-footer></footer>' +
      '</aside>';
    document.body.appendChild(el);

    // Backdrop + close button + Escape key all close the panel.
    el.addEventListener('click', function (e) {
      var t = e.target.closest('[data-action]');
      if (!t) return;
      var action = t.getAttribute('data-action');
      if (action === 'picker-close' || action === 'picker-close-bg') {
        closeSlideout();
      } else if (action === 'picker-view') {
        state.view = t.getAttribute('data-view');
        // Update the active state on both buttons.
        el.querySelectorAll('[data-action="picker-view"]').forEach(function (b) {
          b.setAttribute('data-active', b.getAttribute('data-view') === state.view ? 'true' : 'false');
        });
        renderBody();
      } else if (action === 'picker-pick') {
        var id = t.getAttribute('data-id');
        pickItem(id);
      }
    });

    el.querySelector('[data-action="picker-sort"]').addEventListener('change', function (ev) {
      state.sort = ev.target.value;
      renderBody();
    });

    slideoutEl = el;
    return el;
  }

  function openSlideout() {
    var el = ensureSlideout();
    el.classList.remove('hidden');
    // Defer the animated-in classes to the next frame so the browser
    // applies the initial translate-x-full / opacity-0 state first.
    requestAnimationFrame(function () {
      el.querySelector('aside').classList.remove('translate-x-full');
      el.querySelector('[data-action="picker-close-bg"]').style.opacity = '1';
    });
    // Esc-to-close.
    slideoutEscHandler = function (e) {
      if (e.key === 'Escape') closeSlideout();
    };
    document.addEventListener('keydown', slideoutEscHandler);
  }

  function closeSlideout() {
    if (!slideoutEl) return;
    slideoutEl.querySelector('aside').classList.add('translate-x-full');
    slideoutEl.querySelector('[data-action="picker-close-bg"]').style.opacity = '0';
    // Hide after the animation finishes (matches duration-200).
    setTimeout(function () {
      if (slideoutEl) slideoutEl.classList.add('hidden');
    }, 200);
    if (slideoutEscHandler) {
      document.removeEventListener('keydown', slideoutEscHandler);
      slideoutEscHandler = null;
    }
  }

  // ── Data + render ────────────────────────────────────────────────

  function loadItems() {
    var el = ensureSlideout();
    var body = el.querySelector('[data-picker-body]');
    state.loading = true;
    body.innerHTML = '<div class="p-8 text-center text-sm text-fg-muted">Loading…</div>';

    var url = '/campaigns/' + encodeURIComponent(state.campaignId) +
      '/media/list?page=' + state.page + '&perPage=' + state.perPage;
    Chronicle.apiFetch(url)
      .then(function (res) {
        if (!res.ok) {
          return res.json().then(
            function (b) { throw new Error((b && b.message) || ('HTTP ' + res.status)); },
            function () { throw new Error('HTTP ' + res.status); }
          );
        }
        return res.json();
      })
      .then(function (data) {
        state.loading = false;
        var items = (data && data.items) || [];
        if (state.mimePrefix) {
          items = items.filter(function (it) { return (it.mime_type || '').indexOf(state.mimePrefix) === 0; });
        }
        state.items = items;
        state.total = data.total || 0;
        renderBody();
      })
      .catch(function (err) {
        state.loading = false;
        body.innerHTML =
          '<div class="p-8 text-center text-sm text-red-500">Failed to load: ' +
          Chronicle.escapeHtml(err.message || 'unknown error') + '</div>';
      });
  }

  function renderBody() {
    var el = ensureSlideout();
    var body = el.querySelector('[data-picker-body]');
    var footer = el.querySelector('[data-picker-footer]');

    if (state.loading) return;

    var items = state.items.slice();
    items.sort(sortComparator(state.sort));

    if (items.length === 0) {
      body.innerHTML =
        '<div class="p-8 text-center">' +
        '<i class="fa-solid fa-image text-3xl text-fg-muted mb-2"></i>' +
        '<p class="text-sm text-fg-muted">No matching media in this campaign.</p>' +
        '</div>';
      footer.textContent = '';
      return;
    }

    if (state.view === 'list') {
      body.innerHTML = renderList(items);
    } else {
      body.innerHTML = renderGrid(items);
    }
    footer.textContent = items.length + ' of ' + state.total + ' file' + (state.total === 1 ? '' : 's');
  }

  function renderGrid(items) {
    var html = '<div class="grid grid-cols-2 sm:grid-cols-3 gap-2 p-3">';
    for (var i = 0; i < items.length; i++) {
      var it = items[i];
      html +=
        '<button type="button" class="card p-0 text-left hover:ring-2 hover:ring-accent/50 transition-all overflow-hidden group" data-action="picker-pick" data-id="' + Chronicle.escapeAttr(it.id) + '" title="' + Chronicle.escapeAttr(it.original_name) + '">';
      if (it.thumbnail_url || it.url) {
        html += '<div class="aspect-video bg-surface-alt overflow-hidden">';
        html += '<img src="' + Chronicle.escapeAttr(it.thumbnail_url || it.url) + '" alt="' + Chronicle.escapeAttr(it.original_name) + '" class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200" loading="lazy"/>';
        html += '</div>';
      } else {
        html += '<div class="aspect-video bg-surface-alt flex items-center justify-center"><i class="fa-solid fa-file text-2xl text-fg-muted"></i></div>';
      }
      html += '<div class="p-2 text-xs text-fg truncate">' + Chronicle.escapeHtml(it.original_name) + '</div>';
      html += '</button>';
    }
    html += '</div>';
    return html;
  }

  function renderList(items) {
    var html = '<ul class="divide-y divide-edge">';
    for (var i = 0; i < items.length; i++) {
      var it = items[i];
      var thumb = it.thumbnail_url || it.url || '';
      html +=
        '<li><button type="button" class="w-full px-3 py-2 flex items-center gap-3 text-left hover:bg-surface-alt transition-colors" data-action="picker-pick" data-id="' + Chronicle.escapeAttr(it.id) + '" title="' + Chronicle.escapeAttr(it.original_name) + '">';
      if (thumb) {
        html += '<img src="' + Chronicle.escapeAttr(thumb) + '" alt="" class="w-12 h-12 object-cover rounded shrink-0" loading="lazy"/>';
      } else {
        html += '<div class="w-12 h-12 rounded bg-surface-alt flex items-center justify-center shrink-0"><i class="fa-solid fa-file text-fg-muted"></i></div>';
      }
      html += '<div class="flex-1 min-w-0">';
      html += '<div class="text-sm text-fg truncate">' + Chronicle.escapeHtml(it.original_name) + '</div>';
      html += '<div class="text-[11px] text-fg-muted">' + Chronicle.escapeHtml(it.mime_type) + ' &middot; ' + humanBytes(it.file_size) + ' &middot; ' + relTime(it.created_at) + '</div>';
      html += '</div>';
      html += '</button></li>';
    }
    html += '</ul>';
    return html;
  }

  function pickItem(id) {
    var item = null;
    for (var i = 0; i < state.items.length; i++) {
      if (state.items[i].id === id) { item = state.items[i]; break; }
    }
    if (!item || !state.triggerBtn) {
      closeSlideout();
      return;
    }
    var ev = new CustomEvent(state.eventName, {
      bubbles: true,
      detail: {
        id: item.id,
        url: item.url,
        thumbnailUrl: item.thumbnail_url || '',
        originalName: item.original_name,
        mimeType: item.mime_type,
        fileSize: item.file_size,
      },
    });
    state.triggerBtn.dispatchEvent(ev);
    closeSlideout();
  }

  // ── Helpers ───────────────────────────────────────────────────────

  function sortComparator(mode) {
    if (mode === 'name') {
      return function (a, b) { return (a.original_name || '').localeCompare(b.original_name || ''); };
    }
    if (mode === 'size') {
      return function (a, b) { return (b.file_size || 0) - (a.file_size || 0); };
    }
    // 'recent' default
    return function (a, b) {
      return (new Date(b.created_at || 0)) - (new Date(a.created_at || 0));
    };
  }

  function humanBytes(n) {
    if (!n) return '0 B';
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1024 * 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + ' MB';
    return (n / (1024 * 1024 * 1024)).toFixed(1) + ' GB';
  }

  function relTime(iso) {
    if (!iso) return '';
    var d = new Date(iso);
    var diff = (Date.now() - d.getTime()) / 1000;
    if (diff < 60) return 'just now';
    if (diff < 3600) return Math.floor(diff / 60) + 'm ago';
    if (diff < 86400) return Math.floor(diff / 3600) + 'h ago';
    if (diff < 86400 * 30) return Math.floor(diff / 86400) + 'd ago';
    return d.toLocaleDateString();
  }
})();
