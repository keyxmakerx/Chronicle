/**
 * Entity Map widget
 *
 * Wires the per-entity Map Editor block. Two responsibilities, both
 * Scribe+ only:
 *
 *   1. Picker mode (no map assigned yet): clicking a thumbnail card
 *      issues PUT /campaigns/:id/entities/:eid/map and reloads the
 *      block on success.
 *   2. Embed mode (map assigned): the "Change map" button replaces the
 *      iframe with the picker so the DM can switch maps without
 *      leaving the entity page.
 *
 * The block re-renders server-side after the assign, so we just
 * window.location.reload() — simpler than swapping templ fragments
 * client-side and the iframe needs to fully tear down anyway.
 *
 * Mount: data-widget="entity-map"
 * Config:
 *   data-entity-id    - Entity UUID (required)
 *   data-campaign-id  - Campaign UUID (required)
 *   data-map-id       - Currently-assigned map UUID (empty if none)
 *   data-is-scribe    - "true" if viewer can change/assign maps
 *   data-csrf         - CSRF token (set on the picker mount only)
 */
(function () {
  'use strict';

  Chronicle.register('entity-map', {
    init: function (el, config) {
      this.el = el;
      this.entityId = config.entityId || '';
      this.campaignId = config.campaignId || '';
      this.isScribe = config.isScribe === 'true';
      // No-op for non-Scribe viewers — they have no actions to bind.
      // (The empty state has no widget mount, but defensive anyway.)
      if (!this.isScribe) return;

      this._bindActions();
    },

    /**
     * Wire click handlers for both pick and change-map actions. Uses
     * delegation so re-renders (after a successful pick) don't need
     * to re-bind — but we also tear down on destroy() to avoid leaks
     * if the widget is detached from the DOM by HTMX.
     */
    _bindActions: function () {
      var self = this;
      this._clickHandler = function (e) {
        var pickBtn = e.target.closest('[data-action="entity-map-pick"]');
        if (pickBtn) {
          e.preventDefault();
          self._assign(pickBtn.getAttribute('data-map-id'));
          return;
        }
        var changeBtn = e.target.closest('[data-action="entity-map-change"]');
        if (changeBtn) {
          e.preventDefault();
          // Clearing the map_id (null) drops the entity back to the
          // picker render branch on next page load. Confirm first since
          // it momentarily looks like a destructive action — though no
          // map data is actually deleted.
          if (!window.confirm('Switch to a different map for this entity?')) return;
          self._assign(null);
          return;
        }
      };
      this.el.addEventListener('click', this._clickHandler);
    },

    /**
     * PUT the new map_id (or null to clear). On success, reload the
     * page so the block re-renders with the new render branch (embed
     * vs. picker). On failure, surface the server's error message via
     * the existing toast system rather than swallowing it.
     */
    _assign: function (mapId) {
      var url = '/campaigns/' + encodeURIComponent(this.campaignId) +
        '/entities/' + encodeURIComponent(this.entityId) + '/map';
      Chronicle.apiFetch(url, {
        method: 'PUT',
        body: { map_id: mapId },
      })
        .then(function (resp) {
          if (!resp.ok) {
            return resp.json().then(
              function (body) { throw new Error((body && body.message) || ('HTTP ' + resp.status)); },
              function () { throw new Error('HTTP ' + resp.status); }
            );
          }
          // Hard reload: the block's render branch swaps (picker
          // → iframe or vice versa) and the iframe needs a fresh
          // load anyway.
          window.location.reload();
        })
        .catch(function (err) {
          console.error('[entity-map] Assign failed:', err);
          if (Chronicle.notify) {
            Chronicle.notify(err.message || 'Failed to assign map', 'error');
          }
        });
    },

    destroy: function () {
      if (this._clickHandler) {
        this.el.removeEventListener('click', this._clickHandler);
        this._clickHandler = null;
      }
    },
  });
})();
