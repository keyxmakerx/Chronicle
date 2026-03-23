/**
 * Chronicle Sync - Map/Scene Sync
 *
 * Real-time bidirectional sync between Chronicle maps and Foundry scenes.
 * Handles drawings, tokens, fog of war, and layer state.
 *
 * Sync flow:
 * - Chronicle → Foundry: Map changes arrive via WebSocket, update Foundry scene.
 * - Foundry → Chronicle: Scene changes detected via Hooks, push to Chronicle API.
 *
 * Token movements are debounced (100ms) during drag; final position is pushed
 * on drag end. Drawings are pushed on mouseup (creation complete).
 */

import { getSetting } from './settings.mjs';

const FLAG_SCOPE = 'chronicle-sync';

/**
 * MapSync handles map ↔ scene synchronization.
 */
export class MapSync {
  constructor() {
    /** @type {import('./api-client.mjs').ChronicleAPI|null} */
    this._api = null;
    this._syncing = false;

    // Debounce timer for token position updates.
    this._tokenDebounceTimers = new Map();

    // Bound hook handlers.
    this._onCreateDrawing = this._handleCreateDrawing.bind(this);
    this._onUpdateDrawing = this._handleUpdateDrawing.bind(this);
    this._onDeleteDrawing = this._handleDeleteDrawing.bind(this);
    this._onCreateToken = this._handleCreateToken.bind(this);
    this._onUpdateToken = this._handleUpdateToken.bind(this);
    this._onDeleteToken = this._handleDeleteToken.bind(this);
  }

  /**
   * Initialize map sync.
   * @param {import('./api-client.mjs').ChronicleAPI} api
   */
  async init(api) {
    this._api = api;

    if (!getSetting('syncMaps')) return;

    // Register Foundry hooks for scene document changes.
    Hooks.on('createDrawing', this._onCreateDrawing);
    Hooks.on('updateDrawing', this._onUpdateDrawing);
    Hooks.on('deleteDrawing', this._onDeleteDrawing);
    Hooks.on('createToken', this._onCreateToken);
    Hooks.on('updateToken', this._onUpdateToken);
    Hooks.on('deleteToken', this._onDeleteToken);

    // Add context menu option to link scenes to Chronicle maps.
    Hooks.on('getSceneNavigationContext', (html, options) => {
      options.push({
        name: 'Link to Chronicle Map',
        icon: '<i class="fas fa-link"></i>',
        condition: () => game.user.isGM,
        callback: async (li) => {
          const sceneId = li.data('sceneId');
          const scene = game.scenes.get(sceneId);
          if (scene) await this._showMapLinkDialog(scene);
        },
      });
    });

    console.log('Chronicle: Map sync initialized');
  }

  /**
   * Handle incoming WebSocket messages for map events.
   * @param {object} msg
   */
  async onMessage(msg) {
    if (!getSetting('syncMaps')) return;

    switch (msg.type) {
      case 'drawing.created':
        await this._onDrawingCreated(msg);
        break;
      case 'drawing.updated':
        await this._onDrawingUpdated(msg);
        break;
      case 'drawing.deleted':
        await this._onDrawingDeleted(msg);
        break;
      case 'token.created':
        await this._onTokenCreated(msg);
        break;
      case 'token.moved':
      case 'token.updated':
        await this._onTokenUpdated(msg);
        break;
      case 'token.deleted':
        await this._onTokenDeleted(msg);
        break;
      case 'fog.updated':
        await this._onFogUpdated(msg);
        break;
      case 'layer.updated':
        console.log('Chronicle: Map layer updated', msg.payload?.name || msg.resourceId);
        break;
    }
  }

  /**
   * Clean up hooks on destroy.
   */
  destroy() {
    Hooks.off('createDrawing', this._onCreateDrawing);
    Hooks.off('updateDrawing', this._onUpdateDrawing);
    Hooks.off('deleteDrawing', this._onDeleteDrawing);
    Hooks.off('createToken', this._onCreateToken);
    Hooks.off('updateToken', this._onUpdateToken);
    Hooks.off('deleteToken', this._onDeleteToken);

    // Clear debounce timers.
    for (const timer of this._tokenDebounceTimers.values()) {
      clearTimeout(timer);
    }
    this._tokenDebounceTimers.clear();
  }

