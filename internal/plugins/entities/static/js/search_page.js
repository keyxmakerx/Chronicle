/**
 * search_page.js -- the Alpine component of the campaign search page
 * (searchPageData): two groups of results that follow the words, category and
 * tags as they change. The first group is what has the words in its name, the
 * second is every page that mentions them, each with its line.
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
  // A group shows this many rows until "Show all" is pressed; the rest of the
  // page it fetched is already in hand, so opening it asks for nothing.
  var PREVIEW = 5;
  // A tag list this long gets a find field of its own in its picker.
  var TAG_FIND_FROM = 9;

  // resultUrl is where a result opens: its own url when that is a same-site
  // path (maps, timelines, events and rules are not pages), else the page
  // route. Same rule as topbar_search.js, so both screens open the same thing.
  function resultUrl(e, campaignId) {
    var u = typeof e.url === 'string' ? e.url : '';
    if (u.charAt(0) === '/' && u.charAt(1) !== '/' && u.charAt(1) !== '\\') return u;
    return '/campaigns/' + encodeURIComponent(campaignId) + '/entities/' + encodeURIComponent(e.id);
  }

  // sameSitePath keeps a picture's address only when it is a path on this
  // site, the same test the links use.
  function sameSitePath(u) {
    if (typeof u !== 'string') return '';
    return u.charAt(0) === '/' && u.charAt(1) !== '/' && u.charAt(1) !== '\\' ? u : '';
  }

  // kindWord is what the thing is, as a quiet word: its category and, when it
  // has one, its descriptor. A game night is written that way everywhere.
  function kindWord(e) {
    var name = e.type_name === 'Session' ? 'Game night' : (e.type_name || '');
    return e.type_label ? (name ? name + ' · ' + e.type_label : e.type_label) : name;
  }

  // snippetHtml draws the quoted line. Each run arrives as text with a mark
  // flag, never as markup: every run is escaped here and only the marked ones
  // are wrapped, so nothing in a page's words can become HTML.
  function snippetHtml(parts) {
    var html = '';
    for (var i = 0; i < parts.length; i++) {
      var p = parts[i] || {};
      var text = Chronicle.escapeHtml(String(p.text == null ? '' : p.text));
      html += p.mark ? '<mark class="sp-mark">' + text + '</mark>' : text;
    }
    return html;
  }

  // rowHtml draws one result: the name first, the amber lock after it for a
  // page players can't see, what it is at the row's end, the page's own
  // picture last, and under the name the line that holds the words.
  function rowHtml(e, campaignId) {
    var hidden = e.restricted === 'dm_only';
    var pic = sameSitePath(e.image);
    var lockLabel = 'Hidden from players';
    var line = Array.isArray(e.snippet) && e.snippet.length ? snippetHtml(e.snippet) : '';
    return '<a href="' + Chronicle.escapeAttr(resultUrl(e, campaignId)) + '" class="sp-row cb-link"' + (hidden ? ' data-dm-only' : '') + '>' +
      '<span class="sp-name">' + Chronicle.escapeHtml(e.name) +
        (hidden ? '<i class="cb-dm fa-solid fa-lock sp-lock" role="img" aria-label="' + lockLabel + '" title="' + lockLabel + '"></i>' : '') + '</span>' +
      '<span class="sp-kind">' + Chronicle.escapeHtml(kindWord(e)) + '</span>' +
      (pic ? '<img class="sp-pic" src="' + Chronicle.escapeAttr(pic) + '" alt="" loading="lazy">' : '<span class="sp-nopic"></span>') +
      (line ? '<span class="sp-line">' + line + '</span>' : '') +
      '</a>';
  }

  function newGroup() {
    // entityTotal is how many of the group are pages, which is what its pager
    // steps through; the group's total also holds the hits that are not pages
    // and are not paged. extras is those, kept from the first page so the
    // count does not change when a later page (which omits them) is read.
    return { items: [], entityTotal: 0, extras: 0, total: 0, page: 1, open: false };
  }

  function searchPageData(campaignId, canCreate) {
    var params = new URLSearchParams(window.location.search);
    return {
      query: params.get('q') || '',
      typeFilter: params.get('type') || '',
      activeTags: (params.get('tags') || '').split(',').filter(Boolean),
      allTags: [],
      tagFind: '',
      typeNames: {},
      openPicker: '',
      groups: { named: newGroup(), mention: newGroup() },
      loading: false,
      // True once an answer has come back, so "nothing found" is not said
      // before the first search has run.
      settled: false,
      canCreate: !!canCreate,
      debounceTimer: null,
      // Each search takes a number; an answer from an older one is dropped,
      // so typing quickly never shows the earlier word's results.
      seq: 0,

      init: function () {
        var self = this;
        var opts = self.$root ? self.$root.querySelectorAll('[data-type-id]') : [];
        for (var i = 0; i < opts.length; i++) {
          self.typeNames[opts[i].getAttribute('data-type-id')] = opts[i].textContent.trim();
        }
        Chronicle.apiFetch('/campaigns/' + campaignId + '/tags')
          .then(function (r) { return r.json(); })
          .then(function (tags) { self.allTags = Array.isArray(tags) ? tags : []; })
          .catch(function () {});
        if (self.hasQuery() || self.hasFilters()) self.search();
        self.$watch('query', function () { self.debouncedSearch(); });
      },

      // A search needs two letters, as quick search does.
      hasQuery: function () { return this.query.trim().length >= 2; },
      hasFilters: function () { return !!this.typeFilter || this.activeTags.length > 0; },

      get total() { return this.groups.named.total + this.groups.mention.total; },

      // --- The pickers ---

      togglePicker: function (name) { this.openPicker = this.openPicker === name ? '' : name; this.tagFind = ''; },
      closePickers: function () { this.openPicker = ''; },
      // A click outside one picker closes only that picker: the click that
      // opens the other must not be undone by this one's outside handler.
      closeIf: function (name) { if (this.openPicker === name) this.openPicker = ''; },

      typeLabel: function () {
        return this.typeFilter ? (this.typeNames[this.typeFilter] || 'Category') : 'All categories';
      },

      pickType: function (id) {
        this.typeFilter = id;
        this.openPicker = '';
        this.restart();
      },

      tagName: function (slug) {
        for (var i = 0; i < this.allTags.length; i++) {
          if (this.allTags[i].slug === slug) return this.allTags[i].name;
        }
        return slug;
      },

      tagLabel: function () {
        var n = this.activeTags.length;
        if (n === 0) return 'Any tag';
        return n === 1 ? this.tagName(this.activeTags[0]) : n + ' tags';
      },

      // The find field only exists for a long list; a short one is shown whole.
      visibleTags: function () {
        var q = this.tagFind.trim().toLowerCase();
        if (!q || this.allTags.length < TAG_FIND_FROM) return this.allTags;
        return this.allTags.filter(function (t) { return String(t.name).toLowerCase().indexOf(q) !== -1; });
      },

      toggleTag: function (slug) {
        var idx = this.activeTags.indexOf(slug);
        if (idx >= 0) this.activeTags.splice(idx, 1);
        else this.activeTags.push(slug);
        this.restart();
      },

      clearTags: function () {
        this.activeTags = [];
        this.openPicker = '';
        this.restart();
      },

      // --- Searching ---

      debouncedSearch: function () {
        var self = this;
        clearTimeout(self.debounceTimer);
        self.debounceTimer = setTimeout(function () { self.restart(); }, 250);
      },

      // restart begins a fresh search: both groups go back to their first
      // page, closed.
      restart: function () {
        this.groups.named = newGroup();
        this.groups.mention = newGroup();
        this.search();
      },

      search: function () {
        var self = this;
        var token = ++self.seq;
        self.syncUrl();
        // A word is needed to find where something comes up; with none, only
        // the category and tags can narrow a plain list of pages.
        if (!self.hasQuery() && !self.hasFilters()) {
          self.groups.named = newGroup();
          self.groups.mention = newGroup();
          self.loading = false;
          self.settled = false;
          self.renderAll();
          return;
        }
        self.loading = true;
        var asks = [self.fetchGroup('named', token)];
        if (self.hasQuery()) asks.push(self.fetchGroup('mention', token));
        else self.groups.mention = newGroup();
        Promise.all(asks).then(function () {
          if (token !== self.seq) return;
          self.loading = false;
          self.settled = true;
          self.renderAll();
        });
      },

      groupUrl: function (name) {
        var g = this.groups[name];
        var url = '/campaigns/' + campaignId + '/entities/search?scope=' + name + '&page=' + g.page;
        if (this.hasQuery()) url += '&q=' + encodeURIComponent(this.query.trim());
        if (this.typeFilter) url += '&type=' + encodeURIComponent(this.typeFilter);
        if (this.activeTags.length > 0) url += '&tags=' + this.activeTags.map(encodeURIComponent).join(',');
        return url;
      },

      fetchGroup: function (name, token) {
        var self = this;
        var g = self.groups[name];
        return Chronicle.apiFetch(self.groupUrl(name))
          .then(function (r) { return r.json(); })
          .then(function (data) {
            if (token !== self.seq) return;
            g.items = data.results || [];
            g.entityTotal = typeof data.entity_total === 'number' ? data.entity_total : (data.total || 0);
            if (g.page === 1) g.extras = Math.max(0, (data.total || 0) - g.entityTotal);
            g.total = g.entityTotal + g.extras;
          })
          .catch(function () {
            if (token !== self.seq) return;
            g.items = [];
            g.entityTotal = 0;
            g.extras = 0;
            g.total = 0;
          });
      },

      syncUrl: function () {
        var p = new URLSearchParams();
        if (this.query) p.set('q', this.query);
        if (this.typeFilter) p.set('type', this.typeFilter);
        if (this.activeTags.length > 0) p.set('tags', this.activeTags.join(','));
        try {
          history.replaceState(null, '', window.location.pathname + (p.toString() ? '?' + p.toString() : ''));
        } catch (e) { /* a sandboxed frame; the address just stays */ }
      },

      // --- Drawing ---

      renderAll: function () {
        this.renderGroup('named');
        this.renderGroup('mention');
      },

      renderGroup: function (name) {
        var el = document.getElementById('sp-' + name);
        if (!el) return;
        var g = this.groups[name];
        var items = g.open ? g.items : g.items.slice(0, PREVIEW);
        var html = '';
        for (var i = 0; i < items.length; i++) html += rowHtml(items[i], campaignId);
        el.innerHTML = html;
      },

      // "Show all N" appears while a group is folded and has more than it shows.
      moreFor: function (name) {
        var g = this.groups[name];
        return !g.open && g.total > PREVIEW;
      },

      showAll: function (name) {
        this.groups[name].open = true;
        this.renderGroup(name);
      },

      pagesOf: function (name) {
        return Math.max(1, Math.ceil(this.groups[name].entityTotal / PER_PAGE));
      },

      // turn steps one group to its next or previous page, leaving the other
      // group where it is.
      turn: function (name, by) {
        var self = this;
        var g = self.groups[name];
        var next = g.page + by;
        if (next < 1 || next > self.pagesOf(name)) return;
        g.page = next;
        var token = ++self.seq;
        self.loading = true;
        self.fetchGroup(name, token).then(function () {
          if (token !== self.seq) return;
          self.loading = false;
          self.renderGroup(name);
        });
      },

      namedHeading: function () {
        return this.hasQuery() ? 'Named “' + this.query.trim() + '”' : 'Pages';
      },

      // The count line: one result is "1 result", never "1 results".
      resultsLabel: function () {
        return this.total === 1 ? '1 result' : this.total + ' results';
      },

      showEmpty: function () {
        return this.settled && !this.loading && this.total === 0 && (this.hasQuery() || this.hasFilters());
      },

      emptyText: function () {
        if (!this.hasQuery()) return 'No page is in that category with those tags.';
        return 'Nothing matches “' + this.query.trim() + '”. Try fewer or different words.';
      },

      createHref: function () {
        return '/campaigns/' + encodeURIComponent(campaignId) + '/entities/new?name=' + encodeURIComponent(this.query.trim());
      }
    };
  }

  // Alpine evaluates x-data="searchPageData(...)" by name.
  window.searchPageData = searchPageData;

  Chronicle.register('search-page', {
    init: function (el) { Chronicle.startAlpine(el); },
  });
})();
