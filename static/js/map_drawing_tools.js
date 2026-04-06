/**
 * map_drawing_tools.js -- Chronicle Map Drawing Tools
 *
 * Adds interactive drawing tools to the Leaflet map viewer. Uses Leaflet's
 * native APIs (no Leaflet.Draw dependency) for freehand, rectangle, polygon,
 * ellipse, and text annotation tools.
 *
 * Expects a global `window.chronicleMap` object set by the map show page:
 *   { map, campaignID, mapID, imageW, imageH, isScribe }
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
    var isScribe = ctx.isScribe;

    if (!isScribe || !map) return;

    var activeTool = null;
    var drawingLayer = L.layerGroup().addTo(map);
    var currentPoints = [];
    var currentShape = null;
    var drawColor = '#3b82f6';
    var drawWidth = 3;

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
          drawings.forEach(function (d) { renderDrawing(d); });
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

        // Right-click to delete (Scribe+).
        layer.on('contextmenu', function (e) {
          L.DomEvent.stopPropagation(e);
          if (confirm('Delete this drawing?')) {
            deleteDrawing(d.id).then(function (res) {
              if (res && res.ok) {
                drawingLayer.removeLayer(layer);
              }
            });
          }
        });

        drawingLayer.addLayer(layer);
      }
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
            if (d) renderDrawing(d);
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
            if (d) renderDrawing(d);
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
            if (d) renderDrawing(d);
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
            if (d) renderDrawing(d);
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
          if (d) renderDrawing(d);
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
      updateToolbar();
      document.getElementById('map-container').style.cursor = tool ? 'crosshair' : '';
    }

    function cancelTool() {
      setTool(null);
    }

    // --- Toolbar UI ---

    function createToolbar() {
      var bar = document.createElement('div');
      bar.id = 'map-draw-toolbar';
      bar.className = 'absolute top-3 left-3 z-[1000] bg-surface border border-edge rounded-lg shadow-lg p-1 flex flex-col gap-1';

      var tools = [
        { id: 'freehand', icon: 'fa-pen', label: 'Freehand', fn: startFreehand },
        { id: 'rectangle', icon: 'fa-vector-square', label: 'Rectangle', fn: startRectangle },
        { id: 'ellipse', icon: 'fa-circle', label: 'Circle', fn: startCircle },
        { id: 'polygon', icon: 'fa-draw-polygon', label: 'Polygon', fn: startPolygon },
        { id: 'text', icon: 'fa-font', label: 'Text', fn: startText },
      ];

      tools.forEach(function (t) {
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'map-draw-btn w-9 h-9 flex items-center justify-center rounded-md text-sm text-fg-secondary hover:bg-surface-alt hover:text-fg transition-colors';
        btn.dataset.tool = t.id;
        btn.title = t.label;
        btn.innerHTML = '<i class="fa-solid ' + t.icon + '"></i>';
        btn.addEventListener('click', function () {
          if (activeTool === t.id) {
            cancelTool();
          } else {
            t.fn();
          }
        });
        bar.appendChild(btn);
      });

      // Separator.
      var sep = document.createElement('div');
      sep.className = 'w-full h-px bg-edge my-0.5';
      bar.appendChild(sep);

      // Color picker.
      var colorBtn = document.createElement('div');
      colorBtn.className = 'w-9 h-9 flex items-center justify-center';
      colorBtn.innerHTML = '<input type="color" value="' + drawColor + '" class="w-7 h-7 rounded border border-edge cursor-pointer" title="Draw color"/>';
      colorBtn.querySelector('input').addEventListener('input', function () {
        drawColor = this.value;
      });
      bar.appendChild(colorBtn);

      // Width control.
      var widthBtn = document.createElement('button');
      widthBtn.type = 'button';
      widthBtn.className = 'w-9 h-9 flex items-center justify-center rounded-md text-[10px] text-fg-secondary hover:bg-surface-alt hover:text-fg transition-colors font-mono';
      widthBtn.title = 'Stroke width';
      widthBtn.textContent = drawWidth + 'px';
      widthBtn.addEventListener('click', function () {
        drawWidth = drawWidth >= 8 ? 1 : drawWidth + 1;
        widthBtn.textContent = drawWidth + 'px';
      });
      bar.appendChild(widthBtn);

      var container = document.getElementById('map-container');
      if (container) container.appendChild(bar);
    }

    function updateToolbar() {
      var btns = document.querySelectorAll('.map-draw-btn');
      btns.forEach(function (btn) {
        if (btn.dataset.tool === activeTool) {
          btn.classList.add('bg-accent/20', 'text-accent');
          btn.classList.remove('text-fg-secondary');
        } else {
          btn.classList.remove('bg-accent/20', 'text-accent');
          btn.classList.add('text-fg-secondary');
        }
      });
    }

    // --- Keyboard Shortcuts ---

    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && activeTool) {
        cancelTool();
      }
    });

    // --- Init ---

    createToolbar();
    loadDrawings();
  }
})();
