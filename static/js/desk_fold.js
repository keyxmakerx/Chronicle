/**
 * desk_fold.js -- a desk line unfolds where it stands into its sub menu.
 *
 * A line that has one carries a chevron button (aria-expanded, aria-controls)
 * and an empty region after its row. The first time it opens, the region's
 * content is fetched as a fragment from the owner-only route in its data-url,
 * so the desk itself stays quick. More than one line may be open at once.
 *
 *   Enter or Space on the chevron toggles; so does pressing the row itself
 *   with a pointer (never on a link, a dashed word, a button or a field).
 *   Escape closes the fold the focus is in and returns focus to the chevron.
 *   A keyboard open moves focus to the first control inside, so a person can
 *   act at once; a pointer open leaves focus alone.
 *
 * Inside a fold, every action calls a route that already exists. When one
 * works, the line's own sentence is fetched again and swapped where it stands
 * (it settles in, and the line glows), so the number never lags the page.
 *
 * Motion reads html[data-motion]. Full: the region grows to its measured
 * content height with a fade (--dur-standard, --ease-out) and closes quicker
 * (--dur-micro + 40ms, --ease-in). Calm: it opens and closes at once, only
 * the sentence's swap fades. Off: nothing moves. An open that is interrupted
 * by a close (or the reverse) always ends in the state last asked for.
 *
 * Each control carries its own inline handler built Go-side
 * (campaigns/desk_view.go), because the desk and its fragments arrive by
 * boosted swaps and innerHTML, where a script tag or a delegated listener may
 * never run.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});

  function level() {
    var m = document.documentElement.getAttribute('data-motion');
    return m === 'off' ? 'off' : m === 'calm' ? 'calm' : 'full';
  }
  function token(name, fallback) {
    try {
      var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
      return v || fallback;
    } catch (e) { return fallback; }
  }
  function ms(name, fallback) {
    var v = parseFloat(token(name, ''));
    return isFinite(v) && v >= 0 ? v : fallback;
  }

  // Runs one animation and settles when it ends or is cancelled, so callers
  // can always continue and the element never keeps a held-over style.
  function run(el, frames, opts) {
    if (!el.animate) return { done: Promise.resolve(), anim: null };
    var a;
    try { a = el.animate(frames, opts); } catch (e) { return { done: Promise.resolve(), anim: null }; }
    return { anim: a, done: a.finished.then(function () {}, function () {}) };
  }

  function chevron(line) { return line.querySelector(':scope > .desk-row .desk-chev'); }
  function regionOf(line) { return line.querySelector(':scope > .desk-fold'); }
  function stateOf(line) {
    return line._deskFold || (line._deskFold = { want: false, loaded: false, loading: null, anim: null });
  }

  function setExpanded(line, on) {
    var b = chevron(line);
    if (!b) return;
    b.setAttribute('aria-expanded', String(on));
    b.setAttribute('aria-label', (on ? 'Fold ' : 'Unfold ') + (b.getAttribute('data-title') || ''));
  }

  function stopAnim(st) {
    if (st.anim) { try { st.anim.cancel(); } catch (e) { /* gone */ } st.anim = null; }
  }

  // ---- fetching ----------------------------------------------------------

  function get(url) {
    var f = C.apiFetch || function (u) { return fetch(u, { credentials: 'same-origin' }); };
    return f(url, { headers: { Accept: 'text/html' } }).then(function (r) {
      if (!r.ok) throw new Error('request failed (' + r.status + ')');
      return r.text();
    });
  }

  function failedHTML(line) {
    var a = line.querySelector('.desk-acts a');
    var p = document.createElement('div');
    p.className = 'desk-fold-in';
    var s = document.createElement('p');
    s.className = 'desk-note';
    s.textContent = 'This could not be opened right now.';
    p.appendChild(s);
    if (a) {
      var l = document.createElement('a');
      l.className = 'desk-q';
      l.href = a.getAttribute('href');
      l.textContent = a.textContent;
      p.appendChild(l);
    }
    return p.outerHTML;
  }

  function load(line) {
    var st = stateOf(line), region = regionOf(line);
    if (st.loaded) return Promise.resolve();
    if (st.loading) return st.loading;
    var b = chevron(line);
    if (b) b.setAttribute('aria-busy', 'true');
    st.loading = get(region.getAttribute('data-url')).then(function (html) {
      region.innerHTML = html;
    }, function () {
      region.innerHTML = failedHTML(line);
    }).then(function () {
      st.loaded = true; st.loading = null;
      if (b) b.removeAttribute('aria-busy');
    });
    return st.loading;
  }

  // ---- open and close ----------------------------------------------------

  function reveal(line) {
    var st = stateOf(line), region = regionOf(line), lv = level();
    var from = 0, fromOp = 0;
    if (st.anim) {
      from = region.offsetHeight;
      fromOp = parseFloat(getComputedStyle(region).opacity) || 0;
    }
    stopAnim(st);
    region.hidden = false;
    region.style.overflow = '';
    if (lv !== 'full') return;
    var h = region.scrollHeight;
    if (h <= 0 || h === from) return;
    region.style.overflow = 'hidden';
    var r = run(region, [{ height: from + 'px', opacity: fromOp }, { height: h + 'px', opacity: 1 }],
      { duration: ms('--dur-standard', 200), easing: token('--ease-out', 'ease-out') });
    st.anim = r.anim;
    r.done.then(function () {
      if (st.anim !== r.anim) return;
      st.anim = null;
      region.style.overflow = '';
    });
  }

  function conceal(line) {
    var st = stateOf(line), region = regionOf(line), lv = level();
    var from = region.offsetHeight, fromOp = parseFloat(getComputedStyle(region).opacity);
    if (!isFinite(fromOp)) fromOp = 1;
    stopAnim(st);
    var end = function () { region.hidden = true; region.style.overflow = ''; };
    if (lv !== 'full' || region.hidden || from <= 0) { end(); return; }
    region.style.overflow = 'hidden';
    var r = run(region, [{ height: from + 'px', opacity: fromOp }, { height: '0px', opacity: 0 }],
      { duration: ms('--dur-micro', 120) + 40, easing: token('--ease-in', 'ease-in'), fill: 'forwards' });
    st.anim = r.anim;
    r.done.then(function () {
      if (st.anim !== r.anim) return;
      st.anim = null;
      end();
      // The held end frame (fill: forwards) is dropped with the element
      // hidden, so a later open never starts from a collapsed height.
      try { r.anim.cancel(); } catch (e) { /* gone */ }
    });
  }

  function focusFirst(line) {
    var region = regionOf(line);
    var f = region && region.querySelector('input,.desk-b,button,a[href]');
    if (f && f.focus) f.focus();
  }

  function setOpen(line, want, byKey) {
    var st = stateOf(line);
    st.want = want;
    setExpanded(line, want);
    if (!want) {
      var region = regionOf(line), back = region.contains(document.activeElement);
      conceal(line);
      if (back) { var b = chevron(line); if (b) b.focus({ preventScroll: true }); }
      return;
    }
    load(line).then(function () {
      if (!st.want) return;
      reveal(line);
      if (byKey) focusFirst(line);
      else {
        var region = regionOf(line);
        if (region.scrollIntoView) region.scrollIntoView({ block: 'nearest', behavior: level() === 'full' ? 'smooth' : 'auto' });
      }
    });
  }

  function toggle(btn, ev) {
    var line = btn && btn.closest('.desk-ln');
    if (!line) return;
    setOpen(line, !stateOf(line).want, !!ev && ev.detail === 0);
  }

  // A pointer press on the row itself toggles; anything interactive inside it
  // keeps its own meaning.
  function row(rowEl, ev) {
    if (!ev || !ev.target || !ev.target.closest) return;
    if (ev.target.closest('a,button,input,select,textarea,[role=button],[contenteditable]')) return;
    if (window.getSelection && String(window.getSelection()).length) return;
    toggle(chevron(rowEl.closest('.desk-ln')), { detail: 1 });
  }

  function key(el, ev) {
    if (!ev || ev.key !== 'Escape') return;
    var line = el.closest('.desk-ln');
    if (!line || !stateOf(line).want) return;
    // A menu or a field open inside the fold takes its own Escape first.
    if (ev.defaultPrevented) return;
    ev.preventDefault();
    ev.stopPropagation();
    setOpen(line, false);
    var b = chevron(line);
    if (b) b.focus({ preventScroll: true });
  }

  // ---- after a change ----------------------------------------------------

  function glow(line) {
    if (!line || level() === 'off') return;
    line.classList.remove('is-glow');
    void line.offsetWidth;
    line.classList.add('is-glow');
    setTimeout(function () { line.classList.remove('is-glow'); }, 1300);
  }

  // The line's sentence, worded again by the server and swapped where it
  // stands: it settles in (a fade and a short rise; Calm only fades).
  function refreshSentence(line) {
    var region = regionOf(line);
    if (!region) return Promise.resolve();
    var url = region.getAttribute('data-url').replace(/\/fold$/, '/sentence');
    return get(url).then(function (html) {
      var t = document.createElement('template');
      t.innerHTML = html.trim();
      var fresh = t.content.firstElementChild, old = line.querySelector(':scope > .desk-row > .desk-s');
      if (!fresh || !old) return;
      old.replaceWith(fresh);
      var lv = level();
      if (lv !== 'off') {
        var from = lv === 'full' ? { opacity: 0, transform: 'translateY(6px)' } : { opacity: 0 };
        var to = lv === 'full' ? { opacity: 1, transform: 'none' } : { opacity: 1 };
        run(fresh, [from, to], { duration: ms('--dur-large', 280), easing: token('--ease-out', 'ease-out') });
      }
      glow(line);
    }, function () { /* the sentence stays as it was; the next load words it */ });
  }

  // The fold's own content, fetched again after a row left, with its height
  // easing to the new content.
  function refreshFold(line) {
    var st = stateOf(line), region = regionOf(line);
    return get(region.getAttribute('data-url')).then(function (html) {
      if (!st.want) { st.loaded = false; region.innerHTML = ''; return; }
      var before = region.offsetHeight;
      region.innerHTML = html;
      var after = region.offsetHeight;
      if (level() !== 'full' || before === after) return;
      stopAnim(st);
      region.style.overflow = 'hidden';
      var r = run(region, [{ height: before + 'px' }, { height: after + 'px' }],
        { duration: ms('--dur-standard', 200), easing: token('--ease-out', 'ease-out') });
      st.anim = r.anim;
      r.done.then(function () { if (st.anim === r.anim) { st.anim = null; region.style.overflow = ''; } });
    }, function () { /* keep what is shown */ });
  }

  function say(line, text) {
    var s = line.querySelector('[data-desk-status]');
    if (s) s.textContent = text;
  }

  function fail(err) {
    var msg = (err && err.message) || '';
    if (C.notify) C.notify((msg ? msg + ' ' : '') + 'Nothing changed.', 'error');
  }

  // A row leaves a list: it fades and folds closed (Calm fades, Off is gone).
  function removeRow(rowEl) {
    var lv = level();
    if (lv === 'off') { rowEl.remove(); return Promise.resolve(); }
    var h = rowEl.offsetHeight;
    rowEl.style.overflow = 'hidden';
    var frames = lv === 'full' ? [{ height: h + 'px', opacity: 1 }, { height: '0px', opacity: 0 }] : [{ opacity: 1 }, { opacity: 0 }];
    var r = run(rowEl, frames, { duration: ms('--dur-micro', 120) + 40, easing: token('--ease-in', 'ease-in'), fill: 'forwards' });
    return r.done.then(function () { rowEl.remove(); });
  }

  // One action row: call its existing route; on success the row leaves, the
  // sentence is worded again and the list is read again.
  function act(btn) {
    if (btn.disabled) return;
    var line = btn.closest('.desk-ln'), rowEl = btn.closest('.desk-fr');
    var label = btn.textContent;
    btn.disabled = true;
    btn.textContent = (btn.getAttribute('data-busy') || 'Working') + '…';
    var send = C.apiAction || function (u, o) { return C.apiFetch(u, o); };
    send(btn.getAttribute('data-url'), { method: btn.getAttribute('data-method') || 'POST' }).then(function () {
      var gone = rowEl ? removeRow(rowEl) : Promise.resolve();
      refreshSentence(line);
      return gone.then(function () { return refreshFold(line); });
    }, function (err) {
      btn.disabled = false;
      btn.textContent = label;
      fail(err);
    });
  }

  var EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

  function invite(btn) {
    if (btn.disabled) return;
    var line = btn.closest('.desk-ln'), rowEl = btn.closest('.desk-inv');
    var input = rowEl.querySelector('[data-desk-invite-email]');
    var roleBtn = rowEl.querySelector('.desk-b');
    var email = (input.value || '').trim();
    if (!EMAIL.test(email)) {
      say(line, 'Type their email address first.');
      input.focus();
      return;
    }
    var role = roleBtn ? roleBtn.getAttribute('data-value') : 'player';
    var label = btn.textContent;
    btn.disabled = true;
    btn.textContent = (btn.getAttribute('data-busy') || 'Sending') + '…';
    say(line, '');
    var send = C.apiAction || function (u, o) { return C.apiFetch(u, o); };
    send(btn.getAttribute('data-url'), { method: 'POST', body: { email: email, role: role } }).then(function () {
      btn.disabled = false;
      btn.textContent = label;
      input.value = '';
      say(line, 'Invite sent to ' + email + ' as a ' + role + '.');
      input.focus({ preventScroll: true });
      refreshSentence(line);
    }, function (err) {
      btn.disabled = false;
      btn.textContent = label;
      say(line, ((err && err.message) || 'It did not go.') + ' Nothing was sent.');
      input.focus({ preventScroll: true });
    });
  }

  function inviteKey(input) {
    var b = input.closest('.desk-inv').querySelector('.desk-send');
    if (b) b.click();
  }

  // Called by desk_blank.js after a dashed word inside a fold has saved.
  function changed(blankBtn) {
    var line = blankBtn && blankBtn.closest('.desk-ln');
    if (line && blankBtn.closest('.desk-fold')) refreshSentence(line);
  }

  C.deskFold = {
    toggle: toggle, row: row, key: key, act: act, invite: invite, inviteKey: inviteKey, changed: changed,
    // for tests and the carry move
    refreshSentence: refreshSentence, isOpen: function (line) { return stateOf(line).want; }
  };
})();
