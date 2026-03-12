/**
 * Chronicle Sync - API Client
 *
 * Handles both REST API calls and WebSocket connection to the Chronicle server.
 * Provides auto-reconnect with exponential backoff for WebSocket, a message
 * queue for offline buffering, health metrics, error logging, and a retry
 * queue for failed write operations.
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

    /** @type {Set<Function>} Callbacks invoked when connection state changes. */
    this._stateChangeCallbacks = new Set();

    // --- Health metrics (F-QoL) ---

    /** @type {object} Connection and sync health metrics. */
    this.health = {
      /** Total successful REST API calls this session. */
      restSuccessCount: 0,
      /** Total failed REST API calls this session. */
      restErrorCount: 0,
      /** Total WebSocket reconnection attempts this session. */
      reconnectAttempts: 0,
      /** Timestamp (ms) of when the current connection was established. */
      connectedSince: null,
      /** Total time connected (ms), excluding current connection. */
      totalConnectedMs: 0,
      /** Timestamp (ms) of last successful REST call. */
      lastRestSuccess: null,
      /** Timestamp (ms) of last REST error. */
      lastRestError: null,
    };

    /** @type {Array<{time: number, level: string, method: string, path: string, status: number|null, message: string}>} */
    this._errorLog = [];

    /** @type {number} Maximum error log entries. */
    this._maxErrorLogEntries = 50;

    /** @type {Array<{method: string, path: string, body: string|null, retries: number, maxRetries: number}>} */
    this._retryQueue = [];

    /** @type {boolean} Whether we're currently processing the retry queue. */
    this._retryProcessing = false;
  }

  // --- REST API ---

  /**
   * Make an authenticated REST API call to Chronicle.
   * Tracks health metrics and logs errors.
   * @param {string} path - API path (e.g., '/entities').
   * @param {object} [options] - Fetch options.
   * @returns {Promise<any>} Parsed JSON response.
   */
  async fetch(path, options = {}) {
    const baseUrl = getSetting('apiUrl').replace(/\/+$/, '');
    const apiKey = getSetting('apiKey');
    const campaignId = getSetting('campaignId');

    const url = `${baseUrl}/api/v1/campaigns/${campaignId}${path}`;
    const method = options.method || 'GET';

    const headers = {
      'Authorization': `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
      ...options.headers,
    };

    let response;
    try {
      response = await fetch(url, {
        ...options,
        headers,
      });
    } catch (err) {
      // Network error (no response at all).
      this.health.restErrorCount++;
      this.health.lastRestError = Date.now();
      this._logError('error', method, path, null, err.message || 'Network error');
      throw err;
    }

    if (!response.ok) {
      const errorBody = await response.text();
      this.health.restErrorCount++;
      this.health.lastRestError = Date.now();
      this._logError('error', method, path, response.status, errorBody);
      throw new Error(`Chronicle API error ${response.status}: ${errorBody}`);
    }

    // Success.
    this.health.restSuccessCount++;
    this.health.lastRestSuccess = Date.now();

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
      this.health.restErrorCount++;
      this.health.lastRestError = Date.now();
      this._logError('error', 'POST', '/media', response.status, 'Media upload failed');
      throw new Error(`Media upload failed: ${response.status}`);
    }

    this.health.restSuccessCount++;
    this.health.lastRestSuccess = Date.now();
    return response.json();
  }

  // --- Retry queue (F-QoL) ---

  /**
   * Queue a failed write operation for retry on reconnect.
   * Only queues POST/PUT/PATCH/DELETE — never retries GETs.
   * @param {string} method - HTTP method.
   * @param {string} path - API path.
   * @param {object|null} body - Request body (will be JSON-stringified).
   * @param {number} [maxRetries=3] - Maximum retry attempts.
   */
  queueForRetry(method, path, body = null, maxRetries = 3) {
    if (method === 'GET') return; // Never retry reads.
    if (this._retryQueue.length >= 50) {
      // Cap queue size to prevent memory issues.
      this._logError('warn', method, path, null, 'Retry queue full, dropping operation');
      return;
    }
    this._retryQueue.push({
      method,
      path,
      body: body ? JSON.stringify(body) : null,
      retries: 0,
      maxRetries,
    });
    console.log(`Chronicle: Queued ${method} ${path} for retry (${this._retryQueue.length} pending)`);
  }

  /**
   * Process the retry queue. Called after reconnection.
   * @returns {Promise<{success: number, failed: number}>}
   */
  async processRetryQueue() {
    if (this._retryProcessing || this._retryQueue.length === 0) {
      return { success: 0, failed: 0 };
    }

    this._retryProcessing = true;
    let success = 0;
    let failed = 0;

    // Process a snapshot of the queue.
    const pending = [...this._retryQueue];
    this._retryQueue = [];

    for (const item of pending) {
      try {
        await this.fetch(item.path, {
          method: item.method,
          body: item.body,
        });
        success++;
      } catch (err) {
        item.retries++;
        if (item.retries < item.maxRetries) {
          this._retryQueue.push(item);
          console.warn(`Chronicle: Retry ${item.retries}/${item.maxRetries} failed for ${item.method} ${item.path}`);
        } else {
          failed++;
          this._logError('error', item.method, item.path, null,
            `Permanently failed after ${item.maxRetries} retries: ${err.message}`);
        }
      }
    }

    this._retryProcessing = false;

    if (success > 0 || failed > 0) {
      console.log(`Chronicle: Retry queue processed — ${success} succeeded, ${failed} permanently failed, ${this._retryQueue.length} remaining`);
    }

    return { success, failed };
  }

  /**
   * Get the number of pending retry operations.
   * @returns {number}
   */
  getRetryQueueSize() {
    return this._retryQueue.length;
  }

  // --- Health & error log (F-QoL) ---

  /**
   * Log a REST API error for dashboard display.
   * @param {string} level - 'error' or 'warn'.
   * @param {string} method - HTTP method.
   * @param {string} path - API path.
   * @param {number|null} status - HTTP status code.
   * @param {string} message - Error message.
   * @private
   */
  _logError(level, method, path, status, message) {
    this._errorLog.unshift({
      time: Date.now(),
      timeFormatted: new Date().toLocaleTimeString(),
      level,
      method,
      path,
      status,
      message: message.substring(0, 200), // Truncate long error bodies.
    });
    if (this._errorLog.length > this._maxErrorLogEntries) {
      this._errorLog.length = this._maxErrorLogEntries;
    }
  }

  /**
   * Get the error log entries for dashboard display.
   * @returns {Array<{time: number, level: string, method: string, path: string, status: number|null, message: string}>}
   */
  getErrorLog() {
    return this._errorLog;
  }

  /**
   * Clear the error log.
   */
  clearErrorLog() {
    this._errorLog = [];
  }

  /**
   * Get connection uptime percentage for the current session.
   * @returns {number} 0-100 percentage.
   */
  getUptimePercent() {
    const sessionStart = this.health.totalConnectedMs;
    let connectedMs = sessionStart;

    if (this.health.connectedSince) {
      connectedMs += Date.now() - this.health.connectedSince;
    }

    // Approximate session duration from first success or first error.
    const firstActivity = Math.min(
      this.health.lastRestSuccess || Date.now(),
      this.health.lastRestError || Date.now(),
      this.health.connectedSince || Date.now()
    );
    const sessionDuration = Date.now() - firstActivity;

    if (sessionDuration <= 0) return 100;
    return Math.min(100, Math.round((connectedMs / sessionDuration) * 100));
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
      // Track connected time before disconnecting.
      if (this.health.connectedSince) {
        this.health.totalConnectedMs += Date.now() - this.health.connectedSince;
        this.health.connectedSince = null;
      }
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
      this._logError('error', 'WS', '/ws', null, err.message || 'WebSocket creation failed');
      this._scheduleReconnect();
      return;
    }

    this._ws.onopen = async () => {
      console.log('Chronicle: WebSocket connected');
      this._setState('connected');
      this._reconnectDelay = 1000; // Reset backoff.
      this.health.connectedSince = Date.now();

      // Flush queued messages.
      while (this._messageQueue.length > 0) {
        const msg = this._messageQueue.shift();
        this._ws.send(JSON.stringify(msg));
      }

      // Process retry queue on reconnect.
      if (this._retryQueue.length > 0) {
        const result = await this.processRetryQueue();
        if (result.success > 0 || result.failed > 0) {
          this._emit('sync.retryComplete', { payload: result });
        }
      }

      // Notify listeners.
      this._emit('sync.status', { status: 'connected' });
    };

    this._ws.onclose = (event) => {
      console.log(`Chronicle: WebSocket closed (code=${event.code})`);
      this._ws = null;

      // Track connected time.
      if (this.health.connectedSince) {
        this.health.totalConnectedMs += Date.now() - this.health.connectedSince;
        this.health.connectedSince = null;
      }

      this._setState('disconnected');

      if (this._shouldConnect && event.code !== 1000) {
        this._scheduleReconnect();
      }
    };

    this._ws.onerror = (error) => {
      console.error('Chronicle: WebSocket error', error);
      this._logError('error', 'WS', '/ws', null, 'WebSocket connection error');
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
    this.health.reconnectAttempts++;

    const delay = Math.min(this._reconnectDelay, 30000); // Cap at 30s.
    console.log(`Chronicle: Reconnecting in ${delay}ms (attempt ${this.health.reconnectAttempts})`);

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
   * Register a callback for connection state changes.
   * The callback receives the new state string as its argument.
   * @param {Function} callback - Called with (newState: string).
   */
  onStateChange(callback) {
    this._stateChangeCallbacks.add(callback);
  }

  /**
   * Remove a previously registered state change callback.
   * @param {Function} callback
   */
  offStateChange(callback) {
    this._stateChangeCallbacks.delete(callback);
  }

  /**
   * Update connection state and notify registered callbacks.
   * @param {string} state
   * @private
   */
  _setState(state) {
    const prev = this.state;
    this.state = state;
    if (prev !== state) {
      for (const cb of this._stateChangeCallbacks) {
        try {
          cb(state);
        } catch (err) {
          console.error('Chronicle: State change callback error', err);
        }
      }
    }
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
