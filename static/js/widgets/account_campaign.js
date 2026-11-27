/**
 * account_campaign.js — the dice, calendar, brush and Store sections of one
 * campaign's page in Account settings (campaigns/account_settings.templ).
 *
 * Mount: <div data-widget="account-campaign" data-campaign-id data-user-id>.
 *
 * The dice choices are the very ones the Table's My dice section keeps, in
 * the same browser storage under the same names (rolls/static/js/table.js),
 * so a choice made here is what the Table uses and the other way round. The
 * campaign's own dice (the table's look) come from GET .../rolls/settings.
 *
 * The calendar link shows only while the owner leaves calendar links on
 * (GET .../sessions/feed-settings), and is made the first time it is asked
 * for (POST .../sessions/feed/token), as on the calendar's game night menu.
 *
 * The Customize brush and Store icon buttons, drawn for the owner only, save
 * through PUT /account/brush/:id and PUT /account/store/:id.
 */
(function () {
  'use strict';
  var C = (window.Chronicle = window.Chronicle || {});

  var LOOKS = [['quick', 'Quick 2D'], ['rune', 'Rune 3D'], ['none', 'Numbers only']];
  var LOOK_NOTE = {
    quick: 'Light and quick on any device, and fine on a busy table.',
    rune: 'The rune dice, thrown in 3D. Dice they don’t come in play as quick 2D.',
    none: 'No dice are thrown: the result goes straight to the notice and the log.'
  };

  function esc(s) { return C.escapeHtml ? C.escapeHtml(String(s)) : String(s).replace(/[&<>"']/g, function (c) { return '&#' + c.charCodeAt(0) + ';'; }); }
  function $(sel, root) { return (root || document).querySelector(sel); }
  function lookName(id) { for (var i = 0; i < LOOKS.length; i++) if (LOOKS[i][0] === id) return LOOKS[i][1]; return LOOKS[0][1]; }
  function say(msg, kind) { if (C.notify) C.notify(msg, kind || 'error'); }

  // The Table's storage, read and written exactly as table.js does.
  function store(key, val) {
    try {
      if (val === undefined) return window.localStorage.getItem('chronicle.table.' + key);
      window.localStorage.setItem('chronicle.table.' + key, val);
    } catch (e) { /* private window: nothing is remembered */ }
    return null;
  }

  function seg(attr, cur, items, label) {
    return '<div class="acct-seg" role="group" aria-label="' + esc(label) + '">' + items.map(function (it) {
      return '<button type="button" ' + attr + '="' + it[0] + '" aria-pressed="' + (cur === it[0]) + '">' + esc(it[1]) + '</button>';
    }).join('') + '</div>';
  }

  function Dice(host, base, key) {
    this.body = $('[data-acct-dice-body]', host);
    this.key = key;
    this.tableLook = 'quick';
    this.base = base;
  }
  Dice.prototype.load = function () {
    var self = this;
    C.apiFetch(this.base + '/rolls/settings').then(function (r) { return r.ok ? r.json() : null; }).then(function (s) {
      if (!s) throw new Error('unread');
      if (s.look) self.tableLook = s.look;
      self.draw();
    }).catch(function () {
      self.body.textContent = 'The dice settings could not be read. Reload the page to try again.';
    });
  };
  Dice.prototype.draw = function () {
    var own = store('look.' + this.key) || '';
    var mine = own || this.tableLook;
    var crits = store('crits.' + this.key) !== 'off';
    var html = '<div class="acct-field"><p class="acct-label">How dice are drawn</p>' +
      seg('data-acct-look', mine, LOOKS, 'How your dice are drawn') +
      '<p class="acct-note">' + esc(LOOK_NOTE[mine] || '') + '</p>' +
      (own ? '<button type="button" class="acct-word" data-acct-look-reset>Use the table’s choice (' + esc(lookName(this.tableLook)) + ')</button>' : '<p class="acct-note">Following the table’s choice.</p>') +
      '</div><div class="acct-field"><p class="acct-label">Crit animations</p>' +
      seg('data-acct-crits', crits ? 'on' : 'off', [['on', 'On'], ['off', 'Off']], 'Crit animations') +
      '<p class="acct-note">Off keeps a crit’s coloured edge on the dice, without the effects.</p></div>';
    var wrap = document.createElement('div');
    wrap.setAttribute('data-acct-dice-body', '');
    wrap.innerHTML = html;
    this.body.replaceWith(wrap);
    this.body = wrap;
  };
  Dice.prototype.click = function (b) {
    if (b.hasAttribute('data-acct-look')) store('look.' + this.key, b.getAttribute('data-acct-look'));
    else if (b.hasAttribute('data-acct-look-reset')) store('look.' + this.key, '');
    else if (b.hasAttribute('data-acct-crits')) store('crits.' + this.key, b.getAttribute('data-acct-crits'));
    else return false;
    this.draw();
    var again = $(b.hasAttribute('data-acct-look-reset') ? '[data-acct-look][aria-pressed="true"]' : '[' + (b.hasAttribute('data-acct-crits') ? 'data-acct-crits' : 'data-acct-look') + '="' + (b.getAttribute('data-acct-crits') || b.getAttribute('data-acct-look')) + '"]', this.body);
    if (again) again.focus();
    return true;
  };

  function Calendar(host, base) {
    this.box = $('[data-acct-cal]', host);
    this.base = base;
  }
  Calendar.prototype.load = function () {
    var self = this;
    C.apiFetch(this.base + '/sessions/feed-settings').then(function (r) { return r.ok ? r.json() : null; }).then(function (s) {
      if (s && s.enabled) self.box.hidden = false;
    }).catch(function () { /* left hidden: the calendar page still offers it */ });
  };
  Calendar.prototype.fetch = function (replace) {
    var self = this, body = $('[data-acct-cal-body]', this.box);
    var fail = 'Your calendar link could not be made. Try again.';
    if (this.busy) return;
    this.busy = true;
    C.apiFetch(this.base + '/sessions/feed/token' + (replace ? '/replace' : ''), { method: 'POST', body: {} })
      .then(function (r) { if (!r.ok) throw new Error(fail); return r.json(); })
      .then(function (b) {
        var path = b && b.url;
        if (typeof path !== 'string' || !/^\/sessions\/feed\/[A-Za-z0-9_-]+\.ics$/.test(path)) throw new Error(fail);
        var full = window.location.origin.replace(/\/+$/, '') + path;
        body.innerHTML = '<div class="acct-linkrow"><input type="text" class="input" readonly aria-label="Your private calendar link" value="' + esc(full) + '" data-acct-cal-url>' +
          '<button type="button" class="btn-secondary" data-acct-cal-copy>Copy</button></div>' +
          '<p class="acct-note">Only you have this link. Anyone with it can see this campaign’s game night dates. <button type="button" class="acct-word" data-acct-cal-replace>Replace link</button></p>' +
          '<div class="acct-confirm" data-acct-cal-confirm hidden><p class="acct-note">Replacing stops the old link at once, so any calendar using it stops updating until you add the new one.</p>' +
          '<button type="button" class="btn-secondary" data-acct-cal-replace-yes>Replace it</button> <button type="button" class="acct-word" data-acct-cal-replace-no>Keep this link</button></div>';
        if (replace) say('New link made. The old one no longer works.', 'success');
        var field = $('[data-acct-cal-url]', body);
        if (field) field.focus();
      })
      .catch(function (err) { say((err && err.message) || fail); })
      .then(function () { self.busy = false; });
  };
  Calendar.prototype.click = function (b) {
    var confirm = $('[data-acct-cal-confirm]', this.box);
    if (b.hasAttribute('data-acct-cal-show')) { this.fetch(false); return true; }
    if (b.hasAttribute('data-acct-cal-replace')) { if (confirm) { confirm.hidden = false; $('[data-acct-cal-replace-yes]', confirm).focus(); } return true; }
    if (b.hasAttribute('data-acct-cal-replace-no')) { if (confirm) confirm.hidden = true; $('[data-acct-cal-replace]', this.box).focus(); return true; }
    if (b.hasAttribute('data-acct-cal-replace-yes')) { this.fetch(true); return true; }
    if (b.hasAttribute('data-acct-cal-copy')) {
      var field = $('[data-acct-cal-url]', this.box);
      if (!field) return true;
      var done = function () { say('Link copied.', 'success'); };
      if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(field.value).then(done, function () { field.select(); });
      else { field.select(); }
      return true;
    }
    return false;
  };

  // The owner's shown/put-away choices, outside the widget's host because
  // the page draws them only for the owner. Each is a group marked
  // data-acct-<key>="<campaign id>" whose buttons carry data-<key>, saved to
  // PUT /account/<key>/<id> as {<key>: choice}.
  var CHOICES = ['brush', 'store'];

  function choiceClick(b, key) {
    var group = b.closest('[data-acct-' + key + ']');
    if (!group || b.getAttribute('aria-pressed') === 'true') return;
    var choice = b.getAttribute('data-' + key);
    var all = group.querySelectorAll('button');
    var body = {};
    body[key] = choice;
    Array.prototype.forEach.call(all, function (x) { x.disabled = true; });
    C.apiFetch('/account/' + key + '/' + encodeURIComponent(group.getAttribute('data-acct-' + key)), { method: 'PUT', body: body })
      .then(function (r) {
        if (!r.ok) throw new Error('refused');
        Array.prototype.forEach.call(all, function (x) { x.setAttribute('aria-pressed', String(x === b)); });
      })
      .catch(function () { say('That could not be saved. Try again.'); })
      .then(function () { Array.prototype.forEach.call(all, function (x) { x.disabled = false; }); b.focus(); });
  }

  Chronicle.register('account-campaign', {
    init: function (host) {
      var id = host.getAttribute('data-campaign-id') || '', user = host.getAttribute('data-user-id') || '';
      var base = '/campaigns/' + encodeURIComponent(id);
      var dice = new Dice(host, base, id + '.' + user);
      var cal = new Calendar(host, base);
      dice.load();
      cal.load();
      var groups = CHOICES.map(function (key) { return [key, document.querySelector('[data-acct-' + key + ']')]; });
      host._acctClick = function (e) {
        var b = e.target.closest('button');
        if (!b) return;
        for (var i = 0; i < groups.length; i++) {
          if (groups[i][1] && groups[i][1].contains(b)) { choiceClick(b, groups[i][0]); return; }
        }
        if (!host.contains(b)) return;
        if (!dice.click(b)) cal.click(b);
      };
      document.addEventListener('click', host._acctClick);
    },
    destroy: function (host) {
      if (host._acctClick) document.removeEventListener('click', host._acctClick);
    }
  });
})();
