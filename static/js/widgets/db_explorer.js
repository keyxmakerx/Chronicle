/**
 * db_explorer.js -- Chronicle Database Schema Explorer
 *
 * D3.js force-directed graph showing database tables and their foreign key
 * relationships. Tables are rendered as rounded rectangles colored by plugin
 * ownership. Clicking a table shows its column details in a side panel.
 * Auto-mounted by boot.js on elements with data-widget="db-explorer".
 *
 * Config (from data-* attributes):
 *   data-api-url  - Schema API endpoint (GET /admin/database/schema)
 *
 * Requires D3.js v7. If not loaded, dynamically fetches from CDN.
 */
(function () {
  'use strict';

  var D3_SRC = 'https://cdn.jsdelivr.net/npm/d3@7/dist/d3.min.js';

  // Plugin color palette — tuned for both light and dark modes.
  var PLUGIN_COLORS = {
    core:       { bg: '#64748b', text: '#f8fafc' },
    calendar:   { bg: '#0d9488', text: '#f0fdfa' },
    maps:       { bg: '#059669', text: '#ecfdf5' },
    sessions:   { bg: '#7c3aed', text: '#f5f3ff' },
    timeline:   { bg: '#d97706', text: '#fffbeb' },
    syncapi:    { bg: '#0891b2', text: '#ecfeff' },
    extensions: { bg: '#db2777', text: '#fdf2f8' },
    system:     { bg: '#475569', text: '#f1f5f9' }
  };

  var DEFAULT_COLOR = { bg: '#6b7280', text: '#f9fafb' };

  // Table node dimensions.
  var NODE_W = 160;
  var NODE_H = 52;
  var NODE_RX = 8;

  Chronicle.register('db-explorer', {
    init: function (el, config) {
      var apiUrl = config.apiUrl || '';
      var simulation = null;
      var svgNode = null;

      el.style.position = 'relative';
      el.innerHTML = '<div class="text-center py-12 text-fg-muted text-sm">Loading schema...</div>';

      // Dynamic D3 loading (same pattern as relation_graph.js).
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

      function getPluginColor(plugin) {
        return PLUGIN_COLORS[plugin] || DEFAULT_COLOR;
      }

      function loadAndRender() {
        Chronicle.apiFetch(apiUrl)
          .then(function (r) { return r.json(); })
          .then(function (data) {
            el.innerHTML = '';
            render(data);
          })
          .catch(function (err) {
            el.innerHTML = '<div class="text-center py-12 text-red-500 text-sm">Failed to load schema data</div>';
            console.error('[DBExplorer] Load error:', err);
          });
      }

      function render(data) {
        if (!data.tables || data.tables.length === 0) {
          el.innerHTML = '<div class="text-center py-12 text-fg-muted text-sm">' +
            '<i class="fa-solid fa-database text-3xl mb-3"></i>' +
            '<p>No tables found.</p></div>';
          return;
        }

        var width = el.clientWidth || 900;
        var height = 600;

        // Build node data from tables.
        var nodes = data.tables.map(function (t) {
          return {
            id: t.name,
            name: t.name,
            rowCount: t.rowCount,
            columnCount: t.columnCount,
            dataSizeKB: t.dataSizeKB,
            plugin: t.plugin,
            columns: t.columns || [],
            color: getPluginColor(t.plugin)
          };
        });

        var nodeMap = {};
        nodes.forEach(function (n) { nodeMap[n.id] = n; });

        // Build edge data from foreign keys.
        var edges = [];
        if (data.foreignKeys) {
          data.foreignKeys.forEach(function (fk) {
            // Only include edges where both tables exist.
            if (nodeMap[fk.fromTable] && nodeMap[fk.toTable]) {
              edges.push({
                source: fk.fromTable,
                target: fk.toTable,
                fromColumn: fk.fromColumn,
                toColumn: fk.toColumn
              });
            }
          });
        }

        // Identify plugin groups for clustering.
        var pluginList = [];
        var pluginSet = {};
        nodes.forEach(function (n) {
          if (!pluginSet[n.plugin]) {
            pluginSet[n.plugin] = true;
            pluginList.push(n.plugin);
          }
        });

        // Cluster centers arranged in a circle.
        var clusterCenters = {};
        pluginList.forEach(function (p, i) {
          var angle = (2 * Math.PI * i) / pluginList.length;
          clusterCenters[p] = {
            x: width / 2 + (width * 0.3) * Math.cos(angle),
            y: height / 2 + (height * 0.3) * Math.sin(angle)
          };
        });

        // Create SVG.
        var svg = d3.select(el)
          .append('svg')
          .attr('width', width)
          .attr('height', height)
          .style('background', 'var(--color-surface, #ffffff)')
          .style('border-radius', '0 0 8px 8px');

        svgNode = svg.node();

        var g = svg.append('g');

        // Zoom & pan.
        var zoom = d3.zoom()
          .scaleExtent([0.15, 3])
          .on('zoom', function (event) {
            g.attr('transform', event.transform);
          });
        svg.call(zoom);

        // Arrow markers for FK edges.
        var defs = svg.append('defs');
        defs.append('marker')
          .attr('id', 'db-arrow')
          .attr('viewBox', '0 -5 10 10')
          .attr('refX', NODE_W / 2 + 10)
          .attr('refY', 0)
          .attr('markerWidth', 6)
          .attr('markerHeight', 6)
          .attr('orient', 'auto')
          .append('path')
          .attr('d', 'M0,-5L10,0L0,5')
          .attr('fill', 'var(--color-fg-muted, #9ca3af)');

        // Force simulation.
        simulation = d3.forceSimulation(nodes)
          .force('link', d3.forceLink(edges)
            .id(function (d) { return d.id; })
            .distance(200))
          .force('charge', d3.forceManyBody().strength(-400))
          .force('center', d3.forceCenter(width / 2, height / 2))
          .force('collision', d3.forceCollide()
            .radius(NODE_W / 2 + 20));

        // Cluster forces.
        if (pluginList.length > 1) {
          simulation.force('clusterX', d3.forceX(function (d) {
            return clusterCenters[d.plugin] ? clusterCenters[d.plugin].x : width / 2;
          }).strength(0.08));
          simulation.force('clusterY', d3.forceY(function (d) {
            return clusterCenters[d.plugin] ? clusterCenters[d.plugin].y : height / 2;
          }).strength(0.08));
        }

        // FK edges.
        var links = g.append('g')
          .selectAll('line')
          .data(edges)
          .join('line')
          .attr('stroke', 'var(--color-fg-muted, #9ca3af)')
          .attr('stroke-opacity', 0.4)
          .attr('stroke-width', 1.5)
          .attr('marker-end', 'url(#db-arrow)');

        // FK edge labels.
        var linkLabels = g.append('g')
          .selectAll('text')
          .data(edges)
          .join('text')
          .text(function (d) { return d.fromColumn; })
          .attr('font-size', '8px')
          .attr('fill', 'var(--color-fg-muted, #9ca3af)')
          .attr('text-anchor', 'middle')
          .attr('dy', -4);

        // Table node groups.
        var nodeGroups = g.append('g')
          .selectAll('g')
          .data(nodes)
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

        // Rounded rectangle background.
        nodeGroups.append('rect')
          .attr('width', NODE_W)
          .attr('height', NODE_H)
          .attr('x', -NODE_W / 2)
          .attr('y', -NODE_H / 2)
          .attr('rx', NODE_RX)
          .attr('ry', NODE_RX)
          .attr('fill', function (d) { return d.color.bg; })
          .attr('stroke', 'var(--color-edge, #e5e7eb)')
          .attr('stroke-width', 1)
          .attr('opacity', 0.9);

        // Table name label.
        nodeGroups.append('text')
          .text(function (d) {
            var name = d.name;
            return name.length > 20 ? name.substring(0, 18) + '...' : name;
          })
          .attr('text-anchor', 'middle')
          .attr('dy', -4)
          .attr('font-size', '11px')
          .attr('font-weight', '600')
          .attr('fill', function (d) { return d.color.text; });

        // Subtitle: row count + column count.
        nodeGroups.append('text')
          .text(function (d) {
            return d.rowCount + ' rows \u00B7 ' + d.columnCount + ' cols';
          })
          .attr('text-anchor', 'middle')
          .attr('dy', 12)
          .attr('font-size', '9px')
          .attr('fill', function (d) { return d.color.text; })
          .attr('opacity', 0.8);

        // Click handler — show table detail.
        nodeGroups.on('click', function (event, d) {
          event.stopPropagation();
          showTableDetail(d);
        });

        // Hover highlight.
        nodeGroups
          .on('mouseenter', function () {
            d3.select(this).select('rect')
              .attr('stroke-width', 2)
              .attr('stroke', 'var(--color-accent, #8b5cf6)');
          })
          .on('mouseleave', function () {
            d3.select(this).select('rect')
              .attr('stroke-width', 1)
              .attr('stroke', 'var(--color-edge, #e5e7eb)');
          });

        // Click background to dismiss detail.
        svg.on('click', function () {
          var detail = document.getElementById('table-detail');
          if (detail) {
            detail.classList.add('hidden');
          }
        });

        // Simulation tick.
        simulation.on('tick', function () {
          links
            .attr('x1', function (d) { return d.source.x; })
            .attr('y1', function (d) { return d.source.y; })
            .attr('x2', function (d) { return d.target.x; })
            .attr('y2', function (d) { return d.target.y; });

          linkLabels
            .attr('x', function (d) { return (d.source.x + d.target.x) / 2; })
            .attr('y', function (d) { return (d.source.y + d.target.y) / 2; });

          nodeGroups.attr('transform', function (d) {
            return 'translate(' + d.x + ',' + d.y + ')';
          });
        });

        // Render legend.
        renderLegend(svg, width, height, pluginList);

        // Auto-zoom to fit after stabilization.
        simulation.on('end', function () {
          zoomToFit(svg, g, zoom, width, height, nodes);
        });

        // If simulation settles before end event, zoom after a timeout.
        setTimeout(function () {
          zoomToFit(svg, g, zoom, width, height, nodes);
        }, 3000);
      }

      function zoomToFit(svg, g, zoom, width, height, nodes) {
        if (nodes.length === 0) return;

        var minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
        nodes.forEach(function (d) {
          if (d.x - NODE_W / 2 < minX) minX = d.x - NODE_W / 2;
          if (d.y - NODE_H / 2 < minY) minY = d.y - NODE_H / 2;
          if (d.x + NODE_W / 2 > maxX) maxX = d.x + NODE_W / 2;
          if (d.y + NODE_H / 2 > maxY) maxY = d.y + NODE_H / 2;
        });

        var graphW = maxX - minX + 80;
        var graphH = maxY - minY + 80;
        var scale = Math.min(width / graphW, height / graphH, 1.5);
        var tx = (width - graphW * scale) / 2 - minX * scale + 40 * scale;
        var ty = (height - graphH * scale) / 2 - minY * scale + 40 * scale;

        svg.transition().duration(750)
          .call(zoom.transform, d3.zoomIdentity.translate(tx, ty).scale(scale));
      }

      function renderLegend(svg, width, height, plugins) {
        var legendG = svg.append('g')
          .attr('transform', 'translate(16,' + (height - plugins.length * 20 - 16) + ')');

        legendG.append('rect')
          .attr('x', -8)
          .attr('y', -8)
          .attr('width', 120)
          .attr('height', plugins.length * 20 + 16)
          .attr('rx', 6)
          .attr('fill', 'var(--color-surface, white)')
          .attr('stroke', 'var(--color-edge, #e5e7eb)')
          .attr('opacity', 0.9);

        plugins.forEach(function (p, i) {
          var row = legendG.append('g')
            .attr('transform', 'translate(0,' + (i * 20) + ')');

          row.append('rect')
            .attr('width', 12)
            .attr('height', 12)
            .attr('rx', 3)
            .attr('fill', getPluginColor(p).bg);

          row.append('text')
            .text(p)
            .attr('x', 18)
            .attr('y', 10)
            .attr('font-size', '10px')
            .attr('fill', 'var(--color-fg, #374151)');
        });
      }

      function showTableDetail(table) {
        var detail = document.getElementById('table-detail');
        if (!detail) return;

        detail.classList.remove('hidden');

        var color = table.color;
        var html = '<div class="p-6 space-y-4">';

        // Header.
        html += '<div class="flex items-center justify-between">';
        html += '<div class="flex items-center gap-3">';
        html += '<span class="w-3 h-3 rounded" style="background:' + Chronicle.escapeAttr(color.bg) + '"></span>';
        html += '<h3 class="text-lg font-semibold text-fg">' + Chronicle.escapeHtml(table.name) + '</h3>';
        html += '<span class="text-xs text-fg-muted px-2 py-0.5 bg-surface-alt rounded">' + Chronicle.escapeHtml(table.plugin) + '</span>';
        html += '</div>';
        html += '<div class="text-sm text-fg-secondary">';
        html += '<span>' + table.rowCount + ' rows</span>';
        html += ' &middot; <span>' + table.columnCount + ' columns</span>';
        html += ' &middot; <span>' + table.dataSizeKB + ' KB</span>';
        html += '</div>';
        html += '</div>';

        // Column table.
        html += '<div class="overflow-x-auto">';
        html += '<table class="w-full text-sm">';
        html += '<thead><tr class="border-b border-edge text-left text-fg-muted">';
        html += '<th class="pb-2 pr-4">Column</th>';
        html += '<th class="pb-2 pr-4">Type</th>';
        html += '<th class="pb-2 pr-4">Nullable</th>';
        html += '<th class="pb-2 pr-4">Default</th>';
        html += '<th class="pb-2">Key</th>';
        html += '</tr></thead><tbody>';

        for (var i = 0; i < table.columns.length; i++) {
          var col = table.columns[i];
          var rowClass = i % 2 === 0 ? '' : ' class="bg-surface-alt/50"';
          html += '<tr' + rowClass + '>';
          html += '<td class="py-1.5 pr-4 font-mono text-xs">';
          if (col.key === 'PRI') {
            html += '<i class="fa-solid fa-key text-amber-500 mr-1 text-[10px]"></i>';
          } else if (col.key === 'MUL') {
            html += '<i class="fa-solid fa-link text-blue-400 mr-1 text-[10px]"></i>';
          } else if (col.key === 'UNI') {
            html += '<i class="fa-solid fa-fingerprint text-purple-400 mr-1 text-[10px]"></i>';
          }
          html += Chronicle.escapeHtml(col.name) + '</td>';
          html += '<td class="py-1.5 pr-4 font-mono text-xs text-fg-secondary">' + Chronicle.escapeHtml(col.type) + '</td>';
          html += '<td class="py-1.5 pr-4 text-xs">' + (col.nullable ? '<span class="text-fg-muted">yes</span>' : '<span class="text-fg">no</span>') + '</td>';
          html += '<td class="py-1.5 pr-4 font-mono text-xs text-fg-muted">' + (col.default != null ? Chronicle.escapeHtml(col.default) : '<span class="text-fg-muted">&mdash;</span>') + '</td>';
          html += '<td class="py-1.5 text-xs text-fg-muted">' + Chronicle.escapeHtml(col.key || '') + '</td>';
          html += '</tr>';
        }

        html += '</tbody></table></div>';
        html += '</div>';

        detail.innerHTML = html;

        // Scroll into view.
        detail.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      }

      // Start.
      ensureD3(loadAndRender);

      // Store refs for destroy.
      el._dbExplorerState = { simulation: simulation };
    },

    destroy: function (el) {
      if (el._dbExplorerState && el._dbExplorerState.simulation) {
        el._dbExplorerState.simulation.stop();
      }
      el._dbExplorerState = null;
      el.innerHTML = '';
    }
  });
})();
