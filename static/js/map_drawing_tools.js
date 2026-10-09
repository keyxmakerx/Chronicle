/**
 * map_drawing_tools.js -- Chronicle Map Drawing Tools
 *
 * Renders saved drawings for everyone and, for Scribe+, provides the drawing
 * tools (freehand, rectangle, polygon, ellipse, text, shadow) and pictures on Leaflet's native APIs
 * (no Leaflet.Draw dependency). It owns no UI: the map page's floating tool rail
 * drives it through `window.chronicleMap.draw`, so there is one control surface.
 *
 * Expects a global `window.chronicleMap` object set by the map show page:
 *   { map, campaignID, mapID, imageW, imageH, isScribe,
 *     onDrawingsChange(count), onUndoChange(count) }
 * and publishes `window.chronicleMap.draw` =
 *   { start(shape), cancel(), setStyle({color,width}), setShadowStrength(alpha), undo(),
 *     setVisible(bool), count(), addPicture({id}), pictureSelectMode(bool),
 *     escape(), deleteSelected() }
 * and `window.chronicleMap.pictures` (when the picture module is present), which
 * the hex layer reads: { ready(), get(id), list(), subscribe(fn), straighten(id) }.
 *
 * Drawings are persisted via the REST API:
 *   POST /campaigns/:id/maps/:mid/drawings
 *   PUT  /campaigns/:id/maps/:mid/drawings/:did
 *   DELETE /campaigns/:id/maps/:mid/drawings/:did
 *
 * Coordinate system: percentage-based (0-100) in API, pixel-based in Leaflet.
 */
