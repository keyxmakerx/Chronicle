/**
 * page_editor.js -- the page editor: one page edited where it stands.
 *
 * Mount: the bar beside the page's visibility badge, <div
 * data-widget="page-editor">, drawn in the Annals look for whoever may change
 * the page (internal/plugins/entities/page_editor.go), so only they load this
 * (ADR-063).
 *
 * Edit page puts the page into editing: the page gets the campaign-colour
 * edge, a pen appears beside each part, and Edit page becomes Done in the same
 * spot. Nothing else moves. A part's pen, or a click on the part, opens it:
 * its values become fields where they stand, what has no room drapes on a
 * sheet below it over the page, and the pen becomes the part's tick. E
 * outlines every part (the E in the bar does the same); Esc closes the open
 * thing first, then leaves editing.
 *
 * Each field saves a moment after the last key, sending only its own key (the
 * metadata endpoint is a partial update: null clears), and the bar says
 * Saving… then Saved. A value the server would refuse waits rather than
 * failing as it is typed; closing its part puts it back and says why.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  if (C._pageEditor) return;

  var MAX_ALIASES = 10;
  var DIRTY_ID = 'page-editor';
  // Clicks on these keep their own meaning while editing, rather than
  // opening the part they sit in.
  var PASS = 'a, button, input, select, textarea, label, summary, [role="button"], [contenteditable], #page-eras, #page-history-slot';
  var WHAT = { name: 'the name', label: 'the descriptor', aka: 'the other names', parent: 'the parent page' };
  var OK_ICON = '<svg class="pe-i" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7"></path></svg>';

  // ---- pure helpers (test/js/page_editor.test.mjs) ----

  // The server measures lengths in bytes (Go's len), so the checks here do too.
  function utf8Len(s) {
    var n = 0;
    for (var i = 0; i < s.length; i++) {
      var c = s.charCodeAt(i);
      if (c < 0x80) n += 1;
      else if (c < 0x800) n += 2;
      else if (c >= 0xd800 && c <= 0xdbff && i + 1 < s.length) { n += 4; i++; }
      else n += 3;
    }
    return n;
  }

  function commaKey(s) {
    return s.split(',').map(function (x) { return x.trim(); }).join(',').toLowerCase();
  }

  // splitAliases reads the comma-separated field as the list the server
  // stores: trimmed, empties dropped, the first of any repeat kept. A saved
  // name that holds a comma itself ("Smith, John", from known) stays whole;
  // commas typed now split.
  function splitAliases(text, known) {
    var whole = {}, seen = {}, out = [];
    (known || []).forEach(function (k) { if (k.indexOf(',') >= 0) whole[commaKey(k)] = true; });
    var parts = String(text || '').split(',');
    for (var i = 0; i < parts.length; i++) {
      var a = parts[i];
      for (var j = parts.length; j > i + 1; j--) {
        var run = parts.slice(i, j).join(',');
        if (whole[commaKey(run)]) { a = run; i = j - 1; break; }
      }
      a = a.trim();
      var k = a.toLowerCase();
      if (!a || seen[k]) continue;
      seen[k] = true;
      out.push(a);
    }
    return out;
  }

  // aliasProblem says why the server would refuse the list, or ''.
  function aliasProblem(list) {
    if (list.length > MAX_ALIASES) return 'A page can have up to ' + MAX_ALIASES + ' other names.';
    for (var i = 0; i < list.length; i++) {
      var n = utf8Len(list[i]);
      if (n < 2) return 'Each other name needs at least 2 letters.';
      if (n > 200) return 'An other name can be at most 200 characters.';
    }
    return '';
  }

  // cleanName is the name as typed into the heading, on one line.
  function cleanName(text) { return String(text || '').replace(/\s+/g, ' ').trim(); }

  function nameProblem(name) {
    if (!name) return 'A page needs a name.';
    if (utf8Len(name) > 200) return 'A name can be at most 200 characters.';
    return '';
  }

  // metadataBody is one field's save: only its own key, and an emptied
  // descriptor or parent as null, which clears it.
  function metadataBody(key, value) {
    if (key === 'name') return { name: value };
    if (key === 'label') return { type_label: value === '' ? null : value };
    if (key === 'parent') return { parent_id: value === '' ? null : value };
    return null;
  }

  // keyAction is what a key does while editing: 'mark' outlines every part,
  // 'back' steps back, '' leaves the key alone. E is a typed letter in a
  // field, so it only marks when nothing is being typed.
  function keyAction(e, typing) {
    if (e.ctrlKey || e.metaKey || e.altKey) return '';
    if (e.key === 'Escape') return 'back';
    if ((e.key === 'e' || e.key === 'E') && !typing) return 'mark';
    return '';
  }

  function typingIn(t) {
    return !!t && (!!t.isContentEditable || !!(t.closest && t.closest('input, textarea, select')));
  }

  C._pageEditor = {
    utf8Len: utf8Len, splitAliases: splitAliases, aliasProblem: aliasProblem,
    cleanName: cleanName, nameProblem: nameProblem, metadataBody: metadataBody,
    keyAction: keyAction, typingIn: typingIn
  };

  if (!C.register) return;

  // ---- talking to the server ----

  function send(url, method, body) {
    return C.apiFetch(url, { method: method, body: body }).then(function (res) {
      if (res.ok) return res.json().catch(function () { return {}; });
      return res.json().catch(function () { return {}; }).then(function (b) {
        throw new Error((b && (b.message || b.error)) || 'The server said no.');
      });
    });
  }

  // ---- the Saving… / Saved note ----

  function note(S, state, text) {
    var el = S.saved;
    clearTimeout(S.noteTimer);
    el.classList.remove('is-busy', 'is-bad', 'is-on');
    el.removeAttribute('title');
    if (state === 'busy') { el.textContent = 'Saving…'; el.classList.add('is-on', 'is-busy'); return; }
    if (state === 'bad') { el.textContent = 'Not saved'; el.title = text; el.classList.add('is-on', 'is-bad'); return; }
    if (state === 'ok') {
      el.innerHTML = OK_ICON + '<span>Saved</span>';
      el.classList.add('is-on');
      S.noteTimer = setTimeout(function () { el.classList.remove('is-on'); }, 1600);
      return;
    }
    el.textContent = '';
  }

  // fail says what a field could not save: briefly in the bar, which keeps
  // its width, and in full in a notice.
  function fail(f, msg) {
    f.error = msg;
    if (C.notify) C.notify(msg, 'error', { duration: 8000 });
  }

  function settle(S) {
    var busy = false, waiting = false, bad = '';
    for (var k in S.fields) {
      var f = S.fields[k];
      if (f.busy) busy = true;
      if (f.timer || f.busy) waiting = true;
      if (f.error) bad = f.error;
    }
    if (waiting) { if (C.markDirty) C.markDirty(DIRTY_ID); }
    else if (C.markClean) C.markClean(DIRTY_ID);
    if (busy) note(S, 'busy');
    else if (bad) note(S, 'bad', bad);
    else if (S.didSave) { S.didSave = false; note(S, 'ok'); }
  }

  // ---- a field that saves as it is typed ----

  // field wires one value, drawn by el, to its save. o: read() the value now,
  // same(a, b), check(v) why it can't be saved yet or '', put(v) the save,
  // after(v, data) once saved, show(v) puts a value back on the page.
  function field(S, key, el, o) {
    var f = { key: key, el: el, saved: o.saved, timer: 0, busy: false, again: false, error: '', refused: null };
    function same(a, b) { return o.same ? o.same(a, b) : a === b; }
    f.change = function (delay) {
      clearTimeout(f.timer);
      f.timer = setTimeout(f.flush, delay);
      settle(S);
    };
    f.flush = function () {
      clearTimeout(f.timer);
      f.timer = 0;
      // While a save is out, the next one waits for it and is measured
      // against what it saved.
      if (f.busy) { f.again = true; return; }
      var v = o.read();
      // Back to what is saved, there is nothing left to say was not saved.
      if (same(v, f.saved)) { f.error = ''; f.refused = null; settle(S); return; }
      // A value the server refused is not sent again until it changes.
      if ((f.refused !== null && same(v, f.refused)) || (o.check && o.check(v))) { settle(S); return; }
      f.busy = true;
      f.refused = null;
      settle(S);
      o.put(v).then(function (data) {
        f.saved = v;
        f.error = '';
        S.didSave = true;
        if (o.after) o.after(v, data);
      }, function (err) {
        f.refused = v;
        fail(f, 'Couldn’t save ' + WHAT[key] + ': ' + err.message);
        if (!f.open()) o.show(f.saved);
      }).catch(function () {
        // Showing a saved value elsewhere failed; the save itself stands.
      }).then(function () {
        f.busy = false;
        if (f.again) { f.again = false; f.flush(); }
        else settle(S);
      });
    };
    // close saves what can be saved and puts back what can't, saying why.
    f.close = function () {
      var v = o.read();
      var why = o.check ? o.check(v) : '';
      if (why && !same(v, f.saved)) {
        clearTimeout(f.timer);
        f.timer = 0;
        o.show(f.saved);
        fail(f, 'Not saved: ' + why);
        settle(S);
        return;
      }
      f.flush();
      if (!f.busy) o.show(f.saved);
    };
    f.open = function () { return !!S.open && S.open.contains(el); };
    S.fields[key] = f;
    return f;
  }

  // ---- the header part ----

  function setupHeader(S, part) {
    var h1 = part.querySelector('[data-page-h1]');
    var aka = part.querySelector('[data-pe-f="aka"]');
    var label = part.querySelector('[data-pe-f="label"]');
    var parent = part.querySelector('[data-pe-f="parent"]');
    var meta = S.bar.getAttribute('data-metadata-endpoint');
    var H = { h1: h1, aka: aka, label: label, parent: parent, akaLoaded: false };
    S.header = H;

    if (h1) {
      field(S, 'name', h1, {
        saved: cleanName(h1.textContent),
        read: function () { return cleanName(h1.textContent); },
        check: nameProblem,
        put: function (v) { return send(meta, 'PUT', metadataBody('name', v)); },
        after: function (v, data) { renamed(S, (data && data.name) || v); },
        show: function (v) { if (cleanName(h1.textContent) !== v) h1.textContent = v; }
      });
    }
    if (label) {
      field(S, 'label', label, {
        saved: label.value.trim(),
        read: function () { return label.value.trim(); },
        put: function (v) { return send(meta, 'PUT', metadataBody('label', v)); },
        after: function (v) {
          if (!S.page.isConnected) return;
          var shown = S.page.querySelectorAll('[data-page-label]');
          for (var i = 0; i < shown.length; i++) { shown[i].textContent = v; shown[i].hidden = v === ''; }
        },
        show: function (v) { label.value = v; }
      });
    }
    if (aka) {
      field(S, 'aka', aka, {
        saved: [],
        // Untouched text is the saved list, however it splits.
        read: function () {
          if (!H.akaLoaded || aka.value === H.akaText) return S.fields.aka.saved;
          return splitAliases(aka.value, S.fields.aka.saved);
        },
        same: function (a, b) { return a.join('\n') === b.join('\n'); },
        check: aliasProblem,
        put: function (v) { return send(S.bar.getAttribute('data-aliases-endpoint'), 'PUT', { aliases: v }); },
        after: function () {
          if (C.invalidateAutoLinkCache) C.invalidateAutoLinkCache(S.bar.getAttribute('data-campaign-id'));
          var chips = part.querySelector('[data-widget="aliases"]');
          if (chips && chips.isConnected && C.destroyWidget && C.mountWidget) { C.destroyWidget(chips); C.mountWidget(chips); }
        },
        show: function (v) { if (H.akaLoaded) H.akaText = aka.value = v.join(', '); }
      });
    }
    if (parent) {
      field(S, 'parent', parent, {
        saved: parent.getAttribute('data-id') || '',
        read: function () { return parent.getAttribute('data-id') || ''; },
        put: function (v) { return send(meta, 'PUT', metadataBody('parent', v)); },
        after: function () { H.parentName = parent.value; },
        show: function () {
          parent.setAttribute('data-id', S.fields.parent.saved);
          parent.value = H.parentName;
        }
      });
      H.parentName = parent.value;
    }
  }

  // renamed carries a saved name to everything else on the page that shows
  // it. A save that lands after the page was left touches nothing of the next.
  function renamed(S, name) {
    var old = S.bar.getAttribute('data-name') || '';
    S.bar.setAttribute('data-name', name);
    if (!S.page.isConnected) return;
    var crumbs = S.page.querySelectorAll('[data-page-crumb]');
    for (var i = 0; i < crumbs.length; i++) crumbs[i].textContent = name;
    var id = S.bar.getAttribute('data-entity-id');
    var stars = S.page.querySelectorAll('[data-favorite-toggle]');
    for (var j = 0; j < stars.length; j++) {
      if (stars[j].getAttribute('data-favorite-toggle') === id) stars[j].setAttribute('data-entity-name', name);
    }
    if (old && document.title.indexOf(old) === 0) document.title = name + document.title.slice(old.length);
    document.dispatchEvent(new CustomEvent('chronicle:page-renamed', { bubbles: true, detail: { entityId: id, name: name } }));
  }

  function loadAliases(S) {
    var H = S.header;
    if (!H || !H.aka || H.akaLoaded || H.akaLoading) return;
    H.akaLoading = true;
    C.apiFetch(S.bar.getAttribute('data-aliases-endpoint'), { headers: { Accept: 'application/json' } })
      .then(function (res) { return res.ok ? res.json() : Promise.reject(new Error('aliases')); })
      .then(function (data) {
        var list = ((data && data.aliases) || []).map(function (a) { return a.alias; });
        S.fields.aka.saved = list;
        H.akaText = H.aka.value = list.join(', ');
        H.akaLoaded = true;
        H.aka.disabled = false;
      })
      .catch(function () { H.aka.placeholder = 'Couldn’t load the other names. Reopen to try again.'; })
      .then(function () { H.akaLoading = false; });
  }

  // ---- the parent page list ----

  function listOpen(S) { return !!(S.header && S.header.list && !S.header.list.hidden); }

  function showList(S, on) {
    var H = S.header, list = H.list;
    list.hidden = !on;
    H.parent.setAttribute('aria-expanded', on ? 'true' : 'false');
    if (!on) { H.parent.removeAttribute('aria-activedescendant'); H.active = -1; }
  }

  function renderList(S, results, failed, total) {
    var H = S.header, list = H.list;
    var self = S.bar.getAttribute('data-entity-id');
    list.textContent = '';
    H.options = [{ id: '', name: '' }];
    (results || []).forEach(function (it) {
      if (it && it.id && it.id !== self) H.options.push({ id: it.id, name: it.name || 'Untitled' });
    });
    H.options.forEach(function (o, i) {
      var li = document.createElement('li');
      li.id = 'pe-opt-' + i;
      li.className = 'pe-opt' + (o.id === '' ? ' is-none' : '');
      li.setAttribute('role', 'option');
      li.setAttribute('aria-selected', o.id === (H.parent.getAttribute('data-id') || '') ? 'true' : 'false');
      li.setAttribute('data-i', String(i));
      li.textContent = o.id === '' ? 'No parent page' : o.name;
      list.appendChild(li);
    });
    var say = failed ? 'Couldn’t search the pages. Try again.'
      : results && results.length === 0 ? 'No pages match.'
      : results && total > results.length ? 'More pages match. Type to narrow the list.' : '';
    if (say) {
      var m = document.createElement('li');
      m.className = 'pe-opt-note';
      m.setAttribute('role', 'presentation');
      m.textContent = say;
      list.appendChild(m);
    }
    H.active = -1;
    showList(S, true);
  }

  function searchParents(S) {
    var H = S.header;
    clearTimeout(H.searchTimer);
    H.searchTimer = setTimeout(function () {
      var q = H.parent.value.trim();
      // The search needs two letters; fewer lists the first pages by name.
      if (q.length < 2 || q === H.parentName) q = '';
      var seq = H.seq = (H.seq || 0) + 1;
      // pages=1 leaves out maps, timelines and the rest, which a page can't sit under.
      C.apiFetch(S.bar.getAttribute('data-search-endpoint') + '?pages=1&q=' + encodeURIComponent(q), { headers: { Accept: 'application/json' } })
        .then(function (res) { return res.ok ? res.json() : Promise.reject(new Error('search')); })
        .then(function (data) { if (seq === H.seq && S.open) renderList(S, (data && data.results) || [], false, (data && data.total) || 0); })
        .catch(function () { if (seq === H.seq && S.open) renderList(S, [], true); });
    }, 200);
  }

  function setActive(S, i) {
    var H = S.header, opts = H.list.querySelectorAll('[role="option"]');
    if (!opts.length) return;
    H.active = (i + opts.length) % opts.length;
    for (var j = 0; j < opts.length; j++) opts[j].classList.toggle('is-active', j === H.active);
    H.parent.setAttribute('aria-activedescendant', opts[H.active].id);
    opts[H.active].scrollIntoView({ block: 'nearest' });
  }

  function pick(S, i) {
    var H = S.header, o = H.options && H.options[i];
    if (!o) return;
    H.parent.setAttribute('data-id', o.id);
    H.parent.value = o.name;
    showList(S, false);
    S.fields.parent.flush();
  }

  // ---- open and close ----

  function penOf(part) { return part.querySelector('[data-pe-pen]'); }

  function openPart(S, part) {
    if (S.open === part) return;
    closePart(S, false);
    S.open = part;
    part.classList.add('is-open');
    var pen = penOf(part);
    if (pen) {
      pen.setAttribute('aria-expanded', 'true');
      pen.setAttribute('data-label', pen.getAttribute('aria-label') || '');
      pen.setAttribute('aria-label', 'Done');
      pen.setAttribute('title', 'Done');
    }
    var drape = part.querySelector('[data-pe-drape]');
    if (drape) drape.removeAttribute('inert');
    var H = S.header;
    if (H && H.h1 && part.contains(H.h1)) {
      H.h1.setAttribute('contenteditable', 'plaintext-only');
      // Browsers without the plain-text mode take any editable text instead;
      // paste is then kept plain below.
      if (H.h1.contentEditable !== 'plaintext-only') H.h1.setAttribute('contenteditable', 'true');
      H.h1.setAttribute('role', 'textbox');
      H.h1.setAttribute('aria-label', 'Name');
      H.h1.setAttribute('spellcheck', 'false');
      loadAliases(S);
      if (!('ontouchstart' in window)) {
        // A field clicked while the part opens keeps the focus.
        var from = document.activeElement;
        setTimeout(function () {
          if (S.open !== part || document.activeElement !== from) return;
          H.h1.focus({ preventScroll: true });
          var r = document.createRange();
          r.selectNodeContents(H.h1);
          r.collapse(false);
          var sel = window.getSelection();
          sel.removeAllRanges();
          sel.addRange(r);
        }, 40);
      }
    }
  }

  function closePart(S, refocus) {
    var part = S.open;
    if (!part) return;
    var hadFocus = part.contains(document.activeElement);
    var H = S.header;
    if (H && H.list) showList(S, false);
    for (var k in S.fields) if (part.contains(S.fields[k].el)) S.fields[k].close();
    S.open = null;
    part.classList.remove('is-open');
    var drape = part.querySelector('[data-pe-drape]');
    if (drape) drape.setAttribute('inert', '');
    if (H && H.h1 && part.contains(H.h1)) {
      H.h1.removeAttribute('contenteditable');
      H.h1.removeAttribute('role');
      H.h1.removeAttribute('aria-label');
      H.h1.removeAttribute('spellcheck');
    }
    var pen = penOf(part);
    if (pen) {
      pen.setAttribute('aria-expanded', 'false');
      pen.setAttribute('aria-label', pen.getAttribute('data-label') || '');
      pen.setAttribute('title', pen.getAttribute('data-label') || '');
      if (refocus !== false && hadFocus && S.editing) pen.focus({ preventScroll: true });
    }
  }

  function setMarked(S, on) {
    S.page.toggleAttribute('data-pe-marked', on);
    if (S.mark) S.mark.setAttribute('aria-pressed', on ? 'true' : 'false');
  }

  function setEditing(S, on) {
    if (!on) closePart(S, false);
    else {
      // A new round of editing starts without the last round's Not saved.
      for (var k in S.fields) S.fields[k].error = '';
      note(S, '');
    }
    S.editing = on;
    S.page.toggleAttribute('data-editing', on);
    setMarked(S, false);
    S.edit.textContent = on ? 'Done' : 'Edit page';
    S.edit.setAttribute('aria-pressed', on ? 'true' : 'false');
    S.edit.classList.toggle('is-done', on);
    var pens = S.page.querySelectorAll('[data-pe-pen]');
    for (var i = 0; i < pens.length; i++) pens[i].style.setProperty('--pe-n', String(i));
  }

  // ---- events ----

  function onClick(S, e) {
    var t = e.target;
    if (S.mark && S.mark.contains(t)) { if (S.editing) setMarked(S, !S.page.hasAttribute('data-pe-marked')); return; }
    if (S.edit.contains(t)) { setEditing(S, !S.editing); return; }
    if (!S.editing) return;
    var H = S.header;
    var opt = H && H.list && t.closest && t.closest('.pe-opt');
    if (opt && H.list.contains(opt)) { pick(S, +opt.getAttribute('data-i')); return; }
    var pen = t.closest && t.closest('[data-pe-pen]');
    if (pen && S.page.contains(pen)) {
      var p = pen.closest('[data-pe-part]');
      if (S.open === p) closePart(S); else openPart(S, p);
      return;
    }
    // History and Eras open their panels in the header, so the sheet steps
    // out of their way once their own handlers have run.
    var qw = t.closest && t.closest('.pe-qw');
    if (qw && qw.type !== 'submit') { setTimeout(function () { closePart(S, false); }, 0); return; }
    var part = t.closest && t.closest('[data-pe-part]');
    if (!part || part === S.open || !S.page.contains(part) || t.closest(PASS)) return;
    openPart(S, part);
  }

  function onInput(S, e) {
    var t = e.target, H = S.header;
    if (!H || !S.open) return;
    if (t === H.h1 && S.fields.name) S.fields.name.change(700);
    else if (t === H.label) S.fields.label.change(700);
    else if (t === H.aka) S.fields.aka.change(900);
    else if (t === H.parent) searchParents(S);
  }

  function onFieldKey(S, e) {
    var H = S.header, t = e.target;
    if (!H || !S.open) return;
    if (t === H.h1 && e.key === 'Enter') { e.preventDefault(); S.fields.name.flush(); return; }
    if ((t === H.label || t === H.aka) && e.key === 'Enter') { e.preventDefault(); S.fields[t === H.label ? 'label' : 'aka'].flush(); return; }
    if (t !== H.parent) return;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      if (!listOpen(S)) { searchParents(S); return; }
      var down = e.key === 'ArrowDown';
      // From the field, Down goes to the first option and Up to the last.
      setActive(S, H.active < 0 ? (down ? 0 : -1) : H.active + (down ? 1 : -1));
    } else if (e.key === 'Enter' && listOpen(S) && H.active >= 0) {
      e.preventDefault();
      pick(S, H.active);
    }
  }

  // A pasted name stays plain text where the browser has no plain-text mode.
  function onPaste(S, e) {
    var H = S.header;
    if (!H || e.target !== H.h1 || H.h1.getAttribute('contenteditable') !== 'true') return;
    e.preventDefault();
    var text = cleanName((e.clipboardData || window.clipboardData).getData('text'));
    document.execCommand('insertText', false, text);
  }

  function onFocusIn(S, e) {
    var H = S.header;
    if (H && e.target === H.parent && S.open) searchParents(S);
  }

  function onFocusOut(S, e) {
    var H = S.header, t = e.target;
    if (!H) return;
    if (t === H.parent) {
      // The list closes and the field shows what is chosen, unless focus
      // only moved into the list.
      setTimeout(function () {
        if (document.activeElement === H.parent) return;
        showList(S, false);
        if (!S.fields.parent.busy) {
          H.parent.setAttribute('data-id', S.fields.parent.saved);
          H.parent.value = H.parentName;
        }
      }, 0);
    } else if (t === H.h1 || t === H.label || t === H.aka) {
      S.fields[t === H.h1 ? 'name' : t === H.label ? 'label' : 'aka'].flush();
    }
  }

  function onKey(S, e) {
    // A key something else already handled, a dialog's Escape say, is not ours.
    if (!S.editing || !S.page.isConnected || e.defaultPrevented) return;
    var t = e.target;
    if (t !== document.body && t !== document.documentElement && !S.page.contains(t)) return;
    var act = keyAction(e, typingIn(t));
    if (act === 'mark') {
      e.preventDefault();
      setMarked(S, !S.page.hasAttribute('data-pe-marked'));
    } else if (act === 'back') {
      // Escape in a field outside the open part, the Eras search say, is
      // that field's own.
      if (typingIn(t) && !(S.open && S.open.contains(t))) return;
      // The Eras panel closes on Escape by itself; that is this press.
      var eras = document.getElementById('page-eras');
      if (eras && !eras.hidden && eras.classList.contains('is-open')) return;
      e.preventDefault();
      if (listOpen(S)) showList(S, false);
      else if (S.open) closePart(S);
      else { setEditing(S, false); S.edit.focus({ preventScroll: true }); }
    }
  }

  // ---- mount ----

  var live = new WeakMap();

  Chronicle.register('page-editor', {
    init: function (bar) {
      var page = bar.closest('[data-pe-page]');
      if (!page || live.has(bar)) return;
      // A layout with no part to edit has nothing for Edit page to do; the
      // badge beside it stays.
      if (!page.querySelector('[data-pe-part]')) { bar.setAttribute('data-pe-off', ''); return; }
      var S = {
        page: page, bar: bar, fields: {}, open: null, editing: false,
        edit: bar.querySelector('[data-pe-edit]'),
        mark: bar.querySelector('[data-pe-mark]'),
        saved: bar.querySelector('[data-pe-saved]'),
        didSave: false, noteTimer: 0
      };
      var head = page.querySelector('[data-pe-part="header"]');
      if (head) {
        setupHeader(S, head);
        S.header.list = head.querySelector('#pe-parent-list');
      }
      S.on = {
        click: function (e) { onClick(S, e); },
        input: function (e) { onInput(S, e); },
        keydown: function (e) { onFieldKey(S, e); },
        paste: function (e) { onPaste(S, e); },
        focusin: function (e) { onFocusIn(S, e); },
        focusout: function (e) { onFocusOut(S, e); },
        doc: function (e) { onKey(S, e); },
        // The list keeps focus in the field while an option is clicked.
        down: function (e) { if (e.target.closest && e.target.closest('.pe-list')) e.preventDefault(); }
      };
      page.addEventListener('click', S.on.click);
      page.addEventListener('input', S.on.input);
      page.addEventListener('keydown', S.on.keydown);
      page.addEventListener('paste', S.on.paste);
      page.addEventListener('focusin', S.on.focusin);
      page.addEventListener('focusout', S.on.focusout);
      page.addEventListener('mousedown', S.on.down);
      document.addEventListener('keydown', S.on.doc);
      live.set(bar, S);
    },
    // Leaving the page sends whatever is still waiting to be saved.
    destroy: function (bar) {
      var S = live.get(bar);
      if (!S) return;
      live.delete(bar);
      for (var k in S.fields) {
        var f = S.fields[k];
        if (f.timer) f.flush();
      }
      clearTimeout(S.noteTimer);
      if (S.header) clearTimeout(S.header.searchTimer);
      S.page.removeEventListener('click', S.on.click);
      S.page.removeEventListener('input', S.on.input);
      S.page.removeEventListener('keydown', S.on.keydown);
      S.page.removeEventListener('paste', S.on.paste);
      S.page.removeEventListener('focusin', S.on.focusin);
      S.page.removeEventListener('focusout', S.on.focusout);
      S.page.removeEventListener('mousedown', S.on.down);
      document.removeEventListener('keydown', S.on.doc);
      if (C.markClean) C.markClean(DIRTY_ID);
    }
  });
})();