  /**
   * Handle a sync mapping received during initial sync.
   * Processes map, drawing, and token mappings.
   * @param {object} mapping
   */
  async onSyncMapping(mapping) {
    if (!getSetting('syncMaps')) return;

    if (mapping.chronicle_type === 'map') {
      // Link the Foundry scene to the Chronicle map.
      const scene = game.scenes.get(mapping.external_id);
      if (scene && !scene.getFlag(FLAG_SCOPE, 'mapId')) {
        await scene.setFlag(FLAG_SCOPE, 'mapId', mapping.chronicle_id);
        console.log(`Chronicle: Linked scene "${scene.name}" to map ${mapping.chronicle_id}`);
      }
    } else if (mapping.chronicle_type === 'drawing') {
      // Set drawingId flag on the matching Foundry Drawing.
      const scene = canvas.scene;
      if (!scene) return;
      const drawing = scene.drawings.get(mapping.external_id);
      if (drawing && !drawing.getFlag(FLAG_SCOPE, 'drawingId')) {
        this._syncing = true;
        try {
          await drawing.setFlag(FLAG_SCOPE, 'drawingId', mapping.chronicle_id);
        } finally {
          this._syncing = false;
        }
      }
    } else if (mapping.chronicle_type === 'token') {
      // Set tokenId flag on the matching Foundry Token.
      const scene = canvas.scene;
      if (!scene) return;
      const token = scene.tokens.get(mapping.external_id);
      if (token && !token.getFlag(FLAG_SCOPE, 'tokenId')) {
        this._syncing = true;
        try {
          await token.setFlag(FLAG_SCOPE, 'tokenId', mapping.chronicle_id);
        } finally {
          this._syncing = false;
        }
      }
    }
  }

  /**
   * Perform initial map sync on WebSocket connect.
   * Fetches the current state of drawings, tokens, and fog from Chronicle
   * and reconciles with the active Foundry scene.
   */
  async onInitialSync() {
    if (!getSetting('syncMaps')) return;

    const scene = canvas.scene;
    if (!scene) return;

    let mapId = scene.getFlag(FLAG_SCOPE, 'mapId');

    // If no scene is linked, try to auto-link by fetching campaign maps.
    if (!mapId) {
      try {
        const maps = await this._api.get('/maps');
        if (maps && maps.length === 1) {
          mapId = maps[0].id;
          await scene.setFlag(FLAG_SCOPE, 'mapId', mapId);
          console.log(`Chronicle: Auto-linked scene "${scene.name}" to map "${maps[0].name}"`);

          // Create sync mapping on the server.
          await this._api.post('/sync/mappings', {
            chronicle_type: 'map',
            chronicle_id: mapId,
            external_system: 'foundry',
            external_id: scene.id,
            sync_direction: 'both',
          });
        } else if (maps && maps.length > 1) {
          console.warn(
            `Chronicle: ${maps.length} maps found. Right-click a scene in the navigation bar and select "Link to Chronicle Map" to link manually.`
          );
          return;
        } else {
          return;
        }
      } catch (err) {
        console.error('Chronicle: Failed to fetch maps for auto-link', err);
        return;
      }
    }

    // Pull current drawings, tokens, and fog from Chronicle.
    try {
      const [drawings, tokens, fog] = await Promise.all([
        this._api.get(`/maps/${mapId}/drawings`).catch(() => []),
        this._api.get(`/maps/${mapId}/tokens`).catch(() => []),
        this._api.get(`/maps/${mapId}/fog`).catch(() => []),
      ]);

      // Reconcile drawings: add any that are missing from the scene.
      this._syncing = true;
      try {
        for (const cd of (drawings || [])) {
          const existing = scene.drawings.find(
            (d) => d.getFlag(FLAG_SCOPE, 'drawingId') === cd.id
          );
          if (!existing) {
            const drawingData = this._chronicleDrawingToFoundry(cd, scene);
            if (drawingData) {
              await scene.createEmbeddedDocuments('Drawing', [drawingData]);
            }
          }
        }

        // Reconcile tokens: add any that are missing from the scene.
        for (const ct of (tokens || [])) {
          const existing = scene.tokens.find(
            (t) => t.getFlag(FLAG_SCOPE, 'tokenId') === ct.id
          );
          if (!existing) {
            const tokenData = this._chronicleTokenToFoundry(ct, scene);
            if (tokenData) {
              await scene.createEmbeddedDocuments('Token', [tokenData]);
            }
          }
        }
      } finally {
        this._syncing = false;
      }

      // Reconcile fog regions.
      if (fog && fog.length > 0) {
        await this._reconcileFogRegions(scene, fog);
      }

      console.log('Chronicle: Map initial sync complete');
    } catch (err) {
      console.error('Chronicle: Map initial sync failed', err);
    }
  }