(function () {
  'use strict';

  // Wait for the map to be initialized.
  var checkInterval = setInterval(function () {
    if (!window.chronicleMap) return;
    clearInterval(checkInterval);
    initDrawingTools(window.chronicleMap);
  }, 100);

  // Auto-clear after 10s if map never initializes.
  setTimeout(function () { clearInterval(checkInterval); }, 10000);

  // The script loads once per page but a viewer can be mounted more than once
  // (the focus view over an entity page opens and closes), so a host calls
  // init for each mount. Both this and the load-time poll above may reach the
  // same context, hence the once-per-context guard.
  window.ChronicleMapDrawing = { init: initDrawingTools };

  function initDrawingTools(ctx) {
    if (!ctx || ctx.drawingInitialised) return;
    ctx.drawingInitialised = true;
    var map = ctx.map;
    var campaignID = ctx.campaignID;
    var mapID = ctx.mapID;
    var w = ctx.imageW;
    var h = ctx.imageH;
    // canDraw is the page's per-map draw gate; isScribe is the fallback for
    // embeds that predate it.
    var isScribe = ctx.canDraw !== undefined ? ctx.canDraw : ctx.isScribe;

    if (!map) return;

    var activeTool = null;
    var drawingLayer = L.layerGroup().addTo(map);
    // Shadowed areas are drawn by their own module (smoke, motion rules). It is
    // optional: without it a shadow is simply not drawn, and the server still
    // withholds what lies under it.
    var shadows = window.ChronicleMapShadow ? window.ChronicleMapShadow.attach(map, { toLatLng: toLatLng }) : null;
    // Pictures placed on the map (drawings of type "image"), likewise optional.
    // The registry of pictures the hex layer reads (ctx.pictures). It lives
    // here, not in the picture module, because a picture leaves the map layer
    // while drawings are hidden, and the hexes pinned to it must not go with it.
    var picReg = {};
    var picLive = {};
    var picSubs = [];
    var picSeq = 0;
    var picReady = false;
    function picNotify(kind, id) {
      picSubs.slice().forEach(function (fn) { try { fn(kind, id); } catch (e) { console.error('[map-drawing] picture listener failed:', e); } });
    }
    function pictureChanged(kind, id, points) {
      if (kind === 'geo') picLive[id] = points; else delete picLive[id];
      picNotify(kind, id);
    }
    var pictures = window.ChronicleMapPictures ? window.ChronicleMapPictures.attach(map, {
      mapW: w,
      mapH: h,
      toLatLng: toLatLng,
      // The server signs each picture's URL for this viewer; the bare path is
      // only a fallback for a response from an older server.
      mediaURL: function (d) { return d.image_url || (d.image_id ? '/media/' + encodeURIComponent(d.image_id) : ''); },
      // Signed URLs expire, so a failed load asks the server for a new one.
      refreshURL: function (d) {
        return Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/drawings/' + d.id)
          .then(function (res) { return res.ok ? res.json() : null; })
          .then(function (fresh) { return fresh && fresh.image_url ? fresh.image_url : ''; });
      },
      canEdit: !!isScribe,
      // The server's delete rule: owners and DM access any picture, a scribe
      // the ones they added.
      canDelete: function (d) {
        return !!(ctx.isOwner || ctx.canDmOnly || (ctx.userID && d.created_by === ctx.userID));
      },
      onPatch: patchDrawing,
      onDelete: confirmDelete,
      onChange: pictureChanged,
      hexCover: ctx.hexCover
    }) : null;
    if (pictures) {
      // What the hex layer needs to know about pictures: which exist, where they
      // are right now (live while one is dragged), and when that changes.
      ctx.pictures = {
        ready: function () { return picReady; },
        get: function (id) {
          var d = picReg[id];
          if (!d) return null;
          return { id: d.id, points: picLive[id] || d.points, sort_order: d.sort_order || 0,
            visibility: d.visibility, rotation: Number(d.rotation) || 0, seq: d._seq };
        },
        // Pictures in stacking order, which is how they are numbered for people.
        list: function () {
          return Object.keys(picReg).map(function (id) { return ctx.pictures.get(id); }).sort(function (a, b) {
            return a.sort_order - b.sort_order || a.seq - b.seq;
          });
        },
        subscribe: function (fn) {
          picSubs.push(fn);
          return function () { var i = picSubs.indexOf(fn); if (i !== -1) picSubs.splice(i, 1); };
        },
        // straighten shows a picture upright after the server turned it so,
        // without waiting for a reload.
        straighten: function (id) {
          var d = picReg[id];
          if (!d) return;
          d.rotation = 0;
          var layer = layersByID[id];
          if (layer && layer.reload) layer.reload();
          picNotify('patch', id);
        }
      };
      if (ctx.hexes && ctx.hexes.bindPictures) ctx.hexes.bindPictures();
    }
    ctx.onDestroy = function () {
      picSubs = [];
      if (shadows) shadows.destroy();
      if (pictures) pictures.destroy();
    };
    // How much a new shadow hides: 0.5 "A hint", 0.85 "Almost nothing".
    var shadowStrength = 0.5;
    var currentPoints = [];
    var currentShape = null;
    var drawColor = '#2563eb';
    var drawWidth = 4;
    // Layers by drawing id, and the ids created in THIS page session in order.
    // Undo only ever walks the session stack, so it can never reach back and
    // delete a drawing someone else made earlier.
    var layersByID = {};
    var sessionStack = [];
    var drawingCount = 0;

    function notifyCount() {
      if (typeof ctx.onDrawingsChange === 'function') ctx.onDrawingsChange(drawingCount);
      if (typeof ctx.onUndoChange === 'function') ctx.onUndoChange(sessionStack.length);
    }

    // --- Coordinate Conversion ---

    function toPercent(latlng) {
      return {
        x: Math.max(0, Math.min(100, (latlng.lng / w) * 100)),
        y: Math.max(0, Math.min(100, ((h - latlng.lat) / h) * 100))
      };
    }

    function toLatLng(pt) {
      return L.latLng(h - (pt.y / 100) * h, (pt.x / 100) * w);
    }

    // --- API ---

    function saveDrawing(type, points, opts) {
      opts = opts || {};
      var body = {
        drawing_type: type,
        points: points,
        stroke_color: opts.stroke_color || drawColor,
        stroke_width: opts.stroke_width || drawWidth,
        fill_color: opts.fill_color || null,
        fill_alpha: opts.fill_alpha || 0,
        visibility: 'everyone'
      };
      if (opts.text_content) body.text_content = opts.text_content;
      if (opts.image_id) body.image_id = opts.image_id;
      if (opts.sort_order) body.sort_order = opts.sort_order;

      return Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/drawings', {
        method: 'POST',
        body: body
      }).then(function (res) {
        if (!res.ok) {
          // The server says why a picture is refused (not this campaign's
          // image, too many points); show that instead of a bare failure.
          return res.json().catch(function () { return {}; }).then(function (err) {
            Chronicle.notify(err.message || 'Failed to save drawing', 'error');
            return null;
          });
        }
        return res.json();
      });
    }

    // Sends only the fields that changed (the endpoint is a partial update),
    // so a picture nudge cannot touch its opacity, crop or visibility.
    function patchDrawing(drawingID, fields) {
      return Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/drawings/' + drawingID, {
        method: 'PUT',
        body: fields
      }).then(function (res) {
        if (res.ok) return true;
        return res.json().catch(function () { return {}; }).then(function (err) {
          Chronicle.notify(err.message || 'Could not save that change', 'error');
          return false;
        });
      }).catch(function () {
        Chronicle.notify('Could not save that change', 'error');
        return false;
      });
    }

    // The same confirm the right-click delete uses; the server still decides
    // who may delete.
    function confirmDelete(id) {
      if (!confirm('Delete this drawing?')) return;
      deleteDrawing(id).then(function (res) {
        if (res && res.ok) {
          forget(id);
          notifyCount();
          return;
        }
        Chronicle.notify(res && res.status === 403 ? 'Only owners can delete drawings' : 'Could not delete that drawing', 'error');
      });
    }

    function deleteDrawing(drawingID) {
      return Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/drawings/' + drawingID, {
        method: 'DELETE'
      });
    }

    // A failed load still marks pictures ready: an anchored hex layer waits on
    // that, and would otherwise stay off for good instead of taking its
    // reload/fallback path.
    function loadFailed() {
      picReady = true;
      picNotify('list');
      Chronicle.notify('Could not load the drawings on this map', 'error');
    }

    function loadDrawings() {
      Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/drawings')
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          return res.json();
        })
        .then(function (drawings) {
          if (!drawings || !Array.isArray(drawings)) throw new Error('unexpected response');
          drawingLayer.clearLayers();
          layersByID = {};
          picReg = {};
          picLive = {};
          drawingCount = 0;
          drawings.forEach(function (d) { renderDrawing(d); });
          picReady = true;
          picNotify('list');
          notifyCount();
        })
        .catch(loadFailed);
    }

    // --- Rendering ---

    function renderDrawing(d) {
      var points = d.points || [];
      if (!points.length && d.drawing_type !== 'text') return;

      var opts = {
        color: d.stroke_color || '#000',
        weight: d.stroke_width || 2,
        fillColor: d.fill_color || d.stroke_color || '#000',
        fillOpacity: d.fill_alpha || 0,
        interactive: true
      };

      var layer;
      var latlngs = points.map(toLatLng);

      switch (d.drawing_type) {
        case 'rectangle':
          if (latlngs.length >= 2) {
            layer = L.rectangle([latlngs[0], latlngs[1]], opts);
          }
          break;
        case 'image':
          if (pictures && d.image_id) {
            layer = pictures.layer(d);
            d._seq = picSeq++;
            picReg[d.id] = d;
          }
          break;
        case 'shadow':
          // Only the owner and co-DMs see through it; scribes are hidden like players.
          if (shadows) layer = shadows.add(d, !!ctx.canShadow);
          break;
        case 'ellipse':
          if (latlngs.length >= 2) {
            var center = latlngs[0];
            var edge = latlngs[1];
            var radius = center.distanceTo(edge);
            layer = L.circle(center, Object.assign({}, opts, { radius: radius }));
          }
          break;
        case 'polygon':
          if (latlngs.length >= 3) {
            layer = L.polygon(latlngs, opts);
          }
          break;
        case 'text':
          if (latlngs.length >= 1 && d.text_content) {
            layer = L.marker(latlngs[0], {
              icon: L.divIcon({
                className: 'map-text-annotation',
                html: '<span style="color:' + Chronicle.escapeAttr(d.stroke_color || '#fff') +
                  ';font-size:' + (d.font_size || 14) + 'px;text-shadow:0 1px 3px rgba(0,0,0,0.7);white-space:nowrap;">' +
                  Chronicle.escapeHtml(d.text_content) + '</span>',
                iconSize: null,
                iconAnchor: [0, 0]
              }),
              interactive: true
            });
          }
          break;
        default: // freehand / line
          if (latlngs.length >= 2) {
            layer = L.polyline(latlngs, opts);
          }
      }

      if (layer) {
        layer._drawingID = d.id;
        layer._drawingData = d;

        // Right-click to delete (Scribe+ sees the prompt; the server still
        // decides who may delete).
        if (isScribe) {
          layer.on('contextmenu', function (e) {
            L.DomEvent.stopPropagation(e);
            if (confirm('Delete this drawing?')) {
              deleteDrawing(d.id).then(function (res) {
                if (res && res.ok) {
                  forget(d.id);
                  notifyCount();
                }
              });
            }
          });
        }

        layersByID[d.id] = layer;
        drawingCount++;
        drawingLayer.addLayer(layer);
      }
    }

    // Drops a drawing from the map and from the bookkeeping once it is gone
    // server-side.
    function forget(id) {
      var layer = layersByID[id];
      if (picReg[id]) { delete picReg[id]; delete picLive[id]; picNotify('list', id); }
      if (layer) {
        drawingLayer.removeLayer(layer);
        delete layersByID[id];
        drawingCount = Math.max(0, drawingCount - 1);
      }
      var i = sessionStack.indexOf(id);
      if (i !== -1) sessionStack.splice(i, 1);
    }

    // A freshly saved drawing: render it and remember it for undo.
    function addSaved(d) {
      if (!d) return;
      renderDrawing(d);
      if (picReg[d.id]) picNotify('list', d.id);
      sessionStack.push(d.id);
      notifyCount();
    }

    // Drops a chosen campaign picture onto the middle of the view. The picture
    // is measured first so it keeps its proportions.
    function addPicture(pick) {
      if (!pictures || !pick || !pick.id) return;
      var probe = new Image();
      probe.onload = function () { placePicture(pick.id, probe.naturalWidth, probe.naturalHeight); };
      // A picture the browser cannot measure still goes down, as a square.
      probe.onerror = function () { placePicture(pick.id, 1, 1); };
      probe.src = pick.url || '/media/' + encodeURIComponent(pick.id);
    }

    function placePicture(imageID, natW, natH) {
      var pts = pictures.placement(natW, natH);
      // New pictures land on top of the others.
      saveDrawing('image', pts, { image_id: imageID, fill_alpha: 1, sort_order: pictures.nextSortOrder() }).then(function (d) {
        if (!d) return;
        addSaved(d);
        pictures.select(d.id);
      });
    }

    function undoLast() {
      if (!sessionStack.length) return;
      var id = sessionStack[sessionStack.length - 1];
      deleteDrawing(id).then(function (res) {
        if (res && res.ok) {
          forget(id);
          notifyCount();
          return;
        }
        // The server lets a scribe delete only drawings they created (owners
        // delete any); say so rather than silently leaving the shape on the map.
        var msg = res && res.status === 403
          ? 'You can only delete drawings you created'
          : 'Could not undo that drawing';
        Chronicle.notify(msg, 'error');
      });
    }

    // --- Drawing Handlers ---

    function startFreehand() {
      setTool('freehand');
      currentPoints = [];
      var drawing = false;

      function onMouseDown(e) {
        if (activeTool !== 'freehand') return cleanup();
        drawing = true;
        currentPoints = [toPercent(e.latlng)];
        currentShape = L.polyline([e.latlng], {
          color: drawColor, weight: drawWidth, opacity: 0.7
        }).addTo(map);
        map.dragging.disable();
      }

      function onMouseMove(e) {
        if (!drawing || !currentShape) return;
        currentPoints.push(toPercent(e.latlng));
        currentShape.addLatLng(e.latlng);
      }

      function onMouseUp() {
        if (!drawing) return;
        drawing = false;
        map.dragging.enable();
        if (currentPoints.length >= 2) {
          saveDrawing('freehand', currentPoints).then(function (d) {
            if (currentShape) map.removeLayer(currentShape);
            currentShape = null;
            addSaved(d);
          });
        } else {
          if (currentShape) map.removeLayer(currentShape);
          currentShape = null;
        }
      }

      function cleanup() {
        map.off('mousedown', onMouseDown);
        map.off('mousemove', onMouseMove);
        map.off('mouseup', onMouseUp);
        map.dragging.enable();
      }

      map.on('mousedown', onMouseDown);
      map.on('mousemove', onMouseMove);
      map.on('mouseup', onMouseUp);
      map._drawCleanup = cleanup;
    }

    function startRectangle() {
      setTool('rectangle');
      var startLL = null;
      var rect = null;

      function onClick(e) {
        if (activeTool !== 'rectangle') return cleanup();
        if (!startLL) {
          startLL = e.latlng;
          rect = L.rectangle([startLL, startLL], {
            color: drawColor, weight: drawWidth, fillOpacity: 0.1, dashArray: '5,5'
          }).addTo(map);
        } else {
          var pts = [toPercent(startLL), toPercent(e.latlng)];
          saveDrawing('rectangle', pts, { fill_color: drawColor, fill_alpha: 0.15 }).then(function (d) {
            if (rect) map.removeLayer(rect);
            addSaved(d);
            startLL = null; rect = null;
          });
        }
      }

      function onMove(e) {
        if (rect && startLL) {
          rect.setBounds([startLL, e.latlng]);
        }
      }

      function cleanup() {
        map.off('click', onClick);
        map.off('mousemove', onMove);
        if (rect) { map.removeLayer(rect); rect = null; }
      }

      map.on('click', onClick);
      map.on('mousemove', onMove);
      map._drawCleanup = cleanup;
    }

    // Hide under shadow: drag a box. Unlike the rectangle tool this is one
    // press-drag-release, which is how a person sweeps an area to cover.
    function startShadow() {
      setTool('shadow');
      var startLL = null;
      var rect = null;

      function onDown(e) {
        if (activeTool !== 'shadow') return cleanup();
        startLL = e.latlng;
        rect = L.rectangle([startLL, startLL], {
          color: '#ffffff', weight: 1.5, fillColor: '#0a0c12', fillOpacity: 0.25, dashArray: '5,5', interactive: false
        }).addTo(map);
        map.dragging.disable();
      }

      function onMove(e) {
        if (rect && startLL) rect.setBounds([startLL, e.latlng]);
      }

      function onUp(e) {
        if (!startLL) return;
        var a = toPercent(startLL);
        var b = toPercent(e.latlng);
        startLL = null;
        map.dragging.enable();
        if (rect) { map.removeLayer(rect); rect = null; }
        // A click or a sliver is not a box worth saving.
        if (Math.abs(a.x - b.x) < 1 || Math.abs(a.y - b.y) < 1) return;
        saveDrawing('shadow', [a, b], { fill_color: '#0a0c12', fill_alpha: shadowStrength, stroke_width: 1 }).then(addSaved);
      }

      function cleanup() {
        map.off('mousedown', onDown);
        map.off('mousemove', onMove);
        map.off('mouseup', onUp);
        map.dragging.enable();
        if (rect) { map.removeLayer(rect); rect = null; }
        startLL = null;
      }

      map.on('mousedown', onDown);
      map.on('mousemove', onMove);
      map.on('mouseup', onUp);
      map._drawCleanup = cleanup;
    }

    function startPolygon() {
      setTool('polygon');
      var points = [];
      var preview = null;

      function onClick(e) {
        if (activeTool !== 'polygon') return cleanup();
        points.push(e.latlng);
        if (preview) map.removeLayer(preview);
        preview = L.polygon(points, {
          color: drawColor, weight: drawWidth, fillOpacity: 0.1, dashArray: '5,5'
        }).addTo(map);
      }

      function onDblClick(e) {
        L.DomEvent.stopPropagation(e);
        if (points.length >= 3) {
          var pts = points.map(toPercent);
          saveDrawing('polygon', pts, { fill_color: drawColor, fill_alpha: 0.2 }).then(function (d) {
            if (preview) map.removeLayer(preview);
            addSaved(d);
            points = []; preview = null;
          });
        }
      }

      function cleanup() {
        map.off('click', onClick);
        map.off('dblclick', onDblClick);
        if (preview) { map.removeLayer(preview); preview = null; }
        points = [];
      }

      map.on('click', onClick);
      map.on('dblclick', onDblClick);
      map._drawCleanup = cleanup;
    }

    function startCircle() {
      setTool('ellipse');
      var center = null;
      var circ = null;

      function onClick(e) {
        if (activeTool !== 'ellipse') return cleanup();
        if (!center) {
          center = e.latlng;
          circ = L.circle(center, {
            radius: 1, color: drawColor, weight: drawWidth, fillOpacity: 0.1, dashArray: '5,5'
          }).addTo(map);
        } else {
          var pts = [toPercent(center), toPercent(e.latlng)];
          saveDrawing('ellipse', pts, { fill_color: drawColor, fill_alpha: 0.15 }).then(function (d) {
            if (circ) map.removeLayer(circ);
            addSaved(d);
            center = null; circ = null;
          });
        }
      }

      function onMove(e) {
        if (circ && center) {
          circ.setRadius(center.distanceTo(e.latlng));
        }
      }

      function cleanup() {
        map.off('click', onClick);
        map.off('mousemove', onMove);
        if (circ) { map.removeLayer(circ); circ = null; }
        center = null;
      }

      map.on('click', onClick);
      map.on('mousemove', onMove);
      map._drawCleanup = cleanup;
    }

    function startText() {
      setTool('text');

      function onClick(e) {
        if (activeTool !== 'text') return cleanup();
        var text = prompt('Enter text annotation:');
        if (!text || !text.trim()) return;
        var pt = toPercent(e.latlng);
        saveDrawing('text', [pt], { text_content: text.trim() }).then(function (d) {
          addSaved(d);
        });
      }

      function cleanup() {
        map.off('click', onClick);
      }

      map.on('click', onClick);
      map._drawCleanup = cleanup;
    }

    // --- Tool State ---

    function setTool(tool) {
      // Clean up previous tool.
      if (map._drawCleanup) {
        map._drawCleanup();
        map._drawCleanup = null;
      }
      activeTool = tool;
      map.getContainer().style.cursor = tool ? 'crosshair' : '';
    }

    function cancelTool() {
      setTool(null);
    }

    // --- Public API (driven by the page's tool rail) ---

    var starters = {
      freehand: startFreehand,
      rectangle: startRectangle,
      ellipse: startCircle,
      polygon: startPolygon,
      text: startText,
      shadow: startShadow
    };

    ctx.draw = {
      start: function (shape) {
        if (shape === 'shadow' && !ctx.canShadow) return;
        if (isScribe && starters[shape]) starters[shape]();
      },
      cancel: cancelTool,
      setStyle: function (st) {
        if (st.color) drawColor = st.color;
        if (st.width) drawWidth = st.width;
      },
      setShadowStrength: function (a) { shadowStrength = a >= 0.7 ? 0.85 : 0.5; },
      undo: undoLast,
      setVisible: function (on) {
        if (on) map.addLayer(drawingLayer); else map.removeLayer(drawingLayer);
      },
      count: function () { return drawingCount; },
      addPicture: addPicture,
      // Pictures can be picked only while the move tool is active; any other
      // tool gets every click.
      pictureSelectMode: function (on) { if (pictures) pictures.selectMode(on); },
      // Esc: true when it only deselected a picture (or left cropping).
      escape: function () { return pictures ? pictures.escape() : false; },
      // Delete key: true when a picture was selected (the confirm follows).
      deleteSelected: function () { return pictures ? pictures.deleteSelected() : false; }
    };

    // --- Init ---

    loadDrawings();
  }
})();
