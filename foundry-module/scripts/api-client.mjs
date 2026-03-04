/**
 * Chronicle Sync - API Client
 *
 * Handles both REST API calls and WebSocket connection to the Chronicle server.
 * Provides auto-reconnect with exponential backoff for WebSocket, and a
 * message queue for offline buffering.
 */

import { getSetting } from './settings.mjs';

/**
 * ChronicleAPI handles all communication with the Chronicle backend.
 * Combines REST fetch calls and a persistent WebSocket connection.
 */
export class ChronicleAPI {
  constructor() {
    /** @type {WebSocket|null} */
    this._ws = null;

    /** @type {Map<string, Set<Function>>} */
    this._listeners = new Map();

    /** @type {Array<object>} Messages queued while disconnected. */
    this._messageQueue = [];

    /** @type {number} Current reconnect delay in ms. */
    this._reconnectDelay = 1000;

    /** @type {number|null} Reconnect timer ID. */
    this._reconnectTimer = null;

    /** @type {boolean} Whether we should be connected. */
    this._shouldConnect = false;

    /** @type {string} Connection state: 'disconnected' | 'connecting' | 'connected'. */
    this.state = 'disconnected';
  }

  // --- REST API ---

  /**
   * Make an authenticated REST API call to Chronicle.
   * @param {string} path - API path (e.g., '/entities').
   * @param {object} [options] - Fetch options.
   * @returns {Promise<any>} Parsed JSON response.
   */
  async fetch(path, options = {}) {
    const baseUrl = getSetting('apiUrl').replace(/\/+$/, '');
    const apiKey = getSetting('apiKey');
    const campaignId = getSetting('campaignId');

    const url = `${baseUrl}/api/v1/campaigns/${campaignId}${path}`;

    const headers = {
      'Authorization': `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
      ...options.headers,
    };

    const response = await fetch(url, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const errorBody = await response.text();
      throw new Error(`Chronicle API error ${response.status}: ${errorBody}`);
    }

    // Handle 204 No Content.
    if (response.status === 204) return null;

    return response.json();
  }

  /**
   * GET request to the Chronicle API.
   * @param {string} path
   * @returns {Promise<any>}
   */
  async get(path) {
    return this.fetch(path, { method: 'GET' });
  }

  /**
   * POST request to the Chronicle API.
   * @param {string} path
   * @param {object} body
   * @returns {Promise<any>}
   */
  async post(path, body) {
    return this.fetch(path, {
      method: 'POST',
      body: JSON.stringify(body),
    });
  }

  /**
   * PUT request to the Chronicle API.
   * @param {string} path
   * @param {object} body
   * @returns {Promise<any>}
   */
  async put(path, body) {
    return this.fetch(path, {
      method: 'PUT',
      body: JSON.stringify(body),
    });
  }

  /**
   * PATCH request to the Chronicle API.
   * @param {string} path
   * @param {object} body
   * @returns {Promise<any>}
   */
  async patch(path, body) {
    return this.fetch(path, {
      method: 'PATCH',
      body: JSON.stringify(body),
    });
  }

  /**
   * DELETE request to the Chronicle API.
   * @param {string} path
   * @returns {Promise<any>}
   */
  async delete(path) {
    return this.fetch(path, { method: 'DELETE' });
  }

  /**
   * Upload a file to the Chronicle media API.
   * @param {File|Blob} file
   * @param {string} [filename]
   * @returns {Promise<any>}
   */
  async uploadMedia(file, filename) {
    const baseUrl = getSetting('apiUrl').replace(/\/+$/, '');
    const apiKey = getSetting('apiKey');
    const campaignId = getSetting('campaignId');

    const formData = new FormData();
    formData.append('file', file, filename || file.name);

    const response = await fetch(
      `${baseUrl}/api/v1/campaigns/${campaignId}/media`,
      {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${apiKey}` },
        body: formData,
      }
    );

    if (!response.ok) {
      throw new Error(`Media upload failed: ${response.status}`);
    }

    return response.json();
  }

  // --- WebSocket ---

  /**
   * Connect to the Chronicle WebSocket server.
   * Auto-reconnects with exponential backoff on disconnect.
   */
  connect() {
    this._shouldConnect = true;
    this._doConnect();
  }

  /**
   * Disconnect from the Chronicle WebSocket server.
   */
  disconnect() {
    this._shouldConnect = false;
    this._clearReconnect();
    if (this._ws) {
      this._ws.close(1000, 'Client disconnect');
      this._ws = null;
    }
    this._setState('disconnected');
  }

  /**
   * Send a message over the WebSocket connection.
   * If disconnected, queues the message for delivery on reconnect.
   * @param {object} message
   */
  send(message) {
    if (this._ws && this._ws.readyState === WebSocket.OPEN) {
      this._ws.send(JSON.stringify(message));
    } else {
      this._messageQueue.push(message);
    }
  }

  /**
   * Register a listener for a specific message type.
   * @param {string} type - Message type (e.g., 'entity.updated').
   * @param {Function} callback
   */
  on(type, callback) {
    if (!this._listeners.has(type)) {
      this._listeners.set(type, new Set());
    }
    this._listeners.get(type).add(callback);
  }

  /**
   * Remove a listener for a specific message type.
   * @param {string} type
   * @param {Function} callback
   */
  off(type, callback) {
    const listeners = this._listeners.get(type);
    if (listeners) {
      listeners.delete(callback);
    }
  }

  /**
   * Internal: establish the WebSocket connection.
   * @private
   */
  _doConnect() {
    if (this._ws) return;

    const baseUrl = getSetting('apiUrl').replace(/\/+$/, '');
    const apiKey = getSetting('apiKey');

    // Convert http(s) to ws(s).
    const wsUrl = baseUrl.replace(/^http/, 'ws') + `/ws?token=${encodeURIComponent(apiKey)}`;

    this._setState('connecting');

    try {
      this._ws = new WebSocket(wsUrl);
    } catch (err) {
      console.error('Chronicle: WebSocket creation failed', err);
      this._scheduleReconnect();
      return;
    }

    this._ws.onopen = () => {
      console.log('Chronicle: WebSocket connected');
      this._setState('connected');
      this._reconnectDelay = 1000; // Reset backoff.

      // Flush queued messages.
      while (this._messageQueue.length > 0) {
        const msg = this._messageQueue.shift();
        this._ws.send(JSON.stringify(msg));
      }

      // Notify listeners.
      this._emit('sync.status', { status: 'connected' });
    };

    this._ws.onclose = (event) => {
      console.log(`Chronicle: WebSocket closed (code=${event.code})`);
      this._ws = null;
      this._setState('disconnected');

      if (this._shouldConnect && event.code !== 1000) {
        this._scheduleReconnect();
      }
    };

    this._ws.onerror = (error) => {
      console.error('Chronicle: WebSocket error', error);
      // onclose will fire after onerror, triggering reconnect.
    };

    this._ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        this._emit(msg.type, msg);
      } catch (err) {
        console.warn('Chronicle: Invalid WebSocket message', err);
      }
    };
  }

  /**
   * Schedule a reconnection attempt with exponential backoff.
   * @private
   */
  _scheduleReconnect() {
    this._clearReconnect();
    this._setState('reconnecting');

    const delay = Math.min(this._reconnectDelay, 30000); // Cap at 30s.
    console.log(`Chronicle: Reconnecting in ${delay}ms`);

    this._reconnectTimer = setTimeout(() => {
      this._reconnectTimer = null;
      this._doConnect();
    }, delay);

    this._reconnectDelay *= 2; // Exponential backoff.
  }

  /**
   * Clear any pending reconnect timer.
   * @private
   */
  _clearReconnect() {
    if (this._reconnectTimer) {
      clearTimeout(this._reconnectTimer);
      this._reconnectTimer = null;
    }
  }

  /**
   * Update connection state and notify listeners.
   * @param {string} state
   * @private
   */
  _setState(state) {
    this.state = state;
  }

  /**
   * Emit a message to registered listeners.
   * @param {string} type
   * @param {object} data
   * @private
   */
  _emit(type, data) {
    const listeners = this._listeners.get(type);
    if (listeners) {
      for (const cb of listeners) {
        try {
          cb(data);
        } catch (err) {
          console.error(`Chronicle: Listener error for ${type}`, err);
        }
      }
    }

    // Also emit to wildcard listeners.
    const wildcardListeners = this._listeners.get('*');
    if (wildcardListeners) {
      for (const cb of wildcardListeners) {
        try {
          cb(data);
        } catch (err) {
          console.error('Chronicle: Wildcard listener error', err);
        }
      }
    }
  }
}