  /**
   * Show a dialog for the GM to pick which Chronicle map to link to a scene.
   * @param {Scene} scene
   * @private
   */
  async _showMapLinkDialog(scene) {
    try {
      const maps = await this._api.get('/maps');
      if (!maps || maps.length === 0) {
        ui.notifications.warn('Chronicle: No maps found in this campaign.');
        return;
      }

      const currentMapId = scene.getFlag(FLAG_SCOPE, 'mapId');

      // Build selection options.
      const options = maps.map(
        (m) => `<option value="${m.id}" ${m.id === currentMapId ? 'selected' : ''}>${m.name}</option>`
      ).join('');

      new Dialog({
        title: 'Link to Chronicle Map',
        content: `
          <form>
            <div class="form-group">
              <label>Chronicle Map</label>
              <select name="mapId">${options}</select>
            </div>
          </form>
        `,
        buttons: {
          link: {
            icon: '<i class="fas fa-link"></i>',
            label: 'Link',
            callback: async (html) => {
              const mapId = html.find('[name="mapId"]').val();
              if (mapId) {
                await scene.setFlag(FLAG_SCOPE, 'mapId', mapId);

                // Create sync mapping on the server.
                try {
                  await this._api.post('/sync/mappings', {
                    chronicle_type: 'map',
                    chronicle_id: mapId,
                    external_system: 'foundry',
                    external_id: scene.id,
                    sync_direction: 'both',
                  });
                } catch (err) {
                  console.warn('Chronicle: Failed to create map sync mapping', err);
                }

                ui.notifications.info(`Chronicle: Scene "${scene.name}" linked to map.`);
              }
            },
          },
          unlink: {
            icon: '<i class="fas fa-unlink"></i>',
            label: 'Unlink',
            callback: async () => {
              await scene.unsetFlag(FLAG_SCOPE, 'mapId');
              ui.notifications.info(`Chronicle: Scene "${scene.name}" unlinked.`);
            },
          },
          cancel: {
            icon: '<i class="fas fa-times"></i>',
            label: 'Cancel',
          },
        },
        default: 'link',
      }).render(true);
    } catch (err) {
      console.error('Chronicle: Failed to fetch maps for linking', err);
      ui.notifications.error('Chronicle: Failed to fetch maps. Check console.');
    }
  }

  // --- Chronicle → Foundry ---

  /**
   * Create a drawing on the active scene from Chronicle data.
   * @param {object} msg
   * @private
   */
  async _onDrawingCreated(msg) {
    const scene = this._getLinkedScene(msg.campaignId);
    if (!scene) return;

    const drawingData = this._chronicleDrawingToFoundry(msg.payload, scene);
    if (!drawingData) return;

    this._syncing = true;
    try {
      await scene.createEmbeddedDocuments('Drawing', [drawingData]);
    } finally {
      this._syncing = false;
    }
  }

  async _onDrawingUpdated(msg) {
    const scene = this._getLinkedScene(msg.campaignId);
    if (!scene) return;

    const drawing = scene.drawings.find(
      (d) => d.getFlag(FLAG_SCOPE, 'drawingId') === msg.resourceId
    );
    if (!drawing) return;

    const updates = this._chronicleDrawingToFoundry(msg.payload, scene);
    if (!updates) return;

    this._syncing = true;
    try {
      await drawing.update(updates);
    } finally {
      this._syncing = false;
    }
  }

  async _onDrawingDeleted(msg) {
    const scene = this._getLinkedScene(msg.campaignId);
    if (!scene) return;

    const drawing = scene.drawings.find(
      (d) => d.getFlag(FLAG_SCOPE, 'drawingId') === msg.resourceId
    );
    if (!drawing) return;

    this._syncing = true;
    try {
      await scene.deleteEmbeddedDocuments('Drawing', [drawing.id]);
    } finally {
      this._syncing = false;
    }
  }

  async _onTokenCreated(msg) {
    const scene = this._getLinkedScene(msg.campaignId);
    if (!scene) return;

    const tokenData = this._chronicleTokenToFoundry(msg.payload, scene);
    if (!tokenData) return;

    this._syncing = true;
    try {
      await scene.createEmbeddedDocuments('Token', [tokenData]);
    } finally {
      this._syncing = false;
    }
  }

