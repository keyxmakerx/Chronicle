/**
 * settings_board.js -- the controls on Campaign settings that save where they
 * stand: the Overview's two flips and feature chips, and the Campaign tab's
 * two-way choices and its name and description fields.
 *
 *   flip    a tile that flips between two options (Private and Public,
 *           Hidden and Shown); a press saves at once through the same route
 *           the Campaign tab's choice uses
 *   chip    a feature switched on or off through the add-ons store's toggle
 *   choice  one card of a two-way choice; a press saves at once
 *   input   a name or description being typed: the banner and the card on
 *           the Campaign tab follow it
 *   save    leaving that field saves it; Escape puts the stored words back
 *
 * Each control carries its own inline handler (built in
 * campaigns/desk_view.go) that loads this file on first use and calls
 * Chronicle.settingsBoard.<method>(control, event), because the page arrives
 * by a boosted swap where a script tag or a delegated listener may never run.
 * Nothing here listens on document.
 *
 * Feedback comes first: the new state shows inside a tenth of a second, the
 * control says it is working, and a failed save puts the old state back with
 * a message saying nothing changed. A saved control is lit for about a second
 * (the Glow) and "Saved" is said for the screen reader. Motion is CSS only, so
 * html[data-motion] (Calm, Off) reaches it there.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});

  function options(el) {
    try { return JSON.parse(el.getAttribute('data-options') || '[]'); } catch (e) { return []; }
  }

  function host(el) { return el.closest('[data-desk]') || document; }

  // The words the server sends back when a write fails, or a plain one.
  function send(url, method, body) {
    if (C.apiAction) return C.apiAction(url, { method: method, body: body });
    return C.apiFetch(url, { method: method, body: body }).then(function (r) {
      if (!r.ok) throw new Error('request failed (' + r.status + ')');
      return {};
    });
  }

  function bodyFor(el, value) {
    var b = {};
    b[el.getAttribute('data-field')] = el.hasAttribute('data-bool') ? value === 'true' : value;
    return b;
  }

  function fail(err) {
    var msg = (err && err.message) || '';
    if (C.notify) C.notify((msg ? msg + ' ' : '') + 'Nothing changed.', 'error');
  }

  function busy(el, on) {
    if (on) el.setAttribute('aria-busy', 'true');
    else el.removeAttribute('aria-busy');
  }

  // "Saved", for the screen reader, from the page's one status line.
  function say(el, words) {
    var s = host(el).querySelector('[data-sb-status]');
    if (!s) return;
    s.textContent = '';
    setTimeout(function () { s.textContent = words; }, 30);
  }

  // The Glow: what the owner just changed, lit for about a second.
  function glow(el) {
    if (!el || document.documentElement.getAttribute('data-motion') === 'off') return;
    el.classList.remove('is-glow');
    void el.offsetWidth;
    el.classList.add('is-glow');
    setTimeout(function () { el.classList.remove('is-glow'); }, 1300);
  }

  // A visible "Saved" beside a Campaign tab setting, which settles in and
  // fades (manage_desk.css draws it).
  var CHECK = '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" focusable="false"><path d="M3 8.5l3.2 3L13 4.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';
  function tick(row) {
    var acts = row && row.querySelector('.desk-acts');
    if (!acts) return;
    acts.innerHTML = '';
    var s = document.createElement('span');
    s.className = 'desk-saved';
    s.innerHTML = CHECK + '<span>Saved</span>';
    acts.appendChild(s);
    setTimeout(function () { if (s.isConnected) s.remove(); }, 1900);
  }

  // Who can see the campaign shows in the banner and on the Campaign tab's
  // card, and the guest preview opens only while it is public.
  function syncAccess(el, value) {
    if (el.getAttribute('data-field') !== 'isPublic') return;
    var pub = value === 'true';
    [].forEach.call(document.querySelectorAll('[data-settings-access]'), function (p) {
      p.textContent = pub ? 'Public' : 'Private';
    });
    [].forEach.call(host(el).querySelectorAll('[data-needs-public]'), function (g) {
      g.setAttribute('aria-disabled', String(!pub));
      if (pub) g.removeAttribute('title');
      else g.setAttribute('title', g.getAttribute('data-off') || '');
      var why = host(el).querySelector('#why-preview');
      if (why) why.hidden = pub;
    });
  }

  // ---- flip ---------------------------------------------------------------

  function showFlip(btn, opt, on) {
    btn.setAttribute('aria-pressed', String(on));
    btn.setAttribute('data-value', opt.v);
    var w = btn.querySelector('[data-sb-word]');
    var h = btn.querySelector('[data-sb-hint]');
    if (w) w.textContent = opt.l;
    if (h) h.textContent = opt.h || '';
    var label = (btn.querySelector('.sb-tl') || {}).textContent || '';
    btn.setAttribute('aria-label', label + ': ' + opt.l);
  }

  function flip(btn) {
    if (!btn || btn.getAttribute('aria-disabled') === 'true' || btn.getAttribute('aria-busy') === 'true') return;
    var opts = options(btn);
    if (opts.length !== 2) return;
    var on = btn.getAttribute('aria-pressed') === 'true';
    var was = opts[on ? 1 : 0], next = opts[on ? 0 : 1];
    showFlip(btn, next, !on);
    busy(btn, true);
    send(btn.getAttribute('data-url'), btn.getAttribute('data-method') || 'PUT', bodyFor(btn, next.v)).then(function () {
      busy(btn, false);
      syncAccess(btn, next.v);
      glow(btn);
      say(btn, (btn.querySelector('.sb-tl') || {}).textContent + ' saved: ' + next.l);
    }, function (err) {
      busy(btn, false);
      showFlip(btn, was, on);
      fail(err);
    });
  }

  // ---- chip ---------------------------------------------------------------

  // The count beside Features follows the chips: "12 of 15 on".
  function recount(tile) {
    var c = tile && tile.querySelector('[data-sb-count]');
    if (!c) return;
    var chips = tile.querySelectorAll('[data-sb-chip]');
    var on = tile.querySelectorAll('[data-sb-chip][aria-pressed="true"]').length;
    c.textContent = on + ' of ' + chips.length + ' on';
  }

  function chip(btn) {
    if (!btn || btn.getAttribute('aria-disabled') === 'true' || btn.getAttribute('aria-busy') === 'true') return;
    var tile = btn.closest('[data-sb-features]');
    var campaign = tile && tile.getAttribute('data-campaign');
    if (!campaign) return;
    var on = btn.getAttribute('aria-pressed') === 'true';
    btn.setAttribute('aria-pressed', String(!on));
    recount(tile);
    busy(btn, true);
    var form = new FormData();
    form.append('action', on ? 'disable' : 'enable');
    form.append('redirect_to', 'extensions-hub');
    var url = '/campaigns/' + encodeURIComponent(campaign) + '/addons/' + encodeURIComponent(btn.getAttribute('data-sb-chip')) + '/toggle';
    // The toggle answers an htmx caller with no body instead of a redirect.
    C.apiFetch(url, { method: 'PUT', body: form, headers: { 'HX-Request': 'true' } }).then(function (r) {
      if (!r.ok) {
        return r.text().then(function (t) {
          var j = {};
          try { j = t ? JSON.parse(t) : {}; } catch (e) { j = {}; }
          throw new Error((j && (j.message || j.error)) || 'request failed (' + r.status + ')');
        });
      }
      busy(btn, false);
      glow(btn);
      say(btn, btn.textContent + (on ? ' turned off' : ' turned on'));
    }).catch(function (err) {
      busy(btn, false);
      btn.setAttribute('aria-pressed', String(on));
      recount(tile);
      fail(err);
    });
  }

  // ---- choice -------------------------------------------------------------

  function pressOnly(group, btn) {
    [].forEach.call(group.querySelectorAll('[data-v]'), function (b) {
      b.setAttribute('aria-pressed', String(b === btn));
    });
  }

  function choice(btn) {
    var group = btn && btn.closest('[data-sb-choice]');
    if (!group || btn.getAttribute('aria-disabled') === 'true' || group.getAttribute('aria-busy') === 'true') return;
    if (btn.getAttribute('aria-pressed') === 'true') return;
    var was = group.querySelector('[data-v][aria-pressed="true"]');
    var value = btn.getAttribute('data-v');
    pressOnly(group, btn);
    busy(group, true);
    send(group.getAttribute('data-url'), group.getAttribute('data-method') || 'PUT', bodyFor(group, value)).then(function () {
      busy(group, false);
      syncAccess(group, value);
      var row = group.closest('[data-line]');
      tick(row);
      glow(row);
      say(group, (group.getAttribute('data-label') || '') + ' saved: ' + (btn.querySelector('b') || btn).textContent);
    }, function (err) {
      busy(group, false);
      if (was) pressOnly(group, was);
      fail(err);
    });
  }

  // ---- the name and the description ----------------------------------------

  // What the field is typed into shows at once wherever it is shown: the
  // banner's name and the card on the Campaign tab.
  function input(el) {
    var key = el.getAttribute('data-sb-src');
    var v = el.value;
    if (key === 'name') {
      [].forEach.call(document.querySelectorAll('[data-settings-name]'), function (n) { n.textContent = v.trim() || el.getAttribute('data-stored') || ''; });
    }
    [].forEach.call(host(el).querySelectorAll('[data-sb-bind="' + key + '"]'), function (n) { n.textContent = v; });
    var left = el.parentNode && el.parentNode.querySelector('[data-sb-left]');
    var max = parseInt(el.getAttribute('maxlength'), 10);
    if (left && max) left.textContent = (max - v.length) + ' left';
  }

  function keydown(el, e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      el.value = el.getAttribute('data-stored') || '';
      input(el);
      el.blur();
    } else if (e.key === 'Enter' && (el.tagName !== 'TEXTAREA' || e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      el.blur();
    }
  }

  function save(el) {
    if (!el || el.disabled) return;
    var stored = el.getAttribute('data-stored') || '';
    var v = el.value.trim();
    if (v === stored) { input(el); return; }
    if (el.getAttribute('data-field') === 'name' && !v) {
      el.value = stored;
      input(el);
      if (C.notify) C.notify('A campaign needs a name.', 'error');
      return;
    }
    busy(el, true);
    send(el.getAttribute('data-url'), el.getAttribute('data-method') || 'PUT', bodyFor(el, v)).then(function (res) {
      busy(el, false);
      var key = el.getAttribute('data-field');
      var kept = res && typeof res[key] === 'string' ? res[key] : v;
      el.setAttribute('data-stored', kept);
      if (document.activeElement !== el) el.value = kept;
      input(el);
      var row = el.closest('[data-line]');
      tick(row);
      glow(row);
      say(el, (el.getAttribute('aria-label') || '') + ' saved');
    }, function (err) {
      busy(el, false);
      el.value = stored;
      input(el);
      fail(err);
    });
  }

  C.settingsBoard = {
    flip: flip,
    chip: chip,
    choice: choice,
    input: input,
    keydown: keydown,
    save: save
  };
})();
