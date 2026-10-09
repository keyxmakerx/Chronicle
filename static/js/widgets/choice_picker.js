/**
 * choice_picker.js -- "pick from the game system's list" for character fields.
 *
 * Opens an in-place fold (search box + list of names with a short summary)
 * under an anchor element. The value stored in fields_data stays the chosen
 * entry's NAME as plain text, so Foundry pulls (which write item names) stay
 * compatible; a value that is not in the list is still shown and kept, and
 * "Not listed? Type your own" always works.
 *
 * Shared on purpose: the attributes widget uses it, and so can a system
 * package's own widget (Draw Steel hero pages do not use attributes):
 *
 *   Chronicle.pickChoice({
 *     campaignId: '...',        // required
 *     fieldKey:   'ancestry',   // required, plain identifier
 *     anchorEl:   buttonEl,     // required; the fold opens right after it
 *     current:    'Human',      // value shown as selected (or typed text)
 *     label:      'Ancestry',   // heading; defaults to the field key
 *     entityId:   '...',        // when set (and save !== false) a pick is saved
 *     save:       true,         // false: only resolve with the value
 *     choices:    list          // optional pre-fetched Chronicle.fetchChoices result
 *   }).then(function (res) { res === null ? cancelled : res.value });
 *
 * Saving sends ONLY the changed field ({fields_patch: {key: value}}) to
 * PUT /campaigns/:id/entities/:eid/fields, the same Scribe+ endpoint as any
 * field edit; the server merges it so the other fields are untouched.
 *
 *   Chronicle.fetchChoices(campaignId, fieldKey)
 *     -> Promise<{systemName, choices: [{slug,name,summary,source}]}>
 *
 * Calling pickChoice again on an anchor whose fold is open closes it.
 */