  async _onTokenUpdated(msg) {
    const scene = this._getLinkedScene(msg.campaignId);
    if (!scene) return;

    const token = scene.tokens.find(
      (t) => t.getFlag(FLAG_SCOPE, 'tokenId') === msg.resourceId
    );
    if (!token) return;

    const updates = this._chronicleTokenToFoundry(msg.payload, scene);
    if (!updates) return;

    this._syncing = true;
    try {
      await token.update(updates);
    } finally {
      this._syncing = false;
    }
  }

  async _onTokenDeleted(msg) {
    const scene = this._getLinkedScene(msg.campaignId);
    if (!scene) return;

    const token = scene.tokens.find(
      (t) => t.getFlag(FLAG_SCOPE, 'tokenId') === msg.resourceId
    );
    if (!token) return;

    this._syncing = true;
    try {
      await scene.deleteEmbeddedDocuments('Token', [token.id]);
    } finally {
      this._syncing = false;
    }
  }

  /**
   * Handle fog-of-war updates from Chronicle.
   * Fetches the full fog state and renders regions as semi-transparent
   * overlay drawings on the active Foundry scene. Chronicle fog regions
   * are polygon-based overlays, distinct from Foundry's vision-based fog.
   * @param {object} msg - WebSocket message with fog event data.
   * @private
   */
  async _onFogUpdated(msg) {
    const scene = this._getLinkedScene(msg.campaignId);
    if (!scene) return;

    const mapId = scene.getFlag(FLAG_SCOPE, 'mapId');
    if (!mapId) return;

    const event = msg.payload?.event;

    // On reset, remove all Chronicle fog drawings from the scene.
    if (event === 'reset') {
      await this._clearFogDrawings(scene);
      return;
    }

    // For create events with region data, add the region directly.
    if (event === 'created' && msg.payload?.region) {
      await this._addFogRegionToScene(scene, msg.payload.region);
      return;
    }

    // Fallback: re-fetch all fog regions and reconcile.
    try {
      const regions = await this._api.get(`/maps/${mapId}/fog`);
      await this._reconcileFogRegions(scene, regions || []);
    } catch (err) {
      console.error('Chronicle: Failed to fetch fog regions', err);
    }
  }

  /**
   * Build the Foundry Drawing data for a Chronicle fog region.
   * Returns null if the region data is invalid.
   * @param {Scene} scene
   * @param {object} region - Chronicle fog region data.
   * @returns {object|null} Foundry Drawing document data.
   * @private
   */
  _createFogDrawingData(scene, region) {
    if (!region?.points) return null;

    const dims = scene.dimensions;
    const points = typeof region.points === 'string'
      ? JSON.parse(region.points)
      : region.points;

    if (!Array.isArray(points) || points.length < 3) return null;

    // Convert percentage coords to pixel coords for Foundry.
    const pixelPoints = points.map((p) => [
      (p.x / 100) * dims.width,
      (p.y / 100) * dims.height,
    ]);

    const fillColor = region.is_explored ? '#00000000' : '#000000';
    const fillAlpha = region.is_explored ? 0 : 0.7;

    return {
      shape: { type: 'p', points: pixelPoints.flat() },
      x: 0,
      y: 0,
      fillColor,
      fillAlpha,
      strokeColor: '#000000',
      strokeAlpha: 0.3,
      strokeWidth: 1,
      flags: { [FLAG_SCOPE]: { fogRegionId: region.id } },
    };
  }

  /**
   * Add a single Chronicle fog region as a Drawing on the Foundry scene.
   * @param {Scene} scene
   * @param {object} region - Chronicle fog region data.
   * @private
   */
  async _addFogRegionToScene(scene, region) {
    const drawingData = this._createFogDrawingData(scene, region);
    if (!drawingData) return;

    this._syncing = true;
    try {
      const [created] = await scene.createEmbeddedDocuments('Drawing', [drawingData]);
      if (created) {
        console.log(`Chronicle: Fog region ${region.id} added to scene`);
      }
    } finally {
      this._syncing = false;
    }
  }

