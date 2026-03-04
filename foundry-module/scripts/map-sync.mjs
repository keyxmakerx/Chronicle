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

  // --- Chronicle → Foundry ---

  /**
   * Create a drawing on the active scene from Chronicle data.
   * @param {object} msg
   * @private
   */
  async _onDrawingCreated(msg) {
    const scene = this._getLinkedScene(msg.campaignId);
    if (!scene) return;

    const drawingData = this._chronicleDrawingToFoundry(msg.payload);
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

    const updates = this._chronicleDrawingToFoundry(msg.payload);
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

  async _onFogUpdated(msg) {
    // Fog of war updates will be handled in Phase 5 polish pass.
    console.log('Chronicle: Fog update received (not yet implemented)', msg);
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
    const drawingId = drawing.getFlag(FLAG_SCOPE, 'drawingId');
    if (!mapId || !drawingId) return;

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
   * @param {object} cd - Chronicle drawing data.
   * @returns {object|null}
   * @private
   */
  _chronicleDrawingToFoundry(cd) {
    if (!cd) return null;

    // Map drawing type to Foundry shape type.
    const shapeTypes = {
      freehand: 'f',
      rectangle: 'r',
      ellipse: 'e',
      polygon: 'p',
      text: 't',
    };

    return {
      shape: {
        type: shapeTypes[cd.drawing_type] || 'f',
        width: cd.width || 100,
        height: cd.height || 100,
        points: cd.points || [],
      },
      x: cd.x || 0,
      y: cd.y || 0,
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
   * @param {DrawingDocument} drawing
   * @returns {object}
   * @private
   */
  _foundryDrawingToChronicle(drawing) {
    const typeMap = { f: 'freehand', r: 'rectangle', e: 'ellipse', p: 'polygon', t: 'text' };

    return {
      drawing_type: typeMap[drawing.shape?.type] || 'freehand',
      points: drawing.shape?.points || [],
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