(function () {
  'use strict';

  window.Chronicle = window.Chronicle || {};

  var OPEN = '__chronicleChoiceFold';

  // The saved line is text from the system's data or the user's own typing,
  // so it goes in with textContent, never through an HTML toast.
  function toast(msg) {
    var old = document.querySelector('.ag-toast');
    if (old) old.remove();
    var t = document.createElement('div');
    t.className = 'ag-toast';
    t.setAttribute('role', 'status');
    var i = document.createElement('i');
    i.className = 'fa-solid fa-circle-check';
    var span = document.createElement('span');
    span.textContent = msg;
    t.appendChild(i);
    t.appendChild(span);
    document.body.appendChild(t);
    setTimeout(function () { t.remove(); }, 3200);
  }

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function reduced() {
    return matchMedia('(prefers-reduced-motion: reduce)').matches ||
      !!document.querySelector('.nav-rm, html.rm');
  }

  // Opens or shuts a .ag-fold by animating its height to the content's.
  function fold(el, open, done) {
    var finished = false;
    function fin() {
      if (finished) return;
      finished = true;
      el.removeEventListener('transitionend', onEnd);
      if (open) el.style.height = 'auto';
      if (done) done();
    }
    function onEnd(e) { if (e.target === el && e.propertyName === 'height') fin(); }
    if (reduced()) {
      el.style.height = open ? 'auto' : '0px';
      el.classList.toggle('is-open', open);
      finished = true;
      if (done) done();
      return;
    }
    var from = el.getBoundingClientRect().height;
    el.style.height = 'auto';
    var to = open ? el.scrollHeight : 0;
    el.style.height = from + 'px';
    el.getBoundingClientRect();
    el.classList.toggle('is-open', open);
    el.style.height = to + 'px';
    el.addEventListener('transitionend', onEnd);
    // No transition fires when the height does not change.
    setTimeout(fin, 420);
  }

  Chronicle.fetchChoices = function (campaignId, fieldKey) {
    var url = '/campaigns/' + encodeURIComponent(campaignId) +
      '/character-choices/' + encodeURIComponent(fieldKey);
    return Chronicle.apiFetch(url).then(function (r) {
      if (!r.ok) throw new Error('choices ' + r.status);
      return r.json();
    }).then(function (d) {
      return { systemName: d.systemName || '', choices: d.choices || [] };
    });
  };

  function saveField(opts, value) {
    var patch = {};
    patch[opts.fieldKey] = value;
    return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(opts.campaignId) +
      '/entities/' + encodeURIComponent(opts.entityId) + '/fields', {
      method: 'PUT',
      body: { fields_patch: patch }
    }).then(function (r) {
      if (!r.ok) throw new Error('save ' + r.status);
    });
  }

  Chronicle.pickChoice = function (opts) {
    opts = opts || {};
    var anchor = opts.anchorEl;
    if (!anchor || !opts.campaignId || !opts.fieldKey) {
      return Promise.reject(new Error('pickChoice needs campaignId, fieldKey and anchorEl'));
    }

    // Second call on the same anchor closes the open fold.
    var existing = anchor[OPEN];
    if (existing) { existing.close(null); return Promise.resolve(null); }

    var label = opts.label || opts.fieldKey;
    var willSave = !!opts.entityId && opts.save !== false;
    var current = opts.current == null ? '' : String(opts.current);
    var list = null;      // {systemName, choices}
    var closed = false;
    var resolveFn;
    var promise = new Promise(function (res) { resolveFn = res; });

    var wrap = document.createElement('div');
    wrap.className = 'ag-fold';
    anchor.parentNode.insertBefore(wrap, anchor.nextSibling);
    anchor.setAttribute('aria-expanded', 'true');

    var box = document.createElement('div');
    box.className = 'ag-box ag-pickbox';
    box.style.marginTop = '6px';
    wrap.appendChild(box);

    function close(result) {
      if (closed) return;
      closed = true;
      delete anchor[OPEN];
      anchor.setAttribute('aria-expanded', 'false');
      fold(wrap, false, function () {
        if (wrap.parentNode) wrap.parentNode.removeChild(wrap);
      });
      if (anchor.focus) anchor.focus();
      resolveFn(result);
    }
    anchor[OPEN] = { close: close };

    function header(srcName) {
      return '<div class="ag-head"><h4>' + esc(label) + '</h4>' +
        (srcName ? '<span class="ag-src">from ' + esc(srcName) + '</span>' : '') +
        '<button type="button" class="ag-x" data-x aria-label="Close"><i class="fa-solid fa-xmark"></i></button></div>';
    }

    function renderLoading() {
      box.innerHTML = header('') +
        '<div class="ag-body"><div class="ag-more" role="status"><i class="fa-solid fa-spinner fa-spin"></i> Loading the list…</div></div>';
      wire();
      fold(wrap, true);
    }

    function renderError() {
      box.innerHTML = header('') +
        '<div class="ag-body"><div class="ag-more" role="alert">The list could not be loaded. ' +
        '<button type="button" class="ag-link" data-retry>Try again</button></div>' +
        '<div class="ag-mine"><button type="button" class="ag-link" data-own-only>Type it in instead</button></div></div>';
      wire();
      fold(wrap, true);
    }

    function rows(q) {
      var all = list.choices;
      var m = all.filter(function (x) {
        return !q || (x.name + ' ' + x.summary).toLowerCase().indexOf(q) >= 0;
      });
      if (!m.length) return '<div class="ag-more">No match for “' + esc(q) + '”.</div>';
      return m.map(function (x) {
        return '<label class="ag-row"><input type="radio" name="ag-pick-' + esc(opts.fieldKey) +
          '" value="' + esc(x.name) + '"' + (x.name === current ? ' checked' : '') + '>' +
          '<span class="ag-name"><b>' + esc(x.name) + '</b>' +
          (x.summary ? '<small>' + esc(x.summary) + '</small>' : '') + '</span></label>';
      }).join('') + '<div class="ag-more">' + m.length + ' of ' + all.length + '</div>';
    }

    function renderBody() {
      var has = list.choices.length > 0;
      var sys = list.systemName || 'This game system';
      var html = header(has ? list.systemName : '') + '<div class="ag-body">';
      if (has) {
        html += '<div data-pickpane><div class="ag-search"><i class="fa-solid fa-magnifying-glass"></i>' +
          '<input type="search" class="input w-full text-sm" placeholder="Search ' + list.choices.length +
          '" aria-label="Search ' + esc(label) + '" autocomplete="off" data-q></div>' +
          '<div class="ag-list" role="radiogroup" aria-label="' + esc(label) + '" data-list>' + rows('') + '</div></div>';
      } else {
        html += '<p class="ag-note" style="margin:0 0 6px">' + esc(sys) + ' doesn’t list these yet, type it in.</p>';
      }
      html += '<div data-ownpane' + (has ? ' hidden' : '') + '><input type="text" class="input w-full text-sm" ' +
        'placeholder="Type your own" aria-label="Your own ' + esc(label) + '" maxlength="200" data-own value="' +
        esc(inList(current) ? '' : current) + '"></div>';
      if (has) {
        html += '<div class="ag-mine"><button type="button" class="ag-link" data-mine>Not listed? Type your own</button></div>';
      }
      html += '<div class="ag-hint" role="alert" data-err></div>' +
        '<div class="ag-foot"><span class="ag-sp"></span>' +
        '<button type="button" class="btn-secondary btn-sm" data-x>Cancel</button>' +
        '<button type="button" class="btn-primary btn-sm" data-save disabled>' + (willSave ? 'Save' : 'Use this') + '</button></div></div>';
      box.innerHTML = html;
      wire();
      if (has && !inList(current) && current) toggleOwn(true);
      update();
      fold(wrap, true);
      var first = box.querySelector('[data-q]') || box.querySelector('[data-own]');
      if (first && first.focus) first.focus();
    }

    function inList(v) {
      return !!v && list.choices.some(function (c) { return c.name === v; });
    }

    function chosen() {
      var own = box.querySelector('[data-ownpane]');
      if (own && !own.hidden) return box.querySelector('[data-own]').value.trim();
      var r = box.querySelector('input[type=radio]:checked');
      return r ? r.value : '';
    }

    function update() {
      var save = box.querySelector('[data-save]');
      if (!save) return;
      var v = chosen();
      save.disabled = !v || v === current;
    }

    function toggleOwn(toOwn) {
      var own = box.querySelector('[data-ownpane]');
      var pick = box.querySelector('[data-pickpane]');
      var btn = box.querySelector('[data-mine]');
      if (!own || !pick) return;
      own.hidden = !toOwn;
      pick.hidden = toOwn;
      if (btn) btn.textContent = toOwn ? 'Pick from ' + (list.systemName || 'the list') + ' instead' : 'Not listed? Type your own';
      if (toOwn) box.querySelector('[data-own]').focus();
      update();
    }

    function commit() {
      var v = chosen();
      if (!v || v === current) return;
      var save = box.querySelector('[data-save]');
      var err = box.querySelector('[data-err]');
      if (!willSave) { close({ value: v }); return; }
      save.disabled = true;
      save.textContent = 'Saving…';
      saveField(opts, v).then(function () {
        toast('Saved ' + label + ': ' + v);
        close({ value: v });
      }).catch(function () {
        save.disabled = false;
        save.textContent = 'Save';
        if (err) err.innerHTML = '<i class="fa-solid fa-triangle-exclamation"></i> Could not save. Try again.';
      });
    }

    function wire() {
      box.onclick = function (e) {
        var t = e.target.closest('button');
        if (!t) return;
        if (t.hasAttribute('data-x')) close(null);
        else if (t.hasAttribute('data-mine')) toggleOwn(box.querySelector('[data-ownpane]').hidden);
        else if (t.hasAttribute('data-save')) commit();
        else if (t.hasAttribute('data-retry')) load();
        else if (t.hasAttribute('data-own-only')) { list = { systemName: '', choices: [] }; renderBody(); }
      };
      box.onchange = function (e) { if (e.target.type === 'radio') update(); };
      box.oninput = function (e) {
        if (e.target.hasAttribute('data-own')) { update(); return; }
        if (!e.target.hasAttribute('data-q')) return;
        var keep = box.querySelector('input[type=radio]:checked');
        var keepVal = keep ? keep.value : current;
        var q = e.target.value.trim().toLowerCase();
        box.querySelector('[data-list]').innerHTML = rows(q);
        var again = Array.prototype.filter.call(box.querySelectorAll('input[type=radio]'),
          function (r) { return r.value === keepVal; })[0];
        if (again) again.checked = true;
        update();
      };
      box.onkeydown = function (e) {
        if (e.key === 'Escape') { e.stopPropagation(); close(null); return; }
        if (e.key === 'Enter' && (e.target.hasAttribute('data-q') || e.target.hasAttribute('data-own'))) {
          e.preventDefault();
          commit();
        } else if (e.key === 'ArrowDown' && e.target.hasAttribute('data-q')) {
          var r = box.querySelector('input[type=radio]');
          if (r) { e.preventDefault(); r.focus(); }
        }
      };
    }

    function load() {
      if (opts.choices) { list = opts.choices; renderBody(); return; }
      renderLoading();
      Chronicle.fetchChoices(opts.campaignId, opts.fieldKey).then(function (l) {
        if (closed) return;
        list = l;
        renderBody();
      }).catch(function () {
        if (!closed) renderError();
      });
    }

    load();
    return promise;
  };
})();