  /**
   * Remove all Chronicle fog overlay drawings from a scene.
   * @param {Scene} scene
   * @private
   */
  async _clearFogDrawings(scene) {
    const fogDrawings = scene.drawings.filter(
      (d) => d.getFlag(FLAG_SCOPE, 'fogRegionId')
    );
    if (fogDrawings.length === 0) return;

    this._syncing = true;
    try {
      await scene.deleteEmbeddedDocuments(
        'Drawing',
        fogDrawings.map((d) => d.id)
      );
      console.log(`Chronicle: Cleared ${fogDrawings.length} fog drawings`);
    } finally {
      this._syncing = false;
    }
  }

  /**
   * Reconcile fog regions from Chronicle with fog drawings on the scene.
   * Adds missing regions and removes stale drawings. Uses batched operations
   * with a single _syncing guard to avoid flag corruption.
   * @param {Scene} scene
   * @param {Array} regions - Chronicle fog regions.
   * @private
   */
  async _reconcileFogRegions(scene, regions) {
    const existingDrawings = scene.drawings.filter(
      (d) => d.getFlag(FLAG_SCOPE, 'fogRegionId')
    );
    const existingIds = new Set(
      existingDrawings.map((d) => d.getFlag(FLAG_SCOPE, 'fogRegionId'))
    );
    const regionIds = new Set(regions.map((r) => r.id));

    // Remove drawings for deleted regions.
    const toDelete = existingDrawings
      .filter((d) => !regionIds.has(d.getFlag(FLAG_SCOPE, 'fogRegionId')))
      .map((d) => d.id);

    // Build drawing data for new regions.
    const toAddData = regions
      .filter((r) => !existingIds.has(r.id))
      .map((r) => this._createFogDrawingData(scene, r))
      .filter(Boolean);

    this._syncing = true;
    try {
      if (toDelete.length > 0) {
        await scene.deleteEmbeddedDocuments('Drawing', toDelete);
      }
      if (toAddData.length > 0) {
        await scene.createEmbeddedDocuments('Drawing', toAddData);
      }
    } finally {
      this._syncing = false;
    }
  }

  // --- Foundry → Chronicle ---

  /**
   * Push a new Foundry drawing to Chronicle.
   * @private
   */
  async _handleCreateDrawing(drawing, options, userId) {
    if (this._syncing || userId !== game.user.id) return;

    const mapId = this._getLinkedMapId(drawing.parent);
    if (!mapId) return;

    // Detect fog-like drawings: dark-filled polygons with high alpha.
    if (this._isFogLikeDrawing(drawing)) {
      await this._handleFogDrawingCreate(drawing, mapId);
      return;
    }

    try {
      const chronicleDrawing = this._foundryDrawingToChronicle(drawing);
      const result = await this._api.post(`/maps/${mapId}/drawings`, chronicleDrawing);

      if (result?.id) {
        this._syncing = true;
        try {
          await drawing.setFlag(FLAG_SCOPE, 'drawingId', result.id);
        } finally {
          this._syncing = false;
        }
      }
    } catch (err) {
      console.error('Chronicle: Failed to push drawing', err);
    }
  }

  async _handleUpdateDrawing(drawing, change, options, userId) {
    if (this._syncing || userId !== game.user.id) return;

    const mapId = this._getLinkedMapId(drawing.parent);
    const drawingId = drawing.getFlag(FLAG_SCOPE, 'drawingId');
    if (!mapId || !drawingId) return;

    try {
      const chronicleDrawing = this._foundryDrawingToChronicle(drawing);
      await this._api.put(`/maps/${mapId}/drawings/${drawingId}`, chronicleDrawing);
    } catch (err) {
      console.error('Chronicle: Failed to update drawing', err);
    }
  }

  async _handleDeleteDrawing(drawing, options, userId) {
    if (this._syncing || userId !== game.user.id) return;

    const mapId = this._getLinkedMapId(drawing.parent);
    if (!mapId) return;

    // Check if this is a Chronicle fog drawing.
    const fogRegionId = drawing.getFlag(FLAG_SCOPE, 'fogRegionId');
    if (fogRegionId) {
      await this._handleFogDrawingDelete(mapId, fogRegionId);
      return;
    }

    const drawingId = drawing.getFlag(FLAG_SCOPE, 'drawingId');
    if (!drawingId) return;

    try {
      await this._api.delete(`/maps/${mapId}/drawings/${drawingId}`);
    } catch (err) {
      console.warn('Chronicle: Failed to delete drawing on Chronicle', err);
    }
  }

