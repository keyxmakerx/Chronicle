/**
 * relation_graph.js -- Chronicle Relations Graph Visualization
 *
 * D3.js force-directed graph showing entity relationships within a campaign.
 * Nodes represent entities (colored by type), edges represent relations.
 * Auto-mounted by boot.js on elements with data-widget="relation-graph".
 *
 * Config (from data-* attributes):
 *   data-campaign-id  - Campaign ID
 *   data-api-url      - Graph API endpoint (GET /campaigns/:id/relations-graph)
 *   data-height       - Optional height in px (default: 500)
 *
 * Requires D3.js v7. If not loaded, dynamically fetches from CDN.
 */
(function () {
  'use strict';

  var D3_SRC = 'https://cdn.jsdelivr.net/npm/d3@7/dist/d3.min.js';

  Chronicle.register('relation-graph', {
    init: function (el, config) {
      var apiUrl = config.apiUrl || '';
      var campaignId = config.campaignId || '';
      var height = parseInt(config.height, 10) || 500;

      if (!apiUrl && campaignId) {
        apiUrl = '/campaigns/' + campaignId + '/relations-graph';
      }

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

      function loadAndRender() {
        fetch(apiUrl, { headers: { 'Accept': 'application/json' }, credentials: 'same-origin' })
          .then(function (r) { return r.json(); })
          .then(function (data) { render(data); })
          .catch(function (err) {
            el.innerHTML = '<div class="text-center py-12 text-red-500 text-sm">Failed to load graph data</div>';
            console.error('[RelationGraph] Load error:', err);
            Chronicle.notify('Failed to load graph data', 'error');
          });
      }

      function render(data) {
        if (!data.nodes || data.nodes.length === 0) {
          el.innerHTML = '<div class="card p-8 text-center">' +
            '<i class="fa-solid fa-diagram-project text-3xl text-fg-muted mb-3"></i>' +
            '<p class="text-fg-muted text-sm">No relations found. Create relations between entities to see the graph.</p></div>';
          return;
        }

        el.innerHTML = '';
        var width = el.clientWidth || 800;

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

        // Arrow marker for directed edges.
        svg.append('defs').append('marker')
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

        // Force simulation.
        var simulation = d3.forceSimulation(data.nodes)
          .force('link', d3.forceLink(data.edges).id(function (d) { return d.id; }).distance(120))
          .force('charge', d3.forceManyBody().strength(-300))
          .force('center', d3.forceCenter(width / 2, height / 2))
          .force('collision', d3.forceCollide().radius(30));

        // Edge lines.
        var links = g.append('g')
          .selectAll('line')
          .data(data.edges)
          .join('line')
          .attr('stroke', 'var(--color-fg-muted, #9ca3af)')
          .attr('stroke-opacity', 0.5)
          .attr('stroke-width', 1.5)
          .attr('marker-end', 'url(#arrowhead)');

        // Edge labels.
        var linkLabels = g.append('g')
          .selectAll('text')
          .data(data.edges)
          .join('text')
          .text(function (d) { return d.type; })
          .attr('font-size', '9px')
          .attr('fill', 'var(--color-fg-muted, #9ca3af)')
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

        // Node circles.
        nodes.append('circle')
          .attr('r', 16)
          .attr('fill', function (d) { return d.color || '#6b7280'; })
          .attr('stroke', 'var(--color-surface, white)')
          .attr('stroke-width', 2);

        // Node icons (FontAwesome Unicode — simplified, just show type initial).
        nodes.append('text')
          .text(function (d) { return (d.name || '?')[0].toUpperCase(); })
          .attr('text-anchor', 'middle')
          .attr('dy', '0.35em')
          .attr('font-size', '11px')
          .attr('font-weight', 'bold')
          .attr('fill', 'white')
          .attr('pointer-events', 'none');

        // Node labels below circles.
        nodes.append('text')
          .text(function (d) { return d.name; })
          .attr('text-anchor', 'middle')
          .attr('dy', 30)
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
          tooltip.style('display', 'block')
            .html('<strong>' + Chronicle.escapeHtml(d.name) + '</strong>' +
              (d.type ? '<br><span style="color:var(--color-fg-muted)">' + Chronicle.escapeHtml(d.type) + '</span>' : ''));
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

        // Controls bar.
        var controls = document.createElement('div');
        controls.style.cssText = 'position:absolute;top:8px;right:8px;display:flex;gap:4px;';

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

        controls.appendChild(zoomIn);
        controls.appendChild(zoomOut);
        controls.appendChild(resetBtn);
        el.appendChild(controls);

        // Legend.
        var typeColors = {};
        data.nodes.forEach(function (n) {
          if (n.type && !typeColors[n.type]) typeColors[n.type] = n.color;
        });
        var typeKeys = Object.keys(typeColors);
        if (typeKeys.length > 1) {
          var legend = document.createElement('div');
          legend.style.cssText = 'position:absolute;bottom:8px;left:8px;display:flex;flex-wrap:wrap;gap:8px;font-size:11px;color:var(--color-fg-secondary,#6b7280);';
          typeKeys.forEach(function (type) {
            var item = document.createElement('span');
            item.style.cssText = 'display:flex;align-items:center;gap:4px;';
            item.innerHTML = '<span style="width:10px;height:10px;border-radius:50%;background:' + Chronicle.escapeHtml(typeColors[type]) + ';display:inline-block;"></span>' + Chronicle.escapeHtml(type);
            legend.appendChild(item);
          });
          el.appendChild(legend);
        }

        // Store ref for cleanup.
        el._graphSimulation = simulation;
      }

      ensureD3(loadAndRender);
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
