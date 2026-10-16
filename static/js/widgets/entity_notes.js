/**
 * entity_notes.js -- Chronicle Player Notes Widget
 *
 * Notes people leave on a page, each saying who can read it: only the
 * author (private), the DMs (dm_only), the DMs and scribes (dm_scribe, kept
 * for older notes), everyone, or picked people (custom + sharedWith). The
 * server decides every read and write; the options here only mirror it.
 *
 * Live: /ws entity_note.* nudges carry ids only and trigger a refetch, with
 * a 30 s poll behind them. Rows are matched by id and changed one at a time,
 * so the composer, a note folded open and the reader's scroll survive any
 * update. Auto-mounted by boot.js on data-widget="entity-notes".
 */
(function () {
  'use strict';

  // A delete waits this long for Undo before it reaches the server.
  var UNDO_MS = 5000;
  // Our own writes come back over the socket; inside this window a change
  // to a note we just wrote is ours, so it settles in instead of glowing.
  var OWN_ECHO_MS = 4000;

  var AUDIENCES = {
    'private':   { label: 'Only me',         icon: 'fa-lock',        tone: '' },
    'dm_only':   { label: 'DM only',         icon: 'fa-eye-slash',   tone: 'dm' },
    'dm_scribe': { label: 'DMs and scribes', icon: 'fa-user-shield', tone: 'dm' },
    'everyone':  { label: 'Everyone',        icon: 'fa-users',       tone: 'all' },
    'custom':    { label: 'Picked people',   icon: 'fa-user-plus',   tone: '' }
  };
  var AVATAR_COLOURS = ['#6366f1', '#0d9488', '#c2410c', '#7c3aed', '#be185d', '#0369a1', '#4d7c0f', '#b45309'];

  // Full, calm or off, from the same switches the stylesheet obeys. Only the
  // moves made in script (fold, leave) ask; CSS animations are reduced by
  // the global Calm and Off rules.
  function motionMode() {
    var r = document.documentElement;
    if (r.getAttribute('data-motion') === 'off') return 'off';
    if (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return 'off';
    if (r.getAttribute('data-motion') === 'calm' || r.getAttribute('data-view-motion') === 'calm' || r.hasAttribute('data-cz-reduce')) return 'calm';
    return 'full';
  }

  function escHtml(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function esc(s) { return Chronicle.escapeHtml ? Chronicle.escapeHtml(s == null ? '' : String(s)) : escHtml(s); }

  function initials(name) {
    var parts = String(name || '?').trim().split(/\s+/);
    var out = parts[0].charAt(0) + (parts.length > 1 ? parts[parts.length - 1].charAt(0) : '');
    return out.toUpperCase() || '?';
  }
  function colourFor(id) {
    var h = 0;
    for (var i = 0; i < String(id).length; i++) h = (h * 31 + String(id).charCodeAt(i)) | 0;
    return AVATAR_COLOURS[Math.abs(h) % AVATAR_COLOURS.length];
  }
  function ago(iso) {
    var t = Date.parse(iso);
    if (isNaN(t)) return '';
    var m = Math.round((Date.now() - t) / 60000);
    if (m < 1) return 'just now';
    if (m < 60) return m + ' min ago';
    var h = Math.round(m / 60);
    if (h < 24) return h + ' h ago';
    var d = Math.round(h / 24);
    if (d < 14) return d + ' d ago';
    return new Date(t).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
  }
  function fullDate(iso) {
    var t = Date.parse(iso);
    return isNaN(t) ? '' : new Date(t).toLocaleString();
  }

  // Older notes may carry only ProseMirror JSON; show their words.
  function extractText(jsonBody) {
    try {
      var parsed = typeof jsonBody === 'string' ? JSON.parse(jsonBody) : jsonBody;
      var out = [];
      (function walk(node) {
        if (!node) return;
        if (typeof node.text === 'string') out.push(node.text);
        if (Array.isArray(node.content)) node.content.forEach(walk);
      })(parsed);
      return out.join(' ');
    } catch (e) {
      return '';
    }
  }

  // The plain-text fallback editor (no TipTap bundle) saves paragraphs.
  function textToHTML(text) {
    if (!text) return '';
    return text.split(/\n{2,}/).map(function (p) {
      return '<p>' + escHtml(p).replace(/\n/g, '<br>') + '</p>';
    }).join('');
  }
  function htmlToText(html) {
    var div = document.createElement('div');
    div.innerHTML = html || '';
    var blocks = div.querySelectorAll('p, li, h1, h2, h3, h4, blockquote, pre');
    if (!blocks.length) return (div.textContent || '').trim();
    var out = [];
    for (var i = 0; i < blocks.length; i++) {
      if (blocks[i].querySelector('p, li')) continue;
      out.push((blocks[i].nodeName === 'LI' ? '- ' : '') + (blocks[i].textContent || '').trim());
    }
    return out.join('\n\n');
  }

  function humanError(err) {
    if (err && err.message) return err.message;
    return 'Something went wrong';
  }

  function asJSON(resp) {
    if (!resp.ok) {
      return resp.json().then(
        function (body) { return Promise.reject(new Error((body && (body.message || body.error)) || ('HTTP ' + resp.status))); },
        function () { return Promise.reject(new Error('HTTP ' + resp.status)); }
      );
    }
    return resp.json();
  }

  Chronicle.register('entity-notes', {
    init: function (el, config) {
      var endpoint = config.endpoint || '';
      var campaignId = config.campaignId || '';
      var me = config.currentUserId || '';
      var seesDmNotes = config.seesDmNotes === true;

      var notes = [];       // the server's list, as last fetched
      var loaded = false;
      var filter = 'all';
      var pendingDeletes = {}; // id -> { timer, committing }
      var openIds = {};     // id -> folded open
      var sigs = {};        // id -> what the row last showed
      var touched = {};     // id -> when this window last wrote it
      var lastOwnWrite = 0;
      var members = null;   // user_id -> display name
      var comp = null;      // the open composer, see openComposer
      var lastCount = -1;
      var ws = null, wsDelay = 2000, wsTimer = null, pollTimer = null, agoTimer = null;
      var toastTimer = null;
      var destroyed = false;

      // --- Shell ---

      el.innerHTML =
        '<section class="card cb-card !p-0 en-block" aria-label="Notes on this page">' +
          '<header class="en-head">' +
            '<h3 class="en-title cb-head"><i class="fa-solid fa-note-sticky" aria-hidden="true"></i>Notes <span class="en-count cb-count" data-en-count>0</span></h3>' +
            '<span class="en-spacer"></span>' +
            '<button type="button" class="btn btn-primary btn-sm" data-en-act="new"><i class="fa-solid fa-plus mr-1.5" aria-hidden="true"></i>New note</button>' +
          '</header>' +
          '<div class="en-filters" role="group" aria-label="Show">' +
            chip('all', 'All', true) + chip('mine', 'Mine', false) +
            (seesDmNotes ? chip('dm', 'DM only', false) : chip('shared', 'Shared with me', false)) +
          '</div>' +
          '<div data-en-comp></div>' +
          '<div class="en-list" data-en-list><div class="en-empty">Loading notes…</div></div>' +
        '</section>';
      var block = el.querySelector('.en-block');
      var listEl = el.querySelector('[data-en-list]');
      var compSlot = el.querySelector('[data-en-comp]');
      var countEl = el.querySelector('[data-en-count]');

      function chip(value, label, on) {
        return '<button type="button" class="en-chip" data-en-filter="' + value + '" aria-pressed="' + on + '">' + label + '</button>';
      }

      // --- Data ---

      function find(id) {
        for (var i = 0; i < notes.length; i++) if (notes[i].id === id) return notes[i];
        return null;
      }
      function upsert(note) {
        for (var i = 0; i < notes.length; i++) {
          if (notes[i].id === note.id) { notes[i] = note; return; }
        }
        notes.unshift(note);
      }
      function markOwn(id) {
        lastOwnWrite = Date.now();
        if (id) touched[id] = lastOwnWrite;
      }
      function isOwnEcho(n) {
        var now = Date.now();
        if (touched[n.id] && now - touched[n.id] < OWN_ECHO_MS) return true;
        return n.authorUserId === me && now - lastOwnWrite < OWN_ECHO_MS;
      }

      function loadNotes() {
        return Chronicle.apiFetch(endpoint)
          .then(asJSON)
          .then(function (list) {
            if (destroyed) return;
            notes = Array.isArray(list) ? list : [];
            loaded = true;
            refresh(true);
            checkComposerAgainstServer();
          })
          .catch(function (err) {
            console.error('[EntityNotes] Load error:', err);
            if (!loaded) {
              loaded = true;
              listEl.innerHTML = '<div class="en-empty"><b>Notes could not load</b>They will try again shortly.</div>';
            }
          });
      }

      // Names for each note's author. Anyone who has left the campaign
      // reads as a former member rather than an id.
      function loadMembers() {
        return Chronicle.apiFetch('/campaigns/' + campaignId + '/members', { headers: { 'Accept': 'application/json' } })
          .then(asJSON)
          .then(function (list) {
            members = {};
            (Array.isArray(list) ? list : []).forEach(function (m) {
              if (m && m.user_id) members[m.user_id] = m.display_name || m.email || 'A member';
            });
            if (loaded) refresh(false);
            if (comp) renderPicker();
          })
          .catch(function (err) {
            console.error('[EntityNotes] Members load error:', err);
            members = {};
          });
      }
      function authorName(id) {
        if (id === me) return 'You';
        if (!members) return '…';
        return members[id] || 'Former member';
      }

      // --- The list ---

      function visible() {
        var out = notes.filter(function (n) {
          if (pendingDeletes[n.id]) return false;
          if (filter === 'mine') return n.authorUserId === me;
          if (filter === 'dm') return n.audience === 'dm_only' || n.audience === 'dm_scribe';
          if (filter === 'shared') return n.authorUserId !== me;
          return true;
        });
        // Pinned first, then newest written. Editing never moves a note.
        out.sort(function (a, b) {
          return ((b.pinned ? 1 : 0) - (a.pinned ? 1 : 0)) || (Date.parse(b.createdAt) - Date.parse(a.createdAt));
        });
        return out;
      }

      function audienceLabel(n) {
        if (n.audience === 'dm_only' && seesDmNotes) return 'DMs only';
        if (n.audience === 'custom') {
          var c = Array.isArray(n.sharedWith) ? n.sharedWith.length : 0;
          return c ? 'Picked people (' + c + ')' : 'Picked people';
        }
        return (AUDIENCES[n.audience] || AUDIENCES['private']).label;
      }
      function audienceTitle(n) {
        if (n.audience === 'custom' && members && Array.isArray(n.sharedWith)) {
          return 'Shared with ' + n.sharedWith.map(function (id) { return members[id] || 'a former member'; }).join(', ');
        }
        return audienceHint(n.audience, n.authorUserId === me);
      }
      function wasEdited(n) {
        var c = Date.parse(n.createdAt), u = Date.parse(n.updatedAt);
        return !isNaN(c) && !isNaN(u) && u - c > 1000;
      }

      function noteHTML(n) {
        var mine = n.authorUserId === me;
        var who = authorName(n.authorUserId);
        var a = AUDIENCES[n.audience] || AUDIENCES['private'];
        var isOpen = !!openIds[n.id];
        var body = n.bodyHtml || (n.body ? '<p>' + esc(extractText(n.body)) + '</p>' : '');
        var h = '';
        h += '<span class="en-ava" style="background:' + colourFor(n.authorUserId) + '" aria-hidden="true">' + esc(initials(mine ? (members && members[me]) || 'You' : who)) + '</span>';
        h += '<div class="en-meta">';
        h += '<span class="en-who">' + esc(who) + '</span>';
        h += '<span class="en-aud' + (a.tone ? ' en-aud--' + a.tone : '') + (a.tone === 'dm' ? ' cb-dm' : '') + '" title="' + esc(audienceTitle(n)) + '"><i class="fa-solid ' + a.icon + '" aria-hidden="true"></i>' + esc(audienceLabel(n)) + '</span>';
        h += '<time datetime="' + esc(n.createdAt) + '" title="' + esc(fullDate(n.createdAt)) + '" data-en-ago>' + esc(ago(n.createdAt)) + '</time>';
        if (wasEdited(n)) h += '<span class="en-edited" title="Edited ' + esc(fullDate(n.updatedAt)) + '">edited</span>';
        if (n.pinned) h += '<span class="en-pin" title="Pinned"><i class="fa-solid fa-thumbtack" aria-hidden="true"></i><span class="sr-only">Pinned</span></span>';
        h += '</div>';
        h += '<div class="en-body' + (isOpen ? '' : ' is-clamped') + '" data-en-act="fold">';
        if (n.title) h += '<strong class="en-note-title">' + esc(n.title) + '.</strong> ';
        h += '<div class="en-prose">' + (body || '<p class="en-blank">(no text)</p>') + '</div>';
        h += '</div>';
        h += '<button type="button" class="en-more" data-en-act="fold" aria-expanded="' + isOpen + '" hidden>' + (isOpen ? 'Show less' : 'Show more') + '</button>';
        if (mine) {
          h += '<div class="en-tools">';
          h += '<button type="button" class="en-icon" data-en-act="edit" aria-label="Edit" title="Edit"><i class="fa-solid fa-pen" aria-hidden="true"></i></button>';
          h += '<button type="button" class="en-icon" data-en-act="pin" aria-label="' + (n.pinned ? 'Unpin' : 'Pin') + '" title="' + (n.pinned ? 'Unpin' : 'Pin') + '" aria-pressed="' + !!n.pinned + '"><i class="fa-solid fa-thumbtack" aria-hidden="true"></i></button>';
          h += '<button type="button" class="en-icon en-icon--danger" data-en-act="delete" aria-label="Delete" title="Delete"><i class="fa-solid fa-trash" aria-hidden="true"></i></button>';
          h += '</div>';
        }
        return h;
      }

      function sigOf(n) {
        return JSON.stringify([n.title, n.bodyHtml, n.body ? 1 : 0, n.audience, n.sharedWith, n.pinned, n.updatedAt, !!openIds[n.id], authorName(n.authorUserId)]);
      }

      // The fold control shows only when the note is longer than its
      // two-line preview.
      function markFoldable(row) {
        var body = row.querySelector('.en-body');
        var more = row.querySelector('.en-more');
        if (!body || !more) return;
        var id = row.getAttribute('data-en-id');
        var long = openIds[id] || body.scrollHeight > body.clientHeight + 2;
        more.hidden = !long;
        body.classList.toggle('is-foldable', !!long);
        body.classList.toggle('is-overflowing', !openIds[id] && long);
      }

      // Brings the list in line with the server one row at a time: new
      // rows settle in (or glow when someone else wrote them), changed rows
      // flash where the text changed, gone rows fold away.
      function refresh(fromServer) {
        if (!loaded) return;
        var want = visible();
        var seen = {};
        want.forEach(function (n, i) {
          seen[n.id] = true;
          var sig = sigOf(n);
          var row = listEl.querySelector('.en-note[data-en-id="' + cssEscape(n.id) + '"]:not(.is-leaving)');
          if (!row) {
            row = document.createElement('article');
            row.className = 'en-note';
            row.setAttribute('data-en-id', n.id);
            row.innerHTML = noteHTML(n);
            if (lastCount >= 0) {
              row.classList.add(fromServer && !isOwnEcho(n) ? 'en-glow' : 'en-arrive');
              row.addEventListener('animationend', function () { row.classList.remove('en-glow', 'en-arrive'); });
            }
          } else if (sigs[n.id] !== sig) {
            var before = sigs[n.id] ? JSON.parse(sigs[n.id]) : [];
            row.innerHTML = noteHTML(n);
            var textChanged = before[0] !== n.title || before[1] !== n.bodyHtml;
            if (fromServer && textChanged && !isOwnEcho(n)) {
              row.classList.remove('en-changed');
              void row.offsetWidth;
              row.classList.add('en-changed');
            }
          } else {
            sigs[n.id] = sig;
            place(row, i);
            return;
          }
          sigs[n.id] = sig;
          place(row, i);
          markFoldable(row);
        });
        Array.prototype.slice.call(listEl.querySelectorAll('.en-note:not(.is-leaving)')).forEach(function (row) {
          var id = row.getAttribute('data-en-id');
          if (!seen[id]) { delete sigs[id]; removeRow(row); }
        });

        var empty = listEl.querySelector('.en-empty');
        if (!want.length) {
          var msg = filter === 'all'
            ? '<b>No notes on this page yet</b>Notes you write here show up for you, or for whoever you share them with.'
            : '<b>Nothing here</b>No notes match this filter.';
          if (!empty) {
            listEl.insertAdjacentHTML('beforeend', '<div class="en-empty">' + msg + '</div>');
          } else if (empty.innerHTML !== msg) {
            empty.innerHTML = msg;
          }
        } else if (empty) {
          empty.parentNode.removeChild(empty);
        }

        if (lastCount !== want.length) {
          countEl.textContent = want.length;
          if (lastCount >= 0) {
            countEl.classList.remove('en-bump');
            void countEl.offsetWidth;
            countEl.classList.add('en-bump');
          }
          lastCount = want.length;
        }
      }

      // Puts a row at position i among the rows that are staying; rows on
      // their way out don't count, so nothing is moved (and re-animated)
      // while one folds away.
      function place(row, i) {
        var at = listEl.querySelectorAll('.en-note:not(.is-leaving)')[i] || null;
        if (at !== row) listEl.insertBefore(row, at);
      }

      function cssEscape(s) {
        return window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/["\\]/g, '\\$&');
      }

      function removeRow(row) {
        var mode = motionMode();
        if (mode === 'off') { if (row.parentNode) row.parentNode.removeChild(row); return; }
        row.classList.add('is-leaving');
        row.style.height = row.offsetHeight + 'px';
        void row.offsetWidth;
        row.style.opacity = '0';
        if (mode === 'full') {
          row.style.height = '0px';
          row.style.paddingTop = '0px';
          row.style.paddingBottom = '0px';
          row.style.marginTop = '0px';
        }
        setTimeout(function () { if (row.parentNode) row.parentNode.removeChild(row); }, 180);
      }

      // Fold: the body grows to its real height, then lets go of it.
      function fold(id) {
        var row = listEl.querySelector('.en-note[data-en-id="' + cssEscape(id) + '"]');
        if (!row) return;
        var body = row.querySelector('.en-body');
        var more = row.querySelector('.en-more');
        var from = body.offsetHeight;
        openIds[id] = !openIds[id];
        body.classList.toggle('is-clamped', !openIds[id]);
        body.classList.toggle('is-overflowing', !openIds[id]);
        if (more) {
          more.textContent = openIds[id] ? 'Show less' : 'Show more';
          more.setAttribute('aria-expanded', String(!!openIds[id]));
        }
        var n = find(id);
        if (n) sigs[id] = sigOf(n);
        if (motionMode() !== 'full' || !body.animate) return;
        var to = body.offsetHeight;
        body.animate([{ height: from + 'px' }, { height: to + 'px' }], { duration: 200, easing: 'cubic-bezier(0.16, 1, 0.3, 1)' });
      }

      function tickAgo() {
        Array.prototype.forEach.call(listEl.querySelectorAll('time[data-en-ago]'), function (t) {
          t.textContent = ago(t.getAttribute('datetime'));
        });
      }

      // --- Toast (Undo, Saved) ---

      // One toast at a time. A delete whose Undo is pushed off by a newer
      // toast is carried out then, so a note never goes later without an
      // Undo on screen.
      var onToastGone = null;
      function toast(message, undo, onGone) {
        var old = block.querySelector('.en-toast');
        if (old) old.parentNode.removeChild(old);
        if (toastTimer) clearTimeout(toastTimer);
        if (onToastGone) { var gone = onToastGone; onToastGone = null; gone(); }
        onToastGone = onGone || null;
        var t = document.createElement('div');
        t.className = 'en-toast';
        t.setAttribute('role', 'status');
        t.innerHTML = (undo ? '' : '<i class="fa-solid fa-check" aria-hidden="true"></i>') + '<span>' + esc(message) + '</span>' +
          (undo ? '<button type="button">Undo</button>' : '');
        block.appendChild(t);
        if (undo) {
          t.querySelector('button').addEventListener('click', function () {
            onToastGone = null;
            undo();
            if (t.parentNode) t.parentNode.removeChild(t);
          });
        }
        toastTimer = setTimeout(function () {
          if (t.parentNode) t.parentNode.removeChild(t);
          if (onToastGone === onGone) onToastGone = null;
        }, undo ? UNDO_MS : 1800);
      }

      // --- Writes ---

      function togglePin(id) {
        var n = find(id);
        if (!n) return;
        var was = !!n.pinned;
        n.pinned = !was;
        markOwn(id);
        refresh(false);
        Chronicle.apiFetch(endpoint + '/' + id, { method: 'PUT', body: { pinned: !was } })
          .then(asJSON)
          .then(function (saved) { markOwn(id); upsert(saved); refresh(false); })
          .catch(function (err) {
            n.pinned = was;
            refresh(false);
            Chronicle.notify && Chronicle.notify('Could not ' + (was ? 'unpin' : 'pin') + ' the note: ' + humanError(err), 'error');
          });
      }

      // Delete is undone, not confirmed: the row goes at once and the note
      // is deleted for everyone when the Undo runs out (or the page closes).
      function del(id) {
        if (!find(id)) return;
        if (comp && comp.id === id) closeComposer();
        var entry = { committing: false, timer: null };
        entry.timer = setTimeout(function () { commitDelete(id, false); }, UNDO_MS);
        pendingDeletes[id] = entry;
        refresh(false);
        toast('Note deleted', function () {
          if (entry.committing) return;
          clearTimeout(entry.timer);
          delete pendingDeletes[id];
          refresh(false);
        }, function () { commitDelete(id, false); });
      }

      function commitDelete(id, keepalive) {
        var entry = pendingDeletes[id];
        if (!entry || entry.committing) return;
        clearTimeout(entry.timer);
        entry.committing = true;
        markOwn(id);
        Chronicle.apiFetch(endpoint + '/' + id, { method: 'DELETE', keepalive: !!keepalive })
          .then(function (resp) {
            if (!resp.ok && resp.status !== 404) return asJSON(resp);
            notes = notes.filter(function (n) { return n.id !== id; });
            delete pendingDeletes[id];
            if (!destroyed) refresh(false);
          })
          .catch(function (err) {
            delete pendingDeletes[id];
            if (destroyed) return;
            refresh(false);
            Chronicle.notify && Chronicle.notify('The note was not deleted: ' + humanError(err), 'error');
          });
      }

      function flushDeletes() {
        Object.keys(pendingDeletes).forEach(function (id) { commitDelete(id, true); });
      }

      // --- Composer ---
      //
      // One composer at a time, for a new note or for editing one of the
      // viewer's own. comp = { id, audience, sharedWith, base, editor,
      // textarea, saving, gone }; base is the note as it was when editing
      // began, to notice a change made in another window.

      function audienceOptions(current) {
        var out = ['private', 'dm_only', 'everyone', 'custom'];
        if (current === 'dm_scribe') out.splice(2, 0, 'dm_scribe');
        return out;
      }
      function audienceHint(aud, mine) {
        switch (aud) {
          case 'private': return mine === false ? 'Only its writer.' : 'Nobody else, the DM included.';
          case 'dm_only': return seesDmNotes ? 'You and any co-DMs. Players never see it.' : 'You and the campaign’s DMs.';
          case 'dm_scribe': return 'The DMs and every scribe.';
          case 'everyone': return 'Everyone in the campaign.';
          case 'custom': return 'You and the people you pick.';
        }
        return '';
      }

      function composerDirty() {
        if (!comp) return false;
        var title = comp.form.querySelector('[data-en-f="title"]').value.trim();
        var body = bodyHTML();
        if (!comp.id) return !!(title || body);
        return title !== (comp.base.title || '') || body !== (comp.base.bodyHtml || '');
      }

      function openComposer(id) {
        if (comp) {
          if (comp.id === id) { focusComposer(); return; }
          if (composerDirty()) {
            focusComposer();
            toast('Save or cancel the note you are writing first', null);
            return;
          }
          closeComposer(true);
        }
        var n = id ? find(id) : null;
        if (id && !n) return;
        comp = {
          id: id,
          audience: n ? n.audience : 'private',
          sharedWith: n && Array.isArray(n.sharedWith) ? n.sharedWith.slice() : [],
          base: n ? { title: n.title || '', bodyHtml: n.bodyHtml || '', audience: n.audience, updatedAt: n.updatedAt } : null,
          editor: null, textarea: null, saving: false, gone: false, form: null
        };
        var h = '<form class="en-comp en-grow" novalidate aria-label="' + (n ? 'Edit note' : 'New note') + '">';
        h += '<input type="text" class="input en-field" data-en-f="title" maxlength="200" placeholder="Title (optional)" aria-label="Title" value="' + esc(n ? n.title : '') + '">';
        h += '<div class="en-editor-wrap">';
        h += '<div class="en-fmt" role="toolbar" aria-label="Formatting" hidden>' +
          fmtBtn('bold', 'fa-bold', 'Bold') + fmtBtn('italic', 'fa-italic', 'Italic') + fmtBtn('bulletList', 'fa-list-ul', 'List') + '</div>';
        h += '<div class="en-editor" data-en-f="body"></div>';
        h += '</div>';
        h += '<div class="en-reads"><span class="en-reads-label">Who can read it</span>';
        h += '<div class="en-seg" role="radiogroup" aria-label="Who can read it">';
        audienceOptions(comp.audience).forEach(function (a) {
          var lab = a === 'dm_only' && seesDmNotes ? 'DMs only' : AUDIENCES[a].label;
          h += '<button type="button" role="radio" data-en-aud="' + a + '" aria-checked="false"><i class="fa-solid ' + AUDIENCES[a].icon + '" aria-hidden="true"></i>' + esc(lab) + '</button>';
        });
        h += '</div></div>';
        h += '<div class="en-pick" data-en-pick hidden></div>';
        h += '<div class="en-callout" data-en-warn hidden></div>';
        h += '<div class="en-foot"><span class="en-hint" data-en-hint aria-live="polite"></span><span class="en-spacer"></span>';
        h += '<button type="button" class="btn btn-secondary btn-sm" data-en-act="cancel">Cancel</button>';
        h += '<button type="submit" class="btn btn-primary btn-sm" data-en-act="save">' + (n ? 'Save changes' : 'Save note') + '</button>';
        h += '</div></form>';
        compSlot.innerHTML = h;
        comp.form = compSlot.querySelector('form');
        mountEditor(n ? (n.bodyHtml || (n.body ? '<p>' + esc(extractText(n.body)) + '</p>' : '')) : '');
        setAudience(comp.audience);
        focusComposer(n ? 'body' : 'title');
      }

      function fmtBtn(cmd, icon, label) {
        return '<button type="button" class="en-icon" data-en-fmt="' + cmd + '" aria-label="' + label + '" title="' + label + '" aria-pressed="false"><i class="fa-solid ' + icon + '" aria-hidden="true"></i></button>';
      }

      // TipTap when the bundle is on the page, the same editor the Journal
      // uses; a plain text box otherwise.
      function mountEditor(html) {
        var mount = comp.form.querySelector('[data-en-f="body"]');
        if (window.TipTap && TipTap.Editor && TipTap.StarterKit) {
          var ext = [TipTap.StarterKit.configure({ link: false, underline: false })];
          if (TipTap.Underline) ext.push(TipTap.Underline);
          if (TipTap.Placeholder) ext.push(TipTap.Placeholder.configure({ placeholder: 'Write your note…' }));
          var link = Chronicle.MentionLink || TipTap.Link;
          if (link) ext.push(link.configure({ openOnClick: false, autolink: true }));
          comp.editor = new TipTap.Editor({
            element: mount,
            extensions: ext,
            content: html || '<p></p>',
            editorProps: { attributes: { class: 'en-prose en-editor-area', role: 'textbox', 'aria-multiline': 'true', 'aria-label': 'Note' } },
            onTransaction: syncFmt
          });
          comp.form.querySelector('.en-fmt').hidden = false;
        } else {
          mount.innerHTML = '<textarea class="input en-field en-textarea" rows="4" aria-label="Note" placeholder="Write your note…"></textarea>';
          comp.textarea = mount.querySelector('textarea');
          comp.textarea.value = htmlToText(html);
        }
      }
      function syncFmt() {
        if (!comp || !comp.editor) return;
        var e = comp.editor;
        Array.prototype.forEach.call(comp.form.querySelectorAll('[data-en-fmt]'), function (b) {
          b.setAttribute('aria-pressed', String(e.isActive(b.getAttribute('data-en-fmt'))));
        });
      }
      function bodyHTML() {
        if (!comp) return '';
        if (comp.editor) return comp.editor.isEmpty ? '' : comp.editor.getHTML();
        return textToHTML((comp.textarea && comp.textarea.value || '').trim());
      }
      function focusComposer(which) {
        if (!comp) return;
        if (which !== 'body') {
          var t = comp.form.querySelector('[data-en-f="title"]');
          if (which === 'title' && t) { t.focus({ preventScroll: true }); return; }
        }
        if (comp.editor) comp.editor.commands.focus('end');
        else if (comp.textarea) comp.textarea.focus({ preventScroll: true });
      }

      function setAudience(a) {
        if (!comp) return;
        comp.audience = a;
        Array.prototype.forEach.call(comp.form.querySelectorAll('[data-en-aud]'), function (b) {
          var on = b.getAttribute('data-en-aud') === a;
          b.setAttribute('aria-checked', String(on));
          b.tabIndex = on ? 0 : -1;
        });
        renderPicker();
        setHint();
      }
      function setHint(text) {
        if (!comp) return;
        var h = comp.form.querySelector('[data-en-hint]');
        if (text) { h.textContent = text; h.classList.add('is-warn'); return; }
        h.classList.remove('is-warn');
        if (comp.audience === 'custom' && comp.sharedWith.length && members) {
          h.textContent = 'You and ' + comp.sharedWith.map(function (id) { return members[id] || 'a former member'; }).join(', ') + '.';
        } else {
          h.textContent = audienceHint(comp.audience, true);
        }
      }

      // Picked people: one toggle per member, the viewer left out because
      // a writer always reads their own note.
      function renderPicker() {
        if (!comp) return;
        var box = comp.form.querySelector('[data-en-pick]');
        if (comp.audience !== 'custom') { box.hidden = true; box.innerHTML = ''; return; }
        box.hidden = false;
        if (!members) { box.innerHTML = '<span class="en-hint">Loading people…</span>'; return; }
        var ids = Object.keys(members).filter(function (id) { return id !== me; });
        comp.sharedWith.forEach(function (id) { if (ids.indexOf(id) < 0 && id !== me) ids.push(id); });
        if (!ids.length) { box.innerHTML = '<span class="en-hint">Nobody else is in this campaign yet.</span>'; return; }
        ids.sort(function (a, b) { return String(members[a] || '').localeCompare(String(members[b] || '')); });
        box.innerHTML = '<div class="en-pick-row" role="group" aria-label="People who can read it">' + ids.map(function (id) {
          var on = comp.sharedWith.indexOf(id) >= 0;
          return '<button type="button" class="en-chip" data-en-person="' + esc(id) + '" aria-pressed="' + on + '">' + esc(members[id] || 'Former member') + '</button>';
        }).join('') + '</div>';
      }

      // Editing a note that changed in another window (the same person on
      // two screens) or was deleted there: say so before a save replaces it.
      function checkComposerAgainstServer() {
        if (!comp || !comp.id || comp.saving) return;
        var n = find(comp.id);
        var warn = comp.form.querySelector('[data-en-warn]');
        if (!n && !pendingDeletes[comp.id]) {
          comp.gone = true;
          warn.hidden = false;
          warn.innerHTML = '<i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i><span>This note was deleted in another window. Saving here keeps it as a new note.</span>';
          return;
        }
        if (!n) return;
        comp.gone = false;
        var changed = (n.title || '') !== comp.base.title || (n.bodyHtml || '') !== comp.base.bodyHtml || n.audience !== comp.base.audience;
        if (!changed || isOwnEcho(n)) { warn.hidden = true; warn.innerHTML = ''; return; }
        warn.hidden = false;
        warn.innerHTML = '<i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i><span>This note was changed in another window. Saving here replaces that version.</span>' +
          '<button type="button" class="en-link" data-en-act="take-newer">Use the newer version</button>';
      }
      function takeNewer() {
        var n = comp && find(comp.id);
        if (!n) return;
        comp.base = { title: n.title || '', bodyHtml: n.bodyHtml || '', audience: n.audience, updatedAt: n.updatedAt };
        comp.form.querySelector('[data-en-f="title"]').value = n.title || '';
        if (comp.editor) comp.editor.commands.setContent(n.bodyHtml || '<p></p>');
        else if (comp.textarea) comp.textarea.value = htmlToText(n.bodyHtml);
        comp.sharedWith = Array.isArray(n.sharedWith) ? n.sharedWith.slice() : [];
        setAudience(n.audience);
        checkComposerAgainstServer();
      }

      function closeComposer(instant) {
        if (!comp) return;
        var form = comp.form;
        var editor = comp.editor;
        comp = null;
        var finish = function () {
          if (editor) { try { editor.destroy(); } catch (e) {} }
          if (form.parentNode) form.parentNode.removeChild(form);
        };
        if (instant || motionMode() === 'off' || !form.animate) { finish(); return; }
        var calm = motionMode() === 'calm';
        form.animate([{ opacity: 1, transform: 'none' }, { opacity: 0, transform: calm ? 'none' : 'scale(0.97)' }],
          { duration: 120, easing: 'cubic-bezier(0.4, 0, 1, 1)' }).onfinish = finish;
      }

      function save() {
        if (!comp || comp.saving) return;
        var c = comp;
        var title = c.form.querySelector('[data-en-f="title"]').value.trim();
        var html = bodyHTML();
        if (!title && !html) { setHint('Write something first.'); focusComposer('body'); return; }
        if (c.audience === 'custom' && !c.sharedWith.length) { setHint('Pick at least one person, or choose who else can read it.'); return; }
        var payload = { audience: c.audience, title: title, bodyHtml: html, sharedWith: c.audience === 'custom' ? c.sharedWith.slice() : [] };
        var isNew = !c.id || c.gone;
        if (isNew && c.audience !== 'custom') delete payload.sharedWith;
        var btn = c.form.querySelector('[data-en-act="save"]');
        c.saving = true;
        btn.disabled = true;
        btn.textContent = 'Saving…';
        markOwn(c.id);
        var req = isNew
          ? Chronicle.apiFetch(endpoint, { method: 'POST', body: payload })
          : Chronicle.apiFetch(endpoint + '/' + c.id, { method: 'PUT', body: payload });
        req.then(asJSON).then(function (note) {
          markOwn(note.id);
          upsert(note);
          if (comp === c) closeComposer();
          refresh(false);
          toast(isNew ? 'Note saved' : 'Saved', null);
        }).catch(function (err) {
          c.saving = false;
          if (comp !== c) return;
          btn.disabled = false;
          btn.textContent = isNew ? 'Save note' : 'Save changes';
          setHint('Not saved: ' + humanError(err));
        });
      }

      // --- Events ---

      el.addEventListener('click', function (e) {
        var f = e.target.closest('[data-en-filter]');
        if (f && el.contains(f)) {
          filter = f.getAttribute('data-en-filter');
          Array.prototype.forEach.call(el.querySelectorAll('[data-en-filter]'), function (c) {
            c.setAttribute('aria-pressed', String(c === f));
          });
          refresh(false);
          return;
        }
        var aud = e.target.closest('[data-en-aud]');
        if (aud && comp) { setAudience(aud.getAttribute('data-en-aud')); return; }
        var person = e.target.closest('[data-en-person]');
        if (person && comp) {
          var pid = person.getAttribute('data-en-person');
          var at = comp.sharedWith.indexOf(pid);
          if (at >= 0) comp.sharedWith.splice(at, 1); else comp.sharedWith.push(pid);
          person.setAttribute('aria-pressed', String(at < 0));
          setHint();
          return;
        }
        var fmt = e.target.closest('[data-en-fmt]');
        if (fmt && comp && comp.editor) {
          var chain = comp.editor.chain().focus();
          var cmd = fmt.getAttribute('data-en-fmt');
          if (cmd === 'bold') chain.toggleBold().run();
          else if (cmd === 'italic') chain.toggleItalic().run();
          else if (cmd === 'bulletList') chain.toggleBulletList().run();
          return;
        }
        var t = e.target.closest('[data-en-act]');
        if (!t || !el.contains(t)) return;
        var act = t.getAttribute('data-en-act');
        var row = t.closest('.en-note');
        var id = row && row.getAttribute('data-en-id');
        if (act === 'new') openComposer(null);
        else if (act === 'cancel') closeComposer();
        else if (act === 'take-newer') takeNewer();
        else if (act === 'edit') openComposer(id);
        else if (act === 'pin') togglePin(id);
        else if (act === 'delete') del(id);
        else if (act === 'fold') {
          // A link in the note opens; a fold only when the note is longer
          // than its preview.
          if (e.target.closest('a')) return;
          var body = row && row.querySelector('.en-body');
          if (t.classList.contains('en-body') && !(body && body.classList.contains('is-foldable'))) return;
          if (t.classList.contains('en-body') && window.getSelection && String(window.getSelection())) return;
          fold(id);
        }
      });

      el.addEventListener('submit', function (e) {
        e.preventDefault();
        save();
      });

      el.addEventListener('keydown', function (e) {
        if (!comp || !comp.form.contains(e.target)) {
          return;
        }
        if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); save(); return; }
        // Enter in the title moves on to the note instead of saving it.
        if (e.key === 'Enter' && e.target.getAttribute('data-en-f') === 'title') { e.preventDefault(); focusComposer('body'); return; }
        if (e.key === 'Escape' && !composerDirty()) { e.preventDefault(); closeComposer(); el.querySelector('[data-en-act="new"]').focus(); return; }
        // Arrow keys move between the "Who can read it" choices.
        var radio = e.target.closest && e.target.closest('[data-en-aud]');
        if (radio && (e.key === 'ArrowRight' || e.key === 'ArrowLeft')) {
          var all = Array.prototype.slice.call(comp.form.querySelectorAll('[data-en-aud]'));
          var i = all.indexOf(radio) + (e.key === 'ArrowRight' ? 1 : -1);
          var next = all[(i + all.length) % all.length];
          setAudience(next.getAttribute('data-en-aud'));
          next.focus();
          e.preventDefault();
        }
      });

      // --- Live updates ---

      function connect() {
        if (destroyed || typeof window.WebSocket !== 'function') return;
        try {
          var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
          // The server reads the campaign from ?campaign=.
          ws = new WebSocket(protocol + '//' + window.location.host + '/ws?campaign=' + encodeURIComponent(campaignId));
          ws.addEventListener('open', function () { wsDelay = 2000; });
          ws.addEventListener('message', function (ev) {
            var msg;
            try { msg = JSON.parse(ev.data); } catch (err) { return; }
            if (!msg || msg.campaignId !== campaignId) return;
            if (msg.type === 'entity_note.created' || msg.type === 'entity_note.updated' || msg.type === 'entity_note.deleted') {
              // Refetch rather than apply the message: who may read a note
              // is decided on the server.
              loadNotes();
            }
          });
          ws.addEventListener('close', function () {
            ws = null;
            if (destroyed) return;
            // Come back after a restart or a dropped connection; the poll
            // carries the widget meanwhile.
            wsTimer = setTimeout(connect, wsDelay);
            wsDelay = Math.min(wsDelay * 2, 60000);
          });
          ws.addEventListener('error', function (ev) { if (ev.preventDefault) ev.preventDefault(); });
        } catch (err) {
          ws = null;
        }
      }

      function onVisible() {
        if (document.visibilityState === 'visible') loadNotes();
      }
      function onPageHide() { flushDeletes(); }

      loadMembers();
      loadNotes();
      connect();
      pollTimer = setInterval(loadNotes, 30000);
      agoTimer = setInterval(tickAgo, 60000);
      document.addEventListener('visibilitychange', onVisible);
      window.addEventListener('pagehide', onPageHide);

      // Unmount (HTMX swap, navigation): a delete still waiting for Undo is
      // carried out, and timers and the socket stop.
      el.__entityNotesCleanup = function () {
        flushDeletes();
        destroyed = true;
        if (comp && comp.editor) { try { comp.editor.destroy(); } catch (e) {} }
        comp = null;
        clearInterval(pollTimer);
        clearInterval(agoTimer);
        clearTimeout(wsTimer);
        clearTimeout(toastTimer);
        if (ws) { try { ws.close(); } catch (e) {} ws = null; }
        document.removeEventListener('visibilitychange', onVisible);
        window.removeEventListener('pagehide', onPageHide);
      };
    },

    destroy: function (el) {
      if (typeof el.__entityNotesCleanup === 'function') {
        el.__entityNotesCleanup();
        delete el.__entityNotesCleanup;
      }
      el.innerHTML = '';
    }
  });
})();
