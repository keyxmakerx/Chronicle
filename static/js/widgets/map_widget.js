/**
 * Map Widget
 *
 * Embedded Leaflet map viewer for dashboard and page embeds. Shows a map
 * with its markers in a compact, read-only view. Clicking markers shows
 * tooltip popups. Full editing stays on the dedicated map page.
 *
 * Each widget instance fetches its own data independently — multiple
 * instances of the same map on different pages have no shared state.
 * The source of truth is always the database.
 *
 * Mount: data-widget="map-widget"
 * Config:
 *   data-campaign-id  - Campaign UUID (required)
 *   data-map-id       - Specific map UUID (optional; shows map picker if empty)
 */
(function () {
  'use strict';

  Chronicle.register('map-widget', {
    init: function (el, config) {
      this.el = el;
      this.campaignId = config.campaignId;
      this.mapId = config.mapId;
      this._leafletMap = null;

      if (this.mapId) {
        this._loadMap(this.mapId);
      } else {
        this._loadMapPicker();
      }
    },

    /**
     * Load a specific map and render it in the widget.
     */
    _loadMap: function (mapId) {
      var self = this;
      var url = '/campaigns/' + encodeURIComponent(this.campaignId) + '/maps/' + encodeURIComponent(mapId);

      // Fetch map data as JSON via HX-Request header to get fragment, but
      // we actually need the raw map data. Use the API if available, else
      // parse from the page. For simplicity, fetch the map list and find ours.
      var listUrl = '/campaigns/' + encodeURIComponent(this.campaignId) + '/maps/' + encodeURIComponent(mapId);

      // We need the map's image and markers. Fetch the show page as HTMX
      // fragment to extract the JS data, or better yet, use a direct approach.
      // The map show page embeds data in script tags. Instead, let's fetch
      // map data from the markers API directly.
      this._fetchMapData(mapId);
    },

    /**
     * Fetch map metadata and markers, then render Leaflet.
     */
    _fetchMapData: function (mapId) {
      var self = this;
      var baseUrl = '/campaigns/' + encodeURIComponent(this.campaignId) + '/maps/' + encodeURIComponent(mapId);

      // Fetch the map show page with HX-Request to get the fragment.
      // The fragment contains all the data we need in script tags.
      // Alternative: use the map list API to get basic info.
      Chronicle.apiFetch(baseUrl, {
        headers: { 'Accept': 'application/json' }
      })
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          return res.text();
        })
        .then(function (html) {
          // Parse map data from the response. The show page template embeds
          // JSON in script variables. Extract what we need.
          self._renderFromHTML(html, mapId);
        })
        .catch(function (err) {
          console.error('[map-widget] Failed to load map:', err);
          self._renderError('Failed to load map');
        });
    },

    /**
     * Parse map data from the HTMX response HTML and render Leaflet.
     */
    _renderFromHTML: function (html, mapId) {
      var self = this;

      // Extract mapImageURL, imageW, imageH, and markers from the script.
      var imgMatch = html.match(/var mapImageURL\s*=\s*"([^"]+)"/);
      var wMatch = html.match(/var imageW\s*=\s*"(\d+)"/);
      var hMatch = html.match(/var imageH\s*=\s*"(\d+)"/);
      var markersMatch = html.match(/var markers\s*=\s*(\[[\s\S]*?\]);/);

      if (!imgMatch || !wMatch || !hMatch) {
        // No image — show placeholder.
        this._renderEmpty(mapId);
        return;
      }

      var imageUrl = imgMatch[1];
      var imageW = parseInt(wMatch[1], 10);
      var imageH = parseInt(hMatch[1], 10);
      var markers = [];

      if (markersMatch) {
        try {
          markers = JSON.parse(markersMatch[1]);
        } catch (e) {
          // Ignore parse errors.
        }
      }

      this._renderLeaflet(imageUrl, imageW, imageH, markers, mapId);
    },

    /**
     * Initialize Leaflet map in the widget container.
     */
    _renderLeaflet: function (imageUrl, imageW, imageH, markers, mapId) {
      var self = this;
      var container = this.el.querySelector('.card') || this.el;

      // Clear loading text.
      container.innerHTML = '';
      container.style.minHeight = '250px';
      container.style.position = 'relative';

      // Create map div.
      var mapDiv = document.createElement('div');
      mapDiv.style.cssText = 'width:100%;height:250px;';
      container.appendChild(mapDiv);

      // Check if Leaflet is loaded.
      if (typeof L === 'undefined') {
        this._loadLeafletCSS();
        this._loadLeafletJS(function () {
          self._initLeaflet(mapDiv, imageUrl, imageW, imageH, markers, mapId);
        });
        return;
      }

      this._initLeaflet(mapDiv, imageUrl, imageW, imageH, markers, mapId);
    },

    /**
     * Actually create the Leaflet map instance.
     */
    _initLeaflet: function (mapDiv, imageUrl, imageW, imageH, markers, mapId) {
      var self = this;
      var bounds = [[0, 0], [imageH, imageW]];

      var map = L.map(mapDiv, {
        crs: L.CRS.Simple,
        minZoom: -3,
        maxZoom: 3,
        zoomSnap: 0.25,
        zoomControl: false, // Compact view — hide zoom controls.
        attributionControl: false,
        dragging: true,
        scrollWheelZoom: true,
      });

      this._leafletMap = map;

      if (imageUrl) {
        L.imageOverlay(imageUrl, bounds).addTo(map);
      }
      map.fitBounds(bounds);

      // Add markers.
      markers.forEach(function (mk) {
        var icon = L.divIcon({
          className: 'chronicle-marker',
          html: '<div style="color:' + Chronicle.escapeHtml(mk.color || '#ef4444') +
            ';font-size:16px;text-shadow:0 1px 2px rgba(0,0,0,0.4);">' +
            '<i class="fa-solid ' + Chronicle.escapeHtml(mk.icon || 'fa-map-pin') + '"></i></div>',
          iconSize: [20, 20],
          iconAnchor: [10, 10],
        });

        var marker = L.marker([mk.y, mk.x], { icon: icon }).addTo(map);

        var popupContent = '<div class="text-xs">' +
          '<strong>' + Chronicle.escapeHtml(mk.name) + '</strong>';
        if (mk.description) {
          popupContent += '<br/>' + Chronicle.escapeHtml(mk.description);
        }
        popupContent += '</div>';
        marker.bindPopup(popupContent, { maxWidth: 200 });
      });

      // Add "Open map" overlay link.
      var overlay = document.createElement('a');
      overlay.href = '/campaigns/' + encodeURIComponent(this.campaignId) +
        '/maps/' + encodeURIComponent(mapId);
      overlay.className = 'absolute bottom-2 right-2 z-[1000] bg-surface/90 text-xs ' +
        'text-accent hover:underline px-2 py-1 rounded shadow';
      overlay.innerHTML = '<i class="fa-solid fa-expand mr-1"></i>Full map';
      mapDiv.style.position = 'relative';
      mapDiv.appendChild(overlay);

      // Invalidate size after render.
      setTimeout(function () { map.invalidateSize(); }, 100);
    },

    /**
     * Render empty state when no map image is set.
     */
    _renderEmpty: function (mapId) {
      var container = this.el.querySelector('.card') || this.el;
      container.innerHTML =
        '<div class="flex flex-col items-center justify-center py-8 text-fg-muted">' +
          '<i class="fa-solid fa-map text-2xl mb-2"></i>' +
          '<p class="text-sm">No map image uploaded yet.</p>' +
          '<a href="/campaigns/' + encodeURIComponent(this.campaignId) +
            '/maps/' + encodeURIComponent(mapId) + '" ' +
            'class="text-xs text-accent hover:underline mt-1">' +
            'Open map editor' +
          '</a>' +
        '</div>';
    },

    /**
     * Render error state.
     */
    _renderError: function (msg) {
      var container = this.el.querySelector('.card') || this.el;
      container.innerHTML =
        '<div class="flex flex-col items-center justify-center py-8 text-fg-muted">' +
          '<i class="fa-solid fa-triangle-exclamation text-2xl mb-2"></i>' +
          '<p class="text-sm">' + Chronicle.escapeHtml(msg) + '</p>' +
        '</div>';
    },

    /**
     * Show a map picker when no map_id is configured.
     */
    _loadMapPicker: function () {
      var self = this;
      var container = this.el.querySelector('.card') || this.el;
      var url = '/campaigns/' + encodeURIComponent(this.campaignId) + '/maps';

      // Fetch map list to show a picker.
      Chronicle.apiFetch(url, {
        headers: { 'Accept': 'application/json' }
      })
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          return res.text();
        })
        .then(function (html) {
          // For now, show a message linking to the maps page.
          container.innerHTML =
            '<div class="flex flex-col items-center justify-center py-8 text-fg-muted">' +
              '<i class="fa-solid fa-map text-2xl mb-2"></i>' +
              '<p class="text-sm mb-2">Configure a map for this block.</p>' +
              '<a href="/campaigns/' + encodeURIComponent(self.campaignId) + '/maps" ' +
                'class="text-xs text-accent hover:underline">' +
                'View all maps' +
              '</a>' +
            '</div>';
        })
        .catch(function () {
          self._renderError('Failed to load maps');
        });
    },

    /**
     * Dynamically load Leaflet CSS if not already present.
     */
    _loadLeafletCSS: function () {
      if (document.querySelector('link[href*="leaflet"]')) return;
      var link = document.createElement('link');
      link.rel = 'stylesheet';
      link.href = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.css';
      link.integrity = 'sha256-p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=';
      link.crossOrigin = '';
      document.head.appendChild(link);
    },

    /**
     * Dynamically load Leaflet JS if not already present.
     */
    _loadLeafletJS: function (callback) {
      if (typeof L !== 'undefined') {
        callback();
        return;
      }
      if (document.querySelector('script[src*="leaflet"]')) {
        // Script tag exists but L not yet loaded — wait for it.
        var interval = setInterval(function () {
          if (typeof L !== 'undefined') {
            clearInterval(interval);
            callback();
          }
        }, 50);
        return;
      }
      var script = document.createElement('script');
      script.src = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.js';
      script.integrity = 'sha256-20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV1lvTlZBo=';
      script.crossOrigin = '';
      script.onload = callback;
      document.head.appendChild(script);
    },

    destroy: function (el) {
      if (this._leafletMap) {
        this._leafletMap.remove();
        this._leafletMap = null;
      }
    }
  });
})();
