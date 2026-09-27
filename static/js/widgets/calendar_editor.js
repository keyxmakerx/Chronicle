/**
 * calendar_editor.js — Calendar V5 part A (#741): the Owner/co-Director
 * editing surface layered on top of calendar_view.js. Loaded only when the
 * page's mount div carries data-can-edit="true" (view.templ), which is
 * true for MemberRole >= RoleOwner OR CanAuthorDmOnly() — see view.go's
 * CalendarViewData.CanEdit doc comment.
 *
 * This file does not mount as its own Chronicle widget (there is only one
 * data-widget="calendar_view" element on the page); it augments the
 * already-mounted calendar_view instance, found on `el.calendarView` (or
 * via the 'calendarv5:ready' event if this script's <script defer> tag
 * somehow ran first).
 *
 * GATING NOTE — read before changing any of the checks below: CanEdit
 * (the gate this whole file loads behind) covers Owner OR a granted
 * co-Director. Three of the writes this file makes are narrower than that,
 * because their JSON routes (routes.go) are gated RoleOwner strictly, not
 * CanAuthorDmOnly: creating an event kind, toggling a moon's hidden flag,
 * and deleting an event. Each of those three checks `role >= ROLE_OWNER`
 * instead of the broader canEdit/canAuthorDmOnly — a co-Director who can
 * reach this file at all will still see those specific affordances 403 if
 * this gating is loosened without a matching routes.go change. See the
 * plugin's .ai.md "Honest gaps" section: this mismatch (the UI can become
 * visible for a co-Director before the API allows it) is real and tracked,
 * not hidden.
 */
