/**
 * map_drawing_tools.js -- Chronicle Map Drawing Tools
 *
 * Renders saved drawings for everyone and, for Scribe+, provides the drawing
 * tools (freehand, rectangle, polygon, ellipse, text) on Leaflet's native APIs
 * (no Leaflet.Draw dependency). It owns no UI: the map page's floating tool rail
 * drives it through `window.chronicleMap.draw`, so there is one control surface.
 *
 * Expects a global `window.chronicleMap` object set by the map show page:
 *   { map, campaignID, mapID, imageW, imageH, isScribe,
 *     onDrawingsChange(count), onUndoChange(count) }
 * and publishes `window.chronicleMap.draw` =
 *   { start(shape), cancel(), setStyle({color,width}), undo(), setVisible(bool), count() }
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

  function initDrawingTools(ctx) {
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

      return Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/drawings', {
        method: 'POST',
        body: body
      }).then(function (res) {
        if (!res.ok) {
          Chronicle.notify('Failed to save drawing', 'error');
          return null;
        }
        return res.json();
      });
    }

    function deleteDrawing(drawingID) {
      return Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/drawings/' + drawingID, {
        method: 'DELETE'
      });
    }

    function loadDrawings() {
      Chronicle.apiFetch('/campaigns/' + campaignID + '/maps/' + mapID + '/drawings')
        .then(function (res) { return res.ok ? res.json() : []; })
        .then(function (drawings) {
          if (!drawings || !Array.isArray(drawings)) return;
          drawingLayer.clearLayers();
          layersByID = {};
          drawingCount = 0;
          drawings.forEach(function (d) { renderDrawing(d); });
          notifyCount();
        });
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
      sessionStack.push(d.id);
      notifyCount();
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
        // Drawing deletion is Owner-only on the server; say so rather than
        // silently leaving the shape on the map.
        var msg = res && res.status === 403
          ? 'Only owners can delete drawings'
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
      text: startText
    };

    ctx.draw = {
      start: function (shape) {
        if (isScribe && starters[shape]) starters[shape]();
      },
      cancel: cancelTool,
      setStyle: function (st) {
        if (st.color) drawColor = st.color;
        if (st.width) drawWidth = st.width;
      },
      undo: undoLast,
      setVisible: function (on) {
        if (on) map.addLayer(drawingLayer); else map.removeLayer(drawingLayer);
      },
      count: function () { return drawingCount; }
    };

    // --- Init ---

    loadDrawings();
  }
})();
