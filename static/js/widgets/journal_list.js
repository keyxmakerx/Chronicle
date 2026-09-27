/**
 * journal_list.js -- the Journal list's pure logic: filter, group, sort.
 *
 * No DOM and no fetches: journal.js hands it the index rows plus the list
 * state and draws the flat display list it returns. Kept separate so the
 * rules can be tested headless (test/js/journal_list.test.mjs).
 *
 * Exposes Chronicle.JournalList.
 */
(function (root) {
  'use strict';

  var JL = {};

  var VIS_LABELS = {
    private: 'Private',
    gm: 'GM only',
    party: 'Shared with party',
    custom: 'Specific people'
  };

  /** Label for a visibility value. */
  JL.visLabel = function (v) { return VIS_LABELS[v] || VIS_LABELS.private; };

  /**
   * Search-field qualifiers. A typed "kw:" becomes a token for `field`.
   * Notes carry no tags, so there is no tag: qualifier.
   */
  JL.TOKENS = [
    { kw: 'in', field: 'folder', hint: 'Folder' },
    { kw: 'by', field: 'owner', hint: 'Owner' },
    { kw: 'links', field: 'linksTo', hint: 'Links to a note or page' },
    { kw: 'is', field: 'is', hint: 'Pinned, or who can see it' },
    { kw: 'has', field: 'audio', hint: 'Has a recording' },
    { kw: 'before', field: 'updatedBefore', hint: 'Updated before a date' },
    { kw: 'after', field: 'updatedAfter', hint: 'Updated after a date' }
  ];

  /** The token vocabulary entry for a keyword, or null. */
  JL.tokenByKw = function (kw) {
    for (var i = 0; i < JL.TOKENS.length; i++) {
      if (JL.TOKENS[i].kw === kw) return JL.TOKENS[i];
    }
    return null;
  };

  /** Values the is: qualifier accepts, with their labels. */
  JL.IS_VALUES = [
    { value: 'pinned', label: 'pinned' },
    { value: 'private', label: 'private' },
    { value: 'gm', label: 'gm only' },
    { value: 'party', label: 'party' },
    { value: 'custom', label: 'specific people' }
  ];

  JL.GROUPS = [
    ['none', 'No grouping'], ['folder', 'Folder'], ['link', 'Linked page'],
    ['owner', 'Owner'], ['month', 'Month'], ['visibility', 'Visibility']
  ];
  JL.SORTS = [['updated', 'Last updated'], ['created', 'Date created'], ['title', 'Title']];

  var MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  var MONTHS_FULL = ['January', 'February', 'March', 'April', 'May', 'June', 'July',
    'August', 'September', 'October', 'November', 'December'];
  var DOW = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

  function toMs(v) {
    if (typeof v === 'number') return v;
    var t = Date.parse(v);
    return isNaN(t) ? 0 : t;
  }
  JL.toMs = toMs;

  /**
   * Short relative date for a row: "now", "5m", "3h", a weekday within the
   * week, then "Sep 8" (with the year once it is not this year).
   */
  JL.relDate = function (value, nowMs) {
    var ms = toMs(value);
    var mins = (nowMs - ms) / 60000;
    if (mins < 1) return 'now';
    if (mins < 60) return Math.round(mins) + 'm';
    var hrs = mins / 60;
    if (hrs < 24) return Math.round(hrs) + 'h';
    var d = new Date(ms);
    if (hrs < 24 * 7) return DOW[d.getDay()];
    var label = MONTHS[d.getMonth()] + ' ' + d.getDate();
    if (d.getFullYear() !== new Date(nowMs).getFullYear()) label += ', ' + d.getFullYear();
    return label;
  };

  /** Long date for the peek and the editor footer: "Sep 24, 4:10pm". */
  JL.longDate = function (value) {
    var d = new Date(toMs(value));
    var h = d.getHours();
    var ap = h >= 12 ? 'pm' : 'am';
    var h12 = h % 12 || 12;
    var mi = d.getMinutes();
    return MONTHS[d.getMonth()] + ' ' + d.getDate() + ', ' + h12 + ':' + (mi < 10 ? '0' : '') + mi + ap;
  };

  /** Short date for a date token label: "Sep 8". */
  JL.shortDate = function (ms) {
    var d = new Date(ms);
    return MONTHS[d.getMonth()] + ' ' + d.getDate();
  };

  function monthKey(ms) {
    var d = new Date(ms);
    return d.getFullYear() + '-' + (d.getMonth() < 9 ? '0' : '') + (d.getMonth() + 1);
  }
  function monthLabel(key) {
    var parts = key.split('-');
    return MONTHS_FULL[parseInt(parts[1], 10) - 1] + ' ' + parts[0];
  }

  /**
   * The folder path of a note ("Lore / Gods"), or "" when it sits at the
   * top level or in a folder the viewer cannot see.
   */
  JL.folderPath = function (folderId, folders) {
    var names = [];
    var seen = {};
    var cur = folderId;
    while (cur && folders[cur] && !seen[cur]) {
      seen[cur] = true;
      names.unshift(folders[cur].title || 'Untitled folder');
      cur = folders[cur].parentId;
    }
    return names.join(' / ');
  };

  /**
   * The first outgoing link that reads as a name for this viewer: a page's
   * name, or the title of a note they can see. Links to hidden notes are
   * skipped, never labelled.
   */
  JL.firstLink = function (note, ctx) {
    var links = note.links || [];
    for (var i = 0; i < links.length; i++) {
      var l = links[i];
      if (l.kind === 'page' && l.label) return { key: 'page:' + l.id, label: l.label };
      if (l.kind === 'note') {
        var t = ctx.noteTitle ? ctx.noteTitle(l.id) : '';
        if (t) return { key: 'note:' + l.id, label: t };
      }
    }
    return null;
  };

  /** Whether note passes one filter rule. */
  JL.ruleMatches = function (note, rule, ctx) {
    switch (rule.field) {
      case 'folder':
        return (note.parentId || '') === (rule.value || '');
      case 'owner':
        return note.userId === rule.value;
      case 'linksTo':
        return (note.links || []).some(function (l) { return l.id === rule.value; });
      case 'is':
        if (rule.value === 'pinned') return !!note.pinned;
        return note.visibility === rule.value;
      case 'audio':
        return !!note.hasAudio;
      case 'updatedBefore':
        return toMs(note.updatedAt) < rule.value;
      case 'updatedAfter':
        return toMs(note.updatedAt) > rule.value;
      default:
        return true;
    }
  };

  /**
   * Whether note matches the free-text query: its title, or (searching
   * contents) the server's content hits. Returns {hit, snippet}.
   */
  JL.searchHit = function (note, state, ctx) {
    var q = (state.query || '').trim().toLowerCase();
    if (!q) return { hit: true, snippet: null };
    if ((note.title || '').toLowerCase().indexOf(q) !== -1) return { hit: true, snippet: null };
    if (state.scope === 'contents' && ctx.contentHits && ctx.contentHits[note.id] !== undefined) {
      return { hit: true, snippet: ctx.contentHits[note.id] };
    }
    return { hit: false, snippet: null };
  };

  function sortNotes(list, state) {
    var dir = state.sortDir === 'asc' ? 1 : -1;
    var by = state.sortBy;
    var arr = list.slice();
    arr.sort(function (a, b) {
      if (by === 'title') {
        var at = (a.title || '').toLowerCase();
        var bt = (b.title || '').toLowerCase();
        if (at < bt) return -1 * dir;
        if (at > bt) return 1 * dir;
        return 0;
      }
      var av = toMs(by === 'created' ? a.createdAt : a.updatedAt);
      var bv = toMs(by === 'created' ? b.createdAt : b.updatedAt);
      return (av - bv) * dir;
    });
    return arr;
  }

  function groupKey(note, groupBy, ctx) {
    switch (groupBy) {
      case 'folder': return note.parentId && ctx.folders[note.parentId] ? note.parentId : '';
      case 'owner': return note.userId;
      case 'month': return monthKey(toMs(note.updatedAt));
      case 'visibility': return note.visibility || 'private';
      case 'link':
        var l = JL.firstLink(note, ctx);
        return l ? l.key : '';
      default: return '';
    }
  }

  function groupLabel(key, groupBy, ctx, sample) {
    switch (groupBy) {
      case 'folder': return key ? JL.folderPath(key, ctx.folders) : 'No folder';
      case 'owner': return ctx.memberName ? ctx.memberName(key) : key;
      case 'month': return monthLabel(key);
      case 'visibility': return JL.visLabel(key);
      case 'link':
        if (!key) return 'Not linked';
        var l = JL.firstLink(sample, ctx);
        return l ? l.label : 'Not linked';
      default: return key;
    }
  }

  var VIS_ORDER = { private: 0, gm: 1, party: 2, custom: 3 };

  /**
   * Filter, split out the pinned group, group and sort, into one flat list
   * of {kind:'header'} and {kind:'row'} items for drawing.
   *
   * @param {Array} notes - index rows (no folders).
   * @param {Object} state - query, scope, filters, groupBy, sortBy, sortDir,
   *   archiveView, collapsed ({groupKey: true}).
   * @param {Object} ctx - folders ({id: {title, parentId}}), memberName(id),
   *   noteTitle(id), contentHits ({id: snippet} or null), emptyFolders
   *   (ids of visible folders to show even when empty).
   */
  JL.compute = function (notes, state, ctx) {
    var matched = [];
    var snippets = {};
    for (var i = 0; i < notes.length; i++) {
      var n = notes[i];
      if (state.archiveView ? !n.archived : n.archived) continue;
      var ok = true;
      var rules = state.filters || [];
      for (var r = 0; r < rules.length && ok; r++) ok = JL.ruleMatches(n, rules[r], ctx);
      if (!ok) continue;
      var hit = JL.searchHit(n, state, ctx);
      if (!hit.hit) continue;
      if (hit.snippet) snippets[n.id] = hit.snippet;
      matched.push(n);
    }

    var items = [];
    var collapsed = state.collapsed || {};
    var pinned = state.archiveView ? [] : matched.filter(function (x) { return x.pinned; });
    var rest = state.archiveView ? matched : matched.filter(function (x) { return !x.pinned; });

    if (pinned.length) {
      items.push({ kind: 'header', key: '__pinned__', label: 'Pinned', count: pinned.length, pinned: true });
      if (!collapsed.__pinned__) {
        sortNotes(pinned, state).forEach(function (x) { items.push({ kind: 'row', note: x, snippet: snippets[x.id] || null }); });
      }
    }

    var sorted = sortNotes(rest, state);
    if (!state.groupBy || state.groupBy === 'none') {
      sorted.forEach(function (x) { items.push({ kind: 'row', note: x, snippet: snippets[x.id] || null }); });
    } else {
      var order = [];
      var buckets = {};
      sorted.forEach(function (x) {
        var k = groupKey(x, state.groupBy, ctx);
        if (!buckets[k]) { buckets[k] = []; order.push(k); }
        buckets[k].push(x);
      });
      // An empty folder still gets its header while nothing narrows the
      // list, so a folder just made can be seen, filed into and managed.
      var narrowed = (state.query || '').trim() || (state.filters || []).length || state.archiveView;
      if (state.groupBy === 'folder' && !narrowed) {
        (ctx.emptyFolders || []).forEach(function (fid) {
          if (!buckets[fid]) { buckets[fid] = []; order.push(fid); }
        });
      }
      order.sort(function (a, b) {
        if (state.groupBy === 'month') return a < b ? 1 : a > b ? -1 : 0;
        if (state.groupBy === 'visibility') return VIS_ORDER[a] - VIS_ORDER[b];
        // The catch-all group ("No folder", "Not linked") goes last.
        if (!a) return 1;
        if (!b) return -1;
        var la = groupLabel(a, state.groupBy, ctx, buckets[a][0]).toLowerCase();
        var lb = groupLabel(b, state.groupBy, ctx, buckets[b][0]).toLowerCase();
        return la < lb ? -1 : la > lb ? 1 : 0;
      });
      order.forEach(function (k) {
        items.push({
          kind: 'header', key: k, label: groupLabel(k, state.groupBy, ctx, buckets[k][0]),
          count: buckets[k].length, folderId: state.groupBy === 'folder' ? k : null
        });
        if (!collapsed[k]) {
          buckets[k].forEach(function (x) { items.push({ kind: 'row', note: x, snippet: snippets[x.id] || null }); });
        }
      });
    }

    var archivedTotal = 0;
    for (var j = 0; j < notes.length; j++) if (notes[j].archived) archivedTotal++;
    return { items: items, matched: matched.length, archivedTotal: archivedTotal };
  };

  /**
   * The rows between two ids in display order, inclusive, for shift-click
   * range selection. Returns [] when either end is not a row.
   */
  JL.rangeBetween = function (items, fromId, toId) {
    var ids = [];
    for (var i = 0; i < items.length; i++) if (items[i].kind === 'row') ids.push(items[i].note.id);
    var a = ids.indexOf(fromId);
    var b = ids.indexOf(toId);
    if (a === -1 || b === -1) return [];
    return ids.slice(Math.min(a, b), Math.max(a, b) + 1);
  };

  /**
   * Parses a date typed after before:/after: ("2026-09-01", "Sep 1 2026").
   * Returns epoch ms or null.
   */
  JL.parseDate = function (text) {
    if (!text) return null;
    var t = Date.parse(text);
    return isNaN(t) ? null : t;
  };

  root.Chronicle = root.Chronicle || {};
  root.Chronicle.JournalList = JL;
})(typeof window !== 'undefined' ? window : this);
