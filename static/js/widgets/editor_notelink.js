/**
 * editor_notelink.js -- [[links]] between notes, and from notes to pages.
 *
 * Chronicle.NoteLink is a TipTap inline node for a link to a note. It stores
 * only the note's id: the HTML it writes reads "Journal note", and on screen
 * it is labelled through Chronicle.NoteLabels, which asks the server what
 * this viewer may see. So a note's title is never written into a body, and
 * a link to a note the reader cannot open shows "Private note" instead.
 *
 * Chronicle.WikiLinkExtension({...}) is the "[[" picker: type [[ and a name
 * to link a note or a page. Same lifecycle hooks as MentionExtension
 * (onCreate / onUpdate / onKeyDown / onDestroy), wired by the host editor.
 */
(function () {
  'use strict';

  window.Chronicle = window.Chronicle || {};
  if (!window.TipTap || !window.TipTap.Node) {
    console.error('[NoteLink] TipTap bundle with Node export not loaded.');
    return;
  }

  var NOTE_LINK_TEXT = 'Journal note';
  var HIDDEN_LABEL = 'Private note';
  var ID_RE = /^[0-9a-fA-F-]{8,64}$/;

  // --- Labels -------------------------------------------------------------

  /**
   * Chronicle.NoteLabels caches what each note link should read for this
   * viewer, per campaign: {title, archived} for a note they can see, null
   * for one they cannot (or that no longer exists, which reads the same).
   */
  var cache = {};
  var queued = {};
  var timers = {};
  var listeners = [];

  function key(campaignId, id) { return campaignId + ':' + id; }

  function notify() {
    for (var i = 0; i < listeners.length; i++) {
      try { listeners[i](); } catch (e) { /* a dead view must not stop the rest */ }
    }
  }

  function flush(campaignId) {
    timers[campaignId] = null;
    var ids = Object.keys(queued[campaignId] || {});
    queued[campaignId] = {};
    if (!ids.length) return;
    Chronicle.apiFetch('/campaigns/' + encodeURIComponent(campaignId) + '/notes/labels?ids=' + encodeURIComponent(ids.join(',')))
      .then(function (r) { return r.ok ? r.json() : {}; })
      .then(function (map) {
        ids.forEach(function (id) {
          var l = map && map[id];
          cache[key(campaignId, id)] = l ? { title: l.title, archived: !!l.archived } : null;
        });
        notify();
      })
      .catch(function () {
        ids.forEach(function (id) { cache[key(campaignId, id)] = null; });
        notify();
      });
  }

  Chronicle.NoteLabels = {
    /** The cached label: undefined (not asked yet), null (hidden) or {title, archived}. */
    get: function (campaignId, id) {
      return cache[key(campaignId, id)];
    },
    /** Asks the server for id's label, batched with others asked this tick. */
    want: function (campaignId, id) {
      if (!campaignId || !ID_RE.test(id || '') || cache[key(campaignId, id)] !== undefined) return;
      queued[campaignId] = queued[campaignId] || {};
      queued[campaignId][id] = true;
      if (!timers[campaignId]) timers[campaignId] = setTimeout(function () { flush(campaignId); }, 0);
    },
    /**
     * Fills labels the host already knows (the Journal's own list). Only
     * notes the viewer can see are ever passed in; everything else stays
     * unknown and is asked for.
     */
    prime: function (campaignId, notes) {
      (notes || []).forEach(function (n) {
        cache[key(campaignId, n.id)] = { title: n.title, archived: !!n.archived };
      });
      notify();
    },
    /** Marks one note gone or hidden (deleted, or shared away). */
    forget: function (campaignId, id) {
      cache[key(campaignId, id)] = null;
      notify();
    },
    /**
     * Drops every cached label of a campaign, so each is asked for again.
     * Called when sharing may have changed: a title the viewer could see a
     * moment ago must not outlive their access to it.
     */
    reset: function (campaignId) {
      var prefix = campaignId + ':';
      Object.keys(cache).forEach(function (k) {
        if (k.indexOf(prefix) === 0) delete cache[k];
      });
    },
    subscribe: function (fn) {
      listeners.push(fn);
      return function () {
        var i = listeners.indexOf(fn);
        if (i !== -1) listeners.splice(i, 1);
      };
    },
    /** The text a link to id reads as right now. */
    text: function (campaignId, id) {
      var l = cache[key(campaignId, id)];
      if (l === undefined) return NOTE_LINK_TEXT;
      return l ? l.title : HIDDEN_LABEL;
    }
  };

  function hrefFor(campaignId, id) {
    if (!campaignId || !ID_RE.test(id || '')) return null;
    return '/campaigns/' + encodeURIComponent(campaignId) + '/journal/' + encodeURIComponent(id);
  }

  /**
   * Draws a note link's label into el and keeps its state classes current.
   * The label is text, never HTML.
   */
  function paintLink(el, campaignId, id) {
    var l = Chronicle.NoteLabels.get(campaignId, id);
    el.textContent = Chronicle.NoteLabels.text(campaignId, id);
    el.classList.toggle('note-link--hidden', l === null);
    el.classList.toggle('note-link--archived', !!(l && l.archived));
    el.title = l === null ? 'A note you can’t see' : (l && l.archived ? 'Archived note' : '');
  }

  /**
   * Labels every note link inside root that is not an editor node view:
   * read-only HTML such as a jot's body. Returns an unsubscribe function.
   */
  Chronicle.hydrateNoteLinks = function (root, campaignId) {
    function paint() {
      var links = root.querySelectorAll('a[data-note-id]');
      for (var i = 0; i < links.length; i++) {
        var id = links[i].getAttribute('data-note-id');
        Chronicle.NoteLabels.want(campaignId, id);
        paintLink(links[i], campaignId, id);
        links[i].classList.add('note-link');
      }
    }
    paint();
    return Chronicle.NoteLabels.subscribe(paint);
  };

  // --- The node -------------------------------------------------------------

  /**
   * Chronicle.NoteLink: configure with {campaignId, onOpen(noteId)}. With
   * onOpen, a click opens the note in place (the Journal); without it, the
   * link's href opens the Journal at the note.
   */
  Chronicle.NoteLink = TipTap.Node.create({
    name: 'noteLink',
    group: 'inline',
    inline: true,
    atom: true,
    selectable: true,
    draggable: false,

    addOptions: function () {
      return { campaignId: '', onOpen: null };
    },

    addAttributes: function () {
      return {
        noteId: {
          default: null,
          parseHTML: function (el) {
            var id = el.getAttribute('data-note-id');
            return ID_RE.test(id || '') ? id : null;
          },
          renderHTML: function (attrs) {
            return attrs.noteId ? { 'data-note-id': attrs.noteId } : {};
          }
        }
      };
    },

    // Above the Link mark's `a[href]` rule, so a note link is always this
    // node and never a plain link carrying stale text.
    parseHTML: function () {
      return [{ tag: 'a[data-note-id]', priority: 1000 }];
    },

    renderHTML: function (props) {
      var attrs = { class: 'note-link' };
      var href = hrefFor(this.options.campaignId, props.node.attrs.noteId);
      if (href) attrs.href = href;
      return ['a', TipTap.mergeAttributes(props.HTMLAttributes, attrs), NOTE_LINK_TEXT];
    },

    renderText: function (props) {
      return Chronicle.NoteLabels.text(this.options.campaignId, props.node.attrs.noteId);
    },

    addNodeView: function () {
      var options = this.options;
      return function (props) {
        var id = props.node.attrs.noteId;
        var dom = document.createElement('a');
        dom.className = 'note-link';
        dom.setAttribute('data-note-id', id || '');
        dom.setAttribute('contenteditable', 'false');
        var href = hrefFor(options.campaignId, id);
        if (href) dom.setAttribute('href', href);

        function repaint() {
          // After a reset the label is unknown again: ask, then paint.
          Chronicle.NoteLabels.want(options.campaignId, id);
          paintLink(dom, options.campaignId, id);
        }
        repaint();
        var off = Chronicle.NoteLabels.subscribe(repaint);

        dom.addEventListener('click', function (e) {
          if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
          if (typeof options.onOpen === 'function') {
            e.preventDefault();
            options.onOpen(id);
          }
        });

        return {
          dom: dom,
          update: function (node) {
            return node.type.name === 'noteLink' && node.attrs.noteId === id;
          },
          destroy: off,
          ignoreMutation: function () { return true; }
        };
      };
    }
  });

  // --- The [[ picker ----------------------------------------------------------

  /**
   * Chronicle.WikiLinkExtension(options) — the [[ picker.
   *   campaignId: string
   *   notes(query): [{id, title, sub}] the viewer can link to, already
   *     without archived notes and without the note being edited.
   *   pages: false to offer notes only (default true: pages come from the
   *     entity search, like @mentions).
   */
  Chronicle.WikiLinkExtension = function (options) {
    var editor = null;
    var active = false;
    var startPos = null;
    var popup = null;
    var items = [];
    var index = 0;
    var query = '';
    var pageTimer = null;
    var pageAbort = null;
    var pageItems = [];

    function el() {
      if (!popup) {
        popup = document.createElement('div');
        popup.className = 'nl-ac';
        popup.setAttribute('role', 'listbox');
        popup.addEventListener('mousedown', function (e) {
          var row = e.target.closest('[data-i]');
          if (!row) return;
          e.preventDefault();
          accept(parseInt(row.getAttribute('data-i'), 10));
        });
        document.body.appendChild(popup);
      }
      return popup;
    }

    function render() {
      var box = el();
      if (!items.length) {
        box.innerHTML = '<div class="nl-ac-empty">' +
          (query.length < 2 && options.pages !== false ? 'Keep typing to find a note or a page' : 'No matches — keep typing, or Esc to cancel') +
          '</div>';
        return;
      }
      box.innerHTML = items.map(function (it, i) {
        return '<div class="nl-ac-item' + (i === index ? ' is-active' : '') + '" role="option" data-i="' + i + '">' +
          '<i class="fa-solid ' + (it.kind === 'page' ? 'fa-book-open nl-ac-page' : 'fa-file-lines') + '" aria-hidden="true"></i>' +
          '<span class="nl-ac-text"><span class="nl-ac-ttl">' + Chronicle.escapeHtml(it.title) + '</span>' +
          '<span class="nl-ac-sub">' + Chronicle.escapeHtml(it.sub || '') + '</span></span></div>';
      }).join('');
    }

    function place() {
      if (!popup || !editor) return;
      var c;
      try { c = editor.view.coordsAtPos(editor.state.selection.from); } catch (e) { return; }
      var w = Math.min(290, window.innerWidth - 16);
      var left = Math.max(8, Math.min(c.left, window.innerWidth - w - 8));
      var top = c.bottom + 6;
      if (top + 250 > window.innerHeight) top = Math.max(8, c.top - 256);
      popup.style.left = left + 'px';
      popup.style.top = top + 'px';
    }

    function collect() {
      var notes = (options.notes ? options.notes(query) : []).slice(0, 6).map(function (n) {
        return { kind: 'note', id: n.id, title: n.title, sub: n.sub || 'Journal note' };
      });
      items = notes.concat(pageItems.slice(0, 4));
      if (index >= items.length) index = 0;
    }

    function fetchPages() {
      if (options.pages === false) return;
      if (pageTimer) clearTimeout(pageTimer);
      if (query.length < 2) { pageItems = []; return; }
      var q = query;
      pageTimer = setTimeout(function () {
        if (pageAbort) pageAbort.abort();
        pageAbort = new AbortController();
        Chronicle.apiFetch('/campaigns/' + encodeURIComponent(options.campaignId) + '/entities/search?q=' + encodeURIComponent(q), { signal: pageAbort.signal })
          .then(function (r) { return r.ok ? r.json() : { results: [] }; })
          .then(function (data) {
            if (!active || q !== query) return;
            pageItems = (data.results || []).map(function (e) {
              return { kind: 'page', id: e.id, title: e.name, sub: e.type_name || 'Page', url: e.url };
            });
            collect();
            render();
          })
          .catch(function () { /* transient: the notes already listed stay */ });
      }, 180);
    }

    function open() {
      active = true;
      index = 0;
      el().classList.add('is-open');
      place();
    }

    function close() {
      active = false;
      startPos = null;
      items = [];
      pageItems = [];
      if (pageTimer) clearTimeout(pageTimer);
      if (pageAbort) { pageAbort.abort(); pageAbort = null; }
      if (popup) popup.classList.remove('is-open');
    }

    function accept(i) {
      var it = items[i];
      if (!it || !editor || startPos === null) { close(); return; }
      var from = startPos;
      var to = editor.state.selection.from;
      var chain = editor.chain().focus().deleteRange({ from: from, to: to });
      if (it.kind === 'note') {
        chain.insertContent([{ type: 'noteLink', attrs: { noteId: it.id } }, { type: 'text', text: ' ' }]).run();
      } else {
        var url = it.url || ('/campaigns/' + options.campaignId + '/entities/' + it.id);
        chain.insertContent(
          '<a data-mention-id="' + Chronicle.escapeAttr(it.id) + '" href="' + Chronicle.escapeAttr(url) + '" ' +
          'data-entity-preview="' + Chronicle.escapeAttr(url + '/preview') + '">' + Chronicle.escapeHtml(it.title) + '</a>&nbsp;'
        ).run();
      }
      close();
      if (typeof options.onLinked === 'function') options.onLinked(it);
    }

    return {
      /** Starts a [[ at the cursor, as if typed (the toolbar button). */
      begin: function () {
        if (editor) editor.chain().focus().insertContent('[[').run();
      },
      isOpen: function () { return active; },
      /** Lists the notes again, for a host whose notes arrived after [[ was typed. */
      refresh: function () {
        if (!active) return;
        collect();
        render();
      },
      close: close,
      onCreate: function (ed) { editor = ed; },
      onUpdate: function (ed) {
        editor = ed;
        if (!ed.isEditable) return;
        var state = ed.state;
        var from = state.selection.from;
        var $pos = state.doc.resolve(from);
        if (!$pos.parent || !$pos.parent.isTextblock) { if (active) close(); return; }
        var before = state.doc.textBetween($pos.start(), from, '\0', '\0');
        var m = before.match(/\[\[([^\[\]\n\0]{0,60})$/);
        if (!m) { if (active) close(); return; }
        startPos = from - m[0].length;
        query = m[1];
        if (!active) open();
        collect();
        render();
        place();
        fetchPages();
      },
      onKeyDown: function (ed, e) {
        if (!active) return false;
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
          if (items.length) {
            index = (index + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
            render();
          }
          return true;
        }
        if (e.key === 'Enter' || e.key === 'Tab') {
          if (items.length) { accept(index); return true; }
          return false;
        }
        if (e.key === 'Escape') { close(); return true; }
        return false;
      },
      onDestroy: function () {
        close();
        if (popup && popup.parentNode) popup.parentNode.removeChild(popup);
        popup = null;
        editor = null;
      }
    };
  };
})();
