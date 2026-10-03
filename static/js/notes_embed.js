/**
 * notes_embed.js -- the Journal or Jot notes inside an outside app's frame
 * (the Foundry notebook and jot window).
 *
 * The frame has no Chronicle sign-in. Its parent window (only an allowed
 * origin can frame this page) hands it the player's notes grant; from then
 * on Chronicle.apiFetch sends campaign requests to the grant's routes with
 * that token, and the frame fetches and mounts the same Journal or Jot
 * notes the site shows.
 *
 * Messages, all {type: ...}:
 *   frame -> parent  chronicle:embed-ready     {mode}      ready for a token
 *   parent -> frame  chronicle:notes-token     {token, entityId?}
 *   parent -> frame  chronicle:jots-page       {entityId}  the page in view
 *   parent -> frame  chronicle:open-note       {noteId}    show this note (Journal)
 *   frame -> parent  chronicle:open-note       {noteId}    open in the notebook
 *   frame -> parent  chronicle:grant-rejected  {}          token no longer works
 * The token is accepted only from window.parent. Messages to the parent
 * carry no secrets, so they go to '*'.
 */
(function () {
  'use strict';

  var root = document.getElementById('notes-embed');
  if (!root || window.parent === window) return;

  var cid = root.dataset.campaignId;
  var mode = root.dataset.mode;
  var status = root.querySelector('[data-embed-status]');
  var body = root.querySelector('[data-embed-body]');
  var entityId = '';
  var pendingNote = '';
  var loading = 0;

  // The frame sits in Foundry's dark windows.
  document.documentElement.classList.add('dark');

  function tell(msg) {
    try { window.parent.postMessage(msg, '*'); } catch (e) { /* parent gone */ }
  }

  function say(text) {
    status.textContent = text;
    status.hidden = !text;
  }

  // A page opens on the site in a new tab (the player's own sign-in there);
  // a Journal note opens in the Journal here, or in the app's notebook.
  var journalPath = new RegExp('^/campaigns/[^/]+/journal/([^/?#]+)');
  function go(url) {
    var path = url;
    try { path = new URL(url, window.location.origin).pathname; } catch (e) { /* keep */ }
    var m = journalPath.exec(path);
    if (m) {
      var id = decodeURIComponent(m[1]);
      if (Chronicle.openJournalNote && Chronicle.openJournalNote(id)) return;
      tell({ type: 'chronicle:open-note', noteId: id });
      return;
    }
    window.open(new URL(url, window.location.origin).href, '_blank', 'noopener');
  }

  // Links in notes are site addresses; follow them through go().
  document.addEventListener('click', function (e) {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey) return;
    var a = e.target.closest && e.target.closest('a[href]');
    if (!a || a.target === '_blank') return;
    var href = a.getAttribute('href') || '';
    if (href.charAt(0) === '#') return;
    var u;
    try { u = new URL(href, window.location.origin); } catch (err) { return; }
    e.preventDefault();
    if (u.origin === window.location.origin) go(u.pathname + u.search);
    else window.open(u.href, '_blank', 'noopener');
  });

  // A note asked for before the Journal has mounted opens once it has.
  function openPending() {
    if (pendingNote && Chronicle.openJournalNote && Chronicle.openJournalNote(pendingNote)) pendingNote = '';
  }

  function load() {
    var seq = ++loading;
    var q = '?mode=' + encodeURIComponent(mode);
    if (mode === 'jots' && entityId) q += '&entity=' + encodeURIComponent(entityId);
    Chronicle.apiFetch('/campaigns/' + encodeURIComponent(cid) + '/notes/embed' + q, {
      headers: { Accept: 'text/html' }
    }).then(function (res) {
      if (res.status === 401 || res.status === 403) {
        return res.text().then(function (t) {
          if (seq !== loading) return;
          var msg = '';
          try { msg = JSON.parse(t).message || ''; } catch (e) { /* not JSON */ }
          if (res.status === 401) tell({ type: 'chronicle:grant-rejected' });
          say(msg || 'Chronicle turned this window away. Reconnect from the notebook button.');
        });
      }
      if (!res.ok) throw new Error('embed ' + res.status);
      return res.text().then(function (html) {
        if (seq !== loading) return;
        Array.prototype.forEach.call(body.querySelectorAll('[data-widget]'), function (w) {
          if (Chronicle.destroyWidget) Chronicle.destroyWidget(w);
        });
        body.innerHTML = html;
        say('');
        Chronicle.mountWidgets(body);
        openPending();
      });
    }).catch(function () {
      if (seq === loading) say('Chronicle couldn\'t be reached. It will try again when you reopen this window.');
    });
  }

  window.addEventListener('message', function (e) {
    if (e.source !== window.parent || !e.data || typeof e.data !== 'object') return;
    var d = e.data;
    if (d.type === 'chronicle:notes-token' && typeof d.token === 'string' && d.token) {
      Chronicle.embed = { token: d.token, go: go };
      entityId = typeof d.entityId === 'string' ? d.entityId : '';
      load();
    } else if (d.type === 'chronicle:jots-page' && mode === 'jots' && Chronicle.embed) {
      var next = typeof d.entityId === 'string' ? d.entityId : '';
      if (next !== entityId) {
        entityId = next;
        load();
      }
    } else if (d.type === 'chronicle:open-note' && mode === 'journal' && typeof d.noteId === 'string' && d.noteId) {
      pendingNote = d.noteId;
      openPending();
    }
  });

  tell({ type: 'chronicle:embed-ready', mode: mode });
})();
