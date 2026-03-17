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
    // Refresh on entity updates.
    if (msg.type === 'entity.updated') {
      const entityId = msg.payload?.id;
      if (entityId && this._openWindows.has(entityId)) {
        this._openWindows.get(entityId).refresh();
      }
    }

    // Refresh on relation changes (stock depleted after purchase).
    if (msg.type === 'relation.metadata_updated' || msg.type === 'relation.deleted') {
      for (const window of this._openWindows.values()) {
        window.refresh();
      }
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
    await window.render({ force: true });
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

const { ApplicationV2, HandlebarsApplicationMixin } = foundry.applications.api;

/**
 * ShopWindow is a Foundry ApplicationV2 that displays a shop inventory
 * with drag-and-drop support.
 */
class ShopWindow extends HandlebarsApplicationMixin(ApplicationV2) {
  static DEFAULT_OPTIONS = {
    id: 'chronicle-shop-{id}',
    classes: ['chronicle-shop-window'],
    window: {
      title: 'Shop',
      resizable: true,
    },
    position: {
      width: 400,
      height: 500,
    },
  };

  static PARTS = {
    shop: {
      template: 'modules/chronicle-sync/templates/shop-window.hbs',
    },
  };

  constructor(api, entityId, shopName, onClose) {
    super({
      id: `chronicle-shop-${entityId}`,
      window: { title: `Shop: ${shopName}` },
    });

    this._api = api;
    this._entityId = entityId;
    this._shopName = shopName;
    this._onCloseCallback = onClose;
    this._inventory = [];
    this._entity = null;
  }

  async _prepareContext(options = {}) {
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
          relationId: r.id,
          name: r.targetEntityName || 'Unknown Item',
          image_path: null, // Chronicle entities use FA icons, not image URLs.
          icon: r.targetEntityIcon || 'fa-box',
          color: r.targetEntityColor || '#6b7280',
          type: r.targetEntityType || '',
          price: r.metadata.price,
          currency: r.metadata.currency || 'gp',
          quantity: r.metadata.quantity,
          in_stock: r.metadata.in_stock !== false,
          description: r.metadata.description || '',
        }));

      // Normalize entity fields for template (API uses type_icon/type_color).
      const entity = this._entity ? {
        ...this._entity,
        icon: this._entity.type_icon || 'fa-store',
        color: this._entity.type_color || '#6b7280',
      } : null;

      return {
        entity,
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

  /** Re-fetch data and re-render. */
  async refresh() {
    await this.render({ force: true });
  }

  /**
   * Set up drag-and-drop on shop items after render.
   * @param {object} context - Rendering context.
   * @param {object} options - Render options.
   */
  _onRender(context, options) {
    const el = this.element;

    el.querySelectorAll('.shop-item').forEach((itemEl) => {
      itemEl.setAttribute('draggable', 'true');
      itemEl.addEventListener('dragstart', (event) => {
        const itemId = itemEl.dataset.itemId;
        const relationId = itemEl.dataset.relationId;
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
                shopRelationId: relationId ? parseInt(relationId) : undefined,
                shopPrice: itemData.price,
                shopCurrency: itemData.currency || 'gp',
              },
            },
          })
        );
      });
    });

    // Buy button click handler.
    el.querySelectorAll('.shop-buy-btn').forEach((btn) => {
      btn.addEventListener('click', async () => {
        const itemId = btn.dataset.itemId;
        const relationId = btn.dataset.relationId;
        const itemData = this._inventory.find((item) => item.id === itemId) || {};
        await this._executePurchase(itemData, relationId);
      });
    });
  }

  /**
   * Execute a purchase via the Chronicle API.
   * Creates a transaction record and decrements shop stock.
   * @param {object} itemData - Shop inventory item.
   * @param {string} relationId - Shop inventory relation ID.
   * @private
   */
  async _executePurchase(itemData, relationId) {
    // Determine the buyer (currently controlled character or selected actor).
    const buyer = game.user.character;
    if (!buyer) {
      ui.notifications.warn('No character assigned — select a character in User Configuration to purchase items.');
      return;
    }

    const buyerEntityId = buyer.getFlag(FLAG_SCOPE, 'entityId');
    if (!buyerEntityId) {
      ui.notifications.warn('Your character is not synced with Chronicle. Sync it first to make purchases.');
      return;
    }

    // Check stock.
    if (itemData.quantity !== undefined && itemData.quantity !== null && itemData.quantity <= 0) {
      ui.notifications.warn(`${itemData.name} is out of stock.`);
      return;
    }

    try {
      const result = await this._api.post('/armory/purchase', {
        shop_entity_id: this._entityId,
        item_entity_id: itemData.id,
        buyer_entity_id: buyerEntityId,
        relation_id: relationId ? parseInt(relationId) : 0,
        quantity: 1,
        price_paid: itemData.price ? `${itemData.price} ${itemData.currency || 'gp'}` : '',
        currency: itemData.currency || 'gp',
        price_numeric: typeof itemData.price === 'number' ? itemData.price : parseFloat(itemData.price) || 0,
        transaction_type: 'purchase',
      });

      if (result) {
        ui.notifications.info(`${buyer.name} purchased ${itemData.name}!`);

        // Refresh shop window to show updated stock.
        await this.refresh();
      }
    } catch (err) {
      console.error('Chronicle: Purchase failed', err);
      const msg = err?.message || 'Purchase failed';
      ui.notifications.error(`Purchase failed: ${msg}`);
    }
  }

  async close(options) {
    if (this._onCloseCallback) this._onCloseCallback();
    return super.close(options);
  }
}
