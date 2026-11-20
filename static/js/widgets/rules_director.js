// rules_director.js -- the Director's amber words on a rules entry's page and
// on "Before you play in <campaign>" (internal/systems/rules_page.templ,
// rules_before.templ).
//
// Each word either acts at once (Bring back, Seen) or opens one of the small
// dialogs the server drew beside it, on the shared dialog move (dlg-modal).
// The server decides who sees the words and refuses every write from anyone
// but a Director; this only sends the request and shows what happened. A
// change that alters the page reloads it, so what the Director then reads is
// what the server says is true. Text reaches the page only through
// textContent. Started by rules_page.js for each rules page.
(function () {
  'use strict';

  var SAID = 'rp-director-said';

  function api(url, method, body) {
    var opts = { method: method };
    if (body) opts.body = body;
    var call = window.Chronicle && Chronicle.apiFetch ? Chronicle.apiFetch(url, opts) : fetch(url, {
      method: method, credentials: 'same-origin',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: body ? JSON.stringify(body) : undefined,
    });
    return call.then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (b) { return { ok: r.ok, status: r.status, body: b || {} }; });
    });
  }

  // messageOf turns a refusal into words that say how to fix it.
  function messageOf(res, fallback) {
    var b = (res && res.body) || {};
    return b.message || b.error || fallback;
  }

  function working(btn, on) {
    if (!btn) return;
    if (window.Chronicle && Chronicle.working) Chronicle.working(btn, on);
    else btn.disabled = !!on;
  }

  // reloadWith reloads the page and says what changed once it is back, so
  // the message is read on the page that shows the change.
  function reloadWith(msg, url) {
    try { if (msg) sessionStorage.setItem(SAID, msg); } catch (e) { /* the page still shows the change */ }
    if (url && url !== window.location.pathname + window.location.search) window.location.assign(url);
    else window.location.reload();
  }

  function sayStored() {
    var msg = null;
    try { msg = sessionStorage.getItem(SAID); sessionStorage.removeItem(SAID); } catch (e) { msg = null; }
    if (msg && window.Chronicle && Chronicle.notify) Chronicle.notify(msg, 'success');
  }

  // editorURL opens the entry editor on one entry; made says it was just
  // made, so the editor says who sees it yet; back is the page to return to.
  // ownPage is where the Director lands after making or tying a rewrite:
  // the campaign's entry at its own slug, never the printed page they left.
  function ownPage(page, slug) {
    if (!page) return '';
    var u = new URL(page, window.location.origin);
    u.searchParams.delete('printed');
    if (slug) u.pathname = u.pathname.replace(/[^/]+$/, encodeURIComponent(slug));
    return u.pathname + u.search;
  }

  function editorURL(base, id, made, back) {
    return base + '&entry=' + encodeURIComponent(id) + (made ? '&made=1' : '') + (back ? '&return=' + encodeURIComponent(back) : '');
  }

  function Director(root) {
    this.root = root;
    this.opener = null;
  }

  Director.prototype.tools = function (from) {
    return (from && from.closest('[data-rp-director]')) || this.root.querySelector('[data-rp-director]');
  };

  Director.prototype.fail = function (where, msg) {
    var err = where && (where.querySelector('[data-rp-err]') || where.querySelector('[data-rp-msg]'));
    if (!err) { if (window.Chronicle && Chronicle.notify) Chronicle.notify(msg, 'error'); return; }
    err.textContent = msg;
    err.hidden = false;
  };

  Director.prototype.open = function (id, from) {
    var dlg = document.getElementById(id);
    if (!dlg || !dlg.showModal) return;
    this.opener = from || null;
    dlg.querySelectorAll('[data-rp-err]').forEach(function (e) { e.hidden = true; e.textContent = ''; });
    if (!dlg.open) dlg.showModal();
    var first = dlg.querySelector('textarea, input:not([type="hidden"])') || dlg.querySelector('.btn-primary');
    if (first && !first.closest('[hidden]')) first.focus();
  };

  Director.prototype.close = function (dlg) {
    if (dlg && dlg.open) dlg.close();
  };

  Director.prototype.onClose = function () {
    if (this.opener && document.body.contains(this.opener)) this.opener.focus();
    this.opener = null;
  };

  Director.prototype.act = function (btn) {
    var self = this, act = btn.getAttribute('data-rp-act');
    var dlg = btn.closest('dialog');
    var t = this.tools(btn);
    var base = t ? t.getAttribute('data-api') : '';
    var field = t ? t.getAttribute('data-field') : '';
    var slug = t ? t.getAttribute('data-slug') : '';
    var entry = t ? t.getAttribute('data-entry') : '';
    var editor = t ? t.getAttribute('data-editor-base') : '';
    var where = dlg || t;
    var rewrite = function (name) {
      var body = { fieldKey: field, slug: slug };
      if (name) body.name = name;
      working(btn, true);
      api(base + '/rewrite', 'POST', body).then(function (res) {
        if (!res.ok || !res.body.entry) { working(btn, false); self.fail(where, messageOf(res, 'The copy could not be made. Try again.')); return; }
        window.location.assign(editorURL(editor, res.body.entry.id, true, ownPage(t.getAttribute('data-page'), res.body.entry.slug)));
      }).catch(function () { working(btn, false); self.fail(where, 'The copy could not be made. Check your connection and try again.'); });
    };
    switch (act) {
      case 'open': this.open(btn.getAttribute('data-dialog'), btn); break;
      case 'close': this.close(dlg); break;
      case 'rewrite':
        if (document.getElementById('rp-dlg-rewrite')) this.open('rp-dlg-rewrite', btn);
        else rewrite('');
        break;
      case 'fresh': {
        var box = dlg.querySelector('.rp-dlg__fresh');
        box.hidden = !box.hidden;
        btn.setAttribute('aria-expanded', String(!box.hidden));
        if (!box.hidden) box.querySelector('[data-rp-name]').focus();
        break;
      }
      case 'rewrite-named': {
        var name = (dlg.querySelector('[data-rp-name]').value || '').trim();
        if (!name) { this.fail(dlg, 'Give the copy a name first. It cannot be called what your own entry is called.'); return; }
        rewrite(name);
        break;
      }
      case 'tie': {
        var id = btn.getAttribute('data-tie');
        working(btn, true);
        api(base + '/' + encodeURIComponent(id) + '/tie', 'POST', { slug: slug }).then(function (res) {
          if (!res.ok) { working(btn, false); self.fail(dlg, messageOf(res, 'That did not work. Try again.')); return; }
          window.location.assign(editorURL(editor, id, false, ownPage(t.getAttribute('data-page'), slug)));
        }).catch(function () { working(btn, false); self.fail(dlg, 'That did not work. Check your connection and try again.'); });
        break;
      }
      case 'leaveout': {
        var sentence = (dlg.querySelector('[data-rp-sentence]').value || '').trim();
        working(btn, true);
        api(base + '/left-out', 'PUT', { fieldKey: field, slug: slug, sentence: sentence }).then(function (res) {
          if (!res.ok) { working(btn, false); self.fail(dlg, messageOf(res, 'That did not save. Try again.')); return; }
          // A new leave-out says what it does; a changed sentence says that.
          var already = t.getAttribute('data-kind') === 'left_out';
          reloadWith(already ? 'Saved. Players read the new line now.' : 'Left out. The hero creator no longer offers it.', t.getAttribute('data-page'));
        }).catch(function () { working(btn, false); self.fail(dlg, 'That did not save. Check your connection and try again.'); });
        break;
      }
      case 'bringback':
        working(btn, true);
        api(base + '/left-out/' + encodeURIComponent(field) + '/' + encodeURIComponent(slug), 'DELETE').then(function (res) {
          if (!res.ok) { working(btn, false); self.fail(t, messageOf(res, 'That did not work. Try again.')); return; }
          reloadWith('Brought back. The hero creator offers it again.', t.getAttribute('data-page'));
        }).catch(function () { working(btn, false); self.fail(t, 'That did not work. Check your connection and try again.'); });
        break;
      case 'aside':
        working(btn, true);
        api(base + '/' + encodeURIComponent(entry), 'PUT', { visibility: 'directors' }).then(function (res) {
          if (!res.ok) { working(btn, false); self.fail(dlg, messageOf(res, 'That did not save. Try again.')); return; }
          reloadWith('Set aside. Players read the printed one again.', t.getAttribute('data-page'));
        }).catch(function () { working(btn, false); self.fail(dlg, 'That did not save. Check your connection and try again.'); });
        break;
      case 'menu': this.menu(btn, btn.getAttribute('aria-expanded') !== 'true'); break;
      case 'free': {
        var row = btn.closest('[data-rp-change]');
        working(btn, true);
        api(base + '/old-names/' + encodeURIComponent(btn.getAttribute('data-name-id')), 'DELETE').then(function (res) {
          if (!res.ok) { working(btn, false); self.fail(row, messageOf(res, 'That did not save. Try again.')); return; }
          if (window.Chronicle && Chronicle.notify) Chronicle.notify('Freed. Another entry can take the name now.', 'success');
          self.fold(row);
        }).catch(function () { working(btn, false); self.fail(row, 'That did not save. Check your connection and try again.'); });
        break;
      }
      case 'seen': {
        var line = btn.closest('[data-rp-change]');
        working(btn, true);
        api(base + '/' + encodeURIComponent(btn.getAttribute('data-entry')) + '/seen', 'POST').then(function (res) {
          if (!res.ok) { working(btn, false); self.fail(line, messageOf(res, 'That did not save. Try again.')); return; }
          self.fold(line);
        }).catch(function () { working(btn, false); self.fail(line, 'That did not save. Check your connection and try again.'); });
        break;
      }
    }
  };

  // menu opens or closes the "New entry" menu, growing out of its word.
  Director.prototype.menu = function (btn, open) {
    var m = document.getElementById(btn.getAttribute('aria-controls'));
    if (!m) return;
    btn.setAttribute('aria-expanded', String(open));
    this.openMenu = open ? btn : null;
    if (open) {
      m.hidden = false;
      requestAnimationFrame(function () { m.setAttribute('data-open', ''); });
      var first = m.querySelector('[role="menuitem"]');
      if (first) first.focus();
      return;
    }
    m.removeAttribute('data-open');
    var calm = document.documentElement.getAttribute('data-motion') === 'off';
    setTimeout(function () { if (btn.getAttribute('aria-expanded') !== 'true') m.hidden = true; }, calm ? 0 : 120);
  };

  // fold takes a line away: it fades and folds closed (the standard's
  // "removing from a list"); Calm and Off let it go at once.
  Director.prototype.fold = function (el) {
    if (!el) return;
    var row = el.closest('li');
    var next = row && row.querySelector('a');
    var calm = document.documentElement.hasAttribute('data-motion');
    if (calm || !el.animate) { el.remove(); if (next) next.focus(); return; }
    var h = el.offsetHeight;
    var css = getComputedStyle(document.documentElement);
    var dur = parseFloat(css.getPropertyValue('--dur-micro')) || 120;
    var ease = css.getPropertyValue('--ease-in').trim() || 'ease-in';
    var a = el.animate([{ opacity: 1, height: h + 'px' }, { opacity: 0, height: '0px', marginTop: '0px', paddingTop: '0px', paddingBottom: '0px' }],
      { duration: dur + 40, easing: ease });
    el.style.overflow = 'hidden';
    a.onfinish = function () { el.remove(); if (next) next.focus(); };
  };

  Director.prototype.wire = function () {
    var self = this;
    this.onClick = function (e) {
      // A click off the open menu closes it, losing nothing.
      if (self.openMenu && !e.target.closest('.rp-newmenu')) self.menu(self.openMenu, false);
      var btn = e.target.closest('[data-rp-act]');
      if (!btn || !self.root.contains(btn)) return;
      e.preventDefault();
      self.act(btn);
    };
    this.onDlgClose = function (e) { if (e.target.matches && e.target.matches('dialog.rp-dlg')) self.onClose(); };
    this.onKey = function (e) {
      if (self.openMenu && e.key === 'Escape') { var b = self.openMenu; self.menu(b, false); b.focus(); return; }
      if (self.openMenu && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
        var items = Array.prototype.slice.call(document.querySelectorAll('#' + self.openMenu.getAttribute('aria-controls') + ' [role="menuitem"]'));
        var at = items.indexOf(document.activeElement);
        var next = items[(at + (e.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length];
        if (next) { e.preventDefault(); next.focus(); }
        return;
      }
      // Enter in a one-line field answers the dialog's main question.
      if (e.key !== 'Enter' || e.target.tagName !== 'INPUT') return;
      var dlg = e.target.closest('dialog.rp-dlg');
      var go = dlg && (e.target.closest('.rp-dlg__fresh') || dlg).querySelector('.btn-primary:not([data-rp-act="tie"])');
      if (go) { e.preventDefault(); self.act(go); }
    };
    this.onDocClick = function (e) { if (self.openMenu && !self.root.contains(e.target)) self.menu(self.openMenu, false); };
    document.addEventListener('click', this.onDocClick);
    this.root.addEventListener('click', this.onClick);
    this.root.addEventListener('close', this.onDlgClose, true);
    this.root.addEventListener('keydown', this.onKey);
    // "Set aside" on the gathered page opens this page with the question.
    if (window.location.hash === '#set-aside' && document.getElementById('rp-dlg-aside')) {
      this.open('rp-dlg-aside', this.root.querySelector('[data-dialog="rp-dlg-aside"]'));
    }
    sayStored();
  };

  Director.prototype.unwire = function () {
    document.removeEventListener('click', this.onDocClick);
    this.root.removeEventListener('click', this.onClick);
    this.root.removeEventListener('close', this.onDlgClose, true);
    this.root.removeEventListener('keydown', this.onKey);
  };

  window.ChronicleRulesDirector = {
    init: function (el) {
      if (!el.querySelector('[data-rp-director], [data-rp-act]')) return null;
      var d = new Director(el);
      d.wire();
      return d;
    },
    editorURL: editorURL,
  };
})();
