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
      this.showDrawings = config.showDrawings === 'true';
      this.showTokens = config.showTokens === 'true';
      this.customHeight = parseInt(config.height, 10) || 0;
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
      var height = self.customHeight || 250;
      container.style.minHeight = height + 'px';
      container.style.position = 'relative';

      // Create map div.
      var mapDiv = document.createElement('div');
      mapDiv.style.cssText = 'width:100%;height:' + height + 'px;';
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

      // Add markers with optional clustering for dense maps.
      var markerTarget = map;
      if (typeof L.markerClusterGroup === 'function' && markers.length > 5) {
        var clusterGroup = L.markerClusterGroup({
          maxClusterRadius: 40,
          spiderfyOnMaxZoom: true,
          showCoverageOnHover: false,
          iconCreateFunction: function(cluster) {
            var count = cluster.getChildCount();
            return L.divIcon({
              className: 'chronicle-cluster',
              html: '<div class="chronicle-cluster-icon">' + count + '</div>',
              iconSize: [32, 32],
              iconAnchor: [16, 16],
            });
          }
        });
        markerTarget = clusterGroup;
      }

      markers.forEach(function (mk) {
        var icon = L.divIcon({
          className: 'chronicle-marker',
          html: '<div style="color:' + Chronicle.escapeHtml(mk.color || '#ef4444') +
            ';font-size:16px;text-shadow:0 1px 2px rgba(0,0,0,0.4);">' +
            '<i class="fa-solid ' + Chronicle.escapeHtml(mk.icon || 'fa-map-pin') + '"></i></div>',
          iconSize: [20, 20],
          iconAnchor: [10, 10],
        });

        var marker = L.marker([mk.y, mk.x], { icon: icon }).addTo(markerTarget);

        var popupContent = '<div class="text-xs">' +
          '<strong>' + Chronicle.escapeHtml(mk.name) + '</strong>';
        if (mk.description) {
          popupContent += '<br/>' + Chronicle.escapeHtml(mk.description);
        }
        popupContent += '</div>';
        marker.bindPopup(popupContent, { maxWidth: 200 });
      });

      if (markerTarget !== map) {
        map.addLayer(markerTarget);
      }

      // Load Phase 2 objects (drawings and tokens) if enabled.
      if (self.showDrawings || self.showTokens) {
        self._loadPhase2Objects(map, imageW, imageH, mapId);
      }

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
     * Dynamically load Leaflet CSS and MarkerCluster CSS if not already present.
     */
    _loadLeafletCSS: function () {
      if (!document.querySelector('link[href*="leaflet.css"]')) {
        var link = document.createElement('link');
        link.rel = 'stylesheet';
        link.href = '/static/vendor/leaflet.css';
        document.head.appendChild(link);
      }
      // MarkerCluster CSS.
      if (!document.querySelector('link[href*="MarkerCluster"]')) {
        var mcLink = document.createElement('link');
        mcLink.rel = 'stylesheet';
        mcLink.href = '/static/vendor/MarkerCluster.css';
        document.head.appendChild(mcLink);
      }
    },

    /**
     * Dynamically load Leaflet JS and MarkerCluster JS if not already present.
     */
    _loadLeafletJS: function (callback) {
      var self = this;
      if (typeof L !== 'undefined') {
        self._loadMarkerClusterJS(callback);
        return;
      }
      if (document.querySelector('script[src*="leaflet.js"]')) {
        var interval = setInterval(function () {
          if (typeof L !== 'undefined') {
            clearInterval(interval);
            self._loadMarkerClusterJS(callback);
          }
        }, 50);
        return;
      }
      var script = document.createElement('script');
      script.src = '/static/vendor/leaflet.js';
      script.onload = function () {
        self._loadMarkerClusterJS(callback);
      };
      document.head.appendChild(script);
    },

    /**
     * Load MarkerCluster plugin after Leaflet is available.
     */
    _loadMarkerClusterJS: function (callback) {
      if (typeof L.markerClusterGroup === 'function') {
        callback();
        return;
      }
      if (document.querySelector('script[src*="leaflet.markercluster"]')) {
        var interval = setInterval(function () {
          if (typeof L.markerClusterGroup === 'function') {
            clearInterval(interval);
            callback();
          }
        }, 50);
        return;
      }
      var script = document.createElement('script');
      script.src = '/static/vendor/leaflet.markercluster.js';
      script.onload = callback;
      document.head.appendChild(script);
    },

    /**
     * Load Phase 2 map objects (drawings and tokens) from the API.
     */
    _loadPhase2Objects: function (map, imageW, imageH, mapId) {
      var self = this;
      var baseUrl = '/campaigns/' + encodeURIComponent(this.campaignId) +
        '/maps/' + encodeURIComponent(mapId);

      if (this.showDrawings) {
        Chronicle.apiFetch(baseUrl + '/drawings')
          .then(function (res) { return res.ok ? res.json() : []; })
          .then(function (drawings) {
            self._renderDrawings(map, drawings, imageW, imageH);
          })
          .catch(function () { /* Graceful degradation */ });
      }

      if (this.showTokens) {
        Chronicle.apiFetch(baseUrl + '/tokens')
          .then(function (res) { return res.ok ? res.json() : []; })
          .then(function (tokens) {
            self._renderTokens(map, tokens, imageW, imageH);
          })
          .catch(function () { /* Graceful degradation */ });
      }
    },

    /**
     * Render drawings as SVG overlays on the Leaflet map.
     */
    _renderDrawings: function (map, drawings, imageW, imageH) {
      if (!drawings || !Array.isArray(drawings)) return;

      drawings.forEach(function (d) {
        if (!d.points) return;
        var points;
        try {
          points = typeof d.points === 'string' ? JSON.parse(d.points) : d.points;
        } catch (e) { return; }
        if (!Array.isArray(points) || points.length < 2) return;

        // Convert percentage coords to pixel coords for Leaflet CRS.Simple.
        var latlngs = points.map(function (p) {
          return [imageH - (p.y / 100) * imageH, (p.x / 100) * imageW];
        });

        var opts = {
          color: d.stroke_color || '#000',
          weight: d.stroke_width || 2,
          opacity: 0.8,
          interactive: false
        };

        if (d.fill_color) {
          opts.fillColor = d.fill_color;
          opts.fillOpacity = d.fill_alpha || 0.3;
          opts.fill = true;
        }

        switch (d.drawing_type) {
          case 'rectangle':
            if (latlngs.length >= 2) {
              L.rectangle([latlngs[0], latlngs[1]], opts).addTo(map);
            }
            break;
          case 'ellipse':
            if (latlngs.length >= 2) {
              // Approximate ellipse as a circle centered between two points.
              var cx = (latlngs[0][0] + latlngs[1][0]) / 2;
              var cy = (latlngs[0][1] + latlngs[1][1]) / 2;
              var rx = Math.abs(latlngs[1][1] - latlngs[0][1]) / 2;
              L.circle([cx, cy], { radius: rx, ...opts }).addTo(map);
            }
            break;
          case 'polygon':
            L.polygon(latlngs, opts).addTo(map);
            break;
          default:
            // freehand, line
            L.polyline(latlngs, opts).addTo(map);
            break;
        }
      });
    },

    /**
     * Render tokens as icon markers on the Leaflet map.
     */
    _renderTokens: function (map, tokens, imageW, imageH) {
      if (!tokens || !Array.isArray(tokens)) return;

      tokens.forEach(function (t) {
        if (t.is_hidden) return; // Skip GM-only tokens.

        var lat = imageH - (t.y / 100) * imageH;
        var lng = (t.x / 100) * imageW;

        var icon;
        if (t.image_path) {
          icon = L.icon({
            iconUrl: t.image_path,
            iconSize: [32, 32],
            iconAnchor: [16, 16],
            className: 'chronicle-token'
          });
        } else {
          icon = L.divIcon({
            className: 'chronicle-token',
            html: '<div style="width:28px;height:28px;border-radius:50%;background:var(--color-accent,#3b82f6);' +
              'display:flex;align-items:center;justify-content:center;color:#fff;font-size:11px;font-weight:600;' +
              'border:2px solid rgba(255,255,255,0.8);box-shadow:0 1px 3px rgba(0,0,0,0.4);">' +
              Chronicle.escapeHtml((t.name || '?').charAt(0).toUpperCase()) + '</div>',
            iconSize: [28, 28],
            iconAnchor: [14, 14]
          });
        }

        var marker = L.marker([lat, lng], { icon: icon, interactive: true }).addTo(map);

        // Tooltip with token name.
        var tooltip = '<div class="text-xs"><strong>' + Chronicle.escapeHtml(t.name || 'Token') + '</strong>';
        if (t.bar1_value !== null && t.bar1_max !== null) {
          tooltip += '<br/>HP: ' + t.bar1_value + '/' + t.bar1_max;
        }
        tooltip += '</div>';
        marker.bindPopup(tooltip, { maxWidth: 150 });
      });
    },

    destroy: function (el) {
      if (this._leafletMap) {
        this._leafletMap.remove();
        this._leafletMap = null;
      }
    }
  });
})();