  async _handleCreateToken(token, options, userId) {
    if (this._syncing || userId !== game.user.id) return;

    const mapId = this._getLinkedMapId(token.parent);
    if (!mapId) return;

    try {
      const chronicleToken = this._foundryTokenToChronicle(token);
      const result = await this._api.post(`/maps/${mapId}/tokens`, chronicleToken);

      if (result?.id) {
        this._syncing = true;
        try {
          await token.setFlag(FLAG_SCOPE, 'tokenId', result.id);
        } finally {
          this._syncing = false;
        }
      }
    } catch (err) {
      console.error('Chronicle: Failed to push token', err);
    }
  }

  async _handleUpdateToken(token, change, options, userId) {
    if (this._syncing || userId !== game.user.id) return;

    const mapId = this._getLinkedMapId(token.parent);
    const tokenId = token.getFlag(FLAG_SCOPE, 'tokenId');
    if (!mapId || !tokenId) return;

    // Debounce position-only updates during drag.
    const isPositionOnly = change.x !== undefined || change.y !== undefined;
    if (isPositionOnly && Object.keys(change).every((k) => ['x', 'y', '_id'].includes(k))) {
      this._debounceTokenPosition(mapId, tokenId, token);
      return;
    }

    try {
      const chronicleToken = this._foundryTokenToChronicle(token);
      await this._api.put(`/maps/${mapId}/tokens/${tokenId}`, chronicleToken);
    } catch (err) {
      console.error('Chronicle: Failed to update token', err);
    }
  }

  async _handleDeleteToken(token, options, userId) {
    if (this._syncing || userId !== game.user.id) return;

    const mapId = this._getLinkedMapId(token.parent);
    const tokenId = token.getFlag(FLAG_SCOPE, 'tokenId');
    if (!mapId || !tokenId) return;

    try {
      await this._api.delete(`/maps/${mapId}/tokens/${tokenId}`);
    } catch (err) {
      console.warn('Chronicle: Failed to delete token on Chronicle', err);
    }
  }

  // --- Fog: Foundry → Chronicle ---

  /**
   * Determine if a Foundry Drawing looks like a fog region: polygon shape
   * with a dark fill color and high fill opacity.
   * @param {DrawingDocument} drawing
   * @returns {boolean}
   * @private
   */
  _isFogLikeDrawing(drawing) {
    // Already flagged as a fog region — always treat as fog.
    if (drawing.getFlag(FLAG_SCOPE, 'fogRegionId')) return true;

    // Must be a polygon.
    if (drawing.shape?.type !== 'p') return false;

    // Check for dark fill with significant opacity.
    const alpha = drawing.fillAlpha ?? 0;
    if (alpha < 0.5) return false;

    const color = drawing.fillColor || '';
    return this._isDarkColor(color);
  }

