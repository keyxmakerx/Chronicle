/**
 * search_page.js -- the Alpine component of the campaign search page
 * (searchPageData): live results as the query, type or tags change.
 *
 * Mount: <div data-widget="search-page" x-data x-ignore> around the page in
 * SearchPage (search_page.templ). A page <script> would be stripped by htmx
 * on a boosted swap, so this loads on sight instead; x-ignore holds Alpine
 * off until the component exists, and init hands the page to Alpine.
 */
(function () {
  // The size of a page of results as SearchAPI answers it. The server ignores
  // a per_page request, so paging counts in this and asks for nothing else.
  var PER_PAGE = 20;

  // resultUrl is where a result opens: its own url when that is a same-site
  // path (maps, timelines, events and rules are not pages), else the page
  // route. Same rule as topbar_search.js, so both screens open the same thing.
  function resultUrl(e, campaignId) {
    var u = typeof e.url === 'string' ? e.url : '';
    if (u.charAt(0) === '/' && u.charAt(1) !== '/' && u.charAt(1) !== '\\') return u;
    return '/campaigns/' + encodeURIComponent(campaignId) + '/entities/' + encodeURIComponent(e.id);
  }

  function searchPageData(campaignId) {
    return {
      query: new URLSearchParams(window.location.search).get('q') || '',
      typeFilter: '',
      activeTags: [],
      allTags: [],
      results: [],
      total: 0,
      // How many of total are pages, which is what the pager steps through;
      // total also holds the hits that are not pages and are not paged.
      entityTotal: 0,
      // The non-page hits the first page reported, kept so the count line
      // does not change when a later page (which omits them) is read.
      extras: 0,
      page: 1,
      loading: false,
      debounceTimer: null,

      init: function () {
        var self = this;
        // Load campaign tags for the filter chips.
        Chronicle.apiFetch('/campaigns/' + campaignId + '/tags')
          .then(function (r) { return r.json(); })
          .then(function (tags) { self.allTags = tags || []; })
          .catch(function () {});
        if (self.query) self.search();
        self.$watch('query', function () { self.debouncedSearch(); });
        self.$watch('typeFilter', function () { self.page = 1; self.search(); });
      },

      toggleTag: function (slug) {
        var idx = this.activeTags.indexOf(slug);
        if (idx >= 0) this.activeTags.splice(idx, 1);
        else this.activeTags.push(slug);
        this.page = 1;
        this.search();
      },

      clearTags: function () {
        this.activeTags = [];
        this.page = 1;
        this.search();
      },

      debouncedSearch: function () {
        var self = this;
        clearTimeout(self.debounceTimer);
        self.debounceTimer = setTimeout(function () { self.page = 1; self.search(); }, 250);
      },

      search: function () {
        var self = this;
        if (self.query.length > 0 && self.query.length < 2) return;
        self.loading = true;
        var url = '/campaigns/' + campaignId + '/entities/search?page=' + self.page;
        if (self.query) url += '&q=' + encodeURIComponent(self.query);
        if (self.typeFilter) url += '&type=' + self.typeFilter;
        if (self.activeTags.length > 0) url += '&tags=' + self.activeTags.join(',');
        Chronicle.apiFetch(url)
          .then(function (r) { return r.json(); })
          .then(function (data) {
            self.results = data.results || [];
            self.entityTotal = typeof data.entity_total === 'number' ? data.entity_total : (data.total || 0);
            if (self.page === 1) self.extras = Math.max(0, (data.total || 0) - self.entityTotal);
            self.total = self.entityTotal + self.extras;
            self.loading = false;
            self.renderGrid();
            // Update URL without reload.
            var params = new URLSearchParams();
            if (self.query) params.set('q', self.query);
            if (self.typeFilter) params.set('type', self.typeFilter);
            history.replaceState(null, '', window.location.pathname + (params.toString() ? '?' + params.toString() : ''));
          })
          .catch(function () {
            self.results = [];
            self.total = 0;
            self.entityTotal = 0;
            self.extras = 0;
            self.loading = false;
            self.renderGrid();
          });
      },

      renderGrid: function () {
        var grid = document.getElementById('search-results-grid');
        if (!grid) return;
        if (this.results.length === 0) { grid.innerHTML = ''; return; }
        var html = '';
        for (var i = 0; i < this.results.length; i++) {
          var e = this.results[i];
          var color = Chronicle.escapeAttr(e.type_color || '#6b7280');
          var icon = Chronicle.escapeAttr(e.type_icon || 'fa-file');
          html += '<a href="' + Chronicle.escapeAttr(resultUrl(e, campaignId)) + '" class="card hover:border-accent/30 hover:shadow-sm transition-all group">' +
            '<div class="flex items-start gap-3">' +
            '<div class="w-8 h-8 rounded-lg flex items-center justify-center shrink-0 text-xs" style="background-color:' + color + '20;color:' + color + '">' +
            '<i class="fa-solid ' + icon + '"></i></div>' +
            '<div class="min-w-0 flex-1">' +
            '<div class="text-sm font-medium text-fg group-hover:text-accent transition-colors truncate">' + Chronicle.escapeHtml(e.name) + '</div>' +
            '<div class="text-xs text-fg-muted mt-0.5">' + Chronicle.escapeHtml(e.type_name || '') + '</div>' +
            '</div>' +
            (e.restricted === 'dm_only' ? '<i class="fa-solid fa-lock text-[10px] text-fg-muted mt-1 shrink-0" title="Hidden from players"></i>' : '') +
            '</div></a>';
        }
        grid.innerHTML = html;
      },

      // The count line: one result is "1 result", never "1 results".
      resultsLabel: function () {
        return this.total === 1 ? '1 result' : this.total + ' results';
      },

      // The pager shows when the pages run past one answer, and Next stops
      // at the last of them.
      hasPager: function () { return this.entityTotal > PER_PAGE; },
      hasNext: function () { return this.page * PER_PAGE < this.entityTotal; },

      nextPage: function () { this.page++; this.search(); },
      prevPage: function () { if (this.page > 1) { this.page--; this.search(); } }
    };
  }

  // Alpine evaluates x-data="searchPageData(...)" by name.
  window.searchPageData = searchPageData;

  Chronicle.register('search-page', {
    init: function (el) { Chronicle.startAlpine(el); },
  });
})();
