/**
 * editor_scene.js -- mentions of Foundry scenes in page text.
 *
 * Chronicle.FoundryScenes holds the campaign's scene list as the GM's
 * Foundry world last reported it (GET /campaigns/:id/foundry/scenes), in
 * Foundry's own folder order. Only the owner and those with DM access get
 * it; for everyone else it answers 403 and stays empty, so a player reads a
 * scene mention as its plain name and never learns whether the map exists.
 *
 * Chronicle.SceneMention is the TipTap inline node the @ picker inserts. A
 * scene picked from the list keeps its Foundry id, so two scenes with one
 * name stay apart and a rename doesn't break it; a scene typed in before it
 * exists keeps only its name and counts as present once a scene with that
 * name appears. Saved HTML is <span data-type="foundry-scene"
 * data-scene-id="…">Name</span>, which the sanitizer keeps.
 */
(function () {
  'use strict';

  window.Chronicle = window.Chronicle || {};

  // Foundry document ids are 16 letters and digits; allow a little more.
  var ID_RE = /^[A-Za-z0-9]{1,64}$/;

  function norm(s) { return String(s || '').replace(/\s+/g, ' ').trim().toLowerCase(); }

  /**
   * Whether a mention reads as present or needed, from the scene list.
   * Returns {present, scene, same} where `same` is every scene sharing the
   * mention's name (for the hover), or null when the list isn't available.
   */
  function sceneStatus(list, sceneId, name) {
    if (!list) return null;
    var byId = null;
    if (sceneId) {
      for (var i = 0; i < list.length; i++) if (list[i].id === sceneId) { byId = list[i]; break; }
    }
    var key = norm(name);
    var same = key ? list.filter(function (s) { return norm(s.name) === key; }) : [];
    var scene = byId || same[0] || null;
    return { present: !!scene, scene: scene, same: same };
  }

  /**
   * The hover text for a mention's status: where the scene sits in Foundry,
   * every folder when several scenes share the name, and the scene's current
   * name when it was renamed after being picked.
   */
  function statusTitle(st, name) {
    if (!st) return '';
    if (!st.present) return 'No scene with this name in Foundry yet';
    var where = function (s) { return s.folder ? 'in ' + s.folder : 'at the top level'; };
    if (st.same.length > 1) {
      return 'In Foundry ' + st.same.length + ' times: ' + st.same.map(where).join('; ');
    }
    var t = 'In Foundry, ' + where(st.scene);
    return norm(st.scene.name) !== norm(name) ? t + ', now called ' + st.scene.name : t;
  }

  /**
   * The @ picker's scene rows for a query: matching scenes in Foundry's
   * order, then, when no scene has exactly that name, a row to mark it as
   * needed. Empty while the list isn't available.
   */
  function pickerItems(list, query, max) {
    if (!list) return [];
    var q = norm(query);
    if (!q) return [];
    var out = [];
    for (var i = 0; i < list.length && out.length < (max || 6); i++) {
      if (norm(list[i].name).indexOf(q) !== -1) {
        out.push({ kind: 'scene', sceneId: list[i].id, name: list[i].name, folder: list[i].folder || '' });
      }
    }
    var exact = list.some(function (s) { return norm(s.name) === q; });
    var typed = String(query || '').replace(/\s+/g, ' ').trim();
    if (!exact && typed) out.push({ kind: 'scene', sceneId: null, name: typed, needed: true });
    return out;
  }

  // --- The scene list -------------------------------------------------------

  var lists = {};     // campaignId -> {state: 'loading'|'ok'|'none', scenes}
  var listeners = [];

  function notify() {
    for (var i = 0; i < listeners.length; i++) {
      try { listeners[i](); } catch (e) { /* a dead view must not stop the rest */ }
    }
  }

  Chronicle.FoundryScenes = {
    /** The scenes, or null while loading or when this viewer may not see them. */
    get: function (campaignId) {
      var l = lists[campaignId];
      return l && l.state === 'ok' ? l.scenes : null;
    },
    /** Asks the server once per campaign and page load. */
    want: function (campaignId) {
      if (!campaignId || lists[campaignId] || !Chronicle.apiFetch) return;
      lists[campaignId] = { state: 'loading', scenes: null };
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(campaignId) + '/foundry/scenes')
        .then(function (r) { return r.ok ? r.json() : null; })
        .then(function (data) {
          lists[campaignId] = data && Array.isArray(data.scenes)
            ? { state: 'ok', scenes: data.scenes } : { state: 'none', scenes: null };
          notify();
        })
        .catch(function () { lists[campaignId] = { state: 'none', scenes: null }; notify(); });
    },
    subscribe: function (fn) {
      listeners.push(fn);
      return function () {
        var i = listeners.indexOf(fn);
        if (i !== -1) listeners.splice(i, 1);
      };
    },
    status: function (campaignId, sceneId, name) {
      return sceneStatus(this.get(campaignId), sceneId, name);
    },
    pickerItems: function (campaignId, query, max) {
      return pickerItems(this.get(campaignId), query, max);
    },
    _renderSpec: function (attrs) { return renderSpec(attrs); },
    _sceneStatus: sceneStatus,
    _statusTitle: statusTitle,
    _pickerItems: pickerItems,
    _reset: function () { lists = {}; },
  };

  // --- The node -------------------------------------------------------------

  /** The saved form of a scene mention. */
  function renderSpec(attrs) {
    var a = { 'data-type': 'foundry-scene', class: 'scene-mention' };
    if (attrs.sceneId && ID_RE.test(attrs.sceneId)) a['data-scene-id'] = attrs.sceneId;
    return ['span', a, String(attrs.name || '')];
  }

  /** Draws a mention's mark into dom: just the name, or the name and its status. */
  function paint(dom, campaignId, attrs) {
    var st = Chronicle.FoundryScenes.status(campaignId, attrs.sceneId, attrs.name);
    dom.textContent = '';
    dom.className = 'scene-mention' + (st ? (st.present ? ' scene-mention--present' : ' scene-mention--needed') : '');
    if (st) {
      var ic = document.createElement('i');
      ic.className = 'fa-solid fa-map scene-mention__icon';
      ic.setAttribute('aria-hidden', 'true');
      dom.appendChild(ic);
    }
    dom.appendChild(document.createTextNode(String(attrs.name || '')));
    if (st) {
      var s = document.createElement('span');
      s.className = 'scene-mention__status';
      s.textContent = st.present ? 'in Foundry' : 'map needed';
      dom.appendChild(s);
    }
    dom.title = statusTitle(st, attrs.name);
  }

  if (window.TipTap && window.TipTap.Node) {
    Chronicle.SceneMention = window.TipTap.Node.create({
      name: 'foundryScene',
      group: 'inline',
      inline: true,
      atom: true,
      selectable: true,
      draggable: false,

      addOptions: function () {
        return { campaignId: '' };
      },

      addAttributes: function () {
        return {
          sceneId: {
            default: null,
            parseHTML: function (el) {
              var id = el.getAttribute('data-scene-id');
              return ID_RE.test(id || '') ? id : null;
            },
          },
          name: {
            default: '',
            parseHTML: function (el) { return (el.textContent || '').trim(); },
          },
        };
      },

      parseHTML: function () {
        return [{ tag: 'span[data-type="foundry-scene"]', priority: 1000 }];
      },

      renderHTML: function (props) {
        return renderSpec(props.node.attrs);
      },

      renderText: function (props) {
        return String(props.node.attrs.name || '');
      },

      addNodeView: function () {
        var options = this.options;
        return function (props) {
          var attrs = props.node.attrs;
          var dom = document.createElement('span');
          dom.setAttribute('data-type', 'foundry-scene');
          dom.setAttribute('contenteditable', 'false');
          function repaint() { paint(dom, options.campaignId, attrs); }
          Chronicle.FoundryScenes.want(options.campaignId);
          repaint();
          var off = Chronicle.FoundryScenes.subscribe(repaint);
          return {
            dom: dom,
            update: function (node) {
              if (node.type.name !== 'foundryScene') return false;
              attrs = node.attrs;
              repaint();
              return true;
            },
            destroy: off,
            ignoreMutation: function () { return true; },
          };
        };
      },
    });
  }

})();
