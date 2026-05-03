/**
 * relation_graph.js -- Chronicle Relations Graph Visualization
 *
 * D3.js force-directed graph showing entity relationships within a campaign.
 * Nodes represent entities (colored by type), edges represent relations.
 * Supports @mention edges, type/search filtering, local graph mode, and
 * orphan detection. Auto-mounted by boot.js on elements with
 * data-widget="relation-graph".
 *
 * Config (from data-* attributes):
 *   data-campaign-id    - Campaign ID
 *   data-api-url        - Graph API endpoint (GET /campaigns/:id/relations-graph)
 *   data-height         - Optional height in px (default: 500)
 *   data-entity-types   - JSON array of {slug, name, color, icon} for filter dropdown
 *   data-show-filters   - "true" to show the filter toolbar (default: false)
 *   data-focus-entity   - Entity ID for local/ego graph mode
 *   data-hops           - Hop depth for local graph (default: 2)
 *
 * Requires D3.js v7. If not loaded, dynamically fetches from CDN.
 */
(function () {
  'use strict';

  var D3_SRC = '/static/vendor/d3.min.js';

  Chronicle.register('relation-graph', {
    init: function (el, config) {
      var apiUrl = config.apiUrl || '';
      var campaignId = config.campaignId || '';
      var height = parseInt(config.height, 10) || 500;
      var showFilters = config.showFilters === true;
      var focusEntity = config.focusEntity || '';
      var hops = parseInt(config.hops, 10) || 2;
      var entityTypes = [];
      try {
        entityTypes = JSON.parse(config.entityTypes || '[]');
      } catch (e) { /* ignore */ }

      // Filter state.
      var filters = {
        types: [],
        search: '',
        includeMentions: true,
        includeOrphans: false
      };

      // Container setup.
      el.style.position = 'relative';
      el.innerHTML = '<div class="text-center py-12 text-fg-muted text-sm">Loading graph...</div>';

      function ensureD3(cb) {
        if (typeof d3 !== 'undefined') { cb(); return; }
        var existing = document.querySelector('script[src="' + D3_SRC + '"]');
        if (existing) {
          var poll = setInterval(function () {
            if (typeof d3 !== 'undefined') { clearInterval(poll); cb(); }
          }, 100);
          return;
        }
        var script = document.createElement('script');
        script.src = D3_SRC;
        script.onload = cb;
        script.onerror = function () {
          el.innerHTML = '<div class="text-center py-12 text-red-500 text-sm">Failed to load D3.js</div>';
        };
        document.head.appendChild(script);
      }

      function buildApiUrl() {
        var url = apiUrl;
        var params = [];
        if (filters.types.length > 0) {
          params.push('types=' + encodeURIComponent(filters.types.join(',')));
        }
        if (filters.search) {
          params.push('search=' + encodeURIComponent(filters.search));
        }
        if (!filters.includeMentions) {
          params.push('include_mentions=false');
        }
        if (filters.includeOrphans) {
          params.push('include_orphans=true');
        }
        if (focusEntity) {
          params.push('focus=' + encodeURIComponent(focusEntity));
          params.push('hops=' + hops);
        }
        if (params.length > 0) {
          url += (url.indexOf('?') >= 0 ? '&' : '?') + params.join('&');
        }
        return url;
      }

      function loadAndRender() {
        // Preserve filter toolbar if present.
        var toolbar = el.querySelector('.graph-filter-toolbar');
        var toolbarHTML = toolbar ? toolbar.outerHTML : '';

        Chronicle.apiFetch(buildApiUrl())
          .then(function (r) { return r.json(); })
          .then(function (data) {
            // Clear graph area but keep toolbar.
            el.innerHTML = '';
            if (toolbarHTML) {
              el.insertAdjacentHTML('afterbegin', toolbarHTML);
              attachFilterHandlers();
            }
            render(data);
          })
          .catch(function (err) {
            el.innerHTML = toolbarHTML +
              '<div class="text-center py-12 text-red-500 text-sm">Failed to load graph data</div>';
            if (toolbarHTML) attachFilterHandlers();
            console.error('[RelationGraph] Load error:', err);
            Chronicle.notify('Failed to load graph data', 'error');
          });
      }

      function buildFilterToolbar() {
        if (!showFilters) return;

        var html = '<div class="graph-filter-toolbar flex flex-wrap items-center gap-3 mb-3 p-3 card">';

        // Entity type multi-select as checkboxes dropdown.
        if (entityTypes.length > 0) {
          html += '<div class="relative" data-graph-dropdown>';
          html += '<button type="button" class="btn-secondary text-xs" data-graph-dropdown-toggle>';
          html += '<i class="fa-solid fa-filter mr-1"></i>Types';
          html += '</button>';
          html += '<div class="absolute left-0 top-full mt-1 bg-surface border border-edge rounded-lg shadow-lg p-2 z-20 min-w-[180px] hidden" data-graph-dropdown-menu>';
          for (var i = 0; i < entityTypes.length; i++) {
            var et = entityTypes[i];
            html += '<label class="flex items-center gap-2 px-2 py-1 text-xs text-fg hover:bg-surface-alt rounded cursor-pointer">';
            html += '<input type="checkbox" value="' + Chronicle.escapeHtml(et.slug) + '" class="graph-type-filter" />';
            html += '<span style="color:' + Chronicle.escapeHtml(et.color) + '"><i class="fa-solid ' + Chronicle.escapeHtml(et.icon) + '"></i></span>';
            html += Chronicle.escapeHtml(et.name);
            html += '</label>';
          }
          html += '</div></div>';
        }

        // Search input.
        html += '<div class="flex-1 min-w-[120px] max-w-[240px]">';
        html += '<input type="text" class="input text-xs w-full" placeholder="Search nodes..." data-graph-search />';
        html += '</div>';

        // Mention toggle.
        html += '<label class="flex items-center gap-1 text-xs text-fg-secondary cursor-pointer">';
        html += '<input type="checkbox" checked class="graph-mention-toggle" /> Mentions';
        html += '</label>';

        // Orphan toggle.
        html += '<label class="flex items-center gap-1 text-xs text-fg-secondary cursor-pointer">';
        html += '<input type="checkbox" class="graph-orphan-toggle" /> Orphans';
        html += '</label>';

        html += '</div>';
        el.insertAdjacentHTML('afterbegin', html);
        attachFilterHandlers();
      }

      function attachFilterHandlers() {
        // Dropdown toggle.
        var toggle = el.querySelector('[data-graph-dropdown-toggle]');
        var menu = el.querySelector('[data-graph-dropdown-menu]');
        if (toggle && menu) {
          toggle.addEventListener('click', function (e) {
            e.stopPropagation();
            menu.classList.toggle('hidden');
          });
          document.addEventListener('click', function () {
            menu.classList.add('hidden');
          });
          menu.addEventListener('click', function (e) { e.stopPropagation(); });
        }

        // Type checkboxes.
        var typeBoxes = el.querySelectorAll('.graph-type-filter');
        for (var i = 0; i < typeBoxes.length; i++) {
          typeBoxes[i].addEventListener('change', function () {
            filters.types = [];
            var checked = el.querySelectorAll('.graph-type-filter:checked');
            for (var j = 0; j < checked.length; j++) {
              filters.types.push(checked[j].value);
            }
            debounceReload();
          });
        }

        // Search input with debounce.
        var searchInput = el.querySelector('[data-graph-search]');
        if (searchInput) {
          searchInput.value = filters.search;
          searchInput.addEventListener('input', function () {
            filters.search = this.value.trim();
            debounceReload();
          });
        }

        // Mention toggle.
        var mentionToggle = el.querySelector('.graph-mention-toggle');
        if (mentionToggle) {
          mentionToggle.checked = filters.includeMentions;
          mentionToggle.addEventListener('change', function () {
            filters.includeMentions = this.checked;
            loadAndRender();
          });
        }

        // Orphan toggle.
        var orphanToggle = el.querySelector('.graph-orphan-toggle');
        if (orphanToggle) {
          orphanToggle.checked = filters.includeOrphans;
          orphanToggle.addEventListener('change', function () {
            filters.includeOrphans = this.checked;
            loadAndRender();
          });
        }
      }

      var reloadTimer = null;
      function debounceReload() {
        clearTimeout(reloadTimer);
        reloadTimer = setTimeout(function () { loadAndRender(); }, 300);
      }

      function render(data) {
        if (!data.nodes || data.nodes.length === 0) {
          var msg = '<div class="card p-8 text-center">' +
            '<i class="fa-solid fa-diagram-project text-3xl text-fg-muted mb-3"></i>' +
            '<p class="text-fg-muted text-sm">No relations found. Create relations between entities to see the graph.</p></div>';
          // Append after toolbar if present.
          var existing = el.querySelector('.graph-filter-toolbar');
          if (existing) {
            existing.insertAdjacentHTML('afterend', msg);
          } else {
            el.insertAdjacentHTML('beforeend', msg);
          }
          return;
        }

        var width = el.clientWidth || 800;

        // Count connections per node for sizing.
        var connectionCount = {};
        data.nodes.forEach(function (n) { connectionCount[n.id] = 0; });
        data.edges.forEach(function (e) {
          connectionCount[e.source] = (connectionCount[e.source] || 0) + 1;
          connectionCount[e.target] = (connectionCount[e.target] || 0) + 1;
        });

        // SVG with zoom/pan.
        var svg = d3.select(el)
          .append('svg')
          .attr('width', width)
          .attr('height', height)
          .attr('class', 'relation-graph-svg')
          .style('background', 'var(--color-surface, white)')
          .style('border-radius', '8px')
          .style('border', '1px solid var(--color-edge, #e5e7eb)');

        var g = svg.append('g');

        var zoom = d3.zoom()
          .scaleExtent([0.1, 4])
          .on('zoom', function (event) {
            g.attr('transform', event.transform);
          });
        svg.call(zoom);

        // Arrow markers for directed edges.
        var defs = svg.append('defs');
        defs.append('marker')
          .attr('id', 'arrowhead')
          .attr('viewBox', '0 -5 10 10')
          .attr('refX', 25)
          .attr('refY', 0)
          .attr('markerWidth', 6)
          .attr('markerHeight', 6)
          .attr('orient', 'auto')
          .append('path')
          .attr('d', 'M0,-5L10,0L0,5')
          .attr('fill', 'var(--color-fg-muted, #9ca3af)');

        defs.append('marker')
          .attr('id', 'arrowhead-mention')
          .attr('viewBox', '0 -5 10 10')
          .attr('refX', 25)
          .attr('refY', 0)
          .attr('markerWidth', 6)
          .attr('markerHeight', 6)
          .attr('orient', 'auto')
          .append('path')
          .attr('d', 'M0,-5L10,0L0,5')
          .attr('fill', 'var(--color-accent, #8b5cf6)');

        // Cluster force: pull nodes toward type-specific cluster centers.
        var typeColors = {};
        var typeList = [];
        data.nodes.forEach(function (n) {
          if (n.type && !typeColors[n.type]) {
            typeColors[n.type] = n.color || '#6b7280';
            typeList.push(n.type);
          }
        });

        var clusterCenters = {};
        var angle, cx, cy;
        for (var i = 0; i < typeList.length; i++) {
          angle = (2 * Math.PI * i) / typeList.length;
          cx = width / 2 + (width * 0.25) * Math.cos(angle);
          cy = height / 2 + (height * 0.25) * Math.sin(angle);
          clusterCenters[typeList[i]] = { x: cx, y: cy };
        }

        // Force simulation with clustering.
        var simulation = d3.forceSimulation(data.nodes)
          .force('link', d3.forceLink(data.edges).id(function (d) { return d.id; }).distance(120))
          .force('charge', d3.forceManyBody().strength(-300))
          .force('center', d3.forceCenter(width / 2, height / 2))
          .force('collision', d3.forceCollide().radius(function (d) {
            return nodeRadius(d) + 4;
          }));

        // Cluster force: gentle pull toward type centers.
        if (typeList.length > 1) {
          simulation.force('clusterX', d3.forceX(function (d) {
            return clusterCenters[d.type] ? clusterCenters[d.type].x : width / 2;
          }).strength(0.05));
          simulation.force('clusterY', d3.forceY(function (d) {
            return clusterCenters[d.type] ? clusterCenters[d.type].y : height / 2;
          }).strength(0.05));
        }

        // Node radius: scale by connection count.
        function nodeRadius(d) {
          if (d.orphan) return 8;
          var count = connectionCount[d.id] || 0;
          return Math.max(12, Math.min(24, 12 + count * 2));
        }

        // Edge lines — different styles for relations vs mentions.
        var links = g.append('g')
          .selectAll('line')
          .data(data.edges)
          .join('line')
          .attr('stroke', function (d) {
            return d.kind === 'mention' ? 'var(--color-accent, #8b5cf6)' : 'var(--color-fg-muted, #9ca3af)';
          })
          .attr('stroke-opacity', function (d) {
            return d.kind === 'mention' ? 0.4 : 0.5;
          })
          .attr('stroke-width', function (d) {
            return d.kind === 'mention' ? 1 : 1.5;
          })
          .attr('stroke-dasharray', function (d) {
            return d.kind === 'mention' ? '4,3' : 'none';
          })
          .attr('marker-end', function (d) {
            return d.kind === 'mention' ? 'url(#arrowhead-mention)' : 'url(#arrowhead)';
          });

        // Edge labels.
        var linkLabels = g.append('g')
          .selectAll('text')
          .data(data.edges)
          .join('text')
          .text(function (d) { return d.type; })
          .attr('font-size', '9px')
          .attr('fill', function (d) {
            return d.kind === 'mention' ? 'var(--color-accent, #8b5cf6)' : 'var(--color-fg-muted, #9ca3af)';
          })
          .attr('text-anchor', 'middle')
          .attr('dy', -4);

        // Node groups.
        var nodes = g.append('g')
          .selectAll('g')
          .data(data.nodes)
          .join('g')
          .attr('cursor', 'pointer')
          .call(d3.drag()
            .on('start', function (event, d) {
              if (!event.active) simulation.alphaTarget(0.3).restart();
              d.fx = d.x;
              d.fy = d.y;
            })
            .on('drag', function (event, d) {
              d.fx = event.x;
              d.fy = event.y;
            })
            .on('end', function (event, d) {
              if (!event.active) simulation.alphaTarget(0);
              d.fx = null;
              d.fy = null;
            })
          );

        // Node circles — orphans get dotted border and smaller radius.
        nodes.append('circle')
          .attr('r', function (d) { return nodeRadius(d); })
          .attr('fill', function (d) {
            if (d.orphan) return 'var(--color-surface-alt, #f3f4f6)';
            return d.color || '#6b7280';
          })
          .attr('stroke', function (d) {
            if (d.orphan) return 'var(--color-fg-muted, #9ca3af)';
            return 'var(--color-surface, white)';
          })
          .attr('stroke-width', function (d) { return d.orphan ? 1.5 : 2; })
          .attr('stroke-dasharray', function (d) { return d.orphan ? '3,2' : 'none'; })
          .attr('opacity', function (d) { return d.orphan ? 0.6 : 1; });

        // Node icons (first letter).
        nodes.append('text')
          .text(function (d) { return (d.name || '?')[0].toUpperCase(); })
          .attr('text-anchor', 'middle')
          .attr('dy', '0.35em')
          .attr('font-size', function (d) { return (nodeRadius(d) * 0.7) + 'px'; })
          .attr('font-weight', 'bold')
          .attr('fill', function (d) {
            return d.orphan ? 'var(--color-fg-muted, #9ca3af)' : 'white';
          })
          .attr('pointer-events', 'none');

        // Node labels below circles.
        nodes.append('text')
          .text(function (d) { return d.name || '(unknown)'; })
          .attr('text-anchor', 'middle')
          .attr('dy', function (d) { return nodeRadius(d) + 14; })
          .attr('font-size', '11px')
          .attr('fill', 'var(--color-fg-body, #374151)');

        // Click to navigate to entity.
        nodes.on('click', function (event, d) {
          if (campaignId && d.id) {
            window.location.href = '/campaigns/' + campaignId + '/entities/' + d.id;
          }
        });

        // Tooltip on hover.
        var tooltip = d3.select(el)
          .append('div')
          .attr('class', 'relation-graph-tooltip')
          .style('position', 'absolute')
          .style('display', 'none')
          .style('padding', '6px 10px')
          .style('background', 'var(--color-surface-alt, #f3f4f6)')
          .style('border', '1px solid var(--color-edge, #e5e7eb)')
          .style('border-radius', '6px')
          .style('font-size', '12px')
          .style('color', 'var(--color-fg-body, #374151)')
          .style('pointer-events', 'none')
          .style('z-index', '10');

        nodes.on('mouseenter', function (event, d) {
          var info = '<strong>' + Chronicle.escapeHtml(d.name || '(unknown)') + '</strong>';
          if (d.type) info += '<br><span style="color:var(--color-fg-muted)">' + Chronicle.escapeHtml(d.type) + '</span>';
          if (d.orphan) info += '<br><span style="color:var(--color-warning, #f59e0b)">No connections</span>';
          var count = connectionCount[d.id] || 0;
          if (count > 0) info += '<br><span style="color:var(--color-fg-muted)">' + count + ' connection' + (count !== 1 ? 's' : '') + '</span>';
          tooltip.style('display', 'block').html(info);
        });
        nodes.on('mousemove', function (event) {
          var rect = el.getBoundingClientRect();
          tooltip
            .style('left', (event.clientX - rect.left + 12) + 'px')
            .style('top', (event.clientY - rect.top - 10) + 'px');
        });
        nodes.on('mouseleave', function () {
          tooltip.style('display', 'none');
        });

        // Tick.
        simulation.on('tick', function () {
          links
            .attr('x1', function (d) { return d.source.x; })
            .attr('y1', function (d) { return d.source.y; })
            .attr('x2', function (d) { return d.target.x; })
            .attr('y2', function (d) { return d.target.y; });

          linkLabels
            .attr('x', function (d) { return (d.source.x + d.target.x) / 2; })
            .attr('y', function (d) { return (d.source.y + d.target.y) / 2; });

          nodes.attr('transform', function (d) { return 'translate(' + d.x + ',' + d.y + ')'; });
        });

        // Controls bar (zoom buttons).
        var controls = document.createElement('div');
        controls.style.cssText = 'position:absolute;top:8px;right:8px;display:flex;gap:4px;';
        // Offset for toolbar presence.
        var toolbar = el.querySelector('.graph-filter-toolbar');
        if (toolbar) {
          controls.style.top = (toolbar.offsetHeight + 16) + 'px';
        }

        var zoomIn = document.createElement('button');
        zoomIn.className = 'btn-secondary text-xs';
        zoomIn.innerHTML = '<i class="fa-solid fa-plus"></i>';
        zoomIn.title = 'Zoom in';
        zoomIn.addEventListener('click', function () { svg.transition().call(zoom.scaleBy, 1.5); });

        var zoomOut = document.createElement('button');
        zoomOut.className = 'btn-secondary text-xs';
        zoomOut.innerHTML = '<i class="fa-solid fa-minus"></i>';
        zoomOut.title = 'Zoom out';
        zoomOut.addEventListener('click', function () { svg.transition().call(zoom.scaleBy, 0.67); });

        var resetBtn = document.createElement('button');
        resetBtn.className = 'btn-secondary text-xs';
        resetBtn.innerHTML = '<i class="fa-solid fa-expand"></i>';
        resetBtn.title = 'Reset view';
        resetBtn.addEventListener('click', function () {
          svg.transition().call(zoom.transform, d3.zoomIdentity);
        });

        // Export PNG button.
        var exportBtn = document.createElement('button');
        exportBtn.className = 'btn-secondary text-xs';
        exportBtn.innerHTML = '<i class="fa-solid fa-download"></i>';
        exportBtn.title = 'Export as PNG';
        exportBtn.addEventListener('click', function () {
          var svgEl = el.querySelector('svg.relation-graph-svg');
          if (!svgEl) return;
          // Serialize SVG to a data URL, draw on canvas, then download.
          var serializer = new XMLSerializer();
          var svgStr = serializer.serializeToString(svgEl);
          var canvas = document.createElement('canvas');
          var scale = 2; // 2x for retina clarity
          canvas.width = svgEl.clientWidth * scale;
          canvas.height = svgEl.clientHeight * scale;
          var ctxCanvas = canvas.getContext('2d');
          ctxCanvas.scale(scale, scale);
          // Fill with background color.
          var bgStyle = getComputedStyle(svgEl).backgroundColor;
          ctxCanvas.fillStyle = bgStyle || '#ffffff';
          ctxCanvas.fillRect(0, 0, canvas.width, canvas.height);
          var img = new Image();
          var blob = new Blob([svgStr], { type: 'image/svg+xml;charset=utf-8' });
          var url = URL.createObjectURL(blob);
          img.onload = function () {
            ctxCanvas.drawImage(img, 0, 0, svgEl.clientWidth, svgEl.clientHeight);
            URL.revokeObjectURL(url);
            var link = document.createElement('a');
            link.download = 'relations-graph.png';
            link.href = canvas.toDataURL('image/png');
            link.click();
          };
          img.onerror = function () {
            URL.revokeObjectURL(url);
            Chronicle.notify('Failed to export graph', 'error');
          };
          img.src = url;
        });

        controls.appendChild(zoomIn);
        controls.appendChild(zoomOut);
        controls.appendChild(resetBtn);
        controls.appendChild(exportBtn);
        el.appendChild(controls);

        // Legend.
        var legendItems = [];

        // Entity type colors.
        var typeKeys = Object.keys(typeColors);
        if (typeKeys.length > 1) {
          typeKeys.forEach(function (type) {
            legendItems.push(
              '<span style="display:flex;align-items:center;gap:4px;">' +
              '<span style="width:10px;height:10px;border-radius:50%;background:' + Chronicle.escapeHtml(typeColors[type]) + ';display:inline-block;"></span>' +
              Chronicle.escapeHtml(type) + '</span>'
            );
          });
        }

        // Edge type indicators.
        var hasMentions = data.edges.some(function (e) { return e.kind === 'mention'; });
        if (hasMentions) {
          legendItems.push(
            '<span style="display:flex;align-items:center;gap:4px;">' +
            '<span style="width:16px;height:0;border-top:2px dashed var(--color-accent, #8b5cf6);display:inline-block;"></span>' +
            '<span>mention</span></span>'
          );
          legendItems.push(
            '<span style="display:flex;align-items:center;gap:4px;">' +
            '<span style="width:16px;height:0;border-top:2px solid var(--color-fg-muted, #9ca3af);display:inline-block;"></span>' +
            '<span>relation</span></span>'
          );
        }

        // Orphan indicator.
        var hasOrphans = data.nodes.some(function (n) { return n.orphan; });
        if (hasOrphans) {
          legendItems.push(
            '<span style="display:flex;align-items:center;gap:4px;">' +
            '<span style="width:10px;height:10px;border-radius:50%;border:1.5px dashed var(--color-fg-muted, #9ca3af);display:inline-block;"></span>' +
            '<span>orphan</span></span>'
          );
        }

        if (legendItems.length > 0) {
          var legend = document.createElement('div');
          legend.style.cssText = 'position:absolute;bottom:8px;left:8px;display:flex;flex-wrap:wrap;gap:8px;font-size:11px;color:var(--color-fg-secondary,#6b7280);';
          legend.innerHTML = legendItems.join('');
          el.appendChild(legend);
        }

        // Store ref for cleanup.
        el._graphSimulation = simulation;
      }

      // Initial render.
      ensureD3(function () {
        buildFilterToolbar();
        loadAndRender();
      });
    },

    destroy: function (el) {
      if (el._graphSimulation) {
        el._graphSimulation.stop();
        delete el._graphSimulation;
      }
      el.innerHTML = '';
    }
  });
})();