(function () {
  'use strict';

  var ROLE_OWNER = 3;

  // Shared with calendar_view.js (Chronicle.calendarDate = CalDate there) so
  // "shift events" uses the exact same date arithmetic as the grid/moon
  // phase code, rather than a second copy that could drift from it.
  var CalDate = Chronicle.calendarDate;

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (ch) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch];
    });
  }

  function attach(view) {
    var editor = new CalendarEditor(view);
    editor.install();
  }

  // Loaded unconditionally on every page (internal/app/routes.go's
  // pluginBodyScripts registry, outside the sidebar's hx-boost-swapped
  // region — see tools/check-page-scripts.sh for why a page templ can't
  // conditionally <script src> this instead), so THIS file — not the
  // Templ page — is what gates the whole editing surface on the viewer
  // actually being able to edit: no mount at all (most pages), or a mount
  // whose data-can-edit isn't "true" (a Player on the calendar page), both
  // no-op here, exactly as if the script had never loaded.
  var mount = document.querySelector('[data-widget="calendar_view"]');
  if (mount && mount.dataset.canEdit === 'true') {
    if (mount.calendarView) attach(mount.calendarView);
    else mount.addEventListener('calendarv5:ready', function (e) { attach(e.detail); });
  }

  function CalendarEditor(view) {
    this.view = view;
    this.editing = false;
    this.selection = {}; // dayKey -> true, edit-mode multi-select
    this.anchorKey = null; // for shift-click range select
  }

  CalendarEditor.prototype.install = function () {
    this._injectEditToggle();
    this._injectHEditStrip();
    this._wrapEvpRendering();
    this._wrapMvRendering();
    this._wrapFlapRendering();
    this._bindStageCapture();
    this._bindWingCapture();
    this._bindEvpCapture();
    this._bindMvCapture();
    this._bindFlapCapture();
    this._buildBulkBar();
  };

  // ------------------------------------------------------------------
  // Edit-mode toggle + the header's edit strip (h-edit swaps in for
  // h-sub, at the same height, so the grid never moves — calendar-view.css
  // already carries the .cal.editing show/hide transition for this).
  // ------------------------------------------------------------------
  CalendarEditor.prototype._injectEditToggle = function () {
    var acts = $('.h-acts', this.view.el);
    if (!acts) return;
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'tbtn';
    btn.id = 'cal5-editbtn';
    btn.setAttribute('aria-pressed', 'false');
    btn.innerHTML = '<i class="fa-solid fa-sliders"></i><span>Edit</span>';
    acts.insertBefore(btn, acts.firstChild);
    var self = this;
    btn.addEventListener('click', function () { self.setEditing(!self.editing); });
    this.editBtn = btn;
  };

  CalendarEditor.prototype._injectHEditStrip = function () {
    var subw = $('.h-subw', this.view.el);
    if (!subw) return;
    var strip = document.createElement('div');
    strip.className = 'h-edit';
    strip.innerHTML = '<span class="etag">Editing</span><span class="ehint">Click days to choose them, or an event’s icon to edit it.</span><span class="sp"></span><button type="button" class="btn sm quiet" id="cal5-editdone">Done</button>';
    subw.appendChild(strip);
    $('#cal5-editdone', strip).addEventListener('click', this.setEditing.bind(this, false));
  };

  CalendarEditor.prototype.setEditing = function (on) {
    this.editing = on;
    this.view.calEl.classList.toggle('editing', on);
    this.editBtn.setAttribute('aria-pressed', String(on));
    if (!on) { this.selection = {}; this._syncPicks(); this._renderBar(); }
    this.view.say(on ? 'Editing on. Click days to choose them.' : 'Editing off.');
  };

  // ------------------------------------------------------------------
  // Select-many: while editing, clicking a day toggles it into the
  // selection instead of opening its wing; shift-click extends from the
  // last-clicked day (a simple range, not the mockup's press-and-drag).
  // Captured (not bubble) so it runs before calendar_view.js's own stage
  // click handler and can stop it with stopPropagation.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._bindStageCapture = function () {
    var self = this;
    this.view.stageEl.addEventListener('click', function (e) {
      if (!self.editing) return;
      var day = e.target.closest('.day');
      if (!day || !day.dataset.key) return;
      e.stopPropagation();
      var extend = e.shiftKey || e.ctrlKey || e.metaKey;
      var keys = (e.shiftKey && self.anchorKey) ? self._rangeBetween(self.anchorKey, day.dataset.key) : [day.dataset.key];
      if (!extend) {
        // Plain click replaces the selection with just this one day.
        self.selection = {};
        self.selection[day.dataset.key] = true;
      } else {
        // Shift/Ctrl click extends: if every key in the range/click is
        // already selected, the gesture removes them; otherwise it adds
        // them all. (Mirrors the mockup's applySel 'remove'/'add' — never
        // a per-key toggle, which would leave a range half on/half off.)
        var allOn = keys.every(function (k) { return self.selection[k]; });
        keys.forEach(function (k) {
          if (allOn) delete self.selection[k];
          else self.selection[k] = true;
        });
      }
      self.anchorKey = day.dataset.key;
      self._syncPicks();
      self._renderBar();
    }, true);
  };

  CalendarEditor.prototype._rangeBetween = function (a, b) {
    var buttons = $$('.day[data-key]', this.view.stageEl);
    var keys = buttons.map(function (d) { return d.dataset.key; });
    var ia = keys.indexOf(a), ib = keys.indexOf(b);
    if (ia < 0 || ib < 0) return [b];
    var lo = Math.min(ia, ib), hi = Math.max(ia, ib);
    return keys.slice(lo, hi + 1);
  };

  CalendarEditor.prototype._syncPicks = function () {
    var self = this;
    $$('.day[data-key]', this.view.stageEl).forEach(function (d) {
      d.classList.toggle('pick', !!self.selection[d.dataset.key]);
    });
  };

  CalendarEditor.prototype._selectedKeys = function () {
    var self = this;
    return Object.keys(this.selection).filter(function (k) { return self.selection[k]; });
  };

  // Every distinct event touching any selected day.
  CalendarEditor.prototype._eventsInSelection = function () {
    var view = this.view, seen = {}, out = [];
    this._selectedKeys().forEach(function (k) {
      var d = k.split('_').map(Number);
      view.eventsOnDay(d[0], d[1], d[2]).forEach(function (e) {
        if (!seen[e.id]) { seen[e.id] = true; out.push(e); }
      });
    });
    return out;
  };

  // ------------------------------------------------------------------
  // The bulk bar: rises from the grid's bottom edge as soon as a day is
  // selected. Only the two bulk actions this API can actually perform
  // ship: bulk visibility (Hide/Reveal, gated CanAuthorDmOnly, the same
  // gate the single-event visibility route already uses) and Shift events
  // (moves every event touching the selection by N days). The mockup's
  // Paint weather / Generate… / Lock actions are left out — no
  // per-day weather write endpoint (#765) and no generator engine ship in
  // this PR (see .ai.md); wiring an empty "Generate…" button into a
  // shipped bulk bar would be worse than not shipping it.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._buildBulkBar = function () {
    var wrap = document.createElement('div');
    wrap.className = 'bbw';
    wrap.innerHTML = '<div class="bbar" id="cal5-bbar" hidden></div>';
    this.view.calEl.appendChild(wrap);
    this.bbar = $('#cal5-bbar', wrap);
    var self = this;
    this.bbar.addEventListener('click', function (e) {
      var t = e.target.closest('[data-bb]');
      if (!t) return;
      var a = t.dataset.bb;
      if (a === 'none') { self.selection = {}; self._syncPicks(); self._renderBar(); }
      else if (a === 'hide' || a === 'reveal') self._bulkVisibility(a === 'hide' ? 'dm_only' : 'everyone');
      else if (a === 'shift') self._toggleShiftTray();
    });
  };

  CalendarEditor.prototype._renderBar = function () {
    var n = this._selectedKeys().length;
    if (!n) { this.bbar.hidden = true; this.bbar.innerHTML = ''; this._closeShiftTray(); return; }
    var canVis = this.view.canAuthorDmOnly;
    this.bbar.hidden = false;
    this.bbar.innerHTML =
      '<div class="bbl"><b>' + (n === 1 ? '1 day' : n + ' days') + ' chosen</b><span>' + this._eventsInSelection().length + ' events touched</span></div>' +
      '<div class="bba">' +
        (canVis ? '<button type="button" class="bbb" data-bb="hide"><i class="fa-solid fa-eye-slash"></i><span>Hide</span></button><button type="button" class="bbb" data-bb="reveal"><i class="fa-solid fa-eye"></i><span>Reveal</span></button>' : '') +
        '<button type="button" class="bbb" data-bb="shift" aria-expanded="false"><i class="fa-solid fa-arrows-left-right"></i><span>Shift events</span></button>' +
      '</div>' +
      '<button type="button" class="x" data-bb="none" aria-label="Choose no days">✕</button>';
  };

  CalendarEditor.prototype._bulkVisibility = function (visibility) {
    var self = this, view = this.view, events = this._eventsInSelection();
    if (!events.length) { view.say('No events in the selected days.'); return; }
    Promise.all(events.map(function (e) {
      return Chronicle.apiFetch(view.apiBase + '/events/' + e.id + '/visibility', {
        method: 'PUT', body: { visibility: visibility }
      });
    })).then(function () {
      view.eventsByMonth = {}; // simplest correct invalidation: refetch on next paint
      view.renderMonth();
      view.say((visibility === 'dm_only' ? 'Hidden ' : 'Revealed ') + events.length + (events.length === 1 ? ' event.' : ' events.'));
    });
  };

  CalendarEditor.prototype._toggleShiftTray = function () {
    if (this._shiftTray) { this._closeShiftTray(); return; }
    var self = this, view = this.view;
    var tray = document.createElement('div');
    tray.className = 'btray';
    tray.innerHTML = '<div class="trh">Shift events<small>Moves every event touching the chosen days by this many days.</small></div>' +
      '<div class="shn"><button type="button" data-shift="-1">−</button><output id="cal5-shiftn">+1</output><button type="button" data-shift="1">+</button>' +
      '<button type="button" class="btn primary sm" id="cal5-shiftgo">Apply</button></div>';
    view.calEl.appendChild(tray);
    this._shiftTray = tray;
    this._shiftN = 1;
    $('[data-bb="shift"]', this.bbar).setAttribute('aria-expanded', 'true');
    tray.addEventListener('click', function (e) {
      var d = e.target.closest('[data-shift]');
      if (d) { self._shiftN += parseInt(d.dataset.shift, 10); $('#cal5-shiftn', tray).textContent = (self._shiftN >= 0 ? '+' : '') + self._shiftN; }
      if (e.target.id === 'cal5-shiftgo') self._applyShift();
    });
  };

  CalendarEditor.prototype._closeShiftTray = function () {
    if (!this._shiftTray) return;
    var b = $('[data-bb="shift"]', this.bbar);
    if (b) b.setAttribute('aria-expanded', 'false');
    this._shiftTray.remove();
    this._shiftTray = null;
  };

  CalendarEditor.prototype._applyShift = function () {
    var self = this, view = this.view, cal = view.cal, delta = this._shiftN, events = this._eventsInSelection();
    if (!delta || !events.length) { this._closeShiftTray(); return; }
    Promise.all(events.map(function (e) {
      var start = CalDate.addDays(cal, { y: e.year, m: e.month, d: e.day }, delta);
      var body = { year: start.y, month: start.m, day: start.d };
      if (e.end_year != null) {
        var end = CalDate.addDays(cal, { y: e.end_year, m: e.end_month, d: e.end_day }, delta);
        body.end_year = end.y; body.end_month = end.m; body.end_day = end.d;
      }
      return Chronicle.apiFetch(view.apiBase + '/events/' + e.id, { method: 'PUT', body: body });
    })).then(function () {
      view.eventsByMonth = {};
      view.renderMonth();
      self._closeShiftTray();
      view.say('Shifted ' + events.length + (events.length === 1 ? ' event' : ' events') + ' by ' + delta + ' day' + (Math.abs(delta) === 1 ? '' : 's') + '.');
    });
  };

  // ------------------------------------------------------------------
  // Day wing: bind the "Add event" button calendar_view.js already
  // rendered (it knows CanEdit from its own config, independent of this
  // file loading at all) — the event-editing entry point itself lives in
  // the detail overlay (evp) below, not a second control here.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._bindWingCapture = function () {
    var self = this, view = this.view;
    view.wingEl.addEventListener('click', function (e) {
      if (e.target.closest('[data-add-event]')) {
        e.stopPropagation();
        self._openEventForm(view.wingFor, null);
      }
    }, true);
  };

  // ------------------------------------------------------------------
  // Event detail overlay: adds an Edit / Delete footer for a viewer who
  // may write this event. Delete is Owner-only (routes.go), independent
  // of the broader canEdit/canAuthorDmOnly gate — see the file's gating
  // note at the top.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._wrapEvpRendering = function () {
    var self = this, view = this.view, original = view._evpHTML.bind(view);
    view._evpHTML = function (ev) {
      var html = original(ev);
      var canDelete = view.role >= ROLE_OWNER;
      var foot = '<div class="efoot"><button type="button" class="btn quiet" data-edit-event="' + esc(ev.id) + '"><i class="fa-solid fa-pencil"></i> Edit</button><span class="sp"></span>' +
        (canDelete ? '<button type="button" class="btn danger" data-delete-event="' + esc(ev.id) + '"><i class="fa-solid fa-trash"></i> Delete</button>' : '') + '</div>';
      return html.replace('<!--EVP_FOOT-->', foot);
    };
  };

  CalendarEditor.prototype._bindEvpCapture = function () {
    var self = this, view = this.view;
    view.evpEl.addEventListener('click', function (e) {
      var ed = e.target.closest('[data-edit-event]');
      var del = e.target.closest('[data-delete-event]');
      if (ed) { e.stopPropagation(); self._openEventForm(view.wingFor, view._findEvent(ed.dataset.editEvent)); }
      if (del) {
        e.stopPropagation();
        if (!window.confirm('Delete this event? This cannot be undone.')) return;
        Chronicle.apiFetch(view.apiBase + '/events/' + del.dataset.deleteEvent, { method: 'DELETE' }).then(function (resp) {
          if (!resp.ok) { view.say('Could not delete the event.'); return; }
          view.eventsByMonth = {};
          view.closeEventDetail();
          view.renderMonth();
          view.say('Event deleted.');
        });
      }
    }, true);
  };

  // ------------------------------------------------------------------
  // Event create/edit form: a compact inline form appended into the wing,
  // grouped like the mockup's editor (name, date, kind incl.
  // new-kind-in-place, visibility) without the mockup's full When/Details/
  // Who/More sectioning — the fields that map onto real Chronicle fields,
  // nothing invented.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._openEventForm = function (dayKey, existing) {
    var self = this, view = this.view;
    var d = dayKey.split('_').map(Number);
    var kinds = view.cal.event_kinds || [];
    var isNew = !existing;
    var form = document.createElement('form');
    form.className = 'ebody';
    form.style.marginTop = '10px';
    form.innerHTML =
      '<div class="fld"><input class="etitle" name="name" placeholder="Event name" required value="' + esc(existing ? existing.name : '') + '"/></div>' +
      '<div class="fld"><textarea name="description" rows="2" placeholder="Optional description">' + esc(existing ? existing.description || '' : '') + '</textarea></div>' +
      '<div class="fld">Kind<div class="chips" id="cal5-edkinds">' + this._kindChipsHTML(kinds, existing ? existing.kind_id : null) + '</div></div>' +
      (view.role >= ROLE_OWNER ? '<div id="cal5-nkf-mount"></div>' : '') +
      (view.canAuthorDmOnly ? '<div class="vis" role="group" aria-label="Visibility">' +
        '<button type="button" data-vis="everyone" aria-pressed="' + (!existing || existing.visibility !== 'dm_only') + '">Everyone</button>' +
        '<button type="button" data-vis="dm_only" aria-pressed="' + (!!existing && existing.visibility === 'dm_only') + '">Director only</button></div>' : '') +
      '<div class="efoot"><button type="button" class="btn quiet" data-cancel>Cancel</button><span class="sp"></span><button type="submit" class="btn primary">' + (isNew ? 'Create' : 'Save') + '</button></div>';

    form.dataset.kindId = existing && existing.kind_id != null ? String(existing.kind_id) : '';
    form.dataset.visibility = existing ? existing.visibility : 'everyone';

    form.addEventListener('click', function (e) {
      if (e.target.closest('[data-cancel]')) { form.remove(); return; }
      var chip = e.target.closest('[data-kind]');
      if (chip) { form.dataset.kindId = chip.dataset.kind; $$('#cal5-edkinds [data-kind]', form).forEach(function (b) { b.setAttribute('aria-pressed', String(b === chip)); }); }
      var vis = e.target.closest('[data-vis]');
      if (vis) { form.dataset.visibility = vis.dataset.vis; $$('.vis [data-vis]', form).forEach(function (b) { b.setAttribute('aria-pressed', String(b === vis)); }); }
      var nk = e.target.closest('[data-nkb]');
      if (nk) self._openNewKindForm(form);
    });

    form.addEventListener('submit', function (e) {
      e.preventDefault();
      var body = {
        name: form.name.value.trim(),
        description: form.description.value || null,
        kind_id: form.dataset.kindId ? +form.dataset.kindId : null
      };
      // This compact form has no date or time picker, so it must never
      // resend year/month/day/all_day on an EDIT — a PUT is a partial
      // update (patch.Field): omitting a key preserves the stored value,
      // while resending today's wing day or a hardcoded all_day:true would
      // silently move the event (wrong for a recurring/spanning event
      // viewed from an occurrence other than its own base date) or flip a
      // timed event to all-day on its very first edit through this UI. A
      // brand-new event has no stored date yet, so it takes the day whose
      // wing it was created from and is all-day (the only mode this form
      // supports until a time picker exists).
      if (isNew) {
        body.year = d[0]; body.month = d[1]; body.day = d[2];
        body.all_day = true;
      }
      if (view.canAuthorDmOnly) body.visibility = form.dataset.visibility;
      var req;
      if (isNew) req = Chronicle.apiFetch(view.apiBase + '/events', { method: 'POST', body: body });
      else req = Chronicle.apiFetch(view.apiBase + '/events/' + existing.id, { method: 'PUT', body: body });
      req.then(function (resp) {
        if (!resp.ok) { view.say('Could not save the event.'); return; }
        view.eventsByMonth = {};
        form.remove();
        if (!isNew) view.closeEventDetail();
        view.renderMonth();
        view.openWing(dayKey);
        view.say(isNew ? 'Event created.' : 'Event saved.');
      });
    });

    view.wingEl.querySelector('.wb').appendChild(form);
    form.name.focus();
  };

  CalendarEditor.prototype._kindChipsHTML = function (kinds, activeId) {
    var html = kinds.map(function (k) {
      var style = k.color ? ('color:' + String(k.color).replace(/[^#a-zA-Z0-9(),.% ]/g, '') + ';') : '';
      return '<button type="button" data-kind="' + k.id + '" aria-pressed="' + (k.id === activeId) + '" style="' + style + '">' + esc(k.icon || '') + ' ' + esc(k.name) + '</button>';
    }).join('');
    if (this.view.role >= ROLE_OWNER) html += '<button type="button" class="nkb" data-nkb><i class="fa-solid fa-plus"></i> New kind</button>';
    return html;
  };

  // New-kind-in-place: an inline form (name/icon/colour), POSTs
  // /calendars/event-kinds (campaign-scoped, Owner-only end to end —
  // routes.go — so this entry point itself is only ever rendered for
  // role >= RoleOwner, see _kindChipsHTML above).
  CalendarEditor.prototype._openNewKindForm = function (parentForm) {
    var self = this, view = this.view, mount = $('#cal5-nkf-mount', parentForm);
    if (!mount || mount.querySelector('.nkf')) return;
    var swatches = ['#ef4444', '#f97316', '#84cc16', '#22c55e', '#06b6d4', '#3b82f6', '#8b5cf6', '#ec4899'];
    mount.innerHTML = '<div class="nkf">' +
      '<div class="nkt">New kind of event</div>' +
      '<div class="fld"><input id="cal5-nk-name" placeholder="Name" required/></div>' +
      '<div class="fld"><input id="cal5-nk-icon" placeholder="Icon (a short glyph, e.g. ⚔)" maxlength="8"/></div>' +
      '<div class="swp" role="group" aria-label="Colour">' + swatches.map(function (c, i) {
        return '<button type="button" data-sw="' + c + '" style="--sw:' + c + '" aria-checked="' + (i === 0) + '"></button>';
      }).join('') + '</div>' +
      '<div class="nkfoot"><button type="button" class="btn quiet" data-nk-cancel>Cancel</button><button type="button" class="btn primary" data-nk-save>Add kind</button></div>' +
      '</div>';
    mount.dataset.color = swatches[0];
    mount.addEventListener('click', function (e) {
      if (e.target.closest('[data-nk-cancel]')) { mount.innerHTML = ''; return; }
      var sw = e.target.closest('[data-sw]');
      if (sw) { mount.dataset.color = sw.dataset.sw; $$('.swp button', mount).forEach(function (b) { b.setAttribute('aria-checked', String(b === sw)); }); }
      if (e.target.closest('[data-nk-save]')) self._saveNewKind(mount, parentForm);
    });
  };

  CalendarEditor.prototype._saveNewKind = function (mount, parentForm) {
    var view = this.view;
    var name = $('#cal5-nk-name', mount).value.trim();
    if (!name) { $('#cal5-nk-name', mount).focus(); return; }
    var slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'kind';
    var icon = $('#cal5-nk-icon', mount).value.trim() || '●';
    Chronicle.apiFetch('/campaigns/' + view.campaignId + '/calendars/event-kinds', {
      method: 'POST',
      body: { slug: slug, name: name, icon: icon, color: mount.dataset.color, sort_order: (view.cal.event_kinds || []).length, default_announced: 'on_day' }
    }).then(function (resp) {
      if (!resp.ok) { view.say('Could not create the kind (the slug may already exist).'); return; }
      return resp.json();
    }).then(function (kind) {
      if (!kind) return;
      view.cal.event_kinds = (view.cal.event_kinds || []).concat([kind]);
      mount.innerHTML = '';
      var chipsEl = $('#cal5-edkinds', parentForm);
      if (chipsEl) chipsEl.outerHTML = '<div class="chips" id="cal5-edkinds">' + this._kindChipsHTML(view.cal.event_kinds, kind.id) + '</div>';
      parentForm.dataset.kindId = String(kind.id);
      view.renderLegend();
      view.say('Added the kind “' + name + '”.');
    }.bind(this));
  };

  // ------------------------------------------------------------------
  // Moon hidden toggle, added to the moon view's strip. Owner-only
  // (routes.go: PUT .../moons/:id/hidden is RequireRole(RoleOwner), not
  // CanAuthorDmOnly) — same gating note as event-kind creation.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._wrapMvRendering = function () {
    var self = this, view = this.view, original = view._mvHTML.bind(view);
    view._mvHTML = function () {
      var html = original();
      if (view.role < ROLE_OWNER) return html;
      var moon = view.moonById(view.mvMoonId) || (view.cal.moons || [])[0];
      if (!moon) return html;
      var toggle = '<div class="sw" style="margin-top:10px"><span>Shown to players<small>Hide a moon while it is still a secret.</small></span>' +
        '<button type="button" class="tog" role="switch" aria-checked="' + !moon.hidden_from_players + '" data-toggle-moon="' + moon.id + '"></button></div>';
      return html.replace('<div class="mvstrip">', toggle + '<div class="mvstrip">');
    };
  };

  CalendarEditor.prototype._bindMvCapture = function () {
    var self = this, view = this.view;
    view.mvEl.addEventListener('click', function (e) {
      var t = e.target.closest('[data-toggle-moon]');
      if (!t) return;
      e.stopPropagation();
      var moon = view.moonById(t.dataset.toggleMoon);
      if (!moon) return;
      var hidden = !moon.hidden_from_players;
      Chronicle.apiFetch(view.apiBase + '/moons/' + moon.id + '/hidden', { method: 'PUT', body: { hidden: hidden } }).then(function (resp) {
        if (!resp.ok) { view.say('Could not change the moon’s visibility.'); return; }
        moon.hidden_from_players = hidden;
        view._refreshMv();
        view.say(moon.name + (hidden ? ' hidden from players.' : ' shown to players.'));
      });
    }, true);
  };

  // ------------------------------------------------------------------
  // Era manager: the one piece of the mockup's "structure editor" (months,
  // weekdays, leap rule, moons, eras) whose write API actually exists today
  // (routes.go: POST/PUT/DELETE .../eras[/:eraID], Owner-only end to end).
  // Months, weekdays, the leap-year rule and a moon's own name/cycle/color
  // have NO write route in PR #791 at all — see the plugin's .ai.md "Honest
  // gaps"; building a UI for those would write into nothing. So the flap
  // becomes a real list-manage-add editor only for role >= ROLE_OWNER (a
  // co-Director's flap stays the original read-only single-era view — this
  // route is not gated CanAuthorDmOnly, see the file header's gating note);
  // everyone else keeps calendar_view.js's read-only current-era flap.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._wrapFlapRendering = function () {
    var self = this, view = this.view;
    if (view.role < ROLE_OWNER) return;
    this._eraFormFor = undefined; // undefined = list mode, 'new' = create, <id> = edit
    view.showFlap = function () {
      var btn = $('#cal5-erabtn', view.el);
      self._eraFormFor = undefined;
      view.flapEl.innerHTML = self._eraManagerHTML();
      Chronicle.calendarPanel.growOpen(view.flapEl, btn, view.calEl);
      btn.setAttribute('aria-expanded', 'true');
      view._updateScrim();
    };
  };

  CalendarEditor.prototype._refreshFlap = function () {
    this.view.flapEl.innerHTML = this._eraManagerHTML();
  };

  CalendarEditor.prototype._eraManagerHTML = function () {
    var view = this.view;
    var eras = (view.cal.eras || []).slice().sort(function (a, b) {
      return (a.sort_order || 0) - (b.sort_order || 0) || a.start_year - b.start_year;
    });
    var current = view.eraForDate(view.view.y, view.view.m, 1);
    var rows = eras.map(function (e) {
      var span = e.start_year + (e.end_year != null ? '–' + e.end_year : '–present');
      return '<div class="etl-seg' + (current && e.id === current.id ? ' cur' : '') + '">' +
        '<span>' + esc(e.name) + '<small>' + esc(span) + '</small></span>' +
        '<span class="acts">' +
          '<button type="button" class="qi" data-edit-era="' + e.id + '" aria-label="Edit ' + esc(e.name) + '"><i class="fa-solid fa-pencil"></i></button>' +
          '<button type="button" class="qi" data-del-era="' + e.id + '" aria-label="Delete ' + esc(e.name) + '"><i class="fa-solid fa-trash"></i></button>' +
        '</span></div>';
    }).join('') || '<p class="none">No eras yet.</p>';

    var formFor = this._eraFormFor;
    var body = formFor !== undefined
      ? this._eraFormHTML(formFor === 'new' ? null : eras.filter(function (e) { return String(e.id) === String(formFor); })[0])
      : '<button type="button" class="addev" data-add-era><i class="fa-solid fa-plus"></i>Add an era</button>';

    return '<div class="grab" aria-hidden="true"></div>' +
      '<div class="crease"><span>Eras</span><button type="button" class="x" data-close aria-label="Close">✕</button></div>' +
      '<div class="fb"><div class="etl-track">' + rows + '</div>' + body + '</div>';
  };

  CalendarEditor.prototype._eraFormHTML = function (era) {
    var isNew = !era;
    return '<form class="ebody" id="cal5-eraform" style="margin-top:10px">' +
      '<div class="fld"><input class="etitle" name="name" placeholder="Era name" required value="' + esc(era ? era.name : '') + '"/></div>' +
      '<div class="two">' +
        '<div class="fld">Starts<div class="erow"><span class="rl">Y</span><input type="number" name="start_year" required value="' + (era ? era.start_year : '') + '"/><input type="number" name="start_month" min="1" placeholder="M" value="' + (era ? era.start_month : 1) + '"/><input type="number" name="start_day" min="1" placeholder="D" value="' + (era ? era.start_day : 1) + '"/></div></div>' +
        '<div class="fld">Ends<label class="check"><input type="checkbox" name="ongoing"' + (!era || era.end_year == null ? ' checked' : '') + '/> Ongoing</label>' +
          '<div class="erow" data-end-fields' + (!era || era.end_year == null ? ' hidden' : '') + '><span class="rl">Y</span><input type="number" name="end_year" value="' + (era && era.end_year != null ? era.end_year : '') + '"/><input type="number" name="end_month" min="1" placeholder="M" value="' + (era && era.end_month != null ? era.end_month : '') + '"/><input type="number" name="end_day" min="1" placeholder="D" value="' + (era && era.end_day != null ? era.end_day : '') + '"/></div></div>' +
      '</div>' +
      '<div class="fld"><textarea name="description" rows="2" placeholder="Optional description">' + esc(era && era.description ? era.description : '') + '</textarea></div>' +
      '<div class="efoot"><button type="button" class="btn quiet" data-cancel-era>Cancel</button><span class="sp"></span>' +
        (isNew ? '' : '<button type="button" class="btn danger" data-del-era="' + era.id + '">Delete</button>') +
        '<button type="submit" class="btn primary">' + (isNew ? 'Create' : 'Save') + '</button></div>' +
      '</form>';
  };

  CalendarEditor.prototype._bindFlapCapture = function () {
    var self = this, view = this.view;
    if (view.role < ROLE_OWNER) return;
    view.flapEl.addEventListener('click', function (e) {
      var add = e.target.closest('[data-add-era]');
      var edit = e.target.closest('[data-edit-era]');
      var del = e.target.closest('[data-del-era]');
      var cancel = e.target.closest('[data-cancel-era]');
      var ongoing = e.target.closest('[name="ongoing"]');
      if (add) { self._eraFormFor = 'new'; self._refreshFlap(); }
      else if (edit) { self._eraFormFor = edit.dataset.editEra; self._refreshFlap(); }
      else if (cancel) { self._eraFormFor = undefined; self._refreshFlap(); }
      else if (del) {
        e.stopPropagation();
        if (!window.confirm('Delete this era? This cannot be undone.')) return;
        Chronicle.apiFetch(view.apiBase + '/eras/' + del.dataset.delEra, { method: 'DELETE' }).then(function (resp) {
          if (!resp.ok) { view.say('Could not delete the era.'); return; }
          view.cal.eras = (view.cal.eras || []).filter(function (er) { return String(er.id) !== String(del.dataset.delEra); });
          self._eraFormFor = undefined;
          self._refreshFlap();
          view.say('Era deleted.');
        });
      } else if (ongoing) {
        var fields = view.flapEl.querySelector('[data-end-fields]');
        if (fields) fields.hidden = ongoing.checked;
      }
    });
    view.flapEl.addEventListener('change', function (e) {
      if (e.target.name === 'ongoing') {
        var fields = view.flapEl.querySelector('[data-end-fields]');
        if (fields) fields.hidden = e.target.checked;
      }
    });
    view.flapEl.addEventListener('submit', function (e) {
      var form = e.target.closest('#cal5-eraform');
      if (!form) return;
      e.preventDefault();
      var f = form;
      var ongoing = f.ongoing.checked;
      var body = {
        name: f.name.value.trim(),
        start_year: +f.start_year.value,
        start_month: +f.start_month.value || 1,
        start_day: +f.start_day.value || 1,
        end_year: ongoing ? null : (f.end_year.value ? +f.end_year.value : null),
        end_month: ongoing ? null : (f.end_month.value ? +f.end_month.value : null),
        end_day: ongoing ? null : (f.end_day.value ? +f.end_day.value : null),
        description: f.description.value || null,
        color: '#8b5cf6',
        sort_order: (view.cal.eras || []).length
      };
      var isNew = self._eraFormFor === 'new';
      var req = isNew
        ? Chronicle.apiFetch(view.apiBase + '/eras', { method: 'POST', body: body })
        : Chronicle.apiFetch(view.apiBase + '/eras/' + self._eraFormFor, { method: 'PUT', body: body });
      req.then(function (resp) {
        if (!resp.ok) { view.say('Could not save the era.'); return null; }
        return isNew ? resp.json() : null;
      }).then(function (created) {
        if (isNew && created) {
          view.cal.eras = (view.cal.eras || []).concat([created]);
        } else if (!isNew) {
          var existing = (view.cal.eras || []).filter(function (er) { return String(er.id) === String(self._eraFormFor); })[0];
          if (existing) { for (var k in body) existing[k] = body[k]; }
        }
        self._eraFormFor = undefined;
        self._refreshFlap();
        view.say(isNew ? 'Era created.' : 'Era saved.');
      });
    });
  };
})();
