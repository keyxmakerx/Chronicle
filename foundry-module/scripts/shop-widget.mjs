/**
 * Chronicle Sync - Shop Widget
 *
 * Renders a draggable shop window in Foundry VTT that displays a Chronicle
 * shop entity's inventory. Items can be dragged from the shop onto character
 * sheets, automatically creating Foundry Items on the target actor.
 *
 * The widget fetches inventory from Chronicle's entity relations API and
 * updates in real-time via WebSocket events.
 */

import { getSetting } from './settings.mjs';

const FLAG_SCOPE = 'chronicle-sync';

/**
 * ShopWidget manages the shop window UI and drag-and-drop.
 */
export class ShopWidget {
  constructor() {
    /** @type {import('./api-client.mjs').ChronicleAPI|null} */
    this._api = null;

    /** @type {Map<string, ShopWindow>} Open shop windows keyed by entity ID. */
    this._openWindows = new Map();
  }

  /**
   * Initialize the shop widget module.
   * @param {import('./api-client.mjs').ChronicleAPI} api
   */
  async init(api) {
    this._api = api;

    // Add context menu option to JournalEntries linked to shop entities.
    Hooks.on('getJournalEntryContext', (html, options) => {
      options.push({
        name: 'Open Chronicle Shop',
        icon: '<i class="fas fa-store"></i>',
        condition: (li) => {
          const id = li.data('documentId');
          const journal = game.journal.get(id);
          return journal?.getFlag(FLAG_SCOPE, 'entityType') === 'Shop';
        },
        callback: async (li) => {
          const id = li.data('documentId');
          const journal = game.journal.get(id);
          const entityId = journal?.getFlag(FLAG_SCOPE, 'entityId');
          if (entityId) await this.openShop(entityId, journal.name);
        },
      });
    });

    console.log('Chronicle: Shop widget initialized');
  }

  /**
   * Handle incoming WebSocket messages for shop inventory changes.
   * @param {object} msg
   */
  onMessage(msg) {
    if (msg.type !== 'entity.updated') return;

    // Refresh any open shop windows for the updated entity.
    const entityId = msg.payload?.id;
    if (entityId && this._openWindows.has(entityId)) {
      this._openWindows.get(entityId).refresh();
    }
  }

  /**
   * Open a shop window for a Chronicle shop entity.
   * @param {string} entityId
   * @param {string} shopName
   */
  async openShop(entityId, shopName) {
    // Don't open duplicate windows.
    if (this._openWindows.has(entityId)) {
      this._openWindows.get(entityId).bringToTop();
      return;
    }

    const window = new ShopWindow(this._api, entityId, shopName, () => {
      this._openWindows.delete(entityId);
    });

    this._openWindows.set(entityId, window);
    await window.render(true);
  }

  /**
   * Clean up on destroy.
   */
  destroy() {
    for (const window of this._openWindows.values()) {
      window.close();
    }
    this._openWindows.clear();
  }
}

/**
 * ShopWindow is a Foundry ApplicationV2 that displays a shop inventory
 * with drag-and-drop support.
 */
class ShopWindow extends Application {
  constructor(api, entityId, shopName, onClose) {
    super({
      title: `Shop: ${shopName}`,
      width: 400,
      height: 500,
      resizable: true,
      classes: ['chronicle-shop-window'],
    });

    this._api = api;
    this._entityId = entityId;
    this._shopName = shopName;
    this._onCloseCallback = onClose;
    this._inventory = [];
    this._entity = null;
  }

  static get defaultOptions() {
    return foundry.utils.mergeObject(super.defaultOptions, {
      template: 'modules/chronicle-sync/templates/shop-window.hbs',
      classes: ['chronicle-shop-window'],
    });
  }

  async getData() {
    try {
      // Fetch entity data.
      this._entity = await this._api.get(`/entities/${this._entityId}`);

      // Fetch entity relations with metadata (price, quantity, stock status).
      const relations = await this._api.get(`/entities/${this._entityId}/relations`);

      // Filter for inventory-type relations: those with metadata containing
      // price or quantity fields indicate shop inventory items.
      this._inventory = (relations || [])
        .filter((r) => r.metadata && (r.metadata.price !== undefined || r.metadata.quantity !== undefined))
        .map((r) => ({
          id: r.targetEntityId,
          name: r.targetEntityName || 'Unknown Item',
          image_path: null, // Chronicle entities use FA icons, not image URLs.
          icon: r.targetEntityIcon || 'fa-box',
          color: r.targetEntityColor || '#6b7280',
          type: r.targetEntityType || '',
          price: r.metadata.price,
          quantity: r.metadata.quantity,
          in_stock: r.metadata.in_stock !== false,
          description: r.metadata.description || '',
        }));

      return {
        entity: this._entity,
        inventory: this._inventory,
        shopName: this._shopName,
      };
    } catch (err) {
      console.error('Chronicle: Failed to load shop data', err);
      return {
        entity: null,
        inventory: [],
        shopName: this._shopName,
        error: 'Failed to load shop inventory',
      };
    }
  }

  async refresh() {
    await this.render(false);
  }

  activateListeners(html) {
    super.activateListeners(html);

    // Make shop items draggable.
    html.find('.shop-item').each((i, el) => {
      el.setAttribute('draggable', true);
      el.addEventListener('dragstart', (event) => {
        const itemId = el.dataset.itemId;
        const itemData = this._inventory.find((item) => item.id === itemId) || {};
        event.dataTransfer.setData(
          'text/plain',
          JSON.stringify({
            type: 'Item',
            name: itemData.name || 'Unknown Item',
            img: itemData.image_path || 'icons/svg/item-bag.svg',
            system: {},
            flags: {
              [FLAG_SCOPE]: {
                shopEntityId: this._entityId,
                chronicleItemId: itemId,
              },
            },
          })
        );
      });
    });
  }

  async close(options) {
    if (this._onCloseCallback) this._onCloseCallback();
    return super.close(options);
  }
}