  /**
   * Check if a hex color string is dark (luminance < 0.15).
   * @param {string} hex - CSS hex color (#RGB or #RRGGBB).
   * @returns {boolean}
   * @private
   */
  _isDarkColor(hex) {
    if (!hex || hex === '#000000' || hex === '#000') return true;

    const match = hex.match(/^#?([0-9a-f]{3,6})$/i);
    if (!match) return false;

    let r, g, b;
    const h = match[1];
    if (h.length === 3) {
      r = parseInt(h[0] + h[0], 16);
      g = parseInt(h[1] + h[1], 16);
      b = parseInt(h[2] + h[2], 16);
    } else {
      r = parseInt(h.slice(0, 2), 16);
      g = parseInt(h.slice(2, 4), 16);
      b = parseInt(h.slice(4, 6), 16);
    }

    // Relative luminance.
    const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
    return luminance < 0.15;
  }

  /**
   * Push a fog-like Foundry drawing to Chronicle as a fog region.
   * @param {DrawingDocument} drawing
   * @param {string} mapId
   * @private
   */
  async _handleFogDrawingCreate(drawing, mapId) {
    try {
      const fogRegion = this._foundryDrawingToFogRegion(drawing);
      if (!fogRegion) return;

      const result = await this._api.post(`/maps/${mapId}/fog`, fogRegion);

      if (result?.id) {
        this._syncing = true;
        try {
          await drawing.setFlag(FLAG_SCOPE, 'fogRegionId', result.id);
        } finally {
          this._syncing = false;
        }
      }
    } catch (err) {
      console.error('Chronicle: Failed to push fog region to Chronicle', err);
    }
  }

  /**
   * Delete a Chronicle fog region when its Foundry drawing is removed.
   * @param {string} mapId
   * @param {string} fogRegionId
   * @private
   */
  async _handleFogDrawingDelete(mapId, fogRegionId) {
    try {
      await this._api.delete(`/maps/${mapId}/fog/${fogRegionId}`);
      console.log(`Chronicle: Fog region ${fogRegionId} deleted`);
    } catch (err) {
      console.warn('Chronicle: Failed to delete fog region from Chronicle', err);
    }
  }

  /**
   * Convert a Foundry polygon Drawing to a Chronicle fog region object.
   * Translates pixel coordinates to percentage-based coordinates.
   * @param {DrawingDocument} drawing
   * @returns {object|null} - Chronicle fog region data, or null if invalid.
   * @private
   */
  _foundryDrawingToFogRegion(drawing) {
    const scene = drawing.parent;
    if (!scene) return null;

    const dims = scene.dimensions;
    const rawPoints = drawing.shape?.points || [];
    if (rawPoints.length < 6) return null; // Need at least 3 x,y pairs.

    // Foundry stores polygon points as a flat array [x1,y1,x2,y2,...].
    // Drawing coordinates are relative to drawing.x, drawing.y.
    const points = [];
    for (let i = 0; i < rawPoints.length; i += 2) {
      points.push({
        x: ((rawPoints[i] + (drawing.x || 0)) / dims.width) * 100,
        y: ((rawPoints[i + 1] + (drawing.y || 0)) / dims.height) * 100,
      });
    }

    // Determine explored state: fully opaque dark = unexplored fog,
    // semi-transparent = explored (revealed) area.
    const isExplored = (drawing.fillAlpha ?? 0) < 0.9;

    return {
      points: JSON.stringify(points),
      is_explored: isExplored,
    };
  }

  // --- Helpers ---

  /**
   * Debounce token position updates (100ms) to avoid flooding the API.
   * @private
   */
  _debounceTokenPosition(mapId, tokenId, token) {
    const key = `${mapId}:${tokenId}`;
    if (this._tokenDebounceTimers.has(key)) {
      clearTimeout(this._tokenDebounceTimers.get(key));
    }

    this._tokenDebounceTimers.set(
      key,
      setTimeout(async () => {
        this._tokenDebounceTimers.delete(key);
        try {
          const scene = token.parent;
          // Convert Foundry pixel coords to percentage.
          const x = (token.x / scene.dimensions.width) * 100;
          const y = (token.y / scene.dimensions.height) * 100;

          await this._api.patch(`/maps/${mapId}/tokens/${tokenId}/position`, { x, y });
        } catch (err) {
          console.error('Chronicle: Failed to push token position', err);
        }
      }, 100)
    );
  }

  /**
   * Find the Foundry scene linked to a Chronicle campaign.
   * @param {string} campaignId
   * @returns {Scene|null}
   * @private
   */
  _getLinkedScene(campaignId) {
    // Use the currently viewed scene if it's linked.
    const scene = canvas.scene;
    if (scene?.getFlag(FLAG_SCOPE, 'mapId')) {
      return scene;
    }
    return null;
  }

  /**
   * Get the Chronicle map ID linked to a Foundry scene.
   * @param {Scene} scene
   * @returns {string|null}
   * @private
   */
  _getLinkedMapId(scene) {
    return scene?.getFlag(FLAG_SCOPE, 'mapId') || null;
  }

  /**
   * Convert a Chronicle drawing to Foundry Drawing document data.
   * Converts percentage-based coordinates to pixel coordinates.
   * @param {object} cd - Chronicle drawing data.
   * @param {Scene} scene - Target scene for coordinate conversion.
   * @returns {object|null}
   * @private
   */
  _chronicleDrawingToFoundry(cd, scene) {
    if (!cd) return null;

    const dims = scene.dimensions;

    // Map drawing type to Foundry shape type.
    const shapeTypes = {
      freehand: 'f',
      rectangle: 'r',
      ellipse: 'e',
      polygon: 'p',
      text: 't',
    };

    // Convert percentage coords to pixel coords.
    const x = ((cd.x || 0) / 100) * dims.width;
    const y = ((cd.y || 0) / 100) * dims.height;
    const width = ((cd.width || 1) / 100) * dims.width;
    const height = ((cd.height || 1) / 100) * dims.height;

    // Convert polygon points from percentage to pixel coordinates.
    let points = cd.points || [];
    if (Array.isArray(points) && points.length > 0) {
      points = points.map((val, i) =>
        i % 2 === 0
          ? (val / 100) * dims.width
          : (val / 100) * dims.height
      );
    }

    return {
      shape: {
        type: shapeTypes[cd.drawing_type] || 'f',
        width,
        height,
        points,
      },
      x,
      y,
      strokeColor: cd.stroke_color || '#000000',
      strokeAlpha: 1,
      strokeWidth: cd.stroke_width || 2,
      fillColor: cd.fill_color || '',
      fillAlpha: cd.fill_alpha || 0.5,
      rotation: cd.rotation || 0,
      text: cd.text_content || '',
      fontSize: cd.font_size || 48,
      hidden: cd.visibility === 'dm_only',
      flags: {
        [FLAG_SCOPE]: {
          drawingId: cd.id,
        },
      },
    };
  }

  /**
   * Convert a Foundry Drawing to Chronicle drawing data.
   * Converts pixel coordinates to percentage-based coordinates.
   * @param {DrawingDocument} drawing
   * @returns {object}
   * @private
   */
  _foundryDrawingToChronicle(drawing) {
    const scene = drawing.parent;
    const dims = scene.dimensions;
    const typeMap = { f: 'freehand', r: 'rectangle', e: 'ellipse', p: 'polygon', t: 'text' };

    // Convert pixel coords to percentage.
    const x = (drawing.x / dims.width) * 100;
    const y = (drawing.y / dims.height) * 100;
    const width = ((drawing.shape?.width || 0) / dims.width) * 100;
    const height = ((drawing.shape?.height || 0) / dims.height) * 100;

    // Convert polygon points from pixel to percentage coordinates.
    let points = drawing.shape?.points || [];
    if (Array.isArray(points) && points.length > 0) {
      points = points.map((val, i) =>
        i % 2 === 0
          ? (val / dims.width) * 100
          : (val / dims.height) * 100
      );
    }

    return {
      drawing_type: typeMap[drawing.shape?.type] || 'freehand',
      x,
      y,
      width,
      height,
      points,
      stroke_color: drawing.strokeColor || '#000000',
      stroke_width: drawing.strokeWidth || 2,
      fill_color: drawing.fillColor || null,
      fill_alpha: drawing.fillAlpha || 0.5,
      text_content: drawing.text || '',
      font_size: drawing.fontSize || 48,
      rotation: drawing.rotation || 0,
      visibility: drawing.hidden ? 'dm_only' : 'everyone',
    };
  }

  /**
   * Convert a Chronicle token to Foundry Token document data.
   * @param {object} ct - Chronicle token data.
   * @param {Scene} scene - Target scene for coordinate conversion.
   * @returns {object|null}
   * @private
   */
  _chronicleTokenToFoundry(ct, scene) {
    if (!ct) return null;

    // Convert percentage coords to pixel coords.
    const dims = scene.dimensions;
    const x = (ct.x / 100) * dims.width;
    const y = (ct.y / 100) * dims.height;

    return {
      name: ct.name || 'Token',
      x,
      y,
      texture: { src: ct.image_path || '' },
      width: ct.width || 1,
      height: ct.height || 1,
      rotation: ct.rotation || 0,
      hidden: ct.is_hidden || false,
      elevation: ct.elevation || 0,
      'bar1.value': ct.bar1_value,
      'bar1.max': ct.bar1_max,
      'bar2.value': ct.bar2_value,
      'bar2.max': ct.bar2_max,
      flags: {
        [FLAG_SCOPE]: {
          tokenId: ct.id,
          entityId: ct.entity_id || null,
        },
      },
    };
  }

  /**
   * Convert a Foundry Token to Chronicle token data.
   * @param {TokenDocument} token
   * @returns {object}
   * @private
   */
  _foundryTokenToChronicle(token) {
    const scene = token.parent;
    const dims = scene.dimensions;

    return {
      name: token.name || 'Token',
      image_path: token.texture?.src || '',
      x: (token.x / dims.width) * 100,
      y: (token.y / dims.height) * 100,
      width: token.width || 1,
      height: token.height || 1,
      rotation: token.rotation || 0,
      is_hidden: token.hidden || false,
      elevation: token.elevation || 0,
      bar1_value: token.bar1?.value,
      bar1_max: token.bar1?.max,
      bar2_value: token.bar2?.value,
      bar2_max: token.bar2?.max,
    };
  }
}
