/**
 * Map viewer widget
 *
 * The live Leaflet map: pins, drawings, layers, search, zoom/fit, and for
 * Scribe+ the tool rail and settings sheet. One implementation serves both
 * hosts: the dedicated map page (mounted by boot.js from data-widget="map-viewer"
 * on #map-config) and the focus view that unfolds over an entity page
 * (entity_map.js calls ChronicleMapViewer.open on a fragment it fetched).
 *
 * The server renders the markup and a #map-config element carrying the map's
 * data, already filtered for the viewer's role (see Handler.Show); this file
 * never decides who may see what. Libraries (Leaflet, MarkerCluster, the drawing
 * module) are loaded here on demand, not by a page <script src>, because htmx
 * drops script tags from swapped-in pages and from fetched fragments.
 *
 * Ids inside the markup (#map-wrap, #map-container, #marker-modal, ...) are
 * unique per page, so only one viewer may be mounted at a time.
 */
(function () {
  // ---- Library loading ----
  var scriptLoads = {};
  function loadScript(src) {
    if (!src) return Promise.reject(new Error('missing script url'));
    if (!scriptLoads[src]) {
      scriptLoads[src] = new Promise(function (resolve, reject) {
        var s = document.createElement('script');
        s.src = src;
        s.onload = function () { resolve(); };
        s.onerror = function () { delete scriptLoads[src]; reject(new Error('failed to load ' + src)); };
        document.head.appendChild(s);
      });
    }
    return scriptLoads[src];
  }

  // Leaflet first, then the cluster plugin that extends it (it is optional: the
  // viewer falls back to plain markers if it never arrives), then the drawing
  // module, which reads window.chronicleMap and so must come after mount.
  function ensureLeaflet(cfg) {
    var p = window.L ? Promise.resolve() : loadScript(cfg.leafletSrc);
    return p.then(function () {
      if (typeof L.markerClusterGroup === 'function' || !cfg.clusterSrc) return;
      return loadScript(cfg.clusterSrc).catch(function () { /* clustering is a nicety */ });
    });
  }
  function ensureDrawing(cfg) {
    if (window.ChronicleMapDrawing) return Promise.resolve();
    return loadScript(cfg.drawSrc).catch(function () { /* drawings simply do not render */ });
  }

  // mountViewer builds the viewer on the #map-config element cfgEl. Its body is
  // the viewer proper; opts.onReload replaces the page reload after a save.
  function mountViewer(cfgEl, opts) {
    var cfg = cfgEl;
	// cfg is the #map-config element this viewer is mounted on. The campaign id
	// is read from it so the viewer works on any page URL (the map page and the
	// focus view over an entity page); the path is only a fallback.
	opts = opts || {};
	var campaignID = cfg.dataset.campaignId || window.location.pathname.split('/')[2];
	// After a save that needs fresh server-rendered state (frame, pins) the map
	// page reloads; the focus view passes its own refresh so the person stays on
	// the page the map was unfolded over.
	var reloadView = opts.onReload || function() { window.location.reload(); };
	// Document-level listeners this mount owns, undone by destroy().
	var cleanups = [];
	var destroyed = false;
	function listen(target, evt, fn) { target.addEventListener(evt, fn); cleanups.push(function() { target.removeEventListener(evt, fn); }); }
	var mapID = cfg.dataset.mapId;
	var imageID = cfg.dataset.imageId;
	var mapImageURL = cfg.dataset.imageUrl;
	var imageW = cfg.dataset.imageWidth;
	var imageH = cfg.dataset.imageHeight;
	var isScribe = cfg.dataset.isScribe === 'true';
	var isOwner = cfg.dataset.isOwner === 'true';
	var canDmOnly = cfg.dataset.canDmOnly === 'true';
	// canDraw is what the viewer is OFFERED; the server enforces the
	// map's "who can draw" setting on every drawing write regardless.
	var canDraw = cfg.dataset.canDraw === 'true';
	var userID = cfg.dataset.userId || '';
	// The map's display settings, resolved server-side with every
	// default filled in, so the defaults live in one place (Go).
	var D;
	try { D = JSON.parse(cfg.dataset.display || '{}'); } catch (e) { D = {}; }
	if (!D || typeof D !== 'object') D = {};
	var savedD = JSON.parse(JSON.stringify(D));
	// Coerce defensively: JSON.parse('null') returns null and would
	// crash on every markers.length / markers.forEach below. Server
	// now returns "[]" for empty (mustMarshalMarkers), but keep this
	// belt-and-braces so a stale deploy or a freshly-created map
	// doesn't take the page down.
	var markers;
	try { markers = JSON.parse(cfg.dataset.markers || '[]'); } catch (e) { markers = []; }
	if (!Array.isArray(markers)) markers = [];

	var w = parseInt(imageW) || 1000;
	var h = parseInt(imageH) || 1000;

	// Create Leaflet map with CRS.Simple for image overlay.
	var map = L.map('map-container', {
		crs: L.CRS.Simple,
		minZoom: -3,
		maxZoom: 3,
		zoomSnap: 0.25,
		attributionControl: false,
		// Zoom lives in the bottom-right control cluster; Leaflet's own
		// top-left control would sit under the search box and tool rail.
		zoomControl: false,
		zoomAnimation: !(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches),
		fadeAnimation: !(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches),
		markerZoomAnimation: !(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches),
		// Double-click is repurposed for inline pin creation (Scribe+),
		// so disable Leaflet's default double-click-to-zoom — otherwise
		// the create gesture would also zoom the map underneath it.
		doubleClickZoom: false,
	});

	// Set image bounds and add overlay if image exists.
	var bounds = [[0, 0], [h, w]];
	if (imageID) {
		L.imageOverlay(mapImageURL, bounds).addTo(map);
	}

	// ---- Opening view: the whole map, where this person left off, or a
	// spot the owner chose. "Left off" is stored in this browser per
	// user and map, never on the server. A stored spot is percent of the
	// image plus a zoom relative to the whole-map view, so it means the
	// same on any screen. ----
	var viewKey = 'chronicle.mapview.' + (userID || 'anon') + '.' + mapID;
	var viewReady = false;
	var viewTimer = null;
	function savedView() {
		try {
			var v = JSON.parse(localStorage.getItem(viewKey));
			if (v && isFinite(v.x) && isFinite(v.y) && isFinite(v.z)) return v;
		} catch (e) { /* storage blocked or corrupt: open on the whole map */ }
		return null;
	}
	function applyOpening() {
		var v = null;
		if (D.open_mode === 'last') v = savedView();
		else if (D.open_mode === 'spot') v = { x: D.open_x, y: D.open_y, z: D.open_zoom };
		if (!v) { map.fitBounds(bounds); return; }
		map.setView(pinLatLng(v.x, v.y), fitZoom() + v.z, { animate: false });
	}
	// Remember where the person is, once layout has settled (before that
	// the view is a pre-layout guess and must not overwrite a good one).
	map.on('moveend', function() {
		if (!viewReady) return;
		clearTimeout(viewTimer);
		viewTimer = setTimeout(function() {
			var c = latLngToPercent(map.getCenter());
			try { localStorage.setItem(viewKey, JSON.stringify({ x: c.x, y: c.y, z: map.getZoom() - fitZoom() })); } catch (e) { /* not worth failing for */ }
		}, 300);
	});
	applyOpening();

	// Re-measure once layout settles. This inline IIFE runs during
	// parse, before the flex-sized container (dedicated page) or a
	// freshly-shown embed has its final height — so Leaflet reads the
	// wrong size at construction: the image overlay paints into the
	// wrong box (the container's bg-surface-alt shows through as a
	// pale rectangle) and markers land at wrong pixels (piling near
	// the top-left, under the toolbar). invalidateSize + a re-fit fix
	// both — the same guard the dashboard map widget already uses
	// (map_widget.js). Harmless when the size was already correct.
	setTimeout(function () {
		if (destroyed) return;
		map.invalidateSize();
		applyOpening();
		viewReady = true;
	}, 100);

	// Track placed Leaflet markers by ID for removal/update.
	var leafletMarkers = {};

	// Escape helpers to prevent XSS when building HTML strings from
	// user-supplied marker data (name, description, icon, color).
	function escapeHtml(s) {
		if (!s) return '';
		return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
	}
	function escapeAttr(s) {
		if (!s) return '';
		return s.replace(/&/g,'&amp;').replace(/"/g,'&quot;').replace(/'/g,'&#39;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
	}

	// Use marker clustering when there are many markers.
	var clusterGroup = null;
	if (typeof L.markerClusterGroup === 'function' && markers.length > 5) {
		clusterGroup = L.markerClusterGroup({
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
	}

	// Pin kinds. The five ids are the server's pin_category set (and the
	// Foundry module's pin types); 'other' is the client-only bucket for
	// markers that have none. Labels and colours come from this map's
	// display settings (renamed/recoloured in Map settings), and a pin
	// created from the quick card gets its kind's colour.
	var KINDS = (D.kinds && D.kinds.length ? D.kinds : [
		{ id: 'location', label: 'Places', color: '#2563eb' },
		{ id: 'danger', label: 'Danger', color: '#dc2626' },
		{ id: 'treasure', label: 'Treasure', color: '#d97706' },
		{ id: 'quest', label: 'Quests', color: '#7c3aed' },
		{ id: 'note', label: 'Notes', color: '#0d9488' }
	]).map(function(k) { return { id: k.id, label: k.label, one: k.label, color: k.color }; });
	var OTHER_KIND = { id: 'other', label: 'Other', one: 'Other', color: '#6b7280' };
	function kindInfo(id) {
		for (var i = 0; i < KINDS.length; i++) if (KINDS[i].id === id) return KINDS[i];
		return OTHER_KIND;
	}
	// A stored legacy value outside the set reads as "other"; it is only
	// ever rewritten if the person picks a different kind.
	function kindOf(mk) { return kindInfo(mk.pin_category).id; }

	var reduceMotion = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
	var kindOn = { location: true, danger: true, treasure: true, quest: true, note: true, other: true };
	var showDrawings = true;
	var showHiddenPins = true;

	// Pin visibility is client-side only: filters never touch the server.
	function pinShown(mk) {
		if (!kindOn[kindOf(mk)]) return false;
		if (mk.visibility === 'dm_only' && !(isScribe && showHiddenPins)) return false;
		return true;
	}

	// ---- Pin shape, size and name. The shape is drawn around the marker's
	// own Font Awesome icon; the colour is the marker's. Metrics are per
	// style because each shape's tip (the point on the map) is elsewhere. ----
	var PIN_SCALE = { s: 0.75, m: 1, l: 1.35 };
	// cx, cy: where the icon sits in the 30x38 box; fs: icon size; ax, ay: the
	// map point (the tip of a drop, the foot of a flag, the middle of a dot).
	var PIN_SHAPE = {
		drop: { cx: 15, cy: 15, fs: 12, ax: 15, ay: 37 },
		seal: { cx: 15, cy: 22, fs: 12, ax: 15, ay: 37 },
		flag: { cx: 17, cy: 11, fs: 8, ax: 8, ay: 37 },
		dot: { cx: 15, cy: 19, fs: 11, ax: 15, ay: 19 }
	};
	function pinMetrics() {
		var k = PIN_SCALE[D.pin_size] || 1;
		var sh = PIN_SHAPE[D.pin_style] || PIN_SHAPE.drop;
		return { k: k, sh: sh, w: 30 * k, h: 38 * k, ax: sh.ax * k, ay: sh.ay * k };
	}
	function pinShapeSvg(style, color, dashed) {
		var c = escapeAttr(color);
		var dash = dashed ? ' stroke-dasharray="3 2"' : '';
		if (style === 'seal') return '<svg viewBox="0 0 30 38" aria-hidden="true"><path d="M15 34v4" stroke="' + c + '" stroke-width="2"/><circle cx="15" cy="22" r="12" fill="' + c + '" stroke="#fff" stroke-width="2"' + dash + '/><circle cx="15" cy="22" r="8" fill="none" stroke="rgba(255,255,255,.55)" stroke-width="1.2"/></svg>';
		if (style === 'flag') return '<svg viewBox="0 0 30 38" aria-hidden="true"><path d="M8 37V4" stroke="#3b2f1e" stroke-width="2.4"/><path d="M9 5h17l-4 6 4 6H9z" fill="' + c + '" stroke="#fff" stroke-width="1.5"' + dash + '/></svg>';
		if (style === 'dot') return '<svg viewBox="0 0 30 38" aria-hidden="true"><circle cx="15" cy="19" r="10" fill="' + c + '" stroke="#fff" stroke-width="2.5"' + dash + '/></svg>';
		return '<svg viewBox="0 0 30 38" aria-hidden="true"><path d="M15 37s-12-12.5-12-22a12 12 0 0124 0c0 9.5-12 22-12 22z" fill="' + c + '" stroke="#fff" stroke-width="2"' + dash + '/></svg>';
	}
	function markerIcon(mk) {
		var m = pinMetrics();
		var hidden = mk.visibility === 'dm_only';
		var badge = hidden
			? '<span class="mp-pin-badge" title="Hidden from players"><i class="fa-solid fa-eye-slash"></i></span>' : '';
		var ico = '<i class="fa-solid ' + escapeAttr(mk.icon) + ' mp-pin-ico" style="left:' + (m.sh.cx * m.k) + 'px;top:' + (m.sh.cy * m.k) + 'px;font-size:' + (m.sh.fs * m.k) + 'px"></i>';
		// A name shown always sits under the pin; otherwise it appears above.
		var below = D.pin_labels === 'always';
		return L.divIcon({
			className: 'chronicle-marker',
			html: '<div class="mp-pin" style="width:' + m.w + 'px;height:' + m.h + 'px">' + pinShapeSvg(D.pin_style, mk.color, hidden) + ico + badge + '</div>',
			iconSize: [m.w, m.h],
			iconAnchor: [m.ax, m.ay],
			tooltipAnchor: [m.w / 2 - m.ax, below ? m.h - m.ay + 2 : -m.ay]
		});
	}
	// The name shown on the map: always, when pointed at, or never.
	function bindLabel(lm, mk) {
		lm.unbindTooltip();
		if (D.pin_labels === 'never') return;
		var always = D.pin_labels === 'always';
		lm.bindTooltip(escapeHtml(mk.name), { direction: always ? 'bottom' : 'top', permanent: always, className: 'mp-name' });
	}
	// Re-draw every pin after a style, size, name or kind change.
	function refreshPins() {
		markers.forEach(function(mk) {
			var lm = leafletMarkers[mk.id];
			if (!lm) return;
			lm.setIcon(markerIcon(mk));
			bindLabel(lm, mk);
		});
	}

	// Render all existing markers.
	markers.forEach(function(mk) { addMarkerToMap(mk); });

	// Add cluster group to map if used.
	if (clusterGroup) {
		map.addLayer(clusterGroup);
	}

	function addMarkerToMap(mk) {
		// Convert percentage coords (0-100) to pixel coords.
		var lat = h - (mk.y / 100) * h;
		var lng = (mk.x / 100) * w;

		var lm = L.marker([lat, lng], { icon: markerIcon(mk), draggable: isScribe });
		if (pinShown(mk)) (clusterGroup || map).addLayer(lm);

		// The name on the map (escaped to prevent HTML injection).
		bindLabel(lm, mk);

		// A click opens the read card; scribes reach editing from there.
		lm.on('click', function() { openCard(mk, 'read'); });

		if (isScribe) {
			lm.on('dragend', function(e) {
				var pos = e.target.getLatLng();
				var newX = (pos.lng / w) * 100;
				var newY = ((h - pos.lat) / h) * 100;
				// Clamp to 0-100.
				newX = Math.max(0, Math.min(100, newX));
				newY = Math.max(0, Math.min(100, newY));
				mk.x = newX;
				mk.y = newY;
				updateMarkerPosition(mk);
			});
		}

		leafletMarkers[mk.id] = lm;
	}

	// Show or hide one marker to match the current filters.
	function syncMarkerVisibility(mk) {
		var lm = leafletMarkers[mk.id];
		if (!lm) return;
		var target = clusterGroup || map;
		var want = pinShown(mk);
		var has = target.hasLayer(lm);
		if (want && !has) target.addLayer(lm);
		if (!want && has) target.removeLayer(lm);
	}
	function applyFilters() {
		markers.forEach(syncMarkerVisibility);
		renderPanel();
	}

	// ---- Small DOM builder for cards: user text only ever goes through
	// textContent, so nothing a person typed can become markup. ----
	function el(tag, cls, text) {
		var e = document.createElement(tag);
		if (cls) e.className = cls;
		if (text != null) e.textContent = text;
		return e;
	}
	function field(label, control) {
		var l = el('label', 'mp-field');
		l.appendChild(el('span', null, label));
		l.appendChild(control);
		return l;
	}

	// ---- Pin cards (Leaflet popups restyled as cards) ----
	var draft = null;      // a pin being placed, not yet saved
	var draftPopup = null;

	function cardOptions() {
		return { className: 'mp-card-popup', minWidth: 230, maxWidth: 270, autoPanPadding: L.point(70, 70), closeOnClick: true };
	}
	function showPopup(latlng, node) {
		var pop = L.popup(cardOptions()).setLatLng(latlng).setContent(node);
		pop.openOn(map);
		return pop;
	}
	function pinLatLng(x, y) { return L.latLng(h - (y / 100) * h, (x / 100) * w).clone(); }
	// The popup sits above the pin's tip, not on top of it.
	function popupAnchor(x, y) {
		var ll = pinLatLng(x, y);
		var pt = map.latLngToContainerPoint(ll).subtract([0, 26]);
		return map.containerPointToLatLng(pt);
	}

	function openCard(mk, mode) {
		removeDraft();
		var node = mode === 'edit' ? editCard(mk, null) : readCard(mk);
		showPopup(popupAnchor(mk.x, mk.y), node);
		var first = node.querySelector('input');
		if (first && mode === 'edit') { first.focus(); first.select(); }
	}

	function readCard(mk) {
		var k = kindInfo(mk.pin_category);
		var card = el('div', 'mp-card');
		var title = el('div', 'mp-card-title');
		var dot = el('span', 'mp-dot');
		dot.style.background = k.color;
		title.appendChild(dot);
		title.appendChild(el('strong', null, mk.name));
		card.appendChild(title);
		var meta = el('div', 'mp-card-meta', k.one);
		if (mk.visibility === 'dm_only') {
			var badge = el('span', 'mp-badge');
			badge.appendChild(el('i', 'fa-solid fa-eye-slash'));
			badge.appendChild(document.createTextNode(' Hidden from players'));
			meta.appendChild(badge);
		}
		card.appendChild(meta);
		if (mk.description) card.appendChild(el('div', 'mp-card-desc', mk.description));
		if (mk.entity_name && mk.entity_id) {
			var a = el('a', 'mp-card-link', mk.entity_name);
			a.href = '/campaigns/' + encodeURIComponent(campaignID) + '/entities/' + encodeURIComponent(mk.entity_id);
			card.appendChild(a);
		}
		if (isScribe) {
			var row = el('div', 'mp-card-row');
			var edit = el('button', 'mp-cbtn', 'Edit');
			edit.type = 'button';
			edit.addEventListener('click', function() { openCard(mk, 'edit'); });
			row.appendChild(edit);
			card.appendChild(row);
		}
		return card;
	}

	// editCard builds the quick form. `mk` is a saved marker, or a draft
	// ({x, y, isDraft}) for a pin that does not exist yet.
	function editCard(mk, draftState) {
		var isNew = !!draftState;
		var origKind = isNew ? 'location' : (kindOf(mk) === 'other' ? '' : kindOf(mk));
		var origVis = isNew ? 'everyone' : (mk.visibility || 'everyone');
		var card = el('div', 'mp-card');
		card.appendChild(el('div', 'mp-card-title', isNew ? 'New pin' : 'Edit pin'));

		var name = el('input', 'mp-input');
		name.type = 'text';
		name.maxLength = 255;
		name.value = isNew ? '' : (mk.name || '');
		name.placeholder = 'Pin name';
		card.appendChild(field('Name', name));

		var kind = el('select', 'mp-input');
		if (origKind === '') kind.appendChild(new Option('Other', ''));
		KINDS.forEach(function(k) { kind.appendChild(new Option(k.one, k.id)); });
		kind.value = origKind;
		card.appendChild(field('Kind', kind));

		// Hiding a pin from players is an owner-level power on the server, so
		// only offer it to someone who can use it.
		var vis = null;
		if (canDmOnly) {
			vis = el('select', 'mp-input');
			vis.appendChild(new Option('Everyone', 'everyone'));
			vis.appendChild(new Option('Only owners and co-DMs', 'dm_only'));
			vis.value = origVis;
			card.appendChild(field('Who can see it', vis));
		}

		var row = el('div', 'mp-card-row');
		var save = el('button', 'mp-cbtn mp-cbtn-primary', 'Save');
		save.type = 'button';
		row.appendChild(save);
		// The server allows a scribe to remove only pins they created
		// (created_by is the server's own field), owners any pin.
		if (isNew || isOwner || (isScribe && userID && mk.created_by === userID)) {
			var rm = el('button', 'mp-cbtn', 'Remove');
			rm.type = 'button';
			row.appendChild(rm);
			rm.addEventListener('click', function() {
				if (isNew) { map.closePopup(); return; }
				if (!confirm('Remove this pin?')) return;
				deletePin(mk);
			});
		}
		card.appendChild(row);

		var more = el('button', 'mp-card-more', 'More options');
		more.type = 'button';
		card.appendChild(more);

		function curVis() { return vis ? vis.value : origVis; }
		save.addEventListener('click', function() {
			var nm = name.value.trim();
			if (!nm) { name.focus(); Chronicle.notify('Give the pin a name', 'error'); return; }
			if (isNew) createPin(draftState, nm, kind.value, curVis(), save);
			else updatePin(mk, nm, kind.value, origKind, curVis(), save);
		});
		name.addEventListener('keydown', function(e) { if (e.key === 'Enter') { e.preventDefault(); save.click(); } });
		more.addEventListener('click', function() {
			var carry = { name: name.value.trim(), kind: kind.value, vis: curVis() };
			map.closePopup();
			leaveFullscreen();
			if (isNew) openCreateMarker(draftState.x, draftState.y); else openEditMarker(mk);
			if (carry.name) document.getElementById('mk-name').value = carry.name;
			document.getElementById('mk-kind').value = carry.kind;
			// A new pin starts in its kind's colour, as the quick card's does.
			if (isNew && carry.kind) document.getElementById('mk-color').value = kindInfo(carry.kind).color;
			document.getElementById('mk-visibility').value = carry.vis;
		});
		return card;
	}

	// ---- Quick-card writes. Each sends only what the person changed. ----
	function createPin(d, nm, kind, visibility, btn) {
		var body = { name: nm, x: d.x, y: d.y, visibility: visibility };
		if (kind) { body.pin_category = kind; body.color = kindInfo(kind).color; }
		btn.disabled = true;
		Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/markers', { method: 'POST', body: body })
			.then(function(resp) {
				if (!resp.ok) return resp.json().catch(function() { return {}; }).then(function(e) {
					btn.disabled = false;
					Chronicle.notify(e.message || 'Failed to save pin', 'error');
				});
				return resp.json().then(function(mk) {
					removeDraft();
					markers.push(mk);
					if (kind) kindOn[kind] = true;
					addMarkerToMap(mk);
					syncMarkerVisibility(mk);
					renderPanel();
					openCard(mk, 'read');
				});
			})
			.catch(function(err) { btn.disabled = false; Chronicle.notify('Network error: ' + err.message, 'error'); });
	}

	function updatePin(mk, nm, kind, origKind, visibility, btn) {
		var body = {};
		if (nm !== mk.name) body.name = nm;
		if (kind !== origKind) body.pin_category = kind || null;
		if (visibility !== (mk.visibility || 'everyone')) body.visibility = visibility;
		if (!Object.keys(body).length) { openCard(mk, 'read'); return; }
		btn.disabled = true;
		Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/markers/' + mk.id, { method: 'PUT', body: body })
			.then(function(resp) {
				if (!resp.ok) return resp.json().catch(function() { return {}; }).then(function(e) {
					btn.disabled = false;
					Chronicle.notify(e.message || 'Failed to save pin', 'error');
				});
				if ('name' in body) mk.name = body.name;
				if ('pin_category' in body) { if (body.pin_category) mk.pin_category = body.pin_category; else delete mk.pin_category; }
				if ('visibility' in body) mk.visibility = body.visibility;
				var lm = leafletMarkers[mk.id];
				if (lm) { lm.setIcon(markerIcon(mk)); bindLabel(lm, mk); }
				syncMarkerVisibility(mk);
				renderPanel();
				openCard(mk, 'read');
			})
			.catch(function(err) { btn.disabled = false; Chronicle.notify('Network error: ' + err.message, 'error'); });
	}

	function deletePin(mk) {
		Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/markers/' + mk.id, { method: 'DELETE' })
			.then(function(resp) {
				if (!resp.ok) return resp.json().catch(function() { return {}; }).then(function(e) {
					Chronicle.notify(e.message || 'Failed to remove pin', 'error');
				});
				var lm = leafletMarkers[mk.id];
				if (lm) { (clusterGroup || map).removeLayer(lm); delete leafletMarkers[mk.id]; }
				markers.splice(markers.indexOf(mk), 1);
				map.closePopup();
				renderPanel();
			})
			.catch(function(err) { Chronicle.notify('Network error: ' + err.message, 'error'); });
	}

	// ---- Dropping a pin: a draft marker plus the quick card. Closing the
	// card without saving discards the draft. ----
	function startDraft(x, y) {
		removeDraft();
		var ll = pinLatLng(x, y);
		var marker = L.marker(ll, {
			icon: markerIcon({ icon: 'fa-map-pin', color: KINDS[0].color, visibility: 'everyone' }),
			interactive: false, keyboard: false
		}).addTo(map);
		draft = { x: x, y: y, marker: marker };
		var node = editCard(null, draft);
		draftPopup = showPopup(popupAnchor(x, y), node);
		var first = node.querySelector('input');
		if (first) first.focus();
	}
	function removeDraft() {
		if (!draft) return;
		var d = draft;
		draft = null;
		draftPopup = null;
		map.removeLayer(d.marker);
	}
	map.on('popupclose', function(e) {
		if (draft && e.popup === draftPopup) removeDraft();
	});

	// ---- Fly to a pin and open its card (search results, pin list) ----
	function flyToMarker(mk) {
		var lm = leafletMarkers[mk.id];
		if (!lm) return;
		var done = false;
		function show() { if (done) return; done = true; openCard(mk, 'read'); }
		if (clusterGroup && clusterGroup.hasLayer(lm)) {
			clusterGroup.zoomToShowLayer(lm, show);
			return;
		}
		var ll = lm.getLatLng();
		var z = Math.min(map.getMaxZoom(), Math.max(map.getZoom(), fitZoom() + 1.5));
		if (reduceMotion) {
			map.setView(ll, z, { animate: false });
			show();
			return;
		}
		map.once('moveend', show);
		map.flyTo(ll, z, { duration: 0.6 });
		setTimeout(show, 900);
	}

	// ===== Floating controls =====
	var wrap = document.getElementById('map-wrap');
	function $id(id) { return document.getElementById(id); }
	var tool = 'move';
	var drawShape = 'freehand';
	var drawColor = '#2563eb';
	var drawWidth = 4;
	var SHAPE_ICON = { freehand: 'fa-pen', rectangle: 'fa-vector-square', ellipse: 'fa-circle', polygon: 'fa-draw-polygon', text: 'fa-font' };
	var SHAPE_HINT = {
		freehand: 'Drag to draw a line',
		rectangle: 'Click one corner, then the opposite corner',
		ellipse: 'Click the centre, then click the edge',
		polygon: 'Click each corner, double-click to finish',
		text: 'Click where the label goes'
	};
	function draw() { return window.chronicleMap && window.chronicleMap.draw; }

	// Zoom: 100% is the whole map fitted to the view, so the number means
	// the same thing on every map regardless of image size.
	function fitZoom() { return map.getBoundsZoom(bounds, false); }
	// The Futuristic frame's live readout shares the zoom figure and the
	// pointer position (percent of the image in thousandths, so it reads
	// 000-1000 on any map). Both elements only exist inside that frame.
	var frameEl = document.getElementById('mp-frame');
	var hudZoom = frameEl && frameEl.querySelector('.mp-hudzoom');
	var hudPos = frameEl && frameEl.querySelector('.mp-hudpos');
	function pad3(n) { n = Math.round(n); return (n < 100 ? (n < 10 ? '00' : '0') : '') + n; }
	if (hudPos) map.on('mousemove', function(e) {
		var p = latLngToPercent(e.latlng);
		hudPos.textContent = 'X ' + pad3(p.x * 10) + '  Y ' + pad3(p.y * 10);
	});
	function updateZoomLabel() {
		var pct = Math.round(Math.pow(2, map.getZoom() - fitZoom()) * 100);
		$id('mp-zoom-level').textContent = pct + '%';
		if (hudZoom) hudZoom.textContent = 'ZOOM ' + pct + '%';
	}
	map.on('zoom zoomend resize', updateZoomLabel);
	updateZoomLabel();
	setTimeout(updateZoomLabel, 150);
	$id('mp-zoom-in').addEventListener('click', function() { map.zoomIn(); });
	$id('mp-zoom-out').addEventListener('click', function() { map.zoomOut(); });
	$id('mp-fit').addEventListener('click', function() { map.fitBounds(bounds, { animate: !reduceMotion }); });

	// Full screen on the map wrapper (popups and controls live inside it).
	var fsBtn = $id('mp-fullscreen');
	var fsSupported = !!(document.fullscreenEnabled && wrap.requestFullscreen);
	if (!fsSupported) {
		fsBtn.hidden = true;
	} else {
		fsBtn.addEventListener('click', function() {
			if (document.fullscreenElement) document.exitFullscreen().catch(function() {});
			else wrap.requestFullscreen().catch(function() {});
		});
		listen(document, 'fullscreenchange', function() {
			// The wrapper's size changed under Leaflet; re-measure.
			setTimeout(function() { if (!destroyed) { map.invalidateSize(); updateZoomLabel(); } }, 100);
		});
	}
	function leaveFullscreen() {
		if (document.fullscreenElement && document.exitFullscreen) document.exitFullscreen().catch(function() {});
	}

	// Hint bar.
	var hintEl = $id('mp-hint');
	function setHint(msg) {
		if (!msg) { hintEl.hidden = true; return; }
		hintEl.textContent = msg + ' · Esc to stop';
		hintEl.hidden = false;
	}

	// Popovers beside the tool rail (draw shapes, colour and line).
	var flyDraw = $id('mp-fly-draw');
	var flyStyle = $id('mp-fly-style');
	function closePopovers() {
		var any = false;
		[flyDraw, flyStyle].forEach(function(f) {
			if (f && !f.hidden) { f.hidden = true; any = true; }
		});
		var db = document.querySelector('[data-tool="draw"]');
		if (db) db.setAttribute('aria-expanded', 'false');
		var sb = $id('mp-style-btn');
		if (sb) sb.setAttribute('aria-expanded', 'false');
		return any;
	}
	function openPopover(f, btn) {
		var wasOpen = !f.hidden;
		closePopovers();
		if (wasOpen) return;
		f.hidden = false;
		btn.setAttribute('aria-expanded', 'true');
	}

	function syncRail() {
		document.querySelectorAll('[data-tool]').forEach(function(b) {
			b.setAttribute('aria-pressed', b.dataset.tool === tool ? 'true' : 'false');
		});
		var db = document.querySelector('[data-tool="draw"] i');
		if (db) db.className = 'fa-solid ' + SHAPE_ICON[drawShape];
		document.querySelectorAll('[data-shape]').forEach(function(b) {
			b.setAttribute('aria-pressed', (tool === 'draw' && b.dataset.shape === drawShape) ? 'true' : 'false');
		});
		// The drawing module sets its own crosshair while a shape tool runs.
		if (tool === 'pin') map.getContainer().style.cursor = 'crosshair';
		else if (tool === 'move') map.getContainer().style.cursor = '';
	}

	function setTool(t) {
		if (!isScribe) t = 'move';
		closePopovers();
		if (t !== 'draw' && draw()) draw().cancel();
		tool = t;
		if (t === 'pin') setHint('Click the map to drop a pin');
		else if (t === 'draw') setHint(SHAPE_HINT[drawShape]);
		else setHint('');
		syncRail();
	}
	function startShape(shape) {
		if (!draw()) { Chronicle.notify('Drawing tools are still loading', 'error'); return; }
		closePopovers();
		drawShape = shape;
		tool = 'draw';
		draw().setStyle({ color: drawColor, width: drawWidth });
		draw().start(shape);
		setHint(SHAPE_HINT[shape]);
		syncRail();
	}

	if (isScribe) {
		document.querySelectorAll('[data-tool]').forEach(function(b) {
			b.addEventListener('click', function() {
				var t = b.dataset.tool;
				if (t === 'draw') { openPopover(flyDraw, b); return; }
				setTool(tool === t && t !== 'move' ? 'move' : t);
			});
		});
	}
	// Drawing tools exist only when the viewer may draw on this map.
	if (canDraw) {
		document.querySelectorAll('[data-shape]').forEach(function(b) {
			b.addEventListener('click', function() { startShape(b.dataset.shape); });
		});
		var styleBtn = $id('mp-style-btn');
		styleBtn.addEventListener('click', function() { openPopover(flyStyle, styleBtn); });
		function syncStyle() {
			$id('mp-style-swatch').style.background = drawColor;
			flyStyle.querySelectorAll('[data-color]').forEach(function(b) {
				b.setAttribute('aria-pressed', b.dataset.color === drawColor ? 'true' : 'false');
			});
			flyStyle.querySelectorAll('[data-width]').forEach(function(b) {
				b.setAttribute('aria-pressed', parseInt(b.dataset.width, 10) === drawWidth ? 'true' : 'false');
			});
		}
		flyStyle.querySelectorAll('[data-color]').forEach(function(b) {
			b.addEventListener('click', function() {
				drawColor = b.dataset.color;
				if (draw()) draw().setStyle({ color: drawColor });
				syncStyle();
			});
		});
		flyStyle.querySelectorAll('[data-width]').forEach(function(b) {
			b.addEventListener('click', function() {
				drawWidth = parseInt(b.dataset.width, 10);
				if (draw()) draw().setStyle({ width: drawWidth });
				syncStyle();
			});
		});
		syncStyle();
		var undoBtn = $id('mp-undo');
		undoBtn.addEventListener('click', function() { if (draw()) draw().undo(); });
		undoBtn.disabled = true;
		window.__mpUndoChange = function(n) { undoBtn.disabled = n === 0; };
		syncRail();
	}

	// ---- Pins & layers panel ----
	var panel = $id('mp-layers');
	var layersBtn = $id('mp-layers-btn');
	var drawingCount = 0;
	function dot(color) { return '<span class="mp-dot" style="background:' + escapeAttr(color) + '"></span>'; }
	function renderPanel() {
		if (!panel) return;
		var focusKey = document.activeElement && panel.contains(document.activeElement)
			? document.activeElement.getAttribute('data-key') : null;
		var counts = {};
		var hiddenCount = 0;
		markers.forEach(function(mk) {
			var k = kindOf(mk);
			counts[k] = (counts[k] || 0) + 1;
			if (mk.visibility === 'dm_only') hiddenCount++;
		});
		var html = '<div class="mp-sec"><h3>Show on map</h3>';
		KINDS.concat(counts.other ? [OTHER_KIND] : []).forEach(function(k) {
			html += '<button type="button" class="mp-tg" data-key="kind-' + k.id + '" data-kind="' + k.id + '" aria-pressed="' + kindOn[k.id] + '">'
				+ dot(k.color) + '<span>' + escapeHtml(k.label) + '</span><span class="mp-n">' + (counts[k.id] || 0) + '</span></button>';
		});
		html += '<button type="button" class="mp-tg" data-key="drawings" data-toggle="drawings" aria-pressed="' + showDrawings + '">'
			+ '<i class="fa-solid fa-pen mp-eye"></i><span>Drawings</span><span class="mp-n">' + drawingCount + '</span></button>';
		if (isScribe && (canDmOnly || hiddenCount > 0)) {
			html += '<button type="button" class="mp-tg" data-key="hidden" data-toggle="hidden" aria-pressed="' + showHiddenPins + '">'
				+ '<i class="fa-solid fa-eye-slash mp-eye"></i><span>Pins hidden from players</span><span class="mp-n">' + hiddenCount + '</span></button>';
		}
		html += '</div><div class="mp-sec"><h3>All pins</h3>';
		var list = markers.filter(pinShown).sort(function(a, b) { return (a.name || '').localeCompare(b.name || ''); });
		if (!list.length) html += '<p class="mp-empty">No pins to show</p>';
		list.forEach(function(mk) {
			html += '<button type="button" class="mp-pl" data-key="pin-' + escapeAttr(mk.id) + '" data-id="' + escapeAttr(mk.id) + '">'
				+ dot(kindInfo(mk.pin_category).color) + '<span class="mp-pl-name">' + escapeHtml(mk.name) + '</span>'
				+ (mk.visibility === 'dm_only' ? '<em>hidden</em>' : '') + '</button>';
		});
		html += '</div>';
		panel.innerHTML = html;
		if (focusKey) {
			var again = panel.querySelector('[data-key="' + focusKey.replace(/"/g, '') + '"]');
			if (again) again.focus();
		}
	}
	panel.addEventListener('click', function(e) {
		var b = e.target.closest('button');
		if (!b) return;
		if (b.dataset.kind) { kindOn[b.dataset.kind] = !kindOn[b.dataset.kind]; applyFilters(); return; }
		if (b.dataset.toggle === 'drawings') {
			showDrawings = !showDrawings;
			if (draw()) draw().setVisible(showDrawings);
			renderPanel();
			return;
		}
		if (b.dataset.toggle === 'hidden') { showHiddenPins = !showHiddenPins; applyFilters(); return; }
		if (b.dataset.id) {
			var mk = markers.filter(function(m) { return m.id === b.dataset.id; })[0];
			if (mk) flyToMarker(mk);
		}
	});
	layersBtn.addEventListener('click', function() {
		var open = panel.hidden;
		if (open && window.__mpCloseSheet) window.__mpCloseSheet();
		panel.hidden = !open;
		layersBtn.setAttribute('aria-expanded', open ? 'true' : 'false');
		if (open) renderPanel();
	});
	function closePanel() {
		if (panel.hidden) return false;
		panel.hidden = true;
		layersBtn.setAttribute('aria-expanded', 'false');
		return true;
	}

	// ---- Grid overlay: an SVG laid over the map picture in map
	// coordinates (a Leaflet layer, not CSS), so it scales and pans with
	// the map. Size is measured as if the map were 1000 wide, so the same
	// number looks the same on any picture. The line keeps a constant
	// 1px width at every zoom (non-scaling stroke). ----
	var gridLayer = null;
	var gridPending = null;
	map.createPane('mpGrid');
	map.getPane('mpGrid').style.zIndex = 405;
	map.getPane('mpGrid').style.pointerEvents = 'none';
	function gridPath(kind, cell) {
		var d = '', x, y;
		if (kind === 'square') {
			for (x = 0; x <= w; x += cell) d += 'M' + x.toFixed(1) + ' 0V' + h;
			for (y = 0; y <= h; y += cell) d += 'M0 ' + y.toFixed(1) + 'H' + w;
			return d;
		}
		var r = cell / 2, hw = Math.sqrt(3) * r;
		for (var row = 0; row * 1.5 * r < h + r; row++) {
			for (var col = 0; col * hw < w + hw; col++) {
				var cx = col * hw + (row % 2 ? hw / 2 : 0), cy = row * 1.5 * r;
				d += 'M' + cx.toFixed(1) + ' ' + (cy - r).toFixed(1);
				for (var i = 1; i <= 6; i++) {
					var a = Math.PI / 3 * i - Math.PI / 2;
					d += 'L' + (cx + r * Math.cos(a)).toFixed(1) + ' ' + (cy + r * Math.sin(a)).toFixed(1);
				}
			}
		}
		return d;
	}
	function drawGrid() {
		if (gridLayer) { map.removeLayer(gridLayer); gridLayer = null; }
		if (D.grid_type !== 'square' && D.grid_type !== 'hex') return;
		var cell = D.grid_size * w / 1000;
		// A guard against a degenerate picture shape making millions of cells.
		var cells = (w / cell + 2) * (h / (cell * (D.grid_type === 'hex' ? 0.75 : 1)) + 2);
		if (!(cell >= 2) || cells > 60000) return;
		var ns = 'http://www.w3.org/2000/svg';
		var svg = document.createElementNS(ns, 'svg');
		svg.setAttribute('xmlns', ns);
		svg.setAttribute('viewBox', '0 0 ' + w + ' ' + h);
		svg.setAttribute('preserveAspectRatio', 'none');
		var path = document.createElementNS(ns, 'path');
		path.setAttribute('d', gridPath(D.grid_type, cell));
		path.setAttribute('fill', 'none');
		path.setAttribute('stroke', '#241c10');
		path.setAttribute('stroke-opacity', String(D.grid_strength / 100));
		path.setAttribute('stroke-width', '1');
		path.setAttribute('vector-effect', 'non-scaling-stroke');
		svg.appendChild(path);
		gridLayer = L.svgOverlay(svg, bounds, { pane: 'mpGrid', interactive: false, className: 'mp-grid' }).addTo(map);
	}
	// Slider drags redraw at most once a frame.
	function drawGridSoon() {
		if (gridPending) return;
		gridPending = requestAnimationFrame(function() { gridPending = null; drawGrid(); });
	}
	drawGrid();

	// ---- Map settings sheet (owners). Every change previews on the map
	// behind it straight away; Save sends the changed groups in one PUT
	// (display_settings is partial per group), Cancel puts the map back. ----
	var sheet = $id('mp-sheet');
	var gear = $id('mp-settings-btn');
	var mapContainerEl = $id('map-container');
	var themeBg = mapContainerEl.style.backgroundColor;
	function cloneD(o) { return JSON.parse(JSON.stringify(o)); }

	// What each display_settings group looks like for a given resolved state.
	// Values equal to the default are stored as nothing by the server, so
	// these send the person's choice as it is.
	function groupsOf(R) {
		return {
			frame: { style: R.frame_source === 'map' ? R.frame : '', tint: R.tint },
			pins: { style: R.pin_style, size: R.pin_size, labels: R.pin_labels },
			kinds: (function() {
				var o = {};
				R.kinds.forEach(function(k) { o[k.id] = { label: k.label, color: k.color }; });
				return o;
			})(),
			grid: { type: R.grid_type, size: R.grid_size, strength: R.grid_strength },
			open: R.open_mode === 'spot'
				? { mode: 'spot', x: R.open_x, y: R.open_y, zoom: R.open_zoom }
				: { mode: R.open_mode },
			draw: { who: R.draw_who }
		};
	}

	function syncKindSelects() {
		var sel = document.getElementById('mk-kind');
		if (!sel) return;
		KINDS.forEach(function(k) {
			for (var i = 0; i < sel.options.length; i++) {
				if (sel.options[i].value === k.id) sel.options[i].textContent = k.label;
			}
		});
	}
	syncKindSelects();

	// Push D onto the live map: frame, tint, pins, kinds and grid.
	function applyDisplay() {
		if (frameEl) {
			frameEl.setAttribute('data-frame', D.frame);
			frameEl.setAttribute('data-tint', D.tint ? '1' : '0');
		}
		refreshPins();
		syncKindSelects();
		renderPanel();
		drawGridSoon();
	}

	if (sheet) {
		var sInputs = {
			name: $id('ms-name'), desc: $id('ms-description'),
			tint: $id('ms-tint'), gridSize: $id('ms-grid-size'), gridStrength: $id('ms-grid-strength')
		};
		var bgPicker = $id('ms-background-color');
		var bgFlag = $id('ms-background-color-set');
		var imgChanged = false;
		var origName = sInputs.name.value;
		var origDesc = sInputs.desc.value;

		function viewNow() {
			var c = latLngToPercent(map.getCenter());
			return { x: c.x, y: c.y, zoom: map.getZoom() - fitZoom() };
		}
		function useCurrentView() {
			var v = viewNow();
			D.open_x = v.x; D.open_y = v.y; D.open_zoom = v.zoom;
			syncSheet();
		}

		// Reflect D into the controls.
		function syncSheet() {
			sheet.querySelectorAll('.mp-chip').forEach(function(b) {
				b.setAttribute('aria-pressed', String(D[b.dataset.k]) === b.dataset.v ? 'true' : 'false');
			});
			$id('ms-frame-pick').hidden = D.frame_source !== 'map';
			$id('ms-frame-note').hidden = D.frame_source === 'map';
			sInputs.tint.checked = !!D.tint;
			sInputs.gridSize.value = D.grid_size;
			sInputs.gridStrength.value = D.grid_strength;
			var noGrid = D.grid_type === 'none';
			sInputs.gridSize.disabled = noGrid;
			sInputs.gridStrength.disabled = noGrid;
			D.kinds.forEach(function(k) {
				var c = sheet.querySelector('[data-kc="' + k.id + '"]');
				var n = sheet.querySelector('[data-kn="' + k.id + '"]');
				if (c && c.value !== k.color) c.value = k.color;
				if (n && n.value !== k.label && document.activeElement !== n) n.value = k.label;
			});
			$id('ms-spot').hidden = D.open_mode !== 'spot';
			$id('ms-spot-note').textContent = 'Centre ' + Math.round(D.open_x) + '% across, ' + Math.round(D.open_y) + '% down, at '
				+ Math.round(Math.pow(2, D.open_zoom) * 100) + '% zoom.';
		}

		// Segmented controls.
		sheet.addEventListener('click', function(e) {
			var b = e.target.closest ? e.target.closest('.mp-chip') : null;
			if (!b || !sheet.contains(b)) return;
			var k = b.dataset.k, v = b.dataset.v;
			if (k === 'frame_source') {
				D.frame_source = v;
				if (v === 'campaign') D.frame = D.campaign_frame;
			} else if (k === 'frame') {
				D.frame = v;
				D.frame_source = 'map';
			} else if (k === 'open_mode') {
				// Choosing a spot starts from where the map is now.
				var wasSpot = D.open_mode === 'spot';
				D.open_mode = v;
				if (v === 'spot' && !wasSpot) { var vw = viewNow(); D.open_x = vw.x; D.open_y = vw.y; D.open_zoom = vw.zoom; }
			} else {
				D[k] = v;
			}
			syncSheet();
			applyDisplay();
		});
		sInputs.tint.addEventListener('change', function() { D.tint = this.checked; applyDisplay(); });
		sInputs.gridSize.addEventListener('input', function() {
			D.grid_size = parseInt(this.value, 10);
			if (D.grid_type === 'none') { D.grid_type = 'square'; syncSheet(); }
			applyDisplay();
		});
		sInputs.gridStrength.addEventListener('input', function() {
			D.grid_strength = parseInt(this.value, 10);
			if (D.grid_type === 'none') { D.grid_type = 'square'; syncSheet(); }
			applyDisplay();
		});
		sheet.querySelectorAll('[data-kc]').forEach(function(i) {
			i.addEventListener('input', function() {
				D.kinds.forEach(function(k) { if (k.id === i.dataset.kc) k.color = i.value; });
				applyDisplay();
			});
		});
		sheet.querySelectorAll('[data-kn]').forEach(function(i) {
			i.addEventListener('input', function() {
				D.kinds.forEach(function(k) {
					if (k.id === i.dataset.kn) { k.label = i.value.trim() || savedLabel(k.id); k.one = k.label; }
				});
				KINDS.forEach(function(k) { if (k.id === i.dataset.kn) { k.label = i.value.trim() || savedLabel(k.id); k.one = k.label; } });
				applyDisplay();
			});
		});
		// A blank name falls back to the last saved one, never to nothing.
		function savedLabel(id) {
			var out = id;
			savedD.kinds.forEach(function(k) { if (k.id === id) out = k.label; });
			return out;
		}
		$id('ms-use-view').addEventListener('click', useCurrentView);

		// Background colour: untouched / chosen / reset to theme.
		bgPicker.addEventListener('input', function() { bgFlag.value = 'true'; mapContainerEl.style.backgroundColor = bgPicker.value; });
		$id('ms-bg-reset').addEventListener('click', function() { bgFlag.value = 'false'; mapContainerEl.style.backgroundColor = ''; });

		// Picture: an upload, or one already in the campaign (media picker).
		function setPicture(id, url) {
			$id('ms-image-id').value = id;
			var preview = $id('ms-image-preview');
			preview.src = url;
			preview.hidden = false;
			imgChanged = true;
			var img = new Image();
			img.onload = function() {
				$id('ms-image-width').value = img.naturalWidth;
				$id('ms-image-height').value = img.naturalHeight;
			};
			img.src = url;
		}
		sheet.addEventListener('media-picker:select', function(ev) {
			var d = ev.detail || {};
			if (!d.id || !d.url) return;
			setPicture(d.id, d.url);
			Chronicle.notify('Picture chosen. Save to apply it.', 'success');
		});
		// The picker's slide-out lives outside the full-screen element.
		$id('ms-pick-existing').addEventListener('click', leaveFullscreen);
		$id('ms-image-file').addEventListener('change', async function() {
			if (!this.files || !this.files[0]) return;
			var fd = new FormData();
			fd.append('file', this.files[0]);
			fd.append('campaign_id', campaignID);
			fd.append('usage_type', 'attachment');
			try {
				var resp = await Chronicle.apiFetch('/media/upload', { method: 'POST', body: fd });
				if (resp.ok) {
					var data = await resp.json();
					setPicture(data.id, data.url);
					Chronicle.notify('Picture uploaded. Save to apply it.', 'success');
				} else {
					// Surface the server's specific reason (too large, type not
					// allowed, quota) instead of a generic failure.
					var errData = await resp.json().catch(function() { return {}; });
					Chronicle.notify(errData.message || ('Upload failed (HTTP ' + resp.status + ')'), 'error');
				}
			} catch (err) {
				Chronicle.notify('Network error during upload: ' + err.message, 'error');
			}
		});

		// Save: only what changed. name is required by the endpoint (it
		// rejects a blank one) so it always travels; everything else is
		// sent only if the person touched it.
		async function saveSheet() {
			var name = sInputs.name.value.trim();
			if (!name) { sInputs.name.focus(); Chronicle.notify('Give the map a name', 'error'); return; }
			var body = { name: name };
			if (sInputs.desc.value !== origDesc) body.description = sInputs.desc.value || null;
			if (imgChanged) {
				body.image_id = $id('ms-image-id').value;
				body.image_width = parseInt($id('ms-image-width').value, 10) || 0;
				body.image_height = parseInt($id('ms-image-height').value, 10) || 0;
			}
			if (bgFlag.value === 'true') body.background_color = bgPicker.value;
			else if (bgFlag.value === 'false') body.background_color = '';
			var now = groupsOf(D), was = groupsOf(savedD), ds = {}, any = false;
			Object.keys(now).forEach(function(g) {
				if (JSON.stringify(now[g]) !== JSON.stringify(was[g])) { ds[g] = now[g]; any = true; }
			});
			if (any) body.display_settings = ds;
			var btn = $id('ms-save');
			btn.disabled = true;
			try {
				var resp = await Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID, { method: 'PUT', body: body });
				if (resp.ok) { reloadView(); return; }
				var data = await resp.json().catch(function() { return {}; });
				Chronicle.notify(data.message || 'Failed to save settings', 'error');
			} catch (err) {
				Chronicle.notify('Network error: ' + err.message, 'error');
			}
			btn.disabled = false;
		}
		$id('ms-save').addEventListener('click', saveSheet);

		$id('ms-delete').addEventListener('click', async function() {
			if (!confirm('Delete this map and all its markers? This cannot be undone.')) return;
			try {
				var resp = await Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID, { method: 'DELETE' });
				if (resp.ok) { window.location.href = '/campaigns/' + campaignID + '/maps'; return; }
				var delData = await resp.json().catch(function() { return {}; });
				Chronicle.notify(delData.message || ('Failed to delete map (HTTP ' + resp.status + ')'), 'error');
			} catch (err) {
				Chronicle.notify('Network error during delete: ' + err.message, 'error');
			}
		});

		// Open and close. Closing without saving puts every previewed
		// change back, including the map picture's background colour.
		function openSheet() {
			leaveFullscreen();
			closePanel();
			D = cloneD(savedD);
			KINDS.forEach(function(k) { D.kinds.forEach(function(d) { if (d.id === k.id) { k.label = d.label; k.one = d.label; k.color = d.color; } }); });
			syncSheet();
			sheet.hidden = false;
			gear.setAttribute('aria-expanded', 'true');
			sInputs.name.focus();
		}
		function closeSheet(revert) {
			if (sheet.hidden) return false;
			sheet.hidden = true;
			gear.setAttribute('aria-expanded', 'false');
			if (revert) {
				D = cloneD(savedD);
				KINDS.forEach(function(k) { D.kinds.forEach(function(d) { if (d.id === k.id) { k.label = d.label; k.one = d.label; k.color = d.color; } }); });
				applyDisplay();
				sInputs.name.value = origName;
				sInputs.desc.value = origDesc;
				mapContainerEl.style.backgroundColor = themeBg;
				bgFlag.value = bgFlag.defaultValue;
				bgPicker.value = bgPicker.defaultValue;
			}
			return true;
		}
		window.__mpCloseSheet = function() { return closeSheet(true); };
		gear.addEventListener('click', function() { if (sheet.hidden) openSheet(); else closeSheet(true); });
		$id('mp-sheet-x').addEventListener('click', function() { closeSheet(true); });
		$id('ms-cancel').addEventListener('click', function() { closeSheet(true); });
		syncSheet();
	}

	// ---- Search ----
	(function() {
		var input = $id('marker-search');
		var results = $id('mp-results');
		function hideResults() { results.hidden = true; }
		function showResults() {
			var q = input.value.toLowerCase().trim();
			// Dim non-matches on the map as the person types.
			markers.forEach(function(mk) {
				var lm = leafletMarkers[mk.id];
				if (!lm) return;
				var match = !q || (mk.name || '').toLowerCase().indexOf(q) !== -1 || (mk.description || '').toLowerCase().indexOf(q) !== -1;
				lm.setOpacity(match ? 1 : 0.15);
			});
			if (!q) { hideResults(); return; }
			var hits = markers.filter(function(mk) {
				return pinShown(mk) && ((mk.name || '').toLowerCase().indexOf(q) !== -1 || (mk.description || '').toLowerCase().indexOf(q) !== -1);
			}).slice(0, 8);
			var html = '';
			hits.forEach(function(mk) {
				var k = kindInfo(mk.pin_category);
				html += '<button type="button" data-id="' + escapeAttr(mk.id) + '">' + dot(k.color) + '<span class="mp-pl-name">' + escapeHtml(mk.name) + '</span><small>' + escapeHtml(k.one) + '</small></button>';
			});
			results.innerHTML = html || '<p class="mp-empty">No pin called that</p>';
			results.hidden = false;
		}
		function pick(id) {
			var mk = markers.filter(function(m) { return m.id === id; })[0];
			if (!mk) return;
			input.value = mk.name;
			showResults();
			hideResults();
			flyToMarker(mk);
		}
		input.addEventListener('input', showResults);
		input.addEventListener('keydown', function(e) {
			if (e.key === 'Enter') {
				var first = results.querySelector('button');
				if (first) { e.preventDefault(); pick(first.dataset.id); }
			}
		});
		results.addEventListener('click', function(e) {
			var b = e.target.closest('button');
			if (b) pick(b.dataset.id);
		});
		listen(document, 'pointerdown', function(e) {
			if (!e.target.closest || !e.target.closest('.mp-search')) hideResults();
		});
		window.__mpCloseResults = function() {
			if (results.hidden) return false;
			hideResults();
			return true;
		};
	})();

	// ---- Pin placement ----
	// Coordinate helper: Leaflet latlng (CRS.Simple, origin bottom-left)
	// → marker percentage coords (origin top-left), clamped to 0-100.
	function latLngToPercent(latlng) {
		var x = Math.max(0, Math.min(100, (latlng.lng / w) * 100));
		var y = Math.max(0, Math.min(100, ((h - latlng.lat) / h) * 100));
		return { x: x, y: y };
	}

	// The Pin tool: click the map to drop a pin and name it in a card.
	map.on('click', function(e) {
		if (tool !== 'pin' || !isScribe) return;
		var p = latLngToPercent(e.latlng);
		startDraft(p.x, p.y);
	});

	// Double-click drops a pin without picking the tool (Scribe+). Skipped
	// while a drawing tool is active: the polygon tool finishes on
	// double-click and must not also leave a pin behind.
	map.on('dblclick', function(e) {
		if (!isScribe || tool === 'draw') return;
		var p = latLngToPercent(e.latlng);
		startDraft(p.x, p.y);
	});

	// ---- Keys: V move, P pin, D draw, Esc steps back ----
	function typing(t) {
		if (!t || !t.tagName) return false;
		var n = t.tagName;
		return n === 'INPUT' || n === 'TEXTAREA' || n === 'SELECT' || t.isContentEditable;
	}
	function modalOpen() {
		var m = document.getElementById('marker-modal');
		return m && !m.classList.contains('hidden');
	}
	function onKey(e) {
		// The page may have been swapped away; stop listening if so.
		if (!map.getContainer().isConnected) { document.removeEventListener('keydown', onKey); return; }
		if (destroyed) return;
		if (e.ctrlKey || e.metaKey || e.altKey || modalOpen()) return;
		if (e.key === 'Escape') {
			// Close the nearest thing first: results, popover, panel, card, then the tool.
			// mpConsumed tells a host that wraps the viewer (the focus view) that
			// this Esc closed something inside the map, so it must not also close
			// itself.
			e.mpConsumed = true;
			if (window.__mpCloseResults && window.__mpCloseResults()) return;
			if (closePopovers()) return;
			if (window.__mpCloseSheet && window.__mpCloseSheet()) return;
			if (closePanel()) return;
			if (map._popup && map._popup.isOpen && map._popup.isOpen()) { map.closePopup(); return; }
			if (tool !== 'move') { setTool('move'); return; }
			e.mpConsumed = false;
			return;
		}
		// Letters inside the settings sheet belong to its controls.
		if (!isScribe || typing(e.target) || (e.target.closest && e.target.closest('#mp-sheet'))) return;
		var k = e.key.toLowerCase();
		if (k === 'v') { setTool('move'); e.preventDefault(); }
		else if (k === 'p') { setTool('pin'); e.preventDefault(); }
		else if (k === 'd' && canDraw) { openPopover(flyDraw, document.querySelector('[data-tool="draw"]')); e.preventDefault(); }
	}
	listen(document, 'keydown', onKey);

	// Create marker modal handlers (the full editor behind "More options").
	window.openCreateMarker = function(x, y) {
		resetMarkerModal();
		document.getElementById('mk-x').value = x.toFixed(2);
		document.getElementById('mk-y').value = y.toFixed(2);
		document.getElementById('marker-modal').classList.remove('hidden');
	};

	window.openEditMarker = function(mk) {
		resetMarkerModal();
		document.getElementById('mk-id').value = mk.id;
		document.getElementById('mk-name').value = mk.name || '';
		document.getElementById('mk-description').value = mk.description || '';
		document.getElementById('mk-x').value = mk.x.toFixed(2);
		document.getElementById('mk-y').value = mk.y.toFixed(2);
		document.getElementById('mk-icon').value = mk.icon || 'fa-map-pin';
		document.getElementById('mk-color').value = mk.color || '#3b82f6';
		document.getElementById('mk-visibility').value = mk.visibility || 'everyone';
		var kindSel = document.getElementById('mk-kind');
		// A stored value outside the five kinds shows as "Other" and is only
		// rewritten if the person picks something else.
		kindSel.value = KINDS.some(function(k) { return k.id === mk.pin_category; }) ? mk.pin_category : '';
		kindSel.dataset.original = kindSel.value;
		var entityInput = document.getElementById('mk-entity-id');
		entityInput.value = mk.entity_id || '';
		entityInput.dataset.original = entityInput.value;

		document.getElementById('mk-modal-title').textContent = 'Edit Marker';
		document.getElementById('mk-submit-icon').className = 'fa-solid fa-check mr-1';
		document.getElementById('mk-submit-text').textContent = 'Save Changes';
		// Same rule as the pin card's Remove: owners any marker, a scribe
		// only their own; the server enforces it either way.
		var canRemove = isOwner || (isScribe && userID && mk.created_by === userID);
		document.getElementById('mk-delete-zone').classList.toggle('hidden', !canRemove);
		document.getElementById('marker-modal').classList.remove('hidden');
	};

	window.closeMarkerModal = function() {
		document.getElementById('marker-modal').classList.add('hidden');
		hideMarkerDeleteConfirm();
	};

	function resetMarkerModal() {
		var form = document.getElementById('marker-form');
		form.reset();
		document.getElementById('mk-id').value = '';
		document.getElementById('mk-icon').value = 'fa-map-pin';
		document.getElementById('mk-color').value = '#3b82f6';
		document.getElementById('mk-kind').dataset.original = '';
		document.getElementById('mk-modal-title').textContent = 'New Marker';
		document.getElementById('mk-submit-icon').className = 'fa-solid fa-plus mr-1';
		document.getElementById('mk-submit-text').textContent = 'Create Marker';
		document.getElementById('mk-delete-zone').classList.add('hidden');
		hideMarkerDeleteConfirm();
	}

	// Choosing a kind for a NEW pin in the full editor sets its colour too.
	var kindSel = document.getElementById('mk-kind');
	if (kindSel) kindSel.addEventListener('change', function() {
		var creating = !document.getElementById('mk-id').value;
		if (creating && this.value) document.getElementById('mk-color').value = kindInfo(this.value).color;
	});

	// Form submit — create or update.
	// The modal markup is Scribe+ only; players have no form to wire, and a
	// throw here would skip publishing chronicleMap below.
	var markerForm = document.getElementById('marker-form');
	if (markerForm) markerForm.addEventListener('submit', async function(e) {
		e.preventDefault();
		var fd = new FormData(e.target);
		var body = {
			name: fd.get('name'),
			x: parseFloat(fd.get('x')),
			y: parseFloat(fd.get('y')),
			icon: fd.get('icon') || 'fa-map-pin',
			color: fd.get('color') || '#3b82f6',
			visibility: fd.get('visibility') || 'everyone',
		};
		// Send an explicit null when the operator EMPTIED a field.
		// Under the partial-update contract null clears and absent
		// preserves, so omitting an emptied description or entity
		// link would silently bring the old value back. Everything
		// the form does not own — pin_category, visibility_rules and
		// the Foundry pairing key foundry_id — stays absent, which
		// now preserves it instead of erasing it.
		body.description = fd.get('description') || null;

		var markerID = fd.get('marker_id');
		// Kind (pin_category): a new marker sends it when chosen; an edit
		// sends it only when it changed, with null for "Other" (clear).
		var kindVal = fd.get('pin_category') || '';
		var kindOriginal = document.getElementById('mk-kind').dataset.original || '';
		if (!markerID ? kindVal !== '' : kindVal !== kindOriginal) {
			body.pin_category = kindVal || null;
		}
		// An edit sends the entity link only when it changed: a link
		// to a page this viewer can't see arrives blank, and echoing
		// that blank back as null would unlink it.
		var entityVal = fd.get('entity_id') || '';
		var entityOriginal = document.getElementById('mk-entity-id').dataset.original || '';
		if (!markerID || entityVal !== entityOriginal) {
			body.entity_id = entityVal || null;
		}
		var url, method;
		if (markerID) {
			url = '/campaigns/' + campaignID + '/maps/' + mapID + '/markers/' + markerID;
			method = 'PUT';
		} else {
			url = '/campaigns/' + campaignID + '/maps/' + mapID + '/markers';
			method = 'POST';
		}

		try {
			var resp = await Chronicle.apiFetch(url, {
				method: method, body: body,
			});
			if (resp.ok) {
				closeMarkerModal();
				reloadView();
			} else {
				var data = await resp.json().catch(function() { return {}; });
				Chronicle.notify(data.message || 'Failed to save marker', 'error');
			}
		} catch (err) {
			Chronicle.notify('Network error: ' + err.message, 'error');
		}
	});

	// Drag-end position update (silent PUT).
	async function updateMarkerPosition(mk) {
		try {
			// A drag only moves the marker. Updates are partial (absent
			// keys are preserved), so the body carries just {x, y} and
			// never echoes fields this page may hold stale copies of.
			await Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/markers/' + mk.id, {
				method: 'PUT',
				body: { x: mk.x, y: mk.y },
			});
		} catch (err) {
			console.error('Failed to save marker position:', err);
		}
	}

	// Delete confirmation.
	window.showMarkerDeleteConfirm = function() {
		document.getElementById('mk-delete-confirm').classList.remove('hidden');
		document.getElementById('marker-form').classList.add('hidden');
	};

	window.hideMarkerDeleteConfirm = function() {
		document.getElementById('mk-delete-confirm').classList.add('hidden');
		document.getElementById('marker-form').classList.remove('hidden');
	};

	window.deleteMarker = async function() {
		var markerID = document.getElementById('mk-id').value;
		if (!markerID) return;

		try {
			var resp = await Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/markers/' + markerID, {
				method: 'DELETE',
			});
			if (resp.ok) {
				closeMarkerModal();
				reloadView();
			} else {
				var data = await resp.json().catch(function() { return {}; });
				Chronicle.notify(data.message || 'Failed to delete marker', 'error');
			}
		} catch (err) {
			Chronicle.notify('Network error: ' + err.message, 'error');
		}
	};

	// Expose map context for drawing tools.
	window.chronicleMap = {
		map: map,
		campaignID: campaignID,
		mapID: mapID,
		imageW: w,
		imageH: h,
		isScribe: isScribe,
		// canDraw is what the drawing module builds its tools from; the
		// server enforces the map's "who can draw" rule regardless.
		canDraw: canDraw,
		isOwner: isOwner,
		// Callbacks the drawing module calls so the panel counts and the
		// undo button stay in step with what is on the map.
		onDrawingsChange: function(n) {
			drawingCount = n;
			if (!showDrawings && draw()) draw().setVisible(false);
			renderPanel();
		},
		onUndoChange: function(n) { if (window.__mpUndoChange) window.__mpUndoChange(n); }
	};

	// Tear down a mount so a host that re-creates the viewer (the focus view,
	// opened and closed over a page that stays loaded) leaks nothing: Leaflet's
	// own listeners and layers, this mount's document listeners, and the
	// context the drawing module reads.
	var ctx = window.chronicleMap;
	return {
		map: map,
		ctx: ctx,
		destroy: function() {
			if (destroyed) return;
			destroyed = true;
			clearTimeout(viewTimer);
			cleanups.forEach(function(fn) { fn(); });
			leaveFullscreen();
			try { map.remove(); } catch (e) { /* container already gone */ }
			if (window.chronicleMap === ctx) window.chronicleMap = null;
		}
	};

  }

  // open resolves to a handle with destroy(), once the libraries are in and the
  // viewer is built. Mounting twice on one element returns the first handle.
  function open(cfgEl, opts) {
    if (cfgEl.__mapViewer) return cfgEl.__mapViewer;
    var d = cfgEl.dataset;
    var p = ensureLeaflet({ leafletSrc: d.leafletSrc, clusterSrc: d.clusterSrc })
      .then(function () {
        if (cfgEl.__mapViewerDestroyed) return null;
        var handle = mountViewer(cfgEl, opts);
        cfgEl.__mapViewerHandle = handle;
        return ensureDrawing({ drawSrc: d.drawSrc }).then(function () {
          // The drawing module only polls once, at load; later mounts are
          // started explicitly.
          if (!cfgEl.__mapViewerDestroyed && window.ChronicleMapDrawing && handle.ctx) {
            // Drawings are an add-on: if they fail the map still works.
            try { window.ChronicleMapDrawing.init(handle.ctx); } catch (err) { console.error('[map-viewer] drawing tools failed:', err); }
          }
          return handle;
        });
      });
    cfgEl.__mapViewer = p;
    return p;
  }

  function destroy(cfgEl) {
    cfgEl.__mapViewerDestroyed = true;
    if (cfgEl.__mapViewerHandle) cfgEl.__mapViewerHandle.destroy();
    cfgEl.__mapViewerHandle = null;
    cfgEl.__mapViewer = null;
  }

  window.ChronicleMapViewer = { open: open, destroy: destroy };

  if (window.Chronicle && Chronicle.register) {
    Chronicle.register('map-viewer', {
      init: function (el) {
        open(el).catch(function (err) {
          console.error('[map-viewer] failed to start:', err);
          if (Chronicle.notify) Chronicle.notify('The map could not be loaded.', 'error');
        });
      },
      destroy: function (el) { destroy(el); },
    });
  }
})();
