/**
 * design_library_gallery.js — the server gallery inside the saved-designs
 * library (design_library.js): the "Share to this server's gallery" sheet, the
 * "Browse shared designs" sheet, and the "Your gallery entries" list with its
 * status chips. The library mounts it with attach(box, env) and redraws its
 * section with paint(); everything the server decides (checks, review,
 * visibility) stays on the server, this only shows the outcome.
 *
 * Typed text (a name, a note, a search) is never put through markup: it is set
 * with .value after a redraw. Ids and attribute values read back from the page
 * are only compared against the loaded lists, never concatenated into HTML.
 *
 * The pure helpers are on DesignGallery.util so test/js can run them without a page.
 */
(function () {
  'use strict';

  var STATUS = { visible: 'Visible', pending: 'Waiting for review', rejected: 'Not approved', removed: 'Not approved' };
  var KIND_FILTERS = [['all', 'All'], ['rooms', 'Rooms'], ['crafting', 'Crafting']];
  var REPORT_REASONS = [['adult', 'Adult or vulgar'], ['spam', 'Spam or advertising'], ['other', 'Something else']];
  var PAGE_SIZE = 12;

  /* ---- Pure helpers ---- */
  function statusLabel(s) { return STATUS[s] || STATUS.pending; }
  function statusTone(s) { return s === 'visible' ? 'ok' : s === 'pending' ? 'wait' : STATUS[s] ? 'no' : 'wait'; }
  // What an entry's state means to its maker, in a sentence.
  function statusNote(s) {
    if (s === 'visible') return 'Other owners on this server can find it.';
    if (s === 'rejected' || s === 'removed') return 'An admin looked at it and it is not shown. You can change it and send it again.';
    return 'Only you can see it until an admin looks at it.';
  }
  function browseQuery(f) {
    var q = ['page=' + Math.max(1, f.page | 0 || 1)];
    if (f.all) q.push('system=all');
    if (f.kind && f.kind !== 'all') q.push('kind=' + encodeURIComponent(f.kind));
    if (f.q) q.push('q=' + encodeURIComponent(f.q));
    return '?' + q.join('&');
  }
  function pageInfo(total, page, per) {
    per = per || PAGE_SIZE;
    var pages = Math.max(1, Math.ceil(total / per));
    page = Math.min(Math.max(1, page | 0 || 1), pages);
    return { pages: pages, page: page, from: total ? (page - 1) * per + 1 : 0, to: Math.min(total, page * per), hasPrev: page > 1, hasNext: page < pages };
  }
  function countLine(total, all, systemLabel) {
    return total + ' design' + (total === 1 ? '' : 's') + (all ? ' for any game system' : ' made for ' + systemLabel) + ', newest first';
  }
  // The gentle notes under a card: a different game system first, then what the
  // campaign lacks, as the server worded it.
  function cardNotes(card, campaignSystem, systemName) {
    var out = [];
    if (card && card.sameSystem === false) {
      out.push('Made for ' + systemName(card.systemId) + '. This campaign uses ' + systemName(campaignSystem) + '. Parts it uses may be named differently here.');
    }
    ((card && card.lacks) || []).forEach(function (l) { out.push(String(l)); });
    return out;
  }
  function entryFor(own, designId) {
    for (var i = 0; i < own.length; i++) if (own[i].sourceDesignId === designId) return own[i];
    return null;
  }
  // Library designs not yet in the gallery; each can be shared once.
  function freeDesigns(mine, own) { return (mine || []).filter(function (d) { return !entryFor(own || [], d.id); }); }
  function byId(list, id) {
    for (var i = 0; i < list.length; i++) if (list[i].id === id) return list[i];
    return null;
  }
  // Only these three reasons exist; anything else read from the page is "other".
  function reportReason(v) { for (var i = 0; i < REPORT_REASONS.length; i++) if (REPORT_REASONS[i][0] === v) return v; return 'other'; }
  function kindFilter(v) { for (var i = 0; i < KIND_FILTERS.length; i++) if (KIND_FILTERS[i][0] === v) return v; return 'all'; }

  /* ---- State ---- */
  var env = null, box = null, sheetEl = null, opener = null, searchT = 0;
  var G = { own: [], loaded: false, ask: '', msg: '', err: '', sheet: '', busy: false,
    pub: { designId: '', entryId: '', name: '', note: '', maker: '', err: '' },
    br: { all: false, kind: 'all', q: '', page: 1, data: null, err: '', copied: {}, report: null, reportErr: '', note: '' } };

  function esc(t) { return env.esc(t); }
  function say(t) { G.msg = t || ''; G.err = ''; paintMsg(); }
  function fail(e) { G.err = (e && e.message) || 'Something went wrong. Try again.'; G.msg = ''; paintMsg(); }
  function paintMsg() {
    if (!box) return;
    var e = box.querySelector('[data-dg-err]'), m = box.querySelector('[data-dg-msg]');
    if (e) e.textContent = G.err;
    if (m) m.textContent = G.msg;
  }

  /* ---- The section in the library ---- */
  function browseLabel() { return env.kind() === 'room-theme' ? 'Browse shared rooms' : 'Browse shared designs'; }
  function ownRow(e) {
    var h = '<li class="dg-row"><div><b>' + esc(e.name) + '</b> <span class="dg-chip dg-' + statusTone(e.status) + '">' + esc(statusLabel(e.status)) + '</span>' +
      '<span class="dl-sub">' + esc(env.kindLabel(e.kind)) + ' · ' + esc(env.systemName(e.systemId)) + '</span>' +
      '<span class="dl-sub">' + esc(statusNote(e.status)) + '</span>';
    if (e.status !== 'visible' && e.reasons && e.reasons.length) {
      h += '<ul class="dg-why" aria-label="Why it is waiting">' + e.reasons.map(function (r) { return '<li>' + esc(r) + '</li>'; }).join('') + '</ul>';
    }
    h += '</div>';
    if (G.ask === e.id) {
      return h + '<div class="dl-acts"><span class="dl-ok">Take it off the gallery? Copies already made stay with their owners.</span>' +
        '<button type="button" class="dl-bad" data-dg="unpublish-ok" data-id="' + esc(e.id) + '">Unpublish</button><button type="button" data-dg="unpublish-no">Keep it</button></div></li>';
    }
    var canUpdate = !!byId(env.mine(), e.sourceDesignId);
    return h + '<div class="dl-acts">' +
      (canUpdate ? '<button type="button" data-dg="update" data-id="' + esc(e.id) + '" aria-label="Update ' + esc(e.name) + ' in gallery">Update in gallery</button>' : '') +
      // The server keeps a held entry so taking it down cannot wipe its review; only a visible one offers Unpublish.
      (e.status === 'visible' ? '<button type="button" class="dl-bad" data-dg="unpublish" data-id="' + esc(e.id) + '" aria-label="Unpublish ' + esc(e.name) + '">Unpublish</button>' : '') + '</div></li>';
  }
  function sectionHtml() {
    var h = '<small class="dl-g">Server gallery</small><p class="dl-ok">Share a design with other campaigns on this server, or copy one they made.</p>' +
      '<div class="dl-row"><button type="button" class="dl-btn pri" data-dg="open-publish">Share to this server’s gallery</button>' +
      '<button type="button" class="dl-btn" data-dg="open-browse">' + esc(browseLabel()) + '</button></div>' +
      '<small class="dl-g" style="margin-top:8px">Your gallery entries (' + G.own.length + ')</small>';
    h += G.own.length ? '<ul class="dg-list" aria-label="Your gallery entries">' + G.own.map(ownRow).join('') + '</ul>' :
      '<p class="dl-ok">' + (G.loaded ? 'You have not shared anything yet.' : 'Loading…') + '</p>';
    return h + '<p class="dg-err" role="alert" data-dg-err></p><p class="dg-msg" role="status" aria-live="polite" data-dg-msg></p>';
  }
  var CSS = [
    '.dg-list{list-style:none;margin:0;padding:0;display:grid;gap:6px}.dg-row{border:1px solid var(--color-border,#e5e7eb);border-radius:8px;padding:8px 10px;display:grid;gap:4px;background:var(--color-card-bg,#fff)}',
    '.dg-row{color:var(--color-text-body,#374151);font-size:.82rem}.dg-row b{font-size:.88rem;color:var(--color-text-primary,#111827);overflow-wrap:anywhere}.dg-row .dl-sub{display:block;font-size:.76rem;color:var(--color-text-secondary,#6b7280)}',
    '.dg-chip{display:inline-block;font-size:.68rem;font-weight:700;border-radius:999px;padding:1px 8px;border:1px solid currentColor;vertical-align:middle}',
    '.dg-ok{color:#166534}.dg-wait{color:#92400e}.dg-no{color:#b91c1c}',
    '@media (prefers-color-scheme:dark){:root:not([data-theme="light"]) .dg-ok{color:#86efac}:root:not([data-theme="light"]) .dg-wait{color:#fcd34d}:root:not([data-theme="light"]) .dg-no{color:#fca5a5}}',
    ':root[data-theme="dark"] .dg-ok{color:#86efac}:root[data-theme="dark"] .dg-wait{color:#fcd34d}:root[data-theme="dark"] .dg-no{color:#fca5a5}',
    '.dg-err{margin:0;font-size:.78rem;color:var(--color-danger,#b91c1c)}.dg-msg{margin:0;font-size:.78rem;color:var(--color-text-secondary,#6b7280)}',
    '.dg-why{margin:2px 0 0;padding-left:18px;font-size:.76rem;color:var(--color-text-body,#374151)}',
    '#dg-note,#dg-rnote{min-height:3.6rem}',
    '.dg-seg{display:grid;grid-auto-flow:column;grid-auto-columns:1fr;border:1px solid var(--color-border,#e5e7eb);border-radius:8px;overflow:hidden}',
    '.dg-seg button{border:0;background:transparent;color:var(--color-text-secondary,#6b7280);padding:8px 4px;min-height:40px;font:inherit;font-size:.8rem;cursor:pointer}',
    '.dg-seg button[aria-pressed=true]{background:var(--color-text-primary,#111827);color:var(--color-bg-primary,#fff);font-weight:600}',
    '.dg-cards{display:grid;gap:8px}.dg-card{border:1px solid var(--color-border,#e5e7eb);border-radius:10px;padding:10px;background:var(--color-card-bg,#fff);display:grid;gap:6px}',
    '.dg-card{color:var(--color-text-body,#374151);font-size:.82rem}.dg-card .dl-sub{display:block;font-size:.76rem;color:var(--color-text-secondary,#6b7280)}.dg-card h5{margin:0;font-size:.95rem;color:var(--color-text-primary,#111827);overflow-wrap:anywhere}.dg-sum{font-size:.74rem;color:var(--color-text-muted,#6b7280);overflow-wrap:anywhere}',
    '.dg-pager{display:flex;justify-content:space-between;align-items:center;gap:8px;font-size:.78rem}.dg-rep{display:grid;gap:6px;border-top:1px solid var(--color-border,#e5e7eb);padding-top:6px}',
    '.dg-rep label.dg-rad{display:flex;gap:8px;align-items:center;min-height:36px;font-size:.82rem}',
    '.dg-card button:focus-visible,.dg-seg button:focus-visible,.dg-row button:focus-visible{outline:2px solid var(--shr-acc,#4f46e5);outline-offset:2px}'
  ].join('\n');
  function style() {
    if (document.getElementById('dg-style')) return;
    var st = document.createElement('style'); st.id = 'dg-style'; st.textContent = CSS; document.head.appendChild(st);
  }

  function paint() {
    if (!box || !box.isConnected) return;
    var a = document.activeElement, act = a && box.contains(a) && a.getAttribute('data-dg'), id = act && a.getAttribute('data-id');
    box.innerHTML = sectionHtml();
    paintMsg();
    if (act) {
      var cand = box.querySelectorAll('[data-dg]'), again = null;
      for (var i = 0; i < cand.length; i++) if (cand[i].getAttribute('data-dg') === act && (!id || cand[i].getAttribute('data-id') === id)) again = cand[i];
      if (again) again.focus();
    }
  }
  function load() {
    return env.call('GET', '/designs/gallery/mine').then(function (r) { G.own = r.entries || []; G.loaded = true; paint(); })
      .catch(function (e) { G.loaded = true; fail(e); paint(); });
  }

  /* ---- Sheets (opaque, over the library; Escape closes, Tab stays inside) ---- */
  function openSheet(kind, trigger) {
    closeSheet(true);
    G.sheet = kind; opener = trigger || null;
    sheetEl = document.createElement('div');
    sheetEl.className = 'dl-sheet'; sheetEl.setAttribute('role', 'dialog'); sheetEl.setAttribute('aria-modal', 'true'); sheetEl.setAttribute('aria-labelledby', 'dg-sh-t');
    sheetEl.addEventListener('click', onSheetClick); sheetEl.addEventListener('input', onSheetInput);
    sheetEl.addEventListener('change', onSheetChange); sheetEl.addEventListener('keydown', onSheetKey);
    env.host().appendChild(sheetEl);
  }
  function closeSheet(quiet) {
    clearTimeout(searchT);
    G.sheet = '';
    if (sheetEl) { sheetEl.remove(); sheetEl = null; }
    if (quiet) return;
    if (opener && opener.isConnected) opener.focus();
    else { var b = box && box.querySelector('[data-dg="open-publish"]'); if (b) b.focus(); }
    opener = null;
  }
  function shell(title, body, foot) {
    return '<div class="dl-sh"><h3 id="dg-sh-t" tabindex="-1">' + esc(title) + '</h3><button type="button" class="dl-btn" data-dgs="close">Close</button></div>' +
      '<div class="dl-sb">' + body + '</div>' + (foot ? '<div class="dl-sf">' + esc(foot) + '</div>' : '');
  }
  function focusHeading() { var h = sheetEl && sheetEl.querySelector('#dg-sh-t'); if (h) h.focus(); }
  function onSheetKey(e) {
    if (e.key === 'Escape') { e.stopPropagation(); e.preventDefault(); closeSheet(); return; }
    if (e.key !== 'Tab') return;
    var f = sheetEl.querySelectorAll('button:not([disabled]),input,textarea,select'), first = f[0], last = f[f.length - 1];
    if (e.shiftKey && document.activeElement === first) { last.focus(); e.preventDefault(); }
    else if (!e.shiftKey && document.activeElement === last) { first.focus(); e.preventDefault(); }
  }

  /* ---- Share to the gallery (and Update in gallery) ---- */
  function publishHtml() {
    var p = G.pub, updating = !!p.entryId, choices = updating ? env.mine().filter(function (d) { return d.id === p.designId; }) : freeDesigns(env.mine(), G.own);
    if (!choices.length) return shell('Share to this server’s gallery', '<p class="dl-ok">All of your designs are already in the gallery.</p>', '');
    var h = '<label class="dl-lab" for="dg-design">Which design</label><select id="dg-design" class="dl-in" data-dgs="design"' + (updating ? ' disabled' : '') + '>' +
      choices.map(function (d) { return '<option value="' + esc(d.id) + '"' + (d.id === p.designId ? ' selected' : '') + '>' + esc(d.name) + ' (' + esc(env.kindLabel(d.kind)) + ')</option>'; }).join('') + '</select>' +
      '<label class="dl-lab" for="dg-name">Name in the gallery</label><input id="dg-name" class="dl-in" maxlength="60" autocomplete="off">' +
      '<label class="dl-lab" for="dg-note">A short note <span id="dg-cnt" class="dl-sub"></span></label><textarea id="dg-note" rows="2" cols="1" maxlength="140" placeholder="What is it for?"></textarea>' +
      '<label class="dl-lab" for="dg-maker">Shown as</label><input id="dg-maker" class="dl-in" maxlength="100" autocomplete="off" placeholder="Your display name on this server">' +
      '<p class="dl-ok">Leave “Shown as” empty to use your display name. Anyone who owns a campaign here can browse and copy it. Copies are independent, so changing yours later does not change theirs.</p>' +
      '<p class="dl-err" role="alert" id="dg-perr"></p>' +
      '<div class="dl-row"><button type="button" class="dl-btn pri" data-dgs="publish"' + (G.busy ? ' disabled' : '') + '>' + (updating ? 'Update in gallery' : 'Share it') + '</button><button type="button" class="dl-btn" data-dgs="close">Cancel</button></div>';
    return shell(updating ? 'Update in gallery' : 'Share to this server’s gallery', h, 'Names and notes are checked when you share. Anything held for review is shown to you as Waiting for review.');
  }
  function paintPublish(focus) {
    sheetEl.innerHTML = publishHtml();
    var n = sheetEl.querySelector('#dg-name'), t = sheetEl.querySelector('#dg-note'), m = sheetEl.querySelector('#dg-maker'), err = sheetEl.querySelector('#dg-perr');
    if (n) { n.value = G.pub.name; t.value = G.pub.note; m.value = G.pub.maker; countNote(); }
    if (err) err.textContent = G.pub.err;
    if (focus) focusHeading();
  }
  function countNote() {
    var t = sheetEl && sheetEl.querySelector('#dg-note'), c = sheetEl && sheetEl.querySelector('#dg-cnt');
    if (t && c) c.textContent = t.value.length + ' / 140';
  }
  function openPublish(trigger, entry) {
    var free = entry ? [] : freeDesigns(env.mine(), G.own);
    var cur = entry ? byId(env.mine(), entry.sourceDesignId) : (byId(free, env.selected()) || free[0] || null);
    G.pub = { designId: cur ? cur.id : '', entryId: entry ? entry.id : '', name: entry ? entry.name : (cur ? cur.name : ''), note: entry ? entry.note : '', maker: entry ? entry.makerName : '', err: '' };
    openSheet('publish', trigger); paintPublish(true);
  }
  function readPublish() {
    var n = sheetEl.querySelector('#dg-name'), t = sheetEl.querySelector('#dg-note'), m = sheetEl.querySelector('#dg-maker');
    if (n) { G.pub.name = n.value; G.pub.note = t.value; G.pub.maker = m.value; }
  }
  function sendPublish() {
    readPublish();
    var p = G.pub, name = p.name.trim();
    if (!name) { G.pub.err = 'Give it a name first.'; paintPublish(); return; }
    var body = { name: name, note: p.note.trim(), makerName: p.maker.trim() };
    G.busy = true; paintPublish();
    var req = p.entryId ? env.call('PUT', '/designs/' + encodeURIComponent(p.designId) + '/gallery', body) : env.call('POST', '/designs/' + encodeURIComponent(p.designId) + '/gallery', body);
    req.then(function (m) {
      G.busy = false; closeSheet();
      say(m.status === 'visible' ? '“' + m.name + '” is in the gallery.' : '“' + m.name + '” is waiting for review. The reasons are in your gallery entries.');
      return load();
    }).catch(function (e) { G.busy = false; G.pub.err = e.message || 'Could not share it. Try again.'; paintPublish(); });
  }

  /* ---- Browse shared designs ---- */
  function browseShell() {
    return shell('Shared designs on this server',
      '<div id="dg-b-ctl"></div><label class="dl-lab" for="dg-b-q">Search designs</label><input id="dg-b-q" class="dl-in" type="search" placeholder="Search by name or maker" autocomplete="off">' +
      '<p class="dl-ok" id="dg-b-cnt" role="status" aria-live="polite"></p><div id="dg-b-list" class="dg-cards"></div><div id="dg-b-pg"></div>' +
      '<p class="dl-err" role="alert" id="dg-b-err"></p>', 'Copies are independent. If the maker changes theirs later, your copy stays as it is.');
  }
  function controlsHtml() {
    var b = G.br, sys = env.systemName(env.system());
    return '<div class="dg-seg" role="group" aria-label="Which game systems" style="margin-bottom:6px"><button type="button" data-dgs="sys" data-v="cur" aria-pressed="' + !b.all + '">' + esc(sys) + ' only</button>' +
      '<button type="button" data-dgs="sys" data-v="all" aria-pressed="' + b.all + '">All systems</button></div>' +
      '<div class="dg-seg" role="group" aria-label="Kind of design">' + KIND_FILTERS.map(function (k) {
        return '<button type="button" data-dgs="kind" data-v="' + esc(k[0]) + '" aria-pressed="' + (b.kind === k[0]) + '">' + esc(k[1]) + '</button>';
      }).join('') + '</div>';
  }
  function cardHtml(c) {
    var b = G.br, h = '<article class="dg-card" aria-label="' + esc(c.name) + '"><h5>' + esc(c.name) + '</h5><span class="dl-sub">by ' + esc(c.makerName) + (c.mine ? ' (you)' : '') + '</span>' +
      '<div><span class="dl-tag">' + esc(env.kindLabel(c.kind)) + '</span><span class="dl-sub" style="display:inline">Made for ' + esc(env.systemName(c.systemId)) + '</span></div>';
    if (c.note) h += '<span>' + esc(c.note) + '</span>';
    if (c.summary) h += '<span class="dg-sum">' + esc(c.summary) + '</span>';
    cardNotes(c, env.system(), env.systemName).forEach(function (n) { h += '<div class="dl-note" role="note">' + esc(n) + '</div>'; });
    if (c.mine) return h + '<div class="dl-acts"><span class="dl-tag">Yours</span></div></article>';
    var copied = !!b.copied[c.id];
    h += '<div class="dl-row"><button type="button" class="dl-btn' + (copied ? '' : ' pri') + '" data-dgs="copy" data-id="' + esc(c.id) + '"' + (copied ? ' disabled' : '') + '>' + (copied ? 'In my designs' : 'Copy to my campaign') + '</button></div>';
    if (b.report && b.report.id === c.id) {
      h += '<div class="dg-rep" role="group" aria-label="Report ' + esc(c.name) + '"><span class="dl-lab">What is wrong with it</span>' + REPORT_REASONS.map(function (r) {
        return '<label class="dg-rad"><input type="radio" name="dg-reason" value="' + esc(r[0]) + '"' + (b.report.reason === r[0] ? ' checked' : '') + '> ' + esc(r[1]) + '</label>';
      }).join('') + '<label class="dl-lab" for="dg-rnote">A note, if you like</label><textarea id="dg-rnote" rows="2" cols="1" maxlength="140"></textarea>' +
        '<p class="dl-err" role="alert" id="dg-rerr"></p><div class="dl-row"><button type="button" class="dl-btn pri" data-dgs="report-send">Send report</button><button type="button" class="dl-btn" data-dgs="report-cancel">Cancel</button></div></div>';
    } else {
      h += '<div class="dl-acts">' + (c.reported ? '<span class="dl-ok">You reported this. Thank you.</span>' :
        '<button type="button" class="dl-bad" data-dgs="report-open" data-id="' + esc(c.id) + '" aria-label="Report ' + esc(c.name) + '">Report</button>') + '</div>';
    }
    return h + '</article>';
  }
  function paintBrowse() {
    if (!sheetEl || G.sheet !== 'browse') return;
    var b = G.br, d = b.data, a = document.activeElement, act = a && sheetEl.contains(a) && a.getAttribute('data-dgs'), v = act && a.getAttribute('data-v'), id = act && a.getAttribute('data-id');
    sheetEl.querySelector('#dg-b-ctl').innerHTML = controlsHtml();
    var list = sheetEl.querySelector('#dg-b-list'), cnt = sheetEl.querySelector('#dg-b-cnt'), pg = sheetEl.querySelector('#dg-b-pg');
    sheetEl.querySelector('#dg-b-err').textContent = b.err;
    if (!d) { cnt.textContent = b.err ? '' : 'Loading…'; list.innerHTML = ''; pg.innerHTML = ''; return; }
    cnt.textContent = countLine(d.total, b.all, env.systemName(d.systemId));
    list.innerHTML = d.entries.length ? d.entries.map(cardHtml).join('') :
      '<p class="dl-ok">Nothing here' + (b.q ? ' matches your search' : '') + '.' + (b.all ? '' : ' Try All systems.') + '</p>';
    var rn = list.querySelector('#dg-rnote'), re = list.querySelector('#dg-rerr');
    if (rn) rn.value = b.note;
    if (re) re.textContent = b.reportErr;
    var pi = pageInfo(d.total, d.page, d.perPage);
    pg.innerHTML = pi.pages > 1 ? '<div class="dg-pager"><button type="button" class="dl-btn" data-dgs="prev"' + (pi.hasPrev ? '' : ' disabled') + '>Previous</button><span>' + pi.from + '–' + pi.to + ' of ' + d.total +
      '</span><button type="button" class="dl-btn" data-dgs="next"' + (pi.hasNext ? '' : ' disabled') + '>Next</button></div>' : '';
    if (act) {
      var cand = sheetEl.querySelectorAll('[data-dgs]'), again = null;
      for (var i = 0; i < cand.length; i++) if (cand[i].getAttribute('data-dgs') === act && cand[i].getAttribute('data-v') === v && cand[i].getAttribute('data-id') === id) again = cand[i];
      if (again) again.focus();
    }
  }
  function loadBrowse() {
    var b = G.br, q = b.q;
    env.call('GET', '/designs/gallery' + browseQuery(b)).then(function (d) {
      if (q !== b.q) return;
      b.data = d; b.err = ''; b.page = d.page; paintBrowse();
    }).catch(function (e) { b.err = e.message || 'Could not load the gallery. Try again.'; paintBrowse(); });
  }
  function openBrowse(trigger) {
    G.br.data = null; G.br.err = ''; G.br.report = null; G.br.reportErr = '';
    openSheet('browse', trigger);
    sheetEl.innerHTML = browseShell();
    var q = sheetEl.querySelector('#dg-b-q'); q.value = G.br.q;
    paintBrowse(); focusHeading(); loadBrowse();
  }
  function copyCard(id) {
    var c = byId((G.br.data && G.br.data.entries) || [], id); if (!c) return;
    env.call('POST', '/designs/gallery/' + encodeURIComponent(c.id) + '/copy').then(function () {
      G.br.copied[c.id] = true; G.br.err = ''; paintBrowse(); env.reload();
      var cnt = sheetEl && sheetEl.querySelector('#dg-b-cnt'); if (cnt) cnt.textContent = '“' + c.name + '” was added to your designs.';
    }).catch(function (e) { G.br.err = e.message; paintBrowse(); });
  }
  function sendReport() {
    var b = G.br, rp = b.report, c = rp && byId((b.data && b.data.entries) || [], rp.id); if (!c) return;
    var n = sheetEl.querySelector('#dg-rnote'); if (n) b.note = n.value;
    env.call('POST', '/designs/gallery/' + encodeURIComponent(c.id) + '/report', { reason: rp.reason, note: b.note.trim() }).then(function () {
      c.reported = true; b.report = null; b.note = ''; b.reportErr = ''; paintBrowse();
    }).catch(function (e) { b.reportErr = e.message || 'Could not send the report.'; paintBrowse(); });
  }

  /* ---- Events ---- */
  function onSheetClick(e) {
    var btn = e.target.closest && e.target.closest('button'); if (!btn) return;
    var act = btn.getAttribute('data-dgs'), v = btn.getAttribute('data-v'), id = btn.getAttribute('data-id'), b = G.br;
    if (act === 'close') closeSheet();
    else if (act === 'publish') sendPublish();
    else if (act === 'sys') { b.all = v === 'all'; b.page = 1; b.data = null; paintBrowse(); loadBrowse(); }
    else if (act === 'kind') { b.kind = kindFilter(v); b.page = 1; b.data = null; paintBrowse(); loadBrowse(); }
    else if (act === 'prev' || act === 'next') { b.page = Math.max(1, b.page + (act === 'next' ? 1 : -1)); loadBrowse(); }
    else if (act === 'copy') copyCard(id);
    else if (act === 'report-open') { b.report = { id: byId((b.data && b.data.entries) || [], id) ? id : '', reason: 'other' }; b.note = ''; b.reportErr = ''; paintBrowse(); var t = sheetEl.querySelector('#dg-rnote'); if (t) t.focus(); }
    else if (act === 'report-cancel') { b.report = null; b.reportErr = ''; paintBrowse(); }
    else if (act === 'report-send') sendReport();
  }
  function onSheetInput(e) {
    var t = e.target;
    if (t.id === 'dg-b-q') {
      G.br.q = t.value.trim(); clearTimeout(searchT);
      searchT = setTimeout(function () { G.br.page = 1; loadBrowse(); }, 250);
    } else if (t.id === 'dg-note') countNote();
    else if (t.id === 'dg-rnote') G.br.note = t.value;
  }
  function onSheetChange(e) {
    var t = e.target;
    if (t.name === 'dg-reason' && G.br.report) G.br.report.reason = reportReason(t.value);
    else if (t.id === 'dg-design') {
      readPublish();
      var d = byId(env.mine(), t.value);
      if (d) { G.pub.designId = d.id; G.pub.name = d.name; paintPublish(); var s = sheetEl.querySelector('#dg-design'); if (s) s.focus(); }
    }
  }
  function onBoxClick(e) {
    var btn = e.target.closest && e.target.closest('button'); if (!btn || !box.contains(btn)) return;
    var act = btn.getAttribute('data-dg'), id = btn.getAttribute('data-id');
    if (!act) return;
    var entry = byId(G.own, id);
    if (act === 'open-publish') openPublish(btn, null);
    else if (act === 'open-browse') openBrowse(btn);
    else if (act === 'update' && entry) openPublish(btn, entry);
    else if (act === 'unpublish' && entry) { G.ask = entry.id; paint(); }
    else if (act === 'unpublish-no') { G.ask = ''; paint(); }
    else if (act === 'unpublish-ok' && entry) {
      env.call('DELETE', '/designs/gallery/' + encodeURIComponent(entry.id)).then(function () {
        G.ask = ''; say('“' + entry.name + '” is off the gallery. Copies already made stay with their owners.'); return load();
      }).catch(function (x) { G.ask = ''; fail(x); paint(); });
    }
  }

  /* ---- Mounting ---- */
  // attach is called by the library each time its panel is built. env gives the
  // library's own helpers so this file stays free of its internals.
  function attach(host, e) {
    style();
    env = e;
    if (sheetEl) closeSheet(true);
    if (!host._dg) { host._dg = 1; host.addEventListener('click', onBoxClick); }
    box = host;
    paint();
    load();
  }
  // redraw puts the section back after the library redraws its own markup.
  function redraw(host) { if (host && env) { box = host; if (!host._dg) { host._dg = 1; host.addEventListener('click', onBoxClick); } paint(); } }

  window.DesignGallery = {
    attach: attach, redraw: redraw,
    util: { statusLabel: statusLabel, statusTone: statusTone, statusNote: statusNote, browseQuery: browseQuery, pageInfo: pageInfo, countLine: countLine,
      cardNotes: cardNotes, entryFor: entryFor, freeDesigns: freeDesigns, reportReason: reportReason, kindFilter: kindFilter }
  };
})();
